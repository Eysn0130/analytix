import { chmod, mkdir, mkdtemp, readFile, stat, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve, sep } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { runInNewContext } from 'node:vm'
import { createRequire } from 'node:module'
import {
  ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV,
  ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1,
  applyDesktopLoginItemSettings,
  applyDesktopProtocolRegistrations,
  consumeDesktopExternalStateBoundary,
  resolveDesktopExternalStateBoundary
} from './desktop-external-state-isolation'
import { JsonSettingsStore } from './settings-store'
import {
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

async function isolatedFixture(): Promise<{
  cacheRoot: string
  taskRoot: string
  userDataRoot: string
}> {
  const parent = await mkdtemp(join(tmpdir(), 'analytix-desktop-isolation-'))
  const cacheRoot = join(parent, 'cache')
  const taskRoot = join(cacheRoot, 'tmp', 'task-one')
  const userDataRoot = join(taskRoot, 'user-data')
  for (const path of [cacheRoot, join(cacheRoot, 'tmp'), taskRoot, userDataRoot]) {
    await mkdir(path, { recursive: true, mode: 0o700 })
    await chmod(path, 0o700)
  }
  return { cacheRoot, taskRoot, userDataRoot }
}

function isolatedEnvironment(
  fixture: Awaited<ReturnType<typeof isolatedFixture>>
): NodeJS.ProcessEnv {
  return {
    ANALYTIX_DEV_CACHE_ROOT: fixture.cacheRoot,
    ANALYTIX_USER_DATA_DIR: fixture.userDataRoot,
    [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]:
      ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
  }
}

function settingsFixture(): AppSettingsV1 {
  const claw = defaultClawSettings()
  const schedule = defaultScheduleSettings()
  return {
    version: 1,
    locale: 'en',
    theme: 'light',
    uiFontScale: 'small',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/synthetic-workspace',
    log: { enabled: false, retentionDays: 2 },
    notifications: { turnComplete: false },
    appBehavior: {
      openAtLogin: false,
      startMinimized: false,
      closeAction: 'ask',
      closeToTray: false
    },
    keyboardShortcuts: defaultKeyboardShortcuts(),
    guiUpdate: { channel: 'stable' },
    codePromptPrefix: '',
    disabledSkillIds: [],
    write: defaultWriteSettings(),
    claw,
    schedule
  }
}

const launch: ClawScheduleMcpLaunchConfig = {
  appPath: '/Applications/Analytix.app',
  execPath: '/Applications/Analytix.app/Contents/MacOS/analytix',
  isPackaged: true
}

describe('main-private desktop external-state isolation', () => {
  it('separates retained state from build cache without accepting a different state owner', async () => {
    const fixture = await isolatedFixture()
    const stateRoot = join(fixture.cacheRoot, '..', 'protected-state')
    const canonicalState = resolve(stateRoot)
    const taskRoot = join(canonicalState, 'task-one')
    const userDataRoot = join(taskRoot, 'user-data')
    const homeRoot = join(taskRoot, 'home')
    for (const directory of [canonicalState, taskRoot, userDataRoot, homeRoot]) {
      await mkdir(directory, { mode: 0o700 })
    }
    const env = { ...isolatedEnvironment(fixture), ANALYTIX_DEV_STATE_ROOT: canonicalState,
      ANALYTIX_USER_DATA_DIR: userDataRoot, HOME: homeRoot }
    expect(resolveDesktopExternalStateBoundary(env).isolationRoot).toBe(taskRoot)
    expect(resolveDesktopExternalStateBoundary(env).stateHomeRoot).toBe(homeRoot)
    expect(() => resolveDesktopExternalStateBoundary({ ...env, HOME: userDataRoot })).toThrow()
    expect(() => resolveDesktopExternalStateBoundary({ ...env, ANALYTIX_USER_DATA_DIR: fixture.userDataRoot })).toThrow()
    await chmod(canonicalState, 0o755)
    expect(() => resolveDesktopExternalStateBoundary(env)).toThrow()
  })

  it('preserves the default protocol and login-item integrations when no mode is present', () => {
    const boundary = resolveDesktopExternalStateBoundary({})
    const register = vi.fn(() => true)
    const setLoginItemSettings = vi.fn()

    expect(applyDesktopProtocolRegistrations(
      boundary,
      ['analytix', 'com.analytix.desktop'],
      register
    )).toBe(2)
    expect(applyDesktopLoginItemSettings(boundary, {
      platform: 'darwin',
      openAtLogin: true,
      startMinimized: false,
      hiddenStartArg: '--hidden',
      setLoginItemSettings
    })).toBe(true)

    expect(register.mock.calls).toEqual([
      ['analytix'],
      ['com.analytix.desktop']
    ])
    expect(setLoginItemSettings).toHaveBeenCalledOnce()
    expect(setLoginItemSettings).toHaveBeenCalledWith({ openAtLogin: true, args: [] })
  })

  it('consumes the exact isolated mode and suppresses protocol and login-item writes', async () => {
    const fixture = await isolatedFixture()
    const env = isolatedEnvironment(fixture)
    const boundary = consumeDesktopExternalStateBoundary(env)
    const register = vi.fn(() => true)
    const setLoginItemSettings = vi.fn()

    expect(boundary).toMatchObject({
      isolated: true,
      userDataRoot: fixture.userDataRoot,
      stateHomeRoot: fixture.userDataRoot
    })
    expect(env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]).toBeUndefined()
    expect(applyDesktopProtocolRegistrations(
      boundary,
      ['analytix', 'com.analytix.desktop'],
      register
    )).toBe(0)
    expect(applyDesktopLoginItemSettings(boundary, {
      platform: 'darwin',
      openAtLogin: false,
      startMinimized: false,
      hiddenStartArg: '--hidden',
      setLoginItemSettings
    })).toBe(false)
    expect(applyDesktopLoginItemSettings(boundary, {
      platform: 'darwin',
      openAtLogin: true,
      startMinimized: true,
      hiddenStartArg: '--hidden',
      setLoginItemSettings
    })).toBe(false)
    expect(register).not.toHaveBeenCalled()
    expect(setLoginItemSettings).not.toHaveBeenCalled()
  })

  it('fails closed before an integration callback for unknown, missing, or non-task-owned roots', async () => {
    const fixture = await isolatedFixture()
    const effect = vi.fn()
    const invalidEnvironments: NodeJS.ProcessEnv[] = [
      {
        ...isolatedEnvironment(fixture),
        [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]: 'unknown-mode'
      },
      {
        ANALYTIX_DEV_CACHE_ROOT: fixture.cacheRoot,
        [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]:
          ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
      },
      {
        ANALYTIX_DEV_CACHE_ROOT: fixture.cacheRoot,
        ANALYTIX_USER_DATA_DIR: 'relative/user-data',
        [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]:
          ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
      },
      {
        ANALYTIX_DEV_CACHE_ROOT: fixture.cacheRoot,
        ANALYTIX_USER_DATA_DIR: fixture.cacheRoot,
        [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]:
          ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
      }
    ]

    for (const env of invalidEnvironments) {
      expect(() => {
        resolveDesktopExternalStateBoundary(env)
        effect()
      }).toThrow('Analytix desktop external-state isolation configuration is invalid.')
    }
    expect(effect).not.toHaveBeenCalled()
  })

  it('derives every fresh workspace family under the isolated userData root', async () => {
    const fixture = await isolatedFixture()
    const settingsPath = join(fixture.userDataRoot, 'analytix-settings.json')
    await writeFile(settingsPath, JSON.stringify({
      version: 1,
      claw: {
        channels: [{
          id: 'synthetic-channel',
          provider: 'telegram',
          label: 'Synthetic channel',
          workspaceRoot: '',
          conversations: [{
            id: 'synthetic-conversation',
            chatId: 'synthetic-chat',
            latestMessageId: '',
            workspaceRoot: ''
          }]
        }]
      }
    }), 'utf8')

    const store = new JsonSettingsStore(fixture.userDataRoot, {
      homeRoot: fixture.userDataRoot
    })
    const settings = await store.load()
    const roots = [
      settings.runtime.dataDir,
      settings.workspaceRoot,
      settings.write.defaultWorkspaceRoot,
      settings.write.activeWorkspaceRoot,
      ...settings.write.workspaces,
      ...settings.claw.channels.flatMap((channel) => [
        channel.workspaceRoot,
        ...channel.conversations.map((conversation) => conversation.workspaceRoot)
      ])
    ]

    expect(settings.claw.channels).toHaveLength(1)
    expect(roots.length).toBeGreaterThanOrEqual(5)
    expect(
      roots.every((path) => path.startsWith(`${fixture.userDataRoot}${sep}`)),
      JSON.stringify(roots)
    ).toBe(true)
    for (const path of roots) expect((await stat(path)).isDirectory()).toBe(true)
  })

  it('redirects Claw startup config without reading or mutating a synthetic real-home canary', async () => {
    const fixture = await isolatedFixture()
    const canaryHome = join(fixture.taskRoot, 'real-home-canary')
    const canaryDirectory = join(canaryHome, '.analytix')
    const canaryConfig = join(canaryDirectory, 'config.toml')
    const canaryMcp = join(canaryDirectory, 'mcp.json')
    await mkdir(canaryDirectory, { recursive: true, mode: 0o700 })
    await writeFile(canaryConfig, 'synthetic-canary-config\n', { mode: 0o600 })
    await writeFile(canaryMcp, '{"synthetic":"canary"}\n', { mode: 0o600 })
    await chmod(canaryConfig, 0o000)
    await chmod(canaryMcp, 0o000)

    const isolatedConfig = resolveAnalytixConfigPath(fixture.userDataRoot)
    const isolatedMcp = resolveAnalytixMcpJsonPath(fixture.userDataRoot)
    try {
      await syncClawScheduleMcpConfig(settingsFixture(), launch, {
        configTomlPath: isolatedConfig,
        mcpJsonPath: isolatedMcp
      })
    } finally {
      await chmod(canaryConfig, 0o600)
      await chmod(canaryMcp, 0o600)
    }

    expect(await readFile(canaryConfig, 'utf8')).toBe('synthetic-canary-config\n')
    expect(await readFile(canaryMcp, 'utf8')).toBe('{"synthetic":"canary"}\n')
    expect(JSON.parse(await readFile(isolatedMcp, 'utf8'))).toMatchObject({
      servers: { gui_schedule: { enabled: true } }
    })
  })

  it('wires the boundary before startup effects and keeps it out of default package launch config', async () => {
    const [mainSource, packageJson, builderConfig, releaseScript] = await Promise.all([
      readFile(join(process.cwd(), 'src/main/index.ts'), 'utf8'),
      readFile(join(process.cwd(), 'package.json'), 'utf8'),
      readFile(join(process.cwd(), 'electron-builder.config.cjs'), 'utf8'),
      readFile(join(process.cwd(), 'scripts/release-mac.sh'), 'utf8')
    ])
    const boundaryIndex = mainSource.indexOf('consumeDesktopExternalStateBoundary(process.env)')
    const firstStartupEffectIndex = mainSource.indexOf('captureOpenComputerUseAgentBaselineV1()')

    expect(boundaryIndex).toBeGreaterThanOrEqual(0)
    expect(boundaryIndex).toBeLessThan(firstStartupEffectIndex)
    expect(mainSource).toContain('applyDesktopProtocolRegistrations(')
    expect(mainSource).toContain('applyDesktopLoginItemSettings(')
    expect(mainSource).toContain('homeRoot: desktopStateHomeRoot')
    expect(mainSource).toContain('homeDir: desktopStateHomeRoot')
    expect(mainSource).toContain('clawScheduleMcpConfigPaths')
    expect(mainSource.match(/clawScheduleMcpConfigPaths/gu)).toHaveLength(4)
    expect(mainSource.match(/syncLoginItemSettings\(saved\)/gu)).toHaveLength(2)
    expect(mainSource).toContain('defaultDarwinTerminalProtectedRoots(desktopStateHomeRoot)')
    expect(mainSource).toContain('runLegacyAnalytixDataMigration({')
    expect(mainSource).toContain('createProviderRegistryIpcHandler(')
    expect(mainSource).toContain('migrateLegacyImAccountCredentials({')
    expect(mainSource).toContain('configureAnalytixProcessMainPrivatePaths({')
    // The builder may consume the explicit isolation mode to refuse local
    // release.env reads, but must never inject that mode into a normal package.
    const evaluateBuilder = (env: Record<string, string>) => {
      const reads: string[] = []
      const context = { process: { env }, __dirname: '/synthetic/repo', module: { exports: {} },
        require: (name: string) => {
          if (name === 'node:fs') return { existsSync: (path: string) => path.endsWith('release.local.env'),
            readFileSync: (path: string) => { reads.push(path); return 'ANALYTIX_APP_VERSION=1.2.3' } }
          if (name === 'node:path') return { join }
          if (name.includes('macos-signing-policy')) return { requireOfficialTeamIdentifier: () => { throw new Error('unapproved signing') } }
          if (name.includes('production-mcp-entry-closure-manifest')) return { loadProductionMcpEntryClosureContract: () => ({ files: [] }) }
          if (name.includes('packaged-lifecycle-guard')) return { _internals: { assertNoPrepackagedCommandLine: () => {} } }
          if (name === './scripts/core-package-profile.cjs') return createRequire(import.meta.url)(join(process.cwd(), name))
          throw new Error('unexpected builder dependency')
        } }
      runInNewContext(builderConfig, context)
      return { reads, env, config: context.module.exports }
    }
    const isolated = evaluateBuilder({ [ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]: ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1 })
    expect(isolated.reads).toEqual([])
    expect(isolated.env.ANALYTIX_APP_VERSION).toBeUndefined()
    const ordinary = evaluateBuilder({})
    expect(ordinary.reads).toEqual(['/synthetic/repo/scripts/release.local.env'])
    expect(ordinary.env.ANALYTIX_APP_VERSION).toBe('1.2.3')
    expect(ordinary.env[ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV]).toBeUndefined()
    expect(JSON.stringify(ordinary.config)).not.toContain(ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV)
    const scripts = JSON.parse(packageJson).scripts as Record<string, string>
    const defaultCommands = ['dist', 'dist:mac', 'dist:mac:signed', 'dist:mac:arm64:core:controlled', 'release:all', 'release:mac']
    for (const name of defaultCommands) expect(scripts[name]).toBeTypeOf('string')
    for (const defaultLaunchSource of [...defaultCommands.map(name => scripts[name]), releaseScript]) {
      expect(defaultLaunchSource).not.toContain(ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV)
      expect(defaultLaunchSource).not.toContain(
        ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1
      )
    }
    expect(scripts['dist:mac:arm64:core']).toContain(`${ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ENV}=${ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE_ISOLATED_LOCAL_V1}`)
    expect(scripts['dist:mac:arm64:core']).not.toContain('ANALYTIX_RELEASE_BUILD=1')
  })
})
