import test from 'node:test'
import assert from 'node:assert/strict'
import { tools, indexSource, label, snapshot, compare, prune, eventRefs } from './unused-findings-gate.mjs'

const { eslint, parser } = await tools(process.cwd(), process.env.UNUSED_TOOL_ROOT ?? process.cwd())
async function fixture(code) {
  const file = 'scripts/synthetic-unused.mjs'
  const [result] = await eslint.lintText(code, { filePath: file })
  assert.equal(result.fatalErrorCount, 0)
  const index = indexSource(code, file, parser)
  const findings = label(file, result.messages.filter(m => m.ruleId === 'no-unused-vars'), index)
  return { ...snapshot(findings, index.guards), allGuards: index.guards }
}
const check = (base, declared, actual) => compare(base, declared, actual, actual.allGuards)

test('1 delete old/add new fails even if a PR edits its baseline; prune never adds', async () => {
  const base = await fixture('const oldBinding = 1;')
  const head = await fixture('const newBinding = 1;')
  assert.equal(base.entries.length, head.entries.length)
  assert.throws(() => check(base, base, head), /Findings differ/)
  assert.throws(() => check(base, head, head), /Baseline expansion/)
  assert.throws(() => prune(base, head, head.allGuards), /Baseline expansion/)
  const sameName = await fixture('const stale = 1;')
  const replacement = await fixture('function stale() {}')
  assert.throws(() => check(sameName, replacement, replacement), /Baseline expansion/)
  const sha = 'a'.repeat(40)
  assert.throws(() => eventRefs({ UNUSED_EVENT: 'pull_request', UNUSED_HEAD: sha }), /Missing trusted/)
  assert.throws(() => eventRefs({ UNUSED_EVENT: 'push', UNUSED_BASE: '0'.repeat(40), UNUSED_HEAD: sha }), /Missing trusted/)
})

test('2 fixed finding regression fails after committed baseline prune lifecycle', async () => {
  const original = await fixture('const debt = 1;')
  const fixed = await fixture('const debt = 1; console.log(debt);')
  const committed = prune(original, fixed, fixed.allGuards)
  check(committed, committed, fixed)
  assert.throws(() => check(committed, original, original), /Baseline expansion/)
  assert.throws(() => check(committed, committed, original), /Findings differ/)
})

test('3 pure line shift passes including ambiguous anonymous bindings', async () => {
  const code = 'function outer() { run((_) => 1); run((_) => _); } outer();'
  const base = await fixture(code)
  const head = await fixture('\n\n// position only\n' + code + '\n')
  assert.equal(base.guards.length, 1)
  check(base, base, head)
  assert.deepEqual(base.entries, head.entries)
})

test('4 stale baseline fails until explicit prune; malformed counts/duplicates fail', async () => {
  const base = await fixture('const resolved = 1; const retained = 2;')
  const head = await fixture('const resolved = 1; console.log(resolved); const retained = 2;')
  assert.throws(() => check(base, base, head), /Findings differ/)
  const ambiguous = await fixture('function owner() { run((_) => 1); run((_) => _); } owner();')
  const allFixed = await fixture('function owner() { run((_) => _); run((_) => _); } owner();')
  const emptied = prune(ambiguous, allFixed, allFixed.allGuards)
  check(ambiguous, emptied, allFixed)
  assert.throws(() => check(emptied, ambiguous, ambiguous), /Baseline expansion/)
  const reduced = prune(base, head, head.allGuards)
  assert.equal(reduced.entries.length, base.entries.length - 1)
  check(base, reduced, head)
  assert.throws(() => check(base, { ...base, entries: [...base.entries, base.entries[0]] }, head), /Invalid baseline/)
  assert.throws(() => check(base, { ...base, entries: [{ ...base.entries[0], count: 0 }] }, head), /Invalid baseline/)
})

test('5 two anonymous scopes each bind underscore: swapping used/unused fails closed', async () => {
  const base = await fixture('function owner() { run((_) => 1); run((_) => _); } owner();')
  const head = await fixture('function owner() { run((_) => _); run((_) => 1); } owner();')
  assert.deepEqual(base.entries, head.entries)
  assert.equal(base.guards[0].candidates, 2)
  assert.throws(() => check(base, head, head), /Ambiguous structure changed/)
})

test('6 deleting whole old scope and inserting same-signature scope elsewhere fails', async () => {
  const base = await fixture('function owner() { run((_) => "old"); run((_) => String(_)); } owner();')
  const head = await fixture('function owner() { run((_) => String(_)); run((_) => "new"); } owner();')
  assert.deepEqual(base.entries, head.entries)
  assert.equal(base.guards[0].candidates, head.guards[0].candidates)
  assert.throws(() => check(base, head, head), /Ambiguous structure changed/)
})
