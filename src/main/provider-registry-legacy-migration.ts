import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { z } from 'zod'
import {
  PROVIDER_REGISTRY_FAILURE_MESSAGES_V1,
  PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1,
  providerRegistryCredentialPurposeSchemaV1,
  providerRegistryDecimalSchemaV1,
  providerRegistryFailureSchemaV1,
  providerRegistryIncarnationSchemaV1,
  providerRegistryProviderIdSchemaV1,
  providerRegistryProviderInputSchemaV1,
  providerRegistryPublicProviderSchemaV1,
  type ProviderRegistryFailureCodeV1,
  type ProviderRegistryFailureV1,
  type ProviderRegistryProviderInputV1,
  type ProviderRegistryPublicProviderV1
} from '../../packages/runtime/src/contracts/provider-registry.js'

const PREPARE_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/prepare'
const COMMIT_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/commit'
const ABANDON_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/abandon'
const ROLLBACK_BEGIN_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/rollback/begin'
const ROLLBACK_COMMIT_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/rollback/commit'
const REMIGRATE_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/remigrate'
const PROTECTED_DELETE_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/protected-delete'
const FINALIZE_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/finalize'
const INVENTORY_PATH = '/v1/provider-registry/_private/legacy-migration-recovery/inventory'
const SOURCE_AUTHORITY_CHALLENGE_PATH =
  '/v1/provider-registry/_private/legacy-migration-recovery/source-authority/challenge'
const COMMIT_CONFIRMATION = 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const ABANDON_CONFIRMATION = 'ABANDON_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const ROLLBACK_CONFIRMATION = 'BEGIN_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK'
const ROLLBACK_COMMIT_CONFIRMATION = 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK'
const REMIGRATION_CONFIRMATION = 'REMIGRATE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const PROTECTED_DELETE_CONFIRMATION = 'DELETE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const FINALIZE_CONFIRMATION = 'FINALIZE_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
const INVENTORY_CONFIRMATION = 'INSPECT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERIES'
const SOURCE_AUTHORITY_CHALLENGE_CONFIRMATION = 'ISSUE_PROVIDER_SETTINGS_SOURCE_AUTHORITY_CHALLENGE'
const MAX_REQUEST_BYTES = 2 << 20
const MAX_RESPONSE_BYTES = 64 << 10
const MAX_INVENTORY_RESPONSE_BYTES = 256 << 10
const MAX_LEGACY_MIGRATION_LOCATOR_BYTES = 256
const MAX_LEGACY_MIGRATION_LOCATOR_COUNT = 256
const MAX_LEGACY_MIGRATION_LOCATOR_TOTAL_BYTES = 32 << 10
const MAX_LEGACY_MIGRATION_ROLLBACK_ARTIFACTS = 64
const MAX_LEGACY_MIGRATION_SOURCE_BYTES = 256 << 10
const CREDENTIAL_SENTINELS = [
  'redacted', '[redacted]', '<redacted>', '__redacted__',
  'masked', '[masked]', '<masked>', '__masked__',
  'unset', 'not-set', 'not_set', 'null', 'undefined'
].map((value) => Buffer.from(value, 'utf8'))
const LEGACY_MIGRATION_EXACT_LOCATOR_SUFFIXES = new Set([
  'runtime.apiKey',
  'deepseek.apiKey',
  'agents.reasonix.apiKey',
  'agents.codewhale.apiKey',
  'agents.kun.apiKey',
  'provider.apiKey'
])

type RuntimeRequestResult = { ok: boolean; status: number; body: string }
type RuntimeRequest = (
  path: string,
  method: 'POST',
  body: string
) => Promise<RuntimeRequestResult>

function bytesEqual(left: Uint8Array, right: Uint8Array): boolean {
  if (left.byteLength !== right.byteLength) return false
  for (let index = 0; index < left.byteLength; index += 1) {
    if (left[index] !== right[index]) return false
  }
  return true
}

function bytesContain(haystack: Uint8Array, needle: Uint8Array): boolean {
  if (needle.byteLength === 0 || needle.byteLength > haystack.byteLength) return false
  const lastStart = haystack.byteLength - needle.byteLength
  for (let start = 0; start <= lastStart; start += 1) {
    let matches = true
    for (let index = 0; index < needle.byteLength; index += 1) {
      if (haystack[start + index] !== needle[index]) {
        matches = false
        break
      }
    }
    if (matches) return true
  }
  return false
}

function asciiBytesEqualFold(value: Uint8Array, start: number, end: number, expected: Uint8Array): boolean {
  if (end - start !== expected.byteLength) return false
  for (let index = 0; index < expected.byteLength; index += 1) {
    const current = value[start + index]
    const folded = current >= 65 && current <= 90 ? current + 32 : current
    if (folded !== expected[index]) return false
  }
  return true
}

function isCredentialSentinel(value: Uint8Array): boolean {
  let start = 0
  let end = value.byteLength
  while (start < end && [9, 10, 11, 12, 13, 32].includes(value[start])) start += 1
  while (end > start && [9, 10, 11, 12, 13, 32].includes(value[end - 1])) end -= 1
  if (start === end) return true
  if (CREDENTIAL_SENTINELS.some((sentinel) => asciiBytesEqualFold(value, start, end, sentinel))) return true

  let maskStart = start
  for (let index = start; index < end; index += 1) {
    const current = value[index]
    if (current !== 45 && current !== 58 && current !== 95) continue
    if (index === start) break
    let validPrefix = true
    for (let prefixIndex = start; prefixIndex < index; prefixIndex += 1) {
      const prefix = value[prefixIndex]
      if (!((prefix >= 48 && prefix <= 57) || (prefix >= 65 && prefix <= 90) ||
        (prefix >= 97 && prefix <= 122))) {
        validPrefix = false
        break
      }
    }
    if (validPrefix) maskStart = index + 1
    break
  }

  let maskCount = 0
  for (let index = maskStart; index < end;) {
    const current = value[index]
    if (current === 42 || current === 88 || current === 120) {
      maskCount += 1
      index += 1
      continue
    }
    if (index + 2 < end && current === 0xe2 &&
      ((value[index + 1] === 0x80 && value[index + 2] === 0xa2) ||
        (value[index + 1] === 0x97 && value[index + 2] === 0x8f))) {
      maskCount += 1
      index += 3
      continue
    }
    return false
  }
  return maskCount >= 4
}

function validLegacyMigrationLogicalLocator(locator: string): boolean {
  let suffix: string
  const currentPrefix = 'current:analytix-settings.json:'
  if (locator.startsWith(currentPrefix)) {
    suffix = locator.slice(currentPrefix.length)
  } else {
    const match = /^compatibility:\d{2}:(?:analytix-settings|kun-settings)\.json:(.+)$/.exec(locator)
    if (!match) return false
    suffix = match[1]
  }
  if (LEGACY_MIGRATION_EXACT_LOCATOR_SUFFIXES.has(suffix)) return true
  const profile = /^provider\.providers\[(0|[1-9]\d?)\]\.apiKey$/.exec(suffix)
  return profile !== null && Number(profile[1]) <= 63
}

function validLegacyMigrationSourceLocator(locator: string): boolean {
  return locator === 'current:analytix-settings.json' ||
    /^compatibility:\d{2}:(?:analytix-settings|kun-settings)\.json$/.test(locator)
}

const expectedSchema = z.object({
  registryRevision: providerRegistryDecimalSchemaV1,
  registryIncarnation: providerRegistryIncarnationSchemaV1,
  providerRevision: providerRegistryDecimalSchemaV1,
  providerGeneration: providerRegistryDecimalSchemaV1,
  providerIncarnation: z.union([z.literal(''), providerRegistryIncarnationSchemaV1]),
  providerCredentialPurpose: z.union([z.literal(''), providerRegistryCredentialPurposeSchemaV1])
}).strict().refine((expected) => expected.providerRevision === '0'
  ? expected.providerGeneration === '0' && expected.providerIncarnation === '' &&
    expected.providerCredentialPurpose === ''
  : expected.providerGeneration !== '0' && expected.providerIncarnation !== '', {
  message: 'legacy migration expected state is invalid'
})

const migrationIdSchema = z.string().regex(/^[a-z0-9][a-z0-9._-]{0,95}$/)
const sourceSHA256Schema = z.string().regex(/^[0-9a-f]{64}$/)
const sourceLocatorSchema = z.string().max(MAX_LEGACY_MIGRATION_LOCATOR_BYTES)
  .refine(validLegacyMigrationSourceLocator, 'legacy migration source locator is invalid')
const recoveryCredentialRefSchema = z.string().regex(/^cred_[A-Za-z0-9_-]{43}$/)
const sourceAuthorityChallengeSchema = z.string().regex(/^lmsa_[A-Za-z0-9_-]{43}$/)
const sourceAuthorityProofSchema = z.object({
  challenge: sourceAuthorityChallengeSchema,
  sourcePath: z.string().min(1).max(4096).refine((value) => !value.includes('\0')),
  lockOwnerToken: z.string().uuid(),
  sourceDevice: z.string().regex(/^(?:0|[1-9]\d*)$/),
  sourceInode: z.string().regex(/^[1-9]\d*$/)
}).strict()
const credentialBytesSchema = z.instanceof(Uint8Array).refine(
  (value) => value.byteLength > 0 &&
    value.byteLength <= PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1 &&
    !isCredentialSentinel(value),
  'legacy migration credential is invalid'
)
const rollbackCredentialBytesSchema = z.instanceof(Uint8Array).refine(
  (value) => value.byteLength > 0 && value.byteLength <= PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1,
  'legacy migration rollback value is invalid'
)

const legacyMigrationLocatorSchema = z.string()
  .min(1)
  .max(MAX_LEGACY_MIGRATION_LOCATOR_BYTES)
  .refine(validLegacyMigrationLogicalLocator, 'legacy migration credential locator is invalid')

const rollbackCredentialArtifactSchema = z.object({
  schemaVersion: z.literal(1),
  credentialLocators: z.array(legacyMigrationLocatorSchema)
    .min(1)
    .max(MAX_LEGACY_MIGRATION_LOCATOR_COUNT),
  credential: rollbackCredentialBytesSchema
}).strict()

const prepareRequestSchema = z.object({
  schemaVersion: z.literal(1),
  expected: expectedSchema,
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  expectedCleanedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  provider: providerRegistryProviderInputSchemaV1,
  credentialPurpose: providerRegistryCredentialPurposeSchemaV1,
  credential: credentialBytesSchema,
  sourceSnapshot: z.instanceof(Uint8Array).refine(
    (value) => value.byteLength > 0 && value.byteLength <= MAX_LEGACY_MIGRATION_SOURCE_BYTES,
    'legacy migration source snapshot is invalid'
  ),
  activeCredentialLocators: z.array(legacyMigrationLocatorSchema)
    .min(1)
    .max(MAX_LEGACY_MIGRATION_LOCATOR_COUNT),
  rollbackCredentialArtifacts: z.array(rollbackCredentialArtifactSchema)
    .max(MAX_LEGACY_MIGRATION_ROLLBACK_ARTIFACTS)
    .optional()
}).strict().superRefine((input, context) => {
  if (createHash('sha256').update(input.sourceSnapshot).digest('hex') !== input.sourceSHA256) {
    context.addIssue({ code: 'custom', message: 'legacy migration source snapshot identity is invalid' })
    return
  }
  const activeLocators = input.activeCredentialLocators
  const artifacts = input.rollbackCredentialArtifacts ?? []

  let locatorBytes = 0
  let locatorCount = 0
  const locators = new Set<string>()
  const addLocator = (locator: string): boolean => {
    locatorBytes += Buffer.byteLength(locator, 'utf8')
    locatorCount += 1
    if (locatorBytes > MAX_LEGACY_MIGRATION_LOCATOR_TOTAL_BYTES ||
      locatorCount > MAX_LEGACY_MIGRATION_LOCATOR_COUNT || locators.has(locator) ||
      !locator.startsWith(`${input.sourceLocator}:`)) return false
    locators.add(locator)
    return true
  }
  for (const locator of activeLocators) {
    if (!addLocator(locator)) {
      context.addIssue({ code: 'custom', message: 'legacy migration credential locators are ambiguous' })
      return
    }
  }

  let credentialBytes = input.credential.byteLength
  const credentialValues: Uint8Array[] = [input.credential]
  for (const artifact of artifacts) {
    for (const locator of artifact.credentialLocators) {
      if (!addLocator(locator)) {
        context.addIssue({ code: 'custom', message: 'legacy migration credential locators are ambiguous' })
        return
      }
    }
    credentialBytes += artifact.credential.byteLength
    if (credentialBytes > PROVIDER_REGISTRY_MAX_SECRET_BYTES_V1 ||
      credentialValues.some((value) => bytesEqual(value, artifact.credential))) {
      context.addIssue({ code: 'custom', message: 'legacy migration credential values are ambiguous' })
      return
    }
    credentialValues.push(artifact.credential)
  }
})

const abandonRequestSchema = z.object({
  schemaVersion: z.literal(1),
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(ABANDON_CONFIRMATION)
}).strict()

const commitRequestSchema = z.object({
  schemaVersion: z.literal(1),
  expected: expectedSchema,
  expectedSelectedProviderID: z.union([z.literal(''), providerRegistryProviderIdSchemaV1]),
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(COMMIT_CONFIRMATION)
}).strict()

const rollbackBeginRequestSchema = z.object({
  schemaVersion: z.literal(1),
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema.optional(),
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(ROLLBACK_CONFIRMATION)
}).strict()

const rollbackCommitRequestSchema = z.object({
  schemaVersion: z.literal(1),
  expected: expectedSchema,
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  verifiedCleanedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  verifiedCleanedSource: z.instanceof(Uint8Array).refine(
    (value) => value.byteLength > 0 && value.byteLength <= MAX_LEGACY_MIGRATION_SOURCE_BYTES,
    'legacy migration cleaned source is invalid'
  ),
  sourceAuthority: sourceAuthorityProofSchema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(ROLLBACK_COMMIT_CONFIRMATION)
}).strict().superRefine((input, context) => {
  if (createHash('sha256').update(input.verifiedCleanedSource).digest('hex') !==
    input.verifiedCleanedSourceSHA256) {
    context.addIssue({ code: 'custom', message: 'legacy migration cleaned source identity is invalid' })
  }
})

const remigrateRequestSchema = z.object({
  schemaVersion: z.literal(1),
  expected: expectedSchema,
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  verifiedCleanedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  verifiedCleanedSource: z.instanceof(Uint8Array).refine(
    (value) => value.byteLength > 0 && value.byteLength <= MAX_LEGACY_MIGRATION_SOURCE_BYTES,
    'legacy migration cleaned source is invalid'
  ),
  sourceAuthority: sourceAuthorityProofSchema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(REMIGRATION_CONFIRMATION)
}).strict().superRefine((input, context) => {
  if (createHash('sha256').update(input.verifiedCleanedSource).digest('hex') !==
    input.verifiedCleanedSourceSHA256) {
    context.addIssue({ code: 'custom', message: 'legacy migration cleaned source identity is invalid' })
  }
})

const protectedDeleteRequestSchema = z.object({
  schemaVersion: z.literal(1),
  expected: expectedSchema,
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  verifiedCleanedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  verifiedCleanedSource: z.instanceof(Uint8Array).refine(
    (value) => value.byteLength > 0 && value.byteLength <= MAX_LEGACY_MIGRATION_SOURCE_BYTES,
    'legacy migration cleaned source is invalid'
  ),
  sourceAuthority: sourceAuthorityProofSchema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(PROTECTED_DELETE_CONFIRMATION)
}).strict().superRefine((input, context) => {
  if (createHash('sha256').update(input.verifiedCleanedSource).digest('hex') !==
    input.verifiedCleanedSourceSHA256) {
    context.addIssue({ code: 'custom', message: 'legacy migration cleaned source identity is invalid' })
  }
})

const finalizeRequestSchema = z.object({
  schemaVersion: z.literal(1),
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  verifiedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  verifiedSource: z.instanceof(Uint8Array).refine(
    (value) => value.byteLength > 0 && value.byteLength <= MAX_LEGACY_MIGRATION_SOURCE_BYTES,
    'legacy migration verified source is invalid'
  ),
  sourceAuthority: sourceAuthorityProofSchema,
  recoveryCredentialRef: z.union([z.literal(''), recoveryCredentialRefSchema]),
  confirmation: z.literal(FINALIZE_CONFIRMATION)
}).strict().superRefine((input, context) => {
  if (createHash('sha256').update(input.verifiedSource).digest('hex') !== input.verifiedSourceSHA256) {
    context.addIssue({ code: 'custom', message: 'legacy migration verified source identity is invalid' })
  }
})

const sourceAuthorityChallengeRequestSchema = z.object({
  schemaVersion: z.literal(1),
  operation: z.enum(['rollback-commit', 'finalize', 'remigrate', 'protected-delete']),
  migrationId: migrationIdSchema,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  currentSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  confirmation: z.literal(SOURCE_AUTHORITY_CHALLENGE_CONFIRMATION)
}).strict()

const sourceAuthorityChallengeSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  challenge: sourceAuthorityChallengeSchema
}).strict()

const runtimeResultSchema = z.object({
  ok: z.boolean(),
  status: z.union([z.literal(0), z.number().int().min(100).max(599)]),
  body: z.string()
}).strict()

const runtimeUnavailableSchema = z.object({
  code: z.literal('fetch_failed'),
  message: z.literal('The Analytix runtime is unavailable.')
}).strict()

const verifiedPrepareSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('VERIFIED_RECOVERY'),
  migrationId: migrationIdSchema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  safeToProceedWithProviderMigration: z.literal(true)
}).strict()

const committedRetainedPrepareSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('PROVIDER_COMMITTED_RECOVERY_RETAINED'),
  migrationId: migrationIdSchema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  safeToProceedWithProviderMigration: z.literal(false)
}).strict()

const prepareSuccessSchema = z.discriminatedUnion('status', [
  verifiedPrepareSuccessSchema,
  committedRetainedPrepareSuccessSchema
])

const abandonSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('COMPLETED')
}).strict()

const commitSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('PROVIDER_COMMITTED_RECOVERY_RETAINED'),
  migrationId: migrationIdSchema,
  safeToRemoveLegacyPlaintext: z.literal(true),
  provider: providerRegistryPublicProviderSchemaV1
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
  migrationId: migrationIdSchema
}).strict()
const rollbackCommitSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum(['ROLLBACK_COMMITTED_RECOVERY_RETAINED', 'ALREADY_FINALIZED']),
  migrationId: migrationIdSchema
}).strict()
const remigrateSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.literal('PROVIDER_COMMITTED_RECOVERY_RETAINED'),
  migrationId: migrationIdSchema,
  safeToRemoveLegacyPlaintext: z.literal(false),
  provider: providerRegistryPublicProviderSchemaV1
}).strict()
const protectedDeleteSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum(['COMPLETED', 'ALREADY_DELETED']),
  migrationId: migrationIdSchema
}).strict()
const finalizeSuccessSchema = z.object({
  schemaVersion: z.literal(1),
  status: z.enum(['COMPLETED', 'ALREADY_FINALIZED']),
  outcome: z.enum(['MIGRATION_COMMITTED', 'PROTECTED_RECOVERY_RETAINED']),
  migrationId: migrationIdSchema
}).strict()

const inventoryRecoveryPhaseSchema = z.enum([
  'prepared',
  'secret-durable',
  'verified',
  'provider-commit-prepared',
  'provider-committed-recovery-retained',
  'rollback-cleaned-source-authority-pending',
  'rollback-registry-commit-pending',
  'rollback-committed-recovery-retained',
  'rollback-recovery-retained',
  'protected-recovery-delete-pending',
  'finalizing-committed',
  'finalizing-rollback'
])
const canonicalDecimalSchema = z.string().regex(/^(?:0|[1-9]\d*)$/)
  .refine((value) => {
    try {
      return BigInt(value) <= 18_446_744_073_709_551_615n
    } catch {
      return false
    }
  }, 'legacy migration commit order is invalid')
const inventoryRecoverySchema = z.object({
  migrationId: migrationIdSchema,
  providerId: providerRegistryProviderIdSchemaV1,
  sourceLocator: sourceLocatorSchema,
  sourceSHA256: sourceSHA256Schema,
  expectedCleanedSourceSHA256: sourceSHA256Schema,
  sourcePhysicalIdentitySHA256: sourceSHA256Schema,
  recoveryCredentialRef: recoveryCredentialRefSchema,
  phase: inventoryRecoveryPhaseSchema,
  commitOrder: canonicalDecimalSchema
}).strict()
const inventorySuccessSchema = z.object({
  schemaVersion: z.literal(1),
  recoveries: z.array(inventoryRecoverySchema).max(MAX_LEGACY_MIGRATION_LOCATOR_COUNT)
}).strict()

const failureStatuses: Record<Exclude<ProviderRegistryFailureCodeV1,
  'runtime_unavailable' | 'invalid_response'>, number> = {
  invalid_request: 400,
  method_not_allowed: 405,
  not_found: 404,
  conflict: 409,
  persistence_failure: 503,
  credential_unavailable: 503,
  verification_failure: 500,
  request_too_large: 413,
  unauthorized: 401
}

export type ProviderRegistryLegacyMigrationExpected = z.infer<typeof expectedSchema>

export type ProviderRegistryLegacyMigrationPrepareRequest = {
  schemaVersion: 1
  expected: ProviderRegistryLegacyMigrationExpected
  migrationId: string
  sourceLocator: string
  sourceSHA256: string
  expectedCleanedSourceSHA256: string
  sourcePhysicalIdentitySHA256: string
  provider: ProviderRegistryProviderInputV1
  credentialPurpose: string
  credential: Uint8Array
  sourceSnapshot: Uint8Array
  activeCredentialLocators: string[]
  rollbackCredentialArtifacts?: Array<{
    schemaVersion: 1
    credentialLocators: string[]
    credential: Uint8Array
  }>
}

export type ProviderRegistryLegacyMigrationAbandonRequest = z.infer<typeof abandonRequestSchema>
export type ProviderRegistryLegacyMigrationCommitRequest = z.infer<typeof commitRequestSchema>
export type ProviderRegistryLegacyMigrationPrepareSuccess = z.infer<typeof prepareSuccessSchema>
export type ProviderRegistryLegacyMigrationAbandonSuccess = z.infer<typeof abandonSuccessSchema>
export type ProviderRegistryLegacyMigrationCommitSuccess = {
  schemaVersion: 1
  status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED'
  migrationId: string
  safeToRemoveLegacyPlaintext: true
  provider: ProviderRegistryPublicProviderV1
}
export type ProviderRegistryLegacyMigrationRollbackBeginRequest = z.infer<typeof rollbackBeginRequestSchema>
export type ProviderRegistryLegacyMigrationRollbackCommitRequest = z.infer<typeof rollbackCommitRequestSchema>
export type ProviderRegistryLegacyMigrationRemigrateRequest = z.infer<typeof remigrateRequestSchema>
export type ProviderRegistryLegacyMigrationProtectedDeleteRequest = z.infer<typeof protectedDeleteRequestSchema>
export type ProviderRegistryLegacyMigrationFinalizeRequest = z.infer<typeof finalizeRequestSchema>
export type ProviderRegistryLegacyMigrationSourceAuthorityChallengeRequest =
  z.infer<typeof sourceAuthorityChallengeRequestSchema>
export type ProviderRegistryLegacyMigrationSourceAuthorityChallengeSuccess =
  z.infer<typeof sourceAuthorityChallengeSuccessSchema>
export type ProviderRegistryLegacyMigrationRollbackBeginSuccess = {
  schemaVersion: 1
  status: 'CLEANED_SOURCE_AUTHORITY_REQUIRED' | 'PRE_COMMIT_ROLLBACK_COMPLETED' |
    'ROLLBACK_COMMITTED_RECOVERY_RETAINED' | 'ROLLBACK_RECOVERY_RETAINED' | 'ALREADY_FINALIZED'
  migrationId: string
}
export type ProviderRegistryLegacyMigrationRollbackCommitSuccess = z.infer<typeof rollbackCommitSuccessSchema>
export type ProviderRegistryLegacyMigrationRemigrateSuccess = z.infer<typeof remigrateSuccessSchema>
export type ProviderRegistryLegacyMigrationProtectedDeleteSuccess = z.infer<typeof protectedDeleteSuccessSchema>
export type ProviderRegistryLegacyMigrationFinalizeSuccess = z.infer<typeof finalizeSuccessSchema>
export type ProviderRegistryLegacyMigrationRecoveryDescriptor = z.infer<typeof inventoryRecoverySchema>
export type ProviderRegistryLegacyMigrationInventorySuccess = z.infer<typeof inventorySuccessSchema>

function failure(code: ProviderRegistryFailureCodeV1): ProviderRegistryFailureV1 {
  return { schemaVersion: 1, error: { code, message: PROVIDER_REGISTRY_FAILURE_MESSAGES_V1[code] } }
}

type PrivatePrepareMaterial = {
  encodedCredentials: string[]
  credentials: Uint8Array[]
  credentialLocators: string[]
}

function responseEchoesPrivatePrepareInput(
  body: string,
  material: PrivatePrepareMaterial | undefined
): boolean {
  if (material === undefined) return false
  for (const encoded of material.encodedCredentials) {
    if (body.includes(encoded)) return true
  }
  for (const locator of material.credentialLocators) {
    if (body.includes(locator)) return true
  }
  const bodyBytes = Buffer.from(body, 'utf8')
  try {
    return material.credentials.some((credential) =>
      bytesContain(bodyBytes, credential)
    )
  } finally {
    bodyBytes.fill(0)
  }
}

function containsForbiddenPrivateOutput(
  body: string,
  operation: 'prepare' | 'commit' | 'abandon',
  correlation: { sourceSHA256: string; recoveryCredentialRef?: string }
): boolean {
  if (body.includes(correlation.sourceSHA256) ||
    (correlation.recoveryCredentialRef !== undefined &&
      body.includes(correlation.recoveryCredentialRef))) return true
  const forbiddenKey = operation === 'commit'
    ? /"(?:credential|credentialRef|recoveryCredentialRef|valueBase64|sourceSHA256|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope)"\s*:/i
    : operation === 'prepare'
      ? /"(?:credential|credentialRef|valueBase64|sourceSHA256|provider|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope)"\s*:/i
      : /"(?:credential|credentialRef|recoveryCredentialRef|valueBase64|sourceSHA256|provider|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope)"\s*:/i
  return forbiddenKey.test(body)
}

function parseRuntimeResponse(
  rawResult: unknown,
  operation: 'prepare' | 'commit' | 'abandon',
  correlation: { migrationId: string; sourceSHA256: string; recoveryCredentialRef?: string },
  prepareMaterial?: PrivatePrepareMaterial
): ProviderRegistryLegacyMigrationPrepareSuccess |
  ProviderRegistryLegacyMigrationCommitSuccess |
  ProviderRegistryLegacyMigrationAbandonSuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_RESPONSE_BYTES ||
    containsForbiddenPrivateOutput(raw.data.body, operation, correlation) ||
    responseEchoesPrivatePrepareInput(raw.data.body, prepareMaterial)) {
    return failure('invalid_response')
  }

  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }

  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }

  if (raw.data.ok && raw.data.status === 200) {
    if (operation === 'prepare') {
      const success = prepareSuccessSchema.safeParse(value)
      return success.success && success.data.migrationId === correlation.migrationId
        ? success.data
        : failure('invalid_response')
    }
    if (operation === 'commit') {
      const success = commitSuccessSchema.safeParse(value)
      return success.success && success.data.migrationId === correlation.migrationId
        ? success.data
        : failure('invalid_response')
    }
    const success = abandonSuccessSchema.safeParse(value)
    return success.success ? success.data : failure('invalid_response')
  }

  if (raw.data.ok || raw.data.status < 400) return failure('invalid_response')
  const parsedFailure = providerRegistryFailureSchemaV1.safeParse(value)
  if (!parsedFailure.success || parsedFailure.data.error.code === 'runtime_unavailable' ||
    parsedFailure.data.error.code === 'invalid_response') {
    return failure('invalid_response')
  }
  return failureStatuses[parsedFailure.data.error.code] === raw.data.status
    ? parsedFailure.data
    : failure('invalid_response')
}

function parsePrivateLifecycleFailure(
  raw: z.infer<typeof runtimeResultSchema>,
  value: unknown
): ProviderRegistryFailureV1 {
  if (raw.ok || raw.status < 400) return failure('invalid_response')
  const parsed = providerRegistryFailureSchemaV1.safeParse(value)
  if (!parsed.success || parsed.data.error.code === 'runtime_unavailable' ||
    parsed.data.error.code === 'invalid_response') return failure('invalid_response')
  return failureStatuses[parsed.data.error.code] === raw.status
    ? parsed.data
    : failure('invalid_response')
}

function parseRollbackBeginResponse(
  rawResult: unknown,
  correlation: ProviderRegistryLegacyMigrationRollbackBeginRequest
): ProviderRegistryLegacyMigrationRollbackBeginSuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_RESPONSE_BYTES ||
    raw.data.body.includes(correlation.recoveryCredentialRef) ||
    /"(?:credentialRef|recoveryCredentialRef|provider|ciphertext|nonce|tag|masterKey|rawBody|payload|envelope)"\s*:/i
      .test(raw.data.body)) return failure('invalid_response')
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) return parsePrivateLifecycleFailure(raw.data, value)
  const parsed = rollbackBeginSuccessSchema.safeParse(value)
  return parsed.success && parsed.data.migrationId === correlation.migrationId
    ? parsed.data
    : failure('invalid_response')
}

function parseSimpleLifecycleResponse(
  rawResult: unknown,
  operation: 'rollback-commit' | 'finalize' | 'protected-delete',
  correlation: { migrationId: string; sourceSHA256: string; recoveryCredentialRef: string }
): ProviderRegistryLegacyMigrationRollbackCommitSuccess |
  ProviderRegistryLegacyMigrationFinalizeSuccess |
  ProviderRegistryLegacyMigrationProtectedDeleteSuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_RESPONSE_BYTES ||
    raw.data.body.includes(correlation.sourceSHA256) ||
    (correlation.recoveryCredentialRef !== '' && raw.data.body.includes(correlation.recoveryCredentialRef)) ||
    /"(?:credential|credentialRef|recoveryCredentialRef|valueBase64|sourceSHA256|provider|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope)"\s*:/i
      .test(raw.data.body)) return failure('invalid_response')
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) return parsePrivateLifecycleFailure(raw.data, value)
  const parsed = operation === 'rollback-commit'
    ? rollbackCommitSuccessSchema.safeParse(value)
    : operation === 'finalize'
      ? finalizeSuccessSchema.safeParse(value)
      : protectedDeleteSuccessSchema.safeParse(value)
  return parsed.success && parsed.data.migrationId === correlation.migrationId
    ? parsed.data
    : failure('invalid_response')
}

function parseRemigrationResponse(
  rawResult: unknown,
  correlation: { migrationId: string; sourceSHA256: string; recoveryCredentialRef: string }
): ProviderRegistryLegacyMigrationRemigrateSuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_RESPONSE_BYTES ||
    raw.data.body.includes(correlation.sourceSHA256) ||
    raw.data.body.includes(correlation.recoveryCredentialRef) ||
    /"(?:credentialRef|recoveryCredentialRef|valueBase64|sourceSHA256|sourceSnapshot|rollbackSource|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope|filePath|path)"\s*:/i
      .test(raw.data.body)) return failure('invalid_response')
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) return parsePrivateLifecycleFailure(raw.data, value)
  const parsed = remigrateSuccessSchema.safeParse(value)
  return parsed.success && parsed.data.migrationId === correlation.migrationId
    ? parsed.data
    : failure('invalid_response')
}

function parseInventoryResponse(
  rawResult: unknown
): ProviderRegistryLegacyMigrationInventorySuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_INVENTORY_RESPONSE_BYTES ||
    /"(?:credential|credentialRef|valueBase64|sourceSnapshot|rollbackSource|credentialArtifacts|provider|endpoint|ciphertext|nonce|tag|masterKey|secret|rawBody|payload|envelope|filePath|path)"\s*:/i
      .test(raw.data.body)) return failure('invalid_response')
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) return parsePrivateLifecycleFailure(raw.data, value)
  const parsed = inventorySuccessSchema.safeParse(value)
  if (!parsed.success) return failure('invalid_response')
  const seenMigrations = new Set<string>()
  const seenProviders = new Set<string>()
  const seenRefs = new Set<string>()
  let previousKey = ''
  for (const recovery of parsed.data.recoveries) {
    const key = [
      recovery.sourceLocator,
      recovery.sourceSHA256,
      recovery.expectedCleanedSourceSHA256,
      recovery.sourcePhysicalIdentitySHA256,
      recovery.commitOrder.padStart(20, '0'),
      recovery.migrationId
    ].join('\u0000')
    if (seenMigrations.has(recovery.migrationId) || seenProviders.has(recovery.providerId) ||
      seenRefs.has(recovery.recoveryCredentialRef) || (previousKey !== '' && key <= previousKey)) {
      return failure('invalid_response')
    }
    seenMigrations.add(recovery.migrationId)
    seenProviders.add(recovery.providerId)
    seenRefs.add(recovery.recoveryCredentialRef)
    previousKey = key
  }
  return parsed.data
}

function parseSourceAuthorityChallengeResponse(
  rawResult: unknown
): ProviderRegistryLegacyMigrationSourceAuthorityChallengeSuccess | ProviderRegistryFailureV1 {
  const raw = runtimeResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > MAX_RESPONSE_BYTES ||
    /"(?:credential|credentialRef|valueBase64|sourcePath|path|provider|secret|rawBody|payload|envelope)"\s*:/i
      .test(raw.data.body)) return failure('invalid_response')
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return failure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableSchema.safeParse(value).success
      ? failure('runtime_unavailable')
      : failure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) return parsePrivateLifecycleFailure(raw.data, value)
  const parsed = sourceAuthorityChallengeSuccessSchema.safeParse(value)
  return parsed.success ? parsed.data : failure('invalid_response')
}

export function createProviderRegistryLegacyMigrationClient(runtimeRequest: RuntimeRequest) {
  return {
    async prepare(input: ProviderRegistryLegacyMigrationPrepareRequest) {
      const parsed = prepareRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')

      const activeCredentialLocators = parsed.data.activeCredentialLocators
      const rollbackCredentialArtifacts = parsed.data.rollbackCredentialArtifacts ?? []
      const credentialBuffers = [
        Buffer.from(parsed.data.credential),
        ...rollbackCredentialArtifacts.map((artifact) => Buffer.from(artifact.credential)),
        Buffer.from(parsed.data.sourceSnapshot)
      ]
      const encodedBuffers = credentialBuffers.map((credential) =>
        Buffer.from(credential.toString('base64'), 'ascii')
      )
      let encodedCredentials = encodedBuffers.map((encoded) => encoded.toString('ascii'))
      try {
        const body = JSON.stringify({
          schemaVersion: 1,
          expected: parsed.data.expected,
          migrationId: parsed.data.migrationId,
          sourceLocator: parsed.data.sourceLocator,
          sourceSHA256: parsed.data.sourceSHA256,
          expectedCleanedSourceSHA256: parsed.data.expectedCleanedSourceSHA256,
          sourcePhysicalIdentitySHA256: parsed.data.sourcePhysicalIdentitySHA256,
          provider: parsed.data.provider,
          credential: {
            purpose: parsed.data.credentialPurpose,
            valueBase64: encodedCredentials[0]
          },
          rollbackSource: {
            valueBase64: encodedCredentials[encodedCredentials.length - 1]
          },
          activeCredentialLocators,
          ...(rollbackCredentialArtifacts.length > 0
            ? {
                rollbackCredentialArtifacts: rollbackCredentialArtifacts.map((artifact, index) => ({
                  schemaVersion: 1,
                  credentialLocators: artifact.credentialLocators,
                  credential: { valueBase64: encodedCredentials[index + 1] }
                }))
              }
            : {})
        })
        if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
        try {
          const result = await runtimeRequest(PREPARE_PATH, 'POST', body)
          return parseRuntimeResponse(result, 'prepare', parsed.data, {
            encodedCredentials,
            credentials: credentialBuffers,
            credentialLocators: [
              ...activeCredentialLocators,
              ...rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
            ]
          })
        } catch {
          return failure('runtime_unavailable')
        }
      } finally {
        for (const encoded of encodedBuffers) encoded.fill(0)
        for (const credential of credentialBuffers) credential.fill(0)
        encodedCredentials = []
      }
    },

    async commit(input: ProviderRegistryLegacyMigrationCommitRequest) {
      const parsed = commitRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const body = JSON.stringify(parsed.data)
      if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
      try {
        const result = await runtimeRequest(COMMIT_PATH, 'POST', body)
        return parseRuntimeResponse(result, 'commit', parsed.data)
      } catch {
        return failure('runtime_unavailable')
      }
    },

    async abandon(input: ProviderRegistryLegacyMigrationAbandonRequest) {
      const parsed = abandonRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const body = JSON.stringify(parsed.data)
      if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
      try {
        const result = await runtimeRequest(ABANDON_PATH, 'POST', body)
        return parseRuntimeResponse(result, 'abandon', parsed.data)
      } catch {
        return failure('runtime_unavailable')
      }
    },

    async beginRollback(input: ProviderRegistryLegacyMigrationRollbackBeginRequest) {
      const parsed = rollbackBeginRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const body = JSON.stringify(parsed.data)
      if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
      try {
        const result = await runtimeRequest(ROLLBACK_BEGIN_PATH, 'POST', body)
        return parseRollbackBeginResponse(result, parsed.data)
      } catch {
        return failure('runtime_unavailable')
      }
    },

    async commitRollback(input: ProviderRegistryLegacyMigrationRollbackCommitRequest) {
      const parsed = rollbackCommitRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const verifiedCleanedSource = Buffer.from(parsed.data.verifiedCleanedSource)
      const encodedSource = Buffer.from(verifiedCleanedSource.toString('base64'), 'ascii')
      try {
        const body = JSON.stringify({
          ...parsed.data,
          verifiedCleanedSource: { valueBase64: encodedSource.toString('ascii') }
        })
        if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
        try {
          const result = await runtimeRequest(ROLLBACK_COMMIT_PATH, 'POST', body)
          return parseSimpleLifecycleResponse(result, 'rollback-commit', parsed.data)
        } catch {
          return failure('runtime_unavailable')
        }
      } finally {
        encodedSource.fill(0)
        verifiedCleanedSource.fill(0)
      }
    },

    async remigrate(input: ProviderRegistryLegacyMigrationRemigrateRequest) {
      const parsed = remigrateRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const verifiedCleanedSource = Buffer.from(parsed.data.verifiedCleanedSource)
      const encodedSource = Buffer.from(verifiedCleanedSource.toString('base64'), 'ascii')
      try {
        const body = JSON.stringify({
          ...parsed.data,
          verifiedCleanedSource: { valueBase64: encodedSource.toString('ascii') }
        })
        if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
        try {
          const result = await runtimeRequest(REMIGRATE_PATH, 'POST', body)
          return parseRemigrationResponse(result, parsed.data)
        } catch {
          return failure('runtime_unavailable')
        }
      } finally {
        encodedSource.fill(0)
        verifiedCleanedSource.fill(0)
      }
    },

    async deleteRetainedRecovery(input: ProviderRegistryLegacyMigrationProtectedDeleteRequest) {
      const parsed = protectedDeleteRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const verifiedCleanedSource = Buffer.from(parsed.data.verifiedCleanedSource)
      const encodedSource = Buffer.from(verifiedCleanedSource.toString('base64'), 'ascii')
      try {
        const body = JSON.stringify({
          ...parsed.data,
          verifiedCleanedSource: { valueBase64: encodedSource.toString('ascii') }
        })
        if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
        try {
          const result = await runtimeRequest(PROTECTED_DELETE_PATH, 'POST', body)
          return parseSimpleLifecycleResponse(result, 'protected-delete', parsed.data)
        } catch {
          return failure('runtime_unavailable')
        }
      } finally {
        encodedSource.fill(0)
        verifiedCleanedSource.fill(0)
      }
    },

    async finalize(input: ProviderRegistryLegacyMigrationFinalizeRequest) {
      const parsed = finalizeRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const verifiedSource = Buffer.from(parsed.data.verifiedSource)
      const encodedSource = Buffer.from(verifiedSource.toString('base64'), 'ascii')
      try {
        const body = JSON.stringify({
          ...parsed.data,
          verifiedSource: { valueBase64: encodedSource.toString('ascii') }
        })
        if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
        try {
          const result = await runtimeRequest(FINALIZE_PATH, 'POST', body)
          return parseSimpleLifecycleResponse(result, 'finalize', parsed.data)
        } catch {
          return failure('runtime_unavailable')
        }
      } finally {
        encodedSource.fill(0)
        verifiedSource.fill(0)
      }
    },

    async issueSourceAuthorityChallenge(
      input: ProviderRegistryLegacyMigrationSourceAuthorityChallengeRequest
    ) {
      const parsed = sourceAuthorityChallengeRequestSchema.safeParse(input)
      if (!parsed.success) return failure('invalid_request')
      const body = JSON.stringify(parsed.data)
      if (Buffer.byteLength(body, 'utf8') > MAX_REQUEST_BYTES) return failure('request_too_large')
      try {
        const result = await runtimeRequest(SOURCE_AUTHORITY_CHALLENGE_PATH, 'POST', body)
        return parseSourceAuthorityChallengeResponse(result)
      } catch {
        return failure('runtime_unavailable')
      }
    },

    async inventory() {
      const body = JSON.stringify({
        schemaVersion: 1,
        confirmation: INVENTORY_CONFIRMATION
      })
      try {
        const result = await runtimeRequest(INVENTORY_PATH, 'POST', body)
        return parseInventoryResponse(result)
      } catch {
        return failure('runtime_unavailable')
      }
    }
  }
}
