import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'

const require = createRequire(import.meta.url)
const { appInstallationDigest, appInstallCurrent, beginAppInstall, recordAppInstall } = require('./app-install-state.cjs')
function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-app-install-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const write = (name, content) => {
    mkdirSync(dirname(join(root, name)), { recursive: true })
    writeFileSync(join(root, name), JSON.stringify(content))
  }
  write('package.json', { dependencies: { library: '^1.0.0', linked: 'file:vendor/linked' }, devDependencies: { compiler: '^2.0.0' } })
  write('package-lock.json', { packages: { 'node_modules/library': { version: '1.0.1' }, 'node_modules/compiler': { version: '2.0.1' } } })
  write('node_modules/library/package.json', { version: '1.0.1' })
  write('node_modules/compiler/package.json', { version: '2.0.1' })
  write('node_modules/linked/package.json', { version: '1.0.0' })
  write('vendor/linked/package.json', { version: '1.0.0' })
  return { root, write, digest: () => appInstallationDigest(root) }
}

test('root presence alone is unverified; successful postinstall records exact inputs', t => {
  const f = fixture(t)
  assert.equal(appInstallCurrent(f.root), false)
  recordAppInstall(f.root, f.digest())
  assert.equal(appInstallCurrent(f.root), true)
  f.write('node_modules/compiler/package.json', { version: '1.0.0' })
  assert.equal(appInstallCurrent(f.root), false, 'stale development tools must not pass')
})

test('pulling lock or linked-manifest changes invalidates the stamp', t => {
  const f = fixture(t)
  recordAppInstall(f.root, f.digest())
  f.write('vendor/linked/package.json', { version: '1.0.1' })
  assert.equal(appInstallCurrent(f.root), false)
  assert.throws(() => recordAppInstall(f.root, 'stale-digest'), /inputs changed/)
  assert.throws(() => recordAppInstall(f.root, f.digest()), /incomplete/, 'a stale linked copy must not pass')
  f.write('node_modules/linked/package.json', { version: '1.0.1' })
  recordAppInstall(f.root, f.digest())
  f.write('package-lock.json', { packages: { 'node_modules/library': { version: '1.0.2' } } })
  assert.equal(appInstallCurrent(f.root), false)
  assert.throws(() => recordAppInstall(f.root, f.digest()), /incomplete/)
})

test('a failed same-input reinstall cannot reuse an older success stamp', t => {
  const f = fixture(t)
  recordAppInstall(f.root, f.digest())
  assert.equal(appInstallCurrent(f.root), true)
  const digest = beginAppInstall(f.root)
  assert.equal(appInstallCurrent(f.root), false, 'until the postinstall pipeline finishes, old success is invalid')
  recordAppInstall(f.root, digest)
  assert.equal(appInstallCurrent(f.root), true)
})

test('changed postinstall implementation after pull invalidates old installation', t => {
  const f = fixture(t)
  recordAppInstall(f.root, f.digest())
  f.write('scripts/postinstall.cjs', 'synthetic changed install behavior')
  assert.equal(appInstallCurrent(f.root), false)
})
