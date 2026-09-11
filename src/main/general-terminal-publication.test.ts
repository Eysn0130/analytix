import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT } from '../../packages/runtime/src/contracts/events'
import {
  GENERAL_TERMINAL_PUBLIC_INTEGRITY_SCOPE_V1,
  generalTerminalDeliveryBatchVerificationV1,
  verifiedGeneralTerminalDeliveryBatchV1
} from './general-terminal-publication'

const EVENT_ID_DOMAIN = Buffer.from('analytix/general-terminal-event/v1\0')
const MANIFEST_DOMAIN = Buffer.from('analytix/general-terminal-delivery-manifest/v1\0')
const PROJECTED_DOMAIN = Buffer.from('analytix/general-terminal-projected-events/v1\0')
const BATCH_DOMAIN = Buffer.from('analytix/general-terminal-delivery-batch/v1\0')
const ORDINARY_RESULT_DOMAIN = Buffer.from('analytix/ordinary-result/v1\0')
const HOST_FIXED_PROVIDER_RESULT_WITHHELD_TEXT =
  '普通任务已执行，但模型结果正文未通过普通输出安全投影，因此未予发布。'
const HOST_FIXED_PROTECTED_FACT_BLOCKED_TEXT =
  '本轮模型输出包含必须经过案件证据门核验的事实候选，宿主已阻止该草稿发布。请在已绑定的案件项目中重新发起核验。'

type OrdinaryCandidateOrigin = 'provider_ordinary_only' | 'host_fixed'

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function domainDigest(domain: Buffer, value: string): string {
  return sha256(Buffer.concat([domain, Buffer.from(value)]))
}

function goString(value: string): string {
  return JSON.stringify(value)
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function goCanonical(value: unknown): string {
  if (value === null || typeof value !== 'object') {
    if (typeof value === 'string') return goString(value)
    if (typeof value === 'number' && Object.is(value, -0)) return '-0'
    return JSON.stringify(value)
  }
  if (Array.isArray(value)) return `[${value.map(goCanonical).join(',')}]`
  const record = value as Record<string, unknown>
  const keys = Object.keys(record).sort((left, right) =>
    Buffer.compare(Buffer.from(left), Buffer.from(right)))
  return `{${keys.map((key) => `${goString(key)}:${goCanonical(record[key])}`).join(',')}}`
}

function manifestJSON(manifest: Array<Record<string, string>>): string {
  return `[${manifest.map((entry) =>
    `{"slot":${goString(entry.slot)},"eventId":${goString(entry.eventId)},` +
      `"payloadDigest":${goString(entry.payloadDigest)}}`).join(',')}]`
}

function batchJSON(batch: Record<string, any>): string {
  const field = (key: string, value: unknown): string => `${goString(key)}:${goCanonical(value)}`
  return `{${[
    field('schemaVersion', batch.schemaVersion), field('purpose', batch.purpose),
    field('kind', batch.kind), field('batchDigest', ''), field('threadId', batch.threadId),
    field('turnId', batch.turnId), field('seq', batch.seq), field('firstSeq', batch.firstSeq),
    field('lastSeq', batch.lastSeq), field('timestamp', batch.timestamp),
    field('generalTerminalCommitId', batch.generalTerminalCommitId),
    field('generalTerminalAuthorityKind', batch.generalTerminalAuthorityKind),
    field('generalTerminalAuthorityDigest', batch.generalTerminalAuthorityDigest),
    field('eventManifestDigest', batch.eventManifestDigest),
    field('projectedEventsDigest', batch.projectedEventsDigest),
    field('transportAuthority', batch.transportAuthority),
    field('evidenceAuthority', batch.evidenceAuthority),
    field('citationAuthority', batch.citationAuthority),
    field('factAnswerAllowed', batch.factAnswerAllowed),
    `${goString('events')}:${goCanonical(batch.events)}`,
    `${goString('eventManifest')}:${manifestJSON(batch.eventManifest)}`
  ].join(',')}}`
}

function resealPublicDigests(batch: Record<string, any>): void {
  batch.eventManifestDigest = domainDigest(MANIFEST_DOMAIN, manifestJSON(batch.eventManifest))
  batch.projectedEventsDigest = domainDigest(PROJECTED_DOMAIN, goCanonical(batch.events))
  batch.batchDigest = domainDigest(BATCH_DOMAIN, batchJSON(batch))
}

function ordinaryResultDigestJSON(slot: Record<string, unknown>): string {
  const field = (key: string, value: unknown): string => `${goString(key)}:${goCanonical(value)}`
  return `{${[
    field('schemaVersion', slot.schemaVersion),
    field('purpose', slot.purpose),
    field('projectionVersion', slot.projectionVersion),
    field('logicalEffect', slot.logicalEffect),
    field('ordinaryWork', slot.ordinaryWork),
    field('candidateOrigin', slot.candidateOrigin),
    field('evidenceAuthority', slot.evidenceAuthority),
    field('citationAuthority', slot.citationAuthority),
    field('factAnswerAllowed', slot.factAnswerAllowed),
    field('text', slot.text),
    field('textSha256', slot.textSha256),
    field('resultDigest', '')
  ].join(',')}}`
}

function ordinaryResultSlot(
  text: string,
  candidateOrigin: OrdinaryCandidateOrigin = 'provider_ordinary_only'
): Record<string, unknown> {
  const slot: Record<string, unknown> = {
    schemaVersion: 1,
    purpose: 'analytix.ordinary-result/v1',
    projectionVersion: 'analytix.ordinary-output-projection/v1',
    logicalEffect: 'ordinary',
    ordinaryWork: true,
    candidateOrigin,
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    text,
    textSha256: sha256(text),
    resultDigest: ''
  }
  slot.resultDigest = domainDigest(ORDINARY_RESULT_DOMAIN, ordinaryResultDigestJSON(slot))
  return slot
}

function fixture(): Record<string, any> {
  const timestamp = '2026-07-20T03:00:00Z'
  const threadId = 'thread-general'
  const turnId = 'turn-general'
  const commitId = sha256('general-terminal-commit')
  const slots = ['terminal-item', 'usage', 'terminal']
  const events = [
    {
      kind: 'item_completed', seq: 11, timestamp, threadId, turnId, itemId: 'item-general',
      item: {
        id: 'item-general', turnId, threadId, role: 'assistant', status: 'completed',
        createdAt: timestamp, finishedAt: timestamp, kind: 'assistant_text',
        text: GENERAL_PROVIDER_FINAL_QUARANTINED_TEXT
      }
    },
    {
      kind: 'usage', seq: 12, timestamp, threadId, turnId, model: 'deepseek-chat',
      usage: {
        promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
        cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null,
        cacheMissReasons: [], cacheSuggestions: [], costUsd: 0, costCny: 0,
        priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0, turns: 1
      },
      cacheDiagnostics: {}, usageFinalStatus: 'completed'
    },
    {
      kind: 'turn_completed', seq: 13, timestamp, threadId, turnId,
      status: 'completed', terminalReason: 'success'
    }
  ]
  const eventManifest = slots.map((slot, index) => ({
    slot,
    eventId: domainDigest(EVENT_ID_DOMAIN, `${commitId}\0${slot}`),
    payloadDigest: sha256(`opaque-private-payload-${index}`)
  }))
  const batch: Record<string, any> = {
    schemaVersion: 1,
    purpose: 'analytix.general-terminal-delivery-batch/v1',
    kind: 'general_terminal_batch',
    batchDigest: '',
    threadId,
    turnId,
    seq: 13,
    firstSeq: 11,
    lastSeq: 13,
    timestamp,
    generalTerminalCommitId: commitId,
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: sha256('opaque-host-cas-authority'),
    eventManifestDigest: '',
    projectedEventsDigest: '',
    transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    events,
    eventManifest
  }
  resealPublicDigests(batch)
  return batch
}

function typedFixture(
  text = '已完成代码修改并通过相关测试。',
  candidateOrigin: OrdinaryCandidateOrigin = 'provider_ordinary_only'
): Record<string, any> {
  const batch = fixture()
  batch.events[0].item.text = text
  batch.events[0].item.ordinaryResult = ordinaryResultSlot(text, candidateOrigin)
  resealPublicDigests(batch)
  return batch
}

function failureFixture(): Record<string, any> {
  const batch = fixture()
  const message = 'The provider request timed out before a verified response was available.'
  Object.assign(batch.events[0].item, {
    role: 'system', status: 'failed', kind: 'error', code: 'provider_timeout',
    message, severity: 'error'
  })
  delete batch.events[0].item.text
  batch.events[1].usageFinalStatus = 'failed'
  Object.assign(batch.events[2], {
    kind: 'turn_failed', status: 'failed', terminalReason: 'provider_failure',
    itemId: batch.events[0].itemId, code: 'provider_timeout', message, error: message,
    severity: 'error'
  })
  resealPublicDigests(batch)
  return batch
}

function clone<T>(value: T): T {
  return structuredClone(value)
}

describe('verifiedGeneralTerminalDeliveryBatchV1', () => {
  it('recomputes the four public transport bindings without granting fact authority', () => {
    const batch = fixture()
    const verified = verifiedGeneralTerminalDeliveryBatchV1(batch)

    expect(verified).toMatchObject({
      firstSeq: 11,
      lastSeq: 13,
      generalTerminalCommitId: batch.generalTerminalCommitId,
      verification: GENERAL_TERMINAL_PUBLIC_INTEGRITY_SCOPE_V1
    })
    expect(verified?.events).toHaveLength(3)
    expect(verified?.batch).toMatchObject({
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false
    })
  })

  it('accepts a Go-compatible typed ordinary result with bound candidate origin and digests', () => {
    const batch = typedFixture()
    const slot = batch.events[0].item.ordinaryResult

    expect(slot).toMatchObject({
      candidateOrigin: 'provider_ordinary_only',
      textSha256: 'e6648d12f3d4a8317c791895058d325a6d21a1b22074638d955b4fcbadd9213e',
      resultDigest: 'd32215cd711a592e7c095d1c4f29546c0cdb0d5cb4fb97891e2b7cfc3adbb63f',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false
    })
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)?.events[0]).toMatchObject({
      item: {
        text: batch.events[0].item.text,
        ordinaryResult: slot
      }
    })
  })

  it.each([
    ['ordinary PII', () => typedFixture('Read http://127.0.0.1:8899 for the result.'),
      'general_terminal_batch_ordinary_pii'],
    ['private reasoning', () => {
      const batch = typedFixture()
      batch.events[1].reasoning = 'private chain'
      return batch
    }, 'general_terminal_batch_private_reasoning'],
    ['integrity mismatch', () => {
      const batch = typedFixture()
      batch.batchDigest = 'a'.repeat(64)
      return batch
    }, 'general_terminal_batch_integrity_invalid']
  ])('returns only a closed safe reason for %s', (_label, build, reason) => {
    const result = generalTerminalDeliveryBatchVerificationV1(build())

    expect(result).toEqual({ verified: null, reason })
    expect(Object.keys(result)).toEqual(['verified', 'reason'])
  })

  it.each([
    HOST_FIXED_PROVIDER_RESULT_WITHHELD_TEXT,
    HOST_FIXED_PROTECTED_FACT_BLOCKED_TEXT
  ])('accepts the closed host-fixed ordinary result: %s', (text) => {
    const batch = typedFixture(text, 'host_fixed')
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)?.events[0]).toMatchObject({
      item: { ordinaryResult: { candidateOrigin: 'host_fixed', text } }
    })
  })

  it('rejects arbitrary prose relabelled as host-fixed after every digest is recomputed', () => {
    const batch = typedFixture('已完成代码修改并通过相关测试。', 'host_fixed')
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('rejects an unknown candidate origin after both slot and outer digests are recomputed', () => {
    const batch = typedFixture()
    const slot = batch.events[0].item.ordinaryResult
    slot.candidateOrigin = 'provider_untrusted'
    slot.resultDigest = domainDigest(ORDINARY_RESULT_DOMAIN, ordinaryResultDigestJSON(slot))
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['item text mismatch', (batch: Record<string, any>) => {
      batch.events[0].item.text = '这是另一个普通结果。'
    }],
    ['wrong text SHA-256', (batch: Record<string, any>) => {
      const slot = batch.events[0].item.ordinaryResult
      slot.textSha256 = sha256('different ordinary result')
      slot.resultDigest = domainDigest(ORDINARY_RESULT_DOMAIN, ordinaryResultDigestJSON(slot))
    }],
    ['wrong result digest', (batch: Record<string, any>) => {
      batch.events[0].item.ordinaryResult.resultDigest = sha256('different result digest')
    }]
  ])('rejects a typed ordinary result with %s after the outer batch is resealed', (_label, mutate) => {
    const batch = typedFixture()
    mutate(batch)
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['leading ASCII whitespace', ' 已完成代码修改。'],
    ['trailing Go Unicode whitespace', '已完成代码修改。\u3000']
  ])('rejects typed ordinary text with %s instead of silently trimming it', (_label, text) => {
    const batch = typedFixture(text)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['internal entity reference', `已完成对象整理：cer1_${'a'.repeat(64)}`],
    ['private source-row reference', `已完成来源整理：srow1_${'a'.repeat(64)}`],
    ['JSON-escaped internal reference', `已完成对象整理：\\u0063er1_${'a'.repeat(64)}`],
    ['protected case fact', '该案涉案金额为 2,645,472 元。'],
    ['protected payment fact', '该案支付 2645472 元。']
  ])('rejects a fully resealed typed ordinary result containing %s', (_label, text) => {
    const batch = typedFixture(text)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('accepts the exact two-slot failure profile', () => {
    const batch = fixture()
    batch.events = batch.events.slice(1)
    batch.eventManifest = batch.eventManifest.slice(1)
    batch.firstSeq = 12
    batch.events[0].usageFinalStatus = 'failed'
    batch.events[1].kind = 'turn_failed'
    batch.events[1].status = 'failed'
    batch.events[1].terminalReason = 'provider_failure'
    resealPublicDigests(batch)

    const verified = verifiedGeneralTerminalDeliveryBatchV1(batch)
    expect(verified?.events.map((event) => event.kind)).toEqual(['usage', 'turn_failed'])
    expect(verified).toMatchObject({ firstSeq: 12, lastSeq: 13 })
  })

  it('accepts a host-canonical failure projection and rejects open provider diagnostics', () => {
    const batch = failureFixture()
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)?.events[0]).toMatchObject({
      item: {
        code: 'provider_timeout',
        severity: 'error'
      }
    })

    batch.events[0].item.details = { retryAfterMs: 250, endpointFormat: 'messages' }
    batch.events[2].details = { retryAfterMs: 250, endpointFormat: 'messages' }
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('accepts the Go maximum canonical prompt plus completion total', () => {
    const batch = fixture()
    Object.assign(batch.events[1].usage, {
      promptTokens: 1_000_000_000,
      completionTokens: 1_000_000_000,
      reasoningTokens: 1_000_000_000,
      totalTokens: 2_000_000_000
    })
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).not.toBeNull()
  })

  it('matches Go HTML-safe JSON escaping for projected public text', () => {
    const batch = fixture()
    batch.events[1].model = 'model-<>&\u2028line'
    resealPublicDigests(batch)

    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).not.toBeNull()
  })

  it('rejects repeat-hex placeholders that merely satisfy digest shapes', () => {
    const batch = fixture()
    batch.batchDigest = 'a'.repeat(64)
    batch.generalTerminalCommitId = 'b'.repeat(64)
    batch.generalTerminalAuthorityDigest = 'c'.repeat(64)
    batch.eventManifestDigest = 'd'.repeat(64)
    batch.projectedEventsDigest = 'e'.repeat(64)
    batch.eventManifest[0].eventId = 'f'.repeat(64)
    batch.eventManifest[0].payloadDigest = '1'.repeat(64)

    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['assistant text', (batch: Record<string, any>) => { batch.events[0].item.text = 'fabricated case fact' }],
    ['projected text', (batch: Record<string, any>) => { batch.events[1].model = 'mutated-model' }],
    ['nested sequence', (batch: Record<string, any>) => { batch.events[1].seq = 13 }],
    ['outer sequence', (batch: Record<string, any>) => { batch.seq = 12 }],
    ['slot', (batch: Record<string, any>) => { batch.eventManifest[0].slot = 'usage' }],
    ['event id', (batch: Record<string, any>) => { batch.eventManifest[0].eventId = sha256('other-event') }],
    ['payload manifest', (batch: Record<string, any>) => { batch.eventManifest[1].payloadDigest = sha256('other-payload') }],
    ['manifest digest', (batch: Record<string, any>) => { batch.eventManifestDigest = sha256('other-manifest') }],
    ['projected digest', (batch: Record<string, any>) => { batch.projectedEventsDigest = sha256('other-projection') }],
    ['batch digest', (batch: Record<string, any>) => { batch.batchDigest = sha256('other-batch') }]
  ])('rejects %s mutation', (_name, mutate) => {
    const batch = clone(fixture())
    mutate(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['token above Go bound', (batch: Record<string, any>) => {
      batch.events[1].usage.promptTokens = 1_000_000_001
      batch.events[1].usage.totalTokens = 1_000_000_002
    }],
    ['cost above Go bound', (batch: Record<string, any>) => {
      batch.events[1].usage.costUsd = 1_000_000_000_001
    }],
    ['reasoning above completion', (batch: Record<string, any>) => {
      batch.events[1].usage.reasoningTokens = 2
    }],
    ['missing canonical usage field', (batch: Record<string, any>) => {
      delete batch.events[1].usage.turns
    }],
    ['partial cache counter trio', (batch: Record<string, any>) => {
      batch.events[1].usage.cacheHitTokens = 1
    }],
    ['inconsistent cache rates', (batch: Record<string, any>) => {
      Object.assign(batch.events[1].usage, {
        cachedTokens: 1, cacheHitTokens: 1, cacheMissTokens: 1,
        cacheHitRate: 0.25, cacheableTokenHitRate: 0.5, totalInputTokenHitRate: 0.5
      })
    }],
    ['orphan context epoch diagnostic', (batch: Record<string, any>) => {
      batch.events[1].cacheDiagnostics.contextEpoch = 1
    }],
    ['cache details without presence authority', (batch: Record<string, any>) => {
      Object.assign(batch.events[1].cacheDiagnostics, {
        cacheHitTokens: 1, cacheMissTokens: 0, cacheHitRate: 1,
        cacheTelemetrySource: 'provider_usage'
      })
    }],
    ['negative zero usage token', (batch: Record<string, any>) => {
      batch.events[1].usage.promptTokens = -0
    }],
    ['negative zero cache diagnostic', (batch: Record<string, any>) => {
      batch.events[1].cacheDiagnostics.firstTokenLatencyMs = -0
    }],
    ['negative zero cache rate', (batch: Record<string, any>) => {
      Object.assign(batch.events[1].usage, {
        cachedTokens: 0, cacheHitTokens: 0, cacheMissTokens: 1,
        cacheHitRate: -0, cacheableTokenHitRate: -0, totalInputTokenHitRate: -0
      })
    }],
    ['blank usage source', (batch: Record<string, any>) => {
      batch.events[1].usageSource = '   '
    }],
    ['blank item identity', (batch: Record<string, any>) => {
      batch.events[0].itemId = '   '
      batch.events[0].item.id = '   '
      batch.events[2].itemId = '   '
    }],
    ['NEL item identity boundary', (batch: Record<string, any>) => {
      batch.events[0].itemId = '\u0085item-general'
      batch.events[0].item.id = '\u0085item-general'
      batch.events[2].itemId = '\u0085item-general'
    }],
    ['BOM usage source boundary', (batch: Record<string, any>) => {
      batch.events[1].usageSource = '\ufeffprovider_usage'
    }],
    ['slash item identity', (batch: Record<string, any>) => {
      batch.events[0].itemId = 'item/general'
      batch.events[0].item.id = 'item/general'
      batch.events[2].itemId = 'item/general'
    }],
    ['overlong child run identity', (batch: Record<string, any>) => {
      batch.events[1].childRunId = 'r'.repeat(257)
    }],
    ['created timestamp after finished timestamp', (batch: Record<string, any>) => {
      batch.events[0].item.createdAt = '2026-07-20T03:00:00.000000001Z'
    }],
    ['oversized public text', (batch: Record<string, any>) => {
      batch.events[1].model = 'm'.repeat((1 << 20) + 1)
    }],
    ['private reasoning in schema-valid model text', (batch: Record<string, any>) => {
      batch.events[1].model = 'reasoning_content: hidden'
    }],
    ['credential in schema-valid model text', (batch: Record<string, any>) => {
      batch.events[1].model = 'api_key=sk-test-private-value'
    }],
    ['nested timestamp above nanosecond precision', (batch: Record<string, any>) => {
      batch.events[0].item.createdAt = '2026-07-20T02:59:59.1234567890Z'
    }],
    ['nested timestamp with invalid offset', (batch: Record<string, any>) => {
      batch.events[0].item.createdAt = '2026-07-20T02:59:59+24:00'
    }],
    ['ten digit fractional timestamp', (batch: Record<string, any>) => {
      const timestamp = '2026-07-20T03:00:00.1234567890Z'
      batch.timestamp = timestamp
      for (const event of batch.events) event.timestamp = timestamp
      batch.events[0].item.createdAt = timestamp
      batch.events[0].item.finishedAt = timestamp
    }]
  ])('rejects Go-incompatible %s even after public digests are resealed', (_name, mutate) => {
    const batch = clone(fixture())
    mutate(batch)
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it.each([
    ['unknown code', (batch: Record<string, any>) => {
      batch.events[0].item.code = 'provider_request_failed'
      batch.events[2].code = 'provider_request_failed'
    }],
    ['non-canonical message', (batch: Record<string, any>) => {
      batch.events[0].item.message = 'provider timed out'
      batch.events[2].message = 'provider timed out'
      batch.events[2].error = 'provider timed out'
    }],
    ['wrong severity', (batch: Record<string, any>) => {
      batch.events[0].item.severity = 'warning'
      batch.events[2].severity = 'warning'
    }],
    ['empty failure details', (batch: Record<string, any>) => {
      batch.events[0].item.details = {}
      batch.events[2].details = {}
    }],
    ['different canonical lifecycle failure', (batch: Record<string, any>) => {
      const message = 'The provider rate limit was reached. Retry after the bounded delay.'
      Object.assign(batch.events[2], {
        code: 'provider_rate_limited', message, error: message, severity: 'error'
      })
    }],
    ['different failure details', (batch: Record<string, any>) => {
      batch.events[0].item.details = { retryAfterMs: 250 }
      batch.events[2].details = { retryAfterMs: 251 }
    }],
    ['negative zero failure detail', (batch: Record<string, any>) => {
      batch.events[0].item.details = { retryAfterMs: -0 }
      batch.events[2].details = { retryAfterMs: -0 }
    }]
  ])('rejects %s in a resealed failure batch', (_name, mutate) => {
    const batch = clone(failureFixture())
    mutate(batch)
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('rejects a successful item whose lifecycle carries a canonical failure profile', () => {
    const batch = fixture()
    const message = 'The provider request timed out before a verified response was available.'
    Object.assign(batch.events[2], {
      code: 'provider_timeout', message, error: message, severity: 'error'
    })
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('rejects pending gate cancellation while cancelled=false', () => {
    const batch = fixture()
    batch.events = batch.events.slice(1)
    batch.eventManifest = batch.eventManifest.slice(1)
    batch.firstSeq = 12
    batch.events[0].usageFinalStatus = 'aborted'
    Object.assign(batch.events[1], {
      kind: 'turn_aborted',
      status: 'aborted',
      terminalReason: 'cancel',
      discard: true,
      cancelled: false,
      cancelledPendingGates: 1
    })
    resealPublicDigests(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })

  it('keeps raw payload and host CAS values explicitly opaque even when public digests are resealed', () => {
    const batch = fixture()
    batch.generalTerminalAuthorityDigest = sha256('different-opaque-host-cas')
    batch.eventManifest.forEach((entry: Record<string, string>, index: number) => {
      entry.payloadDigest = sha256(`different-opaque-private-payload-${index}`)
    })
    resealPublicDigests(batch)

    const verified = verifiedGeneralTerminalDeliveryBatchV1(batch)
    expect(verified?.verification).toEqual({
      eventIds: 'recomputed',
      eventManifestDigest: 'recomputed',
      projectedEventsDigest: 'recomputed',
      batchDigest: 'recomputed',
      rawPayloadDigests: 'opaque_bound_only',
      hostCASAuthority: 'opaque_bound_only'
    })
  })

  it.each([
    ['non-canonical timestamp', (batch: Record<string, any>) => { batch.timestamp = '2026-07-20T03:00:00.000Z' }],
    ['offset timestamp', (batch: Record<string, any>) => { batch.timestamp = '2026-07-20T11:00:00+08:00' }],
    ['trimmed thread identity', (batch: Record<string, any>) => { batch.threadId = ' thread-general' }],
    ['unknown field', (batch: Record<string, any>) => { batch.untrusted = true }]
  ])('rejects %s before publication', (_name, mutate) => {
    const batch = clone(fixture())
    mutate(batch)
    expect(verifiedGeneralTerminalDeliveryBatchV1(batch)).toBeNull()
  })
})
