import type {
  OAuthAuthorizationBindingV1,
  OAuthAuthorizationPublicStatusV1,
  OAuthAccountStatusV1,
  createProviderOAuthAuthorizationLifecycle
} from './provider-oauth-lifecycle'
import type {
  ProviderRegistryOAuthBindingV1,
  ProviderRegistryProviderInputV1,
  ProviderRegistryPublicProviderV1
} from '../../packages/runtime/src/contracts/provider-registry.js'

export type ProviderOAuthLifecycle = ReturnType<typeof createProviderOAuthAuthorizationLifecycle>

export function providerRegistryInputWithOAuthBinding(
  provider: ProviderRegistryPublicProviderV1,
  oauthBinding: ProviderRegistryOAuthBindingV1
): ProviderRegistryProviderInputV1 {
  return {
    id: provider.id,
    kind: provider.kind,
    endpoint: provider.endpoint,
    proxy: provider.proxy ?? '',
    models: [...provider.models],
    mediaModels: [...provider.mediaModels],
    selectedModel: provider.selectedModel ?? '',
    selectedMediaModel: provider.selectedMediaModel ?? '',
    selectedRoutes: [...provider.selectedRoutes],
    oauthBinding,
    ...(provider.accountObservation ? { accountObservation: provider.accountObservation } : {})
  }
}

export type ProviderOAuthPublicAuthorizationV1 = {
  ok: true
  authorizationId: string
  expiresAt: string
  phase: 'pending' | 'processing'
}

export type ProviderOAuthPublicFailureV1 = {
  ok: false
  code: 'invalid_request' | 'unavailable' | 'forbidden' | 'conflict'
  message: string
}

export type ProviderOAuthPublicResultV1 =
  | ProviderOAuthPublicAuthorizationV1
  | ProviderOAuthPublicFailureV1

export type OAuthBindingTargetV1 =
  | { owner: 'provider'; providerId: string }
  | { owner: 'mcp'; serverId: string; accountId: string }
  | { owner: 'extension'; pluginId: string; serverId: string; accountId: string }

export function providerOAuthOwnerFenceFromProjection(provider: {
  revision: string
  generation: string
  incarnation: string
}): Pick<OAuthAuthorizationBindingV1, 'ownerRevision' | 'ownerGeneration' | 'ownerIncarnation'> {
  return {
    ownerRevision: provider.revision,
    ownerGeneration: provider.generation,
    ownerIncarnation: provider.incarnation
  }
}

type MainOAuthAuthorityDeps = {
  lifecycle: ProviderOAuthLifecycle
  profileBinding: string
  resolveBinding: (target: OAuthBindingTargetV1) => Promise<OAuthAuthorizationBindingV1 | null>
  openExternal: (authorizationUrl: string) => Promise<void>
  configureProviderBinding: (input: {
    providerId: string
    oauthBinding: ProviderRegistryOAuthBindingV1
  }) => Promise<void>
}

const unavailable = (): ProviderOAuthPublicFailureV1 => ({
  ok: false,
  code: 'unavailable',
  message: 'OAuth account management is unavailable.'
})

function publicAuthorization(status: OAuthAuthorizationPublicStatusV1): ProviderOAuthPublicAuthorizationV1 {
  return {
    ok: true,
    authorizationId: status.authorizationId,
    expiresAt: new Date(status.expiresAtMs).toISOString(),
    phase: status.phase
  }
}

export function createMainOAuthAccountAuthority(deps: MainOAuthAuthorityDeps) {
  const bindingFor = async (target: OAuthBindingTargetV1): Promise<OAuthAuthorizationBindingV1> => {
    const binding = await deps.resolveBinding(target)
    if (!binding) throw new Error('OAuth account binding is unavailable.')
    return binding
  }

  const begin = async (
    target: OAuthBindingTargetV1,
    authority: { webContentsId: number }
  ): Promise<ProviderOAuthPublicResultV1> => {
    try {
      const binding = await bindingFor(target)
      const started = await deps.lifecycle.begin(binding, authority)
      try {
        await deps.openExternal(started.authorizationUrl)
      } catch {
        await deps.lifecycle.cancelAuthorization({
          authorizationId: started.authorizationId,
          profileBinding: deps.profileBinding,
          webContentsId: authority.webContentsId,
          expectedOwner: target.owner
        }).catch(() => undefined)
        return unavailable()
      }
      return publicAuthorization({
        authorizationId: started.authorizationId,
        expiresAtMs: started.expiresAtMs,
        phase: 'pending'
      })
    } catch {
      return unavailable()
    }
  }

  const status = async (
    expectedOwner: OAuthAuthorizationBindingV1['owner'],
    authorizationId: string,
    authority: { webContentsId: number }
  ): Promise<ProviderOAuthPublicResultV1> => {
    try {
      const current = await deps.lifecycle.authorizationStatus({
        authorizationId,
        profileBinding: deps.profileBinding,
        webContentsId: authority.webContentsId,
        expectedOwner
      })
      return current ? publicAuthorization(current) : unavailable()
    } catch {
      return unavailable()
    }
  }

  const cancel = async (
    expectedOwner: OAuthAuthorizationBindingV1['owner'],
    authorizationId: string,
    authority: { webContentsId: number }
  ): Promise<{ ok: true; cancelled: true } | ProviderOAuthPublicFailureV1> => {
    try {
      await deps.lifecycle.cancelAuthorization({
        authorizationId,
        profileBinding: deps.profileBinding,
        webContentsId: authority.webContentsId,
        expectedOwner
      })
      return { ok: true, cancelled: true }
    } catch {
      return unavailable()
    }
  }

  const accountAction = async <T>(
    target: OAuthBindingTargetV1,
    action: (binding: OAuthAuthorizationBindingV1) => Promise<T>
  ): Promise<{ ok: true } | ProviderOAuthPublicFailureV1> => {
    try {
      await action(await bindingFor(target))
      return { ok: true }
    } catch {
      return unavailable()
    }
  }

  return {
    configureProvider: async (input: {
      providerId: string
      oauthBinding: ProviderRegistryOAuthBindingV1
    }) => {
      try {
        await deps.configureProviderBinding(input)
        return { ok: true as const }
      } catch {
        return unavailable()
      }
    },
    beginProvider: (input: { providerId: string }, authority: { webContentsId: number }) =>
      begin({ owner: 'provider', providerId: input.providerId }, authority),
    beginMcp: (input: { serverId: string; accountId: string }, authority: { webContentsId: number }) =>
      begin({ owner: 'mcp', serverId: input.serverId, accountId: input.accountId }, authority),
    beginExtension: (
      input: { pluginId: string; serverId: string; accountId: string },
      authority: { webContentsId: number }
    ) => begin({ owner: 'extension', ...input }, authority),
    statusProvider: (authorizationId: string, authority: { webContentsId: number }) =>
      status('provider', authorizationId, authority),
    cancelProvider: (authorizationId: string, authority: { webContentsId: number }) =>
      cancel('provider', authorizationId, authority),
    statusMcp: (authorizationId: string, authority: { webContentsId: number }) =>
      status('mcp', authorizationId, authority),
    cancelMcp: (authorizationId: string, authority: { webContentsId: number }) =>
      cancel('mcp', authorizationId, authority),
    statusExtension: (authorizationId: string, authority: { webContentsId: number }) =>
      status('extension', authorizationId, authority),
    cancelExtension: (authorizationId: string, authority: { webContentsId: number }) =>
      cancel('extension', authorizationId, authority),
    revokeProvider: (input: { providerId: string }) => accountAction(
      { owner: 'provider', providerId: input.providerId }, deps.lifecycle.revoke
    ),
    deleteProvider: (input: { providerId: string }) => accountAction(
      { owner: 'provider', providerId: input.providerId }, deps.lifecycle.delete
    ),
    replaceProviderSubscription: async (input: { providerId: string; subscriptionToken: string }) => {
      try {
        const binding = await bindingFor({ owner: 'provider', providerId: input.providerId })
        await deps.lifecycle.replaceSubscription(binding, input.subscriptionToken)
        return { ok: true as const }
      } catch {
        return unavailable()
      }
    },
    revokeMcp: (input: { serverId: string; accountId: string }) => accountAction(
      { owner: 'mcp', ...input }, deps.lifecycle.revoke
    ),
    deleteMcp: (input: { serverId: string; accountId: string }) => accountAction(
      { owner: 'mcp', ...input }, deps.lifecycle.delete
    ),
    revokeExtension: (input: { pluginId: string; serverId: string; accountId: string }) => accountAction(
      { owner: 'extension', ...input }, deps.lifecycle.revoke
    ),
    deleteExtension: (input: { pluginId: string; serverId: string; accountId: string }) => accountAction(
      { owner: 'extension', ...input }, deps.lifecycle.delete
    ),
    async handleNativeCallback(callbackUrl: string): Promise<OAuthAccountStatusV1 | null> {
      try {
        return await deps.lifecycle.completeNativeCallback({ callbackUrl, profileBinding: deps.profileBinding })
      } catch {
        return null
      }
    }
  }
}

export type MainOAuthAccountAuthority = ReturnType<typeof createMainOAuthAccountAuthority>
