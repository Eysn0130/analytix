import {
  DEFAULT_MODEL_PROVIDER_ID,
  MODEL_PROVIDER_PRESETS,
  applyAnalytixRuntimePatch,
  getAnalytixRuntimeSettings,
  getModelProviderSettings,
  modelProviderPresetProfile,
  modelProviderTokenPlanProfile,
  normalizeAppSettings,
  tokenPlanProviderId,
  type AppSettingsV1,
  type AnalytixRuntimeSettingsPatchV1,
  type ModelProviderPreset,
  type ModelProviderProfileV1
} from '@shared/app-settings'
import type {
  ProviderRegistryRequest,
  ProviderRegistryResult
} from '@shared/analytix-api'
import { providerRegistryAccountObservationBindingSchemaV1 } from '../../../../packages/runtime/src/contracts/provider-registry.js'

export type InitialSetupAccessMode = 'api' | 'token-plan'

export type InitialSetupDraft = {
  apiKey: string
  baseUrl: string
}

/** Keyed by provider profile id (deepseek, xiaomi, xiaomi-token-plan, ...). */
export type InitialSetupDrafts = Record<string, InitialSetupDraft>

export type InitialSetupSelection = {
  presetId: string
  mode: InitialSetupAccessMode
}

export type CredentialDraftAction =
  | { kind: 'preserve'; value: null }
  | { kind: 'set'; value: string }

const CREDENTIAL_PLACEHOLDERS = new Set([
  'redacted', '[redacted]', '<redacted>', '__redacted__',
  'masked', '[masked]', '<masked>', '__masked__',
  'unset', 'not-set', 'not_set', 'null', 'undefined'
])
const CREDENTIAL_MASK_PATTERN = /^(?:[a-z0-9]+[-_:])?[*•●x]{4,}$/i

export function credentialDraftAction(value: unknown): CredentialDraftAction {
  const trimmed = typeof value === 'string' ? value.trim() : ''
  if (!trimmed || CREDENTIAL_PLACEHOLDERS.has(trimmed.toLowerCase()) || CREDENTIAL_MASK_PATTERN.test(trimmed)) {
    return { kind: 'preserve', value: null }
  }
  return { kind: 'set', value: trimmed }
}

const INITIAL_SETUP_PROVIDER_PRESET_IDS = new Set(['xiaomi', 'minimax'])

export const INITIAL_SETUP_PROVIDER_PRESETS = MODEL_PROVIDER_PRESETS.filter(
  (preset) => INITIAL_SETUP_PROVIDER_PRESET_IDS.has(preset.id)
)

export function initialSetupProfileId(selection: InitialSetupSelection): string {
  if (selection.presetId === DEFAULT_MODEL_PROVIDER_ID) return DEFAULT_MODEL_PROVIDER_ID
  return selection.mode === 'token-plan' ? tokenPlanProviderId(selection.presetId) : selection.presetId
}

/** Seed only key-free endpoints. Credential drafts always start empty. */
export function initialSetupDrafts(settings: AppSettingsV1): InitialSetupDrafts {
  const provider = getModelProviderSettings(settings)
  const byId = new Map(provider.providers.map((profile) => [profile.id, profile]))
  const deepseekProfile = byId.get(DEFAULT_MODEL_PROVIDER_ID)
  const drafts: InitialSetupDrafts = {
    [DEFAULT_MODEL_PROVIDER_ID]: {
      apiKey: '',
      baseUrl: deepseekProfile?.baseUrl ?? provider.baseUrl
    }
  }
  for (const preset of INITIAL_SETUP_PROVIDER_PRESETS) {
    const existing = byId.get(preset.id)
    drafts[preset.id] = {
      apiKey: '',
      baseUrl: existing?.baseUrl ?? preset.baseUrl
    }
    if (!preset.tokenPlan) continue
    const tokenPlanId = tokenPlanProviderId(preset.id)
    const existingTokenPlan = byId.get(tokenPlanId)
    drafts[tokenPlanId] = {
      apiKey: '',
      baseUrl: existingTokenPlan?.baseUrl ?? preset.tokenPlan.baseUrl
    }
  }
  return drafts
}

/** Card and mode to preselect: the active provider when it is one of ours, DeepSeek otherwise. */
export function initialSetupSelection(settings: AppSettingsV1): InitialSetupSelection {
  const provider = getModelProviderSettings(settings)
  const activeId = getAnalytixRuntimeSettings(settings).providerId.trim() || provider.activeProviderId
  for (const preset of INITIAL_SETUP_PROVIDER_PRESETS) {
    if (activeId === preset.id) return { presetId: preset.id, mode: 'api' }
    if (preset.tokenPlan && activeId === tokenPlanProviderId(preset.id)) {
      return { presetId: preset.id, mode: 'token-plan' }
    }
  }
  return { presetId: DEFAULT_MODEL_PROVIDER_ID, mode: 'api' }
}

export type InitialSetupAutoWirePlan = {
  speechProviderId: string
  imageProviderId: string
}

/**
 * Capabilities to point at a just-configured profile. Only fires while the
 * capability is still unconfigured — never overrides a user choice. Speech and
 * image generation can come from a pay-as-you-go profile or a token plan when
 * the provider exposes that capability to subscription keys.
 */
export function initialSetupAutoWirePlan(
  settings: AppSettingsV1,
  drafts: InitialSetupDrafts
): InitialSetupAutoWirePlan {
  const runtime = getAnalytixRuntimeSettings(settings)
  const speechUnconfigured = !runtime.speechToText.enabled && !runtime.speechToText.providerId.trim()
  const imageUnconfigured = !runtime.imageGeneration.enabled && !runtime.imageGeneration.providerId.trim()
  const plan: InitialSetupAutoWirePlan = { speechProviderId: '', imageProviderId: '' }
  for (const preset of INITIAL_SETUP_PROVIDER_PRESETS) {
    const apiKeyFilled = Boolean(drafts[preset.id]?.apiKey.trim())
    const tokenPlanKeyFilled = Boolean(
      preset.tokenPlan && drafts[tokenPlanProviderId(preset.id)]?.apiKey.trim()
    )
    if (speechUnconfigured && !plan.speechProviderId) {
      if (preset.speech && apiKeyFilled) {
        plan.speechProviderId = preset.id
      } else if (preset.tokenPlan?.speech && tokenPlanKeyFilled) {
        plan.speechProviderId = tokenPlanProviderId(preset.id)
      }
    }
    if (imageUnconfigured && !plan.imageProviderId) {
      if (preset.image && apiKeyFilled) {
        plan.imageProviderId = preset.id
      } else if (preset.tokenPlan?.image && tokenPlanKeyFilled) {
        plan.imageProviderId = tokenPlanProviderId(preset.id)
      }
    }
  }
  return plan
}

/**
 * Fold the onboarding drafts into settings: upsert one profile per filled
 * draft, activate the selected profile, and auto-wire speech/image to filled
 * pay-as-you-go profiles. The caller must ensure the selected draft has a key.
 */
export function buildInitialSetupSettings(
  settings: AppSettingsV1,
  drafts: InitialSetupDrafts,
  selection: InitialSetupSelection
): AppSettingsV1 {
  const provider = getModelProviderSettings(settings)
  const profiles = new Map(provider.providers.map((profile) => [profile.id, profile]))

  const deepseekDraft = drafts[DEFAULT_MODEL_PROVIDER_ID]
  const nextBaseUrl = deepseekDraft?.baseUrl.trim() ? deepseekDraft.baseUrl.trim() : provider.baseUrl
  const defaultProfile = profiles.get(DEFAULT_MODEL_PROVIDER_ID)
  if (defaultProfile) {
    profiles.set(DEFAULT_MODEL_PROVIDER_ID, {
      ...defaultProfile,
      baseUrl: nextBaseUrl
    })
  }

  for (const preset of INITIAL_SETUP_PROVIDER_PRESETS) {
    upsertPresetProfile(profiles, preset.id, drafts[preset.id], (baseUrl) => ({
      ...modelProviderPresetProfile(preset),
      ...(baseUrl ? { baseUrl } : {})
    }))
    if (!preset.tokenPlan) continue
    upsertPresetProfile(profiles, tokenPlanProviderId(preset.id), drafts[tokenPlanProviderId(preset.id)], (baseUrl) =>
      modelProviderTokenPlanProfile(preset, baseUrl)
    )
  }

  const selectedId = initialSetupProfileId(selection)
  const next = normalizeAppSettings({
    ...settings,
    provider: {
      activeProviderId: selectedId,
      baseUrl: nextBaseUrl,
      proxy: provider.proxy,
      providers: [...profiles.values()]
    }
  } as AppSettingsV1)

  const runtime = getAnalytixRuntimeSettings(next)
  const selectedProfile = getModelProviderSettings(next).providers.find(
    (profile) => profile.id === selectedId
  )
  const switchingProvider = (runtime.providerId.trim() || DEFAULT_MODEL_PROVIDER_ID) !== selectedId
  const wire = initialSetupAutoWirePlan(settings, drafts)
  const analytixPatch: AnalytixRuntimeSettingsPatchV1 = {
    providerId: selectedId,
    baseUrl: '',
    ...(switchingProvider && selectedProfile?.models[0] ? { model: selectedProfile.models[0] } : {}),
    ...(wire.speechProviderId
      ? { speechToText: { enabled: true, providerId: wire.speechProviderId } }
      : {}),
    ...(wire.imageProviderId
      ? { imageGeneration: { enabled: true, providerId: wire.imageProviderId } }
      : {})
  }
  return applyAnalytixRuntimePatch(next, analytixPatch)
}

type ProviderRegistryRequestFn = (request: ProviderRegistryRequest) => Promise<ProviderRegistryResult>

type ProviderRegistrySnapshot = Extract<ProviderRegistryResult, { providers: unknown }>

type ProviderRegistryProvider = ProviderRegistrySnapshot['providers'][number]

function registrySnapshot(result: ProviderRegistryResult): ProviderRegistrySnapshot {
  if ('error' in result) throw new Error(result.error.message)
  if (!('providers' in result)) throw new Error('The provider registry returned an invalid response.')
  return result as ProviderRegistrySnapshot
}

function registryProviderKind(profile: ModelProviderProfileV1): string {
  switch (profile.endpointFormat) {
    case 'responses': return 'openai-responses'
    case 'messages': return 'anthropic-compatible'
    case 'custom_endpoint': return 'custom-endpoint'
    default: return 'openai-compatible'
  }
}

function registryProviderEndpointFormat(kind: string): ModelProviderProfileV1['endpointFormat'] {
  switch (kind) {
    case 'openai-responses':
    case 'responses':
      return 'responses'
    case 'anthropic-compatible':
    case 'anthropic-messages':
    case 'messages':
      return 'messages'
    case 'custom-endpoint':
    case 'custom_endpoint':
      return 'custom_endpoint'
    default:
      return 'chat_completions'
  }
}

export function providerRegistryInput(
  profile: ModelProviderProfileV1,
  selectedModel: string,
  proxy: string,
  selection: {
    selectedMediaModel?: string
    selectedRoutes?: readonly string[]
    oauthBinding?: ProviderRegistryProvider['oauthBinding']
    accountObservation?: ProviderRegistryProvider['accountObservation']
  } = {}
) {
  const mediaModels = [
    ...(profile.image?.models ?? []),
    ...(profile.speech?.models ?? []),
    ...(profile.textToSpeech?.models ?? []),
    ...(profile.music?.models ?? []),
    ...(profile.video?.models ?? [])
  ].map((model) => model.trim()).filter((model, index, all) => Boolean(model) && all.indexOf(model) === index)
  const normalizedSelectedModel = profile.models.includes(selectedModel) ? selectedModel : ''
  const selectedMediaModel = selection.selectedMediaModel ?? ''
  const normalizedSelectedMediaModel = mediaModels.includes(selectedMediaModel) ? selectedMediaModel : ''
  return {
    id: profile.id,
    kind: registryProviderKind(profile),
    endpoint: profile.baseUrl,
    proxy,
    models: [...profile.models],
    mediaModels,
    selectedModel: normalizedSelectedModel,
    selectedMediaModel: normalizedSelectedMediaModel,
    selectedRoutes: [...(selection.selectedRoutes ?? [])],
    ...(selection.oauthBinding ? { oauthBinding: selection.oauthBinding } : {}),
    ...(selection.accountObservation ? { accountObservation: selection.accountObservation } : {})
  }
}

function registryExpected(
  snapshot: ProviderRegistrySnapshot,
  provider: ProviderRegistryProvider
) {
  return {
    registryRevision: snapshot.registryRevision,
    registryIncarnation: snapshot.registryIncarnation,
    providerRevision: provider.revision,
    providerGeneration: provider.generation,
    providerIncarnation: provider.incarnation,
    providerCredentialPurpose: provider.credentialPurpose ?? ''
  }
}

function providerInputMatches(
  input: ReturnType<typeof providerRegistryInput>,
  provider: ProviderRegistryProvider
): boolean {
  return input.id === provider.id && input.kind === provider.kind &&
    input.endpoint === provider.endpoint && input.proxy === (provider.proxy ?? '') &&
    JSON.stringify(input.models) === JSON.stringify(provider.models) &&
    JSON.stringify(input.mediaModels) === JSON.stringify(provider.mediaModels) &&
    input.selectedModel === (provider.selectedModel ?? '') &&
    input.selectedMediaModel === (provider.selectedMediaModel ?? '') &&
    JSON.stringify(input.selectedRoutes) === JSON.stringify(provider.selectedRoutes) &&
    JSON.stringify(input.oauthBinding) === JSON.stringify(provider.oauthBinding) &&
    JSON.stringify(input.accountObservation) === JSON.stringify(provider.accountObservation)
}

function encodeCredential(value: string): string {
  const bytes = new TextEncoder().encode(value)
  try {
    let binary = ''
    for (let offset = 0; offset < bytes.length; offset += 0x8000) {
      binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
    }
    return btoa(binary)
  } finally {
    bytes.fill(0)
  }
}

async function readRegistrySnapshot(request: ProviderRegistryRequestFn): Promise<ProviderRegistrySnapshot> {
  return registrySnapshot(await request({ schemaVersion: 1, operation: 'list' }))
}

async function syncProviderCredential(input: {
  request: ProviderRegistryRequestFn
  snapshot: ProviderRegistrySnapshot
  profile: ModelProviderProfileV1
  selectedModel: string
  selectedMediaModel?: string
  selectedRoutes?: readonly string[]
  proxy: string
  action: CredentialDraftAction
  updateMetadata?: boolean
}): Promise<ProviderRegistrySnapshot> {
  const existing = input.snapshot.providers.find((provider) => provider.id === input.profile.id)
  const provider = {
    ...providerRegistryInput(input.profile, input.selectedModel, input.proxy, {
      selectedMediaModel: input.selectedMediaModel ?? existing?.selectedMediaModel ?? '',
      selectedRoutes: input.selectedRoutes ?? existing?.selectedRoutes ?? [],
      oauthBinding: existing?.oauthBinding,
      accountObservation: existing?.accountObservation
    }),
    ...(existing && registryProviderEndpointFormat(existing.kind) === input.profile.endpointFormat
      ? { kind: existing.kind }
      : {})
  }
  if (!existing && input.action.kind === 'preserve') return input.snapshot
  let result: ProviderRegistryResult
  if (existing) {
    const metadataChanged = input.updateMetadata === true && !providerInputMatches(provider, existing)
    if (!metadataChanged && input.action.kind === 'preserve') return input.snapshot
    if (!metadataChanged && input.action.kind === 'set') {
      result = await input.request({
        schemaVersion: 1,
        operation: 'credential-replace',
        providerId: existing.id,
        expected: registryExpected(input.snapshot, existing),
        credential: {
          kind: 'set',
          purpose: 'provider-api-key',
          valueBase64: encodeCredential(input.action.value)
        }
      })
    } else {
      result = await input.request({
        schemaVersion: 1,
        operation: 'update',
        providerId: existing.id,
        expected: registryExpected(input.snapshot, existing),
        provider,
        credential: input.action.kind === 'set'
          ? {
              kind: 'set',
              purpose: 'provider-api-key',
              valueBase64: encodeCredential(input.action.value)
            }
          : { kind: 'keep' }
      })
    }
  } else {
    if (input.action.kind !== 'set') return input.snapshot
    result = await input.request({
      schemaVersion: 1,
      operation: 'connect',
      expected: {
        registryRevision: input.snapshot.registryRevision,
        registryIncarnation: input.snapshot.registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      provider,
      credential: {
        kind: 'set',
        purpose: 'provider-api-key',
        valueBase64: encodeCredential(input.action.value)
      }
    })
  }
  if ('error' in result) throw new Error(result.error.message)
  return readRegistrySnapshot(input.request)
}

async function persistProviderSettingsDraftInternal(input: {
  profile: ModelProviderProfileV1
  selectedModel: string
  selectedMediaModel?: string
  selectedRoutes?: readonly string[]
  proxy?: string
  credentialDraft: unknown
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<{
    credentialConfigured: boolean
    snapshot: ProviderRegistrySnapshot
    provider?: ProviderRegistryProvider
  }> {
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const next = await syncProviderCredential({
    request: input.requestProviderRegistry,
    snapshot,
    profile: input.profile,
    selectedModel: input.selectedModel,
    selectedMediaModel: input.selectedMediaModel,
    selectedRoutes: input.selectedRoutes,
    proxy: input.proxy?.trim() ?? '',
    action: credentialDraftAction(input.credentialDraft),
    updateMetadata: true
  })
  return {
    credentialConfigured: next.providers.find((provider) => provider.id === input.profile.id)
      ?.credentialConfigured === true,
    snapshot: next,
    provider: next.providers.find((provider) => provider.id === input.profile.id)
  }
}

export async function persistProviderSettingsDraft(input: {
  profile: ModelProviderProfileV1
  selectedModel: string
  selectedMediaModel?: string
  selectedRoutes?: readonly string[]
  proxy?: string
  credentialDraft: unknown
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<{
    credentialConfigured: boolean
    snapshot: ProviderRegistrySnapshot
    provider?: ProviderRegistryProvider
  }> {
  return persistProviderSettingsDraftInternal(input)
}

export async function persistProviderAccountObservationBinding(input: {
  providerId: string
  accountObservation: NonNullable<ProviderRegistryProvider['accountObservation']> | null
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<ProviderRegistrySnapshot> {
  const binding = input.accountObservation === null
    ? null
    : providerRegistryAccountObservationBindingSchemaV1.parse(input.accountObservation)
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const existing = snapshot.providers.find((provider) => provider.id === input.providerId)
  if (!existing || existing.tombstone) throw new Error('A current Provider is required.')
  if (JSON.stringify(existing.accountObservation ?? null) === JSON.stringify(binding)) return snapshot
  const result = await input.requestProviderRegistry({
    schemaVersion: 1,
    operation: 'update',
    providerId: existing.id,
    expected: registryExpected(snapshot, existing),
    provider: {
      id: existing.id,
      kind: existing.kind,
      endpoint: existing.endpoint,
      proxy: existing.proxy ?? '',
      models: [...existing.models],
      mediaModels: [...existing.mediaModels],
      selectedModel: existing.selectedModel ?? '',
      selectedMediaModel: existing.selectedMediaModel ?? '',
      selectedRoutes: [...existing.selectedRoutes],
      ...(existing.oauthBinding ? { oauthBinding: existing.oauthBinding } : {}),
      ...(binding ? { accountObservation: binding } : {})
    },
    credential: { kind: 'keep' }
  })
  if ('error' in result) throw new Error(result.error.message)
  if (!('provider' in result) || result.registryIncarnation !== snapshot.registryIncarnation ||
    result.registryRevision !== (BigInt(snapshot.registryRevision) + 1n).toString() ||
    result.provider.id !== existing.id ||
    result.provider.revision !== (BigInt(existing.revision) + 1n).toString()) {
    throw new Error('Provider account observation configuration could not be verified.')
  }
  const verified = await readRegistrySnapshot(input.requestProviderRegistry)
  const committed = verified.providers.find((provider) => provider.id === existing.id)
  if (!committed || committed.tombstone || verified.registryRevision !== result.registryRevision ||
    verified.registryIncarnation !== result.registryIncarnation ||
    verified.selectedProviderId !== snapshot.selectedProviderId || committed.revision !== result.provider.revision ||
    committed.kind !== existing.kind || committed.endpoint !== existing.endpoint ||
    (committed.proxy ?? '') !== (existing.proxy ?? '') ||
    JSON.stringify(committed.models) !== JSON.stringify(existing.models) ||
    JSON.stringify(committed.mediaModels) !== JSON.stringify(existing.mediaModels) ||
    (committed.selectedModel ?? '') !== (existing.selectedModel ?? '') ||
    (committed.selectedMediaModel ?? '') !== (existing.selectedMediaModel ?? '') ||
    JSON.stringify(committed.selectedRoutes) !== JSON.stringify(existing.selectedRoutes) ||
    JSON.stringify(committed.oauthBinding ?? null) !== JSON.stringify(existing.oauthBinding ?? null) ||
    JSON.stringify(committed.accountObservation ?? null) !== JSON.stringify(binding) ||
    committed.generation !== existing.generation || committed.incarnation !== existing.incarnation ||
    committed.credentialConfigured !== existing.credentialConfigured ||
    committed.credentialPurpose !== existing.credentialPurpose) {
    throw new Error('Provider account observation configuration could not be verified.')
  }
  return verified
}

export async function persistProviderCredentialDraft(input: {
  profile: ModelProviderProfileV1
  selectedModel: string
  selectedMediaModel?: string
  selectedRoutes?: readonly string[]
  proxy?: string
  credentialDraft: unknown
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<{ credentialConfigured: boolean }> {
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const next = await syncProviderCredential({
    request: input.requestProviderRegistry,
    snapshot,
    profile: input.profile,
    selectedModel: input.selectedModel,
    selectedMediaModel: input.selectedMediaModel,
    selectedRoutes: input.selectedRoutes,
    proxy: input.proxy?.trim() ?? '',
    action: credentialDraftAction(input.credentialDraft),
    updateMetadata: false
  })
  return {
    credentialConfigured: next.providers.find((provider) => provider.id === input.profile.id)
      ?.credentialConfigured === true
  }
}

export async function selectProviderRegistryEntry(input: {
  providerId: string
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<ProviderRegistrySnapshot> {
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const existing = snapshot.providers.find((provider) => provider.id === input.providerId)
  if (!existing || existing.tombstone || !existing.credentialConfigured) {
    throw new Error('A committed Provider credential is required.')
  }
  if (snapshot.selectedProviderId === existing.id) return snapshot
  const result = await input.requestProviderRegistry({
    schemaVersion: 1,
    operation: 'select',
    providerId: existing.id,
    expected: registryExpected(snapshot, existing)
  })
  if ('error' in result) throw new Error(result.error.message)
  const verified = await readRegistrySnapshot(input.requestProviderRegistry)
  if (verified.selectedProviderId !== existing.id) {
    throw new Error('Provider selection could not be verified.')
  }
  return verified
}

export async function disconnectProviderRegistryEntry(input: {
  providerId: string
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<ProviderRegistrySnapshot> {
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const existing = snapshot.providers.find((provider) => provider.id === input.providerId)
  if (!existing || existing.tombstone) return snapshot
  if (!existing.credentialConfigured || !existing.credentialPurpose) {
    throw new Error('A committed Provider credential is required.')
  }
  const result = await input.requestProviderRegistry({
    schemaVersion: 1,
    operation: 'disconnect',
    providerId: existing.id,
    expected: registryExpected(snapshot, existing)
  })
  if ('error' in result) throw new Error(result.error.message)
  const verified = await readRegistrySnapshot(input.requestProviderRegistry)
  const disconnected = verified.providers.find((provider) => provider.id === existing.id)
  if (!disconnected?.tombstone || disconnected.credentialConfigured) {
    throw new Error('Provider disconnect could not be verified.')
  }
  return verified
}

export async function persistInitialSetup(input: {
  settings: AppSettingsV1
  drafts: InitialSetupDrafts
  selection: InitialSetupSelection
  requestProviderRegistry: ProviderRegistryRequestFn
  saveSettings: (settings: AppSettingsV1) => Promise<AppSettingsV1>
}): Promise<AppSettingsV1> {
  const nextSettings = buildInitialSetupSettings(input.settings, input.drafts, input.selection)
  const providerSettings = getModelProviderSettings(nextSettings)
  const selectedId = initialSetupProfileId(input.selection)
  const proxy = providerSettings.proxy.enabled ? providerSettings.proxy.url.trim() : ''
  let snapshot = await readRegistrySnapshot(input.requestProviderRegistry)

  for (const profile of providerSettings.providers) {
    const action = credentialDraftAction(input.drafts[profile.id]?.apiKey)
    if (action.kind === 'preserve' && !snapshot.providers.some((provider) => provider.id === profile.id)) continue
    snapshot = await syncProviderCredential({
      request: input.requestProviderRegistry,
      snapshot,
      profile,
      selectedModel: nextSettings.runtime.model,
      proxy,
      action
    })
  }

  const selected = snapshot.providers.find((provider) => provider.id === selectedId)
  if (!selected || !selected.credentialConfigured) {
    throw new Error('A committed Provider credential is required.')
  }
  if (snapshot.selectedProviderId !== selectedId) {
    const result = await input.requestProviderRegistry({
      schemaVersion: 1,
      operation: 'select',
      providerId: selectedId,
      expected: {
        registryRevision: snapshot.registryRevision,
        registryIncarnation: snapshot.registryIncarnation,
        providerRevision: selected.revision,
        providerGeneration: selected.generation,
        providerIncarnation: selected.incarnation,
        providerCredentialPurpose: selected.credentialPurpose ?? ''
      }
    })
    if ('error' in result) throw new Error(result.error.message)
  }
  const verified = await readRegistrySnapshot(input.requestProviderRegistry)
  const verifiedSelected = verified.providers.find((provider) => provider.id === selectedId)
  if (
    verified.selectedProviderId !== selectedId ||
    !verifiedSelected ||
    verifiedSelected.tombstone ||
    !verifiedSelected.credentialConfigured
  ) {
    throw new Error('Provider selection could not be verified.')
  }
  return input.saveSettings(nextSettings)
}

export async function deleteProviderRegistryEntry(input: {
  providerId: string
  requestProviderRegistry: ProviderRegistryRequestFn
}): Promise<void> {
  const snapshot = await readRegistrySnapshot(input.requestProviderRegistry)
  const existing = snapshot.providers.find((provider) => provider.id === input.providerId)
  if (!existing) return
  const result = await input.requestProviderRegistry({
    schemaVersion: 1,
    operation: 'explicit-delete',
    providerId: existing.id,
    expected: registryExpected(snapshot, existing)
  })
  if ('error' in result) throw new Error(result.error.message)
  if (!('deletedProviderId' in result) || result.deletedProviderId !== existing.id) {
    throw new Error('The provider registry returned an invalid delete response.')
  }
  const verified = await readRegistrySnapshot(input.requestProviderRegistry)
  if (verified.providers.some((provider) => provider.id === existing.id)) {
    throw new Error('The provider registry delete could not be verified.')
  }
}

function upsertPresetProfile(
  profiles: Map<string, ModelProviderProfileV1>,
  id: string,
  draft: InitialSetupDraft | undefined,
  build: (baseUrl: string) => ModelProviderProfileV1 | null
): void {
  if (!credentialDraftAction(draft?.apiKey).value) return
  const built = build(draft?.baseUrl.trim() ?? '')
  if (!built) return
  const existing = profiles.get(id)
  profiles.set(id, existing
    ? {
        ...built,
        name: existing.name.trim() || built.name,
        models: mergeModelIds(built.models, existing.models)
      }
    : built)
}

function mergeModelIds(primary: readonly string[], secondary: readonly string[]): string[] {
  const ids = new Set<string>()
  for (const model of [...primary, ...secondary]) {
    const trimmed = model.trim()
    if (trimmed) ids.add(trimmed)
  }
  return [...ids]
}

export function presetForInitialSetup(presetId: string): ModelProviderPreset | null {
  return INITIAL_SETUP_PROVIDER_PRESETS.find((preset) => preset.id === presetId) ?? null
}
