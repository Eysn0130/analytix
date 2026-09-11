import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  readSync,
  readdirSync,
  realpathSync
} from 'node:fs'
import type { Stats } from 'node:fs'
import { isAbsolute, join, normalize, resolve } from 'node:path'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'
import {
  configureMainOwnedRuntimeAuthoritySourceV1,
  validateMainOwnedRuntimeAuthorityEnvelopeV1,
  type MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'

export const MAIN_OWNED_AUTHORITY_BOOTSTRAP_DIRECTORY_V1 =
  'runtime-authority-bootstrap-v1'
export const MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1 =
  'authority-bootstrap-v1.json'

const MAX_BOOTSTRAP_BYTES_V1 = 32 << 10
const BOOTSTRAP_ERROR = 'Managed runtime authority bootstrap is unavailable.'

type FileIdentityV1 = Readonly<{
  dev: string
  ino: string
  size: number
  uid: number
  gid: number
  mode: number
  mtimeMs: number
  ctimeMs: number
}>

/**
 * Installs the one production source used by packaged Electron. Its fixed
 * leaf lives beneath the Electron-main-owned userData root. No renderer input,
 * setting, runtime argv, or authority-named environment variable supplies the
 * document path or any authority field. The file contains only a public
 * anchor and protected root locators; Go still reopens those roots through
 * secureconfigfs and cryptographically binds them through authorityanchor
 * before activation. The ready witness proves exact transport identity, not
 * an independent installation trust root.
 */
export function configurePackagedMainOwnedRuntimeAuthorityBootstrapV1(input: {
  appIsPackaged: boolean
  userDataDir: string
}): void {
  if (!input?.appIsPackaged) {
    configureMainOwnedRuntimeAuthoritySourceV1(null)
    return
  }
  const root = resolveMainOwnedRuntimeAuthorityBootstrapRootV1(input.userDataDir)
  configureMainOwnedRuntimeAuthoritySourceV1({
    take: async () => readMainOwnedRuntimeAuthorityBootstrapV1(root)
  })
}

export function resolveMainOwnedRuntimeAuthorityBootstrapRootV1(
  userDataDir: string
): string {
  if (typeof userDataDir !== 'string' || userDataDir === '' ||
    userDataDir !== userDataDir.trim() || !isAbsolute(userDataDir) ||
    normalize(userDataDir) !== userDataDir || resolve(userDataDir) !== userDataDir ||
    userDataDir.includes('\0')) {
    throw fixedBootstrapErrorV1()
  }
  return join(userDataDir, MAIN_OWNED_AUTHORITY_BOOTSTRAP_DIRECTORY_V1)
}

export function readMainOwnedRuntimeAuthorityBootstrapV1(
  root: string
): MainOwnedRuntimeAuthorityEnvelopeV1 | null {
  if (!supportedBootstrapHostV1()) throw fixedBootstrapErrorV1()
  let rootState: Stats
  try {
    rootState = lstatSync(root)
  } catch (error) {
    if (isErrnoV1(error, 'ENOENT')) return null
    throw fixedBootstrapErrorV1()
  }
  let body: Buffer | null = null
  try {
    const expectedRoot = validatePrivateBootstrapRootV1(root, rootState)
    assertExactBootstrapInventoryV1(root)
    const first = readExactBootstrapFileV1(root, expectedRoot)
    const second = readExactBootstrapFileV1(root, expectedRoot)
    body = first.body
    try {
      if (!sameFileIdentityV1(first.identity, second.identity) ||
        !first.body.equals(second.body)) {
        throw fixedBootstrapErrorV1()
      }
    } finally {
      second.body.fill(0)
    }
    const currentRoot = validatePrivateBootstrapRootV1(root, lstatSync(root))
    assertSameRootIdentityV1(expectedRoot, currentRoot)
    assertExactBootstrapInventoryV1(root)
    const parsed = parseStrictJsonObject(body, {
      maxBytes: MAX_BOOTSTRAP_BYTES_V1,
      maxDepth: 3,
      maxTokens: 24,
      maxStringBytes: 8 << 10,
      maxNumberBytes: 8
    })
    const validated = validateMainOwnedRuntimeAuthorityEnvelopeV1(
      parsed as MainOwnedRuntimeAuthorityEnvelopeV1
    )
    if (body.toString('utf8') !== JSON.stringify(validated)) {
      throw fixedBootstrapErrorV1()
    }
    return validated
  } catch {
    throw fixedBootstrapErrorV1()
  } finally {
    body?.fill(0)
  }
}

function validatePrivateBootstrapRootV1(root: string, state: Stats): FileIdentityV1 {
  const euid = process.geteuid?.()
  if (!isAbsolute(root) || normalize(root) !== root || resolve(root) !== root ||
    realpathSync.native(root) !== root || !state.isDirectory() ||
    state.isSymbolicLink() || euid === undefined || state.uid !== euid ||
    (state.mode & 0o777) !== 0o700 || (state.mode & 0o7000) !== 0) {
    throw fixedBootstrapErrorV1()
  }
  const descriptor = openSync(
    root,
    constants.O_RDONLY | constants.O_DIRECTORY | (constants.O_NOFOLLOW ?? 0)
  )
  try {
    const opened = fstatSync(descriptor)
    const expected = fileIdentityV1(state)
    const actual = fileIdentityV1(opened)
    assertSameRootIdentityV1(expected, actual)
    return expected
  } finally {
    closeSync(descriptor)
  }
}

function assertExactBootstrapInventoryV1(root: string): void {
  const names = readdirSync(root).sort()
  if (names.length !== 1 || names[0] !== MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1) {
    throw fixedBootstrapErrorV1()
  }
}

function readExactBootstrapFileV1(
  root: string,
  rootIdentity: FileIdentityV1
): Readonly<{ body: Buffer; identity: FileIdentityV1 }> {
  const path = join(root, MAIN_OWNED_AUTHORITY_BOOTSTRAP_FILE_V1)
  const beforeRoot = validatePrivateBootstrapRootV1(root, lstatSync(root))
  assertSameRootIdentityV1(rootIdentity, beforeRoot)
  const before = lstatSync(path)
  const euid = process.geteuid?.()
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 ||
    euid === undefined || before.uid !== euid ||
    (before.mode & 0o077) !== 0 || (before.mode & 0o7000) !== 0 ||
    (before.mode & 0o400) === 0 || before.size <= 0 ||
    before.size > MAX_BOOTSTRAP_BYTES_V1) {
    throw fixedBootstrapErrorV1()
  }
  const descriptor = openSync(
    path,
    constants.O_RDONLY | (constants.O_NONBLOCK ?? 0) | (constants.O_NOFOLLOW ?? 0)
  )
  let body: Buffer | null = null
  try {
    const openedBefore = fstatSync(descriptor)
    const expected = fileIdentityV1(before)
    if (!sameFileIdentityV1(expected, fileIdentityV1(openedBefore))) {
      throw fixedBootstrapErrorV1()
    }
    body = Buffer.allocUnsafe(before.size)
    let offset = 0
    while (offset < body.length) {
      const count = readSync(descriptor, body, offset, body.length - offset, null)
      if (count <= 0) throw fixedBootstrapErrorV1()
      offset += count
    }
    const overflow = Buffer.alloc(1)
    try {
      if (readSync(descriptor, overflow, 0, 1, null) !== 0) {
        throw fixedBootstrapErrorV1()
      }
    } finally {
      overflow.fill(0)
    }
    const openedAfter = fstatSync(descriptor)
    const after = lstatSync(path)
    if (!sameFileIdentityV1(expected, fileIdentityV1(openedAfter)) ||
      !sameFileIdentityV1(expected, fileIdentityV1(after))) {
      throw fixedBootstrapErrorV1()
    }
    const result = body
    body = null
    return Object.freeze({ body: result, identity: expected })
  } finally {
    body?.fill(0)
    closeSync(descriptor)
  }
}

function fileIdentityV1(state: Stats): FileIdentityV1 {
  return Object.freeze({
    dev: String(state.dev),
    ino: String(state.ino),
    size: state.size,
    uid: state.uid,
    gid: state.gid,
    mode: state.mode,
    mtimeMs: state.mtimeMs,
    ctimeMs: state.ctimeMs
  })
}

function sameFileIdentityV1(left: FileIdentityV1, right: FileIdentityV1): boolean {
  return left.dev === right.dev && left.ino === right.ino &&
    left.size === right.size && left.uid === right.uid && left.gid === right.gid &&
    left.mode === right.mode && left.mtimeMs === right.mtimeMs &&
    left.ctimeMs === right.ctimeMs
}

function assertSameRootIdentityV1(left: FileIdentityV1, right: FileIdentityV1): void {
  if (!sameFileIdentityV1(left, right)) throw fixedBootstrapErrorV1()
}

function supportedBootstrapHostV1(): boolean {
  return (process.platform === 'darwin' || process.platform === 'linux') &&
    typeof process.geteuid === 'function'
}

function isErrnoV1(error: unknown, code: string): boolean {
  return typeof error === 'object' && error !== null && 'code' in error &&
    (error as NodeJS.ErrnoException).code === code
}

function fixedBootstrapErrorV1(): Error {
  return new Error(BOOTSTRAP_ERROR)
}
