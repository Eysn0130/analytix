import { createHash } from 'node:crypto'
import {
  GeneralTerminalDeliveryBatchV1Schema,
  type GeneralTerminalDeliveryBatchV1
} from '../../packages/runtime/src/contracts/events.js'
import type { OrdinaryResultSlotV1 } from '../../packages/runtime/src/contracts/items.js'
import {
  containsInternalCaseEntityReference,
  containsOrdinaryPublicPII,
  containsProtectedCaseFactCandidate
} from '../shared/ordinary-log-pii-projection.js'
import { containsPrivateReasoningContent } from '../shared/public-runtime-content.js'
import { containsRestrictedEvidence } from '../shared/restricted-evidence-projection.js'
import { containsSecretMaterial } from '../shared/secret-redaction.js'

const EVENT_ID_DOMAIN = Buffer.from('analytix/general-terminal-event/v1\0')
const MANIFEST_DIGEST_DOMAIN = Buffer.from('analytix/general-terminal-delivery-manifest/v1\0')
const PROJECTED_EVENTS_DIGEST_DOMAIN = Buffer.from('analytix/general-terminal-projected-events/v1\0')
const BATCH_DIGEST_DOMAIN = Buffer.from('analytix/general-terminal-delivery-batch/v1\0')
const ORDINARY_RESULT_DIGEST_DOMAIN = Buffer.from('analytix/ordinary-result/v1\0')
const SHA256_HEX = /^[a-f0-9]{64}$/
const MAX_PUBLIC_TEXT_BYTES_V1 = 1 << 20
const MAX_PUBLIC_AGGREGATE_TEXT_BYTES_V1 = 8 << 20
const MAX_PUBLIC_VALUE_DEPTH_V1 = 64
const MAX_PUBLIC_VALUE_NODES_V1 = 10_000

const UNPAIRED_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?:^|[^\uD800-\uDBFF])[\uDC00-\uDFFF]/u

export const GENERAL_TERMINAL_PUBLIC_INTEGRITY_SCOPE_V1 = Object.freeze({
  eventIds: 'recomputed' as const,
  eventManifestDigest: 'recomputed' as const,
  projectedEventsDigest: 'recomputed' as const,
  batchDigest: 'recomputed' as const,
  rawPayloadDigests: 'opaque_bound_only' as const,
  hostCASAuthority: 'opaque_bound_only' as const
})

export type VerifiedGeneralTerminalDeliveryBatchV1 = Readonly<{
  batch: Record<string, unknown>
  events: Record<string, unknown>[]
  firstSeq: number
  lastSeq: number
  generalTerminalCommitId: string
  verification: typeof GENERAL_TERMINAL_PUBLIC_INTEGRITY_SCOPE_V1
}>

export type GeneralTerminalDeliveryBatchVerificationReason =
  | 'general_terminal_batch_public_tree_invalid'
  | 'general_terminal_batch_private_reasoning'
  | 'general_terminal_batch_restricted_evidence'
  | 'general_terminal_batch_secret_material'
  | 'general_terminal_batch_ordinary_pii'
  | 'general_terminal_batch_json_invalid'
  | 'general_terminal_batch_schema_invalid'
  | 'general_terminal_batch_ordinary_result_invalid'
  | 'general_terminal_batch_event_binding_invalid'
  | 'general_terminal_batch_integrity_invalid'
  | 'general_terminal_batch_verification_failed'

export type GeneralTerminalDeliveryBatchVerificationV1 = Readonly<{
  verified: VerifiedGeneralTerminalDeliveryBatchV1 | null
  reason: GeneralTerminalDeliveryBatchVerificationReason | null
}>

function sha256Domain(domain: Buffer, body: string): string {
  return createHash('sha256').update(domain).update(body, 'utf8').digest('hex')
}

function goJSONString(value: string): string {
  if (UNPAIRED_SURROGATE.test(value)) throw new Error('string is not valid Unicode')
  return JSON.stringify(value)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function compareGoMapKeys(left: string, right: string): number {
  return Buffer.compare(Buffer.from(left, 'utf8'), Buffer.from(right, 'utf8'))
}

/** Mirrors encoding/json for the JSON value kinds admitted by the closed schema. */
function goCanonicalJSON(value: unknown): string {
  if (value === null) return 'null'
  switch (typeof value) {
    case 'string':
      return goJSONString(value)
    case 'boolean':
      return value ? 'true' : 'false'
    case 'number': {
      if (!Number.isFinite(value)) throw new Error('number is not JSON serializable')
      if (Object.is(value, -0)) return '-0'
      return JSON.stringify(value)
    }
    case 'object': {
      if (Array.isArray(value)) {
        if (Object.keys(value).length !== value.length) throw new Error('array is sparse or decorated')
        return `[${value.map((entry) => goCanonicalJSON(entry)).join(',')}]`
      }
      const record = value as Record<string, unknown>
      if (Object.getPrototypeOf(record) !== Object.prototype && Object.getPrototypeOf(record) !== null) {
        throw new Error('value is not a JSON object')
      }
      const keys = Object.keys(record).sort(compareGoMapKeys)
      if (Reflect.ownKeys(record).length !== keys.length) throw new Error('object has non-JSON keys')
      return `{${keys.map((key) => `${goJSONString(key)}:${goCanonicalJSON(record[key])}`).join(',')}}`
    }
    default:
      throw new Error('value is not JSON serializable')
  }
}

function manifestJSON(manifest: GeneralTerminalDeliveryBatchV1['eventManifest']): string {
  return `[${manifest.map((entry) => `{${[
    `${goJSONString('slot')}:${goJSONString(entry.slot)}`,
    `${goJSONString('eventId')}:${goJSONString(entry.eventId)}`,
    `${goJSONString('payloadDigest')}:${goJSONString(entry.payloadDigest)}`
  ].join(',')}}`).join(',')}]`
}

function batchDigestJSON(batch: GeneralTerminalDeliveryBatchV1): string {
  const scalar = (key: string, value: unknown): string => `${goJSONString(key)}:${goCanonicalJSON(value)}`
  return `{${[
    scalar('schemaVersion', batch.schemaVersion),
    scalar('purpose', batch.purpose),
    scalar('kind', batch.kind),
    scalar('batchDigest', ''),
    scalar('threadId', batch.threadId),
    scalar('turnId', batch.turnId),
    scalar('seq', batch.seq),
    scalar('firstSeq', batch.firstSeq),
    scalar('lastSeq', batch.lastSeq),
    scalar('timestamp', batch.timestamp),
    scalar('generalTerminalCommitId', batch.generalTerminalCommitId),
    scalar('generalTerminalAuthorityKind', batch.generalTerminalAuthorityKind),
    scalar('generalTerminalAuthorityDigest', batch.generalTerminalAuthorityDigest),
    scalar('eventManifestDigest', batch.eventManifestDigest),
    scalar('projectedEventsDigest', batch.projectedEventsDigest),
    scalar('transportAuthority', batch.transportAuthority),
    scalar('evidenceAuthority', batch.evidenceAuthority),
    scalar('citationAuthority', batch.citationAuthority),
    scalar('factAnswerAllowed', batch.factAnswerAllowed),
    `${goJSONString('events')}:${goCanonicalJSON(batch.events)}`,
    `${goJSONString('eventManifest')}:${manifestJSON(batch.eventManifest)}`
  ].join(',')}}`
}

function ordinaryResultDigestJSON(slot: OrdinaryResultSlotV1): string {
  const scalar = (key: string, value: unknown): string => `${goJSONString(key)}:${goCanonicalJSON(value)}`
  return `{${[
    scalar('schemaVersion', slot.schemaVersion),
    scalar('purpose', slot.purpose),
    scalar('projectionVersion', slot.projectionVersion),
    scalar('logicalEffect', slot.logicalEffect),
    scalar('ordinaryWork', slot.ordinaryWork),
    scalar('candidateOrigin', slot.candidateOrigin),
    scalar('evidenceAuthority', slot.evidenceAuthority),
    scalar('citationAuthority', slot.citationAuthority),
    scalar('factAnswerAllowed', slot.factAnswerAllowed),
    scalar('text', slot.text),
    scalar('textSha256', slot.textSha256),
    scalar('resultDigest', '')
  ].join(',')}}`
}

export function validOrdinaryResultSlotV1(slot: OrdinaryResultSlotV1): boolean {
  if (!isExactGoText(slot.text) || containsInternalCaseEntityReference(slot.text) ||
      containsProtectedCaseFactCandidate(slot.text)) return false
  if (createHash('sha256').update(slot.text, 'utf8').digest('hex') !== slot.textSha256) return false
  return sha256Domain(ORDINARY_RESULT_DIGEST_DOMAIN, ordinaryResultDigestJSON(slot)) === slot.resultDigest
}

function generalTerminalEventID(commitId: string, slot: string): string {
  const body = `${commitId.trim()}\0${slot.trim()}`
  return sha256Domain(EVENT_ID_DOMAIN, body)
}

function isGoSpace(codePoint: number): boolean {
  return (codePoint >= 0x09 && codePoint <= 0x0d) || codePoint === 0x20 ||
    codePoint === 0x85 || codePoint === 0xa0 || codePoint === 0x1680 ||
    (codePoint >= 0x2000 && codePoint <= 0x200a) || codePoint === 0x2028 ||
    codePoint === 0x2029 || codePoint === 0x202f || codePoint === 0x205f ||
    codePoint === 0x3000 || codePoint === 0xfeff
}

function isExactGoText(value: string): boolean {
  if (value.length === 0) return false
  const codePoints = Array.from(value, (entry) => entry.codePointAt(0) as number)
  return !isGoSpace(codePoints[0]) && !isGoSpace(codePoints[codePoints.length - 1])
}

function isCanonicalGoTimestamp(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?Z$/.exec(value)
  if (!match) return false
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const hour = Number(match[4])
  const minute = Number(match[5])
  const second = Number(match[6])
  const fraction = match[7]
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0)
  const days = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31]
  return month >= 1 && month <= 12 && day >= 1 && day <= days[month - 1] &&
    hour <= 23 && minute <= 59 && second <= 59 && (!fraction || !fraction.endsWith('0'))
}

function hasCanonicalJSONShape(value: unknown): boolean {
  try {
    goCanonicalJSON(value)
    return true
  } catch {
    return false
  }
}

function hasBoundedPublicJSONTreeV1(value: unknown): boolean {
  const state = {
    nodes: 0,
    textBytes: 0,
    active: new WeakSet<object>()
  }
  const consumeText = (text: string): boolean => {
    const bytes = Buffer.byteLength(text, 'utf8')
    if (bytes > MAX_PUBLIC_TEXT_BYTES_V1 ||
        state.textBytes > MAX_PUBLIC_AGGREGATE_TEXT_BYTES_V1 - bytes) return false
    state.textBytes += bytes
    return true
  }
  const visit = (current: unknown, depth: number): boolean => {
    if (depth > MAX_PUBLIC_VALUE_DEPTH_V1 || state.nodes >= MAX_PUBLIC_VALUE_NODES_V1) return false
    state.nodes += 1
    if (current === null || typeof current === 'boolean') return true
    if (typeof current === 'string') return consumeText(current)
    if (typeof current === 'number') return Number.isFinite(current)
    if (!current || typeof current !== 'object' || state.active.has(current)) return false
    state.active.add(current)
    try {
      if (Array.isArray(current)) {
        if (current.length > MAX_PUBLIC_VALUE_NODES_V1 || Object.keys(current).length !== current.length) return false
        return current.every((entry) => visit(entry, depth + 1))
      }
      const prototype = Object.getPrototypeOf(current)
      if (prototype !== Object.prototype && prototype !== null) return false
      const record = current as Record<string, unknown>
      const keys = Object.keys(record)
      if (keys.length > MAX_PUBLIC_VALUE_NODES_V1 || Reflect.ownKeys(record).length !== keys.length) return false
      for (const key of keys) {
        if (!consumeText(key) || !visit(record[key], depth + 1)) return false
      }
      return true
    } finally {
      state.active.delete(current)
    }
  }
  return visit(value, 0)
}

/**
 * Verifies every digest that Electron can recompute from the public batch.
 *
 * The manifest's raw payload digests bind opaque durable records, not the
 * sanitized public projections. Likewise, the host CAS authority digest has
 * no public signature or pinned key. Both are therefore bound into the
 * recomputed manifest/batch digests but deliberately reported as
 * `opaque_bound_only`; this function must never be used as evidence, citation,
 * fact-answer, accepted-final, or authenticated host-CAS authority.
 */
export function generalTerminalDeliveryBatchVerificationV1(
  value: unknown
): GeneralTerminalDeliveryBatchVerificationV1 {
  try {
    if (!hasBoundedPublicJSONTreeV1(value)) {
      return { verified: null, reason: 'general_terminal_batch_public_tree_invalid' }
    }
    if (containsPrivateReasoningContent(value)) {
      return { verified: null, reason: 'general_terminal_batch_private_reasoning' }
    }
    if (containsRestrictedEvidence(value)) {
      return { verified: null, reason: 'general_terminal_batch_restricted_evidence' }
    }
    if (containsSecretMaterial(value)) {
      return { verified: null, reason: 'general_terminal_batch_secret_material' }
    }
    if (containsOrdinaryPublicPII(value)) {
      return { verified: null, reason: 'general_terminal_batch_ordinary_pii' }
    }
    if (!hasCanonicalJSONShape(value)) {
      return { verified: null, reason: 'general_terminal_batch_json_invalid' }
    }
    const parsed = GeneralTerminalDeliveryBatchV1Schema.safeParse(value)
    if (!parsed.success) {
      return { verified: null, reason: 'general_terminal_batch_schema_invalid' }
    }
    const batch = parsed.data
    if (!isExactGoText(batch.threadId) || !isExactGoText(batch.turnId) ||
        !isCanonicalGoTimestamp(batch.timestamp) || !SHA256_HEX.test(batch.batchDigest) ||
        !SHA256_HEX.test(batch.generalTerminalCommitId) ||
        !SHA256_HEX.test(batch.generalTerminalAuthorityDigest) ||
        !SHA256_HEX.test(batch.eventManifestDigest) ||
        !SHA256_HEX.test(batch.projectedEventsDigest)) {
      return { verified: null, reason: 'general_terminal_batch_schema_invalid' }
    }

    if (batch.events.length === 3) {
      const itemEvent = batch.events[0]
      if (itemEvent.kind === 'item_completed' && itemEvent.item.kind === 'assistant_text' &&
          'ordinaryResult' in itemEvent.item && !validOrdinaryResultSlotV1(itemEvent.item.ordinaryResult)) {
        return { verified: null, reason: 'general_terminal_batch_ordinary_result_invalid' }
      }
    }

    const expectedSlots = batch.events.length === 2
      ? ['usage', 'terminal'] as const
      : ['terminal-item', 'usage', 'terminal'] as const
    const seenEventIds = new Set<string>()
    for (let index = 0; index < batch.eventManifest.length; index += 1) {
      const manifest = batch.eventManifest[index]
      const event = batch.events[index]
      const slot = expectedSlots[index]
      if (!manifest || !event || manifest.slot !== slot ||
          !SHA256_HEX.test(manifest.eventId) || !SHA256_HEX.test(manifest.payloadDigest) ||
          seenEventIds.has(manifest.eventId) ||
          manifest.eventId !== generalTerminalEventID(batch.generalTerminalCommitId, slot) ||
          event.threadId !== batch.threadId || event.turnId !== batch.turnId ||
          event.timestamp !== batch.timestamp || event.seq !== batch.firstSeq + index) {
        return { verified: null, reason: 'general_terminal_batch_event_binding_invalid' }
      }
      seenEventIds.add(manifest.eventId)
    }

    if (sha256Domain(MANIFEST_DIGEST_DOMAIN, manifestJSON(batch.eventManifest)) !== batch.eventManifestDigest ||
        sha256Domain(PROJECTED_EVENTS_DIGEST_DOMAIN, goCanonicalJSON(batch.events)) !==
          batch.projectedEventsDigest ||
        sha256Domain(BATCH_DIGEST_DOMAIN, batchDigestJSON(batch)) !== batch.batchDigest) {
      return { verified: null, reason: 'general_terminal_batch_integrity_invalid' }
    }

    const events = batch.events as unknown as Record<string, unknown>[]
    return {
      verified: {
        batch: batch as unknown as Record<string, unknown>,
        events,
        firstSeq: batch.firstSeq,
        lastSeq: batch.lastSeq,
        generalTerminalCommitId: batch.generalTerminalCommitId,
        verification: GENERAL_TERMINAL_PUBLIC_INTEGRITY_SCOPE_V1
      },
      reason: null
    }
  } catch {
    return { verified: null, reason: 'general_terminal_batch_verification_failed' }
  }
}

export function verifiedGeneralTerminalDeliveryBatchV1(
  value: unknown
): VerifiedGeneralTerminalDeliveryBatchV1 | null {
  return generalTerminalDeliveryBatchVerificationV1(value).verified
}
