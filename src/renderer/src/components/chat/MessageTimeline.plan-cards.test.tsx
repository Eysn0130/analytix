// @vitest-environment jsdom
import { act, createElement, Fragment } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { chatBlockFromItem, mergeChatBlocks } from '../../agent/analytix-mapper'
import type { CoreTurnItemJson } from '../../agent/analytix-contract'
import type { ChatBlock } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { createGuiPlanArtifact, useGuiPlanStore } from '../../plan/plan-store'
import { useWorkspaceTabsStore, type QuestionPlanTabOwner } from '../../store/workspace-tabs-store'
import { writeWorkDetailsPreference } from '../../lib/chat-presentation-preferences'
import i18n from '../../i18n'
import { userInputLocalDisplaySchemaV1, type UserInputLocalDisplayV1, type UserInputLocalViewRequestV1 } from '../../../../../packages/runtime/src/contracts/user-input-local-display'
import { MessageTimeline } from './MessageTimeline'
import { SubmittedPlanCards } from './presentation/chat/SubmittedPlanCards'
import { QuestionPlanPreview } from '../workbench/QuestionPlanPreview'

vi.mock('../../agent/registry', () => ({ getProvider: () => ({ getThreadDetail: async () => ({ authoritativeTurnTimings: {}, authoritativeTurnStatuses: {} }) }) }))
const timestamp = '2026-10-11T00:00:00Z', threadId = 'plan-history-thread', turnId = 'plan-history-turn'
type Status = 'submitted' | 'cancelled' | 'continued'
function request(hex: string, status: Status = 'submitted', plan = true, approve = false) {
  const requestId = 'input_' + hex.repeat(64).slice(0, 64), questionId = requestId + '_1'
  const selector = { threadId, turnId, itemId: 'item_' + requestId, requestId }
  const markdown = '# PRIVATE_TITLE_' + hex + '\n\nOriginal paragraph.\n\n## Full original tail\n\nPRIVATE_TAIL_' + hex
  const options = [1, 2].map(ordinal => ({ id: questionId + '_option_' + ordinal, label: 'Original option ' + ordinal, description: '' }))
  const display = userInputLocalDisplaySchemaV1.parse({ schemaVersion: 1, ...selector, status, readOnly: true,
    callId: 'original-exit-call-' + hex, answerState: status === 'submitted' ? 'available' : status === 'cancelled' ? 'cancelled' : 'unavailable',
    questions: [{ id: questionId, header: 'Original review', question: 'Review original request?', detail: markdown, multiSelect: false, options,
      ...(plan ? { intent: { kind: 'plan-review', approveOptionId: options[0].id } } : {}) }],
    ...(status === 'submitted' ? { submission: { schemaVersion: 1, format: 'structured', answers: [{ id: questionId, kind: 'answer', optionIds: [options[approve ? 0 : 1].id] }] } } : {})
  })
  const block = chatBlockFromItem({ id: selector.itemId, threadId, turnId, inputId: requestId,
    role: 'system', kind: 'user_input', status, createdAt: timestamp, prompt: 'User input required',
    questions: [{ id: questionId, header: '', question: 'User input required', multiSelect: false,
      options: options.map(option => ({ ...option, label: 'Option' })) }] } as CoreTurnItemJson)!
  return { selector, block, display, markdown }
}
const first = request('a'), second = request('b', 'cancelled'), third = request('c', 'continued'), ordinary = request('d', 'submitted', false)
const user = chatBlockFromItem({ id: 'plan-history-user', threadId, turnId, role: 'user', kind: 'user_message', status: 'completed', createdAt: timestamp, text: 'Admitted synthetic request' } as CoreTurnItemJson)!
let root: Root, container: HTMLDivElement, frames: Map<number, FrameRequestCallback>, serial: number
let previousChat: ReturnType<typeof useChatStore.getState>, previousPlan: ReturnType<typeof useGuiPlanStore.getState>, previousTabs: ReturnType<typeof useWorkspaceTabsStore.getState>
let api: ReturnType<typeof makeApi>, scrollDescriptors: Record<string, PropertyDescriptor | undefined>
const invalidators = new Set<(id: string | null, reason?: 'authority' | 'background') => void>()
const displays = new Map<string, UserInputLocalDisplayV1>()
function makeApi() {
  return { runtime: {
    openUserInputLocalDisplay: vi.fn(async (input: UserInputLocalViewRequestV1): Promise<unknown> => {
      const display = displays.get(input.requestId)
      return display ? { ok: true, display } : { ok: false, code: 'unavailable' }
    }),
    closeUserInputLocalDisplay: vi.fn(async () => undefined),
    onUserInputLocalDisplayInvalidated: (handler: (id: string | null, reason?: 'authority' | 'background') => void) => { invalidators.add(handler); return () => { invalidators.delete(handler) } },
    onRuntimeStatus: () => () => undefined,
    performUserInputLocalEffect: vi.fn(async () => ({ ok: true })),
    submitUserInputResponse: vi.fn(), cancelUserInput: vi.fn(), saveUserInputDraft: vi.fn()
  }, files: { read: vi.fn(), write: vi.fn() } }
}
async function draw(blocks: ChatBlock[] = [user, first.block, second.block, third.block, ordinary.block], preview?: QuestionPlanTabOwner) {
  useChatStore.setState({ blocks })
  await act(async () => root.render(createElement(Fragment, null,
    createElement(MessageTimeline, { blocks, live: '', activeThreadId: threadId, runtimeConnection: 'ready', onRetryConnection() {}, onOpenSettings() {}, conversationUsage: null,
      runtimeStateOverride: { busy: false, currentTurnId: null, currentTurnUserId: null, authoritativeTurnTimings: {}, authoritativeTurnStatuses: { [turnId]: 'completed' } } }),
    preview ? createElement(QuestionPlanPreview, { owner: preview, presented: true }) : null)))
}
const cards = () => [...container.querySelectorAll<HTMLButtonElement>('[data-plan-card]')]
const cardIds = () => cards().map(card => card.dataset.planCard)
const invalidate = () => [...invalidators].forEach(handler => handler(null, 'authority'))
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
  await i18n.changeLanguage('en'); await writeWorkDetailsPreference('detailed')
  frames = new Map(); serial = 0
  vi.stubGlobal('requestAnimationFrame', (run: FrameRequestCallback) => { frames.set(++serial, run); return serial })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  api = makeApi(); vi.stubGlobal('analytix', api)
  displays.clear(); for (const source of [first, second, third, ordinary]) displays.set(source.selector.requestId, source.display)
  scrollDescriptors = Object.fromEntries(['scrollTo', 'scrollIntoView'].map(key => [key, Object.getOwnPropertyDescriptor(HTMLElement.prototype, key)]))
  Object.defineProperty(HTMLElement.prototype, 'scrollTo', { configurable: true, value: vi.fn() })
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true, value: vi.fn() })
  previousChat = useChatStore.getState(); previousPlan = useGuiPlanStore.getState(); previousTabs = useWorkspaceTabsStore.getState()
  useChatStore.setState({ activeThreadId: threadId, route: 'chat', workspaceRoot: '', blocks: [], busy: false, currentTurnId: null, currentTurnUserId: null })
  useWorkspaceTabsStore.setState({ tabs: [], openedQuestionPlanReviews: {}, activeTabId: null, open: false })
  useGuiPlanStore.setState({ activePlan: createGuiPlanArtifact({ workspaceRoot: '/tmp/other-workspace', threadId: 'other-thread', relativePath: '.analytixsdd/plan/unrelated.md', sourceRequest: 'Unrelated plan' }), content: '# UNRELATED_CURRENT_GUI_PLAN' })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); invalidators.clear(); displays.clear(); document.getSelection()?.removeAllRanges()
  for (const [key, descriptor] of Object.entries(scrollDescriptors)) { if (descriptor) Object.defineProperty(HTMLElement.prototype, key, descriptor); else Reflect.deleteProperty(HTMLElement.prototype, key) }
  useChatStore.setState(previousChat); useGuiPlanStore.setState(previousPlan); useWorkspaceTabsStore.setState(previousTabs)
  await writeWorkDetailsPreference('detailed'); vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('submitted plans through the real history mapper and Timeline', () => {
  it('keeps every rejected, cancelled and continued submission in invocation order without a Build action', async () => {
    await draw()
    expect(cardIds()).toEqual([first.display.callId, second.display.callId, third.display.callId])
    for (const source of [first, second, third]) expect(cards().find(card => card.dataset.planCard === source.display.callId)?.textContent).toContain('PRIVATE_TITLE_' + source.selector.requestId.at(-1))
    expect(container.querySelector('[data-plan-artifacts]')?.textContent).not.toContain('PRIVATE_TITLE_d')
    expect(cards().every(card => card.querySelector('button') === null)).toBe(true)
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    expect(api.runtime.submitUserInputResponse).not.toHaveBeenCalled(); expect(api.runtime.cancelUserInput).not.toHaveBeenCalled(); expect(api.runtime.saveUserInputDraft).not.toHaveBeenCalled()
    expect(api.files.read).not.toHaveBeenCalled(); expect(api.files.write).not.toHaveBeenCalled()
  })
  it('opens the earliest exact selector and complete original Markdown independently of the current GUI plan', async () => {
    const blocks = [user, first.block, second.block], before = JSON.stringify(blocks), currentPlan = useGuiPlanStore.getState().activePlan
    await draw(blocks); await act(async () => cards()[0].click())
    const tab = useWorkspaceTabsStore.getState().tabs[0]
    expect(tab.questionPlan).toEqual({ selector: first.selector, callId: first.display.callId })
    expect(tab.title).toBe(i18n.t('common:userInputPlanPreview')); expect(JSON.stringify(tab)).not.toContain('PRIVATE_TITLE_')
    const priorReads = api.runtime.openUserInputLocalDisplay.mock.calls.map(([input]) => input.viewId)
    await draw(blocks, tab.questionPlan)
    const preview = container.querySelector('[data-plan-preview="original-exit-call-a"]')
    expect(preview?.textContent).toContain('PRIVATE_TAIL_a'); expect(preview?.textContent).not.toContain('PRIVATE_TAIL_b'); expect(preview?.textContent).not.toContain('UNRELATED_CURRENT_GUI_PLAN')
    const newReads = api.runtime.openUserInputLocalDisplay.mock.calls.filter(([input]) => !priorReads.includes(input.viewId))
    expect(newReads.some(([input]) => input.threadId === first.selector.threadId && input.turnId === first.selector.turnId && input.itemId === first.selector.itemId && input.requestId === first.selector.requestId)).toBe(true)
    expect(JSON.stringify(blocks)).toBe(before); expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('PRIVATE_TITLE_')
    expect(useGuiPlanStore.getState().activePlan).toBe(currentPlan); expect(api.files.read).not.toHaveBeenCalled(); expect(api.files.write).not.toHaveBeenCalled()
    expect(api.runtime.submitUserInputResponse).not.toHaveBeenCalled()
  })
  it('does not sort cards by completion time or duplicate a replayed request', async () => {
    const finish: Array<() => void> = []
    api.runtime.openUserInputLocalDisplay.mockImplementation(input => input.requestId === first.selector.requestId
      ? new Promise(resolve => finish.push(() => resolve({ ok: true, display: first.display })))
      : Promise.resolve({ ok: true, display: displays.get(input.requestId) }))
    await draw([user, first.block, second.block, first.block])
    expect(cardIds()).toEqual([second.display.callId]); expect(finish.length).toBeGreaterThan(0)
    await act(async () => { for (const complete of finish) complete() })
    expect(cardIds()).toEqual([first.display.callId, second.display.callId])
  })
  it('keeps an ordinary Question with identical Markdown out of the submitted-plan collection', async () => {
    await act(async () => root.render(createElement(SubmittedPlanCards, { threadId, turnId, blocks: [ordinary.block] })))
    expect(cards()).toEqual([]); expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    for (const [input] of api.runtime.openUserInputLocalDisplay.mock.calls) expect(api.runtime.closeUserInputLocalDisplay).toHaveBeenCalledWith(input.viewId)
  })
  it('releases ordinary Question leases so a later submitted plan is not starved by the native seat budget', async () => {
    const ordinaryRequests = Array.from({ length: 31 }, (_, ordinal) => request((ordinal + 1).toString(16).padStart(2, '0'), 'submitted', false))
    for (const source of ordinaryRequests) displays.set(source.selector.requestId, source.display)
    await act(async () => root.render(createElement(SubmittedPlanCards, { threadId, turnId, blocks: [...ordinaryRequests.map(source => source.block), first.block] })))
    expect(cardIds()).toEqual([first.display.callId])
    for (const [input] of api.runtime.openUserInputLocalDisplay.mock.calls.filter(([input]) => input.requestId !== first.selector.requestId)) {
      expect(api.runtime.closeUserInputLocalDisplay).toHaveBeenCalledWith(input.viewId)
    }
  })
  it('rejects a private response detached from the requested selector', async () => {
    displays.set(first.selector.requestId, second.display)
    await draw([user, first.block])
    expect(cards()).toEqual([]); expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
  })
  it('erases private titles on revoke, retains only a generic explicit opener, and reacquires a fresh lease', async () => {
    await draw([user, first.block]); const views = api.runtime.openUserInputLocalDisplay.mock.calls.map(([input]) => input.viewId)
    await act(async () => invalidate())
    expect(container.textContent).not.toContain('PRIVATE_TITLE_a'); expect(container.textContent).not.toContain('PRIVATE_TAIL_a')
    expect(cardIds()).toEqual([first.display.callId]); expect(cards()[0].textContent).toContain(i18n.t('common:userInputPlanPreview'))
    const retiredReads = api.runtime.openUserInputLocalDisplay.mock.calls.length
    await act(async () => cards()[0].click())
    expect(api.runtime.openUserInputLocalDisplay.mock.calls.length).toBeGreaterThan(retiredReads)
    expect(api.runtime.openUserInputLocalDisplay.mock.calls.slice(retiredReads).every(([input]) => !views.includes(input.viewId))).toBe(true)
    expect(cards()[0].textContent).toContain('PRIVATE_TITLE_a')
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlan).toEqual({ selector: first.selector, callId: first.display.callId })
    for (const view of views) expect(api.runtime.closeUserInputLocalDisplay).toHaveBeenCalledWith(view)
    expect(api.runtime.submitUserInputResponse).not.toHaveBeenCalled(); expect(api.files.write).not.toHaveBeenCalled()
  })
  it('does not undo authority retirement when a retained tail is hidden and shown again', async () => {
    const leaf = (presented: boolean) => createElement(SubmittedPlanCards, { threadId, turnId, blocks: [first.block], presented })
    await act(async () => root.render(leaf(true))); await act(async () => invalidate())
    const retiredReads = api.runtime.openUserInputLocalDisplay.mock.calls.length
    await act(async () => root.render(leaf(false))); await act(async () => root.render(leaf(true)))
    expect(api.runtime.openUserInputLocalDisplay.mock.calls.length).toBe(retiredReads)
    expect(container.textContent).not.toContain('PRIVATE_TITLE_a'); expect(cardIds()).toEqual([first.display.callId])
    await act(async () => cards()[0].click())
    expect(cards()[0].textContent).toContain('PRIVATE_TITLE_a')
  })
  it('honors the existing tab retirement disposition after the tail unmounts and remounts', async () => {
    const leaf = () => createElement(SubmittedPlanCards, { threadId, turnId, blocks: [first.block] })
    await act(async () => root.render(leaf())); await act(async () => cards()[0].click()); await act(async () => invalidate())
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBe(true)
    const retiredReads = api.runtime.openUserInputLocalDisplay.mock.calls.length
    await act(async () => root.render(null)); await act(async () => root.render(leaf()))
    expect(api.runtime.openUserInputLocalDisplay.mock.calls.length).toBe(retiredReads)
    expect(cardIds()).toEqual([first.display.callId]); expect(container.textContent).not.toContain('PRIVATE_TITLE_a')
    await act(async () => cards()[0].click()); expect(cards()[0].textContent).toContain('PRIVATE_TITLE_a')
  })
  it('never acquires raw plan text for a hidden retained tail or accepts a late result after hiding', async () => {
    const leaf = (presented: boolean) => createElement(SubmittedPlanCards, { threadId, turnId, blocks: [first.block], presented })
    await act(async () => root.render(leaf(false))); expect(api.runtime.openUserInputLocalDisplay).not.toHaveBeenCalled()
    let finish!: (value: unknown) => void
    api.runtime.openUserInputLocalDisplay.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await act(async () => root.render(leaf(true))); const view = api.runtime.openUserInputLocalDisplay.mock.calls[0][0].viewId
    await act(async () => root.render(leaf(false))); await act(async () => finish({ ok: true, display: first.display }))
    expect(cards()).toEqual([]); expect(container.textContent).not.toContain('PRIVATE_TITLE_a'); expect(api.runtime.closeUserInputLocalDisplay).toHaveBeenCalledWith(view)
  })
  it('does not substitute a later invocation when the exact opened preview fails callId validation', async () => {
    await draw([user, first.block]); await act(async () => cards()[0].click())
    const owner = useWorkspaceTabsStore.getState().tabs[0].questionPlan!
    displays.set(first.selector.requestId, { ...first.display, callId: 'foreign-call' })
    await draw([], owner)
    expect(container.querySelector('[data-plan-preview]')).toBeNull()
    expect(container.textContent).not.toContain('PRIVATE_TAIL_a'); expect(useWorkspaceTabsStore.getState().tabs[0].questionPlan).toEqual(owner)
    expect(api.files.read).not.toHaveBeenCalled(); expect(api.files.write).not.toHaveBeenCalled()
  })
  it('retires the last-successful create_plan metadata card instead of leaving its global Open/Build buttons alongside history', async () => {
    const output = { schemaVersion: 1, projectionKind: 'plan_status', disclosure: 'metadata_only', status: 'completed', code: 'plan_updated', messageKey: 'plan_updated',
      privatePayloadWithheld: true, factAnswerAllowed: false, evidenceAuthority: false,
      plan: { operation: 'draft', planId: 'public-plan', relativePath: '.analytixsdd/plan/old.md', contentHash: 'e'.repeat(64), byteSize: 12, savedAt: timestamp } }
    const legacy = chatBlockFromItem({ id: 'legacy-plan-result', threadId, turnId, callId: 'legacy-create-call', toolName: 'create_plan', toolKind: 'tool_call', kind: 'tool_result',
      role: 'tool', status: 'completed', createdAt: timestamp, isError: false, output } as CoreTurnItemJson)!
    expect(legacy).toMatchObject({ kind: 'tool', meta: { plan: { relative_path: '.analytixsdd/plan/old.md' } } })
    const blocks = mergeChatBlocks([user, legacy, first.block])
    await draw(blocks)
    expect(cardIds()).toEqual([first.display.callId])
    const deliveries = container.querySelector('[data-plan-artifacts]')?.parentElement
    expect(deliveries?.textContent).not.toContain('.analytixsdd/plan/old.md')
    expect([...container.querySelectorAll('button')].some(button => button.textContent?.trim() === i18n.t('common:planBuild'))).toBe(false)
    expect(useGuiPlanStore.getState().content).toBe('# UNRELATED_CURRENT_GUI_PLAN')
  })
})
