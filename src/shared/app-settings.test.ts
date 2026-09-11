import { describe, expect, it } from 'vitest'
import {
  applyAnalytixRuntimePatch,
  analytixToolPermissionModeFromSettings,
  analytixToolPermissionModeSettings,
  DEFAULT_ANALYTIX_DATA_DIR,
  DEFAULT_ANALYTIX_MODEL,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  CURRENT_EXECUTION_POLICY_VERSION,
  DEFAULT_WEIXIN_BRIDGE_RPC_URL,
  DEFAULT_SCHEDULE_INTERNAL_PORT,
  buildAnalytixRuntimeSettingsKey,
  buildClawRuntimePrompt,
  defaultClawSettings,
  defaultModelProviderSettings,
  mergeAnalytixRuntimeSettings,
  mergeScheduleSettings,
  defaultAnalytixRuntimeSettings,
  defaultScheduleSettings,
  defaultWriteSelectionAssistSettings,
  defaultWriteSettings,
  getModelProviderPreset,
  defaultKeyboardShortcuts,
  modelProviderPresetProfile,
  mergeAppBehaviorSettings,
  mergeWriteSettings,
  normalizeWriteSettings,
  normalizeWriteAgentPresets,
  isAnalytixRuntimeInsecure,
  migrateLegacyAppSettings,
  normalizeAppSettings,
  normalizeAnalytixExecutionPolicy,
  parseClawUserPromptForDisplay,
  inferModelEndpointFormatFromUrl,
  normalizeScheduleSettings,
  scheduleSettingsNeedsMessageKeyMigration,
  resolveAnalytixRuntimeSettings,
  resolveWriteInlineCompletionBaseUrl,
  resolveWriteInlineCompletionModel,
  type AppSettingsV1,
  type ClawImChannelV1,
  type ClawImProvider
} from './app-settings'

function settings(): AppSettingsV1 {
  return {
    version: 1,
    locale: 'en',
    theme: 'system',
    uiFontScale: 'small',
    motionPreference: 'system',
    provider: defaultModelProviderSettings(),
    runtime: defaultAnalytixRuntimeSettings(),
    workspaceRoot: '/tmp/workspace',
    log: { enabled: false, retentionDays: 7 },
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
}

describe('model endpoint format inference', () => {
  it('treats /completions custom endpoints as Chat Completions-shaped', () => {
    expect(inferModelEndpointFormatFromUrl('https://api.example.com/custom/completions')).toBe('chat_completions')
    expect(inferModelEndpointFormatFromUrl('https://api.example.com/custom/completions?api-version=2026-01-01')).toBe(
      'chat_completions'
    )
  })
})

function clawChannel(provider: ClawImProvider, label: string, name = label): ClawImChannelV1 {
  const now = '2026-06-01T00:00:00.000Z'
  return {
    id: `${provider}-${label}`,
    provider,
    label,
    enabled: true,
    model: 'auto',
    threadId: '',
    workspaceRoot: '',
    agentProfile: {
      name,
      description: '',
      identity: '',
      personality: '',
      userContext: '',
      replyRules: ''
    },
    conversations: [],
    createdAt: now,
    updatedAt: now
  }
}

describe('analytix defaults', () => {
  it('keeps a single shared default data directory source', () => {
    expect(defaultAnalytixRuntimeSettings().dataDir).toBe(DEFAULT_ANALYTIX_DATA_DIR)
  })

  it('defaults the assistant model to v4 flash', () => {
    expect(defaultAnalytixRuntimeSettings().model).toBe(DEFAULT_ANALYTIX_MODEL)
    expect(defaultAnalytixRuntimeSettings().model).toBe('deepseek-v4-flash')
  })

  it('normalizes shell motion preference values', () => {
    expect(normalizeAppSettings({ ...settings(), motionPreference: 'on' }).motionPreference).toBe('on')
    expect(normalizeAppSettings({ ...settings(), motionPreference: 'system' }).motionPreference).toBe('system')
    expect(normalizeAppSettings({ ...settings(), motionPreference: 'off' }).motionPreference).toBe('off')
    expect(normalizeAppSettings({
      ...settings(),
      motionPreference: 'fast' as AppSettingsV1['motionPreference']
    }).motionPreference).toBe('on')
  })

  it('normalizes stale DeepSeek runtime settings away from unsupported MiMo models', () => {
    const normalized = normalizeAppSettings({
      ...settings(),
      provider: defaultModelProviderSettings(),
      runtime: {
        ...defaultAnalytixRuntimeSettings(),
        providerId: 'deepseek',
        model: 'mimo-v2-pro'
      }
    })

    expect(normalized.runtime.providerId).toBe('deepseek')
    expect(normalized.runtime.model).toBe('deepseek-v4-flash')
    expect(resolveAnalytixRuntimeSettings(normalized)).toMatchObject({
      providerId: 'deepseek',
      model: 'deepseek-v4-flash'
    })
  })

  it('defaults interactive execution policy to version-2 safe settings', () => {
    expect(defaultAnalytixRuntimeSettings().executionPolicyVersion).toBe(CURRENT_EXECUTION_POLICY_VERSION)
    expect(defaultAnalytixRuntimeSettings().approvalPolicy).toBe(DEFAULT_APPROVAL_POLICY)
    expect(defaultAnalytixRuntimeSettings().approvalPolicy).toBe('on-request')
    expect(defaultAnalytixRuntimeSettings().sandboxMode).toBe(DEFAULT_SANDBOX_MODE)
    expect(defaultAnalytixRuntimeSettings().sandboxMode).toBe('workspace-write')
  })

  it('maps composer permission modes to approval and sandbox settings', () => {
    expect(analytixToolPermissionModeSettings('request-approval')).toEqual({
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
    expect(analytixToolPermissionModeSettings('auto-approval')).toEqual({
      approvalPolicy: 'untrusted',
      sandboxMode: 'danger-full-access'
    })
    expect(analytixToolPermissionModeSettings('full-access')).toEqual({
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })
    expect(analytixToolPermissionModeFromSettings(defaultAnalytixRuntimeSettings())).toBe('request-approval')
    expect(analytixToolPermissionModeFromSettings({
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })).toBe('request-approval')
    expect(analytixToolPermissionModeFromSettings({
      approvalPolicy: 'on-request',
      sandboxMode: 'read-only'
    })).toBe('custom')
  })

  it('migrates only the exact unversioned legacy execution-policy default', () => {
    expect(normalizeAnalytixExecutionPolicy({
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })).toEqual({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
    expect(normalizeAnalytixExecutionPolicy({
      approvalPolicy: 'always',
      sandboxMode: 'danger-full-access'
    })).toEqual({
      executionPolicyVersion: 2,
      approvalPolicy: 'always',
      sandboxMode: 'danger-full-access'
    })
    expect(normalizeAnalytixExecutionPolicy({
      executionPolicyVersion: 2,
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })).toEqual({
      executionPolicyVersion: 2,
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })
  })

  it('replaces invalid execution-policy enum values with the safe pair', () => {
    expect(normalizeAnalytixExecutionPolicy({
      executionPolicyVersion: 2,
      approvalPolicy: 'invalid',
      sandboxMode: 'danger-full-access'
    })).toEqual({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
  })

  it('does not treat numeric version markers as absent or downgrade them', () => {
    for (const executionPolicyVersion of [1, 3]) {
      expect(normalizeAnalytixExecutionPolicy({
        executionPolicyVersion,
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      })).toEqual({
        executionPolicyVersion,
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      })
    }
    expect(normalizeAnalytixExecutionPolicy({
      executionPolicyVersion: 'future',
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access'
    })).toEqual({
      executionPolicyVersion: 2,
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    })
  })

  it('defaults browser and computer-use controls to the highest local permission policy', () => {
    expect(defaultAnalytixRuntimeSettings().browserUse).toMatchObject({
      enabled: true,
      localUrlOpenTarget: 'analytix',
      annotationScreenshotsMode: 'always',
      approvalMode: 'neverAsk',
      fullCdpAccess: true,
      chromeControlEnabled: true,
      sitePermissions: []
    })
    expect(defaultAnalytixRuntimeSettings().computerUse).toMatchObject({
      enabled: true,
      mode: 'always',
      allowWhenLocked: true
    })
  })

  it('defaults token economy mode to off', () => {
    expect(defaultAnalytixRuntimeSettings().tokenEconomyMode).toBe(false)
    expect(defaultAnalytixRuntimeSettings().tokenEconomy).toMatchObject({
      enabled: false,
      compressToolDescriptions: true,
      compressToolResults: true,
      conciseResponses: true,
      historyHygiene: {
        maxToolResultLines: 320,
        maxToolResultBytes: 32768,
        maxToolResultTokens: 8000,
        maxToolArgumentStringBytes: 8192,
        maxToolArgumentStringTokens: 2000,
        maxArrayItems: 80,
        maxCumulativeToolResultTokens: 120000,
        keepRecentToolResults: 4
      }
    })
  })

  it('defaults MCP search discovery to off', () => {
    expect(defaultAnalytixRuntimeSettings().mcpSearch).toMatchObject({
      enabled: false,
      mode: 'auto',
      autoThresholdToolCount: 24,
      topKDefault: 5,
      topKMax: 10
    })
  })

  it('defaults design quality checks to standard', () => {
    expect(defaultAnalytixRuntimeSettings().quality).toEqual({
      enabled: true,
      strictness: 'standard',
      ignoreRules: [],
      ignoreFiles: [],
      maxFindings: 12
    })
  })

  it('defaults image generation to off with empty provider fields', () => {
    expect(defaultAnalytixRuntimeSettings().imageGeneration).toEqual({
      enabled: false,
      providerId: '',
      protocol: 'openai-images',
      baseUrl: '',
      model: '',
      defaultSize: '',
      timeoutMs: 180000
    })
  })

  it('defaults media generation to off with empty provider fields', () => {
    expect(defaultAnalytixRuntimeSettings().textToSpeech).toEqual({
      enabled: false,
      providerId: '',
      protocol: 'openai-speech',
      baseUrl: '',
      model: '',
      voice: '',
      format: 'mp3',
      timeoutMs: 120000
    })
    expect(defaultAnalytixRuntimeSettings().musicGeneration).toEqual({
      enabled: false,
      providerId: '',
      protocol: 'minimax-music',
      baseUrl: '',
      model: '',
      format: 'mp3',
      timeoutMs: 300000
    })
    expect(defaultAnalytixRuntimeSettings().videoGeneration).toEqual({
      enabled: false,
      providerId: '',
      protocol: 'minimax-video',
      baseUrl: '',
      model: '',
      defaultDuration: 6,
      defaultResolution: '1080P',
      timeoutMs: 900000,
      pollIntervalMs: 10000
    })
  })

  it('defaults advanced Analytix runtime tuning to conservative values', () => {
    expect(defaultAnalytixRuntimeSettings()).toMatchObject({
      storage: {
        backend: 'hybrid',
        sqlitePath: ''
      },
      contextCompaction: {
        defaultSoftThreshold: 96000,
        defaultHardThreshold: 108800,
        summaryMode: 'model',
        summaryTimeoutMs: 15000,
        summaryMaxTokens: 1200,
        summaryInputMaxBytes: 98304
      },
      runtimeTuning: {
        streamIdleTimeoutMs: 45000,
        stepLimits: {
          defaultMaxModelSteps: 64,
          userGlobalMaxModelSteps: 0,
          plannerMaxModelSteps: 0,
          headlessMaxModelSteps: 0
        },
        toolStorm: {
          enabled: true,
          windowSize: 8,
          threshold: 3
        },
        toolArgumentRepair: {
          maxStringBytes: 524288
        }
      }
    })
  })
})

describe('app behavior settings', () => {
  it('defaults desktop behavior to off', () => {
    const raw = {
      ...settings(),
      appBehavior: undefined
    } as unknown as AppSettingsV1

    expect(normalizeAppSettings(raw).appBehavior).toEqual({
      openAtLogin: false,
      startMinimized: false,
      closeAction: 'ask',
      closeToTray: false
    })
  })

  it('only keeps start minimized when open at login is enabled', () => {
    const normalized = normalizeAppSettings({
      ...settings(),
      appBehavior: {
        openAtLogin: false,
        startMinimized: true,
        closeToTray: true
      }
    })

    expect(normalized.appBehavior).toEqual({
      openAtLogin: false,
      startMinimized: false,
      closeAction: 'tray',
      closeToTray: true
    })
  })

  it('maps legacy closeToTray patches to explicit close actions', () => {
    const current = normalizeAppSettings({
      ...settings(),
      appBehavior: undefined
    } as unknown as AppSettingsV1)

    expect(current.appBehavior.closeAction).toBe('ask')
    expect(mergeAppBehaviorSettings(current.appBehavior, { closeToTray: true }).closeAction).toBe('tray')
    expect(mergeAppBehaviorSettings(current.appBehavior, { closeToTray: false }).closeAction).toBe('quit')
  })
})

describe('keyboard shortcut settings', () => {
  it('defaults shortcut overrides to empty', () => {
    const raw = {
      ...settings(),
      keyboardShortcuts: undefined
    } as unknown as AppSettingsV1

    expect(normalizeAppSettings(raw).keyboardShortcuts).toEqual({
      bindings: {}
    })
  })
})

describe('claw settings', () => {
  it('stores the WeChat bridge URL in Connect Phone settings', () => {
    const defaults = defaultClawSettings()
    expect(defaults.im.weixinBridgeUrl).toBe(DEFAULT_WEIXIN_BRIDGE_RPC_URL)

    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaults,
        im: {
          ...defaults.im,
          weixinBridgeUrl: '  http://127.0.0.1:8787/rpc  '
        }
      }
    })

    expect(normalized.claw.im.weixinBridgeUrl).toBe('http://127.0.0.1:8787/rpc')
  })

  it('migrates the legacy OpenClaw Gateway URL into the WeChat bridge URL', () => {
    const defaults = defaultClawSettings()
    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaults,
        im: {
          ...defaults.im,
          weixinBridgeUrl: '',
          openClawGatewayUrl: '  http://127.0.0.1:8787/rpc  '
        } as typeof defaults.im & { openClawGatewayUrl: string }
      }
    })

    expect(normalized.claw.im.weixinBridgeUrl).toBe('http://127.0.0.1:8787/rpc')
  })

  it('normalizes phone agent default names without touching custom names', () => {
    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaultClawSettings(),
        channels: [
          clawChannel('weixin', 'WeChat Agent', 'WeChat Agent'),
          clawChannel('feishu', 'Feishu / Lark', 'Feishu Agent'),
          clawChannel('telegram', 'Telegram Bot', 'Telegram Bot'),
          clawChannel('weixin', 'Support Bot', '')
        ]
      }
    })

    expect(normalized.claw.channels.map((channel) => ({
      label: channel.label,
      name: channel.agentProfile.name
    }))).toEqual([
      { label: 'weixin agent', name: 'weixin agent' },
      { label: 'feishu agent', name: 'feishu agent' },
      { label: 'telegram agent', name: 'telegram agent' },
      { label: 'Support Bot', name: 'Support Bot' }
    ])
  })

  it('projects legacy Telegram credentials to key-free channel metadata', () => {
    const telegram = {
      ...clawChannel('telegram', 'Telegram Bot', 'Telegram Bot'),
      platformCredential: {
        kind: 'telegram' as const,
        botToken: ' 123456789:AA_mock_token_with_enough_length ',
        allowedChatIds: ' 1001, 1002 ',
        botUsername: ' @analytix_bot ',
        createdAt: '2026-06-10T00:00:00.000Z'
      }
    }
    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaultClawSettings(),
        channels: [telegram]
      }
    })

    expect(normalized.runtime).toMatchObject({
      providerId: '',
      model: DEFAULT_ANALYTIX_MODEL
    })
    expect(normalized.claw.channels[0]).toMatchObject({
      provider: 'telegram',
      label: 'telegram agent',
      platformAccount: {
        kind: 'telegram',
        accountId: telegram.id,
        allowedChatIds: '1001, 1002',
        botUsername: 'analytix_bot',
        createdAt: '2026-06-10T00:00:00.000Z'
      }
    })
    expect(normalized.claw.channels[0]).not.toHaveProperty('platformCredential')
    expect(JSON.stringify(normalized)).not.toContain('123456789:AA_mock_token_with_enough_length')
  })

  it('keeps the channel welcomeSentAt marker and drops empty values', () => {
    const welcomed = { ...clawChannel('weixin', 'WeChat Agent'), welcomeSentAt: '2026-06-10T00:00:00.000Z' }
    const fresh = { ...clawChannel('feishu', 'Feishu / Lark'), welcomeSentAt: '' }
    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaultClawSettings(),
        channels: [welcomed, fresh]
      }
    })

    expect(normalized.claw.channels[0].welcomeSentAt).toBe('2026-06-10T00:00:00.000Z')
    expect(normalized.claw.channels[1]).not.toHaveProperty('welcomeSentAt')
  })
})

describe('isAnalytixRuntimeInsecure', () => {
  it('keeps an empty runtime token secure for managed ephemeral-token startup', () => {
    expect(
      isAnalytixRuntimeInsecure({
        ...defaultAnalytixRuntimeSettings(),
        insecure: false,
        runtimeToken: ''
      })
    ).toBe(false)
  })

  it('keeps auth enabled when a token exists and insecure is false', () => {
    expect(
      isAnalytixRuntimeInsecure({
        ...defaultAnalytixRuntimeSettings(),
        insecure: false,
        runtimeToken: 'tok-1'
      })
    ).toBe(false)
  })

  it('allows unauthenticated mode only when explicitly enabled', () => {
    expect(
      isAnalytixRuntimeInsecure({
        ...defaultAnalytixRuntimeSettings(),
        insecure: true,
        runtimeToken: ''
      })
    ).toBe(true)
  })
})

describe('mergeAnalytixRuntimeSettings', () => {
  it('merges a direct analytix patch without the envelope wrapper', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      model: 'deepseek-reasoner',
      port: 9000,
      tokenEconomyMode: true
    })
    expect(next.model).toBe('deepseek-reasoner')
    expect(next.port).toBe(9000)
    expect(next.tokenEconomyMode).toBe(true)
    expect(next.tokenEconomy.enabled).toBe(true)
    expect(next.baseUrl).toBe(current.baseUrl)
  })

  it('deep-merges token economy settings and keeps the legacy switch synced', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      tokenEconomy: {
        enabled: true,
        compressToolResults: false,
        historyHygiene: {
          maxToolResultLines: 120,
          maxCumulativeToolResultTokens: 0,
          keepRecentToolResults: 0
        }
      }
    })

    expect(next.tokenEconomyMode).toBe(true)
    expect(next.tokenEconomy.enabled).toBe(true)
    expect(next.tokenEconomy.compressToolDescriptions).toBe(true)
    expect(next.tokenEconomy.compressToolResults).toBe(false)
    expect(next.tokenEconomy.historyHygiene.maxToolResultLines).toBe(120)
    expect(next.tokenEconomy.historyHygiene.maxToolResultBytes).toBe(
      current.tokenEconomy.historyHygiene.maxToolResultBytes
    )
    expect(next.tokenEconomy.historyHygiene.maxCumulativeToolResultTokens).toBe(0)
    expect(next.tokenEconomy.historyHygiene.keepRecentToolResults).toBe(0)

    const legacySwitch = mergeAnalytixRuntimeSettings(next, { tokenEconomyMode: false })
    expect(legacySwitch.tokenEconomyMode).toBe(false)
    expect(legacySwitch.tokenEconomy.enabled).toBe(false)
  })

  it('drops Reasonix auto-plan fields from runtime settings', () => {
    const current = {
      ...defaultAnalytixRuntimeSettings(),
      autoPlan: true
    } as AppSettingsV1['runtime'] & { autoPlan: boolean }
    const next = mergeAnalytixRuntimeSettings(current, {
      model: 'deepseek-v4-flash',
      auto_plan: true
    } as Parameters<typeof mergeAnalytixRuntimeSettings>[1] & { auto_plan: boolean })

    expect(next.model).toBe('deepseek-v4-flash')
    expect('autoPlan' in next).toBe(false)
    expect('auto_plan' in next).toBe(false)
  })

  it('deep-merges MCP search settings', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      mcpSearch: {
        enabled: true,
        mode: 'search',
        topKDefault: 3
      }
    })

    expect(next.mcpSearch.enabled).toBe(true)
    expect(next.mcpSearch.mode).toBe('search')
    expect(next.mcpSearch.topKDefault).toBe(3)
    expect(next.mcpSearch.topKMax).toBe(current.mcpSearch.topKMax)
  })

  it('deep-merges advanced Analytix settings', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      storage: {
        sqlitePath: ' /tmp/analytix.sqlite3 '
      },
      contextCompaction: {
        defaultSoftThreshold: 64000
      },
      runtimeTuning: {
        stepLimits: {
          userGlobalMaxModelSteps: 12
        },
        toolStorm: {
          threshold: 5
        }
      }
    })

    expect(next.storage.backend).toBe('hybrid')
    expect(next.storage.sqlitePath).toBe('/tmp/analytix.sqlite3')
    expect(next.contextCompaction.defaultSoftThreshold).toBe(64000)
    expect(next.contextCompaction.defaultHardThreshold).toBe(64000)
    expect(next.contextCompaction.summaryMode).toBe('model')
    expect(next.runtimeTuning.toolStorm.enabled).toBe(true)
    expect(next.runtimeTuning.toolStorm.windowSize).toBe(current.runtimeTuning.toolStorm.windowSize)
    expect(next.runtimeTuning.toolStorm.threshold).toBe(5)
    expect(next.runtimeTuning.stepLimits).toEqual({
      ...current.runtimeTuning.stepLimits,
      userGlobalMaxModelSteps: 12
    })
    expect(next.runtimeTuning.toolArgumentRepair).toEqual(current.runtimeTuning.toolArgumentRepair)
    expect(next.runtimeTuning.streamIdleTimeoutMs).toBe(current.runtimeTuning.streamIdleTimeoutMs)
  })

  it('normalizes design quality patches structurally', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      quality: {
        enabled: false,
        strictness: 'strict',
        ignoreRules: ['quality-overused-font', ''],
        ignoreFiles: ['vendor/**'],
        maxFindings: 200
      }
    })

    expect(next.quality).toEqual({
      enabled: false,
      strictness: 'strict',
      ignoreRules: ['quality-overused-font'],
      ignoreFiles: ['vendor/**'],
      maxFindings: 100
    })
  })

  it('normalizes subagent profile patches structurally', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      subagents: {
        maxParallel: 4,
        maxChildRuns: 20,
        defaultToolPolicy: 'inherit',
        defaultProfile: 'reviewer',
        profiles: {
          reviewer: {
            prompt: ' Review with care. ',
            model: ' deepseek-v4-pro ',
            effort: 'high',
            toolPolicy: 'readOnly',
            tools: ['grep', 'read', 'grep', '']
          }
        }
      }
    })

    expect(next.subagents).toEqual({
      enabled: true,
      maxParallel: 4,
      maxChildRuns: 20,
      defaultToolPolicy: 'inherit',
      defaultProfile: 'reviewer',
      profiles: {
        reviewer: {
          prompt: 'Review with care.',
          model: 'deepseek-v4-pro',
          effort: 'high',
          toolPolicy: 'readOnly',
          tools: ['grep', 'read']
        }
      }
    })

    const removed = mergeAnalytixRuntimeSettings(next, {
      subagents: {
        defaultProfile: 'reviewer',
        profiles: { reviewer: null }
      }
    })
    expect(removed.subagents.defaultProfile).toBe('')
    expect(removed.subagents.profiles).toEqual({})
  })

  it('normalizes the stream idle timeout (0 disables, out-of-range clamps)', () => {
    const current = defaultAnalytixRuntimeSettings()
    expect(current.runtimeTuning.streamIdleTimeoutMs).toBe(45000)

    const set = mergeAnalytixRuntimeSettings(current, {
      runtimeTuning: { streamIdleTimeoutMs: 300000 }
    })
    expect(set.runtimeTuning.streamIdleTimeoutMs).toBe(300000)
    // Other knobs are untouched by a timeout-only patch.
    expect(set.runtimeTuning.toolStorm).toEqual(current.runtimeTuning.toolStorm)

    // 0 means "disabled" and is preserved rather than coerced to the default.
    expect(
      mergeAnalytixRuntimeSettings(current, { runtimeTuning: { streamIdleTimeoutMs: 0 } })
        .runtimeTuning.streamIdleTimeoutMs
    ).toBe(0)

    // Negative falls back to the default; absurdly large clamps to the cap.
    expect(
      mergeAnalytixRuntimeSettings(current, { runtimeTuning: { streamIdleTimeoutMs: -5 } })
        .runtimeTuning.streamIdleTimeoutMs
    ).toBe(45000)
    expect(
      mergeAnalytixRuntimeSettings(current, { runtimeTuning: { streamIdleTimeoutMs: 999_999_999 } })
        .runtimeTuning.streamIdleTimeoutMs
    ).toBe(3_600_000)
  })

  it('normalizes runtime step limits (0 is valid, invalid values fall back or clamp)', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      runtimeTuning: {
        stepLimits: {
          defaultMaxModelSteps: 0,
          userGlobalMaxModelSteps: 128,
          plannerMaxModelSteps: -1,
          headlessMaxModelSteps: 99_999
        }
      }
    })

    expect(next.runtimeTuning.stepLimits).toEqual({
      defaultMaxModelSteps: 0,
      userGlobalMaxModelSteps: 128,
      plannerMaxModelSteps: current.runtimeTuning.stepLimits.plannerMaxModelSteps,
      headlessMaxModelSteps: 10_000
    })
  })

  it('deep-merges image generation settings and normalizes invalid values', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      imageGeneration: {
        enabled: true,
        baseUrl: ' https://api.siliconflow.cn/v1 ',
        model: 'Kwai-Kolors/Kolors'
      }
    })

    expect(next.imageGeneration).toEqual({
      enabled: true,
      providerId: '',
      protocol: 'openai-images',
      baseUrl: 'https://api.siliconflow.cn/v1',
      model: 'Kwai-Kolors/Kolors',
      defaultSize: '',
      timeoutMs: 180000
    })

    const sized = mergeAnalytixRuntimeSettings(next, {
      imageGeneration: { defaultSize: '1536x1024', timeoutMs: 240000 }
    })
    expect(sized.imageGeneration.defaultSize).toBe('1536x1024')
    expect(sized.imageGeneration.timeoutMs).toBe(240000)
    expect(Object.hasOwn(sized.imageGeneration, 'apiKey')).toBe(false)

    const invalidSize = mergeAnalytixRuntimeSettings(sized, {
      imageGeneration: { defaultSize: 'huge', timeoutMs: -5 }
    })
    expect(invalidSize.imageGeneration.defaultSize).toBe('')
    expect(invalidSize.imageGeneration.timeoutMs).toBe(180000)
  })

  it('deep-merges media generation settings and normalizes invalid values', () => {
    const current = defaultAnalytixRuntimeSettings()
    const next = mergeAnalytixRuntimeSettings(current, {
      textToSpeech: {
        enabled: true,
        protocol: 'minimax-t2a',
        baseUrl: ' https://api.minimax.io ',
        model: 'speech-2.8-hd',
        voice: ' male-qn-qingse ',
        format: 'wav'
      },
      musicGeneration: {
        enabled: true,
        baseUrl: ' https://api.minimax.io ',
        model: 'music-2.6'
      },
      videoGeneration: {
        enabled: true,
        baseUrl: ' https://api.minimax.io ',
        model: 'MiniMax-Hailuo-2.3',
        defaultDuration: 10,
        pollIntervalMs: 20000
      }
    })

    expect(next.textToSpeech).toMatchObject({
      enabled: true,
      protocol: 'minimax-t2a',
      baseUrl: 'https://api.minimax.io',
      model: 'speech-2.8-hd',
      voice: 'male-qn-qingse',
      format: 'wav'
    })
    expect(next.musicGeneration).toMatchObject({
      enabled: true,
      protocol: 'minimax-music',
      baseUrl: 'https://api.minimax.io',
      model: 'music-2.6',
      format: 'mp3'
    })
    expect(next.videoGeneration).toMatchObject({
      enabled: true,
      protocol: 'minimax-video',
      baseUrl: 'https://api.minimax.io',
      model: 'MiniMax-Hailuo-2.3',
      defaultDuration: 10,
      defaultResolution: '1080P',
      pollIntervalMs: 20000
    })

    const invalid = mergeAnalytixRuntimeSettings(next, {
      textToSpeech: { format: 'aac', timeoutMs: -1 },
      videoGeneration: { defaultDuration: -1, pollIntervalMs: -1 }
    })
    expect(invalid.textToSpeech.format).toBe('mp3')
    expect(invalid.textToSpeech.timeoutMs).toBe(120000)
    expect(invalid.videoGeneration.defaultDuration).toBe(6)
    expect(invalid.videoGeneration.pollIntervalMs).toBe(10000)
  })

  it('defaults missing MiniMax media generation settings to the configured MiniMax provider', () => {
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProfile = modelProviderPresetProfile(minimax!)
    const {
      textToSpeech: _textToSpeech,
      musicGeneration: _musicGeneration,
      videoGeneration: _videoGeneration,
      ...legacyAnalytix
    } = defaultAnalytixRuntimeSettings()
    void _textToSpeech
    void _musicGeneration
    void _videoGeneration
    const normalized = normalizeAppSettings({
      ...settings(),
      provider: {
        ...defaultModelProviderSettings(),
        providers: [
          ...defaultModelProviderSettings().providers,
          minimaxProfile
        ]
      },
      runtime: legacyAnalytix as AppSettingsV1['runtime']
    })
    const resolved = resolveAnalytixRuntimeSettings(normalized)

    expect(normalized.runtime.textToSpeech).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-t2a',
      model: 'speech-2.8-hd'
    }))
    expect(normalized.runtime.musicGeneration).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-music',
      model: 'music-2.6'
    }))
    expect(normalized.runtime.videoGeneration).toEqual(expect.objectContaining({
      enabled: true,
      providerId: 'minimax',
      protocol: 'minimax-video',
      model: 'MiniMax-Hailuo-2.3'
    }))
    expect(Object.hasOwn(resolved.textToSpeech, 'apiKey')).toBe(false)
    expect(resolved.musicGeneration.baseUrl).toBe('https://api.minimax.io')
    expect(resolved.videoGeneration.baseUrl).toBe('https://api.minimax.io')
  })
})

describe('analytix runtime helpers', () => {
  it('applies a analytix patch onto full app settings', () => {
    const current = settings()
    const next = applyAnalytixRuntimePatch(current, { model: 'deepseek-reasoner' })
    expect(next.runtime.model).toBe('deepseek-reasoner')
    expect(next.write).toEqual(current.write)
  })

  it('does not restart Go for compatibility-only or Electron-only settings', () => {
    const base = settings()
    const inactive = structuredClone(base)
    inactive.runtime.storage = { backend: 'file', sqlitePath: '/tmp/ignored.sqlite3' }
    inactive.runtime.tokenEconomy.enabled = !inactive.runtime.tokenEconomy.enabled
    inactive.runtime.tokenEconomyMode = inactive.runtime.tokenEconomy.enabled
    inactive.runtime.contextCompaction.defaultSoftThreshold += 1024
    inactive.runtime.runtimeTuning.toolStorm.enabled = !inactive.runtime.runtimeTuning.toolStorm.enabled
    inactive.runtime.runtimeTuning.toolArgumentRepair.maxStringBytes += 1024
    inactive.runtime.quality.enabled = !inactive.runtime.quality.enabled
    inactive.runtime.memoryEnabled = !inactive.runtime.memoryEnabled
    inactive.runtime.imageGeneration.enabled = !inactive.runtime.imageGeneration.enabled
    inactive.runtime.speechToText.enabled = !inactive.runtime.speechToText.enabled
    inactive.runtime.textToSpeech.enabled = !inactive.runtime.textToSpeech.enabled
    inactive.runtime.musicGeneration.enabled = !inactive.runtime.musicGeneration.enabled
    inactive.runtime.videoGeneration.enabled = !inactive.runtime.videoGeneration.enabled
    inactive.runtime.browserUse.enabled = !inactive.runtime.browserUse.enabled
    inactive.runtime.computerUse.allowWhenLocked = !inactive.runtime.computerUse.allowWhenLocked
    inactive.runtime.computerUse.maxImageDimension += 1
    inactive.runtime.computerUse.maxActionsPerTurn += 1
    inactive.runtime.computerUse.mode = inactive.runtime.computerUse.mode === 'auto' ? 'always' : 'auto'

    expect(buildAnalytixRuntimeSettingsKey(inactive)).toBe(buildAnalytixRuntimeSettingsKey(base))
  })

  it('restarts Go when the production Computer Use enablement changes', () => {
    const base = settings()
    const changed = structuredClone(base)
    changed.runtime.computerUse.enabled = !changed.runtime.computerUse.enabled

    expect(buildAnalytixRuntimeSettingsKey(changed)).not.toBe(buildAnalytixRuntimeSettingsKey(base))
  })

  it('restarts Go for active tuning and provider context metadata', () => {
    const base = settings()
    const timeoutChanged = structuredClone(base)
    timeoutChanged.runtime.runtimeTuning.streamIdleTimeoutMs += 1000
    expect(buildAnalytixRuntimeSettingsKey(timeoutChanged)).not.toBe(
      buildAnalytixRuntimeSettingsKey(base)
    )

    const profileChanged = structuredClone(base)
    const provider = profileChanged.provider.providers[0]
    const modelId = Object.keys(provider.modelProfiles)[0]
    provider.modelProfiles[modelId] = {
      ...provider.modelProfiles[modelId],
      contextWindowTokens: (provider.modelProfiles[modelId]?.contextWindowTokens ?? 128000) + 1
    }
    expect(buildAnalytixRuntimeSettingsKey(profileChanged)).not.toBe(
      buildAnalytixRuntimeSettingsKey(base)
    )
  })
})

describe('legacy Analytix defaults migration', () => {
  it('projects untrusted input onto canonical key-free AppSettings fields', () => {
    const canary = ['closure', 'b', 'untrusted-settings'].join('-')
    const polluted = {
      ...settings(),
      apiKey: canary,
      provider: {
        ...defaultModelProviderSettings(),
        apiKey: canary,
        providers: defaultModelProviderSettings().providers.map((provider) => ({
          ...provider,
          apiKey: canary
        }))
      },
      runtime: {
        ...defaultAnalytixRuntimeSettings(),
        apiKey: canary
      }
    } as unknown as AppSettingsV1

    const normalized = normalizeAppSettings(polluted)
    const serialized = JSON.stringify(normalized)

    expect(serialized.includes(canary)).toBe(false)
    expect(serialized.includes('apiKey')).toBe(false)
    expect(Object.keys(normalized).sort()).toEqual(Object.keys(normalizeAppSettings(settings())).sort())
  })

  it('rejects Reasonix auto-plan project and runtime override shapes', () => {
    const base = settings()
    const polluted = {
      ...base,
      autoPlan: true,
      auto_plan: true,
      agent: {
        auto_plan: true
      },
      agentProvider: 'reasonix',
      agents: {
        reasonix: {
          model: 'reasonix-app-model',
          apiKey: 'sk-reasonix-app'
        }
      },
      deepseek: {
        apiKey: 'sk-deepseek-app'
      },
      reasonix: {
        model: 'reasonix-app-model'
      },
      runtime: {
        ...base.runtime,
        autoPlan: true,
        auto_plan: true,
        agent: {
          auto_plan: true
        },
        agentProvider: 'reasonix',
        agents: {
          reasonix: {
            model: 'reasonix-runtime-model',
            apiKey: 'sk-reasonix-runtime'
          }
        },
        deepseek: {
          apiKey: 'sk-deepseek-runtime'
        },
        reasonix: {
          model: 'reasonix-runtime-model'
        }
      }
    } as unknown as AppSettingsV1

    const normalized = normalizeAppSettings(polluted)

    expect(buildAnalytixRuntimeSettingsKey(polluted)).toBe(buildAnalytixRuntimeSettingsKey(base))
    expect('agent' in normalized).toBe(false)
    expect('agentProvider' in normalized).toBe(false)
    expect('agents' in normalized).toBe(false)
    expect('deepseek' in normalized).toBe(false)
    expect('reasonix' in normalized).toBe(false)
    expect('autoPlan' in normalized).toBe(false)
    expect('auto_plan' in normalized).toBe(false)
    expect('autoPlan' in normalized.runtime).toBe(false)
    expect('auto_plan' in normalized.runtime).toBe(false)
    expect('agent' in normalized.runtime).toBe(false)
    expect('agentProvider' in normalized.runtime).toBe(false)
    expect('agents' in normalized.runtime).toBe(false)
    expect('deepseek' in normalized.runtime).toBe(false)
    expect('reasonix' in normalized.runtime).toBe(false)
  })

  it('normalizes old master settings without a runtime block', () => {
    const normalized = normalizeAppSettings({
      version: 1,
      locale: 'zh',
      theme: 'dark',
      uiFontScale: 'small',
      agentProvider: 'deepseek-runtime',
      deepseek: {
        binaryPath: '/usr/local/bin/deepseek',
        port: 8787,
        autoStart: false,
        apiKey: 'sk-old',
        baseUrl: 'https://api.deepseek.com',
        runtimeToken: 'old-token',
        extraCorsOrigins: [],
        approvalPolicy: 'on-request',
        sandboxMode: 'read-only'
      },
      workspaceRoot: '/tmp/legacy-workspace',
      log: { enabled: true, retentionDays: 2 },
      notifications: { turnComplete: true },
      guiUpdate: { channel: 'beta' },
      claw: defaultClawSettings()
    } as unknown as AppSettingsV1)

    expect(normalized.runtime).toEqual(expect.objectContaining({
      binaryPath: '',
      port: 8787,
      autoStart: false,
      runtimeToken: 'old-token',
      approvalPolicy: 'on-request',
      sandboxMode: 'read-only'
    }))
    expect(normalized.provider).toEqual(expect.objectContaining({
      baseUrl: 'https://api.deepseek.com'
    }))
    expect(JSON.stringify(normalized).includes('sk-old')).toBe(false)
    expect('agentProvider' in normalized).toBe(false)
    expect('deepseek' in normalized).toBe(false)
  })

  it('moves the legacy local HTTP default port to the Analytix default port', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'deepseek-runtime',
      deepseek: {
        port: 7878
      }
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime?.port).toBe(8899)
  })

  it('fills image generation defaults for settings stored before the feature existed', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'deepseek-runtime',
      deepseek: {}
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime?.imageGeneration).toEqual({
      enabled: false,
      providerId: '',
      protocol: 'openai-images',
      baseUrl: '',
      model: '',
      defaultSize: '',
      timeoutMs: 180000
    })
  })

  it('uses the current approval policy default for missing legacy local HTTP settings', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      agentProvider: 'deepseek-runtime',
      deepseek: {}
    } as unknown as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime?.approvalPolicy).toBe(DEFAULT_APPROVAL_POLICY)
  })

  it('upgrades old persisted Analytix defaults to the current defaults', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      runtime: {
          dataDir: '~/.analytix/coreagent',
          model: 'deepseek-chat'
        }
    } as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime).toEqual(expect.objectContaining({
      dataDir: DEFAULT_ANALYTIX_DATA_DIR,
      model: DEFAULT_ANALYTIX_MODEL
    }))
  })

  it('preserves a non-legacy Analytix model override', () => {
    const migrated = migrateLegacyAppSettings({
      version: 1,
      runtime: {
          dataDir: '/tmp/custom-analytix',
          model: 'deepseek-v4-flash'
        }
    } as Parameters<typeof migrateLegacyAppSettings>[0])

    expect(migrated.runtime).toEqual(expect.objectContaining({
      dataDir: '/tmp/custom-analytix',
      model: 'deepseek-v4-flash'
    }))
  })

  it('preserves custom model providers while migrating legacy settings', () => {
    const migrated = normalizeAppSettings({
      ...settings(),
      agentProvider: 'deepseek-runtime',
      provider: {
        apiKey: 'sk-default',
        baseUrl: 'https://api.deepseek.com',
        providers: [
          ...defaultModelProviderSettings().providers,
          {
            id: 'custom-provider-2',
            name: 'Custom Provider',
            apiKey: 'sk-custom',
            baseUrl: 'https://custom.example/v1',
            endpointFormat: 'responses',
            models: ['custom-model']
          }
        ]
      },
      runtime: {
          ...defaultAnalytixRuntimeSettings(),
          providerId: 'custom-provider-2',
          model: 'custom-model'
        }
    } as unknown as AppSettingsV1)

    expect(migrated.provider.providers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'custom-provider-2',
          name: 'Custom Provider',
          baseUrl: 'https://custom.example/v1',
          endpointFormat: 'responses',
          models: ['custom-model']
        })
      ])
    )
    expect(migrated.runtime.providerId).toBe('custom-provider-2')
    expect(resolveAnalytixRuntimeSettings(migrated)).toEqual(
      expect.objectContaining({
        baseUrl: 'https://custom.example/v1',
        endpointFormat: 'responses'
      })
    )
    expect(JSON.stringify(migrated).includes('sk-custom')).toBe(false)
  })
})

describe('schedule settings', () => {
  it('provides independent top-level schedule defaults', () => {
    const defaults = defaultScheduleSettings()

    expect(defaults.enabled).toBe(false)
    expect(defaults.keepAwake).toBe(false)
    expect(defaults.internal.port).toBe(DEFAULT_SCHEDULE_INTERNAL_PORT)
    expect(defaults.tasks).toEqual([])
  })

  it('normalizes and merges schedule patches without reading legacy claw tasks', () => {
    const legacyTask = {
      id: 'legacy-claw-task',
      title: 'Legacy task',
      enabled: true,
      prompt: 'Old Claw task',
      workspaceRoot: '/tmp/workspace',
      clawChannelId: 'channel-1',
      model: 'auto',
      reasoningEffort: 'medium' as const,
      mode: 'agent' as const,
      schedule: { kind: 'daily' as const, everyMinutes: 60, timeOfDay: '08:00', atTime: '' },
      createdAt: '2026-06-02T00:00:00.000Z',
      updatedAt: '2026-06-02T00:00:00.000Z',
      lastRunAt: '',
      nextRunAt: '',
      lastStatus: 'idle' as const,
      lastMessage: '' as const,
      lastThreadId: ''
    }
    const normalized = normalizeAppSettings({
      ...settings(),
      claw: {
        ...defaultClawSettings(),
        tasks: [legacyTask]
      },
      schedule: undefined as unknown as AppSettingsV1['schedule']
    })

    expect(normalized.claw.tasks).toHaveLength(1)
    expect(normalized.schedule.tasks).toEqual([])

    const merged = mergeScheduleSettings(normalizeScheduleSettings(undefined), {
      enabled: true,
      defaultWorkspaceRoot: ' /tmp/schedule ',
      internal: { port: 99, secret: ' secret ' },
      tasks: [{
        title: 'Daily',
        prompt: 'Run',
        schedule: { kind: 'daily', everyMinutes: 0, timeOfDay: 'bad', atTime: 'not-a-date' }
      }]
    })

    expect(merged.enabled).toBe(true)
    expect(merged.defaultWorkspaceRoot).toBe('/tmp/schedule')
    expect(merged.internal.port).toBe(1024)
    expect(merged.internal.secret).toBe('secret')
    expect(merged.tasks[0].schedule.everyMinutes).toBe(1)
    expect(merged.tasks[0].schedule.timeOfDay).toBe('09:00')
    expect(merged.tasks[0].schedule.atTime).toBe('')
    expect(merged.tasks[0].clawChannelId).toBe('')
    expect(merged.tasks[0].reasoningEffort).toBe('medium')
  })

  it('normalizes arbitrary and status-inconsistent task messages to closed host keys', () => {
    const rawTask = {
      title: 'Daily',
      prompt: 'Run',
      lastStatus: 'error' as const,
      lastMessage: 'MARKER_FREE_TURN_ERROR_SENTINEL',
      schedule: { kind: 'daily' as const, everyMinutes: 60, timeOfDay: '09:00', atTime: '' }
    }
    const inconsistentTask = {
      ...rawTask,
      lastStatus: 'success' as const,
      lastMessage: 'schedule_task_started'
    }

    const normalized = normalizeScheduleSettings({
      tasks: [rawTask, inconsistentTask] as never
    })

    expect(normalized.tasks.map((task) => task.lastMessage)).toEqual([
      'schedule_task_failed',
      'schedule_task_completed'
    ])
    expect(scheduleSettingsNeedsMessageKeyMigration({ tasks: [rawTask] as never })).toBe(true)
    expect(scheduleSettingsNeedsMessageKeyMigration({ tasks: normalized.tasks })).toBe(false)
  })
})

describe('claw runtime prompts', () => {
  it('does not duplicate default Schedule MCP tool instructions in managed prompts', () => {
    const state = settings()
    state.claw.channels = [{
      id: 'channel-1',
      provider: 'feishu',
      label: 'analytix',
      enabled: true,
      model: 'auto',
      threadId: '',
      workspaceRoot: '',
      conversations: [],
      agentProfile: {
        name: 'analytix',
        description: '',
        identity: '',
        personality: '',
        userContext: '',
        replyRules: ''
      },
      createdAt: '2026-06-01T00:00:00.000Z',
      updatedAt: '2026-06-01T00:00:00.000Z'
    }]

    const prompt = buildClawRuntimePrompt(state, 'hi', { channel: state.claw.channels[0] })

    expect(prompt).toContain('[Connect Phone managed instructions]')
    expect(prompt).not.toContain('[Claw managed instructions]')
    expect(prompt).toContain('[Agent name]\nanalytix')
    expect(prompt).not.toContain('gui_schedule')
    expect(prompt).not.toContain('scheduled-task tools')
  })

  it('does not advertise the unsupported image tool when compatibility settings are present', () => {
    const state = settings()
    state.runtime.imageGeneration = {
      enabled: true,
      providerId: '',
      protocol: 'openai-images',
      baseUrl: 'https://images.example.test/v1',
      model: 'test-image-model',
      defaultSize: '1024x1024',
      timeoutMs: 180000
    }

    const prompt = buildClawRuntimePrompt(state, 'draw a small logo')

    expect(prompt).not.toContain('Image generation is enabled for this Connect Phone agent')
    expect(prompt).not.toContain('Image generation is enabled for this Claw agent')
    expect(prompt).not.toContain('generate_image')
  })

  it('does not advertise unsupported media tools when compatibility settings are present', () => {
    const state = settings()
    state.runtime.textToSpeech = {
      enabled: true,
      providerId: '',
      protocol: 'minimax-t2a',
      baseUrl: 'https://api.minimax.io',
      model: 'speech-2.8-hd',
      voice: 'male-qn-qingse',
      format: 'mp3',
      timeoutMs: 120000
    }
    state.runtime.musicGeneration = {
      enabled: true,
      providerId: '',
      protocol: 'minimax-music',
      baseUrl: 'https://api.minimax.io',
      model: 'music-2.6',
      format: 'mp3',
      timeoutMs: 300000
    }
    state.runtime.videoGeneration = {
      enabled: true,
      providerId: '',
      protocol: 'minimax-video',
      baseUrl: 'https://api.minimax.io',
      model: 'MiniMax-Hailuo-2.3',
      defaultDuration: 6,
      defaultResolution: '1080P',
      timeoutMs: 900000,
      pollIntervalMs: 10000
    }

    const prompt = buildClawRuntimePrompt(state, 'make a voiceover, jingle, and video')

    expect(prompt).not.toContain('Text-to-speech generation is enabled for this Connect Phone agent')
    expect(prompt).not.toContain('generate_speech')
    expect(prompt).not.toContain('Music generation is enabled for this Connect Phone agent')
    expect(prompt).not.toContain('generate_music')
    expect(prompt).not.toContain('Video generation is enabled for this Connect Phone agent')
    expect(prompt).not.toContain('generate_video')
    expect(prompt).not.toContain('Claw agent')
  })

  it('parses managed IM prompts into compact display text', () => {
    const parsed = parseClawUserPromptForDisplay([
      '[Connect Phone managed instructions]',
      '',
      '[Connect Phone agent instructions]',
      '',
      '[Agent name]',
      'analytix',
      '',
      '---',
      '[Current user request]',
      '[Feishu / Lark inbound message]',
      'Chat type: p2p',
      'Sender: user-1',
      '',
      'hi'
    ].join('\n'))

    expect(parsed).toMatchObject({
      text: 'hi',
      managed: true,
      inbound: true,
      sender: 'user-1',
      chatType: 'p2p'
    })
  })
})

describe('write inline completion runtime config', () => {
  it('falls back to the General baseUrl when write has no override', () => {
    const state = settings()
    state.provider.baseUrl = 'https://general.example/v1'
    expect(resolveWriteInlineCompletionBaseUrl(state)).toBe('https://general.example/v1')
  })

  it('preserves an explicit write-only baseUrl override', () => {
    const state = settings()
    state.provider.baseUrl = 'https://general.example/v1'
    state.write.inlineCompletion.baseUrl = 'https://write-only.example/v1'
    expect(resolveWriteInlineCompletionBaseUrl(state)).toBe('https://write-only.example/v1')
  })

  it('falls back to the analytix model when write keeps the default inline model', () => {
    const state = settings()
    state.runtime.model = 'deepseek-chat'
    expect(resolveWriteInlineCompletionModel(state)).toBe('deepseek-chat')
  })

  it('keeps an explicit flash override when write disables inheritance', () => {
    const state = settings()
    state.runtime.model = 'deepseek-chat'
    state.write.inlineCompletion.inheritModel = false
    state.write.inlineCompletion.model = 'deepseek-v4-flash'

    expect(resolveWriteInlineCompletionModel(state)).toBe('deepseek-v4-flash')
  })

  it('preserves an explicit request model before any fallback', () => {
    const state = settings()
    state.runtime.model = 'deepseek-chat'
    expect(resolveWriteInlineCompletionModel(state, 'deepseek-v4-pro')).toBe('deepseek-v4-pro')
  })

  it('tolerates legacy write inline settings without current override fields', async () => {
    const state = settings()
    state.provider.baseUrl = 'https://general.example/v1'
    state.runtime.model = 'deepseek-chat'
    const legacyInlineCompletion = { ...state.write.inlineCompletion } as Partial<AppSettingsV1['write']['inlineCompletion']>
    delete legacyInlineCompletion.baseUrl
    delete legacyInlineCompletion.inheritModel
    delete legacyInlineCompletion.model
    state.write.inlineCompletion = legacyInlineCompletion as AppSettingsV1['write']['inlineCompletion']

    expect('resolveWriteInlineCompletionApiKey' in await import('./app-settings')).toBe(false)
    expect(resolveWriteInlineCompletionBaseUrl(state)).toBe('https://general.example/v1')
    expect(resolveWriteInlineCompletionModel(state)).toBe('deepseek-chat')
  })

  it('treats legacy flash defaults without an inherit flag as inherited', () => {
    const state = settings()
    state.runtime.model = 'deepseek-chat'
    const legacyInlineCompletion = {
      ...state.write.inlineCompletion,
      model: 'deepseek-v4-flash'
    } as Partial<AppSettingsV1['write']['inlineCompletion']>
    delete legacyInlineCompletion.inheritModel
    state.write.inlineCompletion = legacyInlineCompletion as AppSettingsV1['write']['inlineCompletion']

    expect(resolveWriteInlineCompletionModel(state)).toBe('deepseek-chat')
  })
})

describe('write selection assist settings', () => {
  it('defaults to the built-in quick actions with empty overrides', () => {
    const write = defaultWriteSettings()
    expect(write.selectionAssist.infographicPrompt).toBe('')
    expect(write.selectionAssist.quickActions).toEqual([
      { id: 'polish', label: '', prompt: '', mode: 'chat' },
      { id: 'explain', label: '', prompt: '', mode: 'chat' },
      { id: 'reformat', label: '', prompt: '', mode: 'edit' }
    ])
  })

  it('keeps the defaults when legacy settings lack selectionAssist', () => {
    const write = normalizeWriteSettings({ defaultWorkspaceRoot: '/tmp/w' })
    expect(write.selectionAssist).toEqual(defaultWriteSelectionAssistSettings())
  })

  it('replaces quick actions wholesale through a merge patch', () => {
    const current = defaultWriteSettings()
    const next = mergeWriteSettings(current, {
      selectionAssist: {
        quickActions: [{ id: 'polish', label: '提升写作', prompt: '改写得更好' }]
      }
    })
    expect(next.selectionAssist.quickActions).toEqual([
      { id: 'polish', label: '提升写作', prompt: '改写得更好', mode: 'chat' }
    ])
    expect(next.selectionAssist.infographicPrompt).toBe('')
  })

  it('honors an explicit quick action mode and defaults custom actions to chat', () => {
    const write = normalizeWriteSettings({
      selectionAssist: {
        quickActions: [
          { id: 'polish', label: '保留', prompt: '保留', mode: 'chat' },
          { id: 'custom-1', label: 'x', prompt: 'y' }
        ]
      }
    })
    expect(write.selectionAssist.quickActions).toEqual([
      { id: 'polish', label: '保留', prompt: '保留', mode: 'chat' },
      { id: 'custom-1', label: 'x', prompt: 'y', mode: 'chat' }
    ])
  })

  it('preserves quick actions when only the infographic prompt changes', () => {
    const current = mergeWriteSettings(defaultWriteSettings(), {
      selectionAssist: {
        quickActions: [{ id: 'custom-1', label: '重写', prompt: '重写这段' }]
      }
    })
    const next = mergeWriteSettings(current, {
      selectionAssist: { infographicPrompt: '手绘风格' }
    })
    expect(next.selectionAssist.infographicPrompt).toBe('手绘风格')
    expect(next.selectionAssist.quickActions).toEqual([
      { id: 'custom-1', label: '重写', prompt: '重写这段', mode: 'chat' }
    ])
  })

  it('carries the design and prototype prompts through normalization', () => {
    const write = normalizeWriteSettings({
      selectionAssist: {
        designDraftPrompt: '移动端高保真。',
        prototypePrompt: '暗色主题原型。'
      }
    })
    expect(write.selectionAssist.designDraftPrompt).toBe('移动端高保真。')
    expect(write.selectionAssist.prototypePrompt).toBe('暗色主题原型。')

    const next = mergeWriteSettings(defaultWriteSettings(), {
      selectionAssist: { prototypePrompt: '原型用 vue 风格组件。' }
    })
    expect(next.selectionAssist.prototypePrompt).toBe('原型用 vue 风格组件。')
    expect(next.selectionAssist.designDraftPrompt).toBe('')
  })

  it('drops duplicate and id-less quick actions but keeps unfinished custom rows', () => {
    const write = normalizeWriteSettings({
      selectionAssist: {
        quickActions: [
          { id: 'polish', label: '', prompt: '' },
          { id: 'polish', label: 'dupe', prompt: 'dupe' },
          { id: '', label: 'no-id', prompt: 'no-id' },
          { id: 'custom-1', label: '', prompt: '' }
        ]
      }
    })
    expect(write.selectionAssist.quickActions).toEqual([
      { id: 'polish', label: '', prompt: '', mode: 'chat' },
      { id: 'custom-1', label: '', prompt: '', mode: 'chat' }
    ])
  })

  it('does not trim label or prompt text during normalization', () => {
    const write = normalizeWriteSettings({
      selectionAssist: {
        quickActions: [{ id: 'polish', label: 'hello ', prompt: 'world ' }]
      }
    })
    expect(write.selectionAssist.quickActions[0]).toEqual({
      id: 'polish',
      label: 'hello ',
      prompt: 'world ',
      mode: 'chat'
    })
  })

  it('drops pristine retired built-ins and migrates pristine polish to the sidebar mode', () => {
    // Stored rows from before proofread was retired and polish moved to chat.
    const write = normalizeWriteSettings({
      selectionAssist: {
        quickActions: [
          { id: 'polish', label: '', prompt: '', mode: 'edit' },
          { id: 'proofread', label: '', prompt: '', mode: 'edit' },
          { id: 'explain', label: '', prompt: '', mode: 'chat' }
        ]
      }
    })
    expect(write.selectionAssist.quickActions).toEqual([
      { id: 'polish', label: '', prompt: '', mode: 'chat' },
      { id: 'explain', label: '', prompt: '', mode: 'chat' }
    ])
  })

  it('keeps customized retired or edit-mode rows as explicit user choices', () => {
    const write = normalizeWriteSettings({
      selectionAssist: {
        quickActions: [
          { id: 'proofread', label: '校对', prompt: '修正错别字', mode: 'edit' },
          { id: 'polish', label: '', prompt: '自定义润色提示', mode: 'edit' }
        ]
      }
    })
    expect(write.selectionAssist.quickActions).toEqual([
      { id: 'proofread', label: '校对', prompt: '修正错别字', mode: 'edit' },
      { id: 'polish', label: '', prompt: '自定义润色提示', mode: 'edit' }
    ])
  })
})

describe('write agent presets', () => {
  it('defaults to no agents (opt-in, ships no preset templates)', () => {
    expect(defaultWriteSettings().agentPresets).toEqual([])
  })

  it('drops pristine built-in templates left over from older builds', () => {
    expect(
      normalizeWriteAgentPresets([
        { id: 'coordinator', name: '', emoji: '🧭', persona: '' },
        { id: 'editor', name: '', emoji: '✒️', persona: '' }
      ])
    ).toEqual([])
  })

  it('keeps customized built-ins and user-defined agents', () => {
    expect(
      normalizeWriteAgentPresets([
        { id: 'coordinator', name: '我的统筹', emoji: '🧭', persona: '' },
        { id: 'custom-1', name: '', emoji: '🤖', persona: '专属人设' }
      ])
    ).toEqual([
      { id: 'coordinator', name: '我的统筹', emoji: '🧭', persona: '' },
      { id: 'custom-1', name: '', emoji: '🤖', persona: '专属人设' }
    ])
  })
})
