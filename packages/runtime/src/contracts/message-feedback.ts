/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md. */
import { z } from 'zod'

export const MESSAGE_FEEDBACK_NOTE_MAX_BYTES_V1 = 8192
export const MessageFeedbackCategoryV1Schema = z.enum([
  'task-result', 'instruction-following', 'product-interaction', 'service-stability',
  'resource-cost', 'security-privacy-permission', 'other'
])
export const MessageFeedbackRatingV1Schema = z.enum(['positive', 'negative'])
const Identity = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const Version = z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
function wellFormedNote(value: string): boolean {
  for (let index = 0; index < value.length; index++) {
    const unit = value.charCodeAt(index)
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const next = value.charCodeAt(++index)
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false
    } else if (unit >= 0xdc00 && unit <= 0xdfff) return false
  }
  return true
}
const Note = z.string().refine(value => value.trim().length > 0, 'note-blank')
  .refine(wellFormedNote, 'note-invalid-unicode')
  .refine(value => new TextEncoder().encode(value).byteLength <= MESSAGE_FEEDBACK_NOTE_MAX_BYTES_V1, 'note-too-large')
export const MessageFeedbackItemV1Schema = z.object({
  messageId: Identity,
  rating: MessageFeedbackRatingV1Schema,
  note: Note.optional(),
  category: MessageFeedbackCategoryV1Schema.optional(),
  version: Version,
  createdAt: z.number().int().nonnegative().safe(),
  updatedAt: z.number().int().nonnegative().safe()
}).strict().refine(value => value.updatedAt >= value.createdAt)
export const MessageFeedbackPutRequestV1Schema = z.object({
  rating: MessageFeedbackRatingV1Schema,
  note: Note.optional(),
  category: MessageFeedbackCategoryV1Schema.optional(),
  ifVersion: Version.nullable()
}).strict()
export const MessageFeedbackDeleteRequestV1Schema = z.object({ ifVersion: Version }).strict()
export const MessageFeedbackListResponseV1Schema = z.object({
  items: z.array(MessageFeedbackItemV1Schema)
}).strict().refine(value => new Set(value.items.map(item => item.messageId)).size === value.items.length)
export const MessageFeedbackDeleteResponseV1Schema = z.object({ absent: z.literal(true) }).strict()
export const MessageFeedbackConflictV1Schema = z.object({
  code: z.literal('message_feedback_version_conflict'),
  message: z.literal('Message feedback changed. Review the current rating and submit again.'),
  current: MessageFeedbackItemV1Schema.nullable()
}).strict()
export type MessageFeedbackItemV1 = z.infer<typeof MessageFeedbackItemV1Schema>
export type MessageFeedbackPutRequestV1 = z.infer<typeof MessageFeedbackPutRequestV1Schema>
export type MessageFeedbackCategoryV1 = z.infer<typeof MessageFeedbackCategoryV1Schema>
export type MessageFeedbackRatingV1 = z.infer<typeof MessageFeedbackRatingV1Schema>
