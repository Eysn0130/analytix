import { providerDisplayName, providerEndpointKind, providerModelDisplayName } from '@shared/provider-display'
import { useEffect, useRef, useState, type ReactElement, type ReactNode } from 'react'
import type {
  AppSettingsPatch,
  ImageGenerationProtocol,
  AnalytixRuntimeSettingsPatchV1,
  AnalytixRuntimeSettingsV1,
  MusicGenerationProtocol,
  ModelEndpointFormat,
  ModelProviderImageCapabilityV1,
  ModelProviderModelProfileV1,
  ModelProviderMusicCapabilityV1,
  ModelProviderProfileV1,
  ModelProviderSettingsV1,
  ModelProviderSpeechCapabilityV1,
  ModelProviderTextToSpeechCapabilityV1,
  ModelProviderVideoCapabilityV1,
  SpeechToTextProtocol,
  TextToSpeechProtocol,
  VideoGenerationProtocol
} from '@shared/app-settings'
import {
  DEFAULT_IMAGE_GENERATION_PROTOCOL,
  DEFAULT_MUSIC_GENERATION_PROTOCOL,
  DEFAULT_MODEL_PROVIDER_ID,
  DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
  DEFAULT_TEXT_TO_SPEECH_PROTOCOL,
  DEFAULT_VIDEO_GENERATION_PROTOCOL,
  MODEL_ENDPOINT_FORMATS,
  MODEL_PROVIDER_PRESETS,
  TOKEN_PLAN_PROVIDER_ID_SUFFIX,
  defaultMiniMaxMediaGenerationAnalytixPatch,
  defaultModelProviderSettings,
  getModelProviderPreset,
  modelProviderPresetProfile,
  modelSupportsImageInput,
  modelProviderTokenPlanProfile,
  normalizeModelProviderId,
  tokenPlanProviderId
} from '@shared/app-settings'
import type { ModelProviderPreset } from '@shared/model-provider-presets'
import type {
  ProviderRegistryPublicProvider,
  ProviderRegistryResult
} from '@shared/analytix-api'
import {
  AudioLines,
  ChevronDown,
  ChevronUp,
  Clapperboard,
  Download,
  Image as ImageIcon,
  KeyRound,
  Loader2,
  Lock,
  Mic,
  Music2,
  PlugZap,
  Plus,
  Trash2,
  X
} from 'lucide-react'
import {
  InlineNoticeView,
  SecretInput,
  SettingsCard,
  SettingRow,
  Toggle,
  type InlineNotice
} from './settings-controls'
import { providerModelListEntries } from './provider-model-editor'
import { ProviderModelsManager } from './settings-section-provider-models'
import { useChatStore } from '../store/chat-store'
import {
  credentialDraftAction,
  deleteProviderRegistryEntry,
  disconnectProviderRegistryEntry,
  persistProviderAccountObservationBinding,
  persistProviderSettingsDraft,
  selectProviderRegistryEntry
} from './initial-setup-save'

const MODEL_ENDPOINT_FORMAT_LABEL_KEYS: Record<ModelEndpointFormat, string> = {
  chat_completions: 'modelEndpointChatCompletions',
  responses: 'modelEndpointResponses',
  messages: 'modelEndpointMessages',
  custom_endpoint: 'modelEndpointCustomEndpoint'
}

const IMAGE_GENERATION_PROTOCOL_LABEL_KEYS: Record<ImageGenerationProtocol, string> = {
  'openai-images': 'imageGenProtocolOpenAi',
  'minimax-image': 'imageGenProtocolMiniMax'
}

const SPEECH_TO_TEXT_PROTOCOL_LABEL_KEYS: Record<SpeechToTextProtocol, string> = {
  'openai-transcriptions': 'speechProtocolOpenAi',
  'mimo-asr': 'speechProtocolMimoAsr'
}

const TEXT_TO_SPEECH_PROTOCOL_LABEL_KEYS: Record<TextToSpeechProtocol, string> = {
  'openai-speech': 'textToSpeechProtocolOpenAi',
  'minimax-t2a': 'textToSpeechProtocolMiniMax',
  'mimo-tts': 'textToSpeechProtocolMimo'
}

const MUSIC_GENERATION_PROTOCOL_LABEL_KEYS: Record<MusicGenerationProtocol, string> = {
  'minimax-music': 'musicGenerationProtocolMiniMax'
}

const VIDEO_GENERATION_PROTOCOL_LABEL_KEYS: Record<VideoGenerationProtocol, string> = {
  'minimax-video': 'videoGenerationProtocolMiniMax'
}

export function modelProvidersSettingsPatch(input: {
  provider: ModelProviderSettingsV1
  providers: ModelProviderProfileV1[]
  analytix?: AnalytixRuntimeSettingsPatchV1
  currentAnalytix?: Partial<AnalytixRuntimeSettingsV1>
}): AppSettingsPatch {
  const miniMaxMediaDefaults = defaultMiniMaxMediaGenerationAnalytixPatch({
    providers: input.providers,
    currentAnalytix: input.currentAnalytix,
    analytixPatch: input.analytix
  })
  const baseAnalytixPatch = input.analytix?.providerId?.trim()
    ? { ...input.analytix, baseUrl: '' }
    : input.analytix ?? {}
  const analytixPatch = {
    ...baseAnalytixPatch,
    ...(miniMaxMediaDefaults ?? {})
  }
  return {
    ...(Object.keys(analytixPatch).length > 0 ? { runtime: analytixPatch } : {})
  }
}

function endpointFormatForRegistryKind(kind: string): ModelEndpointFormat {
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

export function providerProfileFromRegistry(provider: ProviderRegistryPublicProvider): ModelProviderProfileV1 {
  const preset = getModelProviderPreset(provider.id)
  const catalog = preset
    ? modelProviderPresetProfile(preset)
    : tokenPlanPresetForProfileId(provider.id)
      ? modelProviderTokenPlanProfile(tokenPlanPresetForProfileId(provider.id)!)
      : null
  const committedMedia = new Set(provider.mediaModels)
  const projectMediaCapability = <T extends { baseUrl: string; models: string[] }>(
    capability: T | undefined
  ): T | undefined => {
    if (!capability) return undefined
    const models = capability.models.filter((model) => committedMedia.has(model))
    return models.length > 0 ? { ...capability, baseUrl: provider.endpoint, models } : undefined
  }
  const catalogImage = projectMediaCapability(catalog?.image)
  const speech = projectMediaCapability(catalog?.speech)
  const textToSpeech = projectMediaCapability(catalog?.textToSpeech)
  const music = projectMediaCapability(catalog?.music)
  const video = projectMediaCapability(catalog?.video)
  const classifiedMedia = new Set([
    ...(catalogImage?.models ?? []),
    ...(speech?.models ?? []),
    ...(textToSpeech?.models ?? []),
    ...(music?.models ?? []),
    ...(video?.models ?? [])
  ])
  const imageModels = [
    ...(catalogImage?.models ?? []),
    ...provider.mediaModels.filter((model) => !classifiedMedia.has(model))
  ]
  return {
    ...(catalog ?? {
      id: provider.id,
      name: provider.id,
      baseUrl: provider.endpoint,
      endpointFormat: endpointFormatForRegistryKind(provider.kind),
      models: [],
      modelProfiles: {}
    }),
    id: provider.id,
    name: catalog?.name ?? provider.id,
    baseUrl: provider.endpoint,
    endpointFormat: endpointFormatForRegistryKind(provider.kind),
    models: [...provider.models],
    modelProfiles: Object.fromEntries(provider.models.flatMap((model) => {
      const profile = catalog?.modelProfiles[model]
      return profile ? [[model, profile]] : []
    })),
    image: imageModels.length > 0
      ? {
          ...(catalogImage ?? defaultImageCapability(provider.endpoint)),
          baseUrl: provider.endpoint,
          models: imageModels
        }
      : undefined,
    speech,
    textToSpeech,
    music,
    video
  }
}

type ProviderRegistrySnapshotResult = Extract<ProviderRegistryResult, { providers: unknown }>

export type ProviderRegistryRoutingDraft = {
  selectedModel: string
  selectedMediaModel: string
  routeProviderIds: string[]
}

const providerRegistryProviderIdPattern = /^[a-z0-9][a-z0-9._-]{0,95}$/
const exactProviderRoutePrefix = 'provider:'

function providerRegistryRouteProviderId(policyProviderId: string, route: string): string | null {
  if (route === 'primary') return policyProviderId
  if (route.startsWith(exactProviderRoutePrefix)) {
    const exactProviderId = route.slice(exactProviderRoutePrefix.length)
    return providerRegistryProviderIdPattern.test(exactProviderId) ? exactProviderId : null
  }
  return providerRegistryProviderIdPattern.test(route) ? route : null
}

export function providerRegistryRoutingDraft(
  provider: ProviderRegistryPublicProvider,
  providers: readonly ProviderRegistryPublicProvider[]
): ProviderRegistryRoutingDraft {
  const available = new Set(providers.flatMap((candidate) =>
    !candidate.tombstone && candidate.credentialConfigured && candidate.selectedModel !== undefined &&
      candidate.models.includes(candidate.selectedModel)
      ? [candidate.id]
      : []
  ))
  const routeProviderIds: string[] = []
  const seen = new Set<string>()
  const configuredRoutes = provider.selectedRoutes.length > 0 ? provider.selectedRoutes : ['primary']
  for (const route of configuredRoutes) {
    const providerId = providerRegistryRouteProviderId(provider.id, route)
    if (providerId === null || !available.has(providerId) || seen.has(providerId)) continue
    seen.add(providerId)
    routeProviderIds.push(providerId)
  }
  return {
    selectedModel: provider.selectedModel && provider.models.includes(provider.selectedModel)
      ? provider.selectedModel
      : '',
    selectedMediaModel: provider.selectedMediaModel && provider.mediaModels.includes(provider.selectedMediaModel)
      ? provider.selectedMediaModel
      : '',
    routeProviderIds
  }
}

export function providerRegistrySelectedRoutes(
  providerId: string,
  routeProviderIds: readonly string[]
): string[] {
  if (!providerRegistryProviderIdPattern.test(providerId)) {
    throw new Error('Provider route policy ID is invalid.')
  }
  const result: string[] = []
  const seen = new Set<string>()
  for (const routeProviderId of routeProviderIds) {
    if (!providerRegistryProviderIdPattern.test(routeProviderId)) {
      throw new Error('Provider route ID is invalid.')
    }
    if (seen.has(routeProviderId)) continue
    seen.add(routeProviderId)
    result.push(`${exactProviderRoutePrefix}${routeProviderId}`)
  }
  return result
}

export function providerRegistryRoutingDraftWithSelectedModel(
  draft: ProviderRegistryRoutingDraft,
  policyProviderId: string,
  selectedModel: string
): ProviderRegistryRoutingDraft {
  return {
    ...draft,
    selectedModel,
    routeProviderIds: selectedModel === ''
      ? draft.routeProviderIds.filter((providerId) => providerId !== policyProviderId)
      : draft.routeProviderIds.length === 0
        ? [policyProviderId]
        : draft.routeProviderIds
  }
}

export function providerRegistryRoutingDraftWithToggledProvider(
  draft: ProviderRegistryRoutingDraft,
  providerId: string
): ProviderRegistryRoutingDraft {
  const selected = draft.routeProviderIds.includes(providerId)
  if (selected && draft.routeProviderIds.length === 1 && draft.selectedModel !== '') return draft
  return {
    ...draft,
    routeProviderIds: selected
      ? draft.routeProviderIds.filter((candidate) => candidate !== providerId)
      : [...draft.routeProviderIds, providerId]
  }
}

function tokenPlanPresetForProfileId(id: string): ModelProviderPreset | null {
  if (!id.endsWith(TOKEN_PLAN_PROVIDER_ID_SUFFIX)) return null
  const preset = getModelProviderPreset(id.slice(0, -TOKEN_PLAN_PROVIDER_ID_SUFFIX.length))
  return preset?.tokenPlan ? preset : null
}

// 「套餐订阅」组 = Token Plan 套餐档(<id>-token-plan)或本身就是订阅制的预设(category==='subscription');
// 其余(默认 / 按量预设 / 自定义)归入「按量 API」组,便于一眼分辨两类计费方式。
function isSubscriptionProviderId(id: string): boolean {
  if (tokenPlanPresetForProfileId(id)) return true
  return getModelProviderPreset(id)?.category === 'subscription'
}

function mergeProviderModelIds(primary: readonly string[], secondary: readonly string[]): string[] {
  const ids = new Set<string>()
  for (const model of [...primary, ...secondary]) {
    const trimmed = model.trim()
    if (trimmed) ids.add(trimmed)
  }
  return [...ids]
}

function providerMediaModelIds(provider: ModelProviderProfileV1): string[] {
  return mergeProviderModelIds(
    [
      ...(provider.image?.models ?? []),
      ...(provider.speech?.models ?? []),
      ...(provider.textToSpeech?.models ?? [])
    ],
    [
      ...(provider.music?.models ?? []),
      ...(provider.video?.models ?? [])
    ]
  )
}

function providerModelCount(provider: ModelProviderProfileV1): number {
  return providerModelListEntries(provider).length
}

function defaultImageCapability(baseUrl: string): ModelProviderImageCapabilityV1 {
  return {
    protocol: DEFAULT_IMAGE_GENERATION_PROTOCOL,
    baseUrl: baseUrl.trim(),
    models: []
  }
}

function defaultSpeechCapability(baseUrl: string): ModelProviderSpeechCapabilityV1 {
  return {
    protocol: DEFAULT_SPEECH_TO_TEXT_PROTOCOL,
    baseUrl: baseUrl.trim(),
    models: []
  }
}

function defaultTextToSpeechCapability(baseUrl: string): ModelProviderTextToSpeechCapabilityV1 {
  return {
    protocol: DEFAULT_TEXT_TO_SPEECH_PROTOCOL,
    baseUrl: baseUrl.trim(),
    models: []
  }
}

function defaultMusicCapability(baseUrl: string): ModelProviderMusicCapabilityV1 {
  return {
    protocol: DEFAULT_MUSIC_GENERATION_PROTOCOL,
    baseUrl: baseUrl.trim(),
    models: []
  }
}

function defaultVideoCapability(baseUrl: string): ModelProviderVideoCapabilityV1 {
  return {
    protocol: DEFAULT_VIDEO_GENERATION_PROTOCOL,
    baseUrl: baseUrl.trim(),
    models: []
  }
}

function profileForModel(
  provider: Pick<ModelProviderProfileV1, 'modelProfiles'>,
  model: string
): ModelProviderModelProfileV1 | undefined {
  const trimmed = model.trim()
  if (!trimmed) return undefined
  return provider.modelProfiles[trimmed.toLowerCase()] ?? provider.modelProfiles[trimmed]
}

function presetImageCapability(providerId: string): ModelProviderImageCapabilityV1 | null {
  const preset = getModelProviderPreset(providerId)
  if (!preset?.image) return null
  return { protocol: preset.image.protocol, baseUrl: preset.image.baseUrl, models: [...preset.image.models] }
}

function presetSpeechCapability(provider: ModelProviderProfileV1): ModelProviderSpeechCapabilityV1 | null {
  const direct = getModelProviderPreset(provider.id)
  if (direct?.speech) {
    return { protocol: direct.speech.protocol, baseUrl: direct.speech.baseUrl, models: [...direct.speech.models] }
  }
  const tokenPlanSpeech = tokenPlanPresetForProfileId(provider.id)?.tokenPlan?.speech
  if (tokenPlanSpeech) {
    // 套餐端点自己提供 ASR,语音地址跟随该 profile 的服务地址。
    return { protocol: tokenPlanSpeech.protocol, baseUrl: provider.baseUrl, models: [...tokenPlanSpeech.models] }
  }
  return null
}

function presetTextToSpeechCapability(provider: ModelProviderProfileV1): ModelProviderTextToSpeechCapabilityV1 | null {
  const direct = getModelProviderPreset(provider.id)
  if (direct?.textToSpeech) {
    return {
      protocol: direct.textToSpeech.protocol,
      baseUrl: direct.textToSpeech.baseUrl,
      models: [...direct.textToSpeech.models]
    }
  }
  const tokenPlanTextToSpeech = tokenPlanPresetForProfileId(provider.id)?.tokenPlan?.textToSpeech
  if (tokenPlanTextToSpeech) {
    return {
      protocol: tokenPlanTextToSpeech.protocol,
      baseUrl: tokenPlanTextToSpeech.baseUrl ?? provider.baseUrl,
      models: [...tokenPlanTextToSpeech.models]
    }
  }
  return null
}

function presetMusicCapability(provider: ModelProviderProfileV1): ModelProviderMusicCapabilityV1 | null {
  const direct = getModelProviderPreset(provider.id)
  if (direct?.music) {
    return { protocol: direct.music.protocol, baseUrl: direct.music.baseUrl, models: [...direct.music.models] }
  }
  const tokenPlanMusic = tokenPlanPresetForProfileId(provider.id)?.tokenPlan?.music
  if (tokenPlanMusic) {
    return { protocol: tokenPlanMusic.protocol, baseUrl: tokenPlanMusic.baseUrl, models: [...tokenPlanMusic.models] }
  }
  return null
}

function presetVideoCapability(provider: ModelProviderProfileV1): ModelProviderVideoCapabilityV1 | null {
  const direct = getModelProviderPreset(provider.id)
  if (direct?.video) {
    return { protocol: direct.video.protocol, baseUrl: direct.video.baseUrl, models: [...direct.video.models] }
  }
  const tokenPlanVideo = tokenPlanPresetForProfileId(provider.id)?.tokenPlan?.video
  if (tokenPlanVideo) {
    return { protocol: tokenPlanVideo.protocol, baseUrl: tokenPlanVideo.baseUrl, models: [...tokenPlanVideo.models] }
  }
  return null
}

function isAcceptableHttpUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return true
  if (!/^https?:\/\//i.test(trimmed)) return false
  try {
    new URL(trimmed)
    return true
  } catch {
    return false
  }
}

function providerConnectionFingerprint(provider: ModelProviderProfileV1): string {
  return [provider.id, provider.baseUrl, provider.endpointFormat].join('\0')
}

type ProbeState = {
  fingerprint: string
  mode: 'test' | 'fetch'
  status: 'busy' | 'ok' | 'error'
  latencyMs?: number
  total?: number
  message?: string
}

type ProviderAccountObservationResult = Extract<ProviderRegistryResult, { observedAt: unknown }>

export type AccountObservationState = {
  requestId: number
  selectedProviderId: string
  registryRevision: string
  registryIncarnation: string
  providerId: string
  providerRevision: string
  providerGeneration: string
  providerIncarnation: string
  providerCredentialPurpose: string
  accountObservation: ProviderRegistryPublicProvider['accountObservation'] | null
  status: 'busy' | ProviderAccountObservationResult['status']
  observedAt?: string
  expiresAt?: string
  quota?: number
  usage?: number
  remaining?: number
}

export function providerAccountObservationStateIsCurrent(
  state: AccountObservationState,
  snapshot: ProviderRegistrySnapshotResult,
  nowMs = Date.now()
): boolean {
  const provider = snapshot.providers.find((candidate) => candidate.id === state.providerId)
  if (!provider || provider.tombstone || !provider.credentialConfigured ||
    snapshot.selectedProviderId !== state.selectedProviderId ||
    snapshot.registryRevision !== state.registryRevision ||
    snapshot.registryIncarnation !== state.registryIncarnation ||
    provider.revision !== state.providerRevision || provider.generation !== state.providerGeneration ||
    provider.incarnation !== state.providerIncarnation ||
    (provider.credentialPurpose ?? '') !== state.providerCredentialPurpose ||
    JSON.stringify(provider.accountObservation ?? null) !== JSON.stringify(state.accountObservation)) {
    return false
  }
  if (state.status === 'busy') return true
  const observedAtMs = Date.parse(state.observedAt ?? '')
  const expiresAtMs = Date.parse(state.expiresAt ?? '')
  return Number.isFinite(observedAtMs) && Number.isFinite(expiresAtMs) &&
    observedAtMs <= nowMs && expiresAtMs > nowMs
}

export function providerAccountObservationStateFromResult(input: {
  requestId: number
  expectedSnapshot: ProviderRegistrySnapshotResult
  expectedProvider: ProviderRegistryPublicProvider
  observation: ProviderAccountObservationResult
  currentSnapshot: ProviderRegistrySnapshotResult
  nowMs?: number
}): AccountObservationState | null {
  const { expectedSnapshot, expectedProvider, observation, currentSnapshot } = input
  if (observation.providerId !== expectedProvider.id ||
    observation.registryRevision !== expectedSnapshot.registryRevision ||
    observation.registryIncarnation !== expectedSnapshot.registryIncarnation ||
    observation.providerRevision !== expectedProvider.revision ||
    observation.providerGeneration !== expectedProvider.generation ||
    observation.providerIncarnation !== expectedProvider.incarnation ||
    observation.providerCredentialPurpose !== expectedProvider.credentialPurpose) {
    return null
  }
  const state: AccountObservationState = {
    requestId: input.requestId,
    selectedProviderId: expectedSnapshot.selectedProviderId ?? '',
    registryRevision: observation.registryRevision,
    registryIncarnation: observation.registryIncarnation,
    providerId: observation.providerId,
    providerRevision: observation.providerRevision,
    providerGeneration: observation.providerGeneration,
    providerIncarnation: observation.providerIncarnation,
    providerCredentialPurpose: expectedProvider.credentialPurpose ?? '',
    accountObservation: expectedProvider.accountObservation ?? null,
    status: observation.status,
    observedAt: observation.observedAt,
    expiresAt: observation.expiresAt,
    ...(observation.quota !== undefined ? { quota: observation.quota } : {}),
    ...(observation.usage !== undefined ? { usage: observation.usage } : {}),
    ...(observation.remaining !== undefined ? { remaining: observation.remaining } : {})
  }
  return providerAccountObservationStateIsCurrent(state, currentSnapshot, input.nowMs) ? state : null
}

const fieldLabelClass = 'grid gap-1.5 text-[12px] font-semibold text-ds-muted'
const textInputClass =
  'w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[14px] font-normal text-ds-ink shadow-sm focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30'

function DetailSection({
  title,
  action,
  children
}: {
  title: string
  action?: ReactNode
  children?: ReactNode
}): ReactElement {
  return (
    <section className="grid gap-3 border-t border-ds-border-muted pt-3 first:border-t-0 first:pt-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-[12.5px] font-semibold text-ds-muted">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}

function ProviderBadge({
  tone,
  children
}: {
  tone: 'accent' | 'warning'
  children: ReactNode
}): ReactElement {
  const toneClass =
    tone === 'accent'
      ? 'border-emerald-300/70 bg-emerald-50 text-emerald-700 dark:border-emerald-800/70 dark:bg-emerald-950/30 dark:text-emerald-300'
      : 'border-amber-300/70 bg-amber-50 text-amber-700 dark:border-amber-800/70 dark:bg-amber-950/30 dark:text-amber-300'
  return (
    <span className={`inline-flex shrink-0 items-center rounded-full border px-2 py-0.5 text-[11px] font-medium leading-4 ${toneClass}`}>
      {children}
    </span>
  )
}

function ProviderListGroup({
  label,
  count,
  children
}: {
  label: string
  count: number
  children: ReactNode
}): ReactElement {
  return (
    <div className="grid gap-2">
      <div className="flex items-center gap-2 px-1">
        <span className="text-[11.5px] font-semibold text-ds-muted">{label}</span>
        <span className="inline-flex h-4 min-w-4 items-center justify-center rounded-full bg-ds-main/60 px-1.5 text-[10.5px] font-medium text-ds-faint">
          {count}
        </span>
      </div>
      {children}
    </div>
  )
}

function ModelChipsInput({
  values,
  onChange,
  placeholder,
  inputAriaLabel,
  removeLabel
}: {
  values: string[]
  onChange: (next: string[]) => void
  placeholder: string
  inputAriaLabel: string
  removeLabel: (model: string) => string
}): ReactElement {
  const [draft, setDraft] = useState('')

  const commit = (raw: string): void => {
    const ids = raw.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean)
    setDraft('')
    if (ids.length === 0) return
    const seen = new Set(values)
    const next = [...values]
    for (const id of ids) {
      if (seen.has(id)) continue
      seen.add(id)
      next.push(id)
    }
    if (next.length !== values.length) onChange(next)
  }

  const removeAt = (index: number): void => {
    onChange(values.filter((_, i) => i !== index))
  }

  return (
    <div className="flex w-full min-w-0 flex-wrap items-center gap-1.5 rounded-xl border border-ds-border bg-ds-card px-2 py-1.5 shadow-sm focus-within:border-accent/40 focus-within:ring-1 focus-within:ring-accent/30">
      {values.map((model, index) => (
        <span
          key={`${model}-${index}`}
          className="inline-flex max-w-full items-center gap-1 rounded-full border border-ds-border-muted bg-ds-main/60 py-0.5 pl-2.5 pr-1 font-mono text-[12px] text-ds-ink"
        >
          <span className="truncate">{model}</span>
          <button
            type="button"
            aria-label={removeLabel(model)}
            onClick={() => removeAt(index)}
            className="rounded-full p-0.5 text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
          >
            <X className="h-3 w-3" strokeWidth={2} />
          </button>
        </span>
      ))}
      <input
        className="min-w-[150px] flex-1 bg-transparent px-1 py-1 font-mono text-[12.5px] font-normal text-ds-ink placeholder:text-ds-faint focus:outline-none"
        value={draft}
        placeholder={placeholder}
        aria-label={inputAriaLabel}
        spellCheck={false}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ',') {
            e.preventDefault()
            commit(draft)
          } else if (e.key === 'Backspace' && !draft && values.length > 0) {
            e.preventDefault()
            removeAt(values.length - 1)
          }
        }}
        onBlur={() => commit(draft)}
        onPaste={(e) => {
          const text = e.clipboardData.getData('text')
          if (/[\s,]/.test(text)) {
            e.preventDefault()
            commit(`${draft} ${text}`)
          }
        }}
      />
    </div>
  )
}

export function ProvidersSettingsSection({ ctx }: { ctx: Record<string, any> }): ReactElement {
  const {
    t,
    form,
    provider: providerFromContext,
    analytix,
    update,
    selectControlClass
  } = ctx
  const showTopNotice = useChatStore((s) => s.showTopNotice)
  const provider = providerFromContext ?? defaultModelProviderSettings()
  const [registrySnapshot, setRegistrySnapshot] = useState<ProviderRegistrySnapshotResult | null>(null)
  const [registryLoadState, setRegistryLoadState] = useState<'loading' | 'ready' | 'unavailable'>('loading')
  const registryLoadSequence = useRef(0)
  const [modelProviders, setModelProviders] = useState<ModelProviderProfileV1[]>([])
  const [routingDrafts, setRoutingDrafts] = useState<Record<string, ProviderRegistryRoutingDraft>>({})
  const [selectedProviderId, setSelectedProviderId] = useState<string>('')
  const [addMenuOpen, setAddMenuOpen] = useState(false)
  const addMenuRef = useRef<HTMLDivElement>(null)
  // 点击菜单外部或按 Esc 关闭「添加供应商」下拉。用监听器代替全屏遮罩:全屏 fixed 遮罩会吞掉滚轮事件,
  // 导致下拉打开时整个设置页无法滚动(用户反馈的 bug)。
  useEffect(() => {
    if (!addMenuOpen) return
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (target instanceof Node && addMenuRef.current?.contains(target)) return
      setAddMenuOpen(false)
    }
    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') setAddMenuOpen(false)
    }
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [addMenuOpen])
  const [probeStates, setProbeStates] = useState<Record<string, ProbeState>>({})
  const [accountObservationStates, setAccountObservationStates] = useState<Record<string, AccountObservationState>>({})
  const accountObservationRequestSequence = useRef(0)
  const latestAccountObservationRequest = useRef<Record<string, number>>({})
  const [accountObservationEndpointDrafts, setAccountObservationEndpointDrafts] = useState<Record<string, string>>({})
  const [showApiKey, setShowApiKey] = useState(false)
  const [credentialDrafts, setCredentialDrafts] = useState<Record<string, string>>({})
  const credentialInputRevisions = useRef<Record<string, number>>({})
  const credentialEntryRevision = useRef(0)
  const credentialOperationRevision = useRef(0)
  const credentialInputMounted = useRef(false)
  useEffect(() => {
    credentialInputMounted.current = true
    return () => {
      credentialInputMounted.current = false
      credentialEntryRevision.current += 1
      credentialInputRevisions.current = {}
    }
  }, [])

  // These revisions fence local editing continuations, not Registry authority.
  const invalidateCredentialEntry = (): void => {
    credentialEntryRevision.current += 1
    setShowApiKey(false)
  }
  const clearCredentialDraft = (providerId: string): void => {
    credentialInputRevisions.current[providerId] = (credentialInputRevisions.current[providerId] ?? 0) + 1
    setCredentialDrafts((current) => {
      const next = { ...current }
      delete next[providerId]
      return next
    })
  }
  const selectProviderForEditing = (providerId: string): void => {
    if (providerId !== selectedProviderId) invalidateCredentialEntry()
    setSelectedProviderId(providerId)
  }
  const beginCredentialOperation = (providerId: string) => ({
    entry: credentialEntryRevision.current,
    operation: ++credentialOperationRevision.current,
    input: credentialInputRevisions.current[providerId] ?? 0
  })
  const credentialOperationIsCurrent = (operation: ReturnType<typeof beginCredentialOperation>): boolean =>
    credentialInputMounted.current && operation.entry === credentialEntryRevision.current &&
    operation.operation === credentialOperationRevision.current

  const [credentialConfiguredProviderIds, setCredentialConfiguredProviderIds] = useState<Set<string>>(new Set())
  const [providerProxyDrafts, setProviderProxyDrafts] = useState<Record<string, string>>({})
  const [providerProxyEnabled, setProviderProxyEnabled] = useState<Record<string, boolean>>({})
  const [protectedRecoveryBusy, setProtectedRecoveryBusy] = useState<string | null>(null)
  const [protectedRecoveryStatus, setProtectedRecoveryStatus] = useState<string | null>(null)

  const runProtectedRecoveryAction = async (
    action: 'createDestinationRequest' | 'createSourceBundle' | 'applyDestinationBundle' | 'finalizeSourceReceipt'
  ): Promise<void> => {
    if (protectedRecoveryBusy) return
    setProtectedRecoveryBusy(action)
    setProtectedRecoveryStatus(null)
    try {
      const result = await window.analytix.providerCredentialRecovery[action]()
      setProtectedRecoveryStatus(result.ok ? result.status : result.code)
    } catch {
      setProtectedRecoveryStatus('unavailable')
    } finally {
      setProtectedRecoveryBusy(null)
    }
  }

  const applyRegistrySnapshot = (snapshot: ProviderRegistrySnapshotResult): void => {
    setRegistryLoadState('ready')
    setRegistrySnapshot(snapshot)
    setModelProviders(snapshot.providers.map(providerProfileFromRegistry))
    setRoutingDrafts(Object.fromEntries(
      snapshot.providers.map((provider) => [provider.id, providerRegistryRoutingDraft(provider, snapshot.providers)])
    ))
    setCredentialConfiguredProviderIds(new Set(
      snapshot.providers.filter((provider) => provider.credentialConfigured).map((provider) => provider.id)
    ))
    setProviderProxyDrafts(Object.fromEntries(
      snapshot.providers.map((provider) => [provider.id, provider.proxy ?? ''])
    ))
    setProviderProxyEnabled(Object.fromEntries(
      snapshot.providers.map((provider) => [provider.id, Boolean(provider.proxy)])
    ))
    setAccountObservationEndpointDrafts(Object.fromEntries(
      snapshot.providers.map((provider) => [provider.id, provider.accountObservation?.endpoint ?? ''])
    ))
    setSelectedProviderId((current) => {
      if (snapshot.providers.some((provider) => provider.id === current)) return current
      return snapshot.selectedProviderId ?? snapshot.providers[0]?.id ?? ''
    })
  }

  const refreshRegistry = async (): Promise<ProviderRegistrySnapshotResult> => {
    const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' })
    if ('error' in result || !('providers' in result)) {
      throw new Error('The provider registry returned an invalid response.')
    }
    applyRegistrySnapshot(result)
    return result
  }

  const loadRegistry = async (): Promise<void> => {
    const sequence = ++registryLoadSequence.current
    setRegistryLoadState('loading')
    try {
      const result = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' })
      if (!credentialInputMounted.current || sequence !== registryLoadSequence.current) return
      if ('error' in result || !('providers' in result)) {
        setRegistryLoadState('unavailable')
        return
      }
      applyRegistrySnapshot(result)
    } catch {
      if (credentialInputMounted.current && sequence === registryLoadSequence.current) {
        setRegistryLoadState('unavailable')
      }
    }
  }
  useEffect(() => {
    void loadRegistry()
    return () => { registryLoadSequence.current += 1 }
  }, [])
  useEffect(() => {
    const expiries = Object.values(accountObservationStates)
      .filter((state) => state.status !== 'busy')
      .map((state) => Date.parse(state.expiresAt ?? ''))
      .filter((value) => Number.isFinite(value))
    if (expiries.length === 0) return
    const delay = Math.max(0, Math.min(...expiries) - Date.now() + 1)
    const timeout = window.setTimeout(() => {
      const now = Date.now()
      setAccountObservationStates((current) => Object.fromEntries(
        Object.entries(current).filter(([, state]) =>
          state.status === 'busy' || Date.parse(state.expiresAt ?? '') > now
        )
      ))
    }, Math.min(delay, 2_147_483_647))
    return () => window.clearTimeout(timeout)
  }, [accountObservationStates])
  // 新增供应商先停留在本地草稿,点「添加」才写入设置,避免半配置状态被持久化。
  const [draftProvider, setDraftProvider] = useState<ModelProviderProfileV1 | null>(null)
  const displayProviders = draftProvider ? [...modelProviders, draftProvider] : modelProviders
  const activeProvider =
    displayProviders.find((item) => item.id === selectedProviderId) ??
    modelProviders[0]
  const isDraftActive = Boolean(draftProvider && activeProvider?.id === draftProvider.id)
  const canEditActiveProviderId = Boolean(
    activeProvider && isDraftActive
  )
  const activeRegistryProvider = registrySnapshot?.providers.find(
    (item) => item.id === activeProvider?.id
  )
  const canConfigureAccountObservation = Boolean(
    activeRegistryProvider && !activeRegistryProvider.tombstone &&
    !getModelProviderPreset(activeRegistryProvider.id) && !tokenPlanPresetForProfileId(activeRegistryProvider.id)
  )
  const activeRoutingDraft: ProviderRegistryRoutingDraft | null = activeProvider
    ? routingDrafts[activeProvider.id] ?? {
        selectedModel: activeProvider.models.includes(analytix.model)
          ? analytix.model
          : activeProvider.models[0] ?? '',
        selectedMediaModel: '',
        routeProviderIds: [activeProvider.id]
      }
    : null
  const activeMediaModelIds = activeProvider ? providerMediaModelIds(activeProvider) : []
  const effectiveSelectedModel = activeProvider && activeRoutingDraft &&
    activeProvider.models.includes(activeRoutingDraft.selectedModel)
    ? activeRoutingDraft.selectedModel
    : ''
  const effectiveSelectedMediaModel = activeRoutingDraft &&
    activeMediaModelIds.includes(activeRoutingDraft.selectedMediaModel)
    ? activeRoutingDraft.selectedMediaModel
    : ''
  const availableRouteProviderIds = (registrySnapshot?.providers ?? []).flatMap((candidate) =>
    !candidate.tombstone && candidate.credentialConfigured && candidate.selectedModel !== undefined &&
      candidate.models.includes(candidate.selectedModel)
      ? [candidate.id]
      : []
  )
  const activeProviderCredentialConfigured = activeRegistryProvider?.credentialConfigured === true ||
    (activeProvider ? credentialDraftAction(credentialDrafts[activeProvider.id]).kind === 'set' : false)
  if (activeProvider && (isDraftActive || activeRegistryProvider?.tombstone === false) &&
    effectiveSelectedModel !== '' && activeProviderCredentialConfigured &&
    !availableRouteProviderIds.includes(activeProvider.id)) {
    availableRouteProviderIds.push(activeProvider.id)
  }
  const activeAnalytixProviderId: string = registrySnapshot?.selectedProviderId ?? ''
  const providerProxy = activeProvider
    ? {
        enabled: providerProxyEnabled[activeProvider.id] === true,
        url: providerProxyDrafts[activeProvider.id] ?? ''
      }
    : { enabled: false, url: '' }

  const updateProviderProxy = (patch: Partial<typeof providerProxy>): void => {
    if (!activeProvider) return
    if (patch.enabled !== undefined) {
      setProviderProxyEnabled((current) => ({ ...current, [activeProvider.id]: patch.enabled === true }))
    }
    if (patch.url !== undefined) {
      setProviderProxyDrafts((current) => ({ ...current, [activeProvider.id]: patch.url ?? '' }))
    }
  }

  const updateActiveRoutingDraft = (patch: Partial<ProviderRegistryRoutingDraft>): void => {
    if (!activeProvider || !activeRoutingDraft) return
    setRoutingDrafts((current) => ({
      ...current,
      [activeProvider.id]: { ...activeRoutingDraft, ...patch }
    }))
  }

  const toggleActiveRouteProvider = (providerId: string): void => {
    if (!activeRoutingDraft) return
    updateActiveRoutingDraft(providerRegistryRoutingDraftWithToggledProvider(activeRoutingDraft, providerId))
  }

  const moveActiveRouteProvider = (providerId: string, offset: -1 | 1): void => {
    if (!activeRoutingDraft) return
    const index = activeRoutingDraft.routeProviderIds.indexOf(providerId)
    const nextIndex = index + offset
    if (index < 0 || nextIndex < 0 || nextIndex >= activeRoutingDraft.routeProviderIds.length) return
    const routeProviderIds = [...activeRoutingDraft.routeProviderIds]
    ;[routeProviderIds[index], routeProviderIds[nextIndex]] = [
      routeProviderIds[nextIndex]!,
      routeProviderIds[index]!
    ]
    updateActiveRoutingDraft({ routeProviderIds })
  }

  const confirmAction = async (options: {
    message: string
    detail?: string
    confirmLabel?: string
    cancelLabel?: string
  }): Promise<boolean> => {
    if (typeof window.analytix?.app?.confirmDialog === 'function') {
      return window.analytix.app.confirmDialog(options)
    }
    return true
  }

  const updateModelProviders = (
    providers: ModelProviderProfileV1[],
    analytixPatch?: AnalytixRuntimeSettingsPatchV1
  ): void => {
    setModelProviders(providers)
    if (analytixPatch) {
      update(modelProvidersSettingsPatch({
        provider,
        providers,
        analytix: analytixPatch,
        currentAnalytix: analytix
      }))
    }
  }

  const patchProviderProfile = (
    item: ModelProviderProfileV1,
    transform: (item: ModelProviderProfileV1) => ModelProviderProfileV1
  ): void => {
    if (draftProvider && item.id === draftProvider.id) {
      setDraftProvider(transform(draftProvider))
      return
    }
    updateModelProviders(modelProviders.map((existing) => existing.id === item.id ? transform(existing) : existing))
  }

  const updateModelProvider = (id: string, patch: Partial<ModelProviderProfileV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({ ...item, ...patch }))
  }

  const updateModelProviderImage = (id: string, patch: Partial<ModelProviderImageCapabilityV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({
      ...item,
      image: {
        ...(item.image ?? defaultImageCapability(item.baseUrl)),
        ...patch
      }
    }))
  }

  const removeModelProviderImage = (id: string): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => {
      const { image: _image, ...rest } = item
      void _image
      return rest
    })
  }

  const updateModelProviderSpeech = (id: string, patch: Partial<ModelProviderSpeechCapabilityV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({
      ...item,
      speech: {
        ...(item.speech ?? defaultSpeechCapability(item.baseUrl)),
        ...patch
      }
    }))
  }

  const removeModelProviderSpeech = (id: string): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => {
      const { speech: _speech, ...rest } = item
      void _speech
      return rest
    })
  }

  const updateModelProviderTextToSpeech = (id: string, patch: Partial<ModelProviderTextToSpeechCapabilityV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({
      ...item,
      textToSpeech: {
        ...(item.textToSpeech ?? defaultTextToSpeechCapability(item.baseUrl)),
        ...patch
      }
    }))
  }

  const removeModelProviderTextToSpeech = (id: string): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => {
      const { textToSpeech: _textToSpeech, ...rest } = item
      void _textToSpeech
      return rest
    })
  }

  const updateModelProviderMusic = (id: string, patch: Partial<ModelProviderMusicCapabilityV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({
      ...item,
      music: {
        ...(item.music ?? defaultMusicCapability(item.baseUrl)),
        ...patch
      }
    }))
  }

  const removeModelProviderMusic = (id: string): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => {
      const { music: _music, ...rest } = item
      void _music
      return rest
    })
  }

  const updateModelProviderVideo = (id: string, patch: Partial<ModelProviderVideoCapabilityV1>): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => ({
      ...item,
      video: {
        ...(item.video ?? defaultVideoCapability(item.baseUrl)),
        ...patch
      }
    }))
  }

  const removeModelProviderVideo = (id: string): void => {
    const target = displayProviders.find((item) => item.id === id)
    if (!target) return
    patchProviderProfile(target, (item) => {
      const { video: _video, ...rest } = item
      void _video
      return rest
    })
  }

  const updateModelProviderId = (id: string, value: string): void => {
    if (id === DEFAULT_MODEL_PROVIDER_ID) return
    const nextId = normalizeModelProviderId(value)
    if (!nextId || nextId === id) return
    if (displayProviders.some((item) => item.id === nextId && item.id !== id)) return
    if (draftProvider && id === draftProvider.id) {
      invalidateCredentialEntry()
      clearCredentialDraft(id)
      clearCredentialDraft(nextId)
      setSelectedProviderId(nextId)
      setDraftProvider({ ...draftProvider, id: nextId })
      return
    }
    selectProviderForEditing(nextId)
    updateModelProviders(
      modelProviders.map((item) => item.id === id ? { ...item, id: nextId } : item),
      analytix.providerId === id ? { providerId: nextId } : undefined
    )
  }

  const startProviderDraft = (profile: ModelProviderProfileV1): void => {
    invalidateCredentialEntry()
    if (draftProvider) clearCredentialDraft(draftProvider.id)
    clearCredentialDraft(profile.id)
    setDraftProvider(profile)
    setSelectedProviderId(profile.id)
  }

  const persistCredentialDraft = async (
    profile: ModelProviderProfileV1,
    operation = beginCredentialOperation(profile.id)
  ): Promise<boolean> => {
    const registryProvider = registrySnapshot?.providers.find((provider) => provider.id === profile.id)
    if (registryProvider?.tombstone) {
      showTopNotice({ tone: 'error', message: 'Disconnected Providers must be explicitly deleted before reconnecting.' })
      return false
    }
    const routingDraft = routingDrafts[profile.id] ?? {
      selectedModel: registryProvider?.selectedModel ??
        (profile.models.includes(analytix.model) ? analytix.model : profile.models[0] ?? ''),
      selectedMediaModel: registryProvider?.selectedMediaModel ?? '',
      routeProviderIds: registryProvider
        ? providerRegistryRoutingDraft(registryProvider, registrySnapshot?.providers ?? []).routeProviderIds
        : [profile.id]
    }
    const selectedModel = profile.models.includes(routingDraft.selectedModel) ? routingDraft.selectedModel : ''
    if (routingDraft.routeProviderIds.length === 0 && selectedModel !== '') {
      showTopNotice({ tone: 'error', message: t('modelProviderRouteRequired') })
      return false
    }
    if (routingDraft.routeProviderIds.includes(profile.id) && selectedModel === '') {
      showTopNotice({ tone: 'error', message: t('modelProviderRouteRequired') })
      return false
    }
    if (routingDraft.routeProviderIds.includes(profile.id) && registryProvider?.credentialConfigured !== true &&
      credentialDraftAction(credentialDrafts[profile.id]).kind !== 'set') {
      showTopNotice({ tone: 'error', message: t('modelProviderPresetMissingKeyForProbe') })
      return false
    }
    const mediaModels = providerMediaModelIds(profile)
    try {
      const result = await persistProviderSettingsDraft({
        profile,
        selectedModel,
        selectedMediaModel: mediaModels.includes(routingDraft.selectedMediaModel)
          ? routingDraft.selectedMediaModel
          : '',
        selectedRoutes: providerRegistrySelectedRoutes(profile.id, routingDraft.routeProviderIds),
        proxy: providerProxy.enabled ? providerProxy.url.trim() : '',
        credentialDraft: credentialDrafts[profile.id],
        requestProviderRegistry: async (request) => {
          if (!credentialOperationIsCurrent(operation)) throw new Error('Provider edit is no longer current.')
          const result = await window.analytix.providerRegistry.request(request)
          if (!credentialOperationIsCurrent(operation)) throw new Error('Provider edit is no longer current.')
          return result
        }
      })
      if (!credentialOperationIsCurrent(operation)) return false
      applyRegistrySnapshot(result.snapshot)
      if ((credentialInputRevisions.current[profile.id] ?? 0) === operation.input) {
        clearCredentialDraft(profile.id)
      }
      return result.credentialConfigured
    } catch {
      if (credentialOperationIsCurrent(operation)) {
        showTopNotice({ tone: 'error', message: 'Provider credential update failed.' })
      }
      return false
    }
  }

  const commitProviderDraft = async (): Promise<void> => {
    if (!draftProvider) return
    const operation = beginCredentialOperation(draftProvider.id)
    const configured = await persistCredentialDraft(draftProvider, operation)
    if (!credentialOperationIsCurrent(operation)) return
    if (!configured) {
      showTopNotice({ tone: 'error', message: t('modelProviderPresetMissingKeyForProbe') })
      return
    }
    let selected: ProviderRegistrySnapshotResult
    try {
      selected = await selectProviderRegistryEntry({
        providerId: draftProvider.id,
        requestProviderRegistry: async (request) => {
          if (!credentialOperationIsCurrent(operation)) throw new Error('Provider edit is no longer current.')
          const result = await window.analytix.providerRegistry.request(request)
          if (!credentialOperationIsCurrent(operation)) throw new Error('Provider edit is no longer current.')
          return result
        }
      })
    } catch {
      if (credentialOperationIsCurrent(operation)) {
        showTopNotice({ tone: 'error', message: 'Provider selection failed.' })
      }
      return
    }
    if (!credentialOperationIsCurrent(operation)) return
    applyRegistrySnapshot(selected)
    update(modelProvidersSettingsPatch({
      provider,
      providers: selected.providers.map(providerProfileFromRegistry),
      analytix: { providerId: draftProvider.id, model: draftProvider.models[0] ?? analytix.model },
      currentAnalytix: analytix
    }))
    setDraftProvider(null)
    setSelectedProviderId(draftProvider.id)
  }

  const cancelProviderDraft = (): void => {
    if (!draftProvider) return
    invalidateCredentialEntry()
    clearCredentialDraft(draftProvider.id)
    setDraftProvider(null)
    setSelectedProviderId(activeAnalytixProviderId)
  }

  const addModelProvider = (): void => {
    const baseId = 'custom-provider'
    let index = modelProviders.length + 1
    let id = `${baseId}-${index}`
    const used = new Set(displayProviders.map((item) => item.id))
    while (used.has(id)) {
      index += 1
      id = `${baseId}-${index}`
    }
    startProviderDraft({
      id,
      name: t('modelProviderNewName', { index }),
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'chat_completions',
      models: [],
      modelProfiles: {}
    })
  }

  const addPresetModelProvider = async (
    preset: ModelProviderPreset,
    mode: 'api' | 'token-plan' = 'api'
  ): Promise<void> => {
    const presetProvider = mode === 'token-plan'
      ? modelProviderTokenPlanProfile(preset)
      : modelProviderPresetProfile(preset)
    if (!presetProvider) return
    const existingProvider = modelProviders.find((item) => item.id === presetProvider.id)
    if (existingProvider) {
      const confirmed = await confirmAction({
        message: t('modelProviderUpdatePresetTitle', { name: presetProvider.name }),
        detail: t('modelProviderUpdatePresetDetail'),
        confirmLabel: t('modelProviderUpdatePresetAction'),
        cancelLabel: t('modelProviderCancel')
      })
      if (!confirmed) {
        selectProviderForEditing(presetProvider.id)
        return
      }
    }
    if (!existingProvider) {
      startProviderDraft(presetProvider)
      return
    }
    const nextProvider: ModelProviderProfileV1 = {
      ...presetProvider,
      name: existingProvider.name.trim() || presetProvider.name,
      models: mergeProviderModelIds(presetProvider.models, existingProvider.models),
      modelProfiles: {
        ...existingProvider.modelProfiles,
        ...presetProvider.modelProfiles
      },
      image: presetProvider.image ?? existingProvider.image,
      speech: presetProvider.speech ?? existingProvider.speech,
      textToSpeech: presetProvider.textToSpeech ?? existingProvider.textToSpeech,
      music: presetProvider.music ?? existingProvider.music,
      video: presetProvider.video ?? existingProvider.video
    }
    const nextProviders = modelProviders.map((item) => item.id === presetProvider.id ? nextProvider : item)
    selectProviderForEditing(nextProvider.id)
    updateModelProviders(nextProviders)
  }

  const selectModelProvider = async (id: string): Promise<void> => {
    try {
      const selected = await selectProviderRegistryEntry({
        providerId: id,
        requestProviderRegistry: (request) => window.analytix.providerRegistry.request(request)
      })
      applyRegistrySnapshot(selected)
      const profile = selected.providers.find((provider) => provider.id === id)
      update(modelProvidersSettingsPatch({
        provider,
        providers: selected.providers.map(providerProfileFromRegistry),
        analytix: {
          providerId: id,
          ...(profile?.selectedModel ? { model: profile.selectedModel } : {})
        },
        currentAnalytix: analytix
      }))
    } catch {
      showTopNotice({ tone: 'error', message: 'Provider selection failed.' })
    }
  }

  const disconnectModelProvider = async (id: string): Promise<void> => {
    try {
      const wasSelected = registrySnapshot?.selectedProviderId === id
      const disconnected = await disconnectProviderRegistryEntry({
        providerId: id,
        requestProviderRegistry: (request) => window.analytix.providerRegistry.request(request)
      })
      applyRegistrySnapshot(disconnected)
      if (wasSelected) {
        update(modelProvidersSettingsPatch({
          provider,
          providers: disconnected.providers.map(providerProfileFromRegistry),
          analytix: { providerId: disconnected.selectedProviderId ?? '' },
          currentAnalytix: analytix
        }))
      }
    } catch {
      showTopNotice({ tone: 'error', message: 'Provider disconnect failed.' })
    }
  }

  const removeModelProvider = async (id: string): Promise<void> => {
    const target = modelProviders.find((item) => item.id === id)
    if (!target) return
    const usedByChat = activeAnalytixProviderId === id
    const usedByImage = (analytix.imageGeneration?.providerId ?? '').trim() === id
    const usedBySpeech = (analytix.speechToText?.providerId ?? '').trim() === id
    const usedByTextToSpeech = (analytix.textToSpeech?.providerId ?? '').trim() === id
    const usedByMusic = (analytix.musicGeneration?.providerId ?? '').trim() === id
    const usedByVideo = (analytix.videoGeneration?.providerId ?? '').trim() === id
    const writeInline = form?.write?.inlineCompletion
    const usedByWrite = Boolean(
      writeInline && !writeInline.inheritProvider && writeInline.providerId === id
    )
    const references = [
      ...(usedByChat ? [t('modelProviderDeleteInUseChat')] : []),
      ...(usedByImage ? [t('modelProviderDeleteInUseImage')] : []),
      ...(usedBySpeech ? [t('modelProviderDeleteInUseSpeech')] : []),
      ...(usedByTextToSpeech ? [t('modelProviderDeleteInUseTextToSpeech')] : []),
      ...(usedByMusic ? [t('modelProviderDeleteInUseMusic')] : []),
      ...(usedByVideo ? [t('modelProviderDeleteInUseVideo')] : []),
      ...(usedByWrite ? [t('modelProviderDeleteInUseWrite')] : [])
    ]
    const confirmed = await confirmAction({
      message: t('modelProviderDeleteConfirmTitle', { name: target.name.trim() || target.id }),
      detail: [t('modelProviderDeleteConfirmDetail'), ...references].join('\n'),
      confirmLabel: t('modelProviderDeleteAction'),
      cancelLabel: t('modelProviderCancel')
    })
    if (!confirmed) return
    try {
      await deleteProviderRegistryEntry({
        providerId: id,
        requestProviderRegistry: (request) => window.analytix.providerRegistry.request(request)
      })
    } catch {
      showTopNotice({ tone: 'error', message: 'Provider deletion failed.' })
      return
    }
    const next = await refreshRegistry()
    const nextProviders = next.providers.map(providerProfileFromRegistry)
    const analytixPatch: AnalytixRuntimeSettingsPatchV1 | undefined =
      usedByChat || usedByImage || usedBySpeech || usedByTextToSpeech || usedByMusic || usedByVideo
        ? {
            ...(usedByChat ? { providerId: next.selectedProviderId ?? '' } : {}),
            ...(usedByImage ? { imageGeneration: { providerId: '' } } : {}),
            ...(usedBySpeech ? { speechToText: { providerId: '' } } : {}),
            ...(usedByTextToSpeech ? { textToSpeech: { providerId: '' } } : {}),
            ...(usedByMusic ? { musicGeneration: { providerId: '' } } : {}),
            ...(usedByVideo ? { videoGeneration: { providerId: '' } } : {})
          }
        : undefined
    const patch = modelProvidersSettingsPatch({
      provider,
      providers: nextProviders,
      analytix: analytixPatch,
      currentAnalytix: analytix
    })
    if (usedByWrite) {
      patch.write = { inlineCompletion: { inheritProvider: true, providerId: '' } }
    }
    selectProviderForEditing(next.selectedProviderId ?? nextProviders[0]?.id ?? '')
    update(patch)
  }

  const runProbe = async (target: ModelProviderProfileV1, mode: 'test' | 'fetch'): Promise<void> => {
    const fingerprint = providerConnectionFingerprint(target)
    const registryProvider = registrySnapshot?.providers.find((provider) => provider.id === target.id)
    if (!registrySnapshot || !registryProvider || registryProvider.tombstone ||
      !registryProvider.credentialConfigured || !registryProvider.credentialPurpose) {
      const message = t('modelProviderPresetMissingKeyForProbe')
      setProbeStates((prev) => ({
        ...prev,
        [target.id]: {
          fingerprint,
          mode,
          status: 'error',
          message
        }
      }))
      showTopNotice({ tone: 'error', message })
      return
    }
    setProbeStates((prev) => ({ ...prev, [target.id]: { fingerprint, mode, status: 'busy' } }))
    let result: ProviderRegistryResult
    try {
      result = await window.analytix.providerRegistry.request({
        schemaVersion: 1,
        operation: mode === 'fetch' ? 'discover-models' : 'probe',
        providerId: target.id,
        expected: {
          registryRevision: registrySnapshot.registryRevision,
          registryIncarnation: registrySnapshot.registryIncarnation,
          providerRevision: registryProvider.revision,
          providerGeneration: registryProvider.generation,
          providerIncarnation: registryProvider.incarnation,
          providerCredentialPurpose: registryProvider.credentialPurpose
        }
      })
    } catch {
      const message = 'Provider Registry request failed.'
      setProbeStates((prev) => ({
        ...prev,
        [target.id]: { fingerprint, mode, status: 'error', message }
      }))
      showTopNotice({ tone: 'error', message: t('modelProviderTestFailed', { message }) })
      return
    }
    if ('error' in result) {
      const message = result.error.message
      setProbeStates((prev) => ({
        ...prev,
        [target.id]: { fingerprint, mode, status: 'error', message }
      }))
      showTopNotice({ tone: 'error', message })
      return
    }
    if (mode === 'fetch') {
      if (!('provider' in result)) {
        const message = 'invalid_response'
        setProbeStates((prev) => ({
          ...prev,
          [target.id]: { fingerprint, mode, status: 'error', message }
        }))
        showTopNotice({ tone: 'error', message: t('modelProviderTestFailed', { message }) })
        return
      }
      let refreshed: ProviderRegistrySnapshotResult
      try {
        refreshed = await refreshRegistry()
      } catch {
        const message = 'Provider Registry request failed.'
        setProbeStates((prev) => ({
          ...prev,
          [target.id]: { fingerprint, mode, status: 'error', message }
        }))
        showTopNotice({ tone: 'error', message: t('modelProviderTestFailed', { message }) })
        return
      }
      const committedProvider = refreshed.providers.find((provider) => provider.id === target.id)
      if (!committedProvider || committedProvider.tombstone) {
        const message = 'invalid_response'
        setProbeStates((prev) => ({
          ...prev,
          [target.id]: { fingerprint, mode, status: 'error', message }
        }))
        showTopNotice({ tone: 'error', message: t('modelProviderTestFailed', { message }) })
        return
      }
      const total = committedProvider.models.length
      setProbeStates((prev) => ({
        ...prev,
        [target.id]: {
          fingerprint,
          mode,
          status: 'ok',
          total
        }
      }))
      showTopNotice({ tone: 'success', message: t('modelProviderFetchedModels', { total }) })
      return
    }
    if (!('status' in result) || result.status !== 'reachable') {
      const message = 'status' in result ? result.status : 'invalid_response'
      setProbeStates((prev) => ({
        ...prev,
        [target.id]: { fingerprint, mode, status: 'error', message }
      }))
      showTopNotice({ tone: 'error', message: t('modelProviderTestFailed', { message }) })
      return
    }
    setProbeStates((prev) => ({
      ...prev,
      [target.id]: {
        fingerprint,
        mode,
        status: 'ok',
        latencyMs: result.latencyMs,
        total: result.modelCount
      }
    }))
    showTopNotice({
      tone: 'success',
      message: t('modelProviderTestSuccess', { latency: result.latencyMs, total: result.modelCount })
    })
  }

  const providerKindLabel = (item: ModelProviderProfileV1): string => {
    if (item.id === DEFAULT_MODEL_PROVIDER_ID) return t('modelProviderDefaultBadge')
    if (tokenPlanPresetForProfileId(item.id)) return t('modelProviderTokenPlanBadge')
    const preset = getModelProviderPreset(item.id)
    if (preset?.category === 'subscription') return t('modelProviderPlanBadge')
    if (preset) return t('modelProviderPresetBadge')
    return t('modelProviderCustomBadge')
  }

  const runAccountObservation = async (target: ModelProviderProfileV1): Promise<void> => {
    const expectedSnapshot = registrySnapshot
    const provider = expectedSnapshot?.providers.find((candidate) => candidate.id === target.id)
    if (!expectedSnapshot || !provider || provider.tombstone || !provider.credentialConfigured || !provider.credentialPurpose) {
      return
    }
    const requestId = ++accountObservationRequestSequence.current
    latestAccountObservationRequest.current[provider.id] = requestId
    const busy: AccountObservationState = {
      requestId,
      selectedProviderId: expectedSnapshot.selectedProviderId ?? '',
      registryRevision: expectedSnapshot.registryRevision,
      registryIncarnation: expectedSnapshot.registryIncarnation,
      providerId: provider.id,
      providerRevision: provider.revision,
      providerGeneration: provider.generation,
      providerIncarnation: provider.incarnation,
      providerCredentialPurpose: provider.credentialPurpose,
      accountObservation: provider.accountObservation ?? null,
      status: 'busy'
    }
    setAccountObservationStates((current) => ({
      ...current,
      [provider.id]: busy
    }))
    try {
      const result = await window.analytix.providerRegistry.request({
        schemaVersion: 1,
        operation: 'observe-account',
        providerId: provider.id,
        expected: {
          registryRevision: expectedSnapshot.registryRevision,
          registryIncarnation: expectedSnapshot.registryIncarnation,
          providerRevision: provider.revision,
          providerGeneration: provider.generation,
          providerIncarnation: provider.incarnation,
          providerCredentialPurpose: provider.credentialPurpose
        }
      })
      const currentResult = await window.analytix.providerRegistry.request({ schemaVersion: 1, operation: 'list' })
      if ('error' in currentResult || !('providers' in currentResult)) throw new Error('Registry readback failed.')
      applyRegistrySnapshot(currentResult)
      const next = 'error' in result || !('observedAt' in result) || !('expiresAt' in result)
        ? null
        : providerAccountObservationStateFromResult({
            requestId,
            expectedSnapshot,
            expectedProvider: provider,
            observation: result,
            currentSnapshot: currentResult
          })
      setAccountObservationStates((current) => {
        if (latestAccountObservationRequest.current[provider.id] !== requestId ||
          current[provider.id]?.requestId !== requestId) return current
        if (next) return { ...current, [provider.id]: next }
        const { [provider.id]: _removed, ...remaining } = current
        return remaining
      })
    } catch {
      setAccountObservationStates((current) => {
        if (latestAccountObservationRequest.current[provider.id] !== requestId ||
          current[provider.id]?.requestId !== requestId) return current
        const { [provider.id]: _removed, ...remaining } = current
        return remaining
      })
    }
  }

  const configureAccountObservation = async (remove: boolean): Promise<void> => {
    if (!activeRegistryProvider || !canConfigureAccountObservation) return
    const endpoint = (accountObservationEndpointDrafts[activeRegistryProvider.id] ?? '').trim()
    if (!remove && endpoint === '') {
      showTopNotice({ tone: 'error', message: t('modelProviderQuotaBindingInvalid') })
      return
    }
    try {
      const snapshot = await persistProviderAccountObservationBinding({
        providerId: activeRegistryProvider.id,
        accountObservation: remove
          ? null
          : { schemaVersion: 1, endpoint, method: 'GET', projection: 'normalized-quota-v1' },
        requestProviderRegistry: (request) => window.analytix.providerRegistry.request(request)
      })
      latestAccountObservationRequest.current[activeRegistryProvider.id] = ++accountObservationRequestSequence.current
      setAccountObservationStates((current) => {
        const { [activeRegistryProvider.id]: _removed, ...remaining } = current
        return remaining
      })
      applyRegistrySnapshot(snapshot)
      showTopNotice({
        tone: 'success',
        message: t(remove ? 'modelProviderQuotaBindingRemoved' : 'modelProviderQuotaBindingSaved')
      })
    } catch {
      showTopNotice({ tone: 'error', message: t('modelProviderQuotaBindingInvalid') })
    }
  }

  const activeProbe = activeProvider ? probeStates[activeProvider.id] : undefined
  const activeAccountObservation = activeRegistryProvider
    ? accountObservationStates[activeRegistryProvider.id]
    : undefined
  const activeAccountObservationCurrent = activeAccountObservation && registrySnapshot &&
    providerAccountObservationStateIsCurrent(activeAccountObservation, registrySnapshot)
    ? activeAccountObservation
    : undefined
  const activeProbeFresh = Boolean(
    activeProvider &&
    activeProbe &&
    activeProbe.fingerprint === providerConnectionFingerprint(activeProvider)
  )
  const probeBusy = Boolean(activeProbeFresh && activeProbe?.status === 'busy')
  const probeNotice: InlineNotice | null = (() => {
    if (!activeProbeFresh || !activeProbe) return null
    if (activeProbe.status === 'busy') {
      return { tone: 'info', message: t('modelProviderTesting') }
    }
    if (activeProbe.status === 'error') {
      return { tone: 'error', message: t('modelProviderTestFailed', { message: activeProbe.message ?? '' }) }
    }
    return {
      tone: 'success',
      message: activeProbe.mode === 'fetch'
        ? t('modelProviderFetchedModels', { total: activeProbe.total ?? 0 })
        : t('modelProviderTestSuccess', { latency: activeProbe.latencyMs ?? 0, total: activeProbe.total ?? 0 })
    }
  })()
  const accountObservationNotice: InlineNotice | null = (() => {
    if (!activeAccountObservationCurrent) return null
    if (activeAccountObservationCurrent.status === 'busy') {
      return { tone: 'info', message: t('modelProviderQuotaRefreshing') }
    }
    if (activeAccountObservationCurrent.status !== 'available') {
      return { tone: 'info', message: t('modelProviderQuotaUnavailable') }
    }
    return {
      tone: 'success',
      message: t('modelProviderQuotaAvailable', {
        remaining: activeAccountObservationCurrent.remaining ?? '—',
        quota: activeAccountObservationCurrent.quota ?? '—'
      })
    }
  })()
  const activeBaseUrlInvalid = Boolean(activeProvider && !isAcceptableHttpUrl(activeProvider.baseUrl))
  const activeImageBaseUrlInvalid = Boolean(
    activeProvider?.image && !isAcceptableHttpUrl(activeProvider.image.baseUrl)
  )
  const activeSpeechBaseUrlInvalid = Boolean(
    activeProvider?.speech && !isAcceptableHttpUrl(activeProvider.speech.baseUrl)
  )
  const activeTextToSpeechBaseUrlInvalid = Boolean(
    activeProvider?.textToSpeech && !isAcceptableHttpUrl(activeProvider.textToSpeech.baseUrl)
  )
  const activeMusicBaseUrlInvalid = Boolean(
    activeProvider?.music && !isAcceptableHttpUrl(activeProvider.music.baseUrl)
  )
  const activeVideoBaseUrlInvalid = Boolean(
    activeProvider?.video && !isAcceptableHttpUrl(activeProvider.video.baseUrl)
  )
  const activeTokenPlanRegions = activeProvider
    ? tokenPlanPresetForProfileId(activeProvider.id)?.tokenPlan?.regions ?? []
    : []

  const planProviders = displayProviders.filter((item) => isSubscriptionProviderId(item.id))
  const apiProviders = displayProviders.filter((item) => !isSubscriptionProviderId(item.id))
  // 只要存在任一套餐类供应商就分组展示;否则(通常只有默认 DeepSeek)保持单一平铺列表。
  const grouped = planProviders.length > 0

  const renderProviderButton = (item: ModelProviderProfileV1): ReactElement => {
    const selected = activeProvider?.id === item.id
    const isDraft = draftProvider?.id === item.id
    const inUse = !isDraft && activeAnalytixProviderId === item.id
    const missingKey = !credentialConfiguredProviderIds.has(item.id)
    return (
      <button
        key={item.id}
        type="button"
        aria-pressed={selected}
        onClick={() => selectProviderForEditing(item.id)}
        className={`w-full rounded-xl border px-3 py-2.5 text-left transition ${
          selected
            ? 'border-accent/60 bg-ds-main/45 ring-1 ring-accent/30'
            : 'border-ds-border bg-ds-card hover:bg-ds-hover'
        }`}
      >
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="min-w-0 truncate text-[13.5px] font-semibold text-ds-ink">
            {providerDisplayName(item.id, item.name)}
          </span>
          {isDraft ? <ProviderBadge tone="warning">{t('modelProviderDraftBadge')}</ProviderBadge> : null}
          {inUse ? <ProviderBadge tone="accent">{t('modelProviderInUse')}</ProviderBadge> : null}
          {!isDraft && missingKey ? <ProviderBadge tone="warning">{t('modelProviderMissingKey')}</ProviderBadge> : null}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[12px] text-ds-faint">
          <span>{t('modelProviderModelCount', { total: providerModelCount(item) })}</span>
          <span aria-hidden="true">·</span>
          <span>{providerKindLabel(item)}</span>
          {credentialConfiguredProviderIds.has(item.id) ? <KeyRound className="h-3 w-3" strokeWidth={1.9} /> : null}
          {item.image ? <ImageIcon className="h-3 w-3" strokeWidth={1.9} /> : null}
          {item.models.some((model) =>
            modelSupportsImageInput(profileForModel(item, model))
          ) ? <span className="text-[11px] font-semibold text-ds-muted">{t('modelProviderVisionBadge')}</span> : null}
          {item.speech ? <Mic className="h-3 w-3" strokeWidth={1.9} /> : null}
          {item.textToSpeech ? <AudioLines className="h-3 w-3" strokeWidth={1.9} /> : null}
          {item.music ? <Music2 className="h-3 w-3" strokeWidth={1.9} /> : null}
          {item.video ? <Clapperboard className="h-3 w-3" strokeWidth={1.9} /> : null}
        </div>
      </button>
    )
  }

  const addMenuEntries = MODEL_PROVIDER_PRESETS.flatMap((preset) => {
    const entries: {
      preset: ModelProviderPreset
      mode: 'api' | 'token-plan'
      profileId: string
      label: string
      group: 'subscription' | 'api'
    }[] = [
      {
        preset,
        mode: 'api',
        profileId: preset.id,
        label: preset.name,
        group: preset.category === 'subscription' ? 'subscription' : 'api'
      }
    ]
    if (preset.tokenPlan) {
      entries.push({
        preset,
        mode: 'token-plan',
        profileId: tokenPlanProviderId(preset.id),
        label: `${preset.name} · Token Plan`,
        group: 'subscription'
      })
    }
    return entries
  })
  const planAddEntries = addMenuEntries.filter((entry) => entry.group === 'subscription')
  const apiAddEntries = addMenuEntries.filter((entry) => entry.group === 'api')
  const renderAddEntry = (entry: (typeof addMenuEntries)[number]): ReactElement => {
    const exists = modelProviders.some((item) => item.id === entry.profileId)
    return (
      <button
        key={entry.profileId}
        type="button"
        role="menuitem"
        onClick={() => {
          setAddMenuOpen(false)
          void addPresetModelProvider(entry.preset, entry.mode)
        }}
        className="flex w-full items-center justify-between gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] text-ds-ink transition hover:bg-ds-hover"
      >
        <span>{entry.label}</span>
        <span className="text-[11px] text-ds-faint">
          {exists
            ? t('modelProviderPresetUpdateTag')
            : entry.group === 'subscription'
              ? t('modelProviderPlanBadge')
              : t('modelProviderPresetBadge')}
        </span>
      </button>
    )
  }

  return (
    <SettingsCard title={t('providers')}>
      {registryLoadState !== 'ready' && (
        <div role={registryLoadState === 'unavailable' ? 'alert' : 'status'} className="mb-4 rounded-xl border border-ds-border bg-ds-card p-3 text-sm text-ds-muted">
          <p>{t(registryLoadState === 'loading' ? 'modelProviderRegistryLoading' : 'modelProviderRegistryUnavailable')}</p>
          {registryLoadState === 'unavailable' && (
            <button type="button" className="mt-2 rounded-lg border border-ds-border px-3 py-1.5 text-ds-ink" onClick={() => { void loadRegistry() }}>
              {t('modelProviderRegistryRetry')}
            </button>
          )}
        </div>
      )}
      <SettingRow
        title={t('proxyUrl')}
        description={t('proxyUrlDesc')}
        control={
          <div className="flex w-full min-w-0 flex-col gap-2 md:max-w-md">
            <label className="flex items-center justify-between gap-3 rounded-xl border border-ds-border bg-ds-card px-3 py-2 text-[13px] text-ds-muted shadow-sm">
              <span>{t('proxyEnabled')}</span>
              <Toggle
                checked={providerProxy.enabled === true}
                onChange={(enabled) => updateProviderProxy({ enabled })}
              />
            </label>
            <input
              className={textInputClass}
              placeholder={t('proxyUrlPlaceholder')}
              value={providerProxy.url}
              spellCheck={false}
              onChange={(e) => updateProviderProxy({ url: e.target.value })}
            />
          </div>
        }
      />
      <SettingRow
        title={t('providers')}
        description={t('providersDesc')}
        wideControl
        control={
          <div className="grid gap-4 lg:grid-cols-[220px_minmax(0,1fr)]">
            <div className="flex flex-col gap-3">
              {grouped ? (
                <>
                  <ProviderListGroup label={t('modelProviderGroupPlans')} count={planProviders.length}>
                    {planProviders.map(renderProviderButton)}
                  </ProviderListGroup>
                  <ProviderListGroup label={t('modelProviderGroupApi')} count={apiProviders.length}>
                    {apiProviders.map(renderProviderButton)}
                  </ProviderListGroup>
                </>
              ) : (
                <div className="grid gap-2">{displayProviders.map(renderProviderButton)}</div>
              )}
              <div ref={addMenuRef} className="relative">
                <button
                  type="button"
                  aria-haspopup="menu"
                  aria-expanded={addMenuOpen}
                  disabled={registryLoadState !== 'ready'}
                  onClick={() => setAddMenuOpen((value) => !value)}
                  className="inline-flex h-9 w-full items-center justify-center gap-2 rounded-full border border-ds-border bg-ds-card px-3 text-[12.5px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink"
                >
                  <Plus className="h-3.5 w-3.5" strokeWidth={1.9} />
                  {t('modelProviderAdd')}
                  <ChevronDown className="h-3.5 w-3.5" strokeWidth={1.9} />
                </button>
                {addMenuOpen ? (
                  <div
                    role="menu"
                    className="absolute left-0 right-0 z-20 mt-1 max-h-[min(60vh,420px)] overflow-y-auto rounded-xl border border-ds-border bg-ds-card p-1 shadow-lg"
                  >
                    <div className="px-2.5 pb-1 pt-1 text-[11px] font-semibold text-ds-faint">
                      {t('modelProviderGroupPlans')}
                    </div>
                    {planAddEntries.map(renderAddEntry)}
                    <div className="my-1 border-t border-ds-border-muted" />
                    <div className="px-2.5 pb-1 text-[11px] font-semibold text-ds-faint">
                      {t('modelProviderGroupApi')}
                    </div>
                    {apiAddEntries.map(renderAddEntry)}
                    <div className="my-1 border-t border-ds-border-muted" />
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setAddMenuOpen(false)
                        addModelProvider()
                      }}
                      className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-[13px] text-ds-ink transition hover:bg-ds-hover"
                    >
                      {t('modelProviderAddMenuCustom')}
                    </button>
                  </div>
                ) : null}
              </div>
            </div>
            {activeProvider ? (
              <div className="grid content-start gap-3 rounded-xl border border-ds-border-muted bg-ds-main/35 p-4">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="min-w-0 truncate text-[14px] font-semibold text-ds-ink">
                      {providerDisplayName(activeProvider.id, activeProvider.name)}
                    </span>
                    <span className="text-[11px] font-normal text-ds-faint">
                      {t(`providerConnection_${providerEndpointKind(activeProvider.baseUrl)}`)}
                    </span>
                    {!canEditActiveProviderId ? (
                      <span title={t('modelProviderIdLocked')} className="text-ds-faint">
                        <Lock className="h-3.5 w-3.5" strokeWidth={1.9} />
                      </span>
                    ) : null}
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {activeRegistryProvider && !activeRegistryProvider.tombstone && !isDraftActive ? (
                      <button
                        type="button"
                        onClick={() => void persistCredentialDraft(activeProvider)}
                        className="inline-flex h-8 items-center rounded-full border border-ds-border bg-ds-card px-3 text-[12px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink"
                      >
                        {t('modelProviderSaveChanges')}
                      </button>
                    ) : null}
                    {activeRegistryProvider && !activeRegistryProvider.tombstone &&
                    activeRegistryProvider.credentialConfigured &&
                    registrySnapshot?.selectedProviderId !== activeRegistryProvider.id ? (
                      <button
                        type="button"
                        onClick={() => void selectModelProvider(activeRegistryProvider.id)}
                        className="inline-flex h-8 items-center rounded-full bg-accent px-3 text-[12px] font-semibold text-white shadow-sm transition hover:opacity-90"
                      >
                        {t('modelProviderSelect')}
                      </button>
                    ) : null}
                    <button
                      type="button"
                      disabled={probeBusy || activeRegistryProvider?.tombstone === true}
                      onClick={() => void runProbe(activeProvider, 'test')}
                      className="inline-flex h-8 items-center gap-1.5 rounded-full border border-ds-border bg-ds-card px-3 text-[12px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {probeBusy && activeProbe?.mode === 'test'
                        ? <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.9} />
                        : <PlugZap className="h-3.5 w-3.5" strokeWidth={1.9} />}
                      {t('modelProviderTestConnection')}
                    </button>
                    {activeRegistryProvider?.credentialConfigured ? (
                      <button
                        type="button"
                        disabled={activeAccountObservationCurrent?.status === 'busy' || activeRegistryProvider.tombstone}
                        onClick={() => void runAccountObservation(activeProvider)}
                        className="inline-flex h-8 items-center rounded-full border border-ds-border bg-ds-card px-3 text-[12px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                      >
                        {t('modelProviderQuotaRefresh')}
                      </button>
                    ) : null}
                  </div>
                </div>
                {probeNotice ? <InlineNoticeView notice={probeNotice} /> : null}
                {accountObservationNotice ? <InlineNoticeView notice={accountObservationNotice} /> : null}
                <DetailSection title={t('modelProviderSectionBasics')}>
                  <div className="grid gap-3 md:grid-cols-2">
                    <label className={fieldLabelClass}>
                      {t('modelProviderName')}
                      <input
                        className={textInputClass}
                        value={isDraftActive ? activeProvider.name : providerDisplayName(activeProvider.id, activeProvider.name)}
                        readOnly={!isDraftActive}
                        onChange={(e) => updateModelProvider(activeProvider.id, { name: e.target.value })}
                      />
                    </label>
                    <label className={fieldLabelClass}>
                      {t('modelProviderId')}
                      <span className="relative block">
                        <input
                          className={`w-full min-w-0 rounded-xl border border-ds-border bg-ds-card px-3 py-2 font-mono text-[13px] font-normal shadow-sm ${
                            canEditActiveProviderId
                              ? 'text-ds-ink focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/30'
                              : 'pr-9 text-ds-faint'
                          }`}
                          value={activeProvider.id}
                          readOnly={!canEditActiveProviderId}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderId(activeProvider.id, e.target.value)}
                        />
                        {!canEditActiveProviderId ? (
                          <span
                            title={t('modelProviderIdLocked')}
                            className="absolute right-3 top-1/2 -translate-y-1/2 text-ds-faint"
                          >
                            <Lock className="h-3.5 w-3.5" strokeWidth={1.9} />
                          </span>
                        ) : null}
                      </span>
                    </label>
                  </div>
                </DetailSection>
                <DetailSection title={t('modelProviderSectionConnection')}>
                  <label className={fieldLabelClass}>
                    {t('modelProviderApiKey')}
                    <div className="flex items-center gap-2">
                      <SecretInput
                        value={credentialDrafts[activeProvider.id] ?? ''}
                        onChange={(value) => {
                          credentialInputRevisions.current[activeProvider.id] =
                            (credentialInputRevisions.current[activeProvider.id] ?? 0) + 1
                          setCredentialDrafts((current) => ({ ...current, [activeProvider.id]: value }))
                        }}
                        visible={showApiKey}
                        onToggleVisibility={() => setShowApiKey((value: boolean) => !value)}
                        placeholder={t(activeRegistryProvider?.credentialConfigured === true
                          ? 'modelProviderApiKeySavedPlaceholder'
                          : 'modelProviderApiKeyPlaceholder')}
                        autoComplete="off"
                        showLabel={t('showSecret')}
                        hideLabel={t('hideSecret')}
                      />
                    </div>
                    {activeRegistryProvider?.credentialConfigured === true ? (
                      <span className="text-[12px] font-normal text-ds-muted">
                        {t('modelProviderApiKeySavedHint')}
                      </span>
                    ) : null}
                  </label>
                  <label className={fieldLabelClass}>
                    {t('modelProviderBaseUrl')}
                    <input
                      className={textInputClass}
                      value={activeProvider.baseUrl}
                      placeholder={t('baseUrlPlaceholder')}
                      spellCheck={false}
                      onChange={(e) => updateModelProvider(activeProvider.id, { baseUrl: e.target.value })}
                    />
                    {activeBaseUrlInvalid ? (
                      <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                        {t('modelProviderInvalidUrl')}
                      </span>
                    ) : null}
                  </label>
                  {activeTokenPlanRegions.length > 0 ? (
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className="text-[12px] font-semibold text-ds-muted">
                        {t('modelProviderTokenPlanRegion')}
                      </span>
                      {activeTokenPlanRegions.map((region) => {
                        const active = activeProvider.baseUrl.trim() === region.baseUrl
                        return (
                          <button
                            key={region.id}
                            type="button"
                            onClick={() => {
                              const patch: Partial<ModelProviderProfileV1> = { baseUrl: region.baseUrl }
                              const speech = activeProvider.speech
                              if (speech && activeTokenPlanRegions.some((item) => item.baseUrl === speech.baseUrl.trim())) {
                                patch.speech = { ...speech, baseUrl: region.baseUrl }
                              }
                              const textToSpeech = activeProvider.textToSpeech
                              if (
                                textToSpeech &&
                                activeTokenPlanRegions.some((item) => item.baseUrl === textToSpeech.baseUrl.trim())
                              ) {
                                patch.textToSpeech = { ...textToSpeech, baseUrl: region.baseUrl }
                              }
                              updateModelProvider(activeProvider.id, patch)
                            }}
                            className={`inline-flex h-7 items-center rounded-full border px-2.5 text-[12px] font-medium transition ${
                              active
                                ? 'border-accent/60 bg-ds-main/45 text-ds-ink ring-1 ring-accent/30'
                                : 'border-ds-border bg-ds-card text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
                            }`}
                          >
                            {t(`firstRunRegion_${region.id}`)}
                          </button>
                        )
                      })}
                    </div>
                  ) : null}
                  <label className={fieldLabelClass}>
                    {t('modelProviderEndpointFormat')}
                    <select
                      className={selectControlClass}
                      value={activeProvider.endpointFormat}
                      onChange={(e) => updateModelProvider(activeProvider.id, {
                        endpointFormat: e.target.value as ModelEndpointFormat
                      })}
                    >
                      {MODEL_ENDPOINT_FORMATS.map((format) => (
                        <option key={format} value={format}>
                          {t(MODEL_ENDPOINT_FORMAT_LABEL_KEYS[format])}
                        </option>
                      ))}
                    </select>
                  </label>
                  {activeProvider.endpointFormat === 'custom_endpoint' ? (
                    <p className="text-[12px] leading-5 text-ds-muted">
                      {t('modelEndpointCustomEndpointDesc')}
                    </p>
                  ) : null}
                  {canConfigureAccountObservation && activeRegistryProvider ? (
                    <div className="grid gap-2 rounded-xl border border-ds-border-muted bg-ds-main/30 p-3">
                      <label className={fieldLabelClass}>
                        {t('modelProviderQuotaBindingEndpoint')}
                        <input
                          className={textInputClass}
                          value={accountObservationEndpointDrafts[activeRegistryProvider.id] ?? ''}
                          placeholder={t('modelProviderQuotaBindingEndpointPlaceholder')}
                          spellCheck={false}
                          onChange={(event) => setAccountObservationEndpointDrafts((current) => ({
                            ...current,
                            [activeRegistryProvider.id]: event.target.value
                          }))}
                        />
                      </label>
                      <p className="text-[12px] leading-5 text-ds-muted">
                        {t('modelProviderQuotaBindingDescription')}
                      </p>
                      <div className="flex flex-wrap gap-2">
                        <button
                          type="button"
                          onClick={() => void configureAccountObservation(false)}
                          className="inline-flex h-8 items-center rounded-full border border-ds-border bg-ds-card px-3 text-[12px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink"
                        >
                          {t('modelProviderQuotaBindingSave')}
                        </button>
                        {activeRegistryProvider.accountObservation ? (
                          <button
                            type="button"
                            onClick={() => void configureAccountObservation(true)}
                            className="inline-flex h-8 items-center rounded-full border border-ds-border bg-ds-card px-3 text-[12px] font-medium text-red-600 shadow-sm transition hover:bg-ds-hover"
                          >
                            {t('modelProviderQuotaBindingRemove')}
                          </button>
                        ) : null}
                      </div>
                    </div>
                  ) : null}
                </DetailSection>
                <DetailSection
                  title={`${t('modelProviderModels')} · ${providerModelCount(activeProvider)}`}
                  action={
                    <button
                      type="button"
                      disabled={probeBusy}
                      onClick={() => void runProbe(activeProvider, 'fetch')}
                      className="inline-flex h-7 items-center gap-1.5 rounded-full border border-ds-border bg-ds-card px-2.5 text-[12px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                    >
                      {probeBusy && activeProbe?.mode === 'fetch'
                        ? <Loader2 className="h-3 w-3 animate-spin" strokeWidth={1.9} />
                        : <Download className="h-3 w-3" strokeWidth={1.9} />}
                      {t('modelProviderFetchModels')}
                    </button>
                  }
                >
                  <div className="grid gap-3 md:grid-cols-2">
                    <label className={fieldLabelClass}>
                      {t('modelProviderSelectedModel')}
                      <select
                        className={selectControlClass}
                        value={effectiveSelectedModel}
                        onChange={(event) => {
                          if (!activeRoutingDraft) return
                          updateActiveRoutingDraft(providerRegistryRoutingDraftWithSelectedModel(
                            activeRoutingDraft,
                            activeProvider.id,
                            event.target.value
                          ))
                        }}
                      >
                        <option value="">{t('modelProviderSelectionDisabled')}</option>
                        {activeProvider.models.map((model) => (
                          <option key={model} value={model}>{providerModelDisplayName(model)}</option>
                        ))}
                      </select>
                    </label>
                    <label className={fieldLabelClass}>
                      {t('modelProviderSelectedMediaModel')}
                      <select
                        className={selectControlClass}
                        value={effectiveSelectedMediaModel}
                        onChange={(event) => updateActiveRoutingDraft({ selectedMediaModel: event.target.value })}
                      >
                        <option value="">{t('modelProviderSelectionDisabled')}</option>
                        {activeMediaModelIds.map((model) => (
                          <option key={model} value={model}>{providerModelDisplayName(model)}</option>
                        ))}
                      </select>
                    </label>
                  </div>
                  <ProviderModelsManager
                    key={activeProvider.id}
                    provider={activeProvider}
                    t={t}
                    selectControlClass={selectControlClass}
                    onChange={(next) => patchProviderProfile(activeProvider, () => next)}
                  />
                </DetailSection>
                <DetailSection title={t('modelProviderRoutePool')}>
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderRoutePoolDesc')}</p>
                  <div className="grid gap-2">
                    {availableRouteProviderIds.map((providerId) => {
                      const selectedIndex = activeRoutingDraft?.routeProviderIds.indexOf(providerId) ?? -1
                      const selected = selectedIndex >= 0
                      const profile = displayProviders.find((candidate) => candidate.id === providerId)
                      return (
                        <div
                          key={providerId}
                          className="flex min-w-0 items-center gap-2 rounded-xl border border-ds-border-muted bg-ds-card/60 px-3 py-2"
                        >
                          <input
                            type="checkbox"
                            checked={selected}
                            disabled={selected && activeRoutingDraft?.routeProviderIds.length === 1 &&
                              effectiveSelectedModel !== ''}
                            aria-label={t('modelProviderRouteToggle', { provider: providerId })}
                            onChange={() => toggleActiveRouteProvider(providerId)}
                          />
                          <span className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-ds-ink">
                            {profile ? providerDisplayName(profile.id, profile.name) : providerId}
                          </span>
                          <span className="font-mono text-[11.5px] text-ds-faint">{providerId}</span>
                          {selected ? (
                            <>
                              <span className="rounded-full bg-ds-main/70 px-2 py-0.5 text-[10.5px] font-semibold text-ds-muted">
                                {t('modelProviderRouteOrder', { order: selectedIndex + 1 })}
                              </span>
                              <button
                                type="button"
                                disabled={selectedIndex === 0}
                                aria-label={t('modelProviderRouteMoveUp', { provider: providerId })}
                                onClick={() => moveActiveRouteProvider(providerId, -1)}
                                className="rounded-lg border border-ds-border p-1 text-ds-muted disabled:opacity-40"
                              >
                                <ChevronUp className="h-3.5 w-3.5" />
                              </button>
                              <button
                                type="button"
                                disabled={selectedIndex === (activeRoutingDraft?.routeProviderIds.length ?? 0) - 1}
                                aria-label={t('modelProviderRouteMoveDown', { provider: providerId })}
                                onClick={() => moveActiveRouteProvider(providerId, 1)}
                                className="rounded-lg border border-ds-border p-1 text-ds-muted disabled:opacity-40"
                              >
                                <ChevronDown className="h-3.5 w-3.5" />
                              </button>
                            </>
                          ) : null}
                        </div>
                      )
                    })}
                  </div>
                </DetailSection>
                <DetailSection
                  title={t('modelProviderImageCapability')}
                  action={
                    <Toggle
                      checked={Boolean(activeProvider.image)}
                      onChange={(value) => {
                        if (value) {
                          updateModelProvider(activeProvider.id, {
                            image: presetImageCapability(activeProvider.id) ?? defaultImageCapability(activeProvider.baseUrl)
                          })
                        } else {
                          removeModelProviderImage(activeProvider.id)
                        }
                      }}
                    />
                  }
                >
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderImageCapabilityDesc')}</p>
                  {activeProvider.image ? (
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className={fieldLabelClass}>
                        {t('imageGenProtocol')}
                        <select
                          className={selectControlClass}
                          value={activeProvider.image.protocol}
                          onChange={(e) => updateModelProviderImage(activeProvider.id, {
                            protocol: e.target.value as ImageGenerationProtocol
                          })}
                        >
                          {Object.entries(IMAGE_GENERATION_PROTOCOL_LABEL_KEYS).map(([protocol, key]) => (
                            <option key={protocol} value={protocol}>{t(key)}</option>
                          ))}
                        </select>
                      </label>
                      <label className={fieldLabelClass}>
                        {t('imageGenBaseUrl')}
                        <input
                          className={textInputClass}
                          value={activeProvider.image.baseUrl}
                          placeholder={t('imageGenBaseUrlPlaceholder')}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderImage(activeProvider.id, { baseUrl: e.target.value })}
                        />
                        {activeImageBaseUrlInvalid ? (
                          <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                            {t('modelProviderInvalidUrl')}
                          </span>
                        ) : null}
                      </label>
                      <label className={`${fieldLabelClass} md:col-span-2`}>
                        {t('imageGenModel')}
                        <ModelChipsInput
                          key={`${activeProvider.id}-image`}
                          values={activeProvider.image.models}
                          onChange={(models) => updateModelProviderImage(activeProvider.id, { models })}
                          placeholder={t('modelProviderModelsPlaceholder')}
                          inputAriaLabel={t('imageGenModel')}
                          removeLabel={(model) => t('modelProviderModelRemove', { model })}
                        />
                      </label>
                    </div>
                  ) : null}
                </DetailSection>
                <DetailSection
                  title={t('modelProviderSpeechCapability')}
                  action={
                    <Toggle
                      checked={Boolean(activeProvider.speech)}
                      onChange={(value) => {
                        if (value) {
                          updateModelProvider(activeProvider.id, {
                            speech: presetSpeechCapability(activeProvider) ?? defaultSpeechCapability(activeProvider.baseUrl)
                          })
                        } else {
                          removeModelProviderSpeech(activeProvider.id)
                        }
                      }}
                    />
                  }
                >
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderSpeechCapabilityDesc')}</p>
                  {activeProvider.speech ? (
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className={fieldLabelClass}>
                        {t('speechToTextProtocol')}
                        <select
                          className={selectControlClass}
                          value={activeProvider.speech.protocol}
                          onChange={(e) => updateModelProviderSpeech(activeProvider.id, {
                            protocol: e.target.value as SpeechToTextProtocol
                          })}
                        >
                          {Object.entries(SPEECH_TO_TEXT_PROTOCOL_LABEL_KEYS).map(([protocol, key]) => (
                            <option key={protocol} value={protocol}>{t(key)}</option>
                          ))}
                        </select>
                      </label>
                      <label className={fieldLabelClass}>
                        {t('speechToTextBaseUrl')}
                        <input
                          className={textInputClass}
                          value={activeProvider.speech.baseUrl}
                          placeholder={t('baseUrlPlaceholder')}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderSpeech(activeProvider.id, { baseUrl: e.target.value })}
                        />
                        {activeSpeechBaseUrlInvalid ? (
                          <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                            {t('modelProviderInvalidUrl')}
                          </span>
                        ) : null}
                      </label>
                      <label className={`${fieldLabelClass} md:col-span-2`}>
                        {t('speechToTextModels')}
                        <ModelChipsInput
                          key={`${activeProvider.id}-speech`}
                          values={activeProvider.speech.models}
                          onChange={(models) => updateModelProviderSpeech(activeProvider.id, { models })}
                          placeholder={t('modelProviderModelsPlaceholder')}
                          inputAriaLabel={t('speechToTextModels')}
                          removeLabel={(model) => t('modelProviderModelRemove', { model })}
                        />
                      </label>
                    </div>
                  ) : null}
                </DetailSection>
                <DetailSection
                  title={t('modelProviderTextToSpeechCapability')}
                  action={
                    <Toggle
                      checked={Boolean(activeProvider.textToSpeech)}
                      onChange={(value) => {
                        if (value) {
                          updateModelProvider(activeProvider.id, {
                            textToSpeech: presetTextToSpeechCapability(activeProvider) ??
                              defaultTextToSpeechCapability(activeProvider.baseUrl)
                          })
                        } else {
                          removeModelProviderTextToSpeech(activeProvider.id)
                        }
                      }}
                    />
                  }
                >
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderTextToSpeechCapabilityDesc')}</p>
                  {activeProvider.textToSpeech ? (
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className={fieldLabelClass}>
                        {t('textToSpeechProtocol')}
                        <select
                          className={selectControlClass}
                          value={activeProvider.textToSpeech.protocol}
                          onChange={(e) => updateModelProviderTextToSpeech(activeProvider.id, {
                            protocol: e.target.value as TextToSpeechProtocol
                          })}
                        >
                          {Object.entries(TEXT_TO_SPEECH_PROTOCOL_LABEL_KEYS).map(([protocol, key]) => (
                            <option key={protocol} value={protocol}>{t(key)}</option>
                          ))}
                        </select>
                      </label>
                      <label className={fieldLabelClass}>
                        {t('textToSpeechBaseUrl')}
                        <input
                          className={textInputClass}
                          value={activeProvider.textToSpeech.baseUrl}
                          placeholder={t('textToSpeechBaseUrlPlaceholder')}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderTextToSpeech(activeProvider.id, { baseUrl: e.target.value })}
                        />
                        {activeTextToSpeechBaseUrlInvalid ? (
                          <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                            {t('modelProviderInvalidUrl')}
                          </span>
                        ) : null}
                      </label>
                      <label className={`${fieldLabelClass} md:col-span-2`}>
                        {t('textToSpeechModel')}
                        <ModelChipsInput
                          key={`${activeProvider.id}-tts`}
                          values={activeProvider.textToSpeech.models}
                          onChange={(models) => updateModelProviderTextToSpeech(activeProvider.id, { models })}
                          placeholder={t('modelProviderModelsPlaceholder')}
                          inputAriaLabel={t('textToSpeechModel')}
                          removeLabel={(model) => t('modelProviderModelRemove', { model })}
                        />
                      </label>
                    </div>
                  ) : null}
                </DetailSection>
                <DetailSection
                  title={t('modelProviderMusicCapability')}
                  action={
                    <Toggle
                      checked={Boolean(activeProvider.music)}
                      onChange={(value) => {
                        if (value) {
                          updateModelProvider(activeProvider.id, {
                            music: presetMusicCapability(activeProvider) ?? defaultMusicCapability(activeProvider.baseUrl)
                          })
                        } else {
                          removeModelProviderMusic(activeProvider.id)
                        }
                      }}
                    />
                  }
                >
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderMusicCapabilityDesc')}</p>
                  {activeProvider.music ? (
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className={fieldLabelClass}>
                        {t('musicGenerationProtocol')}
                        <select
                          className={selectControlClass}
                          value={activeProvider.music.protocol}
                          onChange={(e) => updateModelProviderMusic(activeProvider.id, {
                            protocol: e.target.value as MusicGenerationProtocol
                          })}
                        >
                          {Object.entries(MUSIC_GENERATION_PROTOCOL_LABEL_KEYS).map(([protocol, key]) => (
                            <option key={protocol} value={protocol}>{t(key)}</option>
                          ))}
                        </select>
                      </label>
                      <label className={fieldLabelClass}>
                        {t('musicGenerationBaseUrl')}
                        <input
                          className={textInputClass}
                          value={activeProvider.music.baseUrl}
                          placeholder={t('musicGenerationBaseUrlPlaceholder')}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderMusic(activeProvider.id, { baseUrl: e.target.value })}
                        />
                        {activeMusicBaseUrlInvalid ? (
                          <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                            {t('modelProviderInvalidUrl')}
                          </span>
                        ) : null}
                      </label>
                      <label className={`${fieldLabelClass} md:col-span-2`}>
                        {t('musicGenerationModel')}
                        <ModelChipsInput
                          key={`${activeProvider.id}-music`}
                          values={activeProvider.music.models}
                          onChange={(models) => updateModelProviderMusic(activeProvider.id, { models })}
                          placeholder={t('modelProviderModelsPlaceholder')}
                          inputAriaLabel={t('musicGenerationModel')}
                          removeLabel={(model) => t('modelProviderModelRemove', { model })}
                        />
                      </label>
                    </div>
                  ) : null}
                </DetailSection>
                <DetailSection
                  title={t('modelProviderVideoCapability')}
                  action={
                    <Toggle
                      checked={Boolean(activeProvider.video)}
                      onChange={(value) => {
                        if (value) {
                          updateModelProvider(activeProvider.id, {
                            video: presetVideoCapability(activeProvider) ?? defaultVideoCapability(activeProvider.baseUrl)
                          })
                        } else {
                          removeModelProviderVideo(activeProvider.id)
                        }
                      }}
                    />
                  }
                >
                  <p className="text-[12px] leading-5 text-ds-faint">{t('modelProviderVideoCapabilityDesc')}</p>
                  {activeProvider.video ? (
                    <div className="grid gap-3 md:grid-cols-2">
                      <label className={fieldLabelClass}>
                        {t('videoGenerationProtocol')}
                        <select
                          className={selectControlClass}
                          value={activeProvider.video.protocol}
                          onChange={(e) => updateModelProviderVideo(activeProvider.id, {
                            protocol: e.target.value as VideoGenerationProtocol
                          })}
                        >
                          {Object.entries(VIDEO_GENERATION_PROTOCOL_LABEL_KEYS).map(([protocol, key]) => (
                            <option key={protocol} value={protocol}>{t(key)}</option>
                          ))}
                        </select>
                      </label>
                      <label className={fieldLabelClass}>
                        {t('videoGenerationBaseUrl')}
                        <input
                          className={textInputClass}
                          value={activeProvider.video.baseUrl}
                          placeholder={t('videoGenerationBaseUrlPlaceholder')}
                          spellCheck={false}
                          onChange={(e) => updateModelProviderVideo(activeProvider.id, { baseUrl: e.target.value })}
                        />
                        {activeVideoBaseUrlInvalid ? (
                          <span className="text-[12px] font-normal text-amber-600 dark:text-amber-300">
                            {t('modelProviderInvalidUrl')}
                          </span>
                        ) : null}
                      </label>
                      <label className={`${fieldLabelClass} md:col-span-2`}>
                        {t('videoGenerationModel')}
                        <ModelChipsInput
                          key={`${activeProvider.id}-video`}
                          values={activeProvider.video.models}
                          onChange={(models) => updateModelProviderVideo(activeProvider.id, { models })}
                          placeholder={t('modelProviderModelsPlaceholder')}
                          inputAriaLabel={t('videoGenerationModel')}
                          removeLabel={(model) => t('modelProviderModelRemove', { model })}
                        />
                      </label>
                    </div>
                  ) : null}
                </DetailSection>
                {isDraftActive ? (
                  <DetailSection title={t('modelProviderDraftSection')}>
                    <div className="flex flex-wrap items-center gap-3">
                      <button
                        type="button"
                        onClick={commitProviderDraft}
                        className="inline-flex h-9 w-fit items-center gap-2 rounded-full bg-accent px-4 text-[12.5px] font-semibold text-white shadow-sm transition hover:opacity-90"
                      >
                        <Plus className="h-3.5 w-3.5" strokeWidth={2} />
                        {t('modelProviderDraftConfirm')}
                      </button>
                      <button
                        type="button"
                        onClick={cancelProviderDraft}
                        className="inline-flex h-9 w-fit items-center gap-2 rounded-full border border-ds-border bg-ds-card px-3 text-[12.5px] font-medium text-ds-muted shadow-sm transition hover:bg-ds-hover hover:text-ds-ink"
                      >
                        {t('modelProviderDraftDiscard')}
                      </button>
                      <span className="text-[12px] text-ds-faint">
                        {credentialDraftAction(credentialDrafts[activeProvider.id]).kind === 'set'
                          ? t('modelProviderDraftHintReady')
                          : t('modelProviderDraftHintNoKey')}
                      </span>
                    </div>
                  </DetailSection>
                ) : (
                  <DetailSection title={t('modelProviderSectionDanger')}>
                    <div className="flex flex-wrap items-center gap-3">
                      {activeRegistryProvider && !activeRegistryProvider.tombstone ? (
                        <button
                          type="button"
                          onClick={() => void disconnectModelProvider(activeProvider.id)}
                          className="inline-flex h-9 w-fit items-center gap-2 rounded-full border border-amber-300/70 bg-amber-50 px-3 text-[12.5px] font-medium text-amber-700 transition hover:bg-amber-100 dark:border-amber-900/70 dark:bg-amber-950/25 dark:text-amber-200"
                        >
                          {t('modelProviderDisconnect')}
                        </button>
                      ) : null}
                      <button
                        type="button"
                        onClick={() => void removeModelProvider(activeProvider.id)}
                        className="inline-flex h-9 w-fit items-center gap-2 rounded-full border border-red-200/70 bg-red-50 px-3 text-[12.5px] font-medium text-red-700 transition hover:bg-red-100 dark:border-red-900/70 dark:bg-red-950/25 dark:text-red-200 dark:hover:bg-red-950/40"
                      >
                        <Trash2 className="h-3.5 w-3.5" strokeWidth={1.9} />
                        {t('modelProviderRemove')}
                      </button>
                      <span className="text-[12px] text-ds-faint">{t('modelProviderDangerHint')}</span>
                    </div>
                  </DetailSection>
                )}
              </div>
            ) : null}
          </div>
        }
      />
      <details className="border-t border-ds-border-muted px-4 py-3">
        <summary className="cursor-pointer text-[13px] font-medium text-ds-muted focus-visible:outline-accent">
          {t('modelProviderRecoveryTools')}
        </summary>
        <SettingRow
          title="Portable manifest / protected credential recovery"
          description="Complete ordinary portable manifest import first. Recovery files and confirmations stay in the Main process; only bounded status is shown here."
          wideControl
          control={
            <div className="flex flex-wrap items-center gap-2 rounded-xl border border-ds-border bg-ds-card p-3 text-[13px] text-ds-muted">
              <Lock size={15} aria-hidden="true" />
              <button
                type="button"
                className="rounded-lg border border-ds-border px-2.5 py-1.5 hover:bg-ds-hover disabled:opacity-50"
                disabled={protectedRecoveryBusy !== null}
                onClick={() => void runProtectedRecoveryAction('createDestinationRequest')}
              >
                Prepare destination request
              </button>
              <button
                type="button"
                className="rounded-lg border border-ds-border px-2.5 py-1.5 hover:bg-ds-hover disabled:opacity-50"
                disabled={protectedRecoveryBusy !== null}
                onClick={() => void runProtectedRecoveryAction('createSourceBundle')}
              >
                Create source bundle
              </button>
              <button
                type="button"
                className="rounded-lg border border-ds-border px-2.5 py-1.5 hover:bg-ds-hover disabled:opacity-50"
                disabled={protectedRecoveryBusy !== null}
                onClick={() => void runProtectedRecoveryAction('applyDestinationBundle')}
              >
                Apply destination bundle
              </button>
              <button
                type="button"
                className="rounded-lg border border-ds-border px-2.5 py-1.5 hover:bg-ds-hover disabled:opacity-50"
                disabled={protectedRecoveryBusy !== null}
                onClick={() => void runProtectedRecoveryAction('finalizeSourceReceipt')}
              >
                Finalize source receipt
              </button>
              {protectedRecoveryStatus ? <span role="status">{protectedRecoveryStatus}</span> : null}
            </div>
          }
        />
      </details>
    </SettingsCard>
  )
}
