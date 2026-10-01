// Current routing and bounded maintenance measurements. Inventory is explicit;
// selected checks never replace CI, native, privacy or release acceptance.
import { execFileSync, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, lstatSync, readFileSync, realpathSync, readdirSync } from 'node:fs'
import { dirname, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { performance } from 'node:perf_hooks'
import { goTestPartition } from './go-test-partition.mjs'

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
let readReceipt = null
function read(relative) {
  const path = resolve(root, relative)
  if (!realpathSync(path).startsWith(root + sep) || !lstatSync(path).isFile()) {
    throw new Error('Inventory input must be a repository-local regular file.')
  }
  const value = readFileSync(path, 'utf8')
  if (readReceipt) readReceipt.push({ ordinal: readReceipt.length + 1, path: relative, source_read_bytes: Buffer.byteLength(value), sha256: createHash('sha256').update(value).digest('hex') })
  return value
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

const registryTests = [
  'TestExecutionGrantRegistryRequiresHostMembershipAndTracksApproval',
  'TestExecutionGrantRegistryTransitionsDoNotMutateAuthoritySnapshots',
  'TestExecutionGrantRegistryFailsClosedOnTamperAndReplay'
]
const scenarios = {
  'docs-only': ['docs/analytix/README.md'],
  'local-typescript': ['src/shared/gui-update-schedule.ts', 'src/shared/gui-update-schedule.test.ts'],
  'go-authority': ['packages/runtime-go/internal/domain/security/grant_registry.go', 'packages/runtime-go/internal/domain/security/grant_registry_test.go']
}

export function validationPlan(changedPaths) {
  const paths = [...new Set(changedPaths)].sort()
  const known = new Set(Object.values(scenarios).flat())
  const unknown = paths.filter(path => !known.has(path) && !/^(?:docs\/.+\.md|README(?:\.en)?\.md|AGENTS\.md)$/.test(path))
  const commands = []
  const docs = paths.filter(path => path.endsWith('.md'))
  if (docs.length) commands.push({ argv: ['git', 'diff', '--check'], cwd: '.' },
    { argv: ['node', 'scripts/validation-burden.mjs', '--check-links', ...docs], cwd: '.' })
  if (paths.some(path => scenarios['local-typescript'].includes(path))) {
    if (paths.includes('src/shared/gui-update-schedule.ts')) {
      commands.push({ argv: ['npm', 'run', 'typecheck'], cwd: '.', source: 'package.json typecheck / producer and consumer contracts' })
    }
    commands.push({ argv: ['npm', 'test', '--', '--reporter=json', 'src/shared/gui-update-schedule.test.ts'], cwd: '.', source: 'package.json test / vitest.config.ts application lane', exact_typescript_fixture: true })
  }
  if (paths.some(path => scenarios['go-authority'].includes(path))) {
    for (const tags of [[], ['-tags', 'analytix_prod']]) commands.push({
      argv: ['go', 'test', '-json', '-count=1', ...tags, '-run', '^(' + registryTests.join('|') + ')$', './internal/domain/security'],
      cwd: 'packages/runtime-go', source: '.github/workflows/ci.yml ordinary/analytix_prod matrix', exact_go_tests: registryTests
    })
  }
  return { status: paths.length && !unknown.length ? 'focused_plan' : 'unmapped', changed_paths: paths, unknown_paths: unknown,
    commands: unknown.length ? [] : commands, fallback: unknown.length ? ['Select affected owners and existing CI matrix; this tool cannot establish dependency coverage.'] : [],
    acceptance: 'local maintenance only; existing privacy/authority/architecture/platform/tag/CI/package gates remain required where applicable' }
}

export function checkRelativeLinks(paths) {
  for (const path of paths) {
    const body = read(path)
    for (const match of body.matchAll(/\]\(([^\s)]+)(?:\s+"[^"]*")?\)/g)) {
      const target = match[1].replace(/^<|>$/g, '').split('#')[0]
      if (!target || /^[a-z][a-z\d+.-]*:/i.test(target)) continue
      const absolute = resolve(root, dirname(path), decodeURIComponent(target))
      if (!absolute.startsWith(root + sep) || !existsSync(absolute)) throw new Error(`Missing repository link in ${path}: ${target}`)
    }
  }
}

function currentRoutes() {
  const previousReads = readReceipt
  const inputs = []
  readReceipt = inputs
  const entries = ['AGENTS.md', 'README.md', 'docs/analytix/README.md', 'docs/analytix/specs/README.md', 'docs/analytix/handovers/README.md']
  const entryFiles = entries.map(path => ({ path, bytes: Buffer.byteLength(read(path)) }))
  // This is a derived view, not a second specification/status store. Artifact
  // readiness and current implementation evidence are intentionally separate.
  const activeChanges = readdirSync(resolve(root, 'openspec/changes'), { withFileTypes: true })
    .filter(entry => entry.isDirectory() && entry.name !== 'archive').map(entry => {
      const path = `openspec/changes/${entry.name}/tasks.md`
      return { change: entry.name, tasks_path: path, ...derivedTaskProgress(path) }
    }).sort((a, b) => a.change.localeCompare(b.change))
  readReceipt = previousReads
  return { schema_version: 2, mode: 'current_routes', head: git('rev-parse', 'HEAD').trim(), entry_files: entryFiles,
    source_reads: inputs, source_read_bytes: inputs.reduce((n, input) => n + input.source_read_bytes, 0),
    active_changes: activeChanges, artifact_readiness_command: 'openspec status --change <selected-change> --json',
    rule: 'Read only selected target/active-change scope and real dependencies; do not recursively open QA, handovers or archives.',
    inventory_command: 'node scripts/validation-burden.mjs --inventory --json',
    plan_command: 'node scripts/validation-burden.mjs --plan <task-owned-paths> --json',
    evidence: 'Checkboxes describe task bookkeeping; neither planning artifacts nor task counts establish current implementation or acceptance.' }
}

export function derivedTaskProgress(path) {
  let body
  try { body = read(path) } catch (error) {
    if (error.code === 'ENOENT') return { task_state: 'no_tasks', completed: 0, total: 0, remaining: 0 }
    throw error
  }
  // Match the CLI checkbox grammar line by line, including * bullets. This is
  // a derived bookkeeping view; it does not infer artifact or runtime readiness.
  const tasks = body.split('\n').map(line => /^\s*[-*]\s*\[([\sxX])\]\s*(.*)/.exec(line)).filter(Boolean)
  const completed = tasks.filter(task => task[1].toLowerCase() === 'x').length
  return { task_state: 'present', completed, total: tasks.length, remaining: tasks.length - completed }
}

export function assertTypescriptFixtureExecution(stdout) {
  const marker = /\{\s*"numTotalTestSuites"\s*:/.exec(stdout)
  if (!marker) throw new Error('TypeScript fixture execution receipt is absent.')
  const report = JSON.parse(stdout.slice(marker.index).trim())
  const file = report.testResults?.[0]
  if (report.success !== true || report.numTotalTests !== 3 || report.numPassedTests !== 3 ||
    report.numFailedTests !== 0 || report.numPendingTests !== 0 || report.numTodoTests !== 0 ||
    report.testResults.length !== 1 || file.name !== resolve(root, 'src/shared/gui-update-schedule.test.ts') ||
    file.assertionResults?.length !== 3 || file.assertionResults.some(result => result.status !== 'passed')) {
    throw new Error('Every selected TypeScript fixture assertion must execute and pass without skips or todos.')
  }
}

function measureScenario(name) {
  const startedAt = new Date().toISOString()
  if (!scenarios[name]) throw new Error('Unknown fixed maintenance scenario.')
  if (process.platform === 'darwin' && !process.env.ANALYTIX_DEV_CACHE_ROOT) {
    throw new Error('Source ./scripts/use-analytix-cache.sh in the same shell before maintenance execution.')
  }
  const plan = validationPlan(scenarios[name])
  const repetitions = []
  for (let repetition = 1; repetition <= 3; repetition++) {
    const started = performance.now()
    readReceipt = []
    const guides = name === 'go-authority' ? ['packages/runtime-go/AGENTS.md'] : name === 'local-typescript' ? ['src/AGENTS.md'] : ['docs/AGENTS.md']
    const routes = ['AGENTS.md', ...guides, 'docs/analytix/README.md', 'package.json',
      'scripts/validation-burden.mjs', 'scripts/go-test-partition.mjs',
      ...(name === 'docs-only' ? [] : ['.github/workflows/ci.yml']),
      ...(name === 'local-typescript' ? ['vitest.config.ts', 'tsconfig.web.json', 'tsconfig.node.json', 'src/main/gui-updater.ts'] : name === 'go-authority' ? ['packages/runtime-go/go.mod', 'packages/runtime-go/go.sum'] : []), ...scenarios[name]]
    for (const path of routes) read(path)
    const commands = []
    let firstEffectiveValidationMs = null
    for (const command of plan.commands) {
      const began = performance.now()
      const [executable, ...args] = command.argv
      const result = spawnSync(executable, args, { cwd: resolve(root, command.cwd), encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 })
      let passed = !result.error && result.status === 0 && !result.signal
      if (passed && command.exact_go_tests) {
        const partition = goTestPartition('analytix.local/runtime-go/internal/domain/security', command.exact_go_tests)
        for (const line of result.stdout.trim().split('\n')) partition.observeLine(line)
        try { partition.assert() } catch { passed = false }
      }
      if (passed && command.exact_typescript_fixture) {
        try { assertTypescriptFixtureExecution(result.stdout) } catch { passed = false }
      }
      commands.push({ ...command, elapsed_ms: performance.now() - began, exit_code: result.status, signal: result.signal, passed,
        stdout_sha256: createHash('sha256').update(result.stdout ?? '').digest('hex') })
      if (passed && firstEffectiveValidationMs === null) firstEffectiveValidationMs = performance.now() - started
      if (!passed) throw new Error(`Maintenance check failed: ${command.argv.join(' ')}\n${(result.stderr ?? '').slice(0, 2000)}`)
    }
    const reads = readReceipt
    readReceipt = null
    repetitions.push({ repetition, reads, source_read_bytes: reads.reduce((n, r) => n + r.source_read_bytes, 0),
      repeated_reads: reads.length - new Set(reads.map(r => r.path)).size,
      navigation_edges: routes.slice(1).map((to, i) => ({ from: routes[i], to })),
      commands, first_effective_validation_ms: firstEffectiveValidationMs, total_elapsed_ms: performance.now() - started })
  }
  return { schema_version: 2, mode: 'measured_maintenance', scenario: name, started_at: startedAt, completed_at: new Date().toISOString(), head: git('rev-parse', 'HEAD').trim(), plan, repetitions,
    input_fingerprint: createHash('sha256').update(JSON.stringify(repetitions[0].reads.map(({ path, sha256 }) => ({ path, sha256 })))).digest('hex'),
    scope: 'Actual driver reads and current fixture validation, no product mutation. Rehearsal of maintenance routes; no before/after repair latency baseline.',
    limits: ['Child validator reads are not instrumented; driver source bytes are reported separately.',
      'Navigation edges are explicit driver routing, not observed human/model reading.',
      'Elapsed time uses shared local development caches without controlled cold/warm state; it does not measure model tokens/cost, general productivity, Provider quality or native acceptance.'] }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2).filter(arg => arg !== '--json')
  let value
  if (!args.length) value = currentRoutes()
  else if (args.length === 1 && args[0] === '--inventory') value = report()
  else if (args[0] === '--plan') value = validationPlan(args.slice(1))
  else if (args[0] === '--check-links' && args.length > 1) { checkRelativeLinks(args.slice(1)); value = { status: 'pass', checked_paths: args.slice(1) } }
  else if (args[0] === '--measure' && args.length === 2) value = measureScenario(args[1])
  else throw new Error('Usage: validation-burden.mjs [--inventory | --plan paths... | --measure scenario | --check-links paths...] [--json]')
  console.log(JSON.stringify(value, null, 2))
}
