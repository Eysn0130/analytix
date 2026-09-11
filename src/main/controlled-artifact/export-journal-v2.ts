import { createHash, randomBytes } from 'node:crypto'
import { constants, type BigIntStats } from 'node:fs'
import {
  chmod,
  lstat,
  mkdir,
  open,
  readdir,
  realpath,
  unlink,
  type FileHandle
} from 'node:fs/promises'
import { isAbsolute, join, resolve } from 'node:path'
import { parseStrictJsonObject } from './strict-json'

const JOURNAL_ERROR = 'controlled_artifact_export_journal_v2_unavailable'
const JOURNAL_PURPOSE = 'analytix.controlled-artifact-export-cleanup-journal/v2'
const JOURNAL_ID_PATTERN = /^[A-Za-z0-9_-]{22}$/u
const JOURNAL_FILE_PATTERN = /^\.export-v2-([A-Za-z0-9_-]{22})-([0-3])-(created|staged|link-intent|target-linked)\.json$/u
const STAGE_FILE_PATTERN = /^\.stage-v2-[A-Za-z0-9_-]{22}\.tmp$/u
const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const UNSIGNED_INTEGER_PATTERN = /^(?:0|[1-9][0-9]*)$/u
const NO_FOLLOW = process.platform === 'win32' ? 0 : constants.O_NOFOLLOW
const DIRECTORY = process.platform === 'win32' ? 0 : constants.O_DIRECTORY
const MAX_RECORD_BYTES = 16 << 10
const MAX_RECORDS = 1024
const MAX_AGGREGATE_BYTES = 8 << 20
const VERIFY_BUFFER_BYTES = 64 << 10

const JOURNAL_STATES = ['created', 'staged', 'link-intent', 'target-linked'] as const
type JournalStateV2 = typeof JOURNAL_STATES[number]

export type ControlledArtifactExportJournalDescriptorV2 = Readonly<{
  parentPath: string
  parentDev: bigint
  parentIno: bigint
  targetName: string
  stageName: string
  stageDev: bigint
  stageIno: bigint
  targetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
}>

type JournalRecordV2 = {
  schemaVersion: number
  purpose: string
  journalId: string
  state: JournalStateV2
  parentPath: string
  parentDev: string
  parentIno: string
  targetName: string
  stageName: string
  stageDev: string
  stageIno: string
  targetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
}

type FileIdentityV2 = { dev: bigint; ino: bigint }
type JournalFileV2 = { name: string; path: string; identity: FileIdentityV2; record: JournalRecordV2 }
type RecoveryGroupV2 = { record: JournalRecordV2; files: JournalFileV2[]; maxState: number }
type RecoveryPlanV2 = {
  group: RecoveryGroupV2
  stagePath: string | null
  stageIdentity: FileIdentityV2 | null
  parentIdentity: FileIdentityV2
}

export class ControlledArtifactExportJournalV2 {
  private closed = false
  private compromised = false
  private readonly active = new Set<string>()

  private constructor(
    private readonly rootPath: string,
    private readonly rootHandle: FileHandle,
    private readonly rootIdentity: FileIdentityV2
  ) {}

  static async open(rootPath: string): Promise<ControlledArtifactExportJournalV2> {
    let rootHandle: FileHandle | null = null
    try {
      const exactRoot = exactAbsolutePath(rootPath)
      await mkdir(exactRoot, { recursive: true, mode: 0o700 })
      await chmod(exactRoot, 0o700)
      const root = await openPrivateDirectory(exactRoot)
      rootHandle = root.handle
      const journal = new ControlledArtifactExportJournalV2(root.path, root.handle, identityOf(root))
      await journal.recover()
      return journal
    } catch {
      await rootHandle?.close().catch(() => undefined)
      throw fixedJournalError()
    }
  }

  async begin(
    descriptor: ControlledArtifactExportJournalDescriptorV2
  ): Promise<ControlledArtifactExportJournalAttemptV2> {
    if (this.closed || this.compromised || !validDescriptor(descriptor)) throw fixedJournalError()
    const journalId = randomBytes(16).toString('base64url')
    if (!JOURNAL_ID_PATTERN.test(journalId) || this.active.has(journalId)) throw fixedJournalError()
    const record = recordFromDescriptor(journalId, 'created', descriptor)
    this.active.add(journalId)
    try {
      const first = await this.writeState(record)
      return new ControlledArtifactExportJournalAttemptV2(this, record, [first])
    } catch {
      this.active.delete(journalId)
      this.compromised = true
      throw fixedJournalError()
    }
  }

  async close(): Promise<void> {
    if (this.closed) return
    this.closed = true
    let failed = this.compromised || this.active.size !== 0
    try {
      await this.revalidateRoot()
      if ((await readdir(this.rootPath)).length !== 0) failed = true
    } catch {
      failed = true
    }
    await this.rootHandle.close().catch(() => { failed = true })
    if (failed) throw fixedJournalError()
  }

  async appendState(record: JournalRecordV2, state: JournalStateV2): Promise<JournalFileV2> {
    if (this.closed || this.compromised || !this.active.has(record.journalId)) throw fixedJournalError()
    const current = JOURNAL_STATES.indexOf(record.state)
    const next = JOURNAL_STATES.indexOf(state)
    if (current < 0 || next !== current + 1) throw fixedJournalError()
    try {
      return await this.writeState({ ...record, state })
    } catch {
      this.compromised = true
      throw fixedJournalError()
    }
  }

  async complete(journalId: string, files: readonly JournalFileV2[]): Promise<void> {
    if (this.closed || this.compromised || !this.active.has(journalId) || files.length === 0) {
      throw fixedJournalError()
    }
    try {
      await this.revalidateRoot()
      for (const file of files) {
        await requireExactPrivateFile(file.path, file.identity)
        await unlink(file.path)
      }
      await this.rootHandle.sync()
      this.active.delete(journalId)
    } catch {
      this.compromised = true
      throw fixedJournalError()
    }
  }

  markCompromised(): void {
    this.compromised = true
  }

  private async writeState(record: JournalRecordV2): Promise<JournalFileV2> {
    await this.revalidateRoot()
    const name = journalFileName(record.journalId, record.state)
    const path = join(this.rootPath, name)
    const body = Buffer.from(JSON.stringify(record), 'utf8')
    let file: FileHandle | null = null
    try {
      if (body.length === 0 || body.length > MAX_RECORD_BYTES) throw fixedJournalError()
      file = await open(path, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | NO_FOLLOW, 0o600)
      const before = await file.stat({ bigint: true })
      requirePrivateRegularFile(before)
      await writeAll(file, body)
      await file.sync()
      const after = await file.stat({ bigint: true })
      requirePrivateRegularFile(after)
      requireSameIdentity(before, after)
      if (after.size !== BigInt(body.length)) throw fixedJournalError()
      await file.close()
      file = null
      await this.rootHandle.sync()
      await requireExactPrivateFile(path, identityOf(after))
      return { name, path, identity: identityOf(after), record }
    } finally {
      body.fill(0)
      await file?.close().catch(() => undefined)
    }
  }

  private async recover(): Promise<void> {
    const names = await readdir(this.rootPath)
    if (names.length > MAX_RECORDS) throw fixedJournalError()
    const groups = new Map<string, JournalFileV2[]>()
    let aggregateBytes = 0
    for (const name of names.sort()) {
      const match = JOURNAL_FILE_PATTERN.exec(name)
      if (!match) throw fixedJournalError()
      const file = await readJournalFile(this.rootPath, name)
      aggregateBytes += Number((await lstat(file.path, { bigint: true })).size)
      if (aggregateBytes > MAX_AGGREGATE_BYTES ||
        file.record.journalId !== match[1] || JOURNAL_STATES.indexOf(file.record.state) !== Number(match[2]) ||
        stateFileLabel(file.record.state) !== match[3]) {
        throw fixedJournalError()
      }
      const group = groups.get(file.record.journalId) ?? []
      group.push(file)
      groups.set(file.record.journalId, group)
    }
    const plans: RecoveryPlanV2[] = []
    for (const files of groups.values()) plans.push(await planRecovery(files))
    for (const plan of plans) await this.applyRecovery(plan)
  }

  private async applyRecovery(plan: RecoveryPlanV2): Promise<void> {
    const parent = await openSafeParent(plan.group.record.parentPath)
    try {
      requireSameIdentity(parent, plan.parentIdentity)
      if (plan.stagePath && plan.stageIdentity) {
        await requireExactPrivateFile(plan.stagePath, plan.stageIdentity, [1n, 2n])
        await unlink(plan.stagePath)
        await parent.handle.sync()
      }
      await this.revalidateRoot()
      for (const file of plan.group.files.sort((left, right) =>
        JOURNAL_STATES.indexOf(left.record.state) - JOURNAL_STATES.indexOf(right.record.state))) {
        await requireExactPrivateFile(file.path, file.identity)
        await unlink(file.path)
      }
      await this.rootHandle.sync()
    } finally {
      await parent.handle.close()
    }
  }

  private async revalidateRoot(): Promise<void> {
    const current = await openPrivateDirectory(this.rootPath)
    try {
      requireSameIdentity(current, this.rootIdentity)
    } finally {
      await current.handle.close()
    }
  }
}

export class ControlledArtifactExportJournalAttemptV2 {
  private terminal = false

  constructor(
    private readonly journal: ControlledArtifactExportJournalV2,
    private record: JournalRecordV2,
    private readonly files: JournalFileV2[]
  ) {}

  async markStaged(): Promise<void> { await this.advance('staged') }
  async markLinkIntent(): Promise<void> { await this.advance('link-intent') }
  async markTargetLinked(): Promise<void> { await this.advance('target-linked') }

  async complete(): Promise<void> {
    if (this.terminal) return
    try {
      await this.journal.complete(this.record.journalId, this.files)
      this.terminal = true
    } catch {
      this.journal.markCompromised()
      throw fixedJournalError()
    }
  }

  failClosed(): void {
    this.journal.markCompromised()
  }

  private async advance(state: JournalStateV2): Promise<void> {
    if (this.terminal) throw fixedJournalError()
    const file = await this.journal.appendState(this.record, state)
    this.record = file.record
    this.files.push(file)
  }
}

async function planRecovery(files: JournalFileV2[]): Promise<RecoveryPlanV2> {
  if (files.length === 0 || files.length > JOURNAL_STATES.length) throw fixedJournalError()
  const ordered = [...files].sort((left, right) =>
    JOURNAL_STATES.indexOf(left.record.state) - JOURNAL_STATES.indexOf(right.record.state))
  const base = { ...ordered[0].record, state: 'created' as JournalStateV2 }
  const seen = new Set<number>()
  for (const file of ordered) {
    const state = JOURNAL_STATES.indexOf(file.record.state)
    if (state < 0 || seen.has(state) || JSON.stringify({ ...file.record, state: 'created' }) !== JSON.stringify(base)) {
      throw fixedJournalError()
    }
    seen.add(state)
  }
  const maxState = Math.max(...seen)
  const record = ordered[ordered.length - 1].record
  const parent = await openSafeParent(record.parentPath)
  try {
    const expectedParent = { dev: BigInt(record.parentDev), ino: BigInt(record.parentIno) }
    requireSameIdentity(parent, expectedParent)
    const stagePath = join(parent.path, record.stageName)
    const targetPath = join(parent.path, record.targetName)
    const expectedStage = { dev: BigInt(record.stageDev), ino: BigInt(record.stageIno) }
    const stage = await optionalPrivateFile(stagePath, [1n, 2n])
    const target = await optionalPrivateFile(targetPath, [1n, 2n])
    if (stage) requireSameIdentity(stage, expectedStage)
    if (target) {
      requireSameIdentity(target, expectedStage)
      await verifyExactFile(targetPath, expectedStage, record.artifactByteLength, record.artifactSha256, [1n, 2n])
    }
    if (stage && target && !sameIdentity(stage, target)) throw fixedJournalError()
    if (maxState === JOURNAL_STATES.indexOf('target-linked') && !target) throw fixedJournalError()
    return {
      group: { record, files: ordered, maxState },
      stagePath: stage ? stagePath : null,
      stageIdentity: stage ? expectedStage : null,
      parentIdentity: expectedParent
    }
  } finally {
    await parent.handle.close()
  }
}

async function readJournalFile(root: string, name: string): Promise<JournalFileV2> {
  const path = join(root, name)
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  let body = Buffer.alloc(0)
  try {
    const before = await file.stat({ bigint: true })
    requirePrivateRegularFile(before)
    if (before.size <= 0n || before.size > BigInt(MAX_RECORD_BYTES)) throw fixedJournalError()
    body = await file.readFile()
    if (body.length !== Number(before.size)) throw fixedJournalError()
    const value = parseStrictJsonObject(body, { maxBytes: MAX_RECORD_BYTES, maxDepth: 2, maxTokens: 64 })
    const record = validateJournalRecord(value)
    if (!body.equals(Buffer.from(JSON.stringify(record), 'utf8'))) throw fixedJournalError()
    const after = await file.stat({ bigint: true })
    requirePrivateRegularFile(after)
    requireSameIdentity(before, after)
    return { name, path, identity: identityOf(after), record }
  } finally {
    body.fill(0)
    await file.close()
  }
}

function validateJournalRecord(value: Record<string, unknown>): JournalRecordV2 {
  const keys = [
    'schemaVersion', 'purpose', 'journalId', 'state', 'parentPath', 'parentDev', 'parentIno',
    'targetName', 'stageName', 'stageDev', 'stageIno', 'targetIdentityDigest',
    'artifactSha256', 'artifactByteLength'
  ]
  if (!hasExactKeys(value, keys)) throw fixedJournalError()
  const record = value as unknown as JournalRecordV2
  if (record.schemaVersion !== 2 || record.purpose !== JOURNAL_PURPOSE ||
    !JOURNAL_ID_PATTERN.test(record.journalId) || !JOURNAL_STATES.includes(record.state) ||
    exactAbsolutePath(record.parentPath) !== record.parentPath ||
    !validLeaf(record.targetName) || !STAGE_FILE_PATTERN.test(record.stageName) ||
    !UNSIGNED_INTEGER_PATTERN.test(record.parentDev) || !UNSIGNED_INTEGER_PATTERN.test(record.parentIno) ||
    !UNSIGNED_INTEGER_PATTERN.test(record.stageDev) || !UNSIGNED_INTEGER_PATTERN.test(record.stageIno) ||
    !SHA256_PATTERN.test(record.targetIdentityDigest) || !SHA256_PATTERN.test(record.artifactSha256) ||
    !Number.isSafeInteger(record.artifactByteLength) || record.artifactByteLength <= 0 ||
    record.artifactByteLength > 8 << 20) {
    throw fixedJournalError()
  }
  return recordFromDescriptor(record.journalId, record.state, {
    parentPath: record.parentPath,
    parentDev: BigInt(record.parentDev),
    parentIno: BigInt(record.parentIno),
    targetName: record.targetName,
    stageName: record.stageName,
    stageDev: BigInt(record.stageDev),
    stageIno: BigInt(record.stageIno),
    targetIdentityDigest: record.targetIdentityDigest,
    artifactSha256: record.artifactSha256,
    artifactByteLength: record.artifactByteLength
  })
}

function recordFromDescriptor(
  journalId: string,
  state: JournalStateV2,
  descriptor: ControlledArtifactExportJournalDescriptorV2
): JournalRecordV2 {
  return {
    schemaVersion: 2,
    purpose: JOURNAL_PURPOSE,
    journalId,
    state,
    parentPath: descriptor.parentPath,
    parentDev: String(descriptor.parentDev),
    parentIno: String(descriptor.parentIno),
    targetName: descriptor.targetName,
    stageName: descriptor.stageName,
    stageDev: String(descriptor.stageDev),
    stageIno: String(descriptor.stageIno),
    targetIdentityDigest: descriptor.targetIdentityDigest,
    artifactSha256: descriptor.artifactSha256,
    artifactByteLength: descriptor.artifactByteLength
  }
}

function validDescriptor(value: ControlledArtifactExportJournalDescriptorV2): boolean {
  return Boolean(value) && exactAbsolutePath(value.parentPath) === value.parentPath &&
    typeof value.parentDev === 'bigint' && value.parentDev >= 0n &&
    typeof value.parentIno === 'bigint' && value.parentIno >= 0n &&
    validLeaf(value.targetName) && STAGE_FILE_PATTERN.test(value.stageName) &&
    typeof value.stageDev === 'bigint' && value.stageDev >= 0n &&
    typeof value.stageIno === 'bigint' && value.stageIno >= 0n &&
    SHA256_PATTERN.test(value.targetIdentityDigest) && SHA256_PATTERN.test(value.artifactSha256) &&
    Number.isSafeInteger(value.artifactByteLength) && value.artifactByteLength > 0 &&
    value.artifactByteLength <= 8 << 20
}

async function openPrivateDirectory(path: string): Promise<{ path: string; handle: FileHandle } & FileIdentityV2> {
  const exact = exactAbsolutePath(path)
  if (await realpath(exact) !== exact) throw fixedJournalError()
  const before = await lstat(exact, { bigint: true })
  requireDirectory(before, true)
  const handle = await open(exact, constants.O_RDONLY | DIRECTORY | NO_FOLLOW)
  try {
    const opened = await handle.stat({ bigint: true })
    requireDirectory(opened, true)
    requireSameIdentity(before, opened)
    return { path: exact, handle, ...identityOf(opened) }
  } catch {
    await handle.close().catch(() => undefined)
    throw fixedJournalError()
  }
}

async function openSafeParent(path: string): Promise<{ path: string; handle: FileHandle } & FileIdentityV2> {
  const exact = exactAbsolutePath(path)
  if (await realpath(exact) !== exact) throw fixedJournalError()
  const before = await lstat(exact, { bigint: true })
  requireDirectory(before, false)
  const handle = await open(exact, constants.O_RDONLY | DIRECTORY | NO_FOLLOW)
  try {
    const opened = await handle.stat({ bigint: true })
    requireDirectory(opened, false)
    requireSameIdentity(before, opened)
    return { path: exact, handle, ...identityOf(opened) }
  } catch {
    await handle.close().catch(() => undefined)
    throw fixedJournalError()
  }
}

async function optionalPrivateFile(path: string, links: readonly bigint[]): Promise<BigIntStats | null> {
  try {
    const stats = await lstat(path, { bigint: true })
    requirePrivateRegularFile(stats, links)
    return stats
  } catch (error) {
    if (isNodeErrorCode(error, 'ENOENT')) return null
    throw fixedJournalError()
  }
}

async function requireExactPrivateFile(
  path: string,
  expected: FileIdentityV2,
  links: readonly bigint[] = [1n]
): Promise<void> {
  const before = await lstat(path, { bigint: true })
  requirePrivateRegularFile(before, links)
  requireSameIdentity(before, expected)
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  try {
    const opened = await file.stat({ bigint: true })
    requirePrivateRegularFile(opened, links)
    requireSameIdentity(opened, expected)
    requireSameIdentity(before, opened)
  } finally {
    await file.close()
  }
}

async function verifyExactFile(
  path: string,
  expected: FileIdentityV2,
  length: number,
  digest: string,
  links: readonly bigint[]
): Promise<void> {
  const file = await open(path, constants.O_RDONLY | NO_FOLLOW)
  const buffer = Buffer.allocUnsafe(VERIFY_BUFFER_BYTES)
  try {
    const stats = await file.stat({ bigint: true })
    requirePrivateRegularFile(stats, links)
    requireSameIdentity(stats, expected)
    if (stats.size !== BigInt(length)) throw fixedJournalError()
    const hash = createHash('sha256')
    let offset = 0
    while (offset < length) {
      const current = Math.min(buffer.length, length - offset)
      const read = await file.read(buffer, 0, current, offset)
      if (read.bytesRead !== current) throw fixedJournalError()
      hash.update(buffer.subarray(0, current))
      buffer.fill(0, 0, current)
      offset += current
    }
    if (hash.digest('hex') !== digest) throw fixedJournalError()
  } finally {
    buffer.fill(0)
    await file.close()
  }
}

async function writeAll(file: FileHandle, body: Buffer): Promise<void> {
  let offset = 0
  while (offset < body.length) {
    const written = await file.write(body, offset, body.length - offset, null)
    if (written.bytesWritten <= 0) throw fixedJournalError()
    offset += written.bytesWritten
  }
}

function requireDirectory(stats: BigIntStats, privateOnly: boolean): void {
  if (!stats.isDirectory() || stats.isSymbolicLink() || stats.nlink < 1n || !ownedByCurrentUser(stats) ||
    (privateOnly ? (stats.mode & 0o077n) !== 0n : (stats.mode & 0o022n) !== 0n)) {
    throw fixedJournalError()
  }
}

function requirePrivateRegularFile(stats: BigIntStats, links: readonly bigint[] = [1n]): void {
  if (!stats.isFile() || stats.isSymbolicLink() || !links.includes(stats.nlink) ||
    !ownedByCurrentUser(stats) || (stats.mode & 0o077n) !== 0n) {
    throw fixedJournalError()
  }
}

function ownedByCurrentUser(stats: BigIntStats): boolean {
  return typeof process.getuid !== 'function' || stats.uid === BigInt(process.getuid())
}

function journalFileName(journalId: string, state: JournalStateV2): string {
  const index = JOURNAL_STATES.indexOf(state)
  if (!JOURNAL_ID_PATTERN.test(journalId) || index < 0) throw fixedJournalError()
  return `.export-v2-${journalId}-${index}-${stateFileLabel(state)}.json`
}

function stateFileLabel(state: JournalStateV2): string {
  return state === 'link-intent' ? 'link-intent' : state === 'target-linked' ? 'target-linked' : state
}

function validLeaf(value: unknown): value is string {
  return typeof value === 'string' && value !== '' && value !== '.' && value !== '..' &&
    !value.includes('/') && !value.includes('\\') && Buffer.byteLength(value, 'utf8') <= 240
}

function exactAbsolutePath(value: string): string {
  if (typeof value !== 'string' || value === '' || value !== value.trim() || value.includes('\0') ||
    !isAbsolute(value) || resolve(value) !== value || Buffer.byteLength(value, 'utf8') > 4096) {
    throw fixedJournalError()
  }
  return value
}

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const actual = Object.keys(value).sort()
  const wanted = [...expected].sort()
  return actual.length === wanted.length && actual.every((key, index) => key === wanted[index])
}

function identityOf(value: FileIdentityV2): FileIdentityV2 {
  return { dev: value.dev, ino: value.ino }
}

function sameIdentity(left: FileIdentityV2, right: FileIdentityV2): boolean {
  return left.dev === right.dev && left.ino === right.ino
}

function requireSameIdentity(left: FileIdentityV2, right: FileIdentityV2): void {
  if (!sameIdentity(left, right)) throw fixedJournalError()
}

function isNodeErrorCode(error: unknown, code: string): boolean {
  return error instanceof Error && 'code' in error && (error as NodeJS.ErrnoException).code === code
}

function fixedJournalError(): Error {
  return new Error(JOURNAL_ERROR)
}
