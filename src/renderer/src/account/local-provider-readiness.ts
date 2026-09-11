import type { ProviderRegistryResult } from '@shared/analytix-api'

export type LocalProviderReadiness =
  | { kind: 'setup' }
  | { kind: 'ready'; providerId: string }
  | { kind: 'recovery'; message: string }

const LOCAL_PROVIDER_RECOVERY_MESSAGE =
  'Local Provider recovery is required. Open Settings to review the selected Provider.'

export function resolveLocalProviderReadiness(
  result: ProviderRegistryResult
): LocalProviderReadiness {
  if ('error' in result || !('providers' in result)) {
    return { kind: 'recovery', message: LOCAL_PROVIDER_RECOVERY_MESSAGE }
  }

  const selectedId = result.selectedProviderId?.trim() ?? ''
  if (result.providers.length === 0 && !selectedId) return { kind: 'setup' }
  const configured = result.providers.filter((provider) => !provider.tombstone)

  const selected = configured.find((provider) => provider.id === selectedId)
  if (!selected || !selected.credentialConfigured) {
    return { kind: 'recovery', message: LOCAL_PROVIDER_RECOVERY_MESSAGE }
  }

  return { kind: 'ready', providerId: selected.id }
}

export function localProviderRecoveryReadiness(): LocalProviderReadiness {
  return { kind: 'recovery', message: LOCAL_PROVIDER_RECOVERY_MESSAGE }
}
