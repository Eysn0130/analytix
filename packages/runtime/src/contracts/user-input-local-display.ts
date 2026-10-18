import { z } from 'zod'
import { UserInputTimedWaitSchemaV1, UserInputLateReplyDeliverySchemaV1, UserInputActionSelectorSchemaV1 } from './approvals.js'

// Private native display only. Never embed this payload in public items,
// events, replay, model context or generic runtime response handling.
export const USER_INPUT_LOCAL_DISPLAY_PATH_V1 = '/v1/local-display/user-input'
export const USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1 = (8 << 20) + (64 << 10)
const recordId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const text = z.string().refine(value => !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= (4 << 20))
const inputId = z.string().regex(/^input_[a-f0-9]{64}$/)
const questionId = z.string().regex(/^input_[a-f0-9]{64}_[1-3]$/)
const optionId = z.string().regex(/^input_[a-f0-9]{64}_[1-3]_option_[1-3]$/)
export const userInputLocalSelectorSchemaV1 = UserInputActionSelectorSchemaV1
export const userInputLocalViewRequestSchemaV1 = z.object({
  threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  viewId: z.string().uuid()
}).strict().refine(value => value.itemId === 'item_' + value.requestId)
export const userInputLocalQuestionSchemaV1 = z.object({
  id: questionId, header: text, question: text.refine(value => value.trim() !== ''),
  detail: text.optional(), multiSelect: z.boolean(),
  // Issued only from the original signed exit_plan_mode call.
  intent: z.object({ kind: z.literal('plan-review'), approveOptionId: optionId }).strict().nullable().optional(),
  options: z.array(z.object({ id: optionId, label: text.refine(value => value.trim() !== ''), description: text }).strict()).max(3)
}).strict()
const structuredAnswer = z.union([
  z.object({ id: questionId, kind: z.literal('answer'), optionIds: z.array(optionId).max(3), customText: text.refine(value => value.trim() !== '').optional() }).strict(),
  z.object({ id: questionId, kind: z.literal('skip') }).strict()
])
export const userInputLocalSubmissionSchemaV1 = z.union([
  z.object({ schemaVersion: z.literal(1), format: z.literal('structured'), answers: z.array(structuredAnswer).min(1).max(3) }).strict(),
  z.object({ schemaVersion: z.literal(1), format: z.literal('legacy'), answers: z.array(z.object({
    id: text.refine(value => value.trim() !== ''), questionId, label: text.optional(), value: text.refine(value => value.trim() !== '')
  }).strict()).min(1).max(3) }).strict()
]).refine(value => new TextEncoder().encode(JSON.stringify(value.answers)).length <= (4 << 20), 'Private submission size exceeded.')

const draftRevisionV1 = z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
// Go's signed canonical JSON escapes these code points. Match that byte
// budget so a TS-valid draft cannot exceed the unchanged Host snapshot cap.
const privateDraftJSONBytesV1 = (value: unknown) => new TextEncoder().encode(JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, character => '\\u' + character.charCodeAt(0).toString(16).padStart(4, '0'))).length
export const userInputLocalDraftSnapshotSchemaV1 = z.object({
  // Display choice only; it neither renews Host authority nor changes its deadline.
  wait: z.enum(['waiting', 'editing']).optional(),
  currentPage: z.number().int().min(0).max(2),
  answers: z.array(z.object({
    questionId, optionIds: z.array(optionId).max(3), customText: text, skipped: z.boolean()
  }).strict()).max(3)
}).strict().superRefine((value, context) => {
  const invalid = () => context.addIssue({ code: 'custom', message: 'Invalid private draft snapshot.' })
  if (privateDraftJSONBytesV1(value) > (4 << 20) ||
      new Set(value.answers.map(answer => answer.questionId)).size !== value.answers.length) invalid()
  for (const answer of value.answers) {
    if (new Set(answer.optionIds).size !== answer.optionIds.length ||
        answer.optionIds.some(id => !id.startsWith(answer.questionId + '_option_')) ||
        answer.skipped && (answer.optionIds.length !== 0 || answer.customText !== '')) invalid()
  }
})
export type UserInputLocalDraftSnapshotV1 = z.infer<typeof userInputLocalDraftSnapshotSchemaV1>

export const userInputLocalDisplaySchemaV1 = z.object({
  schemaVersion: z.literal(1), threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  status: z.enum(['pending', 'submitted', 'cancelled', 'continued']),
  // Issued only after Host verification of the complete inherited prefix.
  readOnly: z.literal(true).optional(),
  callId: z.string().min(1).max(1024).refine(value => value.trim() === value).nullable().optional(),
  timedWait: UserInputTimedWaitSchemaV1.nullable().optional(),
  continuedReason: z.enum(['user_input_continued', 'user_input_outcome_unknown_after_restart']).nullable().optional(),
  lateReply: UserInputLateReplyDeliverySchemaV1.nullable().optional(),
  // Host-proved association to an existing immutable child reply, never delivery state.
  historyReply: z.object({ itemId: recordId,
    clientUserMessageId: UserInputLateReplyDeliverySchemaV1.shape.clientUserMessageId,
    turnId: recordId }).strict().nullable().optional(),
  draftRevision: draftRevisionV1.nullable().optional(),
  draft: userInputLocalDraftSnapshotSchemaV1.nullable().optional(),
  questions: z.array(userInputLocalQuestionSchemaV1).min(1).max(3),
  answerState: z.enum(['pending', 'available', 'unavailable', 'cancelled']),
  submission: userInputLocalSubmissionSchemaV1.optional()
}).strict().superRefine((value, context) => {
  const invalid = () => context.addIssue({ code: 'custom', message: 'Invalid private question binding.' })
  if (privateDraftJSONBytesV1(value) > USER_INPUT_LOCAL_DISPLAY_MAX_BYTES_V1) invalid()
  if (value.itemId !== 'item_' + value.requestId || value.questions.some((question, index) =>
    question.id !== `${value.requestId}_${index + 1}` || question.options.some((option, ordinal) =>
      option.id !== `${question.id}_option_${ordinal + 1}`))) invalid()
  if (value.readOnly && (value.status === 'pending' || value.draft != null || value.draftRevision != null || value.lateReply != null)) invalid()
  if (value.status === 'continued' ? !['available', 'unavailable'].includes(value.answerState) || !value.readOnly && (!value.timedWait || value.timedWait.timeoutSeconds === -1 && value.continuedReason !== 'user_input_outcome_unknown_after_restart') : value.status === 'submitted' ? !['available', 'unavailable'].includes(value.answerState) : value.answerState !== value.status) invalid()
  if (value.continuedReason != null && value.status !== 'continued') invalid()
  for (const question of value.questions) {
    if (question.intent && (!value.callId || !question.detail?.trim() || question.multiSelect ||
        !question.options.some(option => option.id === question.intent!.approveOptionId))) invalid()
  }
  if (value.readOnly && value.status === 'continued' && value.answerState === 'available' && !value.historyReply) invalid()
  if (value.historyReply && (!value.readOnly || value.status !== 'continued' || value.answerState !== 'available' ||
      !value.submission || value.historyReply.itemId === value.itemId || value.timedWait != null || value.continuedReason != null)) invalid()
  if (value.lateReply && (value.status !== 'continued' || value.answerState !== 'available')) invalid()
  // Raw capture can remain editable after a failed admission. Only lateReply proves delivery.
  if ((['submitted', 'continued'].includes(value.status) && value.answerState === 'available') !== (value.submission !== undefined)) invalid()
  if (value.draft) {
    if (!value.draftRevision || !['pending', 'continued'].includes(value.status) ||
        value.lateReply && value.lateReply.state !== 'discarded' || value.draft.currentPage >= value.questions.length) invalid()
    for (const answer of value.draft.answers) {
      const question = value.questions.find(item => item.id === answer.questionId)
      if (!question || answer.optionIds.some(id => !question.options.some(option => option.id === id)) ||
          !question.multiSelect && answer.optionIds.length + Number(answer.customText.trim() !== '') > 1) invalid()
    }
  }
  if (!value.submission) return
  const answers = value.submission.answers
  const ids = answers.map(answer => 'questionId' in answer ? answer.questionId : answer.id)
  if (answers.length !== value.questions.length || new Set(ids).size !== ids.length ||
      ids.some(id => !value.questions.some(question => question.id === id))) { invalid(); return }
  if (value.submission.format !== 'structured') return
  for (const answer of value.submission.answers) {
    if (answer.kind === 'skip') continue
    const question = value.questions.find(item => item.id === answer.id)!
    if (new Set(answer.optionIds).size !== answer.optionIds.length ||
        answer.optionIds.some(id => !question.options.some(option => option.id === id)) ||
        !answer.optionIds.length && answer.customText === undefined ||
        !question.multiSelect && answer.optionIds.length + Number(answer.customText !== undefined) > 1) invalid()
  }
})
export const userInputLocalViewResultSchemaV1 = z.union([
  z.object({ ok: z.literal(true), display: userInputLocalDisplaySchemaV1 }).strict(),
  z.object({ ok: z.literal(false), code: z.literal('unavailable') }).strict()
])
export type UserInputLocalSelectorV1 = z.infer<typeof userInputLocalSelectorSchemaV1>
export type UserInputLocalViewRequestV1 = z.infer<typeof userInputLocalViewRequestSchemaV1>
export type UserInputLocalDisplayV1 = z.infer<typeof userInputLocalDisplaySchemaV1>
export type UserInputLocalQuestionV1 = z.infer<typeof userInputLocalQuestionSchemaV1>
export type UserInputLocalSubmissionV1 = z.infer<typeof userInputLocalSubmissionSchemaV1>
export type UserInputLocalViewResultV1 = z.infer<typeof userInputLocalViewResultSchemaV1>
export interface UserInputLocalDisplayApiV1 {
  openUserInputLocalDisplay: (request: UserInputLocalViewRequestV1) => Promise<UserInputLocalViewResultV1>
  closeUserInputLocalDisplay: (viewId: string) => Promise<void>
  invalidateUserInputLocalDisplay: () => Promise<void>
  onUserInputLocalDisplayInvalidated: (handler: (viewId: string | null) => void) => () => void
}


// Per-attempt command on the existing private route. The original Question
// turn and the target steering turn are independent Host correlation keys.
export const userInputLocalDiscardLateReplyRequestSchemaV1 = z.object({
  threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  action: z.literal('discardLateReply'),
  clientUserMessageId: z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/),
  expectedTurnId: recordId
}).strict().refine(value => value.itemId === 'item_' + value.requestId)
export const userInputLocalDiscardLateReplyResponseSchemaV1 = z.object({
  schemaVersion: z.literal(1), threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  status: z.literal('continued'),
  lateReply: UserInputLateReplyDeliverySchemaV1.extend({ state: z.literal('discarded') })
}).strict().refine(value => value.itemId === 'item_' + value.requestId)
export type UserInputLocalDiscardLateReplyRequestV1 = z.infer<typeof userInputLocalDiscardLateReplyRequestSchemaV1>
export type UserInputLocalDiscardLateReplyResponseV1 = z.infer<typeof userInputLocalDiscardLateReplyResponseSchemaV1>

// An unfinished snapshot stays private. A save ACK carries no answer, queue
// admission or execution status. The retained cursor survives content clearing.
export const userInputLocalSaveDraftRequestSchemaV1 = z.object({
  threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  action: z.literal('saveDraft'), revision: draftRevisionV1,
  expectedRevision: draftRevisionV1.nullable(), draft: userInputLocalDraftSnapshotSchemaV1
}).strict().superRefine((value, context) => {
  if (value.itemId !== 'item_' + value.requestId || value.expectedRevision === value.revision ||
      value.draft.answers.some(answer => !answer.questionId.startsWith(value.requestId + '_'))) {
    context.addIssue({ code: 'custom', message: 'Invalid private draft request binding.' })
  }
})
export const userInputLocalSaveDraftResponseSchemaV1 = z.object({
  schemaVersion: z.literal(1), threadId: recordId, turnId: recordId,
  itemId: z.string().regex(/^item_input_[a-f0-9]{64}$/), requestId: inputId,
  status: z.enum(['pending', 'continued']), draftRevision: draftRevisionV1,
  draft: userInputLocalDraftSnapshotSchemaV1
}).strict().superRefine((value, context) => {
  if (value.itemId !== 'item_' + value.requestId || value.draft.answers.some(answer => !answer.questionId.startsWith(value.requestId + '_'))) {
    context.addIssue({ code: 'custom', message: 'Invalid private draft response binding.' })
  }
})
export type UserInputLocalSaveDraftRequestV1 = z.infer<typeof userInputLocalSaveDraftRequestSchemaV1>
export type UserInputLocalSaveDraftResponseV1 = z.infer<typeof userInputLocalSaveDraftResponseSchemaV1>
