import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, renameSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { checkSourceFileSize, countLF, eventComparison, sourceExtensions } from './source-file-size-gate.mjs'

function fixture(t, files = {}) {
  const cwd = mkdtempSync(join(tmpdir(), 'analytix-source-size-'))
  t.after(() => rmSync(cwd, { recursive: true, force: true }))
  const env = { ...process.env, GIT_CONFIG_NOSYSTEM: '1', GIT_CONFIG_GLOBAL: '/dev/null', GIT_TERMINAL_PROMPT: '0' }
  const git = (...args) => execFileSync('git', args, { cwd, env, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  git('init', '-b', 'main')
  git('config', 'user.name', 'Synthetic Source Size Test')
  git('config', 'user.email', 'source-size@example.invalid')
  const write = (path, value) => {
    mkdirSync(dirname(join(cwd, path)), { recursive: true })
    writeFileSync(join(cwd, path), value)
  }
  const commit = () => {
    git('add', '--all')
    git('-c', 'commit.gpgsign=false', 'commit', '-m', 'Synthetic source size fixture')
    return git('rev-parse', 'HEAD')
  }
  write('README.md', 'synthetic\n')
  for (const [path, value] of Object.entries(files)) write(path, value)
  const base = commit()
  return { cwd, git, write, commit, base, check: head => checkSourceFileSize({ cwd, base, head }) }
}

const lines = n => Array.from({ length: n }, (_, i) => `synthetic line ${i}\n`).join('')

test('LF byte budget accepts 2000, rejects 2001 and does not add a missing final LF', t => {
  assert.equal(countLF(Buffer.from('a\r\nb\nc')), 2)
  const f = fixture(t)
  f.write('limit.ts', lines(2000))
  f.write('unterminated.ts', lines(2000) + 'last line has no LF')
  f.write('too-large.ts', lines(2001))
  const result = f.check(f.commit())
  assert.equal(result.checked, 3)
  assert.deepEqual(result.violations.map(v => [v.path, v.afterLF]), [['too-large.ts', 2001]])
})

test('all 18 extensions include tests, generated and third-party paths without hidden exemptions', t => {
  assert.deepEqual([...sourceExtensions].sort(), ['.c', '.cjs', '.cs', '.css', '.go', '.html', '.js', '.mjs', '.mts', '.nsh', '.ps1', '.py', '.rs', '.s', '.sh', '.ts', '.tsx', '.zsh'])
  const f = fixture(t)
  for (const ext of sourceExtensions) f.write(`tests/generated/third-party/file${ext.toUpperCase()}`, lines(2001))
  f.write('notes.md', lines(2001))
  const result = f.check(f.commit())
  assert.equal(result.checked, 18)
  assert.equal(result.violations.length, 18)
})

test('unchanged old large source rename is recorded, while non-source to source rename is blocked', t => {
  const f = fixture(t, { 'old.ts': lines(2001), 'old.txt': lines(2001) + 'different\n' })
  renameSync(join(f.cwd, 'old.ts'), join(f.cwd, 'renamed.ts'))
  renameSync(join(f.cwd, 'old.txt'), join(f.cwd, 'promoted.ts'))
  const result = f.check(f.commit())
  assert.deepEqual(result.existingOversize.map(v => [v.oldPath, v.path]), [['old.ts', 'renamed.ts']])
  assert.deepEqual(result.violations.map(v => [v.oldPath, v.path]), [['old.txt', 'promoted.ts']])
})

test('rewritten detected rename reports base path, before/after LF and review reason', t => {
  const f = fixture(t, { 'old.ts': lines(2001) })
  renameSync(join(f.cwd, 'old.ts'), join(f.cwd, 'renamed.ts'))
  f.write('renamed.ts', lines(2002))
  const result = f.check(f.commit())
  const [violation] = result.violations
  assert.match(violation.status, /^R/)
  assert.deepEqual([violation.oldPath, violation.path, violation.beforeLF, violation.afterLF], ['old.ts', 'renamed.ts', 2001, 2002])
  assert.match(violation.reason, /review its base path/)
})

test('a small source cannot become an oversize source through rename', t => {
  const f = fixture(t, { 'old.ts': lines(1999) })
  renameSync(join(f.cwd, 'old.ts'), join(f.cwd, 'renamed.ts'))
  f.write('renamed.ts', lines(2001))
  const [violation] = f.check(f.commit()).violations
  assert.match(violation.status, /^R/)
  assert.deepEqual([violation.beforeLF, violation.afterLF], [1999, 2001])
})

test('copy and unrecognized rewrite cannot bypass the new-file limit', t => {
  const f = fixture(t, { 'old.ts': lines(2001), 'kept.ts': 'retained source\n'.repeat(2001) })
  f.git('rm', 'old.ts')
  f.write('replacement.ts', 'entirely different\n'.repeat(2001))
  f.write('copy.ts', 'retained source\n'.repeat(2001))
  const result = f.check(f.commit())
  assert.deepEqual(result.violations.map(v => [v.path, v.status]), [['copy.ts', 'A'], ['replacement.ts', 'A']])
  assert.equal(result.existingOversize.length, 0)
})

test('committed comparisons preserve dirty worktrees, unusual paths and existing modifications', t => {
  const f = fixture(t, { 'existing.ts': lines(2001) })
  const path = 'new space\tand\nline.ts'
  f.write(path, lines(2001))
  f.write('existing.ts', lines(2002))
  const head = f.commit()
  f.write(path, 'dirty small\n')
  f.write('untracked.ts', lines(2001))
  const before = f.git('status', '--porcelain')
  const result = f.check(head)
  assert.equal(result.violations[0].path, path)
  assert.equal(result.violations[0].afterLF, 2001)
  assert.equal(result.existingOversize[0].afterLF, 2002)
  assert.equal(f.git('status', '--porcelain'), before)
})

test('missing commits and unsupported source symlinks fail instead of using an empty base or local target', t => {
  const f = fixture(t)
  assert.throws(() => checkSourceFileSize({ cwd: f.cwd, base: 'missing-base', head: f.base }), /Cannot resolve base/)
  assert.throws(() => checkSourceFileSize({ cwd: f.cwd, base: '', head: f.base }), /Missing base/)
  assert.throws(() => checkSourceFileSize({ cwd: f.cwd, base: f.base, head: 'missing-head' }), /Cannot resolve head/)
  symlinkSync('not-a-local-source', join(f.cwd, 'link.ts'))
  assert.throws(() => f.check(f.commit()), /regular committed blob/)
})

test('CI ranges use event SHAs, reject shell strings and require a real manual first parent', t => {
  const base = 'a'.repeat(40), head = 'b'.repeat(40)
  for (const event of ['pull_request', 'push']) {
    assert.deepEqual(eventComparison({ SOURCE_SIZE_EVENT: event, SOURCE_SIZE_BASE: base, SOURCE_SIZE_HEAD: head }), { base, head })
  }
  for (const bad of ['', '0'.repeat(40), 'main; injected', '--help']) {
    assert.throws(() => eventComparison({ SOURCE_SIZE_EVENT: 'pull_request', SOURCE_SIZE_BASE: bad, SOURCE_SIZE_HEAD: head }), /base commit SHA/)
    assert.throws(() => eventComparison({ SOURCE_SIZE_EVENT: 'pull_request', SOURCE_SIZE_BASE: base, SOURCE_SIZE_HEAD: bad }), /head commit SHA/)
  }
  const f = fixture(t)
  const range = eventComparison({ SOURCE_SIZE_EVENT: 'workflow_dispatch', SOURCE_SIZE_HEAD: f.base })
  assert.throws(() => checkSourceFileSize({ cwd: f.cwd, ...range }), /Cannot resolve base/)
  f.write('new.ts', 'synthetic\n')
  const headWithParent = f.commit()
  const validManual = eventComparison({ SOURCE_SIZE_EVENT: 'workflow_dispatch', SOURCE_SIZE_HEAD: headWithParent })
  assert.equal(checkSourceFileSize({ cwd: f.cwd, ...validManual }).base, f.base)
  assert.throws(() => eventComparison({ SOURCE_SIZE_EVENT: 'unknown', SOURCE_SIZE_HEAD: head }), /supported CI event/)
})
