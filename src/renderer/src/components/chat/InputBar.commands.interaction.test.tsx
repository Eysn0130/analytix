// @vitest-environment jsdom
import { act, createElement, useState, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { HubAgentMarketplaceSyncResult, HubAgentPluginListItem } from '@shared/analytix-api'
import type { WorkspaceEntry } from '@shared/workspace-file'
import { analytixToolPermissionModeSettings } from '@shared/app-settings'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { InputBar } from './InputBar'

type InputBarProps = ComponentProps<typeof InputBar>
const plugin: HubAgentPluginListItem = {
  name: 'fixture-browser', marketplaceName: 'Fixture marketplace', upstreamMarketplaceName: 'Fixture marketplace',
  pluginName: 'fixture-browser', platform: 'mac-arm64', version: '1.0.0', displayName: 'Fixture Browser',
  shortDescription: 'Browse with the installed fixture plugin.', category: 'tools', developerName: 'Fixture developer',
  source: {}, policy: {}, manifest: {}, marketplaceEntry: {}, mcpServerIds: [], requiredInstall: false, installed: true
}
const marketplace: HubAgentMarketplaceSyncResult = {
  ok: true, platform: 'mac-arm64', marketplaceName: 'Fixture marketplace', marketplaceRoot: '/fixture-marketplace',
  generatedAt: '2026-10-19T00:00:00Z', changed: false, plugins: [plugin], skills: [],
  requiredPluginNames: [], requiredSkillNames: [], copiedSkills: [], removedSkills: [], errors: []
}
const permission = analytixToolPermissionModeSettings('request-approval')
const callbacks = {
  setInput: vi.fn(), setMode: vi.fn(), onSend: vi.fn(), onInterrupt: vi.fn(), onPlanCommand: vi.fn(), onNewCommand: vi.fn(),
  onPickAttachments: vi.fn(), onPickFileReferences: vi.fn(), onOpenFileReferencePicker: vi.fn(), onToggleWorktreeMode: vi.fn(),
  onExecutionSettingsChange: vi.fn(), onOpenPermissionSettings: vi.fn(), onComposerModelChange: vi.fn()
}
let root: Root
let host: HTMLDivElement
let previousStore: ReturnType<typeof useChatStore.getState>
let previousApi: PropertyDescriptor | undefined
let previousLanguage: string
let frames: Map<number, FrameRequestCallback>
let nextFrame: number
let changeDraft: (value: string) => void
let getDraft: () => string
const geometry = ['getClientRects', 'getBoundingClientRect'] as const
let rangeGeometry: Array<PropertyDescriptor | undefined>
let previousScrollIntoView: PropertyDescriptor | undefined
let scrollCalls: Array<{ element: HTMLElement; options?: boolean | ScrollIntoViewOptions }>

function Harness({ initialInput = '', ...overrides }: Partial<InputBarProps> & { initialInput?: string }) {
  const [input, setInput] = useState(initialInput)
  changeDraft = value => { callbacks.setInput(value); setInput(value) }
  getDraft = () => input
  return createElement(InputBar, {
    input, setInput: changeDraft, mode: 'agent', setMode: callbacks.setMode, busy: false,
    runtimeReady: true, hasActiveThread: true, hideThreadContextPanels: true, voiceDictationEnabled: false,
    composerModel: 'deepseek-v4-pro', composerProviderId: 'deepseek',
    composerPickList: ['deepseek-v4-pro', 'deepseek-v4-flash'],
    composerModelGroups: [{ providerId: 'deepseek', label: 'DeepSeek', modelIds: ['deepseek-v4-pro', 'deepseek-v4-flash'] }],
    onComposerModelChange: callbacks.onComposerModelChange,
    executionSettings: permission, onExecutionSettingsChange: callbacks.onExecutionSettingsChange,
    onOpenPermissionSettings: callbacks.onOpenPermissionSettings,
    queuedMessages: [], onRemoveQueuedMessage: vi.fn(), onSend: callbacks.onSend, onInterrupt: callbacks.onInterrupt,
    onPlanCommand: callbacks.onPlanCommand, onNewCommand: callbacks.onNewCommand,
    fileReferenceEnabled: true, onPickFileReferences: callbacks.onPickFileReferences,
    onOpenFileReferencePicker: callbacks.onOpenFileReferencePicker,
    attachmentUploadEnabled: true, onPickAttachments: callbacks.onPickAttachments,
    useWorktreePool: false, onToggleWorktreeMode: callbacks.onToggleWorktreeMode,
    skillCommands: [{ id: 'fixture-skill', name: 'Fixture Skill', description: 'Run the fixture skill.', scope: 'project', root: '/workspace/commands/.agents/skills/fixture-skill' }],
    ...overrides
  })
}

const editor = () => host.querySelector<HTMLElement>('[data-composer-prompt-editor] .ProseMirror')!
const plus = () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('common:composerMenuTitle')}"]`)!
const commandMenu = () => host.querySelector<HTMLElement>('[data-composer-command-menu]')
const commandItem = (id: string) => commandMenu()?.querySelector<HTMLButtonElement>(`[data-command-id="${id}"]`) ?? null
const executionMenu = () => document.querySelector<HTMLElement>('[data-composer-execution-menu]')
const modelMenu = () => document.querySelector<HTMLElement>('[data-model-picker-pane]')
const mentionMenu = () => host.querySelector<HTMLElement>(`[role="listbox"][aria-label="${i18n.t('common:composerAtMentionMenuTitle')}"]`)
const mentionOptions = () => [...(mentionMenu()?.querySelectorAll<HTMLButtonElement>('[role="option"][data-command-id]') ?? [])]
function expectMentionHighlight(index: number) {
  const options = mentionOptions()
  expect(options[index]).toBeDefined()
  expect(options.filter(option => option.dataset.active === 'true')).toEqual([options[index]])
  expect(options.filter(option => option.getAttribute('aria-selected') === 'true')).toEqual([options[index]])
  expect(mentionMenu()?.getAttribute('aria-activedescendant')).toBe(options[index].id)
  return options[index]
}
function mentionFiles(workspaceRoot: string, prefix = 'long-note'): WorkspaceEntry[] {
  return Array.from({ length: 8 }, (_, index) => {
    const name = `${prefix}-${String(index + 1).padStart(2, '0')} details.md`
    return { name, path: `${workspaceRoot}/${name}`, type: 'file', ext: '.md' }
  })
}
function installMentionFileLookup(workspaceRoot: string, entries: WorkspaceEntry[] | Promise<WorkspaceEntry[]>) {
  useChatStore.setState({ workspaceRoot })
  const listDirectory = vi.fn(async ({ path }: { path: string }) => ({ ok: true, entries: path === workspaceRoot ? await entries : [] }))
  Object.defineProperty(window, 'analytix', { configurable: true, value: {
    ...window.analytix, files: { ...window.analytix.files, listDirectory }
  } })
  return listDirectory
}
async function flushFrames() {
  for (let pass = 0; pass < 3 && frames.size > 0; pass++) {
    const pending = [...frames.values()]; frames.clear()
    await act(async () => { for (const callback of pending) callback(0) })
  }
}
async function render(overrides: Partial<InputBarProps> & { initialInput?: string } = {}) {
  await act(async () => root.render(createElement(Harness, overrides)))
  await flushFrames()
}
async function setDraft(value: string) {
  await act(async () => changeDraft(value))
  await flushFrames()
}
async function key(target: HTMLElement, value: string, options: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...options })
  await act(async () => { target.dispatchEvent(event) })
  await flushFrames()
  return event
}
async function openPlus() {
  await act(async () => plus().click())
  await flushFrames()
  expect(commandMenu()).not.toBeNull()
}
async function openCommand(id: 'permission' | 'model', surface: 'plus' | 'slash') {
  if (surface === 'plus') await openPlus()
  else await setDraft(`/${id}`)
  const item = commandItem(id)
  expect(item).not.toBeNull(); expect(item!.disabled).toBe(false)
  await act(async () => item!.click())
  await flushFrames()
}
function expectNoDispatch() {
  for (const [name, callback] of Object.entries(callbacks)) {
    if (name !== 'setInput') expect(callback, name).not.toHaveBeenCalled()
  }
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} })))
  frames = new Map(); nextFrame = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++nextFrame, callback); return nextFrame })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => { frames.delete(id) })
  rangeGeometry = geometry.map(name => Object.getOwnPropertyDescriptor(Range.prototype, name))
  // jsdom has no layout; keep the real ProseMirror selection and paste/history handlers.
  Object.defineProperty(Range.prototype, 'getClientRects', { configurable: true, value: () => [] })
  Object.defineProperty(Range.prototype, 'getBoundingClientRect', { configurable: true, value: () => new DOMRect() })
  previousScrollIntoView = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollIntoView')
  scrollCalls = []
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', { configurable: true,
    value: function (this: HTMLElement, options?: boolean | ScrollIntoViewOptions) { scrollCalls.push({ element: this, options }) } })
  for (const callback of Object.values(callbacks)) callback.mockClear()
  previousStore = useChatStore.getState()
  useChatStore.setState({ activeThreadId: 'commands-thread', activeThreadGoal: null, activeGoalActivation: null,
    route: 'chat', workspaceRoot: '/workspace/commands', blocks: [], threads: [], busy: false,
    userInputDraftInterruptions: {}, deletedUserInputDraftThreadIds: {} })
  previousApi = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: {
    app: { syncHubAgentMarketplace: vi.fn(async () => marketplace) },
    files: { listDirectory: vi.fn(async () => ({ ok: true, entries: [] })) }
  } })
  previousLanguage = i18n.language
  await i18n.changeLanguage('en')
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount())
  host.remove(); useChatStore.setState(previousStore)
  if (previousApi) Object.defineProperty(window, 'analytix', previousApi)
  else Reflect.deleteProperty(window, 'analytix')
  geometry.forEach((name, index) => {
    const descriptor = rangeGeometry[index]
    if (descriptor) Object.defineProperty(Range.prototype, name, descriptor)
    else Reflect.deleteProperty(Range.prototype, name)
  })
  if (previousScrollIntoView) Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', previousScrollIntoView)
  else Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView')
  await i18n.changeLanguage(previousLanguage)
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

describe('real InputBar shared command catalog', () => {
  it('uses the same core groups for plus and slash while retaining plugin, skill and worktree contributions', async () => {
    // Opening the existing @ route exercises the real installed-plugin bridge before comparing both catalog views.
    await render({ initialInput: '@fixture' })
    expect(mentionMenu()?.textContent).toContain(plugin.displayName)
    await setDraft('')
    await openPlus()
    const ids = () => [...commandMenu()!.querySelectorAll<HTMLElement>('[data-command-id]')].map(node => node.dataset.commandId)
    const sections = () => [...commandMenu()!.querySelectorAll<HTMLElement>('[data-command-section]')].map(node => node.dataset.commandSection)
    const plusIds = ids()
    expect(plusIds.slice(0, 8)).toEqual(['file', 'goal', 'plan', 'feedback', 'compact', 'permission', 'model', 'export'])
    expect(plusIds).toEqual(expect.arrayContaining(['plugin:fixture-browser', 'skill:fixture-skill', 'worktree']))
    expect(sections()).toEqual(['add', 'commands', 'extensions', 'skills', 'plugins', 'worktree'])
    await key(document.activeElement as HTMLElement, 'Escape')
    await setDraft('/')
    expect(commandMenu()?.getAttribute('role')).toBe('listbox')
    expect(ids()).toEqual(plusIds)
    expect(sections()).toEqual(['add', 'commands', 'extensions', 'skills', 'plugins', 'worktree'])
    await setDraft('/permission')
    expect(commandItem('permission')).not.toBeNull()
    expect(sections()).toEqual([])
    expectNoDispatch()
  })

  it.each(['feedback', 'export'] as const)('exposes the unavailable %s reason without dispatching from plus, slash or Tab', async id => {
    await render()
    await openPlus()
    const unavailable = commandItem(id)!
    expect(unavailable.disabled).toBe(true)
    const reasonKey = `common:slashCommand${id === 'feedback' ? 'Feedback' : 'Export'}Unavailable`
    expect(i18n.exists(reasonKey)).toBe(true)
    const reason = i18n.t(reasonKey)
    expect(unavailable.querySelector('.dsh-composer-command-description')?.textContent).toBe(reason)
    await act(async () => unavailable.click())
    expectNoDispatch()
    await key(document.activeElement as HTMLElement, 'Escape')
    await setDraft(`/${id}`)
    expect(commandItem(id)!.disabled).toBe(true)
    const tab = await key(editor(), 'Tab')
    expect(tab.defaultPrevented).toBe(true)
    expect(getDraft()).toBe(`/${id}`)
    expectNoDispatch()
  })

  it.each(['plus', 'slash'] as const)('opens the actual permission picker from %s without changing permission until an explicit option', async surface => {
    await render()
    await openCommand('permission', surface)
    expect(commandMenu()).toBeNull()
    expect(executionMenu()).not.toBeNull()
    expect(callbacks.onExecutionSettingsChange).not.toHaveBeenCalled()
    expect(callbacks.onOpenPermissionSettings).not.toHaveBeenCalled()
    const options = executionMenu()!.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]')
    await act(async () => options[1].click())
    expect(callbacks.onExecutionSettingsChange).toHaveBeenCalledExactlyOnceWith({ approvalPolicy: 'on-request', sandboxMode: 'workspace-write' })
    expect(executionMenu()).toBeNull()
    expect(callbacks.onSend).not.toHaveBeenCalled()
  })

  it.each(['plus', 'slash'] as const)('opens the actual model picker from %s without selecting a model until an explicit option', async surface => {
    await render()
    await openCommand('model', surface)
    expect(commandMenu()).toBeNull()
    expect(modelMenu()).not.toBeNull()
    expect(callbacks.onComposerModelChange).not.toHaveBeenCalled()
    if (modelMenu()!.dataset.modelPickerPane !== 'model') {
      await act(async () => modelMenu()!.querySelector<HTMLButtonElement>('[data-model-pane="model"]')!.click())
    }
    const option = document.querySelector<HTMLButtonElement>('[data-model-index="1"]')!
    expect(option).not.toBeNull()
    await act(async () => option.click())
    expect(callbacks.onComposerModelChange).toHaveBeenCalledExactlyOnceWith('deepseek-v4-flash', 'deepseek')
    expect(modelMenu()).toBeNull()
    expect(callbacks.onSend).not.toHaveBeenCalled()
  })
})

describe('real InputBar command dismissal and keyboard ownership', () => {
  it('dismisses slash Escape while preserving the controlled draft and selection capability for the next edit', async () => {
    await render({ initialInput: '/plan' })
    expect(commandItem('plan')).not.toBeNull()
    const escape = await key(editor(), 'Escape')
    expect(escape.defaultPrevented).toBe(true)
    expect(commandMenu()).toBeNull()
    expect(getDraft()).toBe('/plan')
    expect(editor().textContent).toBe('/plan')
    expect(callbacks.setInput).not.toHaveBeenCalled()
    expectNoDispatch()
    await setDraft('/pla')
    expect(commandItem('plan')).not.toBeNull()
  })

  it('picks the highlighted slash command with Tab through its real handler', async () => {
    await render({ initialInput: '/plan' })
    const tab = await key(editor(), 'Tab')
    expect(tab.defaultPrevented).toBe(true)
    expect(callbacks.onPlanCommand).toHaveBeenCalledOnce()
    expect(callbacks.setMode).toHaveBeenCalledExactlyOnceWith('plan')
    expect(getDraft()).toBe('')
    expect(commandMenu()).toBeNull()
    expect(callbacks.onSend).not.toHaveBeenCalled()
  })

  it('dismisses slash Shift+Tab while leaving native focus navigation enabled and retaining the draft', async () => {
    await render({ initialInput: '/plan' })
    const tab = await key(editor(), 'Tab', { shiftKey: true })
    expect(tab.defaultPrevented).toBe(false)
    expect(commandMenu()).toBeNull()
    expect(getDraft()).toBe('/plan')
    expect(callbacks.setInput).not.toHaveBeenCalled()
    expectNoDispatch()
    // jsdom does not perform the browser's native Tab focus traversal.
  })

  it('dismisses @ Shift+Tab without inserting the highlighted installed-plugin token', async () => {
    await render({ initialInput: '@fixture' })
    expect(mentionMenu()?.textContent).toContain(plugin.displayName)
    const tab = await key(editor(), 'Tab', { shiftKey: true })
    expect(tab.defaultPrevented).toBe(false)
    expect(mentionMenu()).toBeNull()
    expect(getDraft()).toBe('@fixture')
    expect(callbacks.setInput).not.toHaveBeenCalled()
    expectNoDispatch()
  })

  it.each((['thread', 'workspace'] as const).flatMap(scope =>
    (['plus', 'permission', 'model', 'slash', 'mention'] as const).map(popup => ({ scope, popup }))
  ))('retires the $popup popup on a $scope change without dispatching the former owner action', async ({ scope, popup }) => {
    await render({ initialInput: popup === 'slash' ? '/plan' : popup === 'mention' ? '@fixture' : '' })
    if (popup === 'plus') await openPlus()
    else if (popup === 'permission' || popup === 'model') await openCommand(popup, 'plus')
    if (popup === 'permission') expect(executionMenu()).not.toBeNull()
    else if (popup === 'model') expect(modelMenu()).not.toBeNull()
    else if (popup === 'mention') expect(mentionMenu()).not.toBeNull()
    else expect(commandMenu()).not.toBeNull()
    const priorDraft = getDraft()
    await act(async () => useChatStore.setState(scope === 'thread'
      ? { activeThreadId: 'commands-other-thread' }
      : { workspaceRoot: '/workspace/commands-other' }))
    await flushFrames()
    expect(commandMenu()).toBeNull()
    expect(executionMenu()).toBeNull()
    expect(modelMenu()).toBeNull()
    expect(mentionMenu()).toBeNull()
    expect(getDraft()).toBe(priorDraft)
    expectNoDispatch()
  })

  it.each(['thread', 'workspace'] as const)('retires old disclosures and intake through a batched %s A → B → A round trip', async scope => {
    await render({ initialInput: 'Retain the owner draft' })
    const originalEditor = editor()
    const roundTrip = () => {
      if (scope === 'thread') {
        useChatStore.setState({ activeThreadId: 'commands-round-trip' })
        useChatStore.setState({ activeThreadId: 'commands-thread' })
      } else {
        useChatStore.setState({ workspaceRoot: '/workspace/commands-round-trip' })
        useChatStore.setState({ workspaceRoot: '/workspace/commands' })
      }
    }
    await openCommand('model', 'plus')
    const formerPicker = modelMenu()!
    await act(async () => roundTrip())
    await flushFrames()
    expect(modelMenu()).toBeNull()
    expect(formerPicker.isConnected).toBe(false)
    expect(getDraft()).toBe('Retain the owner draft')

    await openPlus()
    const formerPlan = commandItem('plan')!
    await act(async () => {
      roundTrip()
      // A still-mounted old menu cannot act between the store transition and React's commit.
      formerPlan.click()
    })
    await flushFrames()
    expect(commandMenu()).toBeNull()
    expect(getDraft()).toBe('Retain the owner draft')

    await setDraft('/plan')
    expect(commandItem('plan')).not.toBeNull()
    await act(async () => roundTrip())
    await flushFrames()
    expect(commandMenu()).toBeNull()
    expect(getDraft()).toBe('/plan')

    await setDraft('@fixture')
    expect(mentionMenu()).not.toBeNull()
    await act(async () => roundTrip())
    await flushFrames()
    expect(mentionMenu()).toBeNull()
    expect(getDraft()).toBe('@fixture')

    const image = new File(['round-trip-image'], 'round-trip.png', { type: 'image/png' })
    const transfer = {
      types: ['Files'], files: [image],
      items: [{ kind: 'file', type: 'image/png', getAsFile: () => image }], dropEffect: 'none'
    }
    const drag = (type: string) => {
      const event = new Event(type, { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'dataTransfer', { value: transfer })
      return event
    }
    await act(async () => document.body.dispatchEvent(drag('dragenter')))
    expect(document.querySelector('[data-composer-drop-overlay]')).not.toBeNull()
    await act(async () => {
      roundTrip()
      document.body.dispatchEvent(drag('drop'))
    })
    await flushFrames()
    expect(document.querySelector('[data-composer-drop-overlay]')).toBeNull()
    expect(editor()).toBe(originalEditor)
    expect(getDraft()).toBe('@fixture')
    expectNoDispatch()
  })

  it.each((['thread', 'workspace'] as const).flatMap(scope =>
    (['plugin', 'file'] as const).map(kind => ({ scope, kind }))
  ))('rejects the old @ $kind handler before commit during a batched $scope A → B → A round trip', async ({ scope, kind }) => {
    const workspaceA = `/workspace/commands-old-mention-${scope}-${kind}`
    const fileName = 'owner-round-trip-note.md'
    const input = kind === 'plugin' ? '@fixture' : '@owner-round-trip-note'
    const onAddFileReference = vi.fn()
    useChatStore.setState({ workspaceRoot: workspaceA })
    if (kind === 'file') {
      const listDirectory = vi.fn(async ({ path }: { path: string }) => ({ ok: true, entries: path === workspaceA ? [
        { name: fileName, path: `${workspaceA}/${fileName}`, type: 'file' as const, ext: '.md' }
      ] : [] }))
      Object.defineProperty(window, 'analytix', { configurable: true, value: {
        ...window.analytix, files: { ...window.analytix.files, listDirectory }
      } })
    }
    // Only advance the real lookup debounce; keep the existing rAF/geometry harness and actual menu handlers.
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: input, onAddFileReference })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      const formerMenu = mentionMenu()
      expect(formerMenu).not.toBeNull()
      const title = kind === 'plugin' ? plugin.displayName : fileName
      const formerOption = [...formerMenu!.querySelectorAll<HTMLButtonElement>('button')]
        .find(button => button.textContent?.includes(title))
      expect(formerOption).toBeDefined()
      callbacks.setInput.mockClear()
      let connectedBeforeClick = false
      await act(async () => {
        if (scope === 'thread') {
          useChatStore.setState({ activeThreadId: 'commands-old-mention-other' })
          useChatStore.setState({ activeThreadId: 'commands-thread' })
        } else {
          useChatStore.setState({ workspaceRoot: `${workspaceA}-other` })
          useChatStore.setState({ workspaceRoot: workspaceA })
        }
        // Prove this exercises the live old callback before React removes the former menu.
        connectedBeforeClick = formerOption!.isConnected
        formerOption!.click()
      })
      await flushFrames()
      expect(connectedBeforeClick).toBe(true)
      expect(formerOption!.isConnected).toBe(false)
      expect(mentionMenu()).toBeNull()
      expect(callbacks.setInput).not.toHaveBeenCalled()
      expect(onAddFileReference).not.toHaveBeenCalled()
      expect(getDraft()).toBe(input)
      expect(editor().textContent).toBe(input)
      expectNoDispatch()
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('real InputBar shared mention menu', () => {
  it('retains plugin, skill and file order within the existing eight-option limit', async () => {
    const workspaceRoot = '/workspace/commands-mention-order'
    const files = mentionFiles(workspaceRoot, 'fixture')
    installMentionFileLookup(workspaceRoot, files)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: '@fixture' })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(mentionOptions().map(option => option.dataset.commandId)).toEqual([
        'plugin:fixture-browser', 'skill:fixture-skill', ...files.slice(0, 6).map(file => `file:file:${file.name}`)
      ])
      expectMentionHighlight(0)
      expectNoDispatch()
    } finally { vi.useRealTimers() }
  })

  it('scrolls all eight file options nearest during keyboard navigation and keeps one highlight after pointer movement', async () => {
    const workspaceRoot = '/workspace/commands-mention-navigation'
    const files = mentionFiles(workspaceRoot)
    const onAddFileReference = vi.fn()
    installMentionFileLookup(workspaceRoot, files)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: '@long-note', onAddFileReference })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(mentionOptions().map(option => option.dataset.commandId)).toEqual(files.map(file => `file:file:${file.name}`))
      expectMentionHighlight(0)
      for (const index of [1, 2, 3, 4, 5, 6, 7, 0]) {
        scrollCalls.length = 0
        expect((await key(editor(), 'ArrowDown')).defaultPrevented).toBe(true)
        const selected = expectMentionHighlight(index)
        expect(scrollCalls.at(-1)).toEqual({ element: selected, options: { block: 'nearest' } })
      }
      scrollCalls.length = 0
      expect((await key(editor(), 'ArrowUp')).defaultPrevented).toBe(true)
      expect(scrollCalls.at(-1)).toEqual({ element: expectMentionHighlight(7), options: { block: 'nearest' } })
      await act(async () => mentionOptions()[3].dispatchEvent(new MouseEvent('mousemove', { bubbles: true })))
      expectMentionHighlight(3)
      scrollCalls.length = 0
      await key(editor(), 'ArrowUp')
      expect(scrollCalls.at(-1)).toEqual({ element: expectMentionHighlight(2), options: { block: 'nearest' } })
      expect(getDraft()).toBe('@long-note')
      expect(onAddFileReference).not.toHaveBeenCalled()
      expectNoDispatch()
    } finally { vi.useRealTimers() }
  })

  it.each(['Enter', 'Tab'])('inserts the highlighted original file reference and quoted token with %s', async acceptKey => {
    const workspaceRoot = `/workspace/commands-mention-accept-${acceptKey.toLowerCase()}`
    const files = mentionFiles(workspaceRoot)
    const onAddFileReference = vi.fn()
    installMentionFileLookup(workspaceRoot, files)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: '@long-note', onAddFileReference })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(mentionOptions()).toHaveLength(8)
      await key(editor(), 'Enter', { isComposing: true })
      expect(onAddFileReference).not.toHaveBeenCalled()
      expect(callbacks.onSend).not.toHaveBeenCalled()
      // The synthetic composing key retains ProseMirror's native newline without accepting a mention.
      expect(getDraft()).toBe('@long-note\n')
      await setDraft('@long-note')
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      await flushFrames()
      expect(mentionOptions()).toHaveLength(8)
      await act(async () => mentionOptions()[5].dispatchEvent(new MouseEvent('mousemove', { bubbles: true })))
      expectMentionHighlight(5)
      await key(editor(), 'ArrowDown')
      expectMentionHighlight(6)
      const accepted = await key(editor(), acceptKey)
      expect(accepted.defaultPrevented).toBe(true)
      expect(onAddFileReference).toHaveBeenCalledExactlyOnceWith({
        path: `${workspaceRoot}/long-note-07 details.md`, relativePath: 'long-note-07 details.md',
        name: 'long-note-07 details.md', type: 'file'
      })
      expect(getDraft()).toBe('@"long-note-07 details.md" ')
      expect(mentionMenu()).toBeNull()
      expectNoDispatch()
    } finally { vi.useRealTimers() }
  })

  it('dismisses a highlighted file with Escape while preserving its controlled draft', async () => {
    const workspaceRoot = '/workspace/commands-mention-file-escape'
    const onAddFileReference = vi.fn()
    installMentionFileLookup(workspaceRoot, mentionFiles(workspaceRoot))
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: 'Retain text @long-note', onAddFileReference })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(mentionOptions()).toHaveLength(8)
      await key(editor(), 'ArrowUp')
      expectMentionHighlight(7)
      callbacks.setInput.mockClear()
      expect((await key(editor(), 'Escape')).defaultPrevented).toBe(true)
      expect(mentionMenu()).toBeNull()
      expect(getDraft()).toBe('Retain text @long-note')
      expect(editor().textContent).toBe('Retain text @long-note')
      expect(callbacks.setInput).not.toHaveBeenCalled()
      expect(onAddFileReference).not.toHaveBeenCalled()
      expectNoDispatch()
    } finally { vi.useRealTimers() }
  })

  it('keeps loading and empty file lookup states outside the selectable options', async () => {
    const workspaceRoot = '/workspace/commands-mention-file-empty'
    let resolveFiles!: (entries: WorkspaceEntry[]) => void
    const files = new Promise<WorkspaceEntry[]>(resolve => { resolveFiles = resolve })
    installMentionFileLookup(workspaceRoot, files)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: '@unmatched-file-query' })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(mentionMenu()?.querySelector('[role="status"]')).not.toBeNull()
      expect(mentionMenu()?.querySelector('[role="status"]')?.textContent).toBe(i18n.t('common:composerAtMentionLoading'))
      expect(mentionOptions()).toHaveLength(0)
      expect(mentionMenu()?.getAttribute('aria-activedescendant')).toBeNull()
      await act(async () => resolveFiles([]))
      expect(mentionMenu()?.querySelector('[role="status"]')).not.toBeNull()
      expect(mentionMenu()?.querySelector('[role="status"]')?.textContent).toBe(i18n.t('common:composerAtMentionEmpty'))
      expect(mentionOptions()).toHaveLength(0)
      expect(mentionMenu()?.querySelector('[data-active="true"]')).toBeNull()
      expectNoDispatch()
    } finally { resolveFiles([]); vi.useRealTimers() }
  })

  it('does not revive or insert a late file lookup after switching threads', async () => {
    const workspaceRoot = '/workspace/commands-mention-file-late-owner'
    let resolveFiles!: (entries: WorkspaceEntry[]) => void
    const files = new Promise<WorkspaceEntry[]>(resolve => { resolveFiles = resolve })
    const listDirectory = installMentionFileLookup(workspaceRoot, files)
    const onAddFileReference = vi.fn()
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    try {
      await render({ initialInput: '@long-note', onAddFileReference })
      await act(async () => { await vi.advanceTimersByTimeAsync(100) })
      expect(listDirectory).toHaveBeenCalledExactlyOnceWith({ workspaceRoot, path: workspaceRoot })
      expect(mentionOptions()).toHaveLength(0)
      callbacks.setInput.mockClear()
      await act(async () => useChatStore.setState({ activeThreadId: 'commands-file-next-thread' }))
      await flushFrames()
      expect(mentionMenu()).toBeNull()
      await act(async () => resolveFiles(mentionFiles(workspaceRoot)))
      await flushFrames()
      expect(mentionMenu()).toBeNull()
      expect(host.querySelector('[data-command-id^="file:"]')).toBeNull()
      expect(getDraft()).toBe('@long-note')
      expect(editor().textContent).toBe('@long-note')
      expect(callbacks.setInput).not.toHaveBeenCalled()
      expect(onAddFileReference).not.toHaveBeenCalled()
      expectNoDispatch()
    } finally { resolveFiles([]); vi.useRealTimers() }
  })
})

describe('real InputBar hero workspace accessory', () => {
  it('mounts the actual Workspace/Git seats only in hero and preserves the editor across hero → active', async () => {
    const getGitBranches = vi.fn(async () => ({ ok: true as const, repositoryRoot: '/workspace/commands',
      currentBranch: 'fixture-main', branches: [{ name: 'fixture-main', current: true }], dirtyCount: 0 }))
    Object.defineProperty(window, 'analytix', { configurable: true, value: {
      ...window.analytix, workspace: { getGitBranches }
    } })
    useChatStore.setState({ runtimeConnection: 'ready', codeWorkspaceRoots: ['/workspace/commands'] })
    await render({ initialInput: 'Same resident editor', presentationPhase: 'hero' })
    const originalEditor = editor()
    const accessory = host.querySelector('.dsh-input-accessory')!
    const workspace = accessory.querySelector('.ds-workspace-project-picker')!
    const git = accessory.querySelector('.ds-git-branch-picker')!
    expect(workspace).not.toBeNull(); expect(git).not.toBeNull()
    expect(workspace.querySelector('button')?.title).toBe(i18n.t('common:workingDirectory'))
    expect(workspace.textContent).toContain('commands')
    expect(git.querySelector('button')?.title).toBe(i18n.t('common:gitBranch'))
    expect(git.textContent).toContain('fixture-main')
    expect(getGitBranches).toHaveBeenCalledExactlyOnceWith('/workspace/commands')

    await render({ presentationPhase: 'active' })
    expect(host.querySelector('.dsh-input-accessory')).toBeNull()
    expect(host.querySelector('.ds-workspace-project-picker')).toBeNull()
    expect(host.querySelector('.ds-git-branch-picker')).toBeNull()
    expect(editor()).toBe(originalEditor)
    expect(editor().textContent).toBe('Same resident editor')
    expect(getDraft()).toBe('Same resident editor')
    expectNoDispatch()
  })
})

describe('real InputBar contenteditable clipboard transaction', () => {
  it('keeps mixed image clipboard text in the native ProseMirror paste transaction and restores it with Undo', async () => {
    await render({ initialInput: 'Existing draft: ' })
    editor().focus()
    const image = new File(['fixture-image'], 'clipboard.png', { type: 'image/png' })
    const text = 'pasted clipboard text'
    const paste = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(paste, 'clipboardData', { value: {
      files: [image], items: [{ kind: 'file', type: 'image/png', getAsFile: () => image }],
      types: ['text/plain', 'Files'], getData: (type: string) => type === 'text/plain' ? text : ''
    } })
    // Dispatch on the actual editable: PM's native listener runs before the React card bubble listener.
    await act(async () => { editor().dispatchEvent(paste) })
    expect(callbacks.onPickAttachments).toHaveBeenCalledExactlyOnceWith([image])
    expect(getDraft()).toBe('Existing draft: pasted clipboard text')
    expect(editor().textContent).toBe('Existing draft: pasted clipboard text')
    expect(callbacks.onSend).not.toHaveBeenCalled()
    const undo = await key(editor(), 'z', /Mac|iPhone|iPad|iPod/.test(navigator.platform) ? { metaKey: true } : { ctrlKey: true })
    expect(undo.defaultPrevented).toBe(true)
    expect(getDraft()).toBe('Existing draft: ')
    expect(editor().textContent).toBe('Existing draft: ')
    expect(callbacks.onPickAttachments).toHaveBeenCalledOnce()
  })
})
