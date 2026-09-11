import { createHash } from 'node:crypto'
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type {
  AccountCredentialDraftV1,
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1,
  AccountCredentialStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import { accountCredentialExpectedState } from './ipc/provider-registry-ipc'
import {
  createProviderOAuthAuthorizationLifecycle,
  createProviderOAuthRefreshScheduler,
  parseOAuthNativeCallback,
  type OAuthAuthorizationBindingV1
} from './provider-oauth-lifecycle'

const registryIncarnation = `inc_${'a'.repeat(43)}`

type Entry = {
  credential?: AccountCredentialDraftV1
  credentialPurpose: string
  status: 'ready' | 'revoked' | 'disconnected'
  revision: number
  generation: number
  incarnation: string
}

function fakeAccountAuthority(options: {
  beforePut?: (
    scope: AccountCredentialScopeV1,
    credential: AccountCredentialDraftV1
  ) => Promise<void> | void
  afterPut?: (
    scope: AccountCredentialScopeV1,
    credential: AccountCredentialDraftV1,
    state: AccountCredentialStateV1
  ) => Promise<void> | void
} = {}) {
  let registryRevision = 1
  let incarnationCounter = 0
  const entries = new Map<string, Entry>()
  const key = (scope: AccountCredentialScopeV1): string => JSON.stringify(scope)
  entries.set(key({
    owner: 'provider', provider: 'provider-a', accountId: 'provider-a',
    purpose: 'provider-oauth-token-bundle'
  }), {
    credential: { kind: 'oauth-token', token: 'synthetic-prior-provider-credential' },
    credentialPurpose: 'provider-api-key', status: 'ready', revision: 1, generation: 1,
    incarnation: `inc_${'p'.repeat(43)}`
  })
  const state = (scope: AccountCredentialScopeV1): AccountCredentialStateV1 => {
    const entry = entries.get(key(scope))
    return {
      schemaVersion: 1,
      scope,
      status: entry?.status ?? 'absent',
      registryRevision: String(registryRevision),
      registryIncarnation,
      providerRevision: entry ? String(entry.revision) : '0',
      providerGeneration: entry ? String(entry.generation) : '0',
      providerIncarnation: entry?.incarnation ?? '',
      ...(entry ? { credentialPurpose: entry.credentialPurpose } : {})
    }
  }
  const conflict = (): AccountCredentialResultV1 => ({
    schemaVersion: 1,
    error: { code: 'conflict', message: 'The provider registry state has changed.' }
  })
  const expectedMatches = (
    request: Exclude<AccountCredentialRequestV1, { operation: 'status' }>,
    current: AccountCredentialStateV1
  ): boolean => request.expected.registryRevision === current.registryRevision &&
    request.expected.registryIncarnation === current.registryIncarnation &&
    request.expected.providerRevision === current.providerRevision &&
    request.expected.providerGeneration === current.providerGeneration &&
    request.expected.providerIncarnation === current.providerIncarnation &&
    request.expected.providerCredentialPurpose === (current.credentialPurpose ?? '')

  const request = async (request: AccountCredentialRequestV1): Promise<AccountCredentialResultV1> => {
    if (request.operation === 'put') await options.beforePut?.(request.scope, request.credential)
    const current = state(request.scope)
    if (request.operation === 'status') return current
    if (!expectedMatches(request, current)) return conflict()
    if (request.operation === 'put') {
      const prior = entries.get(key(request.scope))
      registryRevision += 1
      entries.set(key(request.scope), {
        credential: structuredClone(request.credential),
        credentialPurpose: request.scope.purpose,
        status: 'ready',
        revision: (prior?.revision ?? 0) + 1,
        generation: (prior?.generation ?? 0) + 1,
        incarnation: prior?.incarnation ?? `inc_${String(++incarnationCounter).padStart(43, 'b')}`
      })
      const committed = state(request.scope)
      await options.afterPut?.(request.scope, request.credential, committed)
      return committed
    }
    if (request.operation === 'delete') {
      entries.delete(key(request.scope))
      registryRevision += 1
      return state(request.scope)
    }
    if (request.operation === 'revoke' || request.operation === 'disconnect') {
      const prior = entries.get(key(request.scope))
      if (!prior || prior.status !== 'ready') return conflict()
      registryRevision += 1
      entries.set(key(request.scope), {
        credentialPurpose: '',
        status: request.operation === 'revoke' ? 'revoked' : 'disconnected',
        revision: prior.revision + 1,
        generation: prior.generation + 1,
        incarnation: prior.incarnation
      })
      return state(request.scope)
    }
    return conflict()
  }

  return {
    request,
    resolve: async (scope: AccountCredentialScopeV1) => {
      const entry = entries.get(key(scope))
      return entry?.status === 'ready' && entry.credential
        ? { ok: true as const, state: state(scope), credential: structuredClone(entry.credential) }
        : { ok: false as const, code: 'not_found' as const }
    },
    list: async (
      owner: 'provider' | 'mcp' | 'extension',
      purpose: AccountCredentialScopeV1['purpose']
    ) => [...entries.keys()]
      .map((serialized) => JSON.parse(serialized) as AccountCredentialScopeV1)
      .filter((scope) => scope.owner === owner && scope.purpose === purpose)
      .map(state),
    inject: (scope: AccountCredentialScopeV1, credential: AccountCredentialDraftV1) => {
      registryRevision += 1
      entries.set(key(scope), {
        credential: structuredClone(credential), credentialPurpose: scope.purpose, status: 'ready',
        revision: 1, generation: 1,
        incarnation: `inc_${String(++incarnationCounter).padStart(43, 'i')}`
      })
    },
    mutate: (
      scope: AccountCredentialScopeV1,
      disposition: 'update' | 'replace' | 'revoke' | 'delete'
    ) => {
      const prior = entries.get(key(scope))
      if (!prior) return state(scope)
      registryRevision += 1
      if (disposition === 'delete') {
        entries.delete(key(scope))
        return state(scope)
      }
      if (disposition === 'revoke') {
        entries.set(key(scope), {
          credentialPurpose: '', status: 'revoked',
          revision: prior.revision + 1, generation: prior.generation + 1,
          incarnation: prior.incarnation
        })
        return state(scope)
      }
      entries.set(key(scope), {
        credential: disposition === 'replace'
          ? { kind: 'oauth-token', token: 'synthetic-newer-user-replacement' }
          : structuredClone(prior.credential),
        credentialPurpose: disposition === 'replace' ? scope.purpose : prior.credentialPurpose,
        status: 'ready', revision: prior.revision + 1, generation: prior.generation + 1,
        incarnation: prior.incarnation
      })
      return state(scope)
    },
    state,
    credential: (scope: AccountCredentialScopeV1) => entries.get(key(scope))?.credential
  }
}

type LoopbackRequest = { path: string; form: URLSearchParams }
const closeServers: Array<() => Promise<void>> = []

function syntheticIDToken(claims: Record<string, unknown>): string {
  return [
    Buffer.from(JSON.stringify({ alg: 'RS256', typ: 'JWT' })).toString('base64url'),
    Buffer.from(JSON.stringify(claims)).toString('base64url'),
    Buffer.from('synthetic-signature').toString('base64url')
  ].join('.')
}

async function readBody(request: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = []
  for await (const chunk of request) chunks.push(Buffer.from(chunk))
  return Buffer.concat(chunks).toString('utf8')
}

async function startOAuthServer(options: {
  beforeInitialResponse?: () => Promise<void> | void
  beforeRefreshResponse?: () => Promise<void>
  beforeRevocationResponse?: () => Promise<void> | void
  initialIDToken?: () => string | undefined
} = {}): Promise<{ origin: string; requests: LoopbackRequest[] }> {
  const requests: LoopbackRequest[] = []
  const server = createServer(async (request: IncomingMessage, response: ServerResponse) => {
    const path = new URL(request.url ?? '/', 'http://127.0.0.1').pathname
    const form = new URLSearchParams(await readBody(request))
    requests.push({ path, form })
    if (path === '/token') {
      const grantType = form.get('grant_type')
      if (grantType === 'refresh_token') await options.beforeRefreshResponse?.()
      else await options.beforeInitialResponse?.()
      response.writeHead(200, { 'Content-Type': 'application/json' })
      response.end(JSON.stringify(grantType === 'refresh_token'
        ? {
            access_token: 'synthetic-access-refreshed',
            refresh_token: 'synthetic-refresh-refreshed',
            token_type: 'Bearer',
            expires_in: 7200
          }
        : {
            access_token: 'synthetic-access-initial',
            refresh_token: 'synthetic-refresh-initial',
            ...(options.initialIDToken?.() ? { id_token: options.initialIDToken() } : {}),
            subscription_token: 'synthetic-subscription-initial',
            token_type: 'Bearer',
            expires_in: 3600
          }))
      return
    }
    if (path === '/revoke') {
      await options.beforeRevocationResponse?.()
      response.writeHead(200, { 'Content-Type': 'application/json' })
      response.end('{}')
      return
    }
    response.writeHead(404, { 'Content-Type': 'application/json' })
    response.end('{}')
  })
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  if (!address || typeof address === 'string') throw new Error('loopback server did not bind')
  closeServers.push(() => new Promise<void>((resolve, reject) => {
    server.close((error) => error ? reject(error) : resolve())
  }))
  return { origin: `http://127.0.0.1:${address.port}`, requests }
}

function binding(origin: string, overrides: Partial<OAuthAuthorizationBindingV1> = {}): OAuthAuthorizationBindingV1 {
  const merged = {
    owner: 'provider' as const,
    issuer: `${origin}/`,
    authorizationEndpoint: `${origin}/authorize`,
    tokenEndpoint: `${origin}/token`,
    revocationEndpoint: `${origin}/revoke`,
    clientId: 'client-a',
    provider: 'provider-a',
    accountId: 'provider-a',
    scopes: ['openid', 'profile'],
    bindingKey: '',
    redirectUri: '',
    ownerFingerprint: 'a'.repeat(64),
    ownerRevision: '1',
    ownerGeneration: '1',
    ownerIncarnation: `inc_${'p'.repeat(43)}`,
    profileBinding: 'b'.repeat(64),
    ...overrides
  }
  const bindingKey = `oauthb_${createHash('sha256').update(JSON.stringify({
    owner: merged.owner,
    provider: merged.provider,
    accountId: merged.accountId,
    channelId: merged.channelId ?? '',
    issuer: merged.issuer,
    authorizationEndpoint: merged.authorizationEndpoint,
    tokenEndpoint: merged.tokenEndpoint,
    ownerFingerprint: merged.ownerFingerprint
  })).digest('base64url')}`
  return {
    ...merged,
    bindingKey,
    redirectUri: `com.analytix.desktop:/oauth/callback/${bindingKey}`
  }
}

const initiatingAuthority = { webContentsId: 7 }

async function completeAuthorization(
  lifecycle: ReturnType<typeof createProviderOAuthAuthorizationLifecycle>,
  oauthBinding: OAuthAuthorizationBindingV1,
  start: { authorizationUrl: string },
  code: string,
  stateOverride?: string
) {
  const authorization = new URL(start.authorizationUrl)
  const callback = new URL(oauthBinding.redirectUri)
  callback.searchParams.set('state', stateOverride ?? authorization.searchParams.get('state') ?? '')
  callback.searchParams.set('code', code)
  callback.searchParams.set('iss', oauthBinding.issuer)
  return lifecycle.completeNativeCallback({
    callbackUrl: callback.toString(),
    profileBinding: oauthBinding.profileBinding
  })
}

function providerTokenScope(): AccountCredentialScopeV1 {
  return {
    owner: 'provider', provider: 'provider-a', accountId: 'provider-a',
    purpose: 'provider-oauth-token-bundle'
  }
}

function currentProviderBinding(
  origin: string,
  authority: { state: (scope: AccountCredentialScopeV1) => AccountCredentialStateV1 }
): OAuthAuthorizationBindingV1 {
  const current = authority.state(providerTokenScope())
  return binding(origin, {
    ownerRevision: current.providerRevision,
    ownerGeneration: current.providerGeneration,
    ownerIncarnation: current.providerIncarnation
  })
}

function providerFenceAuthorizer(authority: {
  state: (scope: AccountCredentialScopeV1) => AccountCredentialStateV1
}) {
  return async (candidate: OAuthAuthorizationBindingV1): Promise<boolean> => {
    if (candidate.owner !== 'provider') return true
    const current = authority.state(providerTokenScope())
    return current.status === 'ready' &&
      candidate.ownerRevision === current.providerRevision &&
      candidate.ownerGeneration === current.providerGeneration &&
      candidate.ownerIncarnation === current.providerIncarnation
  }
}

function externalOAuthBinding(
  origin: string,
  owner: 'mcp' | 'extension'
): OAuthAuthorizationBindingV1 {
  return owner === 'mcp'
    ? binding(origin, {
        owner, provider: 'server-a', accountId: 'account-a', channelId: 'server-a'
      })
    : binding(origin, {
        owner, provider: 'extension-demo', accountId: 'account-a', channelId: 'server-demo'
      })
}

function externalTokenScope(binding: OAuthAuthorizationBindingV1): AccountCredentialScopeV1 {
  return {
    owner: binding.owner as 'mcp' | 'extension',
    provider: binding.provider,
    accountId: binding.accountId,
    channelId: binding.channelId,
    purpose: binding.owner === 'mcp' ? 'mcp-oauth-access-token' : 'extension-provider-account-token'
  }
}

afterEach(async () => {
  await Promise.all(closeServers.splice(0).map((close) => close()))
})

describe('Main-owned OAuth account lifecycle', () => {
  it('installs no startup timer without a current refreshable OAuth account', async () => {
    const timers = new Map<number, { callback: () => Promise<void>; delayMs: number }>()
    let nextTimer = 0
    let inventoryScans = 0
    const authority = fakeAccountAuthority()
    const networkEffect = vi.fn(async (): Promise<Response> => {
      throw new Error('synthetic network must remain unused')
    })
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: async () => [],
      authorizeBinding: async () => true,
      fetch: networkEffect,
      now: () => 900_000
    })
    const scheduler = createProviderOAuthRefreshScheduler({
      refreshInventory: async (refreshWindowMs) => {
        inventoryScans++
        return lifecycle.refreshExpiringAccounts(refreshWindowMs)
      },
      now: () => 900_000,
      setTimer: (callback, delayMs) => {
        const id = ++nextTimer
        timers.set(id, { callback, delayMs })
        return id
      },
      clearTimer: (id) => { timers.delete(id as number) },
      onError: vi.fn()
    })
    await scheduler.start()
    expect(inventoryScans).toBe(1)
    expect(networkEffect).not.toHaveBeenCalled()
    expect(timers.size).toBe(0)
  })

  it('arms a bounded timer only after a current refresh need appears', async () => {
    const timers = new Map<number, { callback: () => Promise<void>; delayMs: number }>()
    let refreshable = false
    const scheduler = createProviderOAuthRefreshScheduler({
      refreshInventory: async () => ({
        refreshed: 0, revokedExpired: 0, removedUnbound: 0,
        retained: refreshable ? 1 : 0,
        ...(refreshable ? { nextRefreshAtMs: 1_200_000 } : {})
      }),
      now: () => 900_000,
      setTimer: (callback, delayMs) => {
        timers.set(1, { callback, delayMs })
        return 1
      },
      clearTimer: (id) => { timers.delete(id as number) }
    })
    await scheduler.start()
    expect(timers.size).toBe(0)

    refreshable = true
    await scheduler.rescan()
    expect([...timers.values()].map(({ delayMs }) => delayMs)).toEqual([60_000])
    scheduler.stop()
    expect(timers.size).toBe(0)
  })

  it('does not schedule for a current opaque non-refreshable extension account', async () => {
    const authority = fakeAccountAuthority()
    const extensionScope: AccountCredentialScopeV1 = {
      owner: 'extension', provider: 'extension-demo', accountId: 'account-a',
      channelId: 'server-demo', purpose: 'extension-provider-account-token'
    }
    authority.inject(extensionScope, {
      kind: 'extension-account-token', token: 'synthetic-opaque-extension-token',
      bindingFingerprint: 'e'.repeat(64)
    })
    const networkEffect = vi.fn(async (): Promise<Response> => {
      throw new Error('synthetic network must remain unused')
    })
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: (owner, purpose) =>
        owner === 'extension' && purpose === 'extension-provider-account-token'
          ? authority.list(owner, purpose)
          : Promise.resolve([]),
      authorizeBinding: async () => true,
      fetch: networkEffect,
      now: () => 900_000
    })
    const setTimer = vi.fn()
    const scheduler = createProviderOAuthRefreshScheduler({
      refreshInventory: (refreshWindowMs) => lifecycle.refreshExpiringAccounts(refreshWindowMs),
      now: () => 900_000,
      setTimer
    })
    await scheduler.start()
    expect(setTimer).not.toHaveBeenCalled()
    expect(networkEffect).not.toHaveBeenCalled()
  })

  it.each([
    {
      terminal: 'revoked',
      inventory: { refreshed: 0, revokedExpired: 1, removedUnbound: 0, retained: 0 }
    },
    {
      terminal: 'deleted',
      inventory: { refreshed: 0, revokedExpired: 0, removedUnbound: 0, retained: 0 }
    },
    {
      terminal: 'binding drift',
      inventory: { refreshed: 0, revokedExpired: 0, removedUnbound: 1, retained: 0 }
    },
    { terminal: 'unreadable', inventory: null }
  ])('does not re-arm after $terminal protected inventory', async ({ inventory }) => {
    const timers = new Map<number, { callback: () => Promise<void>; delayMs: number }>()
    let nextTimer = 0
    let terminal = false
    const onError = vi.fn()
    const scheduler = createProviderOAuthRefreshScheduler({
      refreshInventory: async () => {
        if (!terminal) {
          return {
            refreshed: 0, revokedExpired: 0, removedUnbound: 0, retained: 1,
            nextRefreshAtMs: 1_020_000
          }
        }
        if (!inventory) throw new Error('synthetic-private-refresh-token')
        return inventory
      },
      now: () => 900_000,
      setTimer: (callback, delayMs) => {
        const id = ++nextTimer
        timers.set(id, { callback, delayMs })
        return id
      },
      clearTimer: (id) => { timers.delete(id as number) },
      onError
    })
    await scheduler.start()
    expect([...timers.values()].map(({ delayMs }) => delayMs)).toEqual([60_000])

    terminal = true
    const pending = [...timers.values()][0]
    timers.clear()
    await pending?.callback()
    expect(timers.size).toBe(0)
    if (inventory) {
      expect(onError).not.toHaveBeenCalled()
    } else {
      expect(onError).toHaveBeenCalledWith()
      expect(JSON.stringify(onError.mock.calls)).not.toContain('synthetic-private-refresh-token')
    }
  })

  it.each(['revoke', 'delete'] as const)(
    'cancels the pending refresh schedule after an explicit %s',
    async (operation) => {
      const loopback = await startOAuthServer()
      const authority = fakeAccountAuthority()
      let scheduler: ReturnType<typeof createProviderOAuthRefreshScheduler> | undefined
      let rescan = Promise.resolve()
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: async (owner, purpose) =>
          owner === 'provider' && purpose === 'provider-oauth-token-bundle'
            ? [authority.state(providerTokenScope())]
            : authority.list(owner, purpose),
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: async () => true,
        onRefreshInventoryChanged: () => {
          rescan = scheduler?.rescan() ?? Promise.resolve()
        },
        now: () => 2_000_000,
        ttlMs: 30_000
      })
      const start = await lifecycle.begin(binding(loopback.origin), initiatingAuthority)
      await completeAuthorization(lifecycle, binding(loopback.origin), start, `${operation}-schedule-code`)
      const timers = new Map<number, { callback: () => Promise<void>; delayMs: number }>()
      scheduler = createProviderOAuthRefreshScheduler({
        refreshInventory: (refreshWindowMs) => lifecycle.refreshExpiringAccounts(refreshWindowMs),
        now: () => 2_000_000,
        setTimer: (callback, delayMs) => {
          timers.set(1, { callback, delayMs })
          return 1
        },
        clearTimer: (id) => { timers.delete(id as number) }
      })
      await scheduler.start()
      expect(timers.size).toBe(1)

      const currentBinding = currentProviderBinding(loopback.origin, authority)
      if (operation === 'revoke') await lifecycle.revoke(currentBinding)
      else await lifecycle.delete(currentBinding)
      await rescan
      expect(timers.size).toBe(0)
      expect(authority.state(providerTokenScope()).status).toBe(operation === 'revoke' ? 'revoked' : 'absent')
    }
  )

  it('exchanges a one-use PKCE callback and atomically commits a protected token bundle', async () => {
    let initialIDToken: string | undefined
    const loopback = await startOAuthServer({ initialIDToken: () => initialIDToken })
    const authority = fakeAccountAuthority()
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 1_000_000,
      ttlMs: 60_000
    })
    const start = await lifecycle.begin(binding(loopback.origin), initiatingAuthority)
    const authorizationUrl = new URL(start.authorizationUrl)
    const state = authorizationUrl.searchParams.get('state') ?? ''
    const challenge = authorizationUrl.searchParams.get('code_challenge') ?? ''
    const authorizationDraft = authority.credential({
      owner: 'provider', provider: 'provider-a', accountId: 'provider-a',
      channelId: start.authorizationId, purpose: 'provider-oauth-authorization-state'
    })
    expect(authorizationDraft?.kind).toBe('oauth-token')
    const protectedAuthorization = JSON.parse(
      authorizationDraft?.kind === 'oauth-token' ? authorizationDraft.token : '{}'
    ) as Record<string, unknown>
    expect(authorizationUrl.searchParams.get('nonce')).toBe(protectedAuthorization.nonce)
    initialIDToken = syntheticIDToken({
      iss: `${loopback.origin}/`, aud: 'client-a', nonce: protectedAuthorization.nonce,
      exp: 10_000
    })
    expect(protectedAuthorization).toMatchObject({
      state, provider: 'provider-a', accountId: 'provider-a', phase: 'pending'
    })
    await expect(lifecycle.authorizationStatus({
      authorizationId: start.authorizationId,
      profileBinding: binding(loopback.origin).profileBinding,
      webContentsId: initiatingAuthority.webContentsId,
      expectedOwner: 'mcp'
    })).resolves.toBeNull()
    await expect(lifecycle.cancelAuthorization({
      authorizationId: start.authorizationId,
      profileBinding: binding(loopback.origin).profileBinding,
      webContentsId: initiatingAuthority.webContentsId,
      expectedOwner: 'extension'
    })).rejects.toThrow('OAuth authorization cancellation is unavailable.')
    const status = await completeAuthorization(
      lifecycle, binding(loopback.origin), start, 'synthetic-authorization-code', state
    )
    expect(status).toEqual({
      connected: true,
      expiresAtMs: 4_600_000,
      capabilities: { refresh: true, idToken: true, subscription: true }
    })
    expect(JSON.stringify(status)).not.toMatch(/synthetic|credentialRef|authorization-code/)
    const tokenRequest = loopback.requests.find((request) => request.path === '/token')
    expect(tokenRequest?.form.get('code')).toBe('synthetic-authorization-code')
    expect(createHash('sha256').update(tokenRequest?.form.get('code_verifier') ?? '').digest('base64url')).toBe(challenge)
    expect(authority.credential(providerTokenScope())).toMatchObject({
      kind: 'provider-oauth-bundle',
      accessToken: 'synthetic-access-initial',
      refreshToken: 'synthetic-refresh-initial'
    })
    const committedBundle = authority.credential(providerTokenScope())
    const committedState = authority.state(providerTokenScope())
    expect(committedBundle?.kind).toBe('provider-oauth-bundle')
    if (committedBundle?.kind !== 'provider-oauth-bundle') throw new Error('missing provider OAuth bundle')
    expect(committedBundle.oauthBinding).toMatchObject({
      ownerRevision: committedState.providerRevision,
      ownerGeneration: committedState.providerGeneration,
      ownerIncarnation: committedState.providerIncarnation
    })
    await expect(completeAuthorization(
      lifecycle, binding(loopback.origin), start, 'synthetic-replay-code', state
    )).rejects.toThrow('OAuth authorization callback was rejected.')
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(1)
  })

  it('rejects an ID token whose protected nonce is not bound to the authorization', async () => {
    let initialIDToken: string | undefined
    const loopback = await startOAuthServer({ initialIDToken: () => initialIDToken })
    const authority = fakeAccountAuthority()
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 1_000_000,
      ttlMs: 60_000
    })
    const oauthBinding = binding(loopback.origin)
    const start = await lifecycle.begin(oauthBinding, initiatingAuthority)
    initialIDToken = syntheticIDToken({
      iss: oauthBinding.issuer, aud: oauthBinding.clientId, nonce: 'wrong-protected-nonce', exp: 10_000
    })
    await expect(completeAuthorization(
      lifecycle, oauthBinding, start, 'synthetic-wrong-nonce-code'
    )).rejects.toThrow('OAuth provider response was rejected.')
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(1)
    expect(authority.credential(providerTokenScope())).toMatchObject({
      kind: 'oauth-token', token: 'synthetic-prior-provider-credential'
    })
  })

  it('rejects binding/profile/window/issuer drift before any provider effect and cleans the claimed pending state', async () => {
    const loopback = await startOAuthServer()
    const scenarios = [
      { name: 'binding', authorize: false, profile: 'b'.repeat(64), windowCurrent: true, issuer: undefined },
      { name: 'profile', authorize: true, profile: 'e'.repeat(64), windowCurrent: true, issuer: undefined },
      { name: 'window', authorize: true, profile: 'b'.repeat(64), windowCurrent: false, issuer: undefined },
      { name: 'issuer', authorize: true, profile: 'b'.repeat(64), windowCurrent: true, issuer: 'https://mixup.invalid/' }
    ] as const
    for (const scenario of scenarios) {
      const authority = fakeAccountAuthority()
      let bindingCurrent = true
      let windowCurrent = true
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        authorizeBinding: async () => bindingCurrent,
        isInitiatingAuthorityCurrent: () => windowCurrent,
        now: () => 1_200_000,
        ttlMs: 60_000
      })
      const oauthBinding = binding(loopback.origin)
      const start = await lifecycle.begin(oauthBinding, initiatingAuthority)
      bindingCurrent = scenario.authorize
      windowCurrent = scenario.windowCurrent
      const authorization = new URL(start.authorizationUrl)
      const callback = new URL(oauthBinding.redirectUri)
      callback.searchParams.set('state', authorization.searchParams.get('state') ?? '')
      callback.searchParams.set('code', `code-${scenario.name}`)
      callback.searchParams.set('iss', scenario.issuer ?? oauthBinding.issuer)
      await expect(lifecycle.completeNativeCallback({
        callbackUrl: callback.toString(),
        profileBinding: scenario.profile
      })).rejects.toThrow('OAuth authorization callback was rejected.')
      expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(0)
    }
  })

  it('rejects present empty, control, oversized, or ambiguous callback issuer values', () => {
    const oauthBinding = binding('http://127.0.0.1:37821')
    const callback = new URL(oauthBinding.redirectUri)
    callback.searchParams.set('state', 'synthetic-state')
    callback.searchParams.set('code', 'synthetic-code')
    callback.searchParams.set('iss', '')
    expect(parseOAuthNativeCallback(callback.toString())).toBeNull()
    callback.searchParams.set('iss', 'https://issuer.invalid/\u0000suffix')
    expect(parseOAuthNativeCallback(callback.toString())).toBeNull()
    callback.searchParams.set('iss', `https://issuer.invalid/${'a'.repeat(2049)}`)
    expect(parseOAuthNativeCallback(callback.toString())).toBeNull()
    callback.searchParams.set('iss', oauthBinding.issuer)
    callback.searchParams.append('iss', oauthBinding.issuer)
    expect(parseOAuthNativeCallback(callback.toString())).toBeNull()
  })

  it('cleans pending MCP and extension authorizations when their owner config drifts during begin persistence', async () => {
    for (const owner of ['mcp', 'extension'] as const) {
      const loopback = await startOAuthServer()
      let bindingCurrent = true
      const ownedBinding = externalOAuthBinding(loopback.origin, owner)
      const purpose = owner === 'mcp'
        ? 'mcp-oauth-authorization-state' as const
        : 'extension-oauth-authorization-state' as const
      const authority = fakeAccountAuthority({
        afterPut: (scope) => {
          if (scope.owner === owner && scope.purpose === purpose) bindingCurrent = false
        }
      })
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: async () => bindingCurrent,
        now: () => 1_300_000,
        ttlMs: 60_000
      })
      await expect(lifecycle.begin(ownedBinding, initiatingAuthority))
        .rejects.toThrow('OAuth account binding is unavailable.')
      await expect(authority.list(owner, purpose)).resolves.toHaveLength(0)
    }
  })

  it('removes exact stale MCP and extension callback candidates when owner config drifts after token commit', async () => {
    for (const owner of ['mcp', 'extension'] as const) {
      const loopback = await startOAuthServer()
      let bindingCurrent = true
      const ownedBinding = externalOAuthBinding(loopback.origin, owner)
      const tokenScope = externalTokenScope(ownedBinding)
      const authority = fakeAccountAuthority({
        afterPut: (scope) => {
          if (JSON.stringify(scope) === JSON.stringify(tokenScope)) bindingCurrent = false
        }
      })
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: async () => bindingCurrent,
        now: () => 1_400_000,
        ttlMs: 60_000
      })
      const start = await lifecycle.begin(ownedBinding, initiatingAuthority)
      await expect(completeAuthorization(
        lifecycle, ownedBinding, start, `synthetic-${owner}-config-drift-code`
      )).rejects.toThrow('OAuth account binding is unavailable.')
      expect(authority.state(tokenScope).status).toBe('absent')
    }
  })

  it('never removes a newer extension successor while cleaning a drifted post-commit candidate', async () => {
    const loopback = await startOAuthServer()
    let bindingCurrent = true
    let replaceAfterCandidate = true
    let authority!: ReturnType<typeof fakeAccountAuthority>
    const ownedBinding = externalOAuthBinding(loopback.origin, 'extension')
    const tokenScope = externalTokenScope(ownedBinding)
    authority = fakeAccountAuthority({
      afterPut: async (scope, credential, committed) => {
        if (!replaceAfterCandidate || JSON.stringify(scope) !== JSON.stringify(tokenScope) ||
          credential.kind !== 'extension-oauth-bundle') return
        replaceAfterCandidate = false
        bindingCurrent = false
        const successor = await authority.request({
          schemaVersion: 1, operation: 'put', scope,
          expected: accountCredentialExpectedState(committed),
          credential: { ...credential, accessToken: 'synthetic-newer-extension-successor' }
        })
        expect('error' in successor).toBe(false)
      }
    })
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => bindingCurrent,
      now: () => 1_450_000,
      ttlMs: 60_000
    })
    const start = await lifecycle.begin(ownedBinding, initiatingAuthority)
    await expect(completeAuthorization(
      lifecycle, ownedBinding, start, 'synthetic-newer-successor-code'
    )).rejects.toThrow(/could not be verified|binding is unavailable/)
    expect(authority.credential(tokenScope)).toMatchObject({
      kind: 'extension-oauth-bundle', accessToken: 'synthetic-newer-extension-successor'
    })
  })

  it('recovers exactly after the protected callback claim without selecting endpoints from the callback', async () => {
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const shared = {
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      authorizeBinding: async () => true,
      isInitiatingAuthorityCurrent: () => true,
      now: () => 1_500_000,
      ttlMs: 60_000
    }
    const interrupted = createProviderOAuthAuthorizationLifecycle({
      ...shared,
      crash: { afterCallbackClaimed: () => { throw new Error('synthetic-callback-claim-crash') } }
    })
    const oauthBinding = binding(loopback.origin)
    const start = await interrupted.begin(oauthBinding, initiatingAuthority)
    await expect(completeAuthorization(
      interrupted, oauthBinding, start, 'synthetic-claimed-code'
    )).rejects.toThrow('synthetic-callback-claim-crash')
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(0)
    const restarted = createProviderOAuthAuthorizationLifecycle(shared)
    await expect(restarted.recover({
      binding: oauthBinding,
      authorizationId: start.authorizationId
    })).resolves.toMatchObject({ connected: true })
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(1)
  })

  it('recovers deterministically after token-candidate and post-commit crash points', async () => {
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const shared = {
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 2_000_000,
      ttlMs: 60_000
    }
    const first = createProviderOAuthAuthorizationLifecycle({
      ...shared,
      crash: { afterTokenCandidateStored: () => { throw new Error('synthetic-crash-candidate') } }
    })
    const started = await first.begin(binding(loopback.origin), initiatingAuthority)
    const state = new URL(started.authorizationUrl).searchParams.get('state') ?? ''
    await expect(completeAuthorization(
      first, binding(loopback.origin), started, 'synthetic-crash-code', state
    )).rejects.toThrow('synthetic-crash-candidate')
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(1)
    const restarted = createProviderOAuthAuthorizationLifecycle(shared)
    await expect(restarted.recover({
      binding: binding(loopback.origin), authorizationId: started.authorizationId
    })).resolves.toMatchObject({ connected: true })
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(1)

    const second = createProviderOAuthAuthorizationLifecycle({
      ...shared,
      crash: { afterTokenCommitted: () => { throw new Error('synthetic-crash-committed') } }
    })
    const secondBinding = currentProviderBinding(loopback.origin, authority)
    const secondStart = await second.begin(secondBinding, initiatingAuthority)
    const secondState = new URL(secondStart.authorizationUrl).searchParams.get('state') ?? ''
    await expect(completeAuthorization(
      second, secondBinding, secondStart, 'synthetic-second-code', secondState
    )).rejects.toThrow('synthetic-crash-committed')
    const requestsBeforeRecovery = loopback.requests.filter((request) => request.path === '/token').length
    await expect(restarted.recover({
      binding: secondBinding, authorizationId: secondStart.authorizationId
    })).resolves.toMatchObject({ connected: true })
    expect(loopback.requests.filter((request) => request.path === '/token')).toHaveLength(requestsBeforeRecovery)
  })

  it('refresh, subscription replacement, and revocation use exact current-generation CAS', async () => {
    let releaseRefresh!: () => void
    let refreshArrived!: () => void
    const refreshStarted = new Promise<void>((resolve) => { refreshArrived = resolve })
    const refreshRelease = new Promise<void>((resolve) => { releaseRefresh = resolve })
    const loopback = await startOAuthServer({
      beforeRefreshResponse: async () => {
        refreshArrived()
        await refreshRelease
      }
    })
    const authority = fakeAccountAuthority()
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 3_000_000,
      ttlMs: 60_000
    })
    const start = await lifecycle.begin(binding(loopback.origin), initiatingAuthority)
    await completeAuthorization(lifecycle, binding(loopback.origin), start, 'synthetic-initial-code')
    const refresh = lifecycle.refresh(currentProviderBinding(loopback.origin, authority))
    await refreshStarted
    const current = authority.state(providerTokenScope())
    await authority.request({
      schemaVersion: 1, operation: 'revoke', scope: providerTokenScope(),
      expected: accountCredentialExpectedState(current)
    })
    releaseRefresh()
    await expect(refresh).rejects.toThrow('OAuth refresh lost its current-generation fence.')

    const successorAuthority = fakeAccountAuthority()
    const successor = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: successorAuthority.request,
      resolveAccountCredential: successorAuthority.resolve,
      listAccountCredentialStates: successorAuthority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 3_000_000,
      ttlMs: 60_000
    })
    const reconnect = await successor.begin(binding(loopback.origin), initiatingAuthority)
    await completeAuthorization(successor, binding(loopback.origin), reconnect, 'synthetic-reconnect-code')
    const connectedState = successorAuthority.state(providerTokenScope())
    const connectedBundle = successorAuthority.credential(providerTokenScope())
    expect(connectedBundle?.kind).toBe('provider-oauth-bundle')
    if (connectedBundle?.kind !== 'provider-oauth-bundle') throw new Error('missing provider OAuth bundle')
    expect(connectedBundle.oauthBinding.ownerGeneration).toBe(connectedState.providerGeneration)
    await expect(successor.replaceSubscription(
      currentProviderBinding(loopback.origin, successorAuthority),
      'synthetic-subscription-replaced'
    ))
      .resolves.toMatchObject({ capabilities: { subscription: true } })
    const subscriptionState = successorAuthority.state(providerTokenScope())
    const subscriptionBundle = successorAuthority.credential(providerTokenScope())
    expect(subscriptionBundle?.kind).toBe('provider-oauth-bundle')
    if (subscriptionBundle?.kind !== 'provider-oauth-bundle') throw new Error('missing provider OAuth bundle')
    expect(subscriptionBundle.oauthBinding).toMatchObject({
      ownerRevision: subscriptionState.providerRevision,
      ownerGeneration: subscriptionState.providerGeneration
    })
    await expect(successor.revoke(currentProviderBinding(loopback.origin, successorAuthority)))
      .resolves.toEqual({ revoked: true })
    expect(successorAuthority.state(providerTokenScope()).status).toBe('revoked')
    const revokeRequest = loopback.requests.find((request) => request.path === '/revoke')
    expect(revokeRequest?.form.get('token')).toBe('synthetic-refresh-initial')
    expect(JSON.stringify(loopback.requests.map((request) => request.path))).not.toContain('synthetic')
  })

  it('lets Provider user update, replace, revoke, and delete win callback commit CAS', async () => {
    for (const disposition of ['update', 'replace', 'revoke', 'delete'] as const) {
      const loopback = await startOAuthServer()
      let race: typeof disposition | null = null
      let authority!: ReturnType<typeof fakeAccountAuthority>
      authority = fakeAccountAuthority({
        beforePut: (scope) => {
          if (!race || JSON.stringify(scope) !== JSON.stringify(providerTokenScope())) return
          const winner = race
          race = null
          authority.mutate(scope, winner)
        }
      })
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: providerFenceAuthorizer(authority),
        now: () => 3_050_000,
        ttlMs: 60_000
      })
      const ownedBinding = currentProviderBinding(loopback.origin, authority)
      const started = await lifecycle.begin(ownedBinding, initiatingAuthority)
      race = disposition
      await expect(completeAuthorization(
        lifecycle, ownedBinding, started, `synthetic-${disposition}-race-code`
      )).rejects.toThrow()
      const winner = authority.state(providerTokenScope())
      expect(winner.status).toBe(disposition === 'delete' ? 'absent' : disposition === 'revoke' ? 'revoked' : 'ready')
      expect(authority.credential(providerTokenScope())?.kind).not.toBe('provider-oauth-bundle')
      await expect(lifecycle.sweepAuthorizationStates()).resolves.toEqual({
        removed: 1, recovered: 0, retained: 0
      })
    }
  })

  it('lets Provider user update, replace, revoke, and delete win refresh and subscription CAS', async () => {
    for (const operation of ['refresh', 'subscription'] as const) {
      for (const disposition of ['update', 'replace', 'revoke', 'delete'] as const) {
        const loopback = await startOAuthServer()
        let race: typeof disposition | null = null
        let authority!: ReturnType<typeof fakeAccountAuthority>
        authority = fakeAccountAuthority({
          beforePut: (scope) => {
            if (!race || JSON.stringify(scope) !== JSON.stringify(providerTokenScope())) return
            const winner = race
            race = null
            authority.mutate(scope, winner)
          }
        })
        const lifecycle = createProviderOAuthAuthorizationLifecycle({
          requestAccountCredential: authority.request,
          resolveAccountCredential: authority.resolve,
          listAccountCredentialStates: authority.list,
          isInitiatingAuthorityCurrent: () => true,
          authorizeBinding: providerFenceAuthorizer(authority),
          now: () => 3_075_000,
          ttlMs: 60_000
        })
        const initialBinding = currentProviderBinding(loopback.origin, authority)
        const started = await lifecycle.begin(initialBinding, initiatingAuthority)
        await completeAuthorization(lifecycle, initialBinding, started, `synthetic-${operation}-initial-code`)
        const operationBinding = currentProviderBinding(loopback.origin, authority)
        race = disposition
        const mutation = operation === 'refresh'
          ? lifecycle.refresh(operationBinding)
          : lifecycle.replaceSubscription(operationBinding, 'synthetic-racing-subscription')
        await expect(mutation).rejects.toThrow(/current-generation fence/)
        const winner = authority.state(providerTokenScope())
        expect(winner.status).toBe(disposition === 'delete' ? 'absent' : disposition === 'revoke' ? 'revoked' : 'ready')
        const credential = authority.credential(providerTokenScope())
        if (disposition === 'update') {
          expect(credential).toMatchObject({
            kind: 'provider-oauth-bundle', accessToken: 'synthetic-access-initial',
            subscriptionToken: 'synthetic-subscription-initial'
          })
        } else if (disposition === 'replace') {
          expect(credential).toEqual({ kind: 'oauth-token', token: 'synthetic-newer-user-replacement' })
        } else {
          expect(credential).toBeUndefined()
        }
      }
    }
  })

  it('rejects MCP and extension refresh when owner config drifts during the remote effect', async () => {
    for (const owner of ['mcp', 'extension'] as const) {
      let bindingCurrent = true
      const loopback = await startOAuthServer({
        beforeRefreshResponse: async () => { bindingCurrent = false }
      })
      const authority = fakeAccountAuthority()
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: async () => bindingCurrent,
        now: () => 3_100_000,
        ttlMs: 60_000
      })
      const ownedBinding = externalOAuthBinding(loopback.origin, owner)
      const start = await lifecycle.begin(ownedBinding, initiatingAuthority)
      await completeAuthorization(lifecycle, ownedBinding, start, `synthetic-${owner}-initial-code`)
      const scope = externalTokenScope(ownedBinding)
      const prior = authority.credential(scope)
      await expect(lifecycle.refresh(ownedBinding)).rejects.toThrow('OAuth account binding is unavailable.')
      expect(authority.credential(scope)).toEqual(prior)
    }
  })

  it('does not locally revoke MCP or extension credentials after owner config drifts during remote revocation', async () => {
    for (const owner of ['mcp', 'extension'] as const) {
      let bindingCurrent = true
      const loopback = await startOAuthServer({
        beforeRevocationResponse: () => { bindingCurrent = false }
      })
      const authority = fakeAccountAuthority()
      const lifecycle = createProviderOAuthAuthorizationLifecycle({
        requestAccountCredential: authority.request,
        resolveAccountCredential: authority.resolve,
        listAccountCredentialStates: authority.list,
        isInitiatingAuthorityCurrent: () => true,
        authorizeBinding: async () => bindingCurrent,
        now: () => 3_200_000,
        ttlMs: 60_000
      })
      const ownedBinding = externalOAuthBinding(loopback.origin, owner)
      const start = await lifecycle.begin(ownedBinding, initiatingAuthority)
      await completeAuthorization(lifecycle, ownedBinding, start, `synthetic-${owner}-initial-code`)
      const scope = externalTokenScope(ownedBinding)
      await expect(lifecycle.revoke(ownedBinding)).rejects.toThrow('OAuth account binding is unavailable.')
      expect(authority.state(scope).status).toBe('ready')
    }
  })

  it('bounds pending admission and can clean an over-limit expired inventory without deleting a current authorization', async () => {
    let now = 3_300_000
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => now,
      ttlMs: 30_000
    })
    const mcpBinding = binding(loopback.origin, {
      owner: 'mcp', provider: 'server-pressure', accountId: 'account-pressure', channelId: 'server-pressure'
    })
    for (let index = 0; index < 32; index++) {
      await lifecycle.begin(mcpBinding, initiatingAuthority)
    }
    await expect(lifecycle.begin(mcpBinding, initiatingAuthority))
      .rejects.toThrow('OAuth authorization capacity is unavailable.')

    const recoveryAuthority = fakeAccountAuthority()
    const recoveryDeps = {
      requestAccountCredential: recoveryAuthority.request,
      resolveAccountCredential: recoveryAuthority.resolve,
      listAccountCredentialStates: recoveryAuthority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async (candidate: OAuthAuthorizationBindingV1) => candidate.ownerFingerprint !== 'd'.repeat(64),
      now: () => now,
      ttlMs: 30_000
    }
    const preRestartLifecycle = createProviderOAuthAuthorizationLifecycle(recoveryDeps)
    const extensionBinding = binding(loopback.origin, {
      owner: 'extension', provider: 'extension-current', accountId: 'account-current', channelId: 'server-current'
    })
    const current = await preRestartLifecycle.begin(extensionBinding, initiatingAuthority)
    const absentToken = recoveryAuthority.state({
      owner: 'mcp', provider: mcpBinding.provider, accountId: mcpBinding.accountId,
      channelId: mcpBinding.channelId, purpose: 'mcp-oauth-access-token'
    })
    for (let index = 0; index < 64; index++) {
      const authorizationId = `oauth_injected_${index}`
      recoveryAuthority.inject({
        owner: 'mcp', provider: mcpBinding.provider, accountId: mcpBinding.accountId,
        channelId: `${mcpBinding.channelId}:${authorizationId}`,
        purpose: 'mcp-oauth-authorization-state'
      }, {
        kind: 'mcp-oauth-token',
        token: JSON.stringify({
          schemaVersion: 1, authorizationId, ...mcpBinding,
          state: `state-${index}`, nonce: `nonce-${index}`, codeVerifier: `verifier-${index}`,
          expiresAtMs: now - 1, phase: 'pending',
          tokenExpected: {
            registryRevision: absentToken.registryRevision,
            registryIncarnation: absentToken.registryIncarnation,
            providerRevision: '0', providerGeneration: '0', providerIncarnation: '',
            providerCredentialPurpose: ''
          },
          initiatingWebContentsId: initiatingAuthority.webContentsId
        })
      })
    }
    const staleBinding = binding(loopback.origin, {
      owner: 'mcp', provider: 'server-stale', accountId: 'account-stale', channelId: 'server-stale',
      ownerFingerprint: 'd'.repeat(64)
    })
    const staleAuthorizationId = 'oauth_injected_binding_drift'
    recoveryAuthority.inject({
      owner: 'mcp', provider: staleBinding.provider, accountId: staleBinding.accountId,
      channelId: `${staleBinding.channelId}:${staleAuthorizationId}`,
      purpose: 'mcp-oauth-authorization-state'
    }, {
      kind: 'mcp-oauth-token',
      token: JSON.stringify({
        schemaVersion: 1, authorizationId: staleAuthorizationId, ...staleBinding,
        state: 'state-binding-drift', nonce: 'nonce-binding-drift', codeVerifier: 'verifier-binding-drift',
        expiresAtMs: now + 30_000, phase: 'pending',
        tokenExpected: {
          registryRevision: absentToken.registryRevision,
          registryIncarnation: absentToken.registryIncarnation,
          providerRevision: '0', providerGeneration: '0', providerIncarnation: '',
          providerCredentialPurpose: ''
        },
        initiatingWebContentsId: initiatingAuthority.webContentsId
      })
    })
    const restartedLifecycle = createProviderOAuthAuthorizationLifecycle(recoveryDeps)
    await expect(restartedLifecycle.sweepAuthorizationStates()).resolves.toEqual({
      removed: 65, recovered: 0, retained: 1
    })
    await expect(restartedLifecycle.authorizationStatus({
      authorizationId: current.authorizationId,
      profileBinding: extensionBinding.profileBinding,
      webContentsId: initiatingAuthority.webContentsId,
      expectedOwner: 'extension'
    })).resolves.toMatchObject({ authorizationId: current.authorizationId, phase: 'pending' })
    await expect(completeAuthorization(
      restartedLifecycle, extensionBinding, current, 'synthetic-capacity-recovery-code'
    )).resolves.toMatchObject({ connected: true })
  })

  it('recovers a remote revocation crash to one deterministic local revoked state', async () => {
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const shared = {
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => 3_500_000,
      ttlMs: 60_000
    }
    const connected = createProviderOAuthAuthorizationLifecycle(shared)
    const start = await connected.begin(binding(loopback.origin), initiatingAuthority)
    await completeAuthorization(connected, binding(loopback.origin), start, 'revoke-crash-code')
    const interrupted = createProviderOAuthAuthorizationLifecycle({
      ...shared,
      crash: { afterRemoteRevocation: () => { throw new Error('synthetic-revocation-crash') } }
    })
    await expect(interrupted.revoke(currentProviderBinding(loopback.origin, authority)))
      .rejects.toThrow('synthetic-revocation-crash')
    expect(authority.state(providerTokenScope()).status).toBe('ready')
    expect(loopback.requests.filter((request) => request.path === '/revoke')).toHaveLength(1)

    const restarted = createProviderOAuthAuthorizationLifecycle(shared)
    await expect(restarted.sweepAuthorizationStates()).resolves.toEqual({
      removed: 0, recovered: 1, retained: 0
    })
    expect(authority.state(providerTokenScope()).status).toBe('revoked')
    expect(loopback.requests.filter((request) => request.path === '/revoke')).toHaveLength(2)
    await expect(restarted.sweepAuthorizationStates()).resolves.toEqual({
      removed: 0, recovered: 0, retained: 0
    })
  })

  it('supports MCP OAuth bundles while rejecting cross-binding and late callbacks', async () => {
    let now = 4_000_000
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => true,
      now: () => now,
      ttlMs: 30_000
    })
    const mcpBinding = binding(loopback.origin, {
      owner: 'mcp', provider: 'server-a', accountId: 'account-a', channelId: 'server-a'
    })
    const mcpStart = await lifecycle.begin(mcpBinding, initiatingAuthority)
    const mcpState = new URL(mcpStart.authorizationUrl).searchParams.get('state') ?? ''
    await expect(completeAuthorization(
      lifecycle, mcpBinding, mcpStart, 'synthetic-mcp-code', mcpState
    )).resolves.toMatchObject({ connected: true, capabilities: { refresh: true } })
    expect(authority.credential({
      owner: 'mcp', provider: 'server-a', accountId: 'account-a', channelId: 'server-a',
      purpose: 'mcp-oauth-access-token'
    })).toMatchObject({ kind: 'mcp-oauth-bundle', accessToken: 'synthetic-access-initial' })

    const late = await lifecycle.begin(binding(loopback.origin), initiatingAuthority)
    const lateState = new URL(late.authorizationUrl).searchParams.get('state') ?? ''
    await expect(completeAuthorization(
      lifecycle, binding(loopback.origin, { provider: 'provider-b' }), late,
      'cross-provider-code', lateState
    )).rejects.toThrow('OAuth authorization callback was rejected.')
    await expect(completeAuthorization(
      lifecycle, binding(loopback.origin), late, 'state-drift-code', `${lateState}-drift`
    )).rejects.toThrow('OAuth authorization callback was rejected.')
    now += 30_001
    await expect(completeAuthorization(
      lifecycle, binding(loopback.origin), late, 'late-code', lateState
    )).rejects.toThrow('OAuth authorization callback was rejected.')
  })

  it('acquires, refreshes, and revokes an installed-extension OAuth bundle at its exact binding', async () => {
    let now = 4_500_000
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const extensionBinding = binding(loopback.origin, {
      owner: 'extension', provider: 'extension-demo', accountId: 'account-a', channelId: 'server-demo'
    })
    const authorizeBinding = vi.fn(async (candidate: OAuthAuthorizationBindingV1) => (
      candidate.owner === 'extension' && candidate.provider === 'extension-demo' &&
      candidate.accountId === 'account-a' && candidate.channelId === 'server-demo'
    ))
    const lifecycle = createProviderOAuthAuthorizationLifecycle({
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      listAccountCredentialStates: authority.list,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding,
      now: () => now,
      ttlMs: 30_000
    })
    const started = await lifecycle.begin(extensionBinding, initiatingAuthority)
    await expect(completeAuthorization(
      lifecycle, extensionBinding, started, 'synthetic-extension-code'
    )).resolves.toMatchObject({ connected: true, capabilities: { refresh: true } })
    const extensionScope: AccountCredentialScopeV1 = {
      owner: 'extension', provider: 'extension-demo', accountId: 'account-a', channelId: 'server-demo',
      purpose: 'extension-provider-account-token'
    }
    expect(authority.credential(extensionScope)).toMatchObject({
      kind: 'extension-oauth-bundle', accessToken: 'synthetic-access-initial'
    })
    now += 1_000
    await expect(lifecycle.refresh(extensionBinding)).resolves.toMatchObject({
      connected: true, capabilities: { refresh: true }
    })
    expect(authority.credential(extensionScope)).toMatchObject({
      kind: 'extension-oauth-bundle', accessToken: 'synthetic-access-refreshed'
    })
    await expect(lifecycle.revoke(extensionBinding)).resolves.toEqual({ revoked: true })
    expect(authority.state(extensionScope).status).toBe('revoked')
    await expect(lifecycle.begin({ ...extensionBinding, provider: 'extension-other' }, initiatingAuthority))
      .rejects.toThrow('OAuth account binding is unavailable.')
    expect(authorizeBinding).toHaveBeenCalled()
  })

  it('refreshes an expiring protected bundle from its stored exact binding inventory', async () => {
    let now = 5_000_000
    let bindingCurrent = true
    const loopback = await startOAuthServer()
    const authority = fakeAccountAuthority()
    const deps = {
      requestAccountCredential: authority.request,
      resolveAccountCredential: authority.resolve,
      isInitiatingAuthorityCurrent: () => true,
      authorizeBinding: async () => bindingCurrent,
      listAccountCredentialStates: async (
        owner: 'provider' | 'mcp' | 'extension',
        purpose: 'provider-oauth-authorization-state' | 'provider-oauth-token-bundle' |
          'mcp-oauth-authorization-state' | 'mcp-oauth-access-token' |
          'extension-oauth-authorization-state' | 'extension-provider-account-token'
      ) => owner === 'provider' && purpose === 'provider-oauth-token-bundle'
        ? [authority.state(providerTokenScope())]
        : authority.list(owner, purpose),
      now: () => now,
      ttlMs: 30_000
    }
    const lifecycle = createProviderOAuthAuthorizationLifecycle(deps)
    const start = await lifecycle.begin(binding(loopback.origin), initiatingAuthority)
    await completeAuthorization(lifecycle, binding(loopback.origin), start, 'refresh-inventory-code')
    const timers = new Map<number, { callback: () => Promise<void>; delayMs: number }>()
    let nextTimer = 0
    const scheduler = createProviderOAuthRefreshScheduler({
      refreshInventory: (refreshWindowMs) => lifecycle.refreshExpiringAccounts(refreshWindowMs),
      now: () => now,
      setTimer: (callback, delayMs) => {
        const id = ++nextTimer
        timers.set(id, { callback, delayMs })
        return id
      },
      clearTimer: (id) => { timers.delete(id as number) }
    })
    await scheduler.start()
    expect(loopback.requests.filter((request) =>
      request.path === '/token' && request.form.get('grant_type') === 'refresh_token'
    )).toHaveLength(0)
    expect([...timers.values()].map(({ delayMs }) => delayMs)).toEqual([60_000])

    scheduler.stop()
    expect(timers.size).toBe(0)
    now += 3_360_001
    await scheduler.start()
    expect(loopback.requests.filter((request) =>
      request.path === '/token' && request.form.get('grant_type') === 'refresh_token'
    )).toHaveLength(1)
    expect(timers.size).toBe(1)
    expect(authority.credential(providerTokenScope())).toMatchObject({
      kind: 'provider-oauth-bundle', accessToken: 'synthetic-access-refreshed'
    })

    bindingCurrent = false
    const drifted = [...timers.values()][0]
    timers.clear()
    await drifted?.callback()
    expect(timers.size).toBe(0)
    expect(authority.state(providerTokenScope()).status).toBe('absent')
    expect(loopback.requests.filter((request) =>
      request.path === '/token' && request.form.get('grant_type') === 'refresh_token'
    )).toHaveLength(1)
  })
})
