import { spawnSync } from 'node:child_process'
import { constants, closeSync, fstatSync, lstatSync, openSync, readFileSync, writeFileSync, existsSync } from 'node:fs'
import { createRequire } from 'node:module'
import { dirname, isAbsolute, join, resolve, sep } from 'node:path'
import { canonicalJSONString, parseOpenSpecTaskLedger, sha256, sourceIdentity, validateReceiptEnvelope, writeAndRevalidateReceipt } from './runtime-go-rc-receipt.mjs'
import { closeGitEnvironment, normalizeUpstreamArtifactProjection } from './upstream-source-audit.mjs'

const { parseStrictJsonObject } = createRequire(import.meta.url)('./lib/strict-json.cjs')

export const releaseCommandIds = Object.freeze([
  'preflight-source-inspection', 'upstream-source-audit', 'product-sovereignty',
  'typecheck', 'build-runtime', 'full-root-test', 'go-ordinary-matrix',
  'go-prod-matrix', 'go-race-matrix', 'rc-structural-and-performance-suite',
  'artifact-legal-obligations-audit'
])
export const finalReportSections = Object.freeze([
  'Root cause', 'Architecture', 'Implementation files and lines',
  'Commands and results', 'Metrics', 'Package result', 'Residual risks',
  'Unverified platforms and external environments', 'Final aggregation'
])
const changeName = 'case-evidence-publication-gate'
const tasksFile = `openspec/changes/${changeName}/tasks.md`
const closureTaskIds = ['10.4', '10.8', '10.9']
const digestPattern = /^[0-9a-f]{64}$/u

function requireCondition(condition, code) {
  if (!condition) throw new Error(code)
}

function same(left, right) { return canonicalJSONString(left) === canonicalJSONString(right) }

function git(repoRoot, args) {
  const result = spawnSync('git', ['-c', 'core.fsmonitor=false', '-c', 'core.filemode=true', ...args], {
    cwd: repoRoot, env: { ...closeGitEnvironment({ ...process.env }), GIT_OPTIONAL_LOCKS: '0',
      GIT_CONFIG_GLOBAL: process.platform === 'win32' ? 'NUL' : '/dev/null', GIT_CONFIG_NOSYSTEM: '1' }, encoding: null,
    stdio: 'pipe', maxBuffer: 32 << 20
  })
  requireCondition(result.status === 0, 'finalization_git_read_failed')
  return result.stdout
}

function stableBytes(path, maxBytes = 32 << 20) {
  let current = resolve(path)
  while (true) {
    requireCondition(!lstatSync(current).isSymbolicLink(), 'finalization_symlink_rejected')
    const parent = dirname(current)
    if (parent === current) break
    current = parent
  }
  const before = lstatSync(path)
  requireCondition(before.isFile() && before.nlink === 1 && before.size <= maxBytes, 'finalization_file_type_or_size_invalid')
  const fd = openSync(path, constants.O_RDONLY | (constants.O_NOFOLLOW || 0))
  try {
    const opened = fstatSync(fd)
    const bytes = readFileSync(fd)
    const after = lstatSync(path)
    const identity = (value) => [value.dev, value.ino, value.mode, value.size, value.mtimeMs, value.ctimeMs]
    requireCondition(same(identity(before), identity(opened)) && same(identity(before), identity(after)) && bytes.length === before.size,
      'finalization_file_changed_during_read')
    return bytes
  } finally { closeSync(fd) }
}

function strictObject(bytes) {
  return parseStrictJsonObject(bytes, { maxBytes: 32 << 20, maxDepth: 64, maxTokens: 1000000, maxStringBytes: 16 << 20 })
}

function repoPath(path) {
  requireCondition(typeof path === 'string' && /^[a-zA-Z0-9_./-]+$/u.test(path) && !isAbsolute(path) &&
    path.split('/').every((part) => part && part !== '.' && part !== '..'), 'closure_path_invalid')
  return path
}

function taskText(bytes) {
  return new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes)
}

function gitFile(repoRoot, commit, path) {
  repoPath(path)
  const entry = git(repoRoot, ['ls-tree', '-z', commit, '--', path]).toString('utf8')
  if (!entry) return null
  const match = entry.match(/^([0-9]{6}) blob ([0-9a-f]{40})\t([^\0]+)\0$/u)
  requireCondition(match && match[3] === path && ['100644', '100755'].includes(match[1]), 'closure_git_type_invalid')
  const bytes = git(repoRoot, ['cat-file', 'blob', match[2]])
  return { path, mode: match[1], blob: match[2], sha256: sha256(bytes), bytes }
}

function descriptor(file) {
  if (!file) return null
  const { bytes: _bytes, ...value } = file
  return value
}

function assertWorktree(repoRoot, source) {
  requireCondition(same(sourceIdentity(repoRoot), source), 'finalization_source_drift')
  requireCondition(git(repoRoot, ['status', '--porcelain=v1', '-z', '--untracked-files=all']).length === 0,
    'finalization_worktree_or_index_dirty')
  const flags = git(repoRoot, ['ls-files', '-v', '-z']).toString('utf8').split('\0').filter(Boolean)
  requireCondition(flags.every((entry) => entry.startsWith('H ')), 'finalization_hidden_index_entry')
}

function assertWorktreeFile(repoRoot, file) {
  const path = join(repoRoot, file.path)
  const bytes = stableBytes(path)
  const executable = (lstatSync(path).mode & 0o111) !== 0
  requireCondition(sha256(bytes) === file.sha256 && executable === (file.mode === '100755'), 'closure_worktree_hash_or_mode_drift')
  return bytes
}

export function freezeReleaseClosure({ repoRoot, source, taskIds, reportPath }) {
  assertWorktree(repoRoot, source)
  requireCondition(Array.isArray(taskIds) && taskIds.length > 0 && new Set(taskIds).size === taskIds.length &&
    taskIds.every((id) => closureTaskIds.includes(id)), 'closure_task_ids_invalid')
  repoPath(reportPath)
  requireCondition(reportPath.startsWith('docs/analytix/qa/') && reportPath.endsWith('.md'), 'closure_report_path_invalid')
  const tasks = gitFile(repoRoot, source.head, tasksFile)
  requireCondition(tasks?.mode === '100644', 'closure_tasks_missing_or_wrong_mode')
  const ledger = parseOpenSpecTaskLedger(taskText(assertWorktreeFile(repoRoot, tasks)))
  requireCondition(ledger.rcLedgerValid && taskIds.every((id) => ledger.openRcRequiredIds.includes(id)), 'closure_tasks_not_open_rc_rows')
  const report = gitFile(repoRoot, source.head, reportPath)
  if (report) {
    requireCondition(report.mode === '100644', 'closure_report_mode_invalid')
    assertWorktreeFile(repoRoot, report)
  } else requireCondition(!existsSync(join(repoRoot, reportPath)), 'closure_report_untracked')
  return { tasks: descriptor(tasks), taskIds: [...taskIds].sort(), reportPath, reportBefore: descriptor(report) }
}

function readReceipt(ref, runDir, expected) {
  requireCondition(isAbsolute(ref?.path || '') && dirname(ref.path) === runDir && digestPattern.test(ref.sha256 || ''), 'receipt_reference_invalid')
  const bytes = stableBytes(ref.path)
  requireCondition(sha256(bytes) === ref.sha256 && (lstatSync(ref.path).mode & 0o777) === 0o600, 'receipt_bytes_or_mode_drift')
  const envelope = strictObject(bytes)
  requireCondition(validateReceiptEnvelope(envelope, expected).ok && envelope.payloadSha256 === ref.payloadSha256, 'receipt_envelope_or_binding_invalid')
  return envelope.payload
}

function verifyExecutionReceipts({ repoRoot, runDir, normalized, manifest, closure }) {
  const payload = readReceipt(normalized, runDir, { receiptType: 'normalized-execution', commandMode: 'execution', passed: false })
  const { source, runId, toolchain, receiptRefs, checks } = payload
  requireCondition(closure?.tasks?.path === tasksFile && closure.reportPath.startsWith('docs/analytix/qa/') &&
    closure.reportPath.endsWith('.md') && closure.taskIds.length > 0 && new Set(closure.taskIds).size === closure.taskIds.length &&
    closure.taskIds.every((id) => closureTaskIds.includes(id)), 'execution_closure_protocol_invalid')
  requireCondition(same(manifest.map((entry) => entry.id), releaseCommandIds) && manifest.every((entry) => entry.expectedExecutions === 1), 'execution_manifest_invalid')
  requireCondition(Array.isArray(checks) && same(checks.map((check) => check.id), releaseCommandIds) && receiptRefs?.length === releaseCommandIds.length + 3,
    'execution_receipt_denominator_invalid')
  requireCondition(new Set(receiptRefs.map((ref) => ref.id)).size === receiptRefs.length, 'execution_receipt_duplicate')
  requireCondition(same(payload.closure, closure) && same(payload.executionManifest, manifest), 'execution_freeze_binding_drift')
  requireCondition(payload.formalRun?.path === join(dirname(runDir), 'formal-run.json') &&
    typeof runId === 'string' && /^[a-zA-Z0-9_-]+$/u.test(runId) && /^[0-9a-f]{40}$/u.test(source?.head || '') &&
    digestPattern.test(payload.formalRun.sha256 || '') && resolve(runDir) === join(dirname(runDir), runId) &&
    dirname(runDir).endsWith(`${sep}${source.head}`), 'execution_once_lock_path_invalid')
  const lockBytes = stableBytes(payload.formalRun.path)
  const lock = strictObject(lockBytes)
  requireCondition(sha256(lockBytes) === payload.formalRun.sha256 && lock.id === 'analytix-rc-validation-formal-run-lock' &&
    lock.runId === runId && same(lock.source, source) && Number.isSafeInteger(lock.executionPid) && lock.executionPid > 0,
  'execution_once_lock_binding_invalid')
  const reports = new Map()
  for (const [index, entry] of manifest.entries()) {
    const check = checks[index]
    requireCondition(check.command === entry.command && check.cwd === entry.cwd && check.executionCount === 1 &&
      check.sourceStable === true && check.status === 'passed' && check.exitStatus === 0 &&
      check.heavyweight === entry.heavyweight && same(check.logicalChecks, entry.logicalChecks), 'execution_command_not_passed_once')
    const ref = receiptRefs.find((ref) => ref.id === entry.id)
    const command = readReceipt(ref, runDir, {
      receiptType: 'command', runId, source, toolchain, runtimeBinary: payload.runtimeBinary,
      command: { id: entry.id, text: entry.command, cwd: entry.cwd },
      'result.status': 'passed', 'result.exitStatus': 0, 'result.sourceStable': true, 'result.executionCount': 1
    })
    if (command.report) reports.set(entry.id, command.report)
  }
  const performance = reports.get('rc-structural-and-performance-suite')
  for (const id of ['structural-performance', 'rc-performance-benchmark']) {
    const receipt = readReceipt(receiptRefs.find((ref) => ref.id === id), runDir, {
      receiptType: 'logical-check', runId, source, toolchain, runtimeBinary: payload.runtimeBinary,
      'command.id': id, 'command.executionOwner': 'rc-structural-and-performance-suite'
    })
    const result = id === 'structural-performance' ? performance?.structural : performance?.benchmark
    requireCondition(result && Object.entries(result).every(([key, value]) => same(receipt.result[key], value)), 'logical_receipt_report_drift')
  }
  const formal = readReceipt(receiptRefs.find((ref) => ref.id === 'formal-evidence-ingestion'), runDir, {
    receiptType: 'formal-evidence-ingestion', runId, source, toolchain, result: payload.formalEvidenceInput
  }).result
  requireCondition(formal?.provided === true && formal.accepted === true && formal.problems?.length === 0 &&
    ['a0', 'b1', 'electron', 'package'].every((lane) => formal.statuses?.[lane] === 'passed'), 'formal_ingestion_not_passed')
  const tasks = gitFile(repoRoot, source.head, tasksFile)
  requireCondition(same(descriptor(tasks), closure.tasks) && payload.tasksFileSha256 === tasks.sha256, 'initial_tasks_binding_drift')
  const rcLedger = parseOpenSpecTaskLedger(taskText(tasks.bytes), { changeName, tasksFile, tasksFileSha256: tasks.sha256 })
  const evaluated = evaluateReleaseExecution({ initialSource: source, receiptRun: { runDir, toolchain }, checks, receiptRefs, reports, formalEvidenceInput: formal, rcLedger })
  requireCondition(evaluated.executionPassed === true && payload.executionPassed === true, 'substantive_execution_not_passed')
  for (const key of ['controlPlaneReady', 'heavyExecutionCounts', 'performanceBindingChecks', 'boundaryProjectionChecks',
    'upstreamArtifactProjectionChecks', 'artifactAdmissionGatePassed', 'upstreamArtifactAdmissionGatePassed', 'operationalRcBlockers']) {
    requireCondition(same(evaluated[key], payload[key]), 'execution_projection_drift')
  }
  return { payload, lock }
}

export function sealReleaseExecution({ repoRoot, receiptRun, normalized, manifest, closure, formalEvidenceDirectory }) {
  const runDir = receiptRun.runDir
  const { payload, lock } = verifyExecutionReceipts({ repoRoot, runDir, normalized, manifest, closure })
  requireCondition(lock.executionPid === process.pid, 'execution_seal_wrong_process')
  requireCondition(same(freezeReleaseClosure({ repoRoot, source: receiptRun.source, taskIds: closure.taskIds, reportPath: closure.reportPath }), closure), 'execution_closure_inputs_drift')
  const formalReports = []
  for (const lane of ['a0', 'b1']) {
    const ref = payload.formalEvidenceInput.reports[lane]
    requireCondition(ref?.fileName === `packaged-milestone-${lane[0]}.json`, 'formal_report_reference_invalid')
    const bytes = stableBytes(join(formalEvidenceDirectory, ref.fileName), 16 << 20)
    requireCondition(sha256(bytes) === ref.sha256 && bytes.length === ref.byteLength, 'formal_report_changed_after_validation')
    const path = join(runDir, `frozen-formal-${lane}.json`)
    writeFileSync(path, bytes, { flag: 'wx', mode: 0o600 })
    formalReports.push({ lane, path, sha256: sha256(bytes), byteLength: bytes.length })
  }
  assertWorktree(repoRoot, receiptRun.source)
  return writeAndRevalidateReceipt({ runDir, fileName: 'execution-success-seal.json', payload: {
    receiptType: 'execution-success', runId: receiptRun.runId, source: receiptRun.source,
    toolchain: receiptRun.toolchain, executionPid: process.pid, passed: false, executionPassed: true,
    closure, manifest, normalized, formalReports
  } })
}

// Only the supervising wrapper calls this with its own ChildProcess close
// result and IPC message. No CLI or environment exit-code assertion is accepted.
export function confirmSupervisedReleaseExecution({ repoRoot, message, observed }) {
  requireCondition(observed?.exitStatus === 0 && !observed.signal && !observed.spawnError && !observed.interruptedSignal &&
    Number.isSafeInteger(observed.pid) && observed.pid > 0, 'execution_did_not_exit_successfully')
  requireCondition(message?.type === 'analytix-release-execution-seal', 'execution_completion_message_invalid')
  const runDir = dirname(message.seal.path)
  const seal = readReceipt(message.seal, runDir, { receiptType: 'execution-success', executionPassed: true,
    passed: false, executionPid: observed.pid, runId: message.runId })
  const verified = verifyExecutionReceipts({ repoRoot, runDir, normalized: seal.normalized, manifest: seal.manifest, closure: seal.closure })
  requireCondition(verified.lock.executionPid === observed.pid && seal.runId === verified.payload.runId &&
    same(seal.source, verified.payload.source) && same(seal.toolchain, verified.payload.toolchain) &&
    same(freezeReleaseClosure({ repoRoot, source: seal.source, taskIds: seal.closure.taskIds, reportPath: seal.closure.reportPath }), seal.closure),
  'execution_completion_binding_drift')
  return writeAndRevalidateReceipt({ runDir, fileName: 'execution-completion-receipt.json', payload: {
    receiptType: 'execution-completion', runId: seal.runId, source: seal.source, executionSeal: message.seal,
    executor: { pid: observed.pid, command: observed.command, args: observed.args },
    result: { exitStatus: 0, signal: null, spawnError: null, interruptedSignal: null },
    supervisorPid: process.pid, passed: false, executionCompleted: true
  } })
}

function validateReport(bytes, sealRef, seal, receiptRefs) {
  const text = new TextDecoder('utf-8', { fatal: true }).decode(bytes)
  const headings = [...text.matchAll(/^## (.+)$/gmu)]
  requireCondition(same(headings.map((match) => match[1]), finalReportSections), 'final_report_sections_incomplete')
  for (const [index, heading] of headings.entries()) {
    const body = text.slice(heading.index + heading[0].length, headings[index + 1]?.index ?? text.length).trim()
    requireCondition(body.length > 0, 'final_report_section_empty')
    if (heading[1] === 'Final aggregation') requireCondition(body === 'pending', 'final_report_preclaims_final_pass')
  }
  requireCondition(text.includes(`Execution run: ${seal.runId}\n`) && text.includes(`Execution seal SHA256: ${sealRef.sha256}\n`), 'final_report_run_binding_invalid')
  for (const ref of receiptRefs) requireCondition(text.includes(`- receipt ${ref.id} sha256:${ref.sha256}\n`), 'final_report_receipt_reference_missing')
}

function closureDiff(repoRoot, seal) {
  const source = sourceIdentity(repoRoot)
  assertWorktree(repoRoot, source)
  const parents = git(repoRoot, ['rev-list', '--parents', '-n', '1', source.head]).toString('utf8').trim().split(' ')
  requireCondition(same(parents, [source.head, seal.source.head]), 'closure_requires_one_direct_child_commit')
  const closure = seal.closure
  const before = gitFile(repoRoot, seal.source.head, tasksFile)
  const reportBefore = gitFile(repoRoot, seal.source.head, closure.reportPath)
  requireCondition(same(descriptor(before), closure.tasks) && same(descriptor(reportBefore), closure.reportBefore), 'closure_initial_objects_drift')
  const after = gitFile(repoRoot, source.head, tasksFile)
  const report = gitFile(repoRoot, source.head, closure.reportPath)
  requireCondition(after?.mode === before.mode && report?.mode === '100644', 'closure_type_or_mode_drift')
  let expected = taskText(before.bytes)
  for (const id of closure.taskIds) {
    requireCondition(closureTaskIds.includes(id), 'closure_task_id_not_permitted')
    const needle = `- [ ] ${id} `
    const rows = expected.split('\n')
    requireCondition(rows.filter((row) => row.startsWith(needle)).length === 1, 'closure_task_row_missing')
    expected = rows.map((row) => row.startsWith(needle) ? row.replace('- [ ]', '- [x]') : row).join('\n')
  }
  requireCondition(after.bytes.equals(Buffer.from(expected)), 'closure_changed_non_checkbox_bytes')
  const ledger = parseOpenSpecTaskLedger(after.bytes.toString('utf8'), { changeName, tasksFile, tasksFileSha256: after.sha256 })
  requireCondition(ledger.rcLedgerValid && ledger.openRcRequiredIds.length === 0, 'closure_real_rc_obligations_open')
  const paths = git(repoRoot, ['diff-tree', '--no-commit-id', '--no-renames', '--name-only', '-r', '-z', seal.source.head, source.head]).toString('utf8').split('\0').filter(Boolean).sort()
  requireCondition(same(paths, [tasksFile, closure.reportPath].sort()), 'closure_extra_or_missing_paths')
  const raw = git(repoRoot, ['diff-tree', '--no-commit-id', '--no-renames', '--raw', '-r', '-z', seal.source.head, source.head])
  assertWorktreeFile(repoRoot, after)
  const reportBytes = assertWorktreeFile(repoRoot, report)
  return { source, ledger, tasks: descriptor(after), report: descriptor(report), reportBytes, diff: { paths, rawBase64: raw.toString('base64'), sha256: sha256(raw) } }
}

export function finalizeReleaseExecution({ repoRoot, sealPath }) {
  const runDir = dirname(resolve(sealPath))
  const bytes = stableBytes(sealPath)
  const envelope = strictObject(bytes)
  const ref = { path: resolve(sealPath), sha256: sha256(bytes), payloadSha256: envelope.payloadSha256 }
  const seal = readReceipt(ref, runDir, { receiptType: 'execution-success', executionPassed: true, passed: false })
  try {
    process.kill(seal.executionPid, 0)
    throw new Error('execution_process_has_not_exited')
  } catch (error) {
    requireCondition(error?.code === 'ESRCH', 'execution_process_exit_unverified')
  }
  const verified = verifyExecutionReceipts({ repoRoot, runDir, normalized: seal.normalized, manifest: seal.manifest, closure: seal.closure })
  requireCondition(seal.executionPid === verified.lock.executionPid && seal.runId === verified.payload.runId &&
    same(seal.source, verified.payload.source) && same(seal.toolchain, verified.payload.toolchain), 'execution_seal_binding_drift')
  const completionPath = join(runDir, 'execution-completion-receipt.json')
  const completionBytes = stableBytes(completionPath)
  const completionEnvelope = strictObject(completionBytes)
  const completionRef = { path: completionPath, sha256: sha256(completionBytes), payloadSha256: completionEnvelope.payloadSha256 }
  readReceipt(completionRef, runDir, { receiptType: 'execution-completion', runId: seal.runId,
    source: seal.source, executionSeal: ref, 'executor.pid': seal.executionPid,
    result: { exitStatus: 0, signal: null, spawnError: null, interruptedSignal: null }, executionCompleted: true, passed: false })
  requireCondition(seal.formalReports?.length === 2 && new Set(seal.formalReports.map((value) => value.lane)).size === 2, 'formal_frozen_denominator_invalid')
  for (const frozen of seal.formalReports) {
    const original = verified.payload.formalEvidenceInput.reports[frozen.lane]
    requireCondition(original && frozen.path === join(runDir, `frozen-formal-${frozen.lane}.json`), 'formal_frozen_path_invalid')
    const raw = stableBytes(frozen.path, 16 << 20)
    requireCondition(sha256(raw) === original.sha256 && sha256(raw) === frozen.sha256 && raw.length === original.byteLength && raw.length === frozen.byteLength,
      'formal_frozen_bytes_drift')
    requireCondition(strictObject(raw).sourceCommit === seal.source.head, 'formal_frozen_source_drift')
  }
  const closure = closureDiff(repoRoot, seal)
  validateReport(closure.reportBytes, ref, seal, verified.payload.receiptRefs)
  // Re-read the bound files after all checks. This phase never launches checks,
  // rewrites source/tasks, or relabels the S-bound package/A0/B1 receipts as C.
  verifyExecutionReceipts({ repoRoot, runDir, normalized: seal.normalized, manifest: seal.manifest, closure: seal.closure })
  const rechecked = closureDiff(repoRoot, seal)
  for (const frozen of seal.formalReports) requireCondition(sha256(stableBytes(frozen.path)) === frozen.sha256, 'formal_frozen_bytes_drift')
  requireCondition(sha256(stableBytes(completionPath)) === completionRef.sha256, 'execution_completion_receipt_drift')
  requireCondition(same(closure, rechecked) && sha256(stableBytes(sealPath)) === ref.sha256, 'finalization_inputs_changed')
  const payload = {
    receiptType: 'normalized-release', commandMode: 'finalization', runId: seal.runId,
    source: seal.source, testedSourceS: seal.source, closureSourceC: closure.source,
    executionSeal: ref, executionCompletion: completionRef, initialTasksSha256: seal.closure.tasks.sha256, finalTasksSha256: closure.tasks.sha256,
    report: closure.report, closureDiff: closure.diff, openRcRequiredIds: closure.ledger.openRcRequiredIds,
    rcLedgerValid: true, status: 'passed', passed: true, commandPassed: true, productRcGatePassed: true,
    engineeringProductRcReady: true, releaseAuthorized: false,
    toolchain: seal.toolchain, runtimeBinary: verified.payload.runtimeBinary,
    artifactAdmissionGatePassed: true, upstreamArtifactAdmissionGatePassed: true,
    externalReleaseAuthorization: verified.payload.externalReleaseAuthorization,
    claimCeiling: verified.payload.claimCeiling,
    reportStructuralCompleteness: true, reportFactualReview: 'Owner responsibility; not established by structural validation',
    formalEvidence: verified.payload.formalEvidence, receiptRefs: verified.payload.receiptRefs
  }
  const path = join(runDir, 'normalized-release-receipt.json')
  if (existsSync(path)) {
    const existingBytes = stableBytes(path)
    const existing = strictObject(existingBytes)
    const existingRef = { path, sha256: sha256(existingBytes), payloadSha256: existing.payloadSha256 }
    requireCondition(same(readReceipt(existingRef, runDir, { runId: seal.runId }), payload), 'finalization_repeat_input_drift')
    return { ...payload, normalizedReceipt: existingRef, repeatedIdenticalInput: true }
  }
  const receipt = writeAndRevalidateReceipt({ runDir, fileName: 'normalized-release-receipt.json', payload })
  return { ...payload, normalizedReceipt: receipt, repeatedIdenticalInput: false }
}

function exactRepoInputBinding(binding) {
  return binding &&
    typeof binding.path === 'string' && binding.path.length > 0 &&
    /^[0-9a-f]{64}$/.test(String(binding.sha256 || '')) &&
    /^[0-9a-f]{40}$/.test(String(binding.gitBlob || '')) &&
    binding.inputMode === 'release-git-object' &&
    binding.readbackMatched === true
}

function exactUpstreamSourceEvidence(binding) {
  const licenseObject = binding?.licenseObject
  const classification = licenseObject?.classification
  const classificationIdentityValid = licenseObject?.path === null
    ? classification === null
    : classification &&
      ['recognized', 'unrecognized'].includes(classification.status) &&
      classification.contentSha256 === licenseObject?.sha256 &&
      (
        classification.status === 'recognized' &&
        typeof classification.licenseId === 'string' &&
        classification.licenseId.length > 0 &&
        typeof classification.detectedClass === 'string' &&
        classification.detectedClass.length > 0 ||
        classification.status === 'unrecognized' &&
        classification.licenseId === null &&
        classification.detectedClass === null
      )
  const licenseIdentityValid = licenseObject &&
    /^[0-9a-f]{40}$/.test(String(licenseObject.commit || '')) &&
    licenseObject.readbackMatched === true &&
    (
      licenseObject.path === null &&
      licenseObject.blob === null &&
      licenseObject.sha256 === null ||
      typeof licenseObject.path === 'string' && licenseObject.path.length > 0 &&
      /^[0-9a-f]{40}$/.test(String(licenseObject.blob || '')) &&
      /^[0-9a-f]{64}$/.test(String(licenseObject.sha256 || ''))
    ) &&
    classificationIdentityValid
  return binding &&
    typeof binding.sourceId === 'string' && binding.sourceId.length > 0 &&
    typeof binding.licenseClass === 'string' && binding.licenseClass.length > 0 &&
    /^[0-9a-f]{64}$/.test(String(binding.manifestEntrySha256 || '')) &&
    exactRepoInputBinding(binding.provenanceLedger) &&
    licenseIdentityValid
}

function upstreamProjectionHash(projection) {
  if (!projection || typeof projection !== 'object' || Array.isArray(projection)) return ''
  const { projectionSha256: _projectionSha256, ...payload } = projection
  return sha256(canonicalJSONString(payload))
}

export function evaluateReleaseExecution({ initialSource, receiptRun, checks, receiptRefs, reports, formalEvidenceInput, rcLedger, failed = false }) {
  const performanceReport = reports.get('rc-structural-and-performance-suite') || null
  const preflight = reports.get('preflight-source-inspection') || {}
  const artifactLegalPlan = reports.get('artifact-legal-obligations-audit') || {}
  const upstreamSourceAudit = reports.get('upstream-source-audit') || {}
  const upstreamArtifactAdmission = upstreamSourceAudit.artifactAdmission || {}
  const upstreamArtifactState = normalizeUpstreamArtifactProjection(
    upstreamArtifactAdmission
  )
  const upstreamArtifactEntryViews = upstreamArtifactState.entryViews
  const upstreamArtifactSourceEvidence = Array.isArray(upstreamArtifactAdmission.inputs?.sourceEvidence)
    ? upstreamArtifactAdmission.inputs.sourceEvidence
    : []
  const upstreamArtifactProjectionChecks = {
    report: upstreamSourceAudit.schemaVersion === 2 &&
      upstreamSourceAudit.id === 'upstream-source-audit' &&
      upstreamArtifactAdmission.schemaVersion === 1 &&
      upstreamArtifactAdmission.id === 'analytix.upstream-artifact-admission/v1',
    source: upstreamSourceAudit.releaseSource?.verified === true &&
      upstreamSourceAudit.releaseSource?.inputMode === 'release-git-object' &&
      upstreamSourceAudit.releaseSource?.head === initialSource.head &&
      upstreamSourceAudit.releaseSource?.tree === initialSource.tree &&
      upstreamArtifactAdmission.releaseSource?.verified === true &&
      upstreamArtifactAdmission.releaseSource?.head === initialSource.head &&
      upstreamArtifactAdmission.releaseSource?.tree === initialSource.tree,
    manifest: upstreamArtifactAdmission.inputs?.manifest?.declaredAnalytixCommit ===
        upstreamSourceAudit.analytixCommit &&
      exactRepoInputBinding(upstreamArtifactAdmission.inputs?.manifest),
    provenanceLedger: exactRepoInputBinding(
      upstreamArtifactAdmission.inputs?.provenanceLedger
    ),
    sourceEvidence: upstreamArtifactSourceEvidence.length > 0 &&
      upstreamArtifactSourceEvidence.every(exactUpstreamSourceEvidence) &&
      upstreamArtifactEntryViews.every((entry) =>
        upstreamArtifactSourceEvidence.filter(
          (binding) => binding.sourceId === entry.sourceId
        ).length === 1
      ) &&
      upstreamArtifactSourceEvidence.every((binding) =>
        upstreamArtifactEntryViews.some((entry) => entry.sourceId === binding.sourceId)
      ),
    entries: upstreamArtifactState.entriesArrayValid &&
      upstreamArtifactEntryViews.length > 0 &&
      upstreamArtifactEntryViews.every((entry) => entry.structureValid),
    hash: /^[0-9a-f]{64}$/.test(String(upstreamArtifactAdmission.projectionSha256 || '')) &&
      upstreamArtifactAdmission.projectionSha256 === upstreamSourceAudit.projectionSha256 &&
      upstreamArtifactAdmission.projectionSha256 === upstreamProjectionHash(upstreamArtifactAdmission),
    researchDiagnosticOnly: ['passed', 'stale', 'unverified'].includes(
      upstreamSourceAudit.researchFreshness?.status
    ) && upstreamSourceAudit.researchFreshness?.releaseBlocking === false
  }
  const upstreamArtifactProjectionValid = Object.values(
    upstreamArtifactProjectionChecks
  ).every(Boolean)
  const upstreamArtifactLicenseClassGatePassed = upstreamArtifactSourceEvidence.every(
    (binding) => binding.licenseObject?.path === null ||
      binding.licenseObject?.classification?.status === 'recognized' &&
      binding.licenseObject.classification.detectedClass === binding.licenseClass
  )
  const upstreamArtifactAdmissionGatePassed = upstreamArtifactProjectionValid &&
    upstreamArtifactLicenseClassGatePassed &&
    upstreamArtifactAdmission.ok === true &&
    upstreamArtifactAdmission.status === 'passed' &&
    upstreamArtifactAdmission.engineeringAdmission === true &&
    upstreamArtifactEntryViews.every((entry) =>
      entry.status === 'passed' && entry.problems.length === 0
    ) &&
    upstreamArtifactState.projectionProblemsValid &&
    upstreamArtifactState.projectionProblems.length === 0
  const upstreamArtifactAdmissionBlockers = upstreamArtifactState.blockers
  const legalPlanBlockers = Array.isArray(artifactLegalPlan.blockers)
    ? artifactLegalPlan.blockers
    : []
  const formalEvidence = {
    ...formalEvidenceInput.statuses,
    commercialRelease: 'not_authorized'
  }
  const formalEvidenceBlockers = Object.entries(formalEvidenceInput.statuses)
    .filter(([, status]) => status !== 'passed')
    .map(([lane, status]) => `${lane}-formal:${status}`)
  const operationalRcBlockers = [...new Set([
    ...(preflight.finalGateBlockers || []).map((value) => `preflight:${value}`),
    ...legalPlanBlockers.map((entry) =>
      `artifact-legal:${entry.artifactEntry || 'unknown-entry'}:${entry.missingAction || 'missing required action'}`
    ),
    ...upstreamArtifactAdmissionBlockers.map((entry) =>
      `upstream-artifact:${entry.item}:${entry.sourceId}:${entry.disposition}:${entry.problem}`
    ),
    ...formalEvidenceBlockers
  ])]
  const rcRequired = [...new Set([
    ...rcLedger.openRcRequiredIds.map((taskId) => `openspec:${taskId}`),
    ...operationalRcBlockers
  ])]
  function executionCount(id) {
    return checks
      .filter((item) => item.id === id)
      .reduce((total, item) => total + Number(item.executionCount || 0), 0)
  }
  const heavyExecutionCounts = {
    typecheck: executionCount('typecheck'),
    build_runtime: executionCount('build-runtime'),
    full_root_test: executionCount('full-root-test'),
    go_ordinary_matrix: executionCount('go-ordinary-matrix'),
    go_prod_matrix: executionCount('go-prod-matrix'),
    go_race_matrix: executionCount('go-race-matrix'),
    structural_performance: performanceReport?.structural ? 1 : 0,
    rc_performance_benchmark: performanceReport?.benchmark ? 1 : 0
  }
  const expectedHeavyCounts = Object.values(heavyExecutionCounts).every((count) => count === 1)
  const sourceIdentityValid = /^[0-9a-f]{40}$/.test(initialSource.head) &&
    /^[0-9a-f]{40}$/.test(initialSource.tree)
  const performanceBindingChecks = {
    source: sourceIdentityValid &&
      performanceReport?.source?.identityValid === true &&
      performanceReport?.source?.head === initialSource.head &&
      performanceReport?.source?.tree === initialSource.tree &&
      performanceReport?.source?.sourceBoundToHeadTree === true,
    host: performanceReport?.host?.supported === true,
    toolchain: performanceReport?.toolchain?.node === receiptRun.toolchain.node &&
      performanceReport?.toolchain?.npm === receiptRun.toolchain.npm &&
      performanceReport?.toolchain?.go === receiptRun.toolchain.go,
    binary: performanceReport?.runtimeBinary?.buildCount === 1 &&
      performanceReport?.runtimeBinary?.productionTagUsed === true &&
      /^[0-9a-f]{64}$/.test(String(performanceReport?.runtimeBinary?.sha256 || '')),
    sampling: performanceReport?.benchmark?.warmupCount === 2 &&
      performanceReport?.benchmark?.measuredCount === 20 &&
      performanceReport?.benchmark?.validWarmupCount === 2 &&
      performanceReport?.benchmark?.validMeasuredCount === 20 &&
      performanceReport?.benchmark?.statistic === 'nearest-rank-p95' &&
      performanceReport?.benchmark?.thresholdMs === 250 &&
      performanceReport?.benchmark?.warmups?.length === 2 &&
      performanceReport?.benchmark?.samples?.length === 20 &&
      performanceReport.benchmark.warmups.every((sample) => Number.isFinite(sample.first_runtime_event_ms)) &&
      performanceReport.benchmark.samples.every((sample) => Number.isFinite(sample.first_runtime_event_ms)),
    authorization: performanceReport?.rcPerformanceAuthorization === true &&
      performanceReport?.releaseAuthorization === false
  }
  const performanceBindingValid = Object.values(performanceBindingChecks).every(Boolean)
  const receiptIntegrityValid = performanceReport?.structural !== undefined &&
    performanceReport?.benchmark !== undefined &&
    receiptRefs.length === checks.length + 3 &&
    receiptRefs.every((receipt) =>
      typeof receipt.path === 'string' &&
      receipt.path.startsWith(`${receiptRun.runDir}/`) &&
      /^[0-9a-f]{64}$/.test(String(receipt.sha256 || '')) &&
      /^[0-9a-f]{64}$/.test(String(receipt.payloadSha256 || ''))
    )
  const boundaryProjectionChecks = {
    artifactLegalPlan: artifactLegalPlan.id === 'analytix.artifact-legal-obligations/v1' &&
      artifactLegalPlan.sourcePackagePlan?.passed === true &&
      artifactLegalPlan.artifactLegalPlanPassed === true,
    exactArtifactProjection: artifactLegalPlan.id === 'analytix.artifact-legal-obligations/v1' &&
      (
        artifactLegalPlan.exactFormalArtifact?.provided === true &&
        artifactLegalPlan.exactFormalArtifact?.engineeringAdmission === artifactLegalPlan.artifactAdmissionGatePassed ||
        artifactLegalPlan.exactFormalArtifact?.provided === false &&
        artifactLegalPlan.exactFormalArtifact?.status === 'UNVERIFIED' &&
        artifactLegalPlan.artifactAdmissionGatePassed === false
      ),
    upstreamArtifactAdmission: upstreamArtifactProjectionValid
  }
  const boundaryProjectionValid = Object.values(boundaryProjectionChecks).every(Boolean)
  const controlPlaneReady = !failed &&
    sourceIdentityValid &&
    expectedHeavyCounts &&
    performanceBindingValid &&
    receiptIntegrityValid &&
    boundaryProjectionValid &&
    rcLedger.rcLedgerValid &&
    performanceReport?.passed === true
  // Source/package metadata and exact artifact admission remain separate.  Only
  // the exact dependency inventory can authorize the artifact admission bit;
  // commercial, signing, notarization, and release decisions are projected
  // separately and never become engineering blockers here.
  const artifactAdmissionGatePassed = artifactLegalPlan.exactFormalArtifact?.provided === true &&
    artifactLegalPlan.exactFormalArtifact?.engineeringAdmission === true
  const artifactLegalPlanPassed = artifactLegalPlan.artifactLegalPlanPassed === true
  const executionPassed = controlPlaneReady &&
    artifactAdmissionGatePassed &&
    upstreamArtifactAdmissionGatePassed &&
    operationalRcBlockers.length === 0
  return { controlPlaneReady, executionPassed, artifactAdmissionGatePassed, artifactLegalPlanPassed, artifactLegalPlan, upstreamArtifactProjectionValid, upstreamArtifactProjectionChecks, upstreamArtifactLicenseClassGatePassed, upstreamArtifactAdmissionGatePassed, upstreamArtifactAdmission, upstreamArtifactAdmissionBlockers, upstreamSourceAudit, formalEvidence, operationalRcBlockers, rcRequired, heavyExecutionCounts, expectedHeavyCounts, sourceIdentityValid, performanceBindingChecks, performanceBindingValid, receiptIntegrityValid, boundaryProjectionChecks, boundaryProjectionValid }
}
