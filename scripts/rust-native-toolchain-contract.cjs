const crypto = require('node:crypto')
const { execFileSync } = require('node:child_process')
const {
  closeSync,
  existsSync,
  fstatSync,
  lstatSync,
  openSync,
  readSync,
  readdirSync,
  realpathSync
} = require('node:fs')
const { constants: fsConstants } = require('node:fs')
const { homedir } = require('node:os')
const { basename, dirname, join, relative, resolve, sep } = require('node:path')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')

const lockPath = join(__dirname, 'rust-native-toolchain.json')
const MAX_LOCK_BYTES = 64 * 1024
const MAX_TOOLCHAIN_FILE_BYTES = 1024 * 1024 * 1024
const MAX_TOOLCHAIN_FILES = 4096
const MAX_TOOLCHAIN_TOTAL_BYTES = 2 * 1024 * 1024 * 1024
const ROOT_LOCK_KEYS = Object.freeze(['hosts', 'rustVersion', 'schemaVersion'])
const HOST_LOCK_KEYS = Object.freeze([
  'cargoExecutableSha256',
  'cargoVersion',
  'developerDirectory',
  'hostTriple',
  'rustcExecutableSha256',
  'rustcVersion',
  'sdkRoot',
  'sdkSettingsJsonSha256',
  'sdkSettingsPlistSha256',
  'sdkVersion',
  'toolchainFileCount',
  'toolchainTreeSha256',
  'tools'
])
const TOOL_LOCK_KEYS = Object.freeze(['ar', 'clang', 'codesign', 'ld'])
const HOST_TRIPLES = Object.freeze({ 'darwin-arm64': 'aarch64-apple-darwin' })
const lock = parseRustNativeToolchainLock(readStableRegularFile(lockPath, MAX_LOCK_BYTES))

function readStableRegularFile(path, maxBytes, options = {}) {
  const open = options.openSync || openSync
  const fstat = options.fstatSync || fstatSync
  const read = options.readSync || readSync
  const close = options.closeSync || closeSync
  const noFollow = fsConstants.O_NOFOLLOW || 0
  let descriptor
  try {
    descriptor = open(path, fsConstants.O_RDONLY | noFollow)
    const before = fstat(descriptor)
    if (!trustedRegularFileStat(before) || before.size > maxBytes) {
      throw new Error(`[rust-native-toolchain] Untrusted bounded file: ${path}`)
    }
    const value = Buffer.allocUnsafe(before.size)
    let position = 0
    while (position < before.size) {
      const count = read(descriptor, value, position, before.size - position, position)
      if (count <= 0) throw new Error(`[rust-native-toolchain] Short read: ${path}`)
      position += count
    }
    const after = fstat(descriptor)
    if (!sameFileIdentity(before, after)) {
      throw new Error(`[rust-native-toolchain] File identity changed while reading: ${path}`)
    }
    return value
  } finally {
    if (descriptor !== undefined) close(descriptor)
  }
}

function parseRustNativeToolchainLock(input) {
  const parsed = parseStrictJsonObject(input, {
    maxBytes: MAX_LOCK_BYTES,
    maxDepth: 4,
    maxTokens: 128,
    maxStringBytes: 1024,
    maxNumberBytes: 16,
    integerOnly: true
  })
  validateRustNativeToolchainLock(parsed)
  return deepFreeze(parsed)
}

function validateRustNativeToolchainLock(candidate) {
  assertExactKeys(candidate, ROOT_LOCK_KEYS, 'root lock')
  if (candidate.schemaVersion !== 1 || typeof candidate.rustVersion !== 'string' ||
      !/^\d+\.\d+\.\d+$/.test(candidate.rustVersion)) {
    throw new Error('[rust-native-toolchain] Invalid root lock identity')
  }
  assertExactKeys(candidate.hosts, Object.keys(HOST_TRIPLES), 'host inventory')
  for (const [key, expectedTriple] of Object.entries(HOST_TRIPLES)) {
    const host = candidate.hosts[key]
    assertExactKeys(host, HOST_LOCK_KEYS, `host lock ${key}`)
    assertExactKeys(host.tools, TOOL_LOCK_KEYS, `platform tools ${key}`)
    if (host.hostTriple !== expectedTriple ||
        !nonEmptyString(host.cargoVersion) || !nonEmptyString(host.rustcVersion) ||
        !nonEmptyString(host.developerDirectory) || !nonEmptyString(host.sdkRoot) ||
        !nonEmptyString(host.sdkVersion) || !Number.isSafeInteger(host.toolchainFileCount) ||
        host.toolchainFileCount <= 0 || !digest(host.cargoExecutableSha256) ||
        !digest(host.rustcExecutableSha256) || !digest(host.toolchainTreeSha256) ||
        !digest(host.sdkSettingsJsonSha256) || !digest(host.sdkSettingsPlistSha256) ||
        TOOL_LOCK_KEYS.some((name) => !digest(host.tools[name]))) {
      throw new Error(`[rust-native-toolchain] Invalid host lock identity for ${key}`)
    }
  }
  return candidate
}

function assertExactKeys(candidate, expected, label) {
  if (!isPlainRecord(candidate)) {
    throw new Error(`[rust-native-toolchain] ${label} must be a JSON object`)
  }
  const keys = Reflect.ownKeys(candidate)
  if (keys.some((key) => typeof key !== 'string') ||
      keys.map(String).sort().join('\0') !== [...expected].sort().join('\0')) {
    throw new Error(`[rust-native-toolchain] Unexpected ${label} fields`)
  }
  for (const key of keys) {
    const descriptor = Object.getOwnPropertyDescriptor(candidate, key)
    if (!descriptor || !Object.hasOwn(descriptor, 'value') || !descriptor.enumerable) {
      throw new Error(`[rust-native-toolchain] Invalid ${label} field descriptor`)
    }
  }
}

function isPlainRecord(value) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false
  const prototype = Object.getPrototypeOf(value)
  return prototype === null || prototype === Object.prototype
}

function nonEmptyString(value) {
  return typeof value === 'string' && value.length > 0 && value.trim() === value
}

function digest(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
}

function deepFreeze(value) {
  if (value !== null && typeof value === 'object' && !Object.isFrozen(value)) {
    for (const child of Object.values(value)) deepFreeze(child)
    Object.freeze(value)
  }
  return value
}

function normalizePlatform(value) {
  const platform = String(value || process.platform).trim().toLowerCase()
  if (platform === 'windows' || platform === 'win') return 'win32'
  if (platform === 'darwin' || platform === 'linux' || platform === 'win32') return platform
  throw new Error(`[rust-native-toolchain] Unsupported host platform: ${value}`)
}

function normalizeArch(value) {
  const arch = String(value || process.arch).trim().toLowerCase()
  if (arch === 'amd64' || arch === 'x86_64') return 'x64'
  if (arch === 'aarch64') return 'arm64'
  if (arch === 'x64' || arch === 'arm64') return arch
  throw new Error(`[rust-native-toolchain] Unsupported host architecture: ${value}`)
}

function hostKey(platform = process.platform, arch = process.arch) {
  return `${normalizePlatform(platform)}-${normalizeArch(arch)}`
}

function sha256File(path, options = {}) {
  const open = options.openSync || openSync
  const fstat = options.fstatSync || fstatSync
  const read = options.readSync || readSync
  const close = options.closeSync || closeSync
  const noFollow = fsConstants.O_NOFOLLOW || 0
  let descriptor
  try {
    descriptor = open(path, fsConstants.O_RDONLY | noFollow)
    const before = fstat(descriptor)
    if (!trustedRegularFileStat(before) ||
        (options.expectedStat && !sameFileIdentity(before, options.expectedStat))) {
      throw new Error(`[rust-native-toolchain] Untrusted regular file: ${path}`)
    }
    const hash = crypto.createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let position = 0
    while (position < before.size) {
      const length = Math.min(buffer.length, before.size - position)
      const count = read(descriptor, buffer, 0, length, position)
      if (count <= 0) throw new Error(`[rust-native-toolchain] Short read: ${path}`)
      hash.update(buffer.subarray(0, count))
      position += count
    }
    const after = fstat(descriptor)
    if (!sameFileIdentity(before, after)) {
      throw new Error(`[rust-native-toolchain] File identity changed while hashing: ${path}`)
    }
    return hash.digest('hex')
  } finally {
    if (descriptor !== undefined) close(descriptor)
  }
}

function trustedRegularFileStat(stat) {
  return stat.isFile() && !stat.isSymbolicLink() && stat.nlink === 1 && stat.size > 0 &&
    stat.size <= MAX_TOOLCHAIN_FILE_BYTES && (stat.mode & 0o022) === 0
}

function sameFileIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.nlink === right.nlink && left.size === right.size && left.mtimeMs === right.mtimeMs &&
    left.ctimeMs === right.ctimeMs
}

function assertTrustedDirectory(path, options = {}) {
  const stat = (options.lstatSync || lstatSync)(path)
  if (!stat.isDirectory() || stat.isSymbolicLink() || (stat.mode & 0o022) !== 0) {
    throw new Error(`[rust-native-toolchain] Untrusted directory: ${path}`)
  }
}

function assertNoSymlinkPath(path, options = {}) {
  const lstat = options.lstatSync || lstatSync
  const resolved = resolve(path)
  const parsed = dirname(resolved) === resolved ? resolved : resolve(sep)
  let current = parsed
  for (const part of relative(parsed, resolved).split(sep).filter(Boolean)) {
    current = join(current, part)
    const stat = lstat(current)
    if (stat.isSymbolicLink()) {
      throw new Error(`[rust-native-toolchain] Symlink path is forbidden: ${current}`)
    }
  }
}

function toolchainTreeIdentity(root, options = {}) {
  const readdir = options.readdirSync || readdirSync
  const lstat = options.lstatSync || lstatSync
  const files = []
  function walk(directory) {
    assertTrustedDirectory(directory, options)
    const entries = readdir(directory, { withFileTypes: true })
    for (const entry of entries) {
      const path = join(directory, entry.name)
      if (entry.isSymbolicLink()) {
        throw new Error(`[rust-native-toolchain] Toolchain symlink is forbidden: ${path}`)
      }
      if (entry.isDirectory()) walk(path)
      else if (entry.isFile()) files.push(path)
      else throw new Error(`[rust-native-toolchain] Toolchain special file is forbidden: ${path}`)
      if (files.length > MAX_TOOLCHAIN_FILES) {
        throw new Error('[rust-native-toolchain] Toolchain file inventory exceeds its bound')
      }
    }
  }
  walk(root)
  files.sort((left, right) => Buffer.compare(
    Buffer.from(relative(root, left).split('\\').join('/'), 'utf8'),
    Buffer.from(relative(root, right).split('\\').join('/'), 'utf8')
  ))
  const hash = crypto.createHash('sha256')
  let totalBytes = 0
  for (const path of files) {
    const stat = lstat(path)
    totalBytes += stat.size
    if (!Number.isSafeInteger(totalBytes) || totalBytes > MAX_TOOLCHAIN_TOTAL_BYTES) {
      throw new Error('[rust-native-toolchain] Toolchain byte inventory exceeds its bound')
    }
    const relativePath = relative(root, path).split('\\').join('/')
    hash.update(relativePath)
    hash.update('\0')
    hash.update(String(stat.size))
    hash.update('\0')
    hash.update(String(stat.mode & 0o777))
    hash.update('\0')
    hash.update(sha256File(path, { ...options, expectedStat: stat }))
    hash.update('\0')
  }
  return { fileCount: files.length, totalBytes, sha256: hash.digest('hex') }
}

function defaultToolchainRoot(locked, options = {}) {
  const home = options.home || homedir()
  return join(home, '.rustup', 'toolchains', `${lock.rustVersion}-${locked.hostTriple}`)
}

// Read-only release preflight. Unix mode 0555 is not immutable when the
// current effective user owns the inode: that owner can chmod it and perform a
// mutate-run-restore attack. Local toolchains therefore remain audit evidence
// only until a controlled build coordinator supplies an independently anchored
// execution authority.
function nativeRustToolchainReleasePreflight(options = {}) {
  const platform = normalizePlatform(options.platform || process.platform)
  const key = hostKey(platform, options.arch || process.arch)
  const selectedLock = options.lock || lock
  validateRustNativeToolchainLock(selectedLock)
  const locked = selectedLock.hosts?.[key]
  if (!locked) {
    throw new Error(`[rust-native-toolchain] No pinned Rust toolchain for host ${key}`)
  }
  const exists = options.existsSync || existsSync
  const realpath = options.realpathSync || realpathSync
  const lstat = options.lstatSync || lstatSync
  const rootCandidate = resolve(options.toolchainRoot || defaultToolchainRoot(locked, options))
  if (!exists(rootCandidate)) {
    throw new Error(`[rust-native-toolchain] Pinned Rust toolchain is unavailable for host ${key}`)
  }
  assertNoSymlinkPath(rootCandidate, options)
  const root = realpath(rootCandidate)
  const executableSuffix = platform === 'win32' ? '.exe' : ''
  const executableCandidates = [
    join(root, 'bin', `cargo${executableSuffix}`),
    join(root, 'bin', `rustc${executableSuffix}`)
  ]
  for (const candidate of executableCandidates) assertNoSymlinkPath(candidate, options)
  const candidates = [root, ...executableCandidates.map((candidate) => realpath(candidate))]
  const euid = Number.isInteger(options.euid)
    ? options.euid
    : typeof process.geteuid === 'function' ? process.geteuid() : -1
  const inspected = candidates.map((path) => ({ path, stat: lstat(path) }))
  const userWritable = euid >= 0 && inspected.some(({ stat }) => stat.uid === euid)
  return deepFreeze({
    schemaVersion: 1,
    purpose: 'analytix-cargo-execution',
    status: 'blocked',
    trustClass: 'local_provisional',
    releaseEligible: false,
    blocker: userWritable ? 'toolchain_user_writable' : 'toolchain_identity_unbound',
    ownershipClass: userWritable ? 'user_writable' : 'unbound',
    targetKey: key,
    targetTriple: locked.hostTriple,
    inspectedPaths: inspected.map(({ path }) => path)
  })
}

function exactVersion(executable, expected, environment, options = {}) {
  const exec = options.execFileSync || execFileSync
  const output = String(exec(executable, ['--version'], {
    encoding: 'utf8',
    env: environment,
    windowsHide: true
  })).trim()
  if (output !== expected) {
    throw new Error(`[rust-native-toolchain] Version mismatch for ${basename(executable)}`)
  }
  return output
}

function boundedCommand(executable, args, environment, options = {}) {
  const exec = options.execFileSync || execFileSync
  const output = String(exec(executable, args, {
    encoding: 'utf8',
    env: environment,
    windowsHide: true
  })).trim()
  if (!output || Buffer.byteLength(output, 'utf8') > 16 * 1024) {
    throw new Error(`[rust-native-toolchain] Invalid ${basename(executable)} output`)
  }
  return output
}

/**
 * Resolves a local Rustup toolchain for a development-only native build.
 *
 * This path deliberately does not confer release authority. It binds the
 * current build to the repository-pinned Rust version and records the actual
 * executable/tree identities, while allowing a normal user-owned Rustup and
 * current Xcode Command Line Tools installation. Release callers continue to
 * use resolvePinnedRustToolchain plus nativeRustToolchainReleasePreflight.
 */
function resolveDevelopmentRustToolchain(options = {}) {
  const platform = normalizePlatform(options.platform || process.platform)
  const arch = normalizeArch(options.arch || process.arch)
  const key = hostKey(platform, arch)
  const selectedLock = options.lock || lock
  validateRustNativeToolchainLock(selectedLock)
  const locked = selectedLock.hosts?.[key]
  if (!locked) {
    throw new Error(`[rust-native-toolchain] No development Rust target for host ${key}`)
  }
  const exists = options.existsSync || existsSync
  const realpath = options.realpathSync || realpathSync
  const home = resolve(options.home || process.env.HOME || homedir())
  const cargoHome = resolve(options.cargoHome || process.env.CARGO_HOME || join(home, '.cargo'))
  const rustupHome = resolve(options.rustupHome || process.env.RUSTUP_HOME || join(home, '.rustup'))
  const rootCandidate = resolve(
    options.toolchainRoot || join(rustupHome, 'toolchains', `${selectedLock.rustVersion}-${locked.hostTriple}`)
  )
  if (!exists(rootCandidate)) {
    throw new Error(`[rust-native-toolchain] Development Rustup toolchain is unavailable for host ${key}`)
  }
  assertNoSymlinkPath(rootCandidate, options)
  const root = realpath(rootCandidate)
  assertTrustedDirectory(root, options)
  const executableSuffix = platform === 'win32' ? '.exe' : ''
  const cargo = realpath(join(root, 'bin', `cargo${executableSuffix}`))
  const rustc = realpath(join(root, 'bin', `rustc${executableSuffix}`))
  for (const executable of [cargo, rustc]) {
    assertNoSymlinkPath(executable, options)
    sha256File(executable, options)
  }

  if (platform !== 'darwin') {
    throw new Error(`[rust-native-toolchain] Development native tools are unsupported for host ${key}`)
  }
  const discoveryEnvironment = {
    PATH: `${join(root, 'bin')}:/usr/bin:/bin`,
    HOME: home,
    CARGO_HOME: cargoHome,
    RUSTUP_HOME: rustupHome,
    LANG: 'C',
    LC_ALL: 'C',
    TZ: 'UTC'
  }
  const developerDirectoryCandidate = options.developerDirectory || boundedCommand(
    '/usr/bin/xcode-select', ['-p'], discoveryEnvironment, options
  )
  const sdkRootCandidate = options.sdkRoot || boundedCommand(
    '/usr/bin/xcrun', ['--show-sdk-path'], discoveryEnvironment, options
  )
  const developerDirectory = realpath(resolve(developerDirectoryCandidate))
  const sdkRoot = realpath(resolve(sdkRootCandidate))
  assertNoSymlinkPath(developerDirectory, options)
  assertNoSymlinkPath(sdkRoot, options)
  assertTrustedDirectory(developerDirectory, options)
  assertTrustedDirectory(sdkRoot, options)
  const tools = {}
  for (const name of ['ar', 'clang', 'codesign', 'ld']) {
    const candidate = options.toolPaths?.[name] || (name === 'codesign'
      ? '/usr/bin/codesign'
      : boundedCommand('/usr/bin/xcrun', ['--find', name], discoveryEnvironment, options))
    const executable = realpath(resolve(candidate))
    assertNoSymlinkPath(executable, options)
    sha256File(executable, options)
    tools[name] = executable
  }

  const environment = {
    ...discoveryEnvironment,
    PATH: `${join(root, 'bin')}:${join(developerDirectory, 'usr', 'bin')}:/usr/bin:/bin`,
    RUSTC: rustc,
    DEVELOPER_DIR: developerDirectory,
    SDKROOT: sdkRoot,
    CARGO_TERM_COLOR: 'always'
  }
  const cargoVersion = boundedCommand(cargo, ['--version'], environment, options)
  const rustcVersion = boundedCommand(rustc, ['--version'], environment, options)
  const rustcVerbose = boundedCommand(rustc, ['-vV'], environment, options)
  const expectedVersionPrefix = `${selectedLock.rustVersion}`
  const rustcHost = /^host:\s*(\S+)$/mu.exec(rustcVerbose)?.[1] || ''
  if (!cargoVersion.startsWith(`cargo ${expectedVersionPrefix} `) ||
      !rustcVersion.startsWith(`rustc ${expectedVersionPrefix} `) ||
      rustcHost !== locked.hostTriple) {
    throw new Error(`[rust-native-toolchain] Development Rustup identity mismatch for host ${key}`)
  }
  const tree = toolchainTreeIdentity(root, options)
  const sdkVersion = options.sdkVersion || boundedCommand(
    '/usr/bin/xcrun', ['--show-sdk-version'], environment, options
  )
  return {
    key,
    root,
    cargo,
    rustc,
    tools,
    developerDirectory,
    sdkRoot,
    sdkVersion,
    environment,
    tree,
    releaseEligible: false,
    trustClass: 'development_local_non_authoritative',
    lock: {
      cargoExecutableSha256: sha256File(cargo, options),
      cargoVersion,
      rustcExecutableSha256: sha256File(rustc, options),
      rustcVersion
    }
  }
}

function resolvePinnedRustToolchain(options = {}) {
  const key = hostKey(options.platform || process.platform, options.arch || process.arch)
  const selectedLock = options.lock || lock
  validateRustNativeToolchainLock(selectedLock)
  const locked = selectedLock.hosts?.[key]
  if (selectedLock.schemaVersion !== 1 || selectedLock.rustVersion !== lock.rustVersion || !locked ||
      !/^[0-9a-f]{64}$/.test(locked.cargoExecutableSha256 || '') ||
      !/^[0-9a-f]{64}$/.test(locked.rustcExecutableSha256 || '') ||
      !/^[0-9a-f]{64}$/.test(locked.toolchainTreeSha256 || '') ||
      !Number.isSafeInteger(locked.toolchainFileCount) || locked.toolchainFileCount <= 0) {
    throw new Error(`[rust-native-toolchain] No pinned Rust toolchain for host ${key}`)
  }
  const exists = options.existsSync || existsSync
  const realpath = options.realpathSync || realpathSync
  const rootCandidate = resolve(options.toolchainRoot || defaultToolchainRoot(locked, options))
  if (!exists(rootCandidate)) {
    throw new Error(`[rust-native-toolchain] Pinned Rust toolchain is unavailable for host ${key}`)
  }
  assertNoSymlinkPath(rootCandidate, options)
  const root = realpath(rootCandidate)
  assertTrustedDirectory(root, options)
  const cargo = realpath(join(root, 'bin', process.platform === 'win32' ? 'cargo.exe' : 'cargo'))
  const rustc = realpath(join(root, 'bin', process.platform === 'win32' ? 'rustc.exe' : 'rustc'))
  if (relative(root, cargo).startsWith(`..${sep}`) || relative(root, rustc).startsWith(`..${sep}`) ||
      sha256File(cargo, options) !== locked.cargoExecutableSha256 ||
      sha256File(rustc, options) !== locked.rustcExecutableSha256) {
    throw new Error(`[rust-native-toolchain] Pinned Rust executable identity mismatch for host ${key}`)
  }
  const tree = toolchainTreeIdentity(root, options)
  if (tree.fileCount !== locked.toolchainFileCount || tree.sha256 !== locked.toolchainTreeSha256) {
    throw new Error(`[rust-native-toolchain] Rust toolchain tree identity mismatch for host ${key}`)
  }

  const developerDirectory = resolve(locked.developerDirectory || '')
  const sdkRoot = resolve(locked.sdkRoot || '')
  assertNoSymlinkPath(developerDirectory, options)
  assertNoSymlinkPath(sdkRoot, options)
  assertTrustedDirectory(developerDirectory, options)
  assertTrustedDirectory(sdkRoot, options)
  const sdkSettings = {
    json: realpath(join(sdkRoot, 'SDKSettings.json')),
    plist: realpath(join(sdkRoot, 'SDKSettings.plist'))
  }
  if (sha256File(sdkSettings.json, options) !== locked.sdkSettingsJsonSha256 ||
      sha256File(sdkSettings.plist, options) !== locked.sdkSettingsPlistSha256 ||
      typeof locked.sdkVersion !== 'string' || !locked.sdkVersion) {
    throw new Error(`[rust-native-toolchain] Pinned SDK identity mismatch for host ${key}`)
  }
  const tools = {}
  for (const [name, expected] of Object.entries(locked.tools || {})) {
    if (!/^[0-9a-f]{64}$/.test(expected)) {
      throw new Error(`[rust-native-toolchain] Invalid pinned tool digest for ${name}`)
    }
    const candidate = options.toolPaths?.[name] || (name === 'codesign'
      ? join('/usr/bin', name)
      : join(developerDirectory, 'usr', 'bin', name))
    assertNoSymlinkPath(candidate, options)
    const executable = realpath(candidate)
    if (sha256File(executable, options) !== expected) {
      throw new Error(`[rust-native-toolchain] Pinned platform tool identity mismatch for ${name}`)
    }
    tools[name] = executable
  }
  for (const required of ['ar', 'clang', 'codesign', 'ld']) {
    if (!tools[required]) {
      throw new Error(`[rust-native-toolchain] Missing pinned platform tool: ${required}`)
    }
  }

  const home = options.home || homedir()
  const environment = {
    PATH: `${join(root, 'bin')}:${join(developerDirectory, 'usr', 'bin')}:/usr/bin:/bin`,
    HOME: home,
    LANG: 'C',
    LC_ALL: 'C',
    TZ: 'UTC',
    CARGO_HOME: join(home, '.cargo'),
    RUSTUP_HOME: join(home, '.rustup'),
    RUSTC: rustc,
    DEVELOPER_DIR: developerDirectory,
    SDKROOT: sdkRoot,
    CARGO_TERM_COLOR: 'always'
  }
  exactVersion(cargo, locked.cargoVersion, environment, options)
  exactVersion(rustc, locked.rustcVersion, environment, options)
  return {
    key,
    root,
    cargo,
    rustc,
    tools,
    developerDirectory,
    sdkRoot,
    sdkVersion: locked.sdkVersion,
    sdkSettings,
    environment,
    tree,
    lock: locked
  }
}

module.exports = {
  hostKey,
  lock,
  lockPath,
  nativeRustToolchainReleasePreflight,
  parseRustNativeToolchainLock,
  resolveDevelopmentRustToolchain,
  resolvePinnedRustToolchain,
  sha256File,
  toolchainTreeIdentity
}
