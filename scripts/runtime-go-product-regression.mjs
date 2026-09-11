#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'

const repoRoot = process.cwd()
const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const npmCommand = process.platform === 'win32' ? 'npm.cmd' : 'npm'
const goCommand = process.env.GO || 'go'
const pythonCommand = String(process.env.PYTHON || '').trim() || (
  existsSync(join(repoRoot, 'backend', '.venv', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python'))
    ? join(repoRoot, 'backend', '.venv', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python')
    : process.platform === 'win32' ? 'python' : 'python3'
)
const includeMatrixRows = args.has('--matrix-json')
const jsonOutput = args.has('--json') || args.has('--dry-run')
const skipCommands = args.has('--skip-commands') || args.has('--dry-run')
const reportOnly = args.has('--no-gate')
const progressPrefix = '[runtime-go-product-regression:progress] '
const defaultHeartbeatMs = 60_000
const testHeartbeatMs = Number.parseInt(
  String(process.env.ANALYTIX_RUNTIME_VALIDATION_HEARTBEAT_MS || ''),
  10
)
const heartbeatMs = process.env.NODE_ENV === 'test' &&
  Number.isSafeInteger(testHeartbeatMs) &&
  testHeartbeatMs >= 10
  ? testHeartbeatMs
  : defaultHeartbeatMs

let activeChild = null
let interruptedSignal = null

function emitProgress(event) {
  process.stderr.write(`${progressPrefix}${JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-product-regression-progress',
    ...event
  })}\n`)
}

function signalExitCode(signal) {
  if (signal === 'SIGHUP') return 129
  if (signal === 'SIGINT') return 130
  if (signal === 'SIGTERM') return 143
  return 1
}

function signalChildTree(child, signal) {
  if (!child || child.exitCode !== null || child.signalCode !== null) return
  if (process.platform !== 'win32' && child.pid) {
    try {
      process.kill(-child.pid, signal)
      return
    } catch {
      // Fall through to the direct child when process-group delivery races exit.
    }
  }
  try {
    child.kill(signal)
  } catch {
    // The close handler owns final classification when the child already exited.
  }
}

function forwardSignal(signal) {
  interruptedSignal ||= signal
  signalChildTree(activeChild, signal)
}

const signalHandlers = new Map([
  ['SIGHUP', () => forwardSignal('SIGHUP')],
  ['SIGINT', () => forwardSignal('SIGINT')],
  ['SIGTERM', () => forwardSignal('SIGTERM')]
])

for (const [signal, handler] of signalHandlers) {
  process.on(signal, handler)
}

function removeSignalHandlers() {
  for (const [signal, handler] of signalHandlers) {
    process.off(signal, handler)
  }
}

function writeChildChunk(stream, chunk) {
  const destination = jsonOutput || stream === 'stderr' ? process.stderr : process.stdout
  destination.write(chunk)
}

function passedGoTests(stdout) {
  const passedTests = new Set()
  for (const line of String(stdout || '').split(/\r?\n/)) {
    if (!line.trim()) continue
    try {
      const event = JSON.parse(line)
      if (event.Action === 'pass' && typeof event.Test === 'string') {
        passedTests.add(event.Test)
      }
    } catch {
      // Non-JSON subprocess diagnostics cannot prove that a required test ran.
    }
  }
  return passedTests
}

function requiredGoTestsMissing(passedTests, requiredTests) {
  return requiredTests.filter((testName) => !passedTests.has(testName))
}

function isFocusedGoTestCommand(item) {
  if (item.command !== goCommand || item.args?.[0] !== 'test') return false
  return item.args.some((arg) => (
    arg === '-run' ||
    arg.startsWith('-run=') ||
    arg === '-test.run' ||
    arg.startsWith('-test.run=')
  ))
}

function withGoJSONOutput(item) {
  if (item.args.includes('-json')) return item
  return {
    ...item,
    args: [item.args[0], '-json', ...item.args.slice(1)]
  }
}

async function runChildCommand(item, options = {}) {
  const started = Date.now()
  const ordinal = options.ordinal
  const total = options.total
  let lastOutputAt = started
  let capturedStdout = ''
  let spawnError = null

  emitProgress({
    event: 'check_started',
    checkId: item.id,
    ordinal,
    total,
    elapsedMs: 0
  })

  return await new Promise((resolve) => {
    let settled = false
    const child = spawn(item.command, item.args, {
      cwd: item.cwd,
      env: options.env || process.env,
      detached: process.platform !== 'win32',
      stdio: ['ignore', 'pipe', 'pipe']
    })
    activeChild = child

    const heartbeat = setInterval(() => {
      const now = Date.now()
      emitProgress({
        event: 'check_heartbeat',
        checkId: item.id,
        ordinal,
        total,
        childPid: child.pid || null,
        elapsedMs: now - started,
        silentForMs: now - lastOutputAt
      })
    }, heartbeatMs)
    heartbeat.unref()

    const finish = (code, signal) => {
      if (settled) return
      settled = true
      clearInterval(heartbeat)
      if (activeChild === child) activeChild = null
      const durationMs = Date.now() - started
      const status = spawnError || code !== 0 ? 'failed' : 'passed'
      emitProgress({
        event: 'child_finished',
        checkId: item.id,
        ordinal,
        total,
        childPid: child.pid || null,
        elapsedMs: durationMs,
        silentForMs: Date.now() - lastOutputAt,
        status,
        exitStatus: code,
        signal: signal || null
      })
      resolve({
        status: code ?? (signal ? 1 : 1),
        signal,
        error: spawnError,
        stdout: capturedStdout,
        durationMs
      })
    }

    child.stdout.on('data', (chunk) => {
      lastOutputAt = Date.now()
      if (options.captureStdout) {
        capturedStdout += chunk.toString('utf8')
      } else {
        writeChildChunk('stdout', chunk)
      }
    })
    child.stderr.on('data', (chunk) => {
      lastOutputAt = Date.now()
      writeChildChunk('stderr', chunk)
    })
    child.once('error', (error) => {
      spawnError = error
    })
    child.once('close', finish)

    if (interruptedSignal) signalChildTree(child, interruptedSignal)
  })
}

function readRepoText(relativePath) {
  return readFileSync(join(repoRoot, relativePath), 'utf8')
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message)
  }
}

function assertIncludes(text, needle, label) {
  assert(text.includes(needle), `${label} missing: ${needle}`)
}

function gitCheckIgnoreStatus(relativePath) {
  const result = spawnSync('git', ['check-ignore', '-q', '--', relativePath], {
    cwd: repoRoot,
    env: process.env,
    stdio: 'ignore'
  })
  return result.status ?? (result.signal ? 1 : 0)
}

function assertGitIgnored(relativePath) {
  assert(gitCheckIgnoreStatus(relativePath) === 0, `.gitignore must ignore ${relativePath}`)
}

function assertGitNotIgnored(relativePath) {
  assert(gitCheckIgnoreStatus(relativePath) === 1, `.gitignore must not ignore source/test path ${relativePath}`)
}

function listFiles(relativeDir, files = []) {
  const dir = join(repoRoot, relativeDir)
  if (!existsSync(dir)) return files
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const relativePath = `${relativeDir}/${entry.name}`
    if (entry.isDirectory()) {
      listFiles(relativePath, files)
    } else if (entry.isFile()) {
      files.push(relativePath)
    }
  }
  return files
}

const temporaryGoSourceExactPaths = new Set([
  'packages/runtime-go/cmd/contract-sidecar/main.go',
  'packages/runtime-go/cmd/runtime-engine-absorption-report/main.go',
  'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
  'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
  'packages/runtime-go/internal/mcp/manager_test_double.go',
  'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
  'packages/runtime-go/internal/provider/provider_contract_matrix.go',
  'packages/runtime-go/internal/testsupport/providerscript/provider_script.go',
  'packages/runtime-go/internal/readiness/mcp_matrix_contract.go',
  'packages/runtime-go/internal/readiness/provider_matrix_contract.go',
  'packages/runtime-go/internal/conformance/livelocal/harness.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate_handler.go',
  'packages/runtime-go/internal/conformance/livelocal/production_candidate.go'
])

const temporaryGoSourcePathPatterns = [
  /^packages\/runtime-go\/internal\/upstreamaudit\//,
  /^packages\/runtime-go\/internal\/conformance\//,
  /^packages\/runtime-go\/internal\/testsupport\/providerscript\//,
  /^packages\/runtime-go\/internal\/readiness\/[^/]+_conformance\.go$/,
  /^packages\/runtime-go\/internal\/server\/[^/]+_conformance\.go$/
]

function isTemporaryGoSourcePath(relativePath) {
  return temporaryGoSourceExactPaths.has(relativePath) ||
    temporaryGoSourcePathPatterns.some((pattern) => pattern.test(relativePath))
}

function excludesAnalytixProd(sourceText) {
  return sourceText
    .split(/\r?\n/)
    .slice(0, 12)
    .some((line) => {
      const match = line.trim().match(/^\/\/go:build\s+(.+)$/)
      if (!match || match[1].includes('||')) return false
      return /(?:^|[\s(&])!analytix_prod(?:$|[\s)&])/.test(match[1])
    })
}

function isTemporaryGoSourceViolation(relativePath, sourceText) {
  return isTemporaryGoSourcePath(relativePath) && !excludesAnalytixProd(sourceText)
}

function temporaryGoSourceClassificationSelfTest() {
  const productionProofPath = 'packages/runtime-go/internal/domain/toolresult/case_source_binding_proof.go'
  const temporaryPath = 'packages/runtime-go/internal/conformance/temporary_validation.go'
  const exactValidationPath = 'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go'
  const cases = {
    productionProofAllowed: !isTemporaryGoSourcePath(productionProofPath) &&
      !isTemporaryGoSourceViolation(productionProofPath, 'package toolresult\n'),
    temporaryWithoutExclusionRejected: isTemporaryGoSourceViolation(temporaryPath, 'package conformance\n'),
    temporaryWithExclusionAllowed: !isTemporaryGoSourceViolation(
      temporaryPath,
      '//go:build darwin && !analytix_prod\n\npackage conformance\n'
    ),
    temporaryConditionalExclusionRejected: isTemporaryGoSourceViolation(
      temporaryPath,
      '//go:build !analytix_prod || darwin\n\npackage conformance\n'
    ),
    exactValidationWithoutExclusionRejected: isTemporaryGoSourceViolation(
      exactValidationPath,
      'package mcp\n'
    )
  }
  assert(Object.values(cases).every(Boolean), 'temporary Go source classification self-test failed')
  return {
    id: 'runtime-go-temporary-source-classification-self-test',
    status: 'passed',
    passed: true,
    cases
  }
}

if (args.has('--self-test-temporary-source-classification')) {
  console.log(JSON.stringify(temporaryGoSourceClassificationSelfTest()))
  removeSignalHandlers()
  process.exit(0)
}

function runTemporaryEvidenceExposureGuard() {
  const packageJson = JSON.parse(readRepoText('package.json'))
  const scripts = packageJson.scripts && typeof packageJson.scripts === 'object'
    ? packageJson.scripts
    : {}
  const publicRuntimeScript = /^(qa:runtime:packaged|runtime:go:)/
  const numberedRuntimeMarker = ['d0(?:24|25)', '[a-z0-9_-]*'].join('')
  const legacyRuntimeMarker = new RegExp(`\\b(?:${[
    numberedRuntimeMarker,
    ['pr', 'oof'].join(''),
    ['con', 'formance'].join(''),
    ['fix', 'ture'].join(''),
    ['fa', 'ke'].join(''),
    ['mcp', 'Local', 'Pr', 'oof'].join(''),
    ['contract', 'Pr', 'oof'].join('')
  ].join('|')})\\b`, 'i')
  const scriptViolations = []
  for (const [scriptName, command] of Object.entries(scripts)) {
    if (!publicRuntimeScript.test(scriptName)) continue
    if (legacyRuntimeMarker.test(String(command))) {
      scriptViolations.push(`${scriptName}: ${command}`)
    }
  }
  assert(
    scriptViolations.length === 0,
    `public runtime package scripts expose temporary evidence commands:\n${scriptViolations.join('\n')}`
  )
  assertIncludes(
    String(scripts['runtime:go:speed-cache-gate'] || ''),
    'speed-cache-gate',
    'package runtime speed/cache gate script'
  )
  assertIncludes(
    String(scripts['runtime:go:product-regression'] || ''),
    'product-regression',
    'package runtime product regression script'
  )

  const validationWrapper = readRepoText('scripts/runtime-go-validation-command.mjs')
  assertIncludes(validationWrapper, "'speed-cache-gate'", 'runtime validation wrapper')
  assertIncludes(validationWrapper, "'product-regression'", 'runtime validation wrapper')
  assertIncludes(validationWrapper, 'runtime-go-speed-cache-gate.mjs', 'runtime validation wrapper')
  assertIncludes(validationWrapper, 'runtime-go-product-regression.mjs', 'runtime validation wrapper')
  const validationTargetMatches = [...validationWrapper.matchAll(/script:\s*'([^']+)'/g)]
  assert(validationTargetMatches.length > 0, 'runtime validation wrapper must declare script targets')
  const validationTargets = validationTargetMatches.map((match) => match[1])
  const missingValidationTargets = validationTargetMatches
    .map((match) => `scripts/${match[1]}`)
    .filter((relativePath) => !existsSync(join(repoRoot, relativePath)))
  assert(
    missingValidationTargets.length === 0,
    `runtime validation wrapper points at missing scripts:\n${missingValidationTargets.join('\n')}`
  )
  assertGitIgnored('runtime/__analytix_runtime_data_probe__')
  assertGitIgnored('packages/runtime/dist/__analytix_build_probe__.js')
  assertGitIgnored('packages/runtime/node_modules/__analytix_dependency_probe__')
  assertGitNotIgnored('src/main/runtime/__analytix_runtime_source_probe__.ts')
  assertGitNotIgnored('packages/runtime/src/__analytix_runtime_source_probe__.ts')
  assertGitNotIgnored('packages/runtime/tests/__analytix_runtime_test_probe__.ts')
  assertGitNotIgnored('scripts/runtime-go-live-validation.mjs')
  assertGitNotIgnored('src/main/runtime-go-live-validation.test.ts')
  assertGitNotIgnored('scripts/runtime-go-worktree-audit.mjs')
  assertGitNotIgnored('src/main/runtime-go-worktree-audit.test.ts')
  assertGitNotIgnored('scripts/runtime-closure-matrix-audit.mjs')

  const runtimeServerMain = readRepoText('packages/runtime-go/cmd/runtime-server/main.go')
  assertIncludes(runtimeServerMain, 'PrepareRuntimeServerStartupWithPersistenceLeaseContextE', 'runtime-server production startup boundary')
  assert(
    !runtimeServerMain.includes('NewRuntimeServerContractHandler'),
    'runtime-server production composition root must use the product runtime handler, not the contract handler alias'
  )
  const runtimeServerComponents = readRepoText('packages/runtime-go/internal/server/runtime_components.go')
  assertIncludes(runtimeServerComponents, 'func NewRuntimeServerHandlerFromComponents(', 'runtime-server internal composition root')
  const runtimeServerProdConfig = readRepoText('packages/runtime-go/internal/server/runtime_server_config_prod.go')
  assertIncludes(runtimeServerProdConfig, 'type RuntimeServerConfig struct', 'runtime-server internal production config')
  assert(
    !runtimeServerComponents.includes('type RuntimeServerContractConfig struct') &&
      !runtimeServerProdConfig.includes('type RuntimeServerContractConfig struct') &&
      !runtimeServerComponents.includes('runtimeServerContractHandler'),
    'runtime-server internal production type names must not use retired contract handler naming'
  )

  const afterPack = readRepoText('scripts/after-pack.cjs')
  for (const needle of [
    "'-tags', 'analytix_prod'",
    'ANALYTIX_RUNTIME_GO_FORBIDDEN_PATHS',
    'ANALYTIX_RUNTIME_GO_FORBIDDEN_PATTERNS',
    "'runtime-go/bin/runtime-server'",
    'packages/runtime-go/cmd/contract-sidecar/main.go',
    'packages/runtime-go/internal/upstreamaudit/baseline_absorption.go',
    'packages/runtime-go/internal/mcp/mcp_contract_replay_handler.go',
    'packages/runtime-go/internal/mcp/manager_test_double.go',
    'packages/runtime-go/internal/provider/provider_contract_replay_handler.go',
    'packages/runtime-go/internal/provider/provider_contract_matrix.go',
    'packages/runtime-go/internal/testsupport/providerscript/provider_script.go',
    'packages/runtime-go/internal/readiness/mcp_matrix_contract.go',
    'packages/runtime-go/internal/readiness/provider_matrix_contract.go',
    'packages/runtime-go/internal/conformance/livelocal/harness.go',
    '^packages\\/runtime-go\\/internal\\/conformance\\/',
    '^packages\\/runtime-go\\/internal\\/upstreamaudit\\/',
    '^packages\\/runtime-go\\/internal\\/testsupport\\/providerscript\\/',
    '_conformance\\.go'
  ]) {
    assertIncludes(afterPack, needle, 'after-pack production boundary guard')
  }
  const forbiddenRouteOrMarker = new RegExp([
    ['mcp', 'Local', 'Pr', 'oof'].join(''),
    ['contract', 'Pr', 'oof'].join(''),
    ['\\/v1\\/', 'mcp-indexer'].join(''),
    ['\\/v1\\/', 'subagents'].join('')
  ].join('|'), 'i')
  assert(
    !forbiddenRouteOrMarker.test(afterPack),
    'after-pack production boundary must not expose internal contract routes or Reasonix public routes'
  )

  const temporaryGoSources = listFiles('packages/runtime-go')
    .filter((relativePath) => relativePath.endsWith('.go'))
    .filter((relativePath) => isTemporaryGoSourcePath(relativePath))
    .filter((relativePath) => !relativePath.endsWith('_test.go'))
  const temporaryGoViolations = temporaryGoSources
    .filter((relativePath) => isTemporaryGoSourceViolation(relativePath, readRepoText(relativePath)))
  assert(
    temporaryGoViolations.length === 0,
    `temporary Go validation sources must be excluded from analytix_prod builds:\n${temporaryGoViolations.join('\n')}`
  )

  const productionGoSources = listFiles('packages/runtime-go')
    .filter((relativePath) => relativePath.endsWith('.go'))
    .filter((relativePath) => !relativePath.endsWith('_test.go'))
    .filter((relativePath) => !excludesAnalytixProd(readRepoText(relativePath)))
  const retiredGoalEvidenceMarker = ['D', '0244', 'GoalEvidence'].join('')
  const retiredSampleID = ['d', '0241'].join('')
  const retiredProductionMarker = new RegExp([
    ['Run[A-Za-z0-9]*', 'Pr', 'oof'].join(''),
    retiredGoalEvidenceMarker,
    `appr_${retiredSampleID}`,
    `input_[a-z_]*${retiredSampleID}`,
    `thr_${retiredSampleID}`,
    `goal_${retiredSampleID}`,
    `turn_${retiredSampleID}`,
    '\\bShip\\b',
    'Choose direction'
  ].join('|'))
  const retiredProductionMarkerViolations = []
  for (const relativePath of productionGoSources) {
    const lines = readRepoText(relativePath).split(/\r?\n/)
    for (let index = 0; index < lines.length; index += 1) {
      if (retiredProductionMarker.test(lines[index])) {
        retiredProductionMarkerViolations.push(`${relativePath}:${index + 1}: ${lines[index].trim()}`)
      }
    }
  }
  assert(
    retiredProductionMarkerViolations.length === 0,
    `production Go sources expose retired validation/sample markers:\n${retiredProductionMarkerViolations.join('\n')}`
  )

  const legacyUpstreamFieldKey = new RegExp([
    'reasonixCapabilityRedMatrix',
    ['reasonixAbsorptionMatrix', 'D', '0245'].join(''),
    ['reasonixIntegrationTopology', 'D', '0249'].join(''),
    ['reasonixSuperiorityMatrix', 'D', '0250C'].join(''),
    ['defaultBackendReadiness', 'D', '0243'].join(''),
    ['readinessSemantics', 'D', '0247'].join('')
  ].join('|'))
  const legacyUpstreamFieldKeyScanPaths = [
    'packages/runtime/src/contracts/capabilities.ts',
    'packages/runtime-go/internal/app/runtimeinfo/contract_capabilities.go',
    'scripts/scan-product-sovereignty.cjs'
  ]
  const legacyUpstreamFieldKeyViolations = []
  for (const relativePath of legacyUpstreamFieldKeyScanPaths) {
    const lines = readRepoText(relativePath).split(/\r?\n/)
    for (let index = 0; index < lines.length; index += 1) {
      if (legacyUpstreamFieldKey.test(lines[index])) {
        legacyUpstreamFieldKeyViolations.push(`${relativePath}:${index + 1}: ${lines[index].trim()}`)
      }
    }
  }
  assert(
    legacyUpstreamFieldKeyViolations.length === 0,
    `formal upstream absorption metadata exposes legacy numbered field keys:\n${legacyUpstreamFieldKeyViolations.join('\n')}`
  )

  const publicRuntimeScriptNames = Object.keys(scripts)
    .filter((scriptName) => publicRuntimeScript.test(scriptName))
  const transitionalValidationTargetCount = validationTargets
    .filter((target) => /\bd0(?:24|25)[a-z0-9_-]*/i.test(target))
    .length
  const formalValidationTargetCount = validationTargets.length - transitionalValidationTargetCount
  const internalLegacyDelegateCount = validationTargets
    .map((target) => `scripts/${target}`)
    .filter((relativePath) => existsSync(join(repoRoot, relativePath)))
    .filter((relativePath) => /\bd0(?:24|25)[a-z0-9_-]*/i.test(readRepoText(relativePath)))
    .length
  const afterPackForbiddenPathCount = (afterPack.match(/packages\/runtime-go\//g) || []).length
  const afterPackForbiddenPatternCount = (afterPack.match(/\^packages\\\/runtime-go\\\//g) || []).length
  return {
    publicRuntimeScriptCount: publicRuntimeScriptNames.length,
    publicLegacyExposureCount: scriptViolations.length,
    validationWrapperTargetCount: validationTargets.length,
    formalValidationTargetCount,
    transitionalValidationTargetCount,
    internalLegacyDelegateCount,
    afterPackForbiddenPathCount,
    afterPackForbiddenPatternCount,
    temporaryGoSourceCount: temporaryGoSources.length,
    temporaryGoProdExcludedCount: temporaryGoSources.length - temporaryGoViolations.length,
    temporaryGoProdViolationCount: temporaryGoViolations.length,
    productionGoSourceCount: productionGoSources.length,
    retiredProductionMarkerViolationCount: retiredProductionMarkerViolations.length,
    legacyUpstreamFieldKeyViolationCount: legacyUpstreamFieldKeyViolations.length
  }
}

const cacheMaintenanceCommand = {
  id: 'go-build-cache-maintenance-preflight',
  command: goCommand,
  // A real build action opens and closes Go's disk cache. When Go's daily
  // trim is due, doing it here keeps the maintenance phase independently
  // observable through the existing heartbeat instead of attaching minutes
  // of silent stat/unlink work to a completed long-running test matrix.
  // This package build emits no repository artifact and does not replace any
  // subsequent fresh, uncached test command.
  args: ['build', './internal/contracts'],
  cwd: `${repoRoot}/packages/runtime-go`
}

const commands = [
  {
    id: 'production-product-regression-matrix',
    command: goCommand,
    args: [
      'test',
      '-tags',
      'analytix_prod',
      './internal/server',
      '-run',
      'TestProductRegressionMatrix|TestProductionRuntimeToolsExposeOnlyProductDiagnostics|TestSubagentCapabilitiesReflectRuntimeToolAndStoreState'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'production-runtime-build-tag',
    command: goCommand,
    args: ['test', '-p=1', '-count=1', '-tags', 'analytix_prod', './...'],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'runtime-go-contracts',
    command: goCommand,
    args: ['test', '-p=1', '-count=1', './...'],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'cache-first-review-gate',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/cache-first-review-gate.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'live-validation-reporting-contract',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/runtime-go-live-validation.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'worktree-audit-contract',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/runtime-go-worktree-audit.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'runtime-event-contract',
    command: npmCommand,
    args: ['--prefix', 'packages/runtime', 'run', 'test', '--', 'tests/contracts.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'go-runtime-conformance',
    command: npmCommand,
    args: ['--prefix', 'packages/runtime', 'run', 'test', '--', 'tests/go-runtime-conformance.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'desktop-git-checkpoint',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/main/services/git-checkpoint-service.test.ts',
      'src/main/ipc/register-app-ipc-handlers.test.ts',
      'src/preload/preload-runtime-request.test.ts',
      'src/renderer/src/store/chat-store-thread-actions.test.ts',
      'src/renderer/src/store/chat-store-maintenance-actions.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'typescript-checkpoint-rewind-apply-contract',
    command: npmCommand,
    args: [
      '--prefix',
      'packages/runtime',
      'run',
      'test',
      '--',
      'tests/checkpoint-rewind-apply.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'renderer-checkpoint-rewind-apply-control',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/renderer/src/components/RewindPlanApplyControls.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'go-checkpoint-private-snapshot-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerCheckpoint(PlanCapturesWriteFileSnapshotAndAppliesFromPrivateStore|SnapshotFirstTouchWinsAcrossMultipleWrites)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-diff-engine-contract',
    command: goCommand,
    args: ['test', './internal/diff', '-count=1'],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-file-diff-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(WriteFileToolReturnsUnifiedDiffMetadata|EditAfterReadUsesApprovalAndExecutes)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-multi-edit-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(MultiEditToolIsAdvertisedAndExecutes|EditFileSupportsReasonixStyleMultiEditAliases|EditFileMultiEditIsAtomicOnFailure)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-move-file-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerMoveFile(ToolIsAdvertisedAndExecutes|RejectsDestinationExists)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-notebook-edit-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerNotebookEdit(ToolIsAdvertisedAndReplacesCell|InsertsAndDeletesCells|RejectsInvalidTarget|ToolIsAdvertisedAndExecutes|InsertDeleteAndErrors)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-delete-range-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerDeleteRange(ToolIsAdvertisedAndExecutes|RequiresReadBeforeDelete|RejectsDuplicateAnchor)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-delete-symbol-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerDeleteSymbol(ToolIsAdvertisedAndExecutes|RequiresReadBeforeDelete|RejectsMultiNameSpec)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-glob-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerGlob(ToolIsAdvertisedAndExecutes|RejectsWorkspaceEscape)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-grep-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerGrepSkipsNoiseHiddenAndProtectedDirs$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-grep-pruning-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerGrep(SkipsNoiseHiddenAndProtectedDirs|SupportsGlobContextAndColumn|HonorsRootGitignore|HonorsGitignore)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-utf16-file-encoding-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(UTF16ReadGrepAndEditPreserveEncoding|WritePreservesExistingUTF16Encoding)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-code-index-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerCodeIndex(ToolIsAdvertisedAndSearchesGoSymbols|OutlineSkipsNoiseAndFiltersBeforeLimit|RequiresQueryForSearch)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-web-fetch-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerWebFetch(DisabledByDefault|ToolIsAdvertisedAndExecutes|RejectsLinkLocal|UsesConfiguredProxy)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-repeat-guard-contract',
    command: goCommand,
    args: [
      'test',
      '-json',
      '.',
      '-run',
      '^(?:TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite|TestRuntimeServerAllowsRepeatedReadOnlyObservation)$',
      '-count=1'
    ],
    requiredPassedTests: [
      'TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite',
      'TestRuntimeServerAllowsRepeatedReadOnlyObservation'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-failure-storm-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerFailureStormGuard(AnnotatesThirdSameToolFailure|ResetsAfterSuccessfulTool)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-plan-mimo-provider-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerPlanMode(UsesSelectedXiaomiProviderAndModel|CanonicalizesSelectedXiaomiAliasModel)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-interrupted-stream-recovery-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(PostOutputProviderInterruptionRecoversWithTailPrompt|PartialToolCallInterruptionRecoversWithoutExecutingPartialTool)$',
      '-count=1'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'formal-runtime-report-contracts',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/main/runtime-go-packaged-contract-report.test.ts',
      'src/main/runtime-go-engine-absorption-report.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-speed-cache-gate',
    command: npmCommand,
    args: ['run', 'runtime:go:speed-cache-gate', '--', '--json'],
    cwd: repoRoot
  },
  {
    id: 'p0-runtime-health-smoke',
    command: npmCommand,
    args: ['run', 'runtime:go:health-smoke', '--', '--json'],
    cwd: repoRoot
  },
  {
    id: 'p0-packaged-gui-smoke-contract',
    command: npmCommand,
    args: ['run', 'runtime:go:packaged-gui-smoke', '--', '--json'],
    cwd: repoRoot
  },
  {
    id: 'p0-packaged-session-soak-contract',
    command: npmCommand,
    args: ['run', 'runtime:go:packaged-soak', '--', '--json', '--no-write'],
    cwd: repoRoot
  },
  {
    id: 'p0-packaging-config-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/main/packaging-config.test.ts',
      'src/main/data-analysis/native-runtime-paths.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-python-native-path-retirement-contract',
    command: pythonCommand,
    args: ['-m', 'pytest', 'tests/test_native_component_paths.py', '-q'],
    cwd: `${repoRoot}/backend`
  },
  {
    id: 'p0-provider-settings-probe-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/shared/app-settings-provider.test.ts',
      'src/shared/openai-compat-url.test.ts',
      'src/main/provider-connection.test.ts',
      'src/main/upstream-models.test.ts',
      'src/main/services/write-inline-completion-service.test.ts',
      'src/main/claw-scheduled-task-detector.test.ts',
      'src/renderer/src/components/settings-section-agents.test.ts',
      'src/renderer/src/agent/analytix-runtime.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-mcp-runtime-contract',
    command: goCommand,
    args: [
      'test',
      './internal/mcp',
      '.',
      '-run',
      'TestProductionManager(EmptyConfigDoesNotLoadFixtureSpec|HTTPListCallReconnectAndRedaction|HTTPTransportAcceptsSSEJSONRPCResponses|HTTPPromptsAndResourcesCatalog|HTTPTransportUsesConfiguredProxy|UsesCachedSchemaForLazyCatalogAndFirstUseReconnect|CachedSchemaInvalidatesOnSpecFingerprintChange|ReportsStaleCachedSchemaWhenFirstUseToolDisappears|CatalogFingerprintCanonicalizesSchemas|ToolInputSchemaReturnsCanonicalCopy|StdioListAndCall)$|TestRuntimeServer(ConfiguredHTTPMCPToolLoopExecutesAndContinues|ConfiguredStdioMCPToolLoopExecutesAndContinues|RunsReadOnlyMCPToolsInParallel|MCPRefreshLoadsLazyBackgroundCatalog|MCPRefreshRecordsToolCatalogChangedEvent|ToolsExposeRealMCPPromptsAndResources|UsesConfiguredMCPProxyForHTTPServers)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-mcp-ui-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/renderer/src/components/plugin-marketplace-runtime.test.ts',
      'src/renderer/src/components/PluginMarketplaceView.test.ts',
      'src/renderer/src/components/settings-section-agents.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'go-runtime-subagent-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerRunSkillCanExecuteSubagentSkill|TestRuntimeServerExecutesDelegateTaskWithDurableChildRun|TestRuntimeServerDelegateTaskAppliesSubagentProfileModelEffortAndToolScope|TestRuntimeServerSubagentUsageSourceSurvivesRestart|TestRuntimeServerDelegateTaskAppliesConfiguredDefaultSubagentProfile|TestRuntimeServerSubagentConfigEnforcesMaxParallel|TestRuntimeServerSubagentInheritProfileDoesNotSilentlyRunBash|TestRuntimeServerSubagentFiltersRecursiveAndJobToolsEvenIfCalled|TestRuntimeServerApprovalDenyBlocksDelegateTaskBeforeChildRun|TestRuntimeServerParallelTasksCreateDurableChildRuns|TestRuntimeServerParallelTasksHonorDependsOnWaves|TestRuntimeServerParallelTasksRunReadyWaveConcurrently|TestRuntimeServerSubagentContinueAndForkUseDurableTranscript|TestRuntimeServerSubagentContinueInheritsSourceIdentityWhenParentModelChanges|TestRuntimeServerRejectsConcurrentSubagentContinueFromSameReference|TestRuntimeServerAllowsSubagentForkFromAncestorParentThread|TestRuntimeServerCopiesAncestorSubagentContinueIntoCurrentParent|TestRuntimeServerRejectsSubagentContinueWhenToolScopeDrifts|TestRuntimeServerStartupInterruptsStaleRunningSubagentRuns|TestRuntimeServerRejectsCrossParentSubagentContinue|TestRuntimeServerInternalSubagentLineageUsesExistingGoalContract'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'go-runtime-background-job-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServerBackgroundTaskJobsWaitAndOutput|TestRuntimeServerBashRunInBackgroundExposesOutputAndKill|TestRuntimeServerModelCanInspectBackgroundTaskJobs|TestRuntimeServerBackgroundTaskJobKillCancelsChildRun'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-attachments-vision-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(AttachmentPayloadReachesProviderAsImageOrTextFallback|PersistentAttachmentsSurviveRestart|TurnRejectsCrossThreadAttachmentReference)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-legacy-ts-attachments-vision-contract',
    command: npmCommand,
    args: [
      'run',
      '--prefix',
      'packages/runtime',
      'test',
      '--',
      'tests/attachment-store.test.ts',
      'tests/model-client.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-approval-user-input-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(ApprovalDenyAndAllowControlToolExecution|ApprovalResolutionContinuesAfterRestart|BashApprovalDenyAndAllowControlExecution|UserInputOnlyAppearsWhenModelCallsTool|UserInputResolutionContinuesAfterRestart|DisableUserInputRemovesInteractiveToolSchemas|NormalTurnsDoNotCreateSyntheticApprovalOrUserInputGates)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-usage-cost-cache-contract',
    command: goCommand,
    args: [
      'test',
      '.',
      '-run',
      'TestRuntimeServer(UsageEndpointCoversRuntimeThreadDayModelAndThreadDetail|ConfiguredProviderPricingProducesNonZeroCost|CacheDiagnosticsIsolateModelNamespaces|SubagentUsageSourceSurvivesRestart)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-usage-aggregation-contract',
    command: goCommand,
    args: [
      'test',
      './internal/server',
      '-run',
      'TestUsage(AggregationCoversDeltaCumulativeOldCacheAndMixedProviderModel|RuntimeResponseAttributesSubagentSource|EventsIndexBackfillAvoidsPerRequestThreadEventReplay)$'
    ],
    cwd: `${repoRoot}/packages/runtime-go`
  },
  {
    id: 'p0-renderer-usage-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/renderer/src/hooks/use-thread-usage.test.ts',
      'src/renderer/src/hooks/use-daily-usage.test.ts',
      'src/renderer/src/hooks/use-model-usage.test.ts',
      'src/renderer/src/components/chat/FloatingComposer.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'typescript-retired-backend',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/runtime/analytix-adapter.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'legacy-child-retired',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/analytix-process.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'analytix-serve-go-launcher',
    command: npmCommand,
    args: ['--prefix', 'packages/runtime', 'run', 'test', '--', 'tests/serve-entry-go-launcher.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'packaged-go-boundary',
    command: npmCommand,
    args: ['run', 'test', '--', 'src/main/packaging-config.test.ts', '--run'],
    cwd: repoRoot
  },
  {
    id: 'p0-preload-bridge-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/preload/preload-sandbox.test.ts',
      'src/preload/preload-runtime-request.test.ts',
      'src/preload/preload-sse-bridge.test.ts',
      'src/main/ipc/app-ipc-schemas.test.ts',
      'src/main/ipc/register-app-ipc-handlers.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-renderer-browser-bridge-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/renderer/src/lib/browser-analytix-bridge.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'p0-error-display-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/shared/runtime-error.test.ts',
      'src/shared/secret-redaction.test.ts',
      'src/renderer/src/lib/format-runtime-error.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'renderer-timeline-contract',
    command: npmCommand,
    args: [
      'run',
      'test',
      '--',
      'src/renderer/src/agent/analytix-mapper.test.ts',
      'src/renderer/src/components/chat/MessageTimeline.tool-summary.test.ts',
      'src/renderer/src/store/chat-store-runtime.test.ts',
      '--run'
    ],
    cwd: repoRoot
  },
  {
    id: 'product-sovereignty-scan',
    command: npmCommand,
    args: ['run', 'scan:product-sovereignty'],
    cwd: repoRoot
  }
]

function commandText(item) {
  return [item.command, ...item.args].join(' ')
}

function skippedCheck(item) {
  return {
    id: item.id,
    status: 'skipped',
    command: commandText(item),
    durationMs: 0,
    exitStatus: 0,
    reason: 'dry run; product regression command was not executed'
  }
}

const matrixSnapshotCommand = {
  id: 'product-regression-matrix-snapshot',
  command: goCommand,
  args: ['test', '-tags', 'analytix_prod', './internal/server', '-run', 'TestProductRegressionMatrixJSONSnapshot', '-v'],
  cwd: `${repoRoot}/packages/runtime-go`
}

const results = []
let failed = false
let productRegressionMatrix = undefined
let retirementSummary = undefined
const totalChecks = commands.length + 3

{
  const started = Date.now()
  const id = 'temporary-evidence-exposure-guard'
  if (!jsonOutput) console.log(`[runtime-go-product-regression] ${id}`)
  if (!skipCommands) {
    emitProgress({ event: 'check_started', checkId: id, ordinal: 1, total: totalChecks, elapsedMs: 0 })
  }
  try {
    retirementSummary = runTemporaryEvidenceExposureGuard()
    results.push({
      id,
      status: 'passed',
      durationMs: Date.now() - started
    })
    if (!skipCommands) {
      emitProgress({
        event: 'check_finished',
        checkId: id,
        ordinal: 1,
        total: totalChecks,
        elapsedMs: Date.now() - started,
        status: 'passed'
      })
    }
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    results.push({
      id,
      status: 'failed',
      durationMs: Date.now() - started
    })
    if (!skipCommands) {
      emitProgress({
        event: 'check_finished',
        checkId: id,
        ordinal: 1,
        total: totalChecks,
        elapsedMs: Date.now() - started,
        status: 'failed'
      })
    }
    failed = true
  }
}

{
  const id = cacheMaintenanceCommand.id
  if (!jsonOutput) console.log(`[runtime-go-product-regression] ${id}`)
  if (skipCommands) {
    results.push(skippedCheck(cacheMaintenanceCommand))
  } else {
    const result = await runChildCommand(cacheMaintenanceCommand, {
      ordinal: 2,
      total: totalChecks
    })
    const status = result.status ?? (result.signal ? 1 : 0)
    const itemFailed = status !== 0 || Boolean(result.error)
    results.push({
      id,
      status: itemFailed ? 'failed' : 'passed',
      durationMs: result.durationMs
    })
    if (result.error) console.error(result.error.message)
    if (itemFailed) failed = true
    emitProgress({
      event: 'check_finished',
      checkId: id,
      ordinal: 2,
      total: totalChecks,
      elapsedMs: result.durationMs,
      status: itemFailed ? 'failed' : 'passed'
    })
  }
}

{
  const id = matrixSnapshotCommand.id
  let matrixFailed = false
  if (!jsonOutput) console.log(`[runtime-go-product-regression] ${id}`)
  if (skipCommands) {
    results.push(skippedCheck(matrixSnapshotCommand))
  } else {
    const result = await runChildCommand(matrixSnapshotCommand, {
      ordinal: 3,
      total: totalChecks,
      captureStdout: true,
      env: {
        ...process.env,
        ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON: '1'
      }
    })
    const status = result.status ?? (result.signal ? 1 : 0)
    if (status !== 0 && result.stdout) writeChildChunk('stdout', result.stdout)
    if (result.error) {
      console.error(result.error.message)
      matrixFailed = true
    }
    if (status !== 0) {
      matrixFailed = true
    }
    if (!matrixFailed) {
      const match = String(result.stdout || '').match(/ANALYTIX_PRODUCT_REGRESSION_MATRIX_JSON=(\[.*\])/)
      if (!match) {
        console.error('product regression matrix snapshot was not emitted')
        matrixFailed = true
      } else {
        try {
          productRegressionMatrix = JSON.parse(match[1])
        } catch (error) {
          console.error(error instanceof Error ? error.message : String(error))
          matrixFailed = true
        }
      }
    }
    results.push({
      id,
      status: !matrixFailed && status === 0 ? 'passed' : 'failed',
      durationMs: result.durationMs
    })
    emitProgress({
      event: 'check_finished',
      checkId: id,
      ordinal: 3,
      total: totalChecks,
      elapsedMs: result.durationMs,
      status: !matrixFailed && status === 0 ? 'passed' : 'failed'
    })
    if (matrixFailed) failed = true
  }
}

for (const [index, item] of commands.entries()) {
  if (interruptedSignal) break
  const started = Date.now()
  if (!jsonOutput) console.log(`[runtime-go-product-regression] ${item.id}`)
  if (skipCommands) {
    results.push(skippedCheck(item))
    continue
  }
  const focusedGoTest = isFocusedGoTestCommand(item)
  const commandItem = focusedGoTest ? withGoJSONOutput(item) : item
  const result = await runChildCommand(commandItem, {
    ordinal: index + 4,
    total: totalChecks,
    captureStdout: focusedGoTest || Array.isArray(item.requiredPassedTests)
  })
  const status = result.status ?? (result.signal ? 1 : 0)
  const passedTests = focusedGoTest && status === 0
    ? passedGoTests(result.stdout)
    : new Set()
  const emptyFocusedSelection = focusedGoTest && status === 0 && passedTests.size === 0
  const missingRequiredTests = Array.isArray(item.requiredPassedTests) && status === 0
    ? requiredGoTestsMissing(passedTests, item.requiredPassedTests)
    : []
  const itemFailed = status !== 0 || Boolean(result.error) || emptyFocusedSelection || missingRequiredTests.length > 0
  if (result.stdout && (!jsonOutput || itemFailed)) {
    writeChildChunk('stdout', result.stdout)
  }
  if (missingRequiredTests.length > 0) {
    console.error(`${item.id} did not execute required Go tests: ${missingRequiredTests.join(', ')}`)
  } else if (emptyFocusedSelection) {
    console.error(`${item.id} did not execute any Go tests selected by -run`)
  }
  results.push({
    id: item.id,
    status: itemFailed ? 'failed' : 'passed',
    durationMs: result.durationMs ?? (Date.now() - started)
  })
  if (result.error) {
    console.error(result.error.message)
    failed = true
  }
  if (itemFailed) {
    failed = true
  }
  emitProgress({
    event: 'check_finished',
    checkId: item.id,
    ordinal: index + 4,
    total: totalChecks,
    elapsedMs: result.durationMs ?? (Date.now() - started),
    status: itemFailed ? 'failed' : 'passed'
  })
}

const passed = !skipCommands && !failed
const status = failed ? 'failed' : skipCommands ? 'skipped' : 'passed'

removeSignalHandlers()

if (interruptedSignal) {
  process.exitCode = signalExitCode(interruptedSignal)
} else {
  console.log(JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-product-regression',
    status,
    passed,
    dryRunCannotAuthorizeCutover: skipCommands,
    expectedBlocked: skipCommands,
    matrix: productRegressionMatrix ? {
      rowCount: productRegressionMatrix.length,
      features: productRegressionMatrix.map((row) => row.feature),
      ...(includeMatrixRows ? { rows: productRegressionMatrix } : {})
    } : undefined,
    retirement: retirementSummary,
    checks: results
  }, null, 2))

  if (!reportOnly && !passed) process.exitCode = 1
}
