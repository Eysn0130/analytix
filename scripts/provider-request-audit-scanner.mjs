#!/usr/bin/env node

import { spawnSync } from 'node:child_process'
import {
  createHash,
  createPrivateKey,
  createPublicKey,
  sign as signDocument
} from 'node:crypto'
import {
  chmodSync,
  existsSync,
  lstatSync,
  readFileSync,
  realpathSync,
  renameSync,
  unlinkSync,
  writeFileSync
} from 'node:fs'
import net from 'node:net'
import { dirname, isAbsolute, normalize, relative, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'
import {
  PROVIDER_AUDIT_SCANNER_POLICY,
  loadMilestoneBExternalCaseAcceptance
} from './runtime-go-packaged-milestone-b.mjs'

const PROVIDER_AUDIT_CONTRACT = 'analytix.milestone-b.provider-request-audit.v1'
// The wire field is historically called providerFamily, but Go sends ProviderID.
const PROVIDER_ID_PATTERN = /^[a-z0-9][a-z0-9._-]{0,95}$/u
const MAX_HEADER_BYTES = 4 * 1024
const MAX_BODY_BYTES = 64 * 1024 * 1024
const SOCKET_IDLE_TIMEOUT_MS = 20_000
const REQUIRED_SEMANTIC_FIELDS = Object.freeze(
  PROVIDER_AUDIT_SCANNER_POLICY.requiredProviderSafeSemanticProjection.requiredFields
)
const FORBIDDEN_SEMANTIC_FIELDS = new Set(
  PROVIDER_AUDIT_SCANNER_POLICY.requiredProviderSafeSemanticProjection.forbiddenFields
)
const FORBIDDEN_HOST_FIELDS = new Set(
  PROVIDER_AUDIT_SCANNER_POLICY.forbiddenHostMaterial.fieldNames
)
const SUBJECT_REFERENCE = new RegExp(
  PROVIDER_AUDIT_SCANNER_POLICY.requiredProviderSafeSemanticProjection.subjectReferencePattern,
  'u'
)
const SHA256_PATTERN = /^[0-9a-f]{64}$/u
const SOURCE_RECORD_PATTERN = /^srow1_[0-9a-f]{64}$/u
const EXACT_TIMESTAMP_PATTERN = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{6}Z$/u
const TIMEZONE_PATTERN = /^(?:Z|[+-](?:0[0-9]|1[0-3]):[0-5][0-9]|[+-]14:00)$/u
const UNSIGNED_MINOR_PATTERN = /^(?:0|[1-9][0-9]*)$/u
const SIGNED_MINOR_PATTERN = /^(?:0|-?[1-9][0-9]*)$/u
const COVERAGE_FIELDS = Object.freeze([
  'state', 'gaps', 'normalizedSnapshotRows', 'acceptedSnapshotRows',
  'rejectedSnapshotRows', 'duplicateSnapshotRows', 'untimedSubjectRows',
  'observedMatchingRows'
])
const COVERAGE_STATES = new Set(['complete', 'partial', 'observed_no_hit_pending_host_binding'])
const COVERAGE_GAPS = new Set([
  'rejected_source_rows', 'duplicate_source_rows', 'untimed_subject_rows',
  'evidence_row_limit', 'counterparty_resolution'
])
const TRANSACTION_FIELDS = Object.freeze([
  'evidenceRef', 'counterparty', 'occurredAt', 'direction', 'amountMinor',
  'currency', 'minorUnitScale'
])
const ARBITRARY_SQL_PATTERNS = Object.freeze([
  /(?:^|[^A-Z0-9_])SELECT[\s\S]{0,2048}(?:^|[^A-Z0-9_])(?:FROM|WHERE|JOIN|UNION|LIMIT)(?:$|[^A-Z0-9_])/u,
  /(?:^|[^A-Z0-9_])SELECT\s+(?:[0-9]+|NULL|TRUE|FALSE)(?:\s*;?\s*$)/u,
  /(?:^|[^A-Z0-9_])WITH\s*(?:[A-Z_][A-Z0-9_]*\s*)?(?:\([^)]{0,512}\)\s*)?AS\s*\(/u,
  /(?:^|[^A-Z0-9_])PRAGMA\s+[A-Z_][A-Z0-9_]*(?:\s*(?:[=(;]|$))/u,
  /(?:^|[^A-Z0-9_])ATTACH(?:\s+DATABASE)?[\s\S]{1,512}\s+AS\s+[A-Z_][A-Z0-9_]*(?:$|[^A-Z0-9_])/u,
  /(?:^|[^A-Z0-9_])COPY\s+[\s\S]{1,512}\s+(?:FROM|TO)(?:$|[^A-Z0-9_])/u
])

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value)
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  return `{${Object.keys(value).sort().map((key) =>
    `${JSON.stringify(key)}:${canonicalJSON(value[key])}`
  ).join(',')}}`
}

function exactKeys(value, keys) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    JSON.stringify(Object.keys(value).sort()) === JSON.stringify([...keys].sort())
}

function pathContainedBy(root, candidate) {
  const escaped = relative(root, candidate)
  return escaped !== '..' && !escaped.startsWith(`..${sep}`) && !isAbsolute(escaped)
}

function exactOwnerPrivateFile(path, maximumBytes = 1024 * 1024) {
  const state = lstatSync(path)
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  return state.isFile() && !state.isSymbolicLink() && state.nlink === 1 &&
    state.size > 0 && state.size <= maximumBytes &&
    (uid === null || state.uid === uid) && (state.mode & 0o077) === 0
}

function exactOwnerPrivateDirectory(path) {
  const state = lstatSync(path)
  const uid = typeof process.getuid === 'function' ? process.getuid() : null
  return state.isDirectory() && !state.isSymbolicLink() &&
    (uid === null || state.uid === uid) && (state.mode & 0o777) === 0o700 &&
    (state.mode & 0o7000) === 0
}

function decodeHTMLNumericEntities(value) {
  return value.replace(/&#(?:x([0-9a-f]{1,6})|([0-9]{1,7}));?/giu, (match, hex, decimal) => {
    const codePoint = Number.parseInt(hex || decimal, hex ? 16 : 10)
    try {
      return codePoint >= 0 && codePoint <= 0x10ffff ? String.fromCodePoint(codePoint) : match
    } catch {
      return match
    }
  })
}

function decodeJavaScriptUnicodeEscapes(value) {
  return value
    .replace(/\\u\{([0-9a-f]{1,6})\}/giu, (match, hex) => {
      try {
        return String.fromCodePoint(Number.parseInt(hex, 16))
      } catch {
        return match
      }
    })
    .replace(/\\u([0-9a-f]{4})/giu, (_match, hex) =>
      String.fromCharCode(Number.parseInt(hex, 16)))
}

function decodeASCIIPercentEscapes(value) {
  return value.replace(/%([0-7][0-9a-f])/giu, (match, hex) => {
    const byte = Number.parseInt(hex, 16)
    return byte <= 0x7f ? String.fromCharCode(byte) : match
  })
}

function boundedMarkupElision(value) {
  return value.replace(/<[^<>]{0,512}>/gu, '')
}

function decodedCandidates(value) {
  const candidates = new Set([value])
  for (const transform of [
    decodeHTMLNumericEntities,
    decodeJavaScriptUnicodeEscapes,
    decodeASCIIPercentEscapes,
    boundedMarkupElision
  ]) {
    for (const candidate of [...candidates]) candidates.add(transform(candidate))
  }
  return [...candidates]
}

function elideNumericSeparators(value) {
  return value.replace(/[\x20\t\u00a0\u2007\u202f-]/gu, '')
}

function containsArbitrarySQL(value) {
  return decodedCandidates(value).some((candidate) => {
    const upper = candidate.toUpperCase()
    return ARBITRARY_SQL_PATTERNS.some((pattern) => pattern.test(upper))
  })
}

function containsProtectedValue(text, protectedValues) {
  const candidates = decodedCandidates(text)
  for (const protectedValue of protectedValues) {
    if (candidates.some((candidate) => candidate.includes(protectedValue))) return true
    if (/^\d{8,32}$/u.test(protectedValue)) {
      const normalized = elideNumericSeparators(protectedValue)
      if (candidates.some((candidate) => elideNumericSeparators(candidate).includes(normalized))) {
        return true
      }
    }
  }
  return false
}

function walkJSON(value, visitObject, visitString) {
  if (typeof value === 'string') {
    visitString(value)
    const trimmed = value.trim()
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) ||
        (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      try {
        walkJSON(JSON.parse(trimmed), visitObject, visitString)
      } catch {
        // A provider-visible ordinary string need not itself be JSON.
      }
    }
    return
  }
  if (Array.isArray(value)) {
    for (const item of value) walkJSON(item, visitObject, visitString)
    return
  }
  if (!value || typeof value !== 'object') return
  visitObject(value)
  for (const item of Object.values(value)) walkJSON(item, visitObject, visitString)
}

function boundedInteger(value, maximum) {
  return Number.isSafeInteger(value) && value >= 0 && value <= maximum
}

function validExactTimestamp(value) {
  return typeof value === 'string' && EXACT_TIMESTAMP_PATTERN.test(value) &&
    !Number.isNaN(Date.parse(value))
}

function validCoverage(value) {
  return exactKeys(value, COVERAGE_FIELDS) && COVERAGE_STATES.has(value.state) &&
    Array.isArray(value.gaps) && value.gaps.length <= 5 &&
    new Set(value.gaps).size === value.gaps.length &&
    value.gaps.every((gap) => COVERAGE_GAPS.has(gap)) &&
    COVERAGE_FIELDS.slice(2).every((field) => boundedInteger(value[field], 100_000))
}

function validCounterparty(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      !['resolved', 'partial', 'unresolved'].includes(value.status)) return false
  if (value.status === 'unresolved') return exactKeys(value, ['status'])
  if (!exactKeys(value, ['status', 'reference', 'display']) ||
      !SUBJECT_REFERENCE.test(String(value.reference || ''))) return false
  const display = value.display
  if (!display || typeof display !== 'object' || Array.isArray(display)) return false
  const allowed = new Set([
    'entityType', 'stableOrdinal', 'safeSuffix', 'institution', 'accountType', 'text'
  ])
  const keys = Object.keys(display)
  if (!['entityType', 'stableOrdinal', 'accountType', 'text'].every((key) => keys.includes(key)) ||
      keys.some((key) => !allowed.has(key)) ||
      display.entityType !== 'bank_account_number' ||
      !boundedInteger(display.stableOrdinal, 4_294_967_295) || display.stableOrdinal === 0 ||
      display.accountType !== '交易对手账户' || typeof display.text !== 'string' ||
      display.text.length === 0 || Buffer.byteLength(display.text, 'utf8') > 384 ||
      (Object.hasOwn(display, 'safeSuffix') && !/^[0-9]{4}$/u.test(display.safeSuffix)) ||
      (Object.hasOwn(display, 'institution') &&
        (typeof display.institution !== 'string' || display.institution.length === 0 ||
          Buffer.byteLength(display.institution, 'utf8') > 128)) ||
      (value.status === 'resolved' && !Object.hasOwn(display, 'institution'))) return false
  return true
}

function validTransaction(value, semanticCurrency) {
  return exactKeys(value, TRANSACTION_FIELDS) &&
    SOURCE_RECORD_PATTERN.test(String(value.evidenceRef || '')) &&
    validCounterparty(value.counterparty) && validExactTimestamp(value.occurredAt) &&
    ['inflow', 'outflow'].includes(value.direction) &&
    typeof value.amountMinor === 'string' && value.amountMinor.length <= 128 &&
    UNSIGNED_MINOR_PATTERN.test(value.amountMinor) && value.currency === semanticCurrency &&
    value.minorUnitScale === 2
}

function validProviderSafeSemanticBlock(value) {
  if (!exactKeys(value, REQUIRED_SEMANTIC_FIELDS) ||
      Object.keys(value).some((field) => FORBIDDEN_SEMANTIC_FIELDS.has(field)) ||
      !SUBJECT_REFERENCE.test(String(value.subjectRef || '')) ||
      !validExactTimestamp(value.startInclusive) || !validExactTimestamp(value.endInclusive) ||
      value.startInclusive > value.endInclusive ||
      !TIMEZONE_PATTERN.test(String(value.timezone || '')) ||
      !/^[A-Z]{3}$/u.test(String(value.currency || '')) || value.minorUnitScale !== 2 ||
      typeof value.inflowMinor !== 'string' || value.inflowMinor.length > 128 ||
      !UNSIGNED_MINOR_PATTERN.test(value.inflowMinor) ||
      typeof value.outflowMinor !== 'string' || value.outflowMinor.length > 128 ||
      !UNSIGNED_MINOR_PATTERN.test(value.outflowMinor) ||
      typeof value.netMinor !== 'string' || value.netMinor.length > 129 ||
      !SIGNED_MINOR_PATTERN.test(value.netMinor) ||
      BigInt(value.inflowMinor) - BigInt(value.outflowMinor) !== BigInt(value.netMinor) ||
      !boundedInteger(value.transactionCount, 100_000) ||
      !boundedInteger(value.evidenceTransactionCount, 512) ||
      !boundedInteger(value.evidenceRowLimit, 512) || value.evidenceRowLimit === 0 ||
      value.evidenceTransactionCount > value.transactionCount ||
      value.evidenceTransactionCount > value.evidenceRowLimit ||
      typeof value.aggregateComplete !== 'boolean' ||
      typeof value.evidenceRowsComplete !== 'boolean' ||
      typeof value.counterpartySemanticsComplete !== 'boolean' ||
      !validCoverage(value.coverage) || !Array.isArray(value.transactions) ||
      !SHA256_PATTERN.test(String(value.queryHash || '')) ||
      !SHA256_PATTERN.test(String(value.resultHash || ''))) return false
  if (value.transactions.length !== value.evidenceTransactionCount ||
      value.transactions.some((transaction) => !validTransaction(transaction, value.currency)) ||
      new Set(value.transactions.map((transaction) => transaction.evidenceRef)).size !==
        value.transactions.length ||
      (value.evidenceRowsComplete && value.evidenceTransactionCount !== value.transactionCount) ||
      (value.counterpartySemanticsComplete &&
        value.transactions.some((transaction) => transaction.counterparty.status !== 'resolved')) ||
      (!value.counterpartySemanticsComplete &&
        !value.coverage.gaps.includes('counterparty_resolution'))) return false
  return true
}

export function scanOutboundProviderRequestBody({
  body,
  protectedValues,
  forbiddenValues
}) {
  if (!Buffer.isBuffer(body) || body.length === 0 || body.length > MAX_BODY_BYTES ||
      !Array.isArray(protectedValues) || protectedValues.length === 0 ||
      !Array.isArray(forbiddenValues) || forbiddenValues.length < 3) {
    return Object.freeze({
      accepted: false,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 0,
      providerSafeSemanticValidationCount: 0
    })
  }
  const text = body.toString('utf8')
  if (!Buffer.from(text, 'utf8').equals(body)) {
    return Object.freeze({
      accepted: false,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 0,
      providerSafeSemanticValidationCount: 0
    })
  }
  let parsed
  try {
    parsed = JSON.parse(text)
  } catch {
    return Object.freeze({
      accepted: false,
      completeIdentifierFindingCount: 0,
      forbiddenHostMaterialFindingCount: 0,
      providerSafeSemanticBlockCount: 0,
      providerSafeSemanticValidationCount: 0
    })
  }

  let completeIdentifierFindingCount = containsProtectedValue(text, protectedValues) ? 1 : 0
  let forbiddenHostMaterialFindingCount = containsProtectedValue(text, forbiddenValues) ? 1 : 0
  let providerSafeSemanticBlockCount = 0
  let providerSafeSemanticValidationCount = 0
  let forbiddenDatabaseOrSQL = false
  walkJSON(parsed, (candidate) => {
    if (Object.keys(candidate).some((field) => FORBIDDEN_HOST_FIELDS.has(field))) {
      forbiddenHostMaterialFindingCount += 1
    }
    const keys = new Set(Object.keys(candidate))
    if (keys.has('subjectRef') && keys.has('inflowMinor') &&
        keys.has('outflowMinor') && keys.has('transactions')) {
      providerSafeSemanticBlockCount += 1
      if (validProviderSafeSemanticBlock(candidate)) providerSafeSemanticValidationCount += 1
    }
  }, (candidate) => {
    if (completeIdentifierFindingCount === 0 && containsProtectedValue(candidate, protectedValues)) {
      completeIdentifierFindingCount = 1
    }
    if (forbiddenHostMaterialFindingCount === 0 &&
        containsProtectedValue(candidate, forbiddenValues)) {
      forbiddenHostMaterialFindingCount = 1
    }
    if (decodedCandidates(candidate).some((decoded) =>
      containsArbitrarySQL(decoded) ||
      /(?:^|[\\/\s"'])[^\n\r"']*\.(?:duckdb|db|sqlite)(?:$|[?#\s"'])/iu.test(decoded)
    )) {
      forbiddenDatabaseOrSQL = true
    }
  })
  if (forbiddenDatabaseOrSQL) forbiddenHostMaterialFindingCount += 1
  const accepted = completeIdentifierFindingCount === 0 &&
    forbiddenHostMaterialFindingCount === 0 &&
    providerSafeSemanticValidationCount === providerSafeSemanticBlockCount
  return Object.freeze({
    accepted,
    completeIdentifierFindingCount,
    forbiddenHostMaterialFindingCount,
    providerSafeSemanticBlockCount,
    providerSafeSemanticValidationCount
  })
}

function canonicalChallenge(path) {
  if (!exactOwnerPrivateFile(path, 64 * 1024)) throw new Error('challenge_invalid')
  const bytes = readFileSync(path)
  let value
  try {
    value = JSON.parse(bytes.toString('utf8'))
  } finally {
    bytes.fill(0)
  }
  const keys = [
    'contract', 'caseContractSha256', 'caseProvenanceSha256',
    'protectedValueSetDigest', 'protectedValueCount',
    'forbiddenValueSetDigest', 'forbiddenValueCount', 'scannerBuildSha256',
    'scannerPolicySha256', 'runNonceDigest', 'sourceCommit', 'issuedAt', 'expiresAt'
  ]
  if (!exactKeys(value, keys) ||
      value.contract !== 'analytix.milestone-b.provider-request-audit-challenge.v1' ||
      !SHA256_PATTERN.test(value.caseContractSha256) ||
      !SHA256_PATTERN.test(value.caseProvenanceSha256) ||
      !SHA256_PATTERN.test(value.protectedValueSetDigest) ||
      !Number.isSafeInteger(value.protectedValueCount) || value.protectedValueCount <= 0 ||
      !SHA256_PATTERN.test(value.forbiddenValueSetDigest) ||
      !Number.isSafeInteger(value.forbiddenValueCount) || value.forbiddenValueCount < 3 ||
      !SHA256_PATTERN.test(value.scannerBuildSha256) ||
      !SHA256_PATTERN.test(value.scannerPolicySha256) ||
      !SHA256_PATTERN.test(value.runNonceDigest) || !/^[0-9a-f]{40}$/u.test(value.sourceCommit) ||
      Number.isNaN(Date.parse(value.issuedAt)) || Number.isNaN(Date.parse(value.expiresAt)) ||
      Date.now() < Date.parse(value.issuedAt) - 60_000 || Date.now() >= Date.parse(value.expiresAt)) {
    throw new Error('challenge_invalid')
  }
  return value
}

export function createProviderRequestAuditRun({ onFailure = () => {} } = {}) {
  let providerId = ''
  let failed = false
  const reject = () => {
    if (!failed) {
      failed = true
      onFailure()
    }
    return false
  }
  return Object.freeze({
    reject,
    acceptProviderId(value) {
      if (failed) return false
      if (typeof value !== 'string' || value !== value.trim() || !PROVIDER_ID_PATTERN.test(value) ||
          value === 'analytix-hub' || (providerId && value !== providerId)) return reject()
      providerId = value
      return true
    },
    isAccepted() { return !failed && providerId !== '' },
    acceptScannedBody(result, digest, expectedDigest) {
      if (!result.accepted || digest !== expectedDigest) return reject()
      return !failed && providerId !== ''
    },
    receiptFields(totals) {
      return {
        ...totals,
        // A failed run cannot establish one valid Provider identity. Keep real
        // counts unchanged; the existing signed exact-identity gate rejects this
        // receipt permanently, including after a later otherwise valid frame.
        providerFamily: failed ? '' : providerId
      }
    }
  })
}

export function readExactFrame(socket, run) {
  return new Promise((resolveFrame, rejectFrame) => {
    let header = Buffer.alloc(0)
    let body = null
    let offset = 0
    let metadata = null
    let settled = false
    const finish = (error) => {
      if (settled) return
      settled = true
      socket.off('data', onData)
      socket.off('end', onEnd)
      socket.off('error', onError)
      socket.off('close', onEnd)
      if (error) {
        header.fill(0)
        body?.fill(0)
        run.reject()
        rejectFrame(error)
      } else {
        resolveFrame({ metadata, body })
      }
    }
    const onError = () => finish(new Error('frame_invalid'))
    const onEnd = () => finish(new Error('frame_invalid'))
    const onData = (chunk) => {
      if (!body) {
        const newline = chunk.indexOf(0x0a)
        const headerPart = newline >= 0 ? chunk.subarray(0, newline) : chunk
        const combined = Buffer.concat([header, headerPart])
        header.fill(0)
        header = combined
        if (header.length > MAX_HEADER_BYTES) return finish(new Error('frame_invalid'))
        if (newline < 0) return
        try {
          metadata = JSON.parse(header.toString('utf8'))
        } catch {
          return finish(new Error('frame_invalid'))
        } finally {
          header.fill(0)
        }
        if (!exactKeys(metadata, [
          'schemaVersion', 'purpose', 'providerFamily', 'attempt', 'bodyByteLength', 'bodySha256'
        ]) || metadata.schemaVersion !== 1 ||
            metadata.purpose !== 'analytix.provider-request-body-audit/v1' ||
            !Number.isSafeInteger(metadata.attempt) || metadata.attempt <= 0 ||
            !Number.isSafeInteger(metadata.bodyByteLength) || metadata.bodyByteLength <= 0 ||
            metadata.bodyByteLength > MAX_BODY_BYTES || !SHA256_PATTERN.test(metadata.bodySha256)) {
          return finish(new Error('frame_invalid'))
        }
        if (!run.acceptProviderId(metadata.providerFamily)) return finish(new Error('frame_invalid'))
        body = Buffer.allocUnsafe(metadata.bodyByteLength)
        const remainder = chunk.subarray(newline + 1)
        if (remainder.length > body.length) return finish(new Error('frame_invalid'))
        remainder.copy(body, 0)
        offset = remainder.length
      } else {
        if (offset + chunk.length > body.length) return finish(new Error('frame_invalid'))
        chunk.copy(body, offset)
        offset += chunk.length
      }
      if (body && offset === body.length) {
        socket.pause()
        finish()
      }
    }
    socket.setTimeout(SOCKET_IDLE_TIMEOUT_MS, () => finish(new Error('frame_invalid')))
    socket.on('data', onData)
    socket.once('end', onEnd)
    socket.once('error', onError)
    socket.once('close', onEnd)
  })
}

function currentGitCommit(repositoryRoot) {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore']
  })
  return result.status === 0 ? String(result.stdout || '').trim().toLowerCase() : ''
}

async function main() {
  const input = await readPrivateStartupFrame()
  const auditOwnerRoot = realpathSync(input.auditOwnerRoot)
  const socketPath = resolve(input.socketPath)
  const challengePath = realpathSync(input.challengePath)
  const privateKeyPath = realpathSync(input.privateKeyPath)
  const publicKeyPath = realpathSync(input.publicKeyPath)
  const receiptPath = resolve(input.receiptPath)
  const receiptTemporaryPath = `${receiptPath}.next`
  const repositoryRoot = realpathSync(input.repositoryRoot)
  const ownerRoot = realpathSync(input.caseOwnerRoot)
  const workspace = realpathSync(input.caseWorkspace)
  const contractPath = realpathSync(input.caseContractPath)
  const provenancePath = realpathSync(input.caseProvenancePath)
  const scannerPath = realpathSync(fileURLToPath(import.meta.url))
  const challenge = canonicalChallenge(challengePath)
  const auditPaths = [
    socketPath, challengePath, privateKeyPath, publicKeyPath, receiptPath, receiptTemporaryPath
  ]
  if (!exactOwnerPrivateDirectory(auditOwnerRoot) ||
      !auditPaths.every((path) => pathContainedBy(auditOwnerRoot, path)) ||
      new Set(auditPaths).size !== auditPaths.length ||
      [repositoryRoot, ownerRoot, workspace].some((protectedRoot) =>
        pathContainedBy(auditOwnerRoot, protectedRoot) ||
        pathContainedBy(protectedRoot, auditOwnerRoot)
      ) ||
      !isAbsolute(socketPath) || normalize(socketPath) !== socketPath || existsSync(socketPath) ||
      existsSync(receiptPath) || existsSync(receiptTemporaryPath) ||
      !exactOwnerPrivateDirectory(dirname(socketPath)) ||
      !exactOwnerPrivateDirectory(dirname(receiptPath)) ||
      !exactOwnerPrivateFile(privateKeyPath, 64 * 1024) ||
      !exactOwnerPrivateFile(publicKeyPath, 64 * 1024) ||
      !pathContainedBy(realpathSync(dirname(socketPath)), socketPath) ||
      !pathContainedBy(realpathSync(dirname(receiptPath)), receiptPath) ||
      sha256(readFileSync(scannerPath)) !== challenge.scannerBuildSha256 ||
      sha256(canonicalJSON(PROVIDER_AUDIT_SCANNER_POLICY)) !== challenge.scannerPolicySha256 ||
      currentGitCommit(repositoryRoot) !== challenge.sourceCommit) {
    throw new Error('scanner_authority_invalid')
  }

  const caseAuthority = loadMilestoneBExternalCaseAcceptance({
    workspace,
    ownerRoot,
    contractPath,
    provenancePath,
    repositoryRoot,
    startedAt: new Date(challenge.issuedAt),
    diagnosticSynthetic: input.diagnosticSynthetic
  })
  const protectedValues = [...new Set(caseAuthority.protectedValues)].sort()
  const forbiddenValues = [...new Set([
    caseAuthority.workspace,
    caseAuthority.ownerRoot,
    ...caseAuthority.snapshots.map((snapshot) => snapshot.sourcePath)
  ])].sort()
  if (caseAuthority.contractSha256 !== challenge.caseContractSha256 ||
      caseAuthority.provenanceSha256 !== challenge.caseProvenanceSha256 ||
      protectedValues.length !== challenge.protectedValueCount ||
      sha256(canonicalJSON(protectedValues)) !== challenge.protectedValueSetDigest ||
      forbiddenValues.length !== challenge.forbiddenValueCount ||
      sha256(canonicalJSON(forbiddenValues)) !== challenge.forbiddenValueSetDigest) {
    throw new Error('scanner_case_binding_invalid')
  }

  const privateBytes = readFileSync(privateKeyPath)
  const publicBytes = readFileSync(publicKeyPath)
  let privateKey
  let publicKey
  try {
    privateKey = createPrivateKey(privateBytes)
    publicKey = createPublicKey(publicBytes)
  } finally {
    privateBytes.fill(0)
    publicBytes.fill(0)
  }
  const derivedPublic = createPublicKey(privateKey).export({ format: 'der', type: 'spki' })
  const trustedPublic = publicKey.export({ format: 'der', type: 'spki' })
  if (privateKey.asymmetricKeyType !== 'ed25519' || publicKey.asymmetricKeyType !== 'ed25519' ||
      !Buffer.from(derivedPublic).equals(Buffer.from(trustedPublic))) {
    throw new Error('scanner_signing_authority_invalid')
  }
  const keyId = sha256(trustedPublic)
  const payloadDigests = []
  const totals = {
    providerRequestCount: 0,
    scannedRequestBodyCount: 0,
    completeIdentifierFindingCount: 0,
    forbiddenHostMaterialFindingCount: 0,
    providerSafeSemanticBlockCount: 0,
    providerSafeSemanticValidationCount: 0
  }
  let scanQueue = Promise.resolve()
  const run = createProviderRequestAuditRun({ onFailure: () => {
    try {
      writeReceipt()
    } catch {
      // This exact task-owned receipt must not remain valid after a rejected
      // frame even if the atomic replacement fails (for example disk full).
      if (existsSync(receiptPath)) unlinkSync(receiptPath)
    }
  } })

  const writeReceipt = () => {
    const unsigned = {
      contract: PROVIDER_AUDIT_CONTRACT,
      caseContractSha256: challenge.caseContractSha256,
      caseProvenanceSha256: challenge.caseProvenanceSha256,
      runNonceDigest: challenge.runNonceDigest,
      sourceCommit: challenge.sourceCommit,
      protectedValueSetDigest: challenge.protectedValueSetDigest,
      protectedValueCount: challenge.protectedValueCount,
      forbiddenValueSetDigest: challenge.forbiddenValueSetDigest,
      forbiddenValueCount: challenge.forbiddenValueCount,
      scannerBuildSha256: challenge.scannerBuildSha256,
      scannerPolicySha256: challenge.scannerPolicySha256,
      captureScope: 'outbound-provider-request-bodies-before-network-send',
      ...run.receiptFields(totals),
      generatedAt: new Date().toISOString(),
      payloadSetDigest: sha256(canonicalJSON(payloadDigests)),
      keyId
    }
    const signature = signDocument(null, Buffer.from(canonicalJSON(unsigned)), privateKey)
      .toString('base64url')
    writeFileSync(receiptTemporaryPath, JSON.stringify({ ...unsigned, signature }), {
      encoding: 'utf8',
      mode: 0o600,
      flag: 'wx'
    })
    chmodSync(receiptTemporaryPath, 0o600)
    renameSync(receiptTemporaryPath, receiptPath)
  }

  const server = net.createServer((socket) => {
    readExactFrame(socket, run).then(({ metadata, body }) => {
      scanQueue = scanQueue.then(() => {
        const digest = sha256(body)
        const result = scanOutboundProviderRequestBody({ body, protectedValues, forbiddenValues })
        totals.providerRequestCount += 1
        totals.scannedRequestBodyCount += 1
        totals.completeIdentifierFindingCount += result.completeIdentifierFindingCount
        totals.forbiddenHostMaterialFindingCount += result.forbiddenHostMaterialFindingCount
        totals.providerSafeSemanticBlockCount += result.providerSafeSemanticBlockCount
        totals.providerSafeSemanticValidationCount += result.providerSafeSemanticValidationCount
        payloadDigests.push(digest)
        const accepted = run.acceptScannedBody(result, digest, metadata.bodySha256)
        writeReceipt()
        socket.end(`${JSON.stringify({
          schemaVersion: 1,
          purpose: 'analytix.provider-request-body-audit-ack/v1',
          accepted,
          providerFamily: metadata.providerFamily,
          attempt: metadata.attempt,
          bodySha256: digest
        })}\n`)
      }).catch(() => { run.reject(); socket.destroy() })
        .finally(() => body.fill(0))
    }).catch(() => socket.destroy())
  })

  const cleanup = () => {
    server.close(() => {
      if (existsSync(socketPath)) unlinkSync(socketPath)
      process.exit(0)
    })
  }
  process.once('SIGTERM', cleanup)
  process.once('SIGINT', cleanup)
  server.on('error', () => process.exit(1))
  server.listen(socketPath, () => {
    chmodSync(socketPath, 0o600)
    process.stdout.write('ANALYTIX_PROVIDER_AUDIT_SCANNER_READY_V1\n')
  })
}

async function readPrivateStartupFrame() {
  const chunks = []
  let byteLength = 0
  for await (const chunk of process.stdin) {
    const value = Buffer.from(chunk)
    chunks.push(value)
    byteLength += value.length
    if (byteLength > 64 * 1024) throw new Error('scanner_startup_frame_invalid')
  }
  const frame = Buffer.concat(chunks, byteLength)
  for (const chunk of chunks) chunk.fill(0)
  try {
    if (frame.length < 9) throw new Error('scanner_startup_frame_invalid')
    const declared = Number(frame.readBigUInt64BE(0))
    if (!Number.isSafeInteger(declared) || declared <= 0 || declared !== frame.length - 8) {
      throw new Error('scanner_startup_frame_invalid')
    }
    const value = JSON.parse(frame.subarray(8).toString('utf8'))
    if (!exactKeys(value, [
      'schemaVersion', 'purpose', 'auditOwnerRoot', 'socketPath', 'challengePath', 'privateKeyPath',
      'publicKeyPath', 'receiptPath', 'repositoryRoot', 'caseOwnerRoot', 'caseWorkspace',
      'caseContractPath', 'caseProvenancePath', 'diagnosticSynthetic'
    ]) || value.schemaVersion !== 1 ||
        value.purpose !== 'analytix.provider-request-audit-scanner-startup/v1' ||
        typeof value.diagnosticSynthetic !== 'boolean' ||
        Object.entries(value).some(([key, item]) =>
          !['schemaVersion', 'diagnosticSynthetic'].includes(key) &&
          (typeof item !== 'string' || item.trim() !== item || !item)
        )) {
      throw new Error('scanner_startup_frame_invalid')
    }
    return value
  } catch {
    throw new Error('scanner_startup_frame_invalid')
  } finally {
    frame.fill(0)
  }
}

function isDirectExecution() {
  try {
    return realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))
  } catch {
    return false
  }
}

if (isDirectExecution()) {
  if (process.argv.length !== 3 || process.argv[2] !== '--private-startup-frame-v1') {
    process.stderr.write('provider request audit scanner failed closed\n')
    process.exitCode = 1
  } else main().catch(() => {
    process.stderr.write('provider request audit scanner failed closed\n')
    process.exitCode = 1
  })
}
