// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { ToolResultTurnItem } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ToolResultLocalDisplayV1, ToolResultLocalViewResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { PresentRow } from './present-row'
import { presentationPolicyFor } from '../chat-view-model'
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const host = { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: 'completed',
  code: 'tool_completed', messageKey: 'tool_completed', privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false }
const item: CoreTurnItemJson = { threadId: 'thread', turnId: 'turn', callId: selector.callId, id: selector.resultItemId,
  kind: 'tool_result', role: 'tool', status: 'completed', toolName: 'present', toolKind: 'tool_call',
  createdAt: '2026-10-11T00:00:00Z', isError: false, output: host }
const display: ToolResultLocalDisplayV1 = { schemaVersion: 1, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName: 'present',
  inspect: { inputJSON: '{"files":[{"path":"PRIVATE_DECLARED.md"}]}', inputTruncated: false, schemaTruncated: true,
    callEventAt: null, resultEventAt: null, durationMs: null },
  capture: { kind: 'present', status: 'completed', label: '', body: '', truncated: false, durationMs: 0, startLine: 0, endLine: 0, totalLines: 0,
    present: { files: [{ index: 0, path: 'PRIVATE_DECLARED.md', description: 'Actual declaration' }] } } }
let container: HTMLDivElement, root: Root, invalidate: ((id: string | null) => void) | undefined
const open = vi.fn(async (_request: unknown): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display }))
const close = vi.fn(async () => undefined)
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('en'); open.mockReset(); close.mockReset()
  open.mockResolvedValue({ ok: true, display })
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { openToolResultLocalDisplay: open, closeToolResultLocalDisplay: close,
    onToolResultLocalDisplayInvalidated: (listener: typeof invalidate) => { invalidate = listener; return () => { invalidate = undefined } },
    onRuntimeStatus: () => () => undefined } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => { await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals() })
async function draw(block: ToolBlock, visible = true, onInspect = vi.fn()) {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'thread' }, createElement(PresentRow,
    { block, visible, processing: block.status === 'running', resetKey: 'present', privateGeneration: 0, policy: presentationPolicyFor('detailed'), onInspect }))))
}
function actualBlock() {
  expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item)
  if (block?.kind !== 'tool') throw new Error('fixture requires the actual Present result mapper')
  return block
}
it('preloads the exact successful receipt and puts original Inspect inside the expanded row', async () => {
  const inspect = vi.fn(), block = actualBlock(); await draw(block, true, inspect)
  expect(open).toHaveBeenCalledOnce(); expect(open.mock.calls[0][0]).toMatchObject({ threadId: 'thread', ...selector })
  const row = container.querySelector<HTMLElement>('[data-present-row] [data-disclosure-row]')!
  expect(row.textContent).toContain('PRIVATE_DECLARED.md')
  expect(container.querySelector('[data-present-row] svg')?.getAttribute('width')).toBe('14')
  expect([...container.querySelectorAll('button')].some(button => button.textContent === 'Inspect')).toBe(false)
  await act(async () => row.click())
  expect(container.textContent).toContain('Actual declaration')
  const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
  await act(async () => button.click())
  expect(inspect).toHaveBeenCalledExactlyOnceWith('turn', selector.callId)
  await act(async () => invalidate?.(null))
  expect(container.textContent).not.toContain('PRIVATE_DECLARED.md')
})
it('does not invent running paths or expand without recorded result/error details', async () => {
  await draw({ ...actualBlock(), status: 'running' })
  expect(open).not.toHaveBeenCalled(); expect(container.querySelector('[aria-expanded]')).toBeNull()
  expect(container.textContent).not.toContain('PRIVATE_DECLARED.md')
})
it('does not dispatch foreign producers or typed kinds', async () => {
  const block = actualBlock()
  await draw({ ...block, toolKind: 'file_change' }); expect(container.querySelector('[data-present-row]')).toBeNull()
  await draw({ ...block, meta: { ...block.meta, toolName: 'read' } }); expect(container.querySelector('[data-present-row]')).toBeNull()
  expect(open).not.toHaveBeenCalled()
})
it('retires a pending hydration when the process row loses visibility', async () => {
  let finish!: (value: ToolResultLocalViewResultV1) => void
  open.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
  const block = actualBlock(); await draw(block); await draw(block, false)
  await act(async () => finish({ ok: true, display }))
  expect(container.textContent).not.toContain('PRIVATE_DECLARED.md'); expect(close).toHaveBeenCalled()
})
