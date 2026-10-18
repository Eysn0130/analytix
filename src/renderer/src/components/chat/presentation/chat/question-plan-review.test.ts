import { describe, expect, it } from 'vitest'
import type { UserInputLocalDisplayV1 } from '../../../../../../../packages/runtime/src/contracts/user-input-local-display'
import { questionPlanReview } from './question-plan-review'
const requestId = 'input_' + 'd'.repeat(64), questionId = requestId + '_1'
function display(): UserInputLocalDisplayV1 {
  return { schemaVersion: 1, threadId: 'thread-plan', turnId: 'turn-plan', itemId: 'item_' + requestId, requestId,
    status: 'pending', callId: 'actual-plan-call', answerState: 'pending', questions: [{ id: questionId, header: 'Plan review', question: 'Original decision', detail: '# Complete plan', multiSelect: false,
      options: [{ id: questionId + '_option_1', label: 'Keep planning', description: '' }, { id: questionId + '_option_2', label: 'Actual approval', description: '' }],
      intent: { kind: 'plan-review', approveOptionId: questionId + '_option_2' } }] }
}
describe('signed private PlanReview narrowing', () => {
  it('uses the supplied actual option identity rather than labels or ordinal guesses', () => {
    const source = display(), review = questionPlanReview(source)!
    expect(review.approve.id).toBe(questionId + '_option_2'); expect(review.plan).toBe(source.questions[0].detail)
  })
  it('keeps a declared empty original plan in the PlanReview branch, as in the pinned DSH selector', () => {
    const source = display(); source.questions[0].detail = ''
    expect(questionPlanReview(source)?.plan).toBe('')
  })
  it.each(['no-intent', 'no-detail', 'multi', 'many-options', 'foreign-option', 'many-questions'] as const)('retains the ordinary Question branch for %s', reason => {
    const source = display(), question = source.questions[0]
    if (reason === 'no-intent') delete question.intent
    if (reason === 'no-detail') delete question.detail
    if (reason === 'multi') question.multiSelect = true
    if (reason === 'many-options') question.options.push({ id: questionId + '_option_3', label: 'Third', description: '' })
    if (reason === 'foreign-option') question.intent!.approveOptionId = requestId + '_2_option_1'
    if (reason === 'many-questions') source.questions.push({ ...question, id: requestId + '_2' })
    expect(questionPlanReview(source)).toBeNull()
  })
})
