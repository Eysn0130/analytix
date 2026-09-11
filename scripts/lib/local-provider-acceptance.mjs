import { lstatSync, realpathSync } from 'node:fs'
import { isIP } from 'node:net'
import { join, resolve } from 'node:path'

export const LOCAL_PROVIDER_AUTHORITY = 'local-provider-registry-secret-store'
const incarnation = /^inc_[A-Za-z0-9_-]{43}$/u
const identifier = /^[a-z0-9][a-z0-9._-]{0,95}$/u
const positiveRevision = (value) => typeof value === 'string' &&
  /^[1-9][0-9]{0,19}$/u.test(value) && BigInt(value) <= 18446744073709551615n
const record = (value) => value !== null && typeof value === 'object' && !Array.isArray(value)
const endpointFormats = {
  'openai-compatible': 'chat_completions', deepseek: 'chat_completions',
  'chat-completions': 'chat_completions', chat_completions: 'chat_completions',
  'openai-responses': 'responses', responses: 'responses',
  'anthropic-compatible': 'messages', 'anthropic-messages': 'messages', messages: 'messages',
  'custom-endpoint': 'custom_endpoint', custom_endpoint: 'custom_endpoint'
}
const providerKeys = new Set(['id', 'kind', 'endpoint', 'proxy', 'models', 'mediaModels',
  'selectedModel', 'selectedMediaModel', 'selectedRoutes', 'credentialConfigured',
  'credentialPurpose', 'revision', 'generation', 'incarnation', 'tombstone',
  'oauthBinding', 'accountObservation'])

export function freshLocalProviderRegistry(snapshot) {
  return record(snapshot) && snapshot.schemaVersion === 1 &&
    snapshot.registryRevision === '0' && incarnation.test(snapshot.registryIncarnation) &&
    snapshot.selectedProviderId === undefined && Array.isArray(snapshot.providers) &&
    snapshot.providers.length === 0 && Object.keys(snapshot).every((key) =>
      ['schemaVersion', 'registryRevision', 'registryIncarnation', 'providers'].includes(key))
}

function remoteEndpoint(value) {
  try {
    if (typeof value !== 'string' || value.length > 2048 || value !== value.trim()) return null
    const url = new URL(value)
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash ||
        /[\u0000-\u0020\\]/u.test(value) ||
        url.hostname === 'localhost' || url.hostname.endsWith('.localhost')) return null
    const host = url.hostname.replace(/^\[|\]$/gu, '')
    if (isIP(host)) {
      if (host.includes(':')) {
        if (/^(?:::|::1|f[cd][0-9a-f]*:|fe[89ab][0-9a-f]*:)/iu.test(host)) return null
        return url
      }
      const parts = host.split('.').map(Number)
      if (parts[0] === 0 || parts[0] === 10 || parts[0] === 127 ||
          (parts[0] === 169 && parts[1] === 254) ||
          (parts[0] === 172 && parts[1] >= 16 && parts[1] <= 31) ||
          (parts[0] === 192 && parts[1] === 168)) return null
    }
    return url
  } catch { return null }
}

// Public Registry readback only; no readiness claim or Secret Store access.
export function localProviderAuthorityFromRegistry(snapshot) {
  const providers = Array.isArray(snapshot?.providers) ? snapshot.providers : []
  const selected = providers.find((item) => item?.id === snapshot?.selectedProviderId)
  const endpoint = remoteEndpoint(selected?.endpoint)
  const format = Object.hasOwn(endpointFormats, selected?.kind || '')
    ? endpointFormats[selected.kind] : ''
  const model = selected?.selectedModel || ''
  const ok = Boolean(record(snapshot) && snapshot.schemaVersion === 1 &&
    Object.keys(snapshot).every((key) => ['schemaVersion', 'registryRevision', 'registryIncarnation',
      'selectedProviderId', 'providers'].includes(key)) && positiveRevision(snapshot.registryRevision) &&
    incarnation.test(snapshot.registryIncarnation) && providers.length > 0 && providers.length <= 256 &&
    providers.every((item, index) => record(item) && identifier.test(item.id) &&
      (index === 0 || providers[index - 1].id < item.id)) && record(selected) &&
    Object.keys(selected).every((key) => providerKeys.has(key)) &&
    selected.id !== 'analytix-hub' && selected.kind !== 'analytix-hub' && selected.tombstone === false &&
    selected.credentialConfigured === true &&
    typeof selected.credentialPurpose === 'string' && /^[a-z0-9][a-z0-9._:/-]{0,95}$/u.test(selected.credentialPurpose) &&
    positiveRevision(selected.revision) && BigInt(selected.revision) <= BigInt(snapshot.registryRevision) &&
    positiveRevision(selected.generation) && incarnation.test(selected.incarnation) &&
    Array.isArray(selected.models) && selected.models.includes(model) &&
    Array.isArray(selected.mediaModels) && Array.isArray(selected.selectedRoutes) &&
    Boolean(endpoint && format) && typeof model === 'string' && model.length > 0 &&
    model.length <= 128 && model === model.trim())
  return {
    ok, id: ok ? selected.id : '', model, baseUrl: endpoint?.toString() || '', endpointFormat: format,
    registryRevision: ok ? snapshot.registryRevision : '',
    registryIncarnation: ok ? snapshot.registryIncarnation : '',
    providerRevision: ok ? selected.revision : '',
    providerGeneration: ok ? selected.generation : '',
    providerIncarnation: ok ? selected.incarnation : ''
  }
}

// This binds independent public observations. Actual credential resolution and
// execution still require the separate runtime turn/receipt gates.
export function localProviderFromObservation(observation, {
  requiredModel = 'deepseek-v4-flash', requireContextWindow = true
} = {}) {
  const snapshot = observation?.providerRegistry
  const authority = localProviderAuthorityFromRegistry(snapshot)
  const providers = Array.isArray(snapshot?.providers) ? snapshot.providers : []
  const selected = providers.find((item) => item?.id === snapshot?.selectedProviderId)
  const settings = observation?.settings
  const runtime = settings?.runtime
  const providerSettings = settings?.provider
  const profile = Array.isArray(providerSettings?.providers)
    ? providerSettings.providers.find((item) => item?.id === selected?.id)
    : providerSettings?.profile
  const actual = observation?.runtimeInfo?.provider
  const { model, endpointFormat: format } = authority
  const settingsCredentialFree = providerSettings?.topLevelApiKeyEmpty !== undefined
    ? providerSettings.topLevelApiKeyEmpty === true && providerSettings.allProfilesApiKeyEmpty === true &&
      profile?.apiKeyEmpty === true && runtime?.apiKeyEmpty === true && runtime?.runtimeTokenEmpty === true
    : providerSettings?.apiKey === '' && runtime?.apiKey === '' && runtime?.runtimeToken === '' &&
      profile?.apiKey === '' && (providerSettings?.providers || []).every((item) => item?.apiKey === '')
  const registryReady = authority.ok
  const profileReady = registryReady && profile?.id === selected.id &&
    providerSettings?.activeProviderId === selected.id && runtime?.providerId === selected.id &&
    runtime?.model === model && model === requiredModel &&
    profile?.baseUrl === selected.endpoint && profile?.endpointFormat === format &&
    Array.isArray(profile?.models) && profile.models.includes(model) && settingsCredentialFree
  const runtimeReady = registryReady && actual?.id === selected.id && actual?.model === model &&
    actual?.endpointFormat === format && actual?.available === true &&
    actual?.apiKeyConfigured === true && actual?.baseUrlConfigured === true
  const settingsContextWindowTokens = profile?.modelProfiles?.[model]?.contextWindowTokens || 0
  const runtimeContextWindowTokens = actual?.contextWindowTokens || 0
  const contextWindowTokensBound = Number.isSafeInteger(settingsContextWindowTokens) &&
    settingsContextWindowTokens > 0 && runtimeContextWindowTokens === settingsContextWindowTokens
  const ok = Boolean(profileReady && runtimeReady && (!requireContextWindow || contextWindowTokensBound))
  const blocker = ok ? '' : !registryReady ? 'local_provider_registry_authority_not_ready'
    : model !== requiredModel ? 'local_provider_formal_model_invalid'
      : !profileReady ? 'local_provider_settings_invalid'
        : !runtimeReady ? 'local_provider_runtime_not_ready' : 'local_provider_context_window_profile_drift'
  return {
    ...authority, ok, blocked: !registryReady && (!snapshot || freshLocalProviderRegistry(snapshot)), blocker, id: registryReady ? selected.id : '',
    family: registryReady ? selected.id : '', model, baseUrl: authority.baseUrl,
    endpointFormat: format, credentialConfigured: registryReady,
    credentialAuthorityBound: Boolean(registryReady && profileReady && runtimeReady),
    settingsCredentialFree, contextWindowTokensBound, settingsContextWindowTokens,
    runtimeContextWindowTokens, softThresholdTokens: Math.floor(runtimeContextWindowTokens * 0.75),
    hardThresholdTokens: Math.floor(runtimeContextWindowTokens * 0.85)
  }
}

// Metadata only: never export or decrypt the production Secret Store.
export function localProviderSecretStoreEvidence(runtimeDataDir) {
  try {
    const root = resolve(runtimeDataDir)
    if (!runtimeDataDir || realpathSync(root) !== root) throw new Error('invalid_root')
    for (const path of [root, join(root, 'private'), join(root, 'private', 'provider-secrets'),
      join(root, 'private', 'provider-secrets', 'credentials.v1.json')]) {
      const stat = lstatSync(path)
      const file = path.endsWith('/credentials.v1.json')
      if (stat.isSymbolicLink() || (file ? !stat.isFile() || stat.nlink !== 1 || stat.size <= 0 : !stat.isDirectory()) ||
          (stat.mode & 0o7077) !== 0 || (typeof process.getuid === 'function' && stat.uid !== process.getuid())) {
        throw new Error('invalid_store_metadata')
      }
    }
    return { ok: true, protectedStoreOwnerPrivate: true }
  } catch {
    return { ok: false, protectedStoreOwnerPrivate: false }
  }
}


const credentialEvidenceKeys = new Set([
  'ok', 'blocked', 'blocker', 'sourceBound', 'entryMethod', 'automatedCredentialEntryUsed',
  'expectedSecretCount', 'sourceSecretCount', 'uniqueSecretCount', 'status',
  'scannedFileCount', 'findingCount', 'settingsFindingCount', 'reportFindingCount',
  'unsafeEntryCount', 'symlinkCount', 'encodingCoverage', 'protectedStoreOwnerPrivate'
])
export const LOCAL_CREDENTIAL_ENCODING_COVERAGE = Object.freeze([
  'utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'
])

export function localCredentialEvidencePassed(value) {
  return record(value) && Object.keys(value).length === credentialEvidenceKeys.size &&
    Object.keys(value).every((key) => credentialEvidenceKeys.has(key)) &&
    value.ok === true && value.blocked === false && value.blocker === null &&
    value.sourceBound === true && value.protectedStoreOwnerPrivate === true &&
    ((value.entryMethod === 'visible-computer-use' && value.automatedCredentialEntryUsed === true) ||
      (value.entryMethod === 'visible-human' && value.automatedCredentialEntryUsed === false)) &&
    value.expectedSecretCount === 1 && value.sourceSecretCount === 1 && value.uniqueSecretCount === 1 &&
    value.status === 'passed' && Number.isSafeInteger(value.scannedFileCount) && value.scannedFileCount > 0 &&
    value.findingCount === 0 && value.settingsFindingCount === 0 && value.reportFindingCount === 0 &&
    value.unsafeEntryCount === 0 && value.symlinkCount === 0 &&
    JSON.stringify(value.encodingCoverage) === JSON.stringify(LOCAL_CREDENTIAL_ENCODING_COVERAGE)
}

export function sameLocalProviderAuthority(expected, observed) {
  return expected?.ok === true && observed?.ok === true && [
    'id', 'model', 'baseUrl', 'endpointFormat', 'registryRevision', 'registryIncarnation',
    'providerRevision', 'providerGeneration', 'providerIncarnation'
  ].every((key) => typeof expected[key] === 'string' && expected[key].length > 0 &&
    observed[key] === expected[key])
}

export function localProviderPublicSeamPassed(report) {
  return report.provider?.configured === true &&
  report.provider?.credentialConfigured === true &&
  report.provider?.credentialAuthorityBound === true &&
  report.provider?.normalLocalProviderSetupObserved === true &&
  report.provider?.credentialAuthority === LOCAL_PROVIDER_AUTHORITY &&
  localCredentialEvidencePassed(report.provider?.localCredentialEvidence) &&
  !Object.hasOwn(report.provider, 'normalHubLoginObserved') &&
  report.provider?.credentialRecorded === false &&
  report.provider?.networkTurnCompleted === true &&
  report.provider?.threadProviderBound === true &&
  report.provider?.threadModelBound === true &&
  report.provider?.resultTurnProviderReceiptBound === true &&
  report.provider?.providerAttemptTelemetryValid === true &&
  /^[0-9a-f]{64}$/.test(report.provider?.providerAttemptReceiptDigest || '') &&
  Number(report.provider?.providerLogicalCallCount || 0) > 0 &&
  Number(report.provider?.providerAttemptCount || 0) >=
    Number(report.provider?.providerLogicalCallCount || 0) &&
  Number(report.provider?.successfulProviderAttemptCount || 0) ===
    Number(report.provider?.providerLogicalCallCount || 0) &&
  report.syntheticProviderUsed === false &&
  report.localProviderUsed === false
}
