const crypto = require('node:crypto')
const { execFileSync } = require('node:child_process')
const { existsSync, lstatSync, readFileSync, realpathSync } = require('node:fs')
const { userInfo } = require('node:os')
const { dirname, isAbsolute, join, relative, sep } = require('node:path')

const toolchainLockPath = join(__dirname, 'go-runtime-toolchain.json')
const toolchainLock = JSON.parse(readFileSync(toolchainLockPath, 'utf8'))
const MAX_TOOLCHAIN_FILE_BYTES = 512 * 1024 * 1024
const FORBIDDEN_EXACT_ENV = new Set([
  'ANALYTIX_GO_BIN',
  'AR',
  'CC',
  'CXX',
  'GCC',
  'LD',
  'LINK',
  'PKG_CONFIG',
  'GO111MODULE',
  'GOARCH',
  'GOAUTH',
  'GOCACHE',
  'GOCACHEPROG',
  'GOENV',
  'GOEXPERIMENT',
  'GOFLAGS',
  'GOINSECURE',
  'GOMODCACHE',
  'GONOPROXY',
  'GONOSUMDB',
  'GOOS',
  'GOPATH',
  'GOPRIVATE',
  'GOPROXY',
  'GOROOT',
  'GOSUMDB',
  'GOTELEMETRY',
  'GOTOOLDIR',
  'GOTOOLCHAIN',
  'GOTMPDIR',
  'GOVCS',
  'GOWORK',
  '_CL_',
  '_LINK_'
])

function normalizePlatform(value) {
  if (value === 'win32' || value === 'windows') return 'win32'
  if (value === 'darwin' || value === 'linux') return value
  throw new Error(`[go-runtime-build] Unsupported host platform: ${value}`)
}

function normalizeArch(value) {
  if (value === 'amd64') return 'x64'
  if (value === 'aarch64') return 'arm64'
  if (value === 'x64' || value === 'arm64') return value
  throw new Error(`[go-runtime-build] Unsupported host architecture: ${value}`)
}

function hostKey(platform = process.platform, arch = process.arch) {
  return `${normalizePlatform(platform)}-${normalizeArch(arch)}`
}

function assertNoAmbientGoOverrides(environment = process.env) {
  const seen = new Set()
  for (const [rawName, rawValue] of Object.entries(environment || {})) {
    const name = String(rawName).toUpperCase()
    if (seen.has(name)) {
      throw new Error(`[go-runtime-build] Duplicate environment key is forbidden: ${name}`)
    }
    seen.add(name)
    if (!rawValue) continue
    if (
      FORBIDDEN_EXACT_ENV.has(name) ||
      name.startsWith('CGO_') ||
      name.startsWith('DYLD_') ||
      name === 'LD_PRELOAD' ||
      name === 'LD_LIBRARY_PATH'
    ) {
      throw new Error(`[go-runtime-build] Ambient build override is forbidden: ${name}`)
    }
  }
}

function sha256File(path, options = {}) {
  const stat = (options.lstatSync || lstatSync)(path)
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size <= 0 || stat.size > MAX_TOOLCHAIN_FILE_BYTES || (stat.mode & 0o022) !== 0) {
    throw new Error(`[go-runtime-build] Toolchain file is not a trusted regular file: ${path}`)
  }
  return crypto.createHash('sha256').update((options.readFileSync || readFileSync)(path)).digest('hex')
}

function defaultGoCandidates() {
  const username = userInfo().username
  const candidates = [
    '/usr/local/go/bin/go',
    '/opt/homebrew/bin/go',
    '/usr/local/bin/go'
  ]
  if (username) candidates.push(`/Users/${username}/.local/bin/go`)
  return candidates
}

function trustedBaseEnvironment(goExecutable) {
  const home = userInfo().homedir
  return {
    PATH: `${dirname(goExecutable)}:/usr/bin:/bin`,
    HOME: home,
    LANG: 'C',
    LC_ALL: 'C',
    GOENV: 'off',
    GOWORK: 'off',
    GOTOOLCHAIN: 'local'
  }
}

function trustedModuleCachePath(path, options = {}) {
  if (typeof path !== 'string' || !isAbsolute(path)) {
    throw new Error('[go-runtime-build] Authorized Go module cache path is invalid')
  }
  const realpath = options.realpathSync || realpathSync
  const lstat = options.lstatSync || lstatSync
  const original = lstat(path)
  const resolved = realpath(path)
  if (resolved !== path || !original.isDirectory() || original.isSymbolicLink() || (original.mode & 0o022) !== 0) {
    throw new Error('[go-runtime-build] Authorized Go module cache is untrusted')
  }
  const stat = lstat(resolved)
  if (!stat.isDirectory() || stat.isSymbolicLink() || (stat.mode & 0o022) !== 0) {
    throw new Error('[go-runtime-build] Authorized Go module cache is untrusted')
  }
  return resolved
}

function resolvePinnedGoToolchain(options = {}) {
  assertNoAmbientGoOverrides(options.env || process.env)
  const key = hostKey(options.platform || process.platform, options.arch || process.arch)
  const locked = (options.lock || toolchainLock).hosts?.[key]
  if (!locked || locked.goVersion === '' || !/^[0-9a-f]{64}$/.test(locked.goExecutableSha256 || '')) {
    throw new Error(`[go-runtime-build] No pinned Go toolchain for host ${key}`)
  }
  const exists = options.existsSync || existsSync
  const realpath = options.realpathSync || realpathSync
  const exec = options.execFileSync || execFileSync
  let executable = ''
  for (const candidate of options.candidates || defaultGoCandidates()) {
    if (!exists(candidate)) continue
    const resolved = realpath(candidate)
    if (sha256File(resolved, options) === locked.goExecutableSha256) {
      executable = resolved
      break
    }
  }
  if (!executable) {
    throw new Error(`[go-runtime-build] Pinned Go executable is unavailable for host ${key}`)
  }
  const baseEnvironment = trustedBaseEnvironment(executable)
  const authorizedModuleCache = options.authorizedModuleCache
    ? trustedModuleCachePath(options.authorizedModuleCache, options)
    : ''
  if (authorizedModuleCache) baseEnvironment.GOMODCACHE = authorizedModuleCache
  const version = String(exec(executable, ['version'], { encoding: 'utf8', env: baseEnvironment })).trim()
  if (!version.startsWith(`go version ${locked.goVersion} `)) {
    throw new Error(`[go-runtime-build] Go version does not match pinned host ${key}`)
  }
  const environmentOutput = String(exec(executable, ['env', 'GOROOT', 'GOTOOLDIR', 'GOMODCACHE'], {
    encoding: 'utf8', env: baseEnvironment
  })).trimEnd().split(/\r?\n/u)
  if (environmentOutput.length !== 3 || environmentOutput.some((value) => !value)) {
    throw new Error('[go-runtime-build] Go toolchain directories are unavailable')
  }
  const [goRootRaw, toolDirRaw, moduleCacheRaw] = environmentOutput
  const goRoot = realpath(goRootRaw)
  const toolDir = realpath(toolDirRaw)
  const moduleCache = realpath(moduleCacheRaw)
  if (authorizedModuleCache && moduleCache !== authorizedModuleCache) {
    throw new Error('[go-runtime-build] Authorized Go module cache identity mismatch')
  }
  if (relative(goRoot, toolDir).startsWith(`..${sep}`) || relative(goRoot, toolDir) === '..') {
    throw new Error('[go-runtime-build] Go tool directory escapes the pinned GOROOT')
  }
  for (const [name, expected] of Object.entries(locked.tools || {})) {
    const path = realpath(join(toolDir, process.platform === 'win32' ? `${name}.exe` : name))
    if (!/^[0-9a-f]{64}$/.test(expected) || sha256File(path, options) !== expected) {
      throw new Error(`[go-runtime-build] Pinned Go ${name} identity mismatch`)
    }
  }
  return { key, executable, goRoot, toolDir, moduleCache, version, lock: locked, baseEnvironment }
}

function hermeticGoBuildEnvironment(toolchain, target, directories) {
  if (!toolchain?.executable || !target?.goos || !target?.goarch || !directories?.cache || !directories?.path || !directories?.temp) {
    throw new Error('[go-runtime-build] Hermetic build environment input is incomplete')
  }
  return {
    ...trustedBaseEnvironment(toolchain.executable),
    HOME: directories.path,
    GOOS: target.goos,
    GOARCH: target.goarch,
    CGO_ENABLED: '0',
    GOCACHE: directories.cache,
    GOMODCACHE: toolchain.moduleCache,
    GOPATH: directories.path,
    GOTMPDIR: directories.temp,
    GOPROXY: 'off',
    GOSUMDB: 'sum.golang.org',
    GONOSUMDB: '',
    GOPRIVATE: '',
    GONOPROXY: '',
    GOINSECURE: '',
    GOVCS: 'public:git|hg,private:off',
    TZ: 'UTC'
  }
}

module.exports = {
  FORBIDDEN_EXACT_ENV,
  assertNoAmbientGoOverrides,
  defaultGoCandidates,
  hermeticGoBuildEnvironment,
  hostKey,
  resolvePinnedGoToolchain,
  sha256File,
  toolchainLock,
  toolchainLockPath,
  trustedBaseEnvironment,
  trustedModuleCachePath
}
