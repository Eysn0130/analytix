import assert from 'node:assert/strict'
import { createRequire } from 'node:module'
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, existsSync, rmSync } from 'node:fs'
import * as fs from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, isAbsolute, join } from 'node:path'
import { runInNewContext } from 'node:vm'
import test from 'node:test'

const require = createRequire(import.meta.url)
const source = readFileSync(new URL('./ensure-runtime-install.cjs', import.meta.url), 'utf8')

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), 'analytix-install-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  const runtime = join(root, 'packages/runtime')
  mkdirSync(runtime, { recursive: true })
  const manifest = { dependencies: { diff: '1.0.0', zod: '1.0.0', '@modelcontextprotocol/sdk': '1.0.0', 'better-sqlite3': '1.0.0' } }
  writeFileSync(join(runtime, 'package.json'), JSON.stringify(manifest))
  writeFileSync(join(runtime, 'package-lock.json'), '{"lockfileVersion":3}')
  let installs = 0
  let failure = false
  function modules() {
    for (const name of Object.keys(JSON.parse(readFileSync(join(runtime, 'package.json'), 'utf8')).dependencies)) {
      const target = join(runtime, 'node_modules', name, 'package.json')
      mkdirSync(dirname(target), { recursive: true })
      writeFileSync(target, '{}')
    }
  }
  modules()
  function run() {
    const module = { exports: {} }
    const localPath = path => typeof path === 'string' && !isAbsolute(path) ? join(root, path) : path
    const mockedRequire = name => {
      if (name === 'node:fs') return { ...fs,
        existsSync: path => existsSync(localPath(path)),
        rmSync: (path, options) => rmSync(localPath(path), options)
      }
      if (name === 'node:child_process') return { spawnSync: () => {
        installs++
        if (!failure) modules()
        return { status: failure ? 1 : 0 }
      } }
      return require(name)
    }
    mockedRequire.main = module
    const fakeProcess = { ...process, env: { ...process.env }, exit: code => { throw new Error(`exit ${code}`) } }
    runInNewContext(source, { require: mockedRequire, module, __dirname: join(root, 'scripts'), process: fakeProcess, console })
    if (fakeProcess.exitCode) throw new Error(`exit ${fakeProcess.exitCode}`)
  }
  return { root, runtime, run, modules, installs: () => installs, fail: value => { failure = value } }
}

test('existing modules do not conceal a new or changed runtime lockfile', t => {
  const f = fixture(t)
  f.run()
  assert.equal(f.installs(), 1, 'an unverified old installation must be refreshed')
  f.run()
  assert.equal(f.installs(), 1, 'unchanged lockfile reuses the verified installation')
  writeFileSync(join(f.runtime, 'package-lock.json'), '{"lockfileVersion":3,"version":"changed"}')
  f.run()
  assert.equal(f.installs(), 2, 'pulling a changed lockfile must reinstall dependencies')
})

test('missing modules and changed manifest require installation', t => {
  const f = fixture(t)
  f.run()
  rmSync(join(f.runtime, 'node_modules/zod'), { recursive: true })
  f.run()
  assert.equal(f.installs(), 2)
  const manifest = JSON.parse(readFileSync(join(f.runtime, 'package.json'), 'utf8'))
  manifest.description = 'updated manifest'
  writeFileSync(join(f.runtime, 'package.json'), JSON.stringify(manifest))
  f.run()
  assert.equal(f.installs(), 3)
})

test('failed npm ci is not recorded as a successful installation', t => {
  const f = fixture(t)
  f.fail(true)
  assert.throws(f.run)
  f.fail(false)
  f.run()
  assert.equal(f.installs(), 2)
  assert.equal(existsSync(join(f.runtime, 'node_modules/better-sqlite3')), false, 'Electron SQLite remains root-owned')
})

test('a changed linked package manifest refreshes the nested dependency graph', t => {
  const f = fixture(t)
  const linked = join(f.root, 'vendor', 'synthetic-link')
  mkdirSync(linked, { recursive: true })
  writeFileSync(join(linked, 'package.json'), '{"name":"synthetic-link","version":"1.0.0"}')
  const manifest = JSON.parse(readFileSync(join(f.runtime, 'package.json'), 'utf8'))
  manifest.dependencies['synthetic-link'] = 'file:../../vendor/synthetic-link'
  writeFileSync(join(f.runtime, 'package.json'), JSON.stringify(manifest))
  f.run()
  assert.equal(f.installs(), 1)
  writeFileSync(join(linked, 'package.json'), '{"name":"synthetic-link","version":"1.0.1"}')
  f.run()
  assert.equal(f.installs(), 2)
})
