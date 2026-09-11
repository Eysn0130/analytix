import { createHash } from 'node:crypto'
import { z } from 'zod'
import {
  providerRegistryPublicProviderSchemaV1,
  providerRegistrySnapshotResponseSchemaV1,
  type ProviderRegistryProviderInputV1,
  type ProviderRegistryPublicProviderV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1,
  type JsonSettingsStore,
  type LegacyProviderCredentialCandidateSnapshot,
  type LegacyProviderCredentialCleanupRequestV1,
  type LegacyProviderCredentialLiveSourceAuthorityV1,
  type LegacyProviderCredentialInspection
} from './settings-store.js'
import type {
  ProviderRegistryLegacyMigrationCommitRequest,
  ProviderRegistryLegacyMigrationFinalizeRequest,
  ProviderRegistryLegacyMigrationInventorySuccess,
  ProviderRegistryLegacyMigrationPrepareRequest,
  ProviderRegistryLegacyMigrationRollbackBeginRequest,
  ProviderRegistryLegacyMigrationRollbackCommitRequest,
  ProviderRegistryLegacyMigrationSourceAuthorityChallengeRequest
} from './provider-registry-legacy-migration.js'

const CREDENTIAL_PURPOSE = 'provider-api-key'
const COMMIT_CONFIRMATION = 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const ROLLBACK_CONFIRMATION = 'BEGIN_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK'
const ROLLBACK_COMMIT_CONFIRMATION = 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK'
const FINALIZE_CONFIRMATION = 'FINALIZE_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const SOURCE_AUTHORITY_CHALLENGE_CONFIRMATION = 'ISSUE_PROVIDER_SETTINGS_SOURCE_AUTHORITY_CHALLENGE'
const MAX_CANDIDATES = 64
const identifierPattern = /^[a-z0-9][a-z0-9._-]{0,95}$/
const sourceSHA256Pattern = /^[0-9a-f]{64}$/
const sourceLocatorPattern = /^(?:current:analytix-settings\.json|compatibility:[0-9]{2}:(?:analytix-settings|kun-settings)\.json)$/
const commitOrderPattern = /^(?:0|[1-9]\d*)$/

const recoveryCredentialRefSchema = z.string().regex(/^cred_[A-Za-z0-9_-]{43}$/)
const prepareSuccessSchema = z.discriminatedUnion('status', [
  z.object({
    schemaVersion: z.literal(1),
    status: z.literal('VERIFIED_RECOVERY'),
    migrationId: z.string().regex(identifierPattern),
    recoveryCredentialRef: recoveryCredentialRefSchema,
    safeToProceedWithProviderMigration: z.literal(true)
  }).strict(),
  z.object({
    schemaVersion: z.literal(1),
    status: z.literal('PROVIDER_COMMITTED_RECOVERY_RETAINED'),
    migrationId: z.string().regex(identifierPattern),
    recoveryCredentialRef: recoveryCredentialRefSchema,
    safeToProceedWithProviderMigration: z.literal(false)
  }).strict()
])
const commitSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('PROVIDER_COMMITTED_RECOVERY_RETAINED'),
  migrationId: z.string().regex(identifierPattern),
  safeToRemoveLegacyPlaintext: z.literal(true),
  provider: providerRegistryPublicProviderSchemaV1
}).strict()

type RegistrySnapshotReader = () => Promise<unknown>
type LegacyMigrationClient = {
  prepare(input: ProviderRegistryLegacyMigrationPrepareRequest): Promise<unknown>
  commit(input: ProviderRegistryLegacyMigrationCommitRequest): Promise<unknown>
  inventory(): Promise<unknown>
  finalize(input: ProviderRegistryLegacyMigrationFinalizeRequest): Promise<unknown>
  issueSourceAuthorityChallenge(
    input: ProviderRegistryLegacyMigrationSourceAuthorityChallengeRequest
  ): Promise<unknown>
}

type LegacyMigrationRollbackClient = {
  beginRollback(input: ProviderRegistryLegacyMigrationRollbackBeginRequest): Promise<unknown>
  commitRollback(input: ProviderRegistryLegacyMigrationRollbackCommitRequest): Promise<unknown>
  finalize(input: ProviderRegistryLegacyMigrationFinalizeRequest): Promise<unknown>
  issueSourceAuthorityChallenge(
    input: ProviderRegistryLegacyMigrationSourceAuthorityChallengeRequest
  ): Promise<unknown>
}

const sourceAuthorityChallengeSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  challenge: z.string().regex(/^lmsa_[A-Za-z0-9_-]{43}$/)
}).strict()

const rollbackBeginSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum([
    'CLEANED_SOURCE_AUTHORITY_REQUIRED',
    'ROLLBACK_COMMITTED_RECOVERY_RETAINED',
    'ROLLBACK_RECOVERY_RETAINED',
    'PRE_COMMIT_ROLLBACK_COMPLETED',
    'ALREADY_FINALIZED'
  ]),
  migrationId: z.string().regex(identifierPattern)
}).strict()
const rollbackCommitSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum(['ROLLBACK_COMMITTED_RECOVERY_RETAINED', 'ALREADY_FINALIZED']),
  migrationId: z.string().regex(identifierPattern)
}).strict()
const finalizeSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum(['COMPLETED', 'ALREADY_FINALIZED']),
  outcome: z.enum(['MIGRATION_COMMITTED', 'PROTECTED_RECOVERY_RETAINED']),
  migrationId: z.string().regex(identifierPattern)
}).strict()
const inventorySuccessSchema = z.object({
  schemaVersion: z.literal(1),
  recoveries: z.array(z.object({
    migrationId: z.string().regex(identifierPattern),
    providerId: z.string().regex(identifierPattern),
    sourceLocator: z.string().regex(sourceLocatorPattern),
    sourceSHA256: z.string().regex(sourceSHA256Pattern),
    expectedCleanedSourceSHA256: z.string().regex(sourceSHA256Pattern),
    sourcePhysicalIdentitySHA256: z.string().regex(sourceSHA256Pattern),
    recoveryCredentialRef: recoveryCredentialRefSchema,
    phase: z.enum([
      'prepared',
      'secret-durable',
      'verified',
      'provider-commit-prepared',
      'provider-committed-recovery-retained',
      'rollback-cleaned-source-authority-pending',
      'rollback-registry-commit-pending',
      'rollback-committed-recovery-retained',
      'rollback-recovery-retained',
      'finalizing-committed',
      'finalizing-rollback'
    ]),
    commitOrder: z.string().regex(commitOrderPattern)
  }).strict()).max(256)
}).strict()

export type ProviderRegistryLegacyMigrationOrchestrationFailurePhase =
  'inspection' | 'candidate' | 'inventory' | 'snapshot' | 'prepare' | 'commit' | 'cleanup' | 'finalize'

export type ProviderRegistryLegacyMigrationOrchestrationResultV1 = {
  schemaVersion: 1
  status: 'COMPLETED' | 'FAILED'
  candidateCount: number
  completedCandidateCount: number
  committedCandidateCount: number
  retainedReplayCount: number
  failurePhase?: ProviderRegistryLegacyMigrationOrchestrationFailurePhase
}

type OrchestrationInput = {
  settingsStore: Pick<
    JsonSettingsStore,
    'inspectLegacyProviderCredentialSources' | 'cleanupLegacyProviderCredentialSource' |
    'withLegacyProviderCredentialSourceAuthority'
  >
  readRegistrySnapshot: RegistrySnapshotReader
  migrationClient: LegacyMigrationClient
}

type MutableCounts = {
  candidateCount: number
  completedCandidateCount: number
  committedCandidateCount: number
  retainedReplayCount: number
}

function result(
  counts: MutableCounts,
  failurePhase?: ProviderRegistryLegacyMigrationOrchestrationFailurePhase
): ProviderRegistryLegacyMigrationOrchestrationResultV1 {
  return {
    schemaVersion: 1,
    status: failurePhase === undefined ? 'COMPLETED' : 'FAILED',
    ...counts,
    ...(failurePhase === undefined ? {} : { failurePhase })
  }
}

function clearInspectionCredentials(inspection: LegacyProviderCredentialInspection | undefined): void {
  if (!inspection) return
  if (inspection.sourceSnapshot instanceof Uint8Array) inspection.sourceSnapshot.fill(0)
  if (!Array.isArray(inspection.candidates)) return
  for (const candidate of inspection.candidates) {
    if (candidate?.credential instanceof Uint8Array) candidate.credential.fill(0)
    if (!Array.isArray(candidate?.rollbackCredentialArtifacts)) continue
    for (const artifact of candidate.rollbackCredentialArtifacts) {
      if (artifact?.credential instanceof Uint8Array) artifact.credential.fill(0)
    }
  }
}

function bytesEqual(left: Uint8Array, right: Uint8Array): boolean {
  if (left.byteLength !== right.byteLength) return false
  let difference = 0
  for (let index = 0; index < left.byteLength; index += 1) difference |= left[index] ^ right[index]
  return difference === 0
}

async function withOwnedVerifiedSource<T>(
  source: Uint8Array,
  action: (ownedSource: Uint8Array<ArrayBuffer>) => Promise<T>
): Promise<T> {
  const ownedSource = Uint8Array.from(source)
  try {
    return await action(ownedSource)
  } finally {
    ownedSource.fill(0)
  }
}

function validLocatorList(value: unknown): value is string[] {
  return Array.isArray(value) && value.length > 0 && value.length <= 256 &&
    value.every((locator) => typeof locator === 'string' && locator.length > 0) &&
    new Set(value).size === value.length
}

function validCandidateInventory(candidate: LegacyProviderCredentialCandidateSnapshot): boolean {
  if (candidate.schemaVersion !== 1 || !identifierPattern.test(candidate.migrationId) ||
    !identifierPattern.test(candidate.providerId) || typeof candidate.sourceLocator !== 'string' ||
    !(candidate.credential instanceof Uint8Array) || candidate.credential.byteLength === 0 ||
    !validLocatorList(candidate.credentialLocators) ||
    candidate.sourceLocator !== candidate.credentialLocators[0] ||
    !candidate.providerMetadata || typeof candidate.providerMetadata.activeProviderId !== 'string' ||
    typeof candidate.providerMetadata.runtimeModel !== 'string' ||
    !candidate.providerMetadata.proxy || typeof candidate.providerMetadata.proxy.enabled !== 'boolean' ||
    typeof candidate.providerMetadata.proxy.url !== 'string' ||
    !candidate.providerMetadata.profile || !Array.isArray(candidate.rollbackCredentialArtifacts) ||
    candidate.rollbackCredentialArtifacts.length > 64) {
    return false
  }
  const locators = new Set(candidate.credentialLocators)
  const credentials = [candidate.credential]
  for (const artifact of candidate.rollbackCredentialArtifacts) {
    if (artifact.schemaVersion !== 1 || typeof artifact.sourceLocator !== 'string' ||
      !(artifact.credential instanceof Uint8Array) || artifact.credential.byteLength === 0 ||
      !validLocatorList(artifact.credentialLocators) ||
      artifact.sourceLocator !== artifact.credentialLocators[0] ||
      artifact.credentialLocators.some((locator) => locators.has(locator)) ||
      credentials.some((credential) => bytesEqual(credential, artifact.credential))) {
      return false
    }
    artifact.credentialLocators.forEach((locator) => locators.add(locator))
    credentials.push(artifact.credential)
  }
  return true
}

function orderedCandidates(
  inspection: LegacyProviderCredentialInspection
): LegacyProviderCredentialCandidateSnapshot[] | null {
  const hasSource = typeof inspection.sourceLocator === 'string' &&
    typeof inspection.sourceSHA256 === 'string' && sourceSHA256Pattern.test(inspection.sourceSHA256) &&
    typeof inspection.expectedCleanedSourceSHA256 === 'string' &&
    sourceSHA256Pattern.test(inspection.expectedCleanedSourceSHA256) &&
    typeof inspection.sourcePhysicalIdentitySHA256 === 'string' &&
    sourceSHA256Pattern.test(inspection.sourcePhysicalIdentitySHA256) &&
    inspection.sourceSnapshot instanceof Uint8Array && inspection.sourceSnapshot.byteLength > 0
  const hasNoSource = inspection.sourceLocator === null && inspection.sourceSHA256 === null &&
    inspection.expectedCleanedSourceSHA256 === null && inspection.sourcePhysicalIdentitySHA256 === null &&
    inspection.sourceSnapshot === null
  if (inspection.schemaVersion !== 1 || !Array.isArray(inspection.candidates) ||
    inspection.candidates.length > MAX_CANDIDATES ||
    (!hasSource && !hasNoSource) || (inspection.candidates.length > 0 && !hasSource)) {
    return null
  }
  if (inspection.candidates.length === 0) return []
  if (!inspection.candidates.every(validCandidateInventory)) return null
  const providerIds = inspection.candidates.map((candidate) => candidate.providerId)
  const migrationIds = inspection.candidates.map((candidate) => candidate.migrationId)
  if (new Set(providerIds).size !== providerIds.length || new Set(migrationIds).size !== migrationIds.length) {
    return null
  }
  const activeProviderIds = new Set(
    inspection.candidates.map((candidate) => candidate.providerMetadata.activeProviderId)
  )
  if (activeProviderIds.size !== 1) return null
  const activeProviderId = inspection.candidates[0].providerMetadata.activeProviderId
  const preferredProviderId = activeProviderId || (providerIds.includes('deepseek') ? 'deepseek' : '')
  return inspection.candidates
    .map((candidate, index) => ({ candidate, index }))
    .sort((left, right) => {
      const leftPreferred = left.candidate.providerId === preferredProviderId
      const rightPreferred = right.candidate.providerId === preferredProviderId
      if (leftPreferred !== rightPreferred) return leftPreferred ? -1 : 1
      if (left.candidate.providerId !== right.candidate.providerId) {
        return left.candidate.providerId < right.candidate.providerId ? -1 : 1
      }
      return left.index - right.index
    })
    .map(({ candidate }) => candidate)
}

function validSourceReceipt(
  receipt: unknown,
  expected: {
    sourceLocator: string
    sourceSHA256: string
    verifiedSourceSHA256: string
    sourcePhysicalIdentitySHA256: string
  }
): receipt is {
  schemaVersion: 1
  sourceLocator: string
  sourceSHA256: string
  verifiedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  verifiedSource: Uint8Array
} {
  if (typeof receipt !== 'object' || receipt === null) return false
  const value = receipt as Record<string, unknown>
  return value.schemaVersion === 1 && value.sourceLocator === expected.sourceLocator &&
    value.sourceSHA256 === expected.sourceSHA256 &&
    value.verifiedSourceSHA256 === expected.verifiedSourceSHA256 &&
    value.sourcePhysicalIdentitySHA256 === expected.sourcePhysicalIdentitySHA256 &&
    value.verifiedSource instanceof Uint8Array && value.verifiedSource.byteLength > 0 &&
    value.verifiedSource.byteLength <= 256 * 1024 &&
    createHash('sha256').update(value.verifiedSource).digest('hex') === expected.verifiedSourceSHA256
}

const sourceAuthorityFailure = Symbol('source-authority-failure')

async function withFreshLegacyProviderSourceAuthority(
  input: {
    operation: 'rollback-commit' | 'finalize'
    migrationId: string
    sourceLocator: string
    sourceSHA256: string
    currentSourceSHA256: string
    sourcePhysicalIdentitySHA256: string
    recoveryCredentialRef: string
    settingsStore: Pick<JsonSettingsStore, 'withLegacyProviderCredentialSourceAuthority'>
    migrationClient: Pick<LegacyMigrationClient, 'issueSourceAuthorityChallenge'>
  },
  action: (authority: LegacyProviderCredentialLiveSourceAuthorityV1) => Promise<unknown>
): Promise<unknown | typeof sourceAuthorityFailure> {
  let challengeRaw: unknown
  try {
    challengeRaw = await input.migrationClient.issueSourceAuthorityChallenge({
      schemaVersion: 1,
      operation: input.operation,
      migrationId: input.migrationId,
      sourceLocator: input.sourceLocator,
      sourceSHA256: input.sourceSHA256,
      currentSourceSHA256: input.currentSourceSHA256,
      sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
      recoveryCredentialRef: input.recoveryCredentialRef,
      confirmation: SOURCE_AUTHORITY_CHALLENGE_CONFIRMATION
    })
  } catch {
    return sourceAuthorityFailure
  }
  const challenge = sourceAuthorityChallengeSuccessSchema.safeParse(challengeRaw)
  if (!challenge.success) return sourceAuthorityFailure
  try {
    return await input.settingsStore.withLegacyProviderCredentialSourceAuthority({
      schemaVersion: 1,
      challenge: challenge.data.challenge,
      operation: input.operation,
      migrationId: input.migrationId,
      sourceLocator: input.sourceLocator,
      sourceSHA256: input.sourceSHA256,
      currentSourceSHA256: input.currentSourceSHA256,
      sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256
    }, action)
  } catch {
    return sourceAuthorityFailure
  }
}

function cleanupRequest(
  inspection: LegacyProviderCredentialInspection,
  candidates: LegacyProviderCredentialCandidateSnapshot[]
): LegacyProviderCredentialCleanupRequestV1 | null {
  if (candidates.length === 0 || inspection.sourceLocator === null || inspection.sourceSHA256 === null) {
    return null
  }
  const protectedCredentialLocators = candidates.flatMap((candidate) => [
    ...candidate.credentialLocators,
    ...candidate.rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
  ]).sort()
  if (new Set(protectedCredentialLocators).size !== protectedCredentialLocators.length) return null
  return {
    schemaVersion: 1,
    purpose: 'remove-verified-legacy-provider-credentials',
    confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
    sourceLocator: inspection.sourceLocator,
    sourceSHA256: inspection.sourceSHA256,
    expectedCleanedSourceSHA256: inspection.expectedCleanedSourceSHA256 as string,
    protectedCredentialLocators
  }
}

function providerMatchesMigration(
  provider: ProviderRegistryPublicProviderV1,
  expected: ProviderRegistryProviderInputV1
): boolean {
  return provider.id === expected.id && provider.kind === expected.kind &&
    provider.endpoint === expected.endpoint && (provider.proxy ?? '') === expected.proxy &&
    JSON.stringify(provider.models) === JSON.stringify(expected.models) &&
    JSON.stringify(provider.mediaModels) === JSON.stringify(expected.mediaModels) &&
    (provider.selectedModel ?? '') === expected.selectedModel &&
    (provider.selectedMediaModel ?? '') === expected.selectedMediaModel &&
    JSON.stringify(provider.selectedRoutes) === JSON.stringify(expected.selectedRoutes) &&
    provider.credentialConfigured && provider.credentialPurpose === CREDENTIAL_PURPOSE &&
    provider.revision === '1' && provider.generation === '1' && !provider.tombstone
}

export async function orchestrateProviderRegistryLegacyMigration(
  input: OrchestrationInput
): Promise<ProviderRegistryLegacyMigrationOrchestrationResultV1> {
  const counts: MutableCounts = {
    candidateCount: 0,
    completedCandidateCount: 0,
    committedCandidateCount: 0,
    retainedReplayCount: 0
  }
  let inspection: LegacyProviderCredentialInspection | undefined
  try {
    try {
      inspection = await input.settingsStore.inspectLegacyProviderCredentialSources()
    } catch {
      return result(counts, 'inspection')
    }
    counts.candidateCount = Math.min(
      Array.isArray(inspection?.candidates) ? inspection.candidates.length : 0,
      MAX_CANDIDATES
    )
    let candidates = orderedCandidates(inspection)
    if (candidates === null) return result(counts, 'candidate')
    let cleanup = cleanupRequest(inspection, candidates)
    if (cleanup === null && candidates.length > 0) return result(counts, 'candidate')
    let inventoryRaw: unknown
    try {
      inventoryRaw = await input.migrationClient.inventory()
    } catch {
      return result(counts, 'inventory')
    }
    const inventory = inventorySuccessSchema.safeParse(inventoryRaw)
    if (!inventory.success) return result(counts, 'inventory')
    if (candidates.length === 0 && cleanup === null && inventory.data.recoveries.length === 0) {
      return result(counts)
    }
    const inventorySourceLocators = new Set(
      inventory.data.recoveries.map((recovery) => recovery.sourceLocator)
    )
    if (inventorySourceLocators.size > 1) return result(counts, 'candidate')
    if (inventory.data.recoveries.length > 0 &&
      (inspection.sourceLocator === null || !inventorySourceLocators.has(inspection.sourceLocator))) {
      if (candidates.length > 0) return result(counts, 'candidate')
      const exactSourceLocator = inventorySourceLocators.values().next().value
      if (typeof exactSourceLocator !== 'string') return result(counts, 'candidate')
      clearInspectionCredentials(inspection)
      try {
        inspection = await input.settingsStore.inspectLegacyProviderCredentialSources(exactSourceLocator)
      } catch {
        return result(counts, 'inspection')
      }
      candidates = orderedCandidates(inspection)
      if (candidates === null) return result(counts, 'candidate')
      counts.candidateCount = candidates.length
      cleanup = cleanupRequest(inspection, candidates)
      if (cleanup === null && candidates.length > 0) return result(counts, 'candidate')
    }
    if (inspection.sourceLocator === null || inspection.sourceSHA256 === null ||
      inspection.expectedCleanedSourceSHA256 === null) return result(counts, 'candidate')
    const sourceLocator = inspection.sourceLocator as string
    const sourceSHA256 = inspection.sourceSHA256 as string
    const expectedCleanedSourceSHA256 = inspection.expectedCleanedSourceSHA256 as string
    const sourceRecoveries = inventory.data.recoveries.filter(
      (recovery) => recovery.sourceLocator === sourceLocator
    )
    if (sourceRecoveries.some((recovery) => (
      recovery.sourceSHA256 !== sourceRecoveries[0].sourceSHA256 ||
      recovery.expectedCleanedSourceSHA256 !== sourceRecoveries[0].expectedCleanedSourceSHA256 ||
      recovery.sourcePhysicalIdentitySHA256 !== sourceRecoveries[0].sourcePhysicalIdentitySHA256
    ))) return result(counts, 'candidate')
    if (sourceRecoveries.length > 0) {
      const recoverySourceSHA256 = sourceRecoveries[0].sourceSHA256
      const recoveryCleanedSourceSHA256 = sourceRecoveries[0].expectedCleanedSourceSHA256
      const recoveryPhysicalIdentitySHA256 = sourceRecoveries[0].sourcePhysicalIdentitySHA256
      if (sourceSHA256 === recoveryCleanedSourceSHA256) {
        if (candidates.length !== 0 || expectedCleanedSourceSHA256 !== sourceSHA256 ||
          inspection.sourcePhysicalIdentitySHA256 !== recoveryPhysicalIdentitySHA256 ||
          sourceRecoveries.some((recovery) => ![
            'provider-committed-recovery-retained',
            'finalizing-committed'
        ].includes(recovery.phase))) return result(counts, 'candidate')
        const finalizedRaw = await withFreshLegacyProviderSourceAuthority({
          operation: 'finalize',
          migrationId: sourceRecoveries[0].migrationId,
          sourceLocator,
          sourceSHA256: recoverySourceSHA256,
          currentSourceSHA256: sourceSHA256,
          sourcePhysicalIdentitySHA256: recoveryPhysicalIdentitySHA256,
          recoveryCredentialRef: sourceRecoveries[0].recoveryCredentialRef,
          settingsStore: input.settingsStore,
          migrationClient: input.migrationClient
        }, async (authority) => withOwnedVerifiedSource(
          authority.verifiedSource,
          async (verifiedSource) => input.migrationClient.finalize({
            schemaVersion: 1,
            migrationId: sourceRecoveries[0].migrationId,
            sourceLocator,
            sourceSHA256: recoverySourceSHA256,
            verifiedSourceSHA256: authority.verifiedSourceSHA256,
            sourcePhysicalIdentitySHA256: authority.sourcePhysicalIdentitySHA256,
            verifiedSource,
            sourceAuthority: authority.sourceAuthority,
            recoveryCredentialRef: sourceRecoveries[0].recoveryCredentialRef,
            confirmation: FINALIZE_CONFIRMATION
          })
        ))
        if (finalizedRaw === sourceAuthorityFailure) return result(counts, 'finalize')
        const finalized = finalizeSuccessSchema.safeParse(finalizedRaw)
        return finalized.success && finalized.data.migrationId === sourceRecoveries[0].migrationId &&
          finalized.data.outcome === 'MIGRATION_COMMITTED'
          ? result(counts)
          : result(counts, 'finalize')
      }
      if (sourceSHA256 !== recoverySourceSHA256 ||
        expectedCleanedSourceSHA256 !== recoveryCleanedSourceSHA256 ||
        inspection.sourcePhysicalIdentitySHA256 !== recoveryPhysicalIdentitySHA256) {
        return result(counts, 'candidate')
      }
      const candidateIDs = new Set(candidates.map((candidate) => candidate.migrationId))
      if (sourceRecoveries.some((recovery) => !candidateIDs.has(recovery.migrationId))) {
        return result(counts, 'candidate')
      }
    }
    let finalizationAnchor: {
      migrationId: string
      recoveryCredentialRef: string
    } | undefined = sourceRecoveries[0]

    for (const candidate of candidates) {
      let snapshotRaw: unknown
      try {
        snapshotRaw = await input.readRegistrySnapshot()
      } catch {
        return result(counts, 'snapshot')
      }
      const snapshot = providerRegistrySnapshotResponseSchemaV1.safeParse(snapshotRaw)
      if (!snapshot.success) return result(counts, 'snapshot')
      const existing = snapshot.data.providers.find((provider) => provider.id === candidate.providerId)
      const expected = {
        registryRevision: snapshot.data.registryRevision,
        registryIncarnation: snapshot.data.registryIncarnation,
        providerRevision: existing?.revision ?? '0',
        providerGeneration: existing?.generation ?? '0',
        providerIncarnation: existing?.incarnation ?? '',
        providerCredentialPurpose: existing?.credentialPurpose ?? ''
      }
      let provider: ProviderRegistryProviderInputV1
      try {
        provider = mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(candidate)
      } catch {
        return result(counts, 'candidate')
      }
      const prepareInput: ProviderRegistryLegacyMigrationPrepareRequest = {
        schemaVersion: 1,
        expected,
        migrationId: candidate.migrationId,
        sourceLocator: inspection.sourceLocator as string,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256: inspection.sourcePhysicalIdentitySHA256 as string,
        provider,
        credentialPurpose: CREDENTIAL_PURPOSE,
        credential: candidate.credential,
        sourceSnapshot: inspection.sourceSnapshot as Uint8Array,
        activeCredentialLocators: [...candidate.credentialLocators],
        rollbackCredentialArtifacts: candidate.rollbackCredentialArtifacts.map((artifact) => ({
          schemaVersion: 1,
          credentialLocators: [...artifact.credentialLocators],
          credential: artifact.credential
        }))
      }
      let prepareRaw: unknown
      try {
        prepareRaw = await input.migrationClient.prepare(prepareInput)
      } catch {
        return result(counts, 'prepare')
      }
      const prepare = prepareSuccessSchema.safeParse(prepareRaw)
      if (!prepare.success || prepare.data.migrationId !== candidate.migrationId) {
        return result(counts, 'prepare')
      }
      finalizationAnchor ??= {
        migrationId: prepare.data.migrationId,
        recoveryCredentialRef: prepare.data.recoveryCredentialRef
      }
      if (prepare.data.status === 'PROVIDER_COMMITTED_RECOVERY_RETAINED') {
        if (existing === undefined || !providerMatchesMigration(existing, provider)) {
          return result(counts, 'prepare')
        }
        counts.completedCandidateCount += 1
        counts.retainedReplayCount += 1
        continue
      }
      if (existing !== undefined) return result(counts, 'prepare')

      const commitInput: ProviderRegistryLegacyMigrationCommitRequest = {
        schemaVersion: 1,
        expected,
        expectedSelectedProviderID: snapshot.data.selectedProviderId ?? '',
        migrationId: candidate.migrationId,
        sourceLocator: inspection.sourceLocator as string,
        sourceSHA256,
        recoveryCredentialRef: prepare.data.recoveryCredentialRef,
        confirmation: COMMIT_CONFIRMATION
      }
      let commitRaw: unknown
      try {
        commitRaw = await input.migrationClient.commit(commitInput)
      } catch {
        return result(counts, 'commit')
      }
      const commit = commitSuccessSchema.safeParse(commitRaw)
      if (!commit.success || commit.data.migrationId !== candidate.migrationId ||
        !providerMatchesMigration(commit.data.provider, provider)) {
        return result(counts, 'commit')
      }
      counts.completedCandidateCount += 1
      counts.committedCandidateCount += 1
    }
    let cleanupReceipt: Awaited<ReturnType<JsonSettingsStore['cleanupLegacyProviderCredentialSource']>> | undefined
    try {
      cleanupReceipt = await input.settingsStore.cleanupLegacyProviderCredentialSource(cleanup!)
      if (cleanupReceipt.cleanedSourceSHA256 !== expectedCleanedSourceSHA256 ||
        !validSourceReceipt(cleanupReceipt, {
          sourceLocator,
          sourceSHA256,
          verifiedSourceSHA256: expectedCleanedSourceSHA256,
          sourcePhysicalIdentitySHA256: inspection.sourcePhysicalIdentitySHA256 as string
        })) {
        return result(counts, 'cleanup')
      }
    } catch {
      return result(counts, 'cleanup')
    }
    if (!finalizationAnchor) return result(counts, 'finalize')
    cleanupReceipt.verifiedSource.fill(0)
    const finalizedRaw = await withFreshLegacyProviderSourceAuthority({
      operation: 'finalize',
      migrationId: finalizationAnchor.migrationId,
      sourceLocator,
      sourceSHA256,
      currentSourceSHA256: expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256: cleanupReceipt.sourcePhysicalIdentitySHA256,
      recoveryCredentialRef: finalizationAnchor.recoveryCredentialRef,
      settingsStore: input.settingsStore,
      migrationClient: input.migrationClient
    }, async (authority) => withOwnedVerifiedSource(
      authority.verifiedSource,
      async (verifiedSource) => input.migrationClient.finalize({
        schemaVersion: 1,
        migrationId: finalizationAnchor.migrationId,
        sourceLocator,
        sourceSHA256,
        verifiedSourceSHA256: authority.verifiedSourceSHA256,
        sourcePhysicalIdentitySHA256: authority.sourcePhysicalIdentitySHA256,
        verifiedSource,
        sourceAuthority: authority.sourceAuthority,
        recoveryCredentialRef: finalizationAnchor.recoveryCredentialRef,
        confirmation: FINALIZE_CONFIRMATION
      })
    ))
    if (finalizedRaw === sourceAuthorityFailure) return result(counts, 'finalize')
    const finalized = finalizeSuccessSchema.safeParse(finalizedRaw)
    return finalized.success && finalized.data.migrationId === finalizationAnchor.migrationId &&
      finalized.data.outcome === 'MIGRATION_COMMITTED'
      ? result(counts)
      : result(counts, 'finalize')
  } catch {
    return result(counts, 'candidate')
  } finally {
    clearInspectionCredentials(inspection)
  }
}

export type ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1 = {
  schemaVersion: 1
  status: 'PRE_COMMIT_COMPLETED' | 'ROLLBACK_COMMITTED' | 'ALREADY_FINALIZED' | 'FAILED'
  failurePhase?: 'begin' | 'source' | 'snapshot' | 'commit'
}

type RollbackOrchestrationInput = {
  migrationId: string
  providerId: string
  sourceLocator: string
  sourceSHA256: string
  expectedCleanedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  recoveryCredentialRef: string
  commitOrder: string
  settingsStore: Pick<JsonSettingsStore, 'withLegacyProviderCredentialSourceAuthority'>
  readRegistrySnapshot: RegistrySnapshotReader
  migrationClient: Pick<
    LegacyMigrationRollbackClient,
    'beginRollback' | 'commitRollback' | 'issueSourceAuthorityChallenge'
  >
}

type RollbackGroupOrchestrationInput = {
  recoveriesInCommitOrder: Array<{
    migrationId: string
    providerId: string
    recoveryCredentialRef: string
    commitOrder: string
  }>
  sourceLocator: string
  sourceSHA256: string
  expectedCleanedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  settingsStore: Pick<JsonSettingsStore, 'withLegacyProviderCredentialSourceAuthority'>
  readRegistrySnapshot: RegistrySnapshotReader
  migrationClient: Pick<
    LegacyMigrationRollbackClient,
    'beginRollback' | 'commitRollback' | 'issueSourceAuthorityChallenge'
  >
}

function rollbackResult(
  status: ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1['status'],
  failurePhase?: ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1['failurePhase']
): ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1 {
  return { schemaVersion: 1, status, ...(failurePhase === undefined ? {} : { failurePhase }) }
}

export async function orchestrateProviderRegistryLegacyMigrationRollback(
  input: RollbackOrchestrationInput
): Promise<ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1> {
  return orchestrateProviderRegistryLegacyMigrationRollbackGroup({
    recoveriesInCommitOrder: [{
      migrationId: input.migrationId,
      providerId: input.providerId,
      recoveryCredentialRef: input.recoveryCredentialRef,
      commitOrder: input.commitOrder
    }],
    sourceLocator: input.sourceLocator,
    sourceSHA256: input.sourceSHA256,
    expectedCleanedSourceSHA256: input.expectedCleanedSourceSHA256,
    sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
    settingsStore: input.settingsStore,
    readRegistrySnapshot: input.readRegistrySnapshot,
    migrationClient: input.migrationClient
  })
}

export async function orchestrateProviderRegistryLegacyMigrationRollbackGroup(
  input: RollbackGroupOrchestrationInput
): Promise<ProviderRegistryLegacyMigrationRollbackOrchestrationResultV1> {
  const targets = input.recoveriesInCommitOrder
  if (!sourceLocatorPattern.test(input.sourceLocator) || !sourceSHA256Pattern.test(input.sourceSHA256) ||
    !sourceSHA256Pattern.test(input.expectedCleanedSourceSHA256) ||
    !sourceSHA256Pattern.test(input.sourcePhysicalIdentitySHA256) ||
    targets.length === 0 ||
    targets.length > MAX_CANDIDATES || targets.some((target) => (
      !identifierPattern.test(target.migrationId) || !identifierPattern.test(target.providerId) ||
      !recoveryCredentialRefSchema.safeParse(target.recoveryCredentialRef).success ||
      !commitOrderPattern.test(target.commitOrder)
    )) || new Set(targets.map((target) => target.migrationId)).size !== targets.length ||
    new Set(targets.map((target) => target.providerId)).size !== targets.length ||
    new Set(targets.map((target) => target.recoveryCredentialRef)).size !== targets.length ||
    targets.some((target, index) => index > 0 && BigInt(target.commitOrder) <= BigInt(targets[index - 1].commitOrder))) {
    return rollbackResult('FAILED', 'begin')
  }
  const pendingCommit = new Set<string>()
  const alreadyCommitted = new Set<string>()
  const preCommitCompleted = new Set<string>()
  let terminalCount = 0
  for (const target of targets) {
    let beginRaw: unknown
    try {
      beginRaw = await input.migrationClient.beginRollback({
        schemaVersion: 1,
        migrationId: target.migrationId,
        sourceLocator: input.sourceLocator,
        sourceSHA256: input.sourceSHA256,
        sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
        recoveryCredentialRef: target.recoveryCredentialRef,
        confirmation: ROLLBACK_CONFIRMATION
      })
    } catch {
      return rollbackResult('FAILED', 'begin')
    }
    const begin = rollbackBeginSuccessSchema.safeParse(beginRaw)
    if (!begin.success || begin.data.migrationId !== target.migrationId) {
      return rollbackResult('FAILED', 'begin')
    }
    switch (begin.data.status) {
      case 'CLEANED_SOURCE_AUTHORITY_REQUIRED':
        pendingCommit.add(target.migrationId)
        break
      case 'ROLLBACK_COMMITTED_RECOVERY_RETAINED':
        alreadyCommitted.add(target.migrationId)
        break
      case 'PRE_COMMIT_ROLLBACK_COMPLETED':
        preCommitCompleted.add(target.migrationId)
        break
      case 'ROLLBACK_RECOVERY_RETAINED':
      case 'ALREADY_FINALIZED':
        terminalCount += 1
        break
    }
  }
  if (terminalCount !== 0) {
    return terminalCount === targets.length
      ? rollbackResult('ALREADY_FINALIZED')
      : rollbackResult('FAILED', 'begin')
  }
  if (pendingCommit.size === 0) {
    return preCommitCompleted.size === targets.length
      ? rollbackResult('PRE_COMMIT_COMPLETED')
      : rollbackResult('ROLLBACK_COMMITTED')
  }

  for (const target of [...targets].reverse()) {
    if (!pendingCommit.has(target.migrationId) || alreadyCommitted.has(target.migrationId) ||
      preCommitCompleted.has(target.migrationId)) continue
    let liveFailurePhase: 'source' | 'snapshot' | 'commit' | undefined
    const commitRaw = await withFreshLegacyProviderSourceAuthority({
      operation: 'rollback-commit',
      migrationId: target.migrationId,
      sourceLocator: input.sourceLocator,
      sourceSHA256: input.sourceSHA256,
      currentSourceSHA256: input.expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
      recoveryCredentialRef: target.recoveryCredentialRef,
      settingsStore: input.settingsStore,
      migrationClient: input.migrationClient
    }, async (authority) => {
      let snapshotRaw: unknown
      try {
        snapshotRaw = await input.readRegistrySnapshot()
      } catch {
        liveFailurePhase = 'snapshot'
        throw new Error('snapshot unavailable')
      }
      const snapshot = providerRegistrySnapshotResponseSchemaV1.safeParse(snapshotRaw)
      if (!snapshot.success) {
        liveFailurePhase = 'snapshot'
        throw new Error('snapshot invalid')
      }
      const provider = snapshot.data.providers.find((candidate) => candidate.id === target.providerId)
      if (!provider || !provider.credentialConfigured || provider.tombstone) {
        liveFailurePhase = 'snapshot'
        throw new Error('snapshot provider unavailable')
      }
      try {
        return await withOwnedVerifiedSource(
          authority.verifiedSource,
          async (verifiedCleanedSource) => input.migrationClient.commitRollback({
            schemaVersion: 1,
            expected: {
              registryRevision: snapshot.data.registryRevision,
              registryIncarnation: snapshot.data.registryIncarnation,
              providerRevision: provider.revision,
              providerGeneration: provider.generation,
              providerIncarnation: provider.incarnation,
              providerCredentialPurpose: provider.credentialPurpose ?? ''
            },
            migrationId: target.migrationId,
            sourceLocator: input.sourceLocator,
            sourceSHA256: input.sourceSHA256,
            verifiedCleanedSourceSHA256: authority.verifiedSourceSHA256,
            sourcePhysicalIdentitySHA256: authority.sourcePhysicalIdentitySHA256,
            verifiedCleanedSource,
            sourceAuthority: authority.sourceAuthority,
            recoveryCredentialRef: target.recoveryCredentialRef,
            confirmation: ROLLBACK_COMMIT_CONFIRMATION
          })
        )
      } catch {
        liveFailurePhase = 'commit'
        throw new Error('rollback commit unavailable')
      }
    })
    if (commitRaw === sourceAuthorityFailure) {
      return rollbackResult('FAILED', liveFailurePhase ?? 'source')
    }
    const commit = rollbackCommitSuccessSchema.safeParse(commitRaw)
    if (!commit.success || commit.data.migrationId !== target.migrationId ||
      commit.data.status !== 'ROLLBACK_COMMITTED_RECOVERY_RETAINED') {
      return rollbackResult('FAILED', 'commit')
    }
  }
  return rollbackResult('ROLLBACK_COMMITTED')
}

export async function finalizeProviderRegistryLegacyMigrationRecovery(input: {
  migrationId: string
  sourceLocator: string
  sourceSHA256: string
  verifiedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  recoveryCredentialRef: string
  settingsStore: Pick<JsonSettingsStore, 'withLegacyProviderCredentialSourceAuthority'>
  migrationClient: Pick<
    LegacyMigrationRollbackClient,
    'finalize' | 'issueSourceAuthorityChallenge'
  >
}): Promise<{ schemaVersion: 1; status: 'COMPLETED' | 'ALREADY_FINALIZED' | 'FAILED'; outcome?: string }> {
  if (!identifierPattern.test(input.migrationId) || !sourceLocatorPattern.test(input.sourceLocator) ||
    !sourceSHA256Pattern.test(input.sourceSHA256) ||
    !sourceSHA256Pattern.test(input.verifiedSourceSHA256) ||
    !sourceSHA256Pattern.test(input.sourcePhysicalIdentitySHA256) ||
    (input.recoveryCredentialRef !== '' &&
      !recoveryCredentialRefSchema.safeParse(input.recoveryCredentialRef).success)) {
    return { schemaVersion: 1, status: 'FAILED' }
  }
  if (input.recoveryCredentialRef === '') {
    return { schemaVersion: 1, status: 'FAILED' }
  }
  const raw = await withFreshLegacyProviderSourceAuthority({
    operation: 'finalize',
    migrationId: input.migrationId,
    sourceLocator: input.sourceLocator,
    sourceSHA256: input.sourceSHA256,
    currentSourceSHA256: input.verifiedSourceSHA256,
    sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
    recoveryCredentialRef: input.recoveryCredentialRef,
    settingsStore: input.settingsStore,
    migrationClient: input.migrationClient
  }, async (authority) => withOwnedVerifiedSource(
    authority.verifiedSource,
    async (verifiedSource) => input.migrationClient.finalize({
      schemaVersion: 1,
      migrationId: input.migrationId,
      sourceLocator: input.sourceLocator,
      sourceSHA256: input.sourceSHA256,
      verifiedSourceSHA256: authority.verifiedSourceSHA256,
      sourcePhysicalIdentitySHA256: authority.sourcePhysicalIdentitySHA256,
      verifiedSource,
      sourceAuthority: authority.sourceAuthority,
      recoveryCredentialRef: input.recoveryCredentialRef,
      confirmation: FINALIZE_CONFIRMATION
    })
  ))
  if (raw === sourceAuthorityFailure) {
    return { schemaVersion: 1, status: 'FAILED' }
  }
  const parsed = finalizeSuccessSchema.safeParse(raw)
  if (!parsed.success || parsed.data.migrationId !== input.migrationId) {
    return { schemaVersion: 1, status: 'FAILED' }
  }
  return {
    schemaVersion: 1,
    status: parsed.data.status,
    outcome: parsed.data.outcome
  }
}
