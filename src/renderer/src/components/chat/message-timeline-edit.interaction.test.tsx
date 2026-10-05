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

const registry = vi.hoisted(() => ({ getProvider: vi.fn() }))
vi.mock('../../agent/registry', () => ({ getProvider: registry.getProvider }))

let root: Root
let container: HTMLDivElement
let previousState: ReturnType<typeof useChatStore.getState>
let previousBridge: typeof window.analytix
let composer: ReturnType<typeof useThreadComposerDraft>
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
  await act(async () => root.render(createElement('div', null, createElement(MessageBubble, { block: displayBlock }), createElement(Composer))))
}
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  await i18n.changeLanguage('zh')
  previousState = useChatStore.getState()
  previousBridge = window.analytix
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
  vi.restoreAllMocks(); vi.unstubAllGlobals()
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
