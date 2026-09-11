import {
  chmodSync,
  closeSync,
  constants,
  fchmodSync,
  fstatSync,
  fsyncSync,
  linkSync,
  lstatSync,
  mkdirSync,
  openSync,
  readFileSync,
  readdirSync,
  readSync,
  realpathSync,
  rmSync,
  renameSync,
  statSync,
  unlinkSync,
  writeSync
} from 'node:fs'
import type { Stats } from 'node:fs'
import { createHash, randomUUID } from 'node:crypto'
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'

export type LegacyMigrationBlockCode =
  | 'casefold_alias'
  | 'concurrent_migration'
  | 'directory_sync_failed'
  | 'host_filesystem_authority_unavailable'
  | 'invalid_activation_receipt'
  | 'invalid_journal'
  | 'invalid_lease'
  | 'io_error'
  | 'legacy_source_changed'
  | 'legacy_source_conflict'
  | 'path_alias'
  | 'path_conflict'
  | 'permission_denied'
  | 'quiescence_unavailable'
  | 'snapshot_mismatch'
  | 'special_file'
  | 'symlink_forbidden'
  | 'target_conflict'
  | 'unexpected_path_state'

export class LegacyMigrationBlockedError extends Error {
  readonly code: LegacyMigrationBlockCode

  constructor(code: LegacyMigrationBlockCode, cause?: unknown) {
    super(`Legacy data migration blocked (${code}).`, { cause })
    this.name = 'LegacyMigrationBlockedError'
    this.code = code
  }
}

export function migrationBlocked(code: LegacyMigrationBlockCode, cause?: unknown): never {
  throw new LegacyMigrationBlockedError(code, cause)
}

function isErrnoException(error: unknown): error is NodeJS.ErrnoException {
  return typeof error === 'object' && error !== null && 'code' in error
}

function classifyIoError(error: unknown): LegacyMigrationBlockCode {
  if (isErrnoException(error) && ['EACCES', 'EPERM', 'EROFS'].includes(error.code ?? '')) {
    return 'permission_denied'
  }
  return 'io_error'
}

export type StrictPathState =
  | { kind: 'missing' }
  | {
      kind: 'directory' | 'file' | 'symlink' | 'other'
      dev: string
      ino: string
      uid: number
      gid: number
      mode: number
      size: number
    }

function stateFromStats(stats: Stats): Exclude<StrictPathState, { kind: 'missing' }> {
  const common = {
    dev: String(stats.dev),
    ino: String(stats.ino),
    uid: stats.uid,
    gid: stats.gid,
    mode: stats.mode & 0o777,
    size: stats.size
  }
  if (stats.isSymbolicLink()) return { kind: 'symlink', ...common }
  if (stats.isDirectory()) return { kind: 'directory', ...common }
  if (stats.isFile()) return { kind: 'file', ...common }
  return { kind: 'other', ...common }
}

export function inspectPathStrict(path: string): StrictPathState {
  try {
    return stateFromStats(lstatSync(path))
  } catch (error) {
    if (isErrnoException(error) && error.code === 'ENOENT') return { kind: 'missing' }
    migrationBlocked(classifyIoError(error), error)
  }
}

export function assertCanonicalDirectoryPath(path: string): void {
  const state = inspectPathStrict(path)
  if (state.kind === 'symlink') migrationBlocked('symlink_forbidden', { path })
  if (state.kind !== 'directory') migrationBlocked('unexpected_path_state', { path })
  let realPath: string
  try {
    realPath = realpathSync.native(path)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  if (realPath !== resolve(path)) migrationBlocked('path_alias', { path })
}

export type PathCaseSensitivity = 'sensitive' | 'insensitive'

export function detectPathCaseSensitivity(existingPath: string): PathCaseSensitivity {
  let current = resolve(existingPath)
  for (;;) {
    const state = inspectPathStrict(current)
    if (state.kind !== 'directory' && state.kind !== 'file') {
      migrationBlocked('unexpected_path_state', { path: current })
    }
    const name = basename(current)
    const index = name.search(/[A-Za-z]/)
    if (index >= 0) {
      const character = name.charAt(index)
      const toggled = character === character.toLocaleLowerCase('en-US')
        ? character.toLocaleUpperCase('en-US')
        : character.toLocaleLowerCase('en-US')
      const alternatePath = join(
        dirname(current),
        `${name.slice(0, index)}${toggled}${name.slice(index + 1)}`
      )
      const alternate = inspectPathStrict(alternatePath)
      if (alternate.kind === 'missing') return 'sensitive'
      if (
        alternate.kind === state.kind &&
        alternate.dev === state.dev &&
        alternate.ino === state.ino
      ) {
        return 'insensitive'
      }
      // Both spellings exist as distinct objects, which is possible only on a
      // case-sensitive namespace. Inventory collision policy remains separate.
      return 'sensitive'
    }
    const parent = dirname(current)
    if (parent === current) migrationBlocked('path_alias')
    current = parent
  }
}

function assertSameIdentity(
  path: string,
  expected: Exclude<StrictPathState, { kind: 'missing' }>,
  actual: ReturnType<typeof fstatSync>
): void {
  if (
    String(actual.dev) !== expected.dev ||
    String(actual.ino) !== expected.ino ||
    actual.size !== expected.size
  ) {
    migrationBlocked('legacy_source_changed', { path })
  }
}

function noFollowReadFlags(): number {
  return constants.O_RDONLY | (constants.O_NOFOLLOW ?? 0)
}

function noFollowWriteFlags(): number {
  return constants.O_WRONLY |
    constants.O_CREAT |
    constants.O_EXCL |
    (constants.O_NOFOLLOW ?? 0)
}

function hashOpenRegularFile(
  path: string,
  expected?: Exclude<StrictPathState, { kind: 'missing' }>
): { sha256: string; size: number } {
  let fd: number | undefined
  try {
    fd = openSync(path, noFollowReadFlags())
    const before = fstatSync(fd)
    if (!before.isFile()) migrationBlocked('special_file', { path })
    if (expected) assertSameIdentity(path, expected, before)

    const hash = createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let size = 0
    while (true) {
      const count = readSync(fd, buffer, 0, buffer.length, null)
      if (count === 0) break
      hash.update(buffer.subarray(0, count))
      size += count
    }

    const after = fstatSync(fd)
    if (
      String(after.dev) !== String(before.dev) ||
      String(after.ino) !== String(before.ino) ||
      after.size !== before.size ||
      size !== before.size
    ) {
      migrationBlocked('legacy_source_changed', { path })
    }
    return { sha256: hash.digest('hex'), size }
  } catch (error) {
    if (error instanceof LegacyMigrationBlockedError) throw error
    migrationBlocked(classifyIoError(error), error)
  } finally {
    if (fd !== undefined) closeSync(fd)
  }
  migrationBlocked('io_error')
}

function isWithin(root: string, path: string): boolean {
  const rel = relative(root, path)
  return rel === '' || (!rel.startsWith(`..${sep}`) && rel !== '..' && !isAbsolute(rel))
}

function caseFoldName(name: string): string {
  return name.normalize('NFC').toLocaleLowerCase('en-US')
}

export type DirectorySnapshotEntry =
  | {
      kind: 'directory'
      relativePath: string
      mode: number
    }
  | {
      kind: 'file'
      relativePath: string
      mode: number
      size: number
      sha256: string
    }

export type DirectorySnapshotV2 = {
  schemaVersion: 2
  manifestSha256: string
  rootMode: number
  files: number
  directories: number
  totalBytes: number
  entries: DirectorySnapshotEntry[]
}

export function sha256Bytes(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

export function stableJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableJson).join(',')}]`
  if (typeof value === 'object' && value !== null) {
    const record = value as Record<string, unknown>
    return `{${Object.keys(record).sort().map((key) => `${JSON.stringify(key)}:${stableJson(record[key])}`).join(',')}}`
  }
  return JSON.stringify(value)
}

function snapshotManifest(
  rootMode: number,
  entries: DirectorySnapshotEntry[]
): Omit<DirectorySnapshotV2, 'schemaVersion'> {
  const files = entries.filter((entry) => entry.kind === 'file')
  const directories = entries.length - files.length
  const totalBytes = files.reduce((sum, entry) => sum + entry.size, 0)
  return {
    manifestSha256: sha256Bytes(stableJson({ entries, rootMode })),
    rootMode,
    files: files.length,
    directories,
    totalBytes,
    entries
  }
}

function assertCanonicalChild(rootRealPath: string, childPath: string): void {
  let realPath: string
  try {
    realPath = realpathSync.native(childPath)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  if (!isWithin(rootRealPath, realPath)) {
    migrationBlocked('path_alias', { childPath })
  }
}

export function snapshotDirectoryStrict(
  root: string,
  options: { excludedRootEntryNames?: readonly string[] } = {}
): DirectorySnapshotV2 {
  const rootState = inspectPathStrict(root)
  if (rootState.kind === 'symlink') migrationBlocked('symlink_forbidden', { root })
  if (rootState.kind !== 'directory') migrationBlocked('unexpected_path_state', { root })

  let rootRealPath: string
  try {
    rootRealPath = realpathSync.native(root)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }

  const entries: DirectorySnapshotEntry[] = []
  const excludedRootEntryNames = new Set(options.excludedRootEntryNames ?? [])
  const walk = (absoluteDir: string, relativeDir: string): void => {
    const dirState = inspectPathStrict(absoluteDir)
    if (dirState.kind === 'symlink') migrationBlocked('symlink_forbidden', { path: absoluteDir })
    if (dirState.kind !== 'directory') migrationBlocked('unexpected_path_state', { path: absoluteDir })
    assertCanonicalChild(rootRealPath, absoluteDir)

    let names: string[]
    try {
      names = readdirSync(absoluteDir).sort((left, right) => left.localeCompare(right, 'en'))
    } catch (error) {
      migrationBlocked(classifyIoError(error), error)
    }

    const folded = new Map<string, string>()
    for (const name of names) {
      if (name.includes('\\') || name.includes('\0')) {
        migrationBlocked('path_alias', { path: absoluteDir })
      }
      const key = caseFoldName(name)
      const previous = folded.get(key)
      if (previous !== undefined && previous !== name) {
        migrationBlocked('casefold_alias', { path: absoluteDir })
      }
      folded.set(key, name)
    }

    for (const name of names) {
      if (!relativeDir && excludedRootEntryNames.has(name)) continue
      const absolutePath = join(absoluteDir, name)
      const relativePath = relativeDir ? `${relativeDir}/${name}` : name
      const state = inspectPathStrict(absolutePath)
      if (state.kind === 'symlink') migrationBlocked('symlink_forbidden', { path: absolutePath })
      if (state.kind === 'directory') {
        entries.push({ kind: 'directory', relativePath, mode: state.mode })
        walk(absolutePath, relativePath)
        continue
      }
      if (state.kind === 'file') {
        assertCanonicalChild(rootRealPath, absolutePath)
        const content = hashOpenRegularFile(absolutePath, state)
        entries.push({
          kind: 'file',
          relativePath,
          mode: state.mode,
          size: content.size,
          sha256: content.sha256
        })
        continue
      }
      migrationBlocked('special_file', { path: absolutePath })
    }
  }

  walk(root, '')
  const manifest = snapshotManifest(rootState.mode, entries)
  return { schemaVersion: 2, ...manifest }
}

export function snapshotsEqual(left: DirectorySnapshotV2, right: DirectorySnapshotV2): boolean {
  return left.schemaVersion === right.schemaVersion &&
    left.manifestSha256 === right.manifestSha256 &&
    left.rootMode === right.rootMode &&
    left.files === right.files &&
    left.directories === right.directories &&
    left.totalBytes === right.totalBytes &&
    stableJson(left.entries) === stableJson(right.entries)
}

export function assertDirectorySnapshot(
  root: string,
  expected: DirectorySnapshotV2,
  options: { excludedRootEntryNames?: readonly string[] } = {}
): void {
  const actual = snapshotDirectoryStrict(root, options)
  if (!snapshotsEqual(actual, expected)) {
    migrationBlocked('snapshot_mismatch', { root })
  }
}

export function fsyncDirectoryStrict(path: string): void {
  let fd: number | undefined
  try {
    fd = openSync(path, constants.O_RDONLY)
    fsyncSync(fd)
  } catch (error) {
    migrationBlocked('directory_sync_failed', error)
  } finally {
    if (fd !== undefined) closeSync(fd)
  }
}

function writeAllAndSync(path: string, bytes: Buffer, mode: number): void {
  let fd: number | undefined
  try {
    fd = openSync(path, noFollowWriteFlags(), mode)
    let offset = 0
    while (offset < bytes.length) {
      offset += writeSync(fd, bytes, offset, bytes.length - offset)
    }
    fsyncSync(fd)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  } finally {
    if (fd !== undefined) closeSync(fd)
  }
}

function verifyRegularFileBytes(path: string, bytes: Buffer): void {
  const state = inspectPathStrict(path)
  if (state.kind !== 'file') migrationBlocked('unexpected_path_state', { path })
  const actual = hashOpenRegularFile(path, state)
  if (actual.size !== bytes.length || actual.sha256 !== sha256Bytes(bytes)) {
    migrationBlocked('snapshot_mismatch', { path })
  }
}

export function durableWriteExclusive(
  finalPath: string,
  bytes: string | Buffer,
  mode = 0o600,
  pendingPath = `${finalPath}.pending-${randomUUID()}`
): { sha256: string; pendingPath: string } {
  assertCanonicalDirectoryPath(dirname(finalPath))
  const buffer = Buffer.isBuffer(bytes) ? bytes : Buffer.from(bytes, 'utf8')
  if (inspectPathStrict(finalPath).kind !== 'missing') {
    migrationBlocked('target_conflict', { finalPath })
  }
  if (inspectPathStrict(pendingPath).kind !== 'missing') {
    migrationBlocked('target_conflict', { pendingPath })
  }

  writeAllAndSync(pendingPath, buffer, mode)
  verifyRegularFileBytes(pendingPath, buffer)
  try {
    linkSync(pendingPath, finalPath)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(finalPath))
  verifyRegularFileBytes(finalPath, buffer)
  try {
    unlinkSync(pendingPath)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(finalPath))
  return { sha256: sha256Bytes(buffer), pendingPath }
}

export function durableReplaceRegularFile(
  finalPath: string,
  bytes: string | Buffer,
  mode: number
): void {
  assertCanonicalDirectoryPath(dirname(finalPath))
  const before = inspectPathStrict(finalPath)
  if (before.kind !== 'file') migrationBlocked('unexpected_path_state', { finalPath })
  const buffer = Buffer.isBuffer(bytes) ? bytes : Buffer.from(bytes, 'utf8')
  const pendingPath = `${finalPath}.migration-v2-${randomUUID()}`
  writeAllAndSync(pendingPath, buffer, mode)
  verifyRegularFileBytes(pendingPath, buffer)

  const rechecked = inspectPathStrict(finalPath)
  if (
    rechecked.kind !== 'file' ||
    rechecked.dev !== before.dev ||
    rechecked.ino !== before.ino ||
    rechecked.size !== before.size
  ) {
    migrationBlocked('legacy_source_changed', { finalPath })
  }
  try {
    renameSync(pendingPath, finalPath)
    chmodSync(finalPath, mode)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(finalPath))
  verifyRegularFileBytes(finalPath, buffer)
}

function copyFileBoundToSnapshot(
  sourcePath: string,
  targetPath: string,
  expected: Extract<DirectorySnapshotEntry, { kind: 'file' }>
): void {
  const sourceState = inspectPathStrict(sourcePath)
  if (sourceState.kind !== 'file') migrationBlocked('legacy_source_changed', { sourcePath })

  let sourceFd: number | undefined
  let targetFd: number | undefined
  try {
    sourceFd = openSync(sourcePath, noFollowReadFlags())
    const sourceBefore = fstatSync(sourceFd)
    assertSameIdentity(sourcePath, sourceState, sourceBefore)
    targetFd = openSync(targetPath, noFollowWriteFlags(), expected.mode)

    const hash = createHash('sha256')
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let size = 0
    while (true) {
      const count = readSync(sourceFd, buffer, 0, buffer.length, null)
      if (count === 0) break
      hash.update(buffer.subarray(0, count))
      let offset = 0
      while (offset < count) {
        offset += writeSync(targetFd, buffer, offset, count - offset)
      }
      size += count
    }
    fchmodSync(targetFd, expected.mode)
    fsyncSync(targetFd)

    const sourceAfter = fstatSync(sourceFd)
    if (
      String(sourceAfter.dev) !== String(sourceBefore.dev) ||
      String(sourceAfter.ino) !== String(sourceBefore.ino) ||
      sourceAfter.size !== sourceBefore.size ||
      size !== expected.size ||
      hash.digest('hex') !== expected.sha256
    ) {
      migrationBlocked('legacy_source_changed', { sourcePath })
    }
  } catch (error) {
    if (error instanceof LegacyMigrationBlockedError) throw error
    migrationBlocked(classifyIoError(error), error)
  } finally {
    if (targetFd !== undefined) closeSync(targetFd)
    if (sourceFd !== undefined) closeSync(sourceFd)
  }
}

export function copySnapshotToNewStage(
  sourceRoot: string,
  stageRoot: string,
  expected: DirectorySnapshotV2,
  sourceOptions: { excludedRootEntryNames?: readonly string[] } = {}
): void {
  assertCanonicalDirectoryPath(dirname(stageRoot))
  if (inspectPathStrict(stageRoot).kind !== 'missing') {
    migrationBlocked('target_conflict', { stageRoot })
  }
  try {
    mkdirSync(stageRoot, { recursive: false, mode: 0o700 })
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(stageRoot))

  const directories = expected.entries
    .filter((entry): entry is Extract<DirectorySnapshotEntry, { kind: 'directory' }> => entry.kind === 'directory')
    .sort((left, right) => {
      const depth = left.relativePath.split('/').length - right.relativePath.split('/').length
      return depth || left.relativePath.localeCompare(right.relativePath, 'en')
    })
  for (const entry of directories) {
    const targetPath = join(stageRoot, ...entry.relativePath.split('/'))
    try {
      mkdirSync(targetPath, { recursive: false, mode: entry.mode })
    } catch (error) {
      migrationBlocked(classifyIoError(error), error)
    }
    fsyncDirectoryStrict(dirname(targetPath))
  }

  for (const entry of expected.entries) {
    if (entry.kind !== 'file') continue
    const sourcePath = join(sourceRoot, ...entry.relativePath.split('/'))
    const targetPath = join(stageRoot, ...entry.relativePath.split('/'))
    copyFileBoundToSnapshot(sourcePath, targetPath, entry)
    fsyncDirectoryStrict(dirname(targetPath))
  }
  fsyncDirectoryStrict(stageRoot)
  assertDirectorySnapshot(sourceRoot, expected, sourceOptions)
}

export function applySnapshotPermissions(
  root: string,
  expected: DirectorySnapshotV2
): void {
  const directories = expected.entries
    .filter((entry): entry is Extract<DirectorySnapshotEntry, { kind: 'directory' }> => entry.kind === 'directory')
    .sort((left, right) => {
      const depth = right.relativePath.split('/').length - left.relativePath.split('/').length
      return depth || right.relativePath.localeCompare(left.relativePath, 'en')
    })
  for (const entry of directories) {
    const path = join(root, ...entry.relativePath.split('/'))
    const state = inspectPathStrict(path)
    if (state.kind !== 'directory') migrationBlocked('snapshot_mismatch', { path })
    try {
      chmodSync(path, entry.mode)
    } catch (error) {
      migrationBlocked(classifyIoError(error), error)
    }
    const readback = inspectPathStrict(path)
    if (readback.kind !== 'directory' || readback.mode !== entry.mode) {
      migrationBlocked('snapshot_mismatch', { path })
    }
    fsyncDirectoryStrict(path)
  }
  // Flush the complete generation before restoring the source root mode. A
  // restrictive source mode can make a later directory open impossible.
  fsyncDirectoryStrict(root)
  try {
    chmodSync(root, expected.rootMode)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  const rootReadback = inspectPathStrict(root)
  if (rootReadback.kind !== 'directory' || rootReadback.mode !== expected.rootMode) {
    migrationBlocked('snapshot_mismatch', { root })
  }
}

export type HostDirectoryActivationPrimitiveV2 =
  | 'darwin-renameatx-np-rename-excl'
  | 'linux-renameat2-rename-noreplace'
  | 'windows-handle-move-fail-if-exists'

export type HostDirectoryActivationRequestV2 = {
  schemaVersion: 2
  requiredPrimitive: HostDirectoryActivationPrimitiveV2
  hostScopeSha256: string
  hostSemanticReceiptSha256: string
  transactionId: string
  ownerId: string
  stagePath: string
  targetPath: string
  stageDev: string
  stageIno: string
  targetParentDev: string
  targetParentIno: string
  expectedManifestSha256: string
}

export type HostDirectoryActivationReceiptV2 = {
  schemaVersion: 2
  requestSha256: string
  primitive: HostDirectoryActivationPrimitiveV2
  stageDev: string
  stageIno: string
  targetDev: string
  targetIno: string
  targetParentDev: string
  targetParentIno: string
  targetParentDurablySynced: true
}

export function activateStageDirectory(
  stagePath: string,
  targetPath: string,
  expectedStage: DirectorySnapshotV2,
  expectedStageIdentity: { dev: string; ino: string },
  request: HostDirectoryActivationRequestV2,
  activateOrRecoverNoReplace: (
    request: HostDirectoryActivationRequestV2
  ) => HostDirectoryActivationReceiptV2
): {
  dev: string
  ino: string
  activationReceipt: HostDirectoryActivationReceiptV2
} {
  assertCanonicalDirectoryPath(dirname(stagePath))
  assertCanonicalDirectoryPath(dirname(targetPath))
  const stageState = inspectPathStrict(stagePath)
  const targetState = inspectPathStrict(targetPath)
  if (stageState.kind === 'missing' && targetState.kind === 'directory') {
    if (
      targetState.dev !== expectedStageIdentity.dev ||
      targetState.ino !== expectedStageIdentity.ino
    ) {
      migrationBlocked('target_conflict', { targetPath })
    }
    assertDirectorySnapshot(targetPath, expectedStage)
  } else {
    if (stageState.kind !== 'directory') {
      migrationBlocked('unexpected_path_state', { stagePath })
    }
    if (
      stageState.dev !== expectedStageIdentity.dev ||
      stageState.ino !== expectedStageIdentity.ino
    ) {
      migrationBlocked('target_conflict', { stagePath })
    }
    if (targetState.kind !== 'missing') migrationBlocked('target_conflict', { targetPath })
    assertDirectorySnapshot(stagePath, expectedStage)
  }
  const expectedRequestSha256 = sha256Bytes(stableJson(request))
  if (
    request.schemaVersion !== 2 ||
    request.stagePath !== stagePath ||
    request.targetPath !== targetPath ||
    request.stageDev !== expectedStageIdentity.dev ||
    request.stageIno !== expectedStageIdentity.ino ||
    !/^[a-f0-9]{64}$/.test(request.hostScopeSha256) ||
    !/^[a-f0-9]{64}$/.test(request.hostSemanticReceiptSha256) ||
    request.expectedManifestSha256 !== expectedStage.manifestSha256
  ) {
    migrationBlocked('invalid_activation_receipt')
  }
  let receipt: HostDirectoryActivationReceiptV2
  try {
    receipt = activateOrRecoverNoReplace(request)
  } catch (error) {
    if (error instanceof LegacyMigrationBlockedError) throw error
    migrationBlocked('io_error', error)
  }
  if (typeof receipt !== 'object' || receipt === null || Array.isArray(receipt)) {
    migrationBlocked('invalid_activation_receipt')
  }
  const receiptKeys = Object.keys(receipt).sort()
  const expectedReceiptKeys = [
    'schemaVersion',
    'requestSha256',
    'primitive',
    'stageDev',
    'stageIno',
    'targetDev',
    'targetIno',
    'targetParentDev',
    'targetParentIno',
    'targetParentDurablySynced'
  ].sort()
  if (
    receiptKeys.length !== expectedReceiptKeys.length ||
    receiptKeys.some((key, index) => key !== expectedReceiptKeys[index]) ||
    receipt.schemaVersion !== 2 ||
    receipt.requestSha256 !== expectedRequestSha256 ||
    receipt.primitive !== request.requiredPrimitive ||
    receipt.stageDev !== expectedStageIdentity.dev ||
    receipt.stageIno !== expectedStageIdentity.ino ||
    receipt.targetDev !== expectedStageIdentity.dev ||
    receipt.targetIno !== expectedStageIdentity.ino ||
    receipt.targetParentDev !== request.targetParentDev ||
    receipt.targetParentIno !== request.targetParentIno ||
    receipt.targetParentDurablySynced !== true
  ) {
    migrationBlocked('invalid_activation_receipt')
  }
  const activated = inspectPathStrict(targetPath)
  if (
    activated.kind !== 'directory' ||
    activated.dev !== expectedStageIdentity.dev ||
    activated.ino !== expectedStageIdentity.ino ||
    inspectPathStrict(stagePath).kind !== 'missing'
  ) {
    migrationBlocked('target_conflict', { targetPath })
  }
  assertDirectorySnapshot(targetPath, expectedStage)
  return {
    dev: activated.dev,
    ino: activated.ino,
    activationReceipt: receipt
  }
}

export function removeJournalOwnedStage(input: {
  stagePath: string
  targetPath: string
  transactionId: string
  ownerId: string
}): void {
  assertCanonicalDirectoryPath(dirname(input.stagePath))
  const expectedName = `.analytix-migration-v2-${input.transactionId}-${input.ownerId}.stage`
  if (
    dirname(input.stagePath) !== dirname(input.targetPath) ||
    input.stagePath !== join(dirname(input.targetPath), expectedName)
  ) {
    migrationBlocked('invalid_journal')
  }
  const state = inspectPathStrict(input.stagePath)
  if (state.kind === 'missing') return
  if (state.kind === 'symlink') migrationBlocked('symlink_forbidden', { stagePath: input.stagePath })
  if (state.kind !== 'directory') migrationBlocked('unexpected_path_state', { stagePath: input.stagePath })
  // Inventory the exact journal-bound tree before recursive removal so an
  // injected symlink, junction, socket, or case-fold alias is never followed
  // or silently deleted as migration-owned state.
  snapshotDirectoryStrict(input.stagePath)
  try {
    rmSync(input.stagePath, { recursive: true, force: false })
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(input.stagePath))
  if (inspectPathStrict(input.stagePath).kind !== 'missing') {
    migrationBlocked('io_error')
  }
}

export function unlinkExactRegularFile(path: string, expectedSha256?: string): void {
  assertCanonicalDirectoryPath(dirname(path))
  const state = inspectPathStrict(path)
  if (state.kind === 'missing') return
  if (state.kind !== 'file') migrationBlocked('unexpected_path_state', { path })
  if (expectedSha256) {
    const actual = hashOpenRegularFile(path, state)
    if (actual.sha256 !== expectedSha256) migrationBlocked('invalid_lease', { path })
  }
  const before = statSync(path)
  if (String(before.dev) !== state.dev || String(before.ino) !== state.ino) {
    migrationBlocked('path_alias', { path })
  }
  try {
    unlinkSync(path)
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(path))
}

export function readRegularFileStrict(path: string): { raw: string; mode: number; sha256: string } {
  const state = inspectPathStrict(path)
  if (state.kind !== 'file') migrationBlocked('unexpected_path_state', { path })
  try {
    const raw = readFileSync(path, 'utf8')
    const verified = hashOpenRegularFile(path, state)
    if (verified.sha256 !== sha256Bytes(raw)) migrationBlocked('legacy_source_changed', { path })
    return { raw, mode: state.mode, sha256: verified.sha256 }
  } catch (error) {
    if (error instanceof LegacyMigrationBlockedError) throw error
    migrationBlocked(classifyIoError(error), error)
  }
}

export function ensureNewDirectory(path: string, mode = 0o700): void {
  assertCanonicalDirectoryPath(dirname(path))
  if (inspectPathStrict(path).kind !== 'missing') migrationBlocked('target_conflict', { path })
  try {
    mkdirSync(path, { recursive: false, mode })
  } catch (error) {
    migrationBlocked(classifyIoError(error), error)
  }
  fsyncDirectoryStrict(dirname(path))
}

export function readJsonRegularFileStrict(path: string): unknown {
  const file = readRegularFileStrict(path)
  try {
    return JSON.parse(file.raw)
  } catch (error) {
    migrationBlocked('invalid_journal', error)
  }
}
