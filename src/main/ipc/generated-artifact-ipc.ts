import { generatedArtifactPath, generatedArtifactRequestSchema, generatedArtifactResponseSchema, type GeneratedArtifactResponse } from '../../../packages/runtime/src/contracts/generated-artifact'

export function createGeneratedArtifactHandler(
  transport: (path: string, body: string) => Promise<{ ok: boolean; status: number; body: string }>
) {
  return async (payload: unknown): Promise<GeneratedArtifactResponse> => {
    const request = generatedArtifactRequestSchema.safeParse(payload)
    if (!request.success) return { ok: false, code: 'invalid_request' }
    try {
      const response = await transport(generatedArtifactPath, JSON.stringify(request.data))
      const parsed = generatedArtifactResponseSchema.safeParse(JSON.parse(response.body))
      if (parsed.success && response.ok === parsed.data.ok) {
        if (!parsed.data.ok || (
          parsed.data.artifact.threadId === request.data.threadId &&
          parsed.data.artifact.artifactId === request.data.artifactId
        )) return parsed.data
      }
    } catch { /* Never return raw private transport errors. */ }
    return { ok: false, code: 'unavailable' }
  }
}
