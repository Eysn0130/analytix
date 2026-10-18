import { describe, expect, it } from 'vitest'
import type { ToolBlock } from '../../../../../agent/types'
import { toolResultLocalSearchTextV1, type ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { searchCardInput, searchCardModel } from './search-card-model'
const search = { shape: 'matches' as const, files: [{ path: '/synthetic/source.go', matches: Array.from({ length: 12 }, (_, i) => ({ lineNumber: i + 1, line: 'SOURCE_' + i, column: 1, contextBefore: [], contextAfter: [] })) }],
  total: null, truncated: true, toolIsError: false, inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'grep', capture: {
  kind: 'search', status: 'completed', body: toolResultLocalSearchTextV1(search), label: 'grep', durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0, search },
  inspect: { inputJSON: '{"pattern":"SOURCE","path":"/synthetic","glob":"*.go"}', inputTruncated: false, schemaTruncated: false,
    callEventAt: '2026-10-08T00:00:00Z', resultEventAt: '2026-10-08T00:00:00.001Z', durationMs: 1 } }
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: display.resultItemId }
const block: ToolBlock = { kind: 'tool', id: display.resultItemId, toolKind: 'tool_call', status: 'success', summary: 'grep', meta: { toolName: 'grep', ...selector, localResult: selector } }

describe('actual protected Search card admission', () => {
  it('uses the new sealed Inspect input while preserving original matches and paths', () => {
    const model = searchCardModel(block, display)
    expect(model?.search).toEqual(search)
    expect(model?.search.inputJSON).toBeUndefined()
    expect(searchCardInput(display)).toEqual({ pattern: 'SOURCE', source: 'inspect' })
    if (display.capture.kind !== 'search') throw new Error('Search fixture required')
    const paths = { shape: 'paths' as const, paths: ['/synthetic/source.go'], total: 1, truncated: false,
      toolIsError: false, inputTruncated: false, startedAt: search.startedAt }
    const glob: ToolResultLocalDisplayV1 = { ...display, toolName: 'glob',
      inspect: { ...display.inspect!, inputJSON: '{"pattern":"*.go","path":"/synthetic"}' },
      capture: { ...display.capture, label: 'glob', body: toolResultLocalSearchTextV1(paths), search: paths } }
    expect(searchCardModel({ ...block, meta: { ...block.meta, toolName: 'glob' } }, glob)?.search).toEqual(paths)
    expect(searchCardInput(glob)).toEqual({ pattern: '*.go', source: 'inspect' })
  })
  it('retains an explicit legacy fallback only when Inspect is absent and rejects duplicated input', () => {
    if (display.capture.kind !== 'search') throw new Error('Search fixture required')
    const withLegacy = { ...display, capture: { ...display.capture, search: { ...search, inputJSON: '{"pattern":"LEGACY"}' } } }
    expect(searchCardInput(withLegacy)).toBeNull()
    expect(searchCardModel(block, withLegacy)).toBeNull()
    const legacy = { ...withLegacy, inspect: undefined }
    expect(searchCardModel(block, legacy)?.search).toEqual(legacy.capture.search)
    expect(searchCardInput(legacy)).toEqual({ pattern: 'LEGACY', source: 'legacy' })
    expect(searchCardModel(block, { ...legacy, capture: { ...legacy.capture, search: { ...search, inputTruncated: true } } })).toBeNull()
  })
  it.each([
    { inputJSON: undefined, inputTruncated: false },
    { inputJSON: undefined, inputTruncated: true },
    { inputJSON: '{"pattern":"SOURCE"}', inputTruncated: true },
    { inputJSON: '{', inputTruncated: false },
    { inputJSON: 'null', inputTruncated: false },
    { inputJSON: '[]', inputTruncated: false },
    { inputJSON: '{}', inputTruncated: false },
    { inputJSON: '{"pattern":" "}', inputTruncated: false },
    { inputJSON: '{"pattern":42}', inputTruncated: false },
    { inputJSON: '{"pattern":"SOURCE","path":" "}', inputTruncated: false },
    { inputJSON: '{"pattern":"SOURCE","glob":42}', inputTruncated: false },
  ])('does not conceal invalid or unavailable current Inspect input with a legacy copy: %j', input => {
    if (display.capture.kind !== 'search') throw new Error('Search fixture required')
    for (const data of [search, { ...search, inputJSON: '{"pattern":"LEGACY"}' }]) {
      const candidate = { ...display, inspect: { ...display.inspect!, ...input },
        capture: { ...display.capture, search: data } }
      expect(searchCardInput(candidate)).toBeNull()
      expect(searchCardModel(block, candidate)).toBeNull()
    }
  })
  it('accepts unknown total without claiming the configured limit is a full total', () => {
    expect(searchCardModel(block, display)?.search.total).toBeNull()
    expect(searchCardModel(block, display)?.recovery).toEqual({ resultItemId: display.resultItemId, snapshotDigest: display.snapshotDigest })
  })
  it('keeps error, malformed, running and PTC child on the generic fallback', () => {
    for (const candidate of [{ ...block, status: 'error' as const }, { ...block, status: 'running' as const },
      { ...block, meta: { ...block.meta, parentCallId: 'parent' } }, { ...block, meta: { ...block.meta, child: {} } },
      { ...block, toolKind: 'command_execution' as const }]) expect(searchCardModel(candidate, display)).toBeNull()
    expect(searchCardModel(block, { ...display, capture: { ...display.capture, body: 'FORGED' } } as never)).toBeNull()
    expect(searchCardModel(block, { ...display, toolName: 'glob' } as never)).toBeNull()
  })
  it('retains legitimate zero results while refusing a clipped first result as false emptiness', () => {
    if (display.capture.kind !== 'search') throw new Error('Search fixture required')
    const capture = { ...display.capture, body: '', search: { ...search, files: [], total: 0, truncated: false } }
    expect(searchCardModel(block, { ...display, capture })?.search.total).toBe(0)
    expect(searchCardModel(block, { ...display, capture: { ...capture, truncated: true } })).toBeNull()
  })
})
