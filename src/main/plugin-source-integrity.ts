import { createHash } from 'node:crypto'
import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  readFileSync,
  readdirSync,
  realpathSync,
  type Stats
} from 'node:fs'
import { resolve } from 'node:path'

export interface PluginSourceTreeIdentity {
  rootRealPath: string
  treeSha256: string
  fileCount: number
}

export interface PluginSourceTreeIdentityOptions {
  excludeRelativePaths?: readonly string[]
}

interface PluginSourceTreeRecord {
  path: string
  size: number
  sha256: string
}

const PORTABLE_SOURCE_PATH = /^[\x21-\x7e]+$/u

export function computePluginSourceTreeIdentity(
  rootPath: string,
  options: PluginSourceTreeIdentityOptions = {}
): PluginSourceTreeIdentity {
  const absoluteRootPath = resolve(rootPath)
  const rootRealPath = realpathSync(absoluteRootPath)
  if (absoluteRootPath !== rootRealPath) {
    throw new Error('plugin source root must not contain a symbolic link')
  }
  const rootInfo = lstatSync(rootRealPath)
  if (!rootInfo.isDirectory() || rootInfo.isSymbolicLink()) {
    throw new Error('plugin source root must be a real directory')
  }
  const records: PluginSourceTreeRecord[] = []
  const excludedPaths = new Set(options.excludeRelativePaths ?? [])
  for (const excludedPath of excludedPaths) {
    if (
      !PORTABLE_SOURCE_PATH.test(excludedPath) ||
      excludedPath.includes('\\') ||
      excludedPath.startsWith('/') ||
      excludedPath.split('/').some((segment) => !segment || segment === '.' || segment === '..')
    ) throw new Error('plugin source exclusion path is invalid')
  }
  collectPluginSourceTreeRecords(rootRealPath, '', records, excludedPaths)
  records.sort((left, right) => Buffer.from(left.path).compare(Buffer.from(right.path)))
  const treeSha256 = createHash('sha256').update(JSON.stringify(records), 'utf8').digest('hex')
  return { rootRealPath, treeSha256, fileCount: records.length }
}

export function readStablePluginSourceFile(path: string, maxBytes: number): Buffer {
  const pathInfo = lstatSync(path)
  if (
    !pathInfo.isFile() ||
    pathInfo.isSymbolicLink() ||
    !Number.isSafeInteger(maxBytes) ||
    maxBytes <= 0 ||
    pathInfo.size <= 0 ||
    pathInfo.size > maxBytes
  ) throw new Error('plugin source file is invalid')
  return readStableRegularFile(path, pathInfo)
}

function collectPluginSourceTreeRecords(
  rootRealPath: string,
  relativeDirectory: string,
  records: PluginSourceTreeRecord[],
  excludedPaths: ReadonlySet<string>
): void {
  const directory = relativeDirectory ? `${rootRealPath}/${relativeDirectory}` : rootRealPath
  for (const name of readdirSync(directory)) {
    const relativePath = relativeDirectory ? `${relativeDirectory}/${name}` : name
    if (!PORTABLE_SOURCE_PATH.test(relativePath) || relativePath.includes('\\')) {
      throw new Error(`plugin source path is not portable: ${relativePath}`)
    }
    const absolutePath = `${rootRealPath}/${relativePath}`
    const info = lstatSync(absolutePath)
    if (info.isSymbolicLink()) throw new Error(`plugin source tree contains a symbolic link: ${relativePath}`)
    if (excludedPaths.has(relativePath)) {
      if (!info.isFile()) throw new Error(`excluded plugin source entry is not a regular file: ${relativePath}`)
      continue
    }
    if (info.isDirectory()) {
      collectPluginSourceTreeRecords(rootRealPath, relativePath, records, excludedPaths)
      continue
    }
    if (!info.isFile()) throw new Error(`plugin source tree contains a non-regular file: ${relativePath}`)
    const body = readStableRegularFile(absolutePath, info)
    records.push({
      path: relativePath,
      size: body.byteLength,
      sha256: createHash('sha256').update(body).digest('hex')
    })
  }
}

function readStableRegularFile(path: string, pathInfo: Stats): Buffer {
  let descriptor: number | null = null
  try {
    descriptor = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0))
    const before = fstatSync(descriptor)
    if (!before.isFile() || !sameFileIdentity(pathInfo, before)) {
      throw new Error('plugin source file changed before reading')
    }
    const body = readFileSync(descriptor)
    const after = fstatSync(descriptor)
    const finalPathInfo = lstatSync(path)
    if (
      !after.isFile() ||
      finalPathInfo.isSymbolicLink() ||
      !finalPathInfo.isFile() ||
      !sameFileIdentity(before, after) ||
      !sameFileIdentity(before, finalPathInfo) ||
      body.byteLength !== after.size
    ) throw new Error('plugin source file changed while reading')
    return body
  } finally {
    if (descriptor !== null) closeSync(descriptor)
  }
}

function sameFileIdentity(left: Stats, right: Stats): boolean {
  return left.dev === right.dev && left.ino === right.ino && left.size === right.size
}
