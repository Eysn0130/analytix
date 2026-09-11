#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const strictGate = args.has('--gate')

const commands = [
  {
    id: 'runtime-health-smoke',
    command: npmCommand,
    args: ['run', 'runtime:go:health-smoke', '--', '--json'],
    validateReport: validateRuntimeHealthReport
  },
  {
    id: 'product-regression',
    command: npmCommand,
    args: ['run', 'runtime:go:product-regression', '--', '--json']
  },
  {
    id: 'speed-cache-gate',
    command: npmCommand,
    args: ['run', 'runtime:go:speed-cache-gate', '--', '--json']
  }
]

function forwardedOptionArgs(optionNames) {
  const forwarded = []
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    const matched = optionNames.find((name) => item === name || item.startsWith(`${name}=`))
    if (!matched) continue
    forwarded.push(item)
    if (item === matched && rawArgs[index + 1] && !rawArgs[index + 1].startsWith('--')) {
      forwarded.push(rawArgs[index + 1])
      index += 1
    }
  }
  return forwarded
}

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function deterministicCheckEnv() {
  const env = { ...process.env }
  for (const key of [
    'ANALYTIX_RUNTIME_READY',
    'ANALYTIX_RUNTIME_READY_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_OPERATOR_APPROVES_DEFAULT',
    'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE',
    'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE',
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_GUI_SMOKE_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_SESSION_SOAK_EVIDENCE'
  ]) {
    delete env[key]
  }
  return env
}

function recordValue(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function validateRuntimeHealthReport(report) {
  if (report?.id !== 'runtime-go-runtime-health-smoke' || report?.schemaVersion !== 2) {
    return {
      ok: false,
      runtimeIdentityEvidenceValid: false,
      boundaryTupleValid: false,
      providerAgentChainTupleValid: false,
      reason: 'runtime health report id/schema is not the current product-chain contract'
    }
  }
  const smoke = recordValue(report.smoke)
  const checks = Array.isArray(report.checks) ? report.checks.map(recordValue) : []
  const requiredRuntimeCheckIds = [
    'runtime-server-build',
    'runtime-server-ready',
    'runtime-health',
    'runtime-info',
    'runtime-tools',
    'runtime-authority-boundary',
    'runtime-provider-agent-chain',
    'runtime-server-cleanup'
  ]
  const requiredRuntimeChecksPassed = requiredRuntimeCheckIds.every((id) => {
    const matching = checks.filter((item) => item.id === id)
    return matching.length === 1 && matching[0].status === 'passed'
  })
  const runtimeIdentityEvidenceValid = smoke.actualRuntimeProcessStarted === true &&
    smoke.productionRuntime === true &&
    smoke.durableRootConfigured === true &&
    smoke.dataDirScope === 'temporary' &&
    smoke.usesRealProviderConfig === false &&
    smoke.providerConfigMode === 'isolated-contract-provider-config' &&
    smoke.healthOk === true &&
    smoke.runtimeInfoOk === true &&
    smoke.runtimeToolsOk === true &&
    smoke.productionCapabilitiesOk === true &&
    smoke.secretsPrinted === false &&
    requiredRuntimeChecksPassed
  const boundaryCheckPassed = checks.some((item) =>
    item.id === 'runtime-authority-boundary' && item.status === 'passed')
  const providerAgentChainCheckPassed = checks.some((item) =>
    item.id === 'runtime-provider-agent-chain' && item.status === 'passed')
  const boundaryTupleValid = smoke.authorityBoundaryOk === true &&
    smoke.hostAcceptedFinalOk === true &&
    smoke.providerUsageAbsentOk === true &&
    smoke.protectedProviderDispatchAbsent === true &&
    Number.isInteger(smoke.protectedProviderRequestCount) &&
    smoke.protectedProviderRequestCount === 0 &&
    smoke.providerDraftWithheld === true &&
    smoke.protectedTurnCreateOk === true &&
    smoke.protectedSseReplayOk === true &&
    boundaryCheckPassed
  const providerAgentChainTupleValid = smoke.productChainMode === 'production-authority-enrolled-provider-agent-chain' &&
    smoke.productChainOk === true &&
    smoke.providerPositiveChainOk === true &&
    smoke.providerAgentLoopReady === true &&
    smoke.providerAgentLoopCoverage === 'ordinary-turn-with-protected-boundary' &&
    smoke.providerExecutionExpected === true &&
    smoke.providerExecutionBlocked === false &&
    smoke.usageOk === true &&
    smoke.providerUsageObserved === true &&
    smoke.ordinaryThreadDistinct === true &&
    smoke.ordinaryTurnCreateOk === true &&
    smoke.ordinarySseReplayOk === true &&
    smoke.ordinaryPublicFinalOk === true &&
    Number.isInteger(smoke.contractProviderRequestCount) &&
    smoke.contractProviderRequestCount === 1 &&
    smoke.contractProviderPromptSeen === true &&
    smoke.providerAuthorizationConfigured === true &&
    smoke.turnCreateOk === true &&
    smoke.sseReplayOk === true &&
    smoke.attachmentOk === true &&
    providerAgentChainCheckPassed
  return {
    ok: runtimeIdentityEvidenceValid && boundaryTupleValid && providerAgentChainTupleValid,
    runtimeIdentityEvidenceValid,
    boundaryTupleValid,
    providerAgentChainTupleValid,
    reason: !runtimeIdentityEvidenceValid
      ? 'runtime health real runtime identity evidence is incomplete or contradictory'
      : !boundaryTupleValid
        ? 'runtime health protected authority-boundary evidence is incomplete or contradictory'
        : !providerAgentChainTupleValid
          ? 'runtime health ordinary provider Agent-chain evidence is incomplete or contradictory'
          : ''
  }
}

function stringList(values) {
  return Array.isArray(values)
    ? [...new Set(values.map((value) => String(value || '').trim()).filter(Boolean))]
    : []
}

function operatorGateDetail(finalGateBlockers, operatorDependencySummary, operatorCurrentEnvGate) {
  const present = finalGateBlockers.includes('operator:gate')
  const evidenceEnvGate = recordValue(recordValue(operatorDependencySummary).envGate)
  const currentEnvGate = recordValue(operatorCurrentEnvGate)
  const dependencyBlockers = stringList(recordValue(operatorDependencySummary).blockers)
  const evidenceEnvSatisfied = evidenceEnvGate.runtimeReady === true &&
    evidenceEnvGate.operatorApprovesDefault === true
  const currentEnvSatisfied = currentEnvGate.runtimeReady === true &&
    currentEnvGate.operatorApprovesDefault === true
  return {
    present,
    evidenceEnvGate,
    currentEnvGate,
    dependencyBlockers,
    requiresOperatorApproval: present && !evidenceEnvSatisfied && !currentEnvSatisfied,
    requiresOperatorEvidenceRefresh: present && !evidenceEnvSatisfied && currentEnvSatisfied,
    blockedByDependencies: present && evidenceEnvSatisfied && dependencyBlockers.length > 0
  }
}

function providerGateSettingsCoverage(value) {
  const coverage = recordValue(value)
  return {
    status: String(coverage.status || '').trim(),
    source: String(coverage.source || '').trim(),
    providerCount: Number.isFinite(coverage.providerCount) ? coverage.providerCount : 0,
    matchedProviderIds: stringList(coverage.matchedProviderIds),
    usableProviderIds: stringList(coverage.usableProviderIds),
    usableNonDeepSeekProviderIds: stringList(coverage.usableNonDeepSeekProviderIds),
    deepseekSettingsProfileUsable: coverage.deepseekSettingsProfileUsable === true,
    nonDeepSeekSettingsProfileUsable: coverage.nonDeepSeekSettingsProfileUsable === true,
    satisfiesFinalProviderCoverageFromSettings: coverage.satisfiesFinalProviderCoverageFromSettings === true,
    credentialValuesRecorded: coverage.credentialValuesRecorded === true
  }
}

function providerGateDetail(finalGateBlockers, value) {
  const present = finalGateBlockers.includes('provider:at-least-one-non-deepseek-provider')
  const source = recordValue(value)
  const credentialedNonDeepSeekProviderIds = stringList(source.credentialedNonDeepSeekProviderIds)
  return {
    present,
    evidencePath: typeof source.evidencePath === 'string' ? source.evidencePath : '',
    candidateProviderIds: stringList(source.candidateProviderIds),
    candidates: Array.isArray(source.candidates)
      ? source.candidates.map((candidate) => {
        const item = recordValue(candidate)
        return {
          id: String(item.id || '').trim(),
          status: String(item.status || '').trim(),
          missingEnv: stringList(item.missingEnv),
          missingSettingsProfileFields: stringList(item.missingSettingsProfileFields)
        }
      }).filter((item) => item.id)
      : [],
    acceptedSettingsProfilePath: String(source.acceptedSettingsProfilePath || '').trim(),
    settingsProfileCoverage: providerGateSettingsCoverage(source.settingsProfileCoverage),
    credentialedDeepSeekPassed: source.credentialedDeepSeekPassed === true,
    credentialedNonDeepSeekProviderIds,
    requiresNonDeepSeekProvider: present && credentialedNonDeepSeekProviderIds.length === 0
  }
}

function finalGateBlockerSummary(finalGateBlockers, context = {}) {
  const operatorDetail = operatorGateDetail(
    finalGateBlockers,
    context.operatorDependencySummary,
    context.operatorCurrentEnvGate
  )
  const providerDetail = providerGateDetail(finalGateBlockers, context.providerGateDetail)
  const summary = {
    schemaVersion: 1,
    dryRun: [],
    externalInput: [],
    operator: [],
    evidenceHygiene: [],
    worktreeHygiene: [],
    deterministic: [],
    other: [],
    requiresExternalInput: false,
    requiresNonDeepSeekProvider: false,
    providerGateDetail: providerDetail,
    requiresOperatorApproval: false,
    requiresOperatorEvidenceRefresh: false,
    operatorBlockedByDependencies: false,
    operatorDependencyBlockers: [],
    operatorGateDetail: operatorDetail,
    requiresLocalHygiene: false,
    requiresDeterministicFix: false
  }
  for (const blocker of finalGateBlockers) {
    if (blocker === 'dry-run-cannot-authorize-cutover') {
      summary.dryRun.push(blocker)
    } else if (blocker === 'operator:gate') {
      summary.operator.push(blocker)
    } else if (blocker.startsWith('provider:') || blocker.startsWith('mcp:')) {
      summary.externalInput.push(blocker)
    } else if (blocker.startsWith('evidence:') || blocker.startsWith('live-evidence')) {
      summary.evidenceHygiene.push(blocker)
    } else if (blocker.startsWith('worktree:')) {
      summary.worktreeHygiene.push(blocker)
    } else if (blocker.startsWith('deterministic-') || blocker.startsWith('deterministic-check:')) {
      summary.deterministic.push(blocker)
    } else {
      summary.other.push(blocker)
    }
  }
  summary.requiresExternalInput = summary.externalInput.length > 0
  summary.requiresNonDeepSeekProvider = providerDetail.requiresNonDeepSeekProvider
  summary.requiresOperatorApproval = operatorDetail.requiresOperatorApproval
  summary.requiresOperatorEvidenceRefresh = operatorDetail.requiresOperatorEvidenceRefresh
  summary.operatorBlockedByDependencies = operatorDetail.blockedByDependencies
  summary.operatorDependencyBlockers = operatorDetail.dependencyBlockers
  summary.requiresLocalHygiene = summary.evidenceHygiene.length > 0 || summary.worktreeHygiene.length > 0
  summary.requiresDeterministicFix = summary.deterministic.length > 0
  return summary
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-default-readiness-report] ${item.id}`)
  if (skipCommands) {
    return {
      check: {
        id: item.id,
        status: 'skipped',
        command: commandText(item),
        durationMs: 0
      }
    }
  }

  const result = spawnSync(item.command, item.args, {
    cwd: process.cwd(),
    env: deterministicCheckEnv(),
    stdio: 'pipe',
    encoding: 'utf8',
    maxBuffer: 96 * 1024 * 1024
  })
  const status = result.status ?? (result.signal ? 1 : 0)
  const parsed = parseLastJSONObject(result.stdout || '')
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const validation = parsed.ok && typeof item.validateReport === 'function'
    ? item.validateReport(report)
    : { ok: true, reason: '' }
  const passed = status === 0 && reportStatus === 'passed' && validation.ok
  const check = {
    id: item.id,
    status: passed ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    exitStatus: status,
    reportId: typeof report?.id === 'string' ? report.id : '',
    reportStatus,
    reason: failureReason(result, parsed, status, reportStatus, validation)
  }
  if (result.signal) check.signal = result.signal
  return { check, report }
}

function runPreflightGate() {
  const started = Date.now()
  const preflightArgs = [
    'run',
    'runtime:go:preflight',
    '--',
    '--json',
    '--gate',
    ...forwardedOptionArgs(['--live-evidence-json'])
  ]
  const command = [npmCommand, ...preflightArgs].join(' ')
  if (skipCommands) {
    return {
      status: 'skipped',
      passed: false,
      command,
      durationMs: 0,
      finalGateBlocked: true,
      finalGateBlockers: ['dry-run-cannot-authorize-cutover'],
      reason: 'dry-run reports cannot authorize the final Go runtime gate'
    }
  }

  const result = spawnSync(npmCommand, preflightArgs, {
    cwd: process.cwd(),
    env: process.env,
    stdio: 'pipe',
    encoding: 'utf8',
    maxBuffer: 96 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  const parsed = parseLastJSONObject(result.stdout || '')
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const blockers = Array.isArray(report?.finalGateBlockers)
    ? report.finalGateBlockers.map((item) => String(item || '').trim()).filter(Boolean)
    : []
  const finalGateBlocked = report?.finalGateBlocked === true || exitStatus !== 0 || reportStatus !== 'passed'
  const gate = {
    status: finalGateBlocked ? reportStatus || 'failed' : 'passed',
    passed: !finalGateBlocked,
    command,
    durationMs: Date.now() - started,
    exitStatus,
    reportId: typeof report?.id === 'string' ? report.id : '',
    reportStatus,
    localChecksStatus: typeof report?.localChecksStatus === 'string' ? report.localChecksStatus : '',
    gateMode: report?.gateMode === true,
    finalGateBlocked,
    finalGateBlockers: blockers,
    finalGateBlockerSummary: recordValue(report?.finalGateBlockerSummary),
    providerGateDetail: recordValue(recordValue(report?.finalGateBlockerSummary).providerGateDetail),
    operatorCurrentEnvGate: recordValue(report?.operatorCurrentEnvGate),
    operatorDependencySummary: recordValue(report?.operatorDependencySummary),
    reason: failureReason(result, parsed, exitStatus, reportStatus)
  }
  if (result.signal) gate.signal = result.signal
  return gate
}

function failureReason(result, parsed, status, reportStatus, validation = { ok: true, reason: '' }) {
  if (result.error) return result.error.message
  if (status !== 0) return `command exited with status ${status}`
  if (!parsed.ok) return parsed.reason
  if (reportStatus !== 'passed') return `report status is ${reportStatus || 'missing'}`
  if (!validation.ok) return validation.reason || 'report contract validation failed'
  return ''
}

function parseLastJSONObject(text) {
  let start = text.lastIndexOf('{')
  while (start >= 0) {
    const candidate = text.slice(start).trim()
    try {
      const value = JSON.parse(candidate)
      if (value && typeof value === 'object' && !Array.isArray(value)) {
        return { ok: true, value }
      }
    } catch {
      // Keep walking backward; command output may contain log braces before the final report.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return { ok: false, reason: 'command did not emit a trailing JSON object' }
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
}

function summarizeProductRegression(report) {
  const retirement = report?.retirement && typeof report.retirement === 'object'
    ? report.retirement
    : {}
  const matrix = report?.matrix && typeof report.matrix === 'object'
    ? report.matrix
    : {}
  return {
    passed: report?.status === 'passed',
    rowCount: Number.isFinite(matrix.rowCount) ? matrix.rowCount : 0,
    features: Array.isArray(matrix.features) ? matrix.features : [],
    publicLegacyExposureCount: Number.isFinite(retirement.publicLegacyExposureCount)
      ? retirement.publicLegacyExposureCount
      : null,
    internalLegacyDelegateCount: Number.isFinite(retirement.internalLegacyDelegateCount)
      ? retirement.internalLegacyDelegateCount
      : null,
    temporaryGoProdViolationCount: Number.isFinite(retirement.temporaryGoProdViolationCount)
      ? retirement.temporaryGoProdViolationCount
      : null,
    productionGoSourceCount: Number.isFinite(retirement.productionGoSourceCount)
      ? retirement.productionGoSourceCount
      : null,
    retiredProductionMarkerViolationCount: Number.isFinite(retirement.retiredProductionMarkerViolationCount)
      ? retirement.retiredProductionMarkerViolationCount
      : null,
    legacyUpstreamFieldKeyViolationCount: Number.isFinite(retirement.legacyUpstreamFieldKeyViolationCount)
      ? retirement.legacyUpstreamFieldKeyViolationCount
      : null
  }
}

function summarizeSpeedCache(report) {
  const checks = Array.isArray(report?.checks) ? report.checks : []
  return {
    passed: report?.status === 'passed',
    checkCount: checks.length,
    checks: checks.map((item) => ({
      id: item.id,
      status: item.status,
      durationMs: item.durationMs
    }))
  }
}

function summarizeRuntimeHealth(report) {
  const smoke = report?.smoke && typeof report.smoke === 'object'
    ? report.smoke
    : {}
  const hasSmoke = Object.keys(smoke).length > 0
  const checks = Array.isArray(report?.checks) ? report.checks : []
  const validation = validateRuntimeHealthReport(report)
  return {
    passed: report?.status === 'passed' && validation.ok,
    runtimeIdentityEvidenceValid: validation.runtimeIdentityEvidenceValid === true,
    boundaryEvidenceValid: validation.boundaryTupleValid === true,
    providerAgentChainEvidenceValid: validation.providerAgentChainTupleValid === true,
    actualRuntimeProcessStarted: smoke.actualRuntimeProcessStarted === true,
    productionRuntime: smoke.productionRuntime === true,
    durableRootConfigured: smoke.durableRootConfigured === true,
    dataDirScope: typeof smoke.dataDirScope === 'string' ? smoke.dataDirScope : '',
    usesRealProviderConfig: smoke.usesRealProviderConfig === true,
    providerConfigMode: typeof smoke.providerConfigMode === 'string'
      ? smoke.providerConfigMode
      : smoke.usesRealProviderConfig === true
        ? 'real-provider-config'
        : hasSmoke
          ? 'unknown'
          : 'not-run',
    usesIsolatedEmptyProviderConfig: smoke.providerConfigMode === 'isolated-empty-provider-config',
    usesIsolatedContractProviderConfig: smoke.providerConfigMode === 'isolated-contract-provider-config',
    productChainMode: typeof smoke.productChainMode === 'string' ? smoke.productChainMode : 'not-run',
    productChainOk: smoke.productChainOk === true,
    authorityBoundaryOk: smoke.authorityBoundaryOk === true,
    hostAcceptedFinalOk: smoke.hostAcceptedFinalOk === true,
    providerPositiveChainOk: smoke.providerPositiveChainOk === true,
    providerAgentLoopReady: smoke.providerAgentLoopReady === true,
    providerAgentLoopCoverage: typeof smoke.providerAgentLoopCoverage === 'string'
      ? smoke.providerAgentLoopCoverage
      : 'not-run',
    providerExecutionExpected: smoke.providerExecutionExpected === true,
    providerExecutionBlocked: smoke.providerExecutionBlocked === true,
    protectedTurnCreateOk: smoke.protectedTurnCreateOk === true,
    ordinaryTurnCreateOk: smoke.ordinaryTurnCreateOk === true,
    protectedSseReplayOk: smoke.protectedSseReplayOk === true,
    ordinarySseReplayOk: smoke.ordinarySseReplayOk === true,
    ordinaryThreadDistinct: smoke.ordinaryThreadDistinct === true,
    ordinaryPublicFinalOk: smoke.ordinaryPublicFinalOk === true,
    turnCreateOk: smoke.turnCreateOk === true,
    sseReplayOk: smoke.sseReplayOk === true,
    usageOk: smoke.usageOk === true,
    providerUsageObserved: smoke.providerUsageObserved === true,
    providerUsageAbsentOk: smoke.providerUsageAbsentOk === true,
    providerDraftWithheld: smoke.providerDraftWithheld === true,
    protectedProviderDispatchAbsent: smoke.protectedProviderDispatchAbsent === true,
    protectedProviderRequestCount: Number.isFinite(smoke.protectedProviderRequestCount)
      ? smoke.protectedProviderRequestCount
      : null,
    attachmentOk: smoke.attachmentOk === true,
    contractProviderRequestCount: Number.isFinite(smoke.contractProviderRequestCount)
      ? smoke.contractProviderRequestCount
      : null,
    contractProviderPromptSeen: smoke.contractProviderPromptSeen === true,
    providerAuthorizationConfigured: smoke.providerAuthorizationConfigured === true,
    actualPackagedAppLaunched: smoke.actualPackagedAppLaunched === true,
    healthOk: smoke.healthOk === true,
    runtimeInfoOk: smoke.runtimeInfoOk === true,
    runtimeToolsOk: smoke.runtimeToolsOk === true,
    productionCapabilitiesOk: smoke.productionCapabilitiesOk === true,
    secretsPrinted: smoke.secretsPrinted === true,
    checkCount: checks.length,
    checks: checks.map((item) => ({
      id: item.id,
      status: item.status,
      durationMs: item.durationMs
    }))
  }
}

const checks = []
const reports = {}
let failed = false

for (const item of commands) {
  const { check, report } = runCheck(item)
  checks.push(check)
  if (report) reports[item.id] = report
  if (check.status !== 'passed') {
    failed = true
    break
  }
}

const productRegression = summarizeProductRegression(reports['product-regression'])
const speedCacheGate = summarizeSpeedCache(reports['speed-cache-gate'])
const runtimeHealth = summarizeRuntimeHealth(reports['runtime-health-smoke'])
const deterministicPassed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const shouldRunFinalGate = strictGate && !reportOnly && deterministicPassed
const deterministicGateBlockers = strictGate && !reportOnly && !skipCommands && !deterministicPassed
  ? [
      'deterministic-default-readiness-not-passed',
      ...checks
        .filter((item) => item.status !== 'passed')
        .map((item) => `deterministic-check:${item.id}`)
    ]
  : []
const preflightGate = shouldRunFinalGate
  ? runPreflightGate()
  : {
      status: strictGate && !reportOnly && skipCommands ? 'skipped' : 'not_run',
      passed: false,
      command: 'npm run runtime:go:preflight -- --json --gate',
      durationMs: 0,
      finalGateBlocked: strictGate && !reportOnly && skipCommands,
      finalGateBlockers: strictGate && !reportOnly && skipCommands
        ? ['dry-run-cannot-authorize-cutover']
        : [],
      reason: strictGate && !reportOnly && skipCommands
        ? 'dry-run reports cannot authorize the final Go runtime gate'
        : deterministicGateBlockers.length > 0
          ? 'deterministic checks did not pass; preflight final gate was not run'
          : ''
    }
const finalGateEnabled = strictGate && !reportOnly
const finalGateBlocked = finalGateEnabled
  ? deterministicGateBlockers.length > 0 || preflightGate.finalGateBlocked === true || preflightGate.passed !== true
  : false
const finalGateBlockers = finalGateEnabled
  ? deterministicGateBlockers.length > 0
    ? deterministicGateBlockers
    : Array.isArray(preflightGate.finalGateBlockers) && preflightGate.finalGateBlockers.length > 0
    ? preflightGate.finalGateBlockers
    : finalGateBlocked
      ? ['preflight-gate-not-passed']
      : []
  : []
const blockerSummary = finalGateBlockerSummary(finalGateBlockers, {
  providerGateDetail: preflightGate.providerGateDetail,
  operatorDependencySummary: preflightGate.operatorDependencySummary,
  operatorCurrentEnvGate: preflightGate.operatorCurrentEnvGate
})
const passed = deterministicPassed && !finalGateBlocked
const status = passed
  ? 'passed'
  : skipCommands
    ? 'skipped'
    : finalGateBlocked && deterministicPassed
      ? 'live_blocked'
      : 'failed'
const finalAcceptanceReady = finalGateEnabled && passed
const notFinalUntil = finalAcceptanceReady
  ? []
  : [
      ...(finalGateEnabled ? [] : ['run with --gate so preflight final gate is executed']),
      ...finalGateBlockers
    ]

const report = {
  schemaVersion: 1,
  id: 'runtime-go-default-readiness-report',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  goDefaultReady: passed,
  deterministicDefaultReady: deterministicPassed,
  deterministicEvidenceOnly: true,
  finalAcceptanceReady,
  finalAcceptanceStatus: finalAcceptanceReady ? 'ready' : 'blocked',
  notFinalUntil,
  longRunningBenchmarkRequired: false,
  gateMode: strictGate,
  finalGateEnabled,
  finalGateBlocked,
  finalGateBlockers,
  finalGateBlockerSummary: blockerSummary,
  deterministicGateBlockers,
  preflightGate,
  runtimeHealth,
  productRegression,
  speedCacheGate,
  defaultReadiness: {
    deterministicDefaultReady: deterministicPassed,
    finalAcceptanceReady,
    finalAcceptanceStatus: finalAcceptanceReady ? 'ready' : 'blocked',
    notFinalUntil,
    finalGateEnabled,
    finalGateBlocked,
    finalGateBlockers,
    finalGateBlockerSummary: blockerSummary,
    deterministicGateBlockers,
    preflightGateStatus: preflightGate.status,
    runtimeHealthPassed: runtimeHealth.passed,
    runtimeHealthRuntimeIdentityEvidenceValid: runtimeHealth.runtimeIdentityEvidenceValid,
    runtimeHealthBoundaryEvidenceValid: runtimeHealth.boundaryEvidenceValid,
    runtimeHealthProviderAgentChainEvidenceValid: runtimeHealth.providerAgentChainEvidenceValid,
    runtimeHealthActualProcessStarted: runtimeHealth.actualRuntimeProcessStarted,
    runtimeHealthProductionRuntime: runtimeHealth.productionRuntime,
    runtimeHealthDurableRootConfigured: runtimeHealth.durableRootConfigured,
    runtimeHealthUsesTemporaryDataDir: runtimeHealth.dataDirScope === 'temporary',
    runtimeHealthUsesRealProviderConfig: runtimeHealth.usesRealProviderConfig,
    runtimeHealthProviderConfigMode: runtimeHealth.providerConfigMode,
    runtimeHealthUsesIsolatedEmptyProviderConfig: runtimeHealth.usesIsolatedEmptyProviderConfig,
    runtimeHealthUsesIsolatedContractProviderConfig: runtimeHealth.usesIsolatedContractProviderConfig,
    runtimeHealthProductChainMode: runtimeHealth.productChainMode,
    runtimeHealthProductChainOk: runtimeHealth.productChainOk,
    runtimeHealthAuthorityBoundaryOk: runtimeHealth.authorityBoundaryOk,
    runtimeHealthHostAcceptedFinalOk: runtimeHealth.hostAcceptedFinalOk,
    runtimeHealthProviderPositiveChainOk: runtimeHealth.providerPositiveChainOk,
    runtimeHealthProviderAgentLoopReady: runtimeHealth.providerAgentLoopReady,
    runtimeHealthProviderAgentLoopCoverage: runtimeHealth.providerAgentLoopCoverage,
    runtimeHealthProviderExecutionExpected: runtimeHealth.providerExecutionExpected,
    runtimeHealthProviderExecutionBlocked: runtimeHealth.providerExecutionBlocked,
    runtimeHealthProtectedTurnCreateOk: runtimeHealth.protectedTurnCreateOk,
    runtimeHealthOrdinaryTurnCreateOk: runtimeHealth.ordinaryTurnCreateOk,
    runtimeHealthProtectedSseReplayOk: runtimeHealth.protectedSseReplayOk,
    runtimeHealthOrdinarySseReplayOk: runtimeHealth.ordinarySseReplayOk,
    runtimeHealthOrdinaryThreadDistinct: runtimeHealth.ordinaryThreadDistinct,
    runtimeHealthOrdinaryPublicFinalOk: runtimeHealth.ordinaryPublicFinalOk,
    runtimeHealthTurnCreateOk: runtimeHealth.turnCreateOk,
    runtimeHealthSseReplayOk: runtimeHealth.sseReplayOk,
    runtimeHealthUsageOk: runtimeHealth.usageOk,
    runtimeHealthProviderUsageObserved: runtimeHealth.providerUsageObserved,
    runtimeHealthProviderUsageAbsentOk: runtimeHealth.providerUsageAbsentOk,
    runtimeHealthProviderDraftWithheld: runtimeHealth.providerDraftWithheld,
    runtimeHealthProtectedProviderDispatchAbsent: runtimeHealth.protectedProviderDispatchAbsent,
    runtimeHealthProtectedProviderRequestCount: runtimeHealth.protectedProviderRequestCount,
    runtimeHealthAttachmentOk: runtimeHealth.attachmentOk,
    runtimeHealthContractProviderRequestCount: runtimeHealth.contractProviderRequestCount,
    runtimeHealthContractProviderPromptSeen: runtimeHealth.contractProviderPromptSeen,
    runtimeHealthProviderAuthorizationConfigured: runtimeHealth.providerAuthorizationConfigured,
    runtimeHealthActualPackagedAppLaunched: runtimeHealth.actualPackagedAppLaunched,
    runtimeHealthRuntimeInfoOk: runtimeHealth.runtimeInfoOk,
    runtimeHealthRuntimeToolsOk: runtimeHealth.runtimeToolsOk,
    runtimeHealthProductionCapabilitiesOk: runtimeHealth.productionCapabilitiesOk,
    productRegressionPassed: productRegression.passed,
    speedCacheGatePassed: speedCacheGate.passed,
    runtimeHealthCheckCount: runtimeHealth.checkCount,
    productMatrixRowCount: productRegression.rowCount,
    speedCacheCheckCount: speedCacheGate.checkCount,
    directLegacyExposureCount: productRegression.publicLegacyExposureCount,
    internalLegacyDelegateCount: productRegression.internalLegacyDelegateCount,
    temporaryGoProdViolationCount: productRegression.temporaryGoProdViolationCount,
    productionGoSourceCount: productRegression.productionGoSourceCount,
    retiredProductionMarkerViolationCount: productRegression.retiredProductionMarkerViolationCount,
    legacyUpstreamFieldKeyViolationCount: productRegression.legacyUpstreamFieldKeyViolationCount,
    notes: [
      'The default readiness status is derived from a real temporary Go runtime health smoke, formal product-regression, and speed/cache gates.',
      'The runtime health smoke builds and starts the Go runtime-server with temporary data roots and an isolated contract provider, without reading real provider credentials.',
      'The protected Funds-unavailable turn remains a zero-call tripwire with host-accepted SourceUnavailable output and no provider usage.',
      'A distinct ordinary turn in the same temporary Go runtime proves authorized contract-provider dispatch, atomic SSE final publication, and provider usage.',
      'TypeScript backend override is retired; product regression covers retired_backend diagnostics and Go-only launcher/package boundaries.',
      'No long-running live model benchmark is required for this deterministic contract gate.'
    ]
  },
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  console.log(`runtime-health-smoke: ${runtimeHealth.passed ? 'passed' : 'failed'} (${runtimeHealth.checkCount} checks)`)
  console.log(`product-regression: ${productRegression.passed ? 'passed' : 'failed'} (${productRegression.rowCount} matrix rows)`)
  console.log(`speed-cache-gate: ${speedCacheGate.passed ? 'passed' : 'failed'} (${speedCacheGate.checkCount} checks)`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
