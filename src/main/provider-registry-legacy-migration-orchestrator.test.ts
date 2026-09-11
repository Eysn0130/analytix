import { createHash } from 'node:crypto'
import { mkdtemp, readFile, readdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { atomicWriteFile } from '../../packages/runtime/src/adapters/file/atomic-write'
import type {
  ProviderRegistryProviderInputV1,
  ProviderRegistryPublicProviderV1
} from '../../packages/runtime/src/contracts/provider-registry'
import {
  JsonSettingsStore,
  mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1,
  type LegacyProviderCredentialCandidateSnapshot,
  type LegacyProviderCredentialInspection,
  type LegacyProviderCredentialLiveSourceAuthorityV1,
  type LegacyProviderCredentialSourceAuthorityRequestV1
} from './settings-store'
import {
  createProviderRegistryLegacyMigrationClient,
  type ProviderRegistryLegacyMigrationRecoveryDescriptor,
  ProviderRegistryLegacyMigrationCommitRequest,
  ProviderRegistryLegacyMigrationFinalizeRequest,
  ProviderRegistryLegacyMigrationPrepareRequest,
  ProviderRegistryLegacyMigrationRollbackBeginRequest,
  ProviderRegistryLegacyMigrationRollbackCommitRequest
} from './provider-registry-legacy-migration'
import {
  orchestrateProviderRegistryLegacyMigration,
  orchestrateProviderRegistryLegacyMigrationRollback,
  orchestrateProviderRegistryLegacyMigrationRollbackGroup,
  finalizeProviderRegistryLegacyMigrationRecovery
} from './provider-registry-legacy-migration-orchestrator'

const registryIncarnation = `inc_${'R'.repeat(43)}`
const providerIncarnation = `inc_${'P'.repeat(43)}`

function recoveryRef(marker: string): string {
  return `cred_${marker.repeat(43)}`
}

function providerProjection(
  input: ProviderRegistryProviderInputV1,
  overrides: Partial<ProviderRegistryPublicProviderV1> = {}
): ProviderRegistryPublicProviderV1 {
  return {
    id: input.id,
    kind: input.kind,
    endpoint: input.endpoint,
    ...(input.proxy ? { proxy: input.proxy } : {}),
    models: [...input.models],
    mediaModels: [...input.mediaModels],
    ...(input.selectedModel ? { selectedModel: input.selectedModel } : {}),
    ...(input.selectedMediaModel ? { selectedMediaModel: input.selectedMediaModel } : {}),
    selectedRoutes: [...input.selectedRoutes],
    credentialConfigured: true,
    credentialPurpose: 'provider-api-key',
    revision: '1',
    generation: '1',
    incarnation: providerIncarnation,
    tombstone: false,
    ...overrides
  }
}

function snapshot(
  registryRevision: string,
  providers: ProviderRegistryPublicProviderV1[] = [],
  selectedProviderId?: string
) {
  return {
    schemaVersion: 1 as const,
    registryRevision,
    registryIncarnation,
    ...(selectedProviderId ? { selectedProviderId } : {}),
    providers: [...providers].sort((left, right) => left.id < right.id ? -1 : left.id > right.id ? 1 : 0)
  }
}

function prepareVerified(migrationId: string, marker = 'A') {
  return {
    schemaVersion: 1 as const,
    status: 'VERIFIED_RECOVERY' as const,
    migrationId,
    recoveryCredentialRef: recoveryRef(marker),
    safeToProceedWithProviderMigration: true as const
  }
}

function prepareRetained(migrationId: string, marker = 'B') {
  return {
    schemaVersion: 1 as const,
    status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED' as const,
    migrationId,
    recoveryCredentialRef: recoveryRef(marker),
    safeToProceedWithProviderMigration: false as const
  }
}

function commitSuccess(migrationId: string, provider: ProviderRegistryPublicProviderV1) {
  return {
    schemaVersion: 1 as const,
    status: 'PROVIDER_COMMITTED_RECOVERY_RETAINED' as const,
    migrationId,
    safeToRemoveLegacyPlaintext: true as const,
    provider
  }
}

function syntheticCandidate(
  providerId: string,
  index: number,
  activeProviderId = ''
): LegacyProviderCredentialCandidateSnapshot {
  const marker = new TextEncoder().encode(`synthetic-${providerId}-marker`)
  const sourceLocator = `current:analytix-settings.json:provider.providers[${index}].apiKey`
  return {
    schemaVersion: 1,
    migrationId: `migration-${providerId}`,
    sourceLocator,
    credentialLocators: [sourceLocator],
    providerId,
    providerMetadata: {
      activeProviderId,
      runtimeModel: `${providerId}-model`,
      proxy: { enabled: false, url: '' },
      profile: {
        id: providerId,
        name: providerId,
        baseUrl: `https://${providerId}.example/v1`,
        endpointFormat: 'chat_completions',
        models: [`${providerId}-model`],
        modelProfiles: {}
      }
    },
    credential: marker,
    rollbackCredentialArtifacts: []
  }
}

function syntheticInspection(
  candidates: LegacyProviderCredentialCandidateSnapshot[]
): LegacyProviderCredentialInspection {
  const sourceSnapshot = new TextEncoder().encode('synthetic source snapshot')
  const cleanedSource = new TextEncoder().encode('synthetic key-free source representation')
  return {
    schemaVersion: 1,
    sourceLocator: 'current:analytix-settings.json',
    sourceSHA256: createHash('sha256').update(sourceSnapshot).digest('hex'),
    expectedCleanedSourceSHA256: createHash('sha256').update(cleanedSource).digest('hex'),
    sourcePhysicalIdentitySHA256: 'c'.repeat(64),
    sourceSnapshot,
    candidates
  }
}

const testSourceAuthorityChallenge = `lmsa_${'A'.repeat(43)}`
const testSourceAuthorityProof = {
  challenge: testSourceAuthorityChallenge,
  sourcePath: '/private/synthetic-analytix-settings.json',
  lockOwnerToken: '00000000-0000-4000-8000-000000000001',
  sourceDevice: '1',
  sourceInode: '2'
}

function testSourceAuthorityMethod(getCurrentSource: () => Uint8Array | null) {
  return async <T>(
    input: LegacyProviderCredentialSourceAuthorityRequestV1,
    action: (authority: LegacyProviderCredentialLiveSourceAuthorityV1) => Promise<T>
  ): Promise<T> => {
    const currentSource = getCurrentSource()
    if (!currentSource || input.challenge !== testSourceAuthorityChallenge ||
      createHash('sha256').update(currentSource).digest('hex') !== input.currentSourceSHA256) {
      throw new Error('synthetic source authority mismatch')
    }
    const authority = {
      schemaVersion: 1 as const,
      sourceLocator: input.sourceLocator,
      sourceSHA256: input.sourceSHA256,
      verifiedSourceSHA256: input.currentSourceSHA256,
      sourcePhysicalIdentitySHA256: input.sourcePhysicalIdentitySHA256,
      verifiedSource: Uint8Array.from(currentSource),
      sourceAuthority: { ...testSourceAuthorityProof, challenge: input.challenge }
    }
    try {
      return await action(authority)
    } finally {
      authority.verifiedSource.fill(0)
    }
  }
}

function fixedInspectionStore(inspection: LegacyProviderCredentialInspection) {
  const cleanedSource = new TextEncoder().encode('synthetic key-free source representation')
  let currentSource = inspection.sourceSnapshot instanceof Uint8Array
    ? Uint8Array.from(inspection.sourceSnapshot)
    : null
  return {
    inspectLegacyProviderCredentialSources: async () => inspection,
    cleanupLegacyProviderCredentialSource: async () => {
      currentSource = Uint8Array.from(cleanedSource)
      return {
        schemaVersion: 1 as const,
        sourceLocator: inspection.sourceLocator as string,
        sourceSHA256: inspection.sourceSHA256 as string,
        cleanedSourceSHA256: inspection.expectedCleanedSourceSHA256 as string,
        verifiedSourceSHA256: inspection.expectedCleanedSourceSHA256 as string,
        sourcePhysicalIdentitySHA256: inspection.sourcePhysicalIdentitySHA256 as string,
        verifiedSource: Uint8Array.from(cleanedSource)
      }
    },
    withLegacyProviderCredentialSourceAuthority: testSourceAuthorityMethod(() => currentSource)
  }
}

function completeMigrationClient<
  Prepare extends (input: ProviderRegistryLegacyMigrationPrepareRequest) => Promise<unknown>,
  Commit extends (input: ProviderRegistryLegacyMigrationCommitRequest) => Promise<unknown>
>(
  prepare: Prepare,
  commit: Commit
) {
  return {
    prepare,
    commit,
    inventory: async () => ({ schemaVersion: 1 as const, recoveries: [] }),
    issueSourceAuthorityChallenge: async () => ({
      schemaVersion: 1 as const,
      challenge: testSourceAuthorityChallenge
    }),
    finalize: async (input: ProviderRegistryLegacyMigrationFinalizeRequest) => ({
      schemaVersion: 1 as const,
      status: 'COMPLETED' as const,
      outcome: 'MIGRATION_COMMITTED' as const,
      migrationId: input.migrationId
    })
  }
}

function completeRollbackClient<
  Begin extends (input: ProviderRegistryLegacyMigrationRollbackBeginRequest) => Promise<unknown>,
  Commit extends (input: ProviderRegistryLegacyMigrationRollbackCommitRequest) => Promise<unknown>
>(beginRollback: Begin, commitRollback: Commit) {
  return {
    beginRollback,
    commitRollback,
    issueSourceAuthorityChallenge: async () => ({
      schemaVersion: 1 as const,
      challenge: testSourceAuthorityChallenge
    })
  }
}

function committedWinner(
  candidate: LegacyProviderCredentialCandidateSnapshot,
  overrides: Partial<ProviderRegistryPublicProviderV1> = {}
): ProviderRegistryPublicProviderV1 {
  return providerProjection(
    mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(candidate),
    overrides
  )
}

function expectCredentialsCleared(inspection: LegacyProviderCredentialInspection): void {
  if (inspection.sourceSnapshot) {
    expect([...inspection.sourceSnapshot]).toEqual(Array(inspection.sourceSnapshot.byteLength).fill(0))
  }
  for (const candidate of inspection.candidates) {
    expect([...candidate.credential]).toEqual(Array(candidate.credential.byteLength).fill(0))
    for (const artifact of candidate.rollbackCredentialArtifacts) {
      expect([...artifact.credential]).toEqual(Array(artifact.credential.byteLength).fill(0))
    }
  }
}

describe('Provider Registry legacy migration orchestrator', () => {
  it('removes plaintext only after every protected Provider migration is retained and preserves it on failure', async () => {
    const createSource = async (prefix: string) => {
      const userDataDir = await mkdtemp(join(tmpdir(), prefix))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      const sourceValue = {
        version: 1,
        provider: {
          activeProviderId: 'custom-cleanup',
          apiKey: 'synthetic-cleanup-deepseek-marker',
          unknownProviderMetadata: { preserved: true },
          providers: [
            {
              id: 'deepseek',
              name: 'DeepSeek',
              apiKey: 'synthetic-cleanup-deepseek-marker',
              baseUrl: 'https://deepseek-cleanup.example/v1',
              models: ['deepseek-cleanup-model']
            },
            {
              id: 'custom-cleanup',
              name: 'Synthetic Cleanup',
              apiKey: 'synthetic-cleanup-custom-marker',
              baseUrl: 'https://custom-cleanup.example/v1',
              models: ['custom-cleanup-model'],
              unknownProfileMetadata: { preserved: 'exactly' }
            },
            {
              id: 'custom-cleanup',
              name: 'Synthetic Cleanup',
              apiKey: '[REDACTED]',
              baseUrl: 'https://custom-cleanup.example/v1',
              models: ['custom-cleanup-model'],
              unknownProfileMetadata: { preserved: 'exactly' }
            }
          ]
        },
        unknownRootMetadata: { preserved: ['byte-for-byte semantics'] }
      }
      const source = Buffer.from(JSON.stringify(sourceValue), 'utf8')
      await writeFile(settingsPath, source)
      return { userDataDir, settingsPath, source, sourceValue }
    }

    const successful = await createSource('analytix-provider-registry-source-cleanup-success-')
    const successfulStore = new JsonSettingsStore(successful.userDataDir)
    const successfulInspections: LegacyProviderCredentialInspection[] = []
    const successfulSettingsStore = {
      inspectLegacyProviderCredentialSources: async () => {
        const inspection = await successfulStore.inspectLegacyProviderCredentialSources()
        successfulInspections.push(inspection)
        return inspection
      },
      cleanupLegacyProviderCredentialSource: successfulStore.cleanupLegacyProviderCredentialSource.bind(successfulStore),
      withLegacyProviderCredentialSourceAuthority:
        successfulStore.withLegacyProviderCredentialSourceAuthority.bind(successfulStore)
    }
    const successfulPrepareInputs: ProviderRegistryLegacyMigrationPrepareRequest[] = []
    const successfulSnapshot = vi.fn(async () => {
      const call = successfulSnapshot.mock.calls.length
      const inspection = successfulInspections[0]
      const custom = inspection.candidates.find((candidate) => candidate.providerId === 'custom-cleanup')!
      const deepseek = inspection.candidates.find((candidate) => candidate.providerId === 'deepseek')!
      return call === 1
        ? snapshot('0')
        : snapshot('1', [
            committedWinner(custom),
            committedWinner(deepseek)
          ], 'custom-cleanup')
    })
    const successfulPrepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
      successfulPrepareInputs.push(input)
      return input.provider.id === 'custom-cleanup'
        ? prepareVerified(input.migrationId, 'C')
        : prepareRetained(input.migrationId, 'D')
    })
    const successfulCommit = vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) =>
      commitSuccess(input.migrationId, providerProjection(successfulPrepareInputs[0].provider)))

    const successfulResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: successfulSettingsStore,
      readRegistrySnapshot: successfulSnapshot,
      migrationClient: completeMigrationClient(successfulPrepare, successfulCommit)
    })

    expect(successfulResult).toEqual({
      schemaVersion: 1,
      status: 'COMPLETED',
      candidateCount: 2,
      completedCandidateCount: 2,
      committedCandidateCount: 1,
      retainedReplayCount: 1
    })
    const cleaned = JSON.parse(await readFile(successful.settingsPath, 'utf8'))
    const { apiKey: _topLevelApiKey, ...expectedProvider } = successful.sourceValue.provider
    expect(cleaned).toEqual({
      ...successful.sourceValue,
      provider: {
        ...expectedProvider,
        providers: successful.sourceValue.provider.providers.map(({ apiKey: _apiKey, ...provider }) => provider)
      }
    })
    expect(cleaned.provider).not.toHaveProperty('apiKey')
    expect(successfulCommit).toHaveBeenCalledTimes(1)
    expect(successfulInspections).toHaveLength(1)
    expectCredentialsCleared(successfulInspections[0])

    const failed = await createSource('analytix-provider-registry-source-cleanup-failure-')
    const failedStore = new JsonSettingsStore(failed.userDataDir)
    const failedInspections: LegacyProviderCredentialInspection[] = []
    const failedSettingsStore = {
      inspectLegacyProviderCredentialSources: async () => {
        const inspection = await failedStore.inspectLegacyProviderCredentialSources()
        failedInspections.push(inspection)
        return inspection
      },
      cleanupLegacyProviderCredentialSource: failedStore.cleanupLegacyProviderCredentialSource.bind(failedStore),
      withLegacyProviderCredentialSourceAuthority:
        failedStore.withLegacyProviderCredentialSourceAuthority.bind(failedStore)
    }
    let failedSnapshotCalls = 0
    const failedPrepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
      failedSnapshotCalls += 1
      return failedSnapshotCalls === 1
        ? prepareVerified(input.migrationId, 'E')
        : { schemaVersion: 1, error: { code: 'conflict', message: 'The provider registry state has changed.' } }
    })
    const failedCommit = vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) =>
      commitSuccess(input.migrationId, providerProjection(
        mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(
          failedInspections[0].candidates.find((candidate) => candidate.providerId === 'custom-cleanup')!
        )
      )))

    const failedResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: failedSettingsStore,
      readRegistrySnapshot: async () => snapshot(String(failedSnapshotCalls)),
      migrationClient: completeMigrationClient(failedPrepare, failedCommit)
    })

    expect(failedResult).toMatchObject({
      status: 'FAILED',
      failurePhase: 'prepare',
      completedCandidateCount: 1
    })
    expect(await readFile(failed.settingsPath)).toEqual(failed.source)
    expect(failedInspections).toHaveLength(1)
    expectCredentialsCleared(failedInspections[0])
  })

  it('migrates the active legacy Provider first and never recommits retained recovery before cleaning the source', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-legacy-orchestrator-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const source = Buffer.from(JSON.stringify({
      version: 1,
      runtime: { model: 'active-model' },
      provider: {
        activeProviderId: 'provider-active',
        providers: [
          {
            id: 'deepseek',
            apiKey: 'synthetic-deepseek-active-marker',
            baseUrl: 'https://deepseek.example/v1',
            models: ['deepseek-model']
          },
          {
            id: 'provider-active',
            apiKey: 'synthetic-provider-active-marker',
            baseUrl: 'https://active.example/v1',
            models: ['active-model']
          },
          {
            id: 'provider-active',
            apiKey: 'synthetic-provider-shadow-marker',
            baseUrl: 'https://active.example/v1',
            models: ['active-model']
          },
          {
            id: 'provider-active',
            apiKey: 'synthetic-provider-second-shadow-marker',
            baseUrl: 'https://active.example/v1',
            models: ['active-model']
          }
        ]
      }
    }), 'utf8')
    await writeFile(settingsPath, source)
    const beforeEntries = await readdir(userDataDir)
    const sourceSHA256 = createHash('sha256').update(source).digest('hex')
    const realStore = new JsonSettingsStore(userDataDir)
    let inspected: LegacyProviderCredentialInspection | undefined
    const settingsStore = {
      inspectLegacyProviderCredentialSources: async () => {
        inspected = await realStore.inspectLegacyProviderCredentialSources()
        return inspected
      },
      cleanupLegacyProviderCredentialSource: realStore.cleanupLegacyProviderCredentialSource.bind(realStore),
      withLegacyProviderCredentialSourceAuthority:
        realStore.withLegacyProviderCredentialSourceAuthority.bind(realStore)
    }
    const deepseekInput: ProviderRegistryProviderInputV1 = {
      id: 'deepseek',
      kind: 'deepseek',
      endpoint: 'https://deepseek.example/v1',
      proxy: '',
      models: ['deepseek-model'],
      mediaModels: [],
      selectedModel: '',
      selectedMediaModel: '',
      selectedRoutes: []
    }
    const existingDeepseek = providerProjection(deepseekInput)
    const events: string[] = []
    const readRegistrySnapshot = vi.fn(async () => {
      const call = readRegistrySnapshot.mock.calls.length
      events.push(`snapshot-${call}`)
      return call === 1
        ? snapshot('7', [existingDeepseek], 'deepseek')
        : snapshot('8', [existingDeepseek, providerProjection({
            id: 'provider-active',
            kind: 'openai-compatible',
            endpoint: 'https://active.example/v1',
            proxy: '',
            models: ['active-model'],
            mediaModels: [],
            selectedModel: 'active-model',
            selectedMediaModel: '',
            selectedRoutes: []
          })], 'deepseek')
    })
    const prepareInputs: ProviderRegistryLegacyMigrationPrepareRequest[] = []
    const nonZeroCredentialObservations: boolean[] = []
    const migrationClient = completeMigrationClient(
      vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
        events.push(`prepare-${input.provider.id}`)
        prepareInputs.push(input)
        nonZeroCredentialObservations.push(input.credential.some((value) => value !== 0))
        return input.provider.id === 'provider-active'
          ? prepareVerified(input.migrationId, 'A')
          : prepareRetained(input.migrationId, 'B')
      }),
      vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) => {
        events.push('commit-provider-active')
        return commitSuccess(input.migrationId, providerProjection(prepareInputs[0].provider))
      })
    )

    const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore,
      readRegistrySnapshot,
      migrationClient
    })

    expect(orchestrationResult).toEqual({
      schemaVersion: 1,
      status: 'COMPLETED',
      candidateCount: 2,
      completedCandidateCount: 2,
      committedCandidateCount: 1,
      retainedReplayCount: 1
    })
    expect(events).toEqual([
      'snapshot-1',
      'prepare-provider-active',
      'commit-provider-active',
      'snapshot-2',
      'prepare-deepseek'
    ])
    expect(nonZeroCredentialObservations).toEqual([true, true])
    expect(migrationClient.commit).toHaveBeenCalledTimes(1)
    expect(readRegistrySnapshot).toHaveBeenCalledTimes(2)
    expect(prepareInputs.map((input) => input.provider.id)).toEqual(['provider-active', 'deepseek'])
    expect(prepareInputs[0]).toMatchObject({
      schemaVersion: 1,
      expected: {
        registryRevision: '7',
        registryIncarnation,
        providerRevision: '0',
        providerGeneration: '0',
        providerIncarnation: '',
        providerCredentialPurpose: ''
      },
      sourceSHA256,
      provider: {
        id: 'provider-active',
        selectedModel: 'active-model'
      },
      credentialPurpose: 'provider-api-key',
      activeCredentialLocators: [
        'current:analytix-settings.json:provider.providers[1].apiKey'
      ],
      rollbackCredentialArtifacts: [
        {
          schemaVersion: 1,
          credentialLocators: [
            'current:analytix-settings.json:provider.providers[2].apiKey'
          ]
        },
        {
          schemaVersion: 1,
          credentialLocators: [
            'current:analytix-settings.json:provider.providers[3].apiKey'
          ]
        }
      ]
    })
    expect(prepareInputs[1].expected).toEqual({
      registryRevision: '8',
      registryIncarnation,
      providerRevision: '1',
      providerGeneration: '1',
      providerIncarnation,
      providerCredentialPurpose: 'provider-api-key'
    })
    const commitInput = migrationClient.commit.mock.calls[0][0]
    expect(commitInput.expected).toBe(prepareInputs[0].expected)
    expect(commitInput).toEqual({
      schemaVersion: 1,
      expected: prepareInputs[0].expected,
      expectedSelectedProviderID: 'deepseek',
      migrationId: prepareInputs[0].migrationId,
      sourceLocator: 'current:analytix-settings.json',
      sourceSHA256,
      recoveryCredentialRef: recoveryRef('A'),
      confirmation: 'COMMIT_VERIFIED_PROVIDER_SETTINGS_MIGRATION_RECOVERY'
    })
    const cleanedSource = JSON.parse(await readFile(settingsPath, 'utf8'))
    expect(cleanedSource.provider).not.toHaveProperty('apiKey')
    expect(cleanedSource.provider.providers).toHaveLength(4)
    for (const provider of cleanedSource.provider.providers) expect(provider).not.toHaveProperty('apiKey')
    expect(await readdir(userDataDir)).toEqual(beforeEntries)
    expect(inspected).toBeDefined()
    expectCredentialsCleared(inspected!)
    expect(JSON.stringify(orchestrationResult)).not.toMatch(
      /(?:apiKey|credentialRef|sourceSHA256|locator|provider-active|deepseek\.example|synthetic-)/i
    )
  })

  it('fails closed on first and later candidate prepare failures without touching later candidates', async () => {
    for (const failAt of [0, 1]) {
      const inspection = syntheticInspection([
        syntheticCandidate('alpha', 0),
        syntheticCandidate('beta', 1),
        syntheticCandidate('gamma', 2)
      ])
      let prepareCall = 0
      const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
        const current = prepareCall
        prepareCall += 1
        return current === failAt
          ? { schemaVersion: 1, error: { code: 'conflict', message: 'The provider registry state has changed.' } }
          : prepareRetained(input.migrationId)
      })
      const commit = vi.fn(async () => undefined)
      const readRegistrySnapshot = vi.fn(async () => snapshot(
        String(prepareCall),
        [committedWinner(inspection.candidates[prepareCall])]
      ))

      const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: fixedInspectionStore(inspection),
        readRegistrySnapshot,
        migrationClient: completeMigrationClient(prepare, commit)
      })

      expect(orchestrationResult).toMatchObject({
        status: 'FAILED',
        failurePhase: 'prepare',
        completedCandidateCount: failAt,
        committedCandidateCount: 0,
        retainedReplayCount: failAt
      })
      expect(prepare).toHaveBeenCalledTimes(failAt + 1)
      expect(readRegistrySnapshot).toHaveBeenCalledTimes(failAt + 1)
      expect(commit).not.toHaveBeenCalled()
      expectCredentialsCleared(inspection)
    }
  })

  it('reports a key-free cleanup failure only after every retained candidate completes', async () => {
    const inspection = syntheticInspection([syntheticCandidate('alpha', 0)])
    const cleanup = vi.fn(async (
      input: Parameters<JsonSettingsStore['cleanupLegacyProviderCredentialSource']>[0]
    ) => {
      expect(input).toEqual({
        schemaVersion: 1,
        purpose: 'remove-verified-legacy-provider-credentials',
        confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
        sourceLocator: 'current:analytix-settings.json',
        sourceSHA256: 'a'.repeat(64),
        protectedCredentialLocators: [
          'current:analytix-settings.json:provider.providers[0].apiKey'
        ]
      })
      expect(JSON.stringify(input)).not.toContain('synthetic-alpha-marker')
      throw new Error('private cleanup path and body')
    })
    const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) =>
      prepareRetained(input.migrationId))
    const commit = vi.fn(async () => undefined)

    const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: {
        inspectLegacyProviderCredentialSources: async () => inspection,
        cleanupLegacyProviderCredentialSource: cleanup,
        withLegacyProviderCredentialSourceAuthority: testSourceAuthorityMethod(() => null)
      },
      readRegistrySnapshot: async () => snapshot('1', [committedWinner(inspection.candidates[0])]),
      migrationClient: completeMigrationClient(prepare, commit)
    })

    expect(orchestrationResult).toEqual({
      schemaVersion: 1,
      status: 'FAILED',
      candidateCount: 1,
      completedCandidateCount: 1,
      committedCandidateCount: 0,
      retainedReplayCount: 1,
      failurePhase: 'cleanup'
    })
    expect(cleanup).toHaveBeenCalledTimes(1)
    expect(prepare).toHaveBeenCalledTimes(1)
    expect(commit).not.toHaveBeenCalled()
    expectCredentialsCleared(inspection)
    expect(JSON.stringify(orchestrationResult)).not.toMatch(/(?:synthetic-|locator|sourceSHA256|credentialRef)/i)
  })

  it('stops after snapshot, prepare, and commit exceptions or failures and clears every returned buffer', async () => {
    const cases: Array<{
      name: string
      expectedPhase: string
      readRegistrySnapshot: () => Promise<unknown>
      prepare: (input: ProviderRegistryLegacyMigrationPrepareRequest) => Promise<unknown>
      commit: (input: ProviderRegistryLegacyMigrationCommitRequest) => Promise<unknown>
    }> = [
      {
        name: 'snapshot failure',
        expectedPhase: 'snapshot',
        readRegistrySnapshot: async () => { throw new Error('private snapshot failure') },
        prepare: async (input) => prepareRetained(input.migrationId),
        commit: async () => undefined
      },
      {
        name: 'prepare exception',
        expectedPhase: 'prepare',
        readRegistrySnapshot: async () => snapshot('0'),
        prepare: async () => { throw new Error('private prepare failure') },
        commit: async () => undefined
      },
      {
        name: 'commit failure',
        expectedPhase: 'commit',
        readRegistrySnapshot: async () => snapshot('0'),
        prepare: async (input) => prepareVerified(input.migrationId),
        commit: async () => ({
          schemaVersion: 1,
          error: { code: 'conflict', message: 'The provider registry state has changed.' }
        })
      },
      {
        name: 'commit exception',
        expectedPhase: 'commit',
        readRegistrySnapshot: async () => snapshot('0'),
        prepare: async (input) => prepareVerified(input.migrationId),
        commit: async () => { throw new Error('private commit failure') }
      }
    ]

    for (const testCase of cases) {
      const inspection = syntheticInspection([
        syntheticCandidate('alpha', 0),
        syntheticCandidate('beta', 1)
      ])
      const readRegistrySnapshot = vi.fn(testCase.readRegistrySnapshot)
      const prepare = vi.fn(testCase.prepare)
      const commit = vi.fn(testCase.commit)
      const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: fixedInspectionStore(inspection),
        readRegistrySnapshot,
        migrationClient: completeMigrationClient(prepare, commit)
      })

      expect(orchestrationResult, testCase.name).toMatchObject({
        status: 'FAILED',
        failurePhase: testCase.expectedPhase,
        completedCandidateCount: 0
      })
      expect(readRegistrySnapshot, testCase.name).toHaveBeenCalledTimes(1)
      expect(prepare, testCase.name).toHaveBeenCalledTimes(testCase.expectedPhase === 'snapshot' ? 0 : 1)
      expect(commit, testCase.name).toHaveBeenCalledTimes(testCase.expectedPhase === 'commit' ? 1 : 0)
      expectCredentialsCleared(inspection)
    }

    const secondSnapshotInspection = syntheticInspection([
      syntheticCandidate('alpha', 0),
      syntheticCandidate('beta', 1),
      syntheticCandidate('gamma', 2)
    ])
    let snapshotCalls = 0
    const secondSnapshotReader = vi.fn(async () => {
      snapshotCalls += 1
      if (snapshotCalls === 2) throw new Error('private second snapshot failure')
      return snapshot(
        String(snapshotCalls - 1),
        [committedWinner(secondSnapshotInspection.candidates[0])]
      )
    })
    const secondSnapshotPrepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) =>
      prepareRetained(input.migrationId))
    const secondSnapshotResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: fixedInspectionStore(secondSnapshotInspection),
      readRegistrySnapshot: secondSnapshotReader,
      migrationClient: completeMigrationClient(secondSnapshotPrepare, async () => undefined)
    })
    expect(secondSnapshotResult).toMatchObject({
      status: 'FAILED',
      failurePhase: 'snapshot',
      completedCandidateCount: 1,
      retainedReplayCount: 1
    })
    expect(secondSnapshotReader).toHaveBeenCalledTimes(2)
    expect(secondSnapshotPrepare).toHaveBeenCalledTimes(1)
    expectCredentialsCleared(secondSnapshotInspection)
  })

  it('rejects malformed and mismatched prepare or commit output without later transport', async () => {
    const malformedCases: Array<{
      name: string
      snapshotValue: ReturnType<typeof snapshot>
      prepareResult: (input: ProviderRegistryLegacyMigrationPrepareRequest) => unknown
      commitResult: (input: ProviderRegistryLegacyMigrationCommitRequest, provider: ProviderRegistryProviderInputV1) => unknown
      expectedPhase: 'prepare' | 'commit'
      expectedCommitCalls: number
    }> = [
      {
        name: 'mismatched prepare migration',
        snapshotValue: snapshot('0'),
        prepareResult: () => prepareVerified('migration-other'),
        commitResult: () => undefined,
        expectedPhase: 'prepare',
        expectedCommitCalls: 0
      },
      {
        name: 'verified prepare for existing provider',
        snapshotValue: snapshot('1', [providerProjection({
          id: 'alpha',
          kind: 'openai-compatible',
          endpoint: 'https://alpha.example/v1',
          proxy: '',
          models: ['alpha-model'],
          mediaModels: [],
          selectedModel: 'alpha-model',
          selectedMediaModel: '',
          selectedRoutes: []
        })], 'alpha'),
        prepareResult: (input) => prepareVerified(input.migrationId),
        commitResult: () => undefined,
        expectedPhase: 'prepare',
        expectedCommitCalls: 0
      },
      {
        name: 'mismatched committed provider',
        snapshotValue: snapshot('0'),
        prepareResult: (input) => prepareVerified(input.migrationId),
        commitResult: (input, provider) => commitSuccess(
          input.migrationId,
          providerProjection({ ...provider, endpoint: 'https://different.example/v1' })
        ),
        expectedPhase: 'commit',
        expectedCommitCalls: 1
      }
    ]

    for (const testCase of malformedCases) {
      const inspection = syntheticInspection([syntheticCandidate('alpha', 0)])
      let preparedProvider: ProviderRegistryProviderInputV1 | undefined
      const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
        preparedProvider = input.provider
        return testCase.prepareResult(input)
      })
      const commit = vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) =>
        testCase.commitResult(input, preparedProvider!))
      const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: fixedInspectionStore(inspection),
        readRegistrySnapshot: async () => testCase.snapshotValue,
        migrationClient: completeMigrationClient(prepare, commit)
      })

      expect(orchestrationResult, testCase.name).toMatchObject({
        status: 'FAILED',
        failurePhase: testCase.expectedPhase
      })
      expect(commit, testCase.name).toHaveBeenCalledTimes(testCase.expectedCommitCalls)
      expectCredentialsCleared(inspection)
    }
  })

  it('rejects retained replay without the exact committed Provider winner in the pre-prepare snapshot', async () => {
    const cases: Array<{
      name: string
      providers: (candidate: LegacyProviderCredentialCandidateSnapshot) => ProviderRegistryPublicProviderV1[]
    }> = [
      { name: 'missing winner', providers: () => [] },
      {
        name: 'mismatched provider projection',
        providers: (candidate) => [committedWinner(candidate, {
          endpoint: 'https://different.example/v1'
        })]
      },
      {
        name: 'unconfigured credential',
        providers: (candidate) => [committedWinner(candidate, {
          credentialConfigured: false,
          credentialPurpose: undefined
        })]
      },
      {
        name: 'mismatched credential purpose',
        providers: (candidate) => [committedWinner(candidate, {
          credentialPurpose: 'different-purpose'
        })]
      },
      {
        name: 'tombstoned winner',
        providers: (candidate) => [committedWinner(candidate, {
          credentialConfigured: false,
          credentialPurpose: undefined,
          tombstone: true
        })]
      },
      {
        name: 'mismatched committed revision',
        providers: (candidate) => [committedWinner(candidate, { revision: '2' })]
      },
      {
        name: 'mismatched committed generation',
        providers: (candidate) => [committedWinner(candidate, { generation: '2' })]
      }
    ]

    for (const testCase of cases) {
      const inspection = syntheticInspection([
        syntheticCandidate('alpha', 0),
        syntheticCandidate('beta', 1)
      ])
      const readRegistrySnapshot = vi.fn(async () => snapshot(
        '1',
        testCase.providers(inspection.candidates[0])
      ))
      const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) =>
        prepareRetained(input.migrationId))
      const commit = vi.fn(async () => undefined)

      const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: fixedInspectionStore(inspection),
        readRegistrySnapshot,
        migrationClient: completeMigrationClient(prepare, commit)
      })

      expect(orchestrationResult, testCase.name).toMatchObject({
        status: 'FAILED',
        failurePhase: 'prepare',
        completedCandidateCount: 0,
        retainedReplayCount: 0
      })
      expect(readRegistrySnapshot, testCase.name).toHaveBeenCalledTimes(1)
      expect(prepare, testCase.name).toHaveBeenCalledTimes(1)
      expect(commit, testCase.name).not.toHaveBeenCalled()
      expectCredentialsCleared(inspection)
    }
  })

  it('repeats deterministically after cleanup without restoring plaintext or recommitting recovery', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-repeat-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const source = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-repeat-marker',
        baseUrl: 'https://repeat.example/v1'
      }
    }), 'utf8')
    await writeFile(settingsPath, source)
    const realStore = new JsonSettingsStore(userDataDir)
    const inspections: LegacyProviderCredentialInspection[] = []
    const settingsStore = {
      inspectLegacyProviderCredentialSources: async () => {
        const inspection = await realStore.inspectLegacyProviderCredentialSources()
        inspections.push(inspection)
        return inspection
      },
      cleanupLegacyProviderCredentialSource: realStore.cleanupLegacyProviderCredentialSource.bind(realStore),
      withLegacyProviderCredentialSourceAuthority:
        realStore.withLegacyProviderCredentialSourceAuthority.bind(realStore)
    }
    const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) =>
      prepareRetained(input.migrationId))
    const commit = vi.fn(async () => undefined)
    const readRegistrySnapshot = vi.fn(async () => snapshot(
      '1',
      [committedWinner(inspections.at(-1)!.candidates[0])]
    ))

    const first = await orchestrateProviderRegistryLegacyMigration({
      settingsStore,
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })
    const second = await orchestrateProviderRegistryLegacyMigration({
      settingsStore,
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })

    expect(first).toMatchObject({ status: 'COMPLETED', retainedReplayCount: 1 })
    expect(second).toEqual({
      schemaVersion: 1,
      status: 'COMPLETED',
      candidateCount: 0,
      completedCandidateCount: 0,
      committedCandidateCount: 0,
      retainedReplayCount: 0
    })
    expect(prepare).toHaveBeenCalledTimes(1)
    expect(commit).not.toHaveBeenCalled()
    expect(inspections).toHaveLength(2)
    inspections.forEach(expectCredentialsCleared)
    expect(await readFile(settingsPath, 'utf8')).not.toContain('synthetic-repeat-marker')
  })

  it.each([
    ['current', 'analytix-settings.json'],
    ['compatibility', 'kun-settings.json']
  ])('keeps the exact cleaned %s source key-free while committing explicit rollback', async (_kind, fileName) => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-explicit-rollback-'))
    const settingsPath = join(userDataDir, fileName)
    const source = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-orchestrated-rollback',
        providers: [{ id: 'deepseek', apiKey: '[REDACTED]' }]
      }
    }), 'utf8')
    await writeFile(settingsPath, source)
    const store = new JsonSettingsStore(userDataDir)
    const inspection = await store.inspectLegacyProviderCredentialSources()
    const candidate = inspection.candidates[0]
    await store.cleanupLegacyProviderCredentialSource({
      schemaVersion: 1,
      purpose: 'remove-verified-legacy-provider-credentials',
      confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
      sourceLocator: inspection.sourceLocator!,
      sourceSHA256: inspection.sourceSHA256!,
      expectedCleanedSourceSHA256: inspection.expectedCleanedSourceSHA256!,
      protectedCredentialLocators: [
        ...candidate.credentialLocators,
        ...candidate.rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
      ].sort()
    })
    const cleanedSource = await readFile(settingsPath)
    expect(cleanedSource).not.toEqual(source)
    expect(cleanedSource.toString('utf8')).not.toContain('synthetic-orchestrated-rollback')
    const beginRollback = vi.fn(async () => ({
      schemaVersion: 1,
      status: 'CLEANED_SOURCE_AUTHORITY_REQUIRED',
      migrationId: candidate.migrationId
    }))
    const commitRollback = vi.fn(async () => {
      expect(await readFile(settingsPath)).toEqual(cleanedSource)
      return {
        schemaVersion: 1,
        status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED',
        migrationId: candidate.migrationId
      }
    })
    expect('restoreLegacyProviderCredentialSource' in store).toBe(false)
    const result = await orchestrateProviderRegistryLegacyMigrationRollback({
      migrationId: candidate.migrationId,
      providerId: candidate.providerId,
      sourceLocator: inspection.sourceLocator!,
      sourceSHA256: inspection.sourceSHA256!,
      expectedCleanedSourceSHA256: inspection.expectedCleanedSourceSHA256!,
      sourcePhysicalIdentitySHA256: inspection.sourcePhysicalIdentitySHA256!,
      recoveryCredentialRef: recoveryRef('Z'),
      commitOrder: '1',
      settingsStore: store,
      readRegistrySnapshot: async () => snapshot('1', [committedWinner(candidate)], candidate.providerId),
      migrationClient: completeRollbackClient(beginRollback, commitRollback)
    })
    expect(result).toEqual({ schemaVersion: 1, status: 'ROLLBACK_COMMITTED' })
    expect(await readFile(settingsPath)).toEqual(cleanedSource)
    expect(beginRollback).toHaveBeenCalledTimes(1)
    expect(commitRollback).toHaveBeenCalledTimes(1)
  })

  it('keeps a shared source cleaned and resumes reverse-order multi-Provider rollback after interruption', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-group-rollback-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const source = Buffer.from(JSON.stringify({
      provider: {
        activeProviderId: 'provider-alpha',
        providers: [
          {
            id: 'provider-alpha',
            name: 'Provider Alpha',
            apiKey: 'synthetic-group-alpha-not-a-real-key',
            baseUrl: 'https://alpha.invalid/v1',
            models: ['alpha-model']
          },
          {
            id: 'provider-beta',
            name: 'Provider Beta',
            apiKey: 'synthetic-group-beta-not-a-real-key',
            baseUrl: 'https://beta.invalid/v1',
            models: ['beta-model']
          }
        ]
      }
    }), 'utf8')
    await writeFile(settingsPath, source)
    const store = new JsonSettingsStore(userDataDir)
    const inspection = await store.inspectLegacyProviderCredentialSources()
    const candidates = inspection.candidates
    expect(candidates.map((candidate) => candidate.providerId)).toEqual([
      'provider-alpha',
      'provider-beta'
    ])
    const sourceSHA256 = createHash('sha256').update(source).digest('hex')
    const targets = candidates.map((candidate, index) => ({
      migrationId: candidate.migrationId,
      providerId: candidate.providerId,
      recoveryCredentialRef: recoveryRef(index === 0 ? 'A' : 'B'),
      commitOrder: String(index + 1)
    }))
    await store.cleanupLegacyProviderCredentialSource({
      schemaVersion: 1,
      purpose: 'remove-verified-legacy-provider-credentials',
      confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
      sourceLocator: inspection.sourceLocator!,
      sourceSHA256,
      expectedCleanedSourceSHA256: inspection.expectedCleanedSourceSHA256!,
      protectedCredentialLocators: candidates.flatMap((candidate) => [
        ...candidate.credentialLocators,
        ...candidate.rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
      ]).sort()
    })
    const cleanedSource = await readFile(settingsPath)
    const expectedCleanedSourceSHA256 = createHash('sha256').update(cleanedSource).digest('hex')

    const committed = new Set<string>()
    const beginRollback = vi.fn(async (input: ProviderRegistryLegacyMigrationRollbackBeginRequest) => {
      return {
        schemaVersion: 1 as const,
        status: committed.has(input.migrationId)
          ? 'ROLLBACK_COMMITTED_RECOVERY_RETAINED' as const
          : 'CLEANED_SOURCE_AUTHORITY_REQUIRED' as const,
        migrationId: input.migrationId
      }
    })
    const registrySnapshots = [
      snapshot('2', candidates.map((candidate) => committedWinner(candidate)), 'provider-alpha'),
      snapshot('3', [committedWinner(candidates[0])], 'provider-alpha'),
      snapshot('3', [committedWinner(candidates[0])], 'provider-alpha')
    ]
    const readRegistrySnapshot = vi.fn(async () => registrySnapshots.shift())
    let interruptAlphaOnce = true
    const commitOrder: string[] = []
    const commitRollback = vi.fn(async (input: ProviderRegistryLegacyMigrationRollbackCommitRequest) => {
      commitOrder.push(input.migrationId)
      if (input.migrationId === targets[0].migrationId && interruptAlphaOnce) {
        interruptAlphaOnce = false
        throw new Error('synthetic interruption')
      }
      committed.add(input.migrationId)
      return {
        schemaVersion: 1 as const,
        status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED' as const,
        migrationId: input.migrationId
      }
    })
    expect('restoreLegacyProviderCredentialSource' in store).toBe(false)
    const groupInput = {
      recoveriesInCommitOrder: targets,
      sourceLocator: inspection.sourceLocator!,
      sourceSHA256,
      expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256: inspection.sourcePhysicalIdentitySHA256!,
      settingsStore: store,
      readRegistrySnapshot,
      migrationClient: completeRollbackClient(beginRollback, commitRollback)
    }

    expect(await orchestrateProviderRegistryLegacyMigrationRollbackGroup(groupInput)).toEqual({
      schemaVersion: 1,
      status: 'FAILED',
      failurePhase: 'commit'
    })
    expect(await readFile(settingsPath)).toEqual(cleanedSource)
    expect(commitOrder).toEqual([targets[1].migrationId, targets[0].migrationId])
    expect(committed).toEqual(new Set([targets[1].migrationId]))

    expect(await orchestrateProviderRegistryLegacyMigrationRollbackGroup(groupInput)).toEqual({
      schemaVersion: 1,
      status: 'ROLLBACK_COMMITTED'
    })
    expect(commitOrder).toEqual([
      targets[1].migrationId,
      targets[0].migrationId,
      targets[0].migrationId
    ])
    expect(committed).toEqual(new Set(targets.map((target) => target.migrationId)))

    const snapshotReadsBeforeRepeat = readRegistrySnapshot.mock.calls.length
    expect(await orchestrateProviderRegistryLegacyMigrationRollbackGroup(groupInput)).toEqual({
      schemaVersion: 1,
      status: 'ROLLBACK_COMMITTED'
    })
    expect(readRegistrySnapshot).toHaveBeenCalledTimes(snapshotReadsBeforeRepeat)
  })

  it('completes a mixed pre-commit and committed shared-source rollback in one invocation', async () => {
    const sourceLocator = 'current:analytix-settings.json'
    const sourceSnapshot = new TextEncoder().encode(JSON.stringify({
      provider: {
        providers: [{ id: 'provider-mixed-committed', apiKey: 'synthetic-mixed-committed' }]
      }
    }))
    const sourceSHA256 = createHash('sha256').update(sourceSnapshot).digest('hex')
    const cleanedSource = new TextEncoder().encode(JSON.stringify({
      version: 1,
      provider: { providers: [{ id: 'provider-mixed-committed' }] }
    }))
    const expectedCleanedSourceSHA256 = createHash('sha256').update(cleanedSource).digest('hex')
    const sourcePhysicalIdentitySHA256 = 'd'.repeat(64)
    const committedMigrationId = 'migration-mixed-committed'
    const preparedMigrationId = 'migration-mixed-prepared'
    const beginRollback = vi.fn(async (input: ProviderRegistryLegacyMigrationRollbackBeginRequest) => {
      if (input.migrationId === preparedMigrationId) {
        return {
          schemaVersion: 1 as const,
          status: 'PRE_COMMIT_ROLLBACK_COMPLETED' as const,
          migrationId: preparedMigrationId
        }
      }
      return {
        schemaVersion: 1 as const,
        status: 'CLEANED_SOURCE_AUTHORITY_REQUIRED' as const,
        migrationId: committedMigrationId
      }
    })
    const commitRollback = vi.fn(async () => ({
      schemaVersion: 1 as const,
      status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED' as const,
      migrationId: committedMigrationId
    }))
    const provider = providerProjection({
      id: 'provider-mixed-committed',
      kind: 'openai-compatible',
      endpoint: 'https://mixed-committed.invalid/v1',
      proxy: '',
      models: [],
      mediaModels: [],
      selectedModel: '',
      selectedMediaModel: '',
      selectedRoutes: []
    })

    const result = await orchestrateProviderRegistryLegacyMigrationRollbackGroup({
      recoveriesInCommitOrder: [
        {
          migrationId: committedMigrationId,
          providerId: provider.id,
          recoveryCredentialRef: recoveryRef('M'),
          commitOrder: '1'
        },
        {
          migrationId: preparedMigrationId,
          providerId: 'provider-mixed-prepared',
          recoveryCredentialRef: recoveryRef('N'),
          commitOrder: '2'
        }
      ],
      sourceLocator,
      sourceSHA256,
      expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      settingsStore: {
        withLegacyProviderCredentialSourceAuthority:
          testSourceAuthorityMethod(() => cleanedSource)
      },
      readRegistrySnapshot: async () => snapshot('2', [provider], provider.id),
      migrationClient: completeRollbackClient(beginRollback, commitRollback)
    })

    expect(result).toEqual({ schemaVersion: 1, status: 'ROLLBACK_COMMITTED' })
    expect(beginRollback).toHaveBeenCalledTimes(2)
    expect(commitRollback).toHaveBeenCalledTimes(1)
  })

  it('discovers rollback and finalization descriptors after a fresh main restart without retained prepare inputs', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-discovered-restart-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const sourceLocator = 'current:analytix-settings.json'
    const credentials = {
      'provider-alpha': 'synthetic-discovered-alpha-not-a-real-key',
      'provider-beta': 'synthetic-discovered-beta-not-a-real-key'
    }
    const source = Buffer.from(JSON.stringify({
      provider: {
        activeProviderId: 'provider-alpha',
        providers: [
          {
            id: 'provider-alpha',
            name: 'Provider Alpha',
            apiKey: credentials['provider-alpha'],
            baseUrl: 'https://alpha.invalid/v1',
            models: ['alpha-model']
          },
          {
            id: 'provider-beta',
            name: 'Provider Beta',
            apiKey: credentials['provider-beta'],
            baseUrl: 'https://beta.invalid/v1',
            models: ['beta-model']
          }
        ]
      }
    }), 'utf8')
    const sourceSHA256 = createHash('sha256').update(source).digest('hex')
    await writeFile(settingsPath, source)
    const beforeRestartStore = new JsonSettingsStore(userDataDir)
    const beforeRestartInspection = await beforeRestartStore.inspectLegacyProviderCredentialSources()
    const expectedCleanedSourceSHA256 = beforeRestartInspection.expectedCleanedSourceSHA256!
    await beforeRestartStore.cleanupLegacyProviderCredentialSource({
      schemaVersion: 1,
      purpose: 'remove-verified-legacy-provider-credentials',
      confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
      sourceLocator,
      sourceSHA256,
      expectedCleanedSourceSHA256,
      protectedCredentialLocators: [
        `${sourceLocator}:provider.providers[0].apiKey`,
        `${sourceLocator}:provider.providers[1].apiKey`
      ]
    })
    beforeRestartInspection.sourceSnapshot?.fill(0)
    for (const candidate of beforeRestartInspection.candidates) {
      candidate.credential.fill(0)
      for (const artifact of candidate.rollbackCredentialArtifacts) artifact.credential.fill(0)
    }
    expectCredentialsCleared(beforeRestartInspection)
    expect(await readFile(settingsPath, 'utf8')).not.toContain('synthetic-discovered-')

    const descriptors: ProviderRegistryLegacyMigrationRecoveryDescriptor[] = [
      {
        migrationId: 'migration-discovered-alpha',
        providerId: 'provider-alpha',
        sourceLocator,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256: beforeRestartInspection.sourcePhysicalIdentitySHA256!,
        recoveryCredentialRef: recoveryRef('D'),
        phase: 'provider-committed-recovery-retained',
        commitOrder: '4'
      },
      {
        migrationId: 'migration-discovered-beta',
        providerId: 'provider-beta',
        sourceLocator,
        sourceSHA256,
        expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256: beforeRestartInspection.sourcePhysicalIdentitySHA256!,
        recoveryCredentialRef: recoveryRef('E'),
        phase: 'provider-committed-recovery-retained',
        commitOrder: '5'
      }
    ]
    const committed = new Set<string>()
    const finalized = new Set<string>()
    const runtimeRequest = vi.fn(async (path: string, _method: 'POST', body: string) => {
      if (path.endsWith('/inventory')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({ schemaVersion: 1, recoveries: descriptors })
        }
      }
      const request = JSON.parse(body) as { migrationId: string }
      if (path.endsWith('/source-authority/challenge')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            challenge: testSourceAuthorityChallenge
          })
        }
      }
      if (path.endsWith('/rollback/begin')) {
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            status: committed.has(request.migrationId)
              ? 'ROLLBACK_COMMITTED_RECOVERY_RETAINED'
              : 'CLEANED_SOURCE_AUTHORITY_REQUIRED',
            migrationId: request.migrationId
          })
        }
      }
      if (path.endsWith('/rollback/commit')) {
        committed.add(request.migrationId)
        return {
          ok: true,
          status: 200,
          body: JSON.stringify({
            schemaVersion: 1,
            status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED',
            migrationId: request.migrationId
          })
        }
      }
      const alreadyFinalized = finalized.has(request.migrationId)
      finalized.add(request.migrationId)
      return {
        ok: true,
        status: 200,
        body: JSON.stringify({
          schemaVersion: 1,
          status: alreadyFinalized ? 'ALREADY_FINALIZED' : 'COMPLETED',
          outcome: 'PROTECTED_RECOVERY_RETAINED',
          migrationId: request.migrationId
        })
      }
    })

    const restartedStore = new JsonSettingsStore(userDataDir)
    const restartedClient = createProviderRegistryLegacyMigrationClient(runtimeRequest)
    const discovered = await restartedClient.inventory()
    if ('error' in discovered) throw new Error('private recovery inventory was unavailable')
    const recoveriesInCommitOrder = discovered.recoveries.map((descriptor) => ({
      migrationId: descriptor.migrationId,
      providerId: descriptor.providerId,
      recoveryCredentialRef: descriptor.recoveryCredentialRef,
      commitOrder: descriptor.commitOrder
    }))
    const providers = descriptors.map((descriptor): ProviderRegistryPublicProviderV1 => ({
      id: descriptor.providerId,
      kind: 'openai-compatible',
      endpoint: `https://${descriptor.providerId}.invalid/v1`,
      models: [`${descriptor.providerId}-model`],
      mediaModels: [],
      selectedRoutes: [],
      credentialConfigured: true,
      credentialPurpose: 'provider-api-key',
      revision: '1',
      generation: '1',
      incarnation: providerIncarnation,
      tombstone: false
    }))
    await expect(orchestrateProviderRegistryLegacyMigrationRollbackGroup({
      recoveriesInCommitOrder,
      sourceLocator: discovered.recoveries[0].sourceLocator,
      sourceSHA256: discovered.recoveries[0].sourceSHA256,
      expectedCleanedSourceSHA256: discovered.recoveries[0].expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256: discovered.recoveries[0].sourcePhysicalIdentitySHA256,
      settingsStore: restartedStore,
      readRegistrySnapshot: async () => snapshot('2', providers, 'provider-alpha'),
      migrationClient: restartedClient
    })).resolves.toEqual({ schemaVersion: 1, status: 'ROLLBACK_COMMITTED' })
    const cleanedSource = await readFile(settingsPath)
    expect(createHash('sha256').update(cleanedSource).digest('hex')).toBe(expectedCleanedSourceSHA256)
    expect(cleanedSource.includes(Buffer.from('synthetic-discovered-'))).toBe(false)

    for (const descriptor of discovered.recoveries) {
      await expect(finalizeProviderRegistryLegacyMigrationRecovery({
        migrationId: descriptor.migrationId,
        sourceLocator: descriptor.sourceLocator,
        sourceSHA256: descriptor.sourceSHA256,
        verifiedSourceSHA256: descriptor.expectedCleanedSourceSHA256,
        sourcePhysicalIdentitySHA256: descriptor.sourcePhysicalIdentitySHA256,
        recoveryCredentialRef: descriptor.recoveryCredentialRef,
        settingsStore: restartedStore,
        migrationClient: restartedClient
      })).resolves.toMatchObject({ schemaVersion: 1, status: 'COMPLETED' })
    }
    await expect(finalizeProviderRegistryLegacyMigrationRecovery({
      migrationId: discovered.recoveries[0].migrationId,
      sourceLocator: discovered.recoveries[0].sourceLocator,
      sourceSHA256: discovered.recoveries[0].sourceSHA256,
      verifiedSourceSHA256: discovered.recoveries[0].expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256: discovered.recoveries[0].sourcePhysicalIdentitySHA256,
      recoveryCredentialRef: discovered.recoveries[0].recoveryCredentialRef,
      settingsStore: restartedStore,
      migrationClient: restartedClient
    })).resolves.toMatchObject({ schemaVersion: 1, status: 'ALREADY_FINALIZED' })
    cleanedSource.fill(0)
  })

  it('does not rewrite an already key-free source after cleanup', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-key-free-noop-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await writeFile(settingsPath, JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-key-free-noop-marker',
        baseUrl: 'https://key-free-noop.example/v1',
        unknownMetadata: 'preserve-key-free-noop'
      }
    }), 'utf8')
    let atomicWriteCount = 0
    const privateOptions = {
      legacyProviderCleanupAtomicWriteFile: async (
        path: string,
        contents: string,
        options: Parameters<typeof atomicWriteFile>[2]
      ) => {
        atomicWriteCount += 1
        await atomicWriteFile(path, contents, options)
      }
    }
    let preparedProvider: ProviderRegistryProviderInputV1 | undefined
    const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
      preparedProvider = input.provider
      return prepareVerified(input.migrationId, 'N')
    })
    const commit = vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) =>
      commitSuccess(input.migrationId, providerProjection(preparedProvider!)))
    const readRegistrySnapshot = vi.fn(async () => snapshot('0'))

    const first = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: new JsonSettingsStore(userDataDir, privateOptions),
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })
    const afterFirst = await readFile(settingsPath)
    expect(first).toMatchObject({
      status: 'COMPLETED',
      candidateCount: 1,
      completedCandidateCount: 1,
      committedCandidateCount: 1
    })
    expect(atomicWriteCount).toBe(1)
    expect(afterFirst.includes(Buffer.from('synthetic-key-free-noop-marker'))).toBe(false)
    expect(JSON.parse(afterFirst.toString('utf8')).provider.unknownMetadata)
      .toBe('preserve-key-free-noop')

    const second = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: new JsonSettingsStore(userDataDir, privateOptions),
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })
    const afterSecond = await readFile(settingsPath)
    expect(second).toEqual({
      schemaVersion: 1,
      status: 'COMPLETED',
      candidateCount: 0,
      completedCandidateCount: 0,
      committedCandidateCount: 0,
      retainedReplayCount: 0
    })
    expect(atomicWriteCount).toBe(1)
    expect(afterSecond).toEqual(afterFirst)
    expect(prepare).toHaveBeenCalledTimes(1)
    expect(commit).toHaveBeenCalledTimes(1)
    expect(readRegistrySnapshot).toHaveBeenCalledTimes(1)
  })

  it('resumes cleanup finalization from private inventory after a fresh main restart', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-cleanup-finalize-restart-'))
    const currentPath = join(userDataDir, 'analytix-settings.json')
    const settingsPath = join(userDataDir, 'kun-settings.json')
    const sourceLocator = 'compatibility:01:kun-settings.json'
    const source = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-cleanup-finalize-restart',
        baseUrl: 'https://cleanup-finalize-restart.invalid/v1'
      }
    }), 'utf8')
    const cleaned = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        baseUrl: 'https://cleanup-finalize-restart.invalid/v1'
      }
    }), 'utf8')
    const sourceSHA256 = createHash('sha256').update(source).digest('hex')
    const expectedCleanedSourceSHA256 = createHash('sha256').update(cleaned).digest('hex')
    await writeFile(settingsPath, source)

    let preparedProvider: ProviderRegistryProviderInputV1 | undefined
    let preparedMigrationId = ''
    let preparedSourcePhysicalIdentitySHA256 = ''
    let activeRecovery = false
    let interruptFinalizeOnce = true
    let finalizeCalls = 0
    let inventoryCalls = 0
    const migrationClient = {
      prepare: vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
        preparedProvider = input.provider
        preparedMigrationId = input.migrationId
        preparedSourcePhysicalIdentitySHA256 = input.sourcePhysicalIdentitySHA256
        return prepareVerified(input.migrationId, 'Q')
      }),
      commit: vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) => {
        activeRecovery = true
        return commitSuccess(input.migrationId, providerProjection(preparedProvider!))
      }),
      inventory: vi.fn(async () => {
        inventoryCalls += 1
        return {
          schemaVersion: 1 as const,
          recoveries: activeRecovery
            ? [{
                migrationId: preparedMigrationId,
                providerId: 'deepseek',
                sourceLocator,
                sourceSHA256,
                expectedCleanedSourceSHA256,
                sourcePhysicalIdentitySHA256: preparedSourcePhysicalIdentitySHA256,
                recoveryCredentialRef: recoveryRef('Q'),
                phase: 'provider-committed-recovery-retained' as const,
                commitOrder: '0'
              }]
            : []
        }
      }),
      issueSourceAuthorityChallenge: vi.fn(async () => ({
        schemaVersion: 1 as const,
        challenge: testSourceAuthorityChallenge
      })),
      finalize: vi.fn(async (input: { migrationId: string }) => {
        finalizeCalls += 1
        if (interruptFinalizeOnce) {
          interruptFinalizeOnce = false
          throw new Error('synthetic cleanup/finalize interruption')
        }
        activeRecovery = false
        return {
          schemaVersion: 1 as const,
          status: 'COMPLETED' as const,
          outcome: 'MIGRATION_COMMITTED' as const,
          migrationId: input.migrationId
        }
      })
    }
    const readRegistrySnapshot = vi.fn(async () => snapshot('0'))

    const first = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: new JsonSettingsStore(userDataDir),
      readRegistrySnapshot,
      migrationClient
    })
    expect(first).toMatchObject({ status: 'FAILED', failurePhase: 'finalize' })
    expect(await readFile(settingsPath)).toEqual(cleaned)
    expect(finalizeCalls).toBe(1)
    const currentRaw = Buffer.from(JSON.stringify({ version: 1, provider: {} }), 'utf8')
    await writeFile(currentPath, currentRaw)

    const restarted = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: new JsonSettingsStore(userDataDir),
      readRegistrySnapshot,
      migrationClient
    })
    expect(restarted).toEqual({
      schemaVersion: 1,
      status: 'COMPLETED',
      candidateCount: 0,
      completedCandidateCount: 0,
      committedCandidateCount: 0,
      retainedReplayCount: 0
    })
    expect(finalizeCalls).toBe(2)
    expect(activeRecovery).toBe(false)
    expect(await readFile(currentPath)).toEqual(currentRaw)
    expect(await readFile(settingsPath)).toEqual(cleaned)

    const repeated = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: new JsonSettingsStore(userDataDir),
      readRegistrySnapshot,
      migrationClient
    })
    expect(repeated).toMatchObject({ status: 'COMPLETED', candidateCount: 0 })
    expect(finalizeCalls).toBe(2)
    expect(inventoryCalls).toBeGreaterThanOrEqual(3)
  })

  it('does not finalize from a cleanup receipt after the same logical source is atomically replaced', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-stale-cleanup-receipt-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await writeFile(settingsPath, JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-stale-cleanup-receipt',
        baseUrl: 'https://stale-cleanup-receipt.invalid/v1'
      }
    }))
    const store = new JsonSettingsStore(userDataDir)
    let preparedProvider: ProviderRegistryProviderInputV1 | undefined
    const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
      preparedProvider = input.provider
      return prepareVerified(input.migrationId, 'U')
    })
    const commit = vi.fn(async (input: ProviderRegistryLegacyMigrationCommitRequest) =>
      commitSuccess(input.migrationId, providerProjection(preparedProvider!)))
    const finalize = vi.fn(async (input: ProviderRegistryLegacyMigrationFinalizeRequest) => ({
      schemaVersion: 1 as const,
      status: 'COMPLETED' as const,
      outcome: 'MIGRATION_COMMITTED' as const,
      migrationId: input.migrationId
    }))
    const result = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: {
        inspectLegacyProviderCredentialSources: store.inspectLegacyProviderCredentialSources.bind(store),
        cleanupLegacyProviderCredentialSource: async (input) => {
          const receipt = await store.cleanupLegacyProviderCredentialSource(input)
          await atomicWriteFile(settingsPath, JSON.stringify({
            version: 1,
            provider: {
              baseUrl: 'https://stale-cleanup-receipt.invalid/v1',
              replacementGeneration: true
            }
          }), { mode: 0o600 })
          return receipt
        },
        withLegacyProviderCredentialSourceAuthority:
          store.withLegacyProviderCredentialSourceAuthority.bind(store)
      },
      readRegistrySnapshot: async () => snapshot('0'),
      migrationClient: {
        prepare,
        commit,
        inventory: async () => ({ schemaVersion: 1 as const, recoveries: [] }),
        issueSourceAuthorityChallenge: async () => ({
          schemaVersion: 1 as const,
          challenge: testSourceAuthorityChallenge
        }),
        finalize
      }
    })

    expect(result).toMatchObject({ status: 'FAILED', failurePhase: 'finalize' })
    expect(finalize).not.toHaveBeenCalled()
    expect((await readFile(settingsPath)).includes(Buffer.from('synthetic-stale-cleanup-receipt')))
      .toBe(false)
  })

  it('reports stale cleaned-source authority after same-source replacement without false success', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-stale-restore-receipt-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const sourceLocator = 'current:analytix-settings.json'
    const sourceSnapshot = new TextEncoder().encode(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-stale-restore-receipt',
        baseUrl: 'https://stale-restore-receipt.invalid/v1'
      }
    }))
    const sourceSHA256 = createHash('sha256').update(sourceSnapshot).digest('hex')
    await writeFile(settingsPath, JSON.stringify({
      version: 1,
      provider: { baseUrl: 'https://stale-restore-receipt.invalid/v1' }
    }))
    const store = new JsonSettingsStore(userDataDir)
    const cleanedInspection = await store.inspectLegacyProviderCredentialSources()
    const sourcePhysicalIdentitySHA256 = cleanedInspection.sourcePhysicalIdentitySHA256!
    const expectedCleanedSourceSHA256 = cleanedInspection.sourceSHA256!
    cleanedInspection.sourceSnapshot?.fill(0)
    const provider = providerProjection({
      id: 'provider-stale-restore-receipt',
      kind: 'openai-compatible',
      endpoint: 'https://stale-restore-receipt.invalid/v1',
      proxy: '',
      models: [],
      mediaModels: [],
      selectedModel: '',
      selectedMediaModel: '',
      selectedRoutes: []
    })
    const beginRollback = vi.fn(async () => ({
      schemaVersion: 1 as const,
      status: 'CLEANED_SOURCE_AUTHORITY_REQUIRED' as const,
      migrationId: 'migration-stale-restore-receipt'
    }))
    const commitRollback = vi.fn(async () => ({
      schemaVersion: 1 as const,
      status: 'ROLLBACK_COMMITTED_RECOVERY_RETAINED' as const,
      migrationId: 'migration-stale-restore-receipt'
    }))
    const result = await orchestrateProviderRegistryLegacyMigrationRollbackGroup({
      recoveriesInCommitOrder: [{
        migrationId: 'migration-stale-restore-receipt',
        providerId: provider.id,
        recoveryCredentialRef: recoveryRef('V'),
        commitOrder: '1'
      }],
      sourceLocator,
      sourceSHA256,
      expectedCleanedSourceSHA256,
      sourcePhysicalIdentitySHA256,
      settingsStore: {
        withLegacyProviderCredentialSourceAuthority: async (input, action) =>
          store.withLegacyProviderCredentialSourceAuthority(input, async (authority) => {
            await atomicWriteFile(settingsPath, JSON.stringify({
              version: 1,
              provider: {
                baseUrl: 'https://stale-restore-receipt.invalid/v1',
                replacementGeneration: true
              }
            }), { mode: 0o600 })
            return action(authority)
          })
      },
      readRegistrySnapshot: async () => snapshot('2', [provider], provider.id),
      migrationClient: completeRollbackClient(beginRollback, commitRollback)
    })

    expect(result).toMatchObject({ status: 'FAILED', failurePhase: 'source' })
    expect(commitRollback).toHaveBeenCalledTimes(1)
    expect((await readFile(settingsPath)).includes(Buffer.from('synthetic-stale-restore-receipt')))
      .toBe(false)
  })

  it('never finalizes a retained recovery from original or drifted source state', async () => {
    const sourceLocator = 'current:analytix-settings.json'
    const sourceSHA256 = '1'.repeat(64)
    const expectedCleanedSourceSHA256 = '2'.repeat(64)
    const sourcePhysicalIdentitySHA256 = '4'.repeat(64)
    for (const currentSourceSHA256 of [sourceSHA256, '3'.repeat(64)]) {
      const cleanup = vi.fn(async () => { throw new Error('unreachable cleanup') })
      const finalize = vi.fn(async () => ({
        schemaVersion: 1 as const,
        status: 'COMPLETED' as const,
        outcome: 'MIGRATION_COMMITTED' as const,
        migrationId: 'migration-cleanup-state-classification'
      }))
      const migrationClient = {
        prepare: vi.fn(),
        commit: vi.fn(),
        inventory: vi.fn(async () => ({
          schemaVersion: 1 as const,
          recoveries: [{
            migrationId: 'migration-cleanup-state-classification',
            providerId: 'provider-cleanup-state-classification',
            sourceLocator,
            sourceSHA256,
            expectedCleanedSourceSHA256,
            sourcePhysicalIdentitySHA256,
            recoveryCredentialRef: recoveryRef('S'),
            phase: 'provider-committed-recovery-retained' as const,
            commitOrder: '1'
          }]
        })),
        issueSourceAuthorityChallenge: async () => ({
          schemaVersion: 1 as const,
          challenge: testSourceAuthorityChallenge
        }),
        finalize
      }
      const result = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: {
          inspectLegacyProviderCredentialSources: async () => ({
            schemaVersion: 1 as const,
            sourceLocator,
            sourceSHA256: currentSourceSHA256,
            expectedCleanedSourceSHA256,
            sourcePhysicalIdentitySHA256,
            sourceSnapshot: new Uint8Array([1]),
            candidates: []
          }),
          cleanupLegacyProviderCredentialSource: cleanup,
          withLegacyProviderCredentialSourceAuthority: testSourceAuthorityMethod(() => null)
        },
        readRegistrySnapshot: async () => snapshot('0'),
        migrationClient
      })
      expect(result.status).toBe('FAILED')
      expect(cleanup).not.toHaveBeenCalled()
      expect(finalize).not.toHaveBeenCalled()
    }
  })

  it('defaults deepseek first and sorts every remaining non-active candidate deterministically', async () => {
    const inspection = syntheticInspection([
      syntheticCandidate('zeta', 2),
      syntheticCandidate('deepseek', 1),
      syntheticCandidate('alpha', 0)
    ])
    const order: string[] = []
    const expectedOrder = [inspection.candidates[1], inspection.candidates[2], inspection.candidates[0]]
    const prepare = vi.fn(async (input: ProviderRegistryLegacyMigrationPrepareRequest) => {
      order.push(input.provider.id)
      return prepareRetained(input.migrationId)
    })
    const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: fixedInspectionStore(inspection),
      readRegistrySnapshot: async () => snapshot(
        String(order.length),
        [committedWinner(expectedOrder[order.length])]
      ),
      migrationClient: completeMigrationClient(prepare, async () => undefined)
    })

    expect(orchestrationResult).toMatchObject({ status: 'COMPLETED', retainedReplayCount: 3 })
    expect(order).toEqual(['deepseek', 'alpha', 'zeta'])
    expectCredentialsCleared(inspection)

    const nonCredentialedActiveInspection = syntheticInspection([
      syntheticCandidate('zeta', 1, 'selected-without-credential'),
      syntheticCandidate('alpha', 0, 'selected-without-credential')
    ])
    const nonCredentialedActiveOrder: string[] = []
    const nonCredentialedExpectedOrder = [
      nonCredentialedActiveInspection.candidates[1],
      nonCredentialedActiveInspection.candidates[0]
    ]
    const nonCredentialedActiveResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: fixedInspectionStore(nonCredentialedActiveInspection),
      readRegistrySnapshot: async () => snapshot(
        String(nonCredentialedActiveOrder.length),
        [committedWinner(nonCredentialedExpectedOrder[nonCredentialedActiveOrder.length])]
      ),
      migrationClient: completeMigrationClient(
        async (input) => {
          nonCredentialedActiveOrder.push(input.provider.id)
          return prepareRetained(input.migrationId)
        },
        async () => undefined
      )
    })
    expect(nonCredentialedActiveResult).toMatchObject({ status: 'COMPLETED', retainedReplayCount: 2 })
    expect(nonCredentialedActiveOrder).toEqual(['alpha', 'zeta'])
    expectCredentialsCleared(nonCredentialedActiveInspection)
  })

  it('fails inspection and candidate validation without snapshot or transport mutation', async () => {
    const invalidInspection = syntheticInspection([
      syntheticCandidate('alpha', 0, 'alpha'),
      syntheticCandidate('beta', 1, 'beta')
    ])
    const readRegistrySnapshot = vi.fn(async () => snapshot('0'))
    const prepare = vi.fn(async () => undefined)
    const commit = vi.fn(async () => undefined)

    const invalidResult = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: fixedInspectionStore(invalidInspection),
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })
    const inspectionFailure = await orchestrateProviderRegistryLegacyMigration({
      settingsStore: {
        inspectLegacyProviderCredentialSources: async () => {
          throw new Error('private source path and body')
        },
        cleanupLegacyProviderCredentialSource: async () => { throw new Error('unreachable cleanup') },
        withLegacyProviderCredentialSourceAuthority: testSourceAuthorityMethod(() => null)
      },
      readRegistrySnapshot,
      migrationClient: completeMigrationClient(prepare, commit)
    })

    expect(invalidResult).toMatchObject({ status: 'FAILED', failurePhase: 'candidate' })
    expect(inspectionFailure).toEqual({
      schemaVersion: 1,
      status: 'FAILED',
      candidateCount: 0,
      completedCandidateCount: 0,
      committedCandidateCount: 0,
      retainedReplayCount: 0,
      failurePhase: 'inspection'
    })
    expect(readRegistrySnapshot).not.toHaveBeenCalled()
    expect(prepare).not.toHaveBeenCalled()
    expect(commit).not.toHaveBeenCalled()
    expectCredentialsCleared(invalidInspection)
  })

  it('rejects unsupported proxy and Provider metadata before any Registry transport', async () => {
    const values = [
      {
        version: 1,
        provider: {
          proxy: { enabled: true, url: 'socks5://proxy.example:1080' },
          providers: [{
            id: 'custom-provider',
            apiKey: 'synthetic-unsupported-proxy-orchestrator-marker',
            baseUrl: 'https://custom.example/v1',
            models: ['custom-model']
          }]
        }
      },
      {
        version: 1,
        provider: {
          providers: [{
            id: 'custom-provider',
            apiKey: 'synthetic-unsupported-metadata-orchestrator-marker',
            baseUrl: '',
            models: ['custom-model']
          }]
        }
      }
    ]
    for (const value of values) {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-provider-registry-invalid-input-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      const source = Buffer.from(JSON.stringify(value), 'utf8')
      await writeFile(settingsPath, source)
      const beforeEntries = await readdir(userDataDir)
      const readRegistrySnapshot = vi.fn(async () => snapshot('0'))
      const prepare = vi.fn(async () => undefined)
      const commit = vi.fn(async () => undefined)

      const orchestrationResult = await orchestrateProviderRegistryLegacyMigration({
        settingsStore: new JsonSettingsStore(userDataDir),
        readRegistrySnapshot,
        migrationClient: completeMigrationClient(prepare, commit)
      })

      expect(orchestrationResult).toMatchObject({ status: 'FAILED', failurePhase: 'inspection' })
      expect(readRegistrySnapshot).not.toHaveBeenCalled()
      expect(prepare).not.toHaveBeenCalled()
      expect(commit).not.toHaveBeenCalled()
      expect(await readFile(settingsPath)).toEqual(source)
      expect(await readdir(userDataDir)).toEqual(beforeEntries)
    }
  })
})
