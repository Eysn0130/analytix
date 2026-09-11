import { linkSync, readdirSync, rmdirSync, unlinkSync } from 'node:fs'
import { randomUUID } from 'node:crypto'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import {
  durableWriteExclusive,
  ensureNewDirectory,
  fsyncDirectoryStrict,
  inspectPathStrict,
  LegacyMigrationBlockedError,
  migrationBlocked,
  readJsonRegularFileStrict,
  readRegularFileStrict,
  sha256Bytes,
  stableJson,
  unlinkExactRegularFile,
  type DirectorySnapshotV2,
  type HostDirectoryActivationReceiptV2,
  type HostDirectoryActivationRequestV2
} from './legacy-data-migration-platform'

export const LEGACY_MIGRATION_SCHEMA_VERSION = 2 as const
export const LEGACY_MIGRATION_JOURNAL_DIR_NAME = '.analytix-legacy-migration-v2'
export const LEGACY_MIGRATION_LEASE_FILE_NAME = '.analytix-legacy-migration-v2.lock'

export type LegacyMigrationOwnerId = 'user-data' | `home-data-${number}`

export type LegacyMigrationPlannedOwnerV2 = {
  id: LegacyMigrationOwnerId
  sourcePath: string
  targetPath: string
  stagePath: string
  sourceDev: string
  sourceIno: string
  sourceParentDev: string
  sourceParentIno: string
  targetParentDev: string
  targetParentIno: string
  excludedSourceRootEntries: string[]
  sourceSnapshot: DirectorySnapshotV2
}

export type LegacyMigrationHomeMappingDecisionV2 = {
  index: number
  sourcePath: string
  targetPath: string
  sourceState: 'owned-directory' | 'absent'
  targetState: 'missing' | 'existing-directory'
  caseSensitivity: 'sensitive' | 'insensitive'
  targetDev: string
  targetIno: string
}

export type LegacyMigrationPlanV2 = {
  schemaVersion: 2
  transactionId: string
  createdAt: string
  userDataPath: string
  journalParentDev: string
  journalParentIno: string
  owners: LegacyMigrationPlannedOwnerV2[]
  homeMappingDecisions: LegacyMigrationHomeMappingDecisionV2[]
  homeSemanticReceiptSha256: string
  hostFilesystemScopeSha256: string
  hostFilesystemSemanticReceiptSha256: string
}

export type LegacyMigrationStagedOwnerV2 = {
  id: LegacyMigrationOwnerId
  stageSnapshot: DirectorySnapshotV2
  stageDev: string
  stageIno: string
}

export type LegacyMigrationJournalState =
  | 'planned'
  | 'staged'
  | 'activation-intent'
  | 'owner-visible'
  | 'active'

export type LegacyMigrationJournalRecordV2 = {
  schemaVersion: 2
  transactionId: string
  sequence: number
  state: LegacyMigrationJournalState
  previousRecordSha256: string
  payload: unknown
}

export type LoadedLegacyMigrationJournalV2 = {
  rootPath: string
  plan: LegacyMigrationPlanV2
  records: Array<{
    path: string
    sha256: string
    record: LegacyMigrationJournalRecordV2
  }>
}

type LeasePayloadV2 = {
  schemaVersion: 2
  purpose: 'legacy-data-migration-quiescence'
  pid: number
  token: string
  createdAt: string
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function assertExactKeys(value: Record<string, unknown>, keys: readonly string[]): void {
  const actual = Object.keys(value).sort()
  const expected = [...keys].sort()
  if (actual.length !== expected.length || actual.some((key, index) => key !== expected[index])) {
    migrationBlocked('invalid_journal')
  }
}

function isSha256(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
}

function isTransactionId(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9-]{16,64}$/i.test(value)
}

function assertSnapshot(value: unknown): asserts value is DirectorySnapshotV2 {
  if (!isRecord(value)) migrationBlocked('invalid_journal')
  assertExactKeys(value, [
    'schemaVersion',
    'manifestSha256',
    'rootMode',
    'files',
    'directories',
    'totalBytes',
    'entries'
  ])
  if (
    value.schemaVersion !== 2 ||
    !isSha256(value.manifestSha256) ||
    !Number.isSafeInteger(value.rootMode) ||
    (value.rootMode as number) < 0 ||
    (value.rootMode as number) > 0o777 ||
    !Number.isSafeInteger(value.files) ||
    !Number.isSafeInteger(value.directories) ||
    !Number.isSafeInteger(value.totalBytes) ||
    !Array.isArray(value.entries)
  ) {
    migrationBlocked('invalid_journal')
  }
  const paths = new Set<string>()
  let fileCount = 0
  let directoryCount = 0
  let totalBytes = 0
  for (const entry of value.entries) {
    if (!isRecord(entry)) migrationBlocked('invalid_journal')
    if (entry.kind === 'directory') {
      assertExactKeys(entry, ['kind', 'relativePath', 'mode'])
      if (
        !isSafeSnapshotRelativePath(entry.relativePath) ||
        !Number.isSafeInteger(entry.mode) ||
        (entry.mode as number) < 0 ||
        (entry.mode as number) > 0o777
      ) {
        migrationBlocked('invalid_journal')
      }
      directoryCount += 1
      const folded = (entry.relativePath as string).normalize('NFC').toLocaleLowerCase('en-US')
      if (paths.has(folded)) migrationBlocked('invalid_journal')
      paths.add(folded)
      continue
    }
    if (entry.kind === 'file') {
      assertExactKeys(entry, ['kind', 'relativePath', 'mode', 'size', 'sha256'])
      if (
        !isSafeSnapshotRelativePath(entry.relativePath) ||
        !Number.isSafeInteger(entry.mode) ||
        (entry.mode as number) < 0 ||
        (entry.mode as number) > 0o777 ||
        !Number.isSafeInteger(entry.size) ||
        (entry.size as number) < 0 ||
        !isSha256(entry.sha256)
      ) {
        migrationBlocked('invalid_journal')
      }
      fileCount += 1
      totalBytes += entry.size as number
      const folded = (entry.relativePath as string).normalize('NFC').toLocaleLowerCase('en-US')
      if (paths.has(folded)) migrationBlocked('invalid_journal')
      paths.add(folded)
      continue
    }
    migrationBlocked('invalid_journal')
  }
  if (
    value.files !== fileCount ||
    value.directories !== directoryCount ||
    value.totalBytes !== totalBytes
  ) {
    migrationBlocked('invalid_journal')
  }
  if (
    sha256Bytes(stableJson({
      entries: value.entries,
      rootMode: value.rootMode
    })) !== value.manifestSha256
  ) {
    migrationBlocked('invalid_journal')
  }
}

function isSafeSnapshotRelativePath(value: unknown): value is string {
  if (
    typeof value !== 'string' ||
    !value ||
    value.includes('\\') ||
    value.includes('\0') ||
    isAbsolute(value)
  ) {
    return false
  }
  const segments = value.split('/')
  return segments.every((segment) => segment !== '' && segment !== '.' && segment !== '..')
}

function isCleanAbsolutePath(value: unknown): value is string {
  return typeof value === 'string' && isAbsolute(value) && resolve(value) === value
}

function assertOwnerId(value: unknown): asserts value is LegacyMigrationOwnerId {
  if (typeof value !== 'string' || (value !== 'user-data' && !/^home-data-\d+$/.test(value))) {
    migrationBlocked('invalid_journal')
  }
}

function assertPlan(value: unknown): asserts value is LegacyMigrationPlanV2 {
  if (!isRecord(value)) migrationBlocked('invalid_journal')
  assertExactKeys(value, [
    'schemaVersion',
    'transactionId',
    'createdAt',
    'userDataPath',
    'journalParentDev',
    'journalParentIno',
    'owners',
    'homeMappingDecisions',
    'homeSemanticReceiptSha256',
    'hostFilesystemScopeSha256',
    'hostFilesystemSemanticReceiptSha256'
  ])
  if (
    value.schemaVersion !== 2 ||
    !isTransactionId(value.transactionId) ||
    typeof value.createdAt !== 'string' ||
    !Number.isFinite(Date.parse(value.createdAt)) ||
    !isCleanAbsolutePath(value.userDataPath) ||
    typeof value.journalParentDev !== 'string' ||
    !value.journalParentDev ||
    typeof value.journalParentIno !== 'string' ||
    !value.journalParentIno ||
    !Array.isArray(value.owners) ||
    value.owners.length === 0 ||
    !Array.isArray(value.homeMappingDecisions) ||
    (value.homeSemanticReceiptSha256 !== '' && !isSha256(value.homeSemanticReceiptSha256)) ||
    !isSha256(value.hostFilesystemScopeSha256) ||
    !isSha256(value.hostFilesystemSemanticReceiptSha256)
  ) {
    migrationBlocked('invalid_journal')
  }

  const ownerIds = new Set<string>()
  const paths = new Set<string>()
  for (const owner of value.owners) {
    if (!isRecord(owner)) migrationBlocked('invalid_journal')
    assertExactKeys(owner, [
      'id',
      'sourcePath',
      'targetPath',
      'stagePath',
      'sourceDev',
      'sourceIno',
      'sourceParentDev',
      'sourceParentIno',
      'targetParentDev',
      'targetParentIno',
      'excludedSourceRootEntries',
      'sourceSnapshot'
    ])
    assertOwnerId(owner.id)
    if (
      !isCleanAbsolutePath(owner.sourcePath) ||
      !isCleanAbsolutePath(owner.targetPath) ||
      !isCleanAbsolutePath(owner.stagePath) ||
      typeof owner.sourceDev !== 'string' ||
      !owner.sourceDev ||
      typeof owner.sourceIno !== 'string' ||
      !owner.sourceIno ||
      typeof owner.sourceParentDev !== 'string' ||
      !owner.sourceParentDev ||
      typeof owner.sourceParentIno !== 'string' ||
      !owner.sourceParentIno ||
      typeof owner.targetParentDev !== 'string' ||
      !owner.targetParentDev ||
      typeof owner.targetParentIno !== 'string' ||
      !owner.targetParentIno ||
      dirname(owner.stagePath) !== dirname(owner.targetPath) ||
      basename(owner.stagePath) !== `.analytix-migration-v2-${value.transactionId}-${owner.id}.stage` ||
      !Array.isArray(owner.excludedSourceRootEntries) ||
      owner.excludedSourceRootEntries.some((name) =>
        typeof name !== 'string' ||
        !name ||
        name.includes('/') ||
        name.includes('\\') ||
        name === '.' ||
        name === '..'
      ) ||
      new Set(owner.excludedSourceRootEntries).size !== owner.excludedSourceRootEntries.length
    ) {
      migrationBlocked('invalid_journal')
    }
    if (ownerIds.has(owner.id)) migrationBlocked('invalid_journal')
    ownerIds.add(owner.id)
    for (const path of [owner.sourcePath, owner.targetPath, owner.stagePath]) {
      const folded = path.normalize('NFC').toLocaleLowerCase('en-US')
      if (paths.has(folded)) migrationBlocked('invalid_journal')
      paths.add(folded)
    }
    assertSnapshot(owner.sourceSnapshot)
  }

  const decisionIndexes = new Set<number>()
  for (const decision of value.homeMappingDecisions) {
    if (!isRecord(decision)) migrationBlocked('invalid_journal')
    assertExactKeys(decision, [
      'index',
      'sourcePath',
      'targetPath',
      'sourceState',
      'targetState',
      'caseSensitivity',
      'targetDev',
      'targetIno'
    ])
    if (
      !Number.isSafeInteger(decision.index) ||
      (decision.index as number) < 0 ||
      decisionIndexes.has(decision.index as number) ||
      !isCleanAbsolutePath(decision.sourcePath) ||
      !isCleanAbsolutePath(decision.targetPath) ||
      !['owned-directory', 'absent'].includes(String(decision.sourceState)) ||
      !['missing', 'existing-directory'].includes(String(decision.targetState)) ||
      !['sensitive', 'insensitive'].includes(String(decision.caseSensitivity)) ||
      (decision.sourceState === 'owned-directory' && decision.targetState !== 'missing') ||
      typeof decision.targetDev !== 'string' ||
      typeof decision.targetIno !== 'string' ||
      (decision.targetState === 'missing'
        ? decision.targetDev !== '' || decision.targetIno !== ''
        : !decision.targetDev || !decision.targetIno)
    ) {
      migrationBlocked('invalid_journal')
    }
    decisionIndexes.add(decision.index as number)
  }
  const hasHomeOwner = value.owners.some((owner) =>
    isRecord(owner) && typeof owner.id === 'string' && owner.id.startsWith('home-data-')
  )
  if (hasHomeOwner !== isSha256(value.homeSemanticReceiptSha256)) {
    migrationBlocked('invalid_journal')
  }
}

function parseRecord(raw: string): LegacyMigrationJournalRecordV2 {
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (error) {
    migrationBlocked('invalid_journal', error)
  }
  if (!isRecord(parsed)) migrationBlocked('invalid_journal')
  assertExactKeys(parsed, [
    'schemaVersion',
    'transactionId',
    'sequence',
    'state',
    'previousRecordSha256',
    'payload'
  ])
  if (
    parsed.schemaVersion !== 2 ||
    !isTransactionId(parsed.transactionId) ||
    !Number.isSafeInteger(parsed.sequence) ||
    (parsed.sequence as number) < 0 ||
    !['planned', 'staged', 'activation-intent', 'owner-visible', 'active'].includes(
      String(parsed.state)
    ) ||
    (parsed.sequence === 0
      ? parsed.previousRecordSha256 !== ''
      : !isSha256(parsed.previousRecordSha256))
  ) {
    migrationBlocked('invalid_journal')
  }
  return parsed as LegacyMigrationJournalRecordV2
}

function canonicalRecordName(record: LegacyMigrationJournalRecordV2): string {
  return `${String(record.sequence).padStart(3, '0')}-${record.state}.json`
}

function pendingRecordName(record: LegacyMigrationJournalRecordV2): string {
  return `.pending-${String(record.sequence).padStart(3, '0')}-${record.state}-${record.transactionId}.json`
}

function readDirectoryNamesStrict(path: string): string[] {
  try {
    return readdirSync(path).sort((left, right) => left.localeCompare(right, 'en'))
  } catch (error) {
    migrationBlocked('io_error', error)
  }
}

function readRecordFile(path: string): {
  raw: string
  sha256: string
  record: LegacyMigrationJournalRecordV2
} {
  const file = readRegularFileStrict(path)
  if ((file.mode & 0o077) !== 0) migrationBlocked('invalid_journal')
  return { raw: file.raw, sha256: file.sha256, record: parseRecord(file.raw) }
}

function loadCanonicalRecords(rootPath: string): LoadedLegacyMigrationJournalV2['records'] {
  const names = readDirectoryNamesStrict(rootPath)
  const canonicalNames = names.filter((name) => /^\d{3}-(planned|staged|activation-intent|owner-visible|active)\.json$/.test(name))
  const records = canonicalNames.map((name) => {
    const path = join(rootPath, name)
    const loaded = readRecordFile(path)
    if (name !== canonicalRecordName(loaded.record)) migrationBlocked('invalid_journal')
    return { path, sha256: loaded.sha256, record: loaded.record }
  })
  records.sort((left, right) => left.record.sequence - right.record.sequence)
  return records
}

function assertActivationReceipt(input: {
  value: unknown
  transactionId: string
  plan: LegacyMigrationPlanV2
  owner: LegacyMigrationPlannedOwnerV2
  staged: LegacyMigrationStagedOwnerV2
}): asserts input is typeof input & { value: HostDirectoryActivationReceiptV2 } {
  const value = input.value
  if (!isRecord(value)) migrationBlocked('invalid_journal')
  assertExactKeys(value, [
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
  ])
  if (
    value.schemaVersion !== 2 ||
    !isSha256(value.requestSha256) ||
    ![
      'darwin-renameatx-np-rename-excl',
      'linux-renameat2-rename-noreplace',
      'windows-handle-move-fail-if-exists'
    ].includes(String(value.primitive)) ||
    value.stageDev !== input.staged.stageDev ||
    value.stageIno !== input.staged.stageIno ||
    value.targetDev !== input.staged.stageDev ||
    value.targetIno !== input.staged.stageIno ||
    value.targetParentDev !== input.owner.targetParentDev ||
    value.targetParentIno !== input.owner.targetParentIno ||
    value.targetParentDurablySynced !== true
  ) {
    migrationBlocked('invalid_journal')
  }
  const request: HostDirectoryActivationRequestV2 = {
    schemaVersion: 2,
    requiredPrimitive: value.primitive as HostDirectoryActivationReceiptV2['primitive'],
    hostScopeSha256: input.plan.hostFilesystemScopeSha256,
    hostSemanticReceiptSha256: input.plan.hostFilesystemSemanticReceiptSha256,
    transactionId: input.transactionId,
    ownerId: input.owner.id,
    stagePath: input.owner.stagePath,
    targetPath: input.owner.targetPath,
    stageDev: input.staged.stageDev,
    stageIno: input.staged.stageIno,
    targetParentDev: input.owner.targetParentDev,
    targetParentIno: input.owner.targetParentIno,
    expectedManifestSha256: input.staged.stageSnapshot.manifestSha256
  }
  if (value.requestSha256 !== sha256Bytes(stableJson(request))) {
    migrationBlocked('invalid_journal')
  }
}

function validateRecordChain(records: LoadedLegacyMigrationJournalV2['records']): LegacyMigrationPlanV2 {
  if (records.length === 0) migrationBlocked('invalid_journal')
  for (let index = 0; index < records.length; index += 1) {
    const current = records[index]
    if (current.record.sequence !== index) migrationBlocked('invalid_journal')
    if (index === 0) {
      if (current.record.state !== 'planned' || current.record.previousRecordSha256 !== '') {
        migrationBlocked('invalid_journal')
      }
      continue
    }
    const previous = records[index - 1]
    if (
      current.record.transactionId !== previous.record.transactionId ||
      current.record.previousRecordSha256 !== previous.sha256
    ) {
      migrationBlocked('invalid_journal')
    }
  }

  const first = records[0].record
  assertPlan(first.payload)
  if (first.payload.transactionId !== first.transactionId) migrationBlocked('invalid_journal')
  const plan = first.payload
  let expectedState: LegacyMigrationJournalState = 'staged'
  let visibleOwner = 0
  let stagedRecordSha256 = ''
  const stagedOwnerById = new Map<string, LegacyMigrationStagedOwnerV2>()
  let activeSeen = false
  for (const entry of records.slice(1)) {
    const { record } = entry
    if (activeSeen) migrationBlocked('invalid_journal')
    if (record.state !== expectedState) migrationBlocked('invalid_journal')
    if (record.state === 'staged') {
      if (!isRecord(record.payload)) migrationBlocked('invalid_journal')
      assertExactKeys(record.payload, ['owners', 'settingsRewritten'])
      if (!Array.isArray(record.payload.owners) || typeof record.payload.settingsRewritten !== 'boolean') {
        migrationBlocked('invalid_journal')
      }
      if (record.payload.owners.length !== plan.owners.length) migrationBlocked('invalid_journal')
      for (let index = 0; index < record.payload.owners.length; index += 1) {
        const staged = record.payload.owners[index]
        if (!isRecord(staged)) migrationBlocked('invalid_journal')
        assertExactKeys(staged, ['id', 'stageSnapshot', 'stageDev', 'stageIno'])
        if (
          staged.id !== plan.owners[index].id ||
          typeof staged.stageDev !== 'string' ||
          !staged.stageDev ||
          typeof staged.stageIno !== 'string' ||
          !staged.stageIno
        ) {
          migrationBlocked('invalid_journal')
        }
        assertSnapshot(staged.stageSnapshot)
        stagedOwnerById.set(staged.id as string, staged as LegacyMigrationStagedOwnerV2)
      }
      stagedRecordSha256 = entry.sha256
      expectedState = 'activation-intent'
      continue
    }
    if (record.state === 'activation-intent') {
      if (!isRecord(record.payload)) migrationBlocked('invalid_journal')
      assertExactKeys(record.payload, ['stagedRecordSha256'])
      if (
        !isSha256(record.payload.stagedRecordSha256) ||
        record.payload.stagedRecordSha256 !== stagedRecordSha256
      ) {
        migrationBlocked('invalid_journal')
      }
      expectedState = 'owner-visible'
      continue
    }
    if (record.state === 'owner-visible') {
      if (!isRecord(record.payload)) migrationBlocked('invalid_journal')
      assertExactKeys(record.payload, [
        'ownerId',
        'targetManifestSha256',
        'targetDev',
        'targetIno',
        'activationReceipt'
      ])
      const stagedOwner = stagedOwnerById.get(String(record.payload.ownerId))
      const plannedOwner = plan.owners[visibleOwner]
      if (
        record.payload.ownerId !== plannedOwner?.id ||
        !isSha256(record.payload.targetManifestSha256) ||
        record.payload.targetManifestSha256 !== stagedOwner?.stageSnapshot.manifestSha256 ||
        typeof record.payload.targetDev !== 'string' ||
        !record.payload.targetDev ||
        typeof record.payload.targetIno !== 'string' ||
        !record.payload.targetIno ||
        record.payload.targetDev !== stagedOwner?.stageDev ||
        record.payload.targetIno !== stagedOwner?.stageIno
      ) {
        migrationBlocked('invalid_journal')
      }
      assertActivationReceipt({
        value: record.payload.activationReceipt,
        transactionId: record.transactionId,
        plan,
        owner: plannedOwner,
        staged: stagedOwner
      })
      visibleOwner += 1
      expectedState = visibleOwner === plan.owners.length ? 'active' : 'owner-visible'
      continue
    }
    if (record.state === 'active') {
      if (!isRecord(record.payload)) migrationBlocked('invalid_journal')
      assertExactKeys(record.payload, ['activatedAt', 'planSha256', 'stagedRecordSha256'])
      if (
        typeof record.payload.activatedAt !== 'string' ||
        !Number.isFinite(Date.parse(record.payload.activatedAt)) ||
        !isSha256(record.payload.planSha256) ||
        !isSha256(record.payload.stagedRecordSha256) ||
        record.payload.planSha256 !== records[0].sha256 ||
        record.payload.stagedRecordSha256 !== stagedRecordSha256
      ) {
        migrationBlocked('invalid_journal')
      }
      activeSeen = true
    }
  }
  return plan
}

function reconcilePendingRecords(rootPath: string): 'ready' | 'abandoned-empty-root' {
  for (;;) {
    const names = readDirectoryNamesStrict(rootPath)
    const unknown = names.filter((name) =>
      !/^\d{3}-(planned|staged|activation-intent|owner-visible|active)\.json$/.test(name) &&
      !/^\.pending-\d{3}-(planned|staged|activation-intent|owner-visible|active)-[a-f0-9-]{16,64}\.json$/i.test(name)
    )
    if (unknown.length > 0) migrationBlocked('invalid_journal')
    const pendingName = names.find((name) => name.startsWith('.pending-'))
    if (!pendingName) return 'ready'

    const pendingPath = join(rootPath, pendingName)
    const pendingFile = readRegularFileStrict(pendingPath)
    if ((pendingFile.mode & 0o077) !== 0) migrationBlocked('invalid_journal')
    let pending: ReturnType<typeof readRecordFile>
    try {
      pending = {
        raw: pendingFile.raw,
        sha256: pendingFile.sha256,
        record: parseRecord(pendingFile.raw)
      }
    } catch (error) {
      if (!(error instanceof LegacyMigrationBlockedError) || error.code !== 'invalid_journal') {
        throw error
      }
      const match = /^\.pending-(\d{3})-(planned|staged|activation-intent|owner-visible|active)-([a-f0-9-]{16,64})\.json$/i
        .exec(pendingName)
      const existing = loadCanonicalRecords(rootPath)
      if (
        !match ||
        Number(match[1]) !== existing.length ||
        (existing.length === 0
          ? match[2] !== 'planned'
          : match[3] !== existing[0].record.transactionId)
      ) {
        migrationBlocked('invalid_journal')
      }
      if (existing.length > 0) validateRecordChain(existing)
      unlinkExactRegularFile(pendingPath, pendingFile.sha256)
      if (existing.length > 0) continue
      if (readDirectoryNamesStrict(rootPath).length !== 0) migrationBlocked('invalid_journal')
      try {
        rmdirSync(rootPath)
      } catch (removeError) {
        migrationBlocked('io_error', removeError)
      }
      fsyncDirectoryStrict(dirname(rootPath))
      return 'abandoned-empty-root'
    }
    if (pendingName !== pendingRecordName(pending.record)) migrationBlocked('invalid_journal')
    const canonicalPath = join(rootPath, canonicalRecordName(pending.record))
    const canonicalState = inspectPathStrict(canonicalPath)
    if (canonicalState.kind === 'file') {
      const canonical = readRecordFile(canonicalPath)
      if (canonical.sha256 !== pending.sha256) migrationBlocked('invalid_journal')
      unlinkExactRegularFile(pendingPath, pending.sha256)
      continue
    }
    if (canonicalState.kind !== 'missing') migrationBlocked('invalid_journal')

    const existing = loadCanonicalRecords(rootPath)
    if (
      pending.record.sequence !== existing.length ||
      (existing.length === 0
        ? pending.record.previousRecordSha256 !== '' || pending.record.state !== 'planned'
        : pending.record.previousRecordSha256 !== existing.at(-1)?.sha256)
    ) {
      migrationBlocked('invalid_journal')
    }
    if (existing.length > 0) validateRecordChain(existing)
    try {
      validateRecordChain([
        ...existing,
        {
          path: canonicalPath,
          sha256: pending.sha256,
          record: pending.record
        }
      ])
    } catch (error) {
      if (!(error instanceof LegacyMigrationBlockedError) || error.code !== 'invalid_journal') {
        throw error
      }
      // A pending file is not a committed record. If its filename and chain
      // frontier are exact but its state payload is semantically invalid,
      // discard only those unpublished bytes and resume from the authenticated
      // canonical frontier.
      unlinkExactRegularFile(pendingPath, pending.sha256)
      if (existing.length > 0) continue
      if (readDirectoryNamesStrict(rootPath).length !== 0) migrationBlocked('invalid_journal')
      try {
        rmdirSync(rootPath)
      } catch (removeError) {
        migrationBlocked('io_error', removeError)
      }
      fsyncDirectoryStrict(dirname(rootPath))
      return 'abandoned-empty-root'
    }
    try {
      linkSync(pendingPath, canonicalPath)
    } catch (error) {
      migrationBlocked('io_error', error)
    }
    fsyncDirectoryStrict(rootPath)
    const canonical = readRecordFile(canonicalPath)
    if (canonical.sha256 !== pending.sha256) migrationBlocked('invalid_journal')
    try {
      unlinkSync(pendingPath)
    } catch (error) {
      migrationBlocked('io_error', error)
    }
    fsyncDirectoryStrict(rootPath)
  }
}

export function legacyMigrationJournalRoot(userDataPath: string): string {
  return join(dirname(userDataPath), LEGACY_MIGRATION_JOURNAL_DIR_NAME)
}

export function loadLegacyMigrationJournal(
  userDataPath: string,
  options: { allowRecoveryMutation?: boolean } = {}
): LoadedLegacyMigrationJournalV2 | null {
  const rootPath = legacyMigrationJournalRoot(userDataPath)
  const state = inspectPathStrict(rootPath)
  if (state.kind === 'missing') return null
  if (state.kind === 'symlink') migrationBlocked('symlink_forbidden', { rootPath })
  if (state.kind !== 'directory') migrationBlocked('invalid_journal')
  if ((state.mode & 0o077) !== 0) migrationBlocked('invalid_journal')
  if (typeof process.getuid === 'function' && state.uid !== process.getuid()) {
    migrationBlocked('invalid_journal')
  }
  const names = readDirectoryNamesStrict(rootPath)
  if (
    options.allowRecoveryMutation === false &&
    (names.length === 0 || names.some((name) => name.startsWith('.pending-')))
  ) {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  if (names.length === 0) {
    // The only code path that can create this exact private empty root is a
    // crash after durable directory creation but before the first planned
    // record. No stage or target operation is reachable before that record is
    // published, so removing this initialization residue is deterministic.
    try {
      rmdirSync(rootPath)
    } catch (error) {
      migrationBlocked('io_error', error)
    }
    fsyncDirectoryStrict(dirname(rootPath))
    return null
  }
  if (reconcilePendingRecords(rootPath) === 'abandoned-empty-root') return null
  const records = loadCanonicalRecords(rootPath)
  const plan = validateRecordChain(records)
  if (plan.userDataPath !== userDataPath) migrationBlocked('invalid_journal')
  return { rootPath, plan, records }
}

export function createLegacyMigrationJournal(
  plan: LegacyMigrationPlanV2
): LoadedLegacyMigrationJournalV2 {
  assertPlan(plan)
  const rootPath = legacyMigrationJournalRoot(plan.userDataPath)
  ensureNewDirectory(rootPath, 0o700)
  const created = appendLegacyMigrationJournalRecord(
    { rootPath, plan, records: [] },
    'planned',
    plan
  )
  return created
}

export function appendLegacyMigrationJournalRecord(
  journal: LoadedLegacyMigrationJournalV2,
  state: LegacyMigrationJournalState,
  payload: unknown
): LoadedLegacyMigrationJournalV2 {
  const previous = journal.records.at(-1)
  const record: LegacyMigrationJournalRecordV2 = {
    schemaVersion: 2,
    transactionId: journal.plan.transactionId,
    sequence: journal.records.length,
    state,
    previousRecordSha256: previous?.sha256 ?? '',
    payload
  }
  const raw = `${stableJson(record)}\n`
  const finalPath = join(journal.rootPath, canonicalRecordName(record))
  const pendingPath = join(journal.rootPath, pendingRecordName(record))
  validateRecordChain([
    ...journal.records,
    {
      path: finalPath,
      sha256: sha256Bytes(raw),
      record
    }
  ])
  durableWriteExclusive(finalPath, raw, 0o600, pendingPath)
  const loaded = readRecordFile(finalPath)
  if (loaded.record.sequence !== record.sequence || loaded.record.state !== state) {
    migrationBlocked('invalid_journal')
  }
  const next = {
    rootPath: journal.rootPath,
    plan: journal.plan,
    records: [...journal.records, { path: finalPath, sha256: loaded.sha256, record: loaded.record }]
  }
  validateRecordChain(next.records)
  return next
}

function parseLease(value: unknown): LeasePayloadV2 {
  if (!isRecord(value)) migrationBlocked('invalid_lease')
  assertExactKeys(value, ['schemaVersion', 'purpose', 'pid', 'token', 'createdAt'])
  if (
    value.schemaVersion !== 2 ||
    value.purpose !== 'legacy-data-migration-quiescence' ||
    !Number.isSafeInteger(value.pid) ||
    (value.pid as number) <= 0 ||
    !isTransactionId(value.token) ||
    typeof value.createdAt !== 'string' ||
    !Number.isFinite(Date.parse(value.createdAt))
  ) {
    migrationBlocked('invalid_lease')
  }
  return value as LeasePayloadV2
}

function defaultProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    if (typeof error === 'object' && error !== null && 'code' in error) {
      if ((error as NodeJS.ErrnoException).code === 'ESRCH') return false
      if ((error as NodeJS.ErrnoException).code === 'EPERM') return true
    }
    migrationBlocked('invalid_lease', error)
  }
}

export type LegacyMigrationLease = {
  path: string
  token: string
  release: () => void
}

export function acquireLegacyMigrationLease(input: {
  userDataPath: string
  now?: () => Date
  token?: () => string
  isProcessAlive?: (pid: number) => boolean
}): LegacyMigrationLease {
  const parentPath = dirname(input.userDataPath)
  const path = join(parentPath, LEGACY_MIGRATION_LEASE_FILE_NAME)
  const isProcessAlive = input.isProcessAlive ?? defaultProcessAlive
  for (const name of readDirectoryNamesStrict(parentPath)) {
    const match = /^\.analytix-legacy-migration-v2\.lock\.pending-(\d+)-([a-f0-9-]{16,64})$/i.exec(name)
    if (!match) continue
    const pendingPath = join(parentPath, name)
    const pendingState = inspectPathStrict(pendingPath)
    if (pendingState.kind === 'symlink') migrationBlocked('symlink_forbidden')
    if (pendingState.kind !== 'file') migrationBlocked('invalid_lease')
    if (isProcessAlive(Number(match[1]))) migrationBlocked('concurrent_migration')
    unlinkExactRegularFile(pendingPath)
  }
  const existingState = inspectPathStrict(path)
  if (existingState.kind !== 'missing') {
    if (existingState.kind === 'symlink') migrationBlocked('symlink_forbidden', { path })
    if (existingState.kind !== 'file') migrationBlocked('invalid_lease')
    const existingFile = readRegularFileStrict(path)
    const existing = parseLease(readJsonRegularFileStrict(path))
    if (isProcessAlive(existing.pid)) migrationBlocked('concurrent_migration')
    unlinkExactRegularFile(path, existingFile.sha256)
  }

  const token = input.token?.() ?? randomUUID()
  const payload: LeasePayloadV2 = {
    schemaVersion: 2,
    purpose: 'legacy-data-migration-quiescence',
    pid: process.pid,
    token,
    createdAt: (input.now?.() ?? new Date()).toISOString()
  }
  const raw = `${stableJson(payload)}\n`
  const pendingPath = join(
    parentPath,
    `${LEGACY_MIGRATION_LEASE_FILE_NAME}.pending-${process.pid}-${token}`
  )
  let sha256: string
  try {
    sha256 = durableWriteExclusive(path, raw, 0o600, pendingPath).sha256
  } catch (error) {
    if (inspectPathStrict(path).kind === 'file') {
      if (inspectPathStrict(pendingPath).kind === 'file') {
        unlinkExactRegularFile(pendingPath, sha256Bytes(raw))
      }
      migrationBlocked('concurrent_migration', error)
    }
    throw error
  }
  const acquired = parseLease(readJsonRegularFileStrict(path))
  if (acquired.token !== token || acquired.pid !== process.pid) migrationBlocked('invalid_lease')
  let released = false
  return {
    path,
    token,
    release: () => {
      if (released) return
      const current = parseLease(readJsonRegularFileStrict(path))
      if (current.token !== token || current.pid !== process.pid) {
        migrationBlocked('invalid_lease')
      }
      unlinkExactRegularFile(path, sha256)
      released = true
    }
  }
}

export function journalLastState(
  journal: LoadedLegacyMigrationJournalV2
): LegacyMigrationJournalState {
  return journal.records.at(-1)?.record.state ?? 'planned'
}

export function journalRecord(
  journal: LoadedLegacyMigrationJournalV2,
  state: LegacyMigrationJournalState
): LoadedLegacyMigrationJournalV2['records'][number] | undefined {
  return journal.records.find((entry) => entry.record.state === state)
}
