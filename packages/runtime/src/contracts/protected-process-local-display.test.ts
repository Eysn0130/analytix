import { describe, expect, it } from 'vitest'
import {
  processDisplayBlockSchemaV1, processDisplaySourceSchemaV2, processDisplaySnapshotSchemaV1, PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1,
  processDisplayReadRequestSchemaV1, processDisplayTargetRequestSchemaV1,
  processPaneAuthorizeRequestSchemaV1, processPaneReadRequestSchemaV1,
  processPaneExpansionRequestSchemaV1, processPaneTargetRequestSchemaV1,
  processPaneAuthorizeResultSchemaV1, processPaneWebLinkRequestSchemaV1, processDisplayWebLinkRequestSchemaV1,
  processDisplayWebLinkResultSchemaV1, PROCESS_LOCAL_DISPLAY_MAX_WEB_URL_BYTES_V1, hasProcessDisplayPayloadV2
} from './protected-process-local-display'
const scope = { threadId: 'thread-a', turnId: 'turn-a', contextDigest: 'b'.repeat(64), contextEpoch: '1', processIncarnation: 'incarnation-a' }
const viewerId = '11111111-1111-4111-8111-111111111111'
const body = { text: 'original\r\n', before: '', after: '', error: '', addedLines: 0, removedLines: 0, approximate: false }
const source = { kind: 'tool' as const, unitId: 'unit-a', callId: 'call-a', toolCallItemId: 'call-item-a', toolResultItemId: 'result-item-a', startedEventSeq: '12' }
const unit = { unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', placement: 'anchored', calls: [{ callId: source.callId, toolCallItemId: source.toolCallItemId, toolResultItemId: source.toolResultItemId, startedEventSeq: source.startedEventSeq }], precedingCalls: [] }
const block = { blockId: 'block-a', source, kind: 'shell', state: 'timeout', attempt: 0, durationMs: 3, exitCode: 7,
  truncated: false, classification: 'source_exact_local', captureDigest: 'a'.repeat(64), body }

it('attributes only actual reasoning sources as summaries without widening other source shapes', () => {
  const reasoning = { kind: 'provider_reasoning', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', sourceOrdinal: '1', attempt: 1 }
  expect(processDisplaySourceSchemaV2.parse({ ...reasoning, reasoningForm: 'summary' })).toMatchObject({ reasoningForm: 'summary' })
  expect(processDisplaySourceSchemaV2.parse(reasoning)).toEqual(reasoning)
  expect(processDisplaySourceSchemaV2.parse({ ...reasoning, kind: 'native', nativeIndex: 0, reasoningForm: 'summary' })).toMatchObject({ reasoningForm: 'summary' })
  for (const value of [{ ...reasoning, reasoningForm: 'full' }, { ...reasoning, reasoningForm: '' },
    { ...reasoning, kind: 'assistant_draft', sourceOrdinal: undefined, reasoningForm: 'summary' }, { ...source, reasoningForm: 'summary' }]) {
    expect(processDisplaySourceSchemaV2.safeParse(value).success).toBe(false)
  }
})

describe('complete private running input', () => {
  const inputJSON = ' {\n"number":9007199254740993,"exponent":1e+03,"case":"Alice"\n} '
  const running = { blockId: 'raw-a', source: { ...source, toolResultItemId: undefined }, kind: 'tool_input', state: 'running',
    attempt: 0, durationMs: 0, truncated: false, classification: 'source_exact_local', input: { availability: 'available', inputJSON } }
  it('preserves whole text and metadata-only fold without inventing a result', () => {
    for (const kind of ['tool_input', 'shell', 'web_fetch']) {
      const parsed = processDisplayBlockSchemaV1.parse({ ...running, kind })
      expect(parsed.input?.availability === 'available' && parsed.input.inputJSON).toBe(inputJSON)
      expect(hasProcessDisplayPayloadV2(parsed)).toBe(true)
      const folded = processDisplayBlockSchemaV1.parse({ ...running, kind, input: undefined })
      expect(hasProcessDisplayPayloadV2(folded)).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.parse({ ...running, classification: 'withheld', input: { availability: 'unavailable', reason: 'limit' } }).input?.availability).toBe('unavailable')
  })
  it('rejects unbound, historical, prefix and open input shapes', () => {
    for (const extra of [{ state: 'preparing' }, { state: 'completed' }, { source }, { source: { ...running.source, startedEventSeq: undefined } },
      { kind: 'write' }, { body }, { captureDigest: 'a'.repeat(64) }, { title: 'synthesized title' },
      { input: { availability: 'available', inputJSON: 'x'.repeat(65537) } }, { input: { availability: 'available', inputJSON: '\ud800' } },
      { input: { availability: 'available', inputJSON, truncated: true } }, { input: { availability: 'unavailable', reason: 'limit', inputJSON } }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...running, ...extra }).success).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...running, state: 'unknown', classification: 'withheld', input: undefined }).success).toBe(true)
  })
})
describe('closed protected process display contract', () => {
  it('preserves original local bytes, true shell outcome and independent mutation effects', () => {
    expect(processDisplayBlockSchemaV1.parse(block)).toEqual(block)
    const changed = { ...block, kind: 'write', state: 'failed', exitCode: undefined, effectApplied: true,
      settlementStatus: 'failed', path: '/synthetic/file.ts' }
    expect(processDisplayBlockSchemaV1.parse(changed).effectApplied).toBe(true)
    expect(processDisplayBlockSchemaV1.parse({ ...changed, effectApplied: undefined }).effectApplied).toBeUndefined()
    expect(processDisplayBlockSchemaV1.parse({ ...changed, effectApplied: null }).effectApplied).toBeNull()
  })
  it.each([
    ['title', 1024, false], ['preview', 1024, false], ['path', 4096, false],
    ['text', PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1, true], ['before', PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1, true],
    ['after', PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1, true], ['error', PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1, true]
  ] as const)('preserves NUL, Unicode and UTF-8 byte boundaries for %s', (field, limit, bodyField) => {
    const mutation = { ...block, kind: 'write', state: 'completed', exitCode: undefined }
    const accepts = (value: string) => processDisplayBlockSchemaV1.safeParse(bodyField
      ? { ...mutation, body: { ...body, [field]: value } }
      : { ...mutation, [field]: value }).success
    for (const value of ['', '\t\n\r', '中', '\uD83D\uDE00', 'a😀z']) expect(accepts(value)).toBe(true)
    for (const value of ['\0', 'before\0after', '\uD800', '\uDC00', '\uD800x', 'x\uDC00', '\uDC00\uD800', '😀\uD800', '\uDC00😀']) {
      expect(accepts(value)).toBe(false)
    }
    for (const value of ['x'.repeat(limit), '中'.repeat(Math.floor(limit / 3)) + 'x'.repeat(limit % 3), '😀'.repeat(limit / 4)]) {
      expect(accepts(value)).toBe(true)
      expect(accepts(value + 'x')).toBe(false)
    }
  })
  it('preserves only bounded mutation UTF-16 counts under immutable protected capture binding', () => {
    const mutation = { ...block, kind: 'write', state: 'completed', exitCode: undefined }
    for (const inputUtf16CodeUnits of [0, 1025, Number.MAX_SAFE_INTEGER]) {
      expect(processDisplayBlockSchemaV1.parse({ ...mutation, inputUtf16CodeUnits }).inputUtf16CodeUnits).toBe(inputUtf16CodeUnits)
    }
    expect(processDisplayBlockSchemaV1.parse(mutation).inputUtf16CodeUnits).toBeUndefined()
    for (const inputUtf16CodeUnits of [-1, 0.5, null, '1025', Number.NaN, Infinity, Number.MAX_SAFE_INTEGER + 1]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...mutation, inputUtf16CodeUnits }).success).toBe(false)
    }
    for (const extra of [{ kind: 'shell' }, { state: 'running', body: undefined }, { state: 'unknown', body: undefined },
      { state: 'superseded', body: undefined }, { classification: 'withheld', body: undefined }, { captureDigest: undefined }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...mutation, inputUtf16CodeUnits: 1025, ...extra }).success).toBe(false)
    }
  })
  it('preserves sealed native capture binding and removes unsupported private envelopes', () => {
    const native = { ...block, kind: 'native_reasoning', source: { kind: 'native', unitId: 'unit-a', producerId: 'producer-a', providerSequence: '1', sourceOrdinal: '1', attempt: 1, nativeIndex: 1 }, attempt: 1, state: 'completed', exitCode: undefined, nativeIndex: 1 }
    expect(processDisplayBlockSchemaV1.parse(native).body?.text).toBe(body.text)
    for (const extra of [{ state: 'running' }, { state: 'failed' }, { state: 'superseded' }, { signature: 'private' },
      { continuation: 'private' }, { providerJson: {} }, { path: '/private/source' }, { effectApplied: true }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...native, ...extra }).success).toBe(false)
    }
  })
  it('keeps withheld running descriptors value-free and forbids unbound settled bodies', () => {
    const running = { blockId: 'running-a', source: { ...source, toolResultItemId: undefined }, kind: 'shell', state: 'running', attempt: 0,
      durationMs: 0, truncated: false, classification: 'withheld' }
    expect(processDisplayBlockSchemaV1.safeParse(running).success).toBe(true)
    for (const extra of [{ title: '' }, { preview: '' }, { path: '' }, { body }, { exitCode: 0 }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...running, ...extra }).success).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...block, captureDigest: undefined }).success).toBe(false)
    expect(processDisplayBlockSchemaV1.safeParse({ ...block, classification: 'withheld' }).success).toBe(false)
  })
  it('keeps withheld descriptors free of every source-bearing value', () => {
    const withheld = { ...block, kind: 'write', state: 'completed', exitCode: undefined,
      body: undefined, captureDigest: undefined, classification: 'withheld' }
    expect(processDisplayBlockSchemaV1.safeParse(withheld).success).toBe(true)
    for (const fields of [{ title: 'synthetic private title' }, { preview: 'synthetic private preview' },
      { path: '/synthetic/private/file.ts' }, { title: '' }, { preview: '' }, { path: '' },
      { body, captureDigest: block.captureDigest }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...withheld, ...fields }).success).toBe(false)
    }
  })
  it('rejects nested open values, duplicate blocks, invalid text and aggregate overflow', () => {
    for (const value of [
      { ...block, body: { ...body, rawError: 'private' } },
      { ...block, body: { ...body, text: '\ud800' } },
      { ...block, body: { ...body, text: '中'.repeat(10923) } },
      { ...block, durationMs: -1 }, { ...block, exitCode: -1 }, { ...block, settlementStatus: 'provider says success' }
    ]) expect(processDisplayBlockSchemaV1.safeParse(value).success).toBe(false)
    const snapshot = { status: 'snapshot', schemaVersion: 2, units: [unit], revision: '1', scope, blocks: [block] }
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, blocks: [block, block] }).success).toBe(false)
    const large = Array.from({ length: 9 }, (_, index) => ({ ...block, blockId: 'block-' + index,
      body: { ...body, text: 'x'.repeat(32768) } }))
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, blocks: large }).success).toBe(false)
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, viewerToken: viewerId }).success).toBe(false)
  })
  it('keeps Main-only tokens and routes out of renderer requests', () => {
    const authorize = { paneId: 'primary', threadId: scope.threadId, turnId: scope.turnId }
    expect(processPaneAuthorizeRequestSchemaV1.safeParse(authorize).success).toBe(true)
    for (const extra of [{ threadId: 'thread:forged' }, { turnId: 'turn:forged' }, { paneId: 'pane:forged' },
      { viewerToken: viewerId }, { documentNonce: viewerId }, { principal: {} },
      { path: '/v1/local-display/process/read' }, { body: 'forged' }, { current: true }]) {
      expect(processPaneAuthorizeRequestSchemaV1.safeParse({ ...authorize, ...extra }).success).toBe(false)
    }
    expect(processPaneAuthorizeResultSchemaV1.safeParse({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, viewerId, revision: '0', scope }).success).toBe(true)
    expect(processPaneAuthorizeResultSchemaV1.safeParse({ status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, viewerId, revision: '0', scope, viewerToken: viewerId }).success).toBe(false)
  })
  it('bounds polls, revisions and expansion IDs, and admits only capture targets', () => {
    const read = { viewerId, afterRevision: '12', waitMs: 25000 }
    expect(processPaneReadRequestSchemaV1.safeParse(read).success).toBe(true)
    for (const extra of [{ waitMs: 25001 }, { afterRevision: '01' }, { afterRevision: '18446744073709551616' },
      { expandedBlockIds: ['block-a'] }, { viewerToken: viewerId }]) {
      expect(processPaneReadRequestSchemaV1.safeParse({ ...read, ...extra }).success).toBe(false)
    }
    expect(processPaneExpansionRequestSchemaV1.safeParse({ viewerId, expandedBlockIds: [] }).success).toBe(true)
    expect(processPaneExpansionRequestSchemaV1.safeParse({ viewerId, expandedBlockIds: ['a', 'a'] }).success).toBe(false)
    expect(processPaneExpansionRequestSchemaV1.safeParse({ viewerId, expandedBlockIds: Array.from({ length: 129 }, (_, i) => 'block-' + i) }).success).toBe(false)
    expect(processDisplayReadRequestSchemaV1.safeParse({ schemaVersion: 2, action: 'read', viewerToken: viewerId, expandedBlockIds: [], waitMs: 0 }).success).toBe(true)
    const target = { viewerId, blockId: block.blockId, captureDigest: block.captureDigest }
    expect(processPaneTargetRequestSchemaV1.safeParse(target).success).toBe(true)
    expect(processPaneTargetRequestSchemaV1.safeParse({ ...target, path: '/private/current-file' }).success).toBe(false)
    expect(processDisplayTargetRequestSchemaV1.safeParse({ action: 'reveal_target', viewerToken: viewerId, blockId: block.blockId, captureDigest: block.captureDigest }).success).toBe(false)
  })
  it('requires explicit V2 success and validates real source-to-unit anchors without mixing coordinates', () => {
    const authorized = { status: 'authorized', schemaVersion: 2, presence: { nativeBlockCount: 0, toolCaptureCount: 1 }, viewerId, revision: '0', scope }
    for (const extra of [{ schemaVersion: 1 }, { schemaVersion: undefined }, { presence: undefined },
      { presence: { nativeBlockCount: -1, toolCaptureCount: 0 } }, { scope: { ...scope, contextEpoch: '01' } }]) {
      expect(processPaneAuthorizeResultSchemaV1.safeParse({ ...authorized, ...extra }).success).toBe(false)
    }
    const snapshot = { status: 'snapshot', schemaVersion: 2, units: [unit], revision: '1', scope, blocks: [block] }
    for (const extra of [{ units: [] }, { units: [unit, unit] },
      { blocks: [{ ...block, source: { ...source, toolCallItemId: 'wrong-item' } }] },
      { blocks: [{ ...block, source: { ...source, toolResultItemId: 'wrong-result' } }] },
      { blocks: [{ ...block, source: { ...source, startedEventSeq: '13' } }] },
      { blocks: [{ ...block, source: { ...source, sourceOrdinal: '12' } }] },
      { units: [{ ...unit, providerSequence: '01' }] }]) {
      expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, ...extra }).success).toBe(false)
    }
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, blocks: [{ ...block, source: { ...source, startedEventSeq: undefined } }] }).success).toBe(true)
    expect(processDisplayReadRequestSchemaV1.safeParse({ action: 'read', viewerToken: viewerId, expandedBlockIds: [] }).success).toBe(false)
  })
  it('admits only actual attributed running drafts, retaining settled native/tool capture rules', () => {
    const draft = { blockId: 'draft-a', kind: 'assistant_draft', state: 'running', attempt: 2, durationMs: 3,
      source: { kind: 'assistant_draft', unitId: unit.unitId, producerId: unit.producerId, providerSequence: unit.providerSequence, attempt: 2 },
      contentClassification: 'unclassified_inline', classification: 'source_exact_local', truncated: false,
      draftSegments: [{ offsetBytes: '0', text: '<think>仍为过程草稿</think> ' }], draftBytes: String(new TextEncoder().encode('<think>仍为过程草稿</think> ').length) }
    expect(processDisplayBlockSchemaV1.parse(draft)).toEqual(draft)
    expect(processDisplayBlockSchemaV1.parse({ ...draft, contentClassification: 'provider_text' }).kind).toBe('assistant_draft')
    for (const extra of [{ source: { ...draft.source, nativeIndex: 0 } }, { nativeIndex: 0 }, { body }, { title: 'Answer' },
      { captureDigest: 'a'.repeat(64) }, { attempt: 1 }, { state: 'completed' }, { classification: 'withheld' },
      { contentClassification: 'answer' }, { contentClassification: undefined }, { truncated: true }, { draftBytes: undefined }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...draft, ...extra }).success).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...block, draftSegments: draft.draftSegments, draftBytes: draft.draftBytes }).success).toBe(false)
    const retired = { ...draft, state: 'unknown', classification: 'withheld', draftSegments: undefined, draftBytes: undefined }
    expect(processDisplayBlockSchemaV1.safeParse(retired).success).toBe(true)
    expect(processDisplayBlockSchemaV1.safeParse({ ...retired, draftSegments: draft.draftSegments, draftBytes: draft.draftBytes }).success).toBe(false)
  })
  it('assembles full UTF-8 revisions beyond one segment and rejects gaps, overlap and duplicate draft identities', () => {
    const first = '中😀'.repeat(4681), second = '完整末尾 '
    const firstBytes = new TextEncoder().encode(first).length
    const draft = { blockId: 'draft-a', kind: 'assistant_draft', state: 'running', attempt: 1, durationMs: 0,
      source: { kind: 'assistant_draft', unitId: unit.unitId, producerId: unit.producerId, providerSequence: unit.providerSequence, attempt: 1 },
      contentClassification: 'provider_text', classification: 'credential_redacted', truncated: false,
      draftSegments: [{ offsetBytes: '0', text: first }, { offsetBytes: String(firstBytes), text: second }],
      draftBytes: String(firstBytes + new TextEncoder().encode(second).length) }
    expect(Number(draft.draftBytes)).toBeGreaterThan(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1)
    expect(processDisplayBlockSchemaV1.safeParse(draft).success).toBe(true)
    for (const extra of [{ draftBytes: '1' }, { draftSegments: [{ offsetBytes: '1', text: first }] },
      { draftSegments: [draft.draftSegments[0], { ...draft.draftSegments[1], offsetBytes: '0' }] },
      { draftSegments: [{ offsetBytes: '0', text: 'x'.repeat(32769) }] },
      { draftSegments: [{ offsetBytes: '0', text: '\ud800' }] }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...draft, ...extra }).success).toBe(false)
    }
    const value = { status: 'snapshot', schemaVersion: 2, scope, revision: '2', units: [unit], blocks: [draft] }
    expect(processDisplaySnapshotSchemaV1.safeParse(value).success).toBe(true)
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, blocks: [draft, { ...draft, blockId: 'duplicate-source' }] }).success).toBe(false)
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, blocks: [draft, { ...draft, blockId: 'attempt-2', attempt: 2, source: { ...draft.source, attempt: 2 } }] }).success).toBe(false)
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, units: [{ ...unit, producerId: 'wrong' }] }).success).toBe(false)
    expect(processPaneReadRequestSchemaV1.safeParse({ viewerId, draftsOnly: true }).success).toBe(true)
    expect(processDisplayReadRequestSchemaV1.safeParse({ schemaVersion: 2, action: 'read', viewerToken: viewerId, expandedBlockIds: [], waitMs: 0, draftsOnly: true }).success).toBe(false)
  })

})


describe('active protected Web input', () => {
  const inputSource = { ...source, toolResultItemId: undefined }
  const input = { blockId: 'web-input', source: inputSource, kind: 'web_fetch', state: 'running', attempt: 0,
    durationMs: 1, truncated: false, classification: 'source_exact_local', requestUrl: 'https://example.invalid/path?case=alice', inputDigest: 'c'.repeat(64) }
  it('admits the actual running URL while preserving all existing completion guards', () => {
    expect(processDisplayBlockSchemaV1.parse(input).requestUrl).toBe(input.requestUrl)
    for (const extra of [{ body }, { title: '' }, { preview: '' }, { path: '' }, { inputUtf16CodeUnits: 0 },
      { captureDigest: 'a'.repeat(64) }, { kind: 'shell' }, { state: 'completed' }, { classification: 'credential_redacted' },
      { source }, { source: { ...inputSource, startedEventSeq: undefined } }, { requestUrl: undefined }, { inputDigest: undefined }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...input, ...extra }).success).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...input, state: 'unknown', classification: 'withheld', requestUrl: undefined, inputDigest: undefined }).success).toBe(true)
  })
  it('preserves the whole URL budget and refuses invalid or prefix-truncated target sources', () => {
    const prefix = 'https://example.invalid/'
    const exact = prefix + 'x'.repeat(PROCESS_LOCAL_DISPLAY_MAX_WEB_URL_BYTES_V1 - prefix.length)
    expect(processDisplayBlockSchemaV1.safeParse({ ...input, requestUrl: exact }).success).toBe(true)
    expect(processDisplayBlockSchemaV1.parse({ ...input, requestUrl: 'https://example.invalid/a b' }).requestUrl).toBe('https://example.invalid/a b')
    for (const requestUrl of [exact + 'x', 'javascript:alert(1)', 'file:///tmp/source', 'https://user:password@example.invalid/', ' https://example.invalid', 'https://example.invalid/\n']) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...input, requestUrl }).success).toBe(false)
    }
  })
  it('keeps sealed URL resolution in Main-only wire and takes no caller-provided URL', () => {
    const request = { viewerId, blockId: input.blockId, inputDigest: input.inputDigest, role: 'request' }
    expect(processPaneWebLinkRequestSchemaV1.safeParse(request).success).toBe(true)
    for (const extra of [{ targetUrl: input.requestUrl }, { requestUrl: input.requestUrl }, { role: 'result' }, { captureDigest: input.inputDigest }]) {
      expect(processPaneWebLinkRequestSchemaV1.safeParse({ ...request, ...extra }).success).toBe(false)
    }
    const wire = { schemaVersion: 2, action: 'open_web_link', viewerToken: viewerId, blockId: input.blockId, inputDigest: input.inputDigest, role: 'request' }
    expect(processDisplayWebLinkRequestSchemaV1.safeParse(wire).success).toBe(true)
    const sealed = { status: 'web_link', schemaVersion: 2, revision: '1', scope, blockId: input.blockId, inputDigest: input.inputDigest, role: 'request', targetUrl: input.requestUrl }
    expect(processDisplayWebLinkResultSchemaV1.safeParse(sealed).success).toBe(true)
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...sealed, status: 'snapshot', units: [unit], blocks: [block] }).success).toBe(false)
  })
})


describe('attributed live protected process values', () => {
  const pendingUnit = { ...unit, placement: 'pending', calls: [], precedingCalls: [] }
  const nativeSource = { kind: 'native', unitId: unit.unitId, producerId: unit.producerId,
    providerSequence: unit.providerSequence, attempt: 1, nativeIndex: 3, sourceOrdinal: '7' }
  const providerSource = { kind: 'provider_reasoning', unitId: unit.unitId, producerId: unit.producerId,
    providerSequence: unit.providerSequence, attempt: 1, sourceOrdinal: '8' }
  const reasoning = { blockId: 'live-think', kind: 'native_reasoning', source: nativeSource,
    state: 'running', attempt: 1, nativeIndex: 3, title: 'Think', preview: 'Complete paragraph first line',
    durationMs: 3, truncated: false, classification: 'credential_redacted' }
  const shellSource = { ...source, toolResultItemId: undefined }
  const shell = { blockId: 'live-shell', kind: 'shell', source: shellSource, state: 'running', attempt: 0,
    title: 'printf synthetic', durationMs: 3, truncated: false, classification: 'source_exact_local' }
  const preparation = { blockId: 'preparing-write', kind: 'tool_preparation', state: 'preparing', attempt: 1,
    source: { kind: 'tool_preparation', unitId: unit.unitId, producerId: unit.producerId,
      providerSequence: unit.providerSequence, attempt: 1, callId: 'preparing-call', sourceOrdinal: '2' },
    toolName: 'write', path: '/synthetic/partial.ts', preview: 'partial description', inputUtf16CodeUnits: 3,
    durationMs: 0, truncated: false, classification: 'source_exact_local' }
  const snapshotOf = (value: unknown, selectedUnit: unknown = pendingUnit) => ({ status: 'snapshot', schemaVersion: 2,
    revision: '2', scope, units: [selectedUnit], blocks: [value] })

  it('admits bounded native and provider reasoning summaries and expanded running text without settled facts', () => {
    for (const value of [reasoning, { ...reasoning, source: providerSource, nativeIndex: undefined }]) {
      expect(processDisplaySnapshotSchemaV1.parse(snapshotOf(value)).blocks[0]).not.toHaveProperty('body')
      expect(processDisplayBlockSchemaV1.parse({ ...value, body }).body?.text).toBe(body.text)
      for (const extra of [{ captureDigest: block.captureDigest }, { exitCode: 0 }, { path: '/synthetic/source' },
        { effectApplied: true }, { settlementStatus: 'completed' }, { attempt: 2 },
        { classification: 'withheld' }, { state: 'unknown' }, { body: { ...body, text: 'x'.repeat(32769) } }]) {
        expect(processDisplayBlockSchemaV1.safeParse({ ...value, body, ...extra }).success).toBe(false)
      }
      const retired = { ...value, state: 'superseded', classification: 'withheld', title: undefined, preview: undefined }
      expect(processDisplayBlockSchemaV1.safeParse(retired).success).toBe(true)
      expect(processDisplayBlockSchemaV1.safeParse({ ...retired, preview: '' }).success).toBe(false)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...reasoning, source: providerSource }).success).toBe(false)
    expect(processDisplayBlockSchemaV1.safeParse({ ...reasoning, source: { ...providerSource, nativeIndex: 3 }, nativeIndex: undefined }).success).toBe(false)
    expect(processDisplaySnapshotSchemaV1.safeParse(snapshotOf({ ...reasoning, source: { ...nativeSource, producerId: 'forged-producer' } })).success).toBe(false)
  })

  it('admits only the actual started shell command until a result seals its output', () => {
    const activeUnit = { ...unit, calls: [{ ...unit.calls[0], toolResultItemId: undefined }] }
    expect(processDisplaySnapshotSchemaV1.safeParse(snapshotOf(shell, activeUnit)).success).toBe(true)
    expect(processDisplayBlockSchemaV1.parse(shell).title).toBe(shell.title)
    for (const extra of [{ source }, { source: { ...shellSource, startedEventSeq: undefined } },
      { body }, { preview: 'unsettled output' }, { captureDigest: block.captureDigest }, { exitCode: 0 },
      { attempt: 1 }, { path: '/synthetic/source' }, { classification: 'withheld' }, { state: 'completed' }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...shell, ...extra }).success).toBe(false)
    }
    expect(processDisplaySnapshotSchemaV1.safeParse(snapshotOf(shell, unit)).success).toBe(false)
    expect(processDisplayBlockSchemaV1.parse(block).captureDigest).toBe(block.captureDigest)
  })

  it('admits only named attributed preparation summaries without dispatch anchors or expandable payload', () => {
    expect(processDisplaySnapshotSchemaV1.parse(snapshotOf(preparation)).blocks[0]).toEqual(preparation)
    for (const toolName of ['write', 'write_file', 'edit', 'edit_file', 'multi_edit']) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...preparation, toolName }).success).toBe(true)
    }
    expect(processDisplayBlockSchemaV1.safeParse({ ...preparation, toolName: 'bash', inputUtf16CodeUnits: undefined }).success).toBe(true)
    for (const extra of [{ toolName: undefined }, { toolName: '' }, { toolName: 'bash' }, { attempt: 2 },
      { title: 'synthetic' }, { body }, { captureDigest: block.captureDigest }, { exitCode: 0 },
      { source: { ...preparation.source, toolCallItemId: 'forged-item' } },
      { source: { ...preparation.source, startedEventSeq: '1' } }, { source: { ...preparation.source, toolResultItemId: 'forged-result' } },
      { grant: 'forged' }, { sealed: true }, { state: 'running' }, { state: 'completed' }, { truncated: true },
      { inputUtf16CodeUnits: -1 }, { inputUtf16CodeUnits: 0.5 }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...preparation, ...extra }).success).toBe(false)
    }
    for (const source of [{ ...preparation.source, producerId: 'wrong' }, { ...preparation.source, providerSequence: '2' }]) {
      expect(processDisplaySnapshotSchemaV1.safeParse(snapshotOf({ ...preparation, source })).success).toBe(false)
    }
    const retired = { ...preparation, state: 'unknown', classification: 'withheld', toolName: undefined,
      path: undefined, preview: undefined, inputUtf16CodeUnits: undefined }
    expect(processDisplayBlockSchemaV1.safeParse(retired).success).toBe(true)
    for (const extra of [{ toolName: 'write' }, { path: '' }, { preview: '' }, { inputUtf16CodeUnits: 0 }]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...retired, ...extra }).success).toBe(false)
    }
  })

  it('keeps actual mixed origin ordinals independent of response array order and rejects ambiguous origins', () => {
    const earlierPreparation = { ...preparation, source: { ...preparation.source, sourceOrdinal: '1' } }
    const laterReasoning = { ...reasoning, source: { ...nativeSource, sourceOrdinal: '2' } }
    const snapshot = { ...snapshotOf(earlierPreparation), blocks: [laterReasoning, earlierPreparation] }
    const parsed = processDisplaySnapshotSchemaV1.parse(snapshot)
    expect(parsed.blocks.map(value => value.source)).toEqual([laterReasoning.source, earlierPreparation.source])
    expect(processDisplaySnapshotSchemaV1.parse({ ...snapshot, blocks: [...snapshot.blocks].reverse() }).blocks.map(value => value.source))
      .toEqual([earlierPreparation.source, laterReasoning.source])
    for (const sourceOrdinal of [undefined, '0', '-1', '01', '18446744073709551616', 1]) {
      expect(processDisplayBlockSchemaV1.safeParse({ ...preparation, source: { ...preparation.source, sourceOrdinal } }).success).toBe(false)
    }
    const conflicting = { ...laterReasoning, source: { ...laterReasoning.source, sourceOrdinal: '1' } }
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...snapshot, blocks: [conflicting, earlierPreparation] }).success).toBe(false)
  })

  it('rejects duplicate preparation seats and live values from older attempts', () => {
    const value = snapshotOf(preparation)
    const blocks = [preparation, { ...preparation, blockId: 'other-seat' }]
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, blocks }).success).toBe(false)
    const newer = { ...preparation, blockId: 'new-attempt', attempt: 2,
      source: { ...preparation.source, attempt: 2, sourceOrdinal: '3' }, state: 'superseded', classification: 'withheld',
      toolName: undefined, path: undefined, preview: undefined, inputUtf16CodeUnits: undefined }
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, blocks: [preparation, newer] }).success).toBe(false)
    const sibling = { ...preparation, blockId: 'sibling-seat', source: { ...preparation.source, callId: 'other-call', sourceOrdinal: '3' } }
    expect(processDisplaySnapshotSchemaV1.safeParse({ ...value, blocks: [preparation, sibling] }).success).toBe(true)
  })
})
