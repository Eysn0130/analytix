import { z } from 'zod'

const index = z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
const digest = z.string().regex(/^[a-f0-9]{64}$/)
export const nativeOfficeKindSchema = z.enum(['docx', 'xlsx', 'pptx'])
export const nativeOfficeBoundsSchema = z.object({
  x: z.number().int().min(0).max(16384), y: z.number().int().min(0).max(16384),
  width: z.number().int().min(1).max(16384), height: z.number().int().min(1).max(16384)
}).strict()
const localPath = z.string().min(1).max(4096).refine(value => value.trim() === value && !['\0', '\r', '\n'].some(char => value.includes(char)) && !/^[a-z][a-z0-9+.-]*:\/\//i.test(value))
export const nativeOfficePickerRequestSchema = z.object({ kind: nativeOfficeKindSchema, workspace: localPath }).strict()
export const nativeOfficePickerResponseSchema = z.union([
  z.object({ ok: z.literal(true), path: localPath.nullable() }).strict(),
  z.object({ ok: z.literal(false), error: z.enum(['invalid_request', 'unavailable']) }).strict()
])

/** Renderer commands contain no native bytes, URLs, engine envelopes or UNO. */
export const nativeOfficeRequestSchema = z.discriminatedUnion('action', [
  z.object({ action: z.literal('open'), workspace: localPath, path: localPath, bounds: nativeOfficeBoundsSchema }).strict(),
  z.object({ action: z.literal('bounds'), bounds: nativeOfficeBoundsSchema }).strict(),
  z.object({ action: z.literal('hide') }).strict(),
  z.object({ action: z.literal('status') }).strict(),
  z.object({ action: z.literal('captureSelection') }).strict(),
  z.object({ action: z.literal('close'), discard: z.boolean().optional() }).strict()
])
const selectionBase = { documentId: digest, version: digest, changeSequence: index }
export const nativeOfficeSelectionSchema = z.discriminatedUnion('kind', [
  z.object({ ...selectionBase, kind: z.literal('text'), scope: z.literal('current-view-text-only; no verified structural offset or durable anchor'), text: z.string().max(4096) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('cells'), scope: z.literal('sheet-range-address-at-version-and-change-sequence'), text: z.string().max(4096), ranges: z.array(z.object({
    sheet: index, sheetName: z.string().max(4096), startColumn: index, startRow: index, endColumn: index, endRow: index
  }).strict().refine(value => value.endColumn >= value.startColumn && value.endRow >= value.startRow)).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('shapes'), scope: z.literal('page-and-shape-index-at-version-and-change-sequence'), shapes: z.array(z.object({
    pageIndex: index, shapeIndex: index, name: z.string().max(4096), type: z.string().max(256), text: z.string().max(4096)
  }).strict()).max(64) }).strict(),
  z.object({ ...selectionBase, kind: z.literal('unavailable'), scope: z.literal('engine selection interface unavailable in this view') }).strict()
])
export const nativeOfficeErrorSchema = z.enum([
  'invalid_request', 'unavailable', 'package_unavailable', 'package_changed', 'invalid_response',
  'engine_unavailable', 'unsaved_changes', 'conflict', 'unknown', 'save_failed'
])
export const nativeOfficeViewSchema = z.object({
  objectId: digest, path: localPath, kind: nativeOfficeKindSchema, revision: digest,
  dirty: z.boolean(), status: z.enum(['ready', 'error', 'loading']),
  selection: nativeOfficeSelectionSchema.optional(), error: nativeOfficeErrorSchema.optional()
}).strict()
export const nativeOfficeResponseSchema = z.object({
  ok: z.boolean(), view: nativeOfficeViewSchema.nullable(), error: nativeOfficeErrorSchema.optional()
}).strict()
export type NativeOfficeRequest = z.infer<typeof nativeOfficeRequestSchema>
export type NativeOfficeBounds = z.infer<typeof nativeOfficeBoundsSchema>
export type NativeOfficeView = z.infer<typeof nativeOfficeViewSchema>
export type NativeOfficeSelection = z.infer<typeof nativeOfficeSelectionSchema>
export type NativeOfficeResponse = z.infer<typeof nativeOfficeResponseSchema>
export type NativeOfficeError = z.infer<typeof nativeOfficeErrorSchema>
export type NativeOfficeKind = z.infer<typeof nativeOfficeKindSchema>
