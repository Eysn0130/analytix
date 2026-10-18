/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * TrajectoryTable Inspector. Copyright (c) 2026 DeepSeek. MIT; THIRD_PARTY_NOTICES.md. */
import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { MarkdownText, type MarkdownLabels } from '../chat/presentation/markdown/MarkdownText'
import { trajectoryDuration, type TrajectoryRecord } from './trajectory-adapter'
import { trajectoryStatusLabel, type TrajectoryTranslate } from './TrajectoryTimeline'
import css from './TrajectoryPanel.module.css'
const DETAILS_MIN_WIDTH = 320, DETAILS_MAX_WIDTH = 720, TABLE_MIN_WIDTH = 280, DETAILS_RESIZE_STEP = 16
const FACT_LABELS: Readonly<Record<string, string>> = { retrySource: 'Retry source', phase: 'Phase', attempt: 'Attempt',
  maxAttempt: 'Maximum attempts', retryDelay: 'Retry delay', code: 'Public code' }
type Tab = 'overview' | 'preview' | 'content' | 'timing' | 'result' | 'record'
export function TrajectoryInspector({ record, t, onClose, canInspect, onInspect, onRetireInspect, protectedContent }: {
  record: TrajectoryRecord; t: TrajectoryTranslate; onClose: () => void
  canInspect: boolean; onInspect: () => void; onRetireInspect: () => void; protectedContent?: ReactNode
}) {
  const [tab, setTab] = useState<Tab>('overview'), [wrap, setWrap] = useState(true), [copied, setCopied] = useState(false)
  const [width, setWidth] = useState<number | null>(null)
  const root = useRef<HTMLElement>(null), drag = useRef<{ id: number; x: number; width: number; split: number } | null>(null)
  const id = useId()
  useEffect(() => { setTab('overview'); setCopied(false) }, [record.id])
  const tabs: { id: Tab; label: string }[] = [{ id: 'overview', label: t('summary', 'Summary') },
    ...(record.text === undefined ? [] : [{ id: 'preview' as const, label: t('preview', 'Preview') }, { id: 'content' as const, label: t('publicText', 'Public text') }]),
    ...(record.startedAt || record.endedAt || record.metrics ? [{ id: 'timing' as const, label: t('timing', 'Timing') }] : []),
    ...(record.toolMetadata?.projectionKind ? [{ id: 'result' as const, label: t('publicResult', 'Public result') }] : []),
    { id: 'record', label: t('publicRecord', 'Public record') }]
  const active = tabs.some(value => value.id === tab) ? tab : 'overview'
  const time = trajectoryDuration(record)
  const fields: [string, string][] = [
    [t('turn', 'Turn'), record.turnId], [t('sequence', 'Sequence'), `#${record.firstSeq}–#${record.lastSeq}`],
    [t('status', 'Status'), trajectoryStatusLabel(record.status, t)], [t('recordedAt', 'Recorded at'), record.at ?? t('unknown', 'Unknown')],
    [t('start', 'Start'), record.startedAt ?? '—'], [t('end', 'End'), record.endedAt ?? '—'],
    [t('duration', 'Duration'), time === null ? t('unknown', 'Unknown') : `${time} ms`],
    ...Object.entries({
      ...(record.retry ? { retrySource: record.retry.source, phase: record.retry.phase, attempt: record.retry.attempt,
        maxAttempt: record.retry.maxAttempt, ...(record.retry.delayMs === undefined ? {} : { retryDelay: `${record.retry.delayMs} ms` }) } : {}),
      ...record.error
    }).filter(([, value]) => value !== undefined).map(([key, value]): [string, string] => [t(key, FACT_LABELS[key]),
      key === 'phase' ? t(`phaseValue.${value}`, String(value)) : key === 'retrySource' ? t(`retrySourceValue.${value}`, String(value)) : String(value)]),
    ...(record.logicalStepId ? [[t('logicalStep', 'Logical step'), record.logicalStepId] as [string, string]] : []),
    ...(record.tool ? [[t('callId', 'Call ID'), record.tool.callId] as [string, string],
      ...record.tool.toolCallItemId ? [[t('callItem', 'Call item'), record.tool.toolCallItemId] as [string, string]] : [],
      ...record.tool.toolResultItemId ? [[t('resultItem', 'Result item'), record.tool.toolResultItemId] as [string, string]] : []] : [])]
  const payload = active === 'content' ? record.text ?? '' : active === 'result' ? JSON.stringify(record.toolMetadata, null, 2) : JSON.stringify(record, null, 2)
  const labels: MarkdownLabels = { code: { copyLabel: t('copy', 'Copy'), copiedLabel: t('copied', 'Copied') }, footnotes: t('footnotes', 'Footnotes'),
    table: { title: t('table', 'Table'), copy: t('copy', 'Copy'), copied: t('copied', 'Copied'), copyFailed: t('copyFailed', 'Copy failed'), download: '', downloadFailed: '', fullscreen: '', close: '', csv: 'CSV', tsv: 'TSV', markdown: 'Markdown', exportHint: '' } }
  const resize = (value: number, split: number) => setWidth(Math.round(Math.max(DETAILS_MIN_WIDTH, Math.min(value, DETAILS_MAX_WIDTH, split - TABLE_MIN_WIDTH))))
  const changeTab = (next: Tab) => { onRetireInspect(); setTab(next); setCopied(false) }
  return <aside ref={root} className={css.inspector} aria-label={t('inspector', 'Inspector')} style={width === null ? undefined : { width }}>
    <div className={css.inspectorResize} role="separator" aria-label={t('resizeInspector', 'Resize Inspector')} aria-orientation="vertical" tabIndex={0}
      aria-valuemin={DETAILS_MIN_WIDTH} aria-valuemax={DETAILS_MAX_WIDTH} aria-valuenow={width ?? 400}
      onPointerDown={event => {
        if (event.button !== 0 || !root.current?.parentElement) return
        event.preventDefault()
        drag.current = { id: event.pointerId, x: event.clientX, width: root.current.getBoundingClientRect().width, split: root.current.parentElement.getBoundingClientRect().width }
        event.currentTarget.setPointerCapture?.(event.pointerId)
      }} onPointerMove={event => { const current = drag.current; if (current?.id === event.pointerId) resize(current.width + current.x - event.clientX, current.split) }}
      onPointerUp={event => { if (drag.current?.id === event.pointerId) drag.current = null; if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId) }}
      onPointerCancel={() => { drag.current = null }} onKeyDown={event => {
        if (event.key === 'Home') { event.preventDefault(); setWidth(null); return }
        if (!['ArrowLeft', 'ArrowRight'].includes(event.key) || !root.current?.parentElement) return
        event.preventDefault(); resize(root.current.getBoundingClientRect().width + (event.key === 'ArrowLeft' ? 1 : -1) * DETAILS_RESIZE_STEP, root.current.parentElement.getBoundingClientRect().width)
      }} />
    <div className={css.inspectorHeader}><span className={css.dot} /><strong>{record.kind === 'tool' ? record.title : t(record.kind, record.title)}</strong>
      <code className={css.inspectorLocation}>{record.tool?.callId ?? record.logicalStepId ?? `#${record.firstSeq}`}</code>
      <button type="button" className={css.close} aria-label={t('closeInspector', 'Close Inspector')} onClick={onClose}>×</button></div>
    <div className={css.detailTabs} role="tablist" aria-label={t('inspector', 'Inspector')} onKeyDown={event => {
      const index = tabs.findIndex(value => value.id === active)
      const next = event.key === 'ArrowRight' ? (index + 1) % tabs.length : event.key === 'ArrowLeft' ? (index + tabs.length - 1) % tabs.length
        : event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : -1
      if (next < 0) return
      event.preventDefault(); changeTab(tabs[next].id)
      event.currentTarget.querySelector<HTMLButtonElement>(`[data-tab="${tabs[next].id}"]`)?.focus()
    }}>{tabs.map(value => <button type="button" role="tab" className={active === value.id ? `${css.detailTab} ${css.detailTabActive}` : css.detailTab} key={value.id} id={`${id}-${value.id}`} aria-controls={`${id}-body`} data-tab={value.id}
      aria-selected={active === value.id} tabIndex={active === value.id ? 0 : -1} onClick={() => changeTab(value.id)}>{value.label}</button>)}</div>
    <div id={`${id}-body`} className={css.detailBody} role="tabpanel" aria-labelledby={`${id}-${active}`} tabIndex={0}>
      {active === 'overview' ? <><dl className={css.overview}>{fields.map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}</dl>
        {record.kind === 'step' && !record.endedAt ? <p className={css.notice}>{t('missingAssembly', 'Assembly time is unavailable for this step.')}</p> : null}
        {record.ambiguousTool ? <p className={css.notice}>{t('ambiguous', 'Call identity is ambiguous.')}</p> : null}
        {record.toolMetadata?.argumentsWithheld ? <p className={css.notice}>{t('argumentsWithheld', 'Tool arguments are withheld. Inspect uses the existing protected owner.')}</p> : null}
        {canInspect ? <button type="button" className={css.inspect} onClick={onInspect}>{t('inspectTool', 'Inspect tool')}</button> : null}
        {protectedContent}
      </> : active === 'timing' ? <dl className={css.overview}>
        {fields.filter(([label]) => [t('recordedAt', 'Recorded at'), t('start', 'Start'), t('end', 'End'), t('duration', 'Duration')].includes(label)).map(([label, value]) => <div key={label}><dt>{label}</dt><dd title={value}>{value}</dd></div>)}
        {Object.entries(record.metrics ?? {}).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value} {key === 'decodeTokens' ? t('tokens', 'tokens') : 'ms'}</dd></div>)}
      </dl> : active === 'preview' ? <div className={css.previewBody}><MarkdownText text={record.text ?? ''} labels={labels} viewOnly /></div>
        : <><div className={css.payloadTools}><button type="button" aria-pressed={wrap} onClick={() => setWrap(value => !value)}>{t('wrap', 'Wrap lines')}</button>
          {typeof navigator.clipboard?.writeText === 'function' ? <button type="button" onClick={() => { void navigator.clipboard.writeText(payload).then(() => setCopied(true), () => setCopied(false)) }}>{copied ? t('copied', 'Copied') : t('copy', 'Copy')}</button> : null}</div>
          <pre className={css.publicPayload} data-wrapped={wrap || undefined}>{payload}</pre></>}
      {record.textTruncated ? <p className={css.notice}>{t('textTruncated', 'Public text preview is truncated; search covers the retained preview only.')}</p> : null}
    </div>
  </aside>
}
