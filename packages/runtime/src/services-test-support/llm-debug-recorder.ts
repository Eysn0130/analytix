import type { UsageSnapshot } from '../contracts/usage.js'

/**
 * One captured LLM round: the literal HTTP request body sent to the model
 * and the accumulated raw output streamed back. Kept in-memory only so the
 * troubleshooting view can show the last few requests without persisting
 * full prompts/history to disk.
 */
export type LlmDebugRound = {
  id: number
  threadId: string
  turnId: string
  provider: string
  model: string
  /** Redacted endpoint URL the request was sent to. */
  url: string
  startedAt: string
  finishedAt: string
  durationMs: number
  /** The exact JSON body POSTed to the model (system, messages, tools, ...). */
  requestBody: Record<string, unknown> | null
  output: LlmDebugOutput
}

export type LlmDebugToolCall = {
  callId: string
  toolName: string
  arguments: Record<string, unknown>
}

export type LlmDebugOutput = {
  text: string
  reasoning: string
  toolCalls: LlmDebugToolCall[]
  usage?: UsageSnapshot
  stopReason?: string
  error?: string
}

export type LlmDebugRoundMeta = {
  threadId: string
  turnId: string
  provider: string
  model: string
}

/**
 * Narrow sink the model client depends on. The client calls {@link start}
 * before sending, mutates the returned round's `requestBody`/`output` as it
 * streams, then calls {@link finish} once the stream ends.
 */
export interface LlmDebugSink {
  start(meta: LlmDebugRoundMeta): LlmDebugRound
  finish(round: LlmDebugRound): void
}

/** Number of most-recent rounds retained for troubleshooting. */
const CAPACITY = 25

/**
 * Fixed-size in-memory ring buffer of the most recent LLM rounds. Nothing is
 * written to disk; the buffer is cleared on process restart. Shared across all
 * turns (main chat and review) via the single model-client instance.
 */
export class LlmDebugRecorder implements LlmDebugSink {
  private readonly rounds: LlmDebugRound[] = []
  private nextId = 1

  start(meta: LlmDebugRoundMeta): LlmDebugRound {
    const startedAt = new Date().toISOString()
    return {
      id: this.nextId++,
      threadId: meta.threadId,
      turnId: meta.turnId,
      provider: meta.provider,
      model: meta.model,
      url: '',
      startedAt,
      finishedAt: startedAt,
      durationMs: 0,
      requestBody: null,
      output: { text: '', reasoning: '', toolCalls: [] }
    }
  }

  finish(round: LlmDebugRound): void {
    round.finishedAt = new Date().toISOString()
    round.durationMs = Math.max(0, Date.parse(round.finishedAt) - Date.parse(round.startedAt))
    this.rounds.push(round)
    while (this.rounds.length > CAPACITY) this.rounds.shift()
  }

  /** Most-recent-first copy of the retained rounds. */
  snapshot(): LlmDebugRound[] {
    return [...this.rounds].reverse()
  }

  clear(): void {
    this.rounds.length = 0
  }
}
