import { Buffer } from 'node:buffer'
import { z } from 'zod'
import {
  PROVIDER_REGISTRY_FAILURE_MESSAGES_V1,
  PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1,
  PROVIDER_REGISTRY_MAX_RESPONSE_BYTES_V1,
  PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1,
  PROVIDER_REGISTRY_SCHEMA_VERSION_V1,
  accountCredentialDraftSchemaV1,
  accountCredentialRequestSchemaV1,
  accountCredentialStateSchemaV1,
  parseProviderRegistryPortableManifestV1,
  providerRegistryFailureSchemaV1,
  providerRegistryRequestSchemaV1,
  providerRegistrySuccessSchemaV1,
  type AccountCredentialDraftV1,
  type AccountCredentialRequestV1,
  type AccountCredentialResultV1,
  type AccountCredentialScopeV1,
  type AccountCredentialStateV1,
  type ProviderRegistryFailureCodeV1,
  type ProviderRegistryFailureV1,
  type ProviderRegistryExpectedStateV1,
  type ProviderRegistryRequestV1,
  type ProviderRegistryResultV1,
  type ProviderRegistrySuccessV1
} from '../../../packages/runtime/src/contracts/provider-registry.js'
import type { RuntimeRequestResult } from '../../shared/analytix-api'
import { parseStrictJsonValue } from '../controlled-artifact/strict-json'
import {
  ANALYTIX_PROVIDER_REGISTRY_PATH,
  ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH,
  ANALYTIX_PROVIDER_REGISTRY_RECOVER_PATH,
  analytixProviderRegistryAccountObservationPath,
  analytixProviderRegistryCredentialPath,
  analytixProviderRegistryDiscoverModelsPath,
  analytixProviderRegistryDisconnectPath,
  analytixProviderRegistryProbePath,
  analytixProviderRegistryProviderPath,
  analytixProviderRegistrySelectPath
} from '../../shared/analytix-endpoints'

type ProviderRegistryRuntimeRequest = (
  path: string,
  method?: string,
  body?: string
) => Promise<RuntimeRequestResult>

/**
 * Protected recovery is deliberately a separate Main-only transport.  These
 * values are not added to ProviderRegistryRequestV1: the renderer must never
 * be able to select a private route by passing a generic runtime path.
 */
export type ProtectedRecoveryRuntimeRequest = ProviderRegistryRuntimeRequest

export type ProtectedRecoveryOwnerBinding = {
  owner: 'mcp' | 'extension'
  provider: string
  accountId: string
  channelId: string
  purpose: 'mcp-oauth-access-token' | 'extension-provider-account-token'
  fingerprint: string
  correlation?: string
}

export type ProtectedRecoveryLocalBinding = {
  browserWindowId: string
  mainFrameId: string
  profileBinding: string
  dataDirectoryBinding: string
}

export type ProtectedRecoveryLocalAction = {
  localBinding: ProtectedRecoveryLocalBinding
  confirmation: {
    requestDigest: string
    requestFingerprint: string
    manifestDigest: string
    itemSetDigest: string
    operationId: string
    sessionNonce: string
    expiresAt: string
    localBinding: ProtectedRecoveryLocalBinding
  }
}

export type ProtectedRecoveryImportReceipt = {
  manifestJson: string
  entries: Array<{
    correlation: string
    destinationProviderId: string
    status: 'reentry_required'
    fence: {
      revision: string
      generation: string
      incarnation: string
    }
    destinationOwnerBinding?: ProtectedRecoveryOwnerBinding
  }>
}

export type ProtectedRecoveryRuntimeCall = {
  operation:
    | 'prepare-protected-recovery-request'
    | 'confirm-protected-recovery-destination'
    | 'create-protected-recovery-bundle'
    | 'apply-protected-recovery-bundle'
    | 'finalize-protected-recovery-receipt'
    | 'recover-protected-recovery'
    | 'rollback-protected-recovery'
  importReceipt?: ProtectedRecoveryImportReceipt
  requestBytes?: Uint8Array
  bundleBytes?: Uint8Array
  receiptBytes?: Uint8Array
  localAction?: ProtectedRecoveryLocalAction
  ownerBindingInventory?: ProtectedRecoveryOwnerBinding[]
  localBinding?: ProtectedRecoveryLocalBinding
}

export type ProtectedRecoveryRuntimeResult =
  | {
      ok: true
      status: string
      artifactBytes?: Uint8Array
      requestDigest?: string
      requestFingerprint?: string
      manifestDigest?: string
      itemSetDigest?: string
      operationId?: string
      sessionNonce?: string
      expiresAt?: string
      entryCount?: number
    }
  | {
      ok: false
      code: 'invalid_request' | 'runtime_unavailable' | 'conflict' | 'invalid_response' | 'verification_failure'
    }

type ProviderRegistryRuntimeCall = {
  path: string
  method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE'
  body?: string
}

const PRIVATE_ACCOUNT_STATUS_PATH = `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/account-credentials/status`
const PRIVATE_ACCOUNT_LIST_PATH = `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/account-credentials/list`
const PRIVATE_ACCOUNT_PUT_PATH = `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/account-credentials/put`
const PRIVATE_ACCOUNT_RESOLVE_PATH = `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/account-credentials/resolve`
const PRIVATE_ACCOUNT_MUTATE_PATH = `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/account-credentials/mutate`

const PRIVATE_PROTECTED_RECOVERY_PATHS = {
  'prepare-protected-recovery-request': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/prepare`,
  'confirm-protected-recovery-destination': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/confirm-destination`,
  'create-protected-recovery-bundle': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/create-bundle`,
  'apply-protected-recovery-bundle': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/apply`,
  'finalize-protected-recovery-receipt': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/finalize`,
  'recover-protected-recovery': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/recover`,
  'rollback-protected-recovery': `${ANALYTIX_PROVIDER_REGISTRY_PATH}/_private/protected-recovery/rollback`
} as const

const PROTECTED_RECOVERY_MAX_ARTIFACT_BYTES = 1 << 20
const PROTECTED_RECOVERY_MAX_REQUEST_BYTES = 2 << 20

const runtimeRequestResultSchema = z.object({
  ok: z.boolean(),
  status: z.union([z.literal(0), z.number().int().min(100).max(599)]),
  body: z.string()
}).strict()

const runtimeUnavailableEnvelopeSchema = z.object({
  code: z.literal('fetch_failed'),
  message: z.literal('The Analytix runtime is unavailable.')
}).strict()

const providerRegistryFailureStatuses: Record<Exclude<ProviderRegistryFailureCodeV1,
  'runtime_unavailable' | 'invalid_response'>, number> = {
  invalid_request: 400,
  method_not_allowed: 405,
  not_found: 404,
  conflict: 409,
  persistence_failure: 503,
  verification_failure: 500,
  request_too_large: 413,
  unauthorized: 401
}

function providerRegistryFailure(code: ProviderRegistryFailureCodeV1): ProviderRegistryFailureV1 {
  return {
    schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1,
    error: { code, message: PROVIDER_REGISTRY_FAILURE_MESSAGES_V1[code] }
  }
}

function requestBody(value: unknown): string {
  return JSON.stringify(value)
}

function mapProviderRegistryRequest(request: ProviderRegistryRequestV1): ProviderRegistryRuntimeCall {
  switch (request.operation) {
    case 'list':
      return { path: ANALYTIX_PROVIDER_REGISTRY_PATH, method: 'GET' }
    case 'export-portable-manifest':
      return { path: ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH, method: 'GET' }
    case 'import-portable-manifest':
      return {
        path: ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH,
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, manifestJson: request.manifestJson })
      }
    case 'get':
      return { path: analytixProviderRegistryProviderPath(request.providerId), method: 'GET' }
    case 'connect':
      return {
        path: ANALYTIX_PROVIDER_REGISTRY_PATH,
        method: 'POST',
        body: requestBody({
          schemaVersion: request.schemaVersion,
          expected: request.expected,
          provider: request.provider,
          credential: request.credential
        })
      }
    case 'update':
      return {
        path: analytixProviderRegistryProviderPath(request.providerId),
        method: 'PATCH',
        body: requestBody({
          schemaVersion: request.schemaVersion,
          expected: request.expected,
          provider: request.provider,
          credential: request.credential
        })
      }
    case 'select':
      return {
        path: analytixProviderRegistrySelectPath(request.providerId),
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'disconnect':
      return {
        path: analytixProviderRegistryDisconnectPath(request.providerId),
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'explicit-delete':
      return {
        path: analytixProviderRegistryProviderPath(request.providerId),
        method: 'DELETE',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'credential-replace':
      return {
        path: analytixProviderRegistryCredentialPath(request.providerId),
        method: 'PUT',
        body: requestBody({
          schemaVersion: request.schemaVersion,
          expected: request.expected,
          credential: request.credential
        })
      }
    case 'probe':
      return {
        path: analytixProviderRegistryProbePath(request.providerId),
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'discover-models':
      return {
        path: analytixProviderRegistryDiscoverModelsPath(request.providerId),
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'observe-account':
      return {
        path: analytixProviderRegistryAccountObservationPath(request.providerId),
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion, expected: request.expected })
      }
    case 'recover':
      return {
        path: ANALYTIX_PROVIDER_REGISTRY_RECOVER_PATH,
        method: 'POST',
        body: requestBody({ schemaVersion: request.schemaVersion })
      }
  }
}

function requestCredentialValues(request: ProviderRegistryRequestV1): string[] {
  switch (request.operation) {
    case 'connect':
    case 'credential-replace':
      return [request.credential.valueBase64]
    case 'update':
      return request.credential.kind === 'set' ? [request.credential.valueBase64] : []
    default:
      return []
  }
}

function responseEchoesCredential(body: string, credentialValues: readonly string[]): boolean {
  if (credentialValues.length === 0) return false
  const responseBytes = Buffer.from(body, 'utf8')
  try {
    return credentialValues.some((encoded) => {
      if (body.includes(encoded)) return true
      const credentialBytes = Buffer.from(encoded, 'base64')
      try {
        return credentialBytes.length > 0 && responseBytes.includes(credentialBytes)
      } finally {
        credentialBytes.fill(0)
      }
    })
  } finally {
    responseBytes.fill(0)
  }
}

function parsedValueEchoesCredential(value: unknown, credentialValues: readonly string[]): boolean {
  if (credentialValues.length === 0) return false
  const credentialBytes = credentialValues.map((encoded) => Buffer.from(encoded, 'base64'))
  const visit = (entry: unknown): boolean => {
    if (typeof entry === 'string') {
      if (credentialValues.some((encoded) => entry.includes(encoded))) return true
      const entryBytes = Buffer.from(entry, 'utf8')
      try {
        return credentialBytes.some((credential) =>
          credential.length > 0 && entryBytes.includes(credential)
        )
      } finally {
        entryBytes.fill(0)
      }
    }
    if (Array.isArray(entry)) return entry.some((item) => visit(item))
    if (!entry || typeof entry !== 'object') return false
    return Object.values(entry).some((item) => visit(item))
  }
  try {
    return visit(value)
  } finally {
    credentialBytes.forEach((credential) => credential.fill(0))
  }
}

function containsForbiddenProviderRegistryOutput(body: string): boolean {
  return /"(?:credentialRef|valueBase64|ciphertext|nonce|tag|masterKey|secret|rawBody)"\s*:/i.test(body) ||
    /cred_[A-Za-z0-9_-]{43}/.test(body)
}

function containsForbiddenProviderRegistryValue(value: unknown): boolean {
  if (typeof value === 'string') return /cred_[A-Za-z0-9_-]{43}/.test(value)
  if (Array.isArray(value)) return value.some((entry) => containsForbiddenProviderRegistryValue(entry))
  if (!value || typeof value !== 'object') return false
  return Object.entries(value).some(([key, entry]) =>
    /^(?:credentialRef|valueBase64|ciphertext|nonce|tag|masterKey|secret|rawBody)$/i.test(key) ||
    containsForbiddenProviderRegistryValue(entry)
  )
}

function successMatchesOperation(
  request: ProviderRegistryRequestV1,
  success: ProviderRegistrySuccessV1
): boolean {
  const nextDecimal = (value: string): string | null => {
    const next = BigInt(value) + 1n
    return next <= BigInt(PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1) ? next.toString(10) : null
  }
  const providerMatchesInput = (
    projected: Extract<ProviderRegistrySuccessV1, { provider: unknown }>['provider'],
    input: Extract<ProviderRegistryRequestV1, { operation: 'connect' | 'update' }>['provider']
  ): boolean => projected.id === input.id && projected.kind === input.kind &&
    projected.endpoint === input.endpoint && (projected.proxy ?? '') === input.proxy &&
    JSON.stringify(projected.models) === JSON.stringify(input.models) &&
    JSON.stringify(projected.mediaModels) === JSON.stringify(input.mediaModels) &&
    (projected.selectedModel ?? '') === input.selectedModel &&
    (projected.selectedMediaModel ?? '') === input.selectedMediaModel &&
    JSON.stringify(projected.selectedRoutes) === JSON.stringify(input.selectedRoutes) &&
    JSON.stringify(projected.oauthBinding ?? null) === JSON.stringify(input.oauthBinding ?? null) &&
    JSON.stringify(projected.accountObservation ?? null) === JSON.stringify(input.accountObservation ?? null)
  const existingMutationMatches = (
    projected: Extract<ProviderRegistrySuccessV1, { provider: unknown }>['provider'],
    providerId: string,
    expected: ProviderRegistryExpectedStateV1,
    generation: string
  ): boolean => {
    if (!('registryRevision' in success) || !('registryIncarnation' in success) ||
      typeof success.registryRevision !== 'string' || typeof success.registryIncarnation !== 'string') return false
    return success.registryRevision === nextDecimal(expected.registryRevision) &&
      success.registryIncarnation === expected.registryIncarnation &&
      projected.id === providerId &&
      projected.revision === nextDecimal(expected.providerRevision) &&
      projected.generation === generation && projected.incarnation === expected.providerIncarnation
  }

  switch (request.operation) {
    case 'list':
      return 'providers' in success && !('recovered' in success)
    case 'export-portable-manifest':
      return 'manifestJson' in success && parseProviderRegistryPortableManifestV1(success.manifestJson) !== null
    case 'import-portable-manifest': {
      const manifest = parseProviderRegistryPortableManifestV1(request.manifestJson)
      if (!manifest || !('entries' in success) || !('providerCount' in success) ||
        !('accountCount' in success) || !('reentryRequired' in success)) return false
      const correlations = [...manifest.providers, ...manifest.accounts].map((entry) => entry.correlation)
      if (success.providerCount === manifest.providers.length &&
        success.accountCount === manifest.accounts.length &&
        success.reentryRequired === correlations.length &&
        success.entries.length === correlations.length &&
        success.entries.every((entry, index) => entry.correlation === correlations[index] &&
          entry.status === 'reentry_required')) {
        const destinationProviderIds = new Set<string>()
        for (const entry of success.entries) {
          if (destinationProviderIds.has(entry.destinationProviderId)) return false
          destinationProviderIds.add(entry.destinationProviderId)
        }
        return true
      }
      return false
    }
    case 'recover':
      return 'providers' in success && 'recovered' in success && success.recovered === true
    case 'explicit-delete':
      return 'deletedProviderId' in success && success.deletedProviderId === request.providerId &&
        success.registryRevision === nextDecimal(request.expected.registryRevision) &&
        success.registryIncarnation === request.expected.registryIncarnation
    case 'get':
      return 'provider' in success && success.provider.id === request.providerId
    case 'connect':
      return 'provider' in success && success.registryRevision === nextDecimal(request.expected.registryRevision) &&
        success.registryIncarnation === request.expected.registryIncarnation &&
        success.provider.revision === '1' && success.provider.generation === '1' &&
        success.provider.credentialConfigured &&
        success.provider.credentialPurpose === request.credential.purpose && !success.provider.tombstone &&
        providerMatchesInput(success.provider, request.provider)
    case 'update': {
      if (!('provider' in success) || !providerMatchesInput(success.provider, request.provider)) return false
      const credential = request.credential
      const generation = credential.kind === 'set' || credential.kind === 'unset'
        ? nextDecimal(request.expected.providerGeneration)
        : request.expected.providerGeneration
      const credentialPurpose = credential.kind === 'set'
        ? credential.purpose
        : credential.kind === 'unset' ? '' : request.expected.providerCredentialPurpose
      return generation !== null && existingMutationMatches(
        success.provider,
        request.providerId,
        request.expected,
        generation
      ) &&
        !success.provider.tombstone &&
        success.provider.credentialConfigured === (credentialPurpose !== '') &&
        (success.provider.credentialPurpose ?? '') === credentialPurpose
    }
    case 'select':
      return 'provider' in success && existingMutationMatches(
        success.provider,
        request.providerId,
        request.expected,
        request.expected.providerGeneration
      ) && !success.provider.tombstone &&
        success.provider.credentialConfigured === (request.expected.providerCredentialPurpose !== '') &&
        (success.provider.credentialPurpose ?? '') === request.expected.providerCredentialPurpose
    case 'credential-replace': {
      const generation = nextDecimal(request.expected.providerGeneration)
      return generation !== null && 'provider' in success && existingMutationMatches(
        success.provider,
        request.providerId,
        request.expected,
        generation
      ) &&
        !success.provider.tombstone && success.provider.credentialConfigured &&
        success.provider.credentialPurpose === request.credential.purpose
    }
    case 'disconnect': {
      const generation = nextDecimal(request.expected.providerGeneration)
      return generation !== null && 'provider' in success && existingMutationMatches(
        success.provider,
        request.providerId,
        request.expected,
        generation
      ) &&
        success.provider.tombstone && !success.provider.credentialConfigured &&
        success.provider.credentialPurpose === undefined && success.provider.selectedRoutes.length === 0
    }
    case 'probe':
      return 'status' in success && success.providerId === request.providerId &&
        success.registryRevision === request.expected.registryRevision &&
        success.registryIncarnation === request.expected.registryIncarnation &&
        success.providerRevision === request.expected.providerRevision &&
        success.providerGeneration === request.expected.providerGeneration &&
        success.providerIncarnation === request.expected.providerIncarnation
    case 'discover-models':
      return 'provider' in success && existingMutationMatches(
        success.provider,
        request.providerId,
        request.expected,
        request.expected.providerGeneration
      ) && !success.provider.tombstone && success.provider.credentialConfigured &&
        success.provider.credentialPurpose === request.expected.providerCredentialPurpose
    case 'observe-account':
      return 'observedAt' in success && success.providerId === request.providerId &&
        success.registryRevision === request.expected.registryRevision &&
        success.registryIncarnation === request.expected.registryIncarnation &&
        success.providerRevision === request.expected.providerRevision &&
        success.providerGeneration === request.expected.providerGeneration &&
        success.providerIncarnation === request.expected.providerIncarnation &&
        success.providerCredentialPurpose === request.expected.providerCredentialPurpose
  }
}

function parseProviderRegistryResponse(
  request: ProviderRegistryRequestV1,
  rawResult: unknown
): ProviderRegistryResultV1 {
  const raw = runtimeRequestResultSchema.safeParse(rawResult)
  const credentialValues = requestCredentialValues(request)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > PROVIDER_REGISTRY_MAX_RESPONSE_BYTES_V1 ||
    containsForbiddenProviderRegistryOutput(raw.data.body) ||
    responseEchoesCredential(raw.data.body, credentialValues)) {
    return providerRegistryFailure('invalid_response')
  }

  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return providerRegistryFailure('invalid_response')
  }
  if (containsForbiddenProviderRegistryValue(value) || parsedValueEchoesCredential(value, credentialValues)) {
    return providerRegistryFailure('invalid_response')
  }

  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableEnvelopeSchema.safeParse(value).success
      ? providerRegistryFailure('runtime_unavailable')
      : providerRegistryFailure('invalid_response')
  }

  if (raw.data.ok && raw.data.status === 200) {
    const success = providerRegistrySuccessSchemaV1.safeParse(value)
    return success.success && successMatchesOperation(request, success.data)
      ? success.data
      : providerRegistryFailure('invalid_response')
  }

  if (raw.data.ok || raw.data.status < 400) return providerRegistryFailure('invalid_response')
  const failure = providerRegistryFailureSchemaV1.safeParse(value)
  if (!failure.success || failure.data.error.code === 'runtime_unavailable' ||
    failure.data.error.code === 'invalid_response') {
    return providerRegistryFailure('invalid_response')
  }
  return providerRegistryFailureStatuses[failure.data.error.code] === raw.data.status
    ? failure.data
    : providerRegistryFailure('invalid_response')
}

export function createProviderRegistryIpcHandler(runtimeRequest: ProviderRegistryRuntimeRequest) {
  return async (payload: unknown): Promise<ProviderRegistryResultV1> => {
    const parsed = providerRegistryRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return providerRegistryFailure('invalid_request')
    const call = mapProviderRegistryRequest(parsed.data)
    if (call.body !== undefined && Buffer.byteLength(call.body, 'utf8') > PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1) {
      return providerRegistryFailure('request_too_large')
    }
    try {
      const response = call.body === undefined
        ? await runtimeRequest(call.path, call.method)
        : await runtimeRequest(call.path, call.method, call.body)
      return parseProviderRegistryResponse(parsed.data, response)
    } catch {
      return providerRegistryFailure('runtime_unavailable')
    }
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function canonicalBase64(value: unknown, maximumBytes = PROTECTED_RECOVERY_MAX_ARTIFACT_BYTES): Buffer | null {
  if (typeof value !== 'string' || value.length === 0 || value.length > maximumBytes * 2) return null
  if (!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return null
  const decoded = Buffer.from(value, 'base64')
  if (decoded.length === 0 || decoded.length > maximumBytes || decoded.toString('base64') !== value) {
    decoded.fill(0)
    return null
  }
  return decoded
}

function validProtectedRecoveryDigest(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
}

function validProtectedRecoveryFingerprint(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{16}$/.test(value)
}

function validProtectedRecoveryToken(value: unknown, minimum: number, maximum: number): value is string {
  return typeof value === 'string' && value.length >= minimum && value.length <= maximum &&
    value === value.trim() && !/[\\\u0000\r\n\t]/.test(value) &&
    !value.includes('/') && !value.includes('..') && Buffer.byteLength(value, 'utf8') <= maximum
}

function validProtectedRecoveryExpiry(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 64 &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
}

function validProtectedRecoveryFence(value: unknown): value is ProtectedRecoveryImportReceipt['entries'][number]['fence'] {
  if (!isRecord(value) || Object.keys(value).length !== 3) return false
  const counter = (entry: unknown): entry is string =>
    typeof entry === 'string' && /^[1-9][0-9]{0,19}$/.test(entry)
  return counter(value.revision) && counter(value.generation) &&
    typeof value.incarnation === 'string' && /^inc_[A-Za-z0-9_-]{43}$/.test(value.incarnation)
}

type ProtectedRecoveryFailureCode =
  | 'invalid_request'
  | 'runtime_unavailable'
  | 'conflict'
  | 'invalid_response'
  | 'verification_failure'

function protectedRecoveryFailure(code: ProtectedRecoveryFailureCode): ProtectedRecoveryRuntimeResult {
  return { ok: false, code }
}

function protectedRecoveryOwnerInventory(
  inventory: readonly ProtectedRecoveryOwnerBinding[] | undefined,
  requireCorrelation = true
): ProtectedRecoveryOwnerBinding[] | null {
  if (!inventory || inventory.length > 128) return null
  const sorted = inventory.map((entry) => ({ ...entry })).sort((left, right) => (
    `${left.owner}\0${left.provider}\0${left.accountId}\0${left.channelId}\0${left.purpose}\0${left.fingerprint}`
      .localeCompare(`${right.owner}\0${right.provider}\0${right.accountId}\0${right.channelId}\0${right.purpose}\0${right.fingerprint}`)
  ))
  const seen = new Set<string>()
  const seenBindings = new Set<string>()
  const normalized: ProtectedRecoveryOwnerBinding[] = []
  for (const entry of sorted) {
    if (!isRecord(entry) || !Object.keys(entry).every((key) => new Set([
      'owner', 'provider', 'accountId', 'channelId', 'purpose', 'fingerprint', 'correlation'
    ]).has(key)) ||
      (entry.owner !== 'mcp' && entry.owner !== 'extension') ||
      typeof entry.provider !== 'string' || typeof entry.accountId !== 'string' ||
      typeof entry.channelId !== 'string' || typeof entry.purpose !== 'string' ||
      typeof entry.fingerprint !== 'string' ||
      (entry.owner === 'mcp' && entry.purpose !== 'mcp-oauth-access-token') ||
      (entry.owner === 'extension' && entry.purpose !== 'extension-provider-account-token')) return null
    const bindingKey = `${entry.owner}\0${entry.provider}\0${entry.accountId}\0${entry.channelId}\0${entry.purpose}`
    if (seenBindings.has(bindingKey)) return null
    seenBindings.add(bindingKey)
    const correlation = entry.correlation
    if (correlation === undefined) {
      if (requireCorrelation) return null
      normalized.push({ ...entry })
      continue
    }
    if (typeof correlation !== 'string' || !/^(?:provider|account)-[0-9]+$/.test(correlation) || seen.has(correlation)) return null
    seen.add(correlation)
    normalized.push({ ...entry, correlation })
  }
  return normalized
}

function protectedRecoveryImportResult(receipt: ProtectedRecoveryImportReceipt): Record<string, unknown> | null {
  if (!receipt || typeof receipt.manifestJson !== 'string' ||
    Buffer.byteLength(receipt.manifestJson, 'utf8') === 0 ||
    Buffer.byteLength(receipt.manifestJson, 'utf8') > PROTECTED_RECOVERY_MAX_ARTIFACT_BYTES ||
    !Array.isArray(receipt.entries) || receipt.entries.length === 0 || receipt.entries.length > 128) {
    return null
  }
  const manifest = parseProviderRegistryPortableManifestV1(receipt.manifestJson)
  if (!manifest) return null
  if (receipt.entries.some((entry) => !validProtectedRecoveryFence(entry.fence))) return null
  const entries = receipt.entries.map((entry) => ({
    correlation: entry.correlation,
    destinationProviderId: entry.destinationProviderId,
    status: entry.status,
    fence: { ...entry.fence },
    ...(entry.destinationOwnerBinding
      ? { destinationOwnerBinding: { ...entry.destinationOwnerBinding } }
      : {})
  }))
  const expectedCorrelations = [...manifest.providers, ...manifest.accounts].map((entry) => entry.correlation)
  if (entries.length !== expectedCorrelations.length || entries.some((entry, index) =>
    typeof entry.correlation !== 'string' || entry.correlation !== expectedCorrelations[index] ||
    typeof entry.destinationProviderId !== 'string' || !/^[a-z0-9][a-z0-9._-]{0,95}$/.test(entry.destinationProviderId) ||
    entry.status !== 'reentry_required')) return null
  if (new Set(entries.map((entry) => entry.destinationProviderId)).size !== entries.length) return null
  const providerCount = manifest.providers.length
  for (const [index, entry] of entries.entries()) {
    if (index < providerCount) {
      if ('destinationOwnerBinding' in entry) return null
      continue
    }
    if (!('destinationOwnerBinding' in entry)) return null
    const descriptor = manifest.accounts[index - providerCount]
    const binding = entry.destinationOwnerBinding
    if (!binding) return null
    const normalized = protectedRecoveryOwnerInventory([binding], true)
    if (!descriptor || !normalized || normalized.length !== 1 ||
      normalized[0].correlation !== entry.correlation ||
      normalized[0].owner !== descriptor.owner || normalized[0].provider !== descriptor.provider ||
      normalized[0].purpose !== descriptor.purpose) return null
    entry.destinationOwnerBinding = normalized[0]
  }
  return {
    providerCount,
    accountCount: entries.length - providerCount,
    reentryRequired: entries.length,
    entries
  }
}

function canonicalProtectedRecoveryJSON(value: unknown): string {
  return JSON.stringify(value)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function protectedRecoveryBody(
  call: ProtectedRecoveryRuntimeCall
): { path: string; body?: string } | null {
  const path = PRIVATE_PROTECTED_RECOVERY_PATHS[call.operation]
  if (!path) return null
  const requireOwnerCorrelation = call.operation === 'prepare-protected-recovery-request' ||
    call.operation === 'create-protected-recovery-bundle'
  const ownerBindingInventory = protectedRecoveryOwnerInventory(
    call.ownerBindingInventory,
    requireOwnerCorrelation
  )
  if (call.ownerBindingInventory !== undefined && ownerBindingInventory === null) return null
  const localAction = call.localAction
  const localBinding = call.localBinding
  const encode = (bytes: Uint8Array | undefined): string | null => {
    if (!bytes || bytes.length === 0 || bytes.length > PROTECTED_RECOVERY_MAX_ARTIFACT_BYTES) return null
    return Buffer.from(bytes).toString('base64')
  }
  switch (call.operation) {
    case 'prepare-protected-recovery-request': {
      if (!call.importReceipt || !localBinding) return null
      const manifestBytes = Buffer.from(call.importReceipt.manifestJson, 'utf8')
      const manifestBase64 = encode(manifestBytes)
      const importResult = protectedRecoveryImportResult(call.importReceipt)
      if (!manifestBase64 || !importResult) return null
      return {
        path,
        body: requestBody({
          schemaVersion: 1,
          manifestBase64,
          importResult,
          localBinding,
          ownerBindingInventory: ownerBindingInventory ?? []
        })
      }
    }
    case 'confirm-protected-recovery-destination':
    case 'create-protected-recovery-bundle': {
      const requestBase64 = encode(call.requestBytes)
      if (!requestBase64 || !localAction) return null
      return {
        path,
        body: requestBody({
          schemaVersion: 1,
          requestBase64,
          localAction,
          ownerBindingInventory: ownerBindingInventory ?? []
        })
      }
    }
    case 'apply-protected-recovery-bundle': {
      const bundleBase64 = encode(call.bundleBytes)
      const requestBase64 = call.requestBytes && encode(call.requestBytes)
      if (!bundleBase64 || call.requestBytes && !requestBase64) return null
      return {
        path,
        body: requestBody({
          schemaVersion: 1,
          bundleBase64,
          ...(requestBase64 ? { requestBase64 } : {}),
          ownerBindingInventory: ownerBindingInventory ?? []
        })
      }
    }
    case 'finalize-protected-recovery-receipt': {
      const receiptBase64 = encode(call.receiptBytes)
      return receiptBase64 ? { path, body: requestBody({ schemaVersion: 1, receiptBase64 }) } : null
    }
    case 'recover-protected-recovery': {
      const requestBase64 = call.requestBytes && encode(call.requestBytes)
      if (call.requestBytes && !requestBase64) return null
      if (requestBase64 && (!localAction || !ownerBindingInventory)) return null
      return {
        path,
        body: requestBody({
          schemaVersion: 1,
          ...(requestBase64 ? { requestBase64 } : {}),
          ...(localAction ? { localAction } : {}),
          ...(requestBase64 ? { ownerBindingInventory: ownerBindingInventory ?? [] } : {})
        })
      }
    }
    case 'rollback-protected-recovery': {
      const requestBase64 = encode(call.requestBytes)
      return requestBase64 ? { path, body: requestBody({ schemaVersion: 1, requestBase64 }) } : null
    }
  }
}

function parseProtectedRecoveryResponse(
  operation: ProtectedRecoveryRuntimeCall['operation'],
  rawResult: unknown
): ProtectedRecoveryRuntimeResult {
  const raw = runtimeRequestResultSchema.safeParse(rawResult)
  const responseBody = raw.success && raw.data.body.endsWith('\n')
    ? raw.data.body.slice(0, -1)
    : raw.success ? raw.data.body : ''
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > PROTECTED_RECOVERY_MAX_REQUEST_BYTES ||
    containsForbiddenProviderRegistryOutput(raw.data.body)) {
    return protectedRecoveryFailure('invalid_response')
  }
  if (raw.data.status === 0) {
    let unavailable: unknown
    try {
      unavailable = JSON.parse(responseBody)
    } catch {
      return protectedRecoveryFailure('invalid_response')
    }
    return !raw.data.ok && runtimeUnavailableEnvelopeSchema.safeParse(unavailable).success
      ? protectedRecoveryFailure('runtime_unavailable')
      : protectedRecoveryFailure('invalid_response')
  }
  if (!raw.data.ok || raw.data.status !== 200) {
    let failureValue: unknown
    try {
      failureValue = JSON.parse(responseBody)
    } catch {
      return protectedRecoveryFailure('invalid_response')
    }
    const failure = providerRegistryFailureSchemaV1.safeParse(failureValue)
    const failureCode = failure.success ? failure.data.error.code : null
    const failureStatus = failureCode !== null && failureCode in providerRegistryFailureStatuses
      ? providerRegistryFailureStatuses[failureCode as keyof typeof providerRegistryFailureStatuses]
      : undefined
    if (!failure.success || canonicalProtectedRecoveryJSON(failure.data) !== responseBody ||
      failureStatus !== raw.data.status) {
      return protectedRecoveryFailure('invalid_response')
    }
    switch (failure.data.error.code) {
      case 'invalid_request':
      case 'not_found':
      case 'unauthorized':
      case 'method_not_allowed':
      case 'request_too_large':
        return protectedRecoveryFailure('invalid_request')
      case 'conflict':
        return protectedRecoveryFailure('conflict')
      case 'persistence_failure':
        return protectedRecoveryFailure('runtime_unavailable')
      case 'verification_failure':
        return protectedRecoveryFailure('verification_failure')
      case 'runtime_unavailable':
      case 'invalid_response':
        return protectedRecoveryFailure('invalid_response')
    }
  }
  let value: unknown
  try {
    const responseBytes = Buffer.from(responseBody, 'utf8')
    try {
      value = parseStrictJsonValue(responseBytes, {
        maxBytes: PROTECTED_RECOVERY_MAX_REQUEST_BYTES,
        maxDepth: 16,
        maxTokens: 32768,
        maxStringBytes: 65536,
        maxNumberBytes: 64
      })
    } finally {
      responseBytes.fill(0)
    }
  } catch {
    return protectedRecoveryFailure('invalid_response')
  }
  if (!isRecord(value) || value.schemaVersion !== 1 || containsForbiddenProviderRegistryValue(value) ||
    canonicalProtectedRecoveryJSON(value) !== responseBody) {
    return protectedRecoveryFailure('invalid_response')
  }
  const keys = new Set(Object.keys(value))
  const exact = (expected: string[]): boolean => keys.size === expected.length && expected.every((key) => keys.has(key))
  if (operation === 'prepare-protected-recovery-request') {
    if (!exact(['schemaVersion', 'requestBase64', 'requestDigest', 'requestFingerprint', 'manifestDigest', 'itemSetDigest', 'operationId', 'sessionNonce', 'expiresAt'])) {
      return protectedRecoveryFailure('invalid_response')
    }
    const requestBytes = canonicalBase64(value.requestBase64)
    if (!requestBytes || !validProtectedRecoveryDigest(value.requestDigest) ||
      !validProtectedRecoveryFingerprint(value.requestFingerprint) ||
      !validProtectedRecoveryDigest(value.manifestDigest) ||
      !validProtectedRecoveryDigest(value.itemSetDigest) ||
      !validProtectedRecoveryToken(value.operationId, 8, 128) ||
      !validProtectedRecoveryToken(value.sessionNonce, 16, 128) ||
      !validProtectedRecoveryExpiry(value.expiresAt)) {
      requestBytes?.fill(0)
      return protectedRecoveryFailure('invalid_response')
    }
    return {
      ok: true, status: 'request-prepared', artifactBytes: requestBytes,
      requestDigest: value.requestDigest as string, requestFingerprint: value.requestFingerprint as string,
      manifestDigest: value.manifestDigest as string, itemSetDigest: value.itemSetDigest as string,
      operationId: value.operationId as string, sessionNonce: value.sessionNonce as string,
      expiresAt: value.expiresAt as string
    }
  }
  if (operation === 'create-protected-recovery-bundle') {
    if (!exact(['schemaVersion', 'bundleBase64'])) return protectedRecoveryFailure('invalid_response')
    const artifactBytes = canonicalBase64(value.bundleBase64)
    return artifactBytes ? { ok: true, status: 'bundle-created', artifactBytes } : protectedRecoveryFailure('invalid_response')
  }
  if (operation === 'apply-protected-recovery-bundle') {
    if (!exact(['schemaVersion', 'receiptBase64'])) return protectedRecoveryFailure('invalid_response')
    const artifactBytes = canonicalBase64(value.receiptBase64)
    return artifactBytes ? { ok: true, status: 'receipt-created', artifactBytes } : protectedRecoveryFailure('invalid_response')
  }
  if (operation === 'confirm-protected-recovery-destination') {
    if (!exact(['schemaVersion', 'status', 'confirmed']) || value.status !== 'confirmed' || value.confirmed !== true) {
      return protectedRecoveryFailure('invalid_response')
    }
    return { ok: true, status: 'destination-confirmed' }
  }
  if (operation === 'finalize-protected-recovery-receipt') {
    if (!exact(['schemaVersion', 'status', 'confirmed']) || value.status !== 'finalized' || typeof value.confirmed !== 'boolean') {
      return protectedRecoveryFailure('invalid_response')
    }
    return { ok: true, status: 'finalized' }
  }
  if (operation === 'recover-protected-recovery') {
    const statuses = new Set(['none', 'pending', 'reconfirmation_required', 'receipt_ready'])
    const responseKeys = Object.keys(value)
    const receiptBytes = value.status === 'receipt_ready' ? canonicalBase64(value.receiptBase64) : null
    const statusConfirmationIsExact = value.status === 'receipt_ready'
      ? value.confirmed === true
      : value.status === 'reconfirmation_required'
        ? value.confirmed === false
        : typeof value.confirmed === 'boolean'
    if ((!exact(['schemaVersion', 'status', 'confirmed']) &&
      !exact(['schemaVersion', 'status', 'confirmed', 'receiptBase64'])) ||
      typeof value.status !== 'string' || !statuses.has(value.status) || !statusConfirmationIsExact ||
      (value.status === 'receipt_ready' && (!('receiptBase64' in value) || !receiptBytes)) ||
      (value.status !== 'receipt_ready' && responseKeys.includes('receiptBase64'))) {
      receiptBytes?.fill(0)
      return protectedRecoveryFailure('invalid_response')
    }
    const result = { ok: true as const, status: value.status, ...(receiptBytes ? { artifactBytes: receiptBytes } : {}) }
    return result
  }
  if (!exact(['schemaVersion', 'status', 'confirmed']) || value.status !== 'rolled_back' || value.confirmed !== false) {
    return protectedRecoveryFailure('invalid_response')
  }
  return { ok: true, status: 'rolled_back' }
}

/** Create the only Main-side client for protected recovery HTTP operations. */
export function createProtectedRecoveryRuntimeClient(
  runtimeRequest: ProtectedRecoveryRuntimeRequest
): (call: ProtectedRecoveryRuntimeCall) => Promise<ProtectedRecoveryRuntimeResult> {
  return async (call) => {
    const request = protectedRecoveryBody(call)
    if (!request || !request.body || Buffer.byteLength(request.body, 'utf8') > PROTECTED_RECOVERY_MAX_REQUEST_BYTES) {
      return protectedRecoveryFailure('invalid_request')
    }
    try {
      const raw = await runtimeRequest(request.path, 'POST', request.body)
      return parseProtectedRecoveryResponse(call.operation, raw)
    } catch {
      return protectedRecoveryFailure('runtime_unavailable')
    }
  }
}

// Descriptive aliases keep the operation-specific boundary easy to discover
// without creating another generic runtime request surface.
export const createMainProtectedRecoveryRuntimeClient = createProtectedRecoveryRuntimeClient
export const createProviderCredentialRecoveryRuntimeClient = createProtectedRecoveryRuntimeClient

export function accountCredentialExpectedState(
  state: AccountCredentialStateV1
): ProviderRegistryExpectedStateV1 {
  return {
    registryRevision: state.registryRevision,
    registryIncarnation: state.registryIncarnation,
    providerRevision: state.providerRevision,
    providerGeneration: state.providerGeneration,
    providerIncarnation: state.providerIncarnation,
    providerCredentialPurpose: state.credentialPurpose ?? (state.status === 'ready' ? state.scope.purpose : '')
  }
}

function accountCredentialFailureFromRuntime(rawResult: unknown): ProviderRegistryFailureV1 | null {
  const raw = runtimeRequestResultSchema.safeParse(rawResult)
  if (!raw.success || Buffer.byteLength(raw.data.body, 'utf8') > PROVIDER_REGISTRY_MAX_RESPONSE_BYTES_V1) {
    return providerRegistryFailure('invalid_response')
  }
  let value: unknown
  try {
    value = JSON.parse(raw.data.body)
  } catch {
    return providerRegistryFailure('invalid_response')
  }
  if (raw.data.status === 0) {
    return !raw.data.ok && runtimeUnavailableEnvelopeSchema.safeParse(value).success
      ? providerRegistryFailure('runtime_unavailable')
      : providerRegistryFailure('invalid_response')
  }
  if (raw.data.ok && raw.data.status === 200) return null
  if (raw.data.ok || raw.data.status < 400) return providerRegistryFailure('invalid_response')
  const failure = providerRegistryFailureSchemaV1.safeParse(value)
  if (!failure.success || failure.data.error.code === 'runtime_unavailable' ||
    failure.data.error.code === 'invalid_response') {
    return providerRegistryFailure('invalid_response')
  }
  return providerRegistryFailureStatuses[failure.data.error.code] === raw.data.status
    ? failure.data
    : providerRegistryFailure('invalid_response')
}

function parseAccountCredentialStateResponse(
  rawResult: unknown,
  forbiddenValues: readonly string[] = []
): AccountCredentialResultV1 {
  const failure = accountCredentialFailureFromRuntime(rawResult)
  if (failure) return failure
  const raw = runtimeRequestResultSchema.parse(rawResult)
  if (containsForbiddenProviderRegistryOutput(raw.body) ||
    forbiddenValues.some((value) => value !== '' && raw.body.includes(value))) {
    return providerRegistryFailure('invalid_response')
  }
  try {
    const value = JSON.parse(raw.body)
    const parsed = accountCredentialStateSchemaV1.safeParse(value)
    return parsed.success ? parsed.data : providerRegistryFailure('invalid_response')
  } catch {
    return providerRegistryFailure('invalid_response')
  }
}

function encodeAccountCredentialDraft(credential: AccountCredentialDraftV1): {
  valueBase64: string
  rawValues: string[]
} {
  const rawValues = Object.entries(credential)
    .filter(([key]) => key !== 'kind')
    .map(([, value]) => value)
    .filter((value): value is string => typeof value === 'string' && value !== '')
  const bytes = Buffer.from(JSON.stringify(credential), 'utf8')
  try {
    return { valueBase64: bytes.toString('base64'), rawValues }
  } finally {
    bytes.fill(0)
  }
}

function accountCredentialCall(request: AccountCredentialRequestV1): {
  path: string
  body: string
  forbiddenValues: string[]
} {
  switch (request.operation) {
    case 'status':
      return {
        path: PRIVATE_ACCOUNT_STATUS_PATH,
        body: requestBody({ schemaVersion: request.schemaVersion, scope: request.scope }),
        forbiddenValues: []
      }
    case 'put': {
      const encoded = encodeAccountCredentialDraft(request.credential)
      return {
        path: PRIVATE_ACCOUNT_PUT_PATH,
        body: requestBody({
          schemaVersion: request.schemaVersion,
          scope: request.scope,
          expected: request.expected,
          valueBase64: encoded.valueBase64
        }),
        forbiddenValues: [...encoded.rawValues, encoded.valueBase64]
      }
    }
    case 'revoke':
    case 'disconnect':
    case 'delete':
      return {
        path: PRIVATE_ACCOUNT_MUTATE_PATH,
        body: requestBody({
          schemaVersion: request.schemaVersion,
          scope: request.scope,
          expected: request.expected,
          disposition: request.operation
        }),
        forbiddenValues: []
      }
  }
}

export function createAccountCredentialIpcHandler(
  runtimeRequest: ProviderRegistryRuntimeRequest,
  options: { allowPut?: (scope: AccountCredentialScopeV1, credential: AccountCredentialDraftV1) => boolean } = {}
) {
  return async (payload: unknown): Promise<AccountCredentialResultV1> => {
    const parsed = accountCredentialRequestSchemaV1.safeParse(payload)
    if (!parsed.success) return providerRegistryFailure('invalid_request')
    if (
      parsed.data.operation === 'put' &&
      options.allowPut &&
      !options.allowPut(parsed.data.scope, parsed.data.credential)
    ) {
      return providerRegistryFailure('invalid_request')
    }
    const call = accountCredentialCall(parsed.data)
    if (Buffer.byteLength(call.body, 'utf8') > PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1) {
      return providerRegistryFailure('request_too_large')
    }
    try {
      const response = await runtimeRequest(call.path, 'POST', call.body)
      return parseAccountCredentialStateResponse(response, call.forbiddenValues)
    } catch {
      return providerRegistryFailure('runtime_unavailable')
    }
  }
}

export type MainAccountCredentialResolution =
  | { ok: true; state: AccountCredentialStateV1; credential: AccountCredentialDraftV1 }
  | { ok: false; code: ProviderRegistryFailureCodeV1 }

function accountCredentialScopesEqual(
  left: AccountCredentialScopeV1,
  right: AccountCredentialScopeV1
): boolean {
  return left.owner === right.owner &&
    left.provider === right.provider &&
    left.accountId === right.accountId &&
    (left.channelId ?? '') === (right.channelId ?? '') &&
    left.purpose === right.purpose
}

export function createMainAccountCredentialResolver(runtimeRequest: ProviderRegistryRuntimeRequest) {
  return async (scope: AccountCredentialScopeV1): Promise<MainAccountCredentialResolution> => {
    const scopeResult = accountCredentialRequestSchemaV1.safeParse({
      schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1,
      operation: 'status',
      scope
    })
    if (!scopeResult.success) return { ok: false, code: 'invalid_request' }
    const body = requestBody({ schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1, scope })
    if (Buffer.byteLength(body, 'utf8') > PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1) {
      return { ok: false, code: 'request_too_large' }
    }
    let rawResult: RuntimeRequestResult
    try {
      rawResult = await runtimeRequest(PRIVATE_ACCOUNT_RESOLVE_PATH, 'POST', body)
    } catch {
      return { ok: false, code: 'runtime_unavailable' }
    }
    const failure = accountCredentialFailureFromRuntime(rawResult)
    if (failure) return { ok: false, code: failure.error.code }
    const raw = runtimeRequestResultSchema.parse(rawResult)
    let value: unknown
    try {
      value = JSON.parse(raw.body)
    } catch {
      return { ok: false, code: 'invalid_response' }
    }
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return { ok: false, code: 'invalid_response' }
    }
    const { valueBase64, ...stateValue } = value as Record<string, unknown>
    const state = accountCredentialStateSchemaV1.safeParse(stateValue)
    if (
      !state.success ||
      state.data.status !== 'ready' ||
      !accountCredentialScopesEqual(state.data.scope, scope) ||
      typeof valueBase64 !== 'string'
    ) {
      return { ok: false, code: 'invalid_response' }
    }
    const credentialBytes = Buffer.from(valueBase64, 'base64')
    try {
      if (credentialBytes.length === 0 || credentialBytes.toString('base64') !== valueBase64) {
        return { ok: false, code: 'invalid_response' }
      }
      const credential = accountCredentialDraftSchemaV1.safeParse(JSON.parse(credentialBytes.toString('utf8')))
      if (!credential.success) return { ok: false, code: 'invalid_response' }
      const purposeBinding = accountCredentialRequestSchemaV1.safeParse({
        schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1,
        operation: 'put',
        scope,
        expected: accountCredentialExpectedState(state.data),
        credential: credential.data
      })
      return purposeBinding.success
        ? { ok: true, state: state.data, credential: credential.data }
        : { ok: false, code: 'invalid_response' }
    } catch {
      return { ok: false, code: 'invalid_response' }
    } finally {
      credentialBytes.fill(0)
    }
  }
}

const privateAccountListResponseSchema = z.object({
  schemaVersion: z.literal(PROVIDER_REGISTRY_SCHEMA_VERSION_V1),
  accounts: z.array(accountCredentialStateSchemaV1).max(128)
}).strict()

export function createMainAccountCredentialLister(runtimeRequest: ProviderRegistryRuntimeRequest) {
  return async (
    owner: 'provider' | 'mcp' | 'extension',
    purpose: 'provider-oauth-authorization-state' | 'provider-oauth-token-bundle' |
      'mcp-oauth-authorization-state' | 'mcp-oauth-access-token' |
      'extension-provider-account-token' | 'extension-oauth-authorization-state'
  ): Promise<AccountCredentialStateV1[]> => {
    const coherent = (owner === 'provider' && purpose === 'provider-oauth-authorization-state') ||
      (owner === 'provider' && purpose === 'provider-oauth-token-bundle') ||
      (owner === 'mcp' && purpose === 'mcp-oauth-authorization-state') ||
      (owner === 'mcp' && purpose === 'mcp-oauth-access-token') ||
      (owner === 'extension' && (purpose === 'extension-provider-account-token' ||
        purpose === 'extension-oauth-authorization-state'))
    if (!coherent) throw new Error('Private account inventory is unavailable.')
    const body = requestBody({ schemaVersion: PROVIDER_REGISTRY_SCHEMA_VERSION_V1, owner, purpose })
    if (Buffer.byteLength(body, 'utf8') > PROVIDER_REGISTRY_MAX_REQUEST_BYTES_V1) {
      throw new Error('Private account inventory is unavailable.')
    }
    let rawResult: RuntimeRequestResult
    try {
      rawResult = await runtimeRequest(PRIVATE_ACCOUNT_LIST_PATH, 'POST', body)
    } catch {
      throw new Error('Private account inventory is unavailable.')
    }
    const failure = accountCredentialFailureFromRuntime(rawResult)
    if (failure) throw new Error('Private account inventory is unavailable.')
    try {
      const parsed = privateAccountListResponseSchema.safeParse(JSON.parse(rawResult.body))
      if (!parsed.success || parsed.data.accounts.some((account) =>
        account.scope.owner !== owner || account.scope.purpose !== purpose
      )) throw new Error()
      return parsed.data.accounts
    } catch {
      throw new Error('Private account inventory is unavailable.')
    }
  }
}
