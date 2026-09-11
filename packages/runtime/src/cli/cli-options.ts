import { z } from 'zod'
import {
  ApprovalPolicySchema,
  DEFAULT_APPROVAL_POLICY,
  DEFAULT_SANDBOX_MODE,
  SandboxModeSchema
} from '../contracts/policy.js'
import {
  ContextCompactionConfigSchema,
  DEFAULT_ANALYTIX_MODEL,
  DEFAULT_STORAGE_CONFIG,
  ModelConfigSchema,
  ModelProvidersConfigSchema,
  QualityConfigSchema,
  RuntimeTuningConfigSchema,
  StorageConfigSchema,
  TokenEconomyConfigSchema
} from '../config/analytix-config.js'
import {
  DEFAULT_ANALYTIX_CAPABILITIES_CONFIG,
  AnalytixCapabilitiesConfig
} from '../contracts/capabilities.js'
import {
  DEFAULT_MODEL_ENDPOINT_FORMAT,
  MODEL_ENDPOINT_FORMATS,
  preprocessModelEndpointFormat
} from '../contracts/model-endpoint-format.js'
import { HooksConfigSchema } from '../hooks/hook-config.js'

export const DEFAULT_SERVE_PORT = 8899
export const DEFAULT_SERVE_MODEL = DEFAULT_ANALYTIX_MODEL

/**
 * Validated CLI options for `analytix serve`.
 *
 * `host` and `port` decide the bind address. `dataDir` is the on-disk root
 * for thread JSONL logs and indexes. `runtimeToken` is the bearer token
 * the GUI must send for `/v1/*` requests. The optional `insecure` flag
 * disables the token check (only allowed when the GUI is local).
 */
export const ServeOptionsSchema = z.object({
  configPath: z.string().optional(),
  host: z.string().default('127.0.0.1'),
  port: z.number().int().min(0).max(65_535).default(DEFAULT_SERVE_PORT),
  dataDir: z.string().min(1),
  runtimeToken: z.string().default(''),
  apiKey: z.string().default(''),
  baseUrl: z.string().default('https://api.deepseek.com'),
  modelProxyUrl: z.string().default(''),
  endpointFormat: z.preprocess(preprocessModelEndpointFormat, z.enum(MODEL_ENDPOINT_FORMATS)).default(DEFAULT_MODEL_ENDPOINT_FORMAT),
  model: z.string().default(DEFAULT_SERVE_MODEL),
  approvalPolicy: ApprovalPolicySchema.default(DEFAULT_APPROVAL_POLICY),
  sandboxMode: SandboxModeSchema.default(DEFAULT_SANDBOX_MODE),
  tokenEconomyMode: z.boolean().default(false),
  tokenEconomy: TokenEconomyConfigSchema.optional(),
  insecure: z.boolean().default(false),
  storage: StorageConfigSchema.default(DEFAULT_STORAGE_CONFIG),
  models: ModelConfigSchema.optional(),
  modelProviders: ModelProvidersConfigSchema.optional(),
  contextCompaction: ContextCompactionConfigSchema.optional(),
  runtime: RuntimeTuningConfigSchema.optional(),
  capabilities: AnalytixCapabilitiesConfig.default(DEFAULT_ANALYTIX_CAPABILITIES_CONFIG),
  hooks: HooksConfigSchema.optional(),
  quality: QualityConfigSchema.optional()
})
export type ServeOptions = z.infer<typeof ServeOptionsSchema>

export const DEFAULT_SERVE_OPTIONS: ServeOptions = {
  host: '127.0.0.1',
  port: DEFAULT_SERVE_PORT,
  dataDir: '',
  runtimeToken: '',
  apiKey: '',
  baseUrl: 'https://api.deepseek.com',
  modelProxyUrl: '',
  endpointFormat: DEFAULT_MODEL_ENDPOINT_FORMAT,
  model: DEFAULT_SERVE_MODEL,
  approvalPolicy: DEFAULT_APPROVAL_POLICY,
  sandboxMode: DEFAULT_SANDBOX_MODE,
  tokenEconomyMode: false,
  insecure: false,
  storage: DEFAULT_STORAGE_CONFIG,
  capabilities: DEFAULT_ANALYTIX_CAPABILITIES_CONFIG
}
