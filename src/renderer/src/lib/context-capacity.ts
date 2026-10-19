import type { ThreadUsageSnapshot } from '../agent/types'
import type { ModelProviderModelGroup } from '@shared/analytix-api'

/** Resolve only the selected provider/model; an unknown identity has no capacity reading. */
export function resolveComposerContextWindow(options: {
  model: string
  providerId?: string
  groups: readonly ModelProviderModelGroup[]
  runtimeModel?: { id: string; providerId?: string; contextWindowTokens?: number }
}): number | undefined {
  const key = (value: string) => value.trim().toLowerCase()
  const valid = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0
  const runtime = options.runtimeModel
  const providerId = options.providerId?.trim() || runtime?.providerId?.trim()
  const group = options.groups.find(item => item.providerId === providerId)
  const profileEntry = (model: string) => Object.entries(group?.modelProfiles ?? {}).find(([id, profile]) =>
    key(id) === key(model) || profile.aliases?.some(alias => key(alias) === key(model)))
  const selected = profileEntry(options.model)
  if (valid(selected?.[1].contextWindowTokens)) return selected[1].contextWindowTokens
  if (!runtime || !providerId || runtime.providerId?.trim() !== providerId) return undefined
  const runtimeEntry = profileEntry(runtime.id)
  const sameModel = key(options.model) === 'auto' || key(options.model) === key(runtime.id) ||
    selected !== undefined && runtimeEntry !== undefined && key(selected[0]) === key(runtimeEntry[0])
  return sameModel && valid(runtime.contextWindowTokens) ? runtime.contextWindowTokens : undefined
}

/** The renderer has no measured System / Tools / Messages token breakdown. */
export type ContextCategoryKey = 'system' | 'tools' | 'messages'

export type ContextRequestSample = {
  inputTokens: number | null
  model?: string
  providerId?: string
  /** Request-side UTF-8 bytes ÷ 4 estimate; never a share of the input sample. */
  toolDefinitionTokens: number | null
}

export type ContextCapacity = {
  /** Metadata for the current selection, not the historical sample's window. */
  windowTokens: number | null
  model?: string
  providerId?: string
  lastRequest: ContextRequestSample | null
}

export type ContextCapacityInput = {
  windowTokens?: number
  model?: string
  providerId?: string
  threadId: string | null
  lastTurnUsage: {
    threadId: string
    snapshot: Pick<ThreadUsageSnapshot, 'inputTokens' | 'model' | 'providerId' | 'cacheDiagnostics'>
  } | null
}

const positiveTokenCount = (value: unknown): value is number =>
  typeof value === 'number' && Number.isSafeInteger(value) && value > 0

/**
 * Keep historical request observations separate from selected-window metadata.
 * Snapshots have no request-time window, so neither occupancy nor remaining
 * tokens can be derived, even when the model and provider identities match.
 */
export function buildContextCapacity(input: ContextCapacityInput): ContextCapacity {
  const snapshot = input.threadId && input.lastTurnUsage?.threadId === input.threadId
    ? input.lastTurnUsage.snapshot : null
  const diagnostics = snapshot?.cacheDiagnostics
  const toolTokens = diagnostics?.toolSchemaTokens
  const toolDefinitionTokens = diagnostics?.toolSchemaEstimator === 'utf8_bytes_div4' &&
    typeof toolTokens === 'number' && Number.isSafeInteger(toolTokens) && toolTokens >= 0
    ? toolTokens : null

  return {
    windowTokens: positiveTokenCount(input.windowTokens) ? input.windowTokens : null,
    model: input.model?.trim() || undefined,
    providerId: input.providerId?.trim() || undefined,
    lastRequest: snapshot ? {
      // The mapper defaults missing input counters to zero; zero has no presence proof.
      inputTokens: positiveTokenCount(snapshot.inputTokens) ? snapshot.inputTokens : null,
      model: snapshot.model?.trim() || undefined,
      providerId: snapshot.providerId?.trim() || undefined,
      toolDefinitionTokens
    } : null
  }
}
