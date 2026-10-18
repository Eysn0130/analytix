// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { UserInputLocalDisplayV1, UserInputLocalViewRequestV1 } from '../../../../../packages/runtime/src/contracts/user-input-local-display'
import type { UserInputLocalEffectRequestV1 } from '../../../../shared/analytix-api'
import { QuestionPlanPreview } from './QuestionPlanPreview'
import i18n from '../../i18n'
import { useWorkspaceTabsStore, questionPlanWorkspaceTabId, observeQuestionPlanAuthority } from '../../store/workspace-tabs-store'
const requestId = 'input_' + 'c'.repeat(64), questionId = requestId + '_1'
const selector = { threadId: 'thread-plan', turnId: 'turn-plan', itemId: 'item_' + requestId, requestId }
const owner = { selector, callId: 'actual-plan-call' }
const code = Array.from({ length: 18 }, (_, index) => 'original line ' + index).join('\n')
const plan = '# Complete original plan\n\nOriginal paragraph.\n\n```text\n' + code + '\n```\n\n[Original reference](https://example.invalid/path)\n\n![Original diagram](https://example.invalid/image.png)\n\n## Complete final section\n\nOriginal tail beyond the summary.'
const display: UserInputLocalDisplayV1 = { schemaVersion: 1, ...selector, status: 'pending', answerState: 'pending', callId: owner.callId,
  questions: [{ id: questionId, header: 'Original', question: 'Approve original plan?', detail: plan, multiSelect: false,
    options: [{ id: questionId + '_option_1', label: 'Approve', description: '' }], intent: { kind: 'plan-review', approveOptionId: questionId + '_option_1' } }] }
let root: Root, container: HTMLDivElement, previousApi: PropertyDescriptor | undefined
let open: ReturnType<typeof vi.fn<(request: UserInputLocalViewRequestV1) => Promise<unknown>>>
let effect: ReturnType<typeof vi.fn<(request: UserInputLocalEffectRequestV1) => Promise<unknown>>>
let close: ReturnType<typeof vi.fn>, fileRead: ReturnType<typeof vi.fn>, fileWrite: ReturnType<typeof vi.fn>
let invalidate: (id: string | null, reason?: 'authority' | 'background') => void
async function render(presented = true) { await act(async () => root.render(createElement(QuestionPlanPreview, { owner, presented }))) }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); await i18n.changeLanguage('zh')
  useWorkspaceTabsStore.setState({ tabs: [], openedQuestionPlanReviews: {} })
  useWorkspaceTabsStore.getState().openQuestionPlanReview(owner, 'Plan')
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  open = vi.fn(async () => ({ ok: true, display })); effect = vi.fn(async () => ({ ok: true })); close = vi.fn(async () => undefined)
  fileRead = vi.fn(); fileWrite = vi.fn()
  const listeners = new Set<typeof invalidate>()
  invalidate = (id, reason) => [...listeners].forEach(listener => listener(id, reason))
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close,
    onUserInputLocalDisplayInvalidated: (listener: typeof invalidate) => { listeners.add(listener); return () => { listeners.delete(listener) } },
    performUserInputLocalEffect: effect
  }, files: { read: fileRead, write: fileWrite } } })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove(); vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi); else Reflect.deleteProperty(window, 'analytix')
})
describe('complete original plan in the native private sidebar leaf', () => {
  it('renders every original section and full code with real Copy/Wrap/link effects, without file editor writes', async () => {
    await render()
    expect(container.textContent).toContain('Original tail beyond the summary.')
    expect(container.querySelector('pre')?.textContent).toBe(code)
    const copy = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:userInputCopy')}"]`)!
    await act(async () => copy.click())
    expect(effect.mock.calls[0][0]).toMatchObject({ selector, viewId: open.mock.calls[0][0].viewId, effect: { kind: 'copyCode', questionId } })
    expect(JSON.stringify(effect.mock.calls)).not.toContain(code)
    const wrap = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:userInputWrap')}"]`)!
    await act(async () => wrap.click()); expect(container.querySelector('pre')).not.toBeNull()
    const link = container.querySelector<HTMLAnchorElement>('a')!
    expect(link.getAttribute('href')).toBe('https://example.invalid/path'); expect(link.getAttribute('target')).toBe('_blank'); await act(async () => link.click())
    expect(effect.mock.calls.at(-1)?.[0]).toMatchObject({ selector, effect: { kind: 'openDetailLink', questionId } })
    expect(container.querySelector('img')?.getAttribute('src')).toBe('https://example.invalid/image.png')
    expect(container.textContent).not.toContain(i18n.t('common:markdownTableDownload'))
    expect(fileRead).not.toHaveBeenCalled(); expect(fileWrite).not.toHaveBeenCalled()
  })
  it('never acquires raw text for a retained hidden dock and rejects a late result after it hides', async () => {
    await render(false); expect(open).not.toHaveBeenCalled()
    let finish!: (value: unknown) => void; open.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    await render(true); const first = open.mock.calls[0][0].viewId
    await render(false); await act(async () => finish({ ok: true, display }))
    expect(container.textContent).not.toContain('Complete original plan'); expect(close).toHaveBeenCalledWith(first)
    expect(effect).not.toHaveBeenCalled()
  })
  it('keeps an unmounted inactive plan retired after global revoke and automatic neighbor selection', async () => {
    const stop = observeQuestionPlanAuthority(window.analytix.runtime, () => () => {})
    try {
      await render(false); await act(async () => root.render(null))
      await act(async () => invalidate(null, 'background'))
      expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBeUndefined()
      await act(async () => invalidate(null, 'authority'))
      useWorkspaceTabsStore.getState().activateTab(questionPlanWorkspaceTabId(selector))
      await render(true); expect(open).not.toHaveBeenCalled()
      expect(container.textContent).not.toContain('Original tail beyond the summary.')
      await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
      expect(open).toHaveBeenCalledOnce()
      expect(container.textContent).toContain('Original tail beyond the summary.')
    } finally { stop() }
  })
  it('preserves true revoke across retained dock and fresh overlay mounts until explicit retry', async () => {
    const pair = (overlay: boolean) => createElement('div', null,
      createElement(QuestionPlanPreview, { owner, presented: !overlay, key: 'dock' }),
      overlay ? createElement(QuestionPlanPreview, { owner, presented: true, key: 'overlay' }) : null)
    await act(async () => root.render(pair(false)))
    expect(open).toHaveBeenCalledOnce()
    await act(async () => invalidate(null, 'authority'))
    expect(useWorkspaceTabsStore.getState().tabs[0].questionPlanRetired).toBe(true)
    await act(async () => root.render(pair(true)))
    await act(async () => root.render(pair(false)))
    expect(open).toHaveBeenCalledOnce(); expect(container.textContent).not.toContain('Original tail beyond the summary.')
    // Merely selecting a neighbor or closing a different tab retains retirement.
    useWorkspaceTabsStore.getState().activateTab(questionPlanWorkspaceTabId(selector))
    await act(async () => root.render(pair(true)))
    expect(open).toHaveBeenCalledOnce()
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
    expect(open).toHaveBeenCalledTimes(2)
    expect(open.mock.calls[1][0].viewId).not.toBe(open.mock.calls[0][0].viewId)
    expect(container.textContent).toContain('Original tail beyond the summary.')
  })
  it('erases raw text on true revoke and uses a fresh native lease only after an explicit reopen', async () => {
    await render(); const first = open.mock.calls[0][0].viewId
    await act(async () => invalidate(null, 'authority')); await render()
    expect(container.textContent).not.toContain('Original tail beyond the summary.'); expect(open).toHaveBeenCalledOnce()
    await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
    expect(open).toHaveBeenCalledTimes(2); expect(open.mock.calls[1][0].viewId).not.toBe(first)
    expect(container.textContent).toContain('Original tail beyond the summary.')
  })
})
