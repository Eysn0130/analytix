#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import {
  chmodSync,
  copyFileSync,
  cpSync,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  closeSync,
  readFileSync,
  realpathSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  writeFileSync,
  writeSync
} from 'node:fs'
import http from 'node:http'
import { homedir } from 'node:os'
import { basename, dirname, isAbsolute, join, normalize, relative, resolve, sep } from 'node:path'
import process from 'node:process'

const readyPrefix = 'ANALYTIX_RUNTIME_SERVER_READY '
const authorityReadyPrefix = 'ANALYTIX_FORMAL_AUTHORITY_READY_V1 '
const authorityBootstrapFile = 'authority-bootstrap-v1.json'
const authorityBootstrapPurpose = 'analytix.runtime-main-owned-authority/v1'
const startupFramePurpose = 'analytix.runtime-startup-private-frame/v1'
const providerID = 'ab-r2-contract-provider'
const providerModel = 'ab-r2-contract-model'
const providerKey = 'sk-abR2SyntheticProviderKey'
const privateSentinel = 'AB_R2_PRIVATE_SOURCE_SENTINEL_9f07f00d'
const cachePrefix = '/Volumes/AnalytixCache/'
const timeoutMs = 120_000
const packageOperationTimeoutMs = 480_000
const sourceUnavailableText = '当前案件资金分析来源在本轮未通过可用性核验'
const casePromptPrefix = '查询当前案件账户在指定期间的流入、流出、净额和交易笔数。'

const rawArgs = process.argv.slice(2)
const keep = rawArgs.includes('--keep')
const accountFlowDenominator = rawArgs.includes('--account-flow-denominator')
const accountFlowToolName = 'mcp__analytix_funds__analyze_account_flows'
const accountFlowModelPurpose = 'analytix.account-flow-provider-semantics/v3'
const accountFlowArguments = {
  subject_alias: 'acct:1',
  start_inclusive: '2026-08-27T00:00:00.000000Z',
  end_inclusive: '2026-08-27T23:59:59.999999Z',
  evidence_row_limit: 1
}

function argValue(name) {
  const prefix = `${name}=`
  const inline = rawArgs.find((value) => value.startsWith(prefix))
  if (inline) return inline.slice(prefix.length)
  const index = rawArgs.indexOf(name)
  return index >= 0 ? rawArgs[index + 1] || '' : ''
}

function requireCondition(condition, message) {
  if (!condition) throw new Error(message)
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function exactKeys(value, expected) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join('\0') === [...expected].sort().join('\0')
}

function isSHA256(value) {
  return /^[a-f0-9]{64}$/u.test(String(value || ''))
}

function pathContainedBy(root, candidate) {
  const relativePath = relative(root, candidate)
  return relativePath === '' || (relativePath !== '..' && !relativePath.startsWith(`..${sep}`) && !isAbsolute(relativePath))
}

function cachePath(path, label) {
  const absolute = resolve(path)
  requireCondition(absolute.startsWith(cachePrefix), `${label} escaped the Analytix cache volume`)
  return absolute
}

function privateDirectory(path) {
  mkdirSync(path, { recursive: true, mode: 0o700 })
  chmodSync(path, 0o700)
  return path
}

function exactPrivateDirectory(path) {
  const state = lstatSync(path)
  return state.isDirectory() && !state.isSymbolicLink() && state.uid === process.getuid() &&
    (state.mode & 0o7777) === 0o700
}

function canonicalAuthorityBootstrap(body, ownerRoot) {
  let value
  try {
    value = JSON.parse(body)
  } catch {
    throw new Error('formal authority bootstrap is invalid')
  }
  requireCondition(exactKeys(value, [
    'schemaVersion',
    'purpose',
    'authorityAnchorV1',
    'authorityManifestRoot',
    'authorityCredentialProfileRoot',
    'authorityCredentialBundleRoot'
  ]) && value.schemaVersion === 1 && value.purpose === authorityBootstrapPurpose &&
    typeof value.authorityAnchorV1 === 'string' && JSON.stringify(value) === body,
  'formal authority bootstrap is not canonical')
  let anchor
  try {
    anchor = JSON.parse(value.authorityAnchorV1)
  } catch {
    throw new Error('formal authority anchor is invalid')
  }
  requireCondition(exactKeys(anchor, [
    'schemaVersion', 'installationId', 'authorityKeyId', 'authorityPublicKey', 'currentManifestDigest'
  ]) && anchor.schemaVersion === 1 && JSON.stringify(anchor) === value.authorityAnchorV1 &&
    isSHA256(anchor.installationId) && isSHA256(anchor.authorityKeyId) &&
    isSHA256(anchor.currentManifestDigest) && /^[A-Za-z0-9_-]{43}$/u.test(String(anchor.authorityPublicKey || '')),
  'formal authority anchor is invalid')
  const publicKey = Buffer.from(anchor.authorityPublicKey, 'base64url')
  try {
    requireCondition(publicKey.length === 32 && publicKey.toString('base64url') === anchor.authorityPublicKey &&
      sha256(publicKey) === anchor.authorityKeyId,
    'formal authority public key is invalid')
  } finally {
    publicKey.fill(0)
  }
  const roots = [
    value.authorityManifestRoot,
    value.authorityCredentialProfileRoot,
    value.authorityCredentialBundleRoot
  ]
  requireCondition(roots.every((root) => typeof root === 'string' && root !== '' && root === root.trim() &&
    isAbsolute(root) && normalize(root) === root && resolve(root) === root && pathContainedBy(ownerRoot, root) &&
    exactPrivateDirectory(root)) && new Set(roots).size === roots.length,
  'formal authority roots are invalid')
  return {
    protectedAuthorityV1: {
      schemaVersion: 1,
      purpose: authorityBootstrapPurpose,
      authorityAnchorV1: anchor,
      authorityManifestRoot: roots[0],
      authorityCredentialProfileRoot: roots[1],
      authorityCredentialBundleRoot: roots[2]
    },
    identity: {
      installationId: anchor.installationId,
      authorityKeyId: anchor.authorityKeyId,
      currentManifestDigest: anchor.currentManifestDigest
    }
  }
}

function stableReadAuthorityBootstrap(ownerRoot) {
  const bootstrapPath = join(ownerRoot, authorityBootstrapFile)
  const before = lstatSync(bootstrapPath)
  requireCondition(before.isFile() && !before.isSymbolicLink() && before.uid === process.getuid() &&
    (before.mode & 0o7777) === 0o600 && before.size > 0 && before.size <= 32 * 1024,
  'formal authority bootstrap file is invalid')
  const first = readFileSync(bootstrapPath)
  const middle = lstatSync(bootstrapPath)
  const second = readFileSync(bootstrapPath)
  const after = lstatSync(bootstrapPath)
  try {
    for (const state of [middle, after]) {
      requireCondition(state.isFile() && !state.isSymbolicLink() && state.uid === before.uid &&
        state.dev === before.dev && state.ino === before.ino && state.size === before.size &&
        state.mtimeMs === before.mtimeMs && state.ctimeMs === before.ctimeMs,
      'formal authority bootstrap changed during stable read')
    }
    requireCondition(first.equals(second), 'formal authority bootstrap changed during stable read')
    return {
      ...canonicalAuthorityBootstrap(first.toString('utf8'), ownerRoot),
      bootstrapSHA256: sha256(first)
    }
  } finally {
    first.fill(0)
    second.fill(0)
  }
}

function redacted(value) {
  return String(value || '')
    .replace(/\bsk-[A-Za-z0-9_-]{12,}\b/gu, 'sk-<redacted>')
    .replace(/Bearer\s+[A-Za-z0-9._~+/-]+=*/giu, 'Bearer <redacted>')
}

function runtimeErrorCodes(chunks) {
  const matches = String(chunks?.join('') || '').match(
    /\b(?:account_ingress|account_flow|case_entity|data_engine|evidence_registry|funds_query_source|native_component|source_probe)_[a-z0-9_]+\b/gu
  ) || []
  return [...new Set(matches)].sort()
}

function publicEventDiagnostics(events) {
  const names = [...String(events).matchAll(/"(?:kind|code|reason|terminalReason)":"([^"]+)"/gu)]
    .map((match) => match[1])
  return [...new Set(names)].sort()
}

// Keep these closed projections aligned with the public contracts in turns.ts and items.ts.
const publicTurnStatuses = ['queued', 'running', 'completed', 'failed', 'aborted']
const publicTerminalReasons = [
  'success', 'source_unavailable', 'semantic_failure', 'provider_failure', 'cancel',
  'timeout', 'stream_abort', 'recovery', 'approval', 'user_input', 'resume', 'restart',
  'report_fallback', 'step_limit', 'background_completion', 'tool_failure',
  'approval_denied', 'input_cancelled'
]
const publicToolStatuses = ['pending', 'running', 'completed', 'failed', 'aborted']
const publicSettlementReasons = [
  'none', 'success', 'semantic_failure', 'source_unavailable', 'tool_failed',
  'tool_execution_failed', 'account_flow_execution_failed', 'legacy_output_withheld',
  'approval_cancelled', 'approval_denied', 'approval_policy_blocked', 'cancelled',
  'execution_grant_context_mismatch', 'execution_grant_expired', 'execution_grant_invalid',
  'loop_guard', 'not_found', 'publication_receipt_required', 'case_report_publication_receipt_required',
  'runtime_recovered_job_interrupted', 'sandbox_blocked', 'side_effect_duplicate', 'tool_blocked',
  'tool_cancelled', 'tool_completed', 'tool_not_advertised', 'tool_outcome_unknown_after_restart',
  'tool_source_unavailable', 'tool_timeout', 'user_input_cancelled', 'validation_error', 'workspace_escape'
]

function closedEnum(value, allowed, fallback = 'unknown') {
  const normalized = String(value || '')
  return allowed.includes(normalized) ? normalized : fallback
}

function fixedStatusCounts(items) {
  const counts = Object.fromEntries([...publicToolStatuses, 'unknown'].map((status) => [status, 0]))
  for (const item of items) counts[closedEnum(item?.status, publicToolStatuses)] += 1
  return counts
}

function unobservablePublicStage() {
  return {
    label: 'public_stage_unobservable',
    turnTerminalStatus: 'public_stage_unobservable',
    turnTerminalReason: 'public_stage_unobservable',
    toolCallItemCount: 0,
    toolCallStatuses: fixedStatusCounts([]),
    toolResultSettlementItemCount: 0,
    toolResultSettlementStatuses: fixedStatusCounts([]),
    toolResultSettlementReasons: Object.fromEntries(
      [...publicSettlementReasons, 'unknown'].map((reason) => [reason, 0])
    )
  }
}

function publicSSEEvents(body) {
  const events = []
  for (const line of String(body || '').split(/\r?\n/u)) {
    if (!line.startsWith('data:')) continue
    try {
      const value = JSON.parse(line.slice(5).trim())
      if (value && typeof value === 'object' && !Array.isArray(value)) events.push(value)
    } catch {
      // Invalid or non-JSON public SSE data is deliberately unobservable here.
    }
  }
  return events
}

function classifyPublicDurableTurnStage(publicThread, publicEvents, turnID) {
  const turns = Array.isArray(publicThread?.turns) ? publicThread.turns : []
  const turn = turns.find((candidate) => candidate?.id === turnID)
  if (!turn) return unobservablePublicStage()
  const items = Array.isArray(turn.items) ? turn.items : []
  const toolCalls = items.filter((item) => item?.kind === 'tool_call')
  const settlements = items.filter((item) => item?.kind === 'tool_result')
  const reasons = Object.fromEntries([...publicSettlementReasons, 'unknown'].map((reason) => [reason, 0]))
  for (const item of settlements) {
    const candidate = item?.reasonCode ?? item?.output?.reasonCode ?? item?.output?.code
    reasons[closedEnum(candidate, publicSettlementReasons, candidate ? 'unknown' : 'none')] += 1
  }
  const acceptedFinal = turn.acceptedFinalView && typeof turn.acceptedFinalView === 'object'
    ? turn.acceptedFinalView
    : {}
  const terminalEvent = publicSSEEvents(publicEvents).findLast((event) =>
    event?.turnId === turnID && ['turn_completed', 'turn_failed', 'turn_aborted'].includes(event?.kind)) || {}
  return {
    label: 'public_stage_observed',
    turnTerminalStatus: closedEnum(turn.status || terminalEvent.status, publicTurnStatuses),
    turnTerminalReason: closedEnum(
      acceptedFinal.terminalReason || terminalEvent.terminalReason || terminalEvent.reason,
      publicTerminalReasons
    ),
    toolCallItemCount: toolCalls.length,
    toolCallStatuses: fixedStatusCounts(toolCalls),
    toolResultSettlementItemCount: settlements.length,
    toolResultSettlementStatuses: fixedStatusCounts(settlements),
    toolResultSettlementReasons: reasons
  }
}

function completeAccountFlowStage(publicStage, requests) {
  const initialProviderRequestCount = requests.filter((request) => !request.accountFlowSemantic).length
  const semanticFollowupCount = requests.filter((request) => Boolean(request.accountFlowSemantic)).length
  return {
    ...publicStage,
    initialProviderRequestCount,
    semanticFollowupCount,
    enteredProviderFollowup: semanticFollowupCount > 0,
    providerValueSafe: requests.length > 0 && requests.every((request) =>
      request.valueSafe && request.authorizationConfigured)
  }
}

function isAccountFlowStage(value) {
  const statusKeys = [...publicToolStatuses, 'unknown']
  const reasonKeys = [...publicSettlementReasons, 'unknown']
  const countsAreClosed = (record, keys) => exactKeys(record, keys) &&
    keys.every((key) => Number.isInteger(record[key]) && record[key] >= 0)
  return exactKeys(value, [
    'label', 'turnTerminalStatus', 'turnTerminalReason',
    'initialProviderRequestCount', 'semanticFollowupCount',
    'toolCallItemCount', 'toolCallStatuses',
    'toolResultSettlementItemCount', 'toolResultSettlementStatuses',
    'toolResultSettlementReasons', 'enteredProviderFollowup', 'providerValueSafe'
  ]) && ['public_stage_observed', 'public_stage_unobservable'].includes(value.label) &&
    [...publicTurnStatuses, 'unknown', 'public_stage_unobservable'].includes(value.turnTerminalStatus) &&
    [...publicTerminalReasons, 'unknown', 'public_stage_unobservable'].includes(value.turnTerminalReason) &&
    Number.isInteger(value.initialProviderRequestCount) && value.initialProviderRequestCount >= 0 &&
    Number.isInteger(value.semanticFollowupCount) && value.semanticFollowupCount >= 0 &&
    Number.isInteger(value.toolCallItemCount) && value.toolCallItemCount >= 0 &&
    countsAreClosed(value.toolCallStatuses, statusKeys) &&
    Number.isInteger(value.toolResultSettlementItemCount) && value.toolResultSettlementItemCount >= 0 &&
    countsAreClosed(value.toolResultSettlementStatuses, statusKeys) &&
    countsAreClosed(value.toolResultSettlementReasons, reasonKeys) &&
    typeof value.enteredProviderFollowup === 'boolean' && typeof value.providerValueSafe === 'boolean'
}

async function captureAccountFlowFailureStage(runtime, provider, providerRequestStart, threadID, turnID = '') {
  let publicThread = null
  let publicEvents = ''
  try {
    const detail = await httpRequest(`${runtime.ready.url}/v1/threads/${encodeURIComponent(threadID)}`)
    if (detail.status === 200) publicThread = JSON.parse(detail.body)
  } catch {
    publicThread = null
  }
  let attributableTurnID = turnID
  if (!attributableTurnID) {
    const turns = Array.isArray(publicThread?.turns) ? publicThread.turns : []
    if (turns.length === 1 && typeof turns[0]?.id === 'string' && turns[0].id !== '') {
      attributableTurnID = turns[0].id
    }
  }
  if (attributableTurnID) {
    try {
      const events = await httpRequest(`${runtime.ready.url}/v1/threads/${encodeURIComponent(threadID)}/events?since_seq=0`)
      if (events.status === 200) publicEvents = events.body
    } catch {
      publicEvents = ''
    }
  }
  const publicStage = attributableTurnID
    ? classifyPublicDurableTurnStage(publicThread, publicEvents, attributableTurnID)
    : unobservablePublicStage()
  return completeAccountFlowStage(publicStage, provider.requests.slice(providerRequestStart))
}

function httpRequest(url, options = {}) {
  return new Promise((resolvePromise, reject) => {
    const target = new URL(url)
    const requestBody = options.body === undefined ? undefined : String(options.body)
    const request = http.request({
      hostname: target.hostname,
      port: target.port,
      path: `${target.pathname}${target.search}`,
      method: options.method || 'GET',
      timeout: options.timeoutMs || 30_000,
      headers: {
        ...(requestBody === undefined ? {} : {
          'content-type': 'application/json',
          'content-length': Buffer.byteLength(requestBody)
        }),
        ...(options.headers || {})
      }
    }, (response) => {
      let body = ''
      response.setEncoding('utf8')
      response.on('data', (chunk) => { body += chunk })
      response.on('end', () => resolvePromise({ status: response.statusCode || 0, body }))
    })
    request.once('timeout', () => request.destroy(new Error('HTTP request timed out')))
    request.once('error', reject)
    if (requestBody !== undefined) request.write(requestBody)
    request.end()
  })
}

async function httpJSON(url, options = {}) {
  const response = await httpRequest(url, options)
  let decoded
  try {
    decoded = JSON.parse(response.body)
  } catch {
    throw new Error(`HTTP ${response.status} returned invalid JSON`)
  }
  if (response.status < 200 || response.status >= 300) {
    throw new Error(`HTTP ${response.status}: ${redacted(response.body.slice(0, 240))}`)
  }
  return { status: response.status, body: decoded }
}

function providerSSE(response, delta, finishReason) {
  response.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8' })
  response.end([
    `data: ${JSON.stringify({ choices: [{ delta, finish_reason: finishReason }] })}`,
    'data: {"choices":[],"usage":{"prompt_tokens":16,"completion_tokens":4,"total_tokens":20}}',
    'data: [DONE]'
  ].join('\n\n'))
}

function requestMarker(body) {
  const encoded = JSON.stringify(body)
  for (const mode of ['BEFORE_ADMISSION', 'HEALTHY', 'REVOKED', 'STALE', 'DAMAGED', 'DISABLED']) {
    if (encoded.includes(`AB_R3_FUNDS_${mode}`)) return { lane: 'funds', mode, revision: 'AB_R3' }
    if (encoded.includes(`AB_R3_ORDINARY_${mode}`)) return { lane: 'ordinary', mode, revision: 'AB_R3' }
    if (encoded.includes(`AB_R2_FUNDS_${mode}`)) return { lane: 'funds', mode }
    if (encoded.includes(`AB_R2_ORDINARY_${mode}`)) return { lane: 'ordinary', mode }
  }
  return { lane: '', mode: '' }
}

function providerToolNames(body) {
  return Array.isArray(body?.tools)
    ? body.tools.map((tool) => String(tool?.function?.name || tool?.name || '')).filter(Boolean)
    : []
}

function findPurposeObject(value, purpose) {
  if (typeof value === 'string') {
    try {
      return findPurposeObject(JSON.parse(value), purpose)
    } catch {
      return null
    }
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findPurposeObject(item, purpose)
      if (found) return found
    }
    return null
  }
  if (!value || typeof value !== 'object') return null
  if (value.purpose === purpose) return value
  for (const item of Object.values(value)) {
    const found = findPurposeObject(item, purpose)
    if (found) return found
  }
  return null
}

async function startProviderHarness(privatePathFragments = []) {
  const requests = []
  const server = http.createServer((request, response) => {
    void (async () => {
      if (request.method !== 'POST' || request.url !== '/v1/chat/completions') {
        response.writeHead(404, { 'content-type': 'application/json' })
        response.end('{"error":{"message":"not found"}}')
        return
      }
      let raw = ''
      request.setEncoding('utf8')
      for await (const chunk of request) raw += chunk
      let body = {}
      try {
        body = JSON.parse(raw)
      } catch {
        response.writeHead(400, { 'content-type': 'application/json' })
        response.end('{"error":{"message":"invalid request"}}')
        return
      }
      const marker = requestMarker(body)
      const accountFlowSemantic = findPurposeObject(body?.messages, accountFlowModelPurpose)
      const forbiddenProviderValues = [
        privateSentinel,
        '6222021234567890001',
        '6222021234567890',
        'SYNTHID0001',
        'AuthorityEntityRef',
        'aer1_',
        'cer1_',
        '.duckdb',
        'SELECT ',
        ...privatePathFragments
      ]
      const record = {
        marker,
        toolNames: providerToolNames(body),
        sentinelAbsent: !raw.includes(privateSentinel),
        valueSafe: forbiddenProviderValues.every((value) => !raw.includes(value)),
        accountFlowSemantic,
        authorizationConfigured: request.headers.authorization === `Bearer ${providerKey}`
      }
      requests.push(record)
      if (marker.lane === 'ordinary') {
        providerSSE(response, {
          content: `${marker.revision === 'AB_R3' ? 'AB_R3' : 'AB_R2'}_ORDINARY_PROVIDER_OK_${marker.mode}`
        }, 'stop')
        return
      }
      if (marker.lane === 'funds') {
        if (marker.revision === 'AB_R3' && !accountFlowSemantic) {
          providerSSE(response, {
            role: 'assistant',
            tool_calls: [{
              index: 0,
              id: 'call_ab_r3_account_flow_1',
              type: 'function',
              function: { name: accountFlowToolName, arguments: JSON.stringify(accountFlowArguments) }
            }]
          }, 'tool_calls')
          return
        }
        if (marker.revision === 'AB_R3') {
          providerSSE(response, { content: `AB_R3_FUNDS_PROVIDER_TERMINAL_${marker.mode}` }, 'stop')
          return
        }
        providerSSE(response, { content: `AB_R2_FUNDS_PROVIDER_TERMINAL_${marker.mode}` }, 'stop')
        return
      }
      providerSSE(response, { content: 'AB_R2_UNEXPECTED_PROVIDER_REQUEST' }, 'stop')
    })().catch((error) => {
      if (!response.headersSent) response.writeHead(500, { 'content-type': 'application/json' })
      response.end(JSON.stringify({ error: { message: redacted(error?.message) } }))
    })
  })
  await new Promise((resolvePromise, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      server.off('error', reject)
      resolvePromise()
    })
  })
  const address = server.address()
  requireCondition(address && typeof address !== 'string', 'provider harness did not bind')
  return {
    url: `http://127.0.0.1:${address.port}`,
    requests,
    close: () => new Promise((resolvePromise) => server.close(() => resolvePromise()))
  }
}

function writeSyntheticWorkspace(workspace, rowLabel) {
  privateDirectory(workspace)
  const metadata = privateDirectory(join(workspace, '.analytix'))
  writeFileSync(join(metadata, 'case-project.json'), JSON.stringify({
    version: 1,
    workspaceRoot: workspace,
    caseId: 'case_ab_r2_local_nonpublishable',
    source: 'analytix-data-analysis',
    updatedAt: '2026-08-27T08:00:00Z'
  }), { mode: 0o600 })
  const columns = [
    '交易卡号', '交易账号', '账户开户名称', '开户人证件号码', '交易时间', '交易金额', '交易余额', '收付标志',
    '交易对手账卡号', '现金标志', '对手户名', '对手身份证号', '对手开户银行', '摘要说明', '交易币种', '交易网点名称',
    '交易网点代码', '交易发生地', '交易是否成功', '传票号', '终端号', 'IP地址', 'MAC地址', '对手交易余额',
    '交易流水号', '日志号', '凭证种类', '凭证号', '交易柜员号', '商户名称', '商户号', '备注', '交易类型', '查询反馈结果原因'
  ]
  const row = Array(columns.length).fill('')
  row[0] = '6222021234567890001'
  row[1] = '6222021234567890'
  row[2] = '合成账户甲'
  row[3] = 'SYNTHID0001'
  row[4] = '2026-08-27 10:00:00'
  row[5] = '12.5'
  row[6] = '1000'
  row[7] = '进'
  row[8] = 'CP001'
  row[9] = '否'
  row[10] = '合成对手甲'
  row[11] = 'SYNTHCPID001'
  row[12] = '合成银行'
  row[13] = '合成交易'
  row[14] = 'CNY'
  row[31] = `${privateSentinel}_${rowLabel}`
  const csv = `${columns.join(',')}\r\n${row.join(',')}\r\n`
  const sourcePath = join(workspace, `synthetic-${rowLabel.toLowerCase()}.csv`)
  writeFileSync(sourcePath, csv, { mode: 0o600 })
  return sourcePath
}

function runMaterialization(runtimeBinary, dataDir, label) {
  const result = spawnSync(runtimeBinary, [
    'bundled-plugin',
    'materialize-funds-v1',
    '--data-dir', dataDir,
    '--invocation-id', sha256(`ab-r2-materialization:${label}`)
  ], {
    cwd: dirname(runtimeBinary),
    env: { ...process.env, ANALYTIX_API_KEY: '', ANALYTIX_RUNTIME_TOKEN: '' },
    encoding: 'utf8',
    stdio: 'pipe',
    timeout: packageOperationTimeoutMs
  })
  requireCondition(result.status === 0 && String(result.stdout).includes('ANALYTIX_BUNDLED_FUNDS_MATERIALIZATION_READY_V1 '),
    `funds materialization failed: exit=${result.status} ${redacted(result.stderr)}`)
  return result.status
}

function appendBounded(chunks, chunk, maxBytes = 64 * 1024) {
  chunks.push(String(chunk))
  while (Buffer.byteLength(chunks.join(''), 'utf8') > maxBytes && chunks.length > 1) chunks.shift()
}

async function waitAuthorityReady(child, stdout) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const line = stdout.join('').split(/\r?\n/u).find((value) => value.startsWith(authorityReadyPrefix))
    if (line) {
      let ready
      try {
        ready = JSON.parse(line.slice(authorityReadyPrefix.length))
      } catch {
        throw new Error('formal authority ready marker is invalid')
      }
      requireCondition(exactKeys(ready, [
        'schemaVersion', 'bootstrapSha256', 'installationAuthorityKeySha256'
      ]) && ready.schemaVersion === 1 && isSHA256(ready.bootstrapSha256) &&
        isSHA256(ready.installationAuthorityKeySha256),
      'formal authority ready marker is invalid')
      return ready
    }
    if (child.exitCode !== null) throw new Error(`formal authority sidecar exited before ready: exit=${child.exitCode}`)
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 50))
  }
  throw new Error('formal authority sidecar readiness timed out')
}

async function stopAuthoritySidecar(sidecar) {
  if (!sidecar?.child || sidecar.child.exitCode !== null) return sidecar?.child?.exitCode ?? 0
  sidecar.child.kill('SIGTERM')
  const stopped = await Promise.race([
    new Promise((resolvePromise) => sidecar.child.once('close', (code) => resolvePromise(code ?? 0))),
    new Promise((resolvePromise) => setTimeout(() => resolvePromise(null), 5000))
  ])
  requireCondition(stopped !== null, 'formal authority sidecar did not exit after SIGTERM')
  return stopped
}

async function startAuthoritySidecar(sidecarBinary, repositoryRoot) {
  requireCondition(statSync(sidecarBinary).isFile(), 'formal authority sidecar binary is invalid')
  const parent = mkdtempSync('/private/tmp/analytix-ab-r2-authority-')
  chmodSync(parent, 0o700)
  let child
  try {
    requireCondition(parent.startsWith('/private/tmp/') && realpathSync(parent) === parent && exactPrivateDirectory(parent),
      'formal authority owner parent is invalid')
    const protectedRoots = [realpathSync(repositoryRoot), realpathSync('/Volumes/AnalytixCache'), realpathSync(homedir())]
    requireCondition(protectedRoots.every((root) => !pathContainedBy(root, parent) && !pathContainedBy(parent, root)),
      'formal authority owner parent overlaps protected storage')
    const ownerRoot = join(parent, 'product-owner')
    requireCondition(!existsSync(ownerRoot), 'formal authority owner root was not fresh')
    const stdout = []
    const stderr = []
    let spawnError
    child = spawn(sidecarBinary, ['--owner-root', ownerRoot], {
      cwd: repositoryRoot,
      env: { ...process.env },
      stdio: ['ignore', 'pipe', 'pipe']
    })
    child.once('error', (error) => { spawnError = error })
    child.stdout.on('data', (chunk) => appendBounded(stdout, chunk))
    child.stderr.on('data', (chunk) => appendBounded(stderr, chunk))
    const ready = await waitAuthorityReady(child, stdout)
    requireCondition(!spawnError && child.exitCode === null, 'formal authority sidecar is not alive')
    const authority = stableReadAuthorityBootstrap(ownerRoot)
    requireCondition(authority.bootstrapSHA256 === ready.bootstrapSha256,
      'formal authority ready marker does not bind the stable bootstrap')
    return { child, parent, ownerRoot, authority }
  } catch (error) {
    let cleanupError
    if (child && child.exitCode === null) {
      try {
        await stopAuthoritySidecar({ child })
      } catch (failure) {
        cleanupError = failure
      }
    }
    rmSync(parent, { recursive: true, force: true })
    if (cleanupError) throw new AggregateError([error, cleanupError], 'formal authority sidecar startup cleanup failed')
    throw error
  }
}

function encodeRuntimePrivateFrame(authority) {
  const document = {
    schemaVersion: 1,
    purpose: startupFramePurpose,
    protectedAuthorityV1: authority.protectedAuthorityV1
  }
  const body = Buffer.from(JSON.stringify(document), 'utf8')
  try {
    requireCondition(body.length > 0 && body.length <= 512 * 1024, 'runtime private startup frame is invalid')
    const frame = Buffer.alloc(8 + body.length)
    frame.writeBigUInt64BE(BigInt(body.length), 0)
    body.copy(frame, 8)
    return frame
  } finally {
    body.fill(0)
  }
}

async function writeRuntimePrivateFrame(child, authority) {
  requireCondition(child.stdin, 'runtime private startup stdin is unavailable')
  const frame = encodeRuntimePrivateFrame(authority)
  try {
    await new Promise((resolvePromise, reject) => {
      const onError = () => reject(new Error('runtime private startup frame write failed'))
      child.stdin.once('error', onError)
      child.stdin.end(frame, () => {
        child.stdin.off('error', onError)
        resolvePromise()
      })
    })
  } finally {
    frame.fill(0)
  }
}

async function waitRuntimeReady(child, stdout, stderr) {
  const deadline = Date.now() + packageOperationTimeoutMs
  while (Date.now() < deadline) {
    const line = stdout.join('').split(/\r?\n/u).find((value) => value.startsWith(readyPrefix))
    if (line) return JSON.parse(line.slice(readyPrefix.length))
    if (child.exitCode !== null) {
      throw new Error(`runtime exited before ready: exit=${child.exitCode} ${redacted(stderr.join('').slice(-1000))}`)
    }
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 50))
  }
  throw new Error('runtime readiness timed out')
}

async function stopRuntime(child) {
  if (!child || child.exitCode !== null) return child?.exitCode ?? 0
  child.kill('SIGTERM')
  const stopped = await Promise.race([
    new Promise((resolvePromise) => child.once('close', (code) => resolvePromise(code ?? 0))),
    new Promise((resolvePromise) => setTimeout(() => resolvePromise(null), 5000))
  ])
  if (stopped !== null) return stopped
  child.kill('SIGKILL')
  return await new Promise((resolvePromise) => child.once('close', (code) => resolvePromise(code ?? 1)))
}

async function startRuntime(runtimeBinary, profile, dataDir, provider, authority) {
  requireCondition(isAbsolute(dataDir) && normalize(dataDir) === dataDir && exactPrivateDirectory(dataDir),
    'runtime data directory is not the protected authority service data root')
  const durableRoot = privateDirectory(join(profile, 'durable'))
  const userDataDir = privateDirectory(join(profile, 'user-data'))
  const stdout = []
  const stderr = []
  const child = spawn(runtimeBinary, [
    '-insecure',
    '-data-dir', dataDir,
    '-durable-root', durableRoot,
    '-user-data-dir', userDataDir,
    '-provider-id', providerID,
    '-base-url', `${provider.url}/v1`,
    '-model', providerModel,
    '-endpoint-format', 'chat_completions',
    '--private-startup-frame-v1'
  ], {
    cwd: profile,
    env: (() => {
      const environment = {
      ...process.env,
      ANALYTIX_API_KEY: providerKey,
      ANALYTIX_MODEL_PROVIDERS: '',
      ANALYTIX_MCP_CONFIG_PATH: '',
      ANALYTIX_MCP_CONFIG_JSON: '',
      ANALYTIX_RUNTIME_TOKEN: ''
      }
      for (const name of [
        'ANALYTIX_AUTHORITY_ANCHOR_V1',
        'ANALYTIX_AUTHORITY_MANIFEST_ROOT',
        'ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT',
        'ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT'
      ]) delete environment[name]
      return environment
    })(),
    stdio: ['pipe', 'pipe', 'pipe']
  })
  child.stdout.on('data', (chunk) => appendBounded(stdout, chunk, 256 * 1024))
  child.stderr.on('data', (chunk) => appendBounded(stderr, chunk, 256 * 1024))
  try {
    await writeRuntimePrivateFrame(child, authority)
    const ready = await waitRuntimeReady(child, stdout, stderr)
    requireCondition(ready.productionRuntime === true, 'runtime did not identify as analytix_prod')
    requireCondition(ready.witnessedAuthorityV2Configured === true &&
      ready.witnessedAuthorityInstallationId === authority.identity.installationId &&
      ready.witnessedAuthorityKeyId === authority.identity.authorityKeyId &&
      ready.witnessedAuthorityManifestDigest === authority.identity.currentManifestDigest,
    'runtime did not witness the exact sidecar authority identity')
    requireCondition(ready.datasetSnapshotSelectionV2Configured === false &&
      ready.datasetSnapshotAdmissionV2State === 'absent' &&
      ready.datasetSnapshotAdmissionV2InstallationId === '' &&
      ready.datasetSnapshotAdmissionV2RuntimeLaunchNonce === '' &&
      ready.datasetSnapshotAdmissionV2StagingBindingDigest === '' &&
      ready.datasetSnapshotAdmissionV2SelectionDigest === '' &&
      ready.datasetSnapshotAdmissionV2SnapshotId === '' &&
      ready.datasetSnapshotAdmissionV2AuthorityRecordDigest === '' &&
      ready.datasetSnapshotAdmissionV2AckHmacSha256 === '',
    'ordinary startup manufactured dataset snapshot selection authority')
    return { child, ready, stdout, stderr, dataDir, durableRoot, userDataDir }
  } catch (error) {
    await stopRuntime(child)
    throw error
  }
}

async function waitTurn(baseURL, threadID, turnID) {
  const deadline = Date.now() + timeoutMs
  let last = ''
  while (Date.now() < deadline) {
    const response = await httpRequest(`${baseURL}/v1/threads/${encodeURIComponent(threadID)}/events?since_seq=0`)
    last = response.body
    if (response.status === 200 && last.includes(turnID) && last.includes('"kind":"turn_completed"')) return last
    await new Promise((resolvePromise) => setTimeout(resolvePromise, 50))
  }
  throw new Error(`turn ${turnID} did not complete: ${redacted(last.slice(-1000))}`)
}

async function runTurn(runtime, workspace, prompt, accountFlowCapture = null) {
  const created = await httpJSON(`${runtime.ready.url}/v1/threads`, {
    method: 'POST',
    body: JSON.stringify({
      title: prompt.slice(0, 80),
      workspace,
      providerId: providerID,
      model: providerModel,
      mode: 'agent'
    })
  })
  const threadID = String(created.body?.id || '')
  requireCondition(created.status === 201 && threadID, 'thread creation failed')
  const turnBody = JSON.stringify({
    prompt,
    providerId: providerID,
    model: providerModel,
    approvalPolicy: 'never',
    sandboxMode: 'read-only',
    disableUserInput: true
  })
  let turnID = ''
  if (accountFlowCapture) {
    try {
      const started = await httpRequest(`${runtime.ready.url}/v1/threads/${encodeURIComponent(threadID)}/turns`, {
        method: 'POST', body: turnBody
      })
      if (started.status === 202) turnID = String(JSON.parse(started.body)?.turnId || '')
    } catch {
      turnID = ''
    }
    if (!turnID) {
      throw await captureAccountFlowFailureStage(
        runtime, accountFlowCapture.provider, accountFlowCapture.providerRequestStart, threadID
      )
    }
  } else {
    const started = await httpJSON(`${runtime.ready.url}/v1/threads/${encodeURIComponent(threadID)}/turns`, {
      method: 'POST', body: turnBody
    })
    turnID = String(started.body?.turnId || '')
    requireCondition(started.status === 202 && turnID, 'turn start failed')
  }
  let events
  try {
    events = await waitTurn(runtime.ready.url, threadID, turnID)
  } catch (error) {
    if (!accountFlowCapture) throw error
    throw await captureAccountFlowFailureStage(
      runtime, accountFlowCapture.provider, accountFlowCapture.providerRequestStart, threadID, turnID
    )
  }
  let publicThread = null
  if (accountFlowCapture) {
    try {
      const detail = await httpRequest(`${runtime.ready.url}/v1/threads/${encodeURIComponent(threadID)}`)
      if (detail.status === 200) publicThread = JSON.parse(detail.body)
    } catch {
      publicThread = null
    }
  }
  return { threadID, turnID, events, publicThread }
}

async function runOrdinaryTurn(runtime, provider, workspace, mode) {
  const before = provider.requests.length
  const revision = accountFlowDenominator ? 'AB_R3' : 'AB_R2'
  const marker = `${revision}_ORDINARY_${mode}`
  const result = await runTurn(runtime, workspace, `Return exactly ${marker}. This is an ordinary Provider turn.`)
  const requests = provider.requests.slice(before)
  requireCondition(requests.length === 1 && requests[0].marker.lane === 'ordinary' &&
    requests[0].marker.mode === mode && requests[0].authorizationConfigured &&
    result.events.includes(`${revision}_ORDINARY_PROVIDER_OK_${mode}`),
  `ordinary Provider turn failed after ${mode}: requests=${JSON.stringify(requests)} public_events=${JSON.stringify(publicEventDiagnostics(result.events))}`)
  requireCondition(!result.events.includes(privateSentinel), `private source reached ordinary public events after ${mode}`)
  return { providerRequests: requests.length, publicTerminal: true }
}

async function runFundsTurn(runtime, provider, workspace, mode, expectedProviderMinimum) {
  const before = provider.requests.length
  const revision = accountFlowDenominator ? 'AB_R3' : 'AB_R2'
  const ingress = accountFlowDenominator ? ' 银行账号为 6222021234567890。' : ''
  const accountFlowCapture = accountFlowDenominator && mode === 'HEALTHY'
    ? { provider, providerRequestStart: before }
    : null
  const result = await runTurn(
    runtime, workspace, `${casePromptPrefix}${ingress} ${revision}_FUNDS_${mode}`, accountFlowCapture
  )
  const requests = provider.requests.slice(before)
  const turnChecksPassed = requests.length >= expectedProviderMinimum &&
    requests.every((request) => request.sentinelAbsent && request.authorizationConfigured) &&
    !result.events.includes(privateSentinel)
  if (!turnChecksPassed && accountFlowCapture) {
    throw completeAccountFlowStage(
      classifyPublicDurableTurnStage(result.publicThread, result.events, result.turnID), requests
    )
  }
  requireCondition(requests.length >= expectedProviderMinimum,
    `Funds ${mode} provider request count was ${requests.length}; public=${JSON.stringify(publicEventDiagnostics(result.events))}`)
  requireCondition(requests.every((request) => request.sentinelAbsent && request.authorizationConfigured),
    `Funds ${mode} provider projection was unsafe`)
  requireCondition(!result.events.includes(privateSentinel), `private source reached Funds ${mode} public events`)
  return {
    requests,
    events: result.events,
    publicThread: result.publicThread,
    turnID: result.turnID
  }
}

async function admitCSV(runtime, workspace, sourcePath) {
  const headers = { 'X-Analytix-Local-Display': 'typed-v1' }
  const stagedResponse = await httpRequest(`${runtime.ready.url}/v1/local-display/funds-import/stage`, {
    method: 'POST', headers,
    body: JSON.stringify({ workspaceRoot: workspace, sourcePath })
  })
  if (stagedResponse.status < 200 || stagedResponse.status >= 300) {
    throw new Error(`funds CSV stage failed closed: status=${stagedResponse.status}`)
  }
  requireCondition(Buffer.byteLength(stagedResponse.body) > 0 && Buffer.byteLength(stagedResponse.body) <= 64 * 1024,
    'funds CSV stage response violated the closed success contract')
  let staged
  try {
    staged = JSON.parse(stagedResponse.body)
  } catch {
    throw new Error(`funds CSV stage failed closed: status=${stagedResponse.status}`)
  }
  requireCondition(exactKeys(staged, ['status', 'totalRowCount', 'items']) && staged.status === 'ready' &&
    staged.totalRowCount === 1 && Array.isArray(staged.items) && staged.items.length === 1 &&
    exactKeys(staged.items[0], [
      'selector', 'sourceIndex', 'sourceCount', 'sourceLabel', 'rowCount', 'columnCount', 'status'
    ]) && staged.items[0].sourceIndex === 1 && staged.items[0].sourceCount === 1 &&
    staged.items[0].rowCount === 1 && Number.isInteger(staged.items[0].columnCount) &&
    staged.items[0].columnCount > 0 && staged.items[0].status === 'ready',
  'funds CSV stage response violated the closed success contract')
  const selector = String(staged.items[0].selector || '')
  requireCondition(selector.startsWith('tlsel1_'), 'funds CSV stage did not produce a selector')
  const confirmedResponse = await httpRequest(`${runtime.ready.url}/v1/local-display/funds-import/confirm`, {
    method: 'POST', headers,
    body: JSON.stringify({ selector }),
    timeoutMs
  })
  if (confirmedResponse.status < 200 || confirmedResponse.status >= 300) {
    throw new Error(`funds CSV confirm failed closed: status=${confirmedResponse.status}`)
  }
  requireCondition(Buffer.byteLength(confirmedResponse.body) > 0 && Buffer.byteLength(confirmedResponse.body) <= 64 * 1024,
    'funds CSV confirmation violated the closed success contract')
  let confirmed
  try {
    confirmed = JSON.parse(confirmedResponse.body)
  } catch {
    throw new Error(`funds CSV confirm failed closed: status=${confirmedResponse.status}`)
  }
  const sourceArtifactSha256 = String(confirmed.sourceArtifactSha256 || '')
  requireCondition(exactKeys(confirmed, [
    'sourceArtifactSha256', 'sourceArtifactByteLength', 'sourceRowCount'
  ]) && isSHA256(sourceArtifactSha256) && confirmed.sourceArtifactByteLength > 0 && confirmed.sourceRowCount === 1,
  'funds CSV confirmation violated the closed success contract')
  requireCondition(confirmedResponse.status === 200 && confirmed.sourceRowCount === 1,
    'funds CSV confirmation did not admit one row')
  return { sourceRowCount: confirmed.sourceRowCount }
}

async function observeFundsSourceProbeCount(runtime, expected, allowAbsent = false) {
  const response = await httpJSON(`${runtime.ready.url}/v1/runtime/tools`)
  const servers = Array.isArray(response.body?.mcpServers) ? response.body.mcpServers : []
  const funds = servers.filter((server) => server?.id === 'analytix_funds')
  requireCondition(funds.length <= 1, 'runtime tools returned ambiguous analytix_funds diagnostics')
  if (funds.length === 0) {
    requireCondition(allowAbsent && expected === 0, 'runtime tools omitted required analytix_funds diagnostics')
    return { serverObserved: false, sourceProbeCount: 0, toolCount: 0 }
  }
  const record = funds[0]
  requireCondition(Number.isInteger(record.sourceProbeCount) && record.sourceProbeCount >= 0 &&
    record.sourceProbeCount <= 1_000_000_000 && record.sourceProbeCount === expected,
  `runtime tools sourceProbeCount mismatch: expected=${expected}`)
  for (const forbidden of [
    'sourceReady', 'sourceProbeDigest', 'sourceCaseId', 'datasetSnapshotId', 'rowCount',
    'projectionDigest', 'rawResultSHA256', 'grantDigest', 'authorityDigest', 'connectionEpoch',
    'sourcePath', 'sourceValue', 'error'
  ]) {
    requireCondition(!Object.prototype.hasOwnProperty.call(record, forbidden),
      `runtime tools exposed forbidden Funds diagnostic ${forbidden}`)
  }
  requireCondition(!JSON.stringify(response.body).includes(privateSentinel),
    'runtime tools exposed private source bytes')
  requireCondition(Number.isInteger(record.toolCount) && record.toolCount >= 0 && record.toolCount <= 2,
    'runtime tools returned an invalid Funds tool count')
  return { serverObserved: true, sourceProbeCount: record.sourceProbeCount, toolCount: record.toolCount }
}

function evidenceRegistryAbsent(dataDir) {
  return !existsSync(join(dataDir, 'private', 'evidence-registry'))
}

function filesContaining(root, needle, excludedRoot = '') {
  const matches = []
  const stack = [root]
  const excluded = excludedRoot ? resolve(excludedRoot) : ''
  while (stack.length > 0) {
    const current = stack.pop()
    const absolute = resolve(current)
    if (excluded && (absolute === excluded || absolute.startsWith(excluded + sep))) continue
    const stat = lstatSync(absolute)
    if (stat.isSymbolicLink()) continue
    if (stat.isDirectory()) {
      for (const entry of readdirSync(absolute)) stack.push(join(absolute, entry))
      continue
    }
    if (!stat.isFile() || stat.size > 128 * 1024 * 1024) continue
    if (readFileSync(absolute).includes(Buffer.from(needle))) matches.push(relative(root, absolute))
  }
  return matches.sort()
}

function replaceComponentForRunningOwner(componentPath, scratchRoot, componentMode) {
  const backup = join(scratchRoot, 'data-engine.backup')
  const restored = join(scratchRoot, 'data-engine.restored')
  copyFileSync(componentPath, backup)
  chmodSync(backup, componentMode)
  const fd = openSync(componentPath, 'r+')
  try {
    writeSync(fd, Buffer.from([0]), 0, 1, 0)
  } finally {
    closeSync(fd)
  }
  copyFileSync(backup, restored)
  chmodSync(restored, componentMode)
  renameSync(restored, componentPath)
  requireCondition(sha256(readFileSync(componentPath)) === sha256(readFileSync(backup)),
    'packaged data-engine restoration failed')
}

async function run() {
  const repositoryRoot = realpathSync(process.cwd())
  requireCondition(existsSync(join(repositoryRoot, 'packages', 'runtime-go', 'go.mod')),
    'smoke must run from the canonical repository root')
  const appPath = cachePath(argValue('--app'), 'packaged app')
  requireCondition(basename(appPath) === 'analytix.app' && lstatSync(appPath).isDirectory(), 'packaged app is invalid')
  const sidecarBinary = cachePath(argValue('--authority-sidecar'), 'formal authority sidecar')
  requireCondition(basename(sidecarBinary) === 'formal-authority-sidecar', 'formal authority sidecar path is invalid')
  const runtimeBinary = join(appPath, 'Contents', 'Resources', 'runtime-go', 'bin', 'runtime-server')
  const markerPath = join(appPath, 'Contents', 'Resources', 'runtime', 'analytix-native-development-build.json')
  const marker = JSON.parse(readFileSync(markerPath, 'utf8'))
  requireCondition(marker.classification === 'development_non_publishable' && marker.publishable === false &&
    marker.releaseEligible === false && marker.authorityUse === 'development_only',
  'package is not exact development_non_publishable')
  const dataEngine = marker.components?.find((component) => component?.id === 'data-engine')
  requireCondition(dataEngine?.binaryName === 'analytix-data-engine', 'data-engine development marker is unavailable')
  const componentPath = join(dirname(markerPath), dataEngine.binaryName)
  requireCondition(statSync(runtimeBinary).isFile() && statSync(componentPath).isFile(), 'packaged runtime closure is incomplete')
  const packagedDataEngineSHA256 = sha256(readFileSync(componentPath))
  const packagedDataEngineMode = statSync(componentPath).mode & 0o7777
  requireCondition(isSHA256(packagedDataEngineSHA256) && Number.isInteger(packagedDataEngineMode) &&
    packagedDataEngineMode > 0,
  'packaged data-engine identity is invalid')

  const tempParent = cachePath(argValue('--temp-root') || '/Volumes/AnalytixCache/development-v3/tmp', 'temp root')
  privateDirectory(tempParent)
  const tempRoot = mkdtempSync(join(tempParent, accountFlowDenominator
    ? 'ab-r3-account-flow-'
    : 'ab-r2-admitted-funds-'))
  chmodSync(tempRoot, 0o700)
  let provider
  let authoritySidecar
  let runtime
  let disabledRuntime
  const report = {
    schemaVersion: 1,
    id: accountFlowDenominator
      ? 'runtime-go-analyze-account-flows-production-denominator-smoke'
      : 'runtime-go-admitted-funds-lifecycle-smoke',
    packageClassification: marker.classification,
    packagePublishable: marker.publishable,
    packageReleaseEligible: marker.releaseEligible,
    authority: {},
    materializationExit: null,
    runtimeExit: null,
    disabledRuntimeExit: null,
    cold: {},
    admission: {},
    healthy: {},
    runtimeCoveredFaults: ['damaged', 'disabled'],
    runtimeExcludedFaults: ['revoked', 'stale'],
    splitFaultEvidence: 'revoked_stale_focused_go_tests_only',
    faults: {},
    privacy: {},
    processBoundary: {},
    passed: false
  }
  try {
    authoritySidecar = await startAuthoritySidecar(sidecarBinary, repositoryRoot)
    report.authority = {
      sidecarReady: true,
      bootstrapCanonical: true,
      privateFrameOnly: true,
      witnessedIdentityAgreement: false,
      aliveAcrossRuntimeProbes: false,
      sidecarStopped: false,
      authorityParentRemoved: false
    }
    provider = await startProviderHarness(accountFlowDenominator ? [tempRoot] : [])
    const profile = privateDirectory(join(tempRoot, 'profile'))
    const workspace = privateDirectory(join(profile, 'workspace'))
    const ordinaryWorkspace = privateDirectory(join(profile, 'ordinary-workspace'))
    const sourcePath = writeSyntheticWorkspace(workspace, 'GENERATION_ONE')
    const dataDir = join(authoritySidecar.ownerRoot, 'data')
    requireCondition(pathContainedBy(authoritySidecar.ownerRoot, dataDir) && exactPrivateDirectory(dataDir),
      'formal authority service data root is unavailable')
    report.materializationExit = runMaterialization(runtimeBinary, dataDir, 'healthy')
    report.cold.registryAbsentBeforeStartup = evidenceRegistryAbsent(dataDir)
    runtime = await startRuntime(runtimeBinary, profile, dataDir, provider, authoritySidecar.authority)
    report.authority.witnessedIdentityAgreement = true
    report.cold.registryAbsentAfterStartup = evidenceRegistryAbsent(runtime.dataDir)
    report.cold.sourceProbeObservation = await observeFundsSourceProbeCount(runtime, 0)
    const beforeAdmission = await runFundsTurn(runtime, provider, workspace, 'BEFORE_ADMISSION', 0)
    requireCondition(beforeAdmission.requests.length === 0 && beforeAdmission.events.includes(sourceUnavailableText),
      'cold profile manufactured current Funds source authority')
    report.cold.countUnavailableBeforeAdmission = true
    report.cold.ordinary = await runOrdinaryTurn(runtime, provider, ordinaryWorkspace, 'BEFORE_ADMISSION')
    report.cold.registryAbsentAfterOrdinary = evidenceRegistryAbsent(runtime.dataDir)

    report.admission = await admitCSV(runtime, workspace, sourcePath)
    report.admission.registryAbsent = evidenceRegistryAbsent(runtime.dataDir)
    report.admission.datasetSnapshotAuthorityPresent = existsSync(join(runtime.dataDir, 'private', 'dataset-snapshot-authority'))

    let healthy
    try {
      healthy = await runFundsTurn(runtime, provider, workspace, 'HEALTHY', 0)
    } catch (error) {
      if (isAccountFlowStage(error)) throw error
      throw new Error(`${error instanceof Error ? error.message : String(error)}; ` +
        `runtime_error_codes=${JSON.stringify(runtimeErrorCodes(runtime.stderr))}`)
    }
    const healthySourceProbe = await observeFundsSourceProbeCount(runtime, 1)
    if (accountFlowDenominator) {
      requireCondition(healthySourceProbe.toolCount === 2,
        `production account-flow catalog tool count was ${healthySourceProbe.toolCount}`)
    }
    requireCondition(provider.requests.every((request) =>
      !request.toolNames.includes('mcp__analytix_funds__count_case_rows')),
    'internal count_case_rows reached a Provider request')
    if (accountFlowDenominator) {
        const publicStage = classifyPublicDurableTurnStage(healthy.publicThread, healthy.events, healthy.turnID)
        const initialRequest = healthy.requests.find((request) => !request.accountFlowSemantic)
        const semanticRequest = healthy.requests.find((request) => request.accountFlowSemantic)
        const envelope = semanticRequest?.accountFlowSemantic
        const semantic = envelope?.data
        const sourceFieldReference = semantic?.outcome?.sourceFieldReference
        const providerValueSafe = healthy.requests.every((request) =>
          request.valueSafe && request.authorizationConfigured)
        const stageDiagnostics = completeAccountFlowStage(publicStage, healthy.requests)
        const exactAccountFlowSatisfied = report.admission.sourceRowCount === 1 && healthy.requests.length === 2 &&
          initialRequest?.toolNames.includes(accountFlowToolName) &&
          !initialRequest.toolNames.includes('mcp__analytix_funds__count_case_rows') &&
          envelope?.schemaVersion === 3 && envelope.purpose === accountFlowModelPurpose &&
          envelope.semanticStatus === 'success' &&
          semantic?.subjectAlias === accountFlowArguments.subject_alias &&
          semantic?.startInclusive === accountFlowArguments.start_inclusive &&
          semantic?.endInclusive === accountFlowArguments.end_inclusive &&
          semantic?.currency === 'CNY' && semantic?.minorUnitScale === 2 &&
          semantic?.inflowMinor === '1250' && semantic?.outflowMinor === '0' &&
          semantic?.netMinor === '1250' && semantic?.transactionCount === 1 &&
          semantic?.evidenceTransactionCount === 1 && semantic?.evidenceRowLimit === 1 &&
          semantic?.aggregateComplete === true && semantic?.evidenceRowsComplete === true &&
          semantic?.currentness === 'current' && semantic?.coverage?.state === 'complete' &&
          Array.isArray(semantic.coverage.gaps) && semantic.coverage.gaps.length === 0 &&
          semantic.coverage.normalizedSnapshotRows === 1 && semantic.coverage.acceptedSnapshotRows === 1 &&
          semantic.coverage.rejectedSnapshotRows === 0 && semantic.coverage.duplicateSnapshotRows === 0 &&
          semantic.coverage.untimedSubjectRows === 0 && semantic.coverage.observedMatchingRows === 1 &&
          isSHA256(semantic?.queryHash) &&
          isSHA256(semantic?.resultHash) && Array.isArray(semantic?.transactions) &&
          semantic.transactions.length === 1 &&
          /^srow1_[a-f0-9]{64}$/.test(String(semantic.transactions[0]?.evidenceRef || '')) &&
          sourceFieldReference?.schemaVersion === 1 &&
          sourceFieldReference?.purpose === 'analytix.account-flow-typed-source-field-reference/v1' &&
          /^afslot1_[a-f0-9]{64}$/.test(String(sourceFieldReference?.bindingRef || '')) &&
          sourceFieldReference?.field === 'account' && providerValueSafe &&
          !healthy.events.includes(privateSentinel)
        if (!exactAccountFlowSatisfied) throw stageDiagnostics
        const unavailable = await runFundsTurn(runtime, provider, ordinaryWorkspace, 'STALE', 0)
        requireCondition(unavailable.requests.length === 0 && unavailable.events.includes(sourceUnavailableText),
          'workspace without current authority advertised or executed account-flow analysis')
        const healthyOrdinary = await runOrdinaryTurn(runtime, provider, ordinaryWorkspace, 'HEALTHY')
        report.healthy = {
          accountFlowExecuted: true,
          sourceProbeObservation: healthySourceProbe,
          providerRequestCount: healthy.requests.length,
          subjectAlias: semantic.subjectAlias,
          inflowMinor: semantic.inflowMinor,
          outflowMinor: semantic.outflowMinor,
          netMinor: semantic.netMinor,
          transactionCount: semantic.transactionCount,
          evidenceTransactionCount: semantic.evidenceTransactionCount,
          evidenceRefBounded: true,
          sourceFieldReferenceBounded: true,
          aggregateComplete: semantic.aggregateComplete,
          evidenceRowsComplete: semantic.evidenceRowsComplete,
          currentness: semantic.currentness,
          coverage: semantic.coverage,
          queryHash: semantic.queryHash,
          resultHash: semantic.resultHash,
          internalCountNotProviderAdvertised: true,
          unavailableWithoutCurrentAuthority: true,
          ordinary: healthyOrdinary,
          registryAbsent: evidenceRegistryAbsent(runtime.dataDir)
        }
        report.runtimeCoveredFaults = ['current_authority_unavailable']
        report.runtimeExcludedFaults = ['damaged', 'disabled', 'revoked']
        report.splitFaultEvidence = 'not_executed_outside_focused_denominator'
        report.runtimeExit = await stopRuntime(runtime.child)
        runtime = null
        requireCondition(report.runtimeExit === 0, `account-flow runtime exit=${report.runtimeExit}`)
        report.disabledRuntimeExit = 'not_executed'
        report.authority.aliveAcrossRuntimeProbes = authoritySidecar.child.exitCode === null
    } else {
        requireCondition(report.admission.sourceRowCount === 1 && healthy.requests.length === 0 &&
          healthy.events.includes(sourceUnavailableText) && !healthy.events.includes(privateSentinel),
        'healthy count-only Funds boundary was not fail closed before Provider dispatch')
        const healthyOrdinary = await runOrdinaryTurn(runtime, provider, ordinaryWorkspace, 'HEALTHY')
        requireCondition(provider.requests.every((request) =>
          !request.toolNames.includes('mcp__analytix_funds__count_case_rows')),
        'internal count_case_rows reached a Provider request after the healthy ordinary turn')
        report.healthy = {
          hostSourceReadCanaryPassed: true,
          sourceProbeObservation: healthySourceProbe,
          protectedFundsProviderRequests: healthy.requests.length,
          fundsAnalysisUnavailable: true,
          internalCountNotProviderAdvertised: true,
          ordinary: healthyOrdinary,
          registryAbsent: evidenceRegistryAbsent(runtime.dataDir)
        }

      replaceComponentForRunningOwner(componentPath, tempRoot, packagedDataEngineMode)
      const damaged = await runFundsTurn(runtime, provider, workspace, 'DAMAGED', 0)
      requireCondition(damaged.requests.length === 0 && damaged.events.includes(sourceUnavailableText) &&
        !damaged.events.includes(privateSentinel),
      'damaged native component did not close only the protected Funds boundary')
      const damagedSourceProbe = await observeFundsSourceProbeCount(runtime, 0)
      const restoredComponentMode = statSync(componentPath).mode & 0o7777
      const pathArtifactRestored = sha256(readFileSync(componentPath)) === packagedDataEngineSHA256 &&
        restoredComponentMode === packagedDataEngineMode
      requireCondition(pathArtifactRestored, 'data-engine package artifact bytes were not restored')
      report.faults.damaged = {
        fundsClosed: true,
        sourceUnavailable: true,
        protectedFundsProviderRequests: damaged.requests.length,
        countAdmissionClosed: damagedSourceProbe.sourceProbeCount === 0,
        sourceProbeObservation: damagedSourceProbe,
        pathArtifactRestored,
        ordinary: await runOrdinaryTurn(runtime, provider, ordinaryWorkspace, 'DAMAGED')
      }
      requireCondition(provider.requests.every((request) =>
        !request.toolNames.includes('mcp__analytix_funds__count_case_rows')),
      'internal count_case_rows reached a Provider request after damaged Funds closure')

      report.runtimeExit = await stopRuntime(runtime.child)
      runtime = null
      requireCondition(report.runtimeExit === 0, `healthy/fault runtime exit=${report.runtimeExit}`)

      const disabledProfile = join(tempRoot, 'disabled-profile')
      cpSync(profile, disabledProfile, { recursive: true, preserveTimestamps: true, verbatimSymlinks: true })
      const disabledRemovalTargets = [
        join(authoritySidecar.ownerRoot, '.state', 'bundled-plugin-materialization', 'v1'),
        join(authoritySidecar.ownerRoot, 'plugins', 'cache', 'analytix-hub', 'analytix-fund-analysis')
      ]
      for (const target of disabledRemovalTargets) {
        requireCondition(target !== authoritySidecar.ownerRoot && pathContainedBy(authoritySidecar.ownerRoot, target),
          'disabled Funds removal target escaped the isolated authority owner root')
        rmSync(target, { recursive: true, force: true })
      }
      disabledRuntime = await startRuntime(runtimeBinary, disabledProfile, dataDir, provider, authoritySidecar.authority)
      const disabledWorkspace = join(disabledProfile, 'workspace')
      const disabled = await runFundsTurn(disabledRuntime, provider, disabledWorkspace, 'DISABLED', 0)
      requireCondition(disabled.requests.length === 0 && disabled.events.includes(sourceUnavailableText) &&
        !disabled.events.includes(privateSentinel),
      'package declaration or marker minted Funds without admitted installation')
      const disabledSourceProbe = await observeFundsSourceProbeCount(disabledRuntime, 0, true)
      report.faults.disabled = {
        currentDatasetRetained: existsSync(join(disabledRuntime.dataDir, 'private', 'dataset-snapshot-authority')),
        fundsClosed: true,
        sourceUnavailable: true,
        protectedFundsProviderRequests: disabled.requests.length,
        countAdmissionClosed: disabledSourceProbe.sourceProbeCount === 0,
        sourceProbeObservation: disabledSourceProbe,
        ordinary: await runOrdinaryTurn(disabledRuntime, provider, join(disabledProfile, 'ordinary-workspace'), 'DISABLED')
      }
      requireCondition(provider.requests.every((request) =>
        !request.toolNames.includes('mcp__analytix_funds__count_case_rows')),
      'internal count_case_rows reached a Provider request after disabled Funds closure')
      report.disabledRuntimeExit = await stopRuntime(disabledRuntime.child)
      disabledRuntime = null
      requireCondition(report.disabledRuntimeExit === 0, `disabled runtime exit=${report.disabledRuntimeExit}`)
      report.authority.aliveAcrossRuntimeProbes = authoritySidecar.child.exitCode === null
    }

    const userDataRoot = join(profile, 'user-data')
    const dataAnalysisRoot = join(userDataRoot, 'data-analysis')
    const protectedMatches = [
      ...filesContaining(join(dataDir, 'private'), privateSentinel),
      ...filesContaining(dataAnalysisRoot, privateSentinel)
    ]
    const publicForbiddenValues = accountFlowDenominator
      ? [privateSentinel, '6222021234567890001', '6222021234567890', 'SYNTHID0001', 'AuthorityEntityRef', 'aer1_', 'cer1_', '.duckdb', 'SELECT ', tempRoot]
      : [privateSentinel]
    const publicDurableMatches = publicForbiddenValues.flatMap((value) => [
      ...filesContaining(join(profile, 'durable'), value),
      ...filesContaining(userDataRoot, value, dataAnalysisRoot),
      ...filesContaining(dataDir, value, join(dataDir, 'private'))
    ])
    report.privacy = {
      privateSourceExact: readFileSync(sourcePath, 'utf8').includes(privateSentinel),
      modelSafe: provider.requests.length > 0 && provider.requests.every((request) =>
        request.sentinelAbsent && request.valueSafe),
      publicDurable: publicDurableMatches.length === 0,
      publicDurableMatchCount: publicDurableMatches.length,
      protectedLocal: protectedMatches.length > 0,
      protectedLocalMatchCount: protectedMatches.length,
      evidenceRegistryAbsent: evidenceRegistryAbsent(dataDir)
    }
      report.processBoundary = {
        providerRequests: provider.requests.length,
        ordinaryProviderTurns: provider.requests.filter((request) => request.marker.lane === 'ordinary').length,
        protectedFundsProviderRequests: provider.requests.filter((request) => request.marker.lane === 'funds').length,
        providerAuthorizationConfigured: provider.requests.every((request) => request.authorizationConfigured),
        internalCountNotProviderAdvertised: provider.requests.every((request) =>
          !request.toolNames.includes('mcp__analytix_funds__count_case_rows')),
        tempScope: 'fresh-isolated-analytix-cache'
      }
      const commonPassed = report.materializationExit === 0 && report.runtimeExit === 0 &&
        Object.values(report.cold).every(Boolean) && report.admission.sourceRowCount === 1 &&
        report.admission.registryAbsent && report.admission.datasetSnapshotAuthorityPresent &&
        report.cold.sourceProbeObservation.sourceProbeCount === 0 &&
        report.healthy.sourceProbeObservation.sourceProbeCount === 1 && report.healthy.ordinary?.publicTerminal &&
        Object.values(report.privacy).every((value) => typeof value === 'number' || value === true) &&
        report.processBoundary.providerAuthorizationConfigured && report.processBoundary.internalCountNotProviderAdvertised &&
        report.authority.aliveAcrossRuntimeProbes
      report.passed = accountFlowDenominator
        ? commonPassed && report.disabledRuntimeExit === 'not_executed' && report.healthy.accountFlowExecuted &&
          report.healthy.inflowMinor === '1250' && report.healthy.outflowMinor === '0' &&
          report.healthy.netMinor === '1250' && report.healthy.transactionCount === 1 &&
          report.healthy.unavailableWithoutCurrentAuthority &&
          report.runtimeCoveredFaults.join(',') === 'current_authority_unavailable' &&
          report.processBoundary.ordinaryProviderTurns === 2 && report.processBoundary.protectedFundsProviderRequests === 2
        : commonPassed && report.disabledRuntimeExit === 0 && report.healthy.hostSourceReadCanaryPassed &&
          report.runtimeCoveredFaults.join(',') === 'damaged,disabled' &&
          report.runtimeExcludedFaults.join(',') === 'revoked,stale' &&
          ['damaged', 'disabled'].every((name) => {
            const fault = report.faults[name]
            return fault?.fundsClosed && fault.countAdmissionClosed && fault.ordinary?.publicTerminal
          }) && report.processBoundary.ordinaryProviderTurns === 4 &&
          report.processBoundary.protectedFundsProviderRequests === 0
  } finally {
    if (runtime) report.runtimeExit = await stopRuntime(runtime.child)
    if (disabledRuntime) report.disabledRuntimeExit = await stopRuntime(disabledRuntime.child)
    if (authoritySidecar) {
      const sidecarExit = await stopAuthoritySidecar(authoritySidecar)
      report.authority.sidecarStopped = sidecarExit === 0 && authoritySidecar.child.exitCode !== null
    }
    if (provider) await provider.close()
    if (!keep) rmSync(tempRoot, { recursive: true, force: true })
    else process.stderr.write(`${accountFlowDenominator ? 'AB_R3' : 'AB_R2'}_TEMP_ROOT=${tempRoot}\n`)
    if (authoritySidecar) {
      if (!keep) {
        rmSync(authoritySidecar.parent, { recursive: true, force: true })
        report.authority.authorityParentRemoved = !existsSync(authoritySidecar.parent)
      } else {
        process.stderr.write(`AB_R3_AUTHORITY_ROOT=${authoritySidecar.ownerRoot}\n`)
      }
    }
    report.processBoundary.runtimeCommandsInactive = (!runtime || runtime.child.exitCode !== null) &&
      (!disabledRuntime || disabledRuntime.child.exitCode !== null)
    report.processBoundary.sidecarInactive = !authoritySidecar || authoritySidecar.child.exitCode !== null
  }
  report.passed = report.passed && report.authority.sidecarStopped && report.authority.authorityParentRemoved &&
    report.processBoundary.runtimeCommandsInactive && report.processBoundary.sidecarInactive
  requireCondition(report.passed, 'lifecycle smoke did not satisfy every focused boundary')
  process.stdout.write(`${JSON.stringify(report, null, 2)}\n`)
}

run().catch((error) => {
  if (accountFlowDenominator) {
    const publicStage = isAccountFlowStage(error)
      ? error
      : completeAccountFlowStage(unobservablePublicStage(), [])
    process.stderr.write(`AB_R3_ACCOUNT_FLOW_SMOKE_FAILED ${JSON.stringify(publicStage)}\n`)
  } else {
    const failure = error?.stack || error
    process.stderr.write(`AB_R2_ADMITTED_FUNDS_SMOKE_FAILED ${redacted(failure)}\n`)
  }
  process.exitCode = 1
})
