import i18n from '../i18n'
import { redactSecretText } from '@shared/secret-redaction'
import { stripASCIIControlCharacters } from '@shared/public-runtime-content'
import { PublicTurnFailureReasonCode } from '../../../../packages/runtime/src/contracts/errors.js'

type RuntimeErrorPayload = {
  code?: string
  error?: string | { message?: string; status?: number }
  message?: string
  details?: unknown
  providerError?: unknown
  severity?: 'info' | 'warning' | 'error'
  reasonCode?: string
}

export type RuntimeErrorView = {
  summary: string
  detail?: string
  code?: string
  settingsAction?: 'agents'
}

function readJsonPayload(raw: string): RuntimeErrorPayload | null {
  try {
    return JSON.parse(raw) as RuntimeErrorPayload
  } catch {
    return null
  }
}

function stripIpcPrefix(message: string): string {
  return message
    .replace(/^Error invoking remote method ['"][^'"]+['"]:\s*/i, '')
    .replace(/^Error:\s*/i, '')
    .trim()
}

export function getRuntimeErrorCode(error: unknown): string | null {
  const raw = stripIpcPrefix(error instanceof Error ? error.message : String(error ?? ''))
  const payload = readJsonPayload(raw)
  return runtimeErrorCode(payload, raw)
}

function runtimeErrorCode(payload: RuntimeErrorPayload | null, raw: string): string | null {
  const fromCode = typeof payload?.code === 'string' ? payload.code.trim() : ''
  if (fromCode) return fromCode.toLowerCase()
  const fromError = typeof payload?.error === 'string' ? payload.error.trim() : ''
  if (fromError) return fromError.toLowerCase()
  const lowered = stripIpcPrefix(payloadMessage(payload) || raw).toLowerCase()
  if (lowered.includes('fetch failed')) return 'fetch_failed'
  if (lowered.includes('runtime unhealthy')) return 'runtime_unhealthy'
  if (lowered.includes('active turn')) return 'turn_in_progress'
  if (lowered.includes('preload bridge missing')) return 'preload_bridge_missing'
  if (
    lowered.includes('managed runtime npm package missing') ||
    lowered.includes('analytix npm package missing') ||
    lowered.includes('cannot find package.json')
  ) {
    return 'runtime_binary_not_installed'
  }
  return null
}

function payloadMessage(payload: RuntimeErrorPayload | null): string {
  if (typeof payload?.message === 'string' && payload.message.trim()) return payload.message.trim()
  if (payload?.error && typeof payload.error === 'object') {
    const message = payload.error.message
    if (typeof message === 'string' && message.trim()) return message.trim()
  }
  return ''
}

const SAFE_DIAGNOSTIC_KEYS = new Set([
  'status',
  'provider',
  'providerId',
  'family',
  'model',
  'endpointFormat',
  'kind',
  'authStatus',
  'hasApiKey',
  'retryable',
  'attempt',
  'maxAttempt',
  'port',
  'backend',
  'requestedBackend',
  'autoStart',
  'phase'
])

function safeDiagnosticString(value: unknown): string {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return ''
  const safe: Record<string, string | number | boolean> = {}
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    if (!SAFE_DIAGNOSTIC_KEYS.has(key)) continue
    if (typeof entry === 'boolean' || (typeof entry === 'number' && Number.isFinite(entry))) {
      safe[key] = entry
      continue
    }
    if (typeof entry === 'string') {
      const sanitized = stripASCIIControlCharacters(redactSecretText(entry)).trim().slice(0, 256)
      if (sanitized) safe[key] = sanitized
    }
  }
  return Object.keys(safe).length > 0 ? JSON.stringify(safe, null, 2) : ''
}

function payloadProviderError(payload: RuntimeErrorPayload | null): Record<string, unknown> | null {
  const details = payload?.details
  const detailRecord = details && typeof details === 'object' && !Array.isArray(details)
    ? details as Record<string, unknown>
    : null
  const value = payload?.providerError ?? detailRecord?.providerError
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}

function providerErrorString(value: Record<string, unknown> | null, key: string): string {
  const item = value?.[key]
  return typeof item === 'string' ? item.trim() : ''
}

function providerErrorStatus(value: Record<string, unknown> | null): number {
  const item = value?.status
  return typeof item === 'number' && Number.isFinite(item) ? item : 0
}

function providerErrorIsConfigurationIssue(value: Record<string, unknown> | null): boolean {
  const status = providerErrorStatus(value)
  return providerErrorString(value, 'authStatus') === 'required' ||
    providerErrorString(value, 'kind') === 'auth' ||
    status === 400 ||
    status === 401 ||
    status === 403 ||
    status === 404
}

function providerErrorIsBillingIssue(value: Record<string, unknown> | null): boolean {
  const status = providerErrorStatus(value)
  return providerErrorString(value, 'kind') === 'insufficient_balance' ||
    status === 402
}

function payloadReasonCode(payload: RuntimeErrorPayload | null): string {
  const parsed = PublicTurnFailureReasonCode.safeParse(payload?.reasonCode)
  return parsed.success ? parsed.data : ''
}

function reasonIsProviderConfigurationIssue(reasonCode: string): boolean {
  return reasonCode === 'provider_authentication_failed' ||
    reasonCode === 'provider_endpoint_not_found' ||
    reasonCode === 'provider_model_invalid' ||
    reasonCode === 'provider_not_configured' ||
    reasonCode === 'provider_request_rejected'
}

function localizedRuntimeSummary(
  code: string | null,
  text: string,
  providerError: Record<string, unknown> | null = null,
  reasonCode = ''
): string | null {
  const lowered = text.toLowerCase()

  if (providerErrorIsBillingIssue(providerError) || reasonCode === 'provider_insufficient_balance') {
    return i18n.t('common:runtimeProviderBillingError')
  }

  if (isProviderConfigurationHttpError(code, lowered) || providerErrorIsConfigurationIssue(providerError) || reasonIsProviderConfigurationIssue(reasonCode)) {
    return i18n.t('common:runtimeProviderConfigurationError')
  }

  if (code === 'runtime_unavailable' || code === 'fetch_failed' || lowered.includes('fetch failed')) {
    return i18n.t('common:runtimeFetchFailed')
  }

  if (code === 'missing_api_key') {
    return i18n.t('common:runtimeMissingApiKey')
  }

  if (code === 'runtime_offline') {
    return i18n.t('common:runtimeAutoStartDisabled')
  }

  if (code === 'runtime_auth_required') {
    return i18n.t('common:runtimeAuthRequired')
  }

  if (code === 'runtime_request_user_input_unsupported') {
    return i18n.t('common:runtimeUserInputUnsupported')
  }

  if (code === 'runtime_port_conflict') {
    return i18n.t('common:runtimePortConflict')
  }

  if (code === 'runtime_unhealthy' || lowered.includes('runtime unhealthy')) {
    return i18n.t('common:runtimeUnhealthy')
  }

  if (code === 'turn_in_progress' || lowered.includes('active turn')) {
    return i18n.t('common:runtimeActiveTurn')
  }

  if (code === 'runtime_binary_not_installed') {
    return i18n.t('common:runtimeBinaryNotInstalled')
  }

  if (code === 'preload_bridge_missing' || lowered.includes('preload bridge missing')) {
    return i18n.t('common:preloadBridgeMissing')
  }

  return null
}

function isProviderConfigurationHttpError(code: string | null, loweredText = ''): boolean {
  return code === 'http_400' ||
    code === 'http_404' ||
    loweredText.includes('model request failed with status 400') ||
    loweredText.includes('model request failed with status 404')
}

function shouldOpenAgentsSettings(
  code: string | null,
  providerError: Record<string, unknown> | null = null,
  reasonCode = ''
): boolean {
  return code === 'missing_api_key' ||
    code === 'runtime_unavailable' ||
    code === 'runtime_offline' ||
    code === 'runtime_auth_required' ||
    code === 'runtime_port_conflict' ||
    reasonIsProviderConfigurationIssue(reasonCode) ||
    reasonCode === 'provider_insufficient_balance' ||
    providerErrorIsConfigurationIssue(providerError) ||
    providerErrorIsBillingIssue(providerError) ||
    isProviderConfigurationHttpError(code)
}

export function describeRuntimeError(error: unknown): RuntimeErrorView {
  const raw = stripIpcPrefix(error instanceof Error ? error.message : String(error ?? ''))
  const payload = readJsonPayload(raw)
  const errorCode = runtimeErrorCode(payload, raw)
  const payloadText = payloadMessage(payload)
  const text = stripIpcPrefix(payloadText || raw)
  const redactedText = redactSecretText(text)
  const providerError = payloadProviderError(payload)
  const reasonCode = payloadReasonCode(payload)
  const summary = localizedRuntimeSummary(errorCode, redactedText, providerError, reasonCode) ||
    i18n.t('common:runtimeRequestFailed')
  const details: string[] = []
  if (errorCode) details.push(`Code: ${errorCode}`)
  if (reasonCode) details.push(`Reason: ${reasonCode}`)
  if (payload?.severity) details.push(`Severity: ${payload.severity}`)
  const payloadDetails = safeDiagnosticString(payload?.details)
  if (payloadDetails) details.push(`Details:\n${payloadDetails}`)
  const providerDetails = safeDiagnosticString(providerError)
  if (providerDetails) details.push(`Provider diagnostic:\n${providerDetails}`)
  if (isProviderConfigurationHttpError(errorCode) || providerErrorIsConfigurationIssue(providerError) || reasonIsProviderConfigurationIssue(reasonCode)) {
    details.push(i18n.t('common:runtimeProviderConfigurationHint'))
  }
  if (providerErrorIsBillingIssue(providerError) || reasonCode === 'provider_insufficient_balance') {
    details.push(i18n.t('common:runtimeProviderBillingHint'))
  }
  return {
    summary,
    ...(details.length > 0 ? { detail: details.join('\n\n') } : {}),
    ...(errorCode ? { code: errorCode } : {}),
    ...(shouldOpenAgentsSettings(errorCode, providerError, reasonCode) ? { settingsAction: 'agents' as const } : {})
  }
}

export function formatRuntimeError(error: unknown): string {
  return describeRuntimeError(error).summary
}
