/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { Fragment, useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import type { ProviderAttemptTokenAggregate, ThreadUsageSnapshot } from '../../../../agent/types'
import type { ProviderBillingV1, ConversationMetricsV1, ReportedTokenCountV1 } from '../../../../../../../packages/runtime/src/contracts/usage'
import { DshDatabaseOutlineRegular, DshGaugeOutlineRegular } from '../DshToolIcons'
import { normalizePerformanceUsageMode, type AppSettingsV1, type PerformanceUsageMode } from '@shared/app-settings'
import { rendererRuntimeClient } from '../../../../agent/runtime-client'
import { SETTINGS_CHANGED_EVENT } from '../../../../lib/keyboard-shortcut-settings'
import { useAnchoredPanel } from '../useAnchoredPanel'
import panelCss from '../AnchoredPanel.module.css'
import css from './TurnUsagePanel.module.css'

export type { PerformanceUsageMode } from '@shared/app-settings'

const PERFORMANCE_USAGE_CHOICE_EVENT = 'analytix:performance-usage-choice'
let displayMode: PerformanceUsageMode = 'detailed'
let displayRevision = 0
let pendingDisplayChoice = false

/** Volatile display policy only. Persistence belongs to normal SettingsView save/readback. */
export function publishPerformanceUsageChoice(mode: PerformanceUsageMode): void {
  displayMode = normalizePerformanceUsageMode(mode)
  pendingDisplayChoice = true
  displayRevision += 1
  window.dispatchEvent(new CustomEvent(PERFORMANCE_USAGE_CHOICE_EVENT))
}

/** Adopt a fresh normal Host readback without fabricating a successful save event. */
export function adoptPerformanceUsageHostReadback(settings: AppSettingsV1): void {
  displayMode = normalizePerformanceUsageMode(settings.performanceUsageMode)
  pendingDisplayChoice = false
  displayRevision += 1
  window.dispatchEvent(new CustomEvent(PERFORMANCE_USAGE_CHOICE_EVENT))
}

export function usePerformanceUsageMode(): PerformanceUsageMode {
  const [mode, setMode] = useState(displayMode)
  useEffect(() => {
    let disposed = false
    const startRevision = displayRevision
    const readChoice = (): void => { if (!disposed) setMode(displayMode) }
    const accept = adoptPerformanceUsageHostReadback
    const accepted = (event: Event): void => accept((event as CustomEvent<AppSettingsV1>).detail)
    window.addEventListener(PERFORMANCE_USAGE_CHOICE_EVENT, readChoice)
    window.addEventListener(SETTINGS_CHANGED_EVENT, accepted)
    readChoice()
    void rendererRuntimeClient.getSettings().then(settings => {
      if (disposed || displayRevision !== startRevision || pendingDisplayChoice) return
      accept(settings)
    }).catch(() => undefined)
    return () => {
      disposed = true
      window.removeEventListener(PERFORMANCE_USAGE_CHOICE_EVENT, readChoice)
      window.removeEventListener(SETTINGS_CHANGED_EVENT, accepted)
    }
  }, [])
  return mode
}

/** Display-only contract. Missing billing buckets and historical routes stay unknown. */
export type TurnTokenAccounting = {
  inputTokens: number
  outputTokens: number
  totalTokens: number
  uncachedInputTokens?: number
  cacheReadTokens?: number
  cacheWriteTokens?: number
  reasoningTokens?: number
  routes?: readonly { provider: string; model: string }[]
}

const counter = (value: unknown): value is number => typeof value === 'number' &&
  Number.isSafeInteger(value) && value >= 0 && !Object.is(value, -0)

/** Consume the existing physical-attempt summary, never LastResult token fields. */
export function turnAccountingFromSnapshot(snapshot?: ThreadUsageSnapshot): TurnTokenAccounting | undefined {
  const diagnostics = snapshot?.cacheDiagnostics
  const attempts = diagnostics?.providerAttemptCount
  if (diagnostics?.providerAttemptTelemetryValid !== true || !counter(attempts) || attempts === 0) return undefined
  const complete = (value?: ProviderAttemptTokenAggregate): value is ProviderAttemptTokenAggregate & { value: number } =>
    value?.complete === true && value.knownObservationCount === attempts && value.observationCount === attempts && counter(value.value)
  const input = diagnostics.providerAttemptInputTokens
  const output = diagnostics.providerAttemptOutputTokens
  if (!complete(input) || !complete(output) || !counter(input.value + output.value)) return undefined
  // Public cache-hit/miss aggregates do not prove the disjoint uncached/read/write
  // accounting contract. Nor does the final model identify every attempt's route.
  return { inputTokens: input.value, outputTokens: output.value, totalTokens: input.value + output.value }
}

/** Strict ended-turn accounting from the canonical whole physical-attempt inventory. */
export function turnAccountingFromBilling(billing?: ProviderBillingV1): TurnTokenAccounting | undefined {
  if (!billing?.closed || !billing.inventoryComplete || !counter(billing.attemptCount) || billing.attemptCount === 0 ||
    billing.settledAttemptCount !== billing.attemptCount ||
    ![billing.uncachedInputTokens, billing.outputTokens, billing.totalTokens].every(count => count.complete &&
      count.attemptCount === billing.attemptCount && count.knownAttemptCount === billing.attemptCount && counter(count.reportedValue))) return undefined
  const optional = (count: ReportedTokenCountV1): number | undefined => count.complete ? count.reportedValue : undefined
  return { inputTokens: billing.inputTokens.reportedValue, outputTokens: billing.outputTokens.reportedValue,
    totalTokens: billing.totalTokens.reportedValue, uncachedInputTokens: optional(billing.uncachedInputTokens),
    cacheReadTokens: optional(billing.cacheReadTokens), cacheWriteTokens: optional(billing.cacheWriteTokens),
    reasoningTokens: optional(billing.reasoningTokens), routes: billing.routes }
}

type Translate = (key: string, values?: Record<string, unknown>) => string

export function formatDshTokens(value: number, t: Translate): string {
  const scaled = (candidate: number): string => candidate >= 100
    ? String(Math.round(candidate)) : String(Math.round(candidate * 10) / 10)
  if (value < 1_000) return String(value)
  if (value < 1_000_000) return t('dshNumberThousand', { value: scaled(value / 1_000) })
  return t('dshNumberMillion', { value: scaled(value / 1_000_000) })
}

export function formatDshExactTokens(value: number, t: Translate): string {
  const digits = String(value), groups: string[] = []
  for (let end = digits.length; end > 0; end -= 3) groups.unshift(digits.slice(Math.max(0, end - 3), end))
  return groups.join(t('dshNumberGroupSeparator'))
}

function roundedPercentUnits(read: number, denominator: number, decimalPlaces: 0 | 1): number {
  const scale = (decimalPlaces === 0 ? 1 : 10) * 100, doubledScale = scale * 2
  const quotient = Math.floor(denominator / doubledScale), remainder = denominator % doubledScale
  let lower = 0, upper = scale
  while (lower < upper) {
    const candidate = Math.floor((lower + upper + 1) / 2), factor = candidate * 2 - 1
    const threshold = factor * quotient + Math.ceil(factor * remainder / doubledScale)
    if (read >= threshold) lower = candidate
    else upper = candidate - 1
  }
  return lower
}

export function formatDshCacheHitPercent(read: number, input: number, decimalPlaces: 0 | 1 = 0): string | null {
  if (!counter(read) || !counter(input) || read > input || input === 0) return null
  const missed = input - read
  if (missed === 0) return '100'
  const units = roundedPercentUnits(read, input, decimalPlaces)
  if (units < (decimalPlaces === 0 ? 100 : 1_000)) {
    if (decimalPlaces === 0) return String(units)
    const whole = Math.floor(units / 10), tenths = units % 10
    return tenths === 0 ? String(whole) : `${whole}.${tenths}`
  }
  let places = 1, scaledDoubleGap = missed * 200
  const tens = Math.floor(input / 10)
  while (scaledDoubleGap <= tens) { scaledDoubleGap *= 10; places += 1 }
  const ones = input % 10
  let roundedLoss = 5
  for (let loss = 1; loss < 5; loss += 1) {
    const factor = loss * 2 + 1, threshold = factor * tens + Math.floor(factor * ones / 10)
    if (scaledDoubleGap <= threshold) { roundedLoss = loss; break }
  }
  return `99.${'9'.repeat(places - 1)}${10 - roundedLoss}`
}

function StatPill({ stat, icon, label, ariaLabel, title, titleValue, children, turn = false, ownerKey }: {
  stat: 'activity' | 'usage'; icon: ReactNode; label: string | ReactNode; ariaLabel?: string; title?: string; titleValue?: string
  children?: ReactNode; turn?: boolean; ownerKey?: string | null
}) {
  const enabled = Boolean(children) && Boolean(ownerKey)
  const { open, setOpen, rootRef, triggerRef, panelRef, style, id } = useAnchoredPanel<HTMLSpanElement>({ enabled, ownerKey })
  return <span ref={rootRef} className={turn ? css.root : css.anchor} data-composer-stat={turn ? undefined : stat}
    data-analytix-turn-usage={turn || undefined}>
    {enabled ? <button ref={triggerRef} type="button" className={turn ? css.trigger : css.pill} aria-haspopup="dialog" aria-expanded={open}
      aria-controls={open ? id : undefined} aria-label={ariaLabel ?? (typeof label === 'string' ? label : title)}
      onClick={() => setOpen(!open)}>{icon}<span className={css.label}>{label}</span></button>
      : <span className={css.pill}>{icon}<span className={css.label}>{label}</span></span>}
    {open && children && createPortal(<div id={id} ref={panelRef} className={panelCss.panel} role="dialog" aria-label={title}
      style={style}>
      <div className={css.title}><span className={css.titleLabel}>{icon}{title}</span>
        {titleValue !== undefined ? <span className={css.titleValue}>{titleValue}</span> : null}</div>
      <div className={css.titleRule} aria-hidden />{children}
    </div>, document.body)}
  </span>
}

export function TurnUsagePanel({ billing, mode = 'detailed', ownerKey }: { billing: ProviderBillingV1; mode?: PerformanceUsageMode; ownerKey?: string | null }) {
  const { t } = useTranslation('common')
  const usage = turnAccountingFromBilling(billing)
  if (mode !== 'detailed' || !usage) return null
  const count = (value: number): string => t('dshUsageTokens', { tokens: formatDshExactTokens(value, t) })
  const routes = usage.routes?.map(route => `${route.provider}/${route.model}`).join(', ') ?? ''
  const hit = usage.cacheReadTokens === undefined ? null : formatDshCacheHitPercent(usage.cacheReadTokens, usage.inputTokens, 1)
  const total = t('dshUsageTokens', { tokens: formatDshTokens(usage.totalTokens, t) })
  return <StatPill stat="usage" turn ownerKey={ownerKey} icon={<DshDatabaseOutlineRegular />} label={t('dshUsageConsumed', { total })}
    title={t('dshTurnUsage')} titleValue={count(usage.totalTokens)}>
    <dl className={css.details} data-turn-usage-details>
      {routes ? <><dt>{t('dshUsageModel')}</dt><dd className={css.route}>{routes}</dd></> : null}
      {hit !== null ? <><dt>{t('dshUsageCacheHit')}</dt><dd>{hit}%</dd></> : null}
      <dt>{t('dshUsageInput')}</dt><dd>{usage.uncachedInputTokens === undefined ? t('dshUsageUnknown') : count(usage.uncachedInputTokens)}</dd>
      {usage.cacheReadTokens !== undefined ? <><dt>{t('dshUsageCacheRead')}</dt><dd>{count(usage.cacheReadTokens)}</dd></> : null}
      {usage.cacheWriteTokens !== undefined ? <><dt>{t('dshUsageCacheWrite')}</dt><dd>{count(usage.cacheWriteTokens)}</dd></> : null}
      <dt>{t('dshUsageOutput')}</dt><dd>{count(usage.outputTokens)}{usage.reasoningTokens !== undefined
        ? <span className={css.reasoning}>{t('dshUsageReasoningSubset', { tokens: count(usage.reasoningTokens) })}</span> : null}</dd>
    </dl>
  </StatPill>
}

function joined(first: string, second: string | null): ReactNode {
  return second === null ? first : <>{first}<span className={css.sep} aria-hidden>·</span>{second}</>
}

/** Preserve the canonical per-field coverage and paired timing groups. */
export function SessionStatsPills({ mode, billing, metrics, ownerKey }: {
  mode: PerformanceUsageMode; billing?: ProviderBillingV1; metrics?: ConversationMetricsV1; ownerKey?: string | null
}) {
  const { t } = useTranslation('common')
  const duration = (ms: number): string => ms < 60_000 ? t('dshDurationCompactSeconds', { seconds: Math.round(ms / 100) / 10 })
    : t('dshDurationCompactMinutes', { minutes: Math.floor(Math.round(ms / 1_000) / 60), seconds: Math.round(ms / 1_000) % 60 })
  const exact = (value: number): string => t('dshUsageTokens', { tokens: formatDshExactTokens(value, t) })
  const partial = (value: string): string => t('dshUsageReportedPartial', { value })
  const tokenLabel = (count: ReportedTokenCountV1, compact = false): string => {
    if (!count.complete && count.knownAttemptCount === 0) return t('dshUsageUnknown')
    const value = compact ? t('dshUsageTokens', { tokens: formatDshTokens(count.reportedValue, t) }) : exact(count.reportedValue)
    return count.complete ? value : partial(value)
  }
  const timingLabel = (value: number, known: number, complete: boolean): string => complete ? duration(value)
    : known > 0 ? partial(duration(value)) : t('dshUsageUnknown')
  const speedValue = metrics && metrics.decodeSteps > 0 && metrics.decodeMs > 0 ? metrics.decodeTokens / metrics.decodeMs * 1_000 : null
  const speed = speedValue === null ? null : t('dshUsageTokensPerSecond', { tps: speedValue >= 10 ? Math.round(speedValue) : Math.round(speedValue * 10) / 10 })
  const counts = metrics ? t('dshStatsCounts', { turns: metrics.turnCount ?? t('dshUsageUnknown'), steps: metrics.stepCount ?? t('dshUsageUnknown') }) : ''
  const hasLlm = metrics && (metrics.llmMs.knownStepCount > 0 || metrics.llmMs.reportedValue > 0)
  const hasTool = metrics && metrics.toolMs.callCount > 0
  const hasActivityRows = Boolean(hasLlm || hasTool || metrics && metrics.ttftSteps > 0 || speed !== null)
  const percent = billing?.inputTokens.complete && billing.cacheReadTokens.complete
    ? formatDshCacheHitPercent(billing.cacheReadTokens.reportedValue, billing.inputTokens.reportedValue) : null
  const cache = percent === null ? null : t('dshStatsCacheHit', { percent })
  const total = billing ? tokenLabel(billing.totalTokens, true) : ''
  const knownZeroUsage = billing && billing.inventoryComplete && billing.closed && billing.settledAttemptCount === billing.attemptCount &&
    [billing.inputTokens,billing.outputTokens,billing.totalTokens,billing.uncachedInputTokens,
      billing.cacheReadTokens,billing.cacheWriteTokens,billing.reasoningTokens].every(count =>
      count.complete && count.knownAttemptCount === billing.attemptCount && count.reportedValue === 0)
  return <Fragment>
    {metrics && (mode === 'compact' ? speed !== null : (metrics.turnCount !== null && metrics.turnCount > 0) || (metrics.stepCount !== null && metrics.stepCount > 0) ||
      metrics.turnCount === null || metrics.stepCount === null || hasActivityRows) ? <StatPill stat="activity" ownerKey={ownerKey}
      icon={<DshGaugeOutlineRegular />} label={mode === 'compact' ? speed : joined(counts, speed)}
      ariaLabel={speed === null ? counts : `${counts} · ${speed}`} title={t('dshStatsTitle')}>
      {mode === 'detailed' && hasActivityRows ? <dl className={css.details} data-session-stats-details>
        {hasLlm ? <><dt>{t('dshStatsLlmTime')}</dt><dd>{timingLabel(metrics.llmMs.reportedValue, metrics.llmMs.knownStepCount, metrics.llmMs.complete)}</dd></> : null}
        {hasTool ? <><dt>{t('dshStatsToolTime')}</dt><dd>{timingLabel(metrics.toolMs.reportedValue, metrics.toolMs.knownCallCount, metrics.toolMs.complete)}</dd></> : null}
        {metrics.ttftSteps > 0 ? <><dt>{t('dshStatsTtft')}</dt><dd>{duration(metrics.ttftMs / metrics.ttftSteps)}</dd></> : null}
        {speed !== null ? <><dt>{t('dshStatsSpeed')}</dt><dd>{speed}</dd></> : null}
      </dl> : undefined}
    </StatPill> : null}
    {billing && !knownZeroUsage && (mode === 'detailed' || cache !== null) ? <StatPill stat="usage" ownerKey={ownerKey} icon={<DshDatabaseOutlineRegular />}
      label={mode === 'compact' ? cache : joined(total, cache)}
      ariaLabel={[total, cache].filter(Boolean).join(' · ')} title={t('dshStatsUsageTitle')} titleValue={tokenLabel(billing.totalTokens)}>
      {mode === 'detailed' ? <dl className={css.details} data-session-stats-usage>
        {cache !== null ? <><dt>{t('dshUsageCacheHit')}</dt><dd>{percent}%</dd></> : null}
        <dt>{t('dshUsageInput')}</dt><dd>{tokenLabel(billing.uncachedInputTokens)}</dd>
        <dt>{t('dshUsageCacheRead')}</dt><dd>{tokenLabel(billing.cacheReadTokens)}</dd>
        {!billing.cacheWriteTokens.complete || billing.cacheWriteTokens.reportedValue !== 0 ? <><dt>{t('dshUsageCacheWrite')}</dt><dd>{tokenLabel(billing.cacheWriteTokens)}</dd></> : null}
        <dt>{t('dshUsageOutput')}</dt><dd>{tokenLabel(billing.outputTokens)}</dd>
      </dl> : undefined}
    </StatPill> : null}
  </Fragment>
}

export type OwnedSessionStatsProjection = {
  threadId: string
  billing?: ProviderBillingV1
  metrics?: ConversationMetricsV1
}

/** Retire old dialogs on owner change. Absent public producer groups stay absent. */
export function OwnedSessionStatsPills({ threadId, projection }: {
  threadId: string | null; projection?: OwnedSessionStatsProjection
}) {
  const mode = usePerformanceUsageMode()
  if (!threadId || projection?.threadId !== threadId) return null
  return <SessionStatsPills key={threadId} ownerKey={threadId} mode={mode} billing={projection.billing} metrics={projection.metrics} />
}
