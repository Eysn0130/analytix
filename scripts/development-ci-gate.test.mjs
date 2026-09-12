import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { assertDevelopmentCISuccess, requiredDevelopmentJobs } from './development-ci-gate.mjs'
import { goTestSelection } from './go-test-selection.mjs'

const success = () => Object.fromEntries(requiredDevelopmentJobs.map(name => [name, { result: 'success' }]))

test('a focused Go gate must execute the exact subtest, not merely exit zero', () => {
  const events = [
    { Action: 'run', Package: 'fixture', Test: 'TestRestart/owner' },
    { Action: 'pass', Package: 'fixture', Test: 'TestRestart/owner' },
    { Action: 'pass', Package: 'fixture' }
  ]
  const check = input => {
    const selection = goTestSelection('fixture', 'TestRestart/owner')
    input.forEach(event => selection.observe(event))
    selection.assert()
  }
  check(events)
  for (const invalid of [
    [], events.slice(1), events.slice(0, 2), [...events, events[1]],
    [...events, { Action: 'fail', Package: 'fixture', Test: 'TestOther' }],
    [...events, { Action: 'skip', Package: 'fixture', Test: 'TestRestart/owner' }],
    events.map(event => ({ ...event, Package: 'other' })),
    events.map(event => ({ ...event, Test: 'TestRestart/wrong' })),
    [...events, null],
    [{ Action: 'output', Package: 'fixture', Output: JSON.stringify(events) }]
  ]) assert.throws(() => check(invalid), /Go selection failed/)
})

test('CI routes the complementary platform suites into the same required gate', () => {
  const workflow = readFileSync(new URL('../.github/workflows/ci.yml', import.meta.url), 'utf8')
  const gate = workflow.slice(workflow.indexOf('  development-gate:'))
  const needs = gate.match(/needs: \[([^\]]+)\]/)?.[1].split(',').map(name => name.trim())
  assert.deepEqual(needs?.sort(), [...requiredDevelopmentJobs].sort())
  assert.match(workflow, /npm test -- --tags-filter='!macos-integration'/)
  const native = workflow.slice(workflow.indexOf('  macos-integration:'), workflow.indexOf('  filesystem-contracts:'))
  assert.match(native, /runs-on: macos-15/)
  assert.match(native, /npm test -- --tags-filter=macos-integration/)
  assert.doesNotMatch(native, /continue-on-error|passWithNoTests/)
  const restart = workflow.slice(workflow.indexOf('  restart-contracts:'), workflow.indexOf('  go-tests:'))
  assert.match(restart, /set -o pipefail/)
  assert.match(restart, /profile_root="\$\(cd "\$RUNNER_TEMP" && pwd -P\)"/)
  assert.match(restart, /-o "\$profile_root\/held-restart\.test"/)
  assert.match(restart, /node \.\.\/\.\.\/scripts\/go-test-selection\.mjs/)
})

test('all complete CI job families are required for the merge gate', () => {
  assert.equal(assertDevelopmentCISuccess(success()), true)
  for (const name of requiredDevelopmentJobs) {
    for (const result of ['failure', 'cancelled', 'skipped', 'pending', undefined]) {
      assert.throws(() => assertDevelopmentCISuccess({ ...success(), [name]: { result } }), /did not succeed/)
    }
  }
})

test('missing, malformed or unaccounted CI results cannot produce a green gate', () => {
  for (const input of [null, [], {}, 'success', { ...success(), unexpected: { result: 'success' } }]) {
    assert.throws(() => assertDevelopmentCISuccess(input), /job results/)
  }
  for (const name of requiredDevelopmentJobs) {
    const input = success()
    delete input[name]
    assert.throws(() => assertDevelopmentCISuccess(input), /job results/)
    assert.throws(() => assertDevelopmentCISuccess({ ...success(), [name]: null }), /did not succeed/)
  }
})
