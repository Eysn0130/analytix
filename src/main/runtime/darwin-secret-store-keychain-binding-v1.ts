import { createHash } from 'node:crypto'
import { lstatSync, realpathSync } from 'node:fs'
import { isAbsolute, join, relative, sep } from 'node:path'
import type { DesktopExternalStateBoundary } from '../desktop-external-state-isolation'
import {
  ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
  isDesktopExternalStateSafeAbsoluteTokenV1
} from '../desktop-external-state-isolation'
import { goCompatibleJSONStringifyV1 } from '../claw-schedule-mcp-config'

export const DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1 =
  'analytix.runtime-darwin-secret-store-keychain-binding/v1'

const KEYCHAIN_DIRECTORY_NAME = 'darwin-secret-store-keychain'
const KEYCHAIN_FILE_NAME = 'analytix-task.keychain-db'
const BINDING_ERROR = 'Darwin Secret Store task Keychain binding is unavailable.'

export type DarwinSecretStoreKeychainBindingV1 = Readonly<{
  schemaVersion: 1
  purpose: typeof DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1
  externalStateMode: typeof ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
  isolationRoot: string
  userDataDir: string
  dataDir: string
  keychainDBPath: string
  keychainSecurityDigest: string
  bindingDigest: string
}>

export type DarwinTaskKeychainPathsV1 = {
  parentDir: string
  requestedPath: string
  databasePath: string
}

function sha256Hex(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/u.test(value)
}

export function validateDarwinSecretStoreKeychainBindingDocumentV1(
  input: DarwinSecretStoreKeychainBindingV1
): DarwinSecretStoreKeychainBindingV1 {
  if (!input || typeof input !== 'object' || Array.isArray(input) ||
    Object.getOwnPropertySymbols(input).length !== 0 ||
    Object.keys(input).sort().join('\n') !== [
      'bindingDigest', 'dataDir', 'externalStateMode', 'isolationRoot', 'keychainDBPath',
      'keychainSecurityDigest', 'purpose', 'schemaVersion', 'userDataDir'
    ].sort().join('\n') ||
    input.schemaVersion !== 1 ||
    input.purpose !== DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1 ||
    input.externalStateMode !== ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1 ||
    !exactAbsolutePath(input.isolationRoot) || !exactAbsolutePath(input.userDataDir) ||
    !exactAbsolutePath(input.dataDir) || !exactAbsolutePath(input.keychainDBPath) ||
    input.userDataDir === input.dataDir ||
    !strictDescendant(input.isolationRoot, input.userDataDir) ||
    !strictDescendant(input.isolationRoot, input.dataDir) ||
    input.keychainDBPath !== join(input.isolationRoot, KEYCHAIN_DIRECTORY_NAME, KEYCHAIN_FILE_NAME) ||
    !sha256Hex(input.keychainSecurityDigest) || !sha256Hex(input.bindingDigest)) {
    throw bindingError()
  }
  const document: Omit<DarwinSecretStoreKeychainBindingV1, 'bindingDigest'> = {
    schemaVersion: 1 as const,
    purpose: DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1,
    externalStateMode: ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
    isolationRoot: input.isolationRoot,
    userDataDir: input.userDataDir,
    dataDir: input.dataDir,
    keychainDBPath: input.keychainDBPath,
    keychainSecurityDigest: input.keychainSecurityDigest
  }
  if (createHash('sha256').update(goCompatibleJSONStringifyV1(document)).digest('hex') !== input.bindingDigest) {
    throw bindingError()
  }
  return Object.freeze({ ...document, bindingDigest: input.bindingDigest })
}

function bindingError(): Error {
  return new Error(BINDING_ERROR)
}

function exactAbsolutePath(value: string): boolean {
  return isDesktopExternalStateSafeAbsoluteTokenV1(value) && !/login\.keychain/iu.test(value)
}

function strictDescendant(parent: string, candidate: string): boolean {
  const path = relative(parent, candidate)
  return Boolean(path) && path !== '..' && !path.startsWith(`..${sep}`) && !isAbsolute(path)
}

function ownerOnlyDirectory(path: string, ownerUID: bigint): boolean {
  try {
    const identity = lstatSync(path, { bigint: true })
    return identity.isDirectory() && !identity.isSymbolicLink() &&
      identity.uid === ownerUID && (identity.mode & 0o077n) === 0n && realpathSync(path) === path
  } catch {
    return false
  }
}

function keychainSecurityDigest(path: string, ownerUID: bigint): string {
  try {
    const identity = lstatSync(path, { bigint: true })
    if (!identity.isFile() || identity.isSymbolicLink() || identity.nlink !== 1n ||
      identity.uid !== ownerUID || (identity.mode & 0o777n) !== 0o600n ||
      identity.size <= 0n || realpathSync(path) !== path) {
      throw bindingError()
    }
    const document = {
      schemaVersion: 1,
      pathDigest: createHash('sha256').update(path).digest('hex'),
      device: identity.dev.toString(10),
      inode: identity.ino.toString(10),
      owner: identity.uid.toString(10),
      mode: (identity.mode & 0o777n).toString(8),
      links: identity.nlink.toString(10)
    }
    return createHash('sha256').update(JSON.stringify(document)).digest('hex')
  } catch {
    throw bindingError()
  }
}

export function darwinTaskKeychainPathsV1(
  boundary: DesktopExternalStateBoundary
): DarwinTaskKeychainPathsV1 {
  if (!boundary.isolated || !exactAbsolutePath(boundary.isolationRoot) ||
    !exactAbsolutePath(boundary.userDataRoot) ||
    !strictDescendant(boundary.isolationRoot, boundary.userDataRoot)) {
    throw bindingError()
  }
  const parentDir = join(boundary.isolationRoot, KEYCHAIN_DIRECTORY_NAME)
  const requestedPath = join(parentDir, KEYCHAIN_FILE_NAME)
  return {
    parentDir,
    requestedPath,
    databasePath: requestedPath
  }
}

export function resolveDarwinSecretStoreKeychainBindingV1(input: Readonly<{
  boundary: DesktopExternalStateBoundary
  dataDir: string
  platform?: NodeJS.Platform
  ownerUID?: number
}>): DarwinSecretStoreKeychainBindingV1 | null {
  const platform = input.platform ?? process.platform
  if (!input.boundary.isolated || platform !== 'darwin') return null
  const { isolationRoot, userDataRoot } = input.boundary
  const dataDir = input.dataDir
  const ownerUID = input.ownerUID ?? (typeof process.getuid === 'function' ? process.getuid() : -1)
  if (!Number.isSafeInteger(ownerUID) || ownerUID < 0) throw bindingError()
  const ownerUIDBigInt = BigInt(ownerUID)
  const keychain = darwinTaskKeychainPathsV1(input.boundary)
  if (!exactAbsolutePath(dataDir) || dataDir === userDataRoot ||
    !strictDescendant(isolationRoot, dataDir) ||
    !ownerOnlyDirectory(isolationRoot, ownerUIDBigInt) ||
    !ownerOnlyDirectory(userDataRoot, ownerUIDBigInt) ||
    !ownerOnlyDirectory(dataDir, ownerUIDBigInt) ||
    !ownerOnlyDirectory(keychain.parentDir, ownerUIDBigInt) ||
    !strictDescendant(isolationRoot, keychain.databasePath)) {
    throw bindingError()
  }
  const securityDigest = keychainSecurityDigest(keychain.databasePath, ownerUIDBigInt)
  const document: Omit<DarwinSecretStoreKeychainBindingV1, 'bindingDigest'> = {
    schemaVersion: 1 as const,
    purpose: DARWIN_SECRET_STORE_KEYCHAIN_BINDING_PURPOSE_V1,
    externalStateMode: ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
    isolationRoot,
    userDataDir: userDataRoot,
    dataDir,
    keychainDBPath: keychain.databasePath,
    keychainSecurityDigest: securityDigest
  }
  return validateDarwinSecretStoreKeychainBindingDocumentV1(Object.freeze({
    ...document,
    bindingDigest: createHash('sha256').update(goCompatibleJSONStringifyV1(document)).digest('hex')
  }))
}
