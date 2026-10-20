/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * active TrajectoryView/Toolbar. Copyright (c) 2026 DeepSeek. MIT; THIRD_PARTY_NOTICES.md. */
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { trajectoryMatches, type TrajectoryRecord, type TrajectoryToolIdentity } from './trajectory-adapter'
import { trajectorySource, type TrajectorySource } from './trajectory-source'
import { TrajectoryTimeline } from './TrajectoryTimeline'
import { deriveTrajectoryTimeline, trajectoryFocusIds, type TrajectoryRange } from './trajectory-timeline-model'
import { TrajectoryTable, trajectoryGroups, type TrajectoryRevealRequest } from './TrajectoryTable'
import { TrajectoryInspector } from './TrajectoryInspector'
import css from './TrajectoryPanel.module.css'
export type TrajectoryFocusRequest = TrajectoryToolIdentity & { requestId: string }
export type TrajectoryFocusResult = 'found' | 'missing' | 'ambiguous' | 'not_loaded'
export type TrajectoryPanelProps = {
  threadId: string
  onInspectTool?: (identity: TrajectoryToolIdentity) => void
  renderToolInspector?: (identity: TrajectoryToolIdentity, close: () => void) => ReactNode
  source?: TrajectorySource
  /** Exact public identity navigation; never an instruction to capture private content. */
  focusRequest?: TrajectoryFocusRequest | null
  onFocusRequestComplete?: (requestId: string, result: TrajectoryFocusResult) => void
}
export function TrajectoryPanel(props: TrajectoryPanelProps) { return <TrajectoryPanelSeat key={props.threadId} {...props} /> }
const EMPTY_COLLAPSED: ReadonlySet<string> = new Set()
const toggled = (current: ReadonlySet<string>, id: string) => { const next = new Set(current); if (next.has(id)) next.delete(id); else next.add(id); return next }
function TrajectoryPanelSeat({ threadId, onInspectTool, renderToolInspector, source = trajectorySource, focusRequest, onFocusRequestComplete }: TrajectoryPanelProps) {
  const { t, i18n } = useTranslation('common')
  const label = (key: string, fallback: string): string => String(t(`dshTrajectory.${key}`, { defaultValue: fallback }))
  const subscribe = useCallback((listener: () => void) => source.subscribe(threadId, listener), [source, threadId])
  const getSnapshot = useCallback(() => source.getSnapshot(threadId), [source, threadId])
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  const [limit, setLimit] = useState(50), [historyTailId, setHistoryTailId] = useState<string | null>(snapshot.records.at(-1)?.id ?? null)
  const [query, setQuery] = useState(''), [duration, setDuration] = useState(false), [range, setRange] = useState<TrajectoryRange | null>(null)
  const [selectedId, setSelectedId] = useState<string | null>(null), [inspecting, setInspecting] = useState<string | null>(null)
  const [collapsedTurns, setCollapsedTurns] = useState<ReadonlySet<string>>(new Set())
  const [collapsedModels, setCollapsedModels] = useState<ReadonlySet<string>>(new Set())
  const [collapsedCalls, setCollapsedCalls] = useState<ReadonlySet<string>>(new Set())
  const [reveal, setReveal] = useState<TrajectoryRevealRequest | null>(null)
  const revealVersion = useRef(0), handledFocus = useRef<string | null>(null), pendingFocus = useRef<{ requestId: string; recordId: string; version: number } | null>(null), expandedReveal = useRef<number | null>(null)
  useEffect(() => {
    if (snapshot.history !== 'loading' && snapshot.records.length && (!historyTailId || !snapshot.records.some(record => record.id === historyTailId))) setHistoryTailId(snapshot.records.at(-1)!.id)
    const retained = new Set(snapshot.records.map(record => record.turnId))
    for (const update of [setCollapsedTurns]) update(current => {
      const next = new Set([...current].filter(id => retained.has(id)))
      return next.size === current.size ? current : next
    })
  }, [snapshot, historyTailId])
  const tailIndex = historyTailId ? snapshot.records.findIndex(record => record.id === historyTailId) : snapshot.records.length - 1
  const residentStart = Math.max(0, (tailIndex >= 0 ? tailIndex : snapshot.records.length - 1) + 1 - limit)
  const resident = useMemo(() => snapshot.records.slice(residentStart), [snapshot.records, residentStart])
  const sequenceOffset = useMemo(() => snapshot.records.slice(0, residentStart).filter(record => record.kind !== 'message').length, [snapshot.records, residentStart])
  const coordinateEpoch = snapshot.records[0]?.id ?? ''
  const timeline = useMemo(() => deriveTrajectoryTimeline(resident, duration, sequenceOffset, snapshot.records), [resident, duration, sequenceOffset, snapshot.records])
  const groups = useMemo(() => trajectoryGroups(resident), [resident])
  useEffect(() => { setRange(null) }, [coordinateEpoch])
  useEffect(() => {
    const full = trajectoryGroups(snapshot.records), allowed = new Set([...full.modelIds, ...full.callIds, ...groups.modelIds, ...groups.callIds])
    for (const update of [setCollapsedModels, setCollapsedCalls]) update(current => { const next = new Set([...current].filter(id => allowed.has(id))); return next.size === current.size ? current : next })
  }, [snapshot.records, groups])
  const focusedIds = useMemo(() => trajectoryFocusIds(timeline, range), [timeline, range])
  const matches = useMemo(() => resident.filter(record => trajectoryMatches(record, query)), [resident, query])
  const searchMatches = useMemo(() => query.trim() ? new Set(matches.map(record => record.id)) : null, [query, matches])
  const selected = resident.find(record => record.id === selectedId) ?? null
  useEffect(() => { if (!selected || snapshot.history === 'revoked' || selected.ambiguousTool) setInspecting(null) }, [selected, snapshot.history])
  const expand = (record: TrajectoryRecord) => {
    setCollapsedTurns(current => { if (!current.has(record.turnId)) return current; const next = new Set(current); next.delete(record.turnId); return next })
    const groupId = groups.byRecord.get(record.id)
    if (!groupId) return
    const update = record.kind === 'tool' ? setCollapsedCalls : setCollapsedModels
    update(current => { if (!current.has(groupId)) return current; const next = new Set(current); next.delete(groupId); return next })
  }
  useEffect(() => {
    if (!reveal || expandedReveal.current === reveal.version) return
    const record = resident.find(record => record.id === reveal.recordId)
    if (!record) return
    expandedReveal.current = reveal.version; expand(record)
  }, [reveal, resident, groups])
  const select = (record: TrajectoryRecord) => {
    setSelectedId(record.id); setInspecting(null); expand(record)
    if (focusedIds && !focusedIds.has(record.id)) setRange(null)
    setReveal({ recordId: record.id, version: ++revealVersion.current })
  }
  useEffect(() => {
    if (!focusRequest || handledFocus.current === focusRequest.requestId || snapshot.history === 'loading') return
    handledFocus.current = focusRequest.requestId
    pendingFocus.current = null
    const request = focusRequest
    if (request.threadId !== threadId) { onFocusRequestComplete?.(request.requestId, 'missing'); return }
    const found = snapshot.records.filter(record => record.tool?.threadId === request.threadId && record.tool.turnId === request.turnId && record.tool.callId === request.callId &&
      (!request.toolCallItemId || record.tool.toolCallItemId === request.toolCallItemId) && (!request.toolResultItemId || record.tool.toolResultItemId === request.toolResultItemId))
    if (found.length !== 1 || found[0].ambiguousTool) {
      onFocusRequestComplete?.(request.requestId, found.length > 1 || found[0]?.ambiguousTool ? 'ambiguous'
        : snapshot.truncated || snapshot.completeThroughSeq === null ? 'not_loaded' : 'missing')
      return
    }
    const record = found[0], index = snapshot.records.indexOf(record)
    setLimit(current => Math.max(current, Math.ceil(Math.max(1, tailIndex + 1 - index) / 50) * 50))
    setQuery(''); setRange(null); setSelectedId(record.id); setInspecting(null); expand(record)
    const version = ++revealVersion.current
    pendingFocus.current = { requestId: request.requestId, recordId: record.id, version }
    setReveal({ recordId: record.id, version })
  }, [focusRequest, snapshot, threadId, tailIndex, onFocusRequestComplete])
  useEffect(() => {
    const pending = pendingFocus.current
    if (pending && !snapshot.records.some(record => record.id === pending.recordId)) {
      pendingFocus.current = null; onFocusRequestComplete?.(pending.requestId, 'not_loaded')
    }
  }, [snapshot, onFocusRequestComplete])
  const revealed = useCallback((request: TrajectoryRevealRequest) => {
    if (pendingFocus.current?.version !== request.version) return
    onFocusRequestComplete?.(pendingFocus.current.requestId, 'found'); pendingFocus.current = null
  }, [onFocusRequestComplete])
  const turns = [...new Set(resident.map(record => record.turnId))]
  const callGroups = groups.callIds
  const allTurnsCollapsed = turns.length > 0 && turns.every(id => collapsedTurns.has(id))
  const allCallsCollapsed = callGroups.length > 0 && callGroups.every(id => collapsedCalls.has(id))
  const inspect = selected?.tool?.toolResultItemId && !selected.ambiguousTool && (onInspectTool || renderToolInspector) ? selected.tool : null
  const loadingEarlier = snapshot.olderHistory === 'loading'
  const hasEarlier = residentStart > 0 || Boolean(snapshot.canLoadOlder)
  const loadEarlier = () => {
    if (residentStart > 0) { setLimit(current => current + 50); return }
    void source.loadOlder(threadId).then(loaded => {
      if (loaded) { setLimit(50); setHistoryTailId(null); setSelectedId(null); setInspecting(null); setRange(null) }
    })
  }
  const showLatest = () => {
    source.showLatest(threadId); setLimit(50); setHistoryTailId(null); setSelectedId(null); setInspecting(null); setRange(null)
  }
  if (snapshot.history === 'revoked') return <section className={css.panel} aria-label={label('title', 'Trajectory')}><p role="status" className={css.notice}>{label('revoked', 'Public history is unavailable.')}</p></section>
  return <section className={css.panel} lang={i18n.resolvedLanguage ?? i18n.language} aria-label={label('title', 'Trajectory')} data-trajectory-thread={threadId} data-conversation-composer-overlay="">
    <header className={css.toolbar} role="toolbar" aria-label={label('controls', 'Trajectory controls')}>
      <div className={css.toolbarInner}><div className={css.toolbarActions}>
        <button type="button" className={css.toggle} aria-label={label('useActualDuration', 'Use actual duration')} aria-pressed={duration}
          onClick={() => { setDuration(value => !value); setRange(null) }} title={duration ? label('useEqualWidth', 'Use equal-width operations') : label('useActualDuration', 'Use actual duration')}>
          <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.25" aria-hidden><circle cx="8" cy="8" r="5.25" /><path d="M8 4.75V8l2.25 1.5" /></svg>{label('duration', 'Duration')}</button>
        <button type="button" className={css.toolbarAction} aria-pressed={allTurnsCollapsed} aria-label={allTurnsCollapsed ? label('expandTurns', 'Expand turns') : label('collapseTurns', 'Collapse turns')}
          title={allTurnsCollapsed ? label('expandTurns', 'Expand turns') : label('collapseTurns', 'Collapse turns')}
          onClick={() => setCollapsedTurns(allTurnsCollapsed ? new Set() : new Set(turns))}><span className={css.toolbarActionIcon} aria-hidden>{allTurnsCollapsed ? '⊞' : '⊟'}</span>{label('turns', 'Turns')}</button>
        <button type="button" className={css.toolbarAction} aria-pressed={allCallsCollapsed} aria-label={allCallsCollapsed ? label('expandCalls', 'Expand calls') : label('collapseCalls', 'Collapse calls')}
          title={allCallsCollapsed ? label('expandCalls', 'Expand calls') : label('collapseCalls', 'Collapse calls')}
          onClick={() => setCollapsedCalls(allCallsCollapsed ? new Set() : new Set(callGroups))}><span className={css.toolbarActionIcon} aria-hidden>{allCallsCollapsed ? '⊞' : '⊟'}</span>{label('calls', 'Calls')}</button>
        {snapshot.historyWindow === 'older' ? <button type="button" className={css.toolbarAction} onClick={showLatest}>{label('latest', i18n.language.startsWith('zh') ? '返回最新轨迹' : 'Return to latest')}</button> : null}
      </div>
      <label className={css.search}><svg width="11" height="11" viewBox="0 0 16 16" fill="none" stroke="currentColor" aria-hidden><circle cx="6.5" cy="6.5" r="4.5" /><path d="m10 10 4 4" /></svg>
        <input className={css.searchInput} type="search" aria-label={label('search', 'Search loaded public records')} value={query} onChange={event => setQuery(event.target.value)} placeholder={label('search', 'Search loaded public records')} /></label></div>
    </header>
    <TrajectoryTimeline records={resident} duration={duration} range={range} selectedId={selectedId} revealVersion={reveal?.version}
      searchMatches={searchMatches} sequenceOffset={sequenceOffset} coordinateEpoch={coordinateEpoch} coordinateRecords={snapshot.records} loading={snapshot.history === 'loading' || loadingEarlier} hasEarlier={hasEarlier} onLoadEarlier={loadEarlier} onSelect={select} t={label}
      onRangeChange={next => {
        setRange(next)
        if (!next) return
        const first = timeline.spans.find(span => span.start <= next.end && span.end >= next.start)
        if (first) { expand(first.record); setReveal({ recordId: first.record.id, version: ++revealVersion.current }) }
      }} />
    <div className={css.scope} role="status"><span>{label('loaded', 'Loaded')} #{resident[0]?.firstSeq ?? '—'}–#{snapshot.loadedThroughSeq ?? '—'} · {resident.length}/{snapshot.records.length}</span>
      <span>{snapshot.truncated ? label('retainedOnly', 'Earlier history is not retained; search covers loaded previews only') : snapshot.completeThroughSeq === null ? label('incomplete', 'History completeness unknown') : `${label('completeThrough', 'History through')} #${snapshot.completeThroughSeq}`}
        {snapshot.history === 'loading' || loadingEarlier ? ` · ${label('loading', 'Loading')}` : snapshot.history === 'partial' && snapshot.historyWindow !== 'older' ? ` · ${label('partial', 'Replay incomplete')}` : ''}{snapshot.olderHistory === 'unavailable' && (snapshot.truncated || snapshot.completeThroughSeq === null) ? ` · ${label('olderUnavailable', i18n.language.startsWith('zh') ? '更早的公开历史当前不可用' : 'Earlier public history is currently unavailable')}` : ''}{snapshot.historyWindow === 'older' ? ` · ${label('olderWindow', i18n.language.startsWith('zh') ? '有界历史窗口；返回最新轨迹以查看后续记录' : 'Bounded history window; return to latest for newer records')}` : ''}</span></div>
    <div className={css.body}>
      <TrajectoryTable records={matches} unfilteredRecordCount={resident.length} groupIds={groups.byRecord} selectedId={selectedId} focusedIds={focusedIds} onSelect={select} t={label}
        collapsedTurns={searchMatches === null ? collapsedTurns : EMPTY_COLLAPSED}
        collapsedModels={searchMatches === null ? collapsedModels : EMPTY_COLLAPSED} collapsedCalls={searchMatches === null ? collapsedCalls : EMPTY_COLLAPSED}
        onToggleTurn={id => setCollapsedTurns(current => toggled(current, id))} onToggleModel={id => setCollapsedModels(current => toggled(current, id))}
        onToggleCalls={id => setCollapsedCalls(current => toggled(current, id))} hasEarlier={hasEarlier} onLoadEarlier={loadEarlier}
        loading={snapshot.history === 'loading' || loadingEarlier} historyStartSeq={resident[0]?.firstSeq ?? 0} reveal={reveal} onRevealed={revealed} />
      {selected ? <TrajectoryInspector record={selected} t={label} onClose={() => { setSelectedId(null); setInspecting(null) }}
        canInspect={Boolean(inspect)} onInspect={() => { if (inspect) { setInspecting(selected.id); onInspectTool?.(inspect) } }}
        onRetireInspect={() => setInspecting(null)}
        protectedContent={inspect && inspecting === selected.id && renderToolInspector ? renderToolInspector(inspect, () => setInspecting(null)) : undefined} /> : null}
    </div>
  </section>
}
