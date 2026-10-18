import type { ReactElement } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatBlock } from '../../../../agent/types'
import { CircleDot } from '../../../../design/AnalytixUiIcons'
import { dispatchRuntimeDiagnosticsFocus } from '../../../../lib/runtime-diagnostics-focus'
import { InjectedMemoryMetaChip } from '../../injected-memory-meta-chip'
import { formatChildAgentChip, formatJobDiagnosticsChip, formatProviderDiagnosticsChip } from '../../message-timeline-tools'
import { jobDiagnosticsFromMeta, hasRuntimeDiagnosticsSignal, readRuntimeString, runtimeDiagnosticsStatusForFocus,
  runtimeMetaDetailSections, readMetaSources, readMetaStringArray } from './chat-view-model'
import css from './ToolRow.module.css'
export function RuntimeDiagnosticsAction({
  block,
  t
}: {
  block: ChatBlock
  t: (key: string, opts?: Record<string, unknown>) => string
}): ReactElement | null {
  const diagnostics = block.kind === 'tool' || block.kind === 'system'
    ? jobDiagnosticsFromMeta(block.meta)
    : null
  if (!diagnostics || !hasRuntimeDiagnosticsSignal(diagnostics)) return null
  const label = t('openRuntimeDiagnostics')
  return (
    <button
      type="button"
      className={css.diagnosticAction}
      aria-label={label}
      title={label}
      onClick={(event) => {
        event.preventDefault()
        event.stopPropagation()
        dispatchRuntimeDiagnosticsFocus({
          jobId: readRuntimeString(diagnostics, 'jobId') || readRuntimeString(diagnostics, 'childRunId'),
          deliveryId: readRuntimeString(diagnostics, 'deliveryId'),
          childThreadId: readRuntimeString(diagnostics, 'childThreadId'),
          childTurnId: readRuntimeString(diagnostics, 'childTurnId'),
          parentThreadId: readRuntimeString(diagnostics, 'parentThreadId'),
          parentTurnId: readRuntimeString(diagnostics, 'parentTurnId'),
          sourceBlockId: block.id,
          status: runtimeDiagnosticsStatusForFocus(diagnostics)
        })
      }}
    >
      <CircleDot className="h-3.5 w-3.5" strokeWidth={1.9} />
    </button>
  )
}

export function RuntimeMetadataDetails({ block }: { block: ChatBlock }): ReactElement | null {
  const { t } = useTranslation('common')
  const sections = runtimeMetaDetailSections(block, t)
  if (sections.length === 0) return null
  return (
    <div className={css.metadataDetails}>
      {sections.map((section) => (
        <div key={section.id} className={css.metadataSection}>
          <div className={css.metadataTitle}>{section.title}</div>
          <dl className={css.metadataGrid}>
            {section.rows.map((row) => (
              <div key={`${section.id}-${row.label}`} className="contents">
                <dt className="text-ds-faint">{row.label}</dt>
                <dd className="min-w-0 break-all font-mono text-ds-ink">{row.value}</dd>
              </div>
            ))}
          </dl>
        </div>
      ))}
    </div>
  )
}

export function RuntimeMetadata({
  block,
  t
}: {
  block: ChatBlock
  t: (key: string, opts?: Record<string, unknown>) => string
}): ReactElement | null {
  const meta =
    block.kind === 'tool' || block.kind === 'approval' || block.kind === 'user' || block.kind === 'system'
      ? block.meta
      : undefined
  if (!meta) return null
  const sources = readMetaSources(meta)
  const attachmentIds = readMetaStringArray(meta, 'attachmentIds')
  const activeSkillIds = readMetaStringArray(meta, 'activeSkillIds')
  const injectedMemoryIds = readMetaStringArray(meta, 'injectedMemoryIds')
  const child = meta.child && typeof meta.child === 'object' ? meta.child as Record<string, unknown> : null
  const childChip = formatChildAgentChip(child, t)
  const jobDiagnosticsChip = formatJobDiagnosticsChip(meta, t)
  const providerDiagnosticsChip = formatProviderDiagnosticsChip(meta, t)
  if (
    sources.length === 0 &&
    attachmentIds.length === 0 &&
    activeSkillIds.length === 0 &&
    injectedMemoryIds.length === 0 &&
    !childChip &&
    !jobDiagnosticsChip &&
    !providerDiagnosticsChip
  ) {
    return null
  }
  const chipClass = css.metadataItem
  return (
    <div className={css.metadata}>
      {childChip ? (
        <span className={chipClass} title={childChip.title}>
          <span>{t('toolChildAgent')}</span>
          <span className="max-w-28 truncate font-mono text-ds-muted">{childChip.label}</span>
          {childChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{childChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {jobDiagnosticsChip ? (
        <span className={chipClass} title={jobDiagnosticsChip.title}>
          <span>{jobDiagnosticsChip.label}</span>
          {jobDiagnosticsChip.detail ? (
            <span className="max-w-48 truncate text-ds-muted">{jobDiagnosticsChip.detail}</span>
          ) : null}
        </span>
      ) : null}
      {providerDiagnosticsChip ? (
        <span className={chipClass} title={providerDiagnosticsChip.title}>
          <span>{providerDiagnosticsChip.label}</span>
          <span className="max-w-72 truncate text-ds-muted">{providerDiagnosticsChip.detail}</span>
        </span>
      ) : null}
      {activeSkillIds.length > 0 ? (
        <span className={chipClass} title={activeSkillIds.join(', ')}>
          {t('toolActiveSkills')} {activeSkillIds.length}
        </span>
      ) : null}
      {injectedMemoryIds.length > 0 ? (
        <InjectedMemoryMetaChip meta={meta} memoryIds={injectedMemoryIds} chipClass={chipClass} />
      ) : null}
      {attachmentIds.length > 0 ? (
        <span className={chipClass} title={attachmentIds.join(', ')}>
          {t('toolAttachments')} {attachmentIds.length}
        </span>
      ) : null}
      {sources.slice(0, 4).map((source, index) =>
        source.url ? (
          <a
            key={`${source.url}-${index}`}
            href={source.url}
            target="_blank"
            rel="noreferrer"
            className={chipClass}
            title={source.url}
          >
            {t('toolSources')} {index + 1}
            <span className="max-w-32 truncate text-ds-muted">{source.title || source.url}</span>
          </a>
        ) : (
          <span key={`${source.title}-${index}`} className={chipClass} title={source.title}>
            {t('toolSources')} {index + 1}
          </span>
        )
      )}
    </div>
  )
}
