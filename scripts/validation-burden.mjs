// Read-only source/entry inventory using Node's standard library.
// Static counts never select, skip, or run tests.
import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { lstatSync, readFileSync, realpathSync } from 'node:fs'
import { dirname, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const domains = {
  go: ['packages/runtime-go/', ['.go']],
  desktop: ['src/', ['.ts', '.tsx']],
  runtime_ts: ['packages/runtime/', ['.ts', '.tsx']],
  python: ['backend/', ['.py']],
  rust: ['tools/', ['.rs']]
}
const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024 })
const paths = output => output.split('\0').filter(Boolean).sort()
function read(relative) {
  const path = resolve(root, relative)
  if (!realpathSync(path).startsWith(root + sep) || !lstatSync(path).isFile()) {
    throw new Error('Inventory input must be a repository-local regular file.')
  }
  return readFileSync(path, 'utf8')
}
function testFile(path) {
  if (path.endsWith('.go')) return path.endsWith('_test.go')
  if (/\.tsx?$/.test(path)) return /\.(test|spec)\.tsx?$/.test(path)
  if (path.endsWith('.py')) return /(?:^|\/)test_[^/]+$|_test\.py$/.test(path)
  return /#\[(?:test|tokio::test)/.test(read(path))
}
function globRegex(pattern) {
  if (/[!\[\]()\\]/.test(pattern)) throw new Error('Unknown Vitest pattern.')
  let expression = ''
  for (let i = 0; i < pattern.length;) {
    if (pattern.startsWith('**/', i)) { expression += '(?:.*/)?'; i += 3 }
    else if (pattern.startsWith('**', i)) { expression += '.*'; i += 2 }
    else if (pattern[i] === '*') { expression += '[^/]*'; i++ }
    else if (pattern[i] === '{') {
      const end = pattern.indexOf('}', i)
      if (end < 0 || !/^[a-zA-Z0-9]+(?:,[a-zA-Z0-9]+)*$/.test(pattern.slice(i + 1, end))) {
        throw new Error('Unknown Vitest brace pattern.')
      }
      expression += '(?:' + pattern.slice(i + 1, end).split(',').join('|') + ')'
      i = end + 1
    } else { expression += pattern[i].replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); i++ }
  }
  return new RegExp('^' + expression + '$')
}
function vitestFiles(config, prefix, files) {
  const match = /\binclude\s*:\s*\[([^\]]+)\]/.exec(read(config))
  const literal = /['"]([^'"]+)['"]/g
  if (!match || match[1].replace(literal, '').replaceAll(',', '').trim()) {
    throw new Error('Nonliteral Vitest include; discovery is unknown.')
  }
  const patterns = [...match[1].matchAll(literal)].map(m => globRegex(prefix + m[1]))
  return new Set(files.filter(path => patterns.some(pattern => pattern.test(path))))
}

function report() {
  const baseline = JSON.parse(read('docs/analytix/validation-burden-baseline.json'))
  if (baseline.schema_version !== 1) throw new Error('Unsupported burden baseline schema.')
  const tracked = paths(git('ls-files', '-z'))
  const untracked = paths(git('ls-files', '--others', '--exclude-standard', '-z'))
  const present = tracked.filter(path => {
    try { return lstatSync(resolve(root, path)).isFile() } catch { return false }
  })
  const inventory = {}
  for (const [name, [prefix, extensions]] of Object.entries(domains)) {
    const files = present.filter(p => p.startsWith(prefix) && extensions.some(e => p.endsWith(e)))
    inventory[name] = {
      source_files: files.length, test_files: files.filter(testFile).length, extensions,
      test_detection: name === 'rust' ? 'files containing Rust test attributes' : 'test filename convention'
    }
  }
  const goFiles = present.filter(p => p.startsWith('packages/runtime-go/') && p.endsWith('.go'))
  let overlap
  try {
    const app = vitestFiles('vitest.config.ts', '', present)
    const runtime = vitestFiles('packages/runtime/vitest.config.ts', 'packages/runtime/', present)
    overlap = { status: 'static_only', root_files: app.size, runtime_files: runtime.size,
      overlapping_files: [...app].filter(p => runtime.has(p)).length }
  } catch {
    overlap = { status: 'unknown', required_selection: 'full' }
  }
  const entries = baseline.entries.map(row => {
    if (!present.includes(row.path)) throw new Error('A baseline entry is missing from the tracked inventory.')
    const bytes = Buffer.byteLength(read(row.path))
    return { path: row.path, bytes, baseline_bytes: row.bytes, delta_bytes: bytes - row.bytes }
  })
  const sizes = new Map(entries.map(row => [row.path, row]))
  const tasks = baseline.matched_reads.map(task => {
    const rows = task.paths.map(path => sizes.get(path))
    return { id: task.id, paths: task.paths, domains: task.domains,
      declared_read_bytes: rows.reduce((n, r) => n + r.bytes, 0),
      baseline_read_bytes: rows.reduce((n, r) => n + r.baseline_bytes, 0),
      first_effective_validation_seconds: null }
  })
  const fingerprints = names => names.map(path => ({ path,
    sha256: createHash('sha256').update(read(path)).digest('hex') }))
  const scopeFiles = fingerprints(baseline.scope_paths)
  return {
    schema_version: 1, head: git('rev-parse', 'HEAD').trim(), baseline_head: baseline.source_head,
    inventory_scope: 'tracked working-tree files', tracked_paths: tracked.length, untracked_paths: untracked.length,
    domains: inventory, go_directories: new Set(goFiles.map(p => p.slice(0, p.lastIndexOf('/')))).size,
    go_test_fuzz_example_symbols: goFiles.filter(p => p.endsWith('_test.go')).reduce((n, p) =>
      n + [...read(p).matchAll(/^func\s+(?:Test|Fuzz|Example)\w*\(/gm)].length, 0),
    markdown_paths: present.filter(p => /\.mdx?$/.test(p)).length,
    vitest_entry_overlap: overlap, entries, matched_reads: tasks,
    declared_loaded_instructions: fingerprints(baseline.loaded_instruction_paths),
    scope_fingerprint: { files: scopeFiles,
      sha256: createHash('sha256').update(JSON.stringify(scopeFiles)).digest('hex') },
    report_bytes: Buffer.byteLength(read('scripts/validation-burden.mjs')),
    selection_shadow: { mode: 'report_only', dependency_coverage: 'unknown',
      changed_paths: new Set([...paths(git('diff', 'HEAD', '--name-only', '-z')), ...untracked]).size,
      required_selection: 'existing_full_gates' },
    limits: ['Path/symbol counts are not executable discovery or a deletion list.',
      'Static entry overlap does not prove repeated CI execution.',
      'Declared full-file reads are not measured model tokens, latency, correctness or cost.',
      'Existing test discovery, shards and acceptance thresholds are unchanged; verify:baseline prefixes this report.']
  }
}

const args = process.argv.slice(2)
if (args.length > 1 || args.some(arg => arg !== '--json')) throw new Error('Usage: node scripts/validation-burden.mjs [--json]')
const value = report()
if (args.includes('--json')) console.log(JSON.stringify(value, null, 2))
else {
  console.log(`Validation burden (read-only): HEAD ${value.head}`)
  console.log(`Tracked paths ${value.tracked_paths}; untracked paths ${value.untracked_paths}`)
  for (const [domain, counts] of Object.entries(value.domains)) {
    console.log(`${domain}: ${counts.source_files} source paths / ${counts.test_files} test paths (${counts.test_detection})`)
  }
  console.log(`Vitest entry overlap: ${JSON.stringify(value.vitest_entry_overlap)}`)
  for (const task of value.matched_reads) console.log(`${task.id}: declared read ${task.baseline_read_bytes} -> ${task.declared_read_bytes} bytes`)
  console.log('Selection remains the existing full gates; dependency coverage is unknown.')
}
