import { spawnSync } from 'node:child_process'
import { createHash, randomUUID } from 'node:crypto'
import {
  chmodSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const OPEN_TASK_CATEGORIES = [
  { label: 'RC_REQUIRED', field: 'openRcRequiredIds' },
  { label: 'POST_RC', field: 'openPostRcIds' }
]

export function canonicalJSONString(value) {
  if (value === undefined) return 'null'
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  return `{${Object.keys(value)
    .filter((key) => value[key] !== undefined)
    .sort((left, right) => left.localeCompare(right))
    .map((key) => `${JSON.stringify(key)}:${canonicalJSONString(value[key])}`)
    .join(',')}}`
}

export function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function emptyOpenSpecLedger(changeName, tasksFile = '') {
  return {
    openspecChange: changeName,
    tasksFile,
    tasksFileSha256: '',
    totalTasks: 0,
    completedTasks: 0,
    openTaskCount: 0,
    openRcRequiredIds: [],
    openPostRcIds: [],
    classificationCounts: {
      RC_REQUIRED: 0,
      POST_RC: 0
    },
    rcLedgerValid: false,
    problems: []
  }
}

export function parseOpenSpecTaskLedger(tasksText, {
  changeName = '',
  tasksFile = '',
  tasksFileSha256 = sha256(tasksText)
} = {}) {
  const projection = emptyOpenSpecLedger(changeName, tasksFile)
  projection.tasksFileSha256 = tasksFileSha256
  const taskIds = new Set()
  const openIds = new Set()

  for (const [index, line] of String(tasksText || '').split(/\r?\n/).entries()) {
    if (!line.startsWith('- [')) continue
    const match = line.match(/^- \[([^\]]*)\]\s+(\S+)(?:\s+(.*))?$/)
    if (!match) {
      projection.problems.push(`task line ${index + 1} is malformed`)
      continue
    }
    const [, checkbox, taskId, description = ''] = match
    projection.totalTasks += 1
    if (taskIds.has(taskId)) projection.problems.push(`duplicate task id: ${taskId}`)
    taskIds.add(taskId)

    if (description.includes('[BENCHMARK_RESEARCH]')) {
      projection.problems.push(`task ${taskId} uses retired classification: BENCHMARK_RESEARCH`)
    }

    if (checkbox === 'x') {
      projection.completedTasks += 1
      continue
    }
    if (checkbox !== ' ') {
      projection.problems.push(`task ${taskId} has invalid checkbox: ${JSON.stringify(checkbox)}`)
      continue
    }

    projection.openTaskCount += 1
    openIds.add(taskId)
    const categories = OPEN_TASK_CATEGORIES.filter(({ label }) =>
      description.includes(`[${label}]`)
    )
    if (categories.length !== 1) {
      projection.problems.push(categories.length === 0
        ? `open task ${taskId} is unclassified`
        : `open task ${taskId} has multiple classifications`)
      continue
    }
    projection[categories[0].field].push(taskId)
  }

  if (projection.totalTasks === 0) projection.problems.push('tasks ledger contains no task rows')
  if (projection.completedTasks + projection.openTaskCount !== projection.totalTasks) {
    projection.problems.push('task completion counts do not match parsed task rows')
  }
  const classifiedOpenIds = [
    ...projection.openRcRequiredIds,
    ...projection.openPostRcIds
  ]
  if (classifiedOpenIds.length !== projection.openTaskCount ||
      new Set(classifiedOpenIds).size !== classifiedOpenIds.length ||
      classifiedOpenIds.some((taskId) => !openIds.has(taskId))) {
    projection.problems.push('classification counts do not match parsed open tasks')
  }
  projection.classificationCounts = {
    RC_REQUIRED: projection.openRcRequiredIds.length,
    POST_RC: projection.openPostRcIds.length
  }
  projection.rcLedgerValid = projection.problems.length === 0
  return projection
}

export function loadOpenSpecTaskLedger({
  repoRoot,
  changeName,
  expectedTasksFileSha256 = ''
}) {
  const tasksFile = `openspec/changes/${changeName}/tasks.md`
  const changeRoot = resolve(repoRoot, 'openspec', 'changes')
  const tasksPath = resolve(repoRoot, tasksFile)
  const invalid = emptyOpenSpecLedger(changeName, tasksFile)
  if (!/^[a-z0-9][a-z0-9-]*$/.test(String(changeName || '')) ||
      !tasksPath.startsWith(`${changeRoot}${sep}`)) {
    invalid.problems.push('OpenSpec change name or path is invalid')
    return invalid
  }

  let initialBytes
  try {
    initialBytes = readFileSync(tasksPath)
  } catch {
    invalid.problems.push('OpenSpec change or tasks file does not exist')
    return invalid
  }
  const initialSha256 = sha256(initialBytes)
  const projection = parseOpenSpecTaskLedger(initialBytes.toString('utf8'), {
    changeName,
    tasksFile,
    tasksFileSha256: initialSha256
  })
  if (expectedTasksFileSha256 && expectedTasksFileSha256 !== initialSha256) {
    projection.problems.push('tasks file hash drifted from the source-freeze projection')
  }
  let finalSha256 = ''
  try {
    finalSha256 = sha256(readFileSync(tasksPath))
  } catch {
    projection.problems.push('tasks file disappeared during projection')
  }
  if (finalSha256 !== initialSha256) {
    projection.problems.push('tasks file changed during projection')
  }
  projection.rcLedgerValid = projection.problems.length === 0
  return projection
}

export function gateExitCode({ mode, controlPlaneReady, executionPassed, passed }) {
  if (mode === 'control-plane') return controlPlaneReady === true ? 0 : 1
  if (mode === 'execution') return executionPassed === true ? 0 : 1
  if (mode === 'release') return passed === true ? 0 : 1
  return 2
}

function commandOutput(command, args, cwd) {
  const result = spawnSync(command, args, {
    cwd,
    env: process.env,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) return ''
  return String(result.stdout || '').trim()
}

export function sourceIdentity(repoRoot) {
  return {
    head: commandOutput('git', ['rev-parse', 'HEAD'], repoRoot),
    tree: commandOutput('git', ['rev-parse', 'HEAD^{tree}'], repoRoot)
  }
}

export function toolchainIdentity(repoRoot) {
  return {
    node: process.version,
    npm: commandOutput(process.platform === 'win32' ? 'npm.cmd' : 'npm', ['--version'], repoRoot),
    go: commandOutput(process.env.GO || 'go', ['version'], repoRoot)
  }
}

export function createReceiptRun({ repoRoot, cacheRoot = process.env.ANALYTIX_DEV_CACHE_ROOT, resolveToolchain = toolchainIdentity }) {
  if (!String(cacheRoot || '').trim()) {
    throw new Error('ANALYTIX_DEV_CACHE_ROOT is required for RC receipts')
  }
  const source = sourceIdentity(repoRoot)
  if (!/^[0-9a-f]{40}$/.test(source.head) || !/^[0-9a-f]{40}$/.test(source.tree)) {
    throw new Error('exact Git HEAD/tree are unavailable')
  }
  const parent = resolve(cacheRoot, 'evidence', 'rc-validation', source.head)
  mkdirSync(parent, { recursive: true, mode: 0o700 })
  chmodSync(parent, 0o700)
  const runId = `${new Date().toISOString().replaceAll(':', '').replaceAll('.', '')}-${randomUUID()}`
  const formalRunPath = join(parent, 'formal-run.json')
  try {
    writeFileSync(formalRunPath, `${JSON.stringify({
      schemaVersion: 1,
      id: 'analytix-rc-validation-formal-run-lock',
      runId,
      source,
      executionPid: process.pid,
      generatedAt: new Date().toISOString()
    }, null, 2)}\n`, {
      encoding: 'utf8',
      mode: 0o600,
      flag: 'wx'
    })
    chmodSync(formalRunPath, 0o600)
  } catch (error) {
    if (error?.code === 'EEXIST') {
      throw new Error(`a formal RC validation run already exists for ${source.head}/${source.tree}; create a new source freeze instead of retrying`)
    }
    throw error
  }
  const runDir = join(parent, runId)
  mkdirSync(runDir, { mode: 0o700 })
  chmodSync(runDir, 0o700)
  return {
    schemaVersion: 1,
    runId,
    runDir,
    formalRunPath,
    formalRunSha256: sha256(readFileSync(formalRunPath)),
    source,
    executionPid: process.pid,
    toolchain: resolveToolchain(repoRoot)
  }
}

export function receiptEnvelope(payload) {
  const payloadSha256 = sha256(canonicalJSONString(payload))
  return {
    schemaVersion: 1,
    id: 'analytix-rc-validation-receipt',
    payloadSha256,
    payload
  }
}

export function validateReceiptEnvelope(envelope, expected = {}) {
  const problems = []
  if (envelope?.schemaVersion !== 1 || envelope?.id !== 'analytix-rc-validation-receipt') {
    problems.push('receipt schema/id mismatch')
  }
  const actualDigest = sha256(canonicalJSONString(envelope?.payload))
  if (envelope?.payloadSha256 !== actualDigest) problems.push('receipt payload digest mismatch')
  for (const [field, value] of Object.entries(expected)) {
    const path = field.split('.')
    let actual = envelope?.payload
    for (const part of path) actual = actual?.[part]
    if (canonicalJSONString(actual) !== canonicalJSONString(value)) {
      problems.push(`receipt binding mismatch: ${field}`)
    }
  }
  return { ok: problems.length === 0, problems, payloadSha256: actualDigest }
}

export function writeAndRevalidateReceipt({ runDir, fileName, payload, expected = {} }) {
  const envelope = receiptEnvelope(payload)
  const path = join(runDir, fileName)
  writeFileSync(path, `${JSON.stringify(envelope, null, 2)}\n`, {
    encoding: 'utf8',
    mode: 0o600,
    flag: 'wx'
  })
  chmodSync(path, 0o600)
  const parsed = JSON.parse(readFileSync(path, 'utf8'))
  const validation = validateReceiptEnvelope(parsed, expected)
  if (!validation.ok) {
    throw new Error(`receipt validation failed: ${validation.problems.join('; ')}`)
  }
  return { path, sha256: sha256(readFileSync(path)), payloadSha256: validation.payloadSha256 }
}

function selfTest() {
  const payload = {
    runId: 'fixture-run',
    source: { head: 'a'.repeat(40), tree: 'b'.repeat(40) },
    command: { id: 'fixture', text: 'node fixture.mjs', cwd: '/fixture' }
  }
  const envelope = receiptEnvelope(payload)
  const valid = validateReceiptEnvelope(envelope, {
    runId: payload.runId,
    source: payload.source,
    'command.text': payload.command.text
  })
  const commandDrift = validateReceiptEnvelope(envelope, {
    'command.text': 'node changed.mjs'
  })
  const staleRun = validateReceiptEnvelope(envelope, { runId: 'historical-run' })
  const sourceDrift = validateReceiptEnvelope(envelope, {
    'source.tree': 'c'.repeat(40)
  })
  const tampered = validateReceiptEnvelope({
    ...envelope,
    payload: { ...payload, runId: 'tampered' }
  })
  const cacheRoot = mkdtempSync(join(tmpdir(), 'analytix-rc-receipt-self-test-'))
  let duplicateFormalRunRejected = false
  try {
    createReceiptRun({ repoRoot: process.cwd(), cacheRoot })
    try {
      createReceiptRun({ repoRoot: process.cwd(), cacheRoot })
    } catch (error) {
      duplicateFormalRunRejected = String(error?.message || error).includes('already exists')
    }
  } finally {
    rmSync(cacheRoot, { recursive: true, force: true })
  }
  const ledgerRoot = mkdtempSync(join(tmpdir(), 'analytix-rc-ledger-self-test-'))
  const validTasks = [
    '- [x] 1.1 completed task',
    '- [ ] 1.2 **[RC_REQUIRED]** product closure',
    '- [ ] 2.1 **[POST_RC]** later work',
    ''
  ].join('\n')
  let validLedger
  let taskHashDriftRejected = false
  let missingChangeRejected = false
  try {
    const tasksDir = join(ledgerRoot, 'openspec', 'changes', 'fixture-change')
    mkdirSync(tasksDir, { recursive: true })
    writeFileSync(join(tasksDir, 'tasks.md'), validTasks)
    validLedger = loadOpenSpecTaskLedger({ repoRoot: ledgerRoot, changeName: 'fixture-change' })
    taskHashDriftRejected = !loadOpenSpecTaskLedger({
      repoRoot: ledgerRoot,
      changeName: 'fixture-change',
      expectedTasksFileSha256: 'f'.repeat(64)
    }).rcLedgerValid
    missingChangeRejected = !loadOpenSpecTaskLedger({
      repoRoot: ledgerRoot,
      changeName: 'missing-change'
    }).rcLedgerValid
  } finally {
    rmSync(ledgerRoot, { recursive: true, force: true })
  }
  const duplicateTaskRejected = !parseOpenSpecTaskLedger(`${validTasks}- [ ] 1.2 **[RC_REQUIRED]** duplicate\n`).rcLedgerValid
  const unclassifiedTaskRejected = !parseOpenSpecTaskLedger('- [ ] 1.1 missing category\n').rcLedgerValid
  const invalidCheckboxRejected = !parseOpenSpecTaskLedger('- [?] 1.1 **[RC_REQUIRED]** invalid\n').rcLedgerValid
  const classificationMismatchRejected = !parseOpenSpecTaskLedger('- [ ] 1.1 **[RC_REQUIRED]** **[POST_RC]** conflicting\n').rcLedgerValid
  const benchmarkResearchRejected = !parseOpenSpecTaskLedger('- [ ] 1.1 **[BENCHMARK_RESEARCH]** retired research\n').rcLedgerValid
  const completedBenchmarkResearchRejected = !parseOpenSpecTaskLedger('- [x] 1.1 **[BENCHMARK_RESEARCH]** retired research\n').rcLedgerValid
  const releaseFalseExitsNonzero = gateExitCode({
    mode: 'release',
    controlPlaneReady: true,
    passed: false
  }) === 1
  const releaseTrueExitsZero = gateExitCode({
    mode: 'release',
    controlPlaneReady: true,
    passed: true
  }) === 0
  const controlPlaneReadyExitsZero = gateExitCode({
    mode: 'control-plane',
    controlPlaneReady: true,
    passed: false
  }) === 0
  const controlPlaneFailureExitsNonzero = gateExitCode({
    mode: 'control-plane',
    controlPlaneReady: false,
    passed: false
  }) === 1
  const passed = valid.ok && !commandDrift.ok && !staleRun.ok && !sourceDrift.ok &&
    !tampered.ok && duplicateFormalRunRejected && validLedger?.rcLedgerValid === true &&
    validLedger?.totalTasks === 3 && validLedger?.completedTasks === 1 &&
    validLedger?.classificationCounts?.RC_REQUIRED === 1 &&
    validLedger?.classificationCounts?.POST_RC === 1 &&
    duplicateTaskRejected && unclassifiedTaskRejected && invalidCheckboxRejected &&
    classificationMismatchRejected && benchmarkResearchRejected && completedBenchmarkResearchRejected &&
    taskHashDriftRejected && missingChangeRejected &&
    releaseFalseExitsNonzero && releaseTrueExitsZero &&
    controlPlaneReadyExitsZero && controlPlaneFailureExitsNonzero
  console.log(JSON.stringify({
    schemaVersion: 1,
    id: 'runtime-go-rc-receipt-self-test',
    status: passed ? 'passed' : 'failed',
    passed,
    cases: {
      valid: valid.ok,
      commandDriftRejected: !commandDrift.ok,
      staleRunRejected: !staleRun.ok,
      sourceDriftRejected: !sourceDrift.ok,
      tamperRejected: !tampered.ok,
      duplicateFormalRunRejected,
      validLedgerProjected: validLedger?.rcLedgerValid === true,
      duplicateTaskRejected,
      unclassifiedTaskRejected,
      invalidCheckboxRejected,
      classificationMismatchRejected,
      benchmarkResearchRejected,
      completedBenchmarkResearchRejected,
      taskHashDriftRejected,
      missingChangeRejected,
      releaseFalseExitsNonzero,
      releaseTrueExitsZero,
      controlPlaneReadyExitsZero,
      controlPlaneFailureExitsNonzero
    }
  }, null, 2))
  if (!passed) process.exitCode = 1
}

if (process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1]) && process.argv.includes('--self-test')) {
  selfTest()
}
