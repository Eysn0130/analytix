// @vitest-environment jsdom
import { act, createElement, type ComponentProps } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { SddDraftHistoryItem } from '../../sdd/sdd-draft-history'
import { SidebarProjectsSection } from './SidebarProjectsSection'
const io = vi.hoisted(() => ({ drafts: [] as SddDraftHistoryItem[] }))
vi.mock('../../sdd/sdd-draft-history', async (importOriginal) => ({
  ...await importOriginal<typeof import('../../sdd/sdd-draft-history')>(), listSddDraftHistory: async () => io.drafts
}))
let host: HTMLDivElement, root: Root, frames: Map<number, FrameRequestCallback>, frameId: number
let bridge: PropertyDescriptor | undefined
const storageKeys = ['analytix:pinned-thread-ids:v1', 'analytix:sidebar-collapsed-workspaces:v1']
let storage: Array<string | null>
const select = vi.fn(), side = vi.fn(), fork = vi.fn(), schedule = vi.fn(), create = vi.fn(), openDraft = vi.fn()
const workspace = '/synthetic/sidebar-menu'
function props(overrides: Partial<ComponentProps<typeof SidebarProjectsSection>> = {}): ComponentProps<typeof SidebarProjectsSection> {
  return { threads: [{ id: 'thread-a', title: 'Synthetic thread', updatedAt: '2026-10-01T00:00:00Z', model: 'synthetic', mode: 'agent', workspace }],
    activeView: 'chat', activeThreadId: 'thread-a', runtimeReady: true, searchQuery: '', showArchived: false,
    workspaceRoot: workspace, workspaceRoots: [workspace], busy: false, watchTurnCompletion: {}, unreadThreadIds: {}, locale: 'en',
    onPickWorkspace: vi.fn(), onRemoveWorkspace: vi.fn(async () => undefined), onCreateThreadInWorkspace: create,
    onOpenRequirementDraft: openDraft, activeDataAnalysisEntry: null, onOpenDataAnalysis: vi.fn(), onSelectThread: select,
    onRenameThread: vi.fn(async () => undefined), onArchiveThread: vi.fn(async () => undefined),
    onDeleteThread: vi.fn(async () => undefined), onRestoreThread: vi.fn(async () => undefined), onOpenSideChat: side,
    onForkThread: fork, onOpenSchedule: schedule, onSearchQueryChange: vi.fn(), t: key => key, ...overrides }
}
async function render(overrides = {}) { await act(async () => root.render(createElement(SidebarProjectsSection, props(overrides)))) }
async function flush() { await act(async () => { for (const [id, callback] of [...frames]) { frames.delete(id); callback(0) } }) }
async function key(target: HTMLElement, value: string, extra: KeyboardEventInit = {}) {
  const event = new KeyboardEvent('keydown', { key: value, bubbles: true, cancelable: true, ...extra })
  await act(async () => target.dispatchEvent(event)); await flush(); return event
}
function button(text: string, scope: ParentNode = host) {
  const found = [...scope.querySelectorAll<HTMLButtonElement>('button')].find(node => node.textContent?.includes(text))
  if (!found) throw new Error('Missing button: ' + text)
  return found
}
const menu = () => document.querySelector<HTMLElement>('.ds-sidebar-context-menu[role="menu"]')!
const item = (text: string) => button(text, menu())
async function open(origin: HTMLElement, focus = true) {
  origin.focus()
  await act(async () => origin.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 100, clientY: 100 })))
  if (focus) await flush()
}
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} unobserve() {} })
  frames = new Map(); frameId = 0
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  storage = storageKeys.map(key => localStorage.getItem(key)); storageKeys.forEach(key => localStorage.removeItem(key))
  bridge = Object.getOwnPropertyDescriptor(window, 'analytix')
  Object.defineProperty(window, 'analytix', { configurable: true, value: { files: {
    listDirectory: vi.fn(async () => ({ ok: false, message: 'synthetic' })), read: vi.fn(async () => ({ ok: false, message: 'synthetic' })) } } })
  io.drafts = []; [select, side, fork, schedule, create, openDraft].forEach(mock => mock.mockReset())
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
})
afterEach(async () => {
  await act(async () => root.unmount()); host.remove()
  storageKeys.forEach((key, index) => storage[index] === null ? localStorage.removeItem(key) : localStorage.setItem(key, storage[index]!))
  if (bridge) Object.defineProperty(window, 'analytix', bridge); else Reflect.deleteProperty(window, 'analytix')
  vi.restoreAllMocks(); vi.unstubAllGlobals()
})
it('focuses enabled root items and enters/exits Copy with scoped arrow navigation and layered Escape', async () => {
  await render(); const origin = button('Synthetic thread'); await open(origin)
  expect(document.activeElement).toBe(item('sessionActionPin'))
  await key(document.activeElement as HTMLElement, 'End'); expect(document.activeElement).toBe(item('sidebarThreadDelete'))
  await key(document.activeElement as HTMLElement, 'ArrowDown'); expect(document.activeElement).toBe(item('sessionActionPin'))
  const copy = item('sessionActionCopy'); copy.focus(); await key(copy, 'ArrowRight')
  expect(copy.getAttribute('aria-expanded')).toBe('true')
  expect(document.getElementById(copy.getAttribute('aria-controls')!)).not.toBeNull()
  expect(document.activeElement).toBe(item('sessionActionCopyMarkdown'))
  await key(document.activeElement as HTMLElement, 'End'); expect(document.activeElement).toBe(item('sessionActionCopyCodexLink'))
  await key(document.activeElement as HTMLElement, 'Escape', { isComposing: true }); expect(copy.getAttribute('aria-expanded')).toBe('true')
  await key(document.activeElement as HTMLElement, 'ArrowLeft'); expect(document.activeElement).toBe(copy)
  expect(copy.getAttribute('aria-expanded')).toBe('false')
  await key(copy, 'Escape'); expect(menu()).toBeNull(); expect(document.activeElement).toBe(origin)
  expect(select).not.toHaveBeenCalled(); expect(side).not.toHaveBeenCalled(); expect(fork).not.toHaveBeenCalled()
})
it('skips disabled actions and keeps native Tab unprevented', async () => {
  await render({ runtimeReady: false }); const origin = button('Synthetic thread'); await open(origin)
  expect(item('sessionActionBranch').disabled).toBe(true); expect(item('sessionActionOpenSideChat').disabled).toBe(true)
  const copy = item('sessionActionCopy'); copy.focus(); await key(copy, 'ArrowRight')
  expect(document.activeElement).toBe(item('sessionActionCopyThreadId'))
  const event = await key(document.activeElement as HTMLElement, 'Tab')
  expect(event.defaultPrevented).toBe(false); expect(menu()).toBeNull(); expect(document.activeElement).toBe(origin)
})
it('returns to an enabled root when the focused Branch is disabled by the runtime gate', async () => {
  await render(); await open(button('Synthetic thread'))
  const branch = item('sessionActionBranch'); branch.focus(); await key(branch, 'ArrowRight')
  expect(document.activeElement).toBe(item('sessionActionForkCurrent'))
  await render({ runtimeReady: false })
  expect(branch.disabled).toBe(true); expect(branch.getAttribute('aria-expanded')).toBe('false')
  expect(document.activeElement).toBe(item('sessionActionPin')); expect(fork).not.toHaveBeenCalled()
})
it('cancels pending focus frames on outside focus and lets callback-owned focus win', async () => {
  await render(); const outside = document.createElement('button'); document.body.append(outside)
  try {
    await open(button('Synthetic thread'), false)
    await act(async () => outside.focus()); await flush()
    expect(menu()).toBeNull(); expect(document.activeElement).toBe(outside)
    await open(button('Synthetic thread'))
    const copy = item('sessionActionCopy'); copy.focus()
    await act(async () => copy.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true })))
    await act(async () => outside.focus()); await flush()
    expect(menu()).toBeNull(); expect(document.activeElement).toBe(outside)
    await open(button('Synthetic thread'))
    schedule.mockImplementation(() => outside.focus())
    await act(async () => item('sessionActionAddAutomation').click()); await flush()
    expect(schedule).toHaveBeenCalledOnce(); expect(document.activeElement).toBe(outside)
  } finally { outside.remove() }
})
it('does not let pointer leave close a keyboard-focused submenu', async () => {
  await render(); await open(button('Synthetic thread'))
  const copy = item('sessionActionCopy'); copy.focus(); await key(copy, 'ArrowRight')
  await act(async () => menu().dispatchEvent(new MouseEvent('pointerout', { bubbles: true, relatedTarget: document.body })))
  expect(copy.getAttribute('aria-expanded')).toBe('true')
  expect(document.activeElement).toBe(item('sessionActionCopyMarkdown'))
})
it.each(['workspace', 'draft'] as const)('uses the same entry/arrow/return focus contract for the real %s menu', async kind => {
  if (kind === 'draft') io.drafts = [{ id: 'draft-a', workspaceRoot: workspace,
    relativePath: '.analytixsdd/requirements/123e4567-e89b-12d3-a456-426614174000/requirement.md',
    title: 'Synthetic draft', source: 'disk', createdAt: '2026-10-01T00:00:00Z', updatedAt: '2026-10-01T00:00:00Z' }]
  await render(); await act(async () => { for (let n = 0; n < 20; n++) await Promise.resolve() }); await flush()
  if (kind === 'draft') await act(async () => host.querySelector<HTMLButtonElement>('button[aria-label="sddDraftHistoryExpand"]')!.click())
  const origin = button(kind === 'draft' ? 'Synthetic draft' : 'sidebar-menu')
  await open(origin)
  const first = item(kind === 'draft' ? 'sidebarContextOpenDraft' : 'sidebarWorkspaceNewThread')
  expect(document.activeElement).toBe(first)
  await key(first, 'End'); expect(document.activeElement).not.toBe(first)
  await key(document.activeElement as HTMLElement, 'Home'); expect(document.activeElement).toBe(first)
  await act(async () => first.click())
  expect(menu()).toBeNull(); expect(document.activeElement).toBe(origin)
  expect(kind === 'draft' ? openDraft : create).toHaveBeenCalledOnce()
})
