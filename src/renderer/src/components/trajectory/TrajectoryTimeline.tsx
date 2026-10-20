/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * TrajectoryTimeline.tsx/module.css. Copyright (c) 2026 DeepSeek. MIT; THIRD_PARTY_NOTICES.md. */
import { useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent } from 'react'
import { trajectoryDuration, type TrajectoryRecord } from './trajectory-adapter'
import { deriveTrajectoryTimeline, type TrajectoryRange } from './trajectory-timeline-model'
import css from './TrajectoryPanel.module.css'
export type { TrajectoryRange } from './trajectory-timeline-model'
export type TrajectoryTranslate = (key: string, fallback: string) => string
const STATUS_LABELS: ReadonlyMap<string, string> = new Map([
  ['ready', 'Ready'], ['running', 'Running'], ['completed', 'Completed'], ['failed', 'Failed'],
  ['blocked', 'Blocked'], ['cancelled', 'Cancelled'], ['unknown', 'Unknown'], ['recorded', 'Recorded']
])
/** Localized presentation only; raw public status remains unchanged for records and search. */
export function trajectoryStatusLabel(status: string, t: TrajectoryTranslate): string {
  const known = STATUS_LABELS.has(status) ? status : 'unknown'
  return t(`state.${known}`, STATUS_LABELS.get(known)!)
}
const MINIMUM_DRAG_PX = 3, MINIMUM_ZOOM_OPERATIONS = 4
const EDGE_PAN_ZONE_FRACTION = .08, EDGE_PAN_STEP_FRACTION = .025, MAXIMUM_EDGE_PAN_PX = 32
const clamp = (value: number, min = 0, max = 1) => Math.max(min, Math.min(max, value))
type Window = { start: number; end: number }
type Gesture = { id: number; x: number; start: number; viewportStart: number; recordId: string | null; pan: boolean; moved: boolean }
export function TrajectoryTimeline({ records, duration, range, onRangeChange, onSelect, selectedId, revealVersion = 0,
  searchMatches, hasEarlier, onLoadEarlier, sequenceOffset, coordinateEpoch, coordinateRecords, loading = false, t }: {
  records: readonly TrajectoryRecord[]; duration: boolean; range: TrajectoryRange | null
  onRangeChange: (range: TrajectoryRange | null) => void; onSelect: (record: TrajectoryRecord) => void
  selectedId: string | null; revealVersion?: number; searchMatches: ReadonlySet<string> | null
  hasEarlier: boolean; onLoadEarlier: () => void; sequenceOffset: number; coordinateEpoch: string; coordinateRecords: readonly TrajectoryRecord[]; loading?: boolean; t: TrajectoryTranslate
}) {
  const model = useMemo(() => deriveTrajectoryTimeline(records, duration, sequenceOffset, coordinateRecords), [records, duration, sequenceOffset, coordinateRecords])
  const track = useRef<HTMLDivElement>(null), gesture = useRef<Gesture | null>(null), revealed = useRef('')
  const [viewport, setViewport] = useState<Window | null>(null), [draft, setDraft] = useState<Window | null>(null)
  const [panning, setPanning] = useState(false), [hover, setHover] = useState<number | null>(null)
  const mode = duration ? 'duration' : 'sequence'
  const full = model.end - model.start, width = Math.min(full, Math.max(1, viewport ? viewport.end - viewport.start : full))
  const start = viewport ? clamp(viewport.start, model.start, model.end - width) : model.start
  const activeRange = draft ?? range
  const reset = () => { gesture.current = null; setDraft(null); setPanning(false); setViewport(null); onRangeChange(null) }
  useEffect(() => { setViewport(null); setDraft(null); setPanning(false); gesture.current = null }, [duration, coordinateEpoch])
  useEffect(() => {
    const key = `${selectedId}:${revealVersion}`
    if (revealed.current === key) return
    revealed.current = key
    const span = model.spans.find(span => span.record.id === selectedId)
    if (!span || !viewport || span.end >= start && span.start <= start + width) return
    const next = clamp(span.start < start ? span.start : span.end - width, model.start, model.end - width)
    setViewport({ start: next, end: next + width })
  }, [selectedId, revealVersion, model, viewport, start, width])
  useEffect(() => {
    const element = track.current
    if (!element) return
    const wheel = (event: WheelEvent) => {
      event.preventDefault()
      const rect = element.getBoundingClientRect(), anchor = clamp((event.clientX - rect.left) / Math.max(1, rect.width))
      const nextWidth = clamp(width * Math.exp(event.deltaY * .0015), Math.min(duration ? 20 : MINIMUM_ZOOM_OPERATIONS, full), full)
      if (nextWidth >= full * .999) { setViewport(null); return }
      const nextStart = clamp(start + anchor * width - anchor * nextWidth, model.start, model.end - nextWidth)
      setViewport({ start: nextStart, end: nextStart + nextWidth })
    }
    element.addEventListener('wheel', wheel, { passive: false })
    return () => element.removeEventListener('wheel', wheel)
  }, [duration, full, model.start, model.end, start, width])
  const fraction = (event: PointerEvent<HTMLDivElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    return clamp((event.clientX - rect.left) / Math.max(1, rect.width))
  }
  const cancel = () => { gesture.current = null; setDraft(null); setPanning(false) }
  const finish = (event: PointerEvent<HTMLDivElement>) => {
    const drag = gesture.current
    if (!drag || drag.id !== event.pointerId) return
    const moved = drag.moved || Math.abs(event.clientX - drag.x) >= MINIMUM_DRAG_PX
    cancel()
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
    if (drag.pan) { if (!moved) onRangeChange(null); return }
    const span = model.spans.find(span => span.record.id === drag.recordId)
    if (!moved && span) { onRangeChange(null); onSelect(span.record); return }
    const point = start + fraction(event) * width
    const minimum = Math.min(width, full / Math.max(1, model.spans.length))
    let left = Math.min(drag.start, point), right = Math.max(drag.start, point)
    if (right - left < minimum) { const middle = (left + right) / 2; left = clamp(middle - minimum / 2, model.start, model.end - minimum); right = left + minimum }
    onRangeChange({ start: left, end: right, mode })
    if (!moved && model.spans.length) {
      const nearest = [...model.spans].sort((a, b) => Math.min(Math.abs(point - a.start), Math.abs(point - a.end)) - Math.min(Math.abs(point - b.start), Math.abs(point - b.end)))[0]
      onSelect(nearest.record)
    }
  }
  const unknown = duration ? records.filter(record => record.kind !== 'message' && !(record.startedAt ?? record.at)) : []
  const emptyLabel = loading ? t('loading', 'Loading') : !records.length ? t('emptyRecords', 'No public records loaded yet.')
    : duration && records.some(record => record.kind !== 'message')
      ? t('noTimingWithSequence', 'Timeline entries have no recorded timestamps. Turn off Duration to view the sequence.')
      : t('noTimelineEntries', 'These public records have no timeline entries.')
  const turns = new Map<string, number>()
  for (const span of model.spans) if (!turns.has(span.record.turnId)) turns.set(span.record.turnId, span.start)
  return <section className={css.timeline} aria-label={t('timeline', 'Trajectory timeline')}>
    <div className={css.plot}>
      <div className={css.laneLabels} aria-hidden><span>{t('input', 'Input')}</span><span>{t('model', 'Model')}</span><span>{t('tool', 'Tool')}</span></div>
      <div ref={track} className={css.track} tabIndex={0} data-panning={panning || undefined}
        aria-label={t('timelineControls', 'Drag to select; wheel to zoom; right drag to pan; Escape clears selection; Home resets view')}
        onKeyDown={event => { if (event.key === 'Escape') { event.preventDefault(); cancel(); onRangeChange(null) } else if (event.key === 'Home') { event.preventDefault(); reset() } }}
        onContextMenu={event => event.preventDefault()} onDoubleClick={event => { event.preventDefault(); reset() }}
        onPointerDown={event => {
          if (event.button !== 0 && event.button !== 2 || (event.target as HTMLElement).closest('[data-earlier-history],[data-reset-view]')) return
          const point = start + fraction(event) * width
          gesture.current = { id: event.pointerId, x: event.clientX, start: point, viewportStart: start,
            recordId: (event.target as HTMLElement).closest<HTMLElement>('[data-timeline-record]')?.dataset.timelineRecord ?? null, pan: event.button === 2, moved: false }
          setPanning(event.button === 2)
          if (event.button === 0) setDraft({ start: point, end: point })
          event.currentTarget.setPointerCapture?.(event.pointerId)
        }}
        onPointerMove={event => {
          const f = fraction(event); setHover(f)
          const drag = gesture.current
          if (!drag || drag.id !== event.pointerId) return
          drag.moved ||= Math.abs(event.clientX - drag.x) >= MINIMUM_DRAG_PX
          const rect = event.currentTarget.getBoundingClientRect()
          if (drag.pan) {
            if (viewport) { const next = clamp(drag.viewportStart - (event.clientX - drag.x) / Math.max(1, rect.width) * width, model.start, model.end - width); setViewport({ start: next, end: next + width }) }
            return
          }
          let nextStart = start
          if (viewport) {
            const edge = Math.min(MAXIMUM_EDGE_PAN_PX, Math.max(1, rect.width * EDGE_PAN_ZONE_FRACTION))
            const x = event.clientX - rect.left, direction = x < edge ? -1 : x > rect.width - edge ? 1 : 0
            if (direction) { nextStart = clamp(start + direction * width * EDGE_PAN_STEP_FRACTION, model.start, model.end - width); setViewport({ start: nextStart, end: nextStart + width }) }
          }
          const point = nextStart + f * width
          setDraft({ start: Math.min(drag.start, point), end: Math.max(drag.start, point) })
        }} onPointerUp={finish} onPointerCancel={cancel} onPointerLeave={() => { if (!gesture.current) setHover(null) }}>
        {hasEarlier && start === model.start ? <button type="button" className={css.earlier} data-earlier-history aria-label={t('loadOlder', 'Load 50 older records')} title={t('loadOlder', 'Load 50 older records')} onClick={onLoadEarlier}>…</button> : null}
        {viewport ? <button type="button" className={css.resetView} data-reset-view onClick={reset}>{t('resetView', 'Reset view')}</button> : null}
        {!model.spans.length ? <span className={css.emptyTimeline} role="status">{emptyLabel}</span> : null}
        {[...turns].map(([id, at]) => <span key={id} className={css.turnBoundary} style={{ left: `${(at - start) / width * 100}%` }} />)}
        {activeRange ? <div className={css.selection} data-dragging={Boolean(draft) || undefined} style={{ left: `${(activeRange.start - start) / width * 100}%`, width: `${(activeRange.end - activeRange.start) / width * 100}%` }} /> : null}
        {hover !== null && !gesture.current ? <span className={css.hoverLine} style={{ left: `${hover * 100}%` }} /> : null}
        {model.spans.filter(span => span.end >= start && span.start <= start + width).map(span => {
          const record = span.record, elapsed = trajectoryDuration(record)
          const detail = [record.title, record.startedAt ?? record.at, record.endedAt,
            elapsed === null ? t('unknownTime', 'Time unknown') : `${elapsed} ms`,
            record.metrics?.ttftMs === undefined ? null : `TTFT ${record.metrics.ttftMs} ms`,
            record.metrics?.decodeMs === undefined ? null : `${t('decode', 'Decode')} ${record.metrics.decodeMs} ms`].filter(Boolean).join('\n')
          const selected = !activeRange || span.start <= activeRange.end && span.end >= activeRange.start
          const ttft = elapsed && record.metrics?.ttftMs !== undefined ? clamp(record.metrics.ttftMs / elapsed) * 100 : null
          return <button key={record.id} type="button" className={css.mark} data-timeline-record={record.id} data-lane={record.lane} data-kind={record.kind}
            data-point={span.point || undefined} data-unknown-end={record.kind === 'step' && !record.endedAt || undefined}
            data-error={['failed', 'error'].includes(record.status) || undefined} data-selected={selected} data-current={selectedId === record.id || undefined}
            data-search-match={searchMatches === null ? undefined : searchMatches.has(record.id)}
            aria-pressed={selectedId === record.id} aria-label={`${record.title} · #${record.firstSeq} · ${trajectoryStatusLabel(record.status, t)}`} title={detail}
            style={{ '--trajectory-span-left': `${(span.start - start) / width * 100}%`, '--trajectory-span-width': `${(span.end - span.start) / width * 100}%`,
              '--trajectory-span-gap': `min(${(span.end - span.start) / width * 100 * .08}%, 1px)`, '--trajectory-span-lane': span.lane,
              top: `${7 + span.lane * 14}px`, ...(ttft === null ? {} : { '--trajectory-ttft': `${ttft}%` }) } as CSSProperties}
            onClick={event => { if (event.detail === 0) { onRangeChange(null); onSelect(record) } }} />
        })}
        {unknown.length ? <button type="button" className={css.untimed} onClick={() => onSelect(unknown[0])}>{t('unknownTime', 'Time unknown')} ({unknown.length})</button> : null}
      </div>
    </div>
  </section>
}
