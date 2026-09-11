import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export const publicSourcePolicy = JSON.parse(readFileSync(new URL('./public-source-policy.json', import.meta.url), 'utf8'))
const zero = '0'.repeat(40)
const oid = /^[0-9a-f]{40}$/

export function gitRead(cwd, args, input) {
  return execFileSync('git', args, {
    cwd,
    input,
    encoding: 'utf8',
    maxBuffer: 32 * 1024 * 1024,
    env: { ...process.env, GIT_NO_REPLACE_OBJECTS: '1' },
    stdio: ['pipe', 'pipe', 'pipe']
  })
}

export function assertPublicHistory(cwd, commit, policy = publicSourcePolicy) {
  const roots = gitRead(cwd, ['rev-list', '--max-parents=0', commit]).trim().split('\n')
  if (roots.length !== 1 || roots[0] !== policy.publicRoot) {
    throw new Error('Push refused: history is not solely descended from the public baseline. Do not merge or push private archive history; use a reviewed patch on the public branch. Shallow clones must fetch full history first.')
  }
}

export function isPrivateSourcePath(name, policy = publicSourcePolicy) {
  if (policy.excludedPaths.includes(name)) return true
  const base = name.split('/').at(-1)
  if (/^\.env(?:$|\.)/.test(base) && !/^\.env(?:\.[^.]+)*\.example$/.test(base)) return true
  return /\.(?:p12|pfx|p8|pem|key|keychain-db)$/i.test(base)
}

// Check the actual objects being pushed, including an excluded file added and
// deleted in separate commits. This does not scan or print credential values.
function checkPublicUpdates(input, { cwd = process.cwd(), policy = publicSourcePolicy } = {}) {
  const candidates = new Map()
  let updates = 0
  for (const line of input.split(/\r?\n/).filter(Boolean)) {
    const fields = line.trim().split(/\s+/)
    if (fields.length !== 4 || !oid.test(fields[1]) || !oid.test(fields[3]) || !/^refs\/(heads|tags)\//.test(fields[2])) {
      throw new Error('Push refused: unsupported ref update.')
    }
    const [, local, remoteRef, remote] = fields
    if (local === zero) {
      if (remoteRef === 'refs/heads/main') throw new Error('Push refused: deleting main is not a normal synchronization operation.')
      continue
    }
    const commit = gitRead(cwd, ['rev-parse', '--verify', `${local}^{commit}`]).trim()
    assertPublicHistory(cwd, commit, policy)
    let base = policy.publicRoot
    if (remote !== zero) {
      try {
        base = gitRead(cwd, ['rev-parse', '--verify', `${remote}^{commit}`]).trim()
        gitRead(cwd, ['merge-base', '--is-ancestor', base, commit])
      } catch {
        throw new Error('Push refused: fetch and reconcile the remote branch first. Non-fast-forward updates are not enabled by this workflow.')
      }
    }
    const commits = gitRead(cwd, ['rev-list', commit, `^${base}`]).trim().split('\n').filter(Boolean)
    for (const revision of commits) {
      const entries = gitRead(cwd, ['diff-tree', '--no-commit-id', '--root', '-m', '-r', '--raw', '-z', '--no-abbrev', '--no-renames', '--diff-filter=ACMT', revision]).split('\0')
      for (let index = 0; index < entries.length - 1; index += 2) {
        const metadata = entries[index].match(/^:\d{6} \d{6} [0-9a-f]{40} ([0-9a-f]{40}) [ACMT]$/)
        if (!metadata) throw new Error('Push refused: could not inspect a changed source entry.')
        const name = entries[index + 1]
        if (isPrivateSourcePath(name, policy)) throw new Error(`Push refused: excluded local file ${JSON.stringify(name)} is present in an outgoing commit.`)
        candidates.set(metadata[1], name)
      }
    }
    updates++
  }
  if (candidates.size) {
    const objects = gitRead(cwd, ['cat-file', '--batch-check=%(objectname) %(objecttype) %(objectsize)'], [...candidates.keys()].join('\n') + '\n')
    for (const line of objects.trim().split('\n')) {
      const [hash, kind, bytes] = line.split(' ')
      if (kind !== 'blob' || !Number.isSafeInteger(Number(bytes))) throw new Error('Push refused: changed content is not an inspectable source blob.')
      if (Number(bytes) > policy.maxBlobBytes) throw new Error(`Push refused: ${JSON.stringify(candidates.get(hash))} exceeds the public Git blob size limit.`)
    }
  }
  return { updates, checkedObjects: candidates.size }
}

export function checkPublicPush(input, options) {
  const result = checkPublicUpdates(input, options)
  for (const line of input.split(/\r?\n/).filter(Boolean)) {
    const [, local, remoteRef] = line.trim().split(/\s+/)
    if (remoteRef === 'refs/heads/main' && local !== zero) {
      throw new Error('Push refused: main is PR-only. Push a codex/* branch, complete CI/acceptance, then merge the PR without bypassing checks.')
    }
  }
  return result
}

// CI has no pre-push hook invocation. Inspect its real candidate, not just the
// hook's unit tests; include the tip even when the baseline itself is the tip.
export function checkPublicCandidate({ cwd = process.cwd(), revision = 'HEAD', policy = publicSourcePolicy } = {}) {
  const tip = gitRead(cwd, ['rev-parse', '--verify', `${revision}^{commit}`]).trim()
  // Inspect a candidate without attempting the now-forbidden direct main push.
  const result = checkPublicUpdates(`refs/heads/main ${tip} refs/heads/main ${zero}\n`, { cwd, policy })
  let files = 0
  for (const entry of gitRead(cwd, ['ls-tree', '-r', '-l', '-z', tip]).split('\0').filter(Boolean)) {
    const match = entry.match(/^\d{6} (\w+) [a-f0-9]{40}\s+(\d+|-)\t([\s\S]+)$/)
    if (!match || match[1] !== 'blob') throw new Error('Public candidate contains an uninspectable source entry.')
    const [, , size, name] = match
    if (isPrivateSourcePath(name, policy)) throw new Error(`Public candidate contains excluded local file ${JSON.stringify(name)}.`)
    if (Number(size) > policy.maxBlobBytes) throw new Error(`Public candidate exceeds the blob size limit: ${JSON.stringify(name)}.`)
    files++
  }
  return { ...result, files }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const args = process.argv.slice(2)
    if (args.length === 1 && args[0] === '--candidate') {
      const result = checkPublicCandidate()
      console.log(`PASS public ancestry, excluded paths and blob limits: ${result.files} current files; ${result.checkedObjects} post-baseline objects. Not a content secret scan.`)
    } else if (args.length === 0 || (args.length === 2 && !args[0].startsWith('--'))) {
      // Git supplies remote name and location to pre-push. The policy consumes
      // ref updates on stdin; do not log or otherwise use remote credential data.
      checkPublicPush(readFileSync(0, 'utf8'))
    } else throw new Error('Push refused: unsupported inspection arguments.')
  } catch (error) {
    console.error(error instanceof Error && error.message.startsWith('Push refused:')
      ? error.message : 'Push refused: public history inspection failed. Fetch full history and check the local Git configuration.')
    process.exitCode = 1
  }
}
