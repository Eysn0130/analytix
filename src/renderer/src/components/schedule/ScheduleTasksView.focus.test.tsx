// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { ScheduleTasksView } from './ScheduleTasksView'
import i18n from '../../i18n'

it('opens an existing task through the real detail callback from its title and edit action', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const settings = normalizeAppSettings({ schedule: { tasks: [{ id: 'task', title: 'Task title', prompt: 'Synthetic prompt' }] } } as AppSettingsV1)
  Object.assign(window, { analytix: { schedule: { listTasks: vi.fn(async () => ({ kind: 'success', tasks: settings.schedule.tasks })) } } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings)
  const open = vi.fn(), host = document.createElement('div'), root = createRoot(host)
  try {
    await act(async () => root.render(createElement(ScheduleTasksView, { leftSidebarCollapsed: false, onOpenTaskDetail: open })))
    await act(async () => {
      host.querySelector<HTMLButtonElement>('h2 button')!.click()
      host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('scheduleEditTask', { ns: 'common' })}"]`)!.click()
    })
    expect(open.mock.calls).toEqual([['task', 'Task title'], ['task', 'Task title']])
    expect(host.querySelector('[role="dialog"]')).toBeNull()
  } finally { await act(async () => root.unmount()); vi.restoreAllMocks() }
})

it('keeps a draft through nested defaults, traps top-level Tab and closes one layer with Escape', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  Object.assign(window, { analytix: { schedule: { listTasks: vi.fn(async () => ({ kind: 'success', tasks: [] })) } } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(normalizeAppSettings({ workspaceRoot: '/synthetic/a' } as AppSettingsV1))
  const save = vi.spyOn(rendererRuntimeClient, 'setSettings')
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const t = (key: string) => i18n.t(key, { ns: 'common' })
  const click = async (label: string) => act(async () => { const b = [...host.querySelectorAll('button')].find(b => b.textContent?.trim() === label)!; b.focus(); b.click() })
  const key = async (value: string, shiftKey = false) => act(async () => { document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: value, shiftKey, bubbles: true, cancelable: true })) })
  try {
    await act(async () => root.render(createElement(ScheduleTasksView, { leftSidebarCollapsed: false, onOpenThread: vi.fn() })))
    await click(t('scheduleNewTask'))
    const title = host.querySelector<HTMLInputElement>('input[maxlength="50"]')!; expect(document.activeElement).toBe(title)
    await act(async () => { Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(title, 'Synthetic retained draft'); title.dispatchEvent(new Event('input', { bubbles: true })) })
    expect(document.activeElement).toBe(title)
    await click(t('scheduleAdvancedSettings'))
    const layers = host.querySelectorAll<HTMLElement>('[role="dialog"]'); expect(layers).toHaveLength(2)
    const nested = layers[1]; expect(nested.contains(document.activeElement)).toBe(true)
    const fields = [...nested.querySelectorAll<HTMLElement>('button,input,textarea,select')].filter(e => !e.matches(':disabled'))
    fields.at(-1)!.focus(); await key('Tab'); expect(document.activeElement).toBe(fields[0])
    await key('Tab', true); expect(document.activeElement).toBe(fields.at(-1))
    await key('Escape'); expect(host.querySelectorAll('[role="dialog"]')).toHaveLength(1)
    expect(title.value).toBe('Synthetic retained draft'); expect(document.activeElement?.textContent?.trim()).toBe(t('scheduleAdvancedSettings'))
    await key('Escape'); expect(host.querySelectorAll('[role="dialog"]')).toHaveLength(0)
    expect(document.activeElement?.textContent?.trim()).toBe(t('scheduleNewTask')); expect(save).not.toHaveBeenCalled()
  } finally { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks() }
})

it.each(['success', 'error'] as const)('isolates an old save response from a reopened task dialog (%s)', async kind => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const settings = normalizeAppSettings({ workspaceRoot: '/synthetic/a', schedule: { tasks: [
    { id: 'a', title: 'A', prompt: 'Synthetic A' }, { id: 'b', title: 'B', prompt: 'Synthetic B' }
  ] } } as AppSettingsV1)
  let finish!: (value: unknown) => void
  const pending = new Promise<unknown>(resolve => { finish = resolve })
  const updateTask = vi.fn(() => pending)
  Object.assign(window, { analytix: { schedule: { listTasks: vi.fn(async () => ({ kind: 'success', tasks: settings.schedule.tasks })), updateTask } } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  const edit = async (index: number) => act(async () => { host.querySelectorAll<HTMLButtonElement>(`button[aria-label="${i18n.t('scheduleEditTask', { ns: 'common' })}"]`)[index].click() })
  const escape = async () => act(async () => { document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })) })
  try {
    await act(async () => root.render(createElement(ScheduleTasksView, { leftSidebarCollapsed: false })))
    await edit(0)
    await act(async () => { host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })) })
    expect(updateTask).toHaveBeenCalledTimes(1)
    await escape(); await edit(1); await escape(); await edit(0)
    await act(async () => { finish(kind === 'success' ? { kind, task: settings.schedule.tasks[0] } : { kind, message: 'Old failure must not appear' }) })
    expect(host.querySelectorAll('[role="dialog"]')).toHaveLength(1)
    expect(host.querySelector<HTMLInputElement>('input[maxlength="50"]')!.value).toBe('A')
    expect(host.textContent).not.toContain('Old failure must not appear')
  } finally { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks() }
})

it('submits a New task only once while the Host create is pending', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const settings = normalizeAppSettings({ workspaceRoot: '/synthetic/a' } as AppSettingsV1)
  let finish!: (value: unknown) => void
  const pending = new Promise<unknown>(resolve => { finish = resolve })
  const createTask = vi.fn(() => pending)
  Object.assign(window, { analytix: { schedule: { listTasks: vi.fn(async () => ({ kind: 'success', tasks: [] })), createTask } } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  try {
    await act(async () => root.render(createElement(ScheduleTasksView, { leftSidebarCollapsed: false })))
    await act(async () => { [...host.querySelectorAll('button')].find(button => button.textContent?.trim() === i18n.t('scheduleNewTask', { ns: 'common' }))!.click() })
    await act(async () => {
      const title = host.querySelector<HTMLInputElement>('input[maxlength="50"]')!
      const prompt = host.querySelector<HTMLTextAreaElement>('textarea')!
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(title, 'Synthetic task')
      title.dispatchEvent(new Event('input', { bubbles: true }))
      Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(prompt, 'Synthetic prompt')
      prompt.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => {
      const form = host.querySelector('form')!
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })
    expect(createTask).toHaveBeenCalledTimes(1)
    expect(host.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(true)
    await act(async () => { finish({ kind: 'error', message: 'Synthetic create failure' }) })
    expect(host.querySelector<HTMLButtonElement>('button[type="submit"]')!.disabled).toBe(false)
  } finally { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks() }
})

it('saves only an explicit title edit without guessing missing provider, channel or workspace configuration', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  const settings = normalizeAppSettings({ workspaceRoot: '/synthetic/default', schedule: { tasks: [{
    id: 'host-task', title: 'Before', prompt: 'Synthetic prompt', workspaceRoot: '',
    providerId: 'unknown-provider', model: 'unknown-model', clawChannelId: 'removed-channel',
    nextRunAt: '2099-01-01T00:00:00.000Z'
  }] } } as AppSettingsV1)
  const updateTask = vi.fn(async () => ({ kind: 'conflict', message: 'Synthetic conflict' }))
  Object.assign(window, { analytix: { schedule: { listTasks: vi.fn(async () => ({ kind: 'success', tasks: settings.schedule.tasks })), updateTask } } })
  vi.spyOn(rendererRuntimeClient, 'getSettings').mockResolvedValue(settings)
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host)
  try {
    await act(async () => root.render(createElement(ScheduleTasksView, { leftSidebarCollapsed: false })))
    await act(async () => { host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('scheduleEditTask', { ns: 'common' })}"]`)!.click() })
    await act(async () => {
      const title = host.querySelector<HTMLInputElement>('input[maxlength="50"]')!
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(title, 'After')
      title.dispatchEvent(new Event('input', { bubbles: true }))
    })
    await act(async () => { host.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })) })
    expect(updateTask).toHaveBeenCalledWith({ taskId: 'host-task', patch: { title: 'After' }, expected: { title: 'Before' } })
  } finally { await act(async () => root.unmount()); host.remove(); vi.restoreAllMocks() }
})
