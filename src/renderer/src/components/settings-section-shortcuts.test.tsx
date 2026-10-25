// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { KeyboardShortcutsSettingsSection } from './settings-section-shortcuts'
import { useChatStore } from '../store/chat-store'
import { resolveKeyboardShortcutBindings, type KeyboardShortcutsConfigV1 } from '@shared/app-settings'

const initialStore = useChatStore.getState()
let root: Root, host: HTMLDivElement
let shortcuts: KeyboardShortcutsConfigV1
let update: ReturnType<typeof vi.fn>
let notice: Mock<typeof initialStore.showTopNotice>
async function render() {
  await act(async () => root.render(createElement(KeyboardShortcutsSettingsSection, { ctx: {
    t: (key: string, options?: { command: string }) => options ? `${key}:${options.command}` : key,
    form: { keyboardShortcuts: shortcuts }, update
  } })))
}
function row(label: string) {
  return [...host.querySelectorAll('div')].find(node => node.textContent === label)!.parentElement!.parentElement!
}
async function press(button: HTMLButtonElement) { await act(async () => button.click()) }
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  notice = vi.fn(); useChatStore.setState({ showTopNotice: notice })
  shortcuts = { bindings: { 'new-chat': ['Ctrl+Shift+N'], settings: ['Ctrl+N'] } }
  update = vi.fn((patch: { keyboardShortcuts: KeyboardShortcutsConfigV1 }) => { shortcuts = patch.keyboardShortcuts })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host); await render()
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); useChatStore.setState(initialStore); vi.unstubAllGlobals() })

describe('Keyboard shortcut updates', () => {
  it('rejects reset when its default is owned by another command', async () => {
    await press(row('shortcutNewChat').querySelector<HTMLButtonElement>('[aria-label="shortcutReset"]')!)
    expect(update).not.toHaveBeenCalled()
    expect(notice).toHaveBeenCalledWith({ tone: 'error', message: 'shortcutConflict:shortcutSettings' })
    expect(resolveKeyboardShortcutBindings(shortcuts)['new-chat']).toEqual(['Ctrl+Shift+N'])
  })
  it('restores defaults when reset has no conflict', async () => {
    shortcuts = { bindings: { ...shortcuts.bindings, settings: ['Ctrl+Shift+S'] } }; await render()
    await press(row('shortcutNewChat').querySelector<HTMLButtonElement>('[aria-label="shortcutReset"]')!)
    expect(update).toHaveBeenCalledExactlyOnceWith({ keyboardShortcuts: { bindings: { 'new-chat': [], settings: ['Ctrl+Shift+S'] } } })
    expect(resolveKeyboardShortcutBindings(shortcuts)['new-chat']).toEqual(['Ctrl+N'])
  })
  it('keeps capture after conflict and accepts a free key through the same check', async () => {
    await press(row('shortcutNewChat').querySelector('button')!)
    await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, bubbles: true })))
    expect(update).not.toHaveBeenCalled(); expect(host.textContent).toContain('shortcutRecording')
    await act(async () => window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', ctrlKey: true, bubbles: true })))
    expect(update).toHaveBeenCalledExactlyOnceWith({ keyboardShortcuts: { bindings: { 'new-chat': ['Ctrl+J'], settings: ['Ctrl+N'] } } })
    expect(host.textContent).not.toContain('shortcutRecording')
  })
})
