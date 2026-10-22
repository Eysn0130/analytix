import fs from 'node:fs'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'
import { execFileSync } from 'node:child_process'

export const SOURCE = '2c5494642b35f7cdb9cd27939486beec7d5adaaf'
export const RAW_SHA = 'dff6e88701aabb1d26794c84cb8661d04084b89181f5513ca1cbcdda1bc4458c'
export const BASELINE = 'scripts/unused-findings-baseline.json'
export const CHECKER = 'scripts/unused-findings-gate.mjs'
const CONFIG = 'eslint.config.js'
const BOOTSTRAP_SHA = '5f77ca5c94e7e826d68f048f3425cf5f5873f35749275983cdf9aeac32432825'
export const digest = value => createHash('sha256').update(value).digest('hex')
const json = value => JSON.stringify(value)
const options = { vars: 'all', args: 'all', caughtErrors: 'all', ignoreRestSiblings: false }
const rules = ['no-unused-vars', '@typescript-eslint/no-unused-vars']
const git = (root, ...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', maxBuffer: 32 * 1024 * 1024 })
const read = (root, file) => fs.readFileSync(path.join(root, file), 'utf8')

export async function tools(root, toolRoot = root) {
  const req = createRequire(path.join(toolRoot, 'package.json'))
  const load = async name => import(pathToFileURL(req.resolve(name)).href)
  const { ESLint } = await load('eslint')
  const parser = await load('@typescript-eslint/parser')
  let config = read(root, CONFIG)
  for (const name of ['@eslint/js', 'globals', 'eslint-plugin-react-hooks', 'typescript-eslint']) {
    config = config.replace(`from '${name}'`, `from '${pathToFileURL(req.resolve(name)).href}'`)
  }
  const actual = (await import('data:text/javascript,' + encodeURIComponent(config))).default
  const disabled = actual.map(c => ({ ...c, ...(c.rules ? { rules: Object.fromEntries(Object.keys(c.rules).map(r => [r, 'off'])) } : {}) }))
  const eslint = new ESLint({ cwd: root, overrideConfigFile: true, cache: false, fix: false,
    allowInlineConfig: false, overrideConfig: [...disabled,
      { files: ['**/*.{js,mjs,cjs,ts,tsx}'], rules: { 'no-unused-vars': ['error', options] } },
      { files: ['**/*.{ts,tsx}'], rules: { 'no-unused-vars': 'off', '@typescript-eslint/no-unused-vars': ['error', options] } }
    ] })
  return { eslint, parser }
}

// No location or sibling ordinal participates in identity. Tokens omit comments/whitespace.
export function indexSource(input, file, parser) {
  const parsed = parser.parseForESLint(input, { filePath: file, loc: true, range: true, tokens: true,
    comment: false, ecmaVersion: 'latest', sourceType: 'module', ecmaFeatures: { jsx: true }, project: false })
  const parents = new WeakMap(), functionBodies = []
  function walk(node, parent) {
    if (!node || typeof node !== 'object' || typeof node.type !== 'string') return
    if (parent) parents.set(node, parent)
    if (/Function|Class/.test(node.type) && node.body?.range) functionBodies.push(node.body.range)
    for (const [key, value] of Object.entries(node)) {
      if (['parent', 'loc', 'range', 'tokens', 'comments'].includes(key)) continue
      if (Array.isArray(value)) value.forEach(n => walk(n, node))
      else walk(value, node)
    }
  }
  walk(parsed.ast)
  const tokens = parsed.ast.tokens
  const normRange = (start, end) => tokens.filter(t => t.range[0] >= start && t.range[1] <= end).map(t => [t.type, t.value])
  const norm = node => node?.range ? normRange(...node.range) : []
  // A callee may contain earlier callbacks in a chain; retain syntax, not their bodies.
  const skeleton = node => {
    if (!node?.range) return []
    const bodies = functionBodies.filter(([start, end]) => start >= node.range[0] && end <= node.range[1])
    return tokens.filter(t => t.range[0] >= node.range[0] && t.range[1] <= node.range[1]
      && !bodies.some(([start, end]) => t.range[0] >= start && t.range[1] <= end)).map(t => [t.type, t.value])
  }
  function hostPath(node) {
    const out = []
    for (let child = node, p = parents.get(child); p; child = p, p = parents.get(p)) {
      if (p.type === 'VariableDeclarator' && p.init === child) return [...out, ['binding', norm(p.id)]]
      if (['Property', 'MethodDefinition', 'PropertyDefinition'].includes(p.type)) {
        out.push(['property', skeleton(p.key), p.computed ?? false, p.kind ?? null]); continue
      }
      if (['CallExpression', 'NewExpression'].includes(p.type)) {
        out.push([p.type, skeleton(p.callee), p.arguments.find(a => a.type === 'Literal' && typeof a.value === 'string')?.value ?? null,
          p.callee === child ? 'callee' : p.arguments.indexOf(child)]); continue
      }
      if (p.type === 'MemberExpression') {
        out.push([p.type, p.object === child ? 'object' : 'property', skeleton(p.property), p.computed]); continue
      }
      if (p.type === 'ConditionalExpression') {
        out.push([p.type, p.test === child ? 'test' : p.consequent === child ? 'consequent' : 'alternate', skeleton(p.test)]); continue
      }
      if (/Function|Class|Statement$|^Program$/.test(p.type)) break
      if (/Expression$|^TS|^SpreadElement$/.test(p.type)) {
        out.push([p.type, p.operator ?? null, p.left === child ? 'left' : p.right === child ? 'right' : null]); continue
      }
      break
    }
    return out
  }
  function anchor(node) {
    const p = parents.get(node)
    const header = normRange(node.range[0], node.body?.range?.[0] ?? node.range[1])
    if (node.id?.name) return [node.type, node.id.name, header]
    if (node.type === 'BlockStatement' && p && p.type !== 'Program') return [node.type, p.type, normRange(p.range[0], node.range[0])]
    return [node.type, header, hostPath(node)]
  }
  const positions = new Map(), candidates = new Map()
  for (const scope of parsed.scopeManager.scopes) for (const variable of scope.variables) {
    const def = variable.defs[0]
    if (!def) continue
    const category = def.type === 'Parameter' ? 'parameter' : def.type === 'CatchClause' ? 'caught_error' : 'variable'
    const chain = []
    for (let s = scope; s && s.type !== 'global'; s = s.upper) {
      if (s.block.type !== 'Program') chain.unshift([s.type, anchor(s.block)])
    }
    let importOwner = def.name
    while (importOwner && importOwner.type !== 'ImportDeclaration') importOwner = parents.get(importOwner)
    const declaration = { type: def.type, kind: def.parent?.kind ?? null,
      pattern: norm(def.node.id ?? def.name), importSource: importOwner ? norm(importOwner.source) : null }
    const signature = { path: file, rule: /\.tsx?$/.test(file) ? rules[1] : rules[0], binding: variable.name, category, declaration, scope: chain }
    const key = digest(json(signature))
    let host = def.name
    for (let n = def.name; n && n.type !== 'Program'; n = parents.get(n)) {
      host = n
      if (parents.get(n)?.type === 'BlockStatement' || parents.get(n)?.type === 'Program') break
    }
    const row = { key, signature, host, variable }
    const group = candidates.get(key) ?? []
    group.push(row); candidates.set(key, group)
    for (const n of [...variable.identifiers, ...variable.references.map(r => r.identifier)]) {
      if (n.loc) positions.set(`${n.loc.start.line}:${n.loc.start.column + 1}`, row)
    }
  }
  const guards = new Map()
  for (const [key, group] of candidates) {
    const start = Math.min(...group.map(g => g.host.range[0]))
    const end = Math.max(...group.map(g => g.host.range[1]))
    guards.set(key, { key, candidates: group.length, structure: digest(json(normRange(start, end))) })
  }
  return { positions, guards }
}

export function label(file, messages, index) {
  return messages.map(message => {
    const row = index.positions.get(`${message.line}:${message.column}`)
    if (!row || row.signature.rule !== message.ruleId) throw Error(`Unmapped finding: ${file}:${message.line}:${message.column}`)
    return { key: row.key, signature: row.signature, line: message.line, column: message.column }
  })
}
export function snapshot(findings, allGuards) {
  const map = new Map()
  for (const f of findings) {
    const row = map.get(f.key) ?? { key: f.key, signature: f.signature, count: 0 }
    row.count++; map.set(f.key, row)
  }
  const entries = [...map.values()].sort((a, b) => a.key.localeCompare(b.key))
  const guards = entries.map(e => allGuards.get(e.key)).filter(g => g?.candidates > 1).sort((a, b) => a.key.localeCompare(b.key))
  return { schema: 1, entries, guards }
}
function validate(value) {
  if (value.schema !== 1 || !Array.isArray(value.entries) || !Array.isArray(value.guards)) throw Error('Invalid baseline schema')
  const seen = new Set()
  for (const row of value.entries) {
    if (seen.has(row.key) || row.key !== digest(json(row.signature)) || !Number.isSafeInteger(row.count) || row.count < 1) throw Error('Invalid baseline entry')
    seen.add(row.key)
  }
  const guards = new Set()
  for (const row of value.guards) {
    if (guards.has(row.key) || !seen.has(row.key) || !Number.isSafeInteger(row.candidates) || row.candidates < 2 || !/^[a-f0-9]{64}$/.test(row.structure)) throw Error('Invalid ambiguity guard')
    guards.add(row.key)
  }
}
export function compare(trusted, declared, actual, allGuards) {
  for (const value of [trusted, declared, actual]) validate(value)
  const base = new Map(trusted.entries.map(e => [e.key, e.count]))
  for (const row of declared.entries) if (row.count > (base.get(row.key) ?? 0)) throw Error(`Baseline expansion: ${row.key}`)
  const counts = value => value.entries.map(e => [e.key, e.count]).sort((a, b) => a[0].localeCompare(b[0]))
  if (json(counts(declared)) !== json(counts(actual))) throw Error('Findings differ: explicitly prune stale entries; never adopt new findings')
  // Include used sibling bindings: equal unused counts cannot hide a swap or deleted scope.
  const guards = new Map([...trusted.guards, ...actual.guards].map(g => [g.key, g]))
  for (const [key, guard] of guards) {
    if (!actual.entries.some(e => e.key === key)) continue // Explicit prune to zero carries no inherited allowance.
    const old = trusted.guards.find(g => g.key === key)
    const current = allGuards.get(key)
    if (!old || !current || json(old) !== json(current)) throw Error(`Ambiguous structure changed; explicit review required: ${guard.key}`)
  }
  if (json(declared.guards) !== json(actual.guards)) throw Error('Declared ambiguity guards differ')
}
export function encodeBaseline(value) {
  const rows = items => '[\n' + items.map(item => '    ' + json(item)).join(',\n') + '\n  ]'
  return '{\n' + Object.entries(value).map(([key, item]) => {
    const encoded = ['entries', 'guards'].includes(key) ? rows(item)
      : key === 'audit' ? '{"note":' + json(item.note) + ',"rows":' + rows(item.rows) + ',"old_ambiguities":' + rows(item.old_ambiguities) + '}'
        : json(item)
    return '  ' + json(key) + ': ' + encoded
  }).join(',\n') + '\n}\n'
}
export function prune(trusted, actual, allGuards) {
  compare(trusted, actual, actual, allGuards)
  return { ...trusted, entries: actual.entries, guards: actual.guards }
}

export async function scan(root, toolRoot) {
  const { eslint, parser } = await tools(root, toolRoot)
  const results = await eslint.lintFiles(['.'])
  const findings = [], guards = new Map(), files = []
  for (const result of results) {
    files.push(path.relative(root, result.filePath).split(path.sep).join('/'))
    if (result.messages.some(m => m.fatal)) throw Error(`Parse failure: ${result.filePath}`)
    const messages = result.messages.filter(m => rules.includes(m.ruleId))
    if (!messages.length) continue
    const file = path.relative(root, result.filePath).split(path.sep).join('/')
    const index = indexSource(fs.readFileSync(result.filePath, 'utf8'), file, parser)
    findings.push(...label(file, messages, index))
    for (const [key, value] of index.guards) guards.set(key, value)
  }
  return { ...snapshot(findings, guards), allGuards: guards, files }
}
export function verifyTools(root, toolRoot, policy) {
  if (digest(read(root, CONFIG)) !== policy.config_sha256) throw Error('ESLint config drift requires separate reviewed tooling migration')
  const lock = JSON.parse(read(root, 'package-lock.json'))
  const resolve = (from, name, exists) => {
    for (let at = from; ; at = path.posix.dirname(at)) {
      const rel = (at && at !== '.' ? at + '/' : '') + 'node_modules/' + name
      if (exists(rel)) return rel
      if (!at || at === '.') return null
    }
  }
  const pending = ['eslint', '@eslint/js', 'globals', 'eslint-plugin-react-hooks', 'typescript-eslint', '@typescript-eslint/parser']
    .map(name => resolve('', name, rel => Boolean(lock.packages[rel])))
  const visited = new Set()
  while (pending.length) {
    const rel = pending.pop()
    if (!rel) throw Error('Missing tool root')
    if (visited.has(rel)) continue
    visited.add(rel)
    const pkg = lock.packages[rel]
    for (const name of new Set([...Object.keys(pkg.dependencies ?? {}), ...Object.keys(pkg.optionalDependencies ?? {}), ...Object.keys(pkg.peerDependencies ?? {})])) {
      const locked = resolve(rel, name, p => Boolean(lock.packages[p]))
      const installed = resolve(rel, name, p => fs.existsSync(path.join(toolRoot, p, 'package.json')))
      if (locked !== installed) throw Error(`Tool resolution drift: ${rel} -> ${name}`)
      if (locked) pending.push(locked)
      else if (!(name in (pkg.optionalDependencies ?? {})) && !pkg.peerDependenciesMeta?.[name]?.optional) throw Error(`Missing tool dependency: ${name}`)
    }
  }
  if (json([...visited].sort()) !== json(Object.keys(policy.closure).sort())) throw Error('Tool closure changed')
  for (const [rel, expected] of Object.entries(policy.closure)) {
    if (digest(json(lock.packages[rel])) !== expected.lock_sha256) throw Error(`Tool lock drift: ${rel}`)
    const installed = JSON.parse(read(toolRoot, `${rel}/package.json`))
    if (installed.version !== expected.version || digest(read(toolRoot, `${rel}/package.json`)) !== expected.metadata_sha256) throw Error(`Installed tool drift: ${rel}`)
  }
}
export function eventRefs(env) {
  const event = env.UNUSED_EVENT, base = env.UNUSED_BASE, head = env.UNUSED_HEAD
  if (!['pull_request', 'push', 'workflow_dispatch'].includes(event) || !/^[a-f0-9]{40}$/.test(base ?? '') || !/^[a-f0-9]{40}$/.test(head ?? '') || /^0+$/.test(base) || /^0+$/.test(head)) throw Error('Missing trusted event base/head')
  return { base, head }
}
async function main() {
  const root = process.cwd(), toolRoot = process.env.UNUSED_TOOL_ROOT ?? root
  const { base, head } = eventRefs(process.env)
  if (git(root, 'rev-parse', 'HEAD').trim() !== head) throw Error('Checkout must be exact event head, not synthetic merge')
  const mode = git(root, 'ls-tree', head, '--', BASELINE).split(' ')[0]
  if (!['100644', '100755'].includes(mode) || !fs.lstatSync(path.join(root, BASELINE)).isFile()) throw Error('Baseline must be a regular committed file')
  const bytes = git(root, 'show', `${head}:${BASELINE}`), declared = JSON.parse(bytes)
  if (read(root, BASELINE) !== bytes) throw Error('Working baseline differs from event head')
  if (git(root, 'diff', '--name-only', head).trim()) throw Error('Working source differs from event head')
  const baseFiles = git(root, 'ls-tree', '--name-only', base, '--', BASELINE).trim()
  let trusted
  if (!baseFiles) {
    if (base !== SOURCE || digest(bytes) !== BOOTSTRAP_SHA) throw Error('Unapproved or missing bootstrap baseline')
    trusted = declared
  } else trusted = JSON.parse(git(root, 'show', `${base}:${BASELINE}`))
  if (json(trusted.policy) !== json(declared.policy)) throw Error('Baseline policy drift')
  verifyTools(root, toolRoot, trusted.policy)
  const actual = await scan(root, toolRoot)
  if (process.argv.includes('--prune')) {
    const next = prune(trusted, actual, actual.allGuards)
    fs.writeFileSync(path.join(root, BASELINE), encodeBaseline(next))
    console.log('Pruned baseline; inspect and commit reductions explicitly')
  } else compare(trusted, declared, actual, actual.allGuards)
  console.log(json({ status: 'pass', files: actual.files.length, identities: actual.entries.length,
    occurrences: actual.entries.reduce((n, e) => n + e.count, 0), ambiguity_groups: actual.guards.length }))
}
if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch(error => { console.error(error.message); process.exitCode = 1 })
}
