import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { dependencyChecks, toolChecks } from './development-doctor.mjs'

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
