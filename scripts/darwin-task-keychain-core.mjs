// Shared explicit task-Keychain filesystem and PTY operations, extracted from
// the validated B1 provisioning path. Importing performs no host operation.
// Callers retain their own exact scope admission; no default/search-list edits.
import { createHash } from 'node:crypto'
import { chmodSync, closeSync, constants, fsyncSync, fstatSync, linkSync, lstatSync, mkdirSync, openSync,
  readdirSync, realpathSync, unlinkSync } from 'node:fs'
import { execFileSync, spawn } from 'node:child_process'
import { basename, dirname, join, resolve } from 'node:path'

export const sha256 = (value) => createHash('sha256').update(value).digest('hex')
export function requireB1(condition, code) {
  if (!condition) throw new Error(code)
}
export function exactRootName(root) {
  return typeof root === 'string' && /^\/[A-Za-z0-9._/-]+$/.test(root) &&
    !/login\.keychain/iu.test(root) && resolve(root) === root && root !== '/'
}
function identityError(code, differences) {
  return Object.assign(new Error(code), { identityDifferences: Object.freeze(differences) })
}
export function assertSameIdentity(actual, expected, code) {
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
export function keychainExpectProgram(operation, database, passwordFormat = 'synthetic-hex') {
  const root = dirname(dirname(database))
  requireB1(['create-keychain', 'unlock-keychain'].includes(operation) &&
    exactRootName(root) && database === (operation === 'create-keychain' ? keychainPaths(root).creationDatabase : keychainPaths(root).database),
  'b1_keychain_operation_invalid')
  const pathHex = Buffer.from(database).toString('hex')
  requireB1(['synthetic-hex', 'user-password'].includes(passwordFormat), 'b1_keychain_operation_invalid')
  return [
    'set timeout 20', 'log_user 0', 'set secret [gets stdin]',
    passwordFormat === 'synthetic-hex'
      ? 'if {[string length $secret] != 64 || ![regexp {^[0-9a-f]+$} $secret]} { exit 126 }'
      : 'if {[string length $secret] < 1 || [string length $secret] > 1024} { exit 126 }',
    `set keychain [encoding convertfrom utf-8 [binary format H* {${pathHex}}]]`,
    'proc stop_owned_security {} { global secret; set secret {}; catch {exec /bin/kill -KILL -- [exp_pid]}; catch {close}; catch {wait}; exit 124 }',
    'trap {stop_owned_security} {TERM INT}',
    'set prompts 0', `spawn -noecho /usr/bin/security ${operation} -- $keychain`,
    'expect {', '  -re {(?i)password[^\\r\\n]*:} { incr prompts; send -- "$secret\\r"; exp_continue }',
    '  eof {}', '  timeout { stop_owned_security }', '}', 'set secret {}', 'set waited [wait]',
    'if {$prompts < 1} { exit 123 }', 'set code [lindex $waited 3]',
    'if {![string is integer -strict $code]} { exit 125 }', 'exit $code', ''
  ].join('\n')
}

export async function privatePTY(operation, database, root, password, passwordFormat = 'synthetic-hex') {
  requireB1(Buffer.isBuffer(password) && password.length > 0 && password.length <= 4096 &&
    !password.includes(10) && !password.includes(13) && !password.includes(0), 'b1_keychain_password_invalid')
  const previousMask = process.umask(0o077)
  let child
  try { child = spawn('/usr/bin/expect', ['-c', keychainExpectProgram(operation, database, passwordFormat)], {
    cwd: root, detached: true,
    env: { PATH: '/usr/bin:/bin:/usr/sbin:/sbin', LANG: 'C', LC_ALL: 'C' },
    stdio: ['pipe', 'ignore', 'ignore']
  }) } finally { process.umask(previousMask) }
  await new Promise((resolveOperation, reject) => {
    let expired = false
    let hardStop
    const timer = setTimeout(() => {
      expired = true
      // Let the PTY owner terminate and wait for its exact spawned Security
      // child first: that child may have its own terminal process group.
      child.kill('SIGTERM')
      hardStop = setTimeout(() => {
        if (child.pid) { try { process.kill(-child.pid, 'SIGKILL') } catch {} }
      }, 1_000)
    }, 22_000)
    child.once('error', () => { clearTimeout(timer); clearTimeout(hardStop); reject(new Error('b1_keychain_pty_failed')) })
    child.once('close', (code, signal) => {
      clearTimeout(timer)
      clearTimeout(hardStop)
      if (expired || code !== 0 || signal) reject(new Error('b1_keychain_pty_failed'))
      else resolveOperation()
    })
    child.stdin.on('error', () => {})
    child.stdin.write(password)
    child.stdin.end(Buffer.from('\n'))
  }).finally(() => {
    // Success, failed password and timeout all retain database evidence and
    // require the task's native file handles to have drained.
    assertNoKeychainHandles(dirname(database))
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
export const migrationIO = Object.freeze({
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
