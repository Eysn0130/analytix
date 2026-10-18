// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ToolBlock } from '../../agent/types'
import { chatBlockFromItem } from '../../agent/analytix-mapper'
import { userInputLocalSelectorSchemaV1, type UserInputLocalViewRequestV1 } from '../../../../../packages/runtime/src/contracts/user-input-local-display'
import i18n from '../../i18n'
import { previewWorkspaceFile } from '../../lib/workspace-file-preview'
import { openWorkspacePathInEditor } from '../../lib/open-workspace-path'
vi.mock('../../lib/workspace-file-preview', () => ({ previewWorkspaceFile: vi.fn() }))
vi.mock('../../lib/open-workspace-path', () => ({ openWorkspacePathInEditor: vi.fn(async () => ({ ok: true })) }))
import { RUNTIME_DIAGNOSTICS_FOCUS_EVENT } from '../../lib/runtime-diagnostics-focus'
import { useChatStore } from '../../store/chat-store'
import { ToolCallTree } from './presentation/chat/ToolCallTree'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'
let root: Root, container: HTMLDivElement, previous: ReturnType<typeof useChatStore.getState>
let previousApi: PropertyDescriptor | undefined
const tool = (id: string): ToolBlock => ({ kind: 'tool', id, summary: 'read: public fixture', status: 'success', detail: 'tool arguments withheld', meta: { toolName: 'read', diagnostics: { jobId: 'job-safe' } } })
async function expandProcessGroups() {
  for (const header of container.querySelectorAll<HTMLButtonElement>('[data-process-group] > div:first-child button[aria-expanded="false"]')) {
    await act(async () => header.click())
  }
}
async function render(grouped: boolean) {
  await act(async () => root.render(createElement(ToolCallTree, { blocks: grouped ? [tool('tool-a'), tool('tool-b')] : [tool('tool-a')], processing: false })))
  await expandProcessGroups()
}
beforeEach(async () => { vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('zh'); previous = useChatStore.getState(); previousApi = Object.getOwnPropertyDescriptor(window, 'analytix'); useChatStore.setState({ activeThreadId: 'presentation-thread', route: 'chat', activeThreadGoal: null }); container = document.createElement('div'); document.body.append(container); root = createRoot(container) })
afterEach(async () => { await act(async () => root.unmount()); container.remove(); useChatStore.setState(previous); vi.restoreAllMocks(); vi.unstubAllGlobals(); if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix') })
describe('atomic public process disclosure interaction', () => {
  it.each(['Enter', ' '])('preserves the actual file action and line without toggling the row with %s', async key => {
    const block = { ...tool('file-action'), filePath: '/synthetic/action.ts', meta: { toolName: 'read', line: 7 } }
    await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false })))
    await expandProcessGroups()
    const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="file-action"] [data-disclosure-row]')!
    const file = row.querySelector<HTMLButtonElement>('[data-tool-file-link]')!
    const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    await act(async () => file.dispatchEvent(event))
    expect(row.getAttribute('aria-expanded')).toBe('false')
    await act(async () => file.click())
    expect(previewWorkspaceFile).toHaveBeenLastCalledWith({ path: '/synthetic/action.ts', line: 7, workspaceRoot: useChatStore.getState().workspaceRoot })
    await act(async () => file.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })))
    expect(openWorkspacePathInEditor).toHaveBeenLastCalledWith({ path: '/synthetic/action.ts', line: 7 }, useChatStore.getState().workspaceRoot)
    expect(row.getAttribute('aria-expanded')).toBe('false')
  })
  it.each(['custom_web_search_read', 'constructor', '__proto__', 'toString'])('dispatches unknown name %s generically and suppresses success links for actual failures', async toolName => {
    const unknown = { ...tool('unknown'), meta: { toolName } }
    const failed = { ...tool('failed'), status: 'error' as const, filePath: '/synthetic/failed.ts', detail: 'Public failure', meta: { toolName: 'read' } }
    await act(async () => root.render(createElement(ToolCallTree, { blocks: [unknown, failed], processing: true })))
    expect(container.querySelector('[data-analytix-process-block-id="unknown"]')?.getAttribute('data-tool-view')).toBe('tool')
    expect(container.querySelector('[data-analytix-process-block-id="failed"] [data-tool-file-link]')).toBeNull()
    expect(container.textContent).toContain('Public failure')
  })

  it('opens the unknown producer fallback as actual output without inventing input arguments', async () => {
    const block = { ...tool('generic-output'), detail: 'Actual public output', meta: { toolName: 'custom_tool' } }
    await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false })))
    await expandProcessGroups()
    const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="generic-output"] [data-disclosure-row]')!
    expect(container.textContent).not.toContain('Actual public output')
    await act(async () => row.click())
    const body = document.getElementById(row.getAttribute('aria-controls')!)!
    expect(body.textContent).toContain('OUTActual public output')
    expect(body.textContent).not.toContain('IN')
    expect(body.querySelector('input,textarea,[contenteditable="true"]')).toBeNull()
    await act(async () => row.click())
    expect(container.textContent).not.toContain('Actual public output')
  })

  it.each([false, true].flatMap(grouped => ['Enter', ' '].map(key => ({ grouped, key }))))('exposes the actual body with $key (multiple=$grouped)', async ({ grouped, key }) => {
    await render(grouped)
    const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="tool-a"] [data-disclosure-row]')!
    expect(row.getAttribute('aria-expanded')).toBe('false')
    const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    await act(async () => row.dispatchEvent(event))
    expect(event.defaultPrevented).toBe(true); expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(document.getElementById(row.getAttribute('aria-controls')!)!.textContent).toContain('tool arguments withheld')
    if (grouped) expect(container.querySelector('[data-analytix-process-block-id="tool-b"] [data-disclosure-row]')!.getAttribute('aria-expanded')).toBe('false')
    await act(async () => row.click()); expect(row.getAttribute('aria-expanded')).toBe('false'); expect(container.textContent).not.toContain('tool arguments withheld')
  })
  it.each(['Enter', ' '])('keeps the independent diagnostics $key activation isolated', async key => {
    await render(true)
    const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="tool-a"] [data-disclosure-row]')!
    const button = container.querySelector<HTMLButtonElement>(`[data-analytix-process-block-id="tool-a"] button[aria-label="${i18n.t('common:openRuntimeDiagnostics')}"]`)!
    const focused = vi.fn(); window.addEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, focused)
    try {
      const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
      await act(async () => button.dispatchEvent(event)); expect(event.defaultPrevented).toBe(false)
      await act(async () => button.click()); expect(focused).toHaveBeenCalledOnce(); expect(row.getAttribute('aria-expanded')).toBe('false')
    } finally { window.removeEventListener(RUNTIME_DIAGNOSTICS_FOCUS_EVENT, focused) }
  })
  it('opens filtered child diagnostics and controls in one body without private output', async () => {
    const block: ToolBlock = { kind: 'tool', id: 'child-detail', toolKind: 'subagent', status: 'success', summary: 'task: audit',
      detail: JSON.stringify({ output: 'PRIVATE_CHILD_OUTPUT', error: 'PRIVATE_CHILD_ERROR' }),
      meta: { toolName: 'task', child: { childId: 'child', childThreadId: 'child-thread', childRunId: 'job-child',
        childStatus: 'completed', childProfileDescription: 'Synthetic public profile note', parentThreadId: 'parent-thread',
        parentTurnId: 'parent-turn', parentToolCallId: 'parent-call', worktreeBranch: 'codex/synthetic', worktreePath: '/tmp/synthetic' },
        diagnostics: { status: 'completed', leaseOwner: 'synthetic-owner', leaseExpiresAt: '2026-10-06T00:00:00Z',
          recoveryUpdatedAt: '2026-10-06T00:00:01Z', lastHeartbeatAt: '2026-10-06T00:00:02Z',
          recoveryReason: 'PRIVATE_RECOVERY_REASON', deadLetterReason: 'PRIVATE_DEAD_REASON', artifactPath: '/tmp/PRIVATE_ARTIFACT',
          suggestedTools: ['PRIVATE_SUGGESTED_TOOL'] } } }
    await act(async () => root.render(createElement(ToolCallTree, { blocks: [block], processing: false })))
    await expandProcessGroups()
    const row = container.querySelector<HTMLElement>('[data-analytix-process-block-id="child-detail"] [data-disclosure-row]')!
    expect(container.textContent).not.toContain('Synthetic public profile note')
    expect(row.getAttribute('aria-expanded')).toBe('false')
    await act(async () => row.click())
    expect(container.querySelectorAll('[data-analytix-process-block-id="child-detail"] [data-disclosure-row]')).toHaveLength(1)
    const body = document.getElementById(row.getAttribute('aria-controls')!)!
    for (const value of ['Synthetic public profile note', 'parent-thread', 'parent-turn', 'parent-call',
      'codex/synthetic', '/tmp/synthetic', 'synthetic-owner', '2026-10-06T00:00:01Z', '2026-10-06T00:00:02Z']) expect(body.textContent).toContain(value)
    expect(body.querySelector(`button[aria-label="${i18n.t('common:subagentOpenSession')}"]`)).not.toBeNull()
    expect(body.textContent).not.toContain('PRIVATE_')
    await act(async () => row.click()); expect(container.textContent).not.toContain('synthetic-owner')
  })
  it('focuses only the approval composer belonging to this thread even with an equal request id', async () => {
    const wrong = document.createElement('div'), owner = document.createElement('div')
    wrong.setAttribute('data-analytix-gate-composer', ''); wrong.dataset.gateThreadId = 'other-thread'
    owner.setAttribute('data-analytix-gate-composer', ''); owner.dataset.gateThreadId = 'presentation-thread'
    wrong.innerHTML = '<section data-gate-request-id="same"><button>Wrong</button></section>'
    owner.innerHTML = '<section data-gate-request-id="same"><button>Owner</button></section>'
    document.body.append(wrong, owner)
    try {
      await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: 'presentation-thread' }, createElement(ToolCallTree, { processing: true, blocks: [{ kind: 'approval', id: 'gate', approvalId: 'same', summary: 'Synthetic approval', status: 'pending' }] }))))
      await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
      expect(document.activeElement).toBe(owner.querySelector('button'))
      expect(container.querySelector('textarea')).toBeNull()
    } finally { wrong.remove(); owner.remove() }
  })
  it('reopens a real user-input selector through Host and selects only its owned panel', async () => {
    const requestId = 'input_' + 'a'.repeat(64)
    const selector = { threadId: 'presentation-thread', turnId: 'source-turn', itemId: 'item_' + requestId, requestId }
    expect(userInputLocalSelectorSchemaV1.safeParse(selector).success).toBe(true)
    const input = chatBlockFromItem({ kind: 'user_input', id: selector.itemId, inputId: requestId,
      threadId: selector.threadId, turnId: selector.turnId, role: 'assistant', status: 'pending', createdAt: '2026-10-11T00:00:00Z' })
    if (input?.kind !== 'user_input') throw new Error('Fixture must use the actual user-input mapper')
    const anchor: ToolBlock = { ...tool('question-anchor'), status: 'running', toolKind: 'tool_call',
      meta: { toolName: 'ask_user_question', callId: 'question-call', turnId: selector.turnId } }
    const open = vi.fn(async (_request: UserInputLocalViewRequestV1) => ({ ok: false as const, code: 'unavailable' as const }))
    const close = vi.fn(async () => undefined)
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
      openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close,
      onUserInputLocalDisplayInvalidated: () => () => undefined
    } } })
    const notices: unknown[] = [], onReopen = (event: Event) => notices.push((event as CustomEvent).detail)
    window.addEventListener('analytix-user-input-explicit-reopen', onReopen)
    const draw = async (threadId: string) => {
      await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: threadId },
        createElement(ToolCallTree, { processing: true, blocks: [anchor, input] }))))
    }
    try {
      useChatStore.setState({ userInputPanel: undefined })
      await draw(selector.threadId)
      expect(open).toHaveBeenCalledOnce()
      await act(async () => container.querySelector<HTMLButtonElement>('[data-gate-request-id] button')!.click())
      expect(notices).toEqual([selector])
      expect(useChatStore.getState().userInputPanel).toEqual({ threadId: selector.threadId, requestId })
      expect(open).toHaveBeenCalledTimes(2)
      for (const [request] of open.mock.calls) expect(request).toMatchObject(selector)
      expect(open.mock.calls[1][0].viewId).not.toBe(open.mock.calls[0][0].viewId)
      expect(close).toHaveBeenCalledTimes(2)
      expect(container.querySelector('[data-protected-user-input]')).toBeNull()
      useChatStore.setState({ userInputPanel: undefined })
      await draw('other-thread')
      const wrong = container.querySelector<HTMLButtonElement>('[data-gate-request-id] button')!
      expect(wrong.disabled).toBe(true)
      await act(async () => wrong.click())
      expect(useChatStore.getState().userInputPanel).toBeUndefined()
      expect(notices).toEqual([selector]); expect(open).toHaveBeenCalledTimes(2)
    } finally { window.removeEventListener('analytix-user-input-explicit-reopen', onReopen) }
  })
})
