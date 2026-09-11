#!/usr/bin/env node

import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { spawnSync } from 'node:child_process'

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)))
const provenancePath = 'docs/analytix/history-rewrite-provenance-2026-07-10.json'
const manifestPath = 'docs/analytix/upstreams/upstream-sources.json'
const retiredPaths = [
  ['windows-qa-credential-note', ['docs', 'analytix', 'qa', `win-lan-${'remote-control'}.md`].join('/')],
  ['privacy-sensitive-financial-report', '资金研判报告.md']
]
const hashPattern = /^[0-9a-f]{40}$/u
const findings = []

function git(args) {
  const result = spawnSync('git', args, {
    cwd: repoRoot,
    encoding: 'utf8',
    maxBuffer: 32 * 1024 * 1024
  })
  if (result.status !== 0) {
    console.error(`git ${args[0]} failed during repository history verification`)
    process.exit(2)
  }
  return result.stdout
}

function objectExists(spec) {
  return spawnSync('git', ['cat-file', '-e', spec], {
    cwd: repoRoot,
    stdio: 'ignore'
  }).status === 0
}

function retainedRefContainsPath(targetPath) {
  const refs = git(['for-each-ref', '--format=%(refname)']).split('\n').filter(Boolean)
  if (refs.length === 0) return false
  const result = spawnSync('git', ['cat-file', '--batch-check'], {
    cwd: repoRoot,
    encoding: 'utf8',
    input: `${refs.map((ref) => `${ref}:${targetPath}`).join('\n')}\n`,
    maxBuffer: 32 * 1024 * 1024
  })
  if (result.status !== 0) {
    console.error('git cat-file failed during exact retained-ref path verification')
    process.exit(2)
  }
  return result.stdout.split('\n').some((line) => line && !line.endsWith(' missing'))
}

for (const [label, targetPath] of retiredPaths) {
  if (existsSync(resolve(repoRoot, targetPath))) findings.push(`${label}:worktree`)
  if (git(['ls-files', '--cached', '--', targetPath]).trim()) findings.push(`${label}:index`)
  if (git(['log', '--all', '--format=%H', '--', targetPath]).trim()) {
    findings.push(`${label}:commit-history`)
  }
  if (retainedRefContainsPath(targetPath)) findings.push(`${label}:retained-ref`)
}

const provenance = JSON.parse(readFileSync(resolve(repoRoot, provenancePath), 'utf8'))
const manifest = JSON.parse(readFileSync(resolve(repoRoot, manifestPath), 'utf8'))
const retainedObjectIds = new Set(
  git(['rev-list', '--objects', '--all'])
    .split('\n')
    .filter(Boolean)
    .map((line) => line.split(' ', 1)[0])
)
if (provenance.schemaVersion !== 1) findings.push('provenance:schema-version')
if (provenance.scope !== 'analytix-local-commit-identifiers-only') {
  findings.push('provenance:scope')
}
if (!Array.isArray(provenance.mappings) || provenance.mappings.length !== 17) {
  findings.push('provenance:mapping-count')
}

const mappings = new Map()
for (const entry of provenance.mappings ?? []) {
  const before = entry?.preRewriteCommit
  const after = entry?.postRewriteCommit
  if (!hashPattern.test(before ?? '') || !hashPattern.test(after ?? '') || before === after) {
    findings.push('provenance:invalid-mapping')
    continue
  }
  if (mappings.has(before)) findings.push('provenance:duplicate-source')
  mappings.set(before, after)
  if (objectExists(`${before}^{commit}`)) findings.push('provenance:pre-rewrite-object-retained')
  if (!objectExists(`${after}^{commit}`)) findings.push('provenance:post-rewrite-object-missing')
  if (!retainedObjectIds.has(after)) findings.push('provenance:post-rewrite-object-unreachable')
}

if (manifest.analytixHistoryRewriteMap !== provenancePath) {
  findings.push('manifest:provenance-path')
}
if (mappings.get(manifest.analytixPreRewriteCommit) !== manifest.analytixCommit) {
  findings.push('manifest:analytix-commit-mapping')
}
if (!objectExists(`${manifest.analytixCommit}^{commit}`)) {
  findings.push('manifest:analytix-commit-missing')
}

if (findings.length > 0) {
  console.error('Repository history hygiene verification failed; values are intentionally redacted.')
  for (const finding of [...new Set(findings)].sort()) console.error(finding)
  process.exit(1)
}

console.log('Repository history hygiene verification passed.')
console.log(`retired_paths_checked=${retiredPaths.length}`)
console.log(`local_provenance_mappings=${mappings.size}`)
console.log('secret_or_private_values_printed=no')
