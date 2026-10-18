/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * TrajectoryTable scroll/window/rows. Copyright (c) 2026 DeepSeek. MIT; THIRD_PARTY_NOTICES.md. */
import { useLayoutEffect, useMemo, useRef, useState } from 'react'
import { trajectoryDuration, type TrajectoryRecord } from './trajectory-adapter'
import { trajectoryStatusLabel, type TrajectoryTranslate } from './TrajectoryTimeline'
import css from './TrajectoryPanel.module.css'
const ROW_HEIGHT = 30, BOTTOM_FOLLOW_THRESHOLD_PX = 2, OLDER_LOAD_THRESHOLD_PX = 48
const VIRTUALIZATION_THRESHOLD = 100, VIRTUAL_OVERSCAN_ROWS = 12
const SUMMARY_CODEPOINT_LIMIT = 160, TOOL_NAME_LIMIT = 3, TOOL_NAME_CODEPOINT_LIMIT = 24
export type TrajectoryRevealRequest = { recordId: string; version: number }
type Group = 'model' | 'calls'
type Row = { key: string; kind: 'turn'; turnId: string; count: number; records: readonly TrajectoryRecord[] } |
  { key: string; kind: 'group'; turnId: string; group: Group; count: number; records: readonly TrajectoryRecord[] } |
  { key: string; kind: 'record'; record: TrajectoryRecord }
const groupKind = (record: TrajectoryRecord): Group | null => record.kind === 'tool' ? 'calls' : record.kind === 'step' || record.kind === 'message' ? 'model' : null
function foldSummary(records: readonly TrajectoryRecord[], t: TrajectoryTranslate): string {
  if (!records.length) return ''
  const tools = records.filter(record => record.kind === 'tool')
  const counts = [
    [t('hiddenRecords', 'Hidden records'), records.length],
    [t('steps', 'Steps'), records.filter(record => record.kind === 'step').length],
    [t('messages', 'Messages'), records.filter(record => record.kind === 'message').length],
    [t('calls', 'Calls'), tools.length]
  ].filter(([, count]) => count).map(([label, count]) => `${label}: ${count}`)
  const prefix = counts.join(' · '), names = [...new Set(tools.map(record => record.title))]
  if (!names.length) return prefix
  const shown = names.slice(0, TOOL_NAME_LIMIT), omitted = names.length - shown.length
  const suffix = omitted ? `${t('moreToolNames', 'More tool names')}: ${omitted}` : ''
  const budget = SUMMARY_CODEPOINT_LIMIT - [...prefix].length - [...suffix].length - (suffix ? 6 : 3)
  const nameLimit = Math.min(TOOL_NAME_CODEPOINT_LIMIT, Math.floor((budget - (shown.length - 1) * 2) / shown.length))
  const visibleNames = shown.map(name => {
    const points = [...name]
    return points.length > nameLimit ? `${points.slice(0, Math.max(0, nameLimit - 1)).join('')}…` : name
  }).join(', ')
  return [prefix, visibleNames, suffix].filter(Boolean).join(' · ')
}
export function trajectoryGroups(records: readonly TrajectoryRecord[]) {
  const byRecord = new Map<string, string>(), modelIds: string[] = [], callIds: string[] = []
  const turns = new Map<string, TrajectoryRecord[]>()
  for (const record of records) { const turn = turns.get(record.turnId) ?? []; turn.push(record); turns.set(record.turnId, turn) }
  for (const turn of turns.values()) {
    let previous: Group | null = null, key = ''
    for (const record of turn) {
      const kind = groupKind(record)
      if (!kind) { previous = null; continue }
      if (kind !== previous) { key = `${kind}:${record.id}`; (kind === 'model' ? modelIds : callIds).push(key) }
      byRecord.set(record.id, key); previous = kind
    }
  }
  return { byRecord, modelIds, callIds }
}
export function TrajectoryTable({ records, groupIds, selectedId, focusedIds, onSelect, t, collapsedTurns, collapsedModels, collapsedCalls,
  onToggleTurn, onToggleModel, onToggleCalls, hasEarlier, onLoadEarlier, loading, unfilteredRecordCount = records.length, historyStartSeq, reveal, onRevealed }: {
  records: readonly TrajectoryRecord[]; groupIds: ReadonlyMap<string, string>; selectedId: string | null; focusedIds: ReadonlySet<string> | null
  onSelect: (record: TrajectoryRecord) => void; t: TrajectoryTranslate
  collapsedTurns: ReadonlySet<string>; collapsedModels: ReadonlySet<string>; collapsedCalls: ReadonlySet<string>
  onToggleTurn: (turnId: string) => void; onToggleModel: (turnId: string) => void; onToggleCalls: (turnId: string) => void
  hasEarlier: boolean; onLoadEarlier: () => void; loading: boolean; unfilteredRecordCount?: number; historyStartSeq: number
  reveal: TrajectoryRevealRequest | null; onRevealed: (request: TrajectoryRevealRequest) => void
}) {
  const scroll = useRef<HTMLDivElement>(null)
  const [viewport, setViewport] = useState({ top: 0, height: 600 })
  const followsTail = useRef(true), initialized = useRef(false), loadingOlder = useRef(false), appliedReveal = useRef<number | null>(null)
  const anchor = useRef<{ key: string; offset: number } | null>(null)
  const previousRows = useRef<readonly Row[]>([]), previousPrefix = useRef(0), previousHistoryStart = useRef(historyStartSeq)
  const rows = useMemo(() => {
    const turns = new Map<string, TrajectoryRecord[]>()
    for (const record of records) { const turn = turns.get(record.turnId) ?? []; turn.push(record); turns.set(record.turnId, turn) }
    return [...turns].flatMap<Row>(([turnId, turn]) => {
      const collapsed = collapsedTurns.has(turnId), retained = collapsed ? turn.find(record => record.text?.trim()) : undefined
      const result: Row[] = [{ key: `turn:${turnId}`, kind: 'turn', turnId, count: turn.length,
        records: retained ? turn.filter(record => record !== retained) : turn }]
      if (collapsed) {
        if (retained) result.push({ key: retained.id, kind: 'record', record: retained })
        return result
      }
      // Runs are visual sections in event order, never inferred step→tool parents.
      for (let index = 0; index < turn.length;) {
        const record = turn[index], group = groupKind(record), groupKey = groupIds.get(record.id) ?? `${group}:${record.id}`
        if (!group) { result.push({ key: record.id, kind: 'record', record }); index++; continue }
        let end = index + 1
        while (end < turn.length && groupKind(turn[end]) === group && groupIds.get(turn[end].id) === groupKey) end++
        const entries = turn.slice(index, end), closed = (group === 'model' ? collapsedModels : collapsedCalls).has(groupKey)
        const message = closed && group === 'model' ? entries.find(entry => entry.kind === 'message') : undefined
        result.push({ key: groupKey, kind: 'group', turnId, group, count: entries.length,
          records: message ? entries.filter(entry => entry !== message) : entries })
        for (const entry of closed ? message ? [message] : [] : entries) result.push({ key: entry.id, kind: 'record', record: entry })
        index = end
      }
      return result
    })
  }, [records, groupIds, collapsedTurns, collapsedModels, collapsedCalls])
  const prefix = hasEarlier ? ROW_HEIGHT : 0
  const rememberAnchor = (top: number, currentRows = rows, offset = prefix) => {
    const index = Math.min(currentRows.length - 1, Math.max(0, Math.floor((top - offset) / ROW_HEIGHT)))
    const row = currentRows[index]
    if (row) anchor.current = { key: row.key, offset: top - offset - index * ROW_HEIGHT }
  }
  const loadEarlier = () => {
    if (!hasEarlier || loadingOlder.current || loading) return
    const element = scroll.current
    if (element) {
      // Anchor the first actual record; a reused turn header must not hide a prepend.
      const index = rows.findIndex(row => row.kind === 'record')
      if (index >= 0) anchor.current = { key: rows[index].key, offset: element.scrollTop - prefix - index * ROW_HEIGHT }
    }
    followsTail.current = false; loadingOlder.current = true; onLoadEarlier()
  }
  useLayoutEffect(() => {
    const element = scroll.current
    if (!element || typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(() => {
      if (followsTail.current) element.scrollTop = Math.max(0, element.scrollHeight - (element.clientHeight || 600))
      setViewport({ top: element.scrollTop, height: element.clientHeight || 600 })
    })
    observer.observe(element)
    return () => observer.disconnect()
  }, [])
  useLayoutEffect(() => {
    const element = scroll.current
    if (!element) return
    if (!initialized.current && !loading) { initialized.current = true; followsTail.current = true }
    const newReveal = reveal && appliedReveal.current !== reveal.version
    if (initialized.current && !newReveal) {
      if (followsTail.current) element.scrollTop = Math.max(0, (element.scrollHeight || prefix + rows.length * ROW_HEIGHT) - (element.clientHeight || 600))
      else if (anchor.current) {
        const index = rows.findIndex(row => row.key === anchor.current!.key)
        if (index >= 0) element.scrollTop = Math.max(0, prefix + index * ROW_HEIGHT + anchor.current.offset)
      }
    }
    if (previousHistoryStart.current !== historyStartSeq || previousRows.current[0]?.key !== rows[0]?.key || records[0]?.id !== previousRows.current.find(row => row.kind === 'record')?.key || previousPrefix.current !== prefix) loadingOlder.current = false
    previousRows.current = rows; previousPrefix.current = prefix; previousHistoryStart.current = historyStartSeq
    rememberAnchor(element.scrollTop)
    setViewport(current => current.top === element.scrollTop ? current : { top: element.scrollTop, height: element.clientHeight || 600 })
  }, [rows, prefix, loading, records, reveal, historyStartSeq])
  useLayoutEffect(() => {
    if (!reveal || appliedReveal.current === reveal.version) return
    const element = scroll.current, index = rows.findIndex(row => row.key === reveal.recordId)
    if (!element || index < 0) return
    const top = prefix + index * ROW_HEIGHT, height = element.clientHeight || 600
    if (top < element.scrollTop || top + ROW_HEIGHT > element.scrollTop + height) element.scrollTop = Math.max(0, top - height / 2 + ROW_HEIGHT / 2)
    followsTail.current = false
    rememberAnchor(element.scrollTop)
    const node = [...element.querySelectorAll<HTMLElement>('[data-record-id]')].find(node => node.dataset.recordId === reveal.recordId)
    if (!node) { setViewport({ top: element.scrollTop, height }); return }
    appliedReveal.current = reveal.version
    node.focus({ preventScroll: true }); onRevealed(reveal)
  }, [reveal, rows, prefix, viewport, onRevealed])
  const virtual = rows.length > VIRTUALIZATION_THRESHOLD
  const start = virtual ? Math.max(0, Math.floor((viewport.top - prefix) / ROW_HEIGHT) - VIRTUAL_OVERSCAN_ROWS) : 0
  const end = virtual ? Math.min(rows.length, Math.ceil((viewport.top - prefix + viewport.height) / ROW_HEIGHT) + VIRTUAL_OVERSCAN_ROWS) : rows.length
  return <div className={css.ledger}>
    <div className={css.columnHead} role="row"><span>{t('event', 'Event')}</span><span>{t('content', 'Content')}</span></div>
    <div className={css.ledgerScroll} ref={scroll} data-trajectory-scroll role="table" aria-label={t('events', 'Trajectory events')} aria-rowcount={rows.length + (hasEarlier ? 1 : 0)}
      onScroll={event => {
        const element = event.currentTarget
        followsTail.current = element.scrollHeight > 0 ? element.scrollHeight - element.clientHeight - element.scrollTop <= BOTTOM_FOLLOW_THRESHOLD_PX : element.scrollTop === 0
        rememberAnchor(element.scrollTop)
        setViewport({ top: element.scrollTop, height: element.clientHeight || 600 })
        if (element.scrollTop <= OLDER_LOAD_THRESHOLD_PX) loadEarlier()
      }}>
      {hasEarlier ? <button type="button" className={css.load} disabled={loading} onClick={loadEarlier}>{t('loadOlder', 'Load 50 older records')}</button> : null}
      {loading ? <div className={css.loading} role="status">{t('loading', 'Loading')}</div> : null}
      {start > 0 ? <div aria-hidden style={{ height: start * ROW_HEIGHT }} /> : null}
      {rows.slice(start, end).map((row, offset) => row.kind === 'turn' ? <div role="row" key={row.key} aria-rowindex={start + offset + 1}>
        <button className={css.turn} type="button" data-turn-id={row.turnId} aria-expanded={!collapsedTurns.has(row.turnId)} onClick={() => { followsTail.current = false; if (scroll.current) rememberAnchor(scroll.current.scrollTop); onToggleTurn(row.turnId) }}>
          <span aria-hidden>{collapsedTurns.has(row.turnId) ? '▸' : '▾'}</span> {t('turn', 'Turn')} <code>{row.turnId}</code>
          {collapsedTurns.has(row.turnId) && row.records.length ? <span className={css.foldSummary} data-fold-summary>{foldSummary(row.records, t)}</span> : null}<span>{row.count}</span></button>
      </div> : row.kind === 'group' ? <div role="row" key={row.key} aria-rowindex={start + offset + 1}>
        <button type="button" className={css.group} data-group={row.group} data-group-turn={row.turnId}
          aria-expanded={!(row.group === 'model' ? collapsedModels : collapsedCalls).has(row.key)}
          onClick={() => { followsTail.current = false; if (scroll.current) rememberAnchor(scroll.current.scrollTop); (row.group === 'model' ? onToggleModel : onToggleCalls)(row.key) }}>
          <span aria-hidden>{(row.group === 'model' ? collapsedModels : collapsedCalls).has(row.key) ? '▸' : '▾'}</span>
          {row.group === 'model' ? t('model', 'Model') : t('calls', 'Calls')}
          {(row.group === 'model' ? collapsedModels : collapsedCalls).has(row.key) && row.records.length ? <span className={css.foldSummary} data-fold-summary>{foldSummary(row.records, t)}</span> : null}<span>{row.count}</span>
        </button></div> : <div role="row" key={row.key} aria-rowindex={start + offset + 1}>
        <button type="button" className={css.record} data-record-id={row.record.id} data-lane={row.record.lane}
          data-dimmed={focusedIds !== null && !focusedIds.has(row.record.id) || undefined} aria-pressed={selectedId === row.record.id}
          onClick={() => onSelect(row.record)} onKeyDown={event => {
            if (!['ArrowDown', 'ArrowUp'].includes(event.key)) return
            const index = records.findIndex(record => record.id === row.record.id) + (event.key === 'ArrowDown' ? 1 : -1)
            if (records[index]) { event.preventDefault(); onSelect(records[index]) }
          }}>
          <span role="cell" className={css.event}><span className={css.dot} /><span>{t(row.record.kind, row.record.title)}</span><code>#{row.record.firstSeq}</code></span>
          <span role="cell" className={css.preview}><span>{row.record.kind === 'tool' ? row.record.title : row.record.text ?? row.record.logicalStepId ?? '—'}</span>
            <span className={css.rowStatus}>{trajectoryStatusLabel(row.record.status, t)} · {trajectoryDuration(row.record) === null ? '—' : `${trajectoryDuration(row.record)} ms`}</span></span>
        </button></div>)}
      {end < rows.length ? <div aria-hidden style={{ height: (rows.length - end) * ROW_HEIGHT }} /> : null}
      {!rows.length && !loading ? <p className={css.notice} role="status">{unfilteredRecordCount > 0
        ? t('noMatches', 'No matching public records.') : t('emptyRecords', 'No public records loaded yet.')}</p> : null}
    </div>
  </div>
}
