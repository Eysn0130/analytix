import { chmod, link, lstat, mkdir, mkdtemp, readFile, readdir, rm, rmdir, stat, symlink, utimes, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { atomicWriteFile } from '../../packages/runtime/src/adapters/file/atomic-write'
import {
  DEFAULT_APPROVAL_POLICY,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  type AppSettingsV1
} from '../shared/app-settings'
import { DEFAULT_GUI_UPDATE_CHANNEL } from '../shared/gui-update'
import {
  JsonSettingsStore,
  devServerHintUrl,
  mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1
} from './settings-store'

type LegacyProviderCredentialInspection = Awaited<
  ReturnType<JsonSettingsStore['inspectLegacyProviderCredentialSources']>
>

function clearInspectionCredentials(inspection: LegacyProviderCredentialInspection): void {
  inspection.sourceSnapshot?.fill(0)
  for (const candidate of inspection.candidates) {
    candidate.credential.fill(0)
    for (const artifact of candidate.rollbackCredentialArtifacts ?? []) artifact.credential.fill(0)
  }
}

function decodeCredential(credential: Uint8Array): string {
  return new TextDecoder().decode(credential)
}

async function inspectCurrentSettings(
  value: unknown,
  prefix = 'analytix-settings-legacy-provider-inspection-'
): Promise<{
  userDataDir: string
  settingsPath: string
  raw: Buffer
  inspection: LegacyProviderCredentialInspection
}> {
  const userDataDir = await mkdtemp(join(tmpdir(), prefix))
  const settingsPath = join(userDataDir, 'analytix-settings.json')
  const raw = Buffer.from(JSON.stringify(value), 'utf8')
  await writeFile(settingsPath, raw)
  const inspection = await new JsonSettingsStore(userDataDir).inspectLegacyProviderCredentialSources()
  return { userDataDir, settingsPath, raw, inspection }
}

function cleanupRequestFor(inspection: LegacyProviderCredentialInspection) {
  return {
    schemaVersion: 1 as const,
    purpose: 'remove-verified-legacy-provider-credentials' as const,
    confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT' as const,
    sourceLocator: inspection.sourceLocator!,
    sourceSHA256: inspection.sourceSHA256!,
    expectedCleanedSourceSHA256: inspection.expectedCleanedSourceSHA256!,
    protectedCredentialLocators: inspection.candidates.flatMap((candidate) => [
      ...candidate.credentialLocators,
      ...candidate.rollbackCredentialArtifacts.flatMap((artifact) => artifact.credentialLocators)
    ]).sort()
  }
}

describe('desktop renderer entry policy', () => {
  it('ignores a development renderer URL in a packaged app', () => {
    expect(devServerHintUrl(true, 'http://127.0.0.1:5173')).toBeUndefined()
    expect(devServerHintUrl(false, 'http://127.0.0.1:5173')).toBe('http://127.0.0.1:5173')
  })
})

describe('JsonSettingsStore', () => {
  it('serializes canonical settings without legacy Provider credential fields or bytes', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-key-free-canonical-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const store = new JsonSettingsStore(userDataDir)
    const current = await store.load()
    const syntheticCanary = ['synthetic', 'k3', 'canonical', 'credential'].join('-')

    await store.save({
      ...current,
      provider: {
        ...current.provider,
        apiKey: syntheticCanary,
        providers: current.provider.providers.map((provider, index) => ({
          ...provider,
          apiKey: index === 0 ? syntheticCanary : ''
        }))
      },
      runtime: {
        ...current.runtime,
        apiKey: syntheticCanary
      },
      write: {
        ...current.write,
        inlineCompletion: {
          ...current.write.inlineCompletion,
          apiKey: syntheticCanary
        }
      }
    } as unknown as AppSettingsV1)

    const serialized = await readFile(settingsPath, 'utf8')
    const persisted = JSON.parse(serialized) as Record<string, unknown>
    const publicSnapshot = await new JsonSettingsStore(userDataDir).load()
    const ownKeys = (value: unknown): string[] => {
      if (Array.isArray(value)) return value.flatMap(ownKeys)
      if (!value || typeof value !== 'object') return []
      return Object.entries(value as Record<string, unknown>)
        .flatMap(([key, nested]) => [key, ...ownKeys(nested)])
    }

    expect(serialized.includes(syntheticCanary)).toBe(false)
    expect(JSON.stringify(publicSnapshot).includes(syntheticCanary)).toBe(false)
    expect(ownKeys(persisted).includes('apiKey')).toBe(false)
    expect(ownKeys(publicSnapshot).includes('apiKey')).toBe(false)
  })

  it('rejects ordinary transport secret writes without mutating canonical settings or public snapshots', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-key-free-transport-accounts-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const store = new JsonSettingsStore(userDataDir)
    const current = await store.load()
    const telegramToken = '123456789:synthetic-telegram-account-token'
    const weixinSessionKey = 'synthetic-weixin-session-key'
    const feishuAppSecret = 'synthetic-feishu-app-secret'
    const credentialRef = 'cred_synthetic_transport_ref'
    const before = await readFile(settingsPath, 'utf8')
    const channelBase = {
      enabled: true,
      providerId: '',
      model: 'auto',
      threadId: '',
      workspaceRoot: '',
      agentProfile: {
        name: 'analytix',
        description: '',
        identity: '',
        personality: '',
        userContext: '',
        replyRules: ''
      },
      conversations: [],
      createdAt: '2026-08-30T00:00:00.000Z',
      updatedAt: '2026-08-30T00:00:00.000Z'
    }

    await expect(store.save({
      ...current,
      claw: {
        ...current.claw,
        enabled: true,
        im: { ...current.claw.im, enabled: true },
        channels: [
          {
            ...channelBase,
            id: 'transport-telegram',
            provider: 'telegram',
            label: 'Telegram',
            platformCredential: {
              kind: 'telegram',
              botToken: telegramToken,
              allowedChatIds: '1001',
              createdAt: '2026-08-30T00:00:00.000Z',
              credentialRef
            }
          },
          {
            ...channelBase,
            id: 'transport-weixin',
            provider: 'weixin',
            label: 'WeChat',
            platformCredential: {
              kind: 'weixin',
              accountId: 'synthetic-weixin-account',
              sessionKey: weixinSessionKey,
              createdAt: '2026-08-30T00:00:00.000Z',
              credentialRef
            }
          },
          {
            ...channelBase,
            id: 'transport-feishu',
            provider: 'feishu',
            label: 'Feishu',
            platformCredential: {
              kind: 'feishu',
              appId: 'synthetic-feishu-app',
              appSecret: feishuAppSecret,
              domain: 'feishu',
              createdAt: '2026-08-30T00:00:00.000Z',
              credentialRef
            }
          }
        ]
      }
    } as unknown as AppSettingsV1)).rejects.toThrow('Legacy IM plaintext persistence is not permitted.')

    const serialized = await readFile(settingsPath, 'utf8')
    const publicSnapshot = await new JsonSettingsStore(userDataDir).load()
    const publicText = JSON.stringify(publicSnapshot)
    for (const forbidden of [telegramToken, weixinSessionKey, feishuAppSecret, credentialRef]) {
      expect(serialized).not.toContain(forbidden)
      expect(publicText).not.toContain(forbidden)
    }
    expect(serialized).toBe(before)
    expect(serialized).not.toMatch(/"(?:botToken|sessionKey|appSecret|credentialRef)"\s*:/)
    expect(publicText).not.toMatch(/"(?:botToken|sessionKey|appSecret|credentialRef)"\s*:/)
  })

  it('does not expose an ordinary plaintext restore producer', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-no-plaintext-restore-producer-'))
    const store = new JsonSettingsStore(userDataDir)
    expect('restoreLegacyProviderCredentialSource' in store).toBe(false)
  })

  it('inspects supported legacy Provider credentials without mutating their source', async () => {
    const cases = [
      {
        name: 'canonical top-level Provider',
        value: {
          version: 1,
          provider: {
            apiKey: 'synthetic-canonical-provider-marker',
            baseUrl: 'https://provider.example/v1'
          }
        },
        marker: 'synthetic-canonical-provider-marker',
        providerId: 'deepseek',
        locator: 'provider.apiKey',
        metadata: { baseUrl: 'https://provider.example/v1' }
      },
      {
        name: 'canonical Provider profile',
        value: {
          version: 1,
          provider: {
            activeProviderId: 'custom-profile',
            providers: [{
              id: 'custom-profile',
              name: 'Synthetic Custom',
              apiKey: 'synthetic-profile-marker',
              baseUrl: 'https://custom.example/v1',
              endpointFormat: 'messages',
              models: ['synthetic-model']
            }]
          }
        },
        marker: 'synthetic-profile-marker',
        providerId: 'custom-profile',
        locator: 'provider.providers[0].apiKey',
        metadata: {
          name: 'Synthetic Custom',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'messages',
          models: ['synthetic-model']
        }
      },
      {
        name: 'runtime seed',
        value: {
          version: 1,
          runtime: {
            apiKey: 'synthetic-runtime-marker',
            baseUrl: 'https://runtime.example/v1'
          }
        },
        marker: 'synthetic-runtime-marker',
        providerId: 'deepseek',
        locator: 'runtime.apiKey',
        metadata: { baseUrl: 'https://runtime.example/v1' }
      },
      {
        name: 'implicit DeepSeek seed',
        value: {
          version: 1,
          deepseek: {
            apiKey: 'synthetic-deepseek-marker',
            baseUrl: 'https://deepseek.example/v1'
          }
        },
        marker: 'synthetic-deepseek-marker',
        providerId: 'deepseek',
        locator: 'deepseek.apiKey',
        metadata: { baseUrl: 'https://deepseek.example/v1' }
      },
      {
        name: 'Reasonix seed',
        value: {
          version: 1,
          agentProvider: 'reasonix',
          agents: {
            reasonix: {
              apiKey: 'synthetic-reasonix-marker',
              baseUrl: 'https://reasonix.example/v1'
            }
          }
        },
        marker: 'synthetic-reasonix-marker',
        providerId: 'deepseek',
        locator: 'agents.reasonix.apiKey',
        metadata: { baseUrl: 'https://reasonix.example/v1' }
      },
      {
        name: 'Codewhale seed',
        value: {
          version: 1,
          agentProvider: 'codewhale',
          agents: {
            codewhale: {
              apiKey: 'synthetic-codewhale-marker',
              baseUrl: 'https://codewhale.example/v1'
            }
          }
        },
        marker: 'synthetic-codewhale-marker',
        providerId: 'deepseek',
        locator: 'agents.codewhale.apiKey',
        metadata: { baseUrl: 'https://codewhale.example/v1' }
      },
      {
        name: 'Codewhale DeepSeek override',
        value: {
          version: 1,
          agentProvider: 'codewhale',
          agents: {
            codewhale: {
              apiKey: 'synthetic-lower-precedence-marker',
              baseUrl: 'https://lower-precedence.example/v1'
            }
          },
          deepseek: {
            apiKey: 'synthetic-codewhale-override-marker',
            baseUrl: 'https://codewhale-override.example/v1'
          }
        },
        marker: 'synthetic-codewhale-override-marker',
        providerId: 'deepseek',
        locator: 'deepseek.apiKey',
        metadata: { baseUrl: 'https://codewhale-override.example/v1' }
      },
      {
        name: 'DeepSeek runtime seed',
        value: {
          version: 1,
          agentProvider: 'deepseek-runtime',
          deepseek: {
            apiKey: 'synthetic-deepseek-runtime-marker',
            baseUrl: 'https://deepseek-runtime.example/v1'
          }
        },
        marker: 'synthetic-deepseek-runtime-marker',
        providerId: 'deepseek',
        locator: 'deepseek.apiKey',
        metadata: { baseUrl: 'https://deepseek-runtime.example/v1' }
      }
    ]

    for (const testCase of cases) {
      const { settingsPath, raw, inspection } = await inspectCurrentSettings(testCase.value)
      expect(inspection, testCase.name).toMatchObject({
        schemaVersion: 1,
        sourceLocator: 'current:analytix-settings.json',
        sourceSHA256: createHash('sha256').update(raw).digest('hex')
      })
      expect(inspection.candidates, testCase.name).toHaveLength(1)
      expect(inspection.candidates[0], testCase.name).toMatchObject({
        schemaVersion: 1,
        providerId: testCase.providerId,
        sourceLocator: `current:analytix-settings.json:${testCase.locator}`,
        providerMetadata: {
          profile: testCase.metadata ?? {}
        }
      })
      expect(inspection.candidates[0].migrationId).toMatch(/^legacy-provider-settings-v1-[0-9a-f]{64}$/)
      expect(decodeCredential(inspection.candidates[0].credential), testCase.name).toBe(testCase.marker)
      expect('apiKey' in inspection.candidates[0].providerMetadata.profile).toBe(false)
      expect(await readFile(settingsPath), testCase.name).toEqual(raw)
      clearInspectionCredentials(inspection)
    }

    const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-legacy-kun-inspection-'))
    const currentUserDataDir = join(parentDir, 'AnalytixCurrent')
    const kunUserDataDir = join(parentDir, 'Kun')
    const kunSettingsPath = join(kunUserDataDir, 'kun-settings.json')
    const currentSettingsPath = join(currentUserDataDir, 'analytix-settings.json')
    const kunRaw = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-kun-lower-top-level-marker',
        providers: [
          { id: 'deepseek', apiKey: 'synthetic-kun-lower-profile-marker' },
          {
            id: 'custom-kun-profile',
            name: 'Synthetic Kun Custom',
            apiKey: 'synthetic-kun-custom-marker',
            baseUrl: 'https://kun-custom.example/v1',
            models: ['synthetic-kun-custom-model']
          }
        ]
      },
      agents: {
        kun: {
          apiKey: 'synthetic-kun-marker',
          baseUrl: 'https://kun.example/v1',
          model: 'synthetic-kun-model'
        }
      }
    }), 'utf8')
    await mkdir(kunUserDataDir, { recursive: true })
    await writeFile(kunSettingsPath, kunRaw)

    const kunInspection = await new JsonSettingsStore(currentUserDataDir)
      .inspectLegacyProviderCredentialSources()

    expect(kunInspection.sourceLocator).toMatch(/^compatibility:\d{2}:kun-settings\.json$/)
    expect(kunInspection.sourceSHA256).toBe(createHash('sha256').update(kunRaw).digest('hex'))
    expect(kunInspection.candidates).toHaveLength(2)
    const kunDefaultCandidate = kunInspection.candidates.find((candidate) => candidate.providerId === 'deepseek')
    const kunCustomCandidate = kunInspection.candidates.find((candidate) => candidate.providerId === 'custom-kun-profile')
    expect(kunDefaultCandidate).toMatchObject({
      providerId: 'deepseek',
      providerMetadata: {
        profile: {
          baseUrl: 'https://kun.example/v1',
          models: ['synthetic-kun-model']
        }
      }
    })
    expect(kunDefaultCandidate?.sourceLocator).toMatch(
      /^compatibility:\d{2}:kun-settings\.json:agents\.kun\.apiKey$/
    )
    expect(decodeCredential(kunDefaultCandidate?.credential ?? new Uint8Array())).toBe('synthetic-kun-marker')
    expect(kunCustomCandidate).toMatchObject({
      providerMetadata: {
        profile: {
          name: 'Synthetic Kun Custom',
          baseUrl: 'https://kun-custom.example/v1',
          models: ['synthetic-kun-custom-model']
        }
      }
    })
    expect(kunCustomCandidate?.sourceLocator).toMatch(
      /^compatibility:\d{2}:kun-settings\.json:provider\.providers\[1\]\.apiKey$/
    )
    expect(decodeCredential(kunCustomCandidate?.credential ?? new Uint8Array()))
      .toBe('synthetic-kun-custom-marker')
    expect(await readFile(kunSettingsPath)).toEqual(kunRaw)
    await expect(stat(currentSettingsPath)).rejects.toMatchObject({ code: 'ENOENT' })
    clearInspectionCredentials(kunInspection)
  })

  it('maps key-free legacy Provider metadata into strict Registry inputs deterministically', async () => {
    const { inspection } = await inspectCurrentSettings({
      version: 1,
      runtime: { model: 'anthropic-model' },
      provider: {
        activeProviderId: 'anthropic-provider',
        proxy: { enabled: true, url: 'https://proxy.example:8443' },
        providers: [
          {
            id: 'deepseek',
            apiKey: 'synthetic-deepseek-mapping-marker',
            baseUrl: 'https://deepseek.example/v1',
            endpointFormat: 'chat_completions',
            models: ['deepseek-model']
          },
          {
            id: 'openai-provider',
            apiKey: 'synthetic-openai-mapping-marker',
            baseUrl: 'https://openai.example/v1',
            endpointFormat: 'responses',
            models: ['openai-model']
          },
          {
            id: 'anthropic-provider',
            apiKey: 'synthetic-anthropic-mapping-marker',
            baseUrl: 'https://anthropic.example/v1',
            endpointFormat: 'messages',
            models: ['anthropic-model', 'other-model'],
            image: { baseUrl: 'https://media.example/image', models: ['image-model', 'shared-model'] },
            speech: { baseUrl: 'https://media.example/speech', models: ['speech-model', 'shared-model'] },
            textToSpeech: { baseUrl: 'https://media.example/tts', models: ['tts-model', 'shared-model'] },
            music: { baseUrl: 'https://media.example/music', models: ['music-model', 'shared-model'] },
            video: { baseUrl: 'https://media.example/video', models: ['video-model', 'shared-model'] }
          },
          {
            id: 'custom-provider',
            apiKey: 'synthetic-custom-mapping-marker',
            baseUrl: 'https://custom.example/v1/chat/completions',
            endpointFormat: 'custom_endpoint',
            models: ['custom-model']
          }
        ]
      }
    })
    const mapped = Object.fromEntries(inspection.candidates.map((candidate) => [
      candidate.providerId,
      mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(candidate)
    ]))

    expect(mapped.deepseek).toMatchObject({
      id: 'deepseek',
      kind: 'deepseek',
      endpoint: 'https://deepseek.example/v1',
      proxy: 'https://proxy.example:8443/',
      models: ['deepseek-model'],
      selectedModel: ''
    })
    expect(mapped['openai-provider']).toMatchObject({
      kind: 'openai-compatible',
      endpoint: 'https://openai.example/v1',
      selectedModel: ''
    })
    expect(mapped['anthropic-provider']).toEqual({
      id: 'anthropic-provider',
      kind: 'anthropic-compatible',
      endpoint: 'https://anthropic.example/v1',
      proxy: 'https://proxy.example:8443/',
      models: ['anthropic-model', 'other-model'],
      mediaModels: [
        'image-model',
        'shared-model',
        'speech-model',
        'tts-model',
        'music-model',
        'video-model'
      ],
      selectedModel: 'anthropic-model',
      selectedMediaModel: '',
      selectedRoutes: []
    })
    expect(mapped['custom-provider']).toMatchObject({
      kind: 'custom-endpoint',
      endpoint: 'https://custom.example/v1/chat/completions',
      selectedModel: ''
    })
    clearInspectionCredentials(inspection)

    const defaultSelection = await inspectCurrentSettings({
      version: 1,
      runtime: { model: 'deepseek-model' },
      provider: {
        proxy: { enabled: false, url: 'https://ignored.example:8443' },
        providers: [{
          id: 'deepseek',
          apiKey: 'synthetic-default-selection-marker',
          baseUrl: 'https://deepseek.example/v1',
          models: ['deepseek-model']
        }]
      }
    })
    expect(mapLegacyProviderCredentialCandidateToProviderRegistryProviderInputV1(
      defaultSelection.inspection.candidates[0]
    )).toMatchObject({
      id: 'deepseek',
      proxy: '',
      selectedModel: 'deepseek-model'
    })
    clearInspectionCredentials(defaultSelection.inspection)
  })

  it('fails inspection on Registry-unsupported enabled proxy or Provider metadata without source mutation', async () => {
    const cases = [
      {
        name: 'unsupported enabled proxy',
        value: {
          version: 1,
          provider: {
            proxy: { enabled: true, url: 'socks5://proxy.example:1080' },
            providers: [{
              id: 'custom-provider',
              apiKey: 'synthetic-unsupported-proxy-marker',
              baseUrl: 'https://custom.example/v1',
              models: ['custom-model']
            }]
          }
        }
      },
      {
        name: 'missing Provider endpoint',
        value: {
          version: 1,
          provider: {
            providers: [{
              id: 'custom-provider',
              apiKey: 'synthetic-unsupported-metadata-marker',
              baseUrl: '',
              models: ['custom-model']
            }]
          }
        }
      },
      {
        name: 'unknown active Provider',
        value: {
          version: 1,
          provider: {
            activeProviderId: 'missing-provider',
            providers: [{
              id: 'custom-provider',
              apiKey: 'synthetic-unknown-active-provider-marker',
              baseUrl: 'https://custom.example/v1',
              models: ['custom-model']
            }]
          }
        }
      }
    ]
    for (const testCase of cases) {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-registry-mapping-failure-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      const raw = Buffer.from(JSON.stringify(testCase.value), 'utf8')
      await writeFile(settingsPath, raw)
      const beforeEntries = await readdir(userDataDir)

      await expect(new JsonSettingsStore(userDataDir).inspectLegacyProviderCredentialSources(), testCase.name)
        .rejects.toThrow('Legacy Provider credential source inspection failed.')
      expect(await readFile(settingsPath), testCase.name).toEqual(raw)
      expect(await readdir(userDataDir), testCase.name).toEqual(beforeEntries)
    }
  })

  it('rejects invalid UTF-8 and opaque URL credential carriers before producing candidates', async () => {
    const invalidUtf8Raw = Buffer.concat([
      Buffer.from('{"provider":{"apiKey":"synthetic-invalid-utf8-', 'utf8'),
      Buffer.from([0xc3, 0x28]),
      Buffer.from('"}}', 'utf8')
    ])
    const urlCases = [
      'https://provider.example/v1?opaque=synthetic-canary',
      'https://provider.example/v1?api_key=synthetic-canary',
      'https://provider.example/v1?',
      'https://provider.example/v1#synthetic-canary',
      'https://provider.example/v1#token=synthetic-canary',
      'https://provider.example/v1#',
      'https://synthetic-user:synthetic-password@provider.example/v1'
    ]
    const rejectedSources = [
      { name: 'invalid UTF-8', raw: invalidUtf8Raw },
      {
        name: 'invalid Provider URL',
        raw: Buffer.from(JSON.stringify({
          provider: {
            providers: [{
              id: 'custom-invalid-url',
              apiKey: 'synthetic-invalid-url-marker',
              baseUrl: 'not-a-valid-provider-url'
            }]
          }
        }), 'utf8')
      },
      ...urlCases.map((baseUrl) => ({
        name: baseUrl,
        raw: Buffer.from(JSON.stringify({
          provider: {
            providers: [{
              id: 'custom-carrier',
              apiKey: 'synthetic-carrier-marker',
              baseUrl
            }]
          }
        }), 'utf8')
      })),
      {
        name: 'canonical top-level Provider URL',
        raw: Buffer.from(JSON.stringify({
          provider: {
            apiKey: 'synthetic-top-level-carrier-marker',
            baseUrl: 'https://provider.example/v1?opaque=synthetic-canary'
          }
        }), 'utf8')
      },
      {
        name: 'canonical capability URL',
        raw: Buffer.from(JSON.stringify({
          provider: {
            providers: [{
              id: 'custom-capability-carrier',
              apiKey: 'synthetic-capability-carrier-marker',
              baseUrl: 'https://provider.example/v1',
              image: {
                baseUrl: 'https://image.example/v1#synthetic-canary',
                models: ['synthetic-image-model']
              }
            }]
          }
        }), 'utf8')
      },
      {
        name: 'runtime seed URL',
        raw: Buffer.from(JSON.stringify({
          runtime: {
            apiKey: 'synthetic-runtime-carrier-marker',
            baseUrl: 'https://runtime.example/v1?'
          }
        }), 'utf8')
      },
      {
        name: 'implicit DeepSeek seed URL',
        raw: Buffer.from(JSON.stringify({
          deepseek: {
            apiKey: 'synthetic-deepseek-carrier-marker',
            baseUrl: 'https://deepseek.example/v1#'
          }
        }), 'utf8')
      },
      {
        name: 'Reasonix seed URL',
        raw: Buffer.from(JSON.stringify({
          agentProvider: 'reasonix',
          agents: {
            reasonix: {
              apiKey: 'synthetic-reasonix-carrier-marker',
              baseUrl: 'https://synthetic-user:synthetic-password@reasonix.example/v1'
            }
          }
        }), 'utf8')
      },
      {
        name: 'Codewhale merged DeepSeek URL',
        raw: Buffer.from(JSON.stringify({
          agentProvider: 'codewhale',
          agents: {
            codewhale: {
              apiKey: 'synthetic-codewhale-carrier-marker',
              baseUrl: 'https://codewhale.example/v1'
            }
          },
          deepseek: {
            baseUrl: 'https://deepseek-override.example/v1?opaque=synthetic-canary'
          }
        }), 'utf8')
      },
      {
        name: 'DeepSeek runtime seed URL',
        raw: Buffer.from(JSON.stringify({
          agentProvider: 'deepseek-runtime',
          deepseek: {
            apiKey: 'synthetic-deepseek-runtime-carrier-marker',
            baseUrl: 'https://deepseek-runtime.example/v1#synthetic-canary'
          }
        }), 'utf8')
      }
    ]

    for (const testCase of rejectedSources) {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-carrier-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      await writeFile(settingsPath, testCase.raw, { mode: 0o640 })
      const beforeEntries = await readdir(userDataDir)
      const beforeMode = (await stat(settingsPath)).mode & 0o777
      const error = await new JsonSettingsStore(userDataDir)
        .inspectLegacyProviderCredentialSources()
        .then(() => null, (caught: unknown) => caught)

      expect(error, testCase.name).toBeInstanceOf(Error)
      expect((error as Error).message, testCase.name)
        .toBe('Legacy Provider credential source inspection failed.')
      expect((error as Error).message, testCase.name).not.toContain('synthetic-')
      expect((error as Error).message, testCase.name).not.toContain(settingsPath)
      expect(await readFile(settingsPath), testCase.name).toEqual(testCase.raw)
      expect(await readdir(userDataDir), testCase.name).toEqual(beforeEntries)
      expect((await stat(settingsPath)).mode & 0o777, testCase.name).toBe(beforeMode)
    }

    const kunParentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-kun-carrier-'))
    const currentUserDataDir = join(kunParentDir, 'AnalytixCurrent')
    const kunUserDataDir = join(kunParentDir, 'Kun')
    const currentSettingsPath = join(currentUserDataDir, 'analytix-settings.json')
    const kunSettingsPath = join(kunUserDataDir, 'kun-settings.json')
    const kunRaw = Buffer.from(JSON.stringify({
      agents: {
        kun: {
          apiKey: 'synthetic-kun-carrier-marker',
          baseUrl: 'https://kun.example/v1?opaque=synthetic-canary'
        }
      }
    }), 'utf8')
    await mkdir(currentUserDataDir, { recursive: true })
    await mkdir(kunUserDataDir, { recursive: true })
    await writeFile(kunSettingsPath, kunRaw, { mode: 0o640 })
    const beforeKunEntries = await readdir(kunUserDataDir)
    const beforeKunMode = (await stat(kunSettingsPath)).mode & 0o777
    const kunError = await new JsonSettingsStore(currentUserDataDir)
      .inspectLegacyProviderCredentialSources()
      .then(() => null, (caught: unknown) => caught)

    expect(kunError).toBeInstanceOf(Error)
    expect((kunError as Error).message).toBe('Legacy Provider credential source inspection failed.')
    expect((kunError as Error).message).not.toContain('synthetic-')
    expect((kunError as Error).message).not.toContain(kunSettingsPath)
    expect(await readFile(kunSettingsPath)).toEqual(kunRaw)
    expect(await readdir(kunUserDataDir)).toEqual(beforeKunEntries)
    expect((await stat(kunSettingsPath)).mode & 0o777).toBe(beforeKunMode)
    await expect(stat(currentSettingsPath)).rejects.toMatchObject({ code: 'ENOENT' })

    const valid = await inspectCurrentSettings({
      provider: {
        providers: [{
          id: 'custom-clean-url',
          apiKey: 'synthetic-clean-url-marker',
          baseUrl: 'https://provider.example/v1',
          image: {
            baseUrl: 'https://image.example/v1',
            models: ['synthetic-image-model']
          }
        }]
      }
    })
    expect(valid.inspection.candidates).toHaveLength(1)
    expect(valid.inspection.candidates[0].providerMetadata.profile.baseUrl)
      .toBe('https://provider.example/v1')
    expect(valid.inspection.candidates[0].providerMetadata.profile.image?.baseUrl)
      .toBe('https://image.example/v1')
    expect(decodeCredential(valid.inspection.candidates[0].credential))
      .toBe('synthetic-clean-url-marker')
    expect(await readFile(valid.settingsPath)).toEqual(valid.raw)
    clearInspectionCredentials(valid.inspection)
  })

  it('uses the existing settings compatibility precedence without creating the current file', async () => {
    const cases = [
      {
        source: ['AnalytixCurrent', 'kun-settings.json'],
        locator: 'compatibility:01:kun-settings.json',
        value: { agents: { kun: { apiKey: 'synthetic-current-kun-precedence-marker' } } },
        marker: 'synthetic-current-kun-precedence-marker'
      },
      {
        source: ['Kun', 'analytix-settings.json'],
        locator: 'compatibility:02:analytix-settings.json',
        value: { provider: { apiKey: 'synthetic-parent-kun-analytix-marker' } },
        marker: 'synthetic-parent-kun-analytix-marker'
      },
      {
        source: ['Kun', 'kun-settings.json'],
        locator: 'compatibility:03:kun-settings.json',
        value: { agents: { kun: { apiKey: 'synthetic-parent-kun-marker' } } },
        marker: 'synthetic-parent-kun-marker'
      },
      {
        source: ['analytix', 'analytix-settings.json'],
        locator: 'compatibility:04:analytix-settings.json',
        value: { provider: { apiKey: 'synthetic-parent-lowercase-marker' } },
        marker: 'synthetic-parent-lowercase-marker'
      },
      {
        source: ['DeepSeek GUI', 'analytix-settings.json'],
        locator: 'compatibility:05:analytix-settings.json',
        value: { provider: { apiKey: 'synthetic-parent-deepseek-gui-marker' } },
        marker: 'synthetic-parent-deepseek-gui-marker'
      }
    ]

    for (const testCase of cases) {
      const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-precedence-'))
      const currentUserDataDir = join(parentDir, 'AnalytixCurrent')
      const sourcePath = join(parentDir, ...testCase.source)
      const raw = Buffer.from(JSON.stringify(testCase.value), 'utf8')
      await mkdir(join(parentDir, testCase.source[0]), { recursive: true })
      await writeFile(sourcePath, raw)
      const inspection = await new JsonSettingsStore(currentUserDataDir)
        .inspectLegacyProviderCredentialSources()
      expect(inspection.sourceLocator, testCase.locator).toBe(testCase.locator)
      expect(inspection.sourceSHA256, testCase.locator)
        .toBe(createHash('sha256').update(raw).digest('hex'))
      expect(decodeCredential(inspection.candidates[0].credential), testCase.locator)
        .toBe(testCase.marker)
      expect(await readFile(sourcePath), testCase.locator).toEqual(raw)
      await expect(stat(join(currentUserDataDir, 'analytix-settings.json')))
        .rejects.toMatchObject({ code: 'ENOENT' })
      clearInspectionCredentials(inspection)
    }

    const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-current-wins-'))
    const currentUserDataDir = join(parentDir, 'AnalytixCurrent')
    const currentSettingsPath = join(currentUserDataDir, 'analytix-settings.json')
    const currentRaw = Buffer.from(JSON.stringify({ provider: {} }), 'utf8')
    await mkdir(currentUserDataDir, { recursive: true })
    await writeFile(currentSettingsPath, currentRaw)
    await writeFile(join(currentUserDataDir, 'kun-settings.json'), JSON.stringify({
      agents: { kun: { apiKey: 'synthetic-ignored-kun-marker' } }
    }), 'utf8')
    const currentInspection = await new JsonSettingsStore(currentUserDataDir)
      .inspectLegacyProviderCredentialSources()
    expect(currentInspection.sourceLocator).toBe('current:analytix-settings.json')
    expect(currentInspection.candidates).toEqual([])
    expect(await readFile(currentSettingsPath)).toEqual(currentRaw)

    const currentKunEnvelope = await inspectCurrentSettings({
      agents: { kun: { apiKey: 'synthetic-current-file-kun-marker' } }
    })
    expect(currentKunEnvelope.inspection.candidates).toEqual([])

    const absentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-absent-'))
    const absentInspection = await new JsonSettingsStore(absentDir)
      .inspectLegacyProviderCredentialSources()
    expect(absentInspection).toEqual({
      schemaVersion: 1,
      sourceLocator: null,
      sourceSHA256: null,
      expectedCleanedSourceSHA256: null,
      sourcePhysicalIdentitySHA256: null,
      sourceSnapshot: null,
      candidates: []
    })
    await expect(stat(join(absentDir, 'analytix-settings.json'))).rejects.toMatchObject({ code: 'ENOENT' })
  })

  it('does not read through or mutate the settings cache during credential inspection', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-cache-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const workspaceRoot = join(userDataDir, 'workspace')
    const writeRoot = join(userDataDir, 'write')
    const cachedRaw = Buffer.from(JSON.stringify({
      version: 1,
      workspaceRoot,
      write: {
        defaultWorkspaceRoot: writeRoot,
        activeWorkspaceRoot: writeRoot,
        workspaces: [writeRoot]
      },
      runtime: {
        executionPolicyVersion: 2,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write'
      },
      provider: { apiKey: 'synthetic-cached-marker' }
    }), 'utf8')
    await writeFile(settingsPath, cachedRaw)
    const store = new JsonSettingsStore(userDataDir)
    const cached = await store.load()
    expect(JSON.stringify(cached).includes('synthetic-cached-marker')).toBe(false)

    const inspectedRaw = Buffer.from(JSON.stringify({
      version: 1,
      provider: { apiKey: 'synthetic-on-disk-marker' }
    }), 'utf8')
    await writeFile(settingsPath, inspectedRaw)
    const inspection = await store.inspectLegacyProviderCredentialSources()

    expect(decodeCredential(inspection.candidates[0].credential)).toBe('synthetic-on-disk-marker')
    expect(JSON.stringify(await store.load()).includes('synthetic-cached-marker')).toBe(false)
    expect(await readFile(settingsPath)).toEqual(inspectedRaw)
    clearInspectionCredentials(inspection)
  })

  it('captures every precedence-shadowed legacy credential for protected rollback before source cleanup', async () => {
    const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-shadowed-kun-'))
    const currentUserDataDir = join(parentDir, 'AnalytixCurrent')
    const kunUserDataDir = join(parentDir, 'Kun')
    const kunSettingsPath = join(kunUserDataDir, 'kun-settings.json')
    const kunRaw = Buffer.from(JSON.stringify({
      provider: {
        apiKey: 'synthetic-kun-shadow-top-level',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'synthetic-kun-shadow-profile',
            baseUrl: 'https://api.deepseek.com'
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'synthetic-kun-shadow-profile',
            baseUrl: 'https://api.deepseek.com'
          }
        ]
      },
      agents: {
        kun: {
          apiKey: 'synthetic-kun-active',
          baseUrl: 'https://kun.example/v1',
          model: 'synthetic-kun-model'
        }
      }
    }), 'utf8')
    await mkdir(kunUserDataDir, { recursive: true })
    await writeFile(kunSettingsPath, kunRaw)

    const kunInspection = await new JsonSettingsStore(currentUserDataDir)
      .inspectLegacyProviderCredentialSources()
    const kunSourceLocator = kunInspection.sourceLocator ?? ''
    const kunCandidate = kunInspection.candidates.find((candidate) => candidate.providerId === 'deepseek')
    expect(decodeCredential(kunCandidate?.credential ?? new Uint8Array())).toBe('synthetic-kun-active')
    expect(kunCandidate?.credentialLocators).toEqual([
      `${kunSourceLocator}:agents.kun.apiKey`
    ])
    expect(kunCandidate?.rollbackCredentialArtifacts.map((artifact) => ({
      schemaVersion: artifact.schemaVersion,
      sourceLocator: artifact.sourceLocator,
      credentialLocators: artifact.credentialLocators,
      credential: decodeCredential(artifact.credential)
    }))).toEqual([
      {
        schemaVersion: 1,
        sourceLocator: `${kunSourceLocator}:provider.apiKey`,
        credentialLocators: [`${kunSourceLocator}:provider.apiKey`],
        credential: 'synthetic-kun-shadow-top-level'
      },
      {
        schemaVersion: 1,
        sourceLocator: `${kunSourceLocator}:provider.providers[0].apiKey`,
        credentialLocators: [
          `${kunSourceLocator}:provider.providers[0].apiKey`,
          `${kunSourceLocator}:provider.providers[1].apiKey`
        ],
        credential: 'synthetic-kun-shadow-profile'
      }
    ])
    expect(kunCandidate?.credential).not.toBe(kunCandidate?.rollbackCredentialArtifacts[0].credential)
    expect(kunCandidate?.rollbackCredentialArtifacts[0].credential)
      .not.toBe(kunCandidate?.rollbackCredentialArtifacts[1].credential)
    expect(await readFile(kunSettingsPath)).toEqual(kunRaw)

    const codewhaleDifferent = await inspectCurrentSettings({
      agentProvider: 'codewhale',
      agents: {
        codewhale: { apiKey: 'synthetic-codewhale-shadow' }
      },
      deepseek: { apiKey: 'synthetic-codewhale-active' }
    })
    expect(codewhaleDifferent.inspection.candidates[0].credentialLocators).toEqual([
      'current:analytix-settings.json:deepseek.apiKey'
    ])
    expect(decodeCredential(codewhaleDifferent.inspection.candidates[0].credential))
      .toBe('synthetic-codewhale-active')
    expect(codewhaleDifferent.inspection.candidates[0].rollbackCredentialArtifacts.map((artifact) => ({
      credentialLocators: artifact.credentialLocators,
      credential: decodeCredential(artifact.credential)
    }))).toEqual([{
      credentialLocators: ['current:analytix-settings.json:agents.codewhale.apiKey'],
      credential: 'synthetic-codewhale-shadow'
    }])

    const codewhaleIdentical = await inspectCurrentSettings({
      agentProvider: 'codewhale',
      agents: {
        codewhale: { apiKey: 'synthetic-codewhale-identical' }
      },
      deepseek: { apiKey: 'synthetic-codewhale-identical' }
    })
    expect(codewhaleIdentical.inspection.candidates[0].credentialLocators).toEqual([
      'current:analytix-settings.json:deepseek.apiKey',
      'current:analytix-settings.json:agents.codewhale.apiKey'
    ])
    expect(codewhaleIdentical.inspection.candidates[0].rollbackCredentialArtifacts).toEqual([])

    const canonical = await inspectCurrentSettings({
      provider: {
        activeProviderId: 'custom-shadowed',
        apiKey: 'synthetic-canonical-active',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'synthetic-canonical-active',
            baseUrl: 'https://api.deepseek.com'
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'synthetic-canonical-shadow',
            baseUrl: 'https://api.deepseek.com'
          },
          {
            id: 'custom-shadowed',
            name: 'Synthetic Custom',
            apiKey: 'synthetic-custom-active',
            baseUrl: 'https://custom.example/v1'
          },
          {
            id: 'custom-shadowed',
            name: 'Synthetic Custom',
            apiKey: 'synthetic-custom-shadow',
            baseUrl: 'https://custom.example/v1'
          },
          {
            id: 'custom-shadowed',
            name: 'Synthetic Custom',
            apiKey: 'synthetic-custom-shadow',
            baseUrl: 'https://custom.example/v1'
          }
        ]
      }
    })
    const canonicalDefault = canonical.inspection.candidates.find((candidate) => candidate.providerId === 'deepseek')
    const canonicalCustom = canonical.inspection.candidates.find((candidate) => candidate.providerId === 'custom-shadowed')
    expect(canonicalDefault?.credentialLocators).toEqual([
      'current:analytix-settings.json:provider.apiKey',
      'current:analytix-settings.json:provider.providers[0].apiKey'
    ])
    expect(canonicalDefault?.rollbackCredentialArtifacts.map((artifact) => ({
      credentialLocators: artifact.credentialLocators,
      credential: decodeCredential(artifact.credential)
    }))).toEqual([{
      credentialLocators: ['current:analytix-settings.json:provider.providers[1].apiKey'],
      credential: 'synthetic-canonical-shadow'
    }])
    expect(canonicalCustom?.credentialLocators).toEqual([
      'current:analytix-settings.json:provider.providers[2].apiKey'
    ])
    expect(canonicalCustom?.rollbackCredentialArtifacts.map((artifact) => ({
      credentialLocators: artifact.credentialLocators,
      credential: decodeCredential(artifact.credential)
    }))).toEqual([{
      credentialLocators: [
        'current:analytix-settings.json:provider.providers[3].apiKey',
        'current:analytix-settings.json:provider.providers[4].apiKey'
      ],
      credential: 'synthetic-custom-shadow'
    }])

    const sentinelShadow = await inspectCurrentSettings({
      provider: {
        apiKey: 'synthetic-sentinel-active',
        providers: [
          { id: 'deepseek', apiKey: '[REDACTED]' },
          { id: 'deepseek', apiKey: '********' }
        ]
      }
    })
    expect(decodeCredential(sentinelShadow.inspection.candidates[0].credential))
      .toBe('synthetic-sentinel-active')
    expect(sentinelShadow.inspection.candidates[0].rollbackCredentialArtifacts.map((artifact) => ({
      credentialLocators: artifact.credentialLocators,
      credential: decodeCredential(artifact.credential)
    }))).toEqual([
      {
        credentialLocators: ['current:analytix-settings.json:provider.providers[0].apiKey'],
        credential: '[REDACTED]'
      },
      {
        credentialLocators: ['current:analytix-settings.json:provider.providers[1].apiKey'],
        credential: '********'
      }
    ])

    const invalidCases = [
      {
        name: 'unsupported type',
        value: { provider: { apiKey: 'synthetic-valid', providers: [{ id: 'deepseek', apiKey: 42 }] } }
      },
      {
        name: 'ambiguous normalized identity',
        value: {
          provider: {
            providers: [
              { id: 'custom identity', name: 'Same', apiKey: 'synthetic-one' },
              { id: 'custom-identity', name: 'Same', apiKey: 'synthetic-two' }
            ]
          }
        }
      },
      {
        name: 'metadata conflict',
        value: {
          provider: {
            providers: [
              { id: 'custom-conflict', name: 'First', apiKey: 'synthetic-one' },
              { id: 'custom-conflict', name: 'Second', apiKey: 'synthetic-two' }
            ]
          }
        }
      },
      {
        name: 'provider id field bound',
        value: { provider: { providers: [{ id: 'a'.repeat(65), apiKey: 'synthetic-field-bound' }] } }
      },
      {
        name: 'global credential count bound',
        value: {
          provider: {
            apiKey: 'synthetic-count-active',
            providers: Array.from({ length: 64 }, (_, index) => ({
              id: `custom-count-${index}`,
              apiKey: `synthetic-count-${index}`
            }))
          }
        }
      },
      {
        name: 'global decoded credential byte bound',
        value: {
          provider: {
            providers: [
              { id: 'custom-bytes-one', apiKey: `synthetic-${'a'.repeat(600_000)}` },
              { id: 'custom-bytes-two', apiKey: `synthetic-${'b'.repeat(600_000)}` }
            ]
          }
        }
      }
    ]
    for (const testCase of invalidCases) {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-shadowed-invalid-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      const raw = Buffer.from(JSON.stringify(testCase.value), 'utf8')
      await writeFile(settingsPath, raw)
      const beforeEntries = await readdir(userDataDir)
      const error = await new JsonSettingsStore(userDataDir)
        .inspectLegacyProviderCredentialSources()
        .then(() => null, (caught: unknown) => caught)
      expect(error, testCase.name).toBeInstanceOf(Error)
      expect((error as Error).message, testCase.name)
        .toBe('Legacy Provider credential source inspection failed.')
      expect((error as Error).message, testCase.name).not.toContain('synthetic-')
      expect((error as Error).message, testCase.name).not.toContain(settingsPath)
      expect(await readFile(settingsPath), testCase.name).toEqual(raw)
      expect(await readdir(userDataDir), testCase.name).toEqual(beforeEntries)
    }

    kunCandidate?.rollbackCredentialArtifacts[0].credential.fill(0)
    expect(decodeCredential(kunCandidate?.credential ?? new Uint8Array())).toBe('synthetic-kun-active')
    expect(decodeCredential(kunCandidate?.rollbackCredentialArtifacts[1].credential ?? new Uint8Array()))
      .toBe('synthetic-kun-shadow-profile')
    const repeatedKun = await new JsonSettingsStore(currentUserDataDir)
      .inspectLegacyProviderCredentialSources()
    const repeatedKunCandidate = repeatedKun.candidates.find((candidate) => candidate.providerId === 'deepseek')
    expect(decodeCredential(repeatedKunCandidate?.credential ?? new Uint8Array()))
      .toBe('synthetic-kun-active')
    expect(repeatedKunCandidate?.rollbackCredentialArtifacts.map((artifact) => decodeCredential(artifact.credential)))
      .toEqual(['synthetic-kun-shadow-top-level', 'synthetic-kun-shadow-profile'])
    expect(await readFile(kunSettingsPath)).toEqual(kunRaw)

    const inspections = [
      kunInspection,
      repeatedKun,
      codewhaleDifferent.inspection,
      codewhaleIdentical.inspection,
      canonical.inspection,
      sentinelShadow.inspection
    ]
    const returnedSecretBuffers = inspections.flatMap((inspection) => inspection.candidates.flatMap((candidate) => [
      candidate.credential,
      ...candidate.rollbackCredentialArtifacts.map((artifact) => artifact.credential)
    ]))
    for (const inspection of inspections) clearInspectionCredentials(inspection)
    for (const secretBuffer of returnedSecretBuffers) {
      expect(secretBuffer.every((byte) => byte === 0)).toBe(true)
    }
  })

  it('does not promote a shadowed CodeWhale credential when the selected credential is redacted', async () => {
    for (const selectedCredential of ['', '[REDACTED]']) {
      const { userDataDir, settingsPath, raw, inspection } = await inspectCurrentSettings({
        agentProvider: 'codewhale',
        agents: {
          codewhale: { apiKey: 'synthetic-codewhale-shadow-only' }
        },
        deepseek: { apiKey: selectedCredential }
      }, 'analytix-settings-codewhale-redacted-selection-')
      const entries = await readdir(userDataDir)

      expect(inspection.candidates, selectedCredential || 'empty').toEqual([])
      expect(await readFile(settingsPath), selectedCredential || 'empty').toEqual(raw)
      expect(await readdir(userDataDir), selectedCredential || 'empty').toEqual(entries)

      const repeated = await new JsonSettingsStore(userDataDir)
        .inspectLegacyProviderCredentialSources()
      expect(repeated.candidates, selectedCredential || 'empty').toEqual([])
      expect(await readFile(settingsPath), selectedCredential || 'empty').toEqual(raw)
      expect(await readdir(userDataDir), selectedCredential || 'empty').toEqual(entries)
    }

    const fallback = await inspectCurrentSettings({
      agentProvider: 'codewhale',
      agents: {
        codewhale: { apiKey: 'synthetic-codewhale-fallback-active' }
      },
      deepseek: { baseUrl: 'https://deepseek.example/v1' }
    }, 'analytix-settings-codewhale-fallback-')
    expect(fallback.inspection.candidates).toHaveLength(1)
    expect(fallback.inspection.candidates[0].credentialLocators).toEqual([
      'current:analytix-settings.json:agents.codewhale.apiKey'
    ])
    expect(decodeCredential(fallback.inspection.candidates[0].credential))
      .toBe('synthetic-codewhale-fallback-active')
    expect(fallback.inspection.candidates[0].rollbackCredentialArtifacts).toEqual([])
    expect(await readFile(fallback.settingsPath)).toEqual(fallback.raw)
    expect(await readdir(fallback.userDataDir)).toEqual(['analytix-settings.json'])
    clearInspectionCredentials(fallback.inspection)
  })

  it('deduplicates identical Provider credentials and fails closed on metadata conflicts', async () => {
    const identical = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-identical-marker',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'synthetic-identical-marker',
            baseUrl: 'https://api.deepseek.com'
          },
          {
            id: 'custom-identical',
            name: 'Synthetic',
            apiKey: 'synthetic-custom-identical-marker',
            baseUrl: 'https://custom.example/v1'
          },
          {
            id: 'custom-identical',
            name: 'Synthetic',
            apiKey: 'synthetic-custom-identical-marker',
            baseUrl: 'https://custom.example/v1'
          }
        ]
      }
    })
    expect(identical.inspection.candidates).toHaveLength(2)
    const defaultCandidate = identical.inspection.candidates.find((candidate) => candidate.providerId === 'deepseek')
    const customCandidate = identical.inspection.candidates.find((candidate) => candidate.providerId === 'custom-identical')
    expect(defaultCandidate?.credentialLocators).toEqual([
      'current:analytix-settings.json:provider.apiKey',
      'current:analytix-settings.json:provider.providers[0].apiKey'
    ])
    expect(customCandidate?.credentialLocators).toEqual([
      'current:analytix-settings.json:provider.providers[1].apiKey',
      'current:analytix-settings.json:provider.providers[2].apiKey'
    ])
    clearInspectionCredentials(identical.inspection)

    const conflicts = [{
      provider: {
        providers: [
          { id: 'custom-conflict', name: 'First', apiKey: 'synthetic-same-marker' },
          { id: 'custom-conflict', name: 'Second', apiKey: 'synthetic-same-marker' }
        ]
      }
    }]
    for (const value of conflicts) {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-conflict-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      const raw = Buffer.from(JSON.stringify(value), 'utf8')
      await writeFile(settingsPath, raw)
      const error = await new JsonSettingsStore(userDataDir)
        .inspectLegacyProviderCredentialSources()
        .then(() => null, (caught: unknown) => caught)
      expect(error).toBeInstanceOf(Error)
      expect((error as Error).message).toBe('Legacy Provider credential source inspection failed.')
      expect((error as Error).message).not.toContain('synthetic-')
      expect((error as Error).message).not.toContain(settingsPath)
      expect(await readFile(settingsPath)).toEqual(raw)
    }
  })

  it('treats empty and redacted credential sentinels as non-destructive non-candidates', async () => {
    const sentinels = [
      '', '   ', 'redacted', '[REDACTED]', '<redacted>', '__redacted__',
      'masked', '[MASKED]', '<masked>', '__masked__', 'unset', 'not-set',
      'null', 'undefined', '********', '••••••••', 'sk-********', 'xxxxxx'
    ]
    for (const sentinel of sentinels) {
      const { settingsPath, raw, inspection } = await inspectCurrentSettings({
        provider: { apiKey: sentinel }
      }, 'analytix-settings-inspection-sentinel-')
      expect(inspection.candidates, sentinel).toEqual([])
      expect(await readFile(settingsPath), sentinel).toEqual(raw)
    }

    const preserved = await inspectCurrentSettings({
      provider: {
        apiKey: '[REDACTED]',
        providers: [{ id: 'deepseek', apiKey: 'synthetic-preserved-profile-marker' }]
      }
    })
    expect(preserved.inspection.candidates).toHaveLength(1)
    expect(preserved.inspection.candidates[0].sourceLocator).toBe(
      'current:analytix-settings.json:provider.providers[0].apiKey'
    )
    expect(decodeCredential(preserved.inspection.candidates[0].credential))
      .toBe('synthetic-preserved-profile-marker')
    clearInspectionCredentials(preserved.inspection)

    const canonicalAuthority = await inspectCurrentSettings({
      provider: {},
      runtime: { apiKey: 'synthetic-ignored-runtime-marker' }
    })
    expect(canonicalAuthority.inspection.candidates).toEqual([])
  })

  describe('fails closed on malformed, unsupported, or over-limit credential sources without side effects', () => {
    const values: Array<{ name: string; raw: Buffer }> = [
      { name: 'malformed JSON', raw: Buffer.from('{ invalid json', 'utf8') },
      { name: 'non-object root', raw: Buffer.from('[]', 'utf8') },
      { name: 'unsupported Provider credential type', raw: Buffer.from(JSON.stringify({ provider: { apiKey: 42 } }), 'utf8') },
      { name: 'unsupported Provider list type', raw: Buffer.from(JSON.stringify({ provider: { providers: {} } }), 'utf8') },
      { name: 'unsupported Provider id type', raw: Buffer.from(JSON.stringify({ provider: { providers: [{ id: 42, apiKey: 'synthetic-marker' }] } }), 'utf8') },
      { name: 'unsupported Provider model type', raw: Buffer.from(JSON.stringify({ provider: { providers: [{ id: 'custom', apiKey: 'synthetic-marker', models: [42] }] } }), 'utf8') },
      { name: 'unsupported runtime credential type', raw: Buffer.from(JSON.stringify({ runtime: { apiKey: false } }), 'utf8') },
      { name: 'credential-bearing profile metadata URL', raw: Buffer.from(JSON.stringify({ provider: { providers: [{ id: 'custom', apiKey: 'synthetic-marker', baseUrl: 'https://provider.example/v1?api_key=synthetic-query-value' }] } }), 'utf8') },
      {
        name: 'over-limit candidate count',
        raw: Buffer.from(JSON.stringify({
          provider: {
            apiKey: 'synthetic-top-level-marker',
            providers: Array.from({ length: 64 }, (_, index) => ({
              id: `custom-${index}`,
              apiKey: `synthetic-marker-${index}`
            }))
          }
        }), 'utf8')
      },
      {
        name: 'over-limit credential bytes',
        raw: Buffer.from(JSON.stringify({ provider: { apiKey: `synthetic-${'a'.repeat((1 << 20) + 1)}` } }), 'utf8')
      },
      {
        name: 'over-limit source bytes',
        raw: Buffer.from(JSON.stringify({ padding: 'a'.repeat((2 << 20) + 1) }), 'utf8')
      }
    ]

    it.each(values)('$name', async (testCase) => {
      const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-invalid-'))
      const settingsPath = join(userDataDir, 'analytix-settings.json')
      await writeFile(settingsPath, testCase.raw, { mode: 0o640 })
      const beforeMode = (await stat(settingsPath)).mode & 0o777
      const beforeEntries = await readdir(userDataDir)
      const error = await new JsonSettingsStore(userDataDir)
        .inspectLegacyProviderCredentialSources()
        .then(() => null, (caught: unknown) => caught)

      expect(error, testCase.name).toBeInstanceOf(Error)
      expect((error as Error).message, testCase.name)
        .toBe('Legacy Provider credential source inspection failed.')
      expect((error as Error).message, testCase.name).not.toContain('synthetic-')
      expect((error as Error).message, testCase.name).not.toContain(settingsPath)
      expect(await readFile(settingsPath), testCase.name).toEqual(testCase.raw)
      expect(await readdir(userDataDir), testCase.name).toEqual(beforeEntries)
      expect((await stat(settingsPath)).mode & 0o777, testCase.name).toBe(beforeMode)
    })

    it('malformed compatibility source', async () => {
      const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-inspection-invalid-compat-'))
      const currentUserDataDir = join(parentDir, 'AnalytixCurrent')
      const legacyUserDataDir = join(parentDir, 'Kun')
      const legacySettingsPath = join(legacyUserDataDir, 'analytix-settings.json')
      const currentSettingsPath = join(currentUserDataDir, 'analytix-settings.json')
      await mkdir(legacyUserDataDir, { recursive: true })
      await writeFile(legacySettingsPath, '{ invalid legacy json', 'utf8')
      await expect(new JsonSettingsStore(currentUserDataDir).inspectLegacyProviderCredentialSources())
        .rejects.toThrow('Legacy Provider credential source inspection failed.')
      expect(await readFile(legacySettingsPath, 'utf8')).toBe('{ invalid legacy json')
      expect(await readdir(legacyUserDataDir)).toEqual(['analytix-settings.json'])
      await expect(stat(currentSettingsPath)).rejects.toMatchObject({ code: 'ENOENT' })
    })
  })

  it('returns independent candidate buffers and key-free metadata snapshots', async () => {
    const { userDataDir, inspection } = await inspectCurrentSettings({
      provider: {
        providers: [
          {
            id: 'custom-one',
            name: 'One',
            apiKey: 'synthetic-independent-one',
            models: ['shared-model']
          },
          {
            id: 'custom-two',
            name: 'Two',
            apiKey: 'synthetic-independent-two',
            models: ['shared-model']
          }
        ]
      }
    })
    expect(inspection.candidates).toHaveLength(2)
    inspection.candidates[0].providerMetadata.profile.models.push('mutated-model')
    inspection.candidates[0].credential.fill(0)
    expect(inspection.candidates[1].providerMetadata.profile.models).toEqual(['shared-model'])
    expect(decodeCredential(inspection.candidates[1].credential)).toBe('synthetic-independent-two')

    const repeated = await new JsonSettingsStore(userDataDir).inspectLegacyProviderCredentialSources()
    expect(repeated.candidates[0].providerMetadata.profile.models).toEqual(['shared-model'])
    expect(decodeCredential(repeated.candidates[0].credential)).toBe('synthetic-independent-one')
    clearInspectionCredentials(inspection)
    clearInspectionCredentials(repeated)
  })

  it('cleans exact Provider credential fields from the current canonical source', async () => {
    const currentCanonical = await inspectCurrentSettings({
      version: 1,
      provider: {
        activeProviderId: 'custom-cleanup',
        apiKey: 'synthetic-current-canonical-cleanup',
        unknownProviderMetadata: { preserve: true },
        providers: [
          {
            id: 'deepseek',
            apiKey: 'synthetic-current-canonical-cleanup',
            baseUrl: 'https://deepseek-cleanup.example/v1',
            models: ['deepseek-cleanup-model']
          },
          {
            id: 'custom-cleanup',
            apiKey: 'synthetic-current-custom-cleanup',
            baseUrl: 'https://custom-cleanup.example/v1',
            models: ['custom-cleanup-model'],
            unknownProfileMetadata: 'preserve-exactly'
          },
          {
            id: 'custom-cleanup',
            apiKey: '[REDACTED]',
            baseUrl: 'https://custom-cleanup.example/v1',
            models: ['custom-cleanup-model'],
            unknownProfileMetadata: 'preserve-exactly'
          }
        ]
      },
      unknownRootMetadata: { preserve: ['semantics'] }
    }, 'analytix-settings-cleanup-current-canonical-')
    const canonicalStore = new JsonSettingsStore(currentCanonical.userDataDir)
    await canonicalStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(currentCanonical.inspection)
    )
    const canonicalCleaned = JSON.parse(await readFile(currentCanonical.settingsPath, 'utf8'))
    expect(canonicalCleaned.provider).not.toHaveProperty('apiKey')
    expect(canonicalCleaned.provider.providers).toHaveLength(3)
    canonicalCleaned.provider.providers.forEach((provider: Record<string, unknown>) => {
      expect(provider).not.toHaveProperty('apiKey')
    })
    expect(canonicalCleaned.provider.unknownProviderMetadata).toEqual({ preserve: true })
    expect(canonicalCleaned.provider.providers[1].unknownProfileMetadata).toBe('preserve-exactly')
    expect(canonicalCleaned.unknownRootMetadata).toEqual({ preserve: ['semantics'] })
    expect(await canonicalStore.inspectLegacyProviderCredentialSources()).toMatchObject({ candidates: [] })
    if (process.platform !== 'win32') {
      expect((await stat(currentCanonical.settingsPath)).mode & 0o777).toBe(0o600)
    }
    clearInspectionCredentials(currentCanonical.inspection)
  })

  it('cleans exact Provider credential fields from the selected legacy source', async () => {
    const selectedLegacy = await inspectCurrentSettings({
      version: 1,
      agentProvider: 'reasonix',
      agents: {
        reasonix: {
          apiKey: 'synthetic-selected-reasonix-cleanup',
          baseUrl: 'https://reasonix-cleanup.example/v1',
          unknownAgentMetadata: { preserve: 1 }
        }
      },
      unknownRootMetadata: 'preserve-selected'
    }, 'analytix-settings-cleanup-selected-legacy-')
    const selectedStore = new JsonSettingsStore(selectedLegacy.userDataDir)
    await selectedStore.cleanupLegacyProviderCredentialSource(cleanupRequestFor(selectedLegacy.inspection))
    const selectedCleaned = JSON.parse(await readFile(selectedLegacy.settingsPath, 'utf8'))
    expect(selectedCleaned.agents.reasonix).not.toHaveProperty('apiKey')
    expect(selectedCleaned.agents.reasonix.unknownAgentMetadata).toEqual({ preserve: 1 })
    expect(selectedCleaned.unknownRootMetadata).toBe('preserve-selected')
    clearInspectionCredentials(selectedLegacy.inspection)
  })

  it('cleans exact Provider credential fields from the compatibility source', async () => {
    const compatibilityParent = await mkdtemp(join(tmpdir(), 'analytix-settings-cleanup-compatibility-'))
    const compatibilityCurrentDir = join(compatibilityParent, 'AnalytixCurrent')
    const compatibilitySourceDir = join(compatibilityParent, 'Kun')
    const compatibilitySourcePath = join(compatibilitySourceDir, 'analytix-settings.json')
    const compatibilityRaw = Buffer.from(JSON.stringify({
      provider: {
        apiKey: 'synthetic-compatibility-cleanup',
        baseUrl: 'https://compatibility-cleanup.example/v1',
        unknownMetadata: 'preserve-compatibility'
      }
    }), 'utf8')
    await mkdir(compatibilitySourceDir, { recursive: true })
    await writeFile(compatibilitySourcePath, compatibilityRaw)
    const compatibilityStore = new JsonSettingsStore(compatibilityCurrentDir)
    const compatibilityInspection = await compatibilityStore.inspectLegacyProviderCredentialSources()
    expect(compatibilityInspection.sourceLocator).toMatch(/^compatibility:[0-9]{2}:analytix-settings\.json$/)
    await compatibilityStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(compatibilityInspection)
    )
    const compatibilityCleaned = JSON.parse(await readFile(compatibilitySourcePath, 'utf8'))
    expect(compatibilityCleaned.provider).not.toHaveProperty('apiKey')
    expect(compatibilityCleaned.provider.unknownMetadata).toBe('preserve-compatibility')
    await expect(stat(join(compatibilityCurrentDir, 'analytix-settings.json')))
      .rejects.toMatchObject({ code: 'ENOENT' })
    clearInspectionCredentials(compatibilityInspection)
  })

  it('cleans exact Provider credential fields from the Kun compatibility source', async () => {
    const kunParent = await mkdtemp(join(tmpdir(), 'analytix-settings-cleanup-kun-'))
    const kunCurrentDir = join(kunParent, 'AnalytixCurrent')
    const kunSourceDir = join(kunParent, 'Kun')
    const kunSourcePath = join(kunSourceDir, 'kun-settings.json')
    await mkdir(kunSourceDir, { recursive: true })
    await writeFile(kunSourcePath, JSON.stringify({
      agents: {
        kun: {
          apiKey: 'synthetic-kun-active-cleanup',
          baseUrl: 'https://kun-cleanup.example/v1',
          model: 'kun-cleanup-model',
          unknownKunMetadata: true
        }
      },
      provider: {
        apiKey: 'synthetic-kun-shadow-cleanup',
        providers: [{
          id: 'custom-kun-cleanup',
          apiKey: 'synthetic-kun-custom-cleanup',
          baseUrl: 'https://custom-kun-cleanup.example/v1',
          models: ['custom-kun-cleanup-model'],
          unknownCustomMetadata: 'preserve-kun'
        }]
      },
      unknownRootMetadata: 'preserve-kun-root'
    }), 'utf8')
    const kunStore = new JsonSettingsStore(kunCurrentDir)
    const kunInspection = await kunStore.inspectLegacyProviderCredentialSources()
    expect(kunInspection.sourceLocator).toMatch(/^compatibility:[0-9]{2}:kun-settings\.json$/)
    await kunStore.cleanupLegacyProviderCredentialSource(cleanupRequestFor(kunInspection))
    const kunCleaned = JSON.parse(await readFile(kunSourcePath, 'utf8'))
    expect(kunCleaned.agents.kun).not.toHaveProperty('apiKey')
    expect(kunCleaned.provider).not.toHaveProperty('apiKey')
    expect(kunCleaned.provider.providers[0]).not.toHaveProperty('apiKey')
    expect(kunCleaned.agents.kun.unknownKunMetadata).toBe(true)
    expect(kunCleaned.provider.providers[0].unknownCustomMetadata).toBe('preserve-kun')
    expect(kunCleaned.unknownRootMetadata).toBe('preserve-kun-root')

    clearInspectionCredentials(kunInspection)
  })

  it('fails cleanup closed on malformed authority, source CAS drift, and incomplete protection without changing source bytes', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-cleanup-fail-closed',
        baseUrl: 'https://cleanup-fail-closed.example/v1',
        unknownMetadata: 'preserve-on-failure'
      }
    }, 'analytix-settings-cleanup-fail-closed-')
    const store = new JsonSettingsStore(source.userDataDir)
    const valid = cleanupRequestFor(source.inspection)
    const invalidInputs: unknown[] = [
      { ...valid, schemaVersion: 2 },
      { ...valid, purpose: 'generic-cleanup' },
      { ...valid, confirmation: 'CONFIRM' },
      { ...valid, sourceLocator: 'compatibility:00:analytix-settings.json' },
      { ...valid, sourceSHA256: valid.sourceSHA256.toUpperCase() },
      { ...valid, protectedCredentialLocators: [] },
      { ...valid, protectedCredentialLocators: [...valid.protectedCredentialLocators, ...valid.protectedCredentialLocators] },
      { ...valid, unexpected: true }
    ]
    for (const invalidInput of invalidInputs) {
      const error = await store.cleanupLegacyProviderCredentialSource(
        invalidInput as Parameters<JsonSettingsStore['cleanupLegacyProviderCredentialSource']>[0]
      ).then(() => null, (caught: unknown) => caught)
      expect(error).toBeInstanceOf(Error)
      expect((error as Error).message).toBe('Legacy Provider credential source cleanup failed.')
      expect((error as Error).message).not.toContain('synthetic-')
      expect((error as Error).message).not.toContain(source.settingsPath)
      expect(await readFile(source.settingsPath)).toEqual(source.raw)
    }

    const driftedRaw = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-cleanup-fail-closed',
        baseUrl: 'https://cleanup-fail-closed.example/v1',
        unknownMetadata: 'drifted-but-preserved'
      }
    }), 'utf8')
    await writeFile(source.settingsPath, driftedRaw)
    await expect(store.cleanupLegacyProviderCredentialSource(valid))
      .rejects.toThrow('Legacy Provider credential source cleanup failed.')
    expect(await readFile(source.settingsPath)).toEqual(driftedRaw)

    const unprotected = await inspectCurrentSettings({
      provider: { apiKey: 'synthetic-protected-provider-cleanup' },
      runtime: { apiKey: 'synthetic-unprotected-runtime-cleanup' }
    }, 'analytix-settings-cleanup-unprotected-')
    const unprotectedStore = new JsonSettingsStore(unprotected.userDataDir)
    const unprotectedError = await unprotectedStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(unprotected.inspection)
    ).then(() => null, (caught: unknown) => caught)
    expect(unprotectedError).toBeInstanceOf(Error)
    expect((unprotectedError as Error).message).toBe('Legacy Provider credential source cleanup failed.')
    expect(await readFile(unprotected.settingsPath)).toEqual(unprotected.raw)

    const malformedDir = await mkdtemp(join(tmpdir(), 'analytix-settings-cleanup-malformed-'))
    const malformedPath = join(malformedDir, 'analytix-settings.json')
    const malformedRaw = Buffer.from('{"provider":{"apiKey":"synthetic-malformed-cleanup"', 'utf8')
    await writeFile(malformedPath, malformedRaw)
    const malformedStore = new JsonSettingsStore(malformedDir)
    const malformedError = await malformedStore.cleanupLegacyProviderCredentialSource({
      schemaVersion: 1,
      purpose: 'remove-verified-legacy-provider-credentials',
      confirmation: 'REMOVE_VERIFIED_LEGACY_PROVIDER_PLAINTEXT',
      sourceLocator: 'current:analytix-settings.json',
      sourceSHA256: createHash('sha256').update(malformedRaw).digest('hex'),
      expectedCleanedSourceSHA256: '0'.repeat(64),
      protectedCredentialLocators: ['current:analytix-settings.json:provider.apiKey']
    }).then(() => null, (caught: unknown) => caught)
    expect(malformedError).toBeInstanceOf(Error)
    expect((malformedError as Error).message).toBe('Legacy Provider credential source cleanup failed.')
    expect((malformedError as Error).message).not.toContain('synthetic-')
    expect(await readFile(malformedPath)).toEqual(malformedRaw)

    const sentinel = await inspectCurrentSettings({
      provider: {
        apiKey: '[REDACTED]',
        providers: [{ id: 'deepseek', apiKey: '********' }],
        unknownMetadata: 'preserve-sentinel'
      }
    }, 'analytix-settings-cleanup-sentinel-')
    expect(sentinel.inspection.candidates).toEqual([])
    const sentinelStore = new JsonSettingsStore(sentinel.userDataDir)
    await sentinelStore.cleanupLegacyProviderCredentialSource(cleanupRequestFor(sentinel.inspection))
    const sentinelCleaned = JSON.parse(await readFile(sentinel.settingsPath, 'utf8'))
    expect(sentinelCleaned.provider).not.toHaveProperty('apiKey')
    expect(sentinelCleaned.provider.providers[0]).not.toHaveProperty('apiKey')
    expect(sentinelCleaned.provider.unknownMetadata).toBe('preserve-sentinel')

    clearInspectionCredentials(source.inspection)
    clearInspectionCredentials(unprotected.inspection)
  })

  it('rechecks cleanup source CAS inside the exclusive write window and never overwrites an interleaved writer', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-cleanup-exclusive-cas',
        baseUrl: 'https://cleanup-exclusive-cas.example/v1'
      },
      marker: 'original'
    }, 'analytix-settings-cleanup-exclusive-cas-')
    const drifted = Buffer.from(JSON.stringify({
      version: 1,
      provider: {
        apiKey: 'synthetic-cleanup-exclusive-cas',
        baseUrl: 'https://cleanup-exclusive-cas.example/v1'
      },
      marker: 'interleaved-writer'
    }), 'utf8')
    let interleavings = 0
    const store = new JsonSettingsStore(source.userDataDir, {
      legacyProviderBeforeExclusiveWrite: async () => {
        interleavings++
        await writeFile(source.settingsPath, drifted)
      }
    })
    await expect(store.cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection)))
      .rejects.toThrow('Legacy Provider credential source cleanup failed.')
    expect(interleavings).toBe(1)
    expect(await readFile(source.settingsPath)).toEqual(drifted)
    clearInspectionCredentials(source.inspection)
  })

  it('strips plaintext from an ordinary stale save after protected cleanup', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-stale-save-resurrection',
        baseUrl: 'https://stale-save-resurrection.invalid/v1'
      }
    }, 'analytix-settings-cleanup-stale-save-')
    const migrationStore = new JsonSettingsStore(source.userDataDir)
    const staleStore = new JsonSettingsStore(source.userDataDir)
    const staleDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-cleanup-stale-data-'))
    const stale = await new JsonSettingsStore(staleDataDir).load()
    ;(stale.provider as unknown as Record<string, unknown>).apiKey = 'synthetic-stale-save-resurrection'
    stale.provider.baseUrl = 'https://stale-save-resurrection.invalid/v1'
    await migrationStore.cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection))
    const cleaned = await readFile(source.settingsPath)

    await expect(staleStore.save(stale)).resolves.toBeUndefined()
    const after = await readFile(source.settingsPath)
    expect(after).not.toEqual(cleaned)
    expect(after.includes(Buffer.from('synthetic-stale-save-resurrection'))).toBe(false)
    clearInspectionCredentials(source.inspection)
  })

  it('serializes compatibility cleanup and ordinary save on one selected physical source', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-compatibility-save-lock-'))
    const currentPath = join(userDataDir, 'analytix-settings.json')
    const compatibilityPath = join(userDataDir, 'kun-settings.json')
    const compatibilityRaw = Buffer.from(JSON.stringify({
      version: 1,
      agentProvider: 'kun',
      agents: {
        kun: {
          apiKey: 'synthetic-compatibility-save-lock',
          baseUrl: 'https://compatibility-save-lock.invalid/v1'
        }
      }
    }), 'utf8')
    await writeFile(compatibilityPath, compatibilityRaw)
    const inspection = await new JsonSettingsStore(userDataDir).inspectLegacyProviderCredentialSources()
    expect(inspection.sourceLocator).toBe('compatibility:01:kun-settings.json')

    const staleDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-compatibility-stale-data-'))
    const stale = await new JsonSettingsStore(staleDataDir).load()
    ;(stale.provider as unknown as Record<string, unknown>).apiKey = 'synthetic-compatibility-save-lock'
    stale.provider.baseUrl = 'https://compatibility-save-lock.invalid/v1'

    let enteredCleanup!: () => void
    const cleanupEntered = new Promise<void>((resolve) => { enteredCleanup = resolve })
    let releaseCleanup!: () => void
    const cleanupRelease = new Promise<void>((resolve) => { releaseCleanup = resolve })
    const migrationStore = new JsonSettingsStore(userDataDir, {
      legacyProviderCleanupAtomicWriteFile: async (path, contents, options) => {
        enteredCleanup()
        await cleanupRelease
        await atomicWriteFile(path, contents, options)
      }
    })
    const cleanupPromise = migrationStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(inspection)
    )
    await cleanupEntered
    const savePromise = new JsonSettingsStore(userDataDir).save(stale)
    await new Promise((resolve) => setTimeout(resolve, 50))
    const currentCreatedBeforeCleanup = await readFile(currentPath).then(() => true, () => false)
    releaseCleanup()
    const [cleanupResult, saveResult] = await Promise.allSettled([cleanupPromise, savePromise])

    expect(currentCreatedBeforeCleanup).toBe(false)
    expect(cleanupResult.status).toBe('fulfilled')
    expect(saveResult.status).toBe('rejected')
    expect(await readFile(currentPath).then(() => true, () => false)).toBe(false)
    const compatibilityAfter = await readFile(compatibilityPath)
    expect(compatibilityAfter.includes(Buffer.from('synthetic-compatibility-save-lock'))).toBe(false)
    clearInspectionCredentials(inspection)
  })

  it('rejects ordinary load and save through symlink or hardlink settings peers', async () => {
    for (const aliasKind of ['symlink', 'hardlink'] as const) {
      const userDataDir = await mkdtemp(join(tmpdir(), `analytix-settings-ordinary-${aliasKind}-peer-`))
      const currentPath = join(userDataDir, 'analytix-settings.json')
      const peerPath = join(userDataDir, `${aliasKind}-peer.json`)
      const raw = Buffer.from(JSON.stringify({
        version: 1,
        provider: {
          apiKey: `synthetic-ordinary-${aliasKind}-peer`,
          baseUrl: `https://ordinary-${aliasKind}-peer.invalid/v1`
        }
      }), 'utf8')
      await writeFile(peerPath, raw)
      if (aliasKind === 'symlink') await symlink(peerPath, currentPath)
      else await link(peerPath, currentPath)

      const loadError = await new JsonSettingsStore(userDataDir).load().then(
        () => null,
        (error: unknown) => error
      )
      expect(loadError).toBeInstanceOf(Error)
      expect((loadError as Error).message).toBe('Failed to read settings file.')
      expect((loadError as Error).message).not.toContain(userDataDir)
      expect((loadError as Error).message).not.toContain('synthetic-')

      const defaultsDir = await mkdtemp(join(tmpdir(), 'analytix-settings-ordinary-alias-defaults-'))
      const stale = await new JsonSettingsStore(defaultsDir).load()
      ;(stale.provider as unknown as Record<string, unknown>).apiKey = `synthetic-ordinary-${aliasKind}-peer`
      stale.provider.baseUrl = `https://ordinary-${aliasKind}-peer.invalid/v1`
      await expect(new JsonSettingsStore(userDataDir).save(stale))
        .rejects.toThrow('Legacy Provider plaintext persistence is not permitted.')
      expect(await readFile(peerPath)).toEqual(raw)
      if (aliasKind === 'symlink') {
        expect((await lstat(currentPath)).isSymbolicLink()).toBe(true)
      } else {
        const currentInfo = await stat(currentPath)
        const peerInfo = await stat(peerPath)
        expect(currentInfo.ino).toBe(peerInfo.ino)
        expect(currentInfo.nlink).toBe(2)
      }
    }
  })

  it('rejects symlink and hardlink aliases before inspection or protected cleanup', async () => {
    for (const aliasKind of ['symlink', 'hardlink'] as const) {
      const source = await inspectCurrentSettings({
        version: 1,
        provider: {
          apiKey: `synthetic-${aliasKind}-cleanup-alias`,
          baseUrl: `https://${aliasKind}-cleanup-alias.invalid/v1`
        }
      }, `analytix-settings-${aliasKind}-cleanup-alias-`)
      const peerPath = join(source.userDataDir, `${aliasKind}-peer.json`)
      await writeFile(peerPath, source.raw)
      await rm(source.settingsPath)
      if (aliasKind === 'symlink') {
        await symlink(peerPath, source.settingsPath)
      } else {
        await link(peerPath, source.settingsPath)
      }

      let inspectionRejected = false
      try {
        const aliasedInspection = await new JsonSettingsStore(source.userDataDir)
          .inspectLegacyProviderCredentialSources()
        clearInspectionCredentials(aliasedInspection)
      } catch {
        inspectionRejected = true
      }
      expect(inspectionRejected).toBe(true)
      await expect(new JsonSettingsStore(source.userDataDir)
        .cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection)))
        .rejects.toThrow('Legacy Provider credential source cleanup failed.')
      expect(await readFile(peerPath)).toEqual(source.raw)
      clearInspectionCredentials(source.inspection)
    }
  })

  it('serializes lexical directory aliases through one physical protected-source lock', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-physical-alias-lock',
        baseUrl: 'https://physical-alias-lock.invalid/v1'
      }
    }, 'analytix-settings-physical-alias-lock-')
    const aliasUserDataDir = `${source.userDataDir}-alias`
    await symlink(source.userDataDir, aliasUserDataDir, 'dir')
    let arrivals = 0
    let releaseBarrier!: () => void
    const barrier = new Promise<void>((resolve) => { releaseBarrier = resolve })
    let activeWrites = 0
    let maximumActiveWrites = 0
    const options = {
      legacyProviderBeforeExclusiveWrite: async () => {
        arrivals += 1
        if (arrivals === 2) releaseBarrier()
        await barrier
      },
      legacyProviderCleanupAtomicWriteFile: async (
        path: string,
        contents: string,
        atomicOptions: Parameters<typeof atomicWriteFile>[2]
      ) => {
        activeWrites += 1
        maximumActiveWrites = Math.max(maximumActiveWrites, activeWrites)
        try {
          await new Promise((resolve) => setTimeout(resolve, 50))
          await atomicWriteFile(path, contents, atomicOptions)
        } finally {
          activeWrites -= 1
        }
      }
    }
    const request = cleanupRequestFor(source.inspection)
    const results = await Promise.allSettled([
      new JsonSettingsStore(source.userDataDir, options).cleanupLegacyProviderCredentialSource(request),
      new JsonSettingsStore(aliasUserDataDir, options).cleanupLegacyProviderCredentialSource(request)
    ])
    expect(maximumActiveWrites).toBe(1)
    expect(results.filter((result) => result.status === 'fulfilled')).toHaveLength(1)
    expect((await readFile(source.settingsPath)).includes(Buffer.from('synthetic-physical-alias-lock')))
      .toBe(false)
    clearInspectionCredentials(source.inspection)
    await rm(aliasUserDataDir)
  })

  it('never removes a replacement owner installed after a stale-lock observation', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-stale-lock-replacement',
        baseUrl: 'https://stale-lock-replacement.invalid/v1'
      }
    }, 'analytix-settings-stale-lock-replacement-')
    const lockPath = `${source.settingsPath}.analytix-provider-credential-migration.lock`
    const staleToken = '11111111-1111-4111-8111-111111111111'
    const replacementToken = '22222222-2222-4222-8222-222222222222'
    const staleOwnerName = `owner-${staleToken}.json`
    const replacementOwnerName = `owner-${replacementToken}.json`
    await mkdir(lockPath, { mode: 0o700 })
    await writeFile(join(lockPath, staleOwnerName), JSON.stringify({
      schemaVersion: 1,
      purpose: 'analytix-provider-credential-migration-source-lock',
      pid: 424_242,
      token: staleToken,
      createdAtMs: 1
    }), { mode: 0o600 })
    let staleObservations = 0
    const lockHooks = {
      now: () => 60_000,
      isProcessAlive: (pid: number) => pid === process.pid,
      afterStaleObservation: async () => {
        staleObservations += 1
        await rm(join(lockPath, staleOwnerName))
        await rmdir(lockPath)
        await mkdir(lockPath, { mode: 0o700 })
        await writeFile(join(lockPath, replacementOwnerName), JSON.stringify({
          schemaVersion: 1,
          purpose: 'analytix-provider-credential-migration-source-lock',
          pid: process.pid,
          token: replacementToken,
          createdAtMs: 60_000
        }), { mode: 0o600 })
      }
    }
    const options = { legacyProviderSourceLockTestHooks: lockHooks } as never
    await expect(new JsonSettingsStore(source.userDataDir, options)
      .cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection)))
      .rejects.toThrow('Legacy Provider credential source cleanup failed.')
    expect(staleObservations).toBe(1)
    const replacement = JSON.parse(await readFile(join(lockPath, replacementOwnerName), 'utf8')) as {
      token: string
    }
    expect(replacement.token).toBe(replacementToken)
    clearInspectionCredentials(source.inspection)
  })

  it('never removes a populated replacement after observing an ownerless stale lock', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-ownerless-stale-lock-replacement',
        baseUrl: 'https://ownerless-stale-lock-replacement.invalid/v1'
      }
    }, 'analytix-settings-ownerless-stale-lock-replacement-')
    const lockPath = `${source.settingsPath}.analytix-provider-credential-migration.lock`
    const replacementToken = '33333333-3333-4333-8333-333333333333'
    const replacementOwnerName = `owner-${replacementToken}.json`
    await mkdir(lockPath, { mode: 0o700 })
    await utimes(lockPath, new Date(0), new Date(0))
    let staleObservations = 0
    const options = {
      legacyProviderSourceLockTestHooks: {
        now: () => 60_000,
        isProcessAlive: () => true,
        afterStaleObservation: async () => {
          staleObservations += 1
          await rmdir(lockPath)
          await mkdir(lockPath, { mode: 0o700 })
          await writeFile(join(lockPath, replacementOwnerName), JSON.stringify({
            schemaVersion: 1,
            purpose: 'analytix-provider-credential-migration-source-lock',
            pid: process.pid,
            token: replacementToken,
            createdAtMs: 60_000
          }), { mode: 0o600 })
        }
      }
    } as never
    await expect(new JsonSettingsStore(source.userDataDir, options)
      .cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection)))
      .rejects.toThrow('Legacy Provider credential source cleanup failed.')
    expect(staleObservations).toBe(1)
    expect(JSON.parse(await readFile(join(lockPath, replacementOwnerName), 'utf8')))
      .toMatchObject({ token: replacementToken })
    expect(await readFile(source.settingsPath)).toEqual(source.raw)
    clearInspectionCredentials(source.inspection)
  })

  it('does not remove a replacement owner when installed-token verification loses the race', async () => {
    const source = await inspectCurrentSettings({
      version: 1,
      provider: {
        apiKey: 'synthetic-installed-lock-verification-race',
        baseUrl: 'https://installed-lock-verification-race.invalid/v1'
      }
    }, 'analytix-settings-installed-lock-verification-race-')
    const lockPath = `${source.settingsPath}.analytix-provider-credential-migration.lock`
    const replacementToken = '44444444-4444-4444-8444-444444444444'
    const replacementOwnerName = `owner-${replacementToken}.json`
    let installedHooks = 0
    const options = {
      legacyProviderSourceLockTestHooks: {
        afterOwnerInstalled: async () => {
          installedHooks += 1
          const entries = await readdir(lockPath)
          expect(entries).toHaveLength(1)
          await rm(join(lockPath, entries[0]))
          await rmdir(lockPath)
          await mkdir(lockPath, { mode: 0o700 })
          await writeFile(join(lockPath, replacementOwnerName), JSON.stringify({
            schemaVersion: 1,
            purpose: 'analytix-provider-credential-migration-source-lock',
            pid: process.pid,
            token: replacementToken,
            createdAtMs: 60_000
          }), { mode: 0o600 })
        }
      }
    } as never
    await expect(new JsonSettingsStore(source.userDataDir, options)
      .cleanupLegacyProviderCredentialSource(cleanupRequestFor(source.inspection)))
      .rejects.toThrow('Legacy Provider credential source cleanup failed.')
    expect(installedHooks).toBe(1)
    expect(JSON.parse(await readFile(join(lockPath, replacementOwnerName), 'utf8')))
      .toMatchObject({ token: replacementToken })
    expect(await readFile(source.settingsPath)).toEqual(source.raw)
    clearInspectionCredentials(source.inspection)
  })

  it('preserves the old source on pre-rename failure and never restores plaintext after post-rename failure', async () => {
    const preRename = await inspectCurrentSettings({
      provider: {
        apiKey: 'synthetic-pre-rename-cleanup',
        baseUrl: 'https://pre-rename-cleanup.example/v1'
      }
    }, 'analytix-settings-cleanup-pre-rename-')
    const preRenameEntries = await readdir(preRename.userDataDir)
    const preRenameStore = new JsonSettingsStore(preRename.userDataDir, {
      legacyProviderCleanupAtomicWriteFile: async () => {
        throw new Error('private injected pre-rename failure')
      }
    })
    const preRenameError = await preRenameStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(preRename.inspection)
    ).then(() => null, (caught: unknown) => caught)
    expect(preRenameError).toBeInstanceOf(Error)
    expect((preRenameError as Error).message).toBe('Legacy Provider credential source cleanup failed.')
    expect((preRenameError as Error).message).not.toContain('synthetic-')
    expect((preRenameError as Error).message).not.toContain(preRename.settingsPath)
    expect(await readFile(preRename.settingsPath)).toEqual(preRename.raw)
    expect(await readdir(preRename.userDataDir)).toEqual(preRenameEntries)

    const postRename = await inspectCurrentSettings({
      version: 1,
      runtime: {
        executionPolicyVersion: 2,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write'
      },
      provider: {
        apiKey: 'synthetic-post-rename-cleanup',
        baseUrl: 'https://post-rename-cleanup.example/v1',
        unknownMetadata: 'preserve-post-rename'
      }
    }, 'analytix-settings-cleanup-post-rename-')
    const postRenameStore = new JsonSettingsStore(postRename.userDataDir, {
      legacyProviderCleanupAtomicWriteFile: async (path, contents, options) => {
        await atomicWriteFile(path, contents, options)
        throw new Error('private injected post-rename failure')
      }
    })
    const cachedBeforeCleanup = await postRenameStore.load()
    expect(JSON.stringify(cachedBeforeCleanup).includes('synthetic-post-rename-cleanup')).toBe(false)
    const postRenameInspection = await postRenameStore.inspectLegacyProviderCredentialSources()
    const postRenameError = await postRenameStore.cleanupLegacyProviderCredentialSource(
      cleanupRequestFor(postRenameInspection)
    ).then(() => null, (caught: unknown) => caught)
    expect(postRenameError).toBeInstanceOf(Error)
    expect((postRenameError as Error).message).toBe('Legacy Provider credential source cleanup failed.')
    const postRenameBytes = await readFile(postRename.settingsPath)
    expect(postRenameBytes.includes(Buffer.from('synthetic-post-rename-cleanup'))).toBe(false)
    expect(JSON.parse(postRenameBytes.toString('utf8')).provider.unknownMetadata)
      .toBe('preserve-post-rename')
    const restartedInspection = await new JsonSettingsStore(postRename.userDataDir)
      .inspectLegacyProviderCredentialSources()
    expect(restartedInspection.candidates).toEqual([])
    expect(Object.hasOwn((await postRenameStore.load()).provider, 'apiKey')).toBe(false)
    if (process.platform !== 'win32') {
      expect((await stat(postRename.settingsPath)).mode & 0o777).toBe(0o600)
    }

    clearInspectionCredentials(preRename.inspection)
    clearInspectionCredentials(postRename.inspection)
    clearInspectionCredentials(postRenameInspection)
  })

  it('creates and retightens the settings file to owner-only mode on POSIX', async () => {
    if (process.platform === 'win32') return
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-mode-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    await store.save(loaded)
    await chmod(settingsPath, 0o644)
    await store.save(loaded)

    expect((await stat(settingsPath)).mode & 0o777).toBe(0o600)
  })

  it('defaults GUI updates to the stable channel for new settings', async () => {
    const tempRoot = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const userDataDir = join(tempRoot, 'Analytix')
    await mkdir(userDataDir, { recursive: true })

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.theme).toBe('light')
    expect(loaded.guiUpdate.channel).toBe(DEFAULT_GUI_UPDATE_CHANNEL)
    expect(loaded.runtime.approvalPolicy).toBe(DEFAULT_APPROVAL_POLICY)
    expect(loaded.runtime.executionPolicyVersion).toBe(2)
    const persisted = JSON.parse(await readFile(join(userDataDir, 'analytix-settings.json'), 'utf8'))
    expect(persisted.runtime).toMatchObject({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
    expect(loaded.appBehavior).toEqual({
      openAtLogin: false,
      startMinimized: false,
      closeAction: 'ask',
      closeToTray: false
    })
  })

  it('migrates and persists only the unversioned legacy execution-policy default', async () => {
    const legacyDir = await mkdtemp(join(tmpdir(), 'analytix-settings-policy-'))
    const legacyPath = join(legacyDir, 'analytix-settings.json')
    await writeFile(legacyPath, JSON.stringify({
      version: 1,
      runtime: {
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      }
    }), 'utf8')

    const migrated = await new JsonSettingsStore(legacyDir).load()
    expect(migrated.runtime).toMatchObject({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
    const persisted = JSON.parse(await readFile(legacyPath, 'utf8'))
    expect(persisted.runtime).toMatchObject({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })

    const explicitDir = await mkdtemp(join(tmpdir(), 'analytix-settings-policy-'))
    await writeFile(join(explicitDir, 'analytix-settings.json'), JSON.stringify({
      version: 1,
      runtime: {
        executionPolicyVersion: 2,
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      }
    }), 'utf8')
    const explicit = await new JsonSettingsStore(explicitDir).load()
    expect(explicit.runtime).toMatchObject({
      executionPolicyVersion: 2,
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })
  })

  it('atomically migrates legacy scheduled-task result text to an idempotent closed key', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-schedule-message-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const sentinel = 'MARKER_FREE_PROVIDER_RESULT_SENTINEL'
    await writeFile(settingsPath, JSON.stringify({
      version: 1,
      runtime: {
        executionPolicyVersion: 2,
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write'
      },
      schedule: {
        tasks: [{
          id: 'legacy-task',
          title: 'Legacy task',
          enabled: true,
          prompt: 'Run',
          workspaceRoot: userDataDir,
          clawChannelId: '',
          providerId: '',
          model: 'deepseek-v4-flash',
          reasoningEffort: 'medium',
          mode: 'agent',
          schedule: { kind: 'manual', everyMinutes: 60, timeOfDay: '09:00', atTime: '' },
          createdAt: '2026-06-02T00:00:00.000Z',
          updatedAt: '2026-06-02T00:00:00.000Z',
          lastRunAt: '2026-06-02T00:00:00.000Z',
          nextRunAt: '',
          lastStatus: 'success',
          lastMessage: sentinel,
          lastThreadId: 'thread-1'
        }]
      }
    }), 'utf8')

    const firstLoaded = await new JsonSettingsStore(userDataDir).load()
    expect(firstLoaded.schedule.tasks[0].lastMessage).toBe('schedule_task_completed')
    const firstRewrite = await readFile(settingsPath, 'utf8')
    expect(firstRewrite).not.toContain(sentinel)
    expect(JSON.parse(firstRewrite).schedule.tasks[0].lastMessage).toBe('schedule_task_completed')
    expect((await readdir(userDataDir)).filter((entry) => entry.includes('.tmp'))).toEqual([])

    const secondLoaded = await new JsonSettingsStore(userDataDir).load()
    expect(secondLoaded.schedule.tasks[0].lastMessage).toBe('schedule_task_completed')
    expect(await readFile(settingsPath, 'utf8')).toBe(firstRewrite)
  })

  it('creates a write workspace with welcome.md', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const writeWorkspaceRoot = join(userDataDir, 'write-workspace')

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        write: {
          defaultWorkspaceRoot: writeWorkspaceRoot,
          activeWorkspaceRoot: writeWorkspaceRoot,
          workspaces: [writeWorkspaceRoot]
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.write.defaultWorkspaceRoot).toBe(writeWorkspaceRoot)
    expect(loaded.write.workspaces).toContain(loaded.write.defaultWorkspaceRoot)
    expect(loaded.write.inlineCompletion.enabled).toBe(true)
    expect(loaded.write.inlineCompletion.retrievalEnabled).toBe(true)
    expect(loaded.write.inlineCompletion.longCompletionEnabled).toBe(true)
    expect(Object.hasOwn(loaded.write.inlineCompletion, 'apiKey')).toBe(false)
    expect(loaded.write.inlineCompletion.baseUrl).toBe('')
    expect(loaded.write.inlineCompletion.inheritModel).toBe(true)
    expect(loaded.write.inlineCompletion.model).toBe('deepseek-v4-flash')
    expect(loaded.write.inlineCompletion.longMaxTokens).toBe(256)
    expect(await readFile(join(loaded.write.defaultWorkspaceRoot, 'welcome.md'), 'utf8')).toContain('Welcome to Write')
  })

  it('preserves the pro write completion model', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        write: {
          inlineCompletion: {
            model: 'deepseek-v4-pro'
          }
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.write.inlineCompletion.inheritModel).toBe(false)
    expect(loaded.write.inlineCompletion.model).toBe('deepseek-v4-pro')
  })

  it('preserves disabled Skill IDs when settings are reloaded', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        disabledSkillIds: ['test-skill-08', '/skill:test-skill-09', '']
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.disabledSkillIds).toEqual(['test-skill-08', 'test-skill-09'])
  })

  it('treats legacy flash defaults as inherited until the user explicitly overrides them', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        write: {
          inlineCompletion: {
            model: 'deepseek-v4-flash'
          }
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.write.inlineCompletion.inheritModel).toBe(true)
    expect(loaded.write.inlineCompletion.model).toBe('deepseek-v4-flash')
  })

  it('migrates legacy deepseek.autoStart=false into Analytix', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const workspaceRoot = join(userDataDir, 'workspace')
    await mkdir(workspaceRoot, { recursive: true })

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        workspaceRoot,
        deepseek: {
          autoStart: false
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.runtime.autoStart).toBe(false)
  })

  it('migrates existing Analytix metadata without exposing credentials', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        runtime: {
            apiKey: 'sk-existing',
            baseUrl: 'https://runtime.example/v1'
          }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(JSON.stringify(loaded).includes('sk-existing')).toBe(false)
    expect(loaded.provider.baseUrl).toBe('https://runtime.example/v1')
    expect(Object.hasOwn(loaded.runtime, 'apiKey')).toBe(false)
    expect(loaded.runtime.baseUrl).toBe('')
  })

  it('keeps custom model providers when migrated settings are reloaded', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const provider = defaultModelProviderSettings()

    await writeFile(
      settingsPath,
      JSON.stringify({
        version: 1,
        agentProvider: 'deepseek-runtime',
        provider: {
          apiKey: 'sk-default',
          baseUrl: 'https://api.deepseek.com',
          providers: [
            ...provider.providers,
            {
              id: 'custom-provider-2',
              name: 'Custom Provider',
              apiKey: 'sk-custom',
              baseUrl: 'https://custom.example/v1',
              endpointFormat: 'messages',
              models: ['custom-model']
            }
          ]
        },
        runtime: {
            ...defaultAnalytixRuntimeSettings(),
            providerId: 'custom-provider-2',
            model: 'custom-model'
          }
      }),
      'utf8'
    )

    const firstStore = new JsonSettingsStore(userDataDir)
    const firstLoaded = await firstStore.load()

    expect(firstLoaded.provider.providers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'custom-provider-2',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'messages',
          models: ['custom-model']
        })
      ])
    )
    expect(firstLoaded.runtime.providerId).toBe('custom-provider-2')
    await firstStore.save(firstLoaded)

    const secondStore = new JsonSettingsStore(userDataDir)
    const secondLoaded = await secondStore.load()

    expect(secondLoaded.provider.providers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'custom-provider-2',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'messages',
          models: ['custom-model']
        })
      ])
    )
    expect(secondLoaded.runtime.providerId).toBe('custom-provider-2')
  })

  it('loads settings from the legacy lowercase userData directory and writes them into the current path', async () => {
    const supportRoot = await mkdtemp(join(tmpdir(), 'analytix-settings-compat-'))
    const legacyUserDataDir = join(supportRoot, 'analytix')
    const currentUserDataDir = join(supportRoot, 'analytix')
    const currentSettingsPath = join(currentUserDataDir, 'analytix-settings.json')

    await mkdir(legacyUserDataDir, { recursive: true })
    await writeFile(
      join(legacyUserDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        provider: {
          apiKey: 'sk-legacy-provider'
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(currentUserDataDir)
    const loaded = await store.load()

    expect(JSON.stringify(loaded).includes('sk-legacy-provider')).toBe(false)
    expect((await readFile(currentSettingsPath, 'utf8')).includes('sk-legacy-provider')).toBe(false)
  })

  it('creates the configured code workspace on load', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const workspaceRoot = join(userDataDir, 'missing-workspace')

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        workspaceRoot
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.workspaceRoot).toBe(workspaceRoot)
    expect((await stat(workspaceRoot)).isDirectory()).toBe(true)
  })

  it('migrates legacy deepseek-runtime agentProvider to Analytix', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        agentProvider: 'deepseek-runtime',
        deepseek: { port: 8787 }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.runtime.port).toBe(8787)
  })

  it('imports Kun settings from kun-settings.json as a one-time Analytix settings migration', async () => {
    const parentDir = await mkdtemp(join(tmpdir(), 'analytix-settings-parent-'))
    const userDataDir = join(parentDir, 'Analytix')
    const kunUserDataDir = join(parentDir, 'Kun')
    await mkdir(kunUserDataDir, { recursive: true })
    await writeFile(
      join(kunUserDataDir, 'kun-settings.json'),
      JSON.stringify({
        version: 1,
        agentProvider: 'kun',
        agents: {
          kun: {
            apiKey: 'sk-kun',
            baseUrl: 'https://kun.example/v1',
            model: 'kun-model',
            port: 7788,
            runtimeToken: 'kun-token',
            approvalPolicy: 'on-request',
            sandboxMode: 'read-only'
          }
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(JSON.stringify(loaded).includes('sk-kun')).toBe(false)
    expect(loaded.provider.baseUrl).toBe('https://kun.example/v1')
    expect(loaded.provider.providers).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'deepseek',
        baseUrl: 'https://kun.example/v1',
        models: ['kun-model']
      })
    ]))
    expect(loaded.runtime).toEqual(expect.objectContaining({
      model: 'kun-model',
      port: 7788,
      runtimeToken: 'kun-token',
      approvalPolicy: 'on-request',
      sandboxMode: 'read-only',
      baseUrl: '',
      providerId: ''
    }))
    await expect(stat(join(userDataDir, 'analytix-settings.json'))).rejects.toMatchObject({ code: 'ENOENT' })
    expect(await readFile(join(kunUserDataDir, 'kun-settings.json'), 'utf8')).toContain('sk-kun')
  })

  it('backs up invalid JSON and replaces it with defaults', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await writeFile(settingsPath, '{ invalid json', 'utf8')

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()
    const files = await readdir(userDataDir)
    const backupName = files.find((file) => file.startsWith('analytix-settings.invalid-'))

    expect(loaded.workspaceRoot.length).toBeGreaterThan(0)
    expect(backupName).toBeTruthy()
    expect(await readFile(join(userDataDir, backupName ?? ''), 'utf8')).toBe('{ invalid json')
    // 兜底默认值写进新文件名;旧文件保留原状(已经另有 invalid 备份)。
    const replaced = await readFile(join(userDataDir, 'analytix-settings.json'), 'utf8')
    expect(() => JSON.parse(replaced)).not.toThrow()
  })

  it('loads the legacy file name inside the current userData dir and re-saves it under the new name', async () => {
    // userData 整目录迁移后的常见形态:目录已经叫 Analytix,里面还是旧文件名。
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({ version: 1, provider: { apiKey: 'sk-migrated' } }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(JSON.stringify(loaded).includes('sk-migrated')).toBe(false)
    const rewritten = await readFile(join(userDataDir, 'analytix-settings.json'), 'utf8')
    expect(rewritten.includes('sk-migrated')).toBe(false)
  })

  it('throws for non-recoverable read errors', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await mkdir(settingsPath, { recursive: true })

    const store = new JsonSettingsStore(userDataDir)

    await expect(store.load()).rejects.toThrow(/Failed to read settings file/)
  })

  it('merges Analytix settings patches', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const store = new JsonSettingsStore(userDataDir)
    await store.load()

    const saved = await store.patch({
      runtime: {
          model: 'deepseek-v4-pro',
          approvalPolicy: 'on-request'
        }
    })

    expect(saved.runtime.model).toBe('deepseek-v4-pro')
    expect(saved.runtime.approvalPolicy).toBe('on-request')
  })

  it('merges desktop behavior patches without keeping invalid startup state', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const store = new JsonSettingsStore(userDataDir)
    await store.load()

    const enabled = await store.patch({
      appBehavior: {
        openAtLogin: true,
        startMinimized: true,
        closeAction: 'tray'
      }
    })
    const disabled = await store.patch({
      appBehavior: {
        openAtLogin: false,
        closeToTray: false
      }
    })

    expect(enabled.appBehavior).toEqual({
      openAtLogin: true,
      startMinimized: true,
      closeAction: 'tray',
      closeToTray: true
    })
    expect(disabled.appBehavior).toEqual({
      openAtLogin: false,
      startMinimized: false,
      closeAction: 'quit',
      closeToTray: false
    })
  })

  it('omits agentProvider when writing normalized settings to disk', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const store = new JsonSettingsStore(userDataDir)
    await store.load()
    await store.patch({
      runtime: {
          model: 'deepseek-v4-pro'
        }
    })

    const persisted = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, unknown>

    expect('agentProvider' in persisted).toBe(false)
    expect('agents' in persisted).toBe(false)
    expect(persisted.runtime).toEqual(
      expect.objectContaining({ model: 'deepseek-v4-pro' })
    )
  })

  it('rejects legacy Kun agent envelopes as an active runtime fallback', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await writeFile(
      settingsPath,
      JSON.stringify({
        version: 1,
        agentProvider: 'kun',
        agents: {
          kun: {
            apiKey: 'sk-kun',
            baseUrl: 'https://kun.example/v1',
            model: 'kun-model',
            port: 8899
          }
        }
      }),
      'utf8'
    )
    const store = new JsonSettingsStore(userDataDir)

    const loaded = await store.load()
    await store.patch({ runtime: { model: 'deepseek-v4-flash' } })
    const persisted = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, unknown>

    expect(loaded.runtime.model).not.toBe('kun-model')
    expect(Object.hasOwn(loaded.runtime, 'apiKey')).toBe(false)
    expect(loaded.runtime.baseUrl).not.toBe('https://kun.example/v1')
    expect('agentProvider' in persisted).toBe(false)
    expect('agents' in persisted).toBe(false)
  })

  it('drops Reasonix auto-plan config shapes when settings are re-saved', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    await writeFile(
      settingsPath,
      JSON.stringify({
        version: 1,
        agent: {
          auto_plan: true
        },
        autoPlan: true,
        auto_plan: true,
        runtime: {
          model: 'deepseek-v4-pro',
          autoPlan: true,
          auto_plan: true
        }
      }),
      'utf8'
    )
    const store = new JsonSettingsStore(userDataDir)

    const loaded = await store.load()
    await store.patch({ runtime: { model: 'deepseek-v4-flash' } })
    const persisted = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, unknown>
    const persistedRuntime = persisted.runtime as Record<string, unknown>

    expect('agent' in loaded).toBe(false)
    expect('autoPlan' in loaded).toBe(false)
    expect('auto_plan' in loaded).toBe(false)
    expect('autoPlan' in loaded.runtime).toBe(false)
    expect('auto_plan' in loaded.runtime).toBe(false)
    expect('agent' in persisted).toBe(false)
    expect('autoPlan' in persisted).toBe(false)
    expect('auto_plan' in persisted).toBe(false)
    expect('autoPlan' in persistedRuntime).toBe(false)
    expect('auto_plan' in persistedRuntime).toBe(false)
  })

  it('round-trips runtime and provider endpoint formats without legacy settings envelopes', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))
    const settingsPath = join(userDataDir, 'analytix-settings.json')
    const providerSettings = defaultModelProviderSettings()
    const store = new JsonSettingsStore(userDataDir)
    await store.load()

    const patched = await store.patch({
      provider: {
        ...providerSettings,
        providers: [
          ...providerSettings.providers,
          {
            id: 'custom-messages',
            name: 'Custom Messages',
            baseUrl: 'https://custom.example/v1/messages',
            endpointFormat: 'custom_endpoint',
            models: ['custom-messages-model']
          }
        ]
      },
      runtime: {
        providerId: 'custom-messages',
        model: 'custom-messages-model',
        endpointFormat: 'messages'
      }
    })

    expect(patched.runtime).toEqual(
      expect.objectContaining({
        providerId: 'custom-messages',
        model: 'custom-messages-model',
        endpointFormat: 'messages'
      })
    )

    const persisted = JSON.parse(await readFile(settingsPath, 'utf8')) as Record<string, unknown>
    const persistedProvider = persisted.provider as { providers?: Array<Record<string, unknown>> }
    const persistedRuntime = persisted.runtime as Record<string, unknown>

    expect('agentProvider' in persisted).toBe(false)
    expect('agents' in persisted).toBe(false)
    expect(persistedRuntime).toEqual(
      expect.objectContaining({
        providerId: 'custom-messages',
        model: 'custom-messages-model',
        endpointFormat: 'messages'
      })
    )
    expect(persistedProvider.providers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'custom-messages',
          baseUrl: 'https://custom.example/v1/messages',
          endpointFormat: 'custom_endpoint',
          models: ['custom-messages-model']
        })
      ])
    )

    const reloaded = await new JsonSettingsStore(userDataDir).load()
    expect(reloaded.runtime).toEqual(
      expect.objectContaining({
        providerId: 'custom-messages',
        model: 'custom-messages-model',
        endpointFormat: 'messages'
      })
    )
    expect(reloaded.provider.providers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'custom-messages',
          baseUrl: 'https://custom.example/v1/messages',
          endpointFormat: 'custom_endpoint',
          models: ['custom-messages-model']
        })
      ])
    )
  })

  it('folds legacy Claw thread ids into the single Analytix mapping', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        claw: {
          channels: [
            {
              id: 'channel-1',
              provider: 'feishu',
              label: 'Feishu Agent',
              threadId: 'thr_codewhale',
              agentThreadIds: { reasonix: '2026-06-01T01:00:00.000Z' },
              conversations: [
                {
                  id: 'conversation-1',
                  chatId: 'chat-1',
                  latestMessageId: 'message-1',
                  localThreadId: 'thr_conversation_codewhale',
                  agentThreadIds: { reasonix: '2026-06-01T02:00:00.000Z' }
                }
              ]
            }
          ]
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()
    const channel = loaded.claw.channels[0]
    const conversation = channel?.conversations[0]

    expect(channel?.threadId).toBe('thr_codewhale')
    expect(conversation?.localThreadId).toBe('thr_conversation_codewhale')
  })

  it('seeds Reasonix-only Claw conversations into the canonical thread id', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-'))

    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        claw: {
          channels: [
            {
              id: 'channel-1',
              provider: 'feishu',
              label: 'Feishu Agent',
              agentThreadIds: { reasonix: 'reasonix-channel' },
              conversations: [
                {
                  id: 'conversation-1',
                  chatId: 'chat-1',
                  latestMessageId: 'message-1',
                  localThreadId: '',
                  agentThreadIds: { reasonix: 'reasonix-conversation' }
                }
              ]
            }
          ]
        }
      }),
      'utf8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()
    const channel = loaded.claw.channels[0]
    const conversation = channel?.conversations[0]

    expect(channel?.threadId).toBe('reasonix-channel')
    expect(conversation?.localThreadId).toBe('reasonix-conversation')
  })

  it('saves settings atomically (no .tmp file left on success)', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'analytix-settings-atomic-'))

    try {
      const store = new JsonSettingsStore(userDataDir)
      const loaded = await store.load()
      await store.save(loaded)

      // Final file is present and non-empty.
      const finalContents = await readFile(
        join(userDataDir, 'analytix-settings.json'),
        'utf8'
      )
      expect(finalContents.length).toBeGreaterThan(0)

      // No .tmp leftover from the atomic write.
      const entries = await readdir(userDataDir)
      expect(entries.filter((entry) => entry.includes('.tmp'))).toEqual([])
    } finally {
      await rm(userDataDir, { recursive: true, force: true })
    }
  })
})
