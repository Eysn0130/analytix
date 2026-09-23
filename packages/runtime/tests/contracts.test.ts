import { describe, expect, it } from 'vitest'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  ApprovalPolicySchema,
  DEFAULT_APPROVAL_POLICY,
  CreateThreadRequest,
  ThreadGoalSchema,
  ThreadSummarySchema,
  ThreadTodoListSchema,
  SetThreadGoalRequest,
  SetThreadTodosRequest,
  RuntimeEvent,
  PublicProjectionRevokedEvent,
  StartTurnRequest,
  SteerTurnRequest,
  InterruptTurnRequest,
  UpdateThreadRequest,
  UsageSnapshotSchema,
  AttachmentUploadRequest,
  AttachmentUploadResponse,
  MemoryRecord,
  AnalytixErrorBody,
  AnalytixCapabilitiesConfig,
  RuntimeCapabilityManifest,
  buildRuntimeCapabilityManifest,
  AcceptedFinalItemCompletedEventV1Schema,
  AcceptedFinalItemCompletedEventV3Schema,
  AcceptedFinalDeliveryBatchV1Schema,
  AcceptedFinalDeliveryBatchV2Schema,
  ThreadDetailResponseV1Schema,
  ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON,
  acceptedFinalDeliveryRequiresErrorItem,
  AcceptedFinalAuditRecordSchema,
  AcceptedFinalLiveRecordSchema,
  acceptedFinalPublicViewMatchesRecord,
  acceptedFinalPublicViewsMatch,
  AcceptedFinalPublicViewV1Schema,
  AcceptedFinalPublicViewV2Schema,
  AcceptedFinalPublicViewV3Schema,
  AcceptedFinalRecordV3Schema,
  BoundaryAcceptedFinalRecordV3Schema,
  BoundaryAcceptedFinalRecordV5Schema,
  FactFinalWitnessAdmissionV1Schema,
  FactFinalWitnessAdmissionV2Schema,
  WitnessedFactAcceptedFinalRecordV4Schema,
  WitnessedFactAcceptedFinalRecordV5Schema,
  PublicToolCallArgumentsProjectionV1,
  PublicToolResultProjectionV1,
  PrivateAcceptedFinalAssistantTextTurnItem,
  TurnItem,
  TurnSchema,
  emptyUsageSnapshot,
  ModelExecutionRefSchema,
  type AcceptedFinalAuditRecord,
  type AcceptedFinalTerminalReason,
  type AcceptedFinalLiveRecord,
  type AcceptedFinalPublicViewV1,
  type AcceptedFinalPublicViewV2,
  type AcceptedFinalPublicViewV3,
  type AcceptedFinalRecordV3,
  type BoundaryAcceptedFinalRecordV3,
  type BoundaryAcceptedFinalRecordV5,
  type WitnessedFactAcceptedFinalRecordV4,
  type WitnessedFactAcceptedFinalRecordV5,
  type RuntimeEvent as RuntimeEventType
} from '../src/contracts/index.js'
import {
  modelCapabilitiesForModel,
  modelContextProfilesFromConfig
} from '../src/shared/model-context-profile.js'
import {
  parseServeOptionsSafe,
  parseServeOptions,
  validateServeOptions,
  SERVE_USAGE,
  ServeExitCode
} from '../src/cli/serve.js'

describe('derived gate item history', () => {
  const base = {
    id: 'item-gate', turnId: 'turn-derived', threadId: 'thr-derived',
    createdAt: '2026-09-23T00:00:00Z'
  }

  it('preserves terminal approval and user input display after source handles are stripped', () => {
    const approval = { ...base, kind: 'approval', role: 'tool', status: 'expired',
      toolName: 'write_file', summary: 'Approve write_file' }
    const input = { ...base, kind: 'user_input', role: 'system', status: 'cancelled',
      prompt: 'Choose a path', questions: [] }
    expect(TurnItem.safeParse(approval).success).toBe(true)
    expect(TurnItem.safeParse(input).success).toBe(true)
    expect(TurnItem.safeParse({ ...approval, status: 'pending' }).success).toBe(false)
    expect(TurnItem.safeParse({ ...input, status: 'pending' }).success).toBe(false)
  })
})

function reasonixIntegrationTopologyFixture() {
  const row = (
    id: string,
    providerSpecific = false,
    providerFamilies?: string[],
    comparisonConclusion = 'conflict-product-baseline-engine-absorb'
  ) => ({
    id,
    capability: `Reasonix integration ${id} capability`,
    comparisonConclusion,
    decision: 'absorb stronger Reasonix engine behavior through existing Analytix contracts',
    reasonixStrongerBecause: 'Reasonix engine source has stronger internal orchestration for this capability.',
    kunAnalytixRetainedBecause: 'Kun/Analytix remains the product, settings, bridge, provider, and route baseline.',
    conflictPolicy: 'UI, settings, public protocol, and product constants remain Analytix-owned; engine internals may be adapted.',
    adoptedEngineConstants: ['engine lineage/cache/lifecycle semantics'],
    retainedProductConstants: ['window.analytix', 'runtime settings', 'analytix serve'],
    reasonixEngineNodes: [`/Users/sun/Projects/_upstreams/DeepSeek-Reasonix/internal/${id}.go`],
    kunAnalytixBaselineAnchors: ['desktop-entry-ui'],
    analytixEntryPoints: ['existing chat/goal/tool path'],
    runtimeContracts: ['packages/runtime/src/contracts/capabilities.ts'],
    goRuntimeLanding: ['packages/runtime-go/internal/upstreamaudit/reasonix_integration_topology.go'],
    typeScriptFallback: ['packages/runtime/src/loop-test-support/agent-loop.ts'],
    machineChecks: [{
      id: `${id}-contract`,
      kind: 'test',
      status: 'passed',
      evidence: 'packages/runtime-go/reasonix_integration_topology_test.go'
    }],
    ...(providerFamilies ? { providerFamilies } : {}),
    absorptionClass: providerSpecific ? 'code-port-and-adapt' : 'contract-reimplement',
    status: 'green',
    providerSpecific,
    doesNotNarrowProviders: true,
    existingEntryOnly: true,
    topLevelEntrypointAdded: false,
    upstreamPublicProtocolAdded: false,
    rendererContractChanged: false,
    settingsSchemaChanged: false,
    productIdentityChanged: false,
    stablePrefixContainsDynamicState: false,
    evidenceState: {
      codeLevelAbsorbed: true,
      localContractGreen: true,
      requiresCredentialedG6: false,
      credentialedEvidence: 'post-cutover-live-validation-pending',
      defaultCutoverCandidate: true,
      typeScriptFallbackRetain: false,
      blockers: ['credentialed-provider-matrix']
    }
  })
  return {
    schemaVersion: 1,
    changeId: 'reasonix-integration-topology',
    reasonixSourcePath: '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix',
    reasonixSourceCommit: '7032f39336f4ae5f216e1fcb3368e5f679723490',
    analytixSourceRoot: '/Users/sun/Projects/analytix',
    runtimeContract: 'analytix Go runtime server /v1 runtime HTTP/SSE contract',
    principle: 'Kun/Analytix remains the product baseline; Reasonix engine capabilities land only through existing analytix entries and contracts.',
    decisionMethod: [
      'Build Kun/Analytix baseline first.',
      'Absorb only stronger Reasonix engine behavior.',
      'Keep Kun/Analytix for product, UI, settings, bridge, providers, and public protocol.'
    ],
    baselineSurfaces: [{
      id: 'desktop-entry-ui',
      surface: 'Analytix desktop shell and preserved Kun/Analytix product routes.',
      triggerPath: 'renderer route surface and Electron bridge',
      runtimeContract: 'Renderer -> window.analytix -> preload -> main -> analytix runtime HTTP/SSE',
      settingsBridgePolicy: 'retain window.analytix and top-level runtime settings',
      providerPolicy: 'retain multi-model provider controls',
      decision: 'kun-analytix-baseline-retained',
      protectionTests: ['src/preload/preload-sandbox.test.ts']
    }],
    rows: [
      row('deepseek-cache-prefix-provider-adapter', true, ['deepseek', 'openai-compatible', 'anthropic-compatible', 'custom_endpoint']),
      row('agent-loop-job-subagent-lineage'),
      row('mcp-lifecycle-search-call-reconnect-redaction', false, undefined, 'reasonix-engine-stronger-absorb'),
      row('autoresearch-project-state-goal-research'),
      row('workflow-create-loop-internal-planner')
    ],
    baselineSurfaceCount: 1,
    rowCount: 5,
    reasonixStrongerAbsorbedCount: 5,
    conflictPolicyCount: 4,
    kunAnalytixRetainedSurfaceCount: 1,
    codeLevelAbsorbedCount: 5,
    existingEntryOnlyCount: 5,
    localContractGreenCount: 5,
    topLevelEntrypointAddedCount: 0,
    upstreamPublicProtocolAddedCount: 0,
    rendererContractChangedCount: 0,
    settingsSchemaChangedCount: 0,
    productIdentityChangedCount: 0,
    stablePrefixDynamicStateRowCount: 0,
    deepSeekEnhancementScopedOnly: true,
    multiModelNonRegressionProtected: true,
    typeScriptFallbackRetained: false,
    strictG6DefaultCutoverReady: false,
    goDefaultCutoverCandidate: true,
    externalEvidenceBlockers: ['credentialed-provider-matrix'],
    forbiddenTopLevelEntrypoints: ['Workflow', 'Create Loop', 'Subagent', 'AutoResearch', 'MCP-indexer'],
    notes: ['Reasonix integration topology is code-level runtime-info evidence, not a new product route.']
  }
}

const acceptedFinalSha = (character: string) => character.repeat(64)

function acceptedFinalV3Fixture(
  overrides: Partial<AcceptedFinalRecordV3> = {}
): AcceptedFinalRecordV3 {
  return {
    schemaVersion: 3,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: acceptedFinalSha('a'),
    authorityPublicKey: 'A'.repeat(43),
    threadId: 'thr_case',
    turnId: 'turn_case',
    envelopeDigest: acceptedFinalSha('b'),
    contextDigest: acceptedFinalSha('c'),
    contextEpoch: 7,
    datasetSnapshotId: 'snapshot_case_7',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    renderedTextSha256: acceptedFinalSha('d'),
    registrySequence: 0,
    registryStateDigest: acceptedFinalSha('e'),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v2',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    privateRecordDigest: acceptedFinalSha('f'),
    acceptedAt: '2026-07-11T01:02:03Z',
    authoritySignature: 'A'.repeat(86),
    recordDigest: acceptedFinalSha('9'),
    ...overrides
  }
}

function acceptedFinalBoundaryV3Fixture(
  overrides: Partial<BoundaryAcceptedFinalRecordV3> = {}
): BoundaryAcceptedFinalRecordV3 {
  return {
    ...acceptedFinalV3Fixture(),
    variant: 'SourceUnavailableAnswer',
    ...overrides
  }
}

function acceptedFinalV4Fixture(
  overrides: Partial<WitnessedFactAcceptedFinalRecordV4> = {}
): WitnessedFactAcceptedFinalRecordV4 {
  const authorityKeyId = acceptedFinalSha('a')
  const contextDigest = acceptedFinalSha('c')
  const envelopeDigest = acceptedFinalSha('b')
  const renderedTextSha256 = acceptedFinalSha('d')
  const publicationSnapshotProofDigest = acceptedFinalSha('1')
  const registryStateDigest = acceptedFinalSha('e')
  const bundleRecordDigest = acceptedFinalSha('4')
  const bundleGeneration = 5
  return {
    schemaVersion: 4,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId,
    authorityPublicKey: 'A'.repeat(43),
    threadId: 'thr_case',
    turnId: 'turn_case',
    envelopeDigest,
    contextDigest,
    contextEpoch: 7,
    datasetSnapshotId: 'snapshot_case_7',
    variant: 'EvidenceBackedAnswer',
    terminalReason: 'success',
    renderedTextSha256,
    registrySequence: 3,
    registryStateDigest,
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v3',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicationSnapshotProofDigest,
    factFinalWitnessAdmission: {
      schemaVersion: 1,
      purpose: 'analytix.fact-final-witness-admission/v1',
      contextDigest,
      datasetSnapshotId: 'snapshot_case_7',
      sourceManifestHash: acceptedFinalSha('2'),
      envelopeDigest,
      renderedTextSha256,
      publicationSnapshotProofDigest,
      registrySequence: 3,
      registryStateDigest,
      evidenceReceiptIdsDigest: acceptedFinalSha('3'),
      evidenceReceiptCount: 2,
      evidenceAuthorityBundleDigest: bundleRecordDigest,
      evidenceAuthorityBundleGeneration: bundleGeneration,
      evidenceRegistryIndexDigest: acceptedFinalSha('5'),
      evidenceRegistryCount: 3,
      selectedRegistryIndexDigest: acceptedFinalSha('4'),
      selectedRegistryIndexGeneration: 2,
      selectedRegistryCapsuleDigest: acceptedFinalSha('6'),
      witnessBinding: {
        schemaVersion: 1,
        purpose: 'analytix.evidence-authority-witness-binding/v1',
        installationId: acceptedFinalSha('6'),
        enrollmentId: acceptedFinalSha('7'),
        namespace: 'analytix.evidence-registry-authority/v1',
        authorityKeyId,
        witnessKeyId: acceptedFinalSha('8'),
        bundleRecordDigest,
        bundleGeneration,
        observeRequestDigest: acceptedFinalSha('9'),
        checkpointDigest: acceptedFinalSha('0'),
        observationDigest: acceptedFinalSha('1'),
        bindingDigest: acceptedFinalSha('2')
      },
      admittedAt: '2026-07-11T01:02:02.5Z',
      admissionDigest: acceptedFinalSha('3')
    },
    privateRecordDigest: acceptedFinalSha('f'),
    acceptedAt: '2026-07-11T01:02:03Z',
    authoritySignature: 'A'.repeat(86),
    recordDigest: acceptedFinalSha('9'),
    ...overrides
  }
}

function acceptedFinalBoundaryV5Fixture(
  overrides: Partial<BoundaryAcceptedFinalRecordV5> = {}
): BoundaryAcceptedFinalRecordV5 {
  const envelopeDigest = acceptedFinalSha('b')
  const contextDigest = acceptedFinalSha('c')
  const acceptedAt = '2026-07-11T01:02:03Z'
  const publicView: BoundaryAcceptedFinalRecordV5['publicView'] = {
    schemaVersion: 2 as const,
    publicationState: 'accepted' as const,
    envelopeDigest,
    contextDigest,
    contextEpoch: 7,
    datasetSnapshotId: 'snapshot_case_7',
    variant: 'SourceUnavailableAnswer' as const,
    terminalReason: 'source_unavailable' as const,
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable' as const,
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only' as const,
      count: 0,
      setDigest: acceptedFinalSha('8'),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: acceptedAt,
    acceptedAt
  }
  return {
    schemaVersion: 5,
    authorityPurpose: 'analytix.case-final/v1',
    authorityAlgorithm: 'Ed25519',
    authorityKeyId: acceptedFinalSha('a'),
    authorityPublicKey: 'A'.repeat(43),
    threadId: 'thr_case',
    turnId: 'turn_case',
    envelopeDigest,
    contextDigest,
    contextEpoch: 7,
    datasetSnapshotId: 'snapshot_case_7',
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    renderedTextSha256: acceptedFinalSha('d'),
    registrySequence: 0,
    registryStateDigest: acceptedFinalSha('e'),
    rendererVersion: 'analytix.host-final-renderer/v1',
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    verifierVersion: 'analytix.claim-verifier-policy/v1',
    publicView,
    publicViewDigest: acceptedFinalSha('7'),
    privateRecordDigest: acceptedFinalSha('f'),
    acceptedAt,
    authoritySignature: 'A'.repeat(86),
    recordDigest: acceptedFinalSha('9'),
    ...overrides
  }
}

function acceptedFinalV5FactFixture(
  overrides: Partial<WitnessedFactAcceptedFinalRecordV5> = {}
): WitnessedFactAcceptedFinalRecordV5 {
  const historical = acceptedFinalV4Fixture()
  const acceptedAt = historical.acceptedAt
  return {
    ...historical,
    schemaVersion: 5,
    finalGateVersion: 'analytix.final-evidence-gate/v4',
    publicView: {
      schemaVersion: 2,
      publicationState: 'accepted',
      envelopeDigest: historical.envelopeDigest,
      contextDigest: historical.contextDigest,
      contextEpoch: historical.contextEpoch,
      datasetSnapshotId: historical.datasetSnapshotId,
      variant: historical.variant,
      terminalReason: historical.terminalReason,
      blockerCode: '',
      coverageStatus: 'complete',
      checkedScopeDigest: acceptedFinalSha('2'),
      missingScopeCount: 0,
      claimCount: 2,
      claimTypes: ['account', 'amount'],
      receiptMetadata: {
        projection: 'masked_metadata_only',
        count: 2,
        setDigest: acceptedFinalSha('3'),
        citations: [{
          handle: `cite_${acceptedFinalSha('4')}`,
          label: 'evidence-1'
        }, {
          handle: `cite_${acceptedFinalSha('5')}`,
          label: 'evidence-2'
        }]
      },
      noHitWording: '',
      envelopeIssuedAt: acceptedAt,
      acceptedAt
    },
    publicViewDigest: acceptedFinalSha('7'),
    ...overrides
  }
}

function acceptedFinalPublicViewV1Fixture(
  record: AcceptedFinalAuditRecord = acceptedFinalBoundaryV3Fixture(),
  overrides: Partial<AcceptedFinalPublicViewV1> = {}
): AcceptedFinalPublicViewV1 {
  return {
    schemaVersion: 1,
    publicationState: 'accepted',
    acceptedFinalDigest: record.recordDigest,
    envelopeDigest: record.envelopeDigest,
    contextDigest: record.contextDigest,
    contextEpoch: record.contextEpoch,
    datasetSnapshotId: record.datasetSnapshotId,
    variant: record.variant,
    terminalReason: record.terminalReason,
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: acceptedFinalSha('8'),
      citations: []
    },
    noHitWording: '',
    envelopeIssuedAt: record.acceptedAt,
    acceptedAt: record.acceptedAt,
    ...overrides
  }
}

function acceptedFinalPublicViewV2Fixture(
  record: AcceptedFinalLiveRecord = acceptedFinalBoundaryV5Fixture(),
  overrides: Partial<AcceptedFinalPublicViewV2> = {}
): AcceptedFinalPublicViewV2 {
  return {
    ...record.publicView,
    acceptedFinalDigest: record.recordDigest,
    publicViewDigest: record.publicViewDigest,
    ...overrides
  }
}

function acceptedFinalPublicViewV3Fixture(
  record: AcceptedFinalLiveRecord = acceptedFinalBoundaryV5Fixture(),
  overrides: Partial<AcceptedFinalPublicViewV3> = {}
): AcceptedFinalPublicViewV3 {
  const core = record.publicView
  return {
    schemaVersion: 3,
    acceptedFinalDigest: record.recordDigest,
    publicationState: 'accepted',
    variant: core.variant,
    terminalReason: core.terminalReason,
    blockerCode: core.blockerCode,
    coverageStatus: core.coverageStatus,
    checkedScopeDigest: core.checkedScopeDigest,
    missingScopeCount: core.missingScopeCount,
    claimCount: core.claimCount,
    claimTypes: core.claimTypes,
    receiptMetadata: core.receiptMetadata,
    noHitWording: core.noHitWording,
    acceptedAt: core.acceptedAt,
    ...overrides
  }
}

async function goAcceptedFinalV5V2Fixture(): Promise<WitnessedFactAcceptedFinalRecordV5> {
  const value = JSON.parse(await readFile(new URL(
    '../../runtime-go/internal/domain/evidence/testdata/accepted-final-v5-fact-witness-admission-v2.json',
    import.meta.url
  ), 'utf8'))
  return AcceptedFinalLiveRecordSchema.parse(value) as WitnessedFactAcceptedFinalRecordV5
}

function acceptedAssistantFixture(
  record: AcceptedFinalLiveRecord = acceptedFinalBoundaryV5Fixture(),
  view: AcceptedFinalPublicViewV2 = acceptedFinalPublicViewV2Fixture(record)
) {
  return {
    id: 'item_turn_case_assistant',
    turnId: record.turnId,
    threadId: record.threadId,
    role: 'assistant' as const,
    status: 'completed' as const,
    createdAt: '2026-07-11T01:02:02.000Z',
    finishedAt: record.acceptedAt,
    kind: 'assistant_text' as const,
    text: '当前案件来源未通过核验。',
    acceptedFinal: record,
    acceptedFinalView: view
  }
}

function acceptedFinalDeliveryRecordFixture(reason: AcceptedFinalTerminalReason): AcceptedFinalLiveRecord {
  const base = acceptedFinalBoundaryV5Fixture()
  return acceptedFinalBoundaryV5Fixture({
    terminalReason: reason,
    publicView: { ...base.publicView, terminalReason: reason }
  })
}

function acceptedFinalDeliveryContractFixtureV2(reason: AcceptedFinalTerminalReason) {
  const status = ACCEPTED_FINAL_TERMINAL_STATUS_BY_REASON[reason]
  const terminalMessage = status === 'completed'
    ? '案件分析已结束；仅发布通过宿主证据门的固定边界答复。'
    : status === 'aborted'
      ? '案件分析已终止；未经核验的案件事实未发布。'
      : '案件分析未完成；未经核验的案件事实未发布。'
  const record = acceptedFinalDeliveryRecordFixture(reason)
  const item = acceptedAssistantFixture(record) as Record<string, any>
  item.acceptedFinalView = acceptedFinalPublicViewV3Fixture(record)
  delete item.acceptedFinal
  const commit = record.recordDigest
  const timestamp = record.acceptedAt
  const requiresErrorItem = acceptedFinalDeliveryRequiresErrorItem(reason)
  const events: Array<Record<string, unknown>> = [{
    kind: 'item_completed',
    timestamp,
    threadId: record.threadId,
    turnId: record.turnId,
    itemId: item.id,
    item,
    acceptedFinalDigest: commit,
    publicationCommitId: commit,
    publicationEventId: acceptedFinalSha('1'),
    publicationSlot: 'assistant-final',
    publicationPayloadDigest: acceptedFinalSha('2')
  }]
  const errorItemId = `item_${record.turnId}_case_terminal`
  if (requiresErrorItem) {
    events.push({
      kind: 'item_completed',
      timestamp,
      threadId: record.threadId,
      turnId: record.turnId,
      itemId: errorItemId,
      item: {
        id: errorItemId,
        turnId: record.turnId,
        threadId: record.threadId,
        role: 'system',
        status,
        createdAt: timestamp,
        finishedAt: timestamp,
        kind: 'error',
        code: `case_terminal_${reason}`,
        message: terminalMessage,
        severity: status === 'failed' ? 'error' : 'warning',
        acceptedFinalDigest: commit
      },
      acceptedFinalDigest: commit,
      publicationCommitId: commit,
      publicationEventId: acceptedFinalSha('3'),
      publicationSlot: 'terminal-error-item',
      publicationPayloadDigest: acceptedFinalSha('4')
    })
  }
  events.push({
    kind: 'usage',
    timestamp,
    threadId: record.threadId,
    turnId: record.turnId,
    model: 'deepseek-chat',
    usage: emptyUsageSnapshot(),
    cacheDiagnostics: {},
    usageFinalStatus: status,
    acceptedFinalDigest: commit,
    publicationCommitId: commit,
    publicationEventId: acceptedFinalSha('5'),
    publicationSlot: 'usage',
    publicationPayloadDigest: acceptedFinalSha('6')
  })
  events.push({
    kind: `turn_${status}`,
    timestamp,
    threadId: record.threadId,
    turnId: record.turnId,
    status,
    terminalReason: reason,
    code: `case_terminal_${reason}`,
    ...(requiresErrorItem ? { itemId: errorItemId } : {}),
    ...(status === 'failed' ? { error: terminalMessage, message: terminalMessage } : {}),
    ...(status === 'aborted' ? { discard: true, cancelled: true, cancelledPendingGates: 2 } : {}),
    acceptedFinalDigest: commit,
    publicationCommitId: commit,
    publicationEventId: acceptedFinalSha('7'),
    publicationSlot: 'terminal',
    publicationPayloadDigest: acceptedFinalSha('8')
  })
  const firstSeq = 20
  for (const [index, event] of events.entries()) event.seq = firstSeq + index
  const lastSeq = firstSeq + events.length - 1
  const batchId = acceptedFinalSha('9')
  const manifest = acceptedFinalSha('a')
  return {
    schemaVersion: 2,
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    kind: 'accepted_final_batch',
    batchId,
    threadId: record.threadId,
    turnId: record.turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    publicationCommitId: commit,
    eventManifestDigest: manifest,
    publicationAuthority: {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: acceptedFinalSha('b'),
      threadId: record.threadId,
      turnId: record.turnId,
      publicationCommitId: commit,
      acceptedFinalDispositionDigest: acceptedFinalSha('c'),
      terminalDispositionId: acceptedFinalSha('d'),
      eventManifestDigest: manifest,
      sequencedEventsDigest: acceptedFinalSha('e'),
      batchId,
      firstSeq,
      lastSeq,
      timestamp,
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: record.authorityKeyId,
      authorityPublicKey: record.authorityPublicKey,
      authoritySignature: record.authoritySignature
    },
    events
  }
}

function acceptedFinalDeliveryAuditFixtureV1(reason: AcceptedFinalTerminalReason) {
  const batch = structuredClone(acceptedFinalDeliveryContractFixtureV2(reason))
  const record = acceptedFinalDeliveryRecordFixture(reason)
  const privateItem = acceptedAssistantFixture(record)
  batch.schemaVersion = 1
  batch.purpose = 'analytix.accepted-final-delivery-batch/v1'
  batch.events[0].item = privateItem
  batch.events[0].itemId = privateItem.id
  return batch
}

function reasonixSuperiorityMatrixFixture() {
  const baseImpact = (providerScoped = false) => ({
    uiSurfaceChanged: false,
    settingsSchemaChanged: false,
    bridgeChanged: false,
    providerMultiModelChanged: false,
    providerScoped,
    doesNotNarrowProviders: true,
    topLevelEntrypointAdded: false,
    reasonixPublicProtocolAdded: false,
    stablePrefixUsesDynamicState: false
  })
  const row = (
    id: string,
    decision: 'absorb' | 'adapt' | 'keep-kun' | 'reject' | 'defer',
    options: {
      codeLevelAbsorbed?: boolean
      providerScoped?: boolean
      status?: 'green' | 'rejected' | 'deferred'
      absorptionClass?: 'code-port-and-adapt' | 'contract-reimplement' | 'reject' | 'defer'
      requiresLiveEvidence?: boolean
      liveEvidenceStatus?: 'blocked' | 'not-required' | 'post-cutover-live-validation-pending'
      keepKunReason?: string
      kunAnalytixStrongerEvidence?: string[]
      baselineRetainedReason?: string
      rejectedReason?: string
      deferredReason?: string
      reasonixStrongerEvidence?: string[]
      decisionEvidence?: string[]
    } = {}
  ) => ({
    id,
    capability: `Reasonix superiority ${id} capability`,
    reasonixSourcePath: '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix',
    reasonixSourceCommit: '7032f39336f4ae5f216e1fcb3368e5f679723490',
    reasonixSourceSymbols: [`internal/${id}.go:Symbol`],
    kunAnalytixBaselinePaths: ['src/preload/index.ts', 'packages/runtime/src/loop-test-support/agent-loop.ts'],
    currentBehavior: 'Analytix owns the current product/runtime contract.',
    reasonixStrongerEvidence: (decision === 'absorb' || decision === 'adapt')
      ? options.reasonixStrongerEvidence ?? ['source-level deterministic evidence only']
      : [],
    ...((decision === 'reject' || decision === 'defer') ? {
      decisionEvidence: options.decisionEvidence ?? [
        'machine-readable policy or external-blocker evidence only'
      ]
    } : {}),
    deterministicEvidence: [{
      id: `${id}-deterministic-evidence`,
      kind: 'test',
      status: 'passed',
      evidence: 'packages/runtime-go/reasonix_superiority_matrix_test.go'
    }],
    codeReusable: options.codeLevelAbsorbed ?? true,
    codeReuseMode: options.absorptionClass ?? 'contract-reimplement',
    analytixTargetPaths: ['packages/runtime-go/internal/upstreamaudit/reasonix_superiority_matrix.go'],
    productImpact: baseImpact(options.providerScoped),
    regressionTestPaths: ['packages/runtime-go/reasonix_superiority_matrix_test.go'],
    decision,
    absorptionClass: options.absorptionClass ?? 'contract-reimplement',
    status: options.status ?? 'green',
    codeLevelAbsorbed: options.codeLevelAbsorbed ?? true,
    localDeterministicGreen: decision === 'defer' ? false : true,
    requiresLiveEvidence: options.requiresLiveEvidence ?? decision === 'defer',
    liveEvidenceStatus: options.liveEvidenceStatus ?? (decision === 'defer' ? 'post-cutover-live-validation-pending' : 'not-required'),
    ...(options.keepKunReason ? { keepKunReason: options.keepKunReason } : {}),
    ...(options.keepKunReason ? {
      kunAnalytixStrongerEvidence: options.kunAnalytixStrongerEvidence ?? [
        'Analytix/Kun baseline is retained by deterministic contract evidence.'
      ],
      baselineRetainedReason: options.baselineRetainedReason ?? options.keepKunReason
    } : {}),
    ...(options.rejectedReason ? { rejectedReason: options.rejectedReason } : {}),
    ...(options.deferredReason ? { deferredReason: options.deferredReason } : {}),
    typeScriptFallbackRetained: false
  })
  return {
    schemaVersion: 1,
    changeId: 'reasonix-superiority-matrix',
    stage: 'reasonix-superiority-code-stage',
    reasonixAbsorptionStatus: 'code-stage-closed',
    goRuntimeCoreStatus: 'deterministic-core-green',
    goDefaultLiveGateStatus: 'post-cutover-live-validation-pending',
    reasonixSourcePath: '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix',
    reasonixSourceCommit: '7032f39336f4ae5f216e1fcb3368e5f679723490',
    kunSourceSnapshots: [
      'https://github.com/KunAgent/Kun.git refs/tags/v0.2.13^{}=2ba8decc2f56862e7f677fcf89bbc3d402ec3a23',
      'https://github.com/KunAgent/Kun.git refs/tags/v0.2.14^{}=8f2040349fba47fcd8e8b94f50b131943af839b2'
    ],
    analytixSourceRoot: '/Users/sun/Projects/analytix',
    evidencePolicy: 'source-level comparison, deterministic oracles, fixtures, unit/integration tests, runtime telemetry, and JSON evidence only',
    llmAnswerQualityEvidenceUsed: false,
    deterministicEvidenceOnly: true,
    rows: [
      row('deepseek-prefix-cache-stability', 'adapt', { providerScoped: true, absorptionClass: 'code-port-and-adapt' }),
      row('prefix-shape-contract-baseline', 'keep-kun', {
        codeLevelAbsorbed: false,
        providerScoped: true,
        absorptionClass: 'reject',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        keepKunReason: 'Analytix prefix contract is broader.'
      }),
      row('provider-stream-usage-reasoning-guards', 'adapt'),
      row('provider-endpoint-family-request-shape', 'keep-kun', {
        codeLevelAbsorbed: false,
        providerScoped: true,
        absorptionClass: 'reject',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        keepKunReason: 'Analytix endpoint family contract is broader.'
      }),
      row('agent-loop-job-subagent-lineage', 'adapt'),
      row('mcp-lifecycle-lazy-schema-reconnect-redaction', 'absorb'),
      row('mcp-search-meta-tool-boundary', 'keep-kun', {
        codeLevelAbsorbed: false,
        absorptionClass: 'reject',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        keepKunReason: 'Analytix MCP search/meta-tool boundary is broader.'
      }),
      row('approval-user-input-tool-result-history-repair', 'adapt'),
      row('runtime-event-sse-session-durability', 'keep-kun', {
        codeLevelAbsorbed: false,
        absorptionClass: 'reject',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        keepKunReason: 'Analytix durable runtime event replay is stronger.'
      }),
      row('autoresearch-workflow-create-loop-engine-discipline', 'adapt'),
      row('kun-analytix-product-layer-baseline', 'keep-kun', {
        codeLevelAbsorbed: false,
        absorptionClass: 'reject',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        keepKunReason: 'Reasonix product layer is not proven stronger.'
      }),
      row('reasonix-public-protocol-and-top-level-ui', 'reject', {
        codeLevelAbsorbed: false,
        absorptionClass: 'reject',
        status: 'rejected',
        requiresLiveEvidence: false,
        liveEvidenceStatus: 'not-required',
        rejectedReason: 'Public protocol import is forbidden.'
      }),
      row('go-default-live-cutover-evidence', 'defer', {
        codeLevelAbsorbed: false,
        absorptionClass: 'defer',
        status: 'deferred',
        deferredReason: 'Real live evidence is not present.'
      })
    ],
    rowCount: 13,
    absorbCount: 1,
    adaptCount: 5,
    keepKunCount: 5,
    rejectCount: 1,
    deferCount: 1,
    codeLevelAbsorbedCount: 6,
    localDeterministicGreenCount: 12,
    productImpactChangedCount: 0,
    topLevelEntrypointAddedCount: 0,
    reasonixPublicProtocolAddedCount: 0,
    providerMultiModelChangedCount: 0,
    stablePrefixDynamicStateRowCount: 0,
    deepSeekEnhancementScopedOnly: true,
    kunAnalytixProductLayerPreserved: true,
    reasonixEngineRuntimeOnly: true,
    typeScriptFallbackRetained: false,
    codeStageClosed: true,
    deterministicAbsorptionIds: [
      'deepseek-prefix-cache-stability',
      'provider-stream-usage-reasoning-guards',
      'agent-loop-job-subagent-lineage',
      'mcp-lifecycle-lazy-schema-reconnect-redaction',
      'approval-user-input-tool-result-history-repair',
      'autoresearch-workflow-create-loop-engine-discipline'
    ],
    pendingLiveValidationIds: ['go-default-live-cutover-evidence'],
    strictG6DefaultCutoverReady: false,
    goDefaultCutoverCandidate: true,
    liveCutoverStage: 'reasonix-superiority-live-gate',
    liveCutoverStatus: 'post-cutover-live-validation-pending',
    liveEvidenceBlockers: ['credentialed-provider-matrix-json'],
    forbiddenTopLevelEntrypoints: ['Workflow', 'Create Loop', 'Subagent', 'AutoResearch', 'MCP-indexer'],
    notes: ['No LLM answer-quality evidence is used.']
  }
}

describe('contracts', () => {
  it('keeps public projection revocation outside durable runtime events', () => {
    const control = {
      schemaVersion: 1,
      kind: 'public_projection_revoked',
      threadId: 'thread-case',
      historyAuthority: 'case_boundary_only_v1',
      code: 'case_public_authority_rejected',
      action: 'purge_case_projection',
      terminal: true
    }
    expect(PublicProjectionRevokedEvent.parse(control)).toEqual(control)
    expect(RuntimeEvent.safeParse(control).success).toBe(false)
    expect(PublicProjectionRevokedEvent.safeParse({ ...control, seq: 7 }).success).toBe(false)
    expect(PublicProjectionRevokedEvent.safeParse({
      ...control,
      threadId: ' thread-case'
    }).success).toBe(false)
  })

  it('enforces hard provider-bound ModelExecutionRef contracts', () => {
    expect(ModelExecutionRefSchema.parse({
      providerId: 'anthropic-main',
      modelId: 'claude-3-5-sonnet',
      variant: '20260620',
      endpointFormat: 'messages',
      baseUrlFingerprint: 'd1f2a3b4c5d6e7f8',
      capabilityFingerprint: 'a1b2c3d4e5f60718',
      source: 'thread',
      resolvedAt: '2026-06-30T00:00:00.000Z'
    })).toMatchObject({
      providerId: 'anthropic-main',
      modelId: 'claude-3-5-sonnet',
      endpointFormat: 'messages',
      source: 'thread'
    })
    expect(ModelExecutionRefSchema.parse({
      providerId: 'runtime-default',
      modelId: 'default-model',
      endpointFormat: 'chat_completions',
      baseUrlFingerprint: 'f1e2d3c4b5a69788',
      capabilityFingerprint: 'b1c2d3e4f5061728',
      source: 'runtime-default',
      resolvedAt: '2026-06-30T00:00:00.000Z'
    })).toMatchObject({
      providerId: 'runtime-default',
      modelId: 'default-model',
      source: 'runtime-default'
    })
    expect(() => ModelExecutionRefSchema.parse({
      modelId: 'default-model',
      source: 'runtime-default',
      resolvedAt: '2026-06-30T00:00:00.000Z'
    })).toThrow(/providerId/)
    expect(() => ModelExecutionRefSchema.parse({
      providerId: 'runtime-default',
      modelId: 'default-model',
      source: 'fallback',
      resolvedAt: '2026-06-30T00:00:00.000Z'
    })).toThrow(/Invalid option/)
    expect(() => ModelExecutionRefSchema.parse({
      providerId: 'anthropic-main',
      modelId: 'claude-3-5-sonnet',
      source: 'thread',
      resolvedAt: '2026-06-30T00:00:00.000Z',
      fallbackReason: 'should not be here'
    })).toThrow(/fallbackReason/)
  })

  it('round-trips a thread creation payload through zod', () => {
    const parsed = CreateThreadRequest.parse({
      title: 'demo',
      workspace: '/tmp/ws',
      model: 'deepseek-chat',
      providerId: 'zai-coding-plan'
    })
    expect(parsed.title).toBe('demo')
    expect(parsed.providerId).toBe('zai-coding-plan')
    expect(parsed.mode).toBe('agent')
  })

  it('accepts thread goal contracts and events', () => {
    const goal = ThreadGoalSchema.parse({
      threadId: 'thr_1',
      objective: 'ship goal mode',
      status: 'active',
      tokenBudget: 1000,
      tokensUsed: 0,
      timeUsedSeconds: 0,
      blockedReason: 'Need API access',
      blockedCount: 1,
      blockedTurnId: 'turn_1',
      strictCompletion: true,
      selfCheckRequired: true,
      selfCheckCompleted: false,
      selfCheckTurnId: 'turn_1',
      createdAt: '2026-06-03T00:00:00.000Z',
      updatedAt: '2026-06-03T00:00:00.000Z'
    })
    expect(goal.objective).toBe('ship goal mode')
    expect(goal.blockedCount).toBe(1)
    expect(goal.strictCompletion).toBe(true)
    expect(SetThreadGoalRequest.parse({ status: 'paused' }).status).toBe('paused')
    expect(SetThreadGoalRequest.parse({
      status: 'active',
      blockedReason: 'Need API access',
      blockedCount: 1,
      blockedTurnId: 'turn_1',
      strictCompletion: true,
      selfCheckRequired: true,
      selfCheckCompleted: false,
      selfCheckTurnId: 'turn_1'
    })).toMatchObject({
      blockedReason: 'Need API access',
      strictCompletion: true,
      selfCheckRequired: true,
      selfCheckCompleted: false
    })
    const event = RuntimeEvent.parse({
      kind: 'goal_updated',
      seq: 1,
      timestamp: '2026-06-03T00:00:01.000Z',
      threadId: 'thr_1',
      goal
    })
    expect(event.kind).toBe('goal_updated')
    const audit = RuntimeEvent.parse({
      kind: 'goal_evidence_audit',
      seq: 2,
      timestamp: '2026-06-03T00:00:02.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      schemaVersion: 1,
      changeId: 'goal-evidence-audit',
      runtimeContract: 'analytix-go-runtime',
      upstreamSource: 'reasonix-absorbed',
      goalId: 'goal_thr_1',
      result: 'blocked',
      recovered: false,
      missingProjectChecks: 1,
      incompleteTodos: 0,
      commandMismatchMissing: 1,
      latestWriterReceiptIndex: 1,
      blockedStateKey: 'missingprojectchecks:runtime-go-unit:incompletetodos:none',
      missingCheckIds: ['runtime-go-unit'],
      missingCommands: ['go test ./...'],
      usesReasonixPublicProtocol: false,
      usesReasonixConfigRoot: false,
      changesRendererContract: false,
      changesProductIdentity: false
    })
    expect(audit.kind).toBe('goal_evidence_audit')
    const mcpAudit = RuntimeEvent.parse({
      kind: 'mcp_lifecycle_audit',
      seq: 3,
      timestamp: '2026-06-03T00:00:03.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      schemaVersion: 1,
      changeId: 'mcp-lifecycle-contract',
      runtimeContract: 'analytix-go-runtime',
      upstreamSource: 'reasonix-absorbed',
      serverId: 'analytix local',
      transport: 'live-local-jsonrpc',
      protocolVersion: '2025-11-25',
      credentialed: true,
      initialized: true,
      notificationSent: true,
      connected: true,
      reconnect: true,
      toolCount: 2,
      toolNames: ['mcp__analytix_local_56366d__search_issues_244952'],
      searchQuery: 'issue',
      searchMatches: ['mcp__analytix_local_56366d__search_issues_244952'],
      namespacePrefix: 'mcp__analytix_local_56366d__',
      namespacedToolNames: true,
      schemaOrderStable: true,
      schemaHash: 'abc',
      readOnlyHintMapped: true,
      callRequiresApproval: true,
      deniedCallExecuted: false,
      approvedCallExecuted: true,
      approvedCallOutput: 'issue-42',
      credentialRedaction: true,
      diagnostic: 'Authorization=<redacted>',
      rawSecretPresent: false,
      topLevelMcpIndexerExposed: false,
      reasonixPublicProtocolUsed: false,
      usesReasonixConfigRoot: false,
      changesRendererContract: false,
      changesProductIdentity: false
    })
    expect(mcpAudit.kind).toBe('mcp_lifecycle_audit')
    const autoResearchAudit = RuntimeEvent.parse({
      kind: 'autoresearch_state_audit',
      seq: 4,
      timestamp: '2026-06-03T00:00:04.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      schemaVersion: 1,
      changeId: 'autoresearch-state-audit',
      runtimeContract: 'analytix-go-runtime',
      upstreamSource: 'reasonix-absorbed',
      goalMode: 'research',
      stateRelativePath: '.analytix/autoresearch/thr_1',
      taskSpecPath: '.analytix/autoresearch/thr_1/task_spec.md',
      progressPath: '.analytix/autoresearch/thr_1/progress.json',
      findingsPath: '.analytix/autoresearch/thr_1/findings.jsonl',
      directionsTriedPath: '.analytix/autoresearch/thr_1/directions_tried.json',
      iterationLogPath: '.analytix/autoresearch/thr_1/iteration_log.jsonl',
      requiredFiles: [
        'task_spec.md',
        'progress.json',
        'findings.jsonl',
        'directions_tried.json',
        'iteration_log.jsonl'
      ],
      fileCount: 5,
      requirementCount: 2,
      completedRequirementCount: 0,
      staleRequirementCount: 2,
      staleDirectionCount: 1,
      complete: false,
      pivotRequired: true,
      result: 'pivot_required',
      unknownRequirementAccepted: false,
      findingsWrittenForUnknownRequirement: false,
      writesReasonixFile: false,
      writesAgentsFile: false,
      stablePrefixContainsState: false,
      toolSchemaContainsState: false,
      topLevelAutoResearchRouteExposed: false,
      usesReasonixPublicProtocol: false,
      usesReasonixConfigRoot: false,
      changesRendererContract: false,
      changesProductIdentity: false
    })
    expect(autoResearchAudit.kind).toBe('autoresearch_state_audit')
  })

  it('accepts sidecar-backed thread summary preview and counts', () => {
    const summary = ThreadSummarySchema.parse({
      id: 'thr_summary',
      title: 'Sidecar summary',
      workspace: '/tmp/project',
      model: 'deepseek-chat',
      providerId: 'zai-coding-plan',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      preview: 'cached preview line',
      turnCount: 3,
      messageCount: 7,
      createdAt: '2026-06-04T00:00:00.000Z',
      updatedAt: '2026-06-04T00:00:01.000Z'
    })

    expect(summary.preview).toBe('cached preview line')
    expect(summary.providerId).toBe('zai-coding-plan')
    expect(summary.turnCount).toBe(3)
    expect(summary.messageCount).toBe(7)
  })

  it('accepts thread todo contracts and events', () => {
    const todos = ThreadTodoListSchema.parse({
      threadId: 'thr_1',
      updatedAt: '2026-06-03T00:00:00.000Z',
      items: [{
        id: 'todo_1',
        content: 'Implement todo panel',
        status: 'in_progress',
        source: {
          kind: 'plan',
          planId: 'plan_1',
          relativePath: '.analytixsdd/plan/plan.md',
          ordinal: 0,
          contentHash: 'abc'
        },
        createdAt: '2026-06-03T00:00:00.000Z',
        updatedAt: '2026-06-03T00:00:00.000Z'
      }]
    })
    expect(todos.items[0]?.source?.kind).toBe('plan')
    expect(SetThreadTodosRequest.parse({
      todos: [{ content: 'Done', status: 'completed' }]
    }).todos[0]?.status).toBe('completed')
    expect(SetThreadTodosRequest.safeParse({
      todos: [
        { content: 'one', status: 'in_progress' },
        { content: 'two', status: 'in_progress' }
      ]
    }).success).toBe(false)
    const event = RuntimeEvent.parse({
      kind: 'todos_updated',
      seq: 2,
      timestamp: '2026-06-03T00:00:01.000Z',
      threadId: 'thr_1',
      todos
    })
    expect(event.kind).toBe('todos_updated')
  })

  it('rejects invalid start turn payloads', () => {
    const result = StartTurnRequest.safeParse({ prompt: '' })
    expect(result.success).toBe(false)
  })

  it('keeps start turn risk intent raise-only and rejects unknown fields', () => {
    expect(StartTurnRequest.parse({ prompt: 'Inspect evidence', riskIntent: 'case' }).riskIntent).toBe('case')
    expect(StartTurnRequest.safeParse({ prompt: 'Inspect evidence', riskIntent: 'general' }).success).toBe(false)
    expect(StartTurnRequest.safeParse({ prompt: 'Inspect evidence', caseId: 'case-spoof' }).success).toBe(false)
    expect(StartTurnRequest.safeParse({ prompt: 'Inspect evidence', publicationPolicy: 'case_evidence_gate' }).success).toBe(false)
    expect(StartTurnRequest.safeParse({ prompt: 'Inspect evidence', maxModelSteps: 10_001 }).success).toBe(false)
    expect(StartTurnRequest.safeParse({ prompt: 'Inspect evidence', maxModelSteps: 1.5 }).success).toBe(false)
    expect(StartTurnRequest.safeParse({
      prompt: 'Inspect evidence',
      fileReferences: [{ path: '/tmp/a', relativePath: 'a', name: 'a', supportStatus: 'verified' }]
    }).success).toBe(false)
  })

  it('keeps steer risk intent raise-only and closes nested request fields', () => {
    const parsed = SteerTurnRequest.parse({
      text: ' continue ',
      riskIntent: 'case',
      delivery: 'steer',
      attachmentIds: ['att-1'],
      fileReferences: [{ path: '/workspace/a', relativePath: 'a', name: 'a', kind: 'file' }]
    })
    expect(parsed.text).toBe('continue')
    expect(parsed.riskIntent).toBe('case')
    expect(SteerTurnRequest.safeParse({ text: 'continue', riskIntent: 'general' }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({ text: 'continue', caseId: 'case-spoof' }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({ text: 'continue', publicationPolicy: 'general_guidance' }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({ text: 'continue', delivery: 'message' }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({ text: 'continue', attachmentIds: [''] }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({
      text: 'continue',
      fileReferences: [{ path: '/workspace/a', relativePath: 'a', name: 'a', supportStatus: 'verified' }]
    }).success).toBe(false)
    expect(SteerTurnRequest.safeParse({ text: 'continue', attachmentIds: Array(4097).fill('att') }).success).toBe(false)
  })

  it('keeps interrupt request closed and discard boolean-only', () => {
    expect(InterruptTurnRequest.parse({}).discard).toBeUndefined()
    expect(InterruptTurnRequest.parse({ discard: true }).discard).toBe(true)
    expect(InterruptTurnRequest.safeParse({ discard: 'true' }).success).toBe(false)
    expect(InterruptTurnRequest.safeParse({ discard: true, reason: 'caller-owned' }).success).toBe(false)
  })

  it('accepts per-turn reasoning effort on start turn payloads', () => {
    const parsed = StartTurnRequest.parse({
      prompt: 'Compare the approaches',
      model: 'auto',
      providerId: 'zhipu-coding-plan',
      reasoningEffort: 'max'
    })
    expect(parsed.providerId).toBe('zhipu-coding-plan')
    expect(parsed.reasoningEffort).toBe('max')
  })

  it('accepts per-turn execution policy on start turn payloads', () => {
    const parsed = StartTurnRequest.parse({
      prompt: 'Inspect without changing files',
      approvalPolicy: 'on-request',
      sandboxMode: 'read-only'
    })
    expect(parsed.approvalPolicy).toBe('on-request')
    expect(parsed.sandboxMode).toBe('read-only')
  })

  it('accepts Kun-compatible workspace checkpoint ids on user turns', () => {
    const parsed = StartTurnRequest.parse({
      prompt: 'Edit after checkpoint',
      workspaceCheckpointId: 'gcp_1'
    })
    expect(parsed.workspaceCheckpointId).toBe('gcp_1')

    const user = TurnItem.parse({
      id: 'item_1_user',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'user',
      status: 'completed',
      createdAt: '2026-06-03T00:00:01.000Z',
      kind: 'user_message',
      text: 'Edit after checkpoint',
      delivery: 'steer',
      workspaceCheckpointId: 'gcp_1'
    })
    expect(user.kind).toBe('user_message')
    if (user.kind !== 'user_message') {
      throw new Error(`expected user_message, received ${user.kind}`)
    }
    expect(user.delivery).toBe('steer')
    expect(user.workspaceCheckpointId).toBe('gcp_1')
  })

  it('rejects private reasoning items and events at the public runtime contract', () => {
    const item = TurnItem.safeParse({
      id: 'item_reasoning',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'assistant',
      status: 'completed',
      createdAt: '2026-06-03T00:00:01.000Z',
      kind: 'assistant_reasoning',
      text: 'private chain of thought'
    })
    expect(item.success).toBe(false)

    const event = RuntimeEvent.safeParse({
      kind: 'assistant_reasoning_delta',
      seq: 2,
      timestamp: '2026-06-03T00:00:02.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      item: {
        id: 'item_reasoning',
        turnId: 'turn_1',
        threadId: 'thr_1',
        role: 'assistant',
        status: 'running',
        createdAt: '2026-06-03T00:00:01.000Z',
        kind: 'assistant_reasoning',
        text: 'private chain of thought'
      }
    })
    expect(event.success).toBe(false)
  })

  it('accepts only closed metadata-only public tool-result projections', () => {
    const withheld = {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'tool_output_private',
      messageKey: 'tool_output_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    } as const
    const hostStatus = {
      schemaVersion: 1,
      projectionKind: 'host_status',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'tool_completed',
      messageKey: 'tool_completed',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    } as const
    const outcomeUnknown = {
      schemaVersion: 1,
      projectionKind: 'host_status',
      disclosure: 'metadata_only',
      status: 'unknown',
      code: 'tool_outcome_unknown_after_restart',
      messageKey: 'tool_outcome_unknown',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    } as const

    expect(PublicToolResultProjectionV1.parse(withheld)).toEqual(withheld)
    expect(PublicToolResultProjectionV1.parse(hostStatus)).toEqual(hostStatus)
    expect(PublicToolResultProjectionV1.parse(outcomeUnknown)).toEqual(outcomeUnknown)
    expect(PublicToolResultProjectionV1.safeParse({ ...outcomeUnknown, status: 'cancelled' }).success).toBe(false)
    expect(PublicToolResultProjectionV1.safeParse({ ...outcomeUnknown, messageKey: 'tool_cancelled' }).success).toBe(false)
    expect(PublicToolResultProjectionV1.safeParse({ ...outcomeUnknown, code: 'tool_cancelled' }).success).toBe(false)
    expect(TurnItem.safeParse({
      id: 'item_tool_result',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-14T00:00:00.000Z',
      finishedAt: '2026-07-14T00:00:01.000Z',
      kind: 'tool_result',
      toolName: 'read',
      callId: 'call_1',
      toolKind: 'tool_call',
      output: withheld,
      isError: false
    }).success).toBe(true)
    expect(TurnItem.safeParse({
      id: 'item_tool_result',
      turnId: 'turn_1',
      threadId: 'thr_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-14T00:00:00.000Z',
      kind: 'tool_result',
      toolName: 'read',
      callId: 'call_1',
      toolKind: 'tool_call',
      output: withheld,
      isError: false,
      summary: 'PRIVATE_ROOT_SENTINEL',
      dataUrl: 'data:image/png;base64,PRIVATE_ROOT_SENTINEL'
    }).success).toBe(false)
  })

  it.each([
    { content: 'private stdout' },
    { dataUrl: 'data:image/png;base64,cHJpdmF0ZQ==' },
    { previewUrl: 'file:///private/preview.png' },
    { blob: 'private-bytes' },
    {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      status: 'completed',
      messageKey: 'tool_output_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false,
      safeToAnswer: true
    },
    {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      status: 'completed',
      messageKey: 'tool_output_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: true
    },
    {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      status: 'failed',
      code: '6222020202020202020',
      messageKey: 'tool_output_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    },
    {
      schemaVersion: 1,
      projectionKind: 'mcp_diagnostic',
      disclosure: 'metadata_only',
      status: 'failed',
      code: 'mcp_request_rejected',
      messageKey: 'mcp_request_rejected',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false,
      rpcError: { code: 6222020202020, class: 'invalid_params', dataPresent: true }
    },
    {
      schemaVersion: 1,
      projectionKind: 'plan_status',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'plan_updated',
      messageKey: 'plan_updated',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false,
      plan: {
        planId: 'plan-escape',
        relativePath: '../../private.md',
        operation: 'draft',
        contentHash: 'a'.repeat(64),
        byteSize: 1,
        savedAt: 'not-a-timestamp'
      }
    }
  ])('rejects raw, media-bearing, open, or authority-claiming public tool output', (output) => {
    expect(PublicToolResultProjectionV1.safeParse(output).success).toBe(false)
  })

  it('keeps public tool calls and case-source results metadata-only without authority identifiers', () => {
    const argumentsProjection = {
      schemaVersion: 1,
      projectionKind: 'withheld',
      disclosure: 'metadata_only',
      messageKey: 'tool_arguments_withheld',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    } as const
    const output = {
      schemaVersion: 1,
      projectionKind: 'case_source_status',
      disclosure: 'metadata_only',
      status: 'completed',
      code: 'case_source_result_private',
      messageKey: 'case_source_private',
      privatePayloadWithheld: true,
      factAnswerAllowed: false,
      evidenceAuthority: false
    } as const
    const callItem = {
      id: 'item_case_tool_call',
      turnId: 'turn_case_1',
      threadId: 'thread_case_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-14T00:00:00.000Z',
      kind: 'tool_call',
      toolName: 'mcp__analytix-fund-analysis__query_transactions',
      callId: 'call_case_1',
      toolKind: 'tool_call',
      arguments: argumentsProjection
    } as const
    const resultItem = {
      id: 'item_case_tool_result',
      turnId: 'turn_case_1',
      threadId: 'thread_case_1',
      role: 'tool',
      status: 'completed',
      createdAt: '2026-07-14T00:00:00.000Z',
      finishedAt: '2026-07-14T00:00:01.000Z',
      kind: 'tool_result',
      toolName: callItem.toolName,
      callId: callItem.callId,
      toolKind: 'tool_call',
      output,
      isError: false
    } as const
    expect(PublicToolCallArgumentsProjectionV1.parse(argumentsProjection)).toEqual(argumentsProjection)
    expect(TurnItem.safeParse(callItem).success).toBe(true)
    expect(TurnItem.safeParse(resultItem).success).toBe(true)
    expect(TurnItem.safeParse({ ...callItem, contextDigest: 'a'.repeat(64) }).success).toBe(false)
    expect(TurnItem.safeParse({ ...resultItem, executionGrantId: 'grant_private' }).success).toBe(false)
    expect(PublicToolResultProjectionV1.safeParse({
      ...output,
      caseOutcome: {
        contextDigest: 'a'.repeat(64),
        contextEpoch: 7,
        executionGrantId: 'grant_private'
      }
    }).success).toBe(false)
  })

  it('accepts the exact Go case-source projection fixture and rejects added authority fields', async () => {
    const fixture = JSON.parse(await readFile(
      new URL('../src/conformance/fixtures/go-case-source-public-projection-v1.json', import.meta.url),
      'utf8'
    )) as {
      projections: Record<'completed' | 'failed', Record<string, unknown>>
    }
    expect(PublicToolResultProjectionV1.parse(fixture.projections.completed))
      .toEqual(fixture.projections.completed)
    expect(PublicToolResultProjectionV1.parse(fixture.projections.failed))
      .toEqual(fixture.projections.failed)
    for (const projection of Object.values(fixture.projections)) {
      expect(PublicToolResultProjectionV1.safeParse({
        ...projection,
        AuthorityRef: 'PRIVATE_AUTHORITY_REF'
      }).success).toBe(false)
      expect(PublicToolResultProjectionV1.safeParse({
        ...projection,
        caseSourceBindingProof: {
          version: 1,
          purpose: 'analytix.case-source-result-binding/v1',
          digest: 'a'.repeat(64)
        }
      }).success).toBe(false)
      expect(JSON.stringify(projection)).not.toMatch(
        /caseOutcome|caseSourceBindingProof|analytix\.case-source-result-binding|toolCallId|contextDigest|executionGrantId|contextEpoch|datasetSnapshotId|transportStatus|semanticStatus|safeToAnswer|evidenceReceipt|AuthorityRef|6222020202020202020|\/private\/case\.sqlite|SELECT \* FROM private_case|PRIVATE_REVERSE_MAP|PRIVATE_PROVIDER_BODY/
      )
    }
  })

  it('keeps v3 and v4 records audit-only and admits only v5 live records', () => {
    const record = acceptedFinalBoundaryV3Fixture()
    expect(AcceptedFinalRecordV3Schema.parse(record)).toEqual(record)
    expect(BoundaryAcceptedFinalRecordV3Schema.parse(record)).toEqual(record)
    expect(AcceptedFinalAuditRecordSchema.parse(record)).toEqual(record)
    expect(AcceptedFinalLiveRecordSchema.safeParse(record).success).toBe(false)

    expect(AcceptedFinalRecordV3Schema.safeParse({ ...record, schemaVersion: 2 }).success).toBe(false)
    expect(AcceptedFinalRecordV3Schema.safeParse({ ...record, providerSaysSafe: true }).success).toBe(false)
    expect(AcceptedFinalRecordV3Schema.safeParse({
      ...record,
      variant: 'EvidenceBackedAnswer'
    }).success).toBe(false)
    expect(AcceptedFinalRecordV3Schema.safeParse({
      ...record,
      publicationSnapshotProofDigest: acceptedFinalSha('1')
    }).success).toBe(false)
    const historicalFact = {
      ...record,
      variant: 'EvidenceBackedAnswer',
      publicationSnapshotProofDigest: acceptedFinalSha('1')
    }
    expect(AcceptedFinalRecordV3Schema.safeParse(historicalFact).success).toBe(true)
    expect(AcceptedFinalAuditRecordSchema.safeParse(historicalFact).success).toBe(true)
    expect(AcceptedFinalLiveRecordSchema.safeParse(historicalFact).success).toBe(false)

    const witnessedFact = acceptedFinalV4Fixture()
    expect(WitnessedFactAcceptedFinalRecordV4Schema.parse(witnessedFact)).toEqual(witnessedFact)
    expect(AcceptedFinalAuditRecordSchema.parse(witnessedFact)).toEqual(witnessedFact)
    expect(AcceptedFinalLiveRecordSchema.safeParse(witnessedFact).success).toBe(false)

    const liveBoundary = acceptedFinalBoundaryV5Fixture()
    expect(BoundaryAcceptedFinalRecordV5Schema.parse(liveBoundary)).toEqual(liveBoundary)
    expect(AcceptedFinalLiveRecordSchema.parse(liveBoundary)).toEqual(liveBoundary)
    const liveFact = acceptedFinalV5FactFixture()
    expect(WitnessedFactAcceptedFinalRecordV5Schema.parse(liveFact)).toEqual(liveFact)
    expect(AcceptedFinalLiveRecordSchema.parse(liveFact)).toEqual(liveFact)
    expect(AcceptedFinalLiveRecordSchema.safeParse({
      ...liveFact,
      factFinalWitnessAdmission: undefined
    }).success).toBe(false)
    expect(AcceptedFinalLiveRecordSchema.safeParse({
      ...liveFact,
      variant: 'SourceUnavailableAnswer'
    }).success).toBe(false)
    expect(AcceptedFinalLiveRecordSchema.safeParse({
      ...liveFact,
      factFinalWitnessAdmission: {
        ...liveFact.factFinalWitnessAdmission,
        contextDigest: acceptedFinalSha('0')
      }
    }).success).toBe(false)
    expect(AcceptedFinalLiveRecordSchema.safeParse({
      ...liveFact,
      factFinalWitnessAdmission: {
        ...liveFact.factFinalWitnessAdmission,
        witnessBinding: {
          ...liveFact.factFinalWitnessAdmission.witnessBinding,
          providerSaysSafe: true
        }
      }
    }).success).toBe(false)
    expect(AcceptedFinalLiveRecordSchema.safeParse({
      ...liveFact,
      factFinalWitnessAdmission: {
        ...liveFact.factFinalWitnessAdmission,
        witnessBinding: {
          ...liveFact.factFinalWitnessAdmission.witnessBinding,
          authorityKeyId: acceptedFinalSha('0')
        }
      }
    }).success).toBe(false)
  })

  it('accepts the exact Go V2 witness admission and rejects every mixed or malformed authority field', async () => {
    const record = await goAcceptedFinalV5V2Fixture() as Record<string, any>
    const admission = record.factFinalWitnessAdmission as Record<string, any>
    expect(FactFinalWitnessAdmissionV2Schema.parse(admission)).toEqual(admission)
    expect(AcceptedFinalLiveRecordSchema.parse(record)).toEqual(record)
    const v2AsHistoricalV4: Record<string, any> = {
      ...record,
      schemaVersion: 4,
      rendererVersion: 'analytix.host-final-renderer/v1',
      finalGateVersion: 'analytix.final-evidence-gate/v3'
    }
    delete v2AsHistoricalV4.publicView
    delete v2AsHistoricalV4.publicViewDigest
    expect(WitnessedFactAcceptedFinalRecordV4Schema.safeParse(v2AsHistoricalV4).success).toBe(false)

    const historicalV1 = acceptedFinalV4Fixture().factFinalWitnessAdmission
    expect(FactFinalWitnessAdmissionV1Schema.parse(historicalV1)).toEqual(historicalV1)
    expect(FactFinalWitnessAdmissionV1Schema.safeParse({
      ...historicalV1,
      datasetSnapshotIndexDigest: admission.datasetSnapshotIndexDigest
    }).success).toBe(false)

    const v2RequiredFields = [
      'datasetSnapshotIndexDigest',
      'datasetSnapshotCount',
      'selectedDatasetSnapshotIndexDigest',
      'selectedDatasetSnapshotIndexGeneration',
      'selectedDatasetSnapshotRecordDigest',
      'datasetSnapshotManifestDigest',
      'fundsProducerContentId',
      'fundsProducerContentManifestSha256'
    ]
    for (const required of v2RequiredFields) {
      const missing = { ...admission }
      delete missing[required]
      expect(FactFinalWitnessAdmissionV2Schema.safeParse(missing).success).toBe(false)
    }
    for (const digestField of [
      'contextDigest',
      'sourceManifestHash',
      'envelopeDigest',
      'renderedTextSha256',
      'publicationSnapshotProofDigest',
      'registryStateDigest',
      'evidenceReceiptIdsDigest',
      'evidenceAuthorityBundleDigest',
      'evidenceRegistryIndexDigest',
      'selectedRegistryIndexDigest',
      'selectedRegistryCapsuleDigest',
      'datasetSnapshotIndexDigest',
      'selectedDatasetSnapshotIndexDigest',
      'selectedDatasetSnapshotRecordDigest',
      'datasetSnapshotManifestDigest',
      'fundsProducerContentManifestSha256',
      'admissionDigest'
    ]) {
      expect(FactFinalWitnessAdmissionV2Schema.safeParse({
        ...admission,
        [digestField]: 'not-a-sha256'
      }).success).toBe(false)
    }
    for (const bindingDigestField of [
      'installationId',
      'enrollmentId',
      'authorityKeyId',
      'witnessKeyId',
      'bundleRecordDigest',
      'observeRequestDigest',
      'checkpointDigest',
      'observationDigest',
      'bindingDigest'
    ]) {
      expect(FactFinalWitnessAdmissionV2Schema.safeParse({
        ...admission,
        witnessBinding: { ...admission.witnessBinding, [bindingDigestField]: 'not-a-sha256' }
      }).success).toBe(false)
    }
    for (const countField of [
      'registrySequence',
      'evidenceReceiptCount',
      'evidenceAuthorityBundleGeneration',
      'evidenceRegistryCount',
      'selectedRegistryIndexGeneration',
      'datasetSnapshotCount',
      'selectedDatasetSnapshotIndexGeneration'
    ]) {
      expect(FactFinalWitnessAdmissionV2Schema.safeParse({ ...admission, [countField]: 0 }).success).toBe(false)
      expect(FactFinalWitnessAdmissionV2Schema.safeParse({
        ...admission,
        [countField]: Number.MAX_SAFE_INTEGER + 1
      }).success).toBe(false)
    }
    for (const bundleGeneration of [0, Number.MAX_SAFE_INTEGER + 1]) {
      expect(FactFinalWitnessAdmissionV2Schema.safeParse({
        ...admission,
        witnessBinding: { ...admission.witnessBinding, bundleGeneration }
      }).success).toBe(false)
    }
    for (const hostile of [
      { ...admission, schemaVersion: 1, purpose: 'analytix.fact-final-witness-admission/v1' },
      { ...admission, schemaVersion: 2, purpose: 'analytix.fact-final-witness-admission/v1' },
      { ...historicalV1, schemaVersion: 2, purpose: 'analytix.fact-final-witness-admission/v2' },
      { ...admission, selectedRegistryIndexGeneration: admission.evidenceRegistryCount + 1 },
      { ...admission, selectedDatasetSnapshotIndexGeneration: admission.datasetSnapshotCount + 1 },
      { ...admission, datasetSnapshotId: 'snapshot-not-dsv2' },
      { ...admission, datasetSnapshotId: `dsv2_${'a'.repeat(64)}` },
      { ...admission, fundsProducerContentId: `fpc2_${'a'.repeat(64)}` },
      { ...admission, fundsProducerContentId: `fpc1_${'z'.repeat(64)}` },
      { ...admission, publicationSnapshotProof: { private: true } },
      { ...admission, providerSaysSafe: true },
      { ...admission, witnessBinding: { ...admission.witnessBinding, providerSaysSafe: true } }
    ]) {
      expect(FactFinalWitnessAdmissionV2Schema.safeParse(hostile).success).toBe(false)
    }

    for (const mutate of [
      (candidate: Record<string, any>) => { candidate.contextDigest = acceptedFinalSha('0') },
      (candidate: Record<string, any>) => { candidate.datasetSnapshotId = `dsv2_${'a'.repeat(64)}` },
      (candidate: Record<string, any>) => { candidate.envelopeDigest = acceptedFinalSha('0') },
      (candidate: Record<string, any>) => { candidate.renderedTextSha256 = acceptedFinalSha('0') },
      (candidate: Record<string, any>) => { candidate.publicationSnapshotProofDigest = acceptedFinalSha('0') },
      (candidate: Record<string, any>) => { candidate.registrySequence += 1 },
      (candidate: Record<string, any>) => { candidate.registryStateDigest = acceptedFinalSha('0') },
      (candidate: Record<string, any>) => { candidate.evidenceReceiptCount += 1 },
      (candidate: Record<string, any>) => {
        candidate.witnessBinding = { ...candidate.witnessBinding, authorityKeyId: acceptedFinalSha('0') }
      }
    ]) {
      const detached = structuredClone(admission) as Record<string, any>
      mutate(detached)
      expect(AcceptedFinalLiveRecordSchema.safeParse({
        ...record,
        factFinalWitnessAdmission: detached
      }).success).toBe(false)
    }

    for (const privateField of [
      'publicationSnapshotProof',
      'securityContext',
      'publicationIntent',
      'storeDigest',
      'registryHead',
      'rawProviderBody',
      'sourceExactValue',
      'privatePII'
    ]) {
      expect(AcceptedFinalLiveRecordSchema.safeParse({
        ...record,
        [privateField]: { privateSentinel: true }
      }).success).toBe(false)
    }
  })

  it('rejects a witnessed v4 final that predates admission below millisecond precision', () => {
    const base = acceptedFinalV4Fixture()
    const witnessedFact = acceptedFinalV4Fixture({
      acceptedAt: '2026-07-11T01:02:03.0000001Z',
      factFinalWitnessAdmission: {
        ...base.factFinalWitnessAdmission,
        admittedAt: '2026-07-11T01:02:03.0000009Z'
      }
    })
    expect(WitnessedFactAcceptedFinalRecordV4Schema.safeParse(witnessedFact).success).toBe(false)
  })

  it('accepts only a strict masked accepted-final public view', () => {
    const record = acceptedFinalBoundaryV3Fixture()
    const view = acceptedFinalPublicViewV1Fixture(record)
    expect(AcceptedFinalPublicViewV1Schema.parse(view)).toEqual(view)
    expect(AcceptedFinalPublicViewV1Schema.safeParse({ ...view, receiptIds: ['private-receipt'] }).success).toBe(false)
    expect(AcceptedFinalPublicViewV1Schema.safeParse({ ...view, blockerCode: 'unsafe blocker detail' }).success).toBe(false)
    expect(AcceptedFinalPublicViewV1Schema.safeParse({
      ...view,
      receiptMetadata: { ...view.receiptMetadata, count: 1 }
    }).success).toBe(false)
    expect(AcceptedFinalPublicViewV1Schema.safeParse({
      ...view,
      variant: 'VerifiedNoHitAnswer',
      coverageStatus: 'complete'
    }).success).toBe(false)
  })

  it('binds fact-bearing public metadata to the signed record without raw receipt ids', () => {
    const record = acceptedFinalV5FactFixture()
    const view = acceptedFinalPublicViewV2Fixture(record)
    expect(AcceptedFinalPublicViewV2Schema.parse(view)).toEqual(view)
    expect(acceptedFinalPublicViewMatchesRecord(record, view)).toBe(true)
    expect(JSON.stringify(view)).not.toMatch(/receiptId|sourceRecord|queryRange|reasoning|accountId/)
    expect(acceptedFinalPublicViewMatchesRecord(record, {
      ...view,
      acceptedFinalDigest: acceptedFinalSha('6')
    })).toBe(false)
    expect(acceptedFinalPublicViewMatchesRecord(record, {
      ...view,
      receiptMetadata: {
        ...view.receiptMetadata,
        count: 1,
        citations: view.receiptMetadata.citations.slice(0, 1)
      }
    })).toBe(false)
    const historicalV3Fact = acceptedFinalV3Fixture({
      variant: 'EvidenceBackedAnswer',
      terminalReason: 'success',
      publicationSnapshotProofDigest: acceptedFinalSha('1')
    })
    expect(acceptedFinalPublicViewMatchesRecord(historicalV3Fact, view)).toBe(false)
  })

  it('accepts a strict versioned V3 generic view without changing the V2 private expansion', () => {
    const record = acceptedFinalV5FactFixture()
    const v2 = acceptedFinalPublicViewV2Fixture(record)
    const v3 = acceptedFinalPublicViewV3Fixture(record)
    expect(AcceptedFinalPublicViewV2Schema.parse(v2)).toEqual(v2)
    expect(AcceptedFinalPublicViewV3Schema.parse(v3)).toEqual(v3)
    expect(acceptedFinalPublicViewsMatch(v3, structuredClone(v3))).toBe(true)
    expect(acceptedFinalPublicViewsMatch(v2, structuredClone(v2))).toBe(false)
    expect(v3.receiptMetadata).toEqual(v2.receiptMetadata)
    expect(v3.checkedScopeDigest).toBe(v2.checkedScopeDigest)
    expect(v3.publicationState).toBe('accepted')
    expect(Object.keys(v3).sort()).toEqual([
      'schemaVersion', 'acceptedFinalDigest', 'publicationState', 'variant', 'terminalReason',
      'blockerCode', 'coverageStatus', 'checkedScopeDigest', 'missingScopeCount', 'claimCount',
      'claimTypes', 'receiptMetadata', 'noHitWording', 'acceptedAt'
    ].sort())
    expect(Object.keys(v3.receiptMetadata).sort()).toEqual([
      'projection', 'count', 'setDigest', 'citations'
    ].sort())
    expect(v3.receiptMetadata.citations.map((citation) => Object.keys(citation).sort())).toEqual(
      v3.receiptMetadata.citations.map(() => ['handle', 'label'])
    )
    expect(v3).toMatchObject({
      schemaVersion: 3,
      acceptedFinalDigest: record.recordDigest,
      publicationState: 'accepted',
      variant: record.publicView.variant,
      terminalReason: record.publicView.terminalReason,
      blockerCode: record.publicView.blockerCode,
      coverageStatus: record.publicView.coverageStatus,
      checkedScopeDigest: record.publicView.checkedScopeDigest,
      missingScopeCount: record.publicView.missingScopeCount,
      claimCount: record.publicView.claimCount,
      claimTypes: record.publicView.claimTypes,
      receiptMetadata: record.publicView.receiptMetadata,
      noHitWording: record.publicView.noHitWording,
      acceptedAt: record.publicView.acceptedAt
    })
    const serializedV3 = JSON.stringify(v3)
    for (const forbiddenProperty of [
      'publicViewDigest', 'envelopeDigest', 'contextDigest', 'contextEpoch', 'datasetSnapshotId',
      'envelopeIssuedAt', 'threadId', 'turnId', 'renderedTextSha256', 'acceptedFinal',
      'caseBindingHash', 'factFinalWitnessAdmission', 'publicationSnapshotProof',
      'publicationSnapshotProofDigest', 'registryHead', 'publicationIntent', 'storeDigest'
    ]) {
      expect(serializedV3).not.toContain(`"${forbiddenProperty}"`)
    }
    for (const forbiddenValue of [
      record.publicViewDigest,
      record.envelopeDigest,
      record.contextDigest,
      record.datasetSnapshotId,
      record.threadId,
      record.turnId,
      record.renderedTextSha256,
      record.privateRecordDigest
    ]) {
      expect(serializedV3).not.toContain(forbiddenValue)
    }
    for (const hostile of [
      { ...v3, publicationState: 'pending' },
      (() => {
        const { publicationState: _publicationState, ...missingPublicationState } = v3
        return missingPublicationState
      })(),
      { ...v3, publicViewDigest: record.publicViewDigest },
      { ...v3, threadId: record.threadId },
      { ...v3, turnId: record.turnId },
      { ...v3, renderedTextSha256: record.renderedTextSha256 },
      { ...v3, envelopeIssuedAt: record.publicView.envelopeIssuedAt },
      { ...v3, envelopeDigest: record.envelopeDigest },
      { ...v3, contextDigest: record.contextDigest },
      { ...v3, contextEpoch: record.contextEpoch },
      { ...v3, datasetSnapshotId: record.datasetSnapshotId },
      { ...v3, schemaVersion: 2 },
      { ...v2, schemaVersion: 3 },
      { ...v3, providerSaysSafe: true }
    ]) {
      expect(AcceptedFinalPublicViewV3Schema.safeParse(hostile).success).toBe(false)
    }
  })

  it('parses the closed V3 shape but rejects standalone accepted item and turn authority', () => {
    const record = acceptedFinalBoundaryV5Fixture()
    const view = acceptedFinalPublicViewV3Fixture(record)
    const item = {
      id: 'item_turn_case_assistant',
      turnId: record.turnId,
      threadId: record.threadId,
      role: 'assistant' as const,
      status: 'completed' as const,
      createdAt: '2026-07-11T01:02:02.000Z',
      finishedAt: view.acceptedAt,
      kind: 'assistant_text' as const,
      text: '当前案件来源未通过核验。',
      acceptedFinalView: view
    }
    const turn = {
      id: record.turnId,
      threadId: record.threadId,
      status: 'completed' as const,
      prompt: '核验当前案件资金来源',
      createdAt: '2026-07-11T01:02:01.000Z',
      finishedAt: view.acceptedAt,
      items: [item],
      acceptedFinalView: view
    }
    expect(AcceptedFinalPublicViewV3Schema.parse(view)).toEqual(view)
    expect(TurnItem.safeParse(item).success).toBe(false)
    expect(TurnSchema.safeParse(turn).success).toBe(false)
    expect(TurnSchema.safeParse({ ...turn, acceptedFinal: record }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      items: [{ ...item, acceptedFinal: record }]
    }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      items: [{ ...item, turnId: 'turn_other' }]
    }).success).toBe(false)
  })

  it('keeps a private V5+V2 pair outside the generic public turn schema', async () => {
    const record = await goAcceptedFinalV5V2Fixture()
    const view = acceptedFinalPublicViewV2Fixture(record)
    const item = acceptedAssistantFixture(record, view)
    const turn = {
      id: record.turnId,
      threadId: record.threadId,
      status: 'completed' as const,
      prompt: '核验当前案件资金来源',
      createdAt: '2026-07-11T01:02:01.000Z',
      finishedAt: record.acceptedAt,
      items: [item],
      acceptedFinal: record,
      acceptedFinalView: view
    }
    expect(TurnSchema.safeParse(turn).success).toBe(false)
    expect(TurnItem.safeParse(item).success).toBe(false)
    expect(PrivateAcceptedFinalAssistantTextTurnItem.parse(item).kind).toBe('assistant_text')

    expect(TurnSchema.safeParse({ ...turn, acceptedFinal: undefined }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      items: [{ ...item, acceptedFinal: undefined }]
    }).success).toBe(false)
    expect(TurnSchema.safeParse({ ...turn, acceptedFinalView: undefined }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      items: [{ ...item, acceptedFinalView: undefined }]
    }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      acceptedFinalView: { ...view, contextDigest: acceptedFinalSha('2') }
    }).success).toBe(false)
    const synchronouslyTamperedView = {
      ...view,
      blockerCode: 'tampered_blocker'
    }
    expect(TurnSchema.safeParse({
      ...turn,
      acceptedFinalView: synchronouslyTamperedView,
      items: [{ ...item, acceptedFinalView: synchronouslyTamperedView }]
    }).success).toBe(false)
    expect(TurnSchema.safeParse({
      ...turn,
      items: [{
        ...item,
        acceptedFinal: { ...record, contextDigest: acceptedFinalSha('2') }
      }]
    }).success).toBe(false)
    expect(TurnItem.safeParse({
      ...item,
      acceptedFinal: { ...record, turnId: 'turn_other' }
    }).success).toBe(false)

    expect(TurnSchema.safeParse({
      id: 'turn_plain',
      threadId: 'thr_plain',
      status: 'completed',
      prompt: 'ordinary request',
      createdAt: '2026-07-11T01:02:01.000Z',
      finishedAt: '2026-07-11T01:02:03.000Z',
      items: [{
        id: 'item_plain',
        turnId: 'turn_plain',
        threadId: 'thr_plain',
        role: 'assistant',
        status: 'completed',
        createdAt: '2026-07-11T01:02:02.000Z',
        finishedAt: '2026-07-11T01:02:03.000Z',
        kind: 'assistant_text',
        text: 'ordinary answer'
      }]
    }).success).toBe(true)
  })

  it('binds private accepted assistant audit events while generic runtime replay rejects the full V5+V2 record', async () => {
    const record = await goAcceptedFinalV5V2Fixture()
    const item = acceptedAssistantFixture(record)
    const completedItemEvent = {
      kind: 'item_completed' as const,
      seq: 10,
      timestamp: record.acceptedAt,
      threadId: record.threadId,
      turnId: record.turnId,
      itemId: item.id,
      item,
      acceptedFinalDigest: record.recordDigest,
      publicationCommitId: record.recordDigest,
      publicationEventId: acceptedFinalSha('3'),
      publicationSlot: 'assistant-final',
      publicationPayloadDigest: acceptedFinalSha('4')
    }
    expect(RuntimeEvent.safeParse(completedItemEvent).success).toBe(false)
    expect(AcceptedFinalItemCompletedEventV1Schema.parse(completedItemEvent)).toEqual(completedItemEvent)
    expect(AcceptedFinalItemCompletedEventV1Schema.safeParse({
      ...completedItemEvent,
      providerSaysSafe: true
    }).success).toBe(false)
    expect(AcceptedFinalItemCompletedEventV1Schema.safeParse({
      ...completedItemEvent,
      kind: 'assistant_text_delta'
    }).success).toBe(false)
    expect(AcceptedFinalItemCompletedEventV1Schema.safeParse({
      ...completedItemEvent,
      acceptedFinalDigest: acceptedFinalSha('5')
    }).success).toBe(false)
    expect(AcceptedFinalItemCompletedEventV1Schema.safeParse({
      ...completedItemEvent,
      publicationPayloadDigest: undefined
    }).success).toBe(false)

    const terminalEvent = {
      kind: 'turn_completed' as const,
      seq: 11,
      timestamp: record.acceptedAt,
      threadId: record.threadId,
      turnId: record.turnId,
      status: 'completed',
      acceptedFinalDigest: record.recordDigest,
      terminalReason: record.terminalReason
    }
    expect(RuntimeEvent.parse(terminalEvent).kind).toBe('turn_completed')
    expect(RuntimeEvent.safeParse({ ...terminalEvent, terminalReason: undefined }).success).toBe(false)
    expect(RuntimeEvent.safeParse({ ...terminalEvent, status: 'failed' }).success).toBe(false)

    expect(RuntimeEvent.safeParse({
      kind: 'turn_completed',
      seq: 12,
      timestamp: record.acceptedAt,
      threadId: 'thr_plain',
      turnId: 'turn_plain',
      status: 'completed'
    }).success).toBe(true)
  })

  it('keeps accepted-final delivery batches strict, contiguous, and authority-bound', () => {
    const record = acceptedFinalBoundaryV5Fixture()
    const item = acceptedAssistantFixture(record) as Record<string, any>
    item.acceptedFinalView = acceptedFinalPublicViewV3Fixture(record)
    delete item.acceptedFinal
    const commit = record.recordDigest
    const timestamp = record.acceptedAt
    const batchId = acceptedFinalSha('6')
    const manifest = acceptedFinalSha('7')
    const events = [
      {
        kind: 'item_completed' as const,
        seq: 20,
        timestamp,
        threadId: record.threadId,
        turnId: record.turnId,
        itemId: item.id,
        item,
        acceptedFinalDigest: commit,
        publicationCommitId: commit,
        publicationEventId: acceptedFinalSha('1'),
        publicationSlot: 'assistant-final' as const,
        publicationPayloadDigest: acceptedFinalSha('2')
      },
      {
        kind: 'usage' as const,
        seq: 21,
        timestamp,
        threadId: record.threadId,
        turnId: record.turnId,
        model: 'deepseek-chat',
        usage: emptyUsageSnapshot(),
        cacheDiagnostics: {},
        usageFinalStatus: 'completed' as const,
        acceptedFinalDigest: commit,
        publicationCommitId: commit,
        publicationEventId: acceptedFinalSha('3'),
        publicationSlot: 'usage' as const,
        publicationPayloadDigest: acceptedFinalSha('4')
      },
      {
        kind: 'turn_completed' as const,
        seq: 22,
        timestamp,
        threadId: record.threadId,
        turnId: record.turnId,
        status: 'completed' as const,
        terminalReason: record.terminalReason,
        acceptedFinalDigest: commit,
        publicationCommitId: commit,
        publicationEventId: acceptedFinalSha('5'),
        publicationSlot: 'terminal' as const,
        publicationPayloadDigest: acceptedFinalSha('8')
      }
    ] as const
    const publicationAuthority = {
      schemaVersion: 'accepted-final-delivery-seal.v1' as const,
      purpose: 'analytix.accepted-final-delivery-seal/v1' as const,
      sealId: acceptedFinalSha('9'),
      threadId: record.threadId,
      turnId: record.turnId,
      publicationCommitId: commit,
      acceptedFinalDispositionDigest: acceptedFinalSha('a'),
      terminalDispositionId: acceptedFinalSha('b'),
      eventManifestDigest: manifest,
      sequencedEventsDigest: acceptedFinalSha('c'),
      batchId,
      firstSeq: 20,
      lastSeq: 22,
      timestamp,
      authorityAlgorithm: 'Ed25519' as const,
      authorityKeyId: record.authorityKeyId,
      authorityPublicKey: record.authorityPublicKey,
      authoritySignature: record.authoritySignature
    }
    const batch = {
      schemaVersion: 2 as const,
      purpose: 'analytix.accepted-final-delivery-batch/v2' as const,
      kind: 'accepted_final_batch' as const,
      batchId,
      threadId: record.threadId,
      turnId: record.turnId,
      seq: 22,
      firstSeq: 20,
      lastSeq: 22,
      timestamp,
      publicationCommitId: commit,
      eventManifestDigest: manifest,
      publicationAuthority,
      events
    }

    expect(AcceptedFinalItemCompletedEventV3Schema.parse(events[0])).toEqual(events[0])
    expect(RuntimeEvent.safeParse(events[0]).success).toBe(false)
    expect(TurnItem.safeParse(item).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV2Schema.parse(batch)).toEqual(batch)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse({ ...batch, providerSaysSafe: true }).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse({
      ...batch,
      publicationAuthority: { ...publicationAuthority, rawPrivateKey: 'secret' }
    }).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse({
      ...batch,
      events: [events[0], { ...events[1], seq: 23 }, events[2]]
    }).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse({
      ...batch,
      publicationAuthority: { ...publicationAuthority, batchId: acceptedFinalSha('d') }
    }).success).toBe(false)

    const historicalBatch = acceptedFinalDeliveryAuditFixtureV1('success')
    const historicalAssistant = historicalBatch.events[0].item as Record<string, any>
    const currentAssistant = batch.events[0].item as Record<string, any>
    expect(historicalAssistant.acceptedFinal).toBeDefined()
    expect(historicalAssistant.acceptedFinalView.schemaVersion).toBe(2)
    expect(currentAssistant.acceptedFinal).toBeUndefined()
    expect(currentAssistant.acceptedFinalView.schemaVersion).toBe(3)
    expect(historicalBatch.publicationAuthority.schemaVersion).toBe('accepted-final-delivery-seal.v1')
    expect(batch.publicationAuthority.schemaVersion).toBe('accepted-final-delivery-seal.v1')
    expect(AcceptedFinalDeliveryBatchV1Schema.parse(historicalBatch)).toEqual(historicalBatch)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(historicalBatch).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse(batch).success).toBe(false)
    const v1WrapperWithV3Event = structuredClone(batch) as Record<string, any>
    v1WrapperWithV3Event.schemaVersion = 1
    v1WrapperWithV3Event.purpose = 'analytix.accepted-final-delivery-batch/v1'
    expect(v1WrapperWithV3Event.events[0].item.acceptedFinal).toBeUndefined()
    expect(v1WrapperWithV3Event.events[0].item.acceptedFinalView.schemaVersion).toBe(3)
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse(v1WrapperWithV3Event).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(v1WrapperWithV3Event).success).toBe(false)
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse({
      ...historicalBatch,
      events: [{ ...historicalBatch.events[0], privateWireExpansion: true }, ...historicalBatch.events.slice(1)]
    }).success).toBe(false)
    const historicalPrivateTerminalMismatch = structuredClone(historicalBatch) as Record<string, any>
    historicalPrivateTerminalMismatch.events[0].item.acceptedFinal.terminalReason = 'provider_failure'
    expect(historicalPrivateTerminalMismatch.events[0].item.acceptedFinalView.terminalReason).toBe('success')
    expect(AcceptedFinalDeliveryBatchV1Schema.safeParse(historicalPrivateTerminalMismatch).success).toBe(false)
  })

  it.each<AcceptedFinalTerminalReason>([
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
  ])('closes the accepted-final delivery profile for %s', (reason) => {
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(
      acceptedFinalDeliveryContractFixtureV2(reason)
    ).success).toBe(true)
  })

  it('accepts sealed V3 but rejects a complete Go-issued V5+V2 record from generic delivery replay', async () => {
    const batch = structuredClone(acceptedFinalDeliveryContractFixtureV2('source_unavailable'))
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(batch).success).toBe(true)

    const record = await goAcceptedFinalV5V2Fixture()
    expect(record.factFinalWitnessAdmission).toMatchObject({
      schemaVersion: 2,
      purpose: 'analytix.fact-final-witness-admission/v2'
    })
    expect(record.publicationSnapshotProofDigest).toMatch(/^[a-f0-9]{64}$/)
    const leaked = structuredClone(batch)
    ;(leaked.events[0].item as Record<string, unknown>).acceptedFinal = record
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(leaked).success).toBe(false)
    const mixed = structuredClone(batch)
    ;((mixed.events[0].item as Record<string, any>).acceptedFinalView as Record<string, unknown>).contextDigest =
      record.contextDigest
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(mixed).success).toBe(false)
  })

  it('rejects cross-slot accepted-final semantic mutations', () => {
    const mutate = (
      reason: AcceptedFinalTerminalReason,
      mutation: (events: Array<Record<string, unknown>>) => void
    ): void => {
      const batch = structuredClone(acceptedFinalDeliveryContractFixtureV2(reason))
      mutation(batch.events)
      expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(batch).success).toBe(false)
    }

    mutate('provider_failure', (events) => { events[2].usageFinalStatus = 'completed' })
    mutate('provider_failure', (events) => { events[3].kind = 'turn_completed' })
    mutate('provider_failure', (events) => {
      const item = events[1].item as Record<string, unknown>
      item.acceptedFinalDigest = acceptedFinalSha('0')
    })
    mutate('provider_failure', (events) => {
      const item = events[1].item as Record<string, unknown>
      item.status = 'completed'
    })
    mutate('provider_failure', (events) => { events[3].itemId = 'item_detached' })
    mutate('provider_failure', (events) => { events[3].message = 'detached message' })
    mutate('provider_failure', (events) => {
      const item = events[1].item as Record<string, unknown>
      item.message = 'provider-originated failure text'
      events[3].error = item.message
      events[3].message = item.message
    })
    mutate('provider_failure', (events) => {
      const item = events[1].item as Record<string, unknown>
      item.code = 'case_terminal_timeout'
      events[3].code = item.code
    })
    mutate('provider_failure', (events) => { events[2].timestamp = '2026-07-11T01:02:04Z' })
    mutate('cancel', (events) => { delete events[3].cancelledPendingGates })
    mutate('cancel', (events) => {
      events[3].cancelled = false
      events[3].cancelledPendingGates = 1
    })
    mutate('cancel', (events) => { events[3].cancelledPendingGates = -0 })
    mutate('success', (events) => { events[2].itemId = 'item_invented' })
    mutate('success', (events) => { events[2].discard = true })
  })

  it('keeps a four-slot accepted-final terminal error exact and snapshot-bound', () => {
    const delivery = acceptedFinalDeliveryContractFixtureV2('provider_failure')
    const assistantEvent = delivery.events[0] as Record<string, any>
    const errorEvent = delivery.events[1] as Record<string, any>
    const acceptedFinal = assistantEvent.item.acceptedFinal
    const acceptedFinalView = assistantEvent.item.acceptedFinalView
    const thread = {
      id: delivery.threadId,
      title: 'Accepted-final provider failure',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: delivery.timestamp,
      updatedAt: delivery.timestamp,
      historyAuthority: 'case_boundary_only_v1',
      turns: [{
        id: delivery.turnId,
        threadId: delivery.threadId,
        status: 'failed',
        createdAt: delivery.timestamp,
        finishedAt: delivery.timestamp,
        items: [assistantEvent.item, errorEvent.item],
        acceptedFinal,
        acceptedFinalView
      }],
      latestSeq: delivery.lastSeq,
      pendingApprovalIds: [],
      pendingUserInputIds: [],
      messageCount: 1,
      turnCount: 1,
      latestTurnId: delivery.turnId,
      acceptedFinalDelivery: delivery,
      acceptedFinalDeliveries: [delivery]
    }

    const parsed = ThreadDetailResponseV1Schema.safeParse(thread)
    expect(parsed.success).toBe(true)
    if (!parsed.success) return
    expect(parsed.data).toEqual(thread)

    const detachedDigest = structuredClone(thread)
    detachedDigest.turns[0].items[1].acceptedFinalDigest = acceptedFinalSha('0')
    expect(ThreadDetailResponseV1Schema.safeParse(detachedDigest).success).toBe(false)

    const detachedMessage = structuredClone(thread)
    detachedMessage.turns[0].items[1].message = 'detached terminal message'
    expect(ThreadDetailResponseV1Schema.safeParse(detachedMessage).success).toBe(false)

    const missingItem = structuredClone(thread)
    missingItem.turns[0].items.pop()
    expect(ThreadDetailResponseV1Schema.safeParse(missingItem).success).toBe(false)
  })

  it('binds every visible V3 turn to its own ordered delivery batch', () => {
    const first = acceptedFinalDeliveryContractFixtureV2('provider_failure')
    const second = structuredClone(first)
    const secondTurnId = 'turn_case_second'
    const secondCommit = acceptedFinalSha('f')
    const secondFirstSeq = first.lastSeq + 1
    second.turnId = secondTurnId
    second.publicationCommitId = secondCommit
    second.firstSeq = secondFirstSeq
    second.lastSeq = secondFirstSeq + second.events.length - 1
    second.seq = second.lastSeq
    second.publicationAuthority.turnId = secondTurnId
    second.publicationAuthority.publicationCommitId = secondCommit
    second.publicationAuthority.firstSeq = second.firstSeq
    second.publicationAuthority.lastSeq = second.lastSeq
    for (const [index, event] of second.events.entries()) {
      event.turnId = secondTurnId
      event.seq = secondFirstSeq + index
      event.acceptedFinalDigest = secondCommit
      event.publicationCommitId = secondCommit
      if ('item' in event && event.item && typeof event.item === 'object') {
        const item = event.item as Record<string, any>
        item.turnId = secondTurnId
        if ('acceptedFinalDigest' in item) item.acceptedFinalDigest = secondCommit
        if ('acceptedFinalView' in item && item.acceptedFinalView) {
          item.acceptedFinalView.acceptedFinalDigest = secondCommit
        }
      }
    }
    const firstAssistant = first.events[0].item as Record<string, any>
    const firstError = first.events[1].item as Record<string, any>
    const secondAssistant = second.events[0].item as Record<string, any>
    const secondError = second.events[1].item as Record<string, any>
    const thread = {
      id: first.threadId,
      title: 'Two accepted finals',
      model: 'deepseek-chat',
      mode: 'agent',
      status: 'idle',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      relation: 'primary',
      createdAt: first.timestamp,
      updatedAt: second.timestamp,
      historyAuthority: 'case_boundary_only_v1',
      turns: [
        {
          id: first.turnId, threadId: first.threadId, status: 'failed',
          createdAt: first.timestamp, finishedAt: first.timestamp,
          items: [firstAssistant, firstError], acceptedFinalView: firstAssistant.acceptedFinalView
        },
        {
          id: second.turnId, threadId: second.threadId, status: 'failed',
          createdAt: second.timestamp, finishedAt: second.timestamp,
          items: [secondAssistant, secondError], acceptedFinalView: secondAssistant.acceptedFinalView
        }
      ],
      latestSeq: second.lastSeq,
      pendingApprovalIds: [],
      pendingUserInputIds: [],
      messageCount: 2,
      turnCount: 2,
      latestTurnId: second.turnId,
      acceptedFinalDeliveries: [first, second]
    }
    expect(ThreadDetailResponseV1Schema.safeParse(thread).success).toBe(true)

    const mutations: Array<(candidate: Record<string, any>) => void> = [
      (candidate) => { candidate.turns[0].acceptedFinalView.acceptedFinalDigest = secondCommit },
      (candidate) => { candidate.turns[1].items[0].text = 'detached item' },
      (candidate) => { candidate.acceptedFinalDeliveries[0] = candidate.acceptedFinalDeliveries[1] },
      (candidate) => { candidate.acceptedFinalDeliveries[1].firstSeq = first.lastSeq },
      (candidate) => { candidate.acceptedFinalDeliveries[0].publicationCommitId = secondCommit },
      (candidate) => {
        candidate.acceptedFinalDeliveries[0].publicationAuthority =
          candidate.acceptedFinalDeliveries[1].publicationAuthority
      },
      (candidate) => { candidate.acceptedFinalDeliveries.splice(0, 1) },
      (candidate) => { candidate.acceptedFinalDeliveries.push(candidate.acceptedFinalDeliveries[1]) },
      (candidate) => { candidate.acceptedFinalDeliveries.reverse() },
      (candidate) => { candidate.acceptedFinalDelivery = candidate.acceptedFinalDeliveries[0] },
      (candidate) => { candidate.acceptedFinalDeliveries[0].turnId = second.turnId },
      (candidate) => { candidate.acceptedFinalDeliveries[0] = acceptedFinalDeliveryAuditFixtureV1('provider_failure') },
      (candidate) => { candidate.latestSeq = second.lastSeq - 1 }
    ]
    for (const mutate of mutations) {
      const candidate = JSON.parse(JSON.stringify(thread)) as Record<string, any>
      mutate(candidate)
      expect(ThreadDetailResponseV1Schema.safeParse(candidate).success).toBe(false)
    }
  })

  it('accepts per-turn and per-thread runtime step limit overrides', () => {
    const turn = StartTurnRequest.parse({
      prompt: 'Bound this turn',
      maxModelSteps: 0
    })
    expect(turn.maxModelSteps).toBe(0)

    const thread = CreateThreadRequest.parse({
      workspace: '/tmp',
      model: 'fake',
      runtimeStepLimits: { maxModelSteps: 32 }
    })
    expect(thread.runtimeStepLimits?.maxModelSteps).toBe(32)

    const patch = UpdateThreadRequest.parse({
      runtimeStepLimits: null
    })
    expect(patch.runtimeStepLimits).toBeNull()
  })

  it('accepts the IM/headless disableUserInput flag on start turn payloads', () => {
    const parsed = StartTurnRequest.parse({
      prompt: 'Reply to the WeChat user',
      disableUserInput: true
    })
    expect(parsed.disableUserInput).toBe(true)
    expect(StartTurnRequest.parse({ prompt: 'GUI turn' }).disableUserInput).toBeUndefined()
  })

  it('accepts turn failure lifecycle messages', () => {
    const event = RuntimeEvent.parse({
      kind: 'turn_failed',
      seq: 1,
      timestamp: '2026-06-03T00:00:01.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      message: 'model stream exploded'
    })
    expect(event).toMatchObject({
      kind: 'turn_failed',
      message: 'model stream exploded'
    })
  })

  it('accepts GUI plan context on start turn payloads', () => {
    const parsed = StartTurnRequest.parse({
      prompt: 'Plan auth',
      displayText: 'Generate implementation plan',
      guiPlan: {
        operation: 'draft',
        workspaceRoot: '/tmp/ws',
        relativePath: '.analytix/plan/auth.md',
        planId: '/tmp/ws:.analytix/plan/auth.md',
        sourceRequest: 'Add auth',
        title: 'Auth'
      }
    })
    expect(parsed.guiPlan?.relativePath).toBe('.analytix/plan/auth.md')
    expect(parsed.displayText).toBe('Generate implementation plan')
  })

  it('rejects unsafe GUI plan context paths on start turn payloads', () => {
    const result = StartTurnRequest.safeParse({
      prompt: 'Plan auth',
      guiPlan: {
        operation: 'draft',
        workspaceRoot: '/tmp/ws',
        relativePath: '.analytix/plan/nested/auth.md',
        planId: 'plan_bad'
      }
    })
    expect(result.success).toBe(false)
  })

  it('produces a deterministic empty usage snapshot', () => {
    const usage = emptyUsageSnapshot()
    expect(usage.cacheHitRate).toBeNull()
    expect(usage.totalTokens).toBe(0)
  })

  it('parses usage with cache metrics', () => {
    const usage = UsageSnapshotSchema.parse({
      promptTokens: 100,
      completionTokens: 20,
      totalTokens: 120,
      cachedTokens: 60,
      cacheHitTokens: 40,
      cacheMissTokens: 60,
      cacheHitRate: 0.4,
      turns: 1
    })
    expect(usage.cacheHitRate).toBeCloseTo(0.4)
  })

  it('accepts the canonical lifecycle runtime events', () => {
    const samples: RuntimeEventType[] = [
      {
        kind: 'thread_created',
        seq: 1,
        timestamp: '2025-01-01T00:00:00.000Z',
        threadId: 'thr_1',
        title: 'demo'
      },
      {
        kind: 'turn_started',
        seq: 2,
        timestamp: '2025-01-01T00:00:01.000Z',
        threadId: 'thr_1',
        turnId: 'turn_1'
      },
      {
        kind: 'usage',
        seq: 3,
        timestamp: '2025-01-01T00:00:02.000Z',
        threadId: 'thr_1',
        effort: 'high',
        usageSource: 'subagent',
        childRunId: 'child_1',
        usage: emptyUsageSnapshot()
      },
      {
        kind: 'heartbeat',
        seq: 4,
        timestamp: '2025-01-01T00:00:03.000Z',
        threadId: 'thr_1'
      }
    ]
    for (const sample of samples) {
      const parsed = RuntimeEvent.parse(sample)
      expect(parsed.kind).toBe(sample.kind)
    }
  })

  it('accepts extension contracts for attachments, memory, child events, and structured errors', () => {
    expect(AttachmentUploadRequest.parse({
      name: 'shot.png',
      mimeType: 'image/png',
      dataBase64: 'abcd',
      textFallback: {
        dataBase64: 'abcd',
        mimeType: 'image/webp',
        byteSize: 3,
        width: 1,
        height: 1,
        wasCompressed: true
      },
      threadId: 'thr_1',
      workspace: '/tmp/ws'
    }).textFallback?.mimeType).toBe('image/webp')

    expect(AttachmentUploadRequest.parse({
      name: 'brief.pdf',
      mimeType: 'application/pdf',
      dataBase64: 'JVBERi0=',
      documentText: 'PDF body',
      pageCount: 2,
      threadId: 'thr_1',
      workspace: '/tmp/ws'
    }).pageCount).toBe(2)

    expect(AttachmentUploadRequest.safeParse({
      name: 'unbound.txt',
      dataBase64: 'YWJj',
      threadId: 'thr_1'
    }).success).toBe(false)

    const publicAttachment = {
      id: 'att_0123456789abcdef01234567',
      name: 'case.csv',
      kind: 'document' as const,
      mimeType: 'text/csv',
      byteSize: 3,
      scope: 'thread' as const,
      createdAt: '2026-07-14T00:00:00Z',
      updatedAt: '2026-07-14T00:00:00Z'
    }
    expect(AttachmentUploadResponse.parse({ attachment: publicAttachment }).attachment.scope).toBe('thread')
    expect(AttachmentUploadResponse.safeParse({
      attachment: { ...publicAttachment, documentText: 'must stay private' }
    }).success).toBe(false)
    expect(AttachmentUploadResponse.safeParse({
      attachment: { ...publicAttachment, hash: 'a'.repeat(64) }
    }).success).toBe(false)
    expect(AttachmentUploadResponse.safeParse({
      attachment: { ...publicAttachment, localFilePath: '/tmp/private.csv' }
    }).success).toBe(false)
    expect(AttachmentUploadRequest.safeParse({
      name: 'case.csv',
      dataBase64: 'YWJj',
      threadId: 'thr_1',
      workspace: '/tmp/ws',
      localFilePath: '/tmp/private.csv'
    }).success).toBe(false)

    expect(MemoryRecord.parse({
      id: 'mem_1',
      content: 'Use pnpm',
      scope: 'workspace',
      workspace: '/tmp/ws',
      tags: ['frontend'],
      confidence: 0.9,
      provenance: 'manual-general',
      captureMode: 'manual',
      modelInjection: false,
      createdAt: '2026-06-03T00:00:00.000Z',
      updatedAt: '2026-06-03T00:00:00.000Z'
    })).toMatchObject({ tags: ['frontend'] })

    const child = RuntimeEvent.parse({
      kind: 'turn_completed',
      seq: 10,
      timestamp: '2026-06-03T00:00:00.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      child: {
        parentThreadId: 'thr_1',
        parentTurnId: 'turn_1',
        parentToolCallId: 'call_task',
        childId: 'child_1',
        childRunId: 'child_1',
        childThreadId: 'thr_child',
        childTurnId: 'turn_child',
        childLabel: 'research',
        childStatus: 'completed',
        childSeq: 1,
        childModel: 'deepseek-chat',
        childEffort: 'high',
        background: true,
        parallelGroupId: 'group_1',
        parallelIndex: 2
      }
    })
    expect(child.child?.childId).toBe('child_1')
    expect(child.child).toMatchObject({
      parentToolCallId: 'call_task',
      childRunId: 'child_1',
      childThreadId: 'thr_child',
      childTurnId: 'turn_child',
      childEffort: 'high',
      background: true,
      parallelGroupId: 'group_1',
      parallelIndex: 2
    })

    const progress = RuntimeEvent.parse({
      kind: 'tool_progress',
      seq: 11,
      timestamp: '2026-06-03T00:00:01.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      itemId: 'item_tool_1',
      callId: 'call_task',
      toolName: 'task',
      status: 'running',
      message: 'subagent running',
      child: {
        parentThreadId: 'thr_1',
        parentTurnId: 'turn_1',
        childId: 'job-1',
        childStatus: 'running'
      }
    })
    expect(progress.kind).toBe('tool_progress')
    expect(progress.child?.childId).toBe('job-1')

    const subagentStage = RuntimeEvent.parse({
      kind: 'pipeline_stage',
      seq: 12,
      timestamp: '2026-06-03T00:00:02.000Z',
      threadId: 'thr_1',
      turnId: 'turn_1',
      stage: 'subagent_running',
      label: 'Research child'
    })
    if (subagentStage.kind !== 'pipeline_stage') {
      throw new Error(`expected pipeline_stage, received ${subagentStage.kind}`)
    }
    expect(subagentStage.stage).toBe('subagent_running')

    const providerStages = [
      'provider_retrying',
      'provider_error',
      'empty_final_recovered'
    ] as const
    for (const [index, stage] of providerStages.entries()) {
      const event = RuntimeEvent.parse({
        kind: 'pipeline_stage',
        seq: 13 + index,
        timestamp: '2026-06-03T00:00:03.000Z',
        threadId: 'thr_1',
        turnId: 'turn_1',
        stage,
        label: stage
      })
      if (event.kind !== 'pipeline_stage') {
        throw new Error(`expected pipeline_stage, received ${event.kind}`)
      }
      expect(event.stage).toBe(stage)
    }

    expect(AnalytixErrorBody.parse({
      code: 'model_modality_unsupported',
      message: 'model does not support image input'
    }).code).toBe('model_modality_unsupported')
  })
})

describe('cli', () => {
  it('parses serve options with the canonical flags', () => {
    const parsed = parseServeOptions([
      '--host',
      '127.0.0.1',
      '--port',
      '8787',
      '--data-dir',
      '/tmp/ca',
      '--model',
      'deepseek-chat',
      '--approval-policy',
      'auto',
      '--sandbox-mode',
      'workspace-write',
      '--token-economy',
      '--insecure'
    ], { ANALYTIX_RUNTIME_TOKEN: 'abc' })
    expect(parsed.host).toBe('127.0.0.1')
    expect(parsed.port).toBe(8787)
    expect(parsed.tokenEconomyMode).toBe(true)
    expect(parsed.tokenEconomy?.enabled).toBe(true)
    expect(parsed.insecure).toBe(true)
    expect(parsed.runtimeToken).toBe('abc')
  })

  it('rejects a runtime token in argv while retaining its environment route', () => {
    expect(() => parseServeOptions([
      '--data-dir', '/tmp/ca',
      '--runtime-token=secret'
    ])).toThrow(/not argv/)
  })

  it('rejects a Provider key in argv without suggesting an environment credential route', () => {
    let message = ''
    try {
      parseServeOptions(['--data-dir', '/tmp/ca', '--api-key', 'secret'])
    } catch (error) {
      message = error instanceof Error ? error.message : String(error)
    }
    expect(message).toMatch(/local data-directory Registry authority/)
    expect(message).not.toMatch(/environment|secret storage/i)
  })

  it('parses flags in --key=value form', () => {
    const parsed = parseServeOptions([
      '--host=0.0.0.0',
      '--port=9090',
      '--data-dir=/srv/ca',
      '--storage-backend=file'
    ])
    expect(parsed.host).toBe('0.0.0.0')
    expect(parsed.port).toBe(9090)
    expect(parsed.dataDir).toBe('/srv/ca')
    expect(parsed.storage.backend).toBe('file')
  })

  it('parses only key-free model metadata from the analytix env snapshot', () => {
    const parsed = parseServeOptions(['--data-dir', '/tmp/analytix'], {
      ANALYTIX_MODEL_PROVIDERS: JSON.stringify({
        defaultProviderId: 'zai-coding-plan',
        providers: [{
          id: 'zai-coding-plan',
          baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
          endpointFormat: 'custom_endpoint',
          models: ['glm-5']
        }]
      })
    })

    expect(parsed.modelProviders?.defaultProviderId).toBe('zai-coding-plan')
    expect(parsed.modelProviders?.providers[0]).toMatchObject({
      id: 'zai-coding-plan',
      baseUrl: 'https://api.z.ai/api/coding/paas/v4/chat/completions',
      endpointFormat: 'custom_endpoint',
      models: ['glm-5']
    })
  })

  it.each([
    {
      name: 'an ambient Provider API key',
      env: { ANALYTIX_API_KEY: 'synthetic-ambient-provider-credential' }
    },
    {
      name: 'credential-bearing ambient Provider metadata',
      env: {
        ANALYTIX_MODEL_PROVIDERS: JSON.stringify({
          providers: [{
            id: 'provider-ambient',
            baseUrl: 'https://provider.invalid/v1',
            apiKey: 'synthetic-ambient-provider-credential'
          }]
        })
      }
    }
  ])('rejects $name before serve launch', ({ env }) => {
    expect(() => parseServeOptions(['--data-dir', '/tmp/analytix'], env))
      .toThrow(/Registry authority/)
  })

  it('rejects a credential-bearing config file before serve launch', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'analytix-provider-authority-'))
    try {
      const configPath = join(dir, 'analytix.config.json')
      await writeFile(configPath, JSON.stringify({
        serve: {
          dataDir: join(dir, 'data'),
          apiKey: 'synthetic-config-provider-credential'
        }
      }), 'utf8')
      expect(() => parseServeOptions(['--config', configPath]))
        .toThrow(/Registry authority/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('loads serve and context compaction settings from an explicit config file', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'analytix-config-'))
    try {
      const configPath = join(dir, 'analytix.config.json')
      await writeFile(configPath, JSON.stringify({
        serve: {
          host: '0.0.0.0',
          port: 7777,
          dataDir: join(dir, 'data'),
          model: 'deepseek-v4-flash',
          approvalPolicy: 'auto',
          tokenEconomy: {
            enabled: true,
            compressToolDescriptions: false,
            compressToolResults: true,
            conciseResponses: false,
            historyHygiene: {
              maxToolResultLines: 120,
              maxToolResultBytes: 16384,
              maxToolResultTokens: 4000,
              maxToolArgumentStringBytes: 4096,
              maxToolArgumentStringTokens: 1000,
              maxArrayItems: 40,
              maxCumulativeToolResultTokens: 90000,
              keepRecentToolResults: 3
            }
          },
          storage: {
            backend: 'hybrid',
            sqlitePath: join(dir, 'data', 'index.sqlite3')
          }
        },
        contextCompaction: {
          defaultSoftThreshold: 32_000,
          defaultHardThreshold: 48_000,
          summaryMode: 'model',
          summaryTimeoutMs: 15_000,
          summaryMaxTokens: 1_200,
          summaryInputMaxBytes: 98_304
        },
        models: {
          profiles: {
            'custom-1m': {
              aliases: ['vendor/custom-1m'],
              contextWindowTokens: 1_000_000,
              contextCompaction: {
                softRatio: 0.7,
                hardRatio: 0.85
              },
              inputModalities: ['text', 'image'],
              outputModalities: ['text'],
              supportsToolCalling: false,
              messageParts: ['text', 'image_url']
            }
          }
        },
        runtime: {
          toolStorm: {
            enabled: true,
            windowSize: 5,
            threshold: 4
          },
          toolArgumentRepair: {
            maxStringBytes: 4096
          }
        },
        capabilities: {
          web: {
            enabled: true,
            fetchEnabled: true,
            searchEnabled: false,
            provider: 'test'
          },
          skills: {
            enabled: true,
            roots: ['/tmp/skills']
          }
        }
      }), 'utf8')

      const parsed = parseServeOptions([
        '--config',
        configPath,
        '--model',
        'deepseek-v4-pro'
      ], {
        ANALYTIX_PORT: '9091'
      })

      expect(parsed.configPath).toBe(configPath)
      expect(parsed.host).toBe('0.0.0.0')
      expect(parsed.port).toBe(9091)
      expect(parsed.model).toBe('deepseek-v4-pro')
      expect(parsed.approvalPolicy).toBe('auto')
      expect(parsed.tokenEconomyMode).toBe(true)
      expect(parsed.tokenEconomy).toMatchObject({
        enabled: true,
        compressToolDescriptions: false,
        compressToolResults: true,
        conciseResponses: false,
        historyHygiene: {
          maxToolResultLines: 120,
          maxToolResultBytes: 16384,
          maxToolResultTokens: 4000,
          maxToolArgumentStringBytes: 4096,
          maxToolArgumentStringTokens: 1000,
          maxArrayItems: 40,
          maxCumulativeToolResultTokens: 90000,
          keepRecentToolResults: 3
        }
      })
      expect(parsed.storage).toEqual({
        backend: 'hybrid',
        sqlitePath: join(dir, 'data', 'index.sqlite3')
      })
      expect(parsed.contextCompaction?.defaultSoftThreshold).toBe(32_000)
      expect(parsed.contextCompaction?.summaryMode).toBe('model')
      expect(parsed.contextCompaction?.summaryTimeoutMs).toBe(15_000)
      expect(parsed.contextCompaction?.summaryMaxTokens).toBe(1_200)
      expect(parsed.contextCompaction?.summaryInputMaxBytes).toBe(98_304)
      expect(parsed.models?.profiles?.['custom-1m']?.contextCompaction?.softRatio).toBe(0.7)
      expect(parsed.models?.profiles?.['custom-1m']?.inputModalities).toEqual(['text', 'image'])
      expect(parsed.runtime?.toolStorm?.windowSize).toBe(5)
      expect(parsed.runtime?.toolStorm?.threshold).toBe(4)
      expect(parsed.runtime?.toolArgumentRepair?.maxStringBytes).toBe(4096)
      expect(parsed.capabilities.web.enabled).toBe(true)
      expect(parsed.capabilities.web.fetchEnabled).toBe(true)
      expect(parsed.capabilities.skills.roots).toEqual(['/tmp/skills'])
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('fails loudly for unsupported context compaction scorer overrides', async () => {
    const dir = await mkdtemp(join(tmpdir(), 'analytix-config-'))
    try {
      const configPath = join(dir, 'analytix.config.json')
      await writeFile(configPath, JSON.stringify({
        serve: {
          dataDir: join(dir, 'data')
        },
        contextCompaction: {
          summaryMode: 'heuristic',
          summaryScorer: 'custom'
        }
      }), 'utf8')

      expect(() => parseServeOptions(['--config', configPath]))
        .toThrow(/summaryScorer/)
    } finally {
      await rm(dir, { recursive: true, force: true })
    }
  })

  it('normalizes capability config to disabled defaults', () => {
    const config = AnalytixCapabilitiesConfig.parse({})
    expect(config.mcp.enabled).toBe(false)
    expect(config.mcp.search.enabled).toBe(false)
    expect(config.mcp.search.mode).toBe('auto')
    expect(config.web.enabled).toBe(false)
    expect(config.skills.enabled).toBe(false)
    expect(config.subagents.maxParallel).toBe(0)
    expect(config.attachments.allowedMimeTypes).toContain('image/png')
    expect(config.attachments.textFallbackMaxBase64Bytes).toBe(512 * 1024)
    expect(config.attachments.textFallbackMaxImageDimension).toBe(1280)
    expect(config.attachments.textFallbackPreferredMimeType).toBe('image/webp')
    expect(config.memory.scopes).toEqual(['user', 'workspace', 'project'])
    expect(config.imageGen.enabled).toBe(false)
    expect(config.imageGen.timeoutMs).toBe(180_000)
    expect(config.imageGen.maxReferenceImages).toBe(4)
  })

  it('ignores legacy subagent step-limit config fields', () => {
    const config = AnalytixCapabilitiesConfig.parse({
      subagents: {
        enabled: true,
        maxParallel: 2,
        maxChildRuns: 4,
        defaultStepLimit: 99
      }
    })

    expect(config.subagents).toMatchObject({
      enabled: true,
      maxParallel: 2,
      maxChildRuns: 4
    })
    expect('defaultStepLimit' in config.subagents).toBe(false)
  })

  it('parses subagent profiles and defaults the tool policy to read-only', () => {
    const config = AnalytixCapabilitiesConfig.parse({
      subagents: {
        enabled: true,
        maxParallel: 3,
        maxChildRuns: 10,
        defaultProfile: 'reviewer',
        profiles: {
          reviewer: { model: 'deepseek-v4-pro', effort: 'high', promptPreamble: 'Review for bugs.', toolPolicy: 'readOnly' },
          fixer: { toolPolicy: 'inherit' }
        }
      }
    })
    expect(config.subagents.defaultToolPolicy).toBe('readOnly')
    expect(config.subagents.defaultProfile).toBe('reviewer')
    expect(config.subagents.profiles.reviewer).toMatchObject({ model: 'deepseek-v4-pro', effort: 'high', toolPolicy: 'readOnly' })
    // Profiles default toolPolicy to readOnly when omitted.
    expect(config.subagents.profiles.fixer.toolPolicy).toBe('inherit')
  })

  it('rejects a defaultProfile that is not defined in profiles', () => {
    expect(() => AnalytixCapabilitiesConfig.parse({
      subagents: { enabled: true, maxParallel: 1, maxChildRuns: 1, defaultProfile: 'ghost' }
    })).toThrow(/defaultProfile/)
  })

  it('surfaces subagent profiles and policy in the runtime capability manifest', () => {
    const manifest = buildRuntimeCapabilityManifest({
      model: modelCapabilitiesForModel('deepseek-chat'),
      config: AnalytixCapabilitiesConfig.parse({
        subagents: {
          enabled: true,
          maxParallel: 2,
          maxChildRuns: 6,
          defaultProfile: 'reviewer',
          profiles: { reviewer: { model: 'deepseek-v4-pro', effort: 'high', toolPolicy: 'readOnly' } }
        }
      }),
      subagents: { available: true }
    })
    expect(manifest.subagents).toMatchObject({
      maxParallel: 2,
      maxChildRuns: 6,
      defaultToolPolicy: 'readOnly',
      defaultProfile: 'reviewer',
      profiles: [{ name: 'reviewer', model: 'deepseek-v4-pro', effort: 'high', toolPolicy: 'readOnly' }]
    })
  })

  it('resolves model capability fields from configured profiles', () => {
    const profiles = modelContextProfilesFromConfig({
      models: {
        profiles: {
          'vision-model': {
            contextWindowTokens: 128_000,
            contextCompaction: {
              softRatio: 0.7,
              hardRatio: 0.8
            },
            inputModalities: ['text', 'image'],
            supportsToolCalling: false,
            messageParts: ['text', 'image_url']
          }
        }
      }
    })
    const model = modelCapabilitiesForModel('vision-model', profiles)

    expect(model.contextWindowTokens).toBe(128_000)
    expect(model.inputModalities).toEqual(['text', 'image'])
    expect(model.supportsToolCalling).toBe(false)
    expect(model.messageParts).toEqual(['text', 'image_url'])
  })

  it('keeps legacy contextCompaction model profiles as a compatibility path', () => {
    const profiles = modelContextProfilesFromConfig({
      contextCompaction: {
        modelProfiles: {
          'legacy-model': {
            contextWindowTokens: 64_000,
            softThreshold: 48_000,
            hardThreshold: 56_000
          }
        }
      }
    })
    const model = modelCapabilitiesForModel('legacy-model', profiles)
    const legacy = profiles.find((profile) => profile.canonicalModel === 'legacy-model')

    expect(model.contextWindowTokens).toBe(64_000)
    expect(legacy?.softThreshold).toBe(48_000)
    expect(legacy?.hardThreshold).toBe(56_000)
  })

  it('uses 75%/85% of the window as the built-in DeepSeek v4 compaction thresholds', () => {
    // Compaction must trigger with headroom to spare. Triggering at
    // 98%/99% left no room for a large turn to land before the window was
    // exceeded, so the built-in ratios are 0.75 / 0.85 of the 1M window.
    const profile = modelContextProfilesFromConfig()
      .find((candidate) => candidate.canonicalModel === 'deepseek-v4-pro')

    expect(profile?.contextWindowTokens).toBe(1_000_000)
    expect(profile?.softThreshold).toBe(750_000)
    expect(profile?.hardThreshold).toBe(850_000)
  })

  it('keeps built-in DeepSeek v4 models text-only', () => {
    for (const modelId of ['deepseek-v4-pro', 'deepseek-v4-flash', 'deepseek-chat'] as const) {
      const model = modelCapabilitiesForModel(modelId)
      expect(model.inputModalities).toEqual(['text'])
      expect(model.messageParts).toEqual(['text'])
    }
  })

  it('builds runtime capability manifests with unavailable reasons', () => {
    const manifest = RuntimeCapabilityManifest.parse(buildRuntimeCapabilityManifest({
      model: modelCapabilitiesForModel('deepseek-chat')
    }))
    expect(manifest.contractVersion).toBe(1)
    expect(manifest.model.inputModalities).toContain('text')
    expect(manifest.mcp.available).toBe(false)
    expect(manifest.mcp.reason).toMatch(/disabled/)
    expect(manifest.mcp.search.enabled).toBe(false)
    expect(manifest.mcp.search.active).toBe(false)
    expect(manifest.attachments.textFallbackMaxBase64Bytes).toBe(512 * 1024)
    expect(manifest.attachments.textFallbackMaxImageDimension).toBe(1280)
    expect(manifest.attachments.textFallbackPreferredMimeType).toBe('image/webp')
    expect(manifest.memory.maxInjectedRecords).toBe(0)
    expect(manifest.imageGen.available).toBe(false)
    expect(manifest.imageGen.reason).toMatch(/disabled/)

    const enabledButMissingProvider = buildRuntimeCapabilityManifest({
      model: modelCapabilitiesForModel('deepseek-chat'),
      config: AnalytixCapabilitiesConfig.parse({
        web: { enabled: true, fetchEnabled: true, searchEnabled: true, provider: 'test' }
      })
    })
    expect(enabledButMissingProvider.web.enabled).toBe(true)
    expect(enabledButMissingProvider.web.available).toBe(false)
    expect(enabledButMissingProvider.web.reason).toMatch(/no web providers/)
  })

  it('accepts Kun baseline and Reasonix absorption metadata as runtime-info only', () => {
    const manifest = RuntimeCapabilityManifest.parse({
      ...buildRuntimeCapabilityManifest({ model: modelCapabilitiesForModel('deepseek-chat') }),
      upstreamAbsorption: {
        reasonixCapabilityMatrix: {
          schemaVersion: 1,
          changeId: 'reasonix-capability-audit',
          sourcePath: '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix',
          sourceCommit: '7032f39336f4ae5f216e1fcb3368e5f679723490',
          runtimeContract: 'analytix Go runtime server /v1 runtime HTTP/SSE contract',
          readyForG6: false,
          capabilityMatrixGreen: false,
          defaultBackendReady: false,
          defaultBackendReadinessGate: 'runtime durable restart evidence + credentialed provider matrix + credentialed MCP matrix + packaged QA + ANALYTIX_RUNTIME_READY=1',
          rows: [{
            id: 'multi-model-non-regression',
            capability: 'Kun/Analytix multi-model provider baseline under Reasonix absorption.',
            reasonixSources: ['internal/provider'],
            analytixLanding: 'runtime provider turn config.',
            absorptionClass: 'contract-reimplement',
            status: 'green',
            machineChecks: [{
              id: 'runtime-go-multi-model-matrix',
              kind: 'test',
              status: 'passed',
              evidence: 'packages/runtime-go/kun_analytix_baseline_absorption_test.go'
            }],
            analytixEvidence: ['packages/runtime-go/internal/upstreamaudit/baseline_absorption.go'],
            replacesAnalytixWeakness: 'Go runtime turn/server no longer hardcodes DeepSeek.',
            usesReasonixPublicProtocol: false,
            usesReasonixConfigRoot: false,
            changesRendererContract: false,
            changesProductIdentity: false,
            requiresKunProductEntryDrift: false
          }],
          greenCount: 1,
          redCount: 0,
          rejectedCount: 0,
          deferredCount: 0,
          forbiddenPublicProtocolRowCount: 0,
          rendererContractChangedRowCount: 0,
          productIdentityChangedRowCount: 0,
          kunProductEntryDriftRowCount: 0,
          postG6DeleteCandidates: [],
          g6Blockers: [],
          readinessSemantics: ['readyForG6 is a compatibility alias for capabilityMatrixGreen only.'],
          notes: ['Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing.']
        },
        kunAnalytixBaselineGuard: {
          schemaVersion: 1,
          changeId: 'kun-analytix-baseline-guard',
          kunSourceSnapshots: [
            'https://github.com/KunAgent/Kun.git refs/tags/v0.2.13^{}=2ba8decc2f56862e7f677fcf89bbc3d402ec3a23',
            'https://github.com/KunAgent/Kun.git refs/tags/v0.2.14^{}=8f2040349fba47fcd8e8b94f50b131943af839b2'
          ],
          analytixSourceRoot: '/Users/sun/Projects/analytix',
          fullFunctionBaseline: true,
          rows: [{
            id: 'provider-model-multimodel',
            capability: 'DeepSeek, OpenAI-compatible, Anthropic-compatible, and custom full endpoint model flows.',
            kunAnalytixSources: ['src/shared/openai-compat-url.ts'],
            entryPoint: 'provider settings and runtime turn',
            triggerPath: 'providerId/model/endpointFormat/baseUrl/custom endpoint',
            runtimeContract: 'provider family, endpoint format, request URL/body/header, stream parsing, usage parsing',
            settingsSchema: 'runtime.endpointFormat and provider.providers[].endpointFormat',
            uiSurface: 'Provider/model settings keep endpoint-format controls.',
            protectionTests: ['packages/runtime-go/kun_analytix_baseline_absorption_test.go'],
            immutableProductBaseline: true,
            reasonixEnhancementAllowed: 'DeepSeek cache/prefix enhancements only under DeepSeek provider family.',
            status: 'protected'
          }],
          protectedBaselineCount: 1,
          internalEnhancementCount: 1,
          forbiddenTopLevelEntrypoints: ['Workflow', 'Create Loop', 'Subagent', 'AutoResearch', 'MCP-indexer'],
          forbiddenEntrypointExposed: false,
          deprecatedBridgeAliasAllowed: false,
          legacySettingsWriteAllowed: false,
          deepseekOnlyRuntimeAllowed: false,
          readyForReasonixAbsorption: true,
          notes: ['Reasonix is an engine donor; Kun/Analytix product behavior is the baseline.']
        },
        reasonixAbsorptionMatrix: {
          schemaVersion: 1,
          changeId: 'reasonix-absorption-matrix',
          reasonixSourcePath: '/Users/sun/Projects/_upstreams/DeepSeek-Reasonix',
          reasonixSourceCommit: '7032f39336f4ae5f216e1fcb3368e5f679723490',
          rows: [{
            id: 'deepseek-cache-prefix-provider-specific',
            capability: 'Reasonix-style DeepSeek cache/prefix discipline scoped to DeepSeek provider family.',
            reasonixSources: ['internal/agent'],
            absorptionClass: 'code-port-and-adapt',
            status: 'green',
            analytixLanding: 'DeepSeek-only request fields and cache diagnostics.',
            providerSpecific: true,
            doesNotNarrowProviders: true,
            kunBaselineProtection: ['provider-model-multimodel'],
            machineChecks: [{
              id: 'deepseek-cache-scoped',
              kind: 'test',
              status: 'passed',
              evidence: 'packages/runtime-go/kun_analytix_baseline_absorption_test.go'
            }],
            forbiddenProductSurface: false
          }],
          greenCount: 1,
          redCount: 0,
          deferredCount: 0,
          rejectedCount: 0,
          forbiddenProductSurfaceCount: 0,
          providerFamilies: ['deepseek', 'openai-compatible', 'anthropic-compatible', 'custom_endpoint'],
          multiModelNonRegressionGreen: true,
          deepSeekEnhancementScopedOnly: true,
          readyForG6: false,
          absorptionMatrixGreen: false,
          defaultBackendReady: false,
          defaultBackendReadinessGate: 'runtime durable restart evidence + credentialed provider matrix + credentialed MCP matrix + packaged QA + ANALYTIX_RUNTIME_READY=1',
          g6Blockers: [],
          readinessSemantics: ['readyForG6 is a compatibility alias for absorptionMatrixGreen only.'],
          notes: ['Reasonix DeepSeek capability absorption is provider-specific enhancement, not provider narrowing.']
        },
        reasonixIntegrationTopology: reasonixIntegrationTopologyFixture(),
        reasonixSuperiorityMatrix: reasonixSuperiorityMatrixFixture(),
        defaultBackendReadiness: {
          schemaVersion: 1,
          ready: false,
          explicitReadyGate: false,
          durableRestartEvidence: { status: 'missing', required: true, message: 'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS is not set' },
          providerMatrix: { status: 'missing', required: true, message: 'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS is not set' },
          mcpMatrix: { status: 'missing', required: true, message: 'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS is not set' },
          packagedQa: { status: 'missing', required: true, message: 'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS is not set' },
          operatorGate: { status: 'missing', required: true, message: 'ANALYTIX_RUNTIME_READY is not set to 1' },
          missingRequiredChecks: ['durableRestartEvidence', 'providerMatrix', 'mcpMatrix', 'packagedQa', 'operatorGate'],
          defaultGoBackendEnabled: true,
          rendererVisibleGoSwitcher: false,
          notes: ['Runtime readiness is evidence-gated and defaults to not ready.']
        },
        readinessSemantics: {
          schemaVersion: 1,
          changeId: 'runtime-readiness-semantics',
          reasonixCapabilityMatrixGreen: false,
          reasonixAbsorptionMatrixGreen: false,
          kunAnalytixBaselineGuardGreen: true,
          capabilityMatrixGreen: false,
          absorptionMatrixGreen: false,
          baselineGuardGreen: true,
          defaultBackendReady: false,
          defaultBackendGateChangeId: 'runtime-readiness',
          requiredDefaultBackendGates: [
            'reasonixCapabilityMatrixGreen',
            'reasonixAbsorptionMatrixGreen',
            'kunAnalytixBaselineGuardGreen',
            'goRuntimeServerCanary',
            'adapterGoDefaultTest'
          ],
          skippedCountsAsPassed: false,
          fixtureMatrixCountsAsCredentialedPass: false,
          matrixGreenEnablesDefaultBackend: false,
          typeScriptRuntimeDefault: false,
          goRuntimeCandidateInternalOnly: false,
          notes: ['Engine absorption matrix green is not default backend readiness.']
        }
      }
    })

    expect(manifest.upstreamAbsorption?.kunAnalytixBaselineGuard.fullFunctionBaseline).toBe(true)
    expect(manifest.upstreamAbsorption?.reasonixAbsorptionMatrix.providerFamilies)
      .toContain('custom_endpoint')
    expect(manifest.upstreamAbsorption?.reasonixAbsorptionMatrix.deepSeekEnhancementScopedOnly)
      .toBe(true)
    expect(manifest.upstreamAbsorption?.reasonixIntegrationTopology.rowCount).toBe(5)
    expect(manifest.upstreamAbsorption?.reasonixIntegrationTopology.goDefaultCutoverCandidate)
      .toBe(true)
    expect(manifest.upstreamAbsorption?.reasonixIntegrationTopology.topLevelEntrypointAddedCount)
      .toBe(0)
    expect(manifest.upstreamAbsorption?.reasonixSuperiorityMatrix.codeStageClosed)
      .toBe(true)
    expect(manifest.upstreamAbsorption?.reasonixSuperiorityMatrix.goDefaultCutoverCandidate)
      .toBe(true)
    expect(manifest.upstreamAbsorption?.reasonixSuperiorityMatrix.rows)
      .toHaveLength(13)
  })

  it('loads config.json from the data dir when present', async () => {
    const dataDir = await mkdtemp(join(tmpdir(), 'analytix-data-'))
    try {
      await writeFile(join(dataDir, 'config.json'), JSON.stringify({
        serve: {
          baseUrl: 'https://example.invalid/v1',
          model: 'deepseek-v4-flash'
        },
        contextCompaction: {
          defaultSoftThreshold: 12_345,
          defaultHardThreshold: 23_456
        }
      }), 'utf8')

      const parsed = parseServeOptions(['--data-dir', dataDir])

      expect(parsed.configPath).toBe(join(dataDir, 'config.json'))
      expect(parsed.dataDir).toBe(dataDir)
      expect(parsed.baseUrl).toBe('https://example.invalid/v1')
      expect(parsed.model).toBe('deepseek-v4-flash')
      expect(parsed.approvalPolicy).toBe(DEFAULT_APPROVAL_POLICY)
      expect(parsed.contextCompaction?.defaultHardThreshold).toBe(23_456)
    } finally {
      await rm(dataDir, { recursive: true, force: true })
    }
  })

  it('returns a structured error when data-dir is missing', () => {
    const result = parseServeOptionsSafe([
      '--host',
      '127.0.0.1',
      '--port',
      '8899'
    ])
    expect(result.ok).toBe(false)
    if (!result.ok) {
      expect(result.exitCode).toBe(ServeExitCode.config)
    }
  })

  it('validates pre-constructed options', () => {
    const parsed = validateServeOptions({
      host: '127.0.0.1',
      port: 8899,
      dataDir: '/srv/ca',
      runtimeToken: '',
      model: 'deepseek-chat',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      insecure: false
    })
    expect(parsed.port).toBe(8899)
    expect(parsed.storage.backend).toBe('hybrid')
    expect(parsed.capabilities.mcp.enabled).toBe(false)

    expect(() => validateServeOptions({
      ...parsed,
      apiKey: 'synthetic-preconstructed-provider-credential'
    })).toThrow(/Registry authority/)
    expect(() => validateServeOptions({
      ...parsed,
      modelProviders: {
        providers: [{
          id: 'provider-preconstructed',
          baseUrl: 'https://provider.invalid/v1',
          apiKey: ''
        }]
      }
    })).toThrow(/Registry authority/)
  })

  it('exposes a usage string', () => {
    expect(SERVE_USAGE).toContain('analytix serve')
    expect(SERVE_USAGE).not.toContain('ANALYTIX_API_KEY')
  })

  it('surfaces zod issues for invalid configurations', () => {
    const result = parseServeOptionsSafe([
      '--port=abc',
      '--data-dir=/srv/ca'
    ])
    expect(result.ok).toBe(false)
  })

  it('flags unknown enum values through the schema', () => {
    const result = ApprovalPolicySchema.safeParse('mystery')
    expect(result.success).toBe(false)
  })
})
