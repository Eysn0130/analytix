import { execFileSync } from 'node:child_process'
import { extname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const sourceExtensions = new Set([
  '.c', '.cjs', '.cs', '.css', '.go', '.html', '.js', '.mjs', '.mts',
  '.nsh', '.ps1', '.py', '.rs', '.s', '.sh', '.ts', '.tsx', '.zsh'
])
export const sourceLineLimit = 2000

// This is an LF-byte budget, not logical lines, complexity or a deletion target.
export function countLF(bytes) {
  let lines = 0
  for (const byte of bytes) if (byte === 10) lines++
  return lines
}

export function eventComparison(env) {
  const sha = (value, label) => {
    if (!/^[a-f0-9]{40}$/i.test(value ?? '') || /^0+$/.test(value)) {
      throw new Error(`Missing or invalid ${label} commit SHA`)
    }
    return value
  }
  const head = sha(env.SOURCE_SIZE_HEAD, 'head')
  if (env.SOURCE_SIZE_EVENT === 'workflow_dispatch') {
    // Manual main verification compares its first parent; a missing parent fails.
    return { base: `${head}^1`, head }
  }
  if (!['pull_request', 'push'].includes(env.SOURCE_SIZE_EVENT)) {
    throw new Error('Use --base <commit> --head <commit> or a supported CI event')
  }
  return { base: sha(env.SOURCE_SIZE_BASE, 'base'), head }
}

export function checkSourceFileSize({ cwd = process.cwd(), base, head }) {
  const git = (...args) => execFileSync('git', args, {
    cwd, maxBuffer: 64 * 1024 * 1024, stdio: ['ignore', 'pipe', 'pipe']
  })
  const commit = (ref, label) => {
    if (typeof ref !== 'string' || !ref) throw new Error(`Missing ${label} commit`)
    try {
      return git('rev-parse', '--verify', '--end-of-options', `${ref}^{commit}`).toString().trim()
    } catch {
      throw new Error(`Cannot resolve ${label} commit: ${ref}`)
    }
  }
  base = commit(base, 'base')
  head = commit(head, 'head')
  const source = path => sourceExtensions.has(extname(path).toLowerCase())
  const blob = (ref, path) => {
    const entry = git('--literal-pathspecs', 'ls-tree', '-z', ref, '--', path).toString()
    const metadata = entry.slice(0, entry.indexOf('\t')).split(' ')
    if (!['100644', '100755'].includes(metadata[0]) || metadata[1] !== 'blob') {
      throw new Error(`Source accounting requires a regular committed blob: ${JSON.stringify(path)}`)
    }
    return { oid: metadata[2], lf: countLF(git('cat-file', 'blob', metadata[2])) }
  }
  const fields = git('diff', '--no-ext-diff', '--name-status', '-z', '--find-renames=50%',
    '--diff-filter=AMRT', base, head, '--').toString().split('\0')
  fields.pop()
  const violations = [], existingOversize = []
  let checked = 0
  for (let index = 0; index < fields.length;) {
    const status = fields[index++]
    const oldPath = fields[index++]
    const renamed = /^R\d+$/.test(status)
    const path = renamed ? fields[index++] : oldPath
    if (!path || (!renamed && !['A', 'M', 'T'].includes(status))) {
      throw new Error('Unexpected Git change inventory')
    }
    if (!source(path)) continue
    const after = blob(head, path)
    checked++
    if (after.lf <= sourceLineLimit) continue
    const before = status === 'A' ? null : blob(base, oldPath)
    const entry = { path, oldPath: status === 'A' ? null : oldPath,
      beforeLF: before?.lf ?? null, afterLF: after.lf, status }
    if (status === 'M' || (renamed && source(oldPath) &&
      before.lf > sourceLineLimit && before.oid === after.oid)) {
      existingOversize.push(entry)
    } else {
      violations.push({ ...entry, reason: renamed
        ? 'Oversize renamed/newly classified source: review its base path and changed bytes; only unchanged existing large source renames are exempt.'
        : 'New source exceeds 2000 LF; generated/third-party material needs explicit reviewed handling, not an implicit exemption.' })
    }
  }
  return { base, head, limitLF: sourceLineLimit, checked, violations, existingOversize,
    scope: 'New source files and detected renames, not a ratchet over existing modifications.',
    renameDetection: 'Git --find-renames=50% uses similarity; missed renames are treated as additions.' }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2)
    let range
    if (!args.length) range = eventComparison(process.env)
    else if (args.length === 4 && args[0] === '--base' && args[2] === '--head') {
      range = { base: args[1], head: args[3] }
    } else throw new Error('Usage: source-file-size-gate.mjs --base <commit> --head <commit>')
    const result = checkSourceFileSize(range)
    console.log(JSON.stringify(result, null, 2))
    process.exitCode = result.violations.length ? 1 : 0
  } catch (error) {
    console.error(JSON.stringify({ status: 'failed', reason: error.message }))
    process.exitCode = 1
  }
}
