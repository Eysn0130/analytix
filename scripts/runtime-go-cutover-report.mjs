#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import process from 'node:process'
import {
  relevantEvidenceFinalGateBlockers,
  uniqueStrings,
  worktreeAudit,
  worktreeFinalGateBlockers
} from './runtime-go-worktree-audit.mjs'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const defaultLiveEvidencePath = 'docs/analytix/upstreams/runtime-go-live-evidence/live-evidence-report.json'
const finalCoverageEnvKeys = new Set([
  'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
  'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
  'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
  'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
  'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
  'ANALYTIX_RUNTIME_READY',
  'ANALYTIX_RUNTIME_READY_EVIDENCE'
])

const commands = [
  {
    id: 'default-readiness-report',
    command: npmCommand,
    args: ['run', 'runtime:go:default-readiness-report', '--', '--json'],
    expectsJSON: true
  },
  {
    id: 'typecheck',
    command: npmCommand,
    args: ['run', 'typecheck']
  },
  {
    id: 'build-runtime',
    command: npmCommand,
    args: ['run', 'build:runtime']
  },
  {
    id: 'diff-check',
    command: 'git',
    args: ['diff', '--check']
  }
]

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-cutover-report] ${item.id}`)
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
    env: process.env,
    stdio: 'pipe',
    encoding: 'utf8',
    maxBuffer: 128 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  const parsed = item.expectsJSON ? parseLastJSONObject(result.stdout || '') : { ok: true, value: undefined }
  const report = parsed.ok ? parsed.value : undefined
  const reportStatus = typeof report?.status === 'string' ? report.status : ''
  const passed = exitStatus === 0 && (!item.expectsJSON || reportStatus === 'passed')
  const check = {
    id: item.id,
    status: passed ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    exitStatus,
    reportId: typeof report?.id === 'string' ? report.id : '',
    reportStatus,
    reason: failureReason(item, result, parsed, exitStatus, reportStatus)
  }
  if (result.signal) check.signal = result.signal
  return { check, report }
}

function failureReason(item, result, parsed, exitStatus, reportStatus) {
  if (result.error) return result.error.message
  if (exitStatus !== 0) return `command exited with status ${exitStatus}`
  if (item.expectsJSON && !parsed.ok) return parsed.reason
  if (item.expectsJSON && reportStatus !== 'passed') return `report status is ${reportStatus || 'missing'}`
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
      // Command output may contain logs before the final machine-readable report.
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

function optionValue(name) {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) return item.slice(inlinePrefix.length)
    if (item === name && rawArgs[index + 1]) return rawArgs[index + 1]
  }
  return ''
}

function loadCoveredFinalCoverageEnv() {
  const explicitPath = optionValue('--live-evidence-json') ||
    String(process.env.ANALYTIX_RUNTIME_LIVE_EVIDENCE_REPORT || '').trim()
  const evidencePath = explicitPath || (!skipCommands ? defaultLiveEvidencePath : '')
  if (!evidencePath) return { source: 'not-configured', values: {} }
  const absolutePath = resolve(process.cwd(), evidencePath)
  const evidence = readJSONEvidence(absolutePath)
  if (!evidence.ok) return { source: 'missing', path: absolutePath, reason: evidence.reason, values: {} }
  const root = evidence.value
  if (String(root.id || '') !== 'runtime-go-live-evidence-report' ||
    root.credentialSecretsRecorded !== false) {
    return { source: 'invalid', path: absolutePath, reason: 'live evidence report is not accepted', values: {} }
  }
  const covered = objectValue(root.coveredFinalCoverageEnv)
  const values = {}
  for (const [key, value] of Object.entries(covered)) {
    if (!finalCoverageEnvKeys.has(key)) continue
    const text = String(value || '').trim()
    if (text) values[key] = text
  }
  return {
    source: Object.keys(values).length > 0 ? 'live-evidence-covered-env' : 'empty',
    path: absolutePath,
    liveEvidence: summarizeLiveEvidence(root),
    values
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

const defaultReadinessReport = reports['default-readiness-report'] || {}
const productRegression = defaultReadinessReport.productRegression && typeof defaultReadinessReport.productRegression === 'object'
  ? defaultReadinessReport.productRegression
  : {}
const speedCacheGate = defaultReadinessReport.speedCacheGate && typeof defaultReadinessReport.speedCacheGate === 'object'
  ? defaultReadinessReport.speedCacheGate
  : {}
const runtimeHealth = defaultReadinessReport.runtimeHealth && typeof defaultReadinessReport.runtimeHealth === 'object'
  ? defaultReadinessReport.runtimeHealth
  : {}
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : 'failed'
const requiredFinalCoverage = [
  'durable-restart',
  'credentialed-provider',
  'production-authority-enrolled-provider-agent-chain',
  'credentialed-mcp',
  'packaged-desktop-qa',
  'operator-gate',
  'typescript-retired-backend'
]
const coveredFinalCoverageEnv = loadCoveredFinalCoverageEnv()
const finalCoverageEnv = {
  ...coveredFinalCoverageEnv.values,
  ...process.env
}
const finalCoverage = resolveFinalCoverage({
  env: finalCoverageEnv,
  productRegression,
  runtimeHealth,
  passed
})
const worktree = worktreeAudit()
const liveEvidenceFinalGateBlockers = Array.isArray(coveredFinalCoverageEnv.liveEvidence?.finalGateBlockers)
  ? coveredFinalCoverageEnv.liveEvidence.finalGateBlockers
  : []
const finalGateBlockers = uniqueStrings([
  ...liveEvidenceFinalGateBlockers,
  ...relevantEvidenceFinalGateBlockers(worktree),
  ...worktreeFinalGateBlockers(worktree)
])
const finalGateBlocked = finalGateBlockers.length > 0
const finalAcceptanceReady = passed && finalCoverage.missingFinalCoverage.length === 0 && !finalGateBlocked
const finalAcceptanceStatus = finalAcceptanceReady
  ? 'ready'
  : passed && !finalGateBlocked
    ? 'partial'
    : 'blocked'
const finalAcceptanceReason = finalAcceptanceReady
  ? 'deterministic checks and final live evidence are covered'
  : !passed
    ? 'deterministic cutover checks did not pass'
    : finalGateBlocked
      ? 'deterministic checks passed, but preflight final blockers remain'
      : 'deterministic checks passed, but final live credentialed/package/operator coverage is missing'

const report = {
  schemaVersion: 1,
  id: 'runtime-go-cutover-report',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  goRuntimeDefaultCutoverReady: passed,
  deterministicCutoverReady: passed,
  finalAcceptanceReady,
  finalAcceptanceStatus,
  finalStatus: finalAcceptanceStatus,
  finalAcceptanceReason,
  missingFinalCoverage: finalCoverage.missingFinalCoverage,
  finalGateBlocked,
  finalGateBlockers,
  worktree,
  deterministicEvidenceOnly: finalCoverage.missingFinalCoverage.length > 0,
  longRunningBenchmarkRequired: false,
  runtimeHealth,
  finalCoverageEvidenceEnvSource: {
    source: coveredFinalCoverageEnv.source,
    path: coveredFinalCoverageEnv.path || '',
    keys: Object.keys(coveredFinalCoverageEnv.values).sort()
  },
  finalCoverageLiveEvidence: coveredFinalCoverageEnv.liveEvidence || {
    source: coveredFinalCoverageEnv.source,
    path: coveredFinalCoverageEnv.path || '',
    status: 'not-available',
    passed: false,
    componentStatus: {},
    missingExternalInputs: []
  },
  finalCoverage: finalCoverage.rows,
  cutover: {
    defaultReadinessReady: defaultReadinessReport.goDefaultReady === true,
    runtimeHealthPassed: runtimeHealth.passed === true,
    runtimeHealthRuntimeIdentityEvidenceValid: runtimeHealth.runtimeIdentityEvidenceValid === true,
    runtimeHealthBoundaryEvidenceValid: runtimeHealth.boundaryEvidenceValid === true,
    runtimeHealthProviderAgentChainEvidenceValid: runtimeHealth.providerAgentChainEvidenceValid === true,
    runtimeHealthActualProcessStarted: runtimeHealth.actualRuntimeProcessStarted === true,
    runtimeHealthProductionRuntime: runtimeHealth.productionRuntime === true,
    runtimeHealthDurableRootConfigured: runtimeHealth.durableRootConfigured === true,
    runtimeHealthUsesTemporaryDataDir: runtimeHealth.dataDirScope === 'temporary',
    runtimeHealthUsesRealProviderConfig: runtimeHealth.usesRealProviderConfig === true,
    runtimeHealthProviderConfigMode: typeof runtimeHealth.providerConfigMode === 'string'
      ? runtimeHealth.providerConfigMode
      : 'not-run',
    runtimeHealthUsesIsolatedEmptyProviderConfig: runtimeHealth.usesIsolatedEmptyProviderConfig === true,
    runtimeHealthUsesIsolatedContractProviderConfig: runtimeHealth.usesIsolatedContractProviderConfig === true,
    runtimeHealthProductChainMode: typeof runtimeHealth.productChainMode === 'string'
      ? runtimeHealth.productChainMode
      : 'not-run',
    runtimeHealthProductChainOk: runtimeHealth.productChainOk === true,
    runtimeHealthAuthorityBoundaryOk: runtimeHealth.authorityBoundaryOk === true,
    runtimeHealthHostAcceptedFinalOk: runtimeHealth.hostAcceptedFinalOk === true,
    runtimeHealthProviderPositiveChainOk: runtimeHealth.providerPositiveChainOk === true,
    runtimeHealthProviderAgentLoopReady: runtimeHealth.providerAgentLoopReady === true,
    runtimeHealthProviderAgentLoopCoverage: typeof runtimeHealth.providerAgentLoopCoverage === 'string'
      ? runtimeHealth.providerAgentLoopCoverage
      : 'not-run',
    runtimeHealthProviderExecutionExpected: runtimeHealth.providerExecutionExpected === true,
    runtimeHealthProviderExecutionBlocked: runtimeHealth.providerExecutionBlocked === true,
    runtimeHealthProtectedTurnCreateOk: runtimeHealth.protectedTurnCreateOk === true,
    runtimeHealthOrdinaryTurnCreateOk: runtimeHealth.ordinaryTurnCreateOk === true,
    runtimeHealthProtectedSseReplayOk: runtimeHealth.protectedSseReplayOk === true,
    runtimeHealthOrdinarySseReplayOk: runtimeHealth.ordinarySseReplayOk === true,
    runtimeHealthOrdinaryThreadDistinct: runtimeHealth.ordinaryThreadDistinct === true,
    runtimeHealthOrdinaryPublicFinalOk: runtimeHealth.ordinaryPublicFinalOk === true,
    runtimeHealthTurnCreateOk: runtimeHealth.turnCreateOk === true,
    runtimeHealthSseReplayOk: runtimeHealth.sseReplayOk === true,
    runtimeHealthUsageOk: runtimeHealth.usageOk === true,
    runtimeHealthProviderUsageObserved: runtimeHealth.providerUsageObserved === true,
    runtimeHealthProviderUsageAbsentOk: runtimeHealth.providerUsageAbsentOk === true,
    runtimeHealthProviderDraftWithheld: runtimeHealth.providerDraftWithheld === true,
    runtimeHealthProtectedProviderDispatchAbsent: runtimeHealth.protectedProviderDispatchAbsent === true,
    runtimeHealthProtectedProviderRequestCount: Number.isFinite(runtimeHealth.protectedProviderRequestCount)
      ? runtimeHealth.protectedProviderRequestCount
      : null,
    runtimeHealthAttachmentOk: runtimeHealth.attachmentOk === true,
    runtimeHealthContractProviderRequestCount: Number.isFinite(runtimeHealth.contractProviderRequestCount)
      ? runtimeHealth.contractProviderRequestCount
      : null,
    runtimeHealthContractProviderPromptSeen: runtimeHealth.contractProviderPromptSeen === true,
    runtimeHealthProviderAuthorizationConfigured: runtimeHealth.providerAuthorizationConfigured === true,
    runtimeHealthActualPackagedAppLaunched: runtimeHealth.actualPackagedAppLaunched === true,
    runtimeHealthHealthOk: runtimeHealth.healthOk === true,
    runtimeHealthRuntimeInfoOk: runtimeHealth.runtimeInfoOk === true,
    runtimeHealthRuntimeToolsOk: runtimeHealth.runtimeToolsOk === true,
    runtimeHealthProductionCapabilitiesOk: runtimeHealth.productionCapabilitiesOk === true,
    runtimeHealthSecretsPrinted: runtimeHealth.secretsPrinted === true,
    requiredCoverage: requiredFinalCoverage,
    finalAcceptanceReady,
    finalAcceptanceStatus,
    finalStatus: finalAcceptanceStatus,
    finalAcceptanceReason,
    missingFinalCoverage: finalCoverage.missingFinalCoverage,
    finalGateBlocked,
    finalGateBlockers,
    worktreeFinalAcceptanceRequiresCleanOrClassifiedTree: worktree.finalAcceptanceRequiresCleanOrClassifiedTree === true,
    productRegressionPassed: productRegression.passed === true,
    speedCacheGatePassed: speedCacheGate.passed === true,
    typecheckPassed: checks.find((item) => item.id === 'typecheck')?.status === 'passed',
    buildRuntimePassed: checks.find((item) => item.id === 'build-runtime')?.status === 'passed',
    diffCheckPassed: checks.find((item) => item.id === 'diff-check')?.status === 'passed',
    productMatrixRowCount: Number.isFinite(productRegression.rowCount) ? productRegression.rowCount : 0,
    speedCacheCheckCount: Number.isFinite(speedCacheGate.checkCount) ? speedCacheGate.checkCount : 0,
    directLegacyExposureCount: Number.isFinite(productRegression.publicLegacyExposureCount)
      ? productRegression.publicLegacyExposureCount
      : null,
    internalLegacyDelegateCount: Number.isFinite(productRegression.internalLegacyDelegateCount)
      ? productRegression.internalLegacyDelegateCount
      : null,
    temporaryGoProdViolationCount: Number.isFinite(productRegression.temporaryGoProdViolationCount)
      ? productRegression.temporaryGoProdViolationCount
      : null,
    productionGoSourceCount: Number.isFinite(productRegression.productionGoSourceCount)
      ? productRegression.productionGoSourceCount
      : null,
    retiredProductionMarkerViolationCount: Number.isFinite(productRegression.retiredProductionMarkerViolationCount)
      ? productRegression.retiredProductionMarkerViolationCount
      : null,
    legacyUpstreamFieldKeyViolationCount: Number.isFinite(productRegression.legacyUpstreamFieldKeyViolationCount)
      ? productRegression.legacyUpstreamFieldKeyViolationCount
      : null
  },
  rollback: {
    explicitOverride: 'ANALYTIX_RUNTIME_BACKEND=typescript is retired and must return a retired_backend diagnostic.',
    defaultPath: 'Go runtime is the only production agent runtime when the override is absent.',
    coveredByProductMatrix: Array.isArray(productRegression.features) &&
      productRegression.features.includes('TypeScript retired backend')
  },
  checks
}

function resolveFinalCoverage({ env, productRegression, runtimeHealth, passed }) {
  const rows = [
    finalStatusOnlyRow({
      id: 'durable-restart',
      env,
      statusKeys: [
        'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
        'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS'
      ],
      coveredReason: 'Durable restart/session soak status accepted'
    }),
    finalEvidenceRow({
      id: 'credentialed-provider',
      env,
      statusKeys: [
        'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
        'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS'
      ],
      evidenceKeys: [
        'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
        'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE'
      ],
      validate: validateProviderEvidence
    }),
    resolveProductionAuthorityProviderAgentChainCoverage({ runtimeHealth, passed }),
    finalEvidenceRow({
      id: 'credentialed-mcp',
      env,
      statusKeys: [
        'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
        'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS'
      ],
      evidenceKeys: [
        'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
        'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE'
      ],
      validate: validateMCPEvidence
    }),
    finalEvidenceRow({
      id: 'packaged-desktop-qa',
      env,
      statusKeys: [
        'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
        'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS'
      ],
      evidenceKeys: [
        'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
        'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE'
      ],
      validate: validatePackagedEvidence
    }),
    finalEvidenceRow({
      id: 'operator-gate',
      env,
      statusKeys: [
        'ANALYTIX_RUNTIME_READY'
      ],
      evidenceKeys: [
        'ANALYTIX_RUNTIME_READY_EVIDENCE',
        'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE'
      ],
      validate: validateOperatorEvidence
    }),
    resolveTypeScriptRetiredBackendCoverage({ productRegression, runtimeHealth, passed })
  ]
  return {
    rows,
    missingFinalCoverage: rows.filter((row) => row.status !== 'covered').map((row) => row.id)
  }
}

function resolveProductionAuthorityProviderAgentChainCoverage({ runtimeHealth, passed }) {
  const requestCount = runtimeHealth.contractProviderRequestCount
  const covered = passed &&
    runtimeHealth.runtimeIdentityEvidenceValid === true &&
    runtimeHealth.boundaryEvidenceValid === true &&
    runtimeHealth.providerAgentChainEvidenceValid === true &&
    runtimeHealth.actualRuntimeProcessStarted === true &&
    runtimeHealth.productionRuntime === true &&
    runtimeHealth.durableRootConfigured === true &&
    runtimeHealth.dataDirScope === 'temporary' &&
    runtimeHealth.usesRealProviderConfig === false &&
    runtimeHealth.providerConfigMode === 'isolated-contract-provider-config' &&
    runtimeHealth.usesIsolatedContractProviderConfig === true &&
    runtimeHealth.healthOk === true &&
    runtimeHealth.runtimeInfoOk === true &&
    runtimeHealth.runtimeToolsOk === true &&
    runtimeHealth.productionCapabilitiesOk === true &&
    runtimeHealth.secretsPrinted === false &&
    runtimeHealth.productChainMode === 'production-authority-enrolled-provider-agent-chain' &&
    runtimeHealth.productChainOk === true &&
    runtimeHealth.authorityBoundaryOk === true &&
    runtimeHealth.hostAcceptedFinalOk === true &&
    runtimeHealth.providerPositiveChainOk === true &&
    runtimeHealth.providerAgentLoopReady === true &&
    runtimeHealth.providerAgentLoopCoverage === 'ordinary-turn-with-protected-boundary' &&
    runtimeHealth.providerExecutionExpected === true &&
    runtimeHealth.providerExecutionBlocked === false &&
    runtimeHealth.usageOk === true &&
    runtimeHealth.providerUsageObserved === true &&
    runtimeHealth.providerUsageAbsentOk === true &&
    runtimeHealth.protectedProviderDispatchAbsent === true &&
    runtimeHealth.protectedProviderRequestCount === 0 &&
    runtimeHealth.providerDraftWithheld === true &&
    runtimeHealth.protectedTurnCreateOk === true &&
    runtimeHealth.ordinaryTurnCreateOk === true &&
    runtimeHealth.protectedSseReplayOk === true &&
    runtimeHealth.ordinarySseReplayOk === true &&
    runtimeHealth.ordinaryThreadDistinct === true &&
    runtimeHealth.ordinaryPublicFinalOk === true &&
    runtimeHealth.turnCreateOk === true &&
    runtimeHealth.sseReplayOk === true &&
    runtimeHealth.attachmentOk === true &&
    runtimeHealth.contractProviderPromptSeen === true &&
    runtimeHealth.providerAuthorizationConfigured === true &&
    Number.isInteger(requestCount) &&
    requestCount === 1
  return {
    id: 'production-authority-enrolled-provider-agent-chain',
    status: covered ? 'covered' : 'missing',
    statusEnv: '',
    statusValue: covered ? 'passed' : 'missing',
    evidenceEnv: '',
    evidencePath: '',
    evidencePresent: runtimeHealth.passed === true,
    reason: covered
      ? 'Production authority-enrolled provider Agent chain covered by current runtime health evidence'
      : runtimeHealth.productChainMode === 'production-authority-boundary'
        ? 'Runtime health is boundary-only because provider dispatch is blocked before production authority enrollment'
        : 'Production authority-enrolled provider Agent chain evidence is not available'
  }
}

function finalStatusOnlyRow({ id, env, statusKeys, coveredReason }) {
  const statusEnv = firstEnv(env, statusKeys)
  const covered = envStatusPassed(statusEnv.value)
  return {
    id,
    status: covered ? 'covered' : 'missing',
    statusEnv: statusEnv.key || statusKeys[0],
    statusValue: statusEnv.value || 'missing',
    evidenceEnv: '',
    evidencePath: '',
    evidencePresent: false,
    reason: covered
      ? coveredReason
      : `${statusEnv.key || statusKeys[0]} is not passed`
  }
}

function finalEvidenceRow({ id, env, statusKeys, evidenceKeys, validate }) {
  const statusEnv = firstEnv(env, statusKeys)
  const evidenceEnv = firstEnv(env, evidenceKeys)
  const statusPassed = envStatusPassed(statusEnv.value)
  const evidencePath = evidenceEnv.value ? resolve(process.cwd(), evidenceEnv.value) : ''
  const evidence = readJSONEvidence(evidencePath)
  const validation = evidence.ok ? validate(evidence.value) : { ok: false, reason: evidence.reason }
  const covered = statusPassed && validation.ok
  return {
    id,
    status: covered ? 'covered' : 'missing',
    statusEnv: statusEnv.key || statusKeys[0],
    statusValue: statusEnv.value || 'missing',
    evidenceEnv: evidenceEnv.key || evidenceKeys[0],
    evidencePath,
    evidencePresent: evidence.ok,
    reason: covered
      ? 'credentialed evidence accepted'
      : !statusPassed
        ? `${statusEnv.key || statusKeys[0]} is not passed`
        : validation.reason
  }
}

function resolveTypeScriptRetiredBackendCoverage({ productRegression, runtimeHealth, passed }) {
  const features = Array.isArray(productRegression.features) ? productRegression.features : []
  const productMatrixCoversRetirement = productRegression.passed === true &&
    features.includes('TypeScript retired backend')
  const runtimeHealthCoversRetirement = runtimeHealth.typeScriptRetiredBackendPassed === true ||
    runtimeHealth.typeScriptOverrideRetired === true
  const covered = passed && (productMatrixCoversRetirement || runtimeHealthCoversRetirement)
  return {
    id: 'typescript-retired-backend',
    status: covered ? 'covered' : 'missing',
    statusEnv: '',
    statusValue: covered ? 'passed' : 'missing',
    evidenceEnv: '',
    evidencePath: '',
    evidencePresent: productMatrixCoversRetirement || runtimeHealthCoversRetirement,
    reason: covered
      ? 'TypeScript retired backend covered by deterministic product matrix/runtime health evidence'
      : 'TypeScript retired-backend evidence is not available in the cutover report'
  }
}

function firstEnv(env, keys) {
  for (const key of keys) {
    const value = String(env[key] || '').trim()
    if (value) return { key, value }
  }
  return { key: '', value: '' }
}

function envStatusPassed(value) {
  return ['passed', 'pass', 'ok', '1'].includes(String(value || '').trim().toLowerCase())
}

function readJSONEvidence(filePath) {
  if (!filePath) return { ok: false, reason: 'evidence path is not set' }
  if (!existsSync(filePath)) return { ok: false, reason: 'evidence path does not exist' }
  try {
    const value = JSON.parse(readFileSync(filePath, 'utf8'))
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return { ok: false, reason: 'evidence JSON root must be an object' }
    }
    return { ok: true, value }
  } catch (error) {
    return { ok: false, reason: `evidence JSON is unreadable: ${error instanceof Error ? error.message : String(error)}` }
  }
}

function summarizeLiveEvidence(root) {
  return {
    id: String(root.id || ''),
    source: 'live-evidence-report',
    status: String(root.status || ''),
    passed: root.passed === true,
    finalGateBlocked: root.finalGateBlocked === true,
    finalGateBlockers: arrayValue(root.finalGateBlockers).map((item) => String(item || '')).filter(Boolean),
    credentialSecretsRecorded: root.credentialSecretsRecorded === true,
    componentStatus: summarizeComponentStatus(root.componentStatus),
    providerSettingsProfileCoverage: summarizeProviderSettingsProfileCoverage(root),
    missingExternalInputs: arrayValue(root.missingExternalInputs)
      .map(summarizeMissingExternalInput)
      .filter((item) => item.id || item.reason),
    coveredFinalCoverageEnvKeys: Object.keys(objectValue(root.coveredFinalCoverageEnv)).sort(),
    strictGateEnvWhenPassedKeys: Object.keys(objectValue(root.strictGateEnvWhenPassed)).sort()
  }
}

function summarizeProviderSettingsProfileCoverage(root) {
  const nextEvidenceActions = objectValue(root.nextEvidenceActions)
  const providerMatrix = objectValue(nextEvidenceActions.providerMatrix)
  const coverage = objectValue(providerMatrix.settingsProfileCoverage)
  return {
    status: String(coverage.status || ''),
    source: String(coverage.source || ''),
    providerCount: Number.isFinite(coverage.providerCount) ? coverage.providerCount : 0,
    matchedProviderIds: safeProviderIds(coverage.matchedProviderIds),
    usableProviderIds: safeProviderIds(coverage.usableProviderIds),
    usableNonDeepSeekProviderIds: safeProviderIds(coverage.usableNonDeepSeekProviderIds).filter((id) => id !== 'deepseek'),
    deepseekSettingsProfileUsable: coverage.deepseekSettingsProfileUsable === true,
    nonDeepSeekSettingsProfileUsable: coverage.nonDeepSeekSettingsProfileUsable === true,
    satisfiesFinalProviderCoverageFromSettings: coverage.satisfiesFinalProviderCoverageFromSettings === true,
    credentialValuesRecorded: coverage.credentialValuesRecorded === true
  }
}

function safeProviderIds(value) {
  const known = new Set(['deepseek', 'openai-compatible', 'anthropic-compatible', 'custom-endpoint'])
  return arrayValue(value).map((item) => String(item || '')).filter((id) => known.has(id)).sort()
}

function summarizeComponentStatus(value) {
  const root = objectValue(value)
  const status = {}
  for (const key of ['provider', 'mcp', 'packaged', 'packagedGui', 'packagedSoak', 'operator']) {
    const text = String(root[key] || '').trim()
    if (text) status[key] = text
  }
  return status
}

function summarizeMissingExternalInput(value) {
  const root = objectValue(value)
  return {
    id: String(root.id || ''),
    status: String(root.status || ''),
    reason: String(root.reason || ''),
    missingEnv: arrayValue(root.missingEnv).map((item) => String(item || '')).filter(Boolean),
    missingSettingsProfileFields: arrayValue(root.missingSettingsProfileFields)
      .map((item) => String(item || ''))
      .filter(isKnownMissingSettingsProfileField)
      .sort()
  }
}

function isKnownMissingSettingsProfileField(value) {
  return new Set([
    'provider.providers[].matchingProfile',
    'provider.providers[].apiKey',
    'provider.providers[].baseUrl',
    'provider.providers[].models or runtime.model'
  ]).has(String(value || ''))
}

function validateProviderEvidence(root) {
  const redaction = objectValue(root.redaction)
  if (root.id !== 'runtime-go-provider-matrix') {
    return { ok: false, reason: 'provider evidence id is not a runtime-go provider matrix report' }
  }
  if (root.status !== 'passed' || root.passed !== true) {
    return { ok: false, reason: 'provider evidence is not passed' }
  }
  if (root.credentialSecretsRecorded !== false ||
    root.credentialedMatrixEnvGated !== true ||
    root.credentialValuesRecorded === true ||
    redaction.status !== 'passed' ||
    redaction.secretMaterialFound !== false) {
    return { ok: false, reason: 'provider evidence is not credentialed/redacted root evidence' }
  }
  const coverage = providerCredentialedCoverage(root)
  if (!coverage.deepseekPassed) {
    return { ok: false, reason: 'provider evidence missing credentialed DeepSeek probe' }
  }
  if (!coverage.nonDeepSeekProviderPassed) {
    return { ok: false, reason: 'provider evidence missing at least one credentialed non-DeepSeek probe' }
  }
  const missingProbeEvidence = providerPassedProbeEvidenceMissing(root)
  if (missingProbeEvidence.length > 0) {
    return { ok: false, reason: `provider evidence missing probe details: ${missingProbeEvidence.join(', ')}` }
  }
  return { ok: true, reason: 'provider evidence accepted' }
}

function providerCredentialedCoverage(root) {
  const passedIds = arrayValue(root.credentialedProbes)
    .map(objectValue)
    .filter((probe) => probe.status === 'passed' && probe.skipped !== true && probe.credentialed === true)
    .map((probe) => String(probe.id || ''))
    .filter(Boolean)
  return {
    deepseekPassed: passedIds.includes('deepseek'),
    nonDeepSeekProviderPassed: passedIds.some((id) => id !== 'deepseek'),
    passedIds
  }
}

function providerPassedProbeEvidenceMissing(root) {
  const missing = []
  const passedProbes = arrayValue(root.credentialedProbes)
    .map(objectValue)
    .filter((probe) => probe.status === 'passed' && probe.skipped !== true && probe.credentialed === true)
  for (const probe of passedProbes) {
    const id = String(probe.id || 'provider')
    const requestShape = objectValue(probe.requestShape)
    const requestShapeValidation = objectValue(probe.requestShapeValidation)
    const streamParsing = objectValue(probe.streamParsing)
    const usageParsing = objectValue(probe.usageParsing)
    const cacheTelemetry = objectValue(probe.cacheTelemetry)
    const errorHandling = objectValue(probe.errorHandling)
    const errorRequestShape = objectValue(errorHandling.requestShape)
    const redaction = objectValue(probe.redaction)
    const provider = objectValue(probe.provider)
    if (requestShapeValidation.status !== 'passed') missing.push(`${id}.requestShapeValidation`)
    if (requestShape.credentialValueRecorded !== false) missing.push(`${id}.requestShape.redaction`)
    if (requestShape.credentialConfigured !== true) missing.push(`${id}.requestShape.credentialConfigured`)
    if (streamParsing.status !== 'passed' || streamParsing.mode !== 'sse') missing.push(`${id}.streamParsing`)
    if (usageParsing.status !== 'passed' || usageParsing.usageObjectPresent !== true) missing.push(`${id}.usageParsing`)
    if (cacheTelemetry.status !== 'passed') missing.push(`${id}.cacheTelemetry`)
    if (errorHandling.status !== 'passed' || errorHandling.errorProbePassed !== true) {
      missing.push(`${id}.errorHandling`)
    }
    if (errorHandling.rawResponseRecorded !== false || errorHandling.credentialValueRecorded !== false) {
      missing.push(`${id}.errorHandlingRedaction`)
    }
    if (errorRequestShape.credentialValueRecorded !== false || errorRequestShape.requestBodyRecorded !== false) {
      missing.push(`${id}.errorRequestRedaction`)
    }
    if (redaction.status !== 'passed' || redaction.credentialValueRecorded !== false) {
      missing.push(`${id}.redaction`)
    }
    if (provider.hasApiKey !== true || String(provider.source || '') === 'missing') {
      missing.push(`${id}.providerCredentialSource`)
    }
  }
  return missing
}

function validateMCPEvidence(root) {
  if (root.id !== 'runtime-go-mcp-execution') {
    return { ok: false, reason: 'MCP evidence id is not a runtime-go MCP execution report' }
  }
  if (root.status !== 'passed' || root.passed !== true || root.credentialedExecution !== true) {
    return { ok: false, reason: 'MCP evidence is not passed credentialed execution' }
  }
  if (root.topLevelMcpIndexerExposed !== false || root.reasonixPublicProtocolUsed !== false) {
    return { ok: false, reason: 'MCP evidence exposes forbidden product surfaces' }
  }
  const probe = arrayValue(root.credentialedProbes).map(objectValue).find((item) => item.id === 'credentialed-mcp')
  if (!probe) return { ok: false, reason: 'MCP evidence missing credentialed-mcp probe' }
  const required = [
    'connect',
    'toolDiscoverySearch',
    'toolCall',
    'approvalUserInput',
    'reconnect'
  ]
  const missing = required.filter((key) => probe[key] !== true)
  if (probe.status !== 'passed' || probe.skipped === true || probe.credentialedExecution !== true) {
    missing.push('credentialed-pass')
  }
  if (probe.credentialRedaction !== true && probe.redaction !== true) missing.push('credentialRedaction')
  if (probe.commandConfigured !== true && probe.urlConfigured !== true) missing.push('command-or-url-configured')
  const requestShape = objectValue(probe.requestShape)
  if (requestShape.credentialValueRecorded !== false) missing.push('requestShape.credentialValueRecorded:false')
  if (requestShape.toolArgsValueRecorded !== false) missing.push('requestShape.toolArgsValueRecorded:false')
  const methods = arrayValue(requestShape.methods).map((item) => String(item))
  for (const method of ['initialize', 'tools/list', 'tools/call']) {
    if (!methods.includes(method)) missing.push(`requestShape.methods:${method}`)
  }
  if (!isSha256Hex(probe.calledToolNameHash)) missing.push('calledToolNameHash')
  if (!isSha256Hex(probe.toolCatalogDigest)) missing.push('toolCatalogDigest')
  if (!isSha256Hex(probe.reconnectToolNameHash)) missing.push('reconnectToolNameHash')
  if (!isSha256Hex(probe.reconnectToolCatalogDigest)) missing.push('reconnectToolCatalogDigest')
  const approvalEvidence = objectValue(probe.approvalUserInputEvidence)
  if (approvalEvidence.status !== 'passed' ||
    !isSha256Hex(approvalEvidence.evidenceSha256) ||
    approvalEvidence.rawValueRecorded !== false ||
    approvalEvidence.credentialSecretsRecorded !== false) {
    missing.push('approvalUserInputEvidence')
  }
  if (missing.length > 0) return { ok: false, reason: `MCP evidence missing coverage: ${missing.join(', ')}` }
  return { ok: true, reason: 'MCP evidence accepted' }
}

function isSha256Hex(value) {
  return /^[a-f0-9]{64}$/.test(String(value || ''))
}

function isGitCommit(value) {
  return /^[a-f0-9]{40}$/.test(String(value || ''))
}

function validatePackagedEvidence(root) {
  if (root.id !== 'runtime-go-packaged-qa') {
    return { ok: false, reason: 'packaged evidence id is not a runtime-go packaged QA report' }
  }
  if (root.status !== 'passed' || root.passed !== true) {
    return { ok: false, reason: 'packaged evidence is not passed' }
  }
  if (isAggregatePackagedQAReport(root)) {
    const missing = aggregatePackagedEvidenceMissing(root)
    if (missing.length > 0) {
      return { ok: false, reason: `packaged evidence missing aggregate coverage: ${missing.join(', ')}` }
    }
    return { ok: true, reason: 'packaged evidence accepted' }
  }
  const required = new Set([
    'packaged-app-startup',
    'health',
    'runtime-info',
    'thread-list',
    'turn-create',
    'sse-replay',
    'go-runtime-default-gate',
    'typescript-retired-backend'
  ])
  for (const check of arrayValue(root.checks).map(objectValue)) {
    if (check.status === 'passed') required.delete(String(check.id || ''))
  }
  if (required.size > 0) {
    return { ok: false, reason: `packaged evidence missing checks: ${Array.from(required).join(', ')}` }
  }
  return { ok: true, reason: 'packaged evidence accepted' }
}

function isAggregatePackagedQAReport(root) {
  if (root.id !== 'runtime-go-packaged-qa') return false
  const packaged = objectValue(root.packaged)
  if (Object.keys(packaged).length === 0) return false
  return arrayValue(root.checks).map(objectValue).some((item) => item.id === 'cutover-report')
}

function aggregatePackagedEvidenceMissing(root) {
  const missing = []
  if (root.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  const redaction = objectValue(root.redaction)
  if (redaction.status !== 'passed' || redaction.secretMaterialFound !== false) missing.push('redaction')

  const requiredCheckIds = [
    'cutover-report',
    'gui-smoke',
    'session-soak',
    'rollback-evidence',
    'packaging-config'
  ]
  const checks = arrayValue(root.checks).map(objectValue)
  for (const id of requiredCheckIds) {
    const matching = checks.filter((item) => item.id === id)
    if (matching.length !== 1) {
      missing.push(`checks:${id}:unique`)
      continue
    }
    if (matching[0].status !== 'passed') missing.push(`checks:${id}:status`)
  }

  const packaged = objectValue(root.packaged)
  if (packaged.actualPackagedDesktopQAPassed !== true) missing.push('packaged.actualPackagedDesktopQAPassed')
  if (packaged.actualPackagedAppLaunched !== true) missing.push('packaged.actualPackagedAppLaunched')
  if (packaged.actualPackagedSmokeRequested !== true) missing.push('packaged.actualPackagedSmokeRequested')
  if (packaged.actualPackagedSmokePassed !== true) missing.push('packaged.actualPackagedSmokePassed')
  if (packaged.actualPackagedAppEvidenceRequiredForFinalGate !== false) {
    missing.push('packaged.actualPackagedAppEvidenceRequiredForFinalGate:false')
  }
  if (packaged.guiSmokePassed !== true) missing.push('packaged.guiSmokePassed')
  if (packaged.sessionSoakPassed !== true) missing.push('packaged.sessionSoakPassed')
  if (packaged.typeScriptRetiredBackendPassed !== true) missing.push('packaged.typeScriptRetiredBackendPassed')
  if (packaged.packagingConfigPassed !== true) missing.push('packaged.packagingConfigPassed')
  if (packaged.settingsProfilePatchAccepted !== true) missing.push('packaged.settingsProfilePatchAccepted')
  if (packaged.settingsCompatPatchUsed !== false) missing.push('packaged.settingsCompatPatchUsed:false')
  if (packaged.runtimeHealthPassed !== true) missing.push('packaged.runtimeHealthPassed')
  if (packaged.runtimeHealthRuntimeInfoOk !== true) missing.push('packaged.runtimeHealthRuntimeInfoOk')
  if (packaged.runtimeHealthRuntimeToolsOk !== true) missing.push('packaged.runtimeHealthRuntimeToolsOk')
  if (packaged.runtimeHealthProductionCapabilitiesOk !== true) {
    missing.push('packaged.runtimeHealthProductionCapabilitiesOk')
  }
  if (packaged.directLegacyExposureCount !== 0) missing.push('packaged.directLegacyExposureCount:0')
  if (packaged.temporaryGoProdViolationCount !== 0) missing.push('packaged.temporaryGoProdViolationCount:0')
  if (packaged.retiredProductionMarkerViolationCount !== 0) {
    missing.push('packaged.retiredProductionMarkerViolationCount:0')
  }
  if (packaged.legacyUpstreamFieldKeyViolationCount !== 0) {
    missing.push('packaged.legacyUpstreamFieldKeyViolationCount:0')
  }
  const missingFinalCoverage = arrayValue(packaged.missingFinalCoverage).map((item) => String(item))
  if (missingFinalCoverage.includes('packaged-desktop-qa')) {
    missing.push('packaged.missingFinalCoverage:packaged-desktop-qa')
  }
  return missing
}

function validateOperatorEvidence(root) {
  const expectedGate = 'ANALYTIX_RUNTIME_READY=1'
  const envGate = root.operatorGate === expectedGate && root.explicitEnvGate === true
  const legacyGoDefaultApprovedKey = ['goDefault', 'Candidate', 'Approved'].join('')
  const legacyDefaultApprovedKey = ['default', 'Candidate', 'Approved'].join('')
  if (root.id !== 'runtime-go-operator-gate') {
    return { ok: false, reason: 'operator evidence id is not accepted' }
  }
  if (root.status !== 'passed' || root.passed !== true || !envGate) {
    return { ok: false, reason: 'operator evidence is not passed with explicit ANALYTIX_RUNTIME_READY gate' }
  }
  if (root.credentialedEvidenceReviewed !== true ||
    (root.goDefaultApproved !== true &&
      root.defaultRuntimeApproved !== true &&
      root[legacyGoDefaultApprovedKey] !== true &&
      root[legacyDefaultApprovedKey] !== true) ||
    root.typeScriptFallbackRetained === true ||
    root.goDefaultBackendEnabled !== true) {
    return { ok: false, reason: 'operator evidence did not approve reviewed Go default evidence' }
  }
  const missing = operatorReviewEvidenceMissing(root)
  if (missing.length > 0) {
    return { ok: false, reason: `operator evidence missing review details: ${missing.join(', ')}` }
  }
  return { ok: true, reason: 'operator evidence accepted' }
}

function operatorReviewEvidenceMissing(root) {
  const missing = []
  if (!isGitCommit(root.commitHash)) missing.push('commitHash')
  if (!isGitCommit(root.evidenceTargetCommit)) missing.push('evidenceTargetCommit')
  if (root.evidenceDigestAlgorithm !== 'sha256:canonical-json-v1') missing.push('evidenceDigestAlgorithm')
  const evidenceDigests = objectValue(root.evidenceDigests)
  for (const key of ['provider', 'mcp', 'packaged']) {
    if (!isSha256Hex(evidenceDigests[key])) missing.push(`evidenceDigests.${key}`)
  }
  const reportPaths = objectValue(root.reportPaths)
  for (const key of ['provider', 'mcp', 'packaged']) {
    if (!String(reportPaths[key] || '').trim()) missing.push(`reportPaths.${key}`)
  }
  const reviewed = arrayValue(root.evidenceReviewed).map(objectValue)
  for (const id of ['provider-matrix-credentialed', 'mcp-matrix-credentialed', 'packaged-qa']) {
    const row = reviewed.find((item) => item.id === id)
    if (!row) {
      missing.push(`evidenceReviewed.${id}`)
    } else if (row.status !== 'passed' || !String(row.path || '').trim()) {
      missing.push(`evidenceReviewed.${id}:passed`)
    }
  }
  if (root.rendererVisibleGoSwitcher !== false) missing.push('rendererVisibleGoSwitcher:false')
  if (root.reasonixEngineRuntimeOnly !== true) missing.push('reasonixEngineRuntimeOnly:true')
  return missing
}

function objectValue(value) {
  return value && typeof value === 'object' && !Array.isArray(value) ? value : {}
}

function arrayValue(value) {
  return Array.isArray(value) ? value : []
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  console.log(`default-readiness: ${report.cutover.defaultReadinessReady ? 'passed' : 'failed'}`)
  console.log(`runtime-health-smoke: ${report.cutover.runtimeHealthPassed ? 'passed' : 'failed'}`)
  console.log(`typecheck: ${report.cutover.typecheckPassed ? 'passed' : 'failed'}`)
  console.log(`build-runtime: ${report.cutover.buildRuntimePassed ? 'passed' : 'failed'}`)
  console.log(`diff-check: ${report.cutover.diffCheckPassed ? 'passed' : 'failed'}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
