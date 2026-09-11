import { Buffer } from 'node:buffer'
import { createHash } from 'node:crypto'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createProviderRegistryLegacyMigrationClient } from './provider-registry-legacy-migration'

const registryIncarnation = `inc_${'a'.repeat(43)}`
const migrationId = 'migration-settings-alpha'
const sourceLocator = 'current:analytix-settings.json'
const sourceSnapshot = new Uint8Array(Buffer.from('{"synthetic":"legacy-source-snapshot"}', 'utf8'))
const sourceSHA256 = createHash('sha256').update(sourceSnapshot).digest('hex')
const expectedCleanedSource = new Uint8Array(Buffer.from('{"synthetic":"key-free-source"}', 'utf8'))
const expectedCleanedSourceSHA256 = createHash('sha256').update(expectedCleanedSource).digest('hex')
const sourcePhysicalIdentitySHA256 = 'e'.repeat(64)
const sourceAuthorityChallenge = `lmsa_${'A'.repeat(43)}`
const sourceAuthority = {
  challenge: sourceAuthorityChallenge,
  sourcePath: '/private/synthetic-analytix-settings.json',
  lockOwnerToken: '00000000-0000-4000-8000-000000000001',
  sourceDevice: '1',
  sourceInode: '2'
}
const recoveryCredentialRef = `cred_${'R'.repeat(43)}`
const providerCredentialRef = `cred_${'C'.repeat(43)}`
const providerIncarnation = `inc_${'b'.repeat(43)}`
const expected = {
  registryRevision: '0',
  registryIncarnation,
  providerRevision: '0',
  providerGeneration: '0',
  providerIncarnation: '',
  providerCredentialPurpose: ''
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
const publicProvider = {
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

function prepareInput(credential: Uint8Array) {
  return {
    schemaVersion: 1 as const,
    expected,
    migrationId,
    sourceLocator,
    sourceSHA256,
    expectedCleanedSourceSHA256,
    sourcePhysicalIdentitySHA256,
    provider,
    credentialPurpose: 'provider-api-key',
    credential,
    sourceSnapshot,
    activeCredentialLocators: ['current:analytix-settings.json:provider.apiKey']
  }
}

function protectedPrepareInput(
  activeCredential: Uint8Array,
  shadowRuntimeCredential: Uint8Array,
  shadowLegacyCredential: Uint8Array
) {
  return {
    ...prepareInput(activeCredential),
    activeCredentialLocators: [
      'current:analytix-settings.json:agents.kun.apiKey',
      'current:analytix-settings.json:provider.providers[0].apiKey'
    ],
    rollbackCredentialArtifacts: [
      {
        schemaVersion: 1 as const,
        credentialLocators: [
          'current:analytix-settings.json:provider.apiKey',
          'current:analytix-settings.json:runtime.apiKey'
        ],
        credential: shadowRuntimeCredential
      },
      {
        schemaVersion: 1 as const,
        credentialLocators: [
          'current:analytix-settings.json:deepseek.apiKey'
        ],
        credential: shadowLegacyCredential
      }
    ]
  }
}

function prepareSuccess(overrides: Record<string, unknown> = {}) {
  return {
    ok: true,
    status: 200,
    body: JSON.stringify({
      schemaVersion: 1,
      status: 'VERIFIED_RECOVERY',
      migrationId,
      recoveryCredentialRef,
      safeToProceedWithProviderMigration: true,
      ...overrides
    })
  }
}

function committedRetainedPrepareSuccess(overrides: Record<string, unknown> = {}) {
  return {
    ok: true,
    status: 200,
    body: JSON.stringify({
      schemaVersion: 1,
      status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
      migrationId,
      recoveryCredentialRef,
      safeToProceedWithProviderMigration: false,
      ...overrides
    })
  }
}

function abandonInput() {
  return {
    schemaVersion: 1 as const,
    migrationId,
    sourceLocator,
    sourceSHA256,
    recoveryCredentialRef,
    confirmation: 'ABANDON_PROVIDER_SETTINGS_MIGRATION_RECOVERY' as const
  }
}

function commitInput() {
  return {
    schemaVersion: 1 as const,
    expected,
    expectedSelectedProviderID: '',
    migrationId,
    sourceLocator,
    sourceSHA256,
    recoveryCredentialRef,
    confirmation: 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY' as const
  }
}

function commitSuccess(overrides: Record<string, unknown> = {}) {
  return {
    ok: true,
    status: 200,
    body: JSON.stringify({
      schemaVersion: 1,
      status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
      migrationId,
      safeToRemoveLegacyPlaintext: true,
      provider: publicProvider,
      ...overrides
    })
  }
}

describe('main-private Provider Registry legacy migration client', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses only the fixed prepare path and canonical body while clearing temporary buffers', async () => {
    const syntheticCredential = new Uint8Array(Buffer.from('synthetic-main-private-credential'))
    const originalCredential = new Uint8Array(syntheticCredential)
    const encodedCredential = Buffer.from(syntheticCredential).toString('base64')
    const fill = vi.spyOn(Buffer.prototype, 'fill')
    const runtimeRequest = vi.fn(async () => prepareSuccess())
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)

    await expect(client.prepare(prepareInput(syntheticCredential))).resolves.toEqual({
      schemaVersion: 1,
      status: 'VERIFIED_RECOVERY',
      migrationId,
      recoveryCredentialRef,
      safeToProceedWithProviderMigration: true
    })

    expect(runtimeRequest).toHaveBeenCalledTimes(1)
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry/_private/legacy-migration-recovery/prepare',
      'POST',
      JSON.stringify({
        schemaVersion: 1,
        expected,
        migrationId,
        sourceLocator,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256,
        provider,
        credential: { purpose: 'provider-api-key', valueBase64: encodedCredential },
        rollbackSource: { valueBase64: Buffer.from(sourceSnapshot).toString('base64') },
        activeCredentialLocators: ['current:analytix-settings.json:provider.apiKey']
      })
    )
    expect(syntheticCredential).toEqual(originalCredential)
    expect(fill.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it('accepts an exactly correlated committed-retained prepare replay without requesting commit', async () => {
    const credential = new Uint8Array(Buffer.from('synthetic-main-private-retained-replay'))
    const runtimeRequest = vi.fn(async () => committedRetainedPrepareSuccess())
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)

    await expect(client.prepare(prepareInput(credential))).resolves.toEqual({
      schemaVersion: 1,
      status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
      migrationId,
      recoveryCredentialRef,
      safeToProceedWithProviderMigration: false
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })

  it('rejects malformed input before transport and never exposes a read operation', async () => {
    const runtimeRequest = vi.fn()
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    expect(Object.keys(client).sort()).toEqual([
      'abandon', 'beginRollback', 'commit', 'commitRollback', 'deleteRetainedRecovery',
      'finalize', 'inventory', 'issueSourceAuthorityChallenge', 'prepare', 'remigrate'
    ])
    const invalidInputs = [
      { ...prepareInput(new Uint8Array([1])), schemaVersion: 2 },
      { ...prepareInput(new Uint8Array([1])), migrationId: 'migration/settings' },
      { ...prepareInput(new Uint8Array([1])), sourceSHA256: 'A'.repeat(64) },
      { ...prepareInput(new Uint8Array([1])), credentialPurpose: 'Provider API Key' },
      { ...prepareInput(new Uint8Array()), credential: new Uint8Array() },
      { ...prepareInput(new Uint8Array([1])), provider: { ...provider, unknown: true } },
      { ...prepareInput(new Uint8Array([1])), unknown: true }
    ]
    for (const input of invalidInputs) {
      await expect(client.prepare(input as never)).resolves.toMatchObject({
        error: { code: 'invalid_request' }
      })
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('maps ordered active locators and protected shadow credentials onto only the fixed prepare route', async () => {
    const activeCredential = new Uint8Array(Buffer.from('synthetic-main-private-active-credential'))
    const shadowRuntimeCredential = new Uint8Array(Buffer.from('synthetic-main-private-shadow-runtime'))
    const shadowLegacyCredential = new Uint8Array(Buffer.from('synthetic-main-private-shadow-legacy'))
    const originalCredentials = [
      new Uint8Array(activeCredential),
      new Uint8Array(shadowRuntimeCredential),
      new Uint8Array(shadowLegacyCredential)
    ]
    const input = protectedPrepareInput(
      activeCredential,
      shadowRuntimeCredential,
      shadowLegacyCredential
    )
    const encodedCredentials = [activeCredential, shadowRuntimeCredential, shadowLegacyCredential]
      .map((credential) => Buffer.from(credential).toString('base64'))
    const fill = vi.spyOn(Buffer.prototype, 'fill')
    const runtimeRequest = vi.fn(async () => prepareSuccess())
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)

    await expect(client.prepare(input)).resolves.toMatchObject({
      status: 'VERIFIED_RECOVERY',
      migrationId
    })

    expect(runtimeRequest).toHaveBeenCalledTimes(1)
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry/_private/legacy-migration-recovery/prepare',
      'POST',
      JSON.stringify({
        schemaVersion: 1,
        expected,
        migrationId,
        sourceLocator,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256,
        provider,
        credential: { purpose: 'provider-api-key', valueBase64: encodedCredentials[0] },
        rollbackSource: { valueBase64: Buffer.from(sourceSnapshot).toString('base64') },
        activeCredentialLocators: input.activeCredentialLocators,
        rollbackCredentialArtifacts: [
          {
            schemaVersion: 1,
            credentialLocators: input.rollbackCredentialArtifacts[0].credentialLocators,
            credential: { valueBase64: encodedCredentials[1] }
          },
          {
            schemaVersion: 1,
            credentialLocators: input.rollbackCredentialArtifacts[1].credentialLocators,
            credential: { valueBase64: encodedCredentials[2] }
          }
        ]
      })
    )
    expect(activeCredential).toEqual(originalCredentials[0])
    expect(shadowRuntimeCredential).toEqual(originalCredentials[1])
    expect(shadowLegacyCredential).toEqual(originalCredentials[2])
    expect(fill.mock.calls.length).toBeGreaterThanOrEqual(6)
  })

  it('rejects malformed, ambiguous, and over-budget protected prepare input before transport', async () => {
    const active = new Uint8Array(Buffer.from('synthetic-main-private-v2-active'))
    const shadowOne = new Uint8Array(Buffer.from('synthetic-main-private-v2-shadow-one'))
    const shadowTwo = new Uint8Array(Buffer.from('synthetic-main-private-v2-shadow-two'))
    const valid = protectedPrepareInput(active, shadowOne, shadowTwo)
    const missingLocatorInventory: Record<string, unknown> = { ...valid }
    delete missingLocatorInventory.activeCredentialLocators
    delete missingLocatorInventory.rollbackCredentialArtifacts
    const runtimeRequest = vi.fn()
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    const invalidInputs = [
      missingLocatorInventory,
      { ...valid, activeCredentialLocators: undefined },
      { ...valid, activeCredentialLocators: [] },
      { ...valid, activeCredentialLocators: ['/private/settings.json:provider.apiKey'] },
      {
        ...valid,
        rollbackCredentialArtifacts: [{
          ...valid.rollbackCredentialArtifacts[0],
          credentialLocators: [valid.activeCredentialLocators[0]]
        }]
      },
      {
        ...valid,
        rollbackCredentialArtifacts: [{
          ...valid.rollbackCredentialArtifacts[0],
          credential: active
        }]
      },
      {
        ...valid,
        activeCredentialLocators: Array.from(
          { length: 257 },
          (_, index) => `current:analytix-settings.json:provider.providers[${index % 64}].apiKey`
        )
      },
      protectedPrepareInput(
        new Uint8Array(600_000).fill(0x41),
        new Uint8Array(500_000).fill(0x42),
        shadowTwo
      ),
      { ...valid, sourceLocator: '/private/settings.json' }
    ]
    for (const input of invalidInputs) {
      await expect(client.prepare(input as never)).resolves.toMatchObject({
        error: { code: 'invalid_request' }
      })
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('rejects any shadow credential, base64, locator, or source echo from protected prepare', async () => {
    const active = new Uint8Array(Buffer.from('synthetic-main-private-v2-echo-active'))
    const shadowOne = new Uint8Array(Buffer.from('synthetic-main-private-v2-echo-shadow-one'))
    const shadowTwo = new Uint8Array(Buffer.from('synthetic-main-private-v2-echo-shadow-two'))
    const input = protectedPrepareInput(active, shadowOne, shadowTwo)
    const encodedShadow = Buffer.from(shadowOne).toString('base64')
    const cases = [
      prepareSuccess({ echoedShadowPlaintext: 'synthetic-main-private-v2-echo-shadow-one' }),
      prepareSuccess({ echoedShadowBase64: encodedShadow }),
      prepareSuccess({ locator: input.rollbackCredentialArtifacts[0].credentialLocators[0] }),
      prepareSuccess({ sourceSHA256 })
    ]
    for (const result of cases) {
      const client = createProviderRegistryLegacyMigrationClient(vi.fn(async () => result))
      await expect(client.prepare(input)).resolves.toMatchObject({
        error: { code: 'invalid_response' }
      })
    }
  })

  it('uses only the fixed commit path and canonical body and accepts only key-free committed-retained output', async () => {
    const runtimeRequest = vi.fn(async () => commitSuccess())
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)

    await expect(client.commit(commitInput())).resolves.toEqual({
      schemaVersion: 1,
      status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
      migrationId,
      safeToRemoveLegacyPlaintext: true,
      provider: publicProvider
    })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry/_private/legacy-migration-recovery/commit',
      'POST',
      JSON.stringify(commitInput())
    )
  })

  it('rejects malformed commit input before transport', async () => {
    const runtimeRequest = vi.fn()
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    const invalidInputs = [
      { ...commitInput(), schemaVersion: 2 },
      { ...commitInput(), expected: { ...expected, registryRevision: '00' } },
      { ...commitInput(), expected: { ...expected, providerGeneration: 0 } },
      { ...commitInput(), expectedSelectedProviderID: 'provider/alpha' },
      { ...commitInput(), migrationId: 'migration/settings' },
      { ...commitInput(), sourceSHA256: 'A'.repeat(64) },
      { ...commitInput(), recoveryCredentialRef: 'readable-secret-handle' },
      { ...commitInput(), confirmation: 'COMMIT' },
      { ...commitInput(), unknown: true }
    ]
    for (const input of invalidInputs) {
      await expect(client.commit(input as never)).resolves.toMatchObject({
        error: { code: 'invalid_request' }
      })
    }
    expect(runtimeRequest).not.toHaveBeenCalled()
  })

  it('requires exact commit correlation and rejects ref, source, body, secret, trailing, unknown, and oversized results', async () => {
    const cases = [
      commitSuccess({ migrationId: 'migration-settings-other' }),
      commitSuccess({ status: 'VERIFIED_RECOVERY' }),
      commitSuccess({ safeToRemoveLegacyPlaintext: false }),
      commitSuccess({ unknown: true }),
      { ...commitSuccess(), body: commitSuccess().body + '{}' },
      { ...commitSuccess(), body: 'x'.repeat(70 << 10) },
      { ...commitSuccess(), body: commitSuccess().body + ' '.repeat(70 << 10) },
      commitSuccess({ recoveryCredentialRef }),
      commitSuccess({ sourceSHA256 }),
      commitSuccess({ rawBody: 'raw-provider-body' }),
      commitSuccess({ envelope: 'K1-envelope' }),
      commitSuccess({ provider: { ...publicProvider, credentialRef: providerCredentialRef } }),
      commitSuccess({ provider: { ...publicProvider, unknown: true } }),
      { ...commitSuccess(), stdout: 'raw-provider-body' },
      { ok: true, status: 200, body: '{' },
      { ok: false, status: 500, body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'verification_failure', message: 'raw-provider-body at /private/secret-path' }
      }) }
    ]
    for (const result of cases) {
      const client = createProviderRegistryLegacyMigrationClient(vi.fn(async () => result))
      await expect(client.commit(commitInput())).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'invalid_response',
          message: 'The provider registry returned an invalid response.'
        }
      })
    }
  })

  it('preserves exact stable commit failures while rejecting echoed recovery authority', async () => {
    const conflict = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 409,
      body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'conflict', message: 'The provider registry state has changed.' }
      })
    })))
    await expect(conflict.commit(commitInput())).resolves.toMatchObject({
      error: { code: 'conflict' }
    })

    const echoed = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 409,
      body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'conflict', message: 'The provider registry state has changed.' },
        recoveryCredentialRef
      })
    })))
    await expect(echoed.commit(commitInput())).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })
  })

  it('requires exact prepare correlation and rejects echo, unsafe, trailing, unknown, oversized, and malformed results', async () => {
    const credential = new Uint8Array(Buffer.from('synthetic-response-echo-marker'))
    const encoded = Buffer.from(credential).toString('base64')
    const cases = [
      prepareSuccess({ migrationId: 'migration-settings-other' }),
      prepareSuccess({ safeToProceedWithProviderMigration: false }),
      prepareSuccess({ status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED' }),
      committedRetainedPrepareSuccess({ migrationId: 'migration-settings-other' }),
      committedRetainedPrepareSuccess({ recoveryCredentialRef: 'invalid-ref' }),
      committedRetainedPrepareSuccess({ safeToProceedWithProviderMigration: true }),
      prepareSuccess({ unknown: true }),
      { ...prepareSuccess(), body: prepareSuccess().body + '{}' },
      { ...prepareSuccess(), body: 'x'.repeat(70 << 10) },
      prepareSuccess({ echoedCredential: encoded }),
      prepareSuccess({ echoedPlaintext: 'synthetic-response-echo-marker' }),
      prepareSuccess({ sourceSHA256 }),
      { ...prepareSuccess(), stdout: 'raw-provider-body' },
      { ok: true, status: 200, body: '{' },
      { ok: false, status: 500, body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'verification_failure', message: 'raw-provider-body at /private/secret-path' }
      }) },
      { ok: false, status: 500, body: JSON.stringify({
        schemaVersion: 1,
        error: {
          code: 'verification_failure',
          message: 'The provider registry operation could not be verified.'
        },
        recoveryCredentialRef
      }) }
    ]
    for (const result of cases) {
      const client = createProviderRegistryLegacyMigrationClient(vi.fn(async () => result))
      await expect(client.prepare(prepareInput(credential))).resolves.toEqual({
        schemaVersion: 1,
        error: {
          code: 'invalid_response',
          message: 'The provider registry returned an invalid response.'
        }
      })
    }
  })

  it('maps only the exact status-0 unavailable envelope', async () => {
    const credential = new Uint8Array([1])
    const unavailable = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 0,
      body: JSON.stringify({
        code: 'fetch_failed',
        message: 'The Analytix runtime is unavailable.'
      })
    })))
    await expect(unavailable.prepare(prepareInput(credential))).resolves.toMatchObject({
      error: { code: 'runtime_unavailable', message: 'The provider registry is unavailable.' }
    })

    const malformed = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 0,
      body: JSON.stringify({
        code: 'fetch_failed',
        message: 'The Analytix runtime is unavailable.',
        recoveryCredentialRef
      })
    })))
    await expect(malformed.prepare(prepareInput(credential))).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })
  })

  it('uses only the fixed explicit-abandon path and accepts only the minimal completion', async () => {
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({ schemaVersion: 1, status: 'COMPLETED' })
    }))
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    await expect(client.abandon(abandonInput())).resolves.toEqual({
      schemaVersion: 1,
      status: 'COMPLETED'
    })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry/_private/legacy-migration-recovery/abandon',
      'POST',
      JSON.stringify(abandonInput())
    )

    await expect(client.abandon({
      ...abandonInput(),
      confirmation: 'ABANDON'
    } as never)).resolves.toMatchObject({ error: { code: 'invalid_request' } })
    expect(runtimeRequest).toHaveBeenCalledTimes(1)
  })

  it('preserves only canonical stable failures and rejects references on abandon errors', async () => {
    const conflict = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 409,
      body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'conflict', message: 'The provider registry state has changed.' }
      })
    })))
    await expect(conflict.abandon(abandonInput())).resolves.toMatchObject({
      error: { code: 'conflict' }
    })

    const refOnError = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: false,
      status: 409,
      body: JSON.stringify({
        schemaVersion: 1,
        error: { code: 'conflict', message: 'The provider registry state has changed.' },
        recoveryCredentialRef
      })
    })))
    await expect(refOnError.abandon(abandonInput())).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })
  })

  it('uses only key-free bounded private rollback and finalization routes', async () => {
    let rollbackCommitted = false
    const runtimeRequest = vi.fn(async (path: string, _method: 'POST', _body: string) => {
      if (path.endsWith('/rollback/begin')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            status: rollbackCommitted
              ? 'ROLLBACK_RECOVERY_RETAINED'
              : 'CLEANED_SOURCE_AUTHORITY_REQUIRED',
            migrationId
          })
        }
      }
      if (path.endsWith('/rollback/commit')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED',
            migrationId
          })
        }
      }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          status: 'ALREADY_FINALIZED',
          outcome: 'PROTECTED_RECOVERY_RETAINED',
          migrationId
        })
      }
    })
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    const beginInput = {
      schemaVersion: 1 as const,
      migrationId,
      sourceLocator,
      sourceSHA256,
      sourcePhysicalIdentitySHA256,
      recoveryCredentialRef,
      confirmation: 'BEGIN_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK' as const
    }
    const begin = await client.beginRollback(beginInput)
    expect(begin).toMatchObject({
      schemaVersion: 1,
      status: 'CLEANED_SOURCE_AUTHORITY_REQUIRED',
      migrationId
    })
    expect(JSON.stringify(begin)).not.toContain(sourceSHA256)
    expect(JSON.stringify(begin)).not.toContain(recoveryCredentialRef)
    expect(runtimeRequest).toHaveBeenNthCalledWith(
      1,
      '/v1/provider-registry/_private/legacy-migration-recovery/rollback/begin',
      'POST',
      JSON.stringify(beginInput)
    )

    const commitInput = {
      schemaVersion: 1 as const,
      expected: {
        ...expected,
        registryRevision: '1',
        providerRevision: '1',
        providerGeneration: '1',
        providerIncarnation,
        providerCredentialPurpose: 'provider-api-key'
      },
      migrationId,
      sourceLocator,
      sourceSHA256,
      verifiedCleanedSourceSHA256: expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      verifiedCleanedSource: expectedCleanedSource,
      sourceAuthority,
      recoveryCredentialRef,
      confirmation: 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_ROLLBACK' as const
    }
    await expect(client.commitRollback(commitInput)).resolves.toEqual({
      schemaVersion: 1,
      status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED',
      migrationId
    })
    rollbackCommitted = true
    const repeatedBegin = await client.beginRollback(beginInput)
    expect(repeatedBegin).toMatchObject({
      schemaVersion: 1,
      status: 'ROLLBACK_RECOVERY_RETAINED',
      migrationId
    })
    const finalizeInput = {
      schemaVersion: 1 as const,
      migrationId,
      sourceLocator,
      sourceSHA256,
      verifiedSourceSHA256: expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      verifiedSource: expectedCleanedSource,
      sourceAuthority,
      recoveryCredentialRef,
      confirmation: 'FINALIZE_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY' as const
    }
    await expect(client.finalize(finalizeInput)).resolves.toEqual({
      schemaVersion: 1,
      status: 'ALREADY_FINALIZED',
      outcome: 'PROTECTED_RECOVERY_RETAINED',
      migrationId
    })
    expect(runtimeRequest.mock.calls.map(([path]) => path)).toEqual([
      '/v1/provider-registry/_private/legacy-migration-recovery/rollback/begin',
      '/v1/provider-registry/_private/legacy-migration-recovery/rollback/commit',
      '/v1/provider-registry/_private/legacy-migration-recovery/rollback/begin',
      '/v1/provider-registry/_private/legacy-migration-recovery/finalize'
    ])
  })

  it('remigrates and deletes retained recovery only through specialized key-free private routes', async () => {
    const runtimeRequest = vi.fn(async (path: string, _method: 'POST', _body: string) => {
      if (path.endsWith('/remigrate')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
            migrationId,
            safeToRemoveLegacyPlaintext: false,
            provider: publicProvider
          })
        }
      }
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({ schemaVersion: 1, status: 'COMPLETED', migrationId })
      }
    })
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    const baseInput = {
      schemaVersion: 1 as const,
      expected,
      migrationId,
      sourceLocator,
      sourceSHA256,
      verifiedCleanedSourceSHA256: expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      verifiedCleanedSource: expectedCleanedSource,
      sourceAuthority,
      recoveryCredentialRef
    }

    await expect(client.remigrate({
      ...baseInput,
      confirmation: 'REMIGRATE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
    })).resolves.toEqual({
      schemaVersion: 1,
      status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED',
      migrationId,
      safeToRemoveLegacyPlaintext: false,
      provider: publicProvider
    })
    await expect(client.deleteRetainedRecovery({
      ...baseInput,
      confirmation: 'DELETE_RETAINED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
    })).resolves.toEqual({ schemaVersion: 1, status: 'COMPLETED', migrationId })

    expect(runtimeRequest.mock.calls.map(([path]) => path)).toEqual([
      '/v1/provider-registry/_private/legacy-migration-recovery/remigrate',
      '/v1/provider-registry/_private/legacy-migration-recovery/protected-delete'
    ])
    for (const [, , body] of runtimeRequest.mock.calls) {
      expect(body).not.toContain(Buffer.from(expectedCleanedSource).toString('utf8'))
      expect(body).toContain(Buffer.from(expectedCleanedSource).toString('base64'))
    }
  })

  it('discovers only bounded key-free recovery descriptors through the fixed private inventory route', async () => {
    const descriptor = {
      migrationId,
      providerId: provider.id,
      sourceLocator,
      sourceSHA256,
      expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      recoveryCredentialRef,
      phase: 'provider-committed-recovery-retained',
      commitOrder: '7'
    }
    const runtimeRequest = vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({ schemaVersion: 1, recoveries: [descriptor] })
    }))
    const client = createProviderRegistryLegacyMigrationClient(runtimeRequest)

    await expect(client.inventory()).resolves.toEqual({
      schemaVersion: 1,
      recoveries: [descriptor]
    })
    expect(runtimeRequest).toHaveBeenCalledWith(
      '/v1/provider-registry/_private/legacy-migration-recovery/inventory',
      'POST',
      JSON.stringify({
        schemaVersion: 1,
        confirmation: 'INSPECT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERIES'
      })
    )

    for (const forbidden of [
      { ...descriptor, credential: { valueBase64: 'c3ludGhldGlj' } },
      { ...descriptor, sourceSnapshot: 'synthetic' },
      { ...descriptor, provider },
      { ...descriptor, path: '/private/settings.json' }
    ]) {
      const invalid = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
        ok: true,
        status: 200,
        body: JSON.stringify({ schemaVersion: 1, recoveries: [forbidden] })
      })))
      await expect(invalid.inventory()).resolves.toMatchObject({
        error: { code: 'invalid_response' }
      })
    }
  })

  it('accepts the complete 256-entry private inventory without widening ordinary response bounds', async () => {
    const descriptors = Array.from({ length: 256 }, (_, index) => {
      const suffix = index.toString().padStart(3, '0')
      const migrationPrefix = `migration-inventory-${suffix}-`
      const providerPrefix = `provider-inventory-${suffix}-`
      return {
        migrationId: `${migrationPrefix}${'m'.repeat(96 - migrationPrefix.length)}`,
        providerId: `${providerPrefix}${'p'.repeat(96 - providerPrefix.length)}`,
        sourceLocator: 'compatibility:99:analytix-settings.json',
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256,
        recoveryCredentialRef: `cred_${suffix}${'R'.repeat(40)}`,
        phase: 'provider-committed-recovery-retained',
        commitOrder: String(index)
      }
    })
    const body = JSON.stringify({ schemaVersion: 1, recoveries: descriptors })
    expect(Buffer.byteLength(body, 'utf8')).toBeGreaterThan(64 << 10)
    const client = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: true,
      status: 200,
      body
    })))

    await expect(client.inventory()).resolves.toEqual({
      schemaVersion: 1,
      recoveries: descriptors
    })

    const overEntryLimit = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: JSON.stringify({ schemaVersion: 1, recoveries: [...descriptors, {
        ...descriptors[255],
        migrationId: `migration-over-limit-${'z'.repeat(75)}`,
        providerId: `provider-over-limit-${'z'.repeat(76)}`,
        recoveryCredentialRef: `cred_999${'R'.repeat(40)}`,
        commitOrder: '256'
      }] })
    })))
    await expect(overEntryLimit.inventory()).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })

    const inventoryCeiling = 256 << 10
    const oneByteOverBody = `${body}${' '.repeat(inventoryCeiling + 1 - Buffer.byteLength(body, 'utf8'))}`
    const oneByteOver = createProviderRegistryLegacyMigrationClient(vi.fn(async () => ({
      ok: true,
      status: 200,
      body: oneByteOverBody
    })))
    await expect(oneByteOver.inventory()).resolves.toMatchObject({
      error: { code: 'invalid_response' }
    })
  })
})
