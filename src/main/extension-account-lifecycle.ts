import type {
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1,
  AccountCredentialStateV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  accountCredentialExpectedState,
  type MainAccountCredentialResolution
} from './ipc/provider-registry-ipc'

type ExtensionAccountLifecycleDeps = {
  requestAccountCredential: (request: AccountCredentialRequestV1) => Promise<AccountCredentialResultV1>
  resolveAccountCredential: (scope: AccountCredentialScopeV1) => Promise<MainAccountCredentialResolution>
  listAccountCredentialStates: (
    owner: 'extension',
    purpose: 'extension-provider-account-token'
  ) => Promise<AccountCredentialStateV1[]>
  authorizeBinding: (binding: VerifiedInstalledExtensionAccountBindingV1) => Promise<boolean>
}

export type VerifiedInstalledExtensionAccountBindingV1 = {
  pluginId: string
  serverId: string
  accountId: string
  ownerFingerprint: string
}

function bindingKey(input: VerifiedInstalledExtensionAccountBindingV1): string {
  return JSON.stringify(scope(input))
}

function exactComponent(value: string, maximum: number): string {
  const result = value.trim()
  if (!result || result !== value || Buffer.byteLength(result, 'utf8') > maximum || /[\u0000\r\n\t]/.test(result)) {
    throw new Error('Installed extension account binding is invalid.')
  }
  return result
}

function scope(input: VerifiedInstalledExtensionAccountBindingV1): AccountCredentialScopeV1 {
  if (!/^[a-f0-9]{64}$/.test(input.ownerFingerprint)) {
    throw new Error('Installed extension account binding is invalid.')
  }
  return {
    owner: 'extension',
    provider: exactComponent(input.pluginId, 128),
    accountId: exactComponent(input.accountId, 256),
    channelId: exactComponent(input.serverId, 256),
    purpose: 'extension-provider-account-token'
  }
}

function isFailure(
  result: AccountCredentialResultV1
): result is Extract<AccountCredentialResultV1, { error: unknown }> {
  return 'error' in result
}

export function createInstalledExtensionAccountLifecycle(deps: ExtensionAccountLifecycleDeps) {
  const mutate = async (
    binding: VerifiedInstalledExtensionAccountBindingV1,
    disposition: 'revoke' | 'delete'
  ): Promise<void> => {
    if (!await deps.authorizeBinding(binding)) {
      throw new Error('Installed extension account binding is unavailable.')
    }
    const exactScope = scope(binding)
    const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope: exactScope })
    if (isFailure(current)) throw new Error('Installed extension account authority is unavailable.')
    if (current.status === 'absent') return
    const result = await deps.requestAccountCredential({
      schemaVersion: 1, operation: disposition, scope: exactScope,
      expected: accountCredentialExpectedState(current)
    })
    const wanted = disposition === 'revoke' ? 'revoked' : 'absent'
    if (isFailure(result) || result.status !== wanted) {
      throw new Error('Installed extension account mutation lost its current-generation fence.')
    }
  }

  return {
    /** Main-only producer. The binding must come from verified installed-plugin discovery. */
    async replace(
      binding: VerifiedInstalledExtensionAccountBindingV1,
      token: string
    ): Promise<AccountCredentialStateV1> {
      if (!await deps.authorizeBinding(binding)) {
        throw new Error('Installed extension account binding is unavailable.')
      }
      const exactScope = scope(binding)
      const boundedToken = exactComponent(token, 32 * 1024)
      const current = await deps.requestAccountCredential({ schemaVersion: 1, operation: 'status', scope: exactScope })
      if (isFailure(current)) throw new Error('Installed extension account authority is unavailable.')
      const result = await deps.requestAccountCredential({
        schemaVersion: 1, operation: 'put', scope: exactScope,
        expected: accountCredentialExpectedState(current),
        credential: {
          kind: 'extension-account-token', token: boundedToken,
          bindingFingerprint: binding.ownerFingerprint
        }
      })
      if (isFailure(result) || result.status !== 'ready') {
        throw new Error('Installed extension account replacement lost its current-generation fence.')
      }
      return result
    },

    revoke(binding: VerifiedInstalledExtensionAccountBindingV1): Promise<void> {
      return mutate(binding, 'revoke')
    },

    delete(binding: VerifiedInstalledExtensionAccountBindingV1): Promise<void> {
      return mutate(binding, 'delete')
    },

    async reconcileManagedAccounts(
      bindings: readonly VerifiedInstalledExtensionAccountBindingV1[]
    ): Promise<{ deleted: number; retained: number }> {
      if (bindings.length > 128) throw new Error('Installed extension account inventory is unavailable.')
      const allowed = new Map(bindings.map((binding) => [bindingKey(binding), binding.ownerFingerprint]))
      const states = await deps.listAccountCredentialStates('extension', 'extension-provider-account-token')
      if (states.length > 128) throw new Error('Installed extension account inventory is unavailable.')
      let deleted = 0
      let retained = 0
      for (const state of states) {
        if (state.status === 'absent') continue
        const fingerprint = allowed.get(JSON.stringify(state.scope))
        if (fingerprint) {
          const resolution = await deps.resolveAccountCredential(state.scope)
          const currentFingerprint = resolution.ok
            ? resolution.credential.kind === 'extension-account-token'
              ? resolution.credential.bindingFingerprint
              : resolution.credential.kind === 'extension-oauth-bundle'
                ? resolution.credential.oauthBinding.ownerFingerprint
                : ''
            : ''
          if (currentFingerprint === fingerprint) {
            retained++
            continue
          }
        }
        const result = await deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope: state.scope,
          expected: accountCredentialExpectedState(state)
        })
        if (isFailure(result) || result.status !== 'absent') {
          throw new Error('Installed extension account reconciliation lost its current-generation fence.')
        }
        deleted++
      }
      return { deleted, retained }
    },

    async deleteVerifiedPluginAccounts(pluginId: string): Promise<number> {
      const provider = exactComponent(pluginId, 128)
      const states = await deps.listAccountCredentialStates('extension', 'extension-provider-account-token')
      if (states.length > 128) throw new Error('Installed extension account inventory is unavailable.')
      let deleted = 0
      for (const state of states) {
        if (state.scope.owner !== 'extension' || state.scope.provider !== provider || !state.scope.channelId) continue
        if (state.status === 'absent') continue
        const result = await deps.requestAccountCredential({
          schemaVersion: 1, operation: 'delete', scope: state.scope,
          expected: accountCredentialExpectedState(state)
        })
        if (isFailure(result) || result.status !== 'absent') {
          throw new Error('Installed extension account deletion lost its current-generation fence.')
        }
        deleted++
      }
      return deleted
    }
  }
}

export type InstalledExtensionAccountLifecycle = ReturnType<typeof createInstalledExtensionAccountLifecycle>
