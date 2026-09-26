import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  providerRegistrySnapshotResponseSchemaV1,
  type ProviderRegistryPublicProviderV1,
  type ProviderRegistryResultV1
} from '../../packages/runtime/src/contracts/provider-registry'
import { createProviderRegistryIpcHandler } from './ipc/provider-registry-ipc'
import { fetchUpstreamModelIds } from './upstream-models'

function publicProvider(overrides: Partial<ProviderRegistryPublicProviderV1> = {}): ProviderRegistryPublicProviderV1 {
  return {
    id: 'deepseek',
    kind: 'openai-compatible',
    endpoint: 'https://provider.invalid/v1',
    models: ['deepseek-v4-flash', 'deepseek-v4-pro'],
    mediaModels: [],
    selectedModel: 'deepseek-v4-pro',
    selectedRoutes: ['primary'],
    credentialConfigured: true,
    credentialPurpose: 'provider-api-key',
    revision: '2',
    generation: '1',
    incarnation: `inc_${'b'.repeat(43)}`,
    tombstone: false,
    ...overrides
  }
}

function snapshot(providers = [publicProvider()], selectedProviderId: string | undefined = providers.find((provider) => !provider.tombstone)?.id) {
  return providerRegistrySnapshotResponseSchemaV1.parse({
    schemaVersion: 1,
    registryRevision: '3',
    registryIncarnation: `inc_${'a'.repeat(43)}`,
    ...(selectedProviderId ? { selectedProviderId } : {}),
    providers
  })
}

describe('Core Registry composer model catalog', () => {
  afterEach(() => vi.unstubAllGlobals())

  it.each([['https://api.deepseek.com', 'official-deepseek'], ['http://127.0.0.1:5500', 'local'], ['https://gateway.test', 'remote']])('projects stable display labels without changing request IDs at %s', async (endpoint, endpointKind) => {
    const registry = snapshot([publicProvider({ endpoint })])
    const before = structuredClone(registry)
    const result = await fetchUpstreamModelIds(registry)
    expect(result).toEqual({
      ok: true,
      modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{
        providerId: 'deepseek', label: 'DeepSeek', endpointKind,
        modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro'],
        modelLabels: { 'deepseek-v4-flash': 'V4.1 Flash', 'deepseek-v4-pro': 'V4 Pro' }
      }]
    })
    expect(registry).toEqual(before)
    expect(JSON.stringify(result)).not.toMatch(/credential|"endpoint":|incarnation|https?:\/\//)
  })

  it('accepts committed default model IDs through the existing non-secret Registry list transport', async () => {
    const registry = snapshot()
    const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(registry) }))
    const listRegistry = createProviderRegistryIpcHandler(runtimeRequest)
    const result = await fetchUpstreamModelIds(await listRegistry({ schemaVersion: 1, operation: 'list' }))

    expect(runtimeRequest).toHaveBeenCalledExactlyOnceWith('/v1/provider-registry', 'GET')
    expect(result).toEqual({
      ok: true,
      modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro'],
      defaultModelId: 'deepseek-v4-pro',
      modelGroups: [{ providerId: 'deepseek', label: 'DeepSeek', endpointKind: 'remote', modelIds: ['deepseek-v4-flash', 'deepseek-v4-pro'],
        modelLabels: { 'deepseek-v4-flash': 'V4.1 Flash', 'deepseek-v4-pro': 'V4 Pro' } }]
    })
    expect(JSON.stringify(result)).not.toMatch(/credential|"endpoint":|incarnation|https?:\/\//)
  })

  it('does not manufacture default providers, models, or capabilities for an empty Registry', async () => {
    expect(await fetchUpstreamModelIds(snapshot([]))).toEqual({ ok: true, modelIds: [], modelGroups: [] })
  })

  it('returns a closed failure when the Registry transport is unavailable without exposing its body', async () => {
    const canary = 'PRIVATE_REGISTRY_ERROR_CANARY'
    const listRegistry = createProviderRegistryIpcHandler(vi.fn(async () => ({ ok: false, status: 503, body: canary })))
    const result = await fetchUpstreamModelIds(await listRegistry({ schemaVersion: 1, operation: 'list' }))

    expect(result).toEqual({ ok: false, message: 'Provider model catalog is unavailable.' })
    expect(JSON.stringify(result)).not.toContain(canary)
  })

  it('rejects malformed Registry snapshots and legacy settings without a model fallback', async () => {
    const malformed = { ...snapshot(), providers: [{ ...publicProvider(), selectedModel: 'not-committed' }] }
    const legacy = { provider: { providers: [{ id: 'legacy', models: ['legacy-model'] }] }, runtime: { model: 'legacy-model' } }
    for (const value of [malformed, legacy]) {
      expect(await fetchUpstreamModelIds(value as unknown as ProviderRegistryResultV1)).toEqual({
        ok: false,
        message: 'Provider model catalog is unavailable.'
      })
    }
  })

  it.each([
    { name: 'missing credential', provider: publicProvider({ credentialConfigured: false, credentialPurpose: undefined }) },
    { name: 'disconnected provider', provider: publicProvider({ tombstone: true, credentialConfigured: false, credentialPurpose: undefined, selectedRoutes: [] }) }
  ])('does not offer models from a $name', async ({ provider }) => {
    expect(await fetchUpstreamModelIds(snapshot([provider]))).toEqual({ ok: true, modelIds: [], modelGroups: [] })
  })

  it('uses the Registry selected provider and model when model IDs overlap across providers', async () => {
    const registry = snapshot([
      publicProvider({ id: 'alpha', models: ['shared-model'], selectedModel: 'shared-model', selectedRoutes: [] }),
      publicProvider({ id: 'beta', models: ['beta-model', 'shared-model'], selectedModel: 'shared-model' })
    ], 'beta')
    const result = await fetchUpstreamModelIds(registry)

    expect(result).toEqual({
      ok: true,
      modelIds: ['beta-model', 'shared-model'],
      defaultModelId: 'shared-model',
      modelGroups: [
        { providerId: 'beta', label: 'beta', endpointKind: 'remote', modelIds: ['beta-model', 'shared-model'] },
        { providerId: 'alpha', label: 'alpha', endpointKind: 'remote', modelIds: ['shared-model'] }
      ]
    })
  })

  it('uses only committed model IDs and never queries upstream for custom full endpoints', async () => {
    const fetch = vi.fn()
    vi.stubGlobal('fetch', fetch)
    const registry = snapshot([publicProvider({
      kind: 'custom-endpoint',
      endpoint: 'https://provider.invalid/custom-path',
      models: ['custom-model'],
      selectedModel: 'custom-model'
    })])
    const before = structuredClone(registry)
    const result = await fetchUpstreamModelIds(registry)

    expect(result).toEqual({
      ok: true,
      modelIds: ['custom-model'],
      defaultModelId: 'custom-model',
      modelGroups: [{ providerId: 'deepseek', label: 'DeepSeek', endpointKind: 'remote', modelIds: ['custom-model'] }]
    })
    expect(registry).toEqual(before)
    expect(fetch).not.toHaveBeenCalled()
  })

  it('excludes declared media and recognized non-chat models without inventing capability profiles', async () => {
    const result = await fetchUpstreamModelIds(snapshot([publicProvider({
      models: ['banana-canvas', 'custom-chat', 'text-embedding-3-small', 'whisper-1'],
      mediaModels: ['banana-canvas'],
      selectedModel: 'custom-chat'
    })]))

    expect(result).toEqual({
      ok: true,
      modelIds: ['custom-chat'],
      defaultModelId: 'custom-chat',
      modelGroups: [{ providerId: 'deepseek', label: 'DeepSeek', endpointKind: 'remote', modelIds: ['custom-chat'] }]
    })
  })

  it('does not substitute a default chat model for a media-only Registry provider', async () => {
    const result = await fetchUpstreamModelIds(snapshot([publicProvider({
      models: ['banana-canvas'], mediaModels: ['banana-canvas'], selectedModel: 'banana-canvas'
    })]))

    expect(result).toEqual({ ok: true, modelIds: [], modelGroups: [] })
  })
})
