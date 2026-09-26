import { describe, expect, it, vi } from 'vitest'
import type { ProviderRegistryResult } from '@shared/analytix-api'
import { checkLocalProviderReadiness, resolveLocalProviderReadiness } from './local-provider-readiness'

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
      providerId: 'deepseek', model: provider().selectedModel
    })
  })

  it.each([
    snapshot([], 'stale-provider'),
    snapshot([provider()], 'stale-provider'),
    snapshot([provider({ credentialConfigured: false })], 'deepseek'),
    snapshot([provider({ selectedModel: '' })], 'deepseek'),
    snapshot([provider({ selectedModel: 'unconfigured-model' })], 'deepseek'),
    snapshot([provider({ tombstone: true })], 'deepseek'),
    { schemaVersion: 1, error: { code: 'runtime_unavailable', message: 'private path detail' } }
  ] as ProviderRegistryResult[])('fails closed with redacted local recovery guidance for unusable state', (result) => {
    const readiness = resolveLocalProviderReadiness(result)
    expect(readiness.kind).toBe('recovery')
    expect(JSON.stringify(readiness)).toContain('Settings')
    expect(JSON.stringify(readiness)).not.toContain('private path detail')
  })
})


it('checks the committed credential after restart without re-entering or sending it to a Provider', async () => {
  let locked = true
  const state = snapshot([provider()], 'deepseek')
  const original = JSON.stringify(state)
  const request = vi.fn(async (input: import('@shared/analytix-api').ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
    if (input.operation === 'list') return state
    if (input.operation !== 'credential-check') throw new Error('unexpected mutation or network probe')
    if (locked) return { schemaVersion: 1, error: { code: 'credential_unavailable', message: 'ignored private detail' } }
    return { schemaVersion: 1, providerId: input.providerId,
      registryRevision: input.expected.registryRevision, registryIncarnation: input.expected.registryIncarnation,
      providerRevision: input.expected.providerRevision, providerGeneration: input.expected.providerGeneration,
      providerIncarnation: input.expected.providerIncarnation, credentialAvailable: true }
  })
  expect((await checkLocalProviderReadiness(request)).kind).toBe('recovery')
  expect(JSON.stringify(state)).toBe(original)
  locked = false
  expect(await checkLocalProviderReadiness(request)).toEqual({ kind: 'ready', providerId: 'deepseek', model: provider().selectedModel })
  expect(request.mock.calls.map(([r]) => r.operation)).toEqual(['list', 'credential-check', 'list', 'credential-check'])
})

it('directs a legacy Keychain credential to Settings without exposing private details', async () => {
  const state = snapshot([provider()], 'deepseek')
  const request = vi.fn(async (input: import('@shared/analytix-api').ProviderRegistryRequest): Promise<ProviderRegistryResult> => {
    if (input.operation === 'list') return state
    if (input.operation !== 'credential-check') throw new Error('unexpected mutation or network probe')
    return { schemaVersion: 1, error: {
      code: 'credential_reentry_required', message: 'private detail must not be shown'
    } }
  })
  const readiness = await checkLocalProviderReadiness(request)
  expect(readiness.kind).toBe('recovery')
  expect(JSON.stringify(readiness)).toContain('re-entered once in Settings')
  expect(JSON.stringify(readiness)).not.toContain('private detail')
  expect(request.mock.calls.map(([r]) => r.operation)).toEqual(['list', 'credential-check'])
})
