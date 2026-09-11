import type { ImmutablePrefix } from '../cache/immutable-prefix.js'
import type { TurnItem } from '../contracts/items.js'
import type { UsageSnapshot } from '../contracts/usage.js'
import type { ModelClient } from '../ports/model-client.js'
import type { ContextCompactionConfig } from '../shared/model-context-profile.js'

export const DEFAULT_COMPACTION_SUMMARY_TIMEOUT_MS = 15_000
export const DEFAULT_COMPACTION_SUMMARY_MAX_TOKENS = 1_200
export const DEFAULT_COMPACTION_SUMMARY_INPUT_MAX_BYTES = 96 * 1024

export async function summarizeCompactionWithModel(input: {
  threadId: string
  turnId: string
  model: string
  modelClient: ModelClient
  prefix: ImmutablePrefix
  contextCompaction?: ContextCompactionConfig
  items: TurnItem[]
  heuristicSummary: string
  signal: AbortSignal
  recordUsage?: (usage: UsageSnapshot) => Promise<void> | void
  recordFallback?: (message: string) => Promise<void> | void
}): Promise<string | undefined> {
  if (input.signal.aborted) return undefined
  const timeoutMs = Math.max(
    1,
    Math.floor(input.contextCompaction?.summaryTimeoutMs ?? DEFAULT_COMPACTION_SUMMARY_TIMEOUT_MS)
  )
  const controller = new AbortController()
  const onAbort = (): void => controller.abort()
  const timeout = setTimeout(() => controller.abort(), timeoutMs)
  input.signal.addEventListener('abort', onAbort, { once: true })
  let fallbackRecorded = false
  const recordFallback = async (message: string): Promise<void> => {
    if (fallbackRecorded || input.signal.aborted) return
    fallbackRecorded = true
    await input.recordFallback?.(message)
  }
  try {
    const requestItem = {
      id: `item_${input.turnId}_compaction_summary_request`,
      turnId: input.turnId,
      threadId: input.threadId,
      role: 'user' as const,
      status: 'completed' as const,
      createdAt: new Date().toISOString(),
      finishedAt: new Date().toISOString(),
      kind: 'user_message' as const,
      text: buildModelCompactionPrompt({
        items: input.items,
        heuristicSummary: input.heuristicSummary,
        maxBytes: input.contextCompaction?.summaryInputMaxBytes ?? DEFAULT_COMPACTION_SUMMARY_INPUT_MAX_BYTES
      })
    }
    let text = ''
    for await (const chunk of input.modelClient.stream({
      threadId: input.threadId,
      turnId: input.turnId,
      model: input.model,
      systemPrompt: input.prefix.systemPrompt,
      contextInstructions: [
        'Summarize context for a history fold. Preserve durable task state and omit transient chatter.'
      ],
      prefix: input.prefix.fewShots,
      history: [requestItem],
      tools: [],
      stream: true,
      maxTokens: Math.max(
        1,
        Math.floor(input.contextCompaction?.summaryMaxTokens ?? DEFAULT_COMPACTION_SUMMARY_MAX_TOKENS)
      ),
      temperature: 0,
      reasoningEffort: 'off',
      abortSignal: controller.signal
    })) {
      if (input.signal.aborted) return undefined
      if (controller.signal.aborted) {
        await recordFallback(
          `Model compaction summary timed out after ${timeoutMs}ms; using heuristic summary.`
        )
        return undefined
      }
      if (chunk.kind === 'assistant_text_delta') text += chunk.text
      if (chunk.kind === 'usage') await input.recordUsage?.(chunk.usage)
      if (chunk.kind === 'error') {
        await recordFallback(
          `Model compaction summary failed${chunk.code ? ` (${chunk.code})` : ''}: ${chunk.message}. Using heuristic summary.`
        )
        return undefined
      }
    }
    const summary = text.trim()
    if (!summary) {
      await recordFallback('Model compaction summary returned empty text; using heuristic summary.')
      return undefined
    }
    return summary
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    const reason = controller.signal.aborted && !input.signal.aborted
      ? `Model compaction summary timed out after ${timeoutMs}ms`
      : `Model compaction summary threw: ${message}`
    await recordFallback(`${reason}; using heuristic summary.`)
    return undefined
  } finally {
    clearTimeout(timeout)
    input.signal.removeEventListener('abort', onAbort)
  }
}

export function buildModelCompactionPrompt(input: {
  items: readonly TurnItem[]
  heuristicSummary: string
  maxBytes: number
}): string {
  const transcript = fitTextToBytes(
    input.items
      .map(compactionPromptLine)
      .filter((line) => line.length > 0)
      .join('\n'),
    Math.max(1_024, input.maxBytes)
  )
  return [
    'You are compacting a long agent conversation so work can continue past the context window.',
    'Write a dense, factual handoff summary using EXACTLY the following section headers, in this order.',
    'Keep every section; write "- (none)" when a section has no content. Use short bullets, not prose.',
    'Do not invent facts, do not add generic advice, and preserve concrete identifiers verbatim',
    '(file paths, function/variable names, commands, URLs, IDs, error messages).',
    '',
    '## Goal',
    "- The user's overall objective and any explicit requirements or constraints.",
    '## Completed',
    '- Work already done and decisions made, with the concrete outcome of each.',
    '## Key findings',
    '- Important facts discovered (root causes, data values, API shapes) needed to continue.',
    '## Files & locations',
    '- Files created/edited/inspected and the relevant paths or line ranges.',
    '## Tool & command results',
    '- Notable tool/command outcomes, especially errors and their resolution status.',
    '## Pending',
    '- Unresolved next steps and anything explicitly requested but not yet done.',
    '## Constraints & pins',
    '- Durable rules, user preferences, and active/pinned skills that must survive.',
    '',
    'Existing heuristic summary to cross-check (may be incomplete):',
    input.heuristicSummary.trim() || '(none)',
    '',
    'Conversation history to fold:',
    transcript || '(empty)'
  ].join('\n')
}

function compactionPromptLine(item: TurnItem): string {
  switch (item.kind) {
    case 'tool_progress':
      return ''
    case 'user_message':
      return `[user] ${clipForPrompt(item.text, 2_000)}`
    case 'assistant_text':
      return `[assistant] ${clipForPrompt(item.text, 2_000)}`
    case 'tool_call':
      return `[tool_call:${item.toolName}] ${clipForPrompt(stringifyForPrompt(item.arguments), 1_200)}`
    case 'tool_result':
      return `[tool_result:${item.toolName}${item.isError ? ':error' : ''}] ${clipForPrompt(stringifyForPrompt(item.output), 2_000)}`
    case 'approval':
      return `[approval:${item.status}:${item.toolName}] ${clipForPrompt(item.summary, 800)}`
    case 'user_input':
      return `[user_input:${item.status}] ${clipForPrompt(item.prompt, 800)}`
    case 'compaction':
      return (item.replacedTokens ?? 0) > 0 ? `[compaction] ${clipForPrompt(item.summary, 2_000)}` : ''
    case 'review':
      return `[review:${item.title}] ${clipForPrompt(item.reviewText || stringifyForPrompt(item.output), 2_000)}`
    case 'error':
      return `[error${item.code ? `:${item.code}` : ''}] ${clipForPrompt(item.message, 1_200)}`
  }
}

function stringifyForPrompt(value: unknown): string {
  if (typeof value === 'string') return value
  if (value == null) return ''
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

function clipForPrompt(text: string, maxChars: number): string {
  const compact = text.replace(/\s+/g, ' ').trim()
  if (compact.length <= maxChars) return compact
  return `${compact.slice(0, Math.max(0, maxChars - 3)).trim()}...`
}

function fitTextToBytes(text: string, maxBytes: number): string {
  if (Buffer.byteLength(text, 'utf8') <= maxBytes) return text
  let used = 0
  let out = ''
  for (const char of text) {
    const bytes = Buffer.byteLength(char, 'utf8')
    if (used + bytes > maxBytes) break
    out += char
    used += bytes
  }
  return `${out.trimEnd()}\n...[truncated for model compaction summary]`
}
