import { describe, expect, it } from 'vitest'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { LocalToolHost } from '../src/tool-test-support/tool/local-tool-host.js'
import {
  FileMcpToolSchemaCache,
  buildMcpStdioEnvironment,
  buildMcpToolProviders,
  formatMcpConnectionError,
  isMcpServerTrusted,
  normalizeMcpToolName,
  type McpClientLike
} from '../src/tool-test-support/tool/mcp-tool-provider.js'
import { REDACTED_SECRET } from '../src/config/secret-redaction.js'
import { AnalytixCapabilitiesConfig, type McpServerConfig } from '../src/contracts/capabilities.js'
import type { ToolHostContext } from '../src/ports/tool-host.js'

function buildContext(workspace: string): ToolHostContext {
  return {
    threadId: 'thr_1',
    turnId: 'turn_1',
    workspace,
    threadMode: 'agent',
    approvalPolicy: 'auto',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow'
  }
}

function fakeClient(): McpClientLike {
  return {
    async listTools() {
      return {
        tools: [
          {
            name: 'Search Issues',
            description: 'Search issue tracker',
            inputSchema: {
              type: 'object',
              properties: { query: { type: 'string' } },
              required: ['query']
            },
            annotations: { readOnlyHint: true }
          }
        ]
      }
    },
    async callTool(input) {
      return {
        content: [{ type: 'text', text: `called ${input.name}` }],
        structuredContent: input.arguments
      }
    },
    async close() {
      // no-op
    }
  }
}

describe('MCP tool provider', () => {
  it('normalizes stable MCP tool names', () => {
    expect(normalizeMcpToolName('GitHub Server', 'Search Issues')).toBe('mcp_github_server_search_issues')
  })

  it('adds common GUI app command paths to stdio MCP environments', () => {
    const env = buildMcpStdioEnvironment({ NODE_ENV: 'test' }, {
      platform: 'darwin',
      baseEnv: {
        PATH: '/usr/bin:/opt/homebrew/bin',
        HOME: '/Users/alice'
      }
    })

    expect(env.NODE_ENV).toBe('test')
    expect(env.PATH?.split(':')).toEqual([
      '/usr/bin',
      '/opt/homebrew/bin',
      '/usr/local/bin',
      '/opt/local/bin',
      '/Users/alice/.volta/bin',
      '/Users/alice/.local/node/current/bin',
      '/Users/alice/.local/bin',
      '/Users/alice/.bun/bin'
    ])
  })

  it('keeps explicitly configured stdio MCP PATH values ahead of common paths', () => {
    const env = buildMcpStdioEnvironment({ Path: 'C:\\Tools' }, {
      platform: 'win32',
      baseEnv: {
        APPDATA: 'C:\\Users\\alice\\AppData\\Roaming',
        ProgramFiles: 'C:\\Program Files',
        PATH: 'C:\\Windows\\System32'
      }
    })

    expect(env.Path?.split(';')).toEqual([
      'C:\\Tools',
      'C:\\Users\\alice\\AppData\\Roaming\\npm',
      'C:\\Program Files\\nodejs'
    ])
  })

  it('formats missing stdio MCP commands with an actionable PATH hint', () => {
    const server = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          filesystem: {
            transport: 'stdio',
            command: 'npx',
            trustScope: 'user'
          }
        }
      }
    }).mcp.servers.filesystem
    const error = Object.assign(new Error('spawn npx ENOENT'), {
      code: 'ENOENT',
      path: 'npx'
    })

    expect(formatMcpConnectionError(error, server)).toContain('Could not find "npx" on PATH')
  })

  it('evaluates workspace trust scopes', () => {
    const server = {
      enabled: true,
      transport: 'stdio',
      command: 'node',
      args: [],
      url: undefined,
      headers: {},
      env: {},
      trustScope: 'workspace',
      trustedWorkspaceRoots: ['/tmp/project'],
      lowPriority: false,
      backgroundStart: false,
      timeoutMs: 30_000
    } satisfies McpServerConfig

    expect(isMcpServerTrusted(server, '/tmp/project')).toBe(true)
    expect(isMcpServerTrusted(server, '/tmp/project/sub')).toBe(true)
    expect(isMcpServerTrusted(server, '/tmp/other')).toBe(false)
  })

  it('applies codebase-memory and codegraph known overrides without leaking secrets', async () => {
    const captured = new Map<string, McpServerConfig>()
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          codegraph: {
            transport: 'stdio',
            command: 'npx',
            args: ['-y', '@analytix/codegraph'],
            env: { Authorization: 'Bearer server-secret' },
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          },
          'codebase-memory': {
            transport: 'stdio',
            command: 'node',
            args: ['codebase-memory-server.js'],
            cwd: '/tmp/custom-memory',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })

    const built = await buildMcpToolProviders(config.mcp, {
      workspaceRoot: '/tmp/project',
      clientFactory: async (serverId, server) => {
        captured.set(serverId, server)
        return fakeClient()
      }
    })

    expect(captured.get('codegraph')).toMatchObject({
      cwd: '/tmp/project',
      lowPriority: true,
      backgroundStart: true,
      env: expect.objectContaining({ CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS: '5000' })
    })
    expect(captured.get('codebase-memory')).toMatchObject({
      cwd: '/tmp/custom-memory',
      lowPriority: true,
      backgroundStart: true
    })
    expect(built.diagnostics).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'codebase-memory',
        knownOverride: 'codebase-memory',
        effectiveCwd: '/tmp/custom-memory',
        lowPriority: true,
        backgroundStart: true
      }),
      expect.objectContaining({
        id: 'codegraph',
        knownOverride: 'codegraph',
        effectiveCwd: '/tmp/project',
        lowPriority: true,
        backgroundStart: true
      })
    ]))
    expect(JSON.stringify(built.diagnostics)).not.toContain('server-secret')
  })

  it('builds registry providers from connected MCP clients and executes tools', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => fakeClient()
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })

    expect(built.connectedServers).toBe(1)
    expect(built.toolCount).toBe(1)
    expect(built.diagnostics[0]).toMatchObject({ id: 'github', status: 'connected', toolCount: 1 })

    const tools = await host.listTools(buildContext('/tmp/project'))
    expect(tools.map((tool) => tool.name)).toEqual(['mcp_github_search_issues'])
    expect(tools[0]?.providerId).toBe('mcp:github')

    const result = await host.execute({
      callId: 'call_1',
      toolName: 'mcp_github_search_issues',
      arguments: { query: 'bug' }
    }, buildContext('/tmp/project'))
    expect(result.item.kind).toBe('tool_result')
    if (result.item.kind === 'tool_result') {
      expect(result.item.output).toMatchObject({
        serverId: 'github',
        toolName: 'Search Issues'
      })
    }
  })

  it('uses cached MCP schemas after startup failures and lazily reconnects for calls', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'analytix-mcp-schema-cache-'))
    try {
      const schemaCache = new FileMcpToolSchemaCache(
        join(dir, 'tool-schema-cache.json'),
        () => '2026-06-03T00:00:00.000Z'
      )
      const config = AnalytixCapabilitiesConfig.parse({
        mcp: {
          enabled: true,
          servers: {
            github: {
              transport: 'stdio',
              command: 'node',
              trustScope: 'workspace',
              trustedWorkspaceRoots: ['/tmp/project']
            }
          }
        }
      })

      await buildMcpToolProviders(config.mcp, {
        schemaCache,
        clientFactory: async () => ({
          async listTools() {
            return {
              tools: [{
                name: 'Cached Lookup',
                description: 'Cached schema fixture',
                inputSchema: {
                  type: 'object',
                  properties: {
                    b: { type: 'string' },
                    a: { type: 'number' }
                  },
                  required: ['a'],
                  additionalProperties: false
                },
                annotations: { readOnlyHint: true }
              }]
            }
          },
          async callTool(input) {
            return { initial: true, name: input.name }
          },
          async close() {
            // no-op
          }
        })
      })

      let factoryAttempts = 0
      const cached = await buildMcpToolProviders(config.mcp, {
        schemaCache,
        clientFactory: async () => {
          factoryAttempts += 1
          if (factoryAttempts === 1) {
            throw new Error('transport failed token=sk-cached-secret')
          }
          return {
            async listTools() {
              return { tools: [] }
            },
            async callTool(input) {
              return { reconnected: true, name: input.name, arguments: input.arguments }
            },
            async close() {
              // no-op
            }
          }
        }
      })

      expect(cached.connectedServers).toBe(1)
      expect(cached.toolCount).toBe(1)
      expect(cached.diagnostics[0]).toMatchObject({
        id: 'github',
        status: 'connected',
        available: true,
        toolCount: 1,
        lastError: expect.stringContaining(REDACTED_SECRET)
      })
      expect(JSON.stringify(cached.diagnostics)).not.toContain('sk-cached-secret')

      const host = new LocalToolHost({ registry: new CapabilityRegistry(cached.providers) })
      const tools = await host.listTools(buildContext('/tmp/project'))
      expect(tools.map((tool) => tool.name)).toEqual(['mcp_github_cached_lookup'])
      expect(tools[0]?.inputSchema).toMatchObject({
        type: 'object',
        required: ['a'],
        additionalProperties: false
      })

      const result = await host.execute({
        callId: 'call_cached',
        toolName: 'mcp_github_cached_lookup',
        arguments: { a: 1 }
      }, buildContext('/tmp/project'))
      expect(factoryAttempts).toBe(2)
      expect(result.item.kind === 'tool_result' ? result.item.output : {}).toMatchObject({
        result: { reconnected: true, name: 'Cached Lookup', arguments: { a: 1 } }
      })
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('keeps remote MCP auth diagnostics safe when cached schemas keep tools available', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'analytix-mcp-auth-cache-'))
    try {
      const schemaCache = new FileMcpToolSchemaCache(
        join(dir, 'tool-schema-cache.json'),
        () => '2026-06-03T00:00:00.000Z'
      )
      const config = AnalytixCapabilitiesConfig.parse({
        mcp: {
          enabled: true,
          servers: {
            docs: {
              transport: 'streamable-http',
              url: 'https://mcp.example.test/mcp?access_token=secret-token&workspace=main',
              trustScope: 'user'
            }
          }
        }
      })

      await buildMcpToolProviders(config.mcp, {
        schemaCache,
        clientFactory: async () => ({
          async listTools() {
            return {
              tools: [{
                name: 'lookup',
                inputSchema: { type: 'object' },
                annotations: { readOnlyHint: true }
              }]
            }
          },
          async callTool() {
            return { ok: true }
          },
          async close() {
            // no-op
          }
        })
      })

      const cached = await buildMcpToolProviders(config.mcp, {
        schemaCache,
        clientFactory: async () => {
          throw new Error('401 unauthorized token=secret-token')
        }
      })

      expect(cached.connectedServers).toBe(1)
      expect(cached.diagnostics[0]).toMatchObject({
        id: 'docs',
        status: 'connected',
        authStatus: 'required',
        authUrl: 'https://mcp.example.test/mcp?workspace=main',
        lastError: expect.stringContaining(REDACTED_SECRET)
      })
      expect(JSON.stringify(cached.diagnostics)).not.toContain('secret-token')
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('normalizes malformed MCP tool schemas before advertising tools', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => ({
        async listTools() {
          return {
            tools: [
              {
                name: 'bad_schema',
                description: 'Malformed schema fixture',
                inputSchema: ['not', 'an', 'object'] as unknown as Record<string, unknown>,
                annotations: { readOnlyHint: true }
              },
              {
                name: 'bad_required',
                description: 'Malformed required fixture',
                inputSchema: {
                  type: 'object',
                  properties: ['bad'],
                  required: ['query', 1, false]
                } as unknown as Record<string, unknown>,
                annotations: { readOnlyHint: true }
              }
            ]
          }
        },
        async callTool(input) {
          return { called: input.name, arguments: input.arguments }
        },
        async close() {
          // no-op
        }
      })
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    const tools = await host.listTools(buildContext('/tmp/project'))

    expect(tools.find((tool) => tool.name === 'mcp_github_bad_schema')?.inputSchema).toEqual({
      type: 'object',
      properties: {},
      additionalProperties: true
    })
    expect(tools.find((tool) => tool.name === 'mcp_github_bad_required')?.inputSchema).toEqual({
      type: 'object',
      required: ['query']
    })
  })

  it('uses BM25 MCP search meta tools when search discovery is enabled', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        search: {
          enabled: true,
          mode: 'search',
          topKDefault: 2,
          topKMax: 5
        },
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    let callCount = 0
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => ({
        async listTools() {
          return {
            tools: [
              {
                name: 'search_issues',
                title: 'Search issues',
                description: 'Search GitHub issues and pull requests by query',
                inputSchema: {
                  type: 'object',
                  properties: { query: { type: 'string', description: 'Issue search query' } },
                  required: ['query']
                },
                outputSchema: 'not an object' as unknown as Record<string, unknown>,
                annotations: { readOnlyHint: true }
              },
              {
                name: 'create_issue',
                description: 'Create a GitHub issue',
                inputSchema: {
                  type: 'object',
                  properties: { title: { type: 'string' }, body: { type: 'string' } },
                  required: ['title']
                }
              }
            ]
          }
        },
        async callTool(input) {
          callCount += 1
          return { called: input.name, arguments: input.arguments }
        },
        async close() {
          // no-op
        }
      })
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    const context = buildContext('/tmp/project')

    expect(built.toolCount).toBe(2)
    expect(built.search).toMatchObject({
      enabled: true,
      mode: 'search',
      active: true,
      indexedToolCount: 2,
      advertisedToolCount: 4
    })
    expect((await host.listTools(context)).map((tool) => tool.name)).toEqual([
      'mcp_search',
      'mcp_describe',
      'mcp_call',
      'mcp_refresh_catalog'
    ])

    const search = await host.execute({
      callId: 'call_search',
      toolName: 'mcp_search',
      arguments: { query: '查 github issue' }
    }, context)
    expect(search.item.kind).toBe('tool_result')
    if (search.item.kind === 'tool_result') {
      const output = search.item.output as { results: Array<{ toolId: string }> }
      expect(output.results[0]?.toolId).toBe('github/search_issues')
    }

    const describe = await host.execute({
      callId: 'call_describe',
      toolName: 'mcp_describe',
      arguments: { toolId: 'github/search_issues' }
    }, context)
    if (describe.item.kind === 'tool_result') {
      expect(describe.item.output).toMatchObject({
        toolId: 'github/search_issues',
        toolName: 'search_issues'
      })
      expect(describe.item.output).not.toHaveProperty('outputSchema')
    }

    const call = await host.execute({
      callId: 'call_tool',
      toolName: 'mcp_call',
      arguments: { toolId: 'github/search_issues', arguments: { query: 'bug' } }
    }, context)
    if (call.item.kind === 'tool_result') {
      expect(call.item.output).toMatchObject({
        serverId: 'github',
        toolName: 'search_issues',
        result: {
          called: 'search_issues',
          arguments: { query: 'bug' }
        }
      })
    }
    expect(callCount).toBe(1)

    const untrustedContext = buildContext('/tmp/untrusted')
    const untrustedSearch = await host.execute({
      callId: 'call_untrusted_search',
      toolName: 'mcp_search',
      arguments: { query: 'github issue' }
    }, untrustedContext)
    if (untrustedSearch.item.kind === 'tool_result') {
      expect(untrustedSearch.item.output).toMatchObject({
        searchedTools: 0,
        results: []
      })
    }
    const untrustedCall = await host.execute({
      callId: 'call_untrusted_tool',
      toolName: 'mcp_call',
      arguments: { toolId: 'github/search_issues', arguments: { query: 'hidden' } }
    }, untrustedContext)
    if (untrustedCall.item.kind === 'tool_result') {
      expect(untrustedCall.item.output).toMatchObject({
        error: 'unknown MCP tool: github/search_issues'
      })
      expect(untrustedCall.item.isError).toBe(true)
    }
    expect(callCount).toBe(1)

    const deniedCall = await host.execute({
      callId: 'call_denied_mcp_meta',
      toolName: 'mcp_call',
      arguments: { toolId: 'github/search_issues', arguments: { query: 'blocked' } }
    }, {
      ...context,
      approvalPolicy: 'on-request',
      awaitApproval: async (approval) => {
        expect(approval.toolName).toBe('mcp_call')
        return 'deny'
      }
    })
    expect(deniedCall.item.kind).toBe('approval')
    expect(callCount).toBe(1)
  })

  it('hides workspace-scoped tools outside trusted roots', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => fakeClient()
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })

    expect(await host.listTools(buildContext('/tmp/other'))).toEqual([])
    await expect(
      host.execute({
        callId: 'call_1',
        toolName: 'mcp_github_search_issues',
        arguments: { query: 'bug' }
      }, buildContext('/tmp/other'))
    ).rejects.toThrow(/not advertised/)
  })

  it('records diagnostics for failed MCP server connections', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          broken: {
            transport: 'streamable-http',
            url: 'https://example.invalid/mcp',
            trustScope: 'user'
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        throw new Error('connect failed')
      }
    })

    expect(built.providers).toEqual([])
    expect(built.connectedServers).toBe(0)
    expect(built.diagnostics[0]).toMatchObject({
      id: 'broken',
      status: 'error',
      lastError: 'connect failed'
    })
  })

  it('marks remote MCP auth failures with a redacted required-auth diagnostic', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          broken: {
            transport: 'streamable-http',
            url: 'https://mcp.example.test/mcp?access_token=secret-token&workspace=main',
            trustScope: 'user'
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        throw new Error('403 forbidden token=secret-token')
      }
    })

    expect(built.providers).toEqual([])
    expect(built.connectedServers).toBe(0)
    expect(built.diagnostics[0]).toMatchObject({
      id: 'broken',
      status: 'error',
      authStatus: 'required',
      authUrl: 'https://mcp.example.test/mcp?workspace=main',
      lastError: expect.stringContaining(REDACTED_SECRET)
    })
    expect(JSON.stringify(built.diagnostics)).not.toContain('secret-token')
  })

  it('records actionable diagnostics when stdio MCP commands are missing', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          filesystem: {
            transport: 'stdio',
            command: 'npx',
            trustScope: 'user'
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        throw Object.assign(new Error('spawn npx ENOENT'), {
          code: 'ENOENT',
          path: 'npx'
        })
      }
    })

    expect(built.providers).toEqual([])
    expect(built.diagnostics[0]).toMatchObject({
      id: 'filesystem',
      status: 'error'
    })
    expect(built.diagnostics[0]?.lastError).toContain('Could not find "npx" on PATH')
  })

  it('passes MCP timeouts and abort signals to discovery and execution', async () => {
    const listOptions: Array<{ signal?: AbortSignal; timeout?: number } | undefined> = []
    const callOptions: Array<{ signal?: AbortSignal; timeout?: number } | undefined> = []
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project'],
            timeoutMs: 1234
          }
        }
      }
    })
    const client: McpClientLike = {
      async listTools(options) {
        listOptions.push(options)
        return {
          tools: [
            {
              name: 'read',
              inputSchema: { type: 'object' },
              annotations: { readOnlyHint: true }
            }
          ]
        }
      },
      async callTool(_input, options) {
        callOptions.push(options)
        return { ok: true }
      },
      async close() {
        // no-op
      }
    }
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => client
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    const controller = new AbortController()
    const context = { ...buildContext('/tmp/project'), abortSignal: controller.signal }

    await host.execute({
      callId: 'call_1',
      toolName: 'mcp_github_read',
      arguments: {}
    }, context)

    expect(listOptions[0]?.timeout).toBe(1234)
    expect(callOptions[0]?.timeout).toBe(1234)
    expect(callOptions[0]?.signal).toBe(controller.signal)
  })

  it('reconnects and retries once when an MCP tool call fails', async () => {
    let factories = 0
    let closes = 0
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        factories += 1
        const instance = factories
        return {
          async listTools() {
            return {
              tools: [
                {
                  name: 'read',
                  inputSchema: { type: 'object' },
                  annotations: { readOnlyHint: true }
                }
              ]
            }
          },
          async callTool() {
            if (instance === 1) throw new Error('stale connection')
            return { ok: true, instance }
          },
          async close() {
            closes += 1
          }
        }
      }
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    const result = await host.execute({
      callId: 'call_1',
      toolName: 'mcp_github_read',
      arguments: {}
    }, buildContext('/tmp/project'))

    expect(factories).toBe(2)
    expect(closes).toBe(1)
    expect(result.item.kind === 'tool_result' ? result.item.output : {}).toMatchObject({
      result: { ok: true, instance: 2 }
    })
  })

  it('surfaces deterministic MCP protocol errors as tool results without reconnecting', async () => {
    let factories = 0
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        factories += 1
        return {
          async listTools() {
            return {
              tools: [
                {
                  name: 'search',
                  inputSchema: { type: 'object' },
                  annotations: { readOnlyHint: true }
                }
              ]
            }
          },
          async callTool() {
            throw new Error('MCP error -32603: Validation Error: Validation Failed')
          },
          async close() {}
        }
      }
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    const result = await host.execute({
      callId: 'call_1',
      toolName: 'mcp_github_search',
      arguments: {}
    }, buildContext('/tmp/project'))

    expect(factories).toBe(1)
    expect(result.item.kind).toBe('tool_result')
    if (result.item.kind !== 'tool_result') throw new Error('expected tool_result')
    expect(result.item.isError).toBe(true)
    expect(result.item.output).toMatchObject({
      code: 'tool_execution_failed',
      error: expect.stringContaining('-32603')
    })
  })

  it('recovers a server that lost the startup connect race via background reconnect (issue #342)', async () => {
    let factories = 0
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      delay: async () => undefined,
      backgroundReconnect: { baseDelayMs: 0, maxDelayMs: 0 },
      clientFactory: async () => {
        factories += 1
        if (factories === 1) {
          // Mimics the fast startup race timing out on a slow npx cold start.
          throw new Error('MCP server "github" did not connect within 10000ms during startup')
        }
        return {
          async listTools() {
            return {
              tools: [{ name: 'read', inputSchema: { type: 'object' }, annotations: { readOnlyHint: true } }]
            }
          },
          async callTool() {
            return { ok: true }
          },
          async close() {
            // no-op
          }
        }
      }
    })

    // Startup pass: the server failed and advertised no tools.
    expect(built.diagnostics).toEqual([expect.objectContaining({ id: 'github', status: 'error' })])
    expect(built.providers).toHaveLength(0)

    const registry = new CapabilityRegistry(built.providers)
    await built.startBackgroundReconnect((provider) => registry.registerProvider(provider))

    // The background retry connected, registered the tools live, and flipped
    // the diagnostic without a runtime restart.
    expect(factories).toBe(2)
    expect(built.diagnostics).toEqual([
      expect.objectContaining({ id: 'github', status: 'connected', toolCount: 1 })
    ])
    const host = new LocalToolHost({ registry })
    expect((await host.listTools(buildContext('/tmp/project'))).map((tool) => tool.name)).toContain(
      'mcp_github_read'
    )
  })

  it('retries every failed MCP server during background reconnect without re-enabling suspended sources', async () => {
    const factories = new Map<string, number>()
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          },
          linear: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      delay: async () => undefined,
      backgroundReconnect: { baseDelayMs: 0, maxDelayMs: 0 },
      clientFactory: async (serverId) => {
        const attempts = (factories.get(serverId) ?? 0) + 1
        factories.set(serverId, attempts)
        if (attempts === 1) throw new Error(`${serverId} cold start timed out`)
        return {
          async listTools() {
            return {
              tools: [{ name: 'read', inputSchema: { type: 'object' }, annotations: { readOnlyHint: true } }]
            }
          },
          async callTool() {
            return { ok: true }
          },
          async close() {
            // no-op
          }
        }
      }
    })
    const registry = new CapabilityRegistry(built.providers)
    registry.suspendToolSource('mcp:linear', 'disabled during startup retry')

    await built.startBackgroundReconnect((provider) => registry.connectToolSource(provider))

    expect(Object.fromEntries(factories)).toEqual({ github: 2, linear: 2 })
    expect(built.diagnostics).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'github', status: 'connected', toolCount: 1 }),
      expect.objectContaining({ id: 'linear', status: 'error', toolCount: 0 })
    ]))
    const host = new LocalToolHost({ registry })
    expect((await host.listTools(buildContext('/tmp/project'))).map((tool) => tool.name)).toEqual([
      'mcp_github_read'
    ])
  })

  it('does not retry when every MCP server connected at startup', async () => {
    let factories = 0
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      delay: async () => undefined,
      clientFactory: async () => {
        factories += 1
        return fakeClient()
      }
    })
    await built.startBackgroundReconnect(() => {
      throw new Error('register should not be called when nothing failed')
    })
    expect(factories).toBe(1)
  })

  it('reports catalog drift after refreshing MCP search records', async () => {
    let expanded = false
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        search: { enabled: true, mode: 'search' },
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => ({
        async listTools() {
          return {
            tools: [
              { name: 'search_issues', inputSchema: { type: 'object' }, annotations: { readOnlyHint: true } },
              ...(expanded ? [{ name: 'create_issue', inputSchema: { type: 'object' } }] : [])
            ]
          }
        },
        async callTool() {
          return { ok: true }
        },
        async close() {
          // no-op
        }
      })
    })
    const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
    expanded = true
    const refresh = await host.execute({
      callId: 'call_refresh',
      toolName: 'mcp_refresh_catalog',
      arguments: {}
    }, buildContext('/tmp/project'))

    expect(refresh.item.kind === 'tool_result' ? refresh.item.output : {}).toMatchObject({
      totalIndexed: 2,
      catalogDrift: true
    })
  })

  it('redacts secrets from MCP diagnostics', async () => {
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          broken: {
            transport: 'streamable-http',
            url: 'https://mcp.example.test/mcp',
            headers: { Authorization: 'Bearer config-secret' },
            trustScope: 'user'
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => {
        throw new Error('connect failed: authorization: Bearer runtime-secret token=other-secret')
      }
    })

    const encoded = JSON.stringify(built.diagnostics)
    expect(encoded).toContain(REDACTED_SECRET)
    expect(encoded).not.toContain('runtime-secret')
    expect(encoded).not.toContain('other-secret')
    expect(encoded).not.toContain('config-secret')
  })

  it('closes connected MCP clients during shutdown', async () => {
    let closed = 0
    const config = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          github: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: ['/tmp/project']
          }
        }
      }
    })
    const built = await buildMcpToolProviders(config.mcp, {
      clientFactory: async () => ({
        async listTools() {
          return { tools: [] }
        },
        async callTool() {
          return { ok: true }
        },
        async close() {
          closed += 1
        }
      })
    })

    await built.close()

    expect(closed).toBe(1)
  })
})
