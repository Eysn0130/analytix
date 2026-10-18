// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { createMaintenanceActions } from '../../store/chat-store-maintenance-actions'
import { MessageBubble } from './message-timeline-bubbles'
import { useThreadComposerDraft } from './use-thread-composer-draft'
import { clearFileReferenceValidationCache } from '../../lib/file-reference-validation'
import { WORKSPACE_FILE_PREVIEW_EVENT } from '../../lib/workspace-file-preview'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'

const registry = vi.hoisted(() => ({ getProvider: vi.fn() }))
vi.mock('../../agent/registry', () => ({ getProvider: registry.getProvider }))

let root: Root
let container: HTMLDivElement
let previousState: ReturnType<typeof useChatStore.getState>
let previousBridge: typeof window.analytix
let composer: ReturnType<typeof useThreadComposerDraft>
let clipboardDescriptor: PropertyDescriptor | undefined, execDescriptor: PropertyDescriptor | undefined
const workspace = '/synthetic/edit'
const threadId = 'edit-thread'
const attachmentId = `att_${'f'.repeat(24)}`
const block: ChatBlock = { kind: 'user', id: 'edit-user', text: 'original message', meta: {
  turnId: 'edit-turn', workspaceCheckpointId: 'gcp_fixture', attachmentIds: [attachmentId],
  fileReferences: [{ path: `${workspace}/fixture.txt`, relativePath: 'fixture.txt', name: 'fixture.txt', kind: 'file' }]
} }
function Composer() {
  composer = useThreadComposerDraft(workspace, threadId)
  return createElement('textarea', { 'data-composer-probe': true, value: composer.draft.input, readOnly: true })
}
async function render(displayBlock: ChatBlock = block) {
  await act(async () => root.render(createElement('div', null, createElement(MessageBubble, { block: displayBlock, threadId }), createElement(Composer))))
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  clearFileReferenceValidationCache()
  await i18n.changeLanguage('zh')
  previousState = useChatStore.getState()
  previousBridge = window.analytix
  clipboardDescriptor = Object.getOwnPropertyDescriptor(navigator, 'clipboard')
  execDescriptor = Object.getOwnPropertyDescriptor(document, 'execCommand')
  const actions = createMaintenanceActions({ set: useChatStore.setState, get: useChatStore.getState, sseAbortRef: { current: null } })
  useChatStore.setState({ activeThreadId: threadId, workspaceRoot: workspace, route: 'chat', busy: false,
    runtimeConnection: 'ready', blocks: [block], composerDrafts: {}, currentTurnId: null, currentTurnUserId: null,
    threads: [{ id: threadId, title: 'Synthetic edit', updatedAt: '2026-10-05', workspace, status: 'idle', model: 'synthetic', mode: 'agent' }],
    rewindAndResend: actions.rewindAndResend })
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove(); useChatStore.setState(previousState)
  Object.assign(window, { analytix: previousBridge })
  if (clipboardDescriptor) Object.defineProperty(navigator, 'clipboard', clipboardDescriptor)
  else Reflect.deleteProperty(navigator, 'clipboard')
  if (execDescriptor) Object.defineProperty(document, 'execCommand', execDescriptor)
  else Reflect.deleteProperty(document, 'execCommand')
  document.getSelection()?.removeAllRanges()
  clearFileReferenceValidationCache()
  vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
async function beginEdit(text: string) {
  const edit = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:rewindEditMessage')}"]`)!
  await act(async () => edit.click())
  const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(editor, text)
    editor.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
function resendButton() {
  return [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:rewindResend'))!
}
function readyProvider() {
  const getAttachmentMetadata = vi.fn(async () => ({ id: attachmentId, name: 'fixture.txt', kind: 'document', mimeType: 'text/plain', byteSize: 2,
    scope: 'thread', createdAt: '2026-10-05', updatedAt: '2026-10-05' }))
  const rewindThread = vi.fn(async () => undefined)
  const sendMessage = vi.fn(async () => true)
  const restoreGitCheckpoint = vi.fn(async () => ({ ok: true }))
  registry.getProvider.mockReturnValue({ getAttachmentMetadata, rewindThread })
  Object.assign(window, { analytix: { workspace: { restoreGitCheckpoint } } })
  useChatStore.setState({ sendMessage, error: null })
  return { getAttachmentMetadata, rewindThread, sendMessage, restoreGitCheckpoint }
}
describe('actual message edit to composer draft continuity', () => {
  it('retains the editor when connection disconnects after editing and sends only on explicit retry', async () => {
    const h = readyProvider()
    await render()
    await beginEdit('offline edit retained')
    await act(async () => useChatStore.setState({ runtimeConnection: 'offline' }))
    await act(async () => resendButton().click())
    const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')
    expect(editor).not.toBeNull()
    expect(editor!.value).toBe('offline edit retained')
    expect(useChatStore.getState().error).toBe(i18n.t('common:runtimeActionNeedsConnection'))
    expect(useChatStore.getState().blocks).toEqual([block])
    expect(useChatStore.getState().composerDrafts).toEqual({})
    expect(h.getAttachmentMetadata).not.toHaveBeenCalled(); expect(h.restoreGitCheckpoint).not.toHaveBeenCalled()
    expect(h.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
    await act(async () => useChatStore.setState({ runtimeConnection: 'ready' }))
    expect(h.sendMessage).not.toHaveBeenCalled(); expect(h.rewindThread).not.toHaveBeenCalled()
    await act(async () => resendButton().click())
    expect(container.querySelector('[data-user-message-bubble] textarea')).toBeNull()
    expect(h.rewindThread).toHaveBeenCalledExactlyOnceWith(threadId, 'edit-turn')
    expect(h.sendMessage).toHaveBeenCalledExactlyOnceWith('offline edit retained', undefined,
      expect.objectContaining({ attachmentIds: [attachmentId] }))
    expect(useChatStore.getState().composerDrafts).toEqual({})
  })

  it.each(['thread', 'workspace', 'case'])('does not restore cross-%s content when a disconnected edit is unaccepted', async change => {
    const h = readyProvider()
    await render(); await beginEdit('local edit stays here')
    await act(async () => {
      const state = useChatStore.getState()
      useChatStore.setState({ runtimeConnection: 'offline',
        ...(change === 'thread' ? { activeThreadId: 'other-thread', threads: [...state.threads,
          { id: 'other-thread', title: 'Other', workspace: '/synthetic/other', updatedAt: '2026-10-05', model: 'synthetic', mode: 'agent' }] } : {}),
        ...(change === 'workspace' ? { threads: state.threads.map(thread => ({ ...thread, workspace: '/synthetic/moved' })) } : {}),
        ...(change === 'case' ? { threads: state.threads.map(thread => ({ ...thread, historyAuthority: 'case_boundary_only_v1' as const })) } : {}) })
    })
    await act(async () => resendButton().click())
    // Reconnecting cannot transfer the retained local edit to another owner.
    // A fresh metadata authority would reject those old references as well.
    if (change !== 'case') h.getAttachmentMetadata.mockRejectedValueOnce(new Error('OWNER_METADATA_DENIED'))
    await act(async () => useChatStore.setState({ runtimeConnection: 'ready' }))
    await act(async () => resendButton().click())
    expect(container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!.value).toBe('local edit stays here')
    expect(useChatStore.getState().composerDrafts).toEqual({})
    expect(h.getAttachmentMetadata).not.toHaveBeenCalled(); expect(h.restoreGitCheckpoint).not.toHaveBeenCalled()
    expect(h.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it('keeps a retained editor local when its mounted bubble receives a different block', async () => {
    const h = readyProvider()
    await render(); await beginEdit('edit belongs to original block')
    const replacement: ChatBlock = { ...block, id: 'other-user' }
    await act(async () => useChatStore.setState({ blocks: [replacement] }))
    await render(replacement)
    await act(async () => resendButton().click())
    expect(container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!.value).toBe('edit belongs to original block')
    expect(useChatStore.getState().composerDrafts).toEqual({})
    expect(h.getAttachmentMetadata).not.toHaveBeenCalled(); expect(h.restoreGitCheckpoint).not.toHaveBeenCalled()
    expect(h.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it.each(['case', 'invalid-payload'])('retains the editor for an adjacent %s early rejection without saving unsafe payload', async reason => {
    const h = readyProvider()
    await render(); await beginEdit('unaccepted local text')
    await act(async () => {
      if (reason === 'case') useChatStore.setState({ threads: useChatStore.getState().threads.map(thread => ({ ...thread, historyAuthority: 'case_boundary_only_v1' })) })
      else useChatStore.setState({ blocks: [{ ...block, meta: { ...block.meta, attachmentIds: ['invalid-id'] } }] })
    })
    await act(async () => resendButton().click())
    expect(container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!.value).toBe('unaccepted local text')
    expect(useChatStore.getState().composerDrafts).toEqual({})
    expect(h.getAttachmentMetadata).not.toHaveBeenCalled(); expect(h.restoreGitCheckpoint).not.toHaveBeenCalled()
    expect(h.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it('retains a second editor when an earlier edit still owns preflight', async () => {
    const h = readyProvider()
    let fail!: () => void
    const pending = new Promise<never>((_, reject) => { fail = () => reject(new Error('PRIVATE_METADATA_BODY')) })
    h.getAttachmentMetadata.mockReturnValueOnce(pending)
    await render(); await beginEdit('first edit')
    await act(async () => resendButton().click())
    expect(container.querySelector('[data-user-message-bubble] textarea')).toBeNull()
    await beginEdit('second edit')
    await act(async () => resendButton().click())
    expect(container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!.value).toBe('second edit')
    expect(h.getAttachmentMetadata).toHaveBeenCalledTimes(1)
    await act(async () => fail())
    expect(container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!.value).toBe('second edit')
    expect(composer.draft.input).toBe('first edit')
    expect(JSON.stringify(composer.draft)).not.toContain('PRIVATE_METADATA_BODY')
    expect(h.rewindThread).not.toHaveBeenCalled(); expect(h.sendMessage).not.toHaveBeenCalled()
  })

  it.each(['metadata', 'restore'])('retains the closed edit after %s preflight fails, including IDs-only attachments', async seam => {
    let fail!: () => void
    const pending = new Promise<never>((_, reject) => { fail = () => reject(new Error('PRIVATE_PREFLIGHT_BODY')) })
    const metadata = { id: attachmentId, name: 'fixture.txt', kind: 'document', mimeType: 'text/plain', byteSize: 2,
      scope: 'thread', createdAt: '2026-10-05', updatedAt: '2026-10-05' }
    const rewindThread = vi.fn(async () => undefined)
    const sendMessage = vi.fn(async () => true)
    const restoreGitCheckpoint = vi.fn(() => seam === 'restore' ? pending : Promise.resolve({ ok: true }))
    const getAttachmentMetadata = vi.fn(() => seam === 'metadata' ? pending : Promise.resolve(metadata))
    registry.getProvider.mockReturnValue({ getAttachmentMetadata, rewindThread })
    Object.assign(window, { analytix: { workspace: { restoreGitCheckpoint } } })
    useChatStore.setState({ sendMessage })
    await render()
    const edit = container.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:rewindEditMessage')}"]`)!
    expect(edit).not.toBeNull()
    await act(async () => edit.click())
    const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-bubble] textarea')!
    expect(editor).not.toBeNull()
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(editor, 'edited text survives')
      editor.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const resend = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:rewindResend'))!
    await act(async () => resend.click())
    expect(container.querySelector('[data-user-message-bubble] textarea')).toBeNull()
    await act(async () => fail())
    expect(composer.draft.input).toBe('edited text survives')
    expect(composer.draft.attachments.map(item => item.id)).toEqual([attachmentId])
    expect(composer.draft.fileReferences).toEqual([{ path: `${workspace}/fixture.txt`, relativePath: 'fixture.txt', name: 'fixture.txt', type: 'file' }])
    expect(JSON.stringify(composer.draft)).not.toContain('PRIVATE_PREFLIGHT_BODY')
    expect(useChatStore.getState().blocks).toEqual([block])
    expect(rewindThread).not.toHaveBeenCalled(); expect(sendMessage).not.toHaveBeenCalled()
    expect(restoreGitCheckpoint).toHaveBeenCalledTimes(seam === 'restore' ? 1 : 0)
    await act(async () => root.render(createElement('div', null, 'settings')))
    await render()
    expect(composer.draft.input).toBe('edited text survives')
    expect(container.querySelector<HTMLTextAreaElement>('[data-composer-probe]')!.value).toBe('edited text survives')
  })
})


describe('user message DSH chrome with native editing', () => {
  const t = (key: string, values?: Record<string, unknown>) => String(i18n.t('common:' + key, values))
  const actions = () => container.querySelector<HTMLElement>('[data-clock="start"]')!
  const copy = () => actions().querySelector<HTMLButtonElement>('button')!

  it('keeps user Markdown literal and the time before the actual copy and edit actions', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 9, 15, 12, 10))
    const text = '# literal heading\n**literal bold** and `code`'
    const createdAt = new Date(2026, 9, 15, 9, 7).toISOString()
    await render({ ...block, text, createdAt })
    const bubble = container.querySelector('[data-user-message-bubble]')!
    expect(bubble.textContent).toContain(text)
    expect(bubble.querySelector('h1, strong, code')).toBeNull()
    // Attachments and text share the capped 8px stack; the complete footer is
    // its sibling so their separation uses the row's 6px gap.
    const stack = bubble.parentElement!
    const attachments = stack.querySelector('[data-message-attachments]')!
    expect(attachments).not.toBeNull(); expect(attachments.parentElement).toBe(stack)
    expect(attachments.nextElementSibling).toBe(bubble)
    expect(stack.nextElementSibling).toBe(actions())
    expect(actions().parentElement).toBe(stack.parentElement)
    expect(stack.querySelector('[data-clock="start"]')).toBeNull()
    const clock = actions().querySelector('time')!
    expect(clock.dateTime).toBe(createdAt); expect(clock.textContent).toBe('09:07')
    expect(clock.compareDocumentPosition(copy()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(copy().querySelector('svg')?.getAttribute('viewBox')).toBe('0 0 16 16')
    expect(copy().querySelector('svg rect')).not.toBeNull()
    const edit = actions().querySelector<HTMLButtonElement>(`button[aria-label="${t('rewindEditMessage')}"]`)!
    expect(edit).not.toBeNull(); expect(edit.disabled).toBe(false)
    await act(async () => edit.click())
    const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-editor]')!
    expect(editor.value).toBe(text)
    const editorBubble = editor.closest('[data-user-message-bubble]')!
    const editorStack = editorBubble.parentElement!
    expect(editorStack.querySelector('[data-message-attachments]')!.nextElementSibling).toBe(editorBubble)
  })

  it.each([false, true])('omits user model chrome and its empty edit row while retaining metadata (checkpoint=%s)', async checkpoint => {
    const modelLabel = 'Synthetic selected model'
    const displayBlock: ChatBlock = { ...block, modelLabel, createdAt: '2026-10-16T09:07:00Z', meta: { ...block.meta,
      attachmentIds: [], fileReferences: [], workspaceCheckpointId: checkpoint ? 'gcp_fixture' : undefined } }
    await act(async () => useChatStore.setState({ blocks: [displayBlock] }))
    await render(displayBlock)
    expect(container.textContent).not.toContain(modelLabel)
    expect(actions().children).toHaveLength(3)
    expect(actions().firstElementChild?.tagName).toBe('TIME')
    expect(copy().getAttribute('aria-label')).toBe(t('copyMessage'))
    expect(actions().querySelector(`button[aria-label="${t('rewindEditMessage')}"]`)).not.toBeNull()
    expect(useChatStore.getState().blocks[0]).toBe(displayBlock)
    await beginEdit('retained editable text')
    expect(container.textContent).not.toContain(modelLabel)
    const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-editor]')!
    expect(editor.value).toBe('retained editable text')
    const stack = editor.closest('[data-user-message-bubble]')!.parentElement!
    expect(stack.children).toHaveLength(checkpoint ? 2 : 1)
    expect(stack.textContent?.includes(t('rewindFileRollbackNotice'))).toBe(checkpoint)
    expect(useChatStore.getState().blocks[0]).toBe(displayBlock)
    const cancel = [...container.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === t('rewindCancel'))!
    await act(async () => cancel.click())
    expect(container.textContent).not.toContain(modelLabel)
    expect(actions().firstElementChild?.tagName).toBe('TIME')
    expect(useChatStore.getState().blocks[0]).toBe(displayBlock)
  })

  it.each([undefined, '', 'invalid-time'])('omits an unavailable user timestamp (%s)', async createdAt => {
    await render({ ...block, createdAt })
    expect(actions().querySelector('time')).toBeNull()
    expect(copy().getAttribute('aria-label')).toBe(t('copyMessage'))
  })

  it.each([
    { date: new Date(2026, 1, 3, 9, 7), key: 'dshClockMonthDay', values: { y: 2026, m: 2, d: 3 } },
    { date: new Date(2025, 11, 31, 9, 7), key: 'dshClockYearMonthDay', values: { y: 2025, m: 12, d: 31 } }
  ])('uses local calendar dates for $key', async ({ date, key, values }) => {
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 9, 15, 12))
    await render({ ...block, createdAt: date.toISOString() })
    expect(actions().querySelector('time')!.textContent).toBe(t(key, values) + ' 09:07')
  })

  it('refreshes at local year rollover and removes its midnight timer on unmount', async () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date(2026, 11, 31, 23, 59, 59, 500))
    await render({ ...block, createdAt: new Date(2026, 11, 31, 23, 59).toISOString() })
    expect(actions().querySelector('time')!.textContent).toBe('23:59')
    await act(async () => vi.advanceTimersByTime(500))
    expect(actions().querySelector('time')!.textContent).toBe(t('dshClockYearMonthDay', { y: 2026, m: 12, d: 31 }) + ' 23:59')
    await act(async () => root.render(null))
    expect(vi.getTimerCount()).toBe(0)
  })

  it('shows the exact check only after copy succeeds, gates repeats and resets after one second', async () => {
    vi.useFakeTimers()
    let complete!: () => void
    const pending = new Promise<void>(resolve => { complete = resolve })
    const writeText = vi.fn(() => pending)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await render()
    const button = copy()
    await act(async () => { button.click(); button.click() })
    expect(writeText).toHaveBeenCalledExactlyOnceWith('original message')
    expect(button.getAttribute('aria-label')).toBe(t('copyMessage'))
    await act(async () => { complete(); await pending })
    expect(button.getAttribute('aria-label')).toBe(t('copySuccess'))
    expect(button.querySelector('svg rect')).toBeNull()
    expect(button.querySelector('svg path')?.getAttribute('d')).toBe('M2.25 8.5L5.49732 11.7473C5.90519 12.1552 6.57263 12.1344 6.95426 11.7018L13.75 4')
    await act(async () => button.click()); expect(writeText).toHaveBeenCalledOnce()
    await act(async () => vi.advanceTimersByTime(999))
    expect(button.getAttribute('aria-label')).toBe(t('copySuccess'))
    await act(async () => vi.advanceTimersByTime(1))
    expect(button.getAttribute('aria-label')).toBe(t('copyMessage'))
  })

  it('keeps denied copy idle and requires an actual successful fallback', async () => {
    const exec = vi.fn(() => false), writeText = vi.fn(async () => { throw new Error('Synthetic denial') })
    Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await render()
    await act(async () => copy().click())
    expect(exec).not.toHaveBeenCalled(); expect(copy().getAttribute('aria-label')).toBe(t('copyMessage'))
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    await act(async () => copy().click())
    expect(exec).toHaveBeenCalledWith('copy'); expect(copy().getAttribute('aria-label')).toBe(t('copyMessage'))
    expect(document.querySelector('body > textarea[readonly]')).toBeNull()
    exec.mockImplementation(() => {
      expect(document.querySelector<HTMLTextAreaElement>('body > textarea[readonly]')!.value).toBe('original message')
      return true
    })
    await act(async () => copy().click())
    expect(copy().getAttribute('aria-label')).toBe(t('copySuccess'))
    expect(document.querySelector('body > textarea[readonly]')).toBeNull()
  })

  it.each(['text-change', 'unmount'] as const)('retires pending copy on %s without stale success', async change => {
    vi.useFakeTimers()
    let complete!: () => void
    const pending = new Promise<void>(resolve => { complete = resolve })
    const writeText = vi.fn(() => pending)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await render(); await act(async () => copy().click())
    if (change === 'text-change') await render({ ...block, text: 'replacement message' })
    else await act(async () => root.render(null))
    await act(async () => { complete(); await pending })
    expect(container.querySelector(`button[aria-label="${t('copySuccess')}"]`)).toBeNull()
    expect(vi.getTimerCount()).toBe(0)
    if (change === 'text-change') {
      await act(async () => copy().click())
      expect(writeText).toHaveBeenLastCalledWith('replacement message')
    }
  })

  it('retains IME and repeat guards, Escape cancel and explicit keyboard resend', async () => {
    let now = Date.now(); vi.spyOn(Date, 'now').mockImplementation(() => now)
    const h = readyProvider()
    await render(); await beginEdit('keyboard edit')
    const editor = container.querySelector<HTMLTextAreaElement>('[data-user-message-editor]')!
    await act(async () => {
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, isComposing: true, bubbles: true }))
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', isComposing: true, bubbles: true }))
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', metaKey: true, repeat: true, bubbles: true }))
    })
    expect(h.sendMessage).not.toHaveBeenCalled(); expect(editor.isConnected).toBe(true)
    await act(async () => {
      editor.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }))
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
      editor.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }))
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }))
      editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    })
    now += 9
    await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true })))
    expect(h.sendMessage).not.toHaveBeenCalled(); expect(h.rewindThread).not.toHaveBeenCalled(); expect(editor.isConnected).toBe(true)
    now += 1
    await act(async () => editor.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })))
    expect(container.querySelector('[data-user-message-editor]')).toBeNull()
    await beginEdit('explicit resend')
    await act(async () => container.querySelector('[data-user-message-editor]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true })))
    expect(h.sendMessage).toHaveBeenCalledExactlyOnceWith('explicit resend', undefined, expect.objectContaining({ attachmentIds: [attachmentId] }))
  })
})


describe('user inline references on the actual message path', () => {
  const bubble = () => container.querySelector<HTMLElement>('[data-user-message-bubble]')!
  const reference = (title: string) => bubble().querySelector<HTMLElement>(`[data-ref-chip][title="${title}"]`)!

  it('keeps plain text and decorates files, but only navigates a successfully resolved real file', async () => {
    const resolve = vi.fn(async ({ path }: { path: string }) => path === 'docs/fixture.txt'
      ? { ok: true as const, path: `${workspace}/docs/fixture.txt` }
      : { ok: false as const, message: 'unavailable' })
    Object.assign(window, { analytix: { files: { resolve } } })
    const previews = vi.fn(); window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews)
    try {
      await render({ ...block, text: '# plain **literal** @docs/fixture.txt, @docs/missing.txt @docs/' })
      expect(bubble().querySelector('h1, strong, code')).toBeNull()
      expect(bubble().textContent).toContain('# plain **literal** fixture.txt, missing.txt docs')
      expect(resolve).toHaveBeenCalledTimes(2)
      expect(resolve).toHaveBeenCalledWith(expect.objectContaining({ path: 'docs/fixture.txt', workspaceRoot: workspace }))
      expect(reference('@docs/fixture.txt').tagName).toBe('BUTTON')
      expect(reference('@docs/fixture.txt').querySelector('svg path')?.getAttribute('d')).toBe('M4.9375 5.90295H11.0625')
      expect(reference('@docs/missing.txt').tagName).toBe('SPAN')
      expect(reference('@docs/').tagName).toBe('SPAN')
      await act(async () => reference('@docs/fixture.txt').click())
      expect(previews).toHaveBeenCalledOnce()
      expect((previews.mock.calls[0]![0] as CustomEvent).detail).toEqual({ path: `${workspace}/docs/fixture.txt`, workspaceRoot: workspace })
      expect(bubble().parentElement!.nextElementSibling!.querySelector('button[aria-label="' + i18n.t('common:rewindEditMessage') + '"]')).not.toBeNull()
    } finally { window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews) }
  })

  it('guards double clicks and pointer selection while preserving keyboard activation', async () => {
    Object.assign(window, { analytix: { files: { resolve: vi.fn(async () => ({ ok: true, path: `${workspace}/fixture.txt` })) } } })
    const previews = vi.fn(); window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews)
    try {
      await render({ ...block, text: '@fixture.txt' })
      const button = reference('@fixture.txt')
      await act(async () => button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 2 })))
      expect(previews).not.toHaveBeenCalled()
      const selection = document.getSelection()!, range = document.createRange()
      range.selectNodeContents(button); selection.removeAllRanges(); selection.addRange(range)
      await act(async () => button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 })))
      expect(previews).not.toHaveBeenCalled()
      await act(async () => button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 0 })))
      expect(previews).toHaveBeenCalledOnce()
      selection.removeAllRanges()
      await act(async () => button.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 })))
      expect(previews).toHaveBeenCalledTimes(2)
    } finally { window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews) }
  })

  it('decorates slash skills from this message only and resolves an exact local skill ID', async () => {
    const listSkills = vi.fn(async () => ({ ok: true, skills: [
      { id: 'audit', name: 'Misleading display name', root: `${workspace}/.agents/skills/audit`, entryPath: `${workspace}/.agents/skills/audit/SKILL.md`, scope: 'project', legacy: false },
      { id: 'unloaded', name: 'unloaded', root: '/synthetic/unloaded', entryPath: '/synthetic/unloaded/SKILL.md', scope: 'global', legacy: false }
    ], validationErrors: [] }))
    const resolve = vi.fn(async ({ path }: { path: string }) => ({ ok: true, path }))
    Object.assign(window, { analytix: { app: { listSkills }, files: { resolve } } })
    const previews = vi.fn(); window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews)
    try {
      await render({ ...block, text: '/audit /unloaded /audit.md /audit。 $[Wire Audit](skill://audit)',
        meta: { ...block.meta, activeSkillIds: ['audit'] } })
      expect(listSkills).toHaveBeenCalledExactlyOnceWith(workspace)
      expect(bubble().querySelectorAll('[data-ref-chip="skill"]')).toHaveLength(2)
      expect(reference('/audit').tagName).toBe('BUTTON')
      expect(reference('/audit').querySelector('svg')).toBeNull()
      expect(bubble().textContent).toContain('/audit /unloaded /audit.md /audit。 Wire Audit')
      expect(resolve).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ path: `${workspace}/.agents/skills/audit/SKILL.md`, workspaceRoot: workspace }))
      await act(async () => reference('/audit').click())
      expect((previews.mock.calls[0]![0] as CustomEvent).detail.path).toBe(`${workspace}/.agents/skills/audit/SKILL.md`)
      await render({ ...block, text: '/audit $[Wire Audit](skill://audit)', meta: { ...block.meta, activeSkillIds: [] } })
      expect(bubble().querySelectorAll('[data-ref-chip="skill"]')).toHaveLength(1)
      expect(reference('Wire Audit').tagName).toBe('SPAN')
    } finally { window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews) }
  })

  it('keeps missing skill identity decorative and ignores the old workspace catalog response', async () => {
    let finish!: (value: unknown) => void
    const pending = new Promise<unknown>(done => { finish = done })
    const listSkills = vi.fn().mockReturnValueOnce(pending).mockResolvedValue({ ok: true, skills: [], validationErrors: [] })
    const resolve = vi.fn(async ({ path }: { path: string }) => ({ ok: true, path }))
    Object.assign(window, { analytix: { app: { listSkills }, files: { resolve } } })
    await render({ ...block, text: '/audit', meta: { ...block.meta, activeSkillIds: ['audit'] } })
    expect(reference('/audit').tagName).toBe('SPAN')
    await act(async () => useChatStore.setState({ workspaceRoot: '/synthetic/other', threads: [] }))
    await act(async () => { finish({ ok: true, skills: [{ id: 'audit', entryPath: '/synthetic/old/SKILL.md' }], validationErrors: [] }); await pending })
    expect(reference('/audit').tagName).toBe('SPAN')
    expect(resolve).not.toHaveBeenCalled()
  })

  it('does not open a retained old button after the active thread changes', async () => {
    Object.assign(window, { analytix: { files: { resolve: vi.fn(async () => ({ ok: true, path: `${workspace}/fixture.txt` })) } } })
    const previews = vi.fn(); window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews)
    try {
      await render({ ...block, text: '@fixture.txt' })
      const button = reference('@fixture.txt')
      await act(async () => {
        useChatStore.setState({ activeThreadId: 'other-thread' })
        button.click()
      })
      expect(previews).not.toHaveBeenCalled()
    } finally { window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previews) }
  })
})


describe('user reference explicit thread owner', () => {
  it('never resolves another thread message against the active workspace after render', async () => {
    const resolve = vi.fn(async () => ({ ok: true, path: '/synthetic/other/fixture.txt' }))
    const listSkills = vi.fn(async () => ({ ok: true, skills: [], validationErrors: [] }))
    Object.assign(window, { analytix: { files: { resolve }, app: { listSkills } } })
    await act(async () => useChatStore.setState({ activeThreadId: 'other-thread', workspaceRoot: '/synthetic/other',
      threads: [...useChatStore.getState().threads, { id: 'other-thread', title: 'Other', workspace: '/synthetic/other', updatedAt: '2026-10-05', model: 'synthetic', mode: 'agent' }] }))
    await render({ ...block, text: '@fixture.txt /audit', meta: { ...block.meta, activeSkillIds: ['audit'] } })
    expect(container.querySelector('[data-ref-chip="file"]')!.tagName).toBe('SPAN')
    expect(container.querySelector('[data-ref-chip="skill"]')!.tagName).toBe('SPAN')
    expect(resolve).not.toHaveBeenCalled(); expect(listSkills).not.toHaveBeenCalled()
  })
})


describe('nested user reference inherited thread owner', () => {
  it.each([threadId, null])('keeps a secondary or unknown context decorative (scope=%s)', async scope => {
    const resolve = vi.fn(async () => ({ ok: true, path: '/synthetic/other/fixture.txt' }))
    Object.assign(window, { analytix: { files: { resolve } } })
    await act(async () => useChatStore.setState({ activeThreadId: 'other-thread', workspaceRoot: '/synthetic/other' }))
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: scope },
      createElement(MessageBubble, { block: { ...block, text: '@fixture.txt' }, nested: true }))))
    expect(container.querySelector('[data-ref-chip="file"]')!.tagName).toBe('SPAN')
    expect(resolve).not.toHaveBeenCalled()
  })

  it('resolves a nested user reference through its actual active context', async () => {
    const resolve = vi.fn(async () => ({ ok: true, path: `${workspace}/fixture.txt` }))
    Object.assign(window, { analytix: { files: { resolve } } })
    await act(async () => root.render(createElement(ProtectedToolResultScope.Provider, { value: threadId },
      createElement(MessageBubble, { block: { ...block, text: '@fixture.txt' }, nested: true }))))
    expect(container.querySelector('[data-ref-chip="file"]')!.tagName).toBe('BUTTON')
    expect(resolve).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ path: 'fixture.txt', workspaceRoot: workspace }))
  })
})


describe('user reference path validation boundary', () => {
  it('keeps control characters and URL/UNC shapes decorative without resolving them', async () => {
    const resolve = vi.fn(async () => ({ ok: true, path: `${workspace}/forged.txt` }))
    Object.assign(window, { analytix: { files: { resolve } } })
    await render({ ...block, text: '@docs/\u0000file.txt @docs/\u001ffile.txt @docs/\u007ffile.txt @https://example.invalid/file @//server/file' })
    const references = [...container.querySelectorAll('[data-user-message-bubble] [data-ref-chip]')]
    expect(references).toHaveLength(5)
    expect(references.every(reference => reference.tagName === 'SPAN')).toBe(true)
    expect(resolve).not.toHaveBeenCalled()
  })
})
