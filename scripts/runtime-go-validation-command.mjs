#!/usr/bin/env node

import { spawn } from 'node:child_process'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { confirmSupervisedReleaseExecution } from './runtime-go-release-finalization.mjs'

const here = dirname(fileURLToPath(import.meta.url))

const commands = {
  'packaged-qa': { script: 'runtime-go-packaged-qa.mjs' },
  preflight: { script: 'runtime-go-preflight.mjs' },
  'live-evidence': { script: 'runtime-go-live-evidence-collector.mjs' },
  'live-validation': { script: 'runtime-go-live-validation.mjs' },
  'local-validation': { script: 'runtime-go-local-validation.mjs' },
  'runtime-health-smoke': { script: 'runtime-go-runtime-health-smoke.mjs' },
  'performance-check': { script: 'runtime-go-performance-check.mjs' },
  'speed-cache-gate': { script: 'runtime-go-speed-cache-gate.mjs' },
  'product-regression': { script: 'runtime-go-product-regression.mjs' },
  'rc-control-plane': { script: 'runtime-go-release-gate.mjs', args: ['--control-plane-only'] },
  'release-gate': { script: 'runtime-go-release-gate.mjs' },
  'release-execution': { script: 'runtime-go-release-gate.mjs', args: ['--execute'] },
  'release-finalization': { script: 'runtime-go-release-gate.mjs', args: ['--finalize'] },
  'approval-user-input-evidence': { script: 'runtime-go-approval-user-input-evidence.mjs' },
  'packaged-soak': { script: 'runtime-go-packaged-session-soak.mjs' },
  'packaged-gui-smoke': { script: 'runtime-go-packaged-gui-smoke.mjs' },
  'packaged-milestone-a': { script: 'runtime-go-packaged-milestone-a.mjs' },
  'packaged-milestone-b': { script: 'runtime-go-packaged-milestone-b.mjs' },
  'default-readiness-report': { script: 'runtime-go-default-readiness-report.mjs' },
  'rollback-retirement-evidence': { script: 'runtime-go-rollback-retirement-evidence.mjs' },
  'rollback-retirement-report': { script: 'runtime-go-rollback-retirement-report.mjs' },
  'ts-retirement-scan': { script: 'runtime-go-ts-retirement-scan.mjs' },
  'reasonix-live-parity': { script: 'runtime-go-reasonix-live-parity.mjs' },
  'cutover-report': { script: 'runtime-go-cutover-report.mjs' },
  'engine-absorption-report': { script: 'runtime-go-engine-absorption-report.mjs' }
}

const [command, ...forwardedArgs] = process.argv.slice(2)
const target = commands[command]

if (!target) {
  const available = Object.keys(commands).sort().join(', ')
  console.error(`Unknown runtime validation command "${command || ''}". Available: ${available}`)
  process.exit(2)
}

const childArgs = [...(target.args || []), ...forwardedArgs]
const releaseExecution = target.script === 'runtime-go-release-gate.mjs' && childArgs.includes('--execute')
const child = spawn(
  process.execPath,
  [join(here, target.script), ...childArgs],
  {
    cwd: process.cwd(),
    env: process.env,
    detached: process.platform !== 'win32',
    stdio: releaseExecution ? ['inherit', 'inherit', 'inherit', 'ipc'] : 'inherit'
  }
)

let interruptedSignal = null
let executionMessage = null
let executionMessageInvalid = false
if (releaseExecution) child.on('message', (message) => {
  if (executionMessage || message?.type !== 'analytix-release-execution-seal') executionMessageInvalid = true
  else executionMessage = message
})

function signalExitCode(signal) {
  if (signal === 'SIGHUP') return 129
  if (signal === 'SIGINT') return 130
  if (signal === 'SIGTERM') return 143
  return 1
}

function signalChildTree(signal) {
  if (child.exitCode !== null || child.signalCode !== null) return
  if (process.platform !== 'win32' && child.pid) {
    try {
      process.kill(-child.pid, signal)
      return
    } catch {
      // Fall through when process-group delivery races child exit.
    }
  }
  try {
    child.kill(signal)
  } catch {
    // The close event owns final exit classification.
  }
}

function forwardSignal(signal) {
  interruptedSignal ||= signal
  signalChildTree(signal)
}

const signalHandlers = new Map([
  ['SIGHUP', () => forwardSignal('SIGHUP')],
  ['SIGINT', () => forwardSignal('SIGINT')],
  ['SIGTERM', () => forwardSignal('SIGTERM')]
])

for (const [signal, handler] of signalHandlers) {
  process.on(signal, handler)
}

const result = await new Promise((resolve) => {
  let spawnError = null
  child.once('error', (error) => {
    spawnError = error
  })
  child.once('close', (status, signal) => {
    resolve({ status, signal, error: spawnError })
  })
})

for (const [signal, handler] of signalHandlers) {
  process.off(signal, handler)
}

if (result.error) {
  console.error(result.error.message)
  process.exit(1)
}

if (interruptedSignal) {
  process.exit(signalExitCode(interruptedSignal))
}

if (result.signal) {
  process.exit(signalExitCode(result.signal))
}

if (releaseExecution && result.status === 0) {
  try {
    if (executionMessageInvalid || !executionMessage) throw new Error('execution_completion_message_invalid')
    const completion = confirmSupervisedReleaseExecution({ repoRoot: process.cwd(), message: executionMessage,
      observed: { pid: child.pid, exitStatus: result.status, signal: result.signal, spawnError: result.error,
        interruptedSignal, command: process.execPath, args: [join(here, target.script), ...childArgs] } })
    console.log(JSON.stringify({ id: 'runtime-go-execution-completion', passed: false, executionCompleted: true, completion }, null, 2))
  } catch {
    console.error('release execution completion could not be verified')
    process.exit(1)
  }
}

process.exit(result.status ?? 1)
