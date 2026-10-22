import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { execFileSync } from 'node:child_process'
import { tools, indexSource, label, snapshot, compare, prune, eventRefs, scan, sourceFiles } from './unused-findings-gate.mjs'

const { eslint, parser } = await tools(process.cwd(), process.env.UNUSED_TOOL_ROOT ?? process.cwd())
async function fixture(code, extension = 'mjs') {
  const file = 'scripts/synthetic-unused.' + extension
  const [result] = await eslint.lintText(code, { filePath: file })
  assert.equal(result.fatalErrorCount, 0)
  const index = indexSource(code, file, parser)
  const findings = label(file, result.messages.filter(m => ['no-unused-vars', '@typescript-eslint/no-unused-vars'].includes(m.ruleId)), index)
  return { ...snapshot(findings, index.guards), allGuards: index.guards }
}
const check = (base, declared, actual) => compare(base, declared, actual, actual.allGuards)

// Actual commits exercise serialization and the trusted-baseline lifecycle, not just a variable named 'committed'.
function baselineHistory(t) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'analytix-unused-history-'))
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }))
  const git = (...args) => execFileSync('git', ['-C', dir, '-c', 'user.name=SyntheticFixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', ...args],
    { encoding: 'utf8', env: { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_NOSYSTEM: '1' }, stdio: ['ignore', 'pipe', 'pipe'] })
  git('init', '--quiet')
  const commit = (baseline, source) => {
    fs.writeFileSync(path.join(dir, 'baseline.json'), JSON.stringify(baseline), { mode: 0o600 })
    fs.writeFileSync(path.join(dir, 'source.mjs'), source, { mode: 0o600 })
    git('add', '--', 'baseline.json', 'source.mjs')
    git('commit', '--quiet', '-m', 'Synthetic baseline lifecycle')
    return JSON.parse(git('show', 'HEAD:baseline.json'))
  }
  return Object.assign(commit, { dir, git })
}

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

test('2 fixed finding regression fails after committed baseline prune lifecycle', async t => {
  const commit = baselineHistory(t)
  const original = await fixture('const debt = 1;')
  const fixed = await fixture('const debt = 1; console.log(debt);')
  const trusted = commit(original, 'const debt = 1;')
  const committed = commit(prune(trusted, fixed, fixed.allGuards), 'const debt = 1; console.log(debt);')
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

test('4 stale baseline fails until explicit prune; malformed counts/duplicates fail', async t => {
  const commit = baselineHistory(t)
  const base = await fixture('const resolved = 1; const retained = 2;')
  const head = await fixture('const resolved = 1; console.log(resolved); const retained = 2;')
  assert.throws(() => check(base, base, head), /Findings differ/)
  const ambiguous = await fixture('function owner() { run((_) => 1); run((_) => _); } owner();')
  const allFixed = await fixture('function owner() { run((_) => _); run((_) => _); } owner();')
  const emptied = prune(ambiguous, allFixed, allFixed.allGuards)
  check(ambiguous, emptied, allFixed)
  assert.throws(() => check(emptied, ambiguous, ambiguous), /Baseline expansion/)
  const trusted = commit(base, 'const resolved = 1; const retained = 2;')
  const reduced = commit(prune(trusted, head, head.allGuards), 'const resolved = 1; console.log(resolved); const retained = 2;')
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

test('6 deleting whole old scope and inserting same-signature scope elsewhere fails', async t => {
  const base = await fixture('function owner() { run((_) => "old"); run((_) => String(_)); } owner();')
  const head = await fixture('function owner() { run((_) => String(_)); run((_) => "new"); } owner();')
  assert.deepEqual(base.entries, head.entries)
  assert.equal(base.guards[0].candidates, head.guards[0].candidates)
  assert.throws(() => check(base, head, head), /Ambiguous structure changed/)
  const original = await fixture('function pick(xs){ const first=xs.filter((_,i)=>i>0); const second=xs.filter((value,i)=>value>i); return [first,second]; }')
  const moved = await fixture('function pick(xs){ const second=xs.filter((_,i)=>i>0); return [second]; }')
  const oldUnderscore = original.entries.find(e => e.signature.binding === '_')
  const newUnderscore = moved.entries.find(e => e.signature.binding === '_')
  assert.equal(original.allGuards.get(oldUnderscore.key).candidates, 1)
  assert.equal(moved.allGuards.get(newUnderscore.key).candidates, 1)
  assert.notEqual(oldUnderscore.key, newUnderscore.key)
  assert.throws(() => check(original, original, moved), /Findings differ/)
  assert.throws(() => check(original, moved, moved), /Baseline expansion/)
  const propertyOld = await fixture('export const first={x:xs.filter((_,i)=>i>0)};')
  const propertyNew = await fixture('export const second={x:xs.filter((_,i)=>i>0)};')
  assert.throws(() => check(propertyOld, propertyNew, propertyNew), /Baseline expansion/)
  const chainOld = await fixture('export function pick(xs){return xs.filter(value=>value>0).map((_,i)=>i);}')
  const chainEdited = await fixture('export function pick(xs){return xs.filter(value=>value>1).map((_,i)=>i);}')
  check(chainOld, chainOld, chainEdited)
  const assignmentOld = await fixture('function pick(xs){let first,second; first=xs.filter((_,i)=>i>0); second=xs.filter((value,i)=>value>i);return [first,second];}')
  const assignmentNew = await fixture('function pick(xs){let first,second;second=xs.filter((_,i)=>i>0);return [first,second];}')
  assert.throws(() => check(assignmentOld, assignmentOld, assignmentNew), /Findings differ/)
  assert.throws(() => check(assignmentOld, assignmentNew, assignmentNew), /Baseline expansion/)
  const cb = 'xs.filter((_,i)=>i>0)'
  const targets = [
    ['variable', target => `export const ${target}=${cb};`],
    ['property', target => `export const ${target}={x:${cb}};`],
    ['call', target => `export function probe(xs){return ${target}(${cb});}`],
    ['new', target => `export function probe(xs){return new ${target}(${cb});}`],
    ['member receiver', target => `export function probe(xs){return ${target}[${cb}];}`],
    ['conditional', target => `export function probe(xs){return ${target}?${cb}:[];}`],
    ['logical', target => `export function probe(xs){return ${target}&&${cb};}`],
    ['binary', target => `export function probe(xs){return ${target}+${cb};}`],
    ['assignment identifier', target => `export function probe(xs){let ${target};${target}=${cb};return ${target};}`],
    ['assignment static property', target => `export function probe(xs){obj.${target}=${cb};}`],
    ['assignment computed property', target => `export function probe(xs){obj["${target}"]=${cb};}`],
    ['template owner', target => 'export const '+target+'=`${'+cb+'}`;'],
    ['tag', target => 'export function probe(xs){return '+target+'`${'+cb+'}`;}'],
    ['jsx owner', target => `export const ${target}=<X>{${cb}}</X>;`, 'tsx'],
    ['jsx tag', target => `export function probe(xs){return <${target}>{${cb}}</${target}>;}`, 'tsx'],
    ['jsx attribute', target => `export function probe(xs){return <X ${target}={${cb}}/>;}`, 'tsx'],
    ['static field', target => `export class Owner {${target === 'First' ? 'static ' : ''}x=${cb};}`],
    ['static method', target => `export class Owner {${target === 'First' ? 'static ' : ''}x(_,i){return i;}}`],
    ['array owner', target => `export const ${target}=[${cb}];`],
    ['spread array owner', target => `export const ${target}=[...${cb}];`],
    ['sequence owner', target => `export const ${target}=(0,${cb});`],
    ['optional call owner', target => `export const ${target}=xs?.filter((_,i)=>i>0);`],
    ['TS wrapper owner', target => `export const ${target}=(${cb} as unknown);`, 'tsx']
  ]
  for (const [branch, source, extension] of targets) {
    const before = await fixture(source('First'), extension), after = await fixture(source('Second'), extension)
    assert.throws(() => check(before, before, after), /Findings differ/, branch)
    assert.throws(() => check(before, after, after), /Baseline expansion/, branch)
    t.diagnostic('host target branch rejected: ' + branch)
  }
})

test('7 committed source selection excludes install output, retains ignores and admits new files', async t => {
  const history = baselineHistory(t), empty = { schema: 1, entries: [], guards: [] }
  fs.writeFileSync(path.join(history.dir, 'eslint.config.js'), fs.readFileSync('eslint.config.js'), { mode: 0o600 })
  fs.mkdirSync(path.join(history.dir, 'build'))
  fs.writeFileSync(path.join(history.dir, 'build/ignored.mjs'), 'const ignoredDebt = 1;', { mode: 0o600 })
  fs.writeFileSync(path.join(history.dir, '.gitignore'), 'packages/runtime/dist/\n', { mode: 0o600 })
  history.git('add', '--', 'eslint.config.js', '.gitignore', 'build/ignored.mjs')
  history(empty, 'export const retained = 1;')
  fs.mkdirSync(path.join(history.dir, 'packages/runtime/dist'), { recursive: true })
  fs.writeFileSync(path.join(history.dir, 'packages/runtime/dist/emitted.js'), 'const emittedDebt = 1;', { mode: 0o600 })
  fs.writeFileSync(path.join(history.dir, 'untracked.mjs'), 'const uncommittedDebt = 1;', { mode: 0o600 })
  const toolRoot = process.env.UNUSED_TOOL_ROOT ?? process.cwd()
  const before = await scan(history.dir, toolRoot)
  assert.deepEqual(before.files.sort(), ['eslint.config.js', 'source.mjs'])
  check(empty, empty, before)
  fs.writeFileSync(path.join(history.dir, 'new.mjs'), 'const newlyCommittedDebt = 1;', { mode: 0o600 })
  history.git('add', '--', 'new.mjs')
  history(empty, 'export const retained = 1;')
  const after = await scan(history.dir, toolRoot)
  assert.ok(after.files.includes('new.mjs'))
  assert.equal(after.entries.length, 1)
  assert.equal(after.entries[0].signature.binding, 'newlyCommittedDebt')
  assert.throws(() => check(empty, empty, after), /Findings differ/)
  assert.throws(() => sourceFiles(history.dir, 'f'.repeat(40)), /Command failed/)
})
