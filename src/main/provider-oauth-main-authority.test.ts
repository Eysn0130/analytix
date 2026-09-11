import { describe, expect, it, vi } from 'vitest'
import {
  createMainOAuthAccountAuthority,
  providerRegistryInputWithOAuthBinding,
  providerOAuthOwnerFenceFromProjection
} from './provider-oauth-main-authority'
import {
  oauthAuthorizationBindingsEqual,
  type OAuthAuthorizationBindingV1
} from './provider-oauth-lifecycle'

const profileBinding = 'b'.repeat(64)
const binding: OAuthAuthorizationBindingV1 = {
  owner: 'provider',
  provider: 'provider-a',
  accountId: 'provider-a',
  issuer: 'https://issuer.example.test/',
  authorizationEndpoint: 'https://login.example.test/authorize',
  tokenEndpoint: 'https://tokens.example.test/token',
  revocationEndpoint: 'https://tokens.example.test/revoke',
  clientId: 'public-client',
  scopes: ['openid'],
  bindingKey: `oauthb_${'a'.repeat(43)}`,
  redirectUri: `com.analytix.desktop:/oauth/callback/oauthb_${'a'.repeat(43)}`,
  ownerFingerprint: 'c'.repeat(64),
  ownerRevision: '4',
  ownerGeneration: '7',
  ownerIncarnation: `inc_${'d'.repeat(43)}`,
  profileBinding
}

function lifecycle(overrides: Record<string, unknown> = {}) {
  return {
    begin: vi.fn(async () => ({
      authorizationId: 'oauth_abcdefghijklmnopqrstuvwxyz',
      authorizationUrl: 'https://login.example.test/authorize?state=must-stay-main-private',
      expiresAtMs: Date.parse('2026-08-30T10:10:00.000Z')
    })),
    authorizationStatus: vi.fn(),
    cancelAuthorization: vi.fn(async () => ({ cancelled: true as const })),
    replaceSubscription: vi.fn(),
    revoke: vi.fn(),
    delete: vi.fn(),
    completeNativeCallback: vi.fn(),
    ...overrides
  }
}

describe('Main OAuth account authority', () => {
  it('preserves an exact Registry-owned account observation binding during OAuth binding update', () => {
    const accountObservation = {
      schemaVersion: 1 as const,
      endpoint: 'https://quota.provider.invalid/v1/observation',
      method: 'GET' as const,
      projection: 'normalized-quota-v1' as const
    }
    const input = providerRegistryInputWithOAuthBinding({
      id: 'provider-a',
      kind: 'openai-compatible',
      endpoint: 'https://provider.invalid/v1',
      models: ['model-a'],
      mediaModels: [],
      selectedModel: 'model-a',
      selectedRoutes: ['primary'],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '4',
      generation: '7',
      incarnation: `inc_${'e'.repeat(43)}`,
      tombstone: false,
      accountObservation
    }, {
      schemaVersion: 1,
      issuer: binding.issuer,
      authorizationEndpoint: binding.authorizationEndpoint,
      tokenEndpoint: binding.tokenEndpoint,
      revocationEndpoint: binding.revocationEndpoint,
      clientId: binding.clientId,
      scopes: binding.scopes,
      redirectModeVersion: 1
    })
    expect(input.accountObservation).toEqual(accountObservation)
    expect(input.oauthBinding).toMatchObject({ issuer: binding.issuer, clientId: binding.clientId })
    expect(JSON.stringify(input)).not.toMatch(/credentialRef|valueBase64|apiKey/)
  })

  it('uses the actual K2 Provider projection as the OAuth owner fence', () => {
    expect(providerOAuthOwnerFenceFromProjection({
      revision: '41', generation: '17', incarnation: `inc_${'e'.repeat(43)}`
    })).toEqual({
      ownerRevision: '41', ownerGeneration: '17', ownerIncarnation: `inc_${'e'.repeat(43)}`
    })
  })

  it('compares complete OAuth bindings independently of object insertion order', () => {
    const reordered = {
      profileBinding: binding.profileBinding,
      ownerIncarnation: binding.ownerIncarnation,
      ownerGeneration: binding.ownerGeneration,
      ownerRevision: binding.ownerRevision,
      ownerFingerprint: binding.ownerFingerprint,
      redirectUri: binding.redirectUri,
      bindingKey: binding.bindingKey,
      scopes: binding.scopes,
      accountId: binding.accountId,
      provider: binding.provider,
      clientId: binding.clientId,
      revocationEndpoint: binding.revocationEndpoint,
      tokenEndpoint: binding.tokenEndpoint,
      authorizationEndpoint: binding.authorizationEndpoint,
      issuer: binding.issuer,
      owner: binding.owner
    } satisfies OAuthAuthorizationBindingV1
    expect(oauthAuthorizationBindingsEqual(binding, reordered)).toBe(true)
    expect(oauthAuthorizationBindingsEqual(binding, {
      ...reordered,
      ownerGeneration: '8'
    })).toBe(false)
  })

  it('opens the protected authorization URL only in Main and returns a redacted status', async () => {
    const lifecycleMock = lifecycle()
    const openExternal = vi.fn(async () => undefined)
    const resolveBinding = vi.fn(async () => binding)
    const authority = createMainOAuthAccountAuthority({
      lifecycle: lifecycleMock as never,
      profileBinding,
      resolveBinding,
      openExternal,
      configureProviderBinding: vi.fn()
    })
    const result = await authority.beginProvider({ providerId: 'provider-a' }, { webContentsId: 41 })
    expect(resolveBinding).toHaveBeenCalledWith({ owner: 'provider', providerId: 'provider-a' })
    expect(lifecycleMock.begin).toHaveBeenCalledWith(binding, { webContentsId: 41 })
    expect(openExternal).toHaveBeenCalledWith(
      'https://login.example.test/authorize?state=must-stay-main-private'
    )
    expect(result).toEqual({
      ok: true,
      authorizationId: 'oauth_abcdefghijklmnopqrstuvwxyz',
      expiresAt: '2026-08-30T10:10:00.000Z',
      phase: 'pending'
    })
    expect(JSON.stringify(result)).not.toMatch(/authorizationUrl|state|nonce|verifier|token|credentialRef/i)
  })

  it('CAS-cleans the protected pending authorization when external-open fails', async () => {
    const lifecycleMock = lifecycle()
    const authority = createMainOAuthAccountAuthority({
      lifecycle: lifecycleMock as never,
      profileBinding,
      resolveBinding: async () => binding,
      openExternal: vi.fn(async () => { throw new Error('synthetic-open-failure') }),
      configureProviderBinding: vi.fn()
    })
    await expect(authority.beginProvider({ providerId: 'provider-a' }, { webContentsId: 41 }))
      .resolves.toEqual({
        ok: false,
        code: 'unavailable',
        message: 'OAuth account management is unavailable.'
      })
    expect(lifecycleMock.cancelAuthorization).toHaveBeenCalledWith({
      authorizationId: 'oauth_abcdefghijklmnopqrstuvwxyz',
      profileBinding,
      webContentsId: 41,
      expectedOwner: 'provider'
    })
  })
})
