import assert from 'node:assert/strict'
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, renameSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import { identity } from './darwin-task-keychain-core.mjs'
import { assertCommittedDevelopmentKeychainIdentity } from './development-keychain.mjs'
import { assertDevelopmentProfileReady, launchDevelopment, parseDevelopmentArgs, prepareDevelopmentProfile } from './dev-launcher.mjs'

function fixture(t) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-dev-launcher-test-')))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const cache = join(root, 'cache')
  mkdirSync(cache, { mode: 0o700 })
  mkdirSync(join(cache, 'tmp'), { mode: 0o700 })
  return { root, credentialMode: 'isolated-keychain', stateRoot: join(root, 'state'), env: { ANALYTIX_DEV_CACHE_ROOT: cache, PATH: process.env.PATH } }
}

test('isolated development rejects ambient credentials, live-state and code-injection overrides', t => {
  const f = fixture(t)
  Object.assign(f.env, {
    HOME: '/synthetic/live-home', ANALYTIX_USER_DATA_DIR: '/synthetic/live-profile',
    OPENAI_API_KEY: 'synthetic-not-a-credential', ANALYTIX_HUB_TEST_PASSWORD: 'synthetic-not-a-password',
    ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '1', ANALYTIX_GO_RUNTIME_SERVER_BIN: '/synthetic/stale-runtime',
    NODE_OPTIONS: '--require /synthetic/inject.js', ANALYTIX_CHROME_USER_DATA_DIR: '/synthetic/live-browser',
    CSC_LINK: '/synthetic/signing-key', ANALYTIX_RELEASE_ENV: '/synthetic/release.env',
    ANALYTIX_UPDATE_FEED_URL: 'https://production.invalid/'
  })
  const before = { ...f.env }
  const profile = prepareDevelopmentProfile(f)
  assert.deepEqual(f.env, before, 'the invoking environment must not be changed')
  for (const key of ['OPENAI_API_KEY', 'ANALYTIX_HUB_TEST_PASSWORD', 'ANALYTIX_GO_RUNTIME_SERVER_BIN', 'NODE_OPTIONS', 'ANALYTIX_CHROME_USER_DATA_DIR']) assert.equal(profile.env[key], undefined)
  assert.equal(profile.env.ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP, '0')
  assert.equal(profile.env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE, 'isolated-local-v1')
  assert.equal(profile.env.HOME, profile.homeRoot)
  assert.notEqual(profile.homeRoot, profile.userData)
  assert.ok(!profile.homeRoot.startsWith(profile.userData + '/'))
  assert.equal(profile.env.ANALYTIX_USER_DATA_DIR, profile.userData)
  assert.equal(profile.env.ANALYTIX_DEV_STATE_ROOT, f.stateRoot)
  assert.ok(profile.taskRoot.startsWith(f.stateRoot + '/'))
  assert.equal(profile.env.CSC_LINK, undefined)
  assert.equal(profile.env.ANALYTIX_RELEASE_ENV, undefined)
  assert.equal(profile.env.ANALYTIX_UPDATE_FEED_URL, 'https://example.invalid/analytix/development/')
  assert.equal(statSync(profile.userData).mode & 0o777, 0o700)
})

test('restart reuses only the named checkout profile; fresh mode preserves old data', t => {
  const f = fixture(t)
  const first = prepareDevelopmentProfile(f)
  writeFileSync(join(first.userData, 'synthetic-history.txt'), 'synthetic-history')
  assert.equal(prepareDevelopmentProfile(f).userData, first.userData)
  assert.notEqual(prepareDevelopmentProfile({ ...f, profile: 'experiment' }).userData, first.userData)
  assert.notEqual(prepareDevelopmentProfile({ ...f, fresh: true }).userData, first.userData)
  assert.equal(readFileSync(join(first.userData, 'synthetic-history.txt'), 'utf8'), 'synthetic-history')
})

test('missing retained state is rejected without recreating its directories or Keychain', t => {
  const f = fixture(t)
  const profile = prepareDevelopmentProfile(f)
  assert.equal(profile.needsKeychain, true)
  assert.equal(prepareDevelopmentProfile(f).needsKeychain, false)
  const data = join(profile.homeRoot, '.analytix', 'data')
  rmSync(data, { recursive: true })
  assert.throws(() => prepareDevelopmentProfile(f))
  assert.throws(() => statSync(data), { code: 'ENOENT' })
})

test('a prepared directory is not mistaken for a launch-ready Darwin Keychain', t => {
  const f = fixture(t)
  const profile = prepareDevelopmentProfile(f)
  assert.throws(() => assertDevelopmentProfileReady(profile), /task Keychain/)
  const parent = join(profile.taskRoot, 'darwin-secret-store-keychain')
  mkdirSync(parent, { mode: 0o700 })
  const database = join(parent, 'analytix-task.keychain-db')
  writeFileSync(database, '', { mode: 0o600 })
  assert.throws(() => assertDevelopmentProfileReady(profile), /task Keychain/)
  rmSync(database)
  symlinkSync(join(profile.userData, 'missing'), database)
  assert.throws(() => assertDevelopmentProfileReady(profile), /task Keychain/)
})

test('unsafe roots, symlinks, traversal and broadened permissions fail closed', t => {
  const f = fixture(t)
  assert.throws(() => prepareDevelopmentProfile({ ...f, profile: '../live' }), /profile name/)
  assert.throws(() => prepareDevelopmentProfile({ ...f, env: {} }), /cache helper/)
  const profile = prepareDevelopmentProfile(f)
  chmodSync(profile.userData, 0o755)
  assert.throws(() => prepareDevelopmentProfile(f), /owner-only/)
  assert.equal(statSync(profile.userData).mode & 0o777, 0o755, 'do not repair existing unsafe state')
  rmSync(profile.userData, { recursive: true })
  symlinkSync(f.root, profile.userData)
  assert.throws(() => prepareDevelopmentProfile(f), /owner-only/)
})

test('only explicit development modes are accepted', () => {
  assert.deepEqual(parseDevelopmentArgs([]), { fast: false, fresh: false, profile: 'default', unlockKeychain: false, credentialMode: 'development' })
  assert.deepEqual(parseDevelopmentArgs(['--fast', '--profile', 'recovery']), { fast: true, fresh: false, profile: 'recovery', unlockKeychain: false, credentialMode: 'development' })
  for (const args of [['--env-file', 'private.env'], ['--profile'], ['--fresh', '--profile', 'recovery'], ['--built']]) assert.throws(() => parseDevelopmentArgs(args))
})

function launchFixture(needsKeychain) {
  const profile = { needsKeychain }
  const events = []
  const password = Buffer.from('synthetic-task-password')
  const child = {}
  const dependencies = {
    assertProfileReady: current => {
      assert.equal(current, profile)
      events.push('identity')
    },
    runPrerequisite: script => events.push(script),
    promptPassword: () => { events.push('prompt'); return password },
    prepareKeychain: async (current, supplied, options) => {
      assert.equal(current, profile)
      assert.equal(supplied, password)
      assert.equal(supplied.toString(), 'synthetic-task-password')
      assert.deepEqual(options, { create: needsKeychain })
      events.push(needsKeychain ? 'create' : 'unlock')
    },
    startApplication: () => {
      events.push('start')
      return child
    }
  }
  return { profile, events, password, child, dependencies }
}

test('development provisions or unlocks only after prerequisites and clears the password before launch', async () => {
  for (const needsKeychain of [true, false]) {
    const f = launchFixture(needsKeychain)
    f.dependencies.startApplication = () => {
      assert.ok(f.password.every(byte => byte === 0))
      f.events.push('start')
      return f.child
    }
    assert.equal(await launchDevelopment(f.profile, { unlockKeychain: true }, f.dependencies), f.child)
    assert.deepEqual(f.events, [
      ...(!needsKeychain ? ['identity'] : []),
      'doctor', 'build:data-native:development', 'build:runtime',
      'prompt', needsKeychain ? 'create' : 'unlock', 'identity', 'start'
    ])
  }
})

test('any failed prerequisite prevents a password prompt, Keychain operation and application launch', async () => {
  const scripts = ['doctor', 'build:data-native:development', 'build:runtime']
  for (const needsKeychain of [true, false]) {
    for (const failed of scripts) {
      const f = launchFixture(needsKeychain)
      f.dependencies.runPrerequisite = async script => {
        f.events.push(script)
        if (script === failed) throw new Error('synthetic prerequisite failure')
      }
      await assert.rejects(launchDevelopment(f.profile, { unlockKeychain: true }, f.dependencies), /prerequisite failure/)
      assert.deepEqual(f.events, [
        ...(!needsKeychain ? ['identity'] : []), ...scripts.slice(0, scripts.indexOf(failed) + 1)
      ])
    }
  }
})

test('fast retained-profile launch checks identity without prompting unless unlock was requested', async () => {
  const f = launchFixture(false)
  await launchDevelopment(f.profile, { fast: true, unlockKeychain: false }, f.dependencies)
  assert.deepEqual(f.events, ['identity', 'doctor', 'identity', 'start'])
})

test('unsafe retained profile stops before prerequisites and password interaction', async () => {
  const f = launchFixture(false)
  f.dependencies.assertProfileReady = () => { f.events.push('identity'); throw new Error('synthetic unsafe identity') }
  await assert.rejects(launchDevelopment(f.profile, { unlockKeychain: true }, f.dependencies), /unsafe identity/)
  assert.deepEqual(f.events, ['identity'])
})

test('failed Keychain unlock clears the supplied password and never starts the application', async () => {
  const f = launchFixture(false)
  f.dependencies.prepareKeychain = async () => { f.events.push('unlock'); throw new Error('synthetic unlock failure') }
  await assert.rejects(launchDevelopment(f.profile, { fast: true, unlockKeychain: true }, f.dependencies), /unlock failure/)
  assert.ok(f.password.every(byte => byte === 0))
  assert.deepEqual(f.events, ['identity', 'doctor', 'prompt', 'unlock'])
})

test('launcher reads Core committed identity without adopting replacement or repairing missing records', t => {
  const f = fixture(t)
  const profile = prepareDevelopmentProfile(f)
  const parent = join(profile.taskRoot, 'darwin-secret-store-keychain')
  mkdirSync(parent, { mode: 0o700 })
  const database = join(parent, 'analytix-task.keychain-db')
  writeFileSync(database, 'synthetic-database', { mode: 0o600 })
  assertCommittedDevelopmentKeychainIdentity(profile)
  const master = join(profile.homeRoot, '.analytix', 'data', 'private', 'provider-secrets', 'master-key')
  mkdirSync(master, { recursive: true, mode: 0o700 })
  const marker = join(master, 'explicit-task-keychain-binding.v1')
  const current = identity(database, false)
  const record = { schemaVersion: 2, authorityDigest: 'a'.repeat(64), security: {
    schemaVersion: 1, pathDigest: createHash('sha256').update(current.path).digest('hex'),
    device: current.device, inode: current.inode, owner: current.owner, mode: current.mode, links: current.links
  } }
  writeFileSync(marker, JSON.stringify(record), { mode: 0o600 })
  assertCommittedDevelopmentKeychainIdentity(profile)
  if (process.platform !== 'win32') {
    // Replace only after the real lstat checks, at the open boundary. The
    // child timeout detects the old blocking FIFO read without hanging tests.
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', `
      import fs from 'node:fs';
      import {execFileSync} from 'node:child_process';
      import {syncBuiltinESMExports} from 'node:module';
      const profile=JSON.parse(process.argv[1]), marker=process.argv[2];
      const original=fs.openSync; let replaced=false;
      fs.openSync=function(path,...args){
        if(path===marker&&!replaced){
          fs.renameSync(marker,marker+'.saved'); replaced=true;
          execFileSync('/usr/bin/mkfifo',['-m','600',marker]);
        }
        return original.call(fs,path,...args);
      };
      syncBuiltinESMExports();
      const {assertCommittedDevelopmentKeychainIdentity}=await import(${JSON.stringify(new URL('./development-keychain.mjs', import.meta.url).href)});
      let rejected=false;
      try{assertCommittedDevelopmentKeychainIdentity(profile)}catch{rejected=true}
      finally{if(replaced){fs.unlinkSync(marker);fs.renameSync(marker+'.saved',marker)}}
      if(!replaced||!rejected)process.exit(1);
    `, JSON.stringify(profile), marker], { timeout: 5000, encoding: 'utf8' })
    assert.equal(result.error, undefined)
    assert.equal(result.status, 0, result.stderr)
    assertCommittedDevelopmentKeychainIdentity(profile)
  }
  const unsettled = join(master, '.explicit-task-keychain-binding.v1.rollback-required')
  const ambiguous = '{"authorityDigest":"ambiguous",' + JSON.stringify(record).slice(1)
  writeFileSync(marker, ambiguous)
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /unlock\/launch refused/)
  assert.equal(readFileSync(marker, 'utf8'), ambiguous)
  writeFileSync(marker, JSON.stringify(record))
  writeFileSync(unsettled, 'synthetic-unsettled', { mode: 0o600 })
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /unlock\/launch refused/)
  // Only the launch path can defer pending metadata to Core recovery.
  assertCommittedDevelopmentKeychainIdentity(profile, { allowCoreRecovery: true })
  rmSync(unsettled)
  renameSync(database, database + '.retained')
  writeFileSync(database, 'synthetic-database', { mode: 0o600 })
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /unlock\/launch refused/)
  assert.equal(readFileSync(marker, 'utf8'), JSON.stringify(record))
  rmSync(marker)
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /unlock\/launch refused/)
  assert.throws(() => statSync(marker), { code: 'ENOENT' })
  rmSync(master, { recursive: true })
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /identity is missing/)
  rmSync(join(profile.homeRoot, '.analytix', 'data', 'private', 'provider-secrets'), { recursive: true })
  mkdirSync(join(profile.homeRoot, '.analytix', 'data', 'private', 'provider-registry'), { mode: 0o700 })
  assert.throws(() => assertCommittedDevelopmentKeychainIdentity(profile), /identity is missing/)
})

test('ordinary tasks and worktrees share only the development credential authority without Keychain UI', async t => {
  const f = fixture(t)
  const first = prepareDevelopmentProfile({ ...f, credentialMode: 'development' })
  const worktree = join(f.root, 'worktree')
  mkdirSync(worktree, { mode: 0o700 })
  const next = prepareDevelopmentProfile({ ...f, root: worktree, profile: 'next', credentialMode: 'development' })
  assert.notEqual(first.taskRoot, next.taskRoot)
  assert.equal(first.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR, next.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR)
  assert.equal(first.needsKeychain, false)
  assert.equal(next.needsKeychain, false)
  const forbidden = () => { throw new Error('ordinary development requested Keychain interaction') }
  await launchDevelopment(next, { fast: true }, {
    promptPassword: forbidden, prepareKeychain: forbidden,
    runPrerequisite: () => {}, startApplication: () => ({})
  })
  const qa = prepareDevelopmentProfile({ ...f, profile: 'qa' })
  assert.equal(qa.env.ANALYTIX_DEVELOPMENT_PROVIDER_AUTHORITY_DIR, undefined)
  assert.equal(qa.needsKeychain, true)
  assert.throws(() => parseDevelopmentArgs(['--unlock-keychain']))
  assert.equal(parseDevelopmentArgs(['--isolated-keychain', '--unlock-keychain']).credentialMode, 'isolated-keychain')
})
