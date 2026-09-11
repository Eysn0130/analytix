import { z } from 'zod'

export const ANALYTIX_CHECKPOINT_SCHEMA_VERSION = 1
export const ANALYTIX_CHECKPOINT_ID_PREFIX = 'axcp_'
export const ANALYTIX_REWIND_PLAN_SCHEMA_VERSION = 1
export const ANALYTIX_REWIND_PLAN_ID_PREFIX = 'axrp_'
export const ANALYTIX_REWIND_APPLY_SCHEMA_VERSION = 1
export const ANALYTIX_REWIND_APPLY_ID_PREFIX = 'axra_'
export const ANALYTIX_REWIND_RESCUE_ID_PREFIX = 'axrr_'
export const ANALYTIX_REWIND_APPLY_CONFIRMATION_PHRASE = 'APPLY_CHECKPOINT_REWIND'

export const CheckpointStatusSchema = z.enum([
  'captured',
  'rewind_planned',
  'rewound',
  'failed'
])
export type CheckpointStatus = z.infer<typeof CheckpointStatusSchema>

export const CheckpointChangeKindSchema = z.enum([
  'created',
  'modified',
  'deleted',
  'unknown'
])
export type CheckpointChangeKind = z.infer<typeof CheckpointChangeKindSchema>

export const CheckpointChangedFileSchema = z.object({
  relativePath: z.string().min(1),
  changeKind: CheckpointChangeKindSchema.default('unknown'),
  beforeHash: z.string().min(1).nullable().optional(),
  afterHash: z.string().min(1).nullable().optional()
}).strict()
export type CheckpointChangedFile = z.infer<typeof CheckpointChangedFileSchema>

export const CheckpointMetadataSchema = z.object({
  schemaVersion: z.literal(ANALYTIX_CHECKPOINT_SCHEMA_VERSION),
  checkpointId: z
    .string()
    .regex(/^axcp_[A-Za-z0-9._-]+$/, 'checkpointId must use the analytix axcp_ prefix'),
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  workspace: z.string().min(1),
  createdAt: z.string(),
  status: CheckpointStatusSchema,
  changedFiles: z.array(CheckpointChangedFileSchema),
  snapshotStorage: z.literal('runtime_private_cas').optional()
}).strict()
export type CheckpointMetadata = z.infer<typeof CheckpointMetadataSchema>

export const CheckpointRewindScopeSchema = z.enum([
  'code',
  'conversation',
  'combined'
])
export type CheckpointRewindScope = z.infer<typeof CheckpointRewindScopeSchema>

export const CheckpointRestoreFileActionSchema = z.enum([
  'restore_previous_version',
  'delete_created_file',
  'restore_deleted_file',
  'noop',
  'manual_review',
  'blocked'
])
export type CheckpointRestoreFileAction = z.infer<typeof CheckpointRestoreFileActionSchema>

export const CheckpointRestoreFileStatusSchema = z.enum([
  'ready',
  'manual_review',
  'blocked'
])
export type CheckpointRestoreFileStatus = z.infer<typeof CheckpointRestoreFileStatusSchema>

export const CheckpointRestoreFileRiskSchema = z.enum([
  'low',
  'medium',
  'high'
])
export type CheckpointRestoreFileRisk = z.infer<typeof CheckpointRestoreFileRiskSchema>

export const CheckpointRestoreFilePlanSchema = z.object({
  relativePath: z.string().min(1),
  changeKind: CheckpointChangeKindSchema,
  action: CheckpointRestoreFileActionSchema,
  status: CheckpointRestoreFileStatusSchema,
  risk: CheckpointRestoreFileRiskSchema,
  reason: z.string().min(1),
  contentSource: z.literal('checkpoint_metadata_only'),
  beforeHash: z.string().min(1).nullable().optional(),
  afterHash: z.string().min(1).nullable().optional()
}).strict()
export type CheckpointRestoreFilePlan = z.infer<typeof CheckpointRestoreFilePlanSchema>

export const CheckpointConversationRewindPlanSchema = z.object({
  status: z.enum(['ready', 'blocked']),
  reason: z.string().optional(),
  boundaryTurnId: z.string().min(1).optional(),
  checkpointEventSeq: z.number().int().nonnegative().optional(),
  boundarySeq: z.number().int().nonnegative().optional(),
  retainedEventCount: z.number().int().nonnegative(),
  removedEventCount: z.number().int().nonnegative(),
  removedTurnIds: z.array(z.string().min(1)),
  projection: z.object({
    latestSeq: z.number().int().nonnegative(),
    turnCount: z.number().int().nonnegative(),
    itemCount: z.number().int().nonnegative(),
    checkpointCount: z.number().int().nonnegative()
  }).strict()
}).strict()
export type CheckpointConversationRewindPlan = z.infer<typeof CheckpointConversationRewindPlanSchema>

export const CheckpointRewindPlanSchema = z.object({
  schemaVersion: z.literal(ANALYTIX_REWIND_PLAN_SCHEMA_VERSION),
  planId: z
    .string()
    .regex(/^axrp_[A-Za-z0-9._-]+$/, 'rewind planId must use the analytix axrp_ prefix'),
  checkpointId: z
    .string()
    .regex(/^axcp_[A-Za-z0-9._-]+$/, 'checkpointId must use the analytix axcp_ prefix'),
  threadId: z.string().min(1),
  planDigest: z.string().regex(/^[a-f0-9]{64}$/, 'planDigest must be a SHA-256 hex digest'),
  workspace: z.string().min(1),
  createdAt: z.string(),
  scope: CheckpointRewindScopeSchema,
  applyMode: z.literal('plan_only'),
  destructive: z.literal(false),
  checkpoint: CheckpointMetadataSchema,
  files: z.array(CheckpointRestoreFilePlanSchema),
  conversation: CheckpointConversationRewindPlanSchema.optional(),
  summary: z.object({
    fileCount: z.number().int().nonnegative(),
    readyFileCount: z.number().int().nonnegative(),
    manualReviewFileCount: z.number().int().nonnegative(),
    blockedFileCount: z.number().int().nonnegative(),
    retainedEventCount: z.number().int().nonnegative(),
    removedEventCount: z.number().int().nonnegative(),
    removedTurnCount: z.number().int().nonnegative(),
    containsRawPrompt: z.literal(false),
    containsFullFileContent: z.literal(false),
    containsSecretValue: z.literal(false)
  }).strict()
}).strict()
export type CheckpointRewindPlan = z.infer<typeof CheckpointRewindPlanSchema>

export const CreateCheckpointRewindPlanRequestSchema = z.object({
  scope: CheckpointRewindScopeSchema.optional().default('combined')
}).strict()
export type CreateCheckpointRewindPlanRequest = z.infer<typeof CreateCheckpointRewindPlanRequestSchema>

export const CheckpointRewindPlanResponseSchema = z.object({
  plan: CheckpointRewindPlanSchema
}).strict()
export type CheckpointRewindPlanResponse = z.infer<typeof CheckpointRewindPlanResponseSchema>

export const CheckpointApplyFileSnapshotSchema = z.object({
  hash: z.string().min(1),
  content: z.string(),
  encoding: z.literal('utf8')
}).strict()
export type CheckpointApplyFileSnapshot = z.infer<typeof CheckpointApplyFileSnapshotSchema>

export const CheckpointApplySnapshotEvidenceSchema = z.object({
  relativePath: z.string().min(1),
  before: CheckpointApplyFileSnapshotSchema.optional(),
  after: CheckpointApplyFileSnapshotSchema.optional()
}).strict()
export type CheckpointApplySnapshotEvidence = z.infer<typeof CheckpointApplySnapshotEvidenceSchema>

export const CheckpointRewindApplyConfirmationSchema = z.object({
  confirmed: z.literal(true),
  destructive: z.literal(true),
  phrase: z.literal(ANALYTIX_REWIND_APPLY_CONFIRMATION_PHRASE)
}).strict()
export type CheckpointRewindApplyConfirmation = z.infer<typeof CheckpointRewindApplyConfirmationSchema>

export const ApplyCheckpointRewindPlanRequestSchema = z.object({
  plan: CheckpointRewindPlanSchema,
  confirmation: CheckpointRewindApplyConfirmationSchema
}).strict()
export type ApplyCheckpointRewindPlanRequest = z.infer<typeof ApplyCheckpointRewindPlanRequestSchema>

export const CheckpointRewindRescueFileSchema = z.object({
  relativePath: z.string().min(1),
  existed: z.boolean(),
  hash: z.string().min(1).optional(),
  content: z.string().optional(),
  encoding: z.literal('utf8').optional()
}).strict()
export type CheckpointRewindRescueFile = z.infer<typeof CheckpointRewindRescueFileSchema>

export const CheckpointRewindRescueRecordSchema = z.object({
  schemaVersion: z.literal(ANALYTIX_REWIND_APPLY_SCHEMA_VERSION),
  rescueId: z
    .string()
    .regex(/^axrr_[A-Za-z0-9._-]+$/, 'rescueId must use the analytix axrr_ prefix'),
  planId: z
    .string()
    .regex(/^axrp_[A-Za-z0-9._-]+$/, 'rewind planId must use the analytix axrp_ prefix'),
  checkpointId: z
    .string()
    .regex(/^axcp_[A-Za-z0-9._-]+$/, 'checkpointId must use the analytix axcp_ prefix'),
  threadId: z.string().min(1),
  workspace: z.string().min(1),
  createdAt: z.string(),
  files: z.array(CheckpointRewindRescueFileSchema)
}).strict()
export type CheckpointRewindRescueRecord = z.infer<typeof CheckpointRewindRescueRecordSchema>

export const CheckpointRewindApplyFileStatusSchema = z.enum([
  'applied',
  'noop',
  'manual_review',
  'blocked',
  'failed'
])
export type CheckpointRewindApplyFileStatus = z.infer<typeof CheckpointRewindApplyFileStatusSchema>

export const CheckpointRewindApplyFileResultSchema = z.object({
  relativePath: z.string().min(1),
  action: CheckpointRestoreFileActionSchema,
  status: CheckpointRewindApplyFileStatusSchema,
  reason: z.string().min(1),
  beforeHash: z.string().min(1).nullable().optional(),
  afterHash: z.string().min(1).nullable().optional(),
  currentHash: z.string().min(1).nullable().optional()
}).strict()
export type CheckpointRewindApplyFileResult = z.infer<typeof CheckpointRewindApplyFileResultSchema>

export const CheckpointRewindApplyConversationResultSchema = z.object({
  status: z.enum(['not_requested', 'audit_recorded', 'blocked', 'already_applied']),
  reason: z.string().optional(),
  boundaryTurnId: z.string().min(1).optional(),
  retainedEventCount: z.number().int().nonnegative().optional(),
  removedEventCount: z.number().int().nonnegative().optional(),
  removedTurnIds: z.array(z.string().min(1)).optional(),
  auditEventSeq: z.number().int().nonnegative().optional()
}).strict()
export type CheckpointRewindApplyConversationResult = z.infer<typeof CheckpointRewindApplyConversationResultSchema>

export const CheckpointRewindApplyResultSchema = z.object({
  schemaVersion: z.literal(ANALYTIX_REWIND_APPLY_SCHEMA_VERSION),
  applyId: z
    .string()
    .regex(/^axra_[A-Za-z0-9._-]+$/, 'applyId must use the analytix axra_ prefix'),
  planId: z
    .string()
    .regex(/^axrp_[A-Za-z0-9._-]+$/, 'rewind planId must use the analytix axrp_ prefix'),
  checkpointId: z
    .string()
    .regex(/^axcp_[A-Za-z0-9._-]+$/, 'checkpointId must use the analytix axcp_ prefix'),
  threadId: z.string().min(1),
  workspace: z.string().min(1),
  createdAt: z.string(),
  scope: CheckpointRewindScopeSchema,
  status: z.enum(['applied', 'blocked', 'failed', 'already_applied']),
  destructive: z.literal(true),
  rescue: z.object({
    rescueId: z
      .string()
      .regex(/^axrr_[A-Za-z0-9._-]+$/, 'rescueId must use the analytix axrr_ prefix'),
    eventSeq: z.number().int().nonnegative().optional(),
    fileCount: z.number().int().nonnegative()
  }).strict().optional(),
  files: z.array(CheckpointRewindApplyFileResultSchema),
  conversation: CheckpointRewindApplyConversationResultSchema,
  summary: z.object({
    fileAppliedCount: z.number().int().nonnegative(),
    fileNoopCount: z.number().int().nonnegative(),
    fileManualReviewCount: z.number().int().nonnegative(),
    fileBlockedCount: z.number().int().nonnegative(),
    fileFailedCount: z.number().int().nonnegative()
  }).strict(),
  auditEventSeq: z.number().int().nonnegative().optional()
}).strict()
export type CheckpointRewindApplyResult = z.infer<typeof CheckpointRewindApplyResultSchema>

export const CheckpointRewindApplyResponseSchema = z.object({
  apply: CheckpointRewindApplyResultSchema
}).strict()
export type CheckpointRewindApplyResponse = z.infer<typeof CheckpointRewindApplyResponseSchema>
