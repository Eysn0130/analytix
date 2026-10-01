import assert from 'node:assert/strict'
import test from 'node:test'
import { spawnSync } from 'node:child_process'
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import * as vm from 'node:vm'
import { canonicalJSONString, createReceiptRun, gateExitCode, parseOpenSpecTaskLedger, sha256, sourceIdentity, writeAndRevalidateReceipt } from './runtime-go-rc-receipt.mjs'
import { confirmSupervisedReleaseExecution, evaluateReleaseExecution, finalizeReleaseExecution, finalReportSections, freezeReleaseClosure, releaseCommandIds, sealReleaseExecution } from './runtime-go-release-finalization.mjs'

const tasksPath = 'openspec/changes/case-evidence-publication-gate/tasks.md'
const reportPath = 'docs/analytix/qa/final-report.md'
const fixtureChild = process.argv[2]?.startsWith('--fixture-') === true
const scriptPath = fileURLToPath(import.meta.url)
const gatePath = fileURLToPath(new URL('./runtime-go-release-gate.mjs', import.meta.url))
const fixtureEnvironment = { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1', GIT_TERMINAL_PROMPT: '0' }
for (const key of Object.keys(fixtureEnvironment)) {
  if (key.startsWith('GIT_') && !['GIT_CONFIG_GLOBAL', 'GIT_CONFIG_NOSYSTEM', 'GIT_TERMINAL_PROMPT'].includes(key)) delete fixtureEnvironment[key]
}

function git(repoRoot, args) {
  const result = spawnSync('git', ['-c', 'core.hooksPath=/dev/null', '-c', 'commit.gpgsign=false',
    '-c', 'user.name=Synthetic fixture', '-c', 'user.email=fixture@example.invalid', ...args], {
    cwd: repoRoot, env: fixtureEnvironment, encoding: 'utf8'
  })
  assert.equal(result.status, 0, result.stderr)
  return result.stdout.trim()
}

function put(root, name, bytes) {
  const path = join(root, name)
  mkdirSync(dirname(path), { recursive: true })
  writeFileSync(path, bytes)
  return path
}

function fixture(t, { openRealTask = false, existingReport = false } = {}) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-finalization-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const repoRoot = join(root, 'repo')
  mkdirSync(repoRoot)
  git(repoRoot, ['init', '-q'])
  put(repoRoot, 'product.js', 'export const accepted = true\n')
  const tasks = ['- [x] 1.1 **[RC_REQUIRED]** Implement accepted behavior.',
    '- [ ] 10.4 **[RC_REQUIRED]** Execute all applicable suites.',
    '- [ ] 10.8 **[RC_REQUIRED]** Write the final report.',
    '- [ ] 10.9 **[RC_REQUIRED]** Verify every DoD condition.',
    ...(openRealTask ? ['- [ ] 9.5 **[RC_REQUIRED]** Actual packaged milestone.'] : []), ''].join('\n')
  put(repoRoot, tasksPath, tasks)
  if (existingReport) put(repoRoot, reportPath, 'The declared report is pending.\n')
  git(repoRoot, ['add', '.'])
  git(repoRoot, ['commit', '-qm', 'synthetic tested source S'])
  return { root, repoRoot, cacheRoot: join(root, 'cache'), tasks, source: sourceIdentity(repoRoot) }
}

function syntheticEvidence(config, source, toolchain) {
  const runtimeBinary = { sha256: 'b'.repeat(64), buildCount: 1, productionTagUsed: true }
  const performance = { id: 'runtime-go-rc-performance-suite', passed: true,
    source: { ...source, identityValid: true, sourceBoundToHeadTree: true }, host: { supported: true }, toolchain, runtimeBinary,
    structural: { status: 'passed', passed: true }, benchmark: { status: 'passed', passed: true,
      warmupCount: 2, measuredCount: 20, validWarmupCount: 2, validMeasuredCount: 20, statistic: 'nearest-rank-p95', thresholdMs: 250,
      warmups: Array.from({ length: 2 }, () => ({ first_runtime_event_ms: 1 })), samples: Array.from({ length: 20 }, () => ({ first_runtime_event_ms: 1 })) },
    rcPerformanceAuthorization: true, releaseAuthorization: false }
  const inputBinding = { path: 'source.txt', sha256: 'a'.repeat(64), gitBlob: 'a'.repeat(40), inputMode: 'release-git-object', readbackMatched: true }
  const releaseSource = { ...source, verified: true, inputMode: 'release-git-object' }
  const admission = { schemaVersion: 1, id: 'analytix.upstream-artifact-admission/v1', releaseSource,
    inputs: { manifest: { ...inputBinding, declaredAnalytixCommit: source.head }, provenanceLedger: inputBinding,
      sourceEvidence: [{ sourceId: 'fixture-source', licenseClass: 'MIT', manifestEntrySha256: 'a'.repeat(64), provenanceLedger: inputBinding,
        licenseObject: { commit: source.head, path: null, blob: null, sha256: null, readbackMatched: true, classification: null } }] },
    entries: [{ item: 'fixture-item', sourceId: 'fixture-source', disposition: 'shipped', status: 'passed', problems: [] }],
    ok: true, status: 'passed', engineeringAdmission: true, problems: [] }
  admission.projectionSha256 = sha256(canonicalJSONString(admission))
  const upstream = { schemaVersion: 2, id: 'upstream-source-audit', releaseSource, analytixCommit: source.head,
    artifactAdmission: admission, projectionSha256: admission.projectionSha256, researchFreshness: { status: 'passed', releaseBlocking: false } }
  const legal = { id: 'analytix.artifact-legal-obligations/v1', sourcePackagePlan: { passed: true }, artifactLegalPlanPassed: true,
    artifactAdmissionGatePassed: true, exactFormalArtifact: { provided: true, engineeringAdmission: true }, blockers: [] }
  const reports = new Map([['preflight-source-inspection', { id: 'runtime-go-preflight', finalGateBlockers: [] }],
    ['upstream-source-audit', upstream], ['rc-structural-and-performance-suite', performance], ['artifact-legal-obligations-audit', legal]])
  const formalEvidenceInput = { provided: true, accepted: true, problems: [], statuses: { electron: 'passed', package: 'passed', a0: 'passed', b1: 'passed' }, reports: {} }
  const formalDirectory = join(config.root, 'synthetic-formal-input')
  for (const lane of ['a0', 'b1']) {
    const bytes = Buffer.from(JSON.stringify({ sourceCommit: source.head, lane, synthetic: true }))
    const name = `packaged-milestone-${lane[0]}.json`
    put(formalDirectory, name, bytes)
    formalEvidenceInput.reports[lane] = { fileName: name, sha256: sha256(bytes), byteLength: bytes.length }
  }
  return { runtimeBinary, performance, legal, reports, formalEvidenceInput, formalDirectory }
}

// A closed execution stub supplies synthetic command/formal-ingestion results.
// The real receipt, once-lock, aggregation, sealing and Git protocol run intact.
export function executionFixture(config) {
  const { repoRoot, cacheRoot, scenario = 'passed' } = config
  const toolchain = { node: process.version, npm: 'fixture-npm', go: 'fixture-go' }
  const run = createReceiptRun({ repoRoot, cacheRoot, resolveToolchain: () => toolchain })
  const source = run.source
  const closure = freezeReleaseClosure({ repoRoot, source, taskIds: ['10.4', '10.8', '10.9'], reportPath })
  const manifest = releaseCommandIds.map((id) => ({ id, command: `stub ${id}`, cwd: repoRoot,
    heavyweight: ['typecheck', 'build-runtime', 'full-root-test', 'go-ordinary-matrix', 'go-prod-matrix', 'go-race-matrix', 'rc-structural-and-performance-suite'].includes(id),
    logicalChecks: id === 'rc-structural-and-performance-suite' ? ['structural-performance', 'rc-performance-benchmark'] : [id], expectedExecutions: 1 }))
  const checks = manifest.map((entry) => ({ ...entry, durationMs: 1, executionCount: 1, sourceStable: true, status: 'passed', exitStatus: 0 }))
  const { runtimeBinary, performance, legal, reports, formalEvidenceInput, formalDirectory } = syntheticEvidence(config, source, toolchain)
  if (scenario === 'failed') { checks[3].status = 'failed'; checks[3].exitStatus = 1 }
  if (scenario === 'interrupted') { checks[3].status = 'failed'; checks[3].exitStatus = 143 }
  if (scenario === 'skipped') { checks[3].status = 'skipped'; checks[3].executionCount = 0 }
  if (scenario === 'missing') checks.pop()
  if (scenario === 'formal-failed') formalEvidenceInput.statuses.b1 = 'failed'
  if (scenario === 'artifact-failed') legal.exactFormalArtifact.engineeringAdmission = false
  const receiptRefs = []
  for (const [index, check] of checks.entries()) receiptRefs.push({ id: check.id, ...writeAndRevalidateReceipt({ runDir: run.runDir,
    fileName: `${index}-${check.id}.json`, payload: { receiptType: 'command', runId: run.runId, source, toolchain,
      command: { id: check.id, text: check.command, cwd: check.cwd }, runtimeBinary,
      result: { status: check.status, exitStatus: check.exitStatus, durationMs: 1, sourceStable: check.sourceStable, executionCount: check.executionCount },
      report: reports.get(check.id) || null } }) })
  for (const id of ['structural-performance', 'rc-performance-benchmark']) receiptRefs.push({ id, ...writeAndRevalidateReceipt({ runDir: run.runDir,
    fileName: `${id}.json`, payload: { receiptType: 'logical-check', runId: run.runId, source, toolchain, runtimeBinary,
      command: { id, executionOwner: 'rc-structural-and-performance-suite' }, result: id === 'structural-performance' ? performance.structural : performance.benchmark } }) })
  receiptRefs.push({ id: 'formal-evidence-ingestion', ...writeAndRevalidateReceipt({ runDir: run.runDir, fileName: 'formal-evidence-ingestion.json',
    payload: { receiptType: 'formal-evidence-ingestion', runId: run.runId, source, toolchain, result: formalEvidenceInput } }) })
  const tasks = readFileSync(join(repoRoot, tasksPath))
  const rcLedger = parseOpenSpecTaskLedger(tasks.toString(), { tasksFileSha256: sha256(tasks) })
  const evaluated = evaluateReleaseExecution({ initialSource: source, receiptRun: run, checks, receiptRefs, reports, formalEvidenceInput, rcLedger })
  const payload = { ...evaluated, receiptType: 'normalized-execution', commandMode: 'execution', runId: run.runId, source, toolchain,
    formalRun: { path: run.formalRunPath, sha256: run.formalRunSha256 }, passed: false, executionManifest: manifest,
    closure, checks, runtimeBinary, formalEvidenceInput, tasksFileSha256: sha256(tasks), receiptRefs }
  if (scenario === 'forged-success') { payload.executionPassed = true; payload.controlPlaneReady = true; payload.checks[0].sourceStable = false }
  const normalized = writeAndRevalidateReceipt({ runDir: run.runDir, fileName: 'normalized-execution-receipt.json', payload })
  if (scenario === 'task-drift') put(repoRoot, tasksPath, tasks.toString().replace('Execute all', 'Skip all'))
  if (scenario === 'report-drift') put(repoRoot, reportPath, 'Unexpected pre-exit report\n')
  const seal = sealReleaseExecution({ repoRoot, receiptRun: run, normalized, manifest, closure, formalEvidenceDirectory: formalDirectory })
  return { run, seal, normalized, receiptRefs, executionPassed: payload.executionPassed, passed: payload.passed, openRcRequiredIds: rcLedger.openRcRequiredIds }
}

function execute(config, scenario = 'passed') {
  const argv = [scriptPath, '--fixture-execution', JSON.stringify({ ...config, scenario })]
  const result = spawnSync(process.execPath, argv, {
    env: fixtureEnvironment, encoding: 'utf8', maxBuffer: 2 << 20
  })
  const output = JSON.parse(result.stdout)
  if (result.status === 0) confirmSupervisedReleaseExecution({ repoRoot: config.repoRoot,
    message: { type: 'analytix-release-execution-seal', seal: output.seal, runId: output.run.runId },
    observed: { pid: result.pid, exitStatus: result.status, signal: result.signal, spawnError: result.error,
      interruptedSignal: null, command: process.execPath, args: argv } })
  return { status: result.status, output, stderr: result.stderr }
}

function reportText(execution) {
  return `Execution run: ${execution.run.runId}\nExecution seal SHA256: ${execution.seal.sha256}\n\n` +
    finalReportSections.map((section) => `## ${section}\n\n${section === 'Final aggregation' ? 'pending' : section === 'Commands and results'
      ? execution.receiptRefs.map((ref) => `- receipt ${ref.id} sha256:${ref.sha256}`).join('\n')
      : 'Synthetic fixture evidence; factual acceptance remains with the Owner.'}\n`).join('\n')
}

function closeFixture(config, execution, change = () => {}) {
  put(config.repoRoot, tasksPath, config.tasks.replaceAll('- [ ] 10.', '- [x] 10.'))
  put(config.repoRoot, reportPath, reportText(execution))
  change(config.repoRoot)
  git(config.repoRoot, ['add', '.'])
  git(config.repoRoot, ['commit', '-qm', 'synthetic closure C'])
}

async function routeFixture(config) {
  const toolchain = { node: process.version, npm: 'fixture-npm', go: 'fixture-go' }
  const evidence = syntheticEvidence(config, config.source, toolchain)
  const commands = []
  const ordering = []
  const logs = []
  let message
  const localProcess = { ...process, env: { ...fixtureEnvironment }, cwd: () => config.repoRoot,
    argv: [process.execPath, gatePath, '--execute', '--closure-report', reportPath,
      '--closure-task-ids', '10.4,10.8,10.9', '--formal-evidence-dir', evidence.formalDirectory, '--json'],
    send: (value, callback) => { message = value; callback(null) }, disconnect: () => {},
    exit: (code) => { throw new Error(`unexpected_route_exit_${code}: ${logs.join('\n')}`) } }
  const context = vm.createContext({ console: { log: (value) => logs.push(value) }, Date, Map, Set })
  const receiptModule = await import('./runtime-go-rc-receipt.mjs')
  const mocks = {
    'node:process': { default: localProcess },
    'node:child_process': { spawnSync: (command, args, options) => {
      if (command === 'git') return spawnSync(command, args, { ...options, env: fixtureEnvironment })
      const id = releaseCommandIds[commands.length]
      assert.ok(id, 'orchestration attempted an extra execution')
      commands.push({ id, command, args, cwd: options.cwd })
      ordering.push(id)
      return { status: 0, stdout: JSON.stringify(evidence.reports.get(id) || {}), stderr: '' }
    } },
    './runtime-go-rc-receipt.mjs': { ...receiptModule, createReceiptRun: (options) =>
      createReceiptRun({ ...options, cacheRoot: config.cacheRoot, resolveToolchain: () => toolchain }) },
    './runtime-go-formal-evidence.mjs': {
      evaluateRuntimeGoCoreFormalEvidence: () => { throw Error('full_execution_must_not_use_core_stage') },
      preflightRuntimeGoFormalEvidence: () => { ordering.push('formal-preflight'); return { passed: true } },
      evaluateRuntimeGoFormalEvidence: (input) => {
        ordering.push('formal-ingestion')
        assert.equal(canonicalJSONString(input.exactFormalArtifact), canonicalJSONString(evidence.legal.exactFormalArtifact))
        assert.equal(input.source.head, config.source.head)
        return evidence.formalEvidenceInput
      }
    }
  }
  const entry = new vm.SourceTextModule(readFileSync(gatePath, 'utf8'), { context, identifier: gatePath })
  await entry.link(async (specifier) => {
    const exports = mocks[specifier] || await import(new URL(specifier, import.meta.url))
    return new vm.SyntheticModule(Object.keys(exports), function () {
      for (const [name, value] of Object.entries(exports)) this.setExport(name, value)
    }, { context })
  })
  await entry.evaluate()
  assert.equal(localProcess.exitCode, 0)
  assert.ok(message?.seal)
  assert.deepEqual(commands.map((entry) => entry.id), releaseCommandIds)
  assert.ok(ordering.indexOf('artifact-legal-obligations-audit') < ordering.indexOf('formal-ingestion'))
  const report = JSON.parse(logs.at(-1))
  assert.equal(report.executionPassed, true)
  assert.equal(report.passed, false)
  assert.equal(report.productRcGatePassed, false)
  return { message, commands, ordering, report }
}

if (process.argv[2] === '--fixture-execution' || process.argv[2] === '--fixture-route') {
  try { console.log(JSON.stringify(process.argv[2] === '--fixture-route'
    ? await routeFixture(JSON.parse(process.argv[3])) : executionFixture(JSON.parse(process.argv[3])))) }
  catch (error) { console.log(JSON.stringify({ error: error.stack })); process.exitCode = 1 }
}

if (!fixtureChild) test('execution can finish with the closure ledger open without claiming final PASS', () => {
  const ledger = parseOpenSpecTaskLedger('- [ ] 10.4 **[RC_REQUIRED]** Execute the gate.\n')
  const substantivePassed = true
  const finalPassed = substantivePassed && ledger.openRcRequiredIds.length === 0
  assert.equal(finalPassed, false)
  assert.equal(gateExitCode({ mode: 'release', controlPlaneReady: true, passed: finalPassed }), 1)
  assert.equal(gateExitCode({ mode: 'execution', executionPassed: substantivePassed, passed: finalPassed }), 0)
})

if (!fixtureChild) test('one execution seals open obligations, then an exited process and exact closure permit read-only final PASS', (t) => {
  const config = fixture(t, { existingReport: true })
  const result = execute(config)
  assert.equal(result.status, 0, JSON.stringify(result))
  const execution = result.output
  assert.equal(execution.executionPassed, true)
  assert.equal(execution.passed, false)
  assert.deepEqual(execution.openRcRequiredIds, ['10.4', '10.8', '10.9'])
  assert.equal(existsSync(join(execution.run.runDir, 'normalized-release-receipt.json')), false)
  assert.throws(() => createReceiptRun({ repoRoot: config.repoRoot, cacheRoot: config.cacheRoot,
    resolveToolchain: () => { throw new Error('duplicate execution reached toolchain/heavy boundary') } }), /already exists/u)
  assert.throws(() => finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path }), /one_direct_child/u)
  closeFixture(config, execution)
  const tasks = readFileSync(join(config.repoRoot, tasksPath))
  const report = readFileSync(join(config.repoRoot, reportPath))
  const indexBefore = readFileSync(join(config.repoRoot, '.git/index'))
  const final = finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path })
  assert.equal(final.passed, true)
  assert.deepEqual({ ...final.testedSourceS }, config.source)
  assert.notEqual(final.closureSourceC.head, config.source.head)
  assert.equal(final.source.head, config.source.head)
  assert.equal(final.releaseAuthorized, false)
  assert.equal(readFileSync(join(config.repoRoot, tasksPath)).equals(tasks), true)
  assert.equal(readFileSync(join(config.repoRoot, reportPath)).equals(report), true)
  assert.equal(readFileSync(join(config.repoRoot, '.git/index')).equals(indexBefore), true)
  const before = readFileSync(final.normalizedReceipt.path)
  const repeated = finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path })
  assert.equal(repeated.repeatedIdenticalInput, true)
  assert.equal(readFileSync(final.normalizedReceipt.path).equals(before), true)
  assert.equal(readFileSync(join(config.repoRoot, '.git/index')).equals(indexBefore), true)
})

if (!fixtureChild) for (const scenario of ['failed', 'interrupted', 'skipped', 'missing', 'formal-failed', 'artifact-failed', 'forged-success', 'task-drift', 'report-drift']) {
  test(`execution refuses a success seal for ${scenario}`, (t) => {
    const config = fixture(t)
    const result = execute(config, scenario)
    assert.equal(result.status, 1, JSON.stringify(result))
    assert.ok(result.output.error)
    const lock = JSON.parse(readFileSync(join(config.cacheRoot, 'evidence/rc-validation', config.source.head, 'formal-run.json')))
    assert.equal(existsSync(join(config.cacheRoot, 'evidence/rc-validation', config.source.head, lock.runId, 'execution-success-seal.json')), false)
  })
}

if (!fixtureChild) for (const scenario of ['product', 'task-requirement', 'classification', 'task-delete', 'extra-doc', 'mode', 'symlink', 'report-missing', 'report-section', 'report-reference', 'real-open', 'extra-commit', 'merge']) {
  test(`final adjudication rejects ${scenario} drift`, (t) => {
    const config = fixture(t, { openRealTask: scenario === 'real-open' })
    const result = execute(config)
    assert.equal(result.status, 0, JSON.stringify(result))
    const execution = result.output
    closeFixture(config, execution, (root) => {
      if (scenario === 'product') put(root, 'product.js', 'export const accepted = false\n')
      if (scenario === 'task-requirement') put(root, tasksPath, readFileSync(join(root, tasksPath), 'utf8').replace('Execute all', 'Skip all'))
      if (scenario === 'classification') put(root, tasksPath, readFileSync(join(root, tasksPath), 'utf8').replace('[RC_REQUIRED]', '[POST_RC]'))
      if (scenario === 'task-delete') put(root, tasksPath, readFileSync(join(root, tasksPath), 'utf8').split('\n').slice(1).join('\n'))
      if (scenario === 'extra-doc') put(root, 'docs/unrelated.md', 'A documentation-only exemption would be unsafe.\n')
      if (scenario === 'mode') chmodSync(join(root, tasksPath), 0o755)
      if (scenario === 'symlink') { rmSync(join(root, reportPath)); symlinkSync('../../../../product.js', join(root, reportPath)) }
      if (scenario === 'report-missing') rmSync(join(root, reportPath))
      if (scenario === 'report-section') put(root, reportPath, reportText(execution).replace('## Metrics', '## Missing metrics'))
      if (scenario === 'report-reference') put(root, reportPath, reportText(execution).replace(execution.receiptRefs[0].sha256, 'c'.repeat(64)))
    })
    if (scenario === 'extra-commit') git(config.repoRoot, ['commit', '--allow-empty', '-qm', 'extra closure commit'])
    if (scenario === 'merge') {
      const tree = git(config.repoRoot, ['rev-parse', 'HEAD^{tree}'])
      const head = git(config.repoRoot, ['rev-parse', 'HEAD'])
      const merge = git(config.repoRoot, ['commit-tree', tree, '-p', head, '-p', config.source.head, '-m', 'synthetic merge'])
      git(config.repoRoot, ['update-ref', 'HEAD', merge])
    }
    assert.throws(() => finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path }))
    assert.equal(existsSync(join(execution.run.runDir, 'normalized-release-receipt.json')), false)
  })
}

if (!fixtureChild) for (const scenario of ['command-receipt', 'formal-report', 'seal', 'dirty-task', 'dirty-report', 'hidden-index']) {
  test(`final adjudication rejects ${scenario} bytes or index drift`, (t) => {
    const config = fixture(t)
    const result = execute(config)
    assert.equal(result.status, 0, JSON.stringify(result))
    const execution = result.output
    closeFixture(config, execution)
    if (scenario === 'command-receipt') writeFileSync(execution.receiptRefs[0].path, '{}\n')
    if (scenario === 'formal-report') writeFileSync(join(execution.run.runDir, 'frozen-formal-a0.json'), '{}\n')
    if (scenario === 'seal') writeFileSync(execution.seal.path, '{}\n')
    if (scenario === 'dirty-task') put(config.repoRoot, tasksPath, config.tasks)
    if (scenario === 'dirty-report') put(config.repoRoot, reportPath, 'Changed after C\n')
    if (scenario === 'hidden-index') git(config.repoRoot, ['update-index', '--assume-unchanged', 'product.js'])
    assert.throws(() => finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path }))
  })
}

if (!fixtureChild) test('final CLI reaches no heavy command and writes neither task ledger nor source', (t) => {
  const config = fixture(t)
  const executionResult = execute(config)
  assert.equal(executionResult.status, 0, JSON.stringify(executionResult))
  const execution = executionResult.output
  closeFixture(config, execution)
  const guard = put(config.root, 'read-only-guard.cjs', `const cp = require('node:child_process'); const fs = require('node:fs'); const path = require('node:path');
const root = ${JSON.stringify(config.repoRoot)};
const original = cp.spawnSync; cp.spawnSync = (command, args, options) => { if (command !== 'git') throw new Error('HEAVY_COMMAND_FORBIDDEN'); return original(command, args, options) };
for (const name of ['writeFileSync','appendFileSync','rmSync','renameSync','chmodSync','mkdirSync']) { const original = fs[name]; fs[name] = (target, ...args) => { if (typeof target === 'string' && path.resolve(target).startsWith(root + path.sep)) throw new Error('SOURCE_OR_LEDGER_WRITE_FORBIDDEN'); return original(target, ...args) } }
require('node:module').syncBuiltinESMExports();\n`)
  const result = spawnSync(process.execPath, ['--require', guard, gatePath, '--finalize', '--execution-seal', execution.seal.path, '--json'], {
    cwd: config.repoRoot, env: fixtureEnvironment, encoding: 'utf8'
  })
  assert.equal(result.status, 0, result.stdout + result.stderr)
  assert.equal(JSON.parse(result.stdout).passed, true)
  assert.equal(git(config.repoRoot, ['status', '--porcelain']), '')
})


if (!fixtureChild) test('actual execution entry orders artifact evaluation before ingestion and seals all eleven commands', (t) => {
  const config = fixture(t)
  const result = spawnSync(process.execPath, ['--experimental-vm-modules', scriptPath, '--fixture-route', JSON.stringify(config)], {
    env: fixtureEnvironment, encoding: 'utf8', maxBuffer: 4 << 20
  })
  assert.equal(result.status, 0, result.stdout + result.stderr)
  const output = JSON.parse(result.stdout)
  assert.equal(output.commands.length, 11)
  assert.equal(output.report.executionPassed, true)
  assert.equal(output.report.passed, false)
})

if (!fixtureChild) for (const termination of ['success', 'exit-7', 'SIGKILL']) {
  test(`supervising wrapper requires actual successful close after seal: ${termination}`, (t) => {
    const config = fixture(t)
    const wrapperSource = readFileSync(new URL('./runtime-go-validation-command.mjs', import.meta.url), 'utf8')
      .replace("'./runtime-go-release-finalization.mjs'", JSON.stringify(new URL('./runtime-go-release-finalization.mjs', import.meta.url).href))
    const wrapperPath = put(config.root, 'wrapper/runtime-go-validation-command.mjs', wrapperSource)
    const resultPath = join(config.root, 'producer-result.json')
    put(config.root, 'wrapper/runtime-go-release-gate.mjs', `
import { writeFileSync } from 'node:fs';
process.argv[2] = '--fixture-module-only';
const { executionFixture } = await import(${JSON.stringify(pathToFileURL(scriptPath).href)});
const result = executionFixture(${JSON.stringify(config)});
writeFileSync(${JSON.stringify(resultPath)}, JSON.stringify(result));
process.send({ type: 'analytix-release-execution-seal', seal: result.seal, runId: result.run.runId }, () => {
  if (${JSON.stringify(termination)} === 'exit-7') process.exit(7);
  if (${JSON.stringify(termination)} === 'SIGKILL') process.kill(process.pid, 'SIGKILL');
  process.disconnect();
});
`)
    const result = spawnSync(process.execPath, [wrapperPath, 'release-execution'], {
      cwd: config.repoRoot, env: fixtureEnvironment, encoding: 'utf8', maxBuffer: 2 << 20
    })
    assert.equal(result.status, termination === 'success' ? 0 : termination === 'exit-7' ? 7 : 1, result.stdout + result.stderr)
    const execution = JSON.parse(readFileSync(resultPath, 'utf8'))
    assert.equal(existsSync(execution.seal.path), true)
    assert.equal(existsSync(join(execution.run.runDir, 'execution-completion-receipt.json')), termination === 'success')
    closeFixture(config, execution)
    if (termination === 'success') {
      const final = finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path })
      assert.equal(final.passed, true)
      assert.ok(final.executionCompletion.sha256)
    } else {
      assert.throws(() => finalizeReleaseExecution({ repoRoot: config.repoRoot, sealPath: execution.seal.path }), /ENOENT/u)
      assert.equal(existsSync(join(execution.run.runDir, 'normalized-release-receipt.json')), false)
    }
  })
}

if (!fixtureChild) test('run-outside formal lock reference is rejected before opening a harmless sentinel', (t) => {
  const config = fixture(t)
  const execution = execute(config).output
  const sentinel = put(config.root, 'harmless-sentinel.json', '{"synthetic":true}\n')
  const opened = join(config.root, 'sentinel-opened')
  const payload = JSON.parse(readFileSync(execution.normalized.path)).payload
  payload.formalRun.path = sentinel
  const normalized = writeAndRevalidateReceipt({ runDir: execution.run.runDir, fileName: 'altered-normalized.json', payload })
  const sealPayload = JSON.parse(readFileSync(execution.seal.path)).payload
  sealPayload.normalized = normalized
  const alteredSeal = writeAndRevalidateReceipt({ runDir: execution.run.runDir, fileName: 'altered-seal.json', payload: sealPayload })
  const guard = put(config.root, 'sentinel-guard.cjs', `
const fs = require('node:fs'); const original = fs.openSync;
fs.openSync = (path, ...args) => {
  if (path === ${JSON.stringify(sentinel)}) { fs.writeFileSync(${JSON.stringify(opened)}, 'opened'); throw new Error('SENTINEL_OPENED'); }
  return original(path, ...args);
};
require('node:module').syncBuiltinESMExports();
`)
  const result = spawnSync(process.execPath, ['--require', guard, gatePath, '--finalize', '--execution-seal', alteredSeal.path], {
    cwd: config.repoRoot, env: fixtureEnvironment, encoding: 'utf8'
  })
  assert.equal(result.status, 1, result.stdout + result.stderr)
  assert.equal(JSON.parse(result.stdout).reason, 'execution_once_lock_path_invalid')
  assert.equal(existsSync(opened), false)
})
