// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { NormalizedThread } from '../../agent/types'
import i18n from '../../i18n'
import { useChatStore } from '../../store/chat-store'
import { Sidebar } from './Sidebar'

vi.mock('../../sdd/sdd-draft-history', async importOriginal => ({
  ...await importOriginal<typeof import('../../sdd/sdd-draft-history')>(),
  listSddDraftHistory: async () => []
}))

const initial = useChatStore.getState()
const workspace = '/synthetic/sidebar-search'
const inventory: NormalizedThread = {
  id: 'inventory-thread', title: 'Inventory title', preview: 'Inventory preview',
  updatedAt: '2026-10-01T00:00:00Z', model: 'synthetic', mode: 'agent', workspace
}
const match: NormalizedThread = { ...inventory, id: 'body-hit', title: 'Body result title' }
let host: HTMLDivElement, root: Root
let bridge: PropertyDescriptor | undefined
let collapsed: string | null

function props(overrides: Partial<ComponentProps<typeof Sidebar>> = {}): ComponentProps<typeof Sidebar> {
  const noop = vi.fn()
  return {
    threads: [inventory], caseProjects: [], caseProjectIndexStatus: 'ready',
    caseProjectThreadsById: {}, caseProjectLoadingById: {}, caseProjectErrorsById: {},
    caseProjectExpandedById: {}, activeThreadId: inventory.id, activeView: 'chat',
    connectPhoneSidebarOpen: false, runtimeReady: true, threadSearch: '',
    showArchivedThreads: false, onThreadSearchChange: noop, onSetCaseProjectExpanded: noop,
    onSelectThread: noop, onRenameThread: vi.fn(async () => undefined),
    onArchiveThread: vi.fn(async () => undefined), onDeleteThread: vi.fn(async () => undefined),
    onRestoreThread: vi.fn(async () => undefined), onOpenSideChat: noop, onForkThread: noop,
    onNewChat: noop, onNewChatInWorkspace: noop, onNewRequirement: noop,
    onOpenRequirementDraft: noop, activeDataAnalysis: null, onOpenDataAnalysis: noop,
    onOpenSettings: noop, onToggleConnectPhone: noop, onScheduleOpen: noop, ...overrides
  }
}
async function render(overrides: Partial<ComponentProps<typeof Sidebar>> = {}): Promise<void> {
  await act(async () => root.render(createElement(Sidebar, props(overrides))))
}
function button(text: string, scope: ParentNode = host): HTMLButtonElement {
  const found = [...scope.querySelectorAll<HTMLButtonElement>('button')]
    .find(node => node.textContent?.includes(text))
  if (!found) throw new Error('Missing synthetic button: ' + text)
  return found
}

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  await i18n.changeLanguage('en')
  collapsed = localStorage.getItem('analytix:sidebar-collapsed-workspaces:v1')
  localStorage.removeItem('analytix:sidebar-collapsed-workspaces:v1')
  bridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: { files: {
    listDirectory: vi.fn(async () => ({ ok: false, message: 'synthetic' })),
    read: vi.fn(async () => ({ ok: false, message: 'synthetic' }))
  } } })
  useChatStore.setState({ ...initial, threads: [inventory], threadSearchResults: null,
    workspaceRoot: workspace, codeWorkspaceRoots: [workspace], activeThreadId: inventory.id,
    clawChannels: [], busy: false, watchTurnCompletion: {}, unreadThreadIds: {} })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove(); useChatStore.setState(initial)
  if (collapsed === null) localStorage.removeItem('analytix:sidebar-collapsed-workspaces:v1')
  else localStorage.setItem('analytix:sidebar-collapsed-workspaces:v1', collapsed)
  if (bridge) Object.defineProperty(window, 'analytix', bridge)
  else Reflect.deleteProperty(window, 'analytix')
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})

it('renders the current body-search receipt without replacing inventory or rechecking its summary', async () => {
  useChatStore.setState({ threadSearchResults: { query: 'needle-in-message-body', threads: [match] } })
  await render({ threadSearch: 'needle-in-message-body', activeThreadId: match.id })
  expect(host.textContent).toContain(match.title)
  expect(host.textContent).not.toContain(inventory.title)
  expect(useChatStore.getState().threads).toEqual([inventory])
  await render({ threadSearch: '', activeThreadId: inventory.id })
  expect(host.textContent).toContain(inventory.title)
  expect(host.textContent).not.toContain(match.title)
})

it('does not substitute inventory or an old query receipt while the latest query is pending', async () => {
  useChatStore.setState({ threadSearchResults: { query: 'old-query', threads: [match] } })
  await render({ threadSearch: 'new-query', activeThreadId: match.id })
  expect(host.textContent).not.toContain(match.title)
  expect(host.textContent).not.toContain(inventory.title)
  await act(async () => useChatStore.setState({ threadSearchResults: { query: 'new-query', threads: [] } }))
  expect(host.textContent).not.toContain(inventory.title)
})

it('retains the rename dialog and typed title when the actual rename action rejects', async () => {
  const rename = vi.fn(async () => { throw new Error('Synthetic rename refusal') })
  await render({ onRenameThread: rename })
  await act(async () => button(inventory.title).dispatchEvent(new MouseEvent('contextmenu', {
    bubbles: true, cancelable: true, clientX: 100, clientY: 100
  })))
  await act(async () => button(i18n.t('common:sessionActionRename'), document).click())
  const input = document.querySelector<HTMLInputElement>('[role="dialog"] input')!
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'Retained draft')
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await act(async () => document.querySelector<HTMLFormElement>('[role="dialog"] form')!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })))
  expect(rename).toHaveBeenCalledWith(inventory.id, 'Retained draft')
  expect(document.querySelector<HTMLInputElement>('[role="dialog"] input')?.value).toBe('Retained draft')
  expect(document.querySelector<HTMLButtonElement>('[role="dialog"] button[type="submit"]')?.disabled).toBe(false)
})
