import { describe, expect, it } from 'vitest'
import {
  PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1,
  providerRegistryAccountObservationResponseSchemaV1,
  providerRegistryDecimalSchemaV1,
  providerRegistryFailureSchemaV1,
  providerRegistryPortableManifestImportRequestSchemaV1,
  providerRegistryPortableManifestImportResponseSchemaV1,
  providerRegistryProviderInputSchemaV1,
  providerRegistryProbeResponseSchemaV1,
  parseProviderRegistryPortableManifestV1,
  providerRegistryPublicProviderSchemaV1,
  providerRegistryRequestSchemaV1
} from './provider-registry.js'

const registryIncarnation = `inc_${'a'.repeat(43)}`
const providerIncarnation = `inc_${'b'.repeat(43)}`
const expected = {
  registryRevision: '4',
  registryIncarnation,
  providerRevision: '7',
  providerGeneration: '3',
  providerIncarnation,
  providerCredentialPurpose: 'provider-api-key'
}
const provider = {
  id: 'provider-alpha',
  kind: 'openai-compatible',
  endpoint: 'https://provider.invalid/v1',
  proxy: '',
  models: ['model-alpha'],
  mediaModels: [],
  selectedModel: 'model-alpha',
  selectedMediaModel: '',
  selectedRoutes: ['primary']
}

describe('Provider Registry public contract', () => {
  it('validates the strict canonical key-free portable manifest and rejects unsafe variants', () => {
    const manifest = {
      schema: 'analytix.provider-portable-manifest/v1',
      providers: [{
        correlation: 'provider-0',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        models: ['model-secret-token-metadata', 'model<alpha>&'],
        mediaModels: [],
        oauthBinding: {
          schemaVersion: 1,
          issuer: 'https://issuer.example/oauth',
          authorizationEndpoint: 'https://issuer.example/oauth/authorize',
          tokenEndpoint: 'https://issuer.example/oauth/token',
          clientId: 'public-client/alpha<>&',
          scopes: ['https://www.googleapis.com/auth/drive.readonly', 'scope-secret-token'],
          redirectModeVersion: 1
        },
        accountObservation: {
          schemaVersion: 1,
          endpoint: 'https://billing.example/v1/quota',
          method: 'GET',
          projection: 'normalized-quota-v1'
        },
        routes: [],
        intent: 'reentry_required'
      }],
      accounts: []
    } as const
    const canonical = JSON.stringify(manifest)
    expect(parseProviderRegistryPortableManifestV1(canonical)).toEqual(manifest)
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"schema":"analytix.provider-portable-manifest/v1"',
        '"schema":"analytix.provider-portable-manifest/v1","schema":"analytix.provider-portable-manifest/v1"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"routes":[]', '"routes":[],"credentialRef":"synthetic-portable-secret"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"endpoint":"https://provider.invalid/v1"', '"endpoint":"https://provider.invalid/v1?secret=1"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"accounts":[]', '"accounts":[{"correlation":"account-0","owner":"mcp","provider":"server-a","endpoint":"https://provider.invalid/v1","purpose":"mcp-oauth-access-token","intent":"reentry_required","proxy":""}]')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"routes":[]', '"routes":["provider-unknown"]')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"schema":"analytix.provider-portable-manifest/v1"', '"schema":"analytix.provider-portable-manifest/v0"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"correlation":"provider-0"', '"correlation":"provider-alpha"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"clientId":"public-client/alpha<>&"', '"clientId":"client\u2028id"')
    )).toBeNull()
    expect(parseProviderRegistryPortableManifestV1(
      canonical.replace('"clientId":"public-client/alpha<>&"', '"clientId":"client\u2029id"')
    )).toBeNull()
    for (const clientId of ['../client', '/absolute', '~/client', '..\\client', 'client\u0000id', 'c'.repeat(257)]) {
      expect(parseProviderRegistryPortableManifestV1(
        canonical.replace('"clientId":"public-client/alpha<>&"', `"clientId":${JSON.stringify(clientId)}`)
      )).toBeNull()
    }
    for (const scope of ['../scope', '/scope', '~/scope', '..\\scope', 'scope\u0000id', 's'.repeat(257)]) {
      expect(parseProviderRegistryPortableManifestV1(
        canonical.replace('"scopes":["https://www.googleapis.com/auth/drive.readonly","scope-secret-token"]', `"scopes":[${JSON.stringify(scope)}]`)
      )).toBeNull()
    }
    const overCapacity = {
      schema: 'analytix.provider-portable-manifest/v1',
      providers: Array.from({ length: 256 }, (_, index) => ({
        correlation: `provider-${index}`,
        kind: 'openai-compatible',
        endpoint: `https://provider-${String(index).padStart(3, '0')}.invalid/v1`,
        models: [], mediaModels: [], routes: [], intent: 'reentry_required'
      })),
      accounts: [{
        correlation: 'account-0', owner: 'mcp', provider: 'server-a',
        endpoint: 'https://account.invalid/v1', purpose: 'mcp-oauth-access-token', intent: 'reentry_required'
      }]
    }
    expect(parseProviderRegistryPortableManifestV1(JSON.stringify(overCapacity))).toBeNull()
  })

  it('rejects duplicate destination identities and non-canonical import requests', () => {
    const duplicateDestinationResponse = {
      schemaVersion: 1,
      providerCount: 2,
      accountCount: 0,
      reentryRequired: 2,
      entries: [
        { correlation: 'provider-0', destinationProviderId: 'provider-a', status: 'reentry_required' },
        { correlation: 'provider-1', destinationProviderId: 'provider-a', status: 'reentry_required' }
      ]
    }
    expect(providerRegistryPortableManifestImportResponseSchemaV1.safeParse(duplicateDestinationResponse).success)
      .toBe(false)

    const malformedManifestJson = '{"schema":"analytix.provider-portable-manifest/v1","schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[]}'
    expect(providerRegistryPortableManifestImportRequestSchemaV1.safeParse({
      schemaVersion: 1,
      operation: 'import-portable-manifest',
      manifestJson: malformedManifestJson
    }).success).toBe(false)
  })

  it('accepts only canonical uint64 decimals and opaque incarnations', () => {
    expect(providerRegistryDecimalSchemaV1.parse(PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1))
      .toBe(PROVIDER_REGISTRY_MAX_UINT64_DECIMAL_V1)
    for (const value of ['', '00', '01', '-1', '1.0', '18446744073709551616']) {
      expect(providerRegistryDecimalSchemaV1.safeParse(value).success).toBe(false)
    }
    expect(providerRegistryRequestSchemaV1.safeParse({
      schemaVersion: 1,
      operation: 'select',
      providerId: provider.id,
      expected: { ...expected, providerIncarnation: 'inc_not-opaque' }
    }).success).toBe(false)
  })

  it('rejects URL source forms normalized differently from the Go Registry owner', () => {
    const invalidEndpointSources = [
      'https://provider.invalid/v1?',
      'https://provider.invalid/v1?mode=test',
      'https://provider.invalid/v1#fragment',
      'https://@provider.invalid/v1',
      String.raw`https:\\provider.invalid\v1`,
      String.raw`https://provider.invalid\v1`,
      'https:provider.invalid/v1'
    ]
    const validEndpointSources = [
      'HTTPS://provider.invalid/v1',
      'https://provider.invalid/v1#',
      String.raw`https://provider.invalid/v1\literal`
    ]
    const updateRequest = {
      schemaVersion: 1,
      operation: 'update',
      providerId: provider.id,
      expected,
      provider,
      credential: { kind: 'keep' }
    }
    const publicProjection = {
      id: provider.id,
      kind: provider.kind,
      endpoint: provider.endpoint,
      models: provider.models,
      mediaModels: provider.mediaModels,
      selectedModel: provider.selectedModel,
      selectedRoutes: provider.selectedRoutes,
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '7',
      generation: '3',
      incarnation: providerIncarnation,
      tombstone: false
    }

    for (const endpoint of invalidEndpointSources) {
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...updateRequest,
        provider: { ...provider, endpoint }
      }).success).toBe(false)
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...updateRequest,
        provider: { ...provider, proxy: endpoint }
      }).success).toBe(false)
      expect(providerRegistryPublicProviderSchemaV1.safeParse({
        ...publicProjection,
        endpoint
      }).success).toBe(false)
      expect(providerRegistryPublicProviderSchemaV1.safeParse({
        ...publicProjection,
        proxy: endpoint
      }).success).toBe(false)
    }

    for (const endpoint of validEndpointSources) {
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...updateRequest,
        provider: { ...provider, endpoint }
      }).success).toBe(true)
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...updateRequest,
        provider: { ...provider, proxy: endpoint }
      }).success).toBe(true)
      expect(providerRegistryPublicProviderSchemaV1.safeParse({
        ...publicProjection,
        endpoint
      }).success).toBe(true)
      expect(providerRegistryPublicProviderSchemaV1.safeParse({
        ...publicProjection,
        proxy: endpoint
      }).success).toBe(true)
    }
  })

  it('keeps credentials write-only and rejects ambiguous credential mutations', () => {
    const update = {
      schemaVersion: 1,
      operation: 'update',
      providerId: provider.id,
      expected,
      provider,
      credential: { kind: 'keep' }
    }
    expect(providerRegistryRequestSchemaV1.safeParse(update).success).toBe(true)
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      credential: { kind: 'keep', purpose: 'provider-api-key' }
    }).success).toBe(false)
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      credential: { kind: 'keep', valueBase64: 'c2VjcmV0' }
    }).success).toBe(false)
    for (const valueBase64 of ['', 'c2VjcmV', 'c2VjcmV0===', 'cmVkYWN0ZWQ=']) {
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...update,
        credential: { kind: 'set', purpose: 'provider-api-key', valueBase64 }
      }).success).toBe(false)
    }
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: 'c3ludGhldGljLXNlY3JldA==' }
    }).success).toBe(true)
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      unexpected: true
    }).success).toBe(false)
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      operation: 'unknown'
    }).success).toBe(false)
  })

  it('accepts collision-safe exact Provider routes and rejects malformed or ambiguous encodings', () => {
    const update = {
      schemaVersion: 1,
      operation: 'update',
      providerId: provider.id,
      expected,
      provider,
      credential: { kind: 'keep' }
    }
    expect(providerRegistryRequestSchemaV1.safeParse({
      ...update,
      provider: { ...provider, selectedRoutes: ['primary', 'provider-fallback'] }
    }).success).toBe(true)
    const exactRoutes = ['provider:primary', 'provider:provider-fallback']
    const exact = providerRegistryRequestSchemaV1.parse({
      ...update,
      provider: { ...provider, selectedRoutes: exactRoutes }
    })
    expect('provider' in exact && exact.provider.selectedRoutes).toEqual(exactRoutes)
    for (const selectedRoutes of [
      ['Provider-Fallback'],
      ['provider fallback'],
      ['provider/fallback'],
      ['provider:'],
      ['provider:Provider-Fallback'],
      ['provider:provider:fallback'],
      ['primary', `provider:${provider.id}`]
    ]) {
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...update,
        provider: { ...provider, selectedRoutes }
      }).success).toBe(false)
    }
  })

  it('accepts a fixed fenced probe and rejects renderer execution authority', () => {
    const fixedProbe = {
      schemaVersion: 1,
      operation: 'probe',
      providerId: provider.id,
      expected
    }
    expect(providerRegistryRequestSchemaV1.safeParse(fixedProbe).success).toBe(true)
    for (const rendererAuthority of [
      { baseUrl: provider.endpoint },
      { endpoint: provider.endpoint },
      { endpointFormat: 'chat_completions' },
      { apiKey: 'synthetic-renderer-secret' },
      { credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: 'c3ludGhldGljLXNlY3JldA==' } }
    ]) {
      expect(providerRegistryRequestSchemaV1.safeParse({
        ...fixedProbe,
        ...rendererAuthority
      }).success).toBe(false)
    }
  })

  it('accepts only coherent closed probe status, code, and model-count tuples', () => {
    const response = {
      schemaVersion: 1,
      registryRevision: expected.registryRevision,
      registryIncarnation: expected.registryIncarnation,
      providerId: provider.id,
      providerRevision: expected.providerRevision,
      providerGeneration: expected.providerGeneration,
      providerIncarnation: expected.providerIncarnation,
      status: 'reachable',
      code: 200,
      modelCount: 1,
      latencyMs: 12
    }
    for (const valid of [
      response,
      { ...response, status: 'auth_failed', code: 401, modelCount: 0 },
      { ...response, status: 'auth_failed', code: 403, modelCount: 0 },
      { ...response, status: 'redirect_blocked', code: 307, modelCount: 0 },
      { ...response, status: 'provider_error', code: 429, modelCount: 0 },
      { ...response, status: 'timeout', code: undefined, modelCount: 0 },
      { ...response, status: 'unavailable', code: undefined, modelCount: 0 },
      { ...response, status: 'invalid_response', code: 200, modelCount: 0 },
      { ...response, status: 'invalid_response', code: undefined, modelCount: 0 }
    ]) {
      expect(providerRegistryProbeResponseSchemaV1.safeParse(valid).success).toBe(true)
    }
    for (const invalid of [
      { ...response, code: undefined },
      { ...response, code: 300 },
      { ...response, status: 'auth_failed', code: 402, modelCount: 0 },
      { ...response, status: 'auth_failed', code: 401, modelCount: 1 },
      { ...response, status: 'redirect_blocked', code: 200, modelCount: 0 },
      { ...response, status: 'provider_error', code: 307, modelCount: 0 },
      { ...response, status: 'provider_error', code: 401, modelCount: 0 },
      { ...response, status: 'timeout', code: 504, modelCount: 0 },
      { ...response, status: 'unavailable', code: undefined, modelCount: 1 },
      { ...response, status: 'invalid_response', code: 401, modelCount: 0 },
      { ...response, status: 'invalid_response', code: 200, modelCount: 1 }
    ]) {
      expect(providerRegistryProbeResponseSchemaV1.safeParse(invalid).success).toBe(false)
    }
  })

  it('binds account observation to exact key-free Registry metadata and a closed public tuple', () => {
    const accountObservation = {
      schemaVersion: 1,
      endpoint: 'https://quota.provider.invalid/v1/observation',
      method: 'GET',
      projection: 'normalized-quota-v1'
    }
    expect(providerRegistryProviderInputSchemaV1.safeParse({ ...provider, accountObservation }).success).toBe(true)
    for (const invalid of [
      { ...accountObservation, endpoint: 'http://localhost/quota' },
      { ...accountObservation, endpoint: 'http://192.168.1.5/quota' },
      { ...accountObservation, endpoint: 'https://quota.provider.invalid/quota?account=one' },
      { ...accountObservation, endpoint: 'https://user@quota.provider.invalid/quota' },
      { ...accountObservation, method: 'POST' },
      { ...accountObservation, credentialRef: `cred_${'c'.repeat(43)}` }
    ]) {
      expect(providerRegistryProviderInputSchemaV1.safeParse({ ...provider, accountObservation: invalid }).success)
        .toBe(false)
    }

    const response = {
      schemaVersion: 1,
      registryRevision: expected.registryRevision,
      registryIncarnation: expected.registryIncarnation,
      providerId: provider.id,
      providerRevision: expected.providerRevision,
      providerGeneration: expected.providerGeneration,
      providerIncarnation: expected.providerIncarnation,
      providerCredentialPurpose: expected.providerCredentialPurpose,
      status: 'available',
      observedAt: '2026-08-31T00:00:00Z',
      expiresAt: '2026-08-31T00:05:00Z',
      quota: 100,
      usage: 40,
      remaining: 60
    }
    expect(providerRegistryAccountObservationResponseSchemaV1.safeParse(response).success).toBe(true)
    expect(providerRegistryAccountObservationResponseSchemaV1.safeParse({
      ...response,
      expiresAt: '2026-08-31T01:00:00Z'
    }).success).toBe(true)
    for (const invalid of [
      { ...response, status: 'available', quota: undefined, usage: undefined, remaining: undefined },
      { ...response, status: 'unavailable' },
      { ...response, providerCredentialPurpose: undefined },
      { ...response, remaining: -1 },
      { ...response, expiresAt: '2026-08-30T23:59:59Z' },
      { ...response, expiresAt: '2026-08-31T01:00:00.001Z' },
      { ...response, endpoint: accountObservation.endpoint },
      { ...response, credentialRef: `cred_${'d'.repeat(43)}` }
    ]) {
      expect(providerRegistryAccountObservationResponseSchemaV1.safeParse(invalid).success).toBe(false)
    }
  })

  it('rejects oversized credential input before it can become a set mutation', () => {
    const oversizedBase64 = `${'A'.repeat(1_398_100)}AAAA`
    expect(providerRegistryRequestSchemaV1.safeParse({
      schemaVersion: 1,
      operation: 'credential-replace',
      providerId: provider.id,
      expected,
      credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: oversizedBase64 }
    }).success).toBe(false)
  })

  it('accepts only strict key-free Provider projections', () => {
    const projection = {
      id: provider.id,
      kind: provider.kind,
      endpoint: provider.endpoint,
      models: provider.models,
      mediaModels: provider.mediaModels,
      selectedModel: provider.selectedModel,
      selectedRoutes: provider.selectedRoutes,
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '7',
      generation: '3',
      incarnation: providerIncarnation,
      tombstone: false
    }
    expect(providerRegistryPublicProviderSchemaV1.parse(projection)).toEqual(projection)
    for (const forbidden of [
      { credentialRef: `cred_${'c'.repeat(43)}` },
      { valueBase64: 'c3ludGhldGljLXNlY3JldA==' },
      { ciphertext: 'synthetic-ciphertext' },
      { secret: 'synthetic-secret' },
      { unexpected: true }
    ]) {
      expect(providerRegistryPublicProviderSchemaV1.safeParse({ ...projection, ...forbidden }).success)
        .toBe(false)
    }
    expect(providerRegistryPublicProviderSchemaV1.safeParse({ ...projection, revision: '07' }).success)
      .toBe(false)
    expect(providerRegistryPublicProviderSchemaV1.safeParse({
      ...projection,
      credentialConfigured: false
    }).success).toBe(false)
  })

  it('allows only canonical redacted failure messages', () => {
    expect(providerRegistryFailureSchemaV1.safeParse({
      schemaVersion: 1,
      error: { code: 'conflict', message: 'The provider registry state has changed.' }
    }).success).toBe(true)
    expect(providerRegistryFailureSchemaV1.safeParse({
      schemaVersion: 1,
      error: { code: 'conflict', message: 'conflict at /private/provider-registry' }
    }).success).toBe(false)
  })
})
