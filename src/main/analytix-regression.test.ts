import { describe, expect, it } from 'vitest'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  DEFAULT_DEEPSEEK_BASE_URL,
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  migrateLegacyAppSettings,
  type AppSettingsV1
} from '../shared/app-settings'
import { analytixRuntimeAdapter } from './runtime/analytix-adapter'
import { JsonSettingsStore } from './settings-store'

describe('Analytix single-agent regression', () => {
  it('seeds key-free provider metadata and Analytix port from legacy local HTTP settings', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'codewhale',
      agents: {
        codewhale: {
          binaryPath: '/usr/local/bin/codewhale',
          port: 8787,
          apiKey: 'legacy-key',
          baseUrl: DEFAULT_DEEPSEEK_BASE_URL,
          autoStart: false
        }
      },
      deepseek: { port: 8788 }
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime).toEqual(expect.objectContaining({
      baseUrl: '',
      binaryPath: '',
      port: 8788,
      autoStart: false
    }))
    expect(migrated.provider).toEqual(expect.objectContaining({
      baseUrl: DEFAULT_DEEPSEEK_BASE_URL
    }))
    expect(Object.hasOwn(migrated.runtime ?? {}, 'apiKey')).toBe(false)
    expect(Object.hasOwn(migrated.provider ?? {}, 'apiKey')).toBe(false)
    expect(JSON.stringify(migrated)).not.toContain('legacy-key')
  })

  it('does not carry legacy local-runtime binary paths into Analytix', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'deepseek-runtime',
      deepseek: {
        binaryPath: '/Applications/DeepSeek Runtime.app/Contents/MacOS/deepseek-runtime',
        port: 8787
      }
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime).toEqual(expect.objectContaining({
      binaryPath: '',
      port: 8787
    }))
  })

  it('does not keep the legacy default local HTTP port for Analytix', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'codewhale',
      agents: {
        codewhale: {
          port: 7878
        }
      }
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime?.port).toBe(8899)
  })

  it('seeds key-free provider metadata and Analytix model from legacy reasoning settings', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'reasonix',
      agents: {
        reasonix: {
          apiKey: 'reasoning-key',
          baseUrl: 'https://api.deepseek.com',
          model: 'deepseek-reasoner',
          autoStart: false
        }
      }
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime).toEqual(expect.objectContaining({
      baseUrl: '',
      model: 'deepseek-reasoner',
      autoStart: false
    }))
    expect(migrated.provider).toEqual(expect.objectContaining({
      baseUrl: 'https://api.deepseek.com'
    }))
    expect(Object.hasOwn(migrated.runtime ?? {}, 'apiKey')).toBe(false)
    expect(Object.hasOwn(migrated.provider ?? {}, 'apiKey')).toBe(false)
    expect(JSON.stringify(migrated)).not.toContain('reasoning-key')
  })

  it('Analytix adapter reports base url and id', () => {
    const settings: AppSettingsV1 = {
      version: 1,
      locale: 'en',
      theme: 'system',
      uiFontScale: 'small',
      provider: defaultModelProviderSettings(),
      runtime: defaultAnalytixRuntimeSettings(9000),
      workspaceRoot: '/tmp',
      log: { enabled: true, retentionDays: 7 },
      notifications: { turnComplete: true },
      appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
      keyboardShortcuts: defaultKeyboardShortcuts(),
      write: defaultWriteSettings(),
      claw: defaultClawSettings(),
      schedule: defaultScheduleSettings(),
      guiUpdate: { channel: 'stable' },
      codePromptPrefix: '',
      disabledSkillIds: []
    }

    expect(analytixRuntimeAdapter.id).toBe('analytix')
    expect(analytixRuntimeAdapter.getBaseUrl(settings)).toBe('http://127.0.0.1:9000')
  })

  it('JsonSettingsStore saves only Analytix after legacy settings migration', async () => {
    const userDataDir = await mkdtemp(join(tmpdir(), 'ca-settings-'))
    await writeFile(
      join(userDataDir, 'analytix-settings.json'),
      JSON.stringify({
        version: 1,
        agentProvider: 'codewhale',
        deepseek: { port: 8787 }
      }),
      'utf-8'
    )

    const store = new JsonSettingsStore(userDataDir)
    const loaded = await store.load()

    expect(loaded.runtime).toEqual(expect.objectContaining({ port: 8787 }))
    await rm(userDataDir, { recursive: true, force: true })
  })
})
