#!/usr/bin/env node

import { readFile, readdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)))
const agentsSkillsRoot = resolve(repoRoot, '.agents/skills')
const codexSkillsRoot = resolve(repoRoot, '.codex/skills')
const expectedSkillNames = [
  'openspec-apply-change',
  'openspec-archive-change',
  'openspec-explore',
  'openspec-propose',
  'openspec-sync-specs',
  'openspec-update-change'
]
const expectedSkillSet = new Set(expectedSkillNames)
const bannedReferences = [
  ['AskUserQuestion', /\bAskUserQuestion\b/u],
  ['TodoWrite', /\bTodoWrite\b/u],
  ['Task tool', /\bTask tool\b/iu],
  ['Skill tool', /\bSkill tool\b/iu],
  ['legacy /opsx command', /\/opsx(?::|-)/u],
  ['legacy .codex/prompts path', /\.codex\/prompts/u]
]
const findings = []

async function directoryNames(root) {
  try {
    return (await readdir(root, { withFileTypes: true }))
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name)
      .sort()
  } catch (error) {
    if (error?.code === 'ENOENT') return []
    throw error
  }
}

function recordSetDifference(label, actual, expected) {
  for (const name of actual.filter((value) => !expected.includes(value))) {
    findings.push(`${label}:unexpected:${name}`)
  }
  for (const name of expected.filter((value) => !actual.includes(value))) {
    findings.push(`${label}:missing:${name}`)
  }
}

function parseFrontmatter(content, skillName) {
  const match = content.match(/^---\n([\s\S]*?)\n---\n/u)
  if (!match) {
    findings.push(`${skillName}:frontmatter:missing`)
    return new Map()
  }

  const values = new Map()
  for (const line of match[1].split('\n').filter(Boolean)) {
    const field = line.match(/^([a-z][a-z0-9_-]*):\s*(.+)$/u)
    if (!field) {
      findings.push(`${skillName}:frontmatter:invalid-line`)
      continue
    }
    values.set(field[1], field[2])
  }
  return values
}

function parseInterfaceMetadata(content, skillName) {
  const lines = content.split('\n').filter(Boolean)
  if (lines.shift() !== 'interface:') {
    findings.push(`${skillName}:metadata:missing-interface`)
  }

  const values = new Map()
  for (const line of lines) {
    const field = line.match(/^ {2}(display_name|short_description|default_prompt): "([^"]+)"$/u)
    if (!field) {
      findings.push(`${skillName}:metadata:invalid-line`)
      continue
    }
    values.set(field[1], field[2])
  }
  return values
}

const agentDirectories = await directoryNames(agentsSkillsRoot)
const actualOpenSpecSkills = agentDirectories.filter((name) => name.startsWith('openspec-'))
recordSetDifference('canonical-skills', actualOpenSpecSkills, expectedSkillNames)

for (const name of agentDirectories.filter((value) => value.startsWith('source-command-opsx-'))) {
  findings.push(`legacy-source-command-skill:${name}`)
}

for (const name of (await directoryNames(codexSkillsRoot)).filter((value) => value.startsWith('openspec-'))) {
  findings.push(`duplicate-codex-skill:${name}`)
}

for (const skillName of expectedSkillNames) {
  const skillRoot = resolve(agentsSkillsRoot, skillName)
  let content
  let metadataContent
  try {
    ;[content, metadataContent] = await Promise.all([
      readFile(resolve(skillRoot, 'SKILL.md'), 'utf8'),
      readFile(resolve(skillRoot, 'agents/openai.yaml'), 'utf8')
    ])
  } catch (error) {
    findings.push(`${skillName}:required-file:${error?.code ?? 'read-failed'}`)
    continue
  }

  const frontmatter = parseFrontmatter(content, skillName)
  recordSetDifference(
    `${skillName}:frontmatter-keys`,
    [...frontmatter.keys()].sort(),
    ['description', 'name']
  )
  if (frontmatter.get('name') !== skillName) findings.push(`${skillName}:frontmatter:name`)
  const description = frontmatter.get('description') ?? ''
  if (description.length === 0 || description.length > 1024) {
    findings.push(`${skillName}:frontmatter:description-length`)
  }
  if (content.split('\n').length > 500) findings.push(`${skillName}:skill-too-long`)
  if (!content.includes('1.6.0')) findings.push(`${skillName}:minimum-cli-version`)

  for (const [label, pattern] of bannedReferences) {
    if (pattern.test(content)) findings.push(`${skillName}:banned-reference:${label}`)
  }
  for (const reference of content.matchAll(/\$(openspec-[a-z0-9-]+)/gu)) {
    if (!expectedSkillSet.has(reference[1])) {
      findings.push(`${skillName}:unknown-skill-reference:${reference[1]}`)
    }
  }

  const metadata = parseInterfaceMetadata(metadataContent, skillName)
  recordSetDifference(
    `${skillName}:metadata-keys`,
    [...metadata.keys()].sort(),
    ['default_prompt', 'display_name', 'short_description']
  )
  const displayName = metadata.get('display_name') ?? ''
  const shortDescription = metadata.get('short_description') ?? ''
  const defaultPrompt = metadata.get('default_prompt') ?? ''
  if (displayName.length === 0 || displayName.length > 64) {
    findings.push(`${skillName}:metadata:display-name-length`)
  }
  if (shortDescription.length < 25 || shortDescription.length > 64) {
    findings.push(`${skillName}:metadata:short-description-length`)
  }
  if (!defaultPrompt.includes(`$${skillName}`)) {
    findings.push(`${skillName}:metadata:default-prompt-reference`)
  }
}

const archiveSkill = await readFile(
  resolve(agentsSkillsRoot, 'openspec-archive-change/SKILL.md'),
  'utf8'
)
if (!archiveSkill.includes('openspec archive "<name>" --yes --json')) {
  findings.push('openspec-archive-change:canonical-command')
}
if (/```bash[\s\S]*\b(?:mkdir|mv)\s/iu.test(archiveSkill)) {
  findings.push('openspec-archive-change:manual-filesystem-archive')
}

if (findings.length > 0) {
  console.error('OpenSpec Codex skill verification failed:')
  for (const finding of [...new Set(findings)].sort()) console.error(`- ${finding}`)
  process.exit(1)
}

console.log('OpenSpec Codex skill verification passed.')
console.log(`canonical_skills=${expectedSkillNames.length}`)
console.log('legacy_or_duplicate_skills=0')
console.log('banned_references=0')
