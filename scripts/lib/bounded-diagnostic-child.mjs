import { spawn, execFileSync } from 'node:child_process'

const PROBE_TIMEOUT_MS = 750
const MAX_GROUP_MEMBERS = 24

function pause(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

function ps(args) {
  return execFileSync('/bin/ps', args, {
    encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], timeout: PROBE_TIMEOUT_MS
  }).trim()
}

function identity(pid) {
  if (!Number.isSafeInteger(pid) || pid <= 0) return null
  try {
    const command = ps(['-p', String(pid), '-o', 'ppid=,pgid=,command='])
    const born = ps(['-p', String(pid), '-o', 'lstart='])
    const repeatBorn = ps(['-p', String(pid), '-o', 'lstart='])
    const match = command.match(/^(\d+)\s+(\d+)\s+(.+)$/u)
    return match && born && born === repeatBorn
      ? { pid, ppid: Number(match[1]), pgid: Number(match[2]), command: match[3], born }
      : null
  } catch { return null }
}

function sameIdentity(pinned, allowReparent = false) {
  const current = identity(pinned.pid)
  return Boolean(current && current.born === pinned.born &&
    (allowReparent || current.ppid === pinned.ppid) && current.pgid === pinned.pgid &&
    current.command === pinned.command)
}

async function pinChild(child, command, script, closed) {
  const deadline = Date.now() + 5_000
  while (Date.now() < deadline && !closed()) {
    const candidate = identity(child.pid)
    if (candidate && candidate.ppid === process.pid && candidate.pgid === child.pid &&
        candidate.command.startsWith(`${command} ${script} `)) return candidate
    await pause(50)
  }
  return null
}

function groupMembers(groupPid) {
  let rows
  try { rows = ps(['-axo', 'pid=,pgid=,command=']) } catch { return null }
  const members = []
  for (const row of rows.split(/\r?\n/u)) {
    const match = row.trim().match(/^(\d+)\s+(\d+)\s+(.+)$/u)
    if (match && Number(match[2]) === groupPid) members.push(Number(match[1]))
  }
  return members
}

async function stopPinned(pinned, allowReparent = false) {
  if (!sameIdentity(pinned, allowReparent)) return
  try { process.kill(pinned.pid, 'SIGTERM') } catch { return }
  for (let i = 0; i < 20 && sameIdentity(pinned, allowReparent); i += 1) await pause(100)
  if (!sameIdentity(pinned, allowReparent)) return
  try { process.kill(pinned.pid, 'SIGKILL') } catch { return }
  for (let i = 0; i < 20 && sameIdentity(pinned, allowReparent); i += 1) await pause(100)
}

async function stopOwnedGroup(root, allowedResidualCommand) {
  if (!root || !sameIdentity(root)) return { verified: false, reason: 'root_identity_unavailable' }
  const pids = groupMembers(root.pid)
  if (!pids || pids.length > MAX_GROUP_MEMBERS || !pids.includes(root.pid)) {
    return { verified: false, reason: 'group_inventory_unavailable_or_oversize' }
  }
  const members = []
  for (const pid of pids) {
    const pinned = identity(pid)
    if (!pinned || pinned.pgid !== root.pid ||
        (pid !== root.pid && !allowedResidualCommand(pinned.command))) {
      return { verified: false, reason: 'group_member_identity_untrusted' }
    }
    members.push(pinned)
  }
  if (!sameIdentity(root)) return { verified: false, reason: 'root_identity_changed' }
  await stopPinned(root)
  for (const pinned of members) {
    if (pinned.pid !== root.pid) await stopPinned(pinned, true)
  }
  let remaining = groupMembers(root.pid)
  for (let attempt = 0; attempt < 30 && remaining?.length; attempt += 1) {
    await pause(100)
    remaining = groupMembers(root.pid)
  }
  const pinnedPids = new Set(members.map((member) => member.pid))
  return remaining?.length === 0
    ? { verified: true, reason: '' }
    : { verified: false, reason: remaining
      ? remaining.some((pid) => !pinnedPids.has(pid))
        ? 'group_member_not_pinned_before_root_exit' : 'group_process_residual'
      : 'group_recheck_unavailable' }
}

export async function runBoundedDiagnosticChild({ command, args, cwd, env, timeoutMs,
  maxBuffer, allowedResidualCommand }) {
  if (process.platform !== 'darwin' || !Number.isSafeInteger(timeoutMs) || timeoutMs < 1_000 ||
      !Number.isSafeInteger(maxBuffer) || maxBuffer < 1_024 || maxBuffer > 64 * 1024 * 1024 ||
      typeof allowedResidualCommand !== 'function' || !Array.isArray(args) ||
      typeof args[0] !== 'string') {
    throw new Error('diagnostic_child_bound_invalid')
  }
  const child = spawn(command, args, { cwd, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  let closed = false
  let stdout = ''
  let stderr = ''
  let overBuffer = false
  let spawnError = null
  let resolveOverflow
  const overflow = new Promise((resolve) => { resolveOverflow = resolve })
  const accept = (field, chunk) => {
    const previous = field === 'stdout' ? stdout : stderr
    if (Buffer.byteLength(previous) + chunk.length > maxBuffer) {
      overBuffer = true
      resolveOverflow({ overflow: true })
      return
    }
    if (field === 'stdout') stdout += chunk.toString('utf8')
    else stderr += chunk.toString('utf8')
  }
  child.stdout.on('data', (chunk) => accept('stdout', chunk))
  child.stderr.on('data', (chunk) => accept('stderr', chunk))
  child.on('error', (error) => { spawnError = error })
  const completion = new Promise((resolve) => child.once('close', (code, signal) => {
    closed = true
    resolve({ code, signal })
  }))
  const pinned = await pinChild(child, command, args[0], () => closed)
  let timer
  const limit = new Promise((resolve) => {
    timer = setTimeout(() => resolve({ timeout: true }), timeoutMs)
  })
  const result = await Promise.race([completion, limit, overflow])
  clearTimeout(timer)
  if (!result.timeout && !overBuffer && !spawnError) {
    return { status: result.code, signal: result.signal, stdout, stderr, cleanup: null }
  }
  const cleanup = await stopOwnedGroup(pinned, allowedResidualCommand)
  const reason = result.overflow || overBuffer ? 'diagnostic_child_output_limit'
    : result.timeout ? 'diagnostic_child_timeout' : 'diagnostic_child_spawn_failed'
  const error = new Error(reason)
  error.cleanup = cleanup
  throw error
}
