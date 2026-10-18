// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToolResultTurnItem } from '../../../../../../../../packages/runtime/src/contracts/items'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1, type ToolResultLocalViewResultV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import { chatBlockFromItem } from '../../../../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../../../../agent/analytix-contract'
import type { ToolBlock } from '../../../../../agent/types'
import i18n from '../../../../../i18n'
import { useChatStore } from '../../../../../store/chat-store'
import { ProtectedToolResultScope } from '../../ProtectedToolResult'
import { presentationPolicyFor } from '../chat-view-model'
import type { ToolViewProps } from '../ToolViews'
import { TodoRow } from './todo-row'

const timestamp = '2026-10-09T00:00:00Z'
const selector = { turnId: 'turn', callId: 'call_host_' + 'a'.repeat(64), resultItemId: 'item_result_' + 'b'.repeat(64) }
const captureBase = { label: '', body: '', durationMs: 0, startLine: 0, endLine: 0, totalLines: 0, truncated: false } as const
const originalInput = JSON.stringify({ todos: [{ content: 'PRIVATE_REQUESTED_TODO', status: 'pending' }],
  rejectedField: '<img src="PRIVATE_IN_ONLY">' })
function blockFor(status: 'completed' | 'failed' | 'canceled' = 'failed'): ToolBlock {
  const publicStatus = status === 'canceled' ? 'cancelled' : status
  const code = status === 'completed' ? 'tool_completed' : status === 'canceled' ? 'tool_cancelled' : 'tool_failed'
  const item: CoreTurnItemJson = { id: selector.resultItemId, threadId: 'thread', turnId: selector.turnId,
    kind: 'tool_result', role: 'tool', status: status === 'canceled' ? 'aborted' : status,
    toolName: 'todo_write', toolKind: 'tool_call', callId: selector.callId, createdAt: timestamp, isError: status !== 'completed',
    output: { schemaVersion: 1, projectionKind: 'host_status', disclosure: 'metadata_only', status: publicStatus, code, messageKey: code,
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false } }
  expect(ToolResultTurnItem.safeParse(item).success).toBe(true)
  const block = chatBlockFromItem(item)
  if (block?.kind !== 'tool') throw new Error('fixture must reach the actual mapper')
  expect(block.meta?.localResult).toEqual(selector)
  return block
}
function failedDisplay(status: 'failed' | 'canceled' | 'timeout' = 'failed'): ToolResultLocalDisplayV1 {
  return { schemaVersion: 1, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName: 'todo_write',
    capture: { ...captureBase, kind: 'inspect', status },
    inspect: { inputJSON: originalInput, inputTruncated: false, schemaTruncated: false,
      callEventAt: null, resultEventAt: null, durationMs: null } }
}
let root: Root, container: HTMLDivElement, display: ToolResultLocalDisplayV1
let invalidate: (id: string | null) => void, retireRuntime: () => void
let previous: ReturnType<typeof useChatStore.getState>, api: ReturnType<typeof makeApi>
function makeApi() {
  const listeners = new Set<(id: string | null) => void>()
  invalidate = id => { for (const listener of [...listeners]) listener(id) }
  return { openToolResultLocalDisplay: vi.fn(async (_request: unknown): Promise<ToolResultLocalViewResultV1> => ({ ok: true, display })),
    closeToolResultLocalDisplay: vi.fn(async (_id: string) => undefined),
    copyToolResultLocalDisplay: vi.fn(async () => ({ ok: true })), saveToolResultLocalDisplay: vi.fn(async () => ({ ok: true })),
    onToolResultLocalDisplayInvalidated: vi.fn((listener: (id: string | null) => void) => { listeners.add(listener); return () => { listeners.delete(listener) } }),
    onRuntimeStatus: vi.fn((listener: (status: { state: string }) => void) => { retireRuntime = () => listener({ state: 'stopped' }); return () => undefined }) }
}
async function draw(block = blockFor(), props: Partial<ToolViewProps> = {}, scope = 'thread') {
  await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: scope },
    createElement(TodoRow, { block, processing: false, policy: presentationPolicyFor('detailed'),
      visible: true, resetKey: 'reset', privateGeneration: 0, ...props }))))
}
const row = () => container.querySelector<HTMLElement>('[data-disclosure-row]')!
const input = () => container.querySelector<HTMLElement>('[data-todo-input]')
const expand = async () => { await act(async () => row().click()) }
const noInventedResult = () => {
  expect(container.querySelector('[data-tool-details],[data-change],[data-analytix-diff-stats]')).toBeNull()
  expect([...container.querySelectorAll('span')].some(node => node.textContent === 'OUT')).toBe(false)
  expect(container.textContent).not.toContain('PRIVATE_CURRENT_STORE')
  expect(container.textContent).not.toMatch(/Copy|Save|completed|added|removed|updated/)
  expect(api.copyToolResultLocalDisplay).not.toHaveBeenCalled()
  expect(api.saveToolResultLocalDisplay).not.toHaveBeenCalled()
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('en')
  previous = useChatStore.getState()
  useChatStore.setState({ workspaceRoot: '', activeThreadTodos: { threadId: 'thread',
    items: [{ id: 'current', content: 'PRIVATE_CURRENT_STORE', status: 'in_progress', createdAt: timestamp, updatedAt: timestamp }], updatedAt: timestamp } })
  display = failedDisplay(); api = makeApi()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); document.getSelection()?.removeAllRanges()
  useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
describe('Todo actual failed/stopped protected input', () => {
  it('reuses the shared sealed error OUT and retires it with the existing Todo lease', async () => {
    const message = '  Alice\nactual Todo store failure\r\n'
    display = { ...failedDisplay(), inspect: { ...failedDisplay().inspect!, outputError: { availability: 'available',
      entries: [{ message, classification: 'source_exact_local' }] } } }
    await draw(); await expand()
    expect(container.querySelector('[data-tool-raw-error]')?.textContent).toBe(message)
    expect(input()?.querySelectorAll('span')[1].textContent).toBe(originalInput)
    expect(container.querySelector('[data-todo-capture]')).toBeNull()
    await act(async () => invalidate(null))
    expect(container.querySelector('[data-tool-raw-error]')).toBeNull()
    expect(container.textContent).not.toContain(message)
  })
  it.each([['failed', 'error', 'Failed'], ['canceled', 'stopped', 'Stopped']] as const)(
    'renders original authorized %s IN without claiming a committed Todo result', async (status, state, label) => {
      display = failedDisplay(status)
      expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
      await draw(blockFor(status))
      expect(container.querySelector('[data-state="' + state + '"]')).not.toBeNull()
      expect(container.querySelector('[data-tool="todo_write"] svg')).not.toBeNull()
      expect(container.textContent).toContain(label)
      expect(container.textContent).not.toContain('PRIVATE_REQUESTED_TODO')
      await expand()
      expect(input()?.closest('[data-protected-tool-body]')).not.toBeNull()
      expect(input()?.hasAttribute('data-protected-tool-readonly')).toBe(true)
      expect(input()?.textContent).toContain('IN')
      expect(input()?.querySelectorAll('span')[1].textContent).toBe(originalInput)
      expect(container.querySelector('img')).toBeNull()
      noInventedResult()
      await expand()
      expect(input()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_REQUESTED_TODO')
      expect(api.closeToolResultLocalDisplay).not.toHaveBeenCalled()
    })
  it('preserves original IN integers beyond JavaScript precision', async () => {
    const original = '{"sequence":9007199254740993}'
    display = { ...display, inspect: { ...display.inspect!, inputJSON: original } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    await draw(); await expand()
    const rendered = input()?.querySelectorAll('span')[1].textContent
    expect(rendered).toContain('9007199254740993')
    expect(rendered).toBe(original)
    noInventedResult()
  })
  it.each([
    ['exponent notation', '{"scale":1.2300e+10,"small":1E-012,"negativeZero":-0}'],
    ['original whitespace', ' \r\n{\t"todos" : [] ,\n "note" : "PRIVATE_WHITESPACE_IN"}\t ']
  ] as const)('preserves %s in authorized IN verbatim', async (_label, original) => {
    display = { ...display, inspect: { ...display.inspect!, inputJSON: original } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    await draw(); await expand()
    expect(input()?.querySelectorAll('span')[1].textContent).toBe(original)
    noInventedResult()
  })
  it('keeps timeout as a failed call with original IN', async () => {
    display = failedDisplay('timeout'); await draw(); await expand()
    expect(container.querySelector('[data-state="error"]')).not.toBeNull()
    expect(input()?.textContent).toContain('PRIVATE_REQUESTED_TODO'); noInventedResult()
  })
  it('reports an actually truncated capture without partial or reconstructed IN', async () => {
    display = { ...display, inspect: { ...display.inspect!, inputJSON: undefined, inputTruncated: true } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    await draw(); await expand()
    expect(input()?.textContent).toContain('IN')
    expect(input()?.textContent).toContain('Local capture is incomplete')
    expect(container.textContent).not.toContain('PRIVATE_REQUESTED_TODO'); noInventedResult()
  })
  it.each(['unavailable', 'canceled', 'missing-input'] as const)('keeps %s access truthful', async reason => {
    if (reason === 'missing-input') {
      display = { ...display, inspect: { ...display.inspect!, inputJSON: undefined, inputTruncated: false } }
      expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(false)
    } else api.openToolResultLocalDisplay.mockResolvedValue({ ok: false, code: reason })
    await draw(); await expand()
    expect(input()).toBeNull(); expect(container.textContent).toContain('This local result is unavailable.')
    noInventedResult()
  })
  it('blocks IN copy/cut/drag and keeps its actual call Inspect reachable', async () => {
    await draw(); await expand()
    const range = document.createRange(); range.selectNodeContents(input()!)
    document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(range)
    for (const type of ['copy', 'cut', 'dragstart']) {
      const event = new Event(type, { bubbles: true, cancelable: true })
      await act(async () => input()!.dispatchEvent(event)); expect(event.defaultPrevented).toBe(true)
    }
    const button = [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === 'Inspect')!
    await act(async () => button.click())
    const inspector = container.querySelector('[data-tool-result-inspector]')
    expect(inspector?.getAttribute('data-call-id')).toBe(selector.callId)
    expect(inspector?.textContent).toContain(selector.resultItemId)
    expect(inspector?.textContent).not.toMatch(/Copy|Save/)
    noInventedResult()
  })
  it.each(['revoke', 'document-hidden', 'row-hidden', 'scope', 'reset', 'generation', 'runtime-stopped'] as const)(
    'retires displayed IN on %s', async reason => {
      const block = blockFor('canceled')
      display = failedDisplay('canceled'); await draw(block); await expand()
      const selected = document.createRange(); selected.selectNodeContents(input()!)
      document.getSelection()!.removeAllRanges(); document.getSelection()!.addRange(selected)
      const viewId = (api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }).viewId
      if (reason === 'revoke') await act(async () => invalidate(viewId))
      else if (reason === 'document-hidden') {
        vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
        await act(async () => document.dispatchEvent(new Event('visibilitychange')))
      } else if (reason === 'runtime-stopped') await act(async () => retireRuntime())
      else await draw(block, reason === 'row-hidden' ? { visible: false } : reason === 'reset' ? { resetKey: 'next' }
        : reason === 'generation' ? { privateGeneration: 1 } : {}, reason === 'scope' ? 'other-thread' : 'thread')
      expect(input()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_REQUESTED_TODO')
      expect(document.getSelection()?.toString()).toBe('')
      expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(viewId)
      noInventedResult()
    })
  it.each(['hidden', 'revoke'] as const)('discards late failed IN after pending %s retirement', async reason => {
    let finish!: (value: ToolResultLocalViewResultV1) => void
    api.openToolResultLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await draw()
    const viewId = (api.openToolResultLocalDisplay.mock.calls[0][0] as { viewId: string }).viewId
    if (reason === 'hidden') await draw(blockFor(), { visible: false })
    else await act(async () => invalidate(viewId))
    await act(async () => finish({ ok: true, display: failedDisplay() }))
    expect(input()).toBeNull(); expect(container.textContent).not.toContain('PRIVATE_REQUESTED_TODO')
    expect(api.closeToolResultLocalDisplay).toHaveBeenCalledWith(viewId)
    noInventedResult()
  })
  it('does not request or reconstruct a running Todo list', async () => {
    const block = { ...blockFor(), status: 'running' as const }
    await draw(block); await expand()
    expect(container.querySelector('[data-state="running"]')).not.toBeNull()
    expect(api.openToolResultLocalDisplay).not.toHaveBeenCalled()
    expect(input()).toBeNull(); noInventedResult()
  })
  it('retains the immutable successful before/after Todo model', async () => {
    const todo = { id: 'todo-a', content: 'PRIVATE_CAPTURE_AFTER', status: 'completed' as const, createdAt: timestamp, updatedAt: timestamp }
    display = { schemaVersion: 1, resultItemId: selector.resultItemId, snapshotDigest: 'c'.repeat(64), toolName: 'todo_write',
      capture: { ...captureBase, kind: 'todo', status: 'completed', todo: {
        todos: { threadId: 'thread', items: [todo], updatedAt: timestamp },
        baseline: { availability: 'known', todos: { threadId: 'thread',
          items: [{ ...todo, content: 'PRIVATE_CAPTURE_BEFORE', status: 'pending' }], updatedAt: timestamp } } } } }
    expect(toolResultLocalDisplaySchemaV1.safeParse(display).success).toBe(true)
    await draw(blockFor('completed'))
    expect(container.querySelector('[data-state="ok"]')).not.toBeNull()
    expect(container.querySelector('[data-protected-tool-summary]')?.textContent).toContain('1/1 completed')
    await expand()
    expect(input()).toBeNull()
    expect(container.querySelector('[data-tool-details]')).not.toBeNull()
    expect(container.querySelector('[data-change="updated"]')).not.toBeNull()
    expect(container.textContent).toContain('PRIVATE_CAPTURE_AFTER')
    expect(container.textContent).toContain('PRIVATE_CAPTURE_BEFORE')
    expect(container.textContent).not.toContain('PRIVATE_CURRENT_STORE')
  })
})
