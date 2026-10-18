import { describe, expect, it } from 'vitest'
import { UserInputResolutionRequest, UserInputWaitContinueRequestSchemaV1 } from './approvals'
import { userInputLocalDisplaySchemaV1, userInputLocalViewRequestSchemaV1, userInputLocalDiscardLateReplyRequestSchemaV1, userInputLocalDiscardLateReplyResponseSchemaV1, userInputLocalDraftSnapshotSchemaV1, userInputLocalSaveDraftRequestSchemaV1, userInputLocalSaveDraftResponseSchemaV1 } from './user-input-local-display'

const requestId = 'input_' + 'a'.repeat(64)
const q = requestId + '_1'
const selector = { threadId: 'thread-1', turnId: 'turn-1', itemId: 'item_' + requestId, requestId }
const pending = () => ({ schemaVersion: 1, ...selector, status: 'pending', answerState: 'pending',
  questions: [{ id: q, header: ' Header ', question: ' Original question ', detail: '**Original detail**',
    multiSelect: true, options: [{ id: q + '_option_1', label: ' Original option ', description: ' Original description ' }] }] })
const submitted = () => ({ ...pending(), status: 'submitted', answerState: 'available', submission: {
  schemaVersion: 1, format: 'structured', answers: [{ id: q, kind: 'answer', optionIds: [q + '_option_1'], customText: '  original\ninput  ' }]
} })

describe('inherited Question private display and action owner', () => {
  it('accepts readonly frozen history without an active countdown', () => {
    const history = { ...pending(), threadId: 'child', status: 'continued', answerState: 'unavailable', readOnly: true,
      timedWait: null, continuedReason: null, draft: null, draftRevision: null, lateReply: null }
    expect(userInputLocalDisplaySchemaV1.parse(history)).toEqual(history)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...history, readOnly: undefined }).success).toBe(false)
    for (const extra of [{ status: 'pending', answerState: 'pending' }, { readOnly: false }, { readOnly: null }, { draftRevision: '00000000-0000-4000-8000-000000000001' }]) {
      expect(userInputLocalDisplaySchemaV1.safeParse({ ...history, ...extra }).success).toBe(false)
    }
  })
  it('preserves exact selector on all three existing action carriers and legacy absence', () => {
    const answer = { answers: [{ id: q, kind: 'skip' as const }] }
    expect(UserInputResolutionRequest.parse({ ...answer, selector }).selector).toEqual(selector)
    expect(UserInputResolutionRequest.parse({ cancelled: true, selector }).selector).toEqual(selector)
    expect(UserInputWaitContinueRequestSchemaV1.parse({ action: 'continue', selector }).selector).toEqual(selector)
    expect(UserInputResolutionRequest.safeParse({ answers: [{ id: 'legacy-question', value: 'legacy answer' }] }).success).toBe(true)
    for (const invalid of [null, { ...selector, itemId: 'other' }, { ...selector, ThreadId: selector.threadId }, { threadId: selector.threadId }]) {
      expect(UserInputResolutionRequest.safeParse({ ...answer, selector: invalid }).success).toBe(false)
    }
  })
})

describe('private user input display contract', () => {
  it('preserves original string values and structured identities', () => {
    const source = submitted()
    expect(userInputLocalDisplaySchemaV1.parse(source)).toEqual(source)
  })
  it('permits old submitted history without inventing an answer', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'submitted', answerState: 'unavailable' }).success).toBe(true)
  })
  it('retains explicit skip rather than deriving it from submitted state', () => {
    const source = { ...submitted(), submission: { schemaVersion: 1, format: 'structured', answers: [{ id: q, kind: 'skip' }] } }
    expect(userInputLocalDisplaySchemaV1.parse(source)).toEqual(source)
  })
  it('retains a legacy receipt-bound alias and original body without guessing option IDs', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'submitted', answerState: 'available',
      submission: { schemaVersion: 1, format: 'legacy', answers: [{ id: 'q1', questionId: q, label: ' Other ', value: ' Original answer ' }] } }).success).toBe(true)
  })
  it.each([
    { ...selector, itemId: 'item_input_' + 'b'.repeat(64), viewId: '67eeac24-98da-4a49-a4ee-1d5b54574999' },
    { ...selector, threadId: '../thread', viewId: '67eeac24-98da-4a49-a4ee-1d5b54574999' },
    { ...selector, viewId: '67eeac24-98da-4a49-a4ee-1d5b54574999', body: 'raw' }
  ])('rejects unrelated or open request selectors %#', value => {
    expect(userInputLocalViewRequestSchemaV1.safeParse(value).success).toBe(false)
  })
  it.each([
    (value: ReturnType<typeof submitted>) => { value.questions[0].id = requestId + '_2' },
    (value: ReturnType<typeof submitted>) => { value.questions[0].options[0].id = q + '_option_2' },
    (value: ReturnType<typeof submitted>) => { value.submission.answers[0].optionIds = [q + '_option_2'] },
    (value: ReturnType<typeof submitted>) => { value.submission.answers[0].optionIds.push(q + '_option_1') },
    (value: ReturnType<typeof submitted>) => { value.submission.answers.push(value.submission.answers[0]) },
    (value: ReturnType<typeof submitted>) => { value.submission.answers[0].customText = ' \n ' },
    (value: ReturnType<typeof submitted>) => { value.submission.answers[0].customText = '\ud800' },
    (value: ReturnType<typeof submitted>) => { value.questions[0].multiSelect = false },
    (value: ReturnType<typeof submitted>) => { value.status = 'pending'; value.answerState = 'pending' },
    (value: ReturnType<typeof submitted>) => { value.answerState = 'unavailable' }
  ])('rejects malformed authority or answer content %#', mutate => {
    const value = submitted(); mutate(value)
    expect(userInputLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
  })
  it('rejects an available status without a durable submission', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'submitted', answerState: 'available' }).success).toBe(false)
  })
  it('rejects extra fields even when displaying history', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...submitted(), grant: {} }).success).toBe(false)
  })
})

describe('continued private Question display', () => {
  const timedWait = { timeoutSeconds: 120, deadlineAt: '2026-10-08T09:02:00Z' }
  it('keeps pending history nullable and continued without an answer', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), timedWait: null, lateReply: null }).success).toBe(true)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'continued', answerState: 'unavailable', timedWait, lateReply: null }).success).toBe(true)
  })
  it.each(['queued', 'accepted', 'discarded'])('retains raw capture with actual %s delivery', state => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...submitted(), status: 'continued', timedWait,
      lateReply: { state, clientUserMessageId: '00000000-0000-4000-8000-000000000001', turnId: 'turn_2' } }).success).toBe(true)
  })
  it('keeps captured unadmitted raw editable and rejects continued unlimited waits', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...submitted(), status: 'continued', timedWait, lateReply: null }).success).toBe(true)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'continued', answerState: 'unavailable', timedWait: { timeoutSeconds: -1, deadlineAt: null } }).success).toBe(false)
  })
})


describe('private original Question correlation', () => {
  it('accepts the real Host callId and nullable legacy display without deriving it from text', () => {
    expect(userInputLocalDisplaySchemaV1.parse({ ...pending(), callId: 'call_123456789012' }).callId).toBe('call_123456789012')
    expect(userInputLocalDisplaySchemaV1.parse({ ...pending(), callId: null }).callId).toBeNull()
    expect(userInputLocalDisplaySchemaV1.safeParse(pending()).success).toBe(true)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), callId: '' }).success).toBe(false)
  })
})


describe('private per-attempt late reply discard contract', () => {
  const request = { ...selector, action: 'discardLateReply', clientUserMessageId: '00000000-0000-4000-8000-000000000001', expectedTurnId: 'turn-target' }
  const response = { schemaVersion: 1, ...selector, status: 'continued', lateReply: {
    clientUserMessageId: request.clientUserMessageId, turnId: request.expectedTurnId, state: 'discarded'
  } }
  it('keeps original and queue target turns independent', () => {
    expect(userInputLocalDiscardLateReplyRequestSchemaV1.parse(request)).toEqual(request)
    expect(userInputLocalDiscardLateReplyResponseSchemaV1.parse(response)).toEqual(response)
  })
  it.each([
    { ...request, action: 'cancel' }, { ...request, action: null },
    { ...request, clientUserMessageId: 'arbitrary-text' }, { ...request, expectedTurnId: '../target' },
    { ...request, receiptId: 'b'.repeat(64) }, { ...request, answers: [] },
    { ...request, itemId: 'item_input_' + 'b'.repeat(64) }
  ])('rejects open or unbound commands %#', value => {
    expect(userInputLocalDiscardLateReplyRequestSchemaV1.safeParse(value).success).toBe(false)
  })
  it.each([
    { ...response, status: 'cancelled' }, { ...response, lateReply: { ...response.lateReply, state: 'accepted' } },
    { ...response, lateReply: null }, { ...response, submission: {} },
    { ...response, cancellationAuthoritySignature: 'raw-proof' }
  ])('rejects invented disposition and proof disclosure %#', value => {
    expect(userInputLocalDiscardLateReplyResponseSchemaV1.safeParse(value).success).toBe(false)
  })
})

describe('private unfinished Question draft contract', () => {
  const revision = '00000000-0000-4000-8000-000000000001'
  const snapshot = () => ({ currentPage: 0, answers: [{ questionId: q, optionIds: [] as string[], customText: ' \n ', skipped: false }] })
  const command = () => ({ ...selector, action: 'saveDraft', revision, expectedRevision: null, draft: snapshot() })
  it('keeps nullable cursor/content compatible with older producers and independent of answers', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse(pending()).success).toBe(true)
    expect(userInputLocalDisplaySchemaV1.parse({ ...pending(), draftRevision: null, draft: null }).draft).toBeNull()
    const value = { ...pending(), draftRevision: revision, draft: snapshot() }
    expect(userInputLocalDisplaySchemaV1.parse(value)).toEqual(value)
    expect(userInputLocalDisplaySchemaV1.parse({ ...pending(), draftRevision: revision, draft: null }).draftRevision).toBe(revision)
  })
  it('allows blank, empty and partial state while preserving unfinished text', () => {
    expect(userInputLocalSaveDraftRequestSchemaV1.parse(command()).draft.answers[0].customText).toBe(' \n ')
    expect(userInputLocalSaveDraftRequestSchemaV1.safeParse({ ...command(), draft: { currentPage: 2, answers: [] } }).success).toBe(true)
    // Request shape cannot know the actual count; the Host/display enforces it.
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), draftRevision: revision, draft: { currentPage: 2, answers: [] } }).success).toBe(false)
  })
  it.each([
    { ...command(), expectedRevision: revision },
    { ...command(), revision: '00000000-0000-4000-8000-00000000000A' },
    { ...command(), draft: { ...snapshot(), raw: 'private' } },
    { ...command(), draft: { currentPage: 0, answers: [{ questionId: q, optionIds: [], customText: '' }] } },
    { ...command(), draft: { ...snapshot(), answers: [{ ...snapshot().answers[0], optionIds: [q + '_option_1', q + '_option_1'] }] } },
    { ...command(), draft: { ...snapshot(), answers: [snapshot().answers[0], snapshot().answers[0]] } },
    { ...command(), draft: { ...snapshot(), answers: [{ ...snapshot().answers[0], skipped: true }] } },
    { ...command(), draft: { ...snapshot(), answers: [{ ...snapshot().answers[0], customText: '\ud800' }] } },
    { ...command(), draft: { ...snapshot(), answers: [{ ...snapshot().answers[0], questionId: 'input_' + 'b'.repeat(64) + '_1' }] } },
    { ...command(), captureSetSha256: 'a'.repeat(64) }
  ])('rejects ambiguous/open/unbound save state %#', value => {
    expect(userInputLocalSaveDraftRequestSchemaV1.safeParse(value).success).toBe(false)
  })
  it('requires real Host options and single-select exclusivity at display', () => {
    const value = { ...pending(), draftRevision: revision, draft: snapshot() }
    value.draft.answers[0].optionIds = [q + '_option_2']
    expect(userInputLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
    value.draft.answers[0].optionIds = [q + '_option_1']; value.draft.answers[0].customText = 'unfinished'; value.questions[0].multiSelect = false
    expect(userInputLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
  })
  it('does not expose active draft after submit/cancel or queued/accepted delivery', () => {
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...submitted(), draftRevision: revision, draft: snapshot() }).success).toBe(false)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), status: 'cancelled', answerState: 'cancelled', draftRevision: revision, draft: snapshot() }).success).toBe(false)
    for (const state of ['queued', 'accepted']) expect(userInputLocalDisplaySchemaV1.safeParse({ ...submitted(), status: 'continued', timedWait: { timeoutSeconds: 120, deadlineAt: '2026-10-08T09:02:00Z' }, draftRevision: revision, draft: snapshot(), lateReply: { state, clientUserMessageId: revision, turnId: 'turn-target' } }).success).toBe(false)
  })
  it('keeps ACK closed and distinct from submitted/queued/accepted', () => {
    const response = { schemaVersion: 1, ...selector, status: 'pending', draftRevision: revision, draft: snapshot() }
    expect(userInputLocalSaveDraftResponseSchemaV1.parse(response)).toEqual(response)
    for (const status of ['submitted', 'queued', 'accepted']) expect(userInputLocalSaveDraftResponseSchemaV1.safeParse({ ...response, status }).success).toBe(false)
    expect(userInputLocalSaveDraftResponseSchemaV1.safeParse({ ...response, submission: {} }).success).toBe(false)
  })
  it('uses the same escaped JSON byte accounting as Go without enlarging the cap', () => {
    const value = snapshot(); value.answers[0].customText = '<'.repeat(700_000)
    expect(userInputLocalDraftSnapshotSchemaV1.safeParse(value).success).toBe(false)
    value.answers[0].customText = 'x'.repeat(700_000)
    expect(userInputLocalDraftSnapshotSchemaV1.safeParse(value).success).toBe(true)
  })
})

describe('Question Core private display choices and real review intent', () => {
  it('preserves old draft omission and round-trips waiting or editing without null', () => {
    const draft = { currentPage: 0, answers: [] }
    expect(userInputLocalDraftSnapshotSchemaV1.parse(draft)).toEqual(draft)
    for (const wait of ['waiting', 'editing']) expect(userInputLocalDraftSnapshotSchemaV1.parse({ ...draft, wait })).toEqual({ ...draft, wait })
    for (const wait of [null, 'approved', true]) expect(userInputLocalDraftSnapshotSchemaV1.safeParse({ ...draft, wait }).success).toBe(false)
  })
  it('binds review intent to the real call, full detail and Host option', () => {
    const base = pending()
    const question = { ...base.questions[0], multiSelect: false, intent: { kind: 'plan-review', approveOptionId: q + '_option_1' } }
    const review = { ...base, callId: 'call_original', questions: [question] }
    expect(userInputLocalDisplaySchemaV1.parse(review)).toEqual(review)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...base, questions: [{ ...base.questions[0], intent: null }] }).success).toBe(true)
    for (const value of [
      { ...review, callId: null },
      { ...review, questions: [{ ...question, detail: undefined }] },
      { ...review, questions: [{ ...question, detail: " \n " }] },
      { ...review, questions: [{ ...question, multiSelect: true }] },
      { ...review, questions: [{ ...question, intent: { kind: 'plan-review', approveOptionId: q + '_option_2' } }] },
      { ...review, questions: [{ ...question, intent: { ...question.intent, approved: true } }] }
    ]) expect(userInputLocalDisplaySchemaV1.safeParse(value).success).toBe(false)
  })
  it('permits interrupted unlimited continued history only with its exact unknown reason', () => {
    const value = { ...pending(), status: 'continued', answerState: 'unavailable', timedWait: { timeoutSeconds: -1, deadlineAt: null } }
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...value, continuedReason: 'user_input_outcome_unknown_after_restart' }).success).toBe(true)
    for (const continuedReason of [undefined, null, 'user_input_continued', 'approved']) expect(userInputLocalDisplaySchemaV1.safeParse({ ...value, continuedReason }).success).toBe(false)
    expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), continuedReason: 'user_input_outcome_unknown_after_restart' }).success).toBe(false)
  })
})


describe('proven inherited accepted reply association', () => {
 const history = () => ({ ...submitted(), status: 'continued', readOnly: true, timedWait: null, lateReply: null,
   historyReply: { itemId: 'item_steer_' + 'c'.repeat(64), clientUserMessageId: '00000000-0000-4000-8000-000000000001', turnId: 'actual-reply-turn' } })
 it('keeps a fixed child reply tuple separate from active delivery', () => {
   expect(userInputLocalDisplaySchemaV1.parse(history())).toEqual(history())
   expect(userInputLocalDisplaySchemaV1.safeParse({ ...pending(), historyReply: null }).success).toBe(true)
 })
 it.each([
   { historyReply: undefined }, { historyReply: null }, { readOnly: undefined }, { status: 'submitted' }, { answerState: 'unavailable', submission: undefined },
   { submission: undefined }, { timedWait: { timeoutSeconds: 1, deadlineAt: '2026-10-09T00:00:00Z' } },
   { draftRevision: '00000000-0000-4000-8000-000000000001' },
   { lateReply: { state: 'accepted', clientUserMessageId: '00000000-0000-4000-8000-000000000001', turnId: 'actual-reply-turn' } }
 ])('rejects historical reply outside the exact readonly answer mode %#', patch => {
   expect(userInputLocalDisplaySchemaV1.safeParse({ ...history(), ...patch }).success).toBe(false)
 })
 it.each([
   { itemId: selector.itemId }, { itemId: '../fake' }, { turnId: '' }, { clientUserMessageId: 'fake' }, { receiptId: 'raw' }
 ])('rejects malformed, unrelated or open reply tuple %#', patch => {
   const value = history()
   expect(userInputLocalDisplaySchemaV1.safeParse({ ...value, historyReply: { ...value.historyReply, ...patch } }).success).toBe(false)
 })
})
