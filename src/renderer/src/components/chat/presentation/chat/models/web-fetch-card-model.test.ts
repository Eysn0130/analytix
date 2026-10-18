import { describe, expect, it } from 'vitest'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolBlock } from '../../../../../agent/types'
import { webFetchCardModel } from './web-fetch-card-model'

const block: ToolBlock = { kind: 'tool', id: 'fetch', toolKind: 'tool_call', status: 'success', summary: 'PUBLIC_SUMMARY',
  meta: { toolName: 'web_fetch' } }
const response = (statusCode = 201): ToolResultLocalDisplayV1 => ({ schemaVersion: 1, snapshotDigest: 'c'.repeat(64),
  resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'web_fetch', canOpenWeb: { request: true, result: true }, capture: {
    kind: 'web_fetch', status: 'completed', body: 'actual extracted text', label: 'web_fetch', durationMs: 23,
    truncated: false, startLine: 0, endLine: 0, totalLines: 0, web: {
      requestUrl: 'https://request.invalid/path', finalUrl: 'https://result.invalid/final', inputJSON: '{"url":"https://request.invalid/path"}',
      startedAt: '2026-10-08T00:00:00Z', statusCode, fetchTruncated: false, responseComplete: true, toolIsError: statusCode >= 400,
      inputTruncated: false, schemaTruncated: false, requestUrlTruncated: false, finalUrlTruncated: false
    }
  } })

describe('authorized Web fetch card model', () => {
  it.each([201, 204, 404, 503])('retains observed HTTP %s and final URL including complete non-2xx responses', statusCode => {
    expect(webFetchCardModel({ ...block, status: statusCode >= 400 ? 'error' : 'success' }, response(statusCode)))
      .toEqual({ kind: 'fetch', url: 'https://result.invalid/final', statusCode, truncated: false })
  })
  it('keeps producer truncation separate from the private capture cap', () => {
    const display = response()
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    display.capture.truncated = true
    expect(webFetchCardModel(block, display)?.truncated).toBe(false)
    display.capture.web.fetchTruncated = true
    expect(webFetchCardModel(block, display)?.truncated).toBe(true)
  })
  it('cannot reconstruct a card from incomplete reads, a missing final URL/status or public result metadata', () => {
    const display = response()
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    expect(webFetchCardModel(block, { ...display, capture: { ...display.capture,
      status: 'failed', web: { ...display.capture.web, responseComplete: false, fetchTruncated: undefined } } })).toBeNull()
    expect(webFetchCardModel(block, { ...display, capture: { ...display.capture,
      web: { ...display.capture.web, finalUrl: undefined, finalUrlTruncated: true } } })).toBeNull()
    expect(webFetchCardModel(block, { ...display, capture: { ...display.capture,
      web: { ...display.capture.web, statusCode: undefined } } })).toBeNull()
    const publicOnly: ToolBlock = { ...block, meta: { ...block.meta, url: 'https://public.invalid', publicResult: {
      schemaVersion: 1, projectionKind: 'web_fetch_status', disclosure: 'metadata_only', status: 'completed',
      messageKey: 'tool_completed', code: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
      web: { outcome: 'success', statusCode: 200, byteCount: 0, truncated: false } } } }
    expect(webFetchCardModel(publicOnly, {} as never)).toBeNull()
  })
  it.each(['requestUrl', 'finalUrl', 'inputJSON', 'schemaJSON'])('rejects malformed private %s rather than admitting arbitrary payload', key => {
    const display = response()
    if (display.capture.kind !== 'web_fetch') throw new Error('fixture requires Web')
    expect(webFetchCardModel(block, { ...display, capture: { ...display.capture, web: { ...display.capture.web, [key]: {} } } } as never)).toBeNull()
  })
  it('requires exact name/kind, settled root and a strict Web snapshot', () => {
    for (const name of ['WEB_FETCH', 'web_search', 'fetch_url', 'browse']) expect(webFetchCardModel({ ...block, meta: { toolName: name } }, response())).toBeNull()
    for (const toolKind of ['command_execution', 'file_change', 'subagent', undefined] as const) expect(webFetchCardModel({ ...block, toolKind }, response())).toBeNull()
    expect(webFetchCardModel({ ...block, status: 'running' }, response())).toBeNull()
    expect(webFetchCardModel({ ...block, meta: { ...block.meta, parentCallId: 'parent' } }, response())).toBeNull()
    expect(webFetchCardModel({ ...block, meta: { ...block.meta, child: { childId: 'child' } } }, response())).toBeNull()
    expect(webFetchCardModel(block, { ...response(), extra: 'PRIVATE_SENTINEL' } as never)).toBeNull()
  })
})
