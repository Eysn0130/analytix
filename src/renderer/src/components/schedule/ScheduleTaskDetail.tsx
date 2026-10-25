/* Selected TaskDetail layout and staged-edit behavior adapted from pinned DSH 5badb150.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { ScheduledTaskV1 } from '@shared/app-settings'
import { buildScheduleTaskUpdate, scheduleTaskEditable, scheduleTaskExpectedFields,
  type ScheduleTaskEditable, type ScheduleTaskEditPatch, type ScheduleTaskHistoryResult, type ScheduleTaskDeliveryRecord } from '@shared/schedule-task-mutation'
import { DshClockOutlineRegular, DshChevronDownOutlineRegular, DshChevronUpOutlineRegular, DshCloseOutlineRegular, DshEllipsisOutlineRegular, DshPlayOutlineRegular } from '../chat/presentation/DshToolIcons'
import { useModalFocus } from '../../hooks/use-modal-focus'
import { useWorkspaceTabsStore } from '../../store/workspace-tabs-store'
import { scheduleTaskWorkspaceTabId } from '../workbench-schedule-controller'
import { scheduleTaskCatalog, useCurrentScheduleTask } from './schedule-task-catalog'
import { editScheduleTiming, validateScheduledTaskDraft } from './ScheduleTasksView'
import { isValidScheduleTimeZone } from '../../../../../packages/runtime/src/contracts/schedule-rule'
import css from './ScheduleTaskDetail.module.css'

export type ScheduleTaskDetailDraft = { baseline: ScheduledTaskV1; draft: ScheduleTaskEditable }

/** Display countdown only. Catalog freshness never uses this device clock. */
export function scheduleNextRunDistance(at: string, now: number, locale: string): string | null {
  const difference = Date.parse(at) - now
  if (!Number.isFinite(difference)) return null
  if (difference === 0) return new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(0, 'second')
  const seconds = Math.abs(difference) / 1000
  const units = [{ unit: 'day', seconds: 86400 }, { unit: 'hour', seconds: 3600 }, { unit: 'minute', seconds: 60 }, { unit: 'second', seconds: 1 }] as const
  const selected = units.find(unit => seconds >= unit.seconds) ?? units[3]
  const value = Math.max(1, difference > 0 ? Math.ceil(seconds / selected.seconds) : Math.floor(seconds / selected.seconds))
  return new Intl.RelativeTimeFormat(locale, { numeric: 'always' }).format(difference > 0 ? value : -value, selected.unit)
}

/** Refresh unedited fields without changing the expected values of an edit or its companions. */
export function refreshScheduleTaskDraft(edit: ScheduleTaskDetailDraft, current: ScheduledTaskV1): ScheduleTaskDetailDraft {
  const { patch } = buildScheduleTaskUpdate(edit.baseline, edit.draft)
  const expected = new Set(scheduleTaskExpectedFields(patch))
  const baseline = { ...current }, draft = scheduleTaskEditable(current)
  for (const field of expected) {
    Object.assign(baseline, { [field]: edit.baseline[field] })
    Object.assign(draft, { [field]: edit.draft[field] })
  }
  return { baseline, draft }
}

export function ScheduleTaskDetail({ taskId, onClose, onOpenThread }: {
  taskId: string; onClose: () => void; onOpenThread: (threadId: string) => void
}) {
  const { t, i18n } = useTranslation('common')
  const { task, snapshot } = useCurrentScheduleTask(taskId)
  const [selectedTab, setSelectedTab] = useState<'rule' | 'history'>('rule')
  const ruleTab = useRef<HTMLButtonElement>(null), historyTab = useRef<HTMLButtonElement>(null)
  const [edit, setEdit] = useState<ScheduleTaskDetailDraft | null>(null)
  const [pending, setPending] = useState(false), [message, setMessage] = useState('')
  const [menuOpen, setMenuOpen] = useState(false), [confirming, setConfirming] = useState(false)
  const [deletionConfirmed, setDeletionConfirmed] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const request = useRef(0), busy = useRef(false), identity = useRef(taskId), more = useRef<HTMLButtonElement>(null)
  const menuAnchor = useRef<HTMLDivElement>(null), deleteItem = useRef<HTMLButtonElement>(null), panel = useRef<HTMLElement>(null)
  identity.current = taskId
  useEffect(() => { const clock = setInterval(() => setNow(Date.now()), 30_000); return () => clearInterval(clock) }, [])
  useEffect(() => {
    request.current += 1; busy.current = false; setSelectedTab('rule'); setEdit(null); setPending(false); setMessage(''); setMenuOpen(false); setConfirming(false); setDeletionConfirmed(false)
    return () => { request.current += 1 }
  }, [taskId])
  useEffect(() => { panel.current?.focus({ preventScroll: true }) }, [taskId, task?.id])
  useEffect(() => {
    if (!task || snapshot.phase !== 'ready') return
    setEdit(previous => previous ? refreshScheduleTaskDraft(previous, task) : null)
    useWorkspaceTabsStore.getState().patchTab(scheduleTaskWorkspaceTabId(taskId), { title: task.title })
  }, [taskId, task, snapshot.phase])
  useEffect(() => {
    if (deletionConfirmed && task === null && snapshot.phase === 'ready') onClose()
  }, [deletionConfirmed, task, snapshot.phase, onClose])
  useEffect(() => {
    if (!menuOpen) return
    deleteItem.current?.focus()
    const closeOutside = (event: PointerEvent) => {
      if (event.target instanceof Node && !menuAnchor.current?.contains(event.target)) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', closeOutside)
    return () => document.removeEventListener('pointerdown', closeOutside)
  }, [menuOpen])
  const ready = Boolean(task && snapshot.phase === 'ready')
  const disabled = pending || !ready || deletionConfirmed
  const shown = task ? { ...task, ...(edit?.draft ?? {}) } : null
  const dirty = Boolean(edit && Object.keys(buildScheduleTaskUpdate(edit.baseline, edit.draft).patch).length)
  const change = (patch: ScheduleTaskEditPatch) => {
    if (!task || disabled || busy.current) return
    setMessage('')
    setEdit(previous => {
      const next = previous ?? { baseline: task, draft: scheduleTaskEditable(task) }
      return { ...next, draft: { ...next.draft, ...patch, schedule: patch.schedule ? editScheduleTiming(next.draft.schedule, patch.schedule) : next.draft.schedule } }
    })
  }
  const current = (ticket: number) => identity.current === taskId && request.current === ticket
  const perform = async (operation: 'save' | 'delete' | 'run') => {
    if (!task || disabled || busy.current) return
    if (operation === 'run' && dirty) return
    if (operation === 'save') {
      if (!edit || !dirty || !shown) return
      const validation = validateScheduledTaskDraft(shown, t)
      if (validation) { setMessage(validation); return }
    }
    const ticket = ++request.current
    busy.current = true
    setPending(true); setMessage('')
    try {
      const api = window.analytix.schedule
      if (operation === 'run') {
        const result = await api.runTask(taskId)
        if (current(ticket) && !result.ok) setMessage(t('scheduleStatus_error'))
      } else if (operation === 'delete') {
        const result = await api.deleteTask({ taskId, expected: scheduleTaskEditable(task) })
        if (result.kind === 'success') scheduleTaskCatalog.invalidate()
        if (!current(ticket)) return
        if (result.kind !== 'success') { setMessage(result.message); return }
        setDeletionConfirmed(true)
      } else {
        const result = await api.updateTask(buildScheduleTaskUpdate(edit!.baseline, edit!.draft))
        if (result.kind === 'success') scheduleTaskCatalog.invalidate()
        if (!current(ticket)) return
        if (result.kind !== 'success') { setMessage(result.message); return }
        setEdit({ baseline: result.task, draft: scheduleTaskEditable(result.task) })
      }
      if (operation === 'run') scheduleTaskCatalog.invalidate()
      await scheduleTaskCatalog.refresh()
      if (current(ticket) && operation === 'save' && scheduleTaskCatalog.getSnapshot().phase === 'ready') setEdit(null)
    } catch {
      if (current(ticket)) setMessage(t('scheduleStatus_error'))
    } finally {
      if (current(ticket)) { busy.current = false; setPending(false); setConfirming(false) }
    }
  }
  const feedback = snapshot.phase === 'error' || snapshot.phase === 'unavailable'
  if (!shown) return <div className={css.empty} data-analytix-chat-view role="status">
    <p>{task === null ? t('dshSchedule.detailMissing', { defaultValue: 'Task unavailable. It may have been deleted.' })
      : feedback ? t('scheduleStatus_error') : t('loading')}</p>
    {feedback ? <button type="button" onClick={() => void scheduleTaskCatalog.refresh()}>{t('retryConnection')}</button> : null}
  </div>
  return <aside ref={panel} tabIndex={-1} className={css.detail} data-analytix-chat-view data-schedule-task-detail data-task-id={taskId} aria-label={t('schedule')}>
    <div className={css.tabsBar}>
      <div className={css.tabs} role="tablist" aria-label={t('schedule')}>
        {(['rule', 'history'] as const).map(tab => <button key={tab} ref={tab === 'rule' ? ruleTab : historyTab}
          type="button" role="tab" id={`schedule-tab-${tab}-${encodeURIComponent(taskId)}`} aria-selected={selectedTab === tab}
          aria-controls={`schedule-${tab}-${encodeURIComponent(taskId)}`} tabIndex={selectedTab === tab ? 0 : -1} className={css.tab}
          onClick={() => setSelectedTab(tab)} onKeyDown={event => {
            if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
            event.preventDefault()
            const next = event.key === 'Home' ? 'rule' : event.key === 'End' ? 'history' : tab === 'rule' ? 'history' : 'rule'
            setSelectedTab(next); (next === 'rule' ? ruleTab : historyTab).current?.focus()
          }}>{tab === 'rule' ? t('dshSchedule.rule', { defaultValue: 'Rules' }) : t('scheduleHistory')}</button>)}
      </div>
      <div className={css.actions}>
        <button type="button" className={css.iconButton} disabled={disabled || dirty} onClick={() => void perform('run')} aria-label={t('scheduleRunNow')} title={t('scheduleRunNow')}><DshPlayOutlineRegular /></button>
        <div ref={menuAnchor} className={css.menuAnchor} onKeyDown={event => {
          if (event.key === 'Escape' && menuOpen) { event.stopPropagation(); setMenuOpen(false); more.current?.focus() }
        }}>
          <button type="button" ref={more} className={css.iconButton} disabled={disabled} aria-label={t('dshSchedule.more', { defaultValue: 'More' })}
            aria-haspopup="menu" aria-expanded={menuOpen} onClick={() => setMenuOpen(value => !value)}><DshEllipsisOutlineRegular /></button>
          {menuOpen ? <div className={css.menu} role="menu"><button ref={deleteItem} type="button" role="menuitem" disabled={disabled} onClick={() => { setMenuOpen(false); setConfirming(true) }}>{t('delete')}</button></div> : null}
        </div>
        <button type="button" className={css.iconButton} aria-label={t('close')} onClick={onClose}><DshCloseOutlineRegular /></button>
      </div>
    </div>
    <div className={css.scroll}>
      {snapshot.phase !== 'ready' ? <div className={css.notice} role="status">{feedback ? t('scheduleStatus_error') : t('loading')}
        {feedback ? <button type="button" onClick={() => void scheduleTaskCatalog.refresh()}>{t('retryConnection')}</button> : null}</div> : null}
      <div role="tabpanel" id={`schedule-rule-${encodeURIComponent(taskId)}`} aria-labelledby={`schedule-tab-rule-${encodeURIComponent(taskId)}`} hidden={selectedTab !== 'rule'}>
        <input className={css.name} value={shown.title} disabled={disabled} aria-label={t('scheduleTaskName')}
          onChange={event => change({ title: event.target.value })} />
        <p className={css.nextRun}>{shown.nextRunAt && Number.isFinite(Date.parse(shown.nextRunAt)) ? <>{t('scheduleNextRun')}{' '}
          <time dateTime={shown.nextRunAt}>{new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23', ...(isValidScheduleTimeZone(shown.schedule.timeZone) ? { timeZone: shown.schedule.timeZone } : {}) }).format(new Date(shown.nextRunAt))}</time>{' '}
          <span>({scheduleNextRunDistance(shown.nextRunAt, now, i18n.resolvedLanguage || i18n.language)})</span></> : t('scheduleNotScheduled')}</p>
        <textarea className={css.instruction} value={shown.prompt} disabled={disabled} aria-label={t('scheduleTaskPrompt')}
          onChange={event => change({ prompt: event.target.value })} />
        <section className={css.ruleCard} aria-label={t('dshSchedule.runTime', { defaultValue: 'Run time' })}>
          <h3>{t('dshSchedule.runTime', { defaultValue: 'Run time' })}</h3>
          <div className={css.ruleRows}>
            <label className={css.row}><span>{t('dshSchedule.repeat', { defaultValue: 'Repeat' })}</span><select disabled={disabled} value={shown.schedule.kind}
              onChange={event => change({ schedule: { kind: event.target.value as ScheduledTaskV1['schedule']['kind'] } })}>
              {(['daily', 'weekly', 'cron', 'interval', 'at', 'manual'] as const).map(kind => <option key={kind} value={kind}>{t(`scheduleKind_${kind}`)}</option>)}
            </select></label>
            {shown.schedule.kind === 'daily' || shown.schedule.kind === 'weekly' ? <label className={css.row}><span>{t('scheduleDailyTime')}</span><input type="time" step="1" disabled={disabled} value={shown.schedule.timeOfDay} onChange={event => change({ schedule: { timeOfDay: event.target.value } })} /></label> : null}
            {shown.schedule.kind === 'weekly' ? <fieldset className={css.weekdays}><legend>{t('scheduleWeekdays')}</legend>{[1, 2, 3, 4, 5, 6, 7].map(day => <label key={day}><input type="checkbox" disabled={disabled} checked={shown.schedule.weekdays?.includes(day) ?? false} onChange={event => change({ schedule: { weekdays: event.target.checked ? [...(shown.schedule.weekdays ?? []), day].sort((a, b) => a - b) : shown.schedule.weekdays?.filter(value => value !== day) ?? [] } })} />{t(`scheduleWeekday_${day}`)}</label>)}</fieldset> : null}
            {shown.schedule.kind === 'cron' ? <label className={css.row}><span>{t('scheduleCronExpression')}</span><input disabled={disabled} value={shown.schedule.expression ?? ''} onChange={event => change({ schedule: { expression: event.target.value } })} /></label> : null}
            {['daily', 'weekly', 'cron'].includes(shown.schedule.kind) ? <label className={css.row}><span>{t('scheduleTimeZone')}</span><input disabled={disabled} value={shown.schedule.timeZone ?? ''} placeholder={t('scheduleLegacyLocal')} onChange={event => change({ schedule: { timeZone: event.target.value } })} /></label> : null}
            {shown.schedule.kind === 'interval' ? <label className={css.row}><span>{t('scheduleEveryMinutes')}</span><input type="number" min={1} max={10080} disabled={disabled} value={shown.schedule.everyMinutes} onChange={event => change({ schedule: { everyMinutes: Number(event.target.value) } })} /></label> : null}
            {shown.schedule.kind === 'at' ? <label className={css.row}><span>{t('scheduleAtTime')}</span><input type="datetime-local" disabled={disabled}
              value={shown.schedule.atTime ? dateTimeLocal(shown.schedule.atTime) : ''}
              onChange={event => { const date = new Date(event.target.value); change({ schedule: { atTime: Number.isFinite(date.getTime()) ? date.toISOString() : '' } }) }} /></label> : null}
            <label className={css.row}><span>{t('scheduleTaskEnabled')}</span><input type="checkbox" disabled={disabled} checked={shown.enabled} onChange={event => change({ enabled: event.target.checked })} /></label>
          </div>
          {message ? <p className={css.failure} role="alert">{message}</p> : null}
        </section>
        {shown.originalThreadId ? <button type="button" className={css.threadLink} onClick={() => onOpenThread(shown.originalThreadId!)}>{t('scheduleOpenOriginalThread')}</button> : <p className={css.notice}>{t(shown.deliveryKind === 'legacy-headless' ? 'scheduleLegacyHeadless' : 'scheduleOriginalThreadUnavailable')}</p>}
        {shown.lastThreadId ? <button type="button" className={css.threadLink} onClick={() => onOpenThread(shown.lastThreadId)}>{t('scheduleOpenLastThread')}</button> : null}
      </div>
      {selectedTab === 'history' ? <div role="tabpanel" id={`schedule-history-${encodeURIComponent(taskId)}`} aria-labelledby={`schedule-tab-history-${encodeURIComponent(taskId)}`}>
        {ready && !deletionConfirmed ? <ScheduleDeliveryHistory key={`${taskId}\u0000${shown.originalThreadId ?? 'legacy'}`} taskId={taskId} latestAttemptId={shown.latestDeliveryAttemptId} timeZone={shown.schedule.timeZone} /> : null}
      </div> : null}
    </div>
    {dirty ? <footer className={css.footer}><span>{t('unsavedChanges')}</span>
      <button type="button" disabled={pending} onClick={() => { setEdit(null); setMessage('') }}>{t('cancel')}</button>
      <button type="button" className={css.saveButton} disabled={disabled} onClick={() => void perform('save')}>{t(pending ? 'saving' : 'save')}</button>
    </footer> : null}
    {confirming ? <ScheduleDeleteConfirmation title={shown.title} pending={pending} disabled={disabled}
      opener={more.current} onClose={() => setConfirming(false)} onDelete={() => void perform('delete')} /> : null}
  </aside>
}

/* Lazy delivery view and prompt geometry adapted from pinned DSH DeliveryHistory;
 * Copyright (c) 2026 DeepSeek, MIT; see THIRD_PARTY_NOTICES.md. */
export function ScheduleDeliveryHistory({ taskId, latestAttemptId, timeZone }: { taskId: string; latestAttemptId?: string; timeZone?: string }) {
  const { t, i18n } = useTranslation('common')
  type Page = Extract<ScheduleTaskHistoryResult, { kind: 'success' }>
  const [page, setPage] = useState<Page>(), [loading, setLoading] = useState(true)
  const [failure, setFailure] = useState<{ cursor?: string; refresh: boolean }>()
  const request = useRef({ epoch: 0, pending: false })
  const load = useCallback(async (before?: string, refresh = false) => {
    if (request.current.pending && !refresh) return
    const epoch = ++request.current.epoch
    request.current.pending = true; setLoading(true); setFailure(undefined)
    if (refresh) setPage(undefined)
    let result: ScheduleTaskHistoryResult
    try { result = await window.analytix.schedule.readHistory({ taskId, limit: 20, ...(before === undefined ? {} : { before }) }) }
    catch { if (epoch !== request.current.epoch) return; request.current.pending = false; setLoading(false); setFailure({ cursor: before, refresh: false }); return }
    if (epoch !== request.current.epoch) return
    request.current.pending = false; setLoading(false)
    if (result.kind !== 'success') { setFailure({ cursor: result.kind === 'conflict' ? undefined : before, refresh: result.kind === 'conflict' }); return }
    setPage(previous => {
      if (before === undefined || previous === undefined) return result
      const seen = new Set(previous.records.map(record => record.attemptId)), records = [...previous.records]
      for (const record of result.records) if (!seen.has(record.attemptId)) { seen.add(record.attemptId); records.push(record) }
      return { ...result, records }
    })
  }, [taskId])
  useEffect(() => {
    const owner = request.current
    void load(undefined, true)
    return () => { owner.epoch++; owner.pending = false }
  }, [load, latestAttemptId])
  const format = (value: string) => new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
    ...(isValidScheduleTimeZone(timeZone) ? { timeZone } : {})
  }).format(new Date(value))
  return <div className={css.deliveryHistory} aria-busy={loading}>
    <div className={css.historyToolbar}><button type="button" className={css.historyAction} onClick={() => void load(undefined, true)}>{t('scheduleHistoryRefresh')}</button></div>
    {loading && !page?.records.length ? <p role="status" className={css.notice}>{t('loading')}</p> : null}
    {failure ? <div className={css.notice}><p role="alert">{t(failure.refresh ? 'scheduleHistoryCursorError' : 'scheduleHistoryError')}</p>
      <button type="button" className={css.historyAction} onClick={() => void load(failure.cursor, failure.refresh)}>{t(failure.refresh ? 'scheduleHistoryRefresh' : 'retryConnection')}</button></div> : null}
    {!loading && !failure && page?.records.length === 0 ? <div className={css.historyEmpty} role="status"><DshClockOutlineRegular size={24} /><h3>{t('scheduleHistoryEmpty')}</h3>
      {page.deliveryKind === 'legacy-headless' ? <p>{t('scheduleLegacyHeadless')}</p> : null}</div> : null}
    {page?.records.map(record => <section key={record.attemptId} className={css.delivery} aria-label={t('scheduleHistoryRecord')}>
      <DshClockOutlineRegular className={css.deliveryGlyph} /><div className={css.deliveryBody}>
        <div className={css.deliveryHead}><time className={css.deliveryTime} dateTime={record.scheduledAt}>{format(record.scheduledAt)}</time></div>
        <div className={css.deliveryAck}>{t('scheduleHistoryDeliveredAt')} <time dateTime={record.deliveredAt}>{format(record.deliveredAt)}</time></div>
        {record.prompt !== undefined ? <ScheduleSavedPrompt record={record} /> : null}
      </div></section>)}
    {page?.nextBefore !== undefined && !failure ? <button type="button" disabled={loading} className={css.historyAction} onClick={() => void load(page.nextBefore)}>{t('scheduleHistoryLoadMore')}</button> : null}
  </div>
}

function ScheduleSavedPrompt({ record }: { record: ScheduleTaskDeliveryRecord }) {
  const { t } = useTranslation('common'), id = useId(), ref = useRef<HTMLParagraphElement>(null)
  const [expanded, setExpanded] = useState(false), [clamped, setClamped] = useState(false)
  useLayoutEffect(() => {
    if (expanded || !ref.current) return
    const paragraph = ref.current, measure = () => setClamped(paragraph.scrollHeight > paragraph.clientHeight)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(measure); observer.observe(paragraph)
    return () => observer.disconnect()
  }, [expanded, record.prompt])
  return <><p ref={ref} id={id} className={css.savedPrompt} data-expanded={expanded || undefined}>{record.prompt}</p>
    {clamped ? <button type="button" className={css.savedPromptToggle} aria-expanded={expanded} aria-controls={id} onClick={() => setExpanded(value => !value)}>
      {t(expanded ? 'scheduleHistoryCollapse' : 'scheduleHistoryExpand')}{expanded ? <DshChevronUpOutlineRegular /> : <DshChevronDownOutlineRegular />}</button> : null}</>
}

function ScheduleDeleteConfirmation({ title, pending, disabled, opener, onClose, onDelete }: {
  title: string; pending: boolean; disabled: boolean; opener: HTMLElement | null; onClose: () => void; onDelete: () => void
}) {
  const { t } = useTranslation('common')
  const focus = useModalFocus(onClose, !pending, true, opener)
  return createPortal(<div className={css.confirmLayer} data-analytix-chat-view role="presentation">
    <div className={css.confirmMask} aria-hidden="true" onClick={() => { if (!pending) onClose() }} />
    <div ref={focus} className={css.confirm} role="alertdialog" aria-modal="true" aria-label={t('dshSchedule.deleteTitle', { defaultValue: 'Delete this task?' })} data-shortcut-modal="other" tabIndex={-1}>
      <header><h2>{t('dshSchedule.deleteTitle', { defaultValue: 'Delete this task?' })}</h2><button type="button" className={css.iconButton} disabled={pending} aria-label={t('close')} onClick={onClose}><DshCloseOutlineRegular size={14} /></button></header>
      <p>{title}</p><div className={css.confirmActions}>
        <button type="button" disabled={pending} onClick={onClose} data-modal-autofocus>{t('cancel')}</button>
        <button type="button" className={css.deleteButton} disabled={disabled} onClick={onDelete}>{t('delete')}</button>
      </div>
    </div>
  </div>, document.body)
}

function dateTimeLocal(value: string): string {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}
