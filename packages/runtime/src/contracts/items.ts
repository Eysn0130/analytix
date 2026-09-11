import { z } from 'zod'
import { ReviewOutputSchema, ReviewTargetSchema } from './review.js'
import { RuntimeErrorSeverity } from './errors.js'

/**
 * Conversation items returned as part of a thread or turn.
 *
 * Items represent normalized public content (text, tool calls, tool
 * results, approvals, and errors). The renderer maps items into chat
 * blocks; the server only persists and replays them.
 */
export const TurnItemRole = z.enum(['user', 'assistant', 'system', 'tool'])
export type TurnItemRole = z.infer<typeof TurnItemRole>

export const TurnItemStatus = z.enum([
  'pending',
  'running',
  'completed',
  'failed',
  'aborted'
])
export type TurnItemStatus = z.infer<typeof TurnItemStatus>

const Sha256HexSchema = z.string().regex(/^[0-9a-f]{64}$/)
const Ed25519PublicKeySchema = z.string().regex(/^[A-Za-z0-9_-]{43}$/)
const Ed25519SignatureSchema = z.string().regex(/^[A-Za-z0-9_-]{86}$/)
const CanonicalUtcRFC3339NanoPattern = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.([0-9]{1,9}))?Z$/
const CanonicalUtcRFC3339NanoSchema = z.string().datetime().regex(
  CanonicalUtcRFC3339NanoPattern
).refine((value) => !/\.[0-9]*0Z$/.test(value), 'UTC timestamp must use canonical RFC3339Nano precision')

function canonicalUtcRFC3339NanoSortKey(value: string): string {
  const match = CanonicalUtcRFC3339NanoPattern.exec(value)
  if (!match) return ''
  return `${match[1]}.${(match[2] ?? '').padEnd(9, '0')}`
}

export const AcceptedFinalDigestSchema = Sha256HexSchema
export type AcceptedFinalDigest = z.infer<typeof AcceptedFinalDigestSchema>

/**
 * Typed ordinary Agent result. This is deliberately non-evidentiary; the Go
 * host binds it to an exact terminal CAS or accepted-final before delivery.
 */
export const OrdinaryResultSlotV1Schema = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal('analytix.ordinary-result/v1'),
  projectionVersion: z.literal('analytix.ordinary-output-projection/v1'),
  logicalEffect: z.literal('ordinary'),
  ordinaryWork: z.literal(true),
  candidateOrigin: z.enum(['provider_ordinary_only', 'host_fixed']),
  evidenceAuthority: z.literal(false),
  citationAuthority: z.literal(false),
  factAnswerAllowed: z.literal(false),
  text: z.string().min(1).refine((value) => value === value.trim(), 'ordinary result text must be exact'),
  textSha256: Sha256HexSchema,
  resultDigest: Sha256HexSchema
}).strict().superRefine((slot, ctx) => {
  const hostFixed = slot.text === '普通任务已执行，但模型结果正文未通过普通输出安全投影，因此未予发布。' ||
    slot.text === '本轮模型输出包含必须经过案件证据门核验的事实候选，宿主已阻止该草稿发布。请在已绑定的案件项目中重新发起核验。'
  if (slot.candidateOrigin === 'host_fixed' && !hostFixed) {
    ctx.addIssue({ code: 'custom', path: ['text'], message: 'ordinary host-fixed result is not canonical' })
  }
})
export type OrdinaryResultSlotV1 = z.infer<typeof OrdinaryResultSlotV1Schema>

export const AcceptedFinalVariantSchema = z.enum([
  'EvidenceBackedAnswer',
  'PartialEvidenceAnswer',
  'VerifiedNoHitAnswer',
  'SourceUnavailableAnswer',
  'NeedsEvidenceAnswer',
  'GeneralGuidanceAnswer'
])
export type AcceptedFinalVariant = z.infer<typeof AcceptedFinalVariantSchema>

export const AcceptedFinalTerminalReasonSchema = z.enum([
  'success',
  'source_unavailable',
  'semantic_failure',
  'provider_failure',
  'cancel',
  'timeout',
  'stream_abort',
  'recovery',
  'approval',
  'user_input',
  'resume',
  'restart',
  'report_fallback',
  'step_limit',
  'background_completion',
  'tool_failure',
  'approval_denied',
  'input_cancelled'
])
export type AcceptedFinalTerminalReason = z.infer<typeof AcceptedFinalTerminalReasonSchema>

export const ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON = {
  success: 'completed',
  source_unavailable: 'completed',
  semantic_failure: 'failed',
  provider_failure: 'failed',
  cancel: 'aborted',
  timeout: 'failed',
  stream_abort: 'failed',
  recovery: 'completed',
  approval: 'completed',
  user_input: 'completed',
  resume: 'completed',
  restart: 'aborted',
  report_fallback: 'failed',
  step_limit: 'failed',
  background_completion: 'completed',
  tool_failure: 'failed',
  approval_denied: 'completed',
  input_cancelled: 'completed'
} as const satisfies Record<AcceptedFinalTerminalReason, 'completed' | 'failed' | 'aborted'>

const ACCEPTED_FINAL_ERROR_ITEM_REASONS = new Set<AcceptedFinalTerminalReason>([
  'semantic_failure',
  'provider_failure',
  'cancel',
  'timeout',
  'stream_abort',
  'restart',
  'report_fallback',
  'step_limit',
  'tool_failure',
  'approval_denied',
  'input_cancelled'
])

/** Canonical current-write accepted-final transport profile. */
export function acceptedFinalDeliveryRequiresErrorItem(
  reason: AcceptedFinalTerminalReason
): boolean {
  return ACCEPTED_FINAL_ERROR_ITEM_REASONS.has(reason)
}

const FACT_BEARING_ACCEPTED_FINAL_VARIANTS = new Set<AcceptedFinalVariant>([
  'EvidenceBackedAnswer',
  'PartialEvidenceAnswer',
  'VerifiedNoHitAnswer'
])

export const AcceptedFinalFactVariantSchema = z.enum([
  'EvidenceBackedAnswer',
  'PartialEvidenceAnswer',
  'VerifiedNoHitAnswer'
])
export type AcceptedFinalFactVariant = z.infer<typeof AcceptedFinalFactVariantSchema>

export const AcceptedFinalBoundaryVariantSchema = z.enum([
  'SourceUnavailableAnswer',
  'NeedsEvidenceAnswer',
  'GeneralGuidanceAnswer'
])
export type AcceptedFinalBoundaryVariant = z.infer<typeof AcceptedFinalBoundaryVariantSchema>

export const AcceptedFinalClaimTypeSchema = z.enum([
  'amount',
  'count',
  'account',
  'entity',
  'direction',
  'date_range',
  'relationship',
  'quote',
  'device_identifier',
  'ownership',
  'control',
  'address',
  'change',
  'bid_certificate',
  'bid_edit_metadata',
  'legal_characterization'
])
export type AcceptedFinalClaimType = z.infer<typeof AcceptedFinalClaimTypeSchema>

export const AcceptedFinalCoverageStatusSchema = z.enum([
  'complete',
  'partial',
  'unavailable',
  'unverified',
  'guidance_only'
])
export type AcceptedFinalCoverageStatus = z.infer<typeof AcceptedFinalCoverageStatusSchema>

const AcceptedFinalPublicCitationV1Schema = z.object({
  handle: z.string().regex(/^cite_[0-9a-f]{64}$/),
  label: z.string().regex(/^evidence-[1-9][0-9]*$/)
}).strict()

const AcceptedFinalPublicReceiptMetadataV1Schema = z.object({
  projection: z.literal('masked_metadata_only'),
  count: z.number().int().nonnegative().safe(),
  setDigest: Sha256HexSchema,
  citations: z.array(AcceptedFinalPublicCitationV1Schema)
}).strict().superRefine((metadata, ctx) => {
  if (metadata.count !== metadata.citations.length) {
    ctx.addIssue({ code: 'custom', path: ['count'], message: 'receipt count must match masked citations' })
  }
  const handles = new Set<string>()
  metadata.citations.forEach((citation, index) => {
    if (citation.label !== `evidence-${index + 1}`) {
      ctx.addIssue({ code: 'custom', path: ['citations', index, 'label'], message: 'citation labels must be canonical' })
    }
    if (handles.has(citation.handle)) {
      ctx.addIssue({ code: 'custom', path: ['citations', index, 'handle'], message: 'citation handles must be unique' })
    }
    handles.add(citation.handle)
  })
})

/**
 * Display-only projection rebuilt by the Go host from the private accepted
 * final. It intentionally contains no claim payload, receipt id, source row,
 * query range, complete PII, provider assertion, or reasoning content.
 */
export const AcceptedFinalPublicViewV1Schema = z.object({
  schemaVersion: z.literal(1),
  publicationState: z.literal('accepted'),
  acceptedFinalDigest: Sha256HexSchema,
  envelopeDigest: Sha256HexSchema,
  contextDigest: Sha256HexSchema,
  contextEpoch: z.number().int().positive().safe(),
  datasetSnapshotId: z.string().min(1),
  variant: AcceptedFinalVariantSchema,
  terminalReason: AcceptedFinalTerminalReasonSchema,
  blockerCode: z.string().max(96).regex(/^(?:[a-z0-9_-]+)?$/),
  coverageStatus: AcceptedFinalCoverageStatusSchema,
  checkedScopeDigest: z.union([z.literal(''), Sha256HexSchema]),
  missingScopeCount: z.number().int().nonnegative().safe(),
  claimCount: z.number().int().nonnegative().safe(),
  claimTypes: z.array(AcceptedFinalClaimTypeSchema),
  receiptMetadata: AcceptedFinalPublicReceiptMetadataV1Schema,
  noHitWording: z.enum(['', 'not_found_in_checked_scope']),
  envelopeIssuedAt: z.string().datetime({ offset: true }),
  acceptedAt: z.string().datetime({ offset: true })
}).strict().superRefine((view, ctx) => {
  const sortedClaimTypes = [...new Set(view.claimTypes)].sort()
  if (sortedClaimTypes.length !== view.claimTypes.length ||
      sortedClaimTypes.some((claimType, index) => claimType !== view.claimTypes[index])) {
    ctx.addIssue({ code: 'custom', path: ['claimTypes'], message: 'claim types must be unique and canonical' })
  }
  if ((view.claimCount === 0) !== (view.claimTypes.length === 0)) {
    ctx.addIssue({ code: 'custom', path: ['claimTypes'], message: 'claim type summary must match the claim count' })
  }

  const receiptCount = view.receiptMetadata.count
  const hasCheckedScope = view.checkedScopeDigest !== ''
  switch (view.variant) {
    case 'EvidenceBackedAnswer':
      if (view.coverageStatus !== 'complete' || view.claimCount === 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'evidence-backed public view shape is invalid' })
      }
      break
    case 'PartialEvidenceAnswer':
      if (view.coverageStatus !== 'partial' || view.claimCount === 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount === 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'partial public view shape is invalid' })
      }
      break
    case 'VerifiedNoHitAnswer':
      if (view.coverageStatus !== 'complete' || view.claimCount !== 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== 'not_found_in_checked_scope') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'verified no-hit public view shape is invalid' })
      }
      break
    case 'SourceUnavailableAnswer':
      if (view.coverageStatus !== 'unavailable' || view.claimCount !== 0 || receiptCount !== 0 ||
          view.blockerCode === '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'source-unavailable public view shape is invalid' })
      }
      break
    case 'NeedsEvidenceAnswer':
      if (view.coverageStatus !== 'unverified' || view.claimCount !== 0 || receiptCount !== 0 ||
          view.missingScopeCount === 0 || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'needs-evidence public view shape is invalid' })
      }
      break
    case 'GeneralGuidanceAnswer':
      if (view.coverageStatus !== 'guidance_only' || view.claimCount !== 0 || receiptCount !== 0 ||
          hasCheckedScope || view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'general-guidance public view shape is invalid' })
      }
      break
  }
})
export type AcceptedFinalPublicViewV1 = z.infer<typeof AcceptedFinalPublicViewV1Schema>

const AcceptedFinalPublicViewFieldsV2Shape = {
  envelopeDigest: Sha256HexSchema,
  contextDigest: Sha256HexSchema,
  contextEpoch: z.number().int().positive().safe(),
  datasetSnapshotId: z.string().min(1),
  variant: AcceptedFinalVariantSchema,
  terminalReason: AcceptedFinalTerminalReasonSchema,
  blockerCode: z.string().max(96).regex(/^(?:[a-z0-9_-]+)?$/),
  coverageStatus: AcceptedFinalCoverageStatusSchema,
  checkedScopeDigest: z.union([z.literal(''), Sha256HexSchema]),
  missingScopeCount: z.number().int().nonnegative().safe(),
  claimCount: z.number().int().nonnegative().safe(),
  claimTypes: z.array(AcceptedFinalClaimTypeSchema),
  receiptMetadata: AcceptedFinalPublicReceiptMetadataV1Schema,
  noHitWording: z.enum(['', 'not_found_in_checked_scope']),
  envelopeIssuedAt: CanonicalUtcRFC3339NanoSchema,
  acceptedAt: CanonicalUtcRFC3339NanoSchema
} as const

type AcceptedFinalPublicViewShapeV2 = {
  variant: AcceptedFinalVariant
  blockerCode: string
  coverageStatus: AcceptedFinalCoverageStatus
  checkedScopeDigest: string
  missingScopeCount: number
  claimCount: number
  claimTypes: AcceptedFinalClaimType[]
  receiptMetadata: z.infer<typeof AcceptedFinalPublicReceiptMetadataV1Schema>
  noHitWording: '' | 'not_found_in_checked_scope'
}

function refineAcceptedFinalPublicViewSemanticShape(
  view: AcceptedFinalPublicViewShapeV2,
  ctx: z.RefinementCtx
): void {
  const sortedClaimTypes = [...new Set(view.claimTypes)].sort()
  if (sortedClaimTypes.length !== view.claimTypes.length ||
      sortedClaimTypes.some((claimType, index) => claimType !== view.claimTypes[index]) ||
      view.claimTypes.length > view.claimCount ||
      ((view.claimCount === 0) !== (view.claimTypes.length === 0))) {
    ctx.addIssue({ code: 'custom', path: ['claimTypes'], message: 'claim type summary must be canonical and match the claim count' })
  }
  const receiptCount = view.receiptMetadata.count
  const hasCheckedScope = view.checkedScopeDigest !== ''
  switch (view.variant) {
    case 'EvidenceBackedAnswer':
      if (view.coverageStatus !== 'complete' || view.claimCount === 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'evidence-backed public view shape is invalid' })
      }
      break
    case 'PartialEvidenceAnswer':
      if (view.coverageStatus !== 'partial' || view.claimCount === 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount === 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'partial public view shape is invalid' })
      }
      break
    case 'VerifiedNoHitAnswer':
      if (view.coverageStatus !== 'complete' || view.claimCount !== 0 || receiptCount === 0 || !hasCheckedScope ||
          view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== 'not_found_in_checked_scope') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'verified no-hit public view shape is invalid' })
      }
      break
    case 'SourceUnavailableAnswer':
      if (view.coverageStatus !== 'unavailable' || view.claimCount !== 0 || receiptCount !== 0 ||
          view.blockerCode === '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'source-unavailable public view shape is invalid' })
      }
      break
    case 'NeedsEvidenceAnswer':
      if (view.coverageStatus !== 'unverified' || view.claimCount !== 0 || receiptCount !== 0 ||
          view.missingScopeCount === 0 || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'needs-evidence public view shape is invalid' })
      }
      break
    case 'GeneralGuidanceAnswer':
      if (view.coverageStatus !== 'guidance_only' || view.claimCount !== 0 || receiptCount !== 0 ||
          hasCheckedScope || view.missingScopeCount !== 0 || view.blockerCode !== '' || view.noHitWording !== '') {
        ctx.addIssue({ code: 'custom', path: ['variant'], message: 'general-guidance public view shape is invalid' })
      }
      break
  }
}

function refineAcceptedFinalPublicViewV2(
  view: AcceptedFinalPublicViewShapeV2 & { envelopeIssuedAt: string; acceptedAt: string },
  ctx: z.RefinementCtx
): void {
  refineAcceptedFinalPublicViewSemanticShape(view, ctx)
  if (canonicalUtcRFC3339NanoSortKey(view.acceptedAt) < canonicalUtcRFC3339NanoSortKey(view.envelopeIssuedAt)) {
    ctx.addIssue({ code: 'custom', path: ['acceptedAt'], message: 'accepted final predates its envelope' })
  }
}

/** PII-free metadata embedded directly in the signed AcceptedFinal V5. */
export const AcceptedFinalPublicViewCoreV2Schema = z.object({
  schemaVersion: z.literal(2),
  publicationState: z.literal('accepted'),
  ...AcceptedFinalPublicViewFieldsV2Shape
}).strict().superRefine(refineAcceptedFinalPublicViewV2)
export type AcceptedFinalPublicViewCoreV2 = z.infer<typeof AcceptedFinalPublicViewCoreV2Schema>

/** Exact private/legacy expansion of the signed V5 core; its V2 wire is frozen. */
export const AcceptedFinalPublicViewV2Schema = z.object({
  schemaVersion: z.literal(2),
  publicationState: z.literal('accepted'),
  acceptedFinalDigest: Sha256HexSchema,
  publicViewDigest: Sha256HexSchema,
  ...AcceptedFinalPublicViewFieldsV2Shape
}).strict().superRefine(refineAcceptedFinalPublicViewV2)
export type AcceptedFinalPublicViewV2 = z.infer<typeof AcceptedFinalPublicViewV2Schema>

/**
 * Closed generic HTTP/SSE projection. V2 remains the exact private/live-record
 * expansion contract; V3 intentionally omits case/context/dataset authority
 * while preserving the accepted PII-free semantic and masked citation fields.
 */
export const AcceptedFinalPublicViewV3Schema = z.object({
  schemaVersion: z.literal(3),
  acceptedFinalDigest: Sha256HexSchema,
  publicationState: z.literal('accepted'),
  variant: AcceptedFinalVariantSchema,
  terminalReason: AcceptedFinalTerminalReasonSchema,
  blockerCode: z.string().max(96).regex(/^(?:[a-z0-9_-]+)?$/),
  coverageStatus: AcceptedFinalCoverageStatusSchema,
  checkedScopeDigest: z.union([z.literal(''), Sha256HexSchema]),
  missingScopeCount: z.number().int().nonnegative().safe(),
  claimCount: z.number().int().nonnegative().safe(),
  claimTypes: z.array(AcceptedFinalClaimTypeSchema),
  receiptMetadata: AcceptedFinalPublicReceiptMetadataV1Schema,
  noHitWording: z.enum(['', 'not_found_in_checked_scope']),
  acceptedAt: CanonicalUtcRFC3339NanoSchema
}).strict().superRefine(refineAcceptedFinalPublicViewSemanticShape)
export type AcceptedFinalPublicViewV3 = z.infer<typeof AcceptedFinalPublicViewV3Schema>

/** Current generic HTTP/SSE/IPC/UI projection. */
export const AcceptedFinalPublicViewSchema = AcceptedFinalPublicViewV3Schema
export type AcceptedFinalPublicView = z.infer<typeof AcceptedFinalPublicViewSchema>

const AcceptedFinalRecordAuthorityShape = {
  authorityPurpose: z.literal('analytix.case-final/v1'),
  authorityAlgorithm: z.literal('Ed25519'),
  authorityKeyId: Sha256HexSchema,
  authorityPublicKey: Ed25519PublicKeySchema,
  threadId: z.string().min(1),
  turnId: z.string().min(1),
  envelopeDigest: Sha256HexSchema,
  contextDigest: Sha256HexSchema,
  contextEpoch: z.number().int().positive().safe(),
  datasetSnapshotId: z.string().min(1),
  variant: AcceptedFinalVariantSchema,
  terminalReason: AcceptedFinalTerminalReasonSchema,
  renderedTextSha256: Sha256HexSchema,
  registrySequence: z.number().int().nonnegative().safe(),
  registryStateDigest: Sha256HexSchema,
  rendererVersion: z.literal('analytix.host-final-renderer/v1')
} as const

const AcceptedFinalRecordAuthorityV5Shape = {
  ...AcceptedFinalRecordAuthorityShape,
  rendererVersion: z.enum([
    'analytix.host-final-renderer/v1',
    'analytix.host-final-renderer/v2'
  ])
} as const

const AcceptedFinalRecordPrivateShape = {
  privateRecordDigest: Sha256HexSchema
} as const

const AcceptedFinalRecordSignatureShape = {
  authoritySignature: Ed25519SignatureSchema,
  recordDigest: AcceptedFinalDigestSchema
} as const

/** Historical V2 boundary parser. Audit and migration only. */
export const AcceptedFinalRecordV2Schema = z.object({
  schemaVersion: z.literal(2),
  ...AcceptedFinalRecordAuthorityShape,
  variant: AcceptedFinalBoundaryVariantSchema,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v1'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: z.string().datetime({ offset: true }),
  ...AcceptedFinalRecordSignatureShape
}).strict()
export type AcceptedFinalRecordV2 = z.infer<typeof AcceptedFinalRecordV2Schema>

/**
 * Historical V3 parser. Fact-bearing V3 records remain structurally readable
 * for audit and migration, but this schema must never be used by a live
 * item/turn/event projection. Live V3 authority is boundary-only below.
 */
export const AcceptedFinalRecordV3Schema = z.object({
  schemaVersion: z.literal(3),
  ...AcceptedFinalRecordAuthorityShape,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v2'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  publicationSnapshotProofDigest: Sha256HexSchema.optional(),
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: z.string().datetime({ offset: true }),
  ...AcceptedFinalRecordSignatureShape
}).strict().superRefine((record, ctx) => {
  const requiresSnapshotProof = FACT_BEARING_ACCEPTED_FINAL_VARIANTS.has(record.variant)
  if (requiresSnapshotProof !== (record.publicationSnapshotProofDigest !== undefined)) {
    ctx.addIssue({
      code: 'custom',
      path: ['publicationSnapshotProofDigest'],
      message: requiresSnapshotProof
        ? 'fact-bearing accepted final requires a publication snapshot proof digest'
        : 'boundary accepted final must not contain a publication snapshot proof digest'
    })
  }
})
export type AcceptedFinalRecordV3 = z.infer<typeof AcceptedFinalRecordV3Schema>

/** Current V3 live authority: deterministic boundary output with no facts. */
export const BoundaryAcceptedFinalRecordV3Schema = z.object({
  schemaVersion: z.literal(3),
  ...AcceptedFinalRecordAuthorityShape,
  variant: AcceptedFinalBoundaryVariantSchema,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v2'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: z.string().datetime({ offset: true }),
  ...AcceptedFinalRecordSignatureShape
}).strict()
export type BoundaryAcceptedFinalRecordV3 = z.infer<typeof BoundaryAcceptedFinalRecordV3Schema>

export const EvidenceAuthorityWitnessBindingV1Schema = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal('analytix.evidence-authority-witness-binding/v1'),
  installationId: Sha256HexSchema,
  enrollmentId: Sha256HexSchema,
  namespace: z.literal('analytix.evidence-registry-authority/v1'),
  authorityKeyId: Sha256HexSchema,
  witnessKeyId: Sha256HexSchema,
  bundleRecordDigest: Sha256HexSchema,
  bundleGeneration: z.number().int().positive().safe(),
  observeRequestDigest: Sha256HexSchema,
  checkpointDigest: Sha256HexSchema,
  observationDigest: Sha256HexSchema,
  bindingDigest: Sha256HexSchema
}).strict()
export type EvidenceAuthorityWitnessBindingV1 = z.infer<typeof EvidenceAuthorityWitnessBindingV1Schema>

const FactFinalWitnessAdmissionPrefixShape = {
  contextDigest: Sha256HexSchema,
  datasetSnapshotId: z.string().min(1),
  sourceManifestHash: Sha256HexSchema,
  envelopeDigest: Sha256HexSchema,
  renderedTextSha256: Sha256HexSchema,
  publicationSnapshotProofDigest: Sha256HexSchema,
  registrySequence: z.number().int().positive().safe(),
  registryStateDigest: Sha256HexSchema,
  evidenceReceiptIdsDigest: Sha256HexSchema,
  evidenceReceiptCount: z.number().int().positive().safe(),
  evidenceAuthorityBundleDigest: Sha256HexSchema,
  evidenceAuthorityBundleGeneration: z.number().int().positive().safe(),
  evidenceRegistryIndexDigest: Sha256HexSchema,
  evidenceRegistryCount: z.number().int().positive().safe(),
  selectedRegistryIndexDigest: Sha256HexSchema,
  selectedRegistryIndexGeneration: z.number().int().positive().safe(),
  selectedRegistryCapsuleDigest: Sha256HexSchema
} as const

const FactFinalWitnessAdmissionSuffixShape = {
  witnessBinding: EvidenceAuthorityWitnessBindingV1Schema,
  admittedAt: CanonicalUtcRFC3339NanoSchema,
  admissionDigest: Sha256HexSchema
} as const

type FactFinalWitnessAdmissionCommon = {
  evidenceAuthorityBundleDigest: string
  evidenceAuthorityBundleGeneration: number
  evidenceRegistryCount: number
  selectedRegistryIndexGeneration: number
  witnessBinding: EvidenceAuthorityWitnessBindingV1
}

function refineFactFinalWitnessAdmission(
  admission: FactFinalWitnessAdmissionCommon,
  ctx: z.RefinementCtx
): void {
  if (admission.witnessBinding.bundleRecordDigest !== admission.evidenceAuthorityBundleDigest) {
    ctx.addIssue({ code: 'custom', path: ['witnessBinding', 'bundleRecordDigest'], message: 'witness bundle digest is mismatched' })
  }
  if (admission.witnessBinding.bundleGeneration !== admission.evidenceAuthorityBundleGeneration) {
    ctx.addIssue({ code: 'custom', path: ['witnessBinding', 'bundleGeneration'], message: 'witness bundle generation is mismatched' })
  }
  if (admission.selectedRegistryIndexGeneration > admission.evidenceRegistryCount) {
    ctx.addIssue({ code: 'custom', path: ['selectedRegistryIndexGeneration'], message: 'selected registry index exceeds witnessed root' })
  }
}

/** Historical compact witness admission. Parseable for exact signed audit only. */
export const FactFinalWitnessAdmissionV1Schema = z.object({
  schemaVersion: z.literal(1),
  purpose: z.literal('analytix.fact-final-witness-admission/v1'),
  ...FactFinalWitnessAdmissionPrefixShape,
  ...FactFinalWitnessAdmissionSuffixShape
}).strict().superRefine(refineFactFinalWitnessAdmission)
export type FactFinalWitnessAdmissionV1 = z.infer<typeof FactFinalWitnessAdmissionV1Schema>

/** Current compact witness admission emitted by the Go Fact-Final issuer. */
export const FactFinalWitnessAdmissionV2Schema = z.object({
  schemaVersion: z.literal(2),
  purpose: z.literal('analytix.fact-final-witness-admission/v2'),
  ...FactFinalWitnessAdmissionPrefixShape,
  datasetSnapshotId: z.string().regex(/^dsv2_[0-9a-f]{64}$/),
  datasetSnapshotIndexDigest: Sha256HexSchema,
  datasetSnapshotCount: z.number().int().positive().safe(),
  selectedDatasetSnapshotIndexDigest: Sha256HexSchema,
  selectedDatasetSnapshotIndexGeneration: z.number().int().positive().safe(),
  selectedDatasetSnapshotRecordDigest: Sha256HexSchema,
  datasetSnapshotManifestDigest: Sha256HexSchema,
  fundsProducerContentId: z.string().regex(/^fpc1_[0-9a-f]{64}$/),
  fundsProducerContentManifestSha256: Sha256HexSchema,
  ...FactFinalWitnessAdmissionSuffixShape
}).strict().superRefine((admission, ctx) => {
  refineFactFinalWitnessAdmission(admission, ctx)
  if (admission.selectedDatasetSnapshotIndexGeneration > admission.datasetSnapshotCount) {
    ctx.addIssue({
      code: 'custom',
      path: ['selectedDatasetSnapshotIndexGeneration'],
      message: 'selected dataset snapshot index exceeds witnessed root'
    })
  }
  if (admission.datasetSnapshotId !== `dsv2_${admission.datasetSnapshotManifestDigest}`) {
    ctx.addIssue({
      code: 'custom',
      path: ['datasetSnapshotId'],
      message: 'dataset snapshot id is detached from its witnessed manifest'
    })
  }
})
export type FactFinalWitnessAdmissionV2 = z.infer<typeof FactFinalWitnessAdmissionV2Schema>

export const FactFinalWitnessAdmissionSchema = z.discriminatedUnion('schemaVersion', [
  FactFinalWitnessAdmissionV1Schema,
  FactFinalWitnessAdmissionV2Schema
])
export type FactFinalWitnessAdmission = z.infer<typeof FactFinalWitnessAdmissionSchema>

/** Current fact-bearing authority issued only after the shared witness gate. */
export const WitnessedFactAcceptedFinalRecordV4Schema = z.object({
  schemaVersion: z.literal(4),
  ...AcceptedFinalRecordAuthorityShape,
  variant: AcceptedFinalFactVariantSchema,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v3'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  publicationSnapshotProofDigest: Sha256HexSchema,
  factFinalWitnessAdmission: FactFinalWitnessAdmissionV1Schema,
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: CanonicalUtcRFC3339NanoSchema,
  ...AcceptedFinalRecordSignatureShape
}).strict().superRefine((record, ctx) => {
  const admission = record.factFinalWitnessAdmission
  const matches: Array<[boolean, string]> = [
    [admission.contextDigest === record.contextDigest, 'contextDigest'],
    [admission.datasetSnapshotId === record.datasetSnapshotId, 'datasetSnapshotId'],
    [admission.envelopeDigest === record.envelopeDigest, 'envelopeDigest'],
    [admission.renderedTextSha256 === record.renderedTextSha256, 'renderedTextSha256'],
    [admission.publicationSnapshotProofDigest === record.publicationSnapshotProofDigest, 'publicationSnapshotProofDigest'],
    [admission.registrySequence === record.registrySequence, 'registrySequence'],
    [admission.registryStateDigest === record.registryStateDigest, 'registryStateDigest'],
    [admission.witnessBinding.authorityKeyId === record.authorityKeyId, 'witnessBinding.authorityKeyId']
  ]
  for (const [matchesRecord, field] of matches) {
    if (!matchesRecord) {
      ctx.addIssue({ code: 'custom', path: ['factFinalWitnessAdmission', field], message: 'fact witness admission is detached from accepted final' })
    }
  }
  if (canonicalUtcRFC3339NanoSortKey(record.acceptedAt) < canonicalUtcRFC3339NanoSortKey(admission.admittedAt)) {
    ctx.addIssue({ code: 'custom', path: ['acceptedAt'], message: 'accepted final predates witnessed evidence admission' })
  }
})
export type WitnessedFactAcceptedFinalRecordV4 = z.infer<typeof WitnessedFactAcceptedFinalRecordV4Schema>

type AcceptedFinalRecordV5PublicBinding = {
  envelopeDigest: string
  contextDigest: string
  contextEpoch: number
  datasetSnapshotId: string
  variant: AcceptedFinalVariant
  terminalReason: AcceptedFinalTerminalReason
  acceptedAt: string
  publicView: AcceptedFinalPublicViewCoreV2
}

function refineAcceptedFinalRecordV5PublicBinding(
  record: AcceptedFinalRecordV5PublicBinding,
  ctx: z.RefinementCtx
): void {
  const view = record.publicView
  const matches: Array<[boolean, string]> = [
    [view.envelopeDigest === record.envelopeDigest, 'envelopeDigest'],
    [view.contextDigest === record.contextDigest, 'contextDigest'],
    [view.contextEpoch === record.contextEpoch, 'contextEpoch'],
    [view.datasetSnapshotId === record.datasetSnapshotId, 'datasetSnapshotId'],
    [view.variant === record.variant, 'variant'],
    [view.terminalReason === record.terminalReason, 'terminalReason'],
    [view.acceptedAt === record.acceptedAt, 'acceptedAt']
  ]
  for (const [matchesRecord, field] of matches) {
    if (!matchesRecord) {
      ctx.addIssue({ code: 'custom', path: ['publicView', field], message: 'signed public view is detached from accepted final' })
    }
  }
}

const BoundaryAcceptedFinalRecordV5BaseSchema = z.object({
  schemaVersion: z.literal(5),
  ...AcceptedFinalRecordAuthorityV5Shape,
  variant: AcceptedFinalBoundaryVariantSchema,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v4'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  publicView: AcceptedFinalPublicViewCoreV2Schema,
  publicViewDigest: Sha256HexSchema,
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: CanonicalUtcRFC3339NanoSchema,
  ...AcceptedFinalRecordSignatureShape
}).strict()

/** Current V5 boundary authority. */
export const BoundaryAcceptedFinalRecordV5Schema = BoundaryAcceptedFinalRecordV5BaseSchema
  .superRefine(refineAcceptedFinalRecordV5PublicBinding)
export type BoundaryAcceptedFinalRecordV5 = z.infer<typeof BoundaryAcceptedFinalRecordV5Schema>

const WitnessedFactAcceptedFinalRecordV5BaseSchema = z.object({
  schemaVersion: z.literal(5),
  ...AcceptedFinalRecordAuthorityV5Shape,
  variant: AcceptedFinalFactVariantSchema,
  finalGateVersion: z.literal('analytix.final-evidence-gate/v4'),
  verifierVersion: z.literal('analytix.claim-verifier-policy/v1'),
  publicView: AcceptedFinalPublicViewCoreV2Schema,
  publicViewDigest: Sha256HexSchema,
  publicationSnapshotProofDigest: Sha256HexSchema,
  factFinalWitnessAdmission: FactFinalWitnessAdmissionSchema,
  ...AcceptedFinalRecordPrivateShape,
  acceptedAt: CanonicalUtcRFC3339NanoSchema,
  ...AcceptedFinalRecordSignatureShape
}).strict()

/** Current V5 fact authority; live admission still requires fresh witness replay. */
export const WitnessedFactAcceptedFinalRecordV5Schema = WitnessedFactAcceptedFinalRecordV5BaseSchema
  .superRefine((record, ctx) => {
    refineAcceptedFinalRecordV5PublicBinding(record, ctx)
    const admission = record.factFinalWitnessAdmission
    const matches: Array<[boolean, string]> = [
      [admission.contextDigest === record.contextDigest, 'contextDigest'],
      [admission.datasetSnapshotId === record.datasetSnapshotId, 'datasetSnapshotId'],
      [admission.envelopeDigest === record.envelopeDigest, 'envelopeDigest'],
      [admission.renderedTextSha256 === record.renderedTextSha256, 'renderedTextSha256'],
      [admission.publicationSnapshotProofDigest === record.publicationSnapshotProofDigest, 'publicationSnapshotProofDigest'],
      [admission.registrySequence === record.registrySequence, 'registrySequence'],
      [admission.registryStateDigest === record.registryStateDigest, 'registryStateDigest'],
      [admission.witnessBinding.authorityKeyId === record.authorityKeyId, 'witnessBinding.authorityKeyId'],
      [admission.evidenceReceiptCount === record.publicView.receiptMetadata.count, 'evidenceReceiptCount']
    ]
    for (const [matchesRecord, field] of matches) {
      if (!matchesRecord) {
        ctx.addIssue({ code: 'custom', path: ['factFinalWitnessAdmission', field], message: 'fact witness admission is detached from V5 authority' })
      }
    }
    if (canonicalUtcRFC3339NanoSortKey(record.acceptedAt) < canonicalUtcRFC3339NanoSortKey(admission.admittedAt)) {
      ctx.addIssue({ code: 'custom', path: ['acceptedAt'], message: 'accepted final predates witnessed evidence admission' })
    }
  })
export type WitnessedFactAcceptedFinalRecordV5 = z.infer<typeof WitnessedFactAcceptedFinalRecordV5Schema>

export const AcceptedFinalRecordV5Schema = z.union([
  BoundaryAcceptedFinalRecordV5Schema,
  WitnessedFactAcceptedFinalRecordV5Schema
])
export type AcceptedFinalRecordV5 = z.infer<typeof AcceptedFinalRecordV5Schema>

/** Forensic parser only. Never use this schema on public/live projections. */
export const AcceptedFinalAuditRecordSchema = z.union([

  AcceptedFinalRecordV2Schema,
  AcceptedFinalRecordV3Schema,
  WitnessedFactAcceptedFinalRecordV4Schema,
  AcceptedFinalRecordV5Schema
])
export type AcceptedFinalAuditRecord = z.infer<typeof AcceptedFinalAuditRecordSchema>

/** Private/CAS and audit-only current record. Never admit this schema on public turn, item, event, or detail paths. */
export const AcceptedFinalPrivateRecordSchema = z.union([
  BoundaryAcceptedFinalRecordV5Schema,
  WitnessedFactAcceptedFinalRecordV5Schema
])
export type AcceptedFinalPrivateRecord = z.infer<typeof AcceptedFinalPrivateRecordSchema>

/** @deprecated Private/audit-only compatibility name; public schemas must use AcceptedFinalPublicViewV3Schema. */
export const AcceptedFinalLiveRecordSchema = AcceptedFinalPrivateRecordSchema
export type AcceptedFinalLiveRecord = AcceptedFinalPrivateRecord

export function acceptedFinalPublicViewMatchesRecord(
  recordValue: unknown,
  viewValue: unknown
): recordValue is AcceptedFinalLiveRecord {
  const record = AcceptedFinalPrivateRecordSchema.safeParse(recordValue)
  const view = AcceptedFinalPublicViewV2Schema.safeParse(viewValue)
  if (!record.success || !view.success) return false
  const expected: AcceptedFinalPublicViewV2 = {
    schemaVersion: 2,
    publicationState: record.data.publicView.publicationState,
    acceptedFinalDigest: record.data.recordDigest,
    publicViewDigest: record.data.publicViewDigest,
    envelopeDigest: record.data.publicView.envelopeDigest,
    contextDigest: record.data.publicView.contextDigest,
    contextEpoch: record.data.publicView.contextEpoch,
    datasetSnapshotId: record.data.publicView.datasetSnapshotId,
    variant: record.data.publicView.variant,
    terminalReason: record.data.publicView.terminalReason,
    blockerCode: record.data.publicView.blockerCode,
    coverageStatus: record.data.publicView.coverageStatus,
    checkedScopeDigest: record.data.publicView.checkedScopeDigest,
    missingScopeCount: record.data.publicView.missingScopeCount,
    claimCount: record.data.publicView.claimCount,
    claimTypes: record.data.publicView.claimTypes,
    receiptMetadata: record.data.publicView.receiptMetadata,
    noHitWording: record.data.publicView.noHitWording,
    envelopeIssuedAt: record.data.publicView.envelopeIssuedAt,
    acceptedAt: record.data.publicView.acceptedAt
  }
  return JSON.stringify(view.data) === JSON.stringify(expected)
}

export function acceptedFinalPublicViewsMatch(leftValue: unknown, rightValue: unknown): boolean {
  const left = AcceptedFinalPublicViewV3Schema.safeParse(leftValue)
  const right = AcceptedFinalPublicViewV3Schema.safeParse(rightValue)
  return left.success && right.success && JSON.stringify(left.data) === JSON.stringify(right.data)
}

export const TurnItemBase = z.object({
  id: z.string().min(1),
  turnId: z.string().min(1),
  threadId: z.string().min(1),
  role: TurnItemRole,
  status: TurnItemStatus,
  createdAt: z.string(),
  finishedAt: z.string().optional()
})

const UserInputOptionSchema = z.object({
  label: z.string().min(1),
  description: z.string()
})

const UserInputQuestionSchema = z.object({
  header: z.string().min(1),
  id: z.string().min(1),
  question: z.string().min(1),
  options: z.array(UserInputOptionSchema)
})

export const UserFileReferenceSchema = z.object({
  path: z.string().min(1),
  relativePath: z.string().min(1),
  name: z.string().min(1),
  kind: z.enum(['file', 'directory']).optional()
})
export type UserFileReference = z.infer<typeof UserFileReferenceSchema>

export const UserTurnItem = TurnItemBase.extend({
  kind: z.literal('user_message'),
  text: z.string(),
  displayText: z.string().optional(),
  delivery: z.literal('steer').optional(),
  clientUserMessageId: z.string().min(1).optional(),
  attachmentIds: z.array(z.string().min(1)).optional(),
  fileReferences: z.array(UserFileReferenceSchema).optional(),
  workspaceCheckpointId: z.string().min(1).optional()
})
export type UserTurnItem = z.infer<typeof UserTurnItem>

export const AssistantTextTurnItem = TurnItemBase.extend({
  kind: z.literal('assistant_text'),
  text: z.string(),
  ordinaryResult: OrdinaryResultSlotV1Schema.optional()
}).strict().superRefine((item, ctx) => {
  if (item.ordinaryResult) {
    if (item.ordinaryResult.text !== item.text) {
      ctx.addIssue({ code: 'custom', path: ['ordinaryResult', 'text'], message: 'ordinary result text is mismatched' })
    }
  }
})
export type AssistantTextTurnItem = z.infer<typeof AssistantTextTurnItem>

/**
 * Closed V3 assistant shape nested only inside a current Batch V2 delivery
 * group or a thread-detail value whose public sink independently verifies
 * that exact group. Parsing this item alone never grants display authority.
 */
export const AcceptedFinalPublicAssistantTextTurnItemV3 = TurnItemBase.extend({
  kind: z.literal('assistant_text'),
  text: z.string(),
  acceptedFinal: z.never().optional(),
  acceptedFinalView: AcceptedFinalPublicViewV3Schema
}).strict().superRefine((item, ctx) => {
  if (item.role !== 'assistant') {
    ctx.addIssue({ code: 'custom', path: ['role'], message: 'accepted final item must be assistant-authored' })
  }
  if (item.status !== 'completed' || item.finishedAt === undefined) {
    ctx.addIssue({ code: 'custom', path: ['status'], message: 'accepted final item must be durably completed' })
  }
  if (item.finishedAt !== undefined && item.finishedAt !== item.acceptedFinalView.acceptedAt) {
    ctx.addIssue({ code: 'custom', path: ['finishedAt'], message: 'accepted final item timestamp is mismatched' })
  }
})
export type AcceptedFinalPublicAssistantTextTurnItemV3 =
  z.infer<typeof AcceptedFinalPublicAssistantTextTurnItemV3>

/** Private/CAS audit item used only by the private accepted-final verifier. */
export const PrivateAcceptedFinalAssistantTextTurnItem = TurnItemBase.extend({
  kind: z.literal('assistant_text'),
  text: z.string(),
  acceptedFinal: AcceptedFinalPrivateRecordSchema,
  acceptedFinalView: AcceptedFinalPublicViewV2Schema
}).strict().superRefine((item, ctx) => {
  if (!acceptedFinalPublicViewMatchesRecord(item.acceptedFinal, item.acceptedFinalView) ||
      item.role !== 'assistant' || item.status !== 'completed' || item.finishedAt === undefined ||
      item.threadId !== item.acceptedFinal.threadId || item.turnId !== item.acceptedFinal.turnId ||
      item.finishedAt !== item.acceptedFinal.acceptedAt) {
    ctx.addIssue({ code: 'custom', path: ['acceptedFinal'], message: 'private accepted-final item authority is invalid' })
  }
})
export type PrivateAcceptedFinalAssistantTextTurnItem = z.infer<typeof PrivateAcceptedFinalAssistantTextTurnItem>

export const PublicToolCallArgumentsProjectionV1 = z.object({
  schemaVersion: z.literal(1),
  projectionKind: z.literal('withheld'),
  disclosure: z.literal('metadata_only'),
  messageKey: z.literal('tool_arguments_withheld'),
  privatePayloadWithheld: z.literal(true),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false)
}).strict()
export type PublicToolCallArgumentsProjectionV1 = z.infer<typeof PublicToolCallArgumentsProjectionV1>

export const ToolCallTurnItem = TurnItemBase.extend({
  kind: z.literal('tool_call'),
  toolName: z.string().min(1),
  callId: z.string().min(1),
  toolKind: z.enum(['tool_call', 'command_execution', 'file_change', 'subagent']),
  arguments: PublicToolCallArgumentsProjectionV1
}).strict()
export type ToolCallTurnItem = z.infer<typeof ToolCallTurnItem>

const PublicToolResultBaseV1 = {
  schemaVersion: z.literal(1),
  disclosure: z.literal('metadata_only'),
  status: z.enum(['completed', 'failed', 'blocked', 'cancelled', 'unknown']),
  privatePayloadWithheld: z.literal(true),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false)
} as const

const PublicToolResultHostCodeV1 = z.enum([
  'approval_cancelled',
  'approval_denied',
  'approval_policy_blocked',
  'cancelled',
  'execution_grant_context_mismatch',
  'execution_grant_expired',
  'execution_grant_invalid',
  'loop_guard',
  'not_found',
  'publication_receipt_required',
  'case_report_publication_receipt_required',
  'runtime_recovered_job_interrupted',
  'sandbox_blocked',
  'side_effect_duplicate',
  'tool_blocked',
  'tool_cancelled',
  'tool_completed',
  'tool_failed',
  'tool_not_advertised',
  'tool_outcome_unknown_after_restart',
  'tool_source_unavailable',
  'tool_timeout',
  'user_input_cancelled',
  'validation_error',
  'workspace_escape'
])

function isSafePublicPlanRelativePath(value: string): boolean {
  const normalized = value.replaceAll('\\', '/')
  if (normalized.startsWith('/')) return false
  let depth = 0
  for (const segment of normalized.split('/')) {
    if (!segment || segment === '.') continue
    if (segment === '..') {
      if (depth === 0) return false
      depth -= 1
      continue
    }
    depth += 1
  }
  return true
}

const PublicToolResultWithheldV1 = z.object({
  ...PublicToolResultBaseV1,
  projectionKind: z.literal('withheld'),
  messageKey: z.enum(['tool_output_withheld', 'legacy_output_withheld']),
  code: z.enum(['tool_output_private', 'legacy_output_withheld', 'tool_projection_invalid']).optional()
}).strict()

const PublicToolResultHostStatusV1 = z.object({
  ...PublicToolResultBaseV1,
  projectionKind: z.literal('host_status'),
  messageKey: z.enum(['tool_completed', 'tool_failed', 'tool_cancelled', 'tool_blocked', 'tool_outcome_unknown']),
  code: PublicToolResultHostCodeV1.optional()
}).strict()

const PublicToolResultPlanStatusV1 = z.object({
  ...PublicToolResultBaseV1,
  projectionKind: z.literal('plan_status'),
  messageKey: z.enum(['plan_updated', 'plan_failed']),
  code: z.literal('plan_updated'),
  plan: z.object({
    planId: z.string().min(1).max(256),
    relativePath: z.string().min(1).max(1024).refine(isSafePublicPlanRelativePath),
    operation: z.enum(['draft', 'refine']),
    contentHash: z.string().regex(/^[a-f0-9]{64}$/),
    byteSize: z.number().int().nonnegative().safe(),
    savedAt: z.string().datetime({ offset: true })
  }).strict()
}).strict()

const PublicToolResultCaseSourceStatusV1 = z.object({
  ...PublicToolResultBaseV1,
  projectionKind: z.literal('case_source_status'),
  messageKey: z.enum(['case_source_private', 'case_source_failed']),
  code: z.literal('case_source_result_private')
}).strict()

const PublicToolResultMcpDiagnosticV1 = z.object({
  ...PublicToolResultBaseV1,
  projectionKind: z.literal('mcp_diagnostic'),
  messageKey: z.literal('mcp_request_rejected'),
  code: z.literal('mcp_request_rejected'),
  rpcError: z.object({
    code: z.number().int().safe(),
    class: z.enum(['method_not_found', 'invalid_params', 'internal_error', 'parse_error', 'invalid_request', 'server_error']),
    dataPresent: z.boolean()
  }).strict().superRefine((value, ctx) => {
    const canonicalCode = {
      parse_error: -32700,
      invalid_request: -32600,
      method_not_found: -32601,
      invalid_params: -32602,
      internal_error: -32603,
      server_error: -32000
    }[value.class]
    if (value.code !== canonicalCode) {
      ctx.addIssue({ code: 'custom', path: ['code'], message: 'rpc error code does not match its host-normalized class' })
    }
  })
}).strict()

const PublicToolResultProjectionShapeV1 = z.discriminatedUnion('projectionKind', [
  PublicToolResultWithheldV1,
  PublicToolResultHostStatusV1,
  PublicToolResultPlanStatusV1,
  PublicToolResultCaseSourceStatusV1,
  PublicToolResultMcpDiagnosticV1
])
export const PublicToolResultProjectionV1 = PublicToolResultProjectionShapeV1.superRefine((projection, ctx) => {
  if (projection.projectionKind !== 'host_status') return
  const unknownStatus = projection.status === 'unknown'
  const unknownMessage = projection.messageKey === 'tool_outcome_unknown'
  const unknownCode = projection.code === 'tool_outcome_unknown_after_restart'
  if ((unknownStatus || unknownMessage || unknownCode) && !(unknownStatus && unknownMessage && unknownCode)) {
    ctx.addIssue({ code: 'custom', message: 'host outcome-unknown projection is not canonically paired' })
  }
})
export type PublicToolResultProjectionV1 = z.infer<typeof PublicToolResultProjectionV1>

export const ToolResultTurnItem = TurnItemBase.extend({
  kind: z.literal('tool_result'),
  toolName: z.string().min(1),
  callId: z.string().min(1),
  toolKind: z.enum(['tool_call', 'command_execution', 'file_change', 'subagent']),
  output: PublicToolResultProjectionV1,
  isError: z.boolean().default(false)
}).strict().superRefine((item, ctx) => {
  const pending = item.status === 'pending' || item.status === 'running'
  if (pending ? item.isError : item.isError ? item.status !== 'failed' && item.status !== 'aborted' : item.status !== 'completed') {
    ctx.addIssue({ code: 'custom', path: ['status'], message: 'tool result lifecycle status does not match isError' })
  }
  if (pending && item.output.status !== 'unknown') {
    ctx.addIssue({ code: 'custom', path: ['output', 'status'], message: 'pending tool result must remain unknown' })
  }
  if (item.output.status !== 'unknown' && (item.output.status === 'completed') === item.isError) {
    ctx.addIssue({ code: 'custom', path: ['output', 'status'], message: 'public projection status does not match isError' })
  }
  if (item.output.projectionKind === 'mcp_diagnostic' && !item.isError) {
    ctx.addIssue({ code: 'custom', path: ['isError'], message: 'MCP rejection must be an error result' })
  }
})
export type ToolResultTurnItem = z.infer<typeof ToolResultTurnItem>

// Public child lifecycle metadata, shared by HTTP hydration and strict SSE.
// Background ledger identities are not provider tool calls; their private
// callId is deliberately removed by the Go public projection.
const PublicChildIdentityV1 = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/)
const PublicChildHostCallIdV1 = z.string().regex(/^call_host_[a-f0-9]{64}$/)
const PublicChildTimestampV1 = z.string().max(64).datetime({ offset: true })
const PublicChildJobStatusV1 = z.enum([
  'queued', 'running', 'pause_requested', 'paused', 'resume_requested', 'resuming',
  'completed', 'failed', 'aborted', 'killed', 'interrupted', 'canceled', 'timeout', 'unknown'
])
const PublicChildDeliveryStatusV1 = z.enum(['pending', 'retry', 'delivered', 'skipped', 'dead_letter', 'unknown'])
const PublicChildAutoContinueStatusV1 = z.enum(['starting', 'started', 'skipped', 'failed', 'unknown'])
const PUBLIC_CHILD_TERMINAL_STATUSES_V1 = new Set([
  'completed', 'failed', 'aborted', 'killed', 'interrupted', 'canceled', 'timeout'
])
const PublicChildProgressDiagnosticsV1 = z.object({
  kind: z.literal('subagent_task'),
  id: PublicChildIdentityV1,
  jobId: PublicChildIdentityV1.optional(),
  childId: PublicChildIdentityV1.optional(),
  childRunId: PublicChildIdentityV1.optional(),
  childThreadId: PublicChildIdentityV1.optional(),
  childTurnId: PublicChildIdentityV1.optional(),
  parentThreadId: PublicChildIdentityV1.optional(),
  parentTurnId: PublicChildIdentityV1.optional(),
  parentToolCallId: PublicChildHostCallIdV1.optional(),
  notificationKind: z.enum([
    'background_job_completion', 'background_job_auto_continue', 'background_job_delivery',
    'thread_summary_subagent', 'thread_summary_task'
  ]).optional(),
  status: PublicChildJobStatusV1,
  childStatus: PublicChildJobStatusV1,
  terminal: z.boolean(),
  background: z.boolean().optional(),
  lateCompletionSuppressed: z.boolean().optional(),
  deliveryStatus: PublicChildDeliveryStatusV1.optional(),
  autoContinueStatus: PublicChildAutoContinueStatusV1.optional(),
  outputWithheld: z.literal(true),
  outputTrustStatus: z.literal('untrusted_child_output'),
  factAnswerAllowed: z.literal(false),
  evidenceAuthority: z.literal(false),
  canReadOutput: z.literal(false),
  canContinueParent: z.literal(false)
}).strict().superRefine((child, ctx) => {
  if (![child.jobId, child.childId, child.childRunId].includes(child.id) ||
      child.childStatus !== child.status ||
      child.terminal !== PUBLIC_CHILD_TERMINAL_STATUSES_V1.has(child.status)) {
    ctx.addIssue({ code: 'custom', message: 'child progress identity or lifecycle is detached' })
  }
})

export const ToolProgressTurnItem = TurnItemBase.extend({
  kind: z.literal('tool_progress'),
  id: PublicChildIdentityV1,
  threadId: PublicChildIdentityV1,
  turnId: PublicChildIdentityV1,
  role: z.literal('tool'),
  status: z.enum(['running', 'completed', 'failed', 'unknown']),
  createdAt: PublicChildTimestampV1,
  finishedAt: PublicChildTimestampV1.optional(),
  toolName: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/),
  callId: PublicChildHostCallIdV1.optional(),
  summary: z.literal('child output withheld'),
  message: z.literal('child output withheld'),
  arguments: z.object({
    runtimeStatus: z.literal('tool_progress'),
    stage: z.string().max(96),
    status: z.string().max(32),
    diagnostics: PublicChildProgressDiagnosticsV1
  }).strict()
}).strict().superRefine((item, ctx) => {
  const { stage, status, diagnostics: child } = item.arguments
  let validStage = false
  let ledger = false
  if (stage.startsWith('background_job_delivery_')) {
    validStage = PublicChildDeliveryStatusV1.safeParse(status).success &&
      stage === `background_job_delivery_${status}` && child.deliveryStatus === status
    ledger = item.toolName === 'background_delivery'
  } else if (stage.startsWith('background_job_auto_continue_')) {
    validStage = PublicChildAutoContinueStatusV1.safeParse(status).success &&
      stage === `background_job_auto_continue_${status}` && child.autoContinueStatus === status
    ledger = item.toolName === 'background_auto_continue'
  } else if (stage.startsWith('background_job_')) {
    validStage = PublicChildJobStatusV1.safeParse(status).success &&
      stage === `background_job_${status}` && child.status === status &&
      (status === 'unknown' || PUBLIC_CHILD_TERMINAL_STATUSES_V1.has(status))
    ledger = child.notificationKind === 'background_job_completion'
  } else if (stage.startsWith('subagent_')) {
    validStage = PublicChildJobStatusV1.safeParse(status).success &&
      stage === `subagent_${status}` && child.status === status
  }
  if (!validStage || (!item.callId && !ledger)) {
    ctx.addIssue({ code: 'custom', path: ['arguments'], message: 'child progress stage or call identity is invalid' })
  }
  if ((child.parentThreadId !== undefined && child.parentThreadId !== item.threadId) ||
      (child.parentTurnId !== undefined && child.parentTurnId !== item.turnId)) {
    ctx.addIssue({ code: 'custom', path: ['arguments', 'diagnostics'], message: 'child progress parent binding is detached' })
  }
  if ((stage.startsWith('background_job_delivery_') && item.status !== (status === 'dead_letter' ? 'failed' : 'completed')) ||
      (stage.startsWith('background_job_auto_continue_') && item.status !== 'completed')) {
    ctx.addIssue({ code: 'custom', path: ['status'], message: 'child ledger item lifecycle is inconsistent' })
  }
  if (item.finishedAt !== undefined && Date.parse(item.finishedAt) < Date.parse(item.createdAt)) {
    ctx.addIssue({ code: 'custom', path: ['finishedAt'], message: 'child progress completion precedes creation' })
  }
  if ((item.status === 'completed' || item.status === 'failed') && item.finishedAt === undefined) {
    ctx.addIssue({ code: 'custom', path: ['finishedAt'], message: 'terminal child progress requires its completion time' })
  }
})
export type ToolProgressTurnItem = z.infer<typeof ToolProgressTurnItem>

export const ApprovalTurnItem = TurnItemBase.extend({
  kind: z.literal('approval'),
  approvalId: z.string().min(1),
  toolName: z.string().min(1),
  summary: z.string(),
  status: z.enum(['pending', 'allowed', 'denied', 'expired'])
})
export type ApprovalTurnItem = z.infer<typeof ApprovalTurnItem>

export const UserInputTurnItem = TurnItemBase.extend({
  kind: z.literal('user_input'),
  inputId: z.string().min(1),
  prompt: z.string(),
  questions: z.array(UserInputQuestionSchema).default([]),
  status: z.enum(['pending', 'submitted', 'cancelled'])
})
export type UserInputTurnItem = z.infer<typeof UserInputTurnItem>

const TaskContinuationTodoV1Schema = z.object({
  todoId: z.string().min(1),
  content: z.string().min(1),
  status: z.enum(['pending', 'in_progress', 'failed', 'canceled']),
  statusReasonCode: z.string().min(1).optional(),
  stateDigest: z.string().regex(/^[a-f0-9]{64}$/)
}).strict().superRefine((todo, ctx) => {
  const terminal = todo.status === 'failed' || todo.status === 'canceled'
  if (terminal !== (todo.statusReasonCode !== undefined)) {
    ctx.addIssue({
      code: 'custom',
      path: ['statusReasonCode'],
      message: terminal
        ? 'failed or canceled continuation todo requires a reason code'
        : 'non-terminal continuation todo prohibits a reason code'
    })
  }
})

export const TaskContinuationSnapshotV1Schema = z.object({
  schemaVersion: z.literal(1),
  goal: z.object({
    goalId: z.string().min(1),
    objective: z.string().min(1),
    status: z.enum(['active', 'paused', 'blocked', 'usageLimited', 'budgetLimited']),
    stateDigest: z.string().regex(/^[a-f0-9]{64}$/)
  }).strict().optional(),
  todos: z.array(TaskContinuationTodoV1Schema).max(200),
  latestUserConstraints: z.array(z.string().min(1)).max(4),
  evidenceReferences: z.array(z.object({
    referenceDigest: z.string().regex(/^[a-f0-9]{64}$/),
    supportStatus: z.literal('unverified_for_case_facts')
  }).strict()).max(32),
  evidenceAuthority: z.literal('unverified_for_case_facts'),
  previousContinuationDigest: z.string().regex(/^[a-f0-9]{64}$/).optional(),
  previousCompactionSourceDigest: z.string().regex(/^[a-f0-9]{64}$/).optional(),
  stateDigest: z.string().regex(/^[a-f0-9]{64}$/)
}).strict().superRefine((snapshot, ctx) => {
  const todoIds = new Set<string>()
  snapshot.todos.forEach((todo, index) => {
    if (todoIds.has(todo.todoId)) {
      ctx.addIssue({ code: 'custom', path: ['todos', index, 'todoId'], message: 'continuation todo ids must be unique' })
    }
    todoIds.add(todo.todoId)
  })
  const evidenceDigests = new Set<string>()
  snapshot.evidenceReferences.forEach((reference, index) => {
    if (evidenceDigests.has(reference.referenceDigest)) {
      ctx.addIssue({ code: 'custom', path: ['evidenceReferences', index, 'referenceDigest'], message: 'continuation evidence references must be unique' })
    }
    evidenceDigests.add(reference.referenceDigest)
  })
})
export type TaskContinuationSnapshotV1 = z.infer<typeof TaskContinuationSnapshotV1Schema>

const CaseCompactionPublicSummary =
  'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'

export const CompactionTurnItem = TurnItemBase.extend({
  kind: z.literal('compaction'),
  summary: z.string(),
  replacedTokens: z.number().int().nonnegative().optional(),
  auto: z.boolean().optional(),
  pinnedConstraints: z.array(z.string()),
  sourceDigest: z.string().min(1).optional(),
  digestMarker: z.string().min(1).optional(),
  sourceItemIds: z.array(z.string().min(1)).optional(),
  schemaVersion: z.union([z.literal(2), z.literal(3), z.literal(4)]).optional(),
  reasoningExcluded: z.literal(true).optional(),
  reasoningExclusionProof: z.string().regex(/^sha256:[a-f0-9]{64}$/).optional(),
  assistantProseExcluded: z.literal(true).optional(),
  toolPayloadsExcluded: z.literal(true).optional(),
  caseFactsExcluded: z.literal(true).optional(),
  providerHistoryProjectionVersion: z.union([z.literal(1), z.literal(2)]).optional(),
  caseHistoryProjectionVersion: z.union([z.literal(1), z.literal(2)]).optional(),
  taskContinuation: TaskContinuationSnapshotV1Schema.optional()
}).strict().superRefine((item, ctx) => {
  const automaticV4 = item.schemaVersion === 4
  const publicCaseV2 = item.caseHistoryProjectionVersion === 2 || item.summary === CaseCompactionPublicSummary
  if (publicCaseV2) {
    const sourceDigest = item.sourceDigest ?? ''
    if (item.schemaVersion !== 3 || item.summary !== CaseCompactionPublicSummary ||
        item.caseHistoryProjectionVersion !== 2 ||
        item.providerHistoryProjectionVersion !== undefined || item.taskContinuation !== undefined ||
        item.auto === undefined || item.replacedTokens !== undefined || item.finishedAt !== item.createdAt ||
        item.pinnedConstraints.length !== 1 || item.pinnedConstraints[0] !== 'user: preserve recent turns' ||
        item.sourceItemIds === undefined || item.sourceItemIds.length !== 0 ||
        !/^[a-f0-9]{64}$/.test(sourceDigest) || item.digestMarker !== `sha256:${sourceDigest.slice(0, 12)}` ||
        item.reasoningExcluded !== true || item.reasoningExclusionProof === undefined ||
        item.assistantProseExcluded !== true ||
        item.toolPayloadsExcluded !== true || item.caseFactsExcluded !== true) {
      ctx.addIssue({
        code: 'custom',
        path: ['caseHistoryProjectionVersion'],
        message: 'case compaction V2 must be the closed public marker'
      })
    }
  }
  if (!publicCaseV2 && item.replacedTokens === undefined) {
    ctx.addIssue({ code: 'custom', path: ['replacedTokens'], message: 'ordinary compaction requires its token count' })
  }
  if (automaticV4) {
    if (item.auto !== true || item.providerHistoryProjectionVersion !== 2 || !item.taskContinuation) {
      ctx.addIssue({ code: 'custom', path: ['taskContinuation'], message: 'automatic V4 compaction requires its closed continuation' })
    }
  } else if (item.taskContinuation !== undefined) {
    ctx.addIssue({ code: 'custom', path: ['taskContinuation'], message: 'legacy or manual compaction cannot carry task continuation authority' })
  }
})
export type CompactionTurnItem = z.infer<typeof CompactionTurnItem>

export const ReviewTurnItem = TurnItemBase.extend({
  kind: z.literal('review'),
  target: ReviewTargetSchema,
  title: z.string().min(1),
  reviewText: z.string().optional(),
  output: ReviewOutputSchema.optional()
})
export type ReviewTurnItem = z.infer<typeof ReviewTurnItem>

export const ErrorTurnItem = TurnItemBase.extend({
  kind: z.literal('error'),
  message: z.string(),
  code: z.string().optional(),
  details: z.unknown().optional(),
  severity: RuntimeErrorSeverity.optional()
})
export type ErrorTurnItem = z.infer<typeof ErrorTurnItem>

export const TurnItem = z.discriminatedUnion('kind', [
  UserTurnItem,
  AssistantTextTurnItem,
  ToolCallTurnItem,
  ToolResultTurnItem,
  ToolProgressTurnItem,
  ApprovalTurnItem,
  UserInputTurnItem,
  CompactionTurnItem,
  ReviewTurnItem,
  ErrorTurnItem
])
export type TurnItem = z.infer<typeof TurnItem>

export type TurnItemKind = TurnItem['kind']
