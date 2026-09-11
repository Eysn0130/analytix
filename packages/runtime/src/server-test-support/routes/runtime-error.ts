import { jsonResponse, type JsonResponse } from '../response.js'
import type { AnalytixErrorBody } from '../../contracts/errors.js'

export type RuntimeError = AnalytixErrorBody

export function errorResponse(
  body: RuntimeError,
  status: number
): JsonResponse {
  return jsonResponse(body, status)
}

export const ERRORS = {
  unauthorized: (_message = '') =>
    errorResponse({ code: 'unauthorized', message: 'Runtime authentication is required.' }, 401),
  forbidden: (_message = '') =>
    errorResponse({ code: 'forbidden', message: 'The request is not authorized.' }, 403),
  notFound: (_message = '') =>
    errorResponse({ code: 'not_found', message: 'The requested resource was not found.' }, 404),
  validation: (_message: string, _issues?: unknown) =>
    errorResponse({ code: 'validation_error', message: 'The request did not satisfy the runtime contract.' }, 400),
  attachmentValidation: (_message: string, _issues?: unknown) =>
    errorResponse({ code: 'validation_error', message: 'The request did not satisfy the runtime contract.' }, 400),
  conflict: (_message: string) =>
    errorResponse({ code: 'conflict', message: 'The request conflicts with the current runtime state.' }, 409),
  notImplemented: (_message: string) =>
    errorResponse({ code: 'internal_error', message: 'The runtime request could not be completed safely.' }, 501),
  unavailable: (_message: string) =>
    errorResponse({ code: 'internal_error', message: 'The runtime request could not be completed safely.' }, 503),
  internal: (_message: string) =>
    errorResponse({ code: 'internal_error', message: 'The runtime request could not be completed safely.' }, 500)
} as const
