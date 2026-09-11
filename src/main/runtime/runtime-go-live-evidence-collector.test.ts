import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { delimiter, join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'

type RuntimeGoLiveEvidenceModule = {
  buildRuntimeGoLiveEvidenceReport(options?: Record<string, unknown>): Promise<Record<string, unknown>>
  collectMCPExecution(options: Record<string, unknown>): Promise<Record<string, unknown>>
  collectOperatorGate(options: Record<string, unknown>): Record<string, unknown>
  collectPackagedQA(options: Record<string, unknown>): Record<string, unknown>
  collectPackagedGuiSmoke(options: Record<string, unknown>): Record<string, unknown>
  collectPackagedSessionSoak(options: Record<string, unknown>): Record<string, unknown>
  collectPackagedMilestoneA(options: Record<string, unknown>): Record<string, unknown>
  packagedMilestoneAReportProjection(options: Record<string, unknown>): Record<string, unknown>
  packagedMilestoneAMissingExternalInputs(report: Record<string, unknown>): Record<string, unknown>[]
  collectProviderMatrix(options: Record<string, unknown>): Promise<Record<string, unknown>>
  resolveProviderSettingsPath(env: Record<string, string>, options?: Record<string, unknown>): { path: string, source: string }
  strictGateEnv(options: Record<string, unknown>): Record<string, string>
}

let tempDirs: string[] = []

async function loadCollector(): Promise<RuntimeGoLiveEvidenceModule> {
  return await import(pathToFileURL(join(process.cwd(), 'scripts/runtime-go-live-evidence-collector.mjs')).href) as RuntimeGoLiveEvidenceModule
}

afterEach(() => {
  for (const dir of tempDirs) {
    rmSync(dir, { recursive: true, force: true })
  }
  tempDirs = []
})

function tempDir(): string {
  const dir = mkdtempSync(join(tmpdir(), 'analytix-runtime-go-live-evidence-'))
  tempDirs.push(dir)
  return dir
}

function currentTestCommit(): string {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  expect(result.status).toBe(0)
  return result.stdout.trim()
}

function writeExecutable(path: string, source: string): void {
  writeFileSync(path, source, 'utf8')
  chmodSync(path, 0o755)
}

function canonicalJSONString(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSONString(item)}`).join(',')}}`
  }
  return JSON.stringify(value)
}

function sha256(value: unknown): string {
  return createHash('sha256').update(canonicalJSONString(value)).digest('hex')
}

function sha256Text(value: string): string {
  return createHash('sha256').update(value).digest('hex')
}

function writeApprovalEvidence(path: string, overrides: Record<string, unknown> = {}): void {
  writeFileSync(path, JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-mcp-approval-user-input-evidence',
    status: 'passed',
    passed: true,
    approvalUserInput: true,
    rawValueRecorded: false,
    credentialSecretsRecorded: false,
    ...overrides
  }), 'utf8')
}

function passedMCPEvidence(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const probe = {
    id: 'credentialed-mcp',
    status: 'passed',
    skipped: false,
    credentialedExecution: true,
    connect: true,
    toolDiscoverySearch: true,
    toolCall: true,
    approvalUserInput: true,
    reconnect: true,
    credentialRedaction: true,
    commandConfigured: true
  }
  return {
    schemaVersion: 1,
    id: 'runtime-go-mcp-execution',
    status: 'passed',
    passed: true,
    credentialedExecution: true,
    topLevelMcpIndexerExposed: false,
    reasonixPublicProtocolUsed: false,
    requiredCoverage: ['connect', 'toolDiscoverySearch', 'toolCall', 'approvalUserInput', 'reconnect', 'redaction'],
    connect: true,
    toolDiscoverySearch: true,
    toolCall: true,
    approvalUserInput: true,
    reconnect: true,
    redaction: true,
    credentialedProbes: [probe],
    ...overrides
  }
}

function writeBlockedPackagedGuiScript(path: string): void {
  writeFileSync(path, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  stage: 'packaged-gui-bridge-smoke',
  status: 'failed',
  passed: false,
  smoke: { actualPackagedSmokeRequested: true, actualPackagedSmokePassed: false },
  actualPackagedSmoke: {
    status: 'failed',
    passed: false,
    checks: [{ id: 'packaged-app-artifact', status: 'failed', message: 'mock packaged GUI smoke blocked' }],
    redaction: { status: 'passed', secretMaterialFound: false }
  }
}))
`, 'utf8')
}

function writeBlockedPackagedSoakScript(path: string): void {
  writeFileSync(path, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-session-soak',
  stage: 'packaged-session-soak',
  status: 'failed',
  passed: false,
  soak: {
    deterministicContractOnly: true,
    goDurableSessionCovered: false,
    typeScriptRetiredBackendSessionCovered: false,
    resumeForkPairingCovered: false,
    sseReplayCovered: false
  },
  checks: [{ id: 'go-session-durable-contract', status: 'failed', reason: 'mock-blocked' }]
}))
`, 'utf8')
}

function actualPackagedSessionSoakReport(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const actualOverrides = overrides.actualPackagedSessionSoak &&
    typeof overrides.actualPackagedSessionSoak === 'object' &&
    !Array.isArray(overrides.actualPackagedSessionSoak)
    ? overrides.actualPackagedSessionSoak as Record<string, unknown>
    : {}
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-session-soak',
    generatedAt: '2026-06-24T00:00:00.000Z',
    status: 'passed',
    passed: true,
    soak: {
      deterministicContractOnly: false,
      actualPackagedAppLaunched: true,
      actualPackagedAppEvidenceRequiredForFinalGate: false,
      actualPackagedSessionSoakRequested: true,
      actualPackagedSessionSoakPassed: true,
      goDurableSessionCovered: true,
      typeScriptRetiredBackendSessionCovered: true,
      resumeForkPairingCovered: true,
      sseReplayCovered: true,
      mimoPlanCovered: true,
      attachmentFallbackCovered: true,
      approvalUserInputCovered: true,
      threadListSearchCovered: true,
      usageCovered: true
    },
    actualPackagedSessionSoak: {
      status: 'passed',
      passed: true,
      checks: [
        'packaged-app-artifact',
        'packaged-app-launch',
        'packaged-renderer-bridge',
        'packaged-settings-bridge',
        'packaged-provider-profile-settings',
        'packaged-runtime-restart',
        'packaged-session-thread-create',
        'packaged-session-turn-create',
        'packaged-session-sse-replay',
        'packaged-session-tool-timeline',
        'packaged-session-mimo-plan',
        'packaged-session-attachment-fallback',
        'packaged-session-approval-user-input',
        'packaged-session-fork',
        'packaged-session-resume',
        'packaged-session-thread-list',
        'packaged-session-provider-redaction'
      ].map((id) => ({ id, status: 'passed' })),
      app: {
        executableExists: true,
        runtimeServerBinaryExists: true,
        appAsarExists: true,
        bundledRuntimeGoSourcePresent: false
      },
      renderer: {
        apiPresent: true,
        title: 'Analytix',
        legacyKun: false,
        legacyReasonix: false,
        settingsRuntimeTopLevel: true,
        settingsProfilePatchAccepted: true,
        settingsCompatPatchUsed: false,
        restartOk: true,
        healthStatus: 200,
        healthService: 'analytix',
        threadCreateOk: true,
        turnCreateOk: true,
        sseReplayOk: true,
        attachmentUploadOk: true,
        attachmentTurnOk: true,
        attachmentSseReplayOk: true,
        attachmentMetadataOk: true,
        approvalDenyRequestedOk: true,
        approvalDenyResolvedOk: true,
        approvalDenyNoExecuteOk: true,
        approvalAllowRequestedOk: true,
        approvalAllowResolvedOk: true,
        approvalAllowExecutedOk: true,
        userInputRequestedOk: true,
        userInputResolvedOk: true,
        userInputReplayOk: true,
        forkOk: true,
        forkTurnOk: true,
        resumeOk: true,
        resumeTurnOk: true,
        threadListOk: true,
        usageOk: true,
        initialThreadIdHash: 'a'.repeat(64),
        initialTurnIdHash: 'b'.repeat(64),
        forkThreadIdHash: 'c'.repeat(64),
        resumeThreadIdHash: 'd'.repeat(64)
      },
      provider: {
        localContractProviderUsed: true,
        externalProviderNetworkCalled: false,
        requestCount: 3,
        authorizationConfigured: true,
        initialPromptSeen: true,
        attachmentFallbackSeen: true,
        approvalDenySeen: true,
        approvalAllowSeen: true,
        userInputSeen: true,
        forkPromptSeen: true,
        resumePromptSeen: true,
        historyCarriedToChildOrResume: true
      },
      redaction: {
        status: 'passed',
        secretMaterialFound: false
      },
      ...actualOverrides
    },
    checks: [
      { id: 'go-session-durable-contract', status: 'passed' },
      { id: 'typescript-retired-backend-session-contract', status: 'passed' }
    ]
  }
  return {
    ...report,
    ...overrides,
    actualPackagedSessionSoak: {
      ...report.actualPackagedSessionSoak,
      ...actualOverrides
    }
  }
}

function writeActualPackagedSoakScript(path: string, overrides: Record<string, unknown> = {}): void {
  const report = actualPackagedSessionSoakReport(overrides)
  writeFileSync(path, `console.log(JSON.stringify(${JSON.stringify(report)}))\n`, 'utf8')
}

function writeArgvCaptureScript(path: string, id: string, stage: string): void {
  writeFileSync(path, `
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
const capturePath = process.env.ARGV_CAPTURE_PATH
const entries = capturePath && existsSync(capturePath)
  ? JSON.parse(readFileSync(capturePath, 'utf8'))
  : []
entries.push({ id: ${JSON.stringify(id)}, argv: process.argv.slice(2) })
if (capturePath) writeFileSync(capturePath, JSON.stringify(entries))
console.log(JSON.stringify({
  schemaVersion: 1,
  id: ${JSON.stringify(id)},
  stage: ${JSON.stringify(stage)},
  status: 'failed',
  passed: false,
  checks: []
}))
`, 'utf8')
}

const PACKAGED_MILESTONE_A_CHECK_IDS = [
  'trusted-cache-tmpdir',
  'formal-packaged-artifact',
  'configured-network-provider',
  'isolated-profile-and-repository',
  'packaged-first-launch',
  'composer-workflow-submit',
  'ordinary-agent-workflow',
  'case-dependencies-unavailable-with-ordinary-capabilities',
  'real-repository-test',
  'bounded-subagent',
  'git-skill-mcp-research-writing',
  'long-context-continuation',
  'composer-compaction-submit',
  'nonzero-compaction',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'relaunch-provider-continuation',
  'renderer-visual-evidence',
  'exact-thread-recovery',
  'exact-todo-recovery',
  'exact-subagent-recovery',
  'exact-compaction-recovery',
  'exact-result-recovery',
  'renderer-visible-thread-recovery',
  'renderer-visible-todo-recovery',
  'renderer-visible-subagent-recovery',
  'renderer-visible-compaction-recovery',
  'renderer-visible-result-recovery',
  'normal-final-quit',
  'zero-residual-processes',
  'credential-redaction',
  'sandbox-cleanup'
]

const COMMERCIAL_RELEASE_CHECK_IDS = [
  'formal-release-publication-authority',
  'controlled-release-native-receipt'
]

function packagedMilestoneAReport(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const milestoneScriptSha256 = sha256Text(readFileSync(
    join(process.cwd(), 'scripts/runtime-go-packaged-milestone-a.mjs'),
    'utf8'
  ))
  const collectorScriptSha256 = sha256Text(readFileSync(
    join(process.cwd(), 'scripts/runtime-go-live-evidence-collector.mjs'),
    'utf8'
  ))
  return {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-a',
    stage: 'packaged-general-agent-milestone-a',
    status: 'passed',
    passed: true,
    credentialSecretsRecorded: false,
    syntheticProviderUsed: false,
    localProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    composerDomAndPrimaryButtonRequired: true,
    cdpAndBridgeObservationOnly: true,
    harness: {
      scriptSha256: milestoneScriptSha256,
      contractManifestSha256: 'a'.repeat(64),
      entries: [
        {
          name: 'runtime-go-live-evidence-collector.mjs',
          sha256: collectorScriptSha256
        }
      ]
    },
    app: {
      ok: true,
      codeSignatureVerified: true,
      controlledReleaseNativeReceipt: true
    },
    releasePublicationAuthority: {
      ok: true,
      classification: 'verified'
    },
    operatorCheckpoint: {
      declared: true,
      method: 'visible-computer-use',
      automatedCredentialEntryUsed: true,
      status: 'completed'
    },
    commercialRelease: {
      requiredForMilestoneA: false,
      requiredForCommercialPublication: true,
      status: 'passed',
      passed: true,
      checks: COMMERCIAL_RELEASE_CHECK_IDS.map((id) => ({ id, status: 'passed' })),
      failureCheckIds: [],
      missingExternalInputs: []
    },
    provider: {
      configured: true,
      credentialConfigured: true,
      credentialRecorded: false,
      modelHash: sha256Text('deepseek-v4-flash'),
      requiredModelHash: sha256Text('deepseek-v4-flash'),
      reasoningEffort: 'high',
      networkTurnCompleted: true,
      threadProviderBound: true,
      threadModelBound: true,
      resultTurnProviderReceiptBound: true,
      providerAttemptTelemetryValid: true,
      providerAttemptReceiptDigest: 'b'.repeat(64),
      providerLogicalCallCount: 3,
      providerAttemptCount: 3,
      successfulProviderAttemptCount: 3,
      ordinaryEvidence: {
        networkTurnCompleted: true,
        threadProviderBound: true,
        threadModelBound: true,
        resultTurnProviderReceiptBound: true,
        providerAttemptTelemetryValid: true,
        providerAttemptReceiptDigest: 'b'.repeat(64),
        providerLogicalCallCount: 3,
        providerAttemptCount: 3,
        successfulProviderAttemptCount: 3
      },
      longContextEvidence: {
        networkTurnCompleted: true,
        threadProviderBound: true,
        threadModelBound: true,
        resultTurnProviderReceiptBound: true,
        providerAttemptTelemetryValid: true,
        providerAttemptReceiptDigest: '1'.repeat(64),
        providerLogicalCallCount: 1,
        providerAttemptCount: 1,
        successfulProviderAttemptCount: 1
      }
    },
    visualEvidence: {
      firstCompleted: {
        captured: true,
        sha256: 'c'.repeat(64),
        width: 1280,
        height: 800,
        viewport: { width: 1280, height: 800, scale: 1 }
      },
      recoveredAfterRelaunch: {
        captured: true,
        sha256: 'd'.repeat(64),
        width: 1280,
        height: 800,
        viewport: { width: 1280, height: 800, scale: 1 }
      },
      screenshotsUsedAsFunctionalVerdict: false,
      credentialScanCovered: true
    },
    isolation: {
      cacheTmpdirVerified: true,
      systemUserDataTouched: false,
      isolatedHomeUsedByChild: true,
      sameDataDirsOnRelaunch: true,
      sandboxRemoved: true
    },
    publicSeams: {
      firstLaunch: {
        healthOk: true,
        runtimeInfoOk: true,
        runtimeToolsOk: true,
        ordinaryCatalogNonempty: true,
        fundsExecutionUnavailable: true
      },
      secondLaunch: {
        healthOk: true,
        runtimeInfoOk: true,
        runtimeToolsOk: true,
        ordinaryCatalogNonempty: true,
        fundsExecutionUnavailable: true
      }
    },
    caseCapability: {
      additiveNotReplacement: true,
      privateAuthorityInputsAbsent: true,
      privateAuthorityEnvironmentKeyCount: 0,
      privateAuthoritySettingsKeyCount: 0,
      preLaunchUnexpectedUserDataEntryCount: 0,
      firstLaunchFundsExecutionUnavailable: true,
      secondLaunchFundsExecutionUnavailable: true,
      ordinaryCatalogAvailable: true,
      ordinaryWorkflowCompleted: true,
      unavailableWhileOrdinaryCapabilitiesRetained: true
    },
    workflow: {
      firstLaunchObserved: true,
      composerWorkflowSubmitted: true,
      ordinaryWorkflowCompleted: true,
      planModeSelected: true,
      planTurnObserved: true,
      agentModeRestored: true,
      sameThreadPlanAgent: true,
      readObserved: true,
      planObserved: true,
      todoObserved: true,
      writeObserved: true,
      realTestObserved: true,
      subagentObserved: true,
      longContextContinuationObserved: true,
      longContextHostReadBound: true,
      longContextSubagentContinuityBound: true,
      longContextSubagentContinuityBaselineDigest: '8'.repeat(64),
      longContextSubagentContinuityProtectedFundsDigest: '8'.repeat(64),
      longContextSubagentContinuityDigest: '8'.repeat(64),
      longContextProviderReceiptReplayAttempted: true,
      longContextProviderReceiptReplayObserved: true,
      longContextProviderReceiptReplayReasonCode: 'provider_receipt_observed',
      longContextContinuationReasonCode: 'observed',
      longContextContinuationFailureCode: '',
      longContextProviderReceiptBound: true,
      longContextProviderReceiptDigest: '1'.repeat(64),
      longContextAcceptedFinalBound: true,
      successfulToolResultsObserved: true,
      todosCompleted: true,
      exactlyOneBoundedSubagentCompleted: true,
      manualCompactionBaselineBound: true,
      manualCompactionBaselineCount: 1,
      manualCompactionBaselineDigest: '1'.repeat(64),
      manualCompactionCandidateDigest: '2'.repeat(64),
      manualCompactionNewAfterBaseline: true,
      compactionCount: 1,
      manualCompactionObserved: true,
      manualCompactionAuto: false,
      manualCompactionReplacedTokens: 2048,
      manualCompactionSourceDigest: 'e'.repeat(64),
      manualCompactionSourceItemIdsDigest: 'f'.repeat(64),
      manualCompactionSourceAncestryBound: true,
      manualCompactionNonzeroBound: true,
      manualCompactionProjectionClass: 'ordinary_exact',
      normalFirstQuitObserved: true,
      firstExitCode: 0,
      firstExitSignal: null,
      freshProcessRelaunchObserved: true,
      relaunchProviderContinuationObserved: true,
      exactThreadRecovered: true,
      exactTodosRecovered: true,
      exactSubagentsRecovered: true,
      exactCompactionsRecovered: true,
      exactResultRecovered: true,
      rendererVisibleThreadRecovered: true,
      rendererVisibleTodosRecovered: true,
      rendererVisibleSubagentRecovered: true,
      rendererVisibleCompactionRecovered: true,
      rendererVisibleResultRecovered: true,
      normalFinalQuitObserved: true,
      finalExitCode: 0,
      finalExitSignal: null,
      zeroResidualProcesses: true
    },
    redaction: {
      status: 'passed',
      secretMaterialFound: false,
      credentialRecorded: false
    },
    checks: PACKAGED_MILESTONE_A_CHECK_IDS.map((id) => ({ id, status: 'passed' })),
    failedCheckIds: [],
    skippedCheckIds: [],
    liveBlockedCheckIds: [],
    failureCheckIds: [],
    failureCheckIdsSemantics: 'all_non_pass',
    ...overrides
  }
}

function aggregatePackagedQAReport(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  const packagedOverrides = overrides.packaged &&
    typeof overrides.packaged === 'object' &&
    !Array.isArray(overrides.packaged)
    ? overrides.packaged as Record<string, unknown>
    : {}
  const milestoneA = packagedMilestoneAReport()
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-qa',
    generatedAt: '2026-06-24T00:00:00.000Z',
    status: 'passed',
    passed: true,
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false },
    harness: {
      scriptSha256: sha256Text(readFileSync(
        join(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs'),
        'utf8'
      )),
      contractManifestSha256: 'b'.repeat(64)
    },
    milestoneAEvidence: milestoneA,
    packaged: {
      cutoverReady: true,
      cutoverFinalAcceptanceReady: false,
      finalAcceptanceReady: false,
      missingFinalCoverage: ['credentialed-provider', 'credentialed-mcp', 'operator-gate'],
      runtimeHealthPassed: true,
      runtimeHealthRuntimeInfoOk: true,
      runtimeHealthRuntimeToolsOk: true,
      runtimeHealthProductionCapabilitiesOk: true,
      guiSmokePassed: true,
      sessionSoakPassed: true,
      milestoneAPassed: true,
      milestoneAStatus: 'passed',
      milestoneAHarnessScriptSha256:
        (milestoneA.harness as Record<string, unknown>).scriptSha256,
      milestoneAHarnessManifestSha256:
        (milestoneA.harness as Record<string, unknown>).contractManifestSha256,
      milestoneAHarnessBound: true,
      credentialedProviderPublicSeamPassed: true,
      formalGeneralAgentWorkflowPassed: true,
      rendererVisualPublicSeamEvidencePassed: true,
      durableRestartPassed: true,
      durableProcessRelaunchObserved: true,
      actualPackagedSmokeRequested: true,
      actualPackagedSmokePassed: true,
      actualPackagedDesktopQAPassed: true,
      milestoneAFunctionalPassed: true,
      commercialReleaseReady: true,
      commercialReleaseStatus: 'passed',
      formalReleasePublicationAuthorityPassed: true,
      controlledReleaseNativeReceiptPassed: true,
      deterministicContractOnly: false,
      sessionSoakDeterministicContractOnly: false,
      actualPackagedSessionSoakRequested: true,
      actualPackagedSessionSoakPassed: true,
      sessionSoakActualEvidenceRequired: false,
      actualPackagedAppLaunched: true,
      settingsProfilePatchAccepted: true,
      settingsCompatPatchUsed: false,
      settingsPatchMode: 'provider-profile',
      typeScriptRetiredBackendPassed: true,
      actualPackagedAppEvidenceRequiredForFinalGate: false,
      packagingConfigPassed: true,
      directLegacyExposureCount: 0,
      internalLegacyDelegateCount: 2,
      temporaryGoProdViolationCount: 0,
      productionGoSourceCount: 28,
      retiredProductionMarkerViolationCount: 0,
      legacyUpstreamFieldKeyViolationCount: 0,
      ...packagedOverrides
    },
    checks: [
      'cutover-report',
      'gui-smoke',
      'session-soak',
      'milestone-a',
      'rollback-evidence',
      'packaging-config'
    ].map((id) => ({
      id,
      status: 'passed',
      ...(id === 'milestone-a'
        ? { reportId: 'runtime-go-packaged-milestone-a' }
        : {})
    }))
  }
  return {
    ...report,
    ...overrides,
    packaged: {
      ...report.packaged,
      ...packagedOverrides
    }
  }
}

function writeAggregatePackagedQAScript(path: string, overrides: Record<string, unknown> = {}): void {
  const report = aggregatePackagedQAReport(overrides)
  writeFileSync(path, `console.log(JSON.stringify(${JSON.stringify(report)}))\n`, 'utf8')
}

function providerErrorProbeResponse(body: Record<string, unknown>): Response | null {
  if (body.model) return null
  return new Response(JSON.stringify({
    error: {
      message: 'intentional provider error x-api-key: sk-providerErrorSecret123456 token=provider-error-token'
    }
  }), {
    status: 400,
    headers: { 'content-type': 'application/json' }
  })
}

describe('runtime-go live evidence collector', () => {
  it('resolves default provider settings from current lowercase userData before legacy uppercase path', async () => {
    const { resolveProviderSettingsPath } = await loadCollector()
    const home = tempDir()
    const lower = join(home, 'Library', 'Application Support', 'analytix', 'analytix-settings.json')
    const upper = join(home, 'Library', 'Application Support', 'Analytix', 'analytix-settings.json')

    expect(resolveProviderSettingsPath({}, {
      useDefault: true,
      homeDir: home,
      exists: (candidate: string) => candidate === lower || candidate === upper
    })).toEqual({
      path: lower,
      source: 'default-app-settings'
    })

    expect(resolveProviderSettingsPath({}, {
      useDefault: true,
      homeDir: home,
      exists: (candidate: string) => candidate === upper
    })).toEqual({
      path: upper,
      source: 'default-app-settings'
    })
  })

  it('keeps the packaged Milestone A required check closure synchronized with the formal harness', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const report = collectPackagedMilestoneA({ packaged: {}, now: '2026-08-05T00:00:00.000Z', dryRun: true })
    expect(report.requiredCheckIds).toEqual([
      'trusted-cache-tmpdir',
      'formal-packaged-artifact',
      'configured-network-provider',
      'isolated-profile-and-repository',
      'packaged-first-launch',
      'composer-workflow-submit',
      'ordinary-agent-workflow',
      'case-dependencies-unavailable-with-ordinary-capabilities',
      'real-repository-test',
      'bounded-subagent',
      'git-skill-mcp-research-writing',
      'long-context-continuation',
      'composer-compaction-submit',
      'nonzero-compaction',
      'normal-first-quit',
      'fresh-packaged-relaunch',
      'relaunch-provider-continuation',
      'renderer-visual-evidence',
      'exact-thread-recovery',
      'exact-todo-recovery',
      'exact-subagent-recovery',
      'exact-compaction-recovery',
      'exact-result-recovery',
      'renderer-visible-thread-recovery',
      'renderer-visible-todo-recovery',
      'renderer-visible-subagent-recovery',
      'renderer-visible-compaction-recovery',
      'renderer-visible-result-recovery',
      'normal-final-quit',
      'zero-residual-processes',
      'credential-redaction',
      'sandbox-cleanup'
    ])
  })

  it('validates the compatibility non-pass projection in original check order', async () => {
    const { packagedMilestoneAReportProjection } = await loadCollector()
    const base = packagedMilestoneAReport()
    const checks = [
      { id: 'fresh-packaged-relaunch', status: 'live_blocked' },
      ...(base.checks as Array<Record<string, unknown>>).filter((item) =>
        ![
          'fresh-packaged-relaunch',
          'long-context-continuation',
          'composer-compaction-submit'
        ].includes(String(item.id))
      ),
      { id: 'long-context-continuation', status: 'failed' },
      { id: 'composer-compaction-submit', status: 'skipped' }
    ]
    const report = {
      ...base,
      status: 'failed',
      passed: false,
      checks,
      failedCheckIds: ['long-context-continuation'],
      skippedCheckIds: ['composer-compaction-submit'],
      liveBlockedCheckIds: ['fresh-packaged-relaunch'],
      failureCheckIds: [
        'fresh-packaged-relaunch',
        'long-context-continuation',
        'composer-compaction-submit'
      ]
    }

    const projected = packagedMilestoneAReportProjection(report)

    expect(projected.ok).toBe(true)
    expect(projected.failureCheckIds).toEqual(report.failureCheckIds)
  })

  it('fails closed on duplicate, unknown, and conflicting Milestone A check projections', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const qa = { checks: [{ id: 'milestone-a', status: 'passed', reportId: 'runtime-go-packaged-milestone-a' }] }
    for (const checks of [
      [...(base.checks as Array<Record<string, unknown>>), { id: 'ordinary-agent-workflow', status: 'passed' }],
      [...(base.checks as Array<Record<string, unknown>>), { id: 'unknown-check', status: 'passed' }],
      (base.checks as Array<Record<string, unknown>>).map((check) =>
        check.id === 'ordinary-agent-workflow' ? { ...check, status: 'failed' } : check)
    ]) {
      const report = collectPackagedMilestoneA({
        packaged: { milestoneAEvidence: { ...base, checks } , checks: qa.checks },
        now: '2026-08-05T00:00:00.000Z',
        dryRun: false
      })
      expect(report.passed).toBe(false)
      expect(report.missingRequiredEvidence).toEqual(expect.arrayContaining([
        expect.stringMatching(/checks:.*(unique|unknown|status)/)
      ]))
    }
  })

  it('accepts only the synchronized Milestone A auxiliary checks and closes protected-funds evidence', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const workflow = base.workflow as Record<string, unknown>
    const auxiliaryChecks = [
      'external-real-repository-contract',
      'parent-owned-repository-test-cross-check',
      'protected-funds-source-unavailable'
    ].map((id) => ({ id, status: 'passed' }))
    const report = collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          workflow: {
            ...workflow,
            protectedFundsSourceUnavailableObserved: true
          },
          checks: [
            ...(base.checks as Array<Record<string, unknown>>),
            ...auxiliaryChecks
          ]
        },
        checks: [{
          id: 'milestone-a',
          status: 'passed',
          reportId: 'runtime-go-packaged-milestone-a'
        }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })
    expect(report.passed).toBe(true)
    expect(report.missingRequiredEvidence).toBeUndefined()

    const stale = collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          workflow: {
            ...workflow,
            protectedFundsSourceUnavailableObserved: false
          },
          checks: [
            ...(base.checks as Array<Record<string, unknown>>),
            ...auxiliaryChecks
          ]
        },
        checks: [{
          id: 'milestone-a',
          status: 'passed',
          reportId: 'runtime-go-packaged-milestone-a'
        }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })
    expect(stale.passed).toBe(false)
    expect(stale.missingRequiredEvidence).toContain(
      'workflow:protected-funds-source-unavailable:protectedFundsSourceUnavailableObserved'
    )
  })

  it('rejects passed Milestone A checks whose workflow projection still carries stale false evidence', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const workflow = base.workflow as Record<string, unknown>
    const report = collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          workflow: {
            ...workflow,
            ordinaryWorkflowCompleted: false,
            readObserved: false,
            realTestObserved: false,
            subagentObserved: false,
            longContextContinuationObserved: false,
            longContextHostReadBound: false,
            longContextSubagentContinuityBound: false,
            longContextProviderReceiptReplayObserved: false,
            longContextProviderReceiptBound: false,
            longContextAcceptedFinalBound: false
          }
        },
        checks: [{
          id: 'milestone-a',
          status: 'passed',
          reportId: 'runtime-go-packaged-milestone-a'
        }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })
    expect(report.passed).toBe(false)
    expect(report.missingRequiredEvidence).toEqual(expect.arrayContaining([
      'workflow:ordinary-agent-workflow:ordinaryWorkflowCompleted',
      'workflow:ordinary-agent-workflow:readObserved',
      'workflow:real-repository-test:realTestObserved',
      'workflow:bounded-subagent:subagentObserved',
      'workflow:long-context-continuation:longContextContinuationObserved',
      'workflow:long-context-continuation:longContextHostReadBound',
      'workflow:long-context-continuation:longContextSubagentContinuityBound',
      'workflow:long-context-continuation:longContextProviderReceiptReplayObserved',
      'workflow:long-context-continuation:longContextProviderReceiptBound',
      'workflow:long-context-continuation:longContextAcceptedFinalBound'
    ]))
  })

  it('rejects a passed Milestone A report when subagent continuity digests diverge', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const workflow = base.workflow as Record<string, unknown>
    const report = collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          workflow: {
            ...workflow,
            longContextSubagentContinuityProtectedFundsDigest: '9'.repeat(64)
          }
        },
        checks: [{
          id: 'milestone-a',
          status: 'passed',
          reportId: 'runtime-go-packaged-milestone-a'
        }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })
    expect(report.passed).toBe(false)
    expect(report.missingRequiredEvidence).toContain(
      'workflow.longContextSubagentContinuityDigest'
    )
  })

  it('does not let explicit missingExternalInputs hide non-pass check projections', async () => {
    const { packagedMilestoneAMissingExternalInputs } = await loadCollector()
    const report = packagedMilestoneAReport({
      status: 'failed',
      passed: false,
      checks: [
        ...(packagedMilestoneAReport().checks as Array<Record<string, unknown>>),
        { id: 'long-context-continuation', status: 'failed', message: 'continuation failed' }
      ],
      failureCheckIds: ['long-context-continuation'],
      failedCheckIds: ['long-context-continuation'],
      skippedCheckIds: [],
      liveBlockedCheckIds: [],
      missingExternalInputs: [{ id: 'external-prerequisite', status: 'live_blocked', reason: 'missing' }]
    })
    const missing = packagedMilestoneAMissingExternalInputs(report)
    expect(missing.map((item) => item.id)).toEqual(expect.arrayContaining([
      'packaged-milestone-a:external-prerequisite',
      'packaged-milestone-a:long-context-continuation'
    ]))
  })

  it('requires formal model, effort, visible credential declaration, manual compaction ancestry, and relaunch continuation evidence', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const provider = base.provider as Record<string, unknown>
    const workflow = base.workflow as Record<string, unknown>
    const report = collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          provider: {
            ...provider,
            modelHash: '',
            requiredModelHash: '',
            reasoningEffort: 'low',
            longContextEvidence: {}
          },
          operatorCheckpoint: { declared: false, method: 'undeclared', automatedCredentialEntryUsed: null },
          workflow: {
            ...workflow,
            relaunchProviderContinuationObserved: false,
            manualCompactionObserved: false,
            manualCompactionAuto: true,
            manualCompactionReplacedTokens: 0,
            manualCompactionSourceAncestryBound: false,
            longContextProviderReceiptReplayObserved: false,
            longContextProviderReceiptBound: false,
            longContextAcceptedFinalBound: false,
            longContextSubagentContinuityBound: false,
            longContextSubagentContinuityBaselineDigest: '',
            longContextSubagentContinuityProtectedFundsDigest: '',
            longContextSubagentContinuityDigest: '',
            longContextProviderReceiptDigest: ''
          }
        },
        checks: [{ id: 'milestone-a', status: 'passed', reportId: 'runtime-go-packaged-milestone-a' }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })
    expect(report.passed).toBe(false)
    expect(report.missingRequiredEvidence).toEqual(expect.arrayContaining([
      'provider.modelHash:formal-model',
      'provider.reasoningEffort:high',
      'provider.longContextEvidence.networkTurnCompleted',
      'operatorCheckpoint.declared',
      'workflow.longContextProviderReceiptReplayObserved',
      'workflow.longContextProviderReceiptBound',
      'workflow.longContextAcceptedFinalBound',
      'workflow.longContextSubagentContinuityBound',
      'workflow.longContextSubagentContinuityDigest',
      'workflow.longContextProviderReceiptDigest',
      'workflow.manualCompactionObserved',
      'workflow.manualCompactionAuto:false',
      'workflow.manualCompactionReplacedTokens',
      'workflow.manualCompactionSourceAncestryBound',
      'workflow.relaunchProviderContinuationObserved'
    ]))
  })

  it('accepts ordinary exact or case-bound typed compaction proof and rejects replay, auto, and incomplete proof', async () => {
    const { collectPackagedMilestoneA } = await loadCollector()
    const base = packagedMilestoneAReport()
    const provider = base.provider as Record<string, unknown>
    const validWorkflow = base.workflow as Record<string, unknown>
    const baselineFields = {
      manualCompactionBaselineBound: true,
      manualCompactionBaselineCount: 1,
      manualCompactionBaselineDigest: '1'.repeat(64),
      manualCompactionCandidateDigest: '2'.repeat(64),
      manualCompactionNewAfterBaseline: true
    }
    const ordinaryWorkflow = {
      ...validWorkflow,
      ...baselineFields,
      manualCompactionProjectionClass: 'ordinary_exact',
      manualCompactionNonzeroBound: true,
      manualCompactionSourceAncestryBound: true,
      manualCompactionObserved: true,
      manualCompactionAuto: false,
      manualCompactionReplacedTokens: 2048,
      manualCompactionSourceDigest: 'e'.repeat(64),
      manualCompactionSourceItemIdsDigest: 'f'.repeat(64)
    }
    const collect = (workflow: Record<string, unknown>) => collectPackagedMilestoneA({
      packaged: {
        milestoneAEvidence: {
          ...base,
          provider: { ...provider },
          workflow
        },
        checks: [{
          id: 'milestone-a',
          status: 'passed',
          reportId: 'runtime-go-packaged-milestone-a'
        }]
      },
      now: '2026-08-05T00:00:00.000Z',
      dryRun: false
    })

    expect(collect(ordinaryWorkflow).passed).toBe(true)

    const caseBoundWorkflow = {
      ...ordinaryWorkflow,
      manualCompactionProjectionClass: 'case_bound_typed',
      manualCompactionReplacedTokens: 0,
      manualCompactionSourceItemIdsDigest: '',
      manualCompactionNonzeroBound: true,
      manualCompactionSourceAncestryBound: true
    }
    expect(collect(caseBoundWorkflow).passed).toBe(true)

    const invalidCases: Array<[string, Record<string, unknown>]> = [
      ['auto', { manualCompactionAuto: true }],
      ['replay', { manualCompactionNewAfterBaseline: false }],
      ['missing schema proof', { manualCompactionProjectionClass: 'case_bound_typed', manualCompactionNonzeroBound: false }],
      ['missing digest', { manualCompactionSourceDigest: '' }],
      ['ordinary zero', { manualCompactionProjectionClass: 'ordinary_exact', manualCompactionReplacedTokens: 0 }],
      ['ordinary empty ancestry', { manualCompactionProjectionClass: 'ordinary_exact', manualCompactionSourceItemIdsDigest: '', manualCompactionSourceAncestryBound: false }]
    ]
    for (const [label, overrides] of invalidCases) {
      const report = collect({
        ...ordinaryWorkflow,
        ...overrides
      })
      expect(report.passed, label).toBe(false)
      expect(report.missingRequiredEvidence).toEqual(expect.arrayContaining([
        expect.stringMatching(/workflow\.(?:manualCompaction|nonzero-compaction)|workflow:nonzero-compaction/)
      ]))
    }
  })

  it('reports the current lowercase provider settings path when no default settings file exists', async () => {
    const { resolveProviderSettingsPath } = await loadCollector()
    const home = tempDir()

    expect(resolveProviderSettingsPath({}, { useDefault: true, homeDir: home })).toEqual({
      path: join(home, 'Library', 'Application Support', 'analytix', 'analytix-settings.json'),
      source: 'default-app-settings'
    })
  })

  it('passes the live evidence timeout and no-write mode to packaged child evidence commands', async () => {
    const { collectPackagedQA, collectPackagedGuiSmoke, collectPackagedSessionSoak } = await loadCollector()
    const dir = tempDir()
    const capturePath = join(dir, 'argv-capture.json')
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeArgvCaptureScript(packagedScript, 'runtime-go-packaged-qa', 'packaged-desktop-qa')
    writeArgvCaptureScript(packagedGuiScript, 'runtime-go-packaged-gui-smoke', 'packaged-gui-bridge-smoke')
    writeArgvCaptureScript(packagedSoakScript, 'runtime-go-packaged-session-soak', 'packaged-session-soak')
    const env = { ...process.env, ARGV_CAPTURE_PATH: capturePath }

    collectPackagedQA({
      env,
      now: '2026-06-28T00:00:00.000Z',
      noWrite: true,
      timeoutMs: 1234,
      packagedScript
    })
    collectPackagedGuiSmoke({
      env,
      now: '2026-06-28T00:00:00.000Z',
      noWrite: true,
      timeoutMs: 1234,
      packagedGuiScript
    })
    collectPackagedSessionSoak({
      env,
      now: '2026-06-28T00:00:00.000Z',
      noWrite: true,
      timeoutMs: 1234,
      packagedSoakScript
    })

    const entries = JSON.parse(readFileSync(capturePath, 'utf8')) as Array<{ id: string, argv: string[] }>
    expect(entries.map((entry) => entry.id)).toEqual([
      'runtime-go-packaged-qa',
      'runtime-go-packaged-gui-smoke',
      'runtime-go-packaged-session-soak'
    ])
    for (const entry of entries) {
      expect(entry.argv).toEqual(expect.arrayContaining(['--timeout-ms', '1234']))
      expect(entry.argv).toContain('--no-write')
    }
  })

  it('requests actual packaged aggregate evidence from the formal packaged QA runner', async () => {
    const { collectPackagedQA } = await loadCollector()
    const dir = tempDir()
    const capturePath = join(dir, 'argv-capture.json')
    const packagedScript = join(dir, 'runtime-go-packaged-qa.mjs')
    writeArgvCaptureScript(packagedScript, 'runtime-go-packaged-qa', 'packaged-desktop-qa')

    collectPackagedQA({
      env: { ...process.env, ARGV_CAPTURE_PATH: capturePath },
      now: '2026-06-28T00:00:00.000Z',
      timeoutMs: 1234,
      packagedScript
    })

    const entries = JSON.parse(readFileSync(capturePath, 'utf8')) as Array<{ id: string, argv: string[] }>
    expect(entries).toHaveLength(1)
    expect(entries[0].argv).toEqual(expect.arrayContaining(['--json', '--actual', '--timeout-ms', '1234']))
  })

  it('requests actual packaged session soak evidence from the formal session soak runner', async () => {
    const { collectPackagedSessionSoak } = await loadCollector()
    const dir = tempDir()
    const capturePath = join(dir, 'argv-capture.json')
    const packagedSoakScript = join(dir, 'runtime-go-packaged-session-soak.mjs')
    writeArgvCaptureScript(packagedSoakScript, 'runtime-go-packaged-session-soak', 'packaged-session-soak')

    collectPackagedSessionSoak({
      env: { ...process.env, ARGV_CAPTURE_PATH: capturePath },
      now: '2026-06-28T00:00:00.000Z',
      timeoutMs: 1234,
      packagedSoakScript
    })

    const entries = JSON.parse(readFileSync(capturePath, 'utf8')) as Array<{ id: string, argv: string[] }>
    expect(entries).toHaveLength(1)
    expect(entries[0].argv).toEqual(expect.arrayContaining(['--json', '--actual', '--timeout-ms', '1234']))
  })

  it('keeps provider and packaged evidence timeouts separate in full live evidence reports', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const capturePath = join(dir, 'argv-capture.json')
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeArgvCaptureScript(packagedScript, 'runtime-go-packaged-qa', 'packaged-desktop-qa')
    writeArgvCaptureScript(packagedGuiScript, 'runtime-go-packaged-gui-smoke', 'packaged-gui-bridge-smoke')
    writeArgvCaptureScript(packagedSoakScript, 'runtime-go-packaged-session-soak', 'packaged-session-soak')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: { ...process.env, ARGV_CAPTURE_PATH: capturePath },
      reportDir: dir,
      now: '2026-06-28T00:00:00.000Z',
      commitHash: 'test-commit',
      noWrite: true,
      timeoutMs: 1234,
      packagedTimeoutMs: 4321,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    expect(report).toEqual(expect.objectContaining({
      timeoutMs: 1234,
      packagedTimeoutMs: 4321
    }))
    const entries = JSON.parse(readFileSync(capturePath, 'utf8')) as Array<{ id: string, argv: string[] }>
    expect(entries).toHaveLength(3)
    for (const entry of entries) {
      expect(entry.argv).toEqual(expect.arrayContaining(['--timeout-ms', '4321']))
      expect(entry.argv).not.toEqual(expect.arrayContaining(['--timeout-ms', '1234']))
      expect(entry.argv).toContain('--no-write')
    }
  })

  it('uses a release-sized default packaged evidence timeout', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const capturePath = join(dir, 'argv-capture.json')
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeArgvCaptureScript(packagedScript, 'runtime-go-packaged-qa', 'packaged-desktop-qa')
    writeArgvCaptureScript(packagedGuiScript, 'runtime-go-packaged-gui-smoke', 'packaged-gui-bridge-smoke')
    writeArgvCaptureScript(packagedSoakScript, 'runtime-go-packaged-session-soak', 'packaged-session-soak')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: { ...process.env, ARGV_CAPTURE_PATH: capturePath },
      reportDir: dir,
      now: '2026-06-28T00:00:00.000Z',
      commitHash: 'test-commit',
      timeoutMs: 1234,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    expect(report).toEqual(expect.objectContaining({
      timeoutMs: 1234,
      packagedTimeoutMs: 600_000
    }))
    const entries = JSON.parse(readFileSync(capturePath, 'utf8')) as Array<{ id: string, argv: string[] }>
    expect(entries).toHaveLength(3)
    for (const entry of entries) {
      expect(entry.argv).toEqual(expect.arrayContaining(['--timeout-ms', '600000']))
      expect(entry.argv).not.toEqual(expect.arrayContaining(['--timeout-ms', '1234']))
    }
  })

  it('keeps hard gate scripts in operator relevant-clean evidence scope', () => {
    const collector = readFileSync(join(process.cwd(), 'scripts/runtime-go-live-evidence-collector.mjs'), 'utf8')
    const helper = readFileSync(join(process.cwd(), 'scripts/runtime-go-worktree-audit.mjs'), 'utf8')
    const pathsStart = helper.indexOf('export const runtimeGoRelevantEvidencePaths = [')
    const expectedPaths = [
      'package.json',
      'packages/runtime/package.json',
      'scripts/after-pack.cjs',
      'scripts/cache-first-review-gate.mjs',
      'scripts/scan-product-sovereignty.cjs',
      'scripts/runtime-go-validation-command.mjs',
      'scripts/runtime-go-validation-delegate.mjs',
      'scripts/runtime-go-worktree-audit.mjs',
      'scripts/runtime-go-preflight.mjs',
      'scripts/runtime-go-default-readiness-report.mjs',
      'scripts/runtime-go-cutover-report.mjs',
      'scripts/runtime-go-live-evidence-collector.mjs',
      'scripts/runtime-go-local-validation.mjs',
      'scripts/runtime-go-runtime-health-smoke.mjs',
      'scripts/runtime-go-performance-check.mjs',
      'scripts/runtime-go-speed-cache-gate.mjs',
      'scripts/runtime-go-product-regression.mjs',
      'scripts/runtime-go-packaged-milestone-a.mjs',
      'src/main/runtime-go-packaged-milestone-a.test.ts',
      'src/main/cache-first-review-gate.test.ts',
      'src/main/runtime-go-live-validation.test.ts',
      'src/main/runtime-go-worktree-audit.test.ts',
      'src/main/runtime/runtime-go-live-evidence-collector.test.ts'
    ]

    expect(pathsStart).toBeGreaterThan(0)
    for (const expectedPath of expectedPaths) {
      const pathIndex = helper.indexOf(`'${expectedPath}'`)
      expect(pathIndex).toBeGreaterThan(pathsStart)
    }
    expect(helper).toContain('evidence:relevant-clean-scope-dirty')
    expect(collector).toContain('relevantEvidenceFinalGateBlockers')
    expect(collector).toContain('worktreeAudit')
    expect(collector).toContain("failureReasons.push('operator evidence target has uncommitted runtime evidence changes')")
  })

  it('writes live_blocked evidence when real provider, MCP, packaged, and operator inputs are missing', async () => {
    const { buildRuntimeGoLiveEvidenceReport, strictGateEnv } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeFileSync(packagedScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-qa',
  stage: 'packaged-desktop-qa',
  status: 'failed',
  passed: false,
  goDefaultBackendEnabled: true,
  rendererVisibleGoSwitcher: false,
  typeScriptFallbackRetained: false,
  credentialSecretsRecorded: false,
  redaction: { status: 'passed', secretMaterialFound: false },
  checks: [
    { id: 'packaged-app-startup', status: 'failed', message: 'missing ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH' },
    { id: 'health', status: 'failed', message: 'missing ANALYTIX_RUNTIME_GO_RUNTIME_URL' },
    { id: 'runtime-info', status: 'failed', message: 'missing ANALYTIX_RUNTIME_GO_RUNTIME_URL' },
    { id: 'thread-list', status: 'failed' },
    { id: 'turn-create', status: 'failed' },
    { id: 'sse-replay', status: 'failed' },
    { id: 'go-runtime-default-gate', status: 'failed' },
    { id: 'typescript-retired-backend', status: 'failed' }
  ]
}))
`, 'utf8')
    writeFileSync(packagedGuiScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  stage: 'packaged-gui-bridge-smoke',
  status: 'failed',
  passed: false,
  smoke: { actualPackagedSmokeRequested: true, actualPackagedSmokePassed: false },
  actualPackagedSmoke: {
    status: 'failed',
    passed: false,
    checks: [{ id: 'packaged-app-artifact', status: 'failed', message: 'missing packaged app path' }],
    redaction: { status: 'passed', secretMaterialFound: false }
  }
}))
`, 'utf8')
    writeFileSync(packagedSoakScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-session-soak',
  stage: 'packaged-session-soak',
  status: 'failed',
  passed: false,
  soak: {
    deterministicContractOnly: true,
    goDurableSessionCovered: false,
    typeScriptRetiredBackendSessionCovered: false,
    resumeForkPairingCovered: false,
    sseReplayCovered: false
  },
  checks: [{ id: 'go-session-durable-contract', status: 'failed', reason: 'missing packaged app' }]
}))
`, 'utf8')
    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    expect(report).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      defaultBackendReady: true,
      goDefaultRuntimeAllowed: true,
      typeScriptFallbackRetained: false,
      llmAnswerQualityEvidenceUsed: false,
      liveOrActualEvidenceUsed: false,
      deterministicEvidenceOnly: true,
      dryRun: false,
      noWrite: false,
      persistentEvidenceWritten: true,
      finalGateBlocked: true,
      finalGateBlockers: expect.arrayContaining([
        'provider:deepseek',
        'provider:at-least-one-non-deepseek-provider',
        'mcp:credentialed-execution',
        'operator:gate'
      ]),
      missingExternalInputIds: expect.arrayContaining([
        'provider:deepseek',
        'provider:at-least-one-non-deepseek-provider',
        'mcp:credentialed-execution',
        'operator:gate'
      ]),
      nextEvidenceActions: expect.objectContaining({
        credentialSecretsRecorded: false,
        valuesRecorded: false,
        commands: expect.arrayContaining([
          'npm run runtime:go:approval-user-input-evidence -- --json',
          'npm run runtime:go:live-evidence -- --json',
          'npm run runtime:go:live-validation -- --json --live-deepseek-cache-from-settings --live-non-deepseek-provider-from-settings --no-gate',
          'npm run runtime:go:preflight'
        ])
      })
    }))
    expect((report.missingExternalInputs as unknown[]).length).toBeGreaterThan(0)
    expect(JSON.stringify(report.missingExternalInputs)).toContain('ANALYTIX_RUNTIME_DEEPSEEK_API_KEY')
    expect(JSON.stringify(report.missingExternalInputs)).toContain('ANALYTIX_RUNTIME_MCP_COMMAND')
    const operatorMissingInputs = (report.missingExternalInputs as Array<Record<string, unknown>>)
      .filter((item) => item.id === 'operator:gate')
    expect(operatorMissingInputs).toHaveLength(1)
    expect(operatorMissingInputs[0]).toEqual(expect.objectContaining({
      reason: 'operator gate requirements are not satisfied',
      reasons: expect.arrayContaining([
        'ANALYTIX_RUNTIME_READY is not set to 1',
        'operator post-cutover live-validation approval env is not set'
      ])
    }))
    expect(report.missingExternalInputs).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'provider:deepseek',
        missingEnv: expect.arrayContaining([
          'ANALYTIX_RUNTIME_DEEPSEEK_API_KEY',
          'ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL',
          'ANALYTIX_RUNTIME_DEEPSEEK_MODEL'
        ])
      }),
      expect.objectContaining({
        id: 'provider:at-least-one-non-deepseek-provider',
        candidateProviderIds: expect.arrayContaining([
          'openai-compatible',
          'anthropic-compatible',
          'custom-endpoint'
        ])
      })
    ]))
    expect(report.nextEvidenceActions).toEqual(expect.objectContaining({
      providerMatrix: expect.objectContaining({
        evidencePath: join(dir, 'runtime-go-live-evidence/provider-matrix.json'),
        completionOptions: expect.objectContaining({
          credentialValuesRecorded: false,
          settingsProfilePath: 'provider.providers[]',
          requiredNonDeepSeekSettingsFields: expect.arrayContaining([
            'apiKey',
            'endpointFormat or recognized preset/model profile default',
            'baseUrl or recognized preset default',
            'models[] or runtime.model or recognized preset default'
          ]),
          recognizedPresetProviderIds: expect.arrayContaining([
            'minimax',
            'litellm',
            'aliyun',
            'moonshot-cn',
            'moonshot-global',
            'xiaomi',
            'xiaomi-token-plan',
            'minimax-token-plan',
            'tencentcloud',
            'tencentcloud-token-plan',
            'volcengine-coding-plan'
          ]),
          acceptedNonDeepSeekProfileSignals: expect.arrayContaining([
            expect.stringContaining('openai-compatible'),
            expect.stringContaining('anthropic-compatible'),
            expect.stringContaining('custom-endpoint')
          ]),
          envCompletionGroups: expect.arrayContaining([
            expect.objectContaining({
              id: 'openai-compatible',
              requiredEnv: expect.arrayContaining([
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL',
                'ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL'
              ])
            })
          ])
        }),
        requiredEnvGroups: expect.arrayContaining([
          expect.objectContaining({
            id: 'custom-endpoint',
            endpointFormat: 'custom-full-endpoint',
            env: {
              apiKeyEnv: 'ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY',
              baseUrlEnv: 'ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL',
              modelEnv: 'ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL'
            },
            acceptedAliases: {
              apiKeyEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_API_KEY'],
              baseUrlEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_URL'],
              modelEnv: ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_MODEL']
            }
          })
        ])
      }),
      mcpExecution: expect.objectContaining({
        requiredCoverage: expect.arrayContaining(['connect', 'toolCall', 'approvalUserInput']),
        topLevelMcpIndexerExposed: false
      }),
      packagedDesktopQa: expect.objectContaining({
        requiredCheckIds: expect.arrayContaining(['packaged-app-startup', 'typescript-retired-backend'])
      }),
      packagedGuiBridgeSmoke: expect.objectContaining({
        requiredCheckIds: expect.arrayContaining(['packaged-renderer-bridge', 'packaged-runtime-restart', 'packaged-runtime-info'])
      }),
      packagedSessionSoak: expect.objectContaining({
        requiredCheckIds: expect.arrayContaining([
          'go-session-durable-contract',
          'typescript-retired-backend-session-contract',
          'packaged-session-mimo-plan',
          'packaged-session-attachment-fallback',
          'packaged-session-approval-user-input'
        ])
      }),
      operatorGate: expect.objectContaining({
        dependencySummary: expect.objectContaining({
          envGate: {
            runtimeReady: false,
            operatorApprovesDefault: false
          },
          evidence: expect.objectContaining({
            providerPassed: false,
            mcpPassed: false,
            packagedPassed: false,
            credentialedEvidenceReviewed: false
          }),
          blockers: expect.arrayContaining([
            'provider matrix evidence is not passed',
            'MCP execution evidence is not passed',
            'ANALYTIX_RUNTIME_READY is not set to 1',
            'operator post-cutover live-validation approval env is not set'
          ])
        }),
        requiredEnv: {
          g6Ready: 'ANALYTIX_RUNTIME_READY=1',
          approval: 'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT=1'
        }
      })
    }))
    expect(JSON.stringify(report.nextEvidenceActions)).not.toContain('test-key')

    const paths = report.outputPaths as Record<string, string>
    const markdown = readFileSync(paths.markdown, 'utf8')
    expect(markdown.split('\n').slice(0, 6).join('\n')).toContain('Current status: Go runtime default evidence')
    expect(markdown.split('\n').slice(0, 6).join('\n')).toContain('ANALYTIX_RUNTIME_BACKEND=typescript')
    expect(markdown).toContain('## Final Gate')
    expect(markdown).toContain('- Blocked: true')
    expect(markdown).toContain('provider:deepseek')
    expect(markdown).toContain('provider:at-least-one-non-deepseek-provider')
    expect(markdown).toContain('## Provider Settings Coverage')
    expect(markdown).toContain('- Settings source: not-configured')
    expect(markdown).toContain('- Provider profile count: 0')
    expect(markdown).toContain('- Usable non-DeepSeek provider ids: none')
    expect(markdown).toContain('- Satisfies final provider coverage from settings: false')
    expect(markdown).toContain('## Provider Completion Options')
    expect(markdown).toContain('- Settings profile path: provider.providers[]')
    expect(markdown).toContain('- Required non-DeepSeek settings fields:')
    expect(markdown).toContain('baseUrl or recognized preset default')
    expect(markdown).toContain('- Recognized preset provider ids:')
    expect(markdown).toContain('minimax')
    expect(markdown).toContain('openai-compatible')
    expect(markdown).toContain('ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY')
    const provider = JSON.parse(readFileSync(paths.provider, 'utf8')) as Record<string, unknown>
    const mcp = JSON.parse(readFileSync(paths.mcp, 'utf8')) as Record<string, unknown>
    const packaged = JSON.parse(readFileSync(paths.packaged, 'utf8')) as Record<string, unknown>
    const packagedGui = JSON.parse(readFileSync(paths.packagedGui, 'utf8')) as Record<string, unknown>
    const packagedSoak = JSON.parse(readFileSync(paths.packagedSoak, 'utf8')) as Record<string, unknown>
    const packagedMilestoneA = JSON.parse(
      readFileSync(paths.packagedMilestoneA, 'utf8')
    ) as Record<string, unknown>
    const operator = JSON.parse(readFileSync(paths.operator, 'utf8')) as Record<string, unknown>

    expect(report.evidenceDigestAlgorithm).toBe('sha256:canonical-json-v1')
    expect(report.evidenceDigests).toEqual({
      provider: sha256(provider),
      mcp: sha256(mcp),
      packaged: sha256(packaged),
      packagedGui: sha256(packagedGui),
      packagedSoak: sha256(packagedSoak),
      packagedMilestoneA: sha256(packagedMilestoneA),
      operator: sha256(operator)
    })
    expect(provider).toEqual(expect.objectContaining({
      id: 'runtime-go-provider-matrix',
      passed: false,
      credentialSecretsRecorded: false
    }))
    expect(provider).not.toHaveProperty('legacyId')
    expect(mcp).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'live_blocked',
      topLevelMcpIndexerExposed: false,
      reasonixPublicProtocolUsed: false
    }))
    expect(mcp).not.toHaveProperty('legacyId')
    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      passed: false
    }))
    expect(packaged).not.toHaveProperty('legacyId')
    expect(packagedGui).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-gui-smoke',
      passed: false
    }))
    expect(packagedSoak).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-session-soak',
      passed: false
    }))
    expect(packagedMilestoneA).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-milestone-a',
      passed: false
    }))
    expect(operator).toEqual(expect.objectContaining({
      id: 'runtime-go-operator-gate',
      status: 'live_blocked',
      goDefaultApproved: false
    }))
    expect(operator).not.toHaveProperty('legacyId')
  })

  it('does not overwrite persisted live evidence in dry-run mode', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      dryRun: true
    })

    expect(report).toEqual(expect.objectContaining({
      status: 'live_blocked',
      deterministicEvidenceOnly: true,
      liveOrActualEvidenceUsed: false,
      dryRun: true,
      noWrite: true,
      persistentEvidenceWritten: false,
      outputPaths: {},
      evidencePaths: {}
    }))
    for (const relativePath of [
      'live-evidence-report.json',
      'provider-matrix.json',
      'mcp-execution.json',
      'packaged-go-runtime-qa.json',
      'packaged-gui-bridge-smoke.json',
      'packaged-session-soak.json',
      'packaged-milestone-a.json',
      'operator-gate.json',
      'live-evidence-summary.md'
    ]) {
      expect(existsSync(join(dir, 'runtime-go-live-evidence', relativePath))).toBe(false)
    }
  })

  it('preserves existing passed MCP evidence when no new MCP live probe is configured', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const evidenceDir = join(dir, 'runtime-go-live-evidence')
    const mcpPath = join(evidenceDir, 'mcp-execution.json')
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    mkdirSync(evidenceDir, { recursive: true })
    writeFileSync(mcpPath, JSON.stringify(passedMCPEvidence(), null, 2), 'utf8')
    writeAggregatePackagedQAScript(packagedScript)
    writeFileSync(packagedGuiScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  status: 'passed',
  passed: true,
  smoke: {
    bridgePreserved: true,
    sseBridgePreserved: true,
    rendererTimelinePreserved: true,
    approvalAndUserInputCardsCovered: true,
    settingsProfilePatchAccepted: true,
    settingsCompatPatchUsed: false,
    actualPackagedSmokeRequested: false,
    actualPackagedSmokePassed: false
  },
  checks: [{ id: 'gui-bridge-and-timeline-smoke', status: 'passed' }]
}))
`, 'utf8')
    writeFileSync(packagedSoakScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-session-soak',
  status: 'passed',
  passed: true,
  soak: {
    deterministicContractOnly: true,
    actualPackagedAppLaunched: false,
    actualPackagedAppEvidenceRequiredForFinalGate: true,
    goDurableSessionCovered: true,
    typeScriptRetiredBackendSessionCovered: true,
    resumeForkPairingCovered: true,
    sseReplayCovered: true
  },
  checks: [
    { id: 'go-session-durable-contract', status: 'passed' },
    { id: 'typescript-retired-backend-session-contract', status: 'passed' }
  ]
}))
`, 'utf8')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript,
      fetchImpl: async (): Promise<Response> => new Response('{}', { status: 500 })
    })

    expect(report.componentStatus).toEqual(expect.objectContaining({
      mcp: 'passed'
    }))
    const persisted = JSON.parse(readFileSync(mcpPath, 'utf8')) as Record<string, unknown>
    expect(persisted).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'passed',
      passed: true,
      credentialedExecution: true
    }))
    expect(JSON.stringify(report.missingExternalInputs)).not.toContain('mcp:credentialed-execution')
  })

  it('emits only current strict gate env names when every live evidence component passes', async () => {
    const { buildRuntimeGoLiveEvidenceReport, strictGateEnv } = await loadCollector()
    const dir = tempDir()
    const approvalPath = join(dir, 'approval.json')
    const mcpServerPath = join(dir, 'mock-mcp-current-env.mjs')
    const packagedScript = join(dir, 'mock-packaged-qa.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeApprovalEvidence(approvalPath)
    writeAggregatePackagedQAScript(packagedScript)
    writeFileSync(packagedGuiScript, `
const checks = ${JSON.stringify([
  'packaged-app-artifact',
  'packaged-app-launch',
  'packaged-renderer-bridge',
  'packaged-settings-bridge',
  'packaged-provider-profile-settings',
  'packaged-mimo-profile-settings',
  'packaged-runtime-restart',
  'packaged-runtime-health',
  'packaged-runtime-info'
])}.map((id) => ({ id, status: 'passed' }))
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  status: 'passed',
  passed: true,
  smoke: {
    deterministicContractOnly: false,
    actualPackagedAppLaunched: true,
    actualPackagedAppEvidenceRequiredForFinalGate: false,
    actualPackagedSmokeRequested: true,
    actualPackagedSmokePassed: true,
    settingsProfilePatchAccepted: true,
    mimoProfileSettingsAccepted: true,
    settingsCompatPatchUsed: false
  },
  actualPackagedSmoke: {
    status: 'passed',
    passed: true,
    checks,
    app: {
      executableExists: true,
      runtimeServerBinaryExists: true,
      appAsarExists: true,
      bundledRuntimeGoSourcePresent: false
    },
    renderer: {
      apiPresent: true,
      title: 'Analytix',
      legacyKun: false,
      legacyReasonix: false,
      settingsOk: true,
      settingsRuntimeTopLevel: true,
      settingsProfilePatchAccepted: true,
      mimoProfileSettingsOk: true,
      settingsCompatPatchUsed: false,
      restartOk: true,
      healthStatus: 200,
      healthService: 'analytix',
      runtimeInfoStatus: 200,
      runtimeInfoProductClean: true,
      runtimeInfoHasLegacyMcpLocalMarker: false,
      runtimeInfoHasLegacyAbsorptionMarker: false,
      runtimeInfoHasReasonixSurface: false
    },
    redaction: { status: 'passed', secretMaterialFound: false }
  }
}))
`, 'utf8')
    writeFileSync(packagedSoakScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-session-soak',
  status: 'passed',
  passed: true,
  soak: {
    deterministicContractOnly: true,
    actualPackagedAppLaunched: false,
    actualPackagedAppEvidenceRequiredForFinalGate: true,
    goDurableSessionCovered: true,
    typeScriptRetiredBackendSessionCovered: true,
    resumeForkPairingCovered: true,
    sseReplayCovered: true
  },
  checks: [
    { id: 'go-session-durable-contract', status: 'passed' },
    { id: 'typescript-retired-backend-session-contract', status: 'passed' }
  ]
}))
`, 'utf8')
    writeFileSync(mcpServerPath, `
import readline from 'node:readline'
const rl = readline.createInterface({ input: process.stdin })
rl.on('line', (line) => {
  const msg = JSON.parse(line)
  if (msg.method === 'notifications/initialized') return
  if (msg.method === 'initialize') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { protocolVersion: '2025-11-25', capabilities: {}, serverInfo: { name: 'mock', version: '1.0.0' } } }))
    return
  }
  if (msg.method === 'tools/list') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { tools: [{ name: 'current_tool', inputSchema: { type: 'object' } }] } }))
    return
  }
  if (msg.method === 'tools/call') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { content: [{ type: 'text', text: 'ok' }] } }))
  }
})
`, 'utf8')
    const fetchImpl = async (_url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example/v1',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
        ANALYTIX_RUNTIME_MCP_COMMAND: `${JSON.stringify(process.execPath)} ${JSON.stringify(mcpServerPath)}`,
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'current_tool',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS: 'passed',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: approvalPath,
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
      },
      fetchImpl,
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    expect(report).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      liveOrActualEvidenceUsed: true,
      deterministicEvidenceOnly: false,
      llmAnswerQualityEvidenceUsed: false,
      componentStatus: expect.objectContaining({
        provider: 'passed',
        mcp: 'passed',
        packaged: 'passed',
        packagedGui: 'passed',
        packagedSoak: 'passed',
        packagedMilestoneA: 'passed',
        operator: 'live_blocked'
      }),
      finalGateBlocked: true,
      finalGateBlockers: [
        'packaged-soak:actual-packaged-session-soak',
        'operator:gate'
      ],
      missingExternalInputIds: [
        'packaged-soak:actual-packaged-session-soak',
        'operator:gate'
      ]
    }))
    expect(report.missingExternalInputs).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'packaged-soak:actual-packaged-session-soak',
        requiredEvidence: expect.arrayContaining([
          'soak.actualPackagedAppEvidenceRequiredForFinalGate:false',
          'actual packaged app session resume/fork/SSE replay/attachment fallback soak'
        ])
      }),
      expect.objectContaining({
        id: 'operator:gate',
        reasons: expect.arrayContaining([
          'actual packaged session soak evidence is required for final gate'
        ])
      })
    ]))
    expect(report.nextEvidenceActions).toEqual(expect.objectContaining({
      packagedSessionSoak: expect.objectContaining({
        finalGateActualEvidenceRequired: true,
        finalGateMissingRequiredEvidence: expect.arrayContaining([
          'soak.actualPackagedAppEvidenceRequiredForFinalGate:false',
          'actual packaged app session resume/fork/SSE replay/attachment fallback soak'
        ])
      })
    }))
    expect(report.strictGateEnvWhenPassed).toEqual({})
    expect(report.coveredFinalCoverageEnv).toEqual(expect.objectContaining({
      ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
      ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
      ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed'
    }))
    expect(report.coveredFinalCoverageEnv).not.toHaveProperty('ANALYTIX_RUNTIME_READY')
    const paths = report.outputPaths as Record<string, string>
    const currentStrictGateEnv = strictGateEnv({
      paths: {
        provider: paths.provider,
        mcp: paths.mcp,
        packaged: paths.packaged,
        packagedGui: paths.packagedGui,
        packagedSoak: paths.packagedSoak,
        packagedMilestoneA: paths.packagedMilestoneA,
        operator: paths.operator
      }
    })
    expect(currentStrictGateEnv).toEqual(expect.objectContaining({
      ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS: 'passed',
      ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS: 'passed',
      ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS: 'passed',
      ANALYTIX_RUNTIME_MCP_MATRIX_STATUS: 'passed',
      ANALYTIX_RUNTIME_PACKAGED_QA_STATUS: 'passed',
      ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_STATUS: 'passed',
      ANALYTIX_RUNTIME_READY: '1'
    }))
    const strictGateEnvText = JSON.stringify(currentStrictGateEnv)
    expect(strictGateEnvText).not.toContain('D024')
    expect(strictGateEnvText).not.toContain('D025')
    expect(strictGateEnvText).not.toContain('ANALYTIX_GO_RUNTIME_G6_READY')
    const notes = (report.notes as string[]).join('\n')
    expect(notes).toContain('Pending live validation inputs: actual packaged session soak, operator approval.')
    expect(notes).not.toContain('Missing provider credentials')
    expect(notes).not.toContain('MCP execution configuration')
  })

  it('keeps custom endpoint as a full path and scopes DeepSeek cache telemetry to DeepSeek', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const customEndpoint = 'https://user:secret@custom.example/full/path?api_key=secret&signature=supersecret'
    const seenUrls: string[] = []
    const seenBodies: Record<string, unknown>[] = []
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      seenUrls.push(url)
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seenBodies.push(body)
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL: 'https://anthropic.example/v1',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL: 'claude-compatible',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL: customEndpoint,
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'custom-compatible'
      }
    })

    expect(seenUrls).toContain(customEndpoint)
    expect(seenUrls).not.toContain(`${customEndpoint}/chat/completions`)
    expect(seenUrls).toContain('https://openai.example/v1/chat/completions')
    for (const body of seenBodies) {
      expect(body).not.toHaveProperty('prefix_cache')
      expect(body).not.toHaveProperty('cache_prefix')
      expect(body).not.toHaveProperty('prompt_cache_key')
    }

    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const deepseek = probes.find((probe) => probe.id === 'deepseek')
    const openai = probes.find((probe) => probe.id === 'openai-compatible')
    const custom = probes.find((probe) => probe.id === 'custom-endpoint')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true
    }))
    expect(deepseek?.cacheTelemetry).toEqual(expect.objectContaining({
      status: 'passed',
      providerScoped: true,
      deepSeekSpecificRequestFieldsPresent: false
    }))
    expect(openai?.cacheTelemetry).toEqual(expect.objectContaining({
      status: 'passed',
      providerScoped: false,
      deepSeekSpecificRequestFieldsPresent: false
    }))
    for (const probe of probes) {
      expect(probe.errorHandling).toEqual(expect.objectContaining({
        status: 'passed',
        httpStatus: 400,
        errorProbePassed: true,
        rawResponseRecorded: false,
        credentialValueRecorded: false,
        responseSummary: expect.stringContaining('x-api-key: [REDACTED]')
      }))
      expect(probe.errorHandling).toEqual(expect.objectContaining({
        requestShape: expect.objectContaining({
          sameRequestUrl: true,
          intentionallyInvalidBody: true,
          credentialValueRecorded: false,
          requestBodyRecorded: false
        })
      }))
    }
    expect(custom?.requestShape).toEqual(expect.objectContaining({
      requestUrl: 'https://custom.example/full/path',
      urlPolicy: 'custom-full-endpoint-no-append'
    }))
    expect(custom?.requestUrl).toBe('https://custom.example/full/path')
    expect(custom?.requestUrlHash).toBe(sha256Text('https://custom.example/full/path'))
    expect(custom?.requestUrlHash).not.toBe(sha256Text(customEndpoint))
    expect(JSON.stringify(matrix)).not.toContain('user:secret')
    expect(JSON.stringify(matrix)).not.toContain('api_key=secret')
    expect(JSON.stringify(matrix)).not.toContain('signature=supersecret')
    expect(JSON.stringify(matrix)).not.toContain('sk-providerErrorSecret123456')
    expect(JSON.stringify(matrix)).not.toContain('provider-error-token')
  })

  it('hydrates DeepSeek live provider probe from top-level settings provider profile without recording the key', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const dir = tempDir()
    const settingsPath = join(dir, 'analytix-settings.json')
    const seen: Array<{ url: string; authorization: string; body: Record<string, unknown> }> = []
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'deepseek',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'openai-compatible',
            name: 'OpenAI Compatible',
            apiKey: '',
            baseUrl: 'https://openai.example/v1',
            endpointFormat: 'chat_completions',
            models: ['gpt-compatible']
          },
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'sk-settingsDeepSeekSecret1234567890',
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-v4-flash', 'deepseek-v4-pro']
          }
        ]
      },
      runtime: {
        providerId: 'deepseek',
        apiKey: '',
        model: 'deepseek-v4-pro'
      }
    }), 'utf8')
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seen.push({
        url,
        authorization: String((init.headers as Record<string, string>).authorization || ''),
        body
      })
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {},
      settingsPath,
      settingsPathSource: 'default-app-settings'
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const deepseek = probes.find((probe) => probe.id === 'deepseek')
    const openai = probes.find((probe) => probe.id === 'openai-compatible')

    expect(deepseek).toEqual(expect.objectContaining({
      status: 'passed',
      missingEnv: [],
      provider: expect.objectContaining({
        activeProviderId: 'deepseek',
        providerId: 'deepseek',
        model: 'deepseek-v4-pro',
        hasApiKey: true,
        source: 'settings-profile',
        settingsProfileMatched: true,
        settingsProfileId: 'deepseek',
        settingsProviderCount: 2
      })
    }))
    expect(openai).toEqual(expect.objectContaining({
      status: 'skipped',
      missingEnv: expect.arrayContaining(['ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY']),
      missingSettingsProfileFields: ['provider.providers[].apiKey'],
      message: 'missing credential/baseUrl/model from env or settings provider profile; skipped without counting as pass',
      provider: expect.objectContaining({
        source: 'settings-profile',
        settingsProfileMatched: true,
        settingsProfileUsable: false,
        hasApiKey: false
      })
    }))
    expect(matrix).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      credentialedMatrixSettingsProfileGated: true,
      readsRealApiKeysByDefault: true,
      settingsProfileCredentialsRead: true,
      credentialValuesRecorded: false,
      settingsProfile: expect.objectContaining({
        status: 'loaded',
        source: 'default-app-settings',
        activeProviderId: 'deepseek',
        selectedProviderId: 'deepseek',
        providerCount: 2,
        settingsPathConfigured: true,
        settingsPathHash: expect.stringMatching(/^[a-f0-9]{64}$/)
      }),
      settingsProfileCoverage: {
        status: 'loaded',
        source: 'default-app-settings',
        providerCount: 2,
        matchedProviderIds: ['deepseek', 'openai-compatible'],
        usableProviderIds: ['deepseek'],
        usableNonDeepSeekProviderIds: [],
        deepseekSettingsProfileUsable: true,
	        nonDeepSeekSettingsProfileUsable: false,
	        satisfiesFinalProviderCoverageFromSettings: false,
	        credentialValuesRecorded: false
	      }
	    }))
    expect(seen[0]).toEqual(expect.objectContaining({
      url: 'https://api.deepseek.com/v1/chat/completions',
      authorization: 'Bearer sk-settingsDeepSeekSecret1234567890',
      body: expect.objectContaining({
        model: 'deepseek-v4-pro',
        stream: true
      })
    }))
    const serialized = JSON.stringify(matrix)
    expect(serialized).not.toContain('sk-settingsDeepSeekSecret1234567890')
    expect(serialized).not.toContain(settingsPath)
  })

  it('passes live provider matrix with DeepSeek plus one non-DeepSeek credentialed provider', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const fetchImpl = async (_url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example/v1',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible'
      }
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const openai = probes.find((probe) => probe.id === 'openai-compatible')
    const anthropic = probes.find((probe) => probe.id === 'anthropic-compatible')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true,
      requiredCredentialedProviderCoverage: ['deepseek', 'at-least-one-non-deepseek-provider'],
      optionalCredentialedProviderIds: expect.arrayContaining([
        'openai-compatible',
        'anthropic-compatible',
        'custom-endpoint'
      ]),
      credentialedDeepSeekPassed: true,
      credentialedNonDeepSeekProviderIds: ['openai-compatible'],
      credentialedProviderCoverage: expect.objectContaining({
        passed: true,
        deepseekPassed: true,
        nonDeepSeekProviderPassed: true,
        nonDeepSeekPassedIds: ['openai-compatible'],
        missing: []
      })
    }))
    expect(openai).toEqual(expect.objectContaining({
      status: 'passed',
      skipped: false
    }))
    expect(anthropic).toEqual(expect.objectContaining({
      status: 'skipped',
      skipped: true,
      missingEnv: expect.arrayContaining(['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY'])
    }))
  })

  it('hydrates non-DeepSeek preset provider defaults when settings profile leaves baseUrl blank', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const dir = tempDir()
    const settingsPath = join(dir, 'analytix-settings.json')
    const seen: Array<{ url: string; apiKey: string; body: Record<string, unknown> }> = []
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'deepseek',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'sk-settingsDeepSeekSecret1234567890',
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          },
          {
            id: 'minimax',
            name: 'MiniMax',
            apiKey: 'sk-settingsMiniMaxSecret1234567890',
            baseUrl: '',
            endpointFormat: 'messages',
            models: ['MiniMax-M2']
          }
        ]
      },
      runtime: {
        providerId: 'deepseek',
        model: 'deepseek-chat'
      }
    }), 'utf8')
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seen.push({
        url,
        apiKey: String((init.headers as Record<string, string>)['x-api-key'] || ''),
        body
      })
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}',
        'data: {"type":"message_delta","usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":4}}'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {},
      settingsPath,
      settingsPathSource: 'default-app-settings'
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const anthropic = probes.find((probe) => probe.id === 'anthropic-compatible')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true,
      credentialedDeepSeekPassed: true,
      credentialedNonDeepSeekProviderIds: ['anthropic-compatible'],
      settingsProfileCoverage: expect.objectContaining({
        usableProviderIds: ['anthropic-compatible', 'deepseek'],
        usableNonDeepSeekProviderIds: ['anthropic-compatible'],
        satisfiesFinalProviderCoverageFromSettings: true,
        credentialValuesRecorded: false
      })
    }))
    expect(anthropic).toEqual(expect.objectContaining({
      status: 'passed',
      missingEnv: [],
      provider: expect.objectContaining({
        providerId: 'minimax',
        model: 'MiniMax-M2',
        endpointFormat: 'messages',
        hasApiKey: true,
        source: 'settings-profile',
        settingsProfileMatched: true,
        settingsProfileUsable: true,
        settingsProfilePresetFallbackUsed: true
      })
    }))
    expect(seen.some((item) => item.url === 'https://api.minimaxi.com/anthropic/v1/messages')).toBe(true)
    const serialized = JSON.stringify(matrix)
    expect(serialized).not.toContain('sk-settingsMiniMaxSecret1234567890')
    expect(serialized).not.toContain('sk-settingsDeepSeekSecret1234567890')
    expect(serialized).not.toContain(settingsPath)
  })

  it('hydrates source-backed provider preset models when settings profile leaves baseUrl and models blank', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const dir = tempDir()
    const settingsPath = join(dir, 'analytix-settings.json')
    const seen: Array<{ url: string; authorization: string; body: Record<string, unknown> }> = []
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'moonshot-cn',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'sk-settingsDeepSeekSecret1234567890',
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          },
          {
            id: 'moonshot-cn',
            name: 'Moonshot CN',
            apiKey: 'sk-settingsMoonshotSecret1234567890',
            baseUrl: '',
            endpointFormat: 'chat_completions',
            models: []
          }
        ]
      },
      runtime: {
        providerId: 'moonshot-cn',
        model: ''
      }
    }), 'utf8')
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seen.push({
        url,
        authorization: String((init.headers as Record<string, string>).authorization || ''),
        body
      })
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      if (url.includes('api.deepseek.com')) {
        return new Response([
          'data: {"choices":[{"delta":{"content":"ok"}}]}',
          'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
          'data: [DONE]'
        ].join('\n\n'), {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":4}}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {},
      settingsPath,
      settingsPathSource: 'default-app-settings'
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const openai = probes.find((probe) => probe.id === 'openai-compatible')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true,
      credentialedDeepSeekPassed: true,
      credentialedNonDeepSeekProviderIds: ['openai-compatible'],
      settingsProfileCoverage: expect.objectContaining({
        usableProviderIds: ['deepseek', 'openai-compatible'],
        usableNonDeepSeekProviderIds: ['openai-compatible'],
        satisfiesFinalProviderCoverageFromSettings: true,
        credentialValuesRecorded: false
      })
    }))
    expect(openai).toEqual(expect.objectContaining({
      status: 'passed',
      missingEnv: [],
      provider: expect.objectContaining({
        providerId: 'moonshot-cn',
        model: 'kimi-k2.7-code',
        endpointFormat: 'chat-completions',
        hasApiKey: true,
        source: 'settings-profile',
        settingsProfileMatched: true,
        settingsProfileUsable: true,
        settingsProfilePresetFallbackUsed: true,
        settingsProfilePresetModelFallbackUsed: true
      })
    }))
    expect(seen.some((item) =>
      item.url === 'https://api.moonshot.cn/v1/chat/completions' &&
      item.body.model === 'kimi-k2.7-code'
    )).toBe(true)
    const serialized = JSON.stringify(matrix)
    expect(serialized).not.toContain('sk-settingsMoonshotSecret1234567890')
    expect(serialized).not.toContain('sk-settingsDeepSeekSecret1234567890')
    expect(serialized).not.toContain(settingsPath)
  })

  it('hydrates token-plan provider preset defaults from product presets', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const dir = tempDir()
    const settingsPath = join(dir, 'analytix-settings.json')
    const seen: Array<{ url: string; authorization: string; body: Record<string, unknown> }> = []
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'xiaomi-token-plan',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'sk-settingsDeepSeekSecret1234567890',
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          },
          {
            id: 'xiaomi-token-plan',
            name: 'Xiaomi Token Plan',
            apiKey: 'tp-settingsXiaomiSecret1234567890',
            baseUrl: '',
            endpointFormat: '',
            models: []
          }
        ]
      },
      runtime: {
        providerId: 'xiaomi-token-plan',
        model: ''
      }
    }), 'utf8')
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seen.push({
        url,
        authorization: String((init.headers as Record<string, string>).authorization || ''),
        body
      })
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      if (url.includes('api.deepseek.com')) {
        return new Response([
          'data: {"choices":[{"delta":{"content":"ok"}}]}',
          'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
          'data: [DONE]'
        ].join('\n\n'), {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return new Response([
        'data: {"choices":[{"delta":{"content":"ok"}}]}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_tokens_details":{"cached_tokens":4}}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {},
      settingsPath,
      settingsPathSource: 'default-app-settings'
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const openai = probes.find((probe) => probe.id === 'openai-compatible')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true,
      credentialedDeepSeekPassed: true,
      credentialedNonDeepSeekProviderIds: ['openai-compatible']
    }))
    expect(openai).toEqual(expect.objectContaining({
      status: 'passed',
      provider: expect.objectContaining({
        providerId: 'xiaomi-token-plan',
        model: 'mimo-v2.5-pro',
        endpointFormat: 'chat-completions',
        hasApiKey: true,
        settingsProfilePresetFallbackUsed: true,
        settingsProfilePresetModelFallbackUsed: true
      })
    }))
    expect(seen.some((item) =>
      item.url === 'https://token-plan-cn.xiaomimimo.com/v1/chat/completions' &&
      item.body.model === 'mimo-v2.5-pro'
    )).toBe(true)
    const serialized = JSON.stringify(matrix)
    expect(serialized).not.toContain('tp-settingsXiaomiSecret1234567890')
    expect(serialized).not.toContain('sk-settingsDeepSeekSecret1234567890')
    expect(serialized).not.toContain(settingsPath)
  })

  it('honors source-backed per-model endpointFormat overrides for product presets', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const dir = tempDir()
    const settingsPath = join(dir, 'analytix-settings.json')
    const seen: Array<{ url: string; apiKey: string; body: Record<string, unknown> }> = []
    writeFileSync(settingsPath, JSON.stringify({
      provider: {
        activeProviderId: 'opencode-go',
        apiKey: '',
        baseUrl: '',
        providers: [
          {
            id: 'deepseek',
            name: 'DeepSeek',
            apiKey: 'sk-settingsDeepSeekSecret1234567890',
            baseUrl: 'https://api.deepseek.com',
            endpointFormat: 'chat_completions',
            models: ['deepseek-chat']
          },
          {
            id: 'opencode-go',
            name: 'OpenCode Go',
            apiKey: 'sk-settingsOpenCodeSecret1234567890',
            baseUrl: '',
            endpointFormat: '',
            models: []
          }
        ]
      },
      runtime: {
        providerId: 'opencode-go',
        model: 'minimax-m3'
      }
    }), 'utf8')
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      seen.push({
        url,
        apiKey: String((init.headers as Record<string, string>)['x-api-key'] || ''),
        body
      })
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      if (url.includes('api.deepseek.com')) {
        return new Response([
          'data: {"choices":[{"delta":{"content":"ok"}}]}',
          'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7,"prompt_cache_miss_tokens":3}}',
          'data: [DONE]'
        ].join('\n\n'), {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      return new Response([
        'data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}',
        'data: {"type":"message_delta","usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":4}}'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {},
      settingsPath,
      settingsPathSource: 'default-app-settings'
    })
    const probes = matrix.credentialedProbes as Array<Record<string, unknown>>
    const anthropic = probes.find((probe) => probe.id === 'anthropic-compatible')

    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true,
      credentialedDeepSeekPassed: true,
      credentialedNonDeepSeekProviderIds: ['anthropic-compatible'],
      settingsProfileCoverage: expect.objectContaining({
        usableProviderIds: ['anthropic-compatible', 'deepseek'],
        usableNonDeepSeekProviderIds: ['anthropic-compatible'],
        satisfiesFinalProviderCoverageFromSettings: true,
        credentialValuesRecorded: false
      })
    }))
    expect(anthropic).toEqual(expect.objectContaining({
      status: 'passed',
      missingEnv: [],
      provider: expect.objectContaining({
        providerId: 'opencode-go',
        model: 'minimax-m3',
        endpointFormat: 'messages',
        hasApiKey: true,
        source: 'settings-profile',
        settingsProfileMatched: true,
        settingsProfileUsable: true,
        settingsProfilePresetFallbackUsed: true,
        settingsProfilePresetModelFallbackUsed: true
      })
    }))
    expect(seen.some((item) =>
      item.url === 'https://opencode.ai/zen/go/v1/messages' &&
      item.apiKey === 'sk-settingsOpenCodeSecret1234567890' &&
      item.body.model === 'minimax-m3'
    )).toBe(true)
    const serialized = JSON.stringify(matrix)
    expect(serialized).not.toContain('sk-settingsOpenCodeSecret1234567890')
    expect(serialized).not.toContain('sk-settingsDeepSeekSecret1234567890')
    expect(serialized).not.toContain(settingsPath)
  })

  it('does not accept non-SSE JSON as provider stream parsing evidence', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const fetchImpl = async (_url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response(JSON.stringify({
        choices: [{ message: { content: 'ok' } }],
        usage: { prompt_tokens: 10, completion_tokens: 1, prompt_cache_hit_tokens: 7 }
      }), {
        status: 200,
        headers: { 'content-type': 'application/json' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example/v1',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL: 'https://anthropic.example/v1',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL: 'claude-compatible',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL: 'https://custom.example/full/path',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'custom-compatible'
      }
    })

    expect(matrix).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false
    }))
    for (const probe of matrix.credentialedProbes as Array<Record<string, unknown>>) {
      expect(probe.streamParsing).toEqual(expect.objectContaining({
        status: 'failed',
        mode: 'json'
      }))
    }
  })

  it('uses responses-shaped body for a custom full responses endpoint', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const seenBodies = new Map<string, Record<string, unknown>>()
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      if (body.model) seenBodies.set(url, body)
      const errorProbe = providerErrorProbeResponse(body)
      if (errorProbe) return errorProbe
      return new Response([
        'data: {"output_text":"ok"}',
        'data: {"usage":{"prompt_tokens":10,"completion_tokens":1,"prompt_cache_hit_tokens":7}}',
        'data: [DONE]'
      ].join('\n\n'), {
        status: 200,
        headers: { 'content-type': 'text/event-stream' }
      })
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example/v1',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL: 'https://anthropic.example/v1',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL: 'claude-compatible',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL: 'https://custom.example/v1/responses',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'custom-compatible'
      }
    })

    const customBody = seenBodies.get('https://custom.example/v1/responses')
    expect(customBody).toEqual(expect.objectContaining({
      model: 'custom-compatible',
      input: 'Analytix runtime protocol probe.',
      stream: true,
      max_output_tokens: 16
    }))
    expect(customBody).not.toHaveProperty('messages')
    const custom = (matrix.credentialedProbes as Array<Record<string, unknown>>).find((probe) => probe.id === 'custom-endpoint')
    expect(custom?.requestShape).toEqual(expect.objectContaining({
      customEndpointInferredFormat: 'responses',
      inputConfigured: true,
      maxTokensField: 'max_output_tokens'
    }))
    expect(custom?.requestShapeValidation).toEqual(expect.objectContaining({ status: 'passed' }))
    expect(matrix).toEqual(expect.objectContaining({
      status: 'passed',
      passed: true
    }))
  })

  it('redacts header-style provider secrets from failed probe evidence', async () => {
    const { collectProviderMatrix } = await loadCollector()
    const fetchImpl = async (): Promise<Response> => {
      throw new Error('upstream rejected request x-api-key: sk-liveSecretValue1234567890')
    }

    const matrix = await collectProviderMatrix({
      now: '2026-06-24T00:00:00.000Z',
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_DEEPSEEK_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL: 'https://deepseek.example/v1',
        ANALYTIX_RUNTIME_DEEPSEEK_MODEL: 'deepseek-chat',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL: 'https://openai.example/v1',
        ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL: 'gpt-compatible',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL: 'https://anthropic.example/v1',
        ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL: 'claude-compatible',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY: 'test-key',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL: 'https://custom.example/full/path',
        ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL: 'custom-compatible'
      }
    })

    const serialized = JSON.stringify(matrix)
    expect(matrix).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      redaction: expect.objectContaining({ status: 'passed', secretMaterialFound: false })
    }))
    expect(serialized).toContain('x-api-key: [REDACTED]')
    expect(serialized).not.toContain('sk-liveSecretValue1234567890')
  })

  it('replaces component evidence with safe stubs when a collected report contains secret-like material', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-secret.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)
    writeFileSync(packagedScript, `
const checks = ${JSON.stringify([
  'packaged-app-startup',
  'health',
  'runtime-info',
  'thread-list',
  'turn-create',
  'sse-replay',
  'go-runtime-default-gate',
  'typescript-retired-backend'
])}.map((id) => ({ id, status: 'passed' }))
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-qa',
  generatedAt: '2026-06-24T00:00:00.000Z',
  status: 'passed',
  passed: true,
  checks,
  authorization: 'Bearer sk-packagedSecretValue1234567890'
}))
`, 'utf8')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    expect(report).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      credentialSecretsRecorded: false
    }))

    const paths = report.outputPaths as Record<string, string>
    const serializedArtifacts = [
      readFileSync(paths.provider, 'utf8'),
      readFileSync(paths.mcp, 'utf8'),
      readFileSync(paths.packaged, 'utf8'),
      readFileSync(paths.packagedGui, 'utf8'),
      readFileSync(paths.packagedSoak, 'utf8'),
      readFileSync(paths.operator, 'utf8'),
      readFileSync(paths.json, 'utf8'),
      readFileSync(paths.markdown, 'utf8')
    ].join('\n')
    const provider = JSON.parse(readFileSync(paths.provider, 'utf8')) as Record<string, unknown>
    const packaged = JSON.parse(readFileSync(paths.packaged, 'utf8')) as Record<string, unknown>
    const operator = JSON.parse(readFileSync(paths.operator, 'utf8')) as Record<string, unknown>

    expect(provider).toEqual(expect.objectContaining({
      id: 'runtime-go-provider-matrix',
      status: 'failed',
      redaction: expect.objectContaining({
        status: 'failed',
        secretMaterialFound: true
      })
    }))
    expect(provider).not.toHaveProperty('legacyId')
    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'failed',
      passed: false
    }))
    expect(packaged).not.toHaveProperty('legacyId')
    expect(operator).toEqual(expect.objectContaining({
      id: 'runtime-go-operator-gate',
      status: 'failed',
      passed: false
    }))
    expect(operator).not.toHaveProperty('legacyId')
    expect(serializedArtifacts).not.toContain('sk-packagedSecretValue1234567890')
    expect(serializedArtifacts).not.toContain('Bearer sk-packagedSecretValue1234567890')
  })

  it('blocks packaged QA evidence that passes checks but violates fallback/backend or subagent evidence', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-unsafe-backend.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)
    writeFileSync(packagedScript, `
const checks = ${JSON.stringify([
  'packaged-app-startup',
  'health',
  'runtime-info',
  'thread-list',
  'turn-create',
  'sse-replay',
  'go-runtime-default-gate',
  'typescript-retired-backend'
])}.map((id) => ({ id, status: 'passed' }))
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-qa',
  stage: 'packaged-desktop-qa',
  generatedAt: '2026-06-24T00:00:00.000Z',
  status: 'passed',
  passed: true,
  checks,
  goDefaultBackendEnabled: true,
  rendererVisibleGoSwitcher: false,
  typeScriptFallbackRetained: false,
  credentialSecretsRecorded: false,
  redaction: { status: 'passed', secretMaterialFound: false },
  startup: {
    appPathExists: true,
    launchCommandConfigured: true,
    launchCommandReferencesAppPath: true,
    launchCommandHash: '${'0'.repeat(64)}',
    exitCode: 0
  },
  runtime: {
    runtimeUrl: 'http://127.0.0.1:12345',
    runtimeTokenConfigured: true,
    healthOK: true,
    runtimeInfo: {
      candidateContract: true,
      hostLocalhost: true,
      dataDirConfigured: true,
      dataDirHash: '${'3'.repeat(64)}',
      mcpAvailable: true,
      mcpDiagnosticHonest: true,
      subagentsAvailable: false,
      subagentsDiagnosticHonest: true,
      productRuntimeInfoHidesUpstreamEvidence: false,
      reasonixUpstreamAbsorptionPresent: true,
      rawValueRecorded: false,
      infoShapeDigest: '${'4'.repeat(64)}'
    },
    threadIdHash: '${'1'.repeat(64)}',
    turnIdHash: '${'2'.repeat(64)}'
  },
  defaultRuntimeGate: {
    runtimeBackend: 'go-runtime-default',
    explicitRuntimeBackendOverrideEnvSet: false,
    goDefaultBackendEnabled: true
  },
  retiredBackendEvidence: {
    requestedBackend: 'typescript',
    code: 'retired_backend',
    activeBackendAfterRequest: 'go-runtime-default',
    defaultRuntimeStopped: false,
    tsStarted: false,
    runtimeHealthAfterRequest: true,
    defaultRuntimeLaunchCommandHash: '${'0'.repeat(64)}',
    preRequestRuntimeUrl: 'http://127.0.0.1:12345',
    postRequestRuntimeUrl: 'http://127.0.0.1:12345',
    verifiedAt: '2026-06-24T00:00:00.000Z',
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false }
  }
}))
`, 'utf8')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const paths = report.outputPaths as Record<string, string>
    const packaged = JSON.parse(readFileSync(paths.packaged, 'utf8')) as Record<string, unknown>

    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'live_blocked',
      passed: false
    }))
    expect(packaged).not.toHaveProperty('legacyId')
    expect(packaged.missingRequiredEvidence).toEqual(expect.arrayContaining([
      'runtime.runtimeInfo.subagentsAvailable',
      'runtime.runtimeInfo.productRuntimeInfoHidesUpstreamEvidence',
      'runtime.runtimeInfo.reasonixUpstreamAbsorptionPresent:false'
    ]))
  })

  it('accepts formal aggregate packaged QA evidence from the packaged QA runner', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-aggregate.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeAggregatePackagedQAScript(packagedScript)
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const packaged = JSON.parse(readFileSync((report.outputPaths as Record<string, string>).packaged, 'utf8')) as Record<string, unknown>
    expect(report.componentStatus).toEqual(expect.objectContaining({
      packaged: 'passed'
    }))
    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'passed',
      passed: true,
      credentialSecretsRecorded: false,
      redaction: expect.objectContaining({ status: 'passed', secretMaterialFound: false })
    }))
    expect(packaged).not.toHaveProperty('missingRequiredEvidence')
  })

  it('runs fresh packaged QA and degrades matching-commit historical PASS evidence to unverified', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-aggregate-missing.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    const evidenceDir = join(dir, 'runtime-go-live-evidence')
    const existingPackagedPath = join(evidenceDir, 'packaged-go-runtime-qa.json')
    const invokedMarkerPath = join(dir, 'packaged-script-invoked')
    const currentCommit = currentTestCommit()
    mkdirSync(evidenceDir, { recursive: true })
    writeFileSync(existingPackagedPath, JSON.stringify(aggregatePackagedQAReport({
      sourceCommit: currentCommit,
      testHarnessCommit: currentCommit
    })), 'utf8')
    const weakerReport = aggregatePackagedQAReport({
      packaged: {
        missingFinalCoverage: ['credentialed-provider', 'packaged-desktop-qa'],
        actualPackagedSmokeRequested: false,
        actualPackagedSmokePassed: false,
        actualPackagedDesktopQAPassed: false,
        actualPackagedAppLaunched: false,
        settingsProfilePatchAccepted: false,
        actualPackagedAppEvidenceRequiredForFinalGate: true
      }
    })
    writeFileSync(packagedScript, `
import { writeFileSync } from 'node:fs'
const report = ${JSON.stringify(weakerReport)}
writeFileSync(${JSON.stringify(invokedMarkerPath)}, 'invoked', 'utf8')
writeFileSync(${JSON.stringify(existingPackagedPath)}, JSON.stringify(report), 'utf8')
console.log(JSON.stringify(report))
`, 'utf8')
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: currentCommit,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const packaged = JSON.parse(readFileSync((report.outputPaths as Record<string, string>).packaged, 'utf8')) as Record<string, unknown>
    expect(report.componentStatus).toEqual(expect.objectContaining({
      packaged: 'live_blocked'
    }))
    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'live_blocked',
      passed: false,
      historicalEvidence: {
        status: 'unverified',
        passed: false,
        reused: false,
        freshnessVerified: false,
        blocker: 'historical_packaged_evidence_requires_fresh_execution',
        previousStatus: 'passed',
        previousPassed: true,
        generatedAt: '2026-06-24T00:00:00.000Z',
        sourceCommit: currentCommit,
        testHarnessCommit: currentCommit,
        sourceCommitMatchesCurrent: true,
        testHarnessCommitMatchesCurrent: true
      }
    }))
    expect(packaged.missingRequiredEvidence).toEqual(expect.arrayContaining([
      'packaged.actualPackagedDesktopQAPassed',
      'packaged.actualPackagedAppLaunched',
      'packaged.actualPackagedSmokeRequested',
      'packaged.actualPackagedSmokePassed',
      'packaged.actualPackagedAppEvidenceRequiredForFinalGate:false',
      'packaged.settingsProfilePatchAccepted',
      'packaged.missingFinalCoverage:packaged-desktop-qa'
    ]))
    expect(existsSync(invokedMarkerPath)).toBe(true)
    expect(JSON.stringify(report.missingExternalInputs)).toContain('packaged:evidence:packaged.actualPackagedDesktopQAPassed')
  })

  it('runs fresh packaged soak and degrades matching-commit historical PASS evidence to unverified', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-aggregate.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    const evidenceDir = join(dir, 'runtime-go-live-evidence')
    const existingPackagedSoakPath = join(evidenceDir, 'packaged-session-soak.json')
    const invokedMarkerPath = join(dir, 'packaged-soak-script-invoked')
    const currentCommit = currentTestCommit()
    mkdirSync(evidenceDir, { recursive: true })
    writeFileSync(existingPackagedSoakPath, JSON.stringify(actualPackagedSessionSoakReport({
      sourceCommit: currentCommit,
      testHarnessCommit: currentCommit
    })), 'utf8')
    const weakerReport = {
      schemaVersion: 1,
      id: 'runtime-go-packaged-session-soak',
      stage: 'packaged-session-soak',
      status: 'passed',
      passed: true,
      soak: {
        deterministicContractOnly: true,
        actualPackagedAppLaunched: false,
        actualPackagedAppEvidenceRequiredForFinalGate: true,
        goDurableSessionCovered: true,
        typeScriptRetiredBackendSessionCovered: true,
        resumeForkPairingCovered: true,
        sseReplayCovered: true
      },
      checks: [
        { id: 'go-session-durable-contract', status: 'passed' },
        { id: 'typescript-retired-backend-session-contract', status: 'passed' }
      ]
    }
    writeAggregatePackagedQAScript(packagedScript)
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeFileSync(packagedSoakScript, `
import { writeFileSync } from 'node:fs'
const report = ${JSON.stringify(weakerReport)}
writeFileSync(${JSON.stringify(invokedMarkerPath)}, 'invoked', 'utf8')
writeFileSync(${JSON.stringify(existingPackagedSoakPath)}, JSON.stringify(report), 'utf8')
console.log(JSON.stringify(report))
`, 'utf8')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: currentCommit,
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const packagedSoak = JSON.parse(readFileSync((report.outputPaths as Record<string, string>).packagedSoak, 'utf8')) as Record<string, unknown>
    expect(packagedSoak).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-session-soak',
      status: 'passed',
      passed: true,
      historicalEvidence: {
        status: 'unverified',
        passed: false,
        reused: false,
        freshnessVerified: false,
        blocker: 'historical_packaged_evidence_requires_fresh_execution',
        previousStatus: 'passed',
        previousPassed: true,
        generatedAt: '2026-06-24T00:00:00.000Z',
        sourceCommit: currentCommit,
        testHarnessCommit: currentCommit,
        sourceCommitMatchesCurrent: true,
        testHarnessCommitMatchesCurrent: true
      }
    }))
    expect(packagedSoak.soak).toEqual(expect.objectContaining({
      deterministicContractOnly: true,
      actualPackagedAppEvidenceRequiredForFinalGate: true
    }))
    expect(packagedSoak).not.toHaveProperty('missingRequiredEvidence')
    expect(existsSync(invokedMarkerPath)).toBe(true)
    expect(JSON.stringify(report.missingExternalInputs)).toContain('packaged-soak:actual-packaged-session-soak')
  })

  it('rejects aggregate packaged QA evidence when packaged desktop coverage is still missing', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-aggregate-missing.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeAggregatePackagedQAScript(packagedScript, {
      packaged: {
        missingFinalCoverage: ['credentialed-provider', 'packaged-desktop-qa'],
        actualPackagedDesktopQAPassed: false
      }
    })
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const packaged = JSON.parse(readFileSync((report.outputPaths as Record<string, string>).packaged, 'utf8')) as Record<string, unknown>
    expect(packaged).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-qa',
      status: 'live_blocked',
      passed: false
    }))
    expect(packaged.missingRequiredEvidence).toEqual(expect.arrayContaining([
      'packaged.actualPackagedDesktopQAPassed',
      'packaged.missingFinalCoverage:packaged-desktop-qa'
    ]))
  })

  it('blocks packaged QA stdout that claims passed when the subprocess exits non-zero', async () => {
    const { buildRuntimeGoLiveEvidenceReport } = await loadCollector()
    const dir = tempDir()
    const packagedScript = join(dir, 'mock-packaged-nonzero.mjs')
    const packagedGuiScript = join(dir, 'mock-packaged-gui.mjs')
    const packagedSoakScript = join(dir, 'mock-packaged-soak.mjs')
    writeBlockedPackagedGuiScript(packagedGuiScript)
    writeBlockedPackagedSoakScript(packagedSoakScript)
    writeFileSync(packagedScript, `
const checks = ${JSON.stringify([
  'packaged-app-startup',
  'health',
  'runtime-info',
  'thread-list',
  'turn-create',
  'sse-replay',
  'go-runtime-default-gate',
  'typescript-retired-backend'
])}.map((id) => ({ id, status: 'passed' }))
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-qa',
  stage: 'packaged-desktop-qa',
  generatedAt: '2026-06-24T00:00:00.000Z',
  status: 'passed',
  passed: true,
  checks,
  goDefaultBackendEnabled: true,
  rendererVisibleGoSwitcher: false,
  typeScriptFallbackRetained: false,
  credentialSecretsRecorded: false,
  startup: { appPathExists: true, launchCommandConfigured: true, launchCommandReferencesAppPath: true, launchCommandHash: '${'0'.repeat(64)}', exitCode: 0 },
  runtime: {
    runtimeUrl: 'http://127.0.0.1:12345',
    runtimeTokenConfigured: true,
    healthOK: true,
    runtimeInfo: {
      candidateContract: true,
      hostLocalhost: true,
      dataDirConfigured: true,
      dataDirHash: '${'3'.repeat(64)}',
      mcpAvailable: true,
      subagentsAvailable: true,
      rawValueRecorded: false,
      infoShapeDigest: '${'4'.repeat(64)}'
    },
    threadIdHash: '${'1'.repeat(64)}',
    turnIdHash: '${'2'.repeat(64)}'
  },
  defaultRuntimeGate: { runtimeBackend: 'go-runtime-default', explicitRuntimeBackendOverrideEnvSet: false, goDefaultBackendEnabled: true },
  retiredBackendEvidence: {
    requestedBackend: 'typescript',
    code: 'retired_backend',
    activeBackendAfterRequest: 'go-runtime-default',
    defaultRuntimeStopped: false,
    tsStarted: false,
    runtimeHealthAfterRequest: true,
    defaultRuntimeLaunchCommandHash: '${'0'.repeat(64)}',
    preRequestRuntimeUrl: 'http://127.0.0.1:12345',
    postRequestRuntimeUrl: 'http://127.0.0.1:12345',
    verifiedAt: '2026-06-24T00:00:00.000Z',
    credentialSecretsRecorded: false,
    redaction: { status: 'passed', secretMaterialFound: false }
  }
}))
process.exit(42)
`, 'utf8')

    const report = await buildRuntimeGoLiveEvidenceReport({
      env: {},
      reportDir: dir,
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'test-commit',
      packagedScript,
      packagedGuiScript,
      packagedSoakScript
    })

    const packaged = JSON.parse(readFileSync((report.outputPaths as Record<string, string>).packaged, 'utf8')) as Record<string, unknown>
    expect(packaged).toEqual(expect.objectContaining({
      status: 'failed',
      passed: false
    }))
  })

  it('collects packaged GUI bridge smoke evidence and requires it for operator review when present', async () => {
    const { collectOperatorGate, collectPackagedGuiSmoke } = await loadCollector()
    const dir = tempDir()
    const packagedGuiScript = join(dir, 'mock-packaged-gui-smoke.mjs')
    const requiredCheckIds = [
      'packaged-app-artifact',
      'packaged-app-launch',
      'packaged-renderer-bridge',
      'packaged-settings-bridge',
      'packaged-provider-profile-settings',
      'packaged-mimo-profile-settings',
      'packaged-runtime-restart',
      'packaged-runtime-health',
      'packaged-runtime-info'
    ]
    writeFileSync(packagedGuiScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-gui-smoke',
  generatedAt: '2026-06-24T00:00:00.000Z',
  status: 'passed',
  passed: true,
  smoke: {
    deterministicContractOnly: false,
    actualPackagedAppLaunched: true,
    actualPackagedAppEvidenceRequiredForFinalGate: false,
    actualPackagedSmokeRequested: true,
    actualPackagedSmokePassed: true,
    settingsProfilePatchAccepted: true,
    mimoProfileSettingsAccepted: true,
    settingsCompatPatchUsed: false
  },
  actualPackagedSmoke: {
    status: 'passed',
    passed: true,
    checks: ${JSON.stringify(requiredCheckIds)}.map((id) => ({ id, status: 'passed' })),
    app: {
      executableExists: true,
      runtimeServerBinaryExists: true,
      appAsarExists: true,
      bundledRuntimeGoSourcePresent: false
    },
    renderer: {
      apiPresent: true,
      title: 'Analytix',
      legacyKun: false,
      legacyReasonix: false,
      settingsOk: true,
      settingsRuntimeTopLevel: true,
      settingsProfilePatchAccepted: true,
      mimoProfileSettingsOk: true,
      settingsCompatPatchUsed: false,
      restartOk: true,
      healthStatus: 200,
      healthService: 'analytix',
      runtimeInfoStatus: 200,
      runtimeInfoProductClean: true,
      runtimeInfoHasLegacyMcpLocalMarker: false,
      runtimeInfoHasLegacyAbsorptionMarker: false,
      runtimeInfoHasReasonixSurface: false
    },
    redaction: {
      status: 'passed',
      secretMaterialFound: false
    }
  }
}))
`, 'utf8')

    const packagedGui = collectPackagedGuiSmoke({
      env: {},
      now: '2026-06-24T00:00:00.000Z',
      packagedGuiScript
    })

    expect(packagedGui).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-gui-smoke',
      status: 'passed',
      passed: true
    }))

    const evidence = { status: 'passed', passed: true }
    const operator = collectOperatorGate({
      env: {
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
      },
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'not-a-real-commit',
      provider: { ...evidence, id: 'runtime-go-provider-matrix' },
      mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
      packaged: { ...evidence, id: 'runtime-go-packaged-qa' },
      packagedGui,
      paths: {
        provider: 'provider.json',
        mcp: 'mcp.json',
        packaged: 'packaged.json',
        packagedGui: 'packaged-gui.json'
      }
    })

    expect(operator.evidenceReviewed).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'packaged-gui-bridge-smoke', status: 'passed' })
    ]))
    expect(operator.evidenceDigests).toEqual(expect.objectContaining({
      packagedGui: sha256(packagedGui)
    }))
  })

  it('collects packaged session soak evidence and requires it for operator review when present', async () => {
    const { collectOperatorGate, collectPackagedSessionSoak } = await loadCollector()
    const dir = tempDir()
    const packagedSoakScript = join(dir, 'mock-packaged-session-soak.mjs')
    writeFileSync(packagedSoakScript, `
console.log(JSON.stringify({
  schemaVersion: 1,
  id: 'runtime-go-packaged-session-soak',
  generatedAt: '2026-06-24T00:00:00.000Z',
  status: 'passed',
  passed: true,
  soak: {
    deterministicContractOnly: true,
    actualPackagedAppLaunched: false,
    actualPackagedAppEvidenceRequiredForFinalGate: true,
    goDurableSessionCovered: true,
    typeScriptRetiredBackendSessionCovered: true,
    resumeForkPairingCovered: true,
    sseReplayCovered: true
  },
  checks: [
    { id: 'go-session-durable-contract', status: 'passed' },
    { id: 'typescript-retired-backend-session-contract', status: 'passed' }
  ]
}))
`, 'utf8')

    const packagedSoak = collectPackagedSessionSoak({
      env: {},
      now: '2026-06-24T00:00:00.000Z',
      packagedSoakScript
    })

    expect(packagedSoak).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-session-soak',
      status: 'passed',
      passed: true
    }))

    const evidence = { status: 'passed', passed: true }
    const operator = collectOperatorGate({
      env: {
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
      },
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'not-a-real-commit',
      provider: { ...evidence, id: 'runtime-go-provider-matrix' },
      mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
      packaged: { ...evidence, id: 'runtime-go-packaged-qa' },
      packagedGui: { ...evidence, id: 'runtime-go-packaged-gui-smoke' },
      packagedSoak,
      paths: {
        provider: 'provider.json',
        mcp: 'mcp.json',
        packaged: 'packaged.json',
        packagedGui: 'packaged-gui.json',
        packagedSoak: 'packaged-soak.json'
      }
    })

    expect(operator.evidenceReviewed).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'packaged-session-soak', status: 'passed' })
    ]))
    expect(operator.evidenceDigests).toEqual(expect.objectContaining({
      packagedSoak: sha256(packagedSoak)
    }))
  })

  it('accepts actual packaged session soak evidence for final gate review', async () => {
    const { collectOperatorGate, collectPackagedSessionSoak } = await loadCollector()
    const dir = tempDir()
    const packagedSoakScript = join(dir, 'mock-actual-packaged-session-soak.mjs')
    writeActualPackagedSoakScript(packagedSoakScript)

    const packagedSoak = collectPackagedSessionSoak({
      env: {},
      now: '2026-06-24T00:00:00.000Z',
      packagedSoakScript
    })

    expect(packagedSoak).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-session-soak',
      status: 'passed',
      passed: true
    }))
    expect(packagedSoak).not.toHaveProperty('missingRequiredEvidence')

    const evidence = { status: 'passed', passed: true }
    const operator = collectOperatorGate({
      env: {
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
      },
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'not-a-real-commit',
      provider: { ...evidence, id: 'runtime-go-provider-matrix' },
      mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
      packaged: aggregatePackagedQAReport(),
      packagedGui: { ...evidence, id: 'runtime-go-packaged-gui-smoke' },
      packagedSoak,
      paths: {
        provider: 'provider.json',
        mcp: 'mcp.json',
        packaged: 'packaged.json',
        packagedGui: 'packaged-gui.json',
        packagedSoak: 'packaged-soak.json'
      }
    })

    expect(operator.credentialedEvidenceReviewed).toBe(true)
    expect(operator.dependencySummary).toEqual(expect.objectContaining({
      envGate: {
        runtimeReady: true,
        operatorApprovesDefault: true
      },
      evidence: expect.objectContaining({
        providerPassed: true,
        mcpPassed: true,
        packagedPassed: true,
        packagedGuiRequired: true,
        packagedGuiPassed: true,
        packagedSoakRequired: true,
        packagedSoakPassed: true,
        packagedSoakActualFinalEvidencePresent: true,
        credentialedEvidenceReviewed: true
      })
    }))
    expect(operator.failureReasons).not.toContain('actual packaged session soak evidence is required for final gate')
  })

  it('allows operator approval while only collector-owned live evidence outputs are dirty', async () => {
    const { collectOperatorGate } = await loadCollector()
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'cat-file' && args[1] === '-e') process.exit(0)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json',
    ' M docs/analytix/upstreams/runtime-go-live-evidence/operator-gate.json',
    ' M docs/analytix/upstreams/runtime-go-live-evidence/provider-matrix.json'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const evidence = { status: 'passed', passed: true }
      const operator = collectOperatorGate({
        env: {
          ANALYTIX_RUNTIME_READY: '1',
          ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
        },
        now: '2026-06-24T00:00:00.000Z',
        commitHash: 'a'.repeat(40),
        provider: { ...evidence, id: 'runtime-go-provider-matrix' },
        mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
        packaged: { ...evidence, id: 'runtime-go-packaged-qa' },
        paths: {
          provider: 'docs/analytix/upstreams/runtime-go-live-evidence/provider-matrix.json',
          mcp: 'docs/analytix/upstreams/runtime-go-live-evidence/mcp-execution.json',
          packaged: 'docs/analytix/upstreams/runtime-go-live-evidence/packaged-go-runtime-qa.json',
          operator: 'docs/analytix/upstreams/runtime-go-live-evidence/operator-gate.json',
          report: 'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json'
        }
      })

      expect(operator).toEqual(expect.objectContaining({
        status: 'passed',
        passed: true,
        goDefaultApproved: true
      }))
      expect(operator.dependencySummary).toEqual(expect.objectContaining({
        commitBinding: expect.objectContaining({
          required: true,
          formatValid: true,
          exists: true,
          relevantEvidenceClean: true,
          bound: true
        }),
        blockers: []
      }))
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('keeps blocking operator approval when non-collector relevant evidence is dirty', async () => {
    const { collectOperatorGate } = await loadCollector()
    const dir = tempDir()
    const fakeGit = join(dir, process.platform === 'win32' ? 'git.cmd' : 'git')
    const originalPath = process.env.PATH
    writeExecutable(fakeGit, `#!/usr/bin/env node
const args = process.argv.slice(2)
if (args[0] === 'cat-file' && args[1] === '-e') process.exit(0)
if (args[0] === 'status' && args[1] === '--porcelain=v1') {
  console.log([
    ' M docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json',
    ' M scripts/runtime-go-preflight.mjs'
  ].join('\\n'))
  process.exit(0)
}
console.error('unexpected git command: ' + args.join(' '))
process.exit(1)
`)

    try {
      process.env.PATH = `${dir}${delimiter}${originalPath || ''}`
      const evidence = { status: 'passed', passed: true }
      const operator = collectOperatorGate({
        env: {
          ANALYTIX_RUNTIME_READY: '1',
          ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
        },
        now: '2026-06-24T00:00:00.000Z',
        commitHash: 'b'.repeat(40),
        provider: { ...evidence, id: 'runtime-go-provider-matrix' },
        mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
        packaged: { ...evidence, id: 'runtime-go-packaged-qa' },
        paths: {
          provider: 'docs/analytix/upstreams/runtime-go-live-evidence/provider-matrix.json',
          mcp: 'docs/analytix/upstreams/runtime-go-live-evidence/mcp-execution.json',
          packaged: 'docs/analytix/upstreams/runtime-go-live-evidence/packaged-go-runtime-qa.json',
          report: 'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json'
        }
      })

      expect(operator).toEqual(expect.objectContaining({
        status: 'live_blocked',
        passed: false,
        goDefaultApproved: false
      }))
      expect(operator.dependencySummary).toEqual(expect.objectContaining({
        commitBinding: expect.objectContaining({
          required: true,
          formatValid: true,
          exists: true,
          relevantEvidenceClean: false,
          bound: false
        }),
        blockers: expect.arrayContaining([
          'operator evidence target has uncommitted runtime evidence changes'
        ])
      }))
    } finally {
      process.env.PATH = originalPath
    }
  })

  it('blocks operator approval when the target commit is not present in the repository', async () => {
    const { collectOperatorGate } = await loadCollector()
    const evidence = { status: 'passed', passed: true }

    const operator = collectOperatorGate({
      env: {
        ANALYTIX_RUNTIME_READY: '1',
        ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT: '1'
      },
      now: '2026-06-24T00:00:00.000Z',
      commitHash: 'f'.repeat(40),
      provider: { ...evidence, id: 'runtime-go-provider-matrix' },
      mcp: { ...evidence, id: 'runtime-go-mcp-execution' },
      packaged: { ...evidence, id: 'runtime-go-packaged-qa' },
      paths: {
        provider: 'provider.json',
        mcp: 'mcp.json',
        packaged: 'packaged.json'
      }
    })

    expect(operator).toEqual(expect.objectContaining({
      status: 'live_blocked',
      passed: false,
      explicitEnvGate: true,
      credentialedEvidenceReviewed: true,
      goDefaultApproved: false
    }))
    expect(operator.dependencySummary).toEqual(expect.objectContaining({
      envGate: {
        runtimeReady: true,
        operatorApprovesDefault: true
      },
      evidence: expect.objectContaining({
        providerPassed: true,
        mcpPassed: true,
        packagedPassed: true,
        credentialedEvidenceReviewed: true
      }),
      commitBinding: expect.objectContaining({
        required: true,
        formatValid: true,
        exists: false,
        relevantEvidenceClean: false,
        bound: false
      }),
      blockers: expect.arrayContaining([
        'operator evidence target commit does not exist in this Git repository'
      ])
    }))
    expect(operator.failureReasons).toContain('operator evidence target commit does not exist in this Git repository')
  })

  it('passes injected environment variables to the MCP stdio subprocess', async () => {
    const { collectMCPExecution } = await loadCollector()
    const dir = tempDir()
    const approvalPath = join(dir, 'approval.json')
    const mcpServerPath = join(dir, 'mock-mcp-env.mjs')
    writeApprovalEvidence(approvalPath)
    writeFileSync(mcpServerPath, `
import readline from 'node:readline'
const rl = readline.createInterface({ input: process.stdin })
const envVisible = process.env.ANALYTIX_RUNTIME_MCP_TEST_ENV === 'visible-to-child'
rl.on('line', (line) => {
  const msg = JSON.parse(line)
  if (msg.method === 'notifications/initialized') return
  if (msg.method === 'initialize') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { protocolVersion: '2025-11-25', capabilities: {}, serverInfo: { name: 'mock', version: '1.0.0' } } }))
    return
  }
  if (msg.method === 'tools/list') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { tools: envVisible ? [{ name: 'env_tool', inputSchema: { type: 'object' } }] : [] } }))
    return
  }
  if (msg.method === 'tools/call') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { content: [{ type: 'text', text: envVisible ? 'ok' : 'missing-env' }] } }))
  }
})
`, 'utf8')

    const report = await collectMCPExecution({
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      env: {
        ANALYTIX_RUNTIME_MCP_COMMAND: `${JSON.stringify(process.execPath)} ${JSON.stringify(mcpServerPath)}`,
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'env_tool',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS: 'passed',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: approvalPath,
        ANALYTIX_RUNTIME_MCP_TEST_ENV: 'visible-to-child'
      }
    })

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'passed',
      passed: true,
      credentialedExecution: true,
      connect: true,
      toolDiscoverySearch: true,
      toolCall: true,
      approvalUserInput: true,
      reconnect: true
    }))
    expect(report).not.toHaveProperty('legacyId')
    const probe = (report.credentialedProbes as Array<Record<string, unknown>>)[0]
    expect(probe.approvalUserInputEvidence).toEqual(expect.objectContaining({
      status: 'passed',
      evidencePath: approvalPath,
      evidenceSha256: sha256Text(readFileSync(approvalPath, 'utf8')),
      evidenceDigestAlgorithm: 'sha256:file-bytes-v1',
      rawValueRecorded: false,
      credentialSecretsRecorded: false
    }))
    expect(probe).toEqual(expect.objectContaining({
      calledToolNameHash: sha256Text('env_tool'),
      toolCatalogDigest: expect.stringMatching(/^[a-f0-9]{64}$/),
      reconnectToolNameHash: sha256Text('env_tool'),
      reconnectToolCatalogDigest: expect.stringMatching(/^[a-f0-9]{64}$/),
      reconnectToolCount: 1
    }))
  })

  it('probes HTTP MCP URL without recording URL credentials or bearer token values', async () => {
    const { collectMCPExecution } = await loadCollector()
    const dir = tempDir()
    const approvalPath = join(dir, 'approval-http.json')
    writeApprovalEvidence(approvalPath)
    const requests: Array<{ url: string; body: Record<string, unknown>; headers: Record<string, string> }> = []
    const fetchImpl = async (url: string, init: RequestInit): Promise<Response> => {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>
      const headers = init.headers as Record<string, string>
      requests.push({ url, body, headers })
      if (body.method === 'notifications/initialized') {
        return new Response('', { status: 202 })
      }
      if (body.method === 'initialize') {
        return new Response(JSON.stringify({
          jsonrpc: '2.0',
          id: body.id,
          result: { protocolVersion: '2025-11-25', capabilities: {}, serverInfo: { name: 'mock-http', version: '1.0.0' } }
        }), {
          status: 200,
          headers: { 'mcp-session-id': 'session-http-1' }
        })
      }
      if (body.method === 'tools/list') {
        return new Response(`data: ${JSON.stringify({
          jsonrpc: '2.0',
          id: body.id,
          result: { tools: [{ name: 'http_tool', inputSchema: { type: 'object' } }] }
        })}\n\n`, {
          status: 200,
          headers: { 'content-type': 'text/event-stream' }
        })
      }
      if (body.method === 'tools/call') {
        return new Response(JSON.stringify({
          jsonrpc: '2.0',
          id: body.id,
          result: { content: [{ type: 'text', text: 'token=tool-result-secret' }] }
        }), { status: 200 })
      }
      return new Response(JSON.stringify({ error: { message: 'unexpected method' } }), { status: 400 })
    }

    const report = await collectMCPExecution({
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      fetchImpl,
      env: {
        ANALYTIX_RUNTIME_MCP_URL: 'https://user:secret@mcp.example/rpc?token=supersecret&workspace=ok',
        ANALYTIX_RUNTIME_GO_MCP_BEARER_TOKEN: 'bearer-secret-value',
        ANALYTIX_RUNTIME_GO_MCP_HEADERS_JSON: '{"X-Api-Key":"header-secret-value","X-Workspace":"workspace-1"}',
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'http_tool',
        ANALYTIX_RUNTIME_MCP_TOOL_ARGS_JSON: '{"input":"safe","token":"tool-arg-secret"}',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS: 'passed',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: approvalPath
      }
    })

    const probe = (report.credentialedProbes as Array<Record<string, unknown>>)[0]
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'passed',
      passed: true,
      credentialedExecution: true,
      connect: true,
      toolDiscoverySearch: true,
      toolCall: true,
      approvalUserInput: true,
      reconnect: true
    }))
    expect(report).not.toHaveProperty('legacyId')
    expect(probe).toEqual(expect.objectContaining({
      endpointFormat: 'http',
      transport: 'http-json-rpc',
      commandConfigured: false,
      commandHash: '',
      urlConfigured: true,
      url: 'https://mcp.example/rpc?workspace=ok',
      urlHash: expect.stringMatching(/^[a-f0-9]{64}$/),
      bearerAuthConfigured: true,
      requestShape: expect.objectContaining({
        protocol: 'mcp-jsonrpc-over-http',
        transport: 'streamable-http',
        headerNameClasses: expect.arrayContaining(['accept', 'content-type', 'custom-header', 'redacted-credential-header']),
        customHeadersConfigured: true,
        credentialHeaderConfigured: true,
        credentialValueRecorded: false,
        toolArgsValueRecorded: false
      }),
      calledToolNameHash: expect.stringMatching(/^[a-f0-9]{64}$/),
      toolCatalogDigest: expect.stringMatching(/^[a-f0-9]{64}$/),
      reconnectToolNameHash: expect.stringMatching(/^[a-f0-9]{64}$/),
      reconnectToolCatalogDigest: expect.stringMatching(/^[a-f0-9]{64}$/),
      reconnectToolCount: 1,
      toolResultContentTypes: ['text'],
      approvalUserInputEvidence: expect.objectContaining({
        status: 'passed',
        evidenceSha256: sha256Text(readFileSync(approvalPath, 'utf8')),
        rawValueRecorded: false,
        credentialSecretsRecorded: false
      })
    }))
    expect(requests.length).toBeGreaterThanOrEqual(7)
    expect(requests.some((request) => request.headers.authorization === 'Bearer bearer-secret-value')).toBe(true)
    expect(requests.some((request) => Object.entries(request.headers).some(([name, value]) => (
      name.toLowerCase() === 'x-api-key' && value === 'header-secret-value'
    )))).toBe(true)
    expect(requests.some((request) => request.headers['mcp-session-id'] === 'session-http-1')).toBe(true)
    const serialized = JSON.stringify(report)
    expect(serialized).not.toContain('user:secret')
    expect(serialized).not.toContain('supersecret')
    expect(serialized).not.toContain('bearer-secret-value')
    expect(serialized).not.toContain('header-secret-value')
    expect(serialized).not.toContain('tool-arg-secret')
    expect(serialized).not.toContain('tool-result-secret')
  })

  it('rejects secret-like MCP commands without hashing the command', async () => {
    const { collectMCPExecution } = await loadCollector()

    const report = await collectMCPExecution({
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      env: {
        ANALYTIX_RUNTIME_MCP_COMMAND: `${JSON.stringify(process.execPath)} token=command-secret`,
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'secret_tool'
      }
    })

    const probe = (report.credentialedProbes as Array<Record<string, unknown>>)[0]
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'failed',
      passed: false
    }))
    expect(report).not.toHaveProperty('legacyId')
    expect(probe).toEqual(expect.objectContaining({
      status: 'failed',
      skipped: false,
      commandConfigured: true,
      commandHash: '',
      message: 'MCP command contains secret-like material; pass credentials through environment variables instead'
    }))
    expect(JSON.stringify(report)).not.toContain('command-secret')
  })

  it('rejects approval/user-input evidence that only sets passed without passed status', async () => {
    const { collectMCPExecution } = await loadCollector()
    const dir = tempDir()
    const approvalPath = join(dir, 'approval.json')
    const mcpServerPath = join(dir, 'mock-mcp-approval-status.mjs')
    writeApprovalEvidence(approvalPath, { status: 'failed' })
    writeFileSync(mcpServerPath, `
import readline from 'node:readline'
const rl = readline.createInterface({ input: process.stdin })
rl.on('line', (line) => {
  const msg = JSON.parse(line)
  if (msg.method === 'notifications/initialized') return
  if (msg.method === 'initialize') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { protocolVersion: '2025-11-25', capabilities: {}, serverInfo: { name: 'mock', version: '1.0.0' } } }))
    return
  }
  if (msg.method === 'tools/list') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { tools: [{ name: 'approval_tool', inputSchema: { type: 'object' } }] } }))
    return
  }
  if (msg.method === 'tools/call') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { content: [{ type: 'text', text: 'ok' }] } }))
  }
})
`, 'utf8')

    const report = await collectMCPExecution({
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      env: {
        ANALYTIX_RUNTIME_MCP_COMMAND: `${JSON.stringify(process.execPath)} ${JSON.stringify(mcpServerPath)}`,
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'approval_tool',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS: 'passed',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: approvalPath
      }
    })

    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'live_blocked',
      passed: false,
      approvalUserInput: false
    }))
    expect(report).not.toHaveProperty('legacyId')
    expect(JSON.stringify(report)).toContain('approval/user-input evidence must pass')
  })

  it('rejects minimal approval/user-input evidence without audit fields', async () => {
    const { collectMCPExecution } = await loadCollector()
    const dir = tempDir()
    const approvalPath = join(dir, 'approval-minimal.json')
    const mcpServerPath = join(dir, 'mock-mcp-minimal-approval.mjs')
    writeFileSync(approvalPath, JSON.stringify({
      status: 'passed',
      passed: true,
      approvalUserInput: true
    }), 'utf8')
    writeFileSync(mcpServerPath, `
import readline from 'node:readline'
const rl = readline.createInterface({ input: process.stdin })
rl.on('line', (line) => {
  const msg = JSON.parse(line)
  if (msg.method === 'notifications/initialized') return
  if (msg.method === 'initialize') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { protocolVersion: '2025-11-25', capabilities: {}, serverInfo: { name: 'mock', version: '1.0.0' } } }))
    return
  }
  if (msg.method === 'tools/list') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { tools: [{ name: 'minimal_approval_tool', inputSchema: { type: 'object' } }] } }))
    return
  }
  if (msg.method === 'tools/call') {
    console.log(JSON.stringify({ jsonrpc: '2.0', id: msg.id, result: { content: [{ type: 'text', text: 'ok' }] } }))
  }
})
`, 'utf8')

    const report = await collectMCPExecution({
      now: '2026-06-24T00:00:00.000Z',
      timeoutMs: 5_000,
      env: {
        ANALYTIX_RUNTIME_MCP_COMMAND: `${JSON.stringify(process.execPath)} ${JSON.stringify(mcpServerPath)}`,
        ANALYTIX_RUNTIME_MCP_TOOL_NAME: 'minimal_approval_tool',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_STATUS: 'passed',
        ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE: approvalPath
      }
    })

    const probe = (report.credentialedProbes as Array<Record<string, unknown>>)[0]
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-mcp-execution',
      status: 'live_blocked',
      passed: false,
      approvalUserInput: false
    }))
    expect(report).not.toHaveProperty('legacyId')
    expect(probe.approvalUserInputEvidence).toEqual(expect.objectContaining({
      status: 'failed',
      evidenceSha256: sha256Text(readFileSync(approvalPath, 'utf8')),
      missingEvidenceFields: expect.arrayContaining([
        'schemaVersion',
        'id',
        'rawValueRecorded:false',
        'credentialSecretsRecorded:false'
      ])
    }))
  })
})
