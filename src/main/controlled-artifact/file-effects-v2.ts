import { createHash, randomBytes } from 'node:crypto'
import { constants, linkSync, lstatSync, type BigIntStats } from 'node:fs'
import {
  lstat,
  open,
  realpath,
  unlink,
  type FileHandle
} from 'node:fs/promises'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import type {
  ControlledArtifactCommitReceiptV2,
  ControlledArtifactPreparedReleaseV2,
  ControlledArtifactReleaseEffectInputV2,
  ControlledArtifactReleasePreparationV2
} from './host-v2'
import {
  ControlledArtifactExportJournalV2,
  type ControlledArtifactExportJournalAttemptV2
} from './export-journal-v2'

const FILE_EFFECT_ERROR = 'controlled_artifact_file_effect_v2_failed'
const TARGET_IDENTITY_PURPOSE = 'analytix.controlled-artifact-export-target/v2'
const TARGET_IDENTITY_DOMAIN = Buffer.from(`${TARGET_IDENTITY_PURPOSE}\0`, 'utf8')
const CONTROLLED_ARTIFACT_MEDIA_TYPE = 'application/vnd.analytix.controlled-case-evidence+json'
const STAGE_FILE_PATTERN = /^\.stage-v2-[A-Za-z0-9_-]{22}\.tmp$/u
const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const NO_FOLLOW = process.platform === 'win32' ? 0 : constants.O_NOFOLLOW
const DIRECTORY = process.platform === 'win32' ? 0 : constants.O_DIRECTORY
const VERIFY_BUFFER_BYTES = 64 << 10

type FileIdentityV2 = {
  dev: bigint
  ino: bigint
}

type DirectoryAuthorityV2 = FileIdentityV2 & {
  path: string
  handle: FileHandle
}

type FrozenExportTargetV2 = {
  parentPath: string
  parentIdentity: FileIdentityV2
  targetName: string
}

type StagedArtifactMetadataV2 = {
  releaseTargetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
  mediaType: string
  authorizedUntil: string
}

/**
 * Freezes a new export target without exposing it to Go, preload, or the
 * renderer. Artifact bytes are staged only after HostV2 has verified the
 * signed access receipt. The target becomes visible through a no-replace
 * same-directory hard link only inside commitBefore.
 */
export async function freezeControlledArtifactExportTargetV2(
  targetPath: string,
  journal: ControlledArtifactExportJournalV2,
  now: () => string = currentTimestamp
): Promise<ControlledArtifactExportTargetV2> {
  try {
    if (!(journal instanceof ControlledArtifactExportJournalV2) || typeof now !== 'function') {
      throw fixedFileEffectError()
    }
    const target = exactAbsolutePath(targetPath)
    const targetName = basename(target)
    if (!isSafeLeafName(targetName)) throw fixedFileEffectError()
    const parent = await openDirectoryAuthorityV2(dirname(target))
    try {
      await requireAbsentV2(target)
      const frozen: FrozenExportTargetV2 = {
        parentPath: parent.path,
        parentIdentity: identityOf(parent),
        targetName
      }
      return new ControlledArtifactExportTargetV2(frozen, journal, now)
    } finally {
      await parent.handle.close()
    }
  } catch {
    throw fixedFileEffectError()
  }
}

export class ControlledArtifactExportTargetV2 {
  private available = true
  private readonly digest: string

  constructor(
    private readonly target: FrozenExportTargetV2,
    private readonly journal: ControlledArtifactExportJournalV2,
    private readonly now: () => string
  ) {
    this.digest = deriveExportTargetIdentityDigestV2(target)
  }

  targetIdentityDigest(): string {
    return this.digest
  }

  createPreparation(): ControlledArtifactReleasePreparationV2 {
    if (!this.available) throw fixedFileEffectError()
    this.available = false
    return async (input, signal) => {
      try {
        requireNotAborted(signal)
        const metadata = validateReleaseInputV2(input, 'export', this.digest)
        return await stageExportV2(this.target, input.body, metadata, this.journal, this.now, signal)
      } catch {
        throw fixedFileEffectError()
      }
    }
  }

  revoke(): void {
    this.available = false
  }

  close(): void {
    this.revoke()
  }

  toJSON(): never {
    throw fixedFileEffectError()
  }
}

class PreparedExportReleaseV2 implements ControlledArtifactPreparedReleaseV2 {
  private state: 'ready' | 'committing' | 'target_linked' | 'committed' |
    'indeterminate' | 'aborting' | 'aborted' = 'ready'
  private stagePresent = true
  private operationTail: Promise<void> = Promise.resolve()

  constructor(
    private readonly target: FrozenExportTargetV2,
    private readonly stageName: string,
    private readonly stageIdentity: FileIdentityV2,
    private readonly metadata: StagedArtifactMetadataV2,
    private readonly journal: ControlledArtifactExportJournalAttemptV2,
    private readonly now: () => string
  ) {}

  commitBefore(authorizedUntil: string, signal: AbortSignal): Promise<ControlledArtifactCommitReceiptV2> {
    return this.runExclusive(async () => {
      if (this.state !== 'ready' || authorizedUntil !== this.metadata.authorizedUntil) {
        throw fixedFileEffectError()
      }
      this.state = 'committing'
      let parent: DirectoryAuthorityV2 | null = null
      try {
        requireBeforeDeadline(this.now, authorizedUntil, signal)
        parent = await openDirectoryAuthorityV2(this.target.parentPath)
        requireSameIdentity(parent, this.target.parentIdentity)
        const stagePath = join(parent.path, this.stageName)
        const targetPath = join(parent.path, this.target.targetName)
        await requireExactPrivateFileV2(stagePath, this.stageIdentity, [1n])
        await verifyFileDigestV2(
          stagePath,
          this.stageIdentity,
          this.metadata.artifactByteLength,
          this.metadata.artifactSha256,
          [1n],
          signal
        )
        await this.journal.markLinkIntent()
        requireParentAndTargetAbsentSyncV2(parent, this.target.parentIdentity, targetPath)
        requireBeforeDeadline(this.now, authorizedUntil, signal)
        linkSync(stagePath, targetPath)
        this.state = 'target_linked'
        await parent.handle.sync()
        await this.journal.markTargetLinked()
        requireBeforeDeadline(this.now, authorizedUntil, signal)
        await requireExactPrivateFileV2(stagePath, this.stageIdentity, [2n])
        await requireExactPrivateFileV2(targetPath, this.stageIdentity, [2n])
        const finalFile = await verifyFileDigestV2(
          targetPath,
          this.stageIdentity,
          this.metadata.artifactByteLength,
          this.metadata.artifactSha256,
          [2n],
          signal
        )
        await unlink(stagePath)
        this.stagePresent = false
        await parent.handle.sync()
        await requireExactPrivateFileV2(targetPath, this.stageIdentity, [1n])
        await verifyFileDigestV2(
          targetPath,
          this.stageIdentity,
          this.metadata.artifactByteLength,
          this.metadata.artifactSha256,
          [1n],
          signal
        )
        await revalidateDirectoryV2(parent.path, this.target.parentIdentity)
        await this.journal.complete()
        const committedAt = requireBeforeDeadline(this.now, authorizedUntil, signal)
        this.state = 'committed'
        return Object.freeze({
          releaseTargetIdentityDigest: deriveExportTargetIdentityDigestV2(this.target),
          artifactSha256: finalFile.sha256,
          artifactByteLength: finalFile.byteLength,
          mediaType: this.metadata.mediaType,
          committedAt
        })
      } catch {
        this.state = 'indeterminate'
        // Once the no-replace link may exist, never remove the target here or
        // in abort. The caller records an indeterminate release instead.
        throw fixedFileEffectError()
      } finally {
        await parent?.handle.close().catch(() => undefined)
      }
    })
  }

  abort(signal: AbortSignal): Promise<void> {
    return this.runExclusive(async () => {
      if (this.state === 'committed' || this.state === 'aborted') return
      this.state = 'aborting'
      let parent: DirectoryAuthorityV2 | null = null
      try {
        requireNotAborted(signal)
        parent = await openDirectoryAuthorityV2(this.target.parentPath)
        requireSameIdentity(parent, this.target.parentIdentity)
        if (this.stagePresent) {
          const stagePath = join(parent.path, this.stageName)
          await requireExactPrivateFileV2(stagePath, this.stageIdentity, [1n, 2n])
          requireNotAborted(signal)
          await unlink(stagePath)
          this.stagePresent = false
          await parent.handle.sync()
        }
        await this.journal.complete()
        this.state = 'aborted'
      } catch {
        this.state = 'indeterminate'
        throw fixedFileEffectError()
      } finally {
        await parent?.handle.close().catch(() => undefined)
      }
    })
  }

  private async runExclusive<T>(operation: () => Promise<T>): Promise<T> {
    const previous = this.operationTail
    let release!: () => void
    this.operationTail = new Promise<void>((resolve) => { release = resolve })
    await previous
    try {
      return await operation()
    } finally {
      release()
    }
  }
}

async function stageExportV2(
  target: FrozenExportTargetV2,
  body: Buffer,
  metadata: StagedArtifactMetadataV2,
  journal: ControlledArtifactExportJournalV2,
  now: () => string,
  signal: AbortSignal
): Promise<ControlledArtifactPreparedReleaseV2> {
  let parent: DirectoryAuthorityV2 | null = null
  let stage: FileHandle | null = null
  let stageName = ''
  let stageIdentity: FileIdentityV2 | null = null
  let journalAttempt: ControlledArtifactExportJournalAttemptV2 | null = null
  try {
    requireNotAborted(signal)
    parent = await openDirectoryAuthorityV2(target.parentPath)
    requireSameIdentity(parent, target.parentIdentity)
    await requireAbsentV2(join(parent.path, target.targetName))
    stageName = `.stage-v2-${randomBytes(16).toString('base64url')}.tmp`
    if (!STAGE_FILE_PATTERN.test(stageName)) throw fixedFileEffectError()
    const stagePath = join(parent.path, stageName)
    stage = await open(
      stagePath,
      constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | NO_FOLLOW,
      0o600
    )
    const before = await stage.stat({ bigint: true })
    requirePrivateRegularFileV2(before, [1n])
    stageIdentity = identityOf(before)
    journalAttempt = await journal.begin({
      parentPath: target.parentPath,
      parentDev: target.parentIdentity.dev,
      parentIno: target.parentIdentity.ino,
      targetName: target.targetName,
      stageName,
      stageDev: stageIdentity.dev,
      stageIno: stageIdentity.ino,
      targetIdentityDigest: metadata.releaseTargetIdentityDigest,
      artifactSha256: metadata.artifactSha256,
      artifactByteLength: metadata.artifactByteLength
    })
    await writeAllV2(stage, body, signal)
    await stage.sync()
    const after = await stage.stat({ bigint: true })
    requirePrivateRegularFileV2(after, [1n])
    requireSameIdentity(after, stageIdentity)
    if (after.size !== BigInt(metadata.artifactByteLength)) throw fixedFileEffectError()
    await stage.close()
    stage = null
    await requireExactPrivateFileV2(stagePath, stageIdentity, [1n])
    await verifyFileDigestV2(
      stagePath,
      stageIdentity,
      metadata.artifactByteLength,
      metadata.artifactSha256,
      [1n],
      signal
    )
    await revalidateDirectoryV2(parent.path, target.parentIdentity)
    await journalAttempt.markStaged()
    requireNotAborted(signal)
    return new PreparedExportReleaseV2(target, stageName, stageIdentity, metadata, journalAttempt, now)
  } catch {
    await stage?.close().catch(() => undefined)
    let stageRemoved = false
    if (parent && stageName && stageIdentity) {
      try {
        await removeExactStageV2(parent, stageName, stageIdentity)
        stageRemoved = true
      } catch {
        journalAttempt?.failClosed()
      }
    }
    if (journalAttempt && stageRemoved) {
      await journalAttempt.complete().catch(() => journalAttempt?.failClosed())
    }
    throw fixedFileEffectError()
  } finally {
    await parent?.handle.close().catch(() => undefined)
  }
}

function validateReleaseInputV2(
  input: ControlledArtifactReleaseEffectInputV2,
  expectedAction: 'export',
  expectedTargetIdentityDigest: string
): StagedArtifactMetadataV2 {
  const receipt = input?.receipt
  if (!input || input.action !== expectedAction || receipt?.accessAction !== expectedAction ||
    !Buffer.isBuffer(input.body) || input.body.length === 0 ||
    receipt.artifactByteLength !== input.body.length ||
    !SHA256_PATTERN.test(receipt.artifactSha256) || receipt.artifactSha256 !== sha256(input.body) ||
    !SHA256_PATTERN.test(receipt.targetIdentityDigest) ||
    !SHA256_PATTERN.test(input.releaseTargetIdentityDigest) ||
    input.releaseTargetIdentityDigest !== expectedTargetIdentityDigest ||
    receipt.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE ||
    parseRFC3339Nano(receipt.authorizedUntil) === null) {
    throw fixedFileEffectError()
  }
  return Object.freeze({
    releaseTargetIdentityDigest: input.releaseTargetIdentityDigest,
    artifactSha256: receipt.artifactSha256,
    artifactByteLength: receipt.artifactByteLength,
    mediaType: receipt.mediaType,
    authorizedUntil: receipt.authorizedUntil
  })
}

async function openDirectoryAuthorityV2(path: string): Promise<DirectoryAuthorityV2> {
  const exact = exactAbsolutePath(path)
  const resolved = await realpath(exact)
  if (resolved !== exact) throw fixedFileEffectError()
  const before = await lstat(exact, { bigint: true })
  requireDirectoryV2(before)
  const handle = await open(exact, constants.O_RDONLY | DIRECTORY | NO_FOLLOW)
  try {
    const opened = await handle.stat({ bigint: true })
    requireDirectoryV2(opened)
    requireSameIdentity(before, opened)
    const after = await lstat(exact, { bigint: true })
    requireDirectoryV2(after)
    requireSameIdentity(opened, after)
    return { path: exact, handle, ...identityOf(opened) }
  } catch {
    await handle.close().catch(() => undefined)
    throw fixedFileEffectError()
  }
}

async function revalidateDirectoryV2(path: string, expected: FileIdentityV2): Promise<void> {
  const current = await openDirectoryAuthorityV2(path)
  try {
    requireSameIdentity(current, expected)
  } finally {
    await current.handle.close()
  }
}

async function requireExactPrivateFileV2(
  path: string,
  expected: FileIdentityV2,
  expectedLinks: readonly bigint[]
): Promise<void> {
  const before = await lstat(path, { bigint: true })
  requirePrivateRegularFileV2(before, expectedLinks)
  requireSameIdentity(before, expected)
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  try {
    const opened = await file.stat({ bigint: true })
    requirePrivateRegularFileV2(opened, expectedLinks)
    requireSameIdentity(opened, expected)
    requireSameIdentity(before, opened)
  } finally {
    await file.close()
  }
}

async function verifyFileDigestV2(
  path: string,
  expectedIdentity: FileIdentityV2,
  expectedLength: number,
  expectedSHA256: string,
  expectedLinks: readonly bigint[],
  signal: AbortSignal
): Promise<{ byteLength: number; sha256: string }> {
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  const buffer = Buffer.allocUnsafe(VERIFY_BUFFER_BYTES)
  try {
    const before = await file.stat({ bigint: true })
    requirePrivateRegularFileV2(before, expectedLinks)
    requireSameIdentity(before, expectedIdentity)
    if (before.size !== BigInt(expectedLength)) throw fixedFileEffectError()
    const hash = createHash('sha256')
    let offset = 0
    while (offset < expectedLength) {
      requireNotAborted(signal)
      const length = Math.min(buffer.length, expectedLength - offset)
      const result = await file.read(buffer, 0, length, offset)
      if (result.bytesRead !== length) throw fixedFileEffectError()
      hash.update(buffer.subarray(0, length))
      buffer.fill(0, 0, length)
      offset += length
    }
    const actualSHA256 = hash.digest('hex')
    if (actualSHA256 !== expectedSHA256) throw fixedFileEffectError()
    const after = await file.stat({ bigint: true })
    requirePrivateRegularFileV2(after, expectedLinks)
    requireSameIdentity(after, expectedIdentity)
    requireSameIdentity(before, after)
    return { byteLength: Number(after.size), sha256: actualSHA256 }
  } finally {
    buffer.fill(0)
    await file.close()
  }
}

function requireParentAndTargetAbsentSyncV2(
  parent: DirectoryAuthorityV2,
  expectedParent: FileIdentityV2,
  targetPath: string
): void {
  const currentParent = lstatSync(parent.path, { bigint: true })
  requireDirectoryV2(currentParent)
  requireSameIdentity(currentParent, expectedParent)
  try {
    lstatSync(targetPath)
  } catch (error) {
    if (isNodeErrorCode(error, 'ENOENT')) return
    throw fixedFileEffectError()
  }
  throw fixedFileEffectError()
}

async function writeAllV2(file: FileHandle, body: Buffer, signal: AbortSignal): Promise<void> {
  let offset = 0
  while (offset < body.length) {
    requireNotAborted(signal)
    const length = Math.min(VERIFY_BUFFER_BYTES, body.length - offset)
    const result = await file.write(body, offset, length, null)
    if (result.bytesWritten !== length) throw fixedFileEffectError()
    offset += length
  }
  requireNotAborted(signal)
}

async function removeExactStageV2(
  parent: DirectoryAuthorityV2,
  stageName: string,
  expected: FileIdentityV2
): Promise<void> {
  if (!STAGE_FILE_PATTERN.test(stageName)) throw fixedFileEffectError()
  requireSameIdentity(parent, await currentDirectoryIdentityV2(parent.path))
  const stagePath = join(parent.path, stageName)
  await requireExactPrivateFileV2(stagePath, expected, [1n, 2n])
  await unlink(stagePath)
  await parent.handle.sync()
}

async function currentDirectoryIdentityV2(path: string): Promise<FileIdentityV2> {
  const current = await openDirectoryAuthorityV2(path)
  try {
    return identityOf(current)
  } finally {
    await current.handle.close()
  }
}

async function requireAbsentV2(path: string): Promise<void> {
  try {
    await lstat(path)
  } catch (error) {
    if (isNodeErrorCode(error, 'ENOENT')) return
    throw fixedFileEffectError()
  }
  throw fixedFileEffectError()
}

function requireDirectoryV2(stats: BigIntStats): void {
  if (!stats.isDirectory() || stats.isSymbolicLink() || stats.nlink < 1n ||
    !ownedByCurrentUserV2(stats) || (stats.mode & 0o022n) !== 0n) {
    throw fixedFileEffectError()
  }
}

function requirePrivateRegularFileV2(stats: BigIntStats, expectedLinks: readonly bigint[]): void {
  if (!stats.isFile() || stats.isSymbolicLink() || !expectedLinks.includes(stats.nlink) ||
    !ownedByCurrentUserV2(stats) || (stats.mode & 0o077n) !== 0n) {
    throw fixedFileEffectError()
  }
}

function ownedByCurrentUserV2(stats: BigIntStats): boolean {
  return typeof process.getuid !== 'function' || stats.uid === BigInt(process.getuid())
}

function requireSameIdentity(left: FileIdentityV2, right: FileIdentityV2): void {
  if (left.dev !== right.dev || left.ino !== right.ino) throw fixedFileEffectError()
}

function identityOf(value: FileIdentityV2): FileIdentityV2 {
  return { dev: value.dev, ino: value.ino }
}

function deriveExportTargetIdentityDigestV2(target: FrozenExportTargetV2): string {
  const canonical = JSON.stringify({
    schemaVersion: 2,
    purpose: TARGET_IDENTITY_PURPOSE,
    action: 'export',
    platform: process.platform,
    volumeIdentity: String(target.parentIdentity.dev),
    parentRealPathDigest: sha256(Buffer.from(target.parentPath, 'utf8')),
    parentFileIdentity: {
      dev: String(target.parentIdentity.dev),
      ino: String(target.parentIdentity.ino)
    },
    leafNameDigest: sha256(Buffer.from(target.targetName, 'utf8'))
  })
  return createHash('sha256').update(TARGET_IDENTITY_DOMAIN).update(canonical, 'utf8').digest('hex')
}

function requireNotAborted(signal: AbortSignal): void {
  if (!(signal instanceof AbortSignal) || signal.aborted) throw fixedFileEffectError()
}

function requireBeforeDeadline(now: () => string, authorizedUntil: string, signal: AbortSignal): string {
  requireNotAborted(signal)
  const current = now()
  const currentNS = parseRFC3339Nano(current)
  const deadlineNS = parseRFC3339Nano(authorizedUntil)
  if (currentNS === null || deadlineNS === null || currentNS >= deadlineNS) throw fixedFileEffectError()
  return current
}

function parseRFC3339Nano(value: unknown): bigint | null {
  if (typeof value !== 'string') return null
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):([0-5]\d):([0-5]\d)(?:\.(\d{0,8}[1-9]))?Z$/.exec(value)
  if (!match) return null
  const wholeSeconds = `${match[1]}-${match[2]}-${match[3]}T${match[4]}:${match[5]}:${match[6]}Z`
  const milliseconds = Date.parse(wholeSeconds)
  if (!Number.isFinite(milliseconds) ||
    new Date(milliseconds).toISOString().replace('.000Z', 'Z') !== wholeSeconds) {
    return null
  }
  const fraction = (match[7] ?? '').padEnd(9, '0')
  return BigInt(Math.trunc(milliseconds / 1_000)) * 1_000_000_000n + BigInt(fraction || '0')
}

function currentTimestamp(): string {
  const iso = new Date().toISOString()
  return iso.replace(/\.000Z$/, 'Z').replace(/(\.\d*?[1-9])0+Z$/, '$1Z')
}

function exactAbsolutePath(value: string): string {
  if (typeof value !== 'string' || value === '' || value !== value.trim() || value.includes('\0') || !isAbsolute(value)) {
    throw fixedFileEffectError()
  }
  const normalized = resolve(value)
  if (normalized !== value) throw fixedFileEffectError()
  return normalized
}

function isSafeLeafName(value: string): boolean {
  return value !== '' && value !== '.' && value !== '..' && !value.includes('/') && !value.includes('\\') &&
    Buffer.byteLength(value, 'utf8') <= 240
}

function sha256(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function isNodeErrorCode(error: unknown, code: string): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === code
}

function fixedFileEffectError(): Error {
  return new Error(FILE_EFFECT_ERROR)
}
