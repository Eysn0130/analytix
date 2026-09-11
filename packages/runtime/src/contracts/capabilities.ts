import { z } from 'zod'
import { MODEL_ENDPOINT_FORMATS } from './model-endpoint-format.js'
import { providerRegistryOAuthBindingSchemaV1 } from './provider-registry.js'

export const RUNTIME_CAPABILITY_CONTRACT_VERSION = 1

export const RuntimeCapabilityStatus = z.enum(['available', 'disabled', 'unavailable'])
export type RuntimeCapabilityStatus = z.infer<typeof RuntimeCapabilityStatus>

export const RuntimeCapabilityState = z
  .object({
    status: RuntimeCapabilityStatus,
    enabled: z.boolean(),
    available: z.boolean(),
    reason: z.string().optional()
  })
  .strict()
export type RuntimeCapabilityState = z.infer<typeof RuntimeCapabilityState>

export const ModelInputModality = z.enum(['text', 'image'])
export type ModelInputModality = z.infer<typeof ModelInputModality>

export const ModelMessagePartSupport = z.enum(['text', 'image_url', 'input_image'])
export type ModelMessagePartSupport = z.infer<typeof ModelMessagePartSupport>

export const ModelReasoningEffort = z.enum(['auto', 'off', 'low', 'medium', 'high', 'max'])
export type ModelReasoningEffort = z.infer<typeof ModelReasoningEffort>
export const SubagentReasoningEffort = z.enum(['off', 'low', 'medium', 'high', 'max'])
export type SubagentReasoningEffort = z.infer<typeof SubagentReasoningEffort>

export const ModelReasoningRequestProtocol = z.enum([
  'none',
  'deepseek-chat-completions',
  'glm-chat-completions',
  'mimo-chat-completions',
  'openai-responses',
  'anthropic-thinking'
])
export type ModelReasoningRequestProtocol = z.infer<typeof ModelReasoningRequestProtocol>

export const ModelReasoningCapabilityMetadata = z
  .object({
    supportedEfforts: z.array(ModelReasoningEffort).min(1),
    defaultEffort: ModelReasoningEffort,
    requestProtocol: ModelReasoningRequestProtocol
  })
  .strict()
export type ModelReasoningCapabilityMetadata = z.infer<typeof ModelReasoningCapabilityMetadata>

export const ModelCapabilityMetadata = z
  .object({
    id: z.string().min(1),
    providerId: z.string().min(1).optional(),
    family: z.string().min(1).optional(),
    inputModalities: z.array(ModelInputModality).min(1),
    outputModalities: z.array(ModelInputModality).min(1),
    supportsToolCalling: z.boolean(),
    supportsImageInput: z.boolean().optional(),
    contextWindowTokens: z.number().int().positive().optional(),
    messageParts: z.array(ModelMessagePartSupport).min(1),
    reasoning: ModelReasoningCapabilityMetadata.optional(),
    // Per-model wire-format override. Lets one provider route some models to
    // chat completions and others to Anthropic Messages / OpenAI Responses
    // (e.g. OpenCode Go). Absent means "inherit the provider/runtime format".
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional()
  })
  .strict()
export type ModelCapabilityMetadata = z.infer<typeof ModelCapabilityMetadata>

const CapabilityToggleConfig = z
  .object({
    enabled: z.boolean().default(false)
  })
  .strict()

const StringRecord = z.record(z.string(), z.string())

export const McpTransportKind = z.enum(['stdio', 'streamable-http', 'sse'])
export type McpTransportKind = z.infer<typeof McpTransportKind>

export const McpTrustScope = z.enum(['user', 'workspace'])
export type McpTrustScope = z.infer<typeof McpTrustScope>

export const McpToolDiscoveryMode = z.enum(['direct', 'search', 'auto'])
export type McpToolDiscoveryMode = z.infer<typeof McpToolDiscoveryMode>

export const McpAccountCredentialScope = z
  .object({
    owner: z.enum(['mcp', 'extension']).default('mcp'),
    provider: z.string().trim().min(1).max(128),
    accountId: z.string().trim().min(1).max(256),
    purpose: z.enum(['mcp-oauth-access-token', 'extension-provider-account-token']),
    bindingFingerprint: z.string().regex(/^[a-f0-9]{64}$/).optional()
  })
  .strict()
  .superRefine((scope, ctx) => {
    const coherent = scope.owner === 'mcp'
      ? scope.purpose === 'mcp-oauth-access-token'
      : scope.purpose === 'extension-provider-account-token'
    if (!coherent) {
      ctx.addIssue({ code: 'custom', path: ['purpose'], message: 'MCP account credential purpose is not bound to its owner' })
    }
  })
export type McpAccountCredentialScope = z.infer<typeof McpAccountCredentialScope>

export const McpSearchConfig = z
  .object({
    enabled: z.boolean().default(false),
    mode: McpToolDiscoveryMode.default('auto'),
    autoThresholdToolCount: z.number().int().positive().default(24),
    topKDefault: z.number().int().positive().default(5),
    topKMax: z.number().int().positive().default(10),
    minScore: z.number().nonnegative().default(0.15),
    bm25: z
      .object({
        k1: z.number().positive().default(1.2),
        b: z.number().min(0).max(1).default(0.75)
      })
      .strict()
      .default(() => ({ k1: 1.2, b: 0.75 }))
  })
  .strict()
  .superRefine((search, ctx) => {
    if (search.topKDefault > search.topKMax) {
      ctx.addIssue({
        code: 'custom',
        path: ['topKDefault'],
        message: 'topKDefault must be less than or equal to topKMax'
      })
    }
  })
export type McpSearchConfig = z.infer<typeof McpSearchConfig>

export const McpServerConfig = z
  .object({
    enabled: z.boolean().default(true),
    transport: McpTransportKind,
    command: z.string().min(1).optional(),
    args: z.array(z.string()).default([]),
    cwd: z.string().min(1).optional(),
    url: z.string().min(1).optional(),
    headers: StringRecord.default({}),
    accountCredential: McpAccountCredentialScope.optional(),
    oauthBinding: providerRegistryOAuthBindingSchemaV1.optional(),
    env: StringRecord.default({}),
    expectedServerName: z.string().min(1).optional(),
    expectedServerVersion: z.string().min(1).optional(),
    identitySource: z.literal('installed-plugin-manifest').optional(),
    manifestSha256: z.string().regex(/^[a-f0-9]{64}$/i).optional(),
    entrypointPath: z.string().min(1).optional(),
    entrypointSha256: z.string().regex(/^[a-f0-9]{64}$/i).optional(),
    pluginRootPath: z.string().min(1).optional(),
    sourceTreeSha256: z.string().regex(/^[a-f0-9]{64}$/i).optional(),
    trustScope: McpTrustScope.default('workspace'),
    trustedWorkspaceRoots: z.array(z.string().min(1)).default([]),
    lowPriority: z.boolean().default(false),
    backgroundStart: z.boolean().default(false),
    timeoutMs: z.number().int().positive().max(3_600_000).default(30_000)
  })
  .strict()
  .superRefine((server, ctx) => {
    if (server.transport === 'stdio' && !server.command) {
      ctx.addIssue({
        code: 'custom',
        path: ['command'],
        message: 'stdio MCP servers require command'
      })
    }
    if ((server.transport === 'streamable-http' || server.transport === 'sse') && !server.url) {
      ctx.addIssue({
        code: 'custom',
        path: ['url'],
        message: `${server.transport} MCP servers require url`
      })
    }
    if (server.url) {
      try {
        const parsed = new URL(server.url)
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
          ctx.addIssue({
            code: 'custom',
            path: ['url'],
            message: 'MCP server url must use http or https'
          })
        }
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['url'],
          message: 'MCP server url must be a valid URL'
        })
      }
    }
    if (server.accountCredential) {
      if (server.transport === 'stdio') {
        ctx.addIssue({
          code: 'custom',
          path: ['accountCredential'],
          message: 'MCP account credentials require an HTTP transport'
        })
      }
      if (Object.keys(server.headers).some((name) => name.trim().toLowerCase() === 'authorization')) {
        ctx.addIssue({
          code: 'custom',
          path: ['headers'],
          message: 'MCP account credentials cannot be combined with an Authorization header'
        })
      }
    }
    if (server.oauthBinding !== undefined && server.accountCredential === undefined) {
      ctx.addIssue({
        code: 'custom',
        path: ['oauthBinding'],
        message: 'MCP OAuth binding requires one exact account credential scope'
      })
    }
    if (server.trustScope === 'workspace' && server.trustedWorkspaceRoots.length === 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['trustedWorkspaceRoots'],
        message: 'workspace-scoped MCP servers require at least one trusted workspace root'
      })
    }
  })
export type McpServerConfig = z.infer<typeof McpServerConfig>

export const McpServerIdV1 = z.string()
  .regex(/^[a-z0-9][a-z0-9_-]{0,127}$/)
  .refine((value) => !value.includes('__'), 'MCP server id contains the reserved namespace separator')
export type McpServerIdV1 = z.infer<typeof McpServerIdV1>

export const McpCapabilityConfig = CapabilityToggleConfig.extend({
  servers: z.record(McpServerIdV1, McpServerConfig).default({}),
  search: McpSearchConfig.default(() => McpSearchConfig.parse({}))
}).strict()
export type McpCapabilityConfig = z.infer<typeof McpCapabilityConfig>

export const WebCapabilityConfig = CapabilityToggleConfig.extend({
  fetchEnabled: z.boolean().default(false),
  searchEnabled: z.boolean().default(false),
  provider: z.string().min(1).optional(),
  allowDomains: z.array(z.string().min(1)).default([]),
  denyDomains: z.array(z.string().min(1)).default([]),
  /** Upper bound for web_fetch body bytes; fetched pages truncate here. */
  maxFetchBytes: z.number().int().positive().default(1_000_000)
}).strict()
export type WebCapabilityConfig = z.infer<typeof WebCapabilityConfig>

export const SkillsCapabilityConfig = CapabilityToggleConfig.extend({
  roots: z.array(z.string().min(1)).default([]),
  legacySkillMd: z.boolean().default(true)
}).strict()
export type SkillsCapabilityConfig = z.infer<typeof SkillsCapabilityConfig>

export const SubagentToolPolicy = z.enum(['readOnly', 'inherit'])
export type SubagentToolPolicy = z.infer<typeof SubagentToolPolicy>

/**
 * Tools a `readOnly` subagent may call. The list is enforced twice: the
 * child loop advertises only these names (schema filter) and the
 * capability registry re-checks them at execute time (backstop). Keep it
 * to side-effect-free investigation tools — no bash/edit/write, and no
 * nested `delegate_task`.
 */
export const SUBAGENT_READ_ONLY_TOOL_NAMES = ['read', 'grep', 'find', 'ls'] as const

export const SubagentProfileConfig = z
  .object({
    /** Overrides the child provider for this role (falls back to parent provider). */
    providerId: z.string().min(1).optional(),
    /** Overrides the child model for this role (falls back to the server default). */
    model: z.string().min(1).optional(),
    /** Optional model variant carried into child execution lineage. */
    variant: z.string().min(1).optional(),
    /** Optional endpoint format override carried into child execution lineage. */
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional(),
    /** Overrides the child reasoning effort for this role. */
    effort: SubagentReasoningEffort.optional(),
    /** Short instruction prepended to the delegated task prompt. */
    promptPreamble: z.string().min(1).optional(),
    /** Whether the child is restricted to read-only tools or inherits the full set. */
    toolPolicy: SubagentToolPolicy.default('readOnly'),
    /** Optional explicit tool scope for this profile. Empty means policy default. */
    tools: z.array(z.string().min(1)).default([])
  })
  .strict()
export type SubagentProfileConfig = z.infer<typeof SubagentProfileConfig>

export const SubagentsCapabilityConfig = CapabilityToggleConfig.extend({
  /** Max children running at once; extra spawns queue instead of erroring. */
  maxParallel: z.number().int().nonnegative().default(0),
  /** Hard cap on total children per parent thread. */
  maxChildRuns: z.number().int().nonnegative().default(0),
  /** Tool policy applied to children that do not resolve a profile. */
  defaultToolPolicy: SubagentToolPolicy.default('readOnly'),
  /** Profile chosen when `delegate_task` omits an explicit profile. */
  defaultProfile: z.string().min(1).optional(),
  /** Named subagent roles (e.g. researcher/reviewer/verifier). */
  profiles: z.record(z.string().min(1), SubagentProfileConfig).default({}),
  // Accept the removed legacy field so old configs keep loading, but ignore it.
  defaultStepLimit: z.number().int().positive().optional()
})
  .strict()
  .superRefine((config, ctx) => {
    if (config.defaultProfile && !(config.defaultProfile in config.profiles)) {
      ctx.addIssue({
        code: 'custom',
        path: ['defaultProfile'],
        message: `defaultProfile "${config.defaultProfile}" is not defined in profiles`
      })
    }
  })
  .transform(({ defaultStepLimit: _legacyDefaultStepLimit, ...config }) => config)
export type SubagentsCapabilityConfig = z.output<typeof SubagentsCapabilityConfig>

export const RuntimeSubagentProfileState = z
  .object({
    name: z.string().min(1),
    providerId: z.string().optional(),
    model: z.string().optional(),
    variant: z.string().optional(),
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional(),
    effort: SubagentReasoningEffort.optional(),
    toolPolicy: SubagentToolPolicy,
    tools: z.array(z.string().min(1)).optional(),
    /**
     * Runtime diagnostics expose prompt preamble presence as a boolean so the
     * full child instruction does not leak through `/v1/runtime/info`.
     */
	    promptPreamble: z.boolean().optional(),
	    source: z.string().min(1).optional(),
	    hidden: z.boolean().optional(),
	    mode: z.string().min(1).optional()
	  })
  .strict()
export type RuntimeSubagentProfileState = z.infer<typeof RuntimeSubagentProfileState>

export const DEFAULT_ATTACHMENT_TEXT_FALLBACK_MAX_BASE64_BYTES = 512 * 1024
export const DEFAULT_ATTACHMENT_TEXT_FALLBACK_MAX_IMAGE_DIMENSION = 1280
export const DEFAULT_ATTACHMENT_TEXT_FALLBACK_PREFERRED_MIME_TYPE = 'image/webp'
export const DEFAULT_ATTACHMENT_DOCUMENT_MIME_TYPES = [
  'application/pdf',
  'text/plain',
  'text/markdown',
  'text/csv',
  'application/json'
]
export const DEFAULT_ATTACHMENT_MAX_DOCUMENT_BYTES = 10 * 1024 * 1024
export const DEFAULT_ATTACHMENT_MAX_DOCUMENT_TEXT_CHARS = 200_000

export const AttachmentsCapabilityConfig = CapabilityToggleConfig.extend({
  maxImageBytes: z.number().int().positive().default(5 * 1024 * 1024),
  maxImageDimension: z.number().int().positive().default(4096),
  allowedMimeTypes: z.array(z.string().min(1)).default(['image/png', 'image/jpeg', 'image/webp']),
  allowedDocumentMimeTypes: z.array(z.string().min(1)).default(DEFAULT_ATTACHMENT_DOCUMENT_MIME_TYPES),
  maxDocumentBytes: z.number().int().positive().default(DEFAULT_ATTACHMENT_MAX_DOCUMENT_BYTES),
  maxDocumentTextChars: z.number().int().positive().default(DEFAULT_ATTACHMENT_MAX_DOCUMENT_TEXT_CHARS),
  textFallbackMaxBase64Bytes: z.number().int().positive().default(DEFAULT_ATTACHMENT_TEXT_FALLBACK_MAX_BASE64_BYTES),
  textFallbackMaxImageDimension: z.number().int().positive().default(DEFAULT_ATTACHMENT_TEXT_FALLBACK_MAX_IMAGE_DIMENSION),
  textFallbackPreferredMimeType: z.string().min(1).default(DEFAULT_ATTACHMENT_TEXT_FALLBACK_PREFERRED_MIME_TYPE)
}).strict()
export type AttachmentsCapabilityConfig = z.infer<typeof AttachmentsCapabilityConfig>

export const MemoryCapabilityConfig = CapabilityToggleConfig.extend({
  scopes: z.array(z.enum(['user', 'workspace', 'project'])).default(['user', 'workspace', 'project']),
  maxInjectedRecords: z.number().int().positive().default(8)
}).strict()
export type MemoryCapabilityConfig = z.infer<typeof MemoryCapabilityConfig>

export const ImageGenerationProtocol = z.enum(['openai-images', 'minimax-image'])
export type ImageGenerationProtocol = z.infer<typeof ImageGenerationProtocol>

export const ImageGenCapabilityConfig = CapabilityToggleConfig.extend({
  protocol: ImageGenerationProtocol.default('openai-images'),
  baseUrl: z.string().min(1).optional(),
  apiKey: z.string().min(1).optional(),
  model: z.string().min(1).optional(),
  defaultSize: z.string().min(1).optional(),
  timeoutMs: z.number().int().positive().default(180_000),
  maxReferenceImages: z.number().int().positive().max(8).default(4)
}).strict()
export type ImageGenCapabilityConfig = z.infer<typeof ImageGenCapabilityConfig>

export const TextToSpeechProtocol = z.enum(['openai-speech', 'minimax-t2a', 'mimo-tts'])
export type TextToSpeechProtocol = z.infer<typeof TextToSpeechProtocol>

export const SpeechGenCapabilityConfig = CapabilityToggleConfig.extend({
  protocol: TextToSpeechProtocol.default('openai-speech'),
  baseUrl: z.string().min(1).optional(),
  apiKey: z.string().min(1).optional(),
  model: z.string().min(1).optional(),
  voice: z.string().min(1).optional(),
  format: z.string().min(1).default('mp3'),
  timeoutMs: z.number().int().positive().default(120_000)
}).strict()
export type SpeechGenCapabilityConfig = z.infer<typeof SpeechGenCapabilityConfig>

export const MusicGenerationProtocol = z.enum(['minimax-music'])
export type MusicGenerationProtocol = z.infer<typeof MusicGenerationProtocol>

export const MusicGenCapabilityConfig = CapabilityToggleConfig.extend({
  protocol: MusicGenerationProtocol.default('minimax-music'),
  baseUrl: z.string().min(1).optional(),
  apiKey: z.string().min(1).optional(),
  model: z.string().min(1).optional(),
  format: z.string().min(1).default('mp3'),
  timeoutMs: z.number().int().positive().default(300_000)
}).strict()
export type MusicGenCapabilityConfig = z.infer<typeof MusicGenCapabilityConfig>

export const VideoGenerationProtocol = z.enum(['minimax-video'])
export type VideoGenerationProtocol = z.infer<typeof VideoGenerationProtocol>

export const VideoGenCapabilityConfig = CapabilityToggleConfig.extend({
  protocol: VideoGenerationProtocol.default('minimax-video'),
  baseUrl: z.string().min(1).optional(),
  apiKey: z.string().min(1).optional(),
  model: z.string().min(1).optional(),
  defaultDuration: z.number().int().positive().default(6),
  defaultResolution: z.string().min(1).default('1080P'),
  timeoutMs: z.number().int().positive().default(900_000),
  pollIntervalMs: z.number().int().positive().default(10_000)
}).strict()
export type VideoGenCapabilityConfig = z.infer<typeof VideoGenCapabilityConfig>

/**
 * Host computer-use mode. `auto` advertises the tool only when the active
 * model accepts image input, `always` advertises whenever the native backend
 * is available, and `off` never advertises it.
 */
export const ComputerUseMode = z.enum(['auto', 'always', 'off'])
export type ComputerUseMode = z.infer<typeof ComputerUseMode>

export const ComputerUseCapabilityConfig = CapabilityToggleConfig.extend({
  mode: ComputerUseMode.default('auto'),
  maxImageDimension: z.number().int().positive().default(1280),
  maxActionsPerTurn: z.number().int().positive().default(40),
  allowWhenLocked: z.boolean().default(true)
}).strict()
export type ComputerUseCapabilityConfig = z.infer<typeof ComputerUseCapabilityConfig>

export const VisionBridgeMode = z.enum(['auto', 'always', 'off'])
export type VisionBridgeMode = z.infer<typeof VisionBridgeMode>

export const VisionBridgeInjectPolicy = z.enum(['observation_text'])
export type VisionBridgeInjectPolicy = z.infer<typeof VisionBridgeInjectPolicy>
export const ModelCapabilityProbeStatus = z.enum([
  'unknown',
  'supported',
  'unsupported',
  'semantic_failed',
  'auth_failed',
  'http_failed',
  'timeout',
  'failed',
  'stale'
])
export type ModelCapabilityProbeStatus = z.infer<typeof ModelCapabilityProbeStatus>

export const VisionBridgeCapabilityConfig = CapabilityToggleConfig.extend({
  mode: VisionBridgeMode.default('auto'),
  providerId: z.string().min(1).optional(),
  baseUrl: z.string().min(1).optional(),
  apiKey: z.string().optional(),
  endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).default('chat_completions'),
  model: z.string().min(1).optional(),
  maxImageDimension: z.number().int().positive().default(1280),
  maxImageBytes: z.number().int().positive().default(1_500_000),
  maxScreenshotsPerTurn: z.number().int().positive().default(4),
  observationCacheTtlMs: z.number().int().positive().default(120_000),
  injectPolicy: VisionBridgeInjectPolicy.default('observation_text'),
  fallbackWhenPrimaryImageUnsupported: z.boolean().default(true),
  semanticProbeStatus: ModelCapabilityProbeStatus.default('unknown')
}).strict()
export type VisionBridgeCapabilityConfig = z.infer<typeof VisionBridgeCapabilityConfig>

export const AnalytixCapabilitiesConfig = z
  .object({
    mcp: McpCapabilityConfig.default(() => McpCapabilityConfig.parse({})),
    web: WebCapabilityConfig.default(() => WebCapabilityConfig.parse({})),
    skills: SkillsCapabilityConfig.default(() => SkillsCapabilityConfig.parse({})),
    subagents: SubagentsCapabilityConfig.default(() => SubagentsCapabilityConfig.parse({})),
    attachments: AttachmentsCapabilityConfig.default(() => AttachmentsCapabilityConfig.parse({})),
    memory: MemoryCapabilityConfig.default(() => MemoryCapabilityConfig.parse({})),
    imageGen: ImageGenCapabilityConfig.default(() => ImageGenCapabilityConfig.parse({})),
    speechGen: SpeechGenCapabilityConfig.default(() => SpeechGenCapabilityConfig.parse({})),
    musicGen: MusicGenCapabilityConfig.default(() => MusicGenCapabilityConfig.parse({})),
    videoGen: VideoGenCapabilityConfig.default(() => VideoGenCapabilityConfig.parse({})),
    computerUse: ComputerUseCapabilityConfig.default(() => ComputerUseCapabilityConfig.parse({})),
    visionBridge: VisionBridgeCapabilityConfig.default(() => VisionBridgeCapabilityConfig.parse({}))
  })
  .strict()
export type AnalytixCapabilitiesConfig = z.infer<typeof AnalytixCapabilitiesConfig>

export const DEFAULT_ANALYTIX_CAPABILITIES_CONFIG: AnalytixCapabilitiesConfig = AnalytixCapabilitiesConfig.parse({})

export const RuntimeAbsorptionClass = z.enum([
  'replace',
  'code-port-and-adapt',
  'contract-reimplement',
  'reject',
  'defer'
])
export type RuntimeAbsorptionClass = z.infer<typeof RuntimeAbsorptionClass>

export const RuntimeAbsorptionStatus = z.enum(['green', 'red', 'rejected', 'deferred'])
export type RuntimeAbsorptionStatus = z.infer<typeof RuntimeAbsorptionStatus>

export const RuntimeMachineCheck = z
  .object({
    id: z.string().min(1),
    kind: z.string().min(1),
    status: z.enum([
      'passed',
      'red',
      'failed',
      'missing',
      'skipped',
      'expected-blocked',
      'strict-blocked',
      'post-cutover-live-validation-pending'
    ]),
    evidence: z.string().min(1).optional(),
    expectedBlocked: z.boolean().optional(),
    acceptedAsCodePhasePass: z.boolean().optional(),
    defaultBackendReady: z.boolean().optional(),
    blockerIds: z.array(z.string().min(1)).optional()
  })
  .strict()
export type RuntimeMachineCheck = z.infer<typeof RuntimeMachineCheck>

export const ReasonixCapabilityAuditRow = z
  .object({
    id: z.string().min(1),
    capability: z.string().min(1),
    reasonixSources: z.array(z.string().min(1)).min(1),
    analytixLanding: z.string().min(1),
    absorptionClass: RuntimeAbsorptionClass,
    status: RuntimeAbsorptionStatus,
    machineChecks: z.array(RuntimeMachineCheck).min(1),
    analytixEvidence: z.array(z.string().min(1)).default([]),
    blockers: z.array(z.string().min(1)).optional(),
    replacesAnalytixWeakness: z.string().min(1).optional(),
    deleteCandidates: z.array(z.string().min(1)).optional(),
    usesReasonixPublicProtocol: z.boolean(),
    usesReasonixConfigRoot: z.boolean(),
    changesRendererContract: z.boolean(),
    changesProductIdentity: z.boolean(),
    requiresKunProductEntryDrift: z.boolean()
  })
  .strict()
export type ReasonixCapabilityAuditRow = z.infer<typeof ReasonixCapabilityAuditRow>

export const ReasonixCapabilityAuditMatrix = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('reasonix-capability-audit'),
    sourcePath: z.literal('/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    sourceCommit: z.string().min(40),
    runtimeContract: z.string().min(1),
    readyForG6: z.boolean(),
    capabilityMatrixGreen: z.boolean(),
    defaultBackendReady: z.boolean(),
    defaultBackendReadinessGate: z.string().min(1),
    rows: z.array(ReasonixCapabilityAuditRow).min(1),
    greenCount: z.number().int().nonnegative(),
    redCount: z.number().int().nonnegative(),
    rejectedCount: z.number().int().nonnegative(),
    deferredCount: z.number().int().nonnegative(),
    forbiddenPublicProtocolRowCount: z.number().int().nonnegative(),
    rendererContractChangedRowCount: z.number().int().nonnegative(),
    productIdentityChangedRowCount: z.number().int().nonnegative(),
    kunProductEntryDriftRowCount: z.number().int().nonnegative(),
    postG6DeleteCandidates: z.array(z.string().min(1)),
    g6Blockers: z.array(z.string().min(1)),
    readinessSemantics: z.array(z.string().min(1)),
    notes: z.array(z.string().min(1))
  })
  .strict()
export type ReasonixCapabilityAuditMatrix = z.infer<typeof ReasonixCapabilityAuditMatrix>

export const KunAnalytixBaselineRow = z
  .object({
    id: z.string().min(1),
    capability: z.string().min(1),
    kunAnalytixSources: z.array(z.string().min(1)).min(1),
    entryPoint: z.string().min(1),
    triggerPath: z.string().min(1),
    runtimeContract: z.string().min(1),
    settingsSchema: z.string().min(1),
    uiSurface: z.string().min(1),
    protectionTests: z.array(z.string().min(1)).min(1),
    immutableProductBaseline: z.boolean(),
    reasonixEnhancementAllowed: z.string().min(1),
    status: z.literal('protected')
  })
  .strict()
export type KunAnalytixBaselineRow = z.infer<typeof KunAnalytixBaselineRow>

export const KunAnalytixBaselineGuard = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('kun-analytix-baseline-guard'),
    kunSourceSnapshots: z.array(z.string().min(1)).min(1),
    analytixSourceRoot: z.literal('/Users/sun/Projects/analytix'),
    fullFunctionBaseline: z.boolean(),
    rows: z.array(KunAnalytixBaselineRow).min(1),
    protectedBaselineCount: z.number().int().nonnegative(),
    internalEnhancementCount: z.number().int().nonnegative(),
    forbiddenTopLevelEntrypoints: z.array(z.string().min(1)),
    forbiddenEntrypointExposed: z.boolean(),
    deprecatedBridgeAliasAllowed: z.boolean(),
    legacySettingsWriteAllowed: z.boolean(),
    deepseekOnlyRuntimeAllowed: z.boolean(),
    readyForReasonixAbsorption: z.boolean(),
    notes: z.array(z.string().min(1))
  })
  .strict()
export type KunAnalytixBaselineGuard = z.infer<typeof KunAnalytixBaselineGuard>

export const ReasonixAbsorptionRow = z
  .object({
    id: z.string().min(1),
    capability: z.string().min(1),
    reasonixSources: z.array(z.string().min(1)).min(1),
    absorptionClass: RuntimeAbsorptionClass,
    status: RuntimeAbsorptionStatus,
    analytixLanding: z.string().min(1),
    providerSpecific: z.boolean(),
    doesNotNarrowProviders: z.boolean(),
    kunBaselineProtection: z.array(z.string().min(1)).min(1),
    machineChecks: z.array(RuntimeMachineCheck).min(1),
    forbiddenProductSurface: z.boolean(),
    rejectedBecauseBaseline: z.string().min(1).optional(),
    blockers: z.array(z.string().min(1)).optional()
  })
  .strict()
export type ReasonixAbsorptionRow = z.infer<typeof ReasonixAbsorptionRow>

export const ReasonixAbsorptionMatrix = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('reasonix-absorption-matrix'),
    reasonixSourcePath: z.literal('/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    reasonixSourceCommit: z.string().min(40),
    rows: z.array(ReasonixAbsorptionRow).min(1),
    greenCount: z.number().int().nonnegative(),
    redCount: z.number().int().nonnegative(),
    deferredCount: z.number().int().nonnegative(),
    rejectedCount: z.number().int().nonnegative(),
    forbiddenProductSurfaceCount: z.number().int().nonnegative(),
    providerFamilies: z.array(z.string().min(1)).min(1),
    multiModelNonRegressionGreen: z.boolean(),
    deepSeekEnhancementScopedOnly: z.boolean(),
    readyForG6: z.boolean(),
    absorptionMatrixGreen: z.boolean(),
    defaultBackendReady: z.boolean(),
    defaultBackendReadinessGate: z.string().min(1),
    g6Blockers: z.array(z.string().min(1)),
    readinessSemantics: z.array(z.string().min(1)),
    notes: z.array(z.string().min(1))
  })
  .strict()
export type ReasonixAbsorptionMatrix = z.infer<typeof ReasonixAbsorptionMatrix>

export const RuntimeReadinessCheck = z
  .object({
    status: z.enum(['missing', 'skipped', 'failed', 'passed']),
    required: z.boolean(),
    evidence: z.string().min(1).optional(),
    message: z.string().min(1).optional()
  })
  .strict()
export type RuntimeReadinessCheck = z.infer<typeof RuntimeReadinessCheck>

export const RuntimeDefaultBackendReadiness = z
  .object({
    schemaVersion: z.literal(1),
    ready: z.boolean(),
    explicitReadyGate: z.boolean(),
    durableRestartEvidence: RuntimeReadinessCheck,
    providerMatrix: RuntimeReadinessCheck,
    mcpMatrix: RuntimeReadinessCheck,
    packagedQa: RuntimeReadinessCheck,
    operatorGate: RuntimeReadinessCheck,
    missingRequiredChecks: z.array(z.string().min(1)),
    defaultGoBackendEnabled: z.literal(true),
    rendererVisibleGoSwitcher: z.literal(false),
    notes: z.array(z.string().min(1)).optional()
  })
  .strict()
export type RuntimeDefaultBackendReadiness = z.infer<typeof RuntimeDefaultBackendReadiness>

export const RuntimeReadinessSemantics = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('runtime-readiness-semantics'),
    reasonixCapabilityMatrixGreen: z.boolean(),
    reasonixAbsorptionMatrixGreen: z.boolean(),
    kunAnalytixBaselineGuardGreen: z.boolean(),
    capabilityMatrixGreen: z.boolean(),
    absorptionMatrixGreen: z.boolean(),
    baselineGuardGreen: z.boolean(),
    defaultBackendReady: z.boolean(),
    defaultBackendGateChangeId: z.literal('runtime-readiness'),
    requiredDefaultBackendGates: z.array(z.string().min(1)).min(1),
    skippedCountsAsPassed: z.literal(false),
    fixtureMatrixCountsAsCredentialedPass: z.literal(false),
    matrixGreenEnablesDefaultBackend: z.literal(false),
    typeScriptRuntimeDefault: z.literal(false),
    goRuntimeCandidateInternalOnly: z.literal(false),
    notes: z.array(z.string().min(1))
  })
  .strict()
export type RuntimeReadinessSemantics = z.infer<typeof RuntimeReadinessSemantics>

export const ReasonixIntegrationEvidenceState = z
  .object({
    codeLevelAbsorbed: z.literal(true),
    localContractGreen: z.literal(true),
    requiresCredentialedG6: z.literal(false),
    credentialedEvidence: z.string().min(1),
    defaultCutoverCandidate: z.boolean(),
    typeScriptFallbackRetain: z.literal(false),
    blockers: z.array(z.string().min(1))
  })
  .strict()
export type ReasonixIntegrationEvidenceState = z.infer<typeof ReasonixIntegrationEvidenceState>

export const ReasonixIntegrationDecision = z.enum([
  'reasonix-engine-stronger-absorb',
  'kun-analytix-baseline-retained',
  'conflict-product-baseline-engine-absorb'
])
export type ReasonixIntegrationDecision = z.infer<typeof ReasonixIntegrationDecision>

export const ReasonixIntegrationBaselineSurface = z
  .object({
    id: z.string().min(1),
    surface: z.string().min(1),
    triggerPath: z.string().min(1),
    runtimeContract: z.string().min(1),
    settingsBridgePolicy: z.string().min(1),
    providerPolicy: z.string().min(1),
    decision: ReasonixIntegrationDecision,
    protectionTests: z.array(z.string().min(1)).min(1)
  })
  .strict()
export type ReasonixIntegrationBaselineSurface = z.infer<typeof ReasonixIntegrationBaselineSurface>

export const ReasonixIntegrationTopologyRow = z
  .object({
    id: z.string().min(1),
    capability: z.string().min(1),
    comparisonConclusion: ReasonixIntegrationDecision,
    decision: z.string().min(1),
    reasonixStrongerBecause: z.string().min(1),
    kunAnalytixRetainedBecause: z.string().min(1),
    conflictPolicy: z.string().min(1),
    adoptedEngineConstants: z.array(z.string().min(1)).min(1),
    retainedProductConstants: z.array(z.string().min(1)).min(1),
    reasonixEngineNodes: z.array(z.string().min(1)).min(1),
    kunAnalytixBaselineAnchors: z.array(z.string().min(1)).min(1),
    analytixEntryPoints: z.array(z.string().min(1)).min(1),
    runtimeContracts: z.array(z.string().min(1)).min(1),
    goRuntimeLanding: z.array(z.string().min(1)).min(1),
    typeScriptFallback: z.array(z.string().min(1)).min(1),
    machineChecks: z.array(RuntimeMachineCheck).min(1),
    providerFamilies: z.array(z.string().min(1)).optional(),
    absorptionClass: RuntimeAbsorptionClass,
    status: RuntimeAbsorptionStatus,
    providerSpecific: z.boolean(),
    doesNotNarrowProviders: z.literal(true),
    existingEntryOnly: z.literal(true),
    topLevelEntrypointAdded: z.literal(false),
    upstreamPublicProtocolAdded: z.literal(false),
    rendererContractChanged: z.literal(false),
    settingsSchemaChanged: z.literal(false),
    productIdentityChanged: z.literal(false),
    stablePrefixContainsDynamicState: z.literal(false),
    evidenceState: ReasonixIntegrationEvidenceState
  })
  .strict()
export type ReasonixIntegrationTopologyRow = z.infer<typeof ReasonixIntegrationTopologyRow>

export const ReasonixIntegrationTopology = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('reasonix-integration-topology'),
    reasonixSourcePath: z.literal('/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    reasonixSourceCommit: z.string().min(40),
    analytixSourceRoot: z.literal('/Users/sun/Projects/analytix'),
    runtimeContract: z.string().min(1),
    principle: z.string().min(1),
    decisionMethod: z.array(z.string().min(1)).min(1),
    baselineSurfaces: z.array(ReasonixIntegrationBaselineSurface).min(1),
    rows: z.array(ReasonixIntegrationTopologyRow).min(5),
    baselineSurfaceCount: z.number().int().positive(),
    rowCount: z.literal(5),
    reasonixStrongerAbsorbedCount: z.literal(5),
    conflictPolicyCount: z.number().int().nonnegative(),
    kunAnalytixRetainedSurfaceCount: z.number().int().positive(),
    codeLevelAbsorbedCount: z.literal(5),
    existingEntryOnlyCount: z.literal(5),
    localContractGreenCount: z.literal(5),
    topLevelEntrypointAddedCount: z.literal(0),
    upstreamPublicProtocolAddedCount: z.literal(0),
    rendererContractChangedCount: z.literal(0),
    settingsSchemaChangedCount: z.literal(0),
    productIdentityChangedCount: z.literal(0),
    stablePrefixDynamicStateRowCount: z.literal(0),
    deepSeekEnhancementScopedOnly: z.literal(true),
    multiModelNonRegressionProtected: z.literal(true),
    typeScriptFallbackRetained: z.literal(false),
    strictG6DefaultCutoverReady: z.boolean(),
    goDefaultCutoverCandidate: z.boolean(),
    externalEvidenceBlockers: z.array(z.string().min(1)),
    forbiddenTopLevelEntrypoints: z.array(z.string().min(1)),
    notes: z.array(z.string().min(1))
  })
  .strict()
export type ReasonixIntegrationTopology = z.infer<typeof ReasonixIntegrationTopology>

export const ReasonixSuperiorityDecision = z.enum(['absorb', 'adapt', 'keep-kun', 'reject', 'defer'])
export type ReasonixSuperiorityDecision = z.infer<typeof ReasonixSuperiorityDecision>

export const ReasonixSuperiorityProductImpact = z
  .object({
    uiSurfaceChanged: z.literal(false),
    settingsSchemaChanged: z.literal(false),
    bridgeChanged: z.literal(false),
    providerMultiModelChanged: z.literal(false),
    providerScoped: z.boolean(),
    doesNotNarrowProviders: z.literal(true),
    topLevelEntrypointAdded: z.literal(false),
    reasonixPublicProtocolAdded: z.literal(false),
    stablePrefixUsesDynamicState: z.literal(false)
  })
  .strict()
export type ReasonixSuperiorityProductImpact = z.infer<typeof ReasonixSuperiorityProductImpact>

export const ReasonixSuperiorityEvidenceRow = z
  .object({
    id: z.string().min(1),
    capability: z.string().min(1),
    reasonixSourcePath: z.literal('/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    reasonixSourceCommit: z.string().min(40),
    reasonixSourceSymbols: z.array(z.string().min(1)).min(1),
    kunAnalytixBaselinePaths: z.array(z.string().min(1)).min(1),
    currentBehavior: z.string().min(1),
    reasonixStrongerEvidence: z.array(z.string().min(1)),
    decisionEvidence: z.array(z.string().min(1)).min(1).optional(),
    deterministicEvidence: z.array(RuntimeMachineCheck).min(1),
    codeReusable: z.boolean(),
    codeReuseMode: z.string().min(1),
    analytixTargetPaths: z.array(z.string().min(1)).min(1),
    productImpact: ReasonixSuperiorityProductImpact,
    regressionTestPaths: z.array(z.string().min(1)).min(1),
    regressionCommands: z.array(z.string().min(1)).min(1).optional(),
    decision: ReasonixSuperiorityDecision,
    absorptionClass: RuntimeAbsorptionClass,
    status: RuntimeAbsorptionStatus,
    codeLevelAbsorbed: z.boolean(),
    localDeterministicGreen: z.boolean(),
    requiresLiveEvidence: z.boolean(),
    liveEvidenceStatus: z.enum(['blocked', 'ready', 'not-required', 'post-cutover-live-validation-pending']),
    keepKunReason: z.string().min(1).optional(),
    kunAnalytixStrongerEvidence: z.array(z.string().min(1)).min(1).optional(),
    baselineRetainedReason: z.string().min(1).optional(),
    rejectedReason: z.string().min(1).optional(),
    deferredReason: z.string().min(1).optional(),
    typeScriptFallbackRetained: z.literal(false)
  })
  .strict()
  .superRefine((row, ctx) => {
    if (row.decision === 'defer') {
      if (row.localDeterministicGreen !== false) {
        ctx.addIssue({
          code: 'custom',
          path: ['localDeterministicGreen'],
          message: 'defer rows must not count as local deterministic green'
        })
      }
    } else if (row.localDeterministicGreen !== true) {
      ctx.addIssue({
        code: 'custom',
        path: ['localDeterministicGreen'],
        message: 'non-defer rows must count as local deterministic green'
      })
    }
    if (row.decision === 'absorb' || row.decision === 'adapt') {
      if (!row.reasonixStrongerEvidence || row.reasonixStrongerEvidence.length === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['reasonixStrongerEvidence'],
          message: 'absorb/adapt rows must include machine-readable Reasonix stronger evidence'
        })
      }
      if (row.decisionEvidence && row.decisionEvidence.length > 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['decisionEvidence'],
          message: 'absorb/adapt rows must not use decision evidence'
        })
      }
      return
    }
    if (row.reasonixStrongerEvidence && row.reasonixStrongerEvidence.length > 0) {
      ctx.addIssue({
        code: 'custom',
        path: ['reasonixStrongerEvidence'],
        message: 'non-absorb/adapt rows must not claim Reasonix stronger evidence'
      })
    }
    if (row.decision === 'keep-kun') {
      if (!row.keepKunReason) {
        ctx.addIssue({
          code: 'custom',
          path: ['keepKunReason'],
          message: 'keep-kun rows must explain why the Kun/Analytix baseline is retained'
        })
      }
      if (!row.kunAnalytixStrongerEvidence || row.kunAnalytixStrongerEvidence.length === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['kunAnalytixStrongerEvidence'],
          message: 'keep-kun rows must include machine-readable Kun/Analytix stronger evidence'
        })
      }
      if (!row.baselineRetainedReason) {
        ctx.addIssue({
          code: 'custom',
          path: ['baselineRetainedReason'],
          message: 'keep-kun rows must include a machine-readable baseline retention reason'
        })
      }
    }
    if (row.decision === 'reject') {
      if (!row.rejectedReason) {
        ctx.addIssue({
          code: 'custom',
          path: ['rejectedReason'],
          message: 'reject rows must include a machine-readable rejected reason'
        })
      }
      if (!row.decisionEvidence || row.decisionEvidence.length === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['decisionEvidence'],
          message: 'reject rows must include machine-readable decision evidence'
        })
      }
    }
    if (row.decision === 'defer') {
      if (!row.deferredReason) {
        ctx.addIssue({
          code: 'custom',
          path: ['deferredReason'],
          message: 'defer rows must include a machine-readable deferred reason'
        })
      }
      if (!row.decisionEvidence || row.decisionEvidence.length === 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['decisionEvidence'],
          message: 'defer rows must include machine-readable decision evidence'
        })
      }
    }
  })
export type ReasonixSuperiorityEvidenceRow = z.infer<typeof ReasonixSuperiorityEvidenceRow>

export const ReasonixSuperiorityMatrix = z
  .object({
    schemaVersion: z.literal(1),
    changeId: z.literal('reasonix-superiority-matrix'),
    stage: z.literal('reasonix-superiority-code-stage'),
    reasonixAbsorptionStatus: z.literal('code-stage-closed'),
    goRuntimeCoreStatus: z.literal('deterministic-core-green'),
    goDefaultLiveGateStatus: z.enum(['blocked', 'ready', 'post-cutover-live-validation-pending']),
    reasonixSourcePath: z.literal('/Users/sun/Projects/_upstreams/DeepSeek-Reasonix'),
    reasonixSourceCommit: z.string().min(40),
    kunSourceSnapshots: z.array(z.string().min(1)).min(1),
    analytixSourceRoot: z.literal('/Users/sun/Projects/analytix'),
    evidencePolicy: z.string().min(1),
    llmAnswerQualityEvidenceUsed: z.literal(false),
    deterministicEvidenceOnly: z.literal(true),
    rows: z.array(ReasonixSuperiorityEvidenceRow).min(13),
    rowCount: z.literal(13),
    absorbCount: z.literal(1),
    adaptCount: z.literal(5),
    keepKunCount: z.literal(5),
    rejectCount: z.literal(1),
    deferCount: z.literal(1),
    codeLevelAbsorbedCount: z.literal(6),
    localDeterministicGreenCount: z.literal(12),
    productImpactChangedCount: z.literal(0),
    topLevelEntrypointAddedCount: z.literal(0),
    reasonixPublicProtocolAddedCount: z.literal(0),
    providerMultiModelChangedCount: z.literal(0),
    stablePrefixDynamicStateRowCount: z.literal(0),
    deepSeekEnhancementScopedOnly: z.literal(true),
    kunAnalytixProductLayerPreserved: z.literal(true),
    reasonixEngineRuntimeOnly: z.literal(true),
    typeScriptFallbackRetained: z.literal(false),
    codeStageClosed: z.literal(true),
    deterministicAbsorptionIds: z.array(z.string().min(1)).min(6),
    pendingLiveValidationIds: z.array(z.string().min(1)).min(1),
    strictG6DefaultCutoverReady: z.boolean(),
    goDefaultCutoverCandidate: z.boolean(),
    liveCutoverStage: z.literal('reasonix-superiority-live-gate'),
    liveCutoverStatus: z.enum(['blocked', 'ready', 'post-cutover-live-validation-pending']),
    liveEvidenceBlockers: z.array(z.string().min(1)),
    forbiddenTopLevelEntrypoints: z.array(z.string().min(1)).min(1),
    notes: z.array(z.string().min(1))
  })
  .strict()
  .superRefine((matrix, ctx) => {
    const absorbedIds = matrix.rows
      .filter((row) => row.codeLevelAbsorbed)
      .map((row) => row.id)
      .sort()
    const deterministicAbsorptionIds = [...matrix.deterministicAbsorptionIds].sort()
    if (JSON.stringify(absorbedIds) !== JSON.stringify(deterministicAbsorptionIds)) {
      ctx.addIssue({
        code: 'custom',
        path: ['deterministicAbsorptionIds'],
        message: 'deterministicAbsorptionIds must list exactly the code-level absorbed rows'
      })
    }
    const liveIds = matrix.rows
      .filter((row) => row.requiresLiveEvidence)
      .map((row) => row.id)
      .sort()
    const pendingLiveValidationIds = [...matrix.pendingLiveValidationIds].sort()
    if (JSON.stringify(liveIds) !== JSON.stringify(pendingLiveValidationIds)) {
      ctx.addIssue({
        code: 'custom',
        path: ['pendingLiveValidationIds'],
        message: 'pendingLiveValidationIds must list exactly the live-gated rows'
      })
    }
    for (const row of matrix.rows) {
      if ((row.decision === 'absorb' || row.decision === 'adapt') &&
        (row.requiresLiveEvidence || row.liveEvidenceStatus !== 'not-required')) {
        ctx.addIssue({
          code: 'custom',
          path: ['rows', row.id, 'requiresLiveEvidence'],
          message: 'Reasonix absorb/adapt evidence must be deterministic and must not require live provider credentials'
        })
      }
    }
  })
export type ReasonixSuperiorityMatrix = z.infer<typeof ReasonixSuperiorityMatrix>

export const UpstreamAbsorptionCapabilityMetadata = z
  .object({
    reasonixCapabilityMatrix: ReasonixCapabilityAuditMatrix,
    kunAnalytixBaselineGuard: KunAnalytixBaselineGuard,
    reasonixAbsorptionMatrix: ReasonixAbsorptionMatrix,
    reasonixIntegrationTopology: ReasonixIntegrationTopology,
    reasonixSuperiorityMatrix: ReasonixSuperiorityMatrix,
    defaultBackendReadiness: RuntimeDefaultBackendReadiness,
    readinessSemantics: RuntimeReadinessSemantics
  })
  .strict()
export type UpstreamAbsorptionCapabilityMetadata = z.infer<typeof UpstreamAbsorptionCapabilityMetadata>

export const RuntimeCapabilityManifest = z
  .object({
    contractVersion: z.literal(RUNTIME_CAPABILITY_CONTRACT_VERSION),
    model: ModelCapabilityMetadata,
    cli: z
      .object({
        serve: RuntimeCapabilityState,
        run: RuntimeCapabilityState,
        chat: RuntimeCapabilityState,
        exec: RuntimeCapabilityState
      })
      .strict(),
    mcp: RuntimeCapabilityState.extend({
      configuredServers: z.number().int().nonnegative(),
      connectedServers: z.number().int().nonnegative(),
      toolCount: z.number().int().nonnegative(),
      promptCount: z.number().int().nonnegative(),
      resourceCount: z.number().int().nonnegative(),
      catalog: z
        .object({
          status: z.string().min(1),
          reason: z.string().optional(),
          toolCount: z.number().int().nonnegative(),
          advertisedToolCount: z.number().int().nonnegative().optional(),
          promptCount: z.number().int().nonnegative(),
          resourceCount: z.number().int().nonnegative(),
          catalogFingerprint: z.string().optional(),
          catalogDrift: z.boolean().optional()
        })
        .strict(),
      search: z
        .object({
          enabled: z.boolean(),
          mode: McpToolDiscoveryMode,
          active: z.boolean(),
          available: z.boolean().optional(),
          reason: z.string().optional(),
          indexedToolCount: z.number().int().nonnegative(),
          advertisedToolCount: z.number().int().nonnegative(),
          autoThresholdToolCount: z.number().int().nonnegative().optional(),
          topKDefault: z.number().int().nonnegative().optional(),
          topKMax: z.number().int().nonnegative().optional(),
          minScore: z.number().nonnegative().optional(),
          catalogFingerprint: z.string().optional(),
          catalogDrift: z.boolean().optional()
        })
        .strict()
    }).strict(),
    web: RuntimeCapabilityState.extend({
      fetch: RuntimeCapabilityState,
      search: RuntimeCapabilityState,
      provider: z.string().optional(),
      fetchEnabled: z.boolean().optional(),
      searchEnabled: z.boolean().optional()
    }).strict(),
    skills: RuntimeCapabilityState.extend({
      configuredRoots: z.number().int().nonnegative(),
      discoveredSkills: z.number().int().nonnegative()
    }).strict(),
    subagents: RuntimeCapabilityState.extend({
      maxParallel: z.number().int().nonnegative(),
      maxChildRuns: z.number().int().nonnegative(),
      defaultToolPolicy: SubagentToolPolicy,
      defaultProfile: z.string().optional(),
      internalLineageAvailable: z.boolean().optional(),
      productSurfaceExposed: z.boolean().optional(),
      profilesAvailable: z.boolean().optional(),
      durableChildRunStore: z.boolean().optional(),
      parallelExecutionAvailable: z.boolean().optional(),
      taskToolAvailable: z.boolean().optional(),
      parallelTasksToolAvailable: z.boolean().optional(),
      backgroundTaskJobsAvailable: z.boolean().optional(),
      backgroundShellAvailable: z.boolean().optional(),
      backgroundSubagentJobsAvailable: z.boolean().optional(),
      modelJobToolsAvailable: z.boolean().optional(),
      taskJobThreadScopeSupported: z.boolean().optional(),
      taskJobRoutes: z.array(z.string().min(1)).optional(),
      topLevelRouteExposed: z.boolean().optional(),
      profiles: z
        .array(RuntimeSubagentProfileState)
        .default([])
    }).strict(),
    attachments: RuntimeCapabilityState.extend({
      maxImageBytes: z.number().int().positive(),
      maxImageDimension: z.number().int().positive(),
      allowedMimeTypes: z.array(z.string().min(1)),
      allowedDocumentMimeTypes: z.array(z.string().min(1)),
      maxDocumentBytes: z.number().int().positive(),
      maxDocumentTextChars: z.number().int().positive(),
      textFallbackMaxBase64Bytes: z.number().int().positive(),
      textFallbackMaxImageDimension: z.number().int().positive(),
      textFallbackPreferredMimeType: z.string().min(1)
    }).strict(),
    memory: RuntimeCapabilityState.extend({
      mode: z.literal('manual').optional(),
      storeOnly: z.boolean().optional(),
      modelInjection: z.boolean().optional(),
      automaticCapture: z.boolean().optional(),
      scopes: z.array(z.enum(['user', 'workspace', 'project'])),
      maxInjectedRecords: z.number().int().nonnegative()
    }).strict(),
    imageGen: RuntimeCapabilityState.extend({
      model: z.string().optional()
    }).strict(),
    speechGen: RuntimeCapabilityState.extend({
      model: z.string().optional()
    }).strict(),
    musicGen: RuntimeCapabilityState.extend({
      model: z.string().optional()
    }).strict(),
    videoGen: RuntimeCapabilityState.extend({
      model: z.string().optional()
    }).strict(),
    computerUse: RuntimeCapabilityState.extend({
      mode: ComputerUseMode,
      backendId: z.string().optional(),
      preferredBackendId: z.string().optional()
    }).strict(),
    visionBridge: RuntimeCapabilityState.extend({
      mode: VisionBridgeMode,
      model: z.string().optional(),
      providerId: z.string().optional(),
      maxScreenshotsPerTurn: z.number().int().positive(),
      maxImagesPerTurn: z.number().int().positive().optional(),
      maxImageBytes: z.number().int().positive().optional(),
      maxImageDimension: z.number().int().positive().optional(),
      semanticProbeStatus: ModelCapabilityProbeStatus
    }).strict(),
    upstreamAbsorption: UpstreamAbsorptionCapabilityMetadata.optional()
  })
  .strict()
export type RuntimeCapabilityManifest = z.infer<typeof RuntimeCapabilityManifest>

export function buildRuntimeCapabilityManifest(input: {
  config?: AnalytixCapabilitiesConfig
  model: ModelCapabilityMetadata
  mcp?: {
    configuredServers?: number
    connectedServers?: number
    toolCount?: number
    promptCount?: number
    resourceCount?: number
    catalog?: {
      status?: string
      reason?: string
      toolCount?: number
      advertisedToolCount?: number
      promptCount?: number
      resourceCount?: number
      catalogFingerprint?: string
      catalogDrift?: boolean
    }
    lastError?: string
    search?: {
      active?: boolean
      indexedToolCount?: number
      advertisedToolCount?: number
    }
  }
  web?: {
    fetchAvailable?: boolean
    searchAvailable?: boolean
    provider?: string
    reason?: string
  }
  skills?: {
    configuredRoots?: number
    discoveredSkills?: number
    reason?: string
  }
  attachments?: {
    available?: boolean
    reason?: string
  }
  memory?: {
    available?: boolean
    reason?: string
  }
  subagents?: {
    available?: boolean
    reason?: string
  }
  imageGen?: {
    available?: boolean
    reason?: string
  }
  speechGen?: {
    available?: boolean
    reason?: string
  }
  musicGen?: {
    available?: boolean
    reason?: string
  }
  videoGen?: {
    available?: boolean
    reason?: string
  }
  computerUse?: {
    available?: boolean
    reason?: string
    backendId?: string
    preferredBackendId?: string
  }
  visionBridge?: {
    available?: boolean
    reason?: string
  }
}): RuntimeCapabilityManifest {
  const config = AnalytixCapabilitiesConfig.parse(input.config ?? {})
  const configuredMcpServers = input.mcp?.configuredServers ?? Object.keys(config.mcp.servers).length
  const connectedMcpServers = input.mcp?.connectedServers ?? 0
  const mcpToolCount = input.mcp?.toolCount ?? 0
  const mcpPromptCount = input.mcp?.promptCount ?? 0
  const mcpResourceCount = input.mcp?.resourceCount ?? 0
  const mcpState = mcpCapabilityState(config.mcp.enabled, connectedMcpServers, input.mcp?.lastError)
  const webFetchState = providerCapabilityState(
    config.web.enabled && config.web.fetchEnabled,
    'web fetch is disabled by config',
    input.web?.fetchAvailable === true,
    input.web?.reason ?? 'web fetch provider is unavailable'
  )
  const webSearchState = providerCapabilityState(
    config.web.enabled && config.web.searchEnabled,
    'web search is disabled by config',
    input.web?.searchAvailable === true,
    input.web?.reason ?? 'web search provider is unavailable'
  )
  const webState = webCapabilityState(config.web.enabled, webFetchState, webSearchState, input.web?.reason)
  const configuredSkillRoots = input.skills?.configuredRoots ?? config.skills.roots.length
  const discoveredSkills = input.skills?.discoveredSkills ?? 0
  const skillsState = skillsCapabilityState(config.skills.enabled, discoveredSkills, input.skills?.reason)
  return RuntimeCapabilityManifest.parse({
    contractVersion: RUNTIME_CAPABILITY_CONTRACT_VERSION,
    model: input.model,
    cli: {
      serve: available(),
      run: unavailable('not implemented'),
      chat: unavailable('not implemented'),
      exec: unavailable('not implemented')
    },
    mcp: {
      ...mcpState,
      configuredServers: configuredMcpServers,
      connectedServers: connectedMcpServers,
      toolCount: mcpToolCount,
      promptCount: mcpPromptCount,
      resourceCount: mcpResourceCount,
      catalog: {
        status: input.mcp?.catalog?.status ?? mcpState.status,
        reason: input.mcp?.catalog?.reason ?? mcpState.reason,
        toolCount: input.mcp?.catalog?.toolCount ?? mcpToolCount,
        advertisedToolCount: input.mcp?.catalog?.advertisedToolCount,
        promptCount: input.mcp?.catalog?.promptCount ?? mcpPromptCount,
        resourceCount: input.mcp?.catalog?.resourceCount ?? mcpResourceCount,
        catalogFingerprint: input.mcp?.catalog?.catalogFingerprint,
        catalogDrift: input.mcp?.catalog?.catalogDrift
      },
      search: {
        enabled: config.mcp.search.enabled,
        mode: config.mcp.search.mode,
        active: input.mcp?.search?.active ?? false,
        indexedToolCount: input.mcp?.search?.indexedToolCount ?? mcpToolCount,
        advertisedToolCount: input.mcp?.search?.advertisedToolCount ?? mcpToolCount
      }
    },
    web: {
      ...webState,
      fetch: webFetchState,
      search: webSearchState,
      provider: input.web?.provider ?? config.web.provider,
      fetchEnabled: config.web.enabled && config.web.fetchEnabled,
      searchEnabled: config.web.enabled && config.web.searchEnabled
    },
    skills: {
      ...skillsState,
      configuredRoots: configuredSkillRoots,
      discoveredSkills
    },
    subagents: {
      ...providerCapabilityState(
        config.subagents.enabled,
        'subagents are disabled by config',
        input.subagents?.available === true,
        input.subagents?.reason ?? 'subagent runtime is unavailable'
      ),
      maxParallel: config.subagents.maxParallel,
      maxChildRuns: config.subagents.maxChildRuns,
      defaultToolPolicy: config.subagents.defaultToolPolicy,
      ...(config.subagents.defaultProfile ? { defaultProfile: config.subagents.defaultProfile } : {}),
      profiles: Object.entries(config.subagents.profiles).map(([name, profile]) => ({
        name,
        ...(profile.providerId ? { providerId: profile.providerId } : {}),
        ...(profile.model ? { model: profile.model } : {}),
        ...(profile.variant ? { variant: profile.variant } : {}),
        ...(profile.endpointFormat ? { endpointFormat: profile.endpointFormat } : {}),
        ...(profile.effort ? { effort: profile.effort } : {}),
        ...(profile.promptPreamble ? { promptPreamble: true } : {}),
        ...(profile.tools.length > 0 ? { tools: profile.tools } : {}),
        toolPolicy: profile.toolPolicy
      }))
    },
    attachments: {
      ...providerCapabilityState(
        config.attachments.enabled,
        'attachments are disabled by config',
        input.attachments?.available === true,
        input.attachments?.reason ?? 'attachment store is unavailable'
      ),
      maxImageBytes: config.attachments.maxImageBytes,
      maxImageDimension: config.attachments.maxImageDimension,
      allowedMimeTypes: config.attachments.allowedMimeTypes,
      allowedDocumentMimeTypes: config.attachments.allowedDocumentMimeTypes,
      maxDocumentBytes: config.attachments.maxDocumentBytes,
      maxDocumentTextChars: config.attachments.maxDocumentTextChars,
      textFallbackMaxBase64Bytes: config.attachments.textFallbackMaxBase64Bytes,
      textFallbackMaxImageDimension: config.attachments.textFallbackMaxImageDimension,
      textFallbackPreferredMimeType: config.attachments.textFallbackPreferredMimeType
    },
    memory: {
      ...providerCapabilityState(
        config.memory.enabled,
        'memory is disabled by config',
        input.memory?.available === true,
        input.memory?.reason ?? 'memory store is unavailable'
      ),
      scopes: config.memory.scopes,
      // The production Go memory surface is a manual registry. Keep the
      // configured future limit, but do not advertise records as injectable
      // until model-context injection is implemented.
      maxInjectedRecords: 0
    },
    imageGen: {
      ...providerCapabilityState(
        config.imageGen.enabled,
        'image generation is disabled by config',
        input.imageGen?.available === true,
        input.imageGen?.reason ?? 'image generation provider is not configured'
      ),
      ...(config.imageGen.model ? { model: config.imageGen.model } : {})
    },
    speechGen: {
      ...providerCapabilityState(
        config.speechGen.enabled,
        'speech generation is disabled by config',
        input.speechGen?.available === true,
        input.speechGen?.reason ?? 'speech generation provider is not configured'
      ),
      ...(config.speechGen.model ? { model: config.speechGen.model } : {})
    },
    musicGen: {
      ...providerCapabilityState(
        config.musicGen.enabled,
        'music generation is disabled by config',
        input.musicGen?.available === true,
        input.musicGen?.reason ?? 'music generation provider is not configured'
      ),
      ...(config.musicGen.model ? { model: config.musicGen.model } : {})
    },
    videoGen: {
      ...providerCapabilityState(
        config.videoGen.enabled,
        'video generation is disabled by config',
        input.videoGen?.available === true,
        input.videoGen?.reason ?? 'video generation provider is not configured'
      ),
      ...(config.videoGen.model ? { model: config.videoGen.model } : {})
    },
    computerUse: {
      ...providerCapabilityState(
        config.computerUse.enabled && config.computerUse.mode !== 'off',
        'computer use is disabled by config',
        input.computerUse?.available === true,
        input.computerUse?.reason ?? 'computer-use backend is unavailable on this platform'
      ),
      mode: config.computerUse.mode,
      ...(input.computerUse?.backendId ? { backendId: input.computerUse.backendId } : {}),
      ...(input.computerUse?.preferredBackendId ? { preferredBackendId: input.computerUse.preferredBackendId } : {})
    },
    visionBridge: {
      ...providerCapabilityState(
        config.visionBridge.enabled && config.visionBridge.mode !== 'off',
        'vision bridge is disabled by config',
        input.visionBridge?.available === true,
        input.visionBridge?.reason ?? 'vision bridge provider is not configured'
      ),
      mode: config.visionBridge.mode,
      ...(config.visionBridge.model ? { model: config.visionBridge.model } : {}),
      ...(config.visionBridge.providerId ? { providerId: config.visionBridge.providerId } : {}),
      maxScreenshotsPerTurn: config.visionBridge.maxScreenshotsPerTurn,
      maxImagesPerTurn: config.visionBridge.maxScreenshotsPerTurn,
      maxImageBytes: config.visionBridge.maxImageBytes,
      maxImageDimension: config.visionBridge.maxImageDimension,
      semanticProbeStatus: config.visionBridge.semanticProbeStatus
    }
  })
}

function available(): RuntimeCapabilityState {
  return { status: 'available', enabled: true, available: true }
}

function unavailable(reason: string): RuntimeCapabilityState {
  return { status: 'unavailable', enabled: false, available: false, reason }
}

function stateFromEnabled(
  enabled: boolean,
  disabledReason: string,
  unavailableReason: string
): RuntimeCapabilityState {
  return enabled
    ? { status: 'unavailable', enabled: true, available: false, reason: unavailableReason }
    : { status: 'disabled', enabled: false, available: false, reason: disabledReason }
}

function providerCapabilityState(
  enabled: boolean,
  disabledReason: string,
  availableProvider: boolean,
  unavailableReason: string
): RuntimeCapabilityState {
  if (!enabled) return { status: 'disabled', enabled: false, available: false, reason: disabledReason }
  return availableProvider
    ? { status: 'available', enabled: true, available: true }
    : { status: 'unavailable', enabled: true, available: false, reason: unavailableReason }
}

function webCapabilityState(
  enabled: boolean,
  fetchState: RuntimeCapabilityState,
  searchState: RuntimeCapabilityState,
  reason: string | undefined
): RuntimeCapabilityState {
  if (!enabled) return { status: 'disabled', enabled: false, available: false, reason: 'web access is disabled by config' }
  if (fetchState.available || searchState.available) return { status: 'available', enabled: true, available: true }
  return {
    status: 'unavailable',
    enabled: true,
    available: false,
    reason: reason ?? 'no web providers available'
  }
}

function skillsCapabilityState(
  enabled: boolean,
  discoveredSkills: number,
  reason: string | undefined
): RuntimeCapabilityState {
  if (!enabled) return { status: 'disabled', enabled: false, available: false, reason: 'Skills are disabled by config' }
  if (discoveredSkills > 0) return { status: 'available', enabled: true, available: true }
  return {
    status: 'unavailable',
    enabled: true,
    available: false,
    reason: reason ?? 'no Skills discovered'
  }
}

function mcpCapabilityState(
  enabled: boolean,
  connectedServers: number,
  lastError: string | undefined
): RuntimeCapabilityState {
  if (!enabled) return { status: 'disabled', enabled: false, available: false, reason: 'MCP is disabled by config' }
  if (connectedServers > 0) return { status: 'available', enabled: true, available: true }
  return {
    status: 'unavailable',
    enabled: true,
    available: false,
    reason: lastError ?? 'no MCP servers connected'
  }
}
