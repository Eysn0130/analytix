import { TextDecoder } from 'node:util'

export const NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1 = 'ANALYTIX_NATIVE_MANUAL_BEGIN_PHASE_V1 '
export const NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1 = 1024

const phases = [
  'input', 'authority', 'dependencies', 'capture_conflict', 'capture',
  'baseline_open', 'baseline_session', 'baseline_revision', 'prepare', 'prepared_contract'
] as const
const errorKinds = [
  'invalid_request', 'session_invalid', 'scope_invalid', 'projection_unavailable',
  'draft_stale', 'conflict', 'persistence_failure', 'operation_mismatch',
  'operation_not_found', 'forbidden', 'not_text', 'too_large', 'capacity',
  'unavailable', 'capture_unavailable', 'unsafe_path', 'capture_session_mismatch',
  'opaque_execution', 'baseline_failed', 'context_canceled', 'context_deadline',
  'not_found', 'permission', 'other'
] as const
const booleanFields = [
  'manualServicePresent', 'captureBound', 'captureEntered', 'captureReused',
  'baselineEntered', 'baselineOpenOK', 'sessionMatch', 'revisionMatch',
  'captureAcquired', 'prepareEntered', 'prepareOK'
] as const
const fields = ['version', 'phase', 'errorKind', ...booleanFields] as const
const fieldSet: ReadonlySet<PropertyKey> = new Set<PropertyKey>(fields)
const phaseSet: ReadonlySet<string> = new Set<string>(phases)
const errorKindSet: ReadonlySet<string> = new Set<string>(errorKinds)
const prefix = Buffer.from(NATIVE_MANUAL_BEGIN_PHASE_PREFIX_V1, 'ascii')

export type NativeManualBeginPhaseV1 = Readonly<{
  version: 1
  phase: typeof phases[number]
  errorKind: typeof errorKinds[number]
  manualServicePresent: boolean
  captureBound: boolean
  captureEntered: boolean
  captureReused: boolean
  baselineEntered: boolean
  baselineOpenOK: boolean
  sessionMatch: boolean
  revisionMatch: boolean
  captureAcquired: boolean
  prepareEntered: boolean
  prepareOK: boolean
}>

/** Snapshot only the closed frame fields; unknown objects cannot invoke getters. */
export function normalizeNativeManualBeginPhaseV1(value: unknown): NativeManualBeginPhaseV1 | null {
  if (!value || typeof value !== 'object') return null
  try {
    if (Array.isArray(value)) return null
    const keys = Reflect.ownKeys(value)
    if (keys.length !== fields.length || keys.some(key => !fieldSet.has(key))) return null
    const data: Record<string, unknown> = {}
    for (const key of fields) {
      const descriptor = Object.getOwnPropertyDescriptor(value, key)
      if (!descriptor || !Object.hasOwn(descriptor, 'value')) return null
      data[key] = descriptor.value
    }
    if (data.version !== 1 || typeof data.phase !== 'string' || !phaseSet.has(data.phase) ||
        typeof data.errorKind !== 'string' || !errorKindSet.has(data.errorKind) ||
        booleanFields.some(key => typeof data[key] !== 'boolean')) return null
    return {
      version: 1,
      phase: data.phase as NativeManualBeginPhaseV1['phase'],
      errorKind: data.errorKind as NativeManualBeginPhaseV1['errorKind'],
      manualServicePresent: data.manualServicePresent as boolean,
      captureBound: data.captureBound as boolean,
      captureEntered: data.captureEntered as boolean,
      captureReused: data.captureReused as boolean,
      baselineEntered: data.baselineEntered as boolean,
      baselineOpenOK: data.baselineOpenOK as boolean,
      sessionMatch: data.sessionMatch as boolean,
      revisionMatch: data.revisionMatch as boolean,
      captureAcquired: data.captureAcquired as boolean,
      prepareEntered: data.prepareEntered as boolean,
      prepareOK: data.prepareOK as boolean
    }
  } catch {
    return null
  }
}

// JSON.parse has already validated the text. Keep distinct wire keys from
// silently overwriting one another, including differently escaped spellings.
function duplicateTopLevelKey(body: string): boolean {
  const seen = new Set<string>()
  let depth = 0
  let expectsKey = false
  for (let index = 0; index < body.length; index += 1) {
    const character = body[index]
    if (character === '"') {
      let end = index + 1
      for (; end < body.length; end += 1) {
        if (body[end] === '\\') end += 1
        else if (body[end] === '"') break
      }
      if (depth === 1 && expectsKey) {
        const key = JSON.parse(body.slice(index, end + 1)) as string
        if (seen.has(key)) return true
        seen.add(key)
        expectsKey = false
      }
      index = end
    } else if (character === '{' || character === '[') {
      depth += 1
      if (depth === 1 && character === '{') expectsKey = true
    } else if (character === '}' || character === ']') {
      depth -= 1
    } else if (character === ',' && depth === 1) {
      expectsKey = true
    }
  }
  return false
}

/** A frame includes its exact prefix and its one terminating LF. */
export function parseNativeManualBeginPhaseLineV1(line: Buffer | string): NativeManualBeginPhaseV1 | null {
  if (typeof line === 'string' && line.length > NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1) return null
  if (typeof line !== 'string' && !Buffer.isBuffer(line)) return null
  const bytes = typeof line === 'string' ? Buffer.from(line, 'utf8') : line
  if (bytes.length > NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1 ||
      bytes.length <= prefix.length + 1 || bytes.indexOf(10) !== bytes.length - 1 ||
      bytes.includes(13) || !bytes.subarray(0, prefix.length).equals(prefix)) return null
  try {
    const body = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes.subarray(prefix.length, -1))
    const phase = normalizeNativeManualBeginPhaseV1(JSON.parse(body))
    return phase && !duplicateTopLevelKey(body) ? phase : null
  } catch {
    return null
  }
}

/** Retain at most one bounded partial line; overflow is discarded through LF. */
export function createNativeManualBeginPhaseStreamV1(
  onPhase: (phase: NativeManualBeginPhaseV1) => void
): Readonly<{ accept(chunk: Buffer | string): void; close(): void }> {
  const pending = Buffer.alloc(NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1)
  let length = 0
  let dropping = false
  let closed = false

  const reset = () => { pending.fill(0); length = 0; dropping = false }
  const finish = () => {
    let phase: NativeManualBeginPhaseV1 | null = null
    if (!dropping) {
      pending[length] = 10
      phase = parseNativeManualBeginPhaseLineV1(pending.subarray(0, length + 1))
    }
    reset()
    if (phase) {
      try { onPhase(phase) } catch { /* Optional diagnostics cannot affect child lifecycle. */ }
    }
  }

  return {
    accept(chunk) {
      if (closed || typeof chunk !== 'string' && !Buffer.isBuffer(chunk)) return
      let offset = 0
      while (!closed && offset < chunk.length) {
        const newline = typeof chunk === 'string' ? chunk.indexOf('\n', offset) : chunk.indexOf(10, offset)
        const end = newline < 0 ? chunk.length : newline
        if (!dropping) {
          const available = NATIVE_MANUAL_BEGIN_PHASE_MAX_FRAME_BYTES_V1 - 1 - length
          // UTF-8 cannot use fewer bytes than string code units. Avoid copying
          // an unbounded string segment merely to discover that it is too long.
          if (end - offset > available) {
            pending.fill(0); length = 0; dropping = true
          } else {
            const part = typeof chunk === 'string' ? Buffer.from(chunk.slice(offset, end), 'utf8') : chunk.subarray(offset, end)
            if (part.length > available) {
              pending.fill(0); length = 0; dropping = true
            } else {
              part.copy(pending, length)
              length += part.length
            }
          }
        }
        if (newline < 0) break
        finish()
        offset = end + 1
      }
    },
    close() {
      closed = true
      reset()
    }
  }
}
