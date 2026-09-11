import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import {
  existsSync,
  linkSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { afterEach, describe, expect, test } from 'vitest'
import {
  evaluateRuntimeGoFormalEvidence,
  preflightRuntimeGoFormalEvidence,
  runtimeGoFormalEvidenceContract,
  runtimeGoFormalEvidenceTestInternals
// @ts-expect-error The formal-evidence CLI is a JavaScript module with no TypeScript declaration.
} from '../../scripts/runtime-go-formal-evidence.mjs'
import {
  artifactLegalObligationsTestInternals,
  inspectExactArtifactLegalInventory
// @ts-expect-error The artifact legal-obligations CLI is a JavaScript module with no TypeScript declaration.
} from '../../scripts/artifact-legal-obligations-audit.mjs'

const repoRoot = resolve(__dirname, '../..')
const require = createRequire(import.meta.url)
const afterPackModule = require('../../scripts/after-pack.cjs')
const packagedAuthorityContract = afterPackModule._internals
const nativeComponentContract = require('../../scripts/native-component-contract.cjs')
const source = {
  head: '1'.repeat(40),
  tree: '2'.repeat(40)
}
const productPackageAuthor = 'Guoqin He (GitHub: Eysn0130)'
const productPluginAuthor = {
  name: 'Guoqin He',
  url: 'https://github.com/Eysn0130'
}
const openClawShimVersion = '2026.6.6+analytix.shim.0'
const openClawContributor =
  'OpenClaw contributors (upstream v2026.5.18; https://github.com/openclaw/openclaw)'
const openClawProvenance = {
  upstreamRepository: 'https://github.com/openclaw/openclaw',
  upstreamTag: 'v2026.5.18',
  upstreamTagObject: '0c5e335df4311f135a36f7f72fcd87784dd01c96',
  upstreamCommit: '50a2481652b6a62d573ece3cead60400dc77020d'
}
const digests = {
  authoritySha256: createHash('sha256').update('authority-file').digest('hex'),
  authorityDigest: createHash('sha256').update('authority-payload').digest('hex'),
  executableSha256: createHash('sha256').update('electron-executable').digest('hex'),
  runtimeServerSha256: createHash('sha256').update('runtime-server').digest('hex'),
  appAsarSha256: createHash('sha256').update('app-asar').digest('hex'),
  worktreeSnapshotDigest: createHash('sha256').update('worktree-snapshot').digest('hex'),
  appPathHash: createHash('sha256').update('app-path').digest('hex')
}
const temporaryDirectories: string[] = []
const exactArtifactsByDirectory = new Map<
  string,
  ReturnType<typeof inspectExactArtifactLegalInventory>
>()

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true })
  }
})

function sha256(value: Buffer | string): string {
  return createHash('sha256').update(value).digest('hex')
}

function runtimeLicenseArtifactEntries(): Record<string, string> {
  return {
    'packages/runtime/package.json': JSON.stringify({
      name: 'analytix-runtime', version: '0.1.0', author: productPackageAuthor,
      license: 'Apache-2.0'
    }),
    'packages/runtime/package-lock.json': JSON.stringify({
      name: 'analytix-runtime',
      version: '0.1.0',
      lockfileVersion: 3,
      packages: {
        '': { name: 'analytix-runtime', version: '0.1.0', license: 'Apache-2.0' }
      }
    }),
    'node_modules/analytix-computer-use/package.json': JSON.stringify({
      name: 'analytix-computer-use', version: '0.2.3', author: productPackageAuthor,
      license: 'MIT'
    }),
    'node_modules/analytix-computer-use/LICENSE': 'MIT License\nfixture computer use\n',
    'node_modules/analytix-computer-use/plugins/analytix-computer-use/.codex-plugin/plugin.json':
      JSON.stringify({
        name: 'analytix-computer-use', version: '0.2.3', author: productPluginAuthor,
        license: 'MIT'
      }),
    'node_modules/openclaw/package.json': JSON.stringify({
      name: 'openclaw',
      version: openClawShimVersion,
      author: productPackageAuthor,
      maintainers: [productPackageAuthor],
      contributors: [openClawContributor],
      license: 'MIT',
      analytixProvenance: openClawProvenance
    }),
    'node_modules/openclaw/LICENSE': readFileSync(
      join(repoRoot, 'vendor/openclaw-shim/LICENSE'),
      'utf8'
    )
  }
}

function canonicalJSON(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function harnessEntries(relativePaths: readonly string[]) {
  return relativePaths.map((relativePath) => {
    const bytes = readFileSync(join(repoRoot, relativePath))
    return {
      name: relativePath === 'scripts/use-analytix-cache.sh' ||
          relativePath === 'scripts/analytix-cache-storage.zsh'
        ? relativePath
        : basename(relativePath),
      regular: true,
      byteLength: bytes.length,
      sha256: sha256(bytes)
    }
  })
}

function passedChecks(ids: readonly string[], status: 'passed' | 'PASS') {
  return ids.map((id) => ({ id, status, message: status }))
}

function a0StageSnapshot(
  stage: string,
  checkIds: string[],
  completedFields: Record<string, boolean | number | string>
) {
  const projected = { stage, checkIds, completedFields }
  return { ...projected, snapshotDigest: sha256(canonicalJSON(projected)) }
}

function a0PassStageSnapshots() {
  return [
    a0StageSnapshot('first-launch-prerequisites', [
      'external-real-repository-contract',
      'trusted-cache-tmpdir',
      'formal-packaged-artifact',
      'isolated-profile-and-repository',
      'configured-network-provider',
      'packaged-first-launch'
    ], { firstLaunchObserved: true }),
    a0StageSnapshot('plan-turn', ['ordinary-agent-workflow'], {
      planModeSelected: true,
      planTurnObserved: true,
      planReasoningEffortBound: true
    }),
    a0StageSnapshot('ordinary-agent-functional-workflow', [
      'real-repository-test',
      'bounded-subagent',
      'git-skill-mcp-research-writing'
    ], { realTestObserved: true }),
    a0StageSnapshot('ordinary-agent-workflow', [
      'ordinary-agent-workflow',
      'real-repository-test',
      'bounded-subagent'
    ], { ordinaryWorkflowCompleted: true }),
    a0StageSnapshot(
      'protected-funds-source-unavailable',
      ['protected-funds-source-unavailable'],
      { protectedFundsSourceUnavailableObserved: true }
    ),
    a0StageSnapshot(
      'long-context-continuation',
      ['long-context-continuation'],
      {
        longContextContinuationObserved: true,
        compactionCount: 0,
        manualCompactionAuto: false
      }
    ),
    a0StageSnapshot('composer-compaction-submit', [
      'composer-compaction-submit',
      'nonzero-compaction'
    ], { manualCompactionObserved: true, compactionCount: 1 }),
    a0StageSnapshot('normal-first-quit', ['normal-first-quit'], {
      normalFirstQuitObserved: true
    }),
    a0StageSnapshot('relaunch-provider-continuation', [
      'fresh-packaged-relaunch',
      'relaunch-provider-continuation'
    ], { relaunchProviderContinuationObserved: true })
  ]
}

function a0Report() {
  const entries = harnessEntries(runtimeGoFormalEvidenceContract.a0HarnessFiles)
  const harness = {
    scriptSha256: entries[0].sha256,
    contractManifestSha256: sha256(canonicalJSON(entries)),
    contractManifestBound: true,
    harnessSourceRootBound: true,
    sourceClosureBound: true,
    sourceClosureSnapshotDigest: digests.worktreeSnapshotDigest,
    packagedSourceClosureSnapshotDigest: digests.worktreeSnapshotDigest,
    entries,
    formalExecutionBounds: {
      userGlobalMaxModelSteps: 32,
      plannerMaxModelSteps: 12,
      childMaxModelSteps: 8,
      childTimeBudgetMs: 180_000
    }
  }
  const app = {
    targetKey: 'darwin-arm64',
    appPathHash: digests.appPathHash,
    ok: true,
    blocked: false,
    blocker: '',
    sourceCommit: source.head,
    authoritySha256: digests.authoritySha256,
    authorityDigest: digests.authorityDigest,
    executableSha256: digests.executableSha256,
    runtimeServerSha256: digests.runtimeServerSha256,
    appAsarSha256: digests.appAsarSha256,
    packagedSkillSourceMatched: true,
    packagedScheduleSourceContractBound: true,
    packagedScheduleStrictEmptyInputSchemaBound: true,
    packagedScheduleNonMutatingListImplementationBound: true,
    codeSignatureVerified: true,
    bundledRuntimeGoSourcePresent: false,
    worktreeSnapshotBinding: {
      ok: true,
      blocker: '',
      matched: true,
      current: {
        digest: digests.worktreeSnapshotDigest,
        classification: 'clean'
      },
      packaged: {
        digest: digests.worktreeSnapshotDigest,
        classification: 'clean',
        authorityClassification: 'development_clean_non_publishable'
      }
    }
  }
  const phase = (name: string) => ({
    phase: name,
    ok: true,
    blocker: '',
    codeSignatureVerified: true,
    stableFields: true,
    worktreeStable: true,
    observedAuthoritySha256: digests.authoritySha256,
    observedAuthorityDigest: digests.authorityDigest,
    observedExecutableSha256: digests.executableSha256,
    observedRuntimeServerSha256: digests.runtimeServerSha256,
    observedAppAsarSha256: digests.appAsarSha256
  })
  const repositoryDigest = sha256('a0-repository-final-source')
  const publicSeam = () => ({
    healthOk: true,
    runtimeInfoOk: true,
    runtimeToolsOk: true,
    runtimeThreadListProbeOk: true,
    runtimeThreadListProbeStatus: 200,
    ordinaryCatalogNonempty: true,
    gitCommandAvailable: true,
    skillCatalogAvailable: true,
    skillCount: 1,
    ordinaryMCPAvailable: true,
    ordinaryMCPServerCount: 1,
    ordinaryMCPToolCount: 1,
    fundsExecutionUnavailable: true,
    fundsServerDiagnosticCount: 0,
    ordinaryToolContractCount: 1,
    ordinaryToolCatalogHash: sha256('a0-ordinary-tool-catalog'),
    rendererTargetCount: 1,
    electronMainPid: 101,
    runtimeListenerPid: 102,
    runtimeListenerCount: 1,
    runtimeBackendProcessCount: 1,
    runtimeServerProcessCount: 1,
    desktopPrivateHistoryMigrationProcessCount: 0,
    bundledPluginMaterializationProcessCount: 0,
    unknownRuntimeProcessCount: 0,
    rendererReadFailureObserved: false,
    runtimeListenerIsTaskOwnedDescendant: true,
    exactPackagedRuntimeExecutable: true
  })
  const capture = (label: string) => ({
    captured: true,
    sha256: sha256(label),
    width: 1280,
    height: 720,
    viewport: { width: 1280, height: 720, scale: 1 }
  })
  return {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-a',
    stage: 'packaged-general-agent-milestone-a',
    generatedAt: new Date().toISOString(),
    sourceCommit: source.head,
    testHarnessCommit: source.head,
    harness,
    status: 'passed',
    passed: true,
    credentialSecretsRecorded: false,
    rawProviderPayloadRecorded: false,
    syntheticProviderUsed: false,
    localProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    executionBlocker: '',
    operatorCheckpoint: {
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: true,
      method: 'visible-computer-use',
      automatedCredentialEntryUsed: true,
      status: 'completed'
    },
    stageSnapshots: a0PassStageSnapshots(),
    app,
    repository: {
      required: true,
      mode: 'external-preexisting-real-repository',
      configured: true,
      validated: true,
      baselineValidated: true,
      finalValidated: true,
      fixtureSubstituteRejected: true,
      originBound: true,
      freshOriginVerified: true,
      formalAcceptance: true,
      preflightValidated: true,
      mutationApplied: true,
      ownerRootBound: true,
      ownerOnlyIsolationBound: true,
      gitCommonDirContained: true,
      symlinkHardlinkIsolationBound: true,
      planArtifactExcludeBound: true,
      planArtifactPathSafe: true,
      finalOnlyIntendedSourceChanged: true,
      finalExpectedSourceBound: true,
      finalExpectedSourceFileCount: 1,
      finalExpectedSourceMismatchCount: 0,
      finalExpectedSourceDigest: repositoryDigest,
      finalObservedSourceDigest: repositoryDigest,
      finalGitStatusSha256: sha256('a0-final-git-status'),
      preservedAfterHarnessCleanup: true,
      blocker: ''
    },
    artifactRevalidation: {
      beforeFirstLaunch: phase('before-first-launch'),
      afterFirstExit: phase('after-first-exit'),
      beforeSecondLaunch: phase('before-second-launch'),
      afterFinalExit: phase('after-final-exit')
    },
    provider: {
      configured: true,
      credentialConfigured: true,
      credentialAuthorityBound: true,
      credentialRecorded: false,
      credentialAuthority: 'local-provider-registry-secret-store',
      normalLocalProviderSetupObserved: true,
      localCredentialEvidence: { ok: true, blocked: false, blocker: null, sourceBound: true, entryMethod: 'visible-computer-use', automatedCredentialEntryUsed: true, expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1, status: 'passed', scannedFileCount: 3, findingCount: 0, settingsFindingCount: 0, reportFindingCount: 0, unsafeEntryCount: 0, symlinkCount: 0, encodingCoverage: ['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'], protectedStoreOwnerPrivate: true },
      providerIdHash: sha256('a0-provider'),
      modelHash: sha256('deepseek-v4-flash'),
      requiredModelHash: sha256('deepseek-v4-flash'),
      baseUrlOriginHash: sha256('a0-provider-origin'),
      contextWindowTokensBound: true,
      firstLaunchReasoningEffortSelectedThroughVisibleUi: true,
      secondLaunchReasoningEffortSelectedThroughVisibleUi: true,
      planReasoningEffortBound: true,
      ordinaryReasoningEffortBound: true,
      longContextReasoningEffortBound: true,
      relaunchReasoningEffortBound: true,
      networkTurnCompleted: true,
      threadProviderBound: true,
      threadModelBound: true,
      resultTurnProviderReceiptBound: true,
      providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 1,
      providerAttemptCount: 1,
      successfulProviderAttemptCount: 1
    },
    workflow: {
      firstLaunchObserved: true,
      composerWorkflowSubmitted: true,
      planComposerSubmitted: true,
      planModeSelected: true,
      planTurnObserved: true,
      planReasoningEffortBound: true,
      agentModeRestored: true,
      sameThreadPlanAgent: true,
      ordinaryWorkflowCompleted: true,
      readObserved: true,
      planObserved: true,
      todoObserved: true,
      writeObserved: true,
      realTestObserved: true,
      parentOwnedTestCrossCheckPassed: true,
      isolatedGitRepositoryObserved: true,
      protectedRepositoryInputsBound: true,
      onlyIntendedSourceChanged: true,
      subagentObserved: true,
      gitCommandObserved: true,
      skillObserved: true,
      ordinaryMCPObserved: true,
      researchWritingObserved: true,
      successfulToolResultsObserved: true,
      todosCompleted: true,
      exactlyOneBoundedSubagentCompleted: true,
      longContextContinuationObserved: true,
      protectedFundsSourceUnavailableObserved: true,
      manualCompactionBaselineBound: true,
      manualCompactionBaselineCount: 0,
      manualCompactionBaselineDigest: sha256('a0-compaction-baseline'),
      manualCompactionCandidateDigest: sha256('a0-compaction-candidate'),
      manualCompactionNewAfterBaseline: true,
      compactionCount: 1,
      manualCompactionObserved: true,
      manualCompactionAuto: false,
      manualCompactionReplacedTokens: 1,
      manualCompactionSourceDigest: sha256('a0-compaction-source'),
      manualCompactionSourceItemIdsDigest: sha256('a0-compaction-items'),
      manualCompactionSourceAncestryBound: true,
      manualCompactionNonzeroBound: true,
      manualCompactionProjectionClass: 'ordinary_exact',
      normalFirstQuitObserved: true,
      normalFirstQuitReasonCode: 'normal_quit_observed',
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
      normalFinalQuitReasonCode: 'normal_quit_observed',
      finalExitCode: 0,
      finalExitSignal: null,
      zeroResidualProcesses: true
    },
    parentOwnedRepositoryTest: {
      passed: true,
      blocker: '',
      parentProcessOwned: true,
      exitCode: 0,
      signal: '',
      repositoryStableBeforeAndAfter: true,
      sandboxCleanupSucceeded: true,
      substitutesForAgentInvocationBinding: false
    },
    caseCapability: {
      additiveNotReplacement: true,
      bundledFundsMissingAuthorityPreconditionEstablished: true,
      firstLaunchBundledFundsMaterializationFailureObserved: true,
      secondLaunchBundledFundsMaterializationFailureObserved: true,
      fundsTransportAbsentOnBothLaunches: true,
      privateAuthorityInputsAbsent: true,
      firstLaunchFundsExecutionUnavailable: true,
      secondLaunchFundsExecutionUnavailable: true,
      protectedFundsSourceUnavailable: true,
      restartedProtectedFundsSourceUnavailable: true,
      protectedFundsClaimCount: 0,
      protectedFundsReceiptCount: 0,
      protectedFundsSuccessfulExecutionCount: 0,
      ordinaryContinuedAfterProtectedFundsBlock: true,
      ordinaryContinuedAfterRestart: true,
      ordinaryCatalogAvailable: true,
      ordinaryWorkflowCompleted: true,
      unavailableWhileOrdinaryCapabilitiesRetained: true
    },
    publicSeams: {
      firstLaunch: publicSeam(),
      secondLaunch: publicSeam()
    },
    isolation: {
      cacheTmpdirVerified: true,
      systemUserDataTouched: false,
      isolatedHomeUsedByChild: true,
      isolatedLoginKeychainCreated: true,
      isolatedLoginKeychainDefaultBound: true,
      isolatedLoginKeychainReusedForTwoLaunches: true,
      isolatedLoginKeychainPasswordRecorded: false,
      chromiumTempBoundToTrustedCache: true,
      sameDataDirsOnRelaunch: true,
      sandboxRemoved: true
    },
    redaction: {
      status: 'passed',
      secretMaterialFound: false,
      settingsFindingCount: 0,
      reportFindingCount: 0,
      unsafeEntryCount: 0,
      symlinkCount: 0,
      sourceSecretCount: 1,
      uniqueSecretCount: 1,
      expectedSecretCount: 1,
      sourceBound: true,
      scannedFileCount: 3,
      encodingCoverage: ['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'],
      credentialRecorded: false
    },
    visualEvidence: {
      firstCompleted: capture('a0-first-visual'),
      recoveredAfterRelaunch: capture('a0-second-visual'),
      screenshotsUsedAsFunctionalVerdict: false,
      credentialScanCovered: true
    },
    checks: passedChecks([
      ...runtimeGoFormalEvidenceContract.a0RequiredCheckIds,
      ...runtimeGoFormalEvidenceContract.a0AuxiliaryCheckIds
    ], 'passed'),
    failedCheckIds: [],
    skippedCheckIds: [],
    liveBlockedCheckIds: [],
    failureCheckIds: [],
    failureCheckIdsSemantics: 'all_non_pass',
    blockers: [] as string[],
    missingExternalInputs: [] as unknown[]
  }
}

function a0SnapshotByStage(report: ReturnType<typeof a0Report>, stage: string) {
  return report.stageSnapshots.find((snapshot) => snapshot.stage === stage)!
}

function refreshA0StageSnapshotDigest(snapshot: ReturnType<typeof a0StageSnapshot>) {
  snapshot.snapshotDigest = sha256(canonicalJSON({
    stage: snapshot.stage,
    checkIds: snapshot.checkIds,
    completedFields: snapshot.completedFields
  }))
}

function b1Artifact() {
  return {
    ok: true,
    blocked: false,
    blocker: '',
    targetKey: 'darwin-arm64',
    appPathHash: digests.appPathHash,
    sourceCommit: source.head,
    authoritySha256: digests.authoritySha256,
    authorityDigest: digests.authorityDigest,
    executableSha256: digests.executableSha256,
    runtimeServerSha256: digests.runtimeServerSha256,
    appAsarSha256: digests.appAsarSha256,
    worktreeSnapshotDigest: digests.worktreeSnapshotDigest,
    codeSignatureVerified: true,
    fundsPluginArtifactBound: true
  }
}

function b1Report() {
  const entries = harnessEntries(runtimeGoFormalEvidenceContract.b1HarnessFiles)
  const artifact = b1Artifact()
  const phaseLanes = Object.fromEntries(
    Object.entries(runtimeGoFormalEvidenceContract.b1PhaseLanes as Record<
      string,
      { phaseOwner: string; dependencies: readonly string[] }
    >).map(([id, lane]) => [
      id,
      {
        phaseOwner: lane.phaseOwner,
        dependencies: [...lane.dependencies],
        status: 'PASS',
        blocker: '',
        executed: true,
        not_executed: false
      }
    ])
  )
  const publicSeam = (fundsAvailable: boolean) => ({
    healthOk: true,
    runtimeInfoOk: true,
    ordinaryCatalogNonempty: true,
    ordinaryToolContractCount: 1,
    ordinaryToolCatalogHash: sha256('b1-ordinary-tool-catalog'),
    fundsAvailable,
    fundsExplicitlyUnavailable: !fundsAvailable,
    fundsServerDiagnosticCount: 0
  })
  const publicPIIScan = {
    ok: true,
    forbiddenGenericProviderChannels: {
      surfaceCount: 12,
      completePIIFindingCount: 0,
      internalReferenceFindingCount: 0,
      incompleteSurfaceCount: 0,
      unsafeLogEntryCount: 0
    },
    allowedTypedLocalSink: {
      excludedFromForbiddenScan: true,
      sourceExactFieldsAllowed: true,
      scanBoundary: 'typed-local-display-only'
    },
    publicSurfaceCount: 12,
    incompletePublicSurfaceCount: 0,
    publicFindingCount: 0,
    publicInternalReferenceFindingCount: 0,
    logScannedFileCount: 2,
    logFindingCount: 0,
    logInternalReferenceFindingCount: 0,
    logUnsafeEntryCount: 0
  }
  return {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-b',
    stage: 'packaged-real-duckdb-funds-analysis-milestone-b',
    generatedAt: new Date().toISOString(),
    sourceCommit: source.head,
    harnessCommit: source.head,
    harness: {
      scriptSha256: entries[0].sha256,
      contractManifestSha256: sha256(canonicalJSON(entries)),
      entries
    },
    status: 'PASS',
    passed: true,
    runtimeBlocker: '',
    app: { targetKey: 'darwin-arm64', appPathHash: digests.appPathHash },
    acceptanceClass:
      'formal-packaged-typed-local-display-external-owner-isolated-real-case',
    mockUsed: false,
    fixtureUsed: false,
    syntheticProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    directSnapshotIPCUsed: false,
    arbitrarySQLUsed: false,
    rendererDatabasePathReceived: false,
    completePIIRecorded: false,
    phaseLanes,
    postRCChecks: [{
      id: 'authorized-joint-case-linkage',
      status: 'UNVERIFIED',
      message: 'explicit joint-case product behavior is deferred outside the current macOS A0/B1 train'
    }],
    externalCase: {
      configured: true,
      ownerIsolated: true,
      synthetic: false,
      contractSha256: sha256('b1-case-contract'),
      provenanceSha256: sha256('b1-case-provenance'),
      snapshotCount: 2,
      preserved: true
    },
    managedProductData: {
      configured: true,
      bootstrapSha256: sha256('b1-managed-bootstrap'),
      installationAuthorityKeySha256: sha256('b1-installation-authority'),
      volumeIdentityDigest: sha256('b1-product-volume'),
      authorityRootCount: 2,
      bootstrapInstalled: true,
      installationAuthoritySeeded: true
    },
    providerAudit: {
      configured: true,
      challengePublished: true,
      challengeDigest: sha256('b1-provider-audit-challenge'),
      receiptVerified: true,
      receiptSha256: sha256('b1-provider-audit-receipt'),
      minimumObservedProviderRequestCount: 8,
      exactObservedProviderRequestCount: 8,
      providerRequestCount: 8,
      scannedRequestBodyCount: 8,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 6,
      providerSafeSemanticValidationCount: 6
    },
    typedLocalDisplay: {
      required: true,
      status: 'PASS',
      contract: 'analytix.typed-local-display.v1',
      fullModeExactValue: true,
      maskedModeLocalOnly: true,
      acceptedSlotBindingVerified: true,
      ordinaryRendererCompletePIIAbsent: true,
      forbiddenGenericProviderChannelsZero: true,
      invalidatedAfterAuthorityChange: true,
      reason: '',
      blocksMilestoneB: false
    },
    directSourcePreview: {
      required: true,
      status: 'PASS',
      contract: 'analytix.direct-source-preview.v1',
      fullModeExactValue: true,
      maskedModeLocalOnly: true,
      providerRequestAbsent: true,
      claimReceiptFinalGateAbsent: true,
      invalidatedAfterAuthorityChange: true,
      reason: '',
      blocksMilestoneB: false
    },
    provider: {
      configured: true,
      normalLocalProviderSetupObserved: true,
      localCredentialEvidence: { ok: true, blocked: false, blocker: null, sourceBound: true, entryMethod: 'visible-computer-use', automatedCredentialEntryUsed: true, expectedSecretCount: 1, sourceSecretCount: 1, uniqueSecretCount: 1, status: 'passed', scannedFileCount: 3, findingCount: 0, settingsFindingCount: 0, reportFindingCount: 0, unsafeEntryCount: 0, symlinkCount: 0, encodingCoverage: ['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'], protectedStoreOwnerPrivate: true },
      automatedTestBootstrapObserved: false,
      family: 'deepseek',
      credentialAuthority: 'local-provider-registry-secret-store',
      credentialAuthorityBound: true,
      modelHash: sha256('deepseek-v4-flash'),
      baseUrlOriginHash: sha256('b1-provider-origin'),
    },
    isolation: {
      trustedCacheTmpdir: true,
      isolatedHomeUsed: true,
      isolatedUserDataUsed: true,
      sandboxRemoved: true,
      productRunRemoved: true,
      productDataOutsideCache: true,
      productOwnerStable: true,
      productVolumeRevalidated: true,
      userDataRuntimeSeparated: true,
      externalCaseOutsideSandbox: true,
      ordinaryCodeWorkspaceCleaned: true,
      caseBindingRestored: true,
      protectedSourceAliasCleaned: true
    },
    publicSeams: {
      firstLaunch: publicSeam(false),
      snapshotOne: publicSeam(true),
      snapshotTwo: publicSeam(true),
      relaunch: publicSeam(true),
      exactPackagedRuntimeExecutable: true
    },
    workflow: {
      sameThread: true,
      ordinaryTurnCount: 5,
      fundsTurnCount: 6,
      blockedCaseTurnCount: 2,
      compactionCount: 1,
      exactRecovery: true,
      exactSubagentRecovery: true,
      typedLocalDisplayRehydrated: true,
      stableEntityDisplayAcrossSnapshotAndRestart: true,
      completedRunnableFlow: true,
      typedLocalDisplay: {
        fullModeExactValue: true,
        maskedModeLocalOnly: true,
        exactValueOnlyInTypedSink: true,
        acceptedSlotBindingVerified: true,
        forbiddenGenericProviderChannelsZero: true,
        directSourcePreview: {
          status: 'PASS',
          paginationObserved: true,
          invalidatedAfterAuthorityChange: true
        }
      },
      directSourcePreview: {
        fullModeExactValue: true,
        maskedModeLocalOnly: true,
        paginationObserved: true,
        providerRequestAbsent: true,
        agentTurnAbsent: true,
        mcpCallAbsent: true,
        claimReceiptFinalGateAbsent: true,
        invalidatedAfterAuthorityChange: true,
        revokedAfterCaseChange: true
      },
      typedLocalDisplayRevocation: {
        retainedAfterOrdinaryTurn: true,
        retainedSnapshotOneReResolved: true,
        snapshotPreviewChanged: true,
        observationKind: 'accepted_slot_display',
        slotCount: 1
      },
      publicPIIScan
    },
    exit: {
      firstNormal: true,
      finalNormal: true,
      residualProcessCount: 0
    },
    artifact,
    artifactFinal: structuredClone(artifact),
    checks: passedChecks(runtimeGoFormalEvidenceContract.b1RequiredCheckIds, 'PASS'),
    failureCheckIds: [],
    blockedCheckIds: [],
    unverifiedCheckIds: []
  }
}

function exactArtifact() {
  return {
    provided: true,
    passed: true,
    engineeringAdmission: true,
    artifactEntry: '/formal/analytix.app',
    artifact: {
      path: '/formal/analytix.app',
      sha256: sha256('complete-app-directory'),
      byteLength: 4096,
      components: [{
        artifactEntry: 'Contents/Resources/app.asar',
        sha256: digests.appAsarSha256,
        byteLength: Buffer.byteLength('app-asar')
      }]
    }
  }
}

function writePreflightBundle(a0 = a0Report(), b1 = b1Report()): string {
  const directory = mkdtempSync(join(tmpdir(), 'analytix-formal-preflight-'))
  temporaryDirectories.push(directory)
  writeFileSync(join(directory, runtimeGoFormalEvidenceContract.reportFiles.a0), JSON.stringify(a0))
  writeFileSync(join(directory, runtimeGoFormalEvidenceContract.reportFiles.b1), JSON.stringify(b1))
  return directory
}

function mutateAdmissionSentinel(target: Record<string, any>, path: string): void {
  const parts = path.split('.')
  let owner: Record<string, any> = target
  for (const part of parts.slice(0, -1)) owner = owner[part]
  const key = parts.at(-1)!
  const current = owner[key]
  owner[key] = typeof current === 'boolean'
    ? !current
    : typeof current === 'number'
      ? -1
      : typeof current === 'string'
        ? current === '' ? 'blocked' : ''
        : current === null
          ? 'SIGTERM'
          : null
}

const a0AdmissionSentinels = [
  ['a0_operator_checkpoint_pass_projection_invalid', [
    'operatorCheckpoint.kind', 'operatorCheckpoint.required',
    'operatorCheckpoint.declared', 'operatorCheckpoint.method',
    'operatorCheckpoint.automatedCredentialEntryUsed', 'operatorCheckpoint.status'
  ]],
  ['a0_repository_pass_projection_invalid', [
    'repository.required', 'repository.mode', 'repository.configured',
    'repository.validated', 'repository.baselineValidated', 'repository.finalValidated',
    'repository.fixtureSubstituteRejected', 'repository.originBound',
    'repository.freshOriginVerified', 'repository.formalAcceptance',
    'repository.preflightValidated', 'repository.mutationApplied',
    'repository.ownerRootBound', 'repository.ownerOnlyIsolationBound',
    'repository.gitCommonDirContained', 'repository.symlinkHardlinkIsolationBound',
    'repository.planArtifactExcludeBound', 'repository.planArtifactPathSafe',
    'repository.finalOnlyIntendedSourceChanged', 'repository.finalExpectedSourceBound',
    'repository.finalExpectedSourceFileCount', 'repository.finalExpectedSourceMismatchCount',
    'repository.finalExpectedSourceDigest', 'repository.finalObservedSourceDigest',
    'repository.finalGitStatusSha256', 'repository.preservedAfterHarnessCleanup',
    'repository.blocker'
  ]],
  ['a0_provider_pass_projection_invalid', [
    'provider.configured', 'provider.credentialConfigured',
    'provider.credentialAuthorityBound', 'provider.credentialRecorded',
    'provider.credentialAuthority', 'provider.normalLocalProviderSetupObserved',
    'provider.localCredentialEvidence',
    'provider.providerIdHash', 'provider.modelHash', 'provider.requiredModelHash',
    'provider.baseUrlOriginHash', 'provider.contextWindowTokensBound',
    'provider.firstLaunchReasoningEffortSelectedThroughVisibleUi',
    'provider.secondLaunchReasoningEffortSelectedThroughVisibleUi',
    'provider.planReasoningEffortBound', 'provider.ordinaryReasoningEffortBound',
    'provider.longContextReasoningEffortBound', 'provider.relaunchReasoningEffortBound',
    'provider.networkTurnCompleted', 'provider.threadProviderBound',
    'provider.threadModelBound', 'provider.resultTurnProviderReceiptBound',
    'provider.providerAttemptTelemetryValid', 'provider.providerLogicalCallCount',
    'provider.providerAttemptCount', 'provider.successfulProviderAttemptCount'
  ]],
  ['a0_workflow_pass_projection_invalid', [
    'workflow.firstLaunchObserved', 'workflow.composerWorkflowSubmitted',
    'workflow.planComposerSubmitted', 'workflow.planModeSelected',
    'workflow.planTurnObserved', 'workflow.agentModeRestored',
    'workflow.sameThreadPlanAgent', 'workflow.ordinaryWorkflowCompleted',
    'workflow.readObserved', 'workflow.planObserved', 'workflow.todoObserved',
    'workflow.writeObserved', 'workflow.realTestObserved',
    'workflow.parentOwnedTestCrossCheckPassed', 'workflow.isolatedGitRepositoryObserved',
    'workflow.protectedRepositoryInputsBound', 'workflow.onlyIntendedSourceChanged',
    'workflow.subagentObserved', 'workflow.gitCommandObserved', 'workflow.skillObserved',
    'workflow.ordinaryMCPObserved', 'workflow.researchWritingObserved',
    'workflow.successfulToolResultsObserved', 'workflow.todosCompleted',
    'workflow.exactlyOneBoundedSubagentCompleted',
    'workflow.longContextContinuationObserved', 'workflow.manualCompactionBaselineBound',
    'workflow.manualCompactionBaselineCount', 'workflow.manualCompactionBaselineDigest',
    'workflow.manualCompactionCandidateDigest', 'workflow.manualCompactionNewAfterBaseline',
    'workflow.compactionCount', 'workflow.manualCompactionObserved',
    'workflow.manualCompactionAuto', 'workflow.manualCompactionReplacedTokens',
    'workflow.manualCompactionSourceDigest', 'workflow.manualCompactionSourceItemIdsDigest',
    'workflow.manualCompactionSourceAncestryBound',
    'workflow.manualCompactionProjectionClass', 'workflow.normalFirstQuitObserved',
    'workflow.normalFirstQuitReasonCode', 'workflow.firstExitCode',
    'workflow.firstExitSignal', 'workflow.freshProcessRelaunchObserved',
    'workflow.relaunchProviderContinuationObserved', 'workflow.exactThreadRecovered',
    'workflow.exactTodosRecovered', 'workflow.exactSubagentsRecovered',
    'workflow.exactCompactionsRecovered', 'workflow.exactResultRecovered',
    'workflow.rendererVisibleThreadRecovered', 'workflow.rendererVisibleTodosRecovered',
    'workflow.rendererVisibleSubagentRecovered',
    'workflow.rendererVisibleCompactionRecovered',
    'workflow.rendererVisibleResultRecovered', 'workflow.normalFinalQuitObserved',
    'workflow.normalFinalQuitReasonCode', 'workflow.finalExitCode',
    'workflow.finalExitSignal', 'workflow.zeroResidualProcesses'
  ]],
  ['a0_parent_repository_test_pass_projection_invalid', [
    'parentOwnedRepositoryTest.passed', 'parentOwnedRepositoryTest.blocker',
    'parentOwnedRepositoryTest.parentProcessOwned', 'parentOwnedRepositoryTest.exitCode',
    'parentOwnedRepositoryTest.signal',
    'parentOwnedRepositoryTest.repositoryStableBeforeAndAfter',
    'parentOwnedRepositoryTest.sandboxCleanupSucceeded',
    'parentOwnedRepositoryTest.substitutesForAgentInvocationBinding'
  ]],
  ['a0_case_capability_pass_projection_invalid', [
    'caseCapability.additiveNotReplacement',
    'caseCapability.bundledFundsMissingAuthorityPreconditionEstablished',
    'caseCapability.firstLaunchBundledFundsMaterializationFailureObserved',
    'caseCapability.secondLaunchBundledFundsMaterializationFailureObserved',
    'caseCapability.fundsTransportAbsentOnBothLaunches',
    'caseCapability.privateAuthorityInputsAbsent',
    'caseCapability.firstLaunchFundsExecutionUnavailable',
    'caseCapability.secondLaunchFundsExecutionUnavailable',
    'caseCapability.protectedFundsSourceUnavailable',
    'caseCapability.restartedProtectedFundsSourceUnavailable',
    'caseCapability.protectedFundsClaimCount', 'caseCapability.protectedFundsReceiptCount',
    'caseCapability.protectedFundsSuccessfulExecutionCount',
    'caseCapability.ordinaryContinuedAfterProtectedFundsBlock',
    'caseCapability.ordinaryContinuedAfterRestart',
    'caseCapability.ordinaryCatalogAvailable', 'caseCapability.ordinaryWorkflowCompleted',
    'caseCapability.unavailableWhileOrdinaryCapabilitiesRetained'
  ]],
  ['a0_isolation_cleanup_pass_projection_invalid', [
    'isolation.cacheTmpdirVerified', 'isolation.systemUserDataTouched',
    'isolation.isolatedHomeUsedByChild', 'isolation.isolatedLoginKeychainCreated',
    'isolation.isolatedLoginKeychainDefaultBound',
    'isolation.isolatedLoginKeychainReusedForTwoLaunches',
    'isolation.isolatedLoginKeychainPasswordRecorded',
    'isolation.chromiumTempBoundToTrustedCache', 'isolation.sameDataDirsOnRelaunch',
    'isolation.sandboxRemoved'
  ]],
  ['a0_credential_redaction_pass_projection_invalid', [
    'redaction.status', 'redaction.secretMaterialFound', 'redaction.settingsFindingCount',
    'redaction.reportFindingCount', 'redaction.unsafeEntryCount',
    'redaction.symlinkCount', 'redaction.sourceSecretCount',
    'redaction.uniqueSecretCount', 'redaction.expectedSecretCount',
    'redaction.sourceBound', 'redaction.scannedFileCount', 'redaction.encodingCoverage', 'redaction.credentialRecorded'
  ]],
  ['a0_visual_evidence_pass_projection_invalid', [
    'visualEvidence.firstCompleted.captured', 'visualEvidence.firstCompleted.sha256',
    'visualEvidence.firstCompleted.width', 'visualEvidence.firstCompleted.height',
    'visualEvidence.recoveredAfterRelaunch.captured',
    'visualEvidence.recoveredAfterRelaunch.sha256',
    'visualEvidence.recoveredAfterRelaunch.width',
    'visualEvidence.recoveredAfterRelaunch.height',
    'visualEvidence.screenshotsUsedAsFunctionalVerdict',
    'visualEvidence.credentialScanCovered'
  ]]
] as const

const a0PublicSeamSentinels = ['firstLaunch', 'secondLaunch'].flatMap((launch) => [
  'healthOk', 'runtimeInfoOk', 'runtimeToolsOk', 'runtimeThreadListProbeOk',
  'runtimeThreadListProbeStatus', 'ordinaryCatalogNonempty', 'gitCommandAvailable',
  'skillCatalogAvailable', 'skillCount', 'ordinaryMCPAvailable',
  'ordinaryMCPServerCount', 'ordinaryMCPToolCount', 'fundsExecutionUnavailable',
  'fundsServerDiagnosticCount', 'ordinaryToolContractCount', 'ordinaryToolCatalogHash',
  'rendererTargetCount', 'electronMainPid', 'runtimeListenerPid', 'runtimeListenerCount',
  'runtimeBackendProcessCount', 'runtimeServerProcessCount',
  'desktopPrivateHistoryMigrationProcessCount', 'bundledPluginMaterializationProcessCount',
  'unknownRuntimeProcessCount', 'rendererReadFailureObserved',
  'runtimeListenerIsTaskOwnedDescendant', 'exactPackagedRuntimeExecutable'
].map((field) => `publicSeams.${launch}.${field}`))

const b1AdmissionSentinels = [
  ['b1_post_rc_projection_invalid', ['postRCChecks.0.status']],
  ['b1_external_case_pass_projection_invalid', [
    'externalCase.configured', 'externalCase.ownerIsolated', 'externalCase.synthetic',
    'externalCase.snapshotCount', 'externalCase.preserved',
    'externalCase.contractSha256', 'externalCase.provenanceSha256'
  ]],
  ['b1_managed_product_pass_projection_invalid', [
    'managedProductData.configured', 'managedProductData.bootstrapInstalled',
    'managedProductData.installationAuthoritySeeded', 'managedProductData.authorityRootCount',
    'managedProductData.bootstrapSha256',
    'managedProductData.installationAuthorityKeySha256',
    'managedProductData.volumeIdentityDigest'
  ]],
  ['b1_provider_audit_pass_projection_invalid', [
    'providerAudit.configured', 'providerAudit.challengePublished',
    'providerAudit.receiptVerified', 'providerAudit.challengeDigest',
    'providerAudit.receiptSha256', 'providerAudit.minimumObservedProviderRequestCount',
    'providerAudit.exactObservedProviderRequestCount', 'providerAudit.providerRequestCount',
    'providerAudit.scannedRequestBodyCount', 'providerAudit.completeIdentifierFindingCount',
    'providerAudit.forbiddenHostMaterialFindingCount',
    'providerAudit.providerSafeSemanticBlockCount',
    'providerAudit.providerSafeSemanticValidationCount'
  ]],
  ['b1_typed_local_display_pass_projection_invalid', [
    'typedLocalDisplay.required', 'typedLocalDisplay.status', 'typedLocalDisplay.contract',
    'typedLocalDisplay.fullModeExactValue', 'typedLocalDisplay.maskedModeLocalOnly',
    'typedLocalDisplay.acceptedSlotBindingVerified',
    'typedLocalDisplay.ordinaryRendererCompletePIIAbsent',
    'typedLocalDisplay.forbiddenGenericProviderChannelsZero',
    'typedLocalDisplay.invalidatedAfterAuthorityChange', 'typedLocalDisplay.reason',
    'typedLocalDisplay.blocksMilestoneB'
  ]],
  ['b1_direct_source_preview_pass_projection_invalid', [
    'directSourcePreview.required', 'directSourcePreview.status',
    'directSourcePreview.contract', 'directSourcePreview.fullModeExactValue',
    'directSourcePreview.maskedModeLocalOnly', 'directSourcePreview.providerRequestAbsent',
    'directSourcePreview.claimReceiptFinalGateAbsent',
    'directSourcePreview.invalidatedAfterAuthorityChange',
    'directSourcePreview.reason', 'directSourcePreview.blocksMilestoneB'
  ]],
  ['b1_provider_pass_projection_invalid', [
    'provider.configured', 'provider.normalLocalProviderSetupObserved',
    'provider.automatedTestBootstrapObserved', 'provider.family', 'provider.modelHash',
    'provider.baseUrlOriginHash', 'provider.localCredentialEvidence', 'provider.credentialAuthority', 'provider.credentialAuthorityBound'
  ]],
  ['b1_isolation_cleanup_pass_projection_invalid', [
    'isolation.trustedCacheTmpdir', 'isolation.isolatedHomeUsed',
    'isolation.isolatedUserDataUsed', 'isolation.sandboxRemoved',
    'isolation.productRunRemoved', 'isolation.productDataOutsideCache',
    'isolation.productOwnerStable', 'isolation.productVolumeRevalidated',
    'isolation.userDataRuntimeSeparated', 'isolation.externalCaseOutsideSandbox',
    'isolation.ordinaryCodeWorkspaceCleaned', 'isolation.caseBindingRestored',
    'isolation.protectedSourceAliasCleaned'
  ]],
  ['b1_workflow_pass_projection_invalid', [
    'workflow.sameThread', 'workflow.ordinaryTurnCount', 'workflow.fundsTurnCount',
    'workflow.blockedCaseTurnCount', 'workflow.compactionCount', 'workflow.exactRecovery',
    'workflow.exactSubagentRecovery', 'workflow.typedLocalDisplayRehydrated',
    'workflow.stableEntityDisplayAcrossSnapshotAndRestart',
    'workflow.completedRunnableFlow', 'workflow.typedLocalDisplay.fullModeExactValue',
    'workflow.typedLocalDisplay.maskedModeLocalOnly',
    'workflow.typedLocalDisplay.exactValueOnlyInTypedSink',
    'workflow.typedLocalDisplay.acceptedSlotBindingVerified',
    'workflow.typedLocalDisplay.forbiddenGenericProviderChannelsZero',
    'workflow.typedLocalDisplay.directSourcePreview.status',
    'workflow.typedLocalDisplay.directSourcePreview.paginationObserved',
    'workflow.typedLocalDisplay.directSourcePreview.invalidatedAfterAuthorityChange',
    'workflow.directSourcePreview.fullModeExactValue',
    'workflow.directSourcePreview.maskedModeLocalOnly',
    'workflow.directSourcePreview.paginationObserved',
    'workflow.directSourcePreview.providerRequestAbsent',
    'workflow.directSourcePreview.agentTurnAbsent',
    'workflow.directSourcePreview.mcpCallAbsent',
    'workflow.directSourcePreview.claimReceiptFinalGateAbsent',
    'workflow.directSourcePreview.invalidatedAfterAuthorityChange',
    'workflow.directSourcePreview.revokedAfterCaseChange',
    'workflow.typedLocalDisplayRevocation.retainedAfterOrdinaryTurn',
    'workflow.typedLocalDisplayRevocation.retainedSnapshotOneReResolved',
    'workflow.typedLocalDisplayRevocation.snapshotPreviewChanged',
    'workflow.typedLocalDisplayRevocation.observationKind',
    'workflow.typedLocalDisplayRevocation.slotCount', 'workflow.publicPIIScan.ok',
    'workflow.publicPIIScan.publicSurfaceCount',
    'workflow.publicPIIScan.incompletePublicSurfaceCount',
    'workflow.publicPIIScan.publicFindingCount',
    'workflow.publicPIIScan.publicInternalReferenceFindingCount',
    'workflow.publicPIIScan.logFindingCount',
    'workflow.publicPIIScan.logInternalReferenceFindingCount',
    'workflow.publicPIIScan.logUnsafeEntryCount',
    'workflow.publicPIIScan.forbiddenGenericProviderChannels.surfaceCount',
    'workflow.publicPIIScan.forbiddenGenericProviderChannels.completePIIFindingCount',
    'workflow.publicPIIScan.forbiddenGenericProviderChannels.internalReferenceFindingCount',
    'workflow.publicPIIScan.forbiddenGenericProviderChannels.incompleteSurfaceCount',
    'workflow.publicPIIScan.forbiddenGenericProviderChannels.unsafeLogEntryCount',
    'workflow.publicPIIScan.allowedTypedLocalSink.excludedFromForbiddenScan',
    'workflow.publicPIIScan.allowedTypedLocalSink.sourceExactFieldsAllowed',
    'workflow.publicPIIScan.allowedTypedLocalSink.scanBoundary'
  ]],
  ['b1_exit_pass_projection_invalid', [
    'exit.firstNormal', 'exit.finalNormal', 'exit.residualProcessCount'
  ]]
] as const

const b1PublicSeamSentinels = [
  'firstLaunch', 'snapshotOne', 'snapshotTwo', 'relaunch'
].flatMap((launch) => [
  'healthOk', 'runtimeInfoOk', 'ordinaryCatalogNonempty',
  'ordinaryToolContractCount', 'ordinaryToolCatalogHash'
].map((field) => `publicSeams.${launch}.${field}`))

function nativeFixtureBytes(marker: string): Buffer {
  const bytes = Buffer.alloc(0x220)
  bytes.writeUInt32LE(0xfeedfacf, 0)
  bytes.writeUInt32LE(0x0100000c, 4)
  bytes.writeUInt32LE(2, 12)
  bytes.writeUInt32LE(3, 16)
  bytes.writeUInt32LE(160, 20)
  bytes.writeUInt32LE(0x19, 32)
  bytes.writeUInt32LE(72, 36)
  bytes.write('__TEXT', 40, 'ascii')
  bytes.writeBigUInt64LE(0x180n, 80)
  bytes.writeUInt32LE(7, 88)
  bytes.writeUInt32LE(5, 92)
  const linkEdit = 104
  bytes.writeUInt32LE(0x19, linkEdit)
  bytes.writeUInt32LE(72, linkEdit + 4)
  bytes.write('__LINKEDIT', linkEdit + 8, 'ascii')
  bytes.writeBigUInt64LE(0x180n, linkEdit + 40)
  bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), linkEdit + 32)
  bytes.writeBigUInt64LE(BigInt(bytes.length - 0x180), linkEdit + 48)
  bytes.writeUInt32LE(1, linkEdit + 56)
  bytes.writeUInt32LE(1, linkEdit + 60)
  const codeSignature = 176
  bytes.writeUInt32LE(0x1d, codeSignature)
  bytes.writeUInt32LE(16, codeSignature + 4)
  bytes.writeUInt32LE(0x200, codeSignature + 8)
  bytes.writeUInt32LE(0x20, codeSignature + 12)
  Buffer.from(marker, 'utf8').copy(bytes, 0x1c0, 0, 48)
  bytes.writeUInt32BE(0xfade0cc0, 0x200)
  bytes.writeUInt32BE(0x20, 0x204)
  bytes.writeUInt32BE(1, 0x208)
  bytes.writeUInt32BE(0, 0x20c)
  bytes.writeUInt32BE(0x14, 0x210)
  return bytes
}

type AsarNode = {
  files?: Record<string, AsarNode>
  size?: number
  offset?: string
  unpacked?: boolean
}

type AsarFixtureValue = Buffer | string | {
  value: Buffer | string
  unpacked: true
}

function asarFixture(entries: Record<string, AsarFixtureValue>): Buffer {
  const root: Record<string, AsarNode> = {}
  const payloads: Buffer[] = []
  let offset = 0
  for (const [entry, rawValue] of Object.entries(entries)
    .sort(([left], [right]) => left.localeCompare(right))) {
    const segments = entry.split('/')
    let files = root
    for (const segment of segments.slice(0, -1)) {
      files[segment] ||= { files: {} }
      files = files[segment].files!
    }
    const unpacked = !Buffer.isBuffer(rawValue) && typeof rawValue === 'object'
      ? rawValue.unpacked === true
      : false
    const sourceValue = unpacked && !Buffer.isBuffer(rawValue) && typeof rawValue === 'object'
      ? rawValue.value
      : rawValue as Buffer | string
    const value = Buffer.isBuffer(sourceValue) ? sourceValue : Buffer.from(sourceValue)
    files[segments.at(-1)!] = unpacked
      ? { size: value.length, unpacked: true }
      : { size: value.length, offset: String(offset) }
    if (!unpacked) {
      payloads.push(value)
      offset += value.length
    }
  }
  const headerBytes = Buffer.from(JSON.stringify({ files: root }))
  const headerPickle = Buffer.alloc(8 + headerBytes.length)
  headerPickle.writeUInt32LE(headerBytes.length + 4, 0)
  headerPickle.writeUInt32LE(headerBytes.length, 4)
  headerBytes.copy(headerPickle, 8)
  const sizePickle = Buffer.alloc(8)
  sizePickle.writeUInt32LE(4, 0)
  sizePickle.writeUInt32LE(headerPickle.length, 4)
  return Buffer.concat([sizePickle, headerPickle, ...payloads])
}

function writeElectronFuseFixture(appPath: string): void {
  const frameworkRoot = join(
    appPath,
    'Contents',
    'Frameworks',
    'Electron Framework.framework'
  )
  const versionRoot = join(frameworkRoot, 'Versions', 'A')
  const frameworkBinary = join(versionRoot, 'Electron Framework')
  const snapshotRoot = join(versionRoot, 'Resources')
  mkdirSync(snapshotRoot, { recursive: true })
  const sentinel = Buffer.from('dL7pKGdnNz796PbbjQWNKmHXBZaB9tsX', 'ascii')
  const fuseWire = Buffer.from([1, 9, 0x31, 0x31, 0x30, 0x30, 0x31, 0x31, 0x30, 0x30, 0x31])
  writeFileSync(frameworkBinary, Buffer.concat([sentinel, fuseWire]))
  writeFileSync(join(snapshotRoot, 'v8_context_snapshot.arm64.bin'), 'v8-snapshot')
  symlinkSync('A', join(frameworkRoot, 'Versions', 'Current'))
  symlinkSync(
    'Versions/Current/Electron Framework',
    join(frameworkRoot, 'Electron Framework')
  )
}

let cleanSnapshotTemplate: Record<string, unknown> | null = null

function cleanWorktreeSnapshot() {
  if (!cleanSnapshotTemplate) {
    const directory = mkdtempSync(join(tmpdir(), 'analytix-clean-snapshot-'))
    temporaryDirectories.push(directory)
    writeFileSync(join(directory, 'fixture.txt'), 'clean source\n')
    runGit(directory, ['init', '-q'])
    runGit(directory, ['add', '.'])
    runGit(directory, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=analytix-test@example.invalid',
      'commit', '-q', '-m', 'clean fixture'
    ])
    cleanSnapshotTemplate = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(
      directory
    )
  }
  const { snapshotDigest: _oldDigest, ...withoutDigest } = structuredClone(
    cleanSnapshotTemplate
  ) as { snapshotDigest: string } & Record<string, unknown>
  const sourceBound = { ...withoutDigest, sourceCommit: source.head }
  return {
    ...sourceBound,
    snapshotDigest: packagedAuthorityContract.packagedWorktreeSnapshotDigest(sourceBound)
  }
}

function developmentDisposition(executableBinding: Record<string, unknown>) {
  const target = nativeComponentContract.targetContract('darwin', 'arm64')
  return {
    kind: afterPackModule.NATIVE_DISPOSITION_DEVELOPMENT,
    targetKey: target.key,
    marker: { sha256: sha256('fixture-native-marker'), byteLength: 1 },
    components: nativeComponentContract.manifest.components.map(
      (component: { id: string; binaryName: string }) => ({
        id: component.id,
        binaryName: nativeComponentContract.binaryName(component, target.platform),
        markerBinarySha256: executableBinding.preSignSha256,
        markerBinaryByteLength: executableBinding.preSignByteLength,
        payloadSha256: executableBinding.payloadSha256,
        payloadByteLength: executableBinding.payloadByteLength,
        format: executableBinding.format,
        arch: executableBinding.arch
      })
    )
  }
}

function writeBundle(a0 = a0Report(), b1 = b1Report()): string {
  const directory = mkdtempSync(join(tmpdir(), 'analytix-formal-evidence-'))
  temporaryDirectories.push(directory)
  const appOutDir = join(directory, 'package-output')
  const appPath = join(appOutDir, 'analytix.app')
  const runtimeDirectory = join(appPath, 'Contents', 'Resources', 'runtime')
  const resourcesDirectory = join(appPath, 'Contents', 'Resources')
  const fundsPluginDirectory = join(
    resourcesDirectory, 'plugins', 'analytix-fund-analysis'
  )
  mkdirSync(runtimeDirectory, { recursive: true })
  mkdirSync(join(appPath, 'Contents', 'MacOS'), { recursive: true })
  mkdirSync(join(resourcesDirectory, 'runtime-go', 'bin'), { recursive: true })
  mkdirSync(join(fundsPluginDirectory, '.codex-plugin'), { recursive: true })
  mkdirSync(join(fundsPluginDirectory, 'mcp'), { recursive: true })
  const executablePath = join(appPath, 'Contents', 'MacOS', 'analytix')
  const runtimeServerPath = join(resourcesDirectory, 'runtime-go', 'bin', 'runtime-server')
  const appAsarPath = join(resourcesDirectory, 'app.asar')
  const runtimeMetadataEntries = runtimeLicenseArtifactEntries()
  const unpackedRuntimeMetadataEntries = Object.fromEntries(
    Object.entries(runtimeMetadataEntries).map(([entry, value]) => [
      entry,
      { value, unpacked: true }
    ])
  )
  for (const [entry, value] of Object.entries(runtimeMetadataEntries)) {
    const outputPath = join(resourcesDirectory, 'app.asar.unpacked', entry)
    mkdirSync(dirname(outputPath), { recursive: true })
    writeFileSync(outputPath, value)
  }
  writeFileSync(executablePath, nativeFixtureBytes('electron-executable'))
  writeFileSync(runtimeServerPath, nativeFixtureBytes('runtime-server'))
  writeFileSync(appAsarPath, asarFixture({
    'package.json': JSON.stringify({
      name: 'analytix', version: '1.0.0', author: productPackageAuthor,
      license: 'Apache-2.0'
    }),
    ...unpackedRuntimeMetadataEntries,
    'node_modules/fixture-dependency/package.json': JSON.stringify({
      name: 'fixture-dependency', version: '1.0.0', license: 'MIT'
    }),
    'node_modules/fixture-dependency/LICENSE': 'MIT License\nfixture dependency\n'
  }))
  writeFileSync(join(resourcesDirectory, 'LICENSE'), readFileSync(join(repoRoot, 'LICENSE')))
  writeFileSync(
    join(resourcesDirectory, 'THIRD_PARTY_NOTICES.md'),
    readFileSync(join(repoRoot, 'THIRD_PARTY_NOTICES.md'))
  )
  writeFileSync(
    join(fundsPluginDirectory, '.codex-plugin', 'plugin.json'),
    JSON.stringify({
      name: 'analytix-fund-analysis', version: '1.0.0', author: productPluginAuthor,
      license: 'Apache-2.0'
    })
  )
  writeFileSync(join(fundsPluginDirectory, 'mcp', 'server.mjs'), 'export {}\n')
  writeElectronFuseFixture(appPath)
  const context = {
    appOutDir,
    electronPlatformName: 'darwin',
    arch: 'arm64',
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
  const target = nativeComponentContract.targetContract('darwin', 'arm64')
  const executableBinding = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
    executablePath,
    target
  )
  const runtimeServerBinding = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
    runtimeServerPath,
    target
  )
  const appAsarBytes = readFileSync(appAsarPath)
  const worktreeSnapshot = cleanWorktreeSnapshot()
  const authority = packagedAuthorityContract.createPackagedBuildAuthorityV2({
    worktreeSnapshot,
    targetKey: 'darwin-arm64',
    buildContext: packagedAuthorityContract.collectEffectiveBuilderContextV1(context),
    nativeDisposition: developmentDisposition(executableBinding),
    artifacts: {
      executable: executableBinding,
      appAsar: {
        sha256: sha256(appAsarBytes),
        byteLength: appAsarBytes.length
      },
      runtimeServer: runtimeServerBinding,
      fundsPlugin: packagedAuthorityContract.collectPackagedFundsPluginIdentityV2(context)
    },
    stagedPayload: packagedAuthorityContract.collectStagedPayloadClosureV1(context)
  })
  const authorityBytes = Buffer.from(JSON.stringify(authority))
  if (!packagedAuthorityContract.isPackagedBuildAuthorityV2(
    JSON.parse(authorityBytes.toString('utf8'))
  )) {
    throw new Error('fixture packaged authority did not survive canonical serialization')
  }
  writeFileSync(join(runtimeDirectory, 'analytix-packaged-build-authority.json'), authorityBytes)
  const authoritySha256 = sha256(authorityBytes)
  const executableSha256 = sha256(readFileSync(executablePath))
  const runtimeServerSha256 = sha256(readFileSync(runtimeServerPath))
  const appAsarSha256 = sha256(appAsarBytes)
  const snapshotDigest = authority.worktreeSnapshot.snapshotDigest
  if (a0.app && typeof a0.app === 'object') {
    a0.app.authoritySha256 = authoritySha256
    a0.app.authorityDigest = authority.authorityDigest
    a0.app.executableSha256 = executableSha256
    a0.app.runtimeServerSha256 = runtimeServerSha256
    a0.app.appAsarSha256 = appAsarSha256
    a0.app.worktreeSnapshotBinding.current.digest = snapshotDigest
    a0.app.worktreeSnapshotBinding.packaged.digest = snapshotDigest
    a0.app.worktreeSnapshotBinding.packaged.authorityClassification = authority.classification
  }
  a0.harness.sourceClosureSnapshotDigest = snapshotDigest
  a0.harness.packagedSourceClosureSnapshotDigest = snapshotDigest
  for (const phase of Object.values(a0.artifactRevalidation)) {
    phase.observedAuthoritySha256 = authoritySha256
    phase.observedAuthorityDigest = authority.authorityDigest
    phase.observedExecutableSha256 = executableSha256
    phase.observedRuntimeServerSha256 = runtimeServerSha256
    phase.observedAppAsarSha256 = appAsarSha256
  }
  for (const artifact of [b1.artifact, b1.artifactFinal]) {
    if (!artifact || typeof artifact !== 'object') continue
    artifact.authoritySha256 = authoritySha256
    artifact.authorityDigest = authority.authorityDigest
    artifact.executableSha256 = executableSha256
    artifact.runtimeServerSha256 = runtimeServerSha256
    artifact.appAsarSha256 = appAsarSha256
    artifact.worktreeSnapshotDigest = snapshotDigest
  }
  writeFileSync(join(directory, runtimeGoFormalEvidenceContract.reportFiles.a0), JSON.stringify(a0))
  writeFileSync(join(directory, runtimeGoFormalEvidenceContract.reportFiles.b1), JSON.stringify(b1))
  exactArtifactsByDirectory.set(
    directory,
    inspectExactArtifactLegalInventory({ artifact: appPath })
  )
  return directory
}

function evaluate(directory: string, overrides: Record<string, unknown> = {}) {
  return evaluateRuntimeGoFormalEvidence({
    repoRoot,
    bundleDirectory: directory,
    source,
    exactFormalArtifact: exactArtifactsByDirectory.get(directory) || exactArtifact(),
    ...overrides
  })
}

function runGit(cwd: string, args: string[]): string {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8', stdio: 'pipe' })
  if (result.status !== 0) {
    throw new Error(String(result.stderr || result.stdout || 'git fixture command failed'))
  }
  return String(result.stdout || '').trim()
}

function fakeReleaseGateRepo(): { directory: string; head: string } {
  const directory = mkdtempSync(join(tmpdir(), 'analytix-release-preflight-repo-'))
  temporaryDirectories.push(directory)
  for (const relativePath of new Set([
    ...runtimeGoFormalEvidenceContract.a0HarnessFiles,
    ...runtimeGoFormalEvidenceContract.b1HarnessFiles
  ])) {
    const destination = join(directory, relativePath)
    mkdirSync(resolve(destination, '..'), { recursive: true })
    writeFileSync(destination, readFileSync(join(repoRoot, relativePath)))
  }
  const tasksPath = join(
    directory,
    'openspec',
    'changes',
    'case-evidence-publication-gate',
    'tasks.md'
  )
  mkdirSync(resolve(tasksPath, '..'), { recursive: true })
  writeFileSync(tasksPath, '- [ ] 1.1 **[RC_REQUIRED]** fixture release blocker\n')
  runGit(directory, ['init', '-q'])
  runGit(directory, ['add', '.'])
  runGit(directory, [
    '-c', 'user.name=Analytix Test',
    '-c', 'user.email=analytix-test@example.invalid',
    'commit', '-q', '-m', 'fixture source freeze'
  ])
  return { directory, head: runGit(directory, ['rev-parse', 'HEAD']) }
}

function runReleaseGate(
  fixtureRepo: string,
  cacheRoot: string,
  args: string[] = []
) {
  const result = spawnSync(process.execPath, [
    join(repoRoot, 'scripts', 'runtime-go-release-gate.mjs'),
    '--json',
    ...args
  ], {
    cwd: fixtureRepo,
    env: { ...process.env, ANALYTIX_DEV_CACHE_ROOT: cacheRoot },
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return {
    ...result,
    report: JSON.parse(String(result.stdout || '{}'))
  }
}

describe('runtime-go formal evidence admission', () => {
  test('preflights current reports without claiming artifact or formal execution', () => {
    const directory = writeBundle()
    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: directory,
      source
    })

    expect(result).toMatchObject({
      phase: 'pre_lock_read_only',
      provided: true,
      status: 'passed',
      passed: true,
      formalExecution: 'not_executed',
      receiptCreated: false,
      exactArtifactBinding: 'not_evaluated',
      statuses: {
        electron: 'not_executed',
        package: 'not_executed',
        a0: 'not_executed',
        b1: 'not_executed'
      },
      problems: []
    })
  })

  test('accepts only the current A0 and B1 reports bound to one exact artifact', () => {
    const directory = writeBundle()
    const recordedArtifact = exactArtifactsByDirectory.get(directory)!
    const result = evaluate(directory)

    expect(result).toMatchObject({
      contract: 'analytix.runtime-go-formal-evidence/v1',
      provided: true,
      accepted: true,
      statuses: { electron: 'passed', package: 'passed', a0: 'passed', b1: 'passed' },
      problems: []
    })
    expect(result.reports?.a0.lane).toBe('ordinary-a0')
    expect(result.reports?.b1.lane).toBe('typed-local-b1')
    expect(result.artifactBinding).toMatchObject({
      exactArtifactByteLength: recordedArtifact.artifact.byteLength,
      appAsarSha256: recordedArtifact.artifact.components[0].sha256,
      appAsarByteLength: recordedArtifact.artifact.components[0].byteLength,
      exactArtifactRevalidated: true,
      authorityClassification: 'development_clean_non_publishable',
      fundsPluginArtifactBound: true
    })
  })

  test('keeps absent evidence not_executed and never projects a pass', () => {
    expect(evaluateRuntimeGoFormalEvidence({ repoRoot, source })).toEqual({
      contract: 'analytix.runtime-go-formal-evidence/v1',
      provided: false,
      accepted: false,
      statuses: {
        electron: 'not_executed',
        package: 'not_executed',
        a0: 'not_executed',
        b1: 'not_executed'
      },
      reports: null,
      artifactBinding: null,
      problems: []
    })
  })

  test.each(['failed', 'skipped', 'live_blocked'] as const)(
    'rejects an A0 %s required check without substituting B1',
    (status) => {
      const a0 = a0Report()
      const hostileCheck = a0.checks[0] as { status: string }
      hostileCheck.status = status
      const result = evaluate(writeBundle(a0, b1Report()))

      expect(result.statuses).toEqual({
        electron: 'failed', package: 'failed', a0: 'failed', b1: 'passed'
      })
      expect(result.problems).toContain('a0_check_schema_status_or_identity_invalid')
    }
  )

  test.each([
    ['execution blocker', (report: ReturnType<typeof a0Report>) => {
      report.executionBlocker = 'provider_reasoning_missing'
    }],
    ['execution blocker classification', (report: ReturnType<typeof a0Report>) => {
      Object.assign(report, { executionBlockerClassification: 'product_or_harness_failure' })
    }],
    ['blocker ids', (report: ReturnType<typeof a0Report>) => {
      report.blockers = ['composer-workflow-submit']
    }],
    ['missing external inputs', (report: ReturnType<typeof a0Report>) => {
      report.missingExternalInputs = [{ id: 'configured-network-provider' }]
    }]
  ])('rejects contradictory A0 PASS %s projection', (_label, mutate) => {
    const a0 = a0Report()
    mutate(a0)
    const result = evaluate(writeBundle(a0, b1Report()))

    expect(result.statuses).toEqual({
      electron: 'failed', package: 'failed', a0: 'failed', b1: 'passed'
    })
    expect(result.problems).toContain('a0_pass_blocker_projection_invalid')
  })

  test.each([
    ['runtime blocker', (report: ReturnType<typeof b1Report>) => {
      report.runtimeBlocker = 'provider_reasoning_missing'
    }],
    ['runtime failure', (report: ReturnType<typeof b1Report>) => {
      Object.assign(report, {
        runtimeFailure: { reasonCode: 'provider_reasoning_missing' }
      })
    }]
  ])('rejects contradictory B1 PASS %s projection', (_label, mutate) => {
    const b1 = b1Report()
    mutate(b1)
    const result = evaluate(writeBundle(a0Report(), b1))

    expect(result.statuses).toEqual({
      electron: 'failed', package: 'failed', a0: 'passed', b1: 'failed'
    })
    expect(result.problems).toContain('b1_pass_blocker_projection_invalid')
  })

  test('accepts reporter-compatible A0 snapshots with earlier false or smaller progress', () => {
    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writePreflightBundle(a0Report(), b1Report()),
      source
    })

    expect(result).toMatchObject({ passed: true, problems: [] })
  })

  test.each([
    ['tampered digest', (report: ReturnType<typeof a0Report>) => {
      a0SnapshotByStage(report, 'long-context-continuation').snapshotDigest = '0'.repeat(64)
    }],
    ['snapshots not an array', (report: ReturnType<typeof a0Report>) => {
      const mutableReport = report as { stageSnapshots: unknown }
      mutableReport.stageSnapshots = {}
    }],
    ['empty snapshots', (report: ReturnType<typeof a0Report>) => {
      report.stageSnapshots = []
    }],
    ['missing one stage', (report: ReturnType<typeof a0Report>) => {
      report.stageSnapshots = report.stageSnapshots.filter((snapshot) =>
        snapshot.stage !== 'normal-first-quit'
      )
    }],
    ['unknown snapshot property', (report: ReturnType<typeof a0Report>) => {
      Object.assign(a0SnapshotByStage(report, 'long-context-continuation'), {
        unexpected: true
      })
    }],
    ['unknown stage', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.stage = 'unknown-completed-stage'
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['extra stage', (report: ReturnType<typeof a0Report>) => {
      report.stageSnapshots.push(a0StageSnapshot(
        'unexpected-pass-stage',
        ['normal-first-quit'],
        {}
      ))
    }],
    ['duplicate stage', (report: ReturnType<typeof a0Report>) => {
      report.stageSnapshots.push(structuredClone(
        a0SnapshotByStage(report, 'long-context-continuation')
      ))
    }],
    ['failure-only stage', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.stage = 'ordinary-agent-workflow-failure'
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['wrong but passed check id', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.checkIds = ['normal-first-quit']
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['missing expected check id', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'composer-compaction-submit')
      snapshot.checkIds = ['composer-compaction-submit']
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['reordered expected check ids', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'ordinary-agent-workflow')
      snapshot.checkIds.reverse()
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['duplicate check id', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.checkIds.push('long-context-continuation')
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['completed count would raise final progress', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.completedFields.compactionCount = 2
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['unknown check id', (report: ReturnType<typeof a0Report>) => {
      const snapshot = a0SnapshotByStage(report, 'long-context-continuation')
      snapshot.checkIds = ['unknown-check']
      refreshA0StageSnapshotDigest(snapshot)
    }],
    ['non-passed check id', (report: ReturnType<typeof a0Report>) => {
      const check = report.checks.find((item) => item.id === 'long-context-continuation')!
      const mutableCheck = check as { status: string }
      mutableCheck.status = 'failed'
    }]
  ] as const)('rejects A0 PASS stage snapshot inconsistency: %s', (_label, mutate) => {
    const a0 = a0Report()
    mutate(a0)
    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writePreflightBundle(a0, b1Report()),
      source
    })

    expect(result.passed).toBe(false)
    expect(result.problems).toContain('a0_stage_snapshot_pass_projection_invalid')
  })

  test.each([
    ['final funds count above six', (report: ReturnType<typeof b1Report>) => {
      report.workflow.fundsTurnCount = 7
    }, 'b1_workflow_pass_projection_invalid'],
    ['semantic block and validation counts differ', (report: ReturnType<typeof b1Report>) => {
      report.providerAudit.providerSafeSemanticValidationCount = 5
    }, 'b1_provider_audit_pass_projection_invalid'],
    ['semantic count is below funds turns', (report: ReturnType<typeof b1Report>) => {
      report.providerAudit.providerSafeSemanticBlockCount = 5
      report.providerAudit.providerSafeSemanticValidationCount = 5
    }, 'b1_provider_audit_pass_projection_invalid']
  ] as const)('rejects B1 reporter count contradiction: %s', (_label, mutate, problem) => {
    const b1 = b1Report()
    mutate(b1)
    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writePreflightBundle(a0Report(), b1),
      source
    })

    expect(result.passed).toBe(false)
    expect(result.problems).toContain(problem)
  })

  test.each([
    ...a0AdmissionSentinels.flatMap(([problem, paths]) =>
      paths.map((path) => [path, problem] as const)),
    ...a0PublicSeamSentinels.map((path) => [
      path, 'a0_public_seam_pass_projection_invalid'
    ] as const)
  ])('rejects an A0 nested PASS contradiction at %s', (path, problem) => {
    const a0 = a0Report()
    mutateAdmissionSentinel(a0, path)

    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writePreflightBundle(a0, b1Report()),
      source
    })

    expect(result.passed).toBe(false)
    expect(result.problems).toContain(problem)
  })

  test.each([
    ...b1AdmissionSentinels.flatMap(([problem, paths]) =>
      paths.map((path) => [path, problem] as const)),
    ...b1PublicSeamSentinels.map((path) => [
      path, 'b1_public_seam_pass_projection_invalid'
    ] as const),
    ['publicSeams.exactPackagedRuntimeExecutable',
      'b1_public_seam_pass_projection_invalid'] as const
  ])('rejects a B1 nested PASS contradiction at %s', (path, problem) => {
    const b1 = b1Report()
    mutateAdmissionSentinel(b1, path)

    const result = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writePreflightBundle(a0Report(), b1),
      source
    })

    expect(result.passed).toBe(false)
    expect(result.problems).toContain(problem)
  })

  test('keeps commercial publication authorization outside A0 engineering admission', () => {
    const a0 = a0Report()
    Object.assign(a0, {
      releasePublicationAuthority: {
        requested: false,
        ok: false,
        blocker: 'formal_release_publication_authority_unverified'
      },
      commercialRelease: {
        requiredForMilestoneA: false,
        requiredForCommercialPublication: true,
        status: 'unverified',
        passed: false
      }
    })

    expect(evaluate(writeBundle(a0, b1Report()))).toMatchObject({
      accepted: true,
      statuses: { a0: 'passed', b1: 'passed' }
    })
  })

  test('accepts older same-source evidence without inventing a wall-clock expiry', () => {
    const a0 = a0Report()
    const b1 = b1Report()
    a0.generatedAt = '2020-01-01T00:00:00.000Z'
    b1.generatedAt = '2020-01-01T00:00:00.000Z'

    expect(evaluate(writeBundle(a0, b1))).toMatchObject({
      accepted: true,
      problems: []
    })
  })

  test('rejects a partial or unverified B1 lane without substituting A0', () => {
    const b1 = b1Report()
    b1.phaseLanes.b1DeterministicFunds.status = 'UNVERIFIED'
    b1.phaseLanes.b1DeterministicFunds.executed = false
    b1.phaseLanes.b1DeterministicFunds.not_executed = true
    const hostileCheck = b1.checks.find(
      (item) => item.id === 'native-snapshot-one-staging'
    ) as { status: string }
    hostileCheck.status = 'UNVERIFIED'
    const result = evaluate(writeBundle(a0Report(), b1))

    expect(result.statuses).toEqual({
      electron: 'failed', package: 'failed', a0: 'passed', b1: 'failed'
    })
    expect(result.problems).toContain('b1_phase_b1DeterministicFunds_invalid')
  })

  test.each([
    ['old schema', (a0: ReturnType<typeof a0Report>) => { a0.schemaVersion = 0 }],
    ['unknown property', (a0: ReturnType<typeof a0Report>) => {
      Object.assign(a0, { arbitraryAuthorization: true })
    }],
    ['tampered harness digest', (a0: ReturnType<typeof a0Report>) => {
      a0.harness.contractManifestSha256 = 'f'.repeat(64)
    }]
  ])('rejects %s', (_label, mutate) => {
    const a0 = a0Report()
    mutate(a0)
    const result = evaluate(writeBundle(a0, b1Report()))
    expect(result.accepted).toBe(false)
    expect(result.statuses.a0).toBe('failed')
  })

  test('rejects empty, cross-source, cross-artifact, and old-artifact identities', () => {
    const emptySource = evaluate(writeBundle(), { source: { head: '', tree: '' } })
    expect(emptySource.problems).toEqual(['formal_evidence_current_source_invalid'])

    const crossSource = b1Report()
    crossSource.sourceCommit = '3'.repeat(40)
    crossSource.harnessCommit = '3'.repeat(40)
    const crossSourceResult = evaluate(writeBundle(a0Report(), crossSource))
    expect(crossSourceResult.problems).toContain('formal_evidence_cross_lane_source_mismatch')

    const crossArtifactDirectory = writeBundle()
    const crossArtifactPath = join(
      crossArtifactDirectory,
      runtimeGoFormalEvidenceContract.reportFiles.b1
    )
    const crossArtifact = JSON.parse(readFileSync(crossArtifactPath, 'utf8'))
    crossArtifact.artifact.appAsarSha256 = sha256('other-app-asar')
    crossArtifact.artifactFinal.appAsarSha256 = crossArtifact.artifact.appAsarSha256
    writeFileSync(crossArtifactPath, JSON.stringify(crossArtifact))
    const crossArtifactResult = evaluate(crossArtifactDirectory)
    expect(crossArtifactResult.problems).toContain(
      'formal_evidence_cross_lane_artifact_identity_mismatch'
    )

    const oldArtifactDirectory = writeBundle()
    const oldArtifact = structuredClone(exactArtifactsByDirectory.get(oldArtifactDirectory)!)
    oldArtifact.artifact.components[0].sha256 = sha256('old-app-asar')
    const oldArtifactResult = evaluate(oldArtifactDirectory, { exactFormalArtifact: oldArtifact })
    expect(oldArtifactResult.problems).toContain('formal_evidence_exact_artifact_mismatch')

    const emptyArtifactDirectory = writeBundle()
    const emptyArtifact = structuredClone(exactArtifactsByDirectory.get(emptyArtifactDirectory)!)
    emptyArtifact.artifact.byteLength = 0
    const emptyArtifactResult = evaluate(emptyArtifactDirectory, { exactFormalArtifact: emptyArtifact })
    expect(emptyArtifactResult.problems).toContain('exact_artifact_identity_or_size_invalid')
  })

  test('rejects duplicate reports and incomplete bundles', () => {
    const duplicateDirectory = writeBundle()
    const a0Bytes = readFileSync(join(
      duplicateDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0
    ))
    writeFileSync(
      join(duplicateDirectory, runtimeGoFormalEvidenceContract.reportFiles.b1),
      a0Bytes
    )
    const duplicate = evaluate(duplicateDirectory)
    expect(duplicate.problems).toContain('formal_evidence_duplicate_report')
    expect(duplicate.accepted).toBe(false)

    const partialDirectory = mkdtempSync(join(tmpdir(), 'analytix-formal-evidence-partial-'))
    temporaryDirectories.push(partialDirectory)
    writeFileSync(
      join(partialDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0),
      JSON.stringify(a0Report())
    )
    const partial = evaluate(partialDirectory)
    expect(partial.statuses).toEqual({
      electron: 'failed', package: 'failed', a0: 'failed', b1: 'failed'
    })
  })

  test('rejects tampered authority and duplicate-key JSON bytes', () => {
    const authorityDirectory = writeBundle()
    const authorityPath = join(
      authorityDirectory,
      'package-output',
      'analytix.app',
      'Contents',
      'Resources',
      'runtime',
      'analytix-packaged-build-authority.json'
    )
    const authority = JSON.parse(readFileSync(authorityPath, 'utf8'))
    authority.authorityDigest = 'f'.repeat(64)
    writeFileSync(authorityPath, JSON.stringify(authority))
    const tamperedAuthority = evaluate(authorityDirectory)
    expect(tamperedAuthority.problems).toContain('packaged_build_authority_schema_invalid')
    expect(tamperedAuthority.accepted).toBe(false)

    const duplicateKeyDirectory = writeBundle()
    writeFileSync(
      join(duplicateKeyDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0),
      '{"schemaVersion":1,"schemaVersion":1}'
    )
    expect(evaluate(duplicateKeyDirectory).problems).toEqual([
      'formal_evidence_bundle_unreadable'
    ])
  })

  test.each([
    ['missing artifacts', (authority: Record<string, any>) => {
      delete authority.artifacts
    }],
    ['missing appAsar', (authority: Record<string, any>) => {
      delete authority.artifacts.appAsar
    }],
    ['wrong artifacts type', (authority: Record<string, any>) => {
      authority.artifacts = 'invalid'
    }],
    ['unknown root property', (authority: Record<string, any>) => {
      authority.unexpectedAuthorization = true
    }]
  ] as const)('returns a structured failure for malformed authority: %s', (_label, mutate) => {
    const directory = writeBundle()
    const authorityPath = join(
      exactArtifactsByDirectory.get(directory)!.artifact.path,
      'Contents',
      'Resources',
      'runtime',
      'analytix-packaged-build-authority.json'
    )
    const authority = JSON.parse(readFileSync(authorityPath, 'utf8'))
    mutate(authority)
    writeFileSync(authorityPath, JSON.stringify(authority))

    const result = evaluate(directory)

    expect(result).toMatchObject({
      provided: true,
      accepted: false,
      statuses: {
        electron: 'failed', package: 'failed', a0: 'failed', b1: 'failed'
      }
    })
    expect(result.problems).toContain('packaged_build_authority_schema_invalid')
    expect(result.problems).toContain('packaged_build_authority_artifact_verification_failed')
  })

  test('rejects package bytes that drift after both reports completed', () => {
    const directory = writeBundle()
    writeFileSync(join(
      directory,
      'package-output',
      'analytix.app',
      'Contents',
      'Resources',
      'runtime-go',
      'bin',
      'runtime-server'
    ), 'runtime-server-tampered')
    const result = evaluate(directory)

    expect(result.accepted).toBe(false)
    expect(result.problems).toContain('exact_artifact_changed_after_legal_audit')
    expect(result.problems).toContain(
      'packaged_build_authority_artifact_verification_failed'
    )
    expect(result.problems).toContain('formal_evidence_packaged_authority_mismatch')
  })

  test.each([
    ['LICENSE', 'tampered product license\n'],
    ['THIRD_PARTY_NOTICES.md', 'tampered third-party notice\n'],
    ['unexpected-packaged-root.txt', 'unexpected packaged root bytes\n']
  ])('re-reads and rejects %s drift after the exact legal audit', (fileName, bytes) => {
    const directory = writeBundle()
    const appPath = exactArtifactsByDirectory.get(directory)!.artifact.path
    writeFileSync(join(appPath, 'Contents', 'Resources', fileName), bytes)

    const result = evaluate(directory)

    expect(result.accepted).toBe(false)
    expect(result.problems).toContain('exact_artifact_changed_after_legal_audit')
  })

  test.each([
    [
      'runtime package',
      'packages/runtime/package.json',
      JSON.stringify({ name: 'analytix-runtime', version: '0.1.0', license: 'MIT' }),
      'EXACT_ARTIFACT_RUNTIME_LICENSE_METADATA_MISSING_OR_MISMATCHED'
    ],
    [
      'runtime lockfile',
      'packages/runtime/package-lock.json',
      JSON.stringify({
        packages: {
          '': { name: 'analytix-runtime', version: '0.1.0', license: 'MIT' }
        }
      }),
      'EXACT_ARTIFACT_RUNTIME_LOCK_LICENSE_METADATA_MISSING_OR_MISMATCHED'
    ]
  ] as const)('rejects exact %s license metadata drift', (_label, entry, bytes, code) => {
    const result = inspectExactArtifactLegalInventory({
      artifact: {
        label: 'runtime-license-hostile-fixture',
        entries: {
          'package.json': JSON.stringify({
            name: 'analytix', version: '1.0.0', author: productPackageAuthor,
            license: 'Apache-2.0'
          }),
          ...runtimeLicenseArtifactEntries(),
          [entry]: bytes,
          LICENSE: readFileSync(join(repoRoot, 'LICENSE')),
          'THIRD_PARTY_NOTICES.md': readFileSync(
            join(repoRoot, 'THIRD_PARTY_NOTICES.md')
          ),
          'node_modules/fixture-dependency/package.json': JSON.stringify({
            name: 'fixture-dependency', version: '1.0.0', license: 'MIT'
          }),
          'node_modules/fixture-dependency/LICENSE': 'MIT License\nfixture dependency\n'
        }
      }
    })

    expect(result).toMatchObject({ provided: true, passed: false, status: 'blocked' })
    expect(result.mandatoryBlockers).toEqual(expect.arrayContaining([
      expect.objectContaining({ code })
    ]))
  })

  test.each([
    ['missing package', (entries: Record<string, string>) => {
      delete entries['node_modules/openclaw/package.json']
    }, 'EXACT_ARTIFACT_OPENCLAW_PACKAGE_MISSING_OR_MISMATCHED'],
    ['missing maintainer', (entries: Record<string, string>) => {
      const value = JSON.parse(entries['node_modules/openclaw/package.json'])
      delete value.author
      delete value.maintainers
      entries['node_modules/openclaw/package.json'] = JSON.stringify(value)
    }, 'EXACT_ARTIFACT_OPENCLAW_MAINTAINER_IDENTITY_MISSING_OR_MISMATCHED'],
    ['old maintainer', (entries: Record<string, string>) => {
      const value = JSON.parse(entries['node_modules/openclaw/package.json'])
      value.author = 'Analytix Contributors'
      value.maintainers = ['Analytix Contributors']
      entries['node_modules/openclaw/package.json'] = JSON.stringify(value)
    }, 'EXACT_ARTIFACT_OPENCLAW_MAINTAINER_IDENTITY_MISSING_OR_MISMATCHED'],
    ['variant maintainer', (entries: Record<string, string>) => {
      const value = JSON.parse(entries['node_modules/openclaw/package.json'])
      value.author = 'Guoqin He'
      value.maintainers = ['Guoqin He']
      entries['node_modules/openclaw/package.json'] = JSON.stringify(value)
    }, 'EXACT_ARTIFACT_OPENCLAW_MAINTAINER_IDENTITY_MISSING_OR_MISMATCHED'],
    ['wrong upstream provenance', (entries: Record<string, string>) => {
      const value = JSON.parse(entries['node_modules/openclaw/package.json'])
      value.contributors = ['Analytix Contributors']
      value.analytixProvenance.upstreamTagObject = '0'.repeat(40)
      entries['node_modules/openclaw/package.json'] = JSON.stringify(value)
    }, 'EXACT_ARTIFACT_OPENCLAW_UPSTREAM_PROVENANCE_MISSING_OR_MISMATCHED'],
    ['missing MIT metadata', (entries: Record<string, string>) => {
      const value = JSON.parse(entries['node_modules/openclaw/package.json'])
      delete value.license
      entries['node_modules/openclaw/package.json'] = JSON.stringify(value)
    }, 'EXACT_ARTIFACT_OPENCLAW_LICENSE_METADATA_MISSING_OR_MISMATCHED'],
    ['missing nested license', (entries: Record<string, string>) => {
      delete entries['node_modules/openclaw/LICENSE']
    }, 'EXACT_ARTIFACT_OPENCLAW_LICENSE_MISSING_OR_DRIFTED'],
    ['variant nested license', (entries: Record<string, string>) => {
      entries['node_modules/openclaw/LICENSE'] = 'MIT License\nvariant\n'
    }, 'EXACT_ARTIFACT_OPENCLAW_LICENSE_MISSING_OR_DRIFTED']
  ] as const)('rejects exact OpenClaw shim %s', (_label, mutate, code) => {
    const entries = runtimeLicenseArtifactEntries()
    mutate(entries)
    const result = inspectExactArtifactLegalInventory({
      artifact: {
        label: 'openclaw-legal-hostile-fixture',
        entries: {
          'package.json': JSON.stringify({
            name: 'analytix', version: '1.0.0', author: productPackageAuthor,
            license: 'Apache-2.0'
          }),
          ...entries,
          LICENSE: readFileSync(join(repoRoot, 'LICENSE')),
          'THIRD_PARTY_NOTICES.md': readFileSync(
            join(repoRoot, 'THIRD_PARTY_NOTICES.md')
          ),
          'node_modules/fixture-dependency/package.json': JSON.stringify({
            name: 'fixture-dependency', version: '1.0.0', license: 'MIT'
          }),
          'node_modules/fixture-dependency/LICENSE': 'MIT License\nfixture dependency\n'
        }
      }
    })

    expect(result).toMatchObject({
      provided: true,
      passed: false,
      status: 'blocked',
      engineeringAdmission: false
    })
    expect(result.mandatoryBlockers).toEqual(expect.arrayContaining([
      expect.objectContaining({ code })
    ]))
  })

  test('keeps stable standalone app.asar and unpacked entries readable', () => {
    const directory = mkdtempSync(join(tmpdir(), 'analytix-standalone-asar-'))
    temporaryDirectories.push(directory)
    const appAsarPath = join(directory, 'app.asar')
    const unpackedRoot = `${appAsarPath}.unpacked`
    const dependencyPackage = JSON.stringify({
      name: 'fixture-dependency', version: '1.0.0', license: 'MIT'
    })
    mkdirSync(join(unpackedRoot, 'node_modules', 'fixture-dependency'), {
      recursive: true
    })
    writeFileSync(
      join(unpackedRoot, 'node_modules', 'fixture-dependency', 'package.json'),
      dependencyPackage
    )
    writeFileSync(
      join(unpackedRoot, 'node_modules', 'fixture-dependency', 'LICENSE'),
      'MIT License\nfixture dependency\n'
    )
    const runtimeMetadataEntries = runtimeLicenseArtifactEntries()
    for (const [entry, value] of Object.entries(runtimeMetadataEntries)) {
      const outputPath = join(unpackedRoot, entry)
      mkdirSync(dirname(outputPath), { recursive: true })
      writeFileSync(outputPath, value)
    }
    writeFileSync(appAsarPath, asarFixture({
      'package.json': JSON.stringify({
        name: 'analytix', version: '1.0.0', author: productPackageAuthor,
        license: 'Apache-2.0'
      }),
      ...Object.fromEntries(
        Object.entries(runtimeMetadataEntries).map(([entry, value]) => [
          entry,
          { value, unpacked: true }
        ])
      ),
      LICENSE: readFileSync(join(repoRoot, 'LICENSE')),
      'THIRD_PARTY_NOTICES.md': readFileSync(join(repoRoot, 'THIRD_PARTY_NOTICES.md')),
      'node_modules/packed-dependency/package.json': JSON.stringify({
        name: 'packed-dependency', version: '1.0.0', license: 'MIT'
      }),
      'node_modules/packed-dependency/LICENSE': 'MIT License\npacked dependency\n',
      'node_modules/fixture-dependency/package.json': {
        value: dependencyPackage,
        unpacked: true
      },
      'node_modules/fixture-dependency/LICENSE': {
        value: 'MIT License\nfixture dependency\n',
        unpacked: true
      }
    }))

    const direct = inspectExactArtifactLegalInventory({ artifact: appAsarPath })
    expect(direct).toMatchObject({ provided: true, passed: true, status: 'passed' })

    const unpackedReader = artifactLegalObligationsTestInternals.createAsarReader(
      appAsarPath,
      unpackedRoot
    )
    expect(unpackedReader.read('node_modules/fixture-dependency/package.json'))
      .toEqual(Buffer.from(dependencyPackage))
    expect(inspectExactArtifactLegalInventory({ artifact: unpackedReader }))
      .toMatchObject({ provided: true, passed: true, status: 'passed' })
  })

  test('rejects an unsafe standalone app.asar unpacked root', () => {
    const directory = mkdtempSync(join(tmpdir(), 'analytix-unsafe-unpacked-asar-'))
    temporaryDirectories.push(directory)
    const appAsarPath = join(directory, 'app.asar')
    const unpackedTarget = join(directory, 'unpacked-target')
    mkdirSync(unpackedTarget)
    symlinkSync(unpackedTarget, `${appAsarPath}.unpacked`, 'dir')
    writeFileSync(appAsarPath, asarFixture({
      'package.json': JSON.stringify({
        name: 'analytix', version: '1.0.0', author: productPackageAuthor,
        license: 'Apache-2.0'
      }),
      ...runtimeLicenseArtifactEntries(),
      LICENSE: readFileSync(join(repoRoot, 'LICENSE')),
      'THIRD_PARTY_NOTICES.md': readFileSync(join(repoRoot, 'THIRD_PARTY_NOTICES.md')),
      'node_modules/fixture-dependency/package.json': JSON.stringify({
        name: 'fixture-dependency', version: '1.0.0', license: 'MIT'
      }),
      'node_modules/fixture-dependency/LICENSE': 'MIT License\nfixture dependency\n'
    }))

    const result = inspectExactArtifactLegalInventory({ artifact: appAsarPath })

    expect(result).toMatchObject({ provided: false, passed: false, status: 'blocked' })
    expect(result.reason).toContain('exact_artifact_unpack_root_unsafe')
    expect(result.mandatoryBlockers).toEqual([
      expect.objectContaining({ code: 'EXACT_ARTIFACT_INPUT_UNREADABLE' })
    ])
  })

  test.each(['atomic replacement', 'in-place rewrite'] as const)(
    'rejects standalone app.asar %s after its stable read',
    (mutation) => {
      const directory = mkdtempSync(join(tmpdir(), 'analytix-standalone-asar-drift-'))
      temporaryDirectories.push(directory)
      const appAsarPath = join(directory, 'app.asar')
      const bytes = asarFixture({
        'package.json': JSON.stringify({
          name: 'analytix', version: '1.0.0', author: productPackageAuthor,
          license: 'Apache-2.0'
        }),
        ...runtimeLicenseArtifactEntries(),
        LICENSE: readFileSync(join(repoRoot, 'LICENSE')),
        'THIRD_PARTY_NOTICES.md': readFileSync(
          join(repoRoot, 'THIRD_PARTY_NOTICES.md')
        )
      })
      writeFileSync(appAsarPath, bytes)
      const replacement = join(directory, 'replacement.asar')
      if (mutation === 'atomic replacement') writeFileSync(replacement, bytes)

      const reader = artifactLegalObligationsTestInternals.createAsarReader(
        appAsarPath,
        null,
        {
          afterStableRead: () => {
            if (mutation === 'atomic replacement') {
              renameSync(replacement, appAsarPath)
            } else {
              writeFileSync(appAsarPath, Buffer.concat([bytes, Buffer.from('drift')]))
            }
          }
        }
      )
      const result = inspectExactArtifactLegalInventory({ artifact: reader })

      expect(result).toMatchObject({ provided: false, passed: false, status: 'blocked' })
      expect(result.reason).toContain('exact_artifact_regular_file_path_changed_after_read')
    }
  )

  test('rejects a hard-linked app.asar at the actual formal ingestion seam', () => {
    const directory = writeBundle()
    const appPath = exactArtifactsByDirectory.get(directory)!.artifact.path
    const appAsarPath = join(appPath, 'Contents', 'Resources', 'app.asar')
    linkSync(appAsarPath, join(directory, 'app-asar-hardlink'))

    const result = evaluate(directory)

    expect(result.accepted).toBe(false)
    expect(result.problems).toContain('exact_artifact_changed_after_legal_audit')
    expect(result.problems).toContain('packaged_build_authority_artifact_verification_failed')
  })

  test('legal owner rejects hard-linked packaged-root files', () => {
    const directory = writeBundle()
    const appPath = exactArtifactsByDirectory.get(directory)!.artifact.path
    const licensePath = join(appPath, 'Contents', 'Resources', 'LICENSE')
    linkSync(licensePath, join(appPath, 'Contents', 'Resources', 'LICENSE.copy'))

    const result = inspectExactArtifactLegalInventory({ artifact: appPath })

    expect(result).toMatchObject({ provided: false, passed: false, status: 'blocked' })
    expect(result.reason).toContain('exact_artifact_directory_file_hardlink_rejected')
  })

  test('legal owner detects deterministic file and directory inspection interleaves', () => {
    const fileRoot = mkdtempSync(join(tmpdir(), 'analytix-legal-file-interleave-'))
    const directoryRoot = mkdtempSync(join(tmpdir(), 'analytix-legal-directory-interleave-'))
    const hardLinkRoot = mkdtempSync(join(tmpdir(), 'analytix-legal-hardlink-interleave-'))
    const symlinkRoot = mkdtempSync(join(tmpdir(), 'analytix-legal-symlink-interleave-'))
    temporaryDirectories.push(fileRoot, directoryRoot, hardLinkRoot, symlinkRoot)
    const filePath = join(fileRoot, 'entry.txt')
    const hardLinkPath = join(hardLinkRoot, 'entry.txt')
    writeFileSync(filePath, 'before')
    writeFileSync(hardLinkPath, 'stable')
    writeFileSync(join(directoryRoot, 'entry.txt'), 'stable')
    writeFileSync(join(symlinkRoot, 'target-a.txt'), 'a')
    writeFileSync(join(symlinkRoot, 'target-b.txt'), 'b')
    symlinkSync('target-a.txt', join(symlinkRoot, 'current'))

    expect(() => artifactLegalObligationsTestInternals.readStableSingleLinkRegularFile(
      filePath,
      { afterRead: () => writeFileSync(filePath, 'after') }
    )).toThrow(/exact_artifact_regular_file_changed_during_read/)
    expect(() => artifactLegalObligationsTestInternals.readStableSingleLinkRegularFile(
      hardLinkPath,
      {
        afterRead: () => {
          const transient = join(hardLinkRoot, 'transient-hardlink')
          linkSync(hardLinkPath, transient)
          rmSync(transient)
        }
      }
    )).toThrow(/exact_artifact_regular_file_changed_during_read/)
    expect(() => artifactLegalObligationsTestInternals.createDirectoryReader(
      directoryRoot,
      { afterInitialInventory: () => writeFileSync(join(directoryRoot, 'late.txt'), 'late') }
    )).toThrow(/exact_artifact_directory_(?:changed_during_inventory|inventory_changed_during_inspection)/)
    expect(() => artifactLegalObligationsTestInternals.createDirectoryReader(
      symlinkRoot,
      {
        afterInitialInventory: () => {
          rmSync(join(symlinkRoot, 'current'))
          symlinkSync('target-b.txt', join(symlinkRoot, 'current'))
        }
      }
    )).toThrow(/exact_artifact_directory_inventory_changed_during_inspection/)
  })

  test('rejects dirty report and packaged-authority worktree projections', () => {
    const a0 = a0Report()
    a0.app.worktreeSnapshotBinding.current.classification = 'dirty'
    a0.app.worktreeSnapshotBinding.packaged.classification = 'dirty'
    a0.app.worktreeSnapshotBinding.packaged.authorityClassification =
      'development_dirty_non_publishable'
    const reportDirty = preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: writeBundle(a0, b1Report()),
      source
    })
    expect(reportDirty.passed).toBe(false)
    expect(reportDirty.problems).toContain('a0_packaged_artifact_binding_invalid')

    const authorityDirectory = writeBundle()
    const authorityPath = join(
      exactArtifactsByDirectory.get(authorityDirectory)!.artifact.path,
      'Contents',
      'Resources',
      'runtime',
      'analytix-packaged-build-authority.json'
    )
    const authority = JSON.parse(readFileSync(authorityPath, 'utf8'))
    const { snapshotDigest: _snapshotDigest, ...snapshotWithoutDigest } =
      authority.worktreeSnapshot
    const dirtySnapshotWithoutDigest = {
      ...snapshotWithoutDigest,
      state: 'dirty',
      dirty: true,
      stagedPatch: {
        ...snapshotWithoutDigest.stagedPatch,
        count: 1,
        byteLength: 1,
        sha256: sha256('dirty staged patch')
      }
    }
    authority.worktreeSnapshot = {
      ...dirtySnapshotWithoutDigest,
      snapshotDigest: packagedAuthorityContract.packagedWorktreeSnapshotDigest(
        dirtySnapshotWithoutDigest
      )
    }
    authority.classification = 'development_dirty_non_publishable'
    const { authorityDigest: _authorityDigest, ...authorityWithoutDigest } = authority
    authority.authorityDigest = packagedAuthorityContract.packagedBuildAuthorityDigest(
      authorityWithoutDigest
    )
    writeFileSync(authorityPath, JSON.stringify(authority))

    const authorityDirty = evaluate(authorityDirectory)
    expect(authorityDirty.accepted).toBe(false)
    expect(authorityDirty.problems).toContain(
      'packaged_build_authority_worktree_not_clean'
    )
  })

  test('rejects parent traversal even when it resolves to the same directory', () => {
    const directory = writeBundle()
    const traversingPath = `${directory}/../${basename(directory)}`
    expect(evaluate(traversingPath).problems).toEqual([
      'formal_evidence_path_traversal_rejected'
    ])
  })

  test('rejects symlink and hard-link report substitution', () => {
    const symlinkDirectory = mkdtempSync(join(tmpdir(), 'analytix-formal-evidence-link-'))
    temporaryDirectories.push(symlinkDirectory)
    const outside = join(symlinkDirectory, 'outside.json')
    writeFileSync(outside, JSON.stringify(b1Report()))
    writeFileSync(
      join(symlinkDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0),
      JSON.stringify(a0Report())
    )
    symlinkSync(outside, join(
      symlinkDirectory, runtimeGoFormalEvidenceContract.reportFiles.b1
    ))
    expect(evaluate(symlinkDirectory).problems).toEqual([
      'formal_evidence_report_not_unique_regular_file'
    ])

    const hardLinkDirectory = mkdtempSync(join(tmpdir(), 'analytix-formal-evidence-hardlink-'))
    temporaryDirectories.push(hardLinkDirectory)
    const a0Path = join(hardLinkDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0)
    writeFileSync(a0Path, JSON.stringify(a0Report()))
    const extraA0Link = join(hardLinkDirectory, 'a0-copy.json')
    linkSync(a0Path, extraA0Link)
    writeFileSync(
      join(hardLinkDirectory, runtimeGoFormalEvidenceContract.reportFiles.b1),
      JSON.stringify(b1Report())
    )
    expect(evaluate(hardLinkDirectory).problems).toEqual([
      'formal_evidence_report_not_unique_regular_file'
    ])
  })

  test('implements same-fd pre/post identity checks for TOCTOU closure', () => {
    const sourceText = readFileSync(
      join(repoRoot, 'scripts/runtime-go-formal-evidence.mjs'), 'utf8'
    )
    expect(sourceText).toContain('const beforeFd = fstatSync(fd, { bigint: true })')
    expect(sourceText).toContain('const afterFd = fstatSync(fd, { bigint: true })')
    expect(sourceText).toContain('!stableSingleLinkIdentity(beforeFd, afterFd)')
    expect(sourceText).toContain('!stableSingleLinkIdentity(afterFd, afterPath)')
  })

  test('rejects a hard-link count introduced between stable identity samples', () => {
    const singleLink = {
      dev: 1n,
      ino: 2n,
      size: 3n,
      mtimeNs: 4n,
      ctimeNs: 5n,
      nlink: 1n
    }
    const hardLinked = { ...singleLink, nlink: 2n }

    expect(runtimeGoFormalEvidenceTestInternals.stableIdentity(
      singleLink,
      hardLinked
    )).toBe(false)
    expect(runtimeGoFormalEvidenceTestInternals.stableSingleLinkIdentity(
      hardLinked,
      hardLinked
    )).toBe(false)
  })

  test('release gate rejects invalid formal evidence before its lock or any command', () => {
    const fixture = fakeReleaseGateRepo()
    const cacheRoot = mkdtempSync(join(tmpdir(), 'analytix-release-preflight-cache-'))
    const evidenceDirectory = mkdtempSync(join(tmpdir(), 'analytix-invalid-formal-evidence-'))
    temporaryDirectories.push(cacheRoot, evidenceDirectory)
    writeFileSync(
      join(evidenceDirectory, runtimeGoFormalEvidenceContract.reportFiles.a0),
      JSON.stringify({ schemaVersion: 1, unexpectedAuthorization: true })
    )
    writeFileSync(
      join(evidenceDirectory, runtimeGoFormalEvidenceContract.reportFiles.b1),
      JSON.stringify({ schemaVersion: 1 })
    )

    const result = runReleaseGate(fixture.directory, cacheRoot, [
      '--formal-evidence-dir', evidenceDirectory
    ])

    expect(result.status).toBe(1)
    expect(result.report).toMatchObject({
      status: 'failed',
      formalRun: { status: 'not_executed', created: false },
      formalEvidence: {
        electron: 'not_executed',
        package: 'not_executed',
        a0: 'not_executed',
        b1: 'not_executed'
      },
      formalEvidenceInput: { requested: true, consumed: false },
      formalEvidencePreflight: {
        phase: 'pre_lock_read_only',
        status: 'failed',
        formalExecution: 'not_executed',
        receiptCreated: false,
        exactArtifactBinding: 'not_evaluated'
      }
    })
    expect(result.report.formalEvidencePreflight.problems).toContain(
      'a0_report_unknown_property'
    )
    expect(Object.values(result.report.heavyExecutionCounts)).toEqual(
      expect.arrayContaining([0])
    )
    expect(Object.values(result.report.heavyExecutionCounts).every((count) => count === 0))
      .toBe(true)
    expect(result.report.executionManifest.every(
      (entry: { executionCount: number; status: string }) =>
        entry.executionCount === 0 && entry.status === 'not_executed'
    )).toBe(true)
    expect(existsSync(join(
      cacheRoot, 'evidence', 'rc-validation', fixture.head, 'formal-run.json'
    ))).toBe(false)
  })

  test('release gate requires formal evidence before its lock or any command', () => {
    const fixture = fakeReleaseGateRepo()
    const cacheRoot = mkdtempSync(join(tmpdir(), 'analytix-release-required-cache-'))
    temporaryDirectories.push(cacheRoot)

    const result = runReleaseGate(fixture.directory, cacheRoot)

    expect(result.status).toBe(1)
    expect(result.report).toMatchObject({
      status: 'failed',
      reason: 'formal evidence input is required before a product release run',
      formalRun: { status: 'not_executed', created: false },
      formalEvidenceInput: { requested: false, consumed: false },
      formalEvidencePreflight: {
        provided: false,
        status: 'not_requested',
        passed: false,
        formalExecution: 'not_executed',
        receiptCreated: false,
        exactArtifactBinding: 'not_evaluated'
      }
    })
    expect(Object.values(result.report.heavyExecutionCounts).every((count) => count === 0))
      .toBe(true)
    expect(result.report.executionManifest.every(
      (entry: { executionCount: number; status: string }) =>
        entry.executionCount === 0 && entry.status === 'not_executed'
    )).toBe(true)
    expect(existsSync(join(
      cacheRoot, 'evidence', 'rc-validation', fixture.head, 'formal-run.json'
    ))).toBe(false)
  })
})


test.each(['same entry', 'K1 replaced by K2'])('R130-F1 carries the actual private scan through both formal gates: %s', async (entryState) => {
  // @ts-expect-error The private harness module is JavaScript.
  const scanner = await import('../../scripts/lib/local-provider-credential-scan.mjs')
  // @ts-expect-error The acceptance harness module is JavaScript.
  const { localProviderSecretStoreEvidence, localProviderAuthorityFromRegistry } = await import('../../scripts/lib/local-provider-acceptance.mjs')
  const root = mkdtempSync(join(tmpdir(), 'analytix-local-acceptance-'))
  temporaryDirectories.push(root)
  const storeDirectory = join(root, 'private', 'provider-secrets')
  mkdirSync(storeDirectory, { recursive: true, mode: 0o700 })
  writeFileSync(join(storeDirectory, 'credentials.v1.json'), '{"encryptedSyntheticFixture":true}', { mode: 0o600 })
  const settingsPath = join(root, 'settings.json')
  writeFileSync(settingsPath, '{}', { mode: 0o600 })
  const providerRegistry = {
    schemaVersion: 1, registryRevision: '1', registryIncarnation: 'inc_' + 'a'.repeat(43),
    selectedProviderId: 'deepseek', providers: [{
      id: 'deepseek', kind: 'deepseek', endpoint: 'https://api.example.invalid/v1',
      models: ['deepseek-v4-flash'], selectedModel: 'deepseek-v4-flash', mediaModels: [], selectedRoutes: [],
      credentialConfigured: true, credentialPurpose: 'provider-api-key', tombstone: false,
      revision: '1', generation: '1', incarnation: 'inc_' + 'b'.repeat(43)
    }]
  }
  const context = { runId: root, entryAttemptId: 'synthetic-visible-entry',
    provider: localProviderAuthorityFromRegistry(providerRegistry) }
  const credential = Buffer.from('R130-synthetic-only-private-input')
  const sourceHandle = await scanner.captureLocalCredentialEntry({ ...context, providerId: context.provider.id,
    entryMethod: 'visible-computer-use', credential }, async (entered: Buffer) => ({
    completed: entered.equals(credential), providerRegistry
  }))
  try {
    const a0 = a0Report()
    const scan = await scanner.scanLocalCredentialIsolation({ ...context, source: sourceHandle,
      root, settingsPath, reportSnapshot: a0 })
    expect(scan.ok).toBe(true)
    a0.provider.localCredentialEvidence = { ...scan,
      protectedStoreOwnerPrivate: localProviderSecretStoreEvidence(root).protectedStoreOwnerPrivate }
    a0.redaction.scannedFileCount = scan.scannedFileCount
    const positive = preflightRuntimeGoFormalEvidence({ repoRoot, source,
      bundleDirectory: writePreflightBundle(a0, b1Report()) })
    expect(positive.problems).toEqual([])
    const replacement = entryState === 'K1 replaced by K2'
    if (replacement) {
      providerRegistry.registryRevision = '2'
      providerRegistry.providers[0].revision = '2'
      providerRegistry.providers[0].generation = '2'
      context.provider = localProviderAuthorityFromRegistry(providerRegistry)
    }
    // In the replacement case K1 never leaks: only the new K2 appears on disk.
    const leakedEncoding = replacement ? Buffer.from('R130-synthetic-replacement-K2').toString('base64') : credential.toString('base64')
    writeFileSync(join(root, 'escaped-log.txt'), leakedEncoding, { mode: 0o600 })
    const leaked = await scanner.scanLocalCredentialIsolation({ ...context, source: sourceHandle,
      root, settingsPath, reportSnapshot: a0 })
    expect(leaked.status).toBe(replacement ? 'blocked' : 'failed')
    if (replacement) expect(leaked).toMatchObject({ sourceBound: false, sourceSecretCount: 0, scannedFileCount: 0 })
    a0.provider.localCredentialEvidence = { ...leaked, protectedStoreOwnerPrivate: true }
    const b1 = b1Report()
    b1.provider.localCredentialEvidence = { ...leaked, protectedStoreOwnerPrivate: true }
    const negative = preflightRuntimeGoFormalEvidence({ repoRoot, source,
      bundleDirectory: writePreflightBundle(a0, b1) })
    expect(negative.problems).toContain('a0_provider_pass_projection_invalid')
    expect(negative.problems).toContain('b1_provider_pass_projection_invalid')
    expect(JSON.stringify(leaked)).not.toContain(leakedEncoding)
  } finally {
    scanner.disposeLocalCredentialScanSource(sourceHandle)
    credential.fill(0)
  }
})

test('R130 rejects Hub substitution and non-vacuous local credential proof failures at formal ingestion', () => {
  const fields = [
    { sourceSecretCount: 0 }, { sourceBound: false }, { expectedSecretCount: 0 }, { uniqueSecretCount: 0 },
    { status: 'blocked' }, { status: 'failed' }, { scannedFileCount: 0 }, { findingCount: 1 },
    { settingsFindingCount: 1 }, { reportFindingCount: 1 }, { unsafeEntryCount: 1 }, { symlinkCount: 1 },
    { entryMethod: 'visible-human', automatedCredentialEntryUsed: true },
    { encodingCoverage: ['utf8'] }, { credentialFingerprint: 'a'.repeat(64) }
  ]
  for (const changes of fields) {
    const a0 = a0Report()
    const b1 = b1Report()
    Object.assign(a0.provider.localCredentialEvidence, changes)
    Object.assign(b1.provider.localCredentialEvidence, changes)
    const result = preflightRuntimeGoFormalEvidence({ repoRoot, source,
      bundleDirectory: writePreflightBundle(a0, b1) })
    expect(result.problems).toContain('a0_provider_pass_projection_invalid')
    expect(result.problems).toContain('b1_provider_pass_projection_invalid')
  }
  const a0 = a0Report()
  const b1 = b1Report()
  Object.assign(a0.provider, { managedByHub: true, normalHubLoginObserved: true })
  Object.assign(b1.provider, { normalHubLoginObserved: true, credentialFilesOwnerPrivate: true })
  const hub = preflightRuntimeGoFormalEvidence({ repoRoot, source, bundleDirectory: writePreflightBundle(a0, b1) })
  expect(hub.problems).toContain('a0_provider_pass_projection_invalid')
  expect(hub.problems).toContain('b1_provider_pass_projection_invalid')
})
