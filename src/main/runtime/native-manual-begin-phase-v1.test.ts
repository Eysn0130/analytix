import { describe, expect, it } from 'vitest'
import {
  createNativeManualBeginPhaseStreamV1,
  NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1,
  NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1,
  normalizeNativeManualBeginPhaseV1,
  parseNativeManualBeginPhaseLineV1,
  type NativeManualBeginPhaseV1
} from './native-manual-begin-phase-v1'

const phases = [
  'input',
  'authority',
  'dependencies',
  'capture_conflict',
  'capture',
  'baseline_open',
  'baseline_session',
  'baseline_revision',
  'prepare',
  'prepared_contract'
] as const

const errorKinds = [
  'invalid_request',
  'session_invalid',
  'scope_invalid',
  'projection_unavailable',
  'draft_stale',
  'conflict',
  'persistence_failure',
  'operation_mismatch',
  'operation_not_found',
  'forbidden',
  'not_text',
  'too_large',
  'capacity',
  'unavailable',
  'capture_unavailable',
  'unsafe_path',
  'capture_session_mismatch',
  'opaque_execution',
  'baseline_failed',
  'context_canceled',
  'context_deadline',
  'not_found',
  'permission',
  'other'
] as const

const booleanFields = [
  'manualServicePresent',
  'captureBound',
  'captureEntered',
  'captureReused',
  'baselineEntered',
  'baselineOpenOK',
  'sessionMatch',
  'revisionMatch',
  'captureAcquired',
  'prepareEntered',
  'prepareOK'
] as const

function phase(overrides: Partial<NativeManualBeginPhaseV1> = {}): NativeManualBeginPhaseV1 {
  return {
    version: 1,
    phase: 'capture',
    errorKind: 'capture_unavailable',
    manualServicePresent: true,
    captureBound: true,
    captureEntered: true,
    captureReused: false,
    baselineEntered: false,
    baselineOpenOK: false,
    sessionMatch: false,
    revisionMatch: false,
    captureAcquired: false,
    prepareEntered: false,
    prepareOK: false,
    ...overrides
  }
}

function frame(value: unknown = phase()): string {
  return `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}${JSON.stringify(value)}\n`
}

function paddedFrame(bytes: number, value: NativeManualBeginPhaseV1 = phase()): string {
  const body = JSON.stringify(value)
  const padding = bytes - Buffer.byteLength(frame(value), 'utf8')
  if (padding < 0) throw new Error('Requested frame is shorter than its payload')
  return `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}{${' '.repeat(padding)}${body.slice(1)}\n`
}

describe('native manual begin phase normalization', () => {
  it('returns a new object containing exactly the fourteen approved fields', () => {
    const input = Object.freeze(phase())
    const normalized = normalizeNativeManualBeginPhaseV1(input)
    expect(normalized).toEqual(input)
    expect(normalized).not.toBe(input)
    expect(Object.keys(normalized ?? {})).toEqual([
      'version', 'phase', 'errorKind', ...booleanFields
    ])
    const reordered = Object.fromEntries(Object.entries(input).reverse())
    expect(Object.keys(normalizeNativeManualBeginPhaseV1(reordered) ?? {})).toEqual(Object.keys(input))
  })

  it.each(phases)('accepts the exact phase %s', (value) => {
    const input = phase({ phase: value })
    expect(normalizeNativeManualBeginPhaseV1(input)).toEqual(input)
  })

  it.each(errorKinds)('accepts the exact error kind %s', (value) => {
    const input = phase({ errorKind: value })
    expect(normalizeNativeManualBeginPhaseV1(input)).toEqual(input)
  })

  it.each(booleanFields)('preserves both boolean values for %s without coercion', (key) => {
    for (const value of [true, false]) {
      const input = { ...phase(), [key]: value }
      expect(normalizeNativeManualBeginPhaseV1(input)).toEqual(input)
    }
    for (const value of ['true', 'false', 0, 1, null, undefined, [], {}]) {
      expect(normalizeNativeManualBeginPhaseV1({ ...phase(), [key]: value })).toBeNull()
    }
  })

  it('accepts all-false and all-true flags without inferring relationships between them', () => {
    for (const value of [true, false]) {
      const input = { ...phase() }
      for (const key of booleanFields) input[key] = value
      expect(normalizeNativeManualBeginPhaseV1(input)).toEqual(input)
    }
  })

  it('rejects missing or extra fields rather than projecting an unsafe payload', () => {
    for (const key of ['version', 'phase', 'errorKind', ...booleanFields]) {
      const input: Record<string, unknown> = { ...phase() }
      delete input[key]
      expect(normalizeNativeManualBeginPhaseV1(input)).toBeNull()
    }
    for (const key of ['threadId', 'path', 'body', '__proto__']) {
      const input = { ...phase(), [key]: 'RAW_CANARY_DO_NOT_FORWARD' }
      expect(normalizeNativeManualBeginPhaseV1(input)).toBeNull()
    }
  })

  it('rejects nonenumerable and symbol extras as well as inherited required fields', () => {
    const nonenumerable = Object.defineProperty(phase(), 'body', {
      value: 'RAW_CANARY_DO_NOT_FORWARD', enumerable: false
    })
    expect(normalizeNativeManualBeginPhaseV1(nonenumerable)).toBeNull()
    expect(normalizeNativeManualBeginPhaseV1({
      ...phase(), [Symbol('body')]: 'RAW_CANARY_DO_NOT_FORWARD'
    })).toBeNull()
    const inherited = Object.assign(Object.create({ prepareOK: false }), phase())
    delete inherited.prepareOK
    expect(normalizeNativeManualBeginPhaseV1(inherited)).toBeNull()
  })

  it('does not execute accessors and rejects hostile reflection without throwing', () => {
    let reads = 0
    const accessor = Object.defineProperty(phase(), 'phase', {
      enumerable: true,
      get: () => {
        reads += 1
        throw new Error('synthetic accessor failure')
      }
    })
    expect(() => expect(normalizeNativeManualBeginPhaseV1(accessor)).toBeNull()).not.toThrow()
    expect(reads).toBe(0)
    const ownKeysFailure = new Proxy(phase(), {
      ownKeys: () => { throw new Error('synthetic ownKeys failure') }
    })
    const descriptorFailure = new Proxy(phase(), {
      getOwnPropertyDescriptor: () => { throw new Error('synthetic descriptor failure') }
    })
    const revoked = Proxy.revocable(phase(), {})
    revoked.revoke()
    for (const input of [ownKeysFailure, descriptorFailure, revoked.proxy]) {
      expect(() => expect(normalizeNativeManualBeginPhaseV1(input)).toBeNull()).not.toThrow()
    }
  })

  it('rejects wrong roots, versions and nonexact enum values', () => {
    for (const input of [null, undefined, true, 1, 'capture', [], [phase()]]) {
      expect(normalizeNativeManualBeginPhaseV1(input)).toBeNull()
    }
    for (const version of [0, 2, '1', true, null, undefined, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(normalizeNativeManualBeginPhaseV1({ ...phase(), version })).toBeNull()
    }
    for (const value of ['unknown', 'Capture', 'capture ', '/private/RAW_CANARY', '', null, 1]) {
      expect(normalizeNativeManualBeginPhaseV1({ ...phase(), phase: value })).toBeNull()
      expect(normalizeNativeManualBeginPhaseV1({ ...phase(), errorKind: value })).toBeNull()
    }
  })
})

describe('native manual begin phase complete-line parser', () => {
  it('accepts a complete LF-terminated frame as either UTF-8 bytes or a string', () => {
    expect(NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1).toBe('ANALYTIX_NATIVE_MANUAL_BEGIN_PHASE_V1 ')
    expect(NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1).toBe(1024)
    const input = phase({ phase: 'prepared_contract', errorKind: 'operation_mismatch' })
    expect(parseNativeManualBeginPhaseLineV1(frame(input))).toEqual(input)
    expect(parseNativeManualBeginPhaseLineV1(Buffer.from(frame(input), 'utf8'))).toEqual(input)
  })

  it('counts the prefix and final LF in the inclusive 1024-byte limit', () => {
    const maximum = paddedFrame(1024)
    const overlong = paddedFrame(1025)
    expect(Buffer.byteLength(maximum, 'utf8')).toBe(1024)
    expect(Buffer.byteLength(overlong, 'utf8')).toBe(1025)
    for (const input of [maximum, Buffer.from(maximum)]) {
      expect(parseNativeManualBeginPhaseLineV1(input)).toEqual(phase())
    }
    for (const input of [overlong, Buffer.from(overlong)]) {
      expect(parseNativeManualBeginPhaseLineV1(input)).toBeNull()
    }
  })

  it('accepts reordered fields, escaped keys and single-line JSON whitespace', () => {
    const input = phase()
    const reordered = JSON.stringify(Object.fromEntries(Object.entries(input).reverse()))
    const spaced = ` \t{\t${reordered.slice(1, -1).replaceAll(',', ',\t')}\t} \t`
    for (const body of [
      reordered,
      spaced,
      JSON.stringify(input).replace('"phase":', '"\\u0070hase":')
    ]) {
      const line = `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}${body}\n`
      expect(parseNativeManualBeginPhaseLineV1(line)).toEqual(input)
      expect(parseNativeManualBeginPhaseLineV1(Buffer.from(line, 'utf8'))).toEqual(input)
    }
  })

  it('rejects duplicate JSON keys including equivalent escaped spellings', () => {
    const body = JSON.stringify(phase()).slice(0, -1)
    for (const duplicate of [
      '"version":1',
      '"phase":"capture"',
      '"\\u0070hase":"capture"',
      '"\\u0076ersion":1'
    ]) {
      const line = `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}${body},${duplicate}}\n`
      expect(parseNativeManualBeginPhaseLineV1(line)).toBeNull()
      expect(parseNativeManualBeginPhaseLineV1(Buffer.from(line, 'utf8'))).toBeNull()
    }
  })

  it('requires one exact prefix, one complete line and no raw trailing content', () => {
    const valid = frame()
    const body = JSON.stringify(phase())
    for (const input of [
      '',
      valid.slice(0, -1),
      `${valid}\n`,
      `${valid}${valid}`,
      valid.replace(/\n$/, '\r\n'),
      `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}{\r${body.slice(1)}\n`,
      `2026/10/15 00:00:00 ${valid}`,
      ` ${valid}`,
      valid.replace('ANALYTIX_NATIVE', 'analytix_native'),
      `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1.trimEnd()}${body}\n`,
      `${valid.slice(0, -1)} RAW_CANARY_DO_NOT_FORWARD\n`,
      `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}${body}{}\n`
    ]) {
      expect(parseNativeManualBeginPhaseLineV1(input)).toBeNull()
    }
  })

  it('rejects malformed JSON, unknown fields and values, and invalid UTF-8 bytes', () => {
    const missing: Record<string, unknown> = { ...phase() }
    delete missing.prepareOK
    for (const input of [
      `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}{\n`,
      frame(null),
      frame([]),
      frame(missing),
      frame({ ...phase(), phase: 'unlisted_phase' }),
      frame({ ...phase(), errorKind: 'unlisted_error' }),
      frame({ ...phase(), captureEntered: 'true' }),
      frame({ ...phase(), body: 'RAW_CANARY_DO_NOT_FORWARD' }),
      Buffer.from(`\uFEFF${frame()}`, 'utf8'),
      Buffer.from(`${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}\uFEFF${JSON.stringify(phase())}\n`, 'utf8'),
      Buffer.concat([
        Buffer.from(`${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}{"version":1,"phase":"`),
        Buffer.from([0xc0, 0xaf]),
        Buffer.from('"}\n')
      ]),
      Buffer.concat([Buffer.from(frame().slice(0, -1)), Buffer.from([0xff, 0x0a])])
    ]) {
      expect(parseNativeManualBeginPhaseLineV1(input)).toBeNull()
    }
  })
})

describe('native manual begin phase stderr stream', () => {
  it('waits for LF while accepting a frame split at every byte', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const input = phase({ phase: 'baseline_revision', errorKind: 'draft_stale' })
    const bytes = Buffer.from(frame(input), 'utf8')
    for (let index = 0; index < bytes.length - 1; index += 1) {
      stream.accept(bytes.subarray(index, index + 1))
      expect(received).toEqual([])
    }
    stream.accept(bytes.subarray(bytes.length - 1))
    expect(received).toEqual([input])
    stream.close()
  })

  it('handles coalesced frames in a chunk larger than the per-frame bound', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const expected = phases.map((value) => phase({ phase: value }))
    const input = expected.map((value) => frame(value)).join('')
    expect(Buffer.byteLength(input, 'utf8')).toBeGreaterThan(1024)
    stream.accept(Buffer.from(input))
    expect(received).toEqual(expected)
    stream.close()
  })

  it('joins mixed string and byte chunks and retains only the final partial line', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const first = phase({ phase: 'authority', errorKind: 'forbidden' })
    const second = phase({ phase: 'prepare', errorKind: 'persistence_failure' })
    const secondFrame = frame(second)
    stream.accept('ordinary stderr RAW_CANARY_DO_NOT_FORWARD\n')
    stream.accept(Buffer.from(`${frame(first)}${secondFrame.slice(0, 23)}`))
    expect(received).toEqual([first])
    stream.accept(secondFrame.slice(23, -1))
    expect(received).toEqual([first])
    stream.accept(Buffer.from('\n'))
    expect(received).toEqual([first, second])
    stream.close()
  })

  it('accepts the exact maximum frame and rejects the next byte before recovering', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const next = phase({ phase: 'input', errorKind: 'invalid_request' })
    stream.accept(`${paddedFrame(1024)}${paddedFrame(1025)}${frame(next)}`)
    expect(received).toEqual([phase(), next])
    stream.close()
  })

  it('includes a separately received LF in the maximum-frame limit', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    stream.accept(paddedFrame(1024).slice(0, -1))
    expect(received).toEqual([])
    stream.accept(Buffer.from('\n'))
    expect(received).toEqual([phase()])
    stream.accept(Buffer.from(paddedFrame(1025).slice(0, -1)))
    stream.accept('\n')
    expect(received).toEqual([phase()])
    stream.accept(frame())
    expect(received).toEqual([phase(), phase()])
    stream.close()
  })

  it('discards an overlong line until LF even if a valid prefix follows in another chunk', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const next = phase({ phase: 'dependencies', errorKind: 'unavailable' })
    stream.accept(Buffer.alloc(1024, 0x78))
    stream.accept(frame())
    expect(received).toEqual([])
    stream.accept(frame(next))
    expect(received).toEqual([next])
    stream.close()
  })

  it('recovers at LF inside a large chunk without treating a suffix as a new line', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const next = phase({ phase: 'capture_conflict', errorKind: 'conflict' })
    stream.accept(Buffer.from(`${'x'.repeat(8192)}${frame()}${frame(next)}`))
    expect(received).toEqual([next])
    stream.close()
  })

  it('rejects malformed, unknown and raw-canary lines without forwarding their contents', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const invalid = [
      `${NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1}RAW_CANARY_DO_NOT_FORWARD\n`,
      frame({ ...phase(), body: 'RAW_CANARY_DO_NOT_FORWARD' }),
      frame({ ...phase(), phase: 'RAW_CANARY_DO_NOT_FORWARD' }),
      frame({ ...phase(), errorKind: 'RAW_CANARY_DO_NOT_FORWARD' }),
      frame({ ...phase(), prepareOK: 'RAW_CANARY_DO_NOT_FORWARD' }),
      `${frame().slice(0, -1)} RAW_CANARY_DO_NOT_FORWARD\n`,
      frame().replace(/\n$/, '\r\n')
    ]
    stream.accept(invalid.join(''))
    stream.accept(Buffer.from([0xff, 0x0a]))
    expect(received).toEqual([])
    stream.accept(frame())
    expect(received).toEqual([phase()])
    stream.close()
  })

  it('handles multibyte string noise and byte-split UTF-8 without leaking rejected frames', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    const unicodeNoise = '界'.repeat(350)
    expect(unicodeNoise.length).toBeLessThan(1024)
    expect(Buffer.byteLength(unicodeNoise, 'utf8')).toBeGreaterThan(1024)
    stream.accept(unicodeNoise)
    stream.accept(frame())
    expect(received).toEqual([])

    const invalid = Buffer.from(frame({ ...phase(), body: '界' }), 'utf8')
    const multibyteOffset = invalid.indexOf(Buffer.from('界', 'utf8'))
    expect(multibyteOffset).toBeGreaterThan(0)
    stream.accept(invalid.subarray(0, multibyteOffset + 1))
    stream.accept(invalid.subarray(multibyteOffset + 1, multibyteOffset + 2))
    stream.accept(invalid.subarray(multibyteOffset + 2))
    expect(received).toEqual([])
    stream.accept(frame())
    expect(received).toEqual([phase()])
    stream.close()
  })

  it('continues processing later frames when the callback throws', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => {
      received.push(value)
      throw new Error('synthetic callback failure')
    })
    const second = phase({ phase: 'baseline_open', errorKind: 'baseline_failed' })
    const third = phase({ phase: 'prepare', errorKind: 'context_canceled' })
    expect(() => stream.accept(`${frame()}${frame(second)}`)).not.toThrow()
    expect(() => stream.accept(Buffer.from(frame(third)))).not.toThrow()
    expect(received).toEqual([phase(), second, third])
    expect(() => stream.close()).not.toThrow()
  })

  it('stops before the next coalesced frame when the callback closes the stream', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => {
      received.push(value)
      stream.close()
    })
    const later = phase({ phase: 'prepare', errorKind: 'context_deadline' })
    stream.accept(Buffer.from(`${frame()}${frame(later)}`))
    stream.accept(frame(later))
    expect(received).toEqual([phase()])
  })

  it('drops pending data on close and ignores all chunks after close', () => {
    const received: NativeManualBeginPhaseV1[] = []
    const stream = createNativeManualBeginPhaseStreamV1((value) => received.push(value))
    stream.accept(frame().slice(0, -1))
    expect(received).toEqual([])
    expect(() => stream.close()).not.toThrow()
    expect(() => stream.close()).not.toThrow()
    stream.accept('\n')
    stream.accept(frame())
    stream.accept(Buffer.from(frame()))
    expect(received).toEqual([])
  })
})
