import {
  constants as cryptoConstants,
  createHash,
  createHmac,
  createPublicKey,
  randomBytes,
  timingSafeEqual,
  verify
} from 'node:crypto'
import type { IncomingMessage, ServerResponse } from 'node:http'
import { createServer, type Server } from 'node:https'
import type { AddressInfo, Socket } from 'node:net'
import type { Duplex } from 'node:stream'
import { finished } from 'node:stream/promises'
import { TLSSocket } from 'node:tls'
import {
  generateEphemeralControlledArtifactTLSIdentityV2,
  type EphemeralControlledArtifactTLSIdentityV2
} from './ephemeral-tls-identity'
import { parseStrictJsonObject } from './strict-json'

export const CONTROLLED_ARTIFACT_HOST_V2_URL_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_URL'
export const CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TOKEN'
export const CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV =
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION'
export const CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV =
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST'
export const CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV =
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER'
export const CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV =
  'ANALYTIX_CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256'

const HOST = '127.0.0.1'
const PROBE_PATH = '/v2/controlled-artifacts/probe'
const ADMISSION_PATH = '/v2/controlled-artifacts/admission'
const RELEASE_PATH = '/v2/controlled-artifacts/release'
const ADMISSION_PURPOSE = 'analytix.controlled-artifact-admission/v2'
const PROBE_REQUEST_PURPOSE = 'analytix.controlled-artifact-host-probe-request/v2'
const PROBE_RESPONSE_PURPOSE = 'analytix.controlled-artifact-host-probe-response/v2'
const RELEASE_PURPOSE = 'analytix.controlled-artifact-release/v2'
const RELEASE_ACK_PURPOSE = 'analytix.controlled-artifact-release-ack/v2'
const ACCESS_RECEIPT_PURPOSE = 'analytix.controlled-artifact-access-receipt/v2'
const JSON_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-host-v2+json'
const RELEASE_MEDIA_TYPE = 'application/vnd.analytix.controlled-artifact-release-v2'
const CONTROLLED_ARTIFACT_MEDIA_TYPE = 'application/vnd.analytix.controlled-case-evidence+json'
const PROTOCOL_VERSION = 2
const SECRET_BYTES = 32
const MAX_JSON_BYTES = 64 << 10
const MAX_METADATA_BYTES = (256 << 10) + 4096
const MAX_ARTIFACT_BYTES = 8 << 20
const MAX_RELEASE_BYTES = 8 + 4 + MAX_METADATA_BYTES + MAX_ARTIFACT_BYTES
const MAX_ADMISSION_TTL_NS = 15n * 60n * 1_000_000_000n
const MAX_RELEASE_EFFECT_MS = 30_000
const MAX_ABORT_EFFECT_MS = 3_000
const RELEASE_MAGIC = Buffer.from('ANXCRV2\n', 'ascii')
const RELEASE_ACK_DOMAIN = Buffer.from('analytix.controlled-artifact-release-ack/hmac/v2\0', 'utf8')
const HANDLE_DIGEST_DOMAIN = Buffer.from('analytix.controlled-artifact-access/handle/v2\0', 'utf8')
const SLOT_DIGEST_DOMAIN = Buffer.from('analytix.controlled-artifact-access/use-slot/v2\0', 'utf8')
const PRINCIPAL_DIGEST_DOMAIN = Buffer.from(
  'analytix.controlled-artifact-access/renderer-principal/v2\0',
  'utf8'
)
const ACCESS_ID_DOMAIN = Buffer.from('analytix.controlled-artifact-access/id/v2\0', 'utf8')
const RECEIPT_SIGNATURE_DOMAIN = Buffer.from(
  'analytix.controlled-artifact-access-receipt/signature/v2\0',
  'utf8'
)
const RECEIPT_RECORD_DOMAIN = Buffer.from(
  'analytix.controlled-artifact-access-receipt/record/v2\0',
  'utf8'
)
const ED25519_SPKI_PREFIX = Buffer.from('302a300506032b6570032100', 'hex')
const OPAQUE_PATTERN = /^[A-Za-z0-9._~-]{16,1024}$/
const SHA256_PATTERN = /^[a-f0-9]{64}$/
const DATASET_PATTERN = /^dsv2_[a-f0-9]{64}$/

const ADMISSION_REQUEST_KEYS = [
  'schemaVersion', 'purpose', 'context', 'accessAction', 'controlledHandle', 'useSlot',
  'rendererPrincipal', 'rendererGeneration', 'backendGeneration'
] as const
const PROBE_REQUEST_KEYS = ['schemaVersion', 'purpose', 'probeNonce', 'backendGeneration'] as const
const RELEASE_METADATA_KEYS = ['schemaVersion', 'purpose', 'receipt'] as const
const RECEIPT_KEYS = [
  'schemaVersion', 'purpose', 'accessId', 'context', 'requesterUserId', 'accessAction',
  'controlledHandleDigest', 'useSlotDigest', 'rendererPrincipalDigest', 'rendererGeneration',
  'backendGeneration', 'accessPolicyDigest', 'retentionPolicyDigest', 'deliveryId',
  'deliveryOutcomeRecordDigest', 'publicationCommitDigest', 'publicationReceiptDigest',
  'piiProjectionDigest', 'piiAuthorizationDigest', 'claimLedgerDigest', 'targetIdentityDigest',
  'releaseTargetIdentityDigest', 'artifactSha256', 'artifactByteLength', 'mediaType',
  'requestedAt', 'authorizedUntil',
  'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature', 'recordDigest'
] as const
const CONTEXT_KEYS = [
  'version', 'threadId', 'turnId', 'workspaceRealPath', 'tenantId', 'userId', 'caseId',
  'caseBindingHash', 'datasetSnapshotId', 'sourceManifestHash', 'contextEpoch',
  'contextIssuedAt', 'contextDigest'
] as const

export type ControlledArtifactAccessActionV2 = 'display' | 'export'

export type ControlledArtifactContextBindingV2 = {
  version: number
  threadId: string
  turnId: string
  workspaceRealPath: string
  tenantId: string
  userId: string
  caseId: string
  caseBindingHash: string
  datasetSnapshotId: string
  sourceManifestHash: string
  contextEpoch: number
  contextIssuedAt: string
  contextDigest: string
}

export type ControlledArtifactInvocationContextV2 = {
  threadId: string
  turnId: string
  contextEpoch: number
  contextDigest: string
}

export type ControlledArtifactReleaseReceiptV2 = {
  schemaVersion: number
  purpose: string
  accessId: string
  context: ControlledArtifactContextBindingV2
  requesterUserId: string
  accessAction: ControlledArtifactAccessActionV2
  controlledHandleDigest: string
  useSlotDigest: string
  rendererPrincipalDigest: string
  rendererGeneration: number
  backendGeneration: number
  accessPolicyDigest: string
  retentionPolicyDigest: string
  deliveryId: string
  deliveryOutcomeRecordDigest: string
  publicationCommitDigest: string
  publicationReceiptDigest: string
  piiProjectionDigest: string
  piiAuthorizationDigest: string
  claimLedgerDigest: string
  targetIdentityDigest: string
  releaseTargetIdentityDigest: string
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

export type ControlledArtifactReleaseEffectInputV2 = {
  action: ControlledArtifactAccessActionV2
  body: Buffer
  receipt: ControlledArtifactReleaseReceiptV2
  releaseTargetIdentityDigest: string
}

export type ControlledArtifactPreparedReleaseV2 = {
  commitBefore: (
    authorizedUntil: string,
    signal: AbortSignal
  ) => Promise<ControlledArtifactCommitReceiptV2>
  abort: (signal: AbortSignal) => Promise<void>
}

export type ControlledArtifactCommitReceiptV2 = {
  releaseTargetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
  mediaType: string
  committedAt: string
}

export type ControlledArtifactReleasePreparationV2 = (
  input: ControlledArtifactReleaseEffectInputV2,
  signal: AbortSignal
) => Promise<ControlledArtifactPreparedReleaseV2>

export type ControlledArtifactHostRuntimeEnvironmentV2 = Readonly<{
  [CONTROLLED_ARTIFACT_HOST_V2_URL_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV]: string
  [CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV]: string
}>

export type ControlledArtifactHostOptionsV2 = {
  backendGeneration: number
  allocationRecordDigest: string
  acquireRendererLease: (
    webContentsId: number,
    rendererGeneration: number
  ) => ControlledArtifactRendererLeaseV2 | null
  now?: () => string
  monotonicNow?: () => bigint
  random?: (size: number) => Buffer
  createTLSIdentity?: () => Promise<EphemeralControlledArtifactTLSIdentityV2>
  releaseEffectTimeoutMs?: number
  abortEffectTimeoutMs?: number
}

export type ControlledArtifactInvocationInputV2 = {
  context: ControlledArtifactInvocationContextV2
  accessPolicyDigest: string
  retentionPolicyDigest: string
  deliveryId: string
  deliveryOutcomeRecordDigest: string
  publicationCommitDigest: string
  publicationReceiptDigest: string
  piiProjectionDigest: string
  piiAuthorizationDigest: string
  claimLedgerDigest: string
  targetIdentityDigest: string
  releaseTargetIdentityDigest: string
  artifactSha256: string
  artifactByteLength: number
  mediaType: string
  authorityKeyId: string
  authorityPublicKey: string
  action: ControlledArtifactAccessActionV2
  webContentsId: number
  rendererGeneration: number
  authorizedUntil: string
  prepareRelease: ControlledArtifactReleasePreparationV2
}

export type ControlledArtifactRendererLeaseV2 = {
  signal: AbortSignal
  isCurrent: () => boolean
  release: () => void
}

export class ControlledArtifactRuntimeReleaseInvocationV2 {
  readonly #threadId: string
  readonly #turnId: string
  readonly #accessAction: ControlledArtifactAccessActionV2
  readonly #controlledHandle: string
  readonly #useSlot: string
  readonly #rendererPrincipal: string
  readonly #rendererGeneration: number
  readonly #backendGeneration: number

  private constructor(input: {
    threadId: string
    turnId: string
    accessAction: ControlledArtifactAccessActionV2
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
    accessAction: ControlledArtifactAccessActionV2
    controlledHandle: string
    useSlot: string
    rendererPrincipal: string
    rendererGeneration: number
    backendGeneration: number
  }): ControlledArtifactRuntimeReleaseInvocationV2 {
    return new ControlledArtifactRuntimeReleaseInvocationV2(input)
  }

  get threadId(): string { return this.#threadId }
  get turnId(): string { return this.#turnId }
  get accessAction(): ControlledArtifactAccessActionV2 { return this.#accessAction }
  get controlledHandle(): string { return this.#controlledHandle }
  get useSlot(): string { return this.#useSlot }
  get rendererPrincipal(): string { return this.#rendererPrincipal }
  get rendererGeneration(): number { return this.#rendererGeneration }
  get backendGeneration(): number { return this.#backendGeneration }

  toJSON(): never {
    throw new Error('controlled artifact invocation is main-process-only')
  }
}

type EntryState = 'ready' | 'releasing' | 'committed' | 'indeterminate' | 'revoked'
type HostState = 'new' | 'running' | 'revoked' | 'closed'

type ControlledArtifactHostEntryV2 = ControlledArtifactInvocationInputV2 & {
  controlledHandle: string
  controlledHandleDigest: string
  useSlot: string
  useSlotDigest: string
  rendererPrincipal: string
  rendererPrincipalDigest: string
  backendGeneration: number
  authorizedUntilNS: bigint
  authorizedUntilMonotonicNS: bigint
  state: EntryState
}

type HostClockSnapshot = {
  wallNS: bigint
  monotonicNS: bigint
}

type ControlledArtifactAdmissionRequestV2 = {
  schemaVersion: number
  purpose: string
  context: ControlledArtifactContextBindingV2
  accessAction: ControlledArtifactAccessActionV2
  controlledHandle: string
  useSlot: string
  rendererPrincipal: string
  rendererGeneration: number
  backendGeneration: number
}

type ControlledArtifactProbeRequestV2 = {
  schemaVersion: number
  purpose: string
  probeNonce: string
  backendGeneration: number
}

export class ControlledArtifactHostV2 {
  private server: Server | null = null
  private state: HostState = 'new'
  private transportAccepting = false
  private invocationsActive = false
  private invocationsWereActivated = false
  private runtimeEnvironmentIssued = false
  private clockCompromised = false
  private lastWallNS: bigint | null = null
  private lastMonotonicNS: bigint | null = null
  private origin = ''
  private secretBytes: Buffer = Buffer.alloc(0)
  private rootCertificateDERBase64URL = ''
  private leafSPKISHA256 = ''
  private certificateNotAfter: Date | null = null
  private startPromise: Promise<void> | null = null
  private closePromise: Promise<void> | null = null
  private activeHandlers = 0
  private drainWaiters: Array<() => void> = []
  private readonly sockets = new Set<Duplex>()
  private readonly releaseOperations = new Set<Promise<unknown>>()
  private readonly entries = new Map<string, ControlledArtifactHostEntryV2>()
  private readonly entriesByDigest = new Map<string, ControlledArtifactHostEntryV2>()
  private readonly issuedTokens = new Set<string>()
  private readonly now: () => string
  private readonly monotonicNow: () => bigint
  private readonly random: (size: number) => Buffer
  private readonly createTLSIdentity: () => Promise<EphemeralControlledArtifactTLSIdentityV2>
  private readonly backendGeneration: number
  private readonly allocationRecordDigest: string
  private readonly acquireRendererLease: ControlledArtifactHostOptionsV2['acquireRendererLease']
  private readonly releaseEffectTimeoutMs: number
  private readonly abortEffectTimeoutMs: number

  constructor(options: ControlledArtifactHostOptionsV2) {
    if (!options || !isSafePositive(options.backendGeneration) ||
      !SHA256_PATTERN.test(options.allocationRecordDigest) ||
      typeof options.acquireRendererLease !== 'function' ||
      options.releaseEffectTimeoutMs !== undefined &&
        (!isSafePositive(options.releaseEffectTimeoutMs) || options.releaseEffectTimeoutMs > MAX_RELEASE_EFFECT_MS) ||
      options.abortEffectTimeoutMs !== undefined &&
        (!isSafePositive(options.abortEffectTimeoutMs) || options.abortEffectTimeoutMs > MAX_ABORT_EFFECT_MS)) {
      throw new Error('controlled artifact V2 host authority is invalid')
    }
    this.now = options.now ?? (() => formatRFC3339Nano(new Date()))
    this.monotonicNow = options.monotonicNow ?? (() => process.hrtime.bigint())
    this.random = options.random ?? randomBytes
    this.createTLSIdentity = options.createTLSIdentity ?? generateEphemeralControlledArtifactTLSIdentityV2
    this.backendGeneration = options.backendGeneration
    this.allocationRecordDigest = options.allocationRecordDigest
    this.acquireRendererLease = options.acquireRendererLease
    this.releaseEffectTimeoutMs = options.releaseEffectTimeoutMs ?? MAX_RELEASE_EFFECT_MS
    this.abortEffectTimeoutMs = options.abortEffectTimeoutMs ?? MAX_ABORT_EFFECT_MS
  }

  start(): Promise<void> {
    if (this.state === 'running') return Promise.resolve()
    if (this.startPromise) return this.startPromise
    if (this.state !== 'new') throw new Error('controlled artifact V2 host cannot be restarted')
    const operation = this.startOnce()
    this.startPromise = operation
    void operation.finally(() => {
      if (this.startPromise === operation) this.startPromise = null
    }).catch(() => undefined)
    return operation
  }

  private async startOnce(): Promise<void> {
    const secret = this.createSecretBytes()
    let identity: EphemeralControlledArtifactTLSIdentityV2 | null = null
    let privateKeyPEM: Buffer | null = null
    let server: Server | null = null
    try {
      identity = await this.createTLSIdentity()
      privateKeyPEM = identity.takeLeafPrivateKeyPEM()
      server = createServer({
        key: privateKeyPEM,
        cert: identity.leafCertificatePEM,
        minVersion: 'TLSv1.3',
        maxVersion: 'TLSv1.3',
        ALPNProtocols: ['http/1.1'],
        requestCert: false,
        rejectUnauthorized: false,
        handshakeTimeout: 3_000,
        secureOptions: cryptoConstants.SSL_OP_NO_TICKET
      }, (request, response) => {
        this.activeHandlers += 1
        void (async () => {
          try {
            await this.handle(request, response)
          } catch {
            writeFailure(response, httpStatus.internalError)
          }
          try {
            await finished(response, { cleanup: true })
          } catch {
            // An aborted response is still terminal for this handler. The Go
            // caller records a post-sink abort as release_indeterminate.
          } finally {
            this.finishHandler()
          }
        })()
      })
      privateKeyPEM.fill(0)
      privateKeyPEM = null
      server.maxHeadersCount = 16
      server.maxConnections = 32
      server.maxRequestsPerSocket = 1
      server.headersTimeout = 5_000
      server.requestTimeout = 35_000
      server.keepAliveTimeout = 1
      server.on('tlsClientError', () => undefined)
      server.on('connection', (socket) => {
        this.sockets.add(socket)
        socket.once('close', () => this.sockets.delete(socket))
      })
      server.on('secureConnection', (socket) => {
        socket.setNoDelay(true)
        socket.setTimeout(35_000, () => socket.destroy())
        if (socket.alpnProtocol !== 'http/1.1') socket.destroy()
      })
      await listenExactLoopback(server)
      const address = server.address() as AddressInfo | null
      if (!address || address.address !== HOST || address.family !== 'IPv4' ||
        !Number.isSafeInteger(address.port) || address.port <= 0) {
        throw new Error('controlled artifact V2 host did not bind the exact loopback authority')
      }
      this.server = server
      this.secretBytes = secret
      this.origin = `https://${HOST}:${address.port}`
      this.rootCertificateDERBase64URL = identity.rootCertificateDERBase64URL
      this.leafSPKISHA256 = identity.leafSPKISHA256
      this.certificateNotAfter = new Date(identity.notAfter.getTime())
      this.clockSnapshot()
      this.transportAccepting = true
      this.invocationsActive = false
      this.state = 'running'
    } catch (error) {
      secret.fill(0)
      if (server) await closeFailedServer(server)
      this.state = 'closed'
      throw error
    } finally {
      privateKeyPEM?.fill(0)
      identity = null
    }
  }

  runtimeEnvironment(): ControlledArtifactHostRuntimeEnvironmentV2 {
    if (this.state !== 'running' || !this.transportAccepting || this.clockCompromised ||
      !this.server || !this.origin || this.runtimeEnvironmentIssued ||
      this.secretBytes.length !== SECRET_BYTES ||
      !/^[A-Za-z0-9_-]+$/.test(this.rootCertificateDERBase64URL) ||
      !SHA256_PATTERN.test(this.leafSPKISHA256)) {
      throw new Error('controlled artifact V2 host is not running')
    }
    this.runtimeEnvironmentIssued = true
    return Object.freeze({
      [CONTROLLED_ARTIFACT_HOST_V2_URL_ENV]: this.origin,
      [CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV]: this.secretBytes.toString('base64url'),
      [CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV]: String(this.backendGeneration),
      [CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV]: this.allocationRecordDigest,
      [CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV]: this.rootCertificateDERBase64URL,
      [CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV]: this.leafSPKISHA256
    })
  }

  notAfter(): Date {
    if (!this.certificateNotAfter) throw new Error('controlled artifact V2 host is not running')
    return new Date(this.certificateNotAfter.getTime())
  }

  activateInvocations(): void {
    if (this.state !== 'running' || !this.transportAccepting || this.clockCompromised ||
      this.invocationsWereActivated || !this.server) {
      throw new Error('controlled artifact V2 host is not running')
    }
    this.invocationsActive = true
    this.invocationsWereActivated = true
  }

  quiesceInvocations(): void {
    this.invocationsActive = false
  }

  createInvocation(input: ControlledArtifactInvocationInputV2): ControlledArtifactRuntimeReleaseInvocationV2 {
    const now = this.clockSnapshot()
    const certificateNotAfterNS = this.certificateNotAfter
      ? BigInt(this.certificateNotAfter.getTime()) * 1_000_000n
      : 0n
    if (this.state !== 'running' || !this.transportAccepting || !this.invocationsActive || !this.server ||
      !isValidInvocationInput(input, now.wallNS, certificateNotAfterNS)) {
      throw new Error('controlled artifact V2 invocation is unavailable')
    }
    this.pruneExpired(now)
    const controlledHandle = this.uniqueToken()
    const useSlot = this.uniqueToken()
    const rendererPrincipal = this.uniqueToken()
    const entry: ControlledArtifactHostEntryV2 = {
      ...input,
      context: { ...input.context },
      controlledHandle,
      controlledHandleDigest: opaqueDigest(HANDLE_DIGEST_DOMAIN, controlledHandle),
      useSlot,
      useSlotDigest: opaqueDigest(SLOT_DIGEST_DOMAIN, useSlot),
      rendererPrincipal,
      rendererPrincipalDigest: opaqueDigest(PRINCIPAL_DIGEST_DOMAIN, rendererPrincipal),
      backendGeneration: this.backendGeneration,
      authorizedUntilNS: parseRFC3339Nano(input.authorizedUntil)!,
      authorizedUntilMonotonicNS: now.monotonicNS +
        (parseRFC3339Nano(input.authorizedUntil)! - now.wallNS),
      state: 'ready'
    }
    this.entries.set(controlledHandle, entry)
    this.entriesByDigest.set(entry.controlledHandleDigest, entry)
    return ControlledArtifactRuntimeReleaseInvocationV2.fromHost({
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
        (rendererGeneration === undefined || entry.rendererGeneration === rendererGeneration) &&
        entry.state === 'ready') {
        entry.state = 'revoked'
      }
    }
  }

  revokeTransport(): void {
    this.quiesceInvocations()
    if (this.state !== 'running') return
    this.transportAccepting = false
    this.state = 'revoked'
    for (const entry of this.entries.values()) {
      if (entry.state === 'ready') entry.state = 'revoked'
    }
    const server = this.server
    if (server && !this.closePromise) {
      this.closePromise = new Promise<void>((resolve, reject) => {
        server.close((error) => error ? reject(error) : resolve())
        server.closeIdleConnections?.()
      })
    }
  }

  async closeAndDrain(): Promise<void> {
    const starting = this.startPromise
    if (starting) {
      try {
        await starting
      } catch {
        this.state = 'closed'
        return
      }
    }
    if (this.state === 'closed') return
    if (this.state === 'new') {
      this.state = 'closed'
      return
    }
    this.revokeTransport()
    const server = this.server
    let closeError: unknown = null
    try {
      await this.waitForHandlers()
      await this.waitForReleaseOperations()
      for (const socket of this.sockets) socket.destroy()
      server?.closeAllConnections?.()
      await this.closePromise
    } catch (error) {
      closeError = error
    } finally {
      this.server = null
      this.origin = ''
      this.invocationsActive = false
      this.transportAccepting = false
      this.secretBytes.fill(0)
      this.secretBytes = Buffer.alloc(0)
      this.rootCertificateDERBase64URL = ''
      this.leafSPKISHA256 = ''
      this.certificateNotAfter = null
      this.clockCompromised = true
      this.entries.clear()
      this.entriesByDigest.clear()
      this.issuedTokens.clear()
      this.sockets.clear()
      this.releaseOperations.clear()
      this.state = 'closed'
    }
    if (closeError) throw closeError
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
    if (request.url === PROBE_PATH) {
      await this.handleProbe(request, response)
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

  private async handleProbe(request: IncomingMessage, response: ServerResponse): Promise<void> {
    if (singleHeader(request, 'content-type') !== JSON_MEDIA_TYPE || hasEncodedBody(request)) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    if (this.invocationsActive || this.invocationsWereActivated) {
      writeFailure(response, httpStatus.conflict)
      return
    }
    const body = await readRequestBodySingleAllocation(request, MAX_JSON_BYTES)
    if (!body) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    let value: ControlledArtifactProbeRequestV2
    try {
      value = parseCanonicalProbeRequest(body)
    } catch {
      writeFailure(response, httpStatus.badRequest)
      return
    } finally {
      body.fill(0)
    }
    if (value.backendGeneration !== this.backendGeneration) {
      writeFailure(response, httpStatus.conflict)
      return
    }
    writeJson(response, {
      schemaVersion: PROTOCOL_VERSION,
      purpose: PROBE_RESPONSE_PURPOSE,
      probeNonce: value.probeNonce,
      backendGeneration: this.backendGeneration,
      transportReady: true,
      invocationsActive: false
    })
  }

  private authorizeRequest(request: IncomingMessage): boolean {
    const socket = request.socket
    if (this.state !== 'running' || !this.transportAccepting || !this.server ||
      !(socket instanceof TLSSocket) || !socket.encrypted || socket.alpnProtocol !== 'http/1.1' ||
      request.httpVersion !== '1.1' || socket.remoteAddress !== HOST ||
      singleHeader(request, 'host') !== this.origin.slice('https://'.length) ||
      singleHeader(request, 'accept') !== JSON_MEDIA_TYPE) {
      return false
    }
    return bearerMatches(singleHeader(request, 'authorization'), this.secretBytes)
  }

  private async handleAdmission(request: IncomingMessage, response: ServerResponse): Promise<void> {
    if (singleHeader(request, 'content-type') !== JSON_MEDIA_TYPE || hasEncodedBody(request)) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    const body = await readRequestBodySingleAllocation(request, MAX_JSON_BYTES)
    if (!body) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    let value: ControlledArtifactAdmissionRequestV2
    try {
      value = parseCanonicalAdmissionRequest(body)
    } catch {
      writeFailure(response, httpStatus.badRequest)
      return
    } finally {
      body.fill(0)
    }
    const entry = this.entries.get(value.controlledHandle)
    if (!entry || !this.admissionMatches(entry, value)) {
      writeFailure(response, httpStatus.conflict)
      return
    }
    writeJson(response, {
      schemaVersion: PROTOCOL_VERSION,
      purpose: ADMISSION_PURPOSE,
      contextDigest: entry.context.contextDigest,
      accessAction: entry.action,
      controlledHandleDigest: entry.controlledHandleDigest,
      useSlotDigest: entry.useSlotDigest,
      rendererPrincipalDigest: entry.rendererPrincipalDigest,
      rendererGeneration: entry.rendererGeneration,
      backendGeneration: entry.backendGeneration,
      deliveryId: entry.deliveryId,
      deliveryOutcomeRecordDigest: entry.deliveryOutcomeRecordDigest,
      publicationCommitDigest: entry.publicationCommitDigest,
      releaseTargetIdentityDigest: entry.releaseTargetIdentityDigest,
      authorizedUntil: entry.authorizedUntil
    })
  }

  private admissionMatches(
    entry: ControlledArtifactHostEntryV2,
    value: ControlledArtifactAdmissionRequestV2
  ): boolean {
    const now = this.clockSnapshot()
    return (entry.state === 'ready' || entry.state === 'committed') &&
      now.wallNS < entry.authorizedUntilNS && now.monotonicNS < entry.authorizedUntilMonotonicNS &&
      this.backendGeneration === entry.backendGeneration &&
      this.rendererIsCurrent(entry.webContentsId, entry.rendererGeneration) &&
      sameInvocationContext(entry.context, value.context) && value.accessAction === entry.action &&
      safeEqual(value.controlledHandle, entry.controlledHandle) && safeEqual(value.useSlot, entry.useSlot) &&
      safeEqual(value.rendererPrincipal, entry.rendererPrincipal) &&
      value.rendererGeneration === entry.rendererGeneration &&
      value.backendGeneration === entry.backendGeneration
  }

  private async handleRelease(request: IncomingMessage, response: ServerResponse): Promise<void> {
    if (singleHeader(request, 'content-type') !== RELEASE_MEDIA_TYPE || hasEncodedBody(request)) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    const frame = await readRequestBodySingleAllocation(request, MAX_RELEASE_BYTES)
    if (!frame) {
      writeFailure(response, httpStatus.badRequest)
      return
    }
    try {
      let parsed: ReturnType<typeof parseReleaseFrame>
      let receipt: ControlledArtifactReleaseReceiptV2
      try {
        parsed = parseReleaseFrame(frame)
        receipt = parseReleaseReceipt(parsed.metadata)
      } catch {
        writeFailure(response, httpStatus.badRequest)
        return
      }
      const entry = this.entriesByDigest.get(receipt.controlledHandleDigest)
      if (!entry || !this.releaseMatches(entry, receipt, parsed.body)) {
        writeFailure(response, httpStatus.conflict)
        return
      }
      const rendererLease = this.acquireRendererLease(entry.webContentsId, entry.rendererGeneration)
      const rendererSignal = rendererLease ? rendererLeaseSignal(rendererLease) : null
      if (!rendererLease || !rendererSignal || !rendererLeaseIsCurrent(rendererLease, rendererSignal)) {
        if (rendererLease) releaseRendererLease(rendererLease)
        writeFailure(response, httpStatus.conflict)
        return
      }
      entry.state = 'releasing'
      let prepared: ControlledArtifactPreparedReleaseV2 | null = null
      let preparationOperation: Promise<ControlledArtifactPreparedReleaseV2> | null = null
      let committed = false
      const effectSignal = AbortSignal.any([
        AbortSignal.timeout(this.releaseEffectTimeoutMs),
        rendererSignal
      ])
      try {
        try {
          preparationOperation = this.trackReleaseOperation(
            Promise.resolve().then(() => entry.prepareRelease(
              {
                action: entry.action,
                body: parsed.body,
                receipt,
                releaseTargetIdentityDigest: entry.releaseTargetIdentityDigest
              },
              effectSignal
            ))
          )
          prepared = await awaitWithAbort(
            preparationOperation,
            effectSignal
          )
        } catch {
          entry.state = 'indeterminate'
          if (preparationOperation) this.scheduleLatePreparedAbort(preparationOperation)
          writeFailure(response, httpStatus.internalError)
          return
        }
        if (!prepared || typeof prepared.commitBefore !== 'function' || typeof prepared.abort !== 'function') {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.internalError)
          return
        }
        let preCommitAt: HostClockSnapshot
        try {
          preCommitAt = this.clockSnapshot()
        } catch {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.conflict)
          return
        }
        const requestedAtNS = parseRFC3339Nano(receipt.requestedAt)
        if (entry.state !== 'releasing' || requestedAtNS === null || preCommitAt.wallNS < requestedAtNS ||
          preCommitAt.wallNS >= entry.authorizedUntilNS ||
          preCommitAt.monotonicNS >= entry.authorizedUntilMonotonicNS ||
          this.backendGeneration !== entry.backendGeneration ||
          !rendererLeaseIsCurrent(rendererLease, rendererSignal)) {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.conflict)
          return
        }
        let commitReceipt: ControlledArtifactCommitReceiptV2
        try {
          const commitOperation = this.trackReleaseOperation(
            Promise.resolve().then(() => prepared!.commitBefore(entry.authorizedUntil, effectSignal))
          )
          commitReceipt = await awaitWithAbort(
            commitOperation,
            effectSignal
          )
        } catch {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.internalError)
          return
        }
        if (!isControlledArtifactCommitReceiptV2(commitReceipt)) {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.internalError)
          return
        }
        const committedAtNS = parseRFC3339Nano(commitReceipt.committedAt)
        let postCommitAt: HostClockSnapshot
        try {
          postCommitAt = this.clockSnapshot()
        } catch {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.conflict)
          return
        }
        if (entry.state !== 'releasing' || committedAtNS === null || committedAtNS < requestedAtNS ||
          committedAtNS < preCommitAt.wallNS ||
          committedAtNS >= entry.authorizedUntilNS ||
          committedAtNS > postCommitAt.wallNS || postCommitAt.wallNS >= entry.authorizedUntilNS ||
          postCommitAt.monotonicNS >= entry.authorizedUntilMonotonicNS ||
          commitReceipt.releaseTargetIdentityDigest !== entry.releaseTargetIdentityDigest ||
          commitReceipt.artifactSha256 !== entry.artifactSha256 ||
          commitReceipt.artifactByteLength !== entry.artifactByteLength ||
          commitReceipt.mediaType !== entry.mediaType ||
          this.backendGeneration !== entry.backendGeneration ||
          !rendererLeaseIsCurrent(rendererLease, rendererSignal)) {
          entry.state = 'indeterminate'
          writeFailure(response, httpStatus.conflict)
          return
        }
        entry.state = 'committed'
        committed = true
        const ack = {
          schemaVersion: PROTOCOL_VERSION,
          purpose: RELEASE_ACK_PURPOSE,
          committed: true,
          accessId: receipt.accessId,
          accessReceiptDigest: receipt.recordDigest,
          contextDigest: receipt.context.contextDigest,
          deliveryId: receipt.deliveryId,
          deliveryOutcomeRecordDigest: receipt.deliveryOutcomeRecordDigest,
          publicationCommitDigest: receipt.publicationCommitDigest,
          releaseTargetIdentityDigest: receipt.releaseTargetIdentityDigest,
          artifactSha256: receipt.artifactSha256,
          releasedByteLength: receipt.artifactByteLength,
          accessAction: receipt.accessAction,
          controlledHandleDigest: receipt.controlledHandleDigest,
          useSlotDigest: receipt.useSlotDigest,
          rendererPrincipalDigest: receipt.rendererPrincipalDigest,
          rendererGeneration: receipt.rendererGeneration,
          backendGeneration: receipt.backendGeneration,
          committedAt: commitReceipt.committedAt,
          mac: ''
        }
        ack.mac = createHmac('sha256', this.secretBytes).update(releaseAckMACMessage(ack)).digest('base64url')
        writeJson(response, ack)
      } finally {
        try {
          if (prepared && !committed) {
            await this.abortPreparedRelease(prepared)
          }
        } catch {
          this.markAuthorityUnhealthy()
        } finally {
          if (!releaseRendererLease(rendererLease)) this.markAuthorityUnhealthy()
        }
      }
    } finally {
      frame.fill(0)
    }
  }

  private releaseMatches(
    entry: ControlledArtifactHostEntryV2,
    receipt: ControlledArtifactReleaseReceiptV2,
    body: Buffer
  ): boolean {
    const now = this.clockSnapshot()
    const requestedAt = parseRFC3339Nano(receipt.requestedAt)
    return entry.state === 'ready' && requestedAt !== null && requestedAt <= now.wallNS &&
      now.wallNS < entry.authorizedUntilNS && now.monotonicNS < entry.authorizedUntilMonotonicNS &&
      this.backendGeneration === entry.backendGeneration &&
      receipt.accessAction === entry.action &&
      receipt.controlledHandleDigest === entry.controlledHandleDigest &&
      receipt.useSlotDigest === entry.useSlotDigest &&
      receipt.rendererPrincipalDigest === entry.rendererPrincipalDigest &&
      receipt.rendererGeneration === entry.rendererGeneration &&
      receipt.backendGeneration === entry.backendGeneration &&
      receipt.accessPolicyDigest === entry.accessPolicyDigest &&
      receipt.retentionPolicyDigest === entry.retentionPolicyDigest &&
      receipt.deliveryId === entry.deliveryId &&
      receipt.deliveryOutcomeRecordDigest === entry.deliveryOutcomeRecordDigest &&
      receipt.publicationCommitDigest === entry.publicationCommitDigest &&
      receipt.publicationReceiptDigest === entry.publicationReceiptDigest &&
      receipt.piiProjectionDigest === entry.piiProjectionDigest &&
      receipt.piiAuthorizationDigest === entry.piiAuthorizationDigest &&
      receipt.claimLedgerDigest === entry.claimLedgerDigest &&
      receipt.targetIdentityDigest === entry.targetIdentityDigest &&
      receipt.releaseTargetIdentityDigest === entry.releaseTargetIdentityDigest &&
      receipt.artifactSha256 === entry.artifactSha256 &&
      receipt.artifactByteLength === entry.artifactByteLength &&
      receipt.mediaType === entry.mediaType &&
      receipt.authorizedUntil === entry.authorizedUntil &&
      receipt.authorityKeyId === entry.authorityKeyId &&
      receipt.authorityPublicKey === entry.authorityPublicKey &&
      sameInvocationContext(entry.context, receipt.context) &&
      receipt.artifactByteLength === body.length && receipt.artifactSha256 === sha256(body)
  }

  private pruneExpired(now: HostClockSnapshot): void {
    for (const [handle, entry] of this.entries) {
      if (entry.authorizedUntilNS > now.wallNS &&
        entry.authorizedUntilMonotonicNS > now.monotonicNS) continue
      if (entry.state === 'ready') entry.state = 'revoked'
      this.entries.delete(handle)
      this.entriesByDigest.delete(entry.controlledHandleDigest)
      this.issuedTokens.delete(entry.controlledHandle)
      this.issuedTokens.delete(entry.useSlot)
      this.issuedTokens.delete(entry.rendererPrincipal)
    }
  }

  private uniqueToken(): string {
    for (let attempt = 0; attempt < 4; attempt += 1) {
      const bytes = this.createSecretBytes()
      try {
        const token = bytes.toString('base64url')
        if (!this.issuedTokens.has(token)) {
          this.issuedTokens.add(token)
          return token
        }
      } finally {
        bytes.fill(0)
      }
    }
    throw new Error('controlled artifact V2 invocation token collision')
  }

  private createSecretBytes(): Buffer {
    const generated = this.random(SECRET_BYTES)
    try {
      if (!Buffer.isBuffer(generated) || generated.length !== SECRET_BYTES) {
        throw new Error('controlled artifact V2 secret generation failed')
      }
      return Buffer.from(generated)
    } finally {
      if (Buffer.isBuffer(generated)) generated.fill(0)
    }
  }

  private clockSnapshot(): HostClockSnapshot {
    const wallNS = parseRFC3339Nano(this.now())
    const monotonicNS = this.monotonicNow()
    if (wallNS === null || typeof monotonicNS !== 'bigint' || monotonicNS < 0n ||
      this.clockCompromised ||
      this.lastWallNS !== null && wallNS < this.lastWallNS ||
      this.lastMonotonicNS !== null && monotonicNS < this.lastMonotonicNS) {
      this.markAuthorityUnhealthy()
      throw new Error('controlled artifact V2 host clock is invalid')
    }
    this.lastWallNS = wallNS
    this.lastMonotonicNS = monotonicNS
    return { wallNS, monotonicNS }
  }

  private markAuthorityUnhealthy(): void {
    this.clockCompromised = true
    this.revokeTransport()
    for (const entry of this.entries.values()) {
      if (entry.state === 'ready') entry.state = 'revoked'
    }
  }

  private rendererIsCurrent(webContentsId: number, rendererGeneration: number): boolean {
    const lease = this.acquireRendererLease(webContentsId, rendererGeneration)
    if (!lease) return false
    try {
      return rendererLeaseIsCurrent(lease)
    } finally {
      if (!releaseRendererLease(lease)) this.markAuthorityUnhealthy()
    }
  }

  private trackReleaseOperation<T>(operation: Promise<T>): Promise<T> {
    this.releaseOperations.add(operation)
    void operation.finally(() => this.releaseOperations.delete(operation)).catch(() => undefined)
    return operation
  }

  private scheduleLatePreparedAbort(operation: Promise<ControlledArtifactPreparedReleaseV2>): void {
    const cleanup = operation.then(async (prepared) => {
      if (!prepared || typeof prepared.commitBefore !== 'function' || typeof prepared.abort !== 'function') {
        this.markAuthorityUnhealthy()
        return
      }
      try {
        await this.abortPreparedRelease(prepared)
      } catch {
        this.markAuthorityUnhealthy()
      }
    }, () => undefined)
    this.trackReleaseOperation(cleanup)
  }

  private async abortPreparedRelease(prepared: ControlledArtifactPreparedReleaseV2): Promise<void> {
    const abortSignal = AbortSignal.timeout(this.abortEffectTimeoutMs)
    const abortOperation = this.trackReleaseOperation(
      Promise.resolve().then(() => prepared.abort(abortSignal))
    )
    await awaitWithAbort(abortOperation, abortSignal)
  }

  private async waitForReleaseOperations(): Promise<void> {
    while (this.releaseOperations.size > 0) {
      await Promise.allSettled([...this.releaseOperations])
    }
  }

  private finishHandler(): void {
    this.activeHandlers -= 1
    if (this.activeHandlers !== 0) return
    const waiters = this.drainWaiters
    this.drainWaiters = []
    for (const resolve of waiters) resolve()
  }

  private waitForHandlers(): Promise<void> {
    if (this.activeHandlers === 0) return Promise.resolve()
    return new Promise((resolve) => this.drainWaiters.push(resolve))
  }
}

function parseCanonicalProbeRequest(body: Buffer): ControlledArtifactProbeRequestV2 {
  const parsed = parseStrictJsonObject(body, {
    maxBytes: MAX_JSON_BYTES, maxDepth: 2, maxTokens: 16, maxStringBytes: 256
  })
  if (!hasExactKeys(parsed, PROBE_REQUEST_KEYS)) {
    throw new Error('controlled artifact V2 probe request is invalid')
  }
  const canonical: ControlledArtifactProbeRequestV2 = {
    schemaVersion: parsed.schemaVersion as number,
    purpose: parsed.purpose as string,
    probeNonce: parsed.probeNonce as string,
    backendGeneration: parsed.backendGeneration as number
  }
  const nonce = decodeCanonicalBase64URL(canonical.probeNonce, 32)
  try {
    if (canonical.schemaVersion !== PROTOCOL_VERSION || canonical.purpose !== PROBE_REQUEST_PURPOSE ||
      nonce === null || !isSafePositive(canonical.backendGeneration) ||
      !body.equals(Buffer.from(goJSONStringify(canonical), 'utf8'))) {
      throw new Error('controlled artifact V2 probe request is not canonical')
    }
    return canonical
  } finally {
    nonce?.fill(0)
  }
}

function parseCanonicalAdmissionRequest(body: Buffer): ControlledArtifactAdmissionRequestV2 {
  const parsed = parseStrictJsonObject(body, {
    maxBytes: MAX_JSON_BYTES, maxDepth: 4, maxTokens: 128, maxStringBytes: 4096
  })
  if (!hasExactKeys(parsed, ADMISSION_REQUEST_KEYS) || !isObject(parsed.context) ||
    !hasExactKeys(parsed.context, CONTEXT_KEYS)) {
    throw new Error('controlled artifact V2 admission request is invalid')
  }
  const canonical = canonicalAdmissionRequest(parsed)
  if (parsed.schemaVersion !== PROTOCOL_VERSION || parsed.purpose !== ADMISSION_PURPOSE ||
    !isValidContext(canonical.context) ||
    canonical.accessAction !== 'display' && canonical.accessAction !== 'export' ||
    !OPAQUE_PATTERN.test(canonical.controlledHandle) || !OPAQUE_PATTERN.test(canonical.useSlot) ||
    !OPAQUE_PATTERN.test(canonical.rendererPrincipal) ||
    !isSafePositive(canonical.rendererGeneration) || !isSafePositive(canonical.backendGeneration) ||
    !body.equals(Buffer.from(goJSONStringify(canonical), 'utf8'))) {
    throw new Error('controlled artifact V2 admission request is not canonical')
  }
  return canonical
}

function canonicalAdmissionRequest(value: Record<string, unknown>): ControlledArtifactAdmissionRequestV2 {
  const context = value.context as Record<string, unknown>
  return {
    schemaVersion: value.schemaVersion as number,
    purpose: value.purpose as string,
    context: canonicalContext(context),
    accessAction: value.accessAction as ControlledArtifactAccessActionV2,
    controlledHandle: value.controlledHandle as string,
    useSlot: value.useSlot as string,
    rendererPrincipal: value.rendererPrincipal as string,
    rendererGeneration: value.rendererGeneration as number,
    backendGeneration: value.backendGeneration as number
  }
}

function parseReleaseFrame(frame: Buffer): { metadata: Buffer; body: Buffer } {
  if (frame.length < RELEASE_MAGIC.length + 4 ||
    !frame.subarray(0, RELEASE_MAGIC.length).equals(RELEASE_MAGIC)) {
    throw new Error('controlled artifact V2 release frame is invalid')
  }
  const metadataLength = frame.readUInt32BE(RELEASE_MAGIC.length)
  const metadataStart = RELEASE_MAGIC.length + 4
  const bodyStart = metadataStart + metadataLength
  if (metadataLength < 1 || metadataLength > MAX_METADATA_BYTES || bodyStart >= frame.length ||
    frame.length - bodyStart > MAX_ARTIFACT_BYTES) {
    throw new Error('controlled artifact V2 release frame length is invalid')
  }
  return { metadata: frame.subarray(metadataStart, bodyStart), body: frame.subarray(bodyStart) }
}

function parseReleaseReceipt(metadataBody: Buffer): ControlledArtifactReleaseReceiptV2 {
  const metadata = parseStrictJsonObject(metadataBody, {
    maxBytes: MAX_METADATA_BYTES, maxDepth: 6, maxTokens: 512, maxStringBytes: 32 << 10
  })
  if (!hasExactKeys(metadata, RELEASE_METADATA_KEYS) || metadata.schemaVersion !== PROTOCOL_VERSION ||
    metadata.purpose !== RELEASE_PURPOSE || !isObject(metadata.receipt)) {
    throw new Error('controlled artifact V2 release metadata is invalid')
  }
  const rawReceipt = metadata.receipt
  if (!hasExactKeys(rawReceipt, RECEIPT_KEYS) || !isObject(rawReceipt.context) ||
    !hasExactKeys(rawReceipt.context, CONTEXT_KEYS)) {
    throw new Error('controlled artifact V2 receipt shape is invalid')
  }
  const receipt = canonicalReceipt(rawReceipt as unknown as ControlledArtifactReleaseReceiptV2,
    rawReceipt.authoritySignature as string, rawReceipt.recordDigest as string)
  const canonicalMetadata = { schemaVersion: PROTOCOL_VERSION, purpose: RELEASE_PURPOSE, receipt }
  if (!metadataBody.equals(Buffer.from(goJSONStringify(canonicalMetadata), 'utf8')) ||
    !validateReceipt(receipt)) {
    throw new Error('controlled artifact V2 receipt is invalid')
  }
  return receipt
}

function validateReceipt(receipt: ControlledArtifactReleaseReceiptV2): boolean {
  const context = receipt.context
  const requestedAt = parseRFC3339Nano(receipt.requestedAt)
  const authorizedUntil = parseRFC3339Nano(receipt.authorizedUntil)
  const contextIssuedAt = parseRFC3339Nano(context.contextIssuedAt)
  const digestFields = [
    receipt.accessId, receipt.controlledHandleDigest, receipt.useSlotDigest,
    receipt.rendererPrincipalDigest, receipt.accessPolicyDigest, receipt.retentionPolicyDigest,
    receipt.deliveryId, receipt.deliveryOutcomeRecordDigest, receipt.publicationCommitDigest,
    receipt.publicationReceiptDigest, receipt.piiProjectionDigest, receipt.piiAuthorizationDigest,
    receipt.claimLedgerDigest, receipt.targetIdentityDigest, receipt.releaseTargetIdentityDigest,
    receipt.artifactSha256,
    receipt.authorityKeyId, receipt.recordDigest, context.caseBindingHash,
    context.sourceManifestHash, context.contextDigest
  ]
  if (receipt.schemaVersion !== PROTOCOL_VERSION || receipt.purpose !== ACCESS_RECEIPT_PURPOSE ||
    receipt.accessAction !== 'display' && receipt.accessAction !== 'export' ||
    receipt.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE || receipt.authorityAlgorithm !== 'Ed25519' ||
    !isSafePositive(receipt.rendererGeneration) || !isSafePositive(receipt.backendGeneration) ||
    !isSafePositive(receipt.artifactByteLength) || receipt.artifactByteLength > MAX_ARTIFACT_BYTES ||
    !isValidContext(context) || receipt.requesterUserId !== context.userId ||
    receipt.releaseTargetIdentityDigest === receipt.targetIdentityDigest ||
    digestFields.some((value) => !SHA256_PATTERN.test(value)) ||
    receipt.accessId !== sha256(Buffer.concat([ACCESS_ID_DOMAIN, Buffer.from(receipt.useSlotDigest, 'utf8')])) ||
    !isBoundedString(receipt.requesterUserId) || !isBoundedString(receipt.authorityPublicKey) ||
    !isBoundedString(receipt.authoritySignature) || requestedAt === null || authorizedUntil === null ||
    contextIssuedAt === null || requestedAt < contextIssuedAt || requestedAt >= authorizedUntil ||
    authorizedUntil - requestedAt > MAX_ADMISSION_TTL_NS) {
    return false
  }
  return verifyReceiptAuthority(receipt)
}

function verifyReceiptAuthority(receipt: ControlledArtifactReleaseReceiptV2): boolean {
  try {
    const publicKey = decodeCanonicalBase64URL(receipt.authorityPublicKey, 32)
    const signature = decodeCanonicalBase64URL(receipt.authoritySignature, 64)
    if (!publicKey || !signature || sha256(publicKey) !== receipt.authorityKeyId) return false
    const signingRecord = canonicalReceipt(receipt, '', '')
    const signingDigest = createHash('sha256').update(goJSONStringify(signingRecord), 'utf8').digest()
    const signingBytes = Buffer.concat([RECEIPT_SIGNATURE_DOMAIN, signingDigest])
    const key = createPublicKey({
      key: Buffer.concat([ED25519_SPKI_PREFIX, publicKey]),
      format: 'der',
      type: 'spki'
    })
    if (!verify(null, signingBytes, key, signature)) return false
    const record = canonicalReceipt(receipt, receipt.authoritySignature, '')
    return sha256(Buffer.concat([
      RECEIPT_RECORD_DOMAIN,
      Buffer.from(goJSONStringify(record), 'utf8')
    ])) === receipt.recordDigest
  } catch {
    return false
  }
}

function canonicalReceipt(
  receipt: ControlledArtifactReleaseReceiptV2,
  authoritySignature: string,
  recordDigest: string
): ControlledArtifactReleaseReceiptV2 {
  return {
    schemaVersion: receipt.schemaVersion,
    purpose: receipt.purpose,
    accessId: receipt.accessId,
    context: canonicalContext(receipt.context),
    requesterUserId: receipt.requesterUserId,
    accessAction: receipt.accessAction,
    controlledHandleDigest: receipt.controlledHandleDigest,
    useSlotDigest: receipt.useSlotDigest,
    rendererPrincipalDigest: receipt.rendererPrincipalDigest,
    rendererGeneration: receipt.rendererGeneration,
    backendGeneration: receipt.backendGeneration,
    accessPolicyDigest: receipt.accessPolicyDigest,
    retentionPolicyDigest: receipt.retentionPolicyDigest,
    deliveryId: receipt.deliveryId,
    deliveryOutcomeRecordDigest: receipt.deliveryOutcomeRecordDigest,
    publicationCommitDigest: receipt.publicationCommitDigest,
    publicationReceiptDigest: receipt.publicationReceiptDigest,
    piiProjectionDigest: receipt.piiProjectionDigest,
    piiAuthorizationDigest: receipt.piiAuthorizationDigest,
    claimLedgerDigest: receipt.claimLedgerDigest,
    targetIdentityDigest: receipt.targetIdentityDigest,
    releaseTargetIdentityDigest: receipt.releaseTargetIdentityDigest,
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

function canonicalContext(value: Record<string, unknown> | ControlledArtifactContextBindingV2): ControlledArtifactContextBindingV2 {
  return {
    version: value.version as number,
    threadId: value.threadId as string,
    turnId: value.turnId as string,
    workspaceRealPath: value.workspaceRealPath as string,
    tenantId: value.tenantId as string,
    userId: value.userId as string,
    caseId: value.caseId as string,
    caseBindingHash: value.caseBindingHash as string,
    datasetSnapshotId: value.datasetSnapshotId as string,
    sourceManifestHash: value.sourceManifestHash as string,
    contextEpoch: value.contextEpoch as number,
    contextIssuedAt: value.contextIssuedAt as string,
    contextDigest: value.contextDigest as string
  }
}

function isValidInvocationInput(
  input: ControlledArtifactInvocationInputV2,
  now: bigint,
  certificateNotAfter: bigint
): boolean {
  if (!input || typeof input.prepareRelease !== 'function' || !Number.isSafeInteger(input.webContentsId) ||
    input.webContentsId <= 0 || !isSafePositive(input.rendererGeneration) ||
    input.action !== 'display' && input.action !== 'export' ||
    [input.accessPolicyDigest, input.retentionPolicyDigest, input.deliveryId,
      input.deliveryOutcomeRecordDigest, input.publicationCommitDigest, input.publicationReceiptDigest,
      input.piiProjectionDigest, input.piiAuthorizationDigest, input.claimLedgerDigest,
      input.targetIdentityDigest, input.releaseTargetIdentityDigest,
      input.artifactSha256].some((value) => !SHA256_PATTERN.test(value)) ||
    !isSafePositive(input.artifactByteLength) || input.artifactByteLength > MAX_ARTIFACT_BYTES ||
    input.mediaType !== CONTROLLED_ARTIFACT_MEDIA_TYPE ||
    !validAuthorityBinding(input.authorityKeyId, input.authorityPublicKey) ||
    !isValidInvocationContext(input.context)) {
    return false
  }
  const authorizedUntil = parseRFC3339Nano(input.authorizedUntil)
  return authorizedUntil !== null && authorizedUntil > now && authorizedUntil <= certificateNotAfter &&
    authorizedUntil - now <= MAX_ADMISSION_TTL_NS
}

function isValidContext(context: ControlledArtifactContextBindingV2): boolean {
  return Boolean(context) && context.version === 2 && isBoundedString(context.threadId) &&
    isBoundedString(context.turnId) && isBoundedString(context.workspaceRealPath) &&
    isBoundedString(context.tenantId) && isBoundedString(context.userId) &&
    isBoundedString(context.caseId) && context.caseId !== 'unbound' &&
    SHA256_PATTERN.test(context.caseBindingHash) && DATASET_PATTERN.test(context.datasetSnapshotId) &&
    SHA256_PATTERN.test(context.sourceManifestHash) && isSafePositive(context.contextEpoch) &&
    parseRFC3339Nano(context.contextIssuedAt) !== null && SHA256_PATTERN.test(context.contextDigest)
}

function isValidInvocationContext(context: ControlledArtifactInvocationContextV2): boolean {
  return Boolean(context) && hasExactKeys(context as unknown as Record<string, unknown>, [
    'threadId', 'turnId', 'contextEpoch', 'contextDigest'
  ]) && isBoundedString(context.threadId) && isBoundedString(context.turnId) &&
    isSafePositive(context.contextEpoch) && SHA256_PATTERN.test(context.contextDigest)
}

function sameInvocationContext(
  expected: ControlledArtifactInvocationContextV2,
  actual: ControlledArtifactContextBindingV2
): boolean {
  return expected.threadId === actual.threadId && expected.turnId === actual.turnId &&
    expected.contextEpoch === actual.contextEpoch && expected.contextDigest === actual.contextDigest
}

function parseRFC3339Nano(value: unknown): bigint | null {
  if (typeof value !== 'string') return null
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):([0-5]\d):([0-5]\d)(?:\.(\d{0,8}[1-9]))?Z$/.exec(value)
  if (!match) return null
  const wholeSeconds = `${match[1]}-${match[2]}-${match[3]}T${match[4]}:${match[5]}:${match[6]}Z`
  const milliseconds = Date.parse(wholeSeconds)
  if (!Number.isFinite(milliseconds) ||
    new Date(milliseconds).toISOString().replace('.000Z', 'Z') !== wholeSeconds) {
    return null
  }
  const fraction = (match[7] ?? '').padEnd(9, '0')
  return BigInt(Math.trunc(milliseconds / 1_000)) * 1_000_000_000n + BigInt(fraction || '0')
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
  if (encoded === undefined) throw new Error('controlled artifact V2 value is not serializable')
  return encoded
    .replace(/</g, '\\u003c')
    .replace(/>/g, '\\u003e')
    .replace(/&/g, '\\u0026')
    .replace(/\u2028/g, '\\u2028')
    .replace(/\u2029/g, '\\u2029')
}

function releaseAckMACMessage(ack: {
  schemaVersion: number
  purpose: string
  committed: boolean
  accessId: string
  accessReceiptDigest: string
  contextDigest: string
  deliveryId: string
  deliveryOutcomeRecordDigest: string
  publicationCommitDigest: string
  releaseTargetIdentityDigest: string
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
    String(ack.schemaVersion), ack.purpose, String(ack.committed), ack.accessId,
    ack.accessReceiptDigest, ack.contextDigest, ack.deliveryId, ack.deliveryOutcomeRecordDigest,
    ack.publicationCommitDigest, ack.releaseTargetIdentityDigest, ack.artifactSha256,
    String(ack.releasedByteLength),
    ack.accessAction, ack.controlledHandleDigest, ack.useSlotDigest, ack.rendererPrincipalDigest,
    String(ack.rendererGeneration), String(ack.backendGeneration), ack.committedAt
  ]
  const parts: Buffer[] = [RELEASE_ACK_DOMAIN]
  for (const value of values) {
    parts.push(Buffer.from(`${Buffer.byteLength(value, 'utf8')}:`, 'ascii'), Buffer.from(value, 'utf8'))
  }
  return Buffer.concat(parts)
}

function bearerMatches(header: string, secret: Buffer): boolean {
  if (!header.startsWith('Bearer ') || secret.length !== SECRET_BYTES) return false
  const candidate = decodeCanonicalBase64URL(header.slice('Bearer '.length), SECRET_BYTES)
  if (!candidate) return false
  try {
    return timingSafeEqual(candidate, secret)
  } finally {
    candidate.fill(0)
  }
}

function rendererLeaseSignal(lease: ControlledArtifactRendererLeaseV2): AbortSignal | null {
  try {
    return lease.signal instanceof AbortSignal ? lease.signal : null
  } catch {
    return null
  }
}

function rendererLeaseIsCurrent(
  lease: ControlledArtifactRendererLeaseV2,
  signal: AbortSignal = rendererLeaseSignal(lease) ?? AbortSignal.abort()
): boolean {
  try {
    return !signal.aborted && lease.isCurrent()
  } catch {
    return false
  }
}

function releaseRendererLease(lease: ControlledArtifactRendererLeaseV2): boolean {
  try {
    lease.release()
    return true
  } catch {
    return false
  }
}

function awaitWithAbort<T>(operation: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(new Error('controlled artifact V2 operation timed out'))
  return new Promise<T>((resolve, reject) => {
    let settled = false
    const settle = (complete: () => void): void => {
      if (settled) return
      settled = true
      signal.removeEventListener('abort', onAbort)
      complete()
    }
    const onAbort = (): void => settle(() => {
      reject(new Error('controlled artifact V2 operation timed out'))
    })
    signal.addEventListener('abort', onAbort, { once: true })
    operation.then(
      (value) => settle(() => resolve(value)),
      (error) => settle(() => reject(error))
    )
  })
}

function isControlledArtifactCommitReceiptV2(value: unknown): value is ControlledArtifactCommitReceiptV2 {
  if (!isObject(value)) return false
  return SHA256_PATTERN.test(String(value.releaseTargetIdentityDigest ?? '')) &&
    SHA256_PATTERN.test(String(value.artifactSha256 ?? '')) &&
    isSafePositive(value.artifactByteLength) && value.artifactByteLength <= MAX_ARTIFACT_BYTES &&
    value.mediaType === CONTROLLED_ARTIFACT_MEDIA_TYPE && parseRFC3339Nano(value.committedAt) !== null &&
    hasExactKeys(value, [
      'releaseTargetIdentityDigest', 'artifactSha256', 'artifactByteLength', 'mediaType', 'committedAt'
    ])
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

function isBoundedString(value: unknown): value is string {
  return typeof value === 'string' && value.trim() === value && value.length > 0 &&
    Buffer.byteLength(value, 'utf8') <= 2048
}

function opaqueDigest(domain: Buffer, token: string): string {
  return createHash('sha256').update(domain).update(token, 'utf8').digest('hex')
}

function sha256(body: Buffer): string {
  return createHash('sha256').update(body).digest('hex')
}

function safeEqual(left: unknown, right: string): boolean {
  if (typeof left !== 'string') return false
  const leftBody = Buffer.from(left, 'utf8')
  const rightBody = Buffer.from(right, 'utf8')
  try {
    return leftBody.length === rightBody.length && timingSafeEqual(leftBody, rightBody)
  } finally {
    leftBody.fill(0)
    rightBody.fill(0)
  }
}

function singleHeader(request: IncomingMessage, target: string): string {
  const values: string[] = []
  for (let index = 0; index < request.rawHeaders.length; index += 2) {
    if (request.rawHeaders[index]?.toLowerCase() === target) values.push(request.rawHeaders[index + 1] ?? '')
  }
  return values.length === 1 ? values[0] : ''
}

function hasEncodedBody(request: IncomingMessage): boolean {
  return singleHeader(request, 'transfer-encoding') !== '' || singleHeader(request, 'content-encoding') !== ''
}

async function readRequestBodySingleAllocation(request: IncomingMessage, maxBytes: number): Promise<Buffer | null> {
  const contentLength = singleHeader(request, 'content-length')
  if (!/^[1-9][0-9]*$/.test(contentLength)) return null
  const expected = Number(contentLength)
  if (!Number.isSafeInteger(expected) || expected <= 0 || expected > maxBytes) return null
  const body = Buffer.allocUnsafe(expected)
  let offset = 0
  try {
    for await (const chunk of request) {
      const source = Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk)
      try {
        if (offset + source.length > expected) {
          body.fill(0)
          return null
        }
        source.copy(body, offset)
        offset += source.length
      } finally {
        source.fill(0)
      }
    }
  } catch {
    body.fill(0)
    return null
  }
  if (offset !== expected) {
    body.fill(0)
    return null
  }
  return body
}

function writeJson(response: ServerResponse, value: unknown): void {
  if (response.headersSent || response.destroyed) return
  const body = Buffer.from(goJSONStringify(value), 'utf8')
  response.statusCode = httpStatus.ok
  response.setHeader('Content-Type', JSON_MEDIA_TYPE)
  response.setHeader('Content-Length', String(body.length))
  response.end(body, () => body.fill(0))
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

function formatRFC3339Nano(value: Date): string {
  const iso = value.toISOString()
  return iso.replace(/\.000Z$/, 'Z').replace(/(\.\d*?[1-9])0+Z$/, '$1Z')
}

async function listenExactLoopback(server: Server): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error): void => reject(error)
    server.once('error', onError)
    server.listen(0, HOST, () => {
      server.off('error', onError)
      resolve()
    })
  })
}

async function closeFailedServer(server: Server): Promise<void> {
  await new Promise<void>((resolve) => {
    if (!server.listening) {
      server.closeAllConnections?.()
      resolve()
      return
    }
    server.close(() => resolve())
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
