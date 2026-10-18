import { describe, expect, it } from 'vitest'
import { MessageFeedbackConflictV1Schema, MessageFeedbackItemV1Schema, MessageFeedbackListResponseV1Schema,
  MessageFeedbackPutRequestV1Schema } from './message-feedback'

const version = '11111111-1111-4111-8111-111111111111'
const item = { messageId: 'message.1', rating: 'positive', version, createdAt: 1, updatedAt: 1 }
describe('message feedback V1 contract', () => {
  it('admits canonical IDs, optional empty form and lowercase UUIDv4', () => {
    expect(MessageFeedbackItemV1Schema.parse(item)).toEqual(item)
    expect(MessageFeedbackPutRequestV1Schema.parse({ rating: 'negative', ifVersion: null })).toEqual({ rating: 'negative', ifVersion: null })
    expect(MessageFeedbackConflictV1Schema.parse({ code: 'message_feedback_version_conflict', message: 'Message feedback changed. Review the current rating and submit again.', current: null }).current).toBeNull()
  })
  it.each(['../message', 'message..1', '_message', 'message/1', 'a'.repeat(256)])('rejects a storage alias %s', messageId => {
    expect(MessageFeedbackItemV1Schema.safeParse({ ...item, messageId }).success).toBe(false)
  })
  it('enforces UTF-8 bytes and requires a nullable creation precondition', () => {
    const request = { rating: 'positive', ifVersion: null }
    expect(MessageFeedbackPutRequestV1Schema.safeParse({ ...request, note: '界'.repeat(2730) + 'ab' }).success).toBe(true)
    expect(MessageFeedbackPutRequestV1Schema.safeParse({ ...request, note: '界'.repeat(2731) }).success).toBe(false)
    for (const invalid of [{ rating: 'positive' }, { ...request, note: ' ' }, { ...request, category: 'invented' }, { ...request, note: null }, { ...request, extra: true }]) {
      expect(MessageFeedbackPutRequestV1Schema.safeParse(invalid).success).toBe(false)
    }
    expect(MessageFeedbackPutRequestV1Schema.safeParse({ ...request, note: '\ud800' }).success).toBe(false)
  })
  it('rejects duplicate targets, non-v4 versions and backwards timestamps', () => {
    expect(MessageFeedbackListResponseV1Schema.safeParse({ items: [item, item] }).success).toBe(false)
    expect(MessageFeedbackItemV1Schema.safeParse({ ...item, version: version.replace('-4111-', '-1111-') }).success).toBe(false)
    expect(MessageFeedbackItemV1Schema.safeParse({ ...item, updatedAt: 0 }).success).toBe(false)
  })
})
