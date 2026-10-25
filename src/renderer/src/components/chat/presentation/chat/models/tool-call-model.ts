/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ProcessDisplayBlockV1 } from '../../../../../../../../packages/runtime/src/contracts/protected-process-local-display'
import type { ToolBlock } from '../../../../../agent/types'
import { PublicToolResultProjectionV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
import type { ToolRowState } from '../ToolRow'

/** Only real Host states select lifecycle; prose and global processing do not. */
export function toolRowState(block: ToolBlock): ToolRowState {
  const projection = PublicToolResultProjectionV1.safeParse(block.meta?.publicResult)
  const status = projection.success ? projection.data.status : undefined
  if (block.status === 'running') return 'running'
  if (status === 'completed' && block.status === 'error') return 'error'
  if (status === 'cancelled') return 'stopped'
  if (status === 'unknown') return 'unknown'
  const diagnostics = block.meta?.diagnostics
  const diagnosticStatus = diagnostics && typeof diagnostics === 'object' && !Array.isArray(diagnostics)
    ? (diagnostics as Record<string, unknown>).status : undefined
  if (block.status === 'error' && ['canceled', 'aborted', 'stopped'].includes(String(diagnosticStatus))) return 'stopped'
  if (block.status === 'error' || status === 'failed' || status === 'blocked') return 'error'
  const exit = block.meta?.exit_code
  return block.toolKind === 'command_execution' && typeof exit === 'number' && Number.isSafeInteger(exit) && exit !== 0 ? 'error' : 'ok'
}
export function safeWebHref(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  try { return ['http:', 'https:'].includes(new URL(value).protocol) ? value : undefined } catch { return undefined }
}


export type ToolPreparationRowModelV1 = {
  callId: string; toolName: string; summary: string; inputKilobytes: number | undefined
  state: 'preparing'; expandable: false
}
/** The protected Host supplies the real summary. A sealed argument block does
 * not dispatch a tool, and no argument envelope is reconstructed in the leaf. */
export function toolPreparationRowModel(block: ProcessDisplayBlockV1): ToolPreparationRowModelV1 | null {
  if (block.kind !== 'tool_preparation' || block.source.kind !== 'tool_preparation' ||
    block.state !== 'preparing' || block.classification === 'withheld' || !block.toolName) return null
  const summary = block.path || block.preview || ''
  const count = block.inputUtf16CodeUnits
  const mutation = ['write', 'write_file', 'edit', 'edit_file', 'multi_edit'].includes(block.toolName)
  const inputKilobytes = summary && mutation && typeof count === 'number' && Number.isSafeInteger(count) && count >= 0
    ? Math.ceil(count / 1024) : undefined
  return { callId: block.source.callId, toolName: block.toolName, summary, inputKilobytes, state: 'preparing', expandable: false }
}
