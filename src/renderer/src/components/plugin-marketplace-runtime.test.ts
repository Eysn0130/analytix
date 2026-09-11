import { describe, expect, it } from 'vitest'

import type {
  CoreRuntimeInfoJson,
  CoreRuntimeToolDiagnosticsJson
} from '../agent/analytix-contract'
import { RuntimeToolsResponse } from '../../../../packages/runtime/src/contracts/runtime-tools.js'
import { buildMcpMarketplaceOverlay } from './plugin-marketplace-runtime'

type PublicMcpServer = CoreRuntimeToolDiagnosticsJson['mcpServers'][number]

function capability(enabled: boolean, available = enabled) {
  return {
    status: available ? 'available' as const : enabled ? 'unavailable' as const : 'disabled' as const,
    enabled,
    available,
    reasonCode: available ? 'available' as const : enabled ? 'unavailable' as const : 'disabled_by_config' as const
  }
}

function mcpSearch(overrides: Partial<CoreRuntimeToolDiagnosticsJson['mcpSearch']> = {}) {
  return {
    enabled: true,
    mode: 'auto' as const,
    active: false,
    available: true,
    reasonCode: 'available' as const,
    indexedToolCount: 0,
    advertisedToolCount: 0,
    autoThresholdToolCount: 24,
    topKDefault: 5,
    topKMax: 10,
    minScore: 0.15,
    catalogDrift: false,
    ...overrides
  }
}

function mcpServer(
  id: string,
  status: PublicMcpServer['status'],
  toolCount = 0
): PublicMcpServer {
  const connected = status === 'connected'
  return {
    id,
    status,
    ...(status === 'error' ? { failureCode: 'connection_failed' as const } : {}),
    transport: 'stdio',
    authStatus: 'none',
    trustScope: 'user',
    enabled: true,
    available: connected,
    connected,
    schemaHintAvailable: false,
    connectable: true,
    toolCount,
    promptCount: 0,
    resourceCount: 0,
    toolContractQuarantineCount: 0,
    lowPriority: false,
    backgroundStart: false
  }
}

function runtimeTools(input: {
  servers?: PublicMcpServer[]
  search?: Partial<CoreRuntimeToolDiagnosticsJson['mcpSearch']>
} = {}): CoreRuntimeToolDiagnosticsJson {
  return {
    schemaVersion: 2,
    providerCount: 1,
    toolContracts: { count: 0, catalogHash: '0'.repeat(64) },
    mcpServers: input.servers ?? [],
    mcpSearch: mcpSearch(input.search),
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: {
      mode: 'auto',
      configured: false,
      source: 'unknown',
      valid: false,
      credentialsMasked: false
    },
    webProviderCount: 0,
    skills: {
      enabled: false,
      available: false,
      reasonCode: 'disabled_by_config',
      configuredRootCount: 0,
      skillCount: 0,
      validationErrorCount: 0
    },
    attachments: {
      enabled: true,
      count: 0,
      totalBytes: 0,
      maxImageBytes: 5_242_880,
      maxImageDimension: 4096,
      allowedMimeTypes: ['image/png'],
      allowedDocumentMimeTypes: ['application/pdf'],
      maxDocumentBytes: 10_485_760,
      maxDocumentTextChars: 200_000
    },
    memory: { enabled: true, activeCount: 0, tombstoneCount: 0 },
    subagents: {
      ...capability(false),
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
    }
  }
}

function runtimeInfo(input: {
  enabled?: boolean
  available?: boolean
  configuredServers?: number
  connectedServers?: number
  toolCount?: number
  search?: Partial<CoreRuntimeInfoJson['capabilities']['mcp']['search']>
} = {}): CoreRuntimeInfoJson {
  const enabled = input.enabled ?? true
  const available = input.available ?? enabled
  const off = capability(false)
  return {
    schemaVersion: 2,
    status: 'ready',
    listenerScope: 'loopback',
    port: 8899,
    startedAt: '2026-06-03T00:00:00.000Z',
    insecure: false,
    storage: { configured: true, available: true },
    executionPolicy: { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' },
    provider: {
      id: 'deepseek',
      model: 'deepseek-chat',
      family: 'deepseek',
      endpointFormat: 'chat_completions',
      available: true,
      apiKeyConfigured: true,
      baseUrlConfigured: true,
      cacheTelemetrySupported: true,
      supportsImageInput: false
    },
    networkProxy: {
      mode: 'auto',
      configured: false,
      source: 'unknown',
      valid: false,
      credentialsMasked: false
    },
    capabilities: {
      contractVersion: 1,
      model: {
        id: 'deepseek-chat',
        inputModalities: ['text'],
        outputModalities: ['text'],
        supportsToolCalling: true,
        supportsImageInput: false,
        messageParts: ['text']
      },
      cli: { serve: capability(true), run: off, chat: off, exec: off },
      mcp: {
        ...capability(enabled, available),
        configuredServers: input.configuredServers ?? 0,
        connectedServers: input.connectedServers ?? 0,
        toolCount: input.toolCount ?? 0,
        promptCount: 0,
        resourceCount: 0,
        catalog: {
          status: available ? 'available' : enabled ? 'unavailable' : 'disabled',
          reasonCode: available ? 'available' : enabled ? 'unavailable' : 'disabled_by_config',
          toolCount: input.toolCount ?? 0,
          advertisedToolCount: 0,
          promptCount: 0,
          resourceCount: 0,
          catalogDrift: false
        },
        search: mcpSearch(input.search)
      },
      web: { ...off, fetch: off, search: off, provider: 'unknown', fetchEnabled: false, searchEnabled: false },
      skills: { ...off, configuredRoots: 0, discoveredSkills: 0 },
      subagents: {
        ...off,
        maxParallel: 0,
        maxChildRuns: 0,
        defaultToolPolicy: 'readOnly',
        profileCount: 0,
        internalLineageAvailable: false,
        profilesAvailable: false,
        durableChildRunStore: false,
        parallelExecutionAvailable: false,
        taskToolAvailable: false,
        parallelTasksToolAvailable: false,
        backgroundTaskJobsAvailable: false,
        backgroundShellAvailable: false,
        backgroundSubagentJobsAvailable: false,
        modelJobToolsAvailable: false,
        taskJobThreadScopeSupported: false
      },
      attachments: {
        ...off,
        maxImageBytes: 1,
        maxImageDimension: 1,
        allowedMimeTypes: [],
        allowedDocumentMimeTypes: [],
        maxDocumentBytes: 1,
        maxDocumentTextChars: 1,
        textFallbackMaxBase64Bytes: 1,
        textFallbackMaxImageDimension: 1,
        textFallbackPreferredMimeType: 'image/png'
      },
      memory: {
        ...off,
        mode: 'manual',
        storeOnly: true,
        modelInjection: false,
        automaticCapture: false,
        scopes: ['user'],
        maxInjectedRecords: 0
      },
      imageGen: off,
      speechGen: off,
      musicGen: off,
      videoGen: off,
      computerUse: { ...off, mode: 'off' },
      visionBridge: {
        ...off,
        mode: 'off',
        maxScreenshotsPerTurn: 0,
        maxImagesPerTurn: 0,
        maxImageBytes: 0,
        maxImageDimension: 0,
        semanticProbeStatus: 'unknown'
      }
    }
  }
}

describe('buildMcpMarketplaceOverlay', () => {
  it('summarizes connected MCP runtime state', () => {
    const overlay = buildMcpMarketplaceOverlay({
      runtimeInfo: runtimeInfo({ configuredServers: 2, connectedServers: 1, toolCount: 12 }),
      toolDiagnostics: runtimeTools({
        servers: [mcpServer('github', 'connected', 12), mcpServer('local', 'configured')],
        search: { active: true, indexedToolCount: 12, advertisedToolCount: 3 }
      })
    })

    expect(overlay).toMatchObject({
      status: 'connected',
      configuredServers: 2,
      connectedServers: 1,
      toolCount: 12,
      serverIds: ['github', 'local'],
      searchActive: true,
      indexedToolCount: 12,
      advertisedToolCount: 3
    })
  })

  it('uses fixed failure codes and search catalog drift only', () => {
    expect(buildMcpMarketplaceOverlay({
      toolDiagnostics: runtimeTools({ servers: [mcpServer('bad', 'error')] })
    })).toMatchObject({
      status: 'error',
      errorCount: 1,
      failureCode: 'connection_failed'
    })

    expect(buildMcpMarketplaceOverlay({
      toolDiagnostics: runtimeTools({
        servers: [mcpServer('docs', 'connected', 5)],
        search: { catalogDrift: true }
      })
    })).toMatchObject({ status: 'drift', driftCount: 1 })
  })

  it('rejects raw errors and internal diagnostic markers at the v2 contract boundary', () => {
    const valid = runtimeTools({ servers: [mcpServer('bad', 'error')] })
    for (const server of [
      { ...valid.mcpServers[0], lastError: 'Authorization: Bearer PRIVATE' },
      { ...valid.mcpServers[0], localValidationOnly: true },
      { ...valid.mcpServers[0], catalogDrift: true }
    ]) {
      expect(RuntimeToolsResponse.safeParse({ ...valid, mcpServers: [server] }).success).toBe(false)
    }
  })

  it('keeps the MCP runtime healthy when one server fails but tools remain available', () => {
    const servers = [
      mcpServer('gui_schedule', 'connected', 4),
      mcpServer('sequential-thinking', 'connected', 1),
      mcpServer('playwright', 'connected', 23),
      mcpServer('github', 'connected', 12),
      mcpServer('context7', 'connected', 8),
      mcpServer('memory', 'connected', 21),
      mcpServer('brave-search', 'error')
    ]
    const overlay = buildMcpMarketplaceOverlay({
      toolDiagnostics: runtimeTools({
        servers,
        search: { indexedToolCount: 69, advertisedToolCount: 69 }
      })
    })

    expect(overlay).toMatchObject({
      status: 'connected',
      configuredServers: 7,
      connectedServers: 6,
      toolCount: 69,
      errorCount: 1,
      indexedToolCount: 69,
      advertisedToolCount: 69
    })
    expect(overlay.failureCode).toBeUndefined()
  })

  it('reports disabled and offline states', () => {
    expect(buildMcpMarketplaceOverlay({
      runtimeInfo: runtimeInfo({ enabled: false, available: false })
    }).status).toBe('disabled')
    expect(buildMcpMarketplaceOverlay({}).status).toBe('offline')
  })

  it('includes GUI-managed MCP servers before runtime diagnostics connect', () => {
    expect(buildMcpMarketplaceOverlay({
      managedServers: [{ id: 'gui_schedule', toolCount: 4 }]
    })).toMatchObject({
      status: 'offline',
      configuredServers: 1,
      toolCount: 4,
      serverIds: ['gui_schedule']
    })
  })
})
