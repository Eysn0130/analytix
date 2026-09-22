import { createHash } from 'node:crypto'
import { LOCAL_PROVIDER_AUTHORITY, localCredentialEvidencePassed, LOCAL_CREDENTIAL_ENCODING_COVERAGE } from './lib/local-provider-acceptance.mjs'

import {
  closeSync,
  constants,
  fstatSync,
  lstatSync,
  openSync,
  readFileSync,
  realpathSync
} from 'node:fs'
import { createRequire } from 'node:module'
import { basename, dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import { inspectExactArtifactLegalInventory } from './artifact-legal-obligations-audit.mjs'

const require = createRequire(import.meta.url)
const { parseStrictJsonObject } = require('./lib/strict-json.cjs')
let packagedAuthorityContract = null

function packagedAuthority() {
  packagedAuthorityContract ||= require('./after-pack.cjs')._internals
  return packagedAuthorityContract
}

const FORMAL_EVIDENCE_CONTRACT = 'analytix.runtime-go-formal-evidence/v1'
const FORMAL_REPORT_FILES = Object.freeze({
  a0: 'packaged-milestone-a.json',
  b1: 'packaged-milestone-b.json'
})
const MAX_REPORT_BYTES = 16 * 1024 * 1024
const MAX_AUTHORITY_BYTES = 1024 * 1024
const SHA256_RE = /^[0-9a-f]{64}$/u
const GIT_COMMIT_RE = /^[0-9a-f]{40}$/u
const PACKAGED_AUTHORITY_FILE = 'analytix-packaged-build-authority.json'

const A0_REQUIRED_CHECK_IDS = Object.freeze([
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
const A0_AUXILIARY_CHECK_IDS = Object.freeze([
  'external-real-repository-contract',
  'parent-owned-repository-test-cross-check',
  'protected-funds-source-unavailable'
])
const A0_PASS_STAGE_SNAPSHOT_CHECK_IDS = Object.freeze({
  'first-launch-prerequisites': Object.freeze([
    'external-real-repository-contract',
    'trusted-cache-tmpdir',
    'formal-packaged-artifact',
    'isolated-profile-and-repository',
    'configured-network-provider',
    'packaged-first-launch'
  ]),
  'plan-turn': Object.freeze(['ordinary-agent-workflow']),
  'ordinary-agent-functional-workflow': Object.freeze([
    'real-repository-test',
    'bounded-subagent',
    'git-skill-mcp-research-writing'
  ]),
  'ordinary-agent-workflow': Object.freeze([
    'ordinary-agent-workflow',
    'real-repository-test',
    'bounded-subagent'
  ]),
  'protected-funds-source-unavailable': Object.freeze([
    'protected-funds-source-unavailable'
  ]),
  'long-context-continuation': Object.freeze(['long-context-continuation']),
  'composer-compaction-submit': Object.freeze([
    'composer-compaction-submit',
    'nonzero-compaction'
  ]),
  'normal-first-quit': Object.freeze(['normal-first-quit']),
  'relaunch-provider-continuation': Object.freeze([
    'fresh-packaged-relaunch',
    'relaunch-provider-continuation'
  ])
})
const B1_REQUIRED_CHECK_IDS = Object.freeze([
  'trusted-cache-tmpdir',
  'managed-product-data-authority',
  'formal-packaged-artifact',
  'external-owner-isolated-case',
  'canonical-csv-contract-and-provenance',
  'provider-request-audit-authority',
  'configured-network-provider',
  'typed-local-display',
  'direct-source-preview',
  'packaged-first-launch',
  'ordinary-before-case',
  'case-authority-unavailable-fails-closed',
  'ordinary-after-case-block',
  'native-snapshot-one-staging',
  'funds-catalog-after-snapshot-one',
  'real-account-flow-one',
  'host-funds-invocation-and-final-binding',
  'evidence-claim-final-gate-one',
  'default-redacted-ui-one',
  'ordinary-between-funds',
  'native-snapshot-two-staging',
  'snapshot-evolution-and-history',
  'real-account-flow-two',
  'ordinary-after-typed-local-display',
  'same-thread-additive-sequence',
  'mixed-code-todo-subagent-funds',
  'authority-loss-mixed-turn-and-reacquisition',
  'protected-source-general-tool-bypass-denied',
  'case-switch-epoch-and-default-entity-isolation',
  'fork-subagent-case-isolation',
  'public-pii-scan-zero',
  'provider-payload-pii-scan-zero',
  'provider-semantic-context-contract',
  'typed-local-display-revocation',
  'typed-local-display-rehydration',
  'nonzero-compaction',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'exact-thread-recovery',
  'recovered-case-query',
  'normal-final-quit',
  'zero-residual-processes',
  'sandbox-cleanup-and-case-preservation'
])

const A0_HARNESS_FILES = Object.freeze([
  'scripts/runtime-go-packaged-milestone-a.mjs',
  'scripts/lib/local-provider-acceptance.mjs',
  'scripts/lib/local-provider-credential-scan.mjs',
  'scripts/runtime-go-packaged-qa.mjs',
  'scripts/runtime-go-live-evidence-collector.mjs',
  'scripts/runtime-go-validation-command.mjs',
  'scripts/runtime-go-cutover-report.mjs',
  'scripts/runtime-go-packaged-gui-smoke.mjs',
  'scripts/runtime-go-packaged-session-soak.mjs',
  'scripts/runtime-go-rollback-retirement-evidence.mjs',
  'src/main/packaging-config.test.ts',
  'scripts/after-pack.cjs',
  'scripts/lib/packaged-release-publication-authority.mjs',
  'scripts/publish-r2.mjs',
  'scripts/lib/strict-json.cjs',
  'scripts/macos-signing-policy.cjs',
  'scripts/macos-signing-policy.json',
  'build/entitlements.mac.native-helper.plist',
  'package.json',
  'package-lock.json',
  'scripts/use-analytix-cache.sh',
  'scripts/analytix-cache-storage.zsh'
])
const B1_HARNESS_FILES = Object.freeze([
  'scripts/runtime-go-packaged-milestone-b.mjs',
  'scripts/lib/local-provider-acceptance.mjs',
  'scripts/lib/local-provider-credential-scan.mjs',
  'scripts/runtime-go-validation-command.mjs',
  'scripts/after-pack.cjs',
  'package.json'
])

const A0_ROOT_KEYS = new Set([
  'schemaVersion', 'id', 'stage', 'generatedAt', 'sourceCommit',
  'testHarnessCommit', 'harness', 'status', 'passed', 'timeoutMs',
  'credentialSecretsRecorded', 'rawProviderPayloadRecorded',
  'syntheticProviderUsed', 'localProviderUsed', 'directRuntimeTurnDriverUsed',
  'composerDomAndPrimaryButtonRequired', 'cdpAndBridgeObservationOnly',
  'operatorCheckpoint', 'stageSnapshots', 'app', 'repository',
  'artifactRevalidation', 'releasePublicationAuthority', 'commercialRelease',
  'provider', 'visualEvidence', 'isolation', 'caseCapability', 'workflow',
  'publicSeams', 'redaction', 'parentOwnedRepositoryTest', 'executionBlocker',
  'executionBlockerClassification', 'checks', 'failedCheckIds',
  'skippedCheckIds', 'liveBlockedCheckIds', 'failureCheckIds',
  'failureCheckIdsSemantics', 'blockers', 'missingExternalInputs',
  'outputPathHash'
])
const B1_ROOT_KEYS = new Set([
  'schemaVersion', 'id', 'stage', 'generatedAt', 'sourceCommit', 'harnessCommit',
  'harness', 'status', 'passed', 'runtimeBlocker', 'runtimeFailure', 'timeoutMs',
  'app', 'acceptanceClass', 'mockUsed', 'fixtureUsed', 'syntheticProviderUsed',
  'directRuntimeTurnDriverUsed', 'directSnapshotIPCUsed', 'arbitrarySQLUsed',
  'rendererDatabasePathReceived', 'completePIIRecorded', 'phaseLanes',
  'postRCChecks', 'externalCase', 'managedProductData', 'providerAudit',
  'typedLocalDisplay', 'directSourcePreview', 'provider', 'isolation',
  'publicSeams', 'workflow', 'exit', 'artifact', 'artifactFinal', 'checks',
  'failureCheckIds', 'blockedCheckIds', 'unverifiedCheckIds', 'diagnostic',
  'negativeIsolationCaseSynthetic'
])

const A0_APP_KEYS = new Set([
  'targetKey', 'appPathHash', 'ok', 'blocked', 'blocker', 'sourceCommit',
  'authoritySha256', 'authorityDigest', 'executableSha256',
  'runtimeServerSha256', 'appAsarSha256', 'sourceSkillSha256',
  'packagedSkillSha256', 'packagedSkillSourceMatched',
  'packagedScheduleConfigSourceSha256', 'packagedScheduleServerSourceSha256',
  'packagedScheduleSourceContractBound',
  'packagedScheduleStrictEmptyInputSchemaBound',
  'packagedScheduleNonMutatingListImplementationBound',
  'codeSignatureVerified', 'nativeDispositionKind',
  'developmentNativeDisposition', 'controlledReleaseNativeReceipt', 'controlledCoreQualification',
  'worktreeSnapshotBinding', 'bundledRuntimeGoSourcePresent'
])
const B1_ARTIFACT_KEYS = new Set([
  'ok', 'blocked', 'blocker', 'targetKey', 'appPathHash', 'sourceCommit',
  'authoritySha256', 'authorityDigest', 'executableSha256',
  'runtimeServerSha256', 'appAsarSha256', 'worktreeSnapshotDigest',
  'codeSignatureVerified', 'fundsPluginArtifactBound'
])

const ELECTRON_CHECK_IDS = Object.freeze([
  'packaged-first-launch',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'normal-final-quit',
  'zero-residual-processes'
])
const B1_PHASE_LANES = Object.freeze({
  ordinaryProvider: Object.freeze({
    phaseOwner: 'ordinary_provider',
    dependencies: Object.freeze(['packaged_first_launch', 'provider_configuration'])
  }),
  b1DirectSourcePreview: Object.freeze({
    phaseOwner: 'b1_direct_source_preview',
    dependencies: Object.freeze(['packaged_first_launch', 'current_case_snapshot'])
  }),
  b1DeterministicFunds: Object.freeze({
    phaseOwner: 'b1_deterministic_funds',
    dependencies: Object.freeze(['packaged_first_launch', 'current_case_snapshot'])
  }),
  b1AgentAcceptedSlot: Object.freeze({
    phaseOwner: 'b1_agent_accepted_slot',
    dependencies: Object.freeze(['packaged_first_launch', 'provider_configuration', 'funds_result'])
  })
})

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function exactKeys(value, expected) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join('\0') === [...expected].sort().join('\0')
}

function hasOnlyKnownKeys(value, allowed) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).every((key) => allowed.has(key))
}

function staticProblem(problems, condition, code) {
  if (!condition) problems.push(code)
}

function stableIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino &&
    left.size === right.size && left.mtimeNs === right.mtimeNs &&
    left.ctimeNs === right.ctimeNs && left.nlink === right.nlink
}

function stableSingleLinkIdentity(left, right) {
  return stableIdentity(left, right) && left.nlink === 1n && right.nlink === 1n
}

function stableFormalEvidenceDirectory(bundleDirectory) {
  const requestedDirectory = resolve(bundleDirectory)
  const rawSegments = String(bundleDirectory).split(/[\\/]+/u)
  if (rawSegments.includes('..')) throw new Error('formal_evidence_path_traversal_rejected')
  const directoryStat = lstatSync(requestedDirectory, { bigint: true })
  const realDirectory = realpathSync(requestedDirectory)
  if (!directoryStat.isDirectory() || directoryStat.isSymbolicLink() ||
      realDirectory !== requestedDirectory) {
    throw new Error('formal_evidence_directory_not_canonical_regular_directory')
  }
  return { path: realDirectory, identity: directoryStat }
}

function stableFormalEvidenceDirectoryIdentity(directory) {
  const observed = lstatSync(directory.path, { bigint: true })
  return observed.isDirectory() && !observed.isSymbolicLink() &&
    realpathSync(directory.path) === directory.path &&
    stableIdentity(directory.identity, observed)
}

function readStableFormalReport(bundleDirectory, fileName) {
  if (!stableFormalEvidenceDirectoryIdentity(bundleDirectory)) {
    throw new Error('formal_evidence_directory_changed_during_read')
  }
  const reportPath = join(bundleDirectory.path, fileName)
  if (dirname(reportPath) !== bundleDirectory.path || basename(reportPath) !== fileName) {
    throw new Error('formal_evidence_path_escape_rejected')
  }
  const beforePath = lstatSync(reportPath, { bigint: true })
  const realReportPath = realpathSync(reportPath)
  if (!beforePath.isFile() || beforePath.isSymbolicLink() || beforePath.nlink !== 1n ||
      dirname(realReportPath) !== bundleDirectory.path || basename(realReportPath) !== fileName) {
    throw new Error('formal_evidence_report_not_unique_regular_file')
  }
  if (beforePath.size <= 0n || beforePath.size > BigInt(MAX_REPORT_BYTES)) {
    throw new Error('formal_evidence_report_size_invalid')
  }
  const noFollow = Number(constants.O_NOFOLLOW || 0)
  const fd = openSync(reportPath, constants.O_RDONLY | noFollow)
  try {
    const beforeFd = fstatSync(fd, { bigint: true })
    if (!stableSingleLinkIdentity(beforePath, beforeFd)) {
      throw new Error('formal_evidence_report_identity_changed_before_read')
    }
    const bytes = readFileSync(fd)
    const afterFd = fstatSync(fd, { bigint: true })
    const afterPath = lstatSync(reportPath, { bigint: true })
    if (BigInt(bytes.length) !== beforeFd.size ||
        !stableSingleLinkIdentity(beforeFd, afterFd) ||
        !stableSingleLinkIdentity(afterFd, afterPath) ||
        !stableFormalEvidenceDirectoryIdentity(bundleDirectory)) {
      throw new Error('formal_evidence_report_changed_during_read')
    }
    const report = parseStrictJsonObject(bytes, {
      maxBytes: MAX_REPORT_BYTES,
      maxDepth: 64,
      maxTokens: 1_000_000,
      maxStringBytes: 2 * 1024 * 1024,
      maxNumberBytes: 128
    })
    return {
      report,
      sha256: sha256(bytes),
      byteLength: bytes.length,
      identity: `${beforeFd.dev}:${beforeFd.ino}`
    }
  } finally {
    closeSync(fd)
  }
}

function readStableAuthorityFile(filePath) {
  const requestedPath = resolve(filePath)
  const beforePath = lstatSync(requestedPath, { bigint: true })
  if (!beforePath.isFile() || beforePath.isSymbolicLink() || beforePath.nlink !== 1n ||
      realpathSync(requestedPath) !== requestedPath || beforePath.size <= 0n ||
      beforePath.size > BigInt(MAX_AUTHORITY_BYTES)) {
    throw new Error('packaged_build_authority_file_invalid')
  }
  const noFollow = Number(constants.O_NOFOLLOW || 0)
  const fd = openSync(requestedPath, constants.O_RDONLY | noFollow)
  try {
    const beforeFd = fstatSync(fd, { bigint: true })
    if (!stableSingleLinkIdentity(beforePath, beforeFd)) {
      throw new Error('packaged_build_authority_identity_changed_before_read')
    }
    const bytes = readFileSync(fd)
    const afterFd = fstatSync(fd, { bigint: true })
    const afterPath = lstatSync(requestedPath, { bigint: true })
    if (BigInt(bytes.length) !== beforeFd.size ||
        !stableSingleLinkIdentity(beforeFd, afterFd) ||
        !stableSingleLinkIdentity(afterFd, afterPath)) {
      throw new Error('packaged_build_authority_changed_during_read')
    }
    const authority = parseStrictJsonObject(bytes, {
      maxBytes: MAX_AUTHORITY_BYTES,
      maxDepth: 64,
      maxTokens: 200_000,
      maxStringBytes: 256 * 1024,
      maxNumberBytes: 128
    })
    if (bytes.toString('utf8') !== JSON.stringify(authority)) {
      throw new Error('packaged_build_authority_not_canonical')
    }
    return { authority, sha256: sha256(bytes), byteLength: bytes.length }
  } finally {
    closeSync(fd)
  }
}

function currentHarnessEntries(repoRoot, relativePaths) {
  return relativePaths.map((relativePath) => {
    const absolutePath = resolve(repoRoot, relativePath)
    const ownerRelative = relative(repoRoot, absolutePath)
    if (!ownerRelative || ownerRelative === '..' || ownerRelative.startsWith(`..${sep}`) ||
        isAbsolute(ownerRelative)) {
      throw new Error('formal_evidence_harness_path_escape')
    }
    const identity = stableArtifactFileIdentity(absolutePath, MAX_REPORT_BYTES)
    const displayName = relativePath.includes('/')
      ? relativePath.split('/').at(-1)
      : relativePath
    return {
      name: relativePath === 'scripts/use-analytix-cache.sh' ||
          relativePath === 'scripts/analytix-cache-storage.zsh'
        ? relativePath
        : displayName,
      regular: true,
      byteLength: identity.byteLength,
      sha256: identity.sha256
    }
  })
}

function validateHarness(reportHarness, expectedEntries, lane) {
  const problems = []
  const expectedKeys = lane === 'a0'
    ? [
        'scriptSha256', 'contractManifestSha256', 'contractManifestBound',
        'harnessSourceRootBound', 'sourceClosureBound',
        'sourceClosureSnapshotDigest', 'packagedSourceClosureSnapshotDigest',
        'entries', 'formalExecutionBounds'
      ]
    : ['scriptSha256', 'contractManifestSha256', 'entries']
  staticProblem(problems, exactKeys(reportHarness, expectedKeys), `${lane}_harness_schema_invalid`)
  if (!exactKeys(reportHarness, expectedKeys)) return problems
  staticProblem(problems, Array.isArray(reportHarness.entries) &&
    reportHarness.entries.length === expectedEntries.length,
  `${lane}_harness_entry_count_invalid`)
  if (Array.isArray(reportHarness.entries)) {
    reportHarness.entries.forEach((entry, index) => {
      staticProblem(problems, exactKeys(entry, ['name', 'regular', 'byteLength', 'sha256']) &&
        canonicalJSON(entry) === canonicalJSON(expectedEntries[index]),
      `${lane}_harness_entry_mismatch`)
    })
  }
  staticProblem(problems,
    reportHarness.scriptSha256 === expectedEntries[0]?.sha256 &&
      reportHarness.contractManifestSha256 === sha256(canonicalJSON(expectedEntries)),
  `${lane}_harness_digest_mismatch`)
  if (lane === 'a0') {
    staticProblem(problems,
      reportHarness.contractManifestBound === true &&
      reportHarness.harnessSourceRootBound === true &&
      reportHarness.sourceClosureBound === true &&
      SHA256_RE.test(String(reportHarness.sourceClosureSnapshotDigest || '')) &&
      reportHarness.packagedSourceClosureSnapshotDigest ===
        reportHarness.sourceClosureSnapshotDigest,
    'a0_harness_source_closure_invalid')
    staticProblem(problems, exactKeys(reportHarness.formalExecutionBounds, [
      'userGlobalMaxModelSteps', 'plannerMaxModelSteps', 'childMaxModelSteps',
      'childTimeBudgetMs'
    ]) && reportHarness.formalExecutionBounds.userGlobalMaxModelSteps === 32 &&
      reportHarness.formalExecutionBounds.plannerMaxModelSteps === 12 &&
      reportHarness.formalExecutionBounds.childMaxModelSteps === 8 &&
      reportHarness.formalExecutionBounds.childTimeBudgetMs === 180_000,
    'a0_formal_execution_bounds_invalid')
  }
  return [...new Set(problems)]
}

function validateChecks(checks, requiredIds, optionalIds, passedStatus, lane) {
  const problems = []
  const allowedIds = new Set([...requiredIds, ...optionalIds])
  staticProblem(problems, Array.isArray(checks), `${lane}_checks_not_array`)
  if (!Array.isArray(checks)) return problems
  const seen = new Set()
  for (const item of checks) {
    const valid = exactKeys(item, ['id', 'status', 'message']) &&
      typeof item.id === 'string' && allowedIds.has(item.id) &&
      item.status === passedStatus && typeof item.message === 'string' &&
      Buffer.byteLength(item.message, 'utf8') <= 1024 && !seen.has(item.id)
    staticProblem(problems, valid, `${lane}_check_schema_status_or_identity_invalid`)
    if (typeof item?.id === 'string') seen.add(item.id)
  }
  staticProblem(problems, requiredIds.every((id) => seen.has(id)), `${lane}_required_check_missing`)
  return [...new Set(problems)]
}

function validGeneratedAt(value) {
  if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}T/u.test(value)) return false
  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) && timestamp <= Date.now() + 5 * 60_000
}

function record(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : null
}

function positiveSafeInteger(value) {
  return Number.isSafeInteger(value) && value > 0
}

function a0PublicSeamPassed(value) {
  const seam = record(value)
  return Boolean(seam && seam.healthOk === true && seam.runtimeInfoOk === true &&
    seam.runtimeToolsOk === true && seam.runtimeThreadListProbeOk === true &&
    positiveSafeInteger(seam.runtimeThreadListProbeStatus) &&
    seam.ordinaryCatalogNonempty === true && seam.gitCommandAvailable === true &&
    seam.skillCatalogAvailable === true && positiveSafeInteger(seam.skillCount) &&
    seam.ordinaryMCPAvailable === true &&
    positiveSafeInteger(seam.ordinaryMCPServerCount) &&
    positiveSafeInteger(seam.ordinaryMCPToolCount) &&
    seam.fundsExecutionUnavailable === true && seam.fundsServerDiagnosticCount === 0 &&
    positiveSafeInteger(seam.ordinaryToolContractCount) &&
    SHA256_RE.test(String(seam.ordinaryToolCatalogHash || '')) &&
    seam.rendererTargetCount === 1 && positiveSafeInteger(seam.electronMainPid) &&
    positiveSafeInteger(seam.runtimeListenerPid) && seam.runtimeListenerCount === 1 &&
    seam.runtimeBackendProcessCount === 1 && seam.runtimeServerProcessCount === 1 &&
    seam.desktopPrivateHistoryMigrationProcessCount === 0 &&
    seam.bundledPluginMaterializationProcessCount === 0 &&
    seam.unknownRuntimeProcessCount === 0 &&
    seam.rendererReadFailureObserved === false &&
    seam.runtimeListenerIsTaskOwnedDescendant === true &&
    seam.exactPackagedRuntimeExecutable === true)
}

function a0CompactionPassed(workflow) {
  const common = workflow.manualCompactionBaselineBound === true &&
    Number.isSafeInteger(workflow.manualCompactionBaselineCount) &&
    workflow.manualCompactionBaselineCount >= 0 &&
    SHA256_RE.test(String(workflow.manualCompactionBaselineDigest || '')) &&
    SHA256_RE.test(String(workflow.manualCompactionCandidateDigest || '')) &&
    workflow.manualCompactionNewAfterBaseline === true &&
    positiveSafeInteger(workflow.compactionCount) &&
    workflow.manualCompactionObserved === true && workflow.manualCompactionAuto === false &&
    SHA256_RE.test(String(workflow.manualCompactionSourceDigest || '')) &&
    workflow.manualCompactionSourceAncestryBound === true
  const ordinary = workflow.manualCompactionProjectionClass === 'ordinary_exact' &&
    positiveSafeInteger(workflow.manualCompactionReplacedTokens) &&
    SHA256_RE.test(String(workflow.manualCompactionSourceItemIdsDigest || ''))
  const typedCase = workflow.manualCompactionProjectionClass === 'case_bound_typed' &&
    workflow.manualCompactionReplacedTokens === 0 &&
    workflow.manualCompactionSourceItemIdsDigest === '' &&
    workflow.manualCompactionNonzeroBound === true
  return common && (ordinary || typedCase)
}

function validateA0AdmissionCriticalProjection(report) {
  const problems = []
  const checkpoint = record(report.operatorCheckpoint)
  staticProblem(problems, Boolean(checkpoint &&
    checkpoint.kind === 'visible-normal-local-provider-setup' && checkpoint.required === true &&
    checkpoint.declared === true && checkpoint.status === 'completed' &&
    ((checkpoint.method === 'visible-human' &&
        checkpoint.automatedCredentialEntryUsed === false) ||
      (checkpoint.method === 'visible-computer-use' &&
        checkpoint.automatedCredentialEntryUsed === true))),
  'a0_operator_checkpoint_pass_projection_invalid')

  const repository = record(report.repository)
  staticProblem(problems, Boolean(repository && repository.required === true &&
    repository.mode === 'external-preexisting-real-repository' &&
    repository.configured === true && repository.validated === true &&
    repository.baselineValidated === true && repository.finalValidated === true &&
    repository.fixtureSubstituteRejected === true && repository.originBound === true &&
    repository.freshOriginVerified === true && repository.formalAcceptance === true &&
    repository.preflightValidated === true && repository.mutationApplied === true &&
    repository.ownerRootBound === true && repository.ownerOnlyIsolationBound === true &&
    repository.gitCommonDirContained === true &&
    repository.symlinkHardlinkIsolationBound === true &&
    repository.planArtifactExcludeBound === true && repository.planArtifactPathSafe === true &&
    repository.finalOnlyIntendedSourceChanged === true &&
    repository.finalExpectedSourceBound === true &&
    positiveSafeInteger(repository.finalExpectedSourceFileCount) &&
    repository.finalExpectedSourceMismatchCount === 0 &&
    SHA256_RE.test(String(repository.finalExpectedSourceDigest || '')) &&
    repository.finalObservedSourceDigest === repository.finalExpectedSourceDigest &&
    SHA256_RE.test(String(repository.finalGitStatusSha256 || '')) &&
    repository.preservedAfterHarnessCleanup === true && repository.blocker === ''),
  'a0_repository_pass_projection_invalid')

  const provider = record(report.provider)
  staticProblem(problems, Boolean(provider && provider.configured === true &&
    provider.credentialConfigured === true && provider.credentialAuthorityBound === true &&
    provider.credentialRecorded === false && provider.credentialAuthority === LOCAL_PROVIDER_AUTHORITY &&
    !Object.hasOwn(provider, 'managedByHub') && !Object.hasOwn(provider, 'normalHubLoginObserved') &&
    provider.normalLocalProviderSetupObserved === true &&
    localCredentialEvidencePassed(provider.localCredentialEvidence) &&
    provider.localCredentialEvidence.entryMethod === checkpoint?.method &&
    provider.localCredentialEvidence.automatedCredentialEntryUsed === checkpoint?.automatedCredentialEntryUsed &&
    SHA256_RE.test(String(provider.providerIdHash || '')) &&
    SHA256_RE.test(String(provider.modelHash || '')) &&
    provider.modelHash === provider.requiredModelHash &&
    provider.modelHash === createHash('sha256').update('deepseek-v4-flash').digest('hex') &&
    SHA256_RE.test(String(provider.baseUrlOriginHash || '')) &&
    provider.contextWindowTokensBound === true &&
    provider.firstLaunchReasoningEffortSelectedThroughVisibleUi === true &&
    provider.secondLaunchReasoningEffortSelectedThroughVisibleUi === true &&
    provider.planReasoningEffortBound === true &&
    provider.ordinaryReasoningEffortBound === true &&
    provider.longContextReasoningEffortBound === true &&
    provider.relaunchReasoningEffortBound === true &&
    provider.networkTurnCompleted === true && provider.threadProviderBound === true &&
    provider.threadModelBound === true && provider.resultTurnProviderReceiptBound === true &&
    provider.providerAttemptTelemetryValid === true &&
    positiveSafeInteger(provider.providerLogicalCallCount) &&
    positiveSafeInteger(provider.providerAttemptCount) &&
    provider.providerAttemptCount >= provider.providerLogicalCallCount &&
    provider.successfulProviderAttemptCount === provider.providerLogicalCallCount),
  'a0_provider_pass_projection_invalid')

  const workflow = record(report.workflow)
  const workflowBooleans = [
    'firstLaunchObserved', 'composerWorkflowSubmitted', 'planComposerSubmitted',
    'planModeSelected', 'planTurnObserved', 'agentModeRestored', 'sameThreadPlanAgent',
    'ordinaryWorkflowCompleted', 'readObserved', 'planObserved', 'todoObserved',
    'writeObserved', 'realTestObserved', 'parentOwnedTestCrossCheckPassed',
    'isolatedGitRepositoryObserved', 'protectedRepositoryInputsBound',
    'onlyIntendedSourceChanged', 'subagentObserved', 'gitCommandObserved',
    'skillObserved', 'ordinaryMCPObserved', 'researchWritingObserved',
    'successfulToolResultsObserved', 'todosCompleted',
    'exactlyOneBoundedSubagentCompleted', 'longContextContinuationObserved',
    'normalFirstQuitObserved', 'freshProcessRelaunchObserved',
    'relaunchProviderContinuationObserved', 'exactThreadRecovered',
    'exactTodosRecovered', 'exactSubagentsRecovered', 'exactCompactionsRecovered',
    'exactResultRecovered', 'rendererVisibleThreadRecovered',
    'rendererVisibleTodosRecovered', 'rendererVisibleSubagentRecovered',
    'rendererVisibleCompactionRecovered', 'rendererVisibleResultRecovered',
    'normalFinalQuitObserved', 'zeroResidualProcesses'
  ]
  staticProblem(problems, Boolean(workflow &&
    workflowBooleans.every((key) => workflow[key] === true) &&
    workflow.normalFirstQuitReasonCode === 'normal_quit_observed' &&
    workflow.normalFinalQuitReasonCode === 'normal_quit_observed' &&
    workflow.firstExitCode === 0 && !workflow.firstExitSignal &&
    workflow.finalExitCode === 0 && !workflow.finalExitSignal &&
    a0CompactionPassed(workflow)),
  'a0_workflow_pass_projection_invalid')

  const parentTest = record(report.parentOwnedRepositoryTest)
  staticProblem(problems, Boolean(parentTest && parentTest.passed === true &&
    parentTest.blocker === '' && parentTest.parentProcessOwned === true &&
    parentTest.exitCode === 0 && !parentTest.signal &&
    parentTest.repositoryStableBeforeAndAfter === true &&
    parentTest.sandboxCleanupSucceeded === true &&
    parentTest.substitutesForAgentInvocationBinding === false),
  'a0_parent_repository_test_pass_projection_invalid')

  const caseCapability = record(report.caseCapability)
  staticProblem(problems, Boolean(caseCapability &&
    caseCapability.additiveNotReplacement === true &&
    caseCapability.bundledFundsMissingAuthorityPreconditionEstablished === true &&
    caseCapability.firstLaunchBundledFundsMaterializationFailureObserved === true &&
    caseCapability.secondLaunchBundledFundsMaterializationFailureObserved === true &&
    caseCapability.fundsTransportAbsentOnBothLaunches === true &&
    caseCapability.privateAuthorityInputsAbsent === true &&
    caseCapability.firstLaunchFundsExecutionUnavailable === true &&
    caseCapability.secondLaunchFundsExecutionUnavailable === true &&
    caseCapability.protectedFundsSourceUnavailable === true &&
    caseCapability.restartedProtectedFundsSourceUnavailable === true &&
    caseCapability.protectedFundsClaimCount === 0 &&
    caseCapability.protectedFundsReceiptCount === 0 &&
    caseCapability.protectedFundsSuccessfulExecutionCount === 0 &&
    caseCapability.ordinaryContinuedAfterProtectedFundsBlock === true &&
    caseCapability.ordinaryContinuedAfterRestart === true &&
    caseCapability.ordinaryCatalogAvailable === true &&
    caseCapability.ordinaryWorkflowCompleted === true &&
    caseCapability.unavailableWhileOrdinaryCapabilitiesRetained === true),
  'a0_case_capability_pass_projection_invalid')

  const publicSeams = record(report.publicSeams)
  staticProblem(problems, Boolean(publicSeams &&
    a0PublicSeamPassed(publicSeams.firstLaunch) &&
    a0PublicSeamPassed(publicSeams.secondLaunch)),
  'a0_public_seam_pass_projection_invalid')

  const isolation = record(report.isolation)
  staticProblem(problems, Boolean(isolation && isolation.cacheTmpdirVerified === true &&
    isolation.systemUserDataTouched === false && isolation.isolatedHomeUsedByChild === true &&
    isolation.isolatedLoginKeychainCreated === true &&
    isolation.isolatedLoginKeychainDefaultBound === true &&
    isolation.isolatedLoginKeychainReusedForTwoLaunches === true &&
    isolation.isolatedLoginKeychainPasswordRecorded === false &&
    isolation.chromiumTempBoundToTrustedCache === true &&
    isolation.sameDataDirsOnRelaunch === true && isolation.sandboxRemoved === true),
  'a0_isolation_cleanup_pass_projection_invalid')

  const redaction = record(report.redaction)
  staticProblem(problems, Boolean(redaction && redaction.status === 'passed' &&
    redaction.secretMaterialFound === false && redaction.settingsFindingCount === 0 &&
    redaction.reportFindingCount === 0 && redaction.unsafeEntryCount === 0 &&
    redaction.symlinkCount === 0 && redaction.expectedSecretCount === 1 &&
    redaction.sourceSecretCount === 1 && redaction.uniqueSecretCount === 1 &&
    redaction.sourceBound === true && positiveSafeInteger(redaction.scannedFileCount) &&
    JSON.stringify(redaction.encodingCoverage) === JSON.stringify(LOCAL_CREDENTIAL_ENCODING_COVERAGE) &&
    redaction.scannedFileCount === provider?.localCredentialEvidence?.scannedFileCount &&
    !Object.hasOwn(redaction, 'authFilePresent') && !Object.hasOwn(redaction, 'gatewayTokenFilePresent') &&
    redaction.credentialRecorded === false),
  'a0_credential_redaction_pass_projection_invalid')

  const visual = record(report.visualEvidence)
  const validCapture = (value) => {
    const capture = record(value)
    return Boolean(capture && capture.captured === true &&
      SHA256_RE.test(String(capture.sha256 || '')) &&
      positiveSafeInteger(capture.width) && positiveSafeInteger(capture.height))
  }
  staticProblem(problems, Boolean(visual && validCapture(visual.firstCompleted) &&
    validCapture(visual.recoveredAfterRelaunch) &&
    visual.screenshotsUsedAsFunctionalVerdict === false &&
    visual.credentialScanCovered === true),
  'a0_visual_evidence_pass_projection_invalid')
  return problems
}

function a0SnapshotRecoveryConsistent(workflow, completedFields) {
  if (!record(workflow) || !record(completedFields)) return false
  const recovered = { ...workflow }
  for (const [key, value] of Object.entries(completedFields)) {
    const current = recovered[key]
    if (typeof value === 'boolean') {
      if (current === undefined || current === null ||
          (value && current === false && !key.endsWith('Auto'))) {
        recovered[key] = value
      }
      continue
    }
    if (Number.isSafeInteger(value)) {
      if (!Number.isSafeInteger(current) || value > current) recovered[key] = value
      continue
    }
    if (typeof value === 'string') {
      if (typeof current !== 'string' || current.length === 0 ||
          (key === 'manualCompactionProjectionClass' &&
            current === 'not_observed' && value !== 'not_observed')) {
        recovered[key] = value
      }
      continue
    }
    return false
  }
  return canonicalJSON(recovered) === canonicalJSON(workflow)
}

function validateA0StageSnapshots(report) {
  const snapshots = report.stageSnapshots
  const requiredStages = Object.keys(A0_PASS_STAGE_SNAPSHOT_CHECK_IDS)
  if (!Array.isArray(snapshots) || snapshots.length !== requiredStages.length) {
    return ['a0_stage_snapshot_pass_projection_invalid']
  }
  const passedCheckIds = new Set(Array.isArray(report.checks)
    ? report.checks
      .filter((item) => item?.status === 'passed')
      .map((item) => item.id)
    : [])
  const stages = new Set()
  for (const snapshot of snapshots) {
    const expectedCheckIds = A0_PASS_STAGE_SNAPSHOT_CHECK_IDS[snapshot?.stage]
    if (!exactKeys(snapshot, ['stage', 'checkIds', 'completedFields', 'snapshotDigest']) ||
        !expectedCheckIds ||
        stages.has(snapshot.stage) || !Array.isArray(snapshot.checkIds) ||
        canonicalJSON(snapshot.checkIds) !== canonicalJSON(expectedCheckIds) ||
        !expectedCheckIds.every((id) => passedCheckIds.has(id)) ||
        !record(snapshot.completedFields) ||
        !a0SnapshotRecoveryConsistent(report.workflow, snapshot.completedFields)) {
      return ['a0_stage_snapshot_pass_projection_invalid']
    }
    const projected = {
      stage: snapshot.stage,
      checkIds: snapshot.checkIds,
      completedFields: snapshot.completedFields
    }
    if (!SHA256_RE.test(String(snapshot.snapshotDigest || '')) ||
        snapshot.snapshotDigest !== sha256(canonicalJSON(projected))) {
      return ['a0_stage_snapshot_pass_projection_invalid']
    }
    stages.add(snapshot.stage)
  }
  return requiredStages.every((stage) => stages.has(stage))
    ? []
    : ['a0_stage_snapshot_pass_projection_invalid']
}

function b1PublicSeamPassed(value) {
  const seam = record(value)
  return Boolean(seam && seam.healthOk === true && seam.runtimeInfoOk === true &&
    seam.ordinaryCatalogNonempty === true &&
    positiveSafeInteger(seam.ordinaryToolContractCount) &&
    SHA256_RE.test(String(seam.ordinaryToolCatalogHash || '')))
}

function b1PublicPIIScanPassed(value) {
  const scan = record(value)
  const forbidden = record(scan?.forbiddenGenericProviderChannels)
  const allowed = record(scan?.allowedTypedLocalSink)
  return Boolean(scan && scan.ok === true && positiveSafeInteger(scan.publicSurfaceCount) &&
    scan.incompletePublicSurfaceCount === 0 && scan.publicFindingCount === 0 &&
    scan.publicInternalReferenceFindingCount === 0 && scan.logFindingCount === 0 &&
    scan.logInternalReferenceFindingCount === 0 && scan.logUnsafeEntryCount === 0 &&
    forbidden && positiveSafeInteger(forbidden.surfaceCount) &&
    forbidden.completePIIFindingCount === 0 &&
    forbidden.internalReferenceFindingCount === 0 &&
    forbidden.incompleteSurfaceCount === 0 && forbidden.unsafeLogEntryCount === 0 &&
    allowed && allowed.excludedFromForbiddenScan === true &&
    allowed.sourceExactFieldsAllowed === true &&
    allowed.scanBoundary === 'typed-local-display-only')
}

function validateB1AdmissionCriticalProjection(report) {
  const problems = []
  const postRC = report.postRCChecks
  staticProblem(problems, Array.isArray(postRC) && postRC.length === 1 &&
    exactKeys(postRC[0], ['id', 'status', 'message']) &&
    postRC[0].id === 'authorized-joint-case-linkage' &&
    postRC[0].status === 'UNVERIFIED' && typeof postRC[0].message === 'string',
  'b1_post_rc_projection_invalid')

  const externalCase = record(report.externalCase)
  staticProblem(problems, Boolean(externalCase && externalCase.configured === true &&
    externalCase.ownerIsolated === true && externalCase.synthetic === false &&
    externalCase.snapshotCount === 2 && externalCase.preserved === true &&
    SHA256_RE.test(String(externalCase.contractSha256 || '')) &&
    SHA256_RE.test(String(externalCase.provenanceSha256 || ''))),
  'b1_external_case_pass_projection_invalid')

  const managed = record(report.managedProductData)
  staticProblem(problems, Boolean(managed && managed.configured === true &&
    managed.bootstrapInstalled === true && managed.installationAuthoritySeeded === true &&
    positiveSafeInteger(managed.authorityRootCount) &&
    SHA256_RE.test(String(managed.bootstrapSha256 || '')) &&
    SHA256_RE.test(String(managed.installationAuthorityKeySha256 || '')) &&
    SHA256_RE.test(String(managed.volumeIdentityDigest || ''))),
  'b1_managed_product_pass_projection_invalid')

  const providerAudit = record(report.providerAudit)
  staticProblem(problems, Boolean(providerAudit && providerAudit.configured === true &&
    providerAudit.challengePublished === true && providerAudit.receiptVerified === true &&
    SHA256_RE.test(String(providerAudit.challengeDigest || '')) &&
    SHA256_RE.test(String(providerAudit.receiptSha256 || '')) &&
    positiveSafeInteger(providerAudit.minimumObservedProviderRequestCount) &&
    providerAudit.exactObservedProviderRequestCount ===
      providerAudit.minimumObservedProviderRequestCount &&
    providerAudit.providerRequestCount === providerAudit.exactObservedProviderRequestCount &&
    providerAudit.scannedRequestBodyCount === providerAudit.providerRequestCount &&
    providerAudit.completeIdentifierFindingCount === 0 &&
    providerAudit.forbiddenHostMaterialFindingCount === 0 &&
    positiveSafeInteger(providerAudit.providerSafeSemanticBlockCount) &&
    providerAudit.providerSafeSemanticValidationCount ===
      providerAudit.providerSafeSemanticBlockCount &&
    providerAudit.providerSafeSemanticBlockCount >= report.workflow?.fundsTurnCount),
  'b1_provider_audit_pass_projection_invalid')

  const typed = record(report.typedLocalDisplay)
  staticProblem(problems, Boolean(typed && typed.required === true &&
    typed.status === 'PASS' && typed.contract === 'analytix.typed-local-display.v1' &&
    typed.fullModeExactValue === true && typed.maskedModeLocalOnly === true &&
    typed.acceptedSlotBindingVerified === true &&
    typed.ordinaryRendererCompletePIIAbsent === true &&
    typed.forbiddenGenericProviderChannelsZero === true &&
    typed.invalidatedAfterAuthorityChange === true && typed.reason === '' &&
    typed.blocksMilestoneB === false),
  'b1_typed_local_display_pass_projection_invalid')

  const preview = record(report.directSourcePreview)
  staticProblem(problems, Boolean(preview && preview.required === true &&
    preview.status === 'PASS' && preview.contract === 'analytix.direct-source-preview.v1' &&
    preview.fullModeExactValue === true && preview.maskedModeLocalOnly === true &&
    preview.providerRequestAbsent === true &&
    preview.claimReceiptFinalGateAbsent === true &&
    preview.invalidatedAfterAuthorityChange === true && preview.reason === '' &&
    preview.blocksMilestoneB === false),
  'b1_direct_source_preview_pass_projection_invalid')

  const provider = record(report.provider)
  staticProblem(problems, Boolean(provider && provider.configured === true &&
    provider.normalLocalProviderSetupObserved === true &&
    provider.credentialAuthority === LOCAL_PROVIDER_AUTHORITY &&
    !Object.hasOwn(provider, 'normalHubLoginObserved') &&
    localCredentialEvidencePassed(provider.localCredentialEvidence) &&
    provider.credentialAuthorityBound === true &&
    provider.automatedTestBootstrapObserved === false &&
    typeof provider.family === 'string' && provider.family.length > 0 && provider.family !== 'analytix-hub' &&
    provider.modelHash === createHash('sha256').update('deepseek-v4-flash').digest('hex') &&
    SHA256_RE.test(String(provider.baseUrlOriginHash || '')) &&
    provider.localCredentialEvidence.protectedStoreOwnerPrivate === true &&
    !Object.hasOwn(provider, 'credentialFilesOwnerPrivate')),
  'b1_provider_pass_projection_invalid')

  const isolation = record(report.isolation)
  staticProblem(problems, Boolean(isolation && isolation.trustedCacheTmpdir === true &&
    isolation.isolatedHomeUsed === true && isolation.isolatedUserDataUsed === true &&
    isolation.sandboxRemoved === true && isolation.productRunRemoved === true &&
    isolation.productDataOutsideCache === true && isolation.productOwnerStable === true &&
    isolation.productVolumeRevalidated === true &&
    isolation.userDataRuntimeSeparated === true &&
    isolation.externalCaseOutsideSandbox === true &&
    isolation.ordinaryCodeWorkspaceCleaned === true &&
    isolation.caseBindingRestored === true &&
    isolation.protectedSourceAliasCleaned === true),
  'b1_isolation_cleanup_pass_projection_invalid')

  const publicSeams = record(report.publicSeams)
  staticProblem(problems, Boolean(publicSeams &&
    b1PublicSeamPassed(publicSeams.firstLaunch) &&
    b1PublicSeamPassed(publicSeams.snapshotOne) &&
    b1PublicSeamPassed(publicSeams.snapshotTwo) &&
    b1PublicSeamPassed(publicSeams.relaunch) &&
    publicSeams.exactPackagedRuntimeExecutable === true),
  'b1_public_seam_pass_projection_invalid')

  const workflow = record(report.workflow)
  const workflowTyped = record(workflow?.typedLocalDisplay)
  const workflowPreview = record(workflow?.directSourcePreview)
  const workflowRevocation = record(workflow?.typedLocalDisplayRevocation)
  staticProblem(problems, Boolean(workflow && workflow.sameThread === true &&
    workflow.ordinaryTurnCount === 5 && workflow.fundsTurnCount === 6 &&
    workflow.blockedCaseTurnCount === 2 && positiveSafeInteger(workflow.compactionCount) &&
    workflow.exactRecovery === true && workflow.exactSubagentRecovery === true &&
    workflow.typedLocalDisplayRehydrated === true &&
    workflow.stableEntityDisplayAcrossSnapshotAndRestart === true &&
    workflow.completedRunnableFlow === true &&
    workflowTyped && workflowTyped.fullModeExactValue === true &&
    workflowTyped.maskedModeLocalOnly === true &&
    workflowTyped.exactValueOnlyInTypedSink === true &&
    workflowTyped.acceptedSlotBindingVerified === true &&
    workflowTyped.forbiddenGenericProviderChannelsZero === true &&
    record(workflowTyped.directSourcePreview)?.status === 'PASS' &&
    record(workflowTyped.directSourcePreview)?.paginationObserved === true &&
    record(workflowTyped.directSourcePreview)?.invalidatedAfterAuthorityChange === true &&
    workflowPreview && workflowPreview.fullModeExactValue === true &&
    workflowPreview.maskedModeLocalOnly === true &&
    workflowPreview.paginationObserved === true &&
    workflowPreview.providerRequestAbsent === true &&
    workflowPreview.agentTurnAbsent === true && workflowPreview.mcpCallAbsent === true &&
    workflowPreview.claimReceiptFinalGateAbsent === true &&
    workflowPreview.invalidatedAfterAuthorityChange === true &&
    workflowPreview.revokedAfterCaseChange === true &&
    workflowRevocation && workflowRevocation.retainedAfterOrdinaryTurn === true &&
    workflowRevocation.retainedSnapshotOneReResolved === true &&
    workflowRevocation.snapshotPreviewChanged === true &&
    workflowRevocation.observationKind === 'accepted_slot_display' &&
    positiveSafeInteger(workflowRevocation.slotCount) &&
    b1PublicPIIScanPassed(workflow.publicPIIScan)),
  'b1_workflow_pass_projection_invalid')

  const exit = record(report.exit)
  staticProblem(problems, Boolean(exit && exit.firstNormal === true &&
    exit.finalNormal === true && exit.residualProcessCount === 0),
  'b1_exit_pass_projection_invalid')
  return problems
}

function validA0WorktreeBinding(value, harness) {
  return exactKeys(value, ['ok', 'blocker', 'matched', 'current', 'packaged']) &&
    value.ok === true && value.blocker === '' && value.matched === true &&
    exactKeys(value.current, ['digest', 'classification']) &&
    exactKeys(value.packaged, ['digest', 'classification', 'authorityClassification']) &&
    value.current.digest === harness.sourceClosureSnapshotDigest &&
    value.packaged.digest === harness.packagedSourceClosureSnapshotDigest &&
    value.current.classification === 'clean' &&
    value.packaged.classification === 'clean' &&
    [
      'development_clean_non_publishable',
      'controlled_release_clean_candidate_non_publishable'
    ].includes(value.packaged.authorityClassification)
}

function validateA0Revalidation(value, app) {
  const problems = []
  const phases = [
    ['beforeFirstLaunch', 'before-first-launch'],
    ['afterFirstExit', 'after-first-exit'],
    ['beforeSecondLaunch', 'before-second-launch'],
    ['afterFinalExit', 'after-final-exit']
  ]
  staticProblem(problems, exactKeys(value, phases.map(([key]) => key)),
    'a0_artifact_revalidation_schema_invalid')
  if (!exactKeys(value, phases.map(([key]) => key))) return problems
  for (const [key, phase] of phases) {
    const item = value[key]
    staticProblem(problems, exactKeys(item, [
      'phase', 'ok', 'blocker', 'codeSignatureVerified', 'stableFields',
      'worktreeStable', 'observedAuthoritySha256', 'observedAuthorityDigest',
      'observedExecutableSha256', 'observedRuntimeServerSha256',
      'observedAppAsarSha256'
    ]) && item.phase === phase && item.ok === true && item.blocker === '' &&
      item.codeSignatureVerified === true && item.stableFields === true &&
      item.worktreeStable === true &&
      item.observedAuthoritySha256 === app.authoritySha256 &&
      item.observedAuthorityDigest === app.authorityDigest &&
      item.observedExecutableSha256 === app.executableSha256 &&
      item.observedRuntimeServerSha256 === app.runtimeServerSha256 &&
      item.observedAppAsarSha256 === app.appAsarSha256,
    'a0_artifact_revalidation_failed')
  }
  return [...new Set(problems)]
}

function validateA0(report, context) {
  const problems = []
  staticProblem(problems, hasOnlyKnownKeys(report, A0_ROOT_KEYS), 'a0_report_unknown_property')
  staticProblem(problems,
    report.schemaVersion === 1 && report.id === 'runtime-go-packaged-milestone-a' &&
      report.stage === 'packaged-general-agent-milestone-a',
  'a0_report_schema_or_lane_identity_invalid')
  staticProblem(problems, validGeneratedAt(report.generatedAt), 'a0_generated_at_invalid')
  staticProblem(problems,
    report.sourceCommit === context.source.head &&
      report.testHarnessCommit === context.source.head,
  'a0_source_identity_mismatch')
  staticProblem(problems,
    report.status === 'passed' && report.passed === true &&
      report.credentialSecretsRecorded === false &&
      report.rawProviderPayloadRecorded === false &&
      report.syntheticProviderUsed === false && report.localProviderUsed === false &&
      report.directRuntimeTurnDriverUsed === false,
  'a0_status_or_execution_class_invalid')
  staticProblem(problems,
    report.executionBlocker === '' &&
      !Object.hasOwn(report, 'executionBlockerClassification') &&
      Array.isArray(report.blockers) && report.blockers.length === 0 &&
      Array.isArray(report.missingExternalInputs) &&
      report.missingExternalInputs.length === 0,
  'a0_pass_blocker_projection_invalid')
  problems.push(...validateHarness(report.harness, context.a0Harness, 'a0'))
  staticProblem(problems, hasOnlyKnownKeys(report.app, A0_APP_KEYS) &&
    report.app.ok === true && report.app.blocked === false && report.app.blocker === '' &&
    report.app.targetKey === 'darwin-arm64' &&
    report.app.sourceCommit === context.source.head &&
    SHA256_RE.test(String(report.app.appPathHash || '')) &&
    [
      report.app.authoritySha256, report.app.authorityDigest,
      report.app.executableSha256, report.app.runtimeServerSha256,
      report.app.appAsarSha256
    ].every((value) => SHA256_RE.test(String(value || ''))) &&
    report.app.codeSignatureVerified === true &&
    report.app.packagedSkillSourceMatched === true &&
    report.app.packagedScheduleSourceContractBound === true &&
    report.app.packagedScheduleStrictEmptyInputSchemaBound === true &&
    report.app.packagedScheduleNonMutatingListImplementationBound === true &&
    report.app.bundledRuntimeGoSourcePresent === false &&
    validA0WorktreeBinding(report.app.worktreeSnapshotBinding, report.harness),
  'a0_packaged_artifact_binding_invalid')
  problems.push(...validateA0Revalidation(report.artifactRevalidation, report.app || {}))
  problems.push(...validateChecks(
    report.checks, A0_REQUIRED_CHECK_IDS, A0_AUXILIARY_CHECK_IDS, 'passed', 'a0'
  ))
  problems.push(...validateA0StageSnapshots(report))
  problems.push(...validateA0AdmissionCriticalProjection(report))
  staticProblem(problems,
    Array.isArray(report.failedCheckIds) && report.failedCheckIds.length === 0 &&
    Array.isArray(report.skippedCheckIds) && report.skippedCheckIds.length === 0 &&
    Array.isArray(report.liveBlockedCheckIds) && report.liveBlockedCheckIds.length === 0 &&
    Array.isArray(report.failureCheckIds) && report.failureCheckIds.length === 0 &&
    report.failureCheckIdsSemantics === 'all_non_pass',
  'a0_nonpass_projection_invalid')
  return [...new Set(problems)]
}

function validateB1PhaseLanes(value) {
  const problems = []
  staticProblem(problems, exactKeys(value, Object.keys(B1_PHASE_LANES)),
    'b1_phase_lane_set_invalid')
  if (!exactKeys(value, Object.keys(B1_PHASE_LANES))) return problems
  for (const [laneId, expected] of Object.entries(B1_PHASE_LANES)) {
    const lane = value[laneId]
    staticProblem(problems, exactKeys(lane, [
      'phaseOwner', 'dependencies', 'status', 'blocker', 'executed', 'not_executed'
    ]) && lane.phaseOwner === expected.phaseOwner &&
      canonicalJSON(lane.dependencies) === canonicalJSON(expected.dependencies) &&
      lane.status === 'PASS' && lane.blocker === '' && lane.executed === true &&
      lane.not_executed === false,
    `b1_phase_${laneId}_invalid`)
  }
  return [...new Set(problems)]
}

function validateB1Artifact(value, context, suffix) {
  const problems = []
  staticProblem(problems, hasOnlyKnownKeys(value, B1_ARTIFACT_KEYS) &&
    value.ok === true && value.blocked === false && value.blocker === '' &&
    value.targetKey === 'darwin-arm64' && value.sourceCommit === context.source.head &&
    SHA256_RE.test(String(value.appPathHash || '')) &&
    [
      value.authoritySha256, value.authorityDigest, value.executableSha256,
      value.runtimeServerSha256, value.appAsarSha256, value.worktreeSnapshotDigest
    ].every((item) => SHA256_RE.test(String(item || ''))) &&
    value.codeSignatureVerified === true && value.fundsPluginArtifactBound === true,
  `b1_${suffix}_artifact_binding_invalid`)
  return problems
}

function validateB1(report, context) {
  const problems = []
  staticProblem(problems, hasOnlyKnownKeys(report, B1_ROOT_KEYS), 'b1_report_unknown_property')
  staticProblem(problems,
    report.schemaVersion === 1 && report.id === 'runtime-go-packaged-milestone-b' &&
      report.stage === 'packaged-real-duckdb-funds-analysis-milestone-b' &&
      report.acceptanceClass ===
        'formal-packaged-typed-local-display-external-owner-isolated-real-case',
  'b1_report_schema_or_lane_identity_invalid')
  staticProblem(problems, validGeneratedAt(report.generatedAt), 'b1_generated_at_invalid')
  staticProblem(problems,
    report.sourceCommit === context.source.head && report.harnessCommit === context.source.head,
  'b1_source_identity_mismatch')
  staticProblem(problems,
    report.status === 'PASS' && report.passed === true &&
      report.mockUsed === false && report.fixtureUsed === false &&
      report.syntheticProviderUsed === false &&
      report.directRuntimeTurnDriverUsed === false &&
      report.directSnapshotIPCUsed === false && report.arbitrarySQLUsed === false &&
      report.rendererDatabasePathReceived === false && report.completePIIRecorded === false &&
      !Object.hasOwn(report, 'diagnostic'),
  'b1_status_or_execution_class_invalid')
  staticProblem(problems,
    report.runtimeBlocker === '' && !Object.hasOwn(report, 'runtimeFailure'),
  'b1_pass_blocker_projection_invalid')
  problems.push(...validateHarness(report.harness, context.b1Harness, 'b1'))
  problems.push(...validateB1PhaseLanes(report.phaseLanes))
  problems.push(...validateB1Artifact(report.artifact, context, 'initial'))
  problems.push(...validateB1Artifact(report.artifactFinal, context, 'final'))
  staticProblem(problems,
    canonicalJSON(report.artifact) === canonicalJSON(report.artifactFinal),
  'b1_artifact_changed_during_run')
  staticProblem(problems, exactKeys(report.app, ['targetKey', 'appPathHash']) &&
    report.app.targetKey === 'darwin-arm64' &&
    report.app.appPathHash === report.artifact?.appPathHash,
  'b1_app_identity_invalid')
  problems.push(...validateChecks(report.checks, B1_REQUIRED_CHECK_IDS, [], 'PASS', 'b1'))
  problems.push(...validateB1AdmissionCriticalProjection(report))
  staticProblem(problems,
    Array.isArray(report.failureCheckIds) && report.failureCheckIds.length === 0 &&
    Array.isArray(report.blockedCheckIds) && report.blockedCheckIds.length === 0 &&
    Array.isArray(report.unverifiedCheckIds) && report.unverifiedCheckIds.length === 0,
  'b1_nonpass_projection_invalid')
  return [...new Set(problems)]
}

function exactAppAsarBinding(exactFormalArtifact) {
  const artifact = exactFormalArtifact?.artifact
  if (exactFormalArtifact?.provided !== true ||
      exactFormalArtifact?.engineeringAdmission !== true ||
      exactFormalArtifact?.passed !== true || !artifact ||
      !SHA256_RE.test(String(artifact.sha256 || '')) ||
      !Number.isSafeInteger(artifact.byteLength) || artifact.byteLength <= 0) {
    return null
  }
  if (Array.isArray(artifact.components)) {
    if (artifact.components.length !== 1 ||
        !exactKeys(artifact.components[0], ['artifactEntry', 'sha256', 'byteLength'])) {
      return null
    }
    const component = artifact.components[0]
    return /(?:^|\/)app\.asar$/u.test(String(component.artifactEntry || '')) &&
      SHA256_RE.test(String(component.sha256 || '')) &&
      Number.isSafeInteger(component.byteLength) && component.byteLength > 0
      ? { sha256: component.sha256, byteLength: component.byteLength }
      : null
  }
  return /(?:^|\/)app\.asar$/u.test(String(artifact.path || exactFormalArtifact.artifactEntry || ''))
    ? { sha256: artifact.sha256, byteLength: artifact.byteLength }
    : null
}

function revalidateExactArtifactLegalInventory(exactFormalArtifact) {
  const artifactPath = exactFormalArtifact?.artifact?.path
  if (typeof artifactPath !== 'string' || !artifactPath || !isAbsolute(artifactPath)) {
    return {
      problems: ['exact_artifact_legal_inventory_binding_invalid'],
      current: null
    }
  }
  const current = inspectExactArtifactLegalInventory({ artifact: artifactPath })
  const recordedArtifact = exactFormalArtifact?.artifact
  const currentArtifact = current?.artifact
  const matches = exactFormalArtifact?.provided === true &&
    exactFormalArtifact?.passed === true &&
    exactFormalArtifact?.engineeringAdmission === true &&
    current?.provided === true && current?.passed === true &&
    current?.engineeringAdmission === true &&
    canonicalJSON(currentArtifact) === canonicalJSON(recordedArtifact)
  return {
    problems: matches ? [] : ['exact_artifact_changed_after_legal_audit'],
    current
  }
}

function packagedAuthorityPath(exactFormalArtifact) {
  const rawPath = exactFormalArtifact?.artifact?.path
  if (typeof rawPath !== 'string' || !rawPath || !isAbsolute(rawPath)) return ''
  const artifactPath = resolve(rawPath)
  if (/\.app$/u.test(artifactPath)) {
    return join(artifactPath, 'Contents', 'Resources', 'runtime', PACKAGED_AUTHORITY_FILE)
  }
  if (basename(artifactPath) === 'app.asar') {
    return join(dirname(artifactPath), 'runtime', PACKAGED_AUTHORITY_FILE)
  }
  return ''
}

function darwinPackagedContext(exactFormalArtifact) {
  const appPath = resolve(String(exactFormalArtifact?.artifact?.path || ''))
  if (!isAbsolute(appPath) || !/\.app$/u.test(appPath)) return null
  return {
    appPath,
    appOutDir: dirname(appPath),
    electronPlatformName: 'darwin',
    arch: 'arm64',
    packager: {
      appInfo: { productFilename: basename(appPath, '.app') },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
}

function stableArtifactFileIdentity(filePath, maximumBytes = 512 * 1024 * 1024) {
  const requestedPath = resolve(filePath)
  const before = lstatSync(requestedPath, { bigint: true })
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1n ||
      realpathSync(requestedPath) !== requestedPath || before.size <= 0n ||
      before.size > BigInt(maximumBytes)) {
    throw new Error('packaged_artifact_component_invalid')
  }
  const fd = openSync(requestedPath, constants.O_RDONLY | Number(constants.O_NOFOLLOW || 0))
  try {
    const beforeFd = fstatSync(fd, { bigint: true })
    const bytes = readFileSync(fd)
    const afterFd = fstatSync(fd, { bigint: true })
    const after = lstatSync(requestedPath, { bigint: true })
    if (!stableSingleLinkIdentity(before, beforeFd) ||
        !stableSingleLinkIdentity(beforeFd, afterFd) ||
        !stableSingleLinkIdentity(afterFd, after) ||
        BigInt(bytes.length) !== beforeFd.size) {
      throw new Error('packaged_artifact_component_changed_during_read')
    }
    return { sha256: sha256(bytes), byteLength: bytes.length }
  } finally {
    closeSync(fd)
  }
}

function validatePackagedAuthority(exactFormalArtifact, source, appAsar) {
  const problems = []
  const authorityPath = packagedAuthorityPath(exactFormalArtifact)
  if (!authorityPath || !appAsar) {
    return { problems: ['packaged_build_authority_path_or_artifact_invalid'], evidence: null }
  }
  let authorityFile
  let context
  try {
    authorityFile = readStableAuthorityFile(authorityPath)
    context = darwinPackagedContext(exactFormalArtifact)
    if (!context) throw new Error('packaged_build_authority_context_invalid')
  } catch (error) {
    return { problems: [safeReadFailure(error)], evidence: null }
  }
  const authority = authorityFile.authority
  const authorityRecord = record(authority)
  const authoritySchemaValid = Boolean(authorityRecord &&
    packagedAuthority().isPackagedBuildAuthorityV2(authorityRecord))
  staticProblem(problems, authoritySchemaValid, 'packaged_build_authority_schema_invalid')
  staticProblem(problems,
    authoritySchemaValid && authorityRecord.sourceCommit === source.head &&
    authorityRecord.targetKey === 'darwin-arm64' &&
    authorityRecord.publishable === false && authorityRecord.releaseEligible === false &&
    authorityRecord.publicationReceiptIssued === false,
  'packaged_build_authority_source_or_disposition_invalid')
  const worktree = authorityRecord?.worktreeSnapshot
  staticProblem(problems,
    authoritySchemaValid && packagedAuthority().isPackagedWorktreeSnapshotV1(worktree),
  'packaged_build_authority_worktree_snapshot_invalid')
  staticProblem(problems,
    authoritySchemaValid && worktree?.sourceCommit === source.head && worktree?.state === 'clean' &&
      worktree.dirty === false && [
        'development_clean_non_publishable',
        'controlled_release_clean_candidate_non_publishable'
      ].includes(authorityRecord.classification),
  'packaged_build_authority_worktree_not_clean')
  let verifiedArtifacts = null
  let currentExecutable = null
  let currentRuntimeServer = null
  try {
    if (!authoritySchemaValid) throw new Error('packaged_build_authority_schema_invalid')
    // This owner understands the signing-invariant native payload rather than
    // confusing a post-sign file hash with the pre-sign authority binding. It
    // also rechecks app.asar, funds, staged payload, and the Electron fuse set.
    verifiedArtifacts = packagedAuthority().verifyPackagedBuildAuthorityArtifacts(
      context,
      authorityRecord
    )
    currentExecutable = stableArtifactFileIdentity(
      join(context.appPath, 'Contents', 'MacOS', 'analytix')
    )
    currentRuntimeServer = stableArtifactFileIdentity(
      join(context.appPath, 'Contents', 'Resources', 'runtime-go', 'bin', 'runtime-server')
    )
  } catch {
    problems.push('packaged_build_authority_artifact_verification_failed')
  }
  staticProblem(problems,
    authoritySchemaValid && verifiedArtifacts &&
      authorityRecord.artifacts.appAsar.sha256 === appAsar.sha256 &&
      authorityRecord.artifacts.appAsar.byteLength === appAsar.byteLength &&
      canonicalJSON(verifiedArtifacts.appAsar) === canonicalJSON(appAsar),
  'packaged_build_authority_app_asar_identity_invalid')
  return {
    problems: [...new Set(problems)],
    evidence: {
      sha256: authorityFile.sha256,
      byteLength: authorityFile.byteLength,
      digest: String(authorityRecord?.authorityDigest || ''),
      worktreeSnapshotDigest: String(authorityRecord?.worktreeSnapshot?.snapshotDigest || ''),
      classification: String(authorityRecord?.classification || ''),
      executableIdentitySha256:
        sha256(canonicalJSON(authorityRecord?.artifacts?.executable || null)),
      runtimeServerIdentitySha256:
        sha256(canonicalJSON(authorityRecord?.artifacts?.runtimeServer || null)),
      fundsPluginIdentitySha256:
        sha256(canonicalJSON(authorityRecord?.artifacts?.fundsPlugin || null)),
      currentExecutableSha256: currentExecutable?.sha256 || '',
      currentRuntimeServerSha256: currentRuntimeServer?.sha256 || ''
    }
  }
}

function notExecutedProjection() {
  return {
    contract: FORMAL_EVIDENCE_CONTRACT,
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
  }
}

function failedUnreadableProjection(code) {
  return {
    contract: FORMAL_EVIDENCE_CONTRACT,
    provided: true,
    accepted: false,
    statuses: { electron: 'failed', package: 'failed', a0: 'failed', b1: 'failed' },
    reports: null,
    artifactBinding: null,
    problems: [code]
  }
}

function safeReadFailure(error) {
  const message = error instanceof Error ? error.message : ''
  return /^[a-z0-9_]+$/u.test(message)
    ? message
    : 'formal_evidence_bundle_unreadable'
}

function currentSourceValid(source) {
  return source && exactKeys(source, ['head', 'tree']) &&
    GIT_COMMIT_RE.test(String(source.head || '')) &&
    GIT_COMMIT_RE.test(String(source.tree || ''))
}

function readFormalEvidenceBundle({ repoRoot, bundleDirectory, source }) {
  const stableDirectory = stableFormalEvidenceDirectory(bundleDirectory)
  const a0File = readStableFormalReport(stableDirectory, FORMAL_REPORT_FILES.a0)
  const b1File = readStableFormalReport(stableDirectory, FORMAL_REPORT_FILES.b1)
  if (!stableFormalEvidenceDirectoryIdentity(stableDirectory)) {
    throw new Error('formal_evidence_directory_changed_during_read')
  }
  const context = {
    source,
    a0Harness: currentHarnessEntries(repoRoot, A0_HARNESS_FILES),
    b1Harness: currentHarnessEntries(repoRoot, B1_HARNESS_FILES)
  }
  return {
    a0File,
    b1File,
    a0Problems: validateA0(a0File.report, context),
    b1Problems: validateB1(b1File.report, context)
  }
}

function crossReportIdentityProblems(a0File, b1File, source) {
  const problems = []
  staticProblem(problems,
    a0File.sha256 !== b1File.sha256 && a0File.identity !== b1File.identity,
  'formal_evidence_duplicate_report')
  const a0Artifact = a0File.report.app || {}
  const b1Artifact = b1File.report.artifact || {}
  const identityFields = [
    'appPathHash', 'authoritySha256', 'authorityDigest', 'executableSha256',
    'runtimeServerSha256', 'appAsarSha256'
  ]
  staticProblem(problems,
    identityFields.every((field) => a0Artifact[field] === b1Artifact[field]) &&
      a0Artifact.worktreeSnapshotBinding?.packaged?.digest ===
        b1Artifact.worktreeSnapshotDigest,
  'formal_evidence_cross_lane_artifact_identity_mismatch')
  staticProblem(problems,
    a0File.report.sourceCommit === b1File.report.sourceCommit &&
      a0File.report.sourceCommit === source.head,
  'formal_evidence_cross_lane_source_mismatch')
  return [...new Set(problems)]
}

function reportReferences(a0File, b1File) {
  return {
    a0: {
      fileName: FORMAL_REPORT_FILES.a0,
      sha256: a0File.sha256,
      byteLength: a0File.byteLength,
      lane: 'ordinary-a0'
    },
    b1: {
      fileName: FORMAL_REPORT_FILES.b1,
      sha256: b1File.sha256,
      byteLength: b1File.byteLength,
      lane: 'typed-local-b1'
    }
  }
}

function preflightProjection({ provided, status, passed, reports = null, problems = [] }) {
  return {
    contract: FORMAL_EVIDENCE_CONTRACT,
    phase: 'pre_lock_read_only',
    provided,
    status,
    passed,
    formalExecution: 'not_executed',
    receiptCreated: false,
    exactArtifactBinding: 'not_evaluated',
    statuses: {
      electron: 'not_executed',
      package: 'not_executed',
      a0: 'not_executed',
      b1: 'not_executed'
    },
    reports,
    problems: [...new Set(problems)]
  }
}

export function preflightRuntimeGoFormalEvidence({
  repoRoot,
  bundleDirectory = '',
  source
} = {}) {
  if (!bundleDirectory) {
    return preflightProjection({
      provided: false,
      status: 'not_requested',
      passed: false
    })
  }
  if (!currentSourceValid(source)) {
    return preflightProjection({
      provided: true,
      status: 'failed',
      passed: false,
      problems: ['formal_evidence_current_source_invalid']
    })
  }
  let bundle
  try {
    bundle = readFormalEvidenceBundle({ repoRoot, bundleDirectory, source })
  } catch (error) {
    return preflightProjection({
      provided: true,
      status: 'failed',
      passed: false,
      problems: [safeReadFailure(error)]
    })
  }
  const crossProblems = crossReportIdentityProblems(
    bundle.a0File, bundle.b1File, source
  )
  const problems = [
    ...bundle.a0Problems,
    ...bundle.b1Problems,
    ...crossProblems
  ]
  return preflightProjection({
    provided: true,
    status: problems.length === 0 ? 'passed' : 'failed',
    passed: problems.length === 0,
    reports: reportReferences(bundle.a0File, bundle.b1File),
    problems
  })
}

export function evaluateRuntimeGoFormalEvidence({
  repoRoot,
  bundleDirectory = '',
  source,
  exactFormalArtifact
} = {}) {
  if (!bundleDirectory) return notExecutedProjection()
  if (!currentSourceValid(source)) {
    return failedUnreadableProjection('formal_evidence_current_source_invalid')
  }
  let bundle
  try {
    // Deliberately re-read and revalidate after the artifact legal audit.  A
    // successful pre-lock preflight is never reused as formal evidence.
    bundle = readFormalEvidenceBundle({ repoRoot, bundleDirectory, source })
  } catch (error) {
    return failedUnreadableProjection(safeReadFailure(error))
  }
  const { a0File, b1File, a0Problems, b1Problems } = bundle
  const crossProblems = crossReportIdentityProblems(a0File, b1File, source)
  const appAsar = exactAppAsarBinding(exactFormalArtifact)
  staticProblem(crossProblems, Boolean(appAsar), 'exact_artifact_identity_or_size_invalid')
  const authorityValidation = validatePackagedAuthority(
    exactFormalArtifact, source, appAsar
  )
  crossProblems.push(...authorityValidation.problems)
  // The legal inventory owner performs the final whole-artifact reread after
  // the selected authority components have also been verified.
  const exactArtifactRevalidation = revalidateExactArtifactLegalInventory(
    exactFormalArtifact
  )
  crossProblems.push(...exactArtifactRevalidation.problems)
  const a0Artifact = a0File.report.app || {}
  const b1Artifact = b1File.report.artifact || {}
  staticProblem(crossProblems,
    authorityValidation.evidence &&
      a0Artifact.authoritySha256 === authorityValidation.evidence.sha256 &&
      b1Artifact.authoritySha256 === authorityValidation.evidence.sha256 &&
      a0Artifact.authorityDigest === authorityValidation.evidence.digest &&
      b1Artifact.authorityDigest === authorityValidation.evidence.digest &&
      a0Artifact.executableSha256 === authorityValidation.evidence.currentExecutableSha256 &&
      b1Artifact.executableSha256 === authorityValidation.evidence.currentExecutableSha256 &&
      a0Artifact.runtimeServerSha256 ===
        authorityValidation.evidence.currentRuntimeServerSha256 &&
      b1Artifact.runtimeServerSha256 ===
        authorityValidation.evidence.currentRuntimeServerSha256 &&
      a0Artifact.worktreeSnapshotBinding?.packaged?.digest ===
        authorityValidation.evidence.worktreeSnapshotDigest &&
      b1Artifact.worktreeSnapshotDigest ===
        authorityValidation.evidence.worktreeSnapshotDigest &&
      a0Artifact.worktreeSnapshotBinding?.packaged?.authorityClassification ===
        authorityValidation.evidence.classification,
  'formal_evidence_packaged_authority_mismatch')
  staticProblem(crossProblems,
    appAsar && a0Artifact.appAsarSha256 === appAsar.sha256 &&
      b1Artifact.appAsarSha256 === appAsar.sha256,
  'formal_evidence_exact_artifact_mismatch')
  const sharedIdentityProblems = crossProblems.filter((problem) =>
    problem !== 'formal_evidence_duplicate_report'
  )
  const a0Passed = a0Problems.length === 0 && sharedIdentityProblems.length === 0
  const b1Passed = b1Problems.length === 0 && sharedIdentityProblems.length === 0
  const packagePassed = a0Passed && b1Passed && crossProblems.length === 0 &&
    a0File.report.checks.some((item) =>
      item.id === 'formal-packaged-artifact' && item.status === 'passed'
    ) && b1File.report.checks.some((item) =>
      item.id === 'formal-packaged-artifact' && item.status === 'PASS'
    )
  const electronPassed = packagePassed && ELECTRON_CHECK_IDS.every((id) =>
    a0File.report.checks.some((item) => item.id === id && item.status === 'passed') &&
      b1File.report.checks.some((item) => item.id === id && item.status === 'PASS')
  )
  const problems = [...new Set([...a0Problems, ...b1Problems, ...crossProblems])]
  const statuses = {
    electron: electronPassed ? 'passed' : 'failed',
    package: packagePassed ? 'passed' : 'failed',
    a0: a0Passed ? 'passed' : 'failed',
    b1: b1Passed ? 'passed' : 'failed'
  }
  return {
    contract: FORMAL_EVIDENCE_CONTRACT,
    provided: true,
    accepted: Object.values(statuses).every((status) => status === 'passed'),
    statuses,
    reports: reportReferences(a0File, b1File),
    artifactBinding: appAsar
      ? {
          exactArtifactSha256: exactFormalArtifact.artifact.sha256,
          exactArtifactByteLength: exactFormalArtifact.artifact.byteLength,
          exactArtifactRevalidated: exactArtifactRevalidation.problems.length === 0,
          appAsarSha256: appAsar.sha256,
          appAsarByteLength: appAsar.byteLength,
          authoritySha256: a0Artifact.authoritySha256,
          authorityDigest: a0Artifact.authorityDigest,
          authorityClassification: authorityValidation.evidence?.classification || '',
          authorityByteLength: authorityValidation.evidence?.byteLength || 0,
          executableSha256: a0Artifact.executableSha256,
          runtimeServerSha256: a0Artifact.runtimeServerSha256,
          executableAuthorityIdentitySha256:
            authorityValidation.evidence?.executableIdentitySha256 || '',
          runtimeServerAuthorityIdentitySha256:
            authorityValidation.evidence?.runtimeServerIdentitySha256 || '',
          fundsPluginAuthorityIdentitySha256:
            authorityValidation.evidence?.fundsPluginIdentitySha256 || '',
          fundsPluginArtifactBound: b1Artifact.fundsPluginArtifactBound === true
        }
      : null,
    problems
  }
}

export const runtimeGoFormalEvidenceContract = Object.freeze({
  contract: FORMAL_EVIDENCE_CONTRACT,
  reportFiles: FORMAL_REPORT_FILES,
  a0RequiredCheckIds: A0_REQUIRED_CHECK_IDS,
  a0AuxiliaryCheckIds: A0_AUXILIARY_CHECK_IDS,
  b1RequiredCheckIds: B1_REQUIRED_CHECK_IDS,
  a0HarnessFiles: A0_HARNESS_FILES,
  b1HarnessFiles: B1_HARNESS_FILES,
  b1PhaseLanes: B1_PHASE_LANES
})

export const runtimeGoFormalEvidenceTestInternals = Object.freeze({
  stableIdentity,
  stableSingleLinkIdentity
})
