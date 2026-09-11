import { lstatSync } from 'node:fs'
import { isAbsolute, join, relative, resolve, sep, win32 } from 'node:path'

export const DATA_NATIVE_ENV_KEYS = [
  'ANALYTIX_IMPORT_ACCELERATOR_BIN',
  'ANALYTIX_CLEANING_OPS_BIN',
  'ANALYTIX_ANALYSIS_COMPUTE_BIN',
  'ANALYTIX_DATA_ENGINE_BIN'
] as const

export const PACKAGED_DATA_ANALYSIS_ENV_KEYS = [
  ...DATA_NATIVE_ENV_KEYS,
  'ANALYTIX_ARCHIVE_EXTRACTOR_BIN',
  'ANALYTIX_DOCUMENT_ANTIWORD_BIN',
  'ANALYTIX_DOCUMENT_SOFFICE_BIN',
  'ANALYTIX_DOCUMENT_TESSERACT_BIN',
  'ANALYTIX_DOCUMENT_TEXTUTIL_BIN',
  'ANALYTIX_DATA_ANALYSIS_BACKEND_DIR',
  'ANALYTIX_DATA_ANALYSIS_PYTHON',
  'ANALYTIX_DISABLE_DIRECT_DUCKDB_HELPERS',
  'ANALYTIX_DISABLE_DIRECT_PYTHON_DUCKDB',
  'CONDA_PREFIX',
  'PYTHON',
  'PYTHONHOME',
  'PYTHONPATH',
  'PYTHONUSERBASE',
  'TESSDATA_PREFIX',
  'VIRTUAL_ENV'
] as const

export const DATA_ANALYSIS_SUBPROCESS_BASE_ENV_KEYS = [
  'TEMP',
  'TMP',
  'TMPDIR',
  'LANG',
  'LANGUAGE',
  'LC_ADDRESS',
  'LC_ALL',
  'LC_COLLATE',
  'LC_CTYPE',
  'LC_IDENTIFICATION',
  'LC_MEASUREMENT',
  'LC_MESSAGES',
  'LC_MONETARY',
  'LC_NAME',
  'LC_NUMERIC',
  'LC_PAPER',
  'LC_TELEPHONE',
  'LC_TIME',
  'TZ'
] as const

const WINDOWS_SUBPROCESS_BASE_ENV_KEYS = ['SYSTEMROOT'] as const

const DARWIN_SUBPROCESS_BASE_ENV_KEYS = ['__CF_USER_TEXT_ENCODING'] as const

const FORBIDDEN_SUBPROCESS_ENV_NAMES = new Set([
  'CI_JOB_JWT',
  'CI_JOB_JWT_V2',
  'DBUS_SESSION_BUS_ADDRESS',
  'DOCKER_HOST',
  'GPG_AGENT_INFO',
  'KUBECONFIG',
  'NODE_AUTH_TOKEN',
  'SSH_AGENT_PID',
  'SSH_AUTH_SOCK',
  'VAULT_ADDR',
  'WAYLAND_DISPLAY',
  'XDG_RUNTIME_DIR'
])

const FORBIDDEN_SUBPROCESS_ENV_TOKENS = new Set([
  'FILE',
  'KEY',
  'PASSWORD',
  'PROXY',
  'SECRET',
  'SOCK',
  'SOCKET',
  'TOKEN'
])

const SUPPORTED_TARGETS = new Set([
  'darwin-arm64',
  'darwin-x64',
  'linux-x64',
  'win32-x64'
])

export function normalizeNativePlatform(platform: string): string | undefined {
  const normalized = platform.trim().toLowerCase()
  if (normalized === 'win' || normalized === 'windows' || normalized === 'win32') return 'win32'
  if (normalized === 'darwin') return 'darwin'
  if (normalized === 'linux') return 'linux'
  return undefined
}

export function normalizeNativeArch(arch: string): string | undefined {
  const normalized = arch.trim().toLowerCase()
  if (normalized === 'x64' || normalized === 'amd64' || normalized === 'x86_64') return 'x64'
  if (normalized === 'arm64' || normalized === 'aarch64') return 'arm64'
  return undefined
}

export function nativeTargetKey(platform: string, arch: string): string | undefined {
  const normalizedPlatform = normalizeNativePlatform(platform)
  const normalizedArch = normalizeNativeArch(arch)
  if (!normalizedPlatform || !normalizedArch) return undefined
  const key = `${normalizedPlatform}-${normalizedArch}`
  return SUPPORTED_TARGETS.has(key) ? key : undefined
}

export function nativeBinaryFileName(binaryName: string, platform: string): string {
  return normalizeNativePlatform(platform) === 'win32' ? `${binaryName}.exe` : binaryName
}

export function runtimeNativeBinaryPath(options: {
  root: string
  packaged: boolean
  platform: string
  arch: string
  binaryName: string
}): string | undefined {
  void options
  return undefined
}

export function resolveRuntimeNativeBinary(options: {
  root: string
  packaged: boolean
  platform: string
  arch: string
  binaryName: string
}): string | undefined {
  const candidate = runtimeNativeBinaryPath(options)
  return candidate && isRegularNonSymlinkFileWithinRoot(candidate, options.root) ? candidate : undefined
}

export function resolveExplicitNativeBinary(rawPath: string, root: string): string | undefined {
  void rawPath
  void root
  return undefined
}

export function canonicalWindowsSystemExecutablePath(
  rawSystemRoot: string,
  executableName: string
): string | undefined {
  const systemRoot = canonicalWindowsSystemRoot(rawSystemRoot)
  const name = executableName.trim()
  if (!systemRoot || !name || name !== win32.basename(name)) return undefined
  return win32.join(systemRoot, 'System32', name)
}

export function canonicalWindowsSystemRoot(rawSystemRoot: string): string | undefined {
  const systemRoot = rawSystemRoot.trim()
  if (!/^[A-Za-z]:[\\/]/u.test(systemRoot)) return undefined
  const parsed = win32.parse(systemRoot)
  const canonicalRoot = win32.join(parsed.root, 'Windows')
  return win32.resolve(systemRoot).toLowerCase() === win32.resolve(canonicalRoot).toLowerCase()
    ? canonicalRoot
    : undefined
}

export function resolveWindowsSystemExecutable(
  env: NodeJS.ProcessEnv,
  executableName: string
): string | undefined {
  const roots = Object.entries(env)
    .filter(([key, value]) => key.trim().toUpperCase() === 'SYSTEMROOT' && value !== undefined)
    .map(([, value]) => String(value))
  if (roots.length !== 1) return undefined
  const candidate = canonicalWindowsSystemExecutablePath(roots[0], executableName)
  if (!candidate) return undefined
  const systemRoot = win32.dirname(win32.dirname(candidate))
  return isRegularNonSymlinkFileWithinRoot(candidate, systemRoot) ? candidate : undefined
}

export function resolvePackagedPythonExecutable(options: {
  root: string
  packaged: boolean
  platform: string
}): string | undefined {
  if (!options.packaged || normalizeNativePlatform(options.platform) !== 'win32') return undefined
  for (const candidate of [
    join(options.root, '.python-runtime', 'current', 'python', 'python.exe'),
    join(options.root, '.python-runtime', 'current', 'python.exe')
  ]) {
    if (isRegularNonSymlinkFileWithinRoot(candidate, options.root)) return candidate
  }
  return undefined
}

export function resolvePackagedPythonSitePackages(options: {
  root: string
  packaged: boolean
}): string | undefined {
  if (!options.packaged) return undefined
  const candidate = join(options.root, 'python-site-packages')
  return isRegularNonSymlinkDirectoryWithinRoot(candidate, options.root) ? candidate : undefined
}

export function sanitizePackagedNativeEnvironment(
  env: NodeJS.ProcessEnv,
  _packaged: boolean,
  platform: NodeJS.Platform = process.platform
): NodeJS.ProcessEnv {
  const normalizedPlatform = normalizeNativePlatform(platform)
  const allowed = new Set<string>(DATA_ANALYSIS_SUBPROCESS_BASE_ENV_KEYS)
  if (normalizedPlatform === 'win32') {
    for (const key of WINDOWS_SUBPROCESS_BASE_ENV_KEYS) allowed.add(key)
  } else if (normalizedPlatform === 'darwin') {
    for (const key of DARWIN_SUBPROCESS_BASE_ENV_KEYS) allowed.add(key)
  }

  const result: NodeJS.ProcessEnv = {}
  const seen = new Set<string>()
  for (const [key, value] of Object.entries(env)) {
    const canonicalKey = key.trim().toUpperCase()
    if (!canonicalKey) continue
    if (seen.has(canonicalKey)) {
      throw new Error(`Duplicate case-insensitive subprocess environment key is forbidden: ${canonicalKey}`)
    }
    seen.add(canonicalKey)
    if (
      allowed.has(canonicalKey) &&
      !isForbiddenSubprocessEnvironmentName(canonicalKey) &&
      value !== undefined
    ) {
      if (key.includes('\0') || value.includes('\0')) {
        throw new Error(`Subprocess environment contains NUL: ${canonicalKey}`)
      }
      result[key] = value
    }
  }
  if (normalizedPlatform === 'win32') {
    const systemRootEntry = Object.entries(result).find(([key]) => key.trim().toUpperCase() === 'SYSTEMROOT')
    if (systemRootEntry) {
      const canonicalRoot = canonicalWindowsSystemRoot(String(systemRootEntry[1]))
      if (!canonicalRoot) throw new Error('Subprocess Windows SystemRoot is not canonical')
      delete result[systemRootEntry[0]]
      result.SystemRoot = canonicalRoot
    }
  }
  return result
}

function isForbiddenSubprocessEnvironmentName(name: string): boolean {
  if (FORBIDDEN_SUBPROCESS_ENV_NAMES.has(name)) return true
  if (name.startsWith('AWS') || name.startsWith('GITHUB') || name.startsWith('NPM')) return true
  return name
    .split(/[^A-Z0-9]+/u)
    .some((token) => token.length > 0 && FORBIDDEN_SUBPROCESS_ENV_TOKENS.has(token))
}

function isRegularNonSymlinkFile(path: string): boolean {
  try {
    const stat = lstatSync(path)
    return stat.isFile() && !stat.isSymbolicLink()
  } catch {
    return false
  }
}

function isRegularNonSymlinkFileWithinRoot(path: string, root: string): boolean {
  return isPathWithinRootWithoutSymlink(path, root) && isRegularNonSymlinkFile(path)
}

function isRegularNonSymlinkDirectory(path: string): boolean {
  try {
    const stat = lstatSync(path)
    return stat.isDirectory() && !stat.isSymbolicLink()
  } catch {
    return false
  }
}

function isRegularNonSymlinkDirectoryWithinRoot(path: string, root: string): boolean {
  return isPathWithinRootWithoutSymlink(path, root) && isRegularNonSymlinkDirectory(path)
}

function isPathWithinRootWithoutSymlink(path: string, root: string): boolean {
  try {
    const resolvedRoot = resolve(root)
    const resolvedPath = resolve(path)
    const relativePath = relative(resolvedRoot, resolvedPath)
    if (relativePath === '..' || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) return false
    const rootStat = lstatSync(resolvedRoot)
    if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) return false
    let current = resolvedRoot
    for (const part of relativePath ? relativePath.split(sep) : []) {
      current = join(current, part)
      const stat = lstatSync(current)
      if (stat.isSymbolicLink()) return false
    }
    return true
  } catch {
    return false
  }
}
