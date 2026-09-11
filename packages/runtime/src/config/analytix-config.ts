import { existsSync, readFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import { z } from 'zod'
import {
  ApprovalPolicySchema,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  SandboxModeSchema
} from '../contracts/policy.js'
import {
  DEFAULT_ANALYTIX_CAPABILITIES_CONFIG,
  AnalytixCapabilitiesConfig,
  ModelInputModality,
  ModelMessagePartSupport,
  ModelReasoningCapabilityMetadata
} from '../contracts/capabilities.js'
import {
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  MODEL_ENDPOINT_FORMATS,
  preprocessModelEndpointFormat
} from '../contracts/model-endpoint-format.js'
import { HooksConfigSchema } from '../hooks/hook-config.js'

export const ANALYTIX_CONFIG_FILENAME = 'config.json'
export const DEFAULT_ANALYTIX_MODEL = 'deepseek-v4-flash'

const PositiveInt = z.number().int().positive()
const NonNegativeInt = z.number().int().nonnegative()
const PositiveRatio = z.number().positive().max(1)
const ProviderPricingConfigSchema = z
  .object({
    cacheHit: z.number().nonnegative().max(1_000_000).optional(),
    input: z.number().nonnegative().max(1_000_000).optional(),
    output: z.number().nonnegative().max(1_000_000).optional(),
    currency: z.enum(['USD', 'CNY']).optional()
  })
  .strict()

export const ModelContextCompactionProfileConfigSchema = z
  .object({
    softRatio: PositiveRatio.optional(),
    hardRatio: PositiveRatio.optional(),
    softThreshold: PositiveInt.optional(),
    hardThreshold: PositiveInt.optional()
  })
  .strict()
  .superRefine((profile, ctx) => {
    if (
      profile.softThreshold !== undefined &&
      profile.hardThreshold !== undefined &&
      profile.hardThreshold < profile.softThreshold
    ) {
      ctx.addIssue({
        code: 'custom',
        message: 'hardThreshold must be greater than or equal to softThreshold'
      })
    }
  })

export const ModelContextProfileConfigSchema = z
  .object({
    aliases: z.array(z.string().min(1)).optional(),
    contextWindowTokens: PositiveInt.optional(),
    contextCompaction: ModelContextCompactionProfileConfigSchema.optional(),
    softRatio: PositiveRatio.optional(),
    hardRatio: PositiveRatio.optional(),
    softThreshold: PositiveInt.optional(),
    hardThreshold: PositiveInt.optional(),
    inputModalities: z.array(ModelInputModality).optional(),
    outputModalities: z.array(ModelInputModality).optional(),
    supportsToolCalling: z.boolean().optional(),
    messageParts: z.array(ModelMessagePartSupport).optional(),
    reasoning: ModelReasoningCapabilityMetadata.optional(),
    // Per-model wire-format override. Omitted means "inherit the
    // provider/runtime endpointFormat" — no default coercion here, otherwise
    // every model would be pinned to chat_completions.
    endpointFormat: z
      .preprocess(preprocessModelEndpointFormat, z.enum(MODEL_ENDPOINT_FORMATS))
      .optional(),
    price: ProviderPricingConfigSchema.optional()
  })
  .strict()
  .superRefine((profile, ctx) => {
    const hasRatio =
      profile.softRatio !== undefined ||
      profile.hardRatio !== undefined ||
      profile.contextCompaction?.softRatio !== undefined ||
      profile.contextCompaction?.hardRatio !== undefined
    if (hasRatio && profile.contextWindowTokens === undefined) {
      ctx.addIssue({
        code: 'custom',
        message: 'softRatio and hardRatio require contextWindowTokens'
      })
    }
    const softThreshold = profile.contextCompaction?.softThreshold ?? profile.softThreshold
    const hardThreshold = profile.contextCompaction?.hardThreshold ?? profile.hardThreshold
    if (softThreshold !== undefined && hardThreshold !== undefined && hardThreshold < softThreshold) {
      ctx.addIssue({
        code: 'custom',
        message: 'hardThreshold must be greater than or equal to softThreshold'
      })
    }
  })

export const ModelConfigSchema = z
  .object({
    profiles: z.record(z.string().min(1), ModelContextProfileConfigSchema).optional()
  })
  .strict()

export const ModelProviderConfigSchema = z
  .object({
    id: z.string().trim().min(1).max(128),
    name: z.string().optional(),
    apiKey: z.string().optional(),
    baseUrl: z.string().trim().min(1),
    modelProxyUrl: z.string().optional(),
    endpointFormat: z
      .preprocess(preprocessModelEndpointFormat, z.enum(MODEL_ENDPOINT_FORMATS))
      .default(DEFAULT_MODEL_ENDPOINT_FORMAT)
      .optional(),
    models: z.array(z.string().trim().min(1)).default([]),
    modelProfiles: z.record(z.string().min(1), ModelContextProfileConfigSchema).optional(),
    price: ProviderPricingConfigSchema.optional(),
    prices: z.record(z.string().min(1), ProviderPricingConfigSchema).optional()
  })
  .strict()

export const ModelProvidersConfigSchema = z
  .object({
    defaultProviderId: z.string().trim().min(1).max(128).optional(),
    providers: z.array(ModelProviderConfigSchema).default([])
  })
  .strict()

export const ContextCompactionConfigSchema = z
  .object({
    defaultSoftThreshold: PositiveInt.optional(),
    defaultHardThreshold: PositiveInt.optional(),
    summaryMode: z.enum(['heuristic', 'model']).optional(),
    summaryTimeoutMs: PositiveInt.optional(),
    summaryMaxTokens: PositiveInt.optional(),
    summaryInputMaxBytes: PositiveInt.optional(),
    modelProfiles: z.record(z.string().min(1), ModelContextProfileConfigSchema).optional()
  })
  .strict()
  .superRefine((config, ctx) => {
    if (
      config.defaultSoftThreshold !== undefined &&
      config.defaultHardThreshold !== undefined &&
      config.defaultHardThreshold < config.defaultSoftThreshold
    ) {
      ctx.addIssue({
        code: 'custom',
        message: 'defaultHardThreshold must be greater than or equal to defaultSoftThreshold'
      })
    }
  })

export const RuntimeTuningConfigSchema = z
  .object({
    // Max idle gap (ms) between streaming chunks before a turn fails with
    // `stream_idle_timeout`. Local LLM servers prefilling a huge prompt can
    // stay silent well past the 45s default; `0` disables the guard entirely.
    streamIdleTimeoutMs: z.number().int().min(0).optional(),
    stepLimits: z
      .object({
        defaultMaxModelSteps: z.number().int().min(0).optional(),
        userGlobalMaxModelSteps: z.number().int().min(0).optional(),
        plannerMaxModelSteps: z.number().int().min(0).optional(),
        headlessMaxModelSteps: z.number().int().min(0).optional()
      })
      .strict()
      .optional(),
    toolStorm: z
      .object({
        enabled: z.boolean().optional(),
        windowSize: PositiveInt.optional(),
        threshold: z.number().int().min(2).optional()
      })
      .strict()
      .optional(),
    toolArgumentRepair: z
      .object({
        maxStringBytes: PositiveInt.optional()
      })
      .strict()
      .optional()
  })
  .strict()

export const DESIGN_QUALITY_STRICTNESS = ['relaxed', 'standard', 'strict'] as const

export const QualityConfigSchema = z
  .object({
    enabled: z.boolean().default(true),
    strictness: z.enum(DESIGN_QUALITY_STRICTNESS).default('standard'),
    ignoreRules: z.array(z.string().min(1)).default([]),
    ignoreFiles: z.array(z.string().min(1)).default([]),
    maxFindings: z.number().int().positive().max(100).default(12)
  })
  .strict()

export const RequestHistoryHygieneConfigSchema = z
  .object({
    maxToolResultLines: PositiveInt.optional(),
    maxToolResultBytes: PositiveInt.optional(),
    maxToolResultTokens: PositiveInt.optional(),
    maxToolArgumentStringBytes: PositiveInt.optional(),
    maxToolArgumentStringTokens: PositiveInt.optional(),
    maxArrayItems: PositiveInt.optional(),
    maxCumulativeToolResultTokens: NonNegativeInt.optional(),
    keepRecentToolResults: NonNegativeInt.optional()
  })
  .strict()

export const TokenEconomyConfigSchema = z
  .object({
    enabled: z.boolean().optional(),
    compressToolDescriptions: z.boolean().optional(),
    compressToolResults: z.boolean().optional(),
    conciseResponses: z.boolean().optional(),
    historyHygiene: RequestHistoryHygieneConfigSchema.optional()
  })
  .strict()

export const StorageConfigSchema = z
  .object({
    backend: z.enum(['hybrid', 'file']).default('hybrid'),
    sqlitePath: z.string().min(1).optional()
  })
  .strict()

export const DEFAULT_STORAGE_CONFIG: StorageConfig = {
  backend: 'hybrid'
}

export const AnalytixServeConfigSchema = z
  .object({
    host: z.string().optional(),
    port: z.number().int().min(0).max(65_535).optional(),
    dataDir: z.string().min(1).optional(),
    runtimeToken: z.string().optional(),
    apiKey: z.string().optional(),
    baseUrl: z.string().optional(),
    modelProxyUrl: z.string().optional(),
    endpointFormat: z.preprocess(
      preprocessModelEndpointFormat,
      z.enum(MODEL_ENDPOINT_FORMATS)
    ).default(DEFAULT_MODEL_ENDPOINT_FORMAT).optional(),
    model: z.string().min(1).optional(),
    approvalPolicy: ApprovalPolicySchema.default(DEFAULT_APPROVAL_POLICY).optional(),
    sandboxMode: SandboxModeSchema.default(DEFAULT_SANDBOX_MODE).optional(),
    tokenEconomyMode: z.boolean().optional(),
    tokenEconomy: TokenEconomyConfigSchema.optional(),
    insecure: z.boolean().optional(),
    storage: StorageConfigSchema.optional()
  })
  .strict()

export const AnalytixConfigSchema = z
  .object({
    serve: AnalytixServeConfigSchema.optional(),
    models: ModelConfigSchema.optional(),
    contextCompaction: ContextCompactionConfigSchema.optional(),
    modelProviders: ModelProvidersConfigSchema.optional(),
    runtime: RuntimeTuningConfigSchema.optional(),
    capabilities: AnalytixCapabilitiesConfig.default(DEFAULT_ANALYTIX_CAPABILITIES_CONFIG),
    hooks: HooksConfigSchema.optional(),
    quality: QualityConfigSchema.optional()
  })
  .strict()

export type AnalytixConfig = z.infer<typeof AnalytixConfigSchema>
export type QualityConfig = z.infer<typeof QualityConfigSchema>
export const DEFAULT_QUALITY_CONFIG: QualityConfig = QualityConfigSchema.parse({})
export type AnalytixServeConfig = z.infer<typeof AnalytixServeConfigSchema>
export type ModelConfig = z.infer<typeof ModelConfigSchema>
export type ModelProviderConfig = z.infer<typeof ModelProviderConfigSchema>
export type ModelProvidersConfig = z.infer<typeof ModelProvidersConfigSchema>
export type ContextCompactionConfig = z.infer<typeof ContextCompactionConfigSchema>
export type RuntimeTuningConfig = z.infer<typeof RuntimeTuningConfigSchema>
export type TokenEconomyConfig = z.infer<typeof TokenEconomyConfigSchema>
export type StorageConfig = z.infer<typeof StorageConfigSchema>

export type LoadedAnalytixConfig = {
  path: string
  config: AnalytixConfig
}

export function readAnalytixConfigFile(path: string): LoadedAnalytixConfig {
  const resolvedPath = expandHomePath(path)
  const text = readFileSync(resolvedPath, 'utf8')
  let json: unknown
  try {
    json = JSON.parse(text)
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`Failed to parse Analytix config JSON at ${resolvedPath}: ${message}`)
  }
  const parsed = AnalytixConfigSchema.safeParse(json)
  if (!parsed.success) {
    throw new Error(
      `Invalid Analytix config at ${resolvedPath}: ${JSON.stringify(parsed.error.issues, null, 2)}`
    )
  }
  return { path: resolvedPath, config: parsed.data }
}

export function readOptionalAnalytixConfigFile(path: string | undefined): LoadedAnalytixConfig | null {
  if (!path) return null
  const resolvedPath = expandHomePath(path)
  if (!existsSync(resolvedPath)) return null
  return readAnalytixConfigFile(resolvedPath)
}

export function analytixConfigPathForDataDir(dataDir: string | undefined): string | undefined {
  const trimmed = dataDir?.trim()
  if (!trimmed) return undefined
  return join(expandHomePath(trimmed), ANALYTIX_CONFIG_FILENAME)
}

export function expandHomePath(path: string): string {
  if (path === '~') return homedir()
  if (path.startsWith('~/') || path.startsWith('~\\')) {
    return join(homedir(), path.slice(2).replace(/\\/g, '/'))
  }
  return path
}
