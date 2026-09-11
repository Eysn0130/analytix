import { z } from 'zod'

export const AttachmentTextFallback = z.object({
  dataBase64: z.string().min(1),
  mimeType: z.string().min(1),
  byteSize: z.number().int().nonnegative(),
  width: z.number().int().positive().optional(),
  height: z.number().int().positive().optional(),
  wasCompressed: z.boolean().optional()
}).strict()
export type AttachmentTextFallback = z.infer<typeof AttachmentTextFallback>

export const AttachmentMetadata = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  kind: z.enum(['image', 'document']).default('image'),
  mimeType: z.string().min(1),
  byteSize: z.number().int().nonnegative(),
  hash: z.string().min(1),
  width: z.number().int().positive().optional(),
  height: z.number().int().positive().optional(),
  documentText: z.string().optional(),
  pageCount: z.number().int().positive().optional(),
  truncated: z.boolean().optional(),
  localFilePath: z.string().min(1).optional(),
  textFallback: AttachmentTextFallback.optional(),
  threadIds: z.array(z.string().min(1)).default([]),
  workspaces: z.array(z.string().min(1)).default([]),
  createdAt: z.string(),
  updatedAt: z.string()
}).strict()
export type AttachmentMetadata = z.infer<typeof AttachmentMetadata>

export const AttachmentPublicMetadata = z.object({
  id: z.string().regex(/^att_[0-9a-f]{24}$/),
  name: z.string().min(1),
  kind: z.enum(['image', 'document']),
  mimeType: z.string().min(1),
  byteSize: z.number().int().nonnegative(),
  scope: z.literal('thread'),
  width: z.number().int().positive().optional(),
  height: z.number().int().positive().optional(),
  pageCount: z.number().int().positive().optional(),
  truncated: z.boolean().optional(),
  createdAt: z.string(),
  updatedAt: z.string()
}).strict()
export type AttachmentPublicMetadata = z.infer<typeof AttachmentPublicMetadata>

export const AttachmentUploadRequest = z.object({
  name: z.string().min(1),
  mimeType: z.string().min(1).optional(),
  dataBase64: z.string().min(1),
  documentText: z.string().optional(),
  pageCount: z.number().int().positive().optional(),
  textFallback: AttachmentTextFallback.optional(),
  threadId: z.string().min(1),
  workspace: z.string().min(1)
}).strict()
export type AttachmentUploadRequest = z.infer<typeof AttachmentUploadRequest>

export const AttachmentUploadResponse = z.object({
  attachment: AttachmentPublicMetadata
}).strict()
export type AttachmentUploadResponse = z.infer<typeof AttachmentUploadResponse>

export const AttachmentMetadataResponse = z.object({
  attachment: AttachmentPublicMetadata
}).strict()
export type AttachmentMetadataResponse = z.infer<typeof AttachmentMetadataResponse>

export const AttachmentContentResponse = z.object({
  attachment: AttachmentPublicMetadata,
  dataBase64: z.string().min(1)
}).strict()
export type AttachmentContentResponse = z.infer<typeof AttachmentContentResponse>

export const AttachmentDiagnostics = z.object({
  enabled: z.boolean(),
  rootDir: z.string(),
  count: z.number().int().nonnegative(),
  totalBytes: z.number().int().nonnegative()
}).strict()
export type AttachmentDiagnostics = z.infer<typeof AttachmentDiagnostics>
