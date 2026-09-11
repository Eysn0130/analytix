import { randomUUID } from 'node:crypto'
import { basename, dirname, isAbsolute, join, resolve } from 'node:path'
import type { AppSettingsV1 } from '../shared/app-settings'
import {
  acquireLegacyMigrationLease,
  appendLegacyMigrationJournalRecord,
  createLegacyMigrationJournal,
  journalLastState,
  journalRecord,
  loadLegacyMigrationJournal,
  type LegacyMigrationHomeMappingDecisionV2,
  type LegacyMigrationOwnerId,
  type LegacyMigrationPlanV2,
  type LegacyMigrationPlannedOwnerV2,
  type LegacyMigrationStagedOwnerV2,
  type LoadedLegacyMigrationJournalV2
} from './legacy-data-migration-journal'
import {
  activateStageDirectory,
  applySnapshotPermissions,
  assertCanonicalDirectoryPath,
  assertDirectorySnapshot,
  copySnapshotToNewStage,
  detectPathCaseSensitivity,
  durableReplaceRegularFile,
  inspectPathStrict,
  LegacyMigrationBlockedError,
  migrationBlocked,
  readRegularFileStrict,
  removeJournalOwnedStage,
  sha256Bytes,
  snapshotDirectoryStrict,
  stableJson,
  type DirectorySnapshotV2,
  type HostDirectoryActivationPrimitiveV2,
  type HostDirectoryActivationReceiptV2,
  type HostDirectoryActivationRequestV2,
  type StrictPathState
} from './legacy-data-migration-platform'

export { LegacyMigrationBlockedError } from './legacy-data-migration-platform'

/**
 * This migration deliberately preserves every legacy source directory.
 * Compatibility is provided by a gated copy and an activation receipt, never
 * by a symlink/junction or by letting old and new releases share writable data.
 */

export type MigrationLogger = (message: string, detail?: unknown) => void

export const LEGACY_USER_DATA_DIR_NAMES = ['Kun', 'DeepSeek GUI'] as const
export const ELECTRON_SINGLETON_ROOT_ENTRY_NAMES = [
  'SingletonCookie',
  'SingletonLock',
  'SingletonSocket'
] as const
export const LEGACY_HOME_DATA_ROOT = '.analytix'
export const NEW_HOME_DATA_ROOT = '.analytix'

export type HomeDataMigrationMapping = {
  legacySegments: readonly string[]
  nextSegments: readonly string[]
}

export const HOME_DATA_MIGRATION_MAPPINGS: readonly HomeDataMigrationMapping[] = [
  { legacySegments: [LEGACY_HOME_DATA_ROOT, 'analytix'], nextSegments: [NEW_HOME_DATA_ROOT, 'data'] }
] as const

const SETTINGS_FILE_NAMES = ['analytix-settings.json', 'kun-settings.json'] as const

export type LegacyMigrationStatus = 'ready-existing' | 'migrated' | 'recovered'

export type LegacyMigrationBarrierV2 = {
  schemaVersion: 2
  ready: true
  status: LegacyMigrationStatus
  transactionId: string
  receiptSha256: string
}

export type LegacyDataMigrationResult = {
  barrier: LegacyMigrationBarrierV2
  userDataPath: string
  migratedOwnerIds: LegacyMigrationOwnerId[]
  legacySourcesPreserved: true
  settingsRewritten: boolean
}

export type LegacyMigrationCheckpoint =
  | 'after-plan'
  | 'after-staged'
  | 'after-activation-intent'
  | `after-owner-visible:${LegacyMigrationOwnerId}`
  | 'after-active'

export type LegacyMigrationQuiescenceAuthorityV2 = {
  schemaVersion: 2
  kind: 'electron-legacy-user-data-singleton'
  legacyUserDataPath: string
  singletonArtifacts: ElectronSingletonArtifactV2[]
  assertHeld: () => boolean
  filesystem?: LegacyMigrationHostFilesystemAuthorityV2
  homeData?: {
    schemaVersion: 2
    kind: 'host-go-persistence-quiescence'
    sourcePaths: string[]
    semanticReceiptSha256: string
    assertHeld: () => boolean
  }
}

export type LegacyMigrationHostPathBindingV2 = {
  path: string
  kind: 'parent-directory' | 'source-directory'
  dev: string
  ino: string
}

/**
 * Issued only by the installer/native host. Its receipt means every path
 * component in `bindings` is held by an OS directory/file handle and every
 * migration read/write/delete is resolved handle-relative with no-follow
 * semantics for the lifetime of the transaction. Activation is atomic
 * no-replace; a retry after rename but before journal publication returns the
 * same request-bound durable receipt. TypeScript and Electron must never
 * self-mint this authority.
 */
export type LegacyMigrationHostFilesystemAuthorityV2 = {
  schemaVersion: 2
  kind: 'host-handle-relative-legacy-migration-transaction'
  scopeSha256: string
  semanticReceiptSha256: string
  activationPrimitive: HostDirectoryActivationPrimitiveV2
  bindings: LegacyMigrationHostPathBindingV2[]
  assertHeld: () => boolean
  activateOrRecoverDirectoryNoReplace: (
    request: HostDirectoryActivationRequestV2
  ) => HostDirectoryActivationReceiptV2
}

export type ElectronSingletonArtifactV2 = {
  name: (typeof ELECTRON_SINGLETON_ROOT_ENTRY_NAMES)[number]
  kind: 'file' | 'symlink' | 'other'
  dev: string
  ino: string
}

/**
 * Fault hooks are an explicit deterministic crash-cut seam. Production never
 * supplies them; tests use them to stop after a durable boundary and prove the
 * next invocation resumes the authenticated old transaction.
 */
export type LegacyMigrationTestHooks = {
  checkpoint?: (point: LegacyMigrationCheckpoint) => void
  beforeActivation?: (owner: LegacyMigrationPlannedOwnerV2) => void
  afterActivationBeforeReceipt?: (owner: LegacyMigrationPlannedOwnerV2) => void
  isProcessAlive?: (pid: number) => boolean
  now?: () => Date
  transactionId?: () => string
}

type StagedPayloadV2 = {
  owners: LegacyMigrationStagedOwnerV2[]
  settingsRewritten: boolean
}

type SettingsPathProjectionV2 = Partial<Pick<AppSettingsV1, 'workspaceRoot'>> & {
  runtime?: Partial<Pick<AppSettingsV1['runtime'], 'dataDir'>> & {
    storage?: Partial<Pick<AppSettingsV1['runtime']['storage'], 'sqlitePath'>>
  }
  write?: Partial<Pick<AppSettingsV1['write'], 'defaultWorkspaceRoot' | 'activeWorkspaceRoot' | 'workspaces'>>
  claw?: {
    skills?: Partial<Pick<AppSettingsV1['claw']['skills'], 'extraDirs' | 'disabledDirs'>>
    im?: Partial<Pick<AppSettingsV1['claw']['im'], 'workspaceRoot'>>
    channels?: Array<{
      workspaceRoot?: string
      conversations?: Array<{ workspaceRoot?: string }>
    }>
    tasks?: Array<{ workspaceRoot?: string }>
  }
  schedule?: {
    defaultWorkspaceRoot?: string
    skills?: Partial<Pick<AppSettingsV1['schedule']['skills'], 'extraDirs' | 'disabledDirs'>>
    tasks?: Array<{ workspaceRoot?: string }>
  }
}

function noopLog(): void {}

function pathDescription(state: StrictPathState): string {
  return state.kind
}

function assertDirectoryOrMissing(path: string, role: 'source' | 'target' | 'stage'): StrictPathState {
  const state = inspectPathStrict(path)
  if (state.kind === 'symlink') migrationBlocked('symlink_forbidden', { role })
  if (state.kind === 'other' || state.kind === 'file') {
    migrationBlocked(role === 'source' ? 'legacy_source_conflict' : 'path_conflict', {
      role,
      state: pathDescription(state)
    })
  }
  return state
}

function assertTargetMissing(path: string): void {
  const state = assertDirectoryOrMissing(path, 'target')
  if (state.kind !== 'missing') migrationBlocked('target_conflict')
}

function assertStageMissing(path: string): void {
  const state = assertDirectoryOrMissing(path, 'stage')
  if (state.kind !== 'missing') migrationBlocked('path_conflict')
}

function discoverUserDataSource(
  userDataPath: string,
  legacyDirNames: readonly string[]
): string | null {
  const parent = dirname(userDataPath)
  const currentName = basename(userDataPath)
  const candidates: string[] = []
  for (const legacyName of legacyDirNames) {
    if (legacyName === currentName) continue
    const path = join(parent, legacyName)
    const state = assertDirectoryOrMissing(path, 'source')
    if (state.kind === 'directory') candidates.push(path)
  }
  if (candidates.length > 1) migrationBlocked('legacy_source_conflict')
  return candidates[0] ?? null
}

export function inspectLegacyMigrationRequirement(input: {
  userDataPath: string
  legacyDirNames?: readonly string[]
}): {
  legacyUserDataPath: string | null
  singletonBaseline: ElectronSingletonArtifactV2[]
} {
  assertCanonicalDirectoryPath(dirname(input.userDataPath))
  const legacyUserDataPath = discoverUserDataSource(
    input.userDataPath,
    input.legacyDirNames ?? LEGACY_USER_DATA_DIR_NAMES
  )
  return {
    legacyUserDataPath,
    singletonBaseline: legacyUserDataPath
      ? inspectElectronSingletonArtifacts(legacyUserDataPath)
      : []
  }
}

export function inspectElectronSingletonArtifacts(
  legacyUserDataPath: string
): ElectronSingletonArtifactV2[] {
  const artifacts: ElectronSingletonArtifactV2[] = []
  for (const name of ELECTRON_SINGLETON_ROOT_ENTRY_NAMES) {
    const state = inspectPathStrict(join(legacyUserDataPath, name))
    if (state.kind === 'missing') continue
    if (state.kind === 'directory') migrationBlocked('quiescence_unavailable')
    artifacts.push({
      name,
      kind: state.kind,
      dev: state.dev,
      ino: state.ino
    })
  }
  return artifacts
}

export function issueElectronLegacySingletonAuthority(input: {
  legacyUserDataPath: string
  baseline: readonly ElectronSingletonArtifactV2[]
  assertHeld: () => boolean
}): LegacyMigrationQuiescenceAuthorityV2 {
  let held = false
  try {
    held = input.assertHeld()
  } catch {
    migrationBlocked('quiescence_unavailable')
  }
  if (!held) migrationBlocked('quiescence_unavailable')
  const current = inspectElectronSingletonArtifacts(input.legacyUserDataPath)
  for (const artifact of current) {
    const previous = input.baseline.find((entry) => entry.name === artifact.name)
    if (
      previous &&
      previous.kind === artifact.kind &&
      previous.dev === artifact.dev &&
      previous.ino === artifact.ino
    ) {
      // A same-identity pre-existing basename is not evidence that this
      // Electron singleton acquisition created or owns the artifact.
      migrationBlocked('quiescence_unavailable')
    }
  }
  return {
    schemaVersion: 2,
    kind: 'electron-legacy-user-data-singleton',
    legacyUserDataPath: input.legacyUserDataPath,
    singletonArtifacts: current,
    assertHeld: input.assertHeld
  }
}

function sourceSnapshotOptions(owner: LegacyMigrationPlannedOwnerV2): {
  excludedRootEntryNames?: readonly string[]
} {
  return owner.excludedSourceRootEntries.length > 0
    ? { excludedRootEntryNames: owner.excludedSourceRootEntries }
    : {}
}

function buildPlan(input: {
  userDataPath: string
  homeDir: string
  legacyDirNames: readonly string[]
  mappings: readonly HomeDataMigrationMapping[]
  transactionId: string
  now: Date
  homeSemanticReceiptSha256: string
  hostFilesystemScopeSha256: string
  hostFilesystemSemanticReceiptSha256: string
  excludedUserDataRootEntries: readonly string[]
}): LegacyMigrationPlanV2 | null {
  const owners: LegacyMigrationPlannedOwnerV2[] = []
  const homeMappingDecisions: LegacyMigrationHomeMappingDecisionV2[] = []
  const journalParentState = inspectPathStrict(dirname(input.userDataPath))
  if (journalParentState.kind !== 'directory') migrationBlocked('path_alias')
  const userDataState = assertDirectoryOrMissing(input.userDataPath, 'target')
  const userDataSource = discoverUserDataSource(input.userDataPath, input.legacyDirNames)
  if (userDataSource) {
    if (userDataState.kind !== 'missing') migrationBlocked('target_conflict')
    const sourceState = inspectPathStrict(userDataSource)
    if (sourceState.kind !== 'directory') {
      migrationBlocked('legacy_source_changed')
    }
    const ownerId: LegacyMigrationOwnerId = 'user-data'
    const stagePath = join(
      dirname(input.userDataPath),
      `.analytix-migration-v2-${input.transactionId}-${ownerId}.stage`
    )
    assertStageMissing(stagePath)
    owners.push({
      id: ownerId,
      sourcePath: userDataSource,
      targetPath: input.userDataPath,
      stagePath,
      sourceDev: sourceState.dev,
      sourceIno: sourceState.ino,
      sourceParentDev: journalParentState.dev,
      sourceParentIno: journalParentState.ino,
      targetParentDev: journalParentState.dev,
      targetParentIno: journalParentState.ino,
      excludedSourceRootEntries: [...input.excludedUserDataRootEntries],
      sourceSnapshot: snapshotDirectoryStrict(userDataSource, {
        excludedRootEntryNames: input.excludedUserDataRootEntries
      })
    })
  }

  for (let index = 0; index < input.mappings.length; index += 1) {
    const mapping = input.mappings[index]
    const sourcePath = join(input.homeDir, ...mapping.legacySegments)
    const targetPath = join(input.homeDir, ...mapping.nextSegments)
    const mappingParent = dirname(sourcePath)
    if (mappingParent !== dirname(targetPath)) migrationBlocked('path_alias')
    const mappingParentState = inspectPathStrict(mappingParent)
    if (mappingParentState.kind === 'symlink') migrationBlocked('symlink_forbidden')
    if (mappingParentState.kind === 'directory') {
      assertCanonicalDirectoryPath(mappingParent)
    } else if (mappingParentState.kind !== 'missing') {
      migrationBlocked('path_conflict')
    }
    const sourceState = assertDirectoryOrMissing(sourcePath, 'source')
    const targetState = assertDirectoryOrMissing(targetPath, 'target')
    const caseSensitivity = detectPathCaseSensitivity(
      mappingParentState.kind === 'directory' ? mappingParent : input.homeDir
    )
    if (sourceState.kind !== 'directory') {
      homeMappingDecisions.push({
        index,
        sourcePath,
        targetPath,
        sourceState: 'absent',
        targetState: targetState.kind === 'directory' ? 'existing-directory' : 'missing',
        caseSensitivity,
        targetDev: targetState.kind === 'directory' ? targetState.dev : '',
        targetIno: targetState.kind === 'directory' ? targetState.ino : ''
      })
      continue
    }
    if (targetState.kind !== 'missing') migrationBlocked('target_conflict')
    if (mappingParentState.kind !== 'directory') migrationBlocked('legacy_source_changed')

    const ownerId: LegacyMigrationOwnerId = `home-data-${index}`
    const stagePath = join(
      dirname(targetPath),
      `.analytix-migration-v2-${input.transactionId}-${ownerId}.stage`
    )
    assertStageMissing(stagePath)
    owners.push({
      id: ownerId,
      sourcePath,
      targetPath,
      stagePath,
      sourceDev: sourceState.dev,
      sourceIno: sourceState.ino,
      sourceParentDev: mappingParentState.dev,
      sourceParentIno: mappingParentState.ino,
      targetParentDev: mappingParentState.dev,
      targetParentIno: mappingParentState.ino,
      excludedSourceRootEntries: [],
      sourceSnapshot: snapshotDirectoryStrict(sourcePath)
    })
    homeMappingDecisions.push({
      index,
      sourcePath,
      targetPath,
      sourceState: 'owned-directory',
      targetState: 'missing',
      caseSensitivity,
      targetDev: '',
      targetIno: ''
    })
  }

  if (owners.length === 0) {
    if (userDataState.kind === 'symlink') migrationBlocked('symlink_forbidden')
    return null
  }

  // The full all-owner source set is re-read after every snapshot. A later
  // semantic/source failure therefore occurs before a journal namespace or
  // target/stage mutation exists.
  for (const owner of owners) {
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
  }

  return {
    schemaVersion: 2,
    transactionId: input.transactionId,
    createdAt: input.now.toISOString(),
    userDataPath: input.userDataPath,
    journalParentDev: journalParentState.dev,
    journalParentIno: journalParentState.ino,
    owners,
    homeMappingDecisions,
    homeSemanticReceiptSha256: owners.some((owner) => owner.id.startsWith('home-data-'))
      ? input.homeSemanticReceiptSha256
      : '',
    hostFilesystemScopeSha256: input.hostFilesystemScopeSha256,
    hostFilesystemSemanticReceiptSha256: input.hostFilesystemSemanticReceiptSha256
  }
}

function assertJournalPlanScope(input: {
  plan: LegacyMigrationPlanV2
  userDataPath: string
  homeDir: string
  legacyDirNames: readonly string[]
  mappings: readonly HomeDataMigrationMapping[]
}): void {
  if (input.plan.userDataPath !== input.userDataPath) migrationBlocked('invalid_journal')
  const journalParent = inspectPathStrict(dirname(input.userDataPath))
  if (
    journalParent.kind !== 'directory' ||
    journalParent.dev !== input.plan.journalParentDev ||
    journalParent.ino !== input.plan.journalParentIno
  ) {
    migrationBlocked('path_alias')
  }
  const expectedUserSources = new Set(
    input.legacyDirNames
      .filter((name) => name !== basename(input.userDataPath))
      .map((name) => join(dirname(input.userDataPath), name))
  )
  let userOwnerCount = 0
  const homeOwnerIndexes = new Set<number>()
  for (const owner of input.plan.owners) {
    if (owner.id === 'user-data') {
      userOwnerCount += 1
      if (
        owner.targetPath !== input.userDataPath ||
        !expectedUserSources.has(owner.sourcePath) ||
        owner.excludedSourceRootEntries.some((name) =>
          !ELECTRON_SINGLETON_ROOT_ENTRY_NAMES.includes(
            name as (typeof ELECTRON_SINGLETON_ROOT_ENTRY_NAMES)[number]
          )
        )
      ) {
        migrationBlocked('invalid_journal')
      }
      continue
    }
    const match = /^home-data-(\d+)$/.exec(owner.id)
    const index = match ? Number(match[1]) : -1
    const mapping = input.mappings[index]
    if (
      !mapping ||
      homeOwnerIndexes.has(index) ||
      owner.sourcePath !== join(input.homeDir, ...mapping.legacySegments) ||
      owner.targetPath !== join(input.homeDir, ...mapping.nextSegments) ||
      owner.excludedSourceRootEntries.length !== 0
    ) {
      migrationBlocked('invalid_journal')
    }
    homeOwnerIndexes.add(index)
  }
  if (userOwnerCount > 1) migrationBlocked('invalid_journal')
  if (input.plan.homeMappingDecisions.length !== input.mappings.length) {
    migrationBlocked('invalid_journal')
  }
  for (let index = 0; index < input.mappings.length; index += 1) {
    const mapping = input.mappings[index]
    const decision = input.plan.homeMappingDecisions[index]
    if (
      decision?.index !== index ||
      decision.sourcePath !== join(input.homeDir, ...mapping.legacySegments) ||
      decision.targetPath !== join(input.homeDir, ...mapping.nextSegments) ||
      (decision.sourceState === 'owned-directory') !== homeOwnerIndexes.has(index)
    ) {
      migrationBlocked('invalid_journal')
    }
    const mappingParent = dirname(decision.sourcePath)
    const mappingParentState = inspectPathStrict(mappingParent)
    const currentCaseSensitivity = detectPathCaseSensitivity(
      mappingParentState.kind === 'directory' ? mappingParent : input.homeDir
    )
    if (decision.caseSensitivity !== currentCaseSensitivity) {
      migrationBlocked('path_alias')
    }
  }
}

function assertAbsentMappingDecisionsCurrent(plan: LegacyMigrationPlanV2): void {
  for (const decision of plan.homeMappingDecisions) {
    if (decision.sourceState !== 'absent') continue
    if (inspectPathStrict(decision.sourcePath).kind !== 'missing') {
      migrationBlocked('legacy_source_changed')
    }
    const target = inspectPathStrict(decision.targetPath)
    if (
      (decision.targetState === 'missing' && target.kind !== 'missing') ||
      (decision.targetState === 'existing-directory' && (
        target.kind !== 'directory' ||
        target.dev !== decision.targetDev ||
        target.ino !== decision.targetIno
      ))
    ) {
      migrationBlocked('target_conflict')
    }
  }
}

function expectedHostPathBindings(
  plan: LegacyMigrationPlanV2
): LegacyMigrationHostPathBindingV2[] {
  const bindings = new Map<string, LegacyMigrationHostPathBindingV2>()
  const add = (binding: LegacyMigrationHostPathBindingV2): void => {
    const key = `${binding.kind}\0${binding.path}`
    const previous = bindings.get(key)
    if (
      previous &&
      (previous.dev !== binding.dev || previous.ino !== binding.ino)
    ) {
      migrationBlocked('invalid_journal')
    }
    bindings.set(key, binding)
  }
  add({
    path: dirname(plan.userDataPath),
    kind: 'parent-directory',
    dev: plan.journalParentDev,
    ino: plan.journalParentIno
  })
  for (const owner of plan.owners) {
    add({
      path: owner.sourcePath,
      kind: 'source-directory',
      dev: owner.sourceDev,
      ino: owner.sourceIno
    })
    add({
      path: dirname(owner.sourcePath),
      kind: 'parent-directory',
      dev: owner.sourceParentDev,
      ino: owner.sourceParentIno
    })
    add({
      path: dirname(owner.targetPath),
      kind: 'parent-directory',
      dev: owner.targetParentDev,
      ino: owner.targetParentIno
    })
  }
  return [...bindings.values()].sort((left, right) =>
    `${left.kind}\0${left.path}`.localeCompare(`${right.kind}\0${right.path}`, 'en')
  )
}

function assertOwnerPathBindingsCurrent(owner: LegacyMigrationPlannedOwnerV2): void {
  const source = inspectPathStrict(owner.sourcePath)
  const sourceParent = inspectPathStrict(dirname(owner.sourcePath))
  const targetParent = inspectPathStrict(dirname(owner.targetPath))
  if (
    source.kind !== 'directory' ||
    source.dev !== owner.sourceDev ||
    source.ino !== owner.sourceIno
  ) {
    migrationBlocked('legacy_source_changed')
  }
  if (
    sourceParent.kind !== 'directory' ||
    sourceParent.dev !== owner.sourceParentDev ||
    sourceParent.ino !== owner.sourceParentIno ||
    targetParent.kind !== 'directory' ||
    targetParent.dev !== owner.targetParentDev ||
    targetParent.ino !== owner.targetParentIno
  ) {
    migrationBlocked('path_alias')
  }
}

function expectedActivationPrimitive(): HostDirectoryActivationPrimitiveV2 | null {
  if (process.platform === 'darwin') return 'darwin-renameatx-np-rename-excl'
  if (process.platform === 'linux') return 'linux-renameat2-rename-noreplace'
  if (process.platform === 'win32') return 'windows-handle-move-fail-if-exists'
  return null
}

function assertHostFilesystemAuthorityPrewrite(
  authority: LegacyMigrationHostFilesystemAuthorityV2 | undefined,
  requiredParentPaths: readonly string[] = []
): LegacyMigrationHostFilesystemAuthorityV2 {
  const primitive = expectedActivationPrimitive()
  if (
    !primitive ||
    authority?.schemaVersion !== 2 ||
    authority.kind !== 'host-handle-relative-legacy-migration-transaction' ||
    authority.activationPrimitive !== primitive ||
    !/^[a-f0-9]{64}$/.test(authority.scopeSha256) ||
    !/^[a-f0-9]{64}$/.test(authority.semanticReceiptSha256) ||
    !Array.isArray(authority.bindings) ||
    authority.bindings.length === 0 ||
    typeof authority.assertHeld !== 'function' ||
    typeof authority.activateOrRecoverDirectoryNoReplace !== 'function'
  ) {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  const authorityKeys = Object.keys(authority).sort()
  const expectedAuthorityKeys = [
    'schemaVersion',
    'kind',
    'scopeSha256',
    'semanticReceiptSha256',
    'activationPrimitive',
    'bindings',
    'assertHeld',
    'activateOrRecoverDirectoryNoReplace'
  ].sort()
  if (
    authorityKeys.length !== expectedAuthorityKeys.length ||
    authorityKeys.some((key, index) => key !== expectedAuthorityKeys[index])
  ) {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  const keys = new Set<string>()
  for (const binding of authority.bindings) {
    const bindingKeys = Object.keys(binding).sort()
    if (
      bindingKeys.length !== 4 ||
      bindingKeys.some((key, index) =>
        key !== ['dev', 'ino', 'kind', 'path'][index]
      )
    ) {
      migrationBlocked('host_filesystem_authority_unavailable')
    }
    if (
      !binding ||
      !['parent-directory', 'source-directory'].includes(binding.kind) ||
      !isAbsolute(binding.path) ||
      resolve(binding.path) !== binding.path ||
      !binding.dev ||
      !binding.ino
    ) {
      migrationBlocked('host_filesystem_authority_unavailable')
    }
    const key = `${binding.kind}\0${binding.path}`
    if (keys.has(key)) migrationBlocked('host_filesystem_authority_unavailable')
    keys.add(key)
    const state = inspectPathStrict(binding.path)
    if (
      state.kind !== 'directory' ||
      state.dev !== binding.dev ||
      state.ino !== binding.ino
    ) {
      migrationBlocked('host_filesystem_authority_unavailable')
    }
  }
  const sorted = [...authority.bindings].sort((left, right) =>
    `${left.kind}\0${left.path}`.localeCompare(`${right.kind}\0${right.path}`, 'en')
  )
  if (
    stableJson(sorted) !== stableJson(authority.bindings) ||
    sha256Bytes(stableJson(authority.bindings)) !== authority.scopeSha256
  ) {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  for (const requiredPath of requiredParentPaths) {
    const requiredState = inspectPathStrict(requiredPath)
    const binding = authority.bindings.find((entry) =>
      entry.kind === 'parent-directory' && entry.path === requiredPath
    )
    if (
      requiredState.kind !== 'directory' ||
      binding?.dev !== requiredState.dev ||
      binding.ino !== requiredState.ino
    ) {
      migrationBlocked('host_filesystem_authority_unavailable')
    }
  }
  let held = false
  try {
    held = authority.assertHeld()
  } catch {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  if (!held) migrationBlocked('host_filesystem_authority_unavailable')
  return authority
}

function assertHostFilesystemAuthority(
  plan: LegacyMigrationPlanV2,
  authority: LegacyMigrationHostFilesystemAuthorityV2 | undefined
): LegacyMigrationHostFilesystemAuthorityV2 {
  assertHostFilesystemAuthorityPrewrite(authority)
  const bindings = expectedHostPathBindings(plan)
  const scopeSha256 = sha256Bytes(stableJson(bindings))
  const primitive = expectedActivationPrimitive()
  if (
    !primitive ||
    authority?.schemaVersion !== 2 ||
    authority.kind !== 'host-handle-relative-legacy-migration-transaction' ||
    authority.activationPrimitive !== primitive ||
    authority.scopeSha256 !== scopeSha256 ||
    authority.scopeSha256 !== plan.hostFilesystemScopeSha256 ||
    authority.semanticReceiptSha256 !== plan.hostFilesystemSemanticReceiptSha256 ||
    stableJson(authority.bindings) !== stableJson(bindings)
  ) {
    migrationBlocked('host_filesystem_authority_unavailable')
  }
  for (const owner of plan.owners) assertOwnerPathBindingsCurrent(owner)
  return authority
}

function assertQuiescenceAuthority(
  plan: LegacyMigrationPlanV2,
  authority: LegacyMigrationQuiescenceAuthorityV2 | undefined
): LegacyMigrationHostFilesystemAuthorityV2 {
  const userOwner = plan.owners.find((owner) => owner.id === 'user-data')
  // A home-only owner has no old Electron singleton whose ownership can prove
  // that the old runtime is quiescent. Keep it blocked until the Go semantic
  // migration/installer supplies that authority.
  if (!userOwner) migrationBlocked('quiescence_unavailable')
  if (
    authority?.schemaVersion !== 2 ||
    authority.kind !== 'electron-legacy-user-data-singleton' ||
    authority.legacyUserDataPath !== userOwner.sourcePath ||
    JSON.stringify(authority.singletonArtifacts.map((artifact) => artifact.name).sort()) !==
      JSON.stringify([...userOwner.excludedSourceRootEntries].sort())
  ) {
    migrationBlocked('quiescence_unavailable')
  }
  let held = false
  try {
    held = authority.assertHeld()
  } catch {
    migrationBlocked('quiescence_unavailable')
  }
  if (!held) migrationBlocked('quiescence_unavailable')
  const currentSingletonArtifacts = inspectElectronSingletonArtifacts(userOwner.sourcePath)
  if (JSON.stringify(currentSingletonArtifacts) !== JSON.stringify(authority.singletonArtifacts)) {
    migrationBlocked('quiescence_unavailable')
  }
  const homeSourcePaths = plan.owners
    .filter((owner) => owner.id.startsWith('home-data-'))
    .map((owner) => owner.sourcePath)
    .sort((left, right) => left.localeCompare(right, 'en'))
  if (homeSourcePaths.length === 0) {
    return assertHostFilesystemAuthority(plan, authority.filesystem)
  }
  const homeData = authority.homeData
  if (
    homeData?.schemaVersion !== 2 ||
    homeData.kind !== 'host-go-persistence-quiescence' ||
    !/^[a-f0-9]{64}$/.test(homeData.semanticReceiptSha256) ||
    homeData.semanticReceiptSha256 !== plan.homeSemanticReceiptSha256 ||
    JSON.stringify([...homeData.sourcePaths].sort((left, right) => left.localeCompare(right, 'en'))) !==
      JSON.stringify(homeSourcePaths)
  ) {
    migrationBlocked('quiescence_unavailable')
  }
  let homeHeld = false
  try {
    homeHeld = homeData.assertHeld()
  } catch {
    migrationBlocked('quiescence_unavailable')
  }
  if (!homeHeld) migrationBlocked('quiescence_unavailable')
  return assertHostFilesystemAuthority(plan, authority.filesystem)
}

function replacementPairs(
  homeDir: string,
  mappings: readonly HomeDataMigrationMapping[],
  decisions: readonly LegacyMigrationHomeMappingDecisionV2[]
): Array<{ from: string; to: string; caseSensitivity: 'sensitive' | 'insensitive' }> {
  const pairs: Array<{
    from: string
    to: string
    caseSensitivity: 'sensitive' | 'insensitive'
  }> = []
  const add = (
    from: string,
    to: string,
    caseSensitivity: 'sensitive' | 'insensitive'
  ): void => {
    if (!pairs.some((pair) =>
      pair.from === from &&
      pair.to === to &&
      pair.caseSensitivity === caseSensitivity
    )) {
      pairs.push({ from, to, caseSensitivity })
    }
  }
  for (let index = 0; index < mappings.length; index += 1) {
    const mapping = mappings[index]
    const decision = decisions[index]
    if (!decision || decision.index !== index) migrationBlocked('invalid_journal')
    const from = join(homeDir, ...mapping.legacySegments)
    const to = join(homeDir, ...mapping.nextSegments)
    add(from, to, decision.caseSensitivity)
    add(from.replace(/\\/g, '/'), to.replace(/\\/g, '/'), decision.caseSensitivity)
    add(from.replace(/\//g, '\\'), to.replace(/\//g, '\\'), decision.caseSensitivity)
    add(
      `~/${mapping.legacySegments.join('/')}`,
      `~/${mapping.nextSegments.join('/')}`,
      decision.caseSensitivity
    )
    add(
      `~\\${mapping.legacySegments.join('\\')}`,
      `~\\${mapping.nextSegments.join('\\')}`,
      decision.caseSensitivity
    )
  }
  return pairs
}

type SettingsReplacementPair = ReturnType<typeof replacementPairs>[number]

function rewritePath(value: unknown, pairs: readonly SettingsReplacementPair[]): unknown {
  if (typeof value !== 'string') return value
  const fold = (
    input: string,
    caseSensitivity: SettingsReplacementPair['caseSensitivity']
  ): string => {
    const normalized = input.normalize('NFC')
    return caseSensitivity === 'insensitive'
      ? normalized.toLocaleLowerCase('en-US')
      : normalized
  }
  for (const pair of pairs) {
    const prefix = value.slice(0, pair.from.length)
    if (
      fold(prefix, pair.caseSensitivity) !==
      fold(pair.from, pair.caseSensitivity)
    ) continue
    if (value.length === pair.from.length) return pair.to
    if (value.length > pair.from.length) {
      const boundary = value.charAt(pair.from.length)
      if (boundary === '/' || boundary === '\\') {
        return pair.to + value.slice(pair.from.length)
      }
    }
  }
  return value
}

function rewriteStringProperty(
  record: Record<string, unknown> | undefined,
  key: string,
  pairs: readonly SettingsReplacementPair[]
): boolean {
  if (!record || !(key in record)) return false
  const previous = record[key]
  const next = rewritePath(previous, pairs)
  if (next === previous) return false
  record[key] = next
  return true
}

function rewriteStringArray(
  record: Record<string, unknown> | undefined,
  key: string,
  pairs: readonly SettingsReplacementPair[]
): boolean {
  if (!record || !Array.isArray(record[key])) return false
  let changed = false
  record[key] = (record[key] as unknown[]).map((value) => {
    const next = rewritePath(value, pairs)
    changed = changed || next !== value
    return next
  })
  return changed
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined
}

function rewriteTaskRoots(
  value: unknown,
  pairs: readonly SettingsReplacementPair[]
): boolean {
  if (!Array.isArray(value)) return false
  let changed = false
  for (const task of value) changed = rewriteStringProperty(asRecord(task), 'workspaceRoot', pairs) || changed
  return changed
}

function rewriteTypedSettingsPaths(
  parsed: unknown,
  pairs: readonly SettingsReplacementPair[]
): { value: unknown; changed: boolean } {
  const root = asRecord(parsed)
  if (!root) return { value: parsed, changed: false }

  // Clone before mutation. Only the typed path-bearing settings fields below
  // are eligible; prose, prompts, credentials, and arbitrary extension fields
  // are never recursively rewritten.
  const value = structuredClone(parsed) as SettingsPathProjectionV2
  const nextRoot = asRecord(value)
  if (!nextRoot) return { value: parsed, changed: false }
  let changed = rewriteStringProperty(nextRoot, 'workspaceRoot', pairs)

  const runtime = asRecord(nextRoot.runtime)
  changed = rewriteStringProperty(runtime, 'dataDir', pairs) || changed
  changed = rewriteStringProperty(asRecord(runtime?.storage), 'sqlitePath', pairs) || changed

  const write = asRecord(nextRoot.write)
  changed = rewriteStringProperty(write, 'defaultWorkspaceRoot', pairs) || changed
  changed = rewriteStringProperty(write, 'activeWorkspaceRoot', pairs) || changed
  changed = rewriteStringArray(write, 'workspaces', pairs) || changed

  const claw = asRecord(nextRoot.claw)
  const clawSkills = asRecord(claw?.skills)
  changed = rewriteStringArray(clawSkills, 'extraDirs', pairs) || changed
  changed = rewriteStringArray(clawSkills, 'disabledDirs', pairs) || changed
  changed = rewriteStringProperty(asRecord(claw?.im), 'workspaceRoot', pairs) || changed
  if (Array.isArray(claw?.channels)) {
    for (const channelValue of claw.channels) {
      const channel = asRecord(channelValue)
      changed = rewriteStringProperty(channel, 'workspaceRoot', pairs) || changed
      if (Array.isArray(channel?.conversations)) {
        for (const conversation of channel.conversations) {
          changed = rewriteStringProperty(asRecord(conversation), 'workspaceRoot', pairs) || changed
        }
      }
    }
  }
  changed = rewriteTaskRoots(claw?.tasks, pairs) || changed

  const schedule = asRecord(nextRoot.schedule)
  changed = rewriteStringProperty(schedule, 'defaultWorkspaceRoot', pairs) || changed
  const scheduleSkills = asRecord(schedule?.skills)
  changed = rewriteStringArray(scheduleSkills, 'extraDirs', pairs) || changed
  changed = rewriteStringArray(scheduleSkills, 'disabledDirs', pairs) || changed
  changed = rewriteTaskRoots(schedule?.tasks, pairs) || changed

  return { value, changed }
}

function rewriteStagedSettings(input: {
  userDataStagePath: string
  homeDir: string
  mappings: readonly HomeDataMigrationMapping[]
  decisions: readonly LegacyMigrationHomeMappingDecisionV2[]
  log: MigrationLogger
}): boolean {
  const pairs = replacementPairs(input.homeDir, input.mappings, input.decisions)
  let rewritten = false
  for (const fileName of SETTINGS_FILE_NAMES) {
    const path = join(input.userDataStagePath, fileName)
    const state = inspectPathStrict(path)
    if (state.kind === 'missing') continue
    if (state.kind === 'symlink') migrationBlocked('symlink_forbidden')
    if (state.kind !== 'file') migrationBlocked('path_conflict')
    const file = readRegularFileStrict(path)
    let parsed: unknown
    try {
      parsed = JSON.parse(file.raw)
    } catch {
      input.log('legacy-migration: invalid settings JSON preserved for settings-store recovery', {
        fileName
      })
      continue
    }
    const result = rewriteTypedSettingsPaths(parsed, pairs)
    if (!result.changed) continue
    durableReplaceRegularFile(path, `${JSON.stringify(result.value, null, 2)}\n`, file.mode)
    rewritten = true
  }
  return rewritten
}

function readStagedPayload(journal: LoadedLegacyMigrationJournalV2): StagedPayloadV2 {
  const record = journalRecord(journal, 'staged')
  if (!record || typeof record.record.payload !== 'object' || record.record.payload === null) {
    migrationBlocked('invalid_journal')
  }
  return record.record.payload as StagedPayloadV2
}

function stagedOwnerFor(
  payload: StagedPayloadV2,
  ownerId: LegacyMigrationOwnerId
): LegacyMigrationStagedOwnerV2 {
  const owner = payload.owners.find((entry) => entry.id === ownerId)
  if (!owner) migrationBlocked('invalid_journal')
  return owner
}

function stagePlan(input: {
  journal: LoadedLegacyMigrationJournalV2
  homeDir: string
  mappings: readonly HomeDataMigrationMapping[]
  log: MigrationLogger
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
}): LoadedLegacyMigrationJournalV2 {
  let settingsRewritten = false
  assertAbsentMappingDecisionsCurrent(input.journal.plan)
  for (const owner of input.journal.plan.owners) {
    assertQuiescenceAuthority(input.journal.plan, input.quiescenceAuthority)
    assertOwnerPathBindingsCurrent(owner)
    removeJournalOwnedStage({
      stagePath: owner.stagePath,
      targetPath: owner.targetPath,
      transactionId: input.journal.plan.transactionId,
      ownerId: owner.id
    })
    assertTargetMissing(owner.targetPath)
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
    copySnapshotToNewStage(
      owner.sourcePath,
      owner.stagePath,
      owner.sourceSnapshot,
      sourceSnapshotOptions(owner)
    )
    if (owner.id === 'user-data') {
      settingsRewritten = rewriteStagedSettings({
        userDataStagePath: owner.stagePath,
        homeDir: input.homeDir,
        mappings: input.mappings,
        decisions: input.journal.plan.homeMappingDecisions,
        log: input.log
      }) || settingsRewritten
    }
    applySnapshotPermissions(owner.stagePath, owner.sourceSnapshot)
    assertOwnerPathBindingsCurrent(owner)
  }

  const owners = input.journal.plan.owners.map((owner): LegacyMigrationStagedOwnerV2 => {
    const stage = inspectPathStrict(owner.stagePath)
    if (stage.kind !== 'directory') migrationBlocked('unexpected_path_state')
    return {
      id: owner.id,
      stageSnapshot: snapshotDirectoryStrict(owner.stagePath),
      stageDev: stage.dev,
      stageIno: stage.ino
    }
  })
  for (const owner of input.journal.plan.owners) {
    assertOwnerPathBindingsCurrent(owner)
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
  }
  assertAbsentMappingDecisionsCurrent(input.journal.plan)
  return appendLegacyMigrationJournalRecord(input.journal, 'staged', {
    owners,
    settingsRewritten
  } satisfies StagedPayloadV2)
}

function appendActivationIntent(
  journal: LoadedLegacyMigrationJournalV2,
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
): LoadedLegacyMigrationJournalV2 {
  assertQuiescenceAuthority(journal.plan, quiescenceAuthority)
  assertAbsentMappingDecisionsCurrent(journal.plan)
  const staged = journalRecord(journal, 'staged')
  if (!staged) migrationBlocked('invalid_journal')
  for (const owner of journal.plan.owners) {
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
  }
  return appendLegacyMigrationJournalRecord(journal, 'activation-intent', {
    stagedRecordSha256: staged.sha256
  })
}

function visibleOwnerCount(journal: LoadedLegacyMigrationJournalV2): number {
  return journal.records.filter((record) => record.record.state === 'owner-visible').length
}

function activateRemainingOwners(input: {
  journal: LoadedLegacyMigrationJournalV2
  hooks?: LegacyMigrationTestHooks
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
}): LoadedLegacyMigrationJournalV2 {
  let journal = input.journal
  const staged = readStagedPayload(journal)
  let startIndex = visibleOwnerCount(journal)

  for (let index = 0; index < startIndex; index += 1) {
    const owner = journal.plan.owners[index]
    const stagedOwner = stagedOwnerFor(staged, owner.id)
    const target = inspectPathStrict(owner.targetPath)
    if (
      target.kind !== 'directory' ||
      target.dev !== stagedOwner.stageDev ||
      target.ino !== stagedOwner.stageIno
    ) {
      migrationBlocked('target_conflict')
    }
    assertDirectorySnapshot(owner.targetPath, stagedOwner.stageSnapshot)
    if (inspectPathStrict(owner.stagePath).kind !== 'missing') migrationBlocked('target_conflict')
  }

  while (startIndex < journal.plan.owners.length) {
    const owner = journal.plan.owners[startIndex]
    const stagedOwner = stagedOwnerFor(staged, owner.id)
    const stageSnapshot = stagedOwner.stageSnapshot
    assertQuiescenceAuthority(journal.plan, input.quiescenceAuthority)
    assertAbsentMappingDecisionsCurrent(journal.plan)
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
    input.hooks?.beforeActivation?.(owner)
    const filesystem = assertQuiescenceAuthority(journal.plan, input.quiescenceAuthority)
    assertAbsentMappingDecisionsCurrent(journal.plan)
    assertOwnerPathBindingsCurrent(owner)
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
    const activationRequest: HostDirectoryActivationRequestV2 = {
      schemaVersion: 2,
      requiredPrimitive: filesystem.activationPrimitive,
      hostScopeSha256: filesystem.scopeSha256,
      hostSemanticReceiptSha256: filesystem.semanticReceiptSha256,
      transactionId: journal.plan.transactionId,
      ownerId: owner.id,
      stagePath: owner.stagePath,
      targetPath: owner.targetPath,
      stageDev: stagedOwner.stageDev,
      stageIno: stagedOwner.stageIno,
      targetParentDev: owner.targetParentDev,
      targetParentIno: owner.targetParentIno,
      expectedManifestSha256: stageSnapshot.manifestSha256
    }
    const targetIdentity = activateStageDirectory(
      owner.stagePath,
      owner.targetPath,
      stageSnapshot,
      { dev: stagedOwner.stageDev, ino: stagedOwner.stageIno },
      activationRequest,
      filesystem.activateOrRecoverDirectoryNoReplace
    )
    input.hooks?.afterActivationBeforeReceipt?.(owner)
    journal = appendLegacyMigrationJournalRecord(journal, 'owner-visible', {
      ownerId: owner.id,
      targetManifestSha256: stageSnapshot.manifestSha256,
      targetDev: targetIdentity.dev,
      targetIno: targetIdentity.ino,
      activationReceipt: targetIdentity.activationReceipt
    })
    input.hooks?.checkpoint?.(`after-owner-visible:${owner.id}`)
    startIndex += 1
  }
  return journal
}

function appendActive(
  journal: LoadedLegacyMigrationJournalV2,
  now: Date,
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
): LoadedLegacyMigrationJournalV2 {
  assertQuiescenceAuthority(journal.plan, quiescenceAuthority)
  assertAbsentMappingDecisionsCurrent(journal.plan)
  const planned = journal.records[0]
  const staged = journalRecord(journal, 'staged')
  if (!planned || !staged) migrationBlocked('invalid_journal')
  const stagedPayload = readStagedPayload(journal)
  for (const owner of journal.plan.owners) {
    const stagedOwner = stagedOwnerFor(stagedPayload, owner.id)
    const target = inspectPathStrict(owner.targetPath)
    if (
      target.kind !== 'directory' ||
      target.dev !== stagedOwner.stageDev ||
      target.ino !== stagedOwner.stageIno
    ) {
      migrationBlocked('target_conflict')
    }
    assertDirectorySnapshot(owner.sourcePath, owner.sourceSnapshot, sourceSnapshotOptions(owner))
    assertDirectorySnapshot(owner.targetPath, stagedOwner.stageSnapshot)
  }
  return appendLegacyMigrationJournalRecord(journal, 'active', {
    activatedAt: now.toISOString(),
    planSha256: planned.sha256,
    stagedRecordSha256: staged.sha256
  })
}

function activeResult(
  journal: LoadedLegacyMigrationJournalV2,
  status: LegacyMigrationStatus
): LegacyDataMigrationResult {
  const active = journalRecord(journal, 'active')
  if (!active) migrationBlocked('invalid_journal')
  const visibleRecords = journal.records.filter((record) => record.record.state === 'owner-visible')
  for (const owner of journal.plan.owners) {
    const target = inspectPathStrict(owner.targetPath)
    if (target.kind === 'symlink') migrationBlocked('symlink_forbidden')
    if (target.kind !== 'directory') migrationBlocked('target_conflict')
    const visible = visibleRecords.find((record) => {
      const payload = record.record.payload as { ownerId?: unknown }
      return payload.ownerId === owner.id
    })
    const payload = visible?.record.payload as { targetDev?: unknown; targetIno?: unknown } | undefined
    if (payload?.targetDev !== target.dev || payload.targetIno !== target.ino) {
      migrationBlocked('target_conflict')
    }
    if (inspectPathStrict(owner.stagePath).kind !== 'missing') migrationBlocked('target_conflict')
  }
  const staged = readStagedPayload(journal)
  return {
    barrier: {
      schemaVersion: 2,
      ready: true,
      status,
      transactionId: journal.plan.transactionId,
      receiptSha256: active.sha256
    },
    userDataPath: journal.plan.userDataPath,
    migratedOwnerIds: journal.plan.owners.map((owner) => owner.id),
    legacySourcesPreserved: true,
    settingsRewritten: staged.settingsRewritten
  }
}

function readyExistingResult(userDataPath: string): LegacyDataMigrationResult {
  return {
    barrier: {
      schemaVersion: 2,
      ready: true,
      status: 'ready-existing',
      transactionId: '',
      receiptSha256: sha256Bytes('analytix-legacy-migration-v2:no-transaction')
    },
    userDataPath,
    migratedOwnerIds: [],
    legacySourcesPreserved: true,
    settingsRewritten: false
  }
}

function resumeJournal(input: {
  journal: LoadedLegacyMigrationJournalV2
  homeDir: string
  mappings: readonly HomeDataMigrationMapping[]
  log: MigrationLogger
  hooks?: LegacyMigrationTestHooks
  status: 'migrated' | 'recovered'
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
}): LegacyDataMigrationResult {
  let journal = input.journal
  if (journalLastState(journal) === 'active') return activeResult(journal, 'ready-existing')

  if (journalLastState(journal) === 'planned') {
    journal = stagePlan({
      journal,
      homeDir: input.homeDir,
      mappings: input.mappings,
      log: input.log,
      quiescenceAuthority: input.quiescenceAuthority
    })
    input.hooks?.checkpoint?.('after-staged')
  }
  if (journalLastState(journal) === 'staged') {
    journal = appendActivationIntent(journal, input.quiescenceAuthority)
    input.hooks?.checkpoint?.('after-activation-intent')
  }
  if (
    journalLastState(journal) === 'activation-intent' ||
    journalLastState(journal) === 'owner-visible'
  ) {
    journal = activateRemainingOwners({
      journal,
      hooks: input.hooks,
      quiescenceAuthority: input.quiescenceAuthority
    })
  }
  if (journalLastState(journal) === 'owner-visible') {
    journal = appendActive(
      journal,
      input.hooks?.now?.() ?? new Date(),
      input.quiescenceAuthority
    )
    input.hooks?.checkpoint?.('after-active')
  }
  if (journalLastState(journal) !== 'active') migrationBlocked('invalid_journal')
  input.log('legacy-migration: activation barrier is durable', {
    status: input.status,
    ownerCount: journal.plan.owners.length
  })
  return activeResult(journal, input.status)
}

/**
 * Synchronous startup barrier. The caller must run it before constructing the
 * settings store, Go runtime adapter, IPC surfaces, renderer, or any other
 * consumer of a migrated owner. It acquires a dedicated exclusive quiescence
 * lease first; helper processes must skip this entry point entirely.
 *
 * Known conflicts and all unexpected boundary errors throw a fixed-code
 * LegacyMigrationBlockedError. Callers must terminate startup rather than
 * falling back to a legacy writable path.
 */
export function runLegacyAnalytixDataMigration(input: {
  userDataPath: string
  homeDir: string
  legacyDirNames?: readonly string[]
  mappings?: readonly HomeDataMigrationMapping[]
  log?: MigrationLogger
  testHooks?: LegacyMigrationTestHooks
  quiescenceAuthority?: LegacyMigrationQuiescenceAuthorityV2
}): LegacyDataMigrationResult {
  const log = input.log ?? noopLog
  const mappings = input.mappings ?? HOME_DATA_MIGRATION_MAPPINGS
  const legacyDirNames = input.legacyDirNames ?? LEGACY_USER_DATA_DIR_NAMES
  assertCanonicalDirectoryPath(dirname(input.userDataPath))
  assertCanonicalDirectoryPath(input.homeDir)
  const transactionId = input.testHooks?.transactionId?.() ?? randomUUID()
  const now = input.testHooks?.now?.() ?? new Date()
  const makePlan = (): LegacyMigrationPlanV2 | null => buildPlan({
    userDataPath: input.userDataPath,
    homeDir: input.homeDir,
    legacyDirNames,
    mappings,
    transactionId,
    now,
    homeSemanticReceiptSha256:
      input.quiescenceAuthority?.homeData?.semanticReceiptSha256 ?? '',
    hostFilesystemSemanticReceiptSha256:
      input.quiescenceAuthority?.filesystem?.semanticReceiptSha256 ?? '',
    hostFilesystemScopeSha256:
      input.quiescenceAuthority?.filesystem?.scopeSha256 ?? '',
    excludedUserDataRootEntries:
      input.quiescenceAuthority?.singletonArtifacts.map((artifact) => artifact.name) ?? []
  })
  const assertScope = (plan: LegacyMigrationPlanV2): void => {
    assertJournalPlanScope({
      plan,
      userDataPath: input.userDataPath,
      homeDir: input.homeDir,
      legacyDirNames,
      mappings
    })
  }

  try {
    let preliminaryExisting: LoadedLegacyMigrationJournalV2 | null = null
    let recoveryMutationRequired = false
    try {
      preliminaryExisting = loadLegacyMigrationJournal(input.userDataPath, {
        allowRecoveryMutation: false
      })
    } catch (error) {
      if (
        !(error instanceof LegacyMigrationBlockedError) ||
        error.code !== 'host_filesystem_authority_unavailable'
      ) {
        throw error
      }
      recoveryMutationRequired = true
    }

    if (preliminaryExisting && journalLastState(preliminaryExisting) === 'active') {
      assertScope(preliminaryExisting.plan)
      return activeResult(preliminaryExisting, 'ready-existing')
    }

    const preliminaryPlan = preliminaryExisting?.plan ?? (recoveryMutationRequired ? null : makePlan())
    if (!preliminaryPlan && !recoveryMutationRequired) {
      return readyExistingResult(input.userDataPath)
    }
    if (preliminaryPlan) {
      assertScope(preliminaryPlan)
      assertAbsentMappingDecisionsCurrent(preliminaryPlan)
      assertQuiescenceAuthority(preliminaryPlan, input.quiescenceAuthority)
    } else {
      // Empty-root and unpublished-record recovery mutates the journal
      // namespace before a plan can be loaded, so even that cleanup requires a
      // live native host transaction first.
      assertHostFilesystemAuthorityPrewrite(
        input.quiescenceAuthority?.filesystem,
        [dirname(input.userDataPath)]
      )
    }

    const lease = acquireLegacyMigrationLease({
      userDataPath: input.userDataPath,
      isProcessAlive: input.testHooks?.isProcessAlive,
      now: input.testHooks?.now,
      token: () => transactionId
    })
    try {
      const existing = loadLegacyMigrationJournal(input.userDataPath)
      if (existing) {
        assertScope(existing.plan)
        if (journalLastState(existing) !== 'active') {
          assertQuiescenceAuthority(existing.plan, input.quiescenceAuthority)
        }
        return resumeJournal({
          journal: existing,
          homeDir: input.homeDir,
          mappings,
          log,
          hooks: input.testHooks,
          status: 'recovered',
          quiescenceAuthority: input.quiescenceAuthority
        })
      }

      const plan = makePlan()
      if (!plan) return readyExistingResult(input.userDataPath)
      if (preliminaryPlan && stableJson(plan) !== stableJson(preliminaryPlan)) {
        migrationBlocked('legacy_source_changed')
      }
      assertScope(plan)
      assertAbsentMappingDecisionsCurrent(plan)
      assertQuiescenceAuthority(plan, input.quiescenceAuthority)

      const journal = createLegacyMigrationJournal(plan)
      input.testHooks?.checkpoint?.('after-plan')
      return resumeJournal({
        journal,
        homeDir: input.homeDir,
        mappings,
        log,
        hooks: input.testHooks,
        status: 'migrated',
        quiescenceAuthority: input.quiescenceAuthority
      })
    } finally {
      lease.release()
    }
  } catch (error) {
    if (error instanceof LegacyMigrationBlockedError) {
      log('legacy-migration: startup blocked', { code: error.code })
      throw error
    }
    throw new LegacyMigrationBlockedError('io_error', error)
  }
}
