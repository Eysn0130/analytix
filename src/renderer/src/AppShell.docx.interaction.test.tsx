// @vitest-environment jsdom
import { act, createElement, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import type { DocxEditorView } from '@shared/docx-editor'
import AppShell, { prewarmAppShellSurfaces } from './AppShell'
import { rendererRuntimeClient } from './agent/runtime-client'
import i18n from './i18n'
import { useDocxEditorStore } from './office/docx-editor-store'
import { useChatStore } from './store/chat-store'

const state = vi.hoisted(() => ({ mounts: 0, unmounts: 0 }))
vi.mock('./components/Workbench', () => ({ Workbench: () => {
  const [draft, setDraft] = useState(0)
  useEffect(() => { state.mounts++; return () => { state.unmounts++ } }, [])
  return createElement('div', { 'data-test-word-owner': true },
    createElement('button', { 'data-test-word-draft': true, onClick: () => setDraft(value => value + 1) }, `Draft ${draft}`),
    createElement('button', { 'data-test-open-settings': true,
      onClick: () => useChatStore.getState().openSettings('general') }, 'Open settings'))
} }))
// Keep the real SettingsView, Dialog and focus lifecycle; only section bodies
// and desktop reads are fixtures. This test does not exercise Provider access.
vi.mock('./components/settings-sections', async () => {
  const { createElement: h } = await import('react')
  const empty = () => null
  return {
    ...Object.fromEntries(['Providers', 'Write', 'ImageGeneration', 'SpeechToText', 'Agents',
      'Browser', 'ComputerUse', 'ArchivedThreads', 'Permissions', 'Worktree', 'Memory',
      'Claw', 'KeyboardShortcuts', 'Updates'].map(name => [`${name}SettingsSection`, empty])),
    GeneralSettingsSection: () => h('input', { 'aria-label': 'Fixture settings field', 'data-modal-autofocus': true })
  }
})
vi.mock('./components/InitialSetupDialog', () => ({ InitialSetupDialog: () => null }))
vi.mock('./components/RuntimeStatusBanner', () => ({ RuntimeStatusBanner: () => null }))
vi.mock('./components/TopNoticeHost', () => ({ TopNoticeHost: () => null }))
await prewarmAppShellSurfaces()

it.each(['Cancel', 'Escape', 'backdrop'] as const)(
  'retains the Word owner and draft while real Settings isolates focus and closes through %s', async close => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    vi.stubGlobal('fetch', () => { throw new Error('Unexpected fixture network') })
    vi.stubGlobal('WebSocket', class { constructor() { throw new Error('Unexpected fixture network') } })
    const previousChat = useChatStore.getState(), previousDocx = useDocxEditorStore.getState()
    const previousBridge = Object.getOwnPropertyDescriptor(window, 'analytix')
    const previousLanguage = i18n.language
    const previousAppearance = new Map(['style', 'data-theme', 'data-accent', 'data-cursor-spotlight',
      'data-motion-preference', 'data-motion-reduced'].map(name => [name, document.documentElement.getAttribute(name)]))
    const previousOverflow = document.body.style.overflow
    Object.defineProperty(window, 'analytix', { configurable: true, value: {
      app: { platform: 'darwin' }, write: {}, updates: {}, logs: { error: vi.fn(async () => undefined) }
    } })
    const settings = normalizeAppSettings({ theme: 'light', locale: 'en', motionPreference: 'on',
      cursorSpotlight: false, workspaceRoot: '/fixture/word-settings' } as AppSettingsV1)
    vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(structuredClone(settings))
    vi.spyOn(rendererRuntimeClient, 'readSettingsFromHost').mockResolvedValue(structuredClone(settings))
    const write = vi.spyOn(rendererRuntimeClient, 'setSettings').mockImplementation(async () => {
      throw new Error('Closing an unchanged Settings fixture must not write')
    })
    const view = { editorId: 'word', phase: 'editing', dirty: true } as DocxEditorView
    state.mounts = 0; state.unmounts = 0
    useDocxEditorStore.setState({ views: { word: view } })
    useChatStore.setState({ route: 'chat', initialSetupOpen: false, settingsSection: 'general',
      applyI18nFromSettings: vi.fn(async () => undefined), reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => undefined), showTopNotice: vi.fn() })
    const container = document.createElement('div'); document.body.append(container)
    const root = createRoot(container)
    const key = async (target: HTMLElement, value: string, shiftKey = false) => {
      const event = new KeyboardEvent('keydown', { key: value, shiftKey, bubbles: true, cancelable: true })
      await act(async () => { target.dispatchEvent(event) })
      return event
    }
    try {
      await i18n.changeLanguage('en')
      await act(async () => root.render(createElement(AppShell)))
      const owner = container.querySelector<HTMLElement>('[data-test-word-owner]')!
      const draft = owner.querySelector<HTMLButtonElement>('[data-test-word-draft]')!
      const opener = owner.querySelector<HTMLButtonElement>('[data-test-open-settings]')!
      await act(async () => draft.click())
      expect(draft.textContent).toBe('Draft 1')
      opener.focus()
      await act(async () => opener.click())

      const dialog = container.querySelector<HTMLElement>('[role="dialog"][aria-modal="true"]')!
      expect(container.querySelectorAll('[role="dialog"][aria-modal="true"]')).toHaveLength(1)
      expect(dialog).not.toBeNull()
      expect(dialog.closest('[inert]')).toBeNull()
      expect(container.querySelector('[data-test-word-owner]')).toBe(owner)
      expect(owner.isConnected).toBe(true)
      expect(owner.closest('[inert]')).not.toBeNull()
      expect(owner.closest('[hidden]')).toBeNull()
      expect(draft.textContent).toBe('Draft 1')
      expect(useDocxEditorStore.getState().views.word).toBe(view)
      expect(state.mounts).toBe(1); expect(state.unmounts).toBe(0)
      expect(dialog.contains(document.activeElement)).toBe(true)
      expect(document.body.style.overflow).toBe('hidden')

      // jsdom does not implement native inert hit testing. Exercise the actual
      // focus guard and modal event routing, not a synthetic background click.
      await act(async () => draft.focus())
      expect(document.activeElement).not.toBe(draft)
      expect(dialog.contains(document.activeElement)).toBe(true)
      const closeButton = dialog.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('settings:settingsClose')}"]`)!
      const cancel = [...dialog.querySelectorAll<HTMLButtonElement>('button')]
        .find(button => button.textContent?.trim() === i18n.t('settings:settingsCancel'))!
      cancel.focus()
      expect((await key(cancel, 'Tab')).defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(closeButton)
      expect((await key(closeButton, 'Tab', true)).defaultPrevented).toBe(true)
      expect(document.activeElement).toBe(cancel)
      const settingsField = dialog.querySelector<HTMLInputElement>('input[aria-label="Fixture settings field"]')!
      await act(async () => { settingsField.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
      expect(useChatStore.getState().route).toBe('settings')
      expect(dialog.isConnected).toBe(true)

      if (close === 'Cancel') await act(async () => cancel.click())
      else if (close === 'Escape') expect((await key(cancel, 'Escape')).defaultPrevented).toBe(true)
      else {
        const mask = dialog.closest<HTMLElement>('.ui-dialog-mask')!
        await act(async () => { mask.dispatchEvent(new MouseEvent('mousedown', { bubbles: true })) })
      }
      expect(useChatStore.getState().route).toBe('chat')
      expect(container.querySelector('[role="dialog"]')).toBeNull()
      expect(container.querySelector('[data-test-word-owner]')).toBe(owner)
      expect(owner.closest('[inert]')).toBeNull()
      expect(document.activeElement).toBe(opener)
      expect(document.body.style.overflow).toBe(previousOverflow)
      expect(draft.textContent).toBe('Draft 1')
      expect(useDocxEditorStore.getState().views.word).toBe(view)
      expect(state.mounts).toBe(1); expect(state.unmounts).toBe(0)
      expect(write).not.toHaveBeenCalled()
      await act(async () => draft.click())
      expect(draft.textContent).toBe('Draft 2')
      expect(useDocxEditorStore.getState().views.word).toBe(view)
    } finally {
      await act(async () => root.unmount()); container.remove()
      useChatStore.setState(previousChat, true); useDocxEditorStore.setState(previousDocx, true)
      await i18n.changeLanguage(previousLanguage)
      for (const [name, value] of previousAppearance) {
        if (value === null) document.documentElement.removeAttribute(name)
        else document.documentElement.setAttribute(name, value)
      }
      document.body.style.overflow = previousOverflow
      vi.restoreAllMocks(); vi.unstubAllGlobals()
      if (previousBridge) Object.defineProperty(window, 'analytix', previousBridge)
      else Reflect.deleteProperty(window, 'analytix')
    }
  }
)
