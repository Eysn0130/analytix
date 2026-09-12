import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { assertDevelopmentCISuccess, requiredDevelopmentJobs } from './development-ci-gate.mjs'
import { goTestSelection } from './go-test-selection.mjs'
import { runtimePackage, runtimeTestShards, otherGoPackages, goExitStatus } from './go-ci-shards.mjs'

test('Go partitions cover every discovered package and runtime test exactly once', () => {
  const packages = ['analytix.local/runtime-go', runtimePackage, `${runtimePackage}/fixture`]
  assert.deepEqual([...otherGoPackages(packages.join('\n')), runtimePackage].sort(), packages.sort())
  const names = ['TestAlpha', 'TestBeta', 'TestGamma', 'TestDelta', 'TestRestart', 'ExampleRead', 'FuzzDecode']
  const listing = values => `${values.join('\n')}\nok\t${runtimePackage}\t0.001s\n`
  const shards = runtimeTestShards(listing(names))
  assert.equal(shards.length, 4)
  assert.deepEqual(shards.flat().sort(), [...names].sort())
  assert.deepEqual(runtimeTestShards(listing([...names].reverse())), shards)
  for (const bad of ['', listing(names.slice(0, 3)), listing([...names, names[0]]), listing(names) + 'unknown inventory\n', names.join('\n')]) {
    assert.throws(() => runtimeTestShards(bad), /inventory/)
  }
  for (const bad of ['', packages.filter(name => name !== runtimePackage).join('\n'), [...packages, runtimePackage].join('\n')]) {
    assert.throws(() => otherGoPackages(bad), /inventory/)
  }
  assert.equal(goExitStatus({ status: 0 }), 0)
  assert.equal(goExitStatus({ status: 2 }), 2)
  assert.equal(goExitStatus({ status: null, signal: 'SIGKILL' }), 1)
  assert.equal(goExitStatus({ status: 0, error: new Error('spawn failed') }), 1)
})

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
  assert.match(restart, /export GOTMPDIR="\$profile_root"/)
  assert.match(restart, /TestRuntimeTestExecutableIsUnpackagedAndCanonical/)
  assert.match(restart, /node \.\.\/\.\.\/scripts\/go-test-selection\.mjs/)
  assert.match(workflow, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs packages/)
  assert.match(workflow, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs runtime "\$TEST_SHARD"/)
  assert.match(workflow, /shard: \[0, 1, 2, 3\]/)
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
