/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useId, type KeyboardEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { TextShimmer, useTextShimmerDecorativeCopy } from '../TextShimmer'
import { DshInspectOutlineRegular } from '../DshToolIcons'
import { DisclosureRow } from '../DisclosureRow'
import styles from './ToolRow.module.css'

export type ToolRowState = 'preparing' | 'running' | 'ok' | 'error' | 'stopped' | 'unknown'
export type ToolRowBody = { kind: 'details' | 'read' | 'diff' | 'terminal' | 'search' | 'web' | 'image' | 'code'; content: ReactNode }
  | { kind: 'io'; input?: string; output?: string }
export type ToolRowProps = {
  icon?: ReactNode; title: string; summary?: ReactNode; summarySuffix?: ReactNode; running?: boolean; error?: boolean
  state?: ToolRowState; toolName?: string; variant?: string; errorSummary?: string
  expanded?: boolean; expandable?: boolean; onToggle?: () => void
  actions?: ReactNode; children?: ReactNode; body?: ToolRowBody; externalBodyId?: string; diffStats?: { added: number; removed: number }
  filePath?: string; filePathLine?: number; fileLine?: number; onOpenFile?: (path: string, line?: number) => void; onOpenFileInEditor?: (path: string, line?: number) => void
  href?: string; onOpenLink?: () => void; inspect?: { label: string; run: () => void }; keepSummaryWhenOpen?: boolean; contentClassName?: string
}
/** Keep the real entry once; the highlight twin owns only its presentational label. */
function ToolSummaryLink({ label, children }: { label: ReactNode; children: ReactNode }) {
  const decorative = useTextShimmerDecorativeCopy()
  return decorative ? <span className={styles.fileLink}><TextShimmer>{label}</TextShimmer></span> : children
}

export function ToolRow({ icon, title, summary, summarySuffix, running = false, error = false, state: suppliedState, toolName, variant,
  expanded = false, expandable = false, onToggle, actions, children, body, externalBodyId, diffStats, filePath, filePathLine, fileLine: legacyFileLine, onOpenFile,
  onOpenFileInEditor, href, onOpenLink, inspect, errorSummary, keepSummaryWhenOpen = true, contentClassName }: ToolRowProps) {
  const { t } = useTranslation('common')
  const bodyId = useId()
  const fileLine = filePathLine ?? legacyFileLine
  const state = suppliedState ?? (error ? 'error' : running ? 'running' : 'ok')
  const failed = state === 'error' || state === 'stopped' || state === 'unknown'
  const canExpand = state !== 'preparing' && expandable
  const stopKey = (event: KeyboardEvent<HTMLElement>) => { if (event.key === 'Enter' || event.key === ' ') event.stopPropagation() }
  const fileLabel = filePath && fileLine ? `${filePath}:${fileLine}` : filePath
  const fileLink = filePath && onOpenFile && !failed && state !== 'preparing' ? <ToolSummaryLink label={fileLabel}><button type="button" className={styles.fileLink}
    data-tool-file-link onMouseDown={event => event.stopPropagation()} onKeyDown={stopKey}
    onClick={event => { event.preventDefault(); event.stopPropagation(); onOpenFile(filePath, fileLine) }}
    onDoubleClick={onOpenFileInEditor ? event => { event.preventDefault(); event.stopPropagation(); onOpenFileInEditor(filePath, fileLine) } : undefined}>{fileLabel}</button></ToolSummaryLink> : null
  const failureSummary = state === 'error' && errorSummary !== undefined ? errorSummary.split('\n')[0] : undefined
  let summaryContent: ReactNode = failureSummary ?? summary
  if (fileLink && typeof summaryContent === 'string' && filePath) {
    const index = summaryContent.indexOf(filePath)
    summaryContent = index >= 0 ? <>{summaryContent.slice(0, index)}{fileLink}{summaryContent.slice(index + filePath.length)}</> : <>{fileLink}{summaryContent}</>
  } else if (fileLink) summaryContent = <>{fileLink}{summaryContent}</>
  else if (onOpenLink && !failed && state !== 'preparing' && summaryContent) summaryContent = <ToolSummaryLink label={summaryContent}><button type="button" role="link" className={styles.fileLink}
    data-tool-web-link onMouseDown={event => event.stopPropagation()} onKeyDown={stopKey}
    onClick={event => { event.preventDefault(); event.stopPropagation(); onOpenLink() }}>{summaryContent}</button></ToolSummaryLink>
  else if (href && !failed && state !== 'preparing' && summaryContent) summaryContent = <ToolSummaryLink label={summaryContent}><a className={styles.fileLink} href={href} target="_blank" rel="noopener noreferrer"
    onMouseDown={event => event.stopPropagation()} onKeyDown={stopKey} onClick={event => event.stopPropagation()}>{summaryContent}</a></ToolSummaryLink>
  const collapsed = <>
    {summaryContent ? <><span className={styles.sep} data-shimmer-decoration aria-hidden /><span className={`${styles.summary} ${state === 'error' ? styles.errorSummary : state === 'stopped' ? styles.stoppedSummary : ''}`}><TextShimmer>{summaryContent}</TextShimmer></span></> : null}
    {!failed && summarySuffix ? <TextShimmer className={styles.summarySuffix}>{summarySuffix}</TextShimmer> : null}
    {!failed && diffStats ? <TextShimmer className={`${styles.summarySuffix} ${styles.diffStat}`}><span data-analytix-diff-stats>
      <span className={styles.diffAdded}>+{diffStats.added}</span>{' '}<span className={styles.diffRemoved}>-{diffStats.removed}</span>
    </span></TextShimmer> : null}
  </>
  const content = body?.kind === 'io' ? <div className={styles.ioCard}>
    {body.input ? <div className={styles.ioSection}><span className={styles.ioLabel}>IN</span><span className={styles.ioText}>{body.input}</span></div> : null}
    {body.input && body.output ? <div className={styles.ioDivider} /> : null}
    {body.output ? <div className={styles.ioSection}><span className={styles.ioLabel}>OUT</span><span className={styles.ioText} data-error={state === 'error' || undefined}>{body.output}</span></div> : null}
  </div> : body?.kind === 'search' ? body.content : body ? <div className={styles[body.kind + 'Body']}>{body.content}</div> : children
  const statusLabel = state === 'error' ? t('protectedToolFailed') : state === 'stopped' ? t('dshTurnStopped') : state === 'preparing' ? t('protectedToolLoading') : state === 'running' ? t('inspectorStatusRunning') : state === 'unknown' ? t('protectedToolUnknown') : ''
  return <div className={styles.root} data-tool={toolName} data-variant={variant} data-state={state}>
    {statusLabel ? <span className={styles.visuallyHidden}>{statusLabel}</span> : null}
    <DisclosureRow rowClassName={styles.row} titleClassName={styles.title} leadingClassName={styles.leading} contentClassName={contentClassName}
      icon={icon} title={title} running={(state === 'running' || state === 'preparing') && !failed} error={state === 'error'}
      open={expanded && canExpand} expandable={canExpand} onToggle={() => onToggle?.()} expandOnRowClick
      bodyId={externalBodyId ?? bodyId} collapsedContent={collapsed} keepContentWhenOpen={keepSummaryWhenOpen} actions={actions}>
      {!externalBodyId ? <div id={bodyId} className={styles.bodyWrap}>
        {content}
        {expanded && inspect ? <button type="button" className={styles.inspectButton} onClick={inspect.run}><DshInspectOutlineRegular />{inspect.label}</button> : null}
      </div> : null}
    </DisclosureRow>
  </div>
}
