import { z } from 'zod'

// Protected-local display only: never a public event, model/history value or
// evidence authority. Renderer requests contain Main-local handles, never the
// private Go viewer token, principal, source paths or caller-provided bodies.
export const PROCESS_LOCAL_DISPLAY_READ_PATH_V1 = '/v1/local-display/process/read'
export const PROCESS_LOCAL_DISPLAY_TARGET_PATH_V1 = '/v1/local-display/process/target'
export const PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1 = 256 << 10
export const PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1 = 128
export const PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1 = 32 << 10
export const PROCESS_LOCAL_DISPLAY_MAX_WAIT_MS_V1 = 25_000
export const PROCESS_LOCAL_DISPLAY_MAX_WEB_URL_BYTES_V1 = 8192

const recordId = z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/).refine(value => !value.includes('..'))
const uuid = z.string().uuid()
const digest = z.string().regex(/^[a-f0-9]{64}$/)
const count = z.number().int().safe().nonnegative()
export const PROCESS_LOCAL_DISPLAY_SCHEMA_VERSION_V2 = 2 as const
const revision = z.string().regex(/^(?:0|[1-9][0-9]{0,19})$/).refine(value => /^[0-9]{1,20}$/.test(value) && BigInt(value) <= 18446744073709551615n)
const text = (bytes: number) => z.string().refine(value =>
  !value.includes('\0') && !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= bytes)
const webURL = text(PROCESS_LOCAL_DISPLAY_MAX_WEB_URL_BYTES_V1).refine(value => {
  if (value.trim() !== value || /\p{Cc}/u.test(value)) return false
  try { const url = new URL(value); return ['http:', 'https:'].includes(url.protocol) && !!url.hostname && !url.username && !url.password }
  catch { return false }
})
const blockIds = z.array(recordId).max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1).refine(ids => new Set(ids).size === ids.length)

export const processDisplayScopeSchemaV1 = z.object({
  threadId: recordId, turnId: recordId, contextDigest: digest,
  contextEpoch: revision, processIncarnation: recordId
}).strict()
export const processDisplayBodySchemaV1 = z.object({
  text: text(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1),
  before: text(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1),
  after: text(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1),
  error: text(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1),
  addedLines: count, removedLines: count, approximate: z.boolean()
}).strict()
// Coordinates from separate producers are deliberately not comparable.
const callAnchor = z.object({
  callId: recordId, toolCallItemId: recordId,
  toolResultItemId: recordId.optional(), startedEventSeq: revision.optional()
}).strict()
const anchors = z.array(callAnchor).max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1)
  .refine(values => new Set(values.map(value => value.callId)).size === values.length)
export const processDisplayUnitSchemaV2 = z.object({
  unitId: recordId, producerId: recordId, providerSequence: revision,
  placement: z.enum(['anchored', 'pending', 'unit_only']),
  calls: anchors, precedingCalls: anchors
}).strict().refine(unit => unit.placement === 'anchored'
  ? unit.calls.length + unit.precedingCalls.length > 0
  : unit.calls.length + unit.precedingCalls.length === 0)
export const processDisplaySourceSchemaV2 = z.discriminatedUnion('kind', [
  z.object({ kind: z.literal('native'), unitId: recordId, producerId: recordId,
    providerSequence: revision, attempt: count.positive(), nativeIndex: count,
    sourceOrdinal: revision, reasoningForm: z.literal('summary').optional() }).strict(),
  z.object({ kind: z.literal('provider_reasoning'), unitId: recordId, producerId: recordId,
    providerSequence: revision, attempt: count.positive(), sourceOrdinal: revision,
    reasoningForm: z.literal('summary').optional() }).strict(),
  z.object({ kind: z.literal('tool_preparation'), unitId: recordId, producerId: recordId,
    providerSequence: revision, attempt: count.positive(), callId: recordId,
    sourceOrdinal: revision.refine(value => value !== '0') }).strict(),
  z.object({ kind: z.literal('assistant_draft'), unitId: recordId, producerId: recordId,
    providerSequence: revision, attempt: count.positive() }).strict(),
  z.object({ kind: z.literal('tool'), unitId: recordId, callId: recordId,
    toolCallItemId: recordId, toolResultItemId: recordId.optional(),
    startedEventSeq: revision.optional() }).strict()
])
const toolInput = z.discriminatedUnion('availability', [
  z.object({ availability: z.literal('available'), inputJSON: text(64 << 10) }).strict(),
  z.object({ availability: z.literal('unavailable'), reason: z.enum(['missing', 'limit', 'canceled']) }).strict()
])
const presence = z.object({ nativeBlockCount: count.max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1),
  toolCaptureCount: count.max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1),
  assistantDraftCount: count.max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1).optional() }).strict()
const draftSegment = z.object({ offsetBytes: revision,
  text: text(PROCESS_LOCAL_DISPLAY_MAX_BODY_BYTES_V1).refine(value => value.length > 0) }).strict()
const units = z.array(processDisplayUnitSchemaV2).max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1)
  .refine(values => new Set(values.map(value => value.unitId)).size === values.length &&
    values.reduce((total, value) => total + value.calls.length + value.precedingCalls.length, 0) <= PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1)
export const processDisplayBlockSchemaV1 = z.object({
  blockId: recordId, source: processDisplaySourceSchemaV2,
  kind: z.enum(['native_reasoning', 'shell', 'write', 'edit', 'assistant_draft', 'web_fetch', 'tool_preparation', 'tool_input']),
  state: z.enum(['preparing', 'running', 'completed', 'failed', 'timeout', 'canceled', 'unknown', 'superseded', 'withheld']),
  attempt: count, nativeIndex: count.optional(),
  toolName: text(1024).refine(value => value.trim().length > 0).optional(),
  title: text(1024).optional(), preview: text(1024).optional(), path: text(4096).optional(),
  captureDigest: digest.optional(), durationMs: count, exitCode: count.optional(),
  requestUrl: webURL.optional(), inputDigest: digest.optional(),
  body: processDisplayBodySchemaV1.optional(), input: toolInput.optional(),
  contentClassification: z.enum(['unclassified_inline', 'provider_text']).optional(),
  draftSegments: z.array(draftSegment).min(1).max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1).optional(),
  draftBytes: revision.optional(),
  // Exact decoded UTF-16 code units of the actual Write/Edit input, not bytes.
  inputUtf16CodeUnits: count.optional(),
  effectApplied: z.boolean().nullable().optional(),
  settlementStatus: z.enum(['completed', 'failed', 'quarantined', 'not_required', 'no_effect', 'aborted', 'unknown']).optional(),
  truncated: z.boolean(),
  classification: z.enum(['source_exact_local', 'credential_redacted', 'withheld'])
}).strict().superRefine((block, context) => {
  if (block.input !== undefined && (block.source.kind !== 'tool' || block.state !== 'running' ||
      !['tool_input', 'shell', 'web_fetch'].includes(block.kind) || block.source.toolResultItemId !== undefined || block.source.startedEventSeq === undefined ||
      block.input.availability === 'available' && block.classification === 'withheld')) {
    context.addIssue({ code: 'custom', message: 'Invalid protected complete tool input.' })
  }
  if (block.kind === 'tool_input') {
    const active = block.state === 'running' && block.source.kind === 'tool' && block.source.toolResultItemId === undefined && block.source.startedEventSeq !== undefined
    const retired = ['unknown', 'withheld'].includes(block.state) && block.classification === 'withheld' && block.input === undefined
    if ((!active && !retired) || block.source.kind !== 'tool' || block.attempt !== 0 || block.truncated ||
        block.body !== undefined || block.title !== undefined || block.preview !== undefined || block.path !== undefined || block.toolName !== undefined ||
        block.captureDigest !== undefined || block.requestUrl !== undefined || block.inputDigest !== undefined || block.nativeIndex !== undefined ||
        block.inputUtf16CodeUnits !== undefined || block.contentClassification !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined ||
        block.exitCode !== undefined || block.effectApplied !== undefined || block.settlementStatus !== undefined) {
      context.addIssue({ code: 'custom', message: 'Invalid protected running tool source.' })
    }
    return
  }
  if (block.kind === 'web_fetch') {
    const forbidden = block.source.kind !== 'tool' || block.attempt !== 0 || block.truncated ||
      block.toolName !== undefined || block.title !== undefined || block.preview !== undefined || block.path !== undefined || block.body !== undefined ||
      block.captureDigest !== undefined || block.nativeIndex !== undefined || block.inputUtf16CodeUnits !== undefined ||
      block.contentClassification !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined ||
      block.exitCode !== undefined || block.effectApplied !== undefined || block.settlementStatus !== undefined
    const active = block.state === 'running' &&
      block.source.kind === 'tool' && block.source.toolResultItemId === undefined && block.source.startedEventSeq !== undefined &&
      ((block.classification === 'source_exact_local' && block.requestUrl !== undefined && block.inputDigest !== undefined) ||
        block.requestUrl === undefined && block.inputDigest === undefined)
    const retired = ['unknown', 'withheld'].includes(block.state) && block.classification === 'withheld' &&
      block.requestUrl === undefined && block.inputDigest === undefined
    if (forbidden || !active && !retired) context.addIssue({ code: 'custom', message: 'Invalid protected Web input.' })
    return
  }
  if (block.requestUrl !== undefined || block.inputDigest !== undefined) {
    context.addIssue({ code: 'custom', message: 'Web input fields require an active Web source.' })
  }
  if (block.kind === 'tool_preparation' || block.source.kind === 'tool_preparation') {
    const active = block.state === 'preparing' && block.classification !== 'withheld'
    const privateFields = block.toolName !== undefined || block.preview !== undefined || block.path !== undefined || block.inputUtf16CodeUnits !== undefined
    const invalid = block.kind !== 'tool_preparation' || block.source.kind !== 'tool_preparation' ||
      block.source.kind === 'tool_preparation' && block.source.attempt !== block.attempt ||
      !['preparing', 'unknown', 'superseded', 'withheld'].includes(block.state) || block.truncated ||
      active && block.toolName === undefined || !active && (block.classification !== 'withheld' || privateFields) ||
      block.title !== undefined || block.body !== undefined || block.captureDigest !== undefined || block.nativeIndex !== undefined ||
      block.contentClassification !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined ||
      block.exitCode !== undefined || block.effectApplied !== undefined || block.settlementStatus !== undefined ||
      block.inputUtf16CodeUnits !== undefined && !['write', 'write_file', 'edit', 'edit_file', 'multi_edit'].includes(block.toolName ?? '')
    if (invalid) context.addIssue({ code: 'custom', message: 'Invalid protected tool preparation.' })
    return
  }
  if (block.toolName !== undefined || block.state === 'preparing') {
    context.addIssue({ code: 'custom', message: 'Preparation fields require a tool preparation source.' })
  }
  if (block.kind === 'assistant_draft' || block.source.kind === 'assistant_draft') {
    let invalid = block.kind !== 'assistant_draft' || block.source.kind !== 'assistant_draft' ||
      block.source.kind === 'assistant_draft' && block.source.attempt !== block.attempt ||
      block.contentClassification === undefined || !['running', 'unknown', 'superseded'].includes(block.state) || block.truncated ||
      block.state !== 'running' && block.classification !== 'withheld' ||
      block.toolName !== undefined || block.title !== undefined || block.preview !== undefined || block.path !== undefined || block.body !== undefined ||
      block.captureDigest !== undefined || block.nativeIndex !== undefined || block.inputUtf16CodeUnits !== undefined ||
      block.exitCode !== undefined || block.effectApplied !== undefined || block.settlementStatus !== undefined ||
      (block.draftSegments === undefined) !== (block.draftBytes === undefined)
    if (block.draftSegments !== undefined) {
      let bytes = 0n
      for (const segment of block.draftSegments) {
        if (BigInt(segment.offsetBytes) !== bytes) invalid = true
        bytes += BigInt(new TextEncoder().encode(segment.text).length)
      }
      if (block.state !== 'running' || block.classification === 'withheld' ||
        bytes > BigInt(PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1) || bytes !== BigInt(block.draftBytes ?? '0')) invalid = true
    }
    if (invalid) context.addIssue({ code: 'custom', message: 'Invalid protected assistant draft.' })
    return
  }
  if (block.contentClassification !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined) {
    context.addIssue({ code: 'custom', message: 'Draft fields require an assistant draft source.' })
  }
  const reasoningSource = block.source.kind === 'native' || block.source.kind === 'provider_reasoning'
  const runningReasoning = block.kind === 'native_reasoning' && reasoningSource && block.state === 'running'
  const runningShell = block.kind === 'shell' && block.source.kind === 'tool' && block.state === 'running'
  const privateFields = block.title !== undefined || block.preview !== undefined || block.path !== undefined || block.body !== undefined || block.inputUtf16CodeUnits !== undefined
  if ((block.kind === 'native_reasoning') !== reasoningSource ||
      (block.source.kind === 'native' || block.source.kind === 'provider_reasoning') && block.source.attempt !== block.attempt ||
      block.source.kind === 'native' && block.source.nativeIndex !== block.nativeIndex ||
      block.source.kind === 'provider_reasoning' && block.nativeIndex !== undefined ||
      block.source.kind === 'tool' && block.classification !== 'withheld' && !runningShell && block.source.toolResultItemId === undefined ||
      runningShell && (block.attempt !== 0 || block.body !== undefined || block.preview !== undefined || block.source.kind === 'tool' &&
        (block.source.toolResultItemId !== undefined || block.source.startedEventSeq === undefined)) ||
      block.state === 'running' && (!runningReasoning && !runningShell && privateFields || block.captureDigest !== undefined) ||
      block.classification === 'withheld' && privateFields ||
      block.body !== undefined && (block.captureDigest === undefined && !runningReasoning || ['unknown', 'superseded', 'withheld'].includes(block.state)) ||
      block.inputUtf16CodeUnits !== undefined && (!['write', 'edit'].includes(block.kind) || block.captureDigest === undefined ||
        ['running', 'unknown', 'superseded', 'withheld'].includes(block.state)) ||
      block.kind === 'native_reasoning' && (!['running', 'completed'].includes(block.state) && (block.body !== undefined || block.preview !== undefined) ||
        block.effectApplied !== undefined || block.settlementStatus !== undefined || block.path !== undefined) ||
      block.kind === 'shell' && (block.effectApplied !== undefined || block.settlementStatus !== undefined || block.path !== undefined) ||
      block.kind !== 'native_reasoning' && block.nativeIndex !== undefined ||
      block.kind !== 'shell' && block.exitCode !== undefined || block.state === 'running' && block.exitCode !== undefined) {
    context.addIssue({ code: 'custom', message: 'Invalid protected process block.' })
  }
})
const uniqueBlocks = z.array(processDisplayBlockSchemaV1).max(PROCESS_LOCAL_DISPLAY_MAX_BLOCKS_V1)
  .refine(blocks => {
    if (new Set(blocks.map(block => block.blockId)).size !== blocks.length) return false
    // These three sources share the protected owner's one actual observation
    // axis. Drafts and tool result/start coordinates do not belong to it.
    const ordinals = new Set<string>()
    for (const block of blocks) if (['native', 'provider_reasoning', 'tool_preparation'].includes(block.source.kind)) {
      if (!('sourceOrdinal' in block.source)) return false
      const identity = block.source.unitId + ':' + block.source.sourceOrdinal
      if (ordinals.has(identity)) return false
      ordinals.add(identity)
    }
    const attempts = new Set<string>(), latest = new Map<string, number>(), running = new Map<string, number>()
    for (const block of blocks) if (block.source.kind === 'assistant_draft') {
      const { unitId, attempt } = block.source, identity = unitId + ':' + attempt
      if (attempts.has(identity) || block.state === 'running' && running.has(unitId)) return false
      attempts.add(identity); latest.set(unitId, Math.max(latest.get(unitId) ?? 0, attempt))
      if (block.state === 'running') running.set(unitId, attempt)
    }
    const preparations = new Set<string>(), latestPreparation = new Map<string, number>(), activePreparation = new Map<string, { unitId: string; attempt: number }>()
    for (const block of blocks) if (block.source.kind === 'tool_preparation') {
      const { unitId, callId, attempt } = block.source, call = unitId + ':' + callId, identity = call + ':' + attempt
      if (preparations.has(identity) || block.state === 'preparing' && activePreparation.has(call)) return false
      preparations.add(identity); latestPreparation.set(unitId, Math.max(latestPreparation.get(unitId) ?? 0, attempt))
      if (block.state === 'preparing') activePreparation.set(call, { unitId, attempt })
    }
    return [...running].every(([unitId, attempt]) => latest.get(unitId) === attempt) &&
      [...activePreparation.values()].every(({ unitId, attempt }) => latestPreparation.get(unitId) === attempt)
  })
export const processDisplayFailureSchemaV1 = z.object({ status: z.enum(['unavailable', 'busy', 'revoked']) }).strict()
export const processDisplayAuthorizedSchemaV1 = z.object({
  status: z.literal('authorized'), schemaVersion: z.literal(2), revision, scope: processDisplayScopeSchemaV1, presence
}).strict()
export const processDisplaySnapshotSchemaV1 = z.object({
  status: z.literal('snapshot'), schemaVersion: z.literal(2), revision, scope: processDisplayScopeSchemaV1, units, blocks: uniqueBlocks
}).strict().refine(value => new TextEncoder().encode(JSON.stringify(value)).length <= PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1 &&
  value.blocks.every(block => sourceMatchesUnit(block, value.units.find(unit => unit.unitId === block.source.unitId))))
export const processDisplayUnchangedSchemaV1 = z.object({
  status: z.literal('unchanged'), schemaVersion: z.literal(2), revision, scope: processDisplayScopeSchemaV1
}).strict()
export const processDisplayCaptureSchemaV1 = z.object({
  status: z.literal('capture'), schemaVersion: z.literal(2), revision, scope: processDisplayScopeSchemaV1, unit: processDisplayUnitSchemaV2, block: processDisplayBlockSchemaV1
}).strict().refine(value => new TextEncoder().encode(JSON.stringify(value)).length <= PROCESS_LOCAL_DISPLAY_MAX_BYTES_V1 &&
  sourceMatchesUnit(value.block, value.unit))

function sourceMatchesUnit(block: z.infer<typeof processDisplayBlockSchemaV1>, unit: z.infer<typeof processDisplayUnitSchemaV2> | undefined): boolean {
  if (!unit || block.source.unitId !== unit.unitId) return false
  const source = block.source
  if (source.kind === 'native' || source.kind === 'provider_reasoning' || source.kind === 'tool_preparation' || source.kind === 'assistant_draft') return source.producerId === unit.producerId && source.providerSequence === unit.providerSequence
  return unit.calls.some(call => call.callId === source.callId && call.toolCallItemId === source.toolCallItemId &&
    call.toolResultItemId === source.toolResultItemId && (source.startedEventSeq === undefined || call.startedEventSeq === source.startedEventSeq))
}
// Main/preload fence envelope; never exposed through the renderer facade.
export const processPaneControlSchemaV2 = z.object({ schemaVersion: z.literal(2),
  documentEpoch: revision, authorityEpoch: revision }).strict()
export const processPaneAuthorizeEnvelopeSchemaV2 = z.object({
  control: processPaneControlSchemaV2,
  result: z.union([processDisplayAuthorizedSchemaV1.extend({ viewerId: uuid }).strict(), processDisplayFailureSchemaV1])
}).strict()
export const processPaneInvalidationRequestSchemaV2 = z.object({ paneId: recordId }).strict()

// Main-only Go wire. The token is minted by Main and bound by Go authorization;
// the document nonce names one irreversible native-document lease.
export const processDisplayAuthorizeRequestSchemaV1 = z.object({
  schemaVersion: z.literal(2), action: z.literal('authorize'), viewerToken: uuid, documentNonce: uuid,
  threadId: recordId, turnId: recordId
}).strict()
export const processDisplayReadRequestSchemaV1 = z.object({
  schemaVersion: z.literal(2), action: z.literal('read'), viewerToken: uuid, afterRevision: revision.optional(),
  waitMs: count.max(PROCESS_LOCAL_DISPLAY_MAX_WAIT_MS_V1), expandedBlockIds: blockIds
}).strict()
export const processDisplayRevokeRequestSchemaV1 = z.object({ schemaVersion: z.literal(2), action: z.literal('revoke'), viewerToken: uuid }).strict()
export const processDisplayTargetRequestSchemaV1 = z.object({
  schemaVersion: z.literal(2), action: z.literal('open_capture'), viewerToken: uuid, blockId: recordId, captureDigest: digest
}).strict()
export const processDisplayWebLinkRequestSchemaV1 = z.object({
  schemaVersion: z.literal(2), action: z.literal('open_web_link'), viewerToken: uuid,
  blockId: recordId, inputDigest: digest, role: z.literal('request')
}).strict()
// Main-only sealed target. This schema is intentionally absent from ordinary
// renderer process responses and the facade's action result.
export const processDisplayHostWebLinkSchemaV1 = z.object({
  status: z.literal('web_link'), schemaVersion: z.literal(2), revision, scope: processDisplayScopeSchemaV1,
  blockId: recordId, inputDigest: digest, role: z.literal('request'), targetUrl: webURL
}).strict()
export const processDisplayWebLinkResultSchemaV1 = z.union([processDisplayHostWebLinkSchemaV1, processDisplayFailureSchemaV1])
export const processDisplayAuthorizeResultSchemaV1 = z.union([processDisplayAuthorizedSchemaV1, processDisplayFailureSchemaV1])
export const processDisplayReadResultSchemaV1 = z.union([processDisplaySnapshotSchemaV1, processDisplayUnchangedSchemaV1, processDisplayFailureSchemaV1])
export const processDisplayTargetResultSchemaV1 = z.union([processDisplayCaptureSchemaV1, processDisplayFailureSchemaV1])

// Desktop wire. No variant accepts a Go token or arbitrary runtime route.
export const processPaneAuthorizeRequestSchemaV1 = z.object({ paneId: recordId, threadId: recordId, turnId: recordId }).strict()
export const processPaneViewerRequestSchemaV1 = z.object({ viewerId: uuid }).strict()
export const processPaneReadRequestSchemaV1 = processPaneViewerRequestSchemaV1.extend({
  afterRevision: revision.optional(), waitMs: count.max(PROCESS_LOCAL_DISPLAY_MAX_WAIT_MS_V1).optional(),
  // Desktop projection only; never forwarded as a new Go/public wire field.
  draftsOnly: z.boolean().optional()
}).strict()
export const processPaneExpansionRequestSchemaV1 = processPaneViewerRequestSchemaV1.extend({ expandedBlockIds: blockIds }).strict()
export const processPaneTargetRequestSchemaV1 = processPaneViewerRequestSchemaV1.extend({ blockId: recordId, captureDigest: digest }).strict()
export const processPaneWebLinkRequestSchemaV1 = processPaneViewerRequestSchemaV1.extend({
  blockId: recordId, inputDigest: digest, role: z.literal('request')
}).strict()
export const processPaneAuthorizeResultSchemaV1 = z.union([
  processDisplayAuthorizedSchemaV1.extend({ viewerId: uuid }).strict(), processDisplayFailureSchemaV1
])
export const processPaneReadResultSchemaV1 = processDisplayReadResultSchemaV1
export const processPaneTargetResultSchemaV1 = processDisplayTargetResultSchemaV1
export const processPaneActionResultSchemaV1 = z.union([z.object({ status: z.literal('ok') }).strict(), processDisplayFailureSchemaV1])

export type ProcessDisplayScopeV1 = z.infer<typeof processDisplayScopeSchemaV1>
export type ProcessDisplayBlockV1 = z.infer<typeof processDisplayBlockSchemaV1>
export const hasProcessDisplayPayloadV2 = (block: ProcessDisplayBlockV1): boolean =>
  block.input !== undefined || block.body !== undefined || block.draftSegments !== undefined || block.draftBytes !== undefined
export type ProcessDisplayAuthorizeResultV1 = z.infer<typeof processDisplayAuthorizeResultSchemaV1>
export type ProcessDisplayReadResultV1 = z.infer<typeof processDisplayReadResultSchemaV1>
export type ProcessDisplayTargetResultV1 = z.infer<typeof processDisplayTargetResultSchemaV1>
export type ProcessPaneAuthorizeRequestV1 = z.infer<typeof processPaneAuthorizeRequestSchemaV1>
export type ProcessPaneViewerRequestV1 = z.infer<typeof processPaneViewerRequestSchemaV1>
export type ProcessPaneReadRequestV1 = z.infer<typeof processPaneReadRequestSchemaV1>
export type ProcessPaneExpansionRequestV1 = z.infer<typeof processPaneExpansionRequestSchemaV1>
export type ProcessPaneTargetRequestV1 = z.infer<typeof processPaneTargetRequestSchemaV1>
export type ProcessPaneWebLinkRequestV1 = z.infer<typeof processPaneWebLinkRequestSchemaV1>
export type ProcessDisplayWebLinkResultV1 = z.infer<typeof processDisplayWebLinkResultSchemaV1>
export type ProcessPaneAuthorizeResultV1 = z.infer<typeof processPaneAuthorizeResultSchemaV1>
export type ProcessPaneReadResultV1 = z.infer<typeof processPaneReadResultSchemaV1>
export type ProcessPaneTargetResultV1 = z.infer<typeof processPaneTargetResultSchemaV1>
export type ProcessPaneActionResultV1 = z.infer<typeof processPaneActionResultSchemaV1>

// Legacy import names remain source aliases only. All successful data and Go
// actions require V2; this is a coordinated wire upgrade, not V1 fallback.
export const processDisplayScopeSchemaV2 = processDisplayScopeSchemaV1
export const processDisplayBlockSchemaV2 = processDisplayBlockSchemaV1
export const processDisplayAuthorizedSchemaV2 = processDisplayAuthorizedSchemaV1
export const processDisplaySnapshotSchemaV2 = processDisplaySnapshotSchemaV1
export const processDisplayCaptureSchemaV2 = processDisplayCaptureSchemaV1
export type ProcessDisplayUnitV2 = z.infer<typeof processDisplayUnitSchemaV2>
export type ProcessDisplaySourceV2 = z.infer<typeof processDisplaySourceSchemaV2>
export type ProcessDisplayScopeV2 = ProcessDisplayScopeV1
export type ProcessDisplayBlockV2 = ProcessDisplayBlockV1
export type ProcessPaneInvalidationSubscriptionV2 = {
  (request: { paneId: string }, handler: () => void): () => void
  (handler: () => void): () => void
}
