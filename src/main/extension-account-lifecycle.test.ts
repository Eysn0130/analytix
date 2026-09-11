import { describe, expect, it, vi } from 'vitest'
import type {
  AccountCredentialRequestV1,
  AccountCredentialStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import { createInstalledExtensionAccountLifecycle } from './extension-account-lifecycle'

const registryIncarnation = `inc_${'e'.repeat(43)}`
const providerIncarnation = `inc_${'f'.repeat(43)}`
const ownerFingerprint = 'a'.repeat(64)

function state(
  scope: AccountCredentialStateV1['scope'],
  status: AccountCredentialStateV1['status'],
  revision = '1'
): AccountCredentialStateV1 {
  const absent = status === 'absent'
  return {
    schemaVersion: 1, scope, status, registryRevision: revision, registryIncarnation,
    providerRevision: absent ? '0' : revision,
    providerGeneration: absent ? '0' : revision,
    providerIncarnation: absent ? '' : providerIncarnation
  }
}

describe('installed extension account lifecycle', () => {
  it('binds acquisition/revoke/delete to the verified plugin/server/account and returns no token', async () => {
    const binding = {
      pluginId: 'extension-demo', serverId: 'server-demo', accountId: 'account-demo', ownerFingerprint
    }
    const scope = {
      owner: 'extension' as const, provider: binding.pluginId, accountId: binding.accountId,
      channelId: binding.serverId, purpose: 'extension-provider-account-token' as const
    }
    let current = state(scope, 'absent', '0')
    const requests: AccountCredentialRequestV1[] = []
    const requestAccountCredential = vi.fn(async (request: AccountCredentialRequestV1) => {
      requests.push(structuredClone(request))
      if (request.operation === 'status') return current
      if (request.operation === 'put') current = state(scope, 'ready', '1')
      if (request.operation === 'revoke') current = state(scope, 'revoked', '2')
      if (request.operation === 'delete') current = state(scope, 'absent', '3')
      return current
    })
    const lifecycle = createInstalledExtensionAccountLifecycle({
      requestAccountCredential,
      resolveAccountCredential: vi.fn(async () => ({
        ok: true as const, state: current,
        credential: { kind: 'extension-account-token' as const, token, bindingFingerprint: ownerFingerprint }
      })),
      listAccountCredentialStates: vi.fn(async () => [current]),
      authorizeBinding: vi.fn(async () => true)
    })
    const token = 'synthetic-extension-account-token'
    const ready = await lifecycle.replace(binding, token)
    expect(JSON.stringify(ready)).not.toContain(token)
    expect(requests[1]).toMatchObject({
      operation: 'put', scope,
      credential: { kind: 'extension-account-token', token, bindingFingerprint: ownerFingerprint }
    })
    await lifecycle.revoke(binding)
    expect(current.status).toBe('revoked')
    await lifecycle.delete(binding)
    expect(current.status).toBe('absent')
  })

  it('deletes only accounts belonging to the exact verified plugin identity', async () => {
    const ownedScope = {
      owner: 'extension' as const, provider: 'extension-demo', accountId: 'account-a',
      channelId: 'server-a', purpose: 'extension-provider-account-token' as const
    }
    const foreignScope = { ...ownedScope, provider: 'extension-other', channelId: 'server-other' }
    const deleted = vi.fn(async (request: AccountCredentialRequestV1) =>
      request.operation === 'delete' ? state(request.scope, 'absent', '4') : state(request.scope, 'ready', '3'))
    const lifecycle = createInstalledExtensionAccountLifecycle({
      requestAccountCredential: deleted,
      resolveAccountCredential: vi.fn() as never,
      listAccountCredentialStates: vi.fn(async () => [
        state(ownedScope, 'ready', '3'), state(foreignScope, 'ready', '3')
      ]),
      authorizeBinding: vi.fn(async () => true)
    })
    await expect(lifecycle.deleteVerifiedPluginAccounts('extension-demo')).resolves.toBe(1)
    expect(deleted).toHaveBeenCalledTimes(1)
    expect(deleted).toHaveBeenCalledWith(expect.objectContaining({ operation: 'delete', scope: ownedScope }))
  })

  it('rejects acquisition outside canonical installed-extension bindings and reconciles removed accounts', async () => {
    const retainedScope = {
      owner: 'extension' as const, provider: 'extension-demo', accountId: 'account-a',
      channelId: 'server-a', purpose: 'extension-provider-account-token' as const
    }
    const removedScope = { ...retainedScope, accountId: 'account-b', channelId: 'server-b' }
    const requestAccountCredential = vi.fn(async (request: AccountCredentialRequestV1) => (
      request.operation === 'delete'
        ? state(request.scope, 'absent', '4')
        : state(request.scope, 'ready', '3')
    ))
    const lifecycle = createInstalledExtensionAccountLifecycle({
      requestAccountCredential,
      resolveAccountCredential: vi.fn(async (scope) => ({
        ok: true as const, state: state(scope, 'ready', '3'),
        credential: {
          kind: 'extension-account-token' as const, token: 'synthetic-retained', bindingFingerprint: ownerFingerprint
        }
      })),
      listAccountCredentialStates: vi.fn(async () => [
        state(retainedScope, 'ready', '3'), state(removedScope, 'ready', '3')
      ]),
      authorizeBinding: vi.fn(async (binding) => binding.serverId === 'server-a')
    })
    await expect(lifecycle.replace({
      pluginId: 'extension-demo', serverId: 'server-b', accountId: 'account-b', ownerFingerprint
    }, 'synthetic-unbound-token')).rejects.toThrow('Installed extension account binding is unavailable.')
    expect(requestAccountCredential).not.toHaveBeenCalled()
    await expect(lifecycle.reconcileManagedAccounts([{
      pluginId: 'extension-demo', serverId: 'server-a', accountId: 'account-a', ownerFingerprint
    }])).resolves.toEqual({ deleted: 1, retained: 1 })
    expect(requestAccountCredential).toHaveBeenCalledTimes(1)
    expect(requestAccountCredential).toHaveBeenCalledWith(expect.objectContaining({
      operation: 'delete', scope: removedScope
    }))
  })
})
