// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import type { ToolBlock } from '../../../agent/types'
import { ProtectedToolResult } from './ProtectedToolResult'
import type { ToolResultLocalDisplayV1 } from '../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import i18n from '../../../i18n'
it('rejects a cross-kind Search response before any protected content can mount', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
  const display = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'glob', capture: {
    kind: 'search', status: 'completed', body: 'PRIVATE_WRONG_KIND', label: 'glob', durationMs: 0, truncated: false, startLine: 0, endLine: 0, totalLines: 0,
    search: { shape: 'paths', paths: ['PRIVATE_WRONG_KIND'], total: 1, truncated: false, toolIsError: false, inputJSON: '{"pattern":"*"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' } } }
  const api = { openToolResultLocalDisplay: vi.fn(async () => ({ ok: true, display })), closeToolResultLocalDisplay: vi.fn(async () => undefined), onRuntimeStatus: vi.fn(() => () => undefined) }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  const block: ToolBlock = { kind: 'tool', id: 'grep', toolKind: 'tool_call', status: 'success', summary: 'grep', meta: { toolName: 'grep', localResult: selector } }
  const container = document.createElement('div'), root = createRoot(container)
  try {
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread', preload: true, renderContent: value => createElement('pre', null, value.capture.body) })))
    expect(container.textContent).not.toContain('PRIVATE_WRONG_KIND')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); vi.unstubAllGlobals() }
})

it.each([
  { name: 'complete HTTP failure', kind: 'web' as const, toolIsError: true, publicStatus: 'success' as const, expected: 'error' },
  { name: 'complete HTTP response with a non-error producer', kind: 'web' as const, toolIsError: false, publicStatus: 'success' as const, expected: 'ok' },
  { name: 'settled Search with public failure', kind: 'search' as const, toolIsError: false, publicStatus: 'error' as const, expected: 'error' },
  { name: 'settled Search with public interruption', kind: 'search' as const, toolIsError: false, publicStatus: 'error' as const, expected: 'stopped', diagnostics: { status: 'canceled' } },
  { name: 'failed private capture', kind: 'search' as const, toolIsError: false, publicStatus: 'success' as const, expected: 'error', captureStatus: 'failed' as const },
  { name: 'timed-out private capture', kind: 'search' as const, toolIsError: false, publicStatus: 'success' as const, expected: 'error', captureStatus: 'timeout' as const },
])('keeps authorized capture state consistent for $name', async ({ kind, toolIsError, publicStatus, expected, ...extra }) => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
  const captureStatus = ('captureStatus' in extra ? extra.captureStatus : undefined) ?? 'completed'
  const common = { durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0 } as const
  const display: ToolResultLocalDisplayV1 = kind === 'web' ? {
    schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'web_fetch',
    capture: { ...common, status: 'completed', kind: 'web_fetch', body: 'HTTP_RESPONSE', label: 'web_fetch', web: {
      requestUrl: 'https://synthetic.invalid/request', requestUrlTruncated: false, finalUrlTruncated: false,
      inputJSON: '{"url":"https://synthetic.invalid/request"}', inputTruncated: false, schemaTruncated: false,
      statusCode: 503, fetchTruncated: false, responseComplete: true, toolIsError, startedAt: '2026-10-08T00:00:00Z'
    } }
  } : {
    schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'glob',
    capture: { ...common, status: captureStatus, kind: 'search', body: 'synthetic.txt', label: 'glob', search: {
      shape: 'paths', paths: ['synthetic.txt'], total: 1, truncated: false, toolIsError: false,
      inputJSON: '{"pattern":"*"}', inputTruncated: false, startedAt: '2026-10-08T00:00:00Z'
    } }
  }
  const api = { openToolResultLocalDisplay: vi.fn(async () => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async () => undefined), onRuntimeStatus: vi.fn(() => () => undefined) }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  const block: ToolBlock = { kind: 'tool', id: 'state-fixture', toolKind: 'tool_call', status: publicStatus, summary: display.toolName,
    meta: { toolName: display.toolName, localResult: selector, ...extra } }
  const container = document.createElement('div'), root = createRoot(container)
  try {
    await act(async () => root.render(createElement(ProtectedToolResult, { block, threadId: 'thread', preload: true,
      renderView: controls => createElement('span', { 'data-capture-state': controls.state ?? 'pending' }) })))
    expect(container.querySelector('[data-capture-state]')?.getAttribute('data-capture-state')).toBe(expected)
    expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(1)
  } finally { await act(async () => root.unmount()); vi.unstubAllGlobals() }
})
