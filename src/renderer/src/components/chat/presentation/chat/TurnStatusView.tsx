/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import { AssistantMarkdown } from '../../AssistantMarkdown'
import type { ChatBlock } from '../../../../agent/types'
import { getProcessDetail, readRuntimeString, runtimeMetaDetailSections } from './chat-view-model'
import { RuntimeDiagnosticsAction, RuntimeMetadata } from './ToolViewActions'
import { useDisclosure } from './use-disclosure'
import { finiteCounter } from './turn-state/model'
import { CompactionItem, RetryItem, TurnErrorItem } from './turn-state/StatusItems'
import css from './turn-state/TurnState.module.css'

export function TurnStatusView({ block }: { block: Extract<ChatBlock, { kind: 'compaction' | 'system' }> }) {
  const { t, i18n } = useTranslation('common')
  const disclosure = useDisclosure(block.id, block.kind === 'compaction' && block.status === 'error')
  if (block.kind === 'compaction') {
    const replacedTokens = finiteCounter(block.meta && 'replacedTokens' in block.meta ? block.meta.replacedTokens : undefined)
    const shadowedItems = finiteCounter(block.meta?.shadowedItemCount)
    const conversation = block.summaryKind === 'conversation'
    const summary = [conversation ? '' : block.summary,
      shadowedItems === undefined ? '' : t('dshCompactionShadowedItems', { count: shadowedItems }),
      replacedTokens === undefined ? '' : t(block.meta?.tokenCountKind === 'estimated' ? 'dshCompactionEstimatedTokens' : 'dshCompactionReplacedTokens', { count: replacedTokens }),
      (block.status === 'success' || block.status === 'unknown') && !conversation ? t('dshCompactionSummaryUnavailable') : ''
    ].filter(Boolean).join(' · ')
    return <CompactionItem title={t(block.status === 'unknown' ? 'dshCompactionOutcomeUnknown' : block.status === 'running' ? 'compactionRunning' : block.status === 'error' ? 'compactionFailed'
      : block.auto ? 'compactionAutoCompleted' : 'compactionManualCompleted')} summary={summary}
      state={block.status} open={disclosure.open} onToggle={disclosure.toggle}
      detail={conversation && block.detail?.trim() ? <AssistantMarkdown text={block.detail} streaming={block.status === 'running'} /> : undefined} />
  }
  if (block.code === 'history_user_input_discarded') {
    const at = new Date(block.createdAt ?? '')
    return <div className={css.statusText} data-user-input-discard-history role="status" aria-live="off">
      {t('userInputLateDiscardedHistory')}
      {Number.isFinite(at.getTime()) ? <> · <time dateTime={block.createdAt}>{at.toLocaleString(i18n.language)}</time></> : null}
    </div>
  }
  const stage = readRuntimeString(block.meta ?? null, 'stage')
  const turnId = readRuntimeString(block.meta ?? null, 'turnId')
  const providerStage = block.meta?.stage
  const providerProgress = (providerStage === 'pre_send' || providerStage === 'post_send' || providerStage === 'response_received') &&
    (block.severity === undefined || block.severity === 'info') && !block.code && !block.meta?.providerError && !block.meta?.providerRetry
  const detail = getProcessDetail(block, block.text)
  const sections = runtimeMetaDetailSections(block, t)
  const detailText = block.detail?.trim() ? block.detail : !providerProgress && 'text' in detail ? detail.text : undefined
  const hasDetail = Boolean(detailText || stage || block.meta?.providerError || block.meta?.providerRetry) || sections.length > 0
  const body = <>
    {stage ? <dl><dt>{t('toolProviderStatus')}</dt><dd>{stage}</dd>
      {turnId ? <><dt>{t('toolChildTurn')}</dt><dd>{turnId}</dd></> : null}</dl> : null}
    {detailText ? <pre className={css.detailText}>{detailText}</pre> : null}
    {sections.map(section => <section key={section.id}><h4>{section.title}</h4><dl>
      {section.rows.map((row, index) => <div key={index}><dt>{row.label}</dt><dd>{row.value}</dd></div>)}
    </dl></section>)}
    <RuntimeDiagnosticsAction block={block} t={t} /><RuntimeMetadata block={block} t={t} />
  </>
  // A real error stays visible even when its diagnostics carry a request stage.
  if (block.severity === 'error') return <TurnErrorItem title={t('dshTurnFailed')} message={block.text} code={block.code}
    detailLabel={t('dshStatusDetails')} detail={hasDetail ? body : undefined} open={disclosure.open} onToggle={disclosure.toggle} />
  const retry = block.meta?.providerRetry
  if (retry) return <RetryItem message={block.text} code={block.code} attempt={retry.attempt} maxAttempt={retry.maxAttempt}
    phase={retry.lifecycle?.phase} delayMs={retry.lifecycle?.delayMs} deadlineUnixMs={retry.lifecycle?.deadlineUnixMs}
    detail={hasDetail ? body : undefined} t={t} />
  // Legal progress stages supply their own localized state. Retire their old
  // transcript prose, including long-text fallback, rather than folding it.
  // Real provider errors and explicit public diagnostics retain their owners.
  if (providerProgress) return null
  if (stage || block.meta?.providerError) {
    const label = stage === 'response_received' ? t('providerResponseReceivedStatus') : stage === 'pre_send' ? t('providerRequestPreparingStatus')
      : stage === 'post_send' ? t('providerRequestStartedStatus') : stage ? t('dshUnknownStage', { stage }) : undefined
    return <details data-runtime-diagnostics-stage={stage || undefined} data-runtime-provider-notice className={css.statusText} aria-label={t('dshProviderRequestDetails')}>
      <summary className={css.retrySummary}>{t('dshProviderRequestDetails')}</summary>
      <div className={css.statusDetails}>
        {label ? <p>{label}</p> : null}{!providerProgress ? <p className={css.detailText}>{block.text}</p> : null}
        {block.code ? <code>{block.code}</code> : null}
        {body}
      </div>
    </details>
  }
  return <div className={css.statusText} data-analytix-turn-notice role="status" aria-live="polite">
    {hasDetail ? <details><summary className={css.retrySummary}>{block.text}{block.code ? <code> · {block.code}</code> : null}</summary>
      <div className={css.statusDetails}>{body}</div></details> : <><span>{block.text}{block.code ? <code> · {block.code}</code> : null}</span>
        <RuntimeDiagnosticsAction block={block} t={t} /><RuntimeMetadata block={block} t={t} /></>}
  </div>
}
