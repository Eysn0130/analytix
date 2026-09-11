import type { ToolBlock } from '../../agent/types'
import { redactSecretText } from '@shared/secret-redaction'

type Translator = (key: string, opts?: Record<string, unknown>) => string

export function readNumber(meta: Record<string, unknown> | undefined, key: string): number | undefined {
  if (!meta) return undefined
  const v = meta[key]
  return typeof v === 'number' && Number.isFinite(v) ? v : undefined
}

function readString(meta: Record<string, unknown> | undefined, key: string): string {
  if (!meta) return ''
  const v = meta[key]
  return typeof v === 'string' && v.trim() ? v.trim() : ''
}

function readRecord(meta: Record<string, unknown> | undefined, key: string): Record<string, unknown> | undefined {
  const v = meta?.[key]
  return v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : undefined
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : undefined
}

function readRecordString(record: Record<string, unknown> | undefined, key: string): string {
  const value = record?.[key]
  return typeof value === 'string' && value.trim() ? redactSecretText(value.trim()) : ''
}

function readRecordNumber(record: Record<string, unknown> | undefined, key: string): number | undefined {
  const value = record?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function readChildString(child: Record<string, unknown> | null, key: string): string {
  const value = child?.[key]
  return typeof value === 'string' && value.trim() ? value.trim() : ''
}

function readChildNumber(child: Record<string, unknown> | null, key: string): number | undefined {
  const value = child?.[key]
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

export function formatToolTitle(block: ToolBlock, t: (key: string) => string): string {
  if (block.toolKind === 'file_change') return t('toolActionFile')
  if (block.toolKind === 'command_execution') return t('toolActionCommand')
  if (block.toolKind === 'subagent') return t('toolChildAgent')
  return t('toolActionTool')
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.max(1, Math.round(ms))}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`
  if (ms < 3_600_000) {
    const totalSeconds = Math.round(ms / 1000)
    const m = Math.floor(totalSeconds / 60)
    const s = totalSeconds % 60
    return `${m}m ${s}s`
  }
  if (ms < 86_400_000) {
    const totalMinutes = Math.round(ms / 60_000)
    const h = Math.floor(totalMinutes / 60)
    const m = totalMinutes % 60
    return `${h}h ${m}m`
  }
  const totalHours = Math.round(ms / 3_600_000)
  const d = Math.floor(totalHours / 24)
  const h = totalHours % 24
  return `${d}d ${h}h`
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${Math.max(0, Math.round(bytes))}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)}KB`
  return `${(bytes / (1024 * 1024)).toFixed(bytes < 10 * 1024 * 1024 ? 1 : 0)}MB`
}

export function formatJobHeartbeatStatus(status: string | undefined, t: Translator): string {
  switch (status) {
    case 'running':
      return t('toolJobHeartbeatRunning')
    case 'stale':
      return t('toolJobHeartbeatStale')
    case 'lease_expired':
      return t('toolJobHeartbeatLeaseExpired')
    case 'orphaned':
      return t('toolJobHeartbeatOrphaned')
    case 'recovering':
      return t('toolJobHeartbeatRecovering')
    case 'recovered':
      return t('toolJobHeartbeatRecovered')
    case 'dead_lettered':
      return t('toolJobHeartbeatDeadLettered')
    case 'paused':
      return t('subagentStatusPaused')
    default:
      return status ?? ''
  }
}

function formatChildCache(value: number | undefined): string {
  if (value === undefined) return ''
  const normalized = value <= 1 ? value * 100 : value
  return `${Math.max(0, Math.min(100, Math.round(normalized)))}%`
}

export function formatChildAgentChip(
  child: Record<string, unknown> | null,
  t: Translator
): { label: string; detail: string; title: string } | null {
  const label = readChildString(child, 'childLabel') || readChildString(child, 'childId')
  if (!label) return null
  const parentThreadId = readChildString(child, 'parentThreadId')
  const parentTurnId = readChildString(child, 'parentTurnId')
  const parentToolCallId = readChildString(child, 'parentToolCallId')
  const status = readChildString(child, 'childStatus')
  const childId = readChildString(child, 'childId')
  const childRunId = readChildString(child, 'childRunId')
  const childThreadId = readChildString(child, 'childThreadId')
  const childTurnId = readChildString(child, 'childTurnId')
  const jobId = readChildString(child, 'jobId')
  const childEffort = readChildString(child, 'childEffort') || readChildString(child, 'effort')
  const childProfile = readChildString(child, 'childProfile')
  const childProfileMode = readChildString(child, 'childProfileMode')
  const childProfileDescription = readChildString(child, 'childProfileDescription')
  const childToolPolicy = readChildString(child, 'childToolPolicy')
  const returnFormat = readChildString(child, 'returnFormat')
  const evidenceBundleStatus = readChildString(child, 'evidenceBundleStatus')
  const childSeq = readChildNumber(child, 'childSeq')
  const parallelIndex = readChildNumber(child, 'parallelIndex')
  const parallelGroupId = readChildString(child, 'parallelGroupId')
  const toolInvocations = readChildNumber(child, 'toolInvocations')
  const durationMs = readChildNumber(child, 'durationMs')
  const queuedMs = readChildNumber(child, 'queuedMs')
  const totalTokens = readChildNumber(child, 'totalTokens')
  const tokenBudget = readChildNumber(child, 'tokenBudget')
  const timeBudgetMs = readChildNumber(child, 'timeBudgetMs')
  const evidenceCount = readChildNumber(child, 'evidenceCount')
  const cacheHitRate = formatChildCache(readChildNumber(child, 'cacheHitRate'))
  const details = [
    childSeq !== undefined ? `#${childSeq}` : '',
    status,
    childEffort,
    child?.background === true ? t('toolChildBackground') : '',
    parallelIndex !== undefined ? `${t('toolChildParallel')} ${parallelIndex}` : '',
    toolInvocations !== undefined ? `${toolInvocations} ${t('toolChildTools')}` : '',
    durationMs !== undefined && durationMs > 0 ? `${t('toolChildDuration')} ${formatDuration(durationMs)}` : '',
    queuedMs !== undefined && queuedMs > 0 ? `${t('toolChildQueued')} ${formatDuration(queuedMs)}` : '',
    totalTokens !== undefined ? `${totalTokens} ${t('toolChildTokens')}` : '',
    returnFormat ? `${t('toolChildReturnFormat')} ${returnFormat}` : '',
    child?.budgetExceeded === true ? t('toolChildBudgetExceeded') : '',
    evidenceBundleStatus
      ? `${t('toolChildEvidence')} ${evidenceBundleStatus}${evidenceCount !== undefined ? ` (${evidenceCount})` : ''}`
      : '',
    cacheHitRate ? `${t('toolChildCache')} ${cacheHitRate}` : ''
  ].filter(Boolean)
  const title = [
    label,
    childId && childId !== label ? childId : '',
    childRunId && childRunId !== childId ? `childRun:${childRunId}` : '',
    jobId ? `job:${jobId}` : '',
    parentThreadId ? `parentThread:${parentThreadId}` : '',
    parentTurnId ? `parentTurn:${parentTurnId}` : '',
    parentToolCallId ? `parentCall:${parentToolCallId}` : '',
    childThreadId,
    childTurnId,
    childProfile ? `profile:${childProfile}` : '',
    childProfileMode ? `profileMode:${childProfileMode}` : '',
    childProfileDescription ? `profileDescription:${childProfileDescription}` : '',
    childToolPolicy ? `toolPolicy:${childToolPolicy}` : '',
    tokenBudget !== undefined ? `tokenBudget:${tokenBudget}` : '',
    timeBudgetMs !== undefined ? `timeBudget:${formatDuration(timeBudgetMs)}` : '',
    parallelGroupId ? `parallelGroup:${parallelGroupId}` : '',
    ...details
  ].filter(Boolean).join(' · ')
  return { label, detail: details.join(' · '), title }
}

export function formatJobDiagnosticsChip(
  meta: Record<string, unknown> | undefined,
  t: Translator
): { label: string; detail: string; title: string } | null {
  const diagnostics = readRecord(meta, 'diagnostics')
  if (!diagnostics) return null

  const status = readString(diagnostics, 'status')
  const heartbeatStatus = readString(diagnostics, 'heartbeatStatus')
  const heartbeatStatusLabel = formatJobHeartbeatStatus(heartbeatStatus, t)
  const notificationKind = readString(diagnostics, 'notificationKind')
  const autoContinueStatus = readString(diagnostics, 'autoContinueStatus')
  const deliveryStatus = readString(diagnostics, 'deliveryStatus')
  const completionDeliveryAttempt = readNumber(diagnostics, 'completionDeliveryAttempt')
  const completionDeliveryAt = readString(diagnostics, 'completionDeliveryAt')
  const completionDeadLetterAt = readString(diagnostics, 'completionDeadLetterAt')
  const warningCode = readString(diagnostics, 'warningCode')
  const idleMs = readNumber(diagnostics, 'idleMs')
  const heartbeatAgeMs = readNumber(diagnostics, 'heartbeatAgeMs')
  const heartbeatAt = readString(diagnostics, 'heartbeatAt')
  const lastHeartbeatAt = readString(diagnostics, 'lastHeartbeatAt')
  const leaseOwner = readString(diagnostics, 'leaseOwner')
  const leaseExpiresAt = readString(diagnostics, 'leaseExpiresAt')
  const recoveryStatus = readString(diagnostics, 'recoveryStatus')
  const outputBytes = readNumber(diagnostics, 'outputBytes')
  const nextOffset = readNumber(diagnostics, 'nextOffset')
  const stalled = diagnostics.stalled === true
  const background = diagnostics.background === true
  const detail = [
    autoContinueStatus ? `${t('toolAutoContinueStatus')} ${autoContinueStatus}` : '',
    deliveryStatus ? `${t('toolDeliveryStatus')} ${deliveryStatus}` : '',
    completionDeliveryAttempt !== undefined ? `${t('toolDeliveryAttempt')} ${completionDeliveryAttempt}` : '',
    heartbeatStatusLabel ? `${t('toolJobHeartbeatStatus')} ${heartbeatStatusLabel}` : '',
    stalled && !heartbeatStatus ? t('toolJobStalled') : status,
    background ? t('toolChildBackground') : '',
    idleMs !== undefined ? `${t('toolJobIdle')} ${formatDuration(idleMs)}` : '',
    heartbeatAgeMs !== undefined ? `${t('toolJobHeartbeat')} ${formatDuration(heartbeatAgeMs)}` : '',
    outputBytes !== undefined ? `${t('toolJobOutput')} ${formatBytes(outputBytes)}` : ''
  ].filter(Boolean).join(' · ')
  const title = [
    t('toolJobDiagnostics'),
    notificationKind,
    autoContinueStatus ? `${t('toolAutoContinueStatus')} ${autoContinueStatus}` : '',
    deliveryStatus ? `${t('toolDeliveryStatus')} ${deliveryStatus}` : '',
    completionDeliveryAttempt !== undefined ? `${t('toolDeliveryAttempt')} ${completionDeliveryAttempt}` : '',
    completionDeliveryAt ? `${t('toolDeliveryAt')} ${completionDeliveryAt}` : '',
    completionDeadLetterAt ? `${t('toolDeliveryDeadLetterAt')} ${completionDeadLetterAt}` : '',
    heartbeatStatusLabel ? `${t('toolJobHeartbeatStatus')} ${heartbeatStatusLabel}` : '',
    recoveryStatus ? `${t('toolJobRecoveryStatus')} ${recoveryStatus}` : '',
    warningCode,
    status,
    heartbeatAt ? `${t('toolJobHeartbeat')} ${heartbeatAt}` : '',
    lastHeartbeatAt && lastHeartbeatAt !== heartbeatAt ? `${t('toolJobLastHeartbeat')} ${lastHeartbeatAt}` : '',
    heartbeatAgeMs !== undefined ? `${t('toolJobHeartbeat')} ${formatDuration(heartbeatAgeMs)}` : '',
    leaseOwner ? `${t('toolJobLeaseOwner')} ${leaseOwner}` : '',
    leaseExpiresAt ? `${t('toolJobLeaseExpires')} ${leaseExpiresAt}` : '',
    nextOffset !== undefined ? `${t('toolJobOutput')} ${formatBytes(nextOffset)}` : '',
    idleMs !== undefined ? `${t('toolJobIdle')} ${formatDuration(idleMs)}` : ''
  ].filter(Boolean).join(' · ')
  return {
    label: t('toolJobDiagnostics'),
    detail,
    title
  }
}

export function formatProviderDiagnosticsChip(
  meta: Record<string, unknown> | undefined,
  t: Translator
): { label: string; detail: string; title: string } | null {
  const diagnostics = readRecord(meta, 'providerError') ??
    readRecord(asRecord(meta?.diagnostics), 'providerError') ??
    readRecord(asRecord(meta?.details), 'providerError')
  const retryDiagnostics = readRecord(meta, 'providerRetry')
  const recoveryDiagnostics = readRecord(meta, 'providerRecovery')
  const directAttempt = readRecordNumber(meta, 'attempt')
  const directMaxAttempt = readRecordNumber(meta, 'maxAttempt')
  const details = asRecord(meta?.details)
  const attempt = readRecordNumber(retryDiagnostics, 'attempt') ??
    directAttempt ??
    readRecordNumber(details, 'attempt')
  const maxAttempt = readRecordNumber(retryDiagnostics, 'maxAttempt') ??
    directMaxAttempt ??
    readRecordNumber(details, 'maxAttempt')
  const recoveryAttempt = readRecordNumber(recoveryDiagnostics, 'attempt') ??
    readRecordNumber(details, 'recoveryAttempt')
  const maxRecoveryAttempt = readRecordNumber(recoveryDiagnostics, 'maxAttempt') ??
    readRecordNumber(details, 'maxRecoveryAttempts')
  const recoveryKind = readRecordString(recoveryDiagnostics, 'kind') ||
    readRecordString(details, 'recoveryKind')
  const recoveryExhausted = recoveryDiagnostics?.recoveryExhausted === true ||
    recoveryDiagnostics?.exhausted === true ||
    details?.recoveryExhausted === true
  if (
    !diagnostics &&
    attempt === undefined &&
    maxAttempt === undefined &&
    recoveryAttempt === undefined &&
    maxRecoveryAttempt === undefined &&
    !recoveryKind &&
    !recoveryExhausted
  ) return null

  const providerId = readRecordString(diagnostics, 'providerId')
  const family = readRecordString(diagnostics, 'family')
  const endpointFormat = readRecordString(diagnostics, 'endpointFormat')
  const status = readRecordNumber(diagnostics, 'status')
  const kind = readRecordString(diagnostics, 'kind')
  const authStatus = readRecordString(diagnostics, 'authStatus')
  const hasApiKey = typeof diagnostics?.hasApiKey === 'boolean' ? diagnostics.hasApiKey : undefined
  const retryable = diagnostics?.retryable === true
  const authDetail = authStatus ||
    (hasApiKey === false
      ? t('toolProviderApiKeyMissing')
      : hasApiKey === true
        ? t('toolProviderApiKeyConfigured')
        : '')
  const detail = [
    providerId || family || kind,
    status !== undefined ? `${t('toolProviderStatus')} ${status}` : '',
    endpointFormat ? `${t('toolProviderEndpointFormat')} ${endpointFormat}` : '',
    attempt !== undefined && maxAttempt !== undefined
      ? `${t('toolProviderRetry')} ${attempt}/${maxAttempt}`
      : attempt !== undefined
        ? `${t('toolProviderRetry')} ${attempt}`
        : '',
    recoveryAttempt !== undefined && maxRecoveryAttempt !== undefined
      ? `${t('toolProviderRecovery')} ${recoveryAttempt}/${maxRecoveryAttempt}`
      : recoveryAttempt !== undefined
        ? `${t('toolProviderRecovery')} ${recoveryAttempt}`
        : recoveryKind ? t('toolProviderRecovery') : '',
    recoveryExhausted ? t('toolProviderRecoveryExhausted') : '',
    authDetail ? `${t('toolProviderAuth')} ${authDetail}` : '',
    retryable ? t('toolProviderRetryable') : ''
  ].filter(Boolean).join(' · ')
  if (!detail) return null

  return {
    label: t('toolProviderDiagnostics'),
    detail,
    title: [t('toolProviderDiagnostics'), detail].filter(Boolean).join(' · ')
  }
}
