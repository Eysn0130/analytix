// @vitest-environment jsdom
import { act, createElement, StrictMode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppShell, { prewarmAppShellSurfaces } from './AppShell'
import { QuestionPlanPreview } from './components/workbench/QuestionPlanPreview'
import { useChatStore } from './store/chat-store'
import { useWorkspaceTabsStore } from './store/workspace-tabs-store'
import type { UserInputLocalDisplayV1, UserInputLocalViewRequestV1 } from '../../../packages/runtime/src/contracts/user-input-local-display'
import i18n from './i18n'

const fixture = vi.hoisted(() => {
  const requestId = 'input_' + 'b'.repeat(64)
  return { owner: { selector: { threadId: 'shell-plan', turnId: 'shell-turn', itemId: 'item_' + requestId, requestId }, callId: 'shell-actual-call' } }
})
// Mock only neighboring surfaces. The persistent AppShell, workspace metadata,
// private lease hook and complete plan rendering are the production consumers.
vi.mock('./components/Workbench', () => ({ Workbench: function WorkbenchRouteFixture() {
  const route = useChatStore(state => state.route)
  return createElement(QuestionPlanPreview, { owner: fixture.owner, presented: route === 'chat' })
} }))
vi.mock('./components/SettingsView', () => ({ SettingsView: () => 'settings-fixture' }))
vi.mock('./components/InitialSetupDialog', () => ({ InitialSetupDialog: () => null }))
vi.mock('./components/RuntimeStatusBanner', () => ({ RuntimeStatusBanner: () => null }))
vi.mock('./components/TopNoticeHost', () => ({ TopNoticeHost: () => null }))
await prewarmAppShellSurfaces()

let root: Root, container: HTMLDivElement, previousApi: PropertyDescriptor | undefined
let previousChat: ReturnType<typeof useChatStore.getState>, previousTabs: ReturnType<typeof useWorkspaceTabsStore.getState>
let open: ReturnType<typeof vi.fn<(request: UserInputLocalViewRequestV1) => Promise<unknown>>>
let invalidationListeners: Set<(id: string | null, reason?: 'authority' | 'background') => void>
let runtimeListeners: Set<(status: { state: string }) => void>
const originalPlan = '# Full original plan\n\nOriginal content through ordinary Settings navigation.'
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('zh')
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  previousChat = useChatStore.getState(); previousTabs = useWorkspaceTabsStore.getState()
  useChatStore.setState({ route: 'chat', runtimeConnection: 'ready', activeThreadId: fixture.owner.selector.threadId, initialSetupOpen: false })
  useWorkspaceTabsStore.setState({ tabs: [], openedQuestionPlanReviews: {} })
  useWorkspaceTabsStore.getState().openQuestionPlanReview(fixture.owner, 'Plan')
  invalidationListeners = new Set(); runtimeListeners = new Set()
  const questionId = fixture.owner.selector.requestId + '_1'
  const display: UserInputLocalDisplayV1 = { schemaVersion: 1, ...fixture.owner.selector, status: 'pending', answerState: 'pending', callId: fixture.owner.callId,
    questions: [{ id: questionId, header: 'Plan review', question: 'Approve original plan?', detail: originalPlan, multiSelect: false,
      options: [{ id: questionId + '_option_1', label: 'Approve', description: '' }], intent: { kind: 'plan-review', approveOptionId: questionId + '_option_1' } }] }
  open = vi.fn(async () => ({ ok: true, display }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { app: { platform: 'darwin' }, runtime: {
    openUserInputLocalDisplay: open, closeUserInputLocalDisplay: vi.fn(async () => undefined),
    onUserInputLocalDisplayInvalidated: (handler: (id: string | null, reason?: 'authority' | 'background') => void) => {
      invalidationListeners.add(handler); return () => { invalidationListeners.delete(handler) }
    },
    onRuntimeStatus: (handler: (status: { state: string }) => void) => { runtimeListeners.add(handler); return () => { runtimeListeners.delete(handler) } }
  } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  await act(async () => root.render(createElement(StrictMode, null, createElement(AppShell))))
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove()
  useChatStore.setState(previousChat); useWorkspaceTabsStore.setState(previousTabs)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix')
})
async function route(value: 'chat' | 'settings') { await act(async () => useChatStore.setState({ route: value })) }
async function retry() { await act(async () => container.querySelector<HTMLButtonElement>('button')!.click()) }

describe('Question plan authority across the real persistent shell routes', () => {
  it('does not treat StrictMode shell observer replay as a true native revoke', () => {
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBeUndefined()
    expect(container.textContent).toContain('Original content through ordinary Settings navigation.')
    expect(container.textContent).not.toContain(i18n.t('common:protectedToolRetry'))
  })
  it('keeps ordinary Settings navigation usable and reacquires the full original plan without an extra Retry step', async () => {
    expect(container.textContent).toContain('Original content through ordinary Settings navigation.')
    const before = open.mock.calls.length, oldView = open.mock.calls.at(-1)?.[0].viewId
    await route('settings'); expect(container.textContent).toContain('settings-fixture')
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBeUndefined()
    await route('chat')
    expect(open.mock.calls.length).toBeGreaterThan(before)
    expect(open.mock.calls.at(-1)?.[0].viewId).not.toBe(oldView)
    expect(container.textContent).toContain('Original content through ordinary Settings navigation.')
    expect(container.textContent).not.toContain(i18n.t('common:protectedToolRetry'))
  })
  it('observes a true global revoke while the plan is not presented and requires an explicit fresh native read on return', async () => {
    const before = open.mock.calls.length
    await route('settings')
    await act(async () => [...invalidationListeners].forEach(listener => listener(null, 'authority')))
    await route('chat')
    expect(open).toHaveBeenCalledTimes(before)
    expect(container.textContent).not.toContain('Original content through ordinary Settings navigation.')
    await retry(); expect(open.mock.calls.length).toBeGreaterThan(before)
    expect(container.textContent).toContain('Original content through ordinary Settings navigation.')
  })
  it('retains retirement across a stopped-and-running runtime during Settings instead of relying on event replay', async () => {
    const before = open.mock.calls.length
    await route('settings')
    await act(async () => {
      runtimeListeners.forEach(listener => listener({ state: 'stopped' }))
      runtimeListeners.forEach(listener => listener({ state: 'running' }))
    })
    await route('chat'); expect(open).toHaveBeenCalledTimes(before)
    await retry(); expect(open.mock.calls.length).toBeGreaterThan(before)
  })
})
