// This executable is run ONLY by Owner after reviewing the source seal.
// --capture-inputs APP SIDECAR OUT records a new build. --owner-run MANIFEST
// accepts bounded JSON-line lifecycle requests on stdin; it never builds.
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { basename, dirname, join, resolve } from 'node:path'
import { homedir } from 'node:os'
import { chmodSync, existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync,
  readdirSync, realpathSync, rmdirSync, unlinkSync, writeFileSync } from 'node:fs'
import { execFileSync, spawn } from 'node:child_process'
import { createInterface } from 'node:readline'
import { isDeepStrictEqual } from 'node:util'
import { StringDecoder } from 'node:string_decoder'
import { b1ProcessEvidenceRoot, b1ProcessHostParent, createB1ExplicitKeychain, exactRootName, identity, pinRoot, requireB1, sha256 } from './runtime-go-b1-process-keychain.mjs'

const require = createRequire(import.meta.url)
const native = require('./native-component-contract.cjs')
const afterPack = require('./after-pack.cjs')._internals
const repo = realpathSync(join(dirname(fileURLToPath(import.meta.url)), '..'))
const evidenceRoot = b1ProcessEvidenceRoot
const nativeRoot = '/Volumes/AnalytixCache/development-v3/native-components-development/darwin-arm64'
const markerName = 'analytix-native-development-build.json'
const authorityName = 'analytix-packaged-build-authority.json'
const expectedMarker = 'bf70e7106e7d5b99b7bf635907ac2493d71b1c59bc6d15dd972a0ad0c846ecd2'
const baseEnvironment = () => ({ PATH: '/usr/bin:/bin:/usr/sbin:/sbin', HOME: homedir(), LANG: 'C', LC_ALL: 'C', TZ: 'UTC', TMPDIR: evidenceRoot })
const exactKeys = (x, keys) => x && typeof x === 'object' && !Array.isArray(x) && Object.keys(x).sort().join('|') === [...keys].sort().join('|')
const digest = (x) => typeof x === 'string' && /^[a-f0-9]{64}$/u.test(x)
const within = (root, path) => typeof path === 'string' && path.startsWith(root + '/') && resolve(path) === path

const rootAncestorIO = Object.freeze({ stat: (path) => lstatSync(path, { bigint: true }), realpath: realpathSync })
// Match the real immutable installer's traversed-directory prerequisites.
// The Owner's host parent is private. Its changing child inventory is not a
// task identity and must never become part of task cleanup.
export function assertB1ProcessRootAncestors(io = rootAncestorIO) {
  try {
    for (let current = b1ProcessHostParent; ; current = dirname(current)) {
      const entry = io.stat(current)
      requireB1(entry.isDirectory() && !entry.isSymbolicLink() && io.realpath(current) === current &&
        (entry.uid === 0n || entry.uid === BigInt(process.geteuid())) && entry.nlink > 0n &&
        (entry.mode & 0o022n) === 0n, 'b1_private_root_ancestor_invalid')
      if (current === b1ProcessHostParent) requireB1(entry.uid === BigInt(process.geteuid()) &&
        (entry.mode & 0o7777n) === 0o700n, 'b1_private_root_ancestor_invalid')
      if (current === '/') return
    }
  } catch {
    throw new Error('b1_private_root_ancestor_invalid')
  }
}

function stableFile(path, maximum = 512 * 1024 * 1024) {
  const a = lstatSync(path, { bigint: true })
  requireB1(a.isFile() && a.nlink === 1n && a.size > 0n && a.size <= BigInt(maximum) &&
    (a.mode & 0o022n) === 0n && realpathSync(path) === path, 'b1_input_file_invalid')
  const bytes = readFileSync(path)
  const b = lstatSync(path, { bigint: true })
  requireB1(['dev', 'ino', 'size', 'mtimeNs', 'ctimeNs', 'mode'].every((key) => a[key] === b[key]), 'b1_input_file_drift')
  return { bytes, binding: { path, sha256: sha256(bytes), bytes: bytes.length, mode: Number(a.mode & 0o7777n) } }
}
function currentNative() {
  const marker = stableFile(join(nativeRoot, markerName), 256 * 1024)
  const value = JSON.parse(marker.bytes)
  requireB1(marker.binding.sha256 === expectedMarker && value.classification === 'development_non_publishable' &&
    value.publishable === false && value.releaseEligible === false && value.authorityUse === 'development_only' &&
    value.sourceSetSha256 === native.sourceSetDigest(repo) && value.buildContextSha256 === native.nativeBuildContextDigest(repo),
  'b1_current_native_source_mismatch')
  for (const component of value.components) {
    const file = stableFile(join(nativeRoot, component.binaryName))
    requireB1(file.binding.sha256 === component.binarySha256 && file.binding.bytes === component.binarySize,
      'b1_current_native_payload_mismatch')
  }
  return value
}
export function assertManifestShape(m) {
  requireB1(exactKeys(m, ['schemaVersion', 'purpose', 'app', 'sidecar', 'runtime', 'source', 'nativeMarkerSHA256', 'files']) &&
    m.schemaVersion === 1 && m.purpose === 'analytix.own2-b1-process-inputs/v1' &&
    within(join(evidenceRoot, 'dist'), m.app) && basename(m.app) === 'analytix.app' &&
    m.runtime === join(m.app, 'Contents/Resources/runtime-go/bin/runtime-server') &&
    within(evidenceRoot, m.sidecar) && basename(m.sidecar) === 'formal-authority-sidecar' &&
    m.nativeMarkerSHA256 === expectedMarker && Array.isArray(m.files) && m.files.length === 9 &&
    m.files.every((f) => exactKeys(f, ['path', 'sha256', 'bytes', 'mode']) && digest(f.sha256) &&
      Number.isSafeInteger(f.bytes) && f.bytes > 0 && Number.isInteger(f.mode)), 'b1_input_manifest_invalid')
  const resources = join(m.app, 'Contents/Resources')
  const paths = [m.runtime, m.sidecar, join(resources, 'app.asar'),
    join(resources, 'runtime', markerName), join(resources, 'runtime', authorityName),
    ...native.manifest.components.map((c) => join(resources, 'runtime', c.binaryName))]
  requireB1(new Set(m.files.map((f) => f.path)).size === paths.length &&
    paths.every((p) => m.files.some((f) => f.path === p)), 'b1_input_manifest_inventory_invalid')
}
function verifyInputs(m) {
  assertManifestShape(m)
  currentNative()
  requireB1(isDeepStrictEqual(afterPack.collectPackagedWorktreeSnapshotV1(repo), m.source), 'b1_source_freeze_changed')
  for (const file of m.files) requireB1(isDeepStrictEqual(stableFile(file.path).binding, file), 'b1_build_input_changed')
  const resources = join(m.app, 'Contents/Resources')
  const authority = JSON.parse(stableFile(join(resources, 'runtime', authorityName)).bytes)
  requireB1(afterPack.isPackagedBuildAuthorityV2(authority) && !authority.publishable && !authority.releaseEligible &&
    authority.nativeDisposition.kind === 'development_non_publishable' &&
    isDeepStrictEqual(authority.worktreeSnapshot, m.source), 'b1_packaged_source_binding_invalid')
  const packagedMarker = JSON.parse(stableFile(join(resources, 'runtime', markerName)).bytes)
  requireB1(isDeepStrictEqual(packagedMarker, currentNative()), 'b1_packaged_native_changed')
  execFileSync('/usr/bin/codesign', ['--verify', '--deep', '--strict', m.app], { env: baseEnvironment(), stdio: 'ignore', timeout: 60_000 })
}
export function captureInputs(app, sidecar, output) {
  requireB1(within(evidenceRoot, output) && !existsSync(output), 'b1_input_output_invalid')
  const source = afterPack.collectPackagedWorktreeSnapshotV1(repo)
  const resources = join(app, 'Contents/Resources')
  const runtime = join(resources, 'runtime-go/bin/runtime-server')
  const paths = [runtime, sidecar, join(resources, 'app.asar'), join(resources, 'runtime', markerName),
    join(resources, 'runtime', authorityName), ...native.manifest.components.map((c) => join(resources, 'runtime', c.binaryName))]
  const m = { schemaVersion: 1, purpose: 'analytix.own2-b1-process-inputs/v1', app, sidecar, runtime,
    source, nativeMarkerSHA256: expectedMarker, files: paths.map((p) => stableFile(p).binding) }
  verifyInputs(m)
  writeFileSync(output, JSON.stringify(m, null, 2) + '\n', { flag: 'wx', mode: 0o600 })
  return sha256(readFileSync(output))
}

function safeSecurityState() {
  const read = (args) => execFileSync('/usr/bin/security', args, { env: baseEnvironment(), encoding: 'utf8', maxBuffer: 65536, timeout: 10_000 })
  return { search: read(['list-keychains', '-d', 'user']), default: read(['default-keychain', '-d', 'user']) }
}
function sameSecurityState(a, b) { return a.search === b.search && a.default === b.default }
export function parseBootstrap(body, ownerRoot) {
  const v = JSON.parse(body)
  requireB1(exactKeys(v, ['schemaVersion', 'purpose', 'authorityAnchorV1', 'authorityManifestRoot',
    'authorityCredentialProfileRoot', 'authorityCredentialBundleRoot']) && v.schemaVersion === 1 &&
    v.purpose === 'analytix.runtime-main-owned-authority/v1' && JSON.stringify(v) === body, 'b1_bootstrap_invalid')
  const anchor = JSON.parse(v.authorityAnchorV1)
  requireB1(exactKeys(anchor, ['schemaVersion', 'installationId', 'authorityKeyId', 'authorityPublicKey', 'currentManifestDigest']) &&
    anchor.schemaVersion === 1 && digest(anchor.installationId) && digest(anchor.currentManifestDigest) &&
    /^[A-Za-z0-9_-]{43}$/u.test(anchor.authorityPublicKey) &&
    sha256(Buffer.from(anchor.authorityPublicKey, 'base64url')) === anchor.authorityKeyId, 'b1_anchor_invalid')
  for (const path of [v.authorityManifestRoot, v.authorityCredentialProfileRoot, v.authorityCredentialBundleRoot]) {
    requireB1(within(ownerRoot, path), 'b1_authority_root_invalid'); identity(path)
  }
  return { ...v, authorityAnchorV1: anchor }
}
export function privateFrame(authority, binding) {
  requireB1(authority && binding, 'b1_explicit_keychain_required')
  const body = Buffer.from(JSON.stringify({ schemaVersion: 1, purpose: 'analytix.runtime-startup-private-frame/v1',
    protectedAuthorityV1: authority, darwinSecretStoreKeychainBindingV1: binding }))
  const frame = Buffer.alloc(8 + body.length)
  frame.writeBigUInt64BE(BigInt(body.length)); body.copy(frame, 8); body.fill(0)
  return frame
}
function processTable() {
  const output = execFileSync('/bin/ps', ['-axo', 'pid=,ppid=,pgid=,comm='], { env: baseEnvironment(), encoding: 'utf8', timeout: 10_000 }).trim()
  requireB1(output.length > 0, 'b1_child_probe_failed')
  return output.split('\n').map((row) => {
    const m = /^\s*(\d+)\s+(\d+)\s+(\d+)\s+(.+)$/u.exec(row)
    requireB1(m && [m[1], m[2], m[3]].every((value) => Number.isSafeInteger(+value)) && +m[1] > 0, 'b1_child_probe_failed')
    return { pid: +m[1], ppid: +m[2], pgid: +m[3], image: m[4] }
  })
}
export function ownedProcesses(rows, pid) {
  const selected = new Set([pid, ...rows.filter((r) => r.pgid === pid).map((r) => r.pid)]); let changed = true
  while (changed) { changed = false; for (const r of rows) if (selected.has(r.ppid) && !selected.has(r.pid)) { selected.add(r.pid); changed = true } }
  return rows.filter((r) => selected.has(r.pid))
}
function descendants(pid) { return ownedProcesses(processTable(), pid) }
const nativeFailurePrefix = '[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 '
const nativeFailureStages = Object.freeze({
  ADMISSION: ['ARGUMENTS', 'NATIVE_CALL', 'OBJECT_VALIDATION', 'SOURCE_STATS', 'MATERIALIZATION', 'SOURCE_ROWS', 'RESULT_CONSTRUCTION', 'UNKNOWN'],
  RUNNER: ['ADMISSION', 'ACQUIRE', 'PREVIOUS_SESSION', 'SOURCE_MATERIALIZATION', 'INPUT_DUPLICATE', 'OUTPUT_PREPARE',
    'REQUEST_FRAME', 'OUTPUT_HANDOFF', 'SESSION_OPEN', 'ROUND_TRIP', 'SESSION_CLOSE', 'POST_SESSION', 'RESULT_PARSE',
    'OUTPUT_SEAL', 'SOURCE_SETTLE', 'INSTALL', 'INSTALLED_VALIDATION', 'CLEANUP', 'UNKNOWN']
})
const nativeFailureClasses = ['TERMINATION', 'CANCELLED', 'DEADLINE', 'PROTOCOL', 'REGISTRY', 'REQUEST_INVALID',
  'SOURCE_NOT_FOUND', 'SOURCE_MISMATCH', 'SOURCE_CORRUPT', 'UNAVAILABLE', 'UNKNOWN']
function validNativeFailure(value) {
  return value && Object.hasOwn(nativeFailureStages, value.layer) &&
    nativeFailureStages[value.layer].includes(value.stage) && nativeFailureClasses.includes(value.class)
}
export function takeVerifiedNativeFailures(owned, role) {
  const records = owned?.guard.nativeDiagnostics.take() || []
  if (!records.length) return []
  if (role !== 'RUNTIME' || !Number.isSafeInteger(owned.verifiedRuntimePID) || owned.verifiedRuntimePID <= 0 ||
    owned.verifiedRuntimePID !== owned.child.pid) {
    owned.failures.add('b1_native_diagnostic_unverified_runtime')
    return []
  }
  return records.map((value) => ({ ...value, role: 'RUNTIME', pid: owned.verifiedRuntimePID }))
}
export function nativeFailureCollector() {
  let pending = '', dropping = false, invalid = false, count = 0
  const records = []
  return { get invalid() { return invalid },
    observe(chunk) {
      for (const byte of chunk) {
        if (byte !== 10) {
          if (pending.length >= 512) {
            if (pending.startsWith(nativeFailurePrefix)) invalid = true
            dropping = true
          }
          if (!dropping) pending += String.fromCharCode(byte)
          continue
        }
        if (!dropping && pending.startsWith(nativeFailurePrefix)) {
          const match = /^layer=([A-Z_]+) stage=([A-Z_]+) class=([A-Z_]+)$/u.exec(pending.slice(nativeFailurePrefix.length))
          const value = match ? { layer: match[1], stage: match[2], class: match[3] } : null
          if (value && validNativeFailure(value) && count < 8) { records.push(value); count += 1 }
          else invalid = true
        }
        pending = ''; dropping = false
      }
    },
    finish() { if (pending.startsWith(nativeFailurePrefix)) invalid = true; pending = ''; dropping = false },
    take() { return records.splice(0) }
  }
}
export function privateOutputGuard(markers = []) {
  const failures = new Set(); const tails = new Map(); const decoders = new Map()
  const nativeDiagnostics = nativeFailureCollector()
  return { failures, nativeDiagnostics,
    finish() { nativeDiagnostics.finish(); if (nativeDiagnostics.invalid) failures.add('b1_native_diagnostic_invalid') },
    observe(channel, chunk) {
    if (!decoders.has(channel)) decoders.set(channel, new StringDecoder('utf8'))
    const text = (tails.get(channel) || '') + decoders.get(channel).write(chunk)
    for (const code of ['ANALYTIX_RUNTIME_ASYNC_TURN_FAILURE_RECORD_FAILED', 'ANALYTIX_RUNTIME_POST_TERMINAL_GOAL_LINEAGE_FAILED', 'ANALYTIX_RUNTIME_SHUTDOWN_DRAIN_RETRY']) {
      if (text.includes(code)) failures.add(code)
    }
    if (markers.some((marker) => text.includes(marker))) failures.add('b1_private_output_detected')
    tails.set(channel, text.slice(-4096))
    // Every byte is inspected above before any diagnostic filtering occurs.
    if (channel === 'stderr') {
      nativeDiagnostics.observe(chunk)
      if (nativeDiagnostics.invalid) failures.add('b1_native_diagnostic_invalid')
    }
  } }
}
function spawnOwned(binary, args, cwd, prefix, markers = []) {
  const child = spawn(binary, args, { cwd, env: baseEnvironment(), detached: true, stdio: ['pipe', 'pipe', 'pipe'] })
  const guard = privateOutputGuard(markers)
  const observed = new Map()
  const observeProcesses = () => {
    if (!child.pid) return
    for (const row of descendants(child.pid)) observed.set(row.pid, row)
  }
  const monitor = setInterval(() => { try { observeProcesses() } catch { guard.failures.add('b1_child_probe_failed') } }, 1000)
  let text = ''; let settled = false; let outputBytes = 0; const failures = new Set()
  const closed = new Promise((done) => child.once('close', (code, signal) => { clearInterval(monitor); guard.finish(); done({ code, signal }) }))
  const ready = new Promise((done, reject) => {
    const timer = setTimeout(() => reject(new Error('b1_child_ready_timeout')), 480_000)
    const finish = (f) => { if (!settled) { settled = true; clearTimeout(timer); f() } }
    child.once('error', () => finish(() => reject(new Error('b1_child_spawn_failed'))))
    child.once('close', () => finish(() => reject(new Error('b1_child_closed_before_ready'))))
    child.stdout.on('data', (chunk) => {
      guard.observe('stdout', chunk)
      outputBytes += chunk.length
      if (outputBytes > 2 * 1024 * 1024) { failures.add('b1_child_output_limit'); return finish(() => reject(new Error('b1_child_output_limit'))) }
      text += chunk.toString('utf8')
      let nl
      while ((nl = text.indexOf('\n')) >= 0) {
        const line = text.slice(0, nl); text = text.slice(nl + 1)
        if (line.startsWith(prefix)) {
          try { const value = JSON.parse(line.slice(prefix.length)); finish(() => done(value)) }
          catch { finish(() => reject(new Error('b1_child_ready_invalid'))) }
        }
      }
    })
  })
  child.stderr.on('data', (chunk) => guard.observe('stderr', chunk))
  child.stdin.on('error', () => failures.add('b1_child_stdin_failed'))
  ready.catch(() => {})
  return { child, ready, closed, failures, guard, observed, observeProcesses }
}
async function closedOutcome(owned, timeout) {
  let timer
  return Promise.race([owned.closed, new Promise((_, reject) => {
    timer = setTimeout(() => reject(new Error('b1_child_drain_timeout')), timeout)
  })]).finally(() => clearTimeout(timer))
}
const childSignals = new Set(['SIGHUP', 'SIGINT', 'SIGQUIT', 'SIGILL', 'SIGTRAP', 'SIGABRT', 'SIGEMT', 'SIGFPE',
  'SIGKILL', 'SIGBUS', 'SIGSEGV', 'SIGSYS', 'SIGPIPE', 'SIGALRM', 'SIGTERM', 'SIGURG', 'SIGSTOP', 'SIGTSTP',
  'SIGCONT', 'SIGCHLD', 'SIGTTIN', 'SIGTTOU', 'SIGIO', 'SIGXCPU', 'SIGXFSZ', 'SIGVTALRM', 'SIGPROF',
  'SIGWINCH', 'SIGINFO', 'SIGUSR1', 'SIGUSR2'])
const childExitCode = (value) => value === null || Number.isSafeInteger(value) && value >= 0 && value <= 255
const childSignal = (value) => value === null || childSignals.has(value)
const childStopIO = Object.freeze({ processes: processTable, closed: closedOutcome })

// Exit status and process drain are separate observations. A failed workload
// must not prevent the latter check or turn a known exit into UNKNOWN.
export async function stopOwned(owned, signal = true, io = childStopIO) {
  const pid = Number.isSafeInteger(owned?.child.pid) && owned.child.pid > 0 ? owned.child.pid : null
  const outcome = { pid, exitCode: null, signal: null, processGroupEmpty: null }
  const fail = (code) => owned.failures.add(code)
  if (pid !== null) {
    try { owned.observeProcesses() } catch { fail('b1_child_probe_failed') }
    try {
      if (signal && owned.child.exitCode === null && owned.child.signalCode === null) owned.child.kill('SIGTERM')
    } catch { fail('b1_child_signal_failed') }
    try {
      const closed = await io.closed(owned, 60_000)
      if (childExitCode(closed.code) && childSignal(closed.signal) && (closed.code === null || closed.signal === null)) {
        outcome.exitCode = closed.code; outcome.signal = closed.signal
      } else fail('b1_child_exit_invalid')
    } catch { fail('b1_child_drain_timeout') }
    try {
      const remaining = io.processes()
      outcome.processGroupEmpty = ownedProcesses(remaining, pid).length === 0 &&
        [...owned.observed.values()].every((c) => !remaining.some((r) => r.pid === c.pid && r.image === c.image))
    } catch { fail('b1_child_probe_failed') }
  } else fail('b1_child_identity_missing')
  const code = pid === null ? 'b1_child_identity_missing'
    : outcome.exitCode !== 0 || outcome.signal !== null ? 'b1_child_exit_failed'
      : outcome.processGroupEmpty !== true ? 'b1_child_residual'
        : owned.failures.size || owned.guard.failures.size ? 'b1_runtime_output_failure' : null
  if (code) throw Object.assign(new Error(code), { childStopOutcome: outcome })
  return { pid, exit: 0, observedProcessIDs: [...owned.observed.keys()].sort((a, b) => a - b), processGroupEmpty: true, observedFailureCodes: [] }
}
function noHandles(root, allowedPIDs = []) {
  let output
  try { output = execFileSync('/usr/sbin/lsof', ['-nP', '-Fpfan', '+D', root], { env: baseEnvironment(), encoding: 'utf8', timeout: 15_000, maxBuffer: 2 * 1024 * 1024 }) }
  catch (error) { requireB1(error.status === 1 && !String(error.stderr || '').trim(), 'b1_handle_probe_failed'); output = String(error.stdout || '') }
  let pid = 0
  for (const line of output.split('\n')) {
    if (line.startsWith('p')) pid = Number(line.slice(1))
    if (/^f\d+/u.test(line)) requireB1(allowedPIDs.includes(pid), 'b1_private_handles_remain')
  }
}
function cleanupEntryAbsent(path) {
  try { lstatSync(path); return false } catch (error) {
    if (error.code === 'ENOENT') return true
    throw new Error('b1_cleanup_presence_unknown')
  }
}
const cleanupIO = Object.freeze({ stat: (path) => lstatSync(path, { bigint: true }), names: readdirSync,
  realpath: realpathSync, absent: cleanupEntryAbsent, noHandles, unlink: unlinkSync, rmdir: rmdirSync })

// One pinned inventory, one destructive attempt. Repeated success only checks
// absence; partial failure freezes this plan for Owner review, never adopts a
// replacement root or retries against a new inventory. IO injection is tests-only.
export function createExactCleanup(root, checkRoot, beforeDelete = () => {}, io = cleanupIO) {
  let completed, failed, removedEntries = 0, inventory = [], manifestSHA256
  const verify = (item) => {
    const s = io.stat(item.path)
    requireB1(io.realpath(item.path) === item.path && s.dev === item.dev && s.ino === item.ino &&
      s.uid === item.uid && s.mode === item.mode && !s.isSymbolicLink() &&
      (item.directory ? s.isDirectory() : s.isFile() && s.nlink === item.links && s.size === item.size),
    'b1_cleanup_identity_changed')
  }
  return () => {
    if (failed) throw failed
    try {
      if (completed) {
        requireB1(io.absent(root), 'b1_cleanup_root_reappeared')
        return completed
      }
      checkRoot(); io.noHandles(root)
      const walk = (path) => {
        requireB1(inventory.length < 30000, 'b1_cleanup_inventory_limit')
        const s = io.stat(path)
        requireB1(io.realpath(path) === path && s.uid === BigInt(process.getuid()) && !s.isSymbolicLink() &&
          (s.isDirectory() || s.isFile()) && (s.mode & 0o077n) === 0n && (s.isDirectory() || s.nlink === 1n),
        'b1_cleanup_unowned_entry')
        const item = { path, dev: s.dev, ino: s.ino, uid: s.uid, mode: s.mode, links: s.nlink, size: s.size, directory: s.isDirectory() }
        inventory.push(item)
        if (item.directory) for (const name of io.names(path)) {
          requireB1(name !== '.' && name !== '..' && basename(name) === name, 'b1_cleanup_entry_name_invalid')
          walk(join(path, name))
        }
      }
      walk(root)
      beforeDelete(); checkRoot(); io.noHandles(root)
      manifestSHA256 = sha256(JSON.stringify(inventory.map((item) => ({ path: item.path.slice(root.length) || '.',
        device: String(item.dev), inode: String(item.ino), mode: String(item.mode), directory: item.directory }))))
      const directories = inventory.filter((item) => item.directory)
      for (const item of [...inventory].reverse()) {
        for (const parent of directories) if (item.path.startsWith(parent.path + '/')) verify(parent)
        verify(item)
        if (item.directory) io.rmdir(item.path); else io.unlink(item.path)
        requireB1(io.absent(item.path), 'b1_cleanup_removal_unconfirmed')
        removedEntries += 1
      }
      requireB1(io.absent(root), 'b1_cleanup_removal_unconfirmed')
      completed = Object.freeze({ removed: true, entries: inventory.length, manifestSHA256, residuals: 0 })
      return completed
    } catch (error) {
      // A thrown operation can have an unknown side effect. Do not turn the
      // unprocessed inventory count into a claim about actual residual state.
      failed = Object.assign(new Error(/^b1_cleanup_[a-z_]{1,100}$/u.test(error?.message) ? error.message : 'b1_cleanup_failed'), {
        cleanup: Object.freeze({ removed: false, entries: inventory.length, removedEntries,
          manifestSHA256: manifestSHA256 ?? null, residuals: null, retryable: false })
      })
      throw failed
    }
  }
}

export function publicHarnessFailure(error) {
  const result = { code: typeof error?.message === 'string' && /^b1_[a-z_]{1,100}$/u.test(error.message)
    ? error.message : 'b1_owner_harness_failed' }
  const fields = ['pathChanged', 'deviceChanged', 'inodeChanged', 'ownerChanged', 'modeChanged',
    'linksChanged', 'canonicalChanged', 'kindChanged', 'sizeInvalid']
  const differences = Object.fromEntries(fields.filter((key) => typeof error?.identityDifferences?.[key] === 'boolean')
    .map((key) => [key, error.identityDifferences[key]]))
  if (Object.keys(differences).length) result.identityDifferences = differences
  if (error?.cleanup?.retryable === false) result.cleanup = {
    removed: false, residuals: null, retryable: false,
    removedEntries: Number.isSafeInteger(error.cleanup.removedEntries) && error.cleanup.removedEntries >= 0
      ? error.cleanup.removedEntries : null
  }
  return result
}

// Separate from the private stdout request protocol and all child output.
// Events describe only checks already completed by this harness.
export function publicProcessObservation(manifestSHA256, event, value = {}) {
  const result = { schemaVersion: 1, manifestSHA256: digest(manifestSHA256) ? manifestSHA256 : null, event: 'UNKNOWN' }
  if (event === 'NATIVE_FAILURE' && value.role === 'RUNTIME' && Number.isSafeInteger(value.pid) && value.pid > 0 && validNativeFailure(value)) {
    Object.assign(result, { event, role: 'RUNTIME', pid: value.pid, layer: value.layer, stage: value.stage, class: value.class })
  } else if (event === 'CHILD_STOP_OUTCOME' && exactKeys(value, ['role', 'pid', 'exitCode', 'signal', 'processGroupEmpty']) &&
    ['RUNTIME', 'SIDECAR', 'MATERIALIZER'].includes(value.role) &&
    (value.pid === null || Number.isSafeInteger(value.pid) && value.pid > 0) &&
    childExitCode(value.exitCode) && childSignal(value.signal) && (value.exitCode === null || value.signal === null) &&
    (value.processGroupEmpty === null || typeof value.processGroupEmpty === 'boolean') &&
    (value.pid !== null || value.exitCode === null && value.signal === null && value.processGroupEmpty === null)) {
    Object.assign(result, { event, ...value })
  } else if (['CHILD_STARTED', 'CHILD_DRAINED', 'CHILD_DRAIN_UNVERIFIED'].includes(event)) {
    result.event = event
    result.role = ['RUNTIME', 'SIDECAR', 'MATERIALIZER'].includes(value.role) ? value.role : 'UNKNOWN'
    result.pid = Number.isSafeInteger(value.pid) && value.pid > 0 ? value.pid : null
  } else if (event === 'FAILURE_RETAINED') {
    result.event = event
    result.childrenDrained = typeof value.childrenDrained === 'boolean' ? value.childrenDrained : null
    // These are exact task-owned inspection targets, not claims of existence.
    result.residualRoot = exactRootName(value.residualRoot) ? value.residualRoot : null
    result.residualWorkspace = within(evidenceRoot, value.residualWorkspace) &&
      dirname(value.residualWorkspace) === evidenceRoot && /^scenario-\d+$/u.test(basename(value.residualWorkspace))
      ? value.residualWorkspace : null
  } else if (event === 'CLEANUP_COMPLETE') result.event = event
  return result
}

async function registryRequest(baseURL, path, body) {
  requireB1(/^http:\/\/127\.0\.0\.1:\d+$/u.test(baseURL), 'b1_provider_readback_origin_invalid')
  try {
    const response = await fetch(baseURL + path, { method: body ? 'POST' : 'GET', redirect: 'error',
      signal: AbortSignal.timeout(15_000), headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined })
    requireB1(response.status === 200 && response.body, 'b1_provider_readback_http_failed')
    const chunks = []; let bytes = 0
    for await (const chunk of response.body) {
      bytes += chunk.length; requireB1(bytes <= 65536, 'b1_provider_readback_response_limit'); chunks.push(chunk)
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8'))
  } catch { throw new Error('b1_provider_readback_http_failed') }
}
async function probeSyntheticProvider(baseURL) {
  const snapshot = await registryRequest(baseURL, '/v1/provider-registry')
  requireB1(Array.isArray(snapshot.providers) && snapshot.providers.length === 1, 'b1_provider_readback_registry_invalid')
  const provider = snapshot.providers[0]
  requireB1(provider.id === 'b1-process-synthetic' && provider.credentialConfigured === true &&
    provider.credentialPurpose === 'provider-api-key', 'b1_provider_readback_registry_invalid')
  const expected = { registryRevision: snapshot.registryRevision, registryIncarnation: snapshot.registryIncarnation,
    providerRevision: provider.revision, providerGeneration: provider.generation,
    providerIncarnation: provider.incarnation, providerCredentialPurpose: provider.credentialPurpose }
  const result = await registryRequest(baseURL, '/v1/provider-registry/providers/b1-process-synthetic/probe', { schemaVersion: 1, expected })
  requireB1(result.status === 'reachable' && result.code === 200 && result.modelCount === 1 &&
    result.providerId === provider.id && Object.entries(expected).filter(([key]) => key !== 'providerCredentialPurpose')
      .every(([key, value]) => result[key] === value), 'b1_provider_readback_failed')
  return true
}

export async function ownerRun(manifestPath) {
  const manifestFile = stableFile(manifestPath, 2 * 1024 * 1024)
  const m = JSON.parse(manifestFile.bytes); verifyInputs(m)
  let root, checkRoot, keychain, sidecar, material, running, authority, beforeSecurity, workspace, checkWorkspace, privateMarkers
  const drains = []
  const reply = (value) => process.stdout.write(JSON.stringify(value) + '\n')
  const observe = (event, value) => process.stderr.write('ANALYTIX_B1_PROCESS_OBSERVATION_V1 ' +
    JSON.stringify(publicProcessObservation(manifestFile.binding.sha256, event, value)) + '\n')
  const publishNativeFailures = (owned, role) => {
    for (const value of takeVerifiedNativeFailures(owned, role)) observe('NATIVE_FAILURE', value)
  }
  const stopAndObserve = async (owned, role, signal = true) => {
    let result
    try {
      result = await stopOwned(owned, signal)
      publishNativeFailures(owned, role)
      requireB1(owned.failures.size === 0, 'b1_native_diagnostic_unverified_runtime')
      observe('CHILD_DRAINED', { role, pid: result.pid })
      return result
    } catch (error) {
      publishNativeFailures(owned, role)
      const outcome = error.childStopOutcome || (result && {
        pid: result.pid, exitCode: result.exit, signal: null, processGroupEmpty: result.processGroupEmpty
      })
      if (outcome) {
        error.childStopOutcome = outcome
        observe('CHILD_STOP_OUTCOME', { role, ...outcome })
      } else observe('CHILD_DRAIN_UNVERIFIED', { role, pid: owned?.child.pid })
      throw error
    }
  }
  try {
    for await (const line of createInterface({ input: process.stdin, crlfDelay: Infinity })) {
      requireB1(line.length < 65536, 'b1_request_limit')
      const request = JSON.parse(line)
      if (request.action === 'begin') {
        requireB1(!root && exactKeys(request, ['action', 'manifestSHA256', 'workspace', 'privateMarkers']) &&
          request.manifestSHA256 === manifestFile.binding.sha256 && within(evidenceRoot, request.workspace), 'b1_begin_invalid')
        requireB1(Array.isArray(request.privateMarkers) && request.privateMarkers.length >= 6 && request.privateMarkers.length <= 16 &&
          request.privateMarkers.every((v) => typeof v === 'string' && v.length >= 5 && v.length < 1024), 'b1_privacy_markers_invalid')
        privateMarkers = request.privateMarkers
        workspace = request.workspace
        requireB1(/^scenario-[A-Za-z0-9]+$/u.test(basename(workspace)) && dirname(workspace) === evidenceRoot, 'b1_workspace_invalid')
        const workspaceIdentity = identity(workspace)
        checkWorkspace = () => requireB1(JSON.stringify(identity(workspace)) === JSON.stringify(workspaceIdentity), 'b1_workspace_drift')
        assertB1ProcessRootAncestors()
        const hostParentIdentity = identity(b1ProcessHostParent)
        const checkHostParent = () => requireB1(JSON.stringify(identity(b1ProcessHostParent)) === JSON.stringify(hostParentIdentity), 'b1_private_host_parent_drift')
        beforeSecurity = safeSecurityState()
        checkHostParent()
        root = mkdtempSync(join(b1ProcessHostParent, 'analytix-own2-b1-process-')); chmodSync(root, 0o700)
        const checkTaskRoot = pinRoot(root)
        checkRoot = () => { checkHostParent(); checkTaskRoot() }
        const userDataDir = join(root, 'user-data'); mkdirSync(userDataDir, { mode: 0o700 })
        assertB1ProcessRootAncestors(); checkRoot()
        sidecar = spawnOwned(m.sidecar, ['--owner-root', join(root, 'sidecar')], repo, 'ANALYTIX_FORMAL_AUTHORITY_READY_V1 ', privateMarkers)
        sidecar.child.stdin.end()
        const ready = await sidecar.ready
        requireB1(processTable().some((r) => r.pid === sidecar.child.pid && r.image === m.sidecar), 'b1_sidecar_image_mismatch')
        observe('CHILD_STARTED', { role: 'SIDECAR', pid: sidecar.child.pid })
        const bootstrap = stableFile(join(root, 'sidecar/authority-bootstrap-v1.json'), 32768)
        requireB1(bootstrap.binding.sha256 === ready.bootstrapSha256, 'b1_sidecar_bootstrap_mismatch')
        authority = parseBootstrap(bootstrap.bytes.toString(), join(root, 'sidecar')); bootstrap.bytes.fill(0)
        keychain = await createB1ExplicitKeychain(root, () => requireB1(sameSecurityState(beforeSecurity, safeSecurityState()), 'b1_keychain_external_state_changed'))
        requireB1(sameSecurityState(beforeSecurity, safeSecurityState()), 'b1_keychain_external_state_changed')
        const dataDir = join(root, 'sidecar/data'); identity(dataDir)
        material = spawnOwned(m.runtime, ['bundled-plugin', 'materialize-funds-v1', '--data-dir', dataDir,
          '--invocation-id', sha256(manifestFile.binding.sha256 + ':materialize')], repo, 'ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 ', privateMarkers)
        material.child.stdin.end(); await material.ready
        await stopAndObserve(material, 'MATERIALIZER', false); material = null
        reply({ ok: true, root, dataDir, userDataDir, authorityKeyID: authority.authorityAnchorV1.authorityKeyId,
          authorityPublicKey: authority.authorityAnchorV1.authorityPublicKey, manifestSHA256: manifestFile.binding.sha256 })
      } else if (request.action === 'start') {
        requireB1(exactKeys(request, ['action']) && root && !running, 'b1_start_invalid'); checkRoot(); verifyInputs(m)
        requireB1(sidecar.child.exitCode === null && sameSecurityState(beforeSecurity, safeSecurityState()), 'b1_provisioning_drift')
        const transition = await keychain.unlock()
        const binding = keychain.binding(join(root, 'user-data'), join(root, 'sidecar/data'))
        running = spawnOwned(m.runtime, ['-insecure', '-data-dir', binding.dataDir, '-user-data-dir', binding.userDataDir,
          '-durable-root', join(workspace, 'durable'), '--private-startup-frame-v1'], workspace, 'ANALYTIX_RUNTIME_SERVER_READY ', privateMarkers)
        const frame = privateFrame(authority, binding)
        await new Promise((done, reject) => { running.child.stdin.once('error', () => reject(new Error('b1_private_frame_write_failed'))); running.child.stdin.end(frame, done) }).finally(() => frame.fill(0))
        const ready = await running.ready
        const anchor = authority.authorityAnchorV1
        requireB1(ready.productionRuntime === true && ready.witnessedAuthorityV2Configured === true &&
          ready.witnessedAuthorityInstallationId === anchor.installationId && ready.witnessedAuthorityKeyId === anchor.authorityKeyId &&
          ready.witnessedAuthorityManifestDigest === anchor.currentManifestDigest && /^http:\/\/127\.0\.0\.1:\d+$/u.test(ready.url), 'b1_runtime_identity_mismatch')
        requireB1(processTable().some((r) => r.pid === running.child.pid && r.image === m.runtime), 'b1_runtime_image_mismatch')
        const port = new URL(ready.url).port
        const listener = execFileSync('/usr/sbin/lsof', ['-nP', '-a', '-p', String(running.child.pid), '-iTCP:' + port, '-sTCP:LISTEN', '-Fn'], { env: baseEnvironment(), encoding: 'utf8', timeout: 10000 })
        requireB1(listener.includes('n127.0.0.1:' + port), 'b1_runtime_listener_mismatch')
        requireB1(!drains.some((d) => d.pid === running.child.pid), 'b1_runtime_pid_reused')
        running.url = ready.url
        keychain.runtimeStarted(running.child.pid)
        running.verifiedRuntimePID = running.child.pid
        observe('CHILD_STARTED', { role: 'RUNTIME', pid: running.child.pid })
        reply({ ok: true, url: ready.url, pid: running.child.pid, witnessSame: true, keychainTransition: transition })
      } else if (request.action === 'begin-provider-write') {
        requireB1(exactKeys(request, ['action']) && running && drains.length === 0, 'b1_provider_write_invalid')
        const snapshot = await registryRequest(running.url, '/v1/provider-registry')
        requireB1(Array.isArray(snapshot.providers) && snapshot.providers.length === 0, 'b1_provider_write_requires_fresh_registry')
        keychain.beginProviderWrite(running.child.pid); reply({ ok: true })
      } else if (request.action === 'verify-provider') {
        requireB1(exactKeys(request, ['action']) && running && running.child.exitCode === null &&
          processTable().some((r) => r.pid === running.child.pid && r.image === m.runtime), 'b1_provider_readback_process_invalid')
        const transition = await keychain.verifyProvider(running.child.pid, () => probeSyntheticProvider(running.url))
        reply({ ok: true, keychainTransition: transition })
      } else if (request.action === 'stop') {
        requireB1(exactKeys(request, ['action']) && running, 'b1_stop_invalid')
        keychain.assertRuntimeCurrent(running.child.pid)
        const drained = await stopAndObserve(running, 'RUNTIME'); drains.push(drained); running = null
        keychain.runtimeStopped(drained.pid)
        noHandles(root, descendants(sidecar.child.pid).map((p) => p.pid)); reply({ ok: true, ...drained })
      } else if (request.action === 'finish') {
        requireB1(exactKeys(request, ['action']) && root && !running && drains.length === 2, 'b1_finish_invalid')
        await stopAndObserve(sidecar, 'SIDECAR'); sidecar = null
        requireB1(sameSecurityState(beforeSecurity, safeSecurityState()), 'b1_keychain_external_state_changed')
        const cleanup = createExactCleanup(root, checkRoot, keychain.assertCleanupReady)()
        keychain.dispose(); keychain = null
        root = null
        const workspaceCleanup = createExactCleanup(workspace, checkWorkspace)()
        workspace = null
        observe('CLEANUP_COMPLETE')
        reply({ ok: true, cleanup, workspaceCleanup, drains, defaultKeychainConfigurationPreserved: true }); return
      } else throw new Error('b1_action_invalid')
    }
    throw new Error('b1_owner_input_closed_without_finish')
  } catch (error) {
    // Failure is retained for Owner inspection. Do not delete partially-owned
    // state or reconfigure a default/search list to hide an external effect.
    let drained = true
    for (const [child, role] of [[running, 'RUNTIME'], [material, 'MATERIALIZER'], [sidecar, 'SIDECAR']]) {
      if (child) { try { await stopAndObserve(child, role) } catch (stopError) {
        if (stopError.childStopOutcome?.processGroupEmpty !== true) drained = false
      } }
    }
    keychain?.dispose()
    observe('FAILURE_RETAINED', { childrenDrained: drained, residualRoot: root, residualWorkspace: workspace })
    reply({ ok: false, ...publicHarnessFailure(error),
      childrenDrained: drained, residualRoot: root || null, residualWorkspace: workspace || null,
      observedFailureCodes: [...new Set([running, material, sidecar].filter(Boolean).flatMap((child) => [...child.failures, ...child.guard.failures]))].sort() })
    process.exitCode = 1
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [operation, ...args] = process.argv.slice(2)
  try {
    if (operation === '--capture-inputs' && args.length === 3) process.stdout.write(captureInputs(...args) + '\n')
    else if (operation === '--owner-run' && args.length === 1) await ownerRun(args[0])
    else throw new Error('b1_owner_command_invalid')
  } catch { process.stderr.write('b1_owner_command_failed\n'); process.exitCode = 1 }
}
