import type { ActiveMemoryRecord } from '../contracts/memory.js'

const DEFAULT_MAX_MEMORY_CONTEXT_CHARS = 4_000
const DEFAULT_MAX_MEMORY_ITEM_CHARS = 700
const DEFAULT_MAX_MEMORY_REFERENCES = 8
const MIN_CLIPPED_MEMORY_CONTENT_CHARS = 80
const TRUNCATION_MARKER = '...[truncated]'

export type MemoryContextCompilerOptions = {
  maxContextChars?: number
  maxItemChars?: number
  maxReferences?: number
}

export type CompiledMemoryContext = {
  instructions: string[]
  memoryIds: string[]
  omittedMemoryIds: string[]
  originalCount: number
  retainedCount: number
  duplicateCount: number
  totalChars: number
  truncated: boolean
}

type MemoryContextRecord = Pick<ActiveMemoryRecord, 'id' | 'content' | 'scope' | 'confidence' | 'tags'>

// Adapted from DeepSeek-Reasonix internal/memorycompiler/compression.go:
// retain bounded causal anchors, fold noisy duplicates, and keep the compiled
// memory payload out of the stable system prefix.
export function compileMemoryContext(
  memories: readonly MemoryContextRecord[],
  options: MemoryContextCompilerOptions = {}
): CompiledMemoryContext {
  const maxContextChars = positiveInt(options.maxContextChars, DEFAULT_MAX_MEMORY_CONTEXT_CHARS)
  const maxItemChars = positiveInt(options.maxItemChars, DEFAULT_MAX_MEMORY_ITEM_CHARS)
  const maxReferences = positiveInt(options.maxReferences, DEFAULT_MAX_MEMORY_REFERENCES)
  const header = 'Relevant long-term memories for this turn:'
  const seen = new Set<string>()
  const lines: string[] = []
  const memoryIds: string[] = []
  const omittedMemoryIds: string[] = []
  let duplicateCount = 0
  let truncated = false
  let totalChars = header.length

  for (const memory of memories) {
    const normalized = normalizeMemoryContent(memory.content)
    if (!normalized) {
      omittedMemoryIds.push(memory.id)
      continue
    }
    const duplicateKey = `${memory.scope}:${normalized}`
    if (seen.has(duplicateKey)) {
      duplicateCount += 1
      omittedMemoryIds.push(memory.id)
      continue
    }
    seen.add(duplicateKey)
    if (memoryIds.length >= maxReferences) {
      omittedMemoryIds.push(memory.id)
      continue
    }

    const prefix = `- [${memory.id}] (${memory.scope}) `
    const contentBudget = Math.max(0, maxItemChars - prefix.length)
    let content = clipMemoryText(memory.content.trim(), contentBudget)
    if (content !== memory.content.trim()) truncated = true

    let line = `${prefix}${content}`
    const newlineCost = lines.length === 0 ? 1 : 1
    if (totalChars + newlineCost + line.length > maxContextChars) {
      const remaining = maxContextChars - totalChars - newlineCost - prefix.length
      if (remaining < MIN_CLIPPED_MEMORY_CONTENT_CHARS) {
        omittedMemoryIds.push(memory.id)
        continue
      }
      content = clipMemoryText(memory.content.trim(), remaining)
      line = `${prefix}${content}`
      truncated = true
    }

    lines.push(line)
    memoryIds.push(memory.id)
    totalChars += newlineCost + line.length
  }

  return {
    instructions: lines.length ? [[header, ...lines].join('\n')] : [],
    memoryIds,
    omittedMemoryIds,
    originalCount: memories.length,
    retainedCount: memoryIds.length,
    duplicateCount,
    totalChars: lines.length ? totalChars : 0,
    truncated
  }
}

function positiveInt(value: number | undefined, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  const normalized = Math.floor(value)
  return normalized > 0 ? normalized : fallback
}

function clipMemoryText(text: string, maxChars: number): string {
  if (maxChars <= 0) return ''
  if (text.length <= maxChars) return text
  if (maxChars <= TRUNCATION_MARKER.length + 1) return text.slice(0, maxChars)
  return `${text.slice(0, maxChars - TRUNCATION_MARKER.length).trimEnd()}${TRUNCATION_MARKER}`
}

function normalizeMemoryContent(content: string): string {
  return content
    .toLowerCase()
    .replace(/\s+/g, ' ')
    .trim()
}
