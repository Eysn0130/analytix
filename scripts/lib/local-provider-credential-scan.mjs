import { constants } from 'node:fs'
import { lstat, open, readdir } from 'node:fs/promises'
import path from 'node:path'
import { localProviderAuthorityFromRegistry, sameLocalProviderAuthority } from './local-provider-acceptance.mjs'

// Harness-only leakage needles. This module never writes Registry, Secret Store,
// settings, or runtime authority. Handles deliberately carry no public data.
const sources = new WeakMap()
const maximumCredentialBytes = 8192
const chunkBytes = 64 * 1024
const coverage = Object.freeze(['utf8', 'base64', 'base64url', 'hex', 'url', 'json-escape'])
const validId = (value) => typeof value === 'string' && value.length > 0 && value.length <= 512
const methods = new Set(['visible-computer-use', 'visible-human'])
const failure = (code) => Object.assign(new Error(code), { code })

/** The caller owns its input buffer and must clean it up separately. The callback
 * must resolve {completed:true, providerRegistry} with the existing authenticated
 * public Registry readback for this exact visible entry completion. The trusted
 * coordinator must finish that readback before permitting another credential
 * entry; a later first-ready snapshot cannot substitute for it. No handle is
 * issued without a valid completion authority, on cancellation, or changed bytes.
 * This is the existing trusted entry boundary, not a new authentication channel.
 * JavaScript string
 * copies made by the callback/encoders cannot be promised whole-heap erasure. */
export async function captureLocalCredentialEntry(options, enterCredentialCallback) {
  const { runId, entryAttemptId, providerId, entryMethod, credential, signal } = options ?? {}
  if (!validId(runId) || !validId(entryAttemptId) || !validId(providerId) ||
      !methods.has(entryMethod) || !Buffer.isBuffer(credential) ||
      credential.length === 0 || credential.length > maximumCredentialBytes ||
      typeof enterCredentialCallback !== 'function' ||
      (signal !== undefined && !(signal instanceof AbortSignal))) throw failure('credential-entry-invalid')
  const entry = Buffer.from(credential)
  const secret = Buffer.from(credential)
  let retained = false
  let abortListener
  try {
    if (signal?.aborted) throw failure('credential-entry-not-completed')
    // Fatal decoding prevents malformed UTF-8 from producing a different needle.
    let decoded
    try { decoded = new TextDecoder('utf-8', { fatal: true }).decode(secret) } catch {
      throw failure('credential-entry-invalid')
    }
    if (!decoded.trim() || /[\u0000-\u001f\u007f]/u.test(decoded)) throw failure('credential-entry-invalid')
    let result
    try {
      const entryResult = Promise.resolve().then(() => enterCredentialCallback(entry, signal))
      result = signal ? await Promise.race([entryResult, new Promise((_, reject) => {
        abortListener = () => reject(failure('credential-entry-not-completed'))
        signal.addEventListener('abort', abortListener, { once: true })
        if (signal.aborted) abortListener()
      })]) : await entryResult
    } catch {
      throw failure('credential-entry-not-completed')
    }
    if (result?.completed !== true) throw failure('credential-entry-not-completed')
    if (!entry.equals(secret)) throw failure('credential-entry-changed')
    const authority = localProviderAuthorityFromRegistry(result.providerRegistry)
    if (!authority.ok || authority.id !== providerId) throw failure('credential-entry-authority-invalid')
    const source = Object.freeze(Object.create(null))
    sources.set(source, { runId, entryAttemptId, providerId, entryMethod, secret,
      authority: Object.freeze(authority) })
    retained = true
    return source
  } catch (error) {
    const allowed = new Set(['credential-entry-invalid', 'credential-entry-not-completed', 'credential-entry-changed',
      'credential-entry-authority-invalid'])
    throw failure(allowed.has(error?.code) ? error.code : 'credential-entry-not-completed')
  } finally {
    if (abortListener) signal.removeEventListener('abort', abortListener)
    entry.fill(0)
    if (!retained) secret.fill(0)
  }
}

// A late coordinator result must not outlive the bounded setup attempt.
export async function coordinateLocalCredentialEntry(coordinator, context, timeoutMs) {
  if (typeof coordinator !== 'function') return coordinator
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) throw failure('credential-entry-timeout')
  const controller = new AbortController()
  let expired = false
  let timer
  const result = Promise.resolve().then(() => coordinator({ ...context, signal: controller.signal }))
    .then((source) => {
      if (expired) disposeLocalCredentialScanSource(source)
      return source
    })
  try {
    return await Promise.race([result, new Promise((_, reject) => {
      timer = setTimeout(() => {
        expired = true
        controller.abort()
        reject(failure('credential-entry-timeout'))
      }, timeoutMs)
    })])
  } catch (error) {
    throw failure(error?.code === 'credential-entry-timeout'
      ? 'credential-entry-timeout' : 'credential-entry-not-completed')
  } finally {
    clearTimeout(timer)
  }
}

export function disposeLocalCredentialScanSource(source) {
  const value = sources.get(source)
  if (!value) return
  value.secret.fill(0)
  sources.delete(source)
}

export function readLocalCredentialScanSource(source, context = {}) {
  const value = sources.get(source)
  let blocker = null
  if (!value) blocker = 'credential-scan-source-missing'
  else if (!validId(context.runId) || !validId(context.entryAttemptId) || !validId(context.provider?.id)) {
    blocker = 'credential-scan-context-invalid'
  } else if (value.runId !== context.runId || value.entryAttemptId !== context.entryAttemptId ||
      value.providerId !== context.provider.id ||
      !sameLocalProviderAuthority(value.authority, context.provider)) blocker = 'credential-scan-source-stale'
  return {
    ok: blocker === null,
    blocked: blocker !== null,
    blocker,
    sourceBound: blocker === null,
    entryMethod: blocker === null ? value.entryMethod : null,
    automatedCredentialEntryUsed: blocker === null && value.entryMethod === 'visible-computer-use',
    expectedSecretCount: blocker === null ? 1 : 0,
    sourceSecretCount: blocker === null ? 1 : 0,
    uniqueSecretCount: blocker === null ? 1 : 0
  }
}

function encodedNeedles(secret) {
  const text = secret.toString('utf8')
  const url = encodeURIComponent(text)
  const fullUrl = [...secret].map((byte) => `%${byte.toString(16).padStart(2, '0')}`).join('')
  const jsonUnicode = text.split('').map((unit) => `\\u${unit.charCodeAt(0).toString(16).padStart(4, '0')}`).join('')
  const variants = new Set([
    text, secret.toString('base64'), secret.toString('base64').replace(/=+$/u, ''),
    secret.toString('base64url'), secret.toString('hex'), secret.toString('hex').toUpperCase(),
    url, url.replace(/%[0-9A-F]{2}/gu, (match) => match.toLowerCase()),
    fullUrl, fullUrl.toUpperCase(), JSON.stringify(text).slice(1, -1),
    jsonUnicode, jsonUnicode.replace(/\\u([0-9a-f]{4})/gu, (_, code) => `\\u${code.toUpperCase()}`)
  ])
  return [...variants].map((value) => Buffer.from(value, 'utf8'))
}

const identity = (stat) => [stat.dev, stat.ino, stat.mode, stat.nlink, stat.size, stat.mtimeNs, stat.ctimeNs].join(':')
const sameIdentity = (left, right) => identity(left) === identity(right)
const contains = (bytes, needles) => needles.some((needle) => bytes.includes(needle))

/** Scans regular files without an on-disk allowlist, including protected-store
 * ciphertext: encrypted storage must not contain a plaintext credential. Counts
 * are affected files/surfaces, not occurrences, and never include paths or hashes.
 * This is bounded encoding coverage, not a claim about arbitrary transformations.
 * Directory/file identity is checked again before a passing result is emitted. */
export async function scanLocalCredentialIsolation({ source, runId, entryAttemptId, provider, root, settingsPath, reportSnapshot } = {}) {
  const context = { runId, entryAttemptId, provider }
  const metadata = readLocalCredentialScanSource(source, context)
  const report = {
    ...metadata, status: 'blocked', scannedFileCount: 0, findingCount: 0,
    settingsFindingCount: 0, reportFindingCount: 0, unsafeEntryCount: 0,
    symlinkCount: 0, encodingCoverage: [...coverage]
  }
  if (!metadata.ok) return report
  let needles = []
  const snapshots = []
  let settingsSeen = false
  let blockedCode = null
  const block = (code) => { blockedCode ??= code }
  try {
    if (typeof root !== 'string' || !path.isAbsolute(root) || typeof settingsPath !== 'string' || (settingsPath !== '' && !path.isAbsolute(settingsPath))) {
      throw failure('credential-scan-path-invalid')
    }
    const rootPath = path.resolve(root)
    // A disjoint HOME root has no separate settings surface; every file in that
    // root is still scanned. Its caller must separately scan the product root.
    const settings = settingsPath === '' ? null : path.resolve(settingsPath)
    const relativeSettings = settings === null ? null : path.relative(rootPath, settings)
    if (relativeSettings !== null && (!relativeSettings || relativeSettings.startsWith(`..${path.sep}`) || relativeSettings === '..' || path.isAbsolute(relativeSettings))) {
      throw failure('credential-scan-path-invalid')
    }
    needles = encodedNeedles(sources.get(source).secret)
    const overlap = Math.max(...needles.map((needle) => needle.length)) - 1
    const scanFile = async (file, before) => {
      let handle
      let tail = Buffer.alloc(0)
      const chunk = Buffer.alloc(chunkBytes)
      let matched = false
      try {
        handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK)
        const opened = await handle.stat({ bigint: true })
        if (!opened.isFile() || opened.nlink !== 1n || !sameIdentity(before, opened)) throw failure('credential-scan-identity-changed')
        let position = 0
        for (;;) {
          if (!readLocalCredentialScanSource(source, context).ok) throw failure('credential-scan-source-missing')
          const { bytesRead } = await handle.read(chunk, 0, chunk.length, position)
          if (bytesRead === 0) break
          position += bytesRead
          const window = Buffer.concat([tail, chunk.subarray(0, bytesRead)])
          tail.fill(0)
          matched ||= contains(window, needles)
          tail = Buffer.from(window.subarray(Math.max(0, window.length - overlap)))
          window.fill(0)
        }
        const after = await handle.stat({ bigint: true })
        const atPath = await lstat(file, { bigint: true })
        if (!sameIdentity(before, after) || !sameIdentity(before, atPath)) throw failure('credential-scan-identity-changed')
        report.scannedFileCount++
        if (file === settings) { settingsSeen = true; report.settingsFindingCount += Number(matched) }
        report.findingCount += Number(matched)
      } finally {
        chunk.fill(0)
        tail.fill(0)
        await handle?.close()
      }
    }
    const visit = async (entry) => {
      const before = await lstat(entry, { bigint: true })
      if (before.isSymbolicLink()) {
        report.symlinkCount++; report.unsafeEntryCount++; block('credential-scan-unsafe-entry'); return
      }
      snapshots.push([entry, before])
      if (before.isDirectory()) {
        for (const name of await readdir(entry)) await visit(path.join(entry, name))
      } else if (before.isFile() && before.nlink === 1n) await scanFile(entry, before)
      else { report.unsafeEntryCount++; block('credential-scan-unsafe-entry') }
    }
    const rootStat = await lstat(rootPath, { bigint: true })
    if (!rootStat.isDirectory()) {
      report.symlinkCount += Number(rootStat.isSymbolicLink())
      report.unsafeEntryCount++
      throw failure('credential-scan-unsafe-entry')
    }
    await visit(rootPath)
    if (settings !== null && !settingsSeen) block('credential-scan-settings-missing')
    let serialized
    try { serialized = JSON.stringify(reportSnapshot) } catch { throw failure('credential-scan-report-invalid') }
    if (typeof serialized !== 'string') throw failure('credential-scan-report-invalid')
    const reportBytes = Buffer.from(serialized, 'utf8')
    try { report.reportFindingCount = Number(contains(reportBytes, needles)) } finally { reportBytes.fill(0) }
    report.findingCount += report.reportFindingCount
    for (const [entry, before] of snapshots) {
      if (!sameIdentity(before, await lstat(entry, { bigint: true }))) throw failure('credential-scan-identity-changed')
    }
    const current = readLocalCredentialScanSource(source, context)
    if (!current.ok) { Object.assign(report, current); block(current.blocker) }
    report.status = blockedCode ? 'blocked' : report.findingCount > 0 ? 'failed' : 'passed'
    report.blocked = blockedCode !== null
    report.blocker = blockedCode
    report.ok = report.status === 'passed'
  } catch (error) {
    const allowed = new Set(['credential-scan-path-invalid', 'credential-scan-identity-changed',
      'credential-scan-unsafe-entry', 'credential-scan-report-invalid', 'credential-scan-source-missing'])
    report.status = 'blocked'; report.blocked = true; report.ok = false
    report.blocker = allowed.has(error?.code) ? error.code : 'credential-scan-io-failed'
    const current = readLocalCredentialScanSource(source, context)
    if (!current.ok) Object.assign(report, current)
  } finally {
    for (const needle of needles) needle.fill(0)
  }
  return report
}
