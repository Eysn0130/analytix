import {
  PublicToolCallArgumentsProjectionV1,
  PublicToolResultProjectionV1,
  ToolCallTurnItem,
  ToolResultTurnItem
} from '../../packages/runtime/src/contracts/items.js'
import {
  AcceptedFinalDeliveryBatchV2Schema,
  GeneralTerminalDeliveryBatchV1Schema,
  GeneralTerminalErrorItemV1Schema,
  isToolNotAdvertisedDiagnosticDetailsV1
} from '../../packages/runtime/src/contracts/events.js'
import { PublicModelReasoningV2 } from '../../packages/runtime/src/contracts/runtime-info.js'
import { containsRestrictedEvidence } from './restricted-evidence-projection'
import {
  containsInternalCaseEntityReference,
  containsOrdinaryPublicPII,
  containsProtectedCaseFactCandidate
} from './ordinary-log-pii-projection'
import { containsSecretMaterial } from './secret-redaction'

const PRIVATE_REASONING_KINDS = new Set([
  'assistant_reasoning',
  'assistant_reasoning_delta',
  'agent_reasoning'
])

const PUBLIC_REASONING_EFFORTS = new Set(['auto', 'off', 'low', 'medium', 'high', 'max'])

const PRIVATE_TERMINAL_CONTRACT_IDENTITIES = new Set([
  'general-terminal-publication.v1',
  'general-terminal-publication-archive.v1',
  'general-terminal-cas-binding.v1',
  'analytix.general-terminal-publication/v1',
  'analytix.general-terminal-publication-archive/v1',
  'analytix.general-terminal-cas-binding/v1'
])

const PRIVATE_REASONING_FIELD = /(?:["']?)(?:assistant_reasoning|assistantReasoning|reasoning_content|reasoningContent|thinking_content|thinkingContent|reasoning|thinking)(?:["']?)\s*[:=]/i

const PRIVATE_RUNTIME_DIAGNOSTIC_KEYS = new Set([
  'autocontinueerror',
  'completiondeliveryerror',
  'deliveryerror',
  'errorraw',
  'errortext',
  'evidenceledgererror',
  'outputpreview',
  'rawerror',
  'responsebody',
  'stack',
  'stacktrace',
  'stderr',
  'stdout',
  'traceback'
])

const PRIVATE_ACCEPTED_FINAL_STRONG_KEYS = new Set([
  'acceptedfinal',
  'factfinalwitnessadmission',
  'publicationsnapshotproof',
  'publicationsnapshotproofdigest'
])

const SAFE_ERROR_DETAIL_KEYS = new Set([
  'status',
  'providerId',
  'provider',
  'family',
  'model',
  'endpointFormat',
  'kind',
  'authStatus',
  'hasApiKey',
  'retryable',
  'attempt',
  'maxAttempt',
  'port',
  'backend',
  'requestedBackend',
  'autoStart',
  'phase'
])

function normalizedKey(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]/g, '')
}

function isReasoningLikeKey(key: string): boolean {
  const normalized = normalizedKey(key)
  return normalized.includes('reasoning') || normalized.includes('thinking')
}

function publicReasoningMetadataState(
  key: string,
  value: unknown
): 'unrecognized' | 'legacy-empty' | 'valid' | 'invalid' {
  switch (normalizedKey(key)) {
    case 'reasoning':
      return PublicModelReasoningV2.safeParse(value).success ? 'valid' : 'invalid'
    case 'reasoningtokens':
    case 'firstreasoninglatencyms':
      return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? 'valid' : 'invalid'
    case 'reasoningeffort':
      if (value === '') return 'legacy-empty'
      return typeof value === 'string' && value === value.trim() && PUBLIC_REASONING_EFFORTS.has(value)
        ? 'valid'
        : 'invalid'
    case 'reasoningexcluded':
      return value === true ? 'valid' : 'invalid'
    case 'reasoningexclusionproof':
      return typeof value === 'string' && /^sha256:[a-f0-9]{64}$/.test(value) ? 'valid' : 'invalid'
    case 'reasoningtoken':
    case 'reasoningdurationms':
    case 'reasoningstartedat':
    case 'reasoningfinishedat':
      return 'invalid'
    default:
      return 'unrecognized'
  }
}

function hasInvalidPublicReasoningMetadata(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasInvalidPublicReasoningMetadata)
  if (!value || typeof value !== 'object') return false
  return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
    publicReasoningMetadataState(key, entry) === 'invalid' || hasInvalidPublicReasoningMetadata(entry)
  )
}

function isPrivateTerminalAuthorityKey(key: string): boolean {
  return normalizedKey(key).startsWith('generalterminal')
}

function isPrivateTerminalAuthorityObject(record: Record<string, unknown>): boolean {
  return [record.schemaVersion, record.purpose]
    .some((value) => typeof value === 'string' && PRIVATE_TERMINAL_CONTRACT_IDENTITIES.has(value.trim()))
}

function containsPotentialReasoningMarkup(value: string): boolean {
  const lower = value.toLowerCase()
  for (let index = lower.indexOf('<'); index >= 0; index = lower.indexOf('<', index + 1)) {
    const remaining = lower.slice(index)
    for (const name of ['think', 'analysis', 'reasoning']) {
      for (const marker of [`<${name}`, `</${name}`]) {
        if (remaining.length > 1 && marker.startsWith(remaining)) return true
        if (!remaining.startsWith(marker)) continue
        if (remaining.length === marker.length) return true
        const boundary = remaining[marker.length]
        if (boundary === '>' || boundary === '/' || /[\t\n\f\r ]/.test(boundary ?? '')) return true
      }
    }
  }
  return false
}

/**
 * The Go runtime is the only reasoning-markup decoder. Desktop boundaries do
 * not try to repair or incrementally reveal provider text: a complete value
 * containing reasoning markup, restricted evidence, or a closed internal
 * reference is withheld in full, while clean bytes are preserved exactly.
 */
export function sanitizePublicAssistantText(text: string): string {
  return containsPotentialReasoningMarkup(text) || containsRestrictedEvidence(text) ||
    containsInternalCaseEntityReference(text)
    ? ''
    : text
}

export function sanitizePublicSerializedText(value: string): string {
  if (containsPotentialReasoningMarkup(value) || PRIVATE_REASONING_FIELD.test(value) ||
      containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value)) return ''
  return value
}

function isAssistantTextRecord(record: Record<string, unknown>): boolean {
  const kind = typeof record.kind === 'string' ? record.kind : ''
  const role = typeof record.role === 'string' ? record.role : ''
  return role === 'assistant' || kind === 'assistant_text' || kind === 'assistant_text_delta' || kind === 'agent_message'
}

function isUserAuthoredTextRecord(record: Record<string, unknown>): boolean {
  const kind = typeof record.kind === 'string' ? record.kind : ''
  const role = typeof record.role === 'string' ? record.role : ''
  return role === 'user' || kind === 'user_message'
}

function safeErrorCode(value: unknown): string {
  if (typeof value !== 'string') return 'runtime_error'
  const code = value.trim().slice(0, 128)
  return /^[A-Za-z0-9_.-]+$/.test(code) ? code : 'runtime_error'
}

export function stripASCIIControlCharacters(value: string): string {
  let sanitized = ''
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0
    if (codePoint >= 0x20 && codePoint !== 0x7f) sanitized += character
  }
  return sanitized
}

function safeDiagnosticValue(value: unknown): string | number | boolean | undefined {
  if (typeof value === 'boolean') return value
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value !== 'string') return undefined
  const sanitized = stripASCIIControlCharacters(sanitizePublicAssistantText(value)).trim()
  return sanitized ? sanitized.slice(0, 256) : undefined
}

function looksLikeRuntimeError(record: Record<string, unknown>): boolean {
  return record.kind === 'error' || (
    !Object.prototype.hasOwnProperty.call(record, 'kind') &&
    typeof record.code === 'string' &&
    ('message' in record || 'details' in record || 'severity' in record)
  )
}

function acceptedFinalProjectionState(record: Record<string, unknown>): 'absent' | 'valid' | 'invalid' {
  const hasRecord = Object.prototype.hasOwnProperty.call(record, 'acceptedFinal')
  const hasView = Object.prototype.hasOwnProperty.call(record, 'acceptedFinalView')
  if (!hasRecord && !hasView) return 'absent'
  // A V3 object is a closed shape, not display authority. Generic values that
  // carry either field remain invalid; only the Batch V2 fast paths below can
  // admit an accepted-final projection without rewriting its sealed bytes.
  return 'invalid'
}

function closedPublicToolResultProjection(value: unknown): Record<string, unknown> {
  const parsed = PublicToolResultProjectionV1.safeParse(value)
  if (parsed.success) return parsed.data as Record<string, unknown>
  return {
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    status: 'unknown',
    code: 'legacy_output_withheld',
    messageKey: 'legacy_output_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }
}

function closedPublicToolCallArgumentsProjection(): Record<string, unknown> {
  return PublicToolCallArgumentsProjectionV1.parse({
    schemaVersion: 1,
    projectionKind: 'withheld',
    disclosure: 'metadata_only',
    messageKey: 'tool_arguments_withheld',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false
  }) as Record<string, unknown>
}

const PUBLIC_TOOL_CALL_ITEM_KEYS = [
  'id',
  'turnId',
  'threadId',
  'role',
  'status',
  'createdAt',
  'finishedAt',
  'kind',
  'toolName',
  'callId',
  'toolKind',
  'contextDigest',
  'contextEpoch',
  'executionGrantId'
] as const

function closedPublicToolCallItem(record: Record<string, unknown>): Record<string, unknown> | undefined {
  const candidate: Record<string, unknown> = {}
  for (const key of PUBLIC_TOOL_CALL_ITEM_KEYS) {
    if (Object.prototype.hasOwnProperty.call(record, key)) candidate[key] = record[key]
  }
  candidate.arguments = closedPublicToolCallArgumentsProjection()
  const parsed = ToolCallTurnItem.safeParse(candidate)
  return parsed.success ? parsed.data as Record<string, unknown> : undefined
}

const PUBLIC_TOOL_RESULT_ITEM_KEYS = [
  'id',
  'turnId',
  'threadId',
  'role',
  'status',
  'createdAt',
  'finishedAt',
  'kind',
  'toolName',
  'callId',
  'toolKind',
  'isError',
  'contextDigest',
  'contextEpoch',
  'executionGrantId'
] as const

function closedPublicToolResultItem(record: Record<string, unknown>): Record<string, unknown> | undefined {
  const candidate: Record<string, unknown> = {}
  for (const key of PUBLIC_TOOL_RESULT_ITEM_KEYS) {
    if (Object.prototype.hasOwnProperty.call(record, key)) candidate[key] = record[key]
  }
  candidate.output = closedPublicToolResultProjection(record.output)
  const parsed = ToolResultTurnItem.safeParse(candidate)
  return parsed.success ? parsed.data as Record<string, unknown> : undefined
}

export function sanitizePublicRuntimeErrorEnvelope(value: unknown): Record<string, unknown> {
  if (containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value)) {
    return { code: 'runtime_error', message: 'Runtime request failed.' }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return { code: 'runtime_error', message: 'Runtime request failed.' }
  }
  const record = value as Record<string, unknown>
  const closedFallback: Record<string, unknown> = {
    ...(record.kind === 'error' ? { kind: 'error' } : {}),
    code: 'runtime_error',
    message: 'Runtime request failed.'
  }
  if (GeneralTerminalErrorItemV1Schema.safeParse(record).success &&
      !containsRestrictedEvidence(record) && !containsPrivateReasoningContent(record) &&
      !containsPrivateRuntimeDiagnosticContent(record) && !containsSecretMaterial(record) &&
      !containsOrdinaryPublicPII(record) && !containsInternalCaseEntityReference(record)) {
    return record
  }
  const code = safeErrorCode(record.code)
  const fallback: Record<string, unknown> = {
    ...(record.kind === 'error' ? { kind: 'error' } : {}),
    code,
    message: `Runtime request failed (${code}).`
  }
  const sanitized: Record<string, unknown> = { ...fallback }
  for (const key of [
    'id', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'role', 'status', 'createdAt',
    'finishedAt', 'terminal', 'fatal', 'severity', 'retryable'
  ]) {
    const safe = safeDiagnosticValue(record[key])
    if (safe !== undefined) sanitized[key] = safe
  }
  const details = record.details && typeof record.details === 'object' && !Array.isArray(record.details)
    ? record.details as Record<string, unknown>
    : {}
  if (code === 'tool_not_advertised') {
    if (isToolNotAdvertisedDiagnosticDetailsV1(details)) {
      sanitized.details = { ...details }
    }
  } else {
    const safeDetails: Record<string, unknown> = {}
    for (const [key, entry] of Object.entries(details)) {
      if (!SAFE_ERROR_DETAIL_KEYS.has(key)) continue
      const safe = safeDiagnosticValue(entry)
      if (safe !== undefined) safeDetails[key] = safe
    }
    if (Object.keys(safeDetails).length > 0) sanitized.details = safeDetails
  }
  if (containsRestrictedEvidence(sanitized) || containsPrivateReasoningContent(sanitized) ||
      containsPrivateRuntimeDiagnosticContent(sanitized) || containsSecretMaterial(sanitized) ||
      containsOrdinaryPublicPII(sanitized) || containsInternalCaseEntityReference(sanitized)) return closedFallback
  return sanitized
}

/**
 * Removes content-bearing provider reasoning from runtime-shaped JSON. Numeric
 * usage counters and configuration such as reasoning effort remain intact.
 */
function sanitizePublicRuntimeValueInternal(
  value: unknown,
  inheritedRuntimeText: boolean,
  inheritedUserText = false
): unknown {
  if (Array.isArray(value)) {
    return value
      .map((entry) => sanitizePublicRuntimeValueInternal(entry, inheritedRuntimeText, inheritedUserText))
      .filter((entry) => entry !== undefined)
  }
  if (typeof value === 'string') {
    return inheritedRuntimeText ? sanitizePublicSerializedText(value) : value
  }
  if (!value || typeof value !== 'object') return value

  const record = value as Record<string, unknown>
  if (isPrivateTerminalAuthorityObject(record)) return undefined
  const kind = typeof record.kind === 'string' ? record.kind : ''
  if (PRIVATE_REASONING_KINDS.has(kind)) return undefined
  if (kind === 'tool_call') return closedPublicToolCallItem(record)
  if (kind === 'tool_result') return closedPublicToolResultItem(record)
  if (looksLikeRuntimeError(record)) return sanitizePublicRuntimeErrorEnvelope(record)
  const acceptedFinalProjection = acceptedFinalProjectionState(record)
  if (acceptedFinalProjection === 'invalid' && kind === 'assistant_text') return undefined

  const userText = inheritedUserText || isUserAuthoredTextRecord(record)
  const runtimeText = userText
    ? false
    : inheritedRuntimeText || isAssistantTextRecord(record) || Object.keys(record).length > 0
  const sanitized: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(record)) {
    const reasoningMetadataState = publicReasoningMetadataState(key, entry)
    if (reasoningMetadataState === 'legacy-empty') continue
    if ((reasoningMetadataState === 'unrecognized' && isReasoningLikeKey(key)) ||
        isPrivateTerminalAuthorityKey(key)) continue
    if (acceptedFinalProjection === 'invalid' && (key === 'acceptedFinal' || key === 'acceptedFinalView')) continue
    const child = sanitizePublicRuntimeValueInternal(entry, runtimeText, userText)
    if (child !== undefined) sanitized[key] = child
  }
  return sanitized
}

export function sanitizePublicRuntimeValue(value: unknown): unknown {
  if (value && typeof value === 'object' && !Array.isArray(value) &&
      looksLikeRuntimeError(value as Record<string, unknown>)) {
    return sanitizePublicRuntimeErrorEnvelope(value)
  }
  if (containsPrivateAcceptedFinalAuthority(value) ||
      containsRestrictedEvidence(value) || containsInternalCaseEntityReference(value) ||
      hasInvalidPublicReasoningMetadata(value) ||
      containsPrivateRuntimeDiagnosticContent(value)) return undefined
  const sanitized = sanitizePublicRuntimeValueInternal(value, true)
  if (containsSecretMaterial(sanitized) || containsOrdinaryPublicPII(sanitized) ||
      containsInternalCaseEntityReference(sanitized)) return undefined
  return sanitized
}

function containsPrivateRuntimeDiagnosticContentInternal(value: unknown, depth: number): boolean {
  if (depth > 64) return true
  if (Array.isArray(value)) {
    return value.some((entry) => containsPrivateRuntimeDiagnosticContentInternal(entry, depth + 1))
  }
  if (!value || typeof value !== 'object') return false
  return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
    PRIVATE_RUNTIME_DIAGNOSTIC_KEYS.has(normalizedKey(key)) ||
    containsPrivateRuntimeDiagnosticContentInternal(entry, depth + 1)
  )
}

/**
 * Raw provider/tool/job diagnostics are private even when they do not contain
 * a recognizable reasoning marker. Public boundaries retain only closed
 * status/reason codes and numeric/hash diagnostics.
 */
export function containsPrivateRuntimeDiagnosticContent(value: unknown): boolean {
  return containsPrivateRuntimeDiagnosticContentInternal(value, 0)
}

function containsPrivateAcceptedFinalAuthorityInternal(
  value: unknown,
  depth: number,
  active: WeakSet<object>
): boolean {
  if (depth > 64) return true
  if (!value || typeof value !== 'object') return false
  if (active.has(value)) return true
  active.add(value)
  try {
    if (Array.isArray(value)) {
      return value.some((entry) => containsPrivateAcceptedFinalAuthorityInternal(entry, depth + 1, active))
    }
    return Object.entries(value as Record<string, unknown>).some(([key, entry]) =>
      PRIVATE_ACCEPTED_FINAL_STRONG_KEYS.has(normalizedKey(key)) ||
      containsPrivateAcceptedFinalAuthorityInternal(entry, depth + 1, active)
    )
  } finally {
    active.delete(value)
  }
}

/**
 * Detects private accepted-final authority even when a strong marker is
 * detached from its normal signed record. Generic metadata names such as
 * envelope, registryHead, publicationIntent, and storeDigest remain legal.
 */
export function containsPrivateAcceptedFinalAuthority(value: unknown): boolean {
  return containsPrivateAcceptedFinalAuthorityInternal(value, 0, new WeakSet<object>())
}

function acceptedFinalProjectionTreeIsValid(value: unknown, depth = 0): boolean {
  if (depth > 32) return false
  if (Array.isArray(value)) {
    return value.every((entry) => acceptedFinalProjectionTreeIsValid(entry, depth + 1))
  }
  if (!value || typeof value !== 'object') return true
  const record = value as Record<string, unknown>
  if (record.kind === 'tool_call' && !ToolCallTurnItem.safeParse(record).success) return false
  if (record.kind === 'tool_result' && !ToolResultTurnItem.safeParse(record).success) return false
  if (acceptedFinalProjectionState(record) === 'invalid') return false
  return Object.values(record).every((entry) => acceptedFinalProjectionTreeIsValid(entry, depth + 1))
}

function treeContainsAcceptedFinalProjection(value: unknown, depth = 0): boolean {
  if (depth > 32) return true
  if (Array.isArray(value)) return value.some((entry) => treeContainsAcceptedFinalProjection(entry, depth + 1))
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  if (Object.prototype.hasOwnProperty.call(record, 'acceptedFinal') ||
      Object.prototype.hasOwnProperty.call(record, 'acceptedFinalView')) return true
  return Object.values(record).some((entry) => treeContainsAcceptedFinalProjection(entry, depth + 1))
}

function publicSseEventAcceptedProjectionIsValid(value: Record<string, unknown>): boolean {
  if (value.kind === 'accepted_final_batch') {
    return AcceptedFinalDeliveryBatchV2Schema.safeParse(value).success
  }
  if (value.kind === 'general_terminal_batch') {
    return GeneralTerminalDeliveryBatchV1Schema.safeParse(value).success
  }
  if (!acceptedFinalProjectionTreeIsValid(value)) return false
  if (!treeContainsAcceptedFinalProjection(value)) return true
  return false
}

/** Validates the context-isolated main -> preload -> renderer SSE envelope. */
export function isPublicSseIpcPayload(value: unknown): value is {
  streamId: string
  events: Array<Record<string, unknown>>
} {
  if (containsPrivateAcceptedFinalAuthority(value) ||
      containsRestrictedEvidence(value) || containsPrivateReasoningContent(value) ||
      containsSecretMaterial(value) || containsOrdinaryPublicPII(value) ||
      containsInternalCaseEntityReference(value)) return false
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  if (Object.keys(record).some((key) => key !== 'streamId' && key !== 'events')) return false
  if (typeof record.streamId !== 'string' || record.streamId.trim() !== record.streamId ||
      record.streamId.length === 0 || record.streamId.length > 256) return false
  if (!Array.isArray(record.events) || record.events.some((event) => !event || typeof event !== 'object' || Array.isArray(event))) {
    return false
  }
  return record.events.every((event) => publicSseEventAcceptedProjectionIsValid(event as Record<string, unknown>))
}

/** Filters complete runtime events while retaining per-item delta state. */
export class PublicRuntimeEventFilter {
  push(value: unknown): Record<string, unknown> | null {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    const payload = value as Record<string, unknown>
    if (payload.kind === 'assistant_text_delta') return null

    // Accepted-final batches are already a sealed, byte-bound public
    // projection. Sanitizing a nested terminal error would rewrite signed
    // fields and make a valid four-slot batch unverifiable. Admit only the
    // closed current schema here and preserve it exactly for main-process
    // pin/signature/hash verification.
    if (payload.kind === 'accepted_final_batch') {
      if (!AcceptedFinalDeliveryBatchV2Schema.safeParse(payload).success ||
          containsPrivateAcceptedFinalAuthority(payload) ||
          containsPrivateReasoningContent(payload) || containsRestrictedEvidence(payload) ||
          containsSecretMaterial(payload) || containsOrdinaryPublicPII(payload) ||
          containsInternalCaseEntityReference(payload)) return null
      return payload
    }

    // Ordinary terminal text is admitted only as one host-bound atomic batch.
    // Preserve the exact bytes after closed-schema and privacy validation so
    // nested error normalization cannot invalidate the Go transport digests.
    // These hashes are transport integrity only; all authority flags remain
    // literal false in the schema.
    if (payload.kind === 'general_terminal_batch') {
      const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(payload)
      if (!parsed.success ||
          containsPrivateAcceptedFinalAuthority(payload) ||
          containsPrivateReasoningContent(payload) || containsRestrictedEvidence(payload) ||
          containsSecretMaterial(payload) || containsOrdinaryPublicPII(payload) ||
          containsInternalCaseEntityReference(payload)) return null
      const itemEvent = parsed.data.events.length === 3 ? parsed.data.events[0] : undefined
      if (itemEvent?.kind === 'item_completed' && itemEvent.item.kind === 'assistant_text' &&
          'ordinaryResult' in itemEvent.item && containsProtectedCaseFactCandidate(itemEvent.item.text)) return null
      return payload
    }

    const sanitized = sanitizePublicRuntimeValue(payload)
    const result = sanitized && typeof sanitized === 'object' && !Array.isArray(sanitized)
      ? sanitized as Record<string, unknown>
      : null
    if (result && 'item' in payload && !('item' in result)) return null
    if (result && !publicSseEventAcceptedProjectionIsValid(result)) return null
    return result
  }

}

function containsPrivateReasoningContentInternal(
  value: unknown,
  inheritedRuntimeText: boolean,
  inheritedUserText = false
): boolean {
  if (Array.isArray(value)) {
    return value.some((entry) => containsPrivateReasoningContentInternal(entry, inheritedRuntimeText, inheritedUserText))
  }
  if (typeof value === 'string') {
    if (!inheritedRuntimeText) return false
    return containsPotentialReasoningMarkup(value) || PRIVATE_REASONING_FIELD.test(value)
  }
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  const kind = typeof record.kind === 'string' ? record.kind : ''
  if (PRIVATE_REASONING_KINDS.has(kind)) return true
  const userText = inheritedUserText || isUserAuthoredTextRecord(record)
  const runtimeText = userText
    ? false
    : inheritedRuntimeText || isAssistantTextRecord(record) || Object.keys(record).length > 0
  for (const [key, entry] of Object.entries(record)) {
    const reasoningMetadataState = publicReasoningMetadataState(key, entry)
    if (reasoningMetadataState === 'invalid' ||
        (reasoningMetadataState === 'unrecognized' && isReasoningLikeKey(key))) return true
    if (containsPrivateReasoningContentInternal(entry, runtimeText, userText)) return true
  }
  return false
}

export function containsPrivateReasoningContent(value: unknown): boolean {
  return containsPrivateReasoningContentInternal(value, true)
}
