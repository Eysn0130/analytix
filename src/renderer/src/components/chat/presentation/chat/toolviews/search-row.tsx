/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ToolBlock } from '../../../../../agent/types'
import { SearchBlock, type SearchBlockLabels } from '../../SearchBlock'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import { ToolRow, type ToolRowState } from '../ToolRow'
import type { ToolViewProps } from '../ToolViews'
import { toolRowState } from '../models/tool-call-model'
import { CHAT_SEARCH_MAX_LINES, searchCardInput, searchCardModel } from '../models/search-card-model'
import css from '../ToolRow.module.css'
import searchCss from '../../SearchBlock.module.css'

// Pinned DSH regular artwork; this leaf does not alter the shared icon registry.
function SearchIcon() {
  return <svg width={14} height={14} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
    <path d="M6.58727 11.8586C9.55061 11.8586 11.9529 9.45637 11.9529 6.49304C11.9529 3.5297 9.55061 1.12744 6.58727 1.12744C3.62394 1.12744 1.22168 3.5297 1.22168 6.49304C1.22168 9.45637 3.62394 11.8586 6.58727 11.8586Z" stroke="currentColor" />
    <path d="M10.2991 10.3933L14.7783 14.8725" stroke="currentColor" />
  </svg>
}
function firstLine(text: string) { const index = text.indexOf('\n'); return index === -1 ? text : text.slice(0, index) }

function SearchContent({ block, display, onCopy, copied, pending, state }: { block: ToolBlock; display: ToolResultLocalDisplayV1; onCopy: () => void; copied: boolean; pending: boolean; state?: ToolRowState }) {
  const { t } = useTranslation('common'), model = searchCardModel(block, display)
  if (display.capture.kind !== 'search') return null
  const capture = display.capture, search = capture.search
  const labels: SearchBlockLabels = {
    pathsSummary: (shown, total, truncated) => t(total === null ? 'dshSearch.pathsUnknown' : truncated ? 'dshSearch.pathsTruncated' : 'dshSearch.pathsSummary', { shown, total }),
    matchesSummary: (shown, total, files, truncated) => t(total === null ? 'dshSearch.matchesUnknown' : truncated ? 'dshSearch.matchesTruncated' : 'dshSearch.matchesSummary', { shown, total, files }),
    copy: t('dshSearch.copySealed'), copied: t('copySuccess'), noResults: t('dshSearch.noResults'), collapseAria: t('dshSearch.collapseAria'),
    expandAria: hidden => t('dshSearch.expandAria', { hidden }), collapse: t('dshSearch.collapse'), expand: hidden => t('dshSearch.expand', { hidden })
  }
  return <>
    {model ? <SearchBlock search={model.search} labels={labels} className={`${css.searchBody} ${searchCss.chatBlock}`} captureTruncated={capture.truncated}
      maxLines={CHAT_SEARCH_MAX_LINES} onCopy={onCopy} copied={copied} pending={pending} />
      : capture.body !== '' ? <div className={`${css.ioCard} ${searchCss.fallbackCard}`} data-tool-io>
        <div className={css.ioSection} data-protected-tool-copy-section="result"><span className={css.ioLabel}>OUT</span>
          <span className={css.ioText} data-error={state === 'error' || undefined}>{capture.body}</span></div>
      </div> : null}
    {search.truncated ? <div className={css.searchRecovery}>{t('dshSearch.producerTruncated')}</div> : null}
    {capture.truncated ? <div className={css.searchRecovery}>{t('dshSearch.captureTruncated')}</div> : null}
    {search.truncated || capture.truncated ? <div className={css.searchRecovery} data-search-recovery>{t('dshSearch.snapshotLocator')}: {display.resultItemId} · {display.snapshotDigest}</div> : null}
  </>
}
/** Exact grep/glob leaves; other classified search tools retain their existing fallback. */
export type SearchRowProps = ToolViewProps & { renderFallback: (props: ToolViewProps) => ReactNode }
// The dispatcher supplies its existing generic consumer. This leaf must not
// import the dispatcher at runtime and close its ToolCallTree initialization cycle.
export function SearchRow(props: SearchRowProps) {
  const { block, visible, resetKey, privateGeneration, onInspect } = props, { t } = useTranslation('common')
  const name = block.meta?.toolName
  if ((name !== 'grep' && name !== 'glob') || block.toolKind !== 'tool_call') return <>{props.renderFallback(props)}</>
  const rowProps = { icon: <SearchIcon />, title: t('dshSearch.' + name), toolName: name, variant: 'search' }
  const state = toolRowState(block)
  return <div className={searchCss.scope} data-tool-view="search" data-analytix-process-block-id={block.id} data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {block.meta?.localResult && block.status !== 'running' && visible ? <ProtectedToolResult key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} preload onInspect={onInspect}
      renderContent={(display, _openWebLink, actions) => <SearchContent block={block} display={display} {...actions} />}
      renderView={controls => {
        const capture = controls.display?.capture.kind === 'search' ? controls.display.capture : null
        const input = controls.display ? searchCardInput(controls.display) : null
        const pattern = input ? firstLine(input.pattern) : undefined
        const rowState = controls.state ?? state
        const error = rowState === 'error' && capture?.body ? firstLine(capture.body) : undefined
        const summary = error ?? pattern
        return <ToolRow {...rowProps} state={rowState} summary={summary ? <span data-protected-tool-summary
          data-protected-tool-readonly={!error && input?.source === 'inspect' || undefined}
          data-protected-tool-copy-section={error ? 'result' : input?.source === 'legacy' ? 'payload' : undefined}>{summary}</span> : undefined}
          expanded={controls.open} expandable={controls.canOpen} onToggle={controls.onToggle} inspect={controls.inspect}
          actions={controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : undefined}
          body={controls.body ? { kind: 'search', content: controls.body } : undefined} />
      }} /> : props.renderFallback(props)}
  </div>
}
