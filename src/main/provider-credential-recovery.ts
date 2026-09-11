import { app, BrowserWindow, dialog, type BrowserWindow as BrowserWindowType } from 'electron'
import { constants as fsConstants } from 'node:fs'
import { createHash, randomUUID } from 'node:crypto'
import { chmod as fsChmod, lstat as fsLstat, open as fsOpen, rename as fsRename, unlink as fsUnlink } from 'node:fs/promises'
import { dirname, isAbsolute, resolve } from 'node:path'
import { parseProviderRegistryPortableManifestV1 } from '../../packages/runtime/src/contracts/provider-registry.js'
import { parseStrictJsonValue } from './controlled-artifact/strict-json'
import {
  type ProtectedRecoveryImportReceipt,
  type ProtectedRecoveryLocalAction,
  type ProtectedRecoveryLocalBinding,
  type ProtectedRecoveryOwnerBinding,
  type ProtectedRecoveryRuntimeCall,
  type ProtectedRecoveryRuntimeResult
} from './ipc/provider-registry-ipc'

const MAX_ARTIFACT_BYTES = 1 << 20
const MAX_IMPORT_RECEIPT_BYTES = 1 << 20
const ALLOWED_PURPOSES = new Set([
  'provider-api-key',
  'provider-oauth-token-bundle',
  'mcp-oauth-access-token',
  'extension-provider-account-token'
])

type MainFrameLike = {
  routingId?: number
  url?: string
}

type MainWebContentsLike = {
  id: number
  isDestroyed(): boolean
  mainFrame?: MainFrameLike | null
  getURL?: () => string
}

type MainWindowLike = {
  id: number
  isDestroyed(): boolean
  webContents: MainWebContentsLike
}

export type ProtectedRecoveryInvokeEvent = {
  sender: MainWebContentsLike
  senderFrame?: MainFrameLike | null
}

type ProtectedRecoveryProfile = {
  profileBinding: string
  dataDirectory: string
}

type ProtectedRecoveryArtifactRole = 'request' | 'bundle' | 'receipt'

export type MainProviderCredentialRecoveryResult = {
  ok: true
  status: 'request-created' | 'bundle-created' | 'receipt-created' | 'finalized'
  entryCount?: number
} | {
  ok: false
  code:
    | 'invalid_context'
    | 'invalid_path'
    | 'cancelled'
    | 'invalid_request'
    | 'invalid_response'
    | 'runtime_unavailable'
    | 'conflict'
    | 'verification_failure'
}

export type MainProviderCredentialRecoveryAuthority = {
  createDestinationRequest: (event: ProtectedRecoveryInvokeEvent) => Promise<MainProviderCredentialRecoveryResult>
  createSourceBundle: (event: ProtectedRecoveryInvokeEvent) => Promise<MainProviderCredentialRecoveryResult>
  applyDestinationBundle: (event: ProtectedRecoveryInvokeEvent) => Promise<MainProviderCredentialRecoveryResult>
  finalizeSourceReceipt: (event: ProtectedRecoveryInvokeEvent) => Promise<MainProviderCredentialRecoveryResult>
}

type StatLike = {
  isFile(): boolean
  isSymbolicLink(): boolean
  isDirectory(): boolean
  isBlockDevice?: () => boolean
  isCharacterDevice?: () => boolean
  isFIFO?: () => boolean
  isSocket?: () => boolean
}

export type MainProviderCredentialRecoveryFactoryOptions = {
  showOpenDialog?: (window: MainWindowLike, options?: Record<string, unknown>) => Promise<{
    canceled: boolean
    filePaths: string[]
  }>
  showSaveDialog?: (window: MainWindowLike, options?: Record<string, unknown>) => Promise<{
    canceled: boolean
    filePath?: string
  }>
  confirmNative?: (confirmation: Record<string, unknown>) => Promise<boolean>
  lstat?: (path: string) => Promise<StatLike>
  readFile?: (path: string) => Promise<Buffer>
  writeFile?: (path: string, bytes: Uint8Array, options?: { mode?: number; flag?: string }) => Promise<void>
  /** Main-only operation-specific client. It must not be backed by renderer runtimeRequest. */
  protectedRuntimeRequest?: (call: ProtectedRecoveryRuntimeCall) => Promise<ProtectedRecoveryRuntimeResult>
  /** Kept only for the negative assertion that protected flow does not use the generic client. */
  runtimeRequest?: (path: string, method?: string, body?: string) => unknown
  getCurrentWindow?: (event: ProtectedRecoveryInvokeEvent) => MainWindowLike | null
  getCurrentMainFrame?: (event: ProtectedRecoveryInvokeEvent) => MainFrameLike | null
  getCurrentProfile?: () => ProtectedRecoveryProfile | null | Promise<ProtectedRecoveryProfile | null>
  getPortableImportReceipt?: () => ProtectedRecoveryImportReceipt | null | Promise<ProtectedRecoveryImportReceipt | null>
  /** Main-owned current export; renderer/import results never supply source authority. */
  getCurrentPortableManifest?: () => string | Uint8Array | null | Promise<string | Uint8Array | null>
  getCurrentOwnerBindingInventory?: () => ProtectedRecoveryOwnerBinding[] | Promise<ProtectedRecoveryOwnerBinding[]>
  resolveMcpBindingFingerprint?: () => string | null | Promise<string | null>
  resolveExtensionBindingFingerprint?: () => string | null | Promise<string | null>
  /**
   * Tests and the Main artifact coordinator may reserve a native target before
   * the protected prepare call. The selected path is still rechecked by the
   * single writer seam before bytes are written.
   */
  getSaveTarget?: (
    role: ProtectedRecoveryArtifactRole
  ) => string | null | Promise<string | null>
}

type PendingRequest = {
  bytes: Buffer
  requestText: string
  requestDigest: string
  requestFingerprint: string
  manifestDigest: string
  itemSetDigest: string
  operationId: string
  sessionNonce: string
  expiresAt: string
  localBinding: ProtectedRecoveryLocalBinding
  importReceipt: ProtectedRecoveryImportReceipt
}

type PendingRequestMetadata = {
  requestDigest: string
  requestFingerprint: string
  manifestDigest: string
  itemSetDigest: string
  operationId: string
  sessionNonce: string
  expiresAt: string
}

function resultForRuntimeFailure(result: ProtectedRecoveryRuntimeResult): MainProviderCredentialRecoveryResult {
  if (result.ok) return { ok: false, code: 'invalid_response' }
  return { ok: false, code: result.code }
}

function asBuffer(value: Uint8Array): Buffer {
  if (Buffer.isBuffer(value)) return value
  return Buffer.from(value.buffer, value.byteOffset, value.byteLength)
}

function zeroize(value: Uint8Array | null | undefined): void {
  if (!value) return
  try {
    value.fill(0)
  } catch {
    // Best-effort zeroization cannot be treated as physical erasure.
  }
}

function isRegularArtifactStats(stats: {
  isFile(): boolean
  isSymbolicLink(): boolean
  isDirectory(): boolean
  isBlockDevice?: () => boolean
  isCharacterDevice?: () => boolean
  isFIFO?: () => boolean
  isSocket?: () => boolean
}): boolean {
  return stats.isFile() && !stats.isSymbolicLink() && !stats.isDirectory() &&
    !stats.isBlockDevice?.() && !stats.isCharacterDevice?.() &&
    !stats.isFIFO?.() && !stats.isSocket?.()
}

function sameOpenedArtifact(stats: { dev?: unknown; ino?: unknown; size?: number }, pathStats: { dev?: unknown; ino?: unknown; size?: number }): boolean {
  return String(stats.dev ?? '') === String(pathStats.dev ?? '') &&
    String(stats.ino ?? '') === String(pathStats.ino ?? '') && stats.size === pathStats.size
}

async function secureReadArtifactFile(path: string, maximumBytes: number): Promise<Buffer> {
  const flags = fsConstants.O_RDONLY | (fsConstants.O_NONBLOCK ?? 0) | (fsConstants.O_NOFOLLOW ?? 0)
  const handle = await fsOpen(path, flags)
  let body: Buffer | null = null
  try {
    const before = await handle.stat()
    if (!isRegularArtifactStats(before) || !Number.isSafeInteger(before.size) || before.size < 0 || before.size > maximumBytes) {
      throw new Error('invalid_artifact_file')
    }
    body = Buffer.allocUnsafe(before.size)
    let offset = 0
    while (offset < body.length) {
      const readResult = await handle.read(body, offset, body.length - offset, null)
      if (readResult.bytesRead <= 0) throw new Error('short_artifact_file')
      offset += readResult.bytesRead
    }
    const overflow = Buffer.alloc(1)
    try {
      const overflowResult = await handle.read(overflow, 0, 1, null)
      if (overflowResult.bytesRead !== 0) throw new Error('oversized_artifact_file')
    } finally {
      overflow.fill(0)
    }
    const after = await handle.stat()
    const pathAfter = await fsLstat(path)
    if (!isRegularArtifactStats(after) || !isRegularArtifactStats(pathAfter) ||
      !sameOpenedArtifact(before, after) || !sameOpenedArtifact(before, pathAfter)) {
      throw new Error('changed_artifact_file')
    }
    const result = body
    body = null
    return result
  } finally {
    body?.fill(0)
    await handle.close().catch(() => undefined)
  }
}

async function secureWriteArtifactFile(path: string, bytes: Uint8Array): Promise<void> {
  const parent = dirname(path)
  const parentStats = await fsLstat(parent)
  if (!parentStats.isDirectory() || parentStats.isSymbolicLink()) throw new Error('invalid_artifact_parent')
  try {
    const existing = await fsLstat(path)
    if (!isRegularArtifactStats(existing)) throw new Error('invalid_artifact_target')
  } catch (error) {
    if (!isNotFoundError(error)) throw error
  }
  const temporaryPath = `${path}.${process.pid}.${randomUUID()}.tmp`
  const flags = fsConstants.O_WRONLY | fsConstants.O_CREAT | fsConstants.O_EXCL | (fsConstants.O_NOFOLLOW ?? 0)
  let handle: Awaited<ReturnType<typeof fsOpen>> | null = null
  let renamed = false
  try {
    handle = await fsOpen(temporaryPath, flags, 0o600)
    let offset = 0
    while (offset < bytes.byteLength) {
      const writeResult = await handle.write(bytes, offset, bytes.byteLength - offset, null)
      if (writeResult.bytesWritten <= 0) throw new Error('short_artifact_write')
      offset += writeResult.bytesWritten
    }
    await handle.sync()
    await handle.chmod(0o600)
    await handle.close()
    handle = null
    await fsRename(temporaryPath, path)
    renamed = true
    const finalStats = await fsLstat(path)
    if (!isRegularArtifactStats(finalStats)) throw new Error('invalid_artifact_target_after_write')
  } finally {
    await handle?.close().catch(() => undefined)
    if (!renamed) await fsUnlink(temporaryPath).catch(() => undefined)
  }
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : null
}

function hasForbiddenArtifactKey(value: unknown): boolean {
  if (Array.isArray(value)) return value.some(hasForbiddenArtifactKey)
  const object = objectValue(value)
  if (!object) return false
  return Object.entries(object).some(([key, child]) =>
    /^(?:credentialRef|valueBase64|secret|privateKey|privateKeyRef|profileBinding|dataDirectory|dataDirectoryBinding|windowId|frameId|filePath|fileHandle|trust|permissions|sourceProviderId|sourceAccountId)$/i.test(key) ||
    hasForbiddenArtifactKey(child)
  )
}

function canonicalJSON(value: unknown): string {
  // Protected artifact canonicalization follows the Go domain encoder, which
  // deliberately leaves ordinary metadata scalars unescaped.
  return JSON.stringify(value)
}

function validArtifactDigest(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{64}$/.test(value)
}

function validArtifactFingerprint(value: unknown): value is string {
  return typeof value === 'string' && /^[a-f0-9]{16}$/.test(value)
}

function validArtifactToken(value: unknown, minimum: number, maximum: number): value is string {
  return typeof value === 'string' && value.length >= minimum && value.length <= maximum &&
    value === value.trim() && !/[\\\u0000\r\n\t]/.test(value) &&
    !value.includes('/') && !value.includes('..') &&
    Buffer.byteLength(value, 'utf8') <= maximum
}

function validArtifactExpiry(value: unknown): value is string {
  return typeof value === 'string' && value.length <= 64 &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
}

function validPositiveCanonicalCounter(value: unknown): value is string | number {
  if (typeof value === 'number') return Number.isSafeInteger(value) && value > 0
  return typeof value === 'string' && /^[1-9][0-9]{0,19}$/.test(value)
}

function validImportFence(value: unknown): value is NonNullable<ProtectedRecoveryImportReceipt['entries'][number]['fence']> {
  const fence = objectValue(value)
  if (!fence || Object.keys(fence).length !== 3) return false
  return typeof fence.revision === 'string' && /^[1-9][0-9]{0,19}$/.test(fence.revision) &&
    typeof fence.generation === 'string' && /^[1-9][0-9]{0,19}$/.test(fence.generation) &&
    typeof fence.incarnation === 'string' && /^inc_[A-Za-z0-9_-]{43}$/.test(fence.incarnation)
}

function validImportReceiptOwnerBinding(
  value: unknown,
  descriptor: { correlation: string; owner: string; provider: string; purpose: string }
): value is ProtectedRecoveryOwnerBinding {
  const binding = objectValue(value)
  if (!binding || Object.keys(binding).length !== 7 || !Object.keys(binding).every((key) => new Set([
    'correlation', 'owner', 'provider', 'accountId', 'channelId', 'purpose', 'fingerprint'
  ]).has(key))) return false
  return binding.correlation === descriptor.correlation && binding.owner === descriptor.owner &&
    binding.provider === descriptor.provider && binding.purpose === descriptor.purpose &&
    (binding.owner === 'mcp' || binding.owner === 'extension') &&
    typeof binding.accountId === 'string' && binding.accountId.length > 0 &&
    typeof binding.channelId === 'string' && binding.channelId.length > 0 &&
    typeof binding.fingerprint === 'string' && binding.fingerprint.length > 0 &&
    ((binding.owner === 'mcp' && binding.purpose === 'mcp-oauth-access-token') ||
      (binding.owner === 'extension' && binding.purpose === 'extension-provider-account-token'))
}

function validArtifactBase64(value: unknown, expectedBytes?: number): value is string {
  if (typeof value !== 'string' || value.length === 0 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) return false
  const decoded = Buffer.from(value, 'base64')
  try {
    return decoded.length > 0 && decoded.length <= MAX_ARTIFACT_BYTES &&
      (expectedBytes === undefined || decoded.length === expectedBytes) &&
      decoded.toString('base64') === value
  } finally {
    decoded.fill(0)
  }
}

function validArtifact(value: Uint8Array, role: ProtectedRecoveryArtifactRole): boolean {
  if (value.length === 0 || value.length > MAX_ARTIFACT_BYTES) return false
  const text = Buffer.from(value).toString('utf8')
  if (Buffer.byteLength(text, 'utf8') !== value.length || text.includes('\u0000')) return false
  let parsed: unknown
  try {
    parsed = parseStrictJsonValue(Buffer.from(value), {
      maxBytes: MAX_ARTIFACT_BYTES,
      maxDepth: 16,
      maxTokens: 32768,
      maxStringBytes: 65536,
      maxNumberBytes: 64
    })
  } catch {
    return false
  }
  const object = objectValue(parsed)
  const expectedSchema = {
    request: 'analytix.provider-protected-recovery-request/v1',
    bundle: 'analytix.provider-protected-recovery-bundle/v1',
    receipt: 'analytix.provider-protected-recovery-receipt/v1'
  }[role]
  if (!object || object.schema !== expectedSchema) {
    return false
  }
  if (hasForbiddenArtifactKey(parsed) || canonicalJSON(parsed) !== text) return false
  const allowedByRole: Record<ProtectedRecoveryArtifactRole, Set<string>> = {
    request: new Set([
      'schema', 'protocolVersion', 'manifestDigest', 'operationId', 'sessionNonce', 'expiresAt',
      'destinationEphemeralPublicKey', 'verificationFingerprint', 'entries'
    ]),
    bundle: new Set([
      'schema', 'protocolVersion', 'requestDigest', 'manifestDigest', 'operationId', 'sessionNonce',
      'expiresAt', 'sourceEphemeralPublicKey', 'nonce', 'ciphertext', 'entries'
    ]),
    receipt: new Set([
      'schema', 'protocolVersion', 'requestDigest', 'manifestDigest', 'bundleDigest', 'operationId',
      'sessionNonce', 'expiresAt', 'entries', 'authenticationTag'
    ])
  }
  const keys = Object.keys(object)
  if (keys.length !== allowedByRole[role].size || !keys.every((key) => allowedByRole[role].has(key))) return false
  if (object.protocolVersion !== 1 || !validArtifactDigest(object.manifestDigest) ||
    !validArtifactToken(object.operationId, 8, 128) || !validArtifactToken(object.sessionNonce, 16, 128) ||
    !validArtifactExpiry(object.expiresAt)) return false

  const entries = object.entries
  if (!Array.isArray(entries) || entries.length === 0 || entries.length > 128) return false
  if (role === 'request') {
    if (!validArtifactBase64(object.destinationEphemeralPublicKey, 32) ||
      !validArtifactFingerprint(object.verificationFingerprint)) return false
    return entries.every((entry) => {
      const item = objectValue(entry)
      if (!item || Object.keys(item).length !== 5 ||
        !Object.keys(item).every((key) => new Set([
          'correlation', 'destinationProviderId', 'destinationProviderRevision',
          'destinationProviderGeneration', 'destinationProviderIncarnation'
        ]).has(key))) return false
      return typeof item.correlation === 'string' && /^(?:provider|account)-[0-9]+$/.test(item.correlation) &&
        typeof item.destinationProviderId === 'string' && /^[a-z0-9][a-z0-9._-]{0,95}$/.test(item.destinationProviderId) &&
        typeof item.destinationProviderRevision === 'number' && Number.isSafeInteger(item.destinationProviderRevision) && item.destinationProviderRevision > 0 &&
        typeof item.destinationProviderGeneration === 'number' && Number.isSafeInteger(item.destinationProviderGeneration) && item.destinationProviderGeneration > 0 &&
        typeof item.destinationProviderIncarnation === 'string' && /^inc_[A-Za-z0-9_-]{43}$/.test(item.destinationProviderIncarnation)
    })
  }
  if (role === 'bundle') {
    return validArtifactDigest(object.requestDigest) && validArtifactBase64(object.sourceEphemeralPublicKey, 32) &&
      validArtifactBase64(object.nonce, 12) && validArtifactBase64(object.ciphertext) &&
      entries.every((entry) => {
        const item = objectValue(entry)
        if (!item || Object.keys(item).length !== 3 ||
          !Object.keys(item).every((key) => new Set(['correlation', 'destinationProviderId', 'purpose']).has(key))) return false
        return typeof item.correlation === 'string' && /^(?:provider|account)-[0-9]+$/.test(item.correlation) &&
          typeof item.destinationProviderId === 'string' && /^[a-z0-9][a-z0-9._-]{0,95}$/.test(item.destinationProviderId) &&
          typeof item.purpose === 'string' && ALLOWED_PURPOSES.has(item.purpose)
      })
  }
  return validArtifactDigest(object.requestDigest) && validArtifactDigest(object.bundleDigest) &&
    validArtifactBase64(object.authenticationTag, 32) && entries.every((entry) => {
      const item = objectValue(entry)
      if (!item || Object.keys(item).length !== 3 ||
        !Object.keys(item).every((key) => new Set(['correlation', 'destinationProviderId', 'status']).has(key))) return false
      return typeof item.correlation === 'string' && /^(?:provider|account)-[0-9]+$/.test(item.correlation) &&
        typeof item.destinationProviderId === 'string' && /^[a-z0-9][a-z0-9._-]{0,95}$/.test(item.destinationProviderId) &&
        (item.status === 'applied' || item.status === 'reentry_required')
    })
}

function validImportReceipt(value: ProtectedRecoveryImportReceipt): boolean {
  if (!value || typeof value.manifestJson !== 'string' ||
    Buffer.byteLength(value.manifestJson, 'utf8') === 0 ||
    Buffer.byteLength(value.manifestJson, 'utf8') > MAX_IMPORT_RECEIPT_BYTES ||
    !Array.isArray(value.entries) || value.entries.length === 0 || value.entries.length > 128) return false
  let manifest: unknown
  try {
    manifest = JSON.parse(value.manifestJson)
  } catch {
    return false
  }
  const manifestObject = objectValue(manifest)
  const parsedManifest = parseProviderRegistryPortableManifestV1(value.manifestJson)
  if (!manifestObject || manifestObject.schema !== 'analytix.provider-portable-manifest/v1' ||
    !Array.isArray(manifestObject.providers) || !Array.isArray(manifestObject.accounts) ||
    parsedManifest === null ||
    hasForbiddenArtifactKey(manifest) || canonicalJSON(manifest) !== value.manifestJson) return false
  if (!Object.keys(manifestObject).every((key) => new Set(['schema', 'providers', 'accounts']).has(key))) return false
  const providerKeys = new Set([
    'correlation', 'kind', 'endpoint', 'proxy', 'models', 'mediaModels',
    'selectedModel', 'selectedMediaModel', 'oauthBinding', 'accountObservation', 'routes', 'intent'
  ])
  const accountKeys = new Set(['correlation', 'owner', 'provider', 'endpoint', 'purpose', 'intent'])
  if (!manifestObject.providers.every((entry) => {
    const object = objectValue(entry)
    if (!object) return false
    return Object.keys(object).every((key) => providerKeys.has(key))
  }) || !manifestObject.accounts.every((entry) => {
    const object = objectValue(entry)
    if (!object) return false
    return Object.keys(object).every((key) => accountKeys.has(key))
  })) return false
  const seen = new Set<string>()
  for (const [index, entry] of value.entries.entries()) {
    const entryObject = objectValue(entry)
    const accountIndex = index - manifestObject.providers.length
    const isProvider = index < manifestObject.providers.length
    const allowedKeys = new Set([
      'correlation', 'destinationProviderId', 'status', 'fence',
      ...(isProvider ? [] : ['destinationOwnerBinding'])
    ])
    if (!entryObject || !Object.keys(entryObject).every((key) => allowedKeys.has(key)) ||
      Object.keys(entryObject).length !== allowedKeys.size || typeof entry.correlation !== 'string' ||
      typeof entry.destinationProviderId !== 'string' || entry.status !== 'reentry_required' ||
      seen.has(entry.correlation)) return false
    const expected = isProvider
      ? `provider-${index}`
      : `account-${accountIndex}`
    if (entry.correlation !== expected || !entry.destinationProviderId) return false
    if (!validImportFence(entry.fence)) return false
    if (isProvider) {
      if (entry.destinationOwnerBinding !== undefined) return false
    } else {
      const descriptor = parsedManifest.accounts[accountIndex]
      if (!descriptor || !validImportReceiptOwnerBinding(entry.destinationOwnerBinding, descriptor)) return false
    }
    seen.add(entry.correlation)
  }
  return value.entries.length === manifestObject.providers.length + manifestObject.accounts.length
}

function importReceiptsEqual(left: ProtectedRecoveryImportReceipt, right: ProtectedRecoveryImportReceipt): boolean {
  return canonicalJSON(left) === canonicalJSON(right)
}

function importReceiptMatchesRequest(
  receipt: ProtectedRecoveryImportReceipt,
  requestBytes: Uint8Array
): boolean {
  try {
    const request = objectValue(JSON.parse(Buffer.from(requestBytes).toString('utf8')))
    const requestEntries = request && Array.isArray(request.entries) ? request.entries : null
    if (!requestEntries || requestEntries.length !== receipt.entries.length) return false
    return receipt.entries.every((receiptEntry, index) => {
      const requestEntry = objectValue(requestEntries[index])
      if (!requestEntry || receiptEntry.correlation !== requestEntry.correlation ||
        receiptEntry.destinationProviderId !== requestEntry.destinationProviderId) return false
      const fence = receiptEntry.fence
      return validPositiveCanonicalCounter(requestEntry.destinationProviderRevision) &&
        validPositiveCanonicalCounter(requestEntry.destinationProviderGeneration) &&
        String(fence.revision) === String(requestEntry.destinationProviderRevision) &&
        String(fence.generation) === String(requestEntry.destinationProviderGeneration) &&
        fence.incarnation === requestEntry.destinationProviderIncarnation
    })
  } catch {
    return false
  }
}

function digestRequestPayload(parsed: Record<string, unknown>): string {
  const withoutFingerprint = { ...parsed }
  delete withoutFingerprint.verificationFingerprint
  return createHash('sha256').update(canonicalJSON(withoutFingerprint), 'utf8').digest('hex')
}

function metadataFromRequest(bytes: Uint8Array): PendingRequestMetadata | null {
  try {
    const parsed = objectValue(JSON.parse(Buffer.from(bytes).toString('utf8')))
    if (!parsed) return null
    const entries = Array.isArray(parsed.entries) ? parsed.entries : []
    const requestDigest = typeof parsed.requestDigest === 'string'
      ? parsed.requestDigest
      : digestRequestPayload(parsed)
    const manifestDigest = typeof parsed.manifestDigest === 'string' ? parsed.manifestDigest : ''
    const operationId = typeof parsed.operationId === 'string' ? parsed.operationId : ''
    const sessionNonce = typeof parsed.sessionNonce === 'string' ? parsed.sessionNonce : ''
    const expiresAt = typeof parsed.expiresAt === 'string' ? parsed.expiresAt : ''
    const requestFingerprint = typeof parsed.verificationFingerprint === 'string'
      ? parsed.verificationFingerprint
      : createHash('sha256').update(requestDigest).digest('hex').slice(0, 16)
    const itemSetDigest = createHash('sha256').update(canonicalJSON(entries), 'utf8').digest('hex')
    if (!requestDigest || !manifestDigest || !operationId || !sessionNonce || !expiresAt || !requestFingerprint) return null
    return {
      requestDigest, requestFingerprint, manifestDigest, itemSetDigest,
      operationId, sessionNonce, expiresAt
    }
  } catch {
    return null
  }
}

function validSelectedPath(path: string): boolean {
  if (!path || !isAbsolute(path) || path.includes('\u0000')) return false
  const normalized = resolve(path)
  if (normalized !== path || path.split(/[\\/]+/).some((part) => part === '..')) return false
  return true
}

function frameIdentity(frame: MainFrameLike | null | undefined): string {
  if (!frame) return ''
  return `${frame.routingId ?? ''}\0${frame.url ?? ''}`
}

function localBindingsEqual(
  left: ProtectedRecoveryLocalBinding,
  right: ProtectedRecoveryLocalBinding
): boolean {
  return left.browserWindowId === right.browserWindowId &&
    left.mainFrameId === right.mainFrameId &&
    left.profileBinding === right.profileBinding &&
    left.dataDirectoryBinding === right.dataDirectoryBinding
}

function ownerBindingKey(entry: ProtectedRecoveryOwnerBinding): string {
  return [
    entry.correlation ?? '', entry.owner, entry.provider, entry.accountId,
    entry.channelId, entry.purpose, entry.fingerprint
  ].join('\u0000')
}

function ownerBindingsEqual(
  left: ProtectedRecoveryOwnerBinding[],
  right: ProtectedRecoveryOwnerBinding[]
): boolean {
  if (left.length !== right.length) return false
  const leftKeys = left.map(ownerBindingKey).sort()
  const rightKeys = right.map(ownerBindingKey).sort()
  return leftKeys.every((key, index) => key === rightKeys[index])
}

function isNotFoundError(error: unknown): boolean {
  const value = objectValue(error)
  return value?.code === 'ENOENT'
}

function boundedEntryCount(value: number | undefined): number | undefined | null {
  if (value === undefined) return undefined
  return Number.isSafeInteger(value) && value >= 0 && value <= 128 ? value : null
}

function inventoryForManifest(
  manifestJson: string,
  inventory: ProtectedRecoveryOwnerBinding[]
): ProtectedRecoveryOwnerBinding[] | null {
  let manifest: ReturnType<typeof parseProviderRegistryPortableManifestV1>
  try {
    manifest = parseProviderRegistryPortableManifestV1(manifestJson)
  } catch {
    return null
  }
  if (!manifest) return null
  const selected: ProtectedRecoveryOwnerBinding[] = []
  const used = new Set<string>()
  for (const descriptor of manifest.accounts) {
    const candidates = inventory.filter((entry) =>
      entry.owner === descriptor.owner && entry.provider === descriptor.provider && entry.purpose === descriptor.purpose
    )
    if (candidates.length !== 1) return null
    const candidate = candidates[0]!
    const identity = `${candidate.owner}\0${candidate.provider}\0${candidate.accountId}\0${candidate.channelId}\0${candidate.purpose}\0${candidate.fingerprint}`
    if (used.has(identity)) return null
    used.add(identity)
    selected.push({ ...candidate, correlation: descriptor.correlation })
  }
  return selected
}

function inventoryForImportReceipt(
  receipt: ProtectedRecoveryImportReceipt,
  inventory: ProtectedRecoveryOwnerBinding[]
): ProtectedRecoveryOwnerBinding[] | null {
  const manifest = parseProviderRegistryPortableManifestV1(receipt.manifestJson)
  if (!manifest || receipt.entries.length !== manifest.providers.length + manifest.accounts.length) return null
  const selected: ProtectedRecoveryOwnerBinding[] = []
  const used = new Set<string>()
  for (const [index, descriptor] of manifest.accounts.entries()) {
    const receiptEntry = receipt.entries[manifest.providers.length + index]
    const admitted = receiptEntry?.destinationOwnerBinding
    if (!receiptEntry || receiptEntry.correlation !== descriptor.correlation ||
      !validImportReceiptOwnerBinding(admitted, descriptor)) return null
    const candidates = inventory.filter((entry) =>
      entry.owner === admitted.owner && entry.provider === admitted.provider &&
      entry.accountId === admitted.accountId && entry.channelId === admitted.channelId &&
      entry.purpose === admitted.purpose && entry.fingerprint === admitted.fingerprint &&
      (entry.correlation === undefined || entry.correlation === admitted.correlation)
    )
    if (candidates.length !== 1) return null
    const candidate = { ...candidates[0]!, correlation: admitted.correlation }
    if (!ownerBindingsEqual([admitted], [candidate])) return null
    const identity = ownerBindingKey(candidate)
    if (used.has(identity)) return null
    used.add(identity)
    selected.push(candidate)
  }
  return selected
}

export function createMainProviderCredentialRecoveryAuthority(
  options: MainProviderCredentialRecoveryFactoryOptions = {}
): MainProviderCredentialRecoveryAuthority {
  let confirmationWindow: MainWindowLike | null = null
  const openDialog = options.showOpenDialog ?? (async (window, dialogOptions) => {
    const result = await dialog.showOpenDialog(window as unknown as BrowserWindowType, dialogOptions ?? {})
    return { canceled: result.canceled, filePaths: result.filePaths }
  })
  const saveDialog = options.showSaveDialog ?? (async (window, dialogOptions) => {
    const result = await dialog.showSaveDialog(window as unknown as BrowserWindowType, dialogOptions ?? {})
    return { canceled: result.canceled, filePath: result.filePath }
  })
  const confirm = options.confirmNative ?? (async (confirmation: Record<string, unknown>) => {
    if (!confirmationWindow) return false
    const side = confirmation.side === 'source' ? 'source' : 'destination'
    const fingerprint = typeof confirmation.requestFingerprint === 'string'
      ? confirmation.requestFingerprint
      : 'unavailable'
    const itemSetDigest = typeof confirmation.itemSetDigest === 'string'
      ? confirmation.itemSetDigest.slice(0, 16)
      : 'unavailable'
    const expiresAt = typeof confirmation.expiresAt === 'string' ? confirmation.expiresAt : 'unavailable'
    const itemCount = typeof confirmation.entryCount === 'number' ? confirmation.entryCount : 'bounded'
    const response = await dialog.showMessageBox(confirmationWindow as unknown as BrowserWindowType, {
      type: 'warning',
      buttons: ['Continue', 'Cancel'],
      defaultId: 1,
      cancelId: 1,
      noLink: true,
      title: 'Protected provider credential recovery',
      message: `Confirm ${side} protected recovery\nFingerprint: ${fingerprint}\nItem set: ${itemSetDigest} (${itemCount} items)\nExpires: ${expiresAt}`
    })
    return response.response === 0
  })
  const stat = options.lstat ?? (async (path) => fsLstat(path))
  const read = options.readFile ?? (async (path) => secureReadArtifactFile(path, MAX_ARTIFACT_BYTES))
  const write = options.writeFile ?? (async (path, bytes, writeOptions) => {
    if (writeOptions?.flag && writeOptions.flag !== 'w') throw new Error('unsupported_artifact_write_mode')
    await secureWriteArtifactFile(path, bytes)
  })
  const protectedRequest = options.protectedRuntimeRequest ??
    (async (): Promise<ProtectedRecoveryRuntimeResult> => ({ ok: false, code: 'runtime_unavailable' }))
  const callProtected = async (call: ProtectedRecoveryRuntimeCall): Promise<ProtectedRecoveryRuntimeResult> => {
    try {
      return await protectedRequest(call)
    } catch {
      return { ok: false, code: 'runtime_unavailable' }
    }
  }
  let pending: PendingRequest | null = null

  const currentWindow = (event: ProtectedRecoveryInvokeEvent): MainWindowLike | null => {
    if (options.getCurrentWindow) return options.getCurrentWindow(event)
    return BrowserWindow.fromWebContents(event.sender as never) as unknown as MainWindowLike | null
  }
  const currentFrame = (event: ProtectedRecoveryInvokeEvent): MainFrameLike | null => {
    if (options.getCurrentMainFrame) return options.getCurrentMainFrame(event)
    return event.sender.mainFrame ?? null
  }
  const currentProfile = async (): Promise<ProtectedRecoveryProfile | null> => {
    if (options.getCurrentProfile) return options.getCurrentProfile()
    try {
      const dataDirectory = app.getPath('userData')
      return {
        dataDirectory,
        profileBinding: createHash('sha256').update(`analytix-profile-v1\0${dataDirectory}`).digest('hex')
      }
    } catch {
      return null
    }
  }
  const localContext = async (event: ProtectedRecoveryInvokeEvent): Promise<{
    window: MainWindowLike
    frame: MainFrameLike
    profile: ProtectedRecoveryProfile
    binding: ProtectedRecoveryLocalBinding
  } | null> => {
    try {
      const window = currentWindow(event)
      const frame = currentFrame(event)
      if (!window || !frame || window.isDestroyed() || window.webContents.isDestroyed() || event.sender.isDestroyed()) return null
      if (window.webContents !== event.sender || window.webContents.id !== event.sender.id || event.senderFrame !== frame) return null
      if (window.webContents.mainFrame && frameIdentity(window.webContents.mainFrame) !== frameIdentity(frame)) return null
      if (typeof frame.url !== 'string' || frame.url.length === 0) return null
      if (event.sender.getURL) {
        try {
          if (event.sender.getURL() !== frame.url) return null
        } catch {
          return null
        }
      }
      const profile = await currentProfile()
      if (!profile || !profile.profileBinding || !profile.dataDirectory) return null
      return {
        window, frame, profile,
        binding: {
          browserWindowId: String(window.id),
          mainFrameId: String(frame.routingId ?? frame.url),
          profileBinding: profile.profileBinding,
          dataDirectoryBinding: profile.dataDirectory
        }
      }
    } catch {
      return null
    }
  }

  const resolveInventory = async (): Promise<ProtectedRecoveryOwnerBinding[] | null> => {
    try {
      const raw = options.getCurrentOwnerBindingInventory
        ? await options.getCurrentOwnerBindingInventory()
        : []
      if (!Array.isArray(raw) || raw.length > 128) return null
      const inventory = raw.map((entry) => ({ ...entry }))
      const seen = new Set<string>()
      for (const entry of inventory) {
        if (!entry || (entry.owner !== 'mcp' && entry.owner !== 'extension') ||
          !entry.provider || !entry.accountId || !entry.channelId || !entry.fingerprint || !ALLOWED_PURPOSES.has(entry.purpose) ||
          (entry.owner === 'mcp' && entry.purpose !== 'mcp-oauth-access-token') ||
          (entry.owner === 'extension' && entry.purpose !== 'extension-provider-account-token')) return null
        const key = `${entry.correlation ?? ''}\0${entry.owner}\0${entry.provider}\0${entry.accountId}\0${entry.channelId}\0${entry.purpose}\0${entry.fingerprint}`
        if (seen.has(key)) return null
        seen.add(key)
      }
      return inventory
    } catch {
      return null
    }
  }

  const selectedInventoryIsCurrent = async (selected: ProtectedRecoveryOwnerBinding[]): Promise<boolean> => {
    try {
      const [mcpFingerprint, extensionFingerprint] = await Promise.all([
        options.resolveMcpBindingFingerprint?.(),
        options.resolveExtensionBindingFingerprint?.()
      ])
      return (mcpFingerprint === undefined || !selected.some((entry) =>
        entry.owner === 'mcp' && entry.fingerprint !== mcpFingerprint
      )) && (extensionFingerprint === undefined || !selected.some((entry) =>
        entry.owner === 'extension' && entry.fingerprint !== extensionFingerprint
      ))
    } catch {
      return false
    }
  }

  const currentImportReceipt = async (): Promise<ProtectedRecoveryImportReceipt | null> => {
    if (!options.getPortableImportReceipt) return null
    try {
      return await options.getPortableImportReceipt()
    } catch {
      return null
    }
  }

  const currentPortableManifest = async (): Promise<string | null> => {
    if (!options.getCurrentPortableManifest) return null
    let bytes: Buffer | null = null
    try {
      const raw = await options.getCurrentPortableManifest()
      bytes = typeof raw === 'string' ? Buffer.from(raw, 'utf8') : raw ? Buffer.from(raw) : null
      if (!bytes || bytes.length === 0 || bytes.length > MAX_IMPORT_RECEIPT_BYTES) return null
      const text = bytes.toString('utf8')
      if (Buffer.byteLength(text, 'utf8') !== bytes.length || hasForbiddenArtifactKey(JSON.parse(text))) return null
      const parsed = parseProviderRegistryPortableManifestV1(text)
      return parsed && canonicalJSON(parsed) === text ? text : null
    } catch {
      return null
    } finally {
      if (bytes) bytes.fill(0)
    }
  }

  const targetFor = async (role: ProtectedRecoveryArtifactRole): Promise<string | null> => {
    try {
      const target = options.getSaveTarget ? await options.getSaveTarget(role) : null
      return target && validSelectedPath(target) ? target : target === null ? null : ''
    } catch {
      return ''
    }
  }
  const checkPath = async (path: string, kind: 'open' | 'save'): Promise<boolean> => {
    if (!validSelectedPath(path)) return false
    try {
      const stats = await stat(path)
      if (isRegularArtifactStats(stats)) {
        // continue with the parent validation for save targets
      } else if (kind === 'save' && !stats.isSymbolicLink() && !stats.isDirectory() &&
        !stats.isBlockDevice?.() && !stats.isCharacterDevice?.() && !stats.isFIFO?.() && !stats.isSocket?.()) {
        // Test doubles may represent ENOENT with an otherwise empty stat
        // object. Real lstat/open paths never use this branch.
      } else {
        return false
      }
    } catch (error) {
      if (kind !== 'save' || !isNotFoundError(error)) return false
    }
    if (kind !== 'save') return true
    // A save target may be new, but its selected parent must already be a
    // regular non-symlink directory. Main never creates arbitrary parents for
    // a user-selected protected artifact.
    try {
      const parentStats = await stat(dirname(path))
      if (!parentStats.isDirectory() || parentStats.isSymbolicLink() ||
        parentStats.isBlockDevice?.() || parentStats.isCharacterDevice?.() ||
        parentStats.isFIFO?.() || parentStats.isSocket?.()) return false
      return true
    } catch {
      return false
    }
  }
  const selectOpen = async (window: MainWindowLike, role: ProtectedRecoveryArtifactRole): Promise<string | null> => {
    try {
      const result = await openDialog(window, {
        properties: ['openFile'],
        title: role === 'request' ? 'Open protected recovery request' : role === 'bundle' ? 'Open protected recovery bundle' : 'Open protected recovery receipt'
      })
      const path = !result.canceled && result.filePaths.length === 1 ? result.filePaths[0] : null
      return path && await checkPath(path, 'open') ? path : null
    } catch {
      return null
    }
  }
  const selectSave = async (
    window: MainWindowLike,
    role: ProtectedRecoveryArtifactRole,
    preflightTarget: string | null
  ): Promise<string | null> => {
    const result = await saveDialog(window, {
      title: role === 'request' ? 'Save protected recovery request' : role === 'bundle' ? 'Save protected recovery bundle' : 'Save protected recovery receipt'
    })
    const path = !result.canceled && result.filePath ? result.filePath : null
    if (!path || !validSelectedPath(path)) return null
    if (preflightTarget && path !== preflightTarget) return null
    return await checkPath(path, 'save') ? path : null
  }
  const readArtifact = async (window: MainWindowLike, role: ProtectedRecoveryArtifactRole): Promise<Buffer | null> => {
    const path = await selectOpen(window, role)
    if (!path) return null
    let bytes: Buffer | null = null
    try {
      bytes = await read(path)
      if (!Buffer.isBuffer(bytes)) bytes = Buffer.from(bytes)
      if (!validArtifact(bytes, role)) {
        zeroize(bytes)
        return null
      }
      return bytes
    } catch {
      zeroize(bytes)
      return null
    }
  }
  const writeArtifact = async (
    window: MainWindowLike,
    role: ProtectedRecoveryArtifactRole,
    bytes: Buffer,
    preflightTarget: string | null
  ): Promise<boolean> => {
    try {
      const path = await selectSave(window, role, preflightTarget)
      if (!path) return false
      await write(path, bytes, { mode: 0o600, flag: 'w' })
      return true
    } catch {
      return false
    } finally {
      zeroize(bytes)
    }
  }
  const rollbackRequest = async (requestBytes: Uint8Array): Promise<MainProviderCredentialRecoveryResult | null> => {
    const rollbackBytes = Buffer.from(requestBytes)
    try {
      const rolledBack = await callProtected({ operation: 'rollback-protected-recovery', requestBytes: rollbackBytes })
      if (!rolledBack.ok) return resultForRuntimeFailure(rolledBack)
      return rolledBack.status === 'rolled_back'
        ? null
        : { ok: false, code: 'invalid_response' }
    } finally {
      zeroize(rollbackBytes)
    }
  }
  const rollbackAfterMutation = async (
    requestBytes: Uint8Array,
    fallback: MainProviderCredentialRecoveryResult
  ): Promise<MainProviderCredentialRecoveryResult> => {
    const rollbackFailure = await rollbackRequest(requestBytes)
    return rollbackFailure ?? fallback
  }
  const localAction = (
    context: Awaited<ReturnType<typeof localContext>>,
    metadata: PendingRequestMetadata
  ): ProtectedRecoveryLocalAction => ({
    localBinding: context!.binding,
    confirmation: {
      ...metadata,
      localBinding: context!.binding
    }
  })
  const confirmation = async (
    side: 'destination' | 'source',
    context: Awaited<ReturnType<typeof localContext>>,
    metadata: PendingRequestMetadata
  ): Promise<boolean> => {
    try {
      confirmationWindow = context!.window
      return await confirm({
        side,
        ...metadata,
        entryCount: metadata.itemSetDigest ? 'bounded' : 'unavailable'
      })
    } catch {
      return false
    }
  }

  const createDestinationRequest = async (event: ProtectedRecoveryInvokeEvent): Promise<MainProviderCredentialRecoveryResult> => {
    const context = await localContext(event)
    if (!context) return { ok: false, code: 'invalid_context' }
    const receipt = await currentImportReceipt()
    if (!receipt || !validImportReceipt(receipt)) {
      return { ok: false, code: 'invalid_request' }
    }
    const inventory = await resolveInventory()
    if (!inventory) return { ok: false, code: 'invalid_request' }
    const manifestInventory = inventoryForImportReceipt(receipt, inventory)
    if (!manifestInventory || !await selectedInventoryIsCurrent(manifestInventory)) return { ok: false, code: 'invalid_request' }
    const preflightTarget = await targetFor('request')
    if (preflightTarget === '') return { ok: false, code: 'invalid_path' }
    if (preflightTarget && !await checkPath(preflightTarget, 'save')) return { ok: false, code: 'invalid_path' }
    const prepared = await callProtected({
      operation: 'prepare-protected-recovery-request',
      importReceipt: receipt,
      localBinding: context.binding,
      ownerBindingInventory: manifestInventory
    })
    if (!prepared.ok || !prepared.artifactBytes) return prepared.ok ? { ok: false, code: 'invalid_response' } : resultForRuntimeFailure(prepared)
    const requestBytes = asBuffer(prepared.artifactBytes)
    const failPrepared = async (fallback: MainProviderCredentialRecoveryResult): Promise<MainProviderCredentialRecoveryResult> => {
      const result = await rollbackAfterMutation(requestBytes, fallback)
      zeroize(requestBytes)
      return result
    }
    if (!validArtifact(requestBytes, 'request')) {
      return failPrepared({ ok: false, code: 'invalid_response' })
    }
    const artifactMetadata = metadataFromRequest(requestBytes)
    if (!artifactMetadata) {
      return failPrepared({ ok: false, code: 'invalid_response' })
    }
    const metadataFromPrepared = {
      requestDigest: prepared.requestDigest ?? '',
      requestFingerprint: prepared.requestFingerprint ?? '',
      manifestDigest: prepared.manifestDigest ?? '',
      itemSetDigest: prepared.itemSetDigest ?? '',
      operationId: prepared.operationId ?? '',
      sessionNonce: prepared.sessionNonce ?? '',
      expiresAt: prepared.expiresAt ?? ''
    }
    const metadata = Object.values(metadataFromPrepared).every((value) => value.length > 0)
      ? metadataFromPrepared
      : artifactMetadata
    if (!metadata) {
      return failPrepared({ ok: false, code: 'invalid_response' })
    }
    if (metadata.requestDigest !== artifactMetadata.requestDigest ||
      metadata.requestFingerprint !== artifactMetadata.requestFingerprint ||
      metadata.manifestDigest !== artifactMetadata.manifestDigest ||
      metadata.itemSetDigest !== artifactMetadata.itemSetDigest ||
      metadata.operationId !== artifactMetadata.operationId ||
      metadata.sessionNonce !== artifactMetadata.sessionNonce ||
      metadata.expiresAt !== artifactMetadata.expiresAt) {
      return failPrepared({ ok: false, code: 'invalid_response' })
    }
    const promptContext = await localContext(event)
    const promptReceipt = await currentImportReceipt()
    const promptInventory = await resolveInventory()
    const promptOwners = promptInventory && inventoryForImportReceipt(receipt, promptInventory)
    if (!promptContext || !localBindingsEqual(context.binding, promptContext.binding) ||
      !promptReceipt || !validImportReceipt(promptReceipt) || !importReceiptsEqual(receipt, promptReceipt) ||
      !promptOwners || !await selectedInventoryIsCurrent(promptOwners) ||
      !ownerBindingsEqual(manifestInventory, promptOwners)) {
      return failPrepared({ ok: false, code: !promptContext ? 'invalid_context' : 'invalid_request' })
    }
    if (!await confirmation('destination', promptContext, metadata)) {
      return failPrepared({ ok: false, code: 'cancelled' })
    }
    const confirmedContext = await localContext(event)
    if (!confirmedContext || !localBindingsEqual(promptContext.binding, confirmedContext.binding)) {
      return failPrepared({ ok: false, code: 'invalid_context' })
    }
    const confirmedReceipt = await currentImportReceipt()
    const confirmedInventory = await resolveInventory()
    if (!confirmedInventory || !confirmedReceipt || !validImportReceipt(confirmedReceipt) ||
      !importReceiptsEqual(receipt, confirmedReceipt)) {
      return failPrepared({ ok: false, code: 'invalid_request' })
    }
    const confirmedManifestInventory = inventoryForImportReceipt(receipt, confirmedInventory)
    if (!confirmedManifestInventory || !await selectedInventoryIsCurrent(confirmedManifestInventory) ||
      !ownerBindingsEqual(promptOwners, confirmedManifestInventory)) {
      return failPrepared({ ok: false, code: 'invalid_request' })
    }
    const confirmed = await callProtected({
      operation: 'confirm-protected-recovery-destination',
      requestBytes,
      localAction: localAction(confirmedContext, metadata),
      ownerBindingInventory: confirmedManifestInventory
    })
    if (!confirmed.ok) {
      return failPrepared(resultForRuntimeFailure(confirmed))
    }
    if (confirmed.status !== 'destination-confirmed') {
      return failPrepared({ ok: false, code: 'invalid_response' })
    }
    const postConfirmContext = await localContext(event)
    const postConfirmInventory = await resolveInventory()
    const postConfirmOwners = postConfirmInventory && inventoryForImportReceipt(receipt, postConfirmInventory)
    const postConfirmReceipt = await currentImportReceipt()
    if (!postConfirmContext || !localBindingsEqual(confirmedContext.binding, postConfirmContext.binding) ||
      !postConfirmOwners || !await selectedInventoryIsCurrent(postConfirmOwners) ||
      !ownerBindingsEqual(confirmedManifestInventory, postConfirmOwners) ||
      !postConfirmReceipt || !validImportReceipt(postConfirmReceipt) ||
      !importReceiptsEqual(receipt, postConfirmReceipt)) {
      return failPrepared({ ok: false, code: !postConfirmContext ? 'invalid_context' : 'invalid_request' })
    }
    pending = {
      bytes: Buffer.alloc(0),
      requestText: requestBytes.toString('utf8'),
      ...metadata,
      localBinding: confirmedContext.binding,
      importReceipt: receipt
    }
    const requestForRollback = Buffer.from(requestBytes)
    const wrote = await writeArtifact(confirmedContext.window, 'request', requestBytes, preflightTarget)
    if (!wrote) {
      pending = null
      try {
        return await rollbackAfterMutation(requestForRollback, { ok: false, code: 'cancelled' })
      } finally {
        zeroize(requestForRollback)
      }
    }
    return { ok: true, status: 'request-created' }
  }

  const createSourceBundle = async (event: ProtectedRecoveryInvokeEvent): Promise<MainProviderCredentialRecoveryResult> => {
    const context = await localContext(event)
    if (!context) return { ok: false, code: 'invalid_context' }
    const requestBytes = await readArtifact(context.window, 'request')
    if (!requestBytes) return { ok: false, code: 'invalid_path' }
    try {
      const metadata = pending && pending.requestText === requestBytes.toString('utf8')
        ? pending
        : metadataFromRequest(requestBytes)
      if (!metadata) return { ok: false, code: 'invalid_request' }
      const manifest = await currentPortableManifest()
      if (!manifest || createHash('sha256').update(manifest).digest('hex') !== metadata.manifestDigest) {
        return { ok: false, code: 'invalid_request' }
      }
      const inventory = await resolveInventory()
      const sourceInventory = inventory && inventoryForManifest(manifest, inventory)
      if (!sourceInventory || !await selectedInventoryIsCurrent(sourceInventory)) return { ok: false, code: 'invalid_request' }
      const confirmationContext = await localContext(event)
      const confirmationManifest = await currentPortableManifest()
      const confirmationInventory = await resolveInventory()
      const confirmationManifestInventory = confirmationManifest && confirmationInventory &&
        createHash('sha256').update(confirmationManifest).digest('hex') === metadata.manifestDigest
        ? inventoryForManifest(confirmationManifest, confirmationInventory)
        : null
      if (!confirmationContext || !localBindingsEqual(context.binding, confirmationContext.binding) ||
        !confirmationManifestInventory || !await selectedInventoryIsCurrent(confirmationManifestInventory)) {
        return { ok: false, code: !confirmationContext ? 'invalid_context' : 'invalid_request' }
      }
      if (!await confirmation('source', confirmationContext, metadata)) return { ok: false, code: 'cancelled' }
      const bundleContext = await localContext(event)
      const bundleInventory = await resolveInventory()
      if (!bundleContext || !localBindingsEqual(confirmationContext.binding, bundleContext.binding)) {
        return { ok: false, code: 'invalid_context' }
      }
      if (!bundleInventory) return { ok: false, code: 'invalid_request' }
      const bundleManifest = await currentPortableManifest()
      const sourceManifestInventory = bundleManifest &&
        createHash('sha256').update(bundleManifest).digest('hex') === metadata.manifestDigest
        ? inventoryForManifest(bundleManifest, bundleInventory)
        : null
      if (!sourceManifestInventory || !await selectedInventoryIsCurrent(sourceManifestInventory)) return { ok: false, code: 'invalid_request' }
      const created = await callProtected({
        operation: 'create-protected-recovery-bundle',
        requestBytes,
        localAction: localAction(bundleContext, metadata),
        ownerBindingInventory: sourceManifestInventory
      })
      if (!created.ok || !created.artifactBytes) {
        return await rollbackAfterMutation(
          requestBytes,
          created.ok ? { ok: false, code: 'invalid_response' } : resultForRuntimeFailure(created)
        )
      }
      const bundleBytes = asBuffer(created.artifactBytes)
      if (!validArtifact(bundleBytes, 'bundle')) {
        zeroize(bundleBytes)
        return await rollbackAfterMutation(requestBytes, { ok: false, code: 'invalid_response' })
      }
      const postCreateContext = await localContext(event)
      const postCreateManifest = await currentPortableManifest()
      const postCreateInventory = await resolveInventory()
      const postCreateOwners = postCreateManifest && postCreateInventory &&
        createHash('sha256').update(postCreateManifest).digest('hex') === metadata.manifestDigest
        ? inventoryForManifest(postCreateManifest, postCreateInventory)
        : null
      if (!postCreateContext || !localBindingsEqual(bundleContext.binding, postCreateContext.binding) ||
        !postCreateOwners || !await selectedInventoryIsCurrent(postCreateOwners)) {
        zeroize(bundleBytes)
        return await rollbackAfterMutation(requestBytes, { ok: false, code: !postCreateContext ? 'invalid_context' : 'invalid_request' })
      }
      const preflightTarget = await targetFor('bundle')
      if (preflightTarget === '') {
        zeroize(bundleBytes)
        return await rollbackAfterMutation(requestBytes, { ok: false, code: 'invalid_path' })
      }
      if (preflightTarget && !await checkPath(preflightTarget, 'save')) {
        zeroize(bundleBytes)
        return await rollbackAfterMutation(requestBytes, { ok: false, code: 'invalid_path' })
      }
      const wrote = await writeArtifact(bundleContext.window, 'bundle', bundleBytes, preflightTarget)
      if (!wrote) return await rollbackAfterMutation(requestBytes, { ok: false, code: 'cancelled' })
      return { ok: true, status: 'bundle-created' }
    } finally {
      zeroize(requestBytes)
    }
  }

  const recoverReceiptContinuation = async (
    event: ProtectedRecoveryInvokeEvent,
    requestBytes: Buffer,
    metadata: PendingRequestMetadata,
    importReceipt: ProtectedRecoveryImportReceipt | null
  ): Promise<{ result: MainProviderCredentialRecoveryResult | null; receiptBytes?: Buffer; context?: Awaited<ReturnType<typeof localContext>> }> => {
    const context = await localContext(event)
    const inventory = await resolveInventory()
    const currentReceipt = importReceipt ? await currentImportReceipt() : null
    const receiptCurrent = !importReceipt || Boolean(currentReceipt && validImportReceipt(currentReceipt) &&
      importReceiptsEqual(importReceipt, currentReceipt))
    const owners = inventory && (importReceipt ? inventoryForImportReceipt(importReceipt, inventory) : inventory)
    const ownersCurrent = owners && (importReceipt ? await selectedInventoryIsCurrent(owners) : true)
    if (!context || !owners || !ownersCurrent || !receiptCurrent) {
      return { result: !context ? { ok: false, code: 'invalid_context' } : { ok: false, code: 'invalid_request' } }
    }

    // Targeted durable recovery is the authority preflight. An untrusted or
    // stale request must never reach a high-trust native confirmation prompt.
    const recovered = await callProtected({
      operation: 'recover-protected-recovery',
      requestBytes,
      localAction: localAction(context, metadata),
      ownerBindingInventory: owners
    })
    if (!recovered.ok) return { result: resultForRuntimeFailure(recovered) }
    const recoveredArtifact = recovered.artifactBytes ? asBuffer(recovered.artifactBytes) : null
    let handedOffArtifact = false
    try {
      if (recovered.status !== 'receipt_ready' && recovered.status !== 'pending' &&
        recovered.status !== 'reconfirmation_required') {
        return { result: { ok: false, code: 'invalid_response' } }
      }
      if (recovered.status !== 'receipt_ready' && recoveredArtifact) {
        return { result: { ok: false, code: 'invalid_response' } }
      }

      const promptContext = await localContext(event)
      const promptInventory = await resolveInventory()
      const promptReceipt = importReceipt ? await currentImportReceipt() : null
      const promptReceiptCurrent = !importReceipt || Boolean(promptReceipt && validImportReceipt(promptReceipt) &&
        importReceiptsEqual(importReceipt, promptReceipt))
      const promptOwners = promptInventory && (importReceipt
        ? inventoryForImportReceipt(importReceipt, promptInventory)
        : promptInventory)
      const promptOwnersCurrent = promptOwners && (importReceipt ? await selectedInventoryIsCurrent(promptOwners) : true)
      if (!promptContext || !localBindingsEqual(context.binding, promptContext.binding) ||
        !promptOwners || !promptOwnersCurrent || !promptReceiptCurrent || !ownerBindingsEqual(owners, promptOwners)) {
        return { result: !promptContext ? { ok: false, code: 'invalid_context' } : { ok: false, code: 'invalid_request' } }
      }
      if (!await confirmation('destination', promptContext, metadata)) return { result: { ok: false, code: 'cancelled' } }
      const confirmedContext = await localContext(event)
      const confirmedInventory = await resolveInventory()
      const confirmedReceipt = importReceipt ? await currentImportReceipt() : null
      const confirmedReceiptCurrent = !importReceipt || Boolean(confirmedReceipt && validImportReceipt(confirmedReceipt) &&
        importReceiptsEqual(importReceipt, confirmedReceipt))
      const confirmedOwners = confirmedInventory && (importReceipt
        ? inventoryForImportReceipt(importReceipt, confirmedInventory)
        : confirmedInventory)
      const confirmedOwnersCurrent = confirmedOwners && (importReceipt
        ? await selectedInventoryIsCurrent(confirmedOwners)
        : true)
      if (!confirmedContext || !localBindingsEqual(promptContext.binding, confirmedContext.binding) ||
        !confirmedOwners || !confirmedOwnersCurrent || !confirmedReceiptCurrent ||
        !ownerBindingsEqual(promptOwners, confirmedOwners)) {
        return { result: !confirmedContext ? { ok: false, code: 'invalid_context' } : { ok: false, code: 'invalid_request' } }
      }
      if (recovered.status === 'receipt_ready') {
        if (!recoveredArtifact || !validArtifact(recoveredArtifact, 'receipt')) {
          return { result: { ok: false, code: 'invalid_response' } }
        }
        handedOffArtifact = true
        return { result: null, receiptBytes: recoveredArtifact, context: confirmedContext }
      }
      if (recovered.status !== 'pending' && recovered.status !== 'reconfirmation_required') {
        return { result: { ok: false, code: 'invalid_response' } }
      }
      const confirmed = await callProtected({
        operation: 'confirm-protected-recovery-destination',
        requestBytes,
        localAction: localAction(confirmedContext, metadata),
        ownerBindingInventory: confirmedOwners
      })
      if (!confirmed.ok) return { result: resultForRuntimeFailure(confirmed) }
      if (confirmed.status !== 'destination-confirmed') return { result: { ok: false, code: 'invalid_response' } }
      return { result: null, context: confirmedContext }
    } finally {
      if (!handedOffArtifact) zeroize(recoveredArtifact)
    }
  }

  const applyDestinationBundle = async (event: ProtectedRecoveryInvokeEvent): Promise<MainProviderCredentialRecoveryResult> => {
    const context = await localContext(event)
    if (!context) return { ok: false, code: 'invalid_context' }
    let requestBytes: Buffer | null = null
    let bundleBytes: Buffer | null = null
    try {
      requestBytes = pending ? Buffer.from(pending.requestText, 'utf8') : await readArtifact(context.window, 'request')
      if (!requestBytes) return { ok: false, code: 'invalid_path' }
      const metadata = metadataFromRequest(requestBytes)
      if (!metadata) return { ok: false, code: 'invalid_request' }
      const receipt = await currentImportReceipt()
      const currentInventory = await resolveInventory()
      let destinationReceipt: ProtectedRecoveryImportReceipt | null = null
      let destinationOwners: ProtectedRecoveryOwnerBinding[] | null = null
      if (receipt !== null) {
        if (!validImportReceipt(receipt) ||
          createHash('sha256').update(receipt.manifestJson).digest('hex') !== metadata.manifestDigest ||
          !importReceiptMatchesRequest(receipt, requestBytes) ||
          (pending && pending.requestText === requestBytes.toString('utf8') && !importReceiptsEqual(pending.importReceipt, receipt))) {
          return { ok: false, code: 'invalid_request' }
        }
        destinationReceipt = receipt
        destinationOwners = currentInventory && inventoryForImportReceipt(receipt, currentInventory)
        if (!destinationOwners || !await selectedInventoryIsCurrent(destinationOwners)) return { ok: false, code: 'invalid_request' }
      } else {
        // After a Main restart the Go protected session is the durable owner of
        // the canonical import mapping. Pass the complete current Main-owned
        // inventory so Go can match stored correlations and ignore unrelated
        // accounts; no in-memory import receipt is an authority requirement.
        if (pending || !currentInventory) return { ok: false, code: 'invalid_request' }
        destinationOwners = currentInventory
      }
      const sameProcessPending = Boolean(pending && pending.requestText === requestBytes.toString('utf8') &&
        localBindingsEqual(context.binding, pending.localBinding))
      let applyContext = context
      if (!sameProcessPending) {
        const continuation = await recoverReceiptContinuation(event, requestBytes, metadata, destinationReceipt)
        if (continuation.result) return continuation.result
        if (continuation.receiptBytes) {
          const preflightTarget = await targetFor('receipt')
          if (preflightTarget === '' || (preflightTarget && !await checkPath(preflightTarget, 'save'))) {
            zeroize(continuation.receiptBytes)
            return { ok: false, code: 'invalid_path' }
          }
          const recoveredReceipt = continuation.receiptBytes
          const wrote = await writeArtifact(continuation.context!.window, 'receipt', recoveredReceipt, preflightTarget)
          if (wrote) return { ok: true, status: 'receipt-created' }
          const retry = await recoverReceiptContinuation(event, requestBytes, metadata, destinationReceipt)
          if (retry.result) return retry.result
          if (!retry.receiptBytes) return { ok: false, code: 'cancelled' }
          const retryTarget = await targetFor('receipt')
          if (retryTarget === '' || (retryTarget && !await checkPath(retryTarget, 'save'))) {
            zeroize(retry.receiptBytes)
            return { ok: false, code: 'invalid_path' }
          }
          const retryBytes = retry.receiptBytes
          const retryWrote = await writeArtifact(retry.context!.window, 'receipt', retryBytes, retryTarget)
          return retryWrote ? { ok: true, status: 'receipt-created' } : { ok: false, code: 'cancelled' }
        }
        applyContext = continuation.context ?? context
      }
      const applyInventory = await resolveInventory()
      const applyOwners = applyInventory && (destinationReceipt
        ? inventoryForImportReceipt(destinationReceipt, applyInventory)
        : applyInventory)
      const applyOwnersCurrent = applyOwners && (destinationReceipt
        ? await selectedInventoryIsCurrent(applyOwners)
        : true)
      if (!applyOwners || !applyOwnersCurrent) return { ok: false, code: 'invalid_request' }
      const preflightTarget = await targetFor('receipt')
      if (preflightTarget === '' || (preflightTarget && !await checkPath(preflightTarget, 'save'))) {
        return { ok: false, code: 'invalid_path' }
      }
      bundleBytes = await readArtifact(applyContext.window, 'bundle')
      if (!bundleBytes) return { ok: false, code: 'invalid_path' }
      // The native open dialog and bounded read are an untrusted boundary.
      // Rebind both local context and the complete owner inventory immediately
      // before the private apply call so a window/profile/owner change during
      // file selection cannot authorize destination mutation.
      const postOpenContext = await localContext(event)
      const postOpenInventory = await resolveInventory()
      const postOpenReceipt = destinationReceipt ? await currentImportReceipt() : null
      const postOpenReceiptCurrent = !destinationReceipt || Boolean(postOpenReceipt && validImportReceipt(postOpenReceipt) &&
        importReceiptsEqual(destinationReceipt, postOpenReceipt))
      const postOpenOwners = postOpenInventory && (destinationReceipt
        ? inventoryForImportReceipt(destinationReceipt, postOpenInventory)
        : postOpenInventory)
      const postOpenOwnersCurrent = postOpenOwners && (destinationReceipt
        ? await selectedInventoryIsCurrent(postOpenOwners)
        : true)
      if (!postOpenContext || !localBindingsEqual(applyContext.binding, postOpenContext.binding) ||
        !postOpenOwners || !postOpenOwnersCurrent || !postOpenReceiptCurrent ||
        !ownerBindingsEqual(applyOwners, postOpenOwners)) {
        zeroize(bundleBytes)
        return { ok: false, code: !postOpenContext ? 'invalid_context' : 'invalid_request' }
      }
      applyContext = postOpenContext
      const requestForApply = Buffer.from(requestBytes)
      const applied = await callProtected({
        operation: 'apply-protected-recovery-bundle',
        bundleBytes,
        requestBytes: requestForApply,
        ownerBindingInventory: postOpenOwners
      })
      zeroize(requestForApply)
      if (!applied.ok || !applied.artifactBytes) {
        const continuation = await recoverReceiptContinuation(event, requestBytes, metadata, destinationReceipt)
        if (continuation.result) return applied.ok ? { ok: false, code: 'invalid_response' } : resultForRuntimeFailure(applied)
        if (!continuation.receiptBytes) return { ok: false, code: 'invalid_response' }
        const recovered = continuation.receiptBytes
        const wrote = await writeArtifact(continuation.context!.window, 'receipt', recovered, preflightTarget)
        return wrote ? { ok: true, status: 'receipt-created' } : { ok: false, code: 'cancelled' }
      }
      const receiptBytes = asBuffer(applied.artifactBytes)
      if (!validArtifact(receiptBytes, 'receipt')) {
        zeroize(receiptBytes)
        const continuation = await recoverReceiptContinuation(event, requestBytes, metadata, destinationReceipt)
        if (continuation.result) return { ok: false, code: 'invalid_response' }
        if (!continuation.receiptBytes) return { ok: false, code: 'invalid_response' }
        const recovered = continuation.receiptBytes
        const wrote = await writeArtifact(continuation.context!.window, 'receipt', recovered, preflightTarget)
        return wrote ? { ok: true, status: 'receipt-created' } : { ok: false, code: 'cancelled' }
      }
      const count = boundedEntryCount(applied.entryCount)
      if (count === null) {
        zeroize(receiptBytes)
        return { ok: false, code: 'invalid_response' }
      }
      const wrote = await writeArtifact(applyContext.window, 'receipt', receiptBytes, preflightTarget)
      if (wrote) return { ok: true, status: 'receipt-created', ...(typeof count === 'number' ? { entryCount: count } : {}) }
      const continuation = await recoverReceiptContinuation(event, requestBytes, metadata, destinationReceipt)
      if (continuation.result) return continuation.result
      if (!continuation.receiptBytes) return { ok: false, code: 'cancelled' }
      const recovered = continuation.receiptBytes
      const retryTarget = await targetFor('receipt')
      if (retryTarget === '' || (retryTarget && !await checkPath(retryTarget, 'save'))) {
        zeroize(recovered)
        return { ok: false, code: 'invalid_path' }
      }
      const retryWrote = await writeArtifact(continuation.context!.window, 'receipt', recovered, retryTarget)
      return retryWrote
        ? { ok: true, status: 'receipt-created', ...(typeof count === 'number' ? { entryCount: count } : {}) }
        : { ok: false, code: 'cancelled' }
    } finally {
      zeroize(bundleBytes)
      zeroize(requestBytes)
    }
  }

  const finalizeSourceReceipt = async (event: ProtectedRecoveryInvokeEvent): Promise<MainProviderCredentialRecoveryResult> => {
    const context = await localContext(event)
    if (!context) return { ok: false, code: 'invalid_context' }
    const receiptBytes = await readArtifact(context.window, 'receipt')
    if (!receiptBytes) return { ok: false, code: 'invalid_path' }
    try {
      let receiptValue: Record<string, unknown> | null = null
      try {
        receiptValue = objectValue(JSON.parse(receiptBytes.toString('utf8')))
      } catch {
        return { ok: false, code: 'invalid_response' }
      }
      const manifest = await currentPortableManifest()
      const inventory = await resolveInventory()
      const manifestDigest = receiptValue?.manifestDigest
      const selectedInventory = manifest && inventory ? inventoryForManifest(manifest, inventory) : null
      if (!manifest || typeof manifestDigest !== 'string' ||
        createHash('sha256').update(manifest).digest('hex') !== manifestDigest ||
        !selectedInventory || !await selectedInventoryIsCurrent(selectedInventory)) {
        return { ok: false, code: 'invalid_request' }
      }
      const finalizeContext = await localContext(event)
      if (!finalizeContext || !localBindingsEqual(context.binding, finalizeContext.binding)) {
        return { ok: false, code: 'invalid_context' }
      }
      const finalized = await callProtected({
        operation: 'finalize-protected-recovery-receipt',
        receiptBytes
      })
      if (!finalized.ok) return resultForRuntimeFailure(finalized)
      if (finalized.status !== 'finalized') return { ok: false, code: 'invalid_response' }
      const count = boundedEntryCount(finalized.entryCount)
      if (count === null) return { ok: false, code: 'invalid_response' }
      return { ok: true, status: 'finalized', ...(typeof count === 'number' ? { entryCount: count } : {}) }
    } finally {
      zeroize(receiptBytes)
    }
  }

  return { createDestinationRequest, createSourceBundle, applyDestinationBundle, finalizeSourceReceipt }
}
