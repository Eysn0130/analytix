import assert from 'node:assert/strict'
import test from 'node:test'
import { execFileSync } from 'node:child_process'
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { checkPublicCandidate, checkPublicPush, isPrivateSourcePath } from './check-public-push.mjs'
import { setupGitSync } from './setup-git-sync.mjs'

const zero = '0'.repeat(40)
const env = { ...process.env, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null', GIT_TERMINAL_PROMPT: '0' }
function run(cwd, ...args) {
  return execFileSync('git', args, { cwd, env, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] }).trim()
}
function fixture(t) {
  const directory = mkdtempSync(join(tmpdir(), 'analytix-git-sync-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const cwd = join(directory, 'source')
  mkdirSync(cwd)
  run(cwd, 'init', '-b', 'main')
  run(cwd, 'config', 'user.name', 'Analytix Synthetic Git Test')
  run(cwd, 'config', 'user.email', 'git-test@example.invalid')
  const commit = (name, value = 'synthetic fixture\n') => {
    writeFileSync(join(cwd, name), value)
    run(cwd, 'add', '--', name)
    run(cwd, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Synthetic workflow fixture')
    return run(cwd, 'rev-parse', 'HEAD')
  }
  const publicRoot = commit('README.md')
  return { directory, cwd, commit, publicRoot, policy: { publicRoot, excludedPaths: ['local-only.bin'], maxBlobBytes: 1024 } }
}
const update = (tip, base = zero, ref = 'refs/heads/main') => `refs/heads/main ${tip} ${ref} ${base}\n`

test('normal public descendants and new feature branches are accepted', t => {
  const f = fixture(t)
  const tip = f.commit('source.txt')
  assert.deepEqual(checkPublicPush(update(tip, f.publicRoot), f), { updates: 1, checkedObjects: 1 })
  assert.equal(checkPublicPush(update(tip, zero, 'refs/heads/codex/feature'), f).updates, 1)
})

test('CI inspects actual candidate tree and post-baseline history without changing Git', t => {
  const f = fixture(t)
  const tip = f.commit('source.txt')
  const before = run(f.cwd, 'status', '--porcelain')
  assert.equal(checkPublicCandidate(f).files, 2)
  assert.equal(run(f.cwd, 'rev-parse', 'HEAD'), tip)
  assert.equal(run(f.cwd, 'status', '--porcelain'), before)
  f.commit('local-only.bin')
  run(f.cwd, 'rm', 'local-only.bin')
  run(f.cwd, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Remove synthetic excluded file')
  assert.throws(() => checkPublicCandidate(f), /excluded local file/)
})

test('CI rejects forbidden files even when present in the configured root commit', t => {
  const f = fixture(t)
  const root = f.commit('local-only.bin')
  assert.throws(() => checkPublicCandidate({ ...f, policy: { ...f.policy, publicRoot: root } }), /not solely descended/)
  assert.throws(() => checkPublicCandidate({ ...f, policy: { ...f.policy, excludedPaths: ['README.md'] } }), /excluded local file/)
})

test('private orphan history and a merge of private history are both rejected', t => {
  const f = fixture(t)
  run(f.cwd, 'switch', '--orphan', 'private-archive')
  const privateTip = f.commit('private.txt')
  assert.throws(() => checkPublicPush(update(privateTip), f), /not solely descended/)
  run(f.cwd, 'switch', 'main')
  run(f.cwd, '-c', 'commit.gpgsign=false', 'merge', '--allow-unrelated-histories', '--no-edit', 'private-archive')
  assert.throws(() => checkPublicPush(update(run(f.cwd, 'rev-parse', 'HEAD')), f), /not solely descended/)
})

test('an excluded blob cannot be hidden by deleting it at the final tip', t => {
  const f = fixture(t)
  f.commit('local-only.bin')
  run(f.cwd, 'rm', 'local-only.bin')
  run(f.cwd, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Remove synthetic excluded file')
  assert.throws(() => checkPublicPush(update(run(f.cwd, 'rev-parse', 'HEAD'), f.publicRoot), f), /excluded local file/)
})

test('forced staging of a private env file is rejected but an example is accepted', t => {
  const f = fixture(t)
  const example = f.commit('.env.example', 'SYNTHETIC_VALUE=example-only\n')
  assert.equal(checkPublicPush(update(example, f.publicRoot), f).updates, 1)
  assert.throws(() => checkPublicPush(update(f.commit('.env.local', 'SYNTHETIC_VALUE=not-a-real-secret\n'), example), f), /excluded local file/)
  for (const name of ['.env', 'nested/.env.production', 'bundle.p12', 'owner.keychain-db']) assert.equal(isPrivateSourcePath(name, f.policy), true)
  for (const name of ['.env.example', '.env.analytix-e2e.example', 'src/provider.ts']) assert.equal(isPrivateSourcePath(name, f.policy), false)
})

test('oversize source objects and unsupported refs are rejected', t => {
  const f = fixture(t)
  const tip = f.commit('large.txt', 'x'.repeat(2048))
  assert.throws(() => checkPublicPush(update(tip), f), /size limit/)
  assert.throws(() => checkPublicPush(update(f.publicRoot, zero, 'refs/analytix-private/backup'), f), /unsupported ref/)
  assert.throws(() => checkPublicPush('invalid update\n', f), /unsupported ref/)
})

test('non-fast-forward updates require reconciliation', t => {
  const f = fixture(t)
  const remote = f.commit('remote.txt')
  run(f.cwd, 'switch', '-c', 'codex/local', f.publicRoot)
  const tip = f.commit('local.txt')
  assert.throws(() => checkPublicPush(update(tip, remote), f), /fetch and reconcile/)
})

test('annotated release tags retain the same public-history check', t => {
  const f = fixture(t)
  const tip = f.commit('source.txt')
  run(f.cwd, 'tag', '-a', 'synthetic-tag', '-m', 'Synthetic tag')
  const tag = run(f.cwd, 'rev-parse', 'synthetic-tag')
  assert.notEqual(tag, tip)
  assert.equal(checkPublicPush(update(tag, zero, 'refs/tags/synthetic-tag'), f).updates, 1)
})

test('empty pushes and feature-branch deletions work; deleting main is refused', t => {
  const f = fixture(t)
  assert.equal(checkPublicPush('', f).updates, 0)
  assert.equal(checkPublicPush(update(zero, f.publicRoot, 'refs/heads/codex/feature'), f).updates, 0)
  assert.throws(() => checkPublicPush(update(zero, f.publicRoot), f), /deleting main/)
})

function installFixtureHook(f) {
  const hooksPath = join(f.cwd, '.githooks')
  mkdirSync(hooksPath)
  writeFileSync(join(hooksPath, 'pre-push'), '#!/bin/sh\nexit 0\n')
  chmodSync(join(hooksPath, 'pre-push'), 0o755)
  return hooksPath
}

test('setup is idempotent and preserves an existing origin', t => {
  const f = fixture(t)
  const hooksPath = installFixtureHook(f)
  run(f.cwd, 'remote', 'add', 'origin', join(f.directory, 'remote.git'))
  setupGitSync({ ...f, hooksPath })
  setupGitSync({ ...f, hooksPath })
  assert.equal(run(f.cwd, 'config', '--local', 'pull.ff'), 'only')
  assert.equal(run(f.cwd, 'config', '--local', 'push.default'), 'simple')
  assert.equal(run(f.cwd, 'config', '--local', 'branch.main.remote'), 'origin')
  assert.equal(run(f.cwd, 'config', '--local', 'branch.main.merge'), 'refs/heads/main')
  assert.equal(run(f.cwd, 'config', '--local', 'core.hooksPath'), hooksPath)
  assert.equal(run(f.cwd, 'remote', 'get-url', 'origin'), join(f.directory, 'remote.git'))
})

test('setup will not silently replace existing hooks', t => {
  const f = fixture(t)
  const hooksPath = installFixtureHook(f)
  writeFileSync(join(f.cwd, '.git', 'hooks', 'pre-commit'), '#!/bin/sh\nexit 0\n')
  assert.throws(() => setupGitSync({ ...f, hooksPath }), /Existing Git hooks/)
  run(f.cwd, 'config', 'core.hooksPath', 'custom-hooks')
  assert.throws(() => setupGitSync({ ...f, hooksPath }), /existing hooksPath/)
})

test('two clones can pull, commit and push; divergent pull preserves both histories and local work', t => {
  const f = fixture(t)
  const bare = join(f.directory, 'remote.git')
  run(f.directory, 'init', '--bare', '-b', 'main', bare)
  run(f.cwd, 'remote', 'add', 'origin', bare)
  const hooksPath = installFixtureHook(f)
  setupGitSync({ ...f, hooksPath })
  run(f.cwd, 'push', '-u', 'origin', 'main')
  const peer = join(f.directory, 'peer')
  run(f.directory, 'clone', bare, peer)
  run(peer, 'config', 'user.name', 'Analytix Synthetic Peer')
  run(peer, 'config', 'user.email', 'peer@example.invalid')
  writeFileSync(join(peer, 'incoming.txt'), 'synthetic incoming change\n')
  run(peer, 'add', 'incoming.txt')
  run(peer, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Synthetic peer change')
  run(peer, 'push')
  run(f.cwd, 'pull')
  assert.equal(readFileSync(join(f.cwd, 'incoming.txt'), 'utf8'), 'synthetic incoming change\n')
  const published = f.commit('outgoing.txt')
  run(f.cwd, 'push')
  run(peer, 'pull', '--ff-only')
  assert.equal(run(peer, 'rev-parse', 'HEAD'), published)
  const local = f.commit('local-branch.txt')
  writeFileSync(join(peer, 'remote-branch.txt'), 'synthetic competing change\n')
  run(peer, 'add', 'remote-branch.txt')
  run(peer, '-c', 'commit.gpgsign=false', 'commit', '-m', 'Synthetic competing commit')
  run(peer, 'push')
  writeFileSync(join(f.cwd, 'uncommitted.txt'), 'preserve local work\n')
  assert.throws(() => run(f.cwd, 'pull'))
  assert.equal(run(f.cwd, 'rev-parse', 'HEAD'), local)
  assert.equal(readFileSync(join(f.cwd, 'uncommitted.txt'), 'utf8'), 'preserve local work\n')
  assert.equal(run(f.cwd, 'rev-parse', 'origin/main'), run(peer, 'rev-parse', 'HEAD'))
})
