/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { TerminalBlockProps } from '../../TerminalBlock'
export const CHAT_TERMINAL_MAX_LINES = 8
export type TerminalCardModel = Pick<TerminalBlockProps, 'command' | 'output' | 'status' | 'exitCode'> & { durationMs: number; truncated: boolean }
/** Host outcome is authoritative. Missing exit markers/codes never imply exit zero. */
export function terminalCardModel(display: ToolResultLocalDisplayV1): TerminalCardModel | null {
  const c = display.capture, inspect = display.inspect
  if (c.kind !== 'shell' || ['failed', 'timeout', 'canceled'].includes(c.status) ||
      !inspect || inspect.inputTruncated || inspect.inputJSON === undefined) return null
  // The original request owns the command. A bounded capture label or public
  // summary cannot recover missing bytes; execution may trim this input.
  let input: unknown
  try { input = JSON.parse(inspect.inputJSON) } catch { return null }
  if (input === null || typeof input !== 'object' || Array.isArray(input)) return null
  const command = (input as Record<string, unknown>).command
  if (typeof command !== 'string' || command === '') return null
  return { command, output: c.body, status: c.status, exitCode: c.exitCode,
    durationMs: c.durationMs, truncated: c.truncated }
}
export function processTerminalCardModel(block: ProcessDisplayBlockV1): TerminalCardModel | null {
  // Only the actual foreground process source supplies this card. Tool errors
  // retain generic IN/OUT; output prose, truncation and tool names infer nothing.
  if (block.kind !== 'shell' || block.source.kind !== 'tool' || block.classification === 'withheld' ||
      !block.title || !['running', 'completed'].includes(block.state)) return null
  if (block.state === 'running') return { command: block.title, output: undefined, status: 'running',
    durationMs: block.durationMs, truncated: block.truncated }
  if (!block.body) return null
  return { command: block.title ?? '', output: block.body.text, status: block.state as TerminalCardModel['status'],
    exitCode: block.exitCode, durationMs: block.durationMs, truncated: block.truncated }
}
export function terminalFailed(model: TerminalCardModel): boolean {
  return ['failed', 'timeout'].includes(model.status) || model.exitCode !== undefined && model.exitCode !== 0
}
