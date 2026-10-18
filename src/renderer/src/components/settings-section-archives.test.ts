// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NormalizedThread } from '../agent/types'
import { SettingsSidebar } from './SettingsSidebar'
import { ArchivedThreadsSettingsSection, filterArchivedThreads } from './settings-section-archives'
const bridge = vi.hoisted(() => ({ list: vi.fn(), confirm: vi.fn() }))
vi.mock('../agent/registry', () => ({ getProvider: () => ({ listThreads: bridge.list }) }))
vi.mock('../lib/confirm-dialog', () => ({ confirmDialog: bridge.confirm }))

function thread(overrides: Partial<NormalizedThread> & Pick<NormalizedThread, 'id'>): NormalizedThread {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    updatedAt: overrides.updatedAt ?? '2026-06-01T00:00:00.000Z',
    model: overrides.model ?? 'deepseek-chat',
    mode: overrides.mode ?? 'agent',
    workspace: overrides.workspace ?? '/Users/zxy/project-a',
    ...(overrides.archived !== undefined ? { archived: overrides.archived } : {}),
    ...(overrides.preview ? { preview: overrides.preview } : {})
  }
}

const labels: Record<string, string> = {
  back: 'Back',
  general: 'General',
  providers: 'Providers',
  write: 'Write',
  imageGen: 'Image generation',
  mediaGeneration: 'Media generation',
  speechToText: 'Speech to text',
  agents: 'AI assistant',
  permissions: 'Permissions',
  browser: 'Browser',
  computerUse: 'Computer control',
  worktree: 'Worktrees',
  memory: 'Memory',
  archives: 'Archived chats',
  keyboardShortcuts: 'Keyboard shortcuts',
  easterEgg: 'Mode workshop',
  updates: 'Version & updates',
  debug: 'Troubleshooting',
  claw: 'Connect phone',
  settingsFooter: 'Settings',
  settingsNavPersonal: 'Personal',
  settingsNavIntegrations: 'Integrations',
  settingsNavCoding: 'Coding',
  settingsNavArchived: 'Archived group',
  settingsSearchLabel: 'Search settings',
  settingsSearchPlaceholder: 'Search settings',
  settingsSearchClear: 'Clear search',
  settingsSearchEmpty: 'No matching settings',
  archivesTitle: 'Archived chats',
  archivesOverview: 'Archived chat history',
  archivesOverviewDesc: 'Review archived chats.',
  archivesSearchPlaceholder: 'Search archived chats',
  archivesCount: '{{count}} archived',
  archivesWorkspaceCount: '{{count}} chats',
  archivesEmpty: 'No archived chats yet.',
  archivesSearchEmpty: 'No archived chats match your search.',
  archivesOffline: 'Connect the local runtime to refresh archived chats.',
  archivesUntitled: 'Untitled chat',
  archivesRestore: 'Restore',
  archivesDelete: 'Delete archived chat',
  sidebarThreadRestore: 'Restore thread',
  sidebarThreadDelete: 'Delete thread'
}

function t(key: string, options?: Record<string, unknown>): string {
  const template = labels[key] ?? key
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(options?.[name] ?? ''))
}

let root: Root | null, host: HTMLDivElement
let server: NormalizedThread[], ctx: Record<string, any>
beforeEach(() => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  root = null; host = document.createElement('div'); document.body.append(host)
  server = [thread({ id: 'archived-a', title: 'Archived feature work', archived: true, preview: 'Move archived conversations into settings' })]
  bridge.list.mockReset().mockImplementation(async () => server.filter(item => item.archived))
  bridge.confirm.mockReset().mockResolvedValue(true)
  ctx = { t, tCommon: t, threads: [], runtimeReady: true, locale: 'en-US', refreshThreads: vi.fn(),
    openCode: vi.fn(async () => undefined), selectThread: vi.fn(async () => undefined),
    archiveThread: vi.fn(async (id: string) => { server = server.map(item => item.id === id ? { ...item, archived: false } : item) }),
    deleteThread: vi.fn(async (id: string) => { server = server.filter(item => item.id !== id) }) }
})
afterEach(async () => { if (root) await act(async () => root!.unmount()); host.remove(); vi.unstubAllGlobals() })
async function mount() {
  root ??= createRoot(host)
  await act(async () => root!.render(createElement(ArchivedThreadsSettingsSection, { ctx })))
}
async function click(text: string) {
  const button = [...host.querySelectorAll<HTMLButtonElement>('button')].find(item =>
    !item.disabled && (item.textContent?.includes(text) || item.getAttribute('aria-label') === text))
  expect(button).toBeDefined(); await act(async () => button!.click())
}
function held<T>() {
  let resolve!: (value: T) => void, reject!: (error: Error) => void
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail })
  return { promise, resolve, reject }
}

describe('ArchivedThreadsSettingsSection', () => {
  it('filters archived threads by title, preview, workspace, and model', () => {
    const archived = thread({
      id: 'archived',
      title: 'Plan launch',
      preview: 'Contains release checklist',
      archived: true,
      model: 'deepseek-v4-pro'
    })
    const active = thread({
      id: 'active',
      title: 'Plan launch',
      archived: false
    })

    expect(filterArchivedThreads([archived, active], 'release').map((item) => item.id)).toEqual(['archived'])
    expect(filterArchivedThreads([archived, active], 'deepseek-v4').map((item) => item.id)).toEqual(['archived'])
    expect(filterArchivedThreads([archived, active], 'Plan').map((item) => item.id)).toEqual(['archived'])
  })

  it('loads archived chats with restore and delete actions independently of sidebar search', async () => {
    ctx.threadSearch = 'unrelated sidebar query'
    ctx.threads = [thread({ id: 'unrelated', archived: false })]
    await mount()
    expect(host.textContent).toContain('Archived feature work')
    expect(host.textContent).toContain('Move archived conversations into settings')
    expect(host.textContent).toContain('Restore')
    expect(host.querySelector('[aria-label="Delete archived chat"]')).not.toBeNull()
    expect(bridge.list).toHaveBeenCalledExactlyOnceWith({ archivedOnly: true, limit: 0 })
    expect(ctx.refreshThreads).not.toHaveBeenCalled()
  })

  it('searches all 52 synthetic public summaries locally and reloads after restore and delete', async () => {
    server = Array.from({ length: 52 }, (_, index) => thread({ id: `archive-${index}`, title: `Synthetic archive ${index}`, archived: true }))
    await mount(); expect(host.textContent).toContain('52 archived')
    const input = host.querySelector('input')!
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'archive 51')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(host.textContent).toContain('Synthetic archive 51')
    expect(bridge.list).toHaveBeenCalledTimes(1)
    await click('Restore'); expect(ctx.archiveThread).toHaveBeenCalledWith('archive-51', false)
    expect(host.textContent).not.toContain('Synthetic archive 51'); expect(host.textContent).toContain('51 archived')
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, 'archive 50')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await click('Delete archived chat'); expect(ctx.deleteThread).toHaveBeenCalledWith('archive-50')
    expect(host.textContent).not.toContain('Synthetic archive 50'); expect(host.textContent).toContain('50 archived')
    expect(bridge.list).toHaveBeenCalledTimes(3)
  })
  it.each(['success', 'failure'])('ignores late %s from a retired offline entry', async outcome => {
    const old = held<NormalizedThread[]>(); bridge.list.mockImplementationOnce(() => old.promise)
    await mount(); ctx = { ...ctx, runtimeReady: false }; await mount()
    expect(host.textContent).toContain('Connect the local runtime')
    ctx = { ...ctx, runtimeReady: true }; await mount()
    await act(async () => {
      if (outcome === 'success') old.resolve([thread({ id: 'retired', archived: true })])
      else old.reject(new Error('retired failure'))
    })
    expect(host.textContent).toContain('Archived feature work')
    expect(host.textContent).not.toContain('retired'); expect(host.querySelector('[role="alert"]')).toBeNull()
  })
  it('shows a current load error rather than an empty success', async () => {
    bridge.list.mockRejectedValueOnce(new Error('synthetic archive unavailable')); await mount()
    expect(host.querySelector('[role="alert"]')?.textContent).toBe('synthetic archive unavailable')
    expect(host.textContent).not.toContain('No archived chats yet.')
    expect(host.textContent).not.toContain('0 archived')
  })
  it('keeps the newest local reload when two actions complete out of order', async () => {
    server = [thread({ id: 'one', title: 'One', archived: true }), thread({ id: 'two', title: 'Two', archived: true })]
    await mount(); const old = held<NormalizedThread[]>(); bridge.list.mockImplementationOnce(() => old.promise)
    await click('Restore'); await click('Restore')
    expect(host.textContent).toContain('No archived chats yet.')
    await act(async () => old.resolve([thread({ id: 'two', title: 'Two', archived: true })]))
    expect(host.textContent).not.toContain('Two'); expect(host.textContent).toContain('0 archived')
  })
  it('does not delete after a confirmation belongs to a retired entry', async () => {
    const confirmation = held<boolean>(); bridge.confirm.mockImplementationOnce(() => confirmation.promise)
    await mount(); await click('Delete archived chat')
    ctx = { ...ctx, runtimeReady: false }; await mount(); ctx = { ...ctx, runtimeReady: true }; await mount()
    await act(async () => confirmation.resolve(true))
    expect(ctx.deleteThread).not.toHaveBeenCalled(); expect(bridge.list).toHaveBeenCalledTimes(2)
  })
  it('preserves open then select navigation without reading again after unmount', async () => {
    ctx.openCode = vi.fn(async () => { root!.unmount(); root = null })
    await mount(); await click('Archived feature work')
    expect(ctx.openCode).toHaveBeenCalledTimes(1); expect(ctx.selectThread).toHaveBeenCalledWith('archived-a')
    expect(bridge.list).toHaveBeenCalledTimes(1)
  })

  it('keeps archived chats selected as a bottom utility outside the three navigation groups', () => {
    const html = renderToStaticMarkup(createElement(SettingsSidebar, {
      category: 'archives',
      goBack: () => undefined,
      setCategory: () => undefined,
      t
    }))

    const codingIndex = html.indexOf('Coding')
    const archivesIndex = html.indexOf('Archived chats')
    const navigationEndIndex = html.indexOf('</nav>')
    expect(html).toContain('Search settings')
    expect(html).toContain('Browser')
    expect(html).toContain('Computer control')
    expect(html).toContain('Personal')
    expect(html).toContain('Integrations')
    expect(codingIndex).toBeGreaterThanOrEqual(0)
    expect(html).not.toContain('Archived group')
    expect(navigationEndIndex).toBeGreaterThanOrEqual(0)
    expect(archivesIndex).toBeGreaterThan(navigationEndIndex)
    expect(html.match(/Archived chats/g)).toHaveLength(1)
    expect(html).toMatch(/<button[^>]*aria-current="page"[^>]*>[\s\S]*?Archived chats[\s\S]*?<\/button>/)
  })
})
