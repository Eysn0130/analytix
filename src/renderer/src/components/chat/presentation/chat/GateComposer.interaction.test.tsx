// @vitest-environment jsdom
import { act, createElement, StrictMode, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { parse } from 'postcss'
import { isClosedPublicRuntimeSseEvent, isStrictPublicRuntimeSseIpcPayload, projectPublicRuntimeSseBlock } from '../../../../../../shared/public-runtime-sse'
import { PublicRuntimeEventFilter } from '../../../../../../shared/public-runtime-content'
import type { UserInputDraftSaveResultV1 } from '../../../../../../shared/analytix-api'
import { dispatchAnalytixRuntimeEvents } from '../../../../agent/analytix-mapper'
import { buildThreadEventSink } from '../../../../store/chat-store-runtime'
import { clearBusyWatchdog } from '../../../../store/chat-store-schedulers'
import { clearActiveStream } from '../../../../thread/streaming/active-stream-store'
import type { ChatBlock, UserInputSubmission, UserInputWaitState } from '../../../../agent/types'
import { userInputLocalDisplaySchemaV1, type UserInputLocalDisplayV1, type UserInputLocalSelectorV1, type UserInputLocalViewRequestV1, type UserInputLocalSaveDraftRequestV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import i18n from '../../../../i18n'
import { QuestionComposer } from './QuestionComposer'
import { useChatStore } from '../../../../store/chat-store'
import { useWorkspaceTabsStore, questionPlanWorkspaceTabId } from '../../../../store/workspace-tabs-store'
import { getProvider } from '../../../../agent/registry'
import { ApprovalPanel } from './ApprovalPanel'
import { requestUserInputLocalReopen, useUserInputLocalAssociation, useUserInputLocalDisplay } from './useUserInputLocalDisplay'
import questionStyles from './QuestionComposer.module.css'
let root: Root, container: HTMLDivElement
it('stops both PlanReview spinner motions when the application disables animation', async () => {
  const { readFileSync } = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
  const planReviewCss = readFileSync('src/renderer/src/components/chat/presentation/chat/PlanReviewPanel.module.css', 'utf8')
  const animations = new Map<string, string>()
  parse(planReviewCss).walkRules(rule => {
    if (!rule.selector.includes("data-motion-reduced='true'")) return
    rule.walkDecls('animation', declaration => {
      for (const selector of rule.selectors) animations.set(selector.trim(), declaration.value)
    })
  })
  expect([...animations.keys()].some(selector => selector.endsWith('.spinnerMotion'))).toBe(true)
  expect([...animations.keys()].some(selector => selector.endsWith('.spinnerArc'))).toBe(true)
  expect([...animations.values()]).toEqual(['none', 'none'])
})
let previousApi: PropertyDescriptor | undefined
let previousWorkspaceTabs: ReturnType<typeof useWorkspaceTabsStore.getState>
let previousDraftMetadata: Pick<ReturnType<typeof useChatStore.getState>, 'userInputDraftInterruptions' | 'deletedUserInputDraftThreadIds'>
const fixtureRequestId = 'input_' + 'f'.repeat(64)
const fixtureQ1 = fixtureRequestId + '_1', fixtureQ2 = fixtureRequestId + '_2'
const fixtureA = fixtureQ1 + '_option_1', fixtureB = fixtureQ1 + '_option_2'
const fixtureC = fixtureQ2 + '_option_1', fixtureD = fixtureQ2 + '_option_2'
const question: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: 'item_' + fixtureRequestId,
  requestId: fixtureRequestId, status: 'pending', questions: [{ id: fixtureQ1, header: 'Synthetic question',
    question: 'Synthetic freeform', multiSelect: false, options: [] }] }
const approval: Extract<ChatBlock, { kind: 'approval' }> = { kind: 'approval', id: 'approval-block', approvalId: 'approval', status: 'pending', toolName: 'shell', summary: 'Synthetic operation' }
const button = (key: string) => [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent === i18n.t('common:' + key)) ?? [...container.querySelectorAll<HTMLButtonElement>('button')].find(node => node.getAttribute('aria-label') === i18n.t('common:' + key))!
async function render(element: ReturnType<typeof createElement>) { await act(async () => root.render(createElement(StrictMode, null, element))) }
let questionFixture: ReturnType<typeof installPrivateDisplay> | null = null
async function renderQuestion(props: Omit<ComponentProps<typeof QuestionComposer>, 'selector'>) {
  const selector = { threadId: 'fixture-thread', turnId: 'fixture-turn', itemId: props.block.id, requestId: props.block.requestId }
  const sameOwner = questionFixture !== null && Object.entries(selector).every(([key, value]) =>
    questionFixture!.state.display[key as keyof UserInputLocalSelectorV1] === value)
  const previous = sameOwner ? questionFixture!.state.display : null
  // Malformed identity cases are unknown transport data, rejected by the real
  // private schema. Normal rerenders retain one Host API and its signed chain.
  const next = { schemaVersion: 1, ...selector, status: props.block.status,
    answerState: props.block.status === 'submitted' ? 'unavailable' : props.block.status === 'cancelled' ? 'cancelled' : 'pending',
    questions: props.block.questions.map(item => ({ ...item, multiSelect: item.multiSelect ?? false })),
    draftRevision: previous?.draftRevision ?? null,
    draft: props.block.status === 'pending' ? previous?.draft ?? null : null } as UserInputLocalDisplayV1
  const changedQuestions = previous !== null && JSON.stringify(previous.questions) !== JSON.stringify(next.questions)
  if (!sameOwner) {
    questionFixture = installPrivateDisplay(next)
    const fixture = questionFixture
    Object.assign(fixture.api, { saveUserInputDraft: vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      const current = fixture.state.display
      if (Object.entries(selector).some(([key, value]) => request[key as keyof UserInputLocalSelectorV1] !== value) || current.status !== 'pending')
        return { ok: false as const, code: 'retired' as const }
      const replay = request.revision === current.draftRevision && JSON.stringify(request.draft) === JSON.stringify(current.draft)
      if (!replay && request.expectedRevision !== current.draftRevision) return { ok: false as const, code: 'conflict' as const }
      fixture.state.display = { ...current, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...selector, status: 'pending' as const,
        draftRevision: request.revision, draft: request.draft } }
    }) })
  } else questionFixture!.state.display = next
  await render(createElement(QuestionComposer, { ...props, selector,
    block: { ...props.block, meta: { ...props.block.meta, turnId: selector.turnId }, questions: [] } }))
  // The deliberately corrected malformed-Host fixture requires the existing
  // explicit fresh-read path; ordinary props do not fabricate a transport.
  if (changedQuestions) await act(async () => requestUserInputLocalReopen(selector))
  return questionFixture!
}
async function key(node: HTMLElement, key: string, options: KeyboardEventInit = {}) { await act(async () => node.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...options }))) }
beforeEach(async () => {
  questionFixture = null
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  previousWorkspaceTabs = useWorkspaceTabsStore.getState()
  useWorkspaceTabsStore.setState({ open: false, selectorOpen: true, activeTabId: null, tabs: [], openedQuestionPlanReviews: {} })
  const state = useChatStore.getState()
  previousDraftMetadata = { userInputDraftInterruptions: state.userInputDraftInterruptions, deletedUserInputDraftThreadIds: state.deletedUserInputDraftThreadIds }
  useChatStore.setState({ userInputDraftInterruptions: {}, deletedUserInputDraftThreadIds: {} })
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('zh')
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); document.getSelection()?.removeAllRanges()
  useChatStore.setState(previousDraftMetadata)
  useWorkspaceTabsStore.setState(previousWorkspaceTabs)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
})
describe('DSH composer interaction on existing gate authority', () => {
  it.each(['userInputSubmit', 'userInputCancel'])('cancels once from focused %s with Escape and preserves native Enter', async name => {
    let finish!: () => void
    const submit = vi.fn(async () => {}), cancel = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    await renderQuestion({ block: question, onSubmit: submit, onCancel: cancel })
    await typeAnswer('Synthetic answer')
    const target = button(name); target.focus(); expect(document.activeElement).toBe(target)
    const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    await act(async () => { target.dispatchEvent(enter) })
    expect(enter.defaultPrevented).toBe(false); expect(submit).not.toHaveBeenCalled(); expect(cancel).not.toHaveBeenCalled()
    await key(target, 'Escape'); await key(target, 'Escape', { repeat: true }); await key(target, 'Escape')
    expect(cancel).toHaveBeenCalledOnce(); expect(submit).not.toHaveBeenCalled()
    expect(button('userInputCancel').disabled).toBe(true)
    await act(async () => finish())
  })
  it('keeps Escape on buttons behind the IME, composition-end, 229, and repeat guards', async () => {
    const cancel = vi.fn(async () => {}), submit = vi.fn(async () => {})
    await renderQuestion({ block: question, onSubmit: submit, onCancel: cancel })
    await typeAnswer('Synthetic answer')
    const target = button('userInputSubmit'), textarea = container.querySelector('textarea')!
    target.focus()
    await key(target, 'Escape', { isComposing: true }); await key(target, 'Escape', { keyCode: 229 }); await key(target, 'Escape', { repeat: true })
    await act(async () => { textarea.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })) })
    await key(target, 'Escape')
    await act(async () => { textarea.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })) })
    await key(target, 'Escape'); expect(cancel).not.toHaveBeenCalled()
    await act(async () => { target.dispatchEvent(new KeyboardEvent('keyup', { key: 'Escape', bubbles: true })) })
    await key(target, 'Escape'); expect(cancel).toHaveBeenCalledOnce(); expect(submit).not.toHaveBeenCalled()
  })
  it('does not cancel a read-only or already resolving question from its focused button', async () => {
    const cancel = vi.fn(async () => {}), submit = vi.fn(async () => {})
    await renderQuestion({ block: question, readOnly: true, onOpenOwnerThread: vi.fn(), onSubmit: submit, onCancel: cancel })
    const owner = button('respondInOwnedThread'); owner.focus(); await key(owner, 'Escape')
    await renderQuestion({ block: question, resolving: true, onSubmit: submit, onCancel: cancel })
    await key(button('userInputCancel'), 'Escape')
    expect(cancel).not.toHaveBeenCalled(); expect(submit).not.toHaveBeenCalled()
  })
  it.each(['question', 'approval'])('releases the synchronous lock after a failed %s response under StrictMode', async kind => {
    let reject!: (error: Error) => void
    const action = vi.fn().mockImplementationOnce(() => new Promise((_, fail) => { reject = fail })).mockResolvedValueOnce(undefined)
    if (kind === 'question') { await renderQuestion({ block: question, onSubmit: action }); await typeAnswer('Synthetic answer') }
    else await render(createElement(ApprovalPanel, { block: approval, onResolve: action }))
    const submit = () => button(kind === 'question' ? 'userInputSubmit' : 'approvalAllow')
    await act(async () => { submit().click(); submit().click() })
    expect(action).toHaveBeenCalledOnce(); expect(submit().disabled).toBe(true)
    await act(async () => reject(new Error('synthetic failure')))
    expect(container.querySelector('[role="alert"]')).not.toBeNull(); expect(submit().disabled).toBe(false)
    await act(async () => submit().click()); expect(action).toHaveBeenCalledTimes(2)
  })
  it('preserves textarea Shift+Enter and guarded Ctrl/Cmd submit while suppressing IME end, 229, and repeat', async () => {
    const action = vi.fn(async () => {}); await renderQuestion({ block: question, onSubmit: action })
    await typeAnswer('Synthetic answer')
    const textarea = container.querySelector('textarea')!
    await key(textarea, 'Enter', { shiftKey: true }); expect(action).not.toHaveBeenCalled()
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
    await key(textarea, 'Enter', { ctrlKey: true })
    await act(async () => textarea.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
    await key(textarea, 'Enter', { metaKey: true }); expect(action).not.toHaveBeenCalled()
    await act(async () => textarea.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })))
    await key(textarea, 'Enter', { ctrlKey: true, keyCode: 229 }); await key(textarea, 'Enter', { metaKey: true, repeat: true })
    expect(action).not.toHaveBeenCalled()
    await key(textarea, 'Enter', { metaKey: true }); expect(action).toHaveBeenCalledOnce()
  })
  it('submits a freeform Answer and uses the existing translated placeholder', async () => {
    const action = vi.fn(async () => {})
    await renderQuestion({ block: { ...question, answers: [] }, onSubmit: action })
    const textarea = container.querySelector('textarea')!
    expect(textarea.placeholder).toBe(i18n.t('common:userInputCustomPlaceholder'))
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, 'Synthetic typed answer')
      textarea.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => button('userInputSubmit').click())
    expect(action).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [], customText: 'Synthetic typed answer' }])
  })
  it('uses focused approval Enter/Escape once and keeps secondary requests navigation-only', async () => {
    const action = vi.fn(async () => {}), owner = vi.fn()
    await render(createElement(ApprovalPanel, { block: approval, onResolve: action }))
    await key(container.querySelector('section')!, 'Enter', { repeat: true }); expect(action).not.toHaveBeenCalled()
    await key(container.querySelector('section')!, 'Escape'); expect(action).toHaveBeenCalledWith('deny')
    await render(createElement(ApprovalPanel, { block: approval, readOnly: true, onOpenOwnerThread: owner, onResolve: action }))
    await key(container.querySelector('section')!, 'Enter'); expect(action).toHaveBeenCalledOnce()
    await act(async () => button('respondInOwnedThread').click()); expect(owner).toHaveBeenCalledWith('approval')
    expect(button('approvalAllow')).toBeUndefined()
  })
})


function structuredBlock(): Extract<ChatBlock, { kind: 'user_input' }> {
  return { kind: 'user_input', id: 'item_' + fixtureRequestId, requestId: fixtureRequestId, status: 'pending', questions: [
    { id: fixtureQ1, header: 'First', question: 'First synthetic question', multiSelect: false,
      options: [{ id: fixtureA, label: 'Alpha', description: '' }, { id: fixtureB, label: 'Beta', description: '' }] },
    { id: fixtureQ2, header: 'Second', question: 'Second synthetic question', multiSelect: false,
      options: [{ id: fixtureC, label: 'Gamma', description: '' }, { id: fixtureD, label: 'Delta', description: '' }] }
  ] }
}
const optionInput = (id: string) => container.querySelector<HTMLInputElement>(`input[value="${id}"]`)!
const pager = (name: 'Previous' | 'Next') => container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:userInput' + name)}"]`)!
async function typeAnswer(value: string) {
  const textarea = container.querySelector<HTMLTextAreaElement>('textarea')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, value)
    textarea.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

describe('Core-owned structured question flow', () => {
  it('pages a single choice automatically, preserves back edits, and confirms the last question once', async () => {
    const submit = vi.fn(async () => {})
    await renderQuestion({ block: structuredBlock(), onSubmit: submit })
    await act(async () => optionInput(fixtureA).click())
    expect(container.querySelector('h2')?.textContent).toBe('Second synthetic question')
    expect(document.activeElement).toBe(optionInput(fixtureC))
    expect(submit).not.toHaveBeenCalled()
    await act(async () => pager('Previous').click())
    expect(optionInput(fixtureA).checked).toBe(true)
    await act(async () => optionInput(fixtureA).click())
    expect(container.querySelector('h2')?.textContent).toBe('Second synthetic question')
    await act(async () => pager('Previous').click())
    await act(async () => optionInput(fixtureB).click())
    await act(async () => optionInput(fixtureD).click())
    expect(submit).not.toHaveBeenCalled()
    await act(async () => { button('userInputSubmit').click(); button('userInputSubmit').click() })
    expect(submit).toHaveBeenCalledOnce()
    expect(submit).toHaveBeenCalledWith([
      { id: fixtureQ1, kind: 'answer', optionIds: [fixtureB] },
      { id: fixtureQ2, kind: 'answer', optionIds: [fixtureD] }
    ])
  })

  it('confirms modern choices with guarded option Enter and reports the first missing question', async () => {
    const block = structuredBlock()
    block.questions[0].options[0].label += ' (Recommended)'
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    await key(optionInput(fixtureA), 'Enter', { repeat: true })
    await key(optionInput(fixtureA), 'Enter', { isComposing: true })
    expect(submit).not.toHaveBeenCalled()
    expect(container.querySelector('h2')?.textContent).toBe('First synthetic question')
    await key(optionInput(fixtureA), 'Enter')
    expect(submit).not.toHaveBeenCalled()
    expect(container.querySelector('h2')?.textContent).toBe('Second synthetic question')
    expect(container.querySelector('footer [role="alert"]')?.textContent).toBe(i18n.t('common:userInputIncomplete'))
    await act(async () => optionInput(fixtureC).click())
    await act(async () => pager('Previous').click())
    await key(optionInput(fixtureA), 'Enter')
    expect(submit).toHaveBeenCalledOnce()
    expect(submit).toHaveBeenCalledWith([
      { id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] },
      { id: fixtureQ2, kind: 'answer', optionIds: [fixtureC] }
    ])
  })

  it('continues modern textarea answers with plain Enter, preserves Shift+Enter, and guards IME and repeated submission', async () => {
    const block = structuredBlock()
    block.questions = block.questions.map(item => ({ ...item, options: [] }))
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    await typeAnswer('  Synthetic first answer  ')
    const first = container.querySelector('textarea')!
    const shiftEnter = new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true, bubbles: true, cancelable: true })
    await act(async () => first.dispatchEvent(shiftEnter))
    expect(shiftEnter.defaultPrevented).toBe(false)
    expect(container.querySelector('h2')?.textContent).toBe('First synthetic question')
    expect(submit).not.toHaveBeenCalled()
    const next = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    await act(async () => first.dispatchEvent(next))
    expect(next.defaultPrevented).toBe(true)
    expect(container.querySelector('h2')?.textContent).toBe('Second synthetic question')
    expect(submit).not.toHaveBeenCalled()
    await typeAnswer('  Synthetic second answer  ')
    const last = container.querySelector('textarea')!
    await key(last, 'Enter', { isComposing: true })
    await key(last, 'Enter', { keyCode: 229 })
    await key(last, 'Enter', { repeat: true })
    await act(async () => last.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
    await key(last, 'Enter')
    await act(async () => last.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
    await key(last, 'Enter')
    expect(submit).not.toHaveBeenCalled()
    await act(async () => last.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', bubbles: true })))
    await act(async () => {
      last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
      last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    })
    expect(submit).toHaveBeenCalledOnce()
    expect(submit).toHaveBeenCalledWith([
      { id: fixtureQ1, kind: 'answer', optionIds: [], customText: '  Synthetic first answer  ' },
      { id: fixtureQ2, kind: 'answer', optionIds: [], customText: '  Synthetic second answer  ' }
    ])
  })

  it('keeps multiple Core identities on one page and retains them beside custom text', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], multiSelect: true, options: [
      { id: fixtureA, label: 'Other', description: '' },
      { id: fixtureB, label: 'Other', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    await act(async () => { optionInput(fixtureA).click(); optionInput(fixtureB).click() })
    expect(optionInput(fixtureA).checked).toBe(true)
    expect(optionInput(fixtureB).checked).toBe(true)
    expect(container.querySelector('h2')?.textContent).toBe('First synthetic question')
    await typeAnswer('  Synthetic custom answer  ')
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer',
      optionIds: [fixtureA, fixtureB], customText: '  Synthetic custom answer  ' }])
  })

  it('replaces a single selection with custom text without treating a real Other label as custom', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Other', description: '' },
      { id: fixtureB, label: 'Beta', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    await act(async () => optionInput(fixtureA).click())
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(optionInput(fixtureA).checked).toBe(true)
    await typeAnswer('Synthetic replacement')
    expect(optionInput(fixtureA).checked).toBe(false)
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [], customText: 'Synthetic replacement' }])
  })

  it('clears current selections and custom text on skip and submits the final skip as one batch', async () => {
    const submit = vi.fn(async () => {})
    await renderQuestion({ block: structuredBlock(), onSubmit: submit })
    await act(async () => optionInput(fixtureA).click())
    await act(async () => pager('Previous').click())
    await typeAnswer('Synthetic discarded draft')
    await act(async () => button('userInputSkip').click())
    expect(submit).not.toHaveBeenCalled()
    await act(async () => optionInput(fixtureC).click())
    await act(async () => { button('userInputSkip').click(); button('userInputSkip').click() })
    expect(submit).toHaveBeenCalledOnce()
    expect(submit).toHaveBeenCalledWith([
      { id: fixtureQ1, kind: 'skip' }, { id: fixtureQ2, kind: 'skip' }
    ])
    expect(JSON.stringify(submit.mock.calls)).not.toContain('Synthetic discarded draft')
  })

  it('returns to an unanswered earlier question instead of inferring a skip during final confirmation', async () => {
    const submit = vi.fn(async () => {})
    await renderQuestion({ block: structuredBlock(), onSubmit: submit })
    await act(async () => pager('Next').click())
    await act(async () => button('userInputSkip').click())
    expect(submit).not.toHaveBeenCalled()
    expect(container.querySelector('h2')?.textContent).toBe('First synthetic question')
    expect(container.querySelector('footer [role="alert"]')?.textContent).toBe(i18n.t('common:userInputIncomplete'))
    await act(async () => button('userInputSkip').click())
    expect(button('userInputSubmit').disabled).toBe(true)
    await act(async () => button('userInputSkip').click())
    expect(submit).toHaveBeenCalledWith([
      { id: fixtureQ1, kind: 'skip' }, { id: fixtureQ2, kind: 'skip' }
    ])
  })

  it('rejects incomplete private Host identities without falling back to public raw questions or legacy answers', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha', description: '' }, { label: 'Beta', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(container.querySelectorAll('input')).toHaveLength(0)
    expect(container.textContent).toContain(i18n.t('common:userInputOriginalQuestionUnavailable'))
    expect(button('userInputSubmit').disabled).toBe(true); expect(submit).not.toHaveBeenCalled()
    const current = structuredBlock(); current.questions = [{ ...current.questions[0], options: [{ id: fixtureA, label: 'Other', description: '' }] }]
    await renderQuestion({ block: current, onSubmit: submit })
    await act(async () => optionInput(fixtureA).click())
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] }])
  })

  it.each(['cancelled', 'readOnly'] as const)('drops raw custom drafts when the request becomes %s', async state => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    await typeAnswer('Synthetic private draft')
    await renderQuestion({ block: state === 'cancelled' ? { ...block, status: 'cancelled' } : block,
      readOnly: state === 'readOnly', onSubmit: submit })
    expect(container.textContent).not.toContain('Synthetic private draft')
    await renderQuestion({ block, onSubmit: submit })
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(button('userInputSubmit').disabled).toBe(true)
    expect(submit).not.toHaveBeenCalled()
    expect(questionFixture!.state.display.draftRevision).not.toBeNull()
    if (state === 'readOnly') {
      expect(questionFixture!.state.display.draft?.answers[0].customText).toBe('Synthetic private draft')
      await act(async () => requestUserInputLocalReopen({ threadId: 'fixture-thread', turnId: 'fixture-turn', itemId: block.id, requestId: block.requestId }))
      expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic private draft')
    } else expect(questionFixture!.state.display.draft).toBeNull()
  })
})


describe('question navigation activation guards', () => {
  it('suppresses repeated native button activation while preserving ordinary native Enter', async () => {
    const submit = vi.fn(async () => {})
    await renderQuestion({ block: structuredBlock(), onSubmit: submit })
    const skip = button('userInputSkip')
    skip.focus()
    const repeat = new KeyboardEvent('keydown', { key: 'Enter', repeat: true, bubbles: true, cancelable: true })
    const composingEnter = new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })
    const ordinary = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    await act(async () => { skip.dispatchEvent(repeat); skip.dispatchEvent(composingEnter); skip.dispatchEvent(ordinary) })
    expect(repeat.defaultPrevented).toBe(true)
    expect(composingEnter.defaultPrevented).toBe(true)
    expect(ordinary.defaultPrevented).toBe(false)
    expect(container.querySelector('h2')?.textContent).toBe('First synthetic question')
    expect(submit).not.toHaveBeenCalled()
  })

  it.each([true, false])('keeps empty custom text independent from selected multi options: selection=%s', async selected => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], multiSelect: true }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    if (selected) await act(async () => optionInput(fixtureA).click())
    await typeAnswer('   ')
    expect(button('userInputSubmit').disabled).toBe(!selected)
    expect(submit).not.toHaveBeenCalled()
    await act(async () => button('userInputSubmit').click())
    if (selected) expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] }])
    else expect(submit).not.toHaveBeenCalled()
  })
})


describe('private Host question identity', () => {
  it.each(['constructor', 'toString', '__proto__'])('rejects non-Host question id %s without public fallback or response', async id => {
    const block = structuredBlock(); block.questions = [{ ...block.questions[0], id, options: [] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.textContent).not.toContain('First synthetic question')
    expect(button('userInputSubmit').disabled).toBe(true)
    expect(submit).not.toHaveBeenCalled()
  })
})


describe('recommended first option drafts', () => {
  it.each(['(Recommended)', '(recommended)', '（ReCoMmEnDeD）', '(推荐)', '（推荐）'])('preselects only a marked first Core option %s and waits for explicit submission', async suffix => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha ' + suffix, description: '' },
      { id: fixtureB, label: 'Beta', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(true)
    expect(optionInput(fixtureB).checked).toBe(false)
    expect(button('userInputSubmit').disabled).toBe(false)
    expect(submit).not.toHaveBeenCalled()
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] }])
  })

  it('does not preselect a later recommendation when the first option is unmarked', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha', description: '' },
      { id: fixtureB, label: 'Beta (Recommended)', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(false)
    expect(optionInput(fixtureB).checked).toBe(false)
    expect(button('userInputSubmit').disabled).toBe(true)
    expect(submit).not.toHaveBeenCalled()
  })

  it('does not preselect a first option whose recommendation marker is followed by other text', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha (Recommended) notes', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(false)
    expect(button('userInputSubmit').disabled).toBe(true)
    expect(submit).not.toHaveBeenCalled()
  })

  it('submits the Host identity while keeping the original recommendation label purely presentational', async () => {
    const label = 'Alpha (Recommended)', block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [{ id: fixtureA, label, description: '' }, { id: fixtureB, label: 'Beta', description: '' }] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(true); expect(submit).not.toHaveBeenCalled()
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] }])
    expect(JSON.stringify(submit.mock.calls)).not.toContain(label)
  })

  it('does not let old public label/value answers seed or replace a fresh signed private draft', async () => {
    const block = structuredBlock()
    block.answers = [{ id: fixtureQ1, label: 'Beta', value: 'Beta' }]
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha (Recommended)', description: '' }, { id: fixtureB, label: 'Beta', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(true); expect(optionInput(fixtureB).checked).toBe(false)
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: fixtureQ1, kind: 'answer', optionIds: [fixtureA] }])
  })

  it.each(['submitted', 'readOnly'] as const)('does not initialize recommended drafts in a %s request', async state => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha (Recommended)', description: '' }
    ] }]
    const submit = vi.fn(async () => {})
    await renderQuestion({ block: state === 'submitted' ? { ...block, status: 'submitted' } : block,
      readOnly: state === 'readOnly', onSubmit: submit })
    expect(Array.from(container.querySelectorAll<HTMLInputElement>('input')).every(input => input.matches(':disabled'))).toBe(true)
    await renderQuestion({ block, onSubmit: submit })
    expect(optionInput(fixtureA).checked).toBe(false)
    expect(button('userInputSubmit').disabled).toBe(true)
    expect(submit).not.toHaveBeenCalled()
  })
})


describe('pinned question presentation', () => {
  it('shows the current heading, inline recommendation and description, custom row, and single-question pager', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [
      { id: fixtureA, label: 'Alpha (Recommended)', description: 'Synthetic inline description' }
    ] }]
    await renderQuestion({ block, onSubmit: vi.fn(async () => {}), onCancel: vi.fn(async () => {}) })
    expect(container.querySelector('header h2')?.textContent).toBe('First synthetic question')
    expect(container.querySelector('header')?.textContent).toContain('First')
    expect(container.querySelector('legend')).toBeNull()
    const option = optionInput(fixtureA).closest('label')!
    const line = option.querySelector('[class*="optionLine"]')!
    expect(optionInput(fixtureA).getAttribute('aria-label')).toBe('Alpha')
    expect(line.textContent).toContain('Alpha')
    expect(line.querySelector('[class*="badge"]')?.textContent).toBe(i18n.t('common:userInputRecommended'))
    expect(line.querySelector('[class*="description"]')?.textContent).toBe('Synthetic inline description')
    expect(container.querySelector('[data-question-custom] textarea')).not.toBeNull()
    expect(container.querySelectorAll('input')).toHaveLength(1)
    expect(container.querySelector('footer [class*="progress"]')?.textContent?.trim()).toBe('1 / 1')
    expect(pager('Previous').disabled).toBe(true)
    expect(pager('Next').disabled).toBe(true)
    expect(button('userInputCancel').closest('header')).not.toBeNull()
    expect(button('userInputCancel').closest('footer')).toBeNull()
  })

  it('preserves a local draft across collapse and expansion and freezes header actions while busy', async () => {
    const block = structuredBlock()
    block.questions = [{ ...block.questions[0], options: [] }]
    const submit = vi.fn(async () => {}), cancel = vi.fn(async () => {})
    const fixture = await renderQuestion({ block, onSubmit: submit, onCancel: cancel })
    await typeAnswer('Synthetic local draft')
    const admittedRevision = fixture.state.display.draftRevision
    expect(admittedRevision).not.toBeNull()
    expect(fixture.state.display.draft?.answers[0].customText).toBe('Synthetic local draft')
    await act(async () => button('userInputMinimize').click())
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.querySelector('footer')).toBeNull()
    expect(button('userInputMaximize').getAttribute('aria-expanded')).toBe('false')
    expect(submit).not.toHaveBeenCalled()
    expect(cancel).not.toHaveBeenCalled()
    await act(async () => button('userInputMaximize').click())
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic local draft')
    expect(button('userInputMinimize').getAttribute('aria-expanded')).toBe('true')
    await renderQuestion({ block, resolving: true, onSubmit: submit, onCancel: cancel })
    expect(fixture.state.display.draftRevision).toBe(admittedRevision)
    expect(fixture.state.display.draft?.answers[0].customText).toBe('Synthetic local draft')
    expect(Array.from(container.querySelectorAll<HTMLButtonElement>('header button')).every(control => control.disabled)).toBe(true)
    await act(async () => button('userInputMinimize').click())
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic local draft')
    expect(submit).not.toHaveBeenCalled()
    expect(cancel).not.toHaveBeenCalled()
  })
})


const localRequestId = 'input_' + 'e'.repeat(64)
const localSelector: UserInputLocalSelectorV1 = {
  threadId: 'thread-private', turnId: 'turn-private', itemId: 'item_' + localRequestId, requestId: localRequestId
}
function originalDisplay(): UserInputLocalDisplayV1 {
  return { schemaVersion: 1, ...localSelector, status: 'pending', answerState: 'pending', questions: [
    { id: localRequestId + '_1', header: 'Synthetic original header', question: 'Synthetic original question',
      detail: '**Synthetic detail** [reference](https://example.invalid) ![synthetic image](https://example.invalid/image.png)',
      multiSelect: true, options: [
        { id: localRequestId + '_1_option_1', label: 'Synthetic original Alpha', description: 'Synthetic original description' },
        { id: localRequestId + '_1_option_2', label: 'Synthetic original Beta', description: '' }
      ] },
    { id: localRequestId + '_2', header: 'Synthetic second header', question: 'Synthetic second original question',
      multiSelect: false, options: [{ id: localRequestId + '_2_option_1', label: 'Synthetic original Gamma', description: '' }] }
  ] }
}
function singleDraftDisplay(): UserInputLocalDisplayV1 {
    const source = originalDisplay()
    return { ...source, draftRevision: null, draft: null, questions: [{ ...source.questions[0], multiSelect: false, options: [] }] }
  }
function projectedBlock(status: 'pending' | 'submitted' | 'cancelled' | 'continued' = 'pending'): Extract<ChatBlock, { kind: 'user_input' }> {
  const source = originalDisplay()
  return { kind: 'user_input', id: localSelector.itemId, requestId: localRequestId, status,
    meta: { turnId: localSelector.turnId }, questions: source.questions.map(item => ({
      id: item.id, header: 'Projected header', question: 'Projected question', multiSelect: item.multiSelect,
      options: item.options.map(option => ({ id: option.id, label: 'Projected option', description: '' }))
    })) }
}
function installPrivateDisplay(initial = originalDisplay()) {
  const state = { display: initial }
  const invalidators = new Set<(id: string | null) => void>()
  const api = {
    openUserInputLocalDisplay: vi.fn(async (_request: UserInputLocalViewRequestV1) => ({ ok: true as const, display: state.display })),
    closeUserInputLocalDisplay: vi.fn(async (_id: string) => undefined),
    onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidators.add(listener); return () => { invalidators.delete(listener) } },
    onRuntimeStatus: () => () => undefined,
    runtimeRequest: vi.fn()
  }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  return { state, api, invalidate: () => invalidators.forEach(listener => listener(null)) }
}

describe('same-owner timed question terminal feedback', () => {
  it.each(['scheduled', 'ended'] as const)('retires %s feedback when real Host continues the same request', async phase => {
    const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
    const fixture = installPrivateDisplay({ ...originalDisplay(), timedWait: timing })
    const provider = getProvider()
    let notify!: (state: UserInputWaitState) => void
    vi.spyOn(provider, 'observeUserInputWait').mockImplementation((_selector, listener) => {
      notify = listener; listener({ phase, remainingMs: 0 })
      return { focus() {}, blur() {}, engage() {}, takeTime() {}, retry() {}, unsubscribe() {} }
    })
    const settle = vi.spyOn(provider, 'settleUserInputWait').mockImplementation(() => notify?.({ phase: 'ended', remainingMs: 0 }))
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(), timedWait: timing }, selector: localSelector }))
    expect(container.querySelector('footer [role="status"]')?.textContent).toBe(i18n.t('common:' + (phase === 'scheduled' ? 'userInputContinueScheduled' : 'userInputWaitUnknown')))
    const leaf = container.querySelector('[data-gate-request-id]')
    fixture.state.display = { ...fixture.state.display, status: 'continued', answerState: 'unavailable' }
    await render(createElement(QuestionComposer, { block: { ...projectedBlock('continued'), timedWait: timing }, selector: localSelector, lateReplyTargetTurnId: 'turn-current' }))
    expect(container.querySelector('[data-gate-request-id]')).toBe(leaf)
    expect(settle).toHaveBeenCalledWith(localSelector)
    expect(container.textContent).not.toContain(i18n.t('common:userInputContinueScheduled'))
    expect(container.textContent).not.toContain(i18n.t('common:userInputWaitUnknown'))
    expect(container.textContent).toContain(i18n.t('common:userInputContinued'))
  })
})

describe('original question and answer display stays in the same private Composer leaf', () => {
  it('uses receipt text and actionable Question detail without changing the public block', async () => {
    const { api } = installPrivateDisplay()
    const block = projectedBlock(), before = JSON.stringify(block)
    await render(createElement(QuestionComposer, { block, selector: localSelector, onSubmit: vi.fn(async () => {}) }))
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic original question')
    expect(container.querySelector('header')?.textContent).toContain('Synthetic original header')
    expect(container.textContent).toContain('Synthetic original Alpha')
    expect(container.textContent).toContain('Synthetic original description')
    const detail = container.querySelector(`.${questionStyles.detail}`)!
    expect(detail.querySelector('[data-markdown-variant="compact"]')).toBeNull()
    expect(detail.querySelector('strong')?.textContent).toBe('Synthetic detail')
    expect(detail.querySelectorAll('a,img')).toHaveLength(2)
    expect(detail.querySelectorAll('button,[role="dialog"]')).toHaveLength(0)
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledWith({ ...localSelector, viewId: expect.any(String) })
    expect(api.runtimeRequest).not.toHaveBeenCalled(); expect(JSON.stringify(block)).toBe(before)
    expect(JSON.stringify(block)).not.toContain('Synthetic original')
  })

  it.each(['item', 'request', 'turn'])('rejects a private selector detached from the displayed %s identity', async field => {
    const { api } = installPrivateDisplay()
    const block = projectedBlock()
    if (field === 'item') block.id = 'another-item'
    if (field === 'request') block.requestId = 'another-request'
    if (field === 'turn') block.meta = { turnId: 'another-turn' }
    await render(createElement(QuestionComposer, { block, selector: localSelector }))
    expect(api.openUserInputLocalDisplay).not.toHaveBeenCalled()
    expect(container.textContent).not.toContain('Synthetic original question')
    expect(container.textContent).toContain(i18n.t('common:userInputOriginalQuestionUnavailable'))
  })

  it('submits Core identities and original custom whitespace, then reads recorded options, custom and skip', async () => {
    const { state, api } = installPrivateDisplay()
    const block = projectedBlock(), before = JSON.stringify(block)
    const submit = vi.fn(async (answers: UserInputSubmission[]) => {
      state.display = { ...state.display, status: 'submitted', answerState: 'available',
        submission: { schemaVersion: 1, format: 'structured', answers: answers as Extract<NonNullable<UserInputLocalDisplayV1['submission']>, { format: 'structured' }>['answers'] } }
    })
    await render(createElement(QuestionComposer, { block, selector: localSelector, onSubmit: submit }))
    await act(async () => optionInput(localRequestId + '_1_option_2').click())
    await typeAnswer('  Synthetic original custom\nsecond line  ')
    await act(async () => button('userInputNext').click())
    await act(async () => button('userInputSkip').click())
    expect(submit).toHaveBeenCalledOnce()
    expect(submit).toHaveBeenCalledWith([
      { id: localRequestId + '_1', kind: 'answer', optionIds: [localRequestId + '_1_option_2'], customText: '  Synthetic original custom\nsecond line  ' },
      { id: localRequestId + '_2', kind: 'skip' }
    ])
    expect(optionInput(localRequestId + '_1_option_2').checked).toBe(true)
    expect(optionInput(localRequestId + '_1_option_2').matches(':disabled')).toBe(true)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('  Synthetic original custom\nsecond line  ')
    expect(button('userInputSubmit')).toBeUndefined()
    await act(async () => pager('Next').click())
    expect(container.textContent).toContain(i18n.t('common:userInputSkipped'))
    expect(container.querySelector('textarea')).toBeNull()
    expect(JSON.stringify(block)).toBe(before); expect(api.runtimeRequest).not.toHaveBeenCalled()
  })

  it('recovers available history after remount and keeps read-only answers through collapse', async () => {
    const snapshot = originalDisplay()
    snapshot.questions = [snapshot.questions[0]]
    snapshot.status = 'submitted'; snapshot.answerState = 'available'
    snapshot.submission = { schemaVersion: 1, format: 'structured', answers: [{ id: localRequestId + '_1', kind: 'answer',
      optionIds: [localRequestId + '_1_option_1'], customText: 'Synthetic stored original answer' }] }
    const { api } = installPrivateDisplay(snapshot)
    const block = projectedBlock('submitted')
    await render(createElement(QuestionComposer, { block, selector: localSelector, readOnly: true }))
    expect(optionInput(localRequestId + '_1_option_1').checked).toBe(true)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic stored original answer')
    await act(async () => button('userInputMinimize').click())
    expect(container.querySelector('textarea')).toBeNull()
    await act(async () => button('userInputMaximize').click())
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic stored original answer')
    await act(async () => root.render(null))
    expect(api.closeUserInputLocalDisplay).toHaveBeenCalled()
    await render(createElement(QuestionComposer, { block, selector: localSelector, readOnly: true }))
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic stored original answer')
  })

  it('displays unavailable historical answers explicitly without substituting a public submitted echo', async () => {
    const snapshot = { ...originalDisplay(), status: 'submitted' as const, answerState: 'unavailable' as const }
    installPrivateDisplay(snapshot)
    const block = { ...projectedBlock('submitted'), answers: [{ id: localRequestId + '_1', label: 'Synthetic public echo', value: 'Synthetic public echo' }] }
    await render(createElement(QuestionComposer, { block, selector: localSelector }))
    expect(container.textContent).toContain(i18n.t('common:userInputOriginalAnswerUnavailable'))
    expect(container.textContent).not.toContain('Synthetic public echo')
    expect(container.querySelector('textarea')).toBeNull()
    expect(Array.from(container.querySelectorAll<HTMLInputElement>('input')).every(input => !input.checked)).toBe(true)
  })

  it('keeps legacy raw text bound by questionId without inventing identity from duplicate option labels', async () => {
    const snapshot = originalDisplay()
    snapshot.questions = [snapshot.questions[0]]
    snapshot.questions[0].options = snapshot.questions[0].options.map(option => ({ ...option, label: 'Synthetic duplicate label' }))
    snapshot.status = 'submitted'; snapshot.answerState = 'available'
    snapshot.submission = { schemaVersion: 1, format: 'legacy', answers: [{ id: 'legacy-original-id', questionId: localRequestId + '_1',
      label: 'Synthetic duplicate label', value: 'Synthetic duplicate label' }] }
    installPrivateDisplay(snapshot)
    await render(createElement(QuestionComposer, { block: projectedBlock('submitted'), selector: localSelector, readOnly: true }))
    expect(container.querySelector('[class*="answer"]')?.textContent).toBe('Synthetic duplicate label')
    expect(Array.from(container.querySelectorAll<HTMLInputElement>('input')).every(input => !input.checked)).toBe(true)
    expect(container.querySelector('textarea')).toBeNull()
  })

  it('drops an invalidated draft, requires explicit re-read, and does not reopen from an old action completion', async () => {
    const { api, invalidate } = installPrivateDisplay()
    let finish!: () => void
    const cancel = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector, onCancel: cancel }))
    await typeAnswer('Synthetic retired raw draft')
    await act(async () => button('userInputCancel').click())
    const calls = api.openUserInputLocalDisplay.mock.calls.length
    await act(async () => invalidate())
    expect(container.textContent).not.toContain('Synthetic original question')
    expect(container.querySelector('textarea')).toBeNull()
    await act(async () => finish())
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(calls)
    expect(container.textContent).toContain(i18n.t('common:userInputOriginalQuestionUnavailable'))
    await act(async () => button('protectedToolRetry').click())
    expect(api.openUserInputLocalDisplay).toHaveBeenCalledTimes(calls + 1)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
  })

  it('requires fresh writable Host display before cancelling an unreadable original question', async () => {
    const { api } = installPrivateDisplay()
    api.openUserInputLocalDisplay.mockResolvedValue({ ok: false, code: 'unavailable' } as never)
    const cancel = vi.fn(async () => {})
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector, onCancel: cancel }))
    expect(container.textContent).not.toContain('Projected question')
    expect(button('userInputCancel').disabled).toBe(true)
    await act(async () => button('userInputCancel').click())
    expect(cancel).not.toHaveBeenCalled()
  })

  it('clears pending drafts on read-only and cancellation while keeping authorized original questions', async () => {
    const { state } = installPrivateDisplay()
    const block = projectedBlock()
    await render(createElement(QuestionComposer, { block, selector: localSelector }))
    await typeAnswer('Synthetic pending secret draft')
    await render(createElement(QuestionComposer, { block, selector: localSelector, readOnly: true }))
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.textContent).not.toContain('Synthetic pending secret draft')
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic original question')
    await render(createElement(QuestionComposer, { block, selector: localSelector }))
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    state.display = { ...state.display, status: 'cancelled', answerState: 'cancelled' }
    await render(createElement(QuestionComposer, { block: { ...block, status: 'cancelled' }, selector: localSelector }))
    expect(container.querySelector('textarea')).toBeNull()
    expect(container.querySelector('header h2')?.textContent).toBe('Synthetic original question')
    expect(button('userInputCancel')).toBeUndefined()
  })

  it('blocks protected selection transfer while preserving an ordinary focused editor', async () => {
    installPrivateDisplay()
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    const range = document.createRange(); range.selectNodeContents(container.querySelector('header h2')!)
    document.getSelection()!.addRange(range)
    const copy = new Event('copy', { bubbles: true, cancelable: true })
    document.dispatchEvent(copy); expect(copy.defaultPrevented).toBe(false)
    const editor = document.createElement('textarea'); document.body.append(editor); editor.focus()
    const ordinary = new Event('copy', { bubbles: true, cancelable: true })
    editor.dispatchEvent(ordinary); expect(ordinary.defaultPrevented).toBe(false)
    editor.remove()
  })
})

const nativeWitnessFs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
const nativeWitnessProcess = await vi.importActual<{ env: Record<string, string | undefined> }>('node:process')

describe('continued question private answer attempts', () => {
  const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  function continuedDisplay(): UserInputLocalDisplayV1 {
    const original = originalDisplay()
    return { ...original, status: 'continued', callId: 'actual-host-call', timedWait: timing,
      answerState: 'unavailable', questions: [{ ...original.questions[0], multiSelect: false, options: [] }] }
  }
  type ContinuedSseListener = Parameters<typeof window.analytix.runtime.onSseEvent>[0]
  type ContinuedSsePayload = Parameters<ContinuedSseListener>[0]
  function installContinued(initial = continuedDisplay()) {
    const fixture = installPrivateDisplay(initial), listeners = new Set<ContinuedSseListener>()
    const onSseEvent = vi.fn((listener: ContinuedSseListener) => { listeners.add(listener); return () => { listeners.delete(listener) } })
    Object.assign(fixture.api, { onSseEvent })
    const emit = async (payload: ContinuedSsePayload) => { await act(async () => {
      for (const listener of [...listeners]) listener(payload)
      for (let index = 0; index < 20; index++) await Promise.resolve()
    }) }
    return { ...fixture, listeners, onSseEvent, emit,
      refresh: async () => emit({ streamId: 'synthetic-stream', events: [{ kind: 'turn_steered', threadId: localSelector.threadId }] }) }
  }
  const block = (): Extract<ChatBlock, { kind: 'user_input' }> => ({ ...projectedBlock('continued'), timedWait: timing })
  const queuedId = '67eeac24-98da-4a49-a4ee-1d5b54574999'
  function queuedDisplay(): UserInputLocalDisplayV1 {
    return { ...continuedDisplay(), answerState: 'available',
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic queued answer' }] },
      lateReply: { clientUserMessageId: queuedId, turnId: 'old-queued-target', state: 'queued' } }
  }
  // Local fixtures exercise the production public projector/IPC schema. They
  // are not signed Main transport or a native delivery acceptance claim.
  function closedPayload(event: Record<string, unknown>): ContinuedSsePayload {
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    const frame = `id: ${event.seq}\nevent: ${event.kind}\ndata: ${JSON.stringify(event)}`
    const projected = projectPublicRuntimeSseBlock(frame, String(event.threadId), new PublicRuntimeEventFilter())
    expect(projected?.status).toBe('emit')
    if (!projected || projected.status !== 'emit') throw new Error('Synthetic closed event did not cross the production SSE projector')
    const payload = { streamId: 'native-hookup-fixture', events: [projected.event] }
    expect(isStrictPublicRuntimeSseIpcPayload(payload)).toBe(true)
    return payload
  }
  function promotionEvent(): Record<string, unknown> {
    const witnessPath = nativeWitnessProcess.env.ANALYTIX_NATIVE_HOOKUP_EVENT_WITNESS
    if (witnessPath) {
      const witness: unknown = JSON.parse(nativeWitnessFs.readFileSync(witnessPath, 'utf8'))
      if (!witness || typeof witness !== 'object' || Array.isArray(witness)) throw new Error('Native event witness must be an object')
      expect(Object.keys(witness).sort()).toEqual(['expected', 'projected', 'promotion', 'rawBuilder'])
      const event = (witness as { promotion?: unknown }).promotion
      if (!event || typeof event !== 'object' || Array.isArray(event)) throw new Error('Native event witness has no closed promotion event')
      // Preserve the real witness thread/turn/UUID verbatim. DTO owner fields
      // below are adapted to the event, never the other way around.
      expect(isClosedPublicRuntimeSseEvent(event as Record<string, unknown>)).toBe(true)
      return event as Record<string, unknown>
    }
    const timestamp = '2026-10-09T00:00:00Z', threadId = 'synthetic-native-hookup-thread', turnId = 'turn-idle-wake', itemId = 'host-promoted-user'
    return { kind: 'item_created', seq: 17, timestamp, threadId, turnId, itemId,
      item: { id: itemId, threadId, turnId, role: 'user', status: 'completed', kind: 'user_message', text: 'Safe public promoted answer',
        clientUserMessageId: queuedId, delivery: 'steer', createdAt: timestamp, finishedAt: timestamp } }
  }
  function promotionOwner(event: Record<string, unknown>) {
    expect(event.kind).toBe('item_created')
    const item = event.item as Record<string, unknown>
    expect(item.kind).toBe('user_message'); expect(item.role).toBe('user'); expect(item.status).toBe('completed')
    if (typeof event.threadId !== 'string' || typeof event.turnId !== 'string' || typeof event.seq !== 'number' ||
        typeof item.clientUserMessageId !== 'string') throw new Error('Native promotion identity is incomplete')
    const selector = { ...localSelector, threadId: event.threadId, turnId: 'turn-original-question-before-promotion' }
    const display: UserInputLocalDisplayV1 = { ...queuedDisplay(), ...selector,
      lateReply: { clientUserMessageId: item.clientUserMessageId, turnId: 'old-queued-target', state: 'queued' } }
    const input = { ...block(), meta: { turnId: selector.turnId } }
    return { selector, display, input, turnId: event.turnId, clientUserMessageId: item.clientUserMessageId, seq: event.seq }
  }
  const terminalUsage = () => ({ promptTokens: 2, completionTokens: 1, reasoningTokens: 0, totalTokens: 3,
    cacheHitRate: null, cacheableTokenHitRate: null, totalInputTokenHitRate: null, cacheMissReasons: [], cacheSuggestions: [],
    costUsd: 0, costCny: 0, priceConfigured: false, cacheSavingsUsd: 0, cacheSavingsCny: 0, tokenEconomySavingsTokens: 0, turns: 1 })
  function terminalOuter(kind: 'general_terminal_batch' | 'accepted_final_batch'): Record<string, unknown> {
    const threadId = localSelector.threadId, turnId = 'turn-terminal-delivery', timestamp = '2026-10-09T00:00:00Z'
    const common = { threadId, turnId, timestamp }
    if (kind === 'general_terminal_batch') return { ...common, kind, schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1', batchDigest: 'a'.repeat(64), seq: 13, firstSeq: 12, lastSeq: 13,
      generalTerminalCommitId: 'b'.repeat(64), generalTerminalAuthorityKind: 'general_terminal_cas', generalTerminalAuthorityDigest: 'c'.repeat(64),
      eventManifestDigest: 'd'.repeat(64), projectedEventsDigest: 'e'.repeat(64), transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false, citationAuthority: false, factAnswerAllowed: false,
      events: [{ ...common, kind: 'usage', seq: 12, model: 'synthetic-model', usage: terminalUsage(), cacheDiagnostics: {}, usageFinalStatus: 'completed' },
        { ...common, kind: 'turn_completed', seq: 13, status: 'completed', terminalReason: 'success' }],
      eventManifest: [{ slot: 'usage', eventId: '1'.repeat(64), payloadDigest: '2'.repeat(64) },
        { slot: 'terminal', eventId: '3'.repeat(64), payloadDigest: '4'.repeat(64) }] }
    const digest = 'a'.repeat(64), batchId = 'b'.repeat(64), eventManifestDigest = 'c'.repeat(64)
    const publication = { acceptedFinalDigest: digest, publicationCommitId: digest }
    const view = { schemaVersion: 3, acceptedFinalDigest: digest, publicationState: 'accepted', variant: 'GeneralGuidanceAnswer',
      terminalReason: 'success', blockerCode: '', coverageStatus: 'guidance_only', checkedScopeDigest: '', missingScopeCount: 0,
      claimCount: 0, claimTypes: [], receiptMetadata: { projection: 'masked_metadata_only', count: 0, setDigest: 'd'.repeat(64), citations: [] },
      noHitWording: '', acceptedAt: timestamp }
    return { ...common, kind, schemaVersion: 2, purpose: 'analytix.accepted-final-delivery-batch/v2', batchId,
      seq: 13, firstSeq: 11, lastSeq: 13, publicationCommitId: digest, eventManifestDigest,
      publicationAuthority: { ...common, schemaVersion: 'accepted-final-delivery-seal.v1', purpose: 'analytix.accepted-final-delivery-seal/v1',
        sealId: 'd'.repeat(64), publicationCommitId: digest, acceptedFinalDispositionDigest: 'e'.repeat(64), terminalDispositionId: 'f'.repeat(64),
        eventManifestDigest, sequencedEventsDigest: '1'.repeat(64), batchId, firstSeq: 11, lastSeq: 13,
        authorityAlgorithm: 'Ed25519', authorityKeyId: '2'.repeat(64), authorityPublicKey: 'A'.repeat(43), authoritySignature: 'A'.repeat(86) },
      events: [{ ...common, ...publication, kind: 'item_completed', seq: 11, itemId: 'synthetic-final',
        item: { id: 'synthetic-final', threadId, turnId, role: 'assistant', status: 'completed', kind: 'assistant_text',
          text: 'Synthetic guidance', createdAt: timestamp, finishedAt: timestamp, acceptedFinalView: view },
        publicationEventId: '3'.repeat(64), publicationSlot: 'assistant-final', publicationPayloadDigest: '4'.repeat(64) },
        { ...common, ...publication, kind: 'usage', seq: 12, model: 'synthetic-model', usage: terminalUsage(), cacheDiagnostics: {},
          usageFinalStatus: 'completed', publicationEventId: '5'.repeat(64), publicationSlot: 'usage', publicationPayloadDigest: '6'.repeat(64) },
        { ...common, ...publication, kind: 'turn_completed', seq: 13, status: 'completed', terminalReason: 'success',
          publicationEventId: '7'.repeat(64), publicationSlot: 'terminal', publicationPayloadDigest: '8'.repeat(64) }] }
  }
  it('routes completed user_message item_created through the real public projector, mapper and main store while acceptance remains private Host state', async () => {
    const event = promotionEvent(), payload = closedPayload(event), owner = promotionOwner(event)
    const previousState = useChatStore.getState(), fixture = installContinued(owner.display)
    const lateSubmit = vi.fn(), discard = vi.fn()
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    useChatStore.setState({ activeThreadId: owner.selector.threadId, route: 'chat', blocks: [owner.input], busy: false,
      currentTurnId: null, currentTurnUserId: null, lastSeq: owner.seq - 1, rewoundTurnIds: {}, liveAssistant: '', turnUsageById: {} })
    const sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId: owner.selector.threadId })
    try {
      await render(createElement(QuestionComposer, { block: owner.input, selector: owner.selector, onLateSubmit: lateSubmit }))
      expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
      const reads = fixture.api.openUserInputLocalDisplay.mock.calls.length
      await act(async () => { await dispatchAnalytixRuntimeEvents(payload.events, sink, async () => {}) })
      const user = useChatStore.getState().blocks.find(candidate => candidate.kind === 'user' && candidate.id === event.itemId)
      expect(user?.kind === 'user' ? user.meta : undefined).toMatchObject({ clientUserMessageId: owner.clientUserMessageId, turnId: owner.turnId, steeringStatus: 'accepted' })
      await fixture.emit(payload)
      expect(fixture.api.openUserInputLocalDisplay.mock.calls.length).toBeGreaterThan(reads)
      expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
      expect(container.textContent).not.toContain(i18n.t('common:userInputLateAccepted'))
      expect(button('userInputLateDiscard')).toBeDefined()
      const queuedReads = fixture.api.openUserInputLocalDisplay.mock.calls.length
      fixture.state.display = { ...owner.display, lateReply: { clientUserMessageId: owner.clientUserMessageId, turnId: owner.turnId, state: 'accepted' } }
      expect(fixture.api.openUserInputLocalDisplay).toHaveBeenCalledTimes(queuedReads)
      await fixture.emit(payload)
      expect(container.textContent).toContain(i18n.t('common:userInputLateAccepted'))
      expect(button('userInputLateDiscard')).toBeUndefined(); expect(button('userInputLateSubmit')).toBeUndefined()
      expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic queued answer')
      expect(fixture.api.openUserInputLocalDisplay.mock.calls.every(([request]) => request.threadId === owner.selector.threadId && request.turnId === owner.selector.turnId && request.itemId === owner.selector.itemId && request.requestId === owner.selector.requestId)).toBe(true)
      expect(owner.turnId).not.toBe(owner.selector.turnId); expect(owner.turnId).not.toBe('old-queued-target')
      expect(lateSubmit).not.toHaveBeenCalled(); expect(discard).not.toHaveBeenCalled()
    } finally {
      await act(async () => root.render(null))
      expect(fixture.listeners.size).toBe(0)
      clearBusyWatchdog(); clearActiveStream(owner.selector.threadId)
      useChatStore.setState(previousState)
    }
  })
  it.each(['general_terminal_batch', 'accepted_final_batch'] as const)('refreshes queued private state from one complete %s outer notification without relying on a detached terminal suffix', async kind => {
    const event = terminalOuter(kind), payload = closedPayload(event), fixture = installContinued(queuedDisplay())
    const props = { block: block(), selector: localSelector }
    await render(createElement(QuestionComposer, props))
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
    const reads = fixture.api.openUserInputLocalDisplay.mock.calls.length
    fixture.state.display = { ...queuedDisplay(), lateReply: { clientUserMessageId: queuedId, turnId: 'turn-terminal-delivery', state: 'accepted' } }
    expect(fixture.api.openUserInputLocalDisplay).toHaveBeenCalledTimes(reads)
    expect(payload.events).toHaveLength(1); expect(payload.events[0].kind).toBe(kind)
    await fixture.emit(payload)
    expect(fixture.api.openUserInputLocalDisplay.mock.calls.length).toBeGreaterThan(reads)
    expect(container.textContent).toContain(i18n.t('common:userInputLateAccepted'))
    expect(button('userInputLateDiscard')).toBeUndefined(); expect(button('userInputLateSubmit')).toBeUndefined()
    await act(async () => root.render(null)); expect(fixture.listeners.size).toBe(0)
  })
  it('does not reopen private authority after revocation even when a valid closed promotion is broadcast', async () => {
    const event = promotionEvent(), payload = closedPayload(event), owner = promotionOwner(event), fixture = installContinued(owner.display)
    await render(createElement(QuestionComposer, { block: owner.input, selector: owner.selector }))
    await act(async () => fixture.invalidate())
    const reads = fixture.api.openUserInputLocalDisplay.mock.calls.length
    fixture.state.display = { ...owner.display, lateReply: { clientUserMessageId: owner.clientUserMessageId, turnId: owner.turnId, state: 'accepted' } }
    await fixture.emit(payload)
    expect(fixture.api.openUserInputLocalDisplay).toHaveBeenCalledTimes(reads)
    expect(container.textContent).not.toContain('Synthetic queued answer')
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).not.toBe('Synthetic queued answer')
    expect(container.textContent).not.toContain(i18n.t('common:userInputLateAccepted'))
    expect(button('userInputLateDiscard')).toBeUndefined()
    await act(async () => root.render(null)); expect(fixture.listeners.size).toBe(0)
  })
  it('discards the real queued receipt after remount and waits for private Host readback before allowing a new UUID', async () => {
    const fixture = installContinued(queuedDisplay()), discard = vi.fn(async () => ({ schemaVersion: 1, ...localSelector, status: 'continued', lateReply: { clientUserMessageId: queuedId, turnId: 'old-queued-target', state: 'discarded' } }))
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    const submit = vi.fn<(answers: UserInputSubmission[], clientUserMessageId: string, expectedTurnId: string) => Promise<void>>(async () => {})
    const props = { block: block(), selector: localSelector, lateReplyTargetTurnId: 'new-current-target', onLateSubmit: submit }
    await render(createElement(QuestionComposer, props))
    await act(async () => button('userInputLateDiscard').click())
    expect(discard).toHaveBeenCalledWith({ ...localSelector, action: 'discardLateReply', clientUserMessageId: queuedId, expectedTurnId: 'old-queued-target' })
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
    expect(container.querySelector<HTMLFieldSetElement>('fieldset')?.disabled).toBe(true)
    expect(button('userInputLateSubmit')).toBeUndefined(); expect(submit).not.toHaveBeenCalled()
    fixture.state.display = { ...queuedDisplay(), lateReply: { clientUserMessageId: queuedId, turnId: 'old-queued-target', state: 'discarded' } }
    await fixture.refresh()
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic queued answer')
    await typeAnswer('Synthetic replacement answer'); await act(async () => button('userInputLateSubmit').click())
    expect(submit.mock.calls[0]).toEqual([[{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic replacement answer' }], expect.any(String), 'new-current-target'])
    expect(submit.mock.calls[0][1]).not.toBe(queuedId); expect(fixture.api.runtimeRequest).not.toHaveBeenCalled()
  })
  it.each(['queued', 'accepted'] as const)('refreshes actual %s state after a discard conflict and never fabricates cancellation', async state => {
    const fixture = installContinued(queuedDisplay()), discard = vi.fn(async () => { fixture.state.display = { ...queuedDisplay(), lateReply: { clientUserMessageId: queuedId, turnId: 'old-queued-target', state } }; throw new Error('Synthetic 409 conflict') })
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'new-current-target' }))
    await act(async () => button('userInputLateDiscard').click())
    expect(container.textContent).toContain(i18n.t('common:' + (state === 'queued' ? 'userInputLateQueued' : 'userInputLateAccepted')))
    expect(button('userInputLateSubmit')).toBeUndefined()
    expect(Boolean(button('userInputLateDiscard'))).toBe(state === 'queued')
  })
  it('locks repeated discard clicks and withholds stale completion after real privacy invalidation', async () => {
    const fixture = installContinued(queuedDisplay()); let finish!: () => void
    const discard = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector }))
    const action = button('userInputLateDiscard')
    await act(async () => { action.click(); action.click() }); expect(discard).toHaveBeenCalledOnce()
    await act(async () => fixture.invalidate()); const opens = fixture.api.openUserInputLocalDisplay.mock.calls.length
    await act(async () => finish())
    expect(fixture.api.openUserInputLocalDisplay).toHaveBeenCalledTimes(opens)
    expect(container.textContent).not.toContain('Synthetic queued answer'); expect(button('userInputLateDiscard')).toBeUndefined()
  })
  it.each(['readOnly', 'presented'] as const)('blocks queued discard while %s disallows this product surface', async guard => {
    const fixture = installContinued(queuedDisplay()), discard = vi.fn()
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, [guard]: guard === 'presented' ? false : true }))
    const action = button('userInputLateDiscard'); if (action) { expect(action.disabled).toBe(true); await act(async () => action.click()) }
    expect(discard).not.toHaveBeenCalled()
  })
  it('allows queued discard after actual continued even while the old foreground action is resolving', async () => {
    const fixture = installContinued(queuedDisplay())
    const discard = vi.fn(async () => ({ schemaVersion: 1, ...localSelector, status: 'continued', lateReply: { clientUserMessageId: queuedId, turnId: 'old-queued-target', state: 'discarded' } }))
    Object.assign(fixture.api, { discardUserInputLateReply: discard })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, resolving: true }))
    const action = button('userInputLateDiscard'); expect(action.disabled).toBe(false)
    await act(async () => action.click())
    expect(discard).toHaveBeenCalledOnce()
    expect(discard).toHaveBeenCalledWith({ ...localSelector, action: 'discardLateReply', clientUserMessageId: queuedId, expectedTurnId: 'old-queued-target' })
    expect(container.textContent).toContain(i18n.t('common:userInputLateQueued'))
  })
  it('keeps the same UUID, raw answer and target turn after an unknown failure even when the running turn changes', async () => {
    installContinued(); const submit = vi.fn<(answers: UserInputSubmission[], clientUserMessageId: string, expectedTurnId: string) => Promise<void>>(async () => { throw new Error('Synthetic uncertain transport') })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'turn-target-A', onLateSubmit: submit }))
    await typeAnswer('Synthetic late answer'); await act(async () => button('userInputLateSubmit').click())
    const first = submit.mock.calls[0]
    expect(first).toEqual([[{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic late answer' }], expect.any(String), 'turn-target-A'])
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'turn-target-B', onLateSubmit: submit }))
    await act(async () => button('userInputLateSubmit').click())
    expect(submit.mock.calls[1]).toEqual(first)
    await typeAnswer('Synthetic changed uncertain answer'); await act(async () => button('userInputLateSubmit').click())
    expect(submit).toHaveBeenCalledTimes(2)
  })
  it('reads the idle Host anchor only once and keeps exact UUID, answer and anchor for uncertain retry', async () => {
    installContinued()
    const baseline = vi.fn(async () => 'turn-host-terminal-anchor')
    const submit = vi.fn<(answers: UserInputSubmission[], clientUserMessageId: string, expectedTurnId: string) => Promise<void>>(async () => { throw new Error('Synthetic uncertain transport') })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, getLateReplyExpectedTurnId: baseline, onLateSubmit: submit }))
    await typeAnswer('Synthetic idle answer'); await act(async () => button('userInputLateSubmit').click())
    expect(submit.mock.calls[0][2]).toBe('turn-host-terminal-anchor'); expect(baseline).toHaveBeenCalledOnce()
    await act(async () => button('userInputLateSubmit').click())
    expect(submit.mock.calls[1]).toEqual(submit.mock.calls[0]); expect(baseline).toHaveBeenCalledOnce()
  })
  it.each(['hidden', 'revoked', 'readonly', 'unmounted', 'readonly-return', 'hidden-return'] as const)('does not submit after an in-flight idle anchor read becomes %s', async disposition => {
    const fixture = installContinued(); let finish!: (value: string) => void
    const baseline = vi.fn(() => new Promise<string>(resolve => { finish = resolve })), submit = vi.fn(async () => {})
    const props = { block: block(), selector: localSelector, getLateReplyExpectedTurnId: baseline, onLateSubmit: submit }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic idle answer')
    await act(async () => button('userInputLateSubmit').click()); expect(baseline).toHaveBeenCalledOnce()
    if (disposition === 'revoked') await act(async () => fixture.invalidate())
    else if (disposition === 'unmounted') await render(createElement('div'))
    else {
      await render(createElement(QuestionComposer, { ...props, presented: !disposition.startsWith('hidden'), readOnly: disposition.startsWith('readonly') }))
      if (disposition.endsWith('return')) await render(createElement(QuestionComposer, props))
    }
    await act(async () => finish('turn-host-terminal-anchor')); expect(submit).not.toHaveBeenCalled()
  })
  it('allows a fresh UUID and edited answer only after Host confirms that exact attempt was discarded' , async () => {
    const fixture = installContinued(), submit = vi.fn<(answers: UserInputSubmission[], clientUserMessageId: string, expectedTurnId: string) => Promise<void>>(async () => { throw new Error('Synthetic uncertain transport') })
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'turn-target-A', onLateSubmit: submit }))
    await typeAnswer('Synthetic first answer'); await act(async () => button('userInputLateSubmit').click())
    const firstId = submit.mock.calls[0][1] as string
    fixture.state.display = { ...continuedDisplay(), answerState: 'available',
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic first answer' }] },
      lateReply: { clientUserMessageId: firstId, turnId: 'turn-target-A', state: 'discarded' } }
    await fixture.refresh(); await typeAnswer('Synthetic corrected answer'); await act(async () => button('userInputLateSubmit').click())
    expect(submit).toHaveBeenCalledTimes(2); expect(submit.mock.calls[1][1]).not.toBe(firstId)
    expect(submit.mock.calls[1][0]).toEqual([{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic corrected answer' }])
  })
  it('allows a new attempt after definite unadmitted rejection but not after a generic error', async () => {
    installContinued(); const submit = vi.fn().mockRejectedValueOnce(Object.assign(new Error('Synthetic size rejection'), { userInputLateReplyUnadmitted: true })).mockResolvedValueOnce(undefined)
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'turn-target-A', onLateSubmit: submit }))
    await typeAnswer('Synthetic oversized answer'); await act(async () => button('userInputLateSubmit').click())
    const firstId = submit.mock.calls[0][1]
    await typeAnswer('Synthetic bounded answer'); await act(async () => button('userInputLateSubmit').click())
    expect(submit.mock.calls[1][1]).not.toBe(firstId)
  })
  it.each(['queued', 'accepted'] as const)('reviews Host %s privately without a second writable answer or fabricated user bubble', async state => {
    const source = continuedDisplay(), fixture = installContinued({ ...source, answerState: 'available',
      submission: { schemaVersion: 1, format: 'structured', answers: [{ id: localRequestId + '_1', kind: 'answer', optionIds: [], customText: 'Synthetic submitted late answer' }] },
      lateReply: { clientUserMessageId: '67eeac24-98da-4a49-a4ee-1d5b54574999', turnId: 'turn-target', state } })
    const submit = vi.fn(async () => {})
    await render(createElement(QuestionComposer, { block: block(), selector: localSelector, lateReplyTargetTurnId: 'turn-target', onLateSubmit: submit }))
    expect(container.querySelector<HTMLFieldSetElement>('fieldset')?.disabled).toBe(true)
    expect(container.textContent).toContain('Synthetic submitted late answer')
    expect(button('userInputLateSubmit')).toBeUndefined(); expect(container.querySelector('[data-question-reply-item-id]')).toBeNull()
    expect(fixture.api.runtimeRequest).not.toHaveBeenCalled(); expect(submit).not.toHaveBeenCalled()
  })
})

describe('named hide preserves only the same private leaf draft', () => {
  it.each(['saved', 'capacity'] as const)('dismisses immediately and keeps the same latest child chain across delayed %s receipt', async outcome => {
    const source = singleDraftDisplay(); source.callId = 'actual-question-call'
    const fixture = installPrivateDisplay(source), hide = vi.fn(), submit = vi.fn(async () => {})
    const finish: Array<(value: { ok: boolean; receipt?: unknown; code?: string }) => void> = []
    const save = vi.fn((request: UserInputLocalSaveDraftRequestV1) => new Promise(resolve => {
      finish.push(result => {
        if (result.ok) fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
        resolve(result)
      })
    }))
    Object.assign(fixture.api, { saveUserInputDraft: save })
    const props = { block: projectedBlock(), selector: localSelector, onHide: hide, onSubmit: submit }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic first')
    const first = save.mock.calls[0][0]
    await typeAnswer('Synthetic latest'); await act(async () => button('userInputHide').click())
    expect(hide).toHaveBeenCalledOnce(); expect(container.querySelector('section')?.getAttribute('aria-busy')).toBe('false')
    await render(createElement(QuestionComposer, { ...props, presented: false }))
    expect(save).toHaveBeenCalledOnce(); expect(container.textContent).not.toContain(source.questions[0].question)
    const receipt = (request: UserInputLocalSaveDraftRequestV1) => ({ ok: true, receipt: { schemaVersion: 1, ...localSelector,
      status: 'pending', draftRevision: request.revision, draft: request.draft } })
    await act(async () => finish[0](receipt(first)))
    expect(save).toHaveBeenCalledTimes(2)
    const child = save.mock.calls[1][0]
    expect(child.expectedRevision).toBe(first.revision); expect(child.draft.answers[0].customText).toBe('Synthetic latest')
    await act(async () => finish[1](outcome === 'saved' ? receipt(child) : { ok: false, code: 'capacity' }))
    expect(hide).toHaveBeenCalledOnce(); await render(createElement(QuestionComposer, props))
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic latest')
    if (outcome === 'capacity') expect(container.querySelector('[role="alert"]')?.textContent).toContain(i18n.t('common:userInputDraftCapacity'))
    expect(submit).not.toHaveBeenCalled(); expect(fixture.api.runtimeRequest).not.toHaveBeenCalled()
  })
  it('closes the raw Host view on hide, keeps the user draft in this leaf and reads a fresh original on reveal', async () => {
    const fixture = installPrivateDisplay(), submit = vi.fn(async () => {})
    const props = { block: projectedBlock(), selector: localSelector, onSubmit: submit }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic retained private draft')
    const opens = fixture.api.openUserInputLocalDisplay.mock.calls.length
    await render(createElement(QuestionComposer, { ...props, presented: false }))
    expect(fixture.api.closeUserInputLocalDisplay).toHaveBeenCalled(); expect(container.textContent).not.toContain('Synthetic original question')
    await render(createElement(QuestionComposer, props))
    expect(fixture.api.openUserInputLocalDisplay.mock.calls.length).toBeGreaterThan(opens)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic retained private draft')
    expect(submit).not.toHaveBeenCalled()
  })
  it('still clears a hidden user draft after the real privacy invalidation', async () => {
    const fixture = installPrivateDisplay(), props = { block: projectedBlock(), selector: localSelector, onSubmit: vi.fn(async () => {}) }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic revoked private draft')
    await render(createElement(QuestionComposer, { ...props, presented: false })); await act(async () => fixture.invalidate())
    await render(createElement(QuestionComposer, props))
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(container.textContent).not.toContain('Synthetic revoked private draft')
  })
})


function capacitySelector(index: number): UserInputLocalSelectorV1 {
  const requestId = 'input_' + index.toString(16).padStart(64, '0')
  return { threadId: 'thread-capacity', turnId: 'turn-capacity', itemId: 'item_' + requestId, requestId }
}
function capacityDisplay(selector: UserInputLocalSelectorV1, status: 'pending' | 'continued' = 'pending'): UserInputLocalDisplayV1 {
  return { schemaVersion: 1, ...selector, callId: 'call-' + selector.requestId, status, answerState: status === 'pending' ? 'pending' : 'unavailable',
    timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' },
    questions: [{ id: selector.requestId + '_1', header: 'Synthetic capacity', question: 'Synthetic capacity question', multiSelect: false, options: [] }] }
}
function RawCapacityProbe({ index, status = 'pending' }: { index: number; status?: 'pending' | 'continued' }) {
  const local = useUserInputLocalDisplay(capacitySelector(index), status)
  return createElement('span', { 'data-raw-probe': index, 'data-phase': local.phase }, local.display?.callId ?? '')
}
function AssociationCapacityProbe({ index, status = 'pending' }: { index: number; status?: 'pending' | 'continued' }) {
  const selector = capacitySelector(index)
  const ids = useUserInputLocalAssociation(selector, status, [{ kind: 'tool', id: 'tool-' + index, turnId: selector.turnId, callId: 'call-' + selector.requestId }])
  return createElement('span', { 'data-association-probe': index }, ids.join(','))
}
function installCapacityBridge(status: 'pending' | 'continued' = 'pending') {
  const active = new Set<string>(), invalidators = new Set<(id: string | null) => void>(), events = new Set<Parameters<typeof window.analytix.runtime.onSseEvent>[0]>()
  let maxActive = 0
  const open = vi.fn(async (request: UserInputLocalViewRequestV1) => {
    active.add(request.viewId); maxActive = Math.max(maxActive, active.size)
    if (active.size > 32) throw new Error('Synthetic native capacity exceeded')
    const { viewId: _viewId, ...selector } = request
    return { ok: true as const, display: capacityDisplay(selector, status) }
  })
  const close = vi.fn(async (id: string) => { active.delete(id) })
  const api = { openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close,
    onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidators.add(listener); return () => { invalidators.delete(listener) } },
    onSseEvent: (listener: Parameters<typeof window.analytix.runtime.onSseEvent>[0]) => { events.add(listener); return () => { events.delete(listener) } }, onRuntimeStatus: () => () => undefined }
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: api } })
  return { active, open, close, maxActive: () => maxActive,
    revoke: () => { active.clear(); invalidators.forEach(listener => listener(null)) },
    steer: () => events.forEach(listener => listener({ streamId: 'capacity-fixture', events: [{ kind: 'turn_steered', threadId: 'thread-capacity' }] })) }
}
async function drawCapacity(raw: number[], associations: number[], status: 'pending' | 'continued' = 'pending') {
  await act(async () => root.render(createElement('div', null,
    ...raw.map(index => createElement(RawCapacityProbe, { key: 'raw-' + index, index, status })),
    ...associations.map(index => createElement(AssociationCapacityProbe, { key: 'association-' + index, index, status })))))
}
describe('private native lease capacity and revocation races', () => {
  it('keeps at most 31 raw plus one transient native view and retries queue overflow and raw capacity after confirmed release', async () => {
    const fixture = installCapacityBridge(), raw = Array.from({ length: 32 }, (_, index) => index + 1), associations = Array.from({ length: 40 }, (_, index) => index + 100)
    await drawCapacity(raw, associations)
    expect(fixture.maxActive()).toBeLessThanOrEqual(32)
    expect([...container.querySelectorAll('[data-association-probe]')].every(node => node.textContent?.startsWith('tool-'))).toBe(true)
    expect(container.querySelector('[data-raw-probe="32"]')?.getAttribute('data-phase')).toBe('unavailable')
    await drawCapacity(raw.slice(1), associations)
    expect(container.querySelector('[data-raw-probe="32"]')?.getAttribute('data-phase')).toBe('ready')
    expect(fixture.maxActive()).toBeLessThanOrEqual(32)
  })
  it('keeps uncertain close locked across leaf removal and resumes only after real Main all-view revocation', async () => {
    const fixture = installCapacityBridge(); fixture.close.mockRejectedValue(new Error('Synthetic close unknown'))
    await drawCapacity([], [1]); expect(fixture.open).toHaveBeenCalledOnce()
    await drawCapacity([], [2]); expect(fixture.open).toHaveBeenCalledOnce()
    await act(async () => fixture.revoke())
    fixture.close.mockImplementation(async id => { fixture.active.delete(id) })
    await drawCapacity([], [3])
    expect(container.querySelector('[data-association-probe="3"]')?.textContent).toBe('tool-3')
    expect(fixture.maxActive()).toBeLessThanOrEqual(32)
  })
  it('recovers 31 orphaned raw seats only after trusted Main revocation even after all private leaves unmount', async () => {
    const fixture = installCapacityBridge(), raw = Array.from({ length: 31 }, (_, index) => index + 1)
    fixture.close.mockRejectedValue(new Error('Synthetic raw close unknown'))
    await drawCapacity(raw, []); expect(fixture.active.size).toBe(31)
    await drawCapacity([], []); const opens = fixture.open.mock.calls.length
    await drawCapacity([32], []); expect(fixture.open).toHaveBeenCalledTimes(opens)
    expect(container.querySelector('[data-raw-probe="32"]')?.getAttribute('data-phase')).toBe('unavailable')
    await drawCapacity([], []); await act(async () => fixture.revoke())
    fixture.close.mockImplementation(async id => { fixture.active.delete(id) })
    await drawCapacity([33], [])
    expect(container.querySelector('[data-raw-probe="33"]')?.getAttribute('data-phase')).toBe('ready')
    expect(fixture.active.size).toBe(1); expect(fixture.maxActive()).toBeLessThanOrEqual(32)
  })
  it('unblocks a retired hanging open on trusted Main revocation without allowing its delayed result to publish', async () => {
    const fixture = installCapacityBridge()
    let complete!: (value: { ok: true; display: UserInputLocalDisplayV1 }) => void
    fixture.open.mockImplementationOnce(request => {
      fixture.active.add(request.viewId)
      return new Promise(resolve => { complete = resolve })
    })
    await drawCapacity([], [1]); await drawCapacity([], [])
    await act(async () => fixture.revoke()); await drawCapacity([], [2])
    expect(container.querySelector('[data-association-probe="2"]')?.textContent).toBe('tool-2')
    await act(async () => complete({ ok: true, display: capacityDisplay(capacitySelector(1)) }))
    expect(container.querySelector('[data-association-probe="2"]')?.textContent).toBe('tool-2')
  })
  it('honors privacy revoke after an SSE refresh was already queued in the same event batch', async () => {
    const fixture = installCapacityBridge('continued'); await drawCapacity([1], [2], 'continued')
    const calls = fixture.open.mock.calls.length
    await act(async () => { fixture.steer(); fixture.revoke() })
    expect(fixture.open).toHaveBeenCalledTimes(calls)
    expect(container.querySelector('[data-raw-probe="1"]')?.textContent).toBe('')
    expect(container.querySelector('[data-association-probe="2"]')?.textContent).toBe('')
  })
})


describe('confirmed private draft wait disposition', () => {
  const cursor = '33333333-3333-4333-8333-333333333333'
  const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  function waitingDisplay(wait?: 'waiting' | 'editing', text = '', recommended = false): UserInputLocalDisplayV1 {
    const source = originalDisplay(), first = source.questions[0]
    return { ...source, timedWait: timing, draftRevision: cursor,
      questions: [{ ...first, multiSelect: false, options: recommended ? [{ ...first.options[0], label: 'Original (Recommended)' }] : [] }],
      draft: { ...(wait === undefined ? {} : { wait }), currentPage: 0, answers: [{ questionId: first.id,
        optionIds: recommended ? [first.options[0].id!] : [], customText: text, skipped: false }] } }
  }
  function installWait(phase: UserInputWaitState['phase'] = 'counting') {
    const controls = { focus: vi.fn(), blur: vi.fn(), engage: vi.fn(), takeTime: vi.fn(), retry: vi.fn(), unsubscribe: vi.fn() }
    const observe = vi.spyOn(getProvider(), 'observeUserInputWait').mockImplementation((_selector, listener) => {
      listener({ phase, remainingMs: phase === 'counting' ? 30000 : null }); return controls
    })
    vi.spyOn(getProvider(), 'settleUserInputWait').mockImplementation(() => undefined)
    return { controls, observe }
  }
  function installSave(fixture: ReturnType<typeof installPrivateDisplay>) {
    const save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...localSelector, status: fixture.state.display.status === 'continued' ? 'continued' as const : 'pending' as const, draftRevision: request.revision, draft: request.draft } }
    })
    Object.assign(fixture.api, { saveUserInputDraft: save }); return save
  }
  it.each(['waiting', 'editing'] as const)('replays explicit %s once through actual controls without an initial draft write', async disposition => {
    const fixture = installPrivateDisplay(waitingDisplay(disposition, 'Restored original text')), save = installSave(fixture)
    const { controls } = installWait()
    const props = { block: { ...projectedBlock(), timedWait: timing }, selector: localSelector }
    await render(createElement(QuestionComposer, props)); await render(createElement(QuestionComposer, props))
    expect(controls[disposition === 'waiting' ? 'takeTime' : 'engage']).toHaveBeenCalledOnce()
    expect(controls[disposition === 'waiting' ? 'engage' : 'takeTime']).not.toHaveBeenCalled()
    expect(save).not.toHaveBeenCalled()
  })
  it('replays private-only timing after the confirmed draft restores before controls register', async () => {
    const fixture = installPrivateDisplay(waitingDisplay('waiting', 'Original private-only timing')), save = installSave(fixture)
    const { controls, observe } = installWait()
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    expect(observe).toHaveBeenCalledExactlyOnceWith(localSelector, expect.any(Function))
    expect(controls.takeTime).toHaveBeenCalledOnce(); expect(controls.engage).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it.each([false, true])('infers old edited drafts while preserving an unchanged recommended default: recommended=%s', async recommended => {
    installPrivateDisplay(waitingDisplay(undefined, recommended ? '' : 'Original edited text', recommended))
    const { controls } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(), timedWait: timing }, selector: localSelector }))
    expect(controls.engage).toHaveBeenCalledTimes(recommended ? 0 : 1)
    expect(controls.takeTime).not.toHaveBeenCalled()
  })
  it('keeps explicit waiting after an edit and saves the original wait alongside the new content', async () => {
    const fixture = installPrivateDisplay(waitingDisplay('waiting', 'Original draft')), save = installSave(fixture)
    const { controls } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(), timedWait: timing }, selector: localSelector }))
    await typeAnswer('New original draft')
    expect(controls.takeTime).toHaveBeenCalledOnce(); expect(controls.engage).not.toHaveBeenCalled()
    expect(save.mock.calls[0][0].draft).toMatchObject({ wait: 'waiting', answers: [{ customText: 'New original draft' }] })
  })
  it('saves Take time even without editing and keeps waiting on later edits', async () => {
    const source = waitingDisplay(); source.draft = null
    const fixture = installPrivateDisplay(source), save = installSave(fixture), { controls } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(), timedWait: timing }, selector: localSelector }))
    await act(async () => button('userInputTakeTime').click())
    expect(controls.takeTime).toHaveBeenCalledOnce(); expect(save.mock.calls[0][0].draft.wait).toBe('waiting')
    await typeAnswer('Take time kept'); expect(controls.engage).not.toHaveBeenCalled()
  })
  it.each(['readOnly', 'continued'] as const)('does not replay persisted waiting from a %s surface', async surface => {
    const source = waitingDisplay('waiting', 'Original draft')
    if (surface === 'continued') { source.status = 'continued'; source.answerState = 'unavailable' }
    const fixture = installPrivateDisplay(source), save = installSave(fixture), { controls, observe } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(surface === 'continued' ? 'continued' : 'pending'), timedWait: timing }, selector: localSelector, readOnly: surface === 'readOnly' }))
    expect(controls.takeTime).not.toHaveBeenCalled(); expect(controls.engage).not.toHaveBeenCalled(); expect(observe).not.toHaveBeenCalled(); expect(save).not.toHaveBeenCalled()
  })
  it('preserves continued waiting metadata during late editing without observing or replaying a business wait', async () => {
    const source = waitingDisplay('waiting', 'Original late draft'); source.status = 'continued'; source.answerState = 'unavailable'
    const fixture = installPrivateDisplay(source), save = installSave(fixture), { controls, observe } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock('continued'), timedWait: timing }, selector: localSelector,
      lateReplyTargetTurnId: 'actual-running-turn', onLateSubmit: vi.fn(async () => undefined) }))
    await typeAnswer('Edited original late draft')
    expect(save.mock.calls[0][0].draft.wait).toBe('waiting')
    expect(observe).not.toHaveBeenCalled(); expect(controls.takeTime).not.toHaveBeenCalled(); expect(controls.engage).not.toHaveBeenCalled()
  })
  it('shows actual unknown-after-restart continued without a revived wait or timeout claim', async () => {
    const source = waitingDisplay('waiting', 'Original draft')
    source.status = 'continued'; source.answerState = 'unavailable'; source.timedWait = { timeoutSeconds: -1, deadlineAt: null }; source.continuedReason = 'user_input_outcome_unknown_after_restart'
    installPrivateDisplay(source); const { controls, observe } = installWait()
    await render(createElement(QuestionComposer, { block: { ...projectedBlock('continued'), timedWait: source.timedWait }, selector: localSelector }))
    expect(container.textContent).toContain(i18n.t('common:userInputContinuedUnknown'))
    expect(container.textContent).not.toContain(i18n.t('common:userInputContinued'))
    expect(observe).not.toHaveBeenCalled(); expect(controls.takeTime).not.toHaveBeenCalled()
  })
})

// Normal Composer editing uses the same private Host contract as a remount.
// These synthetic readbacks exercise user actions, never a product injection API.
describe('actual private Composer unfinished-draft producer', () => {
  const cursor = '33333333-3333-4333-8333-333333333333'
  async function flushAnswerSurface() {
    const outside = document.createElement('button'); document.body.append(outside)
    try {
      await act(async () => container.querySelector<HTMLTextAreaElement>('textarea')!.focus())
      await act(async () => outside.focus())
    } finally { outside.remove() }
  }
  it.each([true, false])('restores actual Host page and partial content, including an empty snapshot (empty=%s)', async empty => {
    const source = originalDisplay()
    source.questions[0].options[0].label += ' (Recommended)'
    source.draftRevision = cursor
    source.draft = { currentPage: 1, answers: empty ? [] : [{ questionId: source.questions[1].id, optionIds: [], customText: '  partial original\n  ', skipped: false }] }
    const fixture = installPrivateDisplay(source), save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...localSelector, status: 'pending' as const, draftRevision: request.revision, draft: request.draft } }
    })
    Object.assign(fixture.api, { saveUserInputDraft: save })
    const block = projectedBlock(), before = JSON.stringify(block)
    vi.useFakeTimers()
    try {
      await render(createElement(QuestionComposer, { block, selector: localSelector, onSubmit: vi.fn(async () => {}) }))
      expect(container.querySelector('h2')?.textContent).toBe(source.questions[1].question)
      expect(container.querySelector<HTMLTextAreaElement>('textarea')!.value).toBe(empty ? '' : '  partial original\n  ')
      await act(async () => { await vi.advanceTimersByTimeAsync(3001) })
      expect(save).not.toHaveBeenCalled()
      await act(async () => pager('Previous').click())
      expect(optionInput(source.questions[0].options[0].id!).checked).toBe(false)
      await act(async () => { await vi.advanceTimersByTimeAsync(751) })
      expect(save).toHaveBeenCalledOnce(); expect(save.mock.calls[0][0].draft.currentPage).toBe(0)
      expect(save.mock.calls[0][0].expectedRevision).toBe(cursor)
      expect(JSON.stringify(block)).toBe(before)
    } finally { vi.useRealTimers() }
  })
  it('saves original unfinished text and retained-null cursor on actual edit and blur', async () => {
    const source = singleDraftDisplay(); source.draftRevision = cursor
    const fixture = installPrivateDisplay(source)
    const save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...localSelector, status: 'pending' as const, draftRevision: request.revision, draft: request.draft } }
    })
    Object.assign(fixture.api, { saveUserInputDraft: save })
    const block = projectedBlock(), before = JSON.stringify(block)
    await render(createElement(QuestionComposer, { block, selector: localSelector, onSubmit: vi.fn(async () => {}) }))
    expect(save).not.toHaveBeenCalled()
    await typeAnswer('  unfinished original\n  '); await flushAnswerSurface()
    expect(save).toHaveBeenCalledOnce()
    expect(save.mock.calls[0][0]).toEqual({ ...localSelector, action: 'saveDraft', expectedRevision: cursor, revision: expect.any(String),
      draft: { wait: 'editing', currentPage: 0, answers: [{ questionId: source.questions[0].id, optionIds: [], customText: '  unfinished original\n  ', skipped: false }] } })
    expect(save.mock.calls[0][0].revision).not.toBe(cursor)
    expect(fixture.api.runtimeRequest).not.toHaveBeenCalled(); expect(JSON.stringify(block)).toBe(before)
  })
  it('captures the auto-advanced real page with the selected Host option identity', async () => {
    const source = originalDisplay(); source.questions[0].multiSelect = false; source.draftRevision = null; source.draft = null
    const fixture = installPrivateDisplay(source)
    let finish!: (value: unknown) => void
    const save = vi.fn((_request: UserInputLocalSaveDraftRequestV1) => new Promise(resolve => { finish = resolve }))
    Object.assign(fixture.api, { saveUserInputDraft: save })
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    await act(async () => optionInput(source.questions[0].options[0].id!).click())
    expect(container.querySelector('h2')?.textContent).toBe(source.questions[1].question)
    await flushAnswerSurface()
    expect(save).toHaveBeenCalledOnce()
    expect(save.mock.calls[0][0].draft).toEqual({ wait: 'editing', currentPage: 1, answers: [
      { questionId: source.questions[0].id, optionIds: [source.questions[0].options[0].id!], customText: '', skipped: false },
      { questionId: source.questions[1].id, optionIds: [], customText: '', skipped: false }
    ] })
    await act(async () => finish({ ok: false, code: 'capacity' }))
  })
  it.each(['capacity', 'pending'] as const)('valid answer submission proceeds independently of a %s draft save', async outcome => {
    const source = singleDraftDisplay(), fixture = installPrivateDisplay(source), submit = vi.fn(async () => undefined)
    let finish!: (value: unknown) => void
    const save = vi.fn((_request: UserInputLocalSaveDraftRequestV1) => outcome === 'capacity'
      ? Promise.resolve({ ok: false, code: 'capacity' }) : new Promise(resolve => { finish = resolve }))
    Object.assign(fixture.api, { saveUserInputDraft: save })
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector, onSubmit: submit }))
    await typeAnswer('  validated answer  '); await flushAnswerSurface()
    expect(save).toHaveBeenCalledOnce()
    if (outcome === 'capacity') expect(container.querySelector('[role="alert"]')?.textContent).toContain(i18n.t('common:userInputDraftCapacity'))
    expect(button('userInputSubmit').disabled).toBe(false)
    await act(async () => button('userInputSubmit').click())
    expect(submit).toHaveBeenCalledWith([{ id: source.questions[0].id, kind: 'answer', optionIds: [], customText: '  validated answer  ' }])
    if (outcome === 'pending') await act(async () => finish({ ok: false, code: 'retired' }))
    expect(fixture.api.runtimeRequest).not.toHaveBeenCalled()
  })
})

describe('foreground action retirement into the real continued composer', () => {
  it.each(['submit', 'cancel'] as const)('retires pending %s lock without letting the old ACK clear the late draft', async kind => {
    const source = originalDisplay()
    source.questions = [{ ...source.questions[0], multiSelect: false, options: [] }]
    source.callId = null
    const fixture = installPrivateDisplay(source)
    let finish!: () => void, finishLate!: () => void
    const foreground = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const late = vi.fn(() => new Promise<void>(resolve => { finishLate = resolve }))
    const props = { block: projectedBlock(), selector: localSelector, onSubmit: foreground, onCancel: foreground, onLateSubmit: late, lateReplyTargetTurnId: 'turn-running' }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic retained draft')
    await act(async () => button(kind === 'submit' ? 'userInputSubmit' : 'userInputCancel').click())
    expect(foreground).toHaveBeenCalledOnce()
    fixture.state.display = { ...source, status: 'continued', answerState: 'unavailable', timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' } }
    await render(createElement(QuestionComposer, { ...props, block: projectedBlock('continued'), resolving: true }))
    expect(button('userInputLateSubmit').disabled).toBe(false)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic retained draft')
    await act(async () => button('userInputLateSubmit').click())
    expect(late).toHaveBeenCalledOnce()
    await act(async () => finish())
    expect(button('userInputLateSubmit').disabled).toBe(true)
    expect(container.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic retained draft')
    await act(async () => finishLate())
  })
  it('keeps a same-owner pending action locked when no actual continued status arrives', async () => {
    const source = originalDisplay(); source.questions = [{ ...source.questions[0], multiSelect: false, options: [] }]
    installPrivateDisplay(source)
    let finish!: () => void
    const submit = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const props = { block: projectedBlock(), selector: localSelector, onSubmit: submit }
    await render(createElement(QuestionComposer, props)); await typeAnswer('Synthetic pending')
    await act(async () => button('userInputSubmit').click())
    await render(createElement(QuestionComposer, props))
    expect(button('userInputSubmit').disabled).toBe(true)
    await act(async () => finish())
  })
})

describe('ordinary background versus true authority retirement', () => {
  it.each([false, true])('reauthorizes a fresh private view on foreground only without true revoke=%s', async authority => {
    const fixture = installPrivateDisplay()
    let hidden = false
    vi.spyOn(document, 'hidden', 'get').mockImplementation(() => hidden)
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    const first = fixture.api.openUserInputLocalDisplay.mock.calls[0][0].viewId
    await act(async () => { hidden = true; document.dispatchEvent(new Event('visibilitychange')) })
    expect(container.textContent).not.toContain('Synthetic original question')
    if (authority) await act(async () => fixture.invalidate())
    const before = fixture.api.openUserInputLocalDisplay.mock.calls.length
    await act(async () => { hidden = false; document.dispatchEvent(new Event('visibilitychange')) })
    expect(fixture.api.openUserInputLocalDisplay.mock.calls.length).toBe(before + Number(!authority))
    if (!authority) {
      expect(fixture.api.openUserInputLocalDisplay.mock.calls.at(-1)![0].viewId).not.toBe(first)
      expect(container.textContent).toContain('Synthetic original question')
    } else expect(container.textContent).not.toContain('Synthetic original question')
  })
})

describe('real private PlanReview producer consumption', () => {
  function planDisplay(): UserInputLocalDisplayV1 {
    const source = originalDisplay(), first = source.questions[0]
    return { ...source, callId: 'actual-exit-plan-call', questions: [{ ...first, multiSelect: false,
      detail: '# Original plan title\n\nFirst semantic paragraph.\n\n## Complete tail\n\nAll original plan content.',
      options: [{ ...first.options[0], label: 'Keep original planning' }, { ...first.options[1], label: 'Actual original approval' }],
      intent: { kind: 'plan-review', approveOptionId: first.options[1].id! } }] }
  }
  it('renders exact PlanReview controls and submits only the signed approval option ID once', async () => {
    const source = planDisplay(); installPrivateDisplay(source)
    let finish!: () => void
    const submit = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
    const block = projectedBlock(), before = JSON.stringify(block)
    await render(createElement(QuestionComposer, { block, selector: localSelector, onSubmit: submit }))
    expect(container.querySelector('h3')?.textContent).toBe('Original plan title')
    expect(container.querySelector('[class*="description"]')?.textContent).toBe('First semantic paragraph.')
    expect(container.querySelector('fieldset')).toBeNull(); expect(button('userInputMinimize')).toBeUndefined(); expect(button('userInputTakeTime')).toBeUndefined()
    expect(container.querySelector('[data-state="warning"]')).not.toBeNull()
    await act(async () => { button('userInputPlanApprove').click(); button('userInputPlanApprove').click() })
    expect(submit).toHaveBeenCalledExactlyOnceWith([{ id: source.questions[0].id, kind: 'answer', optionIds: [source.questions[0].options[1].id] }])
    expect(container.querySelector('[data-state="ongoing"] svg, svg[data-state="ongoing"]')).not.toBeNull()
    expect(button('userInputPlanApprove').disabled).toBe(true); expect(JSON.stringify(block)).toBe(before)
    await act(async () => finish())
  })
  it('requests changes through actual foreground cancellation for the normal no-wait plan producer', async () => {
    installPrivateDisplay(planDisplay())
    const cancel = vi.fn(async () => undefined), hide = vi.fn(), submit = vi.fn(async () => undefined)
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector, onCancel: cancel, onSubmit: submit, onHide: hide }))
    await act(async () => button('userInputPlanDiscuss').click())
    expect(cancel).toHaveBeenCalledOnce(); expect(hide).not.toHaveBeenCalled(); expect(submit).not.toHaveBeenCalled()
  })
  it('keeps Request changes available for the actual continued plan without granting a late-submit target', async () => {
    const source = planDisplay(); source.status = 'continued'; source.answerState = 'unavailable'
    source.timedWait = { timeoutSeconds: -1, deadlineAt: null }; source.continuedReason = 'user_input_outcome_unknown_after_restart'
    installPrivateDisplay(source); const hide = vi.fn(), submit = vi.fn(async () => {})
    await render(createElement(QuestionComposer, { block: projectedBlock('continued'), selector: localSelector, onHide: hide, onLateSubmit: submit }))
    expect(button('userInputPlanApprove').disabled).toBe(true)
    expect(button('userInputPlanDiscuss').disabled).toBe(false)
    await act(async () => button('userInputPlanDiscuss').click())
    expect(hide).toHaveBeenCalledOnce(); expect(submit).not.toHaveBeenCalled()
  })
  it('opens the full plan tab automatically once and honors manual close across composer remount', async () => {
    const { api } = installPrivateDisplay(planDisplay()); const props = { block: projectedBlock(), selector: localSelector }
    await render(createElement(QuestionComposer, props))
    const id = questionPlanWorkspaceTabId(localSelector), tabs = useWorkspaceTabsStore.getState()
    expect(tabs.activeTabId).toBe(id); expect(JSON.stringify(tabs.tabs)).not.toContain('All original plan content')
    tabs.setOpen(false); await act(async () => root.render(null)); await render(createElement(QuestionComposer, props))
    expect(useWorkspaceTabsStore.getState().open).toBe(false)
    const readsBeforeClick = api.openUserInputLocalDisplay.mock.calls.length
    const oldViewId = api.openUserInputLocalDisplay.mock.calls.at(-1)?.[0].viewId
    await act(async () => button('userInputPlanOpen').click())
    expect(useWorkspaceTabsStore.getState()).toMatchObject({ open: true, activeTabId: id })
    expect(api.openUserInputLocalDisplay.mock.calls.length).toBeGreaterThan(readsBeforeClick)
    expect(api.openUserInputLocalDisplay.mock.calls.at(-1)?.[0]).toMatchObject(localSelector)
    expect(api.openUserInputLocalDisplay.mock.calls.at(-1)?.[0].viewId).not.toBe(oldViewId)
  })
  it('retains the ordinary Question for the same original text without signed private intent', async () => {
    const source = planDisplay(); delete source.questions[0].intent; installPrivateDisplay(source)
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    expect(container.querySelector('fieldset')).not.toBeNull(); expect(button('userInputPlanApprove')).toBeUndefined()
    expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(0)
  })
  it('keeps a settled PlanReview readonly with the full original plan and recorded approval', async () => {
    const source = planDisplay(); source.status = 'submitted'; source.answerState = 'available'
    source.submission = { schemaVersion: 1, format: 'structured', answers: [{ id: source.questions[0].id, kind: 'answer', optionIds: [source.questions[0].options[1].id!] }] }
    installPrivateDisplay(source); const submit = vi.fn(async () => undefined)
    await render(createElement(QuestionComposer, { block: projectedBlock('submitted'), selector: localSelector, readOnly: true, onSubmit: submit }))
    expect(container.textContent).toContain('All original plan content.')
    expect(button('userInputPlanApprove')).toBeUndefined(); expect(submit).not.toHaveBeenCalled(); expect(useWorkspaceTabsStore.getState().tabs).toHaveLength(0)
  })
})


describe('Host inherited Question display never becomes an active handle', () => {
  function history(status: 'submitted' | 'cancelled' | 'continued'): UserInputLocalDisplayV1 {
    return userInputLocalDisplaySchemaV1.parse({ ...originalDisplay(), threadId: 'child-history-thread',
      readOnly: true, status, answerState: status === 'cancelled' ? 'cancelled' : 'unavailable',
      callId: 'original-signed-call', timedWait: null, continuedReason: null, draftRevision: null, draft: null, lateReply: null })
  }
  it.each(['submitted', 'cancelled', 'continued'] as const)('keeps real %s history pageable/reopenable with zero wait, draft or business actions', async status => {
    const snapshot = history(status), selector = { ...localSelector, threadId: snapshot.threadId }
    const fixture = installPrivateDisplay(snapshot)
    const submit = vi.fn(async () => {}), cancel = vi.fn(async () => {}), late = vi.fn(async () => {}), hide = vi.fn()
    const save = vi.fn(), discard = vi.fn(), expectedTurn = vi.fn(async () => 'current-steering-turn')
    Object.assign(fixture.api, { saveUserInputDraft: save, discardUserInputLateReply: discard })
    const provider = getProvider(), observe = vi.spyOn(provider, 'observeUserInputWait'), settle = vi.spyOn(provider, 'settleUserInputWait')
    const block = { ...projectedBlock(status), timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' } }
    const before = JSON.stringify(block)
    const node = () => createElement(QuestionComposer, { block, selector, readOnly: false,
      onSubmit: submit, onCancel: cancel, onLateSubmit: late, getLateReplyExpectedTurnId: expectedTurn, onHide: hide })
    await render(node())
    expect(container.querySelector('h2')?.textContent).toBe(snapshot.questions[0].question)
    expect(container.querySelector('[role="group"]')).not.toBeNull()
    expect([...container.querySelectorAll<HTMLInputElement>('input')].map(input => input.value)).toEqual(snapshot.questions[0].options.map(option => option.id))
    expect([...container.querySelectorAll<HTMLInputElement>('input')].every(input => input.matches(':disabled') && !input.checked)).toBe(true)
    expect(container.querySelector('textarea')).toBeNull()
    expect(button('userInputSubmit')).toBeUndefined(); expect(button('userInputLateSubmit')).toBeUndefined()
    expect(button('userInputCancel')).toBeUndefined(); expect(button('userInputLateDiscard')).toBeUndefined()
    expect(button('respondInOwnedThread')).toBeUndefined()
    await key(container.querySelector('section')!, 'Enter')
    await act(async () => pager('Next').click())
    expect(container.querySelector('h2')?.textContent).toBe(snapshot.questions[1].question)
    expect(container.querySelector('input')?.value).toBe(snapshot.questions[1].options[0].id)
    await act(async () => { container.querySelector('fieldset')!.focus(); window.dispatchEvent(new Event('blur')) })
    await key(container.querySelector('section')!, 'Escape'); expect(hide).toHaveBeenCalledOnce()
    await act(async () => root.render(null)); await render(node())
    expect(container.querySelector('h2')?.textContent).toBe(snapshot.questions[0].question)
    await act(async () => fixture.invalidate())
    expect(container.textContent).not.toContain(snapshot.questions[0].question)
    await act(async () => requestUserInputLocalReopen(selector))
    expect(container.querySelector('h2')?.textContent).toBe(snapshot.questions[0].question)
    await render(createElement(QuestionComposer, { block, selector, presented: false, onSubmit: submit, onCancel: cancel, onLateSubmit: late }))
    await act(async () => root.render(null))
    for (const command of [submit, cancel, late, save, discard, expectedTurn, observe, settle]) expect(command).not.toHaveBeenCalled()
    expect(fixture.api.closeUserInputLocalDisplay).toHaveBeenCalled()
    expect(fixture.api.runtimeRequest).not.toHaveBeenCalled(); expect(JSON.stringify(block)).toBe(before)
  })
  it.each(['pending', 'submitted', 'cancelled', 'continued'] as const)('does not attach, settle or cancel a %s item before its first fresh Host display', async status => {
    const snapshot = status === 'pending' ? originalDisplay() : history(status)
    const selector = { ...localSelector, threadId: snapshot.threadId }
    const fixture = installPrivateDisplay(snapshot)
    let finish!: (value: { ok: true; display: UserInputLocalDisplayV1 }) => void
    const pending = new Promise<{ ok: true; display: UserInputLocalDisplayV1 }>(resolve => { finish = resolve })
    fixture.api.openUserInputLocalDisplay.mockImplementation(() => pending)
    const provider = getProvider(), observe = vi.spyOn(provider, 'observeUserInputWait').mockReturnValue({ focus: vi.fn(), blur: vi.fn(), engage: vi.fn(), takeTime: vi.fn(), retry: vi.fn(), unsubscribe: vi.fn() })
    const settle = vi.spyOn(provider, 'settleUserInputWait'), cancel = vi.fn(async () => {})
    await render(createElement(QuestionComposer, { block: { ...projectedBlock(status), timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' } }, selector, onCancel: cancel }))
    expect(observe).not.toHaveBeenCalled(); expect(settle).not.toHaveBeenCalled()
    if (status === 'pending') { expect(button('userInputCancel').disabled).toBe(true); await act(async () => button('userInputCancel').click()) }
    await key(container.querySelector('section')!, 'Escape'); expect(cancel).not.toHaveBeenCalled()
    await act(async () => finish({ ok: true, display: snapshot }))
    if (status === 'pending') expect(observe).toHaveBeenCalled()
    else { expect(observe).not.toHaveBeenCalled(); expect(settle).not.toHaveBeenCalled() }
  })
})


it('retains an unknown live draft attempt across deferred Host refresh and retries its exact original body', async () => {
  vi.useFakeTimers()
  try {
    const source = singleDraftDisplay(), fixture = installPrivateDisplay(source)
    const save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1): Promise<UserInputDraftSaveResultV1> => ({ ok: true as const, receipt: {
      schemaVersion: 1 as const, ...localSelector, status: 'pending' as const, draftRevision: request.revision, draft: request.draft
    } })).mockResolvedValueOnce({ ok: false, code: 'unavailable' })
    Object.assign(fixture.api, { saveUserInputDraft: save })
    await render(createElement(QuestionComposer, { block: projectedBlock(), selector: localSelector }))
    let finish!: (value: { ok: true; display: UserInputLocalDisplayV1 }) => void
    const read = new Promise<{ ok: true; display: UserInputLocalDisplayV1 }>(resolve => { finish = resolve })
    fixture.api.openUserInputLocalDisplay.mockImplementation(() => read)
    await typeAnswer('Synthetic unfinished unknown attempt')
    expect(save).toHaveBeenCalledOnce()
    const original = structuredClone(save.mock.calls[0][0])
    expect(container.querySelector('textarea')).toBeNull()
    await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
    expect(save).toHaveBeenCalledOnce()
    await act(async () => finish({ ok: true, display: source }))
    expect(button('userInputDraftRetry')).toBeDefined()
    await act(async () => button('userInputDraftRetry').click())
    expect(save).toHaveBeenCalledTimes(2); expect(save.mock.calls[1][0]).toEqual(original)
    expect(original.draft.answers[0].customText).toBe('Synthetic unfinished unknown attempt')
  } finally { vi.useRealTimers() }
})
