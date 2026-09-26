import { describe, expect, it } from 'vitest'
import {
  parseRuntimeErrorBody,
  projectPublicRuntimeHTTPError,
  runtimeErrorToError
} from './runtime-error'

const privateSentinel = 'PRIVATE account=6222021234567890 /private/case.csv token=query-secret'

describe('runtime public error projection', () => {
  it('keeps only a known code and its host-authored message', () => {
    const parsed = parseRuntimeErrorBody(JSON.stringify({
      code: 'attachment_validation_failed',
      message: privateSentinel,
      details: [{ message: privateSentinel }],
      providerError: { response: privateSentinel }
    }), 'fallback')

    expect(parsed).toEqual({
      code: 'attachment_validation_failed',
      message: 'The attachment did not satisfy the runtime contract.'
    })
    expect(JSON.stringify(parsed)).not.toContain(privateSentinel)
  })

  it('round trips only the closed runtime error identity', () => {
    const error = runtimeErrorToError({
      code: 'provider_unavailable',
      message: privateSentinel
    })

    expect(parseRuntimeErrorBody(error.message, 'fallback')).toEqual({
      code: 'provider_unavailable',
      message: 'The model provider is temporarily unavailable.'
    })
    expect(error.message).not.toContain(privateSentinel)
  })

  it('does not preserve transport paths, URLs, or arbitrary diagnostics', () => {
    const parsed = parseRuntimeErrorBody(JSON.stringify({
      code: 'runtime_unavailable',
      message: privateSentinel,
      details: { baseUrl: 'http://127.0.0.1:1', path: `/v1/threads?${privateSentinel}` }
    }), privateSentinel)

    expect(parsed).toEqual({
      code: 'runtime_unavailable',
      message: 'The Analytix runtime is unavailable.'
    })
    expect(JSON.stringify(parsed)).not.toContain(privateSentinel)
  })

  it('projects turn failures through the bounded reason-code registry', () => {
    expect(parseRuntimeErrorBody(JSON.stringify({
      code: 'turn_failed',
      reasonCode: 'provider_authentication_failed',
      message: privateSentinel,
      providerError: { body: privateSentinel }
    }), 'fallback')).toEqual({
      code: 'turn_failed',
      reasonCode: 'provider_authentication_failed',
      message: 'Provider authentication failed. Check the configured credential.'
    })

    expect(parseRuntimeErrorBody(JSON.stringify({
      code: 'turn_failed',
      reasonCode: privateSentinel,
      message: privateSentinel
    }), 'fallback')).toEqual({
      code: 'turn_failed',
      reasonCode: 'turn_failed',
      message: 'The turn failed before a verified response was available.'
    })
  })

  it('never uses malformed, unknown, or fallback text as a public message', () => {
    for (const body of [privateSentinel, JSON.stringify({ code: privateSentinel, message: privateSentinel }), '']) {
      const parsed = parseRuntimeErrorBody(body, privateSentinel)
      expect(parsed).toEqual({
        code: 'unknown',
        message: 'The runtime request could not be completed safely.'
      })
      expect(JSON.stringify(parsed)).not.toContain(privateSentinel)
    }
  })

  it('derives public HTTP errors from status and rejects mismatched body codes', () => {
    expect(projectPublicRuntimeHTTPError(401, {
      code: 'attachment_upload_unavailable',
      message: privateSentinel,
      details: privateSentinel
    })).toEqual({ code: 'unauthorized', message: 'Runtime authentication is required.' })
    expect(projectPublicRuntimeHTTPError(404, {
      code: 'case_history_restricted',
      message: privateSentinel
    })).toEqual({ code: 'not_found', message: 'The requested resource was not found.' })

    expect(projectPublicRuntimeHTTPError(503, {
      code: 'public_projection_pending',
      message: privateSentinel
    })).toEqual({
      code: 'public_projection_pending',
      message: 'The thread public projection is finalizing.'
    })
    expect(projectPublicRuntimeHTTPError(500, {
      code: 'public_projection_pending',
      message: privateSentinel
    })).toEqual({
      code: 'internal_error',
      message: 'The runtime request could not be completed safely.'
    })

    expect(projectPublicRuntimeHTTPError(409, {
      code: 'turn_execution_conflict',
      message: privateSentinel
    })).toEqual({
      code: 'turn_execution_conflict',
      message: 'Another terminal or security transition still owns this thread.'
    })
    expect(projectPublicRuntimeHTTPError(503, {
      code: 'turn_execution_conflict',
      message: privateSentinel
    })).toEqual({
      code: 'internal_error',
      message: 'The runtime request could not be completed safely.'
    })

		expect(projectPublicRuntimeHTTPError(409, {
			code: 'worktree_isolation_authority_required',
			message: privateSentinel,
			approvalId: privateSentinel
		})).toEqual({
			code: 'worktree_isolation_authority_required',
			message: 'Worktree isolation controls require host-issued durable authority.'
		})
  })
})
