// @vitest-environment jsdom
import { act, createElement } from 'react'
import { parse } from 'postcss'
import { createRoot } from 'react-dom/client'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1, type ScheduledTaskV1 } from '@shared/app-settings'
import { buildScheduleTaskUpdate, scheduleTaskEditable, type ScheduleTaskWriteResult } from '@shared/schedule-task-mutation'
import type { ScheduleTaskCatalogSnapshot } from './schedule-task-catalog'
import { ScheduleTaskDetail, ScheduleDeliveryHistory, refreshScheduleTaskDraft, scheduleNextRunDistance } from './ScheduleTaskDetail'
import i18n from '../../i18n'
import { applyTheme } from '../../lib/apply-theme'
import { primaryPaletteCases, resolveThemeToken, themeVariables } from '../chat/presentation/theme-token-test-helpers'

const catalog = vi.hoisted(() => ({
  task: undefined as ScheduledTaskV1 | null | undefined,
  snapshot: { tasks: [], phase: 'ready', readRequest: 1, readSettled: 1 } as ScheduleTaskCatalogSnapshot,
  invalidate: vi.fn(), refresh: vi.fn(async () => {})
}))
vi.mock('./schedule-task-catalog', () => ({
  useCurrentScheduleTask: () => ({ task: catalog.task, snapshot: catalog.snapshot }),
  scheduleTaskCatalog: { invalidate: catalog.invalidate, refresh: catalog.refresh, getSnapshot: () => catalog.snapshot }
}))

function task(id = 'a'): ScheduledTaskV1 {
  return normalizeAppSettings({ schedule: { tasks: [{
    id, title: id.toUpperCase(), prompt: 'Synthetic ' + id, enabled: true,
    schedule: { kind: 'daily', everyMinutes: 60, timeOfDay: '09:00', atTime: '' }
  }] } } as AppSettingsV1).schedule.tasks[0]
}
function deferred<T>() {
  let resolve!: (value: T) => void
  return { promise: new Promise<T>(done => { resolve = done }), resolve: (value: T) => resolve(value) }
}
async function editTitle(host: HTMLElement, value: string) {
  await act(async () => {
    const input = host.querySelector<HTMLInputElement>(`input[aria-label="${i18n.t('scheduleTaskName', { ns: 'common' })}"]`)!
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}
async function clickText(host: HTMLElement, text: string) {
  await act(async () => { [...host.querySelectorAll<HTMLButtonElement>('button')].find(button => button.textContent?.trim() === text)!.click() })
}
beforeEach(async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  await i18n.changeLanguage('en')
  catalog.task = task(); catalog.snapshot = { tasks: [catalog.task], phase: 'ready', readRequest: 1, readSettled: 1 }
  catalog.invalidate.mockClear(); catalog.refresh.mockClear()
})

describe('Schedule task detail staged edits', () => {
  it('keeps a real legacy title save local, then upgrades only a real timing edit', async () => {
    const baseline = catalog.task!, updateTask = vi.fn(async (request: ReturnType<typeof buildScheduleTaskUpdate>) => ({ kind: 'success', task: { ...baseline, ...request.patch } }))
    Object.assign(window, { analytix: { schedule: { updateTask } } })
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: vi.fn() })))
      await editTitle(host, 'Renamed'); await clickText(host, i18n.t('save', { ns: 'common' }))
      expect(updateTask.mock.calls[0][0].patch).toEqual({ title: 'Renamed' })
      expect(Object.hasOwn(baseline.schedule, 'timeZone')).toBe(false)
      await act(async () => {
        const input = host.querySelector<HTMLInputElement>('input[type="time"]')!
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, '10:00')
        input.dispatchEvent(new Event('input', { bubbles: true }))
      })
      await clickText(host, i18n.t('save', { ns: 'common' }))
      const request = updateTask.mock.calls[1][0]
      expect(request.patch.schedule?.timeOfDay).toBe('10:00')
      expect(request.patch.schedule?.timeZone).toBeTruthy()
      expect(request.expected.schedule).toEqual(baseline.schedule)
      expect(request.expected.enabled).toBe(true)
    } finally { await act(async () => root.unmount()) }
  })

  it.each(primaryPaletteCases)('resolves the body-portal $theme $label palette and keeps close-button chrome independent', async ({ theme, accent, fill, foreground, hover, pressed }) => {
    const fs = await vi.importActual<{ readFileSync: (path: string, encoding: 'utf8') => string }>('node:fs')
    const readCss = (relative: string) => fs.readFileSync(decodeURIComponent(new URL(relative, import.meta.url).pathname), 'utf8')
    const detail = parse(readCss('./ScheduleTaskDetail.module.css'))
    const shell = parse(readCss('../../styles/base-shell.css'))
    const tokens = parse(readCss('../chat/presentation/tokens.css'))
    const values = (property: string, dark = false) => {
      const found: string[] = []
      detail.walkRules(rule => {
        if (rule.selector.includes("[data-theme='dark']") === dark) rule.walkDecls(property, declaration => { found.push(declaration.value) })
      })
      return found
    }
    expect(values('--dsw-radius-panel')).toEqual(['28px'])
    expect(values('--dsw-alias-bg-mask-1')).toEqual(['rgba(0, 0, 0, 0.24)'])
    expect(values('--dsw-alias-bg-mask-1', true)).toEqual(['rgba(0, 0, 0, 0.5)'])
    expect(values('--dsw-alias-bg-layer-2')).toEqual(['rgb(255, 255, 255)'])
    expect(values('--dsw-alias-bg-layer-2', true)).toEqual(['rgb(44, 44, 46)'])
    // Source seam for the real confirmation's body-portal origin, not native layout.
    const portal = document.createElement('div')
    portal.setAttribute('data-analytix-chat-view', 'true'); document.body.append(portal)
    const previous = new Map(['style', 'data-theme', 'data-accent'].map(name => [name, document.documentElement.getAttribute(name)]))
    try {
      applyTheme(theme, accent)
      const inherited = themeVariables(portal, [shell, tokens])
      const localFill = values('--dsw-alias-button-primary-fill', theme === 'dark')
      const localHover = values('--dsw-alias-button-primary-hover', theme === 'dark')
      expect(localFill.map(value => resolveThemeToken(value, inherited))).toEqual([fill])
      expect(localHover.map(value => resolveThemeToken(value, inherited))).toEqual([hover])
      inherited.set('--dsw-alias-button-primary-fill', localFill[0])
      inherited.set('--dsw-alias-button-primary-hover', localHover[0])
      const paint = (selector: string, property: string) => {
        let value: string | undefined
        detail.walkRules(selector, rule => { rule.walkDecls(property, declaration => { value = declaration.value }) })
        return resolveThemeToken(value, inherited)
      }
      expect(portal.parentElement).toBe(document.body)
      expect(paint('.footer .saveButton', 'background')).toBe(fill)
      expect(paint('.footer .saveButton', 'color')).toBe(foreground)
      expect(paint('.footer .saveButton:hover:not(:disabled)', 'background')).toBe(hover)
      expect(paint('.footer .saveButton:active:not(:disabled)', 'background')).toBe(pressed)
      expect(resolveThemeToken(inherited.get('--dsw-alias-label-primary'), inherited)).toBe(theme === 'dark' ? 'rgb(249, 250, 251)' : 'rgb(15, 17, 21)')
    } finally {
      portal.remove()
      for (const [name, value] of previous) {
        if (value === null) document.documentElement.removeAttribute(name)
        else document.documentElement.setAttribute(name, value)
      }
    }
    expect(values('--dsw-mask-blur')).toEqual(['none'])
    expect(values('--dsw-elevation-prominent')).toEqual(['var(--dsw-elevation-stroke), 0 3px 8px 0 rgba(0, 0, 0, 0.04), 0 0 20px 0 rgba(0, 0, 0, 0.05)'])
    detail.walkRules(rule => { expect(rule.selector).not.toMatch(/(?:^|,)\s*\.confirm\s+button(?:\s|,|$)/) })
    const scopedFont: string[] = [], scopedColors: string[] = []
    tokens.walkRules(rule => {
      if (!rule.selector.includes('[data-analytix-chat-view]')) return
      rule.walkDecls('--dsw-font-family', declaration => { scopedFont.push(declaration.value) })
      rule.walkDecls('--dsw-alias-label-primary', declaration => { scopedColors.push(declaration.value) })
    })
    expect(scopedFont.some(font => font.includes("'Hiragino Sans GB'") && font.includes("'Helvetica Neue'"))).toBe(true)
    expect(scopedColors).toEqual(['rgb(15, 17, 21)', 'rgb(249, 250, 251)'])
  })

  it('formats the next-run countdown without using it as a catalog freshness signal', () => {
    const now = Date.parse('2099-01-01T00:00:00Z')
    expect(scheduleNextRunDistance('2099-01-01T00:59:00Z', now, 'en')).toBe('in 59 minutes')
    expect(scheduleNextRunDistance('2099-01-01T01:01:00Z', now, 'en')).toBe('in 2 hours')
    expect(scheduleNextRunDistance('2098-12-31T23:00:00Z', now, 'en')).toBe('1 hour ago')
    expect(scheduleNextRunDistance('bad target', now, 'en')).toBeNull()
  })
  it('refreshes unedited fields while retaining original CAS companions without making them new edits', () => {
    const baseline = task(), draft = scheduleTaskEditable(baseline)
    draft.schedule.timeOfDay = '10:00'
    const fresh = { ...baseline, enabled: false, prompt: 'Host changed prompt', lastRunAt: '2099-01-01T00:00:00.000Z' }
    const edit = refreshScheduleTaskDraft({ baseline, draft }, fresh)
    expect(edit.draft.prompt).toBe(fresh.prompt)
    expect(edit.baseline.lastRunAt).toBe(fresh.lastRunAt)
    expect(buildScheduleTaskUpdate(edit.baseline, edit.draft)).toEqual({
      taskId: 'a', patch: { schedule: { ...baseline.schedule, timeOfDay: '10:00' } },
      expected: { schedule: baseline.schedule, enabled: true }
    })
  })

  it('saves only a changed title and retains the draft on a task conflict', async () => {
    const updateTask = vi.fn(async (): Promise<ScheduleTaskWriteResult> => ({ kind: 'conflict', message: 'Synthetic task conflict' }))
    Object.assign(window, { analytix: { schedule: { updateTask } } })
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: vi.fn() })))
      await editTitle(host, 'Edited title')
      expect(host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('scheduleRunNow', { ns: 'common' })}"]`)!.disabled).toBe(true)
      await clickText(host, i18n.t('save', { ns: 'common' }))
      expect(updateTask).toHaveBeenCalledWith({ taskId: 'a', patch: { title: 'Edited title' }, expected: { title: 'A' } })
      expect(host.querySelector<HTMLInputElement>('input')!.value).toBe('Edited title')
      expect(host.querySelector('[role="alert"]')?.textContent).toBe('Synthetic task conflict')
      expect(catalog.invalidate).not.toHaveBeenCalled()
    } finally { await act(async () => root.unmount()) }
  })

  it.each(['success', 'conflict'] as const)('isolates an A response after the task identity changes to B (%s)', async kind => {
    const pending = deferred<ScheduleTaskWriteResult>(), updateTask = vi.fn(() => pending.promise), close = vi.fn()
    Object.assign(window, { analytix: { schedule: { updateTask } } })
    const host = document.createElement('div'), root = createRoot(host)
    const draw = (id: string) => act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: id, onClose: close, onOpenThread: vi.fn() })))
    try {
      await draw('a'); await editTitle(host, 'Edited A'); await clickText(host, i18n.t('save', { ns: 'common' }))
      catalog.task = task('b'); await draw('b')
      await act(async () => pending.resolve(kind === 'success' ? { kind, task: { ...task('a'), title: 'Edited A' } }
        : { kind, message: 'Old A conflict' }))
      expect(host.querySelector<HTMLInputElement>('input')!.value).toBe('B')
      expect(host.textContent).not.toContain('Old A conflict')
      expect(host.querySelector('footer')).toBeNull()
      expect(close).not.toHaveBeenCalled()
      expect(catalog.invalidate).toHaveBeenCalledTimes(kind === 'success' ? 1 : 0)
    } finally { await act(async () => root.unmount()) }
  })

  it('uses atomic deletion and waits for a fresh missing catalog before closing', async () => {
    const current = catalog.task!, deleteTask = vi.fn(async () => ({ kind: 'success', taskId: 'a' })), close = vi.fn()
    Object.assign(window, { analytix: { schedule: { deleteTask } } })
    const host = document.createElement('div'), root = createRoot(host)
    document.body.append(host)
    try {
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: close, onOpenThread: vi.fn() })))
      expect(host.querySelector('[data-schedule-task-detail]')?.hasAttribute('data-analytix-chat-view')).toBe(true)
      await act(async () => host.querySelector<HTMLButtonElement>('button[aria-haspopup="menu"]')!.click())
      expect(document.activeElement?.getAttribute('role')).toBe('menuitem')
      await act(async () => host.querySelector<HTMLButtonElement>('[role="menuitem"]')!.click())
      const dialog = document.querySelector<HTMLElement>('[role="alertdialog"]')!
      expect(dialog.parentElement?.parentElement).toBe(document.body)
      expect(dialog.parentElement?.hasAttribute('data-analytix-chat-view')).toBe(true)
      expect(dialog.contains(document.activeElement)).toBe(true)
      await clickText(dialog, i18n.t('delete', { ns: 'common' }))
      expect(deleteTask).toHaveBeenCalledWith({ taskId: 'a', expected: scheduleTaskEditable(current) })
      expect(close).not.toHaveBeenCalled()
      expect(catalog.invalidate).toHaveBeenCalledTimes(1)
      expect(catalog.refresh).toHaveBeenCalledTimes(1)
      catalog.task = null; catalog.snapshot = { ...catalog.snapshot, phase: 'ready', tasks: [], readRequest: 2, readSettled: 2 }
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: close, onOpenThread: vi.fn() })))
      expect(close).toHaveBeenCalledTimes(1)
    } finally { await act(async () => root.unmount()); host.remove() }
  })

  it('disables task mutations while a retained task is refreshing and has no actions for confirmed deletion', async () => {
    const runTask = vi.fn(), updateTask = vi.fn(), deleteTask = vi.fn()
    Object.assign(window, { analytix: { schedule: { runTask, updateTask, deleteTask } } })
    const host = document.createElement('div'), root = createRoot(host)
    const draw = () => act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: vi.fn() })))
    try {
      catalog.snapshot = { ...catalog.snapshot, phase: 'loading' }; await draw()
      expect(host.querySelector<HTMLInputElement>('input')!.disabled).toBe(true)
      await act(async () => host.querySelector<HTMLButtonElement>(`button[aria-label="${i18n.t('scheduleRunNow', { ns: 'common' })}"]`)!.click())
      expect(runTask).not.toHaveBeenCalled()
      catalog.task = null; catalog.snapshot = { ...catalog.snapshot, phase: 'ready', tasks: [] }; await draw()
      expect(host.textContent).toContain('Task unavailable. It may have been deleted.')
      expect(host.querySelector('input')).toBeNull()
      expect(updateTask).not.toHaveBeenCalled(); expect(deleteTask).not.toHaveBeenCalled()
    } finally { await act(async () => root.unmount()) }
  })
})

describe('Schedule actual Records tab', () => {
  it('loads only after Records is selected and keeps actual original and last-session links separate', async () => {
    catalog.task = { ...task(), originalThreadId: 'real-original', lastThreadId: 'real-last', deliveryKind: 'origin-delivery' }
    const readHistory = vi.fn(async () => ({ kind: 'success', records: [], deliveryKind: 'origin-delivery' })), open = vi.fn()
    Object.assign(window, { analytix: { schedule: { readHistory } } })
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: open })))
      expect(readHistory).not.toHaveBeenCalled()
      await clickText(host, i18n.t('scheduleOpenOriginalThread', { ns: 'common' })); await clickText(host, i18n.t('scheduleOpenLastThread', { ns: 'common' }))
      expect(open.mock.calls.map(call => call[0])).toEqual(['real-original', 'real-last'])
      await clickText(host, i18n.t('scheduleHistory', { ns: 'common' }))
      expect(readHistory).toHaveBeenCalledTimes(1)
      expect(readHistory).toHaveBeenCalledWith({ taskId: 'a', limit: 20 })
      expect(host.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toBe(i18n.t('scheduleHistory', { ns: 'common' }))
    } finally { await act(async () => root.unmount()) }
  })
  it('preserves Host page order and saved prompt, and retires a response after unmount', async () => {
    const late = deferred<{ kind: 'success'; records: []; deliveryKind: 'origin-delivery' }>()
    const record = { attemptId: 'ack-2', scheduledAt: '2026-10-11T10:00:00.000Z', deliveredAt: '2026-10-11T10:00:01.000Z', messageId: 'actual-message', prompt: 'IMMUTABLE_ACK_PROMPT' }
    const readHistory = vi.fn().mockResolvedValueOnce({ kind: 'success', records: [record], nextBefore: 'ack-2', deliveryKind: 'origin-delivery' }).mockImplementationOnce(() => late.promise)
    Object.assign(window, { analytix: { schedule: { readHistory } } })
    const host = document.createElement('div'), root = createRoot(host)
    await act(async () => root.render(createElement(ScheduleDeliveryHistory, { taskId: 'a' })))
    expect(host.textContent).toContain('IMMUTABLE_ACK_PROMPT')
    await clickText(host, i18n.t('scheduleHistoryLoadMore', { ns: 'common' }))
    expect(readHistory).toHaveBeenLastCalledWith({ taskId: 'a', limit: 20, before: 'ack-2' })
    await act(async () => root.unmount())
    await act(async () => late.resolve({ kind: 'success', records: [], deliveryKind: 'origin-delivery' }))
    expect(host.textContent).toBe('')
  })
})


describe('Schedule Records refresh and authoritative History semantics', () => {
  const record = (attemptId: string) => ({ attemptId, scheduledAt: '2026-10-11T10:00:00Z', deliveredAt: '2026-10-11T10:00:01Z', messageId: 'message-' + attemptId, prompt: 'SAVED_' + attemptId })
  it('retires a pending older page on refresh/latest ACK, preserving new Host order without duplicates', async () => {
    const late = deferred<{ kind: 'success'; records: ReturnType<typeof record>[]; deliveryKind: 'origin-delivery' }>()
    const page = (records: ReturnType<typeof record>[], nextBefore?: string) => ({ kind: 'success' as const, records, deliveryKind: 'origin-delivery' as const, ...(nextBefore ? { nextBefore } : {}) })
    const readHistory = vi.fn().mockResolvedValueOnce(page([record('new'), record('middle')], 'middle'))
      .mockImplementationOnce(() => late.promise).mockResolvedValueOnce(page([record('latest'), record('new')], 'new'))
      .mockResolvedValueOnce(page([record('new'), record('older')]))
    Object.assign(window, { analytix: { schedule: { readHistory } } })
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(ScheduleDeliveryHistory, { taskId: 'a', latestAttemptId: 'new' })))
      await clickText(host, i18n.t('scheduleHistoryLoadMore', { ns: 'common' }))
      await act(async () => root.render(createElement(ScheduleDeliveryHistory, { taskId: 'a', latestAttemptId: 'latest' })))
      await act(async () => late.resolve(page([record('STALE')])))
      expect(host.textContent).not.toContain('SAVED_STALE')
      await clickText(host, i18n.t('scheduleHistoryLoadMore', { ns: 'common' }))
      expect([...host.querySelectorAll('[aria-label="' + i18n.t('scheduleHistoryRecord', { ns: 'common' }) + '"]')].map(node => node.textContent?.match(/SAVED_[a-z]+/)?.[0])).toEqual(['SAVED_latest', 'SAVED_new', 'SAVED_older'])
      expect(host.textContent).toContain(i18n.t('scheduleHistoryDeliveredAt', { ns: 'common' }))
      expect(host.textContent).not.toContain('Completed')
    } finally { await act(async () => root.unmount()) }
  })
  it('never derives an original session from lastThreadId, and current prompt edits cannot rewrite saved History', async () => {
    catalog.task = { ...task(), lastThreadId: 'legacy-last', deliveryKind: 'legacy-headless' }
    const readHistory = vi.fn(async () => ({ kind: 'success', records: [record('actual')], deliveryKind: 'origin-delivery' })), open = vi.fn()
    Object.assign(window, { analytix: { schedule: { readHistory } } })
    const host = document.createElement('div'), root = createRoot(host)
    try {
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: open })))
      expect(host.textContent).not.toContain(i18n.t('scheduleOpenOriginalThread', { ns: 'common' }))
      await clickText(host, i18n.t('scheduleHistory', { ns: 'common' }))
      catalog.task = { ...catalog.task!, prompt: 'NEW_CURRENT_PROMPT' }
      await act(async () => root.render(createElement(ScheduleTaskDetail, { taskId: 'a', onClose: vi.fn(), onOpenThread: open })))
      const history = host.querySelector('[id^="schedule-history-"]')!
      expect(history.textContent).toContain('SAVED_actual'); expect(history.textContent).not.toContain('NEW_CURRENT_PROMPT')
      expect(open).not.toHaveBeenCalled()
    } finally { await act(async () => root.unmount()) }
  })
})
