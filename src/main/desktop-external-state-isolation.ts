import { lstatSync, realpathSync, type Stats } from 'node:fs'
import { dirname, isAbsolute, relative, resolve, sep } from 'node:path'

export const ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV =
  'ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE'
export const ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1 =
  'isolated-local-v1'

const INVALID_CONFIGURATION =
  'Analytix desktop external-state isolation configuration is invalid.'
const SAFE_ABSOLUTE_TOKEN_PATTERN = /^\/[A-Za-z0-9._/-]+$/u

export type DesktopExternalStateBoundary = Readonly<{
  isolated: boolean
  isolationRoot: string
  userDataRoot: string
  stateHomeRoot: string
}>

type DesktopExternalStateBoundaryDependencies = Readonly<{
  lstat: (path: string) => Stats
  realpath: (path: string) => string
  ownerUID: () => number | null
}>

const defaultDependencies: DesktopExternalStateBoundaryDependencies = {
  lstat: lstatSync,
  realpath: realpathSync,
  ownerUID: () => typeof process.getuid === 'function' ? process.getuid() : null
}

function invalidConfiguration(): Error {
  return new Error(INVALID_CONFIGURATION)
}

export function isDesktopExternalStateSafeAbsoluteTokenV1(raw: string | undefined): raw is string {
  return Boolean(raw && raw.length <= 1024 && raw === raw.trim() && SAFE_ABSOLUTE_TOKEN_PATTERN.test(raw) &&
    isAbsolute(raw) && resolve(raw) === raw)
}

function exactAbsolutePath(raw: string | undefined): string {
  if (!isDesktopExternalStateSafeAbsoluteTokenV1(raw)) {
    throw invalidConfiguration()
  }
  return raw
}

function ownerOnlyDirectory(
  path: string,
  dependencies: DesktopExternalStateBoundaryDependencies
): boolean {
  try {
    const identity = dependencies.lstat(path)
    const ownerUID = dependencies.ownerUID()
    return identity.isDirectory() && !identity.isSymbolicLink() &&
      ownerUID !== null && identity.uid === ownerUID && (identity.mode & 0o077) === 0
  } catch {
    return false
  }
}

function strictDescendant(parent: string, candidate: string): boolean {
  const path = relative(parent, candidate)
  return Boolean(path) && path !== '..' && !path.startsWith(`..${sep}`) && !isAbsolute(path)
}

export function resolveDesktopExternalStateBoundary(
  env: NodeJS.ProcessEnv = process.env,
  dependencies: DesktopExternalStateBoundaryDependencies = defaultDependencies
): DesktopExternalStateBoundary {
  const requestedMode = env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]
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

  const cacheRoot = exactAbsolutePath(env.ANALYTIX_DEV_CACHE_ROOT)
  const requestedUserDataRoot = exactAbsolutePath(env.ANALYTIX_USER_DATA_DIR)
  const trustedTempRoot = resolve(cacheRoot, 'tmp')
  const taskRoot = dirname(requestedUserDataRoot)
  if (
    resolve(trustedTempRoot) !== trustedTempRoot ||
    !ownerOnlyDirectory(cacheRoot, dependencies) ||
    !ownerOnlyDirectory(trustedTempRoot, dependencies) ||
    !ownerOnlyDirectory(taskRoot, dependencies) ||
    !ownerOnlyDirectory(requestedUserDataRoot, dependencies)
  ) {
    throw invalidConfiguration()
  }

  let physicalTempRoot = ''
  let physicalTaskRoot = ''
  let physicalUserDataRoot = ''
  try {
    physicalTempRoot = dependencies.realpath(trustedTempRoot)
    physicalTaskRoot = dependencies.realpath(taskRoot)
    physicalUserDataRoot = dependencies.realpath(requestedUserDataRoot)
  } catch {
    throw invalidConfiguration()
  }
  if (
    physicalTempRoot !== trustedTempRoot ||
    physicalTaskRoot !== taskRoot ||
    physicalUserDataRoot !== requestedUserDataRoot ||
    !strictDescendant(physicalTempRoot, physicalTaskRoot) ||
    !strictDescendant(physicalTaskRoot, physicalUserDataRoot)
  ) {
    throw invalidConfiguration()
  }

  return Object.freeze({
    isolated: true,
    isolationRoot: physicalTaskRoot,
    userDataRoot: physicalUserDataRoot,
    stateHomeRoot: physicalUserDataRoot
  })
}

export function consumeDesktopExternalStateBoundary(
  env: NodeJS.ProcessEnv = process.env,
  dependencies: DesktopExternalStateBoundaryDependencies = defaultDependencies
): DesktopExternalStateBoundary {
  const boundary = resolveDesktopExternalStateBoundary(env, dependencies)
  delete env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]
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
