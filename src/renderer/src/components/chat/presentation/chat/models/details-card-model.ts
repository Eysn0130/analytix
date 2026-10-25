/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolBlock } from '../../../../../agent/types'
import { PublicToolResultProjectionV1 } from '../../../../../../../../packages/runtime/src/contracts/items'
export type ToolDetailItem = {
  id?: string
  title?: string
  subtitle?: string
  description?: string
  badge?: { label: string; tone: 'neutral' | 'info' | 'success' | 'warning' | 'error' }
  status?: { value: 'completed' | 'in_progress' | 'pending' | 'failed' | 'canceled'; label: string }
  previousStatus?: string
  change?: { value: 'added' | 'removed' | 'updated'; label: string }
  fields: readonly { label: string; value: string }[]
}
export type ToolDetailsModel = {
  items: readonly ToolDetailItem[]
  summary?: string
  expandedSummary?: string
  empty?: string
  caption?: string
  unchanged?: { label: string; items: readonly ToolDetailItem[] }
}
type Translate = (key: string) => string
/** Adapt every currently published Host result branch. No raw-JSON inspection. */
export function detailsCardModel(block: ToolBlock, t: Translate): ToolDetailsModel | null {
  const parsed = PublicToolResultProjectionV1.safeParse(block.meta?.publicResult)
  if (!parsed.success) return null
  const p = parsed.data
  if (p.status === 'completed' && block.status !== 'success') return null
  const field = (label: string, value: unknown) => ({ label: t('dshToolDetail.' + label), value: String(value) })
  const fields = [field('status', p.status), field('message', p.messageKey)]
  if (p.code !== undefined) fields.push(field('code', p.code))
  switch (p.projectionKind) {
    case 'plan_status': fields.push(field('id', p.plan.planId), field('path', p.plan.relativePath), field('operation', p.plan.operation),
      field('bytes', p.plan.byteSize), field('hash', p.plan.contentHash), field('savedAt', p.plan.savedAt)); break
    case 'mcp_diagnostic': fields.push(field('code', p.rpcError.code), field('rpcClass', p.rpcError.class), field('dataPresent', p.rpcError.dataPresent)); break
    case 'artifact_status': fields.push(field('id', p.artifact.artifactId), field('operation', p.artifact.kind),
      field('bytes', p.artifact.byteSize), field('hash', p.artifact.contentHash), field('savedAt', p.artifact.savedAt)); break
    case 'host_status': case 'withheld': case 'case_source_status': break
  }
  return { items: [{ fields }] }
}
