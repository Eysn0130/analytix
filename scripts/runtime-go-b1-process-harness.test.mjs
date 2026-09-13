import assert from 'node:assert/strict'
import test, { mock } from 'node:test'
import fs from 'node:fs'
import childProcess from 'node:child_process'
import { syncBuiltinESMExports } from 'node:module'
import { createK10DarwinExplicitTaskKeychain, k10DarwinTaskKeychainPaths } from './k10-darwin-explicit-keychain.mjs'
import { b1ProcessEvidenceRoot, b1ProcessHostParent, bindingFromIdentity, exactRootName, keychainExpectProgram, keychainLifecycle, keychainLockIdentity, keychainPaths, migrateCreatedKeychain, pinMigratedKeychainLocks, sha256 } from './runtime-go-b1-process-keychain.mjs'
import { assertB1ProcessRootAncestors, assertManifestShape, createExactCleanup, ownedProcesses, privateFrame, privateOutputGuard, publicHarnessFailure, publicProcessObservation, nativeFailureCollector, stopOwned, takeVerifiedNativeFailures } from './runtime-go-b1-process-harness.mjs'

const root = b1ProcessHostParent + '/analytix-own2-b1-process-A1b2C3'
const db = { path: keychainPaths(root).database, device: '12', inode: '34', owner: '501', mode: '600', links: '1' }
const binding = () => bindingFromIdentity(root, root + '/user-data', root + '/sidecar/data', db)

const nativeMarker = '[analytix] event=ANALYTIX_FUNDS_CSV_NATIVE_FAILURE_V1 '
test('native marker collector is bounded, exact, and preserves unknown as unknown', () => {
  const collector = nativeFailureCollector()
  const line = nativeMarker + 'layer=RUNNER stage=SESSION_OPEN class=REGISTRY\n'
  for (const byte of Buffer.from(line)) collector.observe(Buffer.from([byte]))
  collector.observe(Buffer.from(nativeMarker + 'layer=ADMISSION stage=UNKNOWN class=UNKNOWN\n'))
  collector.finish()
  assert.equal(collector.invalid, false)
  assert.deepEqual(collector.take(), [{ layer: 'RUNNER', stage: 'SESSION_OPEN', class: 'REGISTRY' },
    { layer: 'ADMISSION', stage: 'UNKNOWN', class: 'UNKNOWN' }])
  assert.deepEqual(collector.take(), [])
  for (const bad of [
    'layer=RUNNER stage=SESSION_OPEN class=REGISTRY private=canary\n',
    'layer=RUNNER stage=PRIVATE_CANARY class=REGISTRY\n',
    'layer=RUNNER stage=SESSION_OPEN class=PRIVATE_CANARY\n',
    'layer=RUNNER stage=ARGUMENTS class=REGISTRY\n',
    'layer=RUNNER stage=SESSION_OPEN class=REGISTRY class=PROTOCOL\n',
    'layer=RUNNER stage=SESSION_OPEN class=REGISTRY\r\n',
    'layer=RUNNER stage=SESSION_OPEN class=REGISTRY',
    'private-canary'.repeat(100) + '\n'
  ]) {
    const invalid = nativeFailureCollector()
    invalid.observe(Buffer.from(nativeMarker + bad)); invalid.finish()
    assert.equal(invalid.invalid, true); assert.deepEqual(invalid.take(), [])
  }
  const capped = nativeFailureCollector()
  capped.observe(Buffer.from(line.repeat(9)))
  assert.equal(capped.invalid, true); assert.equal(capped.take().length, 8)
})

test('diagnostic filtering never prevents private output detection on either channel', () => {
  for (const channel of ['stdout', 'stderr']) {
    const guard = privateOutputGuard(['private-canary'])
    const raw = Buffer.from(nativeMarker + 'layer=RUNNER stage=SESSION_OPEN class=REGISTRY extra=private-canary\n')
    for (const byte of raw) guard.observe(channel, Buffer.from([byte]))
    guard.finish()
    assert.equal(guard.failures.has('b1_private_output_detected'), true)
    assert.deepEqual(guard.nativeDiagnostics.take(), [])
  }
  const guard = privateOutputGuard(['private-canary'])
  guard.observe('stderr', Buffer.from(nativeMarker + 'x'.repeat(600) + 'private-'))
  guard.observe('stderr', Buffer.from('canary\n'))
  guard.finish()
  assert.equal(guard.failures.has('b1_private_output_detected'), true)
  assert.equal(guard.failures.has('b1_native_diagnostic_invalid'), true)
})

test('native failures flush once and bind only to the verified runtime PID and manifest', () => {
  const manifest = 'a'.repeat(64)
  for (const [role, verified, childPID, admitted] of [
    ['RUNTIME', 123, 123, true], ['RUNTIME', undefined, 123, false], ['RUNTIME', 124, 123, false],
    ['SIDECAR', 123, 123, false], ['RUNTIME', undefined, undefined, false]
  ]) {
    const owned = { child: { pid: childPID }, verifiedRuntimePID: verified, guard: privateOutputGuard(), failures: new Set() }
    owned.guard.observe('stderr', Buffer.from(nativeMarker + 'layer=RUNNER stage=CLEANUP class=TERMINATION\n'))
    owned.guard.finish()
    const values = takeVerifiedNativeFailures(owned, role)
    assert.equal(values.length, admitted ? 1 : 0)
    assert.deepEqual(takeVerifiedNativeFailures(owned, role), [])
    assert.equal(owned.failures.has('b1_native_diagnostic_unverified_runtime'), !admitted)
    if (admitted) assert.deepEqual(publicProcessObservation(manifest, 'NATIVE_FAILURE', { ...values[0], private: 'canary' }),
      { schemaVersion: 1, manifestSHA256: manifest, event: 'NATIVE_FAILURE', role: 'RUNTIME', pid: 123,
        layer: 'RUNNER', stage: 'CLEANUP', class: 'TERMINATION' })
  }
  for (const layer of ['constructor', 'private-canary']) {
    assert.equal(publicProcessObservation(manifest, 'NATIVE_FAILURE', { role: 'RUNTIME', pid: 123, layer, stage: 'CLEANUP', class: 'UNKNOWN' }).event, 'UNKNOWN')
  }
})

test('process observations retain only checked identities and fixed lifecycle outcomes', () => {
  const manifest = 'a'.repeat(64)
  for (const event of ['CHILD_STARTED', 'CHILD_DRAINED', 'CHILD_DRAIN_UNVERIFIED']) {
    assert.deepEqual(publicProcessObservation(manifest, event, { role: 'RUNTIME', pid: 123,
      credential: 'private-canary', bootstrap: 'private-canary', error: new Error('/private/private-canary') }),
    { schemaVersion: 1, manifestSHA256: manifest, event, role: 'RUNTIME', pid: 123 })
  }
  assert.deepEqual(publicProcessObservation(manifest, 'FAILURE_RETAINED', {
    childrenDrained: false, residualRoot: root,
    residualWorkspace: '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6/scenario-123',
    stderr: 'private-canary'
  }), { schemaVersion: 1, manifestSHA256: manifest, event: 'FAILURE_RETAINED', childrenDrained: false,
    residualRoot: root, residualWorkspace: '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6/scenario-123' })
  const unknown = publicProcessObservation('private-canary', 'FAILURE_RETAINED', {
    childrenDrained: 'true', residualRoot: '/private/private-canary', residualWorkspace: root + '/private-canary'
  })
  assert.deepEqual(unknown, { schemaVersion: 1, manifestSHA256: null, event: 'FAILURE_RETAINED',
    childrenDrained: null, residualRoot: null, residualWorkspace: null })
  assert.deepEqual(publicProcessObservation(manifest, 'CHILD_DRAIN_UNVERIFIED', { role: 'RUNTIME', pid: null }),
    { schemaVersion: 1, manifestSHA256: manifest, event: 'CHILD_DRAIN_UNVERIFIED', role: 'RUNTIME', pid: null })
  assert.equal(publicProcessObservation(manifest, 'private-canary').event, 'UNKNOWN')
})

test('safe task name excludes the full login substring before constructing a Security operation', () => {
  assert.equal(keychainPaths(root).database, root + '/darwin-secret-store-keychain/analytix-task.keychain-db')
  for (const path of [root + '/darwin-secret-store-keychain/login.keychain-db',
    root + '/darwin-secret-store-keychain/LoGiN.KeYcHaIn-db',
    '/private/tmp/prefix-login.keychain-suffix/darwin-secret-store-keychain/analytix-task.keychain-db',
    root + '/darwin-secret-store-keychain/../darwin-secret-store-keychain/analytix-task.keychain-db']) {
    assert.throws(() => keychainExpectProgram('unlock-keychain', path), /b1_keychain_operation_invalid/u)
  }
})

test('K10 rejects login parents, aliases and unsafe canonical paths with zero Security calls', async () => {
  let systemCalls = 0, writes = 0
  mock.method(childProcess, 'spawn', () => { systemCalls += 1; throw new Error('unexpected Security effect') })
  mock.method(fs, 'mkdirSync', () => { writes += 1; throw new Error('unexpected filesystem effect') })
  mock.method(fs, 'lstatSync', () => ({ isDirectory: () => true, isSymbolicLink: () => false,
    uid: process.getuid(), mode: 0o700 }))
  mock.method(fs, 'realpathSync', () => '/private/LoGiN.KeYcHaIn-alias')
  syncBuiltinESMExports()
  try {
    const cacheRoot = '/private/synthetic-cache'
    assert.equal(k10DarwinTaskKeychainPaths(cacheRoot + '/tmp/task').databasePath,
      cacheRoot + '/tmp/task/darwin-secret-store-keychain/analytix-task.keychain-db')
    for (const isolationRoot of [cacheRoot + '/tmp/prefix-login.keychain-suffix',
      cacheRoot + '/tmp/LoGiN.KeYcHaIn', cacheRoot + '/tmp/task/../task', cacheRoot + '/tmp/task\nquit',
      cacheRoot + '/tmp/task', '/tmp/task']) {
      await assert.rejects(createK10DarwinExplicitTaskKeychain({ isolationRoot, cacheRoot }), /k10_task_keychain_root_invalid/u)
    }
    assert.equal(systemCalls, 0); assert.equal(writes, 0)
  } finally { mock.restoreAll(); syncBuiltinESMExports() }
})

test('owned process inventory includes reparented group members and nested children only', () => {
  const rows = [{ pid: 20, ppid: 1, pgid: 20 }, { pid: 21, ppid: 1, pgid: 20 },
    { pid: 22, ppid: 21, pgid: 22 }, { pid: 90, ppid: 1, pgid: 90 }]
  assert.deepEqual(ownedProcesses(rows, 20).map((r) => r.pid), [20, 21, 22])
  assert.deepEqual(ownedProcesses(rows.slice(1), 20).map((r) => r.pid), [21, 22])
  assert.deepEqual(ownedProcesses(rows.slice(3), 20), [])
})

test('nonzero close preserves known exit while independently checking process drain', async () => {
  for (const [name, closed, rows, expected] of [
    ['nonzero empty', { code: 1, signal: null }, [], { exitCode: 1, signal: null, processGroupEmpty: true }],
    ['nonzero remaining', { code: 1, signal: null }, [{ pid: 42, ppid: 1, pgid: 40, image: 'child' }], { exitCode: 1, signal: null, processGroupEmpty: false }],
    ['nonzero probe error', { code: 1, signal: null }, null, { exitCode: 1, signal: null, processGroupEmpty: null }],
    ['signal', { code: null, signal: 'SIGKILL' }, [], { exitCode: null, signal: 'SIGKILL', processGroupEmpty: true }],
    ['timeout', null, [], { exitCode: null, signal: null, processGroupEmpty: true }],
    ['contradictory close', { code: 0, signal: 'SIGTERM' }, [], { exitCode: null, signal: null, processGroupEmpty: true }],
    ['observed descendant escaped group', { code: 1, signal: null }, [{ pid: 42, ppid: 1, pgid: 42, image: 'child' }], { exitCode: 1, signal: null, processGroupEmpty: false }]
  ]) {
    let probes = 0
    const owned = { child: { pid: 40, exitCode: closed?.code ?? null, signalCode: closed?.signal ?? null },
      observed: new Map([[42, { pid: 42, image: 'child' }]]), observeProcesses() {}, failures: new Set(), guard: privateOutputGuard() }
    const io = { closed: async () => { if (closed === null) throw new Error('private-timeout-canary'); return closed },
      processes: () => { probes += 1; if (rows === null) throw new Error('private-probe-canary'); return rows } }
    await assert.rejects(stopOwned(owned, false, io), (error) => {
      assert.deepEqual(error.childStopOutcome, { pid: 40, ...expected }, name)
      assert.equal(error.message, 'b1_child_exit_failed')
      const projected = publicProcessObservation('a'.repeat(64), 'CHILD_STOP_OUTCOME', { role: 'SIDECAR', ...error.childStopOutcome })
      assert.deepEqual(projected, { schemaVersion: 1, manifestSHA256: 'a'.repeat(64), event: 'CHILD_STOP_OUTCOME', role: 'SIDECAR', pid: 40, ...expected })
      assert.equal(JSON.stringify(projected).includes('canary'), false)
      return true
    })
    assert.equal(probes, 1, name)
  }
})

test('spawn error stays unknown and clean zero exit alone admits successful drain', async () => {
  const owned = { child: {}, observed: new Map(), observeProcesses() {}, failures: new Set(), guard: privateOutputGuard() }
  const noProbe = () => { throw new Error('must not probe an absent PID') }
  await assert.rejects(stopOwned(owned, false, { closed: noProbe, processes: noProbe }), (error) => {
    assert.deepEqual(error.childStopOutcome, { pid: null, exitCode: null, signal: null, processGroupEmpty: null })
    return error.message === 'b1_child_identity_missing'
  })
  owned.child = { pid: 40, exitCode: 0, signalCode: null }; owned.failures.clear()
  const io = { closed: async () => ({ code: 0, signal: null }), processes: () => [] }
  assert.deepEqual(await stopOwned(owned, false, io), { pid: 40, exit: 0, processGroupEmpty: true, observedProcessIDs: [], observedFailureCodes: [] })
  owned.guard = privateOutputGuard(['private-canary']); owned.guard.observe('stderr', Buffer.from('private-canary'))
  await assert.rejects(stopOwned(owned, false, io), (error) => {
    assert.deepEqual(error.childStopOutcome, { pid: 40, exitCode: 0, signal: null, processGroupEmpty: true })
    return error.message === 'b1_runtime_output_failure'
  })
})

test('failed child outcome projection rejects missing extra and contradictory fields', () => {
  const outcome = { role: 'SIDECAR', pid: 40, exitCode: 1, signal: null, processGroupEmpty: true }
  const rejected = [{ ...outcome, extra: 'private-canary' }, { ...outcome, pid: null }, { ...outcome, pid: 0 },
    { ...outcome, exitCode: -1 }, { ...outcome, exitCode: 256 }, { ...outcome, exitCode: '1' },
    { ...outcome, signal: 'SIGTERM' }, { ...outcome, signal: 'private-canary' }, { ...outcome, processGroupEmpty: 'true' }]
  for (const key of Object.keys(outcome)) { const value = { ...outcome }; delete value[key]; rejected.push(value) }
  for (const value of rejected) assert.deepEqual(publicProcessObservation('a'.repeat(64), 'CHILD_STOP_OUTCOME', value),
    { schemaVersion: 1, manifestSHA256: 'a'.repeat(64), event: 'UNKNOWN' })
})

test('output privacy and fixed failure checks survive chunk boundaries without exposing bodies', () => {
  const guard = privateOutputGuard(['synthetic-private-account'])
  guard.observe('stdout', Buffer.from('synthetic-private-'))
  guard.observe('stdout', Buffer.from('account'))
  guard.observe('stderr', Buffer.from('ANALYTIX_RUNTIME_POST_TERMINAL_'))
  guard.observe('stderr', Buffer.from('GOAL_LINEAGE_FAILED'))
  assert.deepEqual([...guard.failures], ['b1_private_output_detected', 'ANALYTIX_RUNTIME_POST_TERMINAL_GOAL_LINEAGE_FAILED'])
  assert.equal(JSON.stringify([...guard.failures]).includes('synthetic-private-account'), false)
  const unicodeGuard = privateOutputGuard(['合成私密值'])
  const unicode = Buffer.from('合成私密值')
  unicodeGuard.observe('stdout', unicode.subarray(0, 2))
  unicodeGuard.observe('stdout', unicode.subarray(2))
  assert.deepEqual([...unicodeGuard.failures], ['b1_private_output_detected'])
})

test('task root refuses aliases, sibling prefixes and path traversal', () => {
  assert.equal(exactRootName(root), true)
  for (const bad of ['/tmp/analytix-own2-b1-process-A1b2C3', root + '/..', root + '/child', root + 'x',
    '/private/tmp/analytix-own2-b1-authority-A1b2C3', '/private/tmp/analytix-own2-b1-process-A1b2C3',
    root.replace('/own2-b1-host-private-roots/', '/other-parent/'), b1ProcessHostParent,
    root.replace('analytix-own2', 'Analytix-own2'), b1ProcessEvidenceRoot + '/analytix-own2-b1-process-A1b2C3',
    b1ProcessEvidenceRoot, b1ProcessEvidenceRoot + '/scenario-123',
    '/Volumes/AnalytixCache/tmp/test', '/Volumes/AnalytixCache/tmp/analytix-own2-b1-process-A1b2C3']) {
    assert.equal(exactRootName(bad), false)
    assert.equal(publicProcessObservation('a'.repeat(64), 'FAILURE_RETAINED', { residualRoot: bad }).residualRoot, null)
  }
})

function ancestorFixture() {
  const entries = new Map(), visited = []
  for (let current = b1ProcessHostParent; ; current = current.slice(0, current.lastIndexOf('/')) || '/') {
    entries.set(current, { uid: BigInt(process.geteuid()), mode: 0o40700n, nlink: 2n,
      isDirectory: () => true, isSymbolicLink: () => false })
    if (current === '/') break
  }
  entries.get('/').uid = 0n
  const io = { stat: (path) => { visited.push(path); return { ...entries.get(path) } }, realpath: (path) => path }
  return { entries, visited, io }
}

test('fixed host ancestors accept safe owners without freezing parent child inventory', () => {
  const f = ancestorFixture()
  assertB1ProcessRootAncestors(f.io)
  assert.equal(f.visited.length, f.entries.size)
  f.entries.get(b1ProcessHostParent).nlink += 7n
  assertB1ProcessRootAncestors(f.io)
  assert.equal(f.visited.includes(root), false)
})

test('unsafe host ancestors fail before any Security process or filesystem mutation', () => {
  let effects = 0
  const unexpected = () => { effects += 1; throw new Error('unexpected side effect') }
  for (const name of ['spawn', 'execFileSync']) mock.method(childProcess, name, unexpected)
  for (const name of ['mkdirSync', 'mkdtempSync', 'chmodSync']) mock.method(fs, name, unexpected)
  syncBuiltinESMExports()
  try {
    for (const target of ancestorFixture().entries.keys()) {
      for (const mutate of [
        (f) => { f.entries.get(target).mode = 0o41777n },
        (f) => { f.entries.get(target).mode = 0o40720n },
        (f) => { f.entries.get(target).uid = BigInt(process.geteuid()) + 10000n },
        (f) => { f.entries.get(target).nlink = 0n },
        (f) => { f.entries.get(target).isDirectory = () => false },
        (f) => { f.entries.get(target).isSymbolicLink = () => true },
        (f) => { f.io.realpath = (path) => path === target ? path + '/alias' : path },
        (f) => { f.io.stat = () => { throw new Error('private path canary') } }
      ]) {
        const f = ancestorFixture(); mutate(f)
        assert.throws(() => assertB1ProcessRootAncestors(f.io), /^Error: b1_private_root_ancestor_invalid$/u)
      }
    }
    for (const change of [{ mode: 0o40750n }, { mode: 0o41700n }, { uid: 0n }]) {
      const f = ancestorFixture(); Object.assign(f.entries.get(b1ProcessHostParent), change)
      assert.throws(() => assertB1ProcessRootAncestors(f.io), /^Error: b1_private_root_ancestor_invalid$/u)
    }
    assert.equal(effects, 0)
  } finally { mock.restoreAll(); syncBuiltinESMExports() }
})

test('explicit binding pins the exact common root and database identity', () => {
  const actual = binding()
  const { bindingDigest, ...unsigned } = actual
  assert.equal(bindingDigest, sha256(JSON.stringify(unsigned)))
  assert.equal(actual.keychainSecurityDigest, sha256(JSON.stringify({ schemaVersion: 1,
    pathDigest: sha256(db.path), device: '12', inode: '34', owner: '501', mode: '600', links: '1' })))
  for (const change of [{ path: '/Users/synthetic/Library/Keychains/login.keychain-db' }, { links: '2' }, { mode: '644' }]) {
    assert.throws(() => bindingFromIdentity(root, root + '/user-data', root + '/sidecar/data', { ...db, ...change }))
  }
  assert.throws(() => bindingFromIdentity(root, root + '/user-data', '/private/tmp/another/data', db))
})

test('PTY program contains only exact task path and takes the password from stdin', () => {
  for (const operation of ['create-keychain', 'unlock-keychain']) {
    const database = operation === 'create-keychain' ? keychainPaths(root).creationDatabase : db.path
    const program = keychainExpectProgram(operation, database)
    assert.ok(program.includes('set secret [gets stdin]'))
    assert.ok(program.includes('log_user 0'))
    assert.ok(program.includes('timeout { stop_owned_security }'))
    assert.ok(program.includes('trap {stop_owned_security} {TERM INT}'))
    assert.ok(program.includes('catch {wait}'))
    assert.ok(program.includes(`spawn -noecho /usr/bin/security ${operation} -- $keychain`))
    assert.equal(program.includes(' -p '), false)
    assert.equal(program.includes('list-keychains'), false)
    assert.ok(program.includes(Buffer.from(database).toString('hex')))
  }
  assert.equal(keychainPaths(root).creationDatabase.includes('/login.keychain'), false)
  assert.throws(() => keychainExpectProgram('create-keychain', db.path))
  assert.throws(() => keychainExpectProgram('unlock-keychain', keychainPaths(root).creationDatabase))
  for (const operation of ['default-keychain', 'list-keychains', 'delete-keychain']) assert.throws(() => keychainExpectProgram(operation, db.path))
  assert.throws(() => keychainExpectProgram('unlock-keychain', '/Users/synthetic/Library/Keychains/login.keychain-db'))
})

function migrationFixture() {
  const paths = keychainPaths(root)
  const created = { ...db, path: paths.creationDatabase }
  const parent = { path: paths.parent, device: '12', inode: '30', owner: '501', mode: '700', links: '0' }
  const rootIdentity = { ...parent, path: root, inode: '29' }
  const companion = { path: paths.creationLock, device: '12', inode: '33', owner: String(process.getuid()), mode: '400', links: '1', size: '0' }
  const files = new Map([[paths.creationDatabase, { ...created }], [paths.creationLock, { ...companion }]])
  const events = []
  const read = (path, links) => {
    const value = files.get(path)
    if (!value || value.symbolic || value.directory || value.canonical || value.size === '0' ||
      value.owner !== String(process.getuid()) || value.mode !== '600' || value.links !== links) throw new Error('invalid synthetic identity')
    return { ...value }
  }
  const io = {
    identity(path, directory = true) {
      if (!directory) return read(path, '1')
      if (path === root) return { ...rootIdentity }
      if (path === paths.parent) return { ...parent }
      if (path === root + '/user-data' || path === root + '/sidecar/data') return { ...parent, path }
      throw new Error('unexpected directory')
    },
    linkedIdentity: (path) => read(path, '2'),
    lockIdentity: (path) => keychainLockIdentity(path, {
      stat: () => {
        const value = files.get(path)
        if (!value) throw new Error('missing synthetic lock')
        return { dev: BigInt(value.device), ino: BigInt(value.inode), uid: BigInt(value.owner),
          mode: BigInt('0o' + value.mode), nlink: BigInt(value.links), size: BigInt(value.size),
          isFile: () => !value.directory && !value.symbolic, isSymbolicLink: () => !!value.symbolic }
      },
      realpath: () => files.get(path).canonical || path
    }),
    entryAbsent: (path) => !files.has(path),
    names: () => [...files.keys()].map((path) => path.slice(paths.parent.length + 1)),
    assertNoHandles: () => events.push('handles'),
    linkExclusive(source, target) {
      if (files.has(target)) throw new Error('destination exists')
      events.push('link')
      const value = files.get(source)
      value.links = '2'
      files.set(target, { ...value, path: target })
    },
    unlink(path) {
      events.push('unlink')
      files.delete(path)
      files.get(paths.database).links = '1'
    },
    syncDirectory: () => events.push('sync')
  }
  return { paths, created, companion, parent, rootIdentity, files, events, io }
}

test('non-login creation migrates one inode exclusively before final binding', () => {
  const f = migrationFixture()
  const final = migrateCreatedKeychain(root, f.created, f.io)
  assert.deepEqual(final, db)
  assert.deepEqual(f.events, ['handles', 'link', 'sync', 'handles', 'unlink', 'sync', 'handles'])
  assert.deepEqual([...f.files.keys()], [f.paths.creationLock, f.paths.database])
  assert.deepEqual(bindingFromIdentity(root, root + '/user-data', root + '/sidecar/data', final), binding())
})

test('migration preserves the legitimate Apple creation lock companion', () => {
  const f = migrationFixture()
  assert.equal(f.paths.creationLock, f.paths.parent + '/.fl7B8797A2')
  assert.notEqual(f.paths.databaseLock, f.paths.creationLock)
  // Filesystem enumeration order has no authority meaning.
  const names = f.io.names; f.io.names = () => names().reverse()
  assert.deepEqual(migrateCreatedKeychain(root, f.created, f.io), db)
  assert.deepEqual(f.files.get(f.paths.creationLock), f.companion)
})

test('migration rejects missing, unknown and malformed lock companions without mutation', () => {
  const changes = [{ mode: '600' }, { mode: '444' }, { mode: '4400' }, { links: '2' }, { size: '1' },
    { owner: String(process.getuid() + 1) }, { symbolic: true }, { directory: true }, { canonical: root + '/alias' }]
  for (const change of changes) {
    const f = migrationFixture()
    Object.assign(f.files.get(f.paths.creationLock), change)
    assert.throws(() => migrateCreatedKeychain(root, f.created, f.io), /b1_keychain_lock_identity_invalid/u)
    assert.deepEqual(f.events, [])
  }
  for (const change of [
    (f) => f.files.delete(f.paths.creationLock),
    (f) => f.files.set(f.paths.parent + '/.fl00000000', { ...f.companion }),
    (f) => f.files.set(f.paths.databaseLock, { ...f.companion, path: f.paths.databaseLock })
  ]) {
    const f = migrationFixture(); change(f)
    assert.throws(() => migrateCreatedKeychain(root, f.created, f.io))
    assert.deepEqual(f.events, [])
  }
  const f = migrationFixture()
  const unknown = f.paths.parent + '/.fl00000000'
  f.files.set(unknown, { ...f.companion, path: unknown })
  assert.throws(() => f.io.lockIdentity(unknown), /b1_keychain_lock_identity_invalid/u)
})

test('held or raced companions retain source and never mutate the lock inode', () => {
  for (const phase of ['pre-link', 'post-link']) {
    for (const change of ['held', 'replaced', 'missing', 'resized', 'extra']) {
      const f = migrationFixture()
      let probes = 0
      f.io.assertNoHandles = () => {
        if (++probes !== (phase === 'pre-link' ? 1 : 2)) return
        if (change === 'held') throw new Error('synthetic lock holder')
        if (change === 'replaced') f.files.get(f.paths.creationLock).inode = '999'
        if (change === 'missing') f.files.delete(f.paths.creationLock)
        if (change === 'resized') f.files.get(f.paths.creationLock).size = '1'
        if (change === 'extra') f.files.set(f.paths.parent + '/.fl00000000', { ...f.companion })
      }
      assert.throws(() => migrateCreatedKeychain(root, f.created, f.io))
      assert.equal(f.files.has(f.paths.creationDatabase), true)
      assert.equal(f.events.includes('unlink'), false)
      assert.equal(f.events.includes('link'), phase === 'post-link')
      if (change === 'held' || change === 'extra') assert.deepEqual(f.files.get(f.paths.creationLock), f.companion)
    }
  }
})

test('post-migration guards pin only the two exact basename locks through final cleanup', () => {
  const f = migrationFixture()
  migrateCreatedKeychain(root, f.created, f.io)
  const check = pinMigratedKeychainLocks(root, f.companion, f.io)
  assert.doesNotThrow(check)
  const finalLock = { ...f.companion, path: f.paths.databaseLock, inode: '35' }
  f.files.set(f.paths.databaseLock, finalLock)
  assert.throws(check, /b1_keychain_unowned_lock_creation/u)
  assert.doesNotThrow(() => check(true))
  for (const change of [{ inode: '999' }, { mode: '600' }, { links: '2' }, { size: '1' }]) {
    f.files.set(f.paths.databaseLock, { ...finalLock, ...change })
    assert.throws(check)
  }
  f.files.delete(f.paths.databaseLock); assert.throws(check)
  f.files.set(f.paths.databaseLock, finalLock); assert.doesNotThrow(check)
  const unknown = f.paths.parent + '/.fl00000000'
  f.files.set(unknown, { ...f.companion }); assert.throws(check)
  f.files.delete(unknown)
  f.files.get(f.paths.creationLock).inode = '999'; assert.throws(check)
  assert.equal(f.files.has(f.paths.creationLock), true)
  assert.equal(f.files.has(f.paths.databaseLock), true)
})

function lifecycleFixture() {
  const f = migrationFixture()
  migrateCreatedKeychain(root, f.created, f.io)
  let unlocks = 0
  const operations = { assertSecurityUnchanged() {}, async unlock() { unlocks += 1 } }
  const lifecycle = keychainLifecycle(root, db, f.companion, operations, f.io)
  const replaceDB = (inode) => f.files.set(f.paths.database, { ...db, inode })
  const launchBinding = () => lifecycle.binding(root + '/user-data', root + '/sidecar/data')
  return { ...f, operations, lifecycle, replaceDB, launchBinding, unlocks: () => unlocks }
}

test('stable unlock and verified protected write survive strict second launch', async () => {
  const f = lifecycleFixture(), life = f.lifecycle
  let unlocks = 0
  f.operations.unlock = async () => {
    unlocks += 1
    if (unlocks === 1) f.files.set(f.paths.databaseLock, { ...f.companion, path: f.paths.databaseLock, inode: '40' })
  }
  assert.equal((await life.unlock()).inodeChanged, false)
  const first = f.launchBinding()
  assert.equal(first.keychainSecurityDigest, binding().keychainSecurityDigest)
  life.runtimeStarted(101); life.beginProviderWrite(101)
  f.replaceDB('42') // production Put's verified add/readback changes the inode
  let readbacks = 0
  const written = await life.verifyProvider(101, async () => { readbacks += 1; return true })
  assert.deepEqual(written, { operation: 'verified_provider_write', inodeChanged: true, stableReadback: true })
  life.runtimeStopped(101)
  assert.throws(f.launchBinding, /b1_keychain_binding_state_invalid/u)
  await life.unlock()
  const second = f.launchBinding()
  assert.notEqual(second.keychainSecurityDigest, first.keychainSecurityDigest)
  assert.equal(second.keychainDBPath, first.keychainDBPath)
  assert.equal(second.bindingDigest, sha256(JSON.stringify((({ bindingDigest, ...unsigned }) => unsigned)(second))))
  life.runtimeStarted(102)
  assert.throws(() => life.beginProviderWrite(102), /b1_keychain_provider_write_state_invalid/u)
  assert.equal((await life.verifyProvider(102, async () => { readbacks += 1; return true })).inodeChanged, false)
  life.runtimeStopped(102)
  assert.doesNotThrow(() => life.assertCleanupReady())
  assert.equal(readbacks, 2)
  assert.equal(unlocks, 2)
})

test('unowned database drift cannot be refreshed at idle, launch, write or restart boundaries', async () => {
  const idle = lifecycleFixture(); idle.replaceDB('99')
  await assert.rejects(idle.lifecycle.unlock(), /b1_keychain_unlock_precondition_drift/u)
  assert.equal(idle.unlocks(), 0)
  const ready = lifecycleFixture(); await ready.lifecycle.unlock(); ready.replaceDB('99')
  assert.throws(ready.launchBinding, /b1_keychain_launch_identity_drift/u)
  const writing = lifecycleFixture(); await writing.lifecycle.unlock(); writing.lifecycle.runtimeStarted(101); writing.replaceDB('99')
  assert.throws(() => writing.lifecycle.beginProviderWrite(101), /b1_keychain_provider_write_precondition_drift/u)
  const stopped = lifecycleFixture(); await stopped.lifecycle.unlock(); stopped.lifecycle.runtimeStarted(101)
  stopped.lifecycle.beginProviderWrite(101); await stopped.lifecycle.verifyProvider(101, async () => true)
  stopped.replaceDB('98')
  assert.throws(() => stopped.lifecycle.runtimeStopped(101), /b1_keychain_runtime_identity_drift/u)
  stopped.replaceDB('34'); stopped.lifecycle.runtimeStopped(101); stopped.replaceDB('99')
  await assert.rejects(stopped.lifecycle.unlock(), /b1_keychain_unlock_precondition_drift/u)
  assert.equal(stopped.unlocks(), 1)
})

test('failed unlock, invalid database, parent, lock, configuration or held handles never commit', async () => {
  for (const change of [
    (f) => { throw new Error('synthetic unsuccessful unlock') },
    (f) => { f.files.get(f.paths.database).mode = '644' },
    (f) => { f.files.get(f.paths.database).owner = String(process.getuid() + 1) },
    (f) => { f.files.get(f.paths.database).links = '2' },
    (f) => { f.files.get(f.paths.database).size = '0' },
    (f) => { f.files.get(f.paths.database).symbolic = true },
    (f) => { f.files.get(f.paths.database).canonical = '/synthetic/alias' },
    (f) => { f.files.get(f.paths.database).device = '999' },
    (f) => { f.parent.inode = '999' },
    (f) => { f.rootIdentity.inode = '999' },
    (f) => { f.files.get(f.paths.creationLock).inode = '999' },
    (f) => { f.files.set(f.paths.parent + '/.fl00000000', { ...f.companion }) },
    (f) => { f.operations.assertSecurityUnchanged = () => { throw new Error('synthetic configuration drift') } },
    (f) => { f.io.assertNoHandles = () => { throw new Error('synthetic held handle') } }
  ]) {
    const f = lifecycleFixture()
    f.operations.unlock = async () => { f.replaceDB('41'); change(f) }
    await assert.rejects(f.lifecycle.unlock())
    assert.throws(f.launchBinding, /b1_keychain_binding_state_invalid/u)
    await assert.rejects(f.lifecycle.unlock(), /b1_keychain_unlock_state_invalid/u)
  }
  const held = lifecycleFixture()
  held.io.assertNoHandles = () => { throw new Error('synthetic held handle') }
  await assert.rejects(held.lifecycle.unlock()); assert.equal(held.unlocks(), 0)
})

test('protected write requires owned runtime and successful stable production readback', async () => {
  for (const fault of ['failure', 'throw', 'database', 'parent', 'lock', 'config', 'held']) {
    const f = lifecycleFixture(), life = f.lifecycle
    await life.unlock(); life.runtimeStarted(101)
    assert.throws(() => life.beginProviderWrite(999), /b1_keychain_runtime_state_invalid/u)
    await assert.rejects(life.verifyProvider(101, async () => true), /b1_keychain_provider_readback_state_invalid/u)
    life.beginProviderWrite(101); f.replaceDB('42')
    assert.throws(() => life.runtimeStopped(101), /b1_keychain_runtime_state_invalid/u)
    await assert.rejects(life.verifyProvider(101, async () => {
      if (fault === 'throw') throw new Error('synthetic readback failure')
      if (fault === 'database') f.replaceDB('43')
      if (fault === 'parent') f.parent.inode = '999'
      if (fault === 'lock') f.files.get(f.paths.creationLock).inode = '999'
      if (fault === 'config') f.operations.assertSecurityUnchanged = () => { throw new Error('synthetic configuration drift') }
      if (fault === 'held') f.io.assertNoHandles = () => { throw new Error('synthetic held handle') }
      return fault !== 'failure'
    }))
    assert.throws(() => life.runtimeStopped(101))
    assert.throws(f.launchBinding)
  }
})

test('second runtime cannot authorize another identity transition with a readback claim', async () => {
  const f = lifecycleFixture(), life = f.lifecycle
  await life.unlock(); life.runtimeStarted(101); life.beginProviderWrite(101)
  f.replaceDB('42'); await life.verifyProvider(101, async () => true); life.runtimeStopped(101)
  await life.unlock()
  assert.throws(() => life.runtimeStarted(101), /b1_keychain_runtime_state_invalid/u)
  life.runtimeStarted(102); f.replaceDB('43')
  let probes = 0
  await assert.rejects(life.verifyProvider(102, async () => { probes += 1; return true }), /b1_keychain_restart_identity_drift/u)
  assert.equal(probes, 0)
})

test('unlock drift reports only bounded field booleans and cannot enable provisioning cleanup', async () => {
  const f = lifecycleFixture()
  assert.doesNotThrow(f.lifecycle.assertProvisioningCleanupReady)
  f.operations.unlock = async () => f.replaceDB('private-canary-inode')
  await assert.rejects(f.lifecycle.unlock(), (error) => {
    const failure = publicHarnessFailure(error)
    assert.deepEqual(failure, { code: 'b1_keychain_unlock_identity_drift', identityDifferences: {
      pathChanged: false, deviceChanged: false, inodeChanged: true, ownerChanged: false, modeChanged: false, linksChanged: false
    } })
    assert.equal(JSON.stringify(failure).includes('private-canary'), false)
    return true
  })
  assert.throws(f.lifecycle.assertProvisioningCleanupReady)
  assert.deepEqual(publicHarnessFailure(Object.assign(new Error('private-canary-body'), {
    identityDifferences: { inodeChanged: true, ownerChanged: 'private-canary-owner', privatePath: root }
  })), { code: 'b1_owner_harness_failed', identityDifferences: { inodeChanged: true } })
  const ready = lifecycleFixture()
  await ready.lifecycle.unlock(); assert.doesNotThrow(ready.lifecycle.assertProvisioningCleanupReady)
  assert.throws(ready.lifecycle.assertCleanupReady)
  ready.lifecycle.runtimeStarted(101); assert.throws(ready.lifecycle.assertProvisioningCleanupReady)
})

function cleanupFixture() {
  const files = new Map(), operations = []
  const put = (path, directory, ino) => files.set(path, { dev: 1n, ino: BigInt(ino), uid: BigInt(process.getuid()),
    mode: directory ? 0o40700n : 0o100600n, nlink: 1n, size: directory ? 0n : 5n,
    isDirectory: () => directory, isFile: () => !directory, isSymbolicLink: () => false })
  put(root, true, 1); put(root + '/nested', true, 2)
  put(root + '/nested/a', false, 3); put(root + '/nested/b', false, 4)
  const io = {
    stat: (path) => { if (!files.has(path)) throw new Error('private-canary-missing'); return { ...files.get(path) } },
    names: (path) => [...files.keys()].filter((key) => key.startsWith(path + '/') && !key.slice(path.length + 1).includes('/'))
      .map((key) => key.slice(path.length + 1)),
    realpath: (path) => path, absent: (path) => !files.has(path), noHandles: () => {},
    unlink: (path) => { operations.push(path); files.delete(path) },
    rmdir: (path) => { assert.equal([...files.keys()].some((key) => key.startsWith(path + '/')), false); operations.push(path); files.delete(path) }
  }
  const checkRoot = () => assert.equal(files.get(root)?.ino, 1n)
  return { files, operations, io, put, checkRoot }
}

test('exact cleanup verifies the pinned inventory and repeated success never adopts a new root', () => {
  const f = cleanupFixture(), cleanup = createExactCleanup(root, f.checkRoot, () => {}, f.io)
  const result = cleanup()
  assert.equal(result.removed, true); assert.equal(result.entries, 4); assert.equal(result.residuals, 0)
  assert.equal(result.manifestSHA256.length, 64); assert.equal(f.files.size, 0)
  assert.deepEqual(cleanup(), result); assert.equal(f.operations.length, 4)
  f.put(root, true, 99)
  assert.throws(cleanup, /b1_cleanup_root_reappeared/u)
  assert.equal(f.files.get(root).ino, 99n); assert.equal(f.operations.length, 4)
})

test('host task and cache scenario cleanup preserve both parents history and siblings', () => {
  const f = cleanupFixture(), scenario = b1ProcessEvidenceRoot + '/scenario-123'
  const other = b1ProcessHostParent + '/analytix-own2-b1-process-Z9y8X7'
  f.put(b1ProcessHostParent, true, 9)
  f.put(b1ProcessEvidenceRoot, true, 10); f.put(b1ProcessEvidenceRoot + '/historical-seal.json', false, 11)
  f.put(scenario, true, 12); f.put(scenario + '/case-fixture', false, 13); f.put(other, true, 14)
  for (const [path, entry] of f.files) if (path.startsWith(b1ProcessEvidenceRoot)) entry.dev = 2n
  createExactCleanup(root, f.checkRoot, () => {}, f.io)()
  assert.equal(f.files.has(scenario), true); assert.equal(f.files.has(other), true)
  createExactCleanup(scenario, () => assert.equal(f.files.get(scenario)?.ino, 12n), () => {}, f.io)()
  assert.deepEqual([...f.files.keys()].sort(), [b1ProcessHostParent, b1ProcessEvidenceRoot, b1ProcessEvidenceRoot + '/historical-seal.json', other].sort())
  assert.equal(f.operations.every((path) => path === root || path.startsWith(root + '/') || path === scenario || path.startsWith(scenario + '/')), true)
})

test('partial cleanup failure preserves unknown residuals and repeat calls cannot retry deletion', () => {
  const f = cleanupFixture(), unlink = f.io.unlink
  f.io.unlink = (path) => {
    if (path.endsWith('/a')) throw new Error('private-canary-path-and-body')
    unlink(path)
  }
  const cleanup = createExactCleanup(root, f.checkRoot, () => {}, f.io)
  let failed
  assert.throws(cleanup, (error) => {
    failed = error
    assert.deepEqual(publicHarnessFailure(error), { code: 'b1_cleanup_failed', cleanup: {
      removed: false, residuals: null, retryable: false, removedEntries: 1
    } })
    assert.equal(JSON.stringify(error).includes('private-canary'), false)
    return true
  })
  assert.equal(f.files.size, 3); assert.equal(f.operations.length, 1)
  assert.throws(cleanup, (error) => error === failed)
  assert.equal(f.operations.length, 1)
})

test('cleanup rejects held handles, unknown ownership and post-inventory ancestor or file replacement', () => {
  for (const change of [
    (f) => { f.io.noHandles = () => { throw new Error('private-canary-handle') } },
    (f) => { f.files.get(root + '/nested/a').uid += 1n },
    (f) => { f.files.get(root + '/nested/a').isSymbolicLink = () => true },
    (f) => { f.files.get(root + '/nested/a').nlink = 2n }
  ]) {
    const f = cleanupFixture(); change(f)
    assert.throws(createExactCleanup(root, f.checkRoot, () => {}, f.io))
    assert.equal(f.operations.length, 0)
  }
  for (const target of [root, root + '/nested', root + '/nested/b']) {
    const f = cleanupFixture()
    assert.throws(createExactCleanup(root, f.checkRoot, () => { f.files.get(target).ino = 99n }, f.io))
    assert.equal(f.operations.length, 0)
  }
})

test('migration refuses alias, source drift, extra entries, existing target and active handles before mutation', () => {
  for (const change of [
    (f) => { f.created.path = root + '/other/bootstrap.keychain-db' },
    (f) => { f.files.get(f.paths.creationDatabase).inode = '999' },
    (f) => { f.files.get(f.paths.creationDatabase).mode = '644' },
    (f) => { f.files.get(f.paths.creationDatabase).links = '2' },
    (f) => { f.files.get(f.paths.creationDatabase).symbolic = true },
    (f) => { f.files.set(f.paths.parent + '/unexpected', { ...f.created }) },
    (f) => { f.files.set(f.paths.database, { ...db, inode: '999' }) },
    (f) => { f.io.assertNoHandles = () => { throw new Error('synthetic active handle') } },
    (f) => { f.io.assertNoHandles = () => { f.parent.inode = '999' } }
  ]) {
    const f = migrationFixture(); change(f)
    assert.throws(() => migrateCreatedKeychain(root, f.created, f.io))
    assert.equal(f.events.includes('link'), false)
    assert.equal(f.events.includes('unlink'), false)
  }
})

test('migration never overwrites a raced target or removes source after post-link drift', () => {
  const raced = migrationFixture()
  raced.io.assertNoHandles = () => raced.files.set(raced.paths.database, { ...db, inode: '999' })
  assert.throws(() => migrateCreatedKeychain(root, raced.created, raced.io))
  assert.equal(raced.files.get(raced.paths.database).inode, '999')
  assert.equal(raced.files.has(raced.paths.creationDatabase), true)
  const drift = migrationFixture()
  drift.io.syncDirectory = () => { drift.parent.inode = '999' }
  assert.throws(() => migrateCreatedKeychain(root, drift.created, drift.io))
  assert.equal(drift.files.has(drift.paths.creationDatabase), true)
  assert.equal(drift.files.get(drift.paths.database).links, '2')
  assert.equal(drift.events.includes('unlink'), false)
})

test('private frame is length-delimited and cannot omit explicit Keychain authority', () => {
  const frame = privateFrame({ schemaVersion: 1 }, binding())
  assert.equal(Number(frame.readBigUInt64BE()), frame.length - 8)
  const parsed = JSON.parse(frame.subarray(8))
  assert.deepEqual(parsed.darwinSecretStoreKeychainBindingV1, binding())
  assert.equal(parsed.purpose, 'analytix.runtime-startup-private-frame/v1')
  assert.throws(() => privateFrame({}, null))
})

test('manifest rejects historical apps, incomplete closure and duplicate component paths', () => {
  const app = '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6/dist/mac-arm64/analytix.app'
  const resources = app + '/Contents/Resources'
  const sidecar = '/Volumes/AnalytixCache/development-v3/tmp/own2-b1-process-20260910/rev6/formal-authority-sidecar'
  const runtime = resources + '/runtime-go/bin/runtime-server'
  const paths = [runtime, sidecar, resources + '/app.asar', resources + '/runtime/analytix-native-development-build.json',
    resources + '/runtime/analytix-packaged-build-authority.json', ...['import-accelerator', 'cleaning-ops', 'analysis-compute', 'data-engine'].map((name) => resources + '/runtime/analytix-' + name)]
  const m = { schemaVersion: 1, purpose: 'analytix.own2-b1-process-inputs/v1', app, sidecar, runtime, source: {},
    nativeMarkerSHA256: 'bf70e7106e7d5b99b7bf635907ac2493d71b1c59bc6d15dd972a0ad0c846ecd2',
    files: paths.map((path) => ({ path, sha256: 'a'.repeat(64), bytes: 1, mode: 0o700 })) }
  assert.doesNotThrow(() => assertManifestShape(m))
  assert.throws(() => assertManifestShape({ ...m, app: '/historical/analytix.app' }))
  assert.throws(() => assertManifestShape({ ...m, files: m.files.slice(1) }))
  assert.throws(() => assertManifestShape({ ...m, files: [...m.files.slice(1), m.files[1]] }))
})
