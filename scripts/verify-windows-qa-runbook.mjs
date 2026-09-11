#!/usr/bin/env node

import { existsSync, readFileSync } from 'node:fs'
import { extname, resolve } from 'node:path'
import { StringDecoder } from 'node:string_decoder'
import { fileURLToPath } from 'node:url'
import { spawn, spawnSync } from 'node:child_process'

const repoRoot = resolve(fileURLToPath(new URL('..', import.meta.url)))
const runbookPath = 'docs/analytix/qa/windows-qa-operator-runbook.md'
const retiredRunbookPath = ['docs', 'analytix', 'qa', `win-lan-${'remote-control'}.md`].join('/')
const requireHistoryClean = process.argv.includes('--require-history-clean')
const requireIndexed = process.argv.includes('--require-indexed') || requireHistoryClean
const supportedArgs = new Set(['--require-history-clean', '--require-indexed'])
const unknownArgs = process.argv.slice(2).filter((arg) => !supportedArgs.has(arg))

if (unknownArgs.length > 0) {
  console.error(`unsupported argument count: ${unknownArgs.length}; values are intentionally redacted`)
  process.exit(2)
}

function git(args) {
  const result = spawnSync('git', args, {
    cwd: repoRoot,
    encoding: 'utf8',
    maxBuffer: 32 * 1024 * 1024
  })
  if (result.status !== 0) {
    console.error(`git ${args[0]} failed with exit code ${result.status ?? 'unknown'}`)
    process.exit(2)
  }
  return result.stdout
}

async function scanGitBlobs(entries) {
  if (entries.length === 0) return
  await new Promise((resolveScan, rejectScan) => {
    const child = spawn('git', ['cat-file', '--batch'], {
      cwd: repoRoot,
      stdio: ['pipe', 'pipe', 'ignore']
    })
    let entryIndex = 0
    let state = 'header'
    let headerParts = []
    let remaining = 0
    let scanner = null
    let failed = false

    const fail = () => {
      if (failed) return
      failed = true
      child.kill()
      rejectScan(new Error('retained-object stream scan failed'))
    }

    child.stdout.on('data', (value) => {
      if (failed || !Buffer.isBuffer(value)) return
      let offset = 0
      while (offset < value.length && !failed) {
        if (state === 'header') {
          const headerEnd = value.indexOf(10, offset)
          if (headerEnd < 0) {
            headerParts.push(value.subarray(offset))
            break
          }
          headerParts.push(value.subarray(offset, headerEnd))
          const header = Buffer.concat(headerParts).toString('ascii').split(' ')
          headerParts = []
          if (entryIndex >= entries.length || header.length !== 3) {
            fail()
            break
          }
          const objectSize = Number(header[2])
          if (!Number.isSafeInteger(objectSize) || objectSize < 0) {
            fail()
            break
          }
          remaining = objectSize
          scanner = header[1] === 'blob'
            ? createWindowsAccessStreamScanner(entries[entryIndex][1], true)
            : null
          state = 'content'
          offset = headerEnd + 1
          continue
        }

        if (state === 'content') {
          const size = Math.min(remaining, value.length - offset)
          if (size > 0) scanner?.writeBuffer(value.subarray(offset, offset + size))
          remaining -= size
          offset += size
          if (remaining === 0) {
            scanner?.end()
            scanner = null
            state = 'delimiter'
          }
          continue
        }

        if (value[offset] !== 10) {
          fail()
          break
        }
        offset += 1
        entryIndex += 1
        state = 'header'
      }
    })
    child.on('error', fail)
    child.stdin.on('error', fail)
    child.on('close', (code) => {
      if (failed) return
      if (
        code !== 0 ||
        entryIndex !== entries.length ||
        state !== 'header' ||
        headerParts.length !== 0
      ) {
        fail()
        return
      }
      resolveScan()
    })
    child.stdin.end(`${entries.map(([objectId]) => objectId).join('\n')}\n`)
  }).catch(() => {
    console.error('git cat-file failed while scanning retained text objects')
    process.exit(2)
  })
}

function nulPaths(args) {
  return git(args)
    .split('\0')
    .filter(Boolean)
}

function isGeneratedOrHistorical(path) {
  return (
    path === 'output' ||
    path.startsWith('output/') ||
    path === 'validation-evidence' ||
    path.startsWith('validation-evidence/') ||
    path === 'managed-chrome' ||
    path.startsWith('managed-chrome/') ||
    path === 'vendor' ||
    path.startsWith('vendor/') ||
    path === 'docs/legacy' ||
    path.startsWith('docs/legacy/') ||
    path === 'release/legacy' ||
    path.startsWith('release/legacy/') ||
    path === 'packages/runtime-go/output' ||
    path.startsWith('packages/runtime-go/output/') ||
    /^dist(?:-|\/|$)/u.test(path)
  )
}

const textExtensions = new Set([
  '.cjs',
  '.env',
  '.js',
  '.json',
  '.md',
  '.mjs',
  '.ps1',
  '.sh',
  '.toml',
  '.ts',
  '.tsx',
  '.txt',
  '.yaml',
  '.yml'
])

function isTextCandidate(path) {
  return textExtensions.has(extname(path).toLowerCase()) || /(?:^|\/)\.env(?:\.|$)/u.test(path)
}

function lineNumberAt(text, offset) {
  let line = 1
  for (let index = 0; index < offset; index += 1) {
    if (text.charCodeAt(index) === 10) line += 1
  }
  return line
}

const findings = []
const findingKeys = new Set()

function addFinding(path, rule, line, history = false) {
  const key = history ? `history:${path}:${rule}` : `worktree:${path}:${line}:${rule}`
  if (findingKeys.has(key)) return
  findingKeys.add(key)
  findings.push({ path, rule, line })
}

function addHistoryFinding(path, rule) {
  addFinding(path, rule, undefined, true)
}

function scanPattern(path, text, rule, pattern) {
  pattern.lastIndex = 0
  let match
  while ((match = pattern.exec(text)) !== null) {
    addFinding(path, rule, lineNumberAt(text, match.index))
    if (match[0].length === 0) pattern.lastIndex += 1
  }
}

const windowsAccessContextPattern = /(?:rustdesk|\brdp\b|remote[- ]?(?:control|desktop|access)|windows[- ]?qa|windows\s+(?:host|operator)|远控|远程桌面|Windows\s*主机)/iu
const remoteControlIdentifierPattern = /(?:rustdesk|remote[- ]?control|远控)[^\n]{0,96}(?:\bID\b|identifier|编号)\s*(?:[:=|]\s*|\s+)\d{5,}/giu
const credentialAssignmentPatterns = [
  /\b(?:password|passcode|access\s*code|token|api[-_ ]?key)\b\s*(?::|=|\||\bis\b)\s*["'`]?\s*([^\s,，;；"'`|]+)/giu,
  /(?:密码|口令|访问码|永久访问码)\s*(?::|=|：|\||为)\s*["'`]?\s*([^\s,，;；"'`|]+)/gu
]
const safeCredentialMarkers = new Set([
  'none',
  'never',
  'not',
  'must',
  'pending',
  'placeholder',
  'redacted',
  'rotated',
  'stored',
  'unchanged',
  'unavailable'
])

function isWindowsAccessAssociatedPath(path) {
  const normalized = path.toLowerCase()
  return (
    path === runbookPath ||
    path === retiredRunbookPath ||
    /(?:^|[/_.-])(?:win|windows?|win32|win64|rdp|rustdesk)(?:[/_.-]|$)/u.test(normalized) ||
    normalized.includes('remote-control') ||
    normalized.includes('remote_access')
  )
}

function isWindowsCredentialDocumentPath(path) {
  return isWindowsAccessAssociatedPath(path) && /\.(?:md|txt)$/iu.test(path)
}

function looksLikeLiveCredential(value) {
  const normalized = value.trim().replace(/^["'`]+|["'`.,。]+$/gu, '')
  if (normalized.length < 4) return false
  if (/^[<[{(*$%]/u.test(normalized)) return false
  const lower = normalized.toLowerCase()
  if (safeCredentialMarkers.has(lower)) return false
  if (/^(?:redacted|placeholder)(?:[-_].*)?$/u.test(lower)) return false
  return true
}

function isRegexLiteralSourceLine(path, line) {
  if (!/\.(?:cjs|js|mjs|ts|tsx)$/iu.test(path)) return false
  const trimmed = line.trim()
  return trimmed.startsWith('/') && /\/[dgimsuvy]*,?$/u.test(trimmed)
}

function scanWindowsAccessLine(path, line, lineNumber, hasWindowsAccessContext, history) {
  remoteControlIdentifierPattern.lastIndex = 0
  if (remoteControlIdentifierPattern.test(line)) {
    if (history) addHistoryFinding(path, 'remote-control-identifier-in-retained-history')
    else addFinding(path, 'remote-control-identifier', lineNumber)
  }

  if (!isWindowsCredentialDocumentPath(path) && !hasWindowsAccessContext) return
  if (isRegexLiteralSourceLine(path, line)) return

  for (const pattern of credentialAssignmentPatterns) {
    pattern.lastIndex = 0
    let match
    while ((match = pattern.exec(line)) !== null) {
      if (looksLikeLiveCredential(match[1] ?? '')) {
        if (history) addHistoryFinding(path, 'windows-access-credential-in-retained-history')
        else addFinding(path, 'windows-access-credential', lineNumber)
        break
      }
      if (match[0].length === 0) pattern.lastIndex += 1
    }
  }
}

function createWindowsAccessStreamScanner(path, history) {
  const decoder = new StringDecoder('utf8')
  const previousContext = []
  const pendingLines = []
  let carry = ''
  let lineNumber = 1

  const scanPendingLine = () => {
    const current = pendingLines[0]
    let hasWindowsAccessContext = previousContext.includes(true)
    for (let index = 0; index < Math.min(3, pendingLines.length); index += 1) {
      if (pendingLines[index].hasWindowsAccessContext) {
        hasWindowsAccessContext = true
        break
      }
    }
    scanWindowsAccessLine(
      path,
      current.text,
      current.number,
      hasWindowsAccessContext,
      history
    )
    previousContext.push(current.hasWindowsAccessContext)
    if (previousContext.length > 8) previousContext.shift()
    pendingLines.shift()
  }
  const pushLine = (text) => {
    const normalized = text.endsWith('\r') ? text.slice(0, -1) : text
    windowsAccessContextPattern.lastIndex = 0
    pendingLines.push({
      text: normalized,
      number: lineNumber,
      hasWindowsAccessContext: windowsAccessContextPattern.test(normalized)
    })
    lineNumber += 1
    if (pendingLines.length >= 3) scanPendingLine()
  }
  const writeText = (text) => {
    carry += text
    let newline = carry.indexOf('\n')
    while (newline >= 0) {
      pushLine(carry.slice(0, newline))
      carry = carry.slice(newline + 1)
      newline = carry.indexOf('\n')
    }
  }

  return {
    writeBuffer(value) {
      writeText(decoder.write(value))
    },
    writeText,
    end() {
      writeText(decoder.end())
      pushLine(carry)
      carry = ''
      while (pendingLines.length > 0) scanPendingLine()
    }
  }
}

function scanWindowsAccessPatterns(path, text, history = false) {
  const scanner = createWindowsAccessStreamScanner(path, history)
  scanner.writeText(text)
  scanner.end()
}

async function scanRetainedWindowsAccessHistory() {
  const objects = new Map()
  for (const line of git(['rev-list', '--objects', '--all']).split('\n')) {
    const separator = line.indexOf(' ')
    if (separator <= 0) continue
    const objectId = line.slice(0, separator)
    const path = line.slice(separator + 1)
    if (!isTextCandidate(path)) continue
    objects.set(objectId, path)
  }
  await scanGitBlobs([...objects])
}

function hasRetainedObjectAtPath(targetPath) {
  const refs = git(['for-each-ref', '--format=%(refname)']).split('\n').filter(Boolean)
  if (refs.length === 0) return false
  const result = spawnSync('git', ['cat-file', '--batch-check'], {
    cwd: repoRoot,
    encoding: 'utf8',
    input: `${refs.map((ref) => `${ref}:${targetPath}`).join('\n')}\n`,
    maxBuffer: 32 * 1024 * 1024
  })
  if (result.status !== 0) {
    console.error('git cat-file failed while checking retained refs')
    process.exit(2)
  }
  return result.stdout.split('\n').some((line) => line && !line.endsWith(' missing'))
}

const trackedPaths = new Set(nulPaths(['ls-files', '--cached', '-z']))
const candidatePaths = [
  ...new Set(nulPaths(['ls-files', '--cached', '--others', '--exclude-standard', '-z']))
]
  .filter((path) => trackedPaths.has(path) || !isGeneratedOrHistorical(path))
  .filter(isTextCandidate)
  .filter((path) => existsSync(resolve(repoRoot, path)))
  .sort()

const absoluteRunbookPath = resolve(repoRoot, runbookPath)
if (!existsSync(absoluteRunbookPath)) {
  addFinding(runbookPath, 'required-runbook-missing', 1)
}
if (existsSync(resolve(repoRoot, retiredRunbookPath))) {
  addFinding(retiredRunbookPath, 'retired-runbook-still-present', 1)
}
if (requireIndexed && !trackedPaths.has(runbookPath)) {
  addFinding(runbookPath, 'required-runbook-not-indexed', 1)
}
if (requireIndexed && trackedPaths.has(retiredRunbookPath)) {
  addFinding(retiredRunbookPath, 'retired-runbook-still-indexed', 1)
}
if (requireHistoryClean) {
  const retainedCommits = git(['log', '--all', '--format=%H', '--', retiredRunbookPath]).trim()
  if (retainedCommits || hasRetainedObjectAtPath(retiredRunbookPath)) {
    addHistoryFinding(retiredRunbookPath, 'retired-runbook-still-reachable-in-history')
  }
  await scanRetainedWindowsAccessHistory()
}

for (const path of candidatePaths) {
  const text = readFileSync(resolve(repoRoot, path), 'utf8')
  const retiredReferenceOffset = text.indexOf(retiredRunbookPath)
  if (retiredReferenceOffset >= 0) {
    addFinding(path, 'retired-runbook-reference', lineNumberAt(text, retiredReferenceOffset))
  }

  scanPattern(
    path,
    text,
    'private-key-material',
    /-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----/gu
  )
  scanPattern(
    path,
    text,
    'windows-product-key-shape',
    /\b[A-Z0-9]{5}(?:-[A-Z0-9]{5}){4}\b/giu
  )
  scanWindowsAccessPatterns(path, text)

}

if (existsSync(absoluteRunbookPath)) {
  const runbook = readFileSync(absoluteRunbookPath, 'utf8')
  const requiredRunbookContracts = [
    ['approved-host-address', '192.168.31.90'],
    ['approved-hostname', 'DESKTOP-HNJ503B'],
    ['approved-account', 'analytix_remote'],
    ['ssh-alias', 'Host analytix-win-qa'],
    ['ssh-port', 'Port 22'],
    ['ssh-identity-file', 'IdentityFile ~/.ssh/analytix_win_test_ed25519'],
    ['rdp-port', '3389'],
    ['rdp-keychain-locator', 'analytix.windows.qa.rdp'],
    ['remote-password-keychain-locator', 'analytix.windows.qa.rustdesk'],
    ['remote-id-keychain-locator', 'analytix.windows.qa.rustdesk-id'],
    ['hub-test-account-keychain-locator', 'analytix.qa.hub-test-account'],
    ['hub-live-model', 'deepseek-v4-pro'],
    ['batch-mode-verification', 'BatchMode=yes']
  ]

  for (const [rule, marker] of requiredRunbookContracts) {
    if (!runbook.includes(marker)) addFinding(runbookPath, `missing-${rule}`, 1)
  }
}

if (findings.length > 0) {
  console.error('Windows QA runbook verification failed. Matching values are intentionally redacted.')
  for (const finding of findings) {
    console.error(
      finding.line === undefined
        ? `${finding.path} [${finding.rule}]`
        : `${finding.path}:${finding.line} [${finding.rule}]`
    )
  }
  process.exit(1)
}

console.log('Windows QA runbook verification passed.')
console.log(`runbook=${runbookPath}`)
console.log(`indexed=${trackedPaths.has(runbookPath) ? 'yes' : 'no (worktree verification)'}`)
console.log(`maintained_text_files_scanned=${candidatePaths.length}`)
console.log('secret_values_printed=no')
