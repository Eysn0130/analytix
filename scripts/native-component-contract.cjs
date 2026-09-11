const crypto = require('node:crypto')
const {
  closeSync,
  constants,
  existsSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  openSync,
  readFileSync,
  readSync,
  realpathSync,
  readdirSync,
  renameSync,
  writeFileSync
} = require('node:fs')
const { spawnSync } = require('node:child_process')
const { homedir } = require('node:os')
const { delimiter, dirname, isAbsolute, join, relative, resolve, sep } = require('node:path')
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
const {
  projectDevelopmentCacheEnvironment
} = require('./lib/development-cache-environment.cjs')
const {
  nativeBuildProbeAuthorityIdentity,
  runNativeBuildProbe
} = require('./native-build-probe-authority.cjs')
const {
  resolvePinnedRustToolchain
} = require('./rust-native-toolchain-contract.cjs')

const manifestPath = join(__dirname, 'native-components.json')
const manifestBytes = readBoundedRegularFile(manifestPath, 256 * 1024)
const manifest = parseStrictJsonObject(manifestBytes, {
  maxBytes: 256 * 1024,
  maxDepth: 16,
  maxTokens: 8192,
  maxStringBytes: 64 * 1024,
  maxNumberBytes: 32,
  integerOnly: true
})
if (`${JSON.stringify(manifest, null, 2)}\n` !== manifestBytes.toString('utf8')) {
  throw new Error('[data-native] Native component registry must be canonical duplicate-free JSON')
}
const manifestDigest = crypto.createHash('sha256').update(manifestBytes).digest('hex')
const RECEIPT_FILE_NAME = 'analytix-native-components-receipt.json'
const RECEIPT_SCHEMA_VERSION = 6
const MAX_NATIVE_BINARY_BYTES = 2 * 1024 * 1024 * 1024
const FAT_MACH_O_PAYLOAD_DOMAIN = 'AnalytixFatMachOPayloadV1\0'
const MAX_FAT_MACH_O_SLICES = 32
const FROZEN_RUST_TOOLCHAIN_VERSION = '1.94.1'
const NATIVE_BUILD_CONTEXT_PATHS = Object.freeze([
  'scripts/build-data-analysis-native-tools.cjs',
  'scripts/native-component-contract.cjs',
  'scripts/native-build-probe-authority.cjs',
  'scripts/darwin-authority-launch-gate.cjs',
  'scripts/darwin-authority-launch-gate.json',
  'scripts/native/darwin-authority-launch-gate.c',
  'scripts/native-components.json',
  'scripts/lib/strict-json.cjs',
  'scripts/lib/development-cache-environment.cjs',
  'scripts/analytix-cache-storage.zsh',
  'scripts/use-analytix-cache.sh',
  'scripts/go-runtime-build-contract.cjs',
  'scripts/go-runtime-toolchain.json',
  'scripts/rust-native-toolchain-contract.cjs',
  'scripts/rust-native-toolchain.json',
  'packages/runtime-go/go.mod',
  'packages/runtime-go/go.sum',
  'packages/runtime-go/cmd/native-component-build-probe/main.go',
  'packages/runtime-go/internal/domain/artifactgeneration/contracts.go',
  'packages/runtime-go/internal/domain/jsonstrict/validate.go',
  'packages/runtime-go/internal/domain/nativebuild/cargo_execution.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentpublication/main_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentpublication/main_unsupported.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentpublication/publisher_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/manifest.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/registry.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/macho_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/main_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/main_unsupported.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/metadata_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/nativecomponentsnapshot/snapshot_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/store.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/store_unix.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/store_other.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/store_windows.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/extended_security_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/extended_security_linux.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/rename_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/securegeneration/rename_linux.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_bootstrap_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_cwd_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_identity.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/build_probe_metadata_darwin.go',
  'packages/runtime-go/internal/adapters/outbound/processauthority/spawn_darwin.s',
  '.cargo/config',
  '.cargo/config.toml',
  'rust-toolchain',
  'rust-toolchain.toml',
  'tools/.cargo/config',
  'tools/.cargo/config.toml',
  'tools/rust-toolchain',
  'tools/rust-toolchain.toml'
])
const NATIVE_BUILD_ENV_KEYS = Object.freeze([
  'CARGO_HOME',
  'DEVELOPER_DIR',
  'HOME',
  'INCLUDE',
  'LIB',
  'LIBPATH',
  'MACOSX_DEPLOYMENT_TARGET',
  'PATH',
  'RUSTUP_HOME',
  'SDKROOT',
  'SYSTEMROOT',
  'TEMP',
  'TMP',
  'TMPDIR',
  'TOOLCHAINS',
  'UNIVERSALCRTSDKDIR',
  'USERPROFILE',
  'VCINSTALLDIR',
  'VCTOOLSINSTALLDIR',
  'VISUALSTUDIOVERSION',
  'WINDIR',
  'WINDOWSSDKDIR',
  'WINDOWSSDKVERSION'
])
const FORBIDDEN_CARGO_ENV_KEYS = Object.freeze(new Set([
  'CARGO_BUILD_RUSTC',
  'CARGO_BUILD_RUSTC_WRAPPER',
  'CARGO_BUILD_RUSTC_WORKSPACE_WRAPPER',
  'CARGO_BUILD_TARGET',
  'CARGO_ENCODED_RUSTFLAGS',
  'CARGO',
  'RUSTFLAGS',
  'AR',
  'BINDGEN_EXTRA_CLANG_ARGS',
  'CC',
  'CL',
  'CFLAGS',
  'CXX',
  'CXXFLAGS',
  'DYLD_INSERT_LIBRARIES',
  'LD',
  'LDFLAGS',
  'LD_PRELOAD',
  'LINK',
  '_CL_',
  '_LINK_',
  'PKG_CONFIG_PATH',
  'RUSTC',
  'RUSTC_WRAPPER',
  'RUSTC_WORKSPACE_WRAPPER'
]))

const TARGETS = Object.freeze({
  'darwin-arm64': Object.freeze({ platform: 'darwin', arch: 'arm64', triple: 'aarch64-apple-darwin', format: 'mach-o' }),
  'darwin-x64': Object.freeze({ platform: 'darwin', arch: 'x64', triple: 'x86_64-apple-darwin', format: 'mach-o' }),
  'linux-x64': Object.freeze({ platform: 'linux', arch: 'x64', triple: 'x86_64-unknown-linux-gnu', format: 'elf' }),
  'win32-x64': Object.freeze({ platform: 'win32', arch: 'x64', triple: 'x86_64-pc-windows-msvc', format: 'pe' })
})

const FROZEN_COMPONENT_BOUNDARIES = Object.freeze([
  Object.freeze({
    id: 'import-accelerator',
    sourceRoot: 'tools/import_accelerator',
    binaryName: 'analytix-import-accelerator',
    role: 'immutable-data-import',
    consumers: Object.freeze([
      'backend/app/core/import_accelerator.py',
      'backend/app/core/native_component_paths.py',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go'
    ]),
    authorization: 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'
  }),
  Object.freeze({
    id: 'cleaning-ops',
    sourceRoot: 'tools/cleaning_ops',
    binaryName: 'analytix-cleaning-ops',
    role: 'case-data-cleaning',
    consumers: Object.freeze([
      'backend/app/repositories/cleaning_native_runtime.py',
      'backend/app/core/native_component_paths.py',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go'
    ]),
    authorization: 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'
  }),
  Object.freeze({
    id: 'analysis-compute',
    sourceRoot: 'tools/analysis_compute',
    binaryName: 'analytix-analysis-compute',
    role: 'bounded-analysis-compute',
    consumers: Object.freeze([
      'backend/app/core/analysis_compute_runner.py',
      'backend/app/core/native_component_paths.py',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go'
    ]),
    authorization: 'existing-data-plane-boundary@33d6de1d3c40691b73033f698af8bb518b15300d'
  }),
  Object.freeze({
    id: 'data-engine',
    sourceRoot: 'tools/data_engine',
    binaryName: 'analytix-data-engine',
    role: 'single-owner-case-database',
    consumers: Object.freeze([
      'backend/app/core/data_engine_client.py',
      'backend/app/core/db_engine.py',
      'backend/app/core/native_component_paths.py',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentregistry/load_darwin.go',
      'packages/runtime-go/internal/adapters/outbound/nativecomponentrunner/runner_darwin.go'
    ]),
    authorization: 'existing-data-plane-boundary@af0ea1967c76c7b771aaff1c0d5606ca4d5b832c'
  })
])

const FROZEN_EXECUTION_PROBES = Object.freeze({
  'import-accelerator': Object.freeze({
    schemaVersion: 1,
    authorityProtocol: 'analytix-native-build-probe-v1',
    componentProtocol: 'analytix-native-v1',
    policySha256: '0cc3872164fdadfb687a8642846913bf0f2394cd21c26799a5d483dee1ea8d2a',
    authorityTargets: Object.freeze(['darwin-arm64', 'darwin-x64'])
  }),
  'cleaning-ops': Object.freeze({
    schemaVersion: 1,
    authorityProtocol: 'analytix-native-build-probe-v1',
    componentProtocol: 'analytix-native-v1',
    policySha256: '59c4287d312b4972eb6890883865e3c37d226650caf78f65168363413ac85e35',
    authorityTargets: Object.freeze(['darwin-arm64', 'darwin-x64'])
  }),
  'analysis-compute': Object.freeze({
    schemaVersion: 1,
    authorityProtocol: 'analytix-native-build-probe-v1',
    componentProtocol: 'analytix-native-v1',
    policySha256: 'fd2227234c61de1505e50aa33e1e24c2f58c2545fe4d07e41b8064709c18f4d8',
    authorityTargets: Object.freeze(['darwin-arm64', 'darwin-x64'])
  }),
  'data-engine': Object.freeze({
    schemaVersion: 1,
    authorityProtocol: 'analytix-native-build-probe-v1',
    componentProtocol: 'analytix-native-v1',
    policySha256: '97ce13396fb6f5ba7e9d851ab4abc2f23ac8abdd13acaa3e983a326d6bf4a1e0',
    authorityTargets: Object.freeze(['darwin-arm64', 'darwin-x64'])
  })
})

function validateNativeComponentManifest(value) {
  if (
    !exactKeys(value, ['schemaVersion', 'receiptSchemaVersion', 'components']) ||
    value.schemaVersion !== 4 ||
    value.receiptSchemaVersion !== RECEIPT_SCHEMA_VERSION
  ) {
    throw new Error('[data-native] Native component registry schema is invalid')
  }
  if (!Array.isArray(value.components) || value.components.length !== FROZEN_COMPONENT_BOUNDARIES.length) {
    throw new Error('[data-native] Native component registry must contain exactly the authorized data-plane boundaries')
  }
  const expectedComponentKeys = [
    'id',
    'sourceRoot',
    'cargoManifest',
    'binaryName',
    'role',
    'agentCore',
    'consumers',
    'packagePath',
    'supportedTargets',
    'executionProbe',
    'authorization'
  ]
  const targetKeys = Object.keys(TARGETS)
  for (let index = 0; index < FROZEN_COMPONENT_BOUNDARIES.length; index += 1) {
    const expected = FROZEN_COMPONENT_BOUNDARIES[index]
    const component = value.components[index]
    if (!exactKeys(component, expectedComponentKeys)) {
      throw new Error(`[data-native] Native component registry entry ${index} has an invalid schema`)
    }
    for (const field of ['id', 'sourceRoot', 'binaryName', 'role', 'authorization']) {
      if (component[field] !== expected[field]) {
        throw new Error(`[data-native] Native component registry ${field} is not authorized for ${expected.id}`)
      }
    }
    if (
      component.cargoManifest !== `${expected.sourceRoot}/Cargo.toml` ||
      component.packagePath !== `runtime/${expected.binaryName}` ||
      component.agentCore !== false ||
      JSON.stringify(component.consumers) !== JSON.stringify(expected.consumers) ||
      JSON.stringify(component.supportedTargets) !== JSON.stringify(targetKeys)
    ) {
      throw new Error(`[data-native] Native component boundary is not frozen for ${expected.id}`)
    }
    const probe = component.executionProbe
    if (
      !exactKeys(probe, [
        'schemaVersion',
        'authorityProtocol',
        'componentProtocol',
        'policySha256',
        'authorityTargets'
      ]) || probe.schemaVersion !== 1 ||
      probe.authorityProtocol !== 'analytix-native-build-probe-v1' ||
      probe.componentProtocol !== 'analytix-native-v1' ||
      !/^[0-9a-f]{64}$/u.test(probe.policySha256) ||
      !Array.isArray(probe.authorityTargets) ||
      JSON.stringify(probe.authorityTargets) !== JSON.stringify(['darwin-arm64', 'darwin-x64'])
    ) {
      throw new Error(`[data-native] Native component execution probe is invalid for ${expected.id}`)
    }
    if (JSON.stringify(probe) !== JSON.stringify(FROZEN_EXECUTION_PROBES[expected.id])) {
      throw new Error(`[data-native] Native component execution probe is not frozen for ${expected.id}`)
    }
  }
  return value
}

validateNativeComponentManifest(manifest)

function deepFreeze(value) {
  if (!value || typeof value !== 'object' || Object.isFrozen(value)) return value
  for (const child of Object.values(value)) deepFreeze(child)
  return Object.freeze(value)
}

deepFreeze(manifest)

let cachedPinnedRustToolchain

function pinnedRustToolchainForReadOnlyContract() {
  if (!cachedPinnedRustToolchain) {
    cachedPinnedRustToolchain = resolvePinnedRustToolchain()
  }
  return cachedPinnedRustToolchain
}

function sha256Bytes(bytes) {
  return crypto.createHash('sha256').update(bytes).digest('hex')
}

function readBoundedRegularFile(path, maximumBytes) {
  if (!Number.isSafeInteger(maximumBytes) || maximumBytes <= 0) {
    throw new Error('[data-native] Bounded file limit is invalid')
  }
  let openFlags = constants.O_RDONLY
  if (typeof constants.O_CLOEXEC === 'number') openFlags |= constants.O_CLOEXEC
  if (process.platform !== 'win32') {
    if (typeof constants.O_NOFOLLOW !== 'number') {
      throw new Error('[data-native] No-follow file reads are unavailable on this host')
    }
    openFlags |= constants.O_NOFOLLOW
  }
  const descriptor = openSync(path, openFlags)
  try {
    const before = fstatSync(descriptor, { bigint: true })
    const size = Number(before.size)
    const ownerIsTrusted = process.platform === 'win32' || (
      typeof process.geteuid === 'function' &&
      (before.uid === 0n || before.uid === BigInt(process.geteuid()))
    )
    if (
      Number(before.mode & BigInt(constants.S_IFMT)) !== constants.S_IFREG || before.nlink !== 1n ||
      !Number.isSafeInteger(size) || size <= 0 || size > maximumBytes ||
      (Number(before.mode) & 0o022) !== 0 ||
      !ownerIsTrusted
    ) {
      throw new Error('[data-native] Bounded file is not a trusted regular file')
    }
    const body = readFileSync(descriptor)
    const after = fstatSync(descriptor, { bigint: true })
    if (
      body.length !== size || before.dev !== after.dev || before.ino !== after.ino ||
      before.mode !== after.mode || before.uid !== after.uid || before.gid !== after.gid ||
      before.nlink !== after.nlink || before.size !== after.size ||
      before.mtimeNs !== after.mtimeNs || before.ctimeNs !== after.ctimeNs
    ) {
      throw new Error('[data-native] Bounded file changed while it was read')
    }
    return body
  } finally {
    closeSync(descriptor)
  }
}

function sameStableFileIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.mode === right.mode &&
    left.uid === right.uid && left.gid === right.gid && left.nlink === right.nlink &&
    left.size === right.size && left.mtimeNs === right.mtimeNs && left.ctimeNs === right.ctimeNs
}

function sha256File(path, options = {}) {
  const maximumBytes = options.maximumBytes === undefined ? MAX_NATIVE_BINARY_BYTES : options.maximumBytes
  if (!Number.isSafeInteger(maximumBytes) || maximumBytes <= 0) {
    throw new Error('[data-native] Stable file hash limit is invalid')
  }
  let flags = constants.O_RDONLY
  if (typeof constants.O_CLOEXEC === 'number') flags |= constants.O_CLOEXEC
  if (process.platform !== 'win32') {
    if (typeof constants.O_NOFOLLOW !== 'number') {
      throw new Error('[data-native] No-follow file hashing is unavailable on this host')
    }
    flags |= constants.O_NOFOLLOW
  }
  let descriptor
  try {
    descriptor = openSync(path, flags)
    const before = fstatSync(descriptor, { bigint: true })
    const size = Number(before.size)
    const ownerIsTrusted = process.platform === 'win32' || (
      typeof process.geteuid === 'function' &&
      (before.uid === 0n || before.uid === BigInt(process.geteuid()))
    )
    if (
      Number(before.mode & BigInt(constants.S_IFMT)) !== constants.S_IFREG || before.nlink !== 1n ||
      !Number.isSafeInteger(size) || size < 0 || size > maximumBytes ||
      (Number(before.mode) & 0o022) !== 0 || !ownerIsTrusted ||
      (options.expectedStat && !sameStableFileIdentity(before, options.expectedStat))
    ) {
      throw new Error(`[data-native] File is not a stable trusted regular file: ${path}`)
    }
    const hash = crypto.createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let offset = 0
    while (offset < size) {
      const length = Math.min(buffer.length, size - offset)
      const count = readSync(descriptor, buffer, 0, length, offset)
      if (count <= 0 || count > length) {
        throw new Error(`[data-native] Stable file hash short read: ${path}`)
      }
      hash.update(buffer.subarray(0, count))
      offset += count
    }
    const after = fstatSync(descriptor, { bigint: true })
    if (!sameStableFileIdentity(before, after)) {
      throw new Error(`[data-native] File identity changed while hashing: ${path}`)
    }
    return hash.digest('hex')
  } finally {
    if (descriptor !== undefined) closeSync(descriptor)
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

function assertNoSymlinkPath(path, boundaryRoot, options = {}) {
  const resolvedPath = resolve(path)
  const resolvedBoundary = resolve(boundaryRoot)
  const relativePath = relative(resolvedBoundary, resolvedPath)
  if (relativePath === '..' || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error(`[data-native] Provenance path escapes its repository boundary: ${path}`)
  }
  const boundaryStat = lstatSync(resolvedBoundary)
  if (boundaryStat.isSymbolicLink() || !boundaryStat.isDirectory()) {
    throw new Error(`[data-native] Repository boundary must be a regular non-symlink directory: ${resolvedBoundary}`)
  }
  let current = resolvedBoundary
  for (const part of relativePath ? relativePath.split(sep) : []) {
    current = join(current, part)
    if (!pathEntryExists(current)) {
      if (options.allowMissing === true) return
      throw new Error(`[data-native] Provenance path is missing: ${current}`)
    }
    if (lstatSync(current).isSymbolicLink()) {
      throw new Error(`[data-native] Provenance path cannot traverse a symbolic link: ${current}`)
    }
  }
}

function collectSourceFiles(root, current = root, files = []) {
  for (const entry of readdirSync(current, { withFileTypes: true })) {
    const path = join(current, entry.name)
    if (entry.isSymbolicLink()) {
      throw new Error(`[data-native] Source provenance cannot contain a symbolic link: ${path}`)
    }
    if (entry.isDirectory() && current === root && (entry.name === 'target' || entry.name === '.git')) continue
    if (entry.isDirectory()) collectSourceFiles(root, path, files)
    else if (entry.isFile()) files.push(path)
    else throw new Error(`[data-native] Source provenance cannot contain a special file: ${path}`)
  }
  return files
}

function sourceTreeDigest(root, boundaryRoot = root) {
  if (!existsSync(root)) throw new Error(`[data-native] Source root is missing: ${root}`)
  assertNoSymlinkPath(root, boundaryRoot)
  const rootStat = lstatSync(root)
  if (rootStat.isSymbolicLink() || !rootStat.isDirectory()) {
    throw new Error(`[data-native] Source root must be a regular non-symlink directory: ${root}`)
  }
  const hash = crypto.createHash('sha256')
  const files = collectSourceFiles(root).sort((left, right) => Buffer.compare(
    Buffer.from(relative(root, left).split('\\').join('/'), 'utf8'),
    Buffer.from(relative(root, right).split('\\').join('/'), 'utf8')
  ))
  for (const path of files) {
    const stat = lstatSync(path, { bigint: true })
    hash.update(relative(root, path).split('\\').join('/'))
    hash.update('\0')
    hash.update(String(stat.size))
    hash.update('\0')
    hash.update(sha256File(path, { expectedStat: stat }))
    hash.update('\0')
  }
  return hash.digest('hex')
}

function nativeBuildContextDigest(repoRoot) {
  const hash = crypto.createHash('sha256')
  for (const relativePath of NATIVE_BUILD_CONTEXT_PATHS) {
    const path = join(repoRoot, relativePath)
    hash.update(relativePath)
    hash.update('\0')
    assertNoSymlinkPath(path, repoRoot, { allowMissing: true })
    if (!pathEntryExists(path)) {
      hash.update('missing\0')
      continue
    }
    const stat = lstatSync(path, { bigint: true })
    if (stat.isSymbolicLink() || !stat.isFile()) {
      throw new Error(`[data-native] Build context input must be a regular non-symlink file: ${path}`)
    }
    hash.update(String(stat.size))
    hash.update('\0')
    hash.update(sha256File(path, { expectedStat: stat }))
    hash.update('\0')
  }
  return hash.digest('hex')
}

function canonicalEnvironmentMap(env = process.env) {
  const values = new Map()
  for (const [name, rawValue] of Object.entries(env)) {
    const canonicalName = name.toUpperCase()
    if (values.has(canonicalName)) {
      throw new Error(`[data-native] Duplicate case-insensitive environment key is forbidden: ${canonicalName}`)
    }
    values.set(canonicalName, String(rawValue || ''))
  }
  return values
}

function assertHermeticCargoEnvironment(repoRoot, env = process.env) {
  const canonicalEnv = canonicalEnvironmentMap(env)
  const allowedCargoRustEnvironment = new Set([
    ...NATIVE_BUILD_ENV_KEYS,
    'RUST_BACKTRACE',
    'RUST_LOG'
  ])
  for (const [name, rawValue] of canonicalEnv) {
    if (!rawValue.trim()) continue
    if (
      FORBIDDEN_CARGO_ENV_KEYS.has(name) ||
      ((name.startsWith('CARGO_') || name.startsWith('RUST')) && !allowedCargoRustEnvironment.has(name)) ||
      /^(?:BINDGEN_EXTRA_CLANG_ARGS|CFLAGS|CXXFLAGS|LDFLAGS|OUT_DIR|PKG_CONFIG(?:_.+)?|PROFILE|OPT_LEVEL|TARGET)$/.test(name)
    ) {
      throw new Error(`[data-native] Cargo build environment override is forbidden: ${name}`)
    }
  }

  const configCandidates = new Set()
  let ancestor = dirname(resolve(repoRoot))
  while (true) {
    configCandidates.add(join(ancestor, '.cargo', 'config'))
    configCandidates.add(join(ancestor, '.cargo', 'config.toml'))
    const parent = dirname(ancestor)
    if (parent === ancestor) break
    ancestor = parent
  }
  const cargoHome = resolve(canonicalEnv.get('CARGO_HOME') || join(homedir(), '.cargo'))
  configCandidates.add(join(cargoHome, 'config'))
  configCandidates.add(join(cargoHome, 'config.toml'))
  for (const path of configCandidates) {
    if (pathEntryExists(path)) {
      throw new Error(`[data-native] Ambient Cargo configuration is forbidden; move it into the registered repository context: ${path}`)
    }
  }
}

function nativeBuildChildEnvironment(env = process.env) {
  const canonicalEnv = canonicalEnvironmentMap(env)
  const child = {}
  for (const name of NATIVE_BUILD_ENV_KEYS) {
    const value = canonicalEnv.get(name)
    if (value) child[name] = name === 'PATH' ? normalizeExecutableSearchPath(value) : value
  }
  child.CARGO_TERM_COLOR = 'always'
  return child
}

function normalizeExecutableSearchPath(value) {
  const seen = new Set()
  const entries = []
  for (const rawEntry of String(value || '').split(delimiter)) {
    const entry = rawEntry.trim()
    if (!entry) continue
    const normalized = resolve(entry)
    const key = process.platform === 'win32' ? normalized.toLowerCase() : normalized
    if (seen.has(key)) continue
    seen.add(key)
    entries.push(normalized)
  }
  return entries.join(delimiter)
}

function nativeBuildEnvironmentDigest(env = process.env) {
  assertHermeticCargoEnvironment(join(__dirname, '..'), env)
  return environmentDigest(pinnedRustToolchainForReadOnlyContract().environment)
}

function environmentDigest(environment) {
  const hash = crypto.createHash('sha256')
  for (const name of Object.keys(environment).sort()) {
    hash.update(name)
    hash.update('=')
    hash.update(String(environment[name] || ''))
    hash.update('\0')
  }
  return hash.digest('hex')
}

function componentRustFlags(component, target) {
  const flags = []
  if (target.platform === 'win32' && ['cleaning-ops', 'analysis-compute', 'data-engine'].includes(component.id)) {
    flags.push('-C', 'link-arg=Rstrtmgr.lib')
  }
  return flags.join(' ')
}

function nativeComponentBuildEnvironment(component, target, options = {}) {
  const environment = {
    ...(options.baseEnvironment || nativeBuildChildEnvironment(options.env || process.env)),
    RUSTC: options.rustcExecutable || resolveNativeToolchainExecutable('rustc', options.baseEnvironment || process.env),
    CARGO_TERM_COLOR: 'always'
  }
  const targetEnvironmentKey = target.triple.toUpperCase().replaceAll('-', '_')
  if (options.linkerExecutable) {
    environment[`CARGO_TARGET_${targetEnvironmentKey}_LINKER`] = options.linkerExecutable
    environment[`CC_${target.triple}`] = options.linkerExecutable
    environment[`CXX_${target.triple}`] = options.linkerExecutable
  }
  if (options.archiverExecutable) {
    environment[`AR_${target.triple}`] = options.archiverExecutable
  }
  if (options.cargoTargetDirectory) environment.CARGO_TARGET_DIR = resolve(options.cargoTargetDirectory)
  const flags = componentRustFlags(component, target)
  if (flags) environment.RUSTFLAGS = flags
  return environment
}

function nativeComponentBuildEnvironmentDigest(environment) {
  const projected = { ...environment }
  if (Object.hasOwn(projected, 'CARGO_TARGET_DIR')) {
    projected.CARGO_TARGET_DIR = '<analytix-isolated-cargo-target>'
  }
  return environmentDigest(projected)
}

function resolveNativeToolchainExecutable(command, env = process.env) {
  if (command !== 'cargo' && command !== 'rustc') {
    throw new Error(`[data-native] Unregistered native toolchain command: ${command}`)
  }
  void env
  const toolchain = pinnedRustToolchainForReadOnlyContract()
  return command === 'cargo' ? toolchain.cargo : toolchain.rustc
}

function nativeToolchainIdentity(options = {}) {
  const env = options.env || process.env
  void env
  const toolchain = options.toolchain || pinnedRustToolchainForReadOnlyContract()
  const commands = [
    ['cargoVersion', 'cargo', toolchain.cargo, toolchain.lock.cargoVersion],
    ['rustcVersion', 'rustc', toolchain.rustc, toolchain.lock.rustcVersion]
  ]
  const identity = {}
  for (const [field, command, executablePath, expectedVersion] of commands) {
    const result = spawnSync(executablePath, ['--version'], {
      encoding: 'utf8',
      env: toolchain.environment,
      windowsHide: true
    })
    if (result.error) throw result.error
    if (result.status !== 0 || result.signal) {
      throw new Error(`[data-native] Cannot identify ${command}: exit=${result.status} signal=${result.signal || 'none'}`)
    }
    const value = String(result.stdout || '').trim()
    if (!value || value.length > 16_384 || value !== expectedVersion) {
      throw new Error(`[data-native] Invalid ${command} identity output`)
    }
    identity[field] = value
    identity[`${field.replace(/Version$/, '')}ExecutableSha256`] = toolchain.lock[`${command}ExecutableSha256`]
  }
  return identity
}

function sourceSetDigest(repoRoot) {
  const hash = crypto.createHash('sha256')
  for (const component of manifest.components) {
    hash.update(component.id)
    hash.update('\0')
    hash.update(sourceTreeDigest(join(repoRoot, component.sourceRoot), repoRoot))
    hash.update('\0')
  }
  hash.update('build-context\0')
  hash.update(nativeBuildContextDigest(repoRoot))
  hash.update('\0')
  return hash.digest('hex')
}

// Builds the existing source-set digest from host-issued Go snapshot receipts.
// These receipts, rather than a second Node path walk, are authoritative for
// component source and Cargo.lock provenance during native publication.
function sourceSetDigestFromComponentReceipts(receipts, buildContextDigest) {
  if (
    !Array.isArray(receipts) || receipts.length !== manifest.components.length ||
    !/^[0-9a-f]{64}$/u.test(String(buildContextDigest || ''))
  ) {
    throw new Error('[data-native] Native source snapshot receipts are invalid')
  }
  const hash = crypto.createHash('sha256')
  for (let index = 0; index < manifest.components.length; index += 1) {
    const expected = manifest.components[index]
    const receipt = receipts[index]
    if (
      !exactKeys(receipt, [
        'id', 'source_root', 'source_digest', 'cargo_lock_sha256', 'file_count', 'total_bytes'
      ]) || receipt.id !== expected.id || receipt.source_root !== expected.sourceRoot ||
      !/^[0-9a-f]{64}$/u.test(String(receipt.source_digest || '')) ||
      !/^[0-9a-f]{64}$/u.test(String(receipt.cargo_lock_sha256 || '')) ||
      !Number.isSafeInteger(receipt.file_count) || receipt.file_count <= 0 ||
      !Number.isSafeInteger(receipt.total_bytes) || receipt.total_bytes <= 0
    ) {
      throw new Error(`[data-native] Native source snapshot receipt is invalid for ${expected.id}`)
    }
    hash.update(receipt.id)
    hash.update('\0')
    hash.update(receipt.source_digest)
    hash.update('\0')
  }
  hash.update('build-context\0')
  hash.update(buildContextDigest)
  hash.update('\0')
  return hash.digest('hex')
}

function normalizePlatform(value) {
  const platform = String(value || process.platform).trim().toLowerCase()
  return platform === 'win' || platform === 'windows' ? 'win32' : platform
}

function normalizeArch(value) {
  const arch = String(value || process.arch).trim().toLowerCase()
  if (arch === 'amd64' || arch === 'x86_64') return 'x64'
  if (arch === 'aarch64') return 'arm64'
  return arch
}

function targetKey(platform, arch) {
  return `${normalizePlatform(platform)}-${normalizeArch(arch)}`
}

function targetContract(platform, arch) {
  const key = targetKey(platform, arch)
  const target = TARGETS[key]
  if (!target) throw new Error(`[data-native] Unsupported package target: ${key}`)
  return { key, ...target }
}

function binaryName(component, platform) {
  return normalizePlatform(platform) === 'win32' ? `${component.binaryName}.exe` : component.binaryName
}

function stageDirectory(repoRoot, platform, arch) {
  return join(repoRoot, 'runtime', 'native-components', targetKey(platform, arch))
}

function inspectMachO(bytes) {
  if (bytes.length < 32) return null
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  let readUInt32
  let readUInt64
  if (magicBE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32BE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64BE(offset)
  } else if (magicLE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32LE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64LE(offset)
  } else return null
  const cpu = readUInt32(4)
  const fileType = readUInt32(12)
  const commandCount = readUInt32(16)
  const commandsSize = readUInt32(20)
  if (fileType !== 2 || commandCount === 0 || commandCount > 4096 || commandsSize < 8) {
    throw new Error('[data-native] Mach-O executable header is invalid')
  }
  const commandsEnd = 32 + commandsSize
  if (commandsEnd > bytes.length) throw new Error('[data-native] Mach-O load commands are truncated')
  let commandOffset = 32
  let executableSegment = false
  for (let index = 0; index < commandCount; index += 1) {
    if (commandOffset + 8 > commandsEnd) throw new Error('[data-native] Mach-O load command header is truncated')
    const command = readUInt32(commandOffset)
    const commandSize = readUInt32(commandOffset + 4)
    if (commandSize < 8 || commandSize % 4 !== 0 || commandOffset + commandSize > commandsEnd) {
      throw new Error('[data-native] Mach-O load command size is invalid')
    }
    if (command === 0x19) {
      if (commandSize < 72) throw new Error('[data-native] Mach-O segment command is truncated')
      const fileOffset = Number(readUInt64(commandOffset + 40))
      const fileSize = Number(readUInt64(commandOffset + 48))
      const initialProtection = readUInt32(commandOffset + 60)
      if (!Number.isSafeInteger(fileOffset) || !Number.isSafeInteger(fileSize) || fileOffset + fileSize > bytes.length) {
        throw new Error('[data-native] Mach-O segment file range is invalid')
      }
      if ((initialProtection & 0x4) !== 0 && fileSize > 0) executableSegment = true
    }
    commandOffset += commandSize
  }
  if (commandOffset !== commandsEnd || !executableSegment) {
    throw new Error('[data-native] Mach-O executable segment is missing or malformed')
  }
  if (cpu === 0x01000007) return { format: 'mach-o', arch: 'x64' }
  if (cpu === 0x0100000c) return { format: 'mach-o', arch: 'arm64' }
  return { format: 'mach-o', arch: `cpu-${cpu}` }
}

function inspectPE(bytes) {
  if (bytes.length < 0x40 || bytes[0] !== 0x4d || bytes[1] !== 0x5a) return null
  const offset = bytes.readUInt32LE(0x3c)
  if (offset < 0x40 || offset + 24 > bytes.length || bytes.toString('ascii', offset, offset + 4) !== 'PE\0\0') return null
  const machine = bytes.readUInt16LE(offset + 4)
  const sectionCount = bytes.readUInt16LE(offset + 6)
  const optionalHeaderSize = bytes.readUInt16LE(offset + 20)
  const characteristics = bytes.readUInt16LE(offset + 22)
  const optionalHeaderOffset = offset + 24
  const optionalHeaderEnd = optionalHeaderOffset + optionalHeaderSize
  if (
    sectionCount === 0 ||
    sectionCount > 96 ||
    optionalHeaderSize < 112 ||
    optionalHeaderEnd > bytes.length ||
    bytes.readUInt16LE(optionalHeaderOffset) !== 0x20b ||
    (characteristics & 0x0002) === 0 ||
    bytes.readUInt32LE(optionalHeaderOffset + 16) === 0
  ) {
    throw new Error('[data-native] PE32+ executable header is invalid')
  }
  const sizeOfHeaders = bytes.readUInt32LE(optionalHeaderOffset + 60)
  const sectionTableOffset = optionalHeaderEnd
  if (sizeOfHeaders === 0 || sizeOfHeaders > bytes.length || sectionTableOffset + sectionCount * 40 > bytes.length) {
    throw new Error('[data-native] PE section table is truncated')
  }
  let executableSection = false
  for (let index = 0; index < sectionCount; index += 1) {
    const sectionOffset = sectionTableOffset + index * 40
    const rawSize = bytes.readUInt32LE(sectionOffset + 16)
    const rawOffset = bytes.readUInt32LE(sectionOffset + 20)
    const sectionCharacteristics = bytes.readUInt32LE(sectionOffset + 36)
    if (rawSize > 0 && (rawOffset < sizeOfHeaders || rawOffset + rawSize > bytes.length)) {
      throw new Error('[data-native] PE section file range is invalid')
    }
    if ((sectionCharacteristics & 0x20000000) !== 0 && rawSize > 0) executableSection = true
  }
  if (!executableSection) throw new Error('[data-native] PE executable section is missing')
  if (machine === 0x8664) return { format: 'pe', arch: 'x64' }
  if (machine === 0xaa64) return { format: 'pe', arch: 'arm64' }
  return { format: 'pe', arch: `machine-${machine}` }
}

function inspectELF(bytes) {
  if (bytes.length < 64 || bytes[0] !== 0x7f || bytes.toString('ascii', 1, 4) !== 'ELF') return null
  if (bytes[4] !== 2 || (bytes[5] !== 1 && bytes[5] !== 2) || bytes[6] !== 1) {
    throw new Error('[data-native] ELF64 identification is invalid')
  }
  const littleEndian = bytes[5] === 1
  const readUInt16 = littleEndian
    ? (offset) => bytes.readUInt16LE(offset)
    : (offset) => bytes.readUInt16BE(offset)
  const readUInt32 = littleEndian
    ? (offset) => bytes.readUInt32LE(offset)
    : (offset) => bytes.readUInt32BE(offset)
  const readUInt64 = littleEndian
    ? (offset) => bytes.readBigUInt64LE(offset)
    : (offset) => bytes.readBigUInt64BE(offset)
  const fileType = readUInt16(16)
  const machine = readUInt16(18)
  const programHeaderOffset = Number(readUInt64(32))
  const headerSize = readUInt16(52)
  const programHeaderSize = readUInt16(54)
  const programHeaderCount = readUInt16(56)
  if (
    (fileType !== 2 && fileType !== 3) ||
    readUInt32(20) !== 1 ||
    headerSize < 64 ||
    programHeaderSize < 56 ||
    programHeaderCount === 0 ||
    !Number.isSafeInteger(programHeaderOffset) ||
    programHeaderOffset < headerSize ||
    programHeaderOffset + programHeaderSize * programHeaderCount > bytes.length
  ) {
    throw new Error('[data-native] ELF64 executable header is invalid')
  }
  let executableSegment = false
  for (let index = 0; index < programHeaderCount; index += 1) {
    const headerOffset = programHeaderOffset + index * programHeaderSize
    const segmentType = readUInt32(headerOffset)
    const flags = readUInt32(headerOffset + 4)
    const fileOffset = Number(readUInt64(headerOffset + 8))
    const fileSize = Number(readUInt64(headerOffset + 32))
    if (
      !Number.isSafeInteger(fileOffset) ||
      !Number.isSafeInteger(fileSize) ||
      fileOffset + fileSize > bytes.length
    ) {
      throw new Error('[data-native] ELF64 segment file range is invalid')
    }
    if (segmentType === 1 && (flags & 0x1) !== 0 && fileSize > 0) executableSegment = true
  }
  if (!executableSegment) throw new Error('[data-native] ELF64 executable segment is missing')
  if (machine === 62) return { format: 'elf', arch: 'x64' }
  if (machine === 183) return { format: 'elf', arch: 'arm64' }
  return { format: 'elf', arch: `machine-${machine}` }
}

function thinMachOPayloadIdentity(bytes) {
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  let readUInt32
  let readUInt64
  let writeUInt32
  let writeUInt64
  if (magicBE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32BE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64BE(offset)
    writeUInt32 = (target, value, offset) => target.writeUInt32BE(value, offset)
    writeUInt64 = (target, value, offset) => target.writeBigUInt64BE(value, offset)
  } else if (magicLE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32LE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64LE(offset)
    writeUInt32 = (target, value, offset) => target.writeUInt32LE(value, offset)
    writeUInt64 = (target, value, offset) => target.writeBigUInt64LE(value, offset)
  } else {
    throw new Error('[data-native] Mach-O payload identity requires a thin 64-bit executable')
  }

  const commandCount = readUInt32(16)
  const commandsSize = readUInt32(20)
  const commandsEnd = 32 + commandsSize
  if (commandCount === 0 || commandCount > 4096 || commandsSize < 8 || commandsEnd > bytes.length) {
    throw new Error('[data-native] Mach-O payload load commands are invalid')
  }

  let commandOffset = 32
  const codeSignatures = []
  const linkEditSegments = []
  for (let index = 0; index < commandCount; index += 1) {
    if (commandOffset + 8 > commandsEnd) throw new Error('[data-native] Mach-O payload load command is truncated')
    const command = readUInt32(commandOffset)
    const commandSize = readUInt32(commandOffset + 4)
    if (commandSize < 8 || commandSize % 4 !== 0 || commandOffset + commandSize > commandsEnd) {
      throw new Error('[data-native] Mach-O payload load command size is invalid')
    }
    if (command === 0x19) {
      if (commandSize < 72) throw new Error('[data-native] Mach-O payload segment command is truncated')
      const name = bytes.toString('ascii', commandOffset + 8, commandOffset + 24).replace(/\0.*$/s, '')
      if (name === '__LINKEDIT') {
        const fileOffset = Number(readUInt64(commandOffset + 40))
        const fileSize = Number(readUInt64(commandOffset + 48))
        if (!Number.isSafeInteger(fileOffset) || !Number.isSafeInteger(fileSize)) {
          throw new Error('[data-native] Mach-O __LINKEDIT range is unsafe')
        }
        linkEditSegments.push({ commandOffset, fileOffset, fileSize })
      }
    } else if (command === 0x1d) {
      if (commandSize !== 16) throw new Error('[data-native] Mach-O LC_CODE_SIGNATURE command size is invalid')
      codeSignatures.push({
        commandOffset,
        dataOffset: readUInt32(commandOffset + 8),
        dataSize: readUInt32(commandOffset + 12)
      })
    }
    commandOffset += commandSize
  }
  if (commandOffset !== commandsEnd || codeSignatures.length !== 1 || linkEditSegments.length !== 1) {
    throw new Error('[data-native] Mach-O payload requires exactly one __LINKEDIT and LC_CODE_SIGNATURE')
  }

  const signature = codeSignatures[0]
  const linkEdit = linkEditSegments[0]
  if (
    signature.dataSize < 12 ||
    signature.dataOffset < commandsEnd ||
    signature.dataOffset % 8 !== 0 ||
    signature.dataOffset + signature.dataSize !== bytes.length ||
    linkEdit.fileOffset > signature.dataOffset ||
    linkEdit.fileOffset + linkEdit.fileSize !== bytes.length
  ) {
    throw new Error('[data-native] Mach-O code-signature range is invalid or ambiguous')
  }
  const superBlobSize = bytes.readUInt32BE(signature.dataOffset + 4)
  if (
    bytes.readUInt32BE(signature.dataOffset) !== 0xfade0cc0 ||
    superBlobSize < 12 ||
    superBlobSize > signature.dataSize
  ) {
    throw new Error('[data-native] Mach-O embedded code-signature superblob is invalid')
  }

  const payload = Buffer.from(bytes.subarray(0, signature.dataOffset))
  const linkEditPayloadSize = BigInt(signature.dataOffset - linkEdit.fileOffset)
  writeUInt64(payload, linkEditPayloadSize, linkEdit.commandOffset + 32)
  writeUInt64(payload, linkEditPayloadSize, linkEdit.commandOffset + 48)
  writeUInt32(payload, 0, signature.commandOffset + 12)
  return { sha256: sha256Bytes(payload), size: payload.length }
}

function zeroFilledRange(bytes, start, end) {
  for (let offset = start; offset < end; offset += 1) {
    if (bytes[offset] !== 0) return false
  }
  return true
}

function signingAgnosticMachOSlicePayloadIdentity(bytes) {
  const magicBE = bytes.readUInt32BE(0)
  const magicLE = bytes.readUInt32LE(0)
  let readUInt32
  let readUInt64
  let writeUInt32
  let writeUInt64
  if (magicBE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32BE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64BE(offset)
    writeUInt32 = (target, value, offset) => target.writeUInt32BE(value, offset)
    writeUInt64 = (target, value, offset) => target.writeBigUInt64BE(value, offset)
  } else if (magicLE === 0xfeedfacf) {
    readUInt32 = (offset) => bytes.readUInt32LE(offset)
    readUInt64 = (offset) => bytes.readBigUInt64LE(offset)
    writeUInt32 = (target, value, offset) => target.writeUInt32LE(value, offset)
    writeUInt64 = (target, value, offset) => target.writeBigUInt64LE(value, offset)
  } else {
    throw new Error('[data-native] Fat Mach-O slice is not a thin 64-bit image')
  }

  const commandCount = readUInt32(16)
  const commandsSize = readUInt32(20)
  const commandsEnd = 32 + commandsSize
  if (commandCount === 0 || commandCount > 4096 || commandsSize < 8 || commandsEnd > bytes.length) {
    throw new Error('[data-native] Fat Mach-O slice load commands are invalid')
  }
  let commandOffset = 32
  let linkEdit = null
  let signature = null
  for (let index = 0; index < commandCount; index += 1) {
    if (commandOffset + 8 > commandsEnd) {
      throw new Error('[data-native] Fat Mach-O slice load command is truncated')
    }
    const command = readUInt32(commandOffset)
    const commandSize = readUInt32(commandOffset + 4)
    if (commandSize < 8 || commandSize % 4 !== 0 || commandOffset + commandSize > commandsEnd) {
      throw new Error('[data-native] Fat Mach-O slice load command size is invalid')
    }
    if (command === 0x19) {
      if (commandSize < 72) throw new Error('[data-native] Fat Mach-O slice segment is truncated')
      const name = bytes.toString('ascii', commandOffset + 8, commandOffset + 24).replace(/\0.*$/s, '')
      if (name === '__LINKEDIT') {
        if (linkEdit) throw new Error('[data-native] Fat Mach-O slice has multiple __LINKEDIT segments')
        linkEdit = {
          commandOffset,
          fileOffset: Number(readUInt64(commandOffset + 40)),
          fileSize: Number(readUInt64(commandOffset + 48))
        }
      }
    } else if (command === 0x1d) {
      if (commandSize !== 16 || signature) {
        throw new Error('[data-native] Fat Mach-O slice code-signature command is invalid')
      }
      signature = {
        commandOffset,
        dataOffset: readUInt32(commandOffset + 8),
        dataSize: readUInt32(commandOffset + 12)
      }
    }
    commandOffset += commandSize
  }
  if (commandOffset !== commandsEnd || !linkEdit ||
    !Number.isSafeInteger(linkEdit.fileOffset) || !Number.isSafeInteger(linkEdit.fileSize)) {
    throw new Error('[data-native] Fat Mach-O slice __LINKEDIT range is invalid')
  }

  let payloadEnd = bytes.length
  if (signature) {
    if (signature.commandOffset + 16 !== commandsEnd || signature.dataSize < 12 ||
      signature.dataOffset < commandsEnd || signature.dataOffset % 8 !== 0 ||
      signature.dataOffset + signature.dataSize !== bytes.length ||
      bytes.readUInt32BE(signature.dataOffset) !== 0xfade0cc0) {
      throw new Error('[data-native] Fat Mach-O slice code-signature range is invalid')
    }
    const superBlobSize = bytes.readUInt32BE(signature.dataOffset + 4)
    if (superBlobSize < 12 || superBlobSize > signature.dataSize) {
      throw new Error('[data-native] Fat Mach-O slice code-signature superblob is invalid')
    }
    payloadEnd = signature.dataOffset
  }
  if (linkEdit.fileOffset > payloadEnd ||
    linkEdit.fileOffset + linkEdit.fileSize !== bytes.length) {
    throw new Error('[data-native] Fat Mach-O slice payload range is ambiguous')
  }

  const payload = Buffer.from(bytes.subarray(0, payloadEnd))
  const normalizedCommandsSize = commandsSize - (signature ? 16 : 0)
  const normalizedCommandCount = commandCount - (signature ? 1 : 0)
  if (normalizedCommandCount === 0 || normalizedCommandsSize < 8) {
    throw new Error('[data-native] Fat Mach-O slice normalized load commands are invalid')
  }
  writeUInt32(payload, normalizedCommandCount, 16)
  writeUInt32(payload, normalizedCommandsSize, 20)
  if (signature) payload.fill(0, signature.commandOffset, signature.commandOffset + 16)
  const linkEditPayloadSize = BigInt(payloadEnd - linkEdit.fileOffset)
  writeUInt64(payload, linkEditPayloadSize, linkEdit.commandOffset + 32)
  writeUInt64(payload, linkEditPayloadSize, linkEdit.commandOffset + 48)
  return { sha256: sha256Bytes(payload), size: payload.length }
}

function fatMachOPayloadIdentity(bytes, fat64) {
  const entrySize = fat64 ? 32 : 20
  if (bytes.length < 8) throw new Error('[data-native] Fat Mach-O payload header is truncated')
  const count = bytes.readUInt32BE(4)
  const headerEnd = 8 + count * entrySize
  if (count === 0 || count > MAX_FAT_MACH_O_SLICES || headerEnd > bytes.length) {
    throw new Error('[data-native] Fat Mach-O payload architecture table is invalid')
  }

  const records = []
  const ranges = []
  const seen = new Set()
  let payloadSize = 0
  for (let index = 0; index < count; index += 1) {
    const entryOffset = 8 + index * entrySize
    const cpuType = bytes.readUInt32BE(entryOffset)
    const cpuSubtype = bytes.readUInt32BE(entryOffset + 4)
    const sliceOffset = fat64
      ? Number(bytes.readBigUInt64BE(entryOffset + 8))
      : bytes.readUInt32BE(entryOffset + 8)
    const sliceSize = fat64
      ? Number(bytes.readBigUInt64BE(entryOffset + 16))
      : bytes.readUInt32BE(entryOffset + 12)
    const alignmentPower = bytes.readUInt32BE(entryOffset + (fat64 ? 24 : 16))
    if (fat64 && bytes.readUInt32BE(entryOffset + 28) !== 0) {
      throw new Error('[data-native] Fat Mach-O payload reserved field is non-zero')
    }
    if (!Number.isSafeInteger(sliceOffset) || !Number.isSafeInteger(sliceSize) ||
      sliceSize < 32 || alignmentPower > 30 || sliceOffset < headerEnd ||
      sliceOffset % (2 ** alignmentPower) !== 0 ||
      sliceOffset > bytes.length || sliceSize > bytes.length - sliceOffset) {
      throw new Error('[data-native] Fat Mach-O payload slice range is invalid')
    }

    const slice = bytes.subarray(sliceOffset, sliceOffset + sliceSize)
    const magicBE = slice.readUInt32BE(0)
    const magicLE = slice.readUInt32LE(0)
    const readUInt32 = magicBE === 0xfeedfacf
      ? (offset) => slice.readUInt32BE(offset)
      : magicLE === 0xfeedfacf
        ? (offset) => slice.readUInt32LE(offset)
        : null
    if (!readUInt32 || readUInt32(4) !== cpuType || readUInt32(8) !== cpuSubtype) {
      throw new Error('[data-native] Fat Mach-O payload slice identity is inconsistent')
    }
    const key = `${cpuType}:${cpuSubtype}`
    if (seen.has(key)) throw new Error('[data-native] Fat Mach-O payload contains a duplicate architecture')
    seen.add(key)

    const payload = signingAgnosticMachOSlicePayloadIdentity(slice)
    payloadSize += payload.size
    if (!Number.isSafeInteger(payloadSize) || payloadSize > MAX_NATIVE_BINARY_BYTES) {
      throw new Error('[data-native] Fat Mach-O normalized payload exceeds its byte bound')
    }
    records.push({
      cpuType,
      cpuSubtype,
      payloadSha256: payload.sha256,
      payloadByteLength: payload.size
    })
    ranges.push({ start: sliceOffset, end: sliceOffset + sliceSize })
  }

  ranges.sort((left, right) => left.start - right.start)
  let cursor = headerEnd
  for (const range of ranges) {
    if (range.start < cursor || !zeroFilledRange(bytes, cursor, range.start)) {
      throw new Error('[data-native] Fat Mach-O payload padding or overlap is invalid')
    }
    cursor = range.end
  }
  if (!zeroFilledRange(bytes, cursor, bytes.length)) {
    throw new Error('[data-native] Fat Mach-O payload trailing bytes are invalid')
  }

  records.sort((left, right) =>
    left.cpuType - right.cpuType || left.cpuSubtype - right.cpuSubtype
  )
  const manifest = JSON.stringify({ schemaVersion: 1, slices: records })
  return {
    sha256: crypto.createHash('sha256')
      .update(FAT_MACH_O_PAYLOAD_DOMAIN, 'utf8')
      .update(manifest, 'utf8')
      .digest('hex'),
    size: payloadSize
  }
}

function machOPayloadIdentity(bytes) {
  if (bytes.length < 4) {
    throw new Error('[data-native] Mach-O payload identity is truncated')
  }
  const magicBE = bytes.readUInt32BE(0)
  if (magicBE === 0xcafebabe || magicBE === 0xcafebabf) {
    return fatMachOPayloadIdentity(bytes, magicBE === 0xcafebabf)
  }
  return thinMachOPayloadIdentity(bytes)
}

function pePayloadIdentity(bytes) {
  const peOffset = bytes.readUInt32LE(0x3c)
  const optionalHeaderSize = bytes.readUInt16LE(peOffset + 20)
  const optionalHeaderOffset = peOffset + 24
  const optionalHeaderEnd = optionalHeaderOffset + optionalHeaderSize
  const checksumOffset = optionalHeaderOffset + 64
  const directoryCount = bytes.readUInt32LE(optionalHeaderOffset + 108)
  const certificateDirectoryOffset = optionalHeaderOffset + 112 + 4 * 8
  if (
    optionalHeaderSize < 152 ||
    optionalHeaderEnd > bytes.length ||
    directoryCount < 5 ||
    certificateDirectoryOffset + 8 > optionalHeaderEnd
  ) {
    throw new Error('[data-native] PE Authenticode directory is unavailable')
  }

  const sectionCount = bytes.readUInt16LE(peOffset + 6)
  let sectionEnd = 0
  for (let index = 0; index < sectionCount; index += 1) {
    const sectionOffset = optionalHeaderEnd + index * 40
    const rawSize = bytes.readUInt32LE(sectionOffset + 16)
    const rawOffset = bytes.readUInt32LE(sectionOffset + 20)
    if (rawSize > 0) sectionEnd = Math.max(sectionEnd, rawOffset + rawSize)
  }
  const certificateOffset = bytes.readUInt32LE(certificateDirectoryOffset)
  const certificateSize = bytes.readUInt32LE(certificateDirectoryOffset + 4)
  if ((certificateOffset === 0) !== (certificateSize === 0)) {
    throw new Error('[data-native] PE certificate table offset and size must be paired')
  }

  let payloadEnd = bytes.length
  if (certificateOffset === 0) {
    if (sectionEnd !== bytes.length) {
      throw new Error('[data-native] Unsigned PE payload contains an unsupported overlay or gap')
    }
  } else {
    if (
      certificateOffset % 8 !== 0 ||
      certificateSize < 8 ||
      certificateSize % 8 !== 0 ||
      certificateOffset !== sectionEnd ||
      certificateOffset + certificateSize !== bytes.length
    ) {
      throw new Error('[data-native] PE certificate table range is invalid or ambiguous')
    }
    let cursor = certificateOffset
    while (cursor < bytes.length) {
      if (cursor + 8 > bytes.length) throw new Error('[data-native] PE WIN_CERTIFICATE header is truncated')
      const recordLength = bytes.readUInt32LE(cursor)
      const revision = bytes.readUInt16LE(cursor + 4)
      const certificateType = bytes.readUInt16LE(cursor + 6)
      const alignedLength = recordLength
      if (
        recordLength < 8 ||
        recordLength % 8 !== 0 ||
        cursor + alignedLength > bytes.length ||
        ![0x0100, 0x0200].includes(revision) ||
        certificateType !== 0x0002
      ) {
        throw new Error('[data-native] PE WIN_CERTIFICATE record is invalid')
      }
      cursor += alignedLength
    }
    if (cursor !== bytes.length) throw new Error('[data-native] PE certificate table walk is incomplete')
    payloadEnd = certificateOffset
  }

  const payload = Buffer.from(bytes.subarray(0, payloadEnd))
  payload.fill(0, checksumOffset, checksumOffset + 4)
  payload.fill(0, certificateDirectoryOffset, certificateDirectoryOffset + 8)
  return { sha256: sha256Bytes(payload), size: payload.length }
}

function nativePayloadIdentity(bytes, format) {
  if (format === 'mach-o') return machOPayloadIdentity(bytes)
  if (format === 'pe') return pePayloadIdentity(bytes)
  if (format === 'elf') return { sha256: sha256Bytes(bytes), size: bytes.length }
  throw new Error(`[data-native] Unsupported native payload format: ${format}`)
}

function runExecutionProbe(path, component) {
  return runNativeBuildProbe(resolve(path), component, manifestDigest)
}

function verifyDarwinCodeSignature(path, options = {}) {
  const run = options.spawnSync || spawnSync
  const expectedTeamIdentifier = options.expectedTeamIdentifier || ''
  if (expectedTeamIdentifier && !/^[A-Z0-9]{10}$/.test(expectedTeamIdentifier)) {
    throw new Error('[data-native] Expected Apple TeamIdentifier is invalid')
  }
  const verifyArgs = ['--verify', '--strict', '--verbose=2']
  if (expectedTeamIdentifier) {
    verifyArgs.push(
      `-R=anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "${expectedTeamIdentifier}"`
    )
  }
  verifyArgs.push(path)
  const verify = run('/usr/bin/codesign', verifyArgs, {
    encoding: 'utf8'
  })
  if (verify.error) throw verify.error
  if (verify.status !== 0 || verify.signal) {
    const details = `${verify.stdout || ''}${verify.stderr || ''}`.trim()
    throw new Error(`[data-native] macOS code-signature verification failed for ${path}: ${details || `exit=${verify.status}`}`)
  }
  const display = run('/usr/bin/codesign', ['--display', '--verbose=4', path], {
    encoding: 'utf8'
  })
  if (display.error) throw display.error
  const details = `${display.stdout || ''}${display.stderr || ''}`
  if (display.status !== 0 || display.signal) {
    throw new Error(`[data-native] macOS code-signature metadata is unavailable for ${path}`)
  }
  if (options.requireSecureTimestamp === true && !/^Timestamp=/m.test(details)) {
    throw new Error(`[data-native] macOS code signature is missing a secure timestamp: ${path}`)
  }
  const teamIdentifier = details.match(/^TeamIdentifier=(.+)$/m)?.[1]?.trim() || ''
  const authorities = Array.from(details.matchAll(/^Authority=(.+)$/gm), (match) => match[1].trim())
  const flagsMatch = details.match(/flags=0x([0-9a-fA-F]+)(?:\(([^)]*)\))?/)
  const codeDirectoryFlags = flagsMatch ? Number.parseInt(flagsMatch[1], 16) : 0
  const flagNames = flagsMatch?.[2]
    ? flagsMatch[2].split(',').map((value) => value.trim()).filter(Boolean)
    : []
  if (
    (options.requireDeveloperID === true || expectedTeamIdentifier) &&
    (
      !teamIdentifier ||
      teamIdentifier === 'not set' ||
      !authorities.some((authority) => authority.startsWith('Developer ID Application:'))
    )
  ) {
    throw new Error(`[data-native] macOS code signature is not a Developer ID Application identity: ${path}`)
  }
  if (expectedTeamIdentifier && teamIdentifier !== expectedTeamIdentifier) {
    throw new Error(`[data-native] macOS code signature TeamIdentifier mismatch for ${path}`)
  }
  if (
    options.requireHardenedRuntime === true &&
    (codeDirectoryFlags & 0x10000) === 0 &&
    !flagNames.includes('runtime')
  ) {
    throw new Error(`[data-native] macOS code signature is missing Hardened Runtime: ${path}`)
  }

  let entitlements = null
  if (options.inspectEntitlements === true || options.requireEmptyEntitlements === true) {
    const displayedEntitlements = run(
      '/usr/bin/codesign',
      ['--display', '--entitlements', ':-', path],
      { encoding: 'utf8' }
    )
    if (displayedEntitlements.error) throw displayedEntitlements.error
    if (displayedEntitlements.status !== 0 || displayedEntitlements.signal) {
      throw new Error(`[data-native] macOS code-signature entitlements are unavailable for ${path}`)
    }
    const entitlementOutput = String(displayedEntitlements.stdout || '')
    const plistStart = entitlementOutput.indexOf('<?xml')
    if (plistStart < 0) {
      entitlements = {}
    } else {
      const parsed = run(
        '/usr/bin/plutil',
        ['-convert', 'json', '-o', '-', '-'],
        { encoding: 'utf8', input: entitlementOutput.slice(plistStart) }
      )
      if (parsed.error) throw parsed.error
      if (parsed.status !== 0 || parsed.signal) {
        throw new Error(`[data-native] macOS code-signature entitlements are invalid for ${path}`)
      }
      try {
        entitlements = JSON.parse(String(parsed.stdout || ''))
      } catch {
        throw new Error(`[data-native] macOS code-signature entitlements are invalid for ${path}`)
      }
      if (!entitlements || typeof entitlements !== 'object' || Array.isArray(entitlements)) {
        throw new Error(`[data-native] macOS code-signature entitlements are invalid for ${path}`)
      }
    }
    if (options.requireEmptyEntitlements === true && Object.keys(entitlements).length !== 0) {
      throw new Error(`[data-native] macOS strict native code must have an empty entitlement set: ${path}`)
    }
  }
  return {
    status: 'passed',
    secureTimestamp: /^Timestamp=/m.test(details),
    teamIdentifier,
    authorities,
    codeDirectoryFlags,
    flagNames,
    entitlements
  }
}

function inspectNativeBinary(path) {
  const bytes = readBoundedRegularFile(path, MAX_NATIVE_BINARY_BYTES)
  const inspected = inspectMachO(bytes) || inspectPE(bytes) || inspectELF(bytes)
  if (!inspected) throw new Error(`[data-native] Unrecognized native binary format: ${path}`)
  const payload = nativePayloadIdentity(bytes, inspected.format)
  return {
    ...inspected,
    sha256: sha256Bytes(bytes),
    size: bytes.length,
    payloadSha256: payload.sha256,
    payloadSize: payload.size
  }
}

function assertNativeBinaryTarget(path, target) {
  const inspected = inspectNativeBinary(path)
  if (inspected.format !== target.format || inspected.arch !== target.arch) {
    throw new Error(
      `[data-native] Native binary target mismatch for ${path}: expected ${target.format}/${target.arch}, got ${inspected.format}/${inspected.arch}`
    )
  }
  return inspected
}

function exactKeys(value, expected) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value)
  if (actual.length !== expected.length) return false
  const expectedKeys = new Set(expected)
  return actual.every((key) => expectedKeys.has(key))
}

function sameExactRecord(left, right, expected) {
  return exactKeys(left, expected) && exactKeys(right, expected) &&
    expected.every((key) => left[key] === right[key])
}

function validRecordedExecutionProbe(probe, component, recorded, authority) {
  if (!exactKeys(probe, [
    'kind',
    'schema_version',
    'status',
    'component_id',
    'request_nonce',
    'executable_sha256',
    'executable_size',
    'manifest_sha256',
    'policy_sha256',
    'authority_sha256',
    'host_platform',
    'host_arch',
    'loaded_image_bound',
    'working_directory_bound',
    'guardian_authenticated',
    'process_tree_empty'
  ])) return false
  const hostArch = authority.targetKey === 'darwin-arm64' ? 'arm64' : 'x64'
  return probe.kind === 'analytix_native_build_probe_receipt' && probe.schema_version === 1 &&
    probe.status === 'passed' && probe.component_id === component.id &&
    /^[0-9a-f]{64}$/u.test(String(probe.request_nonce || '')) &&
    probe.executable_sha256 === recorded.stagedImageSha256 && probe.executable_size === recorded.stagedImageSize &&
    probe.manifest_sha256 === manifestDigest &&
    probe.policy_sha256 === component.executionProbe.policySha256 &&
    probe.authority_sha256 === authority.binarySha256 &&
    probe.host_platform === 'darwin' && probe.host_arch === hostArch &&
    probe.loaded_image_bound === true && probe.working_directory_bound === true &&
    probe.guardian_authenticated === true && probe.process_tree_empty === true
}

function verifyPackagedComponents(resourceRuntimeDir, target, options = {}) {
  const registeredTarget = target && TARGETS[target.key]
  if (!registeredTarget) throw new Error('[data-native] Packaged target is not registered')
  target = { key: target.key, ...registeredTarget }
  if (target.platform !== 'darwin') {
    throw new Error(`[data-native] Native execution authority is unavailable for target ${target.key}`)
  }
  const repoRoot = options.repoRoot || join(__dirname, '..')
  const repoRelative = relative(resolve(repoRoot), resolve(resourceRuntimeDir))
  const runtimeBoundaryRoot = options.runtimeBoundaryRoot || (
    repoRelative !== '..' && !repoRelative.startsWith(`..${sep}`) && !isAbsolute(repoRelative)
      ? repoRoot
      : dirname(resourceRuntimeDir)
  )
  assertNoSymlinkPath(resourceRuntimeDir, runtimeBoundaryRoot)
  const runtimeStat = lstatSync(resourceRuntimeDir)
  if (runtimeStat.isSymbolicLink() || !runtimeStat.isDirectory()) {
    throw new Error(`[data-native] Packaged runtime must be a regular non-symlink directory: ${resourceRuntimeDir}`)
  }
  const verifyExecution = options.verifyExecution !== false
  const authorityUse = options.authorityUse === undefined ? 'release' : options.authorityUse
  if (authorityUse !== 'release' && authorityUse !== 'local-build') {
    throw new Error('[data-native] Native execution authority use is invalid')
  }
  const receiptPath = join(resourceRuntimeDir, RECEIPT_FILE_NAME)
  if (!existsSync(receiptPath)) throw new Error(`[data-native] Packaged build receipt is missing: ${receiptPath}`)
  const receiptStat = lstatSync(receiptPath)
  if (receiptStat.isSymbolicLink() || !receiptStat.isFile()) {
    throw new Error(`[data-native] Packaged build receipt must be a regular non-symlink file: ${receiptPath}`)
  }
  let receipt
  try {
    const receiptBytes = readBoundedRegularFile(receiptPath, 256 * 1024)
    receipt = parseStrictJsonObject(receiptBytes, {
      maxBytes: 256 * 1024,
      maxDepth: 16,
      maxTokens: 16_384,
      maxStringBytes: 64 * 1024,
      maxNumberBytes: 64,
      integerOnly: true
    })
    if (`${JSON.stringify(receipt, null, 2)}\n` !== receiptBytes.toString('utf8')) {
      throw new Error('receipt is not canonical duplicate-free JSON')
    }
  } catch (error) {
    throw new Error(`[data-native] Packaged build receipt is invalid: ${error instanceof Error ? error.message : String(error)}`)
  }
  if (!exactKeys(receipt, [
    'schemaVersion',
    'manifestSha256',
    'sourceSetSha256',
    'buildEnvironmentSha256',
    'toolchain',
    'targetKey',
    'targetTriple',
    'platform',
    'arch',
    'executionAuthority',
    'publicationAuthority',
    'components'
  ]) || receipt.schemaVersion !== manifest.receiptSchemaVersion) {
    throw new Error('[data-native] Packaged build receipt schema is invalid')
  }
  for (const [field, expected] of [
    ['manifestSha256', manifestDigest],
    ['targetKey', target.key],
    ['targetTriple', target.triple],
    ['platform', target.platform],
    ['arch', target.arch]
  ]) {
    if (receipt[field] !== expected) {
      throw new Error(`[data-native] Packaged build receipt ${field} mismatch: expected ${expected}, got ${receipt[field]}`)
    }
  }
  const currentSourceSetSha256 = sourceSetDigest(repoRoot)
  if (receipt.sourceSetSha256 !== currentSourceSetSha256) {
    throw new Error('[data-native] Packaged native source-set receipt is stale')
  }
  if (!/^[0-9a-f]{64}$/.test(String(receipt.buildEnvironmentSha256 || ''))) {
    throw new Error('[data-native] Packaged native build-environment receipt is invalid')
  }
  if (
    !exactKeys(receipt.executionAuthority, [
      'schemaVersion',
      'trustClass',
      'protocol',
      'targetKey',
      'binarySha256',
      'binarySize',
      'sourceSetSha256',
      'buildEnvironmentSha256',
      'goToolchainKey',
      'goExecutableSha256'
    ]) || receipt.executionAuthority.schemaVersion !== 1 ||
    receipt.executionAuthority.trustClass !== 'controlled_release' ||
    receipt.executionAuthority.protocol !== 'analytix-native-build-probe-v1' ||
    !['darwin-arm64', 'darwin-x64'].includes(receipt.executionAuthority.targetKey) ||
    receipt.executionAuthority.targetKey !== receipt.targetKey ||
    !Number.isSafeInteger(receipt.executionAuthority.binarySize) ||
    receipt.executionAuthority.binarySize <= 0 ||
    ![
      'binarySha256',
      'sourceSetSha256',
      'buildEnvironmentSha256',
      'goExecutableSha256'
    ].every((field) => /^[0-9a-f]{64}$/u.test(String(receipt.executionAuthority[field] || ''))) ||
    typeof receipt.executionAuthority.goToolchainKey !== 'string' ||
    receipt.executionAuthority.goToolchainKey !== receipt.executionAuthority.targetKey
  ) {
    throw new Error('[data-native] Packaged native execution authority receipt is invalid')
  }
  if (
    authorityUse !== 'release' ||
    !exactKeys(receipt.publicationAuthority, [
      'schemaVersion',
      'trustClass',
      'protocol',
      'targetKey',
      'cargoExecutionId',
      'cargoExecutionReceiptSha256',
      'publicationBindingSha256'
    ]) || receipt.publicationAuthority.schemaVersion !== 1 ||
    receipt.publicationAuthority.trustClass !== 'controlled_release' ||
    receipt.publicationAuthority.protocol !== 'analytix-cargo-execution-authority-v1' ||
    receipt.publicationAuthority.targetKey !== target.key ||
    !['cargoExecutionId', 'cargoExecutionReceiptSha256', 'publicationBindingSha256']
      .every((field) => /^[0-9a-f]{64}$/u.test(String(receipt.publicationAuthority[field] || '')))
  ) {
    throw new Error('[data-native] Packaged native publication authority receipt is invalid')
  }
  if (
    !exactKeys(receipt.toolchain, [
      'cargoExecutableSha256',
      'cargoVersion',
      'rustcExecutableSha256',
      'rustcVersion'
    ]) ||
    typeof receipt.toolchain.cargoVersion !== 'string' ||
    !new RegExp(`^cargo ${FROZEN_RUST_TOOLCHAIN_VERSION.replaceAll('.', '\\.')}\\b`).test(receipt.toolchain.cargoVersion) ||
    typeof receipt.toolchain.rustcVersion !== 'string' ||
    !new RegExp(`^rustc ${FROZEN_RUST_TOOLCHAIN_VERSION.replaceAll('.', '\\.')}\\b`).test(receipt.toolchain.rustcVersion) ||
    !/^[0-9a-f]{64}$/.test(receipt.toolchain.cargoExecutableSha256) ||
    !/^[0-9a-f]{64}$/.test(receipt.toolchain.rustcExecutableSha256)
  ) {
    throw new Error('[data-native] Packaged native toolchain receipt is invalid')
  }
  if (options.verifyBuildEnvironment !== false) {
    const ambientEnvironment = projectDevelopmentCacheEnvironment(
      options.env || process.env,
      ['CARGO_TARGET_DIR']
    )
    assertHermeticCargoEnvironment(repoRoot, ambientEnvironment)
    const buildEnvironment = nativeBuildChildEnvironment(ambientEnvironment)
    const rustcExecutable = resolveNativeToolchainExecutable('rustc', buildEnvironment)
    const expectedBuildEnvironmentSha256 = options.expectedBuildEnvironmentSha256 || environmentDigest({
      ...buildEnvironment,
      RUSTC: rustcExecutable
    })
    if (receipt.buildEnvironmentSha256 !== expectedBuildEnvironmentSha256) {
      throw new Error('[data-native] Packaged native build environment changed after receipt issuance')
    }
    const currentToolchain = nativeToolchainIdentity({
      env: buildEnvironment
    })
    if (!sameExactRecord(receipt.toolchain, currentToolchain, [
      'cargoExecutableSha256',
      'cargoVersion',
      'rustcExecutableSha256',
      'rustcVersion'
    ])) {
      throw new Error('[data-native] Packaged native toolchain changed after receipt issuance')
    }
  }
  if (!Array.isArray(receipt.components) || receipt.components.length !== manifest.components.length) {
    throw new Error('[data-native] Packaged build receipt component inventory is incomplete')
  }
  const receiptKeys = [
    'id',
    'binaryName',
    'packagePath',
    'sourceDigest',
    'cargoLockSha256',
    'buildEnvironmentSha256',
    'rawBuildSha256',
    'rawBuildSize',
    'stagedImageSha256',
    'stagedImageSize',
    'payloadSha256',
    'payloadSize',
    'format',
    'arch',
    'executionProbe'
  ]
  for (let index = 0; index < manifest.components.length; index += 1) {
    const component = manifest.components[index]
    const recorded = receipt.components[index]
    if (!exactKeys(recorded, receiptKeys) || recorded.id !== component.id) {
      throw new Error(`[data-native] Packaged build receipt component ${index} is invalid`)
    }
    const expectedBinary = binaryName(component, target.platform)
    const expectedPackagePath = target.platform === 'win32' ? `${component.packagePath}.exe` : component.packagePath
    if (recorded.binaryName !== expectedBinary || recorded.packagePath !== expectedPackagePath) {
      throw new Error(`[data-native] Packaged build receipt path mismatch for ${component.id}`)
    }
    for (const digestField of ['sourceDigest', 'cargoLockSha256', 'rawBuildSha256', 'stagedImageSha256', 'payloadSha256']) {
      if (!/^[0-9a-f]{64}$/.test(String(recorded[digestField] || ''))) {
        throw new Error(`[data-native] Packaged build receipt ${digestField} is invalid for ${component.id}`)
      }
    }
    if (!/^[0-9a-f]{64}$/.test(String(recorded.buildEnvironmentSha256 || ''))) {
      throw new Error(`[data-native] Packaged build receipt component environment is invalid for ${component.id}`)
    }
    if (
      options.expectedComponentBuildEnvironmentDigests &&
      recorded.buildEnvironmentSha256 !== options.expectedComponentBuildEnvironmentDigests[component.id]
    ) {
      throw new Error(`[data-native] Packaged build receipt component environment mismatch for ${component.id}`)
    }
    if (
      !Number.isSafeInteger(recorded.rawBuildSize) || recorded.rawBuildSize <= 0 ||
      !Number.isSafeInteger(recorded.stagedImageSize) || recorded.stagedImageSize <= 0 ||
      !Number.isSafeInteger(recorded.payloadSize) || recorded.payloadSize <= 0 ||
      recorded.payloadSize > recorded.rawBuildSize || recorded.payloadSize > recorded.stagedImageSize
    ) {
      throw new Error(`[data-native] Packaged build receipt size is invalid for ${component.id}`)
    }
    if (
      !validRecordedExecutionProbe(
        recorded.executionProbe,
        component,
        recorded,
        receipt.executionAuthority
      )
    ) {
      throw new Error(`[data-native] Packaged build receipt execution probe is invalid for ${component.id}`)
    }
    const componentRoot = join(repoRoot, component.sourceRoot)
    const currentSourceDigest = sourceTreeDigest(componentRoot, repoRoot)
    const currentCargoLockSha256 = sha256File(join(componentRoot, 'Cargo.lock'))
    if (recorded.sourceDigest !== currentSourceDigest || recorded.cargoLockSha256 !== currentCargoLockSha256) {
      throw new Error(`[data-native] Packaged native source receipt is stale for ${component.id}`)
    }
    const binaryPath = join(resourceRuntimeDir, expectedBinary)
    assertNoSymlinkPath(binaryPath, runtimeBoundaryRoot)
    const inspected = assertNativeBinaryTarget(binaryPath, target)
    if (
      recorded.payloadSha256 !== inspected.payloadSha256 ||
      recorded.payloadSize !== inspected.payloadSize ||
      recorded.format !== inspected.format ||
      recorded.arch !== inspected.arch
    ) {
      throw new Error(`[data-native] Packaged native payload receipt mismatch for ${component.id}`)
    }
    if (
      (options.requireBuildIdentity === true || target.format === 'elf') &&
      (recorded.stagedImageSha256 !== inspected.sha256 || recorded.stagedImageSize !== inspected.size)
    ) {
      throw new Error(`[data-native] Packaged native build identity mismatch for ${component.id}`)
    }
    if (verifyExecution) {
      const freshProbe = runExecutionProbe(binaryPath, component)
      if (
        freshProbe.executable_sha256 !== inspected.sha256 ||
        freshProbe.executable_size !== inspected.size
      ) {
        throw new Error(`[data-native] Packaged execution authority changed for ${component.id}`)
      }
      const finalIdentity = assertNativeBinaryTarget(binaryPath, target)
      if (
        finalIdentity.sha256 !== inspected.sha256 || finalIdentity.size !== inspected.size ||
        finalIdentity.payloadSha256 !== inspected.payloadSha256 || finalIdentity.payloadSize !== inspected.payloadSize ||
        finalIdentity.format !== inspected.format || finalIdentity.arch !== inspected.arch
      ) {
        throw new Error(`[data-native] Packaged native binary changed around execution probe for ${component.id}`)
      }
    }
    const forbiddenCounterpart = target.platform === 'win32'
      ? join(resourceRuntimeDir, component.binaryName)
      : join(resourceRuntimeDir, `${component.binaryName}.exe`)
    if (existsSync(forbiddenCounterpart)) {
      throw new Error(`[data-native] Packaged runtime contains a wrong-target native binary: ${forbiddenCounterpart}`)
    }
  }
  return receipt
}

function atomicWriteJSON(path, value) {
  const bytes = `${JSON.stringify(value, null, 2)}\n`
  const temporary = `${path}.tmp-${process.pid}`
  const file = openSync(temporary, 'w', 0o600)
  try {
    writeFileSync(file, bytes, 'utf8')
    fsyncSync(file)
  } finally {
    closeSync(file)
  }
  renameSync(temporary, path)
  if (process.platform !== 'win32') {
    const directory = openSync(dirname(path), 'r')
    try {
      fsyncSync(directory)
    } finally {
      closeSync(directory)
    }
  }
}

module.exports = {
  RECEIPT_FILE_NAME,
  RECEIPT_SCHEMA_VERSION,
  FROZEN_RUST_TOOLCHAIN_VERSION,
  NATIVE_BUILD_CONTEXT_PATHS,
  NATIVE_BUILD_ENV_KEYS,
  TARGETS,
  assertNativeBinaryTarget,
  assertHermeticCargoEnvironment,
  atomicWriteJSON,
  binaryName,
  inspectNativeBinary,
  nativePayloadIdentity,
  manifest,
  manifestDigest,
  manifestPath,
  nativeBuildContextDigest,
  nativeBuildChildEnvironment,
  nativeBuildEnvironmentDigest,
  nativeComponentBuildEnvironment,
  nativeComponentBuildEnvironmentDigest,
  nativeBuildProbeAuthorityIdentity,
  environmentDigest,
  componentRustFlags,
  nativeToolchainIdentity,
  normalizeExecutableSearchPath,
  resolveNativeToolchainExecutable,
  resolvePinnedRustToolchain,
  normalizeArch,
  normalizePlatform,
  runExecutionProbe,
  sha256File,
  sourceSetDigest,
  sourceSetDigestFromComponentReceipts,
  sourceTreeDigest,
  stageDirectory,
  targetContract,
  targetKey,
  validateNativeComponentManifest,
  verifyDarwinCodeSignature,
  verifyPackagedComponents
}
