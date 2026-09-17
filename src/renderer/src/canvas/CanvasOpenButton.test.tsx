// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import { useWorkspaceTabsStore } from '../store/workspace-tabs-store'
import { WORKSPACE_FILE_PREVIEW_EVENT } from '../lib/workspace-file-preview'
import { CanvasOpenButton } from './CanvasOpenButton'

let root: Root, container: HTMLDivElement
const pickFile = vi.fn()
const previewEvent = vi.fn()
const initialChat = useChatStore.getState()
const initialTabs = useWorkspaceTabsStore.getState()
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('analytix', { canvas: { pickFile } })
  pickFile.mockReset(); previewEvent.mockReset()
  await i18n.changeLanguage('en')
  useChatStore.setState({ activeThreadId: 'thread-a', threads: [], workspaceRoot: '/project' })
  useWorkspaceTabsStore.setState({ tabs: [], activeTabId: null, selectorOpen: true, open: true })
  window.addEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previewEvent)
  container = document.createElement('div'); document.body.append(container); root = createRoot(container)
  await act(async () => root.render(createElement(CanvasOpenButton, { enabled: true })))
})
afterEach(async () => {
  await act(async () => root.unmount()); container.remove()
  window.removeEventListener(WORKSPACE_FILE_PREVIEW_EVENT, previewEvent)
  useChatStore.setState(initialChat); useWorkspaceTabsStore.setState(initialTabs)
  vi.unstubAllGlobals()
})
const button = () => container.querySelector('button')!
const click = () => act(async () => { button().click() })
describe('explicit Canvas entry in the existing workspace', () => {
  it('mounting and hovering do not pick, open, send or change the current thread', async () => {
    await act(async () => { button().dispatchEvent(new MouseEvent('mouseover', { bubbles: true })) })
    expect(pickFile).not.toHaveBeenCalled()
    expect(previewEvent).not.toHaveBeenCalled()
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
    expect(useChatStore.getState().activeThreadId).toBe('thread-a')
  })
  it('opens a chosen object using the ordinary file tab and preview event', async () => {
    pickFile.mockResolvedValue({ ok: true, path: '/project/diagram.canvas' })
    await click()
    expect(pickFile).toHaveBeenCalledExactlyOnceWith({ workspace: '/project' })
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([expect.objectContaining({ kind: 'file', mode: 'file', preview: 'canvas', workspaceRoot: '/project', path: '/project/diagram.canvas' })])
    expect(previewEvent).toHaveBeenCalledTimes(1)
    expect((previewEvent.mock.calls[0][0] as CustomEvent).detail).toEqual({ workspaceRoot: '/project', path: '/project/diagram.canvas' })
    expect(useChatStore.getState().activeThreadId).toBe('thread-a')
  })
  it('does not manufacture a tab when the user cancels or selects an unsupported result', async () => {
    pickFile.mockResolvedValueOnce({ ok: true, path: null }).mockResolvedValueOnce({ ok: true, path: '/project/unexpected.html' })
    await click(); expect(container.querySelector('[role="alert"]')).toBeNull()
    await click(); expect(container.querySelector('[role="alert"]')).not.toBeNull()
    expect(previewEvent).not.toHaveBeenCalled()
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
  })
  it('rejects a late picker result even after switching away and back to the same context', async () => {
    let resolve!: (value: { ok: boolean; path: string }) => void
    pickFile.mockImplementation(() => new Promise(done => { resolve = done }))
    await click()
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-b' }))
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-a' }))
    await act(async () => resolve({ ok: true, path: '/project/late.canvas' }))
    expect(previewEvent).not.toHaveBeenCalled()
    expect(useWorkspaceTabsStore.getState().tabs).toEqual([])
  })
  it('disables missing-context picking and coalesces double clicks while pending', async () => {
    await act(async () => useChatStore.setState({ activeThreadId: null }))
    await click(); expect(pickFile).not.toHaveBeenCalled()
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-a' }))
    let resolve!: (value: { ok: boolean; path: null }) => void
    pickFile.mockImplementation(() => new Promise(done => { resolve = done }))
    await click(); await click()
    expect(pickFile).toHaveBeenCalledTimes(1)
    await act(async () => resolve({ ok: true, path: null }))
  })
  it('ignores picker completion after the selector is unmounted', async () => {
    let resolve!: (value: { ok: boolean; path: string }) => void
    pickFile.mockImplementation(() => new Promise(done => { resolve = done }))
    await click()
    await act(async () => root.render(null))
    await act(async () => resolve({ ok: true, path: '/project/late.png' }))
    expect(previewEvent).not.toHaveBeenCalled()
  })
})
