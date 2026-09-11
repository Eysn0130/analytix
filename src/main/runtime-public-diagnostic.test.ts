import { createHash } from 'node:crypto'
import { describe, expect, it } from 'vitest'
import { runtimeErrorPublicDiagnosticV1 } from './runtime-public-diagnostic'

describe('runtimeErrorPublicDiagnosticV1', () => {
  it('returns only byte length and full SHA-256 for marker-free private errors', () => {
    const sentinel = 'PRIVATE_PROVIDER_FAILURE_7F3C'
    const diagnostic = runtimeErrorPublicDiagnosticV1(new Error(sentinel))

    expect(diagnostic).toEqual({
      errorBytes: Buffer.byteLength(sentinel, 'utf8'),
      errorSha256: createHash('sha256').update(sentinel, 'utf8').digest('hex')
    })
    expect(JSON.stringify(diagnostic)).not.toContain(sentinel)
    expect(diagnostic.errorSha256).toMatch(/^[a-f0-9]{64}$/u)
  })

  it('does not serialize arbitrary object fields', () => {
    const diagnostic = runtimeErrorPublicDiagnosticV1({ secret: 'PRIVATE_OBJECT_FIELD_7F3C' })
    expect(JSON.stringify(diagnostic)).not.toContain('PRIVATE_OBJECT_FIELD_7F3C')
  })

  it('fails closed when hostile object metadata throws during classification', () => {
    const sentinel = 'HOSTILE_DIAGNOSTIC_GETTER_/private/customer-pii-13900000012'
    const hostile = Object.defineProperty({}, Symbol.toStringTag, {
      get() {
        throw new Error(sentinel)
      }
    })

    const diagnostic = runtimeErrorPublicDiagnosticV1(hostile)
    expect(diagnostic).toEqual({
      errorBytes: Buffer.byteLength('unavailable', 'utf8'),
      errorSha256: createHash('sha256').update('unavailable', 'utf8').digest('hex')
    })
    expect(JSON.stringify(diagnostic)).not.toContain(sentinel)
  })
})
