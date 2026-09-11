import { createHash, randomBytes } from 'node:crypto'
import { BrowserWindow, MessageChannelMain } from 'electron'
import type {
  ControlledArtifactCommitReceiptV2,
  ControlledArtifactInvocationContextV2,
  ControlledArtifactPreparedReleaseV2,
  ControlledArtifactReleaseEffectInputV2,
  ControlledArtifactReleasePreparationV2
} from './host-v2'
import { parseStrictJsonObject } from './strict-json'

const DISPLAY_EFFECT_ERROR = 'controlled_artifact_display_effect_v2_failed'
const DISPLAY_TARGET_PURPOSE = 'analytix.controlled-artifact-display-target/v2'
const DISPLAY_TARGET_DOMAIN = Buffer.from(`${DISPLAY_TARGET_PURPOSE}\0`, 'utf8')
const CONTROLLED_ARTIFACT_PURPOSE = 'analytix.controlled-pii-artifact/v1'
const CONTROLLED_ARTIFACT_MEDIA_TYPE =
  'application/vnd.analytix.controlled-case-evidence+json'
const DISPLAY_MODEL_PURPOSE = 'analytix.controlled-artifact-display-model/v2'
const DISPLAY_PORT_PURPOSE = 'analytix.controlled-artifact-display-port/v2'
const DISPLAY_PAYLOAD_PURPOSE = 'analytix.controlled-artifact-display-payload/v2'
const DISPLAY_ACK_PURPOSE = 'analytix.controlled-artifact-display-ack/v2'
const TRUSTED_SINK_MARKER_V2 = 'trusted-sink-v2'
const REAUTHORIZATION_STATE_MARKER_V2 = 'reauthorization-v2'
const MAX_ARTIFACT_BYTES = 8 << 20
const MAX_FIELDS = 256
const MAX_STRING_BYTES = 16 << 10
const SHA256_PATTERN = /^[a-f0-9]{64}$/u
const DATASET_PATTERN = /^dsv2_[a-f0-9]{64}$/u
const TARGET_NONCE_PATTERN = /^[A-Za-z0-9_-]{43}$/u
const CLAIM_TYPE_PATTERN = /^[a-z][a-z0-9_]{0,63}$/u
const FIELD_NAME_PATTERN = /^[A-Za-z][A-Za-z0-9_]{0,127}$/u
const EVIDENCE_ID_PATTERN = /^evr_[a-f0-9]{64}$/u
const CASE_ENTITY_REFERENCE_PATTERN = /cer1_[a-p]{64}/u
const UNSIGNED_DECIMAL_PATTERN = /^(?:0|[1-9][0-9]*)$/u
const SIGNED_DECIMAL_PATTERN = /^(?:0|-?[1-9][0-9]*)$/u
const CURRENCY_PATTERN = /^[A-Z]{3}$/u
const ACCOUNT_FLOW_PURPOSE = 'analytix.controlled-account-flow-analysis/v1'
const MAX_ACCOUNT_FLOW_ANALYSES = 256
const MAX_ACCOUNT_FLOW_TRANSACTIONS = 512

const ARTIFACT_KEYS = [
  'claimLedgerDigest',
  'context',
  'deliveryScope',
  'disclosurePurpose',
  'fields',
  'mediaType',
  'preservedControlledFieldCount',
  'projectionRulesetHash',
  'purpose',
  'renderedAt',
  'requesterUserId',
  'schemaVersion',
  'targetIdentityDigest'
] as const

const ARTIFACT_WITH_ACCOUNT_FLOW_KEYS = [
  ...ARTIFACT_KEYS,
  'accountFlowAnalyses'
] as const

const CONTEXT_KEYS = [
  'caseBindingHash',
  'caseId',
  'contextDigest',
  'contextEpoch',
  'contextIssuedAt',
  'datasetSnapshotId',
  'sourceManifestHash',
  'tenantId',
  'threadId',
  'turnId',
  'userId',
  'version',
  'workspaceRealPath'
] as const

const FIELD_KEYS = [
  'claimId',
  'claimRecordDigest',
  'claimType',
  'evidenceReceiptIds',
  'exactValue',
  'fieldName',
  'piiClass',
  'valueSha256'
] as const

const ACCOUNT_FLOW_KEYS = [
  'accountValueSha256',
  'currency',
  'endAt',
  'evidenceReceiptIds',
  'evidenceTransactionCount',
  'inflowMinor',
  'minorUnitScale',
  'netMinor',
  'outflowMinor',
  'purpose',
  'schemaVersion',
  'startAt',
  'transactionCount',
  'transactions'
] as const

const ACCOUNT_FLOW_TRANSACTION_KEYS = [
  'amountMinor',
  'currency',
  'direction',
  'evidenceRowOrdinal',
  'minorUnitScale',
  'occurredAt'
] as const

export type ControlledArtifactDisplayFieldV2 = Readonly<{
  label: string
  exactValue: string
  evidenceReceiptCount: number
}>

export type ControlledArtifactDisplayAccountFlowTransactionV2 = Readonly<{
  occurredAt: string
  direction: '流入' | '流出'
  amount: string
}>

export type ControlledArtifactDisplayAccountFlowV2 = Readonly<{
  summary: string
  accountLabel: string
  exactAccount: string
  startAt: string
  endAt: string
  inflow: string
  outflow: string
  net: string
  transactionCount: string
  evidenceTransactionCount: string
  evidenceReceiptCount: number
  evidenceReceiptSetDigest: string
  transactions: readonly ControlledArtifactDisplayAccountFlowTransactionV2[]
}>

export type ControlledArtifactDisplayModelV2 = Readonly<{
  schemaVersion: 2
  purpose: typeof DISPLAY_MODEL_PURPOSE
  title: string
  notice: string
  accountFlows: readonly ControlledArtifactDisplayAccountFlowV2[]
  fields: readonly ControlledArtifactDisplayFieldV2[]
}>

type ParsedControlledArtifactFieldV2 = Readonly<{
  model: ControlledArtifactDisplayFieldV2
  piiClass: string
  valueSha256: string
  exactValue: string
  evidenceReceiptIds: readonly string[]
}>

export type ControlledArtifactDisplaySessionV2 = Readonly<{
  committedAt: string
  revoke: () => void
  close: () => void
}>

export type ControlledArtifactDisplaySinkV2 = Readonly<{
  open: (input: Readonly<{
    model: ControlledArtifactDisplayModelV2
    modelDigest: string
    authorizedUntil: string
    isAuthorityCurrent: () => boolean
    signal: AbortSignal
  }>) => Promise<ControlledArtifactDisplaySessionV2>
}>

export type ControlledArtifactDisplayTargetV2Options = Readonly<{
  webContentsId: number
  rendererGeneration: number
  isRendererCurrent: (webContentsId: number, rendererGeneration: number) => boolean
  sink?: ControlledArtifactDisplaySinkV2
  random?: (size: number) => Buffer
  now?: () => string
}>

export class ControlledArtifactDisplayTargetV2 {
  private state: 'ready' | 'prepared' | 'committed' | 'revoked' | 'closed' = 'ready'
  private readonly nonce: string
  private readonly digest: string
  private session: ControlledArtifactDisplaySessionV2 | null = null
  private invocationContext: ControlledArtifactInvocationContextV2 | null = null
  private readonly sink: ControlledArtifactDisplaySinkV2
  private readonly now: () => string

  constructor(private readonly options: ControlledArtifactDisplayTargetV2Options) {
    if (!Number.isSafeInteger(options.webContentsId) || options.webContentsId <= 0 ||
      !Number.isSafeInteger(options.rendererGeneration) || options.rendererGeneration <= 0 ||
      typeof options.isRendererCurrent !== 'function') {
      throw fixedDisplayEffectError()
    }
    const random = options.random ?? randomBytes
    const nonceBytes = random(32)
    if (!Buffer.isBuffer(nonceBytes) || nonceBytes.length !== 32) {
      nonceBytes?.fill?.(0)
      throw fixedDisplayEffectError()
    }
    try {
      this.nonce = nonceBytes.toString('base64url')
    } finally {
      nonceBytes.fill(0)
    }
    if (!TARGET_NONCE_PATTERN.test(this.nonce)) throw fixedDisplayEffectError()
    this.digest = createHash('sha256')
      .update(DISPLAY_TARGET_DOMAIN)
      .update(String(options.webContentsId), 'utf8')
      .update('\0', 'utf8')
      .update(String(options.rendererGeneration), 'utf8')
      .update('\0', 'utf8')
      .update(this.nonce, 'utf8')
      .digest('hex')
    this.sink = options.sink ?? electronControlledArtifactDisplaySinkV2()
    this.now = options.now ?? currentTimestamp
  }

  targetIdentityDigest(): string {
    return this.digest
  }

  bindInvocationContext(context: ControlledArtifactInvocationContextV2): void {
    if (this.state !== 'ready' || this.invocationContext ||
      !validInvocationContextV2(context)) {
      throw fixedDisplayEffectError()
    }
    this.invocationContext = Object.freeze({
      threadId: context.threadId,
      turnId: context.turnId,
      contextEpoch: context.contextEpoch,
      contextDigest: context.contextDigest
    })
  }

  createPreparation(): ControlledArtifactReleasePreparationV2 {
    if (this.state !== 'ready' || !this.invocationContext || !this.isAuthorityCurrent()) {
      throw fixedDisplayEffectError()
    }
    this.state = 'prepared'
    return async (input, signal) => {
      if (this.state !== 'prepared' || !(signal instanceof AbortSignal) || signal.aborted ||
        !this.isAuthorityCurrent()) {
        throw fixedDisplayEffectError()
      }
      const parsed = parseControlledArtifactDisplayModelV2(
        input,
        this.digest,
        this.invocationContext ?? undefined
      )
      return new PreparedControlledArtifactDisplayV2(
        parsed.model,
        parsed.modelDigest,
        input,
        this.sink,
        () => this.isAuthorityCurrent(),
        this.now,
        (session) => {
          this.session = session
          this.state = 'committed'
        },
        () => {
          this.revoke()
        }
      )
    }
  }

  revoke(): void {
    if (this.state === 'closed' || this.state === 'revoked') return
    this.state = 'revoked'
    this.session?.revoke()
  }

  close(): void {
    if (this.state === 'closed') return
    this.state = 'closed'
    this.session?.close()
    this.session = null
  }

  toJSON(): never {
    throw fixedDisplayEffectError()
  }

  private isAuthorityCurrent(): boolean {
    if (this.state === 'revoked' || this.state === 'closed') return false
    try {
      return this.options.isRendererCurrent(
        this.options.webContentsId,
        this.options.rendererGeneration
      ) === true
    } catch {
      return false
    }
  }
}

class PreparedControlledArtifactDisplayV2 implements ControlledArtifactPreparedReleaseV2 {
  private state: 'ready' | 'committing' | 'committed' | 'aborted' | 'indeterminate' = 'ready'
  private operationTail: Promise<void> = Promise.resolve()

  constructor(
    private model: ControlledArtifactDisplayModelV2 | null,
    private readonly modelDigest: string,
    private readonly input: ControlledArtifactReleaseEffectInputV2,
    private readonly sink: ControlledArtifactDisplaySinkV2,
    private readonly isAuthorityCurrent: () => boolean,
    private readonly now: () => string,
    private readonly onCommitted: (session: ControlledArtifactDisplaySessionV2) => void,
    private readonly onAborted: () => void
  ) {}

  commitBefore(
    authorizedUntil: string,
    signal: AbortSignal
  ): Promise<ControlledArtifactCommitReceiptV2> {
    return this.runExclusive(async () => {
      if (this.state !== 'ready' || !this.model ||
        authorizedUntil !== this.input.receipt.authorizedUntil ||
        !this.isAuthorityCurrent()) {
        throw fixedDisplayEffectError()
      }
      requireBeforeDeadline(this.now, authorizedUntil, signal)
      this.state = 'committing'
      const model = this.model
      this.model = null
      try {
        const session = await this.sink.open({
          model,
          modelDigest: this.modelDigest,
          authorizedUntil,
          isAuthorityCurrent: this.isAuthorityCurrent,
          signal
        })
        const observedAt = requireBeforeDeadline(this.now, authorizedUntil, signal)
        if (!session || typeof session.revoke !== 'function' ||
          typeof session.close !== 'function' ||
          !validTimestamp(session.committedAt) ||
          Date.parse(session.committedAt) > Date.parse(observedAt) ||
          Date.parse(session.committedAt) >= Date.parse(authorizedUntil) ||
          !this.isAuthorityCurrent()) {
          session?.close?.()
          throw fixedDisplayEffectError()
        }
        const committedAt = session.committedAt
        this.state = 'committed'
        this.onCommitted(session)
        return Object.freeze({
          releaseTargetIdentityDigest: this.input.releaseTargetIdentityDigest,
          artifactSha256: this.input.receipt.artifactSha256,
          artifactByteLength: this.input.receipt.artifactByteLength,
          mediaType: this.input.receipt.mediaType,
          committedAt
        })
      } catch {
        this.state = 'indeterminate'
        throw fixedDisplayEffectError()
      }
    })
  }

  abort(signal: AbortSignal): Promise<void> {
    return this.runExclusive(async () => {
      if (this.state === 'aborted') return
      if (!(signal instanceof AbortSignal) || signal.aborted) {
        this.state = 'indeterminate'
        this.onAborted()
        throw fixedDisplayEffectError()
      }
      this.model = null
      this.state = 'aborted'
      this.onAborted()
    })
  }

  private async runExclusive<T>(operation: () => Promise<T>): Promise<T> {
    const previous = this.operationTail
    let release!: () => void
    this.operationTail = new Promise<void>((resolve) => { release = resolve })
    await previous
    try {
      return await operation()
    } finally {
      release()
    }
  }
}

export function parseControlledArtifactDisplayModelV2(
  input: ControlledArtifactReleaseEffectInputV2,
  expectedReleaseTargetIdentityDigest: string,
  expectedContext?: ControlledArtifactInvocationContextV2
): Readonly<{ model: ControlledArtifactDisplayModelV2; modelDigest: string }> {
  try {
    if (!input || input.action !== 'display' || input.receipt?.accessAction !== 'display' ||
      !Buffer.isBuffer(input.body) || input.body.length === 0 ||
      input.body.length > MAX_ARTIFACT_BYTES ||
      input.receipt.artifactByteLength !== input.body.length ||
      input.receipt.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE ||
      input.releaseTargetIdentityDigest !== expectedReleaseTargetIdentityDigest ||
      !SHA256_PATTERN.test(expectedReleaseTargetIdentityDigest) ||
      sha256(input.body) !== input.receipt.artifactSha256) {
      throw fixedDisplayEffectError()
    }
    const artifact = parseStrictJsonObject(input.body, {
      maxBytes: MAX_ARTIFACT_BYTES,
      maxDepth: 10,
      maxTokens: 200_000,
      maxStringBytes: MAX_STRING_BYTES
    })
    if ((!sameKeys(artifact, ARTIFACT_KEYS) &&
        !sameKeys(artifact, ARTIFACT_WITH_ACCOUNT_FLOW_KEYS)) ||
      !Buffer.from(JSON.stringify(artifact), 'utf8').equals(input.body) ||
      artifact.schemaVersion !== 1 || artifact.purpose !== CONTROLLED_ARTIFACT_PURPOSE ||
      artifact.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE ||
      artifact.disclosurePurpose !== 'case_report' ||
      artifact.deliveryScope !== 'controlled_artifact' ||
      artifact.claimLedgerDigest !== input.receipt.claimLedgerDigest ||
      artifact.targetIdentityDigest !== input.receipt.targetIdentityDigest ||
      !SHA256_PATTERN.test(String(artifact.projectionRulesetHash)) ||
      artifact.requesterUserId !== input.receipt.requesterUserId ||
      !validTimestamp(artifact.renderedAt) ||
      !sameControlledContextV2(artifact.context, input.receipt.context) ||
      (expectedContext !== undefined &&
        !sameInvocationContextV2(input.receipt.context, expectedContext)) ||
      !Array.isArray(artifact.fields) || artifact.fields.length === 0 ||
      artifact.fields.length > MAX_FIELDS ||
      artifact.preservedControlledFieldCount !== artifact.fields.length) {
      throw fixedDisplayEffectError()
    }
    const parsedFields = artifact.fields.map((field) => displayFieldV2(field))
    const fields = parsedFields.map((field) => field.model)
    const accountFlows = parseAccountFlowAnalysesV2(
      artifact.accountFlowAnalyses,
      parsedFields
    )
    const model: ControlledArtifactDisplayModelV2 = Object.freeze({
      schemaVersion: 2,
      purpose: DISPLAY_MODEL_PURPOSE,
      title: '受控案件研判结果',
      notice: '以下完整标识仅在本次受控授权内显示。',
      accountFlows,
      fields: Object.freeze(fields)
    })
    if (CASE_ENTITY_REFERENCE_PATTERN.test(JSON.stringify(model))) {
      throw fixedDisplayEffectError()
    }
    return Object.freeze({
      model,
      modelDigest: sha256(Buffer.from(JSON.stringify(model), 'utf8'))
    })
  } catch {
    throw fixedDisplayEffectError()
  }
}

function displayFieldV2(value: unknown): ParsedControlledArtifactFieldV2 {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw fixedDisplayEffectError()
  }
  const field = value as Record<string, unknown>
  if (!sameKeys(field, FIELD_KEYS) || !canonicalText(field.piiClass, 128) ||
    !canonicalText(field.claimId, 256) || !SHA256_PATTERN.test(String(field.claimRecordDigest)) ||
    !CLAIM_TYPE_PATTERN.test(String(field.claimType)) ||
    !FIELD_NAME_PATTERN.test(String(field.fieldName)) ||
    !canonicalText(field.exactValue, MAX_STRING_BYTES) ||
    !SHA256_PATTERN.test(String(field.valueSha256)) ||
    // Go has already verified this opaque join as a case-scoped entity/value
    // binding.  A bare hash of the complete identifier would be a stable
    // cross-case correlation token and is therefore explicitly rejected here.
    sha256(Buffer.from(String(field.exactValue), 'utf8')) === field.valueSha256 ||
    !Array.isArray(field.evidenceReceiptIds) || field.evidenceReceiptIds.length === 0 ||
    field.evidenceReceiptIds.length > 4096 ||
    !field.evidenceReceiptIds.every((id) => typeof id === 'string' && EVIDENCE_ID_PATTERN.test(id))) {
    throw fixedDisplayEffectError()
  }
  const piiClass = String(field.piiClass)
  const exactValue = String(field.exactValue)
  const valueSha256 = String(field.valueSha256)
  const evidenceReceiptIds = Object.freeze((field.evidenceReceiptIds as string[]).slice())
  acceptedFinalReceiptSetDigestV1(evidenceReceiptIds)
  return Object.freeze({
    piiClass,
    exactValue,
    valueSha256,
    evidenceReceiptIds,
    model: Object.freeze({
      label: naturalFieldLabelV2(piiClass, String(field.fieldName)),
      exactValue,
      evidenceReceiptCount: field.evidenceReceiptIds.length
    })
  })
}

function parseAccountFlowAnalysesV2(
  value: unknown,
  fields: readonly ParsedControlledArtifactFieldV2[]
): readonly ControlledArtifactDisplayAccountFlowV2[] {
  if (value === undefined) return Object.freeze([])
  if (!Array.isArray(value) || value.length === 0 || value.length > MAX_ACCOUNT_FLOW_ANALYSES) {
    throw fixedDisplayEffectError()
  }
  const accountValues = new Map<string, Readonly<{
    exactValue: string
    label: string
    evidenceReceiptIds: readonly string[]
  }>>()
  for (const field of fields) {
    if (field.piiClass !== 'financial_account_identifier') continue
    const previous = accountValues.get(field.valueSha256)
    if (previous && (previous.exactValue !== field.exactValue ||
      previous.label !== field.model.label ||
      !sameStringListV2(previous.evidenceReceiptIds, field.evidenceReceiptIds))) {
      throw fixedDisplayEffectError()
    }
    accountValues.set(field.valueSha256, Object.freeze({
      exactValue: field.exactValue,
      label: field.model.label,
      evidenceReceiptIds: field.evidenceReceiptIds
    }))
  }
  let previousSortKey: string | null = null
  const analyses = value.map((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      throw fixedDisplayEffectError()
    }
    const analysis = item as Record<string, unknown>
    if (!sameKeys(analysis, ACCOUNT_FLOW_KEYS) || analysis.schemaVersion !== 1 ||
      analysis.purpose !== ACCOUNT_FLOW_PURPOSE ||
      !SHA256_PATTERN.test(String(analysis.accountValueSha256)) ||
      !validTimestamp(analysis.startAt) || !validTimestamp(analysis.endAt) ||
      timestampNanosV2(String(analysis.startAt))! >
        timestampNanosV2(String(analysis.endAt))! ||
      !CURRENCY_PATTERN.test(String(analysis.currency)) || analysis.minorUnitScale !== 2 ||
      !UNSIGNED_DECIMAL_PATTERN.test(String(analysis.inflowMinor)) ||
      !UNSIGNED_DECIMAL_PATTERN.test(String(analysis.outflowMinor)) ||
      !SIGNED_DECIMAL_PATTERN.test(String(analysis.netMinor)) ||
      !UNSIGNED_DECIMAL_PATTERN.test(String(analysis.transactionCount)) ||
      !UNSIGNED_DECIMAL_PATTERN.test(String(analysis.evidenceTransactionCount)) ||
      !Array.isArray(analysis.evidenceReceiptIds) ||
      analysis.evidenceReceiptIds.length === 0 ||
      analysis.evidenceReceiptIds.length > 4096 ||
      !analysis.evidenceReceiptIds.every((id) =>
        typeof id === 'string' && EVIDENCE_ID_PATTERN.test(id)) ||
      !Array.isArray(analysis.transactions) ||
      analysis.transactions.length > MAX_ACCOUNT_FLOW_TRANSACTIONS) {
      throw fixedDisplayEffectError()
    }
    const accountHash = String(analysis.accountValueSha256)
    const analysisSortKey = `${accountHash}\0${String(analysis.startAt)}\0${String(analysis.endAt)}`
    if (previousSortKey !== null && previousSortKey >= analysisSortKey) {
      throw fixedDisplayEffectError()
    }
    previousSortKey = analysisSortKey
    const account = accountValues.get(accountHash)
    if (!account || !sameStringListV2(
      account.evidenceReceiptIds,
      analysis.evidenceReceiptIds as string[]
    )) throw fixedDisplayEffectError()
    const evidenceReceiptSetDigest = acceptedFinalReceiptSetDigestV1(
      analysis.evidenceReceiptIds as string[]
    )
    const exactAccount = account.exactValue
    const accountLabel = account.label
    const currency = String(analysis.currency)
    const scale = Number(analysis.minorUnitScale)
    let observedInflow = 0n
    let observedOutflow = 0n
    const startNanos = timestampNanosV2(String(analysis.startAt))!
    const endNanos = timestampNanosV2(String(analysis.endAt))!
    const transactions = analysis.transactions.map((item, index) => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) {
        throw fixedDisplayEffectError()
      }
      const transaction = item as Record<string, unknown>
      if (!sameKeys(transaction, ACCOUNT_FLOW_TRANSACTION_KEYS) ||
        !validTimestamp(transaction.occurredAt) ||
        (transaction.direction !== 'in' && transaction.direction !== 'out') ||
        !UNSIGNED_DECIMAL_PATTERN.test(String(transaction.amountMinor)) ||
        transaction.currency !== currency || transaction.minorUnitScale !== scale ||
        !UNSIGNED_DECIMAL_PATTERN.test(String(transaction.evidenceRowOrdinal)) ||
        String(transaction.evidenceRowOrdinal) !== String(index + 1) ||
        timestampNanosV2(String(transaction.occurredAt))! < startNanos ||
        timestampNanosV2(String(transaction.occurredAt))! > endNanos) {
        throw fixedDisplayEffectError()
      }
      const amountMinor = BigInt(String(transaction.amountMinor))
      if (transaction.direction === 'in') observedInflow += amountMinor
      else observedOutflow += amountMinor
      return Object.freeze({
        occurredAt: String(transaction.occurredAt),
        direction: transaction.direction === 'in' ? '流入' as const : '流出' as const,
        amount: formatMinorAmountV2(currency, String(transaction.amountMinor), scale)
      })
    })
    const inflowMinor = String(analysis.inflowMinor)
    const outflowMinor = String(analysis.outflowMinor)
    const netMinor = String(analysis.netMinor)
    const transactionCount = String(analysis.transactionCount)
    const evidenceTransactionCount = String(analysis.evidenceTransactionCount)
    if (observedInflow !== BigInt(inflowMinor) || observedOutflow !== BigInt(outflowMinor) ||
      observedInflow - observedOutflow !== BigInt(netMinor) ||
      transactions.length === 0 || BigInt(transactions.length) !== BigInt(transactionCount) ||
      BigInt(transactions.length) !== BigInt(evidenceTransactionCount)) {
      throw fixedDisplayEffectError()
    }
    const startAt = String(analysis.startAt)
    const endAt = String(analysis.endAt)
    const inflow = formatMinorAmountV2(currency, inflowMinor, scale)
    const outflow = formatMinorAmountV2(currency, outflowMinor, scale)
    const net = formatMinorAmountV2(currency, netMinor, scale)
    return Object.freeze({
      summary: `${accountLabel} ${exactAccount} 在 ${startAt} 至 ${endAt} 期间流入 ${inflow}、流出 ${outflow}、净额 ${net}，共 ${transactionCount} 笔交易，由 ${evidenceTransactionCount} 条证据交易支持。`,
      accountLabel,
      exactAccount,
      startAt,
      endAt,
      inflow,
      outflow,
      net,
      transactionCount,
      evidenceTransactionCount,
      evidenceReceiptCount: analysis.evidenceReceiptIds.length,
      evidenceReceiptSetDigest,
      transactions: Object.freeze(transactions)
    })
  })
  return Object.freeze(analyses)
}

function formatMinorAmountV2(currency: string, value: string, scale: number): string {
  const negative = value.startsWith('-')
  const digits = negative ? value.slice(1) : value
  const padded = digits.padStart(scale + 1, '0')
  const major = padded.slice(0, -scale)
  const minor = padded.slice(-scale)
  const grouped = major.replace(/\B(?=(\d{3})+(?!\d))/gu, ',')
  return `${negative ? '-' : ''}${currency} ${grouped}.${minor}`
}

function sameControlledContextV2(value: unknown, expected: Record<string, unknown>): boolean {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
    !expected || typeof expected !== 'object' || Array.isArray(expected)) return false
  const context = value as Record<string, unknown>
  return sameKeys(context, CONTEXT_KEYS) &&
    CONTEXT_KEYS.every((key) => context[key] === expected[key]) &&
    context.version === 2 && Number.isSafeInteger(context.contextEpoch) &&
    Number(context.contextEpoch) > 0 &&
    SHA256_PATTERN.test(String(context.caseBindingHash)) &&
    SHA256_PATTERN.test(String(context.sourceManifestHash)) &&
    SHA256_PATTERN.test(String(context.contextDigest)) &&
    DATASET_PATTERN.test(String(context.datasetSnapshotId)) &&
    validTimestamp(context.contextIssuedAt)
}

function naturalFieldLabelV2(piiClass: string, fieldName: string): string {
  if (piiClass === 'financial_account_identifier') {
    if (/card/iu.test(fieldName)) return '银行卡号'
    return '银行账号'
  }
  if (piiClass === 'person_name') return '姓名'
  if (piiClass === 'device_identifier') return '设备标识'
  if (piiClass === 'address') return '地址'
  return '受控标识'
}

function electronControlledArtifactDisplaySinkV2(): ControlledArtifactDisplaySinkV2 {
  return Object.freeze({
    open: async (input) => openElectronControlledArtifactDisplayV2(input)
  })
}

async function openElectronControlledArtifactDisplayV2({
  model: initialModel,
  modelDigest,
  authorizedUntil,
  isAuthorityCurrent,
  signal
}: Readonly<{
  model: ControlledArtifactDisplayModelV2
  modelDigest: string
  authorizedUntil: string
  isAuthorityCurrent: () => boolean
  signal: AbortSignal
}>): Promise<ControlledArtifactDisplaySessionV2> {
  if (!(signal instanceof AbortSignal) || signal.aborted ||
    !SHA256_PATTERN.test(modelDigest) || !validTimestamp(authorizedUntil) ||
    !authorityIsCurrentV2(isAuthorityCurrent)) {
    throw fixedDisplayEffectError()
  }
  let model: ControlledArtifactDisplayModelV2 | null = initialModel
  const partitionNonce = randomBytes(16).toString('hex')
  const window = new BrowserWindow({
    width: 860,
    height: 680,
    minWidth: 640,
    minHeight: 480,
    show: false,
    title: 'Analytix 受控案件研判结果',
    autoHideMenuBar: true,
    backgroundColor: '#f4f1ea',
    webPreferences: {
      nodeIntegration: false,
      nodeIntegrationInWorker: false,
      contextIsolation: true,
      sandbox: true,
      webviewTag: false,
      devTools: false,
      spellcheck: false,
      partition: `analytix-controlled-v2-${partitionNonce}`
    }
  })
  window.setContentProtection(true)
  window.removeMenu()
  window.webContents.setWindowOpenHandler(() => ({ action: 'deny' }))
  window.webContents.session.setPermissionCheckHandler(() => false)
  window.webContents.session.setPermissionRequestHandler((_contents, _permission, callback) => {
    callback(false)
  })
  window.webContents.on('will-attach-webview', (event) => event.preventDefault())
  window.webContents.on('context-menu', (event) => event.preventDefault())
  window.webContents.on('before-input-event', (event, input) => {
    if (input.control || input.meta || input.key === 'F12') event.preventDefault()
  })
  const displayURL = controlledDisplayDocumentURLV2()
  const reauthorizationURL = controlledReauthorizationDocumentURLV2()
  window.webContents.on('will-navigate', (event, url) => {
    if (url !== displayURL && url !== reauthorizationURL) event.preventDefault()
  })

  let port: Electron.MessagePortMain | null = null
  let closed = false
  let revoked = false
  let expiryTimer: ReturnType<typeof setTimeout> | null = null
  let authorityTimer: ReturnType<typeof setInterval> | null = null
  const clearAuthorityTimers = (): void => {
    if (expiryTimer) clearTimeout(expiryTimer)
    if (authorityTimer) clearInterval(authorityTimer)
    expiryTimer = null
    authorityTimer = null
  }
  const closePort = (): void => {
    port?.close()
    port = null
  }
  const destroy = (): void => {
    if (closed) return
    closed = true
    model = null
    clearAuthorityTimers()
    closePort()
    if (!window.isDestroyed()) window.destroy()
  }
  const revoke = (): void => {
    if (closed || revoked) return
    revoked = true
    model = null
    clearAuthorityTimers()
    closePort()
    if (window.isDestroyed()) {
      closed = true
      return
    }
    // Hide synchronously so no stale complete value remains visible while the
    // isolated document is replaced.  A failed replacement destroys the sink.
    window.hide()
    void window.loadURL(reauthorizationURL).then(() => {
      if (!closed && !window.isDestroyed()) window.show()
    }).catch(() => destroy())
  }
  signal.addEventListener('abort', destroy, { once: true })
  window.once('closed', () => {
    closed = true
    model = null
    signal.removeEventListener('abort', destroy)
    clearAuthorityTimers()
    closePort()
  })
  try {
    await window.loadURL(displayURL)
    if (closed || signal.aborted || !authorityIsCurrentV2(isAuthorityCurrent)) {
      throw fixedDisplayEffectError()
    }
    const channel = new MessageChannelMain()
    port = channel.port2
    const ack = new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(fixedDisplayEffectError()), 5_000)
      port!.once('message', (event) => {
        clearTimeout(timer)
        const value = event.data
        if (!value || typeof value !== 'object' || Array.isArray(value) ||
          Object.keys(value).sort().join('\n') !== ['modelDigest', 'nonce', 'purpose'].join('\n') ||
          value.purpose !== DISPLAY_ACK_PURPOSE || value.nonce !== partitionNonce ||
          value.modelDigest !== modelDigest) {
          reject(fixedDisplayEffectError())
          return
        }
        resolve()
      })
    })
    port.start()
    window.webContents.postMessage(
      'analytix-controlled-artifact-display-v2',
      { purpose: DISPLAY_PORT_PURPOSE },
      [channel.port1]
    )
    port.postMessage({
      purpose: DISPLAY_PAYLOAD_PURPOSE,
      nonce: partitionNonce,
      modelDigest,
      model
    })
    await ack
    model = null
    if (closed || signal.aborted || !authorityIsCurrentV2(isAuthorityCurrent)) {
      throw fixedDisplayEffectError()
    }
    const committedAt = currentTimestamp()
    const expiresAt = Date.parse(authorizedUntil)
    const remainingMs = expiresAt - Date.now()
    if (!Number.isFinite(expiresAt) || remainingMs <= 0) throw fixedDisplayEffectError()
    window.show()
    signal.removeEventListener('abort', destroy)
    expiryTimer = setTimeout(() => {
      expiryTimer = null
      revoke()
    }, remainingMs)
    authorityTimer = setInterval(() => {
      if (!authorityIsCurrentV2(isAuthorityCurrent)) {
        if (authorityTimer) clearInterval(authorityTimer)
        authorityTimer = null
        revoke()
      }
    }, 250)
    authorityTimer.unref?.()
    return Object.freeze({ committedAt, revoke, close: destroy })
  } catch {
    model = null
    signal.removeEventListener('abort', destroy)
    destroy()
    throw fixedDisplayEffectError()
  }
}

export function controlledDisplayDocumentURLV2(): string {
  const script = `
(() => {
  'use strict'
  const PORT_PURPOSE = ${JSON.stringify(DISPLAY_PORT_PURPOSE)}
  const PAYLOAD_PURPOSE = ${JSON.stringify(DISPLAY_PAYLOAD_PURPOSE)}
  const ACK_PURPOSE = ${JSON.stringify(DISPLAY_ACK_PURPOSE)}
  const MODEL_PURPOSE = ${JSON.stringify(DISPLAY_MODEL_PURPOSE)}
  const root = document.getElementById('controlled-root')
  const hex = (bytes) => Array.from(new Uint8Array(bytes)).map((value) => value.toString(16).padStart(2, '0')).join('')
  const exactKeys = (value, keys) => value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join('\\n') === keys.slice().sort().join('\\n')
  window.addEventListener('message', (event) => {
    if (!exactKeys(event.data, ['purpose']) || event.data.purpose !== PORT_PURPOSE ||
      !event.ports || event.ports.length !== 1) return
    const port = event.ports[0]
    port.onmessage = async (message) => {
      const value = message.data
      if (!exactKeys(value, ['model', 'modelDigest', 'nonce', 'purpose']) ||
        value.purpose !== PAYLOAD_PURPOSE || typeof value.nonce !== 'string' ||
        typeof value.modelDigest !== 'string' ||
        !exactKeys(value.model, ['accountFlows', 'fields', 'notice', 'purpose', 'schemaVersion', 'title']) ||
        value.model.schemaVersion !== 2 || value.model.purpose !== MODEL_PURPOSE ||
        !Array.isArray(value.model.accountFlows) || !Array.isArray(value.model.fields) ||
        value.model.fields.length === 0) return
      const digest = hex(await crypto.subtle.digest(
        'SHA-256',
        new TextEncoder().encode(JSON.stringify(value.model))
      ))
      if (digest !== value.modelDigest) return
      root.replaceChildren()
      const header = document.createElement('header')
      const title = document.createElement('h1')
      title.textContent = value.model.title
      const notice = document.createElement('p')
      notice.textContent = value.model.notice
      header.append(title, notice)
      const analyses = document.createElement('div')
      analyses.className = 'analyses'
      for (const flow of value.model.accountFlows) {
        if (!exactKeys(flow, [
          'accountLabel', 'endAt', 'evidenceReceiptCount', 'evidenceTransactionCount',
          'evidenceReceiptSetDigest', 'exactAccount', 'inflow', 'net', 'outflow', 'startAt', 'summary',
          'transactionCount', 'transactions'
        ]) || typeof flow.summary !== 'string' || typeof flow.accountLabel !== 'string' ||
          typeof flow.exactAccount !== 'string' || typeof flow.startAt !== 'string' ||
          typeof flow.endAt !== 'string' || typeof flow.inflow !== 'string' ||
          typeof flow.outflow !== 'string' || typeof flow.net !== 'string' ||
          typeof flow.transactionCount !== 'string' ||
          typeof flow.evidenceTransactionCount !== 'string' ||
          !Number.isSafeInteger(flow.evidenceReceiptCount) ||
          flow.evidenceReceiptCount <= 0 ||
          typeof flow.evidenceReceiptSetDigest !== 'string' ||
          !/^[a-f0-9]{64}$/.test(flow.evidenceReceiptSetDigest) ||
          !Array.isArray(flow.transactions)) return
        const card = document.createElement('section')
        card.className = 'analysis'
        card.setAttribute('data-evidence-receipt-set-digest', flow.evidenceReceiptSetDigest)
        const heading = document.createElement('h2')
        heading.textContent = flow.accountLabel + ' ' + flow.exactAccount
        const summary = document.createElement('p')
        summary.className = 'summary'
        summary.textContent = flow.summary
        const audit = document.createElement('p')
        audit.className = 'audit'
        audit.textContent = '已绑定 ' + flow.evidenceReceiptCount + ' 份证据回执'
        card.append(heading, summary, audit)
        if (flow.transactions.length > 0) {
          const details = document.createElement('details')
          const detailsTitle = document.createElement('summary')
          detailsTitle.textContent = '查看对应证据交易（' + flow.transactions.length + '）'
          const table = document.createElement('table')
          const tableHead = document.createElement('thead')
          const headerRow = document.createElement('tr')
          for (const labelText of ['时间', '方向', '金额']) {
            const cell = document.createElement('th')
            cell.textContent = labelText
            headerRow.append(cell)
          }
          tableHead.append(headerRow)
          const tableBody = document.createElement('tbody')
          for (const transaction of flow.transactions) {
            if (!exactKeys(transaction, ['amount', 'direction', 'occurredAt']) ||
              typeof transaction.occurredAt !== 'string' ||
              (transaction.direction !== '流入' && transaction.direction !== '流出') ||
              typeof transaction.amount !== 'string') return
            const row = document.createElement('tr')
            for (const cellText of [transaction.occurredAt, transaction.direction, transaction.amount]) {
              const cell = document.createElement('td')
              cell.textContent = cellText
              row.append(cell)
            }
            tableBody.append(row)
          }
          table.append(tableHead, tableBody)
          details.append(detailsTitle, table)
          card.append(details)
        }
        analyses.append(card)
      }
      const list = document.createElement('dl')
      for (const field of value.model.fields) {
        if (!exactKeys(field, ['evidenceReceiptCount', 'exactValue', 'label']) ||
          typeof field.label !== 'string' || typeof field.exactValue !== 'string' ||
          !Number.isSafeInteger(field.evidenceReceiptCount) || field.evidenceReceiptCount <= 0) return
        const row = document.createElement('div')
        row.className = 'field'
        const label = document.createElement('dt')
        label.textContent = field.label
        const exact = document.createElement('dd')
        exact.textContent = field.exactValue
        const evidence = document.createElement('span')
        evidence.textContent = '已绑定 ' + field.evidenceReceiptCount + ' 份证据回执'
        exact.append(document.createElement('br'), evidence)
        row.append(label, exact)
        list.append(row)
      }
      root.append(header, analyses, list)
      port.postMessage({ purpose: ACK_PURPOSE, nonce: value.nonce, modelDigest: digest })
    }
    port.start()
  }, { once: true })
})()
`
  const html = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; style-src 'nonce-analytix-controlled-v2'; script-src 'nonce-analytix-controlled-v2'">
  <meta name="referrer" content="no-referrer">
  <meta name="analytix-controlled-artifact" content="${TRUSTED_SINK_MARKER_V2}">
  <meta name="color-scheme" content="light">
  <title>Analytix 受控案件研判结果</title>
  <style nonce="analytix-controlled-v2">
    :root { font-family: ui-serif, 'Noto Serif SC', serif; color: #25231f; background: #f4f1ea; }
    * { box-sizing: border-box; user-select: none; -webkit-user-select: none; }
    body { margin: 0; min-height: 100vh; padding: 40px; }
    main { width: min(760px, 100%); margin: 0 auto; }
    header, .reauthorize { border: 1px solid #c8c1b5; border-radius: 18px; background: #fffdf8; padding: 24px; box-shadow: 0 12px 34px rgba(44, 39, 31, .08); }
    h1 { margin: 0 0 8px; font-size: 24px; }
    h2 { margin: 0 0 10px; font-size: 18px; }
    p { margin: 0; color: #696259; line-height: 1.65; }
    .analyses { margin: 20px 0 0; display: grid; gap: 12px; }
    .analysis { border: 1px solid #c8c1b5; border-radius: 16px; background: #fffdf8; padding: 20px; }
    .summary { color: #25231f; font-size: 15px; }
    .audit { margin-top: 8px; font-size: 12px; }
    details { margin-top: 16px; }
    summary { cursor: pointer; color: #514b43; font-size: 13px; }
    table { width: 100%; margin-top: 10px; border-collapse: collapse; font-size: 12px; }
    th, td { border-bottom: 1px solid #e1dbd0; padding: 8px; text-align: left; }
    dl { margin: 20px 0 0; display: grid; gap: 12px; }
    .field { display: grid; grid-template-columns: minmax(120px, 180px) 1fr; gap: 18px; border: 1px solid #d8d1c5; border-radius: 14px; background: #fff; padding: 18px 20px; }
    dt { color: #696259; font-size: 14px; }
    dd { margin: 0; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 18px; overflow-wrap: anywhere; }
    dd span { color: #7b7469; font-family: ui-sans-serif, system-ui, sans-serif; font-size: 12px; }
  </style>
</head>
<body><main id="controlled-root" data-analytix-controlled-artifact="${TRUSTED_SINK_MARKER_V2}" aria-live="polite"></main><script nonce="analytix-controlled-v2">${script}</script></body>
</html>`
  return `data:text/html;charset=utf-8,${encodeURIComponent(html)}`
}

export function controlledReauthorizationDocumentURLV2(): string {
  const html = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'; style-src 'nonce-analytix-controlled-v2'">
  <meta name="referrer" content="no-referrer">
  <meta name="analytix-controlled-artifact" content="${TRUSTED_SINK_MARKER_V2}">
  <meta name="color-scheme" content="light">
  <title>Analytix 受控案件研判结果</title>
  <style nonce="analytix-controlled-v2">
    :root { font-family: ui-serif, 'Noto Serif SC', serif; color: #25231f; background: #f4f1ea; }
    * { box-sizing: border-box; user-select: none; -webkit-user-select: none; }
    body { margin: 0; min-height: 100vh; padding: 40px; display: grid; place-items: center; }
    main { width: min(680px, 100%); border: 1px solid #c8c1b5; border-radius: 18px; background: #fffdf8; padding: 28px; box-shadow: 0 12px 34px rgba(44, 39, 31, .08); }
    h1 { margin: 0 0 10px; font-size: 24px; }
    p { margin: 0; color: #696259; line-height: 1.65; }
  </style>
</head>
<body><main id="controlled-root" data-analytix-controlled-artifact="${TRUSTED_SINK_MARKER_V2}" data-controlled-artifact-state="${REAUTHORIZATION_STATE_MARKER_V2}" aria-live="polite"><h1>需要重新授权</h1><p>完整标识已隐藏，请重新授权后查看。普通 Agent 和获准的脱敏案件分析可继续使用。</p></main></body>
</html>`
  return `data:text/html;charset=utf-8,${encodeURIComponent(html)}`
}

function sameKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  return Object.keys(value).sort().join('\n') === [...expected].sort().join('\n')
}

function validInvocationContextV2(
  value: unknown
): value is ControlledArtifactInvocationContextV2 {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const context = value as Record<string, unknown>
  return sameKeys(context, ['contextDigest', 'contextEpoch', 'threadId', 'turnId']) &&
    canonicalText(context.threadId, 256) && canonicalText(context.turnId, 256) &&
    Number.isSafeInteger(context.contextEpoch) && Number(context.contextEpoch) > 0 &&
    SHA256_PATTERN.test(String(context.contextDigest))
}

function sameInvocationContextV2(
  value: unknown,
  expected: ControlledArtifactInvocationContextV2
): boolean {
  if (!validInvocationContextV2(expected) || !value || typeof value !== 'object' ||
    Array.isArray(value)) return false
  const context = value as Record<string, unknown>
  return context.threadId === expected.threadId && context.turnId === expected.turnId &&
    context.contextEpoch === expected.contextEpoch &&
    context.contextDigest === expected.contextDigest
}

function canonicalText(value: unknown, maximumBytes: number): value is string {
  if (typeof value !== 'string' || value === '' || value !== value.trim() ||
    Buffer.byteLength(value, 'utf8') > maximumBytes) return false
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0
    if (codePoint === 0 || codePoint <= 0x1f || codePoint === 0x7f) return false
  }
  return true
}

function validTimestamp(value: unknown): value is string {
  return typeof value === 'string' && timestampNanosV2(value) !== null
}

function timestampNanosV2(value: string): bigint | null {
  if (Buffer.byteLength(value, 'utf8') > 64) return null
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):([0-5]\d):([0-5]\d)(?:\.(\d{1,9}))?Z$/u.exec(value)
  if (!match || (match[7]?.endsWith('0') ?? false)) return null
  const base = `${match[1]}-${match[2]}-${match[3]}T${match[4]}:${match[5]}:${match[6]}Z`
  const milliseconds = Date.parse(base)
  if (!Number.isFinite(milliseconds) ||
    new Date(milliseconds).toISOString().replace('.000Z', 'Z') !== base) return null
  const fraction = (match[7] ?? '').padEnd(9, '0')
  return BigInt(milliseconds) * 1_000_000n + BigInt(fraction || '0')
}

function sameStringListV2(left: readonly string[], right: readonly string[]): boolean {
  return left.length === right.length && left.every((value, index) => value === right[index])
}

function acceptedFinalReceiptSetDigestV1(receiptIDs: readonly string[]): string {
  const canonical = [...receiptIDs].sort()
  if (canonical.length === 0 || canonical.length !== receiptIDs.length ||
    canonical.some((value, index) => !EVIDENCE_ID_PATTERN.test(value) ||
      (index > 0 && value <= canonical[index - 1])) ||
    !sameStringListV2(receiptIDs, canonical)) {
    throw fixedDisplayEffectError()
  }
  // Keep byte-for-byte parity with AcceptedFinal receiptMetadata.setDigest:
  // SHA-256 over the JSON encoding of the lexicographically sorted receipt IDs.
  return sha256(Buffer.from(JSON.stringify(canonical), 'utf8'))
}

function requireBeforeDeadline(
  now: () => string,
  authorizedUntil: string,
  signal: AbortSignal
): string {
  if (!(signal instanceof AbortSignal) || signal.aborted || !validTimestamp(authorizedUntil)) {
    throw fixedDisplayEffectError()
  }
  const current = now()
  if (!validTimestamp(current) || Date.parse(current) >= Date.parse(authorizedUntil)) {
    throw fixedDisplayEffectError()
  }
  return current
}

export function formatControlledArtifactTimestampV2(value: Date): string {
  return value.toISOString()
    .replace(/\.000Z$/u, 'Z')
    .replace(/(\.\d*?[1-9])0+Z$/u, '$1Z')
}

function currentTimestamp(): string {
  return formatControlledArtifactTimestampV2(new Date())
}

function authorityIsCurrentV2(isCurrent: () => boolean): boolean {
  try {
    return isCurrent() === true
  } catch {
    return false
  }
}

function sha256(value: Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function fixedDisplayEffectError(): Error {
  return new Error(DISPLAY_EFFECT_ERROR)
}
