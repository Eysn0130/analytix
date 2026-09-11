#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const auditPath = 'docs/analytix/upstreams/kun-reasonix-feature-delta-audit.md'

const commands = [
  {
    id: 'default-readiness-report',
    command: npmCommand,
    args: ['run', 'runtime:go:default-readiness-report', '--', '--json'],
    expectsJSON: true
  }
]

const requiredAuditMarkers = [
  '## 11. Reasonix speed/cache parity',
  '## 12. 引入 Reasonix CONTRIBUTING cache-first gate',
  '反馈速率硬合同',
  'DeepSeek cache 硬合同',
  'Cache-impact:',
  'Provider-compat-impact:',
  'Speed-guard:'
]

const absorptionAreas = [
  'typed streaming',
  'provider stream and tool-call parser',
  'DeepSeek cache-first request shaping',
  'history repair and tool-result pairing',
  'usage and cache diagnostics',
  'MCP lazy catalog and schema cache',
  'subagent task and background job runtime',
  'cache-first engineering gate'
]

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-engine-absorption-report] ${item.id}`)
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
    maxBuffer: 96 * 1024 * 1024
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
      // Command output may include human logs before the final JSON report.
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

function readAuditContract() {
  const fullPath = join(process.cwd(), auditPath)
  if (!existsSync(fullPath)) {
    return {
      status: 'failed',
      path: auditPath,
      missingMarkers: requiredAuditMarkers,
      section11Adopted: false,
      section12Adopted: false
    }
  }
  const text = readFileSync(fullPath, 'utf8')
  const missingMarkers = requiredAuditMarkers.filter((marker) => !text.includes(marker))
  return {
    status: missingMarkers.length === 0 ? 'passed' : 'failed',
    path: auditPath,
    missingMarkers,
    section11Adopted: text.includes('## 11. Reasonix speed/cache parity'),
    section12Adopted: text.includes('## 12. 引入 Reasonix CONTRIBUTING cache-first gate')
  }
}

const checks = []
const reports = {}
let failed = false

const auditContract = readAuditContract()
checks.push({
  id: 'kun-reasonix-feature-delta-audit',
  status: auditContract.status,
  path: auditContract.path,
  missingMarkers: auditContract.missingMarkers
})
if (auditContract.status !== 'passed') failed = true

for (const item of failed ? [] : commands) {
  const { check, report } = runCheck(item)
  checks.push(check)
  if (report) reports[item.id] = report
  if (check.status !== 'passed') {
    failed = true
    break
  }
}

const candidateReport = reports['default-readiness-report'] || {}
const productRegression = candidateReport.productRegression && typeof candidateReport.productRegression === 'object'
  ? candidateReport.productRegression
  : {}
const speedCacheGate = candidateReport.speedCacheGate && typeof candidateReport.speedCacheGate === 'object'
  ? candidateReport.speedCacheGate
  : {}
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : 'failed'

const report = {
  schemaVersion: 1,
  id: 'runtime-go-engine-absorption-report',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  deterministicEvidenceOnly: true,
  longRunningBenchmarkRequired: false,
  auditContract,
  absorption: {
    legacyMatrixId: ['d', '0250c-reasonix-superiority-matrix'].join(''),
    legacyCodeStageReportId: ['d', '0250c-go-runtime-code-stage-report'].join(''),
    kunAnalytixProductContractPreserved: productRegression.passed === true,
    reasonixEngineAbsorptionGatePassed: speedCacheGate.passed === true,
    section11SpeedCachePlanAdopted: auditContract.section11Adopted,
    section12CacheFirstGateAdopted: auditContract.section12Adopted,
    areas: absorptionAreas,
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
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  console.log(`audit sections: ${auditContract.status}`)
  console.log(`product contract: ${report.absorption.kunAnalytixProductContractPreserved ? 'passed' : 'failed'}`)
  console.log(`speed/cache gate: ${report.absorption.reasonixEngineAbsorptionGatePassed ? 'passed' : 'failed'}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason || item.missingMarkers?.join(', ') || 'failed'}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
