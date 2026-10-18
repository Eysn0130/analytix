// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { ToolBlock, ThreadEventSink } from '../../../../../agent/types'
import { dispatchAnalytixRuntimeEvent } from '../../../../../agent/analytix-mapper'
import { toolResultLocalSearchTextV1, type ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { SearchRow } from './search-row'
import { ToolCallTree } from '../ToolCallTree'
import { ToolView, type ToolViewProps } from '../ToolViews'
import { presentationPolicyFor } from '../chat-view-model'
import i18n from '../../../../../i18n'
const search = { shape: 'matches' as const, files: [{ path: '/synthetic/source.go', matches: Array.from({ length: 12 }, (_, i) => ({ lineNumber: i + 1, line: 'SOURCE_' + i, column: 1, contextBefore: [], contextAfter: [] })) }],
  total: null, truncated: true, toolIsError: false, inputTruncated: false, startedAt: '2026-10-08T00:00:00Z' }
const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64), toolName: 'grep', capture: {
  kind: 'search', status: 'completed', body: toolResultLocalSearchTextV1(search), label: 'grep', durationMs: 1, truncated: false, startLine: 0, endLine: 0, totalLines: 0, search },
  inspect: { inputJSON: '{"pattern":"SOURCE","path":"/synthetic","glob":"*.go"}', inputTruncated: false, schemaTruncated: false,
    callEventAt: '2026-10-08T00:00:00Z', resultEventAt: '2026-10-08T00:00:00.001Z', durationMs: 1 } }
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: display.resultItemId }
const renderFallback = (props: ToolViewProps) => createElement(ToolView, { ...props, family: 'search' })
const block: ToolBlock = { kind: 'tool', id: display.resultItemId, toolKind: 'tool_call', status: 'success', summary: 'grep', meta: { toolName: 'grep', ...selector, localResult: selector } }

let root: Root, container: HTMLDivElement, invalidated: ((id: string | null) => void) | undefined
const api = { openToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true, display })), closeToolResultLocalDisplay: vi.fn(async () => undefined),
  copyToolResultLocalDisplay: vi.fn(async (_request: unknown) => ({ ok: true })), onRuntimeStatus: vi.fn(() => () => undefined),
  onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidated = listener; return () => { invalidated = undefined } }) }
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); vi.clearAllMocks(); Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } });
  container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
async function expand() { const row = container.querySelector('[data-tool-view="search"] [data-disclosure-row]') as HTMLElement; expect(row).not.toBeNull(); if (row.getAttribute('aria-expanded') !== 'true') await act(async () => row.click()) }
it('consumes the actual settled runtime mapper and registered Search dispatch after public integration', async () => {
  const onTool = vi.fn()
  await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_finished', threadId: 'thread', turnId: 'turn', item: {
    id: selector.resultItemId, turnId: 'turn', threadId: 'thread', role: 'tool', createdAt: '2026-10-08T00:00:00Z', kind: 'tool_result', callId: selector.callId,
    toolName: 'grep', toolKind: 'tool_call', status: 'completed', isError: false,
    output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed', messageKey: 'tool_completed', code: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
  } }, { onTool } as unknown as ThreadEventSink, async () => undefined)
  const event = onTool.mock.calls[0][0]
  const mapped: ToolBlock = { kind: 'tool', id: event.itemId, summary: event.summary, status: event.status, toolKind: 'tool_call', meta: event.meta }
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(ToolCallTree, { blocks: [mapped], processing: false, policy: presentationPolicyFor('detailed') }))))
  const group = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button')!
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => group.click())
  await expand()
  expect(container.querySelector('[data-search="matches"]')).not.toBeNull()
  expect(container.querySelector('[data-protected-tool-summary]')?.textContent).toBe('SOURCE')
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String) })
})
it('copies sealed output by identity, retains generic child fallback, and clears on Host invalidation', async () => {
  const draw = async (value: ToolBlock) => act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block: value, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'search', privateGeneration: 0
  }))))
  await draw(block); await expand()
  const copy = container.querySelector('[data-search] button:not([aria-expanded])') as HTMLButtonElement
  await act(async () => copy.click())
  expect(api.copyToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String), snapshotDigest: display.snapshotDigest })
  expect(api.copyToolResultLocalDisplay.mock.calls[0][0]).not.toHaveProperty('body')
  await draw({ ...block, meta: { ...block.meta, parentCallId: 'parent' } }); await expand()
  expect(container.querySelector('[data-search]')).toBeNull()
  expect(container.querySelector('[data-tool-io]')?.textContent).toContain('SOURCE_11')
  await act(async () => invalidated?.(null))
  expect(container.textContent).not.toContain('SOURCE_11')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
})

it('uses the actual failure line in the protected collapsed summary', async () => {
  if (display.capture.kind !== 'search') throw new Error('Search fixture required')
  const failed = { ...display, inspect: { ...display.inspect!, inputJSON: '{"pattern":"["}' },
    capture: { ...display.capture, status: 'failed' as const, body: 'ACTUAL_REGEX_ERROR\nmore detail',
    search: { ...search, files: [], total: null, toolIsError: true } } }
  api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: failed }))
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block: { ...block, status: 'error' }, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'failed', privateGeneration: 0
  }))))
  const summary = container.querySelector('[data-protected-tool-summary]')
  expect(summary?.textContent).toBe('ACTUAL_REGEX_ERROR')
  expect(summary?.getAttribute('data-protected-tool-copy-section')).toBe('result')
})


it('opens actual Search Inspect tabs and retires the lease while hidden before fresh recovery', async () => {
  const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'inspect', privateGeneration: 0
  }))))
  await expand()
  const inspect = () => Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(button => button.textContent === 'Inspect')!
  await act(async () => inspect().click())
  const pane = container.querySelector('[data-tool-result-inspector]')!
  expect(pane).not.toBeNull()
  const tab = (name: string) => Array.from(pane.querySelectorAll<HTMLButtonElement>('[role="tab"]')).find(button => button.textContent === name)!
  await act(async () => tab('Result').click())
  expect(pane.querySelector('[role="tabpanel"]')?.textContent).toContain('SOURCE_11')
  await act(async () => tab('Payload').click())
  expect(pane.querySelector('[role="tabpanel"]')?.textContent).toContain('"pattern"')
  expect(pane.querySelector('[role="tabpanel"]')?.textContent).toContain('"SOURCE"')
  expect(pane.querySelector('[role="tabpanel"]')?.hasAttribute('data-protected-tool-readonly')).toBe(true)
  expect(pane.querySelector('[role="tabpanel"] [data-protected-tool-copy-section]')).toBeNull()
  const firstLease = (api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }).viewId
  await act(async () => { hidden.mockReturnValue(true); document.dispatchEvent(new Event('visibilitychange')) })
  expect(container.querySelector('[data-tool-result-inspector]')).toBeNull()
  expect(container.textContent).not.toContain('SOURCE')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(firstLease)
  await act(async () => { hidden.mockReturnValue(false); document.dispatchEvent(new Event('visibilitychange')) })
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledTimes(2)
  const secondLease = (api.openToolResultLocalDisplay.mock.calls[1][0] as { viewId: string }).viewId
  expect(secondLease).not.toBe(firstLease)
  await expand()
  await act(async () => inspect().click())
  expect(container.querySelectorAll('[data-tool-result-inspector]')).toHaveLength(1)
  expect(container.querySelector('[data-tool-result-inspector]')?.textContent).toContain(display.snapshotDigest)
})

it.each([
  { captureStatus: 'canceled' as const, rowState: 'stopped' },
  { captureStatus: 'unknown' as const, rowState: 'unknown' },
])('retains the first pattern line for $rowState without coloring interrupted output as an error', async ({ captureStatus, rowState }) => {
  if (display.capture.kind !== 'search') throw new Error('Search fixture required')
  const interrupted = { ...display, inspect: { ...display.inspect!, inputJSON: '{"pattern":"SOURCE\\nSECOND_PATTERN_LINE"}' },
    capture: { ...display.capture, status: captureStatus, body: 'ACTUAL_INTERRUPTION_DETAIL',
    search: { ...search, files: [], total: null, toolIsError: true } } }
  api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: interrupted }))
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block: { ...block, status: 'error' }, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true,
    resetKey: rowState, privateGeneration: 0
  }))))
  const summary = container.querySelector('[data-protected-tool-summary]')
  expect(container.querySelector('[data-state]')?.getAttribute('data-state')).toBe(rowState)
  expect(summary?.textContent).toBe('SOURCE')
  expect(summary?.getAttribute('data-protected-tool-copy-section')).toBeNull()
  expect(summary?.hasAttribute('data-protected-tool-readonly')).toBe(true)
  expect(container.textContent).not.toContain('ACTUAL_INTERRUPTION_DETAIL')
  const icon = container.querySelector('[data-tool-view="search"] svg')
  expect(icon?.getAttribute('viewBox')).toBe('0 0 16 16')
  expect(icon?.getAttribute('width')).toBe('14')
  expect(icon?.getAttribute('stroke-width')).toBe('1')
  await expand()
  expect(container.querySelector('[data-search]')).toBeNull()
  expect(container.querySelector('[data-tool-io]')?.textContent).toBe('OUTACTUAL_INTERRUPTION_DETAIL')
  expect(container.querySelector('[data-tool-io] [data-error]')).toBeNull()
  expect(container.querySelectorAll('[data-tool-io] pre')).toHaveLength(0)
  const inspect = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === 'Inspect')!
  await act(async () => inspect.click())
  const pane = container.querySelector('[data-tool-result-inspector]')!
  const resultTab = [...pane.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(button => button.textContent === 'Result')!
  await act(async () => resultTab.click())
  expect(pane.querySelector('[role="tabpanel"]')?.textContent).toContain('ACTUAL_INTERRUPTION_DETAIL')
  expect(pane.querySelector('[role="tabpanel"] [data-error]')).toBeNull()
})

it('blocks native copy from the current Inspect summary while keeping sealed result copying available', async () => {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'readonly', privateGeneration: 0
  }))))
  const summary = container.querySelector('[data-protected-tool-summary]')!
  expect(summary.textContent).toBe('SOURCE')
  expect(summary.hasAttribute('data-protected-tool-readonly')).toBe(true)
  expect(summary.hasAttribute('data-protected-tool-copy-section')).toBe(false)
  const range = document.createRange(); range.selectNodeContents(summary)
  const selection = document.getSelection()!; selection.removeAllRanges(); selection.addRange(range)
  const copyEvent = new Event('copy', { bubbles: true, cancelable: true })
  await act(async () => summary.dispatchEvent(copyEvent))
  expect(copyEvent.defaultPrevented).toBe(true)
  expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  selection.removeAllRanges()
  await expand()
  const copy = container.querySelector('[data-search] button:not([aria-expanded])') as HTMLButtonElement
  await act(async () => copy.click())
  expect(api.copyToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String), snapshotDigest: display.snapshotDigest })
})

it('keeps the historical Search input and payload copy classification when Inspect is absent', async () => {
  if (display.capture.kind !== 'search') throw new Error('Search fixture required')
  const legacy = { ...display, inspect: undefined, capture: { ...display.capture, search: { ...search, inputJSON: '{"pattern":"LEGACY"}' } } }
  api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: legacy }))
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'legacy', privateGeneration: 0
  }))))
  const summary = container.querySelector('[data-protected-tool-summary]')!
  expect(summary.textContent).toBe('LEGACY')
  expect(summary.getAttribute('data-protected-tool-copy-section')).toBe('payload')
  expect(summary.hasAttribute('data-protected-tool-readonly')).toBe(false)
  await expand()
  expect(container.querySelector('[data-search="matches"]')).not.toBeNull()
})

it.each([
  { inputJSON: undefined, inputTruncated: true },
  { inputJSON: '{}', inputTruncated: false },
])('keeps the actual output fallback when current Inspect input is unavailable or invalid: %j', async input => {
  if (display.capture.kind !== 'search') throw new Error('Search fixture required')
  const unavailable = { ...display, inspect: { ...display.inspect!, ...input } }
  api.openToolResultLocalDisplay.mockImplementationOnce(async () => ({ ok: true, display: unavailable }))
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(SearchRow, {
    block, renderFallback, processing: false, policy: presentationPolicyFor('detailed'), visible: true, resetKey: 'unavailable', privateGeneration: 0
  }))))
  expect(container.querySelector('[data-protected-tool-summary]')).toBeNull()
  await expand()
  expect(container.querySelector('[data-search]')).toBeNull()
  expect(container.querySelector('[data-tool-io]')?.textContent).toContain('SOURCE_11')
})
