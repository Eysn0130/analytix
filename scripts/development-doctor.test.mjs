import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { dependencyChecks, nativeDependencyCheck, toolChecks } from './development-doctor.mjs'

test('supported source tools pass and missing or old tools never pass', () => {
  const tools = { node: 'v22.22.1', npm: '10.9.4', go: 'go version go1.26.4 darwin/arm64', git: 'git version 2.50.1' }
  assert.ok(toolChecks(tools).every(check => check.passed))
  for (const [name, value] of Object.entries({ node: 'v22.12.0', npm: '9.9.0', go: 'go version go1.21.0', git: '' })) {
    assert.equal(toolChecks({ ...tools, [name]: value }).filter(check => !check.passed).length, 1)
  }
})

test('an empty source checkout is not reported as dependency-ready', t => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-doctor-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  assert.ok(dependencyChecks(root).every(check => !check.passed))
})

test('native doctor verifies Electron module loading, not Node module presence', t => {
  const root = mkdtempSync(join(tmpdir(), 'analytix-doctor-native-test-'))
  t.after(() => rmSync(root, { recursive: true, force: true }))
  mkdirSync(join(root, 'node_modules/electron'), { recursive: true })
  writeFileSync(join(root, 'package.json'), '{}')
  writeFileSync(join(root, 'node_modules/electron/index.js'), 'module.exports = "/synthetic/electron"')
  let calls = 0
  const run = (command, args, options) => {
    calls++
    assert.equal(command, '/synthetic/electron')
    assert.equal(options.env.ELECTRON_RUN_AS_NODE, '1')
    assert.equal(options.env.NODE_OPTIONS, '')
    assert.ok(args[1].includes("':memory:'"))
    assert.ok(args[1].includes("require('node-pty')"))
    assert.equal(options.stdio, 'ignore')
    return { status: 0 }
  }
  assert.equal(nativeDependencyCheck(root, run).passed, true)
  assert.equal(calls, 1)
  for (const result of [{ status: 1 }, { status: null, signal: 'SIGTERM' }, { status: 0, error: new Error('synthetic') }]) {
    assert.equal(nativeDependencyCheck(root, () => result).passed, false)
  }
})
