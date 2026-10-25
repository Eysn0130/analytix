import type { ProviderRegistryRequest, ProviderRegistryResult } from '@shared/analytix-api'

export type LocalProviderRecoveryReason = 'saved_connection_unavailable' | 'credential_reentry_required'

export type LocalProviderReadiness =
  | { kind: 'setup' }
  | { kind: 'ready'; providerId: string; model: string }
  | { kind: 'recovery'; reason: LocalProviderRecoveryReason }

// Listing references classifies setup vs recovery; it does not prove that the
// OS-backed credential is accessible in this process. No network probe runs here.
export async function checkLocalProviderReadiness(
  request: (request: ProviderRegistryRequest) => Promise<ProviderRegistryResult>
): Promise<LocalProviderReadiness> {
  try {
    const snapshot = await request({ schemaVersion: 1, operation: 'list' })
    const readiness = resolveLocalProviderReadiness(snapshot)
    if (readiness.kind !== 'ready' || !('providers' in snapshot)) return readiness
    const provider = snapshot.providers.find((item) => item.id === readiness.providerId)!
    const expected = {
      registryRevision: snapshot.registryRevision,
      registryIncarnation: snapshot.registryIncarnation,
      providerRevision: provider.revision,
      providerGeneration: provider.generation,
      providerIncarnation: provider.incarnation,
      providerCredentialPurpose: provider.credentialPurpose ?? ''
    }
    const result = await request({
      schemaVersion: 1, operation: 'credential-check', providerId: provider.id, expected
    })
    if ('error' in result && result.error.code === 'credential_reentry_required') {
      return { kind: 'recovery', reason: 'credential_reentry_required' }
    }
    if ('credentialAvailable' in result && result.credentialAvailable === true &&
      result.providerId === provider.id && result.registryRevision === expected.registryRevision &&
      result.registryIncarnation === expected.registryIncarnation &&
      result.providerRevision === expected.providerRevision &&
      result.providerGeneration === expected.providerGeneration &&
      result.providerIncarnation === expected.providerIncarnation) return readiness
  } catch { /* A failed read must preserve initialization and allow retry. */ }
  return localProviderRecoveryReadiness()
}

export function resolveLocalProviderReadiness(
  result: ProviderRegistryResult
): LocalProviderReadiness {
  if ('error' in result || !('providers' in result)) {
    return { kind: 'recovery', reason: 'saved_connection_unavailable' }
  }

  const selectedId = result.selectedProviderId?.trim() ?? ''
  if (result.providers.length === 0 && !selectedId) return { kind: 'setup' }
  const configured = result.providers.filter((provider) => !provider.tombstone)

  const selected = configured.find((provider) => provider.id === selectedId)
  if (!selected || !selected.credentialConfigured || !selected.credentialPurpose ||
    !selected.selectedModel || !selected.models.includes(selected.selectedModel)) {
    return { kind: 'recovery', reason: 'saved_connection_unavailable' }
  }

  return { kind: 'ready', providerId: selected.id, model: selected.selectedModel }
}

export function localProviderRecoveryReadiness(): LocalProviderReadiness {
  return { kind: 'recovery', reason: 'saved_connection_unavailable' }
}
