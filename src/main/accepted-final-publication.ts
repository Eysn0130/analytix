import { createHash, createPublicKey, verify } from 'node:crypto'
import {
  AcceptedFinalDeliveryBatchV1Schema,
  AcceptedFinalDeliveryBatchV2Schema,
  AcceptedFinalDeliverySealV1Schema,
  AcceptedFinalItemCompletedEventV1Schema,
  AcceptedFinalItemCompletedEventV3Schema,
  type AcceptedFinalItemCompletedEventV1
} from '../../packages/runtime/src/contracts/events.js'
import {
  ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON,
  AcceptedFinalPrivateRecordSchema,
  acceptedFinalDeliveryRequiresErrorItem,
  type FactFinalWitnessAdmission
} from '../../packages/runtime/src/contracts/items.js'
import { containsPrivateReasoningContent } from '../shared/public-runtime-content'
import { containsInternalCaseEntityReference } from '../shared/ordinary-log-pii-projection'

const ACCEPTED_FINAL_SIGNATURE_DOMAIN = Buffer.from('analytix.final-answer-authority/v1\0')
const ACCEPTED_FINAL_PUBLIC_VIEW_DOMAIN = Buffer.from('analytix.accepted-final-public-view/v2\0')
const ACCEPTED_FINAL_EVENT_DOMAIN = 'analytix.accepted-final-event/v1\0'
const ACCEPTED_FINAL_DELIVERY_SEAL_ID_DOMAIN = Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0')
const ACCEPTED_FINAL_DELIVERY_SEAL_SIGNATURE_DOMAIN = Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0')
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')

export type FinalPublicationAuthorityPinV1 = Readonly<{
  keyId: string
  publicKey: string
}>

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('value is not JSON serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function goCanonicalJSON(value: unknown): string {
  if (value === null || typeof value !== 'object') return goJSONStringify(value)
  if (Array.isArray(value)) return `[${value.map(goCanonicalJSON).join(',')}]`
  const record = value as Record<string, unknown>
  return `{${Object.keys(record).sort().map((key) => `${goJSONStringify(key)}:${goCanonicalJSON(record[key])}`).join(',')}}`
}

export function sha256Hex(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function authorityPublicKeyForPin(pin: FinalPublicationAuthorityPinV1 | null): Buffer | null {
  if (!pin || !/^[a-f0-9]{64}$/.test(pin.keyId) || typeof pin.publicKey !== 'string') return null
  const publicKey = Buffer.from(pin.publicKey, 'base64url')
  if (publicKey.length !== 32 || publicKey.toString('base64url') !== pin.publicKey ||
      sha256Hex(publicKey) !== pin.keyId) return null
  return publicKey
}

function verifyFactFinalWitnessAdmissionIntegrity(
  admission: FactFinalWitnessAdmission
): boolean {
  const binding = admission.witnessBinding
  if (sha256Hex(goJSONStringify({ ...binding, bindingDigest: '' })) !== binding.bindingDigest ||
      sha256Hex(goJSONStringify({ ...admission, admissionDigest: '' })) !== admission.admissionDigest) {
    return false
  }
  switch (admission.schemaVersion) {
    case 1:
      return admission.purpose === 'analytix.fact-final-witness-admission/v1'
    case 2:
      return admission.purpose === 'analytix.fact-final-witness-admission/v2' &&
        admission.selectedDatasetSnapshotIndexGeneration <= admission.datasetSnapshotCount
    default:
      return false
  }
}

export function verifyAcceptedFinalRecordIntegrity(
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): boolean {
  try {
    const pinnedPublicKey = authorityPublicKeyForPin(pin)
    if (!pinnedPublicKey || !pin) return false
    if (containsInternalCaseEntityReference(value)) return false
    const parsed = AcceptedFinalPrivateRecordSchema.safeParse(value)
    if (!parsed.success) return false
    const record = parsed.data
    if (sha256Hex(Buffer.concat([
      ACCEPTED_FINAL_PUBLIC_VIEW_DOMAIN,
      Buffer.from(goJSONStringify(record.publicView))
    ])) !== record.publicViewDigest) {
      return false
    }
    if ('factFinalWitnessAdmission' in record) {
      if (!verifyFactFinalWitnessAdmissionIntegrity(record.factFinalWitnessAdmission)) {
        return false
      }
    }
    const publicKey = Buffer.from(record.authorityPublicKey, 'base64url')
    const signature = Buffer.from(record.authoritySignature, 'base64url')
    if (publicKey.length !== 32 || publicKey.toString('base64url') !== record.authorityPublicKey ||
        signature.length !== 64 || signature.toString('base64url') !== record.authoritySignature ||
        sha256Hex(publicKey) !== record.authorityKeyId || record.authorityKeyId !== pin.keyId ||
        record.authorityPublicKey !== pin.publicKey || !publicKey.equals(pinnedPublicKey)) {
      return false
    }
    const signingRecord = { ...record, authoritySignature: '', recordDigest: '' }
    const signingDigest = createHash('sha256').update(goJSONStringify(signingRecord)).digest()
    const signingBytes = Buffer.concat([ACCEPTED_FINAL_SIGNATURE_DOMAIN, signingDigest])
    const key = createPublicKey({
      key: Buffer.concat([ED25519_SPKI_PREFIX, publicKey]),
      format: 'der',
      type: 'spki'
    })
    if (!verify(null, signingBytes, key, signature)) return false
    return sha256Hex(goJSONStringify({ ...record, recordDigest: '' })) === record.recordDigest
  } catch {
    return false
  }
}

export function acceptedFinalPublicationEventId(recordDigest: string, slot: string): string {
  return sha256Hex(`${ACCEPTED_FINAL_EVENT_DOMAIN}${recordDigest}\0${slot}`)
}

export function acceptedFinalPublicationPayloadDigest(event: Record<string, unknown>): string {
  const canonical = { ...event }
  delete canonical.seq
  delete canonical.publicationPayloadDigest
  return sha256Hex(goCanonicalJSON(canonical))
}

/** Private/CAS audit verifier; generic HTTP/SSE must use verifiedAcceptedFinalDeliveryBatch. */
export function verifiedAcceptedFinalItemCompletedEvent(
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): AcceptedFinalItemCompletedEventV1 | null {
  const accepted = AcceptedFinalItemCompletedEventV1Schema.safeParse(value)
  if (!accepted.success) return null
  const record = accepted.data.item.acceptedFinal
  const text = accepted.data.item.text
  if (
    !record ||
    containsPrivateReasoningContent(text) ||
    sha256Hex(text) !== record.renderedTextSha256 ||
    !verifyAcceptedFinalRecordIntegrity(record, pin) ||
    accepted.data.publicationEventId !== acceptedFinalPublicationEventId(record.recordDigest, 'assistant-final') ||
    accepted.data.publicationPayloadDigest !== acceptedFinalPublicationPayloadDigest(
      accepted.data as Record<string, unknown>
    )
  ) {
    return null
  }
  return accepted.data
}

export type VerifiedAcceptedFinalDeliveryBatch = {
  batch: Record<string, unknown>
  events: Record<string, unknown>[]
  firstSeq: number
  lastSeq: number
  publicationCommitId: string
}

export function verifiedAcceptedFinalDeliveryBatch(
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): VerifiedAcceptedFinalDeliveryBatch | null {
  if (containsInternalCaseEntityReference(value)) return null
  const pinnedPublicKey = authorityPublicKeyForPin(pin)
  if (!pinnedPublicKey || !pin) return null
  // V1 remains a strict private/audit parser only. A shape-valid historical
  // batch is deliberately classified before the current V2 generic parser so
  // it can never be accepted through structural overlap.
  if (AcceptedFinalDeliveryBatchV1Schema.safeParse(value).success) return null
  const parsedBatch = AcceptedFinalDeliveryBatchV2Schema.safeParse(value)
  if (!parsedBatch.success) return null
  const batch = parsedBatch.data as unknown as Record<string, unknown>
  const allowedBatchKeys = new Set([
    'schemaVersion', 'purpose', 'kind', 'batchId', 'threadId', 'turnId', 'seq', 'firstSeq',
    'lastSeq', 'timestamp', 'publicationCommitId', 'eventManifestDigest', 'publicationAuthority', 'events', 'trace'
  ])
  if (Object.keys(batch).some((key) => !allowedBatchKeys.has(key)) ||
      batch.schemaVersion !== 2 || batch.purpose !== 'analytix.accepted-final-delivery-batch/v2' ||
      batch.kind !== 'accepted_final_batch' || typeof batch.threadId !== 'string' ||
      typeof batch.turnId !== 'string' || typeof batch.timestamp !== 'string' ||
      typeof batch.batchId !== 'string' || !/^[a-f0-9]{64}$/.test(batch.batchId) ||
      typeof batch.publicationCommitId !== 'string' || !/^[a-f0-9]{64}$/.test(batch.publicationCommitId) ||
      typeof batch.eventManifestDigest !== 'string' || !/^[a-f0-9]{64}$/.test(batch.eventManifestDigest) ||
      !Number.isSafeInteger(batch.seq) || !Number.isSafeInteger(batch.firstSeq) ||
      !Number.isSafeInteger(batch.lastSeq) || batch.seq !== batch.lastSeq ||
      (batch.firstSeq as number) <= 0 || (batch.lastSeq as number) < (batch.firstSeq as number) ||
      !Array.isArray(batch.events) || (batch.events.length !== 3 && batch.events.length !== 4) ||
      (batch.lastSeq as number) - (batch.firstSeq as number) + 1 !== batch.events.length) {
    return null
  }
  const events = batch.events as unknown[]
  if (events.some((event) => !event || typeof event !== 'object' || Array.isArray(event))) return null
  const records = events as Record<string, unknown>[]
  const expectedSlots = records.length === 3
    ? ['assistant-final', 'usage', 'terminal']
    : ['assistant-final', 'terminal-error-item', 'usage', 'terminal']
  for (let index = 0; index < records.length; index += 1) {
    const event = records[index]
    const slot = expectedSlots[index]
    if (event.threadId !== batch.threadId || event.turnId !== batch.turnId ||
        event.seq !== (batch.firstSeq as number) + index ||
        event.timestamp !== batch.timestamp ||
        event.acceptedFinalDigest !== batch.publicationCommitId ||
        event.publicationCommitId !== batch.publicationCommitId || event.publicationSlot !== slot ||
        event.publicationEventId !== acceptedFinalPublicationEventId(batch.publicationCommitId as string, slot) ||
        event.publicationPayloadDigest !== acceptedFinalPublicationPayloadDigest(event)) {
      return null
    }
  }
  if (records[0]?.kind !== 'item_completed' ||
      records[records.length - 2]?.kind !== 'usage' ||
      (records.length === 4 && records[1]?.kind !== 'item_completed')) return null
  const acceptedGeneric = AcceptedFinalItemCompletedEventV3Schema.safeParse(records[0])
  if (!acceptedGeneric.success) return null
  const acceptedGenericEvent = acceptedGeneric.data
  const acceptedView = acceptedGenericEvent.item.acceptedFinalView
  if (!acceptedView || acceptedView.schemaVersion !== 3 ||
      acceptedView.acceptedFinalDigest !== batch.publicationCommitId ||
      containsPrivateReasoningContent(acceptedGenericEvent.item.text) ||
      acceptedGenericEvent.publicationEventId !== acceptedFinalPublicationEventId(
        acceptedView.acceptedFinalDigest,
        'assistant-final'
      ) ||
      acceptedGenericEvent.publicationPayloadDigest !== acceptedFinalPublicationPayloadDigest(
        acceptedGenericEvent as Record<string, unknown>
      )) {
    return null
  }
  const terminal = records[records.length - 1]
  const terminalReason = acceptedView.terminalReason
  const expectedStatus = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[terminalReason]
  const requiresErrorItem = acceptedFinalDeliveryRequiresErrorItem(terminalReason)
  const usage = records[records.length - 2]
  if (terminal.kind !== `turn_${expectedStatus}` || terminal.status !== expectedStatus ||
      terminal.terminalReason !== terminalReason ||
      terminal.acceptedFinalDigest !== batch.publicationCommitId || terminal.timestamp !== batch.timestamp ||
      usage.usageFinalStatus !== expectedStatus || (records.length === 4) !== requiresErrorItem) {
    return null
  }
  if (expectedStatus === 'aborted') {
    if (typeof terminal.discard !== 'boolean' || typeof terminal.cancelled !== 'boolean' ||
        !Number.isSafeInteger(terminal.cancelledPendingGates) ||
        Object.is(terminal.cancelledPendingGates, -0) ||
        (terminal.cancelledPendingGates as number) < 0 ||
        ((terminal.cancelledPendingGates as number) > 0 && terminal.cancelled === false)) return null
  } else if (terminal.discard !== undefined || terminal.cancelled !== undefined ||
      terminal.cancelledPendingGates !== undefined) {
    return null
  }
  if (expectedStatus !== 'failed' && (terminal.error !== undefined || terminal.message !== undefined)) {
    return null
  }
  if (records.length === 3) {
    if (terminal.itemId !== undefined) return null
  } else {
    const errorEvent = records[1]
    const errorItem = errorEvent.item
    if (!errorItem || typeof errorItem !== 'object' || Array.isArray(errorItem)) return null
    const item = errorItem as Record<string, unknown>
    if (errorEvent.itemId !== item.id || terminal.itemId !== item.id ||
        item.threadId !== batch.threadId || item.turnId !== batch.turnId ||
        item.status !== expectedStatus || item.acceptedFinalDigest !== batch.publicationCommitId ||
        item.createdAt !== batch.timestamp || item.finishedAt !== batch.timestamp ||
        typeof terminal.code !== 'string' || terminal.code.length === 0 || terminal.code !== item.code ||
        typeof item.message !== 'string' || item.message.length === 0) return null
    if (expectedStatus === 'failed') {
      if (terminal.error !== item.message || terminal.message !== item.message || item.severity !== 'error') {
        return null
      }
    } else if (item.severity !== 'warning') {
      return null
    }
  }
  const manifest = records.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))
  if (sha256Hex(goCanonicalJSON(manifest)) !== batch.eventManifestDigest) return null
  const expectedBatchId = sha256Hex(goJSONStringify({
    purpose: batch.purpose,
    threadId: batch.threadId,
    turnId: batch.turnId,
    firstSeq: batch.firstSeq,
    lastSeq: batch.lastSeq,
    publicationCommitId: batch.publicationCommitId,
    eventManifestDigest: batch.eventManifestDigest
  }))
  if (expectedBatchId !== batch.batchId) return null

  const seal = verifiedAcceptedFinalDeliverySeal(
    batch.publicationAuthority,
    batch,
    records,
    pin
  )
  if (!seal || containsPrivateReasoningContent(batch)) return null

  return {
    batch: { ...batch, publicationAuthority: seal, events: records },
    events: records,
    firstSeq: batch.firstSeq as number,
    lastSeq: batch.lastSeq as number,
    publicationCommitId: batch.publicationCommitId as string
  }
}

function verifiedAcceptedFinalDeliverySeal(
  value: unknown,
  batch: Record<string, unknown>,
  events: Record<string, unknown>[],
  pin: FinalPublicationAuthorityPinV1
): Record<string, unknown> | null {
  const seal = verifiedAcceptedFinalDeliverySealAuthority(value, pin)
  if (!seal || seal.threadId !== batch.threadId || seal.turnId !== batch.turnId ||
      seal.publicationCommitId !== batch.publicationCommitId ||
      seal.eventManifestDigest !== batch.eventManifestDigest || seal.batchId !== batch.batchId ||
      seal.firstSeq !== batch.firstSeq || seal.lastSeq !== batch.lastSeq || seal.timestamp !== batch.timestamp ||
      typeof seal.sequencedEventsDigest !== 'string' || !/^[a-f0-9]{64}$/.test(seal.sequencedEventsDigest) ||
      sha256Hex(goCanonicalJSON(events)) !== seal.sequencedEventsDigest) {
    return null
  }
  return seal
}

export function verifiedAcceptedFinalDeliverySealAuthority(
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): Record<string, unknown> | null {
  const pinnedPublicKey = authorityPublicKeyForPin(pin)
  if (!pinnedPublicKey || !pin) return null
  const parsed = AcceptedFinalDeliverySealV1Schema.safeParse(value)
  if (!parsed.success) return null
  const seal = parsed.data as unknown as Record<string, unknown>
  if (seal.authorityKeyId !== pin.keyId || seal.authorityPublicKey !== pin.publicKey ||
      typeof seal.authoritySignature !== 'string') return null
  const signature = Buffer.from(seal.authoritySignature, 'base64url')
  if (signature.length !== 64 || signature.toString('base64url') !== seal.authoritySignature) return null
  const unsigned = acceptedFinalDeliverySealOrdered({ ...seal, authoritySignature: '' })
  const sealIdBody = acceptedFinalDeliverySealOrdered({ ...unsigned, sealId: '' })
  if (sha256Hex(Buffer.concat([
    ACCEPTED_FINAL_DELIVERY_SEAL_ID_DOMAIN,
    Buffer.from(goJSONStringify(sealIdBody))
  ])) !== seal.sealId) return null
  const signingDigest = createHash('sha256').update(goJSONStringify(unsigned)).digest()
  const signingBytes = Buffer.concat([ACCEPTED_FINAL_DELIVERY_SEAL_SIGNATURE_DOMAIN, signingDigest])
  const key = createPublicKey({
    key: Buffer.concat([ED25519_SPKI_PREFIX, pinnedPublicKey]),
    format: 'der',
    type: 'spki'
  })
  return verify(null, signingBytes, key, signature) ? acceptedFinalDeliverySealOrdered(seal) : null
}

function acceptedFinalDeliverySealOrdered(value: Record<string, unknown>): Record<string, unknown> {
  return {
    schemaVersion: value.schemaVersion,
    purpose: value.purpose,
    sealId: value.sealId,
    threadId: value.threadId,
    turnId: value.turnId,
    publicationCommitId: value.publicationCommitId,
    acceptedFinalDispositionDigest: value.acceptedFinalDispositionDigest,
    terminalDispositionId: value.terminalDispositionId,
    eventManifestDigest: value.eventManifestDigest,
    sequencedEventsDigest: value.sequencedEventsDigest,
    batchId: value.batchId,
    firstSeq: value.firstSeq,
    lastSeq: value.lastSeq,
    timestamp: value.timestamp,
    authorityAlgorithm: value.authorityAlgorithm,
    authorityKeyId: value.authorityKeyId,
    authorityPublicKey: value.authorityPublicKey,
    authoritySignature: value.authoritySignature
  }
}
