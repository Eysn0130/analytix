import { z } from 'zod'
import {
  ComputerUseMode,
  McpToolDiscoveryMode,
  ModelCapabilityProbeStatus,
  ModelInputModality,
  ModelMessagePartSupport,
  ModelReasoningEffort,
  ModelReasoningRequestProtocol,
  SubagentToolPolicy,
  VisionBridgeMode
} from './capabilities.js'
import { MODEL_ENDPOINT_FORMATS } from './model-endpoint-format.js'

export const PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION = 2 as const

export const PublicRuntimeIdentifierV2 = z.string().min(1).max(128).refine((value) => {
  const lower = value.toLowerCase()
  return value === value.trim() &&
    /^[\p{L}\p{N}._:/@+-]+$/u.test(value) &&
    !value.startsWith('/') &&
    !value.startsWith('~/') &&
    !value.includes('\\') &&
    !lower.includes('://') &&
    !value.includes('../') &&
    !value.includes('/..') &&
    !value.includes('@') &&
    !(value.length >= 3 && value[1] === ':' && value[2] === '/') &&
    !/\p{N}{12}/u.test(value)
})
const PublicIdentifier = PublicRuntimeIdentifierV2
const Sha256 = z.string().regex(/^[a-f0-9]{64}$/)
const Count = z.number().int().nonnegative().max(1_000_000_000)
const PublicCapabilityReasonCode = z.enum(['available', 'disabled_by_config', 'unavailable'])

export const PublicRuntimeCapabilityStateV2 = z
  .object({
    status: z.enum(['available', 'disabled', 'unavailable']),
    enabled: z.boolean(),
    available: z.boolean(),
    reasonCode: PublicCapabilityReasonCode
  })
  .strict()
  .superRefine((value, context) => {
    const expected = value.available
      ? { status: 'available', enabled: true, reasonCode: 'available' }
      : value.enabled
        ? { status: 'unavailable', enabled: true, reasonCode: 'unavailable' }
        : { status: 'disabled', enabled: false, reasonCode: 'disabled_by_config' }
    if (value.status !== expected.status || value.enabled !== expected.enabled || value.reasonCode !== expected.reasonCode) {
      context.addIssue({ code: 'custom', message: 'capability state fields are inconsistent' })
    }
  })

export const PublicModelReasoningV2 = z
  .object({
    supportedEfforts: z.array(ModelReasoningEffort).min(1),
    defaultEffort: ModelReasoningEffort,
    requestProtocol: ModelReasoningRequestProtocol
  })
  .strict()
  .superRefine((value, context) => {
    if (value.requestProtocol === 'none') {
      context.addIssue({ code: 'custom', message: 'reasoning protocol none has no public capability block' })
    }
    if (!value.supportedEfforts.includes(value.defaultEffort)) {
      context.addIssue({ code: 'custom', message: 'reasoning default effort is not supported' })
    }
    if (new Set(value.supportedEfforts).size !== value.supportedEfforts.length) {
      context.addIssue({ code: 'custom', message: 'reasoning efforts must be unique' })
    }
  })

const PublicModelCapabilityV2 = z
  .object({
    id: PublicIdentifier,
    providerId: PublicIdentifier.optional(),
    family: PublicIdentifier.optional(),
    inputModalities: z.array(ModelInputModality).min(1),
    outputModalities: z.array(ModelInputModality).min(1),
    supportsToolCalling: z.boolean(),
    supportsImageInput: z.boolean(),
    contextWindowTokens: Count.optional(),
    messageParts: z.array(ModelMessagePartSupport).min(1),
    reasoning: PublicModelReasoningV2.optional(),
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional()
  })
  .strict()

const PublicCliCapabilitiesV2 = z
  .object({
    serve: PublicRuntimeCapabilityStateV2,
    run: PublicRuntimeCapabilityStateV2,
    chat: PublicRuntimeCapabilityStateV2,
    exec: PublicRuntimeCapabilityStateV2
  })
  .strict()

export const PublicMcpSearchV2 = z
  .object({
    enabled: z.boolean(),
    mode: McpToolDiscoveryMode,
    active: z.boolean(),
    available: z.boolean(),
    reasonCode: PublicCapabilityReasonCode,
    indexedToolCount: Count,
    advertisedToolCount: Count,
    autoThresholdToolCount: Count,
    topKDefault: Count,
    topKMax: Count,
    minScore: z.number().min(0).max(1),
    catalogFingerprint: Sha256.optional(),
    catalogDrift: z.boolean()
  })
  .strict()
  .superRefine((value, context) => {
    const expectedReason = value.available
      ? 'available'
      : value.enabled
        ? 'unavailable'
        : 'disabled_by_config'
    if (value.reasonCode !== expectedReason || (value.available && !value.enabled) || (value.active && !value.available)) {
      context.addIssue({ code: 'custom', message: 'MCP search state fields are inconsistent' })
    }
  })

export const PublicMcpCatalogV2 = z
  .object({
    status: z.enum(['available', 'unavailable', 'disabled', 'cached', 'lazy']),
    reasonCode: z.enum(['available', 'disabled_by_config', 'schema_hint_only', 'not_live', 'unavailable']),
    toolCount: Count,
    advertisedToolCount: Count,
    promptCount: Count,
    resourceCount: Count,
    catalogFingerprint: Sha256.optional(),
    catalogDrift: z.boolean()
  })
  .strict()
  .superRefine((value, context) => {
    const expectedReason = {
      available: 'available',
      unavailable: 'unavailable',
      disabled: 'disabled_by_config',
      cached: 'schema_hint_only',
      lazy: 'not_live'
    }[value.status]
    if (value.reasonCode !== expectedReason) {
      context.addIssue({ code: 'custom', message: 'MCP catalog status and reason are inconsistent' })
    }
  })

const PublicMcpCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  configuredServers: Count,
  connectedServers: Count,
  toolCount: Count,
  promptCount: Count,
  resourceCount: Count,
  catalog: PublicMcpCatalogV2,
  search: PublicMcpSearchV2
}).strict()

const PublicWebCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  fetch: PublicRuntimeCapabilityStateV2,
  search: PublicRuntimeCapabilityStateV2,
  provider: PublicIdentifier,
  fetchEnabled: z.boolean(),
  searchEnabled: z.boolean()
}).strict()

const PublicSkillsCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  configuredRoots: Count,
  discoveredSkills: Count
}).strict()

const PublicSubagentCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  maxParallel: Count,
  maxChildRuns: Count,
  defaultToolPolicy: SubagentToolPolicy,
  profileCount: Count,
  internalLineageAvailable: z.boolean(),
  profilesAvailable: z.boolean(),
  durableChildRunStore: z.boolean(),
  parallelExecutionAvailable: z.boolean(),
  taskToolAvailable: z.boolean(),
  parallelTasksToolAvailable: z.boolean(),
  backgroundTaskJobsAvailable: z.boolean(),
  backgroundShellAvailable: z.boolean(),
  backgroundSubagentJobsAvailable: z.boolean(),
  modelJobToolsAvailable: z.boolean(),
  taskJobThreadScopeSupported: z.boolean()
}).strict()

const PublicAttachmentsCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  maxImageBytes: Count,
  maxImageDimension: Count,
  allowedMimeTypes: z.array(z.string().min(3).max(128)),
  allowedDocumentMimeTypes: z.array(z.string().min(3).max(128)),
  maxDocumentBytes: Count,
  maxDocumentTextChars: Count,
  textFallbackMaxBase64Bytes: Count,
  textFallbackMaxImageDimension: Count,
  textFallbackPreferredMimeType: z.string().min(3).max(128)
}).strict()

const PublicMemoryCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  mode: z.literal('manual'),
  storeOnly: z.boolean(),
  modelInjection: z.boolean(),
  automaticCapture: z.boolean(),
  scopes: z.array(z.enum(['user', 'workspace', 'project'])),
  maxInjectedRecords: Count
}).strict()

const PublicGeneratedCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  model: PublicIdentifier.optional()
}).strict()

const PublicComputerUseCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  mode: ComputerUseMode,
  backendId: PublicIdentifier.optional(),
  preferredBackendId: PublicIdentifier.optional()
}).strict()

const PublicVisionBridgeCapabilityV2 = PublicRuntimeCapabilityStateV2.safeExtend({
  mode: VisionBridgeMode,
  model: PublicIdentifier.optional(),
  providerId: PublicIdentifier.optional(),
  maxScreenshotsPerTurn: Count,
  maxImagesPerTurn: Count,
  maxImageBytes: Count,
  maxImageDimension: Count,
  semanticProbeStatus: ModelCapabilityProbeStatus
}).strict()

export const PublicRuntimeCapabilitiesV2 = z
  .object({
    contractVersion: z.literal(1),
    model: PublicModelCapabilityV2,
    cli: PublicCliCapabilitiesV2,
    mcp: PublicMcpCapabilityV2,
    web: PublicWebCapabilityV2,
    skills: PublicSkillsCapabilityV2,
    subagents: PublicSubagentCapabilityV2,
    attachments: PublicAttachmentsCapabilityV2,
    memory: PublicMemoryCapabilityV2,
    imageGen: PublicGeneratedCapabilityV2,
    speechGen: PublicGeneratedCapabilityV2,
    musicGen: PublicGeneratedCapabilityV2,
    videoGen: PublicGeneratedCapabilityV2,
    computerUse: PublicComputerUseCapabilityV2,
    visionBridge: PublicVisionBridgeCapabilityV2
  })
  .strict()

export const RuntimeInfoResponse = z
  .object({
    schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION),
    status: z.enum(['ready', 'degraded']),
    listenerScope: z.enum(['loopback', 'non_loopback']),
    port: z.number().int().min(0).max(65_535),
    startedAt: z.string().datetime({ offset: true }),
    insecure: z.boolean(),
    storage: z
      .object({
        configured: z.boolean(),
        available: z.boolean()
      })
      .strict(),
    executionPolicy: z
      .object({
        approvalPolicy: z.enum(['always', 'on-request', 'untrusted', 'never', 'auto', 'suggest', 'unknown']),
        sandboxMode: z.enum(['read-only', 'workspace-write', 'danger-full-access', 'external-sandbox', 'unknown'])
      })
      .strict(),
    provider: z
      .object({
        id: PublicIdentifier,
        model: PublicIdentifier,
        family: PublicIdentifier,
        endpointFormat: z.enum([...MODEL_ENDPOINT_FORMATS, 'unknown']),
        available: z.boolean(),
        apiKeyConfigured: z.boolean(),
        baseUrlConfigured: z.boolean(),
        cacheTelemetrySupported: z.boolean(),
        supportsImageInput: z.boolean(),
        contextWindowTokens: Count.optional(),
        reasoningEffort: z.enum(['auto', 'off', 'low', 'medium', 'high', 'max']).optional()
      })
      .strict()
      .superRefine((value, context) => {
        const expectedAvailable = value.apiKeyConfigured && value.baseUrlConfigured && value.model !== 'unknown'
        if (value.available !== expectedAvailable) {
          context.addIssue({ code: 'custom', message: 'provider availability fields are inconsistent' })
        }
      }),
    networkProxy: z
      .object({
        mode: z.enum(['auto', 'env', 'off', 'custom', 'unknown']),
        configured: z.boolean(),
        source: z.enum(['environment', 'settings.provider.proxy', 'unknown']),
        valid: z.boolean(),
        credentialsMasked: z.boolean()
      })
      .strict(),
    capabilities: PublicRuntimeCapabilitiesV2
  })
  .strict()

export type RuntimeInfoResponse = z.infer<typeof RuntimeInfoResponse>
