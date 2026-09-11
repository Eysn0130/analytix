import { jsonResponse, type JsonResponse } from '../response.js'
import type { ServerRuntime } from './server-runtime.js'

/**
 * Returns the most-recent LLM rounds (request body + raw output) captured
 * in-memory for troubleshooting. Empty when no recorder is configured.
 */
export function llmDebugRoundsResponse(runtime: ServerRuntime): JsonResponse {
  return jsonResponse({ rounds: runtime.llmDebug?.snapshot() ?? [] })
}
