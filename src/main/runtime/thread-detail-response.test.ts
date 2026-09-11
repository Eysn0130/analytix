import { createHash, generateKeyPairSync, sign, type KeyObject } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import {
  ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1,
  AcceptedFinalDeliveryBatchV2Schema
} from '../../../packages/runtime/src/contracts/events'
import { ThreadDetailResponseV1Schema } from '../../../packages/runtime/src/contracts/thread-detail'
import { sanitizePublicRuntimeValue } from '../../shared/public-runtime-content'
import {
  acceptedFinalPublicationEventId,
  acceptedFinalPublicationPayloadDigest
} from '../accepted-final-publication'
import { sanitizeRuntimeResponse } from './analytix-adapter'

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
  return `{${Object.keys(record).sort().map((key) => (
    `${goJSONStringify(key)}:${goCanonicalJSON(record[key])}`
  )).join(',')}}`
}

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function orderedDeliverySeal(value: Record<string, unknown>): Record<string, unknown> {
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

function signedProviderFailureThreadDetail(
  rendererVersion: 'analytix.host-final-renderer/v1' | 'analytix.host-final-renderer/v2' =
    'analytix.host-final-renderer/v1'
): {
  thread: Record<string, unknown>
  pin: { keyId: string; publicKey: string }
  privateKey: KeyObject
  privateRecord: Record<string, any>
} {
  const authority = generateKeyPairSync('ed25519')
  const publicKeyDER = authority.publicKey.export({ format: 'der', type: 'spki' }) as Buffer
  const publicKeyBytes = publicKeyDER.subarray(publicKeyDER.length - 32)
  const pin = {
    keyId: sha256(publicKeyBytes),
    publicKey: publicKeyBytes.toString('base64url')
  }
  const threadId = 'thread-provider-failure-detail'
  const turnId = 'turn-provider-failure-detail'
  const timestamp = '2026-08-02T01:02:03Z'
  const text = '案件分析未完成；未经核验的案件事实未发布。'
  const publicView = {
    schemaVersion: 2,
    publicationState: 'accepted',
    envelopeDigest: 'b'.repeat(64),
    contextDigest: 'c'.repeat(64),
    contextEpoch: 3,
    datasetSnapshotId: 'snapshot-provider-failure-detail',
    variant: 'GeneralGuidanceAnswer',
    terminalReason: 'provider_failure',
    blockerCode: '',
    coverageStatus: 'guidance_only',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: 'd'.repeat(64),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: timestamp,
    acceptedAt: timestamp
  }
  const publicViewDigest = sha256(Buffer.concat([
    Buffer.from('analytix.accepted-final-public-view/v2\0'),
    Buffer.from(goJSONStringify(publicView))
  ]))
  const unsignedRecord = {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: pin.keyId,
    authorityPublicKey: pin.publicKey,
    threadId,
    turnId,
    envelopeDigest: publicView.envelopeDigest,
    contextDigest: publicView.contextDigest,
    contextEpoch: publicView.contextEpoch,
    datasetSnapshotId: publicView.datasetSnapshotId,
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    renderedTextSha256: sha256(text),
    registrySequence: 0,
    registryStateDigest: 'e'.repeat(64),
    rendererVersion,
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest,
    privateRecordDigest: 'f'.repeat(64),
    acceptedAt: timestamp,
    authoritySignature: '',
    recordDigest: ''
  }
  const signingDigest = createHash('sha256')
    .update(goJSONStringify(unsignedRecord))
    .digest()
  const authoritySignature = sign(
    null,
    Buffer.concat([Buffer.from('analytix.final-answer-authority/v1\0'), signingDigest]),
    authority.privateKey
  ).toString('base64url')
  const signedRecord = { ...unsignedRecord, authoritySignature }
  const record = {
    ...signedRecord,
    recordDigest: sha256(goJSONStringify(signedRecord))
  }
  const acceptedFinalView = {
    schemaVersion: 3,
    acceptedFinalDigest: record.recordDigest,
    publicationState: 'accepted',
    variant: publicView.variant,
    terminalReason: publicView.terminalReason,
    blockerCode: publicView.blockerCode,
    coverageStatus: publicView.coverageStatus,
    checkedScopeDigest: publicView.checkedScopeDigest,
    missingScopeCount: publicView.missingScopeCount,
    claimCount: publicView.claimCount,
    claimTypes: publicView.claimTypes,
    receiptMetadata: publicView.receiptMetadata,
    noHitWording: publicView.noHitWording,
    acceptedAt: publicView.acceptedAt
  }
  const assistantItem = {
    id: 'item-provider-failure-assistant',
    turnId,
    threadId,
    role: 'assistant',
    status: 'completed',
    createdAt: timestamp,
    finishedAt: timestamp,
    kind: 'assistant_text',
    text,
    acceptedFinalView
  }
  const errorItem = {
    id: 'item-provider-failure-error',
    turnId,
    threadId,
    role: 'system',
    status: 'failed',
    createdAt: timestamp,
    finishedAt: timestamp,
    kind: 'error',
    code: 'case_terminal_provider_failure',
    message: text,
    severity: 'error',
    acceptedFinalDigest: record.recordDigest
  }
  const firstSeq = 7
  const common = {
    timestamp,
    threadId,
    turnId,
    acceptedFinalDigest: record.recordDigest,
    publicationCommitId: record.recordDigest
  }
  const assistantEvent: Record<string, unknown> = {
    ...common,
    kind: 'item_completed',
    seq: firstSeq,
    itemId: assistantItem.id,
    item: assistantItem,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, 'assistant-final'),
    publicationSlot: 'assistant-final'
  }
  assistantEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(assistantEvent)
  const errorEvent: Record<string, unknown> = {
    ...common,
    kind: 'item_completed',
    seq: firstSeq + 1,
    itemId: errorItem.id,
    item: errorItem,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, 'terminal-error-item'),
    publicationSlot: 'terminal-error-item'
  }
  errorEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(errorEvent)
  const usageEvent: Record<string, unknown> = {
    ...common,
    kind: 'usage',
    seq: firstSeq + 2,
    model: 'model-provider-failure',
    providerId: 'provider-provider-failure',
    usage: {
      promptTokens: 10,
      completionTokens: 3,
      reasoningTokens: 0,
      totalTokens: 13,
      cachedTokens: 0,
      cacheHitTokens: 0,
      cacheMissTokens: 10,
      cacheHitRate: 0,
      turns: 1
    },
    cacheDiagnostics: {},
    usageFinalStatus: 'failed',
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, 'usage'),
    publicationSlot: 'usage'
  }
  usageEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(usageEvent)
  const terminalEvent: Record<string, unknown> = {
    ...common,
    kind: 'turn_failed',
    seq: firstSeq + 3,
    status: 'failed',
    terminalReason: 'provider_failure',
    error: text,
    message: text,
    code: errorItem.code,
    itemId: errorItem.id,
    publicationEventId: acceptedFinalPublicationEventId(record.recordDigest, 'terminal'),
    publicationSlot: 'terminal'
  }
  terminalEvent.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(terminalEvent)
  const events = [assistantEvent, errorEvent, usageEvent, terminalEvent]
  const lastSeq = firstSeq + events.length - 1
  const eventManifestDigest = sha256(goCanonicalJSON(events.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))))
  const batchId = sha256(goJSONStringify({
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    threadId,
    turnId,
    firstSeq,
    lastSeq,
    publicationCommitId: record.recordDigest,
    eventManifestDigest
  }))
  const unsignedSeal = orderedDeliverySeal({
    schemaVersion: 'accepted-final-delivery-seal.v1',
    purpose: 'analytix.accepted-final-delivery-seal/v1',
    sealId: '',
    threadId,
    turnId,
    publicationCommitId: record.recordDigest,
    acceptedFinalDispositionDigest: '1'.repeat(64),
    terminalDispositionId: '2'.repeat(64),
    eventManifestDigest,
    sequencedEventsDigest: sha256(goCanonicalJSON(events)),
    batchId,
    firstSeq,
    lastSeq,
    timestamp,
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: pin.keyId,
    authorityPublicKey: pin.publicKey,
    authoritySignature: ''
  })
  const sealId = sha256(Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0'),
    Buffer.from(goJSONStringify(unsignedSeal))
  ]))
  const unsignedSealWithId = orderedDeliverySeal({ ...unsignedSeal, sealId })
  const sealSignatureDigest = createHash('sha256')
    .update(goJSONStringify(unsignedSealWithId))
    .digest()
  const publicationAuthority = orderedDeliverySeal({
    ...unsignedSealWithId,
    authoritySignature: sign(
      null,
      Buffer.concat([
        Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0'),
        sealSignatureDigest
      ]),
      authority.privateKey
    ).toString('base64url')
  })
  const delivery = {
    schemaVersion: 2,
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    kind: 'accepted_final_batch',
    batchId,
    threadId,
    turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    publicationCommitId: record.recordDigest,
    eventManifestDigest,
    publicationAuthority,
    events
  }
  return {
    pin,
    privateKey: authority.privateKey,
    privateRecord: record,
    thread: {
      id: threadId,
      title: 'Accepted-final provider failure',
      model: 'model-provider-failure',
      providerId: 'provider-provider-failure',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: timestamp,
      updatedAt: timestamp,
      historyAuthority: 'case_boundary_only_v1',
      turns: [{
        id: turnId,
        threadId,
        status: 'failed',
        createdAt: timestamp,
        finishedAt: timestamp,
        items: [assistantItem, errorItem],
        acceptedFinalView
      }],
      latestSeq: lastSeq,
      pendingApprovalIds: [],
      pendingUserInputIds: [],
      messageCount: 1,
      turnCount: 1,
      latestTurnId: turnId,
      acceptedFinalDelivery: delivery,
      acceptedFinalDeliveries: [delivery]
    }
  }
}

function appendSecondSignedProviderFailureFinal(
  fixture: ReturnType<typeof signedProviderFailureThreadDetail>
): Record<string, any> {
  const thread = structuredClone(fixture.thread) as Record<string, any>
  const firstDelivery = thread.acceptedFinalDeliveries[0] as Record<string, any>
  const secondDelivery = structuredClone(firstDelivery) as Record<string, any>
  const turnId = 'turn-provider-failure-detail-second'
  const timestamp = '2026-08-02T01:03:03Z'
  const publicationCommitId = '9'.repeat(64)
  const firstSeq = firstDelivery.lastSeq + 1
  const assistantItemId = 'item-provider-failure-detail-second'
  const errorItemId = 'item-provider-failure-detail-second-error'
  const events = secondDelivery.events as Array<Record<string, any>>
  for (let index = 0; index < events.length; index += 1) {
    const event = events[index]
    event.turnId = turnId
    event.seq = firstSeq + index
    event.timestamp = timestamp
    event.acceptedFinalDigest = publicationCommitId
    event.publicationCommitId = publicationCommitId
    event.publicationEventId = acceptedFinalPublicationEventId(publicationCommitId, event.publicationSlot)
    delete event.publicationPayloadDigest
  }
  const assistantItem = events[0].item as Record<string, any>
  assistantItem.id = assistantItemId
  assistantItem.turnId = turnId
  assistantItem.finishedAt = timestamp
  assistantItem.acceptedFinalView.acceptedFinalDigest = publicationCommitId
  assistantItem.acceptedFinalView.acceptedAt = timestamp
  events[0].itemId = assistantItemId
  const errorItem = events[1].item as Record<string, any>
  errorItem.id = errorItemId
  errorItem.turnId = turnId
  errorItem.acceptedFinalDigest = publicationCommitId
  errorItem.createdAt = timestamp
  errorItem.finishedAt = timestamp
  events[1].itemId = errorItemId
  events[3].itemId = errorItemId
  for (const event of events) {
    event.publicationPayloadDigest = acceptedFinalPublicationPayloadDigest(event)
  }
  const lastSeq = firstSeq + events.length - 1
  const eventManifestDigest = sha256(goCanonicalJSON(events.map((event) => ({
    slot: event.publicationSlot,
    eventId: event.publicationEventId,
    payloadDigest: event.publicationPayloadDigest
  }))))
  const batchId = sha256(goJSONStringify({
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    threadId: thread.id,
    turnId,
    firstSeq,
    lastSeq,
    publicationCommitId,
    eventManifestDigest
  }))
  const unsignedSeal = orderedDeliverySeal({
    ...secondDelivery.publicationAuthority,
    sealId: '',
    turnId,
    publicationCommitId,
    acceptedFinalDispositionDigest: '6'.repeat(64),
    terminalDispositionId: '7'.repeat(64),
    eventManifestDigest,
    sequencedEventsDigest: sha256(goCanonicalJSON(events)),
    batchId,
    firstSeq,
    lastSeq,
    timestamp,
    authoritySignature: ''
  })
  const sealId = sha256(Buffer.concat([
    Buffer.from('analytix/accepted-final-delivery-seal-id/v1\0'),
    Buffer.from(goJSONStringify(unsignedSeal))
  ]))
  const unsignedSealWithId = orderedDeliverySeal({ ...unsignedSeal, sealId })
  const sealSignatureDigest = createHash('sha256')
    .update(goJSONStringify(unsignedSealWithId))
    .digest()
  secondDelivery.batchId = batchId
  secondDelivery.turnId = turnId
  secondDelivery.seq = lastSeq
  secondDelivery.firstSeq = firstSeq
  secondDelivery.lastSeq = lastSeq
  secondDelivery.timestamp = timestamp
  secondDelivery.publicationCommitId = publicationCommitId
  secondDelivery.eventManifestDigest = eventManifestDigest
  secondDelivery.publicationAuthority = orderedDeliverySeal({
    ...unsignedSealWithId,
    authoritySignature: sign(
      null,
      Buffer.concat([
        Buffer.from('analytix/accepted-final-delivery-seal-signature/v1\0'),
        sealSignatureDigest
      ]),
      fixture.privateKey
    ).toString('base64url')
  })
  const secondTurn = {
    id: turnId,
    threadId: thread.id,
    status: 'failed',
    createdAt: timestamp,
    finishedAt: timestamp,
    items: [assistantItem, errorItem],
    acceptedFinalView: assistantItem.acceptedFinalView
  }
  thread.turns.push(secondTurn)
  thread.acceptedFinalDeliveries.push(secondDelivery)
  delete thread.acceptedFinalDelivery
  thread.latestSeq = lastSeq
  thread.updatedAt = timestamp
  thread.messageCount = 2
  thread.turnCount = 2
  thread.latestTurnId = turnId
  return thread
}

function ordinaryThreadDetail(): Record<string, unknown> {
  const timestamp = '2026-07-31T00:00:00Z'
  return {
    id: 'thread-plan-public-detail',
    title: 'Plan public detail',
    workspace: '/Volumes/AnalytixCache/development-v2/acceptance/repository',
    model: 'model-1',
    providerId: 'analytix-hub',
    mode: 'plan',
    status: 'idle',
    executionPolicyVersion: 2,
    approvalPolicy: 'auto',
    sandboxMode: 'danger-full-access',
    relation: 'primary',
    createdAt: timestamp,
    updatedAt: timestamp,
    turns: [{
      id: 'turn-plan-public-detail',
      threadId: 'thread-plan-public-detail',
      status: 'failed',
      prompt: 'Inspect the repository and save a plan.',
      model: 'model-1',
      reasoningEffort: 'auto',
      steering: [],
      createdAt: timestamp,
      startedAt: timestamp,
      finishedAt: timestamp,
      items: [{
        id: 'item-plan-user',
        turnId: 'turn-plan-public-detail',
        threadId: 'thread-plan-public-detail',
        role: 'user',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'user_message',
        text: 'Inspect the repository and save a plan.'
      }, {
        id: 'item-plan-call',
        turnId: 'turn-plan-public-detail',
        threadId: 'thread-plan-public-detail',
        role: 'assistant',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'tool_call',
        toolName: 'create_plan',
        callId: `call_host_${'a1'.repeat(32)}`,
        toolKind: 'file_change',
        arguments: {
          schemaVersion: 1,
          projectionKind: 'withheld',
          disclosure: 'metadata_only',
          messageKey: 'tool_arguments_withheld',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false
        }
      }, {
        id: 'item-plan-result',
        turnId: 'turn-plan-public-detail',
        threadId: 'thread-plan-public-detail',
        role: 'tool',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'tool_result',
        toolName: 'create_plan',
        callId: `call_host_${'a1'.repeat(32)}`,
        toolKind: 'file_change',
        output: {
          schemaVersion: 1,
          projectionKind: 'plan_status',
          disclosure: 'metadata_only',
          status: 'completed',
          code: 'plan_updated',
          messageKey: 'plan_updated',
          privatePayloadWithheld: true,
          factAnswerAllowed: false,
          evidenceAuthority: false,
          plan: {
            planId: '/Volumes/AnalytixCache/development-v2/acceptance/repository:.analytixsdd/plan/plan.md',
            relativePath: '.analytixsdd/plan/plan.md',
            operation: 'draft',
            contentHash: `ab${'1'.repeat(60)}cd`,
            byteSize: 128,
            savedAt: timestamp
          }
        },
        isError: false
      }, {
        id: 'item-plan-error',
        turnId: 'turn-plan-public-detail',
        threadId: 'thread-plan-public-detail',
        role: 'system',
        status: 'failed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'error',
        message: 'Runtime request failed (provider_error).',
        code: 'provider_error',
        severity: 'error'
      }],
      attachmentIds: [],
      activeSkillIds: [],
      injectedMemoryIds: [],
      mode: 'plan'
    }],
    latestSeq: 51,
    usage: {
      promptTokens: 100,
      completionTokens: 20,
      reasoningTokens: 0,
      totalTokens: 120,
      cachedTokens: 0,
      cacheHitTokens: 0,
      cacheMissTokens: 100,
      cacheHitRate: null,
      cacheableTokenHitRate: null,
      totalInputTokenHitRate: 0,
      cacheMissReasons: [],
      cacheSuggestions: [],
      turns: 1,
      costUsd: 0,
      costCny: 0,
      priceConfigured: false,
      cacheSavingsUsd: 0,
      cacheSavingsCny: 0,
      tokenEconomySavingsTokens: 0,
      tokenEconomySavingsUsd: 0,
      tokenEconomySavingsCny: 0,
      last_turn_cache_hit_rate: null,
      last_turn_cacheable_hit_rate: null,
      last_turn_total_input_hit_rate: 0,
      last_cache_miss_reasons: [],
      last_cache_suggestions: [],
      lastTurnCacheHitRate: null,
      cache_hit_tokens: 0,
      cache_miss_tokens: 100,
      input_tokens: 100,
      output_tokens: 20,
      reasoning_tokens: 0,
      total_tokens: 120,
      cost_usd: 0,
      cost_cny: 0,
      price_configured: false,
      token_economy_savings_tokens: 0
    },
    pendingApprovalIds: [],
    pendingUserInputIds: [],
    preview: 'Inspect the repository and save a plan.',
    messageCount: 1,
    turnCount: 1,
    latestTurnId: 'turn-plan-public-detail'
  }
}

function caseCompactionThreadDetail(): Record<string, unknown> {
  const timestamp = '2026-08-02T00:00:00Z'
  return {
    id: 'thread-case-compaction-detail',
    title: 'Case compaction detail',
    model: 'model-1',
    mode: 'agent',
    status: 'idle',
    approvalPolicy: 'on-request',
    sandboxMode: 'workspace-write',
    relation: 'primary',
    createdAt: timestamp,
    updatedAt: timestamp,
    historyAuthority: 'case_boundary_only_v1',
    turns: [{
      id: 'turn-case-compaction-detail',
      threadId: 'thread-case-compaction-detail',
      status: 'completed',
      createdAt: timestamp,
      startedAt: timestamp,
      finishedAt: timestamp,
      items: [{
        id: 'item-case-compaction-detail',
        turnId: 'turn-case-compaction-detail',
        threadId: 'thread-case-compaction-detail',
        role: 'system',
        status: 'completed',
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'compaction',
        summary: 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.',
        auto: false,
        pinnedConstraints: ['user: preserve recent turns'],
        sourceDigest: 'c'.repeat(64),
        digestMarker: `sha256:${'c'.repeat(12)}`,
        sourceItemIds: [],
        schemaVersion: 3,
        reasoningExcluded: true,
        reasoningExclusionProof: `sha256:${'d'.repeat(64)}`,
        assistantProseExcluded: true,
        toolPayloadsExcluded: true,
        caseFactsExcluded: true,
        caseHistoryProjectionVersion: 2
      }]
    }],
    latestSeq: 59,
    pendingApprovalIds: [],
    pendingUserInputIds: [],
    messageCount: 0,
    turnCount: 1,
    latestTurnId: 'turn-case-compaction-detail'
  }
}

describe('thread detail runtime response', () => {
  it('accepts V3 public delivery derived from both private V5 renderer versions', () => {
    for (const rendererVersion of [
      'analytix.host-final-renderer/v1',
      'analytix.host-final-renderer/v2'
    ] as const) {
      const fixture = signedProviderFailureThreadDetail(rendererVersion)
      const parsed = ThreadDetailResponseV1Schema.safeParse(fixture.thread)
      expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)

      const accepted = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(fixture.thread)
      }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
      expect(accepted.ok, accepted.body).toBe(true)
      expect(JSON.parse(accepted.body)).toEqual(fixture.thread)
      const publicView = (fixture.thread.turns as Array<Record<string, any>>)[0].acceptedFinalView as Record<string, any>
      expect(Object.keys(publicView).sort()).toEqual([
        'schemaVersion', 'acceptedFinalDigest', 'publicationState', 'variant', 'terminalReason',
        'blockerCode', 'coverageStatus', 'checkedScopeDigest', 'missingScopeCount', 'claimCount',
        'claimTypes', 'receiptMetadata', 'noHitWording', 'acceptedAt'
      ].sort())
      expect(Object.keys(publicView.receiptMetadata).sort()).toEqual([
        'projection', 'count', 'setDigest', 'citations'
      ].sort())
      const viewBody = JSON.stringify(publicView)
      for (const forbiddenProperty of [
        'publicViewDigest', 'envelopeDigest', 'contextDigest', 'contextEpoch', 'datasetSnapshotId',
        'envelopeIssuedAt', 'threadId', 'turnId', 'renderedTextSha256', 'acceptedFinal',
        'factFinalWitnessAdmission', 'publicationSnapshotProof', 'publicationSnapshotProofDigest',
        'securityContext', 'publicationIntent', 'storeDigest', 'registryHead'
      ]) {
        expect(viewBody).not.toContain(`"${forbiddenProperty}"`)
      }
      for (const forbiddenValue of [
        fixture.privateRecord.publicViewDigest,
        fixture.privateRecord.envelopeDigest,
        fixture.privateRecord.contextDigest,
        fixture.privateRecord.datasetSnapshotId,
        fixture.privateRecord.threadId,
        fixture.privateRecord.turnId,
        fixture.privateRecord.renderedTextSha256,
        fixture.privateRecord.privateRecordDigest
      ]) {
        expect(viewBody).not.toContain(forbiddenValue)
      }
    }
  })

  it('requires an independent delivery attestation for every visible accepted-final turn', () => {
    const fixture = signedProviderFailureThreadDetail()
    const thread = appendSecondSignedProviderFailureFinal(fixture)
    const parsed = ThreadDetailResponseV1Schema.safeParse(thread)
    expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)

    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, `/v1/threads/${thread.id}`, fixture.pin, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(thread)

    const mutations: Array<{ name: string; mutate: (candidate: Record<string, any>) => void }> = []
    for (const [index, label] of [[0, 'old'], [1, 'new']] as const) {
      mutations.push(
        {
          name: `${label} view`,
          mutate: (candidate) => { candidate.turns[index].acceptedFinalView.blockerCode = 'detached_view' }
        },
        {
          name: `${label} item`,
          mutate: (candidate) => { candidate.turns[index].items[0].text = 'DETACHED_PUBLIC_ITEM' }
        },
        {
          name: `${label} batch`,
          mutate: (candidate) => {
            candidate.acceptedFinalDeliveries[index] = structuredClone(
              candidate.acceptedFinalDeliveries[index === 0 ? 1 : 0]
            )
          }
        },
        {
          name: `${label} seq`,
          mutate: (candidate) => { candidate.acceptedFinalDeliveries[index].firstSeq += 1 }
        },
        {
          name: `${label} commit`,
          mutate: (candidate) => {
            candidate.acceptedFinalDeliveries[index].publicationCommitId =
              candidate.acceptedFinalDeliveries[index === 0 ? 1 : 0].publicationCommitId
          }
        },
        {
          name: `${label} seal`,
          mutate: (candidate) => {
            candidate.acceptedFinalDeliveries[index].publicationAuthority.authoritySignature = 'A'.repeat(86)
          }
        }
      )
    }
    mutations.push(
      {
        name: 'missing old attestation',
        mutate: (candidate) => { candidate.acceptedFinalDeliveries.splice(0, 1) }
      },
      {
        name: 'duplicate extra attestation',
        mutate: (candidate) => {
          candidate.acceptedFinalDeliveries.push(structuredClone(candidate.acceptedFinalDeliveries[1]))
        }
      },
      {
        name: 'multi-history singular alias',
        mutate: (candidate) => { candidate.acceptedFinalDelivery = candidate.acceptedFinalDeliveries[0] }
      },
      {
        name: 'old new swap',
        mutate: (candidate) => { candidate.acceptedFinalDeliveries.reverse() }
      },
      {
        name: 'detached turn identity',
        mutate: (candidate) => { candidate.acceptedFinalDeliveries[0].turnId = candidate.turns[1].id }
      },
      {
        name: 'historical V1 current swap',
        mutate: (candidate) => {
          const historical = structuredClone(candidate.acceptedFinalDeliveries[0])
          historical.schemaVersion = 1
          historical.purpose = 'analytix.accepted-final-delivery-batch/v1'
          historical.events[0].item.acceptedFinal = fixture.privateRecord
          historical.events[0].item.acceptedFinalView = fixture.privateRecord.publicView
          candidate.acceptedFinalDeliveries[0] = historical
        }
      },
      {
        name: 'snapshot frontier',
        mutate: (candidate) => { candidate.latestSeq = candidate.acceptedFinalDeliveries[1].lastSeq - 1 }
      },
      {
        name: 'advanced snapshot frontier',
        mutate: (candidate) => { candidate.latestSeq = candidate.acceptedFinalDeliveries[1].lastSeq + 1 }
      }
    )

    for (const test of mutations) {
      const candidate = JSON.parse(JSON.stringify(thread)) as Record<string, any>
      test.mutate(candidate)
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(candidate)
      }, `/v1/threads/${thread.id}`, fixture.pin, 'GET')
      expect(rejected.ok, `${test.name}: ${rejected.body}`).toBe(false)
      expect(rejected.status, test.name).toBe(502)
      expect(rejected.body, test.name).not.toContain('DETACHED_PUBLIC_ITEM')
    }
  })

  it('rejects more than 256 historical delivery groups before generic Main hydration', () => {
    const fixture = signedProviderFailureThreadDetail()
    const thread = structuredClone(fixture.thread) as Record<string, any>
    thread.acceptedFinalDeliveries = Array.from({ length: 256 }, () =>
      structuredClone(thread.acceptedFinalDeliveries[0])
    )
    delete thread.acceptedFinalDelivery

    const bounded = AcceptedFinalDeliveryBatchV2Schema.array()
      .max(ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1)
      .safeParse(thread.acceptedFinalDeliveries)
    expect(bounded.success, bounded.success ? '' : JSON.stringify(bounded.error.issues)).toBe(true)

    thread.acceptedFinalDeliveries.push(structuredClone(thread.acceptedFinalDeliveries[0]))
    thread.acceptedFinalDeliveries[256].turnId = 'PRIVATE_DELIVERY_OVERFLOW_CANARY'

    const parsed = ThreadDetailResponseV1Schema.safeParse(thread)
    expect(parsed.success).toBe(false)
    if (!parsed.success) {
      expect(parsed.error.issues.some((issue) =>
        issue.code === 'too_big' && issue.path[0] === 'acceptedFinalDeliveries'
      )).toBe(true)
    }
    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, `/v1/threads/${thread.id}`, fixture.pin, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain('PRIVATE_DELIVERY_OVERFLOW_CANARY')
  })

  it('does not let a verified thread delivery ambiently authorize a detached V3 item', () => {
    const fixture = signedProviderFailureThreadDetail()
    const thread = structuredClone(fixture.thread) as Record<string, any>
    const detachedValue = 'DETACHED_AMBIENT_ACCEPTED_FINAL'
    const detached = structuredClone(thread.turns[0].items[0])
    detached.text = detachedValue
    thread.detachedAcceptedFinal = detached

    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, `/v1/threads/${thread.id}`, fixture.pin, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain(detachedValue)
  })

  it('admits the durable Go goal id on the exact thread-list public seam', () => {
    const goalEvidence = {
      id: 'goal-evidence-1',
      turnId: 'turn-plan-public-detail',
      step: 'Run the focused verification',
      evidence: ['focused verification passed'],
      createdAt: '2026-08-03T00:00:00Z'
    }
    const goal = {
      id: 'goal-1',
      threadId: 'thread-1',
      objective: 'Complete the repository task',
      status: 'active',
      tokenBudget: null,
      tokensUsed: 0,
      timeUsedSeconds: 0,
      evidenceLedger: [goalEvidence],
      createdAt: '2026-08-03T00:00:00Z',
      updatedAt: '2026-08-03T00:00:01Z'
    }
    const summary = {
      id: 'thread-1',
      title: 'Repository task',
      workspace: '/isolated/repository',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access',
      relation: 'primary',
      goal,
      createdAt: '2026-08-03T00:00:00Z',
      updatedAt: '2026-08-03T00:00:01Z'
    }

    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ threads: [summary] })
    }, '/v1/threads?include=side', null, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual({ threads: [summary] })

    const detail = {
      ...ordinaryThreadDetail(),
      goal: {
        ...goal,
        id: 'goal-detail-1',
        threadId: 'thread-plan-public-detail'
      }
    }
    const detailAccepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(detail)
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')
    expect(detailAccepted.ok, detailAccepted.body).toBe(true)
    expect(JSON.parse(detailAccepted.body)).toEqual(detail)

    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({
        threads: [{ ...summary, goal: { ...goal, privateField: 'must-not-pass' } }]
      })
    }, '/v1/threads?include=side', null, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain('must-not-pass')

    const goalWithPrivateEvidenceDetails = {
      ...goal,
      evidenceLedger: [{
        ...goalEvidence,
        evidenceDetails: [{ hostVerified: true, command: 'must-not-pass' }]
      }]
    }
    for (const [path, body] of [
      ['/v1/threads?include=side', { threads: [{ ...summary, goal: goalWithPrivateEvidenceDetails }] }],
      ['/v1/threads/thread-plan-public-detail', { ...detail, goal: goalWithPrivateEvidenceDetails }]
    ] as const) {
      const privateEvidenceRejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(body)
      }, path, null, 'GET')
      expect(privateEvidenceRejected.ok).toBe(false)
      expect(privateEvidenceRejected.status).toBe(502)
      expect(privateEvidenceRejected.body).not.toContain('evidenceDetails')
      expect(privateEvidenceRejected.body).not.toContain('must-not-pass')
    }
  })

  it('admits one pin-verified four-slot provider failure without widening generic errors', () => {
    const fixture = signedProviderFailureThreadDetail()
    const parsed = ThreadDetailResponseV1Schema.safeParse(fixture.thread)
    expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)
    if (!parsed.success) return
    expect(parsed.data).toEqual(fixture.thread)
    expect(sanitizePublicRuntimeValue(fixture.thread)).not.toEqual(fixture.thread)

    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(fixture.thread)
    }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(accepted.status).toBe(200)
    expect(JSON.parse(accepted.body)).toEqual(fixture.thread)

    const mutations: Array<(thread: Record<string, any>) => void> = [
      (thread) => { thread.turns[0].items[1].acceptedFinalDigest = '0'.repeat(64) },
      (thread) => { thread.turns[0].items[1].message = 'detached terminal message' },
      (thread) => { thread.acceptedFinalDelivery.events[1].item.message = 'detached terminal message' },
      (thread) => { thread.acceptedFinalDelivery.events[1].item.threadId = 'foreign-thread' },
      (thread) => { thread.acceptedFinalDelivery.publicationAuthority.authoritySignature = 'A'.repeat(86) }
    ]
    for (const mutate of mutations) {
      const thread = structuredClone(fixture.thread) as Record<string, any>
      mutate(thread)
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(thread)
      }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
      expect(rejected.ok).toBe(false)
      expect(rejected.status).toBe(502)
      expect(rejected.body).not.toContain('detached terminal message')
    }

    const wrongPin = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(fixture.thread)
    }, `/v1/threads/${fixture.thread.id}`, {
      keyId: '0'.repeat(64),
      publicKey: fixture.pin.publicKey
    }, 'GET')
    expect(wrongPin.ok).toBe(false)
    expect(wrongPin.status).toBe(502)
  })

  it('rejects same-ID duplicate accepted assistant and terminal items with zero hostile bytes', () => {
    const fixture = signedProviderFailureThreadDetail()
    for (const [name, itemIndex, canary] of [
      ['assistant', 0, 'DUPLICATE_ASSISTANT_CANARY'],
      ['terminal', 1, 'DUPLICATE_TERMINAL_CANARY']
    ] as const) {
      const thread = structuredClone(fixture.thread) as Record<string, any>
      const duplicate = structuredClone(thread.turns[0].items[itemIndex])
      if (name === 'assistant') duplicate.text = canary
      else duplicate.message = canary
      expect(duplicate.id).toBe(thread.turns[0].items[itemIndex].id)
      thread.turns[0].items.push(duplicate)

      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(thread)
      }, `/v1/threads/${thread.id}`, fixture.pin, 'GET')
      expect(rejected.ok, name).toBe(false)
      expect(rejected.status, name).toBe(502)
      expect(rejected.body, name).not.toContain(canary)
    }
  })

  it('accepts reasoning effort only on the exact case-bound turn', () => {
    const fixture = signedProviderFailureThreadDetail()
    const acceptedThread = structuredClone(fixture.thread) as Record<string, any>
    acceptedThread.turns[0].reasoningEffort = 'high'

    const parsed = ThreadDetailResponseV1Schema.safeParse(acceptedThread)
    expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(acceptedThread)
    }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)
    expect(JSON.parse(accepted.body)).toEqual(acceptedThread)

    const invalidTurnValue = structuredClone(acceptedThread) as Record<string, any>
    invalidTurnValue.turns[0].reasoningEffort = 'invalid-reasoning-effort-sentinel'
    expect(ThreadDetailResponseV1Schema.safeParse(invalidTurnValue).success).toBe(false)
    const invalidTurnResponse = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(invalidTurnValue)
    }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
    expect(invalidTurnResponse.ok).toBe(false)
    expect(invalidTurnResponse.status).toBe(502)
    expect(invalidTurnResponse.body).not.toContain('invalid-reasoning-effort-sentinel')

    const nonStringTurnValue = structuredClone(acceptedThread) as Record<string, any>
    nonStringTurnValue.turns[0].reasoningEffort = { leaked: 'non-string-reasoning-sentinel' }
    expect(ThreadDetailResponseV1Schema.safeParse(nonStringTurnValue).success).toBe(false)
    const nonStringTurnResponse = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(nonStringTurnValue)
    }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
    expect(nonStringTurnResponse.ok).toBe(false)
    expect(nonStringTurnResponse.status).toBe(502)
    expect(nonStringTurnResponse.body).not.toContain('non-string-reasoning-sentinel')

    const threadRootValue = structuredClone(acceptedThread) as Record<string, any>
    threadRootValue.reasoningEffort = 'high'
    expect(ThreadDetailResponseV1Schema.safeParse(threadRootValue).success).toBe(false)
    const threadRootResponse = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(threadRootValue)
    }, `/v1/threads/${fixture.thread.id}`, fixture.pin, 'GET')
    expect(threadRootResponse.ok).toBe(false)
    expect(threadRootResponse.status).toBe(502)
    expect(threadRootResponse.body).not.toContain('reasoningEffort')
  })

  it('admits the exact computed Go thread detail after a completed create_plan call', () => {
    const thread = ordinaryThreadDetail()
    const parsed = ThreadDetailResponseV1Schema.safeParse(thread)
    expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)
    expect(sanitizePublicRuntimeValue(thread)).toEqual(thread)
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')

    expect(response.ok, response.body).toBe(true)
    expect(JSON.parse(response.body)).toEqual(thread)
  })

  it.each([
    ['import_mapping_preview', 'NEUTRAL_HISTORY_CANARY_IMPORT'],
    ['cleaning_diff_preview', 'NEUTRAL_HISTORY_CANARY_CLEANING'],
    ['direct_source_preview', 'NEUTRAL_HISTORY_CANARY_DIRECT'],
    ['accepted_slot_display', 'NEUTRAL_HISTORY_CANARY_ACCEPTED']
  ] as const)('rejects a PII-neutral %s response laundered into ordinary transcript history', (kind, canary) => {
    const thread = ordinaryThreadDetail()
    const turn = (thread.turns as Array<Record<string, unknown>>)[0]
    const items = turn.items as Array<Record<string, unknown>>
    const text = JSON.stringify({ schemaVersion: 1, kind, nested: { displayValue: canary } })
    turn.prompt = text
    items[0].text = text

    expect(ThreadDetailResponseV1Schema.safeParse(thread).success).toBe(true)
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')
    expect(response.ok).toBe(false)
    expect(response.status).toBe(502)
    expect(response.body).not.toContain(canary)
  })

  it('keeps detail-only fields closed to create responses and rejects unknown detail fields', () => {
    const thread = ordinaryThreadDetail()
    const createResponse = sanitizeRuntimeResponse({
      ok: true,
      status: 201,
      body: JSON.stringify(thread)
    }, '/v1/threads', null, 'POST')
    expect(createResponse.ok).toBe(false)
    expect(createResponse.status).toBe(502)

    const detailResponse = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify({ ...thread, privateDiagnostic: 'must-not-cross' })
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')
    expect(detailResponse.ok).toBe(false)
    expect(detailResponse.status).toBe(502)
    expect(detailResponse.body).not.toContain('must-not-cross')
  })

  it('rejects private diagnostics, long AuthorityRef, and phone PII with zero hostile bytes', () => {
    const authorityRef = `authref_${'A'.repeat(2048)}`
    const phone = '13800138000'
    const cases: Array<{ name: string; canary: string; thread: Record<string, any> }> = [
      {
        name: 'privateDiagnostic',
        canary: 'PRIVATE_THREAD_DIAGNOSTIC_CANARY',
        thread: { ...ordinaryThreadDetail(), privateDiagnostic: 'PRIVATE_THREAD_DIAGNOSTIC_CANARY' }
      },
      {
        name: 'AuthorityRef',
        canary: authorityRef,
        thread: { ...ordinaryThreadDetail(), AuthorityRef: authorityRef }
      },
      {
        name: 'phone PII',
        canary: phone,
        thread: (() => {
          const thread = ordinaryThreadDetail() as Record<string, any>
          thread.turns[0].prompt = `Call ${phone}`
          thread.turns[0].items[0].text = `Call ${phone}`
          return thread
        })()
      }
    ]
    for (const test of cases) {
      const rejected = sanitizeRuntimeResponse({
        ok: true,
        status: 200,
        body: JSON.stringify(test.thread)
      }, '/v1/threads/thread-plan-public-detail', null, 'GET')
      expect(rejected.ok, test.name).toBe(false)
      expect(rejected.status, test.name).toBe(502)
      expect(rejected.body, test.name).not.toContain(test.canary)
    }
  })

  it('admits the exact closed case-compaction marker on the detail public seam', () => {
    const thread = caseCompactionThreadDetail()
    const parsed = ThreadDetailResponseV1Schema.safeParse(thread)
    expect(parsed.success, parsed.success ? '' : JSON.stringify(parsed.error.issues)).toBe(true)
    expect(sanitizePublicRuntimeValue(thread)).toEqual(thread)

    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-case-compaction-detail', null, 'GET')
    expect(response.ok, response.body).toBe(true)
    expect(response.status).toBe(200)
    expect(JSON.parse(response.body)).toEqual(thread)
  })

  it.each([
    ['taskContinuation', (item: Record<string, unknown>) => {
      item.taskContinuation = { schemaVersion: 1, privateText: 'must-not-cross' }
    }],
    ['sourceContextDigest', (item: Record<string, unknown>) => {
      item.sourceContextDigest = 'e'.repeat(64)
    }],
    ['caseCompactionBinding', (item: Record<string, unknown>) => {
      item.caseCompactionBinding = { schemaVersion: 1, privateText: 'must-not-cross' }
    }],
    ['privateDiagnostic', (item: Record<string, unknown>) => {
      item.privateDiagnostic = 'must-not-cross'
    }],
    ['replacedTokens', (item: Record<string, unknown>) => {
      item.replacedTokens = 512
    }],
    ['downgradedCaseProjection', (item: Record<string, unknown>) => {
      item.caseHistoryProjectionVersion = 1
      item.replacedTokens = 512
    }]
  ])('rejects private case-compaction field %s without reflecting it', (field, mutate) => {
    const thread = caseCompactionThreadDetail()
    const turn = (thread.turns as Array<Record<string, unknown>>)[0]
    const item = (turn.items as Array<Record<string, unknown>>)[0]
    mutate(item)

    expect(ThreadDetailResponseV1Schema.safeParse(thread).success).toBe(false)
    const response = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-case-compaction-detail', null, 'GET')
    expect(response.ok).toBe(false)
    expect(response.status).toBe(502)
    expect(response.body).not.toContain(field)
    expect(response.body).not.toContain('must-not-cross')
  })

  it('admits only the closed host message on a general terminal error snapshot', () => {
    const thread = ordinaryThreadDetail()
    const turn = (thread.turns as Array<Record<string, unknown>>)[0]
    const error = (turn.items as Array<Record<string, unknown>>)[3]
    error.code = 'tool_failure_storm'
    error.message = 'Repeated tool failures caused the host to stop the turn.'

    expect(ThreadDetailResponseV1Schema.safeParse(thread).success).toBe(true)
    expect(sanitizePublicRuntimeValue(thread)).toEqual(thread)
    const accepted = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')
    expect(accepted.ok, accepted.body).toBe(true)

    error.message = 'Untrusted runtime diagnostic.'
    expect(sanitizePublicRuntimeValue(thread)).not.toEqual(thread)
    const rejected = sanitizeRuntimeResponse({
      ok: true,
      status: 200,
      body: JSON.stringify(thread)
    }, '/v1/threads/thread-plan-public-detail', null, 'GET')
    expect(rejected.ok).toBe(false)
    expect(rejected.status).toBe(502)
    expect(rejected.body).not.toContain('Untrusted runtime diagnostic.')
  })
})


function ordinaryChildLedgerDetail() {
  const stamp = '2026-09-11T00:00:00Z'
  return {
    id: 'thread-child-ledger', title: 'Ordinary work', workspace: '/Volumes/AnalytixCache/development-v3/tmp/own2-shared-public-consumer-20260911/fixture-abcdefghijklmnopabcdefghijklmnop/ordinary-workspace',
    model: 'test-model', mode: 'agent', status: 'idle', approvalPolicy: 'auto',
    sandboxMode: 'workspace-write', relation: 'primary', createdAt: stamp, updatedAt: stamp,
    latestSeq: 170, messageCount: 0, turnCount: 1, pendingApprovalIds: [], pendingUserInputIds: [],
    turns: [{
      id: 'turn-child-ledger', threadId: 'thread-child-ledger', status: 'completed', prompt: 'Ordinary work',
      createdAt: stamp, finishedAt: stamp, steering: [], attachmentIds: [], activeSkillIds: [], injectedMemoryIds: [],
      items: [{
        id: 'item-child-ledger', threadId: 'thread-child-ledger', turnId: 'turn-child-ledger',
        kind: 'tool_progress', role: 'tool', status: 'completed', createdAt: stamp, finishedAt: stamp,
        toolName: 'background_delivery', summary: 'child output withheld', message: 'child output withheld',
        arguments: {
          runtimeStatus: 'tool_progress', stage: 'background_job_delivery_pending', status: 'pending',
          diagnostics: {
            kind: 'subagent_task', id: 'job-child', jobId: 'job-child', childRunId: 'job-child',
            status: 'completed', childStatus: 'completed', terminal: true, deliveryStatus: 'pending',
            outputWithheld: true, outputTrustStatus: 'untrusted_child_output',
            factAnswerAllowed: false, evidenceAuthority: false, canReadOutput: false, canContinueParent: false
          }
        }
      }]
    }]
  }
}

describe('ordinary child ledger HTTP contract', () => {
  function response(detail = ordinaryChildLedgerDetail()) {
    return sanitizeRuntimeResponse({ ok: true, status: 200, body: JSON.stringify(detail) },
      `/v1/threads/${detail.id}`, null, 'GET')
  }
  it('hydrates the exact public child ledger with no provider callId', () => {
    const detail = ordinaryChildLedgerDetail()
    expect(ThreadDetailResponseV1Schema.safeParse(detail).success).toBe(true)
    expect(response(detail)).toEqual({ ok: true, status: 200, body: expect.any(String) })
    expect(JSON.parse(response(detail).body)).toEqual(detail)
  })
  it('keeps the workspace PII guard and separates it from the item union', () => {
    const detail = ordinaryChildLedgerDetail()
    detail.turns[0].items = []
    expect(response(detail).ok).toBe(true)
    detail.workspace = '/synthetic/TestFixture1234567890/001/ordinary-workspace'
    expect(ThreadDetailResponseV1Schema.safeParse(detail).success).toBe(true)
    expect(response(detail).ok).toBe(false)
  })
  it.each(['unknown kind', 'raw payload', 'authority', 'parent binding', 'status mismatch'])('rejects %s after a valid control', (change) => {
    const detail = ordinaryChildLedgerDetail()
    expect(response(detail).ok).toBe(true)
    const item = detail.turns[0].items[0]
    if (change === 'unknown kind') item.kind = 'unknown_child'
    if (change === 'raw payload') Object.assign(item.arguments, { output: 'raw child output' })
    if (change === 'authority') item.arguments.diagnostics.evidenceAuthority = true
    if (change === 'parent binding') Object.assign(item.arguments.diagnostics, { parentTurnId: 'foreign-turn' })
    if (change === 'status mismatch') item.arguments.status = 'delivered'
    expect(response(detail).ok).toBe(false)
  })
})
