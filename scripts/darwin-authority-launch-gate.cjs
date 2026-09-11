const crypto = require('node:crypto')
const {
  chmodSync,
  closeSync,
  constants,
  fchmodSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdtempSync,
  openSync,
  readSync,
  readdirSync,
  rmdirSync,
  unlinkSync
} = require('node:fs')
const { spawnSync } = require('node:child_process')
const { constants: osConstants } = require('node:os')
const { dirname, isAbsolute, join, parse, relative, resolve, sep } = require('node:path')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')

const GATE_PROTOCOL = 'analytix-darwin-authority-launch-gate-v1'
const GATE_LOCK_PATH = join(__dirname, 'darwin-authority-launch-gate.json')
const PRIVATE_TMP = '/private/tmp'
const MAX_LOCK_BYTES = 64 * 1024
const MAX_SOURCE_BYTES = 256 * 1024
const MAX_GATE_BYTES = 8 * 1024 * 1024
const COMPILER_TIMEOUT_MS = 120_000
const COMPILER_OUTPUT_LIMIT = 64 * 1024
const SHA256 = /^[0-9a-f]{64}$/u
const CDHASH = /^[0-9a-f]{40}$/u
const RESULT_FIELDS = Object.freeze([
  'exitCode',
  'loadedCdhash',
  'outerProcessReaped',
  'signal',
  'stderr',
  'stdout'
])
const GATE_NATIVE_ERROR_CODES = new Set([
  'ANALYTIX_DARWIN_AUTHORITY_GATE_INVALID_REQUEST',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_REJECTED',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_CLEANUP',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_REENTRANT'
])
const GATE_POISONING_ERROR_CODES = new Set([
  'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_CLEANUP',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED',
  'ANALYTIX_DARWIN_AUTHORITY_GATE_REENTRANT'
])
const PROFILE_FD_COUNTS = Object.freeze({
  build_coordinator: 0,
  build_probe: 1,
  source_snapshot: 4,
  source_snapshot_discard: 2,
  generation_publish: 5
})

let cachedGate = null
let cleanupRegistered = false

function invokeDarwinAuthorityPinnedV1(
  profile,
  executableFd,
  expectedExecutableSha256,
  expectedExecutableSize,
  input,
  inheritedFds
) {
  return invokeDarwinAuthorityPinnedWithGateV1(
    darwinAuthorityLaunchGate(),
    profile,
    executableFd,
    expectedExecutableSha256,
    expectedExecutableSize,
    input,
    inheritedFds
  )
}

function invokeDarwinAuthorityPinnedWithGateV1(
  gate,
  profile,
  executableFd,
  expectedExecutableSha256,
  expectedExecutableSize,
  input,
  inheritedFds
) {
  if (
    !gate || typeof gate !== 'object' || !gate.module || !gate.jsState ||
    !Object.hasOwn(PROFILE_FD_COUNTS, profile) ||
    !Number.isInteger(executableFd) || executableFd < 0 ||
    !SHA256.test(String(expectedExecutableSha256 || '')) ||
    !Number.isSafeInteger(expectedExecutableSize) || expectedExecutableSize <= 0 ||
    expectedExecutableSize > 2 * 1024 * 1024 * 1024 ||
    !Buffer.isBuffer(input) || input.length === 0 ||
    !Array.isArray(inheritedFds) || inheritedFds.length !== PROFILE_FD_COUNTS[profile] ||
    inheritedFds.some((fd, index) =>
      !Number.isInteger(fd) || fd < 0 || fd === executableFd || inheritedFds.indexOf(fd) !== index)
  ) {
    throw new Error('[data-native] Darwin authority launch request is invalid')
  }
  assertOpenedGateUnchanged(gate)
  if (gate.jsState.poisoned) {
    throw darwinAuthorityGateError('ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED')
  }
  let result
  try {
    result = gate.module.invokePinnedV1(
      1,
      profile,
      executableFd,
      expectedExecutableSha256,
      expectedExecutableSize,
      input,
      inheritedFds
    )
  } catch (error) {
    const nativeCode = typeof error?.code === 'string' && GATE_NATIVE_ERROR_CODES.has(error.code)
      ? error.code
      : 'ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME'
    if (GATE_POISONING_ERROR_CODES.has(nativeCode) || !GATE_NATIVE_ERROR_CODES.has(String(error?.code || ''))) {
      gate.jsState.poisoned = true
    }
    throw darwinAuthorityGateError(nativeCode)
  }
  if (
    !result || typeof result !== 'object' || Array.isArray(result) ||
    !exactKeys(result, RESULT_FIELDS) ||
    !Number.isInteger(result.exitCode) || result.exitCode < -1 || result.exitCode > 255 ||
    !Number.isInteger(result.signal) || result.signal < 0 || result.signal > 128 ||
    !Buffer.isBuffer(result.stdout) || !Buffer.isBuffer(result.stderr) ||
    !CDHASH.test(String(result.loadedCdhash || '')) || result.outerProcessReaped !== true
  ) {
    gate.jsState.poisoned = true
    throw darwinAuthorityGateError('ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME')
  }
  try {
    return Object.freeze({
      exitCode: result.exitCode,
      signal: result.signal,
      stdout: Buffer.from(result.stdout),
      stderr: Buffer.from(result.stderr),
      loadedCdhash: result.loadedCdhash,
      outerProcessReaped: true
    })
  } catch {
    gate.jsState.poisoned = true
    throw darwinAuthorityGateError('ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME')
  }
}

function freshDarwinAuthorityLaunchGateV1() {
  const gate = darwinAuthorityLaunchGate()
  assertOpenedGateUnchanged(gate)
  return Object.freeze({
    descriptor: gate.descriptor,
    identity: gate.identity,
    sha256: gate.sha256,
    module: loadOpenedGateModuleV1(gate.descriptor),
    jsState: { poisoned: false },
    receipt: gate.receipt
  })
}

function darwinAuthorityGateError(code) {
  const messages = {
    ANALYTIX_DARWIN_AUTHORITY_GATE_INVALID_REQUEST: 'Darwin authority launch gate request is invalid',
    ANALYTIX_DARWIN_AUTHORITY_GATE_REJECTED: 'Darwin authority launch gate rejected execution',
    ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_CLEANUP: 'Darwin authority launch gate cleanup is indeterminate',
    ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME: 'Darwin authority launch gate execution is indeterminate',
    ANALYTIX_DARWIN_AUTHORITY_GATE_POISONED: 'Darwin authority launch gate is poisoned',
    ANALYTIX_DARWIN_AUTHORITY_GATE_REENTRANT: 'Darwin authority launch gate rejected reentrant execution'
  }
  const error = new Error(`[data-native] ${messages[code] || messages.ANALYTIX_DARWIN_AUTHORITY_GATE_INDETERMINATE_AFTER_RESUME}`)
  Object.defineProperty(error, 'code', { value: code })
  return error
}

function darwinAuthorityLaunchGateIdentity() {
  const gate = darwinAuthorityLaunchGate()
  assertOpenedGateUnchanged(gate)
  return Object.freeze({ ...gate.receipt })
}

function darwinAuthorityLaunchGate() {
  if (cachedGate !== null) {
    assertOpenedGateUnchanged(cachedGate)
    return cachedGate
  }
  if (process.platform !== 'darwin' || !['arm64', 'x64'].includes(process.arch)) {
    throw new Error('[data-native] Darwin authority launch gate is unavailable on this host')
  }
  const lock = loadGateLock()
  const hostKey = `darwin-${process.arch}`
  const host = lock.hosts[hostKey]
  const platform = validatePlatformInputs(host)
  const source = readGateSource(lock)
  const root = mkdtempSync(join(PRIVATE_TMP, 'analytix-darwin-authority-gate-'))
  chmodSync(root, 0o700)
  let first = null
  let second = null
  try {
    first = compileGateCopy(root, 'a', source.bytes, host, platform)
    second = compileGateCopy(root, 'b', source.bytes, host, platform)
    if (
      first.identity.size !== second.identity.size || first.sha256 !== second.sha256 ||
      first.bytes.length !== second.bytes.length ||
      !crypto.timingSafeEqual(first.bytes, second.bytes)
    ) {
      throw new Error('[data-native] Darwin authority launch gate build is nondeterministic')
    }
    const loadedModule = loadOpenedGateModuleV1(first.descriptor)
    unlinkExactOpenedFile(first.path, first.descriptor, first.identity)
    first.path = null
    unlinkExactOpenedFile(second.path, second.descriptor, second.identity)
    second.path = null
    closeSync(second.descriptor)
    second.descriptor = -1
    if (readdirSync(root).length !== 0) {
      throw new Error('[data-native] Darwin authority launch gate build root is not empty')
    }
    rmdirSync(root)
    const afterLoad = descriptorIdentity(first.descriptor, { allowUnlinked: true, expectedMode: 0o400 })
    if (
      afterLoad.nlink !== 0n ||
      !sameDescriptorIdentity(first.identity, afterLoad, { ignoreNlink: true, ignoreCtime: true }) ||
      sha256Descriptor(first.descriptor, afterLoad.size) !== first.sha256
    ) {
      throw new Error('[data-native] Darwin authority launch gate changed while loading')
    }
    const toolchainSha256 = canonicalDigest({
      protocol: GATE_PROTOCOL,
      arch: host.arch,
      clangSha256: host.clangSha256,
      ldSha256: host.ldSha256,
      sdkVersion: host.sdkVersion,
      sdkSettingsJsonSha256: host.sdkSettingsJsonSha256,
      sdkSettingsPlistSha256: host.sdkSettingsPlistSha256,
      libSystemTbdSha256: host.libSystemTbdSha256,
      minimumMacosVersion: host.minimumMacosVersion
    })
    const receipt = Object.freeze({
      schemaVersion: 1,
      protocol: GATE_PROTOCOL,
      trustClass: 'local_provisional',
      hostKey,
      sourceSha256: source.sha256,
      binarySha256: first.sha256,
      binarySize: first.identity.size,
      toolchainSha256,
      napiVersion: 1,
      descriptorLoaded: true
    })
    cachedGate = Object.freeze({
      descriptor: first.descriptor,
      identity: afterLoad,
      sha256: first.sha256,
      module: loadedModule,
      jsState: { poisoned: false },
      receipt
    })
    first.descriptor = -1
    registerCleanup()
    return cachedGate
  } catch (error) {
    cleanupBuildOutput(first)
    cleanupBuildOutput(second)
    cleanupEmptyRoot(root)
    throw error
  }
}

function loadOpenedGateModuleV1(descriptor) {
  if (!Number.isInteger(descriptor) || descriptor < 0) {
    throw new Error('[data-native] Darwin authority launch gate descriptor is invalid')
  }
  const loaded = { exports: {} }
  const dlopenFlags = osConstants.dlopen.RTLD_NOW | osConstants.dlopen.RTLD_LOCAL
  process.dlopen(loaded, `/dev/fd/${descriptor}`, dlopenFlags)
  if (
    !loaded.exports || typeof loaded.exports !== 'object' ||
    Reflect.ownKeys(loaded.exports).length !== 1 ||
    typeof loaded.exports.invokePinnedV1 !== 'function'
  ) {
    throw new Error('[data-native] Darwin authority launch gate ABI is invalid')
  }
  return Object.freeze({ invokePinnedV1: loaded.exports.invokePinnedV1 })
}

function compileGateCopy(root, suffix, source, host, platform) {
  const object = openExclusiveOutput(root, `gate-${suffix}.o`)
  const output = openExclusiveOutput(root, `gate-${suffix}.node`)
  try {
    runCompiler(platform.clangPath, [
      '-x', 'c',
      '-std=c11',
      '-O2',
      '-Wall',
      '-Wextra',
      '-Werror',
      '-fvisibility=hidden',
      '-fstack-protector-strong',
      '-fno-ident',
      '-arch', host.arch,
      '-isysroot', platform.sdkRoot,
      `-mmacosx-version-min=${host.minimumMacosVersion}`,
      '-c',
      '-o', '/dev/fd/3',
      '-'
    ], {
      cwd: '/',
      env: compilerEnvironment(root, host, platform),
      input: source,
      stdio: ['pipe', 'pipe', 'pipe', object.descriptor]
    })
    fsyncSync(object.descriptor)
    const objectIdentity = descriptorIdentity(object.descriptor, { expectedMode: 0o600 })
    unlinkExactOpenedFile(object.path, object.descriptor, objectIdentity)
    object.path = null
    runCompiler(platform.clangPath, [
      '-arch', host.arch,
      '-isysroot', platform.sdkRoot,
      `-mmacosx-version-min=${host.minimumMacosVersion}`,
      `-fuse-ld=${platform.ldPath}`,
      '-bundle',
      '-undefined', 'dynamic_lookup',
      '-Wl,-dead_strip',
      '-o', '/dev/fd/4',
      '/dev/fd/3',
      '-lproc'
    ], {
      cwd: '/',
      env: compilerEnvironment(root, host, platform),
      stdio: ['ignore', 'pipe', 'pipe', object.descriptor, output.descriptor]
    })
    fsyncSync(output.descriptor)
    fchmodSync(output.descriptor, 0o400)
    fsyncSync(output.descriptor)
    const identity = descriptorIdentity(output.descriptor, { expectedMode: 0o400 })
    const sha256 = sha256Descriptor(output.descriptor, identity.size)
    const bytes = readDescriptor(output.descriptor, identity.size)
    assertPathMatchesDescriptor(output.path, identity)
    closeSync(object.descriptor)
    object.descriptor = -1
    return {
      path: output.path,
      descriptor: output.descriptor,
      identity,
      sha256,
      bytes
    }
  } catch (error) {
    cleanupBuildOutput(object)
    cleanupBuildOutput(output)
    throw error
  }
}

function runCompiler(executable, arguments_, options) {
  const result = spawnSync(executable, arguments_, {
    ...options,
    timeout: COMPILER_TIMEOUT_MS,
    maxBuffer: COMPILER_OUTPUT_LIMIT,
    killSignal: 'SIGKILL',
    windowsHide: true
  })
  if (
    result.error || result.status !== 0 || result.signal !== null ||
    !Buffer.isBuffer(result.stdout) || !Buffer.isBuffer(result.stderr) ||
    result.stdout.length !== 0 || result.stderr.length !== 0
  ) {
    throw new Error('[data-native] Darwin authority launch gate compilation failed')
  }
}

function compilerEnvironment(root, host, platform) {
  return Object.freeze({
    PATH: '/usr/bin:/bin',
    HOME: root,
    TMPDIR: root,
    LANG: 'C',
    LC_ALL: 'C',
    TZ: 'UTC',
    DEVELOPER_DIR: platform.developerDirectory,
    SDKROOT: platform.sdkRoot,
    MACOSX_DEPLOYMENT_TARGET: host.minimumMacosVersion
  })
}

function openExclusiveOutput(root, name) {
  const path = join(root, name)
  const descriptor = openSync(
    path,
    constants.O_CREAT | constants.O_EXCL | constants.O_RDWR | constants.O_NOFOLLOW,
    0o600
  )
  return { path, descriptor }
}

function cleanupBuildOutput(output) {
  if (!output) return
  if (output.path && output.descriptor >= 0) {
    try {
      const identity = descriptorIdentity(output.descriptor, { allowEmpty: true })
      unlinkExactOpenedFile(output.path, output.descriptor, identity)
      output.path = null
    } catch {
      // Best-effort cleanup after a failed or interrupted authority launch.
    }
  }
  if (output.descriptor >= 0) {
    try {
      closeSync(output.descriptor)
    } catch {
      // Best-effort cleanup after a failed or interrupted authority launch.
    }
    output.descriptor = -1
  }
}

function cleanupEmptyRoot(root) {
  try {
    if (readdirSync(root).length === 0) rmdirSync(root)
  } catch {
    // The root can be non-empty or already removed by a concurrent cleanup.
  }
}

function unlinkExactOpenedFile(path, descriptor, expected) {
  assertPathMatchesDescriptor(path, expected)
  unlinkSync(path)
  const after = descriptorIdentity(descriptor, {
    allowEmpty: expected.size === 0,
    allowUnlinked: true,
    expectedMode: Number(expected.mode & 0o7777n)
  })
  if (
    after.nlink !== 0n ||
    !sameDescriptorIdentity(expected, after, { ignoreNlink: true, ignoreCtime: true })
  ) {
    throw new Error('[data-native] Darwin authority launch gate unlink is indeterminate')
  }
}

function assertPathMatchesDescriptor(path, expected) {
  const stat = lstatSync(path, { bigint: true })
  const actual = identityFromStat(stat)
  if (stat.isSymbolicLink() || !sameDescriptorIdentity(expected, actual)) {
    throw new Error('[data-native] Darwin authority launch gate path identity changed')
  }
}

function descriptorIdentity(descriptor, options = {}) {
  const stat = fstatSync(descriptor, { bigint: true })
  const identity = identityFromStat(stat)
  const mode = Number(identity.mode & 0o7777n)
  if (
    Number(identity.mode & BigInt(constants.S_IFMT)) !== constants.S_IFREG ||
    (!options.allowUnlinked && identity.nlink !== 1n) ||
    (options.allowUnlinked && ![0n, 1n].includes(identity.nlink)) ||
    (!options.allowEmpty && (identity.size <= 0 || identity.size > MAX_GATE_BYTES)) ||
    (options.allowEmpty && (identity.size < 0 || identity.size > MAX_GATE_BYTES)) ||
    (options.expectedMode !== undefined && mode !== options.expectedMode) ||
    (identity.uid !== 0n && identity.uid !== BigInt(process.geteuid()))
  ) {
    throw new Error('[data-native] Darwin authority launch gate file identity is invalid')
  }
  return Object.freeze(identity)
}

function identityFromStat(stat) {
  return {
    dev: stat.dev,
    ino: stat.ino,
    mode: stat.mode,
    uid: stat.uid,
    gid: stat.gid,
    nlink: stat.nlink,
    size: Number(stat.size),
    mtimeNs: stat.mtimeNs,
    ctimeNs: stat.ctimeNs,
    birthtimeNs: stat.birthtimeNs
  }
}

function sameDescriptorIdentity(left, right, options = {}) {
  return Boolean(left && right) &&
    left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid &&
    (options.ignoreNlink || left.nlink === right.nlink) && left.size === right.size &&
    left.mtimeNs === right.mtimeNs && (options.ignoreCtime || left.ctimeNs === right.ctimeNs) &&
    left.birthtimeNs === right.birthtimeNs
}

function readDescriptor(descriptor, size) {
  if (!Number.isSafeInteger(size) || size < 0 || size > MAX_GATE_BYTES) {
    throw new Error('[data-native] Darwin authority launch gate file size is invalid')
  }
  const body = Buffer.allocUnsafe(size)
  let offset = 0
  while (offset < size) {
    const count = readSync(descriptor, body, offset, size - offset, offset)
    if (count <= 0 || count > size - offset) {
      throw new Error('[data-native] Darwin authority launch gate file read failed')
    }
    offset += count
  }
  const extra = Buffer.allocUnsafe(1)
  if (readSync(descriptor, extra, 0, 1, size) !== 0) {
    throw new Error('[data-native] Darwin authority launch gate file grew while reading')
  }
  return body
}

function sha256Descriptor(descriptor, size) {
  return crypto.createHash('sha256').update(readDescriptor(descriptor, size)).digest('hex')
}

function readGateSource(lock) {
  const path = join(__dirname, '..', lock.sourcePath)
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW)
    const before = fstatSync(descriptor, { bigint: true })
    const size = Number(before.size)
    if (
      !before.isFile() || before.isSymbolicLink() || before.nlink !== 1n ||
      size !== lock.sourceSize || size <= 0 || size > MAX_SOURCE_BYTES ||
      (Number(before.mode) & 0o022) !== 0 ||
      (before.uid !== 0n && before.uid !== BigInt(process.geteuid()))
    ) {
      throw new Error('[data-native] Darwin authority launch gate source is unsafe')
    }
    const bytes = readDescriptorBounded(descriptor, size, MAX_SOURCE_BYTES)
    const after = fstatSync(descriptor, { bigint: true })
    const sha256 = crypto.createHash('sha256').update(bytes).digest('hex')
    if (!sameStat(before, after) || sha256 !== lock.sourceSha256) {
      throw new Error('[data-native] Darwin authority launch gate source identity mismatch')
    }
    return Object.freeze({ bytes, sha256 })
  } finally {
    if (descriptor !== undefined) closeSync(descriptor)
  }
}

function loadGateLock() {
  const bytes = readBoundedRegularPath(GATE_LOCK_PATH, MAX_LOCK_BYTES, false)
  let lock
  try {
    lock = parseStrictJsonObject(bytes, {
      maxBytes: MAX_LOCK_BYTES,
      maxDepth: 5,
      maxTokens: 256,
      maxStringBytes: 1024,
      maxNumberBytes: 32,
      integerOnly: true
    })
  } catch {
    throw new Error('[data-native] Darwin authority launch gate lock is invalid')
  }
  if (`${JSON.stringify(lock, null, 2)}\n` !== bytes.toString('utf8')) {
    throw new Error('[data-native] Darwin authority launch gate lock is not canonical')
  }
  const rootFields = ['hosts', 'protocol', 'schemaVersion', 'sourcePath', 'sourceSha256', 'sourceSize']
  if (
    !exactKeys(lock, rootFields) || lock.schemaVersion !== 1 || lock.protocol !== GATE_PROTOCOL ||
    lock.sourcePath !== 'scripts/native/darwin-authority-launch-gate.c' ||
    !SHA256.test(String(lock.sourceSha256 || '')) ||
    !Number.isSafeInteger(lock.sourceSize) || lock.sourceSize <= 0 || lock.sourceSize > MAX_SOURCE_BYTES ||
    !lock.hosts || typeof lock.hosts !== 'object' || Array.isArray(lock.hosts) ||
    !exactKeys(lock.hosts, ['darwin-arm64', 'darwin-x64'])
  ) {
    throw new Error('[data-native] Darwin authority launch gate lock schema is invalid')
  }
  for (const [key, host] of Object.entries(lock.hosts)) validateHostLock(key, host)
  return Object.freeze(lock)
}

function validateHostLock(key, host) {
  const fields = [
    'arch', 'clangPath', 'clangSha256', 'developerDirectory', 'ldPath', 'ldSha256',
    'libSystemTbdSha256', 'minimumMacosVersion', 'sdkRoot', 'sdkSettingsJsonSha256',
    'sdkSettingsPlistSha256', 'sdkVersion'
  ]
  if (
    !host || typeof host !== 'object' || Array.isArray(host) || !exactKeys(host, fields) ||
    host.arch !== (key === 'darwin-arm64' ? 'arm64' : 'x86_64') ||
    ![host.clangPath, host.developerDirectory, host.ldPath, host.sdkRoot].every(isAbsolute) ||
    ![host.clangSha256, host.ldSha256, host.libSystemTbdSha256,
      host.sdkSettingsJsonSha256, host.sdkSettingsPlistSha256].every((value) => SHA256.test(String(value || ''))) ||
    !/^\d+\.\d+$/u.test(String(host.sdkVersion || '')) || host.minimumMacosVersion !== '11.0'
  ) {
    throw new Error('[data-native] Darwin authority launch gate host lock is invalid')
  }
}

function validatePlatformInputs(host) {
  for (const path of [host.developerDirectory, dirname(host.clangPath), dirname(host.ldPath), host.sdkRoot]) {
    assertRootOwnedDirectoryPath(path)
  }
  const clangSha256 = sha256TrustedRootFile(host.clangPath, MAX_GATE_BYTES * 64)
  const ldSha256 = sha256TrustedRootFile(host.ldPath, MAX_GATE_BYTES * 64)
  const settingsJson = join(host.sdkRoot, 'SDKSettings.json')
  const settingsPlist = join(host.sdkRoot, 'SDKSettings.plist')
  const libSystem = join(host.sdkRoot, 'usr', 'lib', 'libSystem.B.tbd')
  if (
    clangSha256 !== host.clangSha256 || ldSha256 !== host.ldSha256 ||
    sha256TrustedRootFile(settingsJson, MAX_LOCK_BYTES) !== host.sdkSettingsJsonSha256 ||
    sha256TrustedRootFile(settingsPlist, MAX_LOCK_BYTES) !== host.sdkSettingsPlistSha256 ||
    sha256TrustedRootFile(libSystem, MAX_GATE_BYTES) !== host.libSystemTbdSha256
  ) {
    throw new Error('[data-native] Darwin authority launch gate platform identity mismatch')
  }
  return Object.freeze({
    developerDirectory: host.developerDirectory,
    clangPath: host.clangPath,
    ldPath: host.ldPath,
    sdkRoot: host.sdkRoot
  })
}

function assertRootOwnedDirectoryPath(path) {
  const resolved = resolve(path)
  const root = parse(resolved).root
  let current = root
  for (const part of relative(root, resolved).split(sep).filter(Boolean)) {
    current = join(current, part)
    const stat = lstatSync(current, { bigint: true })
    if (!stat.isDirectory() || stat.isSymbolicLink() || stat.uid !== 0n || (Number(stat.mode) & 0o022) !== 0) {
      throw new Error('[data-native] Darwin authority launch gate platform directory is unsafe')
    }
  }
}

function sha256TrustedRootFile(path, maximum) {
  assertRootOwnedDirectoryPath(dirname(path))
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW)
    const before = fstatSync(descriptor, { bigint: true })
    const size = Number(before.size)
    if (
      !before.isFile() || before.isSymbolicLink() || before.uid !== 0n || before.nlink !== 1n ||
      (Number(before.mode) & 0o022) !== 0 || !Number.isSafeInteger(size) || size <= 0 || size > maximum
    ) {
      throw new Error('[data-native] Darwin authority launch gate platform file is unsafe')
    }
    const bytes = readDescriptorBounded(descriptor, size, maximum)
    const after = fstatSync(descriptor, { bigint: true })
    if (!sameStat(before, after)) {
      throw new Error('[data-native] Darwin authority launch gate platform file changed')
    }
    return crypto.createHash('sha256').update(bytes).digest('hex')
  } finally {
    if (descriptor !== undefined) closeSync(descriptor)
  }
}

function readBoundedRegularPath(path, maximum, requireRoot) {
  let descriptor
  try {
    descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW)
    const before = fstatSync(descriptor, { bigint: true })
    const size = Number(before.size)
    if (
      !before.isFile() || before.isSymbolicLink() || before.nlink !== 1n ||
      !Number.isSafeInteger(size) || size <= 0 || size > maximum ||
      (Number(before.mode) & 0o022) !== 0 ||
      (requireRoot ? before.uid !== 0n : before.uid !== 0n && before.uid !== BigInt(process.geteuid()))
    ) {
      throw new Error('[data-native] Darwin authority launch gate input is unsafe')
    }
    const body = readDescriptorBounded(descriptor, size, maximum)
    const after = fstatSync(descriptor, { bigint: true })
    if (!sameStat(before, after)) {
      throw new Error('[data-native] Darwin authority launch gate input changed')
    }
    return body
  } finally {
    if (descriptor !== undefined) closeSync(descriptor)
  }
}

function readDescriptorBounded(descriptor, size, maximum) {
  if (!Number.isSafeInteger(size) || size < 0 || size > maximum) {
    throw new Error('[data-native] Darwin authority launch gate bounded read is invalid')
  }
  const body = Buffer.allocUnsafe(size)
  let offset = 0
  while (offset < size) {
    const count = readSync(descriptor, body, offset, size - offset, offset)
    if (count <= 0 || count > size - offset) {
      throw new Error('[data-native] Darwin authority launch gate bounded read failed')
    }
    offset += count
  }
  const extra = Buffer.allocUnsafe(1)
  if (readSync(descriptor, extra, 0, 1, size) !== 0) {
    throw new Error('[data-native] Darwin authority launch gate input grew while reading')
  }
  return body
}

function sameStat(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid && left.nlink === right.nlink &&
    left.size === right.size && left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs &&
    left.birthtimeNs === right.birthtimeNs
}

function assertOpenedGateUnchanged(gate) {
  if (!gate || gate.descriptor < 0) {
    throw new Error('[data-native] Darwin authority launch gate is unavailable')
  }
  const identity = descriptorIdentity(gate.descriptor, { allowUnlinked: true, expectedMode: 0o400 })
  if (
    identity.nlink !== 0n || !sameDescriptorIdentity(gate.identity, identity) ||
    sha256Descriptor(gate.descriptor, identity.size) !== gate.sha256
  ) {
    throw new Error('[data-native] Darwin authority launch gate identity changed')
  }
}

function canonicalDigest(value) {
  const canonical = {}
  for (const key of Object.keys(value).sort()) canonical[key] = value[key]
  return crypto.createHash('sha256').update(JSON.stringify(canonical)).digest('hex')
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value).sort()
  const wanted = [...expected].sort()
  return keys.length === wanted.length && keys.every((key, index) => key === wanted[index])
}

function registerCleanup() {
  if (cleanupRegistered) return
  cleanupRegistered = true
  process.once('exit', () => {
    if (cachedGate !== null) {
      try {
        closeSync(cachedGate.descriptor)
      } catch {
        // Process-exit cleanup cannot recover from a close failure.
      }
      cachedGate = null
    }
  })
}

module.exports = {
  darwinAuthorityLaunchGateIdentity,
  invokeDarwinAuthorityPinnedV1,
  _internals: {
    freshDarwinAuthorityLaunchGateV1,
    invokeDarwinAuthorityPinnedWithGateV1
  }
}
