import { createHash } from 'node:crypto'

export type RuntimeErrorPublicDiagnosticV1 = {
  errorBytes: number
  errorSha256: string
}

function errorDiagnosticText(value: unknown): string {
  try {
    if (value instanceof Error) return value.message
    if (typeof value === 'string') return value
    if (value === null) return 'null'
    if (value === undefined) return 'undefined'
    return Object.prototype.toString.call(value)
  } catch {
    return 'unavailable'
  }
}

/**
 * Converts a private runtime error into opaque correlation metadata. The raw
 * text is never returned and must remain transient in the caller.
 */
export function runtimeErrorPublicDiagnosticV1(value: unknown): RuntimeErrorPublicDiagnosticV1 {
  const text = errorDiagnosticText(value)
  return {
    errorBytes: Buffer.byteLength(text, 'utf8'),
    errorSha256: createHash('sha256').update(text, 'utf8').digest('hex')
  }
}
