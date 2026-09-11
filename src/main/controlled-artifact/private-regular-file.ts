import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  readFileSync,
  type Stats
} from 'node:fs'

export type PrivateRegularFileReadState =
  | { status: 'missing' }
  | { status: 'invalid' }
  | { status: 'valid'; bytes: Buffer }

export function readPrivateRegularFile(
  path: string,
  maxBytes: number
): PrivateRegularFileReadState {
  let descriptor: number | null = null
  try {
    const pathInfo = lstatSync(path)
    if (!validPrivateFileInfo(pathInfo, maxBytes)) return { status: 'invalid' }
    descriptor = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0))
    const before = fstatSync(descriptor)
    if (!validPrivateFileInfo(before, maxBytes) || !sameFileIdentity(pathInfo, before)) {
      return { status: 'invalid' }
    }
    const bytes = readFileSync(descriptor)
    const after = fstatSync(descriptor)
    const finalPathInfo = lstatSync(path)
    if (
      !validPrivateFileInfo(after, maxBytes) ||
      !validPrivateFileInfo(finalPathInfo, maxBytes) ||
      !sameFileIdentity(before, after) ||
      !sameFileIdentity(before, finalPathInfo) ||
      bytes.byteLength !== after.size
    ) return { status: 'invalid' }
    return { status: 'valid', bytes }
  } catch (error) {
    return (error as NodeJS.ErrnoException).code === 'ENOENT'
      ? { status: 'missing' }
      : { status: 'invalid' }
  } finally {
    if (descriptor !== null) closeSync(descriptor)
  }
}

function validPrivateFileInfo(info: Stats, maxBytes: number): boolean {
  return Number.isSafeInteger(maxBytes) &&
    maxBytes > 0 &&
    info.isFile() &&
    !info.isSymbolicLink() &&
    info.nlink === 1 &&
    info.size > 0 &&
    info.size <= maxBytes &&
    (process.platform === 'win32' || (info.mode & 0o777) === 0o600)
}

function sameFileIdentity(left: Stats, right: Stats): boolean {
  return left.dev === right.dev &&
    left.ino === right.ino &&
    left.size === right.size &&
    left.mtimeMs === right.mtimeMs &&
    left.ctimeMs === right.ctimeMs
}
