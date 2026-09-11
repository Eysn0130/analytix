const crypto = require('node:crypto')
const fs = require('node:fs')
const path = require('node:path')
const { nativePayloadIdentity } = require('./native-component-contract.cjs')

const LOCK_PATH = path.join(__dirname, 'windows-python-runtime-lock.json')
const REQUIREMENTS_LOCK_PATH = path.join(__dirname, 'windows-backend-requirements.lock.txt')
const RUNTIME_MANIFEST_NAME = 'analytix-python-runtime-manifest.json'
const SITE_PACKAGES_MANIFEST_NAME = 'analytix-site-packages-manifest.json'
const lockBytes = fs.readFileSync(LOCK_PATH)
const requirementsLockBytes = fs.readFileSync(REQUIREMENTS_LOCK_PATH)
const lock = JSON.parse(lockBytes.toString('utf8'))
const lockSha256 = sha256Bytes(lockBytes)
const BUILD_ONLY_DIRECTORY_NAMES = new Set([
  '.git',
  '.venv',
  '.pytest_cache',
  '__pycache__',
  '__tests__',
  '__fixtures__',
  'benchmark',
  'benchmarks',
  'coverage',
  'demo',
  'demos',
  'doc',
  'docs',
  'documentation',
  'example',
  'examples',
  'fixtures',
  'test',
  'tests',
  'testsupport',
  'upstreamaudit'
])
const BUILD_ONLY_FILE_NAMES = new Set([
  '.DS_Store',
  'README',
  'readme',
  'README.md',
  'readme.md',
  'CHANGELOG',
  'CHANGELOG.md',
  'changelog.md'
])
const RUNTIME_ONLY_REMOVALS = [
  ['python', 'Lib', 'site-packages'],
  ['python', 'Lib', 'ensurepip'],
  ['python', 'Lib', 'venv'],
  ['python', 'Scripts'],
  ['python', 'include'],
  ['python', 'libs']
]

function sha256Bytes(bytes) {
  return crypto.createHash('sha256').update(bytes).digest('hex')
}

function exactKeys(value, expected) {
  return Boolean(
    value &&
    typeof value === 'object' &&
    !Array.isArray(value) &&
    Object.keys(value).sort().join(',') === [...expected].sort().join(',')
  )
}

function isSha256(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
}

function validateLock() {
  if (!exactKeys(lock, ['schemaVersion', 'pythonRuntime', 'sitePackages']) || lock.schemaVersion !== 1) {
    throw new Error('[python-runtime] Frozen runtime lock schema is invalid')
  }
  if (!exactKeys(lock.pythonRuntime, [
    'version',
    'series',
    'target',
    'assetName',
    'archiveSha256',
    'sourceTreeSha256',
    'runtimeContentSha256',
    'runtimeFileCount'
  ])) {
    throw new Error('[python-runtime] Frozen Python runtime authority is invalid')
  }
  if (!exactKeys(lock.sitePackages, [
    'platform',
    'requirementsLockSha256',
    'wheelCount',
    'wheelSetSha256',
    'contentSha256',
    'fileCount'
  ])) {
    throw new Error('[python-runtime] Frozen site-packages authority is invalid')
  }
  for (const value of [
    lock.pythonRuntime.archiveSha256,
    lock.pythonRuntime.sourceTreeSha256,
    lock.pythonRuntime.runtimeContentSha256,
    lock.sitePackages.requirementsLockSha256,
    lock.sitePackages.wheelSetSha256,
    lock.sitePackages.contentSha256
  ]) {
    if (!isSha256(value)) throw new Error('[python-runtime] Frozen authority contains an invalid SHA-256')
  }
  if (
    sha256Bytes(requirementsLockBytes) !== lock.sitePackages.requirementsLockSha256 ||
    !Number.isSafeInteger(lock.pythonRuntime.runtimeFileCount) ||
    lock.pythonRuntime.runtimeFileCount <= 0 ||
    !Number.isSafeInteger(lock.sitePackages.wheelCount) ||
    lock.sitePackages.wheelCount <= 0 ||
    !Number.isSafeInteger(lock.sitePackages.fileCount) ||
    lock.sitePackages.fileCount <= 0
  ) {
    throw new Error('[python-runtime] Frozen requirements or inventory authority is stale')
  }
}

function listRegularFiles(root, current = root, files = []) {
  const rootStat = fs.lstatSync(current)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error(`[python-runtime] Tree contains an untrusted directory: ${current}`)
  }
  for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
    const itemPath = path.join(current, entry.name)
    if (entry.isSymbolicLink()) {
      throw new Error(`[python-runtime] Tree contains a symbolic link: ${itemPath}`)
    }
    if (entry.isDirectory()) listRegularFiles(root, itemPath, files)
    else if (entry.isFile()) files.push(itemPath)
    else throw new Error(`[python-runtime] Tree contains a special file: ${itemPath}`)
  }
  return files
}

function isBuildOnlyFileName(name) {
  return BUILD_ONLY_FILE_NAMES.has(name) ||
    /^(?:readme|changelog)(?:\.[^.]+)?$/iu.test(name) ||
    name.startsWith('._') ||
    /^tsconfig.*\.json$/iu.test(name) ||
    /\.(?:map|pdb|pyc|ts)$/iu.test(name) ||
    /\.d\.ts$/iu.test(name)
}

function pruneProductionPythonTree(root, options = {}) {
  const rootStat = fs.lstatSync(root)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error(`[python-runtime] Production prune root is not trusted: ${root}`)
  }
  function visit(current) {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      const itemPath = path.join(current, entry.name)
      if (entry.isSymbolicLink()) {
        throw new Error(`[python-runtime] Production tree contains a symbolic link: ${itemPath}`)
      }
      if (entry.isDirectory()) {
        if (BUILD_ONLY_DIRECTORY_NAMES.has(entry.name)) fs.rmSync(itemPath, { recursive: true, force: true })
        else visit(itemPath)
      } else if (entry.isFile()) {
        if (isBuildOnlyFileName(entry.name)) fs.rmSync(itemPath, { force: true })
      } else {
        throw new Error(`[python-runtime] Production tree contains a special file: ${itemPath}`)
      }
    }
  }
  if (options.runtime === true) {
    for (const parts of RUNTIME_ONLY_REMOVALS) fs.rmSync(path.join(root, ...parts), { recursive: true, force: true })
  }
  visit(root)
}

function sortByRelativeUtf8(root, files) {
  return files.sort((left, right) => Buffer.compare(
    Buffer.from(relativePath(root, left), 'utf8'),
    Buffer.from(relativePath(root, right), 'utf8')
  ))
}

function relativePath(root, itemPath) {
  return path.relative(root, itemPath).split(path.sep).join('/')
}

function rawTreeIdentity(root, excludedNames = []) {
  const excluded = new Set(excludedNames)
  const files = sortByRelativeUtf8(root, listRegularFiles(root).filter(
    (itemPath) => !excluded.has(relativePath(root, itemPath))
  ))
  const hash = crypto.createHash('sha256')
  const seen = new Set()
  for (const itemPath of files) {
    const relative = relativePath(root, itemPath)
    const canonicalPath = relative.toLowerCase()
    if (seen.has(canonicalPath)) throw new Error(`[python-runtime] Tree contains a case-insensitive path collision: ${relative}`)
    seen.add(canonicalPath)
    const bytes = fs.readFileSync(itemPath)
    hash.update(relative)
    hash.update('\0')
    hash.update(String(bytes.length))
    hash.update('\0')
    hash.update(sha256Bytes(bytes))
    hash.update('\0')
  }
  return { sha256: hash.digest('hex'), fileCount: files.length }
}

function signingInvariantTreeIdentity(root, excludedNames = []) {
  const excluded = new Set(excludedNames)
  const files = sortByRelativeUtf8(root, listRegularFiles(root).filter(
    (itemPath) => !excluded.has(relativePath(root, itemPath))
  ))
  const hash = crypto.createHash('sha256')
  const seen = new Set()
  for (const itemPath of files) {
    const relative = relativePath(root, itemPath)
    const canonicalPath = relative.toLowerCase()
    if (seen.has(canonicalPath)) throw new Error(`[python-runtime] Tree contains a case-insensitive path collision: ${relative}`)
    seen.add(canonicalPath)
    const bytes = fs.readFileSync(itemPath)
    const portableExecutable = /\.(?:exe|dll|pyd)$/iu.test(relative)
    const identity = portableExecutable
      ? nativePayloadIdentity(bytes, 'pe')
      : { sha256: sha256Bytes(bytes), size: bytes.length }
    hash.update(relative)
    hash.update('\0')
    hash.update(portableExecutable ? 'pe-payload' : 'raw')
    hash.update('\0')
    hash.update(String(identity.size))
    hash.update('\0')
    hash.update(identity.sha256)
    hash.update('\0')
  }
  return { sha256: hash.digest('hex'), fileCount: files.length }
}

function wheelInventory(root) {
  const entries = fs.readdirSync(root, { withFileTypes: true })
  if (entries.some((entry) => !entry.isFile() || entry.isSymbolicLink() || !entry.name.endsWith('.whl'))) {
    throw new Error('[python-runtime] Wheelhouse must contain only regular wheel files')
  }
  return entries
    .map((entry) => {
      const bytes = fs.readFileSync(path.join(root, entry.name))
      return { fileName: entry.name, size: bytes.length, sha256: sha256Bytes(bytes) }
    })
    .sort((left, right) => Buffer.compare(Buffer.from(left.fileName, 'utf8'), Buffer.from(right.fileName, 'utf8')))
}

function wheelSetSha256(wheels) {
  const hash = crypto.createHash('sha256')
  const seen = new Set()
  for (const wheel of wheels) {
    if (
      !exactKeys(wheel, ['fileName', 'size', 'sha256']) ||
      typeof wheel.fileName !== 'string' ||
      !wheel.fileName.endsWith('.whl') ||
      !Number.isSafeInteger(wheel.size) ||
      wheel.size <= 0 ||
      !isSha256(wheel.sha256)
    ) {
      throw new Error('[python-runtime] Wheel inventory is invalid')
    }
    const canonicalName = wheel.fileName.toLowerCase()
    if (seen.has(canonicalName)) throw new Error('[python-runtime] Wheel inventory contains a duplicate filename')
    seen.add(canonicalName)
    hash.update(wheel.fileName)
    hash.update('\0')
    hash.update(String(wheel.size))
    hash.update('\0')
    hash.update(wheel.sha256)
    hash.update('\0')
  }
  return hash.digest('hex')
}

function verifyWheelhouse(root) {
  const wheels = wheelInventory(root)
  const digest = wheelSetSha256(wheels)
  if (wheels.length !== lock.sitePackages.wheelCount || digest !== lock.sitePackages.wheelSetSha256) {
    throw new Error('[python-runtime] Wheelhouse does not match the frozen hash lock')
  }
  return { wheels, wheelSetSha256: digest }
}

function parseCanonicalJson(filePath) {
  const text = fs.readFileSync(filePath, 'utf8')
  const value = JSON.parse(text)
  if (`${JSON.stringify(value, null, 2)}\n` !== text) {
    throw new Error(`[python-runtime] Manifest is not canonical duplicate-free JSON: ${filePath}`)
  }
  return value
}

function verifyPythonRuntime(root) {
  const manifest = parseCanonicalJson(path.join(root, RUNTIME_MANIFEST_NAME))
  if (!exactKeys(manifest, [
    'schemaVersion',
    'lockSha256',
    'pythonRuntimeVersion',
    'pythonSeries',
    'pythonTarget',
    'pythonVersionText',
    'assetName',
    'archiveSha256',
    'sourceTreeSha256',
    'runtimeContentSha256',
    'runtimeFileCount'
  ]) || manifest.schemaVersion !== 2) {
    throw new Error('[python-runtime] Python runtime manifest schema is invalid')
  }
  const identity = signingInvariantTreeIdentity(root, [RUNTIME_MANIFEST_NAME])
  if (
    manifest.lockSha256 !== lockSha256 ||
    manifest.pythonRuntimeVersion !== lock.pythonRuntime.version ||
    manifest.pythonSeries !== lock.pythonRuntime.series ||
    manifest.pythonTarget !== lock.pythonRuntime.target ||
    typeof manifest.pythonVersionText !== 'string' ||
    !manifest.pythonVersionText.startsWith(`Python ${lock.pythonRuntime.version.split('+')[0]}`) ||
    manifest.assetName !== lock.pythonRuntime.assetName ||
    manifest.archiveSha256 !== lock.pythonRuntime.archiveSha256 ||
    manifest.sourceTreeSha256 !== lock.pythonRuntime.sourceTreeSha256 ||
    manifest.runtimeContentSha256 !== lock.pythonRuntime.runtimeContentSha256 ||
    manifest.runtimeFileCount !== lock.pythonRuntime.runtimeFileCount ||
    identity.sha256 !== lock.pythonRuntime.runtimeContentSha256 ||
    identity.fileCount !== lock.pythonRuntime.runtimeFileCount
  ) {
    throw new Error('[python-runtime] Python runtime authority is stale or mismatched')
  }
  return { manifest, identity }
}

function verifySitePackages(root) {
  const manifest = parseCanonicalJson(path.join(root, SITE_PACKAGES_MANIFEST_NAME))
  if (!exactKeys(manifest, [
    'schemaVersion',
    'lockSha256',
    'requirementsLockSha256',
    'pythonSeries',
    'platform',
    'wheelCount',
    'wheelSetSha256',
    'wheels',
    'contentSha256',
    'fileCount'
  ]) || manifest.schemaVersion !== 2 || !Array.isArray(manifest.wheels)) {
    throw new Error('[python-runtime] Site-packages manifest schema is invalid')
  }
  const identity = signingInvariantTreeIdentity(root, [SITE_PACKAGES_MANIFEST_NAME])
  if (
    manifest.lockSha256 !== lockSha256 ||
    manifest.requirementsLockSha256 !== lock.sitePackages.requirementsLockSha256 ||
    manifest.pythonSeries !== lock.pythonRuntime.series ||
    manifest.platform !== lock.sitePackages.platform ||
    manifest.wheelCount !== lock.sitePackages.wheelCount ||
    manifest.wheels.length !== lock.sitePackages.wheelCount ||
    wheelSetSha256(manifest.wheels) !== lock.sitePackages.wheelSetSha256 ||
    manifest.wheelSetSha256 !== lock.sitePackages.wheelSetSha256 ||
    manifest.contentSha256 !== lock.sitePackages.contentSha256 ||
    manifest.fileCount !== lock.sitePackages.fileCount ||
    identity.sha256 !== lock.sitePackages.contentSha256 ||
    identity.fileCount !== lock.sitePackages.fileCount
  ) {
    throw new Error('[python-runtime] Site-packages authority is stale or mismatched')
  }
  return { manifest, identity }
}

validateLock()

module.exports = {
  LOCK_PATH,
  REQUIREMENTS_LOCK_PATH,
  RUNTIME_MANIFEST_NAME,
  SITE_PACKAGES_MANIFEST_NAME,
  lock,
  lockSha256,
  pruneProductionPythonTree,
  rawTreeIdentity,
  requirementsLockBytes,
  sha256Bytes,
  signingInvariantTreeIdentity,
  verifyPythonRuntime,
  verifySitePackages,
  verifyWheelhouse,
  wheelInventory,
  wheelSetSha256
}
