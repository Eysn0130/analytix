// Owner-run adapter for this one synthetic process denominator. Importing this
// module performs no provisioning and never selects the default Keychain.
import { createHash, randomBytes } from 'node:crypto'
import { chmodSync, closeSync, constants, fsyncSync, fstatSync, linkSync, lstatSync, mkdirSync, openSync,
  readdirSync, realpathSync, unlinkSync } from 'node:fs'
import { execFileSync, spawn } from 'node:child_process'
import { basename, dirname, join, resolve } from 'node:path'

export const sha256 = (value) => createHash('sha256').update(value).digest('hex')
export const b1ProcessEvidenceRoot = '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6'
export const b1ProcessHostParent = '/Users/sun/.codex/instruction-maintenance/2026-09-07-resume/own2-b1-host-private-roots'
const rootPattern = /^analytix-own2-b1-process-[A-Za-z0-9]{6}$/u
export function requireB1(condition, code) {
  if (!condition) throw new Error(code)
}
export function exactRootName(root) {
  return typeof root === 'string' && !/login\.keychain/iu.test(root) && dirname(root) === b1ProcessHostParent &&
    rootPattern.test(basename(root)) && resolve(root) === root
}
function identityError(code, differences) {
  return Object.assign(new Error(code), { identityDifferences: Object.freeze(differences) })
}
function assertSameIdentity(actual, expected, code) {
  const differences = Object.fromEntries(['path', 'device', 'inode', 'owner', 'mode', 'links']
    .map((key) => [key + 'Changed', actual[key] !== expected[key]]))
  if (Object.values(differences).some(Boolean)) throw identityError(code, differences)
}
export function identity(path, directory = true) {
  return inspectIdentity(path, directory, 1n)
}
function inspectIdentity(path, directory, links) {
  const s = lstatSync(path, { bigint: true })
  const differences = { canonicalChanged: realpathSync(path) !== path || /login\.keychain/iu.test(path),
    ownerChanged: s.uid !== BigInt(process.getuid()),
    kindChanged: s.isSymbolicLink() || (directory ? !s.isDirectory() : !s.isFile()),
    modeChanged: (s.mode & 0o7777n) !== (directory ? 0o700n : 0o600n),
    linksChanged: !directory && s.nlink !== links, sizeInvalid: !directory && s.size <= 0n }
  if (Object.values(differences).some(Boolean)) throw identityError('b1_private_identity_invalid', differences)
  return { path, device: String(s.dev), inode: String(s.ino), owner: String(s.uid),
    mode: (s.mode & 0o777n).toString(8), links: directory ? '0' : String(s.nlink) }
}
export function pinRoot(root) {
  requireB1(exactRootName(root), 'b1_private_root_invalid')
  const pinned = identity(root)
  return () => requireB1(JSON.stringify(identity(root)) === JSON.stringify(pinned), 'b1_private_root_drift')
}
export function keychainPaths(root) {
  requireB1(exactRootName(root), 'b1_private_root_invalid')
  const parent = join(root, 'darwin-secret-store-keychain')
  // Apple Security db15acbe, AtomicFile.cpp: local lock names hash the basename.
  // SHA-1 is only the upstream filename convention, never an authority digest.
  const lock = (name) => join(parent, '.fl' + createHash('sha1').update(name).digest('hex').slice(0, 8).toUpperCase())
  return { parent, creationDatabase: join(parent, 'bootstrap.keychain-db'), database: join(parent, 'analytix-task.keychain-db'),
    creationLock: lock('bootstrap.keychain-db'), databaseLock: lock('analytix-task.keychain-db') }
}
const lockMetadataIO = Object.freeze({ stat: (path) => lstatSync(path, { bigint: true }), realpath: realpathSync })
export function keychainLockIdentity(path, io = lockMetadataIO) {
  const paths = keychainPaths(dirname(dirname(path)))
  const s = io.stat(path)
  // LocalFileLocker creates 0444 under the Owner's required umask 077, takes
  // flock on the empty inode and closes it on unlock without unlinking it.
  requireB1([paths.creationLock, paths.databaseLock].includes(path) && io.realpath(path) === path &&
    s.uid === BigInt(process.getuid()) && s.isFile() && !s.isSymbolicLink() &&
    (s.mode & 0o7777n) === 0o400n && s.nlink === 1n && s.size === 0n, 'b1_keychain_lock_identity_invalid')
  return { path, device: String(s.dev), inode: String(s.ino), owner: String(s.uid), mode: '400', links: '1', size: '0' }
}
export function bindingFromIdentity(root, userDataDir, dataDir, dbIdentity) {
  const paths = keychainPaths(root)
  requireB1(userDataDir === join(root, 'user-data') && dataDir === join(root, 'sidecar', 'data') &&
    dbIdentity.path === paths.database && dbIdentity.mode === '600' && dbIdentity.links === '1',
  'b1_keychain_binding_invalid')
  const security = { schemaVersion: 1, pathDigest: sha256(paths.database), device: dbIdentity.device,
    inode: dbIdentity.inode, owner: dbIdentity.owner, mode: dbIdentity.mode, links: dbIdentity.links }
  const binding = { schemaVersion: 1, purpose: 'analytix.runtime-darwin-secret-store-keychain-binding/v1',
    externalStateMode: 'isolated-local-v1', isolationRoot: root, userDataDir, dataDir,
    keychainDBPath: paths.database, keychainSecurityDigest: sha256(JSON.stringify(security)) }
  return { ...binding, bindingDigest: sha256(JSON.stringify(binding)) }
}
export function keychainExpectProgram(operation, database) {
  const root = dirname(dirname(database))
  requireB1(['create-keychain', 'unlock-keychain'].includes(operation) &&
    exactRootName(root) && database === (operation === 'create-keychain' ? keychainPaths(root).creationDatabase : keychainPaths(root).database),
  'b1_keychain_operation_invalid')
  const pathHex = Buffer.from(database).toString('hex')
  return [
    'set timeout 20', 'log_user 0', 'set secret [gets stdin]',
    'if {[string length $secret] != 64 || ![regexp {^[0-9a-f]+$} $secret]} { exit 126 }',
    `set keychain [encoding convertfrom utf-8 [binary format H* {${pathHex}}]]`,
    'set prompts 0', `spawn -noecho /usr/bin/security ${operation} -- $keychain`,
    'expect {', '  -re {(?i)password[^\\r\\n]*:} { incr prompts; send -- "$secret\\r"; exp_continue }',
    '  eof {}', '  timeout { exit 124 }', '}', 'set secret {}', 'set waited [wait]',
    'if {$prompts < 1} { exit 123 }', 'set code [lindex $waited 3]',
    'if {![string is integer -strict $code]} { exit 125 }', 'exit $code', ''
  ].join('\n')
}

async function privatePTY(operation, database, root, password) {
  const child = spawn('/usr/bin/expect', ['-c', keychainExpectProgram(operation, database)], {
    cwd: root, detached: true,
    env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin', LANG: 'C', LC_ALL: 'C' },
    stdio: ['pipe', 'ignore', 'ignore']
  })
  await new Promise((resolveOperation, reject) => {
    let expired = false
    const timer = setTimeout(() => {
      expired = true
      // Only the new process group created by this operation is eligible.
      if (child.pid) { try { process.kill(-child.pid, 'SIGKILL') } catch {} }
    }, 22_000)
    child.once('error', () => { clearTimeout(timer); reject(new Error('b1_keychain_pty_failed')) })
    child.once('close', (code, signal) => {
      clearTimeout(timer)
      if (expired || code !== 0 || signal) reject(new Error('b1_keychain_pty_failed'))
      else resolveOperation()
    })
    child.stdin.on('error', () => {})
    child.stdin.write(password)
    child.stdin.end(Buffer.from('\n'))
  })
}

function entryAbsent(path) {
  try { lstatSync(path); return false } catch (error) {
    if (error.code === 'ENOENT') return true
    throw error
  }
}
function assertNoKeychainHandles(parent) {
  let output
  try { output = execFileSync('/usr/sbin/lsof', ['-nP', '-Ff', '+D', parent], {
    env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin', LANG: 'C', LC_ALL: 'C' },
    encoding: 'utf8', timeout: 15000, maxBuffer: 256 * 1024
  }) } catch (error) {
    requireB1(error.status === 1 && !String(error.stderr || '').trim(), 'b1_keychain_handle_probe_failed')
    output = String(error.stdout || '')
  }
  requireB1(!output.split('\n').some((line) => /^f\d+/u.test(line)), 'b1_keychain_creation_handles_remain')
}
function syncPrivateDirectory(path) {
  const fd = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW)
  try {
    const state = fstatSync(fd)
    requireB1(state.isDirectory() && state.uid === process.getuid() && (state.mode & 0o7777) === 0o700,
      'b1_keychain_parent_drift')
    fsyncSync(fd)
  } finally { closeSync(fd) }
}
const migrationIO = Object.freeze({
  identity, lockIdentity: keychainLockIdentity, linkedIdentity: (path) => inspectIdentity(path, false, 2n), entryAbsent,
  names: readdirSync, assertNoHandles: assertNoKeychainHandles,
  linkExclusive: linkSync, unlink: unlinkSync, syncDirectory: syncPrivateDirectory
})

// Fresh creation uses a non-login name to avoid Apple's creation-time search
// list registration. After its PTY exits and all file handles close, publish
// the same inode at the required binding path without overwriting any entry.
// Keep the exact old-basename lock inode: unlinking or moving it could split
// Apple's lock domain. Only the database name changes; final cleanup owns the
// retained companion after all task writers drain and handle checks pass.
// The injectable IO is solely for deterministic tests of this harness; the
// Owner-run entry point always uses the fixed filesystem implementation.
export function migrateCreatedKeychain(root, created, io = migrationIO) {
  const paths = keychainPaths(root)
  const rootIdentity = io.identity(root)
  const parentIdentity = io.identity(paths.parent)
  const creationLock = io.lockIdentity(paths.creationLock)
  const same = (a, b) => JSON.stringify(a) === JSON.stringify(b)
  const checkParents = () => requireB1(same(io.identity(root), rootIdentity) &&
    same(io.identity(paths.parent), parentIdentity), 'b1_keychain_parent_drift')
  const checkEntries = (databases) => {
    checkParents()
    requireB1(same(io.lockIdentity(paths.creationLock), creationLock) &&
      same([...io.names(paths.parent)].sort(), [...databases, paths.creationLock].map((path) => basename(path)).sort()),
    'b1_keychain_migration_preflight_failed')
  }
  requireB1(created.path === paths.creationDatabase && created.mode === '600' && created.links === '1', 'b1_keychain_creation_identity_invalid')
  checkEntries([paths.creationDatabase])
  requireB1(same(io.identity(paths.creationDatabase, false), created) && io.entryAbsent(paths.database), 'b1_keychain_migration_preflight_failed')
  io.assertNoHandles(paths.parent)
  checkEntries([paths.creationDatabase])
  requireB1(same(io.identity(paths.creationDatabase, false), created), 'b1_keychain_creation_drift')
  io.linkExclusive(paths.creationDatabase, paths.database)
  // The only two links must be the exact source/destination names we own.
  checkEntries([paths.creationDatabase, paths.database])
  requireB1(same(io.linkedIdentity(paths.creationDatabase), { ...created, links: '2' }) &&
    same(io.linkedIdentity(paths.database), { ...created, path: paths.database, links: '2' }), 'b1_keychain_migration_identity_failed')
  io.syncDirectory(paths.parent)
  io.assertNoHandles(paths.parent)
  checkEntries([paths.creationDatabase, paths.database])
  requireB1(same(io.linkedIdentity(paths.creationDatabase), { ...created, links: '2' }) &&
    same(io.linkedIdentity(paths.database), { ...created, path: paths.database, links: '2' }), 'b1_keychain_migration_identity_failed')
  io.unlink(paths.creationDatabase)
  io.syncDirectory(paths.parent)
  io.assertNoHandles(paths.parent)
  checkEntries([paths.database])
  const final = io.identity(paths.database, false)
  requireB1(io.entryAbsent(paths.creationDatabase) &&
    same(final, { ...created, path: paths.database }), 'b1_keychain_migration_incomplete')
  return final
}

export function pinMigratedKeychainLocks(root, creationLock, io = migrationIO) {
  const paths = keychainPaths(root)
  let databaseLock
  return (allowFinalCreation = false) => {
    requireB1(JSON.stringify(io.lockIdentity(paths.creationLock)) === JSON.stringify(creationLock), 'b1_keychain_lock_drift')
    const names = [basename(paths.database), basename(paths.creationLock)]
    if (!io.entryAbsent(paths.databaseLock)) {
      const current = io.lockIdentity(paths.databaseLock)
      if (!databaseLock) {
        requireB1(allowFinalCreation, 'b1_keychain_unowned_lock_creation')
        databaseLock = current
      }
      requireB1(JSON.stringify(current) === JSON.stringify(databaseLock), 'b1_keychain_lock_drift')
      names.push(basename(paths.databaseLock))
    } else requireB1(!databaseLock, 'b1_keychain_lock_drift')
    requireB1(JSON.stringify([...io.names(paths.parent)].sort()) === JSON.stringify(names.sort()), 'b1_keychain_entries_invalid')
  }
}

// Only a verified protected write can advance the database inode. Unlock,
// idle and restart checks retain the committed identity. Unlock success alone
// establishes neither a password readback nor authority to replace the inode.
// Runtime credential creation additionally requires production Registry probe
// readback through its protected Store, not a caller-supplied success boolean.
export function keychainLifecycle(root, originalDB, originalLock, operations, io = migrationIO) {
  const paths = keychainPaths(root)
  const rootIdentity = io.identity(root), parentIdentity = io.identity(paths.parent)
  const checkLocks = pinMigratedKeychainLocks(root, originalLock, io)
  let database = { ...originalDB }, phase = 'idle', pid, generation = 0, verifiedGeneration = 0, writeUsed = false
  const seenPIDs = new Set()
  const checkParents = () => {
    assertSameIdentity(io.identity(root), rootIdentity, 'b1_keychain_root_drift')
    assertSameIdentity(io.identity(paths.parent), parentIdentity, 'b1_keychain_parent_drift')
  }
  const guard = (allowFinalCreation = false) => {
    checkParents(); checkLocks(allowFinalCreation); operations.assertSecurityUnchanged()
    io.assertNoHandles(paths.parent)
    checkParents(); checkLocks(); operations.assertSecurityUnchanged()
  }
  const current = () => io.identity(paths.database, false)
  const unchanged = (code) => assertSameIdentity(current(), database, code)
  const commit = (candidate, operation) => {
    assertSameIdentity({ ...candidate, inode: database.inode }, database, 'b1_keychain_transition_identity_invalid')
    const result = { operation, inodeChanged: candidate.inode !== database.inode, stableReadback: true }
    database = { ...candidate }
    return result
  }
  const runtime = (runtimePID) => requireB1(phase === 'runtime' && pid === runtimePID, 'b1_keychain_runtime_state_invalid')
  guard(); unchanged('b1_keychain_initial_identity_drift')
  return {
    async unlock() {
      requireB1(phase === 'idle', 'b1_keychain_unlock_state_invalid')
      try {
        guard(); unchanged('b1_keychain_unlock_precondition_drift'); phase = 'unlock'
        await operations.unlock()
        guard(true)
        unchanged('b1_keychain_unlock_identity_drift')
        guard()
        unchanged('b1_keychain_unlock_readback_drift')
        phase = 'launch_ready'
        return { operation: 'owned_unlock', inodeChanged: false, stableReadback: true }
      } catch (error) { phase = 'failed'; throw error }
    },
    binding(userDataDir, dataDir) {
      requireB1(phase === 'launch_ready', 'b1_keychain_binding_state_invalid')
      guard(); unchanged('b1_keychain_launch_identity_drift')
      io.identity(userDataDir); io.identity(dataDir)
      return bindingFromIdentity(root, userDataDir, dataDir, database)
    },
    runtimeStarted(runtimePID) {
      requireB1(phase === 'launch_ready' && generation < 2 && Number.isSafeInteger(runtimePID) && runtimePID > 0 &&
        !seenPIDs.has(runtimePID), 'b1_keychain_runtime_state_invalid')
      guard(); unchanged('b1_keychain_launch_identity_drift')
      pid = runtimePID; seenPIDs.add(pid); generation += 1; phase = 'runtime'
    },
    beginProviderWrite(runtimePID) {
      runtime(runtimePID)
      requireB1(generation === 1 && !writeUsed, 'b1_keychain_provider_write_state_invalid')
      guard(); unchanged('b1_keychain_provider_write_precondition_drift')
      writeUsed = true; phase = 'provider_write'
    },
    async verifyProvider(runtimePID, probe) {
      requireB1(pid === runtimePID && (phase === 'provider_write' || (phase === 'runtime' && generation === 2)) &&
        typeof probe === 'function', 'b1_keychain_provider_readback_state_invalid')
      const allowTransition = phase === 'provider_write'
      try {
        phase = 'provider_readback'; guard(allowTransition)
        const candidate = current()
        if (!allowTransition) unchanged('b1_keychain_restart_identity_drift')
        requireB1(await probe() === true, 'b1_keychain_provider_readback_failed')
        guard()
        assertSameIdentity(current(), candidate, 'b1_keychain_provider_readback_drift')
        const result = commit(candidate, allowTransition ? 'verified_provider_write' : 'verified_provider_readback')
        verifiedGeneration = generation; phase = 'runtime'; return result
      } catch (error) { phase = 'failed'; throw error }
    },
    assertRuntimeCurrent(runtimePID) {
      runtime(runtimePID)
      requireB1(verifiedGeneration === generation, 'b1_keychain_provider_readback_required')
      guard(); unchanged('b1_keychain_runtime_identity_drift')
    },
    runtimeStopped(runtimePID) {
      this.assertRuntimeCurrent(runtimePID)
      pid = undefined; phase = 'idle'
    },
    assertCleanupReady() {
      requireB1(phase === 'idle' && generation === 2 && verifiedGeneration === 2, 'b1_keychain_cleanup_state_invalid')
      guard(); unchanged('b1_keychain_cleanup_identity_drift')
    },
    assertProvisioningCleanupReady() {
      requireB1(generation === 0 && (phase === 'idle' || phase === 'launch_ready'), 'b1_keychain_provisioning_cleanup_state_invalid')
      guard(); unchanged('b1_keychain_cleanup_identity_drift')
    },
    dispose() { phase = 'disposed' }
  }
}

export async function createB1ExplicitKeychain(root, assertSecurityUnchanged) {
  requireB1(process.platform === 'darwin', 'b1_darwin_required')
  requireB1(typeof assertSecurityUnchanged === 'function', 'b1_keychain_configuration_guard_required')
  const checkRoot = pinRoot(root)
  const paths = keychainPaths(root)
  checkRoot()
  mkdirSync(paths.parent, { mode: 0o700 }) // exclusive: never repair existing state
  chmodSync(paths.parent, 0o700)
  const parentIdentity = identity(paths.parent)
  const password = Buffer.from(randomBytes(32).toString('hex'))
  try {
    checkRoot()
    assertSameIdentity(identity(paths.parent), parentIdentity, 'b1_keychain_parent_drift')
    assertSecurityUnchanged()
    await privatePTY('create-keychain', paths.creationDatabase, root, password)
    checkRoot()
    requireB1(JSON.stringify(identity(paths.parent)) === JSON.stringify(parentIdentity), 'b1_keychain_parent_drift')
    assertSecurityUnchanged()
    const originalLock = keychainLockIdentity(paths.creationLock)
    const originalDB = migrateCreatedKeychain(root, identity(paths.creationDatabase, false))
    const lifecycle = keychainLifecycle(root, originalDB, originalLock, { assertSecurityUnchanged,
      unlock: () => privatePTY('unlock-keychain', paths.database, root, password) })
    return {
      ...lifecycle,
      dispose() { password.fill(0); lifecycle.dispose() }
    }
  } catch (error) { password.fill(0); throw error }
}
