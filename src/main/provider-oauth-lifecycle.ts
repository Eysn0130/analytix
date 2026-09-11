import { createHash, randomBytes, timingSafeEqual } from 'node:crypto'
import type {
  AccountCredentialDraftV1,
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1,
  AccountCredentialStateV1,
  ProviderRegistryExpectedStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  accountCredentialExpectedState,
  type MainAccountCredentialResolution
} from './ipc/provider-registry-ipc'

const PROVIDER_AUTHORIZATION_STATE_PURPOSE = 'provider-oauth-authorization-state' as const
const MCP_AUTHORIZATION_STATE_PURPOSE = 'mcp-oauth-authorization-state' as const
const EXTENSION_AUTHORIZATION_STATE_PURPOSE = 'extension-oauth-authorization-state' as const
const PROVIDER_TOKEN_PURPOSE = 'provider-oauth-token-bundle' as const
const MCP_TOKEN_PURPOSE = 'mcp-oauth-access-token' as const
const DEFAULT_OAUTH_AUTHORIZATION_TTL_MS = 10 * 60 * 1000
const DEFAULT_OAUTH_HTTP_TIMEOUT_MS = 10_000
const MAX_OAUTH_RESPONSE_BYTES = 64 * 1024
const MAX_OAUTH_RESPONSE_FIELDS = 64
const MAX_OAUTH_SCOPES = 32
const MAX_CURRENT_OAUTH_AUTHORIZATIONS = 32
const MAX_OAUTH_AUTHORIZATION_INVENTORY = 256

type OAuthAccountOwner = 'provider' | 'mcp' | 'extension'

export type OAuthAuthorizationBindingV1 = {
  owner: OAuthAccountOwner
  issuer: string
  authorizationEndpoint: string
  tokenEndpoint: string
  revocationEndpoint?: string
  clientId: string
  provider: string
  accountId: string
  channelId?: string
  redirectUri: string
  scopes: string[]
  bindingKey: string
  ownerFingerprint: string
  ownerRevision: string
  ownerGeneration: string
  ownerIncarnation: string
  profileBinding: string
}

type NormalizedOAuthBindingV1 = Omit<
  OAuthAuthorizationBindingV1,
  'owner' | 'revocationEndpoint' | 'channelId'
> & {
  owner: OAuthAccountOwner
  revocationEndpoint?: string
  channelId?: string
}

type StoredExpectedStateV1 = ProviderRegistryExpectedStateV1 | {
  registryRevision: string
  registryIncarnation: string
  providerRevision: '0'
  providerGeneration: '0'
  providerIncarnation: ''
  providerCredentialPurpose: ''
}

type OAuthTokenBundleV1 = {
  accessToken: string
  refreshToken?: string
  idToken?: string
  subscriptionToken?: string
  tokenType: 'Bearer'
  expiresAtMs?: number
  oauthBinding: NormalizedOAuthBindingV1
}

type OAuthAuthorizationRecordV1 = NormalizedOAuthBindingV1 & {
  schemaVersion: 1
  authorizationId: string
  state: string
  nonce: string
  codeVerifier: string
  expiresAtMs: number
  phase: 'pending' | 'exchanging' | 'token-candidate' | 'revoking'
  tokenExpected: StoredExpectedStateV1
  callbackCode?: string
  tokenCandidate?: OAuthTokenBundleV1
  initiatingWebContentsId: number
}

export type OAuthAuthorizationStartV1 = {
  authorizationId: string
  authorizationUrl: string
  expiresAtMs: number
}

export type OAuthAuthorizationPublicStatusV1 = {
  authorizationId: string
  expiresAtMs: number
  phase: 'pending' | 'processing'
}

export type OAuthNativeCallbackV1 = {
  callbackUrl: string
  profileBinding: string
}

export type OAuthAccountStatusV1 = {
  connected: true
  expiresAtMs?: number
  capabilities: { refresh: boolean; idToken: boolean; subscription: boolean }
}

export type ParsedOAuthNativeCallbackV1 = {
  bindingKey: string
  state: string
  code?: string
  issuer?: string
  error?: 'access_denied' | 'temporarily_unavailable' | 'server_error' | 'invalid_request'
}

export function parseOAuthNativeCallback(value: string): ParsedOAuthNativeCallbackV1 | null {
  if (!value || Buffer.byteLength(value, 'utf8') > 16 * 1024) return null
  try {
    const parsed = new URL(value)
    if (parsed.protocol !== 'com.analytix.desktop:' || parsed.host || parsed.username || parsed.password ||
      parsed.hash || parsed.pathname.includes('%')) return null
    const match = /^\/oauth\/callback\/(oauthb_[A-Za-z0-9_-]{43})$/.exec(parsed.pathname)
    if (!match) return null
    const allowed = new Set(['code', 'state', 'iss', 'error'])
    if ([...parsed.searchParams.keys()].some((key) => !allowed.has(key))) return null
    const values = (key: string) => parsed.searchParams.getAll(key)
    const state = values('state')
    const code = values('code')
    const issuer = values('iss')
    const error = values('error')
    const issuerValue = issuer[0]
    if (state.length !== 1 || !state[0] || state[0].length > 512 || issuer.length > 1 || error.length > 1 ||
      (code.length === 1) === (error.length === 1) || code.length > 1 ||
      (code.length === 1 && (!code[0] || code[0].length > 8192)) ||
      (issuerValue !== undefined && (
        !issuerValue || issuerValue !== issuerValue.trim() || Buffer.byteLength(issuerValue, 'utf8') > 2048 ||
        /[\u0000-\u001f\u007f]/.test(issuerValue)
      ))) return null
    const errorValue = error[0]
    if (errorValue !== undefined && ![
      'access_denied', 'temporarily_unavailable', 'server_error', 'invalid_request'
    ].includes(errorValue)) return null
    return {
      bindingKey: match[1]!, state: state[0],
      ...(code[0] ? { code: code[0] } : {}),
      ...(issuerValue !== undefined ? { issuer: issuerValue } : {}),
      ...(errorValue ? { error: errorValue as ParsedOAuthNativeCallbackV1['error'] } : {})
    }
  } catch {
    return null
  }
}

type OAuthLifecycleDeps = {
  requestAccountCredential: (request: AccountCredentialRequestV1) => Promise<AccountCredentialResultV1>
  resolveAccountCredential: (scope: AccountCredentialScopeV1) => Promise<MainAccountCredentialResolution>
  authorizeBinding: (binding: OAuthAuthorizationBindingV1) => Promise<boolean>
  listAccountCredentialStates?: (
    owner: 'provider' | 'mcp' | 'extension',
    purpose: 'provider-oauth-authorization-state' | 'provider-oauth-token-bundle' |
      'mcp-oauth-authorization-state' | 'mcp-oauth-access-token' |
      'extension-oauth-authorization-state' | 'extension-provider-account-token'
  ) => Promise<AccountCredentialStateV1[]>
  fetch?: (input: string, init: RequestInit) => Promise<Response>
  now?: () => number
  ttlMs?: number
  httpTimeoutMs?: number
  isInitiatingAuthorityCurrent?: (webContentsId: number, profileBinding: string) => boolean
  onRefreshInventoryChanged?: () => void
  crash?: {
    afterCallbackClaimed?: () => Promise<void> | void
    afterTokenCandidateStored?: () => Promise<void> | void
    afterTokenCommitted?: () => Promise<void> | void
    afterRemoteRevocation?: () => Promise<void> | void
  }
}

type OAuthRefreshInventoryV1 = {
  refreshed: number
  revokedExpired: number
  removedUnbound: number
  retained: number
  nextRefreshAtMs?: number
}

type OAuthRefreshSchedulerDeps = {
  refreshInventory: (refreshWindowMs?: number) => Promise<OAuthRefreshInventoryV1>
  now?: () => number
  refreshWindowMs?: number
  maxRevalidationMs?: number
  setTimer?: (callback: () => Promise<void>, delayMs: number) => unknown
  clearTimer?: (timer: unknown) => void
  onError?: () => void
}

function bounded(value: string, maximum: number): string {
  const result = value.trim()
  if (!result || Buffer.byteLength(result, 'utf8') > maximum || /[\u0000\r\n\t]/.test(result)) {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  return result
}

function isLoopback(hostname: string): boolean {
  const normalized = hostname.toLowerCase().replace(/^\[|\]$/g, '')
  return normalized === '127.0.0.1' || normalized === '::1'
}

function fixedUrl(value: string): URL {
  const source = bounded(value, 2048)
  let parsed: URL
  try {
    parsed = new URL(source)
  } catch {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
  const loopbackHTTP = parsed.protocol === 'http:' && isLoopback(hostname)
  const exactLoopbackHTTP = /^http:\/\/(?:127\.0\.0\.1(?::[0-9]+)?|\[::1\](?::[0-9]+)?)(?:\/|$)/.test(source)
  const forbiddenHost = hostname === 'localhost' || hostname.endsWith('.localhost') ||
    /^10\./.test(hostname) || /^192\.168\./.test(hostname) ||
    /^172\.(?:1[6-9]|2[0-9]|3[01])\./.test(hostname) || /^169\.254\./.test(hostname) ||
    /^(?:fc|fd|fe8|fe9|fea|feb)/i.test(hostname)
  if (parsed.username || parsed.password || parsed.search || parsed.hash || parsed.pathname.includes('%') ||
    forbiddenHost || (parsed.protocol !== 'https:' && !(loopbackHTTP && exactLoopbackHTTP)) || source.includes('\\')) {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  return parsed
}

function nativeRedirectUri(bindingKey: string): string {
  const key = bounded(bindingKey, 64)
  if (!/^oauthb_[A-Za-z0-9_-]{43}$/.test(key)) throw new Error('OAuth account lifecycle request is invalid.')
  return `com.analytix.desktop:/oauth/callback/${key}`
}

function normalizeBinding(input: OAuthAuthorizationBindingV1): NormalizedOAuthBindingV1 {
  const owner = input.owner
  const issuer = fixedUrl(input.issuer)
  const authorizationEndpoint = fixedUrl(input.authorizationEndpoint)
  const tokenEndpoint = fixedUrl(input.tokenEndpoint)
  const redirectUri = nativeRedirectUri(input.bindingKey)
  const revocationEndpoint = input.revocationEndpoint ? fixedUrl(input.revocationEndpoint) : undefined
  if (input.redirectUri !== redirectUri) {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  const channelId = input.channelId?.trim() || undefined
  if ((owner !== 'provider') !== Boolean(channelId)) throw new Error('OAuth account lifecycle request is invalid.')
  const scopes = input.scopes.map((scope) => bounded(scope, 256))
  if (!scopes.length || scopes.length > MAX_OAUTH_SCOPES || new Set(scopes).size !== scopes.length) {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  if (!/^[a-f0-9]{64}$/.test(input.ownerFingerprint) || !/^[a-f0-9]{64}$/.test(input.profileBinding) ||
    !/^(?:0|[1-9][0-9]*)$/.test(input.ownerRevision) || !/^(?:0|[1-9][0-9]*)$/.test(input.ownerGeneration)) {
    throw new Error('OAuth account lifecycle request is invalid.')
  }
  return {
    owner,
    issuer: issuer.toString(),
    authorizationEndpoint: authorizationEndpoint.toString(),
    tokenEndpoint: tokenEndpoint.toString(),
    ...(revocationEndpoint ? { revocationEndpoint: revocationEndpoint.toString() } : {}),
    clientId: bounded(input.clientId, 256),
    provider: bounded(input.provider, 128),
    accountId: bounded(input.accountId, 256),
    ...(channelId ? { channelId: bounded(channelId, 128) } : {}),
    redirectUri,
    scopes,
    bindingKey: bounded(input.bindingKey, 64),
    ownerFingerprint: bounded(input.ownerFingerprint, 64),
    ownerRevision: bounded(input.ownerRevision, 20),
    ownerGeneration: bounded(input.ownerGeneration, 20),
    ownerIncarnation: bounded(input.ownerIncarnation, 256),
    profileBinding: bounded(input.profileBinding, 64)
  }
}

function authorizationScope(binding: NormalizedOAuthBindingV1, authorizationId: string): AccountCredentialScopeV1 {
  if (binding.owner === 'provider') {
    return {
        owner: 'provider', provider: binding.provider, accountId: binding.accountId,
        channelId: authorizationId, purpose: PROVIDER_AUTHORIZATION_STATE_PURPOSE
      }
  }
  if (binding.owner === 'mcp') {
    return {
        owner: 'mcp', provider: binding.provider, accountId: binding.accountId,
        channelId: `${binding.channelId}:${authorizationId}`, purpose: MCP_AUTHORIZATION_STATE_PURPOSE
      }
  }
  return {
    owner: 'extension', provider: binding.provider, accountId: binding.accountId,
    channelId: `${binding.channelId}:${authorizationId}`, purpose: EXTENSION_AUTHORIZATION_STATE_PURPOSE
  }
}

function tokenScope(binding: NormalizedOAuthBindingV1): AccountCredentialScopeV1 {
  if (binding.owner === 'provider') {
    return { owner: 'provider', provider: binding.provider, accountId: binding.accountId, purpose: PROVIDER_TOKEN_PURPOSE }
  }
  if (binding.owner === 'mcp') {
    return {
        owner: 'mcp', provider: binding.provider, accountId: binding.accountId,
        channelId: binding.channelId!, purpose: MCP_TOKEN_PURPOSE
      }
  }
  return {
    owner: 'extension', provider: binding.provider, accountId: binding.accountId,
    channelId: binding.channelId!, purpose: 'extension-provider-account-token'
  }
}

function randomOpaque(bytes = 32): string {
  return randomBytes(bytes).toString('base64url')
}

function constantTimeEqual(left: string, right: string): boolean {
  const leftBytes = Buffer.from(left, 'utf8')
  const rightBytes = Buffer.from(right, 'utf8')
  try {
    return leftBytes.length === rightBytes.length && timingSafeEqual(leftBytes, rightBytes)
  } finally {
    leftBytes.fill(0)
    rightBytes.fill(0)
  }
}

function bindingsEqual(left: NormalizedOAuthBindingV1, right: NormalizedOAuthBindingV1): boolean {
  return left.owner === right.owner && left.issuer === right.issuer &&
    left.authorizationEndpoint === right.authorizationEndpoint && left.tokenEndpoint === right.tokenEndpoint &&
    left.revocationEndpoint === right.revocationEndpoint && left.clientId === right.clientId &&
    left.provider === right.provider && left.accountId === right.accountId && left.channelId === right.channelId &&
    left.redirectUri === right.redirectUri && left.scopes.length === right.scopes.length &&
    left.scopes.every((scope, index) => scope === right.scopes[index]) &&
    left.bindingKey === right.bindingKey && left.ownerFingerprint === right.ownerFingerprint &&
    left.ownerRevision === right.ownerRevision && left.ownerGeneration === right.ownerGeneration &&
    left.ownerIncarnation === right.ownerIncarnation && left.profileBinding === right.profileBinding
}

export function oauthAuthorizationBindingsEqual(
  left: OAuthAuthorizationBindingV1,
  right: OAuthAuthorizationBindingV1
): boolean {
  try {
    return bindingsEqual(normalizeBinding(left), normalizeBinding(right))
  } catch {
    return false
  }
}

function expected(state: AccountCredentialStateV1): StoredExpectedStateV1 {
  return state.status === 'absent'
    ? {
        registryRevision: state.registryRevision, registryIncarnation: state.registryIncarnation,
        providerRevision: '0', providerGeneration: '0', providerIncarnation: '', providerCredentialPurpose: ''
      }
    : accountCredentialExpectedState(state)
}

function successorProviderBundle(
  bundle: OAuthTokenBundleV1,
  prior: StoredExpectedStateV1
): OAuthTokenBundleV1 {
  if (bundle.oauthBinding.owner !== 'provider') return bundle
  const binding = bundle.oauthBinding
  if (prior.providerRevision === '0' || prior.providerGeneration === '0' || !prior.providerIncarnation ||
    binding.ownerRevision !== prior.providerRevision || binding.ownerGeneration !== prior.providerGeneration ||
    binding.ownerIncarnation !== prior.providerIncarnation) {
    throw new Error('OAuth token commit lost its current-generation fence.')
  }
  const ownerRevision = (BigInt(prior.providerRevision) + 1n).toString()
  const ownerGeneration = (BigInt(prior.providerGeneration) + 1n).toString()
  return {
    ...bundle,
    oauthBinding: {
      ...binding,
      ownerRevision,
      ownerGeneration,
      ownerIncarnation: prior.providerIncarnation
    }
  }
}

function sameTokenFence(state: AccountCredentialStateV1, prior: StoredExpectedStateV1): boolean {
  return state.registryIncarnation === prior.registryIncarnation &&
    state.providerRevision === prior.providerRevision &&
    state.providerGeneration === prior.providerGeneration &&
    state.providerIncarnation === prior.providerIncarnation &&
    (state.credentialPurpose ?? (state.status === 'ready' ? state.scope.purpose : '')) ===
      prior.providerCredentialPurpose
}

function authorizationDraft(owner: OAuthAccountOwner, record: OAuthAuthorizationRecordV1): AccountCredentialDraftV1 {
  const token = JSON.stringify(record)
  if (owner === 'provider') return { kind: 'oauth-token', token }
  return owner === 'mcp' ? { kind: 'mcp-oauth-token', token } : { kind: 'extension-oauth-token', token }
}

function tokenDraft(owner: OAuthAccountOwner, bundle: OAuthTokenBundleV1): AccountCredentialDraftV1 {
  if (owner === 'provider') return { kind: 'provider-oauth-bundle', ...bundle }
  if (owner === 'mcp') {
    return {
        kind: 'mcp-oauth-bundle', accessToken: bundle.accessToken,
        ...(bundle.refreshToken ? { refreshToken: bundle.refreshToken } : {}),
        tokenType: 'Bearer', ...(bundle.expiresAtMs ? { expiresAtMs: bundle.expiresAtMs } : {}),
        oauthBinding: bundle.oauthBinding
      }
  }
  return {
    kind: 'extension-oauth-bundle', accessToken: bundle.accessToken,
    ...(bundle.refreshToken ? { refreshToken: bundle.refreshToken } : {}),
    tokenType: 'Bearer', ...(bundle.expiresAtMs ? { expiresAtMs: bundle.expiresAtMs } : {}),
    oauthBinding: bundle.oauthBinding
  }
}

function parseAuthorization(
  binding: NormalizedOAuthBindingV1,
  authorizationId: string,
  resolution: MainAccountCredentialResolution
): { state: AccountCredentialStateV1; record: OAuthAuthorizationRecordV1 } | null {
  if (!resolution.ok ||
    (binding.owner === 'provider' && resolution.credential.kind !== 'oauth-token') ||
    (binding.owner === 'mcp' && resolution.credential.kind !== 'mcp-oauth-token') ||
    (binding.owner === 'extension' && resolution.credential.kind !== 'extension-oauth-token')) return null
  try {
    const token = resolution.credential.kind === 'oauth-token' ||
      resolution.credential.kind === 'mcp-oauth-token' || resolution.credential.kind === 'extension-oauth-token'
      ? resolution.credential.token
      : ''
    if (!token) return null
    const record = JSON.parse(token) as OAuthAuthorizationRecordV1
    if (record.schemaVersion !== 1 || record.authorizationId !== authorizationId || !bindingsEqual(record, binding) ||
      !['pending', 'exchanging', 'token-candidate', 'revoking'].includes(record.phase) ||
      !Number.isSafeInteger(record.expiresAtMs) || !record.state || !record.nonce || !record.codeVerifier) return null
    return { state: resolution.state, record }
  } catch {
    return null
  }
}

function parseBundle(
  binding: NormalizedOAuthBindingV1,
  resolution: MainAccountCredentialResolution
): { state: AccountCredentialStateV1; bundle: OAuthTokenBundleV1 } | null {
  if (!resolution.ok) return null
  if (binding.owner === 'provider' && resolution.credential.kind === 'provider-oauth-bundle') {
    const { kind: _, ...bundle } = resolution.credential
    if (!bindingsEqual(bundle.oauthBinding, binding) ||
      binding.ownerRevision !== resolution.state.providerRevision ||
      binding.ownerGeneration !== resolution.state.providerGeneration ||
      binding.ownerIncarnation !== resolution.state.providerIncarnation) return null
    return { state: resolution.state, bundle }
  }
  if (binding.owner === 'mcp' && resolution.credential.kind === 'mcp-oauth-bundle') {
    const { kind: _, ...bundle } = resolution.credential
    if (!bindingsEqual(bundle.oauthBinding, binding)) return null
    return { state: resolution.state, bundle }
  }
  if (binding.owner === 'extension' && resolution.credential.kind === 'extension-oauth-bundle') {
    const { kind: _, ...bundle } = resolution.credential
    if (!bindingsEqual(bundle.oauthBinding, binding)) return null
    return { state: resolution.state, bundle }
  }
  return null
}

async function boundedJSON(response: Response): Promise<Record<string, unknown>> {
  const contentLength = Number(response.headers.get('content-length') ?? '0')
  if (Number.isFinite(contentLength) && contentLength > MAX_OAUTH_RESPONSE_BYTES) {
    throw new Error('OAuth provider response was rejected.')
  }
  const chunks: Uint8Array[] = []
  let total = 0
  if (response.body) {
    const reader = response.body.getReader()
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        total += value.byteLength
        if (total > MAX_OAUTH_RESPONSE_BYTES) throw new Error('OAuth provider response was rejected.')
        chunks.push(value)
      }
    } finally {
      await reader.cancel().catch(() => undefined)
    }
  }
  const bytes = Buffer.concat(chunks.map((chunk) => Buffer.from(chunk)), total)
  for (const chunk of chunks) chunk.fill(0)
  try {
    if (bytes.length === 0) return {}
    const parsed = JSON.parse(bytes.toString('utf8')) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed) ||
      Object.keys(parsed).length > MAX_OAUTH_RESPONSE_FIELDS) throw new Error()
    return parsed as Record<string, unknown>
  } catch {
    throw new Error('OAuth provider response was rejected.')
  } finally {
    bytes.fill(0)
  }
}

function responseString(record: Record<string, unknown>, key: string, maximum = 32 * 1024): string | undefined {
  const value = record[key]
  if (value === undefined) return undefined
  if (typeof value !== 'string' || !value || value !== value.trim() || Buffer.byteLength(value, 'utf8') > maximum ||
    /[\u0000\r\n]/.test(value)) throw new Error('OAuth provider response was rejected.')
  return value
}

function validateIDTokenBinding(
  value: string,
  binding: NormalizedOAuthBindingV1,
  nowMs: number,
  expectedNonce?: string
): void {
  const parts = value.split('.')
  if (parts.length !== 3 || parts.some((part) => !part || !/^[A-Za-z0-9_-]+$/.test(part))) {
    throw new Error('OAuth provider response was rejected.')
  }
  try {
    const headerBytes = Buffer.from(parts[0]!, 'base64url')
    if (headerBytes.length === 0 || headerBytes.length > 4096) throw new Error()
    try {
      const header = JSON.parse(headerBytes.toString('utf8')) as unknown
      if (!header || typeof header !== 'object' || Array.isArray(header) ||
        !['RS256', 'ES256'].includes(String((header as Record<string, unknown>).alg ?? ''))) throw new Error()
    } finally {
      headerBytes.fill(0)
    }
  } catch {
    throw new Error('OAuth provider response was rejected.')
  }
  let claims: Record<string, unknown>
  try {
    const bytes = Buffer.from(parts[1]!, 'base64url')
    if (bytes.length === 0 || bytes.length > 16 * 1024) throw new Error()
    try {
      const parsed = JSON.parse(bytes.toString('utf8')) as unknown
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error()
      claims = parsed as Record<string, unknown>
    } finally {
      bytes.fill(0)
    }
  } catch {
    throw new Error('OAuth provider response was rejected.')
  }
  const audience = claims.aud
  const audienceMatches = audience === binding.clientId ||
    (Array.isArray(audience) && audience.length > 0 && audience.length <= 16 &&
      audience.every((entry) => typeof entry === 'string') && audience.includes(binding.clientId))
  const expiration = claims.exp
  if (claims.iss !== binding.issuer || !audienceMatches ||
    !Number.isSafeInteger(expiration) || Number(expiration) * 1000 <= nowMs ||
    (expectedNonce !== undefined && claims.nonce !== expectedNonce) ||
    (expectedNonce === undefined && claims.nonce !== undefined)) {
    throw new Error('OAuth provider response was rejected.')
  }
}

function bundleFromResponse(
  record: Record<string, unknown>,
  now: number,
  binding: NormalizedOAuthBindingV1,
  prior?: OAuthTokenBundleV1,
  expectedNonce?: string
): OAuthTokenBundleV1 {
  const accessToken = responseString(record, 'access_token')
  const tokenType = responseString(record, 'token_type', 64)
  const refreshToken = responseString(record, 'refresh_token') ?? prior?.refreshToken
  const returnedIDToken = responseString(record, 'id_token')
  if (returnedIDToken) validateIDTokenBinding(returnedIDToken, binding, now, expectedNonce)
  const idToken = returnedIDToken ?? prior?.idToken
  const subscriptionToken = responseString(record, 'subscription_token') ?? prior?.subscriptionToken
  const expiresIn = record.expires_in
  if (!accessToken || tokenType?.toLowerCase() !== 'bearer' ||
    (expiresIn !== undefined && (!Number.isSafeInteger(expiresIn) || Number(expiresIn) <= 0 || Number(expiresIn) > 31_536_000))) {
    throw new Error('OAuth provider response was rejected.')
  }
  return {
    accessToken,
    ...(refreshToken ? { refreshToken } : {}), ...(idToken ? { idToken } : {}),
    ...(subscriptionToken ? { subscriptionToken } : {}), tokenType: 'Bearer',
    oauthBinding: normalizeBinding(binding),
    ...(expiresIn === undefined ? {} : { expiresAtMs: now + Number(expiresIn) * 1000 })
  }
}

function publicStatus(bundle: OAuthTokenBundleV1): OAuthAccountStatusV1 {
  return {
    connected: true,
    ...(bundle.expiresAtMs ? { expiresAtMs: bundle.expiresAtMs } : {}),
    capabilities: {
      refresh: Boolean(bundle.refreshToken), idToken: Boolean(bundle.idToken),
      subscription: Boolean(bundle.subscriptionToken)
    }
  }
}

function bundlesEqual(left: OAuthTokenBundleV1, right: OAuthTokenBundleV1): boolean {
  return constantTimeEqual(JSON.stringify([
    left.accessToken, left.refreshToken ?? '', left.idToken ?? '', left.subscriptionToken ?? '',
    left.tokenType, left.expiresAtMs ?? 0
  ]), JSON.stringify([
    right.accessToken, right.refreshToken ?? '', right.idToken ?? '', right.subscriptionToken ?? '',
    right.tokenType, right.expiresAtMs ?? 0
  ])) && bindingsEqual(left.oauthBinding, right.oauthBinding)
}

function bundleForOwner(owner: OAuthAccountOwner, bundle: OAuthTokenBundleV1): OAuthTokenBundleV1 {
  return owner === 'provider'
    ? bundle
    : {
        accessToken: bundle.accessToken,
        ...(bundle.refreshToken ? { refreshToken: bundle.refreshToken } : {}),
        tokenType: 'Bearer',
        oauthBinding: bundle.oauthBinding,
        ...(bundle.expiresAtMs ? { expiresAtMs: bundle.expiresAtMs } : {})
      }
}

export function createProviderOAuthRefreshScheduler(deps: OAuthRefreshSchedulerDeps) {
  const now = deps.now ?? Date.now
  const refreshWindowMs = deps.refreshWindowMs ?? 5 * 60 * 1000
  const maxRevalidationMs = deps.maxRevalidationMs ?? 60_000
  const setTimer = deps.setTimer ?? ((callback, delayMs) => {
    const timer = setTimeout(() => { void callback() }, delayMs)
    timer.unref()
    return timer
  })
  const clearTimer = deps.clearTimer ?? ((timer) => clearTimeout(timer as ReturnType<typeof setTimeout>))
  if (!Number.isSafeInteger(refreshWindowMs) || refreshWindowMs < 0 ||
    refreshWindowMs > 30 * 60 * 1000 || !Number.isSafeInteger(maxRevalidationMs) ||
    maxRevalidationMs < 1_000 || maxRevalidationMs > 5 * 60 * 1000) {
    throw new Error('OAuth account refresh scheduling is unavailable.')
  }

  let active = false
  let epoch = 0
  let pendingTimer: unknown
  const cancelTimer = (): void => {
    if (pendingTimer === undefined) return
    clearTimer(pendingTimer)
    pendingTimer = undefined
  }
  const scan = async (expectedEpoch: number): Promise<void> => {
    cancelTimer()
    let inventory: OAuthRefreshInventoryV1
    try {
      inventory = await deps.refreshInventory(refreshWindowMs)
    } catch {
      if (active && epoch === expectedEpoch) deps.onError?.()
      return
    }
    if (!active || epoch !== expectedEpoch || inventory.nextRefreshAtMs === undefined) return
    if (!Number.isSafeInteger(inventory.nextRefreshAtMs) || inventory.nextRefreshAtMs <= 0) {
      deps.onError?.()
      return
    }
    const currentTimeMs = now()
    if (!Number.isSafeInteger(currentTimeMs) || currentTimeMs < 0) {
      deps.onError?.()
      return
    }
    const delayMs = Math.max(1_000, Math.min(maxRevalidationMs, inventory.nextRefreshAtMs - currentTimeMs))
    pendingTimer = setTimer(async () => {
      pendingTimer = undefined
      await scan(expectedEpoch)
    }, delayMs)
  }

  return {
    async start(): Promise<void> {
      cancelTimer()
      active = true
      const expectedEpoch = ++epoch
      await scan(expectedEpoch)
    },
    stop(): void {
      active = false
      epoch++
      cancelTimer()
    },
    async rescan(): Promise<void> {
      if (!active) return
      cancelTimer()
      const expectedEpoch = ++epoch
      await scan(expectedEpoch)
    }
  }
}

export function createProviderOAuthAuthorizationLifecycle(deps: OAuthLifecycleDeps) {
  const now = deps.now ?? Date.now
  const ttlMs = deps.ttlMs ?? DEFAULT_OAUTH_AUTHORIZATION_TTL_MS
  const timeoutMs = deps.httpTimeoutMs ?? DEFAULT_OAUTH_HTTP_TIMEOUT_MS
  const fetchImpl = deps.fetch ?? globalThis.fetch
  if (!Number.isSafeInteger(ttlMs) || ttlMs < 30_000 || ttlMs > 30 * 60 * 1000 ||
    !Number.isSafeInteger(timeoutMs) || timeoutMs < 100 || timeoutMs > 30_000 || typeof fetchImpl !== 'function') {
    throw new Error('OAuth account lifecycle configuration is invalid.')
  }

  const bindingIsAuthorized = async (binding: NormalizedOAuthBindingV1): Promise<boolean> => {
    try {
      return await deps.authorizeBinding(binding)
    } catch {
      return false
    }
  }
  const notifyRefreshInventoryChanged = (): void => {
    try {
      deps.onRefreshInventoryChanged?.()
    } catch {
      // Refresh scheduling is advisory; protected mutations remain authoritative.
    }
  }

  const requestForm = async (endpoint: string, form: Record<string, string>): Promise<Record<string, unknown>> => {
    let response: Response
    try {
      response = await fetchImpl(endpoint, {
        method: 'POST', redirect: 'error',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', Accept: 'application/json' },
        body: new URLSearchParams(form).toString(), signal: AbortSignal.timeout(timeoutMs)
      })
    } catch {
      throw new Error('OAuth provider request failed.')
    }
    if (!response.ok || response.status < 200 || response.status >= 300) {
      await response.body?.cancel().catch(() => undefined)
      throw new Error('OAuth provider request failed.')
    }
    return boundedJSON(response)
  }

  const putRecord = async (
    scope: AccountCredentialScopeV1,
    expectedState: StoredExpectedStateV1,
    record: OAuthAuthorizationRecordV1
  ): Promise<AccountCredentialStateV1> => {
    const result = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'put', scope, expected: expectedState,
      credential: authorizationDraft(record.owner, record)
    })
    if ('error' in result || result.status !== 'ready') throw new Error('OAuth authorization state is unavailable.')
    return result
  }

  const removeExactCommittedCandidate = async (
    scope: AccountCredentialScopeV1,
    committed: AccountCredentialStateV1
  ): Promise<void> => {
    const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
    if ('error' in current) throw new Error('OAuth stale token cleanup could not be verified.')
    if (current.status === 'absent' ||
      !sameTokenFence(current, accountCredentialExpectedState(committed))) return
    const deleted = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'delete', scope,
      expected: accountCredentialExpectedState(current)
    })
    if ('error' in deleted || deleted.status !== 'absent') {
      throw new Error('OAuth stale token cleanup could not be verified.')
    }
  }

  const commitCandidate = async (
    record: OAuthAuthorizationRecordV1,
    authorizationState: AccountCredentialStateV1
  ): Promise<OAuthAccountStatusV1> => {
    if (!record.tokenCandidate) throw new Error('OAuth authorization recovery was rejected.')
    const scope = tokenScope(record)
    const committedCandidate = successorProviderBundle(record.tokenCandidate, record.tokenExpected)
    const current = parseBundle(committedCandidate.oauthBinding, await deps.resolveAccountCredential(scope))
    let committed: AccountCredentialStateV1
    if (current && bundlesEqual(current.bundle, committedCandidate)) {
      committed = current.state
      if (!await bindingIsAuthorized(committedCandidate.oauthBinding)) {
        await removeExactCommittedCandidate(scope, committed)
        throw new Error('OAuth account binding is unavailable.')
      }
    } else {
      if (!await bindingIsAuthorized(record)) {
        throw new Error('OAuth account binding is unavailable.')
      }
      const currentState = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
      if ('error' in currentState || !sameTokenFence(currentState, record.tokenExpected)) {
        throw new Error('OAuth token commit lost its current-generation fence.')
      }
      if (!await bindingIsAuthorized(record)) {
        throw new Error('OAuth account binding is unavailable.')
      }
      const result = await deps.requestAccountCredential({
        schemaVersion: 1, operation: 'put', scope, expected: expected(currentState),
        credential: tokenDraft(record.owner, committedCandidate)
      })
      if ('error' in result || result.status !== 'ready') {
        throw new Error('OAuth token commit lost its current-generation fence.')
      }
      committed = result
    }
    const verified = parseBundle(committedCandidate.oauthBinding, await deps.resolveAccountCredential(scope))
    if (!verified || verified.state.providerRevision !== committed.providerRevision ||
      verified.state.providerGeneration !== committed.providerGeneration ||
      verified.state.providerIncarnation !== committed.providerIncarnation ||
      !bundlesEqual(verified.bundle, committedCandidate)) {
      throw new Error('OAuth token commit could not be verified.')
    }
    if (!await bindingIsAuthorized(committedCandidate.oauthBinding)) {
      await removeExactCommittedCandidate(scope, committed)
      throw new Error('OAuth account binding is unavailable.')
    }
    await deps.crash?.afterTokenCommitted?.()
    const authorizationScopeValue = authorizationScope(record, record.authorizationId)
    const authorizationCurrent = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'status', scope: authorizationScopeValue
    })
    if ('error' in authorizationCurrent || !sameTokenFence(authorizationCurrent, accountCredentialExpectedState(authorizationState))) {
      throw new Error('OAuth authorization cleanup could not be verified.')
    }
    const deleted = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'delete', scope: authorizationScopeValue,
      expected: accountCredentialExpectedState(authorizationCurrent)
    })
    if ('error' in deleted || deleted.status !== 'absent') {
      throw new Error('OAuth authorization cleanup could not be verified.')
    }
    notifyRefreshInventoryChanged()
    return publicStatus(committedCandidate)
  }

  const exchangeAndCommit = async (
    record: OAuthAuthorizationRecordV1,
    authorizationState: AccountCredentialStateV1
  ): Promise<OAuthAccountStatusV1> => {
    if (record.phase === 'token-candidate') return commitCandidate(record, authorizationState)
    if (record.phase !== 'exchanging' || !record.callbackCode) {
      throw new Error('OAuth authorization recovery was rejected.')
    }
    const response = await requestForm(record.tokenEndpoint, {
      grant_type: 'authorization_code', code: record.callbackCode, client_id: record.clientId,
      redirect_uri: record.redirectUri, code_verifier: record.codeVerifier
    })
    const candidateRecord: OAuthAuthorizationRecordV1 = {
      ...record, phase: 'token-candidate', callbackCode: undefined,
      tokenCandidate: bundleForOwner(record.owner, bundleFromResponse(response, now(), record, undefined, record.nonce))
    }
    if (!await bindingIsAuthorized(record)) {
      throw new Error('OAuth account binding is unavailable.')
    }
    const candidateState = await putRecord(
      authorizationScope(record, record.authorizationId),
      accountCredentialExpectedState(authorizationState), candidateRecord
    )
    if (!await bindingIsAuthorized(record)) {
      await deleteKnownAuthorizationState(
        authorizationScope(record, record.authorizationId), candidateState
      )
      throw new Error('OAuth account binding is unavailable.')
    }
    await deps.crash?.afterTokenCandidateStored?.()
    return commitCandidate(candidateRecord, candidateState)
  }

  const deleteKnownAuthorizationState = async (
    scope: AccountCredentialScopeV1,
    known: AccountCredentialStateV1
  ): Promise<void> => {
    const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
    if ('error' in current) throw new Error('OAuth authorization cleanup could not be verified.')
    if (current.status === 'absent') return
    if (!sameTokenFence(current, accountCredentialExpectedState(known))) {
      throw new Error('OAuth authorization cleanup lost its current-generation fence.')
    }
    const deleted = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'delete', scope, expected: accountCredentialExpectedState(current)
    })
    if ('error' in deleted || deleted.status !== 'absent') {
      throw new Error('OAuth authorization cleanup could not be verified.')
    }
  }

  const listAuthorizationStates = async (): Promise<AccountCredentialStateV1[]> => {
    if (!deps.listAccountCredentialStates) throw new Error('OAuth authorization inventory is unavailable.')
    const states = [
      ...await deps.listAccountCredentialStates('provider', PROVIDER_AUTHORIZATION_STATE_PURPOSE),
      ...await deps.listAccountCredentialStates('mcp', MCP_AUTHORIZATION_STATE_PURPOSE),
      ...await deps.listAccountCredentialStates('extension', EXTENSION_AUTHORIZATION_STATE_PURPOSE)
    ]
    if (states.length > MAX_OAUTH_AUTHORIZATION_INVENTORY) {
      throw new Error('OAuth authorization inventory is unavailable.')
    }
    return states
  }

  const resolvedAuthorizationRecord = async (known: AccountCredentialStateV1): Promise<{
    state: AccountCredentialStateV1
    record: OAuthAuthorizationRecordV1
  } | null> => {
    if (known.status !== 'ready') return null
    const resolution = await deps.resolveAccountCredential(known.scope)
    if (!resolution.ok ||
      (known.scope.owner === 'provider' && resolution.credential.kind !== 'oauth-token') ||
      (known.scope.owner === 'mcp' && resolution.credential.kind !== 'mcp-oauth-token') ||
      (known.scope.owner === 'extension' && resolution.credential.kind !== 'extension-oauth-token')) return null
    try {
      const token = resolution.credential.kind === 'oauth-token' ||
        resolution.credential.kind === 'mcp-oauth-token' ||
        resolution.credential.kind === 'extension-oauth-token'
        ? resolution.credential.token
        : ''
      const record = JSON.parse(token) as OAuthAuthorizationRecordV1
      const binding = normalizeBinding(record)
      const parsed = parseAuthorization(binding, record.authorizationId, resolution)
      if (!parsed || !Number.isSafeInteger(record.initiatingWebContentsId) || record.initiatingWebContentsId <= 0 ||
        JSON.stringify(authorizationScope(binding, record.authorizationId)) !== JSON.stringify(known.scope)) return null
      return { state: parsed.state, record: parsed.record }
    } catch {
      return null
    }
  }

  const findAuthorizationRecords = async (predicate: (record: OAuthAuthorizationRecordV1) => boolean) => {
    const matches: Array<{ state: AccountCredentialStateV1; record: OAuthAuthorizationRecordV1 }> = []
    for (const known of await listAuthorizationStates()) {
      const parsed = await resolvedAuthorizationRecord(known)
      if (parsed && predicate(parsed.record)) matches.push(parsed)
    }
    return matches
  }

  const cleanupAuthorizationInventoryForAdmission = async (): Promise<number> => {
    let currentCount = 0
    for (const known of await listAuthorizationStates()) {
      const parsed = await resolvedAuthorizationRecord(known)
      if (!parsed) {
        await deleteKnownAuthorizationState(known.scope, known)
        continue
      }
      const authorized = await bindingIsAuthorized(parsed.record)
      const pendingAuthorityCurrent = parsed.record.phase !== 'pending' || (
        now() <= parsed.record.expiresAtMs &&
        Boolean(deps.isInitiatingAuthorityCurrent?.(
          parsed.record.initiatingWebContentsId,
          parsed.record.profileBinding
        ))
      )
      if (!authorized || !pendingAuthorityCurrent) {
        await deleteKnownAuthorizationState(known.scope, parsed.state)
        continue
      }
      currentCount++
    }
    return currentCount
  }

  let admissionTail = Promise.resolve()
  const withAuthorizationAdmission = async <T>(action: () => Promise<T>): Promise<T> => {
    const prior = admissionTail
    let release!: () => void
    admissionTail = new Promise<void>((resolve) => { release = resolve })
    await prior
    try {
      return await action()
    } finally {
      release()
    }
  }

  const finishRevocation = async (
    record: OAuthAuthorizationRecordV1,
    journalState: AccountCredentialStateV1
  ): Promise<{ revoked: true }> => {
    if (record.phase !== 'revoking' || !record.revocationEndpoint) {
      throw new Error('OAuth revocation recovery was rejected.')
    }
    const scope = tokenScope(record)
    const currentState = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
    if ('error' in currentState) throw new Error('OAuth revocation recovery was rejected.')
    if (currentState.status !== 'ready') {
      await deleteKnownAuthorizationState(authorizationScope(record, record.authorizationId), journalState)
      return { revoked: true }
    }
    if (!sameTokenFence(currentState, record.tokenExpected)) {
      // A newer user replacement wins. The old K1 candidate has already been
      // retired by that mutation, so the journal must not revoke the successor.
      await deleteKnownAuthorizationState(authorizationScope(record, record.authorizationId), journalState)
      return { revoked: true }
    }
    const current = parseBundle(record, await deps.resolveAccountCredential(scope))
    if (!current || !sameTokenFence(current.state, record.tokenExpected)) {
      throw new Error('OAuth revocation recovery was rejected.')
    }
    if (!await bindingIsAuthorized(record)) {
      await deleteKnownAuthorizationState(authorizationScope(record, record.authorizationId), journalState)
      throw new Error('OAuth account binding is unavailable.')
    }
    const refresh = current.bundle.refreshToken
    await requestForm(record.revocationEndpoint, {
      token: refresh ?? current.bundle.accessToken,
      token_type_hint: refresh ? 'refresh_token' : 'access_token', client_id: record.clientId
    })
    await deps.crash?.afterRemoteRevocation?.()
    if (!await bindingIsAuthorized(record)) {
      await deleteKnownAuthorizationState(authorizationScope(record, record.authorizationId), journalState)
      throw new Error('OAuth account binding is unavailable.')
    }
    const result = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'revoke', scope,
      expected: accountCredentialExpectedState(current.state)
    })
    if ('error' in result || result.status !== 'revoked') {
      const winner = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
      if ('error' in winner || (winner.status === 'ready' && sameTokenFence(winner, record.tokenExpected))) {
        throw new Error('OAuth revocation lost its current-generation fence.')
      }
    }
    await deleteKnownAuthorizationState(authorizationScope(record, record.authorizationId), journalState)
    notifyRefreshInventoryChanged()
    return { revoked: true }
  }

  const refreshBinding = async (
    input: OAuthAuthorizationBindingV1,
    notifyInventoryChanged = true
  ): Promise<OAuthAccountStatusV1> => {
    const binding = normalizeBinding(input)
    if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
    const scope = tokenScope(binding)
    const current = parseBundle(binding, await deps.resolveAccountCredential(scope))
    if (!current?.bundle.refreshToken) throw new Error('OAuth refresh is unavailable.')
    const response = await requestForm(binding.tokenEndpoint, {
      grant_type: 'refresh_token', refresh_token: current.bundle.refreshToken, client_id: binding.clientId
    })
    if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
    const candidate = successorProviderBundle(
      bundleFromResponse(response, now(), binding, current.bundle),
      expected(current.state)
    )
    const replaced = await deps.requestAccountCredential({
      schemaVersion: 1, operation: 'put', scope,
      expected: accountCredentialExpectedState(current.state), credential: tokenDraft(binding.owner, candidate)
    })
    if ('error' in replaced || replaced.status !== 'ready') {
      throw new Error('OAuth refresh lost its current-generation fence.')
    }
    const verified = parseBundle(candidate.oauthBinding, await deps.resolveAccountCredential(scope))
    if (!verified || verified.state.providerRevision !== replaced.providerRevision ||
      verified.state.providerGeneration !== replaced.providerGeneration ||
      !bundlesEqual(verified.bundle, candidate)) {
      throw new Error('OAuth refresh could not be verified.')
    }
    if (!await bindingIsAuthorized(candidate.oauthBinding)) {
      await removeExactCommittedCandidate(scope, replaced)
      throw new Error('OAuth account binding is unavailable.')
    }
    if (notifyInventoryChanged) notifyRefreshInventoryChanged()
    return publicStatus(candidate)
  }

  return {
    async begin(input: OAuthAuthorizationBindingV1, authority: {
      webContentsId: number
    }): Promise<OAuthAuthorizationStartV1> {
      return withAuthorizationAdmission(async () => {
        const binding = normalizeBinding(input)
        if (!Number.isSafeInteger(authority.webContentsId) || authority.webContentsId <= 0) {
          throw new Error('OAuth authorization authority is unavailable.')
        }
        if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
        const currentCount = await cleanupAuthorizationInventoryForAdmission()
        if (currentCount >= MAX_CURRENT_OAUTH_AUTHORIZATIONS) {
          throw new Error('OAuth authorization capacity is unavailable.')
        }
        const authorizationId = `oauth_${randomOpaque(18)}`
        const state = randomOpaque()
        const nonce = randomOpaque()
        const codeVerifier = randomOpaque(48)
        const expiresAtMs = now() + ttlMs
        const stateScope = authorizationScope(binding, authorizationId)
        const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope: stateScope })
        const tokenCurrent = await deps.requestAccountCredential({
          schemaVersion: 1, operation: 'status', scope: tokenScope(binding)
        })
        if ('error' in current || current.status !== 'absent' || 'error' in tokenCurrent) {
          throw new Error('OAuth authorization state is unavailable.')
        }
        if (binding.owner === 'provider' && tokenCurrent.status !== 'ready') {
          throw new Error('OAuth authorization state is unavailable.')
        }
        const record: OAuthAuthorizationRecordV1 = {
          schemaVersion: 1, authorizationId, ...binding, state, nonce, codeVerifier, expiresAtMs,
          phase: 'pending', tokenExpected: expected(tokenCurrent),
          initiatingWebContentsId: authority.webContentsId
        }
        const stored = await putRecord(stateScope, expected(current), record)
        if (!await bindingIsAuthorized(record)) {
          await deleteKnownAuthorizationState(stateScope, stored)
          throw new Error('OAuth account binding is unavailable.')
        }
        const url = new URL(binding.authorizationEndpoint)
        url.searchParams.set('response_type', 'code')
        url.searchParams.set('client_id', binding.clientId)
        url.searchParams.set('redirect_uri', binding.redirectUri)
        url.searchParams.set('scope', binding.scopes.join(' '))
        url.searchParams.set('state', state)
        url.searchParams.set('nonce', nonce)
        url.searchParams.set('code_challenge', createHash('sha256').update(codeVerifier).digest('base64url'))
        url.searchParams.set('code_challenge_method', 'S256')
        return { authorizationId, authorizationUrl: url.toString(), expiresAtMs }
      })
    },

    async completeNativeCallback(input: OAuthNativeCallbackV1): Promise<OAuthAccountStatusV1> {
      const callback = parseOAuthNativeCallback(input.callbackUrl)
      const profileBinding = bounded(input.profileBinding, 64)
      if (!callback || !/^[a-f0-9]{64}$/.test(profileBinding)) {
        throw new Error('OAuth authorization callback was rejected.')
      }
      const matches = await findAuthorizationRecords((record) =>
        record.bindingKey === callback.bindingKey &&
        constantTimeEqual(record.state, callback.state) &&
        constantTimeEqual(record.profileBinding, profileBinding)
      )
      if (matches.length !== 1) throw new Error('OAuth authorization callback was rejected.')
      const parsed = matches[0]!
      const scope = authorizationScope(parsed.record, parsed.record.authorizationId)
      if (parsed.record.phase !== 'pending' || !await bindingIsAuthorized(parsed.record) ||
        now() > parsed.record.expiresAtMs ||
        !deps.isInitiatingAuthorityCurrent?.(parsed.record.initiatingWebContentsId, profileBinding)) {
        await deleteKnownAuthorizationState(scope, parsed.state)
        throw new Error('OAuth authorization callback was rejected.')
      }
      if (callback.issuer !== undefined && callback.issuer !== parsed.record.issuer) {
        await deleteKnownAuthorizationState(scope, parsed.state)
        throw new Error('OAuth authorization callback was rejected.')
      }
      if (callback.error || !callback.code) {
        await deleteKnownAuthorizationState(scope, parsed.state)
        throw new Error(`OAuth authorization was not completed: ${callback.error ?? 'invalid_request'}.`)
      }
      const claimed: OAuthAuthorizationRecordV1 = {
        ...parsed.record, phase: 'exchanging', callbackCode: bounded(callback.code, 8192)
      }
      const claimedState = await putRecord(scope, accountCredentialExpectedState(parsed.state), claimed)
      await deps.crash?.afterCallbackClaimed?.()
      return exchangeAndCommit(claimed, claimedState)
    },

    async authorizationStatus(input: {
      authorizationId: string
      profileBinding: string
      webContentsId: number
      expectedOwner: OAuthAccountOwner
    }): Promise<OAuthAuthorizationPublicStatusV1 | null> {
      const authorizationId = bounded(input.authorizationId, 192)
      const profileBinding = bounded(input.profileBinding, 64)
      const matches = await findAuthorizationRecords((record) =>
        record.authorizationId === authorizationId &&
        record.owner === input.expectedOwner &&
        record.initiatingWebContentsId === input.webContentsId &&
        constantTimeEqual(record.profileBinding, profileBinding)
      )
      if (matches.length !== 1) return null
      const matched = matches[0]!
      const record = matched.record
      const current = await bindingIsAuthorized(record) && now() <= record.expiresAtMs &&
        (record.phase !== 'pending' || Boolean(deps.isInitiatingAuthorityCurrent?.(
          record.initiatingWebContentsId, record.profileBinding
        )))
      if (!current) {
        await deleteKnownAuthorizationState(
          authorizationScope(record, record.authorizationId), matched.state
        )
        return null
      }
      return {
        authorizationId, expiresAtMs: record.expiresAtMs,
        phase: record.phase === 'pending' ? 'pending' : 'processing'
      }
    },

    async cancelAuthorization(input: {
      authorizationId: string
      profileBinding: string
      webContentsId: number
      expectedOwner: OAuthAccountOwner
    }): Promise<{ cancelled: true }> {
      const authorizationId = bounded(input.authorizationId, 192)
      const profileBinding = bounded(input.profileBinding, 64)
      const matches = await findAuthorizationRecords((record) =>
        record.authorizationId === authorizationId && record.owner === input.expectedOwner &&
        record.initiatingWebContentsId === input.webContentsId &&
        constantTimeEqual(record.profileBinding, profileBinding)
      )
      if (matches.length !== 1) throw new Error('OAuth authorization cancellation is unavailable.')
      await deleteKnownAuthorizationState(
        authorizationScope(matches[0]!.record, authorizationId), matches[0]!.state
      )
      return { cancelled: true }
    },

    async recover(input: {
      binding: OAuthAuthorizationBindingV1
      authorizationId: string
    }): Promise<OAuthAccountStatusV1 | null> {
      const binding = normalizeBinding(input.binding)
      const authorizationId = bounded(input.authorizationId, 192)
      const scope = authorizationScope(binding, authorizationId)
      const parsed = parseAuthorization(binding, authorizationId, await deps.resolveAccountCredential(scope))
      if (!parsed) return null
      if (!await bindingIsAuthorized(binding)) {
        await deleteKnownAuthorizationState(scope, parsed.state)
        return null
      }
      if (parsed.record.phase === 'revoking') {
        await finishRevocation(parsed.record, parsed.state)
        return null
      }
      if (parsed.record.phase === 'pending') {
        if (now() <= parsed.record.expiresAtMs) return null
        const deleted = await deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope,
          expected: accountCredentialExpectedState(parsed.state)
        })
        if ('error' in deleted || deleted.status !== 'absent') {
          throw new Error('OAuth authorization cleanup could not be verified.')
        }
        return null
      }
      return exchangeAndCommit(parsed.record, parsed.state)
    },

    async sweepAuthorizationStates(): Promise<{ removed: number; recovered: number; retained: number }> {
      const states = await listAuthorizationStates()
      let removed = 0
      let recovered = 0
      let retained = 0
      for (const known of states) {
        if (known.status !== 'ready') {
          await deleteKnownAuthorizationState(known.scope, known)
          removed++
          continue
        }
        const resolution = await deps.resolveAccountCredential(known.scope)
        if (!resolution.ok ||
          (known.scope.owner === 'provider' && resolution.credential.kind !== 'oauth-token') ||
          (known.scope.owner === 'mcp' && resolution.credential.kind !== 'mcp-oauth-token') ||
          (known.scope.owner === 'extension' && resolution.credential.kind !== 'extension-oauth-token')) {
          await deleteKnownAuthorizationState(known.scope, known)
          removed++
          continue
        }
        let record: OAuthAuthorizationRecordV1
        try {
          const token = resolution.credential.kind === 'oauth-token' ||
            resolution.credential.kind === 'mcp-oauth-token' ||
            resolution.credential.kind === 'extension-oauth-token'
            ? resolution.credential.token
            : ''
          if (!token) throw new Error()
          record = JSON.parse(token) as OAuthAuthorizationRecordV1
        } catch {
          await deleteKnownAuthorizationState(known.scope, known)
          removed++
          continue
        }
        let normalized: NormalizedOAuthBindingV1
        try {
          normalized = normalizeBinding(record)
        } catch {
          await deleteKnownAuthorizationState(known.scope, known)
          removed++
          continue
        }
        const parsed = parseAuthorization(normalized, record.authorizationId, resolution)
        if (!parsed || JSON.stringify(authorizationScope(normalized, record.authorizationId)) !== JSON.stringify(known.scope)) {
          await deleteKnownAuthorizationState(known.scope, known)
          removed++
          continue
        }
        if (!await bindingIsAuthorized(normalized)) {
          await deleteKnownAuthorizationState(known.scope, parsed.state)
          removed++
          continue
        }
        if (parsed.record.phase === 'revoking') {
          await finishRevocation(parsed.record, parsed.state)
          recovered++
          continue
        }
        if (parsed.record.phase === 'pending') {
          if (now() <= parsed.record.expiresAtMs &&
            deps.isInitiatingAuthorityCurrent?.(
              parsed.record.initiatingWebContentsId, parsed.record.profileBinding
            )) {
            retained++
            continue
          }
          await deleteKnownAuthorizationState(known.scope, parsed.state)
          removed++
          continue
        }
        await exchangeAndCommit(parsed.record, parsed.state)
        recovered++
      }
      return { removed, recovered, retained }
    },

    async refreshExpiringAccounts(
      refreshWindowMs = 5 * 60 * 1000
    ): Promise<OAuthRefreshInventoryV1> {
      if (!deps.listAccountCredentialStates || !Number.isSafeInteger(refreshWindowMs) ||
        refreshWindowMs < 0 || refreshWindowMs > 30 * 60 * 1000) {
        throw new Error('OAuth account refresh inventory is unavailable.')
      }
      const states = [
        ...await deps.listAccountCredentialStates('provider', PROVIDER_TOKEN_PURPOSE),
        ...await deps.listAccountCredentialStates('mcp', MCP_TOKEN_PURPOSE),
        ...await deps.listAccountCredentialStates('extension', 'extension-provider-account-token')
      ]
      if (states.length > 128) throw new Error('OAuth account refresh inventory is unavailable.')
      let refreshed = 0
      let revokedExpired = 0
      let removedUnbound = 0
      let retained = 0
      let nextRefreshAtMs: number | undefined
      const retainRefreshNeed = (expiresAtMs: number): void => {
        const candidate = expiresAtMs - refreshWindowMs
        nextRefreshAtMs = nextRefreshAtMs === undefined ? candidate : Math.min(nextRefreshAtMs, candidate)
      }
      for (const state of states) {
        if (state.status !== 'ready') continue
        const resolution = await deps.resolveAccountCredential(state.scope)
        if (state.scope.owner === 'extension' && resolution.ok &&
          resolution.credential.kind === 'extension-account-token') {
          retained++
          continue
        }
        if (!resolution.ok ||
          (state.scope.owner === 'provider' && resolution.credential.kind !== 'provider-oauth-bundle') ||
          (state.scope.owner === 'mcp' && resolution.credential.kind !== 'mcp-oauth-bundle') ||
          (state.scope.owner === 'extension' && resolution.credential.kind !== 'extension-oauth-bundle')) {
          throw new Error('OAuth account refresh inventory is unavailable.')
        }
        const oauthBinding = resolution.credential.kind === 'provider-oauth-bundle' ||
          resolution.credential.kind === 'mcp-oauth-bundle' ||
          resolution.credential.kind === 'extension-oauth-bundle'
          ? resolution.credential.oauthBinding
          : null
        if (!oauthBinding) throw new Error('OAuth account refresh inventory is unavailable.')
        const binding = normalizeBinding(oauthBinding)
        if (JSON.stringify(tokenScope(binding)) !== JSON.stringify(state.scope)) {
          throw new Error('OAuth account refresh binding was rejected.')
        }
        if (!await bindingIsAuthorized(binding)) {
          const deleted = await deps.requestAccountCredential({
            schemaVersion: 1, operation: 'delete', scope: state.scope,
            expected: accountCredentialExpectedState(state)
          })
          if ('error' in deleted || deleted.status !== 'absent') {
            throw new Error('OAuth account cleanup lost its current-generation fence.')
          }
          removedUnbound++
          continue
        }
        const current = parseBundle(binding, resolution)
        if (!current) throw new Error('OAuth account refresh binding was rejected.')
        const expiresAtMs = current.bundle.expiresAtMs
        if (expiresAtMs !== undefined && (!Number.isSafeInteger(expiresAtMs) || expiresAtMs <= 0)) {
          throw new Error('OAuth account refresh inventory is unavailable.')
        }
        if (!expiresAtMs || expiresAtMs > now() + refreshWindowMs) {
          retained++
          if (expiresAtMs && current.bundle.refreshToken) retainRefreshNeed(expiresAtMs)
          continue
        }
        if (current.bundle.refreshToken) {
          const refreshedStatus = await refreshBinding(binding, false)
          refreshed++
          if (refreshedStatus.capabilities.refresh && refreshedStatus.expiresAtMs) {
            retainRefreshNeed(refreshedStatus.expiresAtMs)
          }
          continue
        }
        if (expiresAtMs > now()) {
          retained++
          continue
        }
        const revoked = await deps.requestAccountCredential({
          schemaVersion: 1, operation: 'revoke', scope: state.scope,
          expected: accountCredentialExpectedState(current.state)
        })
        if ('error' in revoked || revoked.status !== 'revoked') {
          throw new Error('Expired OAuth account revocation lost its current-generation fence.')
        }
        revokedExpired++
      }
      return {
        refreshed, revokedExpired, removedUnbound, retained,
        ...(nextRefreshAtMs === undefined ? {} : { nextRefreshAtMs })
      }
    },

    refresh: refreshBinding,

    async replaceSubscription(
      input: OAuthAuthorizationBindingV1,
      subscriptionToken: string
    ): Promise<OAuthAccountStatusV1> {
      const binding = normalizeBinding(input)
      if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
      if (binding.owner !== 'provider') throw new Error('OAuth subscription replacement is unavailable.')
      const scope = tokenScope(binding)
      const current = parseBundle(binding, await deps.resolveAccountCredential(scope))
      if (!current) throw new Error('OAuth subscription replacement is unavailable.')
      const candidate = successorProviderBundle({
        ...current.bundle, subscriptionToken: bounded(subscriptionToken, 32 * 1024)
      }, expected(current.state))
      if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
      const result = await deps.requestAccountCredential({
        schemaVersion: 1, operation: 'put', scope,
        expected: accountCredentialExpectedState(current.state), credential: tokenDraft(binding.owner, candidate)
      })
      if ('error' in result || result.status !== 'ready') {
        throw new Error('OAuth subscription replacement lost its current-generation fence.')
      }
      const verified = parseBundle(candidate.oauthBinding, await deps.resolveAccountCredential(scope))
      if (!verified || verified.state.providerRevision !== result.providerRevision ||
        verified.state.providerGeneration !== result.providerGeneration ||
        !bundlesEqual(verified.bundle, candidate)) {
        throw new Error('OAuth subscription replacement could not be verified.')
      }
      if (!await bindingIsAuthorized(candidate.oauthBinding)) {
        await removeExactCommittedCandidate(scope, result)
        throw new Error('OAuth account binding is unavailable.')
      }
      return publicStatus(candidate)
    },

    async revoke(input: OAuthAuthorizationBindingV1): Promise<{ revoked: true }> {
      const binding = normalizeBinding(input)
      if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
      if (!binding.revocationEndpoint) throw new Error('OAuth revocation is unavailable.')
      const scope = tokenScope(binding)
      const current = parseBundle(binding, await deps.resolveAccountCredential(scope))
      if (!current) throw new Error('OAuth revocation is unavailable.')
      const authorizationId = `oauth_revoke_${randomOpaque(18)}`
      const journalScope = authorizationScope(binding, authorizationId)
      const journalCurrent = await deps.requestAccountCredential({
        schemaVersion: 1, operation: 'status', scope: journalScope
      })
      if ('error' in journalCurrent || journalCurrent.status !== 'absent') {
        throw new Error('OAuth revocation is unavailable.')
      }
      const journal: OAuthAuthorizationRecordV1 = {
        schemaVersion: 1, authorizationId, ...binding,
        state: randomOpaque(), nonce: randomOpaque(), codeVerifier: randomOpaque(48),
        expiresAtMs: now() + ttlMs, phase: 'revoking', tokenExpected: expected(current.state),
        initiatingWebContentsId: 1
      }
      const journalState = await putRecord(journalScope, expected(journalCurrent), journal)
      return finishRevocation(journal, journalState)
    },

    async delete(input: OAuthAuthorizationBindingV1): Promise<{ deleted: true }> {
      const binding = normalizeBinding(input)
      if (!await bindingIsAuthorized(binding)) throw new Error('OAuth account binding is unavailable.')
      const scope = tokenScope(binding)
      const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope })
      if ('error' in current) throw new Error('OAuth account delete is unavailable.')
      if (current.status === 'absent') {
        notifyRefreshInventoryChanged()
        return { deleted: true }
      }
      const result = await deps.requestAccountCredential({
        schemaVersion: 1, operation: 'delete', scope, expected: accountCredentialExpectedState(current)
      })
      if ('error' in result || result.status !== 'absent') {
        throw new Error('OAuth account delete lost its current-generation fence.')
      }
      notifyRefreshInventoryChanged()
      return { deleted: true }
    }
  }
}
