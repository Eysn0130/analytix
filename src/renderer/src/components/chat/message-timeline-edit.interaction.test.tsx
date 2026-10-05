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
async function render() {
  await act(async () => root.render(createElement('div', null, createElement(MessageBubble, { block }), createElement(Composer))))
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
describe('actual message edit to composer draft continuity', () => {
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
