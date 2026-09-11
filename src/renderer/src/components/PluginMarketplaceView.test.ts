import { describe, expect, it } from 'vitest'
import type { SkillRootListItem } from '@shared/analytix-api'
import type { CoreRuntimeToolDiagnosticsJson } from '../agent/analytix-contract'
import {
  buildMcpConfig,
  customMcpConfigFragment,
  excludeHubManagedLocalMcpItems,
  hubManagedMcpServerIds,
  localMcpCommandSummary,
  marketplaceItemIsInstalled,
  mcpConfigHasServer,
  mcpMarketplaceItemsFromConfigAndDiagnostics,
  mergeMcpJsonConfig,
  recommendedMarketplaceItemIds,
  removeMcpServerFromConfig,
  setMcpServerEnabled,
  skillMarketplaceItemsFromDiscoveredSkills,
  skillRootOptionsFromRoots
} from './PluginMarketplaceView'

type PublicMcpServer = CoreRuntimeToolDiagnosticsJson['mcpServers'][number]

function publicMcpServer(id: string, status: PublicMcpServer['status'], toolCount = 0): PublicMcpServer {
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

function runtimeToolsWithServers(servers: PublicMcpServer[]): CoreRuntimeToolDiagnosticsJson {
  return {
    schemaVersion: 2,
    providerCount: 1,
    toolContracts: { count: 0, catalogHash: '0'.repeat(64) },
    mcpServers: servers,
    mcpSearch: {
      enabled: true,
      mode: 'auto',
      active: false,
      available: true,
      reasonCode: 'available',
      indexedToolCount: servers.reduce((sum, server) => sum + server.toolCount, 0),
      advertisedToolCount: servers.reduce((sum, server) => sum + server.toolCount, 0),
      autoThresholdToolCount: 24,
      topKDefault: 5,
      topKMax: 10,
      minScore: 0.15,
      catalogDrift: false
    },
    mcpPromptCount: 0,
    mcpResourceCount: 0,
    commands: [],
    networkProxy: { mode: 'auto', configured: false, source: 'unknown', valid: false, credentialsMasked: false },
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
      maxImageBytes: 1,
      maxImageDimension: 1,
      allowedMimeTypes: [],
      allowedDocumentMimeTypes: [],
      maxDocumentBytes: 1,
      maxDocumentTextChars: 1
    },
    memory: { enabled: true, activeCount: 0, tombstoneCount: 0 },
    subagents: {
      status: 'disabled',
      enabled: false,
      available: false,
      reasonCode: 'disabled_by_config',
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

describe('PluginMarketplaceView MCP config helpers', () => {
  it('does not recommend the filesystem MCP server because Analytix has built-in file tools', () => {
    expect(recommendedMarketplaceItemIds()).not.toContain('filesystem')
  })

  it('keeps Hub-managed MCP servers out of local desktop recommendations', () => {
    expect(recommendedMarketplaceItemIds()).not.toEqual(expect.arrayContaining([
      'playwright',
      'github',
      'context7',
      'sequential-thinking',
      'memory',
      'brave-search'
    ]))
    expect(recommendedMarketplaceItemIds()).toContain('gui_schedule')
  })

  it('filters Hub-managed MCP runtime diagnostics out of personal local MCP items', () => {
    const items = mcpMarketplaceItemsFromConfigAndDiagnostics(
      '{}',
      runtimeToolsWithServers([
        publicMcpServer('playwright', 'connected', 23),
        publicMcpServer('memory', 'connected', 9),
        publicMcpServer('team-docs', 'connected', 2)
      ]),
      {
        configured: 'Configured',
        connected: 'Connected',
        error: 'Error',
        disabled: 'Disabled'
      }
    )

    expect([...hubManagedMcpServerIds([
      { pluginName: 'playwright-mcp', mcpServerIds: ['playwright'] },
      { pluginName: 'memory', mcpServerIds: ['memory'] }
    ])]).toEqual(expect.arrayContaining(['playwright', 'memory']))
    expect(excludeHubManagedLocalMcpItems(items, [
      { pluginName: 'playwright-mcp', mcpServerIds: ['playwright'] },
      { pluginName: 'memory', mcpServerIds: ['memory'] }
    ]).map((item) => item.id)).toEqual(['team-docs'])
  })

  it('merges recommended MCP servers into JSON config without dropping existing fields', () => {
    const existing = JSON.stringify({
      timeouts: { read_timeout: 120 },
      servers: {
        gui_schedule: { command: '/Applications/analytix.app' }
      }
    })

    const merged = mergeMcpJsonConfig(
      existing,
      buildMcpConfig('playwright', 'npx', ['-y', '@playwright/mcp@latest'])
    )
    const parsed = JSON.parse(merged.text) as Record<string, any>

    expect(merged.alreadyExists).toBe(false)
    expect(parsed.timeouts).toEqual({ read_timeout: 120 })
    expect(parsed.servers.gui_schedule).toEqual({ command: '/Applications/analytix.app' })
    expect(parsed.servers.playwright).toMatchObject({
      enabled: true,
      transport: 'stdio',
      command: 'npx',
      args: ['-y', '@playwright/mcp@latest'],
      trustScope: 'user'
    })
    expect(mcpConfigHasServer(merged.text, 'playwright')).toBe(true)
  })

  it('detects duplicate MCP servers instead of appending old-style snippets', () => {
    const fragment = buildMcpConfig('context7', 'npx', ['-y', '@upstash/context7-mcp@latest'])
    const first = mergeMcpJsonConfig('', fragment)
    const second = mergeMcpJsonConfig(first.text, fragment)

    expect(first.alreadyExists).toBe(false)
    expect(second.alreadyExists).toBe(true)
    expect(JSON.parse(second.text).servers.context7).toMatchObject({ command: 'npx' })
  })

  it('summarizes recommended MCP launch commands for detail pages', () => {
    expect(localMcpCommandSummary({
      id: 'context7',
      kind: 'mcp',
      mcpConfig: () => buildMcpConfig('context7', 'npx', ['-y', '@upstash/context7-mcp@latest'])
    })).toBe('npx -y @upstash/context7-mcp@latest')
  })

  it('summarizes configured MCP launch commands for personal detail pages', () => {
    expect(localMcpCommandSummary({
      id: 'docs',
      kind: 'mcp'
    }, {
      transport: 'stdio',
      command: 'docs-mcp',
      args: ['--port', 1234]
    })).toBe('docs-mcp --port 1234')
  })

  it('redacts secret-like MCP launch command arguments', () => {
    const summary = localMcpCommandSummary({
      id: 'docs',
      kind: 'mcp'
    }, {
      transport: 'stdio',
      command: 'docs-mcp',
      args: ['--header', 'Authorization: Bearer local-secret', 'token=arg-secret']
    })

    expect(summary).toContain('<redacted>')
    expect(summary).not.toContain('local-secret')
    expect(summary).not.toContain('arg-secret')
  })

  it('accepts custom JSON as either a single server or a Analytix config fragment', () => {
    expect(customMcpConfigFragment(
      'docs',
      '{"transport":"stdio","command":"npx","args":["-y","docs-mcp"]}',
      {}
    )).toEqual({
      servers: {
        docs: {
          transport: 'stdio',
          command: 'npx',
          args: ['-y', 'docs-mcp']
        }
      }
    })

    expect(customMcpConfigFragment(
      'github',
      '{"capabilities":{"mcp":{"servers":{"github":{"transport":"stdio","command":"github-mcp"}}}}}',
      {}
    )).toEqual({
      servers: {
        github: {
          transport: 'stdio',
          command: 'github-mcp'
        }
      }
    })
  })

  it('detects MCP servers from full Analytix capability config', () => {
    const content = JSON.stringify({
      capabilities: {
        mcp: {
          servers: {
            github: {
              transport: 'stdio',
              command: 'github-mcp'
            }
          }
        }
      }
    })

    expect(mcpConfigHasServer(content, 'github')).toBe(true)
  })

  it('turns configured MCP servers into personal marketplace items', () => {
    const items = mcpMarketplaceItemsFromConfigAndDiagnostics(
      '{"servers":{"docs":{"transport":"stdio","command":"docs-mcp"}}}',
      null,
      {
        configured: 'Configured',
        connected: 'Connected',
        error: 'Error',
        disabled: 'Disabled'
      }
    )

    expect(items).toEqual([
      expect.objectContaining({
        id: 'docs',
        kind: 'mcp',
        group: 'personal',
        title: 'docs',
        description: expect.stringContaining('docs-mcp'),
        sourceLabel: 'Configured',
        statusTone: 'default',
        mcpConfigured: true,
        mcpRuntimeSeen: false
      })
    ])
  })

  it('marks runtime-only MCP diagnostics as visible but not configured', () => {
    const items = mcpMarketplaceItemsFromConfigAndDiagnostics(
      '{}',
      runtimeToolsWithServers([publicMcpServer('context7', 'connected', 8)]),
      {
        configured: 'Configured',
        connected: 'Connected',
        error: 'Error',
        disabled: 'Disabled'
      }
    )

    expect(items).toEqual([
      expect.objectContaining({
        id: 'context7',
        group: 'personal',
        sourceLabel: 'Connected',
        mcpConfigured: false,
        mcpRuntimeSeen: true
      })
    ])
  })

  it('uses actual MCP config, not stale installed storage or runtime-only diagnostics, for install state', () => {
    const state = {
      configuredMcpIds: new Set<string>(),
      discoveredSkillIds: new Set<string>(),
      installedIds: ['mcp:context7'],
      mcpConfigText: '{}'
    }

    expect(marketplaceItemIsInstalled({
      id: 'context7',
      kind: 'mcp',
      group: 'recommended'
    }, state)).toBe(false)
    expect(marketplaceItemIsInstalled({
      id: 'context7',
      kind: 'mcp',
      group: 'personal'
    }, state)).toBe(false)
    expect(marketplaceItemIsInstalled({
      id: 'context7',
      kind: 'mcp',
      group: 'recommended'
    }, {
      ...state,
      configuredMcpIds: new Set(['context7'])
    })).toBe(true)
  })

  it('overlays MCP runtime diagnostics onto configured marketplace items', () => {
    const items = mcpMarketplaceItemsFromConfigAndDiagnostics(
      JSON.stringify({
        servers: {
          github: {
            transport: 'stdio',
            command: 'github-mcp'
          },
          disabled_docs: {
            transport: 'stdio',
            command: 'docs-mcp',
            enabled: false
          }
        }
      }),
      runtimeToolsWithServers([
        publicMcpServer('github', 'connected', 12),
        publicMcpServer('bad', 'error')
      ]),
      {
        configured: 'Configured',
        connected: 'Connected',
        error: 'Error',
        disabled: 'Disabled'
      }
    )

    expect(items).toEqual([
      expect.objectContaining({
        id: 'bad',
        sourceLabel: 'Error',
        statusTone: 'error',
        description: expect.stringContaining('connection failed')
      }),
      expect.objectContaining({
        id: 'disabled_docs',
        sourceLabel: 'Disabled',
        statusTone: 'warning'
      }),
      expect.objectContaining({
        id: 'github',
        sourceLabel: 'Connected',
        statusTone: 'success',
        title: 'github',
        description: expect.stringContaining('github-mcp'),
        detail: expect.stringContaining('github-mcp')
      })
    ])
  })

  it('renders only the fixed MCP failure code in marketplace item details', () => {
    const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'
    const items = mcpMarketplaceItemsFromConfigAndDiagnostics(
      JSON.stringify({
        servers: {
          bad: {
            transport: 'stdio',
            command: 'bad-mcp',
            lastError: privateSentinel
          }
        }
      }),
      runtimeToolsWithServers([publicMcpServer('bad', 'error')]),
      {
        configured: 'Configured',
        connected: 'Connected',
        error: 'Error',
        disabled: 'Disabled'
      }
    )

    expect(items[0]?.detail).toContain('connection failed')
    expect(items[0]?.detail).not.toContain('lastError')
    expect(JSON.stringify(items[0])).not.toContain(privateSentinel)
  })

  it('toggles top-level MCP servers without dropping config fields', () => {
    const disabled = setMcpServerEnabled(JSON.stringify({
      timeouts: { read_timeout: 120 },
      servers: {
        docs: {
          transport: 'stdio',
          command: 'docs-mcp',
          args: ['--stdio']
        }
      }
    }), 'docs', false)
    const disabledParsed = JSON.parse(disabled) as Record<string, any>

    expect(disabledParsed.timeouts).toEqual({ read_timeout: 120 })
    expect(disabledParsed.servers.docs).toMatchObject({
      transport: 'stdio',
      command: 'docs-mcp',
      args: ['--stdio'],
      enabled: false
    })

    const enabled = setMcpServerEnabled(disabled, 'docs', true)
    const enabledParsed = JSON.parse(enabled) as Record<string, any>
    expect(enabledParsed.servers.docs.enabled).toBe(true)
    expect(enabledParsed.servers.docs.command).toBe('docs-mcp')
  })

  it('toggles nested Analytix capability MCP servers', () => {
    const text = setMcpServerEnabled(JSON.stringify({
      capabilities: {
        mcp: {
          enabled: true,
          servers: {
            github: {
              transport: 'stdio',
              command: 'github-mcp',
              disabled: true
            }
          }
        }
      }
    }), 'github', true)
    const parsed = JSON.parse(text) as Record<string, any>

    expect(parsed.capabilities.mcp.enabled).toBe(true)
    expect(parsed.capabilities.mcp.servers.github).toMatchObject({
      transport: 'stdio',
      command: 'github-mcp',
      enabled: true
    })
    expect(parsed.capabilities.mcp.servers.github).not.toHaveProperty('disabled')
  })

  it('removes MCP servers from top-level and nested Analytix configs', () => {
    const topLevel = removeMcpServerFromConfig(JSON.stringify({
      servers: {
        github: { command: 'github-mcp' },
        docs: { command: 'docs-mcp' }
      }
    }), 'github')
    expect(JSON.parse(topLevel).servers).toEqual({
      docs: { command: 'docs-mcp' }
    })

    const nested = removeMcpServerFromConfig(JSON.stringify({
      capabilities: {
        mcp: {
          enabled: true,
          servers: {
            memory: { command: 'memory-mcp' },
            docs: { command: 'docs-mcp' }
          }
        }
      }
    }), 'memory')
    expect(JSON.parse(nested).capabilities.mcp.servers).toEqual({
      docs: { command: 'docs-mcp' }
    })
  })

  it('keeps MCP removal idempotent when a server is already absent', () => {
    const topLevel = JSON.stringify({
      servers: {
        github: { command: 'github-mcp' }
      }
    })
    expect(JSON.parse(removeMcpServerFromConfig(topLevel, 'context7'))).toEqual(JSON.parse(topLevel))

    const noServers = JSON.stringify({ capabilities: { mcp: { enabled: true } } })
    expect(removeMcpServerFromConfig(noServers, 'context7')).toBe(noServers)
  })
})

describe('skillMarketplaceItemsFromDiscoveredSkills', () => {
  it('turns discovered project and global skills into personal marketplace items', () => {
    const items = skillMarketplaceItemsFromDiscoveredSkills([
      {
        id: 'openspec-apply-change',
        name: 'Openspec Apply Change',
        description: 'Implement tasks from an OpenSpec change.',
        root: '/workspace/.codex/skills/openspec-apply-change',
        entryPath: '/workspace/.codex/skills/openspec-apply-change/SKILL.md',
        scope: 'project',
        legacy: true
      },
      {
        id: 'remotion-best-practices',
        name: 'Remotion Best Practices',
        description: 'Best practices for Remotion.',
        root: '/Users/demo/.agents/skills/remotion-best-practices',
        entryPath: '/Users/demo/.agents/skills/remotion-best-practices/SKILL.md',
        scope: 'global',
        legacy: true
      }
    ], { project: 'Project', global: 'Global' })

    expect(items).toEqual([
      expect.objectContaining({
        id: 'openspec-apply-change',
        group: 'personal',
        title: 'Openspec Apply Change',
        sourceLabel: 'Project',
        detail: '/workspace/.codex/skills/openspec-apply-change/SKILL.md',
        skillRootPath: '/workspace/.codex/skills/openspec-apply-change',
        skillEntryPath: '/workspace/.codex/skills/openspec-apply-change/SKILL.md'
      }),
      expect.objectContaining({
        id: 'remotion-best-practices',
        group: 'personal',
        title: 'Remotion Best Practices',
        sourceLabel: 'Global',
        detail: '/Users/demo/.agents/skills/remotion-best-practices/SKILL.md',
        skillRootPath: '/Users/demo/.agents/skills/remotion-best-practices',
        skillEntryPath: '/Users/demo/.agents/skills/remotion-best-practices/SKILL.md'
      })
    ])
  })
})

describe('skillRootOptionsFromRoots', () => {
  const roots: SkillRootListItem[] = [
    {
      id: 'workspace-claude',
      disableKey: 'workspace-claude',
      path: '/ws/.claude/skills',
      scope: 'project',
      source: 'common',
      labelKey: 'pluginSkillRootWorkspaceClaude',
      exists: true,
      enabled: true,
      skillCount: 2
    },
    {
      id: 'global-codex',
      disableKey: 'global-codex',
      path: '/home/me/.codex/skills',
      scope: 'global',
      source: 'common',
      labelKey: 'pluginSkillRootGlobalCodex',
      exists: false,
      enabled: false,
      skillCount: 0
    },
    {
      id: '/opt/team/skills',
      disableKey: '/opt/team/skills',
      path: '/opt/team/skills',
      scope: 'global',
      source: 'extra',
      exists: true,
      enabled: true,
      skillCount: 5
    }
  ]

  it('maps backend roots — common (.claude/.codex) and custom dirs — into picker options synced with settings', () => {
    const options = skillRootOptionsFromRoots(roots, (key) => `t:${key}`)

    expect(options).toEqual([
      { id: 'workspace-claude', label: 't:pluginSkillRootWorkspaceClaude', path: '/ws/.claude/skills', scope: 'project', enabled: true, exists: true, skillCount: 2 },
      { id: 'global-codex', label: 't:pluginSkillRootGlobalCodex', path: '/home/me/.codex/skills', scope: 'global', enabled: false, exists: false, skillCount: 0 },
      // Custom extra dir has no i18n labelKey, so it falls back to a short path label.
      { id: '/opt/team/skills', label: 'team/skills', path: '/opt/team/skills', scope: 'global', enabled: true, exists: true, skillCount: 5 }
    ])
  })

  it('returns an empty list when the backend reports no roots', () => {
    expect(skillRootOptionsFromRoots([], (key) => key)).toEqual([])
  })
})
