// Owner-run adapter for the original B1 synthetic process denominator.
// Shared provisioning does not widen this adapter's exact historical scope.
import { randomBytes } from 'node:crypto'
import { chmodSync, mkdirSync } from 'node:fs'
import { basename, dirname, join, resolve } from 'node:path'
import { sha256, requireB1, identity, assertSameIdentity, privatePTY, migrationIO,
  pinRoot as sharedPinRoot, keychainPaths as sharedPaths,
  keychainLockIdentity as sharedLockIdentity, keychainExpectProgram as sharedExpect,
  migrateCreatedKeychain as sharedMigrate, pinMigratedKeychainLocks as sharedPinLocks
} from './darwin-task-keychain-core.mjs'
export { sha256, requireB1, identity }
export const b1ProcessEvidenceRoot = '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6'
export const b1ProcessHostParent = '/Users/sun/.codex/instruction-maintenance/2026-09-07-resume/own2-b1-host-private-roots'
const rootPattern = /^analytix-own2-b1-process-[A-Za-z0-9]{6}$/u
export function exactRootName(root) {
  return typeof root === 'string' && !/login\.keychain/iu.test(root) && dirname(root) === b1ProcessHostParent &&
    rootPattern.test(basename(root)) && resolve(root) === root
}
function admitRoot(root) { requireB1(exactRootName(root), 'b1_private_root_invalid') }
export function pinRoot(root) { admitRoot(root); return sharedPinRoot(root) }
export function keychainPaths(root) { admitRoot(root); return sharedPaths(root) }
export function keychainLockIdentity(path, io) { admitRoot(dirname(dirname(path))); return sharedLockIdentity(path, io) }
export function keychainExpectProgram(operation, database) {
  requireB1(exactRootName(dirname(dirname(database))), 'b1_keychain_operation_invalid')
  return sharedExpect(operation, database)
}
export function migrateCreatedKeychain(root, created, io) { admitRoot(root); return sharedMigrate(root, created, io) }
export function pinMigratedKeychainLocks(root, lock, io) { admitRoot(root); return sharedPinLocks(root, lock, io) }

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
