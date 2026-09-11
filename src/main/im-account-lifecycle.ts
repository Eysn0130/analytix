import type {
  AccountCredentialRequestV1,
  AccountCredentialResultV1,
  AccountCredentialScopeV1
} from '../../packages/runtime/src/contracts/provider-registry.js'
import {
  accountCredentialExpectedState,
  type MainAccountCredentialResolution
} from './ipc/provider-registry-ipc'
import type {
  JsonSettingsStore,
  LegacyImAccountCredentialCandidateV1
} from './settings-store'

type ImAccountLifecycleDeps = {
  store: Pick<JsonSettingsStore,
    'inspectLegacyImAccountCredentials' | 'finalizeLegacyImAccountCredentialMigration'>
  requestAccountCredential: (request: AccountCredentialRequestV1) => Promise<AccountCredentialResultV1>
  resolveAccountCredential: (scope: AccountCredentialScopeV1) => Promise<MainAccountCredentialResolution>
  crash?: { afterCredentialCommit?: (channelId: string) => Promise<void> | void }
}

function failure(
  result: AccountCredentialResultV1
): result is Extract<AccountCredentialResultV1, { error: unknown }> {
  return 'error' in result
}

function credentialsEqual(
  candidate: LegacyImAccountCredentialCandidateV1,
  resolution: MainAccountCredentialResolution
): boolean {
  if (!resolution.ok) return false
  return JSON.stringify(resolution.credential) === JSON.stringify(candidate.credential)
}

/**
 * Migrates only the authenticated canonical settings source. The legacy source
 * remains byte-for-byte unchanged until every exact K1/K2 successor is current
 * and verified. Re-running after any pre-finalization crash is deterministic.
 */
export async function migrateLegacyImAccountCredentials(deps: ImAccountLifecycleDeps): Promise<void> {
  const inspection = await deps.store.inspectLegacyImAccountCredentials()
  if (!inspection) return
  for (const candidate of inspection.candidates) {
    const status = await deps.requestAccountCredential({
      schemaVersion: 1,
      operation: 'status',
      scope: candidate.scope
    })
    if (failure(status)) throw new Error('Legacy IM account credential migration failed.')
    if (status.status === 'ready') {
      if (!credentialsEqual(candidate, await deps.resolveAccountCredential(candidate.scope))) {
        throw new Error('Legacy IM account credential migration failed.')
      }
      continue
    }
    // A prior explicit revoke/disconnect/delete wins over legacy recovery. Do
    // not resurrect it or erase the still-usable legacy source implicitly.
    if (status.status !== 'absent') throw new Error('Legacy IM account credential migration failed.')
    const committed = await deps.requestAccountCredential({
      schemaVersion: 1,
      operation: 'put',
      scope: candidate.scope,
      expected: accountCredentialExpectedState(status),
      credential: candidate.credential
    })
    if (failure(committed) || committed.status !== 'ready' ||
      !credentialsEqual(candidate, await deps.resolveAccountCredential(candidate.scope))) {
      throw new Error('Legacy IM account credential migration failed.')
    }
    await deps.crash?.afterCredentialCommit?.(candidate.channelId)
  }
  await deps.store.finalizeLegacyImAccountCredentialMigration(inspection)
}
