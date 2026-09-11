const {
  chmodSync,
  closeSync,
  constants,
  copyFileSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readSync,
  readdirSync,
  realpathSync,
  renameSync,
  rmdirSync,
  rmSync,
  unlinkSync,
  writeFileSync
} = require('node:fs')
const { execFileSync } = require('node:child_process')
const { createHash, randomBytes } = require('node:crypto')
const { hostname, tmpdir } = require('node:os')
const { basename, dirname, join, resolve } = require('node:path')
const {
  assertHermeticCargoEnvironment,
  assertNativeBinaryTarget,
  binaryName,
  environmentDigest,
  manifest,
  nativeBuildContextDigest,
  nativeComponentBuildEnvironment,
  nativeComponentBuildEnvironmentDigest,
  nativeToolchainIdentity,
  sha256File,
  sourceSetDigest,
  sourceTreeDigest,
  stageDirectory,
  targetContract,
  verifyPackagedComponents
} = require('./native-component-contract.cjs')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const {
  DEVELOPMENT_CACHE_ENVIRONMENT,
  openVerifiedDevelopmentCacheVolume,
  projectDevelopmentCacheEnvironment,
  requireDevelopmentNativeComponentRoot
} = require('./lib/development-cache-environment.cjs')
const { resolveDevelopmentRustToolchain } = require('./rust-native-toolchain-contract.cjs')
const { runNativeBuildCoordinator } = require('./native-build-probe-authority.cjs')

const repoRoot = join(__dirname, '..')
const DEVELOPMENT_BUILD_MARKER = 'analytix-native-development-build.json'
const MAXIMUM_DEVELOPMENT_BUILD_MARKER_BYTES = 256 * 1024

function parseBuildArgs(argv, env = process.env) {
  if (String(env.ANALYTIX_DATA_NATIVE_OUTPUT_DIR || '').trim()) {
    throw new Error('[data-native] ANALYTIX_DATA_NATIVE_OUTPUT_DIR is forbidden; use --output-dir for an explicit non-package build')
  }
  const options = {
    allPackageTargets: false,
    development: false,
    platform: process.platform,
    arch: process.arch,
    outputDir: ''
  }
  const seen = new Set()
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index]
    const separator = argument.indexOf('=')
    const name = separator >= 0 ? argument.slice(0, separator) : argument
    if (!['--all-package-targets', '--development', '--platform', '--arch', '--output-dir'].includes(name)) {
      throw new Error(`[data-native] Unknown argument: ${argument}`)
    }
    if (seen.has(name)) throw new Error(`[data-native] Duplicate argument: ${name}`)
    seen.add(name)
    if (name === '--all-package-targets') {
      if (separator >= 0) throw new Error('[data-native] --all-package-targets does not accept a value')
      options.allPackageTargets = true
      continue
    }
    if (name === '--development') {
      if (separator >= 0) throw new Error('[data-native] --development does not accept a value')
      options.development = true
      continue
    }
    const value = separator >= 0 ? argument.slice(separator + 1) : argv[++index]
    if (!value || value.startsWith('--')) throw new Error(`[data-native] ${name} requires a non-empty value`)
    if (name === '--platform') options.platform = value
    else if (name === '--arch') options.arch = value
    else options.outputDir = value
  }
  if (options.allPackageTargets && options.outputDir) {
    throw new Error('[data-native] --all-package-targets cannot use a shared output directory')
  }
  if (options.allPackageTargets && (seen.has('--platform') || seen.has('--arch'))) {
    throw new Error('[data-native] --all-package-targets cannot be combined with --platform or --arch')
  }
  if (options.allPackageTargets && options.development) {
    throw new Error('[data-native] --development cannot be combined with --all-package-targets')
  }
  if (options.development && options.outputDir) {
    throw new Error('[data-native] --development uses only the verified Analytix cache output')
  }
  return options
}

function packageTargets(options) {
  if (options.allPackageTargets) {
    if (process.platform === 'darwin') {
      return [targetContract('darwin', 'arm64'), targetContract('darwin', 'x64')]
    }
    if (process.platform === 'win32') {
      if (process.arch !== 'x64') throw new Error('[data-native] Windows x64 packaging requires a Windows x64 host')
      return [targetContract('win32', 'x64')]
    }
    if (process.platform === 'linux') {
      if (process.arch !== 'x64') throw new Error('[data-native] Linux x64 packaging requires a Linux x64 host')
      return [targetContract('linux', 'x64')]
    }
    throw new Error(`[data-native] Unsupported build host: ${process.platform}`)
  }
  const target = targetContract(
    options.platform,
    options.arch
  )
  if (target.platform !== process.platform) {
    throw new Error(`[data-native] Cross-platform build is not supported: host=${process.platform} target=${target.key}`)
  }
  if (target.platform !== 'darwin' && target.arch !== process.arch) {
    throw new Error(`[data-native] Cross-architecture build requires a target-native host: host=${process.arch} target=${target.arch}`)
  }
  return [target]
}

function requireLocalBuildAuthorityTarget(target) {
  if (!target || target.platform !== 'darwin') {
    throw new Error(`[data-native] Native execution authority is unavailable for target ${target?.key || '<unknown>'}`)
  }
  return target
}

function targetOutputDirectory(options, target, environment = process.env) {
  if (options.outputDir) return resolve(options.outputDir)
  if (options.development) {
    return join(requireDevelopmentNativeComponentRoot(environment), target.key)
  }
  return stageDirectory(repoRoot, target.platform, target.arch)
}

function assertNonEmptyFile(path, label) {
  if (!pathEntryExists(path)) throw new Error(`[data-native] Missing ${label}: ${path}`)
  const stat = lstatSync(path)
  if (stat.isSymbolicLink() || !stat.isFile() || stat.size <= 0) {
    throw new Error(`[data-native] ${label} is empty or not a file: ${path}`)
  }
}

function pathEntryExists(path) {
  try {
    lstatSync(path)
    return true
  } catch (error) {
    if (error && error.code === 'ENOENT') return false
    throw error
  }
}

function assertOutputDirectorySafe(outputDir) {
  if (!pathEntryExists(outputDir)) return
  const stat = lstatSync(outputDir)
  if (stat.isSymbolicLink() || !stat.isDirectory()) {
    throw new Error(`[data-native] Output stage must be a regular non-symlink directory: ${outputDir}`)
  }
}

function developmentDirectoryIdentity(path, stat) {
  return {
    path: resolve(path),
    dev: String(stat.dev),
    ino: String(stat.ino),
    uid: stat.uid,
    gid: stat.gid,
    mode: stat.mode & 0o7777
  }
}

function assertPrivateDevelopmentDirectory(path, volumeSnapshot, label) {
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  const stat = lstatSync(path)
  if (!stat.isDirectory() || stat.isSymbolicLink() ||
    realpathSync(path) !== resolve(path) || (stat.mode & 0o777) !== 0o700 ||
    (uid !== null && stat.uid !== uid) ||
    String(stat.dev) !== volumeSnapshot?.paths?.cacheMount?.dev) {
    throw new Error(`[data-native] ${label} is not authoritative`)
  }
  return developmentDirectoryIdentity(path, stat)
}

function sameDevelopmentDirectoryIdentity(left, right) {
  return left.path === right.path &&
    left.dev === right.dev &&
    left.ino === right.ino &&
    left.uid === right.uid &&
    left.gid === right.gid &&
    left.mode === right.mode
}

function assertPrivateDevelopmentNativeRoot(root, volumeSnapshot, expected = null) {
  const current = [
    assertPrivateDevelopmentDirectory(
      dirname(root),
      volumeSnapshot,
      'Development cache root'
    ),
    assertPrivateDevelopmentDirectory(
      root,
      volumeSnapshot,
      'Development native cache root'
    )
  ]
  if (expected && (expected.length !== current.length ||
    current.some((identity, index) =>
      !sameDevelopmentDirectoryIdentity(identity, expected[index])))) {
    throw new Error('[data-native] Development native cache root changed during the build')
  }
  return current
}

function assertPrivateDevelopmentTempRoot(volumeSnapshot) {
  const expected = DEVELOPMENT_CACHE_ENVIRONMENT.GOTMPDIR
  if (resolve(tmpdir()) !== expected) {
    throw new Error('[data-native] Development temp root is not authoritative')
  }
  return assertPrivateDevelopmentDirectory(
    expected,
    volumeSnapshot,
    'Development temp root'
  )
}

function assertPrivateDevelopmentOutput(outputDir, nativeRoot, volumeSnapshot) {
  if (dirname(outputDir) !== nativeRoot) {
    throw new Error('[data-native] Development native output escaped its fixed cache root')
  }
  if (!pathEntryExists(outputDir)) return
  const stat = lstatSync(outputDir)
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  if (!stat.isDirectory() || stat.isSymbolicLink() ||
    realpathSync(outputDir) !== resolve(outputDir) ||
    (stat.mode & 0o777) !== 0o700 ||
    (uid !== null && stat.uid !== uid) ||
    String(stat.dev) !== volumeSnapshot?.paths?.cacheMount?.dev) {
    throw new Error('[data-native] Development native output is not authoritative')
  }
}

function syncDirectory(path) {
  const descriptor = openSync(path, constants.O_RDONLY)
  try {
    fsyncSync(descriptor)
  } finally {
    closeSync(descriptor)
  }
}

function writeExclusivePrivateJSON(path, value) {
  const bytes = `${JSON.stringify(value, null, 2)}\n`
  const descriptor = openSync(
    path,
    constants.O_WRONLY |
      constants.O_CREAT |
      constants.O_EXCL |
      (constants.O_NOFOLLOW || 0),
    0o600
  )
  try {
    writeFileSync(descriptor, bytes, 'utf8')
    fsyncSync(descriptor)
  } finally {
    closeSync(descriptor)
  }
  syncDirectory(dirname(path))
}

function readBoundedStableRegularFile(path, expected, maximumBytes) {
  const descriptor = openSync(
    path,
    constants.O_RDONLY | (constants.O_NOFOLLOW || 0)
  )
  try {
    const before = fstatSync(descriptor)
    if (!before.isFile() || before.isSymbolicLink() ||
      before.dev !== expected.dev || before.ino !== expected.ino ||
      before.nlink !== expected.nlink || before.size !== expected.size ||
      before.size <= 0 || before.size > maximumBytes) {
      throw new Error(`[data-native] Bounded file identity is invalid: ${path}`)
    }
    const bytes = Buffer.alloc(before.size + 1)
    let offset = 0
    while (offset < bytes.length) {
      const count = readSync(
        descriptor,
        bytes,
        offset,
        bytes.length - offset,
        offset
      )
      if (count === 0) break
      offset += count
    }
    const after = fstatSync(descriptor)
    const current = lstatSync(path)
    if (offset !== before.size ||
      after.dev !== before.dev || after.ino !== before.ino ||
      after.nlink !== before.nlink || after.size !== before.size ||
      after.mtimeMs !== before.mtimeMs || after.ctimeMs !== before.ctimeMs ||
      current.dev !== after.dev || current.ino !== after.ino ||
      current.nlink !== after.nlink || current.size !== after.size ||
      current.mtimeMs !== after.mtimeMs || current.ctimeMs !== after.ctimeMs) {
      throw new Error(`[data-native] Bounded file changed while reading: ${path}`)
    }
    return bytes.subarray(0, offset)
  } finally {
    closeSync(descriptor)
  }
}

function removePrivateBuildDirectory(path, expected, volumeSnapshot, label) {
  const current = assertPrivateDevelopmentDirectory(path, volumeSnapshot, label)
  if (!sameDevelopmentDirectoryIdentity(expected, current)) {
    throw new Error(`[data-native] ${label} changed before cleanup`)
  }
  rmSync(path, { recursive: true, force: false })
}

function inspectTargetBuildLockArtifact(lockPath, target, expectedOwner = null) {
  const directory = lstatSync(lockPath)
  const parent = lstatSync(dirname(lockPath))
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  if (directory.isSymbolicLink() || !directory.isDirectory() ||
    realpathSync(lockPath) !== resolve(lockPath) ||
    (directory.mode & 0o777) !== 0o700 ||
    directory.dev !== parent.dev ||
    (uid !== null && directory.uid !== uid) ||
    readdirSync(lockPath).sort().join(',') !== 'owner.json') {
    throw new Error(`[data-native] Target build lock directory is invalid: ${lockPath}`)
  }
  const ownerPath = join(lockPath, 'owner.json')
  const ownerStat = lstatSync(ownerPath)
  if (ownerStat.isSymbolicLink() || !ownerStat.isFile() ||
    ownerStat.nlink !== 1 || ownerStat.size <= 0 || ownerStat.size > 4096 ||
    (ownerStat.mode & 0o777) !== 0o600 || ownerStat.dev !== directory.dev ||
    (uid !== null && ownerStat.uid !== uid)) {
    throw new Error(`[data-native] Target build lock owner is invalid: ${ownerPath}`)
  }
  const ownerBytes = readBoundedStableRegularFile(ownerPath, ownerStat, 4096)
  const owner = parseStrictJsonObject(ownerBytes, {
    maxBytes: 4096,
    maxDepth: 2,
    maxTokens: 32,
    maxStringBytes: 256,
    maxNumberBytes: 16,
    integerOnly: true
  })
  if (
    !exactObjectKeys(owner, [
      'schemaVersion',
      'pid',
      'hostname',
      'nonce',
      'targetKey',
      'issuedAt'
    ]) ||
    owner.schemaVersion !== 1 ||
    !Number.isSafeInteger(owner.pid) || owner.pid <= 0 ||
    owner.hostname !== hostname() ||
    owner.targetKey !== target.key ||
    typeof owner.nonce !== 'string' || !/^[a-f0-9]{32}$/u.test(owner.nonce) ||
    typeof owner.issuedAt !== 'string' ||
    !Number.isFinite(Date.parse(owner.issuedAt)) ||
    new Date(owner.issuedAt).toISOString() !== owner.issuedAt ||
    ownerBytes.toString('utf8') !== `${JSON.stringify(owner, null, 2)}\n` ||
    (expectedOwner && JSON.stringify(owner) !== JSON.stringify(expectedOwner))
  ) {
    throw new Error(`[data-native] Target build lock ownership is invalid: ${lockPath}`)
  }
  return { directory, owner, ownerPath, ownerStat }
}

function targetBuildLockOwnerIsDead(owner) {
  if (owner.pid === process.pid) return false
  try {
    process.kill(owner.pid, 0)
    return false
  } catch (error) {
    return Boolean(error && error.code === 'ESRCH')
  }
}

function removeInspectedTargetBuildLockArtifact(lockPath, target, expected) {
  const current = inspectTargetBuildLockArtifact(lockPath, target, expected.owner)
  if (current.directory.dev !== expected.directory.dev ||
    current.directory.ino !== expected.directory.ino ||
    current.ownerStat.dev !== expected.ownerStat.dev ||
    current.ownerStat.ino !== expected.ownerStat.ino) {
    throw new Error(`[data-native] Target build lock changed before cleanup: ${lockPath}`)
  }
  unlinkSync(current.ownerPath)
  rmdirSync(lockPath)
}

function reconcileTargetBuildLockLifecycle(lockPath, target) {
  for (const suffix of ['retired', 'released']) {
    const lifecyclePath = `${lockPath}.${suffix}`
    if (!pathEntryExists(lifecyclePath)) continue
    const stat = lstatSync(lifecyclePath)
    const parent = lstatSync(dirname(lifecyclePath))
    const uid = typeof process.getuid === 'function' ? process.getuid() : null
    if (stat.isSymbolicLink() || !stat.isDirectory() ||
      realpathSync(lifecyclePath) !== resolve(lifecyclePath) ||
      (stat.mode & 0o777) !== 0o700 ||
      stat.dev !== parent.dev ||
      (uid !== null && stat.uid !== uid)) {
      throw new Error(`[data-native] Target build lock lifecycle is invalid: ${lifecyclePath}`)
    }
    const entries = readdirSync(lifecyclePath)
    if (entries.length === 0) {
      rmdirSync(lifecyclePath)
      continue
    }
    const inspected = inspectTargetBuildLockArtifact(lifecyclePath, target)
    if (!targetBuildLockOwnerIsDead(inspected.owner)) {
      throw new Error(`[data-native] Target build lock lifecycle owner is still active: ${lifecyclePath}`)
    }
    removeInspectedTargetBuildLockArtifact(lifecyclePath, target, inspected)
  }
}

function recoverDeadLocalTargetBuildLock(lockPath, target) {
  try {
    const inspected = inspectTargetBuildLockArtifact(lockPath, target)
    if (!targetBuildLockOwnerIsDead(inspected.owner)) return false
    const retiredPath = `${lockPath}.retired`
    if (pathEntryExists(retiredPath)) return false
    renameSync(lockPath, retiredPath)
    const retired = inspectTargetBuildLockArtifact(
      retiredPath,
      target,
      inspected.owner
    )
    if (retired.directory.dev !== inspected.directory.dev ||
      retired.directory.ino !== inspected.directory.ino ||
      retired.ownerStat.dev !== inspected.ownerStat.dev ||
      retired.ownerStat.ino !== inspected.ownerStat.ino) {
      return false
    }
    removeInspectedTargetBuildLockArtifact(retiredPath, target, retired)
    syncDirectory(dirname(lockPath))
    return true
  } catch {
    return false
  }
}

function acquireTargetBuildLock(
  outputDir,
  target,
  allowDeadOwnerRecovery = true,
  allowParentCreation = true
) {
  if (allowParentCreation) {
    mkdirSync(dirname(outputDir), { recursive: true })
  } else if (!pathEntryExists(dirname(outputDir))) {
    throw new Error(`[data-native] Verified target build parent is unavailable: ${dirname(outputDir)}`)
  }
  const lockPath = `${outputDir}.build.lock`
  reconcileTargetBuildLockLifecycle(lockPath, target)
  const owner = {
    schemaVersion: 1,
    pid: process.pid,
    hostname: hostname(),
    nonce: randomBytes(16).toString('hex'),
    targetKey: target.key,
    issuedAt: new Date().toISOString()
  }
  let created = false
  try {
    mkdirSync(lockPath, { mode: 0o700 })
    created = true
    writeExclusivePrivateJSON(join(lockPath, 'owner.json'), owner)
  } catch (error) {
    if (created) {
      try {
        if (readdirSync(lockPath).length === 0) rmdirSync(lockPath)
      } catch {
        // Preserve an ambiguous lock directory for explicit recovery.
      }
    }
    if (
      allowDeadOwnerRecovery && !created && error?.code === 'EEXIST' &&
      recoverDeadLocalTargetBuildLock(lockPath, target)
    ) {
      return acquireTargetBuildLock(outputDir, target, false, allowParentCreation)
    }
    throw new Error(
      `[data-native] Cannot acquire exclusive target build lock ${lockPath}: ${error instanceof Error ? error.message : String(error)}`
    )
  }
  return { lockPath, owner, target }
}

function assertTargetBuildLock(lock) {
  return inspectTargetBuildLockArtifact(lock.lockPath, lock.target, lock.owner)
}

function releaseTargetBuildLock(lock) {
  const before = assertTargetBuildLock(lock)
  const releasedPath = `${lock.lockPath}.released`
  if (pathEntryExists(releasedPath)) {
    throw new Error('[data-native] Released target build lock path is already occupied')
  }
  renameSync(lock.lockPath, releasedPath)
  const released = inspectTargetBuildLockArtifact(releasedPath, lock.target, lock.owner)
  if (released.directory.dev !== before.directory.dev ||
    released.directory.ino !== before.directory.ino ||
    released.ownerStat.dev !== before.ownerStat.dev ||
    released.ownerStat.ino !== before.ownerStat.ino) {
    throw new Error('[data-native] Released target build lock changed identity')
  }
  removeInspectedTargetBuildLockArtifact(releasedPath, lock.target, released)
  syncDirectory(dirname(lock.lockPath))
}

function readBoundedGenerationFile(path, maximumBytes) {
  const stat = lstatSync(path)
  if (stat.isSymbolicLink() || !stat.isFile() || stat.size <= 0 || stat.size > maximumBytes) {
    throw new Error(`[data-native] Generation metadata is invalid: ${path}`)
  }
  const body = readFileSync(path)
  if (body.length !== stat.size) {
    throw new Error(`[data-native] Generation metadata changed while reading: ${path}`)
  }
  return body
}

function parseOuterGenerationReceipt(path) {
  const body = readBoundedGenerationFile(path, 4 * 1024 * 1024)
  const receipt = parseStrictJsonObject(body, {
    maxBytes: 4 * 1024 * 1024,
    maxDepth: 4,
    maxTokens: 128,
    maxStringBytes: 1024,
    maxNumberBytes: 32,
    integerOnly: true
  })
  const fields = [
    'schemaVersion',
    'format',
    'generationId',
    'inventoryDigest',
    'directoryMode',
    'inventoryType',
    'inventoryMode',
    'receiptType',
    'receiptMode',
    'fileCount',
    'totalBytes'
  ]
  if (
    !exactObjectKeys(receipt, fields) || receipt.schemaVersion !== 1 ||
    receipt.format !== 'immutable_generation_v1' || !/^[0-9a-f]{64}$/u.test(receipt.generationId) ||
    !/^[0-9a-f]{64}$/u.test(receipt.inventoryDigest) || receipt.directoryMode !== 0o700 ||
    receipt.inventoryType !== 'regular_file' || receipt.inventoryMode !== 0o600 ||
    receipt.receiptType !== 'regular_file' || receipt.receiptMode !== 0o600 ||
    !Number.isSafeInteger(receipt.fileCount) || receipt.fileCount !== manifest.components.length + 3 ||
    !Number.isSafeInteger(receipt.totalBytes) || receipt.totalBytes <= 0
  ) {
    throw new Error(`[data-native] Generation receipt contract is invalid: ${path}`)
  }
  return { body, receipt }
}

function verifyPublishedGeneration(outputDir, target, response, verifyOptions) {
  const current = join(outputDir, 'current')
  const currentStat = lstatSync(current)
  if (currentStat.isSymbolicLink() || !currentStat.isDirectory()) {
    throw new Error(`[data-native] Published generation is invalid: ${current}`)
  }
  const inventory = readBoundedGenerationFile(join(current, 'inventory.v1.json'), 4 * 1024 * 1024)
  const outer = parseOuterGenerationReceipt(join(current, 'receipt.v1.json'))
  const inner = readBoundedGenerationFile(join(current, 'analytix-native-components-receipt.json'), 256 * 1024)
  if (
    outer.receipt.generationId !== response.generation_id ||
    sha256Bytes(inventory) !== response.inventory_sha256 ||
    sha256Bytes(outer.body) !== response.generation_receipt_sha256 ||
    sha256Bytes(inner) !== response.component_receipt_sha256
  ) {
    throw new Error('[data-native] Published generation does not match the Go publication receipt')
  }
  verifyPackagedComponents(current, target, verifyOptions)
  return current
}

function sha256Bytes(value) {
  return createHash('sha256').update(value).digest('hex')
}

function exactObjectKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value)
  if (keys.length !== expected.length) return false
  const allowed = new Set(expected)
  return keys.every((key) => allowed.has(key))
}

function releaseBinary(component, target, cargoTargetDirectory) {
  return join(cargoTargetDirectory, target.triple, 'release', binaryName(component, target.platform))
}

function cargoBuildArgs(component, target) {
  return ['build', '--release', '--frozen', '--target', target.triple, '--bin', component.binaryName]
}

function normalizeDarwinCodeSignature(path, target, codesignExecutable, exec = execFileSync) {
  if (target.platform !== 'darwin') return
  if (!codesignExecutable) throw new Error('[data-native] Pinned codesign executable is unavailable')
  exec(codesignExecutable, ['--force', '--sign', '-', '--timestamp=none', path], { stdio: 'inherit' })
  exec(codesignExecutable, ['--verify', '--strict', '--verbose=2', path], { stdio: 'inherit' })
}

function copyBuiltComponentExclusive(source, destination) {
  copyFileSync(source, destination, constants.COPYFILE_EXCL)
}

function buildComponent(
  component,
  target,
  outputDir,
  cargoBin,
  buildEnvironment,
  rustcBin,
  rustToolchain,
  cargoTargetDirectory,
  sourceSnapshotRoot,
  verifyCacheEffect
) {
  if (typeof verifyCacheEffect !== 'function') {
    throw new Error('[data-native] Development cache effect verifier is unavailable')
  }
  const componentRoot = join(sourceSnapshotRoot, component.sourceRoot)
  console.log(`[data-native] Building ${component.id} (${component.binaryName}) for ${target.key}`)
  const effectiveEnvironment = nativeComponentBuildEnvironment(component, target, {
    baseEnvironment: buildEnvironment,
    rustcExecutable: rustcBin,
    linkerExecutable: rustToolchain.tools.clang,
    archiverExecutable: rustToolchain.tools.ar,
    cargoTargetDirectory
  })
  const buildEnvironmentSha256 = nativeComponentBuildEnvironmentDigest(effectiveEnvironment)
  verifyCacheEffect()
  execFileSync(
    cargoBin,
    cargoBuildArgs(component, target),
    {
      cwd: componentRoot,
      stdio: 'inherit',
      env: effectiveEnvironment
    }
  )
  verifyCacheEffect()

  const source = releaseBinary(component, target, cargoTargetDirectory)
  assertNonEmptyFile(source, `${component.id} release binary`)
  verifyCacheEffect()
  mkdirSync(outputDir, { recursive: true })
  verifyCacheEffect()
  const destination = join(outputDir, binaryName(component, target.platform))
  copyBuiltComponentExclusive(source, destination)
  verifyCacheEffect()
  if (target.platform !== 'win32') chmodSync(destination, 0o700)
  verifyCacheEffect()
  normalizeDarwinCodeSignature(destination, target, rustToolchain.tools.codesign)
  verifyCacheEffect()
  const inspected = assertNativeBinaryTarget(destination, target)
  console.log(`[data-native] Copied ${component.id} -> ${destination} (${inspected.format}/${inspected.arch})`)
  return {
    id: component.id,
    binaryName: binaryName(component, target.platform),
    buildEnvironmentSha256,
    binarySha256: inspected.sha256,
    binarySize: inspected.size,
    payloadSha256: inspected.payloadSha256,
    payloadSize: inspected.payloadSize,
    format: inspected.format,
    arch: inspected.arch
  }
}

function developmentSourceComponents(repositoryRoot) {
  return manifest.components.map((component) => ({
    id: component.id,
    source_root: component.sourceRoot,
    source_digest: sourceTreeDigest(join(repositoryRoot, component.sourceRoot), repositoryRoot),
    cargo_lock_sha256: sha256File(join(repositoryRoot, component.sourceRoot, 'Cargo.lock'))
  }))
}

function developmentMarkerValid(marker, target) {
  const markerFields = [
    'schemaVersion',
    'kind',
    'classification',
    'publishable',
    'releaseEligible',
    'authorityUse',
    'targetKey',
    'targetTriple',
    'platform',
    'arch',
    'sourceSetSha256',
    'buildContextSha256',
    'buildEnvironmentSha256',
    'toolchain',
    'components'
  ]
  const toolchainFields = [
    'cargoExecutableSha256',
    'cargoVersion',
    'rustcExecutableSha256',
    'rustcVersion'
  ]
  const componentFields = [
    'id',
    'sourceDigest',
    'cargoLockSha256',
    'buildEnvironmentSha256',
    'binaryName',
    'binarySha256',
    'binarySize',
    'payloadSha256',
    'payloadSize',
    'format',
    'arch'
  ]
  const sha256 = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
  const positiveSize = (value) => Number.isSafeInteger(value) && value > 0
  return exactObjectKeys(marker, markerFields) &&
    marker.schemaVersion === 1 &&
    marker.kind === 'analytix_native_development_build' &&
    marker.classification === 'development_non_publishable' &&
    marker.publishable === false &&
    marker.releaseEligible === false &&
    marker.authorityUse === 'development_only' &&
    marker.targetKey === target.key &&
    marker.targetTriple === target.triple &&
    marker.platform === target.platform &&
    marker.arch === target.arch &&
    sha256(marker.sourceSetSha256) &&
    sha256(marker.buildContextSha256) &&
    sha256(marker.buildEnvironmentSha256) &&
    exactObjectKeys(marker.toolchain, toolchainFields) &&
    sha256(marker.toolchain.cargoExecutableSha256) &&
    typeof marker.toolchain.cargoVersion === 'string' &&
    marker.toolchain.cargoVersion.length > 0 &&
    marker.toolchain.cargoVersion.length <= 1024 &&
    sha256(marker.toolchain.rustcExecutableSha256) &&
    typeof marker.toolchain.rustcVersion === 'string' &&
    marker.toolchain.rustcVersion.length > 0 &&
    marker.toolchain.rustcVersion.length <= 1024 &&
    Array.isArray(marker.components) &&
    marker.components.length === manifest.components.length &&
    marker.components.every((component, index) => {
      const expected = manifest.components[index]
      return exactObjectKeys(component, componentFields) &&
        component.id === expected.id &&
        component.binaryName === binaryName(expected, target.platform) &&
        sha256(component.sourceDigest) &&
        sha256(component.cargoLockSha256) &&
        sha256(component.buildEnvironmentSha256) &&
        sha256(component.binarySha256) &&
        positiveSize(component.binarySize) &&
        sha256(component.payloadSha256) &&
        positiveSize(component.payloadSize) &&
        component.payloadSize <= component.binarySize &&
        component.format === target.format &&
        component.arch === target.arch
    })
}

function assertDevelopmentBuildDirectory(root, target, expectedMarker = null) {
  const directory = lstatSync(root)
  const parent = lstatSync(dirname(root))
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  if (!directory.isDirectory() || directory.isSymbolicLink() ||
    realpathSync(root) !== resolve(root) || (directory.mode & 0o777) !== 0o700 ||
    directory.dev !== parent.dev ||
    (uid !== null && directory.uid !== uid)) {
    throw new Error('[data-native] Development generation directory is unsafe')
  }
  const expectedNames = [
    ...manifest.components.map((component) => binaryName(component, target.platform)),
    DEVELOPMENT_BUILD_MARKER
  ].sort()
  const entries = readdirSync(root, { withFileTypes: true })
  const actualNames = entries.map((entry) => entry.name).sort()
  if (actualNames.length !== expectedNames.length ||
    actualNames.some((name, index) => name !== expectedNames[index]) ||
    entries.some((entry) => entry.isSymbolicLink() || !entry.isFile())) {
    throw new Error('[data-native] Development generation inventory is invalid')
  }
  const markerPath = join(root, DEVELOPMENT_BUILD_MARKER)
  const markerStat = lstatSync(markerPath)
  if (!markerStat.isFile() || markerStat.isSymbolicLink() ||
    markerStat.nlink !== 1 || markerStat.size <= 0 ||
    markerStat.size > MAXIMUM_DEVELOPMENT_BUILD_MARKER_BYTES ||
    (markerStat.mode & 0o777) !== 0o600 ||
    markerStat.dev !== directory.dev ||
    (uid !== null && markerStat.uid !== uid)) {
    throw new Error('[data-native] Development generation marker is unsafe')
  }
  const markerBytes = readBoundedStableRegularFile(
    markerPath,
    markerStat,
    MAXIMUM_DEVELOPMENT_BUILD_MARKER_BYTES
  )
  const marker = parseStrictJsonObject(markerBytes, {
    maxBytes: 256 * 1024,
    maxDepth: 16,
    maxTokens: 8192,
    maxStringBytes: 64 * 1024,
    maxNumberBytes: 32,
    integerOnly: true
  })
  if (!developmentMarkerValid(marker, target) ||
    markerBytes.toString('utf8') !== `${JSON.stringify(marker, null, 2)}\n` ||
    (expectedMarker && JSON.stringify(marker) !== JSON.stringify(expectedMarker))) {
    throw new Error('[data-native] Development generation marker is invalid')
  }
  const identities = [{ path: markerPath, stat: markerStat }]
  for (let index = 0; index < manifest.components.length; index += 1) {
    const component = manifest.components[index]
    const recorded = marker.components[index]
    const name = binaryName(component, target.platform)
    const path = join(root, name)
    const stat = lstatSync(path)
    if (!recorded || recorded.id !== component.id ||
      recorded.binaryName !== name ||
      !stat.isFile() || stat.isSymbolicLink() || stat.nlink !== 1 ||
      stat.size <= 0 || (stat.mode & 0o777) !== 0o700 ||
      stat.dev !== directory.dev || (uid !== null && stat.uid !== uid)) {
      throw new Error(`[data-native] Development generation component is unsafe: ${component.id}`)
    }
    const inspected = assertNativeBinaryTarget(path, target)
    if (inspected.sha256 !== recorded.binarySha256 ||
      inspected.size !== recorded.binarySize ||
      inspected.payloadSha256 !== recorded.payloadSha256 ||
      inspected.payloadSize !== recorded.payloadSize ||
      inspected.format !== recorded.format || inspected.arch !== recorded.arch) {
      throw new Error(`[data-native] Development generation component is invalid: ${component.id}`)
    }
    identities.push({ path, stat })
  }
  return { directory, identities, marker }
}

function removeVerifiedDevelopmentBuild(root, target) {
  const verified = assertDevelopmentBuildDirectory(root, target)
  for (const entry of verified.identities) {
    const current = lstatSync(entry.path)
    if (current.dev !== entry.stat.dev || current.ino !== entry.stat.ino ||
      current.nlink !== entry.stat.nlink || current.size !== entry.stat.size) {
      throw new Error('[data-native] Development generation changed before retirement')
    }
  }
  for (const entry of verified.identities) {
    unlinkSync(entry.path)
  }
  rmdirSync(root)
}

function publishDevelopmentBuild({
  outputDir,
  temporaryDir,
  target,
  sourceSetSha256,
  buildContextSha256,
  buildEnvironmentSha256,
  toolchain,
  sourceComponents,
  builtComponents,
  verifyBuildEffect = () => {},
  verifyPublicationEffect = () => {}
}) {
  const marker = {
    schemaVersion: 1,
    kind: 'analytix_native_development_build',
    classification: 'development_non_publishable',
    publishable: false,
    releaseEligible: false,
    authorityUse: 'development_only',
    targetKey: target.key,
    targetTriple: target.triple,
    platform: target.platform,
    arch: target.arch,
    sourceSetSha256,
    buildContextSha256,
    buildEnvironmentSha256,
    toolchain: {
      cargoExecutableSha256: toolchain.cargoExecutableSha256,
      cargoVersion: toolchain.cargoVersion,
      rustcExecutableSha256: toolchain.rustcExecutableSha256,
      rustcVersion: toolchain.rustcVersion
    },
    components: sourceComponents.map((source, index) => ({
      id: source.id,
      sourceDigest: source.source_digest,
      cargoLockSha256: source.cargo_lock_sha256,
      buildEnvironmentSha256: builtComponents[index].buildEnvironmentSha256,
      binaryName: builtComponents[index].binaryName,
      binarySha256: builtComponents[index].binarySha256,
      binarySize: builtComponents[index].binarySize,
      payloadSha256: builtComponents[index].payloadSha256,
      payloadSize: builtComponents[index].payloadSize,
      format: builtComponents[index].format,
      arch: builtComponents[index].arch
    }))
  }
  verifyBuildEffect()
  writeExclusivePrivateJSON(join(temporaryDir, DEVELOPMENT_BUILD_MARKER), marker)
  verifyBuildEffect()
  assertDevelopmentBuildDirectory(temporaryDir, target, marker)

  const previousPath = `${outputDir}.previous`
  let previous = null
  try {
    verifyPublicationEffect()
    if (pathEntryExists(previousPath)) {
      if (!pathEntryExists(outputDir)) {
        const previousOnly = assertDevelopmentBuildDirectory(previousPath, target)
        renameSync(previousPath, outputDir)
        const restored = assertDevelopmentBuildDirectory(outputDir, target)
        if (restored.directory.dev !== previousOnly.directory.dev ||
          restored.directory.ino !== previousOnly.directory.ino) {
          throw new Error('[data-native] Previous development generation restore changed identity')
        }
        syncDirectory(dirname(outputDir))
      } else {
        const current = assertDevelopmentBuildDirectory(outputDir, target)
        removeVerifiedDevelopmentBuild(previousPath, target)
        const currentAfterRetirement = assertDevelopmentBuildDirectory(outputDir, target)
        if (currentAfterRetirement.directory.dev !== current.directory.dev ||
          currentAfterRetirement.directory.ino !== current.directory.ino) {
          throw new Error('[data-native] Current development generation changed during reconciliation')
        }
        syncDirectory(dirname(outputDir))
      }
      verifyPublicationEffect()
    }

    if (pathEntryExists(outputDir)) {
      previous = assertDevelopmentBuildDirectory(outputDir, target)
      verifyPublicationEffect()
      renameSync(outputDir, previousPath)
      const moved = assertDevelopmentBuildDirectory(previousPath, target)
      if (moved.directory.dev !== previous.directory.dev ||
        moved.directory.ino !== previous.directory.ino) {
        throw new Error('[data-native] Previous development generation changed during retirement')
      }
      syncDirectory(dirname(outputDir))
      verifyPublicationEffect()
    }
    if (pathEntryExists(outputDir)) {
      throw new Error('[data-native] Development output was occupied during publication')
    }
    const staged = lstatSync(temporaryDir)
    verifyPublicationEffect()
    renameSync(temporaryDir, outputDir)
    const published = assertDevelopmentBuildDirectory(outputDir, target, marker)
    if (published.directory.dev !== staged.dev ||
      published.directory.ino !== staged.ino) {
      throw new Error('[data-native] Published development output identity changed')
    }
    syncDirectory(outputDir)
    syncDirectory(dirname(outputDir))
    verifyPublicationEffect()
    if (pathEntryExists(previousPath)) {
      removeVerifiedDevelopmentBuild(previousPath, target)
      syncDirectory(dirname(outputDir))
    }
    verifyPublicationEffect()
  } catch (error) {
    if (previous && !pathEntryExists(outputDir) && pathEntryExists(previousPath)) {
      try {
        verifyPublicationEffect()
        const restorable = assertDevelopmentBuildDirectory(previousPath, target)
        if (restorable.directory.dev !== previous.directory.dev ||
          restorable.directory.ino !== previous.directory.ino) {
          throw new Error('[data-native] Previous development generation changed before restore')
        }
        renameSync(previousPath, outputDir)
        const restored = assertDevelopmentBuildDirectory(outputDir, target)
        if (restored.directory.dev !== previous.directory.dev ||
          restored.directory.ino !== previous.directory.ino) {
          throw new Error('[data-native] Previous development generation changed during restore')
        }
        syncDirectory(dirname(outputDir))
        verifyPublicationEffect()
      } catch (recoveryError) {
        throw new AggregateError(
          [error, recoveryError],
          '[data-native] Development publication failed and trusted recovery was unavailable'
        )
      }
    }
    throw error
  }
  return join(outputDir, DEVELOPMENT_BUILD_MARKER)
}

function buildTarget(options, target) {
  requireLocalBuildAuthorityTarget(target)
  if (!options.development) {
    const outputDir = targetOutputDirectory(options, target)
    assertOutputDirectorySafe(outputDir)
    const response = runNativeBuildCoordinator({
      repositoryRoot: repoRoot,
      publicationRoot: outputDir,
      targetKey: target.key
    })
    const current = verifyPublishedGeneration(outputDir, target, response, {
      repoRoot,
      authorityUse: 'release',
      requireBuildIdentity: true,
      verifyBuildEnvironment: false
    })
    console.log(`[data-native] Published immutable target generation -> ${current}`)
    return
  }
  if (options.cacheVolume && process.env.NODE_ENV !== 'test') {
    throw new Error('[data-native] Cache-volume verifier override is test-only')
  }
  const cacheVolume = options.cacheVolume || openVerifiedDevelopmentCacheVolume()
  try {
    let volumeSnapshot = cacheVolume.verify()
    const nativeRoot = requireDevelopmentNativeComponentRoot(process.env)
    const nativeRootIdentity = assertPrivateDevelopmentNativeRoot(
      nativeRoot,
      volumeSnapshot
    )
    const tempRootIdentity = assertPrivateDevelopmentTempRoot(volumeSnapshot)
    let temporaryDir = ''
    let temporaryDirIdentity = null
    let cargoTargetDirectory = ''
    let cargoTargetIdentity = null
    let targetBuildLock = null
    const verifyCacheEffect = () => {
      volumeSnapshot = cacheVolume.verify()
      assertPrivateDevelopmentNativeRoot(
        nativeRoot,
        volumeSnapshot,
        nativeRootIdentity
      )
      const currentTemp = assertPrivateDevelopmentTempRoot(volumeSnapshot)
      if (!sameDevelopmentDirectoryIdentity(tempRootIdentity, currentTemp)) {
        throw new Error('[data-native] Development temp root changed during the build')
      }
      if (targetBuildLock) assertTargetBuildLock(targetBuildLock)
      return volumeSnapshot
    }
    const verifyBuildEffect = () => {
      const current = verifyCacheEffect()
      for (const [path, expected, label] of [
        [temporaryDir, temporaryDirIdentity, 'Native temporary output'],
        [cargoTargetDirectory, cargoTargetIdentity, 'Native Cargo target']
      ]) {
        if (!path) continue
        const identity = assertPrivateDevelopmentDirectory(path, current, label)
        if (!sameDevelopmentDirectoryIdentity(expected, identity)) {
          throw new Error(`[data-native] ${label} changed during the build`)
        }
      }
      return current
    }
    const outputDir = targetOutputDirectory(options, target)
    assertOutputDirectorySafe(outputDir)
    assertPrivateDevelopmentOutput(outputDir, nativeRoot, volumeSnapshot)
    assertHermeticCargoEnvironment(
      repoRoot,
      projectDevelopmentCacheEnvironment(process.env, ['CARGO_TARGET_DIR'])
    )
    const rustToolchain = resolveDevelopmentRustToolchain()
    const buildEnvironment = { ...rustToolchain.environment }
    const cargoBin = rustToolchain.cargo
    const rustcBin = rustToolchain.rustc
    const buildEnvironmentSha256 = environmentDigest(buildEnvironment)
    const toolchain = nativeToolchainIdentity({ env: buildEnvironment, toolchain: rustToolchain })
    volumeSnapshot = verifyCacheEffect()
    assertPrivateDevelopmentOutput(outputDir, nativeRoot, volumeSnapshot)
    targetBuildLock = acquireTargetBuildLock(outputDir, target, true, false)
    let cleanupError = null
    try {
      volumeSnapshot = verifyCacheEffect()
      temporaryDir = mkdtempSync(join(dirname(outputDir), `.${basename(outputDir)}.build-`))
      temporaryDirIdentity = assertPrivateDevelopmentDirectory(
        temporaryDir,
        volumeSnapshot,
        'Native temporary output'
      )
      verifyBuildEffect()
      cargoTargetDirectory = mkdtempSync(join(tmpdir(), `analytix-data-native-${target.key}-`))
      cargoTargetIdentity = assertPrivateDevelopmentDirectory(
        cargoTargetDirectory,
        volumeSnapshot,
        'Native Cargo target'
      )
      verifyBuildEffect()
      const buildContextBefore = nativeBuildContextDigest(repoRoot)
      const sourceRoot = repoRoot
      const sourceComponentsBefore = developmentSourceComponents(repoRoot)
      const sourceSetBefore = sourceSetDigest(repoRoot)
      assertHermeticCargoEnvironment(
        repoRoot,
        projectDevelopmentCacheEnvironment(process.env, ['CARGO_TARGET_DIR'])
      )
      const components = manifest.components.map((component) => buildComponent(
        component,
        target,
        temporaryDir,
        cargoBin,
        buildEnvironment,
        rustcBin,
        rustToolchain,
        cargoTargetDirectory,
        sourceRoot,
        verifyBuildEffect
      ))
      const buildContextAfter = nativeBuildContextDigest(repoRoot)
      const sourceComponentsAfter = developmentSourceComponents(repoRoot)
      const sourceSetAfter = sourceSetDigest(repoRoot)
      if (buildContextAfter !== buildContextBefore || sourceSetAfter !== sourceSetBefore) {
        throw new Error('[data-native] Native source set changed during the build')
      }
      if (JSON.stringify(sourceComponentsAfter) !== JSON.stringify(sourceComponentsBefore)) {
        throw new Error('[data-native] Native development source identities changed during the build')
      }
      const rustToolchainAfter = resolveDevelopmentRustToolchain()
      const toolchainAfter = nativeToolchainIdentity({ env: buildEnvironment, toolchain: rustToolchainAfter })
      if (JSON.stringify(toolchainAfter) !== JSON.stringify(toolchain) ||
        rustToolchainAfter.tree.sha256 !== rustToolchain.tree.sha256 ||
        rustToolchainAfter.tree.fileCount !== rustToolchain.tree.fileCount) {
        throw new Error('[data-native] Native toolchain changed during the build')
      }
      volumeSnapshot = verifyBuildEffect()
      removePrivateBuildDirectory(
        cargoTargetDirectory,
        cargoTargetIdentity,
        volumeSnapshot,
        'Native Cargo target'
      )
      cargoTargetDirectory = ''
      cargoTargetIdentity = null
      volumeSnapshot = verifyCacheEffect()
      assertPrivateDevelopmentOutput(outputDir, nativeRoot, volumeSnapshot)
      assertTargetBuildLock(targetBuildLock)
      const marker = publishDevelopmentBuild({
        outputDir,
        temporaryDir,
        target,
        sourceSetSha256: sourceSetAfter,
        buildContextSha256: buildContextAfter,
        buildEnvironmentSha256,
        toolchain,
        sourceComponents: sourceComponentsAfter,
        builtComponents: components,
        verifyBuildEffect,
        verifyPublicationEffect: verifyCacheEffect
      })
      temporaryDir = ''
      temporaryDirIdentity = null
      volumeSnapshot = verifyCacheEffect()
      assertPrivateDevelopmentOutput(outputDir, nativeRoot, volumeSnapshot)
      console.log(`[data-native] Development build staged -> ${outputDir}`)
      console.log(`[data-native] Non-publishable marker -> ${marker}`)
    } finally {
      for (const cleanup of [
        () => {
          if (!cargoTargetDirectory) return
          const current = verifyBuildEffect()
          removePrivateBuildDirectory(
            cargoTargetDirectory,
            cargoTargetIdentity,
            current,
            'Native Cargo target'
          )
          cargoTargetDirectory = ''
          cargoTargetIdentity = null
        },
        () => {
          if (!temporaryDir) return
          const current = verifyBuildEffect()
          removePrivateBuildDirectory(
            temporaryDir,
            temporaryDirIdentity,
            current,
            'Native temporary output'
          )
          temporaryDir = ''
          temporaryDirIdentity = null
        },
        () => {
          verifyCacheEffect()
          releaseTargetBuildLock(targetBuildLock)
          targetBuildLock = null
          verifyCacheEffect()
        }
      ]) {
        try {
          cleanup()
        } catch (error) {
          cleanupError ||= error
        }
      }
    }
    if (cleanupError) throw cleanupError
  } finally {
    cacheVolume.close()
  }
}

function main(argv = process.argv.slice(2)) {
  const options = parseBuildArgs(argv)
  for (const target of packageTargets(options)) buildTarget(options, target)
}

if (require.main === module) {
  try {
    main()
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  }
}

module.exports = {
  _internals: {
    cargoBuildArgs,
    copyBuiltComponentExclusive,
    acquireTargetBuildLock,
    assertTargetBuildLock,
    buildTarget,
    main,
    normalizeDarwinCodeSignature,
    parseBuildArgs,
    packageTargets,
    publishDevelopmentBuild,
    developmentSourceComponents,
    recoverDeadLocalTargetBuildLock,
    requireLocalBuildAuthorityTarget,
    releaseTargetBuildLock,
    releaseBinary,
    targetOutputDirectory
  }
}
