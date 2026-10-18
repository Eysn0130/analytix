// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { ToolBlock, ThreadEventSink } from '../../../../../agent/types'
import { dispatchAnalytixRuntimeEvent } from '../../../../../agent/analytix-mapper'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { ToolCallTree } from '../ToolCallTree'
import { presentationPolicyFor } from '../chat-view-model'
import { FileResultRow } from './file-result-row'
import i18n from '../../../../../i18n'

const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, snapshotDigest: 'c'.repeat(64), resultItemId: selector.resultItemId, toolName: 'ls',
  inspect: { inputJSON: '{"path":"ORIGINAL_INPUT"}', inputTruncated: false, schemaTruncated: false, callEventAt: null, resultEventAt: null, durationMs: null },
  capture: { kind: 'file_result', status: 'completed', body: '', label: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false,
    fileResult: { operation: 'ls', classification: 'source_exact_local', list: { path: '/synthetic/ORIGINAL_ROOT', relativePath: 'ORIGINAL_ROOT',
      entries: [{ name: 'REAL_CHILD', displayName: 'REAL_CHILD/', kind: 'directory' }], names: ['REAL_CHILD/'], limit: 10, producerTruncated: true, originalCount: 1 } } } }
const block: ToolBlock = { kind: 'tool', id: selector.resultItemId, toolKind: 'tool_call', status: 'success', summary: 'PUBLIC_STATUS',
  meta: { toolName: 'ls', ...selector, localResult: selector } }
let root: Root, container: HTMLDivElement, invalidated: ((id: string | null) => void) | undefined
const api = { openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<{ ok: true; display: ToolResultLocalDisplayV1 }> => ({ ok: true, display })),
  closeToolResultLocalDisplay: vi.fn(async () => undefined), copyToolResultLocalDisplay: vi.fn(), saveToolResultLocalDisplay: vi.fn(),
  onRuntimeStatus: vi.fn(() => () => undefined), onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { invalidated = listener; return () => { invalidated = undefined } }) }
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); vi.clearAllMocks()
  api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); document.getSelection()?.removeAllRanges(); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
async function draw(value = block, visible = true) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(FileResultRow,
    { block: value, visible, processing: false, policy: presentationPolicyFor('detailed'), resetKey: 'result', privateGeneration: 0 }))))
}
async function expand() {
  const row = container.querySelector<HTMLElement>('[data-file-result-row] [data-disclosure-row]')!
  expect(row).not.toBeNull(); if (row.getAttribute('aria-expanded') !== 'true') await act(async () => row.click())
}
it('preserves an already admitted running input seat without requesting a settled snapshot', async () => {
  const setOpen = vi.fn()
  await act(async () => root.render(createElement(FileResultRow, { block: { ...block, status: 'running' }, visible: true, processing: true,
    policy: presentationPolicyFor('detailed'), resetKey: 'running', privateGeneration: 0,
    capture: { blockId: 'running-input', preview: null, body: createElement('span', null, 'ADMITTED_RUNNING_INPUT'),
      expanded: true, expandable: true, setOpen } })))
  expect(container.textContent).toContain('ADMITTED_RUNNING_INPUT')
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLElement>('[data-disclosure-row]')!.click())
  expect(setOpen).toHaveBeenCalledWith(false)
})
it('routes the real settled mapper identity into the existing protected leaf and shows the actual result', async () => {
  const onTool = vi.fn()
  await dispatchAnalytixRuntimeEvent({ kind: 'tool_call_finished', threadId: 'thread', turnId: 'turn', item: {
    id: selector.resultItemId, threadId: 'thread', turnId: 'turn', role: 'tool', createdAt: '2026-10-14T00:00:00Z', kind: 'tool_result',
    callId: selector.callId, toolName: 'ls', toolKind: 'tool_call', status: 'completed', isError: false,
    output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed', messageKey: 'tool_completed',
      code: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
  } }, { onTool } as unknown as ThreadEventSink, async () => undefined)
  const event = onTool.mock.calls[0][0]
  expect(event.toolKind).toBe('tool_call')
  const mapped: ToolBlock = { kind: 'tool', id: event.itemId, summary: event.summary, status: event.status, toolKind: event.toolKind, meta: event.meta }
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(ToolCallTree,
    { blocks: [mapped], processing: false, policy: presentationPolicyFor('detailed') }))))
  const group = container.querySelector<HTMLButtonElement>('[data-process-group] > div:first-child > button')!
  expect(group).not.toBeNull(); expect(group.disabled).toBe(false)
  expect(group.closest('[hidden]')).toBeNull(); expect(group.getAttribute('aria-expanded')).toBe('false')
  expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => group.click())
  expect(group.getAttribute('aria-expanded')).toBe('true')
  expect(container.querySelector('[data-file-result-row="ls"]')).not.toBeNull()
  expect(container.querySelector('[data-file-result-row="ls"] [data-disclosure-row]')?.getAttribute('aria-expanded')).toBe('false')
  expect(container.textContent).not.toContain('REAL_CHILD/')
  expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String) })
  await expand()
  expect(container.querySelector('[data-file-result-row="ls"] [data-disclosure-row]')?.getAttribute('aria-expanded')).toBe('true')
  expect(container.textContent).toContain('REAL_CHILD/')
  expect(container.textContent).toContain('The tool returned a partial result.')
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledOnce()
  expect(api.openToolResultLocalDisplay).toHaveBeenCalledWith({ ...selector, threadId: 'thread', viewId: expect.any(String) })
  await act(async () => invalidated?.(null))
  expect(container.textContent).not.toContain('REAL_CHILD/')
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
})
it('keeps the typed result read-only and retires it on hidden route and Host invalidation', async () => {
  await draw(); await expand()
  const result = container.querySelector('[data-protected-tool-body] [data-protected-tool-readonly]')!
  expect(result).not.toBeNull()
  const selection = document.getSelection()!, range = document.createRange(); range.selectNodeContents(result); selection.removeAllRanges(); selection.addRange(range)
  const copy = new Event('copy', { bubbles: true, cancelable: true })
  await act(async () => result.dispatchEvent(copy))
  expect(copy.defaultPrevented).toBe(true)
  expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled(); expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
  await act(async () => invalidated?.(null))
  expect(container.textContent).not.toContain('REAL_CHILD/')
  expect(container.textContent).not.toContain('ORIGINAL_ROOT')
  await draw(block, false)
  expect(container.querySelector('[data-protected-tool-body]')).toBeNull()
  expect(api.closeToolResultLocalDisplay).toHaveBeenCalled()
})
it.each([
  { ...display, toolName: 'find' }, { ...display, resultItemId: 'item_result_' + 'd'.repeat(64) }
])('rejects a different tool/result identity and host tool kind instead of showing a success payload', async foreign => {
  api.openToolResultLocalDisplay.mockResolvedValue({ ok: true, display: foreign })
  await draw(); await expand()
  expect(container.textContent).not.toContain('REAL_CHILD/')
  await draw({ ...block, toolKind: 'file_change' })
  expect(container.querySelector('[data-file-result-row]')).toBeNull()
})
