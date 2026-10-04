// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from '@shared/app-settings'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { ScheduleTasksView } from './ScheduleTasksView'
import i18n from '../../i18n'

it('keeps a draft through nested defaults, traps top-level Tab and closes one layer with Escape', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  Object.assign(window, { analytix: { schedule: {} } })
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
