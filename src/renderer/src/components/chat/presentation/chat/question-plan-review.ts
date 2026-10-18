/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import type { UserInputLocalDisplayV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'

// Only the signed private producer issues intent. Tool names, labels, order and
// public metadata do not create a PlanReview or a permission to approve it.
export function questionPlanReview(display: UserInputLocalDisplayV1 | null) {
  if (!display || display.questions.length !== 1) return null
  const question = display.questions[0], intent = question.intent
  if (intent?.kind !== 'plan-review' || question.detail === undefined || question.multiSelect !== false || question.options.length > 2) return null
  const approve = question.options.find(option => option.id === intent.approveOptionId)
  return approve ? { question, approve, plan: question.detail, callId: display.callId ?? null } : null
}
