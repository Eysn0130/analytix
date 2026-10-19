// @vitest-environment jsdom
import { act, createElement, Profiler, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { buildThreadEventSink } from '../../store/chat-store-runtime'
import { getProvider } from '../../agent/registry'
import { dispatchAnalytixRuntimeEvent } from '../../agent/analytix-mapper'
import { InputBar } from './InputBar'
import { Dialog } from '../../data-analysis/components/overlay/Dialog'
import * as capacityEstimates from '../../lib/context-capacity'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { normalizeAppSettings, defaultModelProviderSettings, defaultAnalytixRuntimeSettings, normalizeAppBehaviorSettings,
  defaultKeyboardShortcuts, defaultWriteSettings, defaultClawSettings, defaultScheduleSettings, type AppSettingsV1 } from '@shared/app-settings'
import { ProviderBillingV1Schema, ConversationMetricsV1Schema } from '../../../../../packages/runtime/src/contracts/usage'
import * as voiceDictation from './use-voice-dictation'
import type { ThreadUsageSnapshot, ChatBlock, ToolBlock, ThreadGoalStatus } from '../../agent/types'
import { ProtectedToolResultScope } from './presentation/ProtectedToolResult'
import { ToolCallTree } from './presentation/chat/ToolCallTree'
import { presentationPolicyFor } from './presentation/chat/chat-view-model'
import { userInputLocalSelectorSchemaV1, userInputLocalViewRequestSchemaV1,
  type UserInputLocalDisplayV1, type UserInputLocalSaveDraftRequestV1, type UserInputLocalViewRequestV1 } from '../../../../../packages/runtime/src/contracts/user-input-local-display'
import { isClosedPublicRuntimeSseEvent, projectPublicRuntimeSseBlock } from '../../../../shared/public-runtime-sse'
import { PublicRuntimeEventFilter } from '../../../../shared/public-runtime-content'

function floatingElement(busy = false, showThreadContextPanels = false, overrides: Partial<ComponentProps<typeof InputBar>> = {}) {
  return createElement(InputBar, {
    input: '', setInput: vi.fn(), mode: 'agent', setMode: vi.fn(), busy,
    runtimeReady: true, hasActiveThread: true,
    composerModel: '', composerPickList: [], onComposerModelChange: vi.fn(),
    queuedMessages: [], onRemoveQueuedMessage: vi.fn(), onSend: vi.fn(), onInterrupt: vi.fn(),
    onPlanCommand: vi.fn(), onNewCommand: vi.fn(), fileReferenceEnabled: true,
    attachmentUploadEnabled: true, hideThreadContextPanels: !showThreadContextPanels, ...overrides
  })
}

let root: Root, host: HTMLDivElement
let previousApi: PropertyDescriptor | undefined
let previous: ReturnType<typeof useChatStore.getState>
beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => window.setTimeout(() => callback(0), 0))
  vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id))
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  previous = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'menu-fixture', activeThreadGoal: null, route: 'chat',
    workspaceRoot: '/workspace/fixture', blocks: [], busy: false, userInputDraftInterruptions: {}, deletedUserInputDraftThreadIds: {} })
  await i18n.changeLanguage('zh')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  await act(async () => root.render(floatingElement()))
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); useChatStore.setState(previous)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
})
const trigger = () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerMenuTitle')}"]`)!
const menu = () => host.querySelector<HTMLElement>('[role="menu"]')
async function open() {
  // Commit the menu before waiting for its autofocus frame.
  await act(async () => { trigger().click() })
  await act(async () => { await new Promise<void>(resolve => requestAnimationFrame(() => resolve())) })
  expect(menu()).not.toBeNull()
  expect(document.activeElement).toBe(menu()!.querySelector('button:not(:disabled)'))
}
async function key(target: HTMLElement, value: string, options: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...options })
  await act(async () => target.dispatchEvent(event))
  return event
}

describe('ordinary composer keyboard submission and busy follow-ups', () => {
  const geometry = ['getClientRects', 'getBoundingClientRect'] as const
  let rangeGeometry: Array<PropertyDescriptor | undefined>
  beforeEach(() => {
    // jsdom has no layout; ProseMirror still measures a browser selection.
    rangeGeometry = geometry.map(name => Object.getOwnPropertyDescriptor(Range.prototype, name))
    Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
    Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
  })
  afterEach(() => geometry.forEach((name, index) => {
    const descriptor = rangeGeometry[index]
    if (descriptor) Object.defineProperty(Range.prototype, name, descriptor)
    else Reflect.deleteProperty(Range.prototype, name)
  }))
  const editor = () => host.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
  describe('composer autofocus and restored settings focus', () => {
    let frames: Map<number, FrameRequestCallback>, nextFrame: number
    const draft = 'Synthetic retained composer draft'
    const focusComposer = () => floatingElement(false, false, {
      input: draft, runtimeReady: false, hasActiveThread: false, voiceDictationEnabled: false
    })
    const flushFrame = async () => {
      const pending = [...frames.values()]
      frames.clear()
      await act(async () => { for (const callback of pending) callback(0) })
    }
    beforeEach(async () => {
      // Dispose the outer fixture before replacing its timer-backed rAF implementation.
      await act(async () => root.render(null))
      frames = new Map(); nextFrame = 0
      vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
        frames.set(++nextFrame, callback)
        return nextFrame
      })
      vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
      useChatStore.setState({ activeThreadId: null, activeThreadGoal: null, route: 'chat' })
    })
    function SettingsFocusFixture() {
      const route = useChatStore(state => state.route)
      return createElement('div', null,
        createElement('div', { inert: route === 'settings', 'data-settings-background': '' },
          createElement('button', { 'data-settings-opener': '',
            onClick: () => useChatStore.getState().openSettings() }, 'Settings'),
          focusComposer()),
        route === 'settings' ? createElement(Dialog, {
          open: true, title: 'Settings', closeAriaLabel: 'Close settings',
          onClose: () => useChatStore.getState().setRoute('chat'),
          children: createElement('p', null, 'Synthetic settings content')
        }) : null)
    }
    it.each(['Escape', 'close', 'backdrop'] as const)('keeps the focused Settings opener after %s and all queued focus frames', async method => {
      await act(async () => root.render(createElement(SettingsFocusFixture)))
      await flushFrame(); await flushFrame()
      const opener = host.querySelector<HTMLButtonElement>('[data-settings-opener]')!
      opener.focus()
      expect(document.activeElement).toBe(opener)
      await act(async () => opener.click())
      const close = host.querySelector<HTMLButtonElement>('button[aria-label="Close settings"]')!
      expect(document.activeElement).toBe(close)
      expect(host.querySelector('[data-settings-background]')?.hasAttribute('inert')).toBe(true)
      if (method === 'Escape') await key(close, 'Escape')
      else if (method === 'close') await act(async () => close.click())
      else {
        const mask = host.querySelector<HTMLElement>('.ui-dialog-mask')!
        const press = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 })
        await act(async () => { mask.dispatchEvent(press) })
        // jsdom does not perform the browser's native mousedown focus default.
        expect(press.defaultPrevented).toBe(true)
      }
      // The real modal's microtask restores its opener before the InputBar rAF runs.
      expect(document.activeElement).toBe(opener)
      await flushFrame(); await flushFrame()
      expect(useChatStore.getState().route).toBe('chat')
      expect(host.querySelector('[role="dialog"]')).toBeNull()
      expect(host.querySelector('[data-settings-background]')?.hasAttribute('inert')).toBe(false)
      expect(document.activeElement).toBe(opener)
      expect(editor().textContent).toBe(draft)
    })
    it('preserves native mousedown behavior inside the dialog without closing settings', async () => {
      await act(async () => root.render(createElement(SettingsFocusFixture)))
      await flushFrame(); await flushFrame()
      const opener = host.querySelector<HTMLButtonElement>('[data-settings-opener]')!
      await act(async () => { opener.focus(); opener.click() })
      const close = host.querySelector<HTMLButtonElement>('button[aria-label="Close settings"]')!
      const press = new MouseEvent('mousedown', { bubbles: true, cancelable: true, button: 0 })
      await act(async () => { close.dispatchEvent(press) })
      expect(press.defaultPrevented).toBe(false)
      expect(useChatStore.getState().route).toBe('settings')
      expect(host.querySelector('[role="dialog"]')).not.toBeNull()
      expect(document.activeElement).toBe(close)
      expect(editor().textContent).toBe(draft)
    })
    it('preserves a button that takes focus after composer autofocus has been queued', async () => {
      await act(async () => root.render(createElement('div', null,
        createElement('button', { 'data-other-action': '' }, 'Another action'), focusComposer())))
      expect(frames.size).toBeGreaterThan(0)
      const otherAction = host.querySelector<HTMLButtonElement>('[data-other-action]')!
      otherAction.focus()
      expect(document.activeElement).toBe(otherAction)
      await flushFrame(); await flushFrame()
      expect(document.activeElement).toBe(otherAction)
      expect(editor().textContent).toBe(draft)
    })
    it('preserves the composer menu trigger focused after autofocus has been queued', async () => {
      await act(async () => root.render(floatingElement(false, false, {
        input: draft, runtimeReady: true, hasActiveThread: false, voiceDictationEnabled: false
      })))
      expect(frames.size).toBeGreaterThan(0)
      const menuTrigger = trigger()
      expect(menuTrigger.disabled).toBe(false)
      menuTrigger.focus()
      expect(document.activeElement).toBe(menuTrigger)
      await flushFrame(); await flushFrame()
      expect(document.activeElement).toBe(menuTrigger)
      expect(editor().textContent).toBe(draft)
    })
    it('still focuses the composer when no external control owns focus', async () => {
      expect(document.activeElement).toBe(document.body)
      await act(async () => root.render(focusComposer()))
      await flushFrame(); await flushFrame()
      expect(document.activeElement).toBe(editor())
      expect(editor().textContent).toBe(draft)
    })
  })

  const expectConversationGlyph = (button: HTMLButtonElement, kind: 'send' | 'stop') => {
    const svg = button.querySelector('svg')!
    expect(svg.getAttribute('viewBox')).toBe('0 0 16 16')
    if (kind === 'send') {
      expect(svg.querySelector('path')?.getAttribute('d')).toBe('M8.3125 0.980183C8.66767 1.0531 8.97902 1.20418 9.2627 1.43233C9.48724 1.61297 9.73029 1.85793 9.97949 2.10714L14.707 6.83468L13.293 8.24874L9 3.95577V15.0417H7V3.95577L2.70703 8.24874L1.29297 6.83468L6.02051 2.10714C6.26971 1.85793 6.51277 1.61297 6.7373 1.43233C6.97662 1.23986 7.28445 1.04402 7.6875 0.980183C7.8973 0.947006 8.1031 0.95516 8.3125 0.980183Z')
      expect(svg.querySelector('path')?.getAttribute('fill')).toBe('currentColor')
    } else {
      const rect = svg.querySelector('rect')!
      expect(['x', 'y', 'width', 'height', 'rx', 'fill'].map(name => rect.getAttribute(name)))
        .toEqual(['3', '3', '10', '10', '3', 'currentColor'])
    }
  }
  it.each([{ ctrlKey: true }, { metaKey: true }])('composer closure sends an explicit busy queue intent for the accelerated chord: %j', async modifiers => {
    const onSend = vi.fn(), onInterrupt = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { input: 'Follow up', onSend, onInterrupt })))
    await key(editor(), 'Enter', modifiers)
    expect(onSend.mock.calls).toEqual([['queue']])
    await key(editor(), 'Enter')
    await act(async () => host.querySelector<HTMLButtonElement>('.ds-composer-primary-action-button')!.click())
    expect(onSend.mock.calls).toEqual([['queue'], [], []])
    expect(onInterrupt).not.toHaveBeenCalled()
  })
  it('composer closure keeps a failed attachment visible and blocks submission until resolved', async () => {
    const onSend = vi.fn(), retry = vi.fn(), remove = vi.fn()
    const upload = { id: 'pending-file', attemptId: 'first', file: new File(['synthetic'], 'failed.pdf'), status: 'failed' as const, error: 'Synthetic failure' }
    await act(async () => root.render(floatingElement(false, false, { input: 'Keep my file', attachmentUploads: [upload],
      onSend, onRetryAttachmentUpload: retry, onRemoveAttachmentUpload: remove })))
    expect(host.textContent).toContain('failed.pdf')
    expect(host.querySelector<HTMLButtonElement>('.ds-composer-primary-action-button')!.disabled).toBe(true)
    await key(editor(), 'Enter')
    expect(onSend).not.toHaveBeenCalled()
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerRetryAttachment')}"]`)!.click())
    expect(retry).toHaveBeenCalledWith(upload.id)
    await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerRemoveAttachment')}"]`)!.click())
    expect(remove).toHaveBeenCalledWith(upload.id)
  })
  it('submits a draft once but never interrupts from held or empty Enter', async () => {
    const onSend = vi.fn(), onInterrupt = vi.fn()
    await act(async () => root.render(floatingElement(false, false, { input: 'A request', onSend, onInterrupt })))
    expect(host.querySelector('[data-main-chat-composer]')).not.toBeNull()
    const send = host.querySelector<HTMLButtonElement>('.ds-composer-primary-action-button')!
    expect(send.hasAttribute('data-analytix-chat-token-scope')).toBe(true)
    expectConversationGlyph(send, 'send')
    await key(editor(), 'Enter'); expect(onSend).toHaveBeenCalledOnce()
    await act(async () => root.render(floatingElement(true, false, { onSend, onInterrupt })))
    await key(editor(), 'Enter', { repeat: true }); await key(editor(), 'Enter')
    expect(onSend).toHaveBeenCalledOnce(); expect(onInterrupt).not.toHaveBeenCalled()
    const stop = host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:interrupt')}"]`)!
    expect(stop.classList.contains('ds-composer-primary-action-button')).toBe(true)
    expectConversationGlyph(stop, 'stop')
    await act(async () => stop.click()); expect(onInterrupt).toHaveBeenCalledOnce()
  })
  it.each([{ altKey: true }, { isComposing: true }, { keyCode: 229 }, { repeat: true }])('does not submit from modified, composing or repeated Enter: %j', async options => {
    const onSend = vi.fn(), onInterrupt = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { input: 'A request', onSend, onInterrupt })))
    await key(editor(), 'Enter', options)
    expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
  })
  it.each([
    { attachments: [{ id: 'attachment', name: 'fixture.png', mimeType: 'image/png' }] },
    { documentReferenceCount: 1 },
    { fileReferences: [{ relativePath: 'fixture.txt', path: '/workspace/fixture/fixture.txt', name: 'fixture.txt' }] }
  ])('submits supported reference-only busy follow-ups without interrupting: %j', async references => {
    const onSend = vi.fn(), onInterrupt = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { ...references, onSend, onInterrupt })))
    await key(editor(), 'Enter'); expect(onSend).toHaveBeenCalledOnce(); expect(onInterrupt).not.toHaveBeenCalled()
    const send = host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerSteerSend')}"]`)!
    const stop = host.querySelector<HTMLButtonElement>('.ds-composer-secondary-stop')!
    expectConversationGlyph(send, 'send'); expectConversationGlyph(stop, 'stop')
    await act(async () => send.click()); expect(onSend).toHaveBeenCalledTimes(2); expect(onInterrupt).not.toHaveBeenCalled()
    await act(async () => stop.click()); expect(onInterrupt).toHaveBeenCalledOnce(); expect(onSend).toHaveBeenCalledTimes(2)
  })
  it.each(['compact', 'secondary', 'claw'] as const)('keeps the existing primary glyph outside the main chat composer: %s', async owner => {
    await act(async () => {
      if (owner === 'claw') useChatStore.setState({ route: 'claw' })
      root.render(floatingElement(false, false, owner === 'compact' ? { variant: 'compact' } : owner === 'secondary' ? { ownerThreadId: null } : {}))
    })
    expect(host.querySelector('[data-main-chat-composer]')).toBeNull()
    const primary = host.querySelector<HTMLButtonElement>('.ds-composer-primary-action-button')!
    expect(primary.hasAttribute('data-analytix-chat-token-scope')).toBe(false)
    expect(primary.querySelector('svg')?.getAttribute('viewBox')).toBe('0 0 24 24')
  })
  it('selects a busy slash command before submitting and ignores held Enter', async () => {
    const onSend = vi.fn(), onInterrupt = vi.fn(), onNewCommand = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { input: '/new', onSend, onInterrupt, onNewCommand })))
    await key(editor(), 'Enter', { repeat: true }); expect(onNewCommand).not.toHaveBeenCalled()
    await key(editor(), 'Enter'); expect(onNewCommand).toHaveBeenCalledOnce()
    expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
  })
  it('keeps AltGraph and composition-owned Enter out of submission while Shift+Enter inserts a newline', async () => {
    let now = Date.now(); vi.spyOn(Date, 'now').mockImplementation(() => now)
    const onSend = vi.fn(), onInterrupt = vi.fn(), setInput = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { input: 'A request', onSend, onInterrupt, setInput })))
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    Object.defineProperty(event, 'getModifierState', { value: (name: string) => name === 'AltGraph' })
    await act(async () => editor().dispatchEvent(event))
    await act(async () => editor().dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true })))
    await key(editor(), 'Enter')
    expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
    await act(async () => editor().dispatchEvent(new CompositionEvent('compositionend', { bubbles: true })))
    await key(editor(), 'Enter')
    now += 9; await key(editor(), 'Enter')
    expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
    now += 1; setInput.mockClear()
    await key(editor(), 'Enter', { shiftKey: true })
    expect(setInput.mock.calls.some(([value]) => value.includes('\n'))).toBe(true)
    expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
    await key(editor(), 'Enter')
    expect(onSend).toHaveBeenCalledOnce(); expect(onInterrupt).not.toHaveBeenCalled()
  })
  it('does not turn uploading Enter into Stop', async () => {
    const onSend = vi.fn(), onInterrupt = vi.fn()
    await act(async () => root.render(floatingElement(true, false, { input: 'A request', attachmentUploadBusy: true, onSend, onInterrupt })))
    await key(editor(), 'Enter'); expect(onSend).not.toHaveBeenCalled(); expect(onInterrupt).not.toHaveBeenCalled()
  })
})

describe('ordinary composer plus-menu keyboard ownership', () => {
  it('keeps open plus-menu focus while the same input moves from hero to the conversation dock', async () => {
    await act(async () => root.render(floatingElement(false, false, { presentationPhase: 'hero' })))
    await open()
    const focusedItem = document.activeElement
    await act(async () => root.render(floatingElement(false, false, { presentationPhase: 'active' })))
    await act(async () => { await new Promise<void>(resolve => requestAnimationFrame(() => resolve())) })
    expect(menu()).not.toBeNull()
    expect(document.activeElement).toBe(focusedItem)
    expect(menu()!.contains(document.activeElement)).toBe(true)
    await key(focusedItem as HTMLElement, 'Escape')
    expect(menu()).toBeNull()
    expect(document.activeElement).toBe(trigger())
  })
  it.each([false, true])('closes on exit Tab without preventing native navigation (shift=%s)', async shiftKey => {
    await open()
    expect(menu()).not.toBeNull()
    const current = document.activeElement as HTMLElement
    expect(menu()!.contains(current)).toBe(true)
    const event = await key(current, 'Tab', { shiftKey })
    expect(event.defaultPrevented).toBe(false)
    expect(menu()).toBeNull()
    expect(trigger().getAttribute('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger())
  })
  it('keeps arrow/Home/End and Escape behavior while closing when focus leaves', async () => {
    await open()
    expect(menu()!.querySelector('button:disabled')).not.toBeNull()
    const items = [...menu()!.querySelectorAll<HTMLButtonElement>('button:not(:disabled)')]
    await key(items[0], 'End'); expect(document.activeElement).toBe(items.at(-1))
    await key(items.at(-1)!, 'Home'); expect(document.activeElement).toBe(items[0])
    await key(items[0], 'ArrowUp'); expect(document.activeElement).toBe(items.at(-1))
    await key(items.at(-1)!, 'Escape')
    expect(menu()).toBeNull(); expect(document.activeElement).toBe(trigger())
    await open()
    const outside = document.createElement('button'); document.body.append(outside)
    try {
      await act(async () => outside.focus())
      expect(menu()).toBeNull()
      expect(document.activeElement).toBe(outside)
    } finally { outside.remove() }
  })
  it.each([{ key: 'Escape', isComposing: true }, { key: 'Tab', isComposing: true }, { key: 'ArrowDown', isComposing: true }, { key: 'Escape', keyCode: 229 }])('does not treat composition $key keystrokes as menu navigation or dismissal', async options => {
    await open()
    const current = document.activeElement as HTMLElement
    const event = await key(current, options.key, options)
    expect(event.defaultPrevented).toBe(false)
    expect(menu()).not.toBeNull()
    expect(document.activeElement).toBe(current)
  })
})


describe('actual floating Goal lifecycle controls', () => {
  const goal = { threadId: 'menu-fixture', id: 'goal-a', revision: 2, objective: 'Actual counted objective',
    status: 'active' as const, maxGoalRounds: 8, roundsStarted: 3, tokensUsed: 0, timeUsedSeconds: 0,
    createdAt: '2026-10-08T00:00:00Z', updatedAt: '2026-10-08T00:01:00Z' }
  const goalMenu = () => [...menu()!.querySelectorAll<HTMLButtonElement>('[role="menuitemcheckbox"]')]
    .find(button => button.textContent!.includes(i18n.t('common:composerMenuPursueGoal')))!
  it.each(['armed', 'disarmed', 'unknown', 'mismatched', 'legacy'] as const)('uses actual %s activation consistently in menu, row and panel', async state => {
    const action = vi.fn(async (_status: ThreadGoalStatus) => true)
    const current = state === 'legacy' ? { ...goal, id: undefined, revision: undefined, maxGoalRounds: undefined, roundsStarted: undefined } : goal
    await act(async () => {
      useChatStore.setState({ activeThreadGoal: current, setActiveThreadGoalStatus: action,
        activeGoalActivation: state === 'unknown' || state === 'legacy' ? null : {
          id: state === 'mismatched' ? 'other-goal' : goal.id, revision: goal.revision,
          activation: state === 'armed' || state === 'mismatched' ? 'armed' : 'disarmed' } })
      root.render(floatingElement(false, true))
    })
    const pause = state === 'armed' || state === 'legacy'
    await open()
    expect(goalMenu().getAttribute('aria-checked')).toBe(String(pause))
    await act(async () => goalMenu().click())
    expect(action).toHaveBeenLastCalledWith(pause ? 'paused' : 'active')
    const summary = host.querySelector<HTMLButtonElement>('.ds-composer-goal-summary')!
    await act(async () => summary.click())
    const label = i18n.t(`common:${pause ? 'goalActionPause' : 'goalActionResume'}`)
    const row = host.querySelector<HTMLElement>('.ds-composer-goal-details')!
    await act(async () => row.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!.click())
    expect(action).toHaveBeenCalledTimes(2)
    await act(async () => row.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:goalActionEdit')}"]`)!.click())
    const panelButton = [...host.querySelectorAll<HTMLButtonElement>(`button[aria-label="${label}"]`)].find(button => !row.contains(button))!
    await act(async () => panelButton.click())
    expect(action).toHaveBeenCalledTimes(3)
    expect(action.mock.calls.every(([status]) => status === (pause ? 'paused' : 'active'))).toBe(true)
  })
  it('keeps exhausted Resume disabled while preserving explicit budget edit and Clear', async () => {
    const action = vi.fn(async () => true), clear = vi.fn(async () => true)
    await act(async () => {
      useChatStore.setState({ activeThreadGoal: { ...goal, status: 'blocked', roundsStarted: 8, blockedReason: 'Actual round limit' },
        activeGoalActivation: { id: goal.id, revision: goal.revision, activation: 'disarmed' }, setActiveThreadGoalStatus: action, clearActiveThreadGoal: clear })
      root.render(floatingElement(false, true))
    })
    await open(); expect(goalMenu().disabled).toBe(true)
    await key(document.activeElement as HTMLElement, 'Escape')
    await act(async () => host.querySelector<HTMLButtonElement>('.ds-composer-goal-summary')!.click())
    const row = host.querySelector<HTMLElement>('.ds-composer-goal-details')!
    expect(row.textContent).toContain('Actual round limit')
    expect(row.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:goalActionResume')}"]`)!.disabled).toBe(true)
    expect(row.querySelector<HTMLInputElement>('input[type="number"]')!.disabled).toBe(false)
    await act(async () => row.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:goalActionClear')}"]`)!.click())
    expect(clear).toHaveBeenCalledOnce(); expect(action).not.toHaveBeenCalled()
  })
})

describe('actual floating question slot lifecycle', () => {
  const requestId = 'input_' + 'f'.repeat(64)
  const selector = { threadId: 'menu-fixture', turnId: 'turn-question', itemId: 'item_' + requestId, requestId }
  const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  function installQuestion() {
    const state: { display: UserInputLocalDisplayV1 } = { display: { schemaVersion: 1, ...selector, callId: 'actual-float-call', status: 'pending', answerState: 'pending',
      questions: [{ id: requestId + '_1', header: 'Synthetic float', question: 'Synthetic float question', multiSelect: false, options: [] }] } }
    const invalidators = new Set<(id: string | null) => void>()
    const close = vi.fn(async (_id: string) => undefined)
    const open = vi.fn(async (_request: UserInputLocalViewRequestV1) => ({ ok: true as const, display: state.display }))
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
      openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close,
      onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidators.add(listener); return () => { invalidators.delete(listener) } },
      onRuntimeStatus: () => () => undefined
    } } })
    const block: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId, status: 'pending', questions: [], meta: { turnId: selector.turnId } }
    return { state, block, close, open }
  }
  it('preserves a canonical Go user-input event through the real floating selector', async () => {
    const fixture = installQuestion()
    const event = { kind: 'user_input_requested', seq: 7, timestamp: '2026-10-13T00:00:00.000Z',
      threadId: selector.threadId, turnId: selector.turnId, itemId: selector.itemId, inputId: selector.requestId,
      status: 'pending', prompt: 'Synthetic float', questions: fixture.state.display.questions }
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true)
    const projected = projectPublicRuntimeSseBlock(
      `id: ${event.seq}\nevent: ${event.kind}\ndata: ${JSON.stringify(event)}`, event.threadId, new PublicRuntimeEventFilter())
    expect(projected).toEqual({ status: 'emit', event, seq: event.seq })
    if (projected?.status !== 'emit') throw new Error('Canonical fixture must pass normal public projection')
    const sink = buildThreadEventSink(useChatStore.setState, useChatStore.getState, { threadId: event.threadId })
    await act(async () => {
      await dispatchAnalytixRuntimeEvent(projected.event as Parameters<typeof dispatchAnalytixRuntimeEvent>[0], sink, async () => undefined)
    })
    expect(fixture.open).toHaveBeenCalledOnce()
    const request = fixture.open.mock.calls[0][0]
    expect(Object.keys(request).sort()).toEqual(['itemId', 'requestId', 'threadId', 'turnId', 'viewId'])
    expect(userInputLocalViewRequestSchemaV1.safeParse(request).success).toBe(true)
    const { viewId, ...actualSelector } = request
    expect(viewId).toEqual(expect.any(String))
    expect(actualSelector).toEqual(selector)
    expect(userInputLocalSelectorSchemaV1.safeParse(actualSelector).success).toBe(true)
  })
  const leaf = () => host.querySelector<HTMLElement>('[data-gate-request-id]')!
  const answer = () => leaf().querySelector<HTMLTextAreaElement>('textarea')!
  async function enter(value: string) {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(answer(), value)
      answer().dispatchEvent(new Event('input', { bubbles: true }))
    })
  }
  it('retains the same request leaf and user draft when Host continues, alongside the ordinary composer', async () => {
    const fixture = installQuestion()
    await act(async () => useChatStore.setState({ blocks: [fixture.block], userInputPanel: undefined }))
    await enter('Synthetic draft across actual continued'); const originalLeaf = leaf()
    expect(trigger()).toBeNull()
    fixture.state.display = { ...fixture.state.display, status: 'continued', answerState: 'unavailable', timedWait: timing }
    await act(async () => useChatStore.setState({ blocks: [{ ...fixture.block, status: 'continued', timedWait: timing }] }))
    expect(leaf()).toBe(originalLeaf)
    expect(answer().value).toBe('Synthetic draft across actual continued')
    expect(trigger()).not.toBeNull()
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('Synthetic draft across actual continued')
  })
  it('hides and reveals the named request through the existing store without cancelling or dropping the leaf draft', async () => {
    const fixture = installQuestion(), resolve = vi.fn(async () => undefined)
    await act(async () => useChatStore.setState({ blocks: [fixture.block], userInputPanel: undefined, resolveUserInput: resolve }))
    await enter('Synthetic same private leaf draft')
    const hide = leaf().querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:userInputHide')}"]`)!
    await act(async () => hide.click())
    expect(useChatStore.getState().userInputPanel).toEqual({ threadId: selector.threadId, requestId: null })
    expect(host.querySelector<HTMLElement>('[data-analytix-gate-composer]')?.hidden).toBe(true)
    expect(trigger()).not.toBeNull(); expect(fixture.close).toHaveBeenCalled(); expect(resolve).not.toHaveBeenCalled()
    await act(async () => useChatStore.getState().selectUserInputPanel(requestId))
    expect(answer().value).toBe('Synthetic same private leaf draft')
    expect(host.querySelector<HTMLElement>('[data-analytix-gate-composer]')?.hidden).toBe(false)
  })
  it('admits the latest edit on a real store thread switch and preserves only an unconfirmed revision warning across new leaves', async () => {
    const fixture = installQuestion()
    fixture.state.display = { ...fixture.state.display, draftRevision: null, draft: null }
    let finish!: (value: { ok: true; receipt: { schemaVersion: 1; threadId: string; turnId: string; itemId: string; requestId: string; status: 'pending'; draftRevision: string; draft: UserInputLocalSaveDraftRequestV1['draft'] } }) => void
    let admissions = 0
    const save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      const receipt = { schemaVersion: 1 as const, ...selector, status: 'pending' as const, draftRevision: request.revision, draft: request.draft }
      if (++admissions > 1) return await new Promise<{ ok: true; receipt: typeof receipt }>(resolve => { finish = resolve })
      fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt }
    })
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { ...window.analytix.runtime, saveUserInputDraft: save } } })
    await act(async () => useChatStore.setState({ blocks: [fixture.block], userInputPanel: undefined }))
    await enter('Synthetic initial saved draft')
    await enter('Synthetic latest before thread detach')
    expect(save).toHaveBeenCalledOnce()
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-other', blocks: [] }))
    expect(save).toHaveBeenCalledTimes(2)
    const latest = save.mock.calls[1][0]
    expect(latest.draft.answers[0].customText).toBe('Synthetic latest before thread detach')
    expect(Object.values(useChatStore.getState().userInputDraftInterruptions ?? {})).toEqual([{ revision: latest.revision, unsent: false }])
    expect(JSON.stringify(useChatStore.getState().userInputDraftInterruptions)).not.toContain('Synthetic')
    expect(host.textContent).not.toContain('Synthetic latest before thread detach')
    await act(async () => useChatStore.setState({ activeThreadId: selector.threadId, blocks: [fixture.block], userInputPanel: undefined }))
    expect(answer().value).toBe('Synthetic initial saved draft')
    expect(leaf().textContent).toContain(i18n.t('common:userInputDraftInterrupted'))
    fixture.state.display = { ...fixture.state.display, draftRevision: latest.revision, draft: latest.draft }
    await act(async () => finish({ ok: true, receipt: { schemaVersion: 1, ...selector, status: 'pending', draftRevision: latest.revision, draft: latest.draft } }))
    const retry = [...leaf().querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent === i18n.t('common:protectedToolRetry'))!
    await act(async () => retry.click())
    expect(answer().value).toBe('Synthetic latest before thread detach')
    expect(leaf().textContent).not.toContain(i18n.t('common:userInputDraftInterrupted'))
    expect(useChatStore.getState().userInputDraftInterruptions).toEqual({})
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('Synthetic latest before thread detach')
  })

  it('does not flush or recreate a discarded warning from a mounted leaf after the confirmed deletion commit', async () => {
    const fixture = installQuestion()
    fixture.state.display = { ...fixture.state.display, draftRevision: null, draft: null }
    const save = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      fixture.state.display = { ...fixture.state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...selector, status: 'pending' as const, draftRevision: request.revision, draft: request.draft } }
    })
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: { ...window.analytix.runtime, saveUserInputDraft: save } } })
    await act(async () => useChatStore.setState({ blocks: [fixture.block], userInputPanel: undefined }))
    await enter('Synthetic first admitted draft'); await enter('Synthetic discarded by confirmed deletion')
    expect(save).toHaveBeenCalledOnce()
    await act(async () => useChatStore.setState({ activeThreadId: null, blocks: [], userInputDraftInterruptions: {}, deletedUserInputDraftThreadIds: { [selector.threadId]: true } }))
    expect(save).toHaveBeenCalledOnce(); expect(useChatStore.getState().userInputDraftInterruptions).toEqual({})
    expect(host.textContent).not.toContain('Synthetic discarded by confirmed deletion')
  })

  it('unmounts the former thread private leaf instead of carrying its raw draft into the newly selected thread', async () => {
    const fixture = installQuestion()
    await act(async () => useChatStore.setState({ blocks: [fixture.block], userInputPanel: undefined }))
    await enter('Synthetic old thread private draft')
    await act(async () => useChatStore.setState({ activeThreadId: 'thread-other', blocks: [] }))
    expect(host.querySelector('[data-gate-request-id]')).toBeNull()
    expect(host.textContent).not.toContain('Synthetic old thread private draft'); expect(fixture.close).toHaveBeenCalled()
    await act(async () => useChatStore.setState({ activeThreadId: selector.threadId, blocks: [fixture.block], userInputPanel: undefined }))
    expect(answer().value).toBe('')
  })
})


describe('actual Tree and floating late-answer topology', () => {
  const requestId = 'input_' + 'e'.repeat(64), callId = 'actual-topology-call'
  const selector = { threadId: 'menu-fixture', turnId: 'turn-question-source', itemId: 'item_' + requestId, requestId }
  const targetTurnId = 'turn-late-target-A', submittedText = 'Synthetic topology late answer'
  const hostDraftText = 'Synthetic newer Host private draft'
  const timing = { timeoutSeconds: 120, deadlineAt: '2026-10-09T00:00:00.000Z' }
  type Resolve = ReturnType<typeof useChatStore.getState>['resolveUserInput']
  type LateAction = Extract<Parameters<Resolve>[1], { kind: 'late' }>
  type SseListener = Parameters<typeof window.analytix.runtime.onSseEvent>[0]
  function deferred() { let finish!: () => void; const promise = new Promise<void>(resolve => { finish = resolve }); return { promise, finish } }
  function Topology({ copies = 1 }: { copies?: number }) {
    const blocks = useChatStore(state => state.blocks), threadId = useChatStore(state => state.activeThreadId), busy = useChatStore(state => state.busy)
    return createElement('div', null,
      ...Array.from({ length: copies }, (_, index) => createElement('div', { key: index, 'data-topology-timeline': index },
        createElement(ProtectedToolResultScope.Provider, { value: threadId },
          createElement(ToolCallTree, { blocks, processing: false, policy: presentationPolicyFor('detailed') })))),
      floatingElement(busy))
  }
  function installTopology(privateCallId: string | null = callId) {
    const state: { display: UserInputLocalDisplayV1 } = { display: { schemaVersion: 1, ...selector, callId: privateCallId,
      status: 'continued', timedWait: timing, answerState: 'unavailable', draft: null, draftRevision: null,
      questions: [{ id: requestId + '_1', header: 'Synthetic topology header', question: 'Synthetic topology question', multiSelect: false, options: [] }] } }
    const events = new Set<SseListener>(), invalidators = new Set<(id: string | null) => void>(), attempts: LateAction[] = []
    const response = deferred(), discarded = deferred()
    const emitSteered = () => [...events].forEach(listener => listener({ streamId: 'synthetic-topology-stream', events: [{ kind: 'turn_steered', threadId: selector.threadId }] }))
    const openDisplay = vi.fn(async (_request: Parameters<typeof window.analytix.runtime.openUserInputLocalDisplay>[0]) => ({ ok: true as const, display: state.display })), close = vi.fn(async (_id: string): Promise<void> => undefined)
    const discard = vi.fn(() => discarded.promise)
    const saveDraft = vi.fn(async (request: UserInputLocalSaveDraftRequestV1) => {
      state.display = { ...state.display, draftRevision: request.revision, draft: request.draft }
      return { ok: true as const, receipt: { schemaVersion: 1 as const, ...selector, status: 'continued' as const, draftRevision: request.revision, draft: request.draft } }
    })
    Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
      openUserInputLocalDisplay: openDisplay, closeUserInputLocalDisplay: close, saveUserInputDraft: saveDraft, discardUserInputLateReply: discard,
      onUserInputLocalDisplayInvalidated: (listener: (id: string | null) => void) => { invalidators.add(listener); return () => { invalidators.delete(listener) } },
      onSseEvent: (listener: SseListener) => { events.add(listener); return () => { events.delete(listener) } }, onRuntimeStatus: () => () => undefined
    } } })
    const resolve = vi.fn<Resolve>(async (_target, action) => {
      if (action.kind !== 'late') throw new Error('Synthetic topology expects the real late action')
      const answer = action.answers[0]
      if (!answer || !('kind' in answer) || answer.kind !== 'answer') throw new Error('Synthetic topology expects the real structured answer')
      attempts.push(action)
      state.display = { ...state.display, answerState: 'available', draft: null,
        submission: { schemaVersion: 1, format: 'structured', answers: [{ id: answer.id, kind: 'answer', optionIds: answer.optionIds, customText: answer.customText }] },
        lateReply: { clientUserMessageId: action.clientUserMessageId, turnId: action.expectedTurnId, state: 'queued' } }
      // Core publishes turn_steered before the queue RPC returns. Only the
      // private Host readback carries these original answer bytes.
      emitSteered()
      await response.promise
    })
    const selectPanel = vi.fn(useChatStore.getState().selectUserInputPanel)
    const tool: ToolBlock = { kind: 'tool', id: 'actual-topology-tool', toolKind: 'tool_call', status: 'success', summary: 'Projected tool summary', meta: { callId, turnId: selector.turnId, toolName: 'ask_user_question' } }
    const input: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId, status: 'continued', timedWait: timing,
      questions: [{ id: requestId + '_1', header: 'Projected header', question: 'Projected question', multiSelect: false, options: [] }], meta: { turnId: selector.turnId } }
    return { state, events, invalidators, attempts, response, discarded, openDisplay, close, discard, resolve, selectPanel, blocks: [tool, input] as ChatBlock[] }
  }
  const slot = () => host.querySelector<HTMLElement>('[data-analytix-gate-composer]')!
  function visibleButton(key: string, scope: Element = host): HTMLButtonElement {
    const matches = [...scope.querySelectorAll<HTMLButtonElement>('button')].filter(button => button.textContent === i18n.t('common:' + key) && !button.closest('[hidden],[inert]'))
    expect(matches).toHaveLength(1); return matches[0]
  }
  async function show(fixture: ReturnType<typeof installTopology>, copies = 1) {
    await act(async () => {
      useChatStore.setState({ blocks: fixture.blocks, busy: true, currentTurnId: targetTurnId, userInputPanel: undefined,
        resolveUserInput: fixture.resolve, selectUserInputPanel: fixture.selectPanel })
      useChatStore.getState().selectUserInputPanel(requestId)
      root.render(createElement(Topology, { copies }))
    })
    // Detailed history groups start folded. Exercise the normal title action
    // before asserting that a real inline owner is visible for handoff.
    await act(async () => {
      for (const title of host.querySelectorAll<HTMLButtonElement>('[data-topology-timeline] button[data-process-activity][aria-expanded="false"]')) title.click()
    })
    expect(slot().hidden).toBe(false); expect(slot().hasAttribute('inert')).toBe(false)
    expect(slot().querySelector('textarea')).not.toBeNull(); expect(trigger()).not.toBeNull()
    fixture.selectPanel.mockClear()
  }
  it('uses the actual idle MessageBubble route, fresh Host anchor and existing recovery without inventing a running turn', async () => {
    const fixture = installTopology(), provider = getProvider()
    const detail = vi.spyOn(provider, 'getThreadDetail').mockResolvedValue({ blocks: fixture.blocks, latestSeq: 10, threadStatus: 'completed', latestTurnId: 'turn-host-terminal-anchor' })
    const recoveryPending = deferred(), recover = vi.fn(async () => { await recoveryPending.promise; return false })
    const submit = vi.spyOn(provider, 'submitUserInputLateReply').mockImplementation(async (_inputId, answers, attempt) => {
      const answer = answers[0]
      if (!answer || !('kind' in answer) || answer.kind !== 'answer') throw new Error('Synthetic structured answer required')
      const delivery = { clientUserMessageId: attempt.clientUserMessageId, turnId: 'turn-host-idle-wake', state: 'queued' as const }
      fixture.state.display = { ...fixture.state.display, answerState: 'available', draft: null,
        submission: { schemaVersion: 1, format: 'structured', answers: [answer] }, lateReply: delivery }
      return delivery
    })
    await show(fixture)
    await act(async () => useChatStore.setState({ busy: false, currentTurnId: null, resolveUserInput: previous.resolveUserInput, recoverActiveTurn: recover }))
    const textarea = slot().querySelector<HTMLTextAreaElement>('textarea')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(textarea, submittedText)
      textarea.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => visibleButton('userInputLateSubmit', slot()).click())
    expect(detail).toHaveBeenCalledExactlyOnceWith(selector.threadId)
    expect(submit).toHaveBeenCalledWith(requestId, [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: submittedText }],
      { clientUserMessageId: expect.any(String), expectedTurnId: 'turn-host-terminal-anchor' }, selector)
    expect(recover).toHaveBeenCalledOnce()
    expect(useChatStore.getState().busy).toBe(false); expect(useChatStore.getState().currentTurnId).toBeNull()
    expect(useChatStore.getState().blocks.some(block => block.kind === 'user')).toBe(false)
    expect(slot().textContent).toContain(i18n.t('common:userInputLateQueued'))
    expect(fixture.openDisplay.mock.calls.every(([request]) => request.turnId === selector.turnId)).toBe(true)
    await act(async () => recoveryPending.finish())
  })
  async function queue(fixture: ReturnType<typeof installTopology>) {
    const answer = slot().querySelector<HTMLTextAreaElement>('textarea')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(answer, submittedText)
      answer.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const submit = visibleButton('userInputLateSubmit', slot()), readsBeforeSubmit = fixture.openDisplay.mock.calls.length
    expect(submit.disabled).toBe(false)
    await act(async () => submit.click())
    expect(fixture.resolve).toHaveBeenCalledWith({ itemId: selector.itemId, requestId, threadId: selector.threadId, turnId: selector.turnId },
      { kind: 'late', answers: [{ id: requestId + '_1', kind: 'answer', optionIds: [], customText: submittedText }], clientUserMessageId: expect.any(String), expectedTurnId: targetTurnId })
    expect(slot().hidden).toBe(false)
    expect(fixture.events.size).toBeGreaterThan(0)
    expect(fixture.openDisplay.mock.calls.length).toBeGreaterThan(readsBeforeSubmit)
    expect(slot().textContent).toContain(i18n.t('common:userInputLateQueued'))
    const inlineRows = [...host.querySelectorAll<HTMLElement>('[data-question-inline-owned="true"]')]
    if (fixture.state.display.callId) expect(inlineRows.length).toBeGreaterThan(0)
    else expect(inlineRows).toHaveLength(0)
    for (const row of inlineRows) expect(row.dataset.questionLateState).toBe('queued')
    await act(async () => fixture.response.finish())
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain(submittedText)
    expect(host.querySelector('[data-question-reply-item-id]')).toBeNull()
  }
  it('refreshes the parked queued row from completed user_message item_created without reopening its former floating owner', async () => {
    const fixture = installTopology(); await show(fixture)
    const retainedSlot = slot(); await queue(fixture)
    expect(slot()).toBe(retainedSlot); expect(retainedSlot.hidden).toBe(true); expect(retainedSlot.hasAttribute('inert')).toBe(true)
    const queuedRow = host.querySelector<HTMLElement>('[data-question-inline-owned="true"]')!
    expect(queuedRow.dataset.questionLateState).toBe('queued')
    expect(visibleButton('userInputLateDiscard', queuedRow)).toBeDefined()
    const attempt = fixture.attempts[0], actualTurnId = 'turn-host-promoted-topology'
    const reads = fixture.openDisplay.mock.calls.length
    fixture.state.display = { ...fixture.state.display, lateReply: { clientUserMessageId: attempt.clientUserMessageId, turnId: actualTurnId, state: 'accepted' } }
    expect(fixture.openDisplay).toHaveBeenCalledTimes(reads)
    expect(queuedRow.dataset.questionLateState).toBe('queued')
    const timestamp = '2026-10-09T00:00:00Z', itemId = 'host-promoted-topology-user'
    const event: Parameters<SseListener>[0]['events'][number] = { kind: 'item_created', seq: 17, timestamp,
      threadId: selector.threadId, turnId: actualTurnId, itemId,
      item: { id: itemId, threadId: selector.threadId, turnId: actualTurnId, role: 'user', status: 'completed', kind: 'user_message',
        text: 'Safe public promoted topology answer', clientUserMessageId: attempt.clientUserMessageId, delivery: 'steer', createdAt: timestamp } }
    // Synthetic bridge transport, actual closed SSE validation and mounted topology.
    // This does not stand in for a Main/Go native event witness.
    expect(isClosedPublicRuntimeSseEvent(event)).toBe(true); expect(fixture.events.size).toBeGreaterThan(0)
    await act(async () => {
      for (const listener of [...fixture.events]) listener({ streamId: 'topology-promotion', events: [event] })
      for (let index = 0; index < 20; index++) await Promise.resolve()
    })
    expect(fixture.openDisplay.mock.calls.length).toBeGreaterThan(reads)
    expect(fixture.openDisplay.mock.calls.every(([request]) => request.turnId === selector.turnId)).toBe(true)
    const acceptedRow = host.querySelector<HTMLElement>('[data-question-inline-owned="true"]')!
    expect(acceptedRow).not.toBeNull(); expect(acceptedRow.dataset.questionLateState).toBe('accepted')
    expect(acceptedRow.dataset.questionCallId).toBe(callId); expect(acceptedRow.dataset.questionRequestId).toBe(requestId)
    expect(acceptedRow.dataset.questionThreadId).toBe(selector.threadId)
    expect([...acceptedRow.querySelectorAll('button')].some(button => button.textContent === i18n.t('common:userInputLateDiscard'))).toBe(false)
    expect(visibleButton('userInputReview', acceptedRow)).toBeDefined()
    expect(fixture.state.display.lateReply?.clientUserMessageId).toBe(attempt.clientUserMessageId)
    expect(actualTurnId).not.toBe(selector.turnId); expect(actualTurnId).not.toBe(attempt.expectedTurnId)
    expect(fixture.resolve).toHaveBeenCalledOnce(); expect(fixture.discard).not.toHaveBeenCalled()
    expect(slot()).toBe(retainedSlot); expect(retainedSlot.hidden).toBe(true); expect(retainedSlot.hasAttribute('inert')).toBe(true)
    expect(useChatStore.getState().userInputPanel).toEqual({ threadId: selector.threadId, requestId: null })
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain(submittedText)
    await act(async () => root.render(null))
    expect(fixture.events.size).toBe(0)
  })
  it('parks only after queued SSE readback, then reviews, discards and Responds through the same real owner', async () => {
    const fixture = installTopology(); await show(fixture)
    const retainedSlot = slot(); await queue(fixture)
    expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(true); expect(slot().hasAttribute('inert')).toBe(true)
    expect(useChatStore.getState().userInputPanel).toEqual({ threadId: selector.threadId, requestId: null })
    expect(trigger()).not.toBeNull(); expect(fixture.close).toHaveBeenCalled()
    const row = host.querySelector<HTMLElement>('[data-question-inline-owned="true"]')!
    expect(row.dataset.questionCallId).toBe(callId); expect(row.dataset.questionRequestId).toBe(requestId)
    expect(row.dataset.questionThreadId).toBe(selector.threadId); expect(row.dataset.questionLateState).toBe('queued')
    fixture.selectPanel.mockClear()
    await act(async () => visibleButton('userInputReview', row).click())
    expect(row.querySelector('dl')?.textContent).toContain(submittedText); expect(row.querySelector('textarea')).toBeNull()
    expect(fixture.selectPanel).not.toHaveBeenCalled(); expect(slot().hidden).toBe(true)
    await act(async () => useChatStore.setState({ currentTurnId: 'turn-late-target-B' }))
    const discard = visibleButton('userInputLateDiscard', row)
    await act(async () => { discard.click(); discard.click() })
    expect(fixture.discard).toHaveBeenCalledOnce()
    const first = fixture.attempts[0]
    expect(fixture.discard).toHaveBeenCalledWith({ ...selector, action: 'discardLateReply', clientUserMessageId: first.clientUserMessageId, expectedTurnId: targetTurnId })
    fixture.state.display = { ...fixture.state.display, lateReply: { ...fixture.state.display.lateReply!, state: 'discarded' },
      draftRevision: '59799e24-d3f8-4c81-8fc8-64e61c8c31b8', draft: { currentPage: 0, answers: [{ questionId: requestId + '_1', optionIds: [], customText: hostDraftText, skipped: false }] } }
    await act(async () => fixture.discarded.finish())
    const readsBeforeRespond = fixture.openDisplay.mock.calls.length
    const respondingRow = host.querySelector<HTMLElement>('[data-question-inline-owned="true"]')!
    await act(async () => visibleButton('dshRespond', respondingRow).click())
    expect(fixture.selectPanel).toHaveBeenCalledOnce(); expect(fixture.selectPanel).toHaveBeenCalledWith(requestId)
    expect(fixture.openDisplay.mock.calls.length).toBeGreaterThan(readsBeforeRespond)
    expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(false); expect(slot().hasAttribute('inert')).toBe(false)
    expect(slot().querySelector<HTMLTextAreaElement>('textarea')?.value).toBe(hostDraftText)
    expect(visibleButton('userInputLateSubmit', slot()).disabled).toBe(false)
    expect(fixture.resolve).toHaveBeenCalledOnce(); expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain(hostDraftText)
  })
  it.each(['nullable callId', 'ambiguous visible rows'] as const)('retains the full queued card when %s cannot provide a unique owned inline entry', async reason => {
    const fixture = installTopology(reason === 'nullable callId' ? null : callId)
    await show(fixture, reason === 'ambiguous visible rows' ? 2 : 1); const retainedSlot = slot()
    await queue(fixture)
    expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(false); expect(slot().hasAttribute('inert')).toBe(false)
    expect(fixture.selectPanel).not.toHaveBeenCalled()
    expect(host.querySelectorAll('[data-question-inline-owned="true"]')).toHaveLength(reason === 'nullable callId' ? 0 : 2)
    expect(visibleButton('userInputLateDiscard', slot())).toBeDefined(); expect(trigger()).not.toBeNull()
  })
  it('rereads a still-presented request from metadata Respond after privacy revoke without changing its selection', async () => {
    const fixture = installTopology(); await show(fixture)
    const retainedSlot = slot(), selected = useChatStore.getState().userInputPanel
    expect(selected).toEqual({ threadId: selector.threadId, requestId })
    const readsBeforeRevoke = fixture.openDisplay.mock.calls.length
    await act(async () => [...fixture.invalidators].forEach(listener => listener(null)))
    expect(fixture.openDisplay).toHaveBeenCalledTimes(readsBeforeRevoke)
    expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(false); expect(slot().hasAttribute('inert')).toBe(false)
    expect(useChatStore.getState().userInputPanel).toEqual(selected)
    expect(slot().querySelector('textarea')).toBeNull()
    expect(slot().textContent).not.toContain('Synthetic topology question')
    fixture.selectPanel.mockClear()
    await act(async () => visibleButton('dshRespond').click())
    expect(fixture.selectPanel).toHaveBeenCalledOnce(); expect(fixture.selectPanel).toHaveBeenCalledWith(requestId)
    expect(useChatStore.getState().userInputPanel).toEqual(selected)
    expect(fixture.openDisplay.mock.calls.length).toBeGreaterThan(readsBeforeRevoke)
    expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(false)
    expect(slot().querySelector('h2')?.textContent).toBe('Synthetic topology question')
    expect(slot().querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('')
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('Synthetic topology question')
  })
  it('retires an opened fallback disclosure when association finishes and does not remount raw review after revoke', async () => {
    const fixture = installTopology(), associationClose = deferred()
    fixture.close.mockImplementationOnce(() => associationClose.promise)
    try {
      await show(fixture)
      expect(fixture.close).toHaveBeenCalledOnce()
      const closedId = fixture.close.mock.calls[0][0]
      expect(fixture.openDisplay.mock.calls.some(([request]) => request.viewId === closedId)).toBe(true)
      const retainedSlot = slot(), timeline = host.querySelector<HTMLElement>('[data-topology-timeline="0"]')!
      expect(timeline.querySelector('[data-question-inline-owned="true"]')).toBeNull()
      const header = visibleButton('dshRespond', timeline).closest<HTMLElement>('[data-disclosure-row]')!
      expect(header.getAttribute('aria-expanded')).toBe('false')
      await act(async () => header.click())
      expect(header.getAttribute('aria-expanded')).toBe('true')
      expect(timeline.querySelector('section[data-gate-request-id][data-protected-user-input] h2')?.textContent).toBe('Synthetic topology question')
      await act(async () => associationClose.finish())
      expect(timeline.querySelector('[data-question-inline-owned="true"]')).not.toBeNull()
      expect(timeline.querySelector('section[data-gate-request-id]')).toBeNull()
      expect(slot()).toBe(retainedSlot); expect(slot().hidden).toBe(false)
      const readsBeforeRevoke = fixture.openDisplay.mock.calls.length
      await act(async () => [...fixture.invalidators].forEach(listener => listener(null)))
      expect(fixture.openDisplay).toHaveBeenCalledTimes(readsBeforeRevoke)
      expect(timeline.querySelector('[data-question-inline-owned="true"]')).toBeNull()
      expect(timeline.querySelector('section[data-gate-request-id]')).toBeNull()
      expect(visibleButton('dshRespond', timeline).closest('[data-disclosure-row]')?.getAttribute('aria-expanded')).toBe('false')
    } finally { await act(async () => associationClose.finish()) }
  })
  it('keeps a metadata Respond entry after parked privacy revocation and rereads Host only on that user action', async () => {
    const fixture = installTopology(); await show(fixture); await queue(fixture)
    expect(slot().hidden).toBe(true)
    const readsBeforeRevoke = fixture.openDisplay.mock.calls.length
    await act(async () => [...fixture.invalidators].forEach(listener => listener(null)))
    expect(fixture.openDisplay).toHaveBeenCalledTimes(readsBeforeRevoke)
    expect(host.querySelector('[data-question-inline-owned="true"]')).toBeNull()
    expect(slot().hidden).toBe(true); expect(host.textContent).not.toContain(submittedText)
    const readsBeforeRespond = fixture.openDisplay.mock.calls.length
    fixture.selectPanel.mockClear()
    await act(async () => visibleButton('dshRespond').click())
    expect(fixture.selectPanel.mock.calls).toEqual([[requestId], [null]])
    expect(fixture.openDisplay.mock.calls.length).toBeGreaterThan(readsBeforeRespond)
    // Explicit reread restores the real queued inline owner. Only after that
    // verified unique entry returns may the full card park again.
    const recoveredRow = host.querySelector<HTMLElement>('[data-question-inline-owned="true"]')!
    expect(recoveredRow).not.toBeNull(); expect(recoveredRow.closest('[hidden],[inert]')).toBeNull()
    expect(recoveredRow.dataset.questionLateState).toBe('queued')
    expect(slot().hidden).toBe(true); expect(slot().hasAttribute('inert')).toBe(true)
    expect(visibleButton('userInputReview', recoveredRow)).toBeDefined()
    expect(visibleButton('userInputLateDiscard', recoveredRow)).toBeDefined()
    expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain(submittedText)
  })
})

it('opens a selected settled full Review readonly, paginates, closes and reopens through the real store', async () => {
  const requestId = 'input_' + 'f'.repeat(64), threadId = 'thread-settled-review', turnId = 'turn-question'
  const selector = { threadId, turnId, itemId: 'item_' + requestId, requestId }
  const display: UserInputLocalDisplayV1 = { schemaVersion: 1, ...selector, callId: 'settled-call', status: 'submitted', answerState: 'available',
    questions: [1, 2].map(index => ({ id: requestId + '_' + index, header: 'Synthetic header', question: 'Synthetic review question ' + index, multiSelect: false, options: [] })),
    submission: { schemaVersion: 1, format: 'structured', answers: [{ id: requestId + '_1', kind: 'skip' }, { id: requestId + '_2', kind: 'answer', optionIds: [], customText: 'Synthetic readonly saved answer' }] } }
  const open = vi.fn(async () => ({ ok: true, display })), close = vi.fn(async () => undefined), resolve = vi.fn()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close,
    onUserInputLocalDisplayInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
  } } })
  const block: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId,
    status: 'submitted', questions: [], meta: { turnId } }
  await act(async () => useChatStore.setState({ activeThreadId: threadId, blocks: [block], userInputPanel: undefined, resolveUserInput: resolve }))
  expect(host.querySelector('[data-gate-request-id]')).toBeNull()
  await act(async () => useChatStore.getState().selectUserInputPanel(requestId, 'review'))
  const card = () => host.querySelector<HTMLElement>('[data-gate-request-id]')!
  expect(card().querySelector('h2')?.textContent).toBe('Synthetic review question 1')
  expect(card().querySelector<HTMLFieldSetElement>('fieldset')?.disabled).toBe(true)
  const next = card().querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:userInputNext') + '"]')!
  await act(async () => next.click())
  expect(card().querySelector('h2')?.textContent).toBe('Synthetic review question 2')
  expect(card().querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Synthetic readonly saved answer')
  const hide = card().querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:userInputHide') + '"]')!
  await act(async () => hide.click())
  expect(host.querySelector('[data-gate-request-id]')).toBeNull()
  await act(async () => useChatStore.getState().selectUserInputPanel(requestId, 'review'))
  expect(card()).not.toBeNull(); expect(resolve).not.toHaveBeenCalled()
  expect(close).toHaveBeenCalled()
  expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('Synthetic readonly saved answer')
})


it('keeps measured context capacity while missing whole-log accounting supplies no legacy Footer substitute', async () => {
  const usage: ThreadUsageSnapshot = { inputTokens: 100, outputTokens: 900, totalTokens: 1000,
    reasoningTokens: 0, cachedTokens: 50, cacheMissTokens: 50, cacheHitRate: 0.5,
    costUsd: null, costCny: null, priceConfigured: false, tokenEconomySavingsTokens: 0, turns: 1 }
  await act(async () => {
    useChatStore.setState({ lastTurnUsage: { threadId: 'menu-fixture', snapshot: usage }, busy: false })
    root.render(createElement(InputBar, { ...floatingElement().props, contextWindowTokens: 1000, runtimeToolCount: 0, runtimeSkillCount: 0 }))
  })
  const capacity = host.querySelector<HTMLButtonElement>('button.ds-composer-context')!
  expect(capacity).not.toBeNull(); expect(capacity.getAttribute('aria-label')).toContain('10%')
  expect(capacity.textContent).toBe('10%')
  const ring = capacity.querySelector('svg')!
  expect(ring.getAttribute('width')).toBe('14')
  expect(ring.getAttribute('height')).toBe('14')
  expect(ring.getAttribute('viewBox')).toBe('0 0 14 14')
  const circles = ring.querySelectorAll('circle')
  expect(circles).toHaveLength(2)
  for (const circle of circles) {
    expect(circle.getAttribute('r')).toBe('5.5')
    expect(circle.getAttribute('stroke-width')).toBe('2')
  }
  expect(circles[0].getAttribute('stroke')).toBe('var(--dsw-alias-border-l3)')
  expect(circles[1].getAttribute('stroke')).toBe('var(--dsw-alias-label-tertiary)')
  const dock=host.querySelector('[data-composer-stats-dock]')!
  expect(dock.contains(capacity)).toBe(true)
  expect(capacity.closest('.ds-composer-toolbar')).toBeNull()
  expect(dock.querySelector('.ds-composer-footer-hint')).toBeNull()
  expect(host.querySelector('.ds-composer-footer-hint')).toBeNull()
  expect(host.querySelector('.ds-composer-footer')).toBeNull()
  expect(capacity.getAttribute('aria-label')).toBe(i18n.t('common:contextCapacityChipAria', { percent: '10%' }))
  await act(async () => capacity.click())
  expect(capacity.getAttribute('aria-expanded')).toBe('true')
  expect(host.querySelector('.ds-composer-footer-left')).toBeNull()
  expect(host.querySelector('[data-composer-stat]')).toBeNull()
  const panel = document.querySelector<HTMLElement>('[role="dialog"][aria-label="' + i18n.t('common:contextCapacityTitle') + '"]')!
  expect(panel.parentElement).toBe(document.body); expect(host.contains(panel)).toBe(false)
  expect(capacity.getAttribute('aria-controls')).toBe(panel.id)
  await act(async () => useChatStore.setState({ activeThreadId: 'other-owner' }))
  expect(document.body.contains(panel)).toBe(false)
  expect(host.querySelector('[data-composer-stat]')).toBeNull()
  await act(async () => useChatStore.setState({ activeThreadId: 'menu-fixture' }))
  expect(document.querySelector('[role="dialog"][aria-label="' + i18n.t('common:contextCapacityTitle') + '"]')).toBeNull()
})


it.each(['cancelled', 'continued'] as const)('reopens Host readOnly %s history with actual full Composer selection and retained semantic pages', async status => {
  const requestId = 'input_' + 'f'.repeat(64), threadId = 'child-history-review', turnId = 'child-seed-turn'
  const selector = { threadId, turnId, itemId: 'item_' + requestId, requestId }
  const display: UserInputLocalDisplayV1 = { schemaVersion: 1, ...selector, readOnly: true, callId: 'original-signed-call', status,
    answerState: status === 'cancelled' ? 'cancelled' : 'unavailable', timedWait: null, continuedReason: null, draftRevision: null, draft: null, lateReply: null,
    questions: [1, 2].map(index => ({ id: requestId + '_' + index, header: 'Synthetic inherited header',
      question: 'Synthetic inherited question ' + index, multiSelect: index === 1,
      options: [{ id: requestId + '_' + index + '_option_1', label: 'Synthetic inherited choice', description: '' }] })) }
  const open = vi.fn(async () => ({ ok: true, display })), close = vi.fn(async () => undefined), resolve = vi.fn(), save = vi.fn(), discard = vi.fn()
  Object.defineProperty(window, 'analytix', { configurable: true, value: { runtime: {
    openUserInputLocalDisplay: open, closeUserInputLocalDisplay: close, saveUserInputDraft: save, discardUserInputLateReply: discard,
    onUserInputLocalDisplayInvalidated: () => () => undefined, onRuntimeStatus: () => () => undefined
  } } })
  const block: Extract<ChatBlock, { kind: 'user_input' }> = { kind: 'user_input', id: selector.itemId, requestId,
    status, questions: [], meta: { turnId } }
  await act(async () => useChatStore.setState({ activeThreadId: threadId, blocks: [block], userInputPanel: undefined, resolveUserInput: resolve }))
  await act(async () => useChatStore.getState().selectUserInputPanel(requestId, 'review'))
  const card = () => host.querySelector<HTMLElement>('section[data-gate-request-id]')!
  expect(card().querySelector('h2')?.textContent).toBe(display.questions[0].question)
  expect(card().querySelector('[role="group"]')).not.toBeNull()
  expect(card().querySelector<HTMLInputElement>('input')?.value).toBe(display.questions[0].options[0].id)
  expect(card().querySelector<HTMLFieldSetElement>('fieldset')?.disabled).toBe(true)
  expect(card().querySelector('textarea')).toBeNull()
  await act(async () => card().querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:userInputNext') + '"]')!.click())
  expect(card().querySelector('h2')?.textContent).toBe(display.questions[1].question)
  await act(async () => card().querySelector<HTMLButtonElement>('button[aria-label="' + i18n.t('common:userInputHide') + '"]')!.click())
  const hiddenCard = host.querySelector<HTMLElement>('section[data-gate-request-id]')
  if (status === 'continued') {
    expect(hiddenCard?.closest('[hidden][inert]')).not.toBeNull()
    expect(hiddenCard?.textContent).not.toContain('Synthetic inherited')
  } else expect(hiddenCard).toBeNull()
  await act(async () => useChatStore.getState().selectUserInputPanel(requestId, 'review'))
  expect(card()).not.toBeNull(); expect(card().closest('[hidden],[inert]')).toBeNull()
  for (const command of [resolve, save, discard]) expect(command).not.toHaveBeenCalled()
  expect(close).toHaveBeenCalled()
  expect(JSON.stringify(useChatStore.getState().blocks)).not.toContain('Synthetic inherited')
})


describe('real InputBar anchored capacity panel', () => {
  const usage: ThreadUsageSnapshot = { inputTokens: 100, outputTokens: 900, totalTokens: 1000,
    reasoningTokens: 0, cachedTokens: 50, cacheMissTokens: 50, cacheHitRate: 0.5,
    costUsd: null, costCny: null, priceConfigured: false, tokenEconomySavingsTokens: 0, turns: 1 }
  const panel = () => document.querySelector<HTMLElement>('[role="dialog"][aria-label="' + i18n.t('common:contextCapacityTitle') + '"]')
  const ring = () => host.querySelector<HTMLButtonElement>('button.ds-composer-context')!
  const capacityProps = { contextWindowTokens: 1000, runtimeToolCount: 0, runtimeSkillCount: 0 }

  it('uses the portal clamp, preserves focus and dismisses through Escape and outside capture click', async () => {
    vi.stubGlobal('innerWidth', 400); vi.stubGlobal('innerHeight', 600)
    vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(280)
    vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(160)
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(new DOMRect(280, 340, 120, 28))
    const commits: Array<{ owner: string | null; open: boolean }> = []
    await act(async () => {
      useChatStore.setState({ lastTurnUsage: { threadId: 'menu-fixture', snapshot: usage }, busy: false })
      root.render(createElement(Profiler, { id: 'real-capacity', onRender: () => {
        commits.push({ owner: useChatStore.getState().activeThreadId, open: panel() !== null })
      } }, floatingElement(false, false, capacityProps)))
    })
    await act(async () => { ring().focus(); ring().click() })
    expect(panel()!.parentElement).toBe(document.body)
    expect(panel()!.style.left).toBe('108px'); expect(panel()!.style.top).toBe('172px')
    expect(document.activeElement).toBe(ring())
    expect((await key(ring(), 'Escape')).defaultPrevented).toBe(true)
    expect(panel()).toBeNull(); expect(document.activeElement).toBe(ring())
    await act(async () => ring().click())
    const outside = document.createElement('button'); document.body.append(outside)
    outside.addEventListener('click', event => event.stopPropagation())
    await act(async () => outside.click())
    expect(panel()).toBeNull(); outside.remove()
    await act(async () => ring().click())
    commits.length = 0
    await act(async () => useChatStore.setState({ activeThreadId: 'other-owner' }))
    expect(commits.some(commit => commit.owner === 'other-owner')).toBe(true)
    expect(commits.filter(commit => commit.owner === 'other-owner').every(commit => !commit.open)).toBe(true)
    await act(async () => useChatStore.setState({ activeThreadId: 'menu-fixture' }))
    expect(panel()).toBeNull()
  })

  it('composer closure hides unknown capacity immediately and never revives its old panel', async () => {
    await act(async () => root.render(floatingElement(false, false, capacityProps)))
    await act(async () => ring().click())
    expect(panel()).not.toBeNull()
    await act(async () => root.render(floatingElement(false, false, { ...capacityProps, contextWindowTokens: undefined })))
    expect(ring()).toBeNull(); expect(panel()).toBeNull()
    await act(async () => root.render(floatingElement(false, false, capacityProps)))
    expect(ring()).not.toBeNull(); expect(panel()).toBeNull()
    await act(async () => ring().click())
    await act(async () => root.render(floatingElement(false, false, { ...capacityProps, composerModel: 'another-model' })))
    expect(panel()).toBeNull()
  })
  it('composer closure drops the previous busy owner measured numerator', async () => {
    await act(async () => {
      useChatStore.setState({ lastTurnUsage: { threadId: 'menu-fixture', snapshot: { ...usage, inputTokens: 5000, totalTokens: 5900 } } })
      root.render(floatingElement(false, false, { ...capacityProps, contextWindowTokens: 10000 }))
    })
    expect(ring().textContent).toBe('50%')
    await act(async () => {
      useChatStore.setState({ activeThreadId: 'another-busy-thread', blocks: [] })
      root.render(floatingElement(true, false, { ...capacityProps, contextWindowTokens: 10000 }))
    })
    expect(ring().textContent).not.toBe('50%')
  })
  it('retires the panel when recording replaces the real trigger and does not reopen when it returns', async () => {
    // Replace only the voice I/O leaf after retiring its real hook instance.
    // InputBar, its owner subscription and anchored panel remain real.
    await act(async () => root.render(null))
    const voice: ReturnType<typeof voiceDictation.useVoiceDictation> = { status: 'idle', error: null,
      startedAtMs: Date.now(), start: vi.fn(), stop: vi.fn(), toggle: vi.fn(), getLevel: () => 0 }
    vi.spyOn(voiceDictation, 'useVoiceDictation').mockImplementation(() => ({ ...voice }))
    await act(async () => root.render(floatingElement(false, false, capacityProps)))
    await act(async () => ring().click())
    expect(panel()).not.toBeNull()
    voice.status = 'recording'
    await act(async () => root.render(floatingElement(false, false, capacityProps)))
    expect(ring()).toBeNull(); expect(panel()).toBeNull()
    voice.status = 'idle'
    await act(async () => root.render(floatingElement(false, false, capacityProps)))
    expect(ring()).not.toBeNull(); expect(panel()).toBeNull()
  })

  it('does not estimate streaming blocks while closed and resumes estimation only while open', async () => {
    await act(async () => {
      useChatStore.setState({ lastTurnUsage: { threadId: 'menu-fixture', snapshot: usage }, busy: false })
      root.render(floatingElement(false, false, capacityProps))
    })
    await act(async () => root.render(floatingElement(true, false, capacityProps)))
    const estimate = vi.spyOn(capacityEstimates, 'estimateBlockTokens')
    await act(async () => useChatStore.setState({ blocks: [{ kind: 'assistant', id: 'capacity-a', text: 'Synthetic first delta' }] }))
    expect(estimate).not.toHaveBeenCalled()
    await act(async () => ring().click())
    expect(estimate).toHaveBeenCalled()
    await key(ring(), 'Escape'); estimate.mockClear()
    await act(async () => useChatStore.setState({ blocks: [{ kind: 'assistant', id: 'capacity-b', text: 'Synthetic later delta' }] }))
    expect(estimate).not.toHaveBeenCalled()
  })
})

it('lays out whole-session activity, usage and capacity as independent dock entries without shortcut guidance',async()=>{
  const count=(reportedValue:number)=>({reportedValue,knownAttemptCount:1,attemptCount:1,complete:true})
  const billing=ProviderBillingV1Schema.parse({inventoryComplete:true,closed:true,attemptCount:1,settledAttemptCount:1,
    inputTokens:count(10),outputTokens:count(2),totalTokens:count(12),uncachedInputTokens:count(10),
    cacheReadTokens:count(0),cacheWriteTokens:count(0),reasoningTokens:count(0)})
  const metrics=ConversationMetricsV1Schema.parse({turnCount:1,stepCount:1,
    llmMs:{reportedValue:1000,knownStepCount:1,complete:true},toolMs:{reportedValue:0,knownCallCount:0,callCount:0,complete:true},
    ttftMs:10,ttftSteps:1,decodeMs:1000,decodeTokens:2,decodeSteps:1})
  await act(async()=>root.render(null))
  const settings: AppSettingsV1 = {
    version: 1, locale: 'zh', theme: 'light', uiFontScale: 'small', performanceUsageMode: 'detailed',
    provider: defaultModelProviderSettings(), runtime: defaultAnalytixRuntimeSettings(), workspaceRoot: '/workspace/fixture',
    log: { enabled: false, retentionDays: 2 }, notifications: { turnComplete: true },
    appBehavior: normalizeAppBehaviorSettings(), keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(), claw: defaultClawSettings(), schedule: defaultScheduleSettings(),
    guiUpdate: { channel: 'stable' }, codePromptPrefix: '', disabledSkillIds: []
  }
  const readSettings=vi.spyOn(rendererRuntimeClient,'getSettings').mockResolvedValue(normalizeAppSettings(settings))
  vi.spyOn(rendererRuntimeClient,'runtimeRequest').mockImplementation(async()=>({ok:true,status:200,body:JSON.stringify({group_by:'thread',buckets:[{
    thread_id:'menu-fixture',conversationUsage:{schemaVersion:'conversation-usage.v1',billing,metrics,turns:[]}}]})}))
  await act(async()=>{
    useChatStore.setState(state=>({usageRefreshKey:state.usageRefreshKey+1}))
    root.render(floatingElement(false,false,{contextWindowTokens:1000,runtimeToolCount:0,runtimeSkillCount:0}))
  })
  const dock=host.querySelector('[data-composer-stats-dock]')!
  const entries=[...dock.children]
  expect(entries).toHaveLength(3)
  expect(entries[0].getAttribute('data-composer-stat')).toBe('activity')
  expect(entries[1].getAttribute('data-composer-stat')).toBe('usage')
  expect(readSettings).toHaveBeenCalled()
  expect(entries[1].textContent).toContain(i18n.t('common:dshUsageTokens',{tokens:'12'}))
  expect(entries[2].querySelector('button.ds-composer-context')).not.toBeNull()
  expect(host.querySelector('.ds-composer-footer-hint')).toBeNull()
  expect(host.querySelector('.ds-composer-footer')).toBeNull()
  expect(dock.querySelector('.ds-composer-footer-hint')).toBeNull()
  await act(async()=>root.render(createElement(InputBar,{...floatingElement().props,key:'stats-only'})))
  expect([...host.querySelector('[data-composer-stats-dock]')!.children].map(entry=>entry.getAttribute('data-composer-stat'))).toEqual(['activity','usage'])
  expect(host.querySelector('button.ds-composer-context')).toBeNull()
  await act(async()=>root.render(floatingElement(false,false,{contextWindowTokens:1000,presentationPhase:'hero'})))
  expect(host.querySelector('[data-composer-stats-dock] button.ds-composer-context')).not.toBeNull()
  expect(host.querySelector('[data-composer-stat]')).toBeNull()
  expect(host.querySelector('.ds-composer-footer-hint')).toBeNull()
  expect(host.querySelector('.ds-composer-footer')).toBeNull()
})


it('removes ordinary shortcut guidance without removing editor names, placeholders or error feedback', async () => {
  const attachmentUploadError = 'Synthetic attachment failure'
  await act(async () => root.render(floatingElement(false,false,{presentationPhase:'active',attachmentUploadError})))
  const editor = host.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
  expect(host.querySelector('.ds-composer-footer')).toBeNull()
  expect(editor.getAttribute('aria-label')).toBe(i18n.t('common:placeholder'))
  expect(editor.querySelector('[data-placeholder]')?.getAttribute('data-placeholder')).toBe(i18n.t('common:placeholder'))
  expect(host.textContent).toContain(attachmentUploadError)
  await act(async () => root.render(floatingElement(false,false,{presentationPhase:'active',runtimeReady:false,attachmentUploadError})))
  expect(host.querySelector('[data-composer-prompt-editor] .ProseMirror')).toBe(editor)
  expect(editor.getAttribute('aria-label')).toBe(i18n.t('common:runtimeActionNeedsConnection'))
  expect(host.querySelector('.ds-composer-footer-hint')?.textContent).toBe(i18n.t('common:composerOfflineHint'))
  expect(host.textContent).toContain(attachmentUploadError)
})
