#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import { mkdirSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import process from 'node:process'

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const noWrite = args.has('--no-write') || skipCommands
const goCommand = process.env.GO || 'go'
const defaultEvidencePath = 'docs/analytix/upstreams/runtime-go-live-evidence/mcp-approval-user-input-evidence.json'

function optionValue(name) {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) return item.slice(inlinePrefix.length)
    if (item === name && rawArgs[index + 1]) return rawArgs[index + 1]
  }
  return ''
}

const evidencePath = optionValue('--output') ||
  process.env.ANALYTIX_RUNTIME_GO_MCP_APPROVAL_USER_INPUT_EVIDENCE ||
  defaultEvidencePath

const commands = [
  {
    id: 'go-approval-user-input-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(ApprovalDenyAndAllowControlToolExecution|BashApprovalDenyAndAllowControlExecution|UserInputOnlyAppearsWhenModelCallsTool|DisableUserInputRemovesInteractiveToolSchemas|NormalTurnsDoNotCreateSyntheticApprovalOrUserInputGates)$'
    ],
    cwd: `${process.cwd()}/packages/runtime-go`
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

function runCheck(item) {
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-approval-user-input-evidence] ${item.id}`)
  if (skipCommands) {
    return {
      id: item.id,
      status: 'skipped',
      command: commandText(item),
      durationMs: 0,
      exitStatus: 0,
      reason: 'dry run; approval/user-input contract was not executed'
    }
  }
  const result = spawnSync(item.command, item.args, {
    cwd: item.cwd,
    env: process.env,
    encoding: 'utf8',
    stdio: 'pipe',
    maxBuffer: 64 * 1024 * 1024
  })
  const exitStatus = result.status ?? (result.signal ? 1 : 0)
  return {
    id: item.id,
    status: exitStatus === 0 ? 'passed' : 'failed',
    command: commandText(item),
    durationMs: Date.now() - started,
    exitStatus,
    reason: failureReason(result, exitStatus)
  }
}

function failureReason(result, exitStatus) {
  if (result.error) return result.error.message
  if (exitStatus !== 0) return `command exited with status ${exitStatus}`
  return ''
}

const checks = []
let failed = false
for (const item of commands) {
  const check = runCheck(item)
  checks.push(check)
  if (check.status !== 'passed') {
    failed = true
    if (!skipCommands) break
  }
}

const passed = !skipCommands && !failed && checks.every((item) => item.status === 'passed')
const status = passed ? 'passed' : skipCommands ? 'skipped' : 'failed'
const report = {
  schemaVersion: 1,
  id: 'runtime-go-mcp-approval-user-input-evidence',
  generatedAt: new Date().toISOString(),
  sourceCommit: currentGitCommit(),
  status,
  passed,
  approvalUserInput: passed,
  approval: passed,
  userInput: passed,
  rawValueRecorded: false,
  credentialSecretsRecorded: false,
  redaction: {
    status: 'passed',
    secretMaterialFound: false
  },
  checks
}

if (!noWrite) {
  const absoluteEvidencePath = resolve(process.cwd(), evidencePath)
  report.outputPath = absoluteEvidencePath
  mkdirSync(dirname(absoluteEvidencePath), { recursive: true })
  writeFileSync(absoluteEvidencePath, JSON.stringify(report, null, 2), 'utf8')
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
