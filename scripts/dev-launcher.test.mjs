import assert from 'node:assert/strict'
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, statSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { assertDevelopmentProfileReady, parseDevelopmentArgs, prepareDevelopmentProfile } from './dev-launcher.mjs'

function fixture(t) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'analytix-dev-launcher-test-')))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const cache = join(root, 'cache')
  mkdirSync(cache, { mode: 0o700 })
  mkdirSync(join(cache, 'tmp'), { mode: 0o700 })
  return { root, env: { ANALYTIX_DEV_CACHE_ROOT: cache, PATH: process.env.PATH } }
}

test('isolated development rejects ambient credentials, live-state and code-injection overrides', t => {
  const f = fixture(t)
  Object.assign(f.env, {
    HOME: '/synthetic/live-home', ANALYTIX_USER_DATA_DIR: '/synthetic/live-profile',
    OPENAI_API_KEY: 'synthetic-not-a-credential', ANALYTIX_HUB_TEST_PASSWORD: 'synthetic-not-a-password',
    ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP: '1', ANALYTIX_GO_RUNTIME_SERVER_BIN: '/synthetic/stale-runtime',
    NODE_OPTIONS: '--require /synthetic/inject.js', ANALYTIX_CHROME_USER_DATA_DIR: '/synthetic/live-browser'
  })
  const before = { ...f.env }
  const profile = prepareDevelopmentProfile(f)
  assert.deepEqual(f.env, before, 'the invoking environment must not be changed')
  for (const key of ['OPENAI_API_KEY', 'ANALYTIX_HUB_TEST_PASSWORD', 'ANALYTIX_GO_RUNTIME_SERVER_BIN', 'NODE_OPTIONS', 'ANALYTIX_CHROME_USER_DATA_DIR']) assert.equal(profile.env[key], undefined)
  assert.equal(profile.env.ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP, '0')
  assert.equal(profile.env.ANALYTIX_DESKTOP_EXTERNAL_STATE_MODE, 'isolated-local-v1')
  assert.equal(profile.env.HOME, profile.userData)
  assert.equal(profile.env.ANALYTIX_USER_DATA_DIR, profile.userData)
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
  assert.deepEqual(parseDevelopmentArgs([]), { fast: false, fresh: false, profile: 'default' })
  assert.deepEqual(parseDevelopmentArgs(['--fast', '--profile', 'recovery']), { fast: true, fresh: false, profile: 'recovery' })
  for (const args of [['--env-file', 'private.env'], ['--profile'], ['--fresh', '--profile', 'recovery'], ['--built']]) assert.throws(() => parseDevelopmentArgs(args))
})
