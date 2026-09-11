import { z } from 'zod'
import {
  PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION,
  PublicMcpSearchV2,
  PublicRuntimeCapabilityStateV2,
  PublicRuntimeIdentifierV2
} from './runtime-info.js'

const Count = z.number().int().nonnegative().max(1_000_000_000)
const PublicIdentifier = PublicRuntimeIdentifierV2
const Sha256 = z.string().regex(/^[a-f0-9]{64}$/)

export const PublicMcpServerDiagnosticV2 = z
  .object({
    id: PublicIdentifier,
    status: z.enum(['connected', 'configured', 'error', 'unavailable']),
    failureCode: z.literal('connection_failed').optional(),
    transport: z.enum(['stdio', 'streamable-http', 'sse', 'unknown']),
    authStatus: z.enum(['none', 'possible', 'required', 'unknown']),
    trustScope: z.enum(['user', 'workspace', 'unknown']),
    enabled: z.boolean(),
    available: z.boolean(),
    connected: z.boolean(),
    schemaHintAvailable: z.boolean(),
    connectable: z.boolean(),
    toolCount: Count,
    promptCount: Count,
    resourceCount: Count,
    toolContractQuarantineCount: Count,
    sourceProbeCount: Count.optional(),
    lowPriority: z.boolean(),
    backgroundStart: z.boolean()
  })
  .strict()
  .superRefine((value, context) => {
    if (value.available && !value.connected) {
      context.addIssue({ code: 'custom', message: 'available MCP server must be connected' })
    }
    if (value.status === 'connected' && !value.connected) {
      context.addIssue({ code: 'custom', message: 'connected MCP status requires a live connection' })
    }
    if (value.status !== 'connected' && value.status !== 'error' && value.connected) {
      context.addIssue({ code: 'custom', message: 'connected MCP server has an inconsistent status' })
    }
    if ((value.status === 'error') !== (value.failureCode === 'connection_failed')) {
      context.addIssue({ code: 'custom', message: 'MCP failure code and status are inconsistent' })
    }
  })

const PublicAttachmentDiagnosticsBodyV2 = z
  .object({
    enabled: z.boolean(),
    count: Count,
    totalBytes: Count,
    maxImageBytes: Count,
    maxImageDimension: Count,
    allowedMimeTypes: z.array(z.string().min(3).max(128)),
    allowedDocumentMimeTypes: z.array(z.string().min(3).max(128)),
    maxDocumentBytes: Count,
    maxDocumentTextChars: Count
  })
  .strict()

export const PublicAttachmentDiagnosticsV2 = PublicAttachmentDiagnosticsBodyV2.extend({
  schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION).optional()
}).strict()

export const AttachmentDiagnosticsResponseV2 = PublicAttachmentDiagnosticsBodyV2.extend({
  schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION)
}).strict()
export type AttachmentDiagnosticsResponseV2 = z.infer<typeof AttachmentDiagnosticsResponseV2>

const PublicMemoryDiagnosticsBodyV2 = z
  .object({
    enabled: z.boolean(),
    status: z.enum(['ok', 'unavailable']).optional(),
    reasonCode: z.enum(['memory_store_read_failed', 'memory_store_unavailable']).optional(),
    activeCount: Count.optional(),
    tombstoneCount: Count.optional()
  })
  .strict()
  .superRefine((value, ctx) => {
    const status = value.status ?? 'ok'
    if (status === 'ok' && (value.activeCount === undefined || value.tombstoneCount === undefined)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'available memory diagnostics require known counts' })
    }
    if (status === 'unavailable' && (value.activeCount !== undefined || value.tombstoneCount !== undefined)) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: 'unavailable memory diagnostics cannot report counts' })
    }
  })

export const PublicMemoryDiagnosticsV2 = PublicMemoryDiagnosticsBodyV2.extend({
  schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION).optional()
}).strict()

export const MemoryDiagnosticsResponseV2 = PublicMemoryDiagnosticsBodyV2.extend({
  schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION)
}).strict()
export type MemoryDiagnosticsResponseV2 = z.infer<typeof MemoryDiagnosticsResponseV2>

export const PublicSubagentDiagnosticsV2 = PublicRuntimeCapabilityStateV2.safeExtend({
    active: Count,
    queued: Count,
    profileCount: Count,
    maxParallel: Count,
    maxChildRuns: Count,
    defaultToolPolicy: z.enum(['readOnly', 'inherit']),
    internalLineageAvailable: z.boolean(),
    parallelExecutionAvailable: z.boolean(),
    taskToolAvailable: z.boolean(),
    parallelTasksToolAvailable: z.boolean(),
    backgroundTaskJobsAvailable: z.boolean(),
    backgroundShellAvailable: z.boolean(),
    backgroundSubagentJobsAvailable: z.boolean()
  })
  .strict()

export const PublicCommandDiagnosticV2 = z
  .object({
    binary: z.enum(['go', 'node', 'npm', 'git', 'rg', 'rustc', 'cargo', 'make', 'docker', 'python3', 'python']),
    found: z.boolean(),
    status: z.enum(['available', 'unavailable'])
  })
  .strict()
  .superRefine((value, context) => {
    const expectedStatus = value.found ? 'available' : 'unavailable'
    if (value.status !== expectedStatus) {
      context.addIssue({ code: 'custom', message: 'command discovery fields are inconsistent' })
    }
  })

export const PublicSkillDiagnosticsV2 = z
  .object({
    enabled: z.boolean(),
    available: z.boolean(),
    reasonCode: z.enum(['available', 'disabled_by_config', 'unavailable']),
    configuredRootCount: Count,
    skillCount: Count,
    validationErrorCount: Count
  })
  .strict()
  .superRefine((value, context) => {
    const expectedReason = value.available
      ? 'available'
      : value.enabled
        ? 'unavailable'
        : 'disabled_by_config'
    if (value.reasonCode !== expectedReason || (value.available && !value.enabled)) {
      context.addIssue({ code: 'custom', message: 'skill diagnostics state fields are inconsistent' })
    }
  })

export const RuntimeToolsResponse = z
  .object({
    schemaVersion: z.literal(PUBLIC_RUNTIME_DIAGNOSTICS_SCHEMA_VERSION),
    providerCount: Count,
    toolContracts: z
      .object({
        count: Count,
        catalogHash: Sha256
      })
      .strict(),
    mcpServers: z.array(PublicMcpServerDiagnosticV2),
    mcpSearch: PublicMcpSearchV2,
    mcpPromptCount: Count,
    mcpResourceCount: Count,
    commands: z.array(PublicCommandDiagnosticV2),
    networkProxy: z
      .object({
        mode: z.enum(['auto', 'env', 'off', 'custom', 'unknown']),
        configured: z.boolean(),
        source: z.enum(['environment', 'settings.provider.proxy', 'unknown']),
        valid: z.boolean(),
        credentialsMasked: z.boolean()
      })
      .strict(),
    webProviderCount: Count,
    skills: PublicSkillDiagnosticsV2,
    attachments: PublicAttachmentDiagnosticsV2,
    memory: PublicMemoryDiagnosticsV2,
    subagents: PublicSubagentDiagnosticsV2
  })
  .strict()

export type RuntimeToolsResponse = z.infer<typeof RuntimeToolsResponse>
