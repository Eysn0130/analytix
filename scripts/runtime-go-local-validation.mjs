#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const actualPackagedSmoke = args.has('--actual') || process.env.ANALYTIX_RUNTIME_GO_ACTUAL_PACKAGED_SMOKE === '1'

function appendOptionPassThrough(target, name) {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) {
      target.push(item)
      continue
    }
    if (item === name && rawArgs[index + 1]) {
      target.push(item, rawArgs[index + 1])
      index += 1
    }
  }
}

const packagedGuiSmokeArgs = ['run', 'runtime:go:packaged-gui-smoke', '--', '--json']
if (actualPackagedSmoke) packagedGuiSmokeArgs.push('--actual')
appendOptionPassThrough(packagedGuiSmokeArgs, '--app-path')
appendOptionPassThrough(packagedGuiSmokeArgs, '--timeout-ms')

const commands = [
  {
    id: 'runtime-health-smoke',
    command: npmCommand,
    args: ['run', 'runtime:go:health-smoke', '--', '--json'],
    expectsJSON: true
  },
  {
    id: 'default-readiness-report',
    command: npmCommand,
    args: ['run', 'runtime:go:default-readiness-report', '--', '--json'],
    expectsJSON: true
  },
  {
    id: 'gui-smoke',
    command: npmCommand,
    args: packagedGuiSmokeArgs,
    expectsJSON: true
  },
  {
    id: 'session-soak',
    command: npmCommand,
    args: ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write'],
    expectsJSON: true
  }
]

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim() : ''
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
      // npm and child scripts may log before the final JSON report.
    }
    start = text.lastIndexOf('{', start - 1)
  }
  return { ok: false, reason: 'command did not emit a trailing JSON object' }
}

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-local-validation] ${item.id}`)
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

const checks = []
const reports = {}
let failed = false

for (const item of commands) {
  const { check, report } = runCheck(item)
  checks.push(check)
  if (report) reports[item.id] = report
  if (check.status !== 'passed') {
    failed = true
    if (!skipCommands) break
  }
}

const candidate = reports['default-readiness-report'] || {}
const guiSmoke = reports['gui-smoke'] || {}
const sessionSoak = reports['session-soak'] || {}
const runtimeHealth = reports['runtime-health-smoke'] || {}
const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : 'failed'
const deterministicContractOnly = actualPackagedSmoke !== true &&
  (guiSmoke.smoke?.deterministicContractOnly === true ||
    sessionSoak.soak?.deterministicContractOnly === true)
const finalAcceptanceReady = false
const notFinalUntil = [
  'runtime:go:preflight -- --gate passes',
  'credentialed DeepSeek and at least one non-DeepSeek provider evidence passes',
  'operator approval evidence passes',
  'relevant evidence scope is clean or committed'
]
const report = {
  schemaVersion: 1,
  id: 'runtime-go-local-validation',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  finalAcceptanceReady,
  finalAcceptanceStatus: 'blocked',
  notFinalUntil,
  local: {
    finalAcceptanceReady,
    finalAcceptanceStatus: 'blocked',
    notFinalUntil,
    productRegressionPassed: candidate.productRegression?.passed === true,
    speedCacheGatePassed: candidate.speedCacheGate?.passed === true,
    runtimeHealthPassed: runtimeHealth.passed === true,
    runtimeHealthActualProcessStarted: runtimeHealth.smoke?.actualRuntimeProcessStarted === true,
    runtimeHealthProductionRuntime: runtimeHealth.smoke?.productionRuntime === true,
    runtimeHealthUsesTemporaryDataDir: runtimeHealth.smoke?.dataDirScope === 'temporary',
    runtimeHealthRuntimeInfoOk: runtimeHealth.smoke?.runtimeInfoOk === true,
    runtimeHealthRuntimeToolsOk: runtimeHealth.smoke?.runtimeToolsOk === true,
    runtimeHealthProductionCapabilitiesOk: runtimeHealth.smoke?.productionCapabilitiesOk === true,
    guiSmokePassed: guiSmoke.passed === true,
    sessionSoakPassed: sessionSoak.passed === true,
    actualPackagedSmokeRequested: actualPackagedSmoke,
    actualPackagedSmokePassed: guiSmoke.smoke?.actualPackagedSmokePassed === true,
    actualPackagedGuiEvidenceRequiredForFinalGate:
      guiSmoke.smoke?.actualPackagedAppEvidenceRequiredForFinalGate === true,
    sessionSoakDeterministicOnly: sessionSoak.soak?.deterministicContractOnly === true,
    liveProviderEvidenceUsed: false,
    deterministicContractOnly,
    actualPackagedAppLaunched: guiSmoke.smoke?.actualPackagedAppLaunched === true,
    actualPackagedAppEvidenceRequiredForFinalGate:
      guiSmoke.smoke?.actualPackagedAppEvidenceRequiredForFinalGate === true
  },
  checks
}

if (jsonOutput) {
  console.log(JSON.stringify(report, null, 2))
} else {
  console.log(`${status.toUpperCase()} ${report.id}`)
  for (const item of checks.filter((check) => check.status !== 'passed')) {
    console.log(`FAILED ${item.id}: ${item.reason}`)
  }
}

if (!reportOnly && !passed) process.exitCode = 1
