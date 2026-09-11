import {
  createHash,
  createHmac,
  createPublicKey,
  randomBytes,
  timingSafeEqual,
  verify
} from 'node:crypto'
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http'
import type { AddressInfo } from 'node:net'
import { parseStrictJsonObject } from './strict-json'

export const CONTROLLED_ARTIFACT_HOST_URL_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL'
export const CONTROLLED_ARTIFACT_HOST_TOKEN_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN'

const HOST = '127.0.0.1'
const ADMISSION_PATH = '/v1/controlled-artifacts/admission'
const RELEASE_PATH = '/v1/controlled-artifacts/release'
const ADMISSION_PURPOSE = 'analytix.controlled-artifact-admission/v1'
const RELEASE_PURPOSE = 'analytix.controlled-artifact-release/v1'
const RELEASE_ACK_PURPOSE = 'analytix.controlled-artifact-release-ack/v1'
const JSON_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-host+json'
const RELEASE_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-release-v1'
const CONTROLLED_ARTIFACT_MEDIA_TYPE = 'application/vnd.analytix.controlled-case-evidence+json'
const PROTOCOL_VERSION = 1
const SECRET_BYTES = 32
const MAX_JSON_BYTES = 64 << 10
const MAX_METADATA_BYTES = (256 << 10) + 4096
const MAX_ARTIFACT_BYTES = 8 << 20
const MAX_RELEASE_BYTES = 8 + 4 + MAX_METADATA_BYTES + MAX_ARTIFACT_BYTES
const MAX_ADMISSION_TTL_MS = 15 * 60 * 1000
const RELEASE_MAGIC = Buffer.from('ANXCRV1\n', 'ascii')
const RELEASE_ACK_DOMAIN = Buffer.from('analytix.controlled-artifact-release-ack/hmac/v1\0', 'utf8')
const OPAQUE_PATTERN = /^[A-Za-z0-9._~-]{16,1024}$/
const SHA256_PATTERN = /^[a-f0-9]{64}$/
const DATASET_PATTERN = /^dsv1_[a-f0-9]{64}$/

const HANDLE_DIGEST_DOMAIN = Buffer.from('analytix.controlled-artifact-access/handle/v1\0', 'utf8')
const SLOT_DIGEST_DOMAIN = Buffer.from('analytix.controlled-artifact-access/use-slot/v1\0', 'utf8')
const PRINCIPAL_DIGEST_DOMAIN = Buffer.from('analytix.controlled-artifact-access/renderer-principal/v1\0', 'utf8')
const ACCESS_ID_DOMAIN = Buffer.from('analytix.controlled-artifact-access/id/v1\0', 'utf8')
const RECEIPT_SIGNATURE_DOMAIN = Buffer.from(
  'analytix.controlled-artifact-access-receipt/signature/v1\0',
  'utf8'
)
const RECEIPT_RECORD_DOMAIN = Buffer.from(
  'analytix.controlled-artifact-access-receipt/record/v1\0',
  'utf8'
)
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')

const ADMISSION_REQUEST_KEYS = [
  'accessAction', 'backendGeneration', 'caseBindingHash', 'caseId', 'contextDigest', 'contextEpoch',
  'controlledHandle', 'datasetSnapshotId', 'purpose', 'rendererGeneration', 'rendererPrincipal',
  'schemaVersion', 'sourceManifestHash', 'threadId', 'turnId', 'useSlot'
] as const
const RELEASE_METADATA_KEYS = ['purpose', 'receipt', 'schemaVersion'] as const
const RECEIPT_KEYS = [
  'accessAction', 'accessId', 'accessPolicyDigest', 'artifactByteLength', 'artifactSha256',
  'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature', 'authorizedUntil',
  'backendGeneration', 'claimLedgerDigest', 'context', 'controlledHandleDigest', 'mediaType',
  'piiAuthorizationDigest', 'piiProjectionDigest', 'publicationCommitDigest', 'publicationReceiptDigest',
  'purpose', 'recordDigest', 'rendererGeneration', 'rendererPrincipalDigest', 'requestedAt',
  'requesterUserId', 'retentionPolicyDigest', 'schemaVersion', 'targetIdentityDigest', 'useSlotDigest'
] as const
const CONTEXT_KEYS = [
  'caseBindingHash', 'caseId', 'contextDigest', 'contextEpoch', 'contextIssuedAt', 'datasetSnapshotId',
  'sourceManifestHash', 'tenantId', 'threadId', 'turnId', 'userId', 'version', 'workspaceRealPath'
] as const

export type ControlledArtifactAccessActionV1 = 'display' | 'export'

export type ControlledArtifactContextBindingV1 = {
  threadId: string
  turnId: string
  caseId: string
  caseBindingHash: string
  datasetSnapshotId: string
  sourceManifestHash: string
  contextEpoch: number
  contextDigest: string
}

export class ControlledArtifactRuntimeReleaseInvocationV1 {
  readonly #threadId: string
  readonly #turnId: string
  readonly #accessAction: ControlledArtifactAccessActionV1
  readonly #controlledHandle: string
  readonly #useSlot: string
  readonly #rendererPrincipal: string
  readonly #rendererGeneration: number
  readonly #backendGeneration: number

  private constructor(input: {
    threadId: string
    turnId: string
    accessAction: ControlledArtifactAccessActionV1
    controlledHandle: string
    useSlot: string
    rendererPrincipal: string
    rendererGeneration: number
    backendGeneration: number
  }) {
    this.#threadId = input.threadId
    this.#turnId = input.turnId
    this.#accessAction = input.accessAction
    this.#controlledHandle = input.controlledHandle
    this.#useSlot = input.useSlot
    this.#rendererPrincipal = input.rendererPrincipal
    this.#rendererGeneration = input.rendererGeneration
    this.#backendGeneration = input.backendGeneration
    Object.freeze(this)
  }

  static fromHost(input: {
    threadId: string
    turnId: string
    accessAction: ControlledArtifactAccessActionV1
    controlledHandle: string
    useSlot: string
    rendererPrincipal: string
    rendererGeneration: number
    backendGeneration: number
  }): ControlledArtifactRuntimeReleaseInvocationV1 {
    return new ControlledArtifactRuntimeReleaseInvocationV1(input)
  }

  get threadId(): string { return this.#threadId }
  get turnId(): string { return this.#turnId }
  get accessAction(): ControlledArtifactAccessActionV1 { return this.#accessAction }
  get controlledHandle(): string { return this.#controlledHandle }
  get useSlot(): string { return this.#useSlot }
  get rendererPrincipal(): string { return this.#rendererPrincipal }
  get rendererGeneration(): number { return this.#rendererGeneration }
  get backendGeneration(): number { return this.#backendGeneration }

  toJSON(): never {
    throw new Error('controlled artifact invocation is main-process-only')
  }
}

export type ControlledArtifactReleaseReceiptV1 = {
  schemaVersion: number
  purpose: string
  accessId: string
  context: Record<string, unknown>
  requesterUserId: string
  accessAction: ControlledArtifactAccessActionV1
  controlledHandleDigest: string
  useSlotDigest: string
  rendererPrincipalDigest: string
  rendererGeneration: number
  backendGeneration: number
  accessPolicyDigest: string
  retentionPolicyDigest: string
  publicationCommitDigest: string
  publicationReceiptDigest: string
  piiProjectionDigest: string
  piiAuthorizationDigest: string
  claimLedgerDigest: string
  targetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
  mediaType: string
  requestedAt: string
  authorizedUntil: string
  authorityAlgorithm: string
  authorityKeyId: string
  authorityPublicKey: string
  authoritySignature: string
  recordDigest: string
}

export type ControlledArtifactReleaseEffectInputV1 = {
  action: ControlledArtifactAccessActionV1
  body: Buffer
  receipt: ControlledArtifactReleaseReceiptV1
}

export type ControlledArtifactReleaseEffectV1 = (
  input: ControlledArtifactReleaseEffectInputV1
) => Promise<void>

export type ControlledArtifactHostOptionsV1 = {
  validateRenderer: (webContentsId: number, rendererGeneration: number) => boolean
  now?: () => Date
  randomToken?: () => string
}

export type ControlledArtifactInvocationInputV1 = {
  context: ControlledArtifactContextBindingV1
  publicationCommitDigest: string
  authorityKeyId: string
  authorityPublicKey: string
  action: ControlledArtifactAccessActionV1
  webContentsId: number
  rendererGeneration: number
  authorizedUntil: Date
  release: ControlledArtifactReleaseEffectV1
}

type EntryState = 'ready' | 'releasing' | 'committed' | 'indeterminate' | 'revoked'

type ControlledArtifactHostEntryV1 = ControlledArtifactInvocationInputV1 & {
  controlledHandle: string
  controlledHandleDigest: string
  useSlot: string
  useSlotDigest: string
  rendererPrincipal: string
  rendererPrincipalDigest: string
  backendGeneration: number
  authorizedUntilText: string
  state: EntryState
}

export type ControlledArtifactHostRuntimeEnvironmentV1 = {
  [CONTROLLED_ARTIFACT_HOST_URL_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_TOKEN_ENV]: string
}

export class ControlledArtifactHostV1 {
  private server: Server | null = null
  private origin = ''
  private secret = ''
  private secretBytes = Buffer.alloc(0)
  private backendGeneration = 0
  private readonly entries = new Map<string, ControlledArtifactHostEntryV1>()
  private readonly entriesByDigest = new Map<string, ControlledArtifactHostEntryV1>()
  private readonly now: () => Date
  private readonly randomToken: () => string

  constructor(private readonly options: ControlledArtifactHostOptionsV1) {
    if (!options || typeof options.validateRenderer !== 'function') {
      throw new Error('controlled artifact renderer authority is required')
    }
    this.now = options.now ?? (() => new Date())
    this.randomToken = options.randomToken ?? (() => randomBytes(SECRET_BYTES).toString('base64url'))
  }

  async start(): Promise<void> {
    if (this.server) return
    const secret = this.randomToken()
    if (!isCanonicalSecret(secret)) throw new Error('controlled artifact host secret generation failed')
    const server = createServer((request, response) => {
      void this.handle(request, response).catch(() => writeFailure(response, httpStatus.internalError))
    })
    server.maxHeadersCount = 16
    server.headersTimeout = 5_000
    server.requestTimeout = 35_000
    server.keepAliveTimeout = 1
    await new Promise<void>((resolve, reject) => {
      const onError = (error: Error): void => reject(error)
      server.once('error', onError)
      server.listen(0, HOST, () => {
        server.off('error', onError)
        resolve()
      })
    })
    const address = server.address() as AddressInfo | null
    if (!address || address.address !== HOST || address.family !== 'IPv4' || !Number.isSafeInteger(address.port)) {
      await closeServer(server)
      throw new Error('controlled artifact host did not bind the exact loopback authority')
    }
    this.backendGeneration += 1
    if (!Number.isSafeInteger(this.backendGeneration) || this.backendGeneration <= 0) {
      await closeServer(server)
      throw new Error('controlled artifact host generation exhausted')
    }
    this.secret = secret
    this.secretBytes = Buffer.from(secret, 'base64url')
    this.origin = `http://${HOST}:${address.port}`
    this.server = server
  }

  async stop(): Promise<void> {
    const server = this.server
    this.server = null
    this.origin = ''
    this.secret = ''
    this.secretBytes.fill(0)
    this.secretBytes = Buffer.alloc(0)
    for (const entry of this.entries.values()) entry.state = 'revoked'
    this.entries.clear()
    this.entriesByDigest.clear()
    if (server) await closeServer(server)
  }

  runtimeEnvironment(): ControlledArtifactHostRuntimeEnvironmentV1 {
    if (!this.server || !this.origin || !isCanonicalSecret(this.secret)) {
      throw new Error('controlled artifact host is not running')
    }
    return {
      [CONTROLLED_ARTIFACT_HOST_URL_ENV]: this.origin,
      [CONTROLLED_ARTIFACT_HOST_TOKEN_ENV]: this.secret
    }
  }

  createInvocation(input: ControlledArtifactInvocationInputV1): ControlledArtifactRuntimeReleaseInvocationV1 {
    if (!this.server || !this.origin || !isValidInvocationInput(input, this.now())) {
      throw new Error('controlled artifact invocation is unavailable')
    }
    this.pruneExpired()
    const controlledHandle = this.uniqueToken(this.entries)
    const useSlot = this.randomToken()
    const rendererPrincipal = this.randomToken()
    if (!isCanonicalSecret(useSlot) || !isCanonicalSecret(rendererPrincipal)) {
      throw new Error('controlled artifact invocation token generation failed')
    }
    const entry: ControlledArtifactHostEntryV1 = {
      ...input,
      controlledHandle,
      controlledHandleDigest: opaqueDigest(HANDLE_DIGEST_DOMAIN, controlledHandle),
      useSlot,
      useSlotDigest: opaqueDigest(SLOT_DIGEST_DOMAIN, useSlot),
      rendererPrincipal,
      rendererPrincipalDigest: opaqueDigest(PRINCIPAL_DIGEST_DOMAIN, rendererPrincipal),
      backendGeneration: this.backendGeneration,
      authorizedUntilText: formatRFC3339Nano(input.authorizedUntil),
      state: 'ready'
    }
    this.entries.set(controlledHandle, entry)
    this.entriesByDigest.set(entry.controlledHandleDigest, entry)
    return ControlledArtifactRuntimeReleaseInvocationV1.fromHost({
      threadId: input.context.threadId,
      turnId: input.context.turnId,
      accessAction: input.action,
      controlledHandle,
      useSlot,
      rendererPrincipal,
      rendererGeneration: input.rendererGeneration,
      backendGeneration: this.backendGeneration
    })
  }

  revokeRenderer(webContentsId: number, rendererGeneration?: number): void {
    for (const entry of this.entries.values()) {
      if (entry.webContentsId === webContentsId &&
        (rendererGeneration === undefined || entry.rendererGeneration === rendererGeneration)) {
        entry.state = 'revoked'
      }
    }
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    response.setHeader('Cache-Control', 'no-store')
    response.setHeader('X-Content-Type-Options', 'nosniff')
    response.setHeader('Connection', 'close')
    if (!this.authorizeRequest(request)) {
      writeFailure(response, httpStatus.unauthorized)
      return
    }
    if (request.method !== 'POST') {
      writeFailure(response, httpStatus.methodNotAllowed)
      return
    }
    if (request.url === ADMISSION_PATH) {
      await this.handleAdmission(request, response)
      return
    }
    if (request.url === RELEASE_PATH) {
      await this.handleRelease(request, response)
      return
    }
    writeFailure(response, httpStatus.notFound)
  }

  private authorizeRequest(request: IncomingMessage): boolean {
    if (!this.server || request.socket.remoteAddress !== HOST || request.headers.host !== this.origin.slice('http://'.length)) {
      return false
    }
    const authorization = singleHeader(request, 'authorization')
    const accept = singleHeader(request, 'accept')
    const expected = `Bearer ${this.secret}`
    return accept === JSON_MEDIA_TYPE && safeEqual(authorization, expected)
  }

  private async handleAdmission(request: IncomingMessage, response: ServerResponse): Promise<void> {
    if (singleHeader(request, 'content-type') !== JSON_MEDIA_TYPE || hasTransferEncoding(request)) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    const body = await readRequestBody(request, MAX_JSON_BYTES)
    if (!body) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    let value: Record<string, unknown>
    try {
      value = parseStrictJsonObject(body, { maxBytes: MAX_JSON_BYTES, maxDepth: 3, maxTokens: 64, maxStringBytes: 2048 })
    } catch {
      writeFailure(response, httpStatus.badRequest)
      return
    } finally {
      body.fill(0)
    }
    if (!hasExactKeys(value, ADMISSION_REQUEST_KEYS) ||
      value.schemaVersion !== PROTOCOL_VERSION || value.purpose !== ADMISSION_PURPOSE) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    const handle = exactOpaque(value.controlledHandle)
    const entry = handle ? this.entries.get(handle) : undefined
    if (!entry || !this.admissionMatches(entry, value)) {
      writeFailure(response, httpStatus.conflict)
      return
    }
    writeJson(response, {
      schemaVersion: PROTOCOL_VERSION,
      purpose: ADMISSION_PURPOSE,
      controlledHandleDigest: entry.controlledHandleDigest,
      useSlotDigest: entry.useSlotDigest,
      rendererPrincipalDigest: entry.rendererPrincipalDigest,
      rendererGeneration: entry.rendererGeneration,
      backendGeneration: entry.backendGeneration,
      publicationCommitDigest: entry.publicationCommitDigest,
      authorizedUntil: entry.authorizedUntilText
    })
  }

  private admissionMatches(entry: ControlledArtifactHostEntryV1, value: Record<string, unknown>): boolean {
    const now = this.now()
    return entry.state !== 'revoked' && entry.state !== 'indeterminate' && now.getTime() < entry.authorizedUntil.getTime() &&
      this.backendGeneration === entry.backendGeneration &&
      this.options.validateRenderer(entry.webContentsId, entry.rendererGeneration) &&
      value.threadId === entry.context.threadId && value.turnId === entry.context.turnId &&
      value.caseId === entry.context.caseId && value.caseBindingHash === entry.context.caseBindingHash &&
      value.datasetSnapshotId === entry.context.datasetSnapshotId && value.sourceManifestHash === entry.context.sourceManifestHash &&
      value.contextEpoch === entry.context.contextEpoch && value.contextDigest === entry.context.contextDigest &&
      value.accessAction === entry.action && safeEqual(value.controlledHandle, entry.controlledHandle) &&
      safeEqual(value.useSlot, entry.useSlot) && safeEqual(value.rendererPrincipal, entry.rendererPrincipal) &&
      value.rendererGeneration === entry.rendererGeneration && value.backendGeneration === entry.backendGeneration
  }

  private async handleRelease(request: IncomingMessage, response: ServerResponse): Promise<void> {
    if (singleHeader(request, 'content-type') !== RELEASE_MEDIA_TYPE || hasTransferEncoding(request)) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    const frame = await readRequestBody(request, MAX_RELEASE_BYTES)
    if (!frame) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    let entry: ControlledArtifactHostEntryV1 | undefined
    try {
      let parsed: ReturnType<typeof parseReleaseFrame>
      let receipt: ControlledArtifactReleaseReceiptV1
      try {
        parsed = parseReleaseFrame(frame)
        receipt = parseReleaseReceipt(parsed.metadata)
      } catch {
        writeFailure(response, httpStatus.badRequest)
        return
      }
      entry = this.entriesByDigest.get(receipt.controlledHandleDigest)
      if (!entry || !this.releaseMatches(entry, receipt, parsed.body)) {
        writeFailure(response, httpStatus.conflict)
        return
      }
      entry.state = 'releasing'
      try {
        await entry.release({ action: entry.action, body: parsed.body, receipt })
      } catch {
        entry.state = 'indeterminate'
        writeFailure(response, httpStatus.internalError)
        return
      }
      const committedAt = this.now()
      if (entry.state !== 'releasing' || committedAt.getTime() >= entry.authorizedUntil.getTime() ||
        this.backendGeneration !== entry.backendGeneration ||
        !this.options.validateRenderer(entry.webContentsId, entry.rendererGeneration)) {
        entry.state = 'indeterminate'
        writeFailure(response, httpStatus.conflict)
        return
      }
      entry.state = 'committed'
      const ack = {
        schemaVersion: PROTOCOL_VERSION,
        purpose: RELEASE_ACK_PURPOSE,
        committed: true,
        accessId: receipt.accessId,
        accessReceiptDigest: receipt.recordDigest,
        artifactSha256: receipt.artifactSha256,
        releasedByteLength: receipt.artifactByteLength,
        accessAction: receipt.accessAction,
        controlledHandleDigest: receipt.controlledHandleDigest,
        useSlotDigest: receipt.useSlotDigest,
        rendererPrincipalDigest: receipt.rendererPrincipalDigest,
        rendererGeneration: receipt.rendererGeneration,
        backendGeneration: receipt.backendGeneration,
        committedAt: formatRFC3339Nano(committedAt),
        mac: ''
      }
      ack.mac = createHmac('sha256', this.secretBytes).update(releaseAckMACMessage(ack)).digest('base64url')
      writeJson(response, ack)
    } finally {
      frame.fill(0)
    }
  }

  private releaseMatches(
    entry: ControlledArtifactHostEntryV1,
    receipt: ControlledArtifactReleaseReceiptV1,
    body: Buffer
  ): boolean {
    const context = receipt.context
    return entry.state === 'ready' && this.backendGeneration === entry.backendGeneration &&
      this.now().getTime() < entry.authorizedUntil.getTime() &&
      this.options.validateRenderer(entry.webContentsId, entry.rendererGeneration) &&
      receipt.accessAction === entry.action && receipt.controlledHandleDigest === entry.controlledHandleDigest &&
      receipt.useSlotDigest === entry.useSlotDigest && receipt.rendererPrincipalDigest === entry.rendererPrincipalDigest &&
      receipt.rendererGeneration === entry.rendererGeneration && receipt.backendGeneration === entry.backendGeneration &&
      receipt.publicationCommitDigest === entry.publicationCommitDigest && receipt.authorizedUntil === entry.authorizedUntilText &&
      receipt.authorityKeyId === entry.authorityKeyId && receipt.authorityPublicKey === entry.authorityPublicKey &&
      context.threadId === entry.context.threadId && context.turnId === entry.context.turnId &&
      context.caseId === entry.context.caseId && context.caseBindingHash === entry.context.caseBindingHash &&
      context.datasetSnapshotId === entry.context.datasetSnapshotId && context.sourceManifestHash === entry.context.sourceManifestHash &&
      context.contextEpoch === entry.context.contextEpoch && context.contextDigest === entry.context.contextDigest &&
      receipt.artifactByteLength === body.length && receipt.artifactSha256 === sha256(body)
  }

  private pruneExpired(): void {
    const now = this.now().getTime()
    for (const [handle, entry] of this.entries) {
      if (entry.authorizedUntil.getTime() > now) continue
      entry.state = 'revoked'
      this.entries.delete(handle)
      this.entriesByDigest.delete(entry.controlledHandleDigest)
    }
  }

  private uniqueToken(existing: Map<string, unknown>): string {
    for (let attempt = 0; attempt < 4; attempt += 1) {
      const token = this.randomToken()
      if (isCanonicalSecret(token) && !existing.has(token)) return token
    }
    throw new Error('controlled artifact invocation token collision')
  }
}

function parseReleaseFrame(frame: Buffer): { metadata: Record<string, unknown>; body: Buffer } {
  if (frame.length < RELEASE_MAGIC.length + 4 || !frame.subarray(0, RELEASE_MAGIC.length).equals(RELEASE_MAGIC)) {
    throw new Error('controlled artifact release frame is invalid')
  }
  const metadataLength = frame.readUInt32BE(RELEASE_MAGIC.length)
  const metadataStart = RELEASE_MAGIC.length + 4
  const bodyStart = metadataStart + metadataLength
  if (metadataLength < 1 || metadataLength > MAX_METADATA_BYTES || bodyStart >= frame.length ||
    frame.length - bodyStart > MAX_ARTIFACT_BYTES) {
    throw new Error('controlled artifact release frame length is invalid')
  }
  const metadata = parseStrictJsonObject(frame.subarray(metadataStart, bodyStart), {
    maxBytes: MAX_METADATA_BYTES, maxDepth: 6, maxTokens: 512, maxStringBytes: 32 << 10
  })
  return { metadata, body: frame.subarray(bodyStart) }
}

function parseReleaseReceipt(metadata: Record<string, unknown>): ControlledArtifactReleaseReceiptV1 {
  if (!hasExactKeys(metadata, RELEASE_METADATA_KEYS) || metadata.schemaVersion !== PROTOCOL_VERSION ||
    metadata.purpose !== RELEASE_PURPOSE || !isObject(metadata.receipt)) {
    throw new Error('controlled artifact release metadata is invalid')
  }
  const receipt = metadata.receipt
  const context = receipt.context
  if (!hasExactKeys(receipt, RECEIPT_KEYS) || !isObject(context) || !hasExactKeys(context, CONTEXT_KEYS) ||
    receipt.schemaVersion !== PROTOCOL_VERSION || receipt.purpose !== 'analytix.controlled-artifact-access-receipt/v1' ||
    receipt.accessAction !== 'display' && receipt.accessAction !== 'export' ||
    receipt.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE || receipt.authorityAlgorithm !== 'Ed25519' ||
    !isSafePositive(receipt.rendererGeneration) || !isSafePositive(receipt.backendGeneration) ||
    !isSafePositive(receipt.artifactByteLength) || receipt.artifactByteLength > MAX_ARTIFACT_BYTES ||
    !isRFC3339(receipt.requestedAt) || !isRFC3339(receipt.authorizedUntil) ||
    context.version !== 2 || !isSafePositive(context.contextEpoch) ||
    !isRFC3339(context.contextIssuedAt) ||
    !DATASET_PATTERN.test(exactString(context.datasetSnapshotId))) {
    throw new Error('controlled artifact access receipt is invalid')
  }
  const digestFields = [
    receipt.accessId, receipt.controlledHandleDigest, receipt.useSlotDigest, receipt.rendererPrincipalDigest,
    receipt.accessPolicyDigest, receipt.retentionPolicyDigest, receipt.publicationCommitDigest,
    receipt.publicationReceiptDigest, receipt.piiProjectionDigest, receipt.piiAuthorizationDigest,
    receipt.claimLedgerDigest, receipt.targetIdentityDigest, receipt.artifactSha256, receipt.authorityKeyId,
    receipt.recordDigest, context.caseBindingHash, context.sourceManifestHash, context.contextDigest
  ]
  if (digestFields.some((value) => !SHA256_PATTERN.test(exactString(value))) ||
    receipt.accessId !== createHash('sha256').update(ACCESS_ID_DOMAIN).update(exactString(receipt.useSlotDigest), 'utf8').digest('hex') ||
    !isBoundedString(receipt.requesterUserId) || !isBoundedString(receipt.authorityPublicKey) ||
    !isBoundedString(receipt.authoritySignature) || !isBoundedString(context.threadId) ||
    !isBoundedString(context.turnId) || !isBoundedString(context.workspaceRealPath) ||
    !isBoundedString(context.tenantId) || !isBoundedString(context.userId) ||
    !isBoundedString(context.caseId) || context.caseId === 'unbound' || receipt.requesterUserId !== context.userId ||
    !validReceiptTimes(receipt, context) ||
    !verifyReleaseReceiptAuthority(receipt as unknown as ControlledArtifactReleaseReceiptV1)) {
    throw new Error('controlled artifact access receipt digest is invalid')
  }
  return receipt as unknown as ControlledArtifactReleaseReceiptV1
}

function isValidInvocationInput(input: ControlledArtifactInvocationInputV1, now: Date): boolean {
  if (!input || typeof input.release !== 'function' || !Number.isSafeInteger(input.webContentsId) || input.webContentsId <= 0 ||
    !Number.isSafeInteger(input.rendererGeneration) || input.rendererGeneration <= 0 ||
    input.action !== 'display' && input.action !== 'export' || !SHA256_PATTERN.test(input.publicationCommitDigest) ||
    !validAuthorityBinding(input.authorityKeyId, input.authorityPublicKey) ||
    !(input.authorizedUntil instanceof Date) || !Number.isFinite(input.authorizedUntil.getTime()) ||
    input.authorizedUntil.getTime() <= now.getTime() || input.authorizedUntil.getTime() - now.getTime() > MAX_ADMISSION_TTL_MS) {
    return false
  }
  const context = input.context
  return Boolean(context) && isBoundedString(context.threadId) && isBoundedString(context.turnId) &&
    isBoundedString(context.caseId) && SHA256_PATTERN.test(context.caseBindingHash) &&
    DATASET_PATTERN.test(context.datasetSnapshotId) && SHA256_PATTERN.test(context.sourceManifestHash) &&
    Number.isSafeInteger(context.contextEpoch) && context.contextEpoch > 0 && SHA256_PATTERN.test(context.contextDigest)
}

function verifyReleaseReceiptAuthority(receipt: ControlledArtifactReleaseReceiptV1): boolean {
  try {
    const publicKey = decodeCanonicalBase64URL(receipt.authorityPublicKey, 32)
    const signature = decodeCanonicalBase64URL(receipt.authoritySignature, 64)
    if (!publicKey || !signature || sha256(publicKey) !== receipt.authorityKeyId) return false
    const signingRecord = canonicalReleaseReceipt(receipt, '', '')
    const signingDigest = createHash('sha256').update(goJSONStringify(signingRecord), 'utf8').digest()
    const signingBytes = Buffer.concat([RECEIPT_SIGNATURE_DOMAIN, signingDigest])
    const key = createPublicKey({
      key: Buffer.concat([ED25519_SPKI_PREFIX, publicKey]),
      format: 'der',
      type: 'spki'
    })
    if (!verify(null, signingBytes, key, signature)) return false
    const record = canonicalReleaseReceipt(receipt, receipt.authoritySignature, '')
    return sha256(Buffer.concat([RECEIPT_RECORD_DOMAIN, Buffer.from(goJSONStringify(record), 'utf8')])) === receipt.recordDigest
  } catch {
    return false
  }
}

function validReceiptTimes(
  receipt: Record<string, unknown>,
  context: Record<string, unknown>
): boolean {
  const requestedAt = Date.parse(exactString(receipt.requestedAt))
  const authorizedUntil = Date.parse(exactString(receipt.authorizedUntil))
  const contextIssuedAt = Date.parse(exactString(context.contextIssuedAt))
  return Number.isFinite(requestedAt) && Number.isFinite(authorizedUntil) && Number.isFinite(contextIssuedAt) &&
    requestedAt >= contextIssuedAt && requestedAt < authorizedUntil &&
    authorizedUntil - requestedAt <= MAX_ADMISSION_TTL_MS
}

function canonicalReleaseReceipt(
  receipt: ControlledArtifactReleaseReceiptV1,
  authoritySignature: string,
  recordDigest: string
): Record<string, unknown> {
  const context = receipt.context
  return {
    schemaVersion: receipt.schemaVersion,
    purpose: receipt.purpose,
    accessId: receipt.accessId,
    context: {
      version: context.version,
      threadId: context.threadId,
      turnId: context.turnId,
      workspaceRealPath: context.workspaceRealPath,
      tenantId: context.tenantId,
      userId: context.userId,
      caseId: context.caseId,
      caseBindingHash: context.caseBindingHash,
      datasetSnapshotId: context.datasetSnapshotId,
      sourceManifestHash: context.sourceManifestHash,
      contextEpoch: context.contextEpoch,
      contextIssuedAt: context.contextIssuedAt,
      contextDigest: context.contextDigest
    },
    requesterUserId: receipt.requesterUserId,
    accessAction: receipt.accessAction,
    controlledHandleDigest: receipt.controlledHandleDigest,
    useSlotDigest: receipt.useSlotDigest,
    rendererPrincipalDigest: receipt.rendererPrincipalDigest,
    rendererGeneration: receipt.rendererGeneration,
    backendGeneration: receipt.backendGeneration,
    accessPolicyDigest: receipt.accessPolicyDigest,
    retentionPolicyDigest: receipt.retentionPolicyDigest,
    publicationCommitDigest: receipt.publicationCommitDigest,
    publicationReceiptDigest: receipt.publicationReceiptDigest,
    piiProjectionDigest: receipt.piiProjectionDigest,
    piiAuthorizationDigest: receipt.piiAuthorizationDigest,
    claimLedgerDigest: receipt.claimLedgerDigest,
    targetIdentityDigest: receipt.targetIdentityDigest,
    artifactSha256: receipt.artifactSha256,
    artifactByteLength: receipt.artifactByteLength,
    mediaType: receipt.mediaType,
    requestedAt: receipt.requestedAt,
    authorizedUntil: receipt.authorizedUntil,
    authorityAlgorithm: receipt.authorityAlgorithm,
    authorityKeyId: receipt.authorityKeyId,
    authorityPublicKey: receipt.authorityPublicKey,
    authoritySignature,
    recordDigest
  }
}

function validAuthorityBinding(keyId: string, publicKeyText: string): boolean {
  const publicKey = decodeCanonicalBase64URL(publicKeyText, 32)
  return SHA256_PATTERN.test(keyId) && publicKey !== null && sha256(publicKey) === keyId
}

function decodeCanonicalBase64URL(value: unknown, expectedBytes: number): Buffer | null {
  if (typeof value !== 'string' || value === '' || value.includes('=')) return null
  try {
    const decoded = Buffer.from(value, 'base64url')
    return decoded.length === expectedBytes && decoded.toString('base64url') === value ? decoded : null
  } catch {
    return null
  }
}

function goJSONStringify(value: unknown): string {
  const encoded = JSON.stringify(value)
  if (encoded === undefined) throw new Error('controlled artifact receipt is not serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function hasExactKeys(object: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(object).sort()
  const sorted = [...expected].sort()
  return keys.length === sorted.length && keys.every((key, index) => key === sorted[index])
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

function isSafePositive(value: unknown): value is number {
  return Number.isSafeInteger(value) && Number(value) > 0
}

function exactString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function isBoundedString(value: unknown): value is string {
  return typeof value === 'string' && value.trim() === value && value.length > 0 && Buffer.byteLength(value, 'utf8') <= 2048
}

function exactOpaque(value: unknown): string {
  return typeof value === 'string' && OPAQUE_PATTERN.test(value) ? value : ''
}

function opaqueDigest(domain: Buffer, token: string): string {
  return createHash('sha256').update(domain).update(token, 'utf8').digest('hex')
}

function sha256(body: Buffer): string {
  return createHash('sha256').update(body).digest('hex')
}

function isCanonicalSecret(value: string): boolean {
  if (!/^[A-Za-z0-9_-]{43}$/.test(value)) return false
  try {
    const body = Buffer.from(value, 'base64url')
    return body.length === SECRET_BYTES && body.toString('base64url') === value
  } catch {
    return false
  }
}

function safeEqual(left: unknown, right: string): boolean {
  if (typeof left !== 'string') return false
  const leftBody = Buffer.from(left, 'utf8')
  const rightBody = Buffer.from(right, 'utf8')
  return leftBody.length === rightBody.length && timingSafeEqual(leftBody, rightBody)
}

function singleHeader(request: IncomingMessage, target: string): string {
  const values: string[] = []
  for (let index = 0; index < request.rawHeaders.length; index += 2) {
    if (request.rawHeaders[index]?.toLowerCase() === target) values.push(request.rawHeaders[index + 1] ?? '')
  }
  return values.length === 1 ? values[0] : ''
}

function hasTransferEncoding(request: IncomingMessage): boolean {
  return singleHeader(request, 'transfer-encoding') !== ''
}

async function readRequestBody(request: IncomingMessage, maxBytes: number): Promise<Buffer | null> {
  const contentLength = singleHeader(request, 'content-length')
  if (!/^[1-9][0-9]*$/.test(contentLength)) return null
  const expected = Number(contentLength)
  if (!Number.isSafeInteger(expected) || expected <= 0 || expected > maxBytes) return null
  const chunks: Buffer[] = []
  let length = 0
  try {
    for await (const chunk of request) {
      const body = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
      length += body.length
      if (length > expected || length > maxBytes) return null
      chunks.push(body)
    }
  } catch {
    return null
  }
  if (length !== expected) return null
  return Buffer.concat(chunks, length)
}

function writeJson(response: ServerResponse, value: unknown): void {
  if (response.headersSent || response.destroyed) return
  const body = Buffer.from(JSON.stringify(value), 'utf8')
  response.statusCode = httpStatus.ok
  response.setHeader('Content-Type', JSON_MEDIA_TYPE)
  response.setHeader('Content-Length', String(body.length))
  response.end(body)
}

function writeFailure(response: ServerResponse, status: number): void {
  if (response.headersSent || response.destroyed) {
    response.destroy()
    return
  }
  const body = Buffer.from('{"code":"controlled_artifact_host_rejected"}', 'utf8')
  response.statusCode = status
  response.setHeader('Content-Type', 'application/json')
  response.setHeader('Content-Length', String(body.length))
  response.end(body)
}

function releaseAckMACMessage(ack: {
  schemaVersion: number
  purpose: string
  committed: boolean
  accessId: string
  accessReceiptDigest: string
  artifactSha256: string
  releasedByteLength: number
  accessAction: string
  controlledHandleDigest: string
  useSlotDigest: string
  rendererPrincipalDigest: string
  rendererGeneration: number
  backendGeneration: number
  committedAt: string
}): Buffer {
  const values = [
    String(ack.schemaVersion), ack.purpose, String(ack.committed), ack.accessId, ack.accessReceiptDigest,
    ack.artifactSha256, String(ack.releasedByteLength), ack.accessAction, ack.controlledHandleDigest,
    ack.useSlotDigest, ack.rendererPrincipalDigest, String(ack.rendererGeneration),
    String(ack.backendGeneration), ack.committedAt
  ]
  const parts: Buffer[] = [RELEASE_ACK_DOMAIN]
  for (const value of values) {
    parts.push(Buffer.from(`${Buffer.byteLength(value, 'utf8')}:`, 'ascii'), Buffer.from(value, 'utf8'))
  }
  return Buffer.concat(parts)
}

function formatRFC3339Nano(value: Date): string {
  const iso = value.toISOString()
  return iso.replace(/\.000Z$/, 'Z').replace(/(\.\d*?[1-9])0+Z$/, '$1Z')
}

function isRFC3339(value: unknown): value is string {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)) return false
  return Number.isFinite(Date.parse(value))
}

async function closeServer(server: Server): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    server.close((error) => error ? reject(error) : resolve())
    server.closeAllConnections?.()
  })
}

const httpStatus = {
  ok: 200,
  badRequest: 400,
  unauthorized: 401,
  notFound: 404,
  methodNotAllowed: 405,
  conflict: 409,
  internalError: 500
} as const
