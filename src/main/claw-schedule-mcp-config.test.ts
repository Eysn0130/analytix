import { chmod, mkdir, mkdtemp, readFile, stat, writeFile } from 'node:fs/promises'
import { homedir, tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  buildClawScheduleMcpArgs,
  buildRuntimeHostScheduleMcpBindingV1,
  buildSyncedClawScheduleMcpJson,
  clawScheduleMcpSettingsChanged,
  removeLegacyClawScheduleTomlConfig,
  resolveClawScheduleMcpCommand,
  resolveClawScheduleMcpNodeEntryPath,
  resolveAnalytixConfigPath,
  resolveAnalytixMcpJsonPath,
  syncClawScheduleMcpConfig,
  type ClawScheduleMcpLaunchConfig
} from './claw-schedule-mcp-config'
import {
  defaultClawSettings,
  defaultKeyboardShortcuts,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  defaultScheduleSettings,
  defaultWriteSettings,
  type AppSettingsV1
} from '../shared/app-settings'

function createSettings(patch: Partial<AppSettingsV1['schedule']['internal']> = {}): AppSettingsV1 {
  const claw = defaultClawSettings()
  const schedule = defaultScheduleSettings()
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: {
      enabled: true,
      retentionDays: 2
    },
    notifications: {
      turnComplete: true
    },
    appBehavior: { openAtLogin: false, startMinimized: false, closeToTray: false },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    write: defaultWriteSettings(),
    schedule: {
      ...schedule,
      internal: {
        ...schedule.internal,
        ...patch
      }
    },
    guiUpdate: {
      channel: 'stable'
    },
    codePromptPrefix: '',
    disabledSkillIds: [],
    claw: {
      ...claw,
      enabled: true,
      im: {
        ...claw.im,
        enabled: true,
        port: 8787,
        secret: ''
      }
    }
  }
}

const launch: ClawScheduleMcpLaunchConfig = {
  appPath: '/Applications/Analytix.app',
  execPath: '/Applications/Analytix.app/Contents/MacOS/Analytix',
  isPackaged: false
}

describe('claw schedule MCP config', () => {
  it('uses Analytix config files by default', () => {
    expect(resolveAnalytixConfigPath()).toBe(join(homedir(), '.analytix', 'config.toml'))
    expect(resolveAnalytixMcpJsonPath()).toBe(join(homedir(), '.analytix', 'mcp.json'))
  })

  it('writes the gui_schedule server to the Analytix MCP JSON config shape', () => {
    const settings = createSettings({ port: 9787, secret: 'top-secret' })
    const synced = buildSyncedClawScheduleMcpJson(
      {
        timeouts: { connect_timeout: 1 },
        servers: {
          context7: {
            command: 'npx',
            args: ['-y', '@upstash/context7-mcp'],
            env: {},
            url: null
          }
        }
      },
      settings,
      launch
    )

    expect(synced.servers).toMatchObject({
      context7: {
        command: 'npx'
      },
      gui_schedule: {
        command: resolveClawScheduleMcpCommand(launch),
        args: [
          resolveClawScheduleMcpNodeEntryPath(launch),
          '--gui-schedule-mcp-server',
          '--base-url',
          'http://127.0.0.1:9787',
          '--secret',
          'top-secret'
        ],
        env: {
          ELECTRON_RUN_AS_NODE: '1'
        },
        url: null,
        enabled: true
      }
    })
    expect(synced.timeouts).toEqual({ connect_timeout: 1 })
  })

  it('uses the macOS Electron helper for real app bundle paths', () => {
    expect(resolveClawScheduleMcpCommand(launch, 'darwin')).toBe(
      '/Applications/Analytix.app/Contents/Frameworks/Analytix Helper.app/Contents/MacOS/Analytix Helper'
    )
    expect(resolveClawScheduleMcpCommand({
      appPath: '/tmp/analytix-test-app',
      execPath: '/tmp/electron',
      isPackaged: false
    }, 'darwin')).toBe('/tmp/electron')
  })

  it('builds the exact private runtime binding from the same host launch inputs', () => {
    const binding = buildRuntimeHostScheduleMcpBindingV1(
      createSettings({ port: 9787, secret: 'top-secret' }),
      launch
    )
    expect(binding).toEqual({
      schemaVersion: 1,
      purpose: 'analytix.runtime-host-schedule-mcp-binding/v1',
      serverId: 'gui_schedule',
      command: resolveClawScheduleMcpCommand(launch),
      args: [
        resolveClawScheduleMcpNodeEntryPath(launch),
        '--gui-schedule-mcp-server',
        '--base-url',
        'http://127.0.0.1:9787',
        '--secret',
        'top-secret'
      ],
      env: { ELECTRON_RUN_AS_NODE: '1' },
      trustScope: 'user',
      timeoutMs: 5_000
    })
    expect(Object.isFrozen(binding)).toBe(true)
    expect(Object.isFrozen(binding.args)).toBe(true)
    expect(Object.isFrozen(binding.env)).toBe(true)
  })

  it('rejects a private runtime binding that Go cannot decode canonically', () => {
    expect(() => buildRuntimeHostScheduleMcpBindingV1(
      createSettings({ port: 9787, secret: 'invalid-\ud800' }),
      launch
    )).toThrow('Host schedule MCP binding is invalid.')

    for (const port of [0, 65_536, Number.NaN]) {
      expect(() => buildRuntimeHostScheduleMcpBindingV1(
        createSettings({ port, secret: '' }),
        launch
      )).toThrow('Host schedule MCP binding is invalid.')
    }

    expect(() => buildRuntimeHostScheduleMcpBindingV1(
      createSettings({ port: 9787, secret: '&'.repeat(90_000) }),
      launch
    )).toThrow('Host schedule MCP binding is invalid.')
  })

  it('uses the same ECMAScript whitespace boundary as the schedule server and Go', () => {
    const secretWithNEL = '\u0085private\u0085'
    expect(buildClawScheduleMcpArgs(
      createSettings({ port: 9787, secret: secretWithNEL }),
      launch
    ).slice(-2)).toEqual(['--secret', secretWithNEL])
    expect(buildRuntimeHostScheduleMcpBindingV1(
      createSettings({ port: 9787, secret: secretWithNEL }),
      launch
    ).args.slice(-2)).toEqual(['--secret', secretWithNEL])
  })

  it('removes legacy config.toml claw_schedule blocks without touching other MCP servers', () => {
    const cleaned = removeLegacyClawScheduleTomlConfig(
      [
        'provider = "deepseek"',
        '',
        '[mcp_servers.context7]',
        'command = "npx"',
        '',
        '[mcp_servers.claw_schedule]',
        'command = "old"',
        'args = []',
        '',
        '# analytix plugin:mcp:claw-schedule START',
        '[mcp_servers.claw_schedule]',
        'command = "electron"',
        'args = []',
        '# analytix plugin:mcp:claw-schedule END',
        '',
        '[providers.deepseek]',
        'api_key = ""'
      ].join('\n')
    )

    expect(cleaned).toContain('[mcp_servers.context7]')
    expect(cleaned).toContain('[providers.deepseek]')
    expect(cleaned).not.toContain('[mcp_servers.claw_schedule]')
    expect(cleaned).not.toContain('analytix plugin:mcp:claw-schedule')
  })

  it('does not rewrite config.toml text when there is no legacy claw_schedule block', () => {
    const current = [
      'provider = "deepseek"',
      '',
      '[mcp_servers.context7]',
      'command = "npx"',
      '',
      ''
    ].join('\n')

    expect(removeLegacyClawScheduleTomlConfig(current)).toBe(current)
  })

  it('syncs mcp.json and cleans the old config.toml entry on disk', async () => {
    const root = await mkdtemp(join(tmpdir(), 'analytix-mcp-'))
    const analytixDir = join(root, '.analytix')
    const configTomlPath = join(analytixDir, 'config.toml')
    const mcpJsonPath = join(analytixDir, 'mcp.json')
    await mkdir(analytixDir, { recursive: true })
    await writeFile(
      configTomlPath,
      [
        'provider = "deepseek"',
        '',
        '# analytix plugin:mcp:claw-schedule START',
        '[mcp_servers.claw_schedule]',
        'command = "electron"',
        'args = []',
        '# analytix plugin:mcp:claw-schedule END',
        ''
      ].join('\n'),
      'utf8'
    )
    await writeFile(
      mcpJsonPath,
      JSON.stringify({
        servers: {
          existing: {
            command: '/bin/echo',
            args: ['ok'],
            env: {},
            url: null
          }
        }
      }),
      'utf8'
    )
    if (process.platform !== 'win32') {
      await chmod(configTomlPath, 0o644)
      await chmod(mcpJsonPath, 0o644)
    }

    await syncClawScheduleMcpConfig(createSettings(), launch, { configTomlPath, mcpJsonPath })

    const toml = await readFile(configTomlPath, 'utf8')
    const json = JSON.parse(await readFile(mcpJsonPath, 'utf8')) as Record<string, unknown>

    expect(toml).toBe('provider = "deepseek"\n')
    expect(json).toMatchObject({
      servers: {
        existing: {
          command: '/bin/echo'
        },
        gui_schedule: {
          command: resolveClawScheduleMcpCommand(launch),
          args: [
            resolveClawScheduleMcpNodeEntryPath(launch),
            '--gui-schedule-mcp-server',
            '--base-url',
            'http://127.0.0.1:8788'
          ],
          env: {
            ELECTRON_RUN_AS_NODE: '1'
          }
        }
      }
    })
    if (process.platform !== 'win32') {
      expect((await stat(configTomlPath)).mode & 0o777).toBe(0o600)
      expect((await stat(mcpJsonPath)).mode & 0o777).toBe(0o600)
    }
  })

  it('migrates old claw_schedule JSON entries to gui_schedule', () => {
    const synced = buildSyncedClawScheduleMcpJson(
      {
        servers: {
          claw_schedule: {
            command: 'old',
            args: ['--claw-schedule-mcp-server']
          }
        }
      },
      createSettings(),
      launch
    )

    expect((synced.servers as Record<string, unknown>).claw_schedule).toBeUndefined()
    expect(synced.servers).toMatchObject({
      gui_schedule: {
        command: resolveClawScheduleMcpCommand(launch),
        env: {
          ELECTRON_RUN_AS_NODE: '1'
        }
      }
    })
  })

  it('requests a runtime restart when the MCP launch arguments change', () => {
    expect(clawScheduleMcpSettingsChanged(createSettings(), createSettings())).toBe(false)
    expect(clawScheduleMcpSettingsChanged(createSettings(), createSettings({ port: 9876 }))).toBe(true)
    expect(clawScheduleMcpSettingsChanged(createSettings(), createSettings({ secret: 'abc' }))).toBe(true)
  })
})
