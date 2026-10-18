/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useId, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolBlock } from '../../../../../agent/types'
import { recordedJobData } from '../models/recorded-tool-card-model'
import { conversationCallRecord, type ConversationCallIdentity } from '../models/conversation-record-model'
import css from './ToolResultInspector.module.css'
import type { ToolRowState } from '../ToolRow'
import { ToolRawIO } from './ToolRawIO'

type Tab = 'overview' | 'payload' | 'result' | 'schema' | 'timing'
type Section = 'payload' | 'result' | 'schema'
const TABS: readonly Tab[] = ['overview', 'payload', 'result', 'schema', 'timing']

// A large JSON result stays complete as text instead of allocating an unbounded
// component tree. This limits rendering work, never the captured result or copy.
function canRenderTree(value: object): boolean {
  const pending: { value: unknown; depth: number }[] = [{ value, depth: 0 }]
  let nodes = 0
  while (pending.length) {
    const entry = pending.pop()!
    if (++nodes > 512 || entry.depth > 16) return false
    if (typeof entry.value !== 'object' || entry.value === null) continue
    const keys = Array.isArray(entry.value) ? entry.value.length : Object.keys(entry.value).length
    if (keys > 64) return false
    for (const child of Object.values(entry.value)) pending.push({ value: child, depth: entry.depth + 1 })
  }
  return true
}

// Tree rendering is safe only when parsing preserves every JSON token. Ignore
// legal whitespace outside strings; retain escapes, key order and number spelling.
function preservesJsonText(text: string, value: unknown): boolean {
  const compact = text.replace(/"(?:\\[\s\S]|[^"\\])*"|[ \t\r\n]+/g, token => token[0] === '"' ? token : '')
  return compact === JSON.stringify(value)
}

function JsonValue({ value, label, depth = 0, wrapping }: { value: unknown; label?: string; depth?: number; wrapping: boolean }) {
  const [open, setOpen] = useState(depth < 1), [wholeString, setWholeString] = useState(false)
  const { t } = useTranslation('common')
  const object = typeof value === 'object' && value !== null
  const entries = object ? Object.entries(value) : []
  const lines = typeof value === 'string' ? value.split(/\r\n|\r|\n/, 13) : []
  const longString = lines.length > 12
  const text = longString && !wholeString ? lines.slice(0, 12).join('\n') : JSON.stringify(value)
  return <div className={css.jsonNode} data-json-node>
    <div className={css.jsonRow}>
      {object ? <button type="button" className={css.expander} aria-expanded={open}
        aria-label={t(open ? 'dshToolInspect.collapseNode' : 'dshToolInspect.expandNode')}
        onClick={() => setOpen(previous => !previous)}>{open ? '▾' : '▸'}</button> : <span className={css.expander} />}
      {label !== undefined ? <span className={css.jsonKey}>{JSON.stringify(label)}: </span> : null}
      <span className={wrapping ? css.jsonValueWrapped : css.jsonValue}>{object
        ? Array.isArray(value) ? `[${entries.length}]` : `{${entries.length}}` : text}</span>
      {longString ? <button type="button" className={css.textToggle} onClick={() => setWholeString(previous => !previous)}>
        {t(wholeString ? 'dshToolInspect.collapseText' : 'dshToolInspect.expandText')}</button> : null}
    </div>
    {object && open ? <div className={css.jsonChildren}>{entries.map(([key, child]) => <JsonValue key={key} label={key} value={child} depth={depth + 1} wrapping={wrapping} />)}</div> : null}
  </div>
}

function Payload({ value, truncated, wrapping, error = false }: { value?: string; truncated?: boolean; wrapping: boolean; error?: boolean }) {
  const { t } = useTranslation('common')
  if (value === undefined) return <p className={css.unavailable}>{t(truncated ? 'dshToolInspect.captureLimit' : 'dshToolInspect.unavailable')}</p>
  let json: unknown
  try { json = JSON.parse(value) } catch { /* Actual non-JSON tool text remains text. */ }
  const object = json !== null && typeof json === 'object'
  const tree = json !== null && typeof json === 'object' && canRenderTree(json) && preservesJsonText(value, json)
  return <div className={`${css.payload} ${error ? css.error : ''}`} data-error={error || undefined}>
    {object && !tree ? <p className={css.truncated}>{t('dshToolInspect.jsonTextView')}</p> : null}
    {tree ? <JsonValue value={json} wrapping={wrapping} />
      : <pre className={wrapping ? css.wrapped : css.unwrapped}>{value}</pre>}
    {truncated ? <p className={css.truncated}>{t('dshToolInspect.captureLimit')}</p> : null}
  </div>
}

function Schema({ value, truncated, wrapping }: { value?: string; truncated: boolean; wrapping: boolean }) {
  const { t } = useTranslation('common')
  let schema: { name: string; description: string; parameters: object } | undefined
  if (value !== undefined) {
    try {
      const parsed: unknown = JSON.parse(value)
      if (parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const object = parsed as Record<string, unknown>
        if (typeof object.name === 'string' && typeof object.description === 'string' && object.parameters !== null &&
            typeof object.parameters === 'object' && !Array.isArray(object.parameters) && canRenderTree(object.parameters) &&
            preservesJsonText(value, parsed)) {
          schema = { name: object.name, description: object.description, parameters: object.parameters }
        }
      }
    } catch { /* An actual schema that is not this DSH shape remains visible. */ }
  }
  return schema ? <div className={css.schema}>
    <header className={css.schemaIntro}><h3 className={css.schemaName}>{schema.name}</h3><p className={css.schemaDescription}>{schema.description}</p></header>
    <h4 className={css.schemaParametersTitle}>{t('dshToolInspect.parameters')}</h4>
    <div className={css.payload}><JsonValue value={schema.parameters} wrapping={wrapping} /></div>
  </div> : <Payload value={value} truncated={truncated} wrapping={wrapping} />
}

export type ToolResultInspectorPresentation = 'overlay' | 'embedded'

/** Same-window DSH details leaf. It receives only the current authorized snapshot. */
export function ToolResultInspector({ display, publicJob, publicRecord, recordIdentity, callId, onClose, onCopy, copied = false, pending = false, state, presentation = 'overlay' }: {
  callId: string; onClose: () => void; state?: ToolRowState
  recordIdentity?: ConversationCallIdentity
  presentation?: ToolResultInspectorPresentation
  onCopy?: (section: Section) => void; copied?: boolean; pending?: boolean
} & ({ display: ToolResultLocalDisplayV1; publicJob?: never; publicRecord?: never } |
  { display?: never; publicJob: ToolBlock; publicRecord?: never } | { display?: never; publicJob?: never; publicRecord: ToolBlock })) {
  const { t } = useTranslation('common'), id = useId()
  const [tab, setTab] = useState<Tab>('overview'), [wrapping, setWrapping] = useState(true), [width, setWidth] = useState(440)
  const firstTab = useRef<HTMLButtonElement>(null), pointer = useRef<{ start: number; width: number } | null>(null)
  const capture = display?.capture
  useLayoutEffect(() => {
    const pane = firstTab.current?.closest('[data-tool-result-inspector]')
    firstTab.current?.focus({ preventScroll: true })
    return () => {
      const selection = document.getSelection()
      for (let index = 0; pane && selection && index < selection.rangeCount; index++) {
        try { if (selection.getRangeAt(index).intersectsNode(pane)) { selection.removeAllRanges(); break } } catch { /* Retired range. */ }
      }
    }
  }, [])
  const job = publicJob && publicJob.meta?.callId === callId ? recordedJobData(publicJob) : null
  const metadataRecord = publicRecord && typeof publicRecord.meta?.recordThreadId === 'string'
    ? conversationCallRecord(publicRecord.meta.recordThreadId, publicRecord) : null
  const metadataOnly = Boolean(metadataRecord && metadataRecord.callId === callId)
  const inspect = display?.inspect
  const readonlyRecord = Boolean(metadataOnly || job || capture?.kind === 'goal' || capture?.kind === 'todo' || capture?.kind === 'skill' || capture?.kind === 'job' || capture?.kind === 'inspect' || capture?.kind === 'schedule' || capture?.kind === 'present' || capture?.kind === 'file_result')
  if (!inspect && !readonlyRecord && capture?.kind !== 'web_fetch' && capture?.kind !== 'search') return null
  const web = capture?.kind === 'web_fetch' ? capture.web : undefined
  const observation = capture?.kind === 'search' ? capture.search : web
  const publicBlock = publicJob ?? publicRecord
  const actualIdentity = recordIdentity ?? metadataRecord ?? (publicJob && typeof publicJob.meta?.recordThreadId === 'string' ? conversationCallRecord(publicJob.meta.recordThreadId, publicJob) : null)
  const toolName = display?.toolName ?? (typeof publicBlock?.meta?.toolName === 'string' ? publicBlock.meta.toolName : '')
  const resultItemId = display?.resultItemId ?? actualIdentity?.toolResultItemId ?? (typeof publicJob?.meta?.sourceItemId === 'string' ? publicJob.meta.sourceItemId : '')
  const resultError = state !== undefined ? state === 'error' : capture && capture.status !== 'canceled' && capture.status !== 'unknown' &&
    (capture.status === 'failed' || capture.status === 'timeout' || observation?.toolIsError)
  const tabs: readonly Tab[] = metadataOnly ? ['overview'] : inspect ? TABS : readonlyRecord ? ['overview', 'result'] : TABS
  const activeTab = tabs.includes(tab) ? tab : 'overview'
  const clampWidth = (value: number) => Math.max(256, Math.min(760, window.innerWidth - 32, value))
  const section: Section | null = activeTab === 'payload' || activeTab === 'result' || activeTab === 'schema' ? activeTab : null
  const readonlySection = readonlyRecord || Boolean(inspect && (section === 'payload' || section === 'schema'))
  const copyable = !readonlySection && (section === 'result' || section === 'payload' && observation?.inputJSON !== undefined && !observation.inputTruncated ||
    section === 'schema' && web?.schemaJSON !== undefined && !web.schemaTruncated)
  const activateTab = (next: Tab) => setTab(next)
  const keyboardTabs = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (!['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(event.key)) return
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length
    event.preventDefault(); activateTab(tabs[next]); document.getElementById(`${id}-${tabs[next]}`)?.focus()
  }
  const unavailable = t('dshToolInspect.unavailable')
  let body: ReactNode
  if (activeTab === 'overview') body = <dl className={css.overview}>
    <div><dt>{t('dshToolInspect.tool')}</dt><dd>{toolName}</dd></div>
    <div><dt>{t('dshToolInspect.call')}</dt><dd>{callId}</dd></div>
    {actualIdentity ? <><div><dt>{t('dshRecords.thread')}</dt><dd>{actualIdentity.threadId}</dd></div>
      <div><dt>{t('dshRecords.turn')}</dt><dd>{actualIdentity.turnId}</dd></div>
      {actualIdentity.toolCallItemId ? <div><dt>{t('dshRecords.callItem')}</dt><dd>{actualIdentity.toolCallItemId}</dd></div> : null}</> : null}
    {resultItemId ? <div><dt>{t('dshToolInspect.resultItem')}</dt><dd>{resultItemId}</dd></div> : null}
    {display && (readonlyRecord || capture?.kind === 'search') ? <div><dt>{t('dshSearch.snapshotLocator')}</dt><dd>{display.snapshotDigest}</dd></div> : null}
    <div><dt>{t('dshToolInspect.status')}</dt><dd>{capture?.status ?? publicJob?.meta?.publicResult?.status ?? publicRecord?.status}</dd></div>
    {metadataOnly ? <div><dt>{t('dshToolInspect.result')}</dt><dd>{t('dshToolInspect.unavailable')}</dd></div> : null}
    {web?.requestUrl !== undefined ? <div><dt>{t('dshToolInspect.requestUrl')}</dt><dd data-protected-tool-copy-section="request-url">{web?.requestUrl}</dd></div> : null}
    {web?.finalUrl !== undefined ? <div><dt>{t('dshToolInspect.finalUrl')}</dt><dd data-protected-tool-copy-section="result-url">{web?.finalUrl}</dd></div> : null}
    {web?.title !== undefined ? <div><dt>{t('dshToolInspect.title')}</dt><dd>{web?.title}</dd></div> : null}
    {web?.statusCode !== undefined ? <div><dt>HTTP</dt><dd>{web?.statusCode}</dd></div> : null}
    {web?.error ? <div><dt>{t('dshToolInspect.error')}</dt><dd className={css.error}>{web?.error.code}: {web?.error.message}</dd></div> : null}
  </dl>
  else if (activeTab === 'payload') body = <Payload value={inspect ? inspect.inputJSON : observation?.inputJSON}
    truncated={inspect ? inspect.inputTruncated : observation?.inputTruncated} wrapping={wrapping} />
  else if (activeTab === 'result' && capture?.kind === 'inspect' && inspect?.outputText) body = <ToolRawIO outputText={inspect.outputText} />
  else if (activeTab === 'result' && readonlyRecord) {
    // Typed Result only. Common Inspect facts have their own provenance;
    // neither empty reserved body nor public prose reconstructs a result.
    const result = job ?? (capture?.kind === 'goal' ? capture.goal : capture?.kind === 'todo' ? capture.todo :
      capture?.kind === 'skill' ? { ...capture.skill, body: capture.body, truncated: capture.truncated } : capture?.kind === 'job' ? capture.job :
        capture?.kind === 'schedule' ? capture.schedule : capture?.kind === 'present' ? capture.present : capture?.kind === 'file_result' ? capture.fileResult : undefined)
    body = <Payload value={JSON.stringify(result)} truncated={capture?.truncated} wrapping={wrapping} error={Boolean(resultError)} />
  }
  else if (activeTab === 'result') body = <><Payload value={capture?.body} truncated={capture?.truncated} wrapping={wrapping} error={Boolean(resultError)} />
    {web?.fetchTruncated ? <p className={css.truncated}>{t('dshWebFetch.trunc')}</p> : null}
    {capture?.kind === 'search' && capture.search.truncated ? <p className={css.truncated}>{t('dshSearch.producerTruncated')}</p> : null}</>
  else if (activeTab === 'schema') body = <><h3 className={css.schemaParametersTitle}>{t(inspect ? 'dshToolInspect.modelSchema' : 'dshToolInspect.executionSchema')}</h3>
    {inspect ? <Schema value={inspect.schemaJSON} truncated={inspect.schemaTruncated} wrapping={wrapping} />
      : <Payload value={web?.schemaJSON} truncated={web?.schemaTruncated} wrapping={wrapping} />}</>
  else body = <dl className={css.overview}>
    <div><dt>{t('dshToolInspect.callEventAt')}</dt><dd>{inspect?.callEventAt ?? unavailable}</dd></div>
    <div><dt>{t('dshToolInspect.resultEventAt')}</dt><dd>{inspect?.resultEventAt ?? unavailable}</dd></div>
    <div><dt>{t('dshToolInspect.duration')}</dt><dd>{inspect?.durationMs !== null && inspect?.durationMs !== undefined ? inspect.durationMs + ' ms' : unavailable}</dd></div>
    {inspect ? <div><dt>{t('dshToolInspect.timingSource')}</dt><dd>{t('dshToolInspect.eventTiming')}</dd></div> : null}
    {!inspect && observation ? <><div><dt>{t('dshToolInspect.executionStarted')}</dt><dd>{observation.startedAt || unavailable}</dd></div>
      <div><dt>{t('dshToolInspect.executionDuration')}</dt><dd>{capture?.durationMs} ms</dd></div>
      <div><dt>{t('dshToolInspect.timingSource')}</dt><dd>{t('dshToolInspect.hostExecution')}</dd></div></> : null}
  </dl>
  return <aside data-tool-result-inspector data-protected-tool-body data-protected-tool-readonly={readonlyRecord || undefined} data-call-id={callId} className={css.details} data-presentation={presentation} style={presentation === 'embedded' ? undefined : { width }}
    aria-label={t('dshToolInspect.label')} onKeyDown={event => { if (event.key === 'Escape') { event.stopPropagation(); onClose() } }}>
    {presentation === 'overlay' ? <button type="button" className={css.resizeHandle} aria-label={t('dshToolInspect.resize')}
      onPointerDown={event => { pointer.current = { start: event.clientX, width }; event.currentTarget.setPointerCapture(event.pointerId); event.preventDefault() }}
      onPointerMove={event => { if (pointer.current) setWidth(clampWidth(pointer.current.width + pointer.current.start - event.clientX)) }}
      onPointerUp={() => { pointer.current = null }} onPointerCancel={() => { pointer.current = null }}
      onKeyDown={event => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); setWidth(previous => clampWidth(previous + (event.key === 'ArrowLeft' ? 24 : -24))) } }} /> : null}
    <header className={css.detailsHeader}><div className={css.detailsTitle}><span>{toolName}</span><span className={css.detailsLocation} title={callId}>{callId}</span></div>
      <button type="button" className={css.close} aria-label={t('dshToolInspect.close')} onClick={onClose}>×</button></header>
    <div className={css.detailTabs} role="tablist" aria-label={t('dshToolInspect.label')}>
      {tabs.map((item, index) => <button key={item} ref={index === 0 ? firstTab : undefined} type="button" role="tab"
        id={`${id}-${item}`} aria-controls={`${id}-panel`} aria-selected={activeTab === item} tabIndex={activeTab === item ? 0 : -1}
        className={activeTab === item ? `${css.detailTab} ${css.detailTabActive}` : css.detailTab}
        onClick={() => activateTab(item)} onKeyDown={event => keyboardTabs(event, index)}>{t('dshToolInspect.' + item)}</button>)}
    </div>
    {section ? <div className={css.actions}><button type="button" aria-pressed={wrapping} onClick={() => setWrapping(previous => !previous)}>{t('dshToolInspect.wrap')}</button>
      {!readonlySection ? <button type="button" disabled={!copyable || pending} onClick={() => onCopy?.(section)}>{t(copied ? 'copySuccess' : 'dshToolInspect.copy')}</button> : null}</div> : null}
    <div id={`${id}-panel`} className={css.detailBody} role="tabpanel" aria-labelledby={`${id}-${activeTab}`}
      data-protected-tool-readonly={readonlySection || activeTab === 'timing' || undefined}
      data-protected-tool-copy-section={readonlySection ? undefined : section ?? undefined}>{body}</div>
  </aside>
}
