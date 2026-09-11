import { describe, expect, it, vi } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import {
  DEFAULT_MODEL_PROVIDER_ID,
  defaultAnalytixRuntimeSettings,
  defaultModelProviderSettings,
  getModelProviderPreset,
  modelProviderPresetProfile,
  normalizeAppSettings,
  type AppSettingsV1,
  type ModelProviderProfileV1
} from '@shared/app-settings'
import { AgentsSettingsSection, modelProvidersSettingsPatch } from './settings-section-agents'
import { PermissionsSettingsSection } from './settings-section-permissions'
import { ProvidersSettingsSection } from './settings-section-providers'
import { applyWriteSettingsState } from '../write/write-workspace-settings-actions'

const labels: Record<string, string> = {
  agentsQuickBase: 'Base',
  agentsQuickSkill: 'Skills',
  agentsQuickMcp: 'MCP',
  agentsQuickPermissions: 'Permissions',
  agents: 'Agents',
  providers: 'Providers',
  providersDesc: 'Providers description',
  analytixProvider: 'Provider',
  analytixProviderDesc: 'Provider description',
  analytixProviderSelectDesc: 'Provider select description',
  modelProviderAdd: 'Add provider',
  modelProviderAddMenuCustom: 'Custom provider…',
  modelProviderSectionBasics: 'Provider basics',
  modelProviderSectionConnection: 'Provider connection',
  modelProviderSectionDanger: 'Danger zone',
  modelProviderTestConnection: 'Test connection',
  modelProviderFetchModels: 'Fetch from API',
  modelProviderModelsPlaceholder: 'Type a model ID and press Enter',
  modelProviderModelCount: 'models count',
  modelProviderInUse: 'In use',
  modelProviderMissingKey: 'No API key',
  modelProviderDefaultBadge: 'Default',
  modelProviderPresetBadge: 'Preset',
  modelProviderCustomBadge: 'Custom',
  modelProviderDangerHint: 'Danger hint',
  modelProviderIdLocked: 'Provider ID locked',
  modelProviderRemove: 'Remove provider',
  modelProviderName: 'Provider name',
  modelProviderId: 'Provider ID',
  modelProviderApiKey: 'Provider API key',
  modelProviderApiKeyPlaceholder: 'Enter provider API key',
  modelProviderBaseUrl: 'Provider base URL',
  modelProviderEndpointFormat: 'Endpoint format',
  modelProviderFetchEmpty: 'No models found',
  modelEndpointChatCompletions: '/v1/chat/completions',
  modelEndpointResponses: '/v1/responses',
  modelEndpointMessages: '/v1/messages',
  modelEndpointCustomEndpoint: 'Custom full endpoint',
  modelProviderModels: 'Provider models',
  modelProviderImageCapability: 'Image capability',
  modelProviderImageCapabilityDesc: 'Image capability description',
  modelProviderImageEnable: 'Enable image',
  modelProviderImageDisable: 'Disable image',
  imageGenProtocol: 'Image protocol',
  imageGenProtocolOpenAi: 'OpenAI Images',
  imageGenProtocolMiniMax: 'MiniMax image_generation',
  imageGenBaseUrl: 'Image base URL',
  imageGenModel: 'Image model',
  imageGenBaseUrlPlaceholder: 'https://api.example.com/v1',
  baseUrlPlaceholder: 'https://api.example.com/v1',
  analytixApiKey: 'Analytix API key',
  analytixApiKeyDesc: 'Analytix API key description',
  analytixApiKeyPlaceholder: 'Inherit API key',
  analytixApiKeyInherited: 'Inherited API key',
  analytixApiKeyMissing: 'Missing API key',
  analytixApiKeyOverride: 'Override API key',
  analytixBaseUrl: 'Analytix base URL',
  analytixBaseUrlDesc: 'Analytix base URL description',
  analytixBaseUrlPlaceholder: 'Inherit base URL',
  analytixBaseUrlOfficial: 'Official base URL',
  analytixBaseUrlInherited: 'Inherited base URL',
  analytixBaseUrlOverride: 'Override base URL',
  analytixAssistantAdvanced: 'Assistant advanced settings',
  analytixAssistantAdvancedDesc: 'Assistant advanced settings description',
  autoStart: 'Auto start',
  autoStartDesc: 'Auto start description',
  port: 'Port',
  portDesc: 'Port description',
  analytixDataDir: 'Data dir',
  analytixDataDirDesc: 'Data dir description',
  analytixModel: 'Model',
  analytixModelDesc: 'Model description',
  analytixTokenEconomy: 'Token-saving mode',
  analytixTokenEconomyDesc: 'Token-saving mode description',
  analytixTokenEconomySavings: 'Saved {{tokens}} tokens',
  analytixTokenEconomySavingsLoading: 'Loading savings',
  analytixTokenEconomySavingsEmpty: 'Savings empty',
  analytixTokenEconomyAdvanced: 'Token-saving advanced settings',
  analytixTokenEconomyAdvancedDesc: 'Token-saving advanced settings description',
  analytixTokenEconomyOptions: 'Token-saving options',
  analytixTokenEconomyOptionsDesc: 'Token-saving options description',
  analytixCompressToolDescriptions: 'Compress tool descriptions',
  analytixCompressToolResults: 'Compress tool results',
  analytixConciseResponses: 'Concise responses',
  analytixHistoryHygiene: 'History guard',
  analytixHistoryHygieneDesc: 'History guard description',
  analytixHistoryMaxResultLines: 'Max result lines',
  analytixHistoryMaxResultBytes: 'Max result bytes',
  analytixHistoryMaxResultTokens: 'Max result tokens',
  analytixHistoryMaxArgumentBytes: 'Max argument bytes',
  analytixHistoryMaxArgumentTokens: 'Max argument tokens',
  analytixHistoryMaxArrayItems: 'Max array items',
  runtimeToken: 'Runtime token',
  runtimeTokenDesc: 'Runtime token description',
  showSecret: 'Show',
  hideSecret: 'Hide',
  analytixInsecure: 'Insecure',
  analytixInsecureDesc: 'Insecure description',
  analytixInsecureForcedDesc: 'Insecure forced',
  analytixAdvanced: 'Advanced runtime settings',
  analytixAdvancedDetails: 'Runtime timeout and sub-agent controls',
  analytixAdvancedDetailsDesc: 'Production runtime controls',
  analytixStorageBackend: 'Storage backend',
  analytixStorageBackendDesc: 'Storage backend description',
  analytixStorageHybrid: 'Hybrid storage',
  analytixStorageFile: 'Pure JSONL file storage',
  analytixStorageSqlitePath: 'SQLite path',
  analytixStorageSqlitePathDesc: 'SQLite path description',
  analytixStorageSqlitePathPlaceholder: 'Automatic SQLite path',
  analytixModelContextProfile: 'Current model context policy',
  analytixModelContextProfileDesc: 'Current model context policy description',
  analytixModelContextModel: 'Matched model',
  analytixModelContextWindow: 'Context window',
  analytixModelContextSoft: 'Model soft threshold',
  analytixModelContextHard: 'Model hard threshold',
  analytixModelContextSourceBuiltIn: 'Built-in model config',
  analytixModelContextSourceFallback: 'Fallback model config',
  analytixCompactionThresholds: 'Fallback compaction thresholds',
  analytixCompactionThresholdsDesc: 'Fallback compaction thresholds description',
  analytixCompactionSoftThreshold: 'Fallback soft threshold',
  analytixCompactionHardThreshold: 'Fallback hard threshold',
  analytixCompactionSummary: 'Compaction summary',
  analytixCompactionSummaryDesc: 'Compaction summary description',
  analytixCompactionSummaryMode: 'Summary mode',
  analytixCompactionSummaryHeuristic: 'Heuristic summary',
  analytixCompactionSummaryModel: 'Model summary',
  analytixCompactionSummaryTimeout: 'Summary timeout',
  analytixCompactionSummaryMaxTokens: 'Summary max tokens',
  analytixCompactionSummaryInputBytes: 'Summary input bytes',
  analytixStreamIdleTimeout: 'Stream idle timeout',
  analytixStreamIdleTimeoutDesc: 'Zero disables the idle watchdog',
  analytixToolStorm: 'Tool storm',
  analytixToolStormDesc: 'Tool storm description',
  analytixToolStormLimits: 'Tool storm limits',
  analytixToolStormLimitsDesc: 'Tool storm limits description',
  analytixToolStormWindowSize: 'Tool storm window',
  analytixToolStormThreshold: 'Tool storm threshold',
  analytixToolArgumentRepair: 'Tool argument repair',
  analytixToolArgumentRepairDesc: 'Tool argument repair description',
  analytixSubagents: 'Sub-agent profiles',
  analytixSubagentsDesc: 'Sub-agent profiles description',
  analytixSubagentsEnabled: 'Enable sub-agents',
  analytixSubagentsDefaultPolicy: 'Default tool policy',
  analytixSubagentsMaxParallel: 'Max parallel',
  analytixSubagentsMaxChildRuns: 'Max child runs',
  analytixSubagentsDefaultProfile: 'Default profile',
  analytixSubagentsNoDefaultProfile: 'No default profile',
  analytixSubagentsProfileNamePlaceholder: 'profile-name',
  analytixSubagentsAddProfile: 'Add profile',
  analytixSubagentsRemoveProfile: 'Remove profile',
  analytixSubagentsProfileModel: 'Model override',
  analytixSubagentsProfileEffort: 'Effort',
  analytixSubagentsInherit: 'Inherit',
  analytixSubagentsProfilePolicy: 'Tool policy',
  analytixSubagentsProfileTools: 'Tool scope',
  analytixSubagentsProfilePrompt: 'Profile prompt',
  analytixDiagnostics: 'Analytix diagnostics',
  analytixDiagnosticsAdvanced: 'Detailed diagnostics',
  analytixDiagnosticsAdvancedDesc: 'Detailed diagnostics description',
  analytixRuntimeCapabilities: 'Runtime capabilities',
  analytixRuntimeCapabilitiesDesc: 'Runtime capabilities description',
  analytixRuntimeModel: 'Runtime model',
  analytixRuntimePid: 'Runtime PID',
  analytixDiagnosticsRefresh: 'Refresh diagnostics',
  analytixToolDiagnostics: 'Tool diagnostics',
  analytixToolDiagnosticsDesc: 'Tool diagnostics description',
  analytixDiagnosticsProviders: 'Providers',
  analytixDiagnosticsMcpServers: 'MCP servers',
  analytixDiagnosticsSkills: 'Discovered Skills',
  analytixDiagnosticsAttachments: 'Attachments',
  analytixDiagnosticsCommands: 'Commands',
  analytixMemoryRecords: 'Memory records',
  analytixMemoryRecordsDesc: 'Memory records description',
  analytixMemoryEmpty: 'No memories',
  analytixMemoryDisable: 'Disable memory',
  analytixMemoryDelete: 'Delete memory',
  analytixMemoryDisabled: 'Disabled',
  skill: 'Skill',
  skillsLocation: 'Skill location',
  skillsLocationDesc: 'Skill location description',
  skillsPath: 'Skills path',
  skillsPathDesc: 'Skills path description',
  skillsRootUnavailable: 'Unavailable',
  skillsScanDirs: 'Scan dirs',
  skillsScanDirsDesc: 'Scan dirs description',
  skillsActions: 'Skill actions',
  skillsActionsDesc: 'Skill actions description',
  skillsOpenRoot: 'Open root',
  skillsOpenPlugins: 'Open plugins',
  mcp: 'MCP',
  mcpSearchEnabled: 'MCP search enabled',
  mcpSearchEnabledDesc: 'MCP search description',
  mcpAdvanced: 'MCP advanced settings',
  mcpAdvancedDesc: 'MCP advanced settings description',
  mcpSearchMode: 'MCP search mode',
  mcpSearchModeDesc: 'MCP search mode description',
  mcpSearchModeAuto: 'Auto mode',
  mcpSearchModeSearch: 'Search mode',
  mcpSearchModeDirect: 'Direct mode',
  mcpSearchLimits: 'MCP search limits',
  mcpSearchLimitsDesc: 'MCP search limits description',
  mcpSearchAutoThreshold: 'Auto threshold',
  mcpSearchTopKDefault: 'Default results',
  mcpSearchTopKMax: 'Max results',
  mcpSearchMinScore: 'Minimum score',
  mcpSearchDiagnostics: 'MCP search diagnostics',
  mcpSearchDiagnosticsDesc: 'MCP search diagnostics description',
  mcpSearchStatus: 'MCP search status',
  mcpSearchActive: 'Active',
  mcpSearchInactive: 'Inactive',
  mcpSearchIndexed: 'Indexed',
  mcpSearchAdvertised: 'Advertised',
  configFilePath: 'External tool config path',
  mcpPathDesc: 'MCP JSON path description',
  mcpEditor: 'MCP editor',
  mcpEditorDesc: 'Model and API credentials do not live in this MCP file',
  mcpFileStatusReady: 'MCP config ready',
  mcpFileStatusMissing: 'MCP config missing',
  loading: 'Loading',
  mcpActions: 'MCP actions',
  mcpRuntimeHint: 'MCP runtime hint',
  mcpSave: 'Save MCP config',
  mcpReload: 'Reload MCP config',
  mcpOpenDir: 'Open MCP directory',
  permissions: 'Permissions',
  approvalPolicy: 'Approval policy',
  approvalPolicyDesc: 'Approval policy description',
  approvalAuto: 'Auto',
  approvalAlways: 'Always',
  approvalOnRequest: 'On request',
  approvalUntrusted: 'Untrusted',
  approvalSuggest: 'Suggest',
  approvalNever: 'Never',
  permissionsBehaviorHint: 'Full access and confirmation are separate',
  permissionsElevatedHint: 'Elevated access hint',
  sandboxMode: 'Sandbox mode',
  sandboxModeDesc: 'Sandbox description',
  sandboxWorkspaceWrite: 'Workspace write',
  sandboxReadOnly: 'Read only',
  sandboxFullAccess: 'Full access',
  sandboxExternal: 'External sandbox'
}

function t(key: string): string {
  return labels[key] ?? key
}

function baseCtx(): Record<string, unknown> {
  const noop = () => undefined
  const asyncNoop = async () => undefined
  const ref = { current: null }
  const analytix = {
    ...defaultAnalytixRuntimeSettings(),
    autoStart: true,
    runtimeToken: '',
    insecure: true
  }
  return {
    t,
    tCommon: t,
    form: { claw: { skills: { extraDirs: ['/tmp/project/.agents/skills'] } } },
    analytix,
    activeApiKey: '',
    update: noop,
    updateAnalytix: noop,
    updateSharedCredential: noop,
    sharedApiKey: '',
    sharedBaseUrl: '',
    showApiKey: false,
    setShowApiKey: noop,
    showRuntimeToken: false,
    setShowRuntimeToken: noop,
    portError: '',
    selectControlClass: 'select',
    openOnboardingPreview: noop,
    pickWorkspace: asyncNoop,
    resetWorkspaceToDefault: noop,
    workspacePickerError: '',
    guiUpdateInfo: null,
    checkingGuiUpdate: false,
    downloadingGuiUpdate: false,
    installingGuiUpdate: false,
    guiUpdateDownloaded: false,
    guiUpdateProgress: null,
    guiUpdateError: null,
    checkGuiUpdate: asyncNoop,
    downloadGuiUpdate: asyncNoop,
    installGuiUpdate: asyncNoop,
    logPath: '',
    logDirOpenError: '',
    setLogDirOpenError: noop,
    pickWriteWorkspace: asyncNoop,
    resetWriteWorkspaceToDefault: noop,
    writeWorkspacePickerError: '',
    writeInlineBaseUrlInherited: false,
    effectiveWriteInlineBaseUrl: '',
    writeInlineModelInherited: false,
    effectiveWriteInlineModel: '',
    setWriteDebugModalOpen: noop,
    loadWriteDebugEntries: asyncNoop,
    scrollToAgentSection: noop,
    agentsSectionRef: ref,
    skillSectionRef: ref,
    mcpSectionRef: ref,
    permissionsSectionRef: ref,
    skillRoots: [],
    skillRootsLoading: false,
    toggleSkillRoot: noop,
    skillNotice: null,
    openSkillRoot: asyncNoop,
    openPlugins: noop,
    mcpConfigPath: '/tmp/project/.analytix/mcp.json',
    mcpConfigExists: true,
    mcpConfigText: '{"mcpServers":{}}',
    setMcpConfigText: noop,
    mcpLoading: false,
    mcpBusy: false,
    mcpNotice: null,
    saveMcpConfig: asyncNoop,
    loadMcpConfig: asyncNoop,
    openMcpConfigDir: asyncNoop,
    runtimeInfo: null,
    toolDiagnostics: null,
    memoryRecords: [],
    runtimeDiagnosticsBusy: false,
    runtimeDiagnosticsNotice: null,
    refreshAnalytixDiagnostics: asyncNoop,
    disableMemoryRecord: asyncNoop,
    deleteMemoryRecord: asyncNoop,
    pickClawWorkspace: asyncNoop,
    resetClawWorkspaceToDefault: noop,
    clawWorkspacePickerError: '',
    splitSettingsList: (value: string) => value.split('\n').filter(Boolean),
    listSettingsText: (value: string[]) => value.join('\n')
  }
}

function diagnosticState(enabled: boolean, available: boolean) {
  return available
    ? { status: 'available', enabled: true, available: true, reasonCode: 'available' }
    : enabled
      ? { status: 'unavailable', enabled: true, available: false, reasonCode: 'unavailable' }
      : { status: 'disabled', enabled: false, available: false, reasonCode: 'disabled_by_config' }
}

function toolDiagnosticsV2(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const disabled = diagnosticState(false, false)
  return {
    schemaVersion: 2,
    providerCount: 0,
    toolContracts: { count: 0, catalogHash: 'a'.repeat(64) },
    mcpServers: [],
    mcpSearch: {
      enabled: false,
      mode: 'auto',
      active: false,
      available: false,
      reasonCode: 'disabled_by_config',
      indexedToolCount: 0,
      advertisedToolCount: 0,
      autoThresholdToolCount: 24,
      topKDefault: 8,
      topKMax: 24,
      minScore: 0,
      catalogDrift: false
    },
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: { mode: 'off', configured: false, source: 'unknown', valid: true, credentialsMasked: true },
    webProviderCount: 0,
    skills: { enabled: false, available: false, reasonCode: 'disabled_by_config', configuredRootCount: 0, skillCount: 0, validationErrorCount: 0 },
    attachments: {
      enabled: true,
      count: 0,
      totalBytes: 0,
      maxImageBytes: 1,
      maxImageDimension: 1,
      allowedMimeTypes: ['image/png'],
      allowedDocumentMimeTypes: ['text/plain'],
      maxDocumentBytes: 1,
      maxDocumentTextChars: 1
    },
    memory: { enabled: true, activeCount: 0, tombstoneCount: 0 },
    subagents: {
      ...disabled,
      active: 0,
      queued: 0,
      profileCount: 0,
      maxParallel: 0,
      maxChildRuns: 0,
      defaultToolPolicy: 'readOnly',
      internalLineageAvailable: false,
      parallelExecutionAvailable: false,
      taskToolAvailable: false,
      parallelTasksToolAvailable: false,
      backgroundTaskJobsAvailable: false,
      backgroundShellAvailable: false,
      backgroundSubagentJobsAvailable: false
    },
    ...overrides
  }
}

describe('AgentsSettingsSection Analytix diagnostics smoke', () => {
  it('updates runtime selection without writing added Provider metadata to ordinary settings', () => {
    const provider = defaultModelProviderSettings()
    const customProvider = {
      id: 'custom-provider-2',
      name: 'Custom Provider',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'responses',
      models: [],
      modelProfiles: {}
    } satisfies ModelProviderProfileV1

    const patch = modelProvidersSettingsPatch({
      provider,
      providers: [...provider.providers, customProvider],
      analytix: { providerId: customProvider.id }
    })

    expect(patch.provider).toBeUndefined()
    expect(patch.runtime?.providerId).toBe(customProvider.id)
    expect(patch.runtime?.baseUrl).toBe('')
  })

  it('updates runtime selection without writing removed Provider metadata to ordinary settings', () => {
    const provider = defaultModelProviderSettings()

    const patch = modelProvidersSettingsPatch({
      provider: {
        ...provider,
        providers: [
          ...provider.providers,
          {
            id: 'custom-provider-2',
            name: 'Custom Provider',
            baseUrl: 'https://api.example.com/v1',
            endpointFormat: 'responses',
            models: [],
            modelProfiles: {}
          }
        ]
      },
      providers: provider.providers,
      analytix: { providerId: DEFAULT_MODEL_PROVIDER_ID }
    })

    expect(patch.provider).toBeUndefined()
    expect(patch.runtime?.providerId).toBe(DEFAULT_MODEL_PROVIDER_ID)
    expect(patch.runtime?.baseUrl).toBe('')
  })

  it('keeps write AI readiness fail-closed without a public Registry credential projection', () => {
    const settings = normalizeAppSettings({} as AppSettingsV1)
    settings.runtime.model = 'synthetic-chat-model'
    settings.runtime.imageGeneration = {
      ...settings.runtime.imageGeneration,
      enabled: true,
      baseUrl: 'https://image.example/v1',
      model: 'synthetic-image-model'
    }
    settings.write.inlineCompletion = {
      ...settings.write.inlineCompletion,
      baseUrl: 'https://inline.example/v1',
      model: 'synthetic-inline-model'
    }
    const set = vi.fn()

    applyWriteSettingsState(set, settings)

    expect(set).toHaveBeenLastCalledWith(expect.objectContaining({
      inlineCompletionApiReady: false,
      imageGenReady: false,
      prototypeReady: false
    }))
  })

  it('treats preset metadata as catalog input and writes only runtime preferences', () => {
    const provider = defaultModelProviderSettings()
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProvider = modelProviderPresetProfile(xiaomi!)

    const patch = modelProvidersSettingsPatch({
      provider,
      providers: [...provider.providers, xiaomiProvider],
      analytix: {
        providerId: xiaomiProvider.id,
        model: xiaomiProvider.models[0]
      }
    })

    expect(patch.provider).toBeUndefined()
    expect(patch.runtime).toEqual(expect.objectContaining({
      providerId: 'xiaomi',
      model: xiaomiProvider.models[0]
    }))
  })

  it('does not persist discovered Provider models through ordinary settings', () => {
    const provider = defaultModelProviderSettings()
    const customProvider = {
      id: 'custom-provider-2',
      name: 'Custom Provider',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'responses',
      models: ['existing-model'],
      modelProfiles: {}
    } satisfies ModelProviderProfileV1
    const currentProvider = {
      ...provider,
      activeProviderId: customProvider.id,
      providers: [...provider.providers, customProvider]
    }
    const importedProvider = {
      ...customProvider,
      models: ['existing-model', 'api-model-a', 'api-model-b']
    } satisfies ModelProviderProfileV1

    const patch = modelProvidersSettingsPatch({
      provider: currentProvider,
      providers: [...provider.providers, importedProvider]
    })
    expect(patch.runtime).toBeUndefined()
    expect(patch.provider).toBeUndefined()
    expect(JSON.stringify(patch)).not.toContain('api-model-a')
    expect(JSON.stringify(patch)).not.toContain('api-model-b')
  })

  it('defaults MiniMax media generation when adding a configured MiniMax provider', () => {
    const provider = defaultModelProviderSettings()
    const minimax = getModelProviderPreset('minimax')
    expect(minimax).not.toBeNull()
    const minimaxProvider = modelProviderPresetProfile(minimax!)

    const patch = modelProvidersSettingsPatch({
      provider,
      providers: [...provider.providers, minimaxProvider],
      currentAnalytix: defaultAnalytixRuntimeSettings(),
      analytix: {
        providerId: minimaxProvider.id,
        model: minimaxProvider.models[0]
      }
    })

    expect(patch.runtime).toEqual(expect.objectContaining({
      providerId: 'minimax',
      model: minimaxProvider.models[0],
      textToSpeech: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        model: 'speech-2.8-hd'
      }),
      musicGeneration: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        model: 'music-2.6'
      }),
      videoGeneration: expect.objectContaining({
        enabled: true,
        providerId: 'minimax',
        model: 'MiniMax-Hailuo-2.3'
      })
    }))
  })

  it('does not render a custom Provider from ordinary settings before Registry readback', () => {
    const provider = defaultModelProviderSettings()
    const customProvider = {
      id: 'custom-provider-2',
      name: 'Custom Provider',
      baseUrl: 'https://api.example.com/v1',
      endpointFormat: 'messages',
      models: [],
      modelProfiles: {}
    } satisfies ModelProviderProfileV1
    const html = renderToStaticMarkup(createElement(ProvidersSettingsSection, {
      ctx: {
        ...baseCtx(),
        provider: {
          ...provider,
          providers: [...provider.providers, customProvider]
        },
        analytix: {
          ...defaultAnalytixRuntimeSettings(),
          providerId: customProvider.id
        }
      }
    }))
    const providerIdInput = html.match(/<input[^>]+value="custom-provider-2"[^>]*>/)?.[0]

    expect(providerIdInput).toBeUndefined()
    expect(html).not.toContain(customProvider.baseUrl)
    expect(html).toContain('Add provider')
    expect(html).not.toContain('Test connection')
  })

  it('does not render a preset Provider from ordinary settings before Registry readback', () => {
    const provider = defaultModelProviderSettings()
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const html = renderToStaticMarkup(createElement(ProvidersSettingsSection, {
      ctx: {
        ...baseCtx(),
        provider: {
          ...provider,
          providers: [...provider.providers, modelProviderPresetProfile(xiaomi!)]
        },
        analytix: {
          ...defaultAnalytixRuntimeSettings(),
          providerId: 'xiaomi'
        }
      }
    }))
    const providerIdInput = html.match(/<input[^>]+value="xiaomi"[^>]*>/)?.[0]

    expect(providerIdInput).toBeUndefined()
    expect(html).not.toContain('Provider ID locked')
    expect(html).not.toContain('Danger zone')
  })

  it('keeps the Provider surface empty until Registry readback', () => {
    const html = renderToStaticMarkup(createElement(ProvidersSettingsSection, {
      ctx: {
        ...baseCtx(),
        provider: defaultModelProviderSettings(),
        analytix: defaultAnalytixRuntimeSettings()
      }
    }))

    expect(html).not.toContain('Danger zone')
    expect(html).not.toContain('Test connection')
    expect(html).toContain('Add provider')
  })

  it('keeps advanced agent controls behind collapsed disclosures', () => {
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Assistant advanced settings')
    expect(html).toContain('Runtime timeout and sub-agent controls')
    expect(html).toContain('MCP advanced settings')
    expect(html).not.toContain('<details open')
  })

  it('does not expose a custom assistant executable path after Go-only cutover', () => {
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx: baseCtx() }))

    expect(html).not.toContain('Analytix binary')
    expect(html).not.toContain('Bundled Analytix')
    expect(html).not.toContain('binaryPath')
  })

  it('does not render image generation settings inside the agent section', () => {
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx: baseCtx() }))

    expect(html).not.toContain('imageGen')
  })

  it('preselects provider.activeProviderId when runtime providerId is blank', () => {
    const provider = defaultModelProviderSettings()
    const xiaomi = getModelProviderPreset('xiaomi')
    expect(xiaomi).not.toBeNull()
    const xiaomiProvider = modelProviderPresetProfile(xiaomi!)
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, {
      ctx: {
        ...baseCtx(),
        provider: {
          ...provider,
          activeProviderId: 'xiaomi',
          providers: [...provider.providers, xiaomiProvider]
        },
        analytix: {
          ...defaultAnalytixRuntimeSettings(),
          providerId: ''
        }
      }
    }))

    expect(html).toContain(`<option value="xiaomi" selected="">${xiaomiProvider.name}</option>`)
    expect(html).not.toContain('<option value="deepseek" selected="">')
  })

  it('renders conservative permission defaults without an elevated-access warning', () => {
    const html = renderToStaticMarkup(createElement(PermissionsSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Permissions')
    expect(html).toContain('Full access and confirmation are separate')
    expect(html).not.toContain('Elevated access hint')
    expect(html).toContain('<option value="on-request" selected="">On request</option>')
    expect(html).toContain('<option value="always">Always</option>')
    expect(html).toContain('<option value="workspace-write" selected="">Workspace write</option>')
    expect(html).toContain('<option value="danger-full-access">Full access</option>')
  })

  it('hides compatibility-only controls and keeps active runtime controls', () => {
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('Stream idle timeout')
    expect(html).toContain('Sub-agent profiles')
    expect(html).not.toContain('Storage backend')
    expect(html).not.toContain('Token-saving mode')
    expect(html).not.toContain('Current model context policy')
    expect(html).not.toContain('Fallback compaction thresholds')
    expect(html).not.toContain('Tool storm')
    expect(html).not.toContain('Tool argument repair')
    expect(html).not.toContain('Design quality')
  })

  it('renders MCP, Skill, web, attachment, and memory diagnostics', () => {
    const ctx = {
      ...baseCtx(),
      runtimeInfo: {
        schemaVersion: 2,
        status: 'ready',
        capabilities: {
          model: { id: 'deepseek-chat' },
          mcp: { ...diagnosticState(true, true), configuredServers: 2, connectedServers: 2 },
          web: { ...diagnosticState(true, true), provider: 'brave-search' },
          skills: diagnosticState(true, true),
          subagents: diagnosticState(true, true),
          computerUse: { ...diagnosticState(true, true), mode: 'auto' },
          attachments: diagnosticState(true, true),
          memory: diagnosticState(true, true)
        }
      },
      toolDiagnostics: toolDiagnosticsV2({
        providerCount: 4,
        mcpServers: [{
          id: 'github', status: 'connected', transport: 'stdio', authStatus: 'none', trustScope: 'workspace',
          enabled: true, available: true, connected: true, schemaHintAvailable: true, connectable: true,
          toolCount: 1, promptCount: 0, resourceCount: 0, toolContractQuarantineCount: 0,
          lowPriority: false, backgroundStart: false
        }],
        skills: { enabled: true, available: true, reasonCode: 'available', configuredRootCount: 1, skillCount: 1, validationErrorCount: 0 },
        attachments: {
          enabled: true,
          count: 1,
          totalBytes: 1,
          maxImageBytes: 1,
          maxImageDimension: 1,
          allowedMimeTypes: ['image/png'],
          allowedDocumentMimeTypes: ['text/plain'],
          maxDocumentBytes: 1,
          maxDocumentTextChars: 1
        },
        commands: [
          { binary: 'node', found: true, status: 'available' },
          { binary: 'git', found: true, status: 'available' },
          { binary: 'rg', found: false, status: 'unavailable' }
        ]
      }),
      memoryRecords: [
        {
          id: 'mem_1',
          content: 'Prefer pnpm for this workspace',
          scope: 'workspace',
          tags: ['tooling']
        }
      ]
    }

    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx }))

    expect(html).toContain('Analytix diagnostics')
    expect(html).toContain('MCP')
    expect(html).toContain('available')
    expect(html).toContain('2/2')
    expect(html).toContain('brave-search')
    expect(html).toContain('Computer Use')
    expect(html).toContain('auto')
    expect(html).toContain('Providers')
    expect(html).toContain('MCP servers')
    expect(html).toContain('Discovered Skills')
    expect(html).toContain('Commands')
    expect(html).toContain('2/3')
    expect(html).toContain('Prefer pnpm for this workspace')
    expect(html).toContain('mem_1')
    expect(html).toContain('Disable memory')
    expect(html).toContain('Delete memory')
  })

  it('uses only canonical public MCP diagnostics in product-facing counts', () => {
    const ctx = {
      ...baseCtx(),
      runtimeInfo: {
        schemaVersion: 2,
        status: 'degraded',
        capabilities: {
          model: { id: 'deepseek-chat' },
          mcp: {
            ...diagnosticState(true, false),
            configuredServers: 0,
            connectedServers: 0
          },
          web: diagnosticState(false, false),
          skills: diagnosticState(false, false),
          subagents: diagnosticState(true, false),
          attachments: diagnosticState(true, true),
          memory: diagnosticState(true, true)
        }
      },
      toolDiagnostics: toolDiagnosticsV2({ providerCount: 1 })
    }

    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx }))

    expect(html).toContain('MCP')
    expect(html).toContain('0/0')
    expect(html).not.toContain('local validation only')
    expect(html).toContain('MCP servers')
    expect(html).not.toContain('1/1')
    expect(html).not.toContain('analytix-local-validation-marker')
  })

  it('labels internal sub-agent lineage separately from configurable profiles', () => {
    const ctx = {
      ...baseCtx(),
      runtimeInfo: {
        schemaVersion: 2,
        status: 'degraded',
        capabilities: {
          model: { id: 'deepseek-chat' },
          mcp: { ...diagnosticState(true, false), configuredServers: 0, connectedServers: 0 },
          web: diagnosticState(false, false),
          skills: diagnosticState(false, false),
          subagents: {
            ...diagnosticState(false, false),
            internalLineageAvailable: true,
            profilesAvailable: true,
            durableChildRunStore: true,
            parallelExecutionAvailable: true,
            maxParallel: 8,
            maxChildRuns: 64,
            profileCount: 0
          },
          attachments: diagnosticState(true, true),
          memory: diagnosticState(true, true)
        }
      },
      toolDiagnostics: toolDiagnosticsV2({ subagents: {
          ...diagnosticState(false, false),
          active: 0,
          queued: 0,
          profileCount: 0,
          maxParallel: 8,
          maxChildRuns: 64,
          defaultToolPolicy: 'readOnly',
          internalLineageAvailable: true,
          parallelExecutionAvailable: true,
          taskToolAvailable: false,
          parallelTasksToolAvailable: false,
          backgroundTaskJobsAvailable: false,
          backgroundShellAvailable: false,
          backgroundSubagentJobsAvailable: false
        } })
    }

    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx }))

    expect(html).toContain('Subagents')
    expect(html).toContain('unavailable')
    expect(html).toContain('internal lineage only')
    expect(html).not.toContain('profiles unavailable')
    expect(html).not.toContain('profile(s)')
    expect(html).not.toContain('1∥')
  })

  it('does not render private runtime profile records from public diagnostics', () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const ctx = {
      ...baseCtx(),
      runtimeInfo: {
        schemaVersion: 2,
        status: 'ready',
        capabilities: {
          model: { id: 'deepseek-chat' },
          mcp: { ...diagnosticState(true, false), configuredServers: 0, connectedServers: 0 },
          web: diagnosticState(false, false),
          skills: diagnosticState(true, true),
          subagents: {
            ...diagnosticState(true, true),
            defaultToolPolicy: 'readOnly',
            maxParallel: 8,
            maxChildRuns: 64,
            profileCount: 2,
            profiles: [
              {
                name: privateSentinel,
                model: privateSentinel,
                effort: privateSentinel,
                toolPolicy: 'readOnly'
              }
            ]
          },
          attachments: diagnosticState(true, true),
          memory: diagnosticState(true, true)
        }
      },
      toolDiagnostics: toolDiagnosticsV2()
    }

    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx }))

    expect(html).toContain('2 profile(s)')
    expect(html).not.toContain(privateSentinel)
  })

  it('renders editable sub-agent profile settings', () => {
    const ctx = {
      ...baseCtx(),
      analytix: {
        ...defaultAnalytixRuntimeSettings(),
        subagents: {
          enabled: true,
          maxParallel: 3,
          maxChildRuns: 12,
          defaultToolPolicy: 'readOnly',
          defaultProfile: 'reviewer',
          profiles: {
            reviewer: {
              prompt: 'Review this task.',
              model: 'deepseek-v4-pro',
              effort: 'high',
              toolPolicy: 'readOnly',
              tools: ['grep', 'read']
            }
          }
        }
      }
    }

    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx }))

    expect(html).toContain('Sub-agent profiles')
    expect(html).toContain('Add profile')
    expect(html).toContain('reviewer')
    expect(html).toContain('deepseek-v4-pro')
    expect(html).toContain('grep')
    expect(html).toContain('read')
    expect(html).toContain('Review this task.')
  })

  it('describes MCP config as an external-tool JSON file instead of model credentials', () => {
    const html = renderToStaticMarkup(createElement(AgentsSettingsSection, { ctx: baseCtx() }))

    expect(html).toContain('External tool config path')
    expect(html).toContain('/tmp/project/.analytix/mcp.json')
    expect(html).toContain('Model and API credentials do not live in this MCP file')
    expect(html).not.toContain('DeepSeek auth')
    expect(html).not.toContain('Base URL are stored in this file')
    expect(html).not.toContain('config.toml')
  })

  it('defines the LiteLLM provider preset for the Providers menu', () => {
    const litellm = getModelProviderPreset('litellm')
    expect(litellm && modelProviderPresetProfile(litellm)).toMatchObject({
      id: 'litellm',
      name: 'LiteLLM',
      baseUrl: 'http://localhost:4000',
      endpointFormat: 'chat_completions'
    })
  })

  it('defines coding provider presets for the Providers menu', () => {
    const expected = [
      ['zhipu-coding-plan', 'Zhipu Coding Plan', 'https://open.bigmodel.cn/api/coding/paas/v4/chat/completions', 'custom_endpoint'],
      ['zai-coding-plan', 'Z.ai Coding Plan', 'https://api.z.ai/api/coding/paas/v4/chat/completions', 'custom_endpoint'],
      ['kimi-code', 'Kimi Code', 'https://api.kimi.com/coding/v1'],
      ['moonshot-cn', 'Moonshot CN', 'https://api.moonshot.cn/v1'],
      ['moonshot-global', 'Moonshot Global', 'https://api.moonshot.ai/v1']
    ] as const

    for (const [id, name, baseUrl, endpointFormat = 'chat_completions'] of expected) {
      const preset = getModelProviderPreset(id)
      expect(preset && modelProviderPresetProfile(preset)).toMatchObject({
        id,
        name,
        baseUrl,
        endpointFormat
      })
    }
  })
})
