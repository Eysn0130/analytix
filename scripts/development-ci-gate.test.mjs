import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { assertDevelopmentCISuccess, requiredDevelopmentJobs } from './development-ci-gate.mjs'
import { goTestSelection } from './go-test-selection.mjs'
import { runtimePackage, rootPackage, rootPlatformTests, rootPlatformSelection, runtimeShardCount, runtimeShardIndex, runtimeTestShards, runtimeTestPartition, runtimePlatformSelection, runtimePlatformTests, runtimePlatformSubtests, runtimeExternalDiagnostics, otherGoPackages, goExitStatus } from './go-ci-shards.mjs'
import { goTestPartition } from './go-test-partition.mjs'

test('Go partitions cover every discovered package and runtime test exactly once', () => {
  const packages = ['analytix.local/runtime-go', runtimePackage, `${runtimePackage}/fixture`]
  assert.deepEqual([...otherGoPackages(packages.join('\n')), runtimePackage].sort(), packages.sort())
  const required = [...Array.from({ length: runtimeShardCount * 2 + 1 }, (_, i) => `TestFixture${i}`), 'ExampleRead', 'FuzzDecode']
  const external = runtimeExternalDiagnostics.map(entry => entry.test)
  assert.deepEqual(external, [
    'TestRuntimeOptionalPublicFixturePathV1',
    'TestRuntimeOptionalPluginPublicConsumerV1',
    'TestRuntimeOptionalPluginUnauthorizedFirstMCPTerminalDiagnosticV1',
    'TestLocalNonPublishablePackageInspectionWhenExplicitlyProvided'
  ])
  assert.ok(runtimeExternalDiagnostics.every(entry => typeof entry.reason === 'string' && entry.reason.length > 20))
  const names = [...required, runtimePlatformTests[0], ...external]
  const listing = values => `${values.join('\n')}\nok\t${runtimePackage}\t0.001s\n`
  const shards = runtimeTestShards(listing(names))
  assert.equal(shards.length, runtimeShardCount)
  assert.equal(Math.max(...shards.map(shard => shard.length)) - Math.min(...shards.map(shard => shard.length)), 1)
  assert.deepEqual(shards.flat().sort(), [...required].sort())
  const partition = runtimeTestPartition(listing(names))
  assert.deepEqual([
    ...partition.shards.flat(), ...partition.platformRequired.map(entry => entry.test),
    ...partition.externalDiagnostics.map(entry => entry.test)
  ].sort(), partition.discovered)
  assert.equal(new Set(partition.discovered).size, names.length)
  assert.deepEqual(partition.externalDiagnostics, runtimeExternalDiagnostics.map(entry => ({ ...entry, status: 'not_executed' })))
  assert.equal(partition.platformRequired[0].lane, 'runtime-platform')
  assert.equal(partition.platformRequired[0].platform, 'darwin')
  assert.deepEqual(runtimePlatformSelection(listing([...names, runtimePlatformTests[1]])), runtimePlatformTests)
  assert.throws(() => runtimePlatformSelection(listing(names)), /inventory/)
  for (const name of [...external, runtimePlatformTests[0]]) {
    assert.throws(() => runtimeTestPartition(listing(names.filter(value => value !== name))), /inventory/)
  }
  assert.throws(() => runtimeTestPartition(listing([...required.slice(0, 15), runtimePlatformTests[0], ...external])), /empty shard/)
  assert.deepEqual(runtimeTestShards(listing([...names].reverse())), shards)
  for (const bad of ['', listing(names.slice(0, runtimeShardCount - 1)), listing([...names, names[0]]), listing(names) + 'unknown inventory\n', names.join('\n')]) {
    assert.throws(() => runtimeTestShards(bad), /inventory/)
  }
  for (const bad of ['', packages.filter(name => name !== runtimePackage).join('\n'), [...packages, runtimePackage].join('\n')]) {
    assert.throws(() => otherGoPackages(bad), /inventory/)
  }
  assert.equal(goExitStatus({ status: 0 }), 0)
  assert.equal(goExitStatus({ status: 2 }), 2)
  assert.equal(goExitStatus({ status: null, signal: 'SIGKILL' }), 1)
  assert.equal(goExitStatus({ status: 0, error: new Error('spawn failed') }), 1)
  for (let index = 0; index < runtimeShardCount; index++) assert.equal(runtimeShardIndex(String(index)), index)
  for (const bad of [undefined, '', '-1', '01', '1.5', 'NaN', String(runtimeShardCount), '999999999999999999999999']) {
    assert.throws(() => runtimeShardIndex(bad), /Expected runtime shard/)
  }
})

test('required runtime partitions reject zero, missing, duplicate, skipped or incomplete execution', () => {
  const event = (Action, Test) => ({ Action, Package: runtimePackage, ...(Test === undefined ? {} : { Test }) })
  const events = [event('start'), event('run', 'TestOne'), event('run', 'TestOne/child'),
    event('pass', 'TestOne/child'), event('pass', 'TestOne'), event('run', 'TestTwo'),
    event('pass', 'TestTwo'), event('pass')]
  const check = input => {
    const partition = goTestPartition(runtimePackage, ['TestOne', 'TestTwo'], ['TestOne/child'])
    input.forEach(value => partition.observe(value))
    partition.assert()
  }
  check(events)
  for (const input of [
    [], events.slice(1, -1), events.filter(value => value.Test !== 'TestTwo'),
    events.filter(value => value.Test !== 'TestOne/child'),
    events.filter(value => value.Action !== 'run'), [...events, events.at(-1)],
    [...events.slice(0, -1), event('pass', 'TestOne'), event('pass')],
    [...events.slice(0, -1), event('run', 'TestOne'), event('pass')],
    events.map(value => value.Test === 'TestOne/child' && value.Action === 'pass' ? event('skip', value.Test) : value),
    [...events.slice(0, -1), event('run', 'TestOne/unfinished'), event('pass')],
    [...events.slice(0, -1), event('run', 'TestUnexpected'), event('pass', 'TestUnexpected'), event('pass')],
    [...events.slice(0, -1), event('fail', 'TestOne'), event('pass')],
    [...events.slice(0, -1), event('skip'), event('pass')],
    events.map(value => ({ ...value, Package: 'wrong' })),
    [...events, null], [...events, {}], [...events, { Action: 'unknown', Package: runtimePackage }],
    [event('output'), { Action: 'output', Package: runtimePackage, Output: JSON.stringify(events) }]
  ]) assert.throws(() => check(input), /Go partition failed/)
  for (const names of [[], ['TestOne', 'TestOne'], ['TestOne/child'], [''], [null]]) {
    assert.throws(() => goTestPartition(runtimePackage, names), /Go partition failed/)
  }
  assert.throws(() => goTestPartition(runtimePackage, ['TestOne'], ['TestOther/child']), /Go partition failed/)
  for (const { test: name } of runtimeExternalDiagnostics) {
    const partition = goTestPartition(runtimePackage, [name])
    for (const value of [event('run', name), event('skip', name), event('pass')]) partition.observe(value)
    assert.throws(() => partition.assert(), /Go partition failed/)
  }
  for (const line of ['not json', '{}', 'null', '[]']) {
    const partition = goTestPartition(runtimePackage, ['TestOne', 'TestTwo'])
    events.forEach(value => partition.observeLine(JSON.stringify(value)))
    partition.observeLine(line)
    assert.throws(() => partition.assert(), /Go partition failed/)
  }
})

test('the platform receiver requires all five lifecycle faults and the complete schedule seam', () => {
  const lifecycle = 'TestRuntimeOptionalPluginOrdinaryLifecycle'
  const schedule = 'TestRuntimeHTTPHostScheduleListUsesExactContainedLoopback'
  assert.deepEqual(runtimePlatformTests, [lifecycle, schedule])
  assert.deepEqual(runtimePlatformSubtests, ['missing', 'disabled', 'incompatible', 'unauthorized', 'domain-semantic'].map(fault => `${lifecycle}/${fault}`))
  const event = (Action, Test) => ({ Action, Package: runtimePackage, ...(Test === undefined ? {} : { Test }) })
  const events = [event('run', lifecycle),
    ...runtimePlatformSubtests.flatMap(name => [event('run', name), event('pass', name)]),
    event('pass', lifecycle), event('run', schedule), event('pass', schedule), event('pass')]
  const check = input => {
    const partition = goTestPartition(runtimePackage, runtimePlatformTests, runtimePlatformSubtests)
    input.forEach(value => partition.observe(value))
    partition.assert()
  }
  check(events)
  for (const omitted of [...runtimePlatformSubtests, schedule]) {
    assert.throws(() => check(events.filter(value => value.Test !== omitted)), /Go partition failed/)
  }
})

test('root platform receiver discovers and executes ten exact ordinary-tag tests separately', () => {
  const names = [
    'TestRuntimeServerConfiguredStdioMCPToolLoopRejectsMutationWithoutHostSemanticIdentityAndContinues',
    'TestRuntimeServerCheckpointApplyBlocksStagedGitChanges',
    'TestRuntimeServerInterruptStopsLaterToolsInSameProviderStep',
    'TestRuntimeServerBashRunInBackgroundWithholdsOutputAndSupportsKill',
    'TestRuntimeServerGoalTodoCompleteStepAndFinalReadiness',
    'TestRuntimeServerCompleteStepRejectsMismatchedTodoIndexBeforeEvidence',
    'TestRuntimeServerBashApprovalDenyAndAllowControlExecution',
    'TestRuntimeServerBashReportsExitCodeAndDiagnostics',
    'TestRuntimeServerBashTimeoutKillsProcessGroupGrandchild',
    'TestRuntimeServerSideEffectIntentBlocksSecondEquivalentBashWrite'
  ]
  assert.deepEqual(rootPlatformTests, names)
  const listing = values => `${values.join('\n')}\nok\t${rootPackage}\t0.001s\n`
  assert.deepEqual(rootPlatformSelection(listing([...names, 'TestUnrelated']), ''), names)
  assert.throws(() => rootPlatformSelection(listing(names), 'analytix_prod'), /ordinary build tags/)
  assert.throws(() => rootPlatformSelection(listing(names).replace(`ok\t${rootPackage}\t`, `ok\t${runtimePackage}\t`), ''), /inventory/)
  assert.throws(() => rootPlatformSelection(listing([...names, names[0]]), ''), /inventory/)
  const event = (Action, Test) => ({ Action, Package: rootPackage, ...(Test === undefined ? {} : { Test }) })
  const events = [...names.flatMap(name => [event('run', name), event('pass', name)]), event('pass')]
  const check = input => {
    const partition = goTestPartition(rootPackage, names)
    input.forEach(value => partition.observe(value))
    partition.assert()
  }
  check(events)
  for (const name of names) {
    assert.throws(() => rootPlatformSelection(listing(names.filter(value => value !== name)), ''), /inventory/)
    assert.throws(() => check(events.filter(value => value.Test !== name)), /Go partition failed/)
    assert.throws(() => check(events.map(value => value.Test === name && value.Action === 'pass' ? event('skip', name) : value)), /Go partition failed/)
  }
  assert.throws(() => check(events.map(value => ({ ...value, Package: runtimePackage }))), /Go partition failed/)
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
  assert.match(native, /export TMPDIR="\$GOTMPDIR"/)
  assert.match(native, /npm test -- --tags-filter=macos-integration/)
  assert.doesNotMatch(native, /continue-on-error|passWithNoTests/)
  const restart = workflow.slice(workflow.indexOf('  restart-contracts:'), workflow.indexOf('  go-tests:'))
  assert.match(restart, /set -o pipefail/)
  assert.match(restart, /profile_root="\$\(cd "\$RUNNER_TEMP" && pwd -P\)"/)
  assert.match(restart, /-o "\$profile_root\/held-restart\.test"/)
  assert.match(restart, /export GOTMPDIR="\$profile_root"/)
  assert.doesNotMatch(restart, /export TMPDIR=/)
  assert.match(restart, /TestRuntimeTestExecutableIsUnpackagedAndCanonical/)
  assert.match(restart, /node \.\.\/\.\.\/scripts\/go-test-selection\.mjs/)
  assert.match(workflow, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs packages/)
  assert.match(workflow, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs runtime "\$TEST_SHARD"/)
  const matrixShards = workflow.match(/shard: \[([^\]]+)\]/)?.[1].split(',').map(value => Number(value.trim()))
  assert.deepEqual(matrixShards, Array.from({ length: runtimeShardCount }, (_, index) => index))
  const platform = workflow.slice(workflow.indexOf('  runtime-platform:'), workflow.indexOf('  backend-tests:'))
  assert.match(platform, /runs-on: macos-15/)
  assert.match(platform, /tags: \['', analytix_prod\]/)
  assert.match(platform, /run: npm ci/)
  assert.match(platform, /run: npm run build/)
  assert.match(platform, /ANALYTIX_TEST_SCHEDULE_MCP_NODE_V1=.*realpathSync\(process.execPath\)/)
  assert.match(platform, /ANALYTIX_TEST_SCHEDULE_MCP_ENTRYPOINT_V1="\$GITHUB_WORKSPACE\/out\/main\/claw-schedule-mcp-node-entry\.js"/)
  assert.match(platform, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs platform/)
  const rootPlatform = platform.slice(platform.indexOf('      - name: Execute root protected-process positives'))
  assert.match(rootPlatform, /if: \$\{\{ matrix\.tags == '' \}\}/)
  assert.match(rootPlatform, /node \.\.\/\.\.\/scripts\/go-ci-shards\.mjs platform-root/)
  assert.doesNotMatch(platform, /continue-on-error|passWithNoTests|npm run dist|ANALYTIX_TEST_SHARED_/)
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
