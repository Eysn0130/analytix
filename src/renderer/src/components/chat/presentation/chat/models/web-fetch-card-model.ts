/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolBlock } from '../../../../../agent/types'
import { toolResultLocalDisplaySchemaV1, type ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'

export type WebFetchCardModel = { kind: 'fetch'; url: string; statusCode: number; truncated: boolean }

/** The authorized snapshot, never the public metadata, supplies the Fetch card. */
export function webFetchCardModel(block: ToolBlock, display: ToolResultLocalDisplayV1): WebFetchCardModel | null {
  if (block.meta?.toolName !== 'web_fetch' || block.toolKind !== 'tool_call' || block.status === 'running' || block.meta?.child !== undefined ||
      block.meta?.parentCallId !== undefined || ('parentCallId' in block && block.parentCallId !== undefined)) return null
  const parsed = toolResultLocalDisplaySchemaV1.safeParse(display)
  if (!parsed.success || parsed.data.toolName !== 'web_fetch' || parsed.data.capture.kind !== 'web_fetch') return null
  const web = parsed.data.capture.web
  // A complete non-2xx response is still a fetched response. A transport or
  // partial-read failure has real Output, but cannot supply a response card.
  if (!web.responseComplete || web.finalUrl === undefined || web.statusCode === undefined || web.fetchTruncated === undefined) return null
  return { kind: 'fetch', url: web.finalUrl, statusCode: web.statusCode, truncated: web.fetchTruncated }
}
