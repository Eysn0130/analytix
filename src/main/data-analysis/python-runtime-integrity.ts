import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, readdirSync } from 'node:fs'
import { isAbsolute, join, relative, resolve, sep } from 'node:path'
import pythonRuntimeLockBytes from '../../../scripts/windows-python-runtime-lock.json?raw'
import requirementsLockBytes from '../../../scripts/windows-backend-requirements.lock.txt?raw'
import { inspectNativePayload } from './native-payload-identity'

const RUNTIME_MANIFEST_NAME = 'analytix-python-runtime-manifest.json'
const SITE_PACKAGES_MANIFEST_NAME = 'analytix-site-packages-manifest.json'

type FrozenPythonRuntimeLock = {
  schemaVersion: number
  pythonRuntime: {
    version: string
    series: string
    target: string
    assetName: string
    archiveSha256: string
    sourceTreeSha256: string
    runtimeContentSha256: string
    runtimeFileCount: number
  }
  sitePackages: {
    platform: string
    requirementsLockSha256: string
    wheelCount: number
    wheelSetSha256: string
    contentSha256: string
    fileCount: number
  }
}

type WheelRecord = { fileName: string; size: number; sha256: string }
type TreeIdentity = { sha256: string; fileCount: number }

const lock = JSON.parse(pythonRuntimeLockBytes) as FrozenPythonRuntimeLock
const lockSha256 = sha256(Buffer.from(pythonRuntimeLockBytes, 'utf8'))

function sha256(bytes: Buffer): string {
  return createHash('sha256').update(bytes).digest('hex')
}

function exactKeys(value: unknown, expected: string[]): value is Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const actual = Object.keys(value)
  if (actual.length !== expected.length) return false
  const expectedKeys = new Set(expected)
  return actual.every((key) => expectedKeys.has(key))
}

function isSha256(value: unknown): value is string {
  return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value)
}

function assertFrozenLock(): void {
  if (!exactKeys(lock, ['schemaVersion', 'pythonRuntime', 'sitePackages']) || lock.schemaVersion !== 1) {
    throw new Error('Bundled Python runtime lock schema is invalid')
  }
  if (!exactKeys(lock.pythonRuntime, [
    'version',
    'series',
    'target',
    'assetName',
    'archiveSha256',
    'sourceTreeSha256',
    'runtimeContentSha256',
    'runtimeFileCount'
  ]) || !exactKeys(lock.sitePackages, [
    'platform',
    'requirementsLockSha256',
    'wheelCount',
    'wheelSetSha256',
    'contentSha256',
    'fileCount'
  ])) {
    throw new Error('Bundled Python runtime lock inventory is invalid')
  }
  for (const value of [
    lock.pythonRuntime.archiveSha256,
    lock.pythonRuntime.sourceTreeSha256,
    lock.pythonRuntime.runtimeContentSha256,
    lock.sitePackages.requirementsLockSha256,
    lock.sitePackages.wheelSetSha256,
    lock.sitePackages.contentSha256
  ]) {
    if (!isSha256(value)) throw new Error('Bundled Python runtime lock contains an invalid SHA-256')
  }
  if (
    sha256(Buffer.from(requirementsLockBytes, 'utf8')) !== lock.sitePackages.requirementsLockSha256 ||
    !Number.isSafeInteger(lock.pythonRuntime.runtimeFileCount) ||
    lock.pythonRuntime.runtimeFileCount <= 0 ||
    !Number.isSafeInteger(lock.sitePackages.wheelCount) ||
    lock.sitePackages.wheelCount <= 0 ||
    !Number.isSafeInteger(lock.sitePackages.fileCount) ||
    lock.sitePackages.fileCount <= 0
  ) {
    throw new Error('Bundled Python runtime lock is stale')
  }
}

function assertRegularPathWithinRoot(path: string, root: string, kind: 'file' | 'directory'): void {
  const resolvedRoot = resolve(root)
  const resolvedPath = resolve(path)
  const relativePath = relative(resolvedRoot, resolvedPath)
  if (relativePath === '..' || relativePath.startsWith(`..${sep}`) || isAbsolute(relativePath)) {
    throw new Error(`Python runtime path escapes its package root: ${path}`)
  }
  const rootStat = lstatSync(resolvedRoot)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) throw new Error(`Python package root is not trusted: ${root}`)
  let current = resolvedRoot
  for (const part of relativePath ? relativePath.split(sep) : []) {
    current = join(current, part)
    const stat = lstatSync(current)
    if (stat.isSymbolicLink()) throw new Error(`Python runtime path traverses a symbolic link: ${current}`)
  }
  const stat = lstatSync(resolvedPath)
  if (kind === 'file' ? !stat.isFile() : !stat.isDirectory()) {
    throw new Error(`Python runtime ${kind} is invalid: ${path}`)
  }
}

function listRegularFiles(root: string, current = root, files: string[] = []): string[] {
  const stat = lstatSync(current)
  if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error(`Python tree has an untrusted directory: ${current}`)
  for (const entry of readdirSync(current, { withFileTypes: true })) {
    const itemPath = join(current, entry.name)
    if (entry.isSymbolicLink()) throw new Error(`Python tree contains a symbolic link: ${itemPath}`)
    if (entry.isDirectory()) listRegularFiles(root, itemPath, files)
    else if (entry.isFile()) files.push(itemPath)
    else throw new Error(`Python tree contains a special file: ${itemPath}`)
  }
  return files
}

function treeIdentity(root: string, excludedName: string): TreeIdentity {
  const files = listRegularFiles(root)
    .filter((itemPath) => relative(root, itemPath).split(sep).join('/') !== excludedName)
    .sort((left, right) => Buffer.compare(
      Buffer.from(relative(root, left).split(sep).join('/'), 'utf8'),
      Buffer.from(relative(root, right).split(sep).join('/'), 'utf8')
    ))
  const seen = new Set<string>()
  const hash = createHash('sha256')
  for (const itemPath of files) {
    const relativePath = relative(root, itemPath).split(sep).join('/')
    const canonicalPath = relativePath.toLocaleLowerCase('en-US')
    if (seen.has(canonicalPath)) throw new Error(`Python tree contains a case-insensitive path collision: ${relativePath}`)
    seen.add(canonicalPath)
    const bytes = readFileSync(itemPath)
    const portableExecutable = /\.(?:exe|dll|pyd)$/iu.test(relativePath)
    const identity = portableExecutable
      ? inspectNativePayload(bytes, 'pe')
      : { payloadSha256: sha256(bytes), payloadSize: bytes.length }
    hash.update(relativePath)
    hash.update('\0')
    hash.update(portableExecutable ? 'pe-payload' : 'raw')
    hash.update('\0')
    hash.update(String(identity.payloadSize))
    hash.update('\0')
    hash.update(identity.payloadSha256)
    hash.update('\0')
  }
  return { sha256: hash.digest('hex'), fileCount: files.length }
}

function parseCanonicalManifest(path: string): Record<string, unknown> {
  const text = readFileSync(path, 'utf8')
  const value = JSON.parse(text) as unknown
  if (`${JSON.stringify(value, null, 2)}\n` !== text || !value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`Python manifest is not canonical duplicate-free JSON: ${path}`)
  }
  return value as Record<string, unknown>
}

function wheelSetSha256(wheels: unknown[]): string {
  const hash = createHash('sha256')
  const seen = new Set<string>()
  for (const value of wheels) {
    if (
      !exactKeys(value, ['fileName', 'size', 'sha256']) ||
      typeof value.fileName !== 'string' ||
      !value.fileName.endsWith('.whl') ||
      !Number.isSafeInteger(value.size) ||
      Number(value.size) <= 0 ||
      !isSha256(value.sha256)
    ) {
      throw new Error('Python wheel inventory is invalid')
    }
    const wheel = value as WheelRecord
    const canonicalName = wheel.fileName.toLocaleLowerCase('en-US')
    if (seen.has(canonicalName)) throw new Error('Python wheel inventory contains a duplicate filename')
    seen.add(canonicalName)
    hash.update(wheel.fileName)
    hash.update('\0')
    hash.update(String(wheel.size))
    hash.update('\0')
    hash.update(wheel.sha256)
    hash.update('\0')
  }
  return hash.digest('hex')
}

export function verifyPackagedPythonBundleContent(root: string): {
  pythonExecutable: string
  sitePackages: string
  runtimeIdentity: TreeIdentity
  sitePackagesIdentity: TreeIdentity
} {
  assertFrozenLock()
  const runtimeRoot = join(root, '.python-runtime', 'current')
  const sitePackages = join(root, 'python-site-packages')
  const pythonExecutable = join(runtimeRoot, 'python', 'python.exe')
  const runtimeManifestPath = join(runtimeRoot, RUNTIME_MANIFEST_NAME)
  const siteManifestPath = join(sitePackages, SITE_PACKAGES_MANIFEST_NAME)
  for (const directory of [runtimeRoot, sitePackages]) assertRegularPathWithinRoot(directory, root, 'directory')
  for (const file of [pythonExecutable, runtimeManifestPath, siteManifestPath]) {
    assertRegularPathWithinRoot(file, root, 'file')
  }

  const runtimeManifest = parseCanonicalManifest(runtimeManifestPath)
  if (!exactKeys(runtimeManifest, [
    'schemaVersion',
    'lockSha256',
    'pythonRuntimeVersion',
    'pythonSeries',
    'pythonTarget',
    'pythonVersionText',
    'assetName',
    'archiveSha256',
    'sourceTreeSha256',
    'runtimeContentSha256',
    'runtimeFileCount'
  ]) || runtimeManifest.schemaVersion !== 2) {
    throw new Error('Python runtime manifest schema is invalid')
  }
  const runtimeIdentity = treeIdentity(runtimeRoot, RUNTIME_MANIFEST_NAME)
  if (
    runtimeManifest.lockSha256 !== lockSha256 ||
    runtimeManifest.pythonRuntimeVersion !== lock.pythonRuntime.version ||
    runtimeManifest.pythonSeries !== lock.pythonRuntime.series ||
    runtimeManifest.pythonTarget !== lock.pythonRuntime.target ||
    typeof runtimeManifest.pythonVersionText !== 'string' ||
    !runtimeManifest.pythonVersionText.startsWith(`Python ${lock.pythonRuntime.version.split('+')[0]}`) ||
    runtimeManifest.assetName !== lock.pythonRuntime.assetName ||
    runtimeManifest.archiveSha256 !== lock.pythonRuntime.archiveSha256 ||
    runtimeManifest.sourceTreeSha256 !== lock.pythonRuntime.sourceTreeSha256 ||
    runtimeManifest.runtimeContentSha256 !== lock.pythonRuntime.runtimeContentSha256 ||
    runtimeManifest.runtimeFileCount !== lock.pythonRuntime.runtimeFileCount ||
    runtimeIdentity.sha256 !== lock.pythonRuntime.runtimeContentSha256 ||
    runtimeIdentity.fileCount !== lock.pythonRuntime.runtimeFileCount
  ) {
    throw new Error('Python runtime authority is stale or mismatched')
  }

  const siteManifest = parseCanonicalManifest(siteManifestPath)
  if (!exactKeys(siteManifest, [
    'schemaVersion',
    'lockSha256',
    'requirementsLockSha256',
    'pythonSeries',
    'platform',
    'wheelCount',
    'wheelSetSha256',
    'wheels',
    'contentSha256',
    'fileCount'
  ]) || siteManifest.schemaVersion !== 2 || !Array.isArray(siteManifest.wheels)) {
    throw new Error('Python site-packages manifest schema is invalid')
  }
  const sitePackagesIdentity = treeIdentity(sitePackages, SITE_PACKAGES_MANIFEST_NAME)
  if (
    siteManifest.lockSha256 !== lockSha256 ||
    siteManifest.requirementsLockSha256 !== lock.sitePackages.requirementsLockSha256 ||
    siteManifest.pythonSeries !== lock.pythonRuntime.series ||
    siteManifest.platform !== lock.sitePackages.platform ||
    siteManifest.wheelCount !== lock.sitePackages.wheelCount ||
    siteManifest.wheels.length !== lock.sitePackages.wheelCount ||
    wheelSetSha256(siteManifest.wheels) !== lock.sitePackages.wheelSetSha256 ||
    siteManifest.wheelSetSha256 !== lock.sitePackages.wheelSetSha256 ||
    siteManifest.contentSha256 !== lock.sitePackages.contentSha256 ||
    siteManifest.fileCount !== lock.sitePackages.fileCount ||
    sitePackagesIdentity.sha256 !== lock.sitePackages.contentSha256 ||
    sitePackagesIdentity.fileCount !== lock.sitePackages.fileCount
  ) {
    throw new Error('Python site-packages authority is stale or mismatched')
  }
  return { pythonExecutable, sitePackages, runtimeIdentity, sitePackagesIdentity }
}

export function verifyPackagedPythonBundle(options: {
  root: string
  packaged: boolean
  platform: string
  hostExecutable?: string
}): { pythonExecutable: string; sitePackages: string } {
  void options
  throw new Error('data_analysis_native_authority_unavailable')
}

export const pythonRuntimeAuthority = {
  lockSha256,
  runtimeContentSha256: lock.pythonRuntime.runtimeContentSha256,
  sitePackagesContentSha256: lock.sitePackages.contentSha256,
  wheelSetSha256: lock.sitePackages.wheelSetSha256
} as const

export const pythonRuntimeIntegrityInternals = {
  treeIdentity,
  wheelSetSha256
}
