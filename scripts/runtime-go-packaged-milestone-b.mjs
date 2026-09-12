#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import {
  createHash,
  createPrivateKey,
  createPublicKey,
  randomBytes,
  verify as verifySignature
} from 'node:crypto'
import {
  accessSync,
  chmodSync,
  closeSync,
  constants,
  existsSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readlinkSync,
  readSync,
  realpathSync,
  readdirSync,
  renameSync,
  rmSync,
  statSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import net from 'node:net'
import { createRequire } from 'node:module'
import { basename, dirname, isAbsolute, join, normalize, relative, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { freshLocalProviderRegistry, localProviderFromObservation, localProviderSecretStoreEvidence, sameLocalProviderAuthority } from './lib/local-provider-acceptance.mjs'
import {
  coordinateLocalCredentialEntry, readLocalCredentialScanSource, scanLocalCredentialIsolation, disposeLocalCredentialScanSource
} from './lib/local-provider-credential-scan.mjs'

const require = createRequire(import.meta.url)
const {
  PACKAGED_BUILD_AUTHORITY_CONTRACT,
  PACKAGED_BUILD_AUTHORITY_FILE,
  _internals: packagedAuthorityContract
} = require('./after-pack.cjs')

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const jsonOutput = args.has('--json') || args.has('--dry-run')
const dryRun = args.has('--dry-run')
const noWrite = args.has('--no-write') || dryRun
const reportOnly = args.has('--no-gate')
const diagnosticUnpackaged = args.has('--diagnostic-unpackaged')
const CACHE_MOUNT = '/Volumes/AnalytixCache'
const DEFAULT_TIMEOUT_MS = 20 * 60 * 1000
const DEFAULT_OUTPUT =
  'docs/analytix/upstreams/runtime-go-live-evidence/packaged-milestone-b.json'
const EXTERNAL_CASE_CONTRACT = 'analytix.milestone-b.real-case-contract.v1'
const EXTERNAL_CASE_PROVENANCE = 'analytix.milestone-b.real-case-provenance.v1'
const DIAGNOSTIC_CASE_CONTRACT = 'analytix.milestone-b.synthetic-case-contract.v1'
const DIAGNOSTIC_CASE_PROVENANCE = 'analytix.milestone-b.synthetic-case-provenance.v1'
const PROVIDER_AUDIT_CONTRACT = 'analytix.milestone-b.provider-request-audit.v1'
const PROVIDER_AUDIT_SCANNER_PATH = fileURLToPath(
  new URL('./provider-request-audit-scanner.mjs', import.meta.url)
)
export const PROVIDER_AUDIT_SCANNER_POLICY = Object.freeze({
  contract: PROVIDER_AUDIT_CONTRACT,
  captureScope: 'outbound-provider-request-bodies-before-network-send',
  matching: 'canonical-protected-value-match/v1',
  exactTextMatching: true,
  decoding: Object.freeze([
    'html-numeric-entities',
    'javascript-unicode-escapes',
    'ascii-percent-escapes'
  ]),
  numericSeparatorElision: Object.freeze([
    'ascii-space',
    'horizontal-tab',
    'no-break-space',
    'figure-space',
    'narrow-no-break-space',
    'hyphen-minus'
  ]),
  boundedMarkupElision: 'html-tags-up-to-512-code-units',
  forbiddenHostMaterial: Object.freeze({
    fieldNames: Object.freeze([
      'caseId',
      'caseBindingHash',
      'resolvedAccountKey',
      'subjectResolutionDigest',
      'expectedProducerContentId',
      'expectedProducerManifestSha256',
      'duckdbContentSnapshotDigest',
      'duckdbSnapshotManifestSha256',
      'materializationIdentity',
      'sourceFileId',
      'sourceRowNumber',
      'querySQL'
    ]),
    databasePathSuffixes: Object.freeze(['.duckdb', '.db', '.sqlite']),
    arbitrarySQLTokens: Object.freeze(['SELECT', 'WITH', 'PRAGMA', 'ATTACH', 'COPY'])
  }),
  requiredProviderSafeSemanticProjection: Object.freeze({
    subjectReferencePattern: '^cer1_[a-p]{64}$',
    requiredFields: Object.freeze([
      'subjectRef',
      'startInclusive',
      'endInclusive',
      'timezone',
      'currency',
      'minorUnitScale',
      'inflowMinor',
      'outflowMinor',
      'netMinor',
      'transactionCount',
      'evidenceTransactionCount',
      'evidenceRowLimit',
      'aggregateComplete',
      'evidenceRowsComplete',
      'counterpartySemanticsComplete',
      'coverage',
      'transactions',
      'queryHash',
      'resultHash'
    ]),
    forbiddenFields: Object.freeze([
      'completeAccount',
      'accountValue',
      'resolvedAccountKey',
      'databasePath',
      'sql',
      'sourceFileId',
      'sourceRowNumber'
    ])
  }),
  coverage: 'every-request-body-byte-without-truncation',
  unreadableOrTruncatedDisposition: 'fail-closed'
})
const RUNTIME_MAIN_OWNED_AUTHORITY_PURPOSE = 'analytix.runtime-main-owned-authority/v1'
const RUNTIME_AUTHORITY_BOOTSTRAP_DIRECTORY = 'runtime-authority-bootstrap-v1'
const RUNTIME_AUTHORITY_BOOTSTRAP_FILE = 'authority-bootstrap-v1.json'
const ACCOUNT_FLOW_TOOL = 'mcp__analytix_funds__analyze_account_flows'
const TOOL_EXECUTION_OBSERVATION_PATH = '/v1/runtime/tool-executions/observe'
const TOOL_EXECUTION_OBSERVATION_FIELDS = Object.freeze([
  'disclosure',
  'dispositionId',
  'executionGrantId',
  'privatePayloadWithheld',
  'receiptId',
  'resultItemDigest',
  'resultItemId',
  'schemaVersion',
  'status',
  'threadId',
  'toolName',
  'turnId',
  'workId'
].sort())
const SAFE_TURN_FAILURE_REASON_CODES = new Set([
  'turn_failed',
  'turn_cancelled',
  'provider_error',
  'provider_authentication_failed',
  'provider_rate_limited',
  'provider_insufficient_balance',
  'provider_endpoint_not_found',
  'provider_request_rejected',
  'provider_unavailable',
  'provider_network_unavailable',
  'provider_timeout',
  'provider_stream_interrupted',
  'provider_stream_failed',
  'provider_model_invalid',
  'provider_not_configured',
  'provider_tool_arguments_invalid',
  'provider_reasoning_markup_invalid',
  'provider_empty_final',
  'attachment_text_fallback_too_large',
  'publication_receipt_required',
  'validation_error',
  'runtime_restarted',
  'turn_recovery_boundary',
  'approval_denied',
  'input_cancelled',
  'turn_step_limit_exceeded',
  'turn_security_authority_unavailable',
  'turn_security_context_invalid',
  'turn_security_workspace_mismatch',
  'turn_security_case_binding_mismatch',
  'turn_security_dataset_snapshot_mismatch',
  'turn_security_risk_policy_mismatch',
  'tool_call_identity_invalid',
  'tool_not_advertised',
  'tool_schema_missing',
  'tool_schema_invalid',
  'tool_private_arguments',
  'tool_source_unavailable',
  'tool_invalid_arguments_storm',
  'tool_failure_storm',
  'execution_grant_rejected',
  'source_probe_unavailable',
  'tool_pending_continuation_invalid',
  'context_window_hard_limit'
])
const ORDINARY_BEFORE_MARKER = 'MILESTONE_B_ORDINARY_BEFORE_OK'
const ORDINARY_AFTER_BLOCK_MARKER = 'MILESTONE_B_ORDINARY_AFTER_BLOCK_OK'
const ORDINARY_BETWEEN_MARKER = 'MILESTONE_B_ORDINARY_BETWEEN_OK'
const ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER = 'MILESTONE_B_ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_OK'
const MIXED_CODE_MARKER = 'MILESTONE_B_MIXED_CODE_TEST_SUBAGENT_OK'
const HISTORICAL_MARKER = 'MILESTONE_B_HISTORICAL_COMPARISON_OK'
const RECOVERED_MARKER = 'MILESTONE_B_RECOVERED_CASE_OK'
const MIXED_CODE_SOURCE_RELATIVE_PATH = 'analytix-rc-ordinary-code.mjs'
const MIXED_CODE_TEST_RELATIVE_PATH = 'analytix-rc-ordinary-code.test.mjs'
const MIXED_CODE_INITIAL_SOURCE = 'export function sum(left, right) { return left - right }\n'
const MIXED_CODE_EXPECTED_SOURCE = 'export function sum(left, right) { return left + right }\n'
const MIXED_CODE_TEST_SOURCE = `import test from 'node:test'\nimport assert from 'node:assert/strict'\nimport { sum } from './analytix-rc-ordinary-code.mjs'\n\ntest('sum adds exact integers', () => {\n  assert.equal(sum(19, 23), 42)\n})\n`
const MIXED_CODE_TEST_COMMAND = `node --test ${MIXED_CODE_TEST_RELATIVE_PATH}`
const MILESTONE_B_READ_ONLY_SUBAGENT_PROFILE = 'milestone-b-readonly'
const PROTECTED_SOURCE_ALIAS_RELATIVE_PATH = 'analytix-rc-protected-source-alias'
const PROTECTED_SOURCE_BYPASS_MARKER = 'MILESTONE_B_PROTECTED_SOURCE_BYPASS_DENIED'
const PROTECTED_SOURCE_BASH_COMMAND = `/bin/cat ${PROTECTED_SOURCE_ALIAS_RELATIVE_PATH}`
const AUTHORITY_LOSS_MARKER = 'MILESTONE_B_AUTHORITY_LOSS_ORDINARY_OK'
const AUTHORITY_LOSS_BASH_COMMAND = `/usr/bin/printf ${AUTHORITY_LOSS_MARKER}`
const ORDINARY_BASH_COMMANDS = Object.freeze(new Map([
  [ORDINARY_BEFORE_MARKER, `/usr/bin/printf ${ORDINARY_BEFORE_MARKER}`],
  [ORDINARY_AFTER_BLOCK_MARKER, `/usr/bin/printf ${ORDINARY_AFTER_BLOCK_MARKER}`],
  [ORDINARY_BETWEEN_MARKER, `/usr/bin/printf ${ORDINARY_BETWEEN_MARKER}`],
  [AUTHORITY_LOSS_MARKER, AUTHORITY_LOSS_BASH_COMMAND],
  [ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER,
    `/usr/bin/printf ${ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER}`]
]))
const hostToolExecutionPrivateBindings = new WeakMap()
const publicRuntimeObservationPrivateBindings = new WeakMap()
const MAX_PUBLIC_SURFACE_BYTES = 4 * 1024 * 1024
const MAX_CSV_BYTES = 64 * 1024 * 1024
const MAX_CSV_ROWS = 10_000
const CANONICAL_HEADERS = Object.freeze([
  '交易卡号',
  '交易账号',
  '账户开户名称',
  '开户人证件号码',
  '交易时间',
  '交易金额',
  '交易余额',
  '收付标志',
  '交易对手账卡号',
  '现金标志',
  '对手户名',
  '对手身份证号',
  '对手开户银行',
  '摘要说明',
  '交易币种',
  '交易网点名称',
  '交易网点代码',
  '交易发生地',
  '交易是否成功',
  '传票号',
  '终端号',
  'IP地址',
  'MAC地址',
  '对手交易余额',
  '交易流水号',
  '日志号',
  '凭证种类',
  '凭证号',
  '交易柜员号',
  '商户名称',
  '商户号',
  '备注',
  '交易类型',
  '查询反馈结果原因'
])
const PROTECTED_CANONICAL_COLUMN_INDICES = Object.freeze([
  0, 1, 2, 3, 8, 10, 11, 19, 20, 21, 22, 24, 25, 27, 28, 29, 30
])
const REQUIRED_CHECK_IDS = Object.freeze([
  'trusted-cache-tmpdir',
  'managed-product-data-authority',
  'formal-packaged-artifact',
  'external-owner-isolated-case',
  'canonical-csv-contract-and-provenance',
  'provider-request-audit-authority',
  'configured-network-provider',
  'typed-local-display',
  'direct-source-preview',
  'packaged-first-launch',
  'ordinary-before-case',
  'case-authority-unavailable-fails-closed',
  'ordinary-after-case-block',
  'native-snapshot-one-staging',
  'funds-catalog-after-snapshot-one',
  'real-account-flow-one',
  'host-funds-invocation-and-final-binding',
  'evidence-claim-final-gate-one',
  'default-redacted-ui-one',
  'ordinary-between-funds',
  'native-snapshot-two-staging',
  'snapshot-evolution-and-history',
  'real-account-flow-two',
  'ordinary-after-typed-local-display',
  'same-thread-additive-sequence',
  'mixed-code-todo-subagent-funds',
  'authority-loss-mixed-turn-and-reacquisition',
  'protected-source-general-tool-bypass-denied',
  'case-switch-epoch-and-default-entity-isolation',
  'fork-subagent-case-isolation',
  'public-pii-scan-zero',
  'provider-payload-pii-scan-zero',
  'provider-semantic-context-contract',
  'typed-local-display-revocation',
  'typed-local-display-rehydration',
  'nonzero-compaction',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'exact-thread-recovery',
  'recovered-case-query',
  'normal-final-quit',
  'zero-residual-processes',
  'sandbox-cleanup-and-case-preservation'
])
const POST_RC_CHECK_IDS = Object.freeze([
  'authorized-joint-case-linkage'
])

// Milestone B is a dependency graph, not one fail-fast sequence. Keep the
// lane state explicit in the report so a Provider failure cannot masquerade
// as an unexecuted local Direct Source Preview or deterministic B1 slice.
const MILESTONE_B_PHASE_LANE_DEFINITIONS = Object.freeze({
  ordinaryProvider: Object.freeze({
    phaseOwner: 'ordinary_provider',
    dependencies: Object.freeze(['packaged_first_launch', 'provider_configuration'])
  }),
  b1DirectSourcePreview: Object.freeze({
    phaseOwner: 'b1_direct_source_preview',
    dependencies: Object.freeze(['packaged_first_launch', 'current_case_snapshot'])
  }),
  b1DeterministicFunds: Object.freeze({
    phaseOwner: 'b1_deterministic_funds',
    dependencies: Object.freeze(['packaged_first_launch', 'current_case_snapshot'])
  }),
  b1AgentAcceptedSlot: Object.freeze({
    phaseOwner: 'b1_agent_accepted_slot',
    dependencies: Object.freeze(['packaged_first_launch', 'provider_configuration', 'funds_result'])
  })
})
const MILESTONE_B_PHASE_LANE_STATUSES = new Set([
  'PASS', 'FAIL', 'BLOCKED', 'UNVERIFIED'
])

export function createMilestoneBPhaseLanes() {
  return Object.fromEntries(Object.entries(MILESTONE_B_PHASE_LANE_DEFINITIONS).map(([id, definition]) => [
    id,
    {
      phaseOwner: definition.phaseOwner,
      dependencies: [...definition.dependencies],
      status: 'UNVERIFIED',
      blocker: 'lane_not_started',
      executed: false,
      not_executed: true
    }
  ]))
}

export function setMilestoneBPhaseLane(lanes, laneId, outcome = {}) {
  const definition = MILESTONE_B_PHASE_LANE_DEFINITIONS[laneId]
  if (!definition || !lanes || typeof lanes !== 'object') {
    throw new Error('milestone_b_phase_lane_invalid')
  }
  const status = MILESTONE_B_PHASE_LANE_STATUSES.has(outcome.status)
    ? outcome.status
    : 'UNVERIFIED'
  const executed = outcome.executed === true
  const rawBlocker = typeof outcome.blocker === 'string' &&
    (outcome.blocker === '' || /^[a-z0-9_]+$/u.test(outcome.blocker))
    ? outcome.blocker
    : status === 'PASS'
      ? ''
      : status === 'UNVERIFIED' && !executed
        ? 'lane_not_started'
        : 'milestone_b_phase_lane_blocked'
  const blocker = normalizeMilestoneBPhaseLaneBlocker(
    laneId,
    rawBlocker,
    status === 'PASS' ? '' : 'milestone_b_phase_lane_blocked'
  )
  lanes[laneId] = {
    phaseOwner: definition.phaseOwner,
    dependencies: [...definition.dependencies],
    status,
    blocker,
    executed,
    not_executed: !executed
  }
  return lanes[laneId]
}

// This pure projection is also used by the focused regression test. It makes
// the non-propagation rule executable: an ordinary failure does not rewrite
// any independent B1 lane's status.
export function evaluateMilestoneBPhaseDAG(outcomes = {}) {
  const lanes = createMilestoneBPhaseLanes()
  for (const laneId of Object.keys(MILESTONE_B_PHASE_LANE_DEFINITIONS)) {
    const outcome = outcomes?.[laneId]
    if (outcome && typeof outcome === 'object') {
      setMilestoneBPhaseLane(lanes, laneId, outcome)
    }
  }
  return lanes
}

const ORDINARY_PROVIDER_CHECK_IDS = Object.freeze([
  'ordinary-before-case',
  'ordinary-after-case-block',
  'ordinary-between-funds',
  'ordinary-after-typed-local-display'
])
const DETERMINISTIC_FUNDS_CHECK_IDS = Object.freeze([
  'native-snapshot-one-staging',
  'funds-catalog-after-snapshot-one',
  'native-snapshot-two-staging'
])
const AGENT_ACCEPTED_SLOT_CHECK_IDS = Object.freeze([
  'case-authority-unavailable-fails-closed',
  'real-account-flow-one',
  'host-funds-invocation-and-final-binding',
  'evidence-claim-final-gate-one',
  'default-redacted-ui-one',
  'typed-local-display',
  'snapshot-evolution-and-history',
  'real-account-flow-two',
  'authority-loss-mixed-turn-and-reacquisition',
  'typed-local-display-revocation',
  'typed-local-display-rehydration',
  'recovered-case-query'
])

function normalizeMilestoneBPhaseLaneBlocker(laneId, blocker, fallback) {
  const safeBlocker = safeDiagnosticCode(blocker, fallback)
  // The deterministic Funds lane may only report what its local producer,
  // storage, typed-renderer, or retained-snapshot seam actually observed.
  // Provider/configuration failures belong to their owning lane and must not
  // become a deterministic Funds blocker by projection.
  if (laneId === 'b1DeterministicFunds' &&
      /^(?:provider|ordinary_provider)(?:_|$)/u.test(safeBlocker)) {
    return 'deterministic_funds_provider_independent_seam_not_completed'
  }
  return safeBlocker
}

function phaseLaneStatusFromChecks(report, checkIds, executed, fallbackBlocker) {
  const entries = checkIds
    .map((id) => report.checks.find((item) => item.id === id))
    .filter(Boolean)
  const failure = entries.find((item) => item.status === 'FAIL')
  if (failure) return { status: 'FAIL', blocker: safeDiagnosticCode(failure.message, fallbackBlocker), executed }
  const blocked = entries.find((item) => item.status === 'BLOCKED')
  if (blocked) return { status: 'BLOCKED', blocker: safeDiagnosticCode(blocked.message, fallbackBlocker), executed }
  if (executed && entries.length > 0 && entries.every((item) => item.status === 'PASS')) {
    return { status: 'PASS', blocker: '', executed: true }
  }
  return {
    status: executed ? 'UNVERIFIED' : 'UNVERIFIED',
    blocker: executed ? fallbackBlocker : 'lane_not_started',
    executed
  }
}

export function finalizeMilestoneBPhaseLanes(report) {
  if (!report || typeof report !== 'object') return report
  const lanes = report.phaseLanes || (report.phaseLanes = createMilestoneBPhaseLanes())
  const existing = (laneId) => lanes[laneId]?.executed === true
  const preserveExplicitBlockedLane = (laneId, summary) => {
    const current = lanes[laneId]
    return current?.status === 'BLOCKED' && current.executed === false
      ? {
          status: 'BLOCKED',
          blocker: current.blocker,
          executed: false
        }
      : summary
  }
  const summaries = {
    ordinaryProvider: preserveExplicitBlockedLane('ordinaryProvider', phaseLaneStatusFromChecks(
      report, ORDINARY_PROVIDER_CHECK_IDS, existing('ordinaryProvider'), 'ordinary_provider_lane_not_completed'
    )),
    b1DirectSourcePreview: preserveExplicitBlockedLane('b1DirectSourcePreview', phaseLaneStatusFromChecks(
      report, ['direct-source-preview'], existing('b1DirectSourcePreview'), 'direct_source_preview_lane_not_completed'
    )),
    b1DeterministicFunds: preserveExplicitBlockedLane('b1DeterministicFunds', phaseLaneStatusFromChecks(
      report,
      DETERMINISTIC_FUNDS_CHECK_IDS,
      existing('b1DeterministicFunds'),
      'deterministic_funds_provider_independent_seam_not_completed'
    )),
    b1AgentAcceptedSlot: preserveExplicitBlockedLane('b1AgentAcceptedSlot', phaseLaneStatusFromChecks(
      report, AGENT_ACCEPTED_SLOT_CHECK_IDS, existing('b1AgentAcceptedSlot'), 'agent_accepted_slot_lane_not_completed'
    ))
  }
  for (const [laneId, outcome] of Object.entries(summaries)) {
    setMilestoneBPhaseLane(lanes, laneId, outcome)
  }
  return report
}

function optionValue(name, fallback = '') {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const value = rawArgs[index]
    if (value.startsWith(inlinePrefix)) return value.slice(inlinePrefix.length)
    if (value === name && rawArgs[index + 1] && !rawArgs[index + 1].startsWith('--')) {
      return rawArgs[index + 1]
    }
  }
  return fallback
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function exactKeys(value, expected) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join('\0') === [...expected].sort().join('\0')
}

function isSHA256(value) {
  return /^[0-9a-f]{64}$/u.test(String(value || ''))
}

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim().toLowerCase() : ''
}

function expectedPackagedSourceCommit() {
  return optionValue(
    '--expected-source-commit',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT || ''
  ).trim().toLowerCase()
}

function sameFileIdentity(left, right) {
  return left.dev === right.dev && left.ino === right.ino && left.size === right.size &&
    left.mtimeMs === right.mtimeMs
}

function hashRegularFile(path, options = {}) {
  let before
  try {
    before = lstatSync(path)
  } catch {
    return { exists: false, regular: false, byteLength: 0, sha256: '', content: null }
  }
  const maximumBytes = options.maximumBytes || Number.MAX_SAFE_INTEGER
  if (!before.isFile() || before.isSymbolicLink() || before.size <= 0 || before.size > maximumBytes) {
    return {
      exists: true,
      regular: false,
      byteLength: Number(before.size || 0),
      sha256: '',
      content: null
    }
  }
  const descriptor = openSync(path, 'r')
  try {
    const opened = fstatSync(descriptor)
    if (!sameFileIdentity(before, opened)) {
      return { exists: true, regular: false, byteLength: opened.size, sha256: '', content: null }
    }
    const hash = createHash('sha256')
    const chunks = options.capture ? [] : null
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let total = 0
    while (true) {
      const count = readSync(descriptor, buffer, 0, buffer.length, null)
      if (count === 0) break
      const chunk = buffer.subarray(0, count)
      hash.update(chunk)
      if (chunks) chunks.push(Buffer.from(chunk))
      total += count
    }
    const after = fstatSync(descriptor)
    const pathAfter = lstatSync(path)
    if (!sameFileIdentity(opened, after) || !sameFileIdentity(after, pathAfter) || total !== after.size) {
      return { exists: true, regular: false, byteLength: total, sha256: '', content: null }
    }
    return {
      exists: true,
      regular: true,
      byteLength: total,
      sha256: hash.digest('hex'),
      content: chunks ? Buffer.concat(chunks, total) : null,
      stat: after
    }
  } finally {
    closeSync(descriptor)
  }
}

function ownerUID() {
  return typeof process.getuid === 'function' ? process.getuid() : null
}

function ownerOnlyDirectory(path) {
  try {
    const stat = lstatSync(path)
    const uid = ownerUID()
    return stat.isDirectory() && !stat.isSymbolicLink() &&
      (uid === null || stat.uid === uid) && (stat.mode & 0o077) === 0
  } catch {
    return false
  }
}

function ownerPrivateRegularFile(path, maximumBytes = Number.MAX_SAFE_INTEGER) {
  const evidence = hashRegularFile(path, { maximumBytes, capture: true })
  const uid = ownerUID()
  return evidence.regular && evidence.stat.nlink === 1 &&
    (uid === null || evidence.stat.uid === uid) && (evidence.stat.mode & 0o077) === 0
      ? evidence
      : { ...evidence, regular: false }
}

function ownerPrivateRegularFileMetadata(path, maximumBytes = Number.MAX_SAFE_INTEGER) {
  try {
    const state = lstatSync(path)
    const uid = ownerUID()
    return state.isFile() && !state.isSymbolicLink() && state.nlink === 1 &&
      state.size > 0 && state.size <= maximumBytes &&
      (uid === null || state.uid === uid) && (state.mode & 0o077) === 0
        ? state
        : null
  } catch {
    return null
  }
}

function exactOwnerPrivateDirectory(path) {
  try {
    const state = lstatSync(path)
    const uid = ownerUID()
    return state.isDirectory() && !state.isSymbolicLink() &&
      (uid === null || state.uid === uid) && (state.mode & 0o777) === 0o700 &&
      (state.mode & 0o7000) === 0
  } catch {
    return false
  }
}

function pathContainedBy(root, candidate) {
  const escaped = relative(root, candidate)
  return escaped !== '..' && !escaped.startsWith(`..${sep}`) && !isAbsolute(escaped)
}

export function prepareMixedCodeWorkspace(workspace) {
  const root = realpathSync(workspace)
  const sourcePath = resolve(root, MIXED_CODE_SOURCE_RELATIVE_PATH)
  const testPath = resolve(root, MIXED_CODE_TEST_RELATIVE_PATH)
  if (!pathContainedBy(root, sourcePath) || !pathContainedBy(root, testPath) ||
      existsSync(sourcePath) || existsSync(testPath)) {
    throw new Error('mixed_code_workspace_not_exclusive')
  }
  try {
    writeFileSync(sourcePath, MIXED_CODE_INITIAL_SOURCE, {
      encoding: 'utf8', mode: 0o600, flag: 'wx'
    })
    writeFileSync(testPath, MIXED_CODE_TEST_SOURCE, {
      encoding: 'utf8', mode: 0o600, flag: 'wx'
    })
  } catch (error) {
    for (const [path, allowed] of [
      [sourcePath, sha256(MIXED_CODE_INITIAL_SOURCE)],
      [testPath, sha256(MIXED_CODE_TEST_SOURCE)]
    ]) {
      const file = ownerPrivateRegularFile(path, 64 * 1024)
      if (file.regular && file.sha256 === allowed) rmSync(path)
    }
    throw error
  }
  return Object.freeze({ root, sourcePath, testPath })
}

export function mixedCodeWorkspaceEvidence(authority) {
  const source = ownerPrivateRegularFile(authority?.sourcePath || '', 64 * 1024)
  const test = ownerPrivateRegularFile(authority?.testPath || '', 64 * 1024)
  return Object.freeze({
    ok: source.regular && source.sha256 === sha256(MIXED_CODE_EXPECTED_SOURCE) &&
      test.regular && test.sha256 === sha256(MIXED_CODE_TEST_SOURCE),
    sourceSha256: source.regular ? source.sha256 : '',
    testSha256: test.regular ? test.sha256 : ''
  })
}

export function cleanupMixedCodeWorkspace(authority) {
  if (!authority) return false
  const allowedSourceHashes = new Set([
    sha256(MIXED_CODE_INITIAL_SOURCE),
    sha256(MIXED_CODE_EXPECTED_SOURCE)
  ])
  for (const [path, allowed] of [
    [authority.sourcePath, allowedSourceHashes],
    [authority.testPath, new Set([sha256(MIXED_CODE_TEST_SOURCE)])]
  ]) {
    const file = ownerPrivateRegularFile(path, 64 * 1024)
    if (!file.regular || !allowed.has(file.sha256)) return false
  }
  rmSync(authority.sourcePath)
  rmSync(authority.testPath)
  return !existsSync(authority.sourcePath) && !existsSync(authority.testPath)
}

export function prepareCaseSwitchAuthority(workspace) {
  const root = realpathSync(workspace)
  const metadata = realpathSync(join(root, '.analytix'))
  const bindingPath = realpathSync(join(metadata, 'case-project.json'))
  const backupPath = join(metadata, 'case-project.json.formal-b-preserved')
  const original = ownerPrivateRegularFile(bindingPath, 1024 * 1024)
  if (!pathContainedBy(root, metadata) || !pathContainedBy(metadata, bindingPath) ||
      existsSync(backupPath) || !original.regular || !original.content) {
    throw new Error('case_switch_binding_authority_invalid')
  }
  let document
  try {
    document = JSON.parse(original.content.toString('utf8'))
  } catch {
    throw new Error('case_switch_binding_authority_invalid')
  }
  if (!exactKeys(document, ['version', 'workspaceRoot', 'caseId', 'source', 'updatedAt']) ||
      document.version !== 1 || document.workspaceRoot !== root ||
      typeof document.caseId !== 'string' || !document.caseId) {
    throw new Error('case_switch_binding_authority_invalid')
  }
  const negativeCaseID = `case_formal_negative_${sha256(canonicalJSON({
    caseId: document.caseId,
    source: document.source
  })).slice(0, 16)}`
  const negativeBody = Buffer.from(JSON.stringify({
    ...document,
    caseId: negativeCaseID,
    updatedAt: new Date().toISOString()
  }))
  return {
    bindingPath,
    backupPath,
    originalBody: Buffer.from(original.content),
    originalSHA256: original.sha256,
    originalIdentity: {
      dev: original.stat.dev,
      ino: original.stat.ino,
      size: original.stat.size,
      mtimeMs: original.stat.mtimeMs
    },
    negativeBody,
    negativeSHA256: sha256(negativeBody),
    switched: false
  }
}

export function switchToNegativeCase(authority) {
  if (!authority || authority.switched || existsSync(authority.backupPath)) return false
  const current = ownerPrivateRegularFile(authority.bindingPath, 1024 * 1024)
  if (!current.regular || current.sha256 !== authority.originalSHA256 ||
      current.stat.dev !== authority.originalIdentity.dev ||
      current.stat.ino !== authority.originalIdentity.ino) return false
  renameSync(authority.bindingPath, authority.backupPath)
  try {
    writeFileSync(authority.bindingPath, authority.negativeBody, {
      mode: 0o600,
      flag: 'wx'
    })
    chmodSync(authority.bindingPath, 0o600)
  } catch (error) {
    if (existsSync(authority.bindingPath)) rmSync(authority.bindingPath)
    renameSync(authority.backupPath, authority.bindingPath)
    throw error
  }
  const negative = ownerPrivateRegularFile(authority.bindingPath, 1024 * 1024)
  authority.switched = negative.regular && negative.sha256 === authority.negativeSHA256
  return authority.switched
}

export function restoreOriginalCase(authority) {
  if (!authority) return false
  if (authority.switched && existsSync(authority.bindingPath)) rmSync(authority.bindingPath)
  if (existsSync(authority.backupPath) && !existsSync(authority.bindingPath)) {
    renameSync(authority.backupPath, authority.bindingPath)
  }
  authority.switched = false
  const restored = ownerPrivateRegularFile(authority.bindingPath, 1024 * 1024)
  return restored.regular && restored.sha256 === authority.originalSHA256 &&
    restored.stat.dev === authority.originalIdentity.dev &&
    restored.stat.ino === authority.originalIdentity.ino &&
    restored.stat.size === authority.originalIdentity.size &&
    restored.stat.mtimeMs === authority.originalIdentity.mtimeMs &&
    !existsSync(authority.backupPath)
}

export function prepareProtectedSourceAlias(workspace, runtimeDataDir) {
  const root = realpathSync(runtimeDataDir)
  const pending = [root]
  const duckDBFiles = []
  let visited = 0
  while (pending.length > 0) {
    const path = pending.pop()
    const state = lstatSync(path)
    visited += 1
    if (visited > 8192 || state.isSymbolicLink()) {
      throw new Error('protected_source_inventory_invalid')
    }
    if (state.isDirectory()) {
      for (const entry of readdirSync(path)) pending.push(join(path, entry))
    } else if (state.isFile() && path.endsWith('.duckdb') && state.size > 0) {
      duckDBFiles.push(realpathSync(path))
    }
  }
  duckDBFiles.sort()
  if (duckDBFiles.length < 2) throw new Error('protected_source_inventory_incomplete')
  const aliasPath = resolve(realpathSync(workspace), PROTECTED_SOURCE_ALIAS_RELATIVE_PATH)
  if (existsSync(aliasPath) || !pathContainedBy(realpathSync(workspace), aliasPath)) {
    throw new Error('protected_source_alias_not_exclusive')
  }
  const targetPath = duckDBFiles.at(-1)
  symlinkSync(targetPath, aliasPath)
  if (!lstatSync(aliasPath).isSymbolicLink() || readlinkSync(aliasPath) !== targetPath) {
    throw new Error('protected_source_alias_invalid')
  }
  return Object.freeze({ aliasPath, targetPath })
}

export function cleanupProtectedSourceAlias(authority) {
  if (!authority) return false
  try {
    if (!lstatSync(authority.aliasPath).isSymbolicLink() ||
        readlinkSync(authority.aliasPath) !== authority.targetPath) return false
    rmSync(authority.aliasPath)
    return !existsSync(authority.aliasPath)
  } catch {
    return false
  }
}

export function managedProductVolumeInfo(path, options = {}) {
  if (process.platform !== 'darwin') {
    return { ok: false, blocker: 'managed_product_data_requires_macos_apfs' }
  }
  const run = options.spawnSync || spawnSync
  const resolveRealPath = options.realpathSync || realpathSync
  const statPath = options.statSync || statSync
  let realPath
  try {
    realPath = resolveRealPath(path)
  } catch {
    return { ok: false, blocker: 'managed_product_data_volume_unavailable' }
  }
  const filesystem = run('/bin/df', ['-P', realPath], {
    encoding: null,
    stdio: ['ignore', 'pipe', 'pipe'],
    maxBuffer: 1024 * 1024
  })
  const filesystemLines = filesystem.status === 0 && Buffer.isBuffer(filesystem.stdout)
    ? filesystem.stdout.toString('utf8').trim().split('\n')
    : []
  const filesystemFields = filesystemLines.length >= 2
    ? filesystemLines.at(-1).trim().split(/\s+/u)
    : []
  const devicePath = filesystemFields[0] || ''
  if (!/^\/dev\/disk[0-9A-Za-z]+$/u.test(devicePath)) {
    return { ok: false, blocker: 'managed_product_data_volume_unavailable' }
  }
  const disk = run('/usr/sbin/diskutil', ['info', '-plist', devicePath], {
    encoding: null,
    stdio: ['ignore', 'pipe', 'pipe'],
    maxBuffer: 1024 * 1024
  })
  if (disk.status !== 0 || !Buffer.isBuffer(disk.stdout) || disk.stdout.length === 0) {
    return { ok: false, blocker: 'managed_product_data_volume_unavailable' }
  }
  const converted = run('/usr/bin/plutil', ['-convert', 'json', '-o', '-', '-'], {
    input: disk.stdout,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'pipe'],
    maxBuffer: 1024 * 1024
  })
  let info
  try {
    info = converted.status === 0 ? JSON.parse(String(converted.stdout || '')) : null
  } catch {
    info = null
  }
  if (!info || typeof info !== 'object' || Array.isArray(info)) {
    return { ok: false, blocker: 'managed_product_data_volume_unavailable' }
  }
  let mountPoint
  let deviceBound = false
  try {
    mountPoint = resolveRealPath(String(info.MountPoint || ''))
    deviceBound = statPath(realPath).dev === statPath(mountPoint).dev &&
      info.DeviceIdentifier === devicePath.slice('/dev/'.length)
  } catch {
    return { ok: false, blocker: 'managed_product_data_volume_unavailable' }
  }
  const safe = info.FilesystemType === 'apfs' && info.Writable === true &&
    info.WritableVolume === true && info.GlobalPermissionsEnabled === true &&
    info.Internal === true && info.Removable !== true && info.RemovableMedia !== true &&
    info.RemovableMediaOrExternalDevice !== true && info.Ejectable !== true &&
    info.APFSSnapshot !== true &&
    typeof info.VolumeUUID === 'string' && info.VolumeUUID.length > 0 &&
    deviceBound
  return {
    ok: safe,
    blocker: safe ? '' : 'managed_product_data_volume_is_not_internal_nonremovable_apfs',
    mountPoint,
    volumeUUID: String(info.VolumeUUID || ''),
    deviceIdentifier: String(info.DeviceIdentifier || '')
  }
}

function canonicalAuthorityBootstrapEnvelope(value) {
  if (!exactKeys(value, [
    'schemaVersion',
    'purpose',
    'authorityAnchorV1',
    'authorityManifestRoot',
    'authorityCredentialProfileRoot',
    'authorityCredentialBundleRoot'
  ]) || value.schemaVersion !== 1 ||
      value.purpose !== RUNTIME_MAIN_OWNED_AUTHORITY_PURPOSE ||
      typeof value.authorityAnchorV1 !== 'string') {
    throw new Error('managed_product_authority_bootstrap_invalid')
  }
  let anchor
  try {
    anchor = JSON.parse(value.authorityAnchorV1)
  } catch {
    throw new Error('managed_product_authority_bootstrap_invalid')
  }
  if (!exactKeys(anchor, [
    'schemaVersion', 'installationId', 'authorityKeyId', 'authorityPublicKey',
    'currentManifestDigest'
  ]) || anchor.schemaVersion !== 1 || JSON.stringify(anchor) !== value.authorityAnchorV1 ||
      !isSHA256(anchor.installationId) || !isSHA256(anchor.authorityKeyId) ||
      !isSHA256(anchor.currentManifestDigest) ||
      !/^[A-Za-z0-9_-]{43}$/u.test(String(anchor.authorityPublicKey || ''))) {
    throw new Error('managed_product_authority_bootstrap_invalid')
  }
  let publicKey
  try {
    publicKey = Buffer.from(anchor.authorityPublicKey, 'base64url')
    if (publicKey.length !== 32 || publicKey.toString('base64url') !== anchor.authorityPublicKey ||
        sha256(publicKey) !== anchor.authorityKeyId) {
      throw new Error('managed_product_authority_bootstrap_invalid')
    }
  } finally {
    publicKey?.fill(0)
  }
  const roots = [
    value.authorityManifestRoot,
    value.authorityCredentialProfileRoot,
    value.authorityCredentialBundleRoot
  ]
  if (roots.some((root) => typeof root !== 'string' || root === '' ||
      root !== root.trim() || !isAbsolute(root) || normalize(root) !== root ||
      resolve(root) !== root || root.includes('\0')) || new Set(roots).size !== roots.length) {
    throw new Error('managed_product_authority_bootstrap_invalid')
  }
  return Object.freeze({
    envelope: Object.freeze({ ...value }),
    roots: Object.freeze(roots),
    anchor: Object.freeze({ ...anchor })
  })
}

function validateInstallationAuthorityKey(path, ownerRoot, startedAt, anchor) {
  const keyPath = realpathSync(path)
  if (lstatSync(path).isSymbolicLink() || keyPath !== path ||
      !pathContainedBy(ownerRoot, keyPath)) {
    throw new Error('managed_product_installation_authority_key_invalid')
  }
  const file = ownerPrivateRegularFile(keyPath, 4096)
  if (!file.regular || !file.content || file.stat.mtimeMs >= startedAt.getTime()) {
    throw new Error('managed_product_installation_authority_key_invalid')
  }
  let record
  try {
    record = JSON.parse(file.content.toString('utf8'))
  } catch {
    throw new Error('managed_product_installation_authority_key_invalid')
  }
  if (!exactKeys(record, ['schemaVersion', 'algorithm', 'keyId', 'publicKey', 'privateSeed']) ||
      record.schemaVersion !== 1 || record.algorithm !== 'Ed25519' ||
      !isSHA256(record.keyId) || !/^[A-Za-z0-9_-]{43}$/u.test(record.publicKey) ||
      !/^[A-Za-z0-9_-]{43}$/u.test(record.privateSeed) ||
      file.content.toString('utf8') !== JSON.stringify(record)) {
    throw new Error('managed_product_installation_authority_key_invalid')
  }
  const publicKey = Buffer.from(record.publicKey, 'base64url')
  const privateSeed = Buffer.from(record.privateSeed, 'base64url')
  try {
    if (publicKey.length !== 32 || privateSeed.length !== 32 ||
        publicKey.toString('base64url') !== record.publicKey ||
        privateSeed.toString('base64url') !== record.privateSeed ||
        sha256(publicKey) !== record.keyId || record.keyId !== anchor.authorityKeyId ||
        record.publicKey !== anchor.authorityPublicKey) {
      throw new Error('managed_product_installation_authority_key_invalid')
    }
    const privateKey = createPrivateKey({
      key: {
        kty: 'OKP',
        crv: 'Ed25519',
        x: record.publicKey,
        d: record.privateSeed
      },
      format: 'jwk'
    })
    const derived = createPublicKey(privateKey).export({ format: 'jwk' })
    if (derived.x !== record.publicKey) {
      throw new Error('managed_product_installation_authority_key_invalid')
    }
  } catch {
    throw new Error('managed_product_installation_authority_key_invalid')
  } finally {
    publicKey.fill(0)
    privateSeed.fill(0)
  }
  return Object.freeze({ path: keyPath, sha256: file.sha256 })
}

/**
 * Validates the external operator-owned product root used by formal Milestone B.
 * The cache volume remains the only cache/build/temp owner; product state and
 * protected authority roots must instead live on one internal non-removable
 * APFS volume that the Go secure readers will independently revalidate.
 */
export function loadMilestoneBManagedProductAuthority(input) {
  const startedAt = input.startedAt instanceof Date ? input.startedAt : new Date(input.startedAt)
  if (Number.isNaN(startedAt.getTime())) {
    throw new Error('managed_product_data_started_at_invalid')
  }
  const ownerRoot = realpathSync(input.ownerRoot)
  const bootstrapPath = realpathSync(input.bootstrapPath)
  const installationAuthorityKeyPath = realpathSync(input.installationAuthorityKeyPath)
  const repositoryRoot = realpathSync(input.repositoryRoot || process.cwd())
  const cacheRoot = realpathSync(input.cacheRoot || CACHE_MOUNT)
  if (lstatSync(input.ownerRoot).isSymbolicLink() ||
      lstatSync(input.bootstrapPath).isSymbolicLink() ||
      ownerRoot !== input.ownerRoot || bootstrapPath !== input.bootstrapPath ||
      !exactOwnerPrivateDirectory(ownerRoot) ||
      pathContainedBy(repositoryRoot, ownerRoot) || pathContainedBy(ownerRoot, repositoryRoot) ||
      pathContainedBy(cacheRoot, ownerRoot) || pathContainedBy(ownerRoot, cacheRoot) ||
      !pathContainedBy(ownerRoot, bootstrapPath) ||
      basename(bootstrapPath) !== RUNTIME_AUTHORITY_BOOTSTRAP_FILE) {
    throw new Error('managed_product_data_owner_isolation_invalid')
  }
  const ownerState = lstatSync(ownerRoot)
  const bootstrap = parseJSONFile(bootstrapPath, 32 * 1024)
  if (ownerState.mtimeMs >= startedAt.getTime() ||
      bootstrap.file.stat.mtimeMs >= startedAt.getTime()) {
    throw new Error('managed_product_data_authority_not_preexisting')
  }
  const canonical = canonicalAuthorityBootstrapEnvelope(bootstrap.value)
  if (!bootstrap.file.content ||
      bootstrap.file.content.toString('utf8') !== JSON.stringify(canonical.envelope)) {
    throw new Error('managed_product_authority_bootstrap_not_canonical')
  }
  const volumeInfo = typeof input.volumeInfo === 'function'
    ? input.volumeInfo
    : managedProductVolumeInfo
  const ownerVolume = volumeInfo(ownerRoot)
  if (!ownerVolume?.ok || !ownerVolume.volumeUUID) {
    throw new Error(ownerVolume?.blocker || 'managed_product_data_volume_unavailable')
  }
  const resolvedRoots = canonical.roots.map((root) => {
    const resolved = realpathSync(root)
    const state = lstatSync(root)
    const volume = volumeInfo(resolved)
    if (state.isSymbolicLink() || resolved !== root || resolved === ownerRoot ||
        !exactOwnerPrivateDirectory(resolved) ||
        state.mtimeMs >= startedAt.getTime() || !pathContainedBy(ownerRoot, resolved) ||
        !volume?.ok || volume.volumeUUID !== ownerVolume.volumeUUID) {
      throw new Error('managed_product_protected_authority_root_invalid')
    }
    return resolved
  })
  if (new Set(resolvedRoots).size !== resolvedRoots.length ||
      resolvedRoots.some((left, index) => resolvedRoots.some((right, other) =>
        index !== other && (pathContainedBy(left, right) || pathContainedBy(right, left))
      ))) {
    throw new Error('managed_product_protected_authority_roots_overlap')
  }
  const installationAuthorityKey = validateInstallationAuthorityKey(
    installationAuthorityKeyPath,
    ownerRoot,
    startedAt,
    canonical.anchor
  )
  return Object.freeze({
    contract: RUNTIME_MAIN_OWNED_AUTHORITY_PURPOSE,
    ownerRoot,
    bootstrapPath,
    bootstrapSha256: bootstrap.file.sha256,
    canonicalBootstrap: bootstrap.file.content.toString('utf8'),
    installationAuthorityKeyPath: installationAuthorityKey.path,
    installationAuthorityKeySha256: installationAuthorityKey.sha256,
    authorityRootCount: resolvedRoots.length,
    volumeUUID: ownerVolume.volumeUUID,
    volumeIdentityDigest: sha256(canonicalJSON({
      volumeUUID: ownerVolume.volumeUUID,
      mountPoint: ownerVolume.mountPoint,
      deviceIdentifier: ownerVolume.deviceIdentifier
    }))
  })
}

function configuredManagedProductAuthority(startedAt, cache) {
  const ownerRoot = optionValue(
    '--product-data-owner-root',
    process.env.ANALYTIX_MILESTONE_B_PRODUCT_DATA_OWNER_ROOT || ''
  ).trim()
  const bootstrapPath = optionValue(
    '--authority-bootstrap',
    process.env.ANALYTIX_MILESTONE_B_AUTHORITY_BOOTSTRAP || ''
  ).trim()
  const installationAuthorityKeyPath = optionValue(
    '--installation-authority-key',
    process.env.ANALYTIX_MILESTONE_B_INSTALLATION_AUTHORITY_KEY || ''
  ).trim()
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    contract: RUNTIME_MAIN_OWNED_AUTHORITY_PURPOSE,
    ownerRootPathHash: ownerRoot ? sha256(ownerRoot) : '',
    bootstrapPathHash: bootstrapPath ? sha256(bootstrapPath) : '',
    installationAuthorityKeyPathHash: installationAuthorityKeyPath
      ? sha256(installationAuthorityKeyPath)
      : '',
    bootstrapSha256: '',
    installationAuthorityKeySha256: '',
    volumeIdentityDigest: '',
    authorityRootCount: 0,
    authority: null
  }
  if (!ownerRoot || !bootstrapPath || !installationAuthorityKeyPath || !cache?.ok) {
    return {
      ...base,
      blocked: true,
      blocker: !cache?.ok
        ? 'trusted_cache_required_before_managed_product_data_validation'
        : 'managed_product_data_owner_root_authority_bootstrap_and_installation_key_required'
    }
  }
  try {
    const authority = loadMilestoneBManagedProductAuthority({
      ownerRoot,
      bootstrapPath,
      installationAuthorityKeyPath,
      repositoryRoot: process.cwd(),
      cacheRoot: CACHE_MOUNT,
      startedAt
    })
    return {
      ...base,
      ok: true,
      ownerRootPathHash: sha256(authority.ownerRoot),
      bootstrapPathHash: sha256(authority.bootstrapPath),
      bootstrapSha256: authority.bootstrapSha256,
      installationAuthorityKeySha256: authority.installationAuthorityKeySha256,
      volumeIdentityDigest: authority.volumeIdentityDigest,
      authorityRootCount: authority.authorityRootCount,
      authority
    }
  } catch (error) {
    return {
      ...base,
      blocker: error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
        ? error.message
        : 'managed_product_data_authority_unavailable'
    }
  }
}

function installManagedAuthorityBootstrap(authority, userDataDir) {
  const root = join(userDataDir, RUNTIME_AUTHORITY_BOOTSTRAP_DIRECTORY)
  const path = join(root, RUNTIME_AUTHORITY_BOOTSTRAP_FILE)
  mkdirSync(root, { recursive: false, mode: 0o700 })
  chmodSync(root, 0o700)
  writeFileSync(path, authority.canonicalBootstrap, {
    encoding: 'utf8', mode: 0o600, flag: 'wx'
  })
  chmodSync(path, 0o600)
  const installed = ownerPrivateRegularFile(path, 32 * 1024)
  if (!exactOwnerPrivateDirectory(root) || !installed.regular ||
      installed.sha256 !== authority.bootstrapSha256 || !installed.content ||
      installed.content.toString('utf8') !== authority.canonicalBootstrap) {
    throw new Error('managed_product_authority_bootstrap_install_failed')
  }
  return { root, path, sha256: installed.sha256 }
}

function seedManagedInstallationAuthority(authority, runtimeDataDir) {
  const privateRoot = join(runtimeDataDir, 'private')
  const authorityRoot = join(privateRoot, 'authority')
  mkdirSync(privateRoot, { recursive: false, mode: 0o700 })
  chmodSync(privateRoot, 0o700)
  mkdirSync(authorityRoot, { recursive: false, mode: 0o700 })
  chmodSync(authorityRoot, 0o700)
  const source = ownerPrivateRegularFile(authority.installationAuthorityKeyPath, 4096)
  if (!source.regular || !source.content ||
      source.sha256 !== authority.installationAuthorityKeySha256) {
    throw new Error('managed_product_installation_authority_seed_invalid')
  }
  const target = join(authorityRoot, 'final-answer-ed25519-v1.json')
  try {
    writeFileSync(target, source.content, { mode: 0o600, flag: 'wx' })
  } finally {
    source.content.fill(0)
  }
  chmodSync(target, 0o600)
  const installed = ownerPrivateRegularFile(target, 4096)
  if (!installed.regular || installed.sha256 !== authority.installationAuthorityKeySha256) {
    throw new Error('managed_product_installation_authority_seed_invalid')
  }
  installed.content?.fill(0)
  return Object.freeze({ target, sha256: installed.sha256 })
}

function removeExactHarnessOwnedRoot(ownerRoot, target, prefix) {
  try {
    const owner = realpathSync(ownerRoot)
    const exact = realpathSync(target)
    if (!pathContainedBy(owner, exact) || dirname(exact) !== owner ||
        !basename(exact).startsWith(prefix)) {
      return false
    }
    rmSync(exact, { recursive: true, force: false })
    return !existsSync(exact)
  } catch {
    return false
  }
}

function prepareMilestoneBRunTopology({
  cache,
  managedProduct,
  workspace,
  runtimePort,
  providerAuditSocketPath
}) {
  let sandboxRoot = ''
  let productRunRoot = ''
  try {
    sandboxRoot = mkdtempSync(join(cache.path, 'analytix-milestone-b-'))
    chmodSync(sandboxRoot, 0o700)
    productRunRoot = mkdtempSync(join(
      managedProduct.authority.ownerRoot,
      'analytix-milestone-b-run-'
    ))
    chmodSync(productRunRoot, 0o700)
    const isolatedHome = join(sandboxRoot, 'home')
    const userDataDir = join(productRunRoot, 'user-data')
    const runtimeDataDir = join(productRunRoot, 'runtime-data')
    mkdirSync(isolatedHome, { recursive: true, mode: 0o700 })
    writeIsolatedSettings({ userDataDir, runtimeDataDir, workspace, runtimePort })
    const seededInstallationAuthority = seedManagedInstallationAuthority(
      managedProduct.authority,
      runtimeDataDir
    )
    const installedBootstrap = installManagedAuthorityBootstrap(
      managedProduct.authority,
      userDataDir
    )
    const childEnv = safeChildEnvironment({
      isolatedHome,
      userDataDir,
      providerAuditSocketPath
    })
    const runVolume = managedProductVolumeInfo(productRunRoot)
    const evidence = {
      bootstrapInstalled: installedBootstrap.sha256 === managedProduct.bootstrapSha256,
      installationAuthoritySeeded:
        seededInstallationAuthority.sha256 === managedProduct.installationAuthorityKeySha256,
      isolatedHomeUsed: childEnv.HOME === isolatedHome,
      isolatedUserDataUsed: childEnv.ANALYTIX_USER_DATA_DIR === userDataDir,
      productOwnerStable:
        realpathSync(managedProduct.authority.ownerRoot) === managedProduct.authority.ownerRoot &&
        dirname(realpathSync(productRunRoot)) === managedProduct.authority.ownerRoot,
      productVolumeRevalidated:
        runVolume.ok === true &&
        runVolume.volumeUUID === managedProduct.authority.volumeUUID,
      productDataOutsideCache:
        !pathContainedBy(realpathSync(CACHE_MOUNT), realpathSync(productRunRoot)),
      userDataRuntimeSeparated:
        dirname(userDataDir) === productRunRoot && dirname(runtimeDataDir) === productRunRoot &&
        userDataDir !== runtimeDataDir && !pathContainedBy(userDataDir, runtimeDataDir) &&
        !pathContainedBy(runtimeDataDir, userDataDir),
      externalCaseOutsideSandbox:
        !pathContainedBy(sandboxRoot, workspace) && !pathContainedBy(productRunRoot, workspace)
    }
    if (Object.values(evidence).some((value) => value !== true)) {
      throw new Error('managed_product_runtime_topology_invalid')
    }
    return {
      sandboxRoot,
      productRunRoot,
      isolatedHome,
      userDataDir,
      runtimeDataDir,
      childEnv,
      evidence
    }
  } catch (error) {
    if (productRunRoot) {
      removeExactHarnessOwnedRoot(
        managedProduct.authority.ownerRoot,
        productRunRoot,
        'analytix-milestone-b-run-'
      )
    }
    if (sandboxRoot) {
      removeExactHarnessOwnedRoot(cache.path, sandboxRoot, 'analytix-milestone-b-')
    }
    throw error
  }
}

function safeRelativePath(value) {
  if (typeof value !== 'string' || !value || isAbsolute(value) || value.includes('\0')) return false
  const normalized = value.replaceAll('\\', '/')
  return !normalized.split('/').some((part) => !part || part === '.' || part === '..')
}

function parseJSONFile(path, maximumBytes = 1024 * 1024) {
  const file = ownerPrivateRegularFile(path, maximumBytes)
  if (!file.regular || !file.content) throw new Error('owner_private_json_file_invalid')
  let value
  try {
    value = JSON.parse(file.content.toString('utf8'))
  } catch {
    throw new Error('owner_private_json_invalid')
  }
  if (file.content.toString('utf8') !== JSON.stringify(value) &&
      file.content.toString('utf8') !== `${JSON.stringify(value)}\n`) {
    throw new Error('owner_private_json_not_canonical')
  }
  return { value, file }
}

function parseCSVRows(buffer) {
  const text = buffer.toString('utf8')
  if (Buffer.from(text, 'utf8').length !== buffer.length || text.startsWith('\ufeff')) {
    throw new Error('canonical_csv_utf8_invalid')
  }
  const rows = []
  let row = []
  let field = ''
  let quoted = false
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index]
    if (quoted) {
      if (character === '"') {
        if (text[index + 1] === '"') {
          field += '"'
          index += 1
        } else {
          quoted = false
        }
      } else {
        field += character
      }
      continue
    }
    if (character === '"' && field === '') {
      quoted = true
    } else if (character === ',') {
      row.push(field)
      field = ''
    } else if (character === '\n' || character === '\r') {
      if (character === '\r' && text[index + 1] === '\n') index += 1
      row.push(field)
      field = ''
      rows.push(row)
      row = []
      if (rows.length > MAX_CSV_ROWS + 1) throw new Error('canonical_csv_row_limit_exceeded')
    } else {
      field += character
    }
  }
  if (quoted) throw new Error('canonical_csv_quote_invalid')
  if (field !== '' || row.length > 0) {
    row.push(field)
    rows.push(row)
  }
  if (rows.length > 0 && rows.at(-1).length === 1 && rows.at(-1)[0] === '') rows.pop()
  return rows
}

function canonicalTimestamp(value) {
  if (!/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/u.test(value)) return false
  const instant = new Date(`${value.replace(' ', 'T')}Z`)
  return !Number.isNaN(instant.getTime()) &&
    instant.toISOString().slice(0, 19) === value.replace(' ', 'T')
}

function canonicalQueryTimestamp(value) {
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/u.test(value) &&
    !Number.isNaN(Date.parse(value))
}

function decimalMinor(value) {
  if (!/^-?(?:0|[1-9]\d*)(?:\.\d{1,2})?$/u.test(value) || value === '-0') {
    throw new Error('canonical_csv_amount_invalid')
  }
  const negative = value.startsWith('-')
  const unsigned = negative ? value.slice(1) : value
  const [integer, fraction = ''] = unsigned.split('.')
  const minor = BigInt(integer) * 100n + BigInt(fraction.padEnd(2, '0') || '0')
  return negative ? -minor : minor
}

function formatMinorAmount(currency, minor, scale) {
  const value = String(minor)
  const negative = value.startsWith('-')
  const digits = negative ? value.slice(1) : value
  const padded = digits.padStart(scale + 1, '0')
  const major = padded.slice(0, -scale)
  const fraction = padded.slice(-scale)
  return `${negative ? '-' : ''}${currency} ${major.replace(/\B(?=(\d{3})+(?!\d))/gu, ',')}.${fraction}`
}

function canonicalSnapshotTruth(buffer, query) {
  const rows = parseCSVRows(buffer)
  if (rows.length < 2 || rows.length > MAX_CSV_ROWS + 1 ||
      canonicalJSON(rows[0]) !== canonicalJSON(CANONICAL_HEADERS)) {
    throw new Error('canonical_csv_header_or_row_count_invalid')
  }
  const start = query.startInclusive.slice(0, 19).replace('T', ' ')
  const end = query.endInclusive.slice(0, 19).replace('T', ' ')
  let inflow = 0n
  let outflow = 0n
  let transactionCount = 0
  const evidenceRows = []
  const protectedValues = new Set([query.completeAccount])
  const sensitiveColumns = PROTECTED_CANONICAL_COLUMN_INDICES
  for (const values of rows.slice(1)) {
    if (values.length !== CANONICAL_HEADERS.length || values.some((value) =>
      value !== value.trim() || [...value].some((character) => {
        const code = character.codePointAt(0)
        return code !== undefined && (code <= 0x1f || code === 0x7f)
      }))) {
      throw new Error('canonical_csv_record_invalid')
    }
    const timestamp = values[4]
    const amount = values[5]
    const direction = values[7]
    const currency = values[14]
    if (!canonicalTimestamp(timestamp) || (direction !== '进' && direction !== '出') ||
        currency !== 'CNY' || (!values[0] && !values[1])) {
      throw new Error('canonical_csv_record_normalization_invalid')
    }
    const amountValue = decimalMinor(amount)
    if (values[6]) decimalMinor(values[6])
    for (const column of sensitiveColumns) {
      const candidate = values[column]
      if (candidate) protectedValues.add(candidate)
    }
    if ((values[0] === query.completeAccount || values[1] === query.completeAccount) &&
        timestamp >= start && timestamp <= end) {
      const magnitude = amountValue < 0n ? -amountValue : amountValue
      if (direction === '进') inflow += magnitude
      else outflow += magnitude
      transactionCount += 1
      evidenceRows.push(Object.freeze({
        occurredAt: `${timestamp.replace(' ', 'T')}.000000Z`,
        direction: direction === '进' ? '流入' : '流出',
        amount: formatMinorAmount('CNY', magnitude.toString(), 2)
      }))
    }
  }
  return {
    sourceRowCount: rows.length - 1,
    protectedValues: [...protectedValues],
    evidenceRows,
    expected: {
      currency: 'CNY',
      minorUnitScale: 2,
      inflowMinor: inflow.toString(),
      outflowMinor: outflow.toString(),
      netMinor: (inflow - outflow).toString(),
      transactionCount,
      evidenceTransactionCount: Math.min(transactionCount, query.evidenceRowLimit)
    }
  }
}

function validExpected(value) {
  return exactKeys(value, [
    'currency', 'minorUnitScale', 'inflowMinor', 'outflowMinor', 'netMinor',
    'transactionCount', 'evidenceTransactionCount'
  ]) && value.currency === 'CNY' && value.minorUnitScale === 2 &&
    /^\d+$/u.test(value.inflowMinor) && /^\d+$/u.test(value.outflowMinor) &&
    /^-?\d+$/u.test(value.netMinor) && Number.isSafeInteger(value.transactionCount) &&
    value.transactionCount > 0 && Number.isSafeInteger(value.evidenceTransactionCount) &&
    value.evidenceTransactionCount === value.transactionCount
}

function validateSnapshotContract(snapshot) {
  return exactKeys(snapshot, ['label', 'sourceRelativePath', 'sourceRevision', 'query', 'expected']) &&
    (snapshot.label === 'baseline' || snapshot.label === 'evolved') &&
    safeRelativePath(snapshot.sourceRelativePath) &&
    Number.isSafeInteger(snapshot.sourceRevision) && snapshot.sourceRevision > 0 &&
    exactKeys(snapshot.query, [
      'completeAccount', 'startInclusive', 'endInclusive', 'evidenceRowLimit'
    ]) && /^\d{8,32}$/u.test(snapshot.query.completeAccount) &&
    canonicalQueryTimestamp(snapshot.query.startInclusive) &&
    canonicalQueryTimestamp(snapshot.query.endInclusive) &&
    snapshot.query.startInclusive <= snapshot.query.endInclusive &&
    Number.isSafeInteger(snapshot.query.evidenceRowLimit) &&
    snapshot.query.evidenceRowLimit > 0 && snapshot.query.evidenceRowLimit <= 512 &&
    validExpected(snapshot.expected)
}

function snapshotProvenanceRecordDigest(caseContractSha256, proof) {
  return sha256(canonicalJSON({
    contract: 'analytix.milestone-b.snapshot-provenance-record.v1',
    caseContractSha256,
    label: proof.label,
    sourceSha256: proof.sourceSha256,
    sourceByteLength: proof.sourceByteLength,
    acquiredAt: proof.acquiredAt
  }))
}

export function loadMilestoneBExternalCaseAcceptance(input) {
  const startedAt = input.startedAt instanceof Date ? input.startedAt : new Date(input.startedAt)
  if (Number.isNaN(startedAt.getTime())) throw new Error('external_case_started_at_invalid')
  const diagnosticSynthetic = input.diagnosticSynthetic === true
  const expectedContract = diagnosticSynthetic ? DIAGNOSTIC_CASE_CONTRACT : EXTERNAL_CASE_CONTRACT
  const expectedProvenance = diagnosticSynthetic
    ? DIAGNOSTIC_CASE_PROVENANCE
    : EXTERNAL_CASE_PROVENANCE
  const expectedClassification = diagnosticSynthetic
    ? 'isolated-synthetic-diagnostic-case'
    : 'external-preexisting-owner-isolated-real-case'
  const ownerRoot = realpathSync(input.ownerRoot)
  const workspace = realpathSync(input.workspace)
  const repositoryRoot = realpathSync(input.repositoryRoot || process.cwd())
  if (!ownerOnlyDirectory(ownerRoot) || !ownerOnlyDirectory(workspace) ||
      !pathContainedBy(ownerRoot, workspace) || pathContainedBy(repositoryRoot, workspace)) {
    throw new Error('external_case_owner_isolation_invalid')
  }
  const contractPath = realpathSync(input.contractPath)
  const provenancePath = realpathSync(input.provenancePath)
  if (lstatSync(input.contractPath).isSymbolicLink() ||
      lstatSync(input.provenancePath).isSymbolicLink()) {
    throw new Error('external_case_contract_provenance_symlink_rejected')
  }
  if (!pathContainedBy(ownerRoot, contractPath) || !pathContainedBy(ownerRoot, provenancePath)) {
    throw new Error('external_case_contract_provenance_outside_owner_root')
  }
  const contractDocument = parseJSONFile(contractPath)
  const provenanceDocument = parseJSONFile(provenancePath)
  if (contractDocument.file.stat.mtimeMs >= startedAt.getTime() ||
      provenanceDocument.file.stat.mtimeMs >= startedAt.getTime()) {
    throw new Error('external_case_contract_provenance_not_preexisting')
  }
  const contract = contractDocument.value
  const provenance = provenanceDocument.value
  if (!exactKeys(contract, ['contract', 'case', 'snapshots']) ||
      contract.contract !== expectedContract ||
      !exactKeys(contract.case, ['classification', 'canonicalProfile']) ||
      contract.case.classification !== expectedClassification ||
      contract.case.canonicalProfile !== 'canonical_direct_csv_v1' ||
      !Array.isArray(contract.snapshots) || contract.snapshots.length !== 2 ||
      contract.snapshots[0]?.label !== 'baseline' || contract.snapshots[1]?.label !== 'evolved' ||
      !contract.snapshots.every(validateSnapshotContract)) {
    throw new Error('external_case_contract_invalid')
  }
  const contractSha256 = contractDocument.file.sha256
  if (!exactKeys(provenance, ['contract', 'caseContractSha256', 'snapshots']) ||
      provenance.contract !== expectedProvenance ||
      provenance.caseContractSha256 !== contractSha256 ||
      !Array.isArray(provenance.snapshots) || provenance.snapshots.length !== 2) {
    throw new Error('external_case_provenance_invalid')
  }
  const snapshots = []
  const protectedValues = new Set()
  for (let index = 0; index < contract.snapshots.length; index += 1) {
    const declared = contract.snapshots[index]
    const proof = provenance.snapshots[index]
    if (!exactKeys(proof, [
      'label', 'sourceSha256', 'sourceByteLength', 'provenanceRecordDigest', 'acquiredAt'
    ]) || proof.label !== declared.label || !isSHA256(proof.sourceSha256) ||
      !Number.isSafeInteger(proof.sourceByteLength) || proof.sourceByteLength <= 0 ||
      !isSHA256(proof.provenanceRecordDigest) || Number.isNaN(Date.parse(proof.acquiredAt)) ||
      Date.parse(proof.acquiredAt) >= startedAt.getTime() ||
      proof.provenanceRecordDigest !== snapshotProvenanceRecordDigest(
        contractSha256,
        proof
      )) {
      throw new Error('external_case_snapshot_provenance_invalid')
    }
    const sourcePath = realpathSync(join(workspace, declared.sourceRelativePath))
    if (!pathContainedBy(workspace, sourcePath)) throw new Error('external_case_csv_outside_workspace')
    const source = ownerPrivateRegularFile(sourcePath, MAX_CSV_BYTES)
    if (!source.regular || !source.content || source.stat.mtimeMs >= startedAt.getTime() ||
        source.sha256 !== proof.sourceSha256 || source.byteLength !== proof.sourceByteLength) {
      throw new Error('external_case_csv_identity_invalid')
    }
    const truth = canonicalSnapshotTruth(source.content, declared.query)
    if (canonicalJSON(truth.expected) !== canonicalJSON(declared.expected)) {
      throw new Error('external_case_declared_truth_mismatch')
    }
    for (const value of truth.protectedValues) protectedValues.add(value)
    snapshots.push(Object.freeze({
      label: declared.label,
      sourcePath,
      sourceSha256: source.sha256,
      sourceByteLength: source.byteLength,
      sourceRevision: declared.sourceRevision,
      sourceRowCount: truth.sourceRowCount,
      protectedValues: Object.freeze([...truth.protectedValues]),
      query: Object.freeze({ ...declared.query }),
      expected: Object.freeze({ ...truth.expected }),
      evidenceRows: Object.freeze(truth.evidenceRows.map((row) =>
        Object.freeze({ ...row })
      )),
      truthDigest: sha256(canonicalJSON({
        expected: truth.expected,
        evidenceRows: truth.evidenceRows
      })),
      initialIdentity: Object.freeze({
        dev: source.stat.dev,
        ino: source.stat.ino,
        size: source.stat.size,
        mtimeMs: source.stat.mtimeMs,
        sha256: source.sha256
      })
    }))
  }
  if (snapshots[0].sourceSha256 === snapshots[1].sourceSha256 ||
      snapshots[0].sourceRevision >= snapshots[1].sourceRevision ||
      snapshots[0].query.completeAccount !== snapshots[1].query.completeAccount ||
      snapshots[0].truthDigest === snapshots[1].truthDigest) {
    throw new Error('external_case_snapshot_evolution_invalid')
  }
  return Object.freeze({
    diagnosticSynthetic,
    workspace,
    ownerRoot,
    contractPath,
    provenancePath,
    contractSha256,
    provenanceSha256: provenanceDocument.file.sha256,
    contractInitialIdentity: Object.freeze({
      dev: contractDocument.file.stat.dev,
      ino: contractDocument.file.stat.ino,
      size: contractDocument.file.stat.size,
      mtimeMs: contractDocument.file.stat.mtimeMs,
      sha256: contractDocument.file.sha256
    }),
    provenanceInitialIdentity: Object.freeze({
      dev: provenanceDocument.file.stat.dev,
      ino: provenanceDocument.file.stat.ino,
      size: provenanceDocument.file.stat.size,
      mtimeMs: provenanceDocument.file.stat.mtimeMs,
      sha256: provenanceDocument.file.sha256
    }),
    snapshots: Object.freeze(snapshots),
    protectedValues: Object.freeze([...protectedValues])
  })
}

export function verifyMilestoneBExternalCasePreserved(authority) {
  try {
    const protectedDocuments = [
      [authority.contractPath, authority.contractInitialIdentity],
      [authority.provenancePath, authority.provenanceInitialIdentity]
    ]
    const documentsPreserved = protectedDocuments.every(([path, initial]) => {
      const current = ownerPrivateRegularFile(path)
      return current.regular && realpathSync(path) === path &&
        current.sha256 === initial.sha256 && current.stat.dev === initial.dev &&
        current.stat.ino === initial.ino && current.stat.size === initial.size &&
        current.stat.mtimeMs === initial.mtimeMs
    })
    return documentsPreserved && authority.snapshots.every((snapshot) => {
      const current = ownerPrivateRegularFile(snapshot.sourcePath, MAX_CSV_BYTES)
      return current.regular && current.sha256 === snapshot.initialIdentity.sha256 &&
        current.stat.dev === snapshot.initialIdentity.dev &&
        current.stat.ino === snapshot.initialIdentity.ino &&
        current.stat.size === snapshot.initialIdentity.size &&
        current.stat.mtimeMs === snapshot.initialIdentity.mtimeMs
    })
  } catch {
    return false
  }
}

function normalizeTargetPlatform(value) {
  if (value === 'windows') return 'win32'
  if (value === 'darwin' || value === 'win32' || value === 'linux') return value
  throw new Error('unsupported_packaged_target_platform')
}

function normalizeTargetArch(value) {
  if (value === 'amd64') return 'x64'
  if (value === 'aarch64') return 'arm64'
  if (value === 'x64' || value === 'arm64') return value
  throw new Error('unsupported_packaged_target_architecture')
}

function packagedTarget() {
  const platform = normalizeTargetPlatform(optionValue(
    '--target-platform',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_PLATFORM || process.platform
  ))
  const arch = normalizeTargetArch(optionValue(
    '--target-arch',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_TARGET_ARCH || process.arch
  ))
  return { platform, arch, key: `${platform}-${arch}` }
}

function defaultPackagedAppPath(target) {
  if (target.platform === 'darwin') {
    return target.arch === 'arm64'
      ? 'dist/mac-arm64/analytix.app'
      : 'dist/mac/analytix.app'
  }
  if (target.platform === 'win32') return 'dist-standard-win/win-unpacked/analytix.exe'
  return 'dist/linux-unpacked/analytix'
}

function packagedAppPath(target) {
  if (diagnosticUnpackaged) return resolve(process.cwd())
  return resolve(process.cwd(), optionValue(
    '--app-path',
    process.env.ANALYTIX_RUNTIME_GO_MILESTONE_B_APP_PATH ||
      process.env.ANALYTIX_RUNTIME_GO_PACKAGED_APP_PATH ||
      process.env.ANALYTIX_PACKAGED_APP_PATH ||
      defaultPackagedAppPath(target)
  ))
}

function executablePath(appPath, target) {
  return target.platform === 'darwin'
    ? join(appPath, 'Contents', 'MacOS', 'analytix')
    : appPath
}

function runtimeServerPath(appPath, target) {
  if (diagnosticUnpackaged) {
    return resolve(optionValue(
      '--diagnostic-runtime-server',
      process.env.ANALYTIX_RUNTIME_GO_MILESTONE_B_DIAGNOSTIC_RUNTIME_SERVER || ''
    ))
  }
  if (target.platform === 'darwin') {
    return join(appPath, 'Contents', 'Resources', 'runtime-go', 'bin', 'runtime-server')
  }
  return join(
    dirname(appPath),
    'resources',
    'runtime-go',
    'bin',
    target.platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
  )
}

function appAsarPath(appPath, target) {
  return target.platform === 'darwin'
    ? join(appPath, 'Contents', 'Resources', 'app.asar')
    : join(dirname(appPath), 'resources', 'app.asar')
}

function expectedPackagedRendererURL() {
  if (diagnosticUnpackaged) {
    return pathToFileURL(resolve(process.cwd(), 'out/renderer/index.html')).href
  }
  const target = packagedTarget()
  const appPath = packagedAppPath(target)
  return pathToFileURL(join(
    appAsarPath(appPath, target),
    'out',
    'renderer',
    'index.html'
  )).href
}

function runtimeResourcePath(appPath, target) {
  return target.platform === 'darwin'
    ? join(appPath, 'Contents', 'Resources', 'runtime')
    : join(dirname(appPath), 'resources', 'runtime')
}

function bundledRuntimeGoSourcePath(appPath, target) {
  return target.platform === 'darwin'
    ? join(appPath, 'Contents', 'Resources', 'app.asar.unpacked', 'packages', 'runtime-go')
    : join(dirname(appPath), 'resources', 'app.asar.unpacked', 'packages', 'runtime-go')
}

function packagedAuthorityContext(appPath, target) {
  return {
    appOutDir: dirname(appPath),
    electronPlatformName: target.platform,
    arch: target.arch,
    packager: {
      appInfo: { productFilename: 'analytix' },
      config: { executableName: 'analytix' },
      platformSpecificBuildOptions: { executableName: 'analytix' }
    }
  }
}

function trustedCacheTempRoot() {
  const configured = String(process.env.TMPDIR || '').trim()
  if (!configured) {
    return { ok: false, blocked: true, blocker: 'analytix_cache_tmpdir_missing', path: '' }
  }
  if (!isAbsolute(configured)) {
    return { ok: false, blocker: 'analytix_cache_tmpdir_missing', path: '' }
  }
  try {
    const resolvedPath = realpathSync(configured)
    const resolvedMount = realpathSync(CACHE_MOUNT)
    accessSync(resolvedPath, constants.W_OK)
    if (!statSync(resolvedPath).isDirectory() || !pathContainedBy(resolvedMount, resolvedPath)) {
      throw new Error('untrusted_tmpdir')
    }
    return { ok: true, blocker: '', path: resolvedPath, pathHash: sha256(resolvedPath) }
  } catch {
    return {
      ok: false,
      blocker: 'analytix_cache_tmpdir_unavailable_unwritable_or_outside_trusted_mount',
      path: ''
    }
  }
}

function formalPackagedArtifactEvidence(appPath, target, expectedCommit) {
  const authorityPath = join(runtimeResourcePath(appPath, target), PACKAGED_BUILD_AUTHORITY_FILE)
  const authorityFile = hashRegularFile(authorityPath, { capture: true, maximumBytes: 256 * 1024 })
  const executable = hashRegularFile(executablePath(appPath, target))
  const runtimeServer = hashRegularFile(runtimeServerPath(appPath, target))
  const appAsar = hashRegularFile(appAsarPath(appPath, target))
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    targetKey: target.key,
    appPathHash: sha256(appPath),
    sourceCommit: '',
    authoritySha256: authorityFile.sha256,
    authorityDigest: '',
    executableSha256: executable.sha256,
    runtimeServerSha256: runtimeServer.sha256,
    appAsarSha256: appAsar.sha256,
    worktreeSnapshotDigest: '',
    codeSignatureVerified: false,
    fundsPluginArtifactBound: false
  }
  if (target.platform !== 'darwin') {
    return { ...base, blocked: true, blocker: 'native_file_chooser_acceptance_requires_macos' }
  }
  if (!existsSync(appPath)) return { ...base, blocked: true, blocker: 'formal_packaged_app_missing' }
  if (!executable.regular || !runtimeServer.regular || !appAsar.regular) {
    return { ...base, blocker: 'formal_packaged_artifact_incomplete' }
  }
  if (existsSync(bundledRuntimeGoSourcePath(appPath, target))) {
    return { ...base, blocker: 'packaged_runtime_go_source_present' }
  }
  if (!/^[0-9a-f]{40}$/u.test(expectedCommit)) {
    return { ...base, blocked: true, blocker: 'expected_packaged_source_commit_missing_or_invalid' }
  }
  if (!authorityFile.regular || !authorityFile.content) {
    return { ...base, blocked: true, blocker: 'packaged_build_authority_missing' }
  }
  let authority
  try {
    authority = JSON.parse(authorityFile.content.toString('utf8'))
  } catch {
    return { ...base, blocker: 'packaged_build_authority_json_invalid' }
  }
  const canonical = authorityFile.content.toString('utf8') === JSON.stringify(authority)
  const shapeValid = authority?.contract === PACKAGED_BUILD_AUTHORITY_CONTRACT &&
    packagedAuthorityContract.isPackagedBuildAuthorityV2(authority)
  let currentSnapshot = null
  let artifacts = null
  try {
    currentSnapshot = packagedAuthorityContract.collectPackagedWorktreeSnapshotV1(process.cwd())
    artifacts = packagedAuthorityContract.verifyPackagedBuildAuthorityArtifacts(
      packagedAuthorityContext(appPath, target),
      authority
    )
  } catch {
    currentSnapshot = null
    artifacts = null
  }
  const sourceCommit = String(authority?.sourceCommit || '').trim().toLowerCase()
  const worktreeSnapshotDigest = String(authority?.worktreeSnapshot?.snapshotDigest || '')
  const worktreeBound = currentSnapshot &&
    packagedAuthorityContract.isPackagedWorktreeSnapshotV1(currentSnapshot) &&
    currentSnapshot.snapshotDigest === worktreeSnapshotDigest
  const codeSign = spawnSync('/usr/bin/codesign', [
    '--verify', '--deep', '--strict', '--verbose=2', appPath
  ], { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' })
  const codeSignatureVerified = codeSign.status === 0
  const fundsPluginArtifactBound = Boolean(
    artifacts?.fundsPlugin && authority?.artifacts?.fundsPlugin &&
    canonicalJSON(artifacts.fundsPlugin) === canonicalJSON(authority.artifacts.fundsPlugin)
  )
  const ok = canonical && shapeValid && sourceCommit === expectedCommit &&
    currentGitCommit() === expectedCommit && worktreeBound && artifacts &&
    codeSignatureVerified && fundsPluginArtifactBound
  return {
    ...base,
    ok: Boolean(ok),
    blocked: !currentSnapshot,
    blocker: ok
      ? ''
      : !currentSnapshot
        ? 'current_worktree_snapshot_unavailable'
        : !shapeValid || !canonical
          ? 'packaged_build_authority_invalid'
          : sourceCommit !== expectedCommit || currentGitCommit() !== expectedCommit
            ? 'packaged_build_authority_source_commit_mismatch'
            : !worktreeBound
              ? 'packaged_build_authority_worktree_snapshot_mismatch'
              : !artifacts
                ? 'packaged_build_authority_artifact_binding_mismatch'
                : !codeSignatureVerified
                  ? 'formal_package_code_signature_invalid'
                  : 'packaged_funds_plugin_artifact_binding_mismatch',
    sourceCommit,
    authorityDigest: String(authority?.authorityDigest || ''),
    worktreeSnapshotDigest,
    codeSignatureVerified,
    fundsPluginArtifactBound
  }
}

function diagnosticUnpackagedSourceEvidence(expectedCommit) {
  if (!diagnosticUnpackaged) {
    return { ok: false, blocker: 'unpackaged_diagnostic_not_requested' }
  }
  const runtimeServer = runtimeServerPath(process.cwd(), packagedTarget())
  const runtime = hashRegularFile(runtimeServer)
  const main = hashRegularFile(resolve(process.cwd(), 'out/main/index.js'))
  const renderer = hashRegularFile(resolve(process.cwd(), 'out/renderer/index.html'))
  const preload = hashRegularFile(resolve(process.cwd(), 'out/preload/index.cjs'))
  const currentCommit = currentGitCommit()
  const ok = /^[0-9a-f]{40}$/u.test(expectedCommit) &&
    currentCommit === expectedCommit && runtime.regular && main.regular &&
    renderer.regular && preload.regular
  return {
    ok,
    blocker: ok ? '' : 'unpackaged_diagnostic_current_source_build_invalid',
    sourceCommit: currentCommit,
    runtimeServerSha256: runtime.sha256,
    mainSha256: main.sha256,
    rendererSha256: renderer.sha256,
    preloadSha256: preload.sha256
  }
}

function configuredExternalCaseAcceptance(startedAt) {
  const workspace = optionValue(
    '--case-workspace', process.env.ANALYTIX_MILESTONE_B_CASE_WORKSPACE || ''
  ).trim()
  const ownerRoot = optionValue(
    '--case-owner-root', process.env.ANALYTIX_MILESTONE_B_CASE_OWNER_ROOT || ''
  ).trim()
  const contractPath = optionValue(
    '--case-contract', process.env.ANALYTIX_MILESTONE_B_CASE_CONTRACT || ''
  ).trim()
  const provenancePath = optionValue(
    '--case-provenance', process.env.ANALYTIX_MILESTONE_B_CASE_PROVENANCE || ''
  ).trim()
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    workspacePathHash: workspace ? sha256(workspace) : '',
    ownerRootPathHash: ownerRoot ? sha256(ownerRoot) : '',
    contractPathHash: contractPath ? sha256(contractPath) : '',
    provenancePathHash: provenancePath ? sha256(provenancePath) : '',
    contractSha256: '',
    provenanceSha256: '',
    snapshotCount: 0,
    authority: null
  }
  if (!workspace || !ownerRoot || !contractPath || !provenancePath) {
    return {
      ...base,
      blocked: true,
      blocker: 'external_case_workspace_contract_and_provenance_required'
    }
  }
  try {
    const authority = loadMilestoneBExternalCaseAcceptance({
      workspace,
      ownerRoot,
      contractPath,
      provenancePath,
      repositoryRoot: process.cwd(),
      startedAt,
      diagnosticSynthetic: diagnosticUnpackaged
    })
    return {
      ...base,
      ok: true,
      workspacePathHash: sha256(authority.workspace),
      ownerRootPathHash: sha256(authority.ownerRoot),
      contractSha256: authority.contractSha256,
      provenanceSha256: authority.provenanceSha256,
      snapshotCount: authority.snapshots.length,
      authority
    }
  } catch (error) {
    return {
      ...base,
      blocker: error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
        ? error.message
        : 'external_case_acceptance_unavailable'
    }
  }
}

function privateEd25519PublicKey(path, expectedSha256) {
  const file = ownerPrivateRegularFile(path, 64 * 1024)
  if (!file.regular || !file.content || file.sha256 !== expectedSha256) {
    throw new Error('trusted_ed25519_public_key_invalid')
  }
  const key = createPublicKey(file.content)
  if (key.asymmetricKeyType !== 'ed25519') throw new Error('trusted_ed25519_public_key_invalid')
  const keyId = sha256(key.export({ format: 'der', type: 'spki' }))
  return { key, keyId, sha256: file.sha256 }
}

function configuredProviderAuditAuthority(startedAt, caseEvidence, productEvidence, sourceCommit) {
  const ownerRoot = optionValue(
    '--provider-audit-owner-root', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_OWNER_ROOT || ''
  ).trim()
  const receiptPath = optionValue(
    '--provider-audit-receipt', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_RECEIPT || ''
  ).trim()
  const challengePath = optionValue(
    '--provider-audit-challenge', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_CHALLENGE || ''
  ).trim()
  const publicKeyPath = optionValue(
    '--provider-audit-public-key', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PUBLIC_KEY || ''
  ).trim()
  const privateKeyPath = optionValue(
    '--provider-audit-private-key', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_PRIVATE_KEY || ''
  ).trim()
  const socketPath = optionValue(
    '--provider-audit-socket', process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SOCKET || ''
  ).trim()
  const trustedKeySha256 = String(
    process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_TRUSTED_PUBLIC_KEY_SHA256 || ''
  ).trim().toLowerCase()
  const scannerBuildSha256 = String(
    process.env.ANALYTIX_MILESTONE_B_PROVIDER_AUDIT_SCANNER_BUILD_SHA256 || ''
  ).trim().toLowerCase()
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    ownerRootPathHash: ownerRoot ? sha256(ownerRoot) : '',
    receiptPathHash: receiptPath ? sha256(receiptPath) : '',
    challengePathHash: challengePath ? sha256(challengePath) : '',
    publicKeyPathHash: publicKeyPath ? sha256(publicKeyPath) : '',
    privateKeyPathHash: privateKeyPath ? sha256(privateKeyPath) : '',
    socketPathHash: socketPath ? sha256(socketPath) : '',
    publicKeySha256: '',
    keyId: '',
    challengeDigest: '',
    authority: null
  }
  if (!caseEvidence?.ok || !productEvidence?.ok || !ownerRoot || !receiptPath ||
      !challengePath || !publicKeyPath ||
      !privateKeyPath || !socketPath ||
      !isSHA256(trustedKeySha256) || !isSHA256(scannerBuildSha256) ||
      !/^[0-9a-f]{40}$/u.test(sourceCommit)) {
    return {
      ...base,
      blocked: true,
      blocker: 'provider_audit_owner_root_receipt_challenge_keypair_socket_trust_pin_and_scanner_build_required'
    }
  }
  try {
    const root = realpathSync(ownerRoot)
    if (!ownerOnlyDirectory(root)) throw new Error('provider_audit_owner_root_invalid')
    const caseOwnerRoot = realpathSync(caseEvidence.authority.ownerRoot)
    const caseWorkspace = realpathSync(caseEvidence.authority.workspace)
    const productOwnerRoot = realpathSync(productEvidence.authority.ownerRoot)
    const repositoryRoot = realpathSync(process.cwd())
    const cacheRoot = realpathSync(CACHE_MOUNT)
    const overlaps = (left, right) =>
      pathContainedBy(left, right) || pathContainedBy(right, left)
    if ([caseOwnerRoot, caseWorkspace, productOwnerRoot, repositoryRoot, cacheRoot]
      .some((protectedRoot) => overlaps(root, protectedRoot))) {
      throw new Error('provider_audit_owner_root_not_independent')
    }
    const providerAuditVolume = managedProductVolumeInfo(root)
    if (!providerAuditVolume.ok) throw new Error(providerAuditVolume.blocker)
    const receiptParent = realpathSync(dirname(receiptPath))
    const challengeParent = realpathSync(dirname(challengePath))
    const socketParent = realpathSync(dirname(socketPath))
    const keyPath = realpathSync(publicKeyPath)
    const signingKeyPath = realpathSync(privateKeyPath)
    const signingKeyState = ownerPrivateRegularFileMetadata(signingKeyPath, 64 * 1024)
    const publicKeyState = ownerPrivateRegularFileMetadata(keyPath, 64 * 1024)
    const scannerFile = hashRegularFile(PROVIDER_AUDIT_SCANNER_PATH, { maximumBytes: 1024 * 1024 })
    const receiptTemporaryPath = `${receiptPath}.next`
    const authorityPaths = [
      receiptPath, receiptTemporaryPath, challengePath, publicKeyPath, privateKeyPath, socketPath
    ]
    if (!ownerOnlyDirectory(receiptParent) || !ownerOnlyDirectory(challengeParent) ||
        !ownerOnlyDirectory(socketParent) || !signingKeyState || !publicKeyState ||
        !authorityPaths.every((path) =>
          isAbsolute(path) && normalize(path) === path && resolve(path) === path && !path.includes('\0')
        ) ||
        new Set(authorityPaths).size !== authorityPaths.length ||
        !pathContainedBy(root, receiptParent) || !pathContainedBy(root, challengeParent) ||
        !pathContainedBy(root, socketParent) || !pathContainedBy(root, keyPath) ||
        !pathContainedBy(root, signingKeyPath) || keyPath === signingKeyPath ||
        keyPath !== publicKeyPath || signingKeyPath !== privateKeyPath ||
        !pathContainedBy(root, receiptTemporaryPath) ||
        existsSync(receiptPath) || existsSync(receiptTemporaryPath) ||
        existsSync(challengePath) || existsSync(socketPath) ||
        !scannerFile.regular || scannerFile.sha256 !== scannerBuildSha256 ||
        signingKeyState.mtimeMs >= startedAt.getTime() ||
        publicKeyState.mtimeMs >= startedAt.getTime()) {
      throw new Error('provider_audit_paths_not_fresh_owner_isolated')
    }
    const trusted = privateEd25519PublicKey(keyPath, trustedKeySha256)
    const protectedValues = [...new Set(caseEvidence.authority?.protectedValues || [])]
      .filter((value) => typeof value === 'string' && value.length > 0)
      .sort()
    if (protectedValues.length === 0) throw new Error('provider_audit_protected_value_set_empty')
    const forbiddenValues = [...new Set([
      caseEvidence.authority.workspace,
      caseEvidence.authority.ownerRoot,
      ...caseEvidence.authority.snapshots.map((snapshot) => snapshot.sourcePath)
    ])].sort()
    if (forbiddenValues.length < 3) throw new Error('provider_audit_forbidden_value_set_incomplete')
    const runNonceDigest = sha256(randomBytes(32))
    const challenge = {
      contract: 'analytix.milestone-b.provider-request-audit-challenge.v1',
      caseContractSha256: caseEvidence.contractSha256,
      caseProvenanceSha256: caseEvidence.provenanceSha256,
      protectedValueSetDigest: sha256(canonicalJSON(protectedValues)),
      protectedValueCount: protectedValues.length,
      forbiddenValueSetDigest: sha256(canonicalJSON(forbiddenValues)),
      forbiddenValueCount: forbiddenValues.length,
      scannerBuildSha256,
      scannerPolicySha256: sha256(canonicalJSON(PROVIDER_AUDIT_SCANNER_POLICY)),
      runNonceDigest,
      sourceCommit,
      issuedAt: startedAt.toISOString(),
      expiresAt: new Date(startedAt.getTime() + DEFAULT_TIMEOUT_MS + 5 * 60 * 1000).toISOString()
    }
    return {
      ...base,
      ok: true,
      publicKeySha256: trusted.sha256,
      keyId: trusted.keyId,
      challengeDigest: sha256(canonicalJSON(challenge)),
      authority: Object.freeze({
        root,
        receiptPath,
        challengePath,
        privateKeyPath: signingKeyPath,
        publicKeyPath: keyPath,
        socketPath,
        key: trusted.key,
        keyId: trusted.keyId,
        challenge: Object.freeze(challenge)
      })
    }
  } catch (error) {
    return {
      ...base,
      blocker: error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
        ? error.message
        : 'provider_audit_authority_unavailable'
    }
  }
}

function publishProviderAuditChallenge(authority) {
  writeFileSync(authority.challengePath, JSON.stringify(authority.challenge), {
    encoding: 'utf8',
    mode: 0o600,
    flag: 'wx'
  })
  chmodSync(authority.challengePath, 0o600)
  const file = ownerPrivateRegularFile(authority.challengePath, 64 * 1024)
  if (!file.regular || file.sha256 !== sha256(JSON.stringify(authority.challenge))) {
    throw new Error('provider_audit_challenge_publication_failed')
  }
}

function verifyProviderAuditReceipt(
  authority,
  startedAt,
  finishedAt,
  exactProviderRequestCount,
  minimumProviderSafeSemanticBlockCount,
  expectedProviderFamily
) {
  const document = parseJSONFile(authority.receiptPath, 256 * 1024)
  const receipt = document.value
  if (!exactKeys(receipt, [
    'contract', 'caseContractSha256', 'caseProvenanceSha256',
    'runNonceDigest', 'sourceCommit', 'providerFamily',
    'protectedValueSetDigest', 'protectedValueCount',
    'forbiddenValueSetDigest', 'forbiddenValueCount', 'scannerBuildSha256',
    'scannerPolicySha256',
    'captureScope', 'providerRequestCount', 'scannedRequestBodyCount',
    'completeIdentifierFindingCount', 'forbiddenHostMaterialFindingCount',
    'providerSafeSemanticBlockCount', 'providerSafeSemanticValidationCount',
    'generatedAt', 'payloadSetDigest', 'keyId', 'signature'
  ]) || receipt.contract !== PROVIDER_AUDIT_CONTRACT ||
      receipt.caseContractSha256 !== authority.challenge.caseContractSha256 ||
      receipt.caseProvenanceSha256 !== authority.challenge.caseProvenanceSha256 ||
      receipt.protectedValueSetDigest !== authority.challenge.protectedValueSetDigest ||
      receipt.protectedValueCount !== authority.challenge.protectedValueCount ||
      receipt.forbiddenValueSetDigest !== authority.challenge.forbiddenValueSetDigest ||
      receipt.forbiddenValueCount !== authority.challenge.forbiddenValueCount ||
      receipt.scannerBuildSha256 !== authority.challenge.scannerBuildSha256 ||
      receipt.scannerPolicySha256 !== authority.challenge.scannerPolicySha256 ||
      receipt.runNonceDigest !== authority.challenge.runNonceDigest ||
      receipt.sourceCommit !== authority.challenge.sourceCommit ||
      !expectedProviderFamily || receipt.providerFamily !== expectedProviderFamily ||
      receipt.captureScope !== 'outbound-provider-request-bodies-before-network-send' ||
      !Number.isSafeInteger(receipt.providerRequestCount) || receipt.providerRequestCount <= 0 ||
      receipt.providerRequestCount !== exactProviderRequestCount ||
      receipt.scannedRequestBodyCount !== receipt.providerRequestCount ||
      receipt.completeIdentifierFindingCount !== 0 ||
      receipt.forbiddenHostMaterialFindingCount !== 0 ||
      !Number.isSafeInteger(receipt.providerSafeSemanticBlockCount) ||
      receipt.providerSafeSemanticBlockCount < minimumProviderSafeSemanticBlockCount ||
      receipt.providerSafeSemanticValidationCount !== receipt.providerSafeSemanticBlockCount ||
      !isSHA256(receipt.payloadSetDigest) ||
      receipt.keyId !== authority.keyId || Number.isNaN(Date.parse(receipt.generatedAt)) ||
      Date.parse(receipt.generatedAt) < startedAt.getTime() ||
      Date.parse(receipt.generatedAt) > finishedAt.getTime() + 60_000 ||
      !/^[A-Za-z0-9_-]{80,100}$/u.test(String(receipt.signature || ''))) {
    throw new Error('provider_audit_receipt_invalid')
  }
  const unsigned = { ...receipt }
  delete unsigned.signature
  if (!verifySignature(
    null,
    Buffer.from(canonicalJSON(unsigned)),
    authority.key,
    Buffer.from(receipt.signature, 'base64url')
  )) {
    throw new Error('provider_audit_receipt_signature_invalid')
  }
  return {
    ok: true,
    receiptSha256: document.file.sha256,
    providerRequestCount: receipt.providerRequestCount,
    scannedRequestBodyCount: receipt.scannedRequestBodyCount,
    completeIdentifierFindingCount: 0,
    forbiddenHostMaterialFindingCount: 0,
    providerSafeSemanticBlockCount: receipt.providerSafeSemanticBlockCount,
    providerSafeSemanticValidationCount: receipt.providerSafeSemanticValidationCount,
    payloadSetDigest: receipt.payloadSetDigest,
    generatedAt: receipt.generatedAt,
    keyId: receipt.keyId
  }
}

function writeIsolatedSettings({ userDataDir, runtimeDataDir, workspace, runtimePort }) {
  mkdirSync(userDataDir, { recursive: true, mode: 0o700 })
  mkdirSync(runtimeDataDir, { recursive: true, mode: 0o700 })
  const writeWorkspace = join(userDataDir, 'ordinary-write-workspace')
  const scheduleWorkspace = join(userDataDir, 'ordinary-schedule-workspace')
  const clawWorkspace = join(userDataDir, 'ordinary-claw-workspace')
  for (const path of [writeWorkspace, scheduleWorkspace, clawWorkspace]) {
    mkdirSync(path, { recursive: true, mode: 0o700 })
  }
  const settingsPath = join(userDataDir, 'analytix-settings.json')
  const settings = {
    version: 1,
    locale: 'en',
    theme: 'light',
    workspaceRoot: workspace,
    provider: {
      activeProviderId: '',
      apiKey: '',
      baseUrl: '',
      providers: []
    },
    runtime: {
      port: runtimePort,
      autoStart: true,
      dataDir: runtimeDataDir,
      providerId: '',
      model: '',
      endpointFormat: 'chat_completions',
      executionPolicyVersion: 2,
      approvalPolicy: 'auto',
      sandboxMode: 'danger-full-access',
      subagents: {
        enabled: true,
        maxParallel: 2,
        maxChildRuns: 2,
        defaultToolPolicy: 'readOnly',
        defaultProfile: MILESTONE_B_READ_ONLY_SUBAGENT_PROFILE,
        profiles: {
          [MILESTONE_B_READ_ONLY_SUBAGENT_PROFILE]: {
            prompt: 'Inspect only the named ordinary test expectation. Do not access case data or mutate state.',
            toolPolicy: 'readOnly',
            tools: ['read']
          }
        }
      }
    },
    write: {
      defaultWorkspaceRoot: writeWorkspace,
      activeWorkspaceRoot: writeWorkspace,
      workspaces: [writeWorkspace]
    },
    schedule: { defaultWorkspaceRoot: scheduleWorkspace, tasks: [] },
    claw: {
      enabled: false,
      im: { enabled: false, workspaceRoot: clawWorkspace },
      channels: [],
      tasks: []
    }
  }
  writeFileSync(settingsPath, JSON.stringify(settings), { encoding: 'utf8', mode: 0o600 })
  chmodSync(settingsPath, 0o600)
  return settingsPath
}

function safeChildEnvironment({ isolatedHome, userDataDir, providerAuditSocketPath }) {
  const allowed = new Set([
    'PATH', 'SHELL', 'LANG', 'LC_ALL', 'LC_CTYPE', 'SystemRoot',
    'TMPDIR', 'TEMP', 'TMP', 'ANALYTIX_DEV_CACHE_ROOT', 'npm_config_cache',
    'XDG_CACHE_HOME', 'PIP_CACHE_DIR', 'UV_CACHE_DIR', 'PYTHONPYCACHEPREFIX',
    'MYPY_CACHE_DIR', 'RUFF_CACHE_DIR', 'GOCACHE', 'GOMODCACHE', 'GOTMPDIR',
    'CARGO_HOME', 'CARGO_TARGET_DIR', 'CCACHE_DIR', 'SCCACHE_DIR', 'XWIN_CACHE_DIR',
    'COREPACK_HOME', 'ELECTRON_CACHE', 'ELECTRON_BUILDER_CACHE',
    'PLAYWRIGHT_BROWSERS_PATH', 'NODE_COMPILE_CACHE'
  ])
  const childEnv = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (allowed.has(key) && value !== undefined) childEnv[key] = value
  }
  const environment = {
    ...childEnv,
    HOME: isolatedHome,
    USERPROFILE: isolatedHome,
    ANALYTIX_USER_DATA_DIR: userDataDir,
    ANALYTIX_RUNTIME_BACKEND: 'go-runtime-default',
    ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_B: '1',
    ANALYTIX_PROVIDER_AUDIT_SOCKET_PATH: providerAuditSocketPath
  }
  if (diagnosticUnpackaged) {
    const runtimeServer = runtimeServerPath(process.cwd(), packagedTarget())
    const runtimeEvidence = hashRegularFile(runtimeServer)
    if (!runtimeEvidence.regular) {
      throw new Error('diagnostic_runtime_server_missing')
    }
    environment.NODE_ENV = 'development'
    environment.ANALYTIX_RUNTIME_GO_UNPACKAGED_MILESTONE_B_DIAGNOSTIC = '1'
    environment.ANALYTIX_GO_RUNTIME_SERVER_BIN = runtimeServer
    environment.ELECTRON_RENDERER_URL = expectedPackagedRendererURL()
  }
  return environment
}

async function launchProviderAuditScanner(authority, externalCase) {
  const child = spawn(process.execPath, [
    PROVIDER_AUDIT_SCANNER_PATH,
    '--private-startup-frame-v1'
  ], {
    cwd: process.cwd(),
    env: safeScannerEnvironment(),
    stdio: ['pipe', 'pipe', 'ignore']
  })
  child.analytixExit = new Promise((resolvePromise) => {
    child.once('exit', (code, signal) => resolvePromise({ code, signal }))
  })
  child.analytixLaunchIdentity = null
  const document = {
    schemaVersion: 1,
    purpose: 'analytix.provider-request-audit-scanner-startup/v1',
    auditOwnerRoot: authority.root,
    socketPath: authority.socketPath,
    challengePath: authority.challengePath,
    privateKeyPath: authority.privateKeyPath,
    publicKeyPath: authority.publicKeyPath,
    receiptPath: authority.receiptPath,
    repositoryRoot: process.cwd(),
    caseOwnerRoot: externalCase.ownerRoot,
    caseWorkspace: externalCase.workspace,
    caseContractPath: externalCase.contractPath,
    caseProvenancePath: externalCase.provenancePath,
    diagnosticSynthetic: diagnosticUnpackaged
  }
  let body = Buffer.from(JSON.stringify(document), 'utf8')
  const frame = Buffer.alloc(8 + body.length)
  frame.writeBigUInt64BE(BigInt(body.length), 0)
  body.copy(frame, 8)
  body.fill(0)
  body = null
  try {
    try {
      await new Promise((resolveWrite, rejectWrite) => {
        child.stdin.once('error', rejectWrite)
        child.stdin.end(frame, resolveWrite)
      })
    } finally {
      frame.fill(0)
    }
    await waitForProviderAuditScannerReady(child)
    if (!child.analytixLaunchIdentity) await establishLaunchIdentity(child)
    if (!child.analytixLaunchIdentity || !processIdentityMatches(child.analytixLaunchIdentity)) {
      throw new Error('provider_audit_scanner_process_identity_unavailable')
    }
    return child
  } catch (error) {
    const identity = child.analytixLaunchIdentity || await establishLaunchIdentity(child, 500)
    if (identity && processIdentityMatches(identity)) {
      child.analytixLaunchIdentity = identity
      await stopExactTaskOwnedChild(child)
    }
    throw error
  }
}

function safeScannerEnvironment() {
  const environment = {}
  for (const name of ['PATH', 'LANG', 'LC_ALL', 'LC_CTYPE', 'TMPDIR', 'NODE_COMPILE_CACHE']) {
    if (process.env[name] !== undefined) environment[name] = process.env[name]
  }
  return environment
}

function waitForProviderAuditScannerReady(child, timeoutMs = 20_000) {
  return new Promise((resolveReady, rejectReady) => {
    let output = Buffer.alloc(0)
    let settled = false
    const finish = (error) => {
      if (settled) return
      settled = true
      clearTimeout(timeout)
      child.stdout?.off('data', onData)
      child.off('exit', onExit)
      output.fill(0)
      if (error) rejectReady(error)
      else resolveReady()
    }
    const onData = (chunk) => {
      const combined = Buffer.concat([output, Buffer.from(chunk)])
      output.fill(0)
      output = combined
      if (output.length > 256) return finish(new Error('provider_audit_scanner_ready_invalid'))
      if (output.equals(Buffer.from('ANALYTIX_PROVIDER_AUDIT_SCANNER_READY_V1\n'))) finish()
    }
    const onExit = () => finish(new Error('provider_audit_scanner_exited_before_ready'))
    const timeout = setTimeout(() => finish(new Error('provider_audit_scanner_ready_timeout')), timeoutMs)
    child.stdout?.on('data', onData)
    child.once('exit', onExit)
  })
}

async function getFreePort() {
  const server = net.createServer()
  await new Promise((resolvePromise) => server.listen(0, '127.0.0.1', resolvePromise))
  const address = server.address()
  await new Promise((resolvePromise) => server.close(resolvePromise))
  if (!address || typeof address === 'string') throw new Error('local_port_allocation_failed')
  return address.port
}

function sleep(ms) {
  return new Promise((resolvePromise) => setTimeout(resolvePromise, ms))
}

async function debugTargets(port, timeoutMs) {
  const response = await fetch(`http://127.0.0.1:${port}/json/list`, {
    signal: AbortSignal.timeout(timeoutMs)
  })
  if (!response.ok) throw new Error('packaged_renderer_debug_target_unavailable')
  const targets = await response.json()
  return Array.isArray(targets)
    ? targets.filter((item) => item?.type === 'page' && item?.webSocketDebuggerUrl)
    : []
}

async function waitForDebugTarget(port, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let pageTargetCount = 0
  while (Date.now() < deadline) {
    try {
      const targets = await debugTargets(port, Math.min(1000, Math.max(250, deadline - Date.now())))
      pageTargetCount = targets.length
      if (targets.length === 1) return { ...targets[0], pageTargetCount: targets.length }
    } catch {
      // The packaged renderer is still starting or reloading.
    }
    await sleep(250)
  }
  if (pageTargetCount > 1) throw new Error('packaged_renderer_target_count_not_one')
  throw new Error('packaged_renderer_debug_target_not_ready')
}

function readonlyTypedLocalDisplayExpression(position = 'last') {
  return `(() => {
    const roots = Array.from(document.querySelectorAll('[data-analytix-local-display="accepted_slot_display"]'));
    const root = ${JSON.stringify(position)} === 'first' ? roots[0] : roots.at(-1);
    const mode = root?.getAttribute('data-analytix-local-display-mode') || '';
    const values = root
      ? Array.from(root.querySelectorAll('dd')).map((node) => String(node.textContent || ''))
      : [];
    const button = root?.querySelector('button');
    const rect = button?.getBoundingClientRect();
    return {
      kind: root?.getAttribute('data-analytix-local-display') || '',
      mode,
      values,
      slotCount: root?.querySelectorAll('dd').length || 0,
      toggleGeometry: rect && rect.width > 0 && rect.height > 0 ? {
        x: rect.x + rect.width / 2,
        y: rect.y + rect.height / 2
      } : null
    };
  })()`
}

function readonlyDirectSourcePreviewExpression() {
  return `(() => {
    const root = document.querySelector('[data-analytix-local-display="direct_source_preview"]');
    const rows = root
      ? Array.from(root.querySelectorAll('[data-analytix-source-row]'))
      : [];
    const control = (selector) => {
      const button = root?.querySelector(selector);
      const rect = button?.getBoundingClientRect();
      return {
        disabled: button instanceof HTMLButtonElement ? button.disabled : true,
        geometry: rect && rect.width > 0 && rect.height > 0 ? {
          x: rect.x + rect.width / 2,
          y: rect.y + rect.height / 2
        } : null
      };
    };
    return {
      kind: root?.getAttribute('data-analytix-local-display') || '',
      mode: root?.getAttribute('data-analytix-local-display-mode') || '',
      values: root
        ? Array.from(root.querySelectorAll('td')).map((node) => String(node.textContent || ''))
        : [],
      rowIndices: rows.map((row) => Number(row.getAttribute('data-analytix-source-row'))),
      rowCount: rows.length,
      previous: control('[data-analytix-source-page="previous"]'),
      next: control('[data-analytix-source-page="next"]'),
      toggle: control('button:not([data-analytix-source-page])')
    };
  })()`
}

export function directSourcePreviewEvidence(
  raw,
  protectedValues,
  expectedCompleteAccount,
  expectedSourceRowCount
) {
  const first = raw?.first
  const next = raw?.next
  const masked = raw?.masked
  const firstValues = Array.isArray(first?.values) ? first.values : []
  const nextValues = Array.isArray(next?.values) ? next.values : []
  const maskedValues = Array.isArray(masked?.values) ? masked.values : []
  const firstIndices = Array.isArray(first?.rowIndices) ? first.rowIndices : []
  const nextIndices = Array.isArray(next?.rowIndices) ? next.rowIndices : []
  const fullModeExactValue = first?.kind === 'direct_source_preview' &&
    first?.mode === 'full' && firstValues.includes(expectedCompleteAccount)
  const maskedModeLocalOnly = masked?.kind === 'direct_source_preview' &&
    masked?.mode === 'masked' && !maskedValues.includes(expectedCompleteAccount) &&
    scanStringForProtectedValues(maskedValues, protectedValues) === 0
  const expectedFirstPageRows = Math.min(25, Number(expectedSourceRowCount || 0))
  const expectedSecondPageRows = Math.min(25, Math.max(0, Number(expectedSourceRowCount || 0) - 25))
  const firstPage = expectedFirstPageRows === 25 && firstIndices.length === expectedFirstPageRows &&
    firstIndices.every((value, index) =>
    Number.isSafeInteger(value) && value === index) && first?.previous?.disabled === true &&
    first?.next?.disabled === false
  const secondPage = expectedSecondPageRows > 0 && nextIndices.length === expectedSecondPageRows &&
    nextIndices.every((value, index) => Number.isSafeInteger(value) && value === 25 + index) &&
    next?.previous?.disabled === false &&
    next?.next?.disabled === (Number(expectedSourceRowCount || 0) <= 50) &&
    nextValues.length > 0
  const providerRequestAbsent = raw?.providerRequestAbsent === true
  const agentTurnAbsent = raw?.agentTurnAbsent === true
  const mcpCallAbsent = raw?.mcpCallAbsent === true
  const claimReceiptFinalGateAbsent = raw?.claimReceiptFinalGateAbsent === true
  const ok = fullModeExactValue && maskedModeLocalOnly && firstPage && secondPage &&
    providerRequestAbsent && agentTurnAbsent && mcpCallAbsent && claimReceiptFinalGateAbsent
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'direct_source_preview_public_seam_invalid',
    fullModeExactValue,
    maskedModeLocalOnly,
    paginationObserved: firstPage && secondPage,
    providerRequestAbsent,
    agentTurnAbsent,
    mcpCallAbsent,
    claimReceiptFinalGateAbsent
  })
}

export function typedLocalDisplayEvidence(raw, protectedValues, expectedCompleteAccount) {
  const full = raw?.full
  const masked = raw?.masked
  const fullValues = Array.isArray(full?.values) ? full.values : []
  const maskedValues = Array.isArray(masked?.values) ? masked.values : []
  const forbiddenSurface = raw?.forbiddenGenericProviderChannels || {}
  const forbiddenScan = scanStringForProtectedValues(forbiddenSurface, protectedValues)
  const forbiddenInternalReferenceCount = JSON.stringify(forbiddenSurface)
    .match(/\bcer1_[a-z0-9_-]+\b/giu)?.length || 0
  const fullModeExactValue = full?.kind === 'accepted_slot_display' &&
    full?.mode === 'full' && fullValues.includes(expectedCompleteAccount)
  const maskedModeLocalOnly = masked?.kind === 'accepted_slot_display' &&
    masked?.mode === 'masked' && !maskedValues.includes(expectedCompleteAccount)
  const exactValueOnlyInTypedSink = fullModeExactValue && forbiddenScan === 0 &&
    forbiddenInternalReferenceCount === 0
  const acceptedSlotBindingVerified = raw?.acceptedFinalDigestSha256 === true &&
    raw?.claimReceiptRefsOnly === true
  const invalidatedAfterAuthorityChange = raw?.invalidatedAfterAuthorityChange === true
  const ok = fullModeExactValue && maskedModeLocalOnly && exactValueOnlyInTypedSink &&
    acceptedSlotBindingVerified
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'typed_local_display_public_seam_invalid',
    fullModeExactValue,
    maskedModeLocalOnly,
    exactValueOnlyInTypedSink,
    acceptedSlotBindingVerified,
    forbiddenGenericProviderChannelsZero: forbiddenScan === 0 &&
      forbiddenInternalReferenceCount === 0,
    invalidatedAfterAuthorityChange,
    forbiddenCompletePIIFindingCount: forbiddenScan,
    forbiddenInternalReferenceFindingCount: forbiddenInternalReferenceCount
  })
}

async function observeTypedLocalDisplay({ debugPort, position = 'last', timeoutMs = 20_000 }) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  if (typeof target?.id !== 'string' || !target.id || target.pageTargetCount !== 1 ||
      typeof target.webSocketDebuggerUrl !== 'string' || !target.webSocketDebuggerUrl ||
      target.url !== expectedPackagedRendererURL()) {
    throw new Error('typed_local_display_renderer_target_invalid')
  }
  const expression = readonlyTypedLocalDisplayExpression(position)
  const observation = await evaluateReadonlyCdp(target.webSocketDebuggerUrl, expression, timeoutMs)
  if (!observation || typeof observation !== 'object' || Array.isArray(observation)) {
    throw new Error('typed_local_display_observation_invalid')
  }
  return Object.freeze({ target, observation })
}

async function waitForTypedLocalDisplay({
  debugPort,
  mode,
  expectedValue = '',
  position = 'last',
  timeoutMs = 20_000
}) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const observed = await observeTypedLocalDisplay({
        debugPort,
        position,
        timeoutMs: Math.min(5000, Math.max(250, deadline - Date.now()))
      })
      if (observed.observation?.kind === 'accepted_slot_display' &&
          observed.observation?.mode === mode &&
          observed.observation?.slotCount > 0 &&
          (!expectedValue || observed.observation?.values?.includes(expectedValue))) {
        return observed
      }
    } catch {
      // Retained immutable material can be re-resolving after an authority change.
    }
    await sleep(150)
  }
  throw new Error('typed_local_display_state_not_observed')
}

async function observeDirectSourcePreview({ debugPort, timeoutMs = 20_000 }) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  if (typeof target?.id !== 'string' || !target.id || target.pageTargetCount !== 1 ||
      typeof target.webSocketDebuggerUrl !== 'string' || !target.webSocketDebuggerUrl ||
      target.url !== expectedPackagedRendererURL()) {
    throw new Error('direct_source_preview_renderer_target_invalid')
  }
  const observation = await evaluateReadonlyCdp(
    target.webSocketDebuggerUrl,
    readonlyDirectSourcePreviewExpression(),
    timeoutMs
  )
  if (!observation || typeof observation !== 'object' || Array.isArray(observation)) {
    throw new Error('direct_source_preview_observation_invalid')
  }
  return Object.freeze({ target, observation })
}

async function waitForDirectSourcePreview({
  debugPort,
  mode,
  minimumRows = 1,
  firstRowIndex = 0,
  timeoutMs = 20_000
}) {
  const deadline = Date.now() + timeoutMs
  let latest = null
  while (Date.now() < deadline) {
    try {
      latest = await observeDirectSourcePreview({
        debugPort,
        timeoutMs: Math.min(5000, Math.max(250, deadline - Date.now()))
      })
      if (latest.observation?.kind === 'direct_source_preview' &&
          latest.observation?.mode === mode &&
          Number(latest.observation?.rowCount || 0) >= minimumRows &&
          latest.observation?.rowIndices?.[0] === firstRowIndex) {
        return latest
      }
    } catch {
      // Main/Go may still be resolving the current immutable snapshot.
    }
    await sleep(150)
  }
  throw new Error('direct_source_preview_state_not_observed')
}

async function clickDirectSourcePreviewControl(debugPort, name, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const observed = await observeDirectSourcePreview({
        debugPort,
        timeoutMs: Math.min(5000, Math.max(250, deadline - Date.now()))
      })
      const control = observed.observation?.[name]
      if (control?.geometry && control.disabled === false) {
        await clickCdpGeometry(observed.target, control.geometry)
        return { ok: true, blocker: '' }
      }
    } catch {
      // The local projection can be refreshing between page or mode changes.
    }
    await sleep(150)
  }
  return { ok: false, blocker: 'direct_source_preview_control_not_ready' }
}

async function evaluateReadonlyCdp(wsUrl, expression, timeoutMs) {
  if (typeof WebSocket === 'undefined') throw new Error('node_websocket_unavailable')
  const socket = new WebSocket(wsUrl)
  await new Promise((resolvePromise, reject) => {
    socket.addEventListener('open', resolvePromise, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  try {
    return await new Promise((resolvePromise, reject) => {
      const timer = setTimeout(() => reject(new Error('readonly_cdp_timeout')), timeoutMs)
      socket.addEventListener('message', (event) => {
        const message = JSON.parse(String(event.data))
        if (message.id !== 1) return
        clearTimeout(timer)
        if (message.error || message.result?.exceptionDetails) {
          reject(new Error('readonly_cdp_evaluation_failed'))
          return
        }
        resolvePromise(message.result?.result?.value)
      })
      socket.send(JSON.stringify({
        id: 1,
        method: 'Runtime.evaluate',
        params: { expression, awaitPromise: true, returnByValue: true, timeout: timeoutMs }
      }))
    })
  } finally {
    socket.close()
  }
}

async function dispatchCdpCommands(wsUrl, commands, timeoutMs) {
  if (typeof WebSocket === 'undefined') throw new Error('node_websocket_unavailable')
  const socket = new WebSocket(wsUrl)
  await new Promise((resolvePromise, reject) => {
    socket.addEventListener('open', resolvePromise, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  try {
    const results = []
    for (let index = 0; index < commands.length; index += 1) {
      const id = index + 1
      const command = commands[index]
      results.push(await new Promise((resolvePromise, reject) => {
        const timer = setTimeout(() => reject(new Error('cdp_input_timeout')), timeoutMs)
        const onMessage = (event) => {
          const message = JSON.parse(String(event.data))
          if (message.id !== id) return
          socket.removeEventListener('message', onMessage)
          clearTimeout(timer)
          if (message.error) reject(new Error('cdp_input_dispatch_failed'))
          else resolvePromise(message.result || {})
        }
        socket.addEventListener('message', onMessage)
        socket.send(JSON.stringify({ id, method: command.method, params: command.params || {} }))
      }))
    }
    return results
  } finally {
    socket.close()
  }
}

function readonlyComposerGeometryExpression(acceptedLabels = []) {
  return `(() => {
    const rectValue = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    };
    const editors = Array.from(document.querySelectorAll('.ProseMirror[contenteditable="true"]'));
    const editor = editors.find((candidate) => rectValue(candidate)) || null;
    const composer = editor?.closest('.ds-composer-shell') || null;
    const button = composer?.querySelector('.ds-composer-primary-action-button') || null;
    const modelPickerButton = composer?.querySelector('.ds-composer-model-picker button') || null;
    const label = button ? String(button.getAttribute('aria-label') || button.title || '') : '';
    const accepted = ${JSON.stringify(acceptedLabels)};
    return {
      editor: rectValue(editor),
      editorTextLength: editor ? String(editor.innerText || editor.textContent || '').trim().length : 0,
      button: rectValue(button),
      buttonDisabled: !!button?.disabled,
      buttonLoading: !!button?.querySelector('.ds-composer-primary-action-icon.animate-spin'),
      buttonLabelAccepted: accepted.length === 0 || accepted.includes(label),
      modelPickerDisabled: !!modelPickerButton?.disabled
    };
  })()`
}

async function clickCdpGeometry(target, geometry, timeoutMs = 20_000) {
  if (!target?.webSocketDebuggerUrl || !geometry) throw new Error('cdp_click_geometry_missing')
  await dispatchCdpCommands(target.webSocketDebuggerUrl, [
    {
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mousePressed', x: geometry.x, y: geometry.y,
        button: 'left', clickCount: 1
      }
    },
    {
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mouseReleased', x: geometry.x, y: geometry.y,
        button: 'left', clickCount: 1
      }
    }
  ], timeoutMs)
}

async function cdpComposerSubmitToTarget(
  target,
  text,
  acceptedButtonLabels = [],
  resolveCurrentTarget = async () => target
) {
  try {
    if (!target?.webSocketDebuggerUrl) throw new Error('composer_target_unavailable')
    const idleDeadline = Date.now() + 10_000
    let idleTarget = target
    let idle = null
    while (Date.now() < idleDeadline) {
      idleTarget = await resolveCurrentTarget()
      if (!idleTarget?.webSocketDebuggerUrl) throw new Error('composer_target_unavailable')
      idle = await evaluateReadonlyCdp(
        idleTarget.webSocketDebuggerUrl,
        readonlyComposerGeometryExpression(acceptedButtonLabels),
        5000
      )
      if (idle?.editor && idle?.button && idle.editorTextLength === 0 &&
          idle.buttonLoading === false && idle.buttonDisabled === true &&
          idle.buttonLabelAccepted === true) break
      await sleep(100)
    }
    if (!idle?.editor || !idle?.button) {
      return { ok: false, blocker: 'composer_dom_element_missing' }
    }
    if (idle.editorTextLength !== 0) {
      return { ok: false, blocker: 'composer_editor_not_empty' }
    }
    if (idle.buttonLoading !== false) {
      return { ok: false, blocker: 'composer_runtime_not_ready' }
    }
    if (idle.buttonLabelAccepted !== true || idle.buttonDisabled !== true) {
      return { ok: false, blocker: 'composer_primary_button_not_idle' }
    }
    await dispatchCdpCommands(idleTarget.webSocketDebuggerUrl, [
      {
        method: 'Input.dispatchMouseEvent',
        params: {
          type: 'mousePressed', x: idle.editor.x, y: idle.editor.y,
          button: 'left', clickCount: 1
        }
      },
      {
        method: 'Input.dispatchMouseEvent',
        params: {
          type: 'mouseReleased', x: idle.editor.x, y: idle.editor.y,
          button: 'left', clickCount: 1
        }
      },
      { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'a', code: 'KeyA', modifiers: 4 } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'a', code: 'KeyA', modifiers: 4 } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'Backspace', code: 'Backspace' } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'Backspace', code: 'Backspace' } },
      { method: 'Input.insertText', params: { text } }
    ], 20_000)
    const deadline = Date.now() + 10_000
    let latest = null
    while (Date.now() < deadline) {
      const currentTarget = await resolveCurrentTarget()
      if (!currentTarget?.webSocketDebuggerUrl) throw new Error('composer_target_unavailable')
      latest = await evaluateReadonlyCdp(
        currentTarget.webSocketDebuggerUrl,
        readonlyComposerGeometryExpression(acceptedButtonLabels),
        5000
      )
      if (latest?.button && latest.editorTextLength > 0 && latest.buttonLoading === false &&
          latest.buttonDisabled === false && latest.buttonLabelAccepted === true) {
        await clickCdpGeometry(currentTarget, latest.button)
        return { ok: true, blocker: '' }
      }
      await sleep(100)
    }
    return {
      ok: false,
      blocker: latest?.editorTextLength === 0
        ? 'composer_input_not_observed'
        : latest?.buttonLabelAccepted !== true
          ? 'composer_primary_button_not_idle'
          : latest?.buttonLoading === true
            ? 'composer_runtime_not_ready'
            : latest?.buttonDisabled === true
              ? latest?.modelPickerDisabled === true
                ? 'composer_workspace_not_ready'
                : 'composer_input_state_not_updated'
          : 'composer_primary_button_not_ready'
    }
  } catch (error) {
    return {
      ok: false,
      blocker: error instanceof Error ? error.message : 'composer_cdp_input_failed'
    }
  }
}

async function cdpComposerSubmit(debugPort, text, acceptedButtonLabels = []) {
  try {
    return await cdpComposerSubmitToTarget(
      await waitForDebugTarget(debugPort, 20_000),
      text,
      acceptedButtonLabels,
      () => waitForDebugTarget(debugPort, 5000)
    )
  } catch (error) {
    return {
      ok: false,
      blocker: error instanceof Error ? error.message : 'composer_cdp_input_failed'
    }
  }
}

function readonlyVisibleGeometryExpression(labels) {
  return `(() => {
    const labels = ${JSON.stringify(labels)};
    const elements = Array.from(document.querySelectorAll('button,[role="button"],a'));
    for (const element of elements) {
      const text = String(element.innerText || '').trim();
      const aria = String(element.getAttribute('aria-label') || '').trim();
      const title = String(element.getAttribute('title') || '').trim();
      if (!labels.includes(text) && !labels.includes(aria) && !labels.includes(title)) continue;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) continue;
      return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
    }
    return null;
  })()`
}

async function clickVisibleByLabels(debugPort, labels, timeoutMs = 20_000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const target = await waitForDebugTarget(debugPort, Math.min(5000, deadline - Date.now()))
      const geometry = await evaluateReadonlyCdp(
        target.webSocketDebuggerUrl,
        readonlyVisibleGeometryExpression(labels),
        Math.min(5000, deadline - Date.now())
      )
      if (geometry) {
        await clickCdpGeometry(target, geometry)
        return { ok: true, blocker: '' }
      }
    } catch {
      // Renderer navigation may be settling.
    }
    await sleep(200)
  }
  return { ok: false, blocker: 'visible_control_not_found' }
}

function exactUserTurnKey(turn) {
  const users = (Array.isArray(turn?.items) ? turn.items : []).filter((item) =>
    item?.kind === 'user_message' && item?.status === 'completed' &&
    typeof item?.id === 'string' && item.id)
  return users.length === 1 ? users[0].id : ''
}

function readonlyTurnRowPresentationExpression(turnKey, expectedFragments) {
  return `(() => {
    const turnKey = ${JSON.stringify(turnKey)};
    const expectedFragments = ${JSON.stringify(expectedFragments)};
    const rows = Array.from(document.querySelectorAll('[data-turn-key]'))
      .filter((row) => row.getAttribute('data-turn-key') === turnKey);
    const rowText = rows.length === 1 ? String(rows[0].innerText || '') : '';
    const rowTextComplete = new TextEncoder().encode(rowText).byteLength <=
      ${MAX_PUBLIC_SURFACE_BYTES};
    return {
      turnRowCount: rows.length,
      rowTextComplete,
      expectedFragmentsPresent: rowTextComplete && expectedFragments.length > 0 &&
        expectedFragments.every((fragment) => rowText.includes(fragment)),
      internalReferenceAbsent: rowTextComplete && !/\\bcer1_[a-z0-9_-]+\\b/iu.test(rowText)
    };
  })()`
}

export function turnRowPresentationEvidence(raw) {
  const ok = raw?.turnRowCount === 1 && raw?.rowTextComplete === true &&
    raw?.expectedFragmentsPresent === true && raw?.internalReferenceAbsent === true
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'exact_renderer_turn_row_presentation_invalid',
    turnRowCount: Number(raw?.turnRowCount || 0),
    rowTextComplete: raw?.rowTextComplete === true,
    expectedFragmentsPresent: raw?.expectedFragmentsPresent === true,
    internalReferenceAbsent: raw?.internalReferenceAbsent === true
  })
}

async function observeTurnRowPresentation(debugPort, turn, expectedFragments, timeoutMs = 20_000) {
  const turnKey = exactUserTurnKey(turn)
  if (!turnKey || !Array.isArray(expectedFragments) || expectedFragments.length === 0 ||
    expectedFragments.some((value) => typeof value !== 'string' || !value)) {
    return turnRowPresentationEvidence(null)
  }
  try {
    const target = await waitForDebugTarget(debugPort, timeoutMs)
    if (target.url !== expectedPackagedRendererURL()) {
      return turnRowPresentationEvidence(null)
    }
    return turnRowPresentationEvidence(await evaluateReadonlyCdp(
      target.webSocketDebuggerUrl,
      readonlyTurnRowPresentationExpression(turnKey, expectedFragments),
      timeoutMs
    ))
  } catch {
    return turnRowPresentationEvidence(null)
  }
}

export function readonlyObservationExpression(workspace, exactThreadId = '') {
  return `(async () => {
    const api = window.analytix;
    const parse = (response) => {
      if (!response || response.status < 200 || response.status >= 300) return null;
      try { return JSON.parse(response.body || '{}'); } catch { return null; }
    };
    const publicBody = document.body?.cloneNode(true);
    publicBody?.querySelectorAll('[data-analytix-local-display]').forEach((node) => node.remove());
    const bodyText = String(publicBody?.textContent || '');
    const bodyHTML = String(publicBody?.innerHTML || '');
    const result = {
      apiPresent: !!api,
      title: document.title,
      composerPresent: !!document.querySelector('.ProseMirror[contenteditable="true"]'),
      primaryButtonPresent: !!document.querySelector('.ds-composer-primary-action-button'),
      bodyText: bodyText.length <= ${MAX_PUBLIC_SURFACE_BYTES} ? bodyText : '',
      bodyTextComplete: bodyText.length <= ${MAX_PUBLIC_SURFACE_BYTES},
      bodyHTML: bodyHTML.length <= ${MAX_PUBLIC_SURFACE_BYTES} ? bodyHTML : '',
      bodyHTMLComplete: bodyHTML.length <= ${MAX_PUBLIC_SURFACE_BYTES},
      providerRegistry: null,
      settings: null,
      health: null,
      runtimeInfo: null,
      runtimeTools: null,
      thread: null,
      summary: null,
      sseBody: '',
      sseBodyComplete: true,
      providerAttempts: [],
      providerTerminals: [],
      turnFailures: []
    };
    if (!api) return result;
    try {
      result.providerRegistry = await api.providerRegistry?.request?.({ schemaVersion: 1, operation: 'list' });
    } catch {}
    try {
      const settings = await api.settings?.getSettings?.();
      const profiles = Array.isArray(settings?.provider?.providers) ? settings.provider.providers : [];
      const profile = profiles.find((item) => item?.id === settings.runtime?.providerId);
      result.settings = settings ? {
        workspaceRoot: settings.workspaceRoot,
        runtime: {
          port: settings.runtime?.port,
          dataDir: settings.runtime?.dataDir,
          providerId: settings.runtime?.providerId,
          apiKeyEmpty: settings.runtime?.apiKey === '',
          runtimeTokenEmpty: settings.runtime?.runtimeToken === '',
          model: settings.runtime?.model,
          endpointFormat: settings.runtime?.endpointFormat,
          contextWindowTokens: settings.runtime?.contextWindowTokens,
          executionPolicyVersion: settings.runtime?.executionPolicyVersion,
          approvalPolicy: settings.runtime?.approvalPolicy,
          sandboxMode: settings.runtime?.sandboxMode
        },
        provider: {
          activeProviderId: settings.provider?.activeProviderId,
          topLevelApiKeyEmpty: settings.provider?.apiKey === '',
          allProfilesApiKeyEmpty: profiles.every((item) => item?.apiKey === ''),
          profile: profile ? {
            id: profile.id,
            baseUrl: profile.baseUrl,
            endpointFormat: profile.endpointFormat,
            apiKeyEmpty: profile.apiKey === '',
            models: profile.models
          } : null
        }
      } : null;
    } catch {}
    if (!api.runtime || typeof api.runtime.runtimeRequest !== 'function') return result;
    const runtimeJSON = async (path) => {
      try { return parse(await api.runtime.runtimeRequest(path, 'GET')); } catch { return null; }
    };
    result.health = await runtimeJSON('/health');
    result.runtimeInfo = await runtimeJSON('/v1/runtime/info');
    result.runtimeTools = await runtimeJSON('/v1/runtime/tools?refresh=1');
    const list = await runtimeJSON('/v1/threads?include=side');
    const threads = Array.isArray(list?.threads) ? list.threads : [];
    const exact = ${JSON.stringify(exactThreadId)};
    const workspace = ${JSON.stringify(workspace)};
    const selected = exact
      ? threads.find((item) => item?.id === exact)
      : threads.filter((item) => item?.workspace === workspace)
          .sort((left, right) => String(right.updatedAt || '').localeCompare(String(left.updatedAt || '')))[0];
    if (!selected?.id) return result;
    result.thread = await runtimeJSON('/v1/threads/' + encodeURIComponent(selected.id));
    result.summary = await runtimeJSON('/v1/threads/' + encodeURIComponent(selected.id) + '/summary');
    try {
      const replay = await api.runtime.runtimeRequest(
        '/v1/threads/' + encodeURIComponent(selected.id) + '/events?since_seq=0',
        'GET'
      );
      if (replay && replay.status >= 200 && replay.status < 300) {
        const sseBody = String(replay.body || '');
        result.sseBodyComplete = sseBody.length <= ${MAX_PUBLIC_SURFACE_BYTES};
        result.sseBody = result.sseBodyComplete ? sseBody : '';
        for (const block of result.sseBody.split(/\\r?\\n\\r?\\n/)) {
          const data = block.split(/\\r?\\n/).filter((line) => line.startsWith('data:'))
            .map((line) => line.slice(5).trimStart()).join('\\n');
          if (!data) continue;
          let event;
          try { event = JSON.parse(data); } catch { continue; }
          const safeReasons = ${JSON.stringify([...SAFE_TURN_FAILURE_REASON_CODES])};
          const failureReasonCode = event?.kind === 'turn_failed'
            ? event?.code
            : event?.details?.reasonCode;
          if ((event?.kind === 'turn_failed' ||
              (event?.kind === 'pipeline_stage' && event?.stage === 'provider_error')) &&
              safeReasons.includes(failureReasonCode)) {
            result.turnFailures.push({
              kind: event.kind, stage: event.stage, seq: event.seq,
              threadId: event.threadId, turnId: event.turnId,
              reasonCode: failureReasonCode
            });
          }
          if (event?.kind === 'turn_completed') {
            result.providerTerminals.push({
              kind: event.kind, seq: event.seq, threadId: event.threadId,
              turnId: event.turnId, status: event.status
            });
          } else if (event?.kind === 'usage') {
            const usage = event.usage && typeof event.usage === 'object' ? event.usage : {};
            const diagnostics = event.cacheDiagnostics && typeof event.cacheDiagnostics === 'object'
              ? event.cacheDiagnostics : {};
            result.providerAttempts.push({
              kind: event.kind, seq: event.seq, threadId: event.threadId,
              turnId: event.turnId, model: event.model,
              usageFinalStatus: event.usageFinalStatus,
              promptTokens: usage.promptTokens,
              completionTokens: usage.completionTokens,
              totalTokens: usage.totalTokens,
              turns: usage.turns,
              providerAttemptTelemetrySchema: diagnostics.providerAttemptTelemetrySchema,
              providerAttemptTelemetryValid: diagnostics.providerAttemptTelemetryValid,
              providerLogicalCallCount: diagnostics.providerLogicalCallCount,
              providerAttemptCount: diagnostics.providerAttemptCount,
              providerAttemptStatuses: diagnostics.providerAttemptStatuses
            });
          }
        }
      }
    } catch {}
    return result;
  })()`
}
async function observeRenderer({ debugPort, workspace, exactThreadId = '', timeoutMs = 20_000 }) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  const expectedTargetURL = expectedPackagedRendererURL()
  if (typeof target?.id !== 'string' || !target.id || target.pageTargetCount !== 1 ||
    typeof target.webSocketDebuggerUrl !== 'string' || !target.webSocketDebuggerUrl ||
    target.url !== expectedTargetURL) {
    throw new Error('packaged_renderer_public_observation_target_invalid')
  }
  const expression = readonlyObservationExpression(workspace, exactThreadId)
  const observation = await evaluateReadonlyCdp(
    target.webSocketDebuggerUrl,
    expression,
    timeoutMs
  )
  if (!observation || typeof observation !== 'object' || Array.isArray(observation)) {
    throw new Error('packaged_renderer_public_observation_invalid')
  }
  const projected = Object.freeze({ ...observation, rendererTargetCount: target.pageTargetCount })
  const publicContractDigest = sha256(canonicalJSON({
    health: projected.health,
    runtimeInfo: projected.runtimeInfo,
    runtimeTools: projected.runtimeTools,
    thread: projected.thread,
    providerAttempts: projected.providerAttempts,
    providerTerminals: projected.providerTerminals,
    rendererTargetCount: projected.rendererTargetCount
  }))
  publicRuntimeObservationPrivateBindings.set(projected, Object.freeze({
    kind: 'packaged_electron_main_validated_runtime_public_response_v1',
    targetId: target.id,
    targetURL: target.url,
    targetURLDigest: sha256(String(target.url || '')),
    expressionDigest: sha256(expression),
    workspaceDigest: sha256(workspace),
    requestedThreadIdDigest: exactThreadId ? sha256(exactThreadId) : '',
    observedThreadId: String(projected.thread?.id || ''),
    publicContractDigest
  }))
  return projected
}

function privatePublicRuntimeTurnBindingMatches(result) {
  const observation = result?.observation
  const turn = result?.turn
  const binding = publicRuntimeObservationPrivateBindings.get(observation)
  const turns = Array.isArray(observation?.thread?.turns) ? observation.thread.turns : []
  const matchingTurns = turns.filter((candidate) => candidate?.id === turn?.id)
  const recomputedEvidence = matchingTurns.length === 1
    ? publicTurnEvidence(observation, matchingTurns[0])
    : null
  const publicContractDigest = sha256(canonicalJSON({
    health: observation?.health,
    runtimeInfo: observation?.runtimeInfo,
    runtimeTools: observation?.runtimeTools,
    thread: observation?.thread,
    providerAttempts: observation?.providerAttempts,
    providerTerminals: observation?.providerTerminals,
    rendererTargetCount: observation?.rendererTargetCount
  }))
  return Boolean(binding && Object.isFrozen(observation) &&
    binding.kind === 'packaged_electron_main_validated_runtime_public_response_v1' &&
    binding.targetURL === expectedPackagedRendererURL() &&
    binding.targetURLDigest === sha256(binding.targetURL) &&
    /^[0-9a-f]{64}$/u.test(binding.expressionDigest) &&
    /^[0-9a-f]{64}$/u.test(binding.workspaceDigest) &&
    binding.observedThreadId === result?.threadId && observation?.thread?.id === result?.threadId &&
    matchingTurns.length === 1 && matchingTurns[0] === turn &&
    result?.evidence?.id === turn?.id &&
    canonicalJSON(result?.evidence) === canonicalJSON(recomputedEvidence) &&
    binding.publicContractDigest === publicContractDigest)
}

// Unit tests use the same private binding shape as observeRenderer without
// opening a real packaged Electron target. Keep this seam unavailable when
// the harness is executed as the formal script, so the production predicate
// remains dependent on the main-owned renderer observation binding above.
export function bindPackagedRendererTestTurn(observation, turn, workspace = '/milestone-b-test-workspace') {
  if (!new URL(import.meta.url).searchParams.has('test') &&
      process.env.NODE_ENV !== 'test' && process.env.VITEST !== 'true') {
    throw new Error('packaged_renderer_test_binding_unavailable')
  }
  const threadId = String(turn?.threadId || observation?.thread?.id || '')
  const turns = Array.isArray(observation?.thread?.turns) ? observation.thread.turns : []
  if (!threadId || observation?.thread?.id !== threadId ||
      turns.length !== 1 || turns[0] !== turn || turn?.threadId !== threadId) {
    throw new Error('packaged_renderer_test_turn_binding_invalid')
  }
  const projected = Object.freeze({ ...observation, rendererTargetCount: 1 })
  const targetURL = expectedPackagedRendererURL()
  const publicContractDigest = sha256(canonicalJSON({
    health: projected.health,
    runtimeInfo: projected.runtimeInfo,
    runtimeTools: projected.runtimeTools,
    thread: projected.thread,
    providerAttempts: projected.providerAttempts,
    providerTerminals: projected.providerTerminals,
    rendererTargetCount: projected.rendererTargetCount
  }))
  publicRuntimeObservationPrivateBindings.set(projected, Object.freeze({
    kind: 'packaged_electron_main_validated_runtime_public_response_v1',
    targetId: 'packaged-renderer-test-fixture',
    targetURL,
    targetURLDigest: sha256(targetURL),
    expressionDigest: sha256('packaged-renderer-test-fixture'),
    workspaceDigest: sha256(workspace),
    requestedThreadIdDigest: sha256(threadId),
    observedThreadId: threadId,
    publicContractDigest
  }))
  return Object.freeze({
    observation: projected,
    threadId,
    turn,
    evidence: publicTurnEvidence(projected, turn)
  })
}

function hostToolExecutionObservationExpression({
  threadId,
  turnId,
  toolName,
  workspace,
  arguments: toolArguments
}) {
  const request = { threadId, turnId, toolName, workspace, arguments: toolArguments }
  return `(async () => {
    const api = window.analytix;
    if (!api?.runtime || typeof api.runtime.runtimeRequest !== 'function') {
      return { status: 0, observation: null };
    }
    try {
      const response = await api.runtime.runtimeRequest(
        ${JSON.stringify(TOOL_EXECUTION_OBSERVATION_PATH)},
        'POST',
        ${JSON.stringify(JSON.stringify(request))}
      );
      if (!response || response.ok !== true || response.status !== 200) {
        return {
          status: Number.isSafeInteger(response?.status) ? response.status : 0,
          observation: null
        };
      }
      let observation = null;
      try { observation = JSON.parse(response.body || '{}'); } catch {}
      return { status: response.status, observation };
    } catch {
      return { status: 0, observation: null };
    }
  })()`
}

export function hostToolExecutionObservationEvidence(raw, expected) {
  const transportStatus = Number.isSafeInteger(raw?.status) ? raw.status : 0
  const base = Object.freeze({
    ok: false,
    blocker: 'host_owned_tool_invocation_binding_unavailable',
    transportStatus,
    toolName: '',
    disclosure: '',
    privatePayloadWithheld: false,
    observationDigest: '',
    authorityBindingDigest: ''
  })
  const observation = raw?.observation
  if (!raw || typeof raw !== 'object' || Array.isArray(raw) ||
    Object.keys(raw).sort().join(',') !== 'observation,status' ||
    typeof expected?.threadId !== 'string' || !expected.threadId ||
    expected.threadId !== expected.threadId.trim() ||
    typeof expected?.turnId !== 'string' || !expected.turnId ||
    expected.turnId !== expected.turnId.trim() || expected?.toolName !== 'bash' ||
    transportStatus !== 200 || !observation || typeof observation !== 'object' ||
    Array.isArray(observation) ||
    Object.keys(observation).sort().join(',') !== TOOL_EXECUTION_OBSERVATION_FIELDS.join(',') ||
    observation.schemaVersion !== 1 || observation.disclosure !== 'metadata_only' ||
    observation.privatePayloadWithheld !== true ||
    observation.threadId !== expected.threadId || observation.turnId !== expected.turnId ||
    observation.toolName !== 'bash' || observation.status !== 'completed' ||
    ![observation.workId, observation.receiptId, observation.dispositionId,
      observation.executionGrantId, observation.resultItemDigest]
      .every((value) => /^[0-9a-f]{64}$/.test(String(value || ''))) ||
    !/^item_result_[0-9a-f]{64}$/.test(String(observation.resultItemId || ''))) {
    return base
  }
  return Object.freeze({
    ...base,
    ok: true,
    blocker: '',
    toolName: 'bash',
    disclosure: 'metadata_only',
    privatePayloadWithheld: true,
    observationDigest: sha256(canonicalJSON(observation)),
    authorityBindingDigest: sha256(canonicalJSON({
      workId: observation.workId,
      receiptId: observation.receiptId,
      dispositionId: observation.dispositionId,
      executionGrantId: observation.executionGrantId,
      resultItemId: observation.resultItemId,
      resultItemDigest: observation.resultItemDigest
    }))
  })
}

function normalizeOrdinaryBashObservationRequest(request, resolveWorkspaceRealPath = realpathSync) {
  if (!request || typeof request !== 'object' || Array.isArray(request) ||
    Object.keys(request).sort().join(',') !==
      'arguments,threadId,toolName,turnId,workspace' ||
    typeof request.threadId !== 'string' || !request.threadId ||
    request.threadId !== request.threadId.trim() ||
    typeof request.turnId !== 'string' || !request.turnId ||
    request.turnId !== request.turnId.trim() || request.toolName !== 'bash' ||
    typeof request.workspace !== 'string' || !request.workspace ||
    request.workspace !== request.workspace.trim() ||
    typeof resolveWorkspaceRealPath !== 'function' ||
    !request.arguments || typeof request.arguments !== 'object' ||
    Array.isArray(request.arguments) ||
    Object.keys(request.arguments).sort().join(',') !== 'command' ||
    typeof request.arguments.command !== 'string' ||
    ![...ORDINARY_BASH_COMMANDS.values()].includes(request.arguments.command)) {
    return null
  }
  const workspaceRealPath = resolveWorkspaceRealPath(request.workspace)
  const workspaceStat = lstatSync(workspaceRealPath)
  if (!isAbsolute(workspaceRealPath) || workspaceStat.isSymbolicLink() ||
    !workspaceStat.isDirectory()) return null
  const canonicalArguments = canonicalJSON({ command: request.arguments.command })
  return Object.freeze({
    request: Object.freeze({
      threadId: request.threadId,
      turnId: request.turnId,
      toolName: 'bash',
      workspace: workspaceRealPath,
      arguments: Object.freeze({ command: request.arguments.command })
    }),
    workspaceRealPath,
    canonicalArguments
  })
}

function privateHostToolExecutionBindingMatches(evidence, expected) {
  const binding = hostToolExecutionPrivateBindings.get(evidence)
  let normalized = null
  try {
    normalized = normalizeOrdinaryBashObservationRequest(expected)
  } catch {
    normalized = null
  }
  return Boolean(binding && normalized && Object.isFrozen(evidence) && evidence.ok === true &&
    evidence.toolName === 'bash' && evidence.disclosure === 'metadata_only' &&
    evidence.privatePayloadWithheld === true &&
    binding.transportProvenance?.kind === 'cdp_window_analytix_runtime_request_v1' &&
    binding.transportProvenance?.path === TOOL_EXECUTION_OBSERVATION_PATH &&
    binding.transportProvenance?.method === 'POST' &&
    binding.transportProvenance?.status === 200 &&
    binding.transportProvenance?.rendererTargetCount === 1 &&
    /^[0-9a-f]{64}$/.test(String(binding.transportProvenance?.expressionDigest || '')) &&
    binding.threadId === normalized.request.threadId &&
    binding.turnId === normalized.request.turnId && binding.toolName === 'bash' &&
    binding.workspaceRealPath === normalized.workspaceRealPath &&
    binding.canonicalArguments === normalized.canonicalArguments &&
    binding.observationDigest === evidence.observationDigest &&
    binding.authorityBindingDigest === evidence.authorityBindingDigest)
}

export async function observeHostToolExecution(input, internalDependencies = {}) {
  const { debugPort, timeoutMs = 20_000, ...request } = input || {}
  const waitForTarget = typeof internalDependencies.waitForDebugTarget === 'function'
    ? internalDependencies.waitForDebugTarget
    : waitForDebugTarget
  const evaluateBridge = typeof internalDependencies.evaluateReadonlyCdp === 'function'
    ? internalDependencies.evaluateReadonlyCdp
    : evaluateReadonlyCdp
  const resolveWorkspaceRealPath =
    typeof internalDependencies.resolveWorkspaceRealPath === 'function'
      ? internalDependencies.resolveWorkspaceRealPath
      : realpathSync
  let normalized = null
  try {
    normalized = normalizeOrdinaryBashObservationRequest(request, resolveWorkspaceRealPath)
  } catch {
    normalized = null
  }
  const expected = {
    threadId: normalized?.request.threadId || '',
    turnId: normalized?.request.turnId || '',
    toolName: normalized?.request.toolName || ''
  }
  if (!normalized || !Number.isSafeInteger(debugPort) || debugPort <= 0 ||
    !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0) {
    return hostToolExecutionObservationEvidence(
      { status: 0, observation: null },
      expected
    )
  }
  try {
    const target = await waitForTarget(debugPort, timeoutMs)
    if (target?.pageTargetCount !== 1 ||
      typeof target.webSocketDebuggerUrl !== 'string' ||
      !target.webSocketDebuggerUrl) {
      throw new Error('host_tool_execution_observation_target_invalid')
    }
    const expression = hostToolExecutionObservationExpression(normalized.request)
    const raw = await evaluateBridge(target.webSocketDebuggerUrl, expression, timeoutMs)
    const evidence = hostToolExecutionObservationEvidence(raw, expected)
    if (!evidence.ok) return evidence
    hostToolExecutionPrivateBindings.set(evidence, Object.freeze({
      transportProvenance: Object.freeze({
        kind: 'cdp_window_analytix_runtime_request_v1',
        path: TOOL_EXECUTION_OBSERVATION_PATH,
        method: 'POST',
        status: raw.status,
        rendererTargetCount: target.pageTargetCount,
        expressionDigest: sha256(expression)
      }),
      threadId: normalized.request.threadId,
      turnId: normalized.request.turnId,
      toolName: 'bash',
      workspaceRealPath: normalized.workspaceRealPath,
      canonicalArguments: normalized.canonicalArguments,
      observationDigest: evidence.observationDigest,
      authorityBindingDigest: evidence.authorityBindingDigest
    }))
    return evidence
  } catch {
    return hostToolExecutionObservationEvidence(
      { status: 0, observation: null },
      expected
    )
  }
}

export function milestoneBLocalProvider(observation) {
  return localProviderFromObservation(observation, {
    requiredModel: 'deepseek-v4-flash',
    requireContextWindow: false
  })
}

function fundsServerAvailable(tools) {
  const diagnostics = Array.isArray(tools?.mcpServers)
    ? tools.mcpServers.filter((item) => item?.id === 'analytix_funds')
    : []
  return diagnostics.length === 1 && diagnostics[0]?.enabled === true &&
    diagnostics[0]?.available === true && diagnostics[0]?.connected === true &&
    Number(diagnostics[0]?.toolCount || 0) > 0
}

function fundsServerExplicitlyUnavailable(tools) {
  const diagnostics = Array.isArray(tools?.mcpServers)
    ? tools.mcpServers.filter((item) => item?.id === 'analytix_funds')
    : []
  return diagnostics.length === 1 && diagnostics[0]?.status === 'unavailable' &&
    [
      'funds_package_binding_changed',
      'funds_packaged_source_invalid',
      'funds_installed_state_invalid'
    ].includes(diagnostics[0]?.failureCode) &&
    diagnostics[0]?.enabled === false && diagnostics[0]?.available === false &&
    Number.isSafeInteger(diagnostics[0]?.toolCount) && diagnostics[0].toolCount === 0
}

export function ordinaryPublicSeam(observation, runtimePort) {
  const health = observation?.health
  const info = observation?.runtimeInfo
  const tools = observation?.runtimeTools
  const contracts = tools?.toolContracts
  const fundsDiagnostics = Array.isArray(tools?.mcpServers)
    ? tools.mcpServers.filter((item) => item?.id === 'analytix_funds')
    : []
  const ordinaryCatalog = Number.isSafeInteger(contracts?.count) && contracts.count > 0 &&
    isSHA256(contracts?.catalogHash)
  return {
    ok: health?.service === 'analytix' && info?.schemaVersion === 2 && info?.status === 'ready' &&
      info?.listenerScope === 'loopback' && info?.port === runtimePort && info?.insecure === false &&
      info?.storage?.configured === true && info?.storage?.available === true &&
      info?.executionPolicy?.approvalPolicy === 'auto' &&
      info?.executionPolicy?.sandboxMode === 'danger-full-access' &&
      tools?.schemaVersion === 2 && Number(tools?.providerCount || 0) > 0 && ordinaryCatalog,
    healthOk: health?.service === 'analytix',
    runtimeInfoOk: info?.schemaVersion === 2 && info?.status === 'ready',
    ordinaryCatalogNonempty: ordinaryCatalog,
    ordinaryToolContractCount: ordinaryCatalog ? contracts.count : 0,
    ordinaryToolCatalogHash: ordinaryCatalog ? contracts.catalogHash : '',
    fundsAvailable: fundsServerAvailable(tools),
    fundsExplicitlyUnavailable: fundsServerExplicitlyUnavailable(tools),
    fundsServerDiagnosticCount: fundsDiagnostics.length
  }
}

export async function waitForLocalProviderWorkbench({
  debugPort, workspace, runtimePort, runtimeDataDir, timeoutMs, exactThreadId = '',
  expectedFunds = 'independent', providerRequired = expectedFunds === 'available',
  onFreshRegistry = null, expectedProvider = null
}, internalDependencies = {}) {
  const observe = internalDependencies.observeRenderer || observeRenderer
  const pause = internalDependencies.sleep || sleep
  const deadline = Date.now() + timeoutMs
  let latest = null
  let normalLocalProviderSetupObserved = false
  let initialRegistry = null
  let lastObservationBlocker = ''
  const observationSliceMs = diagnosticUnpackaged ? 30_000 : 10_000
  while (Date.now() < deadline) {
    try {
      latest = await observe({
        debugPort,
        workspace,
        exactThreadId,
        timeoutMs: Math.min(observationSliceMs, Math.max(1000, deadline - Date.now()))
      })
      const registry = latest?.providerRegistry
      if (!initialRegistry && freshLocalProviderRegistry(registry)) {
        initialRegistry = registry
        if (typeof onFreshRegistry === 'function') {
          await onFreshRegistry(Math.max(1, deadline - Date.now()))
          continue
        }
      }
      const provider = milestoneBLocalProvider(latest)
      if (provider.ok && expectedProvider?.ok && !sameLocalProviderAuthority(expectedProvider, provider)) {
        return { ok: false, blocked: false, blocker: 'local_provider_authority_changed',
          observation: latest, provider, publicSeam: ordinaryPublicSeam(latest, runtimePort),
          normalLocalProviderSetupObserved }
      }
      normalLocalProviderSetupObserved ||= Boolean(initialRegistry && provider.ok &&
        registry?.registryIncarnation === initialRegistry.registryIncarnation &&
        registry.registryRevision !== initialRegistry.registryRevision)
      const publicSeam = ordinaryPublicSeam(latest, runtimePort)
      const settingsReady = latest?.settings?.workspaceRoot === workspace &&
        latest?.settings?.runtime?.dataDir === runtimeDataDir &&
        latest?.settings?.runtime?.executionPolicyVersion === 2 &&
        latest?.settings?.runtime?.approvalPolicy === 'auto' &&
        latest?.settings?.runtime?.sandboxMode === 'danger-full-access'
      const rendererReady = latest?.rendererTargetCount === 1 && latest?.apiPresent === true &&
        latest?.composerPresent === true && latest?.primaryButtonPresent === true
      // Runtime diagnostics are global transport state, not a per-turn case
      // authority seam. Before DSV2 admission, the actual protected turn below
      // must prove fail-closed behavior; ordinary workbench readiness stays
      // independent from absent or unavailable funds diagnostics.
      const fundsReady = expectedFunds !== 'available' || publicSeam.fundsAvailable
      const localRuntimeReady = rendererReady && settingsReady && publicSeam.healthOk &&
        publicSeam.runtimeInfoOk
      const providerReady = provider.ok && publicSeam.ok
      if (localRuntimeReady && (!providerRequired || providerReady) && fundsReady) {
        return {
          ok: true,
          blocked: false,
          blocker: '',
          observation: latest,
          provider,
          publicSeam,
          normalLocalProviderSetupObserved,
          providerRequired
        }
      }
    } catch (error) {
      // Visible Provider setup, renderer reload, and runtime restart are retried through read-only seams.
      lastObservationBlocker = error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
        ? error.message
        : 'packaged_renderer_public_observation_failed'
    }
    await pause(750)
  }
  const provider = milestoneBLocalProvider(latest)
  const publicSeam = ordinaryPublicSeam(latest, runtimePort)
  return {
    ok: false,
    blocked: providerRequired && !provider.ok,
    blocker: !latest
      ? lastObservationBlocker || 'packaged_renderer_not_ready'
      : (providerRequired && provider.blocker) || (expectedFunds === 'available'
          ? 'packaged_funds_server_not_available'
          : 'packaged_ordinary_public_seam_not_ready'),
    observation: latest,
    provider,
    publicSeam,
    normalLocalProviderSetupObserved,
    providerRequired
  }
}

function successfulToolExecutions(turn) {
  const calls = new Map()
  const results = []
  for (const item of Array.isArray(turn?.items) ? turn.items : []) {
    if (item?.kind === 'tool_call' && item.status === 'completed' && item.callId && item.toolName) {
      calls.set(item.callId, item)
    } else if (item?.kind === 'tool_result') {
      results.push(item)
    }
  }
  return results.flatMap((item) => {
    const call = calls.get(item?.callId)
    if (!call || call.toolName !== item.toolName || call.toolKind !== item.toolKind ||
        item.status !== 'completed' || item.isError !== false || item.output?.status !== 'completed') {
      return []
    }
    return [{
      callId: item.callId,
      toolName: item.toolName,
      toolKind: item.toolKind,
      resultProjectionKind: item.output?.projectionKind || '',
      resultMessageKey: item.output?.messageKey || ''
    }]
  })
}

function assistantText(turn) {
  return (Array.isArray(turn?.items) ? turn.items : [])
    .filter((item) => item?.kind === 'assistant_text' && item.status === 'completed')
    .map((item) => String(item.text || ''))
    .join('\n')
}

const GENERAL_COMPACTION_SUMMARY_V3 =
  'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.'
const CASE_COMPACTION_SUMMARY_V1 =
  'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'
const TASK_CONTINUATION_EVIDENCE_STATE_V1 = 'unverified_for_case_facts'

function goJSON(value) {
  return JSON.stringify(value)
    .replaceAll('&', '\\u0026')
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('\u2028', '\\u2028')
    .replaceAll('\u2029', '\\u2029')
}

function goCanonicalJSON(value) {
  return canonicalJSON(value)
    .replaceAll('&', '\\u0026')
    .replaceAll('<', '\\u003c')
    .replaceAll('>', '\\u003e')
    .replaceAll('\u2028', '\\u2028')
    .replaceAll('\u2029', '\\u2029')
}

function validCompactionStringList(value, allowEmpty = false) {
  if (!Array.isArray(value) || (!allowEmpty && value.length === 0)) return false
  const normalized = value.map((item) => typeof item === 'string' ? item.trim() : '')
  return normalized.every((item, index) => item && item === value[index]) &&
    new Set(normalized).size === normalized.length
}

function exactOptionalKeys(value, required, optional) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const keys = Object.keys(value)
  if (!required.every((key) => keys.includes(key)) ||
      keys.some((key) => !required.includes(key) && !optional.includes(key))) {
    return false
  }
  return true
}

function trimmedNonempty(value) {
  return typeof value === 'string' && value.length > 0 && value === value.trim()
}

function safeCompactionRecordID(value) {
  let cleaned = String(value || '').trim()
    .replaceAll('/', '_')
    .replaceAll('\\', '_')
    .replaceAll(':', '_')
    .replaceAll('..', '__')
  if (!cleaned || cleaned === '.') cleaned = '_'
  return cleaned
}

function goTaskContinuationGoal(goal) {
  return {
    goalId: goal.goalId,
    objective: goal.objective,
    status: goal.status,
    stateDigest: goal.stateDigest
  }
}

function goTaskContinuationTodo(todo) {
  const out = {
    todoId: todo.todoId,
    content: todo.content,
    status: todo.status
  }
  if (todo.statusReasonCode !== undefined) out.statusReasonCode = todo.statusReasonCode
  out.stateDigest = todo.stateDigest
  return out
}

function goTaskContinuationEvidenceReference(reference) {
  return {
    referenceDigest: reference.referenceDigest,
    supportStatus: reference.supportStatus
  }
}

function goTaskContinuationEvidenceWitnessV2(witness, bindingDigest = witness.bindingDigest) {
  return {
    schemaVersion: witness.schemaVersion,
    purpose: witness.purpose,
    installationId: witness.installationId,
    enrollmentId: witness.enrollmentId,
    namespace: witness.namespace,
    authorityKeyId: witness.authorityKeyId,
    witnessKeyId: witness.witnessKeyId,
    bundleRecordDigest: witness.bundleRecordDigest,
    bundleGeneration: witness.bundleGeneration,
    observeRequestDigest: witness.observeRequestDigest,
    checkpointDigest: witness.checkpointDigest,
    observationDigest: witness.observationDigest,
    evidenceAuthorityBindingDigest: witness.evidenceAuthorityBindingDigest,
    bindingDigest
  }
}

function validTaskContinuationEvidenceWitnessV2(witness) {
  if (!exactKeys(witness, [
    'schemaVersion', 'purpose', 'installationId', 'enrollmentId', 'namespace',
    'authorityKeyId', 'witnessKeyId', 'bundleRecordDigest', 'bundleGeneration',
    'observeRequestDigest', 'checkpointDigest', 'observationDigest',
    'evidenceAuthorityBindingDigest', 'bindingDigest'
  ]) || witness.schemaVersion !== 2 ||
      witness.purpose !== 'analytix.task-continuation-evidence-witness/v2' ||
      witness.namespace !== 'analytix.evidence-registry-authority/v1' ||
      !Number.isSafeInteger(witness.bundleGeneration) || witness.bundleGeneration <= 0) {
    return false
  }
  for (const key of [
    'installationId', 'enrollmentId', 'authorityKeyId', 'witnessKeyId',
    'bundleRecordDigest', 'observeRequestDigest', 'checkpointDigest',
    'observationDigest', 'evidenceAuthorityBindingDigest', 'bindingDigest'
  ]) {
    if (!isSHA256(witness[key])) return false
  }
  const body = goJSON(goTaskContinuationEvidenceWitnessV2(witness, ''))
  return witness.bindingDigest === sha256(Buffer.concat([
    Buffer.from('analytix.task-continuation-evidence-witness/digest/v2\0'),
    Buffer.from(body)
  ]))
}

function goTaskContinuationEvidenceReferenceV2(reference, referenceDigest = reference.referenceDigest) {
  return {
    schemaVersion: reference.schemaVersion,
    purpose: reference.purpose,
    contextDigest: reference.contextDigest,
    datasetSnapshotId: reference.datasetSnapshotId,
    registrySequence: reference.registrySequence,
    registryStateDigest: reference.registryStateDigest,
    evidenceRegistryIndexDigest: reference.evidenceRegistryIndexDigest,
    evidenceRegistryCount: reference.evidenceRegistryCount,
    selectedRegistryIndexDigest: reference.selectedRegistryIndexDigest,
    selectedRegistryIndexGeneration: reference.selectedRegistryIndexGeneration,
    selectedRegistryCapsuleDigest: reference.selectedRegistryCapsuleDigest,
    registryIndexPathDigest: reference.registryIndexPathDigest,
    registryIndexPathCount: reference.registryIndexPathCount,
    witnessBinding: goTaskContinuationEvidenceWitnessV2(reference.witnessBinding),
    supportStatus: reference.supportStatus,
    referenceDigest
  }
}

function validTaskContinuationEvidenceReferenceV2(reference) {
  if (!exactKeys(reference, [
    'schemaVersion', 'purpose', 'contextDigest', 'datasetSnapshotId', 'registrySequence',
    'registryStateDigest', 'evidenceRegistryIndexDigest', 'evidenceRegistryCount',
    'selectedRegistryIndexDigest', 'selectedRegistryIndexGeneration',
    'selectedRegistryCapsuleDigest', 'registryIndexPathDigest', 'registryIndexPathCount',
    'witnessBinding', 'supportStatus', 'referenceDigest'
  ]) || reference.schemaVersion !== 2 ||
      reference.purpose !== 'analytix.task-continuation-evidence-reference/v2' ||
      !/^dsv2_[0-9a-f]{64}$/u.test(String(reference.datasetSnapshotId || '')) ||
      reference.supportStatus !== 'trace_only_unverified_for_case_facts' ||
      !Number.isSafeInteger(reference.registrySequence) || reference.registrySequence <= 0 ||
      !Number.isSafeInteger(reference.evidenceRegistryCount) || reference.evidenceRegistryCount <= 0 ||
      !Number.isSafeInteger(reference.selectedRegistryIndexGeneration) ||
      reference.selectedRegistryIndexGeneration <= 0 ||
      reference.selectedRegistryIndexGeneration > reference.evidenceRegistryCount ||
      !Number.isSafeInteger(reference.registryIndexPathCount) ||
      reference.registryIndexPathCount !==
        reference.evidenceRegistryCount - reference.selectedRegistryIndexGeneration + 1 ||
      !validTaskContinuationEvidenceWitnessV2(reference.witnessBinding)) {
    return false
  }
  for (const key of [
    'contextDigest', 'registryStateDigest', 'evidenceRegistryIndexDigest',
    'selectedRegistryIndexDigest', 'selectedRegistryCapsuleDigest',
    'registryIndexPathDigest', 'referenceDigest'
  ]) {
    if (!isSHA256(reference[key])) return false
  }
  const body = goJSON(goTaskContinuationEvidenceReferenceV2(reference, ''))
  return reference.referenceDigest === sha256(Buffer.concat([
    Buffer.from('analytix.task-continuation-evidence-reference/digest/v2\0'),
    Buffer.from(body)
  ]))
}

function goTaskContinuationSnapshot(snapshot, includeStateDigest) {
  const out = { schemaVersion: snapshot.schemaVersion }
  if (snapshot.goal !== undefined) out.goal = goTaskContinuationGoal(snapshot.goal)
  out.todos = snapshot.todos.map(goTaskContinuationTodo)
  out.latestUserConstraints = [...snapshot.latestUserConstraints]
  out.evidenceReferences = snapshot.evidenceReferences.map(goTaskContinuationEvidenceReference)
  if (snapshot.evidenceRegistryReferenceV2 !== undefined) {
    out.evidenceRegistryReferenceV2 = goTaskContinuationEvidenceReferenceV2(
      snapshot.evidenceRegistryReferenceV2
    )
  }
  out.evidenceAuthority = snapshot.evidenceAuthority
  if (snapshot.previousContinuationDigest !== undefined) {
    out.previousContinuationDigest = snapshot.previousContinuationDigest
  }
  if (snapshot.previousCompactionSourceDigest !== undefined) {
    out.previousCompactionSourceDigest = snapshot.previousCompactionSourceDigest
  }
  if (includeStateDigest) out.stateDigest = snapshot.stateDigest
  return out
}

function validTaskContinuationSnapshotV1(snapshot) {
  if (!exactOptionalKeys(snapshot, [
    'schemaVersion', 'todos', 'latestUserConstraints', 'evidenceReferences',
    'evidenceAuthority', 'stateDigest'
  ], [
    'goal', 'evidenceRegistryReferenceV2', 'previousContinuationDigest',
    'previousCompactionSourceDigest'
  ]) || snapshot.schemaVersion !== 1 ||
      snapshot.evidenceAuthority !== TASK_CONTINUATION_EVIDENCE_STATE_V1 ||
      !Array.isArray(snapshot.todos) || snapshot.todos.length > 200 ||
      !Array.isArray(snapshot.latestUserConstraints) ||
      snapshot.latestUserConstraints.length > 4 ||
      !Array.isArray(snapshot.evidenceReferences) || snapshot.evidenceReferences.length > 32 ||
      !isSHA256(snapshot.stateDigest)) {
    return false
  }
  if (snapshot.goal !== undefined && (!exactKeys(snapshot.goal, [
    'goalId', 'objective', 'status', 'stateDigest'
  ]) || !trimmedNonempty(snapshot.goal.goalId) || !trimmedNonempty(snapshot.goal.objective) ||
      !['active', 'paused', 'blocked', 'usageLimited', 'budgetLimited'].includes(snapshot.goal.status) ||
      !isSHA256(snapshot.goal.stateDigest))) {
    return false
  }
  const todoIDs = new Set()
  for (const todo of snapshot.todos) {
    const terminal = todo?.status === 'failed' || todo?.status === 'canceled'
    const required = ['todoId', 'content', 'status', 'stateDigest']
    const optional = terminal ? ['statusReasonCode'] : []
    if (!exactOptionalKeys(todo, required, optional) || !trimmedNonempty(todo.todoId) ||
        todoIDs.has(todo.todoId) || !trimmedNonempty(todo.content) ||
        !['pending', 'in_progress', 'failed', 'canceled'].includes(todo.status) ||
        terminal !== trimmedNonempty(todo.statusReasonCode) || !isSHA256(todo.stateDigest)) {
      return false
    }
    todoIDs.add(todo.todoId)
  }
  if (!snapshot.latestUserConstraints.every(trimmedNonempty)) return false
  const evidenceDigests = new Set()
  for (const reference of snapshot.evidenceReferences) {
    if (!exactKeys(reference, ['referenceDigest', 'supportStatus']) ||
        !isSHA256(reference.referenceDigest) || evidenceDigests.has(reference.referenceDigest) ||
        reference.supportStatus !== TASK_CONTINUATION_EVIDENCE_STATE_V1) {
      return false
    }
    evidenceDigests.add(reference.referenceDigest)
  }
  if (snapshot.evidenceRegistryReferenceV2 !== undefined &&
      !validTaskContinuationEvidenceReferenceV2(snapshot.evidenceRegistryReferenceV2)) {
    return false
  }
  for (const key of ['previousContinuationDigest', 'previousCompactionSourceDigest']) {
    if (snapshot[key] !== undefined && !isSHA256(snapshot[key])) return false
  }
  return snapshot.stateDigest === sha256(goJSON(goTaskContinuationSnapshot(snapshot, false)))
}

function goCaseCompactionBinding(binding) {
  const out = {
    schemaVersion: binding.schemaVersion,
    purpose: binding.purpose,
    threadIdHash: binding.threadIdHash,
    sourceContextDigest: binding.sourceContextDigest,
    compactedTurnsDigest: binding.compactedTurnsDigest,
    continuationDigest: binding.continuationDigest,
    authorityTurnIds: [...binding.authorityTurnIds],
    operationStamp: binding.operationStamp
  }
  if (binding.previousCompactionSourceDigest !== undefined) {
    out.previousCompactionSourceDigest = binding.previousCompactionSourceDigest
  }
  return out
}

function validCaseCompactionBinding(thread, item) {
  const binding = item?.caseCompactionBinding
  if (!exactOptionalKeys(binding, [
    'schemaVersion', 'purpose', 'threadIdHash', 'sourceContextDigest',
    'compactedTurnsDigest', 'continuationDigest', 'authorityTurnIds', 'operationStamp'
  ], ['previousCompactionSourceDigest']) || binding.schemaVersion !== 1 ||
      binding.purpose !== 'analytix.case-compaction-operation-binding/v1' ||
      binding.threadIdHash !== sha256(String(thread?.id || '')) ||
      binding.sourceContextDigest !== item.sourceContextDigest ||
      binding.continuationDigest !== item.taskContinuation?.stateDigest ||
      !isSHA256(binding.compactedTurnsDigest) ||
      !validCompactionStringList(binding.authorityTurnIds) ||
      [...binding.authorityTurnIds].sort().join('\0') !== binding.authorityTurnIds.join('\0') ||
      !/^[1-9][0-9]*$/u.test(String(binding.operationStamp || '')) ||
      binding.operationStamp !== String(item.id || '').split('_').at(-1) ||
      (binding.previousCompactionSourceDigest !== undefined &&
        !isSHA256(binding.previousCompactionSourceDigest))) {
    return false
  }
  return item.sourceDigest === sha256(goJSON(goCaseCompactionBinding(binding)))
}

function compactionCommonValid(thread, turn, item, allowEmptySourceIDs) {
  if (!item || typeof item !== 'object' || Array.isArray(item)) return false
  const sourceDigest = typeof item.sourceDigest === 'string' ? item.sourceDigest.trim() : ''
  const operationStamp = String(item.id || '').split('_').at(-1) || ''
  const safeThreadID = safeCompactionRecordID(thread?.id)
  const proofRecord = { ...item }
  delete proofRecord.reasoningExclusionProof
  const createdAt = Date.parse(String(item.createdAt || ''))
  const finishedAt = Date.parse(String(item.finishedAt || ''))
  return item.kind === 'compaction' && item.role === 'system' && item.status === 'completed' &&
    turn?.status === 'completed' && trimmedNonempty(thread?.id) &&
    item.threadId === thread.id && item.turnId === turn?.id &&
    trimmedNonempty(item.id) && /^[1-9][0-9]*$/u.test(operationStamp) &&
    item.id === `compaction_${safeThreadID}_${operationStamp}` &&
    item.turnId === `turn_${safeThreadID}_compaction_${operationStamp}` &&
    Number.isSafeInteger(item.replacedTokens) && item.replacedTokens > 0 &&
    isSHA256(sourceDigest) && item.sourceDigest === sourceDigest &&
    item.digestMarker === `sha256:${sourceDigest.slice(0, 12)}` &&
    validCompactionStringList(item.sourceItemIds, allowEmptySourceIDs) &&
    item.reasoningExcluded === true &&
    item.reasoningExclusionProof === `sha256:${sha256(goCanonicalJSON(proofRecord))}` &&
    item.auto !== undefined && typeof item.auto === 'boolean' &&
    Array.isArray(item.pinnedConstraints) &&
    canonicalJSON(item.pinnedConstraints) === canonicalJSON(['user: preserve recent turns']) &&
    Number.isFinite(createdAt) && Number.isFinite(finishedAt) &&
    finishedAt === createdAt && String(item.createdAt) === String(item.finishedAt)
}

export function inspectCompactionItem(thread, turn, item) {
  if (!compactionCommonValid(thread, turn, item, true)) {
    return { ok: false, schemaKind: 'invalid', blocker: 'compaction_common_contract_invalid' }
  }
  const generalV3Keys = [
    'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind',
    'summary', 'replacedTokens', 'auto', 'pinnedConstraints', 'sourceDigest', 'digestMarker',
    'sourceItemIds', 'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof',
    'assistantProseExcluded', 'toolPayloadsExcluded', 'caseFactsExcluded',
    'providerHistoryProjectionVersion'
  ]
  const generalV4Keys = [...generalV3Keys, 'taskContinuation']
  const caseV2Keys = [
    'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind',
    'summary', 'replacedTokens', 'auto', 'pinnedConstraints', 'sourceDigest', 'digestMarker',
    'sourceItemIds', 'schemaVersion', 'reasoningExcluded', 'reasoningExclusionProof',
    'caseFactsExcluded', 'caseHistoryProjectionVersion'
  ]
  const caseV3Keys = [
    ...caseV2Keys, 'taskContinuation', 'sourceContextDigest', 'caseCompactionBinding'
  ]
  if (exactKeys(item, generalV3Keys) && item.schemaVersion === 3 && item.auto === false &&
      item.summary === GENERAL_COMPACTION_SUMMARY_V3 &&
      validCompactionStringList(item.sourceItemIds) &&
      item.providerHistoryProjectionVersion === 1 && item.assistantProseExcluded === true &&
      item.toolPayloadsExcluded === true && item.caseFactsExcluded === true) {
    return { ok: true, schemaKind: 'general_v3', blocker: '' }
  }
  if (exactKeys(item, generalV4Keys) && item.schemaVersion === 4 && item.auto === true &&
      item.summary === GENERAL_COMPACTION_SUMMARY_V3 &&
      validCompactionStringList(item.sourceItemIds) &&
      item.providerHistoryProjectionVersion === 2 && item.assistantProseExcluded === true &&
      item.toolPayloadsExcluded === true && item.caseFactsExcluded === true &&
      validTaskContinuationSnapshotV1(item.taskContinuation)) {
    return { ok: true, schemaKind: 'general_v4', blocker: '' }
  }
  if (exactKeys(item, caseV2Keys) && item.schemaVersion === 2 &&
      item.summary === CASE_COMPACTION_SUMMARY_V1 && item.caseFactsExcluded === true &&
      item.caseHistoryProjectionVersion === 1 && item.sourceItemIds.length === 0) {
    return { ok: true, schemaKind: 'case_v2', blocker: '' }
  }
  if (exactKeys(item, caseV3Keys) && item.schemaVersion === 3 &&
      item.summary === CASE_COMPACTION_SUMMARY_V1 && item.caseFactsExcluded === true &&
      item.caseHistoryProjectionVersion === 2 && item.sourceItemIds.length === 0 &&
      isSHA256(item.sourceContextDigest) && validTaskContinuationSnapshotV1(item.taskContinuation) &&
      validCaseCompactionBinding(thread, item)) {
    return { ok: true, schemaKind: 'case_v3', blocker: '' }
  }
  return { ok: false, schemaKind: 'invalid', blocker: 'compaction_schema_contract_invalid' }
}

function publicCompactionSourceItems(thread) {
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  if (turns.length <= 2) return { ok: false, blocker: 'pre_compaction_history_too_short' }
  const items = []
  const ids = []
  const seen = new Set()
  for (const turn of turns.slice(0, -2)) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      const kind = String(item?.kind || '').trim()
      if (!item || typeof item !== 'object' || Array.isArray(item) ||
          ['error', 'assistant_reasoning', 'compaction'].includes(kind) ||
          /^item_.+_assistant_process_/u.test(String(item?.id || ''))) {
        continue
      }
      const id = String(item.id || '').trim()
      if (!id || id !== item.id || seen.has(id)) {
        return { ok: false, blocker: 'pre_compaction_public_source_identity_invalid' }
      }
      seen.add(id)
      ids.push(id)
      items.push(structuredClone(item))
    }
  }
  if (items.length === 0) return { ok: false, blocker: 'pre_compaction_public_source_empty' }
  return { ok: true, blocker: '', items, ids }
}

function exactGeneralCompactionReplacement(beforeThread, afterThread, compactionTurn) {
  const beforeTurns = Array.isArray(beforeThread?.turns) ? beforeThread.turns : []
  const afterTurns = Array.isArray(afterThread?.turns) ? afterThread.turns : []
  if (beforeTurns.length <= 2 || afterTurns.length !== 3 || afterTurns[0] !== compactionTurn) {
    return false
  }
  return canonicalJSON(afterTurns.slice(1)) === canonicalJSON(beforeTurns.slice(-2))
}

function caseCompactionPublicProjectionWithheld(beforeThread, afterThread) {
  if (afterThread?.historyAuthority !== 'case_boundary_only_v1') return false
  const beforeIDs = new Set((Array.isArray(beforeThread?.turns) ? beforeThread.turns : [])
    .map((turn) => String(turn?.id || '')))
  return (Array.isArray(afterThread?.turns) ? afterThread.turns : []).some((turn) =>
    !beforeIDs.has(String(turn?.id || '')) &&
    /^turn_.+_compaction_[1-9][0-9]*$/u.test(String(turn?.id || '')) &&
    Array.isArray(turn?.items) && turn.items.length === 0 && turn?.status === 'completed'
  )
}

export function compactionTransitionEvidence(beforeThread, afterThread) {
  if (!beforeThread || !afterThread || beforeThread.id !== afterThread.id ||
      !trimmedNonempty(beforeThread.id)) {
    return { ok: false, blocker: 'compaction_thread_identity_invalid', compactions: [] }
  }
  const candidates = []
  for (const turn of Array.isArray(afterThread.turns) ? afterThread.turns : []) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      if (item?.kind !== 'compaction') continue
      candidates.push({ turn, item, contract: inspectCompactionItem(afterThread, turn, item) })
    }
  }
  if (candidates.length !== 1) {
    const caseWithheld = candidates.length === 0 &&
      caseCompactionPublicProjectionWithheld(beforeThread, afterThread)
    return {
      ok: false,
      blocker: caseWithheld
        ? 'case_compaction_public_attestation_unavailable'
        : candidates.length === 0
          ? 'compaction_not_observed'
          : 'compaction_candidate_count_invalid',
      schemaKind: caseWithheld ? 'case_public_projection_withheld' : 'invalid',
      compactions: []
    }
  }
  const candidate = candidates[0]
  if (!candidate.contract.ok) {
    return { ...candidate.contract, compactions: [] }
  }
  if (candidate.contract.schemaKind.startsWith('case_')) {
    return {
      ok: false,
      blocker: 'case_compaction_source_not_publicly_recomputable',
      schemaKind: candidate.contract.schemaKind,
      compactions: []
    }
  }
  const source = publicCompactionSourceItems(beforeThread)
  if (!source.ok) return { ...source, schemaKind: candidate.contract.schemaKind, compactions: [] }
  if (canonicalJSON(candidate.item.sourceItemIds) !== canonicalJSON(source.ids)) {
    return {
      ok: false,
      blocker: 'compaction_source_item_membership_mismatch',
      schemaKind: candidate.contract.schemaKind,
      compactions: []
    }
  }
  const sourceDigest = sha256(goCanonicalJSON(source.items))
  if (candidate.item.sourceDigest !== sourceDigest) {
    return {
      ok: false,
      blocker: 'compaction_source_digest_not_publicly_recomputable',
      schemaKind: candidate.contract.schemaKind,
      compactions: []
    }
  }
  const replacedTokens = Math.max(1, Math.floor(source.items.reduce(
    (total, item) => total + Buffer.byteLength(goCanonicalJSON(item)), 0
  ) / 4))
  if (candidate.item.replacedTokens !== replacedTokens) {
    return {
      ok: false,
      blocker: 'compaction_replaced_token_count_mismatch',
      schemaKind: candidate.contract.schemaKind,
      compactions: []
    }
  }
  if (!exactGeneralCompactionReplacement(beforeThread, afterThread, candidate.turn)) {
    return {
      ok: false,
      blocker: 'compaction_replacement_projection_mismatch',
      schemaKind: candidate.contract.schemaKind,
      compactions: []
    }
  }
  const sourceProjectionDigest = sha256(goCanonicalJSON(source.items))
  const replacementDigest = sha256(canonicalJSON({
    compactedTurn: candidate.turn,
    retainedTail: afterThread.turns.slice(1)
  }))
  const transition = {
    schemaKind: candidate.contract.schemaKind,
    threadId: afterThread.id,
    compactionItemId: candidate.item.id,
    sourceProjectionDigest,
    sourceItemIds: source.ids,
    replacementDigest
  }
  return {
    ok: true,
    blocker: '',
    schemaKind: candidate.contract.schemaKind,
    compactions: [candidate.item],
    sourceProjectionDigest,
    replacementDigest,
    beforeThreadDigest: sha256(canonicalJSON(beforeThread)),
    afterThreadDigest: sha256(canonicalJSON(afterThread)),
    transitionDigest: sha256(canonicalJSON(transition))
  }
}

export function compactionItems(thread, beforeThread) {
  if (!beforeThread) return []
  const evidence = compactionTransitionEvidence(beforeThread, thread)
  return evidence.ok ? evidence.compactions : []
}

export function exactCompactionRecoveryMatches(compactedEvidence, recoveredEvidence) {
  const compacted = compactedEvidence?.compactionEvidence
  const recovered = recoveredEvidence?.compactionEvidence
  return compacted?.ok === true && recovered?.ok === true &&
    compacted.schemaKind === recovered.schemaKind &&
    compacted.beforeThreadDigest === recovered.beforeThreadDigest &&
    compacted.afterThreadDigest === recovered.afterThreadDigest &&
    compacted.sourceProjectionDigest === recovered.sourceProjectionDigest &&
    compacted.replacementDigest === recovered.replacementDigest &&
    compacted.transitionDigest === recovered.transitionDigest &&
    compactedEvidence?.compactionCount === 1 && recoveredEvidence?.compactionCount === 1 &&
    compactedEvidence.compactionsDigest === recoveredEvidence.compactionsDigest &&
    isSHA256(compactedEvidence.compactionsDigest)
}

function terminalThread(thread) {
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  return turns.length > 0 && turns.every((turn) =>
    ['completed', 'failed', 'aborted'].includes(turn?.status)
  )
}

export function providerReceiptForTurn(observation, turn) {
  const provider = milestoneBLocalProvider(observation)
  const thread = observation?.thread
  const attempts = (Array.isArray(observation?.providerAttempts)
    ? observation.providerAttempts
    : []).filter((item) => item?.kind === 'usage' &&
      item?.threadId === thread?.id && item?.turnId === turn?.id)
  const terminals = (Array.isArray(observation?.providerTerminals)
    ? observation.providerTerminals
    : []).filter((item) => item?.kind === 'turn_completed' &&
      item?.threadId === thread?.id && item?.turnId === turn?.id)
  const attempt = attempts.length === 1 ? attempts[0] : null
  const statuses = attempt?.providerAttemptStatuses
  const counts = statuses && typeof statuses === 'object'
    ? [statuses.succeeded, statuses.failed, statuses.cancelled,
        statuses.timedOut, statuses.streamAborted]
    : []
  const attemptsTotal = counts.length === 5 &&
    counts.every((value) => Number.isSafeInteger(value) && value >= 0)
    ? counts.reduce((total, value) => total + value, 0)
    : -1
  const attemptValid = Boolean(attempt && provider.ok && attempt.model === provider.model &&
    (attempt.usageFinalStatus === undefined || attempt.usageFinalStatus === 'completed') &&
    Number.isSafeInteger(attempt.seq) && attempt.seq > 0 &&
    attempt.providerAttemptTelemetrySchema === 'provider-attempt-telemetry.v1' &&
    attempt.providerAttemptTelemetryValid === true &&
    Number.isSafeInteger(attempt.providerLogicalCallCount) &&
    attempt.providerLogicalCallCount > 0 && Number.isSafeInteger(attempt.providerAttemptCount) &&
    attempt.providerAttemptCount >= attempt.providerLogicalCallCount &&
    attemptsTotal === attempt.providerAttemptCount &&
    statuses?.succeeded === attempt.providerLogicalCallCount &&
    Number.isSafeInteger(attempt.promptTokens) && attempt.promptTokens > 0 &&
    Number.isSafeInteger(attempt.completionTokens) && attempt.completionTokens > 0 &&
    Number.isSafeInteger(attempt.totalTokens) &&
    attempt.totalTokens === attempt.promptTokens + attempt.completionTokens && attempt.turns === 1)
  const terminal = terminals.length === 1 ? terminals[0] : null
  const terminalValid = Boolean(attemptValid && terminal?.status === 'completed' &&
    Number.isSafeInteger(terminal.seq) && terminal.seq === attempt.seq + 1)
  return {
    ok: Boolean(provider.ok && thread?.providerId === provider.id &&
      thread?.model === provider.model && attemptValid && terminalValid &&
      turn?.status === 'completed'),
    digest: attemptValid && terminalValid ? sha256(canonicalJSON({ attempt, terminal })) : ''
  }
}

function publicTurnEvidence(observation, turn) {
  const toolExecutions = successfulToolExecutions(turn)
  const accepted = turn?.acceptedFinal || null
  const text = assistantText(turn)
  return {
    id: String(turn?.id || ''),
    status: String(turn?.status || ''),
    toolNames: toolExecutions.map((item) => item.toolName),
    toolExecutionDigest: sha256(canonicalJSON(toolExecutions)),
    assistantText: text,
    assistantTextDigest: text ? sha256(text) : '',
    acceptedFinal: accepted,
    acceptedFinalDigest: String(accepted?.recordDigest || ''),
    provider: providerReceiptForTurn(observation, turn)
  }
}

async function submitAndWait({
  debugPort, workspace, threadId = '', prompt, timeoutMs, requireMarker = ''
}) {
  let before = null
  try {
    before = await observeRenderer({ debugPort, workspace, exactThreadId: threadId, timeoutMs: 20_000 })
  } catch {
    before = null
  }
  const previousTurnCount = Array.isArray(before?.thread?.turns) ? before.thread.turns.length : 0
  const submitted = await cdpComposerSubmit(debugPort, prompt, [
    'Send message', 'Send', '发送消息', '发送'
  ])
  if (!submitted.ok) throw new Error(submitted.blocker)
  return waitForSubmittedTurn({
    debugPort,
    workspace,
    threadId,
    previousTurnCount,
    timeoutMs,
    requireMarker
  })
}

async function waitForSubmittedTurn({
  debugPort, workspace, threadId = '', previousTurnCount, timeoutMs, requireMarker = ''
}) {
  const deadline = Date.now() + timeoutMs
  let latest = null
  while (Date.now() < deadline) {
    try {
      latest = await observeRenderer({
        debugPort,
        workspace,
        exactThreadId: threadId,
        timeoutMs: Math.min(20_000, Math.max(1000, deadline - Date.now()))
      })
      const turns = Array.isArray(latest?.thread?.turns) ? latest.thread.turns : []
      if (turns.length > previousTurnCount && terminalThread(latest.thread)) {
        const turn = turns.at(-1)
        const evidence = publicTurnEvidence(latest, turn)
        if (submittedTurnObservationReady(turn, evidence, requireMarker)) {
          return {
            observation: latest,
            threadId: String(latest.thread?.id || ''),
            threadTitle: String(latest.thread?.title || ''),
            turn,
            evidence,
            previousTurnCount,
            turnCount: turns.length
          }
        }
      }
    } catch {
      // The public thread projection may briefly lag the renderer stream.
    }
    await sleep(750)
  }
  throw new Error(submittedTurnTimeoutBlocker(latest, previousTurnCount, requireMarker))
}

export function submittedTurnObservationReady(turn, evidence, requireMarker = '') {
  if (turn?.status === 'failed' || turn?.status === 'aborted') return true
  return turn?.status === 'completed' &&
    (!requireMarker || String(evidence?.assistantText || '').includes(requireMarker))
}

export function submittedTurnTimeoutBlocker(
  observation,
  previousTurnCount = 0,
  requireMarker = ''
) {
  const turns = Array.isArray(observation?.thread?.turns)
    ? observation.thread.turns
    : []
  if (turns.length <= previousTurnCount) return 'packaged_turn_not_created'
  const turn = turns.at(-1)
  if (turn?.status === 'failed') {
    const reasonCode = publicTurnFailureReasonCode(turn, observation)
    return reasonCode ? `packaged_turn_failed_${reasonCode}` : 'packaged_turn_failed'
  }
  if (turn?.status === 'aborted') return 'packaged_turn_aborted'
  if (turn?.status === 'completed') {
    return requireMarker && !assistantText(turn).includes(requireMarker)
      ? 'packaged_turn_required_marker_not_observed'
      : 'packaged_turn_terminal_projection_not_observed'
  }
  return 'packaged_turn_completion_timeout'
}

export function publicTurnFailureReasonCode(turn, observation = null) {
  const itemValues = (Array.isArray(turn?.items) ? turn.items : [])
    .filter((item) => item?.kind === 'error' && item?.status === 'failed')
    .flatMap((item) => [item?.code, item?.details?.reasonCode])
  const eventValues = (Array.isArray(observation?.turnFailures)
    ? observation.turnFailures
    : [])
    .filter((item) => item?.threadId === turn?.threadId && item?.turnId === turn?.id)
    .map((item) => item?.reasonCode)
  const values = new Set([...itemValues, ...eventValues]
    .filter((value) => SAFE_TURN_FAILURE_REASON_CODES.has(value)))
  return values.size === 1 ? [...values][0] : ''
}

export function publicTurnFailureProjection(turn, observation = null) {
  const itemCodes = [...new Set((Array.isArray(turn?.items) ? turn.items : [])
    .filter((item) => item?.kind === 'error' && item?.status === 'failed')
    .flatMap((item) => [item?.code, item?.details?.reasonCode])
    .filter((value) => SAFE_TURN_FAILURE_REASON_CODES.has(value)))]
    .sort()
  const events = (Array.isArray(observation?.turnFailures)
    ? observation.turnFailures
    : [])
    .filter((item) => item?.threadId === turn?.threadId && item?.turnId === turn?.id &&
      SAFE_TURN_FAILURE_REASON_CODES.has(item?.reasonCode) &&
      (item?.kind === 'turn_failed' ||
        (item?.kind === 'pipeline_stage' && item?.stage === 'provider_error')))
    .map((item) => ({
      kind: item.kind,
      stage: item.kind === 'pipeline_stage' ? 'provider_error' : '',
      seq: Number.isSafeInteger(item.seq) && item.seq > 0 ? item.seq : 0,
      reasonCode: item.reasonCode
    }))
  const turnStatus = ['completed', 'failed', 'aborted'].includes(turn?.status)
    ? turn.status
    : 'unknown'
  const reasonCodes = new Set([
    ...itemCodes,
    ...events.map((item) => item.reasonCode)
  ])
  return Object.freeze({
    turnStatus,
    reasonCode: reasonCodes.size === 1 ? [...reasonCodes][0] : '',
    itemCodes,
    events
  })
}

export function cleanupOptionalHarnessResource(resource, cleanup) {
  return resource ? cleanup(resource) === true : true
}

export function fundsOnlyTurnLifecycleEvidence(turn) {
  const items = Array.isArray(turn?.items) ? turn.items : []
  const indexed = items.map((item, index) => ({ item, index }))
  const users = indexed.filter(({ item }) => item?.kind === 'user_message' &&
    item.status === 'completed' && typeof item.id === 'string' && item.id)
  const calls = indexed.filter(({ item }) => item?.kind === 'tool_call')
  const results = indexed.filter(({ item }) => item?.kind === 'tool_result')
  const approvals = indexed.filter(({ item }) => item?.kind === 'approval')
  const call = calls[0]?.item
  const result = results[0]?.item
  const execution = successfulToolExecutions(turn)
  const identityValid = typeof turn?.id === 'string' && turn.id &&
    typeof turn?.threadId === 'string' && turn.threadId &&
    items.every((item) => item?.turnId === turn.id && item?.threadId === turn.threadId)
  const ok = identityValid && users.length === 1 && calls.length === 1 &&
    results.length === 1 && approvals.length === 0 &&
    call?.toolName === ACCOUNT_FLOW_TOOL && call?.status === 'completed' &&
    typeof call?.callId === 'string' && call.callId && call?.toolKind === 'tool_call' &&
    result?.toolName === ACCOUNT_FLOW_TOOL && result?.status === 'completed' &&
    result?.callId === call.callId && result?.toolKind === call.toolKind &&
    result?.isError === false && result?.output?.status === 'completed' &&
    result?.output?.projectionKind === 'case_source_status' &&
    result?.output?.messageKey === 'case_source_private' &&
    calls[0].index < results[0].index && execution.length === 1 &&
    execution[0].toolName === ACCOUNT_FLOW_TOOL && execution[0].callId === call.callId
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'funds_turn_exact_tool_lifecycle_invalid',
    turnKey: users.length === 1 ? users[0].item.id : '',
    toolExecutionCount: execution.length
  })
}

function expectedFundsAnswerFragments(expected) {
  const currency = expected?.currency
  return [
    '资金汇总：',
    `流入 ${expected?.inflowMinor} ${currency}`,
    `流出 ${expected?.outflowMinor} ${currency}`,
    `有符号净额 ${expected?.netMinor} ${currency}`,
    `交易笔数 ${expected?.transactionCount}`,
    '（限定范围：'
  ]
}

export function exactFundsFinalEvidence(result, snapshot) {
  const accepted = result?.evidence?.acceptedFinal
  const publicView = accepted?.publicView
  const admission = accepted?.factFinalWitnessAdmission
  const receiptMetadata = publicView?.receiptMetadata
  const claimTypes = Array.isArray(publicView?.claimTypes) ? publicView.claimTypes : []
  const text = result?.evidence?.assistantText || ''
  const expected = snapshot?.expected || {}
  const requiredText = expectedFundsAnswerFragments(expected)
  const subjectMatch = text.match(/主体\s+([^\n。]+?)\s+的/u)
  const exactToolCount = result.evidence.toolNames.filter((name) => name === ACCOUNT_FLOW_TOOL).length
  const requiredClaimTypes = ['amount', 'count']
  const exactClaimVocabulary = claimTypes.length === requiredClaimTypes.length &&
    requiredClaimTypes.every((value, index) => claimTypes[index] === value)
  const hostPublicContractValidated = privatePublicRuntimeTurnBindingMatches(result)
  const lifecycle = fundsOnlyTurnLifecycleEvidence(result?.turn)
  const ok = hostPublicContractValidated && lifecycle.ok && result.evidence.status === 'completed' &&
    result.evidence.provider.ok &&
    exactToolCount === 1 && accepted?.schemaVersion === 5 &&
    accepted?.authorityPurpose === 'analytix.case-final/v1' &&
    accepted?.authorityAlgorithm === 'Ed25519' && isSHA256(accepted?.authorityKeyId) &&
    typeof accepted?.authorityPublicKey === 'string' && accepted.authorityPublicKey.length > 0 &&
    typeof accepted?.authoritySignature === 'string' && accepted.authoritySignature.length > 0 &&
    accepted?.threadId === result?.threadId && accepted?.turnId === result?.turn?.id &&
    isSHA256(accepted?.envelopeDigest) && isSHA256(accepted?.contextDigest) &&
    Number.isSafeInteger(accepted?.contextEpoch) && accepted.contextEpoch > 0 &&
    /^dsv2_[0-9a-f]{64}$/u.test(String(accepted?.datasetSnapshotId || '')) &&
    Number.isSafeInteger(accepted?.registrySequence) && accepted.registrySequence > 0 &&
    isSHA256(accepted?.registryStateDigest) &&
    accepted?.variant === 'EvidenceBackedAnswer' && accepted?.terminalReason === 'success' &&
    accepted?.finalGateVersion === 'analytix.final-evidence-gate/v4' &&
    accepted?.verifierVersion === 'analytix.claim-verifier-policy/v1' &&
    accepted?.rendererVersion === 'analytix.host-final-renderer/v2' &&
    isSHA256(accepted?.privateRecordDigest) && isSHA256(accepted?.publicViewDigest) &&
    isSHA256(accepted?.publicationSnapshotProofDigest) &&
    isSHA256(accepted?.recordDigest) && accepted?.renderedTextSha256 === sha256(text) &&
    result?.evidence?.assistantTextDigest === accepted.renderedTextSha256 &&
    isSHA256(result?.evidence?.toolExecutionDigest) &&
    isSHA256(result?.evidence?.provider?.digest) &&
    admission?.schemaVersion === 1 &&
    admission?.purpose === 'analytix.fact-final-witness-admission/v1' &&
    admission?.contextDigest === accepted.contextDigest &&
    admission?.datasetSnapshotId === accepted.datasetSnapshotId &&
    admission?.renderedTextSha256 === accepted.renderedTextSha256 &&
    admission?.publicationSnapshotProofDigest === accepted.publicationSnapshotProofDigest &&
    admission?.registrySequence === accepted.registrySequence &&
    admission?.registryStateDigest === accepted.registryStateDigest &&
    isSHA256(admission?.sourceManifestHash) && isSHA256(admission?.envelopeDigest) &&
    isSHA256(admission?.evidenceReceiptIdsDigest) && isSHA256(admission?.admissionDigest) &&
    Number.isSafeInteger(admission?.evidenceReceiptCount) && admission.evidenceReceiptCount > 0 &&
    publicView?.schemaVersion === 2 && publicView?.publicationState === 'accepted' &&
    publicView?.envelopeDigest === accepted.envelopeDigest &&
    publicView?.contextDigest === accepted.contextDigest &&
    publicView?.contextEpoch === accepted.contextEpoch &&
    publicView?.datasetSnapshotId === accepted.datasetSnapshotId &&
    publicView?.variant === 'EvidenceBackedAnswer' && publicView?.coverageStatus === 'complete' &&
    publicView?.terminalReason === 'success' && publicView?.blockerCode === '' &&
    publicView?.missingScopeCount === 0 && publicView?.noHitWording === '' &&
    isSHA256(publicView?.checkedScopeDigest) &&
    Number.isSafeInteger(publicView?.claimCount) && publicView.claimCount > 0 &&
    exactClaimVocabulary &&
    receiptMetadata?.projection === 'masked_metadata_only' &&
    receiptMetadata?.count === admission.evidenceReceiptCount && receiptMetadata.count > 0 &&
    isSHA256(receiptMetadata?.setDigest) &&
    Array.isArray(receiptMetadata?.citations) &&
    receiptMetadata.citations.length === receiptMetadata.count &&
    requiredText.every((value) => text.includes(value)) &&
    Boolean(subjectMatch?.[1]) && !/\bcer1_[a-z0-9_-]+\b/iu.test(text)
  return {
    ok,
    lifecycleBound: lifecycle.ok,
    lifecycleBlocker: lifecycle.blocker,
    hostPublicContractValidated,
    acceptedFinalDigest: String(accepted?.recordDigest || ''),
    contextEpoch: Number(accepted?.contextEpoch || 0),
    datasetSnapshotIdHash: accepted?.datasetSnapshotId ? sha256(accepted.datasetSnapshotId) : '',
    datasetSnapshotId: String(accepted?.datasetSnapshotId || ''),
    claimCount: Number(publicView?.claimCount || 0),
    claimTypes,
    evidenceReceiptCount: Number(accepted?.factFinalWitnessAdmission?.evidenceReceiptCount || 0),
    evidenceReceiptSetDigest: String(receiptMetadata?.setDigest || ''),
    subjectDisplayLabelHash: subjectMatch?.[1] ? sha256(subjectMatch[1]) : '',
    subjectDisplayLabel: subjectMatch?.[1] || '',
    toolExecutionDigest: result?.evidence?.toolExecutionDigest || '',
    providerReceiptDigest: result?.evidence?.provider?.digest || '',
    assistantTextDigest: result?.evidence?.assistantTextDigest || ''
  }
}

async function waitForBodyTextState(debugPort, expected, present, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const target = await waitForDebugTarget(debugPort, Math.min(5000, deadline - Date.now()))
      const value = await evaluateReadonlyCdp(
        target.webSocketDebuggerUrl,
        `String(document.body?.innerText || '').includes(${JSON.stringify(expected)})`,
        Math.min(5000, deadline - Date.now())
      )
      if (value === present) return true
    } catch {
      // Snapshot acceptance restarts the Go sidecar and may briefly reload the renderer.
    }
    await sleep(300)
  }
  return false
}

async function waitForBodyText(debugPort, expected, timeoutMs) {
  return waitForBodyTextState(debugPort, expected, true, timeoutMs)
}

async function waitForAnyBodyTextState(debugPort, expectedValues, present, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const target = await waitForDebugTarget(
        debugPort,
        Math.min(5000, Math.max(250, deadline - Date.now()))
      )
      const value = await evaluateReadonlyCdp(
        target.webSocketDebuggerUrl,
        `(() => {
          const values = ${JSON.stringify(expectedValues)};
          const text = String(document.body?.innerText || '');
          return values.some((value) => text.includes(value));
        })()`,
        Math.min(5000, Math.max(250, deadline - Date.now()))
      )
      if (value === present) return true
    } catch {
      // Snapshot acceptance may briefly reload the renderer.
    }
    await sleep(200)
  }
  return false
}

function nativeSelectFile(path) {
  const script = `
set csvPath to ${JSON.stringify(path)}
tell application "System Events"
  delay 0.4
  keystroke "g" using {command down, shift down}
  delay 0.4
  keystroke csvPath
  delay 0.4
  key code 36
  delay 0.7
  key code 36
end tell
`
  const result = spawnSync('/usr/bin/osascript', ['-'], {
    cwd: process.cwd(),
    input: script,
    encoding: 'utf8',
    stdio: ['pipe', 'pipe', 'pipe'],
    timeout: 30_000
  })
  return {
    ok: result.status === 0,
    blocker: result.status === 0 ? '' : 'native_file_chooser_accessibility_or_selection_failed'
  }
}

async function openDataImportDialog(debugPort) {
  let importEntry = await clickVisibleByLabels(debugPort, [
    'Open data import entry', '打开数据导入入口', 'Data import', '数据导入'
  ], 5000)
  if (!importEntry.ok) {
    const actionsMenu = await clickVisibleByLabels(debugPort, [
      'Chat actions', '会话操作'
    ], 10_000)
    if (!actionsMenu.ok) return { ok: false, blocked: false, blocker: actionsMenu.blocker }
    await sleep(300)
    importEntry = await clickVisibleByLabels(debugPort, [
      'Open data import entry', '打开数据导入入口', 'Data import', '数据导入'
    ], 10_000)
  }
  return importEntry.ok
    ? { ok: true, blocked: false, blocker: '' }
    : { ok: false, blocked: false, blocker: importEntry.blocker }
}

async function stageSnapshotThroughNativeUI({ debugPort, sourcePath, timeoutMs }) {
  const importEntry = await openDataImportDialog(debugPort)
  if (!importEntry.ok) return importEntry
  const stageButton = await clickVisibleByLabels(debugPort, [
    'Select CSV and create snapshot', '选择 CSV 并建立快照'
  ], 20_000)
  if (!stageButton.ok) return { ok: false, blocked: false, blocker: stageButton.blocker }
  const successMessages = [
    'The Go host created, startup-validated, and selected the immutable data snapshot as the current case evidence source.',
    '不可变数据快照已由 Go 主机建立、启动校验并切换为当前案件证据源。'
  ]
  const priorSuccessCleared = await waitForAnyBodyTextState(
    debugPort,
    successMessages,
    false,
    5000
  )
  if (!priorSuccessCleared) {
    return { ok: false, blocked: false, blocker: 'native_snapshot_staging_prior_success_not_cleared' }
  }
  const selected = nativeSelectFile(sourcePath)
  if (!selected.ok) return { ok: false, blocked: true, blocker: selected.blocker }
  const success = await waitForAnyBodyTextState(
    debugPort,
    successMessages,
    true,
    timeoutMs
  )
  return {
    ok: success,
    priorSuccessCleared,
    blocked: false,
    blocker: success ? '' : 'native_snapshot_staging_success_not_observed'
  }
}

async function reopenThreadInRenderer(debugPort, threadTitle, timeoutMs = 20_000) {
  if (!threadTitle) return { ok: false, blocker: 'thread_title_unavailable' }
  const clicked = await clickVisibleByLabels(debugPort, [threadTitle], timeoutMs)
  if (!clicked.ok) return clicked
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const target = await waitForDebugTarget(debugPort, 5000)
      const ready = await evaluateReadonlyCdp(
        target.webSocketDebuggerUrl,
        `!!document.querySelector('.ProseMirror[contenteditable="true"]')`,
        5000
      )
      if (ready) return { ok: true, blocker: '' }
    } catch {
      // Navigation is still settling.
    }
    await sleep(200)
  }
  return { ok: false, blocker: 'thread_renderer_not_reopened' }
}

async function forkCurrentThreadInRenderer(debugPort, workspace, parentThreadId, timeoutMs) {
  const menu = await clickVisibleByLabels(debugPort, ['Chat actions', '会话操作'], 20_000)
  if (!menu.ok) return { ok: false, blocker: menu.blocker }
  const forked = await clickVisibleByLabels(
    debugPort,
    ['Fork current chat', '从当前对话分支'],
    20_000
  )
  if (!forked.ok) return { ok: false, blocker: forked.blocker }
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const observation = await observeRenderer({
        debugPort,
        workspace,
        timeoutMs: Math.min(20_000, Math.max(1000, deadline - Date.now()))
      })
      if (observation?.thread?.id && observation.thread.id !== parentThreadId &&
          observation.thread.forkedFromThreadId === parentThreadId &&
          terminalThread(observation.thread)) {
        return { ok: true, blocker: '', observation, threadId: observation.thread.id }
      }
    } catch {
      // Fork persistence and renderer selection may settle independently.
    }
    await sleep(250)
  }
  return { ok: false, blocker: 'forked_thread_not_observed' }
}

async function cdpNormalQuit(debugPort) {
  try {
    const target = await waitForDebugTarget(debugPort, 20_000)
    await dispatchCdpCommands(target.webSocketDebuggerUrl, [
      { method: 'Input.dispatchKeyEvent', params: { type: 'rawKeyDown', key: 'q', code: 'KeyQ', modifiers: 4 } },
      { method: 'Input.dispatchKeyEvent', params: { type: 'keyUp', key: 'q', code: 'KeyQ', modifiers: 4 } }
    ], 20_000)
    return { ok: true, blocker: '' }
  } catch (error) {
    return {
      ok: false,
      blocker: error instanceof Error ? error.message : 'normal_app_quit_input_failed'
    }
  }
}

function launchPackagedApp({ appPath, target, debugPort, childEnv }) {
  const command = diagnosticUnpackaged ? require('electron') : executablePath(appPath, target)
  const commandArguments = diagnosticUnpackaged
    ? [`--remote-debugging-port=${debugPort}`, process.cwd()]
    : [`--remote-debugging-port=${debugPort}`]
  const child = spawn(command, commandArguments, {
    cwd: process.cwd(),
    env: childEnv,
    stdio: ['ignore', 'ignore', 'ignore']
  })
  child.analytixExit = new Promise((resolvePromise) => {
    child.once('exit', (code, signal) => resolvePromise({ code, signal }))
  })
  child.analytixLaunchIdentity = processIdentity(child.pid)
  return child
}

async function establishLaunchIdentity(child, timeoutMs = 5000) {
  const deadline = Date.now() + timeoutMs
  let previous = null
  while (Date.now() < deadline) {
    const identity = processIdentity(child?.pid)
    if (identity && previous && identity.pid === previous.pid &&
        identity.startTime === previous.startTime &&
        identity.commandDigest === previous.commandDigest &&
        identity.executableSetDigest === previous.executableSetDigest) {
      child.analytixLaunchIdentity = identity
      return identity
    }
    if (!processExists(child?.pid)) return null
    previous = identity
    await sleep(100)
  }
  return null
}

async function waitForExit(child, timeoutMs) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return { exited: true, code: child.exitCode, signal: child.signalCode }
  }
  const timeout = new Promise((resolvePromise) => {
    setTimeout(() => resolvePromise({ exited: false, code: null, signal: null }), timeoutMs)
  })
  return Promise.race([
    child.analytixExit.then((result) => ({ exited: true, ...result })),
    timeout
  ])
}

function processExists(pid) {
  if (!Number.isInteger(pid) || pid <= 0) return false
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

function descendantPids(rootPid) {
  if (!Number.isInteger(rootPid) || rootPid <= 0 || process.platform === 'win32') return []
  const result = spawnSync('/bin/ps', ['-axo', 'pid=,ppid='], {
    cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe'
  })
  if (result.status !== 0) return []
  const children = new Map()
  for (const line of String(result.stdout || '').split(/\r?\n/u)) {
    const match = line.trim().match(/^(\d+)\s+(\d+)$/u)
    if (!match) continue
    const values = children.get(Number(match[2])) || []
    values.push(Number(match[1]))
    children.set(Number(match[2]), values)
  }
  const found = []
  const pending = [...(children.get(rootPid) || [])]
  while (pending.length > 0) {
    const pid = pending.shift()
    if (!pid || found.includes(pid)) continue
    found.push(pid)
    pending.push(...(children.get(pid) || []))
  }
  return found
}

function processIdentity(pid) {
  if (!Number.isInteger(pid) || pid <= 0 || !processExists(pid)) return null
  const started = spawnSync('/bin/ps', ['-p', String(pid), '-o', 'lstart='], {
    cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe'
  })
  const command = spawnSync('/bin/ps', ['-p', String(pid), '-o', 'command='], {
    cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe'
  })
  const executable = spawnSync('/usr/sbin/lsof', [
    '-a', '-p', String(pid), '-d', 'txt', '-Fn'
  ], { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' })
  const startTime = String(started.stdout || '').trim()
  const commandText = String(command.stdout || '').trim()
  const executablePaths = String(executable.stdout || '').split(/\r?\n/u)
    .filter((line) => line.startsWith('n/'))
    .map((line) => line.slice(1))
    .sort()
  if (started.status !== 0 || command.status !== 0 || executable.status !== 0 ||
      !startTime || !commandText || executablePaths.length === 0) return null
  return Object.freeze({
    pid,
    startTime,
    commandDigest: sha256(commandText),
    executableSetDigest: sha256(canonicalJSON(executablePaths))
  })
}

function processIdentityMatches(identity) {
  const current = processIdentity(identity?.pid)
  return Boolean(current && identity && current.pid === identity.pid &&
    current.startTime === identity.startTime &&
    current.commandDigest === identity.commandDigest &&
    current.executableSetDigest === identity.executableSetDigest)
}

function refreshTaskOwnedChildIdentity(child) {
  const pinned = child?.analytixLaunchIdentity
  const current = processIdentity(child?.pid)
  if (!pinned || !current || current.pid !== pinned.pid ||
      current.startTime !== pinned.startTime) {
    return null
  }
  child.analytixLaunchIdentity = current
  return current
}

function captureExactTaskOwnedProcessTree(child) {
  const launchIdentity = refreshTaskOwnedChildIdentity(child)
  if (!launchIdentity || !processIdentityMatches(launchIdentity)) return []
  const descendants = descendantPids(child.pid)
  const identities = descendants.map((pid) => processIdentity(pid)).filter(Boolean)
  // Revalidate both root and ancestry after identity capture. A descendant
  // that exited or was reparented between samples is conservatively omitted;
  // its bare PID is never authorized for a signal.
  if (!processIdentityMatches(launchIdentity)) return []
  const confirmedDescendants = new Set(descendantPids(child.pid))
  if (!processIdentityMatches(launchIdentity)) return []
  return [
    ...identities.filter((identity) =>
      confirmedDescendants.has(identity.pid) && processIdentityMatches(identity)
    ),
    launchIdentity
  ]
}

async function stopExactProcessIdentities(exactIdentities) {
  for (const signal of ['SIGTERM', 'SIGKILL']) {
    for (const identity of [...exactIdentities].reverse()) {
      if (!processIdentityMatches(identity)) continue
      try {
        process.kill(identity.pid, signal)
      } catch {
        // It exited between the final identity sample and the exact signal.
      }
    }
    await sleep(signal === 'SIGTERM' ? 750 : 250)
    if (exactIdentities.every((identity) => !processIdentityMatches(identity))) break
  }
  return exactIdentities
}

async function waitForExactProcessIdentitiesToExit(identities, timeoutMs = 5000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (identities.every((identity) => !processIdentityMatches(identity))) return true
    await sleep(100)
  }
  return identities.every((identity) => !processIdentityMatches(identity))
}

async function stopExactTaskOwnedChild(child) {
  return stopExactProcessIdentities(captureExactTaskOwnedProcessTree(child))
}

function listeningPids(port) {
  const result = spawnSync('/usr/sbin/lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN'], {
    cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe'
  })
  if (result.status !== 0) return []
  return String(result.stdout || '').split(/\r?\n/u)
    .map((item) => Number(item.trim()))
    .filter((pid) => Number.isInteger(pid) && pid > 0)
}

function exactRuntimeProcessEvidence(appPid, runtimePort, expectedRuntimePath) {
  const descendants = descendantPids(appPid)
  const listeners = listeningPids(runtimePort)
  if (listeners.length !== 1 || !descendants.includes(listeners[0])) {
    return { ok: false, listenerCount: listeners.length, exactPackagedRuntimeExecutable: false }
  }
  const result = spawnSync('/usr/sbin/lsof', [
    '-a', '-p', String(listeners[0]), '-d', 'txt', '-Fn'
  ], { cwd: process.cwd(), encoding: 'utf8', stdio: 'pipe' })
  let expected = ''
  try {
    expected = realpathSync(expectedRuntimePath)
  } catch {
    expected = ''
  }
  const exact = result.status === 0 && String(result.stdout || '').split(/\r?\n/u)
    .filter((line) => line.startsWith('n/'))
    .map((line) => line.slice(1))
    .some((path) => {
      try { return realpathSync(path) === expected } catch { return false }
    })
  return { ok: exact, listenerCount: listeners.length, exactPackagedRuntimeExecutable: exact }
}

function decodedProtectedScanText(value) {
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  return text
    .replace(/&#x([0-9a-f]{1,6});?/giu, (match, digits) => {
      const codePoint = Number.parseInt(digits, 16)
      return Number.isSafeInteger(codePoint) && codePoint <= 0x10ffff
        ? String.fromCodePoint(codePoint)
        : match
    })
    .replace(/&#([0-9]{1,7});?/gu, (match, digits) => {
      const codePoint = Number.parseInt(digits, 10)
      return Number.isSafeInteger(codePoint) && codePoint <= 0x10ffff
        ? String.fromCodePoint(codePoint)
        : match
    })
    .replace(/\\u\{?([0-9a-f]{4,6})\}?/giu, (match, digits) => {
      const codePoint = Number.parseInt(digits, 16)
      return Number.isSafeInteger(codePoint) && codePoint <= 0x10ffff
        ? String.fromCodePoint(codePoint)
        : match
    })
    .replace(/%([0-9a-f]{2})/giu, (match, digits) => {
      const codePoint = Number.parseInt(digits, 16)
      return codePoint <= 0x7f ? String.fromCodePoint(codePoint) : match
    })
}

function scanStringForProtectedValues(value, protectedValues) {
  const text = decodedProtectedScanText(value)
  const digitStream = text
    .replace(/<[^>]{0,512}>/gu, '')
    .replace(/[ \t\u00a0\u2007\u202f-]/gu, '')
  return protectedValues.filter((needle) => {
    if (!needle) return false
    const decodedNeedle = decodedProtectedScanText(needle)
    if (text.includes(decodedNeedle)) return true
    const canonicalDigits = decodedNeedle.replace(/[ \t\u00a0\u2007\u202f-]/gu, '')
    return /^\d{8,32}$/u.test(canonicalDigits) && digitStream.includes(canonicalDigits)
  }).length
}

function scanRegularFilesForProtectedValues(roots, protectedValues) {
  const pending = roots.filter((path) => existsSync(path))
  let scannedFileCount = 0
  let findingCount = 0
  let internalReferenceFindingCount = 0
  let unsafeEntryCount = 0
  while (pending.length > 0) {
    const path = pending.pop()
    let stat
    try {
      stat = lstatSync(path)
    } catch {
      unsafeEntryCount += 1
      continue
    }
    if (stat.isSymbolicLink()) {
      unsafeEntryCount += 1
      continue
    }
    if (stat.isDirectory()) {
      try {
        for (const entry of readdirSync(path)) pending.push(join(path, entry))
      } catch {
        unsafeEntryCount += 1
      }
      continue
    }
    if (!stat.isFile()) continue
    if (stat.size > MAX_PUBLIC_SURFACE_BYTES) {
      unsafeEntryCount += 1
      continue
    }
    scannedFileCount += 1
    const file = hashRegularFile(path, { capture: true, maximumBytes: MAX_PUBLIC_SURFACE_BYTES })
    if (!file.regular || !file.content) {
      unsafeEntryCount += 1
      continue
    }
    const fileText = file.content.toString('utf8')
    findingCount += scanStringForProtectedValues(fileText, protectedValues)
    internalReferenceFindingCount += fileText.match(/\bcer1_[a-z0-9_-]+\b/giu)?.length || 0
  }
  return { scannedFileCount, findingCount, internalReferenceFindingCount, unsafeEntryCount }
}

export function publicPIIScanEvidence(observations, protectedValues, logRoots) {
  const surfaces = observations.map((observation) => ({
    thread: observation?.thread,
    summary: observation?.summary,
    sseBody: observation?.sseBody,
    bodyText: observation?.bodyText,
    bodyHTML: observation?.bodyHTML
  }))
  const publicFindingCount = surfaces.reduce((total, surface) =>
    total + scanStringForProtectedValues(surface, protectedValues), 0)
  const publicInternalReferenceFindingCount = surfaces.reduce((total, surface) =>
    total + (JSON.stringify(surface).match(/\bcer1_[a-z0-9_-]+\b/giu)?.length || 0), 0)
  const incompletePublicSurfaceCount = observations.filter((observation) =>
    observation?.bodyTextComplete !== true ||
    observation?.bodyHTMLComplete !== true ||
    observation?.sseBodyComplete !== true
  ).length
  const logs = scanRegularFilesForProtectedValues(logRoots, protectedValues)
  const forbiddenGenericProviderChannels = {
    surfaceCount: surfaces.length,
    completePIIFindingCount: publicFindingCount + logs.findingCount,
    internalReferenceFindingCount:
      publicInternalReferenceFindingCount + logs.internalReferenceFindingCount,
    incompleteSurfaceCount: incompletePublicSurfaceCount,
    unsafeLogEntryCount: logs.unsafeEntryCount
  }
  return {
    ok: incompletePublicSurfaceCount === 0 && publicFindingCount === 0 &&
      publicInternalReferenceFindingCount === 0 && logs.findingCount === 0 &&
      logs.internalReferenceFindingCount === 0 && logs.unsafeEntryCount === 0,
    forbiddenGenericProviderChannels,
    allowedTypedLocalSink: {
      excludedFromForbiddenScan: true,
      sourceExactFieldsAllowed: true,
      scanBoundary: 'typed-local-display-only'
    },
    publicSurfaceCount: surfaces.length,
    incompletePublicSurfaceCount,
    publicFindingCount,
    publicInternalReferenceFindingCount,
    logScannedFileCount: logs.scannedFileCount,
    logFindingCount: logs.findingCount,
    logInternalReferenceFindingCount: logs.internalReferenceFindingCount,
    logUnsafeEntryCount: logs.unsafeEntryCount
  }
}

export async function scanMilestoneBLocalCredentials({
  source, runId, entryAttemptId, provider, runtimeDataDir,
  sandboxRoot, productRunRoot, settingsPath, reportSnapshot
}) {
  const sourceEvidence = readLocalCredentialScanSource(source, { runId, entryAttemptId, provider })
  const store = localProviderSecretStoreEvidence(runtimeDataDir)
  const scans = []
  for (const root of [sandboxRoot, productRunRoot]) {
    scans.push(await scanLocalCredentialIsolation({
      source, runId, entryAttemptId, provider, root,
      settingsPath: root === productRunRoot ? settingsPath : '', reportSnapshot
    }))
  }
  const failed = scans.find((scan) => scan.status === 'failed')
  const blocked = scans.find((scan) => scan.status !== 'passed')
  const ok = sourceEvidence.ok && store.ok && scans.every((scan) => scan.status === 'passed')
  const counters = Object.fromEntries([
    'scannedFileCount', 'findingCount', 'settingsFindingCount', 'reportFindingCount',
    'unsafeEntryCount', 'symlinkCount'
  ].map((key) => [key, scans.reduce((total, scan) => total + (scan[key] || 0), 0)]))
  return {
    ...sourceEvidence,
    ...counters,
    ok,
    blocked: !ok && !failed,
    status: ok ? 'passed' : failed ? 'failed' : 'blocked',
    blocker: ok ? null : (failed ? 'local_provider_credential_exposure_detected' : '') || sourceEvidence.blocker ||
      (store.ok ? '' : 'local_provider_secret_store_not_owner_private') || blocked?.blocker ||
      'local_provider_credential_isolation_scan_not_completed',
    protectedStoreOwnerPrivate: store.protectedStoreOwnerPrivate,
    encodingCoverage: scans[0]?.encodingCoverage || []
  }
}

function checkStatus(id, status, message) {
  return { id, status, message }
}

function setCheck(report, id, status, message) {
  const index = report.checks.findIndex((item) => item.id === id)
  const value = checkStatus(id, status, message)
  if (index >= 0) report.checks[index] = value
  else report.checks.push(value)
}

function safeDiagnosticCode(value, fallback) {
  const code = String(value || '')
  return /^[a-z0-9_]+$/u.test(code) ? code : fallback
}

export function attributeMilestoneBActiveFailure(report, activeOwner, reasonCode) {
  const phase = safeDiagnosticCode(activeOwner?.phase, 'runtime_verification')
  const candidateCheckId = String(activeOwner?.checkId || '')
  const ownedCheck = /^[a-z0-9-]+$/u.test(candidateCheckId) &&
    report.checks.some((item) => item.id === candidateCheckId)
    ? candidateCheckId
    : ''
  const safeReasonCode = safeDiagnosticCode(
    reasonCode,
    'packaged_milestone_b_runtime_verification_failed'
  )
  report.runtimeBlocker = safeReasonCode
  report.runtimeFailure = {
    phase,
    checkId: ownedCheck,
    reasonCode: safeReasonCode
  }
  if (ownedCheck) setCheck(report, ownedCheck, 'FAIL', safeReasonCode)
  return report.runtimeFailure
}

export function providerAuditDependentEvidence({
  auditReceipt,
  observedProviderRequestCount,
  minimumProviderSafeSemanticBlockCount,
  upstreamReasonCode
}) {
  const observed = Number.isSafeInteger(observedProviderRequestCount) &&
    observedProviderRequestCount > 0
    ? observedProviderRequestCount
    : 0
  if (observed === 0) {
    const upstream = safeDiagnosticCode(upstreamReasonCode, '')
    const blocker = upstream
      ? `provider_audit_blocked_by_${upstream}`
      : 'provider_request_not_observed'
    const blocked = { ok: false, blocked: true, blocker }
    return Object.freeze({ payload: blocked, semantic: blocked })
  }
  const payload = auditReceipt?.ok === true
    ? auditReceipt
    : {
        ok: false,
        blocker: safeDiagnosticCode(
          auditReceipt?.blocker,
          'provider_audit_receipt_invalid'
        )
      }
  const semanticOK = auditReceipt?.ok === true &&
    auditReceipt.providerSafeSemanticValidationCount >= minimumProviderSafeSemanticBlockCount
  return Object.freeze({
    payload,
    semantic: {
      ok: semanticOK,
      blocker: semanticOK
        ? ''
        : safeDiagnosticCode(
            auditReceipt?.blocker,
            'provider_safe_semantic_context_not_completely_validated'
          )
    }
  })
}

function configuredTypedLocalDisplayStatus() {
  return {
    required: true,
    status: 'UNVERIFIED',
    contract: 'analytix.typed-local-display.v1',
    fullModeExactValue: false,
    maskedModeLocalOnly: false,
    acceptedSlotBindingVerified: false,
    ordinaryRendererCompletePIIAbsent: false,
    forbiddenGenericProviderChannelsZero: false,
    invalidatedAfterAuthorityChange: false,
    reason: 'typed_local_display_public_seam_not_exercised',
    blocksMilestoneB: true
  }
}

function configuredDirectSourcePreviewStatus() {
  return {
    required: true,
    status: 'UNVERIFIED',
    contract: 'analytix.direct-source-preview.v1',
    fullModeExactValue: false,
    maskedModeLocalOnly: false,
    providerRequestAbsent: false,
    claimReceiptFinalGateAbsent: false,
    invalidatedAfterAuthorityChange: false,
    reason: 'direct_source_preview_public_seam_not_exercised',
    blocksMilestoneB: true
  }
}

export function milestoneBHarnessManifestEvidence() {
  const entries = [
    ['runtime-go-packaged-milestone-b.mjs', fileURLToPath(import.meta.url)],
    ['local-provider-acceptance.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-acceptance.mjs')],
    ['local-provider-credential-scan.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-credential-scan.mjs')],
    ['runtime-go-validation-command.mjs', resolve(process.cwd(), 'scripts/runtime-go-validation-command.mjs')],
    ['after-pack.cjs', resolve(process.cwd(), 'scripts/after-pack.cjs')],
    ['package.json', resolve(process.cwd(), 'package.json')]
  ].map(([name, path]) => {
    const evidence = hashRegularFile(path)
    return { name, regular: evidence.regular, byteLength: evidence.byteLength, sha256: evidence.sha256 }
  })
  return {
    scriptSha256: entries[0]?.sha256 || '',
    contractManifestSha256: sha256(canonicalJSON(entries)),
    entries
  }
}

function safeReportSkeleton({ target, appPath, sourceCommit, timeoutMs, typedLocalDisplay }) {
  const report = {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-b',
    stage: diagnosticUnpackaged
      ? 'unpackaged-synthetic-electron-funds-diagnostic'
      : 'packaged-real-duckdb-funds-analysis-milestone-b',
    generatedAt: new Date().toISOString(),
    sourceCommit,
    harnessCommit: currentGitCommit(),
    harness: milestoneBHarnessManifestEvidence(),
    status: 'UNVERIFIED',
    passed: false,
    runtimeBlocker: '',
    timeoutMs,
    app: { targetKey: target.key, appPathHash: sha256(appPath) },
    acceptanceClass: diagnosticUnpackaged
      ? 'development-only-unpackaged-synthetic-case-diagnostic'
      : 'formal-packaged-typed-local-display-external-owner-isolated-real-case',
    mockUsed: false,
    fixtureUsed: false,
    syntheticProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    directSnapshotIPCUsed: false,
    arbitrarySQLUsed: null,
    rendererDatabasePathReceived: null,
    completePIIRecorded: null,
    phaseLanes: createMilestoneBPhaseLanes(),
    postRCChecks: POST_RC_CHECK_IDS.map((id) => checkStatus(
      id,
      'UNVERIFIED',
      'explicit joint-case product behavior is deferred outside the current macOS A0/B1 train'
    )),
    externalCase: {
      configured: false,
      ownerIsolated: false,
      contractSha256: '',
      provenanceSha256: '',
      snapshotCount: 0,
      preserved: false,
      workspacePathHash: '',
      ownerRootPathHash: ''
    },
    managedProductData: {
      configured: false,
      contract: RUNTIME_MAIN_OWNED_AUTHORITY_PURPOSE,
      ownerRootPathHash: '',
      bootstrapPathHash: '',
      bootstrapSha256: '',
      installationAuthorityKeySha256: '',
      volumeIdentityDigest: '',
      authorityRootCount: 0,
      bootstrapInstalled: false,
      installationAuthoritySeeded: false
    },
    providerAudit: {
      configured: false,
      challengePublished: false,
      challengeDigest: '',
      receiptVerified: false,
      receiptSha256: '',
      minimumObservedProviderRequestCount: 0,
      exactObservedProviderRequestCount: 0,
      providerRequestCount: 0,
      scannedRequestBodyCount: 0,
      completeIdentifierFindingCount: null,
      forbiddenHostMaterialFindingCount: null,
      providerSafeSemanticBlockCount: 0,
      providerSafeSemanticValidationCount: 0,
      keyId: ''
    },
    typedLocalDisplay,
    directSourcePreview: configuredDirectSourcePreviewStatus(),
    provider: {
      configured: false,
      credentialAuthority: 'local-provider-registry-secret-store',
      normalLocalProviderSetupObserved: false,
      automatedTestBootstrapObserved: false,
      family: '',
      modelHash: '',
      baseUrlOriginHash: '',
      credentialAuthorityBound: false,
      registryRevision: '',
      registryIncarnation: '',
      providerRevision: '',
      providerGeneration: '',
      providerIncarnation: '',
      localCredentialEvidence: {
        status: 'blocked', blocked: true, ok: false,
        blocker: 'local_provider_credential_scan_source_not_available',
        sourceBound: false, protectedStoreOwnerPrivate: false,
        expectedSecretCount: 0, sourceSecretCount: 0, uniqueSecretCount: 0,
        entryMethod: null, automatedCredentialEntryUsed: false,
        scannedFileCount: 0, findingCount: 0, settingsFindingCount: 0,
        reportFindingCount: 0, unsafeEntryCount: 0, symlinkCount: 0, encodingCoverage: []
      }
    },
    isolation: {
      trustedCacheTmpdir: false,
      cacheTmpdirPathHash: '',
      isolatedHomeUsed: false,
      isolatedUserDataUsed: false,
      sandboxRemoved: false,
      productRunRemoved: false,
      productDataOutsideCache: false,
      productOwnerStable: false,
      productVolumeRevalidated: false,
      userDataRuntimeSeparated: false,
      externalCaseOutsideSandbox: false,
      ordinaryCodeWorkspaceCleaned: false,
      caseBindingRestored: false,
      protectedSourceAliasCleaned: false
    },
    publicSeams: {
      firstLaunch: null,
      snapshotOne: null,
      snapshotTwo: null,
      relaunch: null,
      exactPackagedRuntimeExecutable: false
    },
    workflow: {
      threadIdHash: '',
      sameThread: false,
      ordinaryTurnCount: 0,
      ordinaryExecutions: [],
      fundsTurnCount: 0,
      blockedCaseTurnCount: 0,
      snapshotOne: null,
      snapshotTwo: null,
      snapshotEvolution: null,
      historicalComparison: null,
      typedLocalDisplay: null,
      directSourcePreview: null,
      typedLocalDisplayRevocation: null,
      compactionCount: 0,
      preRelaunchProjectionDigest: '',
      recoveredProjectionDigest: '',
      exactRecovery: false,
      recoveredCase: null,
      stableEntityDisplayAcrossSnapshotAndRestart: false,
      publicPIIScan: null,
      completedRunnableFlow: false
    },
    exit: {
      firstNormal: false,
      finalNormal: false,
      residualProcessCount: null
    },
    artifact: null,
    artifactFinal: null,
    checks: REQUIRED_CHECK_IDS.map((id) =>
      checkStatus(id, 'UNVERIFIED', 'check was not executed')
    ),
    failureCheckIds: [...REQUIRED_CHECK_IDS],
    blockedCheckIds: [],
    unverifiedCheckIds: [...REQUIRED_CHECK_IDS]
  }
  if (diagnosticUnpackaged) {
    report.fixtureUsed = true
    report.diagnostic = {
      mode: 'unpackaged-synthetic-current-source',
      passed: false,
      formalArtifactAccepted: false,
      formalMilestoneBPassed: false,
      syntheticCase: true,
      realNetworkProvider: false,
      sourceCommit
    }
  }
  return report
}

function checkFromEvidence(report, id, evidence, successMessage) {
  setCheck(
    report,
    id,
    evidence?.ok === true ? 'PASS' : evidence?.blocked === true ? 'BLOCKED' : 'FAIL',
    evidence?.ok === true ? successMessage : evidence?.blocker || 'verification_failed'
  )
}

function sanitizedPublicSeam(value) {
  return {
    healthOk: value?.healthOk === true,
    runtimeInfoOk: value?.runtimeInfoOk === true,
    ordinaryCatalogNonempty: value?.ordinaryCatalogNonempty === true,
    ordinaryToolContractCount: Number(value?.ordinaryToolContractCount || 0),
    ordinaryToolCatalogHash: String(value?.ordinaryToolCatalogHash || ''),
    fundsAvailable: value?.fundsAvailable === true,
    fundsExplicitlyUnavailable: value?.fundsExplicitlyUnavailable === true,
    fundsServerDiagnosticCount: Number(value?.fundsServerDiagnosticCount || 0)
  }
}

function blockedFinalEvidence(result) {
  const accepted = result?.evidence?.acceptedFinal
  const view = accepted?.publicView
  return {
    ok: privatePublicRuntimeTurnBindingMatches(result) &&
      result?.evidence?.status === 'completed' && accepted?.schemaVersion === 5 &&
      accepted?.variant === 'SourceUnavailableAnswer' &&
      accepted?.terminalReason === 'source_unavailable' &&
      accepted?.finalGateVersion === 'analytix.final-evidence-gate/v4' &&
      view?.schemaVersion === 2 && view?.publicationState === 'accepted' &&
      view?.variant === 'SourceUnavailableAnswer' && view?.claimCount === 0 &&
      result?.evidence?.toolNames?.includes(ACCOUNT_FLOW_TOOL) !== true,
    acceptedFinalDigest: String(accepted?.recordDigest || ''),
    contextEpoch: Number(accepted?.contextEpoch || 0),
    datasetSnapshotId: String(accepted?.datasetSnapshotId || '')
  }
}

export function ordinaryTurnEvidence(result, marker, hostObservation, workspace) {
  const command = ORDINARY_BASH_COMMANDS.get(marker) || ''
  const exactBashCount = result?.evidence?.toolNames
    ?.filter((name) => name === 'bash').length || 0
  const hostOwnedToolInvocationBound = Boolean(command) &&
    privateHostToolExecutionBindingMatches(hostObservation, {
      threadId: result?.threadId,
      turnId: result?.evidence?.id,
      toolName: 'bash',
      workspace,
      arguments: { command }
    })
  return {
    ok: privatePublicRuntimeTurnBindingMatches(result) &&
      result?.evidence?.status === 'completed' && result?.evidence?.provider?.ok === true &&
      exactBashCount === 1 && hostOwnedToolInvocationBound &&
      result?.evidence?.assistantText?.includes(marker) === true &&
      result?.evidence?.toolNames?.includes(ACCOUNT_FLOW_TOOL) !== true,
    turnIdHash: result?.evidence?.id ? sha256(result.evidence.id) : '',
    providerReceiptDigest: result?.evidence?.provider?.digest || '',
    toolExecutionDigest: result?.evidence?.toolExecutionDigest || '',
    hostPublicContractValidated: privatePublicRuntimeTurnBindingMatches(result),
    hostOwnedToolInvocationBound,
    publicFailure: publicTurnFailureProjection(result?.turn, result?.observation),
    hostObservationDigest: hostOwnedToolInvocationBound
      ? hostObservation.observationDigest
      : '',
    hostAuthorityBindingDigest: hostOwnedToolInvocationBound
      ? hostObservation.authorityBindingDigest
      : ''
  }
}

export function subagentIsolationProjection(summary) {
  const subagents = Array.isArray(summary?.subagents) ? summary.subagents : []
  return subagents.map((item) => ({
    id: String(item?.id || ''),
    parentThreadId: String(item?.parentThreadId || ''),
    parentTurnId: String(item?.parentTurnId || ''),
    parentToolCallId: String(item?.parentToolCallId || ''),
    childRunId: String(item?.childRunId || ''),
    childThreadId: String(item?.childThreadId || ''),
    childTurnId: String(item?.childTurnId || ''),
    profile: String(item?.profile || ''),
    toolPolicy: String(item?.toolPolicy || ''),
    status: String(item?.status || ''),
    rawStatus: String(item?.rawStatus || ''),
    terminal: item?.diagnostics?.terminal === true,
    background: item?.background === true,
    outputWithheld: item?.outputWithheld === true,
    factAnswerAllowed: item?.factAnswerAllowed === true,
    evidenceAuthority: item?.evidenceAuthority === true,
    canReadOutput: item?.canReadOutput === true,
    canContinueParent: item?.canContinueParent === true
  }))
}

export function mixedCodeTodoSubagentEvidence(
  result,
  hostObservation,
  workspace,
  codeEvidence
) {
  const executions = successfulToolExecutions(result?.turn)
  const toolNames = executions.map((item) => item.toolName)
  const taskExecutions = executions.filter((item) =>
    ['task', 'delegate_task'].includes(item.toolName))
  const taskCallIDs = new Set(taskExecutions.map((item) => item.callId))
  const todoCount = toolNames.filter((name) => ['todo_write', 'todo_ops'].includes(name)).length
  const readCount = toolNames.filter((name) => name === 'read_file').length
  const editCount = toolNames.filter((name) =>
    ['edit_file', 'write_file', 'multi_edit'].includes(name)).length
  const bashCount = toolNames.filter((name) => name === 'bash').length
  const todos = Array.isArray(result?.observation?.thread?.todos?.items)
    ? result.observation.thread.todos.items
    : []
  const subagents = subagentIsolationProjection(result?.observation?.summary)
  const subagentBound = subagents.length === 1 && taskExecutions.length === 1 &&
    subagents[0].parentThreadId === result?.threadId &&
    subagents[0].parentTurnId === result?.evidence?.id &&
    taskCallIDs.has(subagents[0].parentToolCallId) &&
    subagents[0].profile === MILESTONE_B_READ_ONLY_SUBAGENT_PROFILE &&
    subagents[0].toolPolicy === 'readOnly' && subagents[0].status === 'done' &&
    subagents[0].rawStatus === 'completed' && subagents[0].terminal === true &&
    subagents[0].background === false && subagents[0].outputWithheld === true &&
    subagents[0].factAnswerAllowed === false &&
    subagents[0].evidenceAuthority === false && subagents[0].canReadOutput === false &&
    subagents[0].canContinueParent === false && subagents[0].childRunId !== '' &&
    subagents[0].childThreadId !== '' && subagents[0].childTurnId !== ''
  const hostOwnedTestBound = privateHostToolExecutionBindingMatches(hostObservation, {
    threadId: result?.threadId,
    turnId: result?.evidence?.id,
    toolName: 'bash',
    workspace,
    arguments: { command: MIXED_CODE_TEST_COMMAND }
  })
  const todosBound = todos.length === 3 &&
    todos.every((todo) => todo?.status === 'completed' && typeof todo?.id === 'string' && todo.id)
  const ok = privatePublicRuntimeTurnBindingMatches(result) &&
    result?.evidence?.status === 'completed' && result?.evidence?.provider?.ok === true &&
    result?.evidence?.assistantText?.includes(MIXED_CODE_MARKER) === true &&
    result?.threadId === result?.observation?.thread?.id && codeEvidence?.ok === true &&
    todoCount >= 2 && readCount >= 2 && editCount === 1 && bashCount === 1 &&
    !toolNames.includes(ACCOUNT_FLOW_TOOL) && todosBound && subagentBound && hostOwnedTestBound
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'mixed_code_todo_subagent_public_seam_invalid',
    todoCount: todos.length,
    todosTerminal: todosBound,
    subagentCount: subagents.length,
    subagentBound,
    readCount,
    editCount,
    bashCount,
    hostOwnedTestBound,
    sourceSha256: codeEvidence?.sourceSha256 || '',
    testSha256: codeEvidence?.testSha256 || '',
    subagentContinuityDigest: sha256(canonicalJSON(subagents))
  })
}

export function protectedSourceBypassEvidence(result, protectedValues) {
  const items = Array.isArray(result?.turn?.items) ? result.turn.items : []
  const calls = items.filter((item) => item?.kind === 'tool_call')
  const results = items.filter((item) => item?.kind === 'tool_result')
  const exactPair = (toolName) => {
    const matchingCalls = calls.filter((item) => item?.toolName === toolName)
    const matchingResults = results.filter((item) => item?.toolName === toolName)
    return matchingCalls.length === 1 && matchingResults.length === 1 &&
      matchingCalls[0].callId === matchingResults[0].callId &&
      matchingResults[0].isError === true
      ? matchingResults[0]
      : null
  }
  const readResult = exactPair('read_file')
  const bashResult = exactPair('bash')
  const readDenied = readResult?.output?.code === 'protected_dir' ||
    readResult?.output?.code === 'host_metadata_protected'
  const bashDenied = bashResult?.output?.status === 'failed'
  const protectedFindingCount = scanStringForProtectedValues({
    read: readResult?.output,
    bash: bashResult?.output,
    assistant: result?.evidence?.assistantText
  }, protectedValues)
  const ok = privatePublicRuntimeTurnBindingMatches(result) &&
    result?.evidence?.status === 'completed' && result?.evidence?.provider?.ok === true &&
    result?.evidence?.assistantText?.includes(PROTECTED_SOURCE_BYPASS_MARKER) === true &&
    readDenied && bashDenied && protectedFindingCount === 0
  return Object.freeze({
    ok,
    blocker: ok ? '' : 'protected_source_general_tool_bypass_not_denied',
    readDenied,
    bashDenied,
    protectedFindingCount
  })
}

function readonlyAuthorityLossLocalDisplayExpression(protectedValues) {
  return `(() => {
    const protectedValues = ${JSON.stringify(protectedValues)};
    const roots = Array.from(document.querySelectorAll('[data-analytix-local-display]'));
    const text = roots.map((root) => String(root.innerText || '')).join('\n');
    return {
      unavailableCount: roots.filter((root) =>
        root.getAttribute('data-analytix-local-display') === 'unavailable').length,
      acceptedCount: roots.filter((root) =>
        root.getAttribute('data-analytix-local-display') === 'accepted_slot_display').length,
      directPreviewCount: roots.filter((root) =>
        root.getAttribute('data-analytix-local-display') === 'direct_source_preview').length,
      protectedFindingCount: protectedValues.filter((value) => value && text.includes(value)).length
    };
  })()`
}

async function refreshAndObserveAuthorityLossLocalDisplay(
  debugPort,
  protectedValues,
  timeoutMs = 20_000
) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const target = await waitForDebugTarget(
        debugPort,
        Math.min(5000, Math.max(250, deadline - Date.now()))
      )
      await dispatchCdpCommands(target.webSocketDebuggerUrl, [{
        method: 'Runtime.evaluate',
        params: { expression: `window.dispatchEvent(new Event('focus')); true`, returnByValue: true }
      }], 5000)
      await sleep(200)
      const observed = await evaluateReadonlyCdp(
        target.webSocketDebuggerUrl,
        readonlyAuthorityLossLocalDisplayExpression(protectedValues),
        5000
      )
      if (observed?.unavailableCount > 0 && observed.acceptedCount === 0 &&
          observed.directPreviewCount === 0 && observed.protectedFindingCount === 0) {
        return Object.freeze({ ok: true, ...observed })
      }
    } catch {
      // The authority refresh can race the case-bound catalog transition.
    }
    await sleep(200)
  }
  return Object.freeze({
    ok: false,
    unavailableCount: 0,
    acceptedCount: 0,
    directPreviewCount: 0,
    protectedFindingCount: 0,
    blocker: 'case_switch_local_display_revocation_not_observed'
  })
}

function threadRecoveryProjection(thread, beforeCompactionThread) {
  const compactionEvidence = compactionTransitionEvidence(beforeCompactionThread, thread)
  const compactions = compactionEvidence.ok ? compactionEvidence.compactions : []
  const value = {
    id: thread?.id || '',
    workspace: thread?.workspace || '',
    providerId: thread?.providerId || '',
    model: thread?.model || '',
    turns: thread?.turns || [],
    todos: thread?.todos || null,
    compactions
  }
  return {
    digest: sha256(canonicalJSON(value)),
    compactions,
    compactionsDigest: sha256(canonicalJSON(compactions)),
    compactionCount: compactions.length,
    compactionEvidence
  }
}

async function waitForCompaction({
  debugPort, workspace, threadId, beforeCompactionThread, minimum, timeoutMs
}) {
  const deadline = Date.now() + timeoutMs
  let latest = null
  let validation = {
    ok: false,
    blocker: 'compaction_not_observed',
    compactions: []
  }
  while (Date.now() < deadline) {
    try {
      latest = await observeRenderer({
        debugPort,
        workspace,
        exactThreadId: threadId,
        timeoutMs: Math.min(20_000, Math.max(1000, deadline - Date.now()))
      })
      validation = compactionTransitionEvidence(beforeCompactionThread, latest?.thread)
      const count = validation.ok ? validation.compactions.length : 0
      if (validation.ok && count >= minimum && terminalThread(latest?.thread)) {
        return { ok: true, blocker: '', observation: latest, count, validation }
      }
      if (validation.blocker === 'case_compaction_public_attestation_unavailable' &&
          terminalThread(latest?.thread)) {
        return { ok: false, blocker: validation.blocker, observation: latest, count: 0, validation }
      }
    } catch {
      // Compaction publication may briefly race the renderer projection.
    }
    await sleep(750)
  }
  return {
    ok: false,
    blocker: validation.blocker || 'compaction_not_observed',
    observation: latest,
    count: validation.ok ? validation.compactions.length : 0,
    validation
  }
}

async function waitForProviderAuditReceipt(
  authority,
  startedAt,
  timeoutMs,
  exactProviderRequestCount,
  minimumProviderSafeSemanticBlockCount,
  expectedProviderFamily
) {
  const deadline = Date.now() + timeoutMs
  let lastError = 'provider_audit_receipt_not_observed'
  while (Date.now() < deadline) {
    if (existsSync(authority.receiptPath)) {
      try {
        return verifyProviderAuditReceipt(
          authority,
          startedAt,
          new Date(),
          exactProviderRequestCount,
          minimumProviderSafeSemanticBlockCount,
          expectedProviderFamily
        )
      } catch (error) {
        lastError = error instanceof Error ? error.message : 'provider_audit_receipt_invalid'
      }
    }
    await sleep(500)
  }
  return { ok: false, blocker: lastError }
}

function minimumObservedProviderRequestCount(observations) {
  const attempts = new Map()
  for (const observation of observations) {
    for (const item of Array.isArray(observation?.providerAttempts)
      ? observation.providerAttempts
      : []) {
      const key = `${item?.threadId || ''}\0${item?.turnId || ''}\0${item?.seq || ''}`
      if (!item?.threadId || !item?.turnId || !Number.isSafeInteger(item?.seq) ||
          !Number.isSafeInteger(item?.providerAttemptCount) || item.providerAttemptCount <= 0) {
        continue
      }
      attempts.set(key, item.providerAttemptCount)
    }
  }
  return [...attempts.values()].reduce((total, value) => total + value, 0)
}

export function directSourcePreviewBoundaryProjection(observation) {
  const turns = Array.isArray(observation?.thread?.turns) ? observation.thread.turns : []
  const executions = turns.flatMap((turn) => successfulToolExecutions(turn))
  return Object.freeze({
    turnCount: turns.length,
    toolExecutionCount: executions.length,
    fundsToolExecutionCount: executions.filter((item) => item.toolName === ACCOUNT_FLOW_TOOL).length,
    acceptedFinalDigests: turns
      .map((turn) => String(turn?.acceptedFinal?.recordDigest || ''))
      .filter((value) => isSHA256(value))
      .sort(),
    providerRequestCount: minimumObservedProviderRequestCount([observation])
  })
}

export function directSourcePreviewBoundaryUnchanged(before, after) {
  return Boolean(before && after &&
    before.turnCount === after.turnCount &&
    before.toolExecutionCount === after.toolExecutionCount &&
    before.fundsToolExecutionCount === after.fundsToolExecutionCount &&
    before.providerRequestCount === after.providerRequestCount &&
    canonicalJSON(before.acceptedFinalDigests) === canonicalJSON(after.acceptedFinalDigests))
}

export function directSourcePreviewAuditBindingEvidence({
  minimumProviderRequestCount,
  auditReceipt,
  providerRequestAbsent
}) {
  const requestCount = Number.isSafeInteger(minimumProviderRequestCount) &&
    minimumProviderRequestCount >= 0
    ? minimumProviderRequestCount
    : 0
  if (requestCount === 0) return providerRequestAbsent === true
  return auditReceipt?.ok === true &&
    auditReceipt.providerRequestCount === requestCount &&
    providerRequestAbsent === true
}

function finalizeReport(report) {
  report.failureCheckIds = report.checks
    .filter((item) => item.status === 'FAIL')
    .map((item) => item.id)
  report.blockedCheckIds = report.checks
    .filter((item) => item.status === 'BLOCKED')
    .map((item) => item.id)
  report.unverifiedCheckIds = report.checks
    .filter((item) => item.status === 'UNVERIFIED')
    .map((item) => item.id)
  report.passed = REQUIRED_CHECK_IDS.every((id) =>
    report.checks.some((item) => item.id === id && item.status === 'PASS')
  )
  report.status = report.passed
    ? 'PASS'
    : report.failureCheckIds.length > 0
      ? 'FAIL'
      : report.blockedCheckIds.length > 0
        ? 'BLOCKED'
        : 'UNVERIFIED'
  if (diagnosticUnpackaged && report.diagnostic) {
    const diagnosticCheckIds = REQUIRED_CHECK_IDS.filter((id) =>
      id !== 'formal-packaged-artifact'
    )
    report.diagnostic.realNetworkProvider = report.provider?.configured === true
    report.diagnostic.passed = report.workflow?.completedRunnableFlow === true &&
      report.diagnostic.sourceStable === true && !report.runtimeBlocker &&
      diagnosticCheckIds.every((id) =>
        report.checks.some((item) => item.id === id && item.status === 'PASS')
      )
  }
  return report
}

export async function runMilestoneB({ credentialScanSource = null, entryAttemptId = '' } = {}) {
  let source = typeof credentialScanSource === 'function' ? null : credentialScanSource
  try {
    return await runMilestoneBWithCredentialSource({
      entryAttemptId,
      credentialScanSource: typeof credentialScanSource === 'function'
        ? async (context) => { source = await credentialScanSource(context); return source }
        : source
    })
  } finally {
    disposeLocalCredentialScanSource(source)
  }
}

async function runMilestoneBWithCredentialSource({ credentialScanSource, entryAttemptId }) {
  const startedAt = new Date()
  const target = packagedTarget()
  const appPath = packagedAppPath(target)
  const sourceCommit = expectedPackagedSourceCommit()
  const configuredTimeout = Number(optionValue(
    '--timeout-ms', process.env.ANALYTIX_RUNTIME_GO_MILESTONE_B_TIMEOUT_MS || DEFAULT_TIMEOUT_MS
  ))
  const timeoutMs = Number.isSafeInteger(configuredTimeout) && configuredTimeout >= 60_000
    ? configuredTimeout
    : DEFAULT_TIMEOUT_MS
  const typedLocalDisplay = configuredTypedLocalDisplayStatus()
  const report = safeReportSkeleton({ target, appPath, sourceCommit, timeoutMs, typedLocalDisplay })
  setCheck(
    report,
    'typed-local-display',
    typedLocalDisplay.status,
    typedLocalDisplay.reason
  )
  const cache = trustedCacheTempRoot()
  report.isolation.trustedCacheTmpdir = cache.ok
  report.isolation.cacheTmpdirPathHash = cache.pathHash || ''
  checkFromEvidence(report, 'trusted-cache-tmpdir', cache, 'trusted writable cache TMPDIR verified')

  const managedProduct = configuredManagedProductAuthority(startedAt, cache)
  report.managedProductData = {
    ...report.managedProductData,
    configured: managedProduct.ok,
    ownerRootPathHash: managedProduct.ownerRootPathHash,
    bootstrapPathHash: managedProduct.bootstrapPathHash,
    bootstrapSha256: managedProduct.bootstrapSha256,
    installationAuthorityKeySha256: managedProduct.installationAuthorityKeySha256,
    volumeIdentityDigest: managedProduct.volumeIdentityDigest,
    authorityRootCount: managedProduct.authorityRootCount
  }
  checkFromEvidence(
    report,
    'managed-product-data-authority',
    managedProduct,
    'pre-existing owner-private product data and protected authority roots verified on internal non-removable APFS'
  )

  const artifact = formalPackagedArtifactEvidence(appPath, target, sourceCommit)
  report.artifact = artifact
  if (diagnosticUnpackaged) {
    setCheck(
      report,
      'formal-packaged-artifact',
      'BLOCKED',
      'development diagnostic does not admit or substitute for a formal packaged artifact'
    )
  } else {
    checkFromEvidence(
      report,
      'formal-packaged-artifact',
      artifact,
      'formal package authority, exact source/worktree, funds artifact, fuses, and codesign verified'
    )
  }
  const diagnosticSource = diagnosticUnpackaged
    ? diagnosticUnpackagedSourceEvidence(sourceCommit)
    : null
  if (report.diagnostic) report.diagnostic.sourceBuild = diagnosticSource

  const externalCase = configuredExternalCaseAcceptance(startedAt)
  report.externalCase = {
    ...report.externalCase,
    configured: externalCase.ok,
    ownerIsolated: externalCase.ok,
    synthetic: externalCase.authority?.diagnosticSynthetic === true,
    contractSha256: externalCase.contractSha256,
    provenanceSha256: externalCase.provenanceSha256,
    snapshotCount: externalCase.snapshotCount,
    workspacePathHash: externalCase.workspacePathHash,
    ownerRootPathHash: externalCase.ownerRootPathHash
  }
  checkFromEvidence(
    report,
    'external-owner-isolated-case',
    externalCase,
    diagnosticUnpackaged
      ? 'isolated synthetic diagnostic case with contract and snapshot provenance validated'
      : 'external pre-existing owner-isolated case with contract and snapshot provenance validated'
  )
  checkFromEvidence(
    report,
    'canonical-csv-contract-and-provenance',
    externalCase,
    diagnosticUnpackaged
      ? 'two canonical synthetic CSV snapshots and independent expected truth verified for development diagnostics only'
      : 'two canonical real CSV snapshots and independent expected truth verified'
  )

  const providerAudit = configuredProviderAuditAuthority(
    startedAt,
    externalCase,
    managedProduct,
    sourceCommit
  )
  report.providerAudit.configured = providerAudit.ok
  report.providerAudit.challengeDigest = providerAudit.challengeDigest
  report.providerAudit.keyId = providerAudit.keyId
  checkFromEvidence(
    report,
    'provider-request-audit-authority',
    providerAudit,
    'fresh owner-isolated signed provider audit authority verified'
  )

  const artifactOrDiagnostic = artifact.ok || diagnosticSource?.ok === true
  // Provider-body audit is required only for a lane that actually sends a
  // Provider request. It is not a prerequisite for the local Direct Preview
  // or its zero-request privacy boundary.
  const prerequisites = cache.ok && managedProduct.ok && artifactOrDiagnostic &&
    externalCase.ok
  if (dryRun || !prerequisites) {
    finalizeMilestoneBPhaseLanes(report)
    return finalizeReport(report)
  }

  const runtimePort = await getFreePort()
  const firstDebugPort = await getFreePort()
  const secondDebugPort = await getFreePort()
  const topology = prepareMilestoneBRunTopology({
    cache,
    managedProduct,
    workspace: externalCase.authority.workspace,
    runtimePort,
    providerAuditSocketPath: providerAudit.ok ? providerAudit.authority.socketPath : ''
  })
  const {
    sandboxRoot, productRunRoot, userDataDir, runtimeDataDir, childEnv
  } = topology
  report.managedProductData.bootstrapInstalled = topology.evidence.bootstrapInstalled
  report.managedProductData.installationAuthoritySeeded =
    topology.evidence.installationAuthoritySeeded
  report.isolation.isolatedHomeUsed = topology.evidence.isolatedHomeUsed
  report.isolation.isolatedUserDataUsed = topology.evidence.isolatedUserDataUsed
  report.isolation.productDataOutsideCache = topology.evidence.productDataOutsideCache
  report.isolation.productOwnerStable = topology.evidence.productOwnerStable
  report.isolation.productVolumeRevalidated = topology.evidence.productVolumeRevalidated
  report.isolation.userDataRuntimeSeparated = topology.evidence.userDataRuntimeSeparated
  report.isolation.externalCaseOutsideSandbox = topology.evidence.externalCaseOutsideSandbox
  if (providerAudit.ok) {
    publishProviderAuditChallenge(providerAudit.authority)
    report.providerAudit.challengePublished = true
  }

  const observations = []
  const exactExitIdentities = []
  let providerAuditScanner = null
  let credentialSource = null
  let credentialProvider = null
  let finalCredentialProvider = null
  let firstChild = null
  let secondChild = null
  let threadId = ''
  let threadTitle = ''
  let firstFunds = null
  let secondFunds = null
  let mixedCodeWorkspace = null
  let caseSwitchAuthority = null
  let protectedSourceAlias = null
  let preRelaunchProjection = null
  let firstNormalExit = false
  let finalNormalExit = false
  let flowCompleted = false
  let activeOwner = Object.freeze({
    phase: 'first_launch',
    checkId: 'packaged-first-launch'
  })
  const ownActiveCheck = (phase, checkId) => {
    activeOwner = Object.freeze({ phase, checkId })
  }

  try {
    ownActiveCheck('first_launch', 'packaged-first-launch')
    mixedCodeWorkspace = prepareMixedCodeWorkspace(externalCase.authority.workspace)
    if (providerAudit.ok) {
      providerAuditScanner = await launchProviderAuditScanner(
        providerAudit.authority,
        externalCase.authority
      )
    }
    firstChild = launchPackagedApp({
      appPath,
      target,
      debugPort: firstDebugPort,
      childEnv
    })
    if (!await establishLaunchIdentity(firstChild)) {
      throw new Error('packaged_process_identity_unavailable')
    }
    const firstReady = await waitForLocalProviderWorkbench({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      expectedFunds: 'independent',
      // Renderer/runtime readiness is a prerequisite for every lane. Provider
      // readiness belongs only to the ordinary and Agent-dependent lanes, so
      // a missing Provider cannot prevent DirectSourcePreview from starting.
      providerRequired: false,
      onFreshRegistry: typeof credentialScanSource === 'function'
        ? async (remainingMs) => {
            credentialSource = await coordinateLocalCredentialEntry(
              credentialScanSource, { runId: sandboxRoot, entryAttemptId }, remainingMs
            )
          }
        : null
    })
    if (firstReady.observation) observations.push(firstReady.observation)
    report.provider.normalLocalProviderSetupObserved = firstReady.normalLocalProviderSetupObserved
    report.provider.automatedTestBootstrapObserved = false
    report.provider.configured = firstReady.provider?.ok === true
    report.provider.family = firstReady.provider?.family || ''
    report.provider.modelHash = firstReady.provider?.model
      ? sha256(firstReady.provider.model)
      : ''
    report.provider.baseUrlOriginHash = firstReady.provider?.baseUrl
      ? sha256(new URL(firstReady.provider.baseUrl).origin)
      : ''
    credentialProvider = firstReady.provider
    if (credentialSource === null && typeof credentialScanSource !== 'function') {
      credentialSource = credentialScanSource
    }
    const credentialSourceEvidence = readLocalCredentialScanSource(credentialSource, {
      runId: sandboxRoot, entryAttemptId, provider: credentialProvider
    })
    const secretStore = localProviderSecretStoreEvidence(runtimeDataDir)
    report.provider.localCredentialEvidence = {
      ...credentialSourceEvidence, protectedStoreOwnerPrivate: secretStore.protectedStoreOwnerPrivate,
      status: 'blocked', blocker: credentialSourceEvidence.blocker ||
        'local_provider_credential_isolation_scan_not_completed'
    }
    report.provider.credentialAuthorityBound = firstReady.provider?.credentialAuthorityBound === true
    for (const field of ['registryRevision', 'registryIncarnation', 'providerRevision',
      'providerGeneration', 'providerIncarnation']) {
      report.provider[field] = firstReady.provider?.[field] || ''
    }
    report.publicSeams.firstLaunch = sanitizedPublicSeam(firstReady.publicSeam)
    const runtimeProcess = exactRuntimeProcessEvidence(
      firstChild.pid,
      runtimePort,
      runtimeServerPath(appPath, target)
    )
    report.publicSeams.exactPackagedRuntimeExecutable = runtimeProcess.ok
    checkFromEvidence(
      report,
      'configured-network-provider',
      {
        ok: firstReady.provider?.ok === true && firstReady.normalLocalProviderSetupObserved === true &&
          credentialSourceEvidence.ok && secretStore.ok,
        blocked: firstReady.provider?.ok !== true || firstReady.blocked ||
          !credentialSourceEvidence.ok || !secretStore.ok,
        blocker: credentialSourceEvidence.blocker || (secretStore.ok ? '' :
          'local_provider_secret_store_not_owner_private') || firstReady.provider?.blocker || firstReady.blocker ||
          'packaged_local_provider_normal_setup_transition_not_observed'
      },
      'visible normal local Provider setup bound Registry and Secret Store authority to the network provider'
    )
    checkFromEvidence(
      report,
      'packaged-first-launch',
      {
        ok: firstReady.ok && runtimeProcess.ok,
        blocked: firstReady.blocked,
        blocker: firstReady.blocker || 'exact_packaged_runtime_process_not_observed'
      },
      diagnosticUnpackaged
        ? 'current-source unpackaged Electron renderer, health, Go runtime, and permanent ordinary catalog started'
        : 'formal packaged Electron renderer, health, Go runtime, and permanent ordinary catalog started'
    )
    if (!firstReady.ok || !runtimeProcess.ok) {
      throw new Error(firstReady.blocker || 'packaged_first_launch_failed')
    }

    const providerLaneAvailable = firstReady.provider?.ok === true &&
      firstReady.normalLocalProviderSetupObserved === true && credentialSourceEvidence.ok && secretStore.ok
    if (providerLaneAvailable) {
      // The ordinary lane starts only when its first Provider turn is about
      // to be submitted. Other lanes are started at their own seam below.
      setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
        status: 'UNVERIFIED', executed: true, blocker: ''
      })
    } else {
      setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      })
      setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      })
    }

    let ordinaryBeforeResult = null
    let ordinaryBeforeOK = false
    let ordinaryBeforeBlocker = 'provider_not_configured'
    if (!providerLaneAvailable) {
      setCheck(report, 'ordinary-before-case', 'BLOCKED', ordinaryBeforeBlocker)
      setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
        status: 'BLOCKED', blocker: ordinaryBeforeBlocker, executed: false
      })
    } else {
      ownActiveCheck('ordinary_before_case', 'ordinary-before-case')
      try {
        ordinaryBeforeResult = await submitAndWait({
          debugPort: firstDebugPort,
          workspace: externalCase.authority.workspace,
          prompt: `Use the bash tool exactly once to run /usr/bin/printf ${ORDINARY_BEFORE_MARKER}. ` +
            `Then report the exact marker ${ORDINARY_BEFORE_MARKER}. Do not answer without the real tool.`,
          timeoutMs,
          requireMarker: ORDINARY_BEFORE_MARKER
        })
        threadId = ordinaryBeforeResult.threadId
        threadTitle = ordinaryBeforeResult.threadTitle
        observations.push(ordinaryBeforeResult.observation)
        const ordinaryBeforeHostObservation = await observeHostToolExecution({
          debugPort: firstDebugPort,
          threadId,
          turnId: ordinaryBeforeResult.evidence.id,
          toolName: 'bash',
          workspace: externalCase.authority.workspace,
          arguments: { command: ORDINARY_BASH_COMMANDS.get(ORDINARY_BEFORE_MARKER) },
          timeoutMs: 20_000
        })
        const ordinaryBefore = ordinaryTurnEvidence(
          ordinaryBeforeResult,
          ORDINARY_BEFORE_MARKER,
          ordinaryBeforeHostObservation,
          externalCase.authority.workspace
        )
        const ordinaryBeforePresentation = await observeTurnRowPresentation(
          firstDebugPort,
          ordinaryBeforeResult.turn,
          [ORDINARY_BEFORE_MARKER]
        )
        ordinaryBeforeOK = ordinaryBefore.ok && ordinaryBeforePresentation.ok
        ordinaryBeforeBlocker = ordinaryBefore.publicFailure.reasonCode
          ? `ordinary_before_case_${ordinaryBefore.publicFailure.reasonCode}`
          : ordinaryBefore.ok
            ? ordinaryBeforePresentation.blocker
            : 'ordinary_bash_before_case_not_observed'
        report.workflow.ordinaryTurnCount += ordinaryBeforeOK ? 1 : 0
        report.workflow.ordinaryExecutions.push({
          phase: 'before-case',
          turnIdHash: ordinaryBefore.turnIdHash,
          toolExecutionDigest: ordinaryBefore.toolExecutionDigest,
          providerReceiptDigest: ordinaryBefore.providerReceiptDigest,
          hostPublicContractValidated: ordinaryBefore.hostPublicContractValidated,
          rendererPresented: ordinaryBeforePresentation.ok,
          publicFailure: ordinaryBefore.publicFailure,
          hostObservationDigest: ordinaryBefore.hostObservationDigest,
          hostAuthorityBindingDigest: ordinaryBefore.hostAuthorityBindingDigest
        })
        checkFromEvidence(
          report,
          'ordinary-before-case',
          { ok: ordinaryBeforeOK, blocker: ordinaryBeforeBlocker },
          'ordinary bash remained available before any case authority'
        )
        setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
          status: ordinaryBeforeOK ? 'UNVERIFIED' : 'FAIL',
          blocker: ordinaryBeforeOK ? '' : safeDiagnosticCode(
            ordinaryBeforeBlocker, 'ordinary_provider_turn_failed'
          ),
          executed: true
        })
      } catch (error) {
        ordinaryBeforeBlocker = safeDiagnosticCode(
          error instanceof Error ? error.message : '',
          'ordinary_provider_submission_failed'
        )
        setCheck(report, 'ordinary-before-case', 'FAIL', ordinaryBeforeBlocker)
        setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
          status: 'FAIL', blocker: ordinaryBeforeBlocker, executed: true
        })
      }
    }
    report.workflow.threadIdHash = threadId ? sha256(threadId) : ''

    const baseline = externalCase.authority.snapshots[0]
    let unavailableResult = null
    let unavailable = {
      ok: false,
      acceptedFinalDigest: '',
      contextEpoch: 0,
      datasetSnapshotId: ''
    }
    if (providerLaneAvailable) {
      setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
        status: 'UNVERIFIED', blocker: '', executed: true
      })
      ownActiveCheck(
        'case_authority_unavailable',
        'case-authority-unavailable-fails-closed'
      )
      try {
        unavailableResult = await submitAndWait({
          debugPort: firstDebugPort,
          workspace: externalCase.authority.workspace,
          threadId,
          prompt: `For the current active case snapshot, analyze complete bank account ` +
            `${baseline.query.completeAccount} from ${baseline.query.startInclusive} through ` +
            `${baseline.query.endInclusive}: inflow, outflow, net, transaction count, and evidence rows. ` +
            `Use only the protected funds capability and fail closed if current snapshot authority is absent.`,
          timeoutMs
        })
        observations.push(unavailableResult.observation)
        unavailable = blockedFinalEvidence(unavailableResult)
        if (!threadId) {
          threadId = unavailableResult.threadId
          threadTitle = unavailableResult.threadTitle
          report.workflow.threadIdHash = threadId ? sha256(threadId) : ''
        }
        report.workflow.blockedCaseTurnCount += unavailable.ok ? 1 : 0
        checkFromEvidence(
          report,
          'case-authority-unavailable-fails-closed',
          { ok: unavailable.ok, blocker: 'missing_dsv2_did_not_close_only_case_fact_lane' },
          'funds fact request failed closed through accepted SourceUnavailableAnswer before DSV2'
        )
        setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
          status: unavailable.ok ? 'UNVERIFIED' : 'FAIL',
          blocker: unavailable.ok ? '' : 'case_authority_unavailable_gate_failed',
          executed: true
        })
      } catch (error) {
        const blocker = safeDiagnosticCode(
          error instanceof Error ? error.message : '',
          'case_authority_unavailable_submission_failed'
        )
        setCheck(report, 'case-authority-unavailable-fails-closed', 'BLOCKED', blocker)
        setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
          status: 'BLOCKED', blocker, executed: true
        })
      }
    } else {
      setCheck(report, 'case-authority-unavailable-fails-closed', 'BLOCKED', 'provider_not_configured')
      setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      })
    }

    let ordinaryAfterBlockResult = null
    let ordinaryAfterBlockOK = false
    if (providerLaneAvailable && threadId) {
      ownActiveCheck('ordinary_after_case_block', 'ordinary-after-case-block')
      try {
        ordinaryAfterBlockResult = await submitAndWait({
          debugPort: firstDebugPort,
          workspace: externalCase.authority.workspace,
          threadId,
          prompt: `The prior case fact lane was unavailable. Use bash exactly once to run /usr/bin/printf ` +
            `${ORDINARY_AFTER_BLOCK_MARKER}, then report ${ORDINARY_AFTER_BLOCK_MARKER}.`,
          timeoutMs,
          requireMarker: ORDINARY_AFTER_BLOCK_MARKER
        })
        observations.push(ordinaryAfterBlockResult.observation)
        const ordinaryAfterBlockHostObservation = await observeHostToolExecution({
          debugPort: firstDebugPort,
          threadId,
          turnId: ordinaryAfterBlockResult.evidence.id,
          toolName: 'bash',
          workspace: externalCase.authority.workspace,
          arguments: { command: ORDINARY_BASH_COMMANDS.get(ORDINARY_AFTER_BLOCK_MARKER) },
          timeoutMs: 20_000
        })
        const ordinaryAfterBlock = ordinaryTurnEvidence(
          ordinaryAfterBlockResult,
          ORDINARY_AFTER_BLOCK_MARKER,
          ordinaryAfterBlockHostObservation,
          externalCase.authority.workspace
        )
        const ordinaryAfterBlockPresentation = await observeTurnRowPresentation(
          firstDebugPort,
          ordinaryAfterBlockResult.turn,
          [ORDINARY_AFTER_BLOCK_MARKER]
        )
        ordinaryAfterBlockOK = ordinaryAfterBlock.ok && ordinaryAfterBlockPresentation.ok
        report.workflow.ordinaryTurnCount += ordinaryAfterBlockOK ? 1 : 0
        report.workflow.ordinaryExecutions.push({
          phase: 'after-case-block',
          turnIdHash: ordinaryAfterBlock.turnIdHash,
          toolExecutionDigest: ordinaryAfterBlock.toolExecutionDigest,
          providerReceiptDigest: ordinaryAfterBlock.providerReceiptDigest,
          hostPublicContractValidated: ordinaryAfterBlock.hostPublicContractValidated,
          rendererPresented: ordinaryAfterBlockPresentation.ok,
          publicFailure: ordinaryAfterBlock.publicFailure,
          hostObservationDigest: ordinaryAfterBlock.hostObservationDigest,
          hostAuthorityBindingDigest: ordinaryAfterBlock.hostAuthorityBindingDigest
        })
        checkFromEvidence(
          report,
          'ordinary-after-case-block',
          { ok: ordinaryAfterBlockOK, blocker: ordinaryAfterBlock.ok
              ? ordinaryAfterBlockPresentation.blocker
              : 'ordinary_tool_lost_after_case_lane_block' },
          'ordinary bash continued in the same Agent and thread after the case lane failed closed'
        )
        if (!ordinaryAfterBlockOK) setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
          status: 'FAIL', blocker: 'ordinary_after_case_block_failed', executed: true
        })
      } catch (error) {
        const blocker = safeDiagnosticCode(
          error instanceof Error ? error.message : '',
          'ordinary_after_case_block_submission_failed'
        )
        setCheck(report, 'ordinary-after-case-block', 'FAIL', blocker)
        setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
          status: 'FAIL', blocker, executed: true
        })
      }
    } else {
      setCheck(report, 'ordinary-after-case-block', 'BLOCKED', providerLaneAvailable
        ? 'ordinary_thread_not_available'
        : 'provider_not_configured')
      if (!providerLaneAvailable) setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
        status: 'BLOCKED', blocker: 'provider_not_configured', executed: false
      })
    }

    let directPreviewReady = false
    let directPreviewNextObservation = null
    let stagedOne = { ok: false, blocker: 'direct_source_preview_not_executed' }
    try {
      // Direct Source Preview is independently runnable once the packaged
      // renderer/runtime seam is ready; it starts before any Provider lane.
      setMilestoneBPhaseLane(report.phaseLanes, 'b1DirectSourcePreview', {
        status: 'UNVERIFIED', blocker: '', executed: true
      })
      ownActiveCheck('direct_source_preview', 'direct-source-preview')
      const previewBoundaryBeforeObservation = await observeRenderer({
        debugPort: firstDebugPort,
        workspace: externalCase.authority.workspace,
        exactThreadId: threadId,
        timeoutMs: 20_000
      })
      const previewBoundaryBefore = directSourcePreviewBoundaryProjection(
        previewBoundaryBeforeObservation
      )
      ownActiveCheck('native_snapshot_one_staging', 'native-snapshot-one-staging')
      // This is the first provider-independent deterministic Funds seam. Do
      // not mark the lane executed until this actual staging attempt starts.
      setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
        status: 'UNVERIFIED', blocker: '', executed: true
      })
      stagedOne = await stageSnapshotThroughNativeUI({
        debugPort: firstDebugPort,
        sourcePath: baseline.sourcePath,
        timeoutMs
      })
      if (!stagedOne.ok) throw new Error(stagedOne.blocker)
      ownActiveCheck('direct_source_preview', 'direct-source-preview')
      const previewFirst = await waitForDirectSourcePreview({
        debugPort: firstDebugPort,
        mode: 'full',
        minimumRows: 25,
        firstRowIndex: 0,
        timeoutMs
      })
      const previewNextClicked = await clickDirectSourcePreviewControl(
        firstDebugPort,
        'next',
        20_000
      )
      if (!previewNextClicked.ok) throw new Error(previewNextClicked.blocker)
      const previewNext = await waitForDirectSourcePreview({
        debugPort: firstDebugPort,
        mode: 'full',
        minimumRows: 1,
        firstRowIndex: 25,
        timeoutMs
      })
      directPreviewNextObservation = previewNext.observation
      const previewModeClicked = await clickDirectSourcePreviewControl(
        firstDebugPort,
        'toggle',
        20_000
      )
      if (!previewModeClicked.ok) throw new Error(previewModeClicked.blocker)
      const previewMasked = await waitForDirectSourcePreview({
        debugPort: firstDebugPort,
        mode: 'masked',
        minimumRows: 25,
        firstRowIndex: 0,
        timeoutMs
      })
      const previewBoundaryAfterObservation = await observeRenderer({
        debugPort: firstDebugPort,
        workspace: externalCase.authority.workspace,
        exactThreadId: threadId,
        timeoutMs: 20_000
      })
      const previewBoundaryAfter = directSourcePreviewBoundaryProjection(
        previewBoundaryAfterObservation
      )
      const previewBoundaryStable = directSourcePreviewBoundaryUnchanged(
        previewBoundaryBefore,
        previewBoundaryAfter
      )
      const directPreview = directSourcePreviewEvidence({
        first: previewFirst.observation,
        next: previewNext.observation,
        masked: previewMasked.observation,
        providerRequestAbsent: previewBoundaryStable,
        agentTurnAbsent: previewBoundaryStable,
        mcpCallAbsent: previewBoundaryStable,
        claimReceiptFinalGateAbsent: previewBoundaryStable
      }, externalCase.authority.protectedValues, baseline.query.completeAccount,
      baseline.sourceRowCount)
      report.workflow.directSourcePreview = {
        fullModeExactValue: directPreview.fullModeExactValue,
        maskedModeLocalOnly: directPreview.maskedModeLocalOnly,
        paginationObserved: directPreview.paginationObserved,
        providerRequestAbsent: directPreview.providerRequestAbsent,
        agentTurnAbsent: directPreview.agentTurnAbsent,
        mcpCallAbsent: directPreview.mcpCallAbsent,
        claimReceiptFinalGateAbsent: directPreview.claimReceiptFinalGateAbsent
      }
      report.directSourcePreview = {
        ...report.directSourcePreview,
        status: directPreview.ok ? 'PASS' : 'BLOCKED',
        fullModeExactValue: directPreview.fullModeExactValue,
        maskedModeLocalOnly: directPreview.maskedModeLocalOnly,
        providerRequestAbsent: directPreview.providerRequestAbsent,
        claimReceiptFinalGateAbsent: directPreview.claimReceiptFinalGateAbsent,
        reason: directPreview.blocker,
        blocksMilestoneB: !directPreview.ok
      }
      checkFromEvidence(
        report,
        'direct-source-preview',
        { ok: directPreview.ok, blocker: directPreview.blocker },
        'Direct Source Preview showed full, masked, and paginated source rows with zero Agent, Provider, MCP, ClaimRecord, or Final Gate activity'
      )
      if (!directPreview.ok) throw new Error(directPreview.blocker)
      const previewClosed = await clickVisibleByLabels(firstDebugPort, ['Close', '关闭'], 20_000)
      if (!previewClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')
      if (threadTitle) {
        const reopenedOne = await reopenThreadInRenderer(firstDebugPort, threadTitle, 30_000)
        if (!reopenedOne.ok) throw new Error(reopenedOne.blocker)
      }
      directPreviewReady = true
      setMilestoneBPhaseLane(report.phaseLanes, 'b1DirectSourcePreview', {
        status: 'PASS', blocker: '', executed: true
      })
    } catch (error) {
      const blocker = safeDiagnosticCode(
        error instanceof Error ? error.message : '',
        'direct_source_preview_lane_failed'
      )
      setCheck(report, 'direct-source-preview', 'FAIL', blocker)
      report.directSourcePreview = {
        ...report.directSourcePreview,
        status: 'FAIL',
        reason: blocker,
        blocksMilestoneB: true
      }
      setMilestoneBPhaseLane(report.phaseLanes, 'b1DirectSourcePreview', {
        status: 'FAIL', blocker, executed: true
      })
      if (report.phaseLanes.b1DeterministicFunds?.executed !== true) {
        setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
          status: 'BLOCKED',
          blocker: 'deterministic_funds_provider_independent_seam_not_started',
          executed: false
        })
      }
    }
    let snapshotOneReady = null
    if (directPreviewReady && stagedOne.ok) {
      ownActiveCheck('funds_catalog_snapshot_one', 'funds-catalog-after-snapshot-one')
      snapshotOneReady = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
        debugPort: firstDebugPort,
        workspace: externalCase.authority.workspace,
        runtimePort,
        runtimeDataDir,
        timeoutMs,
        exactThreadId: threadId,
        expectedFunds: 'available',
        providerRequired: false
      })
      if (snapshotOneReady.observation) observations.push(snapshotOneReady.observation)
      report.publicSeams.snapshotOne = sanitizedPublicSeam(snapshotOneReady.publicSeam)
      const snapshotOneCatalogReady = snapshotOneReady.ok &&
        snapshotOneReady.publicSeam?.fundsAvailable === true
      checkFromEvidence(
        report,
        'funds-catalog-after-snapshot-one',
        {
          ok: snapshotOneCatalogReady,
          blocked: snapshotOneReady.blocked,
          blocker: snapshotOneReady.blocker || 'funds_catalog_after_snapshot_one_unavailable'
        },
        'the current DSV2 admitted the deterministic Funds catalog without requiring a Provider'
      )
      checkFromEvidence(
        report,
        'native-snapshot-one-staging',
        {
          ok: stagedOne.ok && snapshotOneCatalogReady && isSHA256(baseline.sourceSha256),
          blocker: 'baseline_native_staging_current_snapshot_binding_invalid'
        },
        'Main verified the selected source identity and current DSV2 catalog before any Provider-dependent funds turn'
      )
      setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
        status: snapshotOneCatalogReady ? 'UNVERIFIED' : 'FAIL',
        blocker: snapshotOneCatalogReady
          ? 'deterministic_funds_provider_independent_seam_not_completed'
          : safeDiagnosticCode(snapshotOneReady.blocker, 'funds_catalog_after_snapshot_one_unavailable'),
        executed: true
      })
    }
    if (providerLaneAvailable && directPreviewReady) {
      try {
        if (!snapshotOneReady?.ok) {
          throw new Error(snapshotOneReady?.blocker || 'funds_catalog_after_snapshot_one_unavailable')
        }

    ownActiveCheck('real_account_flow_one', 'real-account-flow-one')
    const firstFundsResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `Analyze complete bank account ${baseline.query.completeAccount} from ` +
        `${baseline.query.startInclusive} through ${baseline.query.endInclusive}. ` +
        `Call the protected account-flow funds tool and answer inflow, outflow, net, transaction count, ` +
        `and corresponding evidence rows. Do not use arbitrary SQL or a row-count substitute.`,
      timeoutMs
    })
    observations.push(firstFundsResult.observation)
    firstFunds = exactFundsFinalEvidence(firstFundsResult, baseline)
    const firstFundsPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      firstFundsResult.turn,
      [...expectedFundsAnswerFragments(baseline.expected), firstFunds.subjectDisplayLabel]
        .filter(Boolean)
    )
    report.workflow.fundsTurnCount += firstFunds.ok ? 1 : 0
    report.workflow.snapshotOne = {
      acceptedFinalDigest: firstFunds.acceptedFinalDigest,
      hostPublicContractValidated: firstFunds.hostPublicContractValidated,
      contextEpoch: firstFunds.contextEpoch,
      datasetSnapshotIdHash: firstFunds.datasetSnapshotIdHash,
      claimCount: firstFunds.claimCount,
      claimTypes: firstFunds.claimTypes,
      evidenceReceiptCount: firstFunds.evidenceReceiptCount,
      subjectDisplayLabelHash: firstFunds.subjectDisplayLabelHash,
      toolExecutionDigest: firstFunds.toolExecutionDigest,
      providerReceiptDigest: firstFunds.providerReceiptDigest,
      expectedTruthDigest: baseline.truthDigest
    }
    checkFromEvidence(
      report,
      'real-account-flow-one',
      { ok: firstFunds.ok && firstFundsPresentation.ok, blocker: firstFunds.ok
          ? firstFundsPresentation.blocker
          : 'baseline_real_account_flow_truth_not_observed' },
      'baseline real DuckDB account-flow result matched independently computed CSV truth'
    )
    checkFromEvidence(
      report,
      'native-snapshot-one-staging',
      {
        ok: stagedOne.ok && firstFunds.ok &&
          /^dsv2_[0-9a-f]{64}$/u.test(firstFunds.datasetSnapshotId) &&
          isSHA256(baseline.sourceSha256),
        blocker: 'baseline_native_staging_source_snapshot_binding_invalid'
      },
      'Main verified the selected source identity and the admitted current DSV2 produced the independently recomputed baseline truth'
    )
    checkFromEvidence(
      report,
      'host-funds-invocation-and-final-binding',
      {
        ok: firstFunds.ok && firstFunds.hostPublicContractValidated &&
          firstFunds.evidenceReceiptCount > 0 && firstFunds.claimCount > 0 &&
          isSHA256(firstFunds.acceptedFinalDigest),
        blocker: 'host_funds_invocation_or_accepted_final_binding_invalid'
      },
      'the fixed host funds tool was the only tool invocation and its receipts, claims, Final Gate, and accepted final were bound'
    )
    checkFromEvidence(
      report,
      'evidence-claim-final-gate-one',
      {
        ok: firstFunds.ok && firstFunds.evidenceReceiptCount > 0 && firstFunds.claimCount > 0,
        blocker: 'baseline_evidence_claim_final_gate_chain_invalid'
      },
      'EvidenceReceipt, ClaimRecord, witnessed Final Gate V4, and accepted public view were bound'
    )
    const firstUILocalScan = scanStringForProtectedValues(
      {
        bodyText: firstFundsResult.observation?.bodyText,
        bodyHTML: firstFundsResult.observation?.bodyHTML,
        assistantText: firstFundsResult.evidence.assistantText
      },
      externalCase.authority.protectedValues
    )
    checkFromEvidence(
      report,
      'default-redacted-ui-one',
      {
        ok: firstFunds.ok && firstFundsPresentation.ok && firstUILocalScan === 0 &&
          !/\bcer1_[a-z0-9_-]+\b/iu.test(firstFundsResult.evidence.assistantText),
        blocker: 'baseline_default_ui_exposed_complete_pii_or_internal_reference'
      },
      'ordinary UI showed natural masked analysis while withholding complete PII and internal references'
    )
    if (!firstFunds.ok || !firstFundsPresentation.ok || firstUILocalScan !== 0) {
      throw new Error('baseline_funds_final_failed')
    }

    ownActiveCheck('typed_local_display_snapshot_one', 'typed-local-display')
    const retainedSnapshotOneDisplay = await waitForTypedLocalDisplay({
      debugPort: firstDebugPort,
      mode: 'full',
      expectedValue: baseline.query.completeAccount,
      position: 'first',
      timeoutMs: Math.min(timeoutMs, 30_000)
    })
    const retainedSnapshotOneDisplayBound =
      retainedSnapshotOneDisplay.observation.slotCount > 0 &&
      isSHA256(firstFunds.acceptedFinalDigest)

    ownActiveCheck('ordinary_between_funds', 'ordinary-between-funds')
    const ordinaryBetweenResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `Without changing Agent or thread, complete one ordinary code task. ` +
        `First use the Todo tool to create exactly three items for inspect, fix, and test. ` +
        `Delegate exactly one foreground bounded subagent with the configured read-only profile to inspect only ` +
        `${MIXED_CODE_TEST_RELATIVE_PATH} and identify its expected arithmetic behavior; do not give the child ` +
        `case facts, funds tools, identifiers, or source rows. Then use read_file on both ` +
        `${MIXED_CODE_SOURCE_RELATIVE_PATH} and ${MIXED_CODE_TEST_RELATIVE_PATH}, edit only ` +
        `${MIXED_CODE_SOURCE_RELATIVE_PATH} so the test passes, and use bash exactly once to run ` +
        `${MIXED_CODE_TEST_COMMAND}. Mark all three Todos completed only after the real test passes. ` +
        `Finish with ${MIXED_CODE_MARKER}.`,
      timeoutMs,
      requireMarker: MIXED_CODE_MARKER
    })
    observations.push(ordinaryBetweenResult.observation)
    const ordinaryBetweenHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId,
      turnId: ordinaryBetweenResult.evidence.id,
      toolName: 'bash',
      workspace: externalCase.authority.workspace,
      arguments: { command: MIXED_CODE_TEST_COMMAND },
      timeoutMs: 20_000
    })
    const mixedCodeEvidence = mixedCodeTodoSubagentEvidence(
      ordinaryBetweenResult,
      ordinaryBetweenHostObservation,
      externalCase.authority.workspace,
      mixedCodeWorkspaceEvidence(mixedCodeWorkspace)
    )
    const mixedSubagent = subagentIsolationProjection(
      ordinaryBetweenResult.observation?.summary
    )[0]
    const mixedSubagentObservation = mixedSubagent?.childThreadId
      ? await observeRenderer({
          debugPort: firstDebugPort,
          workspace: externalCase.authority.workspace,
          exactThreadId: mixedSubagent.childThreadId,
          timeoutMs: 20_000
        })
      : null
    if (mixedSubagentObservation) observations.push(mixedSubagentObservation)
    const mixedSubagentTurn = (Array.isArray(mixedSubagentObservation?.thread?.turns)
      ? mixedSubagentObservation.thread.turns
      : []).find((turn) => turn?.id === mixedSubagent?.childTurnId)
    const mixedSubagentProviderAccounted = Boolean(
      mixedSubagentObservation?.thread?.id === mixedSubagent?.childThreadId &&
      mixedSubagentTurn && providerReceiptForTurn(
        mixedSubagentObservation,
        mixedSubagentTurn
      ).ok
    )
    const ordinaryBetweenPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      ordinaryBetweenResult.turn,
      [MIXED_CODE_MARKER]
    )
    const ordinaryBetweenOK = mixedCodeEvidence.ok && ordinaryBetweenPresentation.ok &&
      mixedSubagentProviderAccounted
    report.workflow.ordinaryTurnCount += ordinaryBetweenOK ? 1 : 0
    report.workflow.ordinaryExecutions.push({
      phase: 'mixed-code-between-funds',
      turnIdHash: sha256(ordinaryBetweenResult.evidence.id),
      toolExecutionDigest: ordinaryBetweenResult.evidence.toolExecutionDigest,
      providerReceiptDigest: ordinaryBetweenResult.evidence.provider.digest,
      hostPublicContractValidated: privatePublicRuntimeTurnBindingMatches(ordinaryBetweenResult),
      rendererPresented: ordinaryBetweenPresentation.ok,
      publicFailure: publicTurnFailureProjection(
        ordinaryBetweenResult.turn,
        ordinaryBetweenResult.observation
      ),
      todoCount: mixedCodeEvidence.todoCount,
      subagentCount: mixedCodeEvidence.subagentCount,
      subagentContinuityDigest: mixedCodeEvidence.subagentContinuityDigest,
      subagentProviderRequestAccounted: mixedSubagentProviderAccounted,
      sourceSha256: mixedCodeEvidence.sourceSha256,
      testSha256: mixedCodeEvidence.testSha256,
      hostObservationDigest: mixedCodeEvidence.hostOwnedTestBound
        ? ordinaryBetweenHostObservation.observationDigest
        : '',
      hostAuthorityBindingDigest: mixedCodeEvidence.hostOwnedTestBound
        ? ordinaryBetweenHostObservation.authorityBindingDigest
        : ''
    })
    checkFromEvidence(
      report,
      'ordinary-between-funds',
      {
        ok: ordinaryBetweenOK,
        blocker: !mixedCodeEvidence.ok
          ? mixedCodeEvidence.blocker
          : !mixedSubagentProviderAccounted
            ? 'mixed_subagent_provider_request_not_accounted'
            : ordinaryBetweenPresentation.blocker
      },
      'ordinary read/edit/test/Todo/subagent work completed in the same thread between protected funds turns'
    )
    checkFromEvidence(
      report,
      'mixed-code-todo-subagent-funds',
      {
        ok: ordinaryBetweenOK,
        blocker: mixedCodeEvidence.blocker ||
          (!mixedSubagentProviderAccounted
            ? 'mixed_subagent_provider_request_not_accounted'
            : ordinaryBetweenPresentation.blocker)
      },
      'one same-thread code repair, real host-bound test, terminal Todo set, and one output-withheld read-only subagent interleaved between exact funds turns'
    )
    if (!ordinaryBetweenOK) throw new Error('ordinary_between_funds_failed')

    ownActiveCheck('native_snapshot_two_staging', 'native-snapshot-two-staging')
    const evolved = externalCase.authority.snapshots[1]
    const stagedTwo = await stageSnapshotThroughNativeUI({
      debugPort: firstDebugPort,
      sourcePath: evolved.sourcePath,
      timeoutMs
    })
    if (!stagedTwo.ok) throw new Error(stagedTwo.blocker)
    ownActiveCheck('direct_source_preview_evolution', 'direct-source-preview')
    const evolvedPreviewFirst = await waitForDirectSourcePreview({
      debugPort: firstDebugPort,
      mode: 'full',
      minimumRows: 25,
      firstRowIndex: 0,
      timeoutMs
    })
    const evolvedPreviewNextClicked = await clickDirectSourcePreviewControl(
      firstDebugPort,
      'next',
      20_000
    )
    if (!evolvedPreviewNextClicked.ok) throw new Error(evolvedPreviewNextClicked.blocker)
    const evolvedPreviewNext = await waitForDirectSourcePreview({
      debugPort: firstDebugPort,
      mode: 'full',
      minimumRows: 1,
      firstRowIndex: 25,
      timeoutMs
    })
    const evolvedPreviewRowCountExact =
      evolvedPreviewFirst.observation.rowCount === Math.min(25, evolved.sourceRowCount) &&
      evolvedPreviewNext.observation.rowCount ===
        Math.min(25, Math.max(0, evolved.sourceRowCount - 25)) &&
      evolvedPreviewNext.observation.next?.disabled === (evolved.sourceRowCount <= 50)
    const snapshotPreviewChanged = evolvedPreviewRowCountExact &&
      sha256(canonicalJSON({
        rows: evolvedPreviewNext.observation.rowIndices,
        values: evolvedPreviewNext.observation.values
      })) !== sha256(canonicalJSON({
        rows: directPreviewNextObservation?.rowIndices || [],
        values: directPreviewNextObservation?.values || []
      }))
    report.workflow.directSourcePreview.invalidatedAfterAuthorityChange = snapshotPreviewChanged
    report.directSourcePreview.invalidatedAfterAuthorityChange = snapshotPreviewChanged
    const evolvedPreviewClosed = await clickVisibleByLabels(
      firstDebugPort,
      ['Close', '关闭'],
      20_000
    )
    if (!evolvedPreviewClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')
    const reopenedTwo = await reopenThreadInRenderer(firstDebugPort, threadTitle, 30_000)
    if (!reopenedTwo.ok) throw new Error(reopenedTwo.blocker)
    ownActiveCheck('native_snapshot_two_staging', 'native-snapshot-two-staging')
    const snapshotTwoReady = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      exactThreadId: threadId,
      expectedFunds: 'available'
    })
    if (snapshotTwoReady.observation) observations.push(snapshotTwoReady.observation)
    report.publicSeams.snapshotTwo = sanitizedPublicSeam(snapshotTwoReady.publicSeam)
    if (!snapshotTwoReady.ok) throw new Error(snapshotTwoReady.blocker)

    ownActiveCheck('retained_snapshot_resolution', 'typed-local-display-revocation')
    const retainedAfterSnapshotChangeDisplay = await waitForTypedLocalDisplay({
      debugPort: firstDebugPort,
      mode: 'full',
      expectedValue: baseline.query.completeAccount,
      position: 'first',
      timeoutMs: Math.min(timeoutMs, 30_000)
    })
    const retainedSnapshotOneReResolved = retainedSnapshotOneDisplayBound &&
      retainedAfterSnapshotChangeDisplay.observation.slotCount ===
        retainedSnapshotOneDisplay.observation.slotCount

    ownActiveCheck('real_account_flow_two', 'real-account-flow-two')
    const secondFundsResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `The current case has a newer immutable snapshot. Continue the same account analysis for ` +
        `${evolved.query.completeAccount} from ${evolved.query.startInclusive} through ` +
        `${evolved.query.endInclusive}. Call the protected account-flow funds tool and answer inflow, ` +
        `outflow, net, transaction count, and evidence rows from only the current snapshot.`,
      timeoutMs
    })
    observations.push(secondFundsResult.observation)
    secondFunds = exactFundsFinalEvidence(secondFundsResult, evolved)
    const secondFundsPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      secondFundsResult.turn,
      [...expectedFundsAnswerFragments(evolved.expected), secondFunds.subjectDisplayLabel]
        .filter(Boolean)
    )
    report.workflow.fundsTurnCount += secondFunds.ok ? 1 : 0
    report.workflow.snapshotTwo = {
      acceptedFinalDigest: secondFunds.acceptedFinalDigest,
      hostPublicContractValidated: secondFunds.hostPublicContractValidated,
      contextEpoch: secondFunds.contextEpoch,
      datasetSnapshotIdHash: secondFunds.datasetSnapshotIdHash,
      claimCount: secondFunds.claimCount,
      claimTypes: secondFunds.claimTypes,
      evidenceReceiptCount: secondFunds.evidenceReceiptCount,
      subjectDisplayLabelHash: secondFunds.subjectDisplayLabelHash,
      toolExecutionDigest: secondFunds.toolExecutionDigest,
      providerReceiptDigest: secondFunds.providerReceiptDigest,
      expectedTruthDigest: evolved.truthDigest
    }
    const oldFinalRetained = (Array.isArray(secondFundsResult.observation?.thread?.turns)
      ? secondFundsResult.observation.thread.turns
      : []).some((turn) => turn?.acceptedFinal?.recordDigest === firstFunds.acceptedFinalDigest)
    const evolvedAuthority = firstFunds.ok && secondFunds.ok &&
      firstFunds.datasetSnapshotId !== secondFunds.datasetSnapshotId &&
      secondFunds.contextEpoch > firstFunds.contextEpoch && oldFinalRetained
    report.workflow.snapshotEvolution = {
      datasetSnapshotChanged: firstFunds.datasetSnapshotId !== secondFunds.datasetSnapshotId,
      contextEpochRaised: secondFunds.contextEpoch > firstFunds.contextEpoch,
      priorAcceptedFinalRetained: oldFinalRetained,
      structuredPublicAuthority: evolvedAuthority,
      directPreviewGenerationChanged: snapshotPreviewChanged
    }
    checkFromEvidence(
      report,
      'native-snapshot-two-staging',
      {
        ok: stagedTwo.ok && secondFunds.ok &&
          /^dsv2_[0-9a-f]{64}$/u.test(secondFunds.datasetSnapshotId) &&
          isSHA256(evolved.sourceSha256) && snapshotPreviewChanged,
        blocker: 'evolved_native_staging_source_snapshot_binding_invalid'
      },
      'Main verified the evolved source identity and both current DSV2 funds truth and Direct Preview changed to the new immutable snapshot'
    )
    checkFromEvidence(
      report,
      'snapshot-evolution-and-history',
      {
        ok: evolvedAuthority && snapshotPreviewChanged,
        blocker: 'snapshot_epoch_evolution_or_historical_final_retention_invalid'
      },
      'typed current and historical snapshot authority was observed'
    )
    checkFromEvidence(
      report,
      'real-account-flow-two',
      { ok: secondFunds.ok && secondFundsPresentation.ok, blocker: secondFunds.ok
          ? secondFundsPresentation.blocker
          : 'evolved_real_account_flow_truth_not_observed' },
      'evolved real DuckDB account-flow result matched independently computed CSV truth'
    )
    if (!secondFunds.ok || !secondFundsPresentation.ok || !evolvedAuthority ||
        !snapshotPreviewChanged) {
      throw new Error('evolved_funds_final_failed')
    }

    ownActiveCheck(
      'protected_source_bypass',
      'protected-source-general-tool-bypass-denied'
    )
    protectedSourceAlias = prepareProtectedSourceAlias(
      externalCase.authority.workspace,
      runtimeDataDir
    )
    const protectedBypassResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `Test the ordinary-tool privacy boundary without using the funds capability. ` +
        `Call read_file exactly once for ${PROTECTED_SOURCE_ALIAS_RELATIVE_PATH}, then call bash ` +
        `exactly once with ${PROTECTED_SOURCE_BASH_COMMAND}. Do not substitute another path or command, ` +
        `and do not report any bytes. Both attempts must fail closed. Finish with ` +
        `${PROTECTED_SOURCE_BYPASS_MARKER}.`,
      timeoutMs,
      requireMarker: PROTECTED_SOURCE_BYPASS_MARKER
    })
    observations.push(protectedBypassResult.observation)
    const protectedBypass = protectedSourceBypassEvidence(
      protectedBypassResult,
      externalCase.authority.protectedValues
    )
    const protectedBypassPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      protectedBypassResult.turn,
      [PROTECTED_SOURCE_BYPASS_MARKER]
    )
    checkFromEvidence(
      report,
      'protected-source-general-tool-bypass-denied',
      {
        ok: protectedBypass.ok && protectedBypassPresentation.ok,
        blocker: protectedBypass.ok
          ? protectedBypassPresentation.blocker
          : protectedBypass.blocker
      },
      'read_file and bash both failed closed on one relative symlink alias to the mandatory private DuckDB root with zero protected bytes'
    )
    if (!protectedBypass.ok || !protectedBypassPresentation.ok) {
      throw new Error('protected_source_bypass_gate_failed')
    }

    ownActiveCheck(
      'case_switch_isolation',
      'case-switch-epoch-and-default-entity-isolation'
    )
    caseSwitchAuthority = prepareCaseSwitchAuthority(externalCase.authority.workspace)
    if (!switchToNegativeCase(caseSwitchAuthority)) {
      throw new Error('case_switch_binding_transition_failed')
    }
    report.negativeIsolationCaseSynthetic = true
    const negativeCaseDialog = await openDataImportDialog(firstDebugPort)
    if (!negativeCaseDialog.ok) throw new Error(negativeCaseDialog.blocker)
    const revokedLocalDisplay = await refreshAndObserveAuthorityLossLocalDisplay(
      firstDebugPort,
      externalCase.authority.protectedValues,
      Math.min(timeoutMs, 30_000)
    )
    const negativeCaseDialogClosed = await clickVisibleByLabels(
      firstDebugPort,
      ['Close', '关闭'],
      20_000
    )
    if (!negativeCaseDialogClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')
    const negativeCaseReady = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      exactThreadId: threadId,
      expectedFunds: 'independent'
    })
    if (negativeCaseReady.observation) observations.push(negativeCaseReady.observation)
    if (!negativeCaseReady.ok) throw new Error(negativeCaseReady.blocker)

    ownActiveCheck(
      'authority_loss_reacquisition',
      'authority-loss-mixed-turn-and-reacquisition'
    )
    const authorityLossResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `The active case binding changed and its protected source is unavailable. In one mixed ` +
        `request, use bash exactly once to run ${AUTHORITY_LOSS_BASH_COMMAND}, and also attempt to ` +
        `continue the prior account-flow question only through the protected funds capability without ` +
        `asking for or reproducing a complete identifier. Preserve the ordinary result, fail only the ` +
        `case-fact slot closed, and finish with ${AUTHORITY_LOSS_MARKER}.`,
      timeoutMs,
      requireMarker: AUTHORITY_LOSS_MARKER
    })
    observations.push(authorityLossResult.observation)
    const authorityLossHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId,
      turnId: authorityLossResult.evidence.id,
      toolName: 'bash',
      workspace: externalCase.authority.workspace,
      arguments: { command: AUTHORITY_LOSS_BASH_COMMAND },
      timeoutMs: 20_000
    })
    const authorityLossOrdinary = ordinaryTurnEvidence(
      authorityLossResult,
      AUTHORITY_LOSS_MARKER,
      authorityLossHostObservation,
      externalCase.authority.workspace
    )
    const authorityLossCase = blockedFinalEvidence(authorityLossResult)
    const authorityLossPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      authorityLossResult.turn,
      [AUTHORITY_LOSS_MARKER]
    )
    const authorityLossPublicPIIZero = scanStringForProtectedValues({
      thread: authorityLossResult.observation.thread,
      summary: authorityLossResult.observation.summary,
      bodyText: authorityLossResult.observation.bodyText,
      bodyHTML: authorityLossResult.observation.bodyHTML,
      assistantText: authorityLossResult.evidence.assistantText
    }, externalCase.authority.protectedValues) === 0
    const authorityLossMixedOK = authorityLossOrdinary.ok && authorityLossCase.ok &&
      authorityLossPresentation.ok && authorityLossPublicPIIZero && revokedLocalDisplay.ok
    report.workflow.ordinaryTurnCount += authorityLossMixedOK ? 1 : 0
    report.workflow.blockedCaseTurnCount += authorityLossCase.ok ? 1 : 0
    report.workflow.ordinaryExecutions.push({
      phase: 'authority-loss-mixed-turn',
      turnIdHash: sha256(authorityLossResult.evidence.id),
      toolExecutionDigest: authorityLossResult.evidence.toolExecutionDigest,
      providerReceiptDigest: authorityLossResult.evidence.provider.digest,
      hostPublicContractValidated: privatePublicRuntimeTurnBindingMatches(authorityLossResult),
      rendererPresented: authorityLossPresentation.ok,
      publicFailure: authorityLossOrdinary.publicFailure,
      hostObservationDigest: authorityLossOrdinary.hostObservationDigest,
      hostAuthorityBindingDigest: authorityLossOrdinary.hostAuthorityBindingDigest
    })
    ownActiveCheck(
      'case_switch_restoration',
      'case-switch-epoch-and-default-entity-isolation'
    )
    if (!restoreOriginalCase(caseSwitchAuthority)) {
      throw new Error('case_switch_binding_restoration_failed')
    }
    report.isolation.caseBindingRestored = true
    ownActiveCheck(
      'authority_reacquisition',
      'authority-loss-mixed-turn-and-reacquisition'
    )
    const reacquiredReady = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      exactThreadId: threadId,
      expectedFunds: 'available'
    })
    if (reacquiredReady.observation) observations.push(reacquiredReady.observation)
    if (!reacquiredReady.ok) throw new Error(reacquiredReady.blocker)
    const reacquiredResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `The original case authority is current again. Continue the same stable host-owned entity ` +
        `without asking for or reproducing a complete identifier. Call the protected account-flow funds ` +
        `tool for the current snapshot and return exact inflow, outflow, net, transaction count, and ` +
        `evidence rows.`,
      timeoutMs
    })
    observations.push(reacquiredResult.observation)
    const reacquiredFunds = exactFundsFinalEvidence(reacquiredResult, evolved)
    const reacquiredPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      reacquiredResult.turn,
      [...expectedFundsAnswerFragments(evolved.expected), reacquiredFunds.subjectDisplayLabel]
        .filter(Boolean)
    )
    const reacquiredCore = reacquiredFunds.ok && reacquiredPresentation.ok &&
      reacquiredFunds.datasetSnapshotId === secondFunds.datasetSnapshotId &&
      reacquiredFunds.subjectDisplayLabelHash === secondFunds.subjectDisplayLabelHash &&
      reacquiredFunds.contextEpoch > authorityLossCase.contextEpoch &&
      authorityLossCase.contextEpoch > secondFunds.contextEpoch
    report.workflow.fundsTurnCount += reacquiredFunds.ok ? 1 : 0
    report.workflow.reacquiredCase = {
      acceptedFinalDigest: reacquiredFunds.acceptedFinalDigest,
      contextEpoch: reacquiredFunds.contextEpoch,
      datasetSnapshotIdHash: reacquiredFunds.datasetSnapshotIdHash,
      subjectDisplayLabelHash: reacquiredFunds.subjectDisplayLabelHash,
      expectedTruthDigest: evolved.truthDigest
    }
    checkFromEvidence(
      report,
      'authority-loss-mixed-turn-and-reacquisition',
      {
        ok: authorityLossMixedOK && reacquiredCore,
        blocker: 'authority_loss_mixed_result_or_reacquisition_invalid'
      },
      'post-success authority loss blocked only the case slot, preserved a host-bound ordinary bash result, and reacquired the exact current funds source in the same thread'
    )
    checkFromEvidence(
      report,
      'case-switch-epoch-and-default-entity-isolation',
      {
        ok: authorityLossMixedOK && reacquiredCore &&
          authorityLossCase.datasetSnapshotId === '' &&
          revokedLocalDisplay.acceptedCount === 0 &&
          revokedLocalDisplay.directPreviewCount === 0 &&
          revokedLocalDisplay.protectedFindingCount === 0,
        blocker: 'case_switch_epoch_entity_or_display_isolation_invalid'
      },
      'a supplemental synthetic negative case binding advanced epoch, exposed no prior entity/snapshot/source value, revoked typed display, and restored the untouched real case authority'
    )
    if (!authorityLossMixedOK || !reacquiredCore) {
      throw new Error('authority_loss_reacquisition_failed')
    }

    ownActiveCheck('historical_continuation', 'snapshot-evolution-and-history')
    const historicalResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `Continue this multi-round case analysis. Distinguish the current snapshot result from the ` +
        `historical baseline, preserve contrary evidence and data gaps, and never upgrade historical facts ` +
        `to current facts. Finish with ${HISTORICAL_MARKER}.`,
      timeoutMs,
      requireMarker: HISTORICAL_MARKER
    })
    observations.push(historicalResult.observation)
    const historicalText = historicalResult.evidence.assistantText
    const historicalContinuation = privatePublicRuntimeTurnBindingMatches(historicalResult) &&
      historicalResult.evidence.status === 'completed' &&
      historicalResult.evidence.provider.ok && historicalText.includes(HISTORICAL_MARKER) &&
      /historical|history|历史|旧快照|基线/iu.test(historicalText) &&
      historicalResult.threadId === threadId
    report.workflow.historicalComparison = {
      completed: historicalContinuation,
      assistantTextDigest: historicalResult.evidence.assistantTextDigest,
      providerReceiptDigest: historicalResult.evidence.provider.digest,
      classification: 'provider-narrative-continuity-only',
      substitutesForStructuredSnapshotAuthority: false
    }
    if (!historicalContinuation) throw new Error('historical_continuation_failed')

    ownActiveCheck('typed_local_display', 'typed-local-display')
    const typedDisplayResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `继续当前活动案件的资金分析。请调用受保护的资金账户流分析工具，` +
        `使用当前快照和已绑定的稳定实体引用完成经证据门控的答复；不要在模型答复、` +
        `历史或普通界面文本中复述完整账号，完成后说明 ${ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER} ` +
        `之前的本地 typed AcceptedSlotDisplay 由主机按需解析。`,
      timeoutMs
    })
    observations.push(typedDisplayResult.observation)
    const typedDisplayFunds = exactFundsFinalEvidence(typedDisplayResult, evolved)
    report.workflow.fundsTurnCount += typedDisplayFunds.ok ? 1 : 0
    const typedDisplayFull = await observeTypedLocalDisplay({
      debugPort: firstDebugPort,
      timeoutMs: Math.min(timeoutMs, 30_000)
    })
    if (typedDisplayFull.observation.kind !== 'accepted_slot_display' ||
        typedDisplayFull.observation.mode !== 'full' ||
        typedDisplayFull.observation.slotCount <= 0 ||
        !typedDisplayFull.observation.toggleGeometry) {
      setCheck(report, 'typed-local-display', 'BLOCKED', 'typed_local_display_full_mode_not_observed')
      throw new Error('typed_local_display_full_mode_not_observed')
    }
    await clickCdpGeometry(
      typedDisplayFull.target,
      typedDisplayFull.observation.toggleGeometry,
      Math.min(timeoutMs, 20_000)
    )
    await sleep(300)
    const typedDisplayMasked = await observeTypedLocalDisplay({
      debugPort: firstDebugPort,
      timeoutMs: Math.min(timeoutMs, 30_000)
    })
    const genericObservation = await observeRenderer({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      exactThreadId: threadId,
      timeoutMs: 20_000
    })
    observations.push(genericObservation)
    const forbiddenGenericProviderChannels = {
      thread: genericObservation.thread,
      summary: genericObservation.summary,
      sseBody: genericObservation.sseBody,
      bodyText: genericObservation.bodyText,
      bodyHTML: genericObservation.bodyHTML
    }
    const typedDisplayCore = typedLocalDisplayEvidence({
      full: typedDisplayFull.observation,
      masked: typedDisplayMasked.observation,
      forbiddenGenericProviderChannels,
      acceptedFinalDigestSha256: isSHA256(typedDisplayFunds.acceptedFinalDigest),
      claimReceiptRefsOnly: typedDisplayFunds.ok && typedDisplayFunds.evidenceReceiptCount > 0,
      invalidatedAfterAuthorityChange: snapshotPreviewChanged && retainedSnapshotOneReResolved
    }, externalCase.authority.protectedValues, evolved.query.completeAccount)
    const typedDisplayCoreOK = typedDisplayCore.fullModeExactValue &&
      typedDisplayCore.maskedModeLocalOnly && typedDisplayCore.exactValueOnlyInTypedSink &&
      typedDisplayCore.acceptedSlotBindingVerified &&
      typedDisplayCore.forbiddenGenericProviderChannelsZero
    checkFromEvidence(
      report,
      'typed-local-display',
      {
        ok: typedDisplayCoreOK,
        blocker: typedDisplayCoreOK ? '' : typedDisplayCore.blocker
      },
      'AcceptedSlotDisplay showed full and masked values only through the typed local sink'
    )
    if (!typedDisplayCoreOK) throw new Error('typed_local_display_publication_chain_failed')

    ownActiveCheck(
      'ordinary_after_typed_local_display',
      'ordinary-after-typed-local-display'
    )
    const ordinaryAfterTypedDisplayResult = await submitAndWait({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `本地 typed display 已完成后继续普通任务。请只调用一次 bash 执行 ` +
        `/usr/bin/printf ${ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER}，然后回报精确标记 ` +
        `${ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER}。`,
      timeoutMs,
      requireMarker: ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER
    })
    observations.push(ordinaryAfterTypedDisplayResult.observation)
    const ordinaryAfterTypedDisplayHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId,
      turnId: ordinaryAfterTypedDisplayResult.evidence.id,
      toolName: 'bash',
      workspace: externalCase.authority.workspace,
      arguments: { command: ORDINARY_BASH_COMMANDS.get(ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER) },
      timeoutMs: 20_000
    })
    const ordinaryAfterTypedDisplay = ordinaryTurnEvidence(
      ordinaryAfterTypedDisplayResult,
      ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER,
      ordinaryAfterTypedDisplayHostObservation,
      externalCase.authority.workspace
    )
    const ordinaryAfterTypedDisplayPresentation = await observeTurnRowPresentation(
      firstDebugPort,
      ordinaryAfterTypedDisplayResult.turn,
      [ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER]
    )
    const ordinaryAfterTypedDisplayOK = ordinaryAfterTypedDisplay.ok &&
      ordinaryAfterTypedDisplayPresentation.ok
    report.workflow.ordinaryTurnCount += ordinaryAfterTypedDisplayOK ? 1 : 0
    report.workflow.ordinaryExecutions.push({
      phase: 'after-typed-local-display',
      turnIdHash: ordinaryAfterTypedDisplay.turnIdHash,
      toolExecutionDigest: ordinaryAfterTypedDisplay.toolExecutionDigest,
      providerReceiptDigest: ordinaryAfterTypedDisplay.providerReceiptDigest,
      hostPublicContractValidated: ordinaryAfterTypedDisplay.hostPublicContractValidated,
      rendererPresented: ordinaryAfterTypedDisplayPresentation.ok,
      publicFailure: ordinaryAfterTypedDisplay.publicFailure,
      hostObservationDigest: ordinaryAfterTypedDisplay.hostObservationDigest,
      hostAuthorityBindingDigest: ordinaryAfterTypedDisplay.hostAuthorityBindingDigest
    })
    checkFromEvidence(
      report,
      'ordinary-after-typed-local-display',
      {
        ok: ordinaryAfterTypedDisplayOK,
        blocker: ordinaryAfterTypedDisplay.ok
          ? ordinaryAfterTypedDisplayPresentation.blocker
          : 'ordinary_tool_failed_after_typed_local_display'
      },
      'the next ordinary turn completed exact bash work without inheriting the typed local display'
    )
    if (!ordinaryAfterTypedDisplayOK) throw new Error('ordinary_after_typed_local_display_failed')

    ownActiveCheck('typed_local_display_revocation', 'typed-local-display-revocation')
    let postAuthorityDisplay = null
    try {
      postAuthorityDisplay = await observeTypedLocalDisplay({
        debugPort: firstDebugPort,
        timeoutMs: Math.min(timeoutMs, 30_000)
      })
    } catch {
      postAuthorityDisplay = null
    }
    const retainedAfterOrdinaryTurn = postAuthorityDisplay?.observation?.kind ===
      'accepted_slot_display' && postAuthorityDisplay.observation.slotCount > 0
    const typedDisplayFinal = typedLocalDisplayEvidence({
      full: typedDisplayFull.observation,
      masked: typedDisplayMasked.observation,
      forbiddenGenericProviderChannels,
      acceptedFinalDigestSha256: isSHA256(typedDisplayFunds.acceptedFinalDigest),
      claimReceiptRefsOnly: typedDisplayFunds.ok && typedDisplayFunds.evidenceReceiptCount > 0,
      invalidatedAfterAuthorityChange: snapshotPreviewChanged &&
        retainedSnapshotOneReResolved && revokedLocalDisplay.ok
    }, externalCase.authority.protectedValues, evolved.query.completeAccount)
    report.workflow.typedLocalDisplay = {
      fullModeExactValue: typedDisplayFinal.fullModeExactValue,
      maskedModeLocalOnly: typedDisplayFinal.maskedModeLocalOnly,
      exactValueOnlyInTypedSink: typedDisplayFinal.exactValueOnlyInTypedSink,
      acceptedSlotBindingVerified: typedDisplayFinal.acceptedSlotBindingVerified,
      forbiddenGenericProviderChannelsZero: typedDisplayFinal.forbiddenGenericProviderChannelsZero,
      directSourcePreview: {
        status: report.directSourcePreview.status,
        paginationObserved: report.workflow.directSourcePreview.paginationObserved,
        invalidatedAfterAuthorityChange:
          report.workflow.directSourcePreview.invalidatedAfterAuthorityChange
      }
    }
    report.workflow.typedLocalDisplayRevocation = {
      retainedAfterOrdinaryTurn,
      retainedSnapshotOneReResolved,
      snapshotPreviewChanged,
      observationKind: postAuthorityDisplay?.observation?.kind || '',
      slotCount: Number(postAuthorityDisplay?.observation?.slotCount || 0)
    }
    report.typedLocalDisplay = {
      ...report.typedLocalDisplay,
      status: typedDisplayFinal.ok ? 'PASS' : 'BLOCKED',
      fullModeExactValue: typedDisplayFinal.fullModeExactValue,
      maskedModeLocalOnly: typedDisplayFinal.maskedModeLocalOnly,
      acceptedSlotBindingVerified: typedDisplayFinal.acceptedSlotBindingVerified,
      ordinaryRendererCompletePIIAbsent: typedDisplayFinal.forbiddenGenericProviderChannelsZero,
      forbiddenGenericProviderChannelsZero: typedDisplayFinal.forbiddenGenericProviderChannelsZero,
      invalidatedAfterAuthorityChange: typedDisplayFinal.invalidatedAfterAuthorityChange,
      reason: typedDisplayFinal.ok ? '' : typedDisplayFinal.blocker,
      blocksMilestoneB: !typedDisplayFinal.ok
    }
    checkFromEvidence(
      report,
      'typed-local-display-revocation',
      {
        ok: typedDisplayFinal.invalidatedAfterAuthorityChange &&
          retainedAfterOrdinaryTurn,
        blocker: 'typed_local_display_snapshot_revocation_or_retained_re_resolution_invalid'
      },
      'snapshot authority change replaced the current Direct Preview generation while the old accepted final re-resolved only from its retained immutable snapshot'
    )
    if (!typedDisplayFinal.ok) throw new Error(typedDisplayFinal.blocker)

    ownActiveCheck('same_thread_sequence', 'same-thread-additive-sequence')
    const sequenceThreadIds = [
      ordinaryBeforeResult, unavailableResult, ordinaryAfterBlockResult, firstFundsResult,
      ordinaryBetweenResult, secondFundsResult, protectedBypassResult, authorityLossResult,
      reacquiredResult, historicalResult, typedDisplayResult, ordinaryAfterTypedDisplayResult
    ].map((item) => item.threadId)
    const sameThread = sequenceThreadIds.every((id) => id === threadId)
    const sameThreadSequenceOK = sameThread && report.workflow.ordinaryTurnCount === 5 &&
      report.workflow.fundsTurnCount === 4 && report.workflow.blockedCaseTurnCount === 2
    report.workflow.sameThread = sameThread
    checkFromEvidence(
      report,
      'same-thread-additive-sequence',
      {
        ok: sameThreadSequenceOK,
        blocker: 'ordinary_case_ordinary_case_sequence_not_same_thread'
      },
      'one Agent and thread alternated ordinary and protected case capabilities without replacement mode'
    )
    if (!sameThreadSequenceOK) throw new Error('same_thread_additive_sequence_failed')

    ownActiveCheck('compaction', 'nonzero-compaction')
    const beforeCompactionObservation = await observeRenderer({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      exactThreadId: threadId,
      timeoutMs: 20_000
    })
    observations.push(beforeCompactionObservation)
    const beforeCompactionThread = structuredClone(beforeCompactionObservation.thread)
    const beforeCompactionSubagents = subagentIsolationProjection(
      beforeCompactionObservation.summary
    )
    const beforeCompactionSubagentsDigest = sha256(canonicalJSON(beforeCompactionSubagents))
    report.workflow.preRelaunchSubagentsDigest = beforeCompactionSubagentsDigest
    if (beforeCompactionThread?.id !== threadId || !terminalThread(beforeCompactionThread)) {
      throw new Error('pre_compaction_public_thread_unavailable')
    }
    const compactSubmit = await cdpComposerSubmit(firstDebugPort, '/compact', [
      'Send message', 'Send', '发送消息', '发送'
    ])
    if (!compactSubmit.ok) throw new Error(compactSubmit.blocker)
    const compacted = await waitForCompaction({
      debugPort: firstDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      beforeCompactionThread,
      minimum: 1,
      timeoutMs
    })
    if (compacted.observation) observations.push(compacted.observation)
    report.workflow.compactionCount = compacted.count
    const caseCompactionAttestationUnavailable = !compacted.ok &&
      compacted.blocker === 'case_compaction_public_attestation_unavailable' &&
      compacted.observation && terminalThread(compacted.observation.thread)
    checkFromEvidence(
      report,
      'nonzero-compaction',
      {
        ok: compacted.ok && compacted.count > 0,
        blocked: caseCompactionAttestationUnavailable,
        blocker: compacted.blocker || 'durable_compaction_not_observed'
      },
      'real composer compaction was independently bound to the exact public source and replacement'
    )
    if (!compacted.ok && !caseCompactionAttestationUnavailable) {
      throw new Error('compaction_failed')
    }
    preRelaunchProjection = threadRecoveryProjection(
      compacted.observation.thread,
      beforeCompactionThread
    )
    report.workflow.preRelaunchProjectionDigest = preRelaunchProjection.digest
    report.workflow.preRelaunchCompactionsDigest = preRelaunchProjection.compactionsDigest
    report.workflow.compactionSchemaKind = preRelaunchProjection.compactionEvidence.schemaKind
    report.workflow.compactionSourceProjectionDigest =
      preRelaunchProjection.compactionEvidence.sourceProjectionDigest
    report.workflow.compactionReplacementDigest =
      preRelaunchProjection.compactionEvidence.replacementDigest

    ownActiveCheck('normal_first_quit', 'normal-first-quit')
    const firstExitIdentities = captureExactTaskOwnedProcessTree(firstChild)
    exactExitIdentities.push(...firstExitIdentities)
    const firstQuit = await cdpNormalQuit(firstDebugPort)
    const firstExit = firstQuit.ok ? await waitForExit(firstChild, 30_000) : { exited: false }
    const firstTreeExited = firstExit.exited === true &&
      await waitForExactProcessIdentitiesToExit(firstExitIdentities)
    firstNormalExit = firstQuit.ok && firstExit.exited === true && firstExit.code === 0 &&
      !firstExit.signal && firstExitIdentities.length > 0 &&
      firstTreeExited
    report.exit.firstNormal = firstNormalExit
    checkFromEvidence(
      report,
      'normal-first-quit',
      { ok: firstNormalExit, blocker: firstQuit.blocker || 'first_packaged_exit_not_normal' },
      'first packaged process exited normally through the visible application shortcut'
    )
    if (!firstNormalExit) throw new Error('normal_first_quit_failed')

    ownActiveCheck('fresh_relaunch', 'fresh-packaged-relaunch')
    secondChild = launchPackagedApp({
      appPath,
      target,
      debugPort: secondDebugPort,
      childEnv
    })
    if (!await establishLaunchIdentity(secondChild)) {
      throw new Error('packaged_process_identity_unavailable')
    }
    const relaunched = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
      debugPort: secondDebugPort,
      workspace: externalCase.authority.workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      exactThreadId: threadId,
      expectedFunds: 'available'
    })
    const secondRuntimeProcess = exactRuntimeProcessEvidence(
      secondChild.pid,
      runtimePort,
      runtimeServerPath(appPath, target)
    )
    if (relaunched.observation) observations.push(relaunched.observation)
    report.publicSeams.relaunch = sanitizedPublicSeam(relaunched.publicSeam)
    checkFromEvidence(
      report,
      'fresh-packaged-relaunch',
      {
        ok: relaunched.ok && secondRuntimeProcess.ok,
        blocked: relaunched.blocked,
        blocker: relaunched.ok
          ? secondRuntimeProcess.blocker || 'second_exact_packaged_runtime_process_not_observed'
          : relaunched.blocker
      },
      'fresh packaged Electron and Go processes relaunched with the same isolated durable data'
    )
    if (!relaunched.ok || !secondRuntimeProcess.ok) {
      throw new Error(relaunched.blocker || 'second_exact_packaged_runtime_process_not_observed')
    }
    ownActiveCheck('exact_thread_recovery', 'exact-thread-recovery')
    const reopenedRecovered = await reopenThreadInRenderer(secondDebugPort, threadTitle, 30_000)
    if (!reopenedRecovered.ok) throw new Error(reopenedRecovered.blocker)
    let rehydratedTypedDisplay = null
    try {
      rehydratedTypedDisplay = await observeTypedLocalDisplay({
        debugPort: secondDebugPort,
        timeoutMs: Math.min(timeoutMs, 30_000)
      })
    } catch {
      rehydratedTypedDisplay = null
    }
    const typedDisplayRehydrated = rehydratedTypedDisplay?.observation?.kind ===
      'accepted_slot_display' && rehydratedTypedDisplay.observation.mode === 'full' &&
      rehydratedTypedDisplay.observation.values.includes(evolved.query.completeAccount)
    report.workflow.typedLocalDisplayRehydrated = typedDisplayRehydrated
    setCheck(
      report,
      'typed-local-display-rehydration',
      typedDisplayRehydrated ? 'PASS' : 'BLOCKED',
      typedDisplayRehydrated
        ? 'typed local display re-resolved the same immutable snapshot after relaunch'
        : 'typed_local_display_rehydration_not_observed'
    )
    const recoveredObservation = await observeRenderer({
      debugPort: secondDebugPort,
      workspace: externalCase.authority.workspace,
      exactThreadId: threadId,
      timeoutMs: 20_000
    })
    observations.push(recoveredObservation)
    const recoveredProjection = threadRecoveryProjection(
      recoveredObservation.thread,
      beforeCompactionThread
    )
    const recoveredSubagents = subagentIsolationProjection(recoveredObservation.summary)
    const recoveredSubagentsDigest = sha256(canonicalJSON(recoveredSubagents))
    const exactSubagentRecovery = beforeCompactionSubagents.length === 1 &&
      recoveredSubagents.length === 1 &&
      beforeCompactionSubagentsDigest === recoveredSubagentsDigest
    report.workflow.recoveredSubagentsDigest = recoveredSubagentsDigest
    report.workflow.exactSubagentRecovery = exactSubagentRecovery
    const recoveredRendererPresentation = await observeTurnRowPresentation(
      secondDebugPort,
      ordinaryAfterTypedDisplayResult.turn,
      [ORDINARY_AFTER_TYPED_LOCAL_DISPLAY_MARKER]
    )
    report.workflow.recoveredProjectionDigest = recoveredProjection.digest
    report.workflow.recoveredCompactionsDigest = recoveredProjection.compactionsDigest
    const compactionRecoveryBound = compacted.ok
      ? exactCompactionRecoveryMatches(preRelaunchProjection, recoveredProjection)
      : caseCompactionAttestationUnavailable &&
        recoveredProjection.compactionEvidence.blocker ===
          'case_compaction_public_attestation_unavailable'
    const exactRecovery = recoveredObservation.thread?.id === threadId &&
      recoveredProjection.digest === preRelaunchProjection.digest &&
      recoveredProjection.compactionCount === preRelaunchProjection.compactionCount &&
      compactionRecoveryBound &&
      exactSubagentRecovery && recoveredObservation.bodyText.includes(threadTitle) &&
      recoveredRendererPresentation.ok
    report.workflow.exactRecovery = exactRecovery
    checkFromEvidence(
      report,
      'exact-thread-recovery',
      { ok: exactRecovery, blocker: 'thread_or_compaction_projection_not_exactly_recovered' },
      'relaunch recovered the exact public thread projection and a visible retained turn row'
    )
    if (!exactRecovery) throw new Error('exact_thread_recovery_failed')

    ownActiveCheck('recovered_case_query', 'recovered-case-query')
    const recoveredResult = await submitAndWait({
      debugPort: secondDebugPort,
      workspace: externalCase.authority.workspace,
      threadId,
      prompt: `Continue the same current-case account-flow analysis using the stable host-owned entity identity. ` +
        `Do not ask for or reproduce any complete identifier. Call the protected funds tool for the current ` +
        `snapshot and finish with ${RECOVERED_MARKER}.`,
      timeoutMs,
      requireMarker: RECOVERED_MARKER
    })
    observations.push(recoveredResult.observation)
    const recoveredFunds = exactFundsFinalEvidence(recoveredResult, evolved)
    const recoveredFundsPresentation = await observeTurnRowPresentation(
      secondDebugPort,
      recoveredResult.turn,
      [...expectedFundsAnswerFragments(evolved.expected), RECOVERED_MARKER]
    )
    const recoveredCore = recoveredFunds.ok && recoveredFundsPresentation.ok &&
      recoveredFunds.datasetSnapshotId === secondFunds.datasetSnapshotId &&
      recoveredFunds.contextEpoch > secondFunds.contextEpoch &&
      recoveredResult.evidence.assistantText.includes(RECOVERED_MARKER)
    const stableEntityDisplay = recoveredCore &&
      firstFunds.subjectDisplayLabelHash === secondFunds.subjectDisplayLabelHash &&
      secondFunds.subjectDisplayLabelHash === recoveredFunds.subjectDisplayLabelHash &&
      isSHA256(recoveredFunds.subjectDisplayLabelHash)
    report.workflow.fundsTurnCount += recoveredFunds.ok ? 1 : 0
    report.workflow.recoveredCase = {
      acceptedFinalDigest: recoveredFunds.acceptedFinalDigest,
      hostPublicContractValidated: recoveredFunds.hostPublicContractValidated,
      contextEpoch: recoveredFunds.contextEpoch,
      datasetSnapshotIdHash: recoveredFunds.datasetSnapshotIdHash,
      subjectDisplayLabelHash: recoveredFunds.subjectDisplayLabelHash,
      toolExecutionDigest: recoveredFunds.toolExecutionDigest,
      providerReceiptDigest: recoveredFunds.providerReceiptDigest,
      expectedTruthDigest: evolved.truthDigest
    }
    report.workflow.stableEntityDisplayAcrossSnapshotAndRestart = stableEntityDisplay
    checkFromEvidence(
      report,
      'recovered-case-query',
      {
        ok: stableEntityDisplay,
        blocker: 'recovered_case_query_snapshot_truth_or_stable_entity_mismatch'
      },
      'post-restart funds query reused stable case identity and current snapshot without complete PII input'
    )
    if (!stableEntityDisplay) throw new Error('recovered_case_query_failed')

    ownActiveCheck('fork_case_isolation', 'fork-subagent-case-isolation')
    const forked = await forkCurrentThreadInRenderer(
      secondDebugPort,
      externalCase.authority.workspace,
      threadId,
      Math.min(timeoutMs, 60_000)
    )
    if (!forked.ok) throw new Error(forked.blocker)
    observations.push(forked.observation)
    const forkSubagents = subagentIsolationProjection(forked.observation.summary)
    const forkSubagentStateIsolated = forkSubagents.length <= 1 &&
      forkSubagents.every((item) => item.outputWithheld === true &&
        item.factAnswerAllowed === false && item.evidenceAuthority === false &&
        item.canReadOutput === false && item.canContinueParent === false)
    const forkResult = await submitAndWait({
      debugPort: secondDebugPort,
      workspace: externalCase.authority.workspace,
      threadId: forked.threadId,
      prompt: `In this permitted fork, continue the same stable host-owned entity and current immutable ` +
        `snapshot without asking for or reproducing a complete identifier. Call the protected account-flow ` +
        `funds tool and return exact inflow, outflow, net, transaction count, and evidence rows.`,
      timeoutMs
    })
    observations.push(forkResult.observation)
    const forkFunds = exactFundsFinalEvidence(forkResult, evolved)
    const forkFundsPresentation = await observeTurnRowPresentation(
      secondDebugPort,
      forkResult.turn,
      [...expectedFundsAnswerFragments(evolved.expected), forkFunds.subjectDisplayLabel]
        .filter(Boolean)
    )
    const forkPublicPIIZero = scanStringForProtectedValues({
      thread: forkResult.observation.thread,
      summary: forkResult.observation.summary,
      bodyText: forkResult.observation.bodyText,
      bodyHTML: forkResult.observation.bodyHTML,
      assistantText: forkResult.evidence.assistantText
    }, externalCase.authority.protectedValues) === 0
    const forkIsolation = forkFunds.ok && forkFundsPresentation.ok && forkPublicPIIZero &&
      forkResult.threadId === forked.threadId &&
      forked.observation.thread.forkedFromThreadId === threadId &&
      forkFunds.datasetSnapshotId === recoveredFunds.datasetSnapshotId &&
      forkFunds.subjectDisplayLabelHash === recoveredFunds.subjectDisplayLabelHash &&
      forkFunds.contextEpoch >= recoveredFunds.contextEpoch && forkSubagentStateIsolated
    report.workflow.fundsTurnCount += forkFunds.ok ? 1 : 0
    report.workflow.fork = {
      forkThreadIdHash: sha256(forked.threadId),
      parentThreadIdHash: sha256(threadId),
      datasetSnapshotIdHash: forkFunds.datasetSnapshotIdHash,
      subjectDisplayLabelHash: forkFunds.subjectDisplayLabelHash,
      contextEpoch: forkFunds.contextEpoch,
      subagentCount: forkSubagents.length,
      subagentStateIsolated: forkSubagentStateIsolated,
      publicPIIZero: forkPublicPIIZero
    }
    checkFromEvidence(
      report,
      'fork-subagent-case-isolation',
      {
        ok: forkIsolation,
        blocker: 'fork_case_identity_or_subagent_isolation_invalid'
      },
      'a permitted fork revalidated the same case-scoped entity and snapshot while inherited subagent state remained output-withheld and non-authoritative'
    )
    if (!forkIsolation) throw new Error('fork_case_isolation_failed')

    ownActiveCheck('normal_final_quit', 'normal-final-quit')
    finalCredentialProvider = milestoneBLocalProvider(await observeRenderer({
      debugPort: secondDebugPort, workspace: externalCase.authority.workspace,
      exactThreadId: threadId, timeoutMs: Math.min(timeoutMs, 10_000)
    }))
    const secondExitIdentities = captureExactTaskOwnedProcessTree(secondChild)
    exactExitIdentities.push(...secondExitIdentities)
    const finalQuit = await cdpNormalQuit(secondDebugPort)
    const finalExit = finalQuit.ok ? await waitForExit(secondChild, 30_000) : { exited: false }
    const secondTreeExited = finalExit.exited === true &&
      await waitForExactProcessIdentitiesToExit(secondExitIdentities)
    finalNormalExit = finalQuit.ok && finalExit.exited === true && finalExit.code === 0 &&
      !finalExit.signal && secondExitIdentities.length > 0 &&
      secondTreeExited
    report.exit.finalNormal = finalNormalExit
    checkFromEvidence(
      report,
      'normal-final-quit',
      { ok: finalNormalExit, blocker: finalQuit.blocker || 'final_packaged_exit_not_normal' },
      'final packaged process exited normally through the visible application shortcut'
    )
    if (!finalNormalExit) throw new Error('normal_final_quit_failed')
    flowCompleted = true
      } catch (error) {
        const blocker = safeDiagnosticCode(
          error instanceof Error ? error.message : '',
          'provider_dependent_b1_lane_failed'
        )
        const status = blocker.startsWith('provider_') ? 'BLOCKED' : 'FAIL'
        const activeCheck = activeOwner.checkId
        const ordinaryLaneActive = ORDINARY_PROVIDER_CHECK_IDS.includes(activeCheck)
        const fundsLaneActive = DETERMINISTIC_FUNDS_CHECK_IDS.includes(activeCheck)
        const agentLaneActive = AGENT_ACCEPTED_SLOT_CHECK_IDS.includes(activeCheck)
        if (ordinaryLaneActive) {
          setMilestoneBPhaseLane(report.phaseLanes, 'ordinaryProvider', {
            status, blocker, executed: true
          })
        } else if (fundsLaneActive) {
          setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
            status, blocker, executed: true
          })
        } else if (agentLaneActive) {
          setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
            status, blocker, executed: true
          })
        }
      }
    } else {
      // Keep unstarted lanes explicit without attributing them to an
      // ordinary/provider failure. The deterministic lane owns only its
      // provider-independent producer/storage/display seam; if that seam was
      // never reached, report its own non-Provider dependency blocker.
      if (report.phaseLanes.b1DeterministicFunds?.executed !== true) {
        setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
          status: 'BLOCKED',
          blocker: 'deterministic_funds_provider_independent_seam_not_started',
          executed: false
        })
      }
      if (report.phaseLanes.b1AgentAcceptedSlot?.executed !== true) {
        setMilestoneBPhaseLane(report.phaseLanes, 'b1AgentAcceptedSlot', {
          status: 'BLOCKED',
          blocker: providerLaneAvailable
            ? 'agent_accepted_slot_lane_not_reached'
            : 'provider_not_configured',
          executed: false
        })
      }
    }

    const snapshotTwoCheck = report.checks.find((item) => item.id === 'native-snapshot-two-staging')
    if (directPreviewReady && snapshotTwoCheck?.status !== 'PASS' &&
        firstChild && processExists(firstChild.pid)) {
      try {
        ownActiveCheck('direct_preview_authority_transitions', 'native-snapshot-two-staging')
        const transitionBoundaryBefore = directSourcePreviewBoundaryProjection(
          await observeRenderer({
            debugPort: firstDebugPort,
            workspace: externalCase.authority.workspace,
            exactThreadId: threadId,
            timeoutMs: 20_000
          })
        )
        const evolved = externalCase.authority.snapshots[1]
        const stagedTwo = await stageSnapshotThroughNativeUI({
          debugPort: firstDebugPort,
          sourcePath: evolved.sourcePath,
          timeoutMs
        })
        if (!stagedTwo.ok) throw new Error(stagedTwo.blocker)
        const evolvedPreviewFirst = await waitForDirectSourcePreview({
          debugPort: firstDebugPort,
          mode: 'full',
          minimumRows: Math.min(25, evolved.sourceRowCount),
          firstRowIndex: 0,
          timeoutMs
        })
        let evolvedPreviewNext = null
        if (evolved.sourceRowCount > 25) {
          const evolvedPreviewNextClicked = await clickDirectSourcePreviewControl(
            firstDebugPort,
            'next',
            20_000
          )
          if (!evolvedPreviewNextClicked.ok) throw new Error(evolvedPreviewNextClicked.blocker)
          evolvedPreviewNext = await waitForDirectSourcePreview({
            debugPort: firstDebugPort,
            mode: 'full',
            minimumRows: Math.min(25, evolved.sourceRowCount - 25),
            firstRowIndex: 25,
            timeoutMs
          })
        }
        const evolvedPreviewProjection = evolvedPreviewNext?.observation ||
          evolvedPreviewFirst.observation
        const snapshotPreviewChanged =
          evolvedPreviewFirst.observation.rowCount === Math.min(25, evolved.sourceRowCount) &&
          sha256(canonicalJSON({
            rows: evolvedPreviewProjection.rowIndices,
            values: evolvedPreviewProjection.values
          })) !== sha256(canonicalJSON({
            rows: directPreviewNextObservation?.rowIndices || [],
            values: directPreviewNextObservation?.values || []
          }))
        if (!snapshotPreviewChanged) throw new Error('direct_source_preview_snapshot_change_not_observed')
        const evolvedPreviewClosed = await clickVisibleByLabels(
          firstDebugPort,
          ['Close', '关闭'],
          20_000
        )
        if (!evolvedPreviewClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')

        caseSwitchAuthority = prepareCaseSwitchAuthority(externalCase.authority.workspace)
        if (!switchToNegativeCase(caseSwitchAuthority)) {
          throw new Error('direct_source_preview_case_switch_failed')
        }
        report.negativeIsolationCaseSynthetic = true
        const negativeCaseDialog = await openDataImportDialog(firstDebugPort)
        if (!negativeCaseDialog.ok) throw new Error(negativeCaseDialog.blocker)
        const revokedLocalDisplay = await refreshAndObserveAuthorityLossLocalDisplay(
          firstDebugPort,
          externalCase.authority.protectedValues,
          Math.min(timeoutMs, 30_000)
        )
        const negativeCaseDialogClosed = await clickVisibleByLabels(
          firstDebugPort,
          ['Close', '关闭'],
          20_000
        )
        if (!negativeCaseDialogClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')
        if (!revokedLocalDisplay.ok) throw new Error(revokedLocalDisplay.blocker)
        if (!restoreOriginalCase(caseSwitchAuthority)) {
          throw new Error('direct_source_preview_case_restore_failed')
        }
        report.isolation.caseBindingRestored = true

        const restoredReady = await waitForLocalProviderWorkbench({
      expectedProvider: credentialProvider,
          debugPort: firstDebugPort,
          workspace: externalCase.authority.workspace,
          runtimePort,
          runtimeDataDir,
          timeoutMs,
          exactThreadId: threadId,
          expectedFunds: 'independent',
          providerRequired: false
        })
        if (!restoredReady.ok) throw new Error(restoredReady.blocker)
        const restoredBaseline = await stageSnapshotThroughNativeUI({
          debugPort: firstDebugPort,
          sourcePath: baseline.sourcePath,
          timeoutMs
        })
        if (!restoredBaseline.ok) throw new Error(restoredBaseline.blocker)
        const restoredPreview = await waitForDirectSourcePreview({
          debugPort: firstDebugPort,
          mode: 'full',
          minimumRows: Math.min(25, baseline.sourceRowCount),
          firstRowIndex: 0,
          timeoutMs
        })
        if (!restoredPreview.observation.values.includes(baseline.query.completeAccount)) {
          throw new Error('direct_source_preview_baseline_restore_not_observed')
        }
        const restoredPreviewClosed = await clickVisibleByLabels(
          firstDebugPort,
          ['Close', '关闭'],
          20_000
        )
        if (!restoredPreviewClosed.ok) throw new Error('direct_source_preview_dialog_not_closed')
        const transitionBoundaryAfter = directSourcePreviewBoundaryProjection(
          await observeRenderer({
            debugPort: firstDebugPort,
            workspace: externalCase.authority.workspace,
            exactThreadId: threadId,
            timeoutMs: 20_000
          })
        )
        if (!directSourcePreviewBoundaryUnchanged(
          transitionBoundaryBefore,
          transitionBoundaryAfter
        )) {
          throw new Error('direct_source_preview_authority_transition_created_agent_activity')
        }
        report.workflow.directSourcePreview.invalidatedAfterAuthorityChange = true
        report.workflow.directSourcePreview.revokedAfterCaseChange = true
        report.directSourcePreview.invalidatedAfterAuthorityChange = true
        report.directSourcePreview.revokedAfterCaseChange = true
        checkFromEvidence(
          report,
          'native-snapshot-two-staging',
          { ok: true, blocker: '' },
          'the provider-independent current-snapshot preview changed, revoked on case change, and restored without Agent activity'
        )
        setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
          status: 'PASS', blocker: '', executed: true
        })
      } catch (error) {
        const blocker = safeDiagnosticCode(
          error instanceof Error ? error.message : '',
          'direct_source_preview_authority_transition_failed'
        )
        setCheck(report, 'native-snapshot-two-staging', 'FAIL', blocker)
        setMilestoneBPhaseLane(report.phaseLanes, 'b1DeterministicFunds', {
          status: 'FAIL', blocker, executed: true
        })
        setCheck(report, 'direct-source-preview', 'FAIL', blocker)
        setMilestoneBPhaseLane(report.phaseLanes, 'b1DirectSourcePreview', {
          status: 'FAIL', blocker, executed: true
        })
      }
    }
  } catch (error) {
    attributeMilestoneBActiveFailure(
      report,
      activeOwner,
      error instanceof Error ? error.message : ''
    )
  } finally {
    // Preserve the final public identity before stopping the isolated app.
    // Failure leaves no authority for a passing scan; the first-ready tuple
    // must never be substituted for an unavailable or changed final readback.
    if (credentialSource && finalCredentialProvider === null) {
      try {
        const debugPort = secondChild && processExists(secondChild.pid)
          ? secondDebugPort : firstChild && processExists(firstChild.pid) ? firstDebugPort : null
        if (debugPort !== null) {
          finalCredentialProvider = milestoneBLocalProvider(await observeRenderer({
            debugPort, workspace: externalCase.authority.workspace,
            exactThreadId: threadId, timeoutMs: Math.min(timeoutMs, 10_000)
          }))
        }
      } catch { finalCredentialProvider = null }
    }
    if (firstChild && processExists(firstChild.pid)) {
      exactExitIdentities.push(...await stopExactTaskOwnedChild(firstChild))
    }
    if (secondChild && processExists(secondChild.pid)) {
      exactExitIdentities.push(...await stopExactTaskOwnedChild(secondChild))
    }
    if (providerAuditScanner && processExists(providerAuditScanner.pid)) {
      exactExitIdentities.push(...await stopExactTaskOwnedChild(providerAuditScanner))
    }
    await stopExactProcessIdentities(
      exactExitIdentities.filter((identity) => processIdentityMatches(identity))
    )
    report.isolation.caseBindingRestored = cleanupOptionalHarnessResource(
      caseSwitchAuthority,
      restoreOriginalCase
    )
    report.isolation.protectedSourceAliasCleaned = cleanupOptionalHarnessResource(
      protectedSourceAlias,
      cleanupProtectedSourceAlias
    )
    report.isolation.ordinaryCodeWorkspaceCleaned = cleanupMixedCodeWorkspace(
      mixedCodeWorkspace
    )
  }

  const publicScan = publicPIIScanEvidence(
    observations,
    externalCase.authority.protectedValues,
    [join(userDataDir, 'logs'), join(runtimeDataDir, 'logs')]
  )
  report.workflow.publicPIIScan = publicScan
  checkFromEvidence(
    report,
    'public-pii-scan-zero',
    { ok: publicScan.ok, blocker: 'complete_pii_found_on_public_surface_or_logs' },
    'complete identifiers scanned to zero across public history, SSE, renderer, compaction, and logs'
  )

  const minimumProviderRequestCount = minimumObservedProviderRequestCount(observations)
  report.providerAudit.minimumObservedProviderRequestCount = minimumProviderRequestCount
  report.providerAudit.exactObservedProviderRequestCount = minimumProviderRequestCount
  const auditReceipt = minimumProviderRequestCount > 0 && providerAudit.ok
    ? await waitForProviderAuditReceipt(
        providerAudit.authority,
        startedAt,
        Math.min(timeoutMs, 120_000),
        minimumProviderRequestCount,
        report.workflow.fundsTurnCount,
        report.provider.family
      )
    : {
        ok: false,
        blocker: minimumProviderRequestCount > 0
          ? providerAudit.ok
            ? 'provider_request_not_observed'
            : 'provider_request_audit_authority_not_configured'
          : 'provider_request_not_observed'
      }
  if (auditReceipt.ok) {
    report.providerAudit.receiptVerified = true
    report.providerAudit.receiptSha256 = auditReceipt.receiptSha256
    report.providerAudit.providerRequestCount = auditReceipt.providerRequestCount
    report.providerAudit.scannedRequestBodyCount = auditReceipt.scannedRequestBodyCount
    report.providerAudit.completeIdentifierFindingCount = 0
    report.providerAudit.forbiddenHostMaterialFindingCount = 0
    report.providerAudit.providerSafeSemanticBlockCount =
      auditReceipt.providerSafeSemanticBlockCount
    report.providerAudit.providerSafeSemanticValidationCount =
      auditReceipt.providerSafeSemanticValidationCount
  }
  const providerAuditEvidence = providerAuditDependentEvidence({
    auditReceipt,
    observedProviderRequestCount: minimumProviderRequestCount,
    minimumProviderSafeSemanticBlockCount: report.workflow.fundsTurnCount,
    upstreamReasonCode: report.runtimeBlocker
  })
  checkFromEvidence(
    report,
    'provider-payload-pii-scan-zero',
    providerAuditEvidence.payload,
    'trusted signed outbound provider-body audit scanned nonzero requests with zero complete identifiers'
  )
  checkFromEvidence(
    report,
    'provider-semantic-context-contract',
    providerAuditEvidence.semantic,
    'trusted provider audit observed only bounded provider-safe account-flow semantic projections'
  )
  // Direct Source Preview is a local current-snapshot seam. With zero
  // Provider requests there is no provider audit receipt to wait for; only a
  // nonzero request run must bind its exact signed audit count here.
  const directPreviewProviderAuditExact = directSourcePreviewAuditBindingEvidence({
    minimumProviderRequestCount,
    auditReceipt,
    providerRequestAbsent: report.workflow.directSourcePreview?.providerRequestAbsent
  })
  if (report.workflow.directSourcePreview) {
    report.workflow.directSourcePreview.providerRequestAbsent = directPreviewProviderAuditExact
    report.directSourcePreview.providerRequestAbsent = directPreviewProviderAuditExact
    const directPreviewFinal = report.directSourcePreview.fullModeExactValue === true &&
      report.directSourcePreview.maskedModeLocalOnly === true &&
      report.workflow.directSourcePreview.paginationObserved === true &&
      report.workflow.directSourcePreview.agentTurnAbsent === true &&
      report.workflow.directSourcePreview.mcpCallAbsent === true &&
      report.directSourcePreview.claimReceiptFinalGateAbsent === true &&
      report.phaseLanes.b1DirectSourcePreview?.status === 'PASS' &&
      directPreviewProviderAuditExact
    report.directSourcePreview.status = directPreviewFinal ? 'PASS' : 'BLOCKED'
    report.directSourcePreview.blocksMilestoneB = !directPreviewFinal
    report.directSourcePreview.reason = directPreviewFinal
      ? ''
      : 'direct_source_preview_provider_audit_or_public_seam_invalid'
    checkFromEvidence(
      report,
      'direct-source-preview',
      {
        ok: directPreviewFinal,
        blocked: providerAuditEvidence.payload.blocked === true,
        blocker: report.directSourcePreview.reason
      },
      'Direct Source Preview full, masked, pagination, and zero Agent/Provider/MCP/final activity were bound to the exact signed provider audit count'
    )
  }

  const residualIdentities = exactExitIdentities.filter((identity) =>
    processIdentityMatches(identity)
  )
  report.exit.residualProcessCount = residualIdentities.length
  checkFromEvidence(
    report,
    'zero-residual-processes',
    { ok: residualIdentities.length === 0, blocker: 'task_owned_packaged_processes_remain' },
    'all exact task-owned packaged Electron and Go descendant processes exited'
  )

  if (diagnosticUnpackaged) {
    const finalSource = diagnosticUnpackagedSourceEvidence(sourceCommit)
    report.artifactFinal = finalSource
    const sourceStable = finalSource.ok && diagnosticSource?.ok === true &&
      canonicalJSON(finalSource) === canonicalJSON(diagnosticSource)
    if (report.diagnostic) report.diagnostic.sourceStable = sourceStable
    if (!sourceStable) report.runtimeBlocker = 'unpackaged_diagnostic_source_changed_during_run'
  } else {
    const finalArtifact = formalPackagedArtifactEvidence(appPath, target, sourceCommit)
    report.artifactFinal = finalArtifact
    const artifactStable = finalArtifact.ok && artifact.ok &&
      finalArtifact.authoritySha256 === artifact.authoritySha256 &&
      finalArtifact.executableSha256 === artifact.executableSha256 &&
      finalArtifact.runtimeServerSha256 === artifact.runtimeServerSha256 &&
      finalArtifact.appAsarSha256 === artifact.appAsarSha256
    if (!artifactStable) {
      setCheck(report, 'formal-packaged-artifact', 'FAIL',
        finalArtifact.blocker || 'formal_package_changed_during_acceptance')
    }
  }

  const casePreserved = verifyMilestoneBExternalCasePreserved(externalCase.authority)
  report.externalCase.preserved = casePreserved
  try {
    report.provider.localCredentialEvidence = await scanMilestoneBLocalCredentials({
      source: credentialSource, runId: sandboxRoot, entryAttemptId, provider: finalCredentialProvider,
      runtimeDataDir, sandboxRoot, productRunRoot,
      settingsPath: join(userDataDir, 'analytix-settings.json'), reportSnapshot: report
    })
    const credentials = report.provider.localCredentialEvidence
    if (!credentials.ok) {
      setCheck(report, 'configured-network-provider',
        credentials.status === 'failed' ? 'FAIL' : 'BLOCKED', credentials.blocker)
    }
  } finally {
    disposeLocalCredentialScanSource(credentialSource)
  }

  const sandboxRemoved = removeExactHarnessOwnedRoot(
    cache.path,
    sandboxRoot,
    'analytix-milestone-b-'
  )
  const productRunRemoved = removeExactHarnessOwnedRoot(
    managedProduct.authority.ownerRoot,
    productRunRoot,
    'analytix-milestone-b-run-'
  )
  report.isolation.sandboxRemoved = sandboxRemoved
  report.isolation.productRunRemoved = productRunRemoved
  checkFromEvidence(
    report,
    'sandbox-cleanup-and-case-preservation',
    {
      ok: sandboxRemoved && productRunRemoved && casePreserved &&
        report.isolation.ordinaryCodeWorkspaceCleaned &&
        report.isolation.caseBindingRestored &&
        report.isolation.protectedSourceAliasCleaned,
      blocker: !casePreserved
        ? 'external_case_input_changed'
        : !report.isolation.caseBindingRestored
          ? 'case_binding_restoration_failed'
        : !report.isolation.protectedSourceAliasCleaned
          ? 'protected_source_alias_cleanup_failed'
        : !report.isolation.ordinaryCodeWorkspaceCleaned
          ? 'mixed_code_workspace_cleanup_failed'
        : !productRunRemoved
          ? 'managed_product_run_cleanup_failed'
          : 'isolated_cache_sandbox_cleanup_failed'
    },
    'exact harness-owned cache and product-run roots were removed and external source snapshots remained byte-identical'
  )

  report.workflow.completedRunnableFlow = flowCompleted
  finalizeMilestoneBPhaseLanes(report)
  return finalizeReport(report)
}

function reportOutputPath() {
  return resolve(process.cwd(), optionValue(
    '--output', process.env.ANALYTIX_RUNTIME_GO_MILESTONE_B_OUTPUT || DEFAULT_OUTPUT
  ))
}

async function main() {
  let report
  try {
    report = await runMilestoneB()
  } catch (error) {
    const target = (() => {
      try { return packagedTarget() } catch { return { platform: process.platform, arch: process.arch, key: 'invalid' } }
    })()
    const appPath = (() => {
      try { return packagedAppPath(target) } catch { return resolve(process.cwd(), 'dist/unknown') }
    })()
    report = safeReportSkeleton({
      target,
      appPath,
      sourceCommit: expectedPackagedSourceCommit(),
      timeoutMs: DEFAULT_TIMEOUT_MS,
      typedLocalDisplay: configuredTypedLocalDisplayStatus()
    })
    report.runtimeBlocker = error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
      ? error.message
      : 'packaged_milestone_b_preflight_failed'
    setCheck(report, 'formal-packaged-artifact', 'FAIL', report.runtimeBlocker)
    finalizeReport(report)
  }
  if (!noWrite) {
    const output = reportOutputPath()
    mkdirSync(dirname(output), { recursive: true })
    writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8')
  }
  if (jsonOutput) process.stdout.write(`${JSON.stringify(report)}\n`)
  else process.stdout.write(`Milestone B: ${report.status}\n`)
  if (!reportOnly && !report.passed) process.exitCode = 1
}

function isDirectExecution() {
  try {
    return realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url))
  } catch {
    return false
  }
}

if (isDirectExecution()) await main()
