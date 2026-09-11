import type {
  ToolContractEntry,
  ToolHostContext,
  ToolKind,
  ToolProviderKind,
  ToolProviderPolicy
} from '../../ports/tool-host.js'
import {
  buildToolCatalogFingerprint,
  canonicalizeToolInputSchema
} from '../../cache/tool-catalog-fingerprint.js'
import type { LocalTool } from './local-tool-host.js'
import { isToolAdvertisedInSandbox } from './sandbox-policy.js'

export type CapabilityToolRecord = {
  provider: ToolProviderPolicy
  tool: LocalTool
}

export type CapabilityToolProvider = ToolProviderPolicy & {
  tools: readonly LocalTool[]
}

export type CapabilityToolSourceDiagnostic = ToolProviderPolicy & {
  toolCount: number
  toolNames: string[]
  catalogFingerprint: string
}

export type CapabilityToolSpec = {
  name: string
  description: string
  inputSchema: Record<string, unknown>
  toolKind?: ToolKind
  snipHint?: LocalTool['snipHint']
  providerId: string
  providerKind: ToolProviderKind
}

export type CapabilityToolContractEntry = ToolContractEntry

const PLAN_MODE_ALLOWED_TOOL_NAMES = new Set([
  'read',
  'grep',
  'find',
  'ls',
  'create_plan',
  // Adapted from DeepSeek-Reasonix internal/planmode/policy.go: Plan
  // mode may inspect active-goal state, but execution-state tools remain
  // unavailable until the user accepts the plan and returns to Agent mode.
  'get_goal',
  'todo_list',
  'todo_write',
  'user_input',
  'request_user_input'
])

export class CapabilityRegistry {
  private readonly providers = new Map<string, CapabilityToolProvider>()
  private readonly tools = new Map<string, CapabilityToolRecord>()
  private readonly suspendedProviders = new Map<string, string>()

  static fromLocalTools(tools: readonly LocalTool[]): CapabilityRegistry {
    return new CapabilityRegistry([
      {
        id: 'builtin',
        kind: 'built-in',
        enabled: true,
        available: true,
        tools
      }
    ])
  }

  constructor(providers: readonly CapabilityToolProvider[] = []) {
    for (const provider of providers) {
      this.registerProvider(provider)
    }
  }

  registerProvider(provider: CapabilityToolProvider): void {
    if (this.providers.has(provider.id)) {
      throw new Error(`duplicate tool provider: ${provider.id}`)
    }
    if (this.suspendedProviders.has(provider.id)) {
      this.providers.set(provider.id, suspendedProvider(provider, this.suspendedProviders.get(provider.id)))
      return
    }
    this.installProvider(provider)
  }

  connectToolSource(provider: CapabilityToolProvider): boolean {
    const suspendedReason = this.suspendedProviders.get(provider.id)
    if (suspendedReason) {
      this.removeProviderTools(provider.id)
      this.providers.set(provider.id, suspendedProvider(provider, suspendedReason))
      return false
    }
    this.assertNoDuplicateProviderTools(provider)
    this.removeProviderTools(provider.id)
    this.installProvider(provider)
    return true
  }

  disconnectToolSource(providerId: string, reason = 'disconnected'): boolean {
    const provider = this.providers.get(providerId)
    if (!provider) return false
    const disconnected: CapabilityToolProvider = {
      ...provider,
      available: false,
      reason
    }
    const policy = providerPolicy(disconnected)
    this.providers.set(providerId, disconnected)
    for (const record of this.tools.values()) {
      if (record.provider.id === providerId) {
        record.provider = policy
      }
    }
    return true
  }

  suspendToolSource(providerId: string, reason = 'suspended'): boolean {
    this.suspendedProviders.set(providerId, reason)
    const provider = this.providers.get(providerId)
    if (!provider) return false
    this.removeProviderTools(providerId)
    this.providers.set(providerId, suspendedProvider(provider, reason))
    return true
  }

  resumeToolSource(providerId: string): boolean {
    return this.suspendedProviders.delete(providerId)
  }

  private installProvider(provider: CapabilityToolProvider): void {
    this.assertNoDuplicateProviderTools(provider)
    this.providers.set(provider.id, provider)
    const policy = providerPolicy(provider)
    for (const tool of provider.tools) {
      this.tools.set(tool.name, { provider: policy, tool })
    }
  }

  private assertNoDuplicateProviderTools(provider: CapabilityToolProvider): void {
    for (const tool of provider.tools) {
      const existing = this.tools.get(tool.name)
      if (existing && existing.provider.id !== provider.id) {
        throw new Error(`duplicate tool name: ${tool.name}`)
      }
    }
  }

  listTools(context?: ToolHostContext): CapabilityToolSpec[] {
    return this.visibleToolRecords(context).map((record) => this.specForRecord(record))
  }

  contractEntries(context?: ToolHostContext): CapabilityToolContractEntry[] {
    return this.visibleToolRecords(context)
      .map((record) => this.contractEntryForRecord(record))
      .sort((a, b) => a.name.localeCompare(b.name) || a.providerId.localeCompare(b.providerId))
  }

  private visibleToolRecords(context?: ToolHostContext): CapabilityToolRecord[] {
    const records: CapabilityToolRecord[] = []
    for (const record of this.tools.values()) {
      if (!this.canUseProvider(record.provider, context)) continue
      if (!this.canUseTool(record.tool.name, context)) continue
      if (!isToolAdvertisedInSandbox(record.tool, context)) continue
      if (record.tool.shouldAdvertise) {
        if (!context || !record.tool.shouldAdvertise(context)) continue
      }
      records.push(record)
    }
    return records
  }

  private specForRecord(record: CapabilityToolRecord): CapabilityToolSpec {
    return {
      name: record.tool.name,
      description: record.tool.description,
      inputSchema: canonicalizeToolInputSchema(record.tool.inputSchema),
      toolKind: record.tool.toolKind,
      ...(record.tool.snipHint ? { snipHint: record.tool.snipHint } : {}),
      providerId: record.provider.id,
      providerKind: record.provider.kind
    }
  }

  private contractEntryForRecord(record: CapabilityToolRecord): CapabilityToolContractEntry {
    return {
      name: record.tool.name,
      description: record.tool.description,
      inputSchema: canonicalizeToolInputSchema(record.tool.inputSchema),
      toolKind: record.tool.toolKind,
      providerId: record.provider.id,
      providerKind: record.provider.kind,
      toolPolicy: record.tool.policy,
      providerEnabled: record.provider.enabled,
      providerAvailable: record.provider.available,
      ...(record.provider.reason ? { providerReason: record.provider.reason } : {})
    }
  }

  resolveTool(toolName: string, context: ToolHostContext, providerId?: string): CapabilityToolRecord {
    const record = this.tools.get(toolName)
    if (!record) {
      throw new Error(`unknown tool: ${toolName}`)
    }
    if (providerId && providerId !== record.provider.id) {
      throw new Error(`tool ${toolName} is not provided by ${providerId}`)
    }
    if (!this.canUseProvider(record.provider, context)) {
      throw new Error(`tool ${toolName} is not advertised by provider ${record.provider.id}`)
    }
    if (!this.canUseTool(toolName, context)) {
      throw new Error(`tool ${toolName} is not advertised by active tool policy`)
    }
    if (record.tool.shouldAdvertise && !record.tool.shouldAdvertise(context)) {
      throw new Error(`tool ${toolName} is not advertised in this turn context`)
    }
    return record
  }

  diagnostics(): CapabilityToolSourceDiagnostic[] {
    return [...this.providers.values()]
      .map((provider) => {
        const tools = this.toolsForProvider(provider.id)
        const catalog = buildToolCatalogFingerprint(tools)
        return {
          ...providerPolicy(provider),
          toolCount: tools.length,
          toolNames: catalog.toolNames,
          catalogFingerprint: catalog.fingerprint
        }
      })
      .sort((a, b) => a.id.localeCompare(b.id))
  }

  private removeProviderTools(providerId: string): void {
    for (const [toolName, record] of this.tools.entries()) {
      if (record.provider.id === providerId) {
        this.tools.delete(toolName)
      }
    }
  }

  private toolsForProvider(providerId: string): LocalTool[] {
    const tools: LocalTool[] = []
    for (const record of this.tools.values()) {
      if (record.provider.id === providerId) {
        tools.push(record.tool)
      }
    }
    return tools
  }

  private canUseProvider(provider: ToolProviderPolicy, context?: ToolHostContext): boolean {
    if (!provider.enabled || !provider.available) return false
    const allowed = context?.allowedProviderIds
    if (allowed && !allowed.includes(provider.id)) return false
    return true
  }

  private canUseTool(toolName: string, context?: ToolHostContext): boolean {
    if (isPlanModeContext(context) && !PLAN_MODE_ALLOWED_TOOL_NAMES.has(toolName)) {
      return false
    }
    const allowed = context?.allowedToolNames
    return !allowed || allowed.includes(toolName)
  }
}

function isPlanModeContext(context: ToolHostContext | undefined): boolean {
  return context?.threadMode === 'plan' || Boolean(context?.guiPlan)
}

function providerPolicy(provider: ToolProviderPolicy): ToolProviderPolicy {
  return {
    id: provider.id,
    kind: provider.kind,
    enabled: provider.enabled,
    available: provider.available,
    ...(provider.reason ? { reason: provider.reason } : {})
  }
}

function suspendedProvider(
  provider: CapabilityToolProvider,
  reason = 'suspended'
): CapabilityToolProvider {
  return {
    ...provider,
    available: false,
    reason
  }
}
