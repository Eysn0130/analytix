import {
  providerRegistrySnapshotResponseSchemaV1,
  type ProviderRegistryResultV1
} from '../../packages/runtime/src/contracts/provider-registry'
import { isComposerChatModelId } from '../shared/app-settings'
import type { ModelProviderModelGroup, UpstreamModelsResult } from '../shared/analytix-api'
import { providerDisplayName, providerEndpointKind, providerModelDisplayName } from '../shared/provider-display'

/**
 * Projects the Core Registry's committed, non-secret catalog for every composer.
 * The historical IPC name does not authorize upstream model discovery, settings
 * fallback, or credential reads. Model capabilities absent from the Registry
 * remain absent here; presets are not evidence of committed capabilities.
 */
export async function fetchUpstreamModelIds(registry: ProviderRegistryResultV1): Promise<UpstreamModelsResult> {
  const parsed = providerRegistrySnapshotResponseSchemaV1.safeParse(registry)
  if (!parsed.success) return { ok: false, message: 'Provider model catalog is unavailable.' }

  const snapshot = parsed.data
  const modelGroups: ModelProviderModelGroup[] = []
  for (const provider of snapshot.providers) {
    if (provider.tombstone || !provider.credentialConfigured) continue
    const modelIds = provider.models.filter((model) => isComposerChatModelId(model, provider.mediaModels))
      .sort((left, right) => left.localeCompare(right))
    if (modelIds.length === 0) continue
    const modelLabels = Object.fromEntries(modelIds.flatMap((id) => {
      const label = providerModelDisplayName(id)
      return label === id ? [] : [[id, label]]
    }))
    modelGroups.push({
      providerId: provider.id,
      label: providerDisplayName(provider.id, provider.id),
      endpointKind: providerEndpointKind(provider.endpoint),
      modelIds,
      ...(Object.keys(modelLabels).length > 0 ? { modelLabels } : {})
    })
  }

  // Existing consumers resolve an otherwise unbound model to its first group.
  // Preserve the Registry's selected provider when providers share a model ID.
  const selectedIndex = modelGroups.findIndex((group) => group.providerId === snapshot.selectedProviderId)
  if (selectedIndex > 0) modelGroups.unshift(...modelGroups.splice(selectedIndex, 1))
  const selectedProvider = snapshot.providers.find((provider) => provider.id === snapshot.selectedProviderId)
  const selectedModel = selectedProvider?.selectedModel
  const defaultModelId = selectedModel && modelGroups.some((group) =>
    group.providerId === selectedProvider?.id && group.modelIds.includes(selectedModel)
  ) ? selectedModel : undefined

  return {
    ok: true,
    modelIds: [...new Set(modelGroups.flatMap((group) => group.modelIds))].sort((left, right) => left.localeCompare(right)),
    ...(defaultModelId ? { defaultModelId } : {}),
    modelGroups
  }
}
