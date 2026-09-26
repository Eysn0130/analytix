import { spawnSync } from 'node:child_process'
import { lstatSync, realpathSync, type Stats } from 'node:fs'
import { isAbsolute, posix, resolve, win32 } from 'node:path'
import windowsDevelopmentProfileScript from '../../scripts/windows-development-profile.ps1?raw'

export const ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV =
  'ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE'
export const ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1 =
  'isolated-local-v1'

const INVALID_CONFIGURATION =
  'Analytix desktop external-state isolation configuration is invalid.'
const SAFE_ABSOLUTE_TOKEN_PATTERN = /^\/[A-Za-z0-9._/-]+$/u

export type DesktopExternalStateBoundary = Readonly<{
  developmentProviderAuthorityDir?: string
  isolated: boolean
  isolationRoot: string
  userDataRoot: string
  stateHomeRoot: string
}>

type DesktopExternalStateBoundaryDependencies = Readonly<{
  lstat: (path: string) => Stats
  realpath: (path: string) => string
  ownerUID: () => number | null
  windowsPrivateDirectory?: (path: string) => boolean
}>

function validateWindowsPrivateDirectory(path: string): boolean {
  const systemRoot = process.env.SystemRoot || process.env.SYSTEMROOT
  if (!systemRoot || !/^[A-Za-z]:\\[^:*?"<>|]+$/u.test(systemRoot) || win32.normalize(systemRoot) !== systemRoot) {
    return false
  }
  const powershell = win32.join(systemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe')
  const command = `& {\n${windowsDevelopmentProfileScript}\n} -Directory $env:ANALYTIX_PRIVATE_DIRECTORY`
  const result = spawnSync(powershell, [
    '-NoProfile', '-NonInteractive', '-EncodedCommand', Buffer.from(command, 'utf16le').toString('base64')
  ], {
    env: { SystemRoot: systemRoot, ANALYTIX_PRIVATE_DIRECTORY: path },
    stdio: 'ignore', windowsHide: true, timeout: 10_000
  })
  return !result.error && !result.signal && result.status === 0
}

const defaultDependencies: DesktopExternalStateBoundaryDependencies = {
  lstat: lstatSync,
  realpath: realpathSync,
  ownerUID: () => typeof process.getuid === 'function' ? process.getuid() : null,
  windowsPrivateDirectory: validateWindowsPrivateDirectory
}

function invalidConfiguration(): Error {
  return new Error(INVALID_CONFIGURATION)
}

export function isDesktopExternalStateSafeAbsoluteTokenV1(raw: string | undefined): raw is string {
  return Boolean(raw && raw.length <= 1024 && raw === raw.trim() && SAFE_ABSOLUTE_TOKEN_PATTERN.test(raw) &&
    isAbsolute(raw) && resolve(raw) === raw)
}

function exactAbsolutePath(raw: string | undefined, platform: NodeJS.Platform): string {
  const windowsValid = platform === 'win32' && typeof raw === 'string' && raw.length <= 1024 &&
    raw === raw.trim() && /^[A-Za-z]:\\/u.test(raw) && !raw.slice(2).includes(':') &&
    !/[<>|"*?]/u.test(raw) && win32.resolve(raw) === raw && win32.normalize(raw) === raw
  if (!windowsValid && !(platform !== 'win32' && isDesktopExternalStateSafeAbsoluteTokenV1(raw))) {
    throw invalidConfiguration()
  }
  return raw as string
}

function ownerOnlyDirectory(
  path: string,
  dependencies: DesktopExternalStateBoundaryDependencies,
  platform: NodeJS.Platform
): boolean {
  try {
    if (platform === 'win32') return dependencies.windowsPrivateDirectory?.(path) === true
    const identity = dependencies.lstat(path)
    const ownerUID = dependencies.ownerUID()
    return identity.isDirectory() && !identity.isSymbolicLink() &&
      ownerUID !== null && identity.uid === ownerUID && (identity.mode & 0o077) === 0
  } catch {
    return false
  }
}

function strictDescendant(parent: string, candidate: string, platform: NodeJS.Platform): boolean {
  const paths = platform === 'win32' ? win32 : posix
  const path = paths.relative(parent, candidate)
  return Boolean(path) && path !== '..' && !path.startsWith(`..${paths.sep}`) && !paths.isAbsolute(path)
}

export function resolveDesktopExternalStateBoundary(
  env: NodeJS.ProcessEnv = process.env,
  dependencies: DesktopExternalStateBoundaryDependencies = defaultDependencies,
  appIsPackaged = false,
  platform: NodeJS.Platform = process.platform
): DesktopExternalStateBoundary {
  const developmentRoot = env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR
  const requestedMode = env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]
  if (developmentRoot && (appIsPackaged || !requestedMode)) throw invalidConfiguration()
  if (requestedMode === undefined || requestedMode === '') {
    return Object.freeze({
      isolated: false,
      isolationRoot: '',
      userDataRoot: '',
      stateHomeRoot: ''
    })
  }
  if (requestedMode !== ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1) {
    throw invalidConfiguration()
  }

  const paths = platform === 'win32' ? win32 : posix
  const cacheRoot = exactAbsolutePath(env.ANALYTIX_DEV_CACHE_ROOT, platform)
  const requestedUserDataRoot = exactAbsolutePath(env.ANALYTIX_USER_DATA_DIR, platform)
  const trustedTempRoot = paths.resolve(cacheRoot, 'tmp')
  const trustedStateRoot = env.ANALYTIX_DEV_STATE_ROOT === undefined
    ? trustedTempRoot
    : exactAbsolutePath(env.ANALYTIX_DEV_STATE_ROOT, platform)
  const taskRoot = paths.dirname(requestedUserDataRoot)
  if (
    paths.resolve(trustedTempRoot) !== trustedTempRoot ||
    !ownerOnlyDirectory(cacheRoot, dependencies, platform) ||
    !ownerOnlyDirectory(trustedTempRoot, dependencies, platform) ||
    !ownerOnlyDirectory(trustedStateRoot, dependencies, platform) ||
    !ownerOnlyDirectory(taskRoot, dependencies, platform) ||
    !ownerOnlyDirectory(requestedUserDataRoot, dependencies, platform)
  ) {
    throw invalidConfiguration()
  }

  let physicalTempRoot = ''
  let physicalTaskRoot = ''
  let physicalUserDataRoot = ''
  try {
    physicalTempRoot = dependencies.realpath(trustedStateRoot)
    physicalTaskRoot = dependencies.realpath(taskRoot)
    physicalUserDataRoot = dependencies.realpath(requestedUserDataRoot)
  } catch {
    throw invalidConfiguration()
  }
  if (
    physicalTempRoot !== trustedStateRoot ||
    physicalTaskRoot !== taskRoot ||
    physicalUserDataRoot !== requestedUserDataRoot ||
    !strictDescendant(physicalTempRoot, physicalTaskRoot, platform) ||
    !strictDescendant(physicalTaskRoot, physicalUserDataRoot, platform)
  ) {
    throw invalidConfiguration()
  }

  // The developer launcher uses separate owners: Electron userData and the
  // runtime's default HOME/.analytix/data must not contain one another.
  const stateHomeRoot = env.ANALYTIX_DEV_STATE_ROOT === undefined
    ? physicalUserDataRoot
    : paths.resolve(physicalTaskRoot, 'home')
  if (env.ANALYTIX_DEV_STATE_ROOT !== undefined &&
    (env.HOME !== stateHomeRoot || !ownerOnlyDirectory(stateHomeRoot, dependencies, platform) ||
      dependencies.realpath(stateHomeRoot) !== stateHomeRoot)) throw invalidConfiguration()
  if (developmentRoot && (developmentRoot !== paths.resolve(trustedStateRoot, 'provider-credentials') ||
    !ownerOnlyDirectory(developmentRoot, dependencies, platform) || dependencies.realpath(developmentRoot) !== developmentRoot)) throw invalidConfiguration()
  return Object.freeze({
    ...(developmentRoot ? { developmentProviderAuthorityDir: developmentRoot } : {}),
    isolated: true,
    isolationRoot: physicalTaskRoot,
    userDataRoot: physicalUserDataRoot,
    stateHomeRoot
  })
}

export function consumeDesktopExternalStateBoundary(
  env: NodeJS.ProcessEnv = process.env,
  dependencies: DesktopExternalStateBoundaryDependencies = defaultDependencies,
  appIsPackaged = false
): DesktopExternalStateBoundary {
  const boundary = resolveDesktopExternalStateBoundary(env, dependencies, appIsPackaged)
  delete env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]
  delete env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR
  return boundary
}

export function applyDesktopProtocolRegistrations(
  boundary: DesktopExternalStateBoundary,
  protocols: readonly string[],
  register: (protocol: string) => boolean
): number {
  if (boundary.isolated) return 0
  for (const protocol of protocols) register(protocol)
  return protocols.length
}

export function applyDesktopLoginItemSettings(
  boundary: DesktopExternalStateBoundary,
  input: Readonly<{
    platform: NodeJS.Platform
    openAtLogin: boolean
    startMinimized: boolean
    hiddenStartArg: string
    setLoginItemSettings: (settings: {
      openAtLogin: boolean
      args: string[]
    }) => void
  }>
): boolean {
  if (boundary.isolated || (input.platform !== 'win32' && input.platform !== 'darwin')) {
    return false
  }
  input.setLoginItemSettings({
    openAtLogin: input.openAtLogin,
    args: input.platform === 'win32' && input.openAtLogin && input.startMinimized
      ? [input.hiddenStartArg]
      : []
  })
  return true
}
