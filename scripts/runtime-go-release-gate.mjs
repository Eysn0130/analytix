#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'
import {
  canonicalJSONString,
  createReceiptRun,
  gateExitCode,
  loadOpenSpecTaskLedger,
  sha256,
  sourceIdentity,
  writeAndRevalidateReceipt
} from './runtime-go-rc-receipt.mjs'
import {
  evaluateRuntimeGoFormalEvidence,
  preflightRuntimeGoFormalEvidence
} from './runtime-go-formal-evidence.mjs'
import { closeGitEnvironment } from './upstream-source-audit.mjs'
import { evaluateReleaseExecution, freezeReleaseClosure, sealReleaseExecution, finalizeReleaseExecution } from './runtime-go-release-finalization.mjs'

const repoRoot = process.cwd()
const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
closeGitEnvironment(process.env)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const nodeCommand = process.execPath
const goCommand = process.env.GO || 'go'
const dryRun = args.has('--dry-run')
const forbiddenSkip = args.has('--skip-commands')
const jsonOutput = args.has('--json') || dryRun
const controlPlaneOnly = args.has('--control-plane-only')
const executionOnly = args.has('--execute')
const finalizationOnly = args.has('--finalize')
const gateMode = controlPlaneOnly ? 'control-plane' : executionOnly ? 'execution' : 'release'
const gateId = controlPlaneOnly ? 'runtime-go-rc-control-plane' : 'runtime-go-release-gate'
const openspecChange = 'case-evidence-publication-gate'
const claimCeiling = 'RC admission and validation semantics ready for product closure'
const exactArtifactEnvironmentKey = 'ANALYTIX_EXACT_ARTIFACT_PATH'
const formalEvidenceOptionName = '--formal-evidence-dir'

function singlePathOption(name) {
  const values = []
  let malformed = false
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item === name) {
      const value = rawArgs[index + 1]
      if (!value || value.startsWith('--')) {
        malformed = true
      } else {
        values.push(value)
        index += 1
      }
      continue
    }
    if (item.startsWith(`${name}=`)) {
      const value = item.slice(name.length + 1)
      if (!value) malformed = true
      else values.push(value)
    }
  }
  return {
    provided: values.length > 0,
    value: values[0] || '',
    valid: !malformed && values.length <= 1
  }
}

const formalEvidenceOption = singlePathOption(formalEvidenceOptionName)
const executionSealOption = singlePathOption('--execution-seal')
const closureReportOption = singlePathOption('--closure-report')
const closureTasksOption = singlePathOption('--closure-task-ids')

// Final adjudication branches before the run lock, toolchain inspection,
// formal artifact/OS validation, or any execution-manifest command.
if (finalizationOnly) {
  try {
    if (controlPlaneOnly || executionOnly || dryRun || forbiddenSkip || formalEvidenceOption.provided ||
      closureReportOption.provided || closureTasksOption.provided || !executionSealOption.valid || !executionSealOption.provided ||
      rawArgs.filter((arg) => arg === '--finalize').length !== 1) throw new Error('finalization_options_invalid')
    console.log(JSON.stringify(finalizeReleaseExecution({ repoRoot, sealPath: executionSealOption.value }), null, 2))
    process.exit(0)
  } catch (error) {
    console.log(JSON.stringify({ id: gateId, commandMode: 'finalization', status: 'failed', passed: false,
      commandPassed: false, releaseAuthorized: false,
      reason: /^[a-z_]+$/u.test(error?.message || '') ? error.message : 'finalization_input_unreadable' }, null, 2))
    process.exit(1)
  }
}
const releaseCommandSource = sourceIdentity(repoRoot)

const commands = [
  {
    id: 'preflight-source-inspection',
    command: nodeCommand,
    args: ['./scripts/runtime-go-preflight.mjs', '--rc-control-plane', '--json'],
    cwd: repoRoot,
    captureJson: true,
    reportId: 'runtime-go-preflight',
    heavyweight: false
  },
  {
    id: 'upstream-source-audit',
    command: nodeCommand,
    args: [
      './scripts/upstream-source-audit.mjs',
      '--json',
      '--release-head',
      releaseCommandSource.head,
      '--release-tree',
      releaseCommandSource.tree
    ],
    cwd: repoRoot,
    captureJson: true,
    reportId: 'upstream-source-audit',
    heavyweight: false
  },
  {
    id: 'product-sovereignty',
    command: npmCommand,
    args: ['run', 'scan:product-sovereignty'],
    cwd: repoRoot,
    heavyweight: false
  },
  {
    id: 'typecheck',
    command: npmCommand,
    args: ['run', 'typecheck'],
    cwd: repoRoot,
    heavyweight: true
  },
  {
    id: 'build-runtime',
    command: npmCommand,
    args: ['run', 'build:runtime'],
    cwd: repoRoot,
    heavyweight: true
  },
  {
    id: 'full-root-test',
    command: npmCommand,
    args: ['run', 'test'],
    cwd: repoRoot,
    heavyweight: true
  },
  {
    id: 'go-ordinary-matrix',
    command: goCommand,
    args: ['test', '-p=1', '-count=1', '-timeout=15m', './...'],
    cwd: `${repoRoot}/packages/runtime-go`,
    heavyweight: true
  },
  {
    id: 'go-prod-matrix',
    command: goCommand,
    args: ['test', '-p=1', '-count=1', '-tags', 'analytix_prod', '-timeout=15m', './...'],
    cwd: `${repoRoot}/packages/runtime-go`,
    heavyweight: true
  },
  {
    id: 'go-race-matrix',
    command: goCommand,
    args: ['test', '-p=1', '-count=1', '-race', '-timeout=15m', './...'],
    cwd: `${repoRoot}/packages/runtime-go`,
    heavyweight: true
  },
  {
    id: 'rc-structural-and-performance-suite',
    command: nodeCommand,
    args: ['./scripts/runtime-go-performance-check.mjs', '--rc-suite', '--json', '--gate'],
    cwd: repoRoot,
    captureJson: true,
    reportId: 'runtime-go-rc-performance-suite',
    heavyweight: true,
    logicalChecks: ['structural-performance', 'rc-performance-benchmark']
  },
  {
    id: 'artifact-legal-obligations-audit',
    command: nodeCommand,
    args: ['./scripts/artifact-legal-obligations-audit.mjs', '--json'],
    cwd: repoRoot,
    captureJson: true,
    reportId: 'analytix.artifact-legal-obligations/v1',
    heavyweight: false
  }
]

function externalReleaseAuthorizationProjection() {
  return {
    status: 'not_authorized',
    commercialLicense: false,
    signing: false,
    notarization: false,
    publication: false,
    release: false,
    engineeringBlocking: false
  }
}

function zeroHeavyExecutionCounts() {
  return {
    typecheck: 0,
    build_runtime: 0,
    full_root_test: 0,
    go_ordinary_matrix: 0,
    go_prod_matrix: 0,
    go_race_matrix: 0,
    structural_performance: 0,
    rc_performance_benchmark: 0
  }
}

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function commandEnvironment(item) {
  const environment = { ...process.env }
  if (item.id !== 'artifact-legal-obligations-audit') {
    delete environment[exactArtifactEnvironmentKey]
  }
  return environment
}

function parseJSONObject(output) {
  const text = String(output || '').trim()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    const first = text.indexOf('{')
    const last = text.lastIndexOf('}')
    if (first >= 0 && last > first) {
      try {
        return JSON.parse(text.slice(first, last + 1))
      } catch {
        return null
      }
    }
    return null
  }
}

function worktreeSourceFreeze() {
  const result = spawnSync('git', ['status', '--porcelain=v1', '--untracked-files=all'], {
    cwd: repoRoot,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  const entries = String(result.stdout || '').split(/\r?\n/).filter(Boolean)
  const allowedUserEntries = entries.filter((entry) => entry === ' M AGENTS.md')
  const blockingEntries = entries.filter((entry) => entry !== ' M AGENTS.md')
  return {
    status: result.status === 0 && blockingEntries.length === 0 ? 'passed' : 'failed',
    passed: result.status === 0 && blockingEntries.length === 0,
    allowedUserDirtyPaths: allowedUserEntries.map(() => 'AGENTS.md'),
    blockingEntries
  }
}

function dryRunReport(reason) {
  return {
    schemaVersion: 2,
    id: gateId,
    commandMode: gateMode,
    status: 'skipped',
    passed: false,
    commandPassed: false,
    controlPlaneReady: false,
    productRcGatePassed: false,
    artifactAdmissionGatePassed: false,
    upstreamArtifactAdmissionGatePassed: false,
    releaseAuthorized: false,
    claimCeiling,
    dryRunCannotAuthorizeRelease: true,
    formalEvidence: {
      electron: 'not_executed',
      package: 'not_executed',
      a0: 'not_executed',
      b1: 'not_executed',
      commercialRelease: 'not_authorized'
    },
    formalEvidenceInput: {
      requested: formalEvidenceOption.provided,
      consumed: false
    },
    externalReleaseAuthorization: {
      status: 'not_authorized',
      commercialLicense: false,
      signing: false,
      notarization: false,
      publication: false,
      release: false,
      engineeringBlocking: false
    },
    reason,
    executionManifest: commands.map((item) => ({
      id: item.id,
      command: commandText(item),
      cwd: item.cwd,
      heavyweight: item.heavyweight,
      logicalChecks: item.logicalChecks || [item.id],
      expectedExecutions: 1
    }))
  }
}

if (dryRun || forbiddenSkip || !formalEvidenceOption.valid) {
  console.log(JSON.stringify(dryRunReport(
    !formalEvidenceOption.valid
      ? 'formal evidence option must be supplied exactly once with one directory'
      : forbiddenSkip
        ? 'skip flags cannot authorize the RC control plane'
        : 'dry run; no command or receipt was executed'
  ), null, 2))
  process.exit(1)
}

if ((!controlPlaneOnly && !executionOnly) || (controlPlaneOnly && executionOnly) || executionSealOption.provided ||
  (executionOnly && (!closureReportOption.valid || !closureReportOption.provided || !closureTasksOption.valid || !closureTasksOption.provided ||
    rawArgs.filter((arg) => arg === '--execute').length !== 1))) {
  console.log(JSON.stringify({ id: gateId, status: 'failed', passed: false, commandPassed: false,
    reason: 'choose explicit --execute with closure declaration, or --finalize with the execution seal' }, null, 2))
  process.exit(1)
}

if (controlPlaneOnly && formalEvidenceOption.provided) {
  console.log(JSON.stringify({
    schemaVersion: 2,
    id: gateId,
    commandMode: gateMode,
    status: 'failed',
    passed: false,
    commandPassed: false,
    controlPlaneReady: false,
    productRcGatePassed: false,
    artifactAdmissionGatePassed: false,
    upstreamArtifactAdmissionGatePassed: false,
    releaseAuthorized: false,
    claimCeiling,
    reason: 'formal evidence input is accepted only by the product release gate'
  }, null, 2))
  process.exit(1)
}

const freeze = worktreeSourceFreeze()
if (!freeze.passed) {
  console.log(JSON.stringify({
    schemaVersion: 2,
    id: gateId,
    commandMode: gateMode,
    status: 'failed',
    passed: false,
    commandPassed: false,
    controlPlaneReady: false,
    productRcGatePassed: false,
    artifactAdmissionGatePassed: false,
    upstreamArtifactAdmissionGatePassed: false,
    releaseAuthorized: false,
    claimCeiling,
    reason: 'source freeze contains task or untracked changes',
    sourceFreeze: freeze
  }, null, 2))
  process.exit(1)
}

const initialRcLedger = loadOpenSpecTaskLedger({ repoRoot, changeName: openspecChange })
if (!initialRcLedger.rcLedgerValid) {
  console.log(JSON.stringify({
    schemaVersion: 2,
    id: gateId,
    commandMode: gateMode,
    status: 'failed',
    passed: false,
    commandPassed: false,
    controlPlaneReady: false,
    productRcGatePassed: false,
    artifactAdmissionGatePassed: false,
    upstreamArtifactAdmissionGatePassed: false,
    releaseAuthorized: false,
    claimCeiling,
    reason: 'active OpenSpec RC ledger is invalid',
    ...initialRcLedger
  }, null, 2))
  process.exit(1)
}

const preLockSource = sourceIdentity(repoRoot)
let closure = null
if (executionOnly) {
  try {
    if (typeof process.send !== 'function') throw new Error('execution_requires_validation_wrapper')
    closure = freezeReleaseClosure({ repoRoot, source: preLockSource,
      taskIds: closureTasksOption.value.split(','), reportPath: closureReportOption.value })
  } catch {
    console.log(JSON.stringify({ id: gateId, status: 'failed', passed: false, commandPassed: false,
      reason: 'closure declaration or frozen source inputs are invalid' }, null, 2))
    process.exit(1)
  }
}
const executionManifest = commands.map((item) => ({ id: item.id, command: commandText(item), cwd: item.cwd,
  heavyweight: item.heavyweight, logicalChecks: item.logicalChecks || [item.id], expectedExecutions: 1 }))
const formalEvidencePreflight = !controlPlaneOnly
  ? preflightRuntimeGoFormalEvidence({
      repoRoot,
      bundleDirectory: formalEvidenceOption.value,
      source: preLockSource
    })
  : null
if (formalEvidencePreflight && formalEvidencePreflight.passed !== true) {
  console.log(JSON.stringify({
    schemaVersion: 2,
    id: gateId,
    commandMode: gateMode,
    status: 'failed',
    passed: false,
    commandPassed: false,
    controlPlaneReady: false,
    productRcGatePassed: false,
    engineeringProductRcReady: false,
    artifactAdmissionGatePassed: false,
    upstreamArtifactAdmissionGatePassed: false,
    releaseAuthorized: false,
    claimCeiling,
    productRcStatus: 'blocked',
    reason: formalEvidenceOption.provided
      ? 'formal evidence failed the read-only pre-lock preflight'
      : 'formal evidence input is required before a product release run',
    source: preLockSource,
    sourceFreeze: freeze,
    formalRun: { status: 'not_executed', created: false },
    formalEvidence: {
      electron: 'not_executed',
      package: 'not_executed',
      a0: 'not_executed',
      b1: 'not_executed',
      commercialRelease: 'not_authorized'
    },
    formalEvidenceInput: {
      requested: formalEvidenceOption.provided,
      consumed: false
    },
    formalEvidencePreflight,
    externalReleaseAuthorization: externalReleaseAuthorizationProjection(),
    heavyExecutionCounts: zeroHeavyExecutionCounts(),
    executionManifest: commands.map((item) => ({
      id: item.id,
      command: commandText(item),
      cwd: item.cwd,
      heavyweight: item.heavyweight,
      logicalChecks: item.logicalChecks || [item.id],
      executionCount: 0,
      status: 'not_executed'
    }))
  }, null, 2))
  process.exit(1)
}

const receiptRun = createReceiptRun({ repoRoot })
const initialSource = receiptRun.source
const checks = []
const receiptRefs = []
const reports = new Map()
let failed = false

for (const item of commands) {
  const beforeSource = sourceIdentity(repoRoot)
  const beforeLedger = loadOpenSpecTaskLedger({ repoRoot, changeName: openspecChange,
    expectedTasksFileSha256: initialRcLedger.tasksFileSha256 })
  if (beforeSource.head !== initialSource.head || beforeSource.tree !== initialSource.tree ||
    !beforeLedger.rcLedgerValid || !worktreeSourceFreeze().passed) {
    failed = true
    checks.push({
      id: item.id,
      status: 'failed',
      command: commandText(item),
      cwd: item.cwd,
      durationMs: 0,
      exitStatus: 1,
      reason: 'source changed before command execution',
      sourceStable: false,
      executionCount: 0,
      heavyweight: item.heavyweight,
      logicalChecks: item.logicalChecks || [item.id]
    })
    break
  }
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-release-gate] ${item.id}`)
  const capture = item.captureJson === true
  const result = spawnSync(item.command, item.args, {
    cwd: item.cwd,
    env: commandEnvironment(item),
    encoding: capture ? 'utf8' : undefined,
    stdio: capture ? 'pipe' : 'inherit',
    maxBuffer: capture ? 64 * 1024 * 1024 : undefined
  })
  const status = result.status ?? 1
  const parsed = capture ? parseJSONObject(result.stdout) : null
  const reportValid = !capture || parsed?.id === item.reportId
  if (capture && result.stderr) process.stderr.write(result.stderr)
  if (capture && status !== 0 && result.stdout) process.stderr.write(result.stdout)
  const afterSource = sourceIdentity(repoRoot)
  const afterFreeze = worktreeSourceFreeze()
  const afterLedger = loadOpenSpecTaskLedger({ repoRoot, changeName: openspecChange,
    expectedTasksFileSha256: initialRcLedger.tasksFileSha256 })
  const sourceStable = afterSource.head === initialSource.head &&
    afterSource.tree === initialSource.tree &&
    afterFreeze.passed && afterLedger.rcLedgerValid
  const check = {
    id: item.id,
    status: status === 0 && sourceStable && reportValid ? 'passed' : 'failed',
    command: commandText(item),
    cwd: item.cwd,
    durationMs: Date.now() - started,
    exitStatus: status,
    sourceStable,
    reportValid,
    sourceFreeze: afterFreeze,
    executionCount: 1,
    heavyweight: item.heavyweight,
    logicalChecks: item.logicalChecks || [item.id]
  }
  checks.push(check)
  if (parsed) reports.set(item.id, parsed)

  if (result.error || status !== 0 || !sourceStable || !reportValid) {
    failed = true
    break
  }
}

const performanceReport = reports.get('rc-structural-and-performance-suite') || null
const sharedRuntimeBinary = performanceReport?.runtimeBinary || { status: 'unavailable' }
for (const [index, check] of checks.entries()) {
  const receipt = writeAndRevalidateReceipt({
    runDir: receiptRun.runDir,
    fileName: `${String(index + 1).padStart(2, '0')}-${check.id}.json`,
    payload: {
      receiptType: 'command',
      runId: receiptRun.runId,
      source: initialSource,
      toolchain: receiptRun.toolchain,
      command: { id: check.id, text: check.command, cwd: check.cwd },
      result: {
        status: check.status,
        exitStatus: check.exitStatus,
        durationMs: check.durationMs,
        sourceStable: check.sourceStable,
        executionCount: check.executionCount
      },
      runtimeBinary: sharedRuntimeBinary,
      report: reports.get(check.id) || null
    },
    expected: {
      runId: receiptRun.runId,
      source: initialSource,
      toolchain: receiptRun.toolchain,
      'command.id': check.id,
      'command.text': check.command,
      'command.cwd': check.cwd,
      runtimeBinary: sharedRuntimeBinary
    }
  })
  receiptRefs.push({ id: check.id, ...receipt })
}
if (performanceReport?.structural && performanceReport?.benchmark) {
  const performanceCommand = commands.find((item) => item.id === 'rc-structural-and-performance-suite')
  for (const logicalId of ['structural-performance', 'rc-performance-benchmark']) {
    const logicalPayload = logicalId === 'structural-performance'
      ? {
          ...performanceReport.structural,
          contractCoverage: {
            durableBeforePublish: 'runtime public-seam durable finite replay plus full Go matrices',
            firstEventAndTerminalOrder: 'safe-progress-precedes-atomic-terminal',
            providerDraftAndReasoningLeak: [
              'provider-draft-never-published',
              'ordinary-text-turn-no-reasoning-leak'
            ],
            productSseContractTests: 'full-root-test command passed once in the release manifest',
            providerAttemptAndRejectionDiagnostics: {
              matrixCommandsPassed: ['go-ordinary-matrix', 'go-prod-matrix', 'go-race-matrix'].every((id) =>
                checks.some((check) => check.id === id && check.status === 'passed')
              ),
              requiredTestIds: [
                'TestRuntimeServerRejectsProviderModelMismatchBeforeProviderRequest',
                'TestRejectCancelledRuntimeResultDropsDraftAndPreservesProviderDiagnostics',
                'TestRuntimeRunnerRejectsProviderTerminalErrorWithoutDurableDisposition'
              ]
            },
            latencyThresholdApplied: false
          }
        }
      : performanceReport.benchmark
    const receipt = writeAndRevalidateReceipt({
      runDir: receiptRun.runDir,
      fileName: `${logicalId}.json`,
      payload: {
        receiptType: 'logical-check',
        runId: receiptRun.runId,
        source: initialSource,
        toolchain: receiptRun.toolchain,
        command: {
          id: logicalId,
          executionOwner: 'rc-structural-and-performance-suite',
          text: commandText(performanceCommand),
          cwd: repoRoot
        },
        result: logicalPayload,
        runtimeBinary: performanceReport.runtimeBinary
      },
      expected: {
        runId: receiptRun.runId,
        source: initialSource,
        toolchain: receiptRun.toolchain,
        'command.id': logicalId,
        'command.executionOwner': 'rc-structural-and-performance-suite',
        runtimeBinary: performanceReport.runtimeBinary
      }
    })
    receiptRefs.push({ id: logicalId, ...receipt })
  }
}

const formalEvidenceInput = evaluateRuntimeGoFormalEvidence({
  repoRoot,
  bundleDirectory: formalEvidenceOption.value,
  source: initialSource,
  exactFormalArtifact: reports.get('artifact-legal-obligations-audit')?.exactFormalArtifact
})
const formalEvidenceReceipt = writeAndRevalidateReceipt({
  runDir: receiptRun.runDir,
  fileName: 'formal-evidence-ingestion.json',
  payload: {
    receiptType: 'formal-evidence-ingestion',
    runId: receiptRun.runId,
    source: initialSource,
    toolchain: receiptRun.toolchain,
    result: formalEvidenceInput
  },
  expected: {
    receiptType: 'formal-evidence-ingestion',
    runId: receiptRun.runId,
    source: initialSource,
    toolchain: receiptRun.toolchain,
    result: formalEvidenceInput
  }
})
receiptRefs.push({ id: 'formal-evidence-ingestion', ...formalEvidenceReceipt })
const rcLedger = loadOpenSpecTaskLedger({
  repoRoot,
  changeName: openspecChange,
  expectedTasksFileSha256: initialRcLedger.tasksFileSha256
})
const { controlPlaneReady, executionPassed, artifactAdmissionGatePassed, artifactLegalPlanPassed, artifactLegalPlan, upstreamArtifactProjectionValid, upstreamArtifactProjectionChecks, upstreamArtifactLicenseClassGatePassed, upstreamArtifactAdmissionGatePassed, upstreamArtifactAdmission, upstreamArtifactAdmissionBlockers, upstreamSourceAudit, formalEvidence, operationalRcBlockers, rcRequired, heavyExecutionCounts, expectedHeavyCounts, sourceIdentityValid, performanceBindingChecks, performanceBindingValid, receiptIntegrityValid, boundaryProjectionChecks, boundaryProjectionValid } = evaluateReleaseExecution({
  initialSource, receiptRun, checks, receiptRefs, reports, formalEvidenceInput, rcLedger, failed
})
const productRcGatePassed = false // Only independent final adjudication can publish final PASS.
const commandPassed = controlPlaneOnly ? controlPlaneReady : executionPassed
const status = controlPlaneOnly
  ? (controlPlaneReady ? 'control_plane_ready' : 'failed')
  : (executionPassed ? 'execution_passed' : (controlPlaneReady ? 'blocked' : 'failed'))
const externalReleaseAuthorization = externalReleaseAuthorizationProjection()
const finalPayload = {
  receiptType: executionOnly ? 'normalized-execution' : 'normalized-control-plane',
  runId: receiptRun.runId,
  commandMode: gateMode,
  executionPassed,
  executionManifest,
  closure,
  source: initialSource,
  toolchain: receiptRun.toolchain,
  formalRun: {
    path: receiptRun.formalRunPath,
    sha256: receiptRun.formalRunSha256
  },
  status,
  passed: productRcGatePassed,
  commandPassed,
  controlPlaneReady,
  productRcGatePassed,
  engineeringProductRcReady: productRcGatePassed,
  artifactAdmissionGatePassed,
  upstreamArtifactProjectionValid,
  upstreamArtifactProjectionChecks,
  upstreamArtifactLicenseClassGatePassed,
  upstreamArtifactAdmissionGatePassed,
  upstreamArtifactAdmission,
  upstreamArtifactAdmissionBlockers,
  researchFreshness: upstreamSourceAudit.researchFreshness || null,
  releaseAuthorized: false,
  claimCeiling,
  productRcStatus: productRcGatePassed ? 'passed' : 'blocked',
  formalEvidence,
  formalEvidenceInput,
  formalEvidencePreflight,
  externalReleaseAuthorization,
  openspecChange: rcLedger.openspecChange,
  tasksFileSha256: rcLedger.tasksFileSha256,
  totalTasks: rcLedger.totalTasks,
  completedTasks: rcLedger.completedTasks,
  openRcRequiredIds: rcLedger.openRcRequiredIds,
  openPostRcIds: rcLedger.openPostRcIds,
  rcLedgerValid: rcLedger.rcLedgerValid,
  rcLedgerProblems: rcLedger.problems,
  sourceFreeze: freeze,
  heavyExecutionCounts,
  expectedHeavyCounts,
  sourceIdentityValid,
  performanceBindingChecks,
  performanceBindingValid,
  receiptIntegrityValid,
  boundaryProjectionChecks,
  boundaryProjectionValid,
  checks,
  runtimeBinary: performanceReport?.runtimeBinary || null,
  performance: performanceReport
    ? {
        status: performanceReport.status,
        structural: performanceReport.structural,
        benchmark: performanceReport.benchmark
      }
    : null,
  artifactLegalPlanPassed,
  artifactLegalPlan,
  formalArtifact: artifactLegalPlan.exactFormalArtifact || {
    status: 'UNVERIFIED',
    provided: false,
    passed: false
  },
  operationalRcBlockers,
  rcRequired,
  receiptRefs
}
const normalizedReceipt = writeAndRevalidateReceipt({
  runDir: receiptRun.runDir,
  fileName: executionOnly ? 'normalized-execution-receipt.json' : 'normalized-control-plane-receipt.json',
  payload: finalPayload,
  expected: {
    runId: receiptRun.runId,
    source: initialSource,
    toolchain: receiptRun.toolchain,
    controlPlaneReady,
    passed: productRcGatePassed,
    commandPassed,
    productRcGatePassed,
    artifactAdmissionGatePassed,
    upstreamArtifactProjectionValid,
    upstreamArtifactLicenseClassGatePassed,
    upstreamArtifactAdmissionGatePassed,
    upstreamArtifactAdmission,
    upstreamArtifactAdmissionBlockers,
    researchFreshness: upstreamSourceAudit.researchFreshness || null,
    releaseAuthorized: false,
    openspecChange: rcLedger.openspecChange,
    tasksFileSha256: rcLedger.tasksFileSha256,
    rcLedgerValid: rcLedger.rcLedgerValid,
    heavyExecutionCounts,
    receiptIntegrityValid,
    boundaryProjectionValid,
    artifactLegalPlanPassed,
    formalEvidenceInput,
    formalEvidencePreflight
  }
})

let executionSeal = null
if (executionOnly && executionPassed) {
  try {
    executionSeal = sealReleaseExecution({ repoRoot, receiptRun, normalized: normalizedReceipt,
      manifest: executionManifest, closure, formalEvidenceDirectory: formalEvidenceOption.value })
    process.send({ type: 'analytix-release-execution-seal', seal: executionSeal, runId: receiptRun.runId }, (error) => {
      if (error) process.exitCode = 1
      process.disconnect()
    })
  } catch (error) {
    console.log(JSON.stringify({ id: gateId, commandMode: gateMode, status: 'failed', passed: false,
      commandPassed: false, executionPassed: false, normalizedReceipt,
      reason: /^[a-z_]+$/u.test(error?.message || '') ? error.message : 'execution_seal_revalidation_failed' }, null, 2))
    process.exit(1)
  }
}

console.log(JSON.stringify({
  schemaVersion: 2,
  id: gateId,
  commandMode: gateMode,
  executionPassed,
  executionSeal,
  closure,
  status,
  passed: productRcGatePassed,
  commandPassed,
  controlPlaneReady,
  productRcGatePassed,
  engineeringProductRcReady: productRcGatePassed,
  artifactAdmissionGatePassed,
  upstreamArtifactProjectionValid,
  upstreamArtifactProjectionChecks,
  upstreamArtifactLicenseClassGatePassed,
  upstreamArtifactAdmissionGatePassed,
  upstreamArtifactAdmission,
  upstreamArtifactAdmissionBlockers,
  researchFreshness: upstreamSourceAudit.researchFreshness || null,
  releaseAuthorized: false,
  claimCeiling,
  productRcStatus: productRcGatePassed ? 'passed' : 'blocked',
  formalEvidence,
  formalEvidenceInput,
  formalEvidencePreflight,
  externalReleaseAuthorization,
  openspecChange: rcLedger.openspecChange,
  tasksFileSha256: rcLedger.tasksFileSha256,
  totalTasks: rcLedger.totalTasks,
  completedTasks: rcLedger.completedTasks,
  openRcRequiredIds: rcLedger.openRcRequiredIds,
  openPostRcIds: rcLedger.openPostRcIds,
  rcLedgerValid: rcLedger.rcLedgerValid,
  rcLedgerProblems: rcLedger.problems,
  source: initialSource,
  sourceFreeze: freeze,
  heavyExecutionCounts,
  sourceIdentityValid,
  performanceBindingChecks,
  performanceBindingValid,
  receiptIntegrityValid,
  boundaryProjectionChecks,
  boundaryProjectionValid,
  checks,
  performance: finalPayload.performance,
  artifactLegalPlanPassed,
  artifactLegalPlan: finalPayload.artifactLegalPlan,
  formalArtifact: finalPayload.formalArtifact,
  operationalRcBlockers,
  rcRequired,
  normalizedReceipt
}, null, 2))

process.exitCode = gateExitCode({ mode: gateMode, controlPlaneReady, executionPassed, passed: productRcGatePassed })
