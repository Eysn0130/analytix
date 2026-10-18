/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { ToolResultLocalDisplayV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ChatBlock, RuntimeChildMetadata, ToolBlock } from '../../../../../agent/types'
import type { ToolDetailsModel, ToolDetailItem } from './details-card-model'
import { toolRowState } from './tool-call-model'
import type { ToolRowState } from '../ToolRow'

type Translate = (key: string, options?: Record<string, unknown>) => string
const spawnNames = new Set(['task', 'delegate_task', 'parallel_tasks'])
export function subagentNativeIdentity(child: RuntimeChildMetadata): string {
  return JSON.stringify([child.parentThreadId, child.parentTurnId, child.parentToolCallId,
    child.childId, child.jobId, child.childRunId])
}
function badge(status: string, t: Translate): NonNullable<ToolDetailItem['badge']> {
  return { label: t('dshRecordedTool.jobStatus.' + status), tone: status === 'completed' ? 'success'
    : status === 'failed' ? 'error' : ['aborted', 'interrupted', 'killed', 'canceled', 'timeout'].includes(status) ? 'warning'
      : status === 'unknown' ? 'neutral' : 'info' }
}
/** The call and the child are independent observations. A failed call stays failed. */
export function subagentRowState(block: ToolBlock, child: RuntimeChildMetadata | null, captureState?: ToolRowState): ToolRowState {
  const state = toolRowState(block)
  if (state === 'error' || state === 'stopped' || state === 'unknown') return state
  if (captureState === 'error' || captureState === 'stopped' || captureState === 'unknown') return captureState
  if (state === 'running') return state
  if (!child) return captureState ?? state
  return child.childStatus === 'failed' ? 'error' : ['aborted', 'interrupted', 'killed'].includes(child.childStatus) ? 'stopped'
    : child.childStatus === 'completed' || child.childStatus === 'paused' ? 'ok' : 'running'
}
/** Native lifecycle fields never inherit the enclosing tool's completion. */
export function nativeSubagentCardModel(child: RuntimeChildMetadata | null, t: Translate): ToolDetailsModel {
  if (!child) return { items: [], empty: t('dshSubagent.receiptUnavailable', { defaultValue: 'Receipt unavailable' }) }
  const title = child.childLabel || child.childName || child.childId
  return { summary: title, items: [{ id: subagentNativeIdentity(child), title, badge: badge(child.childStatus, t), fields: [
    { label: t('dshSubagent.agent', { defaultValue: 'Agent' }), value: child.childId },
    ...(child.jobId || child.childRunId ? [{ label: t('dshRecordedTool.job.id'), value: child.jobId || child.childRunId! }] : [])
  ] }] }
}
/** Only a Host-authorized spawn capture can supply the private started receipt. */
export function subagentCardModel(block: ToolBlock, display: ToolResultLocalDisplayV1, t: Translate): ToolDetailsModel | null {
  const name = block.meta?.toolName
  if (block.toolKind !== 'subagent' || typeof name !== 'string' || !spawnNames.has(name) || display.toolName !== name ||
      display.capture.kind !== 'job' || display.capture.job.operation !== 'spawn') return null
  const capture = display.capture, data = capture.job
  const started = block.status === 'success' && capture.status === 'completed' && data.jobs.length > 0
  const callState = toolRowState(block)
  const receiptBadge = started ? { label: t('dshSubagent.started', { defaultValue: 'Started' }), tone: 'info' as const }
    : badge(callState === 'stopped' ? 'canceled' : callState === 'error' || capture.status === 'failed' ? 'failed' : 'unknown', t)
  // Single-call input belongs to this mounted lease. Parallel inputs have no
  // per-job association and cannot be paired by index, label or equal names.
  let prompt: string | undefined
  if (name !== 'parallel_tasks' && data.jobs.length === 1 && display.inspect?.inputJSON) {
    try {
      const args: unknown = JSON.parse(display.inspect.inputJSON)
      if (args && typeof args === 'object' && !Array.isArray(args) && 'prompt' in args && typeof args.prompt === 'string') prompt = args.prompt
    } catch { /* No argument reconstruction or public fallback. */ }
  }
  const items = data.jobs.map<ToolDetailItem>(job => ({ id: job.jobId, title: prompt || job.name || job.label || job.jobId,
    badge: receiptBadge,
    fields: [
      { label: t('dshRecordedTool.job.id'), value: job.jobId },
      { label: t('dshRecordedTool.job.kind'), value: t('dshRecordedTool.jobKind.' + job.kind) },
      { label: t('dshSubagent.recordedStatus', { defaultValue: 'Recorded status' }), value: badge(job.status, t).label },
      ...(job.name !== null ? [{ label: t('dshRecordedTool.job.name'), value: job.name }] : []),
      ...(job.label !== null ? [{ label: t('dshRecordedTool.job.label'), value: job.label }] : [])
    ], description: t('dshRecordedTool.job.outputUnavailable') }))
  return { items, summary: items.length === 1 ? items[0].title : items.length > 0 ? t('dshRecordedTool.job.count', { count: data.total })
    : t('dshSubagent.receiptUnavailable', { defaultValue: 'Receipt unavailable' }),
    expandedSummary: started ? t('dshSubagent.started', { defaultValue: 'Started' }) : undefined,
    empty: t('dshSubagent.receiptUnavailable', { defaultValue: 'Receipt unavailable' }),
    caption: data.truncated ? t('dshRecordedTool.job.listTruncated', { count: items.length, total: data.total }) : undefined }
}
/** Public summary contains only the actual public lifecycle or safe call failure. */
export function subagentPublicSummary(block: ChatBlock, child: RuntimeChildMetadata | null, t: Translate): string {
  if (block.kind === 'tool') {
    const state = toolRowState(block)
    if (state === 'stopped') return t('dshTurnStopped')
    if (state === 'error') return t('subagentStatusFailed', { defaultValue: 'Failed' })
  }
  return child ? `${child.childLabel || child.childName || child.childId} · ${badge(child.childStatus, t).label}`
    : t('dshSubagent.receiptUnavailable', { defaultValue: 'Receipt unavailable' })
}
