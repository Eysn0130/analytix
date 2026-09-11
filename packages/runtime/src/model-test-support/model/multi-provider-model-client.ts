import type { ModelCapabilityMetadata } from '../../contracts/capabilities.js'
import type { VisionBridgeCapabilityConfig } from '../../contracts/capabilities.js'
import {
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  normalizeModelEndpointFormat,
  type ModelEndpointFormat
} from '../../contracts/model-endpoint-format.js'
import type { ModelProviderConfig } from '../../config/analytix-config.js'
import type { ModelClient, ModelRequest, ModelStreamChunk } from '../../ports/model-client.js'
import type { ThreadStore } from '../../ports/thread-store.js'
import type { LlmDebugSink } from '../../services-test-support/llm-debug-recorder.js'
import { CompatModelClient, type CompatModelClientConfig } from './compat-model-client.js'

export type ModelClientDiagnostics = {
  provider?: string
  providerId?: string
  providerBaseUrl?: string
  endpointFormat?: string
  configuredModel?: string
}

export type MultiProviderModelClientOptions = {
  defaultClient: CompatModelClient
  threadStore: ThreadStore
  providers: readonly ModelProviderConfig[]
  defaultProviderId?: string
  defaultApiKey: string
  defaultModel: string
  defaultModelProxyUrl?: string
  defaultEndpointFormat?: ModelEndpointFormat
  streamIdleTimeoutMs?: number
  fetchImpl?: typeof fetch
  modelCapabilities?: (model: string) => ModelCapabilityMetadata
  visionBridge?: VisionBridgeCapabilityConfig
  debugSink?: LlmDebugSink
}

type ProviderSelection = {
  providerId?: string
  client: CompatModelClient
  config: CompatModelClientConfig
}

type ProviderNotFoundSource = 'request' | 'thread' | 'default-provider'

export class MultiProviderModelClient implements ModelClient {
  readonly provider = 'multi-provider'
  readonly model: string

  private readonly defaultConfig: CompatModelClientConfig
  private readonly clients = new Map<string, ProviderSelection>()
  private readonly providers = new Map<string, ModelProviderConfig>()

  constructor(private readonly options: MultiProviderModelClientOptions) {
    this.model = options.defaultClient.model
    this.defaultConfig = {
      baseUrl: options.defaultClient.config.baseUrl,
      apiKey: options.defaultApiKey,
      model: options.defaultModel,
      modelProxyUrl: options.defaultModelProxyUrl,
      endpointFormat: options.defaultEndpointFormat ?? DEFAULT_MODEL_ENDPOINT_FORMAT,
      modelCapabilities: options.modelCapabilities,
      visionBridge: options.visionBridge,
      debugSink: options.debugSink,
      ...(options.fetchImpl ? { fetchImpl: options.fetchImpl } : {}),
      ...(options.streamIdleTimeoutMs !== undefined ? { streamIdleTimeoutMs: options.streamIdleTimeoutMs } : {})
    }
    for (const provider of options.providers) {
      const id = provider.id.trim()
      if (!id || !provider.baseUrl.trim()) continue
      this.providers.set(id, { ...provider, id })
    }
  }

  async *stream(request: ModelRequest): AsyncIterable<ModelStreamChunk> {
    const selection = await this.selectionForRequest(request)
    yield* selection.client.stream(request)
  }

  async diagnosticsForRequest(request: Pick<ModelRequest, 'threadId' | 'model' | 'providerId'>): Promise<ModelClientDiagnostics> {
    const selection = await this.selectionForRequest(request)
    const requestModel = request.model?.trim() || selection.config.model
    return {
      provider: selection.client.provider,
      ...(selection.providerId ? { providerId: selection.providerId } : {}),
      providerBaseUrl: selection.config.baseUrl,
      endpointFormat: endpointFormatForModel(selection.config, requestModel),
      configuredModel: requestModel
    }
  }

  private async selectionForRequest(
    request: Pick<ModelRequest, 'threadId' | 'model' | 'providerId'>
  ): Promise<ProviderSelection> {
    const thread = await this.options.threadStore.get(request.threadId)
    const requestProviderId = request.providerId?.trim()
    const threadProviderId = thread?.providerId?.trim()
    const defaultProviderId = this.options.defaultProviderId?.trim()
    const providerId = requestProviderId || threadProviderId || defaultProviderId
    if (!providerId) {
      return {
        client: this.options.defaultClient,
        config: this.defaultClientConfig(request.model)
      }
    }
    const provider = this.providers.get(providerId)
    if (!provider) {
      if (requestProviderId || threadProviderId) {
        throw providerNotFoundError(providerId, requestProviderId ? 'request' : 'thread', request.threadId)
      }
      return {
        client: this.options.defaultClient,
        config: this.defaultClientConfig(request.model)
      }
    }
    validateProviderRequestModel(provider, request.model)
    const cached = this.clients.get(providerId)
    if (cached) return cached
    const config: CompatModelClientConfig = {
      baseUrl: provider.baseUrl,
      apiKey: provider.apiKey ?? this.options.defaultApiKey,
      model: provider.models[0] ?? this.options.defaultModel,
      modelProxyUrl: provider.modelProxyUrl ?? this.options.defaultModelProxyUrl,
      endpointFormat: provider.endpointFormat ?? DEFAULT_MODEL_ENDPOINT_FORMAT,
      modelCapabilities: this.options.modelCapabilities,
      visionBridge: this.options.visionBridge,
      debugSink: this.options.debugSink,
      ...(this.options.fetchImpl ? { fetchImpl: this.options.fetchImpl } : {}),
      ...(this.options.streamIdleTimeoutMs !== undefined
        ? { streamIdleTimeoutMs: this.options.streamIdleTimeoutMs }
        : {})
    }
    const selection = {
      providerId,
      client: new CompatModelClient(config),
      config
    }
    this.clients.set(providerId, selection)
    return selection
  }

  private defaultClientConfig(model: string): CompatModelClientConfig {
    return {
      ...this.defaultConfig,
      model: model.trim() || this.options.defaultModel,
      baseUrl: compatBaseUrl(this.options.defaultClient)
    }
  }
}

function compatBaseUrl(client: CompatModelClient): string {
  return client.config.baseUrl
}

function endpointFormatForModel(config: CompatModelClientConfig, model: string): ModelEndpointFormat {
  const perModel = config.modelCapabilities?.(model).endpointFormat
  return normalizeModelEndpointFormat(perModel ?? config.endpointFormat ?? DEFAULT_MODEL_ENDPOINT_FORMAT)
}

function providerNotFoundError(providerId: string, source: ProviderNotFoundSource, threadId: string): Error {
  return Object.assign(new Error(`configured model provider was not found: ${providerId}`), {
    code: 'provider_not_found',
    providerId,
    source,
    threadId
  })
}

function validateProviderRequestModel(provider: ModelProviderConfig, model: string | undefined): void {
  const requested = normalizeModelLookup(model)
  if (!requested || !providerHasBoundedModelCatalog(provider)) return
  if (providerModelMatches(provider, requested)) return
  throw Object.assign(new Error(`model ${model?.trim() ?? ''} is not configured for provider ${provider.id}`), {
    code: 'invalid_model',
    providerId: provider.id,
    model: model?.trim() ?? '',
    configuredModel: provider.models[0] ?? Object.keys(provider.modelProfiles ?? {}).sort()[0] ?? ''
  })
}

function providerHasBoundedModelCatalog(provider: ModelProviderConfig): boolean {
  return provider.models.some((model) => model.trim()) || Object.keys(provider.modelProfiles ?? {}).some((model) => model.trim())
}

function providerModelMatches(provider: ModelProviderConfig, requested: string): boolean {
  if (provider.models.some((model) => normalizeModelLookup(model) === requested)) return true
  const entries = Object.entries(provider.modelProfiles ?? {})
  return entries.some(([model, profile]) =>
    normalizeModelLookup(model) === requested ||
    (profile.aliases ?? []).some((alias) => normalizeModelLookup(alias) === requested)
  )
}

function normalizeModelLookup(model: string | undefined): string {
  return model?.trim().toLowerCase() ?? ''
}
