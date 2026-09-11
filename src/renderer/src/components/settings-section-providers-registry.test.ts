import { describe, expect, it } from 'vitest'
import type { ModelProviderProfileV1 } from '@shared/app-settings'
import type { ProviderRegistryPublicProvider } from '@shared/analytix-api'
import providerSettingsSource from './settings-section-providers.tsx?raw'
import {
  modelProvidersSettingsPatch,
  providerAccountObservationStateFromResult,
  providerAccountObservationStateIsCurrent,
  providerProfileFromRegistry,
  providerRegistryRoutingDraft,
  providerRegistryRoutingDraftWithSelectedModel,
  providerRegistryRoutingDraftWithToggledProvider,
  providerRegistrySelectedRoutes
} from './settings-section-providers'

const registryProvider: ModelProviderProfileV1 = {
  id: 'provider-registry-alpha',
  name: 'Registry Alpha',
  baseUrl: 'https://provider.invalid/v1',
  endpointFormat: 'chat_completions',
  models: ['model-alpha'],
  modelProfiles: {}
}

describe('Provider Settings Registry authority', () => {
  it('does not serialize Registry-owned Provider metadata into ordinary settings', () => {
    const patch = modelProvidersSettingsPatch({
      provider: { baseUrl: '', proxy: { enabled: false, url: '' }, providers: [] },
      providers: [registryProvider]
    })

    expect(patch).not.toHaveProperty('provider')
    expect(JSON.stringify(patch)).not.toContain(registryProvider.baseUrl)
    expect(JSON.stringify(patch)).not.toContain(registryProvider.id)
  })

  it('projects built-in and custom metadata from Registry values instead of catalog/settings values', () => {
    const committed = {
      id: 'deepseek',
      kind: 'anthropic-compatible',
      endpoint: 'https://registry-owned.invalid/messages',
      proxy: 'http://registry-proxy.invalid:8080',
      models: ['registry-chat-model'],
      mediaModels: ['registry-media-model'],
      selectedModel: 'registry-chat-model',
      selectedMediaModel: 'registry-media-model',
      selectedRoutes: [],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '4',
      generation: '2',
      incarnation: 'inc_provider-registry-alpha-1234567890',
      tombstone: false
    } satisfies ProviderRegistryPublicProvider

    expect(providerProfileFromRegistry(committed)).toMatchObject({
      id: committed.id,
      baseUrl: committed.endpoint,
      endpointFormat: 'messages',
      models: committed.models,
      image: { baseUrl: committed.endpoint, models: committed.mediaModels }
    })
    expect(providerProfileFromRegistry({ ...committed, id: 'custom-registry', kind: 'custom-endpoint' }))
      .toMatchObject({
        id: 'custom-registry',
        name: 'custom-registry',
        baseUrl: committed.endpoint,
        endpointFormat: 'custom_endpoint',
        models: committed.models
      })
  })

  it('uses catalog only to classify Registry-committed media metadata', () => {
    const committed = {
      id: 'minimax',
      kind: 'openai-compatible',
      endpoint: 'https://registry-minimax.invalid/v1',
      models: ['MiniMax-M2.7'],
      mediaModels: ['music-2.6'],
      selectedModel: 'MiniMax-M2.7',
      selectedMediaModel: 'music-2.6',
      selectedRoutes: [],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '3',
      generation: '1',
      incarnation: 'inc_provider-registry-minimax-1234567890',
      tombstone: false
    } satisfies ProviderRegistryPublicProvider

    const projection = providerProfileFromRegistry(committed)
    expect(projection.models).toEqual(committed.models)
    expect(projection.music?.models).toEqual(committed.mediaModels)
    expect(projection.speech).toBeUndefined()
    expect(projection.textToSpeech).toBeUndefined()
    expect(projection.video).toBeUndefined()
    expect(projection.image).toBeUndefined()
    expect(JSON.stringify(projection)).not.toContain('speech-2.8-hd')
  })

  it('does not create a local-only model import after Registry discovery commits', () => {
    expect(providerSettingsSource).not.toContain('setPendingImport(')
    expect(providerSettingsSource).not.toContain('<ProviderModelImportDialog')
    expect(providerSettingsSource).not.toContain('importPickedModels')
  })

  it('keeps protected recovery as explicit Main-owned actions with no artifact input', () => {
    expect(providerSettingsSource).toContain('Portable manifest / protected credential recovery')
    expect(providerSettingsSource).toContain("runProtectedRecoveryAction('createDestinationRequest')")
    expect(providerSettingsSource).toContain("runProtectedRecoveryAction('createSourceBundle')")
    expect(providerSettingsSource).toContain("runProtectedRecoveryAction('applyDestinationBundle')")
    expect(providerSettingsSource).toContain("runProtectedRecoveryAction('finalizeSourceReceipt')")
    expect(providerSettingsSource).not.toContain('requestPath')
    expect(providerSettingsSource).not.toContain('bundlePath')
    expect(providerSettingsSource).not.toContain('receiptPath')
  })

  it('keeps custom observation configuration operation-specific and deliberate', () => {
    expect(providerSettingsSource).toContain('persistProviderAccountObservationBinding({')
    expect(providerSettingsSource).toContain("accountObservation: remove")
    expect(providerSettingsSource).toContain("method: 'GET'")
    expect(providerSettingsSource).toContain("projection: 'normalized-quota-v1'")
    expect(providerSettingsSource).toContain("'modelProviderQuotaBindingRemove'")
    expect(providerSettingsSource).not.toMatch(/accountObservationEndpointDrafts[\s\S]{0,500}(?:credentialRef|valueBase64)/)
    expect(providerSettingsSource).toContain('latestAccountObservationRequest.current[provider.id] !== requestId')
    expect(providerSettingsSource).toContain('current[provider.id]?.requestId !== requestId')
  })

  it('discards observation results after any full-fence binding selection or expiry drift', () => {
    const binding = {
      schemaVersion: 1 as const,
      endpoint: 'https://quota.provider.invalid/v1/observation',
      method: 'GET' as const,
      projection: 'normalized-quota-v1' as const
    }
    const provider = {
      id: 'provider-observation', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
      models: ['model-alpha'], mediaModels: [], selectedModel: 'model-alpha', selectedRoutes: ['primary'],
      credentialConfigured: true, credentialPurpose: 'provider-api-key' as const,
      revision: '7', generation: '3', incarnation: `inc_${'b'.repeat(43)}`, tombstone: false,
      accountObservation: binding
    }
    const snapshot = {
      schemaVersion: 1 as const,
      registryRevision: '9', registryIncarnation: `inc_${'a'.repeat(43)}`,
      selectedProviderId: provider.id,
      providers: [provider]
    }
    const observation = {
      schemaVersion: 1 as const,
      registryRevision: snapshot.registryRevision,
      registryIncarnation: snapshot.registryIncarnation,
      providerId: provider.id,
      providerRevision: provider.revision,
      providerGeneration: provider.generation,
      providerIncarnation: provider.incarnation,
      providerCredentialPurpose: provider.credentialPurpose,
      status: 'available' as const,
      observedAt: '2026-08-31T00:00:00Z',
      expiresAt: '2026-08-31T00:05:00Z',
      quota: 100,
      remaining: 80
    }
    const nowMs = Date.parse('2026-08-31T00:01:00Z')
    const state = providerAccountObservationStateFromResult({
      requestId: 4,
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation,
      currentSnapshot: snapshot,
      nowMs
    })
    expect(state).toMatchObject({
      requestId: 4,
      selectedProviderId: provider.id,
      registryRevision: snapshot.registryRevision,
      registryIncarnation: snapshot.registryIncarnation,
      providerRevision: provider.revision,
      providerGeneration: provider.generation,
      providerIncarnation: provider.incarnation,
      providerCredentialPurpose: provider.credentialPurpose,
      accountObservation: binding,
      expiresAt: observation.expiresAt,
      remaining: 80
    })
    expect(providerAccountObservationStateIsCurrent(state!, snapshot, nowMs)).toBe(true)
    expect(providerAccountObservationStateFromResult({
      requestId: 4,
      expectedSnapshot: snapshot,
      expectedProvider: provider,
      observation: { ...observation, providerCredentialPurpose: 'provider-oauth-token-bundle' },
      currentSnapshot: snapshot,
      nowMs
    })).toBeNull()

    for (const currentSnapshot of [
      { ...snapshot, selectedProviderId: 'provider-other' },
      { ...snapshot, registryRevision: '10' },
      { ...snapshot, registryIncarnation: `inc_${'c'.repeat(43)}` },
      { ...snapshot, providers: [{ ...provider, revision: '8' }] },
      { ...snapshot, providers: [{ ...provider, generation: '4' }] },
      { ...snapshot, providers: [{ ...provider, incarnation: `inc_${'d'.repeat(43)}` }] },
      { ...snapshot, providers: [{ ...provider, credentialPurpose: 'provider-oauth-token-bundle' as const }] },
      { ...snapshot, providers: [{ ...provider, tombstone: true, credentialConfigured: false }] },
      { ...snapshot, providers: [{ ...provider, accountObservation: { ...binding, endpoint: 'https://changed.invalid/quota' } }] }
    ]) {
      expect(providerAccountObservationStateFromResult({
        requestId: 4,
        expectedSnapshot: snapshot,
        expectedProvider: provider,
        observation,
        currentSnapshot,
        nowMs
      })).toBeNull()
    }
    expect(providerAccountObservationStateIsCurrent(state!, snapshot, Date.parse(observation.expiresAt))).toBe(false)
  })

  it('repairs legacy primary routes into an ordered available local route draft', () => {
    const primary = {
      id: 'provider-primary',
      kind: 'openai-compatible',
      endpoint: 'https://provider-primary.invalid/v1',
      models: ['model-primary'],
      mediaModels: ['media-primary'],
      selectedModel: 'model-primary',
      selectedMediaModel: 'removed-media',
      selectedRoutes: ['primary', 'provider-fallback', 'provider-missing'],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '3',
      generation: '2',
      incarnation: `inc_${'a'.repeat(43)}`,
      tombstone: false
    } satisfies ProviderRegistryPublicProvider
    const fallback = {
      ...primary,
      id: 'provider-fallback',
      endpoint: 'https://provider-fallback.invalid/v1',
      selectedRoutes: []
    } satisfies ProviderRegistryPublicProvider

    expect(providerRegistryRoutingDraft(primary, [primary, fallback])).toEqual({
      selectedModel: 'model-primary',
      selectedMediaModel: '',
      routeProviderIds: ['provider-primary', 'provider-fallback']
    })
    expect(providerRegistrySelectedRoutes(primary.id, ['provider-fallback', primary.id]))
      .toEqual(['provider:provider-fallback', 'provider:provider-primary'])
  })

  it('round-trips an exact Provider ID primary without colliding with the legacy self alias', () => {
    const policy = {
      id: 'provider-policy',
      kind: 'openai-compatible',
      endpoint: 'https://provider-policy.invalid/v1',
      models: ['model-policy'],
      mediaModels: [],
      selectedModel: 'model-policy',
      selectedRoutes: ['provider:primary', 'provider:provider-policy'],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '3',
      generation: '2',
      incarnation: `inc_${'a'.repeat(43)}`,
      tombstone: false
    } satisfies ProviderRegistryPublicProvider
    const exactPrimary = {
      ...policy,
      id: 'primary',
      endpoint: 'https://exact-primary.invalid/v1',
      selectedRoutes: []
    } satisfies ProviderRegistryPublicProvider

    expect(providerRegistryRoutingDraft(policy, [policy, exactPrimary]).routeProviderIds)
      .toEqual(['primary', 'provider-policy'])
    expect(providerRegistrySelectedRoutes(policy.id, ['primary', policy.id]))
      .toEqual(['provider:primary', 'provider:provider-policy'])
    expect(providerRegistryRoutingDraft({ ...policy, selectedRoutes: ['primary'] }, [policy, exactPrimary])
      .routeProviderIds).toEqual(['provider-policy'])
  })

  it('omits a credential-configured model-disabled Provider and repairs its policy to a usable fallback', () => {
    const usable = {
      id: 'provider-usable',
      kind: 'openai-compatible',
      endpoint: 'https://provider-usable.invalid/v1',
      models: ['model-usable'],
      mediaModels: [],
      selectedModel: 'model-usable',
      selectedRoutes: [],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '3',
      generation: '2',
      incarnation: `inc_${'a'.repeat(43)}`,
      tombstone: false
    } satisfies ProviderRegistryPublicProvider
    const disabled = {
      ...usable,
      id: 'provider-disabled',
      endpoint: 'https://provider-disabled.invalid/v1',
      selectedModel: undefined,
      selectedRoutes: ['provider:provider-usable']
    } satisfies ProviderRegistryPublicProvider
    const policy = {
      ...usable,
      id: 'provider-policy',
      endpoint: 'https://provider-policy.invalid/v1',
      selectedRoutes: ['provider:provider-disabled', 'provider:provider-usable']
    } satisfies ProviderRegistryPublicProvider

    expect(providerRegistryRoutingDraft(policy, [policy, disabled, usable]).routeProviderIds)
      .toEqual(['provider-usable'])
    expect(providerRegistryRoutingDraft(disabled, [disabled, usable])).toEqual({
      selectedModel: '',
      selectedMediaModel: '',
      routeProviderIds: ['provider-usable']
    })
  })

  it('commits an empty disabled terminal and restores self only for a non-empty selection', () => {
    const policyId = 'provider-policy'
    const enabled = {
      selectedModel: 'model-policy',
      selectedMediaModel: '',
      routeProviderIds: [policyId]
    }
    const disabled = providerRegistryRoutingDraftWithSelectedModel(enabled, policyId, '')

    expect(disabled).toEqual({
      selectedModel: '',
      selectedMediaModel: '',
      routeProviderIds: []
    })
    expect(providerRegistrySelectedRoutes(policyId, disabled.routeProviderIds)).toEqual([])

    const restored = providerRegistryRoutingDraftWithSelectedModel(disabled, policyId, 'model-policy')
    expect(restored.routeProviderIds).toEqual([policyId])
    expect(providerRegistrySelectedRoutes(policyId, restored.routeProviderIds))
      .toEqual(['provider:provider-policy'])

    expect(providerRegistryRoutingDraftWithSelectedModel({
      ...enabled,
      routeProviderIds: [policyId, 'provider-fallback']
    }, policyId, '').routeProviderIds).toEqual(['provider-fallback'])

    const disabledExternalRoute = {
      ...disabled,
      routeProviderIds: ['provider-fallback']
    }
    expect(providerRegistryRoutingDraftWithToggledProvider(disabledExternalRoute, 'provider-fallback')
      .routeProviderIds).toEqual([])
    expect(providerRegistryRoutingDraftWithToggledProvider(enabled, policyId)).toEqual(enabled)
  })
})
