import { describe, expect, it, vi } from 'vitest'
import {
  createAccountCredentialIpcHandler,
  createMainAccountCredentialResolver,
  createProviderRegistryIpcHandler,
  createProtectedRecoveryRuntimeClient
} from './provider-registry-ipc'
import { analytixProviderRegistryProviderPath } from '../../shared/analytix-endpoints'

const registryIncarnation = `inc_${'a'.repeat(43)}`
const providerIncarnation = `inc_${'b'.repeat(43)}`
const expectedExisting = {
  registryRevision: '4',
  registryIncarnation,
  providerRevision: '7',
  providerGeneration: '3',
  providerIncarnation,
  providerCredentialPurpose: 'provider-api-key'
}
const providerInput = {
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

function publicProvider(overrides: Record<string, unknown> = {}) {
  return {
    id: 'provider-alpha',
    kind: 'openai-compatible',
    endpoint: 'https://provider.invalid/v1',
    models: ['model-alpha'],
    mediaModels: [],
    selectedModel: 'model-alpha',
    selectedRoutes: ['primary'],
    credentialConfigured: true,
    credentialPurpose: 'provider-api-key',
    revision: '8',
    generation: '3',
    incarnation: providerIncarnation,
    tombstone: false,
    ...overrides
  }
}

function providerResponse(overrides: Record<string, unknown> = {}) {
  return {
    ok: true,
    status: 200,
    body: JSON.stringify({
      schemaVersion: 1,
      registryRevision: '5',
      registryIncarnation,
      provider: publicProvider(),
      ...overrides
    })
  }
}

describe('provider registry IPC', () => {
  it('keeps protected recovery on operation-specific private paths and returns only bounded artifacts', async () => {
    const requestBytes = Buffer.from('{"schema":"synthetic-request"}')
    const runtimeRequest = vi.fn(async (path: string, method?: string, body?: string) => {
      expect(path).toBe('/v1/provider-registry/_private/protected-recovery/prepare')
      expect(method).toBe('POST')
      const request = JSON.parse(body ?? '{}') as Record<string, unknown>
      expect(request).toHaveProperty('manifestBase64')
      expect(request).not.toHaveProperty('credentialRef')
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          requestBase64: requestBytes.toString('base64'),
          requestDigest: 'a'.repeat(64),
          requestFingerprint: 'b'.repeat(16),
          manifestDigest: 'c'.repeat(64),
          itemSetDigest: 'd'.repeat(64),
          operationId: 'protected-recovery-operation',
          sessionNonce: 'protected-recovery-session-nonce',
          expiresAt: '2026-09-01T00:00:00.000Z'
        })
      }
    })
    const client = createProtectedRecoveryRuntimeClient(runtimeRequest)
    const result = await client({
      operation: 'prepare-protected-recovery-request',
      importReceipt: {
        manifestJson: '{"schema":"analytix.provider-portable-manifest/v1","providers":[{"correlation":"provider-0","kind":"openai-compatible","endpoint":"https://portable-provider.invalid","models":[],"mediaModels":[],"routes":[],"intent":"reentry_required"}],"accounts":[]}',
        entries: [{
          correlation: 'provider-0', destinationProviderId: 'provider-a', status: 'reentry_required',
          fence: { revision: '1', generation: '1', incarnation: `inc_${'a'.repeat(43)}` }
        }]
      },
      localBinding: {
        browserWindowId: 'window', mainFrameId: 'frame', profileBinding: 'profile', dataDirectoryBinding: '/tmp/profile'
      },
      ownerBindingInventory: []
    })
    expect(result).toMatchObject({ ok: true, status: 'request-prepared' })
    expect((result as { artifactBytes: Uint8Array }).artifactBytes).toEqual(requestBytes)
    expect(JSON.stringify(result)).not.toContain('credentialRef')
  })

  it('preserves the exact ordinary-import fence in the private protected-recovery prepare body', async () => {
    const fence = {
      revision: '17',
      generation: '5',
      incarnation: `inc_${'f'.repeat(43)}`
    }
    let observedPath = ''
    let observedMethod = ''
    let observedBody = ''
    const runtimeRequest = vi.fn(async (path: string, method?: string, body?: string) => {
      observedPath = path
      observedMethod = method ?? ''
      observedBody = body ?? ''
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          requestBase64: Buffer.from('{"schema":"synthetic-request"}').toString('base64'),
          requestDigest: 'a'.repeat(64),
          requestFingerprint: 'b'.repeat(16),
          manifestDigest: 'c'.repeat(64),
          itemSetDigest: 'd'.repeat(64),
          operationId: 'protected-recovery-operation',
          sessionNonce: 'protected-recovery-session-nonce',
          expiresAt: '2026-09-01T00:00:00.000Z'
        })
      }
    })

    const result = await createProtectedRecoveryRuntimeClient(runtimeRequest)({
      operation: 'prepare-protected-recovery-request',
      importReceipt: {
        manifestJson: '{"schema":"analytix.provider-portable-manifest/v1","providers":[{"correlation":"provider-0","kind":"openai-compatible","endpoint":"https://portable-provider.invalid","models":[],"mediaModels":[],"routes":[],"intent":"reentry_required"}],"accounts":[]}',
        entries: [{
          correlation: 'provider-0',
          destinationProviderId: 'provider-fenced-destination',
          status: 'reentry_required',
          fence
        }]
      },
      localBinding: {
        browserWindowId: 'window', mainFrameId: 'frame', profileBinding: 'profile', dataDirectoryBinding: '/tmp/profile'
      },
      ownerBindingInventory: []
    })

    expect(observedPath).toBe('/v1/provider-registry/_private/protected-recovery/prepare')
    expect(observedMethod).toBe('POST')
    const request = JSON.parse(observedBody) as {
      importResult?: { entries?: Array<Record<string, unknown>> }
    }
    expect(request.importResult?.entries).toEqual([{
      correlation: 'provider-0',
      destinationProviderId: 'provider-fenced-destination',
      status: 'reentry_required',
      fence
    }])
    expect(result).toMatchObject({ ok: true, status: 'request-prepared' })
  })

  it('does not synthesize positional correlations for destination continuation inventory', async () => {
    let observedBody = ''
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      observedBody = body ?? ''
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({ schemaVersion: 1, status: 'pending', confirmed: true })
      }
    })
    const localBinding = {
      browserWindowId: 'window', mainFrameId: 'frame',
      profileBinding: 'profile', dataDirectoryBinding: '/tmp/profile'
    }
    const localAction = {
      localBinding,
      confirmation: {
        requestDigest: 'a'.repeat(64), requestFingerprint: 'b'.repeat(16),
        manifestDigest: 'c'.repeat(64), itemSetDigest: 'd'.repeat(64),
        operationId: 'protected-recovery-operation', sessionNonce: 'protected-recovery-session-nonce',
        expiresAt: '2026-09-01T00:00:00.000Z', localBinding
      }
    }
    const unrelated = {
      owner: 'extension' as const,
      provider: 'aaa-unrelated-provider', accountId: 'unrelated-account', channelId: 'unrelated-channel',
      purpose: 'extension-provider-account-token' as const, fingerprint: 'unrelated-binding'
    }
    const admittedTarget = {
      owner: 'mcp' as const,
      provider: 'target-provider', accountId: 'target-account', channelId: 'target-channel',
      purpose: 'mcp-oauth-access-token' as const, fingerprint: 'target-binding'
    }

    const result = await createProtectedRecoveryRuntimeClient(runtimeRequest)({
      operation: 'recover-protected-recovery',
      requestBytes: Buffer.from('{"schema":"synthetic-request"}'),
      localAction,
      ownerBindingInventory: [admittedTarget, unrelated]
    })

    const request = JSON.parse(observedBody) as {
      ownerBindingInventory?: Array<Record<string, unknown>>
    }
    expect(request.ownerBindingInventory).toEqual([unrelated, admittedTarget])
    expect(request.ownerBindingInventory?.every((entry) => !('correlation' in entry))).toBe(true)
    expect(result).toEqual({ ok: true, status: 'pending' })
  })

  it('rejects unknown, duplicate, or noncanonical private responses before returning artifacts', async () => {
    const malformed = [
      '{"schemaVersion":1,"status":"confirmed","confirmed":true,"extra":1}',
      '{"schemaVersion":1,"status":"confirmed","confirmed":true,"confirmed":true}',
      '{ "schemaVersion": 1, "status": "confirmed", "confirmed": true }'
    ]
    for (const body of malformed) {
      const runtimeRequest = vi.fn(async () => ({ ok: true, status: 200, body }))
      const client = createProtectedRecoveryRuntimeClient(runtimeRequest)
      await expect(client({
        operation: 'confirm-protected-recovery-destination',
        requestBytes: Buffer.from('{"request":true}'),
        localAction: { confirmation: {}, localBinding: {} } as never
      }))
        .resolves.toEqual({ ok: false, code: 'invalid_response' })
    }
  })

  it('requires exact confirmation booleans for durable protected-recovery status', async () => {
    const call = {
      operation: 'recover-protected-recovery' as const,
      requestBytes: Buffer.from('{"request":true}'),
      localAction: { confirmation: {}, localBinding: {} },
      ownerBindingInventory: []
    } as never
    const receiptBase64 = Buffer.from('synthetic-receipt').toString('base64')
    const cases = [
      {
        value: { status: 'receipt_ready', confirmed: false, receiptBase64 },
        expected: { ok: false, code: 'invalid_response' }
      },
      {
        value: { status: 'reconfirmation_required', confirmed: true },
        expected: { ok: false, code: 'invalid_response' }
      },
      {
        value: { status: 'receipt_ready', confirmed: true, receiptBase64 },
        expected: { ok: true, status: 'receipt_ready' }
      },
      {
        value: { status: 'reconfirmation_required', confirmed: false },
        expected: { ok: true, status: 'reconfirmation_required' }
      }
    ]
    for (const response of cases) {
      const runtimeRequest = vi.fn(async () => ({
        ok: true, status: 200, body: JSON.stringify({ schemaVersion: 1, ...response.value })
      }))
      const client = createProtectedRecoveryRuntimeClient(runtimeRequest)
      await expect(client(call)).resolves.toMatchObject(response.expected)
    }
  })

  it('keeps account credential bytes and opaque references out of the renderer management seam', async () => {
    const syntheticToken = 'synthetic-telegram-account-token'
    const scope = {
      owner: 'transport' as const,
      provider: 'telegram',
      accountId: 'account-a',
      channelId: 'channel-a',
      purpose: 'transport-telegram-bot-token' as const
    }
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      expect(body).toContain(Buffer.from(JSON.stringify({
        kind: 'telegram',
        botToken: syntheticToken,
        allowedChatIds: '1001'
      })).toString('base64'))
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          scope,
          status: 'ready',
          registryRevision: '5',
          registryIncarnation,
          providerRevision: '1',
          providerGeneration: '1',
          providerIncarnation
        })
      }
    })
    const handler = createAccountCredentialIpcHandler(runtimeRequest)
    const result = await handler({
      schemaVersion: 1,
      operation: 'put',
      scope,
      expected: {
        registryRevision: '4',
        registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      credential: { kind: 'telegram', botToken: syntheticToken, allowedChatIds: '1001' }
    })
    expect(result).toMatchObject({ status: 'ready', scope })
    expect(JSON.stringify(result)).not.toContain(syntheticToken)
    expect(JSON.stringify(result)).not.toMatch(/credentialRef|valueBase64|botToken/)
  })

  it('resolves an exact purpose-bound account only inside the Main consumer seam', async () => {
    const syntheticToken = 'synthetic-mcp-oauth-token'
    const scope = {
      owner: 'mcp' as const,
      provider: 'server-a',
      accountId: 'account-a',
      channelId: 'channel-a',
      purpose: 'mcp-oauth-access-token' as const
    }
    const runtimeRequest = vi.fn(async (_path: string, _method?: string, body?: string) => {
      expect(JSON.parse(body ?? '{}')).toEqual({ schemaVersion: 1, scope })
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          scope,
          status: 'ready',
          registryRevision: '7',
          registryIncarnation,
          providerRevision: '3',
          providerGeneration: '2',
          providerIncarnation,
          valueBase64: Buffer.from(JSON.stringify({
            kind: 'mcp-oauth-token', token: syntheticToken
          })).toString('base64')
        })
      }
    })
    const resolve = createMainAccountCredentialResolver(runtimeRequest)
    await expect(resolve(scope)).resolves.toMatchObject({
      ok: true,
      state: { providerGeneration: '2', providerIncarnation },
      credential: { kind: 'mcp-oauth-token', token: syntheticToken }
    })

    const crossPurpose = createMainAccountCredentialResolver(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        scope: { ...scope, purpose: 'mcp-oauth-refresh-token' },
        status: 'ready',
        registryRevision: '7',
        registryIncarnation,
        providerRevision: '3',
        providerGeneration: '2',
        providerIncarnation,
        valueBase64: Buffer.from(JSON.stringify({
          kind: 'mcp-oauth-token', token: syntheticToken
        })).toString('base64')
      })
    })))
    await expect(crossPurpose(scope)).resolves.toEqual({ ok: false, code: 'invalid_response' })

    const crossCredentialKind = createMainAccountCredentialResolver(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        scope,
        status: 'ready',
        registryRevision: '7',
        registryIncarnation,
        providerRevision: '3',
        providerGeneration: '2',
        providerIncarnation,
        valueBase64: Buffer.from(JSON.stringify({
          kind: 'telegram', botToken: syntheticToken, allowedChatIds: ''
        })).toString('base64')
      })
    })))
    await expect(crossCredentialKind(scope)).resolves.toEqual({ ok: false, code: 'invalid_response' })
  })

  it('can keep OAuth, MCP, and extension token writes out of the renderer writer', async () => {
    const runtimeRequest = vi.fn()
    const handler = createAccountCredentialIpcHandler(runtimeRequest, {
      allowPut: (scope) => scope.owner === 'transport'
    })
    await expect(handler({
      schemaVersion: 1,
      operation: 'put',
      scope: {
        owner: 'mcp',
        provider: 'server-a',
        accountId: 'account-a',
        channelId: 'server-a',
        purpose: 'mcp-oauth-access-token'
      },
      expected: {
        registryRevision: '0',
        registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      credential: { kind: 'mcp-oauth-token', token: 'must-stay-main-private' }
    })).resolves.toMatchObject({ error: { code: 'invalid_request' } })
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it.each([undefined, true])('routes typed Provider connect with deferSelection %s without exposing secrets', async (deferSelection) => {
    const syntheticSecretMarker = 'synthetic-provider-registry-secret-marker'
    const valueBase64 = Buffer.from(syntheticSecretMarker).toString('base64')
    const credentialRef = `cred_${'c'.repeat(43)}`
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        registryRevision: '1',
        registryIncarnation,
        provider: {
          id: 'provider-alpha',
          kind: 'openai-compatible',
          endpoint: 'https://provider.invalid/v1',
          models: ['model-alpha'],
          mediaModels: [],
          selectedModel: 'model-alpha',
          selectedRoutes: ['primary'],
          credentialConfigured: true,
          credentialPurpose: 'provider-api-key',
          revision: '1',
          generation: '1',
          incarnation: providerIncarnation,
          tombstone: false
        }
      })
    }))
    const handler = createProviderRegistryIpcHandler(runtimeRequest)

    const result = await handler({
      schemaVersion: 1,
      operation: 'connect',
      ...(deferSelection === undefined ? {} : { deferSelection }),
      expected: {
        registryRevision: '0',
        registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      provider: {
        id: 'provider-alpha',
        kind: 'openai-compatible',
        endpoint: 'https://provider.invalid/v1',
        proxy: '',
        models: ['model-alpha'],
        mediaModels: [],
        selectedModel: 'model-alpha',
        selectedMediaModel: '',
        selectedRoutes: ['primary']
      },
      credential: {
        kind: 'set',
        purpose: 'provider-api-key',
        valueBase64
      }
    })

    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry',
      'POST',
      JSON.stringify({
        schemaVersion: 1,
        ...(deferSelection === undefined ? {} : { deferSelection }),
        expected: {
          registryRevision: '0',
          registryIncarnation,
          providerRevision: '0',
          providerGeneration: '0',
          providerIncarnation: '',
          providerCredentialPurpose: ''
        },
        provider: {
          id: 'provider-alpha',
          kind: 'openai-compatible',
          endpoint: 'https://provider.invalid/v1',
          proxy: '',
          models: ['model-alpha'],
          mediaModels: [],
          selectedModel: 'model-alpha',
          selectedMediaModel: '',
          selectedRoutes: ['primary']
        },
        credential: { kind: 'set', purpose: 'provider-api-key', valueBase64 }
      })
    )
    expect(result).toMatchObject({ schemaVersion: 1, provider: { credentialConfigured: true } })
    expect(JSON.stringify(result)).not.toContain(syntheticSecretMarker)
    expect(JSON.stringify(result)).not.toContain(valueBase64)
    expect(JSON.stringify(result)).not.toContain(credentialRef)
    expect(JSON.stringify(result)).not.toContain('credentialRef')
  })

  it('maps every typed operation and update variant to exact fixed method path and CAS body', async () => {
    const replacementBase64 = Buffer.from('synthetic-provider-replacement').toString('base64')
    const updateBase64 = Buffer.from('synthetic-provider-update').toString('base64')
    const operations = [
      {
        request: { schemaVersion: 1, operation: 'list' },
        call: ['/v1/provider-registry', 'GET'],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            registryRevision: '4',
            registryIncarnation,
            selectedProviderId: 'provider-alpha',
            providers: [publicProvider({ revision: '7' })]
          })
        }
      },
      {
        request: { schemaVersion: 1, operation: 'export-portable-manifest' },
        call: ['/v1/provider-registry/portable-manifest', 'GET'],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            manifestJson: JSON.stringify({
              schema: 'analytix.provider-portable-manifest/v1',
              providers: [{
                correlation: 'provider-0', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
                models: ['model-alpha'], mediaModels: [], routes: [], intent: 'reentry_required'
              }],
              accounts: []
            })
          })
        }
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'import-portable-manifest',
          manifestJson: JSON.stringify({
            schema: 'analytix.provider-portable-manifest/v1',
            providers: [{
              correlation: 'provider-0', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
              models: ['model-alpha'], mediaModels: [], routes: [], intent: 'reentry_required'
            }],
            accounts: []
          })
        },
        call: ['/v1/provider-registry/portable-manifest', 'POST', JSON.stringify({
          schemaVersion: 1,
          manifestJson: JSON.stringify({
            schema: 'analytix.provider-portable-manifest/v1',
            providers: [{
              correlation: 'provider-0', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
              models: ['model-alpha'], mediaModels: [], routes: [], intent: 'reentry_required'
            }],
            accounts: []
          })
        })],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1, providerCount: 1, accountCount: 0, reentryRequired: 1,
            entries: [{ correlation: 'provider-0', destinationProviderId: 'provider-destination', status: 'reentry_required' }]
          })
        }
      },
      {
        request: { schemaVersion: 1, operation: 'get', providerId: 'provider-alpha' },
        call: ['/v1/provider-registry/providers/provider-alpha', 'GET'],
        response: providerResponse()
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'update',
          providerId: 'provider-alpha',
          expected: expectedExisting,
          provider: providerInput,
          credential: { kind: 'keep' }
        },
        call: ['/v1/provider-registry/providers/provider-alpha', 'PATCH', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting,
          provider: providerInput,
          credential: { kind: 'keep' }
        })],
        response: providerResponse()
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'update',
          providerId: 'provider-alpha',
          expected: expectedExisting,
          provider: providerInput,
          credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: updateBase64 }
        },
        call: ['/v1/provider-registry/providers/provider-alpha', 'PATCH', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting,
          provider: providerInput,
          credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: updateBase64 }
        })],
        response: providerResponse({ provider: publicProvider({ generation: '4' }) })
      },
      ...(['select', 'disconnect'] as const).map((operation) => ({
        request: { schemaVersion: 1, operation, providerId: 'provider-alpha', expected: expectedExisting },
        call: [
          `/v1/provider-registry/providers/provider-alpha/${operation}`,
          'POST',
          JSON.stringify({ schemaVersion: 1, expected: expectedExisting })
        ],
        response: operation === 'disconnect'
          ? providerResponse({
              provider: publicProvider({
                credentialConfigured: false,
                credentialPurpose: undefined,
                selectedRoutes: [],
                generation: '4',
                tombstone: true
              })
            })
          : providerResponse()
      })),
      {
        request: {
          schemaVersion: 1,
          operation: 'explicit-delete',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        call: ['/v1/provider-registry/providers/provider-alpha', 'DELETE', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting
        })],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            registryRevision: '5',
            registryIncarnation,
            deletedProviderId: 'provider-alpha'
          })
        }
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'credential-replace',
          providerId: 'provider-alpha',
          expected: expectedExisting,
          credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: replacementBase64 }
        },
        call: ['/v1/provider-registry/providers/provider-alpha/credential', 'PUT', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting,
          credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: replacementBase64 }
        })],
        response: providerResponse({ provider: publicProvider({ generation: '4' }) })
      },
      {
        request: { schemaVersion: 1, operation: 'credential-check', providerId: 'provider-alpha', expected: expectedExisting },
        call: ['/v1/provider-registry/providers/provider-alpha/credential-check', 'POST', JSON.stringify({ schemaVersion: 1, expected: expectedExisting })],
        response: { ok: true, status: 200, body: JSON.stringify({
          schemaVersion: 1, registryRevision: '4', registryIncarnation, providerId: 'provider-alpha',
          providerRevision: '7', providerGeneration: '3', providerIncarnation, credentialAvailable: true
        }) }
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'probe',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        call: ['/v1/provider-registry/providers/provider-alpha/probe', 'POST', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting
        })],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            registryRevision: '4',
            registryIncarnation,
            providerId: 'provider-alpha',
            providerRevision: '7',
            providerGeneration: '3',
            providerIncarnation,
            status: 'reachable',
            code: 200,
            modelCount: 1,
            latencyMs: 12
          })
        }
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'discover-models',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        call: ['/v1/provider-registry/providers/provider-alpha/discover-models', 'POST', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting
        })],
        response: providerResponse({
          provider: publicProvider({ models: ['model-discovered'], selectedModel: 'model-discovered' })
        })
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'observe-account',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        call: ['/v1/provider-registry/providers/provider-alpha/account-observation', 'POST', JSON.stringify({
          schemaVersion: 1,
          expected: expectedExisting
        })],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            registryRevision: '4',
            registryIncarnation,
            providerId: 'provider-alpha',
            providerRevision: '7',
            providerGeneration: '3',
            providerIncarnation,
            providerCredentialPurpose: 'provider-api-key',
            status: 'available',
            observedAt: '2026-08-31T00:00:00Z',
            expiresAt: '2026-08-31T00:05:00Z',
            quota: 100,
            usage: 40,
            remaining: 60
          })
        }
      },
      {
        request: { schemaVersion: 1, operation: 'recover' },
        call: ['/v1/provider-registry/recover', 'POST', JSON.stringify({ schemaVersion: 1 })],
        response: {
          ok: true, status: 200, body: JSON.stringify({
            schemaVersion: 1,
            registryRevision: '4',
            registryIncarnation,
            selectedProviderId: 'provider-alpha',
            providers: [publicProvider({ revision: '7' })],
            recovered: true
          })
        }
      }
    ]
    const runtimeRequest = vi.fn()
    const handler = createProviderRegistryIpcHandler(runtimeRequest)
    const publicResults: unknown[] = []

    for (const operation of operations) {
      runtimeRequest.mockResolvedValueOnce(operation.response)
      const result = await handler(operation.request)
      publicResults.push(result)
      expect(result).not.toHaveProperty('error')
      expect(runtimeRequest).toHaveBeenLastCalledWith(...operation.call)
    }
    expect(JSON.stringify(publicResults)).not.toContain('synthetic-provider-replacement')
    expect(JSON.stringify(publicResults)).not.toContain(`cred_${'c'.repeat(43)}`)
    expect(analytixProviderRegistryProviderPath('provider/alpha?#'))
      .toBe('/v1/provider-registry/providers/provider%2Falpha%3F%23')
  })

  it('requires portable import responses to match the requested manifest correlations', async () => {
    const manifestJson = JSON.stringify({
      schema: 'analytix.provider-portable-manifest/v1',
      providers: [{
        correlation: 'provider-0', kind: 'openai-compatible', endpoint: 'https://provider.invalid/v1',
        models: ['model-alpha'], mediaModels: [], routes: [], intent: 'reentry_required'
      }],
      accounts: []
    })
    const request = { schemaVersion: 1, operation: 'import-portable-manifest' as const, manifestJson }
    const invalidResponses = [
      {
        schemaVersion: 1, providerCount: 1, accountCount: 0, reentryRequired: 1,
        entries: [{ correlation: 'account-0', destinationProviderId: 'provider-destination', status: 'reentry_required' }]
      },
      {
        schemaVersion: 1, providerCount: 0, accountCount: 1, reentryRequired: 1,
        entries: [{ correlation: 'account-0', destinationProviderId: 'provider-destination', status: 'reentry_required' }]
      }
    ]
    for (const response of invalidResponses) {
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({
        ok: true, status: 200, body: JSON.stringify(response)
      })))
      await expect(handler(request)).resolves.toMatchObject({ error: { code: 'invalid_response' } })
    }

    const multiManifestJson = JSON.stringify({
      schema: 'analytix.provider-portable-manifest/v1',
      providers: [
        {
          correlation: 'provider-0', kind: 'openai-compatible', endpoint: 'https://a.provider.invalid/v1',
          models: [], mediaModels: [], routes: [], intent: 'reentry_required'
        },
        {
          correlation: 'provider-1', kind: 'openai-compatible', endpoint: 'https://b.provider.invalid/v1',
          models: [], mediaModels: [], routes: [], intent: 'reentry_required'
        }
      ],
      accounts: []
    })
    const duplicateDestinationResponse = {
      schemaVersion: 1,
      providerCount: 2,
      accountCount: 0,
      reentryRequired: 2,
      entries: [
        { correlation: 'provider-0', destinationProviderId: 'provider-destination', status: 'reentry_required' },
        { correlation: 'provider-1', destinationProviderId: 'provider-destination', status: 'reentry_required' }
      ]
    }
    const duplicateDestinationRuntimeRequest = vi.fn(async () => ({
      ok: true, status: 200, body: JSON.stringify(duplicateDestinationResponse)
    }))
    const duplicateDestinationHandler = createProviderRegistryIpcHandler(duplicateDestinationRuntimeRequest)
    await expect(duplicateDestinationHandler({
      schemaVersion: 1, operation: 'import-portable-manifest', manifestJson: multiManifestJson
    })).resolves.toMatchObject({ error: { code: 'invalid_response' } })
  })

  it('rejects malformed portable import requests before runtime transport', async () => {
    const runtimeRequest = vi.fn()
    const handler = createProviderRegistryIpcHandler(runtimeRequest)
    const malformedManifestJson = '{"schema":"analytix.provider-portable-manifest/v1","schema":"analytix.provider-portable-manifest/v1","providers":[],"accounts":[]}'
    await expect(handler({
      schemaVersion: 1,
      operation: 'import-portable-manifest',
      manifestJson: malformedManifestJson
    })).resolves.toMatchObject({ error: { code: 'invalid_request' } })
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('rejects renderer endpoint and credential authority before runtime transport', async () => {
    const runtimeRequest = vi.fn()
    const handler = createProviderRegistryIpcHandler(runtimeRequest)
    for (const injected of [
      { baseUrl: 'https://renderer.invalid/v1' },
      { endpoint: 'https://renderer.invalid/v1' },
      { endpointFormat: 'chat_completions' },
      { apiKey: 'synthetic-renderer-secret' },
      { credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: 'c3ludGhldGljLXNlY3JldA==' } }
    ]) {
      await expect(handler({
        schemaVersion: 1,
        operation: 'probe',
        providerId: 'provider-alpha',
        expected: expectedExisting,
        ...injected
      })).resolves.toMatchObject({ error: { code: 'invalid_request' } })
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('rejects impossible probe status, code, and model-count tuples as invalid responses', async () => {
    const response = {
      schemaVersion: 1,
      registryRevision: expectedExisting.registryRevision,
      registryIncarnation,
      providerId: 'provider-alpha',
      providerRevision: expectedExisting.providerRevision,
      providerGeneration: expectedExisting.providerGeneration,
      providerIncarnation,
      status: 'reachable',
      code: 200,
      modelCount: 1,
      latencyMs: 12
    }
    for (const impossible of [
      { ...response, code: 401 },
      { ...response, status: 'auth_failed', code: 200, modelCount: 0 },
      { ...response, status: 'auth_failed', code: 401, modelCount: 1 },
      { ...response, status: 'redirect_blocked', code: 200, modelCount: 0 },
      { ...response, status: 'provider_error', code: 307, modelCount: 0 },
      { ...response, status: 'timeout', code: 504, modelCount: 0 },
      { ...response, status: 'unavailable', code: undefined, modelCount: 1 },
      { ...response, status: 'invalid_response', code: 401, modelCount: 0 },
      { ...response, status: 'invalid_response', code: 200, modelCount: 1 }
    ]) {
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify(impossible)
      })))
      await expect(handler({
        schemaVersion: 1,
        operation: 'probe',
        providerId: 'provider-alpha',
        expected: expectedExisting
      })).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'invalid_response',
          message: 'The provider registry returned an invalid response.'
        }
      })
    }
  })

  it('rejects an account observation whose credential-purpose fence mismatches the request', async () => {
    const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({
        schemaVersion: 1,
        registryRevision: expectedExisting.registryRevision,
        registryIncarnation,
        providerId: 'provider-alpha',
        providerRevision: expectedExisting.providerRevision,
        providerGeneration: expectedExisting.providerGeneration,
        providerIncarnation,
        providerCredentialPurpose: 'provider-oauth-token-bundle',
        status: 'available',
        observedAt: '2026-08-31T00:00:00Z',
        expiresAt: '2026-08-31T00:05:00Z',
        remaining: 60
      })
    })))
    await expect(handler({
      schemaVersion: 1,
      operation: 'observe-account',
      providerId: 'provider-alpha',
      expected: expectedExisting
    })).resolves.toEqual({
      schemaVersion: 1,
      error: {
        code: 'invalid_response',
        message: 'The provider registry returned an invalid response.'
      }
    })
  })

  it('fails closed on malformed unknown secret-bearing and oversized runtime responses', async () => {
    const syntheticSecretMarker = 'synthetic-provider-response-secret'
    const credentialBase64 = Buffer.from(syntheticSecretMarker).toString('base64')
    const request = {
      schemaVersion: 1,
      operation: 'credential-replace',
      providerId: 'provider-alpha',
      expected: expectedExisting,
      credential: { kind: 'set', purpose: 'provider-api-key', valueBase64: credentialBase64 }
    }
    const cleanWire = JSON.parse(providerResponse().body)
    const invalidBodies = [
      JSON.stringify({
        ...cleanWire,
        provider: { ...cleanWire.provider, credentialRef: `cred_${'c'.repeat(43)}` }
      }),
      JSON.stringify({
        ...cleanWire,
        provider: {
          ...cleanWire.provider,
          models: [`cred_${'c'.repeat(43)}`],
          selectedModel: `cred_${'c'.repeat(43)}`
        }
      }),
      JSON.stringify({ ...cleanWire, provider: { ...cleanWire.provider, secret: syntheticSecretMarker } }),
      JSON.stringify({ ...cleanWire, unknown: true }),
      `${JSON.stringify(cleanWire)}{}`,
      JSON.stringify({ ...cleanWire, provider: { ...cleanWire.provider, revision: '08' } }),
      JSON.stringify({
        ...cleanWire,
        provider: {
          ...cleanWire.provider,
          models: [syntheticSecretMarker],
          selectedModel: syntheticSecretMarker
        }
      }),
      JSON.stringify({ ...cleanWire, padding: 'x'.repeat((16 << 20) + 1) })
    ]

    for (const body of invalidBodies) {
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({ ok: true, status: 200, body })))
      await expect(handler(request)).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'invalid_response',
          message: 'The provider registry returned an invalid response.'
        }
      })
    }
  })

  it('rejects escaped credential echoes after JSON parsing', async () => {
    const invalidResponse = {
      schemaVersion: 1,
      error: {
        code: 'invalid_response',
        message: 'The provider registry returned an invalid response.'
      }
    }
    const escapedBase64 = '//8='
    const decodedSecret = 'synthetic-provider-unicode-echo'
    const cases = [
      {
        valueBase64: escapedBase64,
        echoedValue: escapedBase64,
        escape: (body: string) => body.replaceAll(escapedBase64, escapedBase64.replaceAll('/', '\\/'))
      },
      {
        valueBase64: Buffer.from(decodedSecret).toString('base64'),
        echoedValue: decodedSecret,
        escape: (body: string) => body.replaceAll(
          decodedSecret,
          Array.from(decodedSecret)
            .map((character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`)
            .join('')
        )
      }
    ]

    for (const testCase of cases) {
      const cleanWire = JSON.parse(providerResponse({
        provider: publicProvider({
          models: [testCase.echoedValue],
          selectedModel: testCase.echoedValue,
          generation: '4'
        })
      }).body)
      const body = testCase.escape(JSON.stringify(cleanWire))
      expect(body).not.toContain(testCase.echoedValue)
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({ ok: true, status: 200, body })))

      await expect(handler({
        schemaVersion: 1,
        operation: 'credential-replace',
        providerId: 'provider-alpha',
        expected: expectedExisting,
        credential: {
          kind: 'set',
          purpose: 'provider-api-key',
          valueBase64: testCase.valueBase64
        }
      })).resolves.toEqual(invalidResponse)
    }
  })

  it('binds every existing-provider mutation response to the requested provider', async () => {
    const replacementBase64 = Buffer.from('synthetic-provider-identity-binding').toString('base64')
    const cases = [
      {
        request: {
          schemaVersion: 1,
          operation: 'select',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        response: providerResponse({ provider: publicProvider({ id: 'provider-foreign' }) })
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'disconnect',
          providerId: 'provider-alpha',
          expected: expectedExisting
        },
        response: providerResponse({
          provider: publicProvider({
            id: 'provider-foreign',
            credentialConfigured: false,
            credentialPurpose: undefined,
            selectedRoutes: [],
            generation: '4',
            tombstone: true
          })
        })
      },
      {
        request: {
          schemaVersion: 1,
          operation: 'credential-replace',
          providerId: 'provider-alpha',
          expected: expectedExisting,
          credential: {
            kind: 'set',
            purpose: 'provider-api-key',
            valueBase64: replacementBase64
          }
        },
        response: providerResponse({
          provider: publicProvider({ id: 'provider-foreign', generation: '4' })
        })
      }
    ]

    for (const testCase of cases) {
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => testCase.response))
      await expect(handler(testCase.request)).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'invalid_response',
          message: 'The provider registry returned an invalid response.'
        }
      })
    }
  })

  it('projects the production-shaped runtime transport failure as unavailable', async () => {
    const listRequest = { schemaVersion: 1, operation: 'list' }
    for (const code of ['fetch_failed', 'runtime_unavailable']) {
      const productionTransportFailure = createProviderRegistryIpcHandler(vi.fn(async () => ({
        ok: false,
        status: 0,
        body: JSON.stringify({ code, message: 'The Analytix runtime is unavailable.' })
      })))
      await expect(productionTransportFailure(listRequest)).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'runtime_unavailable',
          message: 'The provider registry is unavailable.'
        }
      })
    }

    const malformedTransportFailures = [
      { code: 'fetch_failed', message: 'The Analytix runtime is unavailable.', detail: 'raw-provider-body' },
      { code: 'runtime_unavailable', message: 'The Analytix runtime is unavailable.', detail: 'raw-provider-body' },
      { code: 'runtime_unavailable', message: 'runtime failed at /private/provider-registry' },
      { code: 'fetch_failed', message: 'runtime failed at /private/provider-registry' },
      { code: 'other_failure', message: 'The Analytix runtime is unavailable.' },
      {
        code: 'fetch_failed',
        message: 'The Analytix runtime is unavailable.',
        credentialRef: `cred_${'e'.repeat(43)}`
      }
    ]
    for (const body of malformedTransportFailures) {
      const handler = createProviderRegistryIpcHandler(vi.fn(async () => ({
        ok: false,
        status: 0,
        body: JSON.stringify(body)
      })))
      await expect(handler(listRequest)).resolves.toMatchObject({
        error: { code: 'invalid_response' }
      })
    }
    const falseSuccess = createProviderRegistryIpcHandler(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({ code: 'runtime_unavailable', message: 'The Analytix runtime is unavailable.' })
    })))
    await expect(falseSuccess(listRequest)).resolves.toMatchObject({ error: { code: 'invalid_response' } })
  })

  it('preserves canonical Go failures while redacting raw and transport failures', async () => {
    const listRequest = { schemaVersion: 1, operation: 'list' }
    const failures = [
      ['invalid_request', 400, 'The provider registry request was rejected.'],
      ['method_not_allowed', 405, 'The HTTP method is not allowed for this provider registry operation.'],
      ['not_found', 404, 'The requested provider was not found.'],
      ['conflict', 409, 'The provider registry state has changed.'],
      ['persistence_failure', 503, 'The provider registry is temporarily unavailable.'],
      ['verification_failure', 500, 'The provider registry operation could not be verified.'],
      ['request_too_large', 413, 'The provider registry request is too large.'],
      ['unauthorized', 401, 'Provider registry authentication is required.']
    ] as const
    for (const [code, status, message] of failures) {
      const failure = createProviderRegistryIpcHandler(vi.fn(async () => ({
        ok: false,
        status,
        body: JSON.stringify({ schemaVersion: 1, error: { code, message } })
      })))
      await expect(failure(listRequest)).resolves.toEqual({
        schemaVersion: 1,
        error: { code, message }
      })
    }

    const rawFailure = createProviderRegistryIpcHandler(vi.fn(async () => ({
      ok: false,
      status: 503,
      body: JSON.stringify({
        schemaVersion: 1,
        error: {
          code: 'persistence_failure',
          message: 'failure at /private/provider-registry with raw-provider-body'
        },
        credentialRef: `cred_${'d'.repeat(43)}`
      })
    })))
    await expect(rawFailure(listRequest)).resolves.toMatchObject({
      error: { code: 'invalid_response', message: 'The provider registry returned an invalid response.' }
    })

    const transportFailure = createProviderRegistryIpcHandler(vi.fn(async () => {
      throw new Error('stderr /private/provider-registry raw-provider-body')
    }))
    await expect(transportFailure(listRequest)).resolves.toEqual({
      schemaVersion: 1,
      error: { code: 'runtime_unavailable', message: 'The provider registry is unavailable.' }
    })

    const malformedTransport = createProviderRegistryIpcHandler(vi.fn(async () => ({
      ok: true, status: 200, body: providerResponse().body, stdout: 'raw-provider-body'
    })))
    await expect(malformedTransport(listRequest)).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })
  })
})


it('rejects stale or secret-bearing credential availability replies', async () => {
  for (const patch of [{ providerGeneration: '2' }, { credential: 'synthetic-leaked-value' }]) {
    const response = { schemaVersion: 1, registryRevision: '4', registryIncarnation, providerId: 'provider-alpha',
      providerRevision: '7', providerGeneration: '3', providerIncarnation, credentialAvailable: true, ...patch }
    const transport = vi.fn(async () => ({ ok: true, status: 200, body: JSON.stringify(response) }))
    const handler = createProviderRegistryIpcHandler(transport)
    await expect(handler({ schemaVersion: 1, operation: 'credential-check', providerId: 'provider-alpha', expected: expectedExisting }))
      .resolves.toMatchObject({ error: { code: 'invalid_response' } })
  }
})
