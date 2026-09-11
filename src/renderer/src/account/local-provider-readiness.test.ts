import { describe, expect, it } from 'vitest'
import type { ProviderRegistryResult } from '@shared/analytix-api'
import { resolveLocalProviderReadiness } from './local-provider-readiness'

const REGISTRY_INCARNATION = `inc_${'a'.repeat(43)}`
const PROVIDER_INCARNATION = `inc_${'b'.repeat(43)}`

function snapshot(
  providers: Extract<ProviderRegistryResult, { providers: unknown[] }>['providers'],
  selectedProviderId?: string
): ProviderRegistryResult {
  return {
    schemaVersion: 1,
    registryRevision: '1',
    registryIncarnation: REGISTRY_INCARNATION,
    ...(selectedProviderId ? { selectedProviderId } : {}),
    providers
  }
}

function provider(patch: Record<string, unknown> = {}) {
  return {
    id: 'deepseek',
    kind: 'deepseek',
    endpoint: 'https://api.deepseek.com',
    models: ['deepseek-chat'],
    mediaModels: [],
    selectedModel: 'deepseek-chat',
    selectedRoutes: [],
    credentialConfigured: true,
    credentialPurpose: 'provider-api-key',
    revision: '1',
    generation: '1',
    incarnation: PROVIDER_INCARNATION,
    tombstone: false,
    ...patch
  }
}

describe('local Provider startup readiness', () => {
  it('routes a fresh empty profile to protected local setup', () => {
    expect(resolveLocalProviderReadiness(snapshot([]))).toEqual({ kind: 'setup' })
  })

  it('enters the workspace only for the selected committed usable Provider', () => {
    expect(resolveLocalProviderReadiness(snapshot([provider()], 'deepseek'))).toEqual({
      kind: 'ready',
      providerId: 'deepseek'
    })
  })

  it.each([
    snapshot([], 'stale-provider'),
    snapshot([provider()], 'stale-provider'),
    snapshot([provider({ credentialConfigured: false })], 'deepseek'),
    snapshot([provider({ tombstone: true })], 'deepseek'),
    { schemaVersion: 1, error: { code: 'runtime_unavailable', message: 'private path detail' } }
  ] as ProviderRegistryResult[])('fails closed with redacted local recovery guidance for unusable state', (result) => {
    const readiness = resolveLocalProviderReadiness(result)
    expect(readiness.kind).toBe('recovery')
    expect(JSON.stringify(readiness)).toContain('Settings')
    expect(JSON.stringify(readiness)).not.toContain('private path detail')
  })
})
