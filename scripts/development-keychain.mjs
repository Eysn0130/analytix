import { execFileSync } from 'node:child_process'
import { mkdirSync, lstatSync, openSync, fstatSync, readSync, closeSync, constants } from 'node:fs'
import { join } from 'node:path'
import { createHash } from 'node:crypto'
import { identity, keychainPaths, keychainLockIdentity, migrateCreatedKeychain, migrationIO,
  pinMigratedKeychainLocks, pinRoot, privatePTY } from './darwin-task-keychain-core.mjs'

const systemEnv = { PATH: '/usr/bin:/bin:/usr/sbin:/sbin', LANG: 'C', LC_ALL: 'C' }

function configurationSnapshot() {
  // Metadata only, held privately for equality. Never print global paths or
  // enumerate/read any Keychain item, including an existing Provider key.
  return ['default-keychain', 'list-keychains'].map(operation =>
    execFileSync('/usr/bin/security', [operation], { env: systemEnv, timeout: 5000, stdio: ['ignore', 'pipe', 'ignore'] }))
}

export function promptDevelopmentKeychainPassword() {
  let result
  try {
    result = execFileSync('/usr/bin/osascript', ['-e',
      'text returned of (display dialog "Enter the password for this isolated Analytix development Keychain. Keep it for later unlocks. This does not use your login Keychain." default answer "" with hidden answer buttons {"Cancel", "Continue"} default button "Continue")'
    ], { env: systemEnv, stdio: ['ignore', 'pipe', 'ignore'], maxBuffer: 4096 })
    const password = Buffer.from(result.subarray(0, result.length - (result.at(-1) === 10 ? 1 : 0)))
    if (password.length === 0 || password.includes(10) || password.includes(13) || password.includes(0)) {
      password.fill(0)
      throw new Error('invalid')
    }
    return password
  } catch {
    throw new Error('Isolated Keychain setup was cancelled or no valid password was supplied.')
  } finally { result?.fill(0) }
}

export async function prepareDevelopmentKeychain(profile, password, { create = false } = {}) {
  if (process.platform !== 'darwin') throw new Error('Development Keychain requires macOS.')
  const root = profile.taskRoot
  const checkRoot = pinRoot(root)
  const paths = keychainPaths(root)
  if (!create) assertCommittedDevelopmentKeychainIdentity(profile)
  const baseline = configurationSnapshot()
  const checkConfiguration = () => {
    const current = configurationSnapshot()
    try {
      if (!current.every((value, index) => value.equals(baseline[index]))) {
        throw new Error('System Keychain configuration changed; isolated setup stopped.')
      }
    } finally { current.forEach(value => value.fill(0)) }
  }
  try {
    checkRoot()
    if (create) {
      // Exclusive creation and inode-preserving publication reuse the B1
      // bootstrap mechanism. No replacement, login-name creation or repair.
      mkdirSync(paths.parent, { mode: 0o700 })
      const parent = identity(paths.parent)
      await privatePTY('create-keychain', paths.creationDatabase, root, password, 'user-password')
      checkRoot(); checkConfiguration()
      if (JSON.stringify(identity(paths.parent)) !== JSON.stringify(parent)) throw new Error('Keychain parent changed.')
      migrateCreatedKeychain(root, identity(paths.creationDatabase, false))
    }
    const originalParent = identity(paths.parent)
    const original = identity(paths.database, false)
    const checkLocks = pinMigratedKeychainLocks(root, keychainLockIdentity(paths.creationLock))
    checkLocks(true)
    migrationIO.assertNoHandles(paths.parent)
    await privatePTY('unlock-keychain', paths.database, root, password, 'user-password')
    checkRoot(); checkConfiguration()
    checkLocks(true)
    migrationIO.assertNoHandles(paths.parent)
    checkLocks()
    if (JSON.stringify(identity(paths.parent)) !== JSON.stringify(originalParent) ||
      JSON.stringify(identity(paths.database, false)) !== JSON.stringify(original)) {
      throw new Error('Keychain identity changed during unlock; launch refused.')
    }
    return { created: create, unlocked: true }
  } finally {
    try { checkConfiguration() } finally { baseline.forEach(value => value.fill(0)) }
  }
}

// Read the Core-owned committed record before unlock/launch. This consumer never
// creates, upgrades or repairs authority. Core repeats complete admission.
export function assertCommittedDevelopmentKeychainIdentity(profile, { allowCoreRecovery = false } = {}) {
  const secretRoot = join(profile.homeRoot, '.analytix', 'data', 'private', 'provider-secrets')
  const masterRoot = join(secretRoot, 'master-key')
  const exists = path => { try { lstatSync(path); return true } catch (error) { if (error.code === 'ENOENT') return false; throw error } }
  if (!exists(masterRoot)) {
    if (exists(secretRoot) || exists(join(profile.homeRoot, '.analytix', 'data', 'private', 'provider-registry'))) {
      throw new Error('Committed Keychain identity is missing; state preserved.')
    }
    return
  }
  identity(masterRoot)
  const path = join(masterRoot, 'explicit-task-keychain-binding.v1')
  let descriptor
  let bytes
  try {
    for (const suffix of ['commit-journal', 'previous', 'committed', 'rollback-required']) {
      if (!allowCoreRecovery && exists(join(masterRoot, '.explicit-task-keychain-binding.v1.' + suffix))) throw new Error('unsettled')
    }
    const before = lstatSync(path)
    identity(path, false)
    // A replacement between lstat and open must not block on a FIFO. Bound
    // reads even if the file grows after fstat, then recheck the opened inode.
    descriptor = openSync(path, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK)
    const stat = fstatSync(descriptor)
    const same = other => ['dev', 'ino', 'mode', 'uid', 'gid', 'nlink', 'size', 'mtimeMs', 'ctimeMs'].every(key => stat[key] === other[key])
    if (!stat.isFile() || !same(before) || stat.size > 2048 || stat.nlink !== 1 || stat.uid !== process.getuid() || (stat.mode & 0o7777) !== 0o600) throw new Error('invalid')
    bytes = Buffer.alloc(2049)
    let length = 0
    while (length < bytes.length) {
      const count = readSync(descriptor, bytes, length, bytes.length - length, null)
      if (count === 0) break
      length += count
    }
    if (length !== stat.size || !same(fstatSync(descriptor)) || !same(lstatSync(path))) throw new Error('invalid')
    const content = bytes.subarray(0, length).toString('utf8')
    const record = JSON.parse(content)
    const current = identity(keychainPaths(profile.taskRoot).database, false)
    const expected = { schemaVersion: 1, pathDigest: createHash('sha256').update(current.path).digest('hex'),
      device: current.device, inode: current.inode, owner: current.owner, mode: current.mode, links: current.links }
    if (record.schemaVersion !== 2 || !/^[a-f0-9]{64}$/.test(record.authorityDigest) ||
      Object.keys(record).sort().join(',') !== 'authorityDigest,schemaVersion,security' ||
      JSON.stringify(record.security) !== JSON.stringify(expected) ||
      content !== JSON.stringify({ schemaVersion: 2, authorityDigest: record.authorityDigest, security: expected })) throw new Error('invalid')
  } catch {
    throw new Error('Committed Keychain identity is missing, unsupported or changed; state preserved and unlock/launch refused.')
  } finally { bytes?.fill(0); if (descriptor !== undefined) closeSync(descriptor) }
}
