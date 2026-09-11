import { createHash } from 'node:crypto'
import type { TurnItem } from '../contracts/items.js'
import type { UsageSnapshot } from '../contracts/usage.js'
import type { ModelToolSpec } from '../ports/model-client.js'
import { canonicalizeToolInputSchema } from './tool-catalog-fingerprint.js'

const CHARS_PER_TOKEN = 4

export type CachePrefixShape = {
  prefixHash: string
  systemHash: string
  modeHash?: string
  prefixItemsHash: string
  toolsHash: string
  toolSchemaTokens: number
  toolCount?: number
  toolSourcesHash?: string
  toolSourceIds?: string[]
  route?: string
  provider?: string
  providerId?: string
  endpointFormat?: string
  model?: string
}

export type CacheDiagnostics = CachePrefixShape & {
  prefixChanged: boolean
  prefixChangeReasons: string[]
  toolSourceChanged: boolean
  toolSourceChangeReasons: string[]
  cacheTelemetrySupported: boolean
  firstTokenLatencyMs?: number
  durationMs?: number
  cacheHitTokens?: number
  cacheMissTokens?: number
}

export type ToolSourceCacheShape = {
  id: string
  kind: string
  status?: string
  catalogFingerprint?: string
}

export function captureCachePrefixShape(input: {
  systemPrompt?: string
  modeInstruction?: string
  prefix?: readonly TurnItem[]
  tools?: readonly ModelToolSpec[]
  toolSources?: readonly ToolSourceCacheShape[]
  toolCount?: number
  route?: string
  provider?: string
  providerId?: string
  endpointFormat?: string
  model?: string
}): CachePrefixShape {
  const tools = normalizeToolSpecs(input.tools ?? [])
  const toolSources = normalizeToolSources(input.toolSources ?? [])
  const toolSchemaText = JSON.stringify(tools)
  const system = input.systemPrompt ?? ''
  const mode = input.modeInstruction ?? ''
  const prefixItems = normalizePrefixItems(input.prefix ?? [])
  const diagnosticToolCount = input.toolCount ?? (input.route ? tools.length : undefined)
  const base = {
    system,
    mode,
    prefixItems,
    tools,
    provider: input.provider ?? '',
    providerId: input.providerId ?? '',
    endpointFormat: input.endpointFormat ?? '',
    model: input.model ?? ''
  }
  const toolSourcesHash = hashObject(toolSources)
  return {
    prefixHash: hashObject(base),
    systemHash: hashObject(system),
    ...(mode ? { modeHash: hashObject(mode) } : {}),
    prefixItemsHash: hashObject(prefixItems),
    toolsHash: hashObject(tools),
    toolSchemaTokens: estimateTokens(toolSchemaText),
    ...(diagnosticToolCount !== undefined ? { toolCount: diagnosticToolCount } : {}),
    ...(toolSources.length > 0
      ? {
          toolSourcesHash,
          toolSourceIds: toolSources.map((source) => source.id)
        }
      : {}),
    ...(input.route ? { route: input.route } : {}),
    ...(input.provider ? { provider: input.provider } : {}),
    ...(input.providerId ? { providerId: input.providerId } : {}),
    ...(input.endpointFormat ? { endpointFormat: input.endpointFormat } : {}),
    ...(input.model ? { model: input.model } : {})
  }
}

export function compareCachePrefixShapes(
  previous: CachePrefixShape,
  current: CachePrefixShape,
  usage: UsageSnapshot,
  timing?: {
    firstTokenLatencyMs?: number
    durationMs?: number
  }
): CacheDiagnostics {
  const prefixChangeReasons: string[] = []
  const toolSourceChangeReasons: string[] = []
  if (previous.systemHash !== current.systemHash) prefixChangeReasons.push('system')
  if ((previous.modeHash ?? '') !== (current.modeHash ?? '')) prefixChangeReasons.push('mode')
  if (previous.prefixItemsHash !== current.prefixItemsHash) prefixChangeReasons.push('prefix')
  if (previous.toolsHash !== current.toolsHash) prefixChangeReasons.push('tools')
  if (
    (previous.provider ?? '') !== (current.provider ?? '') ||
    (previous.providerId ?? '') !== (current.providerId ?? '') ||
    (previous.endpointFormat ?? '') !== (current.endpointFormat ?? '')
  ) {
    prefixChangeReasons.push('provider')
  }
  if ((previous.model ?? '') !== (current.model ?? '')) prefixChangeReasons.push('model')
  if ((previous.toolSourcesHash ?? '') !== (current.toolSourcesHash ?? '')) {
    toolSourceChangeReasons.push('tool_sources')
  }
  const cacheTelemetrySupported =
    typeof usage.cacheHitTokens === 'number' || typeof usage.cacheMissTokens === 'number'
  return {
    ...current,
    prefixChanged: prefixChangeReasons.length > 0,
    prefixChangeReasons,
    toolSourceChanged: toolSourceChangeReasons.length > 0,
    toolSourceChangeReasons,
    cacheTelemetrySupported,
    ...(isNonNegativeFiniteNumber(timing?.firstTokenLatencyMs)
      ? { firstTokenLatencyMs: Math.floor(timing.firstTokenLatencyMs) }
      : {}),
    ...(isNonNegativeFiniteNumber(timing?.durationMs)
      ? { durationMs: Math.floor(timing.durationMs) }
      : {}),
    ...(typeof usage.cacheHitTokens === 'number' ? { cacheHitTokens: usage.cacheHitTokens } : {}),
    ...(typeof usage.cacheMissTokens === 'number' ? { cacheMissTokens: usage.cacheMissTokens } : {})
  }
}

function isNonNegativeFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function normalizeToolSpecs(tools: readonly ModelToolSpec[]): Array<{
  name: string
  description: string
  inputSchema: Record<string, unknown>
}> {
  return [...tools]
    .map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: canonicalizeSchema(tool.inputSchema)
    }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

function canonicalizeSchema(value: unknown): Record<string, unknown> {
  return canonicalizeToolInputSchema(value)
}

function normalizeToolSources(sources: readonly ToolSourceCacheShape[]): ToolSourceCacheShape[] {
  return [...sources]
    .map((source) => ({
      id: source.id,
      kind: source.kind,
      ...(source.status ? { status: source.status } : {}),
      ...(source.catalogFingerprint ? { catalogFingerprint: source.catalogFingerprint } : {})
    }))
    .sort((a, b) => a.id.localeCompare(b.id))
}

function canonicalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalize)
  if (!value || typeof value !== 'object') return value
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(value as Record<string, unknown>).sort()) {
    out[key] = canonicalize((value as Record<string, unknown>)[key])
  }
  return out
}

function normalizePrefixItems(items: readonly TurnItem[]): unknown[] {
  return items.map(prefixItemCacheShape).filter((item) => item !== null)
}

function prefixItemCacheShape(item: TurnItem): unknown {
  switch (item.kind) {
    case 'user_message':
      return { kind: item.kind, text: item.text }
    case 'assistant_text':
      return { kind: item.kind, text: item.text }
    case 'tool_call':
      return {
        kind: item.kind,
        callId: item.callId,
        toolName: item.toolName,
        arguments: canonicalize(item.arguments)
      }
    case 'tool_result':
      return {
        kind: item.kind,
        callId: item.callId,
        output: canonicalize(item.output)
      }
    case 'approval':
    case 'user_input':
    case 'compaction':
    case 'review':
    case 'error':
      return null
  }
}

function estimateTokens(value: string): number {
  if (!value) return 0
  return Math.max(1, Math.ceil(value.length / CHARS_PER_TOKEN))
}

function hashObject(value: unknown): string {
  return createHash('sha256').update(JSON.stringify(canonicalize(value))).digest('hex').slice(0, 16)
}
