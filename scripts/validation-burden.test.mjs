import test from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import { resolve } from 'node:path'
import { validationPlan, derivedTaskProgress, assertTypescriptFixtureExecution } from './validation-burden.mjs'

test('unmapped executable paths cannot borrow a narrow maintenance plan', () => {
  for (const path of ['src/shared/other.ts', 'packages/runtime-go/internal/app/other.go', 'docs/fixture.json', '../README.md']) {
    const plan = validationPlan([path])
    assert.equal(plan.status, 'unmapped')
    assert.deepEqual(plan.unknown_paths, [path])
    assert.ok(plan.fallback.length)
  }
  assert.equal(validationPlan([]).status, 'unmapped')
})

test('authority maintenance retains ordinary and production-tag checks', () => {
  const plan = validationPlan(['packages/runtime-go/internal/domain/security/grant_registry.go'])
  assert.equal(plan.status, 'focused_plan')
  assert.equal(plan.commands.length, 2)
  assert.ok(plan.commands[1].argv.includes('analytix_prod'))
  for (const command of plan.commands) assert.equal(command.exact_go_tests.length, 3)
})

test('TypeScript producer maintenance checks consumers and tests-only scope stays bounded', () => {
  const producer = validationPlan(['src/shared/gui-update-schedule.ts'])
  assert.equal(producer.status, 'focused_plan')
  assert.ok(producer.commands.some(command => command.argv.join(' ') === 'npm run typecheck'))
  assert.equal(producer.commands.filter(command => command.exact_typescript_fixture).length, 1)
  const testsOnly = validationPlan(['src/shared/gui-update-schedule.test.ts'])
  assert.equal(testsOnly.commands.length, 1)
  assert.equal(testsOnly.commands[0].exact_typescript_fixture, true)
  const mixed = validationPlan(['src/shared/gui-update-schedule.ts', 'src/main/unknown.ts'])
  assert.equal(mixed.status, 'unmapped')
  assert.deepEqual(mixed.commands, [])
})

test('default routing derives active status without historical inputs', () => {
  const result = spawnSync(process.execPath, ['scripts/validation-burden.mjs', '--json'], { encoding: 'utf8' })
  assert.equal(result.status, 0, result.stderr)
  const value = JSON.parse(result.stdout)
  assert.equal(value.mode, 'current_routes')
  assert.ok(Array.isArray(value.active_changes))
  for (const entry of value.active_changes) {
    assert.equal(entry.remaining, entry.total - entry.completed)
    assert.ok(!entry.tasks_path.includes('/archive/'))
  }
  assert.ok(value.entry_files.every(entry => !entry.path.includes('/qa/')))
})


test('incomplete active-change planning does not block the default routing gate', () => {
  assert.deepEqual(derivedTaskProgress('openspec/changes/stage1-nonexistent/tasks.md'), { task_state: 'no_tasks', completed: 0, total: 0, remaining: 0 });
  assert.throws(() => derivedTaskProgress('package.json/invalid'), /ENOTDIR/);
});

test('mixed unknown scopes cannot reuse a fixture plan and skipped TS receipts fail', () => {
  const plan = validationPlan(['src/shared/gui-update-schedule.ts', 'src/main/unknown.ts']);
  assert.equal(plan.status, 'unmapped');
  assert.deepEqual(plan.commands, []);
  assert.throws(() => assertTypescriptFixtureExecution('{}'), /absent/);
  assert.throws(() => assertTypescriptFixtureExecution(JSON.stringify({ numTotalTestSuites: 1, success: true, numTotalTests: 3, numPassedTests: 0, numFailedTests: 0, numPendingTests: 3, numTodoTests: 0, testResults: [] })), /execute and pass/);
});


function typescriptFixtureReceipt(name) {
  return {
    numTotalTestSuites: 1, success: true, numTotalTests: 3, numPassedTests: 3,
    numFailedTests: 0, numPendingTests: 0, numTodoTests: 0,
    testResults: [{ name, assertionResults: Array.from({ length: 3 }, () => ({ status: 'passed' })) }]
  }
}

test('Windows fixture receipts match the slash-normalized expected absolute owner', () => {
  const expected = 'C:\\work\\analytix\\src\\shared\\gui-update-schedule.test.ts'
  assert.doesNotThrow(() => assertTypescriptFixtureExecution(JSON.stringify(typescriptFixtureReceipt(
    resolve('src/shared/gui-update-schedule.test.ts').replaceAll('\\', '/')
  ))))
  assert.doesNotThrow(() => assertTypescriptFixtureExecution(JSON.stringify(typescriptFixtureReceipt(
    expected.replaceAll('\\', '/')
  )), expected))
})

test('fixture receipts retain exact owner, unique file and complete passed assertions', () => {
  const expected = 'C:\\work\\analytix\\src\\shared\\gui-update-schedule.test.ts'
  for (const change of [
    report => { report.testResults[0].name = 'C:/other/src/shared/gui-update-schedule.test.ts' },
    report => { report.testResults.push(report.testResults[0]) },
    report => { report.numPassedTests = 2 },
    report => { report.numFailedTests = 1 },
    report => { report.numPendingTests = 1 },
    report => { report.numTodoTests = 1 },
    report => { report.testResults[0].assertionResults.pop() },
    report => { report.testResults[0].assertionResults[1].status = 'failed' },
    report => { report.testResults[0].assertionResults[1].status = 'skipped' },
    report => { report.testResults[0].assertionResults[1].status = 'pending' },
    report => { report.testResults[0].assertionResults[1].status = 'todo' }
  ]) {
    const report = typescriptFixtureReceipt(expected.replaceAll('\\', '/'))
    change(report)
    assert.throws(() => assertTypescriptFixtureExecution(JSON.stringify(report), expected), /execute and pass/)
  }
})
