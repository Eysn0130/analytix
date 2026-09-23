import { describe, expect, it, vi } from 'vitest'
import {
  getAnalytixRuntimeSettings,
  getModelProviderSettings,
  modelProviderTokenPlanProfile,
  normalizeAppSettings,
  type AppSettingsV1,
  type ModelProviderProfileV1
} from '@shared/app-settings'
import type { ProviderRegistryRequest, ProviderRegistryResult } from '@shared/analytix-api'
import {
  buildInitialSetupSettings,
  credentialDraftAction,
  deleteProviderRegistryEntry,
  disconnectProviderRegistryEntry,
  INITIAL_SETUP_PROVIDER_PRESETS,
  initialSetupDrafts,
  initialSetupProfileId,
  initialSetupSelection,
  persistInitialSetup,
  persistProviderAccountObservationBinding,
  persistProviderCredentialDraft,
  persistProviderSettingsDraft,
  providerRegistryInput,
  selectProviderRegistryEntry
} from './initial-setup-save'

function settings(patch: Record<string, unknown> = {}): AppSettingsV1 {
  return normalizeAppSettings(patch as AppSettingsV1)
}

function settingsWithActiveXiaomi(): AppSettingsV1 {
  return settings({
    provider: {
      baseUrl: 'https://api.deepseek.com',
      providers: [
        { id: 'xiaomi', name: 'Xiaomi', baseUrl: 'https://api.xiaomimimo.com/v1', models: ['mimo-v2.5'] }
      ]
    },
    runtime: { providerId: 'xiaomi' }
  })
}

const REGISTRY_INCARNATION = `inc_${'a'.repeat(43)}`
const PROVIDER_INCARNATION = `inc_${'b'.repeat(43)}`
type RegistrySnapshot = Extract<ProviderRegistryResult, { providers: unknown[] }>
type PublicProvider = RegistrySnapshot['providers'][number]

function publicProvider(
  id = 'deepseek',
  credentialConfigured = true,
  patch: Partial<PublicProvider> = {}
): PublicProvider {
  return {
    id,
    kind: id,
    endpoint: 'https://api.deepseek.com',
    models: ['deepseek-v4-pro', 'deepseek-v4-flash'],
    mediaModels: [],
    selectedModel: 'deepseek-v4-pro',
    selectedRoutes: [],
    credentialConfigured,
    ...(credentialConfigured ? { credentialPurpose: 'provider-api-key' } : {}),
    revision: '1',
    generation: '1',
    incarnation: PROVIDER_INCARNATION,
    tombstone: false,
    ...patch
  }
}

function registryHarness(initialProviders: PublicProvider[] = [], selectedProviderId?: string) {
  let providers = [...initialProviders]
  let selected = selectedProviderId
  let revision = 1
  const snapshot = (): RegistrySnapshot => ({
    schemaVersion: 1,
    registryRevision: String(revision),
    registryIncarnation: REGISTRY_INCARNATION,
    ...(selected ? { selectedProviderId: selected } : {}),
    providers
  })
  const request = vi.fn(async (input: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
    if (input.operation === 'list') return snapshot()
    if (input.operation === 'probe') return {
      schemaVersion: 1, registryRevision: String(revision), registryIncarnation: REGISTRY_INCARNATION,
      providerId: input.providerId, providerRevision: input.expected.providerRevision,
      providerGeneration: input.expected.providerGeneration, providerIncarnation: input.expected.providerIncarnation,
      status: 'reachable', code: 200, modelCount: 2, latencyMs: 1
    }
    if (input.operation === 'connect' || input.operation === 'update') {
      const existing = providers.find((provider) => provider.id === input.provider.id)
      const configured = input.credential.kind === 'set' || existing?.credentialConfigured === true
      revision += 1
      if (input.operation === 'connect' && !selected && !input.deferSelection) selected = input.provider.id
      providers = [
        ...providers.filter((provider) => provider.id !== input.provider.id),
        {
          ...publicProvider(input.provider.id, configured),
          kind: input.provider.kind,
          endpoint: input.provider.endpoint,
          ...(input.provider.proxy ? { proxy: input.provider.proxy } : {}),
          models: input.provider.models,
          mediaModels: input.provider.mediaModels,
          selectedModel: input.provider.selectedModel || undefined,
          selectedMediaModel: input.provider.selectedMediaModel || undefined,
          selectedRoutes: input.provider.selectedRoutes,
          ...(input.provider.oauthBinding ? { oauthBinding: input.provider.oauthBinding } : {}),
          ...(input.provider.accountObservation ? { accountObservation: input.provider.accountObservation } : {}),
          revision: String(Number(existing?.revision ?? 0) + 1)
        }
      ].sort((left, right) => left.id.localeCompare(right.id))
      return {
        schemaVersion: 1,
        registryRevision: String(revision),
        registryIncarnation: REGISTRY_INCARNATION,
        provider: providers.find((provider) => provider.id === input.provider.id)!
      }
    }
    if (input.operation === 'credential-replace') {
      const existing = providers.find((provider) => provider.id === input.providerId)
      if (!existing) throw new Error('Provider not found')
      revision += 1
      const provider = {
        ...existing,
        credentialConfigured: true,
        credentialPurpose: input.credential.purpose,
        generation: String(Number(existing.generation) + 1)
      }
      providers = providers.map((item) => item.id === input.providerId ? provider : item)
      return {
        schemaVersion: 1,
        registryRevision: String(revision),
        registryIncarnation: REGISTRY_INCARNATION,
        provider
      }
    }
    if (input.operation === 'disconnect') {
      const existing = providers.find((provider) => provider.id === input.providerId)
      if (!existing) throw new Error('Provider not found')
      revision += 1
      const provider = {
        ...existing,
        credentialConfigured: false,
        credentialPurpose: undefined,
        selectedRoutes: [],
        generation: String(Number(existing.generation) + 1),
        tombstone: true
      }
      providers = providers.map((item) => item.id === input.providerId ? provider : item)
      if (selected === input.providerId) selected = undefined
      return {
        schemaVersion: 1,
        registryRevision: String(revision),
        registryIncarnation: REGISTRY_INCARNATION,
        provider
      }
    }
    if (input.operation === 'explicit-delete') {
      revision += 1
      providers = providers.filter((provider) => provider.id !== input.providerId)
      if (selected === input.providerId) selected = undefined
      return {
        schemaVersion: 1,
        registryRevision: String(revision),
        registryIncarnation: REGISTRY_INCARNATION,
        deletedProviderId: input.providerId
      }
    }
    if (input.operation === 'select') {
      selected = input.providerId
      revision += 1
      return snapshot()
    }
    throw new Error(`Unexpected registry operation: ${input.operation}`)
  })
  return { request, snapshot }
}

describe('initialSetupSelection', () => {
  it('preselects the active provider card when it is a known preset', () => {
    expect(initialSetupSelection(settingsWithActiveXiaomi())).toEqual({ presetId: 'xiaomi', mode: 'api' })
  })

  it('preselects the token plan mode for token plan profiles', () => {
    const minimax = INITIAL_SETUP_PROVIDER_PRESETS.find((preset) => preset.id === 'minimax')
    const tokenPlanProfile = minimax ? modelProviderTokenPlanProfile(minimax) : null
    expect(tokenPlanProfile).toBeTruthy()
    const current = settings({
      provider: { providers: [tokenPlanProfile!] },
      runtime: { providerId: 'minimax-token-plan' }
    })
    expect(initialSetupSelection(current)).toEqual({ presetId: 'minimax', mode: 'token-plan' })
  })

  it('preselects provider.activeProviderId when runtime providerId is blank', () => {
    const current = settings({
      provider: {
        activeProviderId: 'xiaomi',
        providers: [
          { id: 'xiaomi', name: 'Xiaomi', baseUrl: 'https://api.xiaomimimo.com/v1', models: ['mimo-v2.5'] }
        ]
      },
      runtime: { providerId: '' }
    })
    expect(initialSetupSelection(current)).toEqual({ presetId: 'xiaomi', mode: 'api' })
  })

  it('falls back to deepseek for unknown or empty active providers', () => {
    expect(initialSetupSelection(settings())).toEqual({ presetId: 'deepseek', mode: 'api' })
    expect(initialSetupSelection(settings({ runtime: { providerId: 'custom-provider-2' } })))
      .toEqual({ presetId: 'deepseek', mode: 'api' })
  })
})

describe('initialSetupDrafts', () => {
  it('seeds only key-free endpoints even when legacy input contains credential bytes', () => {
    const canary = ['closure', 'b', 'draft'].join('-')
    const current = settings({
      provider: {
        apiKey: canary,
        baseUrl: 'https://api.deepseek.com',
        providers: [{
          id: 'deepseek',
          name: 'DeepSeek',
          apiKey: canary,
          baseUrl: 'https://api.deepseek.com',
          models: ['deepseek-chat']
        }]
      }
    })
    const drafts = initialSetupDrafts(current)

    expect(Object.values(drafts).every((draft) => draft.apiKey === '')).toBe(true)
    expect(JSON.stringify(current).includes(canary)).toBe(false)
    expect(drafts.deepseek.baseUrl).toBe('https://api.deepseek.com')
    expect(drafts['xiaomi-token-plan'].baseUrl).toBe('https://token-plan-cn.xiaomimimo.com/v1')
  })

  it('keeps unsupported presets out of onboarding', () => {
    const excludedIds = ['litellm', 'zhipu-coding-plan', 'zai-coding-plan', 'kimi-code', 'moonshot-cn']
    const drafts = initialSetupDrafts(settings())
    expect(INITIAL_SETUP_PROVIDER_PRESETS.map((preset) => preset.id)).toEqual(['xiaomi', 'minimax'])
    for (const id of excludedIds) expect(drafts[id]).toBeUndefined()
  })
})

describe('buildInitialSetupSettings', () => {
  it('activates DeepSeek while keeping every settings projection key-free', () => {
    const canary = ['closure', 'b', 'settings'].join('-')
    const drafts = initialSetupDrafts(settingsWithActiveXiaomi())
    drafts.deepseek = { apiKey: canary, baseUrl: 'https://new.example/v1' }

    const next = buildInitialSetupSettings(settingsWithActiveXiaomi(), drafts, { presetId: 'deepseek', mode: 'api' })
    const serialized = JSON.stringify(next)

    expect(getAnalytixRuntimeSettings(next).providerId).toBe('deepseek')
    expect(getModelProviderSettings(next).baseUrl).toBe('https://new.example/v1')
    expect(serialized.includes(canary)).toBe(false)
    expect(serialized.includes('apiKey')).toBe(false)
  })

  it('creates and activates a key-free token-plan profile', () => {
    const drafts = initialSetupDrafts(settings())
    drafts['xiaomi-token-plan'] = {
      apiKey: ['token', 'plan', 'draft'].join('-'),
      baseUrl: 'https://token-plan-sgp.xiaomimimo.com/v1'
    }
    const next = buildInitialSetupSettings(settings(), drafts, { presetId: 'xiaomi', mode: 'token-plan' })
    const profile = getModelProviderSettings(next).providers.find((item) => item.id === 'xiaomi-token-plan')

    expect(profile?.baseUrl).toBe('https://token-plan-sgp.xiaomimimo.com/v1')
    expect(profile?.speech?.protocol).toBe('mimo-asr')
    expect(getAnalytixRuntimeSettings(next).providerId).toBe('xiaomi-token-plan')
    expect(JSON.stringify(next).includes('apiKey')).toBe(false)
  })

  it('auto-wires speech and image from explicitly filled drafts without storing them', () => {
    const drafts = initialSetupDrafts(settings())
    drafts.xiaomi = { ...drafts.xiaomi, apiKey: 'explicit-xiaomi-draft' }
    drafts.minimax = { ...drafts.minimax, apiKey: 'explicit-minimax-draft' }
    const next = buildInitialSetupSettings(settings(), drafts, { presetId: 'xiaomi', mode: 'api' })
    const runtime = getAnalytixRuntimeSettings(next)

    expect(runtime.speechToText.providerId).toBe('xiaomi')
    expect(runtime.imageGeneration.providerId).toBe('minimax')
    expect(JSON.stringify(next).includes('explicit-')).toBe(false)
  })

  it('never overrides existing speech config while auto-wiring', () => {
    const configured = settings({ runtime: { speechToText: { providerId: 'custom' } } })
    const drafts = initialSetupDrafts(configured)
    drafts.xiaomi = { ...drafts.xiaomi, apiKey: 'explicit-draft' }
    const next = buildInitialSetupSettings(configured, drafts, { presetId: 'xiaomi', mode: 'api' })
    expect(getAnalytixRuntimeSettings(next).speechToText.providerId).toBe('custom')
  })

  it('preserves unrelated custom provider metadata but strips legacy credentials', () => {
    const canary = ['legacy', 'custom', 'credential'].join('-')
    const current = settings({
      provider: {
        providers: [{
          id: 'custom-provider-2',
          name: 'zenmux',
          apiKey: canary,
          baseUrl: 'https://zenmux.ai/api'
        }]
      }
    })
    const next = buildInitialSetupSettings(current, initialSetupDrafts(current), {
      presetId: 'deepseek',
      mode: 'api'
    })
    const custom = getModelProviderSettings(next).providers.find((item) => item.id === 'custom-provider-2')

    expect(custom?.baseUrl).toBe('https://zenmux.ai/api')
    expect(JSON.stringify(next).includes(canary)).toBe(false)
  })
})

describe('credential persistence', () => {
  it('configures and deliberately removes a custom Provider observation binding without touching its credential', async () => {
    const existing = publicProvider('custom-observation', true, {
      kind: 'custom-endpoint',
      endpoint: 'https://custom.provider.invalid/v1',
      selectedRoutes: ['primary']
    })
    const registry = registryHarness([existing], existing.id)
    const binding = {
      schemaVersion: 1 as const,
      endpoint: 'https://quota.custom-provider.invalid/v1/observation',
      method: 'GET' as const,
      projection: 'normalized-quota-v1' as const
    }

    const configured = await persistProviderAccountObservationBinding({
      providerId: existing.id,
      accountObservation: binding,
      requestProviderRegistry: registry.request
    })
    expect(configured.providers[0]).toMatchObject({
      accountObservation: binding,
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      generation: existing.generation,
      incarnation: existing.incarnation
    })
    const configure = registry.request.mock.calls.map(([request]) => request)
      .find((request) => request.operation === 'update')
    expect(configure).toMatchObject({
      operation: 'update',
      provider: { accountObservation: binding },
      credential: { kind: 'keep' }
    })
    expect(JSON.stringify(configure)).not.toMatch(/credentialRef|valueBase64|secret/)

    const removed = await persistProviderAccountObservationBinding({
      providerId: existing.id,
      accountObservation: null,
      requestProviderRegistry: registry.request
    })
    expect(removed.providers[0]?.accountObservation).toBeUndefined()
    expect(removed.providers[0]).toMatchObject({ credentialConfigured: true, generation: existing.generation })
    const updates = registry.request.mock.calls.map(([request]) => request)
      .filter((request) => request.operation === 'update')
    expect(updates.at(-1)).not.toHaveProperty('provider.accountObservation')
    expect(updates.at(-1)).toMatchObject({ credential: { kind: 'keep' } })
  })

  it('rejects invalid or stale observation configuration without deleting current metadata', async () => {
    const existing = publicProvider('custom-observation-stale', true, {
      kind: 'custom-endpoint',
      accountObservation: {
        schemaVersion: 1,
        endpoint: 'https://quota.current.invalid/v1',
        method: 'GET',
        projection: 'normalized-quota-v1'
      }
    })
    const registry = registryHarness([existing], existing.id)
    await expect(persistProviderAccountObservationBinding({
      providerId: existing.id,
      accountObservation: {
        schemaVersion: 1,
        endpoint: 'http://localhost/quota',
        method: 'GET',
        projection: 'normalized-quota-v1'
      },
      requestProviderRegistry: registry.request
    })).rejects.toThrow()
    expect(registry.request).not.toHaveBeenCalled()

    const staleRequest = vi.fn(async (request: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
      if (request.operation === 'list') return registry.snapshot()
      return {
        schemaVersion: 1,
        error: {
          code: 'conflict',
          message: 'The provider registry state has changed.'
        }
      }
    })
    await expect(persistProviderAccountObservationBinding({
      providerId: existing.id,
      accountObservation: {
        schemaVersion: 1,
        endpoint: 'https://quota.changed.invalid/v1',
        method: 'GET',
        projection: 'normalized-quota-v1'
      },
      requestProviderRegistry: staleRequest
    })).rejects.toThrow('state has changed')
    expect(registry.snapshot().providers[0]?.accountObservation).toEqual(existing.accountObservation)
  })

  it('commits only explicit valid model/media selections and ordered local routes', async () => {
    const profile = {
      id: 'provider-selection',
      name: 'Provider Selection',
      baseUrl: 'https://provider-selection.invalid/v1',
      endpointFormat: 'chat_completions',
      models: ['chat-alpha', 'chat-beta'],
      modelProfiles: {},
      image: {
        protocol: 'openai-images',
        baseUrl: 'https://provider-selection.invalid/v1',
        models: ['image-alpha']
      }
    } satisfies ModelProviderProfileV1

    expect(providerRegistryInput(profile, 'removed-chat', '', {
      selectedMediaModel: 'removed-image',
      selectedRoutes: ['primary', 'provider-fallback']
    })).toMatchObject({
      selectedModel: '',
      selectedMediaModel: '',
      selectedRoutes: ['primary', 'provider-fallback']
    })

    const existing = publicProvider(profile.id, true, {
      kind: 'openai-compatible',
      endpoint: profile.baseUrl,
      models: profile.models,
      mediaModels: ['image-alpha'],
      selectedRoutes: ['primary']
    })
    const registry = registryHarness([existing], existing.id)
    await persistProviderSettingsDraft({
      profile,
      selectedModel: 'chat-beta',
      selectedMediaModel: 'image-alpha',
      selectedRoutes: ['primary', 'provider-fallback'],
      credentialDraft: '',
      requestProviderRegistry: registry.request
    })

    const update = registry.request.mock.calls.map(([request]) => request)
      .find((request) => request.operation === 'update')
    expect(update).toMatchObject({
      operation: 'update',
      provider: {
        selectedModel: 'chat-beta',
        selectedMediaModel: 'image-alpha',
        selectedRoutes: ['primary', 'provider-fallback']
      },
      credential: { kind: 'keep' }
    })
  })

  it('classifies omitted, empty, masked, sentinel and redacted drafts as preserve', () => {
    for (const value of [
      undefined, '', '   ', '****', '••••', '●●●●', 'xxxx', 'sk-****', 'key_xxxx',
      'redacted', '[redacted]', '<redacted>', '__redacted__',
      'masked', '[masked]', '<masked>', '__masked__',
      'unset', 'not-set', 'not_set', 'null', 'undefined'
    ]) {
      expect(credentialDraftAction(value)).toEqual({ kind: 'preserve', value: null })
    }
    expect(credentialDraftAction('synthetic-nonempty-value')).toEqual({
      kind: 'set',
      value: 'synthetic-nonempty-value'
    })
  })

  it('writes an explicit nonempty credential only through Registry and saves key-free settings', async () => {
    const canary = ['closure', 'b', 'registry'].join('-')
    const drafts = initialSetupDrafts(settings())
    drafts.deepseek = { ...drafts.deepseek, apiKey: canary }
    const registry = registryHarness()
    const saveSettings = vi.fn(async (next: AppSettingsV1) => next)

    const saved = await persistInitialSetup({
      settings: settings(),
      drafts,
      selection: { presetId: 'deepseek', mode: 'api' },
      requestProviderRegistry: registry.request,
      saveSettings
    })
    const requests = registry.request.mock.calls.map(([request]) => request)
    const connect = requests.find((request) => request.operation === 'connect')

    expect(connect?.operation === 'connect' && atob(connect.credential.valueBase64) === canary).toBe(true)
    expect(JSON.stringify(saved).includes(canary)).toBe(false)
    expect(JSON.stringify(saved).includes('apiKey')).toBe(false)
    expect(requests.some((request) => request.operation === 'select')).toBe(true)
    expect(requests.some((request) => request.operation === 'disconnect' || request.operation === 'explicit-delete')).toBe(false)
    expect(saveSettings).toHaveBeenCalledTimes(1)
  })

  it('verifies the committed credential and connection before settings and final selection', async () => {
    const drafts = initialSetupDrafts(settings())
    drafts.deepseek = { ...drafts.deepseek, apiKey: 'synthetic-setup-order' }
    const registry = registryHarness()
    const events: string[] = []
    const requestProviderRegistry = vi.fn(async (request: ProviderRegistryRequest) => {
      events.push(`registry:${request.operation}`)
      return registry.request(request)
    })
    const saveSettings = vi.fn(async (next: AppSettingsV1) => {
      events.push('settings:save')
      return next
    })

    await persistInitialSetup({
      settings: settings(),
      drafts,
      selection: { presetId: 'deepseek', mode: 'api' },
      requestProviderRegistry,
      saveSettings
    })

    expect(events).toEqual([
      'registry:list',
      'registry:connect',
      'registry:list',
      'registry:probe',
      'settings:save',
      'registry:select',
      'registry:list'
    ])
  })

  it('reports failure when final Registry selection cannot be verified', async () => {
    const drafts = initialSetupDrafts(settings())
    drafts.deepseek = { ...drafts.deepseek, apiKey: 'synthetic-stale-selection' }
    const registry = registryHarness()
    let selected = false
    const requestProviderRegistry = vi.fn(async (request: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
      const result = await registry.request(request)
      if (request.operation === 'select') selected = true
      if (request.operation === 'list' && selected && 'providers' in result) {
        return { ...result, selectedProviderId: undefined }
      }
      return result
    })
    const saveSettings = vi.fn(async (next: AppSettingsV1) => next)

    await expect(persistInitialSetup({
      settings: settings(),
      drafts,
      selection: { presetId: 'deepseek', mode: 'api' },
      requestProviderRegistry,
      saveSettings
    })).rejects.toThrow('Provider selection could not be verified')
    expect(saveSettings).toHaveBeenCalledTimes(1)
  })

  it('replaces an existing credential without overwriting Registry provider metadata', async () => {
    const canary = ['closure', 'b', 'provider-form'].join('-')
    const existing = publicProvider('deepseek', true, {
      endpoint: 'https://registry-owned.example/v1',
      proxy: 'https://registry-proxy.example',
      models: ['registry-model'],
      mediaModels: ['registry-image'],
      selectedModel: 'registry-model',
      selectedMediaModel: 'registry-image',
      selectedRoutes: ['chat', 'image']
    })
    const registry = registryHarness([existing], 'deepseek')
    const profile = getModelProviderSettings(settings()).providers.find((item) => item.id === 'deepseek')!

    const result = await persistProviderCredentialDraft({
      profile,
      selectedModel: profile.models[0] ?? '',
      credentialDraft: canary,
      requestProviderRegistry: registry.request
    })
    const requests = registry.request.mock.calls.map(([request]) => request)
    const replacement = requests.find((request) => request.operation === 'credential-replace')

    expect(replacement?.operation === 'credential-replace' &&
      atob(replacement.credential.valueBase64) === canary).toBe(true)
    expect(replacement).toMatchObject({
      providerId: 'deepseek',
      expected: {
        registryRevision: '1',
        registryIncarnation: REGISTRY_INCARNATION,
        providerRevision: '1',
        providerGeneration: '1',
        providerIncarnation: PROVIDER_INCARNATION,
        providerCredentialPurpose: 'provider-api-key'
      }
    })
    expect(registry.snapshot().providers[0]).toMatchObject({
      endpoint: existing.endpoint,
      proxy: existing.proxy,
      models: existing.models,
      mediaModels: existing.mediaModels,
      selectedModel: existing.selectedModel,
      selectedMediaModel: existing.selectedMediaModel,
      selectedRoutes: existing.selectedRoutes
    })
    expect(result).toEqual({ credentialConfigured: true })
    expect(requests.some((request) =>
      request.operation === 'update' || request.operation === 'disconnect' ||
      request.operation === 'explicit-delete' || request.operation === 'select'
    )).toBe(false)
  })

  it.each([
    undefined, '', '   ', '****', '••••', '●●●●', 'xxxx', 'sk-****', 'key_xxxx',
    'redacted', '[redacted]', '<redacted>', '__redacted__',
    'masked', '[masked]', '<masked>', '__masked__',
    'unset', 'not-set', 'not_set', 'null', 'undefined'
  ])('preserves an existing committed credential with zero mutation for draft %j',
    async (draftValue) => {
      const drafts = initialSetupDrafts(settings())
      drafts.deepseek = { ...drafts.deepseek, apiKey: draftValue ?? '' }
      const registry = registryHarness([publicProvider()], 'deepseek')
      const saveSettings = vi.fn(async (next: AppSettingsV1) => next)

      await persistInitialSetup({
        settings: settings(),
        drafts,
        selection: { presetId: 'deepseek', mode: 'api' },
        requestProviderRegistry: registry.request,
        saveSettings
      })
      const requests = registry.request.mock.calls.map(([request]) => request)

      expect(requests.some((request) =>
        request.operation === 'connect' || request.operation === 'update' ||
        request.operation === 'credential-replace' || request.operation === 'disconnect' ||
        request.operation === 'explicit-delete' || request.operation === 'select'
      )).toBe(false)
      expect(saveSettings).toHaveBeenCalledTimes(1)
    }
  )

  it.each([
    undefined, '', '   ', '****', '••••', '●●●●', 'xxxx', 'sk-****', 'key_xxxx',
    'redacted', '[redacted]', '<redacted>', '__redacted__',
    'masked', '[masked]', '<masked>', '__masked__',
    'unset', 'not-set', 'not_set', 'null', 'undefined'
  ])('keeps a new Provider preserve draft at zero Registry mutation for %j', async (draftValue) => {
    const registry = registryHarness()
    const profile = getModelProviderSettings(settings()).providers.find((item) => item.id === 'deepseek')!

    const result = await persistProviderCredentialDraft({
      profile,
      selectedModel: profile.models[0] ?? '',
      credentialDraft: draftValue,
      requestProviderRegistry: registry.request
    })

    expect(result).toEqual({ credentialConfigured: false })
    expect(registry.request.mock.calls.map(([request]) => request.operation)).toEqual(['list'])
  })

  it('updates Registry metadata with credential keep and treats masked input as write-only no-op', async () => {
    const profile = getModelProviderSettings(settings()).providers.find((item) => item.id === 'deepseek')!
    const committedInput = providerRegistryInput(profile, profile.models[0] ?? '', '')
    const accountObservation = {
      schemaVersion: 1 as const,
      endpoint: 'https://quota.deepseek.invalid/v1/observation',
      method: 'GET' as const,
      projection: 'normalized-quota-v1' as const
    }
    const existing = publicProvider(profile.id, true, {
      kind: 'deepseek',
      endpoint: committedInput.endpoint,
      proxy: undefined,
      models: committedInput.models,
      mediaModels: committedInput.mediaModels,
      selectedModel: committedInput.selectedModel || undefined,
      selectedMediaModel: committedInput.selectedMediaModel || undefined,
      selectedRoutes: committedInput.selectedRoutes,
      accountObservation
    })
    const registry = registryHarness([existing], existing.id)

    const noOp = await persistProviderSettingsDraft({
      profile,
      selectedModel: profile.models[0] ?? '',
      credentialDraft: '****',
      requestProviderRegistry: registry.request
    })
    expect(noOp.credentialConfigured).toBe(true)
    expect(registry.request.mock.calls.map(([request]) => request.operation)).toEqual(['list'])

    const changed = await persistProviderSettingsDraft({
      profile: { ...profile, baseUrl: 'https://registry-updated.invalid/v1' },
      selectedModel: profile.models[0] ?? '',
      credentialDraft: '[redacted]',
      requestProviderRegistry: registry.request
    })
    const updateRequest = registry.request.mock.calls.map(([request]) => request)
      .find((request) => request.operation === 'update')
    expect(updateRequest).toMatchObject({
      operation: 'update',
      provider: {
        endpoint: 'https://registry-updated.invalid/v1',
        selectedMediaModel: existing.selectedMediaModel ?? '',
        selectedRoutes: existing.selectedRoutes,
        accountObservation
      },
      credential: { kind: 'keep' }
    })
    expect(JSON.stringify(updateRequest)).not.toContain('redacted')
    expect(changed.provider?.endpoint).toBe('https://registry-updated.invalid/v1')
    expect(changed.provider?.accountObservation).toEqual(accountObservation)
  })

  it('keeps select, disconnect, and explicit delete as separate fenced operations', async () => {
    const existing = publicProvider('deepseek', true)
    const registry = registryHarness([existing])

    const selected = await selectProviderRegistryEntry({
      providerId: existing.id,
      requestProviderRegistry: registry.request
    })
    expect(selected.selectedProviderId).toBe(existing.id)
    const disconnected = await disconnectProviderRegistryEntry({
      providerId: existing.id,
      requestProviderRegistry: registry.request
    })
    expect(disconnected.providers.find((provider) => provider.id === existing.id)).toMatchObject({
      tombstone: true,
      credentialConfigured: false
    })
    await deleteProviderRegistryEntry({
      providerId: existing.id,
      requestProviderRegistry: registry.request
    })
    expect(registry.snapshot().providers).toEqual([])
    expect(registry.request.mock.calls.map(([request]) => request.operation)).toEqual([
      'list', 'select', 'list', 'list', 'disconnect', 'list', 'list', 'explicit-delete', 'list'
    ])
  })

  it('passes custom endpoint metadata and key-free proxy through a new Registry connect', async () => {
    const registry = registryHarness()
    const profile = {
      id: 'custom-registry-provider',
      name: 'Custom Registry Provider',
      baseUrl: 'https://custom-provider.invalid/models',
      endpointFormat: 'custom_endpoint',
      models: ['custom-model'],
      modelProfiles: {}
    } satisfies ModelProviderProfileV1

    await persistProviderSettingsDraft({
      profile,
      selectedModel: 'custom-model',
      proxy: 'https://configured-proxy.example',
      credentialDraft: 'synthetic-new-provider-value',
      requestProviderRegistry: registry.request
    })

    const connect = registry.request.mock.calls
      .map(([request]) => request)
      .find((request) => request.operation === 'connect')
    expect(connect).toMatchObject({
      operation: 'connect',
      provider: {
        id: profile.id,
        kind: 'custom-endpoint',
        endpoint: profile.baseUrl,
        proxy: 'https://configured-proxy.example',
        models: profile.models,
        selectedModel: 'custom-model'
      }
    })
  })

  it('explicitly deletes a Registry provider and verifies absence before returning', async () => {
    const registry = registryHarness([publicProvider('custom-provider')], 'custom-provider')

    await deleteProviderRegistryEntry({
      providerId: 'custom-provider',
      requestProviderRegistry: registry.request
    })

    expect(registry.request.mock.calls.map(([request]) => request.operation))
      .toEqual(['list', 'explicit-delete', 'list'])
    expect(registry.request.mock.calls[1]?.[0]).toMatchObject({
      operation: 'explicit-delete',
      providerId: 'custom-provider',
      expected: {
        registryRevision: '1',
        registryIncarnation: REGISTRY_INCARNATION,
        providerRevision: '1',
        providerGeneration: '1',
        providerIncarnation: PROVIDER_INCARNATION,
        providerCredentialPurpose: 'provider-api-key'
      }
    })
    expect(registry.snapshot().providers).toEqual([])
  })

  it('treats an already absent Registry provider as an idempotent explicit delete', async () => {
    const registry = registryHarness()

    await deleteProviderRegistryEntry({
      providerId: 'custom-provider',
      requestProviderRegistry: registry.request
    })

    expect(registry.request.mock.calls.map(([request]) => request.operation)).toEqual(['list'])
  })

  it('fails closed when explicit delete fails or cannot be verified', async () => {
    const snapshot = registryHarness([publicProvider('custom-provider')], 'custom-provider').snapshot()
    const rejected = vi.fn(async (request: ProviderRegistryRequest): Promise<ProviderRegistryResult> =>
      request.operation === 'list'
        ? snapshot
        : {
            schemaVersion: 1,
            error: { code: 'conflict', message: 'The provider registry state has changed.' }
          }
    )
    await expect(deleteProviderRegistryEntry({
      providerId: 'custom-provider',
      requestProviderRegistry: rejected
    })).rejects.toThrow('The provider registry state has changed.')

    const stale = vi.fn(async (request: ProviderRegistryRequest): Promise<ProviderRegistryResult> =>
      request.operation === 'explicit-delete'
        ? {
            schemaVersion: 1,
            registryRevision: '2',
            registryIncarnation: REGISTRY_INCARNATION,
            deletedProviderId: 'custom-provider'
          }
        : snapshot
    )
    await expect(deleteProviderRegistryEntry({
      providerId: 'custom-provider',
      requestProviderRegistry: stale
    })).rejects.toThrow('The provider registry delete could not be verified.')
  })


  it('fails closed when preserve is requested without a committed Registry credential', async () => {
    const drafts = initialSetupDrafts(settings())
    const registry = registryHarness()
    const saveSettings = vi.fn(async (next: AppSettingsV1) => next)

    await expect(persistInitialSetup({
      settings: settings(),
      drafts,
      selection: { presetId: 'deepseek', mode: 'api' },
      requestProviderRegistry: registry.request,
      saveSettings
    })).rejects.toThrow('A committed Provider credential is required.')
    expect(registry.request.mock.calls.some(([request]) =>
      request.operation === 'credential-replace' || request.operation === 'disconnect' ||
      request.operation === 'explicit-delete'
    )).toBe(false)
    expect(saveSettings).not.toHaveBeenCalled()
  })
})

describe('initialSetupProfileId', () => {
  it('maps selection to profile ids', () => {
    expect(initialSetupProfileId({ presetId: 'deepseek', mode: 'api' })).toBe('deepseek')
    expect(initialSetupProfileId({ presetId: 'xiaomi', mode: 'token-plan' })).toBe('xiaomi-token-plan')
    expect(initialSetupProfileId({ presetId: 'minimax', mode: 'api' })).toBe('minimax')
  })
})


describe('initialization completion and recovery boundaries', () => {
  it('does not select or mark complete when the stored credential fails the Provider probe', async () => {
    const registry = registryHarness()
    const drafts = initialSetupDrafts(settings())
    drafts.deepseek.apiKey = 'synthetic-rejected-key'
    const saveSettings = vi.fn(async (next: AppSettingsV1) => next)
    const request = vi.fn(async (input: ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
      const result = await registry.request(input)
      return input.operation === 'probe' && 'status' in result
        ? { ...result, status: 'auth_failed', code: 401, modelCount: 0 } : result
    })
    await expect(persistInitialSetup({ settings: settings(), drafts,
      selection: { presetId: 'deepseek', mode: 'api' }, requestProviderRegistry: request, saveSettings
    })).rejects.toThrow('connection could not be verified')
    expect(saveSettings).not.toHaveBeenCalled()
    expect(registry.snapshot().selectedProviderId).toBeUndefined()
    expect(registry.snapshot().providers[0].credentialConfigured).toBe(true)
    expect(request.mock.calls.some(([r]) => r.operation === 'disconnect' || r.operation === 'explicit-delete')).toBe(false)
  })

  it('retains the committed credential after settings failure and retries without a new key', async () => {
    const registry = registryHarness()
    const drafts = initialSetupDrafts(settings())
    drafts.deepseek.apiKey = 'synthetic-retry-key'
    await expect(persistInitialSetup({ settings: settings(), drafts,
      selection: { presetId: 'deepseek', mode: 'api', model: 'deepseek-flash' },
      requestProviderRegistry: registry.request,
      saveSettings: async () => { throw new Error('synthetic settings write failure') }
    })).rejects.toThrow('settings write failure')
    expect(registry.snapshot().selectedProviderId).toBeUndefined()
    const count = registry.request.mock.calls.length
    const saved = await persistInitialSetup({ settings: settings(), drafts: initialSetupDrafts(settings()),
      selection: { presetId: 'deepseek', mode: 'api', model: 'deepseek-flash' },
      requestProviderRegistry: registry.request, saveSettings: async next => next
    })
    expect(saved.runtime.model).toBe('deepseek-flash')
    expect(registry.snapshot().selectedProviderId).toBe('deepseek')
    expect(registry.request.mock.calls.slice(count).some(([r]) =>
      r.operation === 'connect' || r.operation === 'credential-replace')).toBe(false)
  })
})
