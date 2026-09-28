export function observedSingleInstanceLockFailure(text) {
  return /gotSingleInstanceLock["']?\s*[:=]\s*false/iu.test(text)
}

export function processProbeFailureKind(error, command) {
  if (error?.code === 'ETIMEDOUT') return 'timeout'
  // pgrep uses exit 1 for a successful search with no matches. Every other
  // failed probe is unknown until the target process is independently checked.
  if (command === 'pgrep' && error?.status === 1) return 'no_match'
  return 'unknown'
}

export function exactTaskGroupMembers(processRows, groupPid, appContentsRoot) {
  if (!Number.isSafeInteger(groupPid) || groupPid <= 0 || !appContentsRoot) return null
  const members = []
  for (const line of processRows.split(/\r?\n/u)) {
    const match = line.trim().match(/^(\d+)\s+(\d+)(?:\s+(.+))?$/u)
    if (!match || Number(match[2]) !== groupPid) continue
    const pid = Number(match[1])
    if (!Number.isSafeInteger(pid) || pid <= 0 || !match[3] ||
        !match[3].startsWith(`${appContentsRoot}/`)) return null
    members.push(pid)
  }
  return members
}

export function exactMainCommandMatches(commandRow, parentPid, mainPid, executable, debugPort) {
  const match = commandRow.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/u)
  return Boolean(match && Number(match[1]) === parentPid && Number(match[2]) === mainPid &&
    match[3].startsWith(`${executable} --remote-debugging-port=${debugPort} `))
}

export function exactCoreRuntimeCommandMatches(command, executable, runtimeDataDir) {
  if (!command || !executable || !runtimeDataDir) return false
  const args = command.trim().split(/\s+/u)
  if (args[0] !== executable || args[1] !== '--addr' || !args[2]) return false
  if (args.filter((arg) => arg === '--data-dir').length !== 1 ||
      args.filter((arg) => arg === '--private-startup-frame-v1').length !== 1) return false
  const dataDirIndex = args.indexOf('--data-dir')
  return args[dataDirIndex + 1] === runtimeDataDir
}

export function normalQuitFailureCode({ quitRequestOk, targetClosed, mainExited, mainExitCode,
  mainSignal, fallbackUsed, residualProcessCount }) {
  if (fallbackUsed) return 'normal_quit_required_fallback'
  if (!quitRequestOk) return 'normal_quit_request_failed'
  if (!targetClosed) return 'normal_quit_target_stayed_open'
  if (!mainExited) return 'exact_main_process_did_not_exit'
  if (mainExitCode !== 0 || mainSignal) return 'normal_quit_main_abnormal_exit'
  if (residualProcessCount === null) return 'task_owned_process_residual_unknown'
  if (residualProcessCount !== 0) return 'task_owned_process_residual'
  return ''
}

export function classifyExactTargetList(targets, capturedTargetId) {
  if (typeof capturedTargetId !== 'string' || capturedTargetId.length === 0 ||
      capturedTargetId.length > 128 || !Array.isArray(targets) ||
      targets.some((target) => !target || typeof target !== 'object' ||
        typeof target.id !== 'string')) return 'malformed'
  return targets.some((target) => target.id === capturedTargetId)
    ? 'target_present' : 'target_absent'
}

export function residualMembersAlreadyPinned(currentPids, pinnedPids) {
  return Array.isArray(currentPids) && currentPids.every((pid) => pinnedPids.has(pid))
}

export function parseRuntimeStartupNumericDiagnostic(line) {
  const match = line.match(/^\[analytix\] \[main\] event=main_info detail=(\{.*\})$/u)
  if (!match) return null
  try {
    const data = JSON.parse(match[1])
    if (!data || typeof data !== 'object' || Array.isArray(data) ||
        Object.keys(data).length !== 1) return null
    for (const key of ['maxEventLoopLagMs', 'maxConcurrentWaitForHealthProbes']) {
      if (Object.hasOwn(data, key) && Number.isSafeInteger(data[key]) && data[key] >= 0) {
        return { key, value: data[key] }
      }
    }
  } catch { /* malformed line */ }
  return null
}

const goStartupPhases = Object.freeze({
  'go preflight:begin': 'preflightBegin',
  'go capability materialization:done': 'capabilityMaterializationDone',
  'go authority:done': 'authorityDone',
  'go process:spawned': 'processSpawned',
  'go private frame:done': 'privateFrameDone',
  'go ready line:received': 'readyLineReceived',
  'go ready identity:verified': 'readyIdentityVerified',
  'go adapter:done': 'adapterDone',
  'go adapter:failed': 'adapterFailed'
})

const goOwnerPhases = new Set([
  'funds_package_inspected', 'funds_source_inspected', 'funds_static_admitted',
  'funds_active_resolved', 'funds_ready_built', 'go_lease_acquired',
  'go_semantic_prepared', 'go_listener_bound', 'go_activated',
  'go_host_probed', 'go_ready_built'
])

export function parseGoOwnerPhase(line) {
  const match = line.match(/^\[analytix\] \[main\] event=main_info detail=(\{.*\})$/u)
  if (!match) return null
  try {
    const data = JSON.parse(match[1])
    if (!data || typeof data !== 'object' || Array.isArray(data) ||
        Object.keys(data).length !== 2 || !goOwnerPhases.has(data.stage) ||
        !Number.isSafeInteger(data.durationMs) || data.durationMs < 0 ||
        data.durationMs > 30 * 60_000) return null
    return { stage: data.stage, durationMs: data.durationMs }
  } catch { return null }
}

const runtimeTransitionPhases = new Set([
  'runtime settings apply:queued',
  'runtime settings apply:begin',
  'runtime settings apply:restart begin',
  'runtime settings apply:restart done',
  'runtime settings apply:done',
  'runtime settings apply:failed',
  'runtime IPC restart:requested',
  'runtime IPC restart:done',
  'runtime restart:begin',
  'runtime restart:after settings apply',
  'runtime restart:done',
  'window close:requested',
  'window all closed',
  'app before quit:begin',
  'app before quit:prepared',
  'app before quit:closing',
  'app before quit:close allowed',
  'app before quit:close veto prior',
  'app before quit:close veto unprepared',
  'app before quit:close veto frame',
  'app before quit:close veto loading',
  'app before quit:close veto late',
  'app before quit:window closed',
  'app before quit:close unknown',
  'app before quit:windows closed',
  'app before quit:office teardown settled',
  'app before quit:office teardown failed',
  'app before quit:stop failed',
  'app before quit:blocked',
  'app before quit:runtime stopped',
  'app before quit:committed',
  'app before quit:cancelled'
])

export function parseRuntimeTransitionPhase(line) {
  const match = line.match(/^\[analytix\] \[main\] event=main_info detail=(\{.*\})$/u)
  if (!match) return null
  try {
    const data = JSON.parse(match[1])
    if (!data || typeof data !== 'object' || Array.isArray(data) ||
        Object.keys(data).length !== 2 || !runtimeTransitionPhases.has(data.stage) ||
        !Number.isSafeInteger(data.elapsedMs) || data.elapsedMs < 0) return null
    return { phase: data.stage, elapsedMs: data.elapsedMs }
  } catch { return null }
}

export function parseGoStartupPhase(line) {
  const match = line.match(/^\[analytix\] \[main\] event=main_info detail=(\{.*\})$/u)
  if (!match) return null
  try {
    const data = JSON.parse(match[1])
    if (!data || typeof data !== 'object' || Array.isArray(data) ||
        Object.keys(data).length !== 2 || typeof data.stage !== 'string' ||
        !Object.hasOwn(goStartupPhases, data.stage) ||
        !Number.isSafeInteger(data.elapsedMs) || data.elapsedMs < 0) return null
    return { phase: goStartupPhases[data.stage], elapsedMs: data.elapsedMs }
  } catch { return null }
}

export function isGoStartupAttemptMarker(line) {
  return parseGoStartupPhase(line)?.phase === 'preflightBegin'
}

const fundsCSVAdmissionPrefixV2 = 'ANALYTIX_FUNDS_CSV_ADMISSION_V2 '
const fundsCSVAdmissionAnomalyPrefixV1 = 'ANALYTIX_FUNDS_CSV_ADMISSION_ANOMALY_V1 '
const fundsCSVActivationCodesV2 = new Set([
  'native_owner_unavailable',
  'shared_evidence_enrollment_absent',
  'shared_evidence_credential_unavailable',
  'dataset_snapshot_unavailable',
  'ready',
  'other_unavailable'
])

export function parseFundsCSVAdmissionMarkerV2(line) {
  if (typeof line !== 'string' || line.length > 128 || !line.startsWith(fundsCSVAdmissionPrefixV2)) return null
  const fixed = line.slice(fundsCSVAdmissionPrefixV2.length)
  if (fixed === 'semantic_preparation not_evaluated') {
    return { phase: 'semantic_preparation', code: 'not_evaluated' }
  }
  const activationPrefix = 'activation '
  if (!fixed.startsWith(activationPrefix)) return null
  const code = fixed.slice(activationPrefix.length)
  return fundsCSVActivationCodesV2.has(code) ? { phase: 'activation', code } : null
}

function parseFundsCSVAdmissionAnomalyV1(line) {
  if (typeof line !== 'string' || line.length > 128 || !line.startsWith(fundsCSVAdmissionAnomalyPrefixV1)) return null
  const phase = line.slice(fundsCSVAdmissionAnomalyPrefixV1.length)
  return phase === 'semantic_preparation' || phase === 'activation' ? phase : null
}

// K10 sees Main's stdout, not Core's private stderr. Only a complete Main
// stdout line after the actual Core spawn can be attributed to that attempt.
export function createFundsCSVAdmissionAttemptRecorderV2(enabled) {
  let attemptCount = 0
  let current = null
  const attempts = []
  return {
    acceptLine(channel, line, complete = true) {
      if (!enabled || channel !== 'stdout' || !complete) return
      const phase = parseGoStartupPhase(line)?.phase
      if (phase === 'preflightBegin') {
        attemptCount = Math.min(attemptCount + 1, 9)
        current = attemptCount <= 8
          ? { coreAttemptOrdinal: attemptCount, spawned: false, semanticPreparationCode: null, activationCode: null, duplicate: false }
          : null
        if (current) attempts.push(current)
        return
      }
      if (!current) return
      if (phase === 'processSpawned') {
        current.spawned = true
        return
      }
      if (phase === 'adapterDone' || phase === 'adapterFailed') {
        current = null
        return
      }
      if (!current.spawned) return
      if (parseFundsCSVAdmissionAnomalyV1(line)) {
        current.duplicate = true
        return
      }
      const marker = parseFundsCSVAdmissionMarkerV2(line)
      if (!marker) return
      const key = marker.phase === 'activation' ? 'activationCode' : 'semanticPreparationCode'
      if (current[key]) current.duplicate = true
      else current[key] = marker.code
    },
    evidence() {
      return attempts.filter((attempt) => attempt.spawned)
        .map(({ coreAttemptOrdinal, semanticPreparationCode, activationCode, duplicate }) => {
          const status = semanticPreparationCode === 'not_evaluated' && activationCode && !duplicate
            ? 'complete' : 'incomplete'
          const nativeOwner = status !== 'complete' ? 'unknown'
            : activationCode === 'native_owner_unavailable' ? 'unavailable' : 'available'
          return { coreAttemptOrdinal, status, semanticPreparationCode, code: activationCode, nativeOwner }
        })
    }
  }
}

export function projectFundsCSVAdmissionDiagnosticV2(launchRecorders) {
  const observations = []
  for (const [index, recorder] of launchRecorders.entries()) {
    for (const entry of recorder.evidence()) {
      if (!Number.isSafeInteger(entry.coreAttemptOrdinal) || entry.coreAttemptOrdinal < 1 ||
          entry.coreAttemptOrdinal > 8) continue
      observations.push({ launchOrdinal: index + 1, ...entry })
    }
  }
  return observations.length > 0 ? { fundsCSVAdmissionDiagnostic: observations } : {}
}
