import { describe, expect, it } from 'vitest'
import { InMemoryEventBus } from '../src/adapters/in-memory-event-bus.js'
import { InMemorySessionStore } from '../src/adapters/in-memory-session-store.js'
import { InMemoryThreadStore } from '../src/adapters/in-memory-thread-store.js'
import { CapabilityRegistry } from '../src/tool-test-support/tool/capability-registry.js'
import {
  buildGoalLocalTools,
  COMPLETE_STEP_TOOL_NAME,
  GET_GOAL_TOOL_NAME,
  UPDATE_GOAL_TOOL_NAME
} from '../src/tool-test-support/tool/goal-tools.js'
import { LocalToolHost, defaultLocalTools } from '../src/tool-test-support/tool/local-tool-host.js'
import { buildTodoLocalTools } from '../src/tool-test-support/tool/todo-tools.js'
import {
  buildToolCatalogFingerprint,
  canonicalizeToolInputSchema
} from '../src/cache/tool-catalog-fingerprint.js'
import { modelCapabilitiesForModel } from '../src/shared/model-context-profile.js'
import { SequentialIdGenerator } from '../src/ports/id-generator.js'
import type { ToolHostContext } from '../src/ports/tool-host.js'
import { RuntimeEventRecorder } from '../src/services-test-support/runtime-event-recorder.js'
import { ThreadService } from '../src/services-test-support/thread-service.js'

function buildContext(overrides: Partial<ToolHostContext> = {}): ToolHostContext {
  return {
    threadId: 'thr_1',
    turnId: 'turn_1',
    workspace: '/tmp/ws',
    threadMode: 'agent',
    model: modelCapabilitiesForModel('deepseek-chat'),
    memoryPolicy: { enabled: false },
    delegationPolicy: { enabled: false },
    approvalPolicy: 'auto',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow',
    ...overrides
  }
}

function buildThreadService(): ThreadService {
  const bus = new InMemoryEventBus()
  const sessionStore = new InMemorySessionStore()
  let now = 1_700_000_000_000
  const nowIso = () => new Date((now += 1000)).toISOString()
  const events = new RuntimeEventRecorder({
    eventBus: bus,
    sessionStore,
    allocateSeq: (threadId) => bus.allocateSeq(threadId),
    nowIso
  })
  return new ThreadService({
    threadStore: new InMemoryThreadStore(),
    sessionStore,
    events,
    ids: new SequentialIdGenerator(),
    nowIso
  })
}

describe('CapabilityRegistry', () => {
  function dynamicTool(name: string, description = name) {
    return LocalToolHost.defineTool({
      name,
      description,
      inputSchema: { type: 'object' },
      policy: 'auto',
      execute: async () => ({ output: { ok: true } })
    })
  }

  function noisySchemaTool(name: string, description = name) {
    return LocalToolHost.defineTool({
      name,
      description,
      inputSchema: {
        required: ['z', 'a', 'z'],
        properties: {
          z: { enum: ['b', 'a'], type: 'string' },
          a: { type: ['string', 'null'] }
        },
        type: 'object'
      },
      policy: 'untrusted',
      execute: async () => ({ output: { ok: true } })
    })
  }

  it('preserves built-in tool names through the registry-backed host', async () => {
    const directHost = new LocalToolHost({ tools: defaultLocalTools })
    const registryHost = new LocalToolHost({
      registry: CapabilityRegistry.fromLocalTools(defaultLocalTools)
    })

    const directNames = (await directHost.listTools(buildContext())).map((tool) => tool.name).sort()
    const registryTools = await registryHost.listTools(buildContext())

    expect(registryTools.map((tool) => tool.name).sort()).toEqual(directNames)
    expect(registryTools.every((tool) => tool.providerId === 'builtin')).toBe(true)
    expect(registryTools.every((tool) => tool.providerKind === 'built-in')).toBe(true)
    const snipHintByToolName = Object.fromEntries(registryTools.map((tool) => [tool.name, tool.snipHint]))
    expect(snipHintByToolName).toMatchObject({
      find: { head: 80, tail: 8, headChars: 10000, tailChars: 1000 },
      grep: { head: 80, tail: 8, headChars: 10000, tailChars: 1000 },
      ls: { head: 80, tail: 8, headChars: 10000, tailChars: 1000 },
      lsp: { head: 60, tail: 10, headChars: 10000, tailChars: 1500 },
      read: { head: 120, tail: 12, headChars: 12000, tailChars: 2000 }
    })
  })

  it('requires each default built-in tool to declare an explicit snip stance', async () => {
    // Adapted from DeepSeek-Reasonix internal/tool/contract_test.go:
    // a new or renamed built-in must not silently inherit generic stale-result snipping.
    const acceptsDefaultSnip = new Set([
      'echo',
      'edit',
      'request_user_input',
      'user_input',
      'write'
    ])
    const host = new LocalToolHost({
      registry: CapabilityRegistry.fromLocalTools(defaultLocalTools)
    })

    for (const tool of await host.listTools(buildContext({
      awaitUserInput: async () => ({ status: 'cancelled' })
    }))) {
      const hasSnipHint = Boolean(tool.snipHint)
      const acceptsDefault = acceptsDefaultSnip.has(tool.name)
      expect(
        hasSnipHint !== acceptsDefault,
        `${tool.name} must either provide snipHint or be explicitly allowed to use default snipping`
      ).toBe(true)
    }
  })

  it('exposes an auditable default local tool contract snapshot', async () => {
    const host = new LocalToolHost({
      registry: CapabilityRegistry.fromLocalTools(defaultLocalTools)
    })
    const context = buildContext({
      awaitUserInput: async () => ({ status: 'cancelled' })
    })
    const entries = host.contractEntries(context)
    const listedTools = await host.listTools(context)

    expect(entries.map((entry) => entry.name)).toEqual([
      'echo',
      'edit',
      'find',
      'grep',
      'ls',
      'lsp',
      'read',
      'request_user_input',
      'user_input',
      'write'
    ])
    const providerSurface = listedTools
      .map(({ name, description, inputSchema, toolKind, providerId, providerKind }) => ({
        name,
        description,
        inputSchema,
        toolKind,
        providerId,
        providerKind
      }))
      .sort((a, b) => a.name.localeCompare(b.name) || (a.providerId ?? '').localeCompare(b.providerId ?? ''))
    const contractSurface = entries.map(({
      name,
      description,
      inputSchema,
      toolKind,
      providerId,
      providerKind
    }) => ({
      name,
      description,
      inputSchema,
      toolKind,
      providerId,
      providerKind
    }))
    expect(contractSurface).toEqual(providerSurface)
    for (const entry of entries) {
      expect(entry.providerId).toBe('builtin')
      expect(entry.providerKind).toBe('built-in')
      expect(entry.providerEnabled).toBe(true)
      expect(entry.providerAvailable).toBe(true)
      expect(entry.description.trim()).not.toBe('')
      expect(entry.inputSchema).toEqual(canonicalizeToolInputSchema(entry.inputSchema))
      expect(entry.toolKind).toMatch(/^(tool_call|command_execution|file_change|subagent)$/)
    }

    const policyByName = Object.fromEntries(entries.map((entry) => [
      entry.name,
      { toolKind: entry.toolKind, toolPolicy: entry.toolPolicy }
    ]))
    expect(policyByName).toMatchObject({
      edit: { toolKind: 'file_change', toolPolicy: 'on-request' },
      write: { toolKind: 'file_change', toolPolicy: 'on-request' },
      read: { toolKind: 'tool_call', toolPolicy: 'auto' },
      grep: { toolKind: 'tool_call', toolPolicy: 'auto' },
      find: { toolKind: 'tool_call', toolPolicy: 'auto' },
      ls: { toolKind: 'tool_call', toolPolicy: 'auto' },
      lsp: { toolKind: 'tool_call', toolPolicy: 'auto' },
      user_input: { toolKind: 'tool_call', toolPolicy: 'auto' },
      request_user_input: { toolKind: 'tool_call', toolPolicy: 'auto' },
      echo: { toolKind: 'tool_call', toolPolicy: 'auto' }
    })
    expect(entries.some((entry) => 'snipHint' in entry)).toBe(false)
    expect(JSON.stringify(entries)).not.toMatch(/Reasonix|Kun|SessionAPI|Create Loop|create_loop/i)
  })

  it('exposes canonical goal and todo tool contracts without upstream protocol leakage', () => {
    const service = buildThreadService()
    const registry = new CapabilityRegistry([
      {
        id: 'goal',
        kind: 'gui',
        enabled: true,
        available: true,
        tools: buildGoalLocalTools(service)
      },
      {
        id: 'todo',
        kind: 'gui',
        enabled: true,
        available: true,
        tools: buildTodoLocalTools(service)
      }
    ])

    const entries = registry.contractEntries(buildContext())

    expect(entries.map((entry) => entry.name)).toEqual([
      'complete_step',
      'create_goal',
      'get_goal',
      'record_research_direction',
      'todo_list',
      'todo_write',
      'update_goal'
    ])
    for (const entry of entries) {
      expect(entry.providerKind).toBe('gui')
      expect(entry.providerEnabled).toBe(true)
      expect(entry.providerAvailable).toBe(true)
      expect(entry.toolPolicy).toBe('auto')
      expect(entry.inputSchema).toEqual(canonicalizeToolInputSchema(entry.inputSchema))
    }

    const byName = new Map(entries.map((entry) => [entry.name, entry]))
    expect(byName.get('create_goal')).toMatchObject({
      providerId: 'goal',
      inputSchema: {
        additionalProperties: false,
        properties: {
          objective: expect.any(Object),
          strict_completion: {
            description: expect.stringContaining('self-check'),
            type: 'boolean'
          },
          token_budget: expect.any(Object)
        },
        required: ['objective'],
        type: 'object'
      }
    })
    expect(byName.get('complete_step')).toMatchObject({
      providerId: 'goal',
      inputSchema: {
        additionalProperties: false,
        properties: {
          evidence: expect.any(Object),
          requirement_id: expect.any(Object),
          self_check: {
            description: expect.stringContaining('strict-completion self-check'),
            type: 'boolean'
          },
          step: expect.any(Object),
          summary: expect.any(Object)
        },
        required: ['evidence', 'step'],
        type: 'object'
      }
    })
    expect(byName.get('update_goal')).toMatchObject({
      providerId: 'goal',
      inputSchema: {
        additionalProperties: false,
        properties: {
          reason: {
            description: expect.stringContaining('blocked'),
            type: 'string'
          },
          status: {
            enum: ['blocked', 'complete'],
            type: 'string'
          }
        },
        required: ['status'],
        type: 'object'
      }
    })
    expect(byName.get('todo_write')).toMatchObject({
      providerId: 'todo',
      inputSchema: {
        additionalProperties: false,
        properties: {
          todos: expect.objectContaining({
            items: expect.objectContaining({
              additionalProperties: false
            }),
            maxItems: 200,
            type: 'array'
          })
        },
        required: ['todos'],
        type: 'object'
      }
    })
    expect(JSON.stringify(entries)).not.toMatch(/Reasonix|Kun|SessionAPI|Create Loop|create_loop/i)
  })

  it('does not let Plan-mode allow-lists reopen goal execution tools', async () => {
    const service = buildThreadService()
    const host = new LocalToolHost({
      registry: new CapabilityRegistry([{
        id: 'goal',
        kind: 'gui',
        enabled: true,
        available: true,
        tools: buildGoalLocalTools(service)
      }])
    })
    const planContext = buildContext({
      threadMode: 'plan',
      allowedToolNames: [
        GET_GOAL_TOOL_NAME,
        COMPLETE_STEP_TOOL_NAME,
        UPDATE_GOAL_TOOL_NAME
      ]
    })

    expect((await host.listTools(planContext)).map((tool) => tool.name)).toEqual([
      GET_GOAL_TOOL_NAME
    ])
    await expect(
      host.execute(
        {
          callId: 'call_complete_step_in_plan_mode',
          toolName: COMPLETE_STEP_TOOL_NAME,
          arguments: { step: 'implementation step', evidence: ['plan mode must not execute it'] }
        },
        planContext
      )
    ).rejects.toThrow(/active tool policy/)
    await expect(
      host.execute(
        {
          callId: 'call_update_goal_in_plan_mode',
          toolName: UPDATE_GOAL_TOOL_NAME,
          arguments: { status: 'complete' }
        },
        planContext
      )
    ).rejects.toThrow(/active tool policy/)
  })

  it('keeps dynamic tool source catalog fingerprints stable across connect order', async () => {
    const alpha = dynamicTool('alpha_search', 'Search through source alpha.')
    const zeta = dynamicTool('zeta_fetch', 'Fetch through source zeta.')
    const first = new LocalToolHost({
      registry: new CapabilityRegistry([
        { id: 'mcp:zeta', kind: 'mcp', enabled: true, available: true, tools: [zeta] },
        { id: 'mcp:alpha', kind: 'mcp', enabled: true, available: true, tools: [alpha] }
      ])
    })
    const second = new LocalToolHost({
      registry: new CapabilityRegistry([
        { id: 'mcp:alpha', kind: 'mcp', enabled: true, available: true, tools: [alpha] },
        { id: 'mcp:zeta', kind: 'mcp', enabled: true, available: true, tools: [zeta] }
      ])
    })

    const firstTools = await first.listTools(buildContext())
    const secondTools = await second.listTools(buildContext())

    expect(firstTools.map((tool) => tool.name)).toEqual(['zeta_fetch', 'alpha_search'])
    expect(secondTools.map((tool) => tool.name)).toEqual(['alpha_search', 'zeta_fetch'])
    expect(buildToolCatalogFingerprint(firstTools).fingerprint).toBe(
      buildToolCatalogFingerprint(secondTools).fingerprint
    )
  })

  it('keeps dynamic tool source contracts stable across provider connect order', () => {
    const alpha = noisySchemaTool('alpha_search', 'Search through source alpha.')
    const zeta = noisySchemaTool('zeta_fetch', 'Fetch through source zeta.')
    const first = new CapabilityRegistry([
      { id: 'mcp:zeta', kind: 'mcp', enabled: true, available: true, tools: [zeta] },
      { id: 'mcp:alpha', kind: 'mcp', enabled: true, available: true, tools: [alpha] }
    ])
    const second = new CapabilityRegistry([
      { id: 'mcp:alpha', kind: 'mcp', enabled: true, available: true, tools: [alpha] },
      { id: 'mcp:zeta', kind: 'mcp', enabled: true, available: true, tools: [zeta] }
    ])

    expect(first.contractEntries(buildContext())).toEqual(second.contractEntries(buildContext()))
    expect(first.contractEntries(buildContext())).toEqual([
      expect.objectContaining({
        name: 'alpha_search',
        providerId: 'mcp:alpha',
        providerKind: 'mcp',
        toolPolicy: 'untrusted',
        inputSchema: {
          properties: {
            a: { type: ['null', 'string'] },
            z: { enum: ['a', 'b'], type: 'string' }
          },
          required: ['a', 'z'],
          type: 'object'
        }
      }),
      expect.objectContaining({
        name: 'zeta_fetch',
        providerId: 'mcp:zeta',
        providerKind: 'mcp',
        toolPolicy: 'untrusted'
      })
    ])
  })

  it('advertises canonical input schemas from the registry surface', async () => {
    const tool = LocalToolHost.defineTool({
      name: 'schema_probe',
      description: 'schema probe',
      inputSchema: {
        required: ['z', 'a', 'z'],
        properties: {
          z: { enum: ['b', 'a'], type: 'string' },
          a: { type: ['string', 'null'] }
        },
        dependentRequired: {
          z: ['b', 'a', 'b'],
          empty: []
        },
        type: 'object'
      },
      policy: 'auto',
      execute: async () => ({ output: { ok: true } })
    })
    const host = new LocalToolHost({ tools: [tool] })

    const listed = await host.listTools(buildContext())

    expect(listed[0]?.inputSchema).toEqual({
      dependentRequired: {
        z: ['a', 'b']
      },
      properties: {
        a: { type: ['null', 'string'] },
        z: { enum: ['a', 'b'], type: 'string' }
      },
      required: ['a', 'z'],
      type: 'object'
    })
    expect(JSON.stringify(listed[0]?.inputSchema)).toBe(
      '{"dependentRequired":{"z":["a","b"]},"properties":{"a":{"type":["null","string"]},"z":{"enum":["a","b"],"type":"string"}},"required":["a","z"],"type":"object"}'
    )
  })

  it('connects and disconnects dynamic tool sources through diagnostics without leaking source metadata into tool schemas', async () => {
    const registry = new CapabilityRegistry()
    const host = new LocalToolHost({ registry })
    const provider = {
      id: 'mcp:research',
      kind: 'mcp' as const,
      enabled: true,
      available: true,
      tools: [dynamicTool('research_lookup', 'Lookup research notes.')]
    }

    registry.connectToolSource(provider)
    expect((await host.listTools(buildContext())).map((tool) => tool.name)).toEqual([
      'research_lookup'
    ])
    expect((await host.listTools(buildContext()))[0]?.inputSchema).toEqual({ type: 'object' })
    expect(registry.diagnostics()).toEqual([
      expect.objectContaining({
        id: 'mcp:research',
        available: true,
        toolCount: 1,
        toolNames: ['research_lookup'],
        catalogFingerprint: expect.any(String)
      })
    ])

    expect(registry.disconnectToolSource('mcp:research', 'disabled by connect_tool_source policy')).toBe(true)
    expect(await host.listTools(buildContext())).toEqual([])
    expect(registry.diagnostics()).toEqual([
      expect.objectContaining({
        id: 'mcp:research',
        available: false,
        reason: 'disabled by connect_tool_source policy',
        toolCount: 1,
        toolNames: ['research_lookup']
      })
    ])

    registry.connectToolSource(provider)
    expect((await host.listTools(buildContext())).map((tool) => tool.name)).toEqual([
      'research_lookup'
    ])
  })

  it('keeps suspended tool sources from being reinstalled by late background connects', async () => {
    const registry = new CapabilityRegistry()
    const host = new LocalToolHost({ registry })
    const provider = {
      id: 'mcp:github',
      kind: 'mcp' as const,
      enabled: true,
      available: true,
      tools: [dynamicTool('mcp_github_read', 'Read through GitHub MCP.')]
    }

    registry.connectToolSource(provider)
    expect((await host.listTools(buildContext())).map((tool) => tool.name)).toEqual([
      'mcp_github_read'
    ])

    expect(registry.suspendToolSource('mcp:github', 'disabled by MCP config reload')).toBe(true)
    expect(await host.listTools(buildContext())).toEqual([])
    expect(registry.connectToolSource(provider)).toBe(false)
    expect(await host.listTools(buildContext())).toEqual([])
    expect(registry.diagnostics()).toEqual([
      expect.objectContaining({
        id: 'mcp:github',
        available: false,
        reason: 'disabled by MCP config reload',
        toolCount: 0,
        toolNames: []
      })
    ])

    expect(registry.resumeToolSource('mcp:github')).toBe(true)
    expect(registry.connectToolSource(provider)).toBe(true)
    expect((await host.listTools(buildContext())).map((tool) => tool.name)).toEqual([
      'mcp_github_read'
    ])
  })

  it('rejects duplicate tool names across providers', () => {
    const tool = LocalToolHost.defineTool({
      name: 'same',
      description: 'same',
      inputSchema: { type: 'object' },
      policy: 'auto',
      execute: async () => ({ output: {} })
    })

    expect(() => new CapabilityRegistry([
      { id: 'p1', kind: 'built-in', enabled: true, available: true, tools: [tool] },
      { id: 'p2', kind: 'web', enabled: true, available: true, tools: [tool] }
    ])).toThrow(/duplicate tool name/)
  })

  it('hides disabled providers and rejects execution before the provider is reached', async () => {
    let executed = false
    const tool = LocalToolHost.defineTool({
      name: 'web_fetch',
      description: 'fetch',
      inputSchema: { type: 'object' },
      policy: 'auto',
      execute: async () => {
        executed = true
        return { output: { ok: true } }
      }
    })
    const registry = new CapabilityRegistry([
      {
        id: 'web',
        kind: 'web',
        enabled: false,
        available: false,
        reason: 'disabled by config',
        tools: [tool]
      }
    ])
    const host = new LocalToolHost({ registry })

    expect(await host.listTools(buildContext())).toEqual([])
    await expect(
      host.execute(
        { callId: 'call_1', toolName: 'web_fetch', arguments: {} },
        buildContext()
      )
    ).rejects.toThrow(/not advertised/)
    expect(executed).toBe(false)
  })

  it('honors provider allow-lists before executing a tool', async () => {
    let executed = false
    const tool = LocalToolHost.defineTool({
      name: 'memory_create',
      description: 'remember',
      inputSchema: { type: 'object' },
      policy: 'auto',
      execute: async () => {
        executed = true
        return { output: { ok: true } }
      }
    })
    const host = new LocalToolHost({
      registry: new CapabilityRegistry([
        { id: 'memory', kind: 'memory', enabled: true, available: true, tools: [tool] }
      ])
    })

    const blocked = buildContext({ allowedProviderIds: ['builtin'] })
    expect(await host.listTools(blocked)).toEqual([])
    await expect(
      host.execute(
        { callId: 'call_1', toolName: 'memory_create', arguments: {} },
        blocked
      )
    ).rejects.toThrow(/not advertised/)
    expect(executed).toBe(false)
  })

  it('passes extended turn context to tool advertisement gates', async () => {
    const tool = LocalToolHost.defineTool({
      name: 'vision_tool',
      description: 'needs images',
      inputSchema: { type: 'object' },
      policy: 'auto',
      shouldAdvertise: (context) =>
        Boolean(context.model?.inputModalities.includes('image') && context.delegationPolicy?.enabled),
      execute: async () => ({ output: { ok: true } })
    })
    const host = new LocalToolHost({ tools: [tool] })

    expect(await host.listTools(buildContext())).toEqual([])
    const visible = await host.listTools(
      buildContext({
        model: {
          ...modelCapabilitiesForModel('vision-model'),
          inputModalities: ['text', 'image'],
          messageParts: ['text', 'image_url']
        },
        delegationPolicy: { enabled: true, maxParallel: 2 }
      })
    )
    expect(visible.map((entry) => entry.name)).toEqual(['vision_tool'])
  })
})
