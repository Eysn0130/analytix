import {
  TYPED_LOCAL_DATA_SURFACE_KINDS_V1,
  TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1
} from '../../packages/runtime/src/contracts/typed-local-data-surface.js'

const MAX_SERIALIZED_JSON_BYTES = 1024 * 1024
const MAX_INSPECTION_DEPTH = 64
const MAX_INSPECTION_NODES = 100_000
const MAX_SERIALIZED_JSON_CANDIDATES = 32

const RESTRICTED_TYPED_LOCAL_RESPONSE_KINDS_V1 = new Set<string>(
  TYPED_LOCAL_DATA_SURFACE_KINDS_V1
)

const EXACT_RESTRICTED_PURPOSES = new Set([
  'analytix.raw-artifact-acquisition-intent/v1',
  'analytix.raw-artifact-content-chunk-descriptor/v1',
  'analytix.raw-artifact-content-index-page-descriptor/v1',
  'analytix.raw-artifact-content-index-page/v1',
  'analytix.raw-artifact-content-root/v1',
  'analytix.raw-artifact-entry/v1',
  'analytix.raw-artifact-manifest-page-descriptor/v1',
  'analytix.raw-artifact-manifest-page/v1',
  'analytix.raw-artifact-manifest/v1',
  'analytix.raw-artifact-source-locator/v1',
  'analytix.parsed-generation-identity/v1',
  'analytix.parsed-generation-receipt/v1',
  'analytix.parsed-outcome/v1',
  'analytix.parsed-page-descriptor/v1',
  'analytix.parsed-page-index-descriptor/v1',
  'analytix.parsed-page-index/v1',
  'analytix.parsed-page/v1',
  'analytix.source-row-ledger-index-page-descriptor/v1',
  'analytix.source-row-ledger-index-page/v1',
  'analytix.source-row-ledger-page-descriptor/v1',
  'analytix.source-row-ledger-page-entry/v1',
  'analytix.source-row-ledger-page/v1',
  'analytix.source-row-ledger-root/v1',
  'analytix.source-row-lineage/v1',
  'analytix.source-row-locator/v1',
  'analytix.source-row-record/v1',
  'analytix.source-row-witness/v1',
  'analytix.dataset-snapshot-authority/v1',
  'analytix.dataset-snapshot-authority/v2',
  'analytix.dataset-snapshot-index/v1',
  'analytix.dataset-snapshot-manifest/v2',
  'analytix.source-field-binding/v2',
  'analytix.canonical-evidence/v2',
  'analytix.canonical-evidence/v3'
])

const RESTRICTED_EXACT_REFERENCE_TRIPLES = [
  ['rawartifactmanifestdigest', 'rawartifactmanifestsha256', 'rawartifactmanifestbytelength'],
  ['parsedgenerationreceiptdigest', 'parsedgenerationreceiptsha256', 'parsedgenerationreceiptbytelength'],
  ['classificationledgerdigest', 'classificationledgersha256', 'classificationledgerbytelength'],
  ['sourcerowledgerrootdigest', 'sourcerowledgerrootsha256', 'sourcerowledgerrootbytelength'],
  ['lineagedigest', 'lineagesha256', 'lineagebytelength'],
  ['acquisitionintentdigest', 'acquisitionintentsha256', 'acquisitionintentbytelength'],
  ['sourcelocatordigest', 'sourcelocatorsha256', 'sourcelocatorbytelength'],
  ['contentrootdigest', 'contentrootsha256', 'contentrootbytelength'],
  ['rawartifactmanifestpagedigest', 'rawartifactmanifestpagesha256', 'rawartifactmanifestpagebytelength'],
  ['rawartifactentrydigest', 'rawartifactentrysha256', 'rawartifactentrybytelength'],
  ['rawsourcelocatordigest', 'rawsourcelocatorsha256', 'rawsourcelocatorbytelength'],
  ['parsedpagedigest', 'parsedpagesha256', 'parsedpagebytelength']
] as const

type InspectionState = {
  nodes: number
  active: WeakSet<object>
}

function normalizedKey(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]/g, '')
}

function isRestrictedPurpose(value: unknown): boolean {
  if (typeof value !== 'string') return false
  const purpose = value.trim().toLowerCase()
  return EXACT_RESTRICTED_PURPOSES.has(purpose)
}

export function isRestrictedEvidenceObject(record: Record<string, unknown>): boolean {
  if (
    record.schemaVersion === TYPED_LOCAL_DATA_SURFACE_RESPONSE_SCHEMA_VERSION_V1 &&
    typeof record.kind === 'string' &&
    RESTRICTED_TYPED_LOCAL_RESPONSE_KINDS_V1.has(record.kind)
  ) return true
  const values = new Map<string, unknown[]>()
  for (const [key, value] of Object.entries(record)) {
    const normalized = normalizedKey(key)
    values.set(normalized, [...(values.get(normalized) ?? []), value])
    if (normalized === 'purpose' && isRestrictedPurpose(value)) return true
  }
  if (RESTRICTED_EXACT_REFERENCE_TRIPLES.some(([digest, sha256, byteLength]) =>
    hasSHA256(values, digest) && hasSHA256(values, sha256) && hasPositiveJSONSafeInteger(values, byteLength)
  )) return true
  return hasNonEmptyString(values, 'sourceexactvalue') && hasSHA256(values, 'sourceexactvaluesha256') && hasSHA256(values, 'bindingdigest') ||
    hasSHA256(values, 'rawartifactsha256') && hasSHA256(values, 'sourcerecordsha256') && hasSHA256(values, 'sourceexactvaluesha256') ||
    hasNonEmptyArray(values, 'facts') && hasNonEmptyArray(values, 'sourcefieldbindings') && hasSHA256(values, 'sourcefieldbindingsetdigest') ||
    hasNonEmptyArray(values, 'facts') && hasNonEmptyArray(values, 'acceptedslotsourcebindings') && hasSHA256(values, 'acceptedslotsourcebindingsetdigest') ||
    hasNonEmptyString(values, 'sourcerecordid') && hasNonEmptyString(values, 'sourcefileid') &&
      hasPositiveJSONSafeInteger(values, 'sourcerownumber') && hasNonEmptyString(values, 'field') && hasSHA256(values, 'bindingdigest') ||
    hasNonEmptyString(values, 'datasetsnapshotid') && hasSHA256(values, 'manifestdigest') && hasSHA256(values, 'manifestsha256') &&
      hasPositiveJSONSafeInteger(values, 'manifestbytelength') && hasNonEmptyString(values, 'authoritysignature') && hasSHA256(values, 'recorddigest') ||
    hasSHA256(values, 'snapshotauthorityrecorddigest') && hasSHA256(values, 'ledgerrootdigest') &&
      hasSHA256(values, 'ledgerindexpagedigest') && hasSHA256(values, 'ledgerpagedigest') && hasSHA256(values, 'witnessdigest') ||
    hasSHA256(values, 'sourcefileiddigest') && hasPositiveJSONSafeInteger(values, 'sourcerownumber') && hasSHA256(values, 'locatordigest')
}

function hasSHA256(values: Map<string, unknown[]>, key: string): boolean {
  return (values.get(key) ?? []).some(
    (value) => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
  )
}

function hasPositiveJSONSafeInteger(values: Map<string, unknown[]>, key: string): boolean {
  return (values.get(key) ?? []).some((value) => {
    if (typeof value === 'number') return Number.isSafeInteger(value) && value > 0
    if (typeof value !== 'string' || !/^[0-9]+$/.test(value.trim())) return false
    const parsed = Number(value.trim())
    return Number.isSafeInteger(parsed) && parsed > 0
  })
}

function hasNonEmptyString(values: Map<string, unknown[]>, key: string): boolean {
  return (values.get(key) ?? []).some(
    (value) => typeof value === 'string' && value.trim().length > 0
  )
}

function hasNonEmptyArray(values: Map<string, unknown[]>, key: string): boolean {
  return (values.get(key) ?? []).some((value) => Array.isArray(value) && value.length > 0)
}

function exceedsUTF8ByteBudget(value: string, maximum: number): boolean {
  let bytes = 0
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0
    bytes += codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4
    if (bytes > maximum) return true
  }
  return false
}

type BalancedJSONCandidateScan =
  | { status: 'balanced'; endIndex: number }
  | { status: 'not-balanced' }
  | { status: 'depth-budget-exceeded' }

function scanBalancedJSONCandidate(
  value: string,
  startIndex: number,
  maximumDepth: number
): BalancedJSONCandidateScan {
  if (maximumDepth <= 0) return { status: 'depth-budget-exceeded' }
  const closingStack: string[] = []
  let inString = false
  let escaped = false
  for (let index = startIndex; index < value.length; index += 1) {
    const character = value[index]
    if (inString) {
      if (escaped) {
        escaped = false
      } else if (character === '\\') {
        escaped = true
      } else if (character === '"') {
        inString = false
      }
      continue
    }
    if (character === '"') {
      inString = true
      continue
    }
    if (character === '{' || character === '[') {
      closingStack.push(character === '{' ? '}' : ']')
      if (closingStack.length > maximumDepth) return { status: 'depth-budget-exceeded' }
      continue
    }
    if (character !== '}' && character !== ']') continue
    if (closingStack.length === 0 || closingStack[closingStack.length - 1] !== character) {
      return { status: 'not-balanced' }
    }
    closingStack.pop()
    if (closingStack.length === 0) return { status: 'balanced', endIndex: index }
  }
  return { status: 'not-balanced' }
}

class StrictJSONKeyScanner {
  private index = 0

  constructor(private readonly value: string) {}

  hasDuplicateKeys(): boolean {
    try {
      const duplicate = this.scanValue()
      this.skipWhitespace()
      return duplicate || this.index !== this.value.length
    } catch {
      return true
    }
  }

  private scanValue(): boolean {
    this.skipWhitespace()
    const current = this.value[this.index]
    if (current === '{') return this.scanObject()
    if (current === '[') return this.scanArray()
    if (current === '"') {
      this.scanString()
      return false
    }
    const start = this.index
    while (this.index < this.value.length && !/[\s,}\]]/.test(this.value[this.index] ?? '')) {
      this.index += 1
    }
    if (this.index === start) throw new Error('invalid JSON value')
    return false
  }

  private scanObject(): boolean {
    this.index += 1
    this.skipWhitespace()
    if (this.value[this.index] === '}') {
      this.index += 1
      return false
    }
    const keys = new Set<string>()
    let duplicate = false
    while (this.index < this.value.length) {
      this.skipWhitespace()
      const key = this.scanString()
      if (keys.has(key)) duplicate = true
      keys.add(key)
      this.skipWhitespace()
      if (this.value[this.index] !== ':') throw new Error('missing JSON object colon')
      this.index += 1
      duplicate = this.scanValue() || duplicate
      this.skipWhitespace()
      const separator = this.value[this.index]
      this.index += 1
      if (separator === '}') return duplicate
      if (separator !== ',') throw new Error('invalid JSON object separator')
    }
    throw new Error('incomplete JSON object')
  }

  private scanArray(): boolean {
    this.index += 1
    this.skipWhitespace()
    if (this.value[this.index] === ']') {
      this.index += 1
      return false
    }
    let duplicate = false
    while (this.index < this.value.length) {
      duplicate = this.scanValue() || duplicate
      this.skipWhitespace()
      const separator = this.value[this.index]
      this.index += 1
      if (separator === ']') return duplicate
      if (separator !== ',') throw new Error('invalid JSON array separator')
    }
    throw new Error('incomplete JSON array')
  }

  private scanString(): string {
    if (this.value[this.index] !== '"') throw new Error('invalid JSON string')
    const start = this.index
    this.index += 1
    let escaped = false
    while (this.index < this.value.length) {
      const current = this.value[this.index]
      this.index += 1
      if (escaped) {
        escaped = false
      } else if (current === '\\') {
        escaped = true
      } else if (current === '"') {
        return JSON.parse(this.value.slice(start, this.index)) as string
      }
    }
    throw new Error('incomplete JSON string')
  }

  private skipWhitespace(): void {
    while (/\s/.test(this.value[this.index] ?? '')) this.index += 1
  }
}

function containsSerializedRestrictedEvidence(
  value: string,
  depth: number,
  state: InspectionState
): boolean {
  if (exceedsUTF8ByteBudget(value, MAX_SERIALIZED_JSON_BYTES)) return true
  let candidateCount = 0
  for (let index = 0; index < value.length; index += 1) {
    if (value[index] !== '{' && value[index] !== '[') continue
    candidateCount += 1
    if (candidateCount > MAX_SERIALIZED_JSON_CANDIDATES) return true
    const scan = scanBalancedJSONCandidate(value, index, MAX_INSPECTION_DEPTH - depth)
    if (scan.status === 'depth-budget-exceeded') return true
    if (scan.status !== 'balanced') continue
    const candidate = value.slice(index, scan.endIndex + 1)
    try {
      const decoded: unknown = JSON.parse(candidate)
      if (new StrictJSONKeyScanner(candidate).hasDuplicateKeys()) return true
      if (containsRestrictedEvidenceInternal(decoded, depth + 1, state)) return true
      index = scan.endIndex
    } catch {
      // A balanced prose fragment can contain a later nested JSON value, so
      // continue from the next opening under the same candidate budget.
    }
  }
  return false
}

function containsRestrictedEvidenceInternal(
  value: unknown,
  depth: number,
  state: InspectionState
): boolean {
  if (depth > MAX_INSPECTION_DEPTH || state.nodes <= 0) return true
  state.nodes -= 1
  if (typeof value === 'string') return containsSerializedRestrictedEvidence(value, depth, state)
  if (!value || typeof value !== 'object') return false
  if (state.active.has(value)) return true
  state.active.add(value)
  try {
    if (Array.isArray(value)) {
      return value.some((entry) => containsRestrictedEvidenceInternal(entry, depth + 1, state))
    }
    const record = value as Record<string, unknown>
    if (isRestrictedEvidenceObject(record)) return true
    return Object.values(record)
      .some((entry) => containsRestrictedEvidenceInternal(entry, depth + 1, state))
  } finally {
    state.active.delete(value)
  }
}

/**
 * Returns true for private evidence contracts or values that cannot be
 * inspected within fixed bounds. Plain SHA-256 values and a standalone
 * datasetSnapshotId remain valid public metadata.
 */
export function containsRestrictedEvidence(value: unknown): boolean {
  return containsRestrictedEvidenceInternal(value, 0, {
    nodes: MAX_INSPECTION_NODES,
    active: new WeakSet<object>()
  })
}
