import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { mkdtemp, realpath, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import { LocalToolHost } from '../src/tool-test-support/tool/local-tool-host.js'
import { buildMcpToolProviders, type McpClientLike } from '../src/tool-test-support/tool/mcp-tool-provider.js'
import { buildToolCatalogFingerprint, canonicalizeToolInputSchema } from '../src/cache/tool-catalog-fingerprint.js'
import { AnalytixCapabilitiesConfig, type McpServerConfig } from '../src/contracts/capabilities.js'
import { runLiveLocalIndexerContract } from '../src/conformance/mcp-live-local-indexer.js'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { McpToolLifecycleContract } from '../src/conformance/runtime-parity-fixtures.js'
import { buildHarness, readJson } from './http-server-test-harness.js'
import type { ToolHostContext } from '../src/ports/tool-host.js'
import type { RuntimeToolsResponse } from '../src/contracts/runtime-tools.js'

const contractUrl = new URL(
  '../src/conformance/fixtures/mcp-tool-lifecycle-contract.json',
  import.meta.url
)

function loadContract(): McpToolLifecycleContract {
  return McpToolLifecycleContract.parse(
    JSON.parse(readFileSync(contractUrl, 'utf8'))
  )
}

function buildContext(overrides: Partial<ToolHostContext> = {}): ToolHostContext {
  return {
    threadId: 'thr_lifecycle',
    turnId: 'turn_lifecycle',
    workspace: '/tmp/analytix-mcp-lifecycle',
    threadMode: 'agent',
    approvalPolicy: 'auto',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow',
    ...overrides
  }
}

function lifecycleTool(name: string, inputSchema: Record<string, unknown> = { type: 'object' }) {
  return LocalToolHost.defineTool({
    name,
    description: `${name} fixture`,
    inputSchema,
    policy: 'auto',
    execute: async () => ({ output: { ok: true, name } })
  })
}

function lifecycleMcpClient(): McpClientLike {
  return {
    async listTools() {
      return { tools: [{ name: 'status', inputSchema: { type: 'object' }, annotations: { readOnlyHint: true } }] }
    },
    async callTool() {
      return { ok: true }
    },
    async close() {
      // no-op
    }
  }
}

function mcpTextPayload(output: unknown): Record<string, unknown> {
  const result = (output as { result?: { content?: Array<{ text?: string }> } }).result
  const text = result?.content?.[0]?.text
  if (!text) throw new Error('expected text MCP result')
  return JSON.parse(text) as Record<string, unknown>
}

describe('MCP/tool lifecycle conformance fixture', () => {
  it('pins connect, disconnect, reload, cancel, error, and diagnostic redaction behavior', async () => {
    const contract = loadContract()
    const registry = new CapabilityRegistry()
    const host = new LocalToolHost({ registry })
    const context = buildContext()
    const alphaSchema = {
      type: 'object',
      properties: { b: { type: 'string' }, a: { type: 'number' } },
      required: ['a']
    }
    const alphaSchemaReordered = {
      required: ['a'],
      properties: { a: { type: 'number' }, b: { type: 'string' } },
      type: 'object'
    }

    registry.connectToolSource({
      id: contract.providerId,
      kind: 'mcp',
      enabled: true,
      available: true,
      tools: [lifecycleTool('alpha_lookup', alphaSchema)]
    })
    expect((await host.listTools(context)).map((tool) => tool.name)).toEqual(contract.connect.toolNames)
    expect(registry.diagnostics()).toEqual([
      expect.objectContaining({
        id: contract.providerId,
        available: contract.connect.diagnostic.available,
        toolCount: contract.connect.diagnostic.toolCount
      })
    ])
    const firstFingerprint = buildToolCatalogFingerprint(await host.listTools(context)).fingerprint

    expect(registry.disconnectToolSource(contract.providerId, contract.disconnect.reason)).toBe(true)
    expect((await host.listTools(context)).map((tool) => tool.name)).toEqual(contract.disconnect.toolNames)
    expect(registry.diagnostics()).toEqual([
      expect.objectContaining({
        id: contract.providerId,
        available: contract.disconnect.diagnostic.available,
        reason: contract.disconnect.reason,
        toolCount: contract.disconnect.diagnostic.toolCount
      })
    ])

    registry.connectToolSource({
      id: contract.providerId,
      kind: 'mcp',
      enabled: true,
      available: true,
      tools: [
        lifecycleTool('alpha_lookup', alphaSchemaReordered),
        lifecycleTool('beta_fetch')
      ]
    })
    const reloadedTools = await host.listTools(context)
    expect(reloadedTools.map((tool) => tool.name)).toEqual(contract.reload.toolNames)
    const alphaOnlyAfterReload = reloadedTools.filter((tool) => tool.name === 'alpha_lookup')
    expect(buildToolCatalogFingerprint(alphaOnlyAfterReload).fingerprint).toBe(firstFingerprint)

    let cancelledExecuted = false
    const cancelledTool = lifecycleTool('cancel_before_start')
    const cancelRegistry = new CapabilityRegistry([{
      id: 'mcp:cancel',
      kind: 'mcp',
      enabled: true,
      available: true,
      tools: [{
        ...cancelledTool,
        execute: async () => {
          cancelledExecuted = true
          return { output: { ok: true } }
        }
      }]
    }])
    const cancelHost = new LocalToolHost({ registry: cancelRegistry })
    const aborted = new AbortController()
    aborted.abort()
    await expect(cancelHost.execute(
      { callId: 'call_cancel', toolName: 'cancel_before_start', arguments: {} },
      buildContext({ abortSignal: aborted.signal })
    )).rejects.toThrow(contract.cancel.errorSubstring)
    expect(cancelledExecuted).toBe(contract.cancel.executed)

    const errorHost = new LocalToolHost({
      tools: [
        LocalToolHost.defineTool({
          name: 'mcp_error',
          description: 'returns a tool-level error',
          inputSchema: { type: 'object' },
          policy: 'auto',
          execute: async () => {
            throw new Error('MCP fixture protocol error')
          }
        })
      ]
    })
    const errorResult = await errorHost.execute(
      { callId: 'call_error', toolName: 'mcp_error', arguments: {} },
      buildContext()
    )
    expect(errorResult.approved).toBe(contract.error.approved)
    expect(errorResult.item.kind).toBe('tool_result')
    if (errorResult.item.kind !== 'tool_result') throw new Error('expected tool_result')
    expect(errorResult.item.output).toMatchObject({ code: contract.error.code })

    let reconnectFactories = 0
    let reconnectCloses = 0
    const callReconnectConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          [contract.callReconnect.serverId]: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.knownOverride.workspaceRoot]
          }
        }
      }
    })
    const reconnectProviders = await buildMcpToolProviders(callReconnectConfig.mcp, {
      workspaceRoot: contract.knownOverride.workspaceRoot,
      clientFactory: async () => {
        reconnectFactories += 1
        const instance = reconnectFactories
        return {
          async listTools() {
            return {
              tools: [{
                name: contract.callReconnect.toolName,
                inputSchema: { type: 'object' },
                annotations: { readOnlyHint: true }
              }]
            }
          },
          async callTool() {
            if (instance === 1) throw new Error(contract.callReconnect.transportError)
            return { ok: true, instance }
          },
          async close() {
            reconnectCloses += 1
          }
        }
      }
    })
    const reconnectHost = new LocalToolHost({ registry: new CapabilityRegistry(reconnectProviders.providers) })
    const reconnectResult = await reconnectHost.execute({
      callId: 'call_reconnect',
      toolName: contract.callReconnect.normalizedToolName,
      arguments: {}
    }, buildContext())
    expect(reconnectFactories).toBe(contract.callReconnect.staleConnection.factoryAttempts)
    expect(reconnectCloses).toBe(contract.callReconnect.staleConnection.closeCount)
    expect(reconnectResult.item.kind === 'tool_result' ? reconnectResult.item.output : {}).toMatchObject({
      result: { ok: true, instance: contract.callReconnect.staleConnection.resultInstance }
    })

    let protocolFactories = 0
    let protocolCloses = 0
    const protocolProviders = await buildMcpToolProviders(callReconnectConfig.mcp, {
      workspaceRoot: contract.knownOverride.workspaceRoot,
      clientFactory: async () => {
        protocolFactories += 1
        return {
          async listTools() {
            return {
              tools: [{
                name: contract.callReconnect.toolName,
                inputSchema: { type: 'object' },
                annotations: { readOnlyHint: true }
              }]
            }
          },
          async callTool() {
            throw new Error(contract.callReconnect.protocolError)
          },
          async close() {
            protocolCloses += 1
          }
        }
      }
    })
    const protocolHost = new LocalToolHost({ registry: new CapabilityRegistry(protocolProviders.providers) })
    const protocolResult = await protocolHost.execute({
      callId: 'call_protocol_error',
      toolName: contract.callReconnect.normalizedToolName,
      arguments: {}
    }, buildContext())
    expect(protocolFactories).toBe(contract.callReconnect.protocolFailure.factoryAttempts)
    expect(protocolCloses).toBe(contract.callReconnect.protocolFailure.closeCount)
    expect(protocolResult.item.kind).toBe('tool_result')
    if (protocolResult.item.kind !== 'tool_result') throw new Error('expected tool_result')
    expect(protocolResult.item.isError).toBe(contract.callReconnect.protocolFailure.isError)
    expect(protocolResult.item.output).toMatchObject({
      code: contract.callReconnect.protocolFailure.code,
      error: expect.stringContaining('-32603')
    })

    let annotatedExecutions = 0
    const approvalConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          [contract.approvalAnnotations.serverId]: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.knownOverride.workspaceRoot]
          }
        }
      }
    })
    const approvalProviders = await buildMcpToolProviders(approvalConfig.mcp, {
      workspaceRoot: contract.knownOverride.workspaceRoot,
      clientFactory: async () => ({
        async listTools() {
          return {
            tools: [{
              name: contract.approvalAnnotations.toolName,
              inputSchema: { type: 'object' },
              annotations: contract.approvalAnnotations.annotations
            }]
          }
        },
        async callTool() {
          annotatedExecutions += 1
          return { ok: true }
        },
        async close() {
          // no-op
        }
      })
    })
    const approvalHost = new LocalToolHost({ registry: new CapabilityRegistry(approvalProviders.providers) })
    expect((await approvalHost.listTools(buildContext())).map((tool) => tool.name))
      .toEqual([contract.approvalAnnotations.normalizedToolName])
    const approvalResult = await approvalHost.execute({
      callId: 'call_mcp_delete',
      toolName: contract.approvalAnnotations.normalizedToolName,
      arguments: { target: 'stale-index' }
    }, buildContext({
      approvalPolicy: 'on-request',
      awaitApproval: async (approval) => {
        expect(approval.id).toBe(contract.approvalAnnotations.approvalId)
        expect(approval.toolName).toBe(contract.approvalAnnotations.normalizedToolName)
        return contract.approvalAnnotations.decision
      }
    }))
    expect(approvalResult.approved).toBe(false)
    expect(approvalResult.item.kind).toBe(contract.approvalAnnotations.resultKind)
    if (approvalResult.item.kind !== 'approval') throw new Error('expected approval item')
    expect(approvalResult.item.approvalId).toBe(contract.approvalAnnotations.approvalId)
    expect(annotatedExecutions).toBe(0)

    const h = buildHarness()
    const baselineResponse = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    const baseline = await readJson(baselineResponse) as RuntimeToolsResponse
    h.runtime.toolDiagnostics = () => ({
      ...baseline,
      providerCount: 1,
      mcpServers: [{
        id: 'research',
        status: 'error',
        failureCode: 'connection_failed',
        transport: 'stdio',
        authStatus: 'required',
        trustScope: 'workspace',
        enabled: true,
        available: false,
        connected: false,
        schemaHintAvailable: true,
        connectable: true,
        toolCount: 0,
        promptCount: 0,
        resourceCount: 0,
        toolContractQuarantineCount: 0,
        lowPriority: false,
        backgroundStart: false
      }]
    })
    const response = await dispatchRequest(
      h.router,
      new Request('http://localhost/v1/runtime/tools', {
        headers: { authorization: 'Bearer tok-1' }
      })
    )
    expect(response.status).toBe(200)
    const parsedBody = await readJson(response) as RuntimeToolsResponse
    const body = JSON.stringify(parsedBody)
    expect(body).not.toContain(contract.diagnosticsRedaction.secret)
    expect(parsedBody.mcpServers[0]?.failureCode).toBe('connection_failed')

    const captured = new Map<string, McpServerConfig>()
    const knownOverrideConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          [contract.knownOverride.serverId]: {
            transport: 'stdio',
            command: 'npx',
            args: ['-y', '@analytix/codegraph'],
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.knownOverride.workspaceRoot]
          }
        }
      }
    })
    const knownOverride = await buildMcpToolProviders(knownOverrideConfig.mcp, {
      workspaceRoot: contract.knownOverride.workspaceRoot,
      clientFactory: async (serverId, server) => {
        captured.set(serverId, server)
        return lifecycleMcpClient()
      }
    })
    expect(captured.get(contract.knownOverride.serverId)).toMatchObject({
      cwd: contract.knownOverride.workspaceRoot,
      lowPriority: true,
      backgroundStart: true,
      env: expect.objectContaining({
        CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS: contract.knownOverride.daemonIdleTimeoutMs
      })
    })
    expect(knownOverride.diagnostics[0]).toMatchObject(contract.knownOverride.diagnostic)

    const explicitCwdConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: {
          [contract.knownOverride.serverId]: {
            transport: 'stdio',
            command: 'npx',
            args: ['-y', '@analytix/codegraph'],
            cwd: contract.knownOverride.explicitCwd,
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.knownOverride.workspaceRoot]
          }
        }
      }
    })
    await buildMcpToolProviders(explicitCwdConfig.mcp, {
      workspaceRoot: contract.knownOverride.workspaceRoot,
      clientFactory: async (serverId, server) => {
        captured.set(`${serverId}:explicit`, server)
        return lifecycleMcpClient()
      }
    })
    expect(captured.get(`${contract.knownOverride.serverId}:explicit`)?.cwd).toBe(
      contract.knownOverride.explicitCwd
    )

    for (const variant of contract.knownOverrideVariants) {
      const variantCaptured = new Map<string, McpServerConfig>()
      const isCodegraph = variant.diagnostic.knownOverride === 'codegraph'
      const variantConfig = AnalytixCapabilitiesConfig.parse({
        mcp: {
          enabled: true,
          servers: {
            [variant.serverId]: {
              transport: 'stdio',
              command: isCodegraph ? 'npx' : 'node',
              args: isCodegraph ? ['-y', '@analytix/codegraph'] : ['codebase-memory-server.js'],
              ...(variant.explicitCwd ? { cwd: variant.explicitCwd } : {}),
              trustScope: 'workspace',
              trustedWorkspaceRoots: [variant.workspaceRoot]
            }
          }
        }
      })
      const builtVariant = await buildMcpToolProviders(variantConfig.mcp, {
        workspaceRoot: variant.workspaceRoot,
        clientFactory: async (serverId, server) => {
          variantCaptured.set(serverId, server)
          return lifecycleMcpClient()
        }
      })
      const capturedServer = variantCaptured.get(variant.serverId)
      expect(capturedServer).toMatchObject({
        cwd: variant.diagnostic.effectiveCwd,
        lowPriority: true,
        backgroundStart: true
      })
      if (variant.daemonIdleTimeoutMs) {
        expect(capturedServer?.env).toMatchObject({
          CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS: variant.daemonIdleTimeoutMs
        })
      }
      expect(builtVariant.diagnostics[0]).toMatchObject(variant.diagnostic)
    }

    const attempts = new Map<string, number>()
    const reconnectConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        servers: Object.fromEntries(contract.backgroundReconnect.failedServerIds.map((serverId) => [
          serverId,
          {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.knownOverride.workspaceRoot]
          }
        ]))
      }
    })
    const reconnect = await buildMcpToolProviders(reconnectConfig.mcp, {
      delay: async () => undefined,
      backgroundReconnect: { baseDelayMs: 0, maxDelayMs: 0 },
      clientFactory: async (serverId) => {
        const nextAttempt = (attempts.get(serverId) ?? 0) + 1
        attempts.set(serverId, nextAttempt)
        if (nextAttempt === 1) throw new Error(`${serverId} cold start timed out`)
        return lifecycleMcpClient()
      }
    })
    const reconnectRegistry = new CapabilityRegistry(reconnect.providers)
    reconnectRegistry.suspendToolSource(
      contract.backgroundReconnect.suspendedProviderId,
      contract.backgroundReconnect.suspendedReason
    )
    await reconnect.startBackgroundReconnect((provider) => reconnectRegistry.connectToolSource(provider))

    expect(Object.fromEntries(attempts)).toEqual(Object.fromEntries(
      contract.backgroundReconnect.failedServerIds.map((serverId) => [
        serverId,
        contract.backgroundReconnect.attemptsPerFailedServer
      ])
    ))
    for (const serverId of contract.backgroundReconnect.expectedConnectedServerIds) {
      expect(reconnect.diagnostics).toEqual(expect.arrayContaining([
        expect.objectContaining({ id: serverId, status: 'connected', toolCount: 1 })
      ]))
    }
    for (const serverId of contract.backgroundReconnect.expectedErrorServerIds) {
      expect(reconnect.diagnostics).toEqual(expect.arrayContaining([
        expect.objectContaining({ id: serverId, status: 'error', toolCount: 0 })
      ]))
    }
    expect(contract.backgroundReconnect.requiresRuntimeRestart).toBe(false)
    expect((await new LocalToolHost({ registry: reconnectRegistry }).listTools(context)).map((tool) => tool.name))
      .toEqual(['mcp_github_status'])

    let refreshExpanded = false
    const refreshConfig = AnalytixCapabilitiesConfig.parse({
      mcp: {
        enabled: true,
        search: { enabled: true, mode: 'search' },
        servers: {
          [contract.searchMetaTools.refreshDrift.serverId]: {
            transport: 'stdio',
            command: 'node',
            trustScope: 'workspace',
            trustedWorkspaceRoots: [contract.searchMetaTools.trustedWorkspace]
          }
        }
      }
    })
    const refreshProviders = await buildMcpToolProviders(refreshConfig.mcp, {
      clientFactory: async () => ({
        async listTools() {
          const names = refreshExpanded
            ? contract.searchMetaTools.refreshDrift.expandedToolNames
            : contract.searchMetaTools.refreshDrift.initialToolNames
          return {
            tools: names.map((name) => ({
              name,
              inputSchema: { type: 'object' },
              annotations: { readOnlyHint: name.startsWith('search_') }
            }))
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
    refreshExpanded = true
    const refreshResult = await new LocalToolHost({
      registry: new CapabilityRegistry(refreshProviders.providers)
    }).execute({
      callId: 'call_refresh_catalog',
      toolName: 'mcp_refresh_catalog',
      arguments: {}
    }, buildContext({ workspace: contract.searchMetaTools.trustedWorkspace }))
    expect(refreshResult.item.kind).toBe('tool_result')
    if (refreshResult.item.kind !== 'tool_result') throw new Error('expected tool_result')
    expect(refreshResult.item.output).toMatchObject({
      totalIndexed: contract.searchMetaTools.refreshDrift.expectedTotalIndexed,
      catalogDrift: contract.searchMetaTools.refreshDrift.expectedCatalogDrift
    })
  })

  it('keeps cache fingerprints stable across schema set ordering and malformed required fields', () => {
    const schema = {
      type: ['object'],
      required: ['b', 1, 'a', 'a', false],
      dependentRequired: { z: ['q', 'p', 3, 'p'], bad: true, empty: [] },
      properties: {
        child: { type: 'object', required: ['y', 'x', false] },
        n: { type: 'string', enum: ['b', 'a'] }
      }
    }
    const reordered = {
      properties: {
        n: { enum: ['a', 'b'], type: 'string' },
        child: { required: ['x', 'y'], type: 'object' }
      },
      dependentRequired: { bad: false, z: ['p', 'q'] },
      required: ['a', 'b'],
      type: ['object']
    }

    expect(canonicalizeToolInputSchema(schema)).toEqual({
      dependentRequired: { z: ['p', 'q'] },
      properties: {
        child: { required: ['x', 'y'], type: 'object' },
        n: { enum: ['a', 'b'], type: 'string' }
      },
      required: ['a', 'b'],
      type: ['object']
    })
    expect(buildToolCatalogFingerprint([lifecycleTool('schema_probe', schema)]).fingerprint)
      .toBe(buildToolCatalogFingerprint([lifecycleTool('schema_probe', reordered)]).fingerprint)
  })

  it('covers local indexer restart, tombstone, retry-all, cwd, and redaction semantics', () => {
    const contract = loadContract()
    const output = runLiveLocalIndexerContract(contract.liveLocalIndexer)

    expect(output).toEqual(contract.liveLocalIndexer.expectedOutput)
    expect(output.secretSafeDiagnostic).toContain(contract.diagnosticsRedaction.replacement)
    expect(output.secretSafeDiagnostic).not.toContain(contract.liveLocalIndexer.secretDiagnostic)
  })

  it('runs an executable stdio indexer server for restart and tombstone coverage', async () => {
    const contract = loadContract()
    const dir = await mkdtemp(join(tmpdir(), 'analytix-mcp-indexer-'))
    const realDir = await realpath(dir)
    const statePath = join(dir, 'state.json')
    const serverPath = fileURLToPath(new URL('./fixtures/contract-mcp-indexer-server.mjs', import.meta.url))
    let built: Awaited<ReturnType<typeof buildMcpToolProviders>> | undefined
    let rebuilt: Awaited<ReturnType<typeof buildMcpToolProviders>> | undefined
    try {
      const mcp = AnalytixCapabilitiesConfig.parse({
        mcp: {
          enabled: true,
          servers: {
            codegraph: {
              transport: 'stdio',
              command: process.execPath,
              args: [serverPath],
              cwd: realDir,
              env: {
                ANALYTIX_CONTRACT_INDEXER_STATE: statePath,
                ANALYTIX_CONTRACT_INDEXER_SECRET: contract.diagnosticsRedaction.secret
              },
              trustScope: 'workspace',
              trustedWorkspaceRoots: [realDir],
              timeoutMs: 5000
            }
          }
        }
      }).mcp
      built = await buildMcpToolProviders(mcp, { workspaceRoot: realDir, startupConnectTimeoutMs: 5000 })
      expect(built.connectedServers).toBe(1)
      expect(built.diagnostics).toEqual([
        expect.objectContaining({
          id: 'codegraph',
          status: 'connected',
          effectiveCwd: realDir,
          lowPriority: true,
          backgroundStart: true,
          knownOverride: 'codegraph'
        })
      ])
      const host = new LocalToolHost({ registry: new CapabilityRegistry(built.providers) })
      const context = buildContext({ workspace: realDir })
      const toolNames = (await host.listTools(context)).map((tool) => tool.name)
      expect(toolNames).toEqual(expect.arrayContaining([
        'mcp_codegraph_index_seed',
        'mcp_codegraph_index_status',
        'mcp_codegraph_index_tombstone',
        'mcp_codegraph_index_resume'
      ]))

      await host.execute({
        callId: 'seed',
        toolName: 'mcp_codegraph_index_seed',
        arguments: { files: contract.liveLocalIndexer.initialFiles }
      }, context)
      await host.execute({
        callId: 'tombstone',
        toolName: 'mcp_codegraph_index_tombstone',
        arguments: { path: contract.liveLocalIndexer.lateTombstonePath }
      }, context)
      await built.close()

      rebuilt = await buildMcpToolProviders(mcp, { workspaceRoot: realDir, startupConnectTimeoutMs: 5000 })
      const resumedHost = new LocalToolHost({ registry: new CapabilityRegistry(rebuilt.providers) })
      const resumeResult = await resumedHost.execute({
        callId: 'resume',
        toolName: 'mcp_codegraph_index_resume',
        arguments: { files: contract.liveLocalIndexer.resumeFiles }
      }, context)
      const resumePayload = mcpTextPayload(
        resumeResult.item.kind === 'tool_result' ? resumeResult.item.output : {}
      )
      expect(resumePayload).toMatchObject({
        activePaths: contract.liveLocalIndexer.expectedOutput.activePaths,
        tombstoneCount: contract.liveLocalIndexer.expectedOutput.tombstoneCount
      })
      const statusResult = await resumedHost.execute({
        callId: 'status',
        toolName: 'mcp_codegraph_index_status',
        arguments: {}
      }, context)
      expect(mcpTextPayload(statusResult.item.kind === 'tool_result' ? statusResult.item.output : {})).toMatchObject({
        cwd: realDir,
        activePaths: contract.liveLocalIndexer.expectedOutput.activePaths
      })
      const failureResult = await resumedHost.execute({
        callId: 'failure',
        toolName: contract.liveLocalIndexer.executionError.toolName,
        arguments: {}
      }, context)
      expect(failureResult.item.kind).toBe('tool_result')
      if (failureResult.item.kind !== 'tool_result') throw new Error('expected tool_result')
      expect(failureResult.item.isError).toBe(contract.liveLocalIndexer.executionError.isError)
      const failureJson = JSON.stringify(failureResult.item.output)
      expect(failureJson).toContain(contract.liveLocalIndexer.executionError.secretSafeError)
      expect(failureJson).not.toContain(contract.liveLocalIndexer.secretDiagnostic)
    } finally {
      await built?.close()
      await rebuilt?.close()
      await rm(dir, { recursive: true, force: true })
    }
  })
})
