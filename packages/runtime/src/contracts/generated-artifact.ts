import { z } from 'zod'

export const generatedArtifactPath = '/v1/local-display/generated-artifact'
export const generatedArtifactMetadataSchema = z.object({
  artifactId: z.string().regex(/^[a-f0-9]{64}$/),
  kind: z.literal('docx'),
  contentHash: z.string().regex(/^[a-f0-9]{64}$/),
  byteSize: z.number().int().positive().max(16 * 1024 * 1024),
  savedAt: z.string().datetime({ offset: true })
}).strict()
export type GeneratedArtifactMetadata = z.infer<typeof generatedArtifactMetadataSchema>
export const generatedArtifactRequestSchema = z.object({
  threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/),
  artifactId: z.string().regex(/^[a-f0-9]{64}$/)
}).strict()
const localPath = z.string().min(1).max(32768).refine(value => !value.includes('\0'))
export const generatedArtifactResponseSchema = z.discriminatedUnion('ok', [
  z.object({
    ok: z.literal(true),
    artifact: z.object({
      ...generatedArtifactRequestSchema.shape,
      path: localPath,
      workspace: localPath,
      kind: z.literal('docx'),
      revision: z.string().regex(/^[a-f0-9]{64}$/),
      byteSize: z.number().int().positive().max(16 * 1024 * 1024),
      changed: z.boolean()
    }).strict()
  }).strict(),
  z.object({ ok: z.literal(false), code: z.enum(['artifact_unavailable', 'invalid_request', 'unavailable']) }).strict()
])
export type GeneratedArtifactRequest = z.infer<typeof generatedArtifactRequestSchema>
export type GeneratedArtifactResponse = z.infer<typeof generatedArtifactResponseSchema>
