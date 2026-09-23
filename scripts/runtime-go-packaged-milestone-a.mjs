#!/usr/bin/env node

import { spawn, spawnSync } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import {
  accessSync,
  chmodSync,
  closeSync,
  constants,
  fchmodSync,
  existsSync,
  fstatSync,
  fsyncSync,
  ftruncateSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readSync,
  realpathSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync
} from 'node:fs'
import net from 'node:net'
import { createRequire } from 'node:module'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'
import {
  releasePublicationConfiguration,
  verifyPackagedReleasePublicationAuthority
} from './lib/packaged-release-publication-authority.mjs'

import {
  LOCAL_PROVIDER_AUTHORITY, localProviderFromObservation,
  freshLocalProviderRegistry, localProviderSecretStoreEvidence, sameLocalProviderAuthority
} from './lib/local-provider-acceptance.mjs'
import {
  coordinateLocalCredentialEntry, readLocalCredentialScanSource, scanLocalCredentialIsolation,
  disposeLocalCredentialScanSource
} from './lib/local-provider-credential-scan.mjs'

const require = createRequire(import.meta.url)
const {
  PACKAGED_BUILD_AUTHORITY_CONTRACT,
  PACKAGED_BUILD_AUTHORITY_FILE,
  _internals: packagedAuthorityContract
} = require('./after-pack.cjs')

const rawArgs = process.argv.slice(2)
const args = new Set(rawArgs)
const HELP_TEXT = [
  'Usage: node scripts/runtime-go-packaged-milestone-a.mjs [options]',
  '',
  'Options:',
  '  -h, --help  Show this help message.'
].join('\n') + '\n'
const jsonOutput = args.has('--json') || args.has('--dry-run')
const dryRun = args.has('--dry-run')
const noWrite = args.has('--no-write') || dryRun
const reportOnly = args.has('--no-gate')
const CACHE_MOUNT = '/Volumes/AnalytixCache'
const DEFAULT_TIMEOUT_MS = 20 * 60 * 1000
const DEFAULT_OUTPUT =
  'docs/analytix/upstreams/runtime-go-live-evidence/packaged-milestone-a.json'
const RESULT_MARKER = 'MILESTONE_A_RESULT_OK'
const PLAN_MARKER = 'MILESTONE_A_PLAN_OK'
const TEST_MARKER = 'MILESTONE_A_REAL_TEST_EXECUTED'
const PROVIDER_RECEIPT_REASON_CODES = Object.freeze([
  'provider_receipt_not_observed',
  'provider_receipt_input_invalid',
  'provider_receipt_timeout',
  'provider_receipt_event_envelope_invalid',
  'provider_receipt_event_sequence_invalid',
  'provider_receipt_event_scope_invalid',
  'provider_receipt_non_general_terminal',
  'provider_receipt_target_turn_duplicate',
  'provider_receipt_target_turn_missing',
  'provider_receipt_terminal_batch_invalid',
  'provider_receipt_terminal_pair_invalid',
  'provider_receipt_item_invalid',
  'provider_receipt_usage_telemetry_invalid',
  'provider_receipt_attempt_status_invalid',
  'provider_receipt_attempt_counts_invalid',
  'provider_receipt_ack_rejected',
  'provider_receipt_ack_failed',
  'provider_receipt_stream_ended_without_batch',
  'provider_receipt_stream_error',
  'provider_receipt_stream_event_rejected',
  'provider_receipt_sse_accepted_final_authority_pin_origin_mismatch',
  'provider_receipt_sse_accepted_final_authority_pin_unavailable',
  'provider_receipt_sse_accepted_final_authority_unavailable',
  'provider_receipt_sse_accepted_final_batch_invalid',
  'provider_receipt_sse_accepted_final_authority_invalid',
  'provider_receipt_sse_general_terminal_batch_invalid',
  'provider_receipt_sse_general_terminal_batch_public_tree_invalid',
  'provider_receipt_sse_general_terminal_batch_private_reasoning',
  'provider_receipt_sse_general_terminal_batch_restricted_evidence',
  'provider_receipt_sse_general_terminal_batch_secret_material',
  'provider_receipt_sse_general_terminal_batch_ordinary_pii',
  'provider_receipt_sse_general_terminal_batch_json_invalid',
  'provider_receipt_sse_general_terminal_batch_schema_invalid',
  'provider_receipt_sse_general_terminal_batch_ordinary_result_invalid',
  'provider_receipt_sse_general_terminal_batch_event_binding_invalid',
  'provider_receipt_sse_general_terminal_batch_integrity_invalid',
  'provider_receipt_sse_general_terminal_batch_verification_failed',
  'provider_receipt_sse_malformed_frame',
  'provider_receipt_sse_missing_transport_binding',
  'provider_receipt_sse_event_kind_mismatch',
  'provider_receipt_sse_event_sequence_mismatch',
  'provider_receipt_sse_event_thread_mismatch',
  'provider_receipt_sse_unsupported_event_kind',
  'provider_receipt_sse_invalid_public_projection',
  'provider_receipt_stream_transport_error',
  'provider_receipt_stream_http_error',
  'provider_receipt_stream_setup_error',
  'provider_receipt_listener_invalid',
  'provider_receipt_start_rejected',
  'provider_receipt_start_failed',
  'provider_receipt_observer_failed',
  'provider_receipt_trace_invalid',
  'provider_receipt_observed',
  'accepted_final_not_observed',
  'accepted_final_input_invalid',
  'accepted_final_timeout',
  'accepted_final_event_envelope_invalid',
  'accepted_final_event_sequence_invalid',
  'accepted_final_event_scope_invalid',
  'accepted_final_duplicate',
  'accepted_final_schema_invalid',
  'accepted_final_terminal_missing',
  'accepted_final_ack_rejected',
  'accepted_final_ack_failed',
  'accepted_final_start_rejected',
  'accepted_final_start_failed',
  'accepted_final_observer_failed',
  'accepted_final_observed'
])
const CACHE_HELPER_SOURCE_RELATIVE_PATH = 'scripts/use-analytix-cache.sh'
const CACHE_HELPER_RUNTIME_SOURCE_CLOSURE = Object.freeze([
  CACHE_HELPER_SOURCE_RELATIVE_PATH,
  'scripts/analytix-cache-storage.zsh'
])
const TOOL_EXECUTION_OBSERVATION_PATH = '/v1/runtime/tool-executions/observe'
const HOST_TOOL_EXECUTION_OBSERVATION_REASON_CODES = Object.freeze([
  'observed',
  'not_observed',
  'http_error',
  'transport_unavailable',
  'response_schema_invalid'
])
const HOST_TOOL_EXECUTION_MAX_404_RETRIES = 2
const HOST_TOOL_EXECUTION_404_RETRY_DELAYS_MS = Object.freeze([100, 250])
const READONLY_CDP_FAILURE_REASON_CODES = Object.freeze([
  'readonly_cdp_execution_context_unavailable',
  'readonly_cdp_protocol_invalid_params',
  'readonly_cdp_protocol_error',
  'readonly_cdp_exception',
  'readonly_cdp_timeout',
  'readonly_cdp_transport_unavailable',
  'readonly_cdp_target_unavailable',
  'readonly_cdp_evaluation_failed'
])
const READONLY_CDP_TRANSIENT_REASON_CODE = 'readonly_cdp_execution_context_unavailable'
const READONLY_CDP_MAX_TRANSIENT_RETRIES = 1
const READONLY_CDP_RETRY_DELAY_MS = 50
export const READONLY_OBSERVATION_READ_ERROR_STAGES = Object.freeze([
  'api_access',
  'document_title',
  'composer_query',
  'primary_button_query',
  'thread_detail',
  'thread_summary',
  'provider_receipt_replay',
  'accepted_final_replay',
  'unknown'
])
const PLAN_OBSERVATION_PHASE_CODES = Object.freeze([
  'none',
  'provider_receipt_replay',
  'unknown'
])
const PLAN_OBSERVATION_OPERATION_CODES = Object.freeze([
  'none',
  'read',
  'unknown'
])
const READONLY_COMPOSER_MODE_PHASE_CODES = Object.freeze([
  'none',
  'initial_closed',
  'open',
  'closed_after_selection',
  'verified',
  'unknown'
])
const READONLY_COMPOSER_MODE_OPERATION_CODES = Object.freeze([
  'none',
  'read',
  'menu_click',
  'plan_click',
  'cleanup_click',
  'sleep',
  'unknown'
])
const READONLY_COMPOSER_MODE_READ_ERROR_STAGES = Object.freeze([
  'document_query',
  'dom_collection',
  'button_query',
  'geometry',
  'attribute',
  'unknown'
])
const READONLY_COMPOSER_GEOMETRY_READ_ERROR_STAGES = Object.freeze([
  'document_query',
  'dom_collection',
  'editor_query',
  'composer_query',
  'button_query',
  'geometry',
  'attribute',
  'text',
  'state',
  'unknown'
])
const COMPOSER_MODE_REASON_CODES = Object.freeze([
  '',
  'composer_mode_selection_failed',
  'composer_mode_menu_button_missing',
  'composer_plan_mode_control_missing',
  'composer_mode_menu_button_missing_after_selection',
  'composer_plan_mode_not_selected',
  'composer_agent_mode_not_selected',
  ...READONLY_CDP_FAILURE_REASON_CODES
])
const COMPOSER_SUBMIT_PHASE_CODES = Object.freeze([
  'none',
  'initial_idle',
  'editor_input',
  'post_input_ready',
  'primary_button',
  'unknown'
])
const COMPOSER_SUBMIT_OPERATION_CODES = Object.freeze([
  'none',
  'read',
  'editor_input_dispatch',
  'primary_button_dispatch',
  'sleep',
  'unknown'
])
const COMPOSER_SUBMIT_REASON_CODES = Object.freeze([
  '',
  'composer_submit_failed',
  'composer_idle_timeout_invalid',
  'composer_dom_element_missing',
  'composer_editor_not_empty',
  'composer_runtime_not_ready',
  'composer_primary_button_label_not_idle',
  'composer_primary_button_busy',
  'composer_input_not_observed',
  'composer_primary_button_not_idle',
  'composer_primary_button_not_ready',
  'composer_cdp_input_failed',
  ...READONLY_CDP_FAILURE_REASON_CODES
])
const TOOL_EXECUTION_OBSERVATION_FIELDS = [
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
].sort()
const REPOSITORY_ACCEPTANCE_CONTRACT = 'analytix.milestone-a.repository-acceptance.v1'
const EXTERNAL_REPOSITORY_CONTRACT =
  'analytix.milestone-a.real-repository-contract.v1'
const EXTERNAL_REPOSITORY_AUTHORITY_CONTRACT =
  'analytix.milestone-a.real-repository-authority.v1'
const EXTERNAL_REPOSITORY_PROVENANCE_CONTRACT =
  'analytix.milestone-a.external-repository-provenance.v1'
const TEST_RECEIPT_CONTRACT = 'analytix.milestone-a.external-test-receipt.v1'
// One final source line plus the terminal split slot from a trailing newline.
const EXTERNAL_REPOSITORY_LONG_CONTEXT_LAST_MARKER_MAX_TRAILING_LINE_SLOTS = 2
const PLAN_ARTIFACT_RELATIVE_DIR = '.analytixsdd/plan'
const PLAN_ARTIFACT_MAX_BYTES = 2 * 1024 * 1024
const RESEARCH_MARKER = 'MILESTONE_A_RESEARCH_OK'
const WRITING_MARKER = 'MILESTONE_A_WRITING_OK'
const FIXTURE_SOURCE_PATH = 'src/counter.mjs'
const FIXTURE_BROKEN_SOURCE = 'export function add(left, right) {\n  return left - right\n}\n'
const FIXTURE_REPAIRED_SOURCE = 'export function add(left, right) {\n  return left + right\n}\n'
const FIXTURE_TEST_SCRIPT = 'node --test test/counter.test.mjs'
const FIXTURE_PROTECTED_PATHS = [
  '.gitignore',
  'package.json',
  'README.md',
  'test/counter.test.mjs',
  'docs/acceptance-context.txt'
]
const FORMAL_MODEL = 'deepseek-v4-flash'
const FORMAL_REASONING_EFFORT = 'high'
const FORMAL_REASONING_LABEL = 'High'
const FORMAL_USER_GLOBAL_MAX_MODEL_STEPS = 32
const FORMAL_PLANNER_MAX_MODEL_STEPS = 12
const FORMAL_CHILD_MAX_MODEL_STEPS = 8
const FORMAL_CHILD_TIME_BUDGET_MS = 180_000
const PACKAGED_ORDINARY_MCP_SERVER_ID = 'gui_schedule'
const PACKAGED_ORDINARY_MCP_TOOL =
  'mcp__gui_schedule__gui_schedule_list'
const PACKAGED_ORDINARY_MCP_TOOL_COUNT = 8
const PACKAGED_SKILL_ID = 'analytix-computer-use'
const PACKAGED_READ_ONLY_SUBAGENT_PROFILE = 'milestone-a-readonly'
const PACKAGED_SUBAGENT_DELEGATION_TOOLS = [
  'task',
  'delegate_task',
  'parallel_tasks'
]
const PACKAGED_SKILL_SOURCE_RELATIVE_PATH =
  'vendor/analytix-computer-use/plugins/analytix-computer-use/skills/analytix-computer-use/SKILL.md'
const PACKAGED_SCHEDULE_CONFIG_SOURCE_RELATIVE_PATH =
  'src/main/claw-schedule-mcp-config.ts'
const PACKAGED_SCHEDULE_SERVER_SOURCE_RELATIVE_PATH =
  'src/main/claw-schedule-mcp-server.ts'
const ISOLATED_DISABLED_SKILL_DIRS = [
  'workspace-agents',
  'workspace-claude',
  'workspace-codex',
  'workspace-skills',
  'global-agents',
  'global-claude',
  'global-codex',
  'global-deepseek'
]
const ACCOUNT_FLOW_TOOL = 'mcp__analytix_funds__analyze_account_flows'
const BUNDLED_FUNDS_MATERIALIZATION_STATE_COMPONENTS = [
  '.state',
  'bundled-plugin-materialization',
  'v1'
]
const BUNDLED_FUNDS_MATERIALIZATION_AUTHORITY_COMPONENTS = [
  'private',
  'plugin-materialization-authority',
  'bundled-plugin-materialization-ed25519-v1.json'
]
const PROTECTED_FUNDS_REQUEST_PROMPT = [
  'Continue in this exact Agent and thread.',
  'Attempt a protected funds fact request for the current case without asking for, inventing, or displaying any complete account, card, identity, or phone value.',
  'Request inflow, outflow, net amount, transaction count, and evidence rows for the current protected account and current authorized time range.',
  'If an exact current case and DatasetSnapshot authority is absent, fail closed only for this protected fact effect through the typed host boundary.',
  'Do not use ordinary read, file, shell, Todo, Skill, subagent, or non-funds MCP tools for this request.'
].join(' ')
const DARWIN_EXPECT_PATH = '/usr/bin/expect'
const DARWIN_SECURITY_PATH = '/usr/bin/security'
const ISOLATED_LOGIN_KEYCHAIN_NAME = 'login.keychain'
const ISOLATED_KEYCHAIN_OPERATION_TIMEOUT_MS = 20_000
const MAX_SCREENSHOT_BYTES = 8 * 1024 * 1024
const REQUIRED_TOOL_GROUPS = {
  read: ['read', 'read_file'],
  plan: ['create_plan'],
  todo: ['todo_write', 'todo_ops', 'todo_patch'],
  write: ['write', 'write_file', 'edit', 'edit_file', 'multi_edit'],
  test: ['bash'],
  subagent: ['task'],
  skills: ['run_skill']
}
const REQUIRED_CHECK_IDS = [
  'trusted-cache-tmpdir',
  'formal-packaged-artifact',
  'configured-network-provider',
  'isolated-profile-and-repository',
  'packaged-first-launch',
  'composer-workflow-submit',
  'ordinary-agent-workflow',
  'case-dependencies-unavailable-with-ordinary-capabilities',
  'real-repository-test',
  'bounded-subagent',
  'git-skill-mcp-research-writing',
  'long-context-continuation',
  'composer-compaction-submit',
  'nonzero-compaction',
  'normal-first-quit',
  'fresh-packaged-relaunch',
  'relaunch-provider-continuation',
  'renderer-visual-evidence',
  'exact-thread-recovery',
  'exact-todo-recovery',
  'exact-subagent-recovery',
  'exact-compaction-recovery',
  'exact-result-recovery',
  'renderer-visible-thread-recovery',
  'renderer-visible-todo-recovery',
  'renderer-visible-subagent-recovery',
  'renderer-visible-compaction-recovery',
  'renderer-visible-result-recovery',
  'normal-final-quit',
  'zero-residual-processes',
  'credential-redaction',
  'sandbox-cleanup'
]
const REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS = [
  'formal-release-publication-authority',
  'controlled-release-native-receipt'
]
const PACKAGED_STARTUP_TRACE_CHECKPOINT_CODES = Object.freeze({
  'main module evaluated': 'main_module_evaluated',
  'legacy data migration barrier ready': 'legacy_data_migration_barrier_ready',
  'app icon loaded': 'app_icon_loaded',
  'single instance lock checked': 'single_instance_lock_checked',
  'app.whenReady:start': 'app_when_ready_start',
  'install webview guards:start': 'install_webview_guards_start',
  'install webview guards:done': 'install_webview_guards_done',
  'settings load:start': 'settings_load_start',
  'settings load:done': 'settings_load_done',
  'desktop private history migration:start':
    'desktop_private_history_migration_start',
  'desktop private history migration:done':
    'desktop_private_history_migration_done',
  'logger configured': 'logger_configured',
  'ipc registration:start': 'ipc_registration_start',
  'ipc registration:done': 'ipc_registration_done',
  'createWindow:start': 'create_window_start',
  'createWindow:load': 'create_window_load',
  'createWindow:returned': 'create_window_returned',
  'window:ready-to-show': 'window_ready_to_show',
  'window:did-finish-load': 'window_did_finish_load',
  'window:did-fail-load': 'window_did_fail_load',
  'window:startup-surface-ready': 'window_startup_surface_ready',
  'window:fallback-show-timeout': 'window_fallback_show_timeout'
})
const PACKAGED_STARTUP_TRACE_MAX_PARTIAL_LENGTH = 4096
const repositoryContextMarkers = new WeakMap()
const hostToolExecutionPrivateBindings = new WeakMap()
const externalHostToolExecutionArguments = new Set()
const packagedStartupTraceRecorders = new WeakMap()

function optionValue(name, fallback = '') {
  const inlinePrefix = `${name}=`
  for (let index = 0; index < rawArgs.length; index += 1) {
    const item = rawArgs[index]
    if (item.startsWith(inlinePrefix)) return item.slice(inlinePrefix.length)
    if (item === name && rawArgs[index + 1] && !rawArgs[index + 1].startsWith('--')) {
      return rawArgs[index + 1]
    }
  }
  return fallback
}

export function credentialEntryDeclaration(raw) {
  const method = typeof raw === 'string' ? raw.trim() : ''
  if (method === 'visible-human') {
    return Object.freeze({
      declared: true,
      method,
      automatedCredentialEntryUsed: false
    })
  }
  if (method === 'visible-computer-use') {
    return Object.freeze({
      declared: true,
      method,
      automatedCredentialEntryUsed: true
    })
  }
  if (method === 'visible-automation') {
    return Object.freeze({
      declared: false,
      method,
      automatedCredentialEntryUsed: true
    })
  }
  return Object.freeze({
    declared: false,
    method: 'undeclared',
    automatedCredentialEntryUsed: null
  })
}

function configuredCredentialEntryDeclaration() {
  return credentialEntryDeclaration(optionValue(
    '--credential-entry-method',
    process.env.ANALYTIX_RUNTIME_GO_MILESTONE_A_CREDENTIAL_ENTRY_METHOD || ''
  ))
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

function currentGitCommit() {
  const result = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return result.status === 0 ? String(result.stdout || '').trim().toLowerCase() : ''
}

function expectedPackagedSourceCommit() {
  return optionValue(
    '--expected-source-commit',
    process.env.ANALYTIX_RUNTIME_GO_PACKAGED_SOURCE_COMMIT || ''
  ).trim().toLowerCase()
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
  return resolve(process.cwd(), optionValue(
    '--app-path',
    process.env.ANALYTIX_RUNTIME_GO_MILESTONE_A_APP_PATH ||
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

function packagedAppUnpackedRoot(appPath, target) {
  return target.platform === 'darwin'
    ? join(appPath, 'Contents', 'Resources', 'app.asar.unpacked')
    : join(dirname(appPath), 'resources', 'app.asar.unpacked')
}

function bundledComputerUsePackageRoot(appPath, target) {
  return join(
    packagedAppUnpackedRoot(appPath, target),
    'packages',
    'runtime',
    'node_modules',
    'analytix-computer-use'
  )
}

function bundledComputerUseSkillRoot(appPath, target) {
  return join(
    bundledComputerUsePackageRoot(appPath, target),
    'plugins',
    'analytix-computer-use',
    'skills'
  )
}

function bundledComputerUseSkillPath(appPath, target) {
  return join(
    bundledComputerUseSkillRoot(appPath, target),
    PACKAGED_SKILL_ID,
    'SKILL.md'
  )
}

function repositorySourcePath(relativePath) {
  return resolve(process.cwd(), relativePath)
}

function sameFileIdentity(left, right) {
  return left.dev === right.dev &&
    left.ino === right.ino &&
    left.size === right.size &&
    left.mtimeMs === right.mtimeMs
}

function hashRegularFile(path, options = {}) {
  let before
  try {
    before = lstatSync(path)
  } catch {
    return { exists: false, regular: false, sha256: '', byteLength: 0, content: null }
  }
  const maximumBytes = options.maximumBytes || Number.MAX_SAFE_INTEGER
  if (!before.isFile() || before.isSymbolicLink() || before.size <= 0 || before.size > maximumBytes) {
    return {
      exists: true,
      regular: false,
      sha256: '',
      byteLength: Number(before.size || 0),
      content: null
    }
  }
  const fd = openSync(path, 'r')
  try {
    const opened = fstatSync(fd)
    if (!sameFileIdentity(before, opened)) {
      return { exists: true, regular: false, sha256: '', byteLength: opened.size, content: null }
    }
    const hash = createHash('sha256')
    const chunks = options.capture ? [] : null
    const buffer = Buffer.allocUnsafe(1024 * 1024)
    let total = 0
    while (true) {
      const read = readSync(fd, buffer, 0, buffer.length, null)
      if (read === 0) break
      const chunk = buffer.subarray(0, read)
      hash.update(chunk)
      if (chunks) chunks.push(Buffer.from(chunk))
      total += read
    }
    const after = fstatSync(fd)
    const pathAfter = lstatSync(path)
    if (!sameFileIdentity(opened, after) || !sameFileIdentity(after, pathAfter) || total !== after.size) {
      return { exists: true, regular: false, sha256: '', byteLength: total, content: null }
    }
    return {
      exists: true,
      regular: true,
      sha256: hash.digest('hex'),
      byteLength: total,
      content: chunks ? Buffer.concat(chunks, total) : null
    }
  } finally {
    closeSync(fd)
  }
}

export function fullTextReadLineLimit(content) {
  if (!Buffer.isBuffer(content) || content.length === 0) return 0
  const normalized = content.toString('utf8')
    .replaceAll('\r\n', '\n')
    .replaceAll('\r', '\n')
  return normalized.split('\n').length
}

function literalOccurrenceCount(value, needle) {
  if (typeof value !== 'string' || typeof needle !== 'string' || !needle) return 0
  let count = 0
  let offset = 0
  while (offset <= value.length - needle.length) {
    const found = value.indexOf(needle, offset)
    if (found < 0) break
    count += 1
    offset = found + 1
  }
  return count
}

export function externalRepositoryLongContextBindingEvidence(context, expected) {
  const base = {
    ok: false,
    lineLimit: 0,
    nonEmptyLineCount: 0,
    firstLineNumber: 0,
    completionLineNumber: 0,
    lastLineNumber: 0
  }
  if (!context?.regular || !Buffer.isBuffer(context.content) ||
    !expected || typeof expected !== 'object') return base
  const markers = [
    expected.firstMarker,
    expected.completionMarker,
    expected.lastMarker
  ]
  if (markers.some((marker) => typeof marker !== 'string' || !marker)) return base
  const text = context.content.toString('utf8')
    .replaceAll('\r\n', '\n')
    .replaceAll('\r', '\n')
  const lines = text.split('\n')
  const nonEmptyLines = lines
    .map((line, index) => ({ line, lineNumber: index + 1 }))
    .filter(({ line }) => line.trim().length > 0)
  const markerLineNumbers = markers.map((marker) => lines
    .flatMap((line, index) => line.includes(marker) ? [index + 1] : []))
  const firstLine = nonEmptyLines[0]
  const [firstMarkerLines, completionMarkerLines, lastMarkerLines] = markerLineNumbers
  const firstLineNumber = firstMarkerLines?.length === 1 ? firstMarkerLines[0] : 0
  const completionLineNumber = completionMarkerLines?.length === 1
    ? completionMarkerLines[0]
    : 0
  const lastLineNumber = lastMarkerLines?.length === 1 ? lastMarkerLines[0] : 0
  const lineLimit = lines.length
  const markersUnique = markers.every((marker) => literalOccurrenceCount(text, marker) === 1)
  const linesDistinctAndOrdered = firstLineNumber > 0 &&
    firstLineNumber < completionLineNumber && completionLineNumber < lastLineNumber
  const lastMarkerWithinBoundaryWindow = lastLineNumber > 0 &&
    lines.length - lastLineNumber <=
      EXTERNAL_REPOSITORY_LONG_CONTEXT_LAST_MARKER_MAX_TRAILING_LINE_SLOTS
  const ok = context.sha256 === expected.sha256 &&
    Number.isSafeInteger(context.byteLength) &&
    Number.isSafeInteger(expected.minimumBytes) &&
    context.byteLength >= expected.minimumBytes &&
    nonEmptyLines.length >= 3 && markersUnique && linesDistinctAndOrdered &&
    firstLineNumber === firstLine?.lineNumber &&
    lastMarkerWithinBoundaryWindow
  return {
    ...base,
    ok,
    lineLimit,
    nonEmptyLineCount: nonEmptyLines.length,
    firstLineNumber,
    completionLineNumber,
    lastLineNumber
  }
}

// A0's ordinary continuation is deliberately small and bounded.  Keep the
// legacy long-context binding above for parser-only historical fixtures, but
// formal acceptance uses this projection and never requires a 64 KiB read or
// synthetic three-marker echo.
export function continuationReadBindingEvidence(context, expected) {
  const base = {
    ok: false,
    byteLength: 0,
    lineLimit: 0,
    sha256: ''
  }
  if (!context?.regular || !Buffer.isBuffer(context.content) ||
      !expected || typeof expected !== 'object') return base
  const expectedHash = String(expected.sha256 || '')
  const lineLimit = Number(expected.lineLimit)
  const maxBytes = Number(expected.maxBytes)
  const normalized = context.content.toString('utf8')
    .replaceAll('\r\n', '\n')
    .replaceAll('\r', '\n')
  // The bounded continuation contract counts physical content lines and does
  // not treat the trailing newline's empty slot as another line.  The legacy
  // helper intentionally preserves that slot for historical fixtures.
  const actualLineLimit = normalized.endsWith('\n')
    ? Math.max(0, normalized.split('\n').length - 1)
    : normalized.split('\n').length
  const ok = /^[0-9a-f]{64}$/u.test(expectedHash) &&
    context.sha256 === expectedHash &&
    Number.isSafeInteger(context.byteLength) && context.byteLength > 0 &&
    Number.isSafeInteger(maxBytes) && maxBytes > 0 &&
    context.byteLength <= maxBytes &&
    Number.isSafeInteger(lineLimit) && lineLimit > 0 && lineLimit <= 4096 &&
    actualLineLimit === lineLimit
  return {
    ...base,
    ok,
    byteLength: context.byteLength,
    lineLimit: actualLineLimit,
    sha256: context.sha256
  }
}

export function boundedContinuationPrompt({ path = '', limit = 0 } = {}) {
  const safePath = safeRepositoryRelativePath(path)
  const safeLimit = Number(limit)
  if (!safePath || /case|host-owned|protected\s+funds/iu.test(safePath) ||
      !Number.isSafeInteger(safeLimit) || safeLimit <= 0 || safeLimit > 4096) return ''
  const argumentsJSON = JSON.stringify({ path: safePath, limit: safeLimit })
  return [
    'Continue an ordinary repository continuation in this exact thread.',
    `Make exactly one read tool call with exactly this JSON: ${argumentsJSON}.`,
    'Make no other tool call and use no delegation.',
    'Only after the read succeeds, return one concise natural-language confirmation grounded in that read.'
  ].join(' ')
}

export function packagedSkillSourceBindingEvidence(sourcePath, packagedPath) {
  const source = hashRegularFile(sourcePath)
  const packaged = hashRegularFile(packagedPath)
  const matched = source.regular && packaged.regular &&
    source.byteLength === packaged.byteLength &&
    source.sha256 === packaged.sha256
  return {
    ok: matched,
    sourceSha256: source.sha256,
    sourceByteLength: source.byteLength,
    packagedSha256: packaged.sha256,
    packagedByteLength: packaged.byteLength
  }
}

export function packagedScheduleSourceContractEvidence() {
  const config = hashRegularFile(
    repositorySourcePath(PACKAGED_SCHEDULE_CONFIG_SOURCE_RELATIVE_PATH),
    { capture: true, maximumBytes: 512 * 1024 }
  )
  const server = hashRegularFile(
    repositorySourcePath(PACKAGED_SCHEDULE_SERVER_SOURCE_RELATIVE_PATH),
    { capture: true, maximumBytes: 2 * 1024 * 1024 }
  )
  const configSource = config.content?.toString('utf8') || ''
  const serverSource = server.content?.toString('utf8') || ''
  const listStart = serverSource.indexOf('const registerListTool')
  const listEnd = serverSource.indexOf('const registerCreateTool', listStart)
  const listSource = listStart >= 0 && listEnd > listStart
    ? serverSource.slice(listStart, listEnd)
    : ''
  const strictEmptyInputSchemaBound =
    listSource.includes('inputSchema: z.strictObject({})')
  const nonMutatingListImplementationBound = [
    'registerListTool("gui_schedule_list")',
    'await postJson(options, "/schedule/internal/list", {})',
    'readOnlyHint: true',
    'destructiveHint: false',
    'idempotentHint: true',
    'openWorldHint: false'
  ].every((marker) => listSource.includes(marker))
  const packagedEntrypointBound =
    configSource.includes("GUI_SCHEDULE_MCP_SERVER_NAME = 'gui_schedule'") &&
    configSource.includes("GUI_SCHEDULE_MCP_NODE_ENTRY = 'out/main/claw-schedule-mcp-node-entry.js'")
  const ok = config.regular && server.regular &&
    strictEmptyInputSchemaBound &&
    nonMutatingListImplementationBound &&
    packagedEntrypointBound
  return {
    ok,
    configSourceSha256: config.sha256,
    serverSourceSha256: server.sha256,
    strictEmptyInputSchemaBound,
    nonMutatingListImplementationBound,
    packagedEntrypointBound
  }
}

function packagedScheduleHelperPath(appPath, target) {
  if (target.platform !== 'darwin') return executablePath(appPath, target)
  return join(
    appPath,
    'Contents',
    'Frameworks',
    'analytix Helper.app',
    'Contents',
    'MacOS',
    'analytix Helper'
  )
}

export function packagedScheduleMcpConfigEvidence({
  isolatedHome,
  appPath,
  target,
  schedulePort
}) {
  const configPath = join(isolatedHome, '.analytix', 'mcp.json')
  const configFile = hashRegularFile(configPath, {
    capture: true,
    maximumBytes: 512 * 1024
  })
  const helperPath = packagedScheduleHelperPath(appPath, target)
  const helperFile = hashRegularFile(helperPath)
  const entrypoint = join(
    appAsarPath(appPath, target),
    'out',
    'main',
    'claw-schedule-mcp-node-entry.js'
  )
  const expectedServer = {
    command: helperPath,
    args: [
      entrypoint,
      '--gui-schedule-mcp-server',
      '--base-url',
      `http://127.0.0.1:${schedulePort}`
    ],
    env: { ELECTRON_RUN_AS_NODE: '1' },
    url: null,
    connect_timeout: null,
    execute_timeout: null,
    read_timeout: null,
    disabled: false,
    enabled: true,
    required: false,
    enabled_tools: [],
    disabled_tools: []
  }
  const expected = {
    timeouts: {
      connect_timeout: 10,
      execute_timeout: 60,
      read_timeout: 120
    },
    servers: { [PACKAGED_ORDINARY_MCP_SERVER_ID]: expectedServer }
  }
  const expectedBytes = `${JSON.stringify(expected, null, 2)}\n`
  let configMode = 0
  let configLinkCount = 0
  try {
    const stat = lstatSync(configPath)
    configMode = stat.mode & 0o777
    configLinkCount = stat.nlink
  } catch {
    // The evidence remains fail-closed below.
  }
  const exactConfigBound = configFile.content?.toString('utf8') === expectedBytes
  const ok = configFile.regular && configMode === 0o600 && configLinkCount === 1 &&
    helperFile.regular && exactConfigBound
  return {
    ok,
    configSha256: configFile.sha256,
    configByteLength: configFile.byteLength,
    configMode,
    configLinkCount,
    helperExecutableSha256: helperFile.sha256,
    commandSha256: sha256(helperPath),
    entrypointSha256: sha256(entrypoint),
    argumentsDigest: sha256(canonicalJSON(expectedServer.args)),
    exactConfigBound
  }
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

function controlledCoreDispositionValid(appPath, target, authority) {
  if (!packagedAuthorityContract.isPackagedBuildAuthorityV2(authority) ||
      authority.nativeDisposition.kind !== 'core_controlled_release' || target.key !== 'darwin-arm64') return false
  try {
    require('./mac-notarize.cjs')._internals.verifyDataNativeAfterSign(
      packagedAuthorityContext(appPath, target), { requireDeveloperID: true, requireSecureTimestamp: true })
    return true
  } catch { return false }
}

function controlledNativeDispositionValid(appPath, target, authority) {
  if (authority?.nativeDisposition?.kind !== 'controlled_release_receipt') return false
  const receiptPath = join(runtimeResourcePath(appPath, target), 'analytix-native-components-receipt.json')
  const receipt = hashRegularFile(receiptPath, { maximumBytes: 1024 * 1024 })
  return receipt.regular &&
    authority.nativeDisposition.targetKey === target.key &&
    authority.nativeDisposition.receiptSha256 === receipt.sha256
}

export function captureCurrentWorktreeSnapshotEvidence(options = {}) {
  const repoRoot = options.repoRoot || process.cwd()
  const collectSnapshot = options.collectSnapshot ||
    packagedAuthorityContract.collectPackagedWorktreeSnapshotV1
  const validateSnapshot = options.validateSnapshot ||
    packagedAuthorityContract.isPackagedWorktreeSnapshotV1
  try {
    const snapshot = collectSnapshot(repoRoot)
    if (!snapshot || !validateSnapshot(snapshot)) {
      return {
        ok: false,
        blocked: true,
        blocker: 'current_worktree_snapshot_missing',
        snapshot: null,
        digest: '',
        classification: 'missing'
      }
    }
    return {
      ok: true,
      blocked: false,
      blocker: '',
      snapshot,
      digest: snapshot.snapshotDigest,
      classification: snapshot.state
    }
  } catch (error) {
    const unstable = error instanceof Error &&
      error.message.includes('Worktree changed while the package snapshot was captured')
    return {
      ok: false,
      blocked: true,
      blocker: unstable
        ? 'current_worktree_snapshot_unstable'
        : 'current_worktree_snapshot_unavailable',
      snapshot: null,
      digest: '',
      classification: unstable ? 'unstable' : 'unavailable'
    }
  }
}

export function packagedWorktreeSnapshotBindingEvidence(currentEvidence, authority) {
  const packagedSnapshot = authority?.worktreeSnapshot
  const packagedDigest = typeof packagedSnapshot?.snapshotDigest === 'string'
    ? packagedSnapshot.snapshotDigest
    : ''
  const packagedClassification = packagedSnapshot &&
    (packagedSnapshot.state === 'clean' || packagedSnapshot.state === 'dirty')
    ? packagedSnapshot.state
    : packagedSnapshot
      ? 'invalid'
      : 'missing'
  const evidence = {
    ok: false,
    blocker: '',
    matched: false,
    current: {
      digest: typeof currentEvidence?.digest === 'string' ? currentEvidence.digest : '',
      classification: typeof currentEvidence?.classification === 'string'
        ? currentEvidence.classification
        : 'missing'
    },
    packaged: {
      digest: packagedDigest,
      classification: packagedClassification,
      authorityClassification: typeof authority?.classification === 'string'
        ? authority.classification
        : 'missing'
    }
  }
  if (currentEvidence?.ok !== true) {
    return {
      ...evidence,
      blocker: currentEvidence?.blocker || 'current_worktree_snapshot_missing'
    }
  }
  if (!packagedSnapshot || !packagedDigest) {
    return {
      ...evidence,
      blocker: 'packaged_build_authority_worktree_snapshot_missing'
    }
  }
  if (!/^[0-9a-f]{64}$/.test(packagedDigest) || packagedClassification === 'invalid') {
    return {
      ...evidence,
      blocker: 'packaged_build_authority_worktree_snapshot_invalid'
    }
  }
  if (packagedDigest !== currentEvidence.digest) {
    return {
      ...evidence,
      blocker: 'packaged_build_authority_worktree_snapshot_mismatch'
    }
  }
  return { ...evidence, ok: true, matched: true }
}

function formalPackagedArtifactEvidence(appPath, target, expectedCommit) {
  const currentWorktreeSnapshot = captureCurrentWorktreeSnapshotEvidence()
  const appExecutable = executablePath(appPath, target)
  const runtimeServer = runtimeServerPath(appPath, target)
  const asarPath = appAsarPath(appPath, target)
  const packagedSkillPath = bundledComputerUseSkillPath(appPath, target)
  const sourceSkillPath = repositorySourcePath(PACKAGED_SKILL_SOURCE_RELATIVE_PATH)
  const authorityPath = join(runtimeResourcePath(appPath, target), PACKAGED_BUILD_AUTHORITY_FILE)
  const executableFile = hashRegularFile(appExecutable)
  const runtimeFile = hashRegularFile(runtimeServer)
  const asarFile = hashRegularFile(asarPath)
  const skillSourceBinding = packagedSkillSourceBindingEvidence(
    sourceSkillPath,
    packagedSkillPath
  )
  const scheduleSourceContract = packagedScheduleSourceContractEvidence()
  const authorityFile = hashRegularFile(authorityPath, {
    capture: true,
    maximumBytes: 256 * 1024
  })
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    appPathHash: sha256(appPath),
    targetKey: target.key,
    sourceCommit: '',
    authoritySha256: authorityFile.sha256,
    authorityDigest: '',
    executableSha256: executableFile.sha256,
    runtimeServerSha256: runtimeFile.sha256,
    appAsarSha256: asarFile.sha256,
    sourceSkillSha256: skillSourceBinding.sourceSha256,
    packagedSkillSha256: skillSourceBinding.packagedSha256,
    packagedSkillSourceMatched: skillSourceBinding.ok,
    packagedScheduleConfigSourceSha256: scheduleSourceContract.configSourceSha256,
    packagedScheduleServerSourceSha256: scheduleSourceContract.serverSourceSha256,
    packagedScheduleSourceContractBound: scheduleSourceContract.ok,
    packagedScheduleStrictEmptyInputSchemaBound:
      scheduleSourceContract.strictEmptyInputSchemaBound,
    packagedScheduleNonMutatingListImplementationBound:
      scheduleSourceContract.nonMutatingListImplementationBound,
    codeSignatureVerified: false,
    nativeDispositionKind: '',
    developmentNativeDisposition: false,
    controlledReleaseNativeReceipt: false,
    controlledCoreQualification: false,
    worktreeSnapshotBinding: packagedWorktreeSnapshotBindingEvidence(
      currentWorktreeSnapshot,
      null
    ),
    bundledRuntimeGoSourcePresent: existsSync(bundledRuntimeGoSourcePath(appPath, target))
  }
  if (!existsSync(appPath)) {
    return { ...base, blocked: true, blocker: 'formal_packaged_app_missing' }
  }
  if (!executableFile.regular || !runtimeFile.regular || !asarFile.regular ||
    !skillSourceBinding.sourceSha256 || !skillSourceBinding.packagedSha256) {
    return { ...base, blocker: 'formal_packaged_artifact_incomplete' }
  }
  if (base.bundledRuntimeGoSourcePresent) {
    return { ...base, blocker: 'packaged_runtime_go_source_present' }
  }
  if (!/^[0-9a-f]{40}$/.test(expectedCommit)) {
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
  const sourceCommit = String(authority?.sourceCommit || '').trim().toLowerCase()
  const authorityDigest = String(authority?.authorityDigest || '')
  const worktreeSnapshotBinding = packagedWorktreeSnapshotBindingEvidence(
    currentWorktreeSnapshot,
    authority
  )
  const canonical = authorityFile.content.toString('utf8') === JSON.stringify(authority)
  const shapeValid = authority?.contract === PACKAGED_BUILD_AUTHORITY_CONTRACT &&
    packagedAuthorityContract.isPackagedBuildAuthorityV2(authority)
  let executableBinding = null
  let runtimeBinding = null
  try {
    const authorityTarget = {
      format: target.platform === 'darwin' ? 'mach-o' : target.platform === 'win32' ? 'pe' : 'elf',
      arch: target.arch
    }
    executableBinding = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
      appExecutable,
      authorityTarget
    )
    runtimeBinding = packagedAuthorityContract.signingInvariantNativeArtifactBinding(
      runtimeServer,
      authorityTarget
    )
  } catch {
    executableBinding = null
    runtimeBinding = null
  }
  const contentBindingMatches = (binding, observed) =>
    binding &&
    Object.keys(binding).sort().join(',') === 'byteLength,sha256' &&
    binding.sha256 === observed.sha256 &&
    binding.byteLength === observed.byteLength
  const nativeBindingMatches = (binding, observed) =>
    binding &&
    observed &&
    binding.payloadSha256 === observed.payloadSha256 &&
    binding.payloadByteLength === observed.payloadByteLength &&
    binding.format === observed.format &&
    binding.arch === observed.arch
  const controlledReleaseNativeReceipt =
    controlledNativeDispositionValid(appPath, target, authority)
  const nativeDispositionKind = String(authority?.nativeDisposition?.kind || '')
  const developmentNativeDisposition = nativeDispositionKind === 'development_non_publishable'
  let codeSignatureVerified = target.platform !== 'darwin'
  if (target.platform === 'darwin') {
    const codeSign = spawnSync('/usr/bin/codesign', [
      '--verify',
      '--deep',
      '--strict',
      '--verbose=2',
      appPath
    ], {
      cwd: process.cwd(),
      encoding: 'utf8',
      stdio: 'pipe'
    })
    codeSignatureVerified = codeSign.status === 0
  }
  const valid = currentWorktreeSnapshot.ok &&
    worktreeSnapshotBinding.ok &&
    canonical &&
    shapeValid &&
    authority.targetKey === target.key &&
    sourceCommit === expectedCommit &&
    nativeBindingMatches(authority?.artifacts?.executable, executableBinding) &&
    contentBindingMatches(authority?.artifacts?.appAsar, asarFile) &&
    nativeBindingMatches(authority?.artifacts?.runtimeServer, runtimeBinding) &&
    skillSourceBinding.ok &&
    scheduleSourceContract.ok &&
    codeSignatureVerified
  let blocker = ''
  if (!valid) {
    if (!currentWorktreeSnapshot.ok) {
      blocker = currentWorktreeSnapshot.blocker
    } else if (
      worktreeSnapshotBinding.blocker ===
      'packaged_build_authority_worktree_snapshot_missing'
    ) {
      blocker = worktreeSnapshotBinding.blocker
    } else if (!shapeValid || !canonical) {
      blocker = 'packaged_build_authority_invalid'
    } else if (!worktreeSnapshotBinding.ok) {
      blocker = worktreeSnapshotBinding.blocker
    } else if (sourceCommit !== expectedCommit) {
      blocker = 'packaged_build_authority_source_commit_mismatch'
    } else if (!skillSourceBinding.ok) {
      blocker = 'packaged_skill_source_binding_mismatch'
    } else if (!scheduleSourceContract.ok) {
      blocker = 'packaged_schedule_source_contract_invalid'
    } else if (!codeSignatureVerified) {
      blocker = 'formal_package_code_signature_invalid'
    } else {
      blocker = 'packaged_build_authority_artifact_binding_mismatch'
    }
  }
  return {
    ...base,
    ok: valid,
    blocked: !currentWorktreeSnapshot.ok,
    blocker,
    sourceCommit,
    authorityDigest,
    codeSignatureVerified,
    nativeDispositionKind,
    developmentNativeDisposition,
    controlledReleaseNativeReceipt,
    controlledCoreQualification: controlledCoreDispositionValid(appPath, target, authority),
    worktreeSnapshotBinding
  }
}

const FORMAL_ARTIFACT_REVALIDATION_FIELDS = [
  'appAsarSha256',
  'authorityDigest',
  'authoritySha256',
  'codeSignatureVerified',
  'executableSha256',
  'packagedScheduleConfigSourceSha256',
  'packagedScheduleServerSourceSha256',
  'packagedScheduleSourceContractBound',
  'packagedSkillSha256',
  'packagedSkillSourceMatched',
  'runtimeServerSha256',
  'sourceSkillSha256',
  'sourceCommit',
  'targetKey'
]

export function packagedArtifactRevalidationEvidence(baseline, observed, phase = '') {
  const stableFields = FORMAL_ARTIFACT_REVALIDATION_FIELDS.every((field) =>
    observed?.[field] === baseline?.[field]
  )
  const worktreeStable = observed?.worktreeSnapshotBinding?.ok === true &&
    baseline?.worktreeSnapshotBinding?.ok === true &&
    observed.worktreeSnapshotBinding.current.digest ===
      baseline.worktreeSnapshotBinding.current.digest &&
    observed.worktreeSnapshotBinding.packaged.digest ===
      baseline.worktreeSnapshotBinding.packaged.digest
  const ok = baseline?.ok === true && observed?.ok === true && stableFields && worktreeStable
  return {
    phase,
    ok,
    blocker: ok
      ? ''
      : observed?.blocker ||
        (stableFields ? 'formal_package_worktree_changed_between_revalidations' :
          'formal_package_artifact_changed_between_revalidations'),
    codeSignatureVerified: observed?.codeSignatureVerified === true,
    stableFields,
    worktreeStable,
    observedAuthoritySha256: String(observed?.authoritySha256 || ''),
    observedAuthorityDigest: String(observed?.authorityDigest || ''),
    observedExecutableSha256: String(observed?.executableSha256 || ''),
    observedRuntimeServerSha256: String(observed?.runtimeServerSha256 || ''),
    observedAppAsarSha256: String(observed?.appAsarSha256 || '')
  }
}

function nonlocalHTTPSURL(value) {
  let url
  try {
    url = new URL(String(value || '').trim())
  } catch {
    return null
  }
  const hostname = url.hostname.toLowerCase()
  const localHost = hostname === 'localhost' ||
    hostname === '0.0.0.0' ||
    hostname === '::1' ||
    hostname.endsWith('.localhost') ||
    /^127\./.test(hostname) ||
    /^10\./.test(hostname) ||
    /^192\.168\./.test(hostname) ||
    /^169\.254\./.test(hostname) ||
    /^172\.(1[6-9]|2\d|3[01])\./.test(hostname)
  return url.protocol === 'https:' && !localHost ? url : null
}

export function milestoneALocalProvider(observation) {
  return localProviderFromObservation(observation, { requiredModel: FORMAL_MODEL })
}

function trustedCacheTempRoot() {
  const configured = String(process.env.TMPDIR || '').trim()
  if (!configured || !isAbsolute(configured)) {
    return { ok: false, blocker: 'analytix_cache_tmpdir_missing', path: '' }
  }
  let resolvedPath
  let resolvedMount
  let resolvedPathStat
  let resolvedMountStat
  try {
    resolvedPath = realpathSync(configured)
    resolvedMount = realpathSync(CACHE_MOUNT)
    accessSync(resolvedPath, constants.W_OK)
    resolvedPathStat = statSync(resolvedPath)
    resolvedMountStat = statSync(resolvedMount)
    if (!resolvedPathStat.isDirectory() || !resolvedMountStat.isDirectory()) {
      throw new Error('not_directory')
    }
  } catch {
    return { ok: false, blocker: 'analytix_cache_tmpdir_unavailable_or_unwritable', path: '' }
  }
  const escaped = relative(resolvedMount, resolvedPath)
  if (escaped === '..' || escaped.startsWith(`..${sep}`) || isAbsolute(escaped)) {
    return { ok: false, blocker: 'analytix_cache_tmpdir_outside_trusted_mount', path: '' }
  }
  const deviceBinding = trustedCacheDeviceBindingEvidence(
    resolvedMountStat,
    [resolvedPathStat]
  )
  if (!deviceBinding.ok) {
    return { ok: false, blocker: 'analytix_cache_tmpdir_device_mismatch', path: '' }
  }
  return { ok: true, blocker: '', path: resolvedPath, pathHash: sha256(resolvedPath) }
}

export function trustedCacheDeviceBindingEvidence(cacheStat, candidateStats = []) {
  const cacheDevice = Number(cacheStat?.dev)
  const candidateDevices = Array.isArray(candidateStats)
    ? candidateStats.map((item) => Number(item?.dev))
    : []
  if (!Number.isSafeInteger(cacheDevice) || cacheDevice < 0 ||
      candidateDevices.length === 0 ||
      candidateDevices.some((device) => !Number.isSafeInteger(device) || device < 0)) {
    return Object.freeze({
      ok: false,
      reasonCode: 'trusted_cache_device_identity_invalid'
    })
  }
  if (candidateDevices.some((device) => device !== cacheDevice)) {
    return Object.freeze({
      ok: false,
      reasonCode: 'trusted_cache_device_mismatch'
    })
  }
  return Object.freeze({ ok: true, reasonCode: 'trusted_cache_device_bound' })
}

function writeFixtureRepository(workspace) {
  const sourceDir = join(workspace, 'src')
  const testDir = join(workspace, 'test')
  const docsDir = join(workspace, 'docs')
  mkdirSync(sourceDir, { recursive: true, mode: 0o700 })
  mkdirSync(testDir, { recursive: true, mode: 0o700 })
  mkdirSync(docsDir, { recursive: true, mode: 0o700 })
  writeFileSync(join(workspace, '.gitignore'), '.analytixsdd/\n', {
    encoding: 'utf8',
    mode: 0o600
  })
  writeFileSync(join(workspace, 'package.json'), `${JSON.stringify({
    name: 'analytix-milestone-a-isolated-repo',
    private: true,
    type: 'module',
    scripts: {
      test: FIXTURE_TEST_SCRIPT
    }
  }, null, 2)}\n`, { encoding: 'utf8', mode: 0o600 })
  writeFileSync(
    join(workspace, 'README.md'),
    '# Counter repair\n\nFix `add` so the existing test passes. Keep the change minimal.\n',
    { encoding: 'utf8', mode: 0o600 }
  )
  writeFileSync(
    join(sourceDir, 'counter.mjs'),
    FIXTURE_BROKEN_SOURCE,
    { encoding: 'utf8', mode: 0o600 }
  )
  writeFileSync(
    join(testDir, 'counter.test.mjs'),
    `import test from 'node:test'\n` +
      `import assert from 'node:assert/strict'\n` +
      `import { add } from '../src/counter.mjs'\n\n` +
      `test('adds two numbers', () => {\n` +
      `  assert.equal(add(2, 3), 5)\n` +
      `  assert.equal(add(-2, 3), 1)\n` +
      `  assert.equal(add(0, 0), 0)\n` +
      `})\n`,
    { encoding: 'utf8', mode: 0o600 }
  )
  const contextMarkers = Object.freeze({
    first: `MILESTONE_A_CONTEXT_FIRST_${randomBytes(24).toString('hex')}`,
    completion: `MILESTONE_A_CONTEXT_OK_${randomBytes(24).toString('hex')}`,
    last: `MILESTONE_A_CONTEXT_LAST_${randomBytes(24).toString('hex')}`
  })
  const contextLines = Array.from({ length: 512 }, (_, index) => {
    const ordinal = String(index + 1).padStart(4, '0')
    const boundary = index === 0
      ? contextMarkers.first
      : index === 255
        ? contextMarkers.completion
        : index === 511
          ? contextMarkers.last
          : `MILESTONE_A_CONTEXT_ROW_${ordinal}`
    return `${boundary} preserve the verified plan, Todo completion, source repair, parent-owned test cross-check, host-owned tool observation binding, bounded subagent result, restart recovery, and normal shutdown evidence.\n`
  })
  writeFileSync(
    join(docsDir, 'acceptance-context.txt'),
    contextLines.join(''),
    { encoding: 'utf8', mode: 0o600 }
  )
  return contextMarkers
}

function gitCapture(workspace, gitArgs) {
  const result = spawnSync('git', gitArgs, {
    cwd: workspace,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  return {
    ok: result.status === 0,
    status: result.status,
    stdout: String(result.stdout || ''),
    stderr: String(result.stderr || '')
  }
}

function exactObjectKeys(value, expected) {
  return value && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).sort().join(',') === [...expected].sort().join(',')
}

function safeRepositoryRelativePath(value) {
  if (typeof value !== 'string' || !value || value !== value.trim() ||
    /[\0\r\n]/u.test(value) || value.includes('\\') || isAbsolute(value)) return ''
  const normalized = value.split('/').filter((part) => part && part !== '.').join('/')
  if (normalized !== value || normalized === '.git' || normalized.startsWith('.git/') ||
    value.split('/').some((part) => part === '..')) return ''
  return normalized
}

function shellQuotedArgument(value) {
  return `'${String(value).replaceAll("'", `'"'"'`)}'`
}

function cacheHelperCommandPrefix() {
  return `source ${shellQuotedArgument(repositorySourcePath(
    CACHE_HELPER_SOURCE_RELATIVE_PATH
  ))} && `
}

function contractTestCommand(test) {
  return `${cacheHelperCommandPrefix()}${[
    test.executable,
    ...test.arguments
  ].map(shellQuotedArgument).join(' ')}`
}

function pathIsOutside(root, candidate) {
  const escaped = relative(root, candidate)
  return escaped === '..' || escaped.startsWith(`..${sep}`) || isAbsolute(escaped)
}

function ownerUID() {
  return typeof process.getuid === 'function' ? process.getuid() : null
}

function ownerOnlyDirectory(path, exactOwnerOnly = false) {
  try {
    const stat = lstatSync(path)
    const uid = ownerUID()
    return stat.isDirectory() && !stat.isSymbolicLink() &&
      (uid === null || stat.uid === uid) &&
      (exactOwnerOnly ? (stat.mode & 0o777) === 0o700 : (stat.mode & 0o022) === 0)
  } catch {
    return false
  }
}

function ownerPrivateRegularFile(path) {
  try {
    const stat = lstatSync(path)
    const uid = ownerUID()
    return stat.isFile() && !stat.isSymbolicLink() && stat.nlink === 1 &&
      (uid === null || stat.uid === uid) && (stat.mode & 0o022) === 0
  } catch {
    return false
  }
}

function stableOwnerPrivateFile(path, maximumBytes = 64 * 1024) {
  let before
  try {
    before = lstatSync(path)
  } catch {
    return {
      exists: false,
      regular: false,
      content: null,
      sha256: sha256(Buffer.alloc(0)),
      byteLength: 0
    }
  }
  const uid = ownerUID()
  if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 ||
    (uid !== null && before.uid !== uid) || (before.mode & 0o022) !== 0 ||
    before.size < 0 || before.size > maximumBytes) {
    return { exists: true, regular: false, content: null, sha256: '', byteLength: before.size }
  }
  const fd = openSync(path, 'r')
  try {
    const opened = fstatSync(fd)
    if (!sameFileIdentity(before, opened)) {
      return { exists: true, regular: false, content: null, sha256: '', byteLength: opened.size }
    }
    const content = readFileSync(fd)
    const after = fstatSync(fd)
    const pathAfter = lstatSync(path)
    if (!sameFileIdentity(opened, after) || !sameFileIdentity(after, pathAfter) ||
      content.length !== after.size) {
      return { exists: true, regular: false, content: null, sha256: '', byteLength: content.length }
    }
    return {
      exists: true,
      regular: true,
      content,
      sha256: sha256(content),
      byteLength: content.length,
      identity: {
        dev: opened.dev,
        ino: opened.ino,
        uid: opened.uid,
        mode: opened.mode,
        nlink: opened.nlink
      }
    }
  } finally {
    closeSync(fd)
  }
}

function effectiveGitExcludePatterns(content) {
  if (!Buffer.isBuffer(content) || content.includes(0)) return null
  return content.toString('utf8').split(/\r?\n/u).flatMap((line) => {
    const trimmed = line.trim()
    return !trimmed || trimmed.startsWith('#') ? [] : [line]
  })
}

function gitCheckIgnoreEvidence(workspace, relativePath, probes, excludeRule) {
  const result = spawnSync('git', [
    '-c', 'core.excludesFile=/dev/null',
    'check-ignore', '-v', '--no-index', '--', relativePath, ...probes
  ], {
    cwd: workspace,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  const lines = String(result.stdout || '').split(/\r?\n/u).filter(Boolean)
  const fields = lines.length === 1 ? lines[0].split('\t') : []
  const metadata = fields[0] || ''
  return {
    exactTargetIgnored: result.status === 0 && fields[1] === relativePath &&
      metadata.includes('.git/info/exclude:') && metadata.endsWith(`:${excludeRule}`),
    siblingProbesVisible: lines.length === 1
  }
}

export function milestoneAPlanInspectPaths(authority) {
  const inspectPaths = Array.isArray(authority?.inspectPaths) ? authority.inspectPaths : []
  const contextPath = typeof authority?.context?.path === 'string'
    ? authority.context.path
    : ''
  return contextPath
    ? inspectPaths.filter((path) => path !== contextPath)
    : [...inspectPaths]
}

export function milestoneAChildReviewPaths(authority) {
  const expected = Array.isArray(authority?.expectedChangedFiles)
    ? authority.expectedChangedFiles
    : []
  const changedPaths = expected
    .map((item) => typeof item?.path === 'string' ? item.path : '')
    .filter(Boolean)
  const inspectPaths = milestoneAPlanInspectPaths(authority)
  const paths = [...new Set([...changedPaths, ...inspectPaths])]
  return changedPaths.length === expected.length && paths.length > 0
    ? paths
    : []
}

export function milestoneAChildReviewPrompt(authority) {
  const paths = milestoneAChildReviewPaths(authority)
  return [
    'Review only the final contract-authorized source change and test intent.',
    `The exact user request is: ${authority?.taskRequest || ''}`,
    `Call read exactly once for each contract-authorized source or task-inspection path, with only its relative path and no optional offset or limit: ${paths
      .map((path) => JSON.stringify(path)).join(', ')}.`,
    `Assess those source bytes against this exact required test command without executing it: ${authority?.testCommand || ''}`,
    'Do not mutate state. Do not run commands. Do not delegate or inspect any other path.'
  ].join(' ')
}

export function milestoneAFormalExecutionBounds() {
  return Object.freeze({
    userGlobalMaxModelSteps: FORMAL_USER_GLOBAL_MAX_MODEL_STEPS,
    plannerMaxModelSteps: FORMAL_PLANNER_MAX_MODEL_STEPS,
    childMaxModelSteps: FORMAL_CHILD_MAX_MODEL_STEPS,
    childTimeBudgetMs: FORMAL_CHILD_TIME_BUDGET_MS
  })
}

export function milestoneAChildTaskArguments(authority) {
  const bounds = milestoneAFormalExecutionBounds()
  return Object.freeze({
    prompt: milestoneAChildReviewPrompt(authority),
    profile: PACKAGED_READ_ONLY_SUBAGENT_PROFILE,
    toolPolicy: 'readOnly',
    max_steps: bounds.childMaxModelSteps,
    time_budget_ms: bounds.childTimeBudgetMs,
    run_in_background: false
  })
}

export function milestoneAWorkflowPrompt(repositoryAuthority, skillId) {
  const childTaskArguments = milestoneAChildTaskArguments(repositoryAuthority)
  return [
    'Continue in this exact thread and work only in the pre-existing isolated non-case code repository.',
    `Implement this user request exactly: ${repositoryAuthority.taskRequest}`,
    'Use a thread Todo list to execute the recorded plan.',
    `Modify only these contract-authorized files: ${repositoryAuthority.expectedChangedFiles
      .map((item) => JSON.stringify(item.path)).join(', ')}; do not stage, commit, rewrite Git history, or modify any other repository path.`,
    `Call foreground bash exactly once with this exact test command and no optional arguments: ${repositoryAuthority.testCommand}`,
    `Separately call foreground bash exactly once with this exact Git command and no optional arguments: ${repositoryAuthority.gitCheckCommand}`,
    'Make the real repository test pass; do not create or edit harness evidence.',
    `Use run_skill exactly once with the packaged Skill ${JSON.stringify(skillId)} to load its instructions as Skill evidence; run_skill is not delegation, must not create a child, and must not cause any extra shell command.`,
    `Use task exactly once as the only child subagent. Pass exactly this JSON object as its arguments, without adding, removing, or renaming fields: ${JSON.stringify(childTaskArguments)}. Wait for its foreground completion.`,
    `Call the exact package-bound non-mutating list canary ${JSON.stringify(PACKAGED_ORDINARY_MCP_TOOL)} exactly once with an empty object to read the isolated profile's real scheduled-task state; do not call any other MCP tool and never call a funds MCP tool.`,
    'Finish the Todo list.',
    'Finish with a concise evidence-backed research note and a natural-language implementation summary grounded only in completed tool outcomes.',
    `Only after every required tool has completed successfully, include these exact completion markers in the final response: ${RESULT_MARKER} ${RESEARCH_MARKER} ${WRITING_MARKER}. Do not emit any marker when a required tool fails.`,
    'Do not run /compact; the acceptance harness will invoke compaction separately.'
  ].join(' ')
}

export function milestoneAChildReviewReadSetEvidence(
  authority,
  thread,
  childTurnId,
  expectedPathObservationMatches
) {
  const expectedPaths = milestoneAChildReviewPaths(authority)
  const attempts = toolCallAttempts(thread).filter((item) => item.turnId === childTurnId)
  const readAttempts = attempts.filter((item) => item.toolName === 'read')
  const matches = Array.isArray(expectedPathObservationMatches)
    ? expectedPathObservationMatches
    : []
  const exactReadOnlyInventory = expectedPaths.length > 0 &&
    attempts.length === expectedPaths.length &&
    readAttempts.length === expectedPaths.length
  const expectedPathsBound = matches.length === expectedPaths.length &&
    matches.every((value) => value === true)
  return Object.freeze({
    ok: exactReadOnlyInventory && expectedPathsBound,
    expectedReadCount: expectedPaths.length,
    toolAttemptCount: attempts.length,
    readAttemptCount: readAttempts.length,
    exactReadOnlyInventory,
    expectedPathsBound,
    toolAttemptDigest: sha256(canonicalJSON(attempts))
  })
}

export function milestoneAPlanReadSetEvidence(
  authority,
  evidence,
  expectedPathObservationMatches
) {
  const expectedPaths = milestoneAPlanInspectPaths(authority)
  const attempts = Array.isArray(evidence?.resultTurnToolCallAttempts)
    ? evidence.resultTurnToolCallAttempts
    : []
  const readAttempts = attempts.filter((item) => item?.toolName === 'read')
  const matches = Array.isArray(expectedPathObservationMatches)
    ? expectedPathObservationMatches
    : []
  const exactReadAttemptCount = expectedPaths.length > 0 &&
    readAttempts.length === expectedPaths.length
  const expectedPathsBound = matches.length === expectedPaths.length &&
    matches.every((value) => value === true)
  const contextPath = typeof authority?.context?.path === 'string'
    ? authority.context.path
    : ''
  const longContextReadAbsent = !contextPath || (
    !expectedPaths.includes(contextPath) &&
    exactReadAttemptCount &&
    expectedPathsBound
  )
  return Object.freeze({
    ok: exactReadAttemptCount && expectedPathsBound && longContextReadAbsent,
    expectedReadCount: expectedPaths.length,
    readAttemptCount: readAttempts.length,
    expectedPathsBound,
    exactReadAttemptCount,
    longContextReadAbsent,
    readAttemptDigest: sha256(canonicalJSON(readAttempts))
  })
}

export function milestoneAPlanPrompt(authority) {
  const inspectPaths = milestoneAPlanInspectPaths(authority)
  return [
    'Work only in this pre-existing isolated non-case code repository and inspect the task through tools.',
    `The user request is: ${authority.taskRequest}`,
    `Call read exactly once for each contract-required task inspection path, with only its relative path and no optional offset or limit: ${inspectPaths
      .map((path) => JSON.stringify(path)).join(', ')}.`,
    'Do not read the contract-bound continuation file in this planning turn; the acceptance harness validates it in a dedicated later bounded continuation.',
    'Use no tools other than those exact read calls and one create_plan call.',
    'Do not call list, search, code index, web fetch, bash, Todo, subagent/delegation, skill, MCP, or read any other path.',
    'Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.',
    'Complete those exact read calls before recording the plan.',
    'Call create_plan exactly once after those reads complete.',
    'Record a concrete implementation and verification plan at the GUI-reserved plan path.',
    'After create_plan returns successfully, end the turn immediately without another tool call.',
    'If every required read succeeds, do not send a final response or the completion marker until create_plan has returned successfully.',
    `In the final response include the exact marker ${PLAN_MARKER}.`,
    'Do not edit files, run the test, delegate, or run /compact in this planning turn.'
  ].join(' ')
}

// Keep this byte-for-byte equivalent to src/shared/gui-plan.ts. The packaged
// renderer reserves the plan path from the complete composer request before
// the Go runtime sees the turn, so the harness must bind that same path.
export function milestoneAPlanFeatureName(request) {
  const normalized = request
    .normalize('NFKC')
    .toLowerCase()
    .split('')
    .map((char) => (char.charCodeAt(0) < 32 ? ' ' : char))
    .join('')
    .replace(/[<>:"|?*\\/]+/gu, ' ')
    .replace(/[\s_]+/gu, '-')
    .replace(/-+/gu, '-')
    .replace(/^[.\-\s]+/u, '')
    .replace(/[.\-\s]+$/u, '')
  const safe = normalized || 'plan'
  return safe.slice(0, 96).replace(/[.\-\s]+$/u, '') || 'plan'
}

export function milestoneAPlanRelativePath(authority) {
  return `${PLAN_ARTIFACT_RELATIVE_DIR}/${milestoneAPlanFeatureName(
    milestoneAPlanPrompt(authority)
  )}.md`
}

function milestoneAPlanExcludeRule(relativePath) {
  return `/${relativePath}`
}

function milestoneAPlanExcludeProbes(relativePath) {
  const stem = relativePath.replace(/\.md$/iu, '')
  return [
    `${stem}-extra.md`,
    `${PLAN_ARTIFACT_RELATIVE_DIR}/other.md`,
    '.analytixsdd/other.md',
    PLAN_ARTIFACT_RELATIVE_DIR
  ]
}

function milestoneAPlanExcludeEvidence(authority) {
  const base = {
    ok: false,
    excludeSha256: '',
    excludeByteLength: 0,
    exactRuleCount: 0,
    exactTargetIgnored: false,
    siblingProbesVisible: false
  }
  const expectedPath = join(authority?.gitCommonDir || '', 'info', 'exclude')
  const expectedRelativePath = String(authority?.planArtifactRelativePath || '')
  const expectedRule = milestoneAPlanExcludeRule(expectedRelativePath)
  if (!authority || safeRepositoryRelativePath(expectedRelativePath) !== expectedRelativePath ||
    dirname(expectedRelativePath) !== PLAN_ARTIFACT_RELATIVE_DIR ||
    authority.planArtifactExcludeRule !== expectedRule ||
    authority.planArtifactExcludePath !== expectedPath ||
    pathIsOutside(authority.gitCommonDir, expectedPath) ||
    !ownerOnlyDirectory(dirname(expectedPath))) return base
  const observed = stableOwnerPrivateFile(expectedPath)
  const patterns = observed.regular ? effectiveGitExcludePatterns(observed.content) : null
  const exactRuleCount = Array.isArray(patterns)
    ? patterns.filter((pattern) => pattern === expectedRule).length
    : 0
  const ignore = gitCheckIgnoreEvidence(
    authority.workspace,
    expectedRelativePath,
    milestoneAPlanExcludeProbes(expectedRelativePath),
    expectedRule
  )
  const exactTargetIgnored = ignore.exactTargetIgnored
  const siblingProbesVisible = ignore.siblingProbesVisible
  const ok = observed.regular && (observed.identity.mode & 0o777) === 0o600 &&
    observed.sha256 === authority.planArtifactExcludeSha256 &&
    observed.byteLength === authority.planArtifactExcludeByteLength &&
    patterns?.length === 1 && exactRuleCount === 1 &&
    exactTargetIgnored && siblingProbesVisible
  return {
    ...base,
    ok,
    excludeSha256: observed.sha256,
    excludeByteLength: observed.byteLength,
    exactRuleCount,
    exactTargetIgnored,
    siblingProbesVisible
  }
}

function milestoneAPlanArtifactPathEvidence(workspace, relativePath) {
  const metadataDir = join(workspace, '.analytixsdd')
  const planDir = join(metadataDir, 'plan')
  const artifactPath = relativePath
    ? join(workspace, ...String(relativePath).split('/'))
    : ''
  const optionalDirectorySafe = (path) => {
    try {
      const stat = lstatSync(path)
      return stat.isDirectory() && !stat.isSymbolicLink() &&
        (ownerUID() === null || stat.uid === ownerUID()) &&
        (stat.mode & 0o022) === 0 && realpathSync(path) === path
    } catch (error) {
      return error?.code === 'ENOENT'
    }
  }
  if (!artifactPath || !optionalDirectorySafe(metadataDir) || !optionalDirectorySafe(planDir)) {
    return { safe: false, present: false, planEntryCount: 0 }
  }
  let planEntryCount = 0
  try {
    planEntryCount = readdirSync(planDir).length
  } catch (error) {
    if (error?.code !== 'ENOENT') return { safe: false, present: false, planEntryCount: 0 }
  }
  try {
    const artifactStat = lstatSync(artifactPath)
    return {
      safe: artifactStat.isFile() && !artifactStat.isSymbolicLink() &&
        artifactStat.nlink === 1 && ownerPrivateRegularFile(artifactPath) &&
        realpathSync(artifactPath) === artifactPath,
      present: true,
      planEntryCount
    }
  } catch (error) {
    return { safe: error?.code === 'ENOENT', present: false, planEntryCount }
  }
}

function inspectMilestoneAPlanArtifactExcludeInput(workspace, gitCommonDir, relativePath) {
  if (safeRepositoryRelativePath(relativePath) !== relativePath ||
    dirname(relativePath) !== PLAN_ARTIFACT_RELATIVE_DIR ||
    !relativePath.toLowerCase().endsWith('.md')) {
    throw new Error('external_repository_plan_artifact_path_invalid')
  }
  const excludeRule = milestoneAPlanExcludeRule(relativePath)
  const planPath = milestoneAPlanArtifactPathEvidence(workspace, relativePath)
  if (!planPath.safe) {
    throw new Error('external_repository_plan_artifact_path_unsafe')
  }
  if (planPath.present) {
    throw new Error('external_repository_plan_artifact_preexists')
  }
  if (planPath.planEntryCount !== 0) {
    throw new Error('external_repository_plan_directory_not_empty')
  }
  const excludePath = join(gitCommonDir, 'info', 'exclude')
  if (pathIsOutside(gitCommonDir, excludePath) || !ownerOnlyDirectory(dirname(excludePath))) {
    throw new Error('external_repository_plan_exclude_parent_invalid')
  }
  const before = stableOwnerPrivateFile(excludePath)
  if (before.exists && !before.regular) {
    throw new Error('external_repository_plan_exclude_unsafe')
  }
  const beforeContent = before.content || Buffer.alloc(0)
  const beforePatterns = effectiveGitExcludePatterns(beforeContent)
  if (!Array.isArray(beforePatterns) || beforePatterns.length !== 0) {
    throw new Error('external_repository_plan_exclude_preconfigured')
  }
  const separator = beforeContent.length > 0 && beforeContent.at(-1) !== 0x0a ? '\n' : ''
  const afterContent = Buffer.concat([
    beforeContent,
    Buffer.from(`${separator}${excludeRule}\n`, 'utf8')
  ])
  const authorityFields = Object.freeze({
    planArtifactRelativePath: relativePath,
    planArtifactExcludeRule: excludeRule,
    planArtifactExcludePath: excludePath,
    planArtifactExcludeBeforeSha256: before.sha256,
    planArtifactExcludeBeforeByteLength: before.byteLength,
    planArtifactExcludeSha256: sha256(afterContent),
    planArtifactExcludeByteLength: afterContent.length
  })
  return Object.freeze({
    before,
    afterContent,
    authorityFields
  })
}

function installMilestoneAPlanArtifactExclude(workspace, gitCommonDir, relativePath) {
  const inspected = inspectMilestoneAPlanArtifactExcludeInput(
    workspace,
    gitCommonDir,
    relativePath
  )
  const { before, afterContent, authorityFields } = inspected
  const { planArtifactExcludePath: excludePath } = authorityFields
  const flags = constants.O_WRONLY | (constants.O_NOFOLLOW || 0) |
    (before.exists ? 0 : constants.O_CREAT | constants.O_EXCL)
  const fd = openSync(excludePath, flags, 0o600)
  try {
    const opened = fstatSync(fd)
    if (!opened.isFile() || opened.nlink !== 1 ||
      (ownerUID() !== null && opened.uid !== ownerUID()) || (opened.mode & 0o022) !== 0 ||
      (before.exists && (opened.dev !== before.identity.dev || opened.ino !== before.identity.ino))) {
      throw new Error('external_repository_plan_exclude_identity_changed')
    }
    fchmodSync(fd, 0o600)
    ftruncateSync(fd, 0)
    writeFileSync(fd, afterContent)
    fsyncSync(fd)
    const written = fstatSync(fd)
    const pathAfter = lstatSync(excludePath)
    if (!written.isFile() || written.nlink !== 1 ||
      (ownerUID() !== null && written.uid !== ownerUID()) ||
      (written.mode & 0o777) !== 0o600 || written.size !== afterContent.length ||
      written.dev !== opened.dev || written.ino !== opened.ino ||
      pathAfter.dev !== written.dev || pathAfter.ino !== written.ino ||
      pathAfter.size !== written.size || pathAfter.nlink !== written.nlink) {
      throw new Error('external_repository_plan_exclude_write_changed')
    }
  } finally {
    closeSync(fd)
  }
  if (!milestoneAPlanExcludeEvidence({
    workspace,
    gitCommonDir,
    ...authorityFields
  }).ok) {
    throw new Error('external_repository_plan_exclude_binding_failed')
  }
  return authorityFields
}

function canonicalNonlocalRepositoryOrigin(value) {
  const url = nonlocalHTTPSURL(value)
  if (!url || url.username || url.password || url.search || url.hash) return ''
  return url.toString()
}

export function parseMilestoneAExternalRepositoryProvenance(value) {
  if (!exactObjectKeys(value, [
    'admissionKind', 'baselineCommit', 'baselineTree', 'contract', 'originUrl'
  ]) || value.contract !== EXTERNAL_REPOSITORY_PROVENANCE_CONTRACT ||
    value.admissionKind !== 'independent-pre-admission' ||
    !/^[0-9a-f]{40,64}$/.test(String(value.baselineCommit || '')) ||
    !/^[0-9a-f]{40,64}$/.test(String(value.baselineTree || ''))) return null
  const originUrl = canonicalNonlocalRepositoryOrigin(value.originUrl)
  if (!originUrl || originUrl !== value.originUrl) return null
  return Object.freeze({
    contract: EXTERNAL_REPOSITORY_PROVENANCE_CONTRACT,
    admissionKind: 'independent-pre-admission',
    originUrl,
    baselineCommit: value.baselineCommit,
    baselineTree: value.baselineTree
  })
}

export function verifyFreshExternalRepositoryOrigin(provenance, options = {}) {
  const base = {
    ok: false,
    blocker: '',
    originUrlSha256: '',
    baselineCommit: '',
    baselineTree: '',
    fetchExitCode: null,
    fetchAttemptCount: 0,
    fetchExitCodes: [],
    cleanupSucceeded: false
  }
  const parsed = parseMilestoneAExternalRepositoryProvenance(provenance)
  if (!parsed) return { ...base, blocker: 'external_repository_provenance_invalid' }
  const cache = options.trustedCacheTempRoot?.() || trustedCacheTempRoot()
  if (!cache.ok) return { ...base, blocker: cache.blocker }
  const runner = options.spawnSync || spawnSync
  const fetchExitCodes = []
  let evidence = {
    ...base,
    blocker: 'external_repository_origin_probe_failed',
    originUrlSha256: sha256(parsed.originUrl)
  }
  let allCleanupSucceeded = true
  for (let attempt = 1; attempt <= 2; attempt += 1) {
    let probeRoot = ''
    let retryTransport = false
    try {
      probeRoot = mkdtempSync(join(cache.path, 'analytix-milestone-a-origin-probe-'))
      chmodSync(probeRoot, 0o700)
      const repository = join(probeRoot, 'repository.git')
      const initialize = runner('git', ['init', '--bare', '--quiet', repository], {
        cwd: probeRoot,
        encoding: 'utf8',
        stdio: 'pipe',
        timeout: 30_000
      })
      if (initialize.status !== 0) {
        evidence = {
          ...evidence,
          blocker: 'external_repository_origin_probe_initialization_failed'
        }
      } else {
        const fetchResult = runner('git', [
          '-C', repository,
          '-c', 'credential.helper=',
          'fetch', '--quiet', '--no-tags', '--depth=1', '--',
          parsed.originUrl, parsed.baselineCommit
        ], {
          cwd: probeRoot,
          encoding: 'utf8',
          stdio: 'pipe',
          timeout: 120_000,
          env: {
            PATH: String(process.env.PATH || '/usr/bin:/bin:/usr/sbin:/sbin'),
            HOME: probeRoot,
            GIT_CONFIG_NOSYSTEM: '1',
            GIT_TERMINAL_PROMPT: '0',
            LC_ALL: 'C'
          }
        })
        const fetchExitCode = Number.isInteger(fetchResult.status)
          ? fetchResult.status
          : null
        fetchExitCodes.push(fetchExitCode)
        if (fetchResult.status !== 0) {
          evidence = {
            ...evidence,
            blocker: 'external_repository_origin_fetch_failed',
            fetchExitCode,
            fetchAttemptCount: fetchExitCodes.length,
            fetchExitCodes: [...fetchExitCodes]
          }
          retryTransport = attempt < 2
        } else {
          const commit = runner('git', ['-C', repository, 'rev-parse', 'FETCH_HEAD^{commit}'], {
            cwd: probeRoot, encoding: 'utf8', stdio: 'pipe', timeout: 30_000
          })
          const tree = runner('git', ['-C', repository, 'rev-parse', 'FETCH_HEAD^{tree}'], {
            cwd: probeRoot, encoding: 'utf8', stdio: 'pipe', timeout: 30_000
          })
          const observedCommit = String(commit.stdout || '').trim().toLowerCase()
          const observedTree = String(tree.stdout || '').trim().toLowerCase()
          const ok = commit.status === 0 && tree.status === 0 &&
            observedCommit === parsed.baselineCommit && observedTree === parsed.baselineTree
          evidence = {
            ...evidence,
            ok,
            blocker: ok ? '' : 'external_repository_origin_commit_or_tree_mismatch',
            baselineCommit: observedCommit,
            baselineTree: observedTree,
            fetchExitCode,
            fetchAttemptCount: fetchExitCodes.length,
            fetchExitCodes: [...fetchExitCodes]
          }
        }
      }
    } catch {
      evidence = {
        ...evidence,
        ok: false,
        blocker: 'external_repository_origin_probe_failed',
        fetchAttemptCount: fetchExitCodes.length,
        fetchExitCodes: [...fetchExitCodes]
      }
    } finally {
      let cleanupSucceeded = true
      if (probeRoot) {
        try {
          rmSync(probeRoot, { recursive: true, force: true })
          cleanupSucceeded = !existsSync(probeRoot)
        } catch {
          cleanupSucceeded = false
        }
      }
      allCleanupSucceeded = allCleanupSucceeded && cleanupSucceeded
      evidence.cleanupSucceeded = allCleanupSucceeded
    }
    if (!allCleanupSucceeded || !retryTransport) break
  }
  if (!evidence.cleanupSucceeded) {
    return { ...evidence, ok: false, blocker: 'external_repository_origin_probe_cleanup_failed' }
  }
  return evidence
}

function externalRepositoryFixtureSubstitute(workspace) {
  const packageFile = hashRegularFile(join(workspace, 'package.json'), {
    capture: true,
    maximumBytes: 256 * 1024
  })
  let packageName = ''
  if (packageFile.regular && packageFile.content) {
    try {
      packageName = String(JSON.parse(packageFile.content.toString('utf8'))?.name || '')
    } catch {
      packageName = ''
    }
  }
  const source = hashRegularFile(join(workspace, FIXTURE_SOURCE_PATH), {
    capture: true,
    maximumBytes: 64 * 1024
  })
  const readme = hashRegularFile(join(workspace, 'README.md'), {
    capture: true,
    maximumBytes: 256 * 1024
  })
  return packageName === 'analytix-milestone-a-isolated-repo' ||
    source.content?.toString('utf8') === FIXTURE_BROKEN_SOURCE ||
    source.content?.toString('utf8') === FIXTURE_REPAIRED_SOURCE ||
    readme.content?.toString('utf8').startsWith('# Counter repair\n') === true
}

export function externalRepositoryContractShapeValid(contract) {
  if (!contract || typeof contract !== 'object' || Array.isArray(contract) ||
      contract.contract !== EXTERNAL_REPOSITORY_CONTRACT) return false
  const hasContinuationRead = Object.prototype.hasOwnProperty.call(contract, 'continuationRead')
  const hasLegacyLongContext = Object.prototype.hasOwnProperty.call(contract, 'longContext')
  const contractKeys = hasContinuationRead
    ? ['contract', 'continuationRead', 'repository', 'task', 'test']
    : ['contract', 'longContext', 'repository', 'task', 'test']
  if ((!hasContinuationRead && !hasLegacyLongContext) ||
      !exactObjectKeys(contract, contractKeys)) return false
  if (!exactObjectKeys(contract.repository, [
    'baselineCommit', 'baselineTree', 'kind', 'minimumCommitCount', 'originUrlSha256'
  ]) || contract.repository.kind !== 'preexisting-isolated-real-code-repository' ||
    !/^[0-9a-f]{40,64}$/.test(String(contract.repository.baselineCommit || '')) ||
    !/^[0-9a-f]{40,64}$/.test(String(contract.repository.baselineTree || '')) ||
    !/^[0-9a-f]{64}$/.test(String(contract.repository.originUrlSha256 || '')) ||
    !Number.isSafeInteger(contract.repository.minimumCommitCount) ||
    contract.repository.minimumCommitCount < 2) return false
  if (!exactObjectKeys(contract.task, [
    'expectedChangedFiles', 'inspectPaths', 'request'
  ]) || typeof contract.task.request !== 'string' ||
    contract.task.request !== contract.task.request.trim() ||
    contract.task.request.length < 20 || contract.task.request.length > 4000 ||
    !Array.isArray(contract.task.inspectPaths) || contract.task.inspectPaths.length < 2 ||
    contract.task.inspectPaths.length > 32 ||
    !Array.isArray(contract.task.expectedChangedFiles) ||
    contract.task.expectedChangedFiles.length < 1 ||
    contract.task.expectedChangedFiles.length > 16) return false
  const inspectPaths = contract.task.inspectPaths.map(safeRepositoryRelativePath)
  if (inspectPaths.some((path) => !path) || new Set(inspectPaths).size !== inspectPaths.length) {
    return false
  }
  const changedPaths = []
  for (const item of contract.task.expectedChangedFiles) {
    const hasEquivalentSha256s = Object.prototype.hasOwnProperty.call(
      item || {},
      'equivalentSha256s'
    )
    if (!exactObjectKeys(item, hasEquivalentSha256s
      ? ['path', 'sha256', 'equivalentSha256s']
      : ['path', 'sha256'])) return false
    const path = safeRepositoryRelativePath(item.path)
    if (!path || !/^[0-9a-f]{64}$/.test(String(item.sha256 || ''))) return false
    const equivalentSha256s = hasEquivalentSha256s ? item.equivalentSha256s : []
    if (hasEquivalentSha256s && (!Array.isArray(equivalentSha256s) ||
        equivalentSha256s.length < 1 || equivalentSha256s.length > 3 ||
        equivalentSha256s.some((value) => !/^[0-9a-f]{64}$/.test(String(value))) ||
        new Set(equivalentSha256s).size !== equivalentSha256s.length ||
        equivalentSha256s.includes(item.sha256))) return false
    changedPaths.push(path)
  }
  if (new Set(changedPaths).size !== changedPaths.length ||
    changedPaths.some((path) => !inspectPaths.includes(path) || [
      '.gitignore', '.gitattributes', '.gitmodules'
    ].includes(path))) return false
  if (!exactObjectKeys(contract.test, ['arguments', 'executable', 'timeoutMs']) ||
    typeof contract.test.executable !== 'string' ||
    !/^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$/u.test(contract.test.executable) ||
    !Array.isArray(contract.test.arguments) || contract.test.arguments.length < 1 ||
    contract.test.arguments.length > 64 ||
    contract.test.arguments.some((argument) => typeof argument !== 'string' ||
      !argument || argument.length > 512 || /[\0\r\n]/u.test(argument)) ||
    contractTestCommand(contract.test).length > 4096 ||
    !Number.isSafeInteger(contract.test.timeoutMs) || contract.test.timeoutMs < 1000 ||
    contract.test.timeoutMs > 10 * 60 * 1000) return false
  if (hasContinuationRead) {
    const continuation = contract.continuationRead
    if (!exactObjectKeys(continuation, [
      'lineLimit', 'maxBytes', 'path', 'sha256', 'successMarker'
    ])) return false
    const contextPath = safeRepositoryRelativePath(continuation.path)
    const marker = continuation.successMarker
    return Boolean(contextPath && inspectPaths.includes(contextPath) &&
      !changedPaths.includes(contextPath) &&
      /^[0-9a-f]{64}$/.test(String(continuation.sha256 || '')) &&
      Number.isSafeInteger(continuation.lineLimit) && continuation.lineLimit > 0 &&
      continuation.lineLimit <= 4096 && Number.isSafeInteger(continuation.maxBytes) &&
      continuation.maxBytes > 0 && continuation.maxBytes <= 256 * 1024 &&
      typeof marker === 'string' && marker === marker.trim() &&
      marker.length >= 8 && marker.length <= 256 && !/[\0\r\n]/u.test(marker) &&
      !contract.task.request.includes(marker))
  }
  const context = contract.longContext
  if (!exactObjectKeys(context, [
    'completionMarker', 'firstMarker', 'lastMarker', 'minimumBytes', 'path', 'sha256'
  ])) return false
  const contextPath = safeRepositoryRelativePath(context.path)
  const markers = [context.firstMarker, context.completionMarker, context.lastMarker]
  return Boolean(contextPath && inspectPaths.includes(contextPath) &&
    !changedPaths.includes(contextPath) &&
    /^[0-9a-f]{64}$/.test(String(context.sha256 || '')) &&
    Number.isSafeInteger(context.minimumBytes) && context.minimumBytes >= 64 * 1024 &&
    context.minimumBytes <= 2 * 1024 * 1024 &&
    markers.every((marker) => typeof marker === 'string' && marker === marker.trim() &&
      marker.length >= 8 && marker.length <= 512 && !/[\0\r\n]/u.test(marker)) &&
    new Set(markers).size === markers.length &&
    markers.every((marker) => !contract.task.request.includes(marker)))
}

function externalRepositoryStatusMatches(status, expectedChangedFiles, phase) {
  if (!status.ok) return false
  if (phase === 'baseline') return status.stdout === ''
  const entries = status.stdout.split('\0').filter(Boolean)
  const expected = new Set(expectedChangedFiles.map((item) => ` M ${item.path}`))
  return entries.length === expected.size && entries.every((entry) => expected.has(entry))
}

function externalRepositoryExpectedChangedFileShapeValid(file) {
  const acceptedSha256s = file?.acceptedSha256s
  return exactObjectKeys(file, ['path', 'sha256', 'acceptedSha256s', 'baselineSha256']) &&
    safeRepositoryRelativePath(file.path) === file.path &&
    /^[0-9a-f]{64}$/.test(String(file.sha256 || '')) &&
    /^[0-9a-f]{64}$/.test(String(file.baselineSha256 || '')) &&
    Array.isArray(acceptedSha256s) && acceptedSha256s.length >= 1 &&
    acceptedSha256s.length <= 4 && acceptedSha256s[0] === file.sha256 &&
    acceptedSha256s.every((value) => /^[0-9a-f]{64}$/.test(String(value))) &&
    new Set(acceptedSha256s).size === acceptedSha256s.length &&
    !acceptedSha256s.includes(file.baselineSha256)
}

function externalRepositoryAuthorityExpectedChangedFilesBound(authority) {
  let contractFile
  try {
    contractFile = hashRegularFile(authority.contractPath, {
      capture: true,
      maximumBytes: 1024 * 1024
    })
  } catch {
    return false
  }
  if (!contractFile.regular || !contractFile.content ||
      contractFile.sha256 !== authority.contractSha256) return false
  let contract
  try {
    contract = JSON.parse(contractFile.content.toString('utf8'))
  } catch {
    return false
  }
  if (!externalRepositoryContractShapeValid(contract) ||
      contract.task.expectedChangedFiles.length !== authority.expectedChangedFiles.length) {
    return false
  }
  return contract.task.expectedChangedFiles.every((file, index) => {
    const actual = authority.expectedChangedFiles[index]
    const equivalentSha256s = Array.isArray(file.equivalentSha256s)
      ? file.equivalentSha256s
      : []
    const acceptedSha256s = [file.sha256, ...equivalentSha256s]
    return actual.path === file.path &&
      actual.acceptedSha256s.length === acceptedSha256s.length &&
      actual.acceptedSha256s.every((value, valueIndex) =>
        value === acceptedSha256s[valueIndex]
      )
  })
}

function externalRepositoryAuthorityShapeValid(authority) {
  return authority?.contract === EXTERNAL_REPOSITORY_AUTHORITY_CONTRACT &&
    authority.mode === 'external-preexisting-real-repository' &&
    (authority.acceptanceClass === 'synthetic-parser-only' ||
      authority.acceptanceClass === 'formal-fresh-origin-verified') &&
    typeof authority.workspace === 'string' && authority.workspace &&
    /^[0-9a-f]{64}$/.test(String(authority.workspaceSha256 || '')) &&
    typeof authority.ownerRoot === 'string' && authority.ownerRoot &&
    /^[0-9a-f]{64}$/.test(String(authority.ownerRootSha256 || '')) &&
    typeof authority.contractPath === 'string' && authority.contractPath &&
    /^[0-9a-f]{64}$/.test(String(authority.contractSha256 || '')) &&
    typeof authority.provenancePath === 'string' && authority.provenancePath &&
    /^[0-9a-f]{64}$/.test(String(authority.provenanceSha256 || '')) &&
    typeof authority.gitCommonDir === 'string' && authority.gitCommonDir &&
    /^[0-9a-f]{40,64}$/.test(String(authority.baselineCommit || '')) &&
    /^[0-9a-f]{40,64}$/.test(String(authority.baselineTree || '')) &&
    /^[0-9a-f]{64}$/.test(String(authority.originUrlSha256 || '')) &&
    Array.isArray(authority.inspectPaths) && authority.inspectPaths.length >= 2 &&
    Array.isArray(authority.expectedChangedFiles) && authority.expectedChangedFiles.length >= 1 &&
    authority.expectedChangedFiles.every(externalRepositoryExpectedChangedFileShapeValid) &&
    externalRepositoryAuthorityExpectedChangedFilesBound(authority) &&
    typeof authority.testExecutable === 'string' &&
    Array.isArray(authority.testArguments) && authority.testArguments.length >= 1 &&
    typeof authority.testCommand === 'string' &&
    authority.testCommand.startsWith(cacheHelperCommandPrefix()) &&
    authority.testCommand === contractTestCommand({
      executable: authority.testExecutable,
      arguments: authority.testArguments
    }) &&
    Number.isSafeInteger(authority.testTimeoutMs) &&
    authority.context && typeof authority.taskRequest === 'string' &&
    typeof authority.gitCheckCommand === 'string' && authority.gitCheckCommand &&
    authority.planArtifactRelativePath === milestoneAPlanRelativePath(authority) &&
    authority.planArtifactExcludeRule ===
      milestoneAPlanExcludeRule(authority.planArtifactRelativePath) &&
    typeof authority.planArtifactExcludePath === 'string' &&
    authority.planArtifactExcludePath === join(authority.gitCommonDir, 'info', 'exclude') &&
    /^[0-9a-f]{64}$/.test(String(authority.planArtifactExcludeBeforeSha256 || '')) &&
    Number.isSafeInteger(authority.planArtifactExcludeBeforeByteLength) &&
    authority.planArtifactExcludeBeforeByteLength >= 0 &&
    /^[0-9a-f]{64}$/.test(String(authority.planArtifactExcludeSha256 || '')) &&
    Number.isSafeInteger(authority.planArtifactExcludeByteLength) &&
    authority.planArtifactExcludeByteLength > 0 &&
    authority.freshOriginVerified ===
      (authority.acceptanceClass === 'formal-fresh-origin-verified')
}

export function verifyMilestoneAExternalRepositoryAcceptance(
  authority,
  phase = 'final',
  options = {}
) {
  const preflightOnly = options.preflightOnly === true
  const base = {
    ok: false,
    blocker: '',
    phase,
    gitRepository: false,
    workspaceBound: false,
    baselineCommitBound: false,
    baselineTreeBound: false,
    protectedInputsBound: false,
    protectedInputsWriteProtected: false,
    validatorOutsideRepository: true,
    validatorBound: false,
    acceptanceContractBound: false,
    onlyIntendedSourceChanged: false,
    expectedSourceBound: false,
    expectedSourceFileCount: 0,
    expectedSourceMismatchCount: 0,
    expectedSourceDigest: '',
    observedSourceDigest: '',
    gitStatusSha256: '',
    protectedInputsDigest: '',
    sourceSha256: '',
    validatorSha256: '',
    contractSha256: '',
    provenanceSha256: '',
    baselineHistoryBound: false,
    originBound: false,
    fixtureSubstituteRejected: false,
    formalAcceptance: false,
    freshOriginVerified: false,
    ownerRootBound: false,
    ownerOnlyIsolationBound: false,
    gitCommonDirContained: false,
    symlinkHardlinkIsolationBound: false,
    provenanceBound: false,
    planArtifactExcludeBound: false,
    planArtifactPathSafe: false
  }
  if (!externalRepositoryAuthorityShapeValid(authority)) {
    return { ...base, blocker: 'external_repository_authority_invalid' }
  }
  if ((phase !== 'baseline' && phase !== 'final') || (preflightOnly && phase !== 'baseline')) {
    return { ...base, blocker: 'external_repository_acceptance_phase_invalid' }
  }
  let workspace
  let contractPath
  try {
    workspace = realpathSync(authority.workspace)
    contractPath = realpathSync(authority.contractPath)
  } catch {
    return { ...base, blocker: 'external_repository_path_unavailable' }
  }
  const planExcludeBeforeStatus = preflightOnly
    ? null
    : milestoneAPlanExcludeEvidence(authority)
  const planPathBeforeStatus = milestoneAPlanArtifactPathEvidence(
    workspace,
    authority.planArtifactRelativePath
  )
  const topLevel = gitCapture(workspace, ['rev-parse', '--show-toplevel'])
  const head = gitCapture(workspace, ['rev-parse', 'HEAD'])
  const tree = gitCapture(workspace, ['rev-parse', 'HEAD^{tree}'])
  const history = gitCapture(workspace, ['rev-list', '--count', 'HEAD'])
  const origin = gitCapture(workspace, ['remote', 'get-url', 'origin'])
  const commonDir = gitCapture(workspace, ['rev-parse', '--git-common-dir'])
  const status = gitCapture(workspace, [
    'status', '--porcelain=v1', '-z', '--untracked-files=all'
  ])
  const contractFile = hashRegularFile(contractPath, {
    capture: true,
    maximumBytes: 1024 * 1024
  })
  const provenanceFile = hashRegularFile(authority.provenancePath, {
    capture: true,
    maximumBytes: 256 * 1024
  })
  const expectedFiles = authority.expectedChangedFiles.map((expected) => {
    const observed = hashRegularFile(join(workspace, expected.path))
    return {
      path: expected.path,
      baselineSha256: expected.baselineSha256,
      expectedSha256: expected.sha256,
      acceptedSha256s: expected.acceptedSha256s,
      observedSha256: observed.sha256,
      regular: observed.regular
    }
  })
  const inspectedFiles = authority.inspectPaths.map((path) => {
    const observed = hashRegularFile(join(workspace, path))
    return { path, regular: observed.regular, sha256: observed.sha256 }
  })
  const context = hashRegularFile(join(workspace, authority.context.path), {
    capture: true,
    maximumBytes: authority.context.deprecated === true
      ? 2 * 1024 * 1024
      : authority.context.maxBytes
  })
  const contextBound = authority.context.deprecated === true
    ? externalRepositoryLongContextBindingEvidence(context, authority.context).ok
    : continuationReadBindingEvidence(context, authority.context).ok
  const gitRepository = topLevel.ok && topLevel.stdout.trim() === workspace
  const workspaceBound = workspace === authority.workspace &&
    sha256(workspace) === authority.workspaceSha256
  const baselineCommitBound = head.ok && head.stdout.trim() === authority.baselineCommit
  const baselineTreeBound = tree.ok && tree.stdout.trim() === authority.baselineTree
  const baselineHistoryBound = history.ok && Number(history.stdout.trim()) >= authority.minimumCommitCount
  const originBound = Boolean(origin.ok && origin.stdout.trim() &&
    sha256(origin.stdout.trim()) === authority.originUrlSha256)
  let observedCommonDir = ''
  try {
    observedCommonDir = commonDir.ok
      ? realpathSync(resolve(workspace, commonDir.stdout.trim()))
      : ''
  } catch {
    observedCommonDir = ''
  }
  let observedOwnerRoot = ''
  let observedCacheRoot = ''
  try {
    observedOwnerRoot = realpathSync(authority.ownerRoot)
    observedCacheRoot = realpathSync(CACHE_MOUNT)
  } catch {
    observedOwnerRoot = ''
    observedCacheRoot = ''
  }
  const ownerRootBound = authority.ownerRoot === observedOwnerRoot &&
    sha256(authority.ownerRoot) === authority.ownerRootSha256 &&
    !pathIsOutside(authority.ownerRoot, workspace) &&
    Boolean(observedCacheRoot) && !pathIsOutside(observedCacheRoot, authority.ownerRoot)
  const gitCommonDirContained = observedCommonDir === authority.gitCommonDir &&
    !pathIsOutside(workspace, observedCommonDir) && ownerOnlyDirectory(observedCommonDir)
  const ownerOnlyIsolationBound = ownerOnlyDirectory(authority.ownerRoot, true) &&
    ownerOnlyDirectory(workspace) && ownerOnlyDirectory(observedCommonDir)
  const symlinkHardlinkIsolationBound = ownerPrivateRegularFile(contractPath) &&
    ownerPrivateRegularFile(authority.provenancePath) &&
    inspectedFiles.every((file) => ownerPrivateRegularFile(join(workspace, file.path))) &&
    authority.expectedChangedFiles.every((file) =>
      ownerPrivateRegularFile(join(workspace, file.path)))
  const provenanceBound = provenanceFile.regular &&
    provenanceFile.sha256 === authority.provenanceSha256
  const planExcludeAfterStatus = preflightOnly
    ? null
    : milestoneAPlanExcludeEvidence(authority)
  const planPathAfterStatus = milestoneAPlanArtifactPathEvidence(
    workspace,
    authority.planArtifactRelativePath
  )
  let preflightPlanExcludeBound = false
  if (preflightOnly) {
    try {
      const inspected = inspectMilestoneAPlanArtifactExcludeInput(
        workspace,
        authority.gitCommonDir,
        authority.planArtifactRelativePath
      )
      preflightPlanExcludeBound = canonicalJSON(inspected.authorityFields) === canonicalJSON({
        planArtifactRelativePath: authority.planArtifactRelativePath,
        planArtifactExcludeRule: authority.planArtifactExcludeRule,
        planArtifactExcludePath: authority.planArtifactExcludePath,
        planArtifactExcludeBeforeSha256: authority.planArtifactExcludeBeforeSha256,
        planArtifactExcludeBeforeByteLength: authority.planArtifactExcludeBeforeByteLength,
        planArtifactExcludeSha256: authority.planArtifactExcludeSha256,
        planArtifactExcludeByteLength: authority.planArtifactExcludeByteLength
      })
    } catch {
      preflightPlanExcludeBound = false
    }
  }
  const planExcludeBound = preflightOnly
    ? preflightPlanExcludeBound
    : planExcludeBeforeStatus.ok && planExcludeAfterStatus.ok &&
      planExcludeBeforeStatus.excludeSha256 === planExcludeAfterStatus.excludeSha256 &&
      planExcludeBeforeStatus.excludeByteLength === planExcludeAfterStatus.excludeByteLength
  const expectedPlanPresence = phase === 'baseline'
    ? !planPathBeforeStatus.present && planPathBeforeStatus.planEntryCount === 0 &&
      !planPathAfterStatus.present && planPathAfterStatus.planEntryCount === 0
    : planPathBeforeStatus.present && planPathBeforeStatus.planEntryCount === 1 &&
      planPathAfterStatus.present && planPathAfterStatus.planEntryCount === 1
  const planArtifactPathSafe = planPathBeforeStatus.safe && planPathAfterStatus.safe &&
    expectedPlanPresence
  const acceptanceContractBound = contractFile.regular &&
    contractFile.sha256 === authority.contractSha256 &&
    pathIsOutside(workspace, contractPath)
  const fixtureSubstituteRejected = !externalRepositoryFixtureSubstitute(workspace)
  const onlyIntendedSourceChanged = externalRepositoryStatusMatches(
    status,
    authority.expectedChangedFiles,
    phase
  )
  const expectedFileBound = (file) => file.regular && (
    phase === 'baseline'
      ? file.observedSha256 === file.baselineSha256
      : file.acceptedSha256s.includes(file.observedSha256) &&
        file.observedSha256 !== file.baselineSha256
  )
  const expectedSourceBound = expectedFiles.every(expectedFileBound)
  const expectedSourceMismatchCount = expectedFiles.filter(
    (file) => !expectedFileBound(file)
  ).length
  const observedSourceDigest = sha256(canonicalJSON(expectedFiles.map((file) => ({
    path: file.path,
    sha256: file.observedSha256
  }))))
  const expectedSourceDigest = sha256(canonicalJSON(expectedFiles.map((file) => ({
    path: file.path,
    sha256: expectedSourceBound
      ? file.observedSha256
      : phase === 'baseline' ? file.baselineSha256 : file.expectedSha256
  }))))
  const protectedInputsBound = inspectedFiles.every((file) => file.regular) && contextBound &&
    acceptanceContractBound
  const ok = gitRepository && workspaceBound && baselineCommitBound && baselineTreeBound &&
    baselineHistoryBound && originBound && acceptanceContractBound && fixtureSubstituteRejected &&
    protectedInputsBound && onlyIntendedSourceChanged && expectedSourceBound &&
    ownerRootBound && ownerOnlyIsolationBound && gitCommonDirContained &&
    symlinkHardlinkIsolationBound && provenanceBound && planExcludeBound &&
    planArtifactPathSafe &&
    (authority.acceptanceClass !== 'formal-fresh-origin-verified' || authority.freshOriginVerified)
  let blocker = ''
  if (!ok) {
    if (!gitRepository) blocker = 'external_repository_git_identity_invalid'
    else if (!workspaceBound) blocker = 'external_repository_workspace_identity_changed'
    else if (!baselineCommitBound || !baselineTreeBound) {
      blocker = 'external_repository_baseline_changed'
    } else if (!baselineHistoryBound) blocker = 'external_repository_history_insufficient'
    else if (!originBound) blocker = 'external_repository_origin_mismatch'
    else if (!ownerRootBound) blocker = 'external_repository_owner_root_invalid'
    else if (!ownerOnlyIsolationBound) blocker = 'external_repository_permissions_invalid'
    else if (!gitCommonDirContained) blocker = 'external_repository_git_common_dir_invalid'
    else if (!symlinkHardlinkIsolationBound) blocker = 'external_repository_link_isolation_invalid'
    else if (!provenanceBound) blocker = 'external_repository_provenance_changed'
    else if (!planExcludeBound) blocker = 'external_repository_plan_exclude_changed'
    else if (!planArtifactPathSafe) blocker = 'external_repository_plan_artifact_unsafe'
    else if (!acceptanceContractBound) blocker = 'external_repository_contract_changed'
    else if (!fixtureSubstituteRejected) blocker = 'external_repository_fixture_substitute_rejected'
    else if (!protectedInputsBound) blocker = 'external_repository_inspection_input_changed'
    else if (!onlyIntendedSourceChanged) blocker = 'external_repository_unexpected_change'
    else blocker = phase === 'baseline'
      ? 'external_repository_baseline_target_changed'
      : 'external_repository_expected_result_mismatch'
  }
  return {
    ...base,
    ok,
    blocker,
    gitRepository,
    workspaceBound,
    baselineCommitBound,
    baselineTreeBound,
    protectedInputsBound,
    acceptanceContractBound,
    onlyIntendedSourceChanged,
    expectedSourceBound,
    expectedSourceFileCount: expectedFiles.length,
    expectedSourceMismatchCount,
    expectedSourceDigest,
    observedSourceDigest,
    gitStatusSha256: status.ok ? sha256(status.stdout) : '',
    protectedInputsDigest: sha256(canonicalJSON(inspectedFiles)),
    sourceSha256: observedSourceDigest,
    contractSha256: contractFile.sha256,
    baselineHistoryBound,
    originBound,
    fixtureSubstituteRejected,
    formalAcceptance: authority.acceptanceClass === 'formal-fresh-origin-verified' &&
      authority.freshOriginVerified,
    freshOriginVerified: authority.freshOriginVerified,
    ownerRootBound,
    ownerOnlyIsolationBound,
    gitCommonDirContained,
    symlinkHardlinkIsolationBound,
    provenanceBound,
    planArtifactExcludeBound: planExcludeBound,
    planArtifactPathSafe
  }
}

export function projectMilestoneARepositoryAcceptance(repository, acceptance) {
  const projected = repository && typeof repository === 'object' && !Array.isArray(repository)
    ? { ...repository }
    : {}
  const safeDigest = (value) => /^[0-9a-f]{64}$/u.test(String(value || ''))
    ? value
    : ''
  const safeCount = (value) => Number.isSafeInteger(value) && value >= 0 ? value : 0
  const blocker = typeof acceptance?.blocker === 'string' &&
    /^[a-z0-9_]{1,128}$/u.test(acceptance.blocker)
    ? acceptance.blocker
    : ''
  return {
    ...projected,
    finalValidated: acceptance?.ok === true,
    fixtureSubstituteRejected: acceptance?.fixtureSubstituteRejected === true,
    blocker,
    formalAcceptance: acceptance?.formalAcceptance === true,
    freshOriginVerified: acceptance?.freshOriginVerified === true,
    ownerRootBound: acceptance?.ownerRootBound === true,
    ownerOnlyIsolationBound: acceptance?.ownerOnlyIsolationBound === true,
    gitCommonDirContained: acceptance?.gitCommonDirContained === true,
    symlinkHardlinkIsolationBound: acceptance?.symlinkHardlinkIsolationBound === true,
    planArtifactExcludeBound: acceptance?.planArtifactExcludeBound === true,
    planArtifactPathSafe: acceptance?.planArtifactPathSafe === true,
    finalOnlyIntendedSourceChanged: acceptance?.onlyIntendedSourceChanged === true,
    finalExpectedSourceBound: acceptance?.expectedSourceBound === true,
    finalExpectedSourceFileCount: safeCount(acceptance?.expectedSourceFileCount),
    finalExpectedSourceMismatchCount: safeCount(acceptance?.expectedSourceMismatchCount),
    finalExpectedSourceDigest: safeDigest(acceptance?.expectedSourceDigest),
    finalObservedSourceDigest: safeDigest(acceptance?.observedSourceDigest),
    finalGitStatusSha256: safeDigest(acceptance?.gitStatusSha256)
  }
}

export function projectMilestoneARepositoryBaseline(repository, baseline) {
  const projected = repository && typeof repository === 'object' && !Array.isArray(repository)
    ? { ...repository }
    : {}
  const baselineValidated = baseline?.ok === true
  return {
    ...projected,
    baselineValidated,
    preflightValidated: baselineValidated,
    ownerRootBound: baselineValidated && baseline?.ownerRootBound === true,
    ownerOnlyIsolationBound: baselineValidated && baseline?.ownerOnlyIsolationBound === true,
    gitCommonDirContained: baselineValidated && baseline?.gitCommonDirContained === true,
    symlinkHardlinkIsolationBound:
      baselineValidated && baseline?.symlinkHardlinkIsolationBound === true,
    planArtifactExcludeBound:
      baselineValidated && baseline?.planArtifactExcludeBound === true,
    planArtifactPathSafe: baselineValidated && baseline?.planArtifactPathSafe === true
  }
}

export function preflightMilestoneAExternalRepositoryAcceptance(
  workspacePath,
  contractFilePath,
  provenanceFilePath,
  ownerRootPath,
  options = {}
) {
  return loadMilestoneAExternalRepositoryAcceptance(
    workspacePath,
    contractFilePath,
    provenanceFilePath,
    ownerRootPath,
    { ...options, preflightOnly: true }
  )
}

export function loadMilestoneAExternalRepositoryAcceptance(
  workspacePath,
  contractFilePath,
  provenanceFilePath,
  ownerRootPath,
  options = {}
) {
  const verificationMode = options.verificationMode === 'synthetic-parser-only'
    ? 'synthetic-parser-only'
    : 'formal-fresh-origin-verified'
  const preflightOnly = options.preflightOnly === true
  if (![workspacePath, contractFilePath, provenanceFilePath, ownerRootPath]
    .every((path) => isAbsolute(path))) {
    throw new Error('external_repository_paths_must_be_absolute')
  }
  let workspaceStat
  let contractStat
  let provenanceStat
  let ownerRootStat
  try {
    workspaceStat = lstatSync(workspacePath)
    contractStat = lstatSync(contractFilePath)
    provenanceStat = lstatSync(provenanceFilePath)
    ownerRootStat = lstatSync(ownerRootPath)
  } catch {
    throw new Error('external_repository_input_unavailable')
  }
  if (!workspaceStat.isDirectory() || workspaceStat.isSymbolicLink() ||
    !contractStat.isFile() || contractStat.isSymbolicLink() || contractStat.nlink !== 1 ||
    !provenanceStat.isFile() || provenanceStat.isSymbolicLink() || provenanceStat.nlink !== 1 ||
    !ownerRootStat.isDirectory() || ownerRootStat.isSymbolicLink()) {
    throw new Error('external_repository_input_type_invalid')
  }
  let workspace
  let contractPath
  let provenancePath
  let ownerRoot
  try {
    workspace = realpathSync(workspacePath)
    contractPath = realpathSync(contractFilePath)
    provenancePath = realpathSync(provenanceFilePath)
    ownerRoot = realpathSync(ownerRootPath)
  } catch {
    throw new Error('external_repository_input_unavailable')
  }
  const sourceRoot = realpathSync(process.cwd())
  const cacheRoot = realpathSync(CACHE_MOUNT)
  if (!pathIsOutside(sourceRoot, workspace) || !pathIsOutside(workspace, contractPath) ||
    !pathIsOutside(workspace, provenancePath) || pathIsOutside(cacheRoot, ownerRoot) ||
    pathIsOutside(ownerRoot, workspace) || pathIsOutside(ownerRoot, contractPath) ||
    pathIsOutside(ownerRoot, provenancePath)) {
    throw new Error('external_repository_must_be_preexisting_and_outside_source_worktree')
  }
  const cacheDeviceBinding = trustedCacheDeviceBindingEvidence(
    statSync(cacheRoot),
    [ownerRootStat, workspaceStat, contractStat, provenanceStat]
  )
  if (!cacheDeviceBinding.ok) {
    throw new Error('external_repository_trusted_cache_device_mismatch')
  }
  if (!ownerOnlyDirectory(ownerRoot, true) || !ownerOnlyDirectory(workspace) ||
    !ownerPrivateRegularFile(contractPath) || !ownerPrivateRegularFile(provenancePath)) {
    throw new Error('external_repository_permissions_invalid')
  }
  const contractFile = hashRegularFile(contractPath, {
    capture: true,
    maximumBytes: 1024 * 1024
  })
  if (!contractFile.regular || !contractFile.content) {
    throw new Error('external_repository_contract_unavailable')
  }
  let parsed
  try {
    parsed = JSON.parse(contractFile.content.toString('utf8'))
  } catch {
    throw new Error('external_repository_contract_invalid_json')
  }
  if (!externalRepositoryContractShapeValid(parsed) ||
    contractFile.content.toString('utf8') !== `${JSON.stringify(parsed, null, 2)}\n`) {
    throw new Error('external_repository_contract_invalid')
  }
  if (verificationMode === 'formal-fresh-origin-verified' && !parsed.continuationRead) {
    throw new Error('external_repository_formal_continuation_read_required')
  }
  const provenanceFile = hashRegularFile(provenancePath, {
    capture: true,
    maximumBytes: 256 * 1024
  })
  let provenance
  try {
    provenance = parseMilestoneAExternalRepositoryProvenance(
      JSON.parse(provenanceFile.content?.toString('utf8') || '')
    )
  } catch {
    provenance = null
  }
  if (!provenanceFile.regular || !provenance ||
    provenanceFile.content.toString('utf8') !== `${JSON.stringify(provenance, null, 2)}\n` ||
    provenance.baselineCommit !== parsed.repository.baselineCommit ||
    provenance.baselineTree !== parsed.repository.baselineTree ||
    sha256(provenance.originUrl) !== parsed.repository.originUrlSha256) {
    throw new Error('external_repository_provenance_invalid')
  }
  const topLevel = gitCapture(workspace, ['rev-parse', '--show-toplevel'])
  const head = gitCapture(workspace, ['rev-parse', 'HEAD'])
  const tree = gitCapture(workspace, ['rev-parse', 'HEAD^{tree}'])
  const commonDir = gitCapture(workspace, ['rev-parse', '--git-common-dir'])
  const status = gitCapture(workspace, [
    'status', '--porcelain=v1', '-z', '--untracked-files=all'
  ])
  if (!topLevel.ok || topLevel.stdout.trim() !== workspace || !head.ok || !tree.ok ||
    !status.ok || status.stdout !== '' ||
    head.stdout.trim() !== parsed.repository.baselineCommit ||
    tree.stdout.trim() !== parsed.repository.baselineTree) {
    throw new Error('external_repository_clean_baseline_mismatch')
  }
  let gitCommonDir = ''
  try {
    gitCommonDir = commonDir.ok
      ? realpathSync(resolve(workspace, commonDir.stdout.trim()))
      : ''
  } catch {
    gitCommonDir = ''
  }
  if (!gitCommonDir || pathIsOutside(workspace, gitCommonDir) ||
    !ownerOnlyDirectory(gitCommonDir)) {
    throw new Error('external_repository_git_common_dir_invalid')
  }
  if (externalRepositoryFixtureSubstitute(workspace)) {
    throw new Error('external_repository_fixture_substitute_rejected')
  }
  const inspectPaths = parsed.task.inspectPaths.map(safeRepositoryRelativePath)
  for (const path of inspectPaths) {
    const tracked = gitCapture(workspace, ['ls-files', '--error-unmatch', '--', path])
    const file = hashRegularFile(join(workspace, path))
    if (!tracked.ok || !file.regular || !ownerPrivateRegularFile(join(workspace, path))) {
      throw new Error('external_repository_inspection_path_invalid')
    }
  }
  const planArtifactRelativePath = milestoneAPlanRelativePath({
    taskRequest: parsed.task.request,
    inspectPaths
  })
  const expectedChangedFiles = parsed.task.expectedChangedFiles.map((item) => {
    const path = safeRepositoryRelativePath(item.path)
    const baseline = hashRegularFile(join(workspace, path))
    const equivalentSha256s = Array.isArray(item.equivalentSha256s)
      ? item.equivalentSha256s
      : []
    const acceptedSha256s = Object.freeze([item.sha256, ...equivalentSha256s])
    if (!baseline.regular || !ownerPrivateRegularFile(join(workspace, path)) ||
      acceptedSha256s.includes(baseline.sha256)) {
      throw new Error('external_repository_expected_change_invalid')
    }
    return Object.freeze({
      path,
      sha256: item.sha256,
      acceptedSha256s,
      baselineSha256: baseline.sha256
    })
  })
  const continuationRead = parsed.continuationRead || parsed.longContext
  const context = hashRegularFile(join(workspace, continuationRead.path), {
    capture: true,
    maximumBytes: parsed.continuationRead
      ? parsed.continuationRead.maxBytes
      : 2 * 1024 * 1024
  })
  const contextBinding = parsed.continuationRead
    ? continuationReadBindingEvidence(context, parsed.continuationRead)
    : externalRepositoryLongContextBindingEvidence(context, parsed.longContext)
  if (!contextBinding.ok) {
    throw new Error(parsed.continuationRead
      ? 'external_repository_continuation_read_contract_mismatch'
      : 'external_repository_long_context_contract_mismatch')
  }
  const contextReadLineLimit = contextBinding.lineLimit
  if (!Number.isSafeInteger(contextReadLineLimit) || contextReadLineLimit <= 0) {
    throw new Error('external_repository_long_context_line_limit_invalid')
  }
  const origin = gitCapture(workspace, ['remote', 'get-url', 'origin'])
  if (!origin.ok || origin.stdout.trim() !== provenance.originUrl) {
    throw new Error('external_repository_origin_mismatch')
  }
  const freshOrigin = verificationMode === 'formal-fresh-origin-verified'
    ? verifyFreshExternalRepositoryOrigin(provenance, options)
    : {
        ok: false,
        blocker: 'synthetic_parser_only',
        fetchAttemptCount: 0,
        fetchExitCodes: [],
        cleanupSucceeded: true
      }
  if (verificationMode === 'formal-fresh-origin-verified' &&
    (!freshOrigin.ok || freshOrigin.cleanupSucceeded !== true)) {
    const error = new Error(
      freshOrigin.blocker || 'external_repository_origin_probe_cleanup_failed'
    )
    error.fetchAttemptCount = freshOrigin.fetchAttemptCount
    error.fetchExitCodes = [...freshOrigin.fetchExitCodes]
    throw error
  }
  const planArtifactExclude = preflightOnly
    ? inspectMilestoneAPlanArtifactExcludeInput(
      workspace,
      gitCommonDir,
      planArtifactRelativePath
    ).authorityFields
    : installMilestoneAPlanArtifactExclude(
      workspace,
      gitCommonDir,
      planArtifactRelativePath
    )
  const gitCheckCommand = `git diff --check -- ${expectedChangedFiles
    .map((item) => shellQuotedArgument(item.path)).join(' ')}`
  const authority = Object.freeze({
    contract: EXTERNAL_REPOSITORY_AUTHORITY_CONTRACT,
    mode: 'external-preexisting-real-repository',
    acceptanceClass: verificationMode,
    workspace,
    workspaceSha256: sha256(workspace),
    ownerRoot,
    ownerRootSha256: sha256(ownerRoot),
    contractPath,
    contractSha256: contractFile.sha256,
    provenancePath,
    provenanceSha256: provenanceFile.sha256,
    gitCommonDir,
    baselineCommit: parsed.repository.baselineCommit,
    baselineTree: parsed.repository.baselineTree,
    minimumCommitCount: parsed.repository.minimumCommitCount,
    originUrlSha256: parsed.repository.originUrlSha256,
    inspectPaths: Object.freeze([...inspectPaths]),
    expectedChangedFiles: Object.freeze(expectedChangedFiles),
    taskRequest: parsed.task.request,
    gitCheckCommand,
    testCommand: contractTestCommand(parsed.test),
    testExecutable: parsed.test.executable,
    testArguments: Object.freeze([...parsed.test.arguments]),
    testTimeoutMs: parsed.test.timeoutMs,
    context: Object.freeze({
      ...continuationRead,
      ...(parsed.continuationRead
        ? { deprecated: false }
        : { deprecated: true })
    }),
    ...planArtifactExclude,
    freshOriginVerified: freshOrigin.ok === true,
    freshOriginFetchAttemptCount: freshOrigin.fetchAttemptCount,
    freshOriginFetchExitCodes: Object.freeze([...freshOrigin.fetchExitCodes])
  })
  const baseline = verifyMilestoneAExternalRepositoryAcceptance(
    authority,
    'baseline',
    { preflightOnly }
  )
  if (!baseline.ok) throw new Error(baseline.blocker)
  if (preflightOnly) {
    return Object.freeze({
      contract: 'analytix.milestone-a.real-repository-preflight.v1',
      ok: true,
      mutationApplied: false,
      acceptanceClass: verificationMode,
      workspaceSha256: authority.workspaceSha256,
      ownerRootSha256: authority.ownerRootSha256,
      contractSha256: authority.contractSha256,
      provenanceSha256: authority.provenanceSha256,
      baselineCommit: authority.baselineCommit,
      baselineTree: authority.baselineTree,
      minimumCommitCount: authority.minimumCommitCount,
      originUrlSha256: authority.originUrlSha256,
      inspectPathCount: authority.inspectPaths.length,
      expectedChangedFileCount: authority.expectedChangedFiles.length,
      contextSha256: context.sha256,
      contextByteLength: context.byteLength,
      contextLineLimit: contextReadLineLimit,
      planArtifactPathHash: sha256(authority.planArtifactRelativePath),
      planArtifactExcludeBeforeSha256: authority.planArtifactExcludeBeforeSha256,
      planArtifactExcludeBeforeByteLength: authority.planArtifactExcludeBeforeByteLength,
      freshOriginVerified: authority.freshOriginVerified,
      freshOriginFetchAttemptCount: authority.freshOriginFetchAttemptCount,
      freshOriginFetchExitCodes: [...authority.freshOriginFetchExitCodes],
      formalAcceptance: baseline.formalAcceptance === true,
      ownerRootBound: baseline.ownerRootBound === true,
      ownerOnlyIsolationBound: baseline.ownerOnlyIsolationBound === true,
      gitCommonDirContained: baseline.gitCommonDirContained === true,
      symlinkHardlinkIsolationBound: baseline.symlinkHardlinkIsolationBound === true,
      planArtifactExcludeBound: baseline.planArtifactExcludeBound === true,
      planArtifactPathSafe: baseline.planArtifactPathSafe === true
    })
  }
  repositoryContextMarkers.set(authority, Object.freeze({
    first: authority.context.firstMarker || '',
    completion: authority.context.completionMarker || authority.context.successMarker ||
      'MILESTONE_A_CONTINUATION_OK',
    last: authority.context.lastMarker || ''
  }))
  externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
    authority.workspace,
    'bash',
    canonicalJSON({ command: authority.testCommand })
  ))
  externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
    authority.workspace,
    'bash',
    canonicalJSON({ command: authority.gitCheckCommand })
  ))
  for (const path of milestoneAPlanInspectPaths(authority)) {
    externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
      authority.workspace,
      'read',
      canonicalJSON({ path })
    ))
  }
  externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
    authority.workspace,
    'read',
    canonicalJSON({ path: authority.context.path, limit: contextReadLineLimit })
  ))
  return authority
}

function verifyRepositoryAcceptance(authority, phase = 'final') {
  return externalRepositoryAuthorityShapeValid(authority)
    ? verifyMilestoneAExternalRepositoryAcceptance(authority, phase)
    : verifyMilestoneARepositoryAcceptance(authority, phase)
}

function protectedRepositoryInputs(workspace) {
  const entries = FIXTURE_PROTECTED_PATHS.map((relativePath) => {
    const path = join(workspace, relativePath)
    const evidence = hashRegularFile(path)
    let writeProtected = false
    try {
      writeProtected = (lstatSync(path).mode & 0o222) === 0
    } catch {
      writeProtected = false
    }
    return {
      path: relativePath,
      regular: evidence.regular,
      byteLength: evidence.byteLength,
      sha256: evidence.sha256,
      writeProtected
    }
  })
  return {
    entries,
    digest: sha256(canonicalJSON(entries.map(({ path, byteLength, sha256 }) => ({
      path,
      byteLength,
      sha256
    }))))
  }
}

function harnessOwnedRepositoryValidatorSource(authority) {
  const sealedAuthority = {
    contract: authority.contract,
    nonce: authority.nonce,
    workspace: authority.workspace,
    workspaceSha256: authority.workspaceSha256,
    receiptPath: authority.receiptPath,
    baselineCommit: authority.baselineCommit,
    baselineTree: authority.baselineTree,
    protectedFiles: authority.protectedFiles,
    protectedInputsDigest: authority.protectedInputsDigest,
    expectedSourceSha256: authority.expectedSourceSha256,
    expectedStatus: authority.expectedStatus,
    expectedStatusSha256: authority.expectedStatusSha256,
    testScript: authority.testScript,
    testScriptSha256: authority.testScriptSha256
  }
  return `#!/usr/bin/env node
import { spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import {
  closeSync,
  constants,
  fstatSync,
  fsyncSync,
  lstatSync,
  openSync,
  readFileSync,
  realpathSync,
  writeFileSync
} from 'node:fs'
import { join } from 'node:path'
import process from 'node:process'
import { fileURLToPath, pathToFileURL } from 'node:url'

const authority = Object.freeze(${JSON.stringify(sealedAuthority)})
const startedAt = new Date().toISOString()

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return '[' + value.map(canonicalJSON).join(',') + ']'
  if (value && typeof value === 'object') {
    return '{' + Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => JSON.stringify(key) + ':' + canonicalJSON(item))
      .join(',') + '}'
  }
  return JSON.stringify(value)
}

function fail(code) {
  throw new Error(code)
}

function regularFile(path) {
  const before = lstatSync(path)
  if (!before.isFile() || before.isSymbolicLink()) fail('validator_input_not_regular')
  const content = readFileSync(path)
  const after = lstatSync(path)
  if (before.dev !== after.dev || before.ino !== after.ino ||
    before.size !== after.size || before.mtimeMs !== after.mtimeMs) {
    fail('validator_input_changed_during_read')
  }
  return {
    content,
    byteLength: content.byteLength,
    sha256: sha256(content),
    writeProtected: (after.mode & 0o222) === 0
  }
}

function git(gitArgs) {
  const result = spawnSync('git', gitArgs, {
    cwd: authority.workspace,
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) fail('validator_git_command_failed')
  return String(result.stdout || '')
}

function processAncestors() {
  const records = []
  let pid = process.ppid
  for (let index = 0; index < 12 && Number.isInteger(pid) && pid > 1; index += 1) {
    const result = spawnSync('/bin/ps', ['-o', 'ppid=,command=', '-p', String(pid)], {
      encoding: 'utf8',
      stdio: 'pipe'
    })
    if (result.status !== 0) break
    const match = String(result.stdout || '').match(/^\\s*(\\d+)\\s+([\\s\\S]+?)\\s*$/u)
    if (!match) break
    records.push({ pid, command: match[2].replace(/\\s+/gu, ' ').trim() })
    pid = Number(match[1])
  }
  return records
}

async function main() {
  if (realpathSync(process.cwd()) !== realpathSync(authority.workspace)) {
    fail('validator_workspace_mismatch')
  }
  if (sha256(realpathSync(process.cwd())) !== authority.workspaceSha256) {
    fail('validator_workspace_digest_mismatch')
  }
  const selfPath = realpathSync(fileURLToPath(import.meta.url))
  const configuredValidator = String(
    process.env.ANALYTIX_MILESTONE_A_VALIDATOR_PATH || ''
  ).trim()
  if (!configuredValidator || realpathSync(configuredValidator) !== selfPath) {
    fail('validator_path_authority_mismatch')
  }
  if (process.env.npm_lifecycle_event !== 'test' ||
    process.env.npm_command !== 'test' ||
    process.env.npm_package_name !== 'analytix-milestone-a-isolated-repo' ||
    process.env.npm_lifecycle_script !== authority.testScript ||
    sha256(process.env.npm_lifecycle_script) !== authority.testScriptSha256 ||
    realpathSync(String(process.env.INIT_CWD || '')) !== realpathSync(authority.workspace) ||
    !String(process.env.npm_execpath || '').includes('npm')) {
    fail('validator_npm_test_lifecycle_missing')
  }
  const ancestors = processAncestors()
  const npmTestAncestorObserved = ancestors.some(({ command }) =>
    /(?:^|[\\/\\s])npm(?:-cli\\.js)?["']?\\s+test(?:\\s|$)/u.test(command)
  )
  const packageScriptParentObserved = Boolean(ancestors[0]?.command.includes(
    'node --test test/counter.test.mjs'
  ) && ancestors[0]?.command.includes('ANALYTIX_MILESTONE_A_VALIDATOR_PATH'))
  if (!npmTestAncestorObserved || !packageScriptParentObserved) {
    fail('validator_npm_test_process_lineage_missing')
  }
  if (git(['rev-parse', '--show-toplevel']).trim() !== realpathSync(authority.workspace) ||
    git(['rev-parse', 'HEAD']).trim() !== authority.baselineCommit ||
    git(['rev-parse', 'HEAD^{tree}']).trim() !== authority.baselineTree) {
    fail('validator_git_baseline_mismatch')
  }
  const status = git(['status', '--porcelain=v1', '-z', '--untracked-files=all'])
  if (status !== authority.expectedStatus || sha256(status) !== authority.expectedStatusSha256) {
    fail('validator_unexpected_repository_change')
  }
  const protectedFiles = authority.protectedFiles.map((expected) => {
    const observed = regularFile(join(authority.workspace, expected.path))
    if (observed.sha256 !== expected.sha256 ||
      observed.byteLength !== expected.byteLength ||
      observed.writeProtected !== true) {
      fail('validator_protected_input_mismatch')
    }
    return {
      path: expected.path,
      byteLength: observed.byteLength,
      sha256: observed.sha256
    }
  })
  if (sha256(canonicalJSON(protectedFiles)) !== authority.protectedInputsDigest) {
    fail('validator_protected_input_digest_mismatch')
  }
  const sourcePath = join(authority.workspace, ${JSON.stringify(FIXTURE_SOURCE_PATH)})
  const source = regularFile(sourcePath)
  if (source.sha256 !== authority.expectedSourceSha256) {
    fail('validator_source_repair_mismatch')
  }
  const implementation = await import(pathToFileURL(sourcePath).href + '?acceptance=' +
    encodeURIComponent(authority.nonce))
  if (typeof implementation.add !== 'function' ||
    implementation.add(2, 3) !== 5 ||
    implementation.add(-2, 3) !== 1 ||
    implementation.add(0, 0) !== 0) {
    fail('validator_source_assertion_failed')
  }
  const validator = regularFile(selfPath)
  const completedAt = new Date().toISOString()
  const receipt = {
    contract: ${JSON.stringify(TEST_RECEIPT_CONTRACT)},
    marker: ${JSON.stringify(TEST_MARKER)},
    nonce: authority.nonce,
    workspaceSha256: authority.workspaceSha256,
    baselineCommit: authority.baselineCommit,
    baselineTree: authority.baselineTree,
    protectedInputsDigest: authority.protectedInputsDigest,
    expectedSourceSha256: authority.expectedSourceSha256,
    observedSourceSha256: source.sha256,
    validatorSha256: validator.sha256,
    npmLifecycleEvent: process.env.npm_lifecycle_event,
    npmLifecycleScriptSha256: sha256(process.env.npm_lifecycle_script),
    npmCommand: process.env.npm_command,
    npmPackageName: process.env.npm_package_name,
    npmTestAncestorObserved,
    packageScriptParentObserved,
    gitStatusSha256: sha256(status),
    assertionCount: 3,
    startedAt,
    completedAt,
    ancestorChainDigest: sha256(canonicalJSON(ancestors.map(({ command }) => sha256(command))))
  }
  const flags = constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL |
    (constants.O_NOFOLLOW || 0)
  const fd = openSync(authority.receiptPath, flags, 0o600)
  try {
    writeFileSync(fd, JSON.stringify(receipt) + '\\n', 'utf8')
    fsyncSync(fd)
    const written = fstatSync(fd)
    if (!written.isFile() || written.size <= 0) fail('validator_receipt_write_failed')
  } finally {
    closeSync(fd)
  }
}

try {
  await main()
} catch (error) {
  process.stderr.write((error instanceof Error ? error.message : 'validator_failed') + '\\n')
  process.exitCode = 1
}
`
}

function repositoryAuthorityShapeValid(authority) {
  return authority?.contract === REPOSITORY_ACCEPTANCE_CONTRACT &&
    typeof authority.workspace === 'string' && authority.workspace &&
    typeof authority.validatorPath === 'string' && authority.validatorPath &&
    typeof authority.receiptPath === 'string' && authority.receiptPath &&
    /^[0-9a-f]{40,64}$/.test(String(authority.baselineCommit || '')) &&
    /^[0-9a-f]{40,64}$/.test(String(authority.baselineTree || '')) &&
    /^[0-9a-f]{64}$/.test(String(authority.expectedSourceSha256 || '')) &&
    /^[0-9a-f]{64}$/.test(String(authority.protectedInputsDigest || '')) &&
    /^[0-9a-f]{64}$/.test(String(authority.validatorSha256 || '')) &&
    Array.isArray(authority.protectedFiles) && authority.protectedFiles.length > 0
}

export function verifyMilestoneARepositoryAcceptance(authority, phase = 'final') {
  const base = {
    ok: false,
    blocker: '',
    phase,
    gitRepository: false,
    baselineCommitBound: false,
    baselineTreeBound: false,
    protectedInputsBound: false,
    protectedInputsWriteProtected: false,
    validatorOutsideRepository: false,
    validatorBound: false,
    onlyIntendedSourceChanged: false,
    expectedSourceBound: false,
    gitStatusSha256: '',
    protectedInputsDigest: '',
    sourceSha256: '',
    validatorSha256: ''
  }
  if (!repositoryAuthorityShapeValid(authority)) {
    return { ...base, blocker: 'repository_acceptance_authority_invalid' }
  }
  let workspace
  let validatorPath
  try {
    workspace = realpathSync(authority.workspace)
    validatorPath = realpathSync(authority.validatorPath)
  } catch {
    return { ...base, blocker: 'repository_acceptance_path_unavailable' }
  }
  const validatorRelative = relative(workspace, validatorPath)
  const validatorOutsideRepository = validatorRelative === '..' ||
    validatorRelative.startsWith(`..${sep}`) || isAbsolute(validatorRelative)
  const topLevel = gitCapture(workspace, ['rev-parse', '--show-toplevel'])
  const head = gitCapture(workspace, ['rev-parse', 'HEAD'])
  const tree = gitCapture(workspace, ['rev-parse', 'HEAD^{tree}'])
  const status = gitCapture(workspace, [
    'status', '--porcelain=v1', '-z', '--untracked-files=all'
  ])
  const source = hashRegularFile(join(workspace, FIXTURE_SOURCE_PATH))
  const validator = hashRegularFile(validatorPath)
  const protectedInputs = protectedRepositoryInputs(workspace)
  const protectedInputsBound = protectedInputs.entries.length === authority.protectedFiles.length &&
    protectedInputs.entries.every((entry, index) => {
      const expected = authority.protectedFiles[index]
      return entry.path === expected.path && entry.regular &&
        entry.byteLength === expected.byteLength && entry.sha256 === expected.sha256
    }) && protectedInputs.digest === authority.protectedInputsDigest
  const protectedInputsWriteProtected = protectedInputs.entries.every((entry) =>
    entry.writeProtected
  )
  const expectedStatus = phase === 'baseline' ? '' : authority.expectedStatus
  const expectedSourceSha256 = phase === 'baseline'
    ? authority.baselineSourceSha256
    : authority.expectedSourceSha256
  const gitRepository = topLevel.ok && topLevel.stdout.trim() === workspace
  const baselineCommitBound = head.ok && head.stdout.trim() === authority.baselineCommit
  const baselineTreeBound = tree.ok && tree.stdout.trim() === authority.baselineTree
  const onlyIntendedSourceChanged = status.ok && status.stdout === expectedStatus
  const expectedSourceBound = source.regular && source.sha256 === expectedSourceSha256
  const validatorBound = validator.regular && validator.sha256 === authority.validatorSha256
  const ok = gitRepository && baselineCommitBound && baselineTreeBound &&
    protectedInputsBound && protectedInputsWriteProtected && validatorOutsideRepository &&
    validatorBound && onlyIntendedSourceChanged && expectedSourceBound
  let blocker = ''
  if (!ok) {
    if (!gitRepository) blocker = 'isolated_repository_git_identity_invalid'
    else if (!baselineCommitBound || !baselineTreeBound) blocker = 'isolated_repository_baseline_changed'
    else if (!protectedInputsBound) blocker = 'isolated_repository_protected_input_changed'
    else if (!protectedInputsWriteProtected) blocker = 'isolated_repository_protected_input_writable'
    else if (!validatorOutsideRepository) blocker = 'repository_validator_not_external'
    else if (!validatorBound) blocker = 'repository_validator_changed'
    else if (!onlyIntendedSourceChanged) blocker = 'isolated_repository_unexpected_change'
    else blocker = phase === 'baseline'
      ? 'isolated_repository_baseline_source_changed'
      : 'isolated_repository_repair_not_exact'
  }
  return {
    ...base,
    ok,
    blocker,
    gitRepository,
    baselineCommitBound,
    baselineTreeBound,
    protectedInputsBound,
    protectedInputsWriteProtected,
    validatorOutsideRepository,
    validatorBound,
    onlyIntendedSourceChanged,
    expectedSourceBound,
    gitStatusSha256: status.ok ? sha256(status.stdout) : '',
    protectedInputsDigest: protectedInputs.digest,
    sourceSha256: source.sha256,
    validatorSha256: validator.sha256
  }
}

export function createMilestoneARepositoryAcceptance(workspace, harnessRoot) {
  if (!isAbsolute(workspace) || !isAbsolute(harnessRoot)) {
    throw new Error('repository_acceptance_paths_must_be_absolute')
  }
  mkdirSync(harnessRoot, { recursive: true, mode: 0o700 })
  const contextMarkers = writeFixtureRepository(workspace)
  const init = gitCapture(workspace, [
    '-c', 'init.defaultBranch=main', 'init', '--quiet', '.'
  ])
  const add = gitCapture(workspace, ['add', '--all'])
  const commit = gitCapture(workspace, [
    '-c', 'user.name=Analytix Milestone A Harness',
    '-c', 'user.email=milestone-a@analytix.invalid',
    '-c', 'commit.gpgsign=false',
    '-c', 'core.hooksPath=/dev/null',
    'commit', '--quiet', '-m', 'Milestone A immutable acceptance baseline'
  ])
  if (!init.ok || !add.ok || !commit.ok) {
    throw new Error('isolated_repository_git_initialization_failed')
  }
  const baselineCommit = gitCapture(workspace, ['rev-parse', 'HEAD'])
  const baselineTree = gitCapture(workspace, ['rev-parse', 'HEAD^{tree}'])
  const source = hashRegularFile(join(workspace, FIXTURE_SOURCE_PATH))
  const protectedInputs = protectedRepositoryInputs(workspace)
  if (!baselineCommit.ok || !baselineTree.ok || !source.regular ||
    protectedInputs.entries.some((entry) => !entry.regular)) {
    throw new Error('isolated_repository_baseline_capture_failed')
  }
  const validatorPath = join(harnessRoot, 'repository-test-validator.mjs')
  const receiptPath = join(harnessRoot, 'repository-test-receipt.json')
  const expectedStatus = ` M ${FIXTURE_SOURCE_PATH}\0`
  const baseAuthority = {
    contract: REPOSITORY_ACCEPTANCE_CONTRACT,
    nonce: randomBytes(32).toString('hex'),
    workspace: realpathSync(workspace),
    workspaceSha256: sha256(realpathSync(workspace)),
    validatorPath,
    receiptPath,
    baselineCommit: baselineCommit.stdout.trim(),
    baselineTree: baselineTree.stdout.trim(),
    baselineSourceSha256: source.sha256,
    expectedSourceSha256: sha256(FIXTURE_REPAIRED_SOURCE),
    expectedStatus,
    expectedStatusSha256: sha256(expectedStatus),
    protectedFiles: protectedInputs.entries.map(({ path, byteLength, sha256 }) => ({
      path,
      byteLength,
      sha256
    })),
    protectedInputsDigest: protectedInputs.digest,
    testScript: FIXTURE_TEST_SCRIPT,
    testScriptSha256: sha256(FIXTURE_TEST_SCRIPT)
  }
  writeFileSync(validatorPath, harnessOwnedRepositoryValidatorSource(baseAuthority), {
    encoding: 'utf8',
    mode: 0o400
  })
  chmodSync(validatorPath, 0o400)
  for (const relativePath of FIXTURE_PROTECTED_PATHS) {
    chmodSync(join(workspace, relativePath), 0o400)
  }
  const validator = hashRegularFile(validatorPath)
  if (!validator.regular) throw new Error('repository_validator_creation_failed')
  const authority = Object.freeze({
    ...baseAuthority,
    validatorSha256: validator.sha256
  })
  const baseline = verifyMilestoneARepositoryAcceptance(authority, 'baseline')
  if (!baseline.ok) throw new Error(baseline.blocker)
  repositoryContextMarkers.set(authority, contextMarkers)
  externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
    authority.workspace,
    'bash',
    canonicalJSON({ command: 'npm test' })
  ))
  externalHostToolExecutionArguments.add(hostToolArgumentAuthorityKey(
    authority.workspace,
    'read',
    canonicalJSON({ path: 'docs/acceptance-context.txt' })
  ))
  return authority
}

function privateRepositoryContextMarkers(authority) {
  const markers = repositoryContextMarkers.get(authority)
  const fixtureMarkersValid = markers &&
    /^MILESTONE_A_CONTEXT_FIRST_[0-9a-f]{48}$/.test(markers.first) &&
    /^MILESTONE_A_CONTEXT_OK_[0-9a-f]{48}$/.test(markers.completion) &&
    /^MILESTONE_A_CONTEXT_LAST_[0-9a-f]{48}$/.test(markers.last)
  const externalMarkersValid = markers &&
    externalRepositoryAuthorityShapeValid(authority) &&
    (authority.context.deprecated === true
      ? markers.first === authority.context.firstMarker &&
        markers.completion === authority.context.completionMarker &&
        markers.last === authority.context.lastMarker
      : markers.first === '' &&
        markers.completion === (authority.context.successMarker || '') &&
        markers.last === '')
  const validContinuation = externalRepositoryAuthorityShapeValid(authority) &&
    authority.context.deprecated !== true && markers && markers.first === '' &&
    markers.last === '' && typeof markers.completion === 'string'
  if ((!fixtureMarkersValid && !externalMarkersValid && !validContinuation) ||
    (validContinuation ? false : new Set(Object.values(markers || {})).size !== 3)) {
    throw new Error('repository_context_marker_authority_unavailable')
  }
  return markers
}

function resolveExecutableFromPath(name, pathValue) {
  for (const directory of String(pathValue || '').split(':').filter(Boolean)) {
    const candidate = join(directory, name)
    try {
      accessSync(candidate, constants.X_OK)
      const resolved = realpathSync(candidate)
      const identity = hashRegularFile(resolved, { maximumBytes: 16 * 1024 * 1024 })
      if (identity.regular) return { path: resolved, identity }
    } catch {
      // Continue through the fixed PATH inventory.
    }
  }
  return null
}

export function runParentOwnedRepositoryTest(authority) {
  const external = externalRepositoryAuthorityShapeValid(authority)
  const base = {
    passed: false,
    blocker: '',
    parentProcessOwned: true,
    command: external ? 'contract-bound-test-command' : 'npm test',
    commandDigest: '',
    executableSha256: '',
    cwdSha256: '',
    environmentKeyDigest: '',
    environmentDigest: '',
    exitCode: null,
    signal: '',
    durationMs: 0,
    stdoutSha256: '',
    stderrSha256: '',
    repositoryStableBeforeAndAfter: false,
    sandboxCleanupSucceeded: false
  }
  if (!repositoryAuthorityShapeValid(authority) && !external) {
    return { ...base, blocker: 'repository_acceptance_authority_invalid' }
  }
  const before = verifyRepositoryAcceptance(authority, 'final')
  if (!before.ok) return { ...base, blocker: before.blocker }
  const workspace = realpathSync(authority.workspace)
  const cacheTemp = trustedCacheTempRoot()
  if (!cacheTemp.ok) return { ...base, blocker: cacheTemp.blocker }
  const parentRoot = mkdtempSync(join(cacheTemp.path, 'analytix-milestone-a-parent-test-'))
  const isolatedHome = join(parentRoot, 'home')
  const npmCache = join(parentRoot, 'npm-cache')
  const tempRoot = join(parentRoot, 'tmp')
  for (const path of [isolatedHome, npmCache, tempRoot]) {
    mkdirSync(path, { recursive: true, mode: 0o700 })
    chmodSync(path, 0o700)
  }
  const pathValue = String(process.env.PATH || '/usr/bin:/bin:/usr/sbin:/sbin')
  const executable = external
    ? (() => {
        const path = '/bin/zsh'
        const identity = hashRegularFile(path, { maximumBytes: 16 * 1024 * 1024 })
        return identity.regular ? { path, identity } : null
      })()
    : resolveExecutableFromPath('npm', pathValue)
  if (!executable) {
    rmSync(parentRoot, { recursive: true, force: true })
    return { ...base, blocker: 'parent_owned_test_executable_unavailable' }
  }
  const environment = {
    PATH: pathValue,
    HOME: isolatedHome,
    USERPROFILE: isolatedHome,
    TMPDIR: tempRoot,
    TEMP: tempRoot,
    TMP: tempRoot,
    LANG: 'C',
    LC_ALL: 'C',
    NO_COLOR: '1',
    NPM_CONFIG_CACHE: npmCache,
    npm_config_cache: npmCache,
    NPM_CONFIG_USERCONFIG: '/dev/null',
    npm_config_userconfig: '/dev/null',
    NPM_CONFIG_AUDIT: 'false',
    NPM_CONFIG_FUND: 'false',
    NPM_CONFIG_UPDATE_NOTIFIER: 'false',
    npm_config_ignore_scripts: 'false'
  }
  if (process.platform === 'win32' && process.env.SystemRoot) {
    environment.SystemRoot = process.env.SystemRoot
  }
  for (const key of [
    'ANALYTIX_DEV_CACHE_ROOT',
    'GOCACHE',
    'GOMODCACHE',
    'GOTMPDIR',
    'CARGO_HOME',
    'CARGO_TARGET_DIR',
    'CCACHE_DIR',
    'SCCACHE_DIR',
    'XWIN_CACHE_DIR',
    'COREPACK_HOME',
    'ELECTRON_CACHE',
    'ELECTRON_BUILDER_CACHE',
    'PLAYWRIGHT_BROWSERS_PATH',
    'NODE_COMPILE_CACHE',
    'XDG_CACHE_HOME',
    'PIP_CACHE_DIR',
    'UV_CACHE_DIR',
    'PYTHONPYCACHEPREFIX',
    'MYPY_CACHE_DIR',
    'RUFF_CACHE_DIR'
  ]) {
    if (typeof process.env[key] === 'string' && process.env[key]) {
      environment[key] = process.env[key]
    }
  }
  const commandArgs = external ? ['-c', authority.testCommand] : ['test']
  const environmentKeys = Object.keys(environment).sort()
  const environmentDigest = sha256(canonicalJSON(environmentKeys.map((key) => ({
    key,
    valueSha256: sha256(String(environment[key]))
  }))))
  const commandDigest = sha256(canonicalJSON({
    executablePathSha256: sha256(executable.path),
    executableSha256: executable.identity.sha256,
    args: commandArgs,
    cwdSha256: sha256(workspace),
    environmentDigest
  }))
  const startedAt = Date.now()
  const result = spawnSync(executable.path, commandArgs, {
    cwd: workspace,
    env: environment,
    encoding: 'buffer',
    stdio: 'pipe',
    timeout: external ? authority.testTimeoutMs : 60_000,
    maxBuffer: 4 * 1024 * 1024
  })
  const durationMs = Date.now() - startedAt
  const after = verifyRepositoryAcceptance(authority, 'final')
  const repositoryStableBeforeAndAfter = before.ok && after.ok &&
    before.gitStatusSha256 === after.gitStatusSha256 &&
    before.sourceSha256 === after.sourceSha256 &&
    before.protectedInputsDigest === after.protectedInputsDigest
  let sandboxCleanupSucceeded = true
  try {
    rmSync(parentRoot, { recursive: true, force: true })
    sandboxCleanupSucceeded = !existsSync(parentRoot)
  } catch {
    sandboxCleanupSucceeded = false
  }
  const passed = result.status === 0 && !result.signal && !result.error &&
    repositoryStableBeforeAndAfter && sandboxCleanupSucceeded
  let blocker = ''
  if (!passed) {
    if (!repositoryStableBeforeAndAfter) blocker = after.blocker || 'parent_owned_test_changed_repository'
    else if (!sandboxCleanupSucceeded) blocker = 'parent_owned_npm_test_cleanup_failed'
    else if (result.error?.code === 'ETIMEDOUT') blocker = 'parent_owned_repository_test_timed_out'
    else blocker = 'parent_owned_repository_test_failed'
  }
  return {
    ...base,
    passed,
    blocker,
    commandDigest,
    executableSha256: executable.identity.sha256,
    cwdSha256: sha256(workspace),
    environmentKeyDigest: sha256(canonicalJSON(environmentKeys)),
    environmentDigest,
    exitCode: Number.isInteger(result.status) ? result.status : null,
    signal: String(result.signal || ''),
    durationMs,
    stdoutSha256: sha256(result.stdout || Buffer.alloc(0)),
    stderrSha256: sha256(result.stderr || Buffer.alloc(0)),
    repositoryStableBeforeAndAfter,
    sandboxCleanupSucceeded
  }
}

function writeIsolatedSettings({
  userDataDir,
  runtimeDataDir,
  workspace,
  writeWorkspace,
  scheduleWorkspace,
  clawWorkspace,
  runtimePort,
  schedulePort,
  packagedSkillRoot
}) {
  mkdirSync(userDataDir, { recursive: true, mode: 0o700 })
  mkdirSync(runtimeDataDir, { recursive: true, mode: 0o700 })
  for (const path of [workspace, writeWorkspace, scheduleWorkspace, clawWorkspace]) {
    mkdirSync(path, { recursive: true, mode: 0o700 })
  }
  const formalExecutionBounds = milestoneAFormalExecutionBounds()
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
      runtimeTuning: {
        stepLimits: {
          userGlobalMaxModelSteps: formalExecutionBounds.userGlobalMaxModelSteps,
          plannerMaxModelSteps: formalExecutionBounds.plannerMaxModelSteps
        }
      },
      subagents: {
        enabled: true,
        maxParallel: 2,
        maxChildRuns: 2,
        defaultToolPolicy: 'readOnly',
        defaultProfile: PACKAGED_READ_ONLY_SUBAGENT_PROFILE,
        profiles: {
          [PACKAGED_READ_ONLY_SUBAGENT_PROFILE]: {
            prompt: 'Review only the named repository files and test intent. Do not mutate state.',
            toolPolicy: 'readOnly',
            tools: ['read']
          }
        }
      },
      computerUse: {
        enabled: false,
        mode: 'off',
        maxImageDimension: 1280,
        maxActionsPerTurn: 40,
        allowWhenLocked: false
      }
    },
    write: {
      defaultWorkspaceRoot: writeWorkspace,
      activeWorkspaceRoot: writeWorkspace,
      workspaces: [writeWorkspace]
    },
    schedule: {
      defaultWorkspaceRoot: scheduleWorkspace,
      internal: {
        port: schedulePort,
        secret: ''
      },
      tasks: []
    },
    claw: {
      enabled: false,
      skills: {
        defaultNames: [],
        extraDirs: [packagedSkillRoot],
        disabledDirs: [...ISOLATED_DISABLED_SKILL_DIRS],
        promptPrefix: ''
      },
      im: {
        enabled: false,
        workspaceRoot: clawWorkspace
      },
      channels: [],
      tasks: []
    }
  }
  writeFileSync(settingsPath, JSON.stringify(settings), { encoding: 'utf8', mode: 0o600 })
  chmodSync(settingsPath, 0o600)
  return settingsPath
}

export function prepareMissingBundledFundsMaterializationAuthoritySeam(runtimeDataDir) {
  if (!isAbsolute(runtimeDataDir) || resolve(runtimeDataDir) !== runtimeDataDir ||
    realpathSync(runtimeDataDir) !== runtimeDataDir) {
    throw new Error('packaged_funds_materialization_runtime_data_root_invalid')
  }
  const runtimeDataStat = lstatSync(runtimeDataDir)
  if (!runtimeDataStat.isDirectory() || runtimeDataStat.isSymbolicLink() ||
    (runtimeDataStat.mode & 0o777) !== 0o700 ||
    (typeof process.getuid === 'function' && runtimeDataStat.uid !== process.getuid())) {
    throw new Error('packaged_funds_materialization_runtime_data_root_unsafe')
  }
  const stateRoot = join(runtimeDataDir, ...BUNDLED_FUNDS_MATERIALIZATION_STATE_COMPONENTS)
  const authorityPath = join(
    runtimeDataDir,
    ...BUNDLED_FUNDS_MATERIALIZATION_AUTHORITY_COMPONENTS
  )
  if (existsSync(authorityPath)) {
    throw new Error('packaged_funds_materialization_authority_preexists')
  }
  mkdirSync(stateRoot, { recursive: true, mode: 0o700 })
  const stateStat = lstatSync(stateRoot)
  if (!stateStat.isDirectory() || stateStat.isSymbolicLink() ||
    realpathSync(stateRoot) !== stateRoot || (stateStat.mode & 0o777) !== 0o700 ||
    (typeof process.getuid === 'function' && stateStat.uid !== process.getuid()) ||
    readdirSync(stateRoot).length !== 0) {
    throw new Error('packaged_funds_materialization_missing_authority_seam_invalid')
  }
  return Object.freeze({
    stateRoot,
    authorityPath,
    stateIdentity: Object.freeze({
      dev: stateStat.dev,
      ino: stateStat.ino,
      uid: stateStat.uid,
      mode: stateStat.mode
    }),
    evidence: Object.freeze({
      preconditionEstablished: true,
      stateRootSha256: sha256(stateRoot),
      authorityPathSha256: sha256(authorityPath),
      stateDirectoryEmpty: true,
      authorityAbsent: true
    })
  })
}

export function missingBundledFundsMaterializationAuthorityEvidence(seam) {
  const base = {
    ok: false,
    stateRootSha256: String(seam?.evidence?.stateRootSha256 || ''),
    authorityPathSha256: String(seam?.evidence?.authorityPathSha256 || ''),
    stateDirectoryIdentityPreserved: false,
    stateDirectoryEmpty: false,
    authorityAbsent: false
  }
  try {
    const stateStat = lstatSync(seam.stateRoot)
    const identity = seam.stateIdentity
    const stateDirectoryIdentityPreserved = stateStat.isDirectory() &&
      !stateStat.isSymbolicLink() && realpathSync(seam.stateRoot) === seam.stateRoot &&
      stateStat.dev === identity.dev && stateStat.ino === identity.ino &&
      stateStat.uid === identity.uid && stateStat.mode === identity.mode
    const stateDirectoryEmpty = readdirSync(seam.stateRoot).length === 0
    const authorityAbsent = !existsSync(seam.authorityPath)
    return {
      ...base,
      ok: stateDirectoryIdentityPreserved && stateDirectoryEmpty && authorityAbsent,
      stateDirectoryIdentityPreserved,
      stateDirectoryEmpty,
      authorityAbsent
    }
  } catch {
    return base
  }
}

function safeChildEnvironment({ isolatedHome, userDataDir, chromiumTempDir }) {
  const allowed = new Set([
    'PATH',
    'SHELL',
    'LANG',
    'LC_ALL',
    'LC_CTYPE',
    'SystemRoot',
    'TMPDIR',
    'TEMP',
    'TMP',
    'ANALYTIX_DEV_CACHE_ROOT',
    'npm_config_cache',
    'XDG_CACHE_HOME',
    'PIP_CACHE_DIR',
    'UV_CACHE_DIR',
    'PYTHONPYCACHEPREFIX',
    'MYPY_CACHE_DIR',
    'RUFF_CACHE_DIR',
    'GOCACHE',
    'GOMODCACHE',
    'GOTMPDIR',
    'CARGO_HOME',
    'CARGO_TARGET_DIR',
    'CCACHE_DIR',
    'SCCACHE_DIR',
    'XWIN_CACHE_DIR',
    'COREPACK_HOME',
    'ELECTRON_CACHE',
    'ELECTRON_BUILDER_CACHE',
    'PLAYWRIGHT_BROWSERS_PATH',
    'NODE_COMPILE_CACHE'
  ])
  const childEnv = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (allowed.has(key) && value !== undefined) childEnv[key] = value
  }
  return {
    ...childEnv,
    HOME: isolatedHome,
    USERPROFILE: isolatedHome,
    TMPDIR: chromiumTempDir,
    TEMP: chromiumTempDir,
    TMP: chromiumTempDir,
    ...(process.platform === 'darwin' ? { MAC_CHROMIUM_TMPDIR: chromiumTempDir } : {}),
    ANALYTIX_USER_DATA_DIR: userDataDir,
    ANALYTIX_RUNTIME_BACKEND: 'go-runtime-default',
    ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A: '1'
  }
}

function createRandomHexPasswordBuffer() {
  const entropy = randomBytes(32)
  if (!Buffer.isBuffer(entropy) || entropy.length !== 32) {
    throw new Error('isolated_login_keychain_random_source_invalid')
  }
  const alphabet = Buffer.from('0123456789abcdef', 'ascii')
  const password = Buffer.alloc(entropy.length * 2)
  for (let index = 0; index < entropy.length; index += 1) {
    password[index * 2] = alphabet[entropy[index] >> 4]
    password[(index * 2) + 1] = alphabet[entropy[index] & 0x0f]
  }
  entropy.fill(0)
  return password
}

function ensureExactOwnerOnlyDirectory(path) {
  try {
    const existing = lstatSync(path)
    if (!existing.isDirectory() || existing.isSymbolicLink()) {
      throw new Error('isolated_login_keychain_directory_invalid')
    }
  } catch (error) {
    if (error?.code !== 'ENOENT') throw error
    mkdirSync(path, { mode: 0o700 })
  }
  chmodSync(path, 0o700)
  if (!ownerOnlyDirectory(path, true)) {
    throw new Error('isolated_login_keychain_directory_not_owner_only')
  }
}

function ownerOnlyRegularFile(path) {
  try {
    const stat = lstatSync(path)
    const uid = ownerUID()
    return stat.isFile() && !stat.isSymbolicLink() && stat.nlink === 1 &&
      (uid === null || stat.uid === uid) && (stat.mode & 0o077) === 0
  } catch {
    return false
  }
}

function isolatedKeychainEnvironment(isolatedHome) {
  return {
    PATH: '/usr/bin:/bin:/usr/sbin:/sbin',
    HOME: isolatedHome,
    USERPROFILE: isolatedHome,
    LANG: 'C',
    LC_ALL: 'C'
  }
}

function isolatedDefaultKeychainEvidence({ isolatedHome, databasePath }) {
  const result = spawnSync(DARWIN_SECURITY_PATH, ['default-keychain'], {
    cwd: isolatedHome,
    env: isolatedKeychainEnvironment(isolatedHome),
    encoding: 'utf8',
    stdio: 'pipe',
    timeout: 5000,
    maxBuffer: 4096
  })
  const observed = String(result.stdout || '').trim().replace(/^"|"$/gu, '')
  let observedRealPath = ''
  try {
    observedRealPath = realpathSync(observed)
  } catch {
    observedRealPath = ''
  }
  return {
    ok: result.status === 0 && !result.signal && !result.error &&
      observedRealPath === realpathSync(databasePath),
    observedPathHash: observedRealPath ? sha256(observedRealPath) : ''
  }
}

function expectSecurityScript(operation, keychainPath) {
  if (operation !== 'create-keychain' && operation !== 'unlock-keychain') {
    throw new Error('isolated_login_keychain_operation_invalid')
  }
  if (/[^\x20-\x7e]/u.test(keychainPath)) {
    throw new Error('isolated_login_keychain_path_invalid')
  }
  const pathHex = Buffer.from(keychainPath, 'utf8').toString('hex')
  return [
    'set timeout 20',
    'log_user 0',
    'set secret [gets stdin]',
    'if {[string length $secret] < 32} { exit 126 }',
    `set keychain [encoding convertfrom utf-8 [binary format H* {${pathHex}}]]`,
    `# task-owned-keychain ${keychainPath}`,
    'set prompts 0',
    `spawn -noecho ${DARWIN_SECURITY_PATH} ${operation} -- $keychain`,
    'expect {',
    '  -re {(?i)password[^\\r\\n]*:} { incr prompts; send -- "$secret\\r"; exp_continue }',
    '  eof {}',
    '  timeout { exit 124 }',
    '}',
    'set secret {}',
    'set waited [wait]',
    'if {$prompts < 1} { exit 123 }',
    'set code [lindex $waited 3]',
    'if {![string is integer -strict $code]} { exit 125 }',
    'exit $code',
    ''
  ].join('\n')
}

async function runSecurityWithTaskOwnedPTY({
  operation,
  keychainPath,
  isolatedHome,
  password
}) {
  const expectIdentity = hashRegularFile(DARWIN_EXPECT_PATH, {
    maximumBytes: 16 * 1024 * 1024
  })
  const securityIdentity = hashRegularFile(DARWIN_SECURITY_PATH, {
    maximumBytes: 16 * 1024 * 1024
  })
  if (!expectIdentity.regular || !securityIdentity.regular ||
    !Buffer.isBuffer(password) || password.length !== 64) {
    throw new Error('isolated_login_keychain_pty_prerequisite_invalid')
  }
  const child = spawn(
    DARWIN_EXPECT_PATH,
    ['-c', expectSecurityScript(operation, keychainPath)],
    {
      cwd: isolatedHome,
      env: isolatedKeychainEnvironment(isolatedHome),
      stdio: ['pipe', 'ignore', 'ignore']
    }
  )
  const outcome = await new Promise((resolvePromise) => {
    let timedOut = false
    let killTimer = null
    const timeout = setTimeout(() => {
      timedOut = true
      child.kill('SIGTERM')
      killTimer = setTimeout(() => {
        if (child.exitCode === null && !child.signalCode) child.kill('SIGKILL')
      }, 1000)
    }, ISOLATED_KEYCHAIN_OPERATION_TIMEOUT_MS)
    child.once('error', () => {
      clearTimeout(timeout)
      if (killTimer) clearTimeout(killTimer)
      resolvePromise({ ok: false, timedOut: false })
    })
    child.once('close', (code, signal) => {
      clearTimeout(timeout)
      if (killTimer) clearTimeout(killTimer)
      resolvePromise({
        ok: !timedOut && code === 0 && !signal,
        timedOut
      })
    })
    child.stdin.on('error', () => {
      // The close/error handlers above own the fail-closed result.
    })
    child.stdin.write(password)
    child.stdin.end(Buffer.from('\n', 'ascii'))
  })
  if (!outcome.ok) {
    throw new Error(outcome.timedOut
      ? 'isolated_login_keychain_pty_timed_out'
      : `isolated_login_keychain_${operation.replace('-', '_')}_failed`)
  }
  return {
    expectSha256: expectIdentity.sha256,
    securitySha256: securityIdentity.sha256
  }
}

export async function createIsolatedDarwinLoginKeychain(isolatedHome) {
  if (process.platform !== 'darwin' || !isAbsolute(isolatedHome)) {
    throw new Error('isolated_login_keychain_requires_darwin_absolute_home')
  }
  const home = realpathSync(isolatedHome)
  const trustedTemp = trustedCacheTempRoot()
  if (!trustedTemp.ok || home !== resolve(isolatedHome) ||
    pathIsOutside(trustedTemp.path, home) || !ownerOnlyDirectory(home, true)) {
    throw new Error('isolated_login_keychain_home_invalid')
  }
  const libraryPath = join(home, 'Library')
  const preferencesPath = join(libraryPath, 'Preferences')
  const keychainsPath = join(libraryPath, 'Keychains')
  for (const path of [libraryPath, preferencesPath, keychainsPath]) {
    ensureExactOwnerOnlyDirectory(path)
  }
  const requestedPath = join(keychainsPath, ISOLATED_LOGIN_KEYCHAIN_NAME)
  const databasePath = `${requestedPath}-db`
  if (existsSync(requestedPath) || existsSync(databasePath)) {
    throw new Error('isolated_login_keychain_preexisting_state_rejected')
  }
  const password = createRandomHexPasswordBuffer()
  let disposed = false
  let unlockCount = 0
  let executableEvidence = null
  let defaultEvidence = { ok: false, observedPathHash: '' }
  const retireCreatedKeychain = () => {
    if (!ownerOnlyDirectory(keychainsPath, true) ||
      !ownerOnlyRegularFile(databasePath) ||
      realpathSync(databasePath) !== databasePath) {
      throw new Error('isolated_login_keychain_cleanup_identity_invalid')
    }
    // `security create-keychain` also adds this task Keychain to the macOS
    // search list. Removing only its directory leaves a stale global entry.
    const result = spawnSync(DARWIN_SECURITY_PATH, ['delete-keychain', requestedPath], {
      cwd: home,
      env: isolatedKeychainEnvironment(home),
      encoding: 'utf8',
      stdio: 'pipe',
      timeout: 5000,
      maxBuffer: 4096
    })
    if (result.error || result.signal || result.status !== 0 ||
      existsSync(requestedPath) || existsSync(databasePath)) {
      throw new Error('isolated_login_keychain_cleanup_failed')
    }
  }
  const snapshot = () => Object.freeze({
    ok: !disposed && ownerOnlyRegularFile(databasePath) && defaultEvidence.ok,
    created: ownerOnlyRegularFile(databasePath),
    directoriesOwnerOnly: [libraryPath, preferencesPath, keychainsPath]
      .every((path) => ownerOnlyDirectory(path, true)),
    defaultKeychainBound: defaultEvidence.ok,
    unlockCount,
    reusedForTwoLaunches: unlockCount >= 2,
    isolatedHomePathHash: sha256(home),
    requestedPathHash: sha256(requestedPath),
    databasePathHash: sha256(databasePath),
    observedDefaultPathHash: defaultEvidence.observedPathHash,
    expectExecutableSha256: executableEvidence?.expectSha256 || '',
    securityExecutableSha256: executableEvidence?.securitySha256 || '',
    passwordTransport: 'process-memory-to-stdin-to-task-owned-pty',
    passwordInArgv: false,
    passwordInEnvironment: false,
    passwordInFile: false,
    passwordRecorded: false
  })
  try {
    executableEvidence = await runSecurityWithTaskOwnedPTY({
      operation: 'create-keychain',
      keychainPath: requestedPath,
      isolatedHome: home,
      password
    })
    if (!ownerOnlyRegularFile(databasePath)) {
      throw new Error('isolated_login_keychain_database_invalid')
    }
    defaultEvidence = isolatedDefaultKeychainEvidence({
      isolatedHome: home,
      databasePath
    })
    if (!defaultEvidence.ok) {
      throw new Error('isolated_login_keychain_default_binding_failed')
    }
  } catch (error) {
    password.fill(0)
    disposed = true
    if (ownerOnlyRegularFile(databasePath)) retireCreatedKeychain()
    throw error
  }
  return Object.freeze({
    evidence: snapshot,
    async unlockForLaunch() {
      if (disposed) throw new Error('isolated_login_keychain_disposed')
      executableEvidence = await runSecurityWithTaskOwnedPTY({
        operation: 'unlock-keychain',
        keychainPath: requestedPath,
        isolatedHome: home,
        password
      })
      defaultEvidence = isolatedDefaultKeychainEvidence({
        isolatedHome: home,
        databasePath
      })
      if (!defaultEvidence.ok || !ownerOnlyRegularFile(databasePath)) {
        throw new Error('isolated_login_keychain_unlock_binding_failed')
      }
      unlockCount += 1
      return snapshot()
    },
    dispose() {
      if (disposed) return
      password.fill(0)
      retireCreatedKeychain()
      disposed = true
    }
  })
}

function privateCaseAuthorityInputEvidence(childEnv, settingsPath) {
  const protectedName = /(?:case|dataset[_-]?snapshot|evidence|controlled[_-]?artifact|pii|funds)/iu
  const environmentKeys = Object.keys(childEnv || {})
    .filter((key) => key.startsWith('ANALYTIX_') && protectedName.test(key))
    .sort()
  let settings = null
  try {
    const document = hashRegularFile(settingsPath, { capture: true, maximumBytes: 1024 * 1024 })
    if (document.regular && document.content) {
      settings = JSON.parse(document.content.toString('utf8'))
    }
  } catch {
    settings = null
  }
  const settingsKeys = []
  const pending = settings && typeof settings === 'object' ? [settings] : []
  while (pending.length > 0) {
    const value = pending.pop()
    if (!value || typeof value !== 'object') continue
    for (const [key, item] of Object.entries(value)) {
      if (protectedName.test(key)) settingsKeys.push(key)
      if (item && typeof item === 'object') pending.push(item)
    }
  }
  let preLaunchUnexpectedUserDataEntryCount = 1
  try {
    preLaunchUnexpectedUserDataEntryCount = readdirSync(dirname(settingsPath))
      .filter((name) => name !== 'analytix-settings.json')
      .length
  } catch {
    preLaunchUnexpectedUserDataEntryCount = 1
  }
  return {
    privateAuthorityInputsAbsent:
      settings !== null && environmentKeys.length === 0 && settingsKeys.length === 0 &&
      preLaunchUnexpectedUserDataEntryCount === 0,
    privateAuthorityEnvironmentKeyCount: environmentKeys.length,
    privateAuthoritySettingsKeyCount: settingsKeys.length,
    preLaunchUnexpectedUserDataEntryCount
  }
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

async function waitForDebugTarget(port, timeoutMs) {
  const deadline = Date.now() + timeoutMs
  let lastPageTargetCount = 0
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/list`, {
        signal: AbortSignal.timeout(1000)
      })
      if (response.ok) {
        const targets = await response.json()
        const pageTargets = Array.isArray(targets)
          ? targets.filter((item) => item?.type === 'page')
          : []
        lastPageTargetCount = pageTargets.length
        if (pageTargets.length === 1 && pageTargets[0]?.webSocketDebuggerUrl) {
          return { ...pageTargets[0], pageTargetCount: pageTargets.length }
        }
      }
    } catch {
      // The packaged renderer is still starting.
    }
    await sleep(250)
  }
  if (lastPageTargetCount > 0) {
    throw new Error('packaged_renderer_target_count_not_one')
  }
  throw new Error('packaged_renderer_debug_target_not_ready')
}

function readonlyCdpFailureError(reasonCode, metadata = {}) {
  const safeReasonCode = READONLY_CDP_FAILURE_REASON_CODES.includes(reasonCode)
    ? reasonCode
    : 'readonly_cdp_evaluation_failed'
  const error = new Error(safeReasonCode)
  error.readonlyCdpReasonCode = safeReasonCode
  if (READONLY_COMPOSER_MODE_READ_ERROR_STAGES.includes(metadata?.expressionStage) ||
      READONLY_COMPOSER_GEOMETRY_READ_ERROR_STAGES.includes(metadata?.expressionStage) ||
      READONLY_OBSERVATION_READ_ERROR_STAGES.includes(metadata?.expressionStage)) {
    error.readonlyCdpExpressionStage = metadata.expressionStage
  }
  return error
}

export function managedWorkbenchReadDiagnostic(observation, rendererTargetCount = 0) {
  const value = observation && typeof observation === 'object' && !Array.isArray(observation)
    ? observation
    : {}
  const account = value.account && typeof value.account === 'object' && !Array.isArray(value.account)
    ? value.account
    : null
  const readErrorStage = safeReadonlyObservationReadErrorStage(value.readErrorStage)
  return Object.freeze({
    observed: Object.keys(value).length > 0,
    rendererTargetCount: safeNonNegativeCount(rendererTargetCount),
    readErrorStage,
    apiPresent: value.apiPresent === true,
    composerPresent: value.composerPresent === true,
    primaryButtonPresent: value.primaryButtonPresent === true,
    accountObserved: Boolean(account),
    accountAuthenticated: account?.authenticated === true,
    accountGatewayConfigured: account?.gatewayConfigured === true,
    accountReady: account?.accountReady === true,
    accountUserReady: account?.userAccountReady === true,
    accountSourceHub: account?.source === 'hub',
    healthObserved: Boolean(value.health && typeof value.health === 'object'),
    runtimeInfoObserved: Boolean(value.runtimeInfo && typeof value.runtimeInfo === 'object'),
    runtimeToolsObserved: Boolean(value.runtimeTools && typeof value.runtimeTools === 'object'),
    runtimeThreadListProbeOk: value.runtimeThreadListProbeOk === true,
    runtimeThreadListProbeStatus: safeNonNegativeCount(value.runtimeThreadListProbeStatus)
  })
}

function safeReadonlyObservationReadErrorStage(value) {
  return READONLY_OBSERVATION_READ_ERROR_STAGES.includes(value) ? value : 'unknown'
}

function safePlanObservationPhase(value) {
  return PLAN_OBSERVATION_PHASE_CODES.includes(value) ? value : 'unknown'
}

function safePlanObservationOperation(value) {
  return PLAN_OBSERVATION_OPERATION_CODES.includes(value) ? value : 'unknown'
}

function planObservationExpressionStage(value) {
  if (value === 'none') return 'none'
  return safeReadonlyObservationReadErrorStage(value)
}

function planObservationReasonCode(value) {
  return READONLY_CDP_FAILURE_REASON_CODES.includes(value) ? value : ''
}

export function applyPlanObservationDiagnostic(report, diagnostic) {
  if (!report || typeof report !== 'object') return report
  if (!report.workflow || typeof report.workflow !== 'object') report.workflow = {}
  const value = diagnostic && typeof diagnostic === 'object' ? diagnostic : {}
  report.workflow.planObservationFailureReasonCode = planObservationReasonCode(
    value.reasonCode
  )
  report.workflow.planObservationFailurePhase = safePlanObservationPhase(
    value.phase === undefined ? 'none' : value.phase
  )
  report.workflow.planObservationFailureOperation = safePlanObservationOperation(
    value.operation === undefined ? 'none' : value.operation
  )
  report.workflow.planObservationFailureExpressionStage = planObservationExpressionStage(
    value.expressionStage === undefined ? 'none' : value.expressionStage
  )
  return report
}

function safeComposerModePhase(value) {
  return READONLY_COMPOSER_MODE_PHASE_CODES.includes(value) ? value : 'unknown'
}

function safeComposerModeOperation(value) {
  return READONLY_COMPOSER_MODE_OPERATION_CODES.includes(value) ? value : 'unknown'
}

function safeComposerModeExpressionStage(value) {
  if (value === 'none') return 'none'
  return READONLY_COMPOSER_MODE_READ_ERROR_STAGES.includes(value) ? value : 'unknown'
}

function safeComposerModeReasonCode(value, fallback = 'composer_mode_selection_failed') {
  return COMPOSER_MODE_REASON_CODES.includes(value)
    ? value
    : (COMPOSER_MODE_REASON_CODES.includes(fallback)
        ? fallback
        : 'composer_mode_selection_failed')
}

function composerModeDiagnostic({
  reasonCode = '',
  phase = 'none',
  operation = 'none',
  expressionStage = 'none'
} = {}) {
  return Object.freeze({
    reasonCode: safeComposerModeReasonCode(reasonCode, ''),
    phase: safeComposerModePhase(phase),
    operation: safeComposerModeOperation(operation),
    expressionStage: safeComposerModeExpressionStage(expressionStage)
  })
}

export function applyComposerModeDiagnostic(report, modeResult) {
  if (!report || typeof report !== 'object') return report
  if (!report.workflow || typeof report.workflow !== 'object') report.workflow = {}
  const diagnostic = modeResult?.diagnostic && typeof modeResult.diagnostic === 'object'
    ? modeResult.diagnostic
    : {}
  report.workflow.composerModeFailureReasonCode = safeComposerModeReasonCode(
    diagnostic.reasonCode || modeResult?.blocker || '',
    ''
  )
  report.workflow.composerModeFailurePhase = safeComposerModePhase(diagnostic.phase)
  report.workflow.composerModeFailureOperation = safeComposerModeOperation(diagnostic.operation)
  report.workflow.composerModeFailureExpressionStage = safeComposerModeExpressionStage(
    diagnostic.expressionStage
  )
  return report
}

function safeComposerSubmitPhase(value) {
  return COMPOSER_SUBMIT_PHASE_CODES.includes(value) ? value : 'unknown'
}

function safeComposerSubmitOperation(value) {
  return COMPOSER_SUBMIT_OPERATION_CODES.includes(value) ? value : 'unknown'
}

function safeComposerSubmitExpressionStage(value) {
  if (value === 'none') return 'none'
  return READONLY_COMPOSER_GEOMETRY_READ_ERROR_STAGES.includes(value) ? value : 'unknown'
}

function safeComposerSubmitReasonCode(value, fallback = 'composer_submit_failed') {
  return COMPOSER_SUBMIT_REASON_CODES.includes(value)
    ? value
    : (COMPOSER_SUBMIT_REASON_CODES.includes(fallback) ? fallback : 'composer_submit_failed')
}

function composerSubmitDiagnostic({
  reasonCode = '',
  phase = 'none',
  operation = 'none',
  expressionStage = 'none'
} = {}) {
  return Object.freeze({
    reasonCode: safeComposerSubmitReasonCode(reasonCode, ''),
    phase: safeComposerSubmitPhase(phase),
    operation: safeComposerSubmitOperation(operation),
    expressionStage: safeComposerSubmitExpressionStage(expressionStage)
  })
}

function composerSubmitExpressionStageFromError(error) {
  try {
    return safeComposerSubmitExpressionStage(error?.readonlyCdpExpressionStage || 'none')
  } catch {
    return 'unknown'
  }
}

export function applyComposerSubmitDiagnostic(report, submitResult) {
  if (!report || typeof report !== 'object') return report
  if (!report.workflow || typeof report.workflow !== 'object') report.workflow = {}
  const diagnostic = submitResult?.diagnostic && typeof submitResult.diagnostic === 'object'
    ? submitResult.diagnostic
    : {}
  report.workflow.composerSubmitFailureReasonCode = safeComposerSubmitReasonCode(
    diagnostic.reasonCode || submitResult?.blocker || '',
    ''
  )
  report.workflow.composerSubmitFailurePhase = safeComposerSubmitPhase(diagnostic.phase)
  report.workflow.composerSubmitFailureOperation = safeComposerSubmitOperation(
    diagnostic.operation
  )
  report.workflow.composerSubmitFailureExpressionStage = safeComposerSubmitExpressionStage(
    diagnostic.expressionStage
  )
  return report
}

const ORDINARY_WORKFLOW_DIRECT_FAILURE_CODES = new Set([
  'packaged_ordinary_host_post_response_failure',
  'packaged_ordinary_provider_transport_succeeded_terminal_failure'
])

export function readonlyCdpFailureReasonCode(value) {
  const explicit = value?.readonlyCdpReasonCode || value?.reasonCode
  if (READONLY_CDP_FAILURE_REASON_CODES.includes(explicit)) return explicit
  const explicitMessage = value instanceof Error
    ? value.message
    : typeof value?.message === 'string' ? value.message : ''

  const result = value?.result && typeof value.result === 'object' ? value.result : null
  const exceptionDetails = value?.exceptionDetails || result?.exceptionDetails
  if (exceptionDetails && typeof exceptionDetails === 'object') {
    const exceptionText = [exceptionDetails.text, exceptionDetails.description,
      exceptionDetails.exception?.description, exceptionDetails.exception?.value]
      .filter((item) => typeof item === 'string')
      .join(' ')
    if (/(?:execution context (?:was )?destroyed|cannot find context with specified id|world was destroyed)/iu.test(
      exceptionText
    )) {
      return READONLY_CDP_TRANSIENT_REASON_CODE
    }
    return 'readonly_cdp_exception'
  }
  if (READONLY_CDP_FAILURE_REASON_CODES.includes(explicitMessage)) return explicitMessage

  const protocolError = value?.error && typeof value.error === 'object'
    ? value.error
    : null
  const protocolMessage = typeof protocolError?.message === 'string'
    ? protocolError.message
    : explicitMessage
  const protocolCode = protocolError?.code
  const contextUnavailable = protocolCode === -32000 &&
    /(execution context|context with specified id|context was destroyed|world was destroyed)/iu.test(
      protocolMessage
    )
  if (contextUnavailable ||
      /(execution context (?:was )?destroyed|cannot find context with specified id|world was destroyed)/iu.test(
        protocolMessage
      )) {
    return READONLY_CDP_TRANSIENT_REASON_CODE
  }
  if (protocolCode === -32602 || /invalid params|invalid parameter/iu.test(protocolMessage)) {
    return 'readonly_cdp_protocol_invalid_params'
  }
  if (protocolError) return 'readonly_cdp_protocol_error'
  if (value?.timeout === true || explicit === 'readonly_cdp_timeout') {
    return 'readonly_cdp_timeout'
  }
  if (value?.transportUnavailable === true || explicit === 'readonly_cdp_transport_unavailable') {
    return 'readonly_cdp_transport_unavailable'
  }
  if (value?.targetUnavailable === true || explicit === 'readonly_cdp_target_unavailable') {
    return 'readonly_cdp_target_unavailable'
  }
  return 'readonly_cdp_evaluation_failed'
}

function safeErrorDiagnosticCode(error, fallback = 'milestone_a_execution_failed') {
  const reasonCode = error?.readonlyCdpReasonCode
  if (READONLY_CDP_FAILURE_REASON_CODES.includes(reasonCode)) return reasonCode
  const message = error instanceof Error ? error.message : ''
  if (ORDINARY_WORKFLOW_DIRECT_FAILURE_CODES.has(message)) return message
  return READONLY_CDP_FAILURE_REASON_CODES.includes(message) ? message : fallback
}

export async function withFreshReadonlyCdpTargetRetry({
  getTarget,
  evaluate,
  sleep: sleepDependency,
  isTransientResult,
  maxRetries = READONLY_CDP_MAX_TRANSIENT_RETRIES,
  retryDelayMs = READONLY_CDP_RETRY_DELAY_MS
} = {}) {
  if (typeof getTarget !== 'function' || typeof evaluate !== 'function') {
    throw readonlyCdpFailureError('readonly_cdp_target_unavailable')
  }
  const retryLimit = Number.isSafeInteger(maxRetries) && maxRetries >= 0
    ? Math.min(maxRetries, READONLY_CDP_MAX_TRANSIENT_RETRIES)
    : READONLY_CDP_MAX_TRANSIENT_RETRIES
  const delayMs = Number.isSafeInteger(retryDelayMs) && retryDelayMs >= 0
    ? Math.min(retryDelayMs, 1000)
    : READONLY_CDP_RETRY_DELAY_MS
  const sleepForRetry = typeof sleepDependency === 'function' ? sleepDependency : sleep
  let retryCount = 0
  while (true) {
    let target
    try {
      target = await getTarget()
    } catch (error) {
      throw readonlyCdpFailureError('readonly_cdp_target_unavailable')
    }
    if (!target?.webSocketDebuggerUrl || target?.pageTargetCount !== 1) {
      throw readonlyCdpFailureError('readonly_cdp_target_unavailable')
    }
    try {
      const value = await evaluate(target)
      if (typeof isTransientResult === 'function' && isTransientResult(value)) {
        throw readonlyCdpFailureError(READONLY_CDP_TRANSIENT_REASON_CODE)
      }
      return { target, value, retryCount }
    } catch (error) {
      const reasonCode = readonlyCdpFailureReasonCode(error)
      if (reasonCode !== READONLY_CDP_TRANSIENT_REASON_CODE || retryCount >= retryLimit) {
        throw readonlyCdpFailureError(reasonCode)
      }
      retryCount += 1
      await sleepForRetry(delayMs)
    }
  }
}

async function evaluateReadonlyCdp(wsUrl, expression, timeoutMs) {
  if (typeof WebSocket === 'undefined') {
    throw readonlyCdpFailureError('readonly_cdp_transport_unavailable')
  }
  let socket = null
  try {
    socket = new WebSocket(wsUrl)
    await new Promise((resolvePromise, reject) => {
      socket.addEventListener('open', resolvePromise, { once: true })
      socket.addEventListener('error', () => {
        reject(readonlyCdpFailureError('readonly_cdp_transport_unavailable'))
      }, { once: true })
    })
    return await new Promise((resolvePromise, reject) => {
      const timer = setTimeout(() => {
        reject(readonlyCdpFailureError('readonly_cdp_timeout'))
      }, timeoutMs)
      socket.addEventListener('message', (event) => {
        let message
        try {
          message = JSON.parse(String(event.data))
        } catch {
          clearTimeout(timer)
          reject(readonlyCdpFailureError('readonly_cdp_protocol_error'))
          return
        }
        if (message.id !== 1) return
        clearTimeout(timer)
        if (message.error || message.result?.exceptionDetails) {
          reject(readonlyCdpFailureError(readonlyCdpFailureReasonCode({
            error: message.error,
            result: message.result
          })))
          return
        }
        resolvePromise(message.result?.result?.value)
      })
      socket.send(JSON.stringify({
        id: 1,
        method: 'Runtime.evaluate',
        params: {
          expression,
          awaitPromise: true,
          returnByValue: true,
          timeout: timeoutMs
        }
      }))
    })
  } catch (error) {
    if (error?.readonlyCdpReasonCode) throw error
    throw readonlyCdpFailureError(readonlyCdpFailureReasonCode(error))
  } finally {
    socket?.close()
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
      const result = await new Promise((resolvePromise, reject) => {
        const timer = setTimeout(() => reject(new Error('cdp_input_timeout')), timeoutMs)
        const onMessage = (event) => {
          const message = JSON.parse(String(event.data))
          if (message.id !== id) return
          socket.removeEventListener('message', onMessage)
          clearTimeout(timer)
          if (message.error) {
            reject(new Error('cdp_input_dispatch_failed'))
            return
          }
          resolvePromise(message.result || {})
        }
        socket.addEventListener('message', onMessage)
        socket.send(JSON.stringify({
          id,
          method: command.method,
          params: command.params || {}
        }))
      })
      results.push(result)
    }
    return results
  } finally {
    socket.close()
  }
}

function pngDimensions(data) {
  const signature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])
  if (!Buffer.isBuffer(data) ||
    data.length < 24 ||
    !data.subarray(0, signature.length).equals(signature) ||
    data.subarray(12, 16).toString('ascii') !== 'IHDR') {
    return { width: 0, height: 0 }
  }
  return {
    width: data.readUInt32BE(16),
    height: data.readUInt32BE(20)
  }
}

async function captureRendererScreenshot(debugPort, outputPath, timeoutMs = 20_000) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  const [layout, screenshot] = await dispatchCdpCommands(target.webSocketDebuggerUrl, [
    { method: 'Page.getLayoutMetrics' },
    {
      method: 'Page.captureScreenshot',
      params: {
        format: 'png',
        fromSurface: true,
        captureBeyondViewport: false
      }
    }
  ], timeoutMs)
  const data = Buffer.from(String(screenshot?.data || ''), 'base64')
  const dimensions = pngDimensions(data)
  const visualViewport = layout?.cssVisualViewport || layout?.visualViewport || {}
  const viewport = {
    width: Number(visualViewport.clientWidth || 0),
    height: Number(visualViewport.clientHeight || 0),
    scale: Number(visualViewport.scale || 0)
  }
  const valid = data.length > 0 &&
    data.length <= MAX_SCREENSHOT_BYTES &&
    dimensions.width > 0 &&
    dimensions.height > 0 &&
    Number.isFinite(viewport.width) &&
    viewport.width > 0 &&
    Number.isFinite(viewport.height) &&
    viewport.height > 0 &&
    Number.isFinite(viewport.scale) &&
    viewport.scale > 0
  if (!valid) throw new Error('renderer_screenshot_invalid_or_oversized')
  mkdirSync(dirname(outputPath), { recursive: true, mode: 0o700 })
  writeFileSync(outputPath, data, { mode: 0o600 })
  const receipt = hashRegularFile(outputPath)
  if (!receipt.regular || receipt.byteLength !== data.length || receipt.sha256 !== sha256(data)) {
    throw new Error('renderer_screenshot_receipt_mismatch')
  }
  return {
    captured: true,
    sha256: receipt.sha256,
    width: dimensions.width,
    height: dimensions.height,
    viewport
  }
}

export function readonlyComposerGeometryExpression(acceptedLabels = []) {
  return `(() => {
    if (typeof document === 'undefined' || !document) {
      return { domReady: false };
    }
    let querySelectorAllAvailable = false;
    try {
      querySelectorAllAvailable = typeof document.querySelectorAll === 'function';
    } catch {
      return { domReady: true, readErrorStage: 'document_query' };
    }
    if (!querySelectorAllAvailable) return { domReady: false };
    const safeCall = (stage, callback) => {
      try {
        return { ok: true, value: callback() };
      } catch {
        return { ok: false, readErrorStage: stage };
      }
    };
    const readError = (result) => ({
      domReady: true,
      readErrorStage: result.readErrorStage
    });
    const rectValue = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    };
    const editorsResult = safeCall(
      'document_query',
      () => document.querySelectorAll('.ProseMirror[contenteditable="true"]')
    );
    if (!editorsResult.ok) return readError(editorsResult);
    const editorListResult = safeCall(
      'dom_collection',
      () => Array.from(editorsResult.value)
    );
    if (!editorListResult.ok) return readError(editorListResult);
    let editor = null;
    let editorRect = null;
    for (const candidate of editorListResult.value) {
      const editorCandidateResult = safeCall('editor_query', () => {
        if (!candidate || typeof candidate.getBoundingClientRect !== 'function') {
          throw new Error('editor_geometry_unavailable');
        }
        return candidate;
      });
      if (!editorCandidateResult.ok) return readError(editorCandidateResult);
      const candidateRectResult = safeCall(
        'geometry',
        () => rectValue(editorCandidateResult.value)
      );
      if (!candidateRectResult.ok) return readError(candidateRectResult);
      if (candidateRectResult.value) {
        editor = candidate;
        editorRect = candidateRectResult.value;
        break;
      }
    }
    const composerResult = safeCall(
      'composer_query',
      () => editor ? editor.closest('.ds-composer-shell') : null
    );
    if (!composerResult.ok) return readError(composerResult);
    const composer = composerResult.value;
    const buttonResult = safeCall(
      'button_query',
      () => composer
        ? composer.querySelector('.ds-composer-primary-action-button')
        : null
    );
    if (!buttonResult.ok) return readError(buttonResult);
    const button = buttonResult.value;
    const buttonRectResult = safeCall('geometry', () => rectValue(button));
    if (!buttonRectResult.ok) return readError(buttonRectResult);
    const buttonLabelResult = safeCall(
      'attribute',
      () => button
        ? String(button.getAttribute('aria-label') || button.getAttribute('title') || '')
        : ''
    );
    if (!buttonLabelResult.ok) return readError(buttonLabelResult);
    const editorTextResult = safeCall(
      'text',
      () => editor ? String(editor.innerText || editor.textContent || '').trim().length : 0
    );
    if (!editorTextResult.ok) return readError(editorTextResult);
    const buttonDisabledResult = safeCall(
      'state',
      () => !!(button && button.disabled)
    );
    if (!buttonDisabledResult.ok) return readError(buttonDisabledResult);
    const buttonLoadingResult = safeCall(
      'button_query',
      () => !!(button && button.querySelector('.ds-composer-primary-action-icon.animate-spin'))
    );
    if (!buttonLoadingResult.ok) return readError(buttonLoadingResult);
    const buttonLabel = buttonLabelResult.value;
    const accepted = ${JSON.stringify(acceptedLabels)};
    return {
      domReady: true,
      editor: editorRect,
      editorTextLength: editorTextResult.value,
      button: buttonRectResult.value,
      buttonDisabled: buttonDisabledResult.value,
      buttonLoading: buttonLoadingResult.value,
      buttonLabel,
      buttonLabelAccepted: accepted.length === 0 || accepted.includes(buttonLabel)
    };
  })()`
}

export function composerPrimaryIdleEvidence(value) {
  const evidence = {
    editorObserved: Boolean(value?.editor),
    buttonObserved: Boolean(value?.button),
    editorEmpty: value?.editorTextLength === 0,
    buttonNotLoading: value?.buttonLoading === false,
    buttonDisabled: value?.buttonDisabled === true,
    buttonLabelAccepted: value?.buttonLabelAccepted === true
  }
  let blocker = ''
  if (!evidence.editorObserved || !evidence.buttonObserved) {
    blocker = 'composer_dom_element_missing'
  } else if (!evidence.editorEmpty) {
    blocker = 'composer_editor_not_empty'
  } else if (!evidence.buttonNotLoading) {
    blocker = 'composer_runtime_not_ready'
  } else if (!evidence.buttonLabelAccepted) {
    blocker = 'composer_primary_button_label_not_idle'
  } else if (!evidence.buttonDisabled) {
    blocker = 'composer_primary_button_busy'
  }
  return Object.freeze({
    ok: blocker === '',
    blocker,
    ...evidence
  })
}

export function composerIdleFailureDiagnostic(value, waitedMs) {
  const boundedWaitedMs = Number.isSafeInteger(waitedMs) && waitedMs >= 0 ? waitedMs : 0
  return Object.freeze({
    editorObserved: Boolean(value?.editor),
    buttonObserved: Boolean(value?.button),
    editorEmpty: value?.editorTextLength === 0,
    buttonNotLoading: value?.buttonLoading === false,
    buttonDisabled: value?.buttonDisabled === true,
    buttonLabelAccepted: value?.buttonLabelAccepted === true,
    waitedMs: boundedWaitedMs
  })
}

export async function readComposerGeometry(
  debugPort,
  acceptedLabels,
  timeoutMs,
  internalDependencies = {}
) {
  const waitForTarget = typeof internalDependencies.waitForDebugTarget === 'function'
    ? internalDependencies.waitForDebugTarget
    : waitForDebugTarget
  const evaluate = typeof internalDependencies.evaluateReadonlyCdp === 'function'
    ? internalDependencies.evaluateReadonlyCdp
    : evaluateReadonlyCdp
  const retried = await withFreshReadonlyCdpTargetRetry({
    getTarget: () => waitForTarget(debugPort, timeoutMs),
    evaluate: (target) => evaluate(
      target.webSocketDebuggerUrl,
      readonlyComposerGeometryExpression(acceptedLabels),
      timeoutMs
    ),
    isTransientResult: (value) => value?.domReady === false &&
      !Object.prototype.hasOwnProperty.call(value || {}, 'readErrorStage'),
    sleep: internalDependencies.sleep
  })
  let hasReadErrorStage = false
  let expressionStage = 'unknown'
  try {
    hasReadErrorStage = Object.prototype.hasOwnProperty.call(
      retried.value || {},
      'readErrorStage'
    )
    if (hasReadErrorStage &&
        READONLY_COMPOSER_GEOMETRY_READ_ERROR_STAGES.includes(retried.value?.readErrorStage)) {
      expressionStage = retried.value.readErrorStage
    }
  } catch {
    hasReadErrorStage = true
  }
  if (hasReadErrorStage) {
    throw readonlyCdpFailureError('readonly_cdp_exception', { expressionStage })
  }
  return { target: retried.target, value: retried.value }
}

export function readonlyComposerModeGeometryExpression() {
  return `(() => {
    if (typeof document === 'undefined' || !document) {
      return { domReady: false };
    }
    let querySelectorAllAvailable = false;
    try {
      querySelectorAllAvailable = typeof document.querySelectorAll === 'function';
    } catch {
      return { domReady: true, readErrorStage: 'document_query' };
    }
    if (!querySelectorAllAvailable) return { domReady: false };
    const safeCall = (stage, callback) => {
      try {
        return { ok: true, value: callback() };
      } catch {
        return { ok: false, readErrorStage: stage };
      }
    };
    const readError = (result) => ({
      domReady: true,
      readErrorStage: result.readErrorStage
    });
    const menuButtonResult = safeCall(
      'document_query',
      () => document.querySelector('button[aria-label="More actions"]')
    );
    if (!menuButtonResult.ok) return readError(menuButtonResult);
    const buttonsResult = safeCall('document_query', () => document.querySelectorAll('button'));
    if (!buttonsResult.ok) return readError(buttonsResult);
    const buttonListResult = safeCall('dom_collection', () => Array.from(buttonsResult.value));
    if (!buttonListResult.ok) return readError(buttonListResult);
    let planButton = null;
    for (const button of buttonListResult.value) {
      const modeSwitchResult = safeCall(
        'button_query',
        () => button && typeof button.querySelector === 'function'
          ? button.querySelector('[role="switch"]')
          : null
      );
      if (!modeSwitchResult.ok) return readError(modeSwitchResult);
      const labelResult = safeCall(
        'attribute',
        () => String(button && button.innerText || '').trim()
      );
      if (!labelResult.ok) return readError(labelResult);
      if (modeSwitchResult.value && labelResult.value.includes('Plan mode')) {
        planButton = button;
        break;
      }
    }
    const modeSwitchResult = safeCall(
      'button_query',
      () => planButton && typeof planButton.querySelector === 'function'
        ? planButton.querySelector('[role="switch"]')
        : null
    );
    if (!modeSwitchResult.ok) return readError(modeSwitchResult);
    const menuRectResult = safeCall('geometry', () => {
      if (!menuButtonResult.value) return null;
      const rect = menuButtonResult.value.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    });
    if (!menuRectResult.ok) return readError(menuRectResult);
    const planRectResult = safeCall('geometry', () => {
      if (!planButton) return null;
      const rect = planButton.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    });
    if (!planRectResult.ok) return readError(planRectResult);
    const planCheckedResult = safeCall(
      'attribute',
      () => modeSwitchResult.value
        ? modeSwitchResult.value.getAttribute('aria-checked') === 'true'
        : null
    );
    if (!planCheckedResult.ok) return readError(planCheckedResult);
    return {
      domReady: true,
      menuButton: menuRectResult.value,
      planButton: planRectResult.value,
      planChecked: planCheckedResult.value
    };
  })()`
}

export async function readComposerModeGeometry(
  debugPort,
  timeoutMs = 20_000,
  internalDependencies = {}
) {
  const waitForTarget = typeof internalDependencies.waitForDebugTarget === 'function'
    ? internalDependencies.waitForDebugTarget
    : waitForDebugTarget
  const evaluate = typeof internalDependencies.evaluateReadonlyCdp === 'function'
    ? internalDependencies.evaluateReadonlyCdp
    : evaluateReadonlyCdp
  const retried = await withFreshReadonlyCdpTargetRetry({
    getTarget: () => waitForTarget(debugPort, timeoutMs),
    evaluate: (target) => evaluate(
      target.webSocketDebuggerUrl,
      readonlyComposerModeGeometryExpression(),
      timeoutMs
    ),
    isTransientResult: (value) => value?.domReady === false &&
      !Object.prototype.hasOwnProperty.call(value || {}, 'readErrorStage'),
    sleep: internalDependencies.sleep
  })
  let hasReadErrorStage = false
  let expressionStage = 'unknown'
  try {
    hasReadErrorStage = Object.prototype.hasOwnProperty.call(
      retried.value || {},
      'readErrorStage'
    )
    if (hasReadErrorStage &&
        READONLY_COMPOSER_MODE_READ_ERROR_STAGES.includes(retried.value?.readErrorStage)) {
      expressionStage = retried.value.readErrorStage
    }
  } catch {
    hasReadErrorStage = true
  }
  if (hasReadErrorStage) {
    throw readonlyCdpFailureError('readonly_cdp_exception', { expressionStage })
  }
  return { target: retried.target, value: retried.value }
}

export function readonlyComposerReasoningGeometryExpression(expectedModel, desiredLabel) {
  return `(() => {
    if (typeof document === 'undefined' || !document ||
        typeof document.querySelectorAll !== 'function') {
      return { domReady: false };
    }
    const rectValue = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    };
    const controls = Array.from(
      document.querySelectorAll('button[aria-label="Model and reasoning settings"]')
    ).filter((button) => rectValue(button));
    const options = Array.from(
      document.querySelectorAll('button[role="menuitemradio"]')
    ).filter((button) =>
      rectValue(button) && String(button.getAttribute('title') || '') ===
        ${JSON.stringify(desiredLabel)}
    );
    const control = controls.length === 1 ? controls[0] : null;
    const option = options.length === 1 ? options[0] : null;
    return {
      domReady: true,
      controlCount: controls.length,
      control: rectValue(control),
      controlTitle: control ? String(control.getAttribute('title') || '') : '',
      controlExpanded: control ? control.getAttribute('aria-expanded') === 'true' : null,
      expectedSelectedTitle: ${JSON.stringify(`${expectedModel} / ${desiredLabel}`)},
      optionCount: options.length,
      option: rectValue(option),
      optionChecked: option ? option.getAttribute('aria-checked') === 'true' : null
    };
  })()`
}

export async function readComposerReasoningGeometry(
  debugPort,
  expectedModel,
  desiredLabel,
  timeoutMs = 20_000,
  internalDependencies = {}
) {
  const waitForTarget = typeof internalDependencies.waitForDebugTarget === 'function'
    ? internalDependencies.waitForDebugTarget
    : waitForDebugTarget
  const evaluate = typeof internalDependencies.evaluateReadonlyCdp === 'function'
    ? internalDependencies.evaluateReadonlyCdp
    : evaluateReadonlyCdp
  const retried = await withFreshReadonlyCdpTargetRetry({
    getTarget: () => waitForTarget(debugPort, timeoutMs),
    evaluate: (target) => evaluate(
      target.webSocketDebuggerUrl,
      readonlyComposerReasoningGeometryExpression(expectedModel, desiredLabel),
      timeoutMs
    ),
    isTransientResult: (value) => value?.domReady === false,
    sleep: internalDependencies.sleep
  })
  return { target: retried.target, value: retried.value }
}

export function composerReasoningClosedSelectionEvidence(value) {
  const controlValid = value?.controlCount === 1 &&
    Boolean(value?.control) &&
    value?.controlExpanded === false &&
    typeof value?.controlTitle === 'string' &&
    typeof value?.expectedSelectedTitle === 'string'
  const selected = controlValid &&
    value.controlTitle === value.expectedSelectedTitle
  return Object.freeze({
    controlValid,
    selected,
    controlTitleSha256: selected ? sha256(value.controlTitle) : ''
  })
}

async function clickCdpGeometry(target, geometry, timeoutMs = 20_000) {
  if (!target?.webSocketDebuggerUrl || !geometry) {
    throw new Error('cdp_click_geometry_missing')
  }
  await dispatchCdpCommands(target.webSocketDebuggerUrl, [
    {
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mousePressed',
        x: geometry.x,
        y: geometry.y,
        button: 'left',
        clickCount: 1
      }
    },
    {
      method: 'Input.dispatchMouseEvent',
      params: {
        type: 'mouseReleased',
        x: geometry.x,
        y: geometry.y,
        button: 'left',
        clickCount: 1
      }
    }
  ], timeoutMs)
}

export async function selectComposerMode(debugPort, desiredMode, internalDependencies = {}) {
  const readMode = typeof internalDependencies.readComposerModeGeometry === 'function'
    ? internalDependencies.readComposerModeGeometry
    : readComposerModeGeometry
  const click = typeof internalDependencies.clickCdpGeometry === 'function'
    ? internalDependencies.clickCdpGeometry
    : clickCdpGeometry
  const pause = typeof internalDependencies.sleep === 'function'
    ? internalDependencies.sleep
    : sleep
  let phase = 'initial_closed'
  let operation = 'read'
  const failure = (blocker, error = null) => {
    const reasonCode = safeComposerModeReasonCode(blocker)
    let expressionStage = 'none'
    try {
      expressionStage = error?.readonlyCdpExpressionStage || 'none'
    } catch {
      expressionStage = 'unknown'
    }
    return {
      ok: false,
      blocked: false,
      blocker: reasonCode,
      diagnostic: composerModeDiagnostic({
        reasonCode,
        phase,
        operation,
        expressionStage
      })
    }
  }
  try {
    phase = 'initial_closed'
    operation = 'read'
    const closed = await readMode(debugPort)
    if (!closed.value?.menuButton) {
      return failure('composer_mode_menu_button_missing')
    }
    operation = 'menu_click'
    await click(closed.target, closed.value.menuButton)
    operation = 'sleep'
    await pause(100)
    phase = 'open'
    operation = 'read'
    const open = await readMode(debugPort)
    if (!open.value?.planButton || typeof open.value?.planChecked !== 'boolean') {
      return failure('composer_plan_mode_control_missing')
    }
    const desiredChecked = desiredMode === 'plan'
    if (open.value.planChecked !== desiredChecked) {
      operation = 'plan_click'
      await click(open.target, open.value.planButton)
    } else {
      operation = 'menu_click'
      await click(open.target, open.value.menuButton)
    }
    operation = 'sleep'
    await pause(100)

    phase = 'closed_after_selection'
    operation = 'read'
    const closedAfterSelection = await readMode(debugPort)
    if (!closedAfterSelection.value?.menuButton) {
      return failure('composer_mode_menu_button_missing_after_selection')
    }
    operation = 'menu_click'
    await click(closedAfterSelection.target, closedAfterSelection.value.menuButton)
    operation = 'sleep'
    await pause(100)
    phase = 'verified'
    operation = 'read'
    const verified = await readMode(debugPort)
    const selected = verified.value?.planChecked === desiredChecked
    if (verified.value?.menuButton) {
      operation = 'cleanup_click'
      await click(verified.target, verified.value.menuButton)
      operation = 'sleep'
      await pause(100)
    }
    return {
      ok: selected,
      blocked: false,
      blocker: selected ? '' : safeComposerModeReasonCode(
        `composer_${desiredMode}_mode_not_selected`
      ),
      diagnostic: composerModeDiagnostic({
        reasonCode: selected ? '' : `composer_${desiredMode}_mode_not_selected`,
        phase: selected ? 'none' : phase,
        operation: selected ? 'none' : 'read',
        expressionStage: 'none'
      })
    }
  } catch (error) {
    let safeReasonCode = 'composer_mode_selection_failed'
    try {
      safeReasonCode = safeErrorDiagnosticCode(error, 'composer_mode_selection_failed')
    } catch {
      safeReasonCode = 'composer_mode_selection_failed'
    }
    return failure(safeReasonCode, error)
  }
}

async function selectComposerReasoningEffort(debugPort, expectedModel, desiredEffort) {
  if (expectedModel !== FORMAL_MODEL || desiredEffort !== FORMAL_REASONING_EFFORT) {
    return {
      ok: false,
      blocked: false,
      blocker: 'composer_formal_reasoning_contract_invalid',
      controlTitleSha256: ''
    }
  }
  try {
    const closed = await readComposerReasoningGeometry(
      debugPort,
      expectedModel,
      FORMAL_REASONING_LABEL
    )
    const closedSelection = composerReasoningClosedSelectionEvidence(closed.value)
    if (!closedSelection.controlValid) {
      return {
        ok: false,
        blocked: false,
        blocker: 'composer_reasoning_control_missing_or_ambiguous',
        controlTitleSha256: ''
      }
    }
    if (closedSelection.selected) {
      return {
        ok: true,
        blocked: false,
        blocker: '',
        controlTitleSha256: closedSelection.controlTitleSha256
      }
    }
    await clickCdpGeometry(closed.target, closed.value.control)
    await sleep(100)
    const open = await readComposerReasoningGeometry(
      debugPort,
      expectedModel,
      FORMAL_REASONING_LABEL
    )
    if (open.value?.controlCount !== 1 || open.value?.controlExpanded !== true ||
        open.value?.optionCount !== 1 || !open.value?.option ||
        typeof open.value?.optionChecked !== 'boolean') {
      return {
        ok: false,
        blocked: false,
        blocker: 'composer_reasoning_option_missing_or_ambiguous',
        controlTitleSha256: ''
      }
    }
    await clickCdpGeometry(open.target, open.value.option)
    await sleep(100)
    const verified = await readComposerReasoningGeometry(
      debugPort,
      expectedModel,
      FORMAL_REASONING_LABEL
    )
    const selected = verified.value?.controlCount === 1 &&
      Boolean(verified.value?.control) &&
      verified.value?.controlExpanded === false &&
      verified.value?.controlTitle === verified.value?.expectedSelectedTitle &&
      verified.value?.optionCount === 0
    return {
      ok: selected,
      blocked: false,
      blocker: selected ? '' : 'composer_reasoning_effort_not_selected',
      controlTitleSha256: selected ? sha256(verified.value.controlTitle) : ''
    }
  } catch (error) {
    return {
      ok: false,
      blocked: false,
      blocker: safeErrorDiagnosticCode(
        error,
        'composer_reasoning_effort_selection_failed'
      ),
      controlTitleSha256: ''
    }
  }
}

export async function cdpComposerSubmit(
  debugPort,
  text,
  acceptedButtonLabels,
  idleTimeoutMs = 10_000,
  internalDependencies = {}
) {
  const readGeometry = typeof internalDependencies.readComposerGeometry === 'function'
    ? internalDependencies.readComposerGeometry
    : readComposerGeometry
  const dispatch = typeof internalDependencies.dispatchCdpCommands === 'function'
    ? internalDependencies.dispatchCdpCommands
    : dispatchCdpCommands
  const pause = typeof internalDependencies.sleep === 'function'
    ? internalDependencies.sleep
    : sleep
  let phase = 'initial_idle'
  let operation = 'none'
  const failure = (blocker, error = null, extra = {}) => {
    let reasonCode = safeComposerSubmitReasonCode(blocker, 'composer_submit_failed')
    try {
      reasonCode = safeComposerSubmitReasonCode(
        safeErrorDiagnosticCode(error, reasonCode),
        reasonCode
      )
    } catch {
      reasonCode = safeComposerSubmitReasonCode(reasonCode, 'composer_submit_failed')
    }
    return {
      ok: false,
      blocked: false,
      ...extra,
      blocker: reasonCode,
      diagnostic: composerSubmitDiagnostic({
        reasonCode,
        phase,
        operation,
        expressionStage: composerSubmitExpressionStageFromError(error)
      })
    }
  }
  try {
    if (!Number.isSafeInteger(idleTimeoutMs) || idleTimeoutMs < 1000 ||
        idleTimeoutMs > 120_000) {
      return failure('composer_idle_timeout_invalid')
    }
    const idleStartedAt = Date.now()
    const idleDeadline = Date.now() + idleTimeoutMs
    let editorGeometry = null
    let latestGeometry = null
    while (Date.now() < idleDeadline) {
      phase = 'initial_idle'
      operation = 'read'
      const observed = await readGeometry(
        debugPort,
        acceptedButtonLabels,
        Math.min(5000, Math.max(1000, idleDeadline - Date.now()))
      )
      latestGeometry = observed
      if (composerPrimaryIdleEvidence(observed.value).ok) {
        editorGeometry = observed
        break
      }
      operation = 'sleep'
      await pause(100)
    }
    if (!editorGeometry?.value?.editor || !editorGeometry?.value?.button) {
      operation = 'read'
      const idleEvidence = composerPrimaryIdleEvidence(latestGeometry?.value)
      return failure(idleEvidence.blocker, null, {
        idleFailure: composerIdleFailureDiagnostic(
          latestGeometry?.value,
          Math.max(0, Date.now() - idleStartedAt)
        )
      })
    }
    const editor = editorGeometry.value.editor
    phase = 'editor_input'
    operation = 'editor_input_dispatch'
    await dispatch(editorGeometry.target.webSocketDebuggerUrl, [
      {
        method: 'Input.dispatchMouseEvent',
        params: { type: 'mousePressed', x: editor.x, y: editor.y, button: 'left', clickCount: 1 }
      },
      {
        method: 'Input.dispatchMouseEvent',
        params: { type: 'mouseReleased', x: editor.x, y: editor.y, button: 'left', clickCount: 1 }
      },
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'rawKeyDown', key: 'a', code: 'KeyA', modifiers: 4 }
      },
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'keyUp', key: 'a', code: 'KeyA', modifiers: 4 }
      },
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'rawKeyDown', key: 'Backspace', code: 'Backspace' }
      },
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'keyUp', key: 'Backspace', code: 'Backspace' }
      },
      {
        method: 'Input.insertText',
        params: { text }
      }
    ], 20_000)
    let buttonGeometry = null
    const readyDeadline = Date.now() + 10_000
    while (Date.now() < readyDeadline) {
      phase = 'post_input_ready'
      operation = 'read'
      const observed = await readGeometry(
        debugPort,
        acceptedButtonLabels,
        Math.min(5000, Math.max(1000, readyDeadline - Date.now()))
      )
      if (observed.value?.button &&
        observed.value?.editorTextLength > 0 &&
        observed.value?.buttonLoading === false &&
        observed.value?.buttonDisabled === false &&
        observed.value?.buttonLabelAccepted === true) {
        buttonGeometry = observed
        break
      }
      latestGeometry = observed
      operation = 'sleep'
      await pause(100)
    }
    const button = buttonGeometry?.value?.button
    if (!button) {
      operation = 'read'
      const blocker = latestGeometry?.value?.editorTextLength === 0
        ? 'composer_input_not_observed'
        : latestGeometry?.value?.buttonLabelAccepted !== true
          ? 'composer_primary_button_not_idle'
          : 'composer_primary_button_not_ready'
      return failure(blocker)
    }
    phase = 'primary_button'
    operation = 'primary_button_dispatch'
    await dispatch(buttonGeometry.target.webSocketDebuggerUrl, [
      {
        method: 'Input.dispatchMouseEvent',
        params: { type: 'mousePressed', x: button.x, y: button.y, button: 'left', clickCount: 1 }
      },
      {
        method: 'Input.dispatchMouseEvent',
        params: { type: 'mouseReleased', x: button.x, y: button.y, button: 'left', clickCount: 1 }
      }
    ], 20_000)
    return {
      ok: true,
      blocked: false,
      blocker: '',
      diagnostic: composerSubmitDiagnostic()
    }
  } catch (error) {
    return failure('composer_submit_failed', error)
  }
}

function readonlyRecoveredTimelineExpression(expectedResultDigest = '') {
  return `(async () => {
    const rectValue = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    };
    const buttonByLabel = (labels) => Array.from(document.querySelectorAll('button')).find(
      (button) => labels.includes(String(button.getAttribute('aria-label') || '').trim())
    ) || null;
    const timeline = document.querySelector('[data-analytix-message-timeline-scroller]');
    const timelineText = timeline ? String(timeline.innerText || '') : '';
    const digest = async (value) => {
      const bytes = new TextEncoder().encode(value);
      const hash = await crypto.subtle.digest('SHA-256', bytes);
      return Array.from(new Uint8Array(hash), (byte) => byte.toString(16).padStart(2, '0')).join('');
    };
    const resultRows = Array.from(document.querySelectorAll(
      '[data-analytix-timeline-row="turn"]'
    )).map((row) => Array.from(row.querySelectorAll('.ds-chat-answer'))
      .map((answer) => String(answer.innerText || '').trim())
      .filter(Boolean));
    const resultDigests = await Promise.all(resultRows.map((texts) =>
      texts.length > 0 ? digest(JSON.stringify(texts)) : Promise.resolve('')
    ));
    const expectedResultDigest = ${JSON.stringify(expectedResultDigest)};
    return {
      timeline: rectValue(timeline),
      latestResultDigest: resultDigests.filter(Boolean).at(-1) || '',
      resultVisible: /^[0-9a-f]{64}$/.test(expectedResultDigest) &&
        resultDigests.includes(expectedResultDigest),
      compactionVisible: timelineText.includes('Compacted context') ||
        timelineText.includes('已压缩上下文'),
      todoButton: rectValue(buttonByLabel(['Todo'])),
      summaryButton: rectValue(buttonByLabel(['Toggle pinned summary', '切换置顶摘要']))
    };
  })()`
}

async function rendererResultDigestEvidence(debugPort, timeoutMs = 20_000) {
  try {
    const target = await waitForDebugTarget(debugPort, timeoutMs)
    const observed = await evaluateReadonlyCdp(
      target.webSocketDebuggerUrl,
      readonlyRecoveredTimelineExpression(''),
      timeoutMs
    )
    const resultDigest = typeof observed?.latestResultDigest === 'string' &&
        /^[0-9a-f]{64}$/u.test(observed.latestResultDigest)
      ? observed.latestResultDigest
      : ''
    return {
      ok: Boolean(observed?.timeline && resultDigest),
      resultDigest
    }
  } catch {
    return { ok: false, resultDigest: '' }
  }
}

function readonlyRecoveredTodoExpression(expectedTodoCount) {
  return `(() => {
    const panel = document.querySelector('.ds-right-sidebar-pane[data-open="true"]');
    const title = panel ? panel.querySelector('.ds-right-panel-title') : null;
    const completed = panel
      ? Array.from(panel.querySelectorAll('button')).filter((button) => {
          const label = String(button.getAttribute('aria-label') || '').trim();
          return label === 'Done' || label === '完成';
        })
      : [];
    const rect = panel ? panel.getBoundingClientRect() : null;
    return {
      panelVisible: !!(rect && rect.width > 0 && rect.height > 0),
      titleVisible: ['Thread Todo', '线程 Todo'].includes(String(title?.textContent || '').trim()),
      completedTodoCount: completed.length,
      expectedTodoCount: ${JSON.stringify(expectedTodoCount)},
      todosVisible: completed.length === ${JSON.stringify(expectedTodoCount)} &&
        ${JSON.stringify(expectedTodoCount)} > 0
    };
  })()`
}

function readonlyRecoveredSubagentExpression(expectedSubagentCount) {
  return `(() => {
    const rectValue = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      if (rect.width <= 0 || rect.height <= 0) return null;
      return {
        x: rect.left + (rect.width / 2),
        y: rect.top + (rect.height / 2),
        width: rect.width,
        height: rect.height
      };
    };
    const panel = document.querySelector('[data-summary-panel][data-visible="true"]');
    const sections = panel ? Array.from(panel.querySelectorAll('section')) : [];
    const section = sections.find((candidate) => {
      const button = Array.from(candidate.children).find((child) => child.tagName === 'BUTTON');
      return String(button?.textContent || '').trim().startsWith('子智能体');
    }) || null;
    const toggle = section
      ? Array.from(section.children).find((child) => child.tagName === 'BUTTON') || null
      : null;
    const content = section?.querySelector('.ds-summary-section-content') || null;
    const rows = content ? Array.from(content.querySelectorAll('[role="button"]')) : [];
    return {
      panelVisible: !!rectValue(panel),
      toggle: rectValue(toggle),
      expanded: toggle?.getAttribute('aria-expanded') === 'true' &&
        content?.getAttribute('data-expanded') === 'true',
      visibleSubagentCount: rows.length,
      expectedSubagentCount: ${JSON.stringify(expectedSubagentCount)},
      terminalStateVisible: rows.length === ${JSON.stringify(expectedSubagentCount)} &&
        rows.every((row) => String(row.textContent || '').includes('完成')),
      firstRow: rectValue(rows[0] || null)
    };
  })()`
}

function readonlyRecoveredSubagentInspectorExpression(expectedSubagentCount) {
  return `(() => {
    const panel = document.querySelector('[data-subagent-inspector-panel]');
    const rect = panel ? panel.getBoundingClientRect() : null;
    const tabs = panel
      ? Array.from(panel.querySelectorAll('[role="tablist"][aria-label="子智能体"] [role="tab"]'))
      : [];
    const selected = tabs.filter((tab) => tab.getAttribute('aria-selected') === 'true');
    const subtitle = panel?.querySelector('.ds-right-panel-subtitle');
    return {
      panelVisible: !!(rect && rect.width > 0 && rect.height > 0),
      tabCount: tabs.length,
      expectedSubagentCount: ${JSON.stringify(expectedSubagentCount)},
      exactlyOneSelected: selected.length === 1,
      terminalStateVisible: String(subtitle?.textContent || '').includes('完成')
    };
  })()`
}

async function rendererVisibleRecoveryEvidence({
  debugPort,
  expectedTodoCount,
  expectedSubagentCount,
  expectedResultDigest,
  timeoutMs = 30_000
}) {
  const todoCount = Number.isSafeInteger(expectedTodoCount) && expectedTodoCount > 0
    ? expectedTodoCount
    : -1
  const subagentCount = Number.isSafeInteger(expectedSubagentCount) && expectedSubagentCount > 0
    ? expectedSubagentCount
    : -1
  const resultDigest = typeof expectedResultDigest === 'string' &&
      /^[0-9a-f]{64}$/u.test(expectedResultDigest)
    ? expectedResultDigest
    : ''
  if (!resultDigest) {
    return { ok: false, blocker: 'renderer_recovery_result_digest_invalid' }
  }
  const deadline = Date.now() + timeoutMs
  try {
    const target = await waitForDebugTarget(debugPort, Math.min(timeoutMs, 20_000))
    const readUntil = async (expression, accepted) => {
      let latest = null
      while (Date.now() < deadline) {
        latest = await evaluateReadonlyCdp(
          target.webSocketDebuggerUrl,
          expression,
          Math.min(5000, Math.max(1000, deadline - Date.now()))
        )
        if (accepted(latest)) return latest
        await sleep(150)
      }
      return latest
    }
    const timeline = await readUntil(
      readonlyRecoveredTimelineExpression(resultDigest),
      (value) => Boolean(
        value?.timeline && value?.todoButton && value?.summaryButton &&
        value?.resultVisible && value?.compactionVisible
      )
    )
    if (!timeline?.todoButton || !timeline?.summaryButton) {
      return { ok: false, blocker: 'renderer_recovery_controls_not_visible' }
    }

    await clickCdpGeometry(target, timeline.todoButton)
    const todo = await readUntil(
      readonlyRecoveredTodoExpression(todoCount),
      (value) => value?.panelVisible === true && value?.titleVisible === true &&
        value?.todosVisible === true
    )

    const afterTodo = await readUntil(
      readonlyRecoveredTimelineExpression(resultDigest),
      (value) => Boolean(value?.summaryButton)
    )
    if (!afterTodo?.summaryButton) {
      return { ok: false, blocker: 'renderer_summary_control_not_visible' }
    }
    await clickCdpGeometry(target, afterTodo.summaryButton)
    let subagent = await readUntil(
      readonlyRecoveredSubagentExpression(subagentCount),
      (value) => value?.panelVisible === true && Boolean(value?.toggle)
    )
    if (subagent?.toggle && subagent?.expanded !== true) {
      await clickCdpGeometry(target, subagent.toggle)
      subagent = await readUntil(
        readonlyRecoveredSubagentExpression(subagentCount),
        (value) => value?.expanded === true && value?.terminalStateVisible === true &&
          Boolean(value?.firstRow)
      )
    } else {
      subagent = await readUntil(
        readonlyRecoveredSubagentExpression(subagentCount),
        (value) => value?.expanded === true && value?.terminalStateVisible === true &&
          Boolean(value?.firstRow)
      )
    }

    let inspector = null
    if (subagent?.firstRow) {
      await clickCdpGeometry(target, subagent.firstRow)
      inspector = await readUntil(
        readonlyRecoveredSubagentInspectorExpression(subagentCount),
        (value) => value?.panelVisible === true &&
          value?.tabCount === subagentCount &&
          value?.exactlyOneSelected === true &&
          value?.terminalStateVisible === true
      )
    }
    return {
      ok: true,
      blocker: '',
      timelineVisible: Boolean(timeline?.timeline),
      threadVisible: timeline?.resultVisible === true,
      resultVisible: timeline?.resultVisible === true,
      compactionVisible: timeline?.compactionVisible === true,
      todoVisible: todo?.panelVisible === true && todo?.titleVisible === true &&
        todo?.todosVisible === true && todo?.completedTodoCount === todoCount,
      subagentVisible: subagent?.panelVisible === true && subagent?.expanded === true &&
        subagent?.visibleSubagentCount === subagentCount &&
        subagent?.terminalStateVisible === true &&
        inspector?.panelVisible === true &&
        inspector?.tabCount === subagentCount &&
        inspector?.exactlyOneSelected === true &&
        inspector?.terminalStateVisible === true,
      todoCount,
      subagentCount
    }
  } catch (error) {
    return {
      ok: false,
      blocker: error instanceof Error ? error.message : 'renderer_visible_recovery_observation_failed'
    }
  }
}

export function normalQuitLifecycleEvidence(input = {}) {
  const requestOk = input?.requestOk === true
  const processExited = input?.processExited === true
  const exitCode = Number.isSafeInteger(input?.exitCode) ? input.exitCode : null
  const signal = typeof input?.signal === 'string' && input.signal.length > 0
  const residualOwnedProcess = input?.residualOwnedProcess === true
  const debugPortOpen = input?.debugPortOpen === true
  const runtimePortOpen = input?.runtimePortOpen === true
  const artifactRevalidationOk = input?.artifactRevalidationOk === true
  let reasonCode = 'normal_quit_observed'
  if (!requestOk) reasonCode = 'normal_quit_request_failed'
  else if (!processExited) reasonCode = 'normal_quit_process_not_exited'
  else if (signal) reasonCode = 'normal_quit_signal_exit'
  else if (exitCode === null) reasonCode = 'normal_quit_exit_status_missing'
  else if (exitCode !== 0) reasonCode = 'normal_quit_nonzero_exit'
  else if (residualOwnedProcess) reasonCode = 'normal_quit_residual_owned_process'
  else if (debugPortOpen) reasonCode = 'normal_quit_debug_port_open'
  else if (runtimePortOpen) reasonCode = 'normal_quit_runtime_port_open'
  else if (!artifactRevalidationOk) reasonCode = 'normal_quit_artifact_revalidation_failed'
  return Object.freeze({
    ok: reasonCode === 'normal_quit_observed',
    blocked: input?.blocked === true,
    reasonCode,
    blocker: reasonCode === 'normal_quit_observed' ? '' : reasonCode
  })
}

async function cdpNormalQuit(debugPort) {
  try {
    const target = await waitForDebugTarget(debugPort, 20_000)
    await dispatchCdpCommands(target.webSocketDebuggerUrl, [
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'rawKeyDown', key: 'F4', code: 'F4', modifiers: 1 }
      },
      {
        method: 'Input.dispatchKeyEvent',
        params: { type: 'keyUp', key: 'F4', code: 'F4', modifiers: 1 }
      }
    ], 20_000)
    return { ok: true, blocked: false, blocker: '' }
  } catch {
    return {
      ok: false,
      blocked: false,
      blocker: 'normal_quit_request_failed'
    }
  }
}

function emptyProviderReceiptTrace(reasonCode = 'provider_receipt_not_observed') {
  return {
    reasonCode,
    started: false,
    acknowledged: false,
    ended: false,
    eventCount: 0
  }
}

const ORDINARY_RESULT_RECEIPT_REASON_CODES = Object.freeze([
  'ordinary_result_not_observed',
  'ordinary_result_absent',
  'ordinary_result_legacy_boundary',
  'ordinary_result_invalid',
  'ordinary_result_observed',
  'ordinary_result_receipt_invalid'
])

function emptyOrdinaryResultReceipt(reasonCode = 'ordinary_result_not_observed') {
  return {
    observed: false,
    reasonCode,
    candidateOrigin: 'not_observed',
    projectionClass: 'not_observed',
    resultDigest: '',
    textSha256: '',
    resultMarkerObserved: false,
    researchMarkerObserved: false,
    writingMarkerObserved: false
  }
}

export function ordinaryResultReceiptEvidence(value) {
  const invalid = emptyOrdinaryResultReceipt('ordinary_result_receipt_invalid')
  if (!exactObjectKeys(value, Object.keys(invalid)) ||
      !ORDINARY_RESULT_RECEIPT_REASON_CODES.includes(value.reasonCode) ||
      typeof value.observed !== 'boolean' ||
      typeof value.candidateOrigin !== 'string' ||
      typeof value.projectionClass !== 'string' ||
      typeof value.resultDigest !== 'string' ||
      typeof value.textSha256 !== 'string' ||
      typeof value.resultMarkerObserved !== 'boolean' ||
      typeof value.researchMarkerObserved !== 'boolean' ||
      typeof value.writingMarkerObserved !== 'boolean') return invalid
  if (!value.observed) {
    return value.reasonCode !== 'ordinary_result_observed' &&
        value.candidateOrigin === 'not_observed' &&
        value.projectionClass === 'not_observed' &&
        value.resultDigest === '' && value.textSha256 === '' &&
        value.resultMarkerObserved === false &&
        value.researchMarkerObserved === false &&
        value.writingMarkerObserved === false
      ? Object.freeze({ ...value })
      : invalid
  }
  const origins = ['provider_ordinary_only', 'host_fixed']
  const projectionClasses = [
    'provider_ordinary_only',
    'host_fixed_provider_result_withheld',
    'host_fixed_protected_fact_blocked'
  ]
  if (value.reasonCode !== 'ordinary_result_observed' ||
      !origins.includes(value.candidateOrigin) ||
      !projectionClasses.includes(value.projectionClass) ||
      !/^[0-9a-f]{64}$/u.test(value.resultDigest) ||
      !/^[0-9a-f]{64}$/u.test(value.textSha256) ||
      (value.candidateOrigin === 'provider_ordinary_only') !==
        (value.projectionClass === 'provider_ordinary_only') ||
      (value.candidateOrigin === 'host_fixed' && (
        value.resultMarkerObserved || value.researchMarkerObserved ||
        value.writingMarkerObserved
      ))) return invalid
  return Object.freeze({ ...value })
}

export function providerReceiptTraceEvidence(value) {
  const invalid = emptyProviderReceiptTrace('provider_receipt_trace_invalid')
  if (!exactObjectKeys(value, Object.keys(invalid)) ||
      !PROVIDER_RECEIPT_REASON_CODES.includes(value.reasonCode)) return invalid
  const booleans = ['started', 'acknowledged', 'ended']
  const counts = ['eventCount']
  if (booleans.some((key) => typeof value[key] !== 'boolean') ||
      counts.some((key) => !Number.isSafeInteger(value[key]) || value[key] < 0) ||
      value.eventCount > 3) return invalid
  return Object.freeze({ ...value })
}

export async function observeProviderTerminalViaSse(
  runtimeApi,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  timeoutMs = 5000
) {
  const reasonCodes = new Set([
    'provider_receipt_not_observed', 'provider_receipt_input_invalid',
    'provider_receipt_timeout', 'provider_receipt_event_envelope_invalid',
    'provider_receipt_event_sequence_invalid', 'provider_receipt_event_scope_invalid',
    'provider_receipt_non_general_terminal', 'provider_receipt_terminal_batch_invalid',
    'provider_receipt_terminal_pair_invalid', 'provider_receipt_item_invalid',
    'provider_receipt_usage_telemetry_invalid', 'provider_receipt_attempt_status_invalid',
    'provider_receipt_attempt_counts_invalid', 'provider_receipt_ack_rejected',
    'provider_receipt_ack_failed', 'provider_receipt_stream_ended_without_batch',
    'provider_receipt_stream_error', 'provider_receipt_stream_event_rejected',
    ...PROVIDER_RECEIPT_REASON_CODES.filter((code) => code.startsWith('provider_receipt_sse_')),
    'provider_receipt_stream_transport_error', 'provider_receipt_stream_http_error',
    'provider_receipt_stream_setup_error', 'provider_receipt_listener_invalid',
    'provider_receipt_start_rejected', 'provider_receipt_start_failed',
    'provider_receipt_observer_failed', 'provider_receipt_target_turn_duplicate',
    'provider_receipt_target_turn_missing', 'provider_receipt_observed'
  ])
  const trace = {
    reasonCode: 'provider_receipt_not_observed',
    started: false,
    acknowledged: false,
    ended: false,
    eventCount: 0
  }
  const emptyOrdinaryResultReceipt = (reasonCode = 'ordinary_result_not_observed') => ({
    observed: false,
    reasonCode,
    candidateOrigin: 'not_observed',
    projectionClass: 'not_observed',
    resultDigest: '',
    textSha256: '',
    resultMarkerObserved: false,
    researchMarkerObserved: false,
    writingMarkerObserved: false
  })
  const empty = (reasonCode) => ({
    providerAttempts: [],
    providerTerminals: [],
    ordinaryResultReceipt: emptyOrdinaryResultReceipt(),
    providerReceiptTrace: {
      ...trace,
      reasonCode: reasonCodes.has(reasonCode) ? reasonCode : 'provider_receipt_observer_failed'
    }
  })
  const api = runtimeApi
  const exactThreadId = typeof threadId === 'string' ? threadId.trim() : ''
  const exactTurnId = typeof expectedTurnId === 'string' ? expectedTurnId.trim() : ''
  if (!api || !exactThreadId || exactThreadId !== threadId || exactThreadId.length > 256 ||
      !exactTurnId || exactTurnId !== expectedTurnId || exactTurnId.length > 256 ||
      !Number.isSafeInteger(baselineSeq) || baselineSeq < 0 ||
      !Number.isSafeInteger(expectedHighestSeq) || expectedHighestSeq <= baselineSeq ||
      !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 60_000 ||
      typeof api.startSse !== 'function' || typeof api.ackSseEvent !== 'function' ||
      typeof api.stopSse !== 'function' || typeof api.onSseEvent !== 'function' ||
      typeof api.onSseEnd !== 'function' || typeof api.onSseError !== 'function') {
    return empty('provider_receipt_input_invalid')
  }

  const unsubscribers = []
  let startConfirmed = false
  let acknowledged = false
  let ended = false
  let settled = false
  let pending = null
  // The receipt observer owns only the atomic general-terminal batch. Replaying
  // the entire turn also subjects unrelated progress events to this narrower
  // observer. A batch contains two or three contiguous events and the public
  // thread cursor may be one child-lifecycle event later, so this cursor lands
  // immediately before or inside the target batch; the Go SSE adapter then
  // rewinds an inside cursor to the batch boundary atomically.
  const terminalReplayCursor = Math.max(baselineSeq, expectedHighestSeq - 3)
  let cursor = terminalReplayCursor
  let replayEventCount = 0
  const streamId = `milestone-a-observation-${Math.random().toString(36).slice(2)}`
  let processing = Promise.resolve()
  let finish
  const outcome = new Promise((resolve) => {
    finish = (reasonCode, accepted = false) => {
      if (settled) return
      settled = true
      const receipt = accepted && pending ? pending : empty(reasonCode)
      resolve({
        ...receipt,
        providerReceiptTrace: {
          ...trace,
          reasonCode: reasonCodes.has(reasonCode)
            ? reasonCode
            : 'provider_receipt_observer_failed',
          started: startConfirmed,
          acknowledged,
          ended
        }
      })
    }
  })
  const timer = setTimeout(() => finish('provider_receipt_timeout'), timeoutMs)
  const maybeFinish = () => {
    if (!ended || settled) return
    if (!pending) {
      finish('provider_receipt_target_turn_missing')
    } else if (startConfirmed && acknowledged) {
      finish('provider_receipt_observed', true)
    }
  }
  const exactNonNegativeInteger = (value) =>
    Number.isSafeInteger(value) && value >= 0
  const exactKeys = (value, keys) => value && typeof value === 'object' &&
    !Array.isArray(value) && Object.keys(value).length === keys.length &&
    keys.every((key) => Object.prototype.hasOwnProperty.call(value, key))
  const safeAttemptStatuses = (value) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return null
    const keys = ['succeeded', 'failed', 'cancelled', 'timedOut', 'streamAborted']
    if (Object.keys(value).length !== keys.length ||
        keys.some((key) => !exactNonNegativeInteger(value[key]))) return null
    return Object.fromEntries(keys.map((key) => [key, value[key]]))
  }
  const claimsAcceptedFinalAuthority = (...records) => {
    const keys = [
      'acceptedFinal', 'acceptedFinalView', 'acceptedFinalDigest', 'publicationCommitId',
      'publicationEventId', 'publicationSlot', 'publicationPayloadDigest'
    ]
    return records.some((record) => record && typeof record === 'object' &&
      !Array.isArray(record) && keys.some((key) =>
        Object.prototype.hasOwnProperty.call(record, key)
      ))
  }
  const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
  const canonicalTimestamp = (value) => typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u.test(value)
  const publicId = (value) => typeof value === 'string' &&
    /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/u.test(value)
  const resultSlotFields = [
    'schemaVersion', 'purpose', 'projectionVersion', 'logicalEffect',
    'ordinaryWork', 'candidateOrigin', 'evidenceAuthority',
    'citationAuthority', 'factAnswerAllowed', 'text', 'textSha256',
    'resultDigest'
  ]
  const hostFixedProviderResultWithheld =
    '普通任务已执行，但模型结果正文未通过普通输出安全投影，因此未予发布。'
  const hostFixedProtectedFactBlocked =
    '本轮模型输出包含必须经过案件证据门核验的事实候选，宿主已阻止该草稿发布。请在已绑定的案件项目中重新发起核验。'
  const projectOrdinaryResultReceipt = (itemEvent) => {
    if (!itemEvent) return emptyOrdinaryResultReceipt('ordinary_result_absent')
    const item = itemEvent.item
    if (!item || typeof item !== 'object' || Array.isArray(item) ||
        !publicId(itemEvent.itemId) || itemEvent.itemId !== item.id ||
        item.threadId !== exactThreadId || item.turnId !== exactTurnId ||
        itemEvent.threadId !== item.threadId || itemEvent.turnId !== item.turnId ||
        itemEvent.timestamp !== item.finishedAt ||
        !exactKeys(item, item.ordinaryResult === undefined
          ? ['id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind', 'text']
          : ['id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind', 'text', 'ordinaryResult']) ||
        !publicId(item.id) || item.role !== 'assistant' ||
        item.status !== 'completed' || item.kind !== 'assistant_text' ||
        !canonicalTimestamp(item.createdAt) || !canonicalTimestamp(item.finishedAt) ||
        typeof item.text !== 'string' || !item.text || item.text !== item.text.trim()) {
      return emptyOrdinaryResultReceipt('ordinary_result_invalid')
    }
    if (item.ordinaryResult === undefined) {
      return item.text ===
        '本轮模型自由文本尚无宿主可验证的发布权限，草稿未进入消息、历史或导出。请使用受验证的工具或结构化成果物完成任务。'
        ? emptyOrdinaryResultReceipt('ordinary_result_legacy_boundary')
        : emptyOrdinaryResultReceipt('ordinary_result_invalid')
    }
    const slot = item.ordinaryResult
    if (!exactKeys(slot, resultSlotFields) || slot.schemaVersion !== 1 ||
        slot.purpose !== 'analytix.ordinary-result/v1' ||
        slot.projectionVersion !== 'analytix.ordinary-output-projection/v1' ||
        slot.logicalEffect !== 'ordinary' || slot.ordinaryWork !== true ||
        !['provider_ordinary_only', 'host_fixed'].includes(slot.candidateOrigin) ||
        slot.evidenceAuthority !== false || slot.citationAuthority !== false ||
        slot.factAnswerAllowed !== false || slot.text !== item.text ||
        !digest(slot.textSha256) || !digest(slot.resultDigest)) {
      return emptyOrdinaryResultReceipt('ordinary_result_invalid')
    }
    let projectionClass = 'provider_ordinary_only'
    if (slot.candidateOrigin === 'host_fixed') {
      if (slot.text === hostFixedProviderResultWithheld) {
        projectionClass = 'host_fixed_provider_result_withheld'
      } else if (slot.text === hostFixedProtectedFactBlocked) {
        projectionClass = 'host_fixed_protected_fact_blocked'
      } else {
        return emptyOrdinaryResultReceipt('ordinary_result_invalid')
      }
    }
    return {
      observed: true,
      reasonCode: 'ordinary_result_observed',
      candidateOrigin: slot.candidateOrigin,
      projectionClass,
      resultDigest: slot.resultDigest,
      textSha256: slot.textSha256,
      resultMarkerObserved: slot.text.includes('MILESTONE_A_RESULT_OK'),
      researchMarkerObserved: slot.text.includes('MILESTONE_A_RESEARCH_OK'),
      writingMarkerObserved: slot.text.includes('MILESTONE_A_WRITING_OK')
    }
  }
  const streamErrorReasonCode = (payload) => {
    const code = typeof payload?.code === 'string' &&
      /^[A-Za-z0-9_.-]{1,128}$/u.test(payload.code)
      ? payload.code
      : ''
    if (code === 'sse_event_rejected') {
      const reasonCode = typeof payload?.reasonCode === 'string' &&
        /^[a-z0-9_]{1,128}$/u.test(payload.reasonCode)
        ? `provider_receipt_sse_${payload.reasonCode}`
        : ''
      if (PROVIDER_RECEIPT_REASON_CODES.includes(reasonCode)) return reasonCode
      return 'provider_receipt_stream_event_rejected'
    }
    if (code === 'sse_stream_error') {
      return 'provider_receipt_stream_transport_error'
    }
    if (code) return 'provider_receipt_stream_setup_error'
    if (Number.isSafeInteger(payload?.status) &&
        payload.status >= 100 && payload.status <= 599) {
      return 'provider_receipt_stream_http_error'
    }
    return 'provider_receipt_stream_error'
  }
  const projectGeneralTerminalBatch = (batch) => {
    const batchFields = [
      'schemaVersion', 'purpose', 'kind', 'batchDigest', 'threadId', 'turnId',
      'seq', 'firstSeq', 'lastSeq', 'timestamp', 'generalTerminalCommitId',
      'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest',
      'eventManifestDigest', 'projectedEventsDigest', 'transportAuthority',
      'evidenceAuthority', 'citationAuthority', 'factAnswerAllowed', 'events',
      'eventManifest'
    ]
    if (!batch || typeof batch !== 'object' || Array.isArray(batch) ||
        !exactKeys(batch, batchFields) ||
        batch.schemaVersion !== 1 ||
        batch.purpose !== 'analytix.general-terminal-delivery-batch/v1' ||
        batch.kind !== 'general_terminal_batch' || batch.threadId !== exactThreadId ||
        typeof batch.turnId !== 'string' || !batch.turnId || batch.turnId.length > 256 ||
        !digest(batch.batchDigest) || !canonicalTimestamp(batch.timestamp) ||
        !digest(batch.generalTerminalCommitId) ||
        batch.generalTerminalAuthorityKind !== 'general_terminal_cas' ||
        !digest(batch.generalTerminalAuthorityDigest) ||
        !digest(batch.eventManifestDigest) || !digest(batch.projectedEventsDigest) ||
        batch.transportAuthority !== 'host_batch_digest_v1' ||
        batch.evidenceAuthority !== false || batch.citationAuthority !== false ||
        batch.factAnswerAllowed !== false || claimsAcceptedFinalAuthority(batch) ||
        !Number.isSafeInteger(batch.seq) || batch.seq <= cursor ||
        batch.seq > expectedHighestSeq ||
        !Number.isSafeInteger(batch.firstSeq) || batch.firstSeq <= baselineSeq ||
        !Number.isSafeInteger(batch.lastSeq) || batch.lastSeq !== batch.seq ||
        !Array.isArray(batch.events) || ![2, 3].includes(batch.events.length) ||
        !Array.isArray(batch.eventManifest) ||
        batch.eventManifest.length !== batch.events.length ||
        batch.lastSeq - batch.firstSeq + 1 !== batch.events.length) {
      return { reasonCode: 'provider_receipt_terminal_batch_invalid', projected: null }
    }
    const expectedSlots = batch.events.length === 3
      ? ['terminal-item', 'usage', 'terminal']
      : ['usage', 'terminal']
    const eventIds = new Set()
    for (let index = 0; index < batch.eventManifest.length; index += 1) {
      const manifest = batch.eventManifest[index]
      if (!exactKeys(manifest, ['slot', 'eventId', 'payloadDigest']) ||
          manifest.slot !== expectedSlots[index] || !digest(manifest.eventId) ||
          !digest(manifest.payloadDigest) || eventIds.has(manifest.eventId)) {
        return { reasonCode: 'provider_receipt_terminal_batch_invalid', projected: null }
      }
      eventIds.add(manifest.eventId)
    }
    const usage = batch.events[batch.events.length - 2]
    const terminal = batch.events[batch.events.length - 1]
    if (!usage || typeof usage !== 'object' || Array.isArray(usage) ||
        !terminal || typeof terminal !== 'object' || Array.isArray(terminal) ||
        usage.kind !== 'usage' || usage.threadId !== exactThreadId ||
        terminal.threadId !== exactThreadId || usage.turnId !== batch.turnId ||
        usage.timestamp !== batch.timestamp || terminal.timestamp !== batch.timestamp ||
        terminal.turnId !== batch.turnId || usage.seq !== batch.lastSeq - 1 ||
        terminal.seq !== batch.lastSeq ||
        claimsAcceptedFinalAuthority(usage, terminal) ||
        !['turn_completed', 'turn_failed', 'turn_aborted'].includes(terminal.kind) ||
        !['completed', 'failed', 'aborted'].includes(terminal.status) ||
        terminal.kind !== `turn_${terminal.status}` ||
        (usage.usageFinalStatus !== undefined && usage.usageFinalStatus !== terminal.status)) {
      return { reasonCode: 'provider_receipt_terminal_pair_invalid', projected: null }
    }
    const itemEvent = batch.events.length === 3 ? batch.events[0] : null
    if (itemEvent) {
      const item = itemEvent
      if (!item || typeof item !== 'object' || Array.isArray(item) ||
          item.kind !== 'item_completed' || item.threadId !== exactThreadId ||
          item.turnId !== batch.turnId || item.seq !== batch.firstSeq ||
          item.timestamp !== batch.timestamp ||
          claimsAcceptedFinalAuthority(item, item.item)) {
        return { reasonCode: 'provider_receipt_item_invalid', projected: null }
      }
    }
    const usageSnapshot = usage.usage
    const diagnostics = usage.cacheDiagnostics
    if (!usageSnapshot || typeof usageSnapshot !== 'object' || Array.isArray(usageSnapshot) ||
        !diagnostics || typeof diagnostics !== 'object' || Array.isArray(diagnostics) ||
        !exactNonNegativeInteger(usageSnapshot.promptTokens) ||
        !exactNonNegativeInteger(usageSnapshot.completionTokens) ||
        !exactNonNegativeInteger(usageSnapshot.totalTokens) ||
        !exactNonNegativeInteger(usageSnapshot.turns) ||
        typeof usage.model !== 'string' || usage.model.length > 256 ||
        (usage.usageFinalStatus !== undefined &&
          !['completed', 'failed', 'aborted'].includes(usage.usageFinalStatus)) ||
        diagnostics.providerAttemptTelemetrySchema !== 'provider-attempt-telemetry.v1' ||
        diagnostics.providerAttemptTelemetryValid !== true ||
        !exactNonNegativeInteger(diagnostics.providerLogicalCallCount) ||
        !exactNonNegativeInteger(diagnostics.providerAttemptCount)) {
      return { reasonCode: 'provider_receipt_usage_telemetry_invalid', projected: null }
    }
    const statuses = safeAttemptStatuses(diagnostics.providerAttemptStatuses)
    if (!statuses) {
      return { reasonCode: 'provider_receipt_attempt_status_invalid', projected: null }
    }
    const attemptStatusCount = Object.values(statuses).reduce(
      (total, value) => total + value,
      0
    )
    if (diagnostics.providerLogicalCallCount > diagnostics.providerAttemptCount ||
        attemptStatusCount !== diagnostics.providerAttemptCount ||
        statuses.succeeded > diagnostics.providerLogicalCallCount) {
      return { reasonCode: 'provider_receipt_attempt_counts_invalid', projected: null }
    }
    const ordinaryResultReceipt = batch.turnId === exactTurnId && terminal.status === 'completed'
      ? projectOrdinaryResultReceipt(itemEvent)
      : emptyOrdinaryResultReceipt('ordinary_result_absent')
    if (batch.turnId === exactTurnId && terminal.status === 'completed' &&
        ordinaryResultReceipt.reasonCode === 'ordinary_result_invalid') {
      return { reasonCode: 'provider_receipt_item_invalid', projected: null }
    }
    if (batch.turnId === exactTurnId) trace.eventCount = batch.events.length
    return {
      reasonCode: 'provider_receipt_observed',
      projected: batch.turnId === exactTurnId ? {
        providerAttempts: [{
          kind: 'usage',
          seq: usage.seq,
          threadId: usage.threadId,
          turnId: usage.turnId,
          model: usage.model,
          usageFinalStatus: usage.usageFinalStatus,
          promptTokens: usageSnapshot.promptTokens,
          completionTokens: usageSnapshot.completionTokens,
          totalTokens: usageSnapshot.totalTokens,
          turns: usageSnapshot.turns,
          providerAttemptTelemetrySchema: diagnostics.providerAttemptTelemetrySchema,
          providerAttemptTelemetryValid: diagnostics.providerAttemptTelemetryValid,
          providerLogicalCallCount: diagnostics.providerLogicalCallCount,
          providerAttemptCount: diagnostics.providerAttemptCount,
          providerAttemptStatuses: statuses
        }],
        providerTerminals: [{
          kind: terminal.kind,
          seq: terminal.seq,
          threadId: terminal.threadId,
          turnId: terminal.turnId,
          status: terminal.status
        }],
        ordinaryResultReceipt
      } : null,
      target: batch.turnId === exactTurnId
    }
  }

  try {
    const unsubscribeEvent = api.onSseEvent((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(async () => {
        if (settled) return
        if (!exactKeys(payload, ['streamId', 'events']) ||
            !Array.isArray(payload.events) || payload.events.length < 1 ||
            // The typed preload bridge validates every event but does not impose
            // a per-IPC-envelope count. Complex replay chunks can therefore
            // legitimately exceed 128 events. Keep the observer bounded by its
            // cumulative replay limit instead of inventing a narrower producer
            // contract.
            replayEventCount + payload.events.length > 4096) {
          finish('provider_receipt_event_envelope_invalid')
          return
        }
        if (pending) {
          finish('provider_receipt_event_scope_invalid')
          return
        }
        let batchMaxSeq = cursor
        for (const event of payload.events) {
          if (!event || typeof event !== 'object' || Array.isArray(event) ||
              !Number.isSafeInteger(event.seq) || event.seq > expectedHighestSeq) {
            finish('provider_receipt_event_sequence_invalid')
            return
          }
          if (event.threadId !== exactThreadId) {
            finish('provider_receipt_event_scope_invalid')
            return
          }
          if (event.kind === 'heartbeat') {
            if (event.seq !== batchMaxSeq || typeof event.timestamp !== 'string' ||
                !event.timestamp.trim() || event.timestamp.length > 128 ||
                event.turnId !== undefined || event.itemId !== undefined ||
                claimsAcceptedFinalAuthority(event)) {
              finish('provider_receipt_event_sequence_invalid')
              return
            }
            replayEventCount += 1
            continue
          }
          if (event.seq <= batchMaxSeq) {
            finish('provider_receipt_event_sequence_invalid')
            return
          }
          if (event.kind === 'accepted_final_batch') {
            finish('provider_receipt_non_general_terminal')
            return
          }
          if (event.kind === 'general_terminal_batch') {
            if (payload.events.length !== 1) {
              finish('provider_receipt_event_envelope_invalid')
              return
            }
            const projection = projectGeneralTerminalBatch(event)
            if (projection.reasonCode !== 'provider_receipt_observed') {
              finish(projection.reasonCode)
              return
            }
            if (!projection.target || !projection.projected) {
              finish('provider_receipt_event_scope_invalid')
              return
            }
            pending = projection.projected
          }
          batchMaxSeq = event.seq
          replayEventCount += 1
        }
        let accepted
        try {
          accepted = await api.ackSseEvent(streamId, batchMaxSeq)
        } catch {
          finish('provider_receipt_ack_failed')
          return
        }
        if (accepted !== true) {
          finish('provider_receipt_ack_rejected')
          return
        }
        acknowledged = true
        cursor = batchMaxSeq
      }).catch(() => finish('provider_receipt_observer_failed'))
    })
    const unsubscribeEnd = api.onSseEnd((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => {
        if (!exactKeys(payload, ['streamId'])) {
          finish('provider_receipt_event_envelope_invalid')
          return
        }
        ended = true
        maybeFinish()
      }).catch(() => finish('provider_receipt_observer_failed'))
    })
    const unsubscribeError = api.onSseError((payload) => {
      if (payload && payload.streamId === streamId) {
        finish(streamErrorReasonCode(payload))
      }
    })
    for (const unsubscribe of [unsubscribeEvent, unsubscribeEnd, unsubscribeError]) {
      if (typeof unsubscribe === 'function') unsubscribers.push(unsubscribe)
    }
    if (unsubscribers.length !== 3) {
      finish('provider_receipt_listener_invalid')
    }
    if (settled) return await outcome
    void Promise.resolve(api.startSse(
      exactThreadId,
      terminalReplayCursor,
      streamId
    )).then((started) => {
      if (!started || started.streamId !== streamId) {
        finish('provider_receipt_start_rejected')
        return
      }
      startConfirmed = true
      maybeFinish()
    }).catch(() => finish('provider_receipt_start_failed'))
    return await outcome
  } catch {
    finish('provider_receipt_observer_failed')
    return await outcome
  } finally {
    clearTimeout(timer)
    for (const unsubscribe of unsubscribers) {
      try { unsubscribe() } catch {
        // Best-effort cleanup after the observation has already settled.
      }
    }
    try { await api.stopSse(streamId) } catch {
      // Best-effort cleanup after the observation has already settled.
    }
  }
}

function acceptedFinalPublicViewV3ShapeValid(
  value,
  expectedDigest = '',
  expectedTimestamp = ''
) {
  const exactKeys = (candidate, keys) => candidate && typeof candidate === 'object' &&
    !Array.isArray(candidate) && Object.keys(candidate).length === keys.length &&
    keys.every((key) => Object.prototype.hasOwnProperty.call(candidate, key))
  const digest = (candidate) => typeof candidate === 'string' &&
    /^[0-9a-f]{64}$/u.test(candidate)
  const timestamp = (candidate) => {
    if (typeof candidate !== 'string' || /\.[0-9]*0Z$/u.test(candidate)) return false
    const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.([0-9]{1,9}))?Z$/u
      .exec(candidate)
    if (!match) return false
    const [year, month, day, hour, minute, second] = match.slice(1, 7).map(Number)
    if (month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59) {
      return false
    }
    const date = new Date(0)
    date.setUTCFullYear(year, month - 1, day)
    date.setUTCHours(hour, minute, second, 0)
    return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 &&
      date.getUTCDate() === day && date.getUTCHours() === hour &&
      date.getUTCMinutes() === minute && date.getUTCSeconds() === second
  }
  const topLevelKeys = [
    'schemaVersion', 'acceptedFinalDigest', 'publicationState', 'variant',
    'terminalReason', 'blockerCode', 'coverageStatus', 'checkedScopeDigest',
    'missingScopeCount', 'claimCount', 'claimTypes', 'receiptMetadata',
    'noHitWording', 'acceptedAt'
  ]
  const terminalReasons = new Set([
    'success', 'source_unavailable', 'semantic_failure', 'provider_failure',
    'cancel', 'timeout', 'stream_abort', 'recovery', 'approval', 'user_input',
    'resume', 'restart', 'report_fallback', 'step_limit',
    'background_completion', 'tool_failure', 'approval_denied', 'input_cancelled'
  ])
  const claimTypes = new Set([
    'amount', 'count', 'account', 'entity', 'direction', 'date_range',
    'relationship', 'quote', 'device_identifier', 'ownership', 'control',
    'address', 'change', 'bid_certificate', 'bid_edit_metadata',
    'legal_characterization'
  ])
  if (!exactKeys(value, topLevelKeys) || value.schemaVersion !== 3 ||
      value.publicationState !== 'accepted' || !digest(value.acceptedFinalDigest) ||
      (expectedDigest && value.acceptedFinalDigest !== expectedDigest) ||
      !terminalReasons.has(value.terminalReason) ||
      typeof value.blockerCode !== 'string' || value.blockerCode.length > 96 ||
      !/^(?:[a-z0-9_-]+)?$/u.test(value.blockerCode) ||
      (value.checkedScopeDigest !== '' && !digest(value.checkedScopeDigest)) ||
      !Number.isSafeInteger(value.missingScopeCount) || value.missingScopeCount < 0 ||
      !Number.isSafeInteger(value.claimCount) || value.claimCount < 0 ||
      !Array.isArray(value.claimTypes) ||
      value.claimTypes.some((candidate) => !claimTypes.has(candidate)) ||
      value.claimTypes.some((candidate, index) => index > 0 && candidate <= value.claimTypes[index - 1]) ||
      value.claimTypes.length > value.claimCount ||
      ((value.claimCount === 0) !== (value.claimTypes.length === 0)) ||
      !['', 'not_found_in_checked_scope'].includes(value.noHitWording) ||
      !timestamp(value.acceptedAt) ||
      (expectedTimestamp && value.acceptedAt !== expectedTimestamp)) {
    return false
  }
  const receipts = value.receiptMetadata
  if (!exactKeys(receipts, ['projection', 'count', 'setDigest', 'citations']) ||
      receipts.projection !== 'masked_metadata_only' ||
      !Number.isSafeInteger(receipts.count) || receipts.count < 0 ||
      !digest(receipts.setDigest) || !Array.isArray(receipts.citations) ||
      receipts.citations.length !== receipts.count) {
    return false
  }
  const handles = new Set()
  for (const [index, citation] of receipts.citations.entries()) {
    if (!exactKeys(citation, ['handle', 'label']) ||
        typeof citation.handle !== 'string' || !/^cite_[0-9a-f]{64}$/u.test(citation.handle) ||
        citation.label !== `evidence-${index + 1}` || handles.has(citation.handle)) {
      return false
    }
    handles.add(citation.handle)
  }
  const hasCheckedScope = value.checkedScopeDigest !== ''
  switch (value.variant) {
    case 'EvidenceBackedAnswer':
      return value.coverageStatus === 'complete' && value.claimCount > 0 &&
        receipts.count > 0 && hasCheckedScope && value.missingScopeCount === 0 &&
        value.blockerCode === '' && value.noHitWording === ''
    case 'PartialEvidenceAnswer':
      return value.coverageStatus === 'partial' && value.claimCount > 0 &&
        receipts.count > 0 && hasCheckedScope && value.missingScopeCount > 0 &&
        value.blockerCode === '' && value.noHitWording === ''
    case 'VerifiedNoHitAnswer':
      return value.coverageStatus === 'complete' && value.claimCount === 0 &&
        receipts.count > 0 && hasCheckedScope && value.missingScopeCount === 0 &&
        value.blockerCode === '' && value.noHitWording === 'not_found_in_checked_scope'
    case 'SourceUnavailableAnswer':
      return value.coverageStatus === 'unavailable' && value.claimCount === 0 &&
        receipts.count === 0 && value.blockerCode !== '' && value.noHitWording === ''
    case 'NeedsEvidenceAnswer':
      return value.coverageStatus === 'unverified' && value.claimCount === 0 &&
        receipts.count === 0 && value.missingScopeCount > 0 && value.noHitWording === ''
    case 'GeneralGuidanceAnswer':
      return value.coverageStatus === 'guidance_only' && value.claimCount === 0 &&
        receipts.count === 0 && !hasCheckedScope && value.missingScopeCount === 0 &&
        value.blockerCode === '' && value.noHitWording === ''
    default:
      return false
  }
}

function acceptedFinalDeliveryV2ShapeBound(delivery, threadId, turnId, view) {
  const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
  const batchKeys = [
    'schemaVersion', 'purpose', 'kind', 'batchId', 'threadId', 'turnId', 'seq',
    'firstSeq', 'lastSeq', 'timestamp', 'publicationCommitId', 'eventManifestDigest',
    'publicationAuthority', 'events'
  ]
  const authorityKeys = [
    'schemaVersion', 'purpose', 'sealId', 'threadId', 'turnId', 'publicationCommitId',
    'acceptedFinalDispositionDigest', 'terminalDispositionId', 'eventManifestDigest',
    'sequencedEventsDigest', 'batchId', 'firstSeq', 'lastSeq', 'timestamp',
    'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature'
  ]
  if ((!exactObjectKeys(delivery, batchKeys) &&
      !exactObjectKeys(delivery, [...batchKeys, 'trace'])) ||
      delivery.schemaVersion !== 2 ||
      delivery.purpose !== 'analytix.accepted-final-delivery-batch/v2' ||
      delivery.kind !== 'accepted_final_batch' || delivery.threadId !== threadId ||
      delivery.turnId !== turnId || !digest(delivery.batchId) ||
      !digest(delivery.publicationCommitId) || !digest(delivery.eventManifestDigest) ||
      !Number.isSafeInteger(delivery.firstSeq) || delivery.firstSeq <= 0 ||
      !Number.isSafeInteger(delivery.lastSeq) || delivery.lastSeq < delivery.firstSeq ||
      delivery.seq !== delivery.lastSeq || !Array.isArray(delivery.events) ||
      ![3, 4].includes(delivery.events.length) ||
      delivery.lastSeq - delivery.firstSeq + 1 !== delivery.events.length ||
      !acceptedFinalPublicViewV3ShapeValid(
        view,
        delivery.publicationCommitId,
        delivery.timestamp
      )) {
    return false
  }
  const authority = delivery.publicationAuthority
  if (!exactObjectKeys(authority, authorityKeys) ||
      authority.schemaVersion !== 'accepted-final-delivery-seal.v1' ||
      authority.purpose !== 'analytix.accepted-final-delivery-seal/v1' ||
      !digest(authority.sealId) || authority.threadId !== threadId ||
      authority.turnId !== turnId ||
      authority.publicationCommitId !== delivery.publicationCommitId ||
      !digest(authority.acceptedFinalDispositionDigest) ||
      !digest(authority.terminalDispositionId) ||
      authority.eventManifestDigest !== delivery.eventManifestDigest ||
      !digest(authority.sequencedEventsDigest) || authority.batchId !== delivery.batchId ||
      authority.firstSeq !== delivery.firstSeq || authority.lastSeq !== delivery.lastSeq ||
      authority.timestamp !== delivery.timestamp || authority.authorityAlgorithm !== 'Ed25519' ||
      !digest(authority.authorityKeyId) ||
      typeof authority.authorityPublicKey !== 'string' ||
      !/^[A-Za-z0-9_-]{43}$/u.test(authority.authorityPublicKey) ||
      typeof authority.authoritySignature !== 'string' ||
      !/^[A-Za-z0-9_-]{86}$/u.test(authority.authoritySignature)) {
    return false
  }
  for (const [index, event] of delivery.events.entries()) {
    if (!event || typeof event !== 'object' || Array.isArray(event) ||
        event.threadId !== threadId || event.turnId !== turnId ||
        event.seq !== delivery.firstSeq + index || event.timestamp !== delivery.timestamp ||
        event.acceptedFinalDigest !== delivery.publicationCommitId ||
        event.publicationCommitId !== delivery.publicationCommitId ||
        !digest(event.publicationEventId) || !digest(event.publicationPayloadDigest)) {
      return false
    }
  }
  const assistant = delivery.events[0]
  const usage = delivery.events.at(-2)
  const terminal = delivery.events.at(-1)
  if (assistant.kind !== 'item_completed' || assistant.publicationSlot !== 'assistant-final' ||
      assistant.itemId !== assistant.item?.id || assistant.item?.acceptedFinal !== undefined ||
      !exactObjectKeys(assistant.item, [
        'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
        'kind', 'text', 'acceptedFinalView'
      ]) || assistant.item.threadId !== threadId || assistant.item.turnId !== turnId ||
      assistant.item.role !== 'assistant' || assistant.item.status !== 'completed' ||
      assistant.item.kind !== 'assistant_text' || assistant.item.finishedAt !== delivery.timestamp ||
      canonicalJSON(assistant.item.acceptedFinalView) !== canonicalJSON(view) ||
      usage?.kind !== 'usage' || usage?.publicationSlot !== 'usage' ||
      terminal?.publicationSlot !== 'terminal' || terminal?.terminalReason !== view.terminalReason) {
    return false
  }
  const terminalStatus = new Map([
    ['success', 'completed'], ['source_unavailable', 'completed'],
    ['semantic_failure', 'failed'], ['provider_failure', 'failed'], ['cancel', 'aborted'],
    ['timeout', 'failed'], ['stream_abort', 'failed'], ['recovery', 'completed'],
    ['approval', 'completed'], ['user_input', 'completed'], ['resume', 'completed'],
    ['restart', 'aborted'], ['report_fallback', 'failed'], ['step_limit', 'failed'],
    ['background_completion', 'completed'], ['tool_failure', 'failed'],
    ['approval_denied', 'completed'], ['input_cancelled', 'completed']
  ]).get(view.terminalReason)
  const requiresErrorItem = new Set([
    'semantic_failure', 'provider_failure', 'cancel', 'timeout', 'stream_abort',
    'restart', 'report_fallback', 'step_limit', 'tool_failure', 'approval_denied',
    'input_cancelled'
  ]).has(view.terminalReason)
  return terminalStatus !== undefined && terminal.kind === `turn_${terminalStatus}` &&
    terminal.status === terminalStatus && usage.usageFinalStatus === terminalStatus &&
    (delivery.events.length === 4) === requiresErrorItem
}

export async function observeAcceptedFinalOrdinaryViaSse(
  runtimeApi,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  timeoutMs = 5000
) {
  const trace = {
    reasonCode: 'accepted_final_not_observed',
    started: false,
    acknowledged: false,
    ended: false,
    eventCount: 0
  }
  const empty = (reasonCode, overrides = {}) => ({
    observed: false,
    observationCode: reasonCode,
    reasonCode,
    threadId: '',
    turnId: '',
    baselineSeq,
    firstSeq: 0,
    terminalSeq: 0,
    highestSeq: expectedHighestSeq,
    acceptedFinalDigest: '',
    deliveryManifestDigest: '',
    terminalReason: '',
    providerAttempts: [],
    providerTerminals: [],
    providerReceiptTrace: { ...trace, reasonCode, ...overrides }
  })
  const exactThreadId = typeof threadId === 'string' ? threadId.trim() : ''
  const exactTurnId = typeof expectedTurnId === 'string' ? expectedTurnId.trim() : ''
  if (!runtimeApi || !exactThreadId || exactThreadId !== threadId || exactThreadId.length > 256 ||
      !exactTurnId || exactTurnId !== expectedTurnId || exactTurnId.length > 256 ||
      !Number.isSafeInteger(baselineSeq) || baselineSeq < 0 ||
      !Number.isSafeInteger(expectedHighestSeq) || expectedHighestSeq <= baselineSeq ||
      !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 60_000 ||
      typeof runtimeApi.startSse !== 'function' || typeof runtimeApi.ackSseEvent !== 'function' ||
      typeof runtimeApi.stopSse !== 'function' || typeof runtimeApi.onSseEvent !== 'function' ||
      typeof runtimeApi.onSseEnd !== 'function' || typeof runtimeApi.onSseError !== 'function') {
    return empty('accepted_final_input_invalid')
  }

  const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
  const canonicalTimestamp = (value) => typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u.test(value)
  const exactKeys = (value, keys) => value && typeof value === 'object' &&
    !Array.isArray(value) && Object.keys(value).length === keys.length &&
    keys.every((key) => Object.prototype.hasOwnProperty.call(value, key))
  const validTrace = (value) => value === undefined || (
    exactKeys(value, ['sse_sent_at', 'sse_live_emitted_at']) &&
    Number.isFinite(value.sse_sent_at) && value.sse_sent_at >= 0 &&
    Number.isFinite(value.sse_live_emitted_at) && value.sse_live_emitted_at >= 0
  )
  const exactKeysWithOptionalTrace = (value, keys) =>
    exactKeys(value, keys) || (
      exactKeys(value, [...keys, 'trace']) && validTrace(value.trace)
    )
  const turnStartedKeys = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'status'
  ]
  const turnStartedValid = (event, cursor) =>
    exactKeysWithOptionalTrace(event, turnStartedKeys) &&
    event.kind === 'turn_started' && event.threadId === exactThreadId &&
    event.turnId === exactTurnId && event.status === 'running' &&
    event.seq === cursor + 1 && canonicalTimestamp(event.timestamp) &&
    validTrace(event.trace)
  const batchKeys = [
    'schemaVersion', 'purpose', 'kind', 'batchId', 'threadId', 'turnId', 'seq',
    'firstSeq', 'lastSeq', 'timestamp', 'publicationCommitId', 'eventManifestDigest',
    'publicationAuthority', 'events'
  ]
  const batchKeysWithTrace = [...batchKeys, 'trace']
  const authorityKeys = [
    'schemaVersion', 'purpose', 'sealId', 'threadId', 'turnId', 'publicationCommitId',
    'acceptedFinalDispositionDigest', 'terminalDispositionId', 'eventManifestDigest',
    'sequencedEventsDigest', 'batchId', 'firstSeq', 'lastSeq', 'timestamp',
    'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature'
  ]
  const statusesValid = (value) => value && typeof value === 'object' && !Array.isArray(value) &&
    exactKeys(value, ['succeeded', 'failed', 'cancelled', 'timedOut', 'streamAborted']) &&
    Object.values(value).every((item) => Number.isSafeInteger(item) && item >= 0)
  const projectBatch = (batch, cursor) => {
    if (!batch || typeof batch !== 'object' || Array.isArray(batch)) {
      return { reasonCode: 'accepted_final_schema_invalid', projected: null }
    }
    if (Number.isSafeInteger(batch.firstSeq) && batch.firstSeq > 0 &&
        batch.firstSeq !== cursor + 1) {
      return { reasonCode: 'accepted_final_event_sequence_invalid', projected: null }
    }
    if ((!exactKeys(batch, batchKeys) && !exactKeys(batch, batchKeysWithTrace)) ||
        !validTrace(batch.trace) || batch.schemaVersion !== 2 ||
        batch.purpose !== 'analytix.accepted-final-delivery-batch/v2' ||
        batch.kind !== 'accepted_final_batch' || batch.threadId !== exactThreadId ||
        batch.turnId !== exactTurnId || !digest(batch.batchId) ||
        !digest(batch.publicationCommitId) || !digest(batch.eventManifestDigest) ||
        !Number.isSafeInteger(batch.seq) || !Number.isSafeInteger(batch.firstSeq) ||
        !Number.isSafeInteger(batch.lastSeq) || batch.seq !== batch.lastSeq ||
        batch.firstSeq <= cursor || batch.lastSeq > expectedHighestSeq ||
        batch.lastSeq - batch.firstSeq + 1 !== 3 || !canonicalTimestamp(batch.timestamp) ||
        !exactKeys(batch.publicationAuthority, authorityKeys) ||
        !Array.isArray(batch.events) || batch.events.length !== 3) {
      const rangeLooksTerminalMissing = Array.isArray(batch.events) && batch.events.length !== 3
      return {
        reasonCode: rangeLooksTerminalMissing
          ? 'accepted_final_terminal_missing'
          : 'accepted_final_schema_invalid',
        projected: null
      }
    }
    const authority = batch.publicationAuthority
    if (authority.schemaVersion !== 'accepted-final-delivery-seal.v1' ||
        authority.purpose !== 'analytix.accepted-final-delivery-seal/v1' ||
        !digest(authority.sealId) || authority.threadId !== exactThreadId ||
        authority.turnId !== exactTurnId ||
        authority.publicationCommitId !== batch.publicationCommitId ||
        !digest(authority.acceptedFinalDispositionDigest) ||
        !digest(authority.terminalDispositionId) ||
        authority.eventManifestDigest !== batch.eventManifestDigest ||
        !digest(authority.sequencedEventsDigest) || authority.batchId !== batch.batchId ||
        authority.firstSeq !== batch.firstSeq || authority.lastSeq !== batch.lastSeq ||
        authority.timestamp !== batch.timestamp || authority.authorityAlgorithm !== 'Ed25519' ||
        !digest(authority.authorityKeyId) ||
        typeof authority.authorityPublicKey !== 'string' ||
        !/^[A-Za-z0-9_-]{43}$/u.test(authority.authorityPublicKey) ||
        typeof authority.authoritySignature !== 'string' ||
        !/^[A-Za-z0-9_-]{86}$/u.test(authority.authoritySignature)) {
      return { reasonCode: 'accepted_final_schema_invalid', projected: null }
    }
    const [assistantEvent, usageEvent, terminalEvent] = batch.events
    if (!assistantEvent || !usageEvent || !terminalEvent ||
        assistantEvent.kind !== 'item_completed' ||
        usageEvent.kind !== 'usage' || terminalEvent.kind !== 'turn_completed' ||
        terminalEvent.status !== 'completed' || terminalEvent.terminalReason !== 'success') {
      return { reasonCode: 'accepted_final_terminal_missing', projected: null }
    }
    for (const [index, event] of batch.events.entries()) {
      if (!event || typeof event !== 'object' || Array.isArray(event) ||
          event.threadId !== exactThreadId || event.turnId !== exactTurnId ||
          event.timestamp !== batch.timestamp || event.seq !== batch.firstSeq + index ||
          event.acceptedFinalDigest !== batch.publicationCommitId ||
          event.publicationCommitId !== batch.publicationCommitId ||
          !digest(event.publicationEventId) || !digest(event.publicationPayloadDigest)) {
        return { reasonCode: 'accepted_final_schema_invalid', projected: null }
      }
    }
    if (assistantEvent.publicationSlot !== 'assistant-final' ||
        usageEvent.publicationSlot !== 'usage' || terminalEvent.publicationSlot !== 'terminal' ||
        !assistantEvent.item || typeof assistantEvent.item !== 'object' ||
        !exactKeys(assistantEvent.item, [
          'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
          'kind', 'text', 'acceptedFinalView'
        ]) || assistantEvent.item.kind !== 'assistant_text' ||
        assistantEvent.item.role !== 'assistant' || assistantEvent.item.status !== 'completed' ||
        assistantEvent.item.threadId !== exactThreadId ||
        assistantEvent.item.turnId !== exactTurnId ||
        assistantEvent.item.finishedAt !== batch.timestamp ||
        typeof assistantEvent.item.text !== 'string' ||
        !acceptedFinalPublicViewV3ShapeValid(
          assistantEvent.item.acceptedFinalView,
          batch.publicationCommitId,
          batch.timestamp
        ) ||
        usageEvent.usageFinalStatus !== 'completed' || typeof usageEvent.model !== 'string' ||
        usageEvent.model.length === 0 || usageEvent.model.length > 256 ||
        !usageEvent.usage || typeof usageEvent.usage !== 'object' ||
        !Number.isSafeInteger(usageEvent.usage.promptTokens) ||
        usageEvent.usage.promptTokens <= 0 ||
        !Number.isSafeInteger(usageEvent.usage.completionTokens) ||
        usageEvent.usage.completionTokens <= 0 ||
        !Number.isSafeInteger(usageEvent.usage.totalTokens) ||
        usageEvent.usage.totalTokens !==
          usageEvent.usage.promptTokens + usageEvent.usage.completionTokens ||
        usageEvent.usage.turns !== 1 ||
        !usageEvent.cacheDiagnostics ||
        usageEvent.cacheDiagnostics.providerAttemptTelemetrySchema !==
          'provider-attempt-telemetry.v1' ||
        usageEvent.cacheDiagnostics.providerAttemptTelemetryValid !== true ||
        !Number.isSafeInteger(usageEvent.cacheDiagnostics.providerLogicalCallCount) ||
        !Number.isSafeInteger(usageEvent.cacheDiagnostics.providerAttemptCount) ||
        !statusesValid(usageEvent.cacheDiagnostics.providerAttemptStatuses)) {
      return { reasonCode: 'accepted_final_schema_invalid', projected: null }
    }
    const statuses = usageEvent.cacheDiagnostics.providerAttemptStatuses
    const attemptCount = Object.values(statuses).reduce((total, value) => total + value, 0)
    if (usageEvent.cacheDiagnostics.providerLogicalCallCount <= 0 ||
        usageEvent.cacheDiagnostics.providerAttemptCount <
          usageEvent.cacheDiagnostics.providerLogicalCallCount ||
        attemptCount !== usageEvent.cacheDiagnostics.providerAttemptCount ||
        statuses.succeeded !== usageEvent.cacheDiagnostics.providerLogicalCallCount) {
      return { reasonCode: 'accepted_final_schema_invalid', projected: null }
    }
    const providerAttempt = {
      kind: 'usage',
      seq: usageEvent.seq,
      threadId: usageEvent.threadId,
      turnId: usageEvent.turnId,
      model: usageEvent.model,
      usageFinalStatus: usageEvent.usageFinalStatus,
      promptTokens: usageEvent.usage.promptTokens,
      completionTokens: usageEvent.usage.completionTokens,
      totalTokens: usageEvent.usage.totalTokens,
      turns: usageEvent.usage.turns,
      providerAttemptTelemetrySchema: usageEvent.cacheDiagnostics.providerAttemptTelemetrySchema,
      providerAttemptTelemetryValid: usageEvent.cacheDiagnostics.providerAttemptTelemetryValid,
      providerLogicalCallCount: usageEvent.cacheDiagnostics.providerLogicalCallCount,
      providerAttemptCount: usageEvent.cacheDiagnostics.providerAttemptCount,
      providerAttemptStatuses: { ...statuses }
    }
    const providerTerminal = {
      kind: terminalEvent.kind,
      seq: terminalEvent.seq,
      threadId: terminalEvent.threadId,
      turnId: terminalEvent.turnId,
      status: terminalEvent.status
    }
    return {
      reasonCode: 'accepted_final_observed',
      projected: {
        observed: true,
        observationCode: 'accepted_final_observed',
        reasonCode: 'accepted_final_observed',
        threadId: exactThreadId,
        turnId: exactTurnId,
        baselineSeq,
        firstSeq: batch.firstSeq,
        terminalSeq: batch.lastSeq,
        highestSeq: expectedHighestSeq,
        acceptedFinalDigest: batch.publicationCommitId,
        deliveryManifestDigest: batch.eventManifestDigest,
        terminalReason: assistantEvent.item.acceptedFinalView.terminalReason,
        providerAttempts: [providerAttempt],
        providerTerminals: [providerTerminal],
        providerReceiptTrace: {
          ...trace,
          reasonCode: 'accepted_final_observed',
          eventCount: batch.events.length
        }
      },
      ackBinding: Object.freeze({
        batchId: batch.batchId,
        threadId: batch.threadId,
        turnId: batch.turnId,
        publicationCommitId: batch.publicationCommitId
      })
    }
  }

  const unsubscribers = []
  const streamId = `milestone-a-accepted-final-${Math.random().toString(36).slice(2)}`
  let processing = Promise.resolve()
  let settled = false
  let started = false
  let acknowledged = false
  let ended = false
  let cursor = baselineSeq
  let acceptedBatch = null
  let turnStartedSeen = false
  let cursorAdvancedSeen = false
  let finish
  const outcome = new Promise((resolve) => {
    finish = (reasonCode, accepted = false) => {
      if (settled) return
      settled = true
      if (accepted && acceptedBatch) {
        resolve({
          ...acceptedBatch,
          providerReceiptTrace: {
            ...acceptedBatch.providerReceiptTrace,
            started,
            acknowledged,
            ended
          }
        })
      } else {
        resolve(empty(reasonCode, { started, acknowledged, ended }))
      }
    }
  })
  const maybeFinish = () => {
    if (!ended || settled) return
    if (!acceptedBatch) {
      finish('accepted_final_terminal_missing')
    } else if (cursor !== expectedHighestSeq) {
      finish('accepted_final_event_sequence_invalid')
    } else if (started && acknowledged) {
      finish('accepted_final_observed', true)
    }
  }
  const timer = setTimeout(() => finish('accepted_final_timeout'), timeoutMs)
  const ack = async (nextCursor, acceptedFinalBinding) => {
    let accepted
    try {
      accepted = acceptedFinalBinding === undefined
        ? await runtimeApi.ackSseEvent(streamId, nextCursor)
        : await runtimeApi.ackSseEvent(streamId, nextCursor, acceptedFinalBinding)
    } catch {
      finish('accepted_final_ack_failed')
      return false
    }
    if (accepted !== true) {
      finish('accepted_final_ack_rejected')
      return false
    }
    acknowledged = true
    cursor = Math.max(cursor, nextCursor)
    return true
  }
  try {
    const unsubscribeEvent = runtimeApi.onSseEvent((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(async () => {
        if (settled) return
        const payloadKeys = ['streamId', 'events']
        if (!exactKeys(payload, payloadKeys) || !Array.isArray(payload.events) ||
            payload.events.length < 1 || payload.events.length > 128) {
          finish('accepted_final_event_envelope_invalid')
          return
        }
        let batchMaxSeq = cursor
        let acceptedFinalAckBinding
        for (const event of payload.events) {
          if (!event || typeof event !== 'object' || Array.isArray(event) ||
              !Number.isSafeInteger(event.seq) || event.seq > expectedHighestSeq ||
              event.seq < 0 || event.threadId !== exactThreadId) {
            finish('accepted_final_event_scope_invalid')
            return
          }
          if (event.kind === 'heartbeat') {
            if (!exactKeysWithOptionalTrace(
              event,
              ['kind', 'seq', 'timestamp', 'threadId']
            ) || event.seq !== batchMaxSeq || !canonicalTimestamp(event.timestamp) ||
                !validTrace(event.trace)) {
              finish('accepted_final_event_sequence_invalid')
              return
            }
            continue
          }
          if (event.kind === 'turn_started') {
            if (turnStartedSeen || cursorAdvancedSeen || acceptedBatch ||
                !turnStartedValid(event, batchMaxSeq)) {
              finish('accepted_final_event_scope_invalid')
              return
            }
            turnStartedSeen = true
            batchMaxSeq = event.seq
            continue
          }
          if (event.kind === 'cursor_advanced') {
            if (!exactKeysWithOptionalTrace(
              event,
              ['kind', 'seq', 'timestamp', 'threadId', 'reason']
            ) || event.seq <= batchMaxSeq ||
                event.reason !== 'restricted_content_removed' ||
                !canonicalTimestamp(event.timestamp) || !validTrace(event.trace)) {
              finish('accepted_final_event_sequence_invalid')
              return
            }
            cursorAdvancedSeen = true
            batchMaxSeq = event.seq
            continue
          }
          if (event.kind !== 'accepted_final_batch') {
            finish('accepted_final_event_scope_invalid')
            return
          }
          if (payload.events.length !== 1 || acceptedBatch) {
            finish('accepted_final_duplicate')
            return
          }
          if (event.turnId !== exactTurnId) {
            finish('accepted_final_event_scope_invalid')
            return
          }
          const projection = projectBatch(event, batchMaxSeq)
          if (!projection.projected) {
            finish(projection.reasonCode)
            return
          }
          acceptedBatch = projection.projected
          acceptedFinalAckBinding = projection.ackBinding
          batchMaxSeq = event.lastSeq
        }
        if (!await ack(batchMaxSeq, acceptedFinalAckBinding)) return
      }).catch(() => finish('accepted_final_observer_failed'))
    })
    const unsubscribeEnd = runtimeApi.onSseEnd((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => {
        if (!exactKeys(payload, ['streamId'])) {
          finish('accepted_final_event_envelope_invalid')
          return
        }
        ended = true
        maybeFinish()
      }).catch(() => finish('accepted_final_observer_failed'))
    })
    const unsubscribeError = runtimeApi.onSseError((payload) => {
      if (payload && payload.streamId === streamId) finish('accepted_final_observer_failed')
    })
    for (const unsubscribe of [unsubscribeEvent, unsubscribeEnd, unsubscribeError]) {
      if (typeof unsubscribe === 'function') unsubscribers.push(unsubscribe)
    }
    if (unsubscribers.length !== 3) finish('accepted_final_observer_failed')
    if (settled) return await outcome
    void Promise.resolve(runtimeApi.startSse(exactThreadId, baselineSeq, streamId)).then((result) => {
      if (!result || result.streamId !== streamId) {
        finish('accepted_final_start_rejected')
        return
      }
      started = true
      maybeFinish()
    }).catch(() => finish('accepted_final_start_failed'))
    return await outcome
  } catch {
    finish('accepted_final_observer_failed')
    return await outcome
  } finally {
    clearTimeout(timer)
    for (const unsubscribe of unsubscribers) {
      try { unsubscribe() } catch { /* best-effort diagnostic cleanup */ }
    }
    try { await runtimeApi.stopSse(streamId) } catch { /* best-effort diagnostic cleanup */ }
  }
}

const PROVIDER_FAILURE_OBSERVATION_CODES = Object.freeze([
  'provider_failure_not_observed',
  'provider_failure_input_invalid',
  'provider_failure_timeout',
  'provider_failure_listener_invalid',
  'provider_failure_start_rejected',
  'provider_failure_start_failed',
  'provider_failure_event_envelope_invalid',
  'provider_failure_event_sequence_invalid',
  'provider_failure_event_scope_invalid',
  'provider_failure_event_schema_invalid',
  'provider_failure_diagnostic_invalid',
  'provider_failure_duplicate',
  'provider_failure_terminal_invalid',
  'provider_failure_terminal_missing',
  'provider_failure_diagnostic_missing',
  'provider_failure_terminal_observed',
  'provider_failure_ack_rejected',
  'provider_failure_ack_failed',
  'provider_failure_stream_error',
  'provider_failure_observer_failed',
  'host_context_budget_observed',
  'provider_failure_observed'
])

function emptyProviderFailureDiagnostic(
  observationCode = 'provider_failure_not_observed'
) {
  return {
    observed: false,
    observationCode: PROVIDER_FAILURE_OBSERVATION_CODES.includes(observationCode)
      ? observationCode
      : 'provider_failure_observer_failed',
    reasonCode: 'none',
    providerKind: 'not_observed',
    providerStatus: null,
    providerRetryable: null,
    providerAuthStatus: 'not_observed',
    diagnosticDigest: ''
  }
}

export function providerFailureDiagnosticEvidence(
  value,
  {
    expectedThreadId = '',
    expectedTurnId = '',
    expectedBaselineSeq = 0,
    expectedHighestSeq = 0
  } = {}
) {
  const empty = (code) => emptyProviderFailureDiagnostic(code)
  const rawKeys = [
    'observed', 'observationCode', 'threadId', 'turnId', 'baselineSeq', 'terminalSeq',
    'highestSeq', 'reasonCode', 'providerKind', 'providerStatus', 'providerRetryable',
    'providerAuthStatus'
  ]
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      !exactObjectKeys(value, rawKeys) ||
      !PROVIDER_FAILURE_OBSERVATION_CODES.includes(value.observationCode)) {
    return empty('provider_failure_diagnostic_invalid')
  }
  if (value.observed !== true) {
    return empty(value.observed === false &&
        value.observationCode !== 'provider_failure_observed' &&
        value.observationCode !== 'provider_failure_terminal_observed' &&
        value.observationCode !== 'host_context_budget_observed'
      ? value.observationCode
      : 'provider_failure_diagnostic_invalid')
  }
  const providerKinds = new Set([
    'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network',
    'http', 'invalid_model', 'unknown'
  ])
  const providerReasonCodes = new Set([
    'provider_error', 'provider_authentication_failed', 'provider_rate_limited',
    'provider_insufficient_balance', 'provider_endpoint_not_found',
    'provider_request_rejected', 'provider_request_failed', 'provider_request_error',
    'provider_unavailable', 'provider_network_unavailable', 'provider_timeout',
    'provider_stream_interrupted', 'provider_model_invalid', 'provider_not_configured',
    'provider_tool_arguments_invalid', 'provider_reasoning_markup_invalid',
    'provider_empty_final', 'provider_stream_failed', 'turn_failed',
    'context_window_hard_limit'
  ])
  const hostContextBudget = value.observationCode === 'host_context_budget_observed'
  const authStatuses = new Set(['none', 'possible', 'required', 'unknown', 'not_observed'])
  const optionalInteger = (candidate, maximum) => candidate === null || (
    Number.isSafeInteger(candidate) && candidate >= 0 && candidate <= maximum
  )
  const optionalBoolean = (candidate) => candidate === null || typeof candidate === 'boolean'
  if ((!hostContextBudget && ![
    'provider_failure_observed',
    'provider_failure_terminal_observed'
  ].includes(value.observationCode)) ||
      (hostContextBudget && value.observationCode !== 'host_context_budget_observed') ||
      typeof expectedThreadId !== 'string' || !expectedThreadId ||
      typeof expectedTurnId !== 'string' || !expectedTurnId ||
      value.threadId !== expectedThreadId || value.turnId !== expectedTurnId ||
      !Number.isSafeInteger(expectedBaselineSeq) || expectedBaselineSeq <= 0 ||
      !Number.isSafeInteger(expectedHighestSeq) ||
      expectedHighestSeq <= expectedBaselineSeq ||
      value.baselineSeq !== expectedBaselineSeq ||
      value.highestSeq !== expectedHighestSeq ||
      !Number.isSafeInteger(value.terminalSeq) ||
      value.terminalSeq <= value.baselineSeq || value.terminalSeq > value.highestSeq ||
      !providerReasonCodes.has(value.reasonCode) ||
      (hostContextBudget
        ? value.reasonCode !== 'context_window_hard_limit' ||
          value.providerKind !== 'host_context_budget' ||
          value.providerStatus !== null || value.providerRetryable !== null ||
          value.providerAuthStatus !== 'not_observed'
        : value.reasonCode === 'context_window_hard_limit' ||
          !providerKinds.has(value.providerKind)) ||
      !optionalInteger(value.providerStatus, 599) ||
      !optionalBoolean(value.providerRetryable) ||
      !authStatuses.has(value.providerAuthStatus)) {
    return empty('provider_failure_diagnostic_invalid')
  }
  const digestInput = {
    threadIdHash: sha256(value.threadId),
    turnIdHash: sha256(value.turnId),
    baselineSeq: value.baselineSeq,
    terminalSeq: value.terminalSeq,
    highestSeq: value.highestSeq,
    reasonCode: value.reasonCode,
    providerKind: value.providerKind,
    providerStatus: value.providerStatus,
    providerRetryable: value.providerRetryable,
    providerAuthStatus: value.providerAuthStatus
  }
  return Object.freeze({
    observed: true,
    observationCode: value.observationCode,
    reasonCode: value.reasonCode,
    providerKind: value.providerKind,
    providerStatus: value.providerStatus,
    providerRetryable: value.providerRetryable,
    providerAuthStatus: value.providerAuthStatus,
    diagnosticDigest: sha256(canonicalJSON(digestInput))
  })
}

export async function observeProviderFailureDiagnosticViaSse(
  runtimeApi,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  priorCompletedTurnIdOrTimeout = '',
  timeoutMs = 5000
) {
  const legacyTimeout = Number.isSafeInteger(priorCompletedTurnIdOrTimeout)
    ? priorCompletedTurnIdOrTimeout
    : null
  const priorCompletedTurnId = typeof priorCompletedTurnIdOrTimeout === 'string'
    ? priorCompletedTurnIdOrTimeout.trim()
    : ''
  if (legacyTimeout !== null) timeoutMs = legacyTimeout
  const observationCodes = new Set([
    'provider_failure_not_observed', 'provider_failure_input_invalid',
    'provider_failure_timeout', 'provider_failure_listener_invalid',
    'provider_failure_start_rejected', 'provider_failure_start_failed',
    'provider_failure_event_envelope_invalid', 'provider_failure_event_sequence_invalid',
    'provider_failure_event_scope_invalid', 'provider_failure_event_schema_invalid',
    'provider_failure_diagnostic_invalid', 'provider_failure_duplicate',
    'provider_failure_terminal_invalid', 'provider_failure_terminal_missing',
    'provider_failure_diagnostic_missing', 'provider_failure_terminal_observed',
    'provider_failure_ack_rejected',
    'provider_failure_ack_failed', 'provider_failure_stream_error',
    'provider_failure_observer_failed', 'host_context_budget_observed',
    'provider_failure_observed'
  ])
  const empty = (observationCode) => ({
    observed: false,
    observationCode: observationCodes.has(observationCode)
      ? observationCode
      : 'provider_failure_observer_failed',
    threadId: '',
    turnId: '',
    baselineSeq: 0,
    terminalSeq: 0,
    highestSeq: 0,
    reasonCode: 'none',
    providerKind: 'not_observed',
    providerStatus: null,
    providerRetryable: null,
    providerAuthStatus: 'not_observed'
  })
  const api = runtimeApi
  const exactThreadId = typeof threadId === 'string' ? threadId.trim() : ''
  const exactTurnId = typeof expectedTurnId === 'string' ? expectedTurnId.trim() : ''
  const exactPriorCompletedTurnId = priorCompletedTurnId
  const safeIdentifier = (value) =>
    typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/u.test(value)
  if (!api || !exactThreadId || exactThreadId !== threadId || exactThreadId.length > 256 ||
      !exactTurnId || exactTurnId !== expectedTurnId || exactTurnId.length > 256 ||
      (priorCompletedTurnIdOrTimeout !== '' && legacyTimeout === null &&
        (typeof priorCompletedTurnIdOrTimeout !== 'string' ||
          exactPriorCompletedTurnId !== priorCompletedTurnIdOrTimeout)) ||
      (exactPriorCompletedTurnId &&
        (!safeIdentifier(exactPriorCompletedTurnId) || exactPriorCompletedTurnId === exactTurnId)) ||
      !Number.isSafeInteger(baselineSeq) || baselineSeq <= 0 ||
      !Number.isSafeInteger(expectedHighestSeq) || expectedHighestSeq <= baselineSeq ||
      !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 60_000 ||
      typeof api.startSse !== 'function' || typeof api.ackSseEvent !== 'function' ||
      typeof api.stopSse !== 'function' || typeof api.onSseEvent !== 'function' ||
      typeof api.onSseEnd !== 'function' || typeof api.onSseError !== 'function') {
    return empty('provider_failure_input_invalid')
  }

  const exactKeys = (value, allowed) => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false
    const keys = Object.keys(value)
    return keys.every((key) => allowed.includes(key))
  }
  const safeProviderCodes = new Set([
    'provider_error', 'provider_authentication_failed', 'provider_rate_limited',
    'provider_insufficient_balance', 'provider_endpoint_not_found',
    'provider_request_rejected', 'provider_request_failed', 'provider_request_error',
    'provider_unavailable', 'provider_network_unavailable', 'provider_timeout',
    'provider_stream_interrupted', 'provider_model_invalid', 'provider_not_configured',
    'provider_tool_arguments_invalid', 'provider_reasoning_markup_invalid',
    'provider_empty_final', 'provider_stream_failed', 'turn_failed',
    'context_window_hard_limit'
  ])
  const terminalOnlyOutputPolicyCodes = new Set([
    'provider_tool_arguments_invalid',
    'provider_reasoning_markup_invalid',
    'provider_empty_final'
  ])
  const providerKinds = new Set([
    'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network',
    'http', 'invalid_model', 'unknown'
  ])
  const authStatuses = new Set(['none', 'required'])
  const endpointFormats = new Set([
    'chat_completions', 'responses', 'messages', 'custom_endpoint'
  ])
  const optionalInteger = (value, maximum) => value === undefined || (
    Number.isSafeInteger(value) && value >= 0 && value <= maximum
  )
  const optionalBoolean = (value) => value === undefined || typeof value === 'boolean'
  const optionalEnum = (value, allowed) => value === undefined || allowed.has(value)
  const canonicalTimestamp = (value) => typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u.test(value)
  const validSseTrace = (value) => value === undefined || (
    exactKeys(value, ['sse_sent_at', 'sse_live_emitted_at']) &&
    Object.keys(value).length === 2 &&
    Number.isFinite(value.sse_sent_at) && value.sse_sent_at >= 0 &&
    Number.isFinite(value.sse_live_emitted_at) && value.sse_live_emitted_at >= 0
  )
  const terminalProviderAttemptFailure = (usage) => {
    if (!usage || typeof usage !== 'object' || Array.isArray(usage) ||
        usage.kind !== 'usage' || usage.usageFinalStatus !== 'failed') return ''
    const diagnostics = usage.cacheDiagnostics
    const statuses = diagnostics?.providerAttemptStatuses
    const statusKeys = ['succeeded', 'failed', 'cancelled', 'timedOut', 'streamAborted']
    if (!diagnostics || typeof diagnostics !== 'object' || Array.isArray(diagnostics) ||
        diagnostics.providerAttemptTelemetrySchema !== 'provider-attempt-telemetry.v1' ||
        diagnostics.providerAttemptTelemetryValid !== true ||
        !Number.isSafeInteger(diagnostics.providerLogicalCallCount) ||
        diagnostics.providerLogicalCallCount <= 0 ||
        !Number.isSafeInteger(diagnostics.providerAttemptCount) ||
        diagnostics.providerAttemptCount <= 0 ||
        diagnostics.providerLogicalCallCount > diagnostics.providerAttemptCount ||
        !exactKeys(statuses, statusKeys) || Object.keys(statuses).length !== statusKeys.length ||
        statusKeys.some((key) => !Number.isSafeInteger(statuses[key]) || statuses[key] < 0)) {
      return ''
    }
    const statusCount = statusKeys.reduce((total, key) => total + statuses[key], 0)
    if (statusCount !== diagnostics.providerAttemptCount ||
        statuses.succeeded > diagnostics.providerLogicalCallCount) return ''
    return statuses.streamAborted === 1 && statuses.failed === 0 &&
      statuses.cancelled === 0 && statuses.timedOut === 0 &&
      statuses.succeeded + statuses.streamAborted === diagnostics.providerAttemptCount
      ? 'provider_stream_interrupted'
      : ''
  }
  const eventKeys = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'trace', 'stage', 'label', 'details'
  ]
  const recoveryDetailKeys = [
    'visibleRecovery', 'recoveryKind', 'recoveryAttempt',
    'maxRecoveryAttempts', 'recoveryExhausted'
  ]
  const detailKeys = ['providerError', 'reasonCode', ...recoveryDetailKeys]
  const providerKeys = [
    'kind', 'endpointFormat', 'authStatus', 'hasApiKey', 'retryable', 'status',
    'retryAfterMs'
  ]
  const projectProviderFailure = (event) => {
    if (exactKeys(event, eventKeys) && event.kind === 'pipeline_stage' &&
        event.stage === 'provider_admission_rejected' &&
        event.label === 'Provider admission rejected' &&
        event.threadId === exactThreadId && event.turnId === exactTurnId &&
        canonicalTimestamp(event.timestamp) &&
        [8, 9].includes(Object.keys(event).length) &&
        validSseTrace(event.trace) && exactKeys(event.details, [
          'reasonCode', 'projectedRequestTokens', 'hardThresholdTokens',
          'providerAttemptCount'
        ]) && event.details.reasonCode === 'context_window_hard_limit' &&
        Number.isSafeInteger(event.details.projectedRequestTokens) &&
        Number.isSafeInteger(event.details.hardThresholdTokens) &&
        Number.isSafeInteger(event.details.providerAttemptCount) &&
        event.details.hardThresholdTokens > 0 &&
        event.details.projectedRequestTokens >= event.details.hardThresholdTokens &&
        event.details.providerAttemptCount === 0) {
      return {
        reasonCode: 'context_window_hard_limit',
        providerKind: 'host_context_budget',
        providerStatus: null,
        providerRetryable: null,
        providerAuthStatus: 'not_observed'
      }
    }
    if (!exactKeys(event, eventKeys) || event.kind !== 'pipeline_stage' ||
        event.stage !== 'provider_error' || event.label !== 'Provider stream failed' ||
        event.threadId !== exactThreadId || event.turnId !== exactTurnId ||
        !canonicalTimestamp(event.timestamp) ||
        ![8, 9].includes(Object.keys(event).length) ||
        !exactKeys(event.details, detailKeys) ||
        !validSseTrace(event.trace)) return null
    const details = event.details
    const reasonCode = details.reasonCode
    const diagnostic = details.providerError
    const recoveryPresent = recoveryDetailKeys.some((key) =>
      Object.prototype.hasOwnProperty.call(details, key))
    if (typeof reasonCode !== 'string' || !safeProviderCodes.has(reasonCode) ||
        reasonCode === 'context_window_hard_limit') return null
    if (recoveryPresent) {
      if (diagnostic !== undefined || reasonCode !== 'provider_empty_final' ||
          Object.keys(details).length !== 6 || details.visibleRecovery !== true ||
          details.recoveryKind !== 'empty_final' || details.recoveryExhausted !== true ||
          !Number.isSafeInteger(details.recoveryAttempt) || details.recoveryAttempt < 0 ||
          !Number.isSafeInteger(details.maxRecoveryAttempts) ||
          details.maxRecoveryAttempts < details.recoveryAttempt ||
          details.maxRecoveryAttempts > 1_000_000) return null
      return {
        reasonCode,
        providerKind: 'unknown',
        providerStatus: null,
        providerRetryable: null,
        providerAuthStatus: 'not_observed'
      }
    }
    if (![1, 2].includes(Object.keys(details).length)) return null
    if (diagnostic === undefined) {
      return {
        reasonCode,
        providerKind: 'unknown',
        providerStatus: null,
        providerRetryable: null,
        providerAuthStatus: 'not_observed'
      }
    }
    if (!exactKeys(diagnostic, providerKeys) || !providerKinds.has(diagnostic.kind) ||
        !optionalEnum(diagnostic.endpointFormat, endpointFormats) ||
        !optionalEnum(diagnostic.authStatus, authStatuses) ||
        !optionalBoolean(diagnostic.hasApiKey) || !optionalBoolean(diagnostic.retryable) ||
        !optionalInteger(diagnostic.status, 599) ||
        !optionalInteger(diagnostic.retryAfterMs, 900_000)) {
      return null
    }
    return {
      reasonCode: typeof reasonCode === 'string' ? reasonCode : '',
      providerKind: diagnostic.kind,
      providerStatus: diagnostic.status ?? null,
      providerRetryable: diagnostic.retryable ?? null,
      providerAuthStatus: diagnostic.authStatus || 'not_observed'
    }
  }
  const acceptedFinalBatchKeys = [
    'schemaVersion', 'purpose', 'kind', 'batchId', 'threadId', 'turnId', 'seq',
    'firstSeq', 'lastSeq', 'timestamp', 'publicationCommitId', 'eventManifestDigest',
    'publicationAuthority', 'events', 'trace'
  ]
  const acceptedFinalAuthorityKeys = [
    'schemaVersion', 'purpose', 'sealId', 'threadId', 'turnId', 'publicationCommitId',
    'acceptedFinalDispositionDigest', 'terminalDispositionId', 'eventManifestDigest',
    'sequencedEventsDigest', 'batchId', 'firstSeq', 'lastSeq', 'timestamp',
    'authorityAlgorithm', 'authorityKeyId', 'authorityPublicKey', 'authoritySignature'
  ]
  const acceptedFinalBatchBaseValid = (batch, cursor, expectedBatchTurnId) => {
    const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
    if (!exactKeys(batch, acceptedFinalBatchKeys) || batch.kind !== 'accepted_final_batch' ||
        batch.schemaVersion !== 2 || batch.purpose !== 'analytix.accepted-final-delivery-batch/v2' ||
        batch.threadId !== exactThreadId || batch.turnId !== expectedBatchTurnId ||
        !safeIdentifier(batch.turnId) ||
        !digest(batch.batchId) || !digest(batch.publicationCommitId) ||
        !digest(batch.eventManifestDigest) || !Number.isSafeInteger(batch.seq) ||
        !Number.isSafeInteger(batch.firstSeq) || !Number.isSafeInteger(batch.lastSeq) ||
        batch.seq !== batch.lastSeq || batch.firstSeq <= 0 || batch.lastSeq <= baselineSeq ||
        batch.firstSeq > cursor + 1 || batch.lastSeq > expectedHighestSeq ||
        !canonicalTimestamp(batch.timestamp) || !validSseTrace(batch.trace) ||
        !Array.isArray(batch.events) || ![3, 4].includes(batch.events.length) ||
        batch.lastSeq - batch.firstSeq + 1 !== batch.events.length ||
        !exactKeys(batch.publicationAuthority, acceptedFinalAuthorityKeys)) return false
    const authority = batch.publicationAuthority
    if (authority.schemaVersion !== 'accepted-final-delivery-seal.v1' ||
        authority.purpose !== 'analytix.accepted-final-delivery-seal/v1' ||
        !digest(authority.sealId) || authority.threadId !== exactThreadId ||
        authority.turnId !== batch.turnId || authority.publicationCommitId !== batch.publicationCommitId ||
        !digest(authority.acceptedFinalDispositionDigest) || !digest(authority.terminalDispositionId) ||
        authority.eventManifestDigest !== batch.eventManifestDigest ||
        !digest(authority.sequencedEventsDigest) || authority.batchId !== batch.batchId ||
        authority.firstSeq !== batch.firstSeq || authority.lastSeq !== batch.lastSeq ||
        authority.timestamp !== batch.timestamp ||
        authority.authorityAlgorithm !== 'Ed25519' || !digest(authority.authorityKeyId) ||
        typeof authority.authorityPublicKey !== 'string' ||
        !/^[A-Za-z0-9_-]{43}$/u.test(authority.authorityPublicKey) ||
        typeof authority.authoritySignature !== 'string' ||
        !/^[A-Za-z0-9_-]{86}$/u.test(authority.authoritySignature)) {
      return false
    }
    return true
  }
  const acceptedFinalEventsIdentityValid = (batch, expectedSlots, expectedKinds) =>
    batch.events.every((nested, index) => {
      const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
      const kind = typeof nested?.kind === 'string' ? nested.kind : ''
      return nested && typeof nested === 'object' && !Array.isArray(nested) &&
        nested.threadId === exactThreadId && nested.turnId === batch.turnId &&
        nested.seq === batch.firstSeq + index && nested.timestamp === batch.timestamp &&
        nested.publicationCommitId === batch.publicationCommitId &&
        nested.acceptedFinalDigest === batch.publicationCommitId &&
        nested.publicationSlot === expectedSlots[index] &&
        digest(nested.publicationEventId) && digest(nested.publicationPayloadDigest) &&
        (expectedKinds[index] === 'terminal' ? kind.startsWith('turn_') : kind === expectedKinds[index])
    })
  const acceptedFinalReplayValid = (batch, cursor) => {
    if (!exactPriorCompletedTurnId ||
        !acceptedFinalBatchBaseValid(batch, cursor, exactPriorCompletedTurnId) ||
        batch.events.length !== 3 ||
        !acceptedFinalEventsIdentityValid(
          batch,
          ['assistant-final', 'usage', 'terminal'],
          ['item_completed', 'usage', 'terminal']
        )) return false
    const assistant = batch.events[0]
    const usage = batch.events[1]
    const terminalEvent = batch.events[2]
    return exactKeys(assistant?.item, [
      'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
      'kind', 'text', 'acceptedFinalView'
    ]) && assistant.item.id === assistant.itemId &&
      assistant.item.threadId === exactThreadId && assistant.item.turnId === batch.turnId &&
      assistant.item.role === 'assistant' && assistant.item.status === 'completed' &&
      assistant.item.kind === 'assistant_text' && assistant.item.finishedAt === batch.timestamp &&
      typeof assistant.item.text === 'string' &&
      acceptedFinalPublicViewV3ShapeValid(
        assistant.item.acceptedFinalView,
        batch.publicationCommitId,
        batch.timestamp
      ) && usage.usageFinalStatus === 'completed' &&
      terminalEvent.kind === 'turn_completed' && terminalEvent.status === 'completed' &&
      terminalEvent.terminalReason === assistant.item.acceptedFinalView.terminalReason
  }
  const projectCaseFailedAcceptedFinal = (batch, cursor) => {
    if (!acceptedFinalBatchBaseValid(batch, cursor, exactTurnId) ||
        batch.events.length !== 4 || batch.firstSeq !== cursor + 1 ||
        batch.lastSeq !== expectedHighestSeq ||
        !acceptedFinalEventsIdentityValid(
          batch,
          ['assistant-final', 'terminal-error-item', 'usage', 'terminal'],
          ['item_completed', 'item_completed', 'usage', 'terminal']
        )) return null
    const [assistantEvent, errorEvent, usageEvent, terminalEvent] = batch.events
    const assistantItem = assistantEvent?.item
    const errorItem = errorEvent?.item
    const terminalItemId = `item_${exactTurnId}_case_terminal`
    const fixedMessage = '案件分析未完成；未经核验的案件事实未发布。'
    const publicationCommitId = batch.publicationCommitId
    const acceptedTerminalReason = assistantItem?.acceptedFinalView?.terminalReason
    const contextAdmissionTerminal = acceptedTerminalReason === 'semantic_failure'
    const expectedTerminalReason = contextAdmissionTerminal
      ? 'semantic_failure'
      : 'provider_failure'
    const expectedTerminalCode = contextAdmissionTerminal
      ? 'case_terminal_semantic_failure'
      : 'case_terminal_provider_failure'
    if (!exactKeys(assistantEvent, [
      'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'item',
      'acceptedFinalDigest', 'publicationCommitId', 'publicationEventId',
      'publicationSlot', 'publicationPayloadDigest'
    ]) || !exactKeys(assistantItem, [
      'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
      'kind', 'text', 'acceptedFinalView'
    ]) || assistantEvent.itemId !== assistantItem?.id ||
        assistantItem?.threadId !== exactThreadId || assistantItem?.turnId !== exactTurnId ||
        assistantItem?.role !== 'assistant' || assistantItem?.status !== 'completed' ||
        assistantItem?.kind !== 'assistant_text' ||
        acceptedTerminalReason !== expectedTerminalReason ||
        assistantItem?.finishedAt !== batch.timestamp ||
        !acceptedFinalPublicViewV3ShapeValid(
          assistantItem?.acceptedFinalView,
          publicationCommitId,
          batch.timestamp
        )) return null
    if (!exactKeys(errorEvent, [
      'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'item',
      'acceptedFinalDigest', 'publicationCommitId', 'publicationEventId',
      'publicationSlot', 'publicationPayloadDigest'
    ]) || !exactKeys(errorItem, [
      'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
      'kind', 'code', 'message', 'severity', 'acceptedFinalDigest'
    ]) || errorEvent.itemId !== terminalItemId || errorItem?.id !== terminalItemId ||
        errorItem?.threadId !== exactThreadId || errorItem?.turnId !== exactTurnId ||
        errorItem?.role !== 'system' || errorItem?.status !== 'failed' ||
        errorItem?.kind !== 'error' || errorItem?.code !== expectedTerminalCode ||
        errorItem?.message !== fixedMessage || errorItem?.severity !== 'error' ||
        errorItem?.acceptedFinalDigest !== publicationCommitId ||
        errorItem?.createdAt !== batch.timestamp || errorItem?.finishedAt !== batch.timestamp) {
      return null
    }
    if (!exactKeys(usageEvent, [
      'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'model', 'providerId',
      'endpointFormat', 'usage', 'cacheDiagnostics', 'usageSource', 'childRunId', 'effort',
      'usageFinalStatus', 'acceptedFinalDigest', 'publicationCommitId',
      'publicationEventId', 'publicationSlot', 'publicationPayloadDigest'
    ]) || usageEvent.usageFinalStatus !== 'failed' ||
        !usageEvent.usage || typeof usageEvent.usage !== 'object' ||
        Array.isArray(usageEvent.usage) || !usageEvent.cacheDiagnostics ||
        typeof usageEvent.cacheDiagnostics !== 'object' ||
        Array.isArray(usageEvent.cacheDiagnostics)) return null
    if (!exactKeys(terminalEvent, [
      'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'status',
      'acceptedFinalDigest', 'terminalReason', 'error', 'message', 'code',
      'itemId', 'publicationCommitId', 'publicationEventId', 'publicationSlot',
      'publicationPayloadDigest'
    ]) || terminalEvent.kind !== 'turn_failed' || terminalEvent.status !== 'failed' ||
        terminalEvent.terminalReason !== expectedTerminalReason ||
        terminalEvent.code !== expectedTerminalCode ||
        terminalEvent.itemId !== terminalItemId || terminalEvent.message !== fixedMessage ||
        terminalEvent.error !== fixedMessage) return null
    return {
      reasonCode: contextAdmissionTerminal
        ? 'context_window_hard_limit'
        : 'case_terminal_provider_failure',
      terminalSeq: batch.lastSeq,
      genericProviderFailure: !contextAdmissionTerminal,
      attemptFailureReasonCode: terminalProviderAttemptFailure(usageEvent)
    }
  }
  const cursorAdvancedValid = (event, cursor) =>
    exactKeys(event, ['kind', 'seq', 'timestamp', 'threadId', 'reason', 'trace']) &&
    event.kind === 'cursor_advanced' && event.threadId === exactThreadId &&
    event.reason === 'restricted_content_removed' && event.seq > cursor &&
    event.seq <= expectedHighestSeq &&
    canonicalTimestamp(event.timestamp) &&
    validSseTrace(event.trace)
  const heartbeatValid = (event, cursor) =>
    exactKeys(event, ['kind', 'seq', 'timestamp', 'threadId', 'trace']) &&
    event.kind === 'heartbeat' && event.threadId === exactThreadId && event.seq === cursor &&
    canonicalTimestamp(event.timestamp) &&
    validSseTrace(event.trace)
  const terminalBatchKeys = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose',
    'batchDigest', 'firstSeq', 'lastSeq', 'generalTerminalCommitId',
    'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest',
    'eventManifestDigest', 'projectedEventsDigest', 'transportAuthority',
    'evidenceAuthority', 'citationAuthority', 'factAnswerAllowed', 'events',
    'eventManifest'
  ]
  const projectTerminalBatch = (batch, lastSeenSeq) => {
    const sha256Hex = (value) =>
      typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
    const timestamp = typeof batch?.timestamp === 'string' ? batch.timestamp : ''
    if (!exactKeys(batch, terminalBatchKeys) || batch.kind !== 'general_terminal_batch' ||
        batch.schemaVersion !== 1 ||
        batch.purpose !== 'analytix.general-terminal-delivery-batch/v1' ||
        batch.threadId !== exactThreadId || batch.turnId !== exactTurnId ||
        !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/u
          .test(timestamp) ||
        !sha256Hex(batch.batchDigest) || !sha256Hex(batch.generalTerminalCommitId) ||
        batch.generalTerminalAuthorityKind !== 'general_terminal_cas' ||
        !sha256Hex(batch.generalTerminalAuthorityDigest) ||
        !sha256Hex(batch.eventManifestDigest) || !sha256Hex(batch.projectedEventsDigest) ||
        batch.transportAuthority !== 'host_batch_digest_v1' ||
        batch.evidenceAuthority !== false || batch.citationAuthority !== false ||
        batch.factAnswerAllowed !== false || !Number.isSafeInteger(batch.seq) ||
        batch.seq <= baselineSeq || batch.seq > expectedHighestSeq ||
        batch.lastSeq !== batch.seq || !Number.isSafeInteger(batch.firstSeq) ||
        batch.firstSeq <= lastSeenSeq || !Array.isArray(batch.events) ||
        ![2, 3].includes(batch.events.length) ||
        batch.lastSeq - batch.firstSeq + 1 !== batch.events.length ||
        !Array.isArray(batch.eventManifest) ||
        batch.eventManifest.length !== batch.events.length) return null
    const expectedSlots = batch.events.length === 3
      ? ['terminal-item', 'usage', 'terminal']
      : ['usage', 'terminal']
    for (let index = 0; index < batch.events.length; index += 1) {
      const nested = batch.events[index]
      const manifest = batch.eventManifest[index]
      if (!nested || typeof nested !== 'object' || Array.isArray(nested) ||
          nested.threadId !== exactThreadId || nested.turnId !== exactTurnId ||
          nested.seq !== batch.firstSeq + index ||
          !exactKeys(manifest, ['slot', 'eventId', 'payloadDigest']) ||
          manifest.slot !== expectedSlots[index] || !sha256Hex(manifest.eventId) ||
          !sha256Hex(manifest.payloadDigest)) return null
    }
    const usage = batch.events.at(-2)
    const terminal = batch.events.at(-1)
    if (!usage || usage.kind !== 'usage' ||
        !terminal || typeof terminal !== 'object' || Array.isArray(terminal) ||
        terminal.kind !== 'turn_failed' || terminal.status !== 'failed' ||
        terminal.threadId !== exactThreadId || terminal.turnId !== exactTurnId ||
        terminal.seq !== batch.lastSeq) return null
    const contextAdmissionTerminal = terminal.terminalReason === 'semantic_failure' &&
      terminal.code === 'context_window_hard_limit'
    if (!contextAdmissionTerminal &&
        (terminal.terminalReason !== 'provider_failure' ||
          terminal.code === 'context_window_hard_limit' ||
          !safeProviderCodes.has(terminal.code))) return null
    return {
      reasonCode: contextAdmissionTerminal ? 'context_window_hard_limit' : terminal.code,
      terminalSeq: batch.seq,
      genericProviderFailure: !contextAdmissionTerminal && terminal.code === 'provider_error',
      attemptFailureReasonCode: terminalProviderAttemptFailure(usage)
    }
  }

  const streamId = `milestone-a-provider-failure-${Math.random().toString(36).slice(2)}`
  const unsubscribers = []
  let started = false
  let settled = false
  let processing = Promise.resolve()
  let eventCount = 0
  let lastSeenSeq = baselineSeq
  let transportCursor = baselineSeq
  let acceptedFinalReplaySeen = false
  let nonHeartbeatEventSeen = false
  let providerFailure = null
  let terminal = null
  let finish
  const outcome = new Promise((resolve) => {
    finish = (observationCode, accepted = false) => {
      if (settled) return
      settled = true
      if (!accepted || !providerFailure || !terminal) {
        resolve(empty(observationCode))
        return
      }
      if (!terminal.genericProviderFailure && providerFailure.reasonCode &&
          providerFailure.reasonCode !== terminal.reasonCode) {
        resolve(empty('provider_failure_diagnostic_invalid'))
        return
      }
      resolve({
        observed: true,
        observationCode: providerFailure.providerKind === 'host_context_budget'
          ? 'host_context_budget_observed'
          : observationCode === 'provider_failure_terminal_observed'
            ? observationCode
            : 'provider_failure_observed',
        threadId: exactThreadId,
        turnId: exactTurnId,
        baselineSeq,
        terminalSeq: terminal.terminalSeq,
        highestSeq: expectedHighestSeq,
        reasonCode: providerFailure.reasonCode || terminal.reasonCode,
        providerKind: providerFailure.providerKind,
        providerStatus: providerFailure.providerStatus,
        providerRetryable: providerFailure.providerRetryable,
        providerAuthStatus: providerFailure.providerAuthStatus
      })
    }
  })
  const timer = setTimeout(() => finish(
    providerFailure && !terminal
      ? 'provider_failure_terminal_missing'
      : 'provider_failure_timeout'
  ), timeoutMs)
  const handlePayload = async (payload) => {
    if (settled) return
    if (!exactKeys(payload, ['streamId', 'events']) || payload.streamId !== streamId ||
        !Array.isArray(payload.events) || payload.events.length < 1 ||
        eventCount + payload.events.length > 4096) {
      finish('provider_failure_event_envelope_invalid')
      return
    }
    if (terminal) {
      finish('provider_failure_event_scope_invalid')
      return
    }
    let batchMaxSeq = transportCursor
    let shouldAck = false
    let acceptedFinalAck = null
    for (const event of payload.events) {
      if (!event || typeof event !== 'object' || Array.isArray(event) ||
          !Number.isSafeInteger(event.seq)) {
        finish('provider_failure_event_sequence_invalid')
        return
      }
      if (event.kind === 'heartbeat') {
        if (!heartbeatValid(event, transportCursor)) {
          finish('provider_failure_event_sequence_invalid')
          return
        }
        eventCount += 1
        continue
      }
      if (event.kind === 'accepted_final_batch') {
        if (event.turnId === exactTurnId) {
          const projected = projectCaseFailedAcceptedFinal(event, lastSeenSeq)
          if (payload.events.length !== 1 || terminal || !projected) {
            finish('provider_failure_event_scope_invalid')
            return
          }
          terminal = projected
          lastSeenSeq = event.lastSeq
          batchMaxSeq = Math.max(batchMaxSeq, event.lastSeq)
          acceptedFinalAck = {
            batchId: event.batchId,
            threadId: event.threadId,
            turnId: event.turnId,
            publicationCommitId: event.publicationCommitId
          }
          nonHeartbeatEventSeen = true
          shouldAck = true
          eventCount += 1
          continue
        }
        if (payload.events.length !== 1 || acceptedFinalReplaySeen ||
            nonHeartbeatEventSeen || !acceptedFinalReplayValid(event, lastSeenSeq)) {
          finish('provider_failure_event_scope_invalid')
          return
        }
        if (event.lastSeq <= lastSeenSeq) {
          finish('provider_failure_event_sequence_invalid')
          return
        }
        lastSeenSeq = event.lastSeq
        batchMaxSeq = Math.max(batchMaxSeq, event.lastSeq)
        acceptedFinalAck = {
          batchId: event.batchId,
          threadId: event.threadId,
          turnId: event.turnId,
          publicationCommitId: event.publicationCommitId
        }
        acceptedFinalReplaySeen = true
        nonHeartbeatEventSeen = true
        shouldAck = true
        eventCount += 1
        continue
      }
      if (event.seq <= lastSeenSeq) {
        finish('provider_failure_event_sequence_invalid')
        return
      }
      if (event.seq > expectedHighestSeq) {
        finish('provider_failure_event_sequence_invalid')
        return
      }
      if (event.kind === 'general_terminal_batch') {
        if (payload.events.length !== 1) {
          finish('provider_failure_event_envelope_invalid')
          return
        }
        terminal = projectTerminalBatch(event, lastSeenSeq)
        if (!terminal) {
          finish('provider_failure_terminal_invalid')
          return
        }
      } else {
        if (event.kind === 'cursor_advanced') {
          if (!cursorAdvancedValid(event, lastSeenSeq)) {
            finish('provider_failure_event_scope_invalid')
            return
          }
        } else if (event.threadId !== exactThreadId ||
            (event.turnId !== undefined && event.turnId !== exactTurnId) ||
            !validSseTrace(event.trace)) {
          finish('provider_failure_event_scope_invalid')
          return
        }
        if (event.kind === 'pipeline_stage' &&
            ['provider_error', 'provider_admission_rejected'].includes(event.stage)) {
          const projected = projectProviderFailure(event)
          if (!projected) {
            finish('provider_failure_diagnostic_invalid')
            return
          }
          if (providerFailure) {
            finish('provider_failure_duplicate')
            return
          }
          providerFailure = projected
        }
      }
      if (event.kind !== 'cursor_advanced') nonHeartbeatEventSeen = true
      lastSeenSeq = event.seq
      batchMaxSeq = event.seq
      shouldAck = true
      eventCount += 1
    }
    if (!shouldAck) return
    let acknowledged
    try {
      acknowledged = acceptedFinalAck
        ? await api.ackSseEvent(streamId, batchMaxSeq, acceptedFinalAck)
        : await api.ackSseEvent(streamId, batchMaxSeq)
    } catch {
      finish('provider_failure_ack_failed')
      return
    }
    if (acknowledged !== true) {
      finish('provider_failure_ack_rejected')
      return
    }
    transportCursor = Math.max(transportCursor, batchMaxSeq)
  }

  try {
    const unsubscribeEvent = api.onSseEvent((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => handlePayload(payload))
        .catch(() => finish('provider_failure_observer_failed'))
    })
    const unsubscribeEnd = api.onSseEnd((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => {
        if (!exactKeys(payload, ['streamId'])) {
          finish('provider_failure_event_envelope_invalid')
        } else if (!terminal) {
          finish('provider_failure_terminal_missing')
        } else if (!started) {
          finish('provider_failure_start_rejected')
        } else if (!providerFailure) {
          const terminalFailureReasonCode = terminalOnlyOutputPolicyCodes.has(
            terminal.reasonCode
          )
            ? terminal.reasonCode
            : terminal.attemptFailureReasonCode
          if (terminalFailureReasonCode) {
            providerFailure = {
              reasonCode: terminalFailureReasonCode,
              providerKind: 'unknown',
              providerStatus: null,
              providerRetryable: null,
              providerAuthStatus: 'not_observed'
            }
            finish('provider_failure_terminal_observed', true)
          } else {
            finish('provider_failure_diagnostic_missing')
          }
        } else {
          finish('provider_failure_observed', true)
        }
      }).catch(() => finish('provider_failure_observer_failed'))
    })
    const unsubscribeError = api.onSseError((payload) => {
      if (payload && payload.streamId === streamId) finish('provider_failure_stream_error')
    })
    for (const unsubscribe of [unsubscribeEvent, unsubscribeEnd, unsubscribeError]) {
      if (typeof unsubscribe === 'function') unsubscribers.push(unsubscribe)
    }
    if (unsubscribers.length !== 3) finish('provider_failure_listener_invalid')
    void Promise.resolve(api.startSse(exactThreadId, baselineSeq, streamId)).then((result) => {
      if (!result || result.streamId !== streamId) {
        finish('provider_failure_start_rejected')
        return
      }
      started = true
    }).catch(() => finish('provider_failure_start_failed'))
    return await outcome
  } catch {
    finish('provider_failure_observer_failed')
    return await outcome
  } finally {
    clearTimeout(timer)
    for (const unsubscribe of unsubscribers) {
      try { unsubscribe() } catch {
        // Best-effort cleanup after the fixed diagnostic has settled.
      }
    }
    try { await api.stopSse(streamId) } catch {
      // Best-effort cleanup after the fixed diagnostic has settled.
    }
  }
}

function providerFailureDiagnosticObservationExpression({
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  priorCompletedTurnId,
  timeoutMs
}) {
  return `(async () => {
    const acceptedFinalPublicViewV3ShapeValid = ${acceptedFinalPublicViewV3ShapeValid.toString()};
    const observeProviderFailureDiagnosticViaSse = ${observeProviderFailureDiagnosticViaSse.toString()};
    return observeProviderFailureDiagnosticViaSse(
      window.analytix?.runtime,
      ${JSON.stringify(threadId)},
      ${JSON.stringify(baselineSeq)},
      ${JSON.stringify(expectedTurnId)},
      ${JSON.stringify(expectedHighestSeq)},
      ${JSON.stringify(priorCompletedTurnId)},
      ${JSON.stringify(timeoutMs)}
    );
  })()`
}

async function observeProviderFailureDiagnostic({
  debugPort,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  priorCompletedTurnId,
  timeoutMs = 20_000
}) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  return evaluateReadonlyCdp(
    target.webSocketDebuggerUrl,
    providerFailureDiagnosticObservationExpression({
      threadId,
      baselineSeq,
      expectedTurnId,
      expectedHighestSeq,
      priorCompletedTurnId,
      timeoutMs: Math.min(timeoutMs, 60_000)
    }),
    timeoutMs
  )
}

const TOOL_FAILURE_OBSERVATION_CODES = Object.freeze([
  'tool_failure_not_observed',
  'tool_failure_input_invalid',
  'tool_failure_timeout',
  'tool_failure_listener_invalid',
  'tool_failure_start_rejected',
  'tool_failure_start_failed',
  'tool_failure_event_envelope_invalid',
  'tool_failure_event_sequence_invalid',
  'tool_failure_event_scope_invalid',
  'tool_failure_guard_invalid',
  'tool_failure_guard_missing',
  'tool_failure_terminal_invalid',
  'tool_failure_terminal_missing',
  'tool_failure_ack_rejected',
  'tool_failure_ack_failed',
  'tool_failure_stream_error',
  'tool_failure_observer_failed',
  'tool_failure_guard_history_observed',
  'tool_failure_observed'
])

const TOOL_FAILURE_EVIDENCE_CODES = Object.freeze([
  'tool_failure_evidence_not_observed',
  'tool_failure_evidence_observer_rejected',
  'tool_failure_evidence_input_invalid',
  'tool_failure_evidence_scope_invalid',
  'tool_failure_evidence_binding_invalid',
  'tool_failure_evidence_guard_history_only',
  'tool_failure_evidence_observed'
])

function emptyToolFailureDiagnostic(
  observationCode = 'tool_failure_not_observed',
  evidenceCode = 'tool_failure_evidence_not_observed'
) {
  return {
    observed: false,
    observationCode: TOOL_FAILURE_OBSERVATION_CODES.includes(observationCode)
      ? observationCode
      : 'tool_failure_observer_failed',
    evidenceCode: TOOL_FAILURE_EVIDENCE_CODES.includes(evidenceCode)
      ? evidenceCode
      : 'tool_failure_evidence_input_invalid',
    toolFailureGuardBound: false,
    guardCount: 0,
    maxStormCount: 0,
    guardKind: 'none',
    toolCategory: 'none',
    toolNameHash: '',
    invalidArgumentGuardCount: 0,
    invalidArgumentMaxStormCount: 0,
    invalidArgumentToolCategory: 'none',
    invalidArgumentToolNameHash: '',
    terminalReason: 'none',
    terminalCode: 'none',
    diagnosticDigest: ''
  }
}

export function toolFailureDiagnosticEvidence(
  value,
  {
    expectedThreadId = '',
    expectedTurnId = '',
    expectedBaselineSeq = 0,
    expectedHighestSeq = 0
  } = {}
) {
  const empty = (observationCode, evidenceCode) =>
    emptyToolFailureDiagnostic(observationCode, evidenceCode)
  const rawKeys = [
    'observed', 'observationCode', 'threadId', 'turnId', 'baselineSeq', 'terminalSeq',
    'highestSeq', 'toolFailureGuardBound', 'guardCount', 'maxStormCount', 'guardKind',
    'toolCategory', 'toolNameHash', 'invalidArgumentGuardCount',
    'invalidArgumentMaxStormCount', 'invalidArgumentToolCategory',
    'invalidArgumentToolNameHash', 'terminalReason', 'terminalCode'
  ]
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      !exactObjectKeys(value, rawKeys) ||
      !TOOL_FAILURE_OBSERVATION_CODES.includes(value.observationCode)) {
    return empty('tool_failure_observer_failed', 'tool_failure_evidence_input_invalid')
  }
  if (value.observed !== true) {
    if (value.observed !== false || [
      'tool_failure_observed', 'tool_failure_guard_history_observed'
    ].includes(value.observationCode)) {
      return empty(value.observationCode, 'tool_failure_evidence_binding_invalid')
    }
    return empty(
      value.observationCode,
      value.observationCode === 'tool_failure_not_observed'
        ? 'tool_failure_evidence_not_observed'
        : 'tool_failure_evidence_observer_rejected'
    )
  }
  if (!['tool_failure_observed', 'tool_failure_guard_history_observed'].includes(
    value.observationCode
  ) ||
      typeof expectedThreadId !== 'string' || !expectedThreadId ||
      typeof expectedTurnId !== 'string' || !expectedTurnId ||
      !Number.isSafeInteger(expectedBaselineSeq) || expectedBaselineSeq <= 0 ||
      !Number.isSafeInteger(expectedHighestSeq) || expectedHighestSeq <= expectedBaselineSeq) {
    return empty(value.observationCode, 'tool_failure_evidence_input_invalid')
  }
  if (value.threadId !== expectedThreadId || value.turnId !== expectedTurnId ||
      value.baselineSeq !== expectedBaselineSeq || value.highestSeq !== expectedHighestSeq ||
      !Number.isSafeInteger(value.terminalSeq) || value.terminalSeq <= value.baselineSeq ||
      value.terminalSeq > value.highestSeq) {
    return empty(value.observationCode, 'tool_failure_evidence_scope_invalid')
  }
  const invalidArgumentHistoryValid =
    Number.isSafeInteger(value.invalidArgumentGuardCount) &&
    value.invalidArgumentGuardCount >= 0 && value.invalidArgumentGuardCount <= 128 &&
    Number.isSafeInteger(value.invalidArgumentMaxStormCount) &&
    value.invalidArgumentMaxStormCount >= 0 && value.invalidArgumentMaxStormCount <= 1_000_000 &&
    (value.invalidArgumentGuardCount === 0
      ? value.invalidArgumentMaxStormCount === 0 &&
        value.invalidArgumentToolCategory === 'none' &&
        value.invalidArgumentToolNameHash === ''
      : value.invalidArgumentMaxStormCount > 0 &&
        WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES.includes(value.invalidArgumentToolCategory) &&
        /^[0-9a-f]{64}$/u.test(value.invalidArgumentToolNameHash))
  const toolFailureBindingValid = value.observationCode === 'tool_failure_observed' &&
    value.toolFailureGuardBound === true &&
    Number.isSafeInteger(value.guardCount) && value.guardCount > 0 &&
    value.guardCount <= 128 &&
    Number.isSafeInteger(value.maxStormCount) && value.maxStormCount > 0 &&
    value.maxStormCount <= 1_000_000 && value.guardKind === 'tool_failure' &&
    WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES.includes(value.toolCategory) &&
    /^[0-9a-f]{64}$/u.test(value.toolNameHash)
  const invalidArgumentHistoryOnlyValid =
    value.observationCode === 'tool_failure_guard_history_observed' &&
    value.toolFailureGuardBound === false && value.guardCount === 0 &&
    value.maxStormCount === 0 && value.guardKind === 'none' &&
    value.toolCategory === 'none' && value.toolNameHash === '' &&
    value.invalidArgumentGuardCount > 0
  if (!invalidArgumentHistoryValid ||
      (!toolFailureBindingValid && !invalidArgumentHistoryOnlyValid) ||
      value.terminalReason !== 'tool_failure' || ![
        'tool_failure_storm', 'tool_invalid_arguments_storm'
      ].includes(value.terminalCode)) {
    return empty(value.observationCode, 'tool_failure_evidence_binding_invalid')
  }
  const digestInput = {
    threadIdHash: sha256(value.threadId),
    turnIdHash: sha256(value.turnId),
    baselineSeq: value.baselineSeq,
    terminalSeq: value.terminalSeq,
    highestSeq: value.highestSeq,
    toolFailureGuardBound: value.toolFailureGuardBound,
    guardCount: value.guardCount,
    maxStormCount: value.maxStormCount,
    guardKind: value.guardKind,
    toolCategory: value.toolCategory,
    toolNameHash: value.toolNameHash,
    invalidArgumentGuardCount: value.invalidArgumentGuardCount,
    invalidArgumentMaxStormCount: value.invalidArgumentMaxStormCount,
    invalidArgumentToolCategory: value.invalidArgumentToolCategory,
    invalidArgumentToolNameHash: value.invalidArgumentToolNameHash,
    terminalReason: value.terminalReason,
    terminalCode: value.terminalCode
  }
  return Object.freeze({
    observed: value.toolFailureGuardBound,
    observationCode: value.observationCode,
    evidenceCode: value.toolFailureGuardBound
      ? 'tool_failure_evidence_observed'
      : 'tool_failure_evidence_guard_history_only',
    toolFailureGuardBound: value.toolFailureGuardBound,
    guardCount: value.guardCount,
    maxStormCount: value.maxStormCount,
    guardKind: value.guardKind,
    toolCategory: value.toolCategory,
    toolNameHash: value.toolNameHash,
    invalidArgumentGuardCount: value.invalidArgumentGuardCount,
    invalidArgumentMaxStormCount: value.invalidArgumentMaxStormCount,
    invalidArgumentToolCategory: value.invalidArgumentToolCategory,
    invalidArgumentToolNameHash: value.invalidArgumentToolNameHash,
    terminalReason: value.terminalReason,
    terminalCode: value.terminalCode,
    diagnosticDigest: sha256(canonicalJSON(digestInput))
  })
}

export async function observeToolFailureDiagnosticViaSse(
  runtimeApi,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  timeoutMs = 5000
) {
  const codes = new Set(TOOL_FAILURE_OBSERVATION_CODES)
  const empty = (observationCode) => ({
    observed: false,
    observationCode: codes.has(observationCode)
      ? observationCode
      : 'tool_failure_observer_failed',
    threadId: '', turnId: '', baselineSeq: 0, terminalSeq: 0, highestSeq: 0,
    toolFailureGuardBound: false,
    guardCount: 0, maxStormCount: 0, guardKind: 'none', toolCategory: 'none',
    toolNameHash: '', invalidArgumentGuardCount: 0, invalidArgumentMaxStormCount: 0,
    invalidArgumentToolCategory: 'none', invalidArgumentToolNameHash: '',
    terminalReason: 'none', terminalCode: 'none'
  })
  const api = runtimeApi
  const exactThreadId = typeof threadId === 'string' ? threadId.trim() : ''
  const exactTurnId = typeof expectedTurnId === 'string' ? expectedTurnId.trim() : ''
  if (!api || !exactThreadId || exactThreadId !== threadId || exactThreadId.length > 256 ||
      !exactTurnId || exactTurnId !== expectedTurnId || exactTurnId.length > 256 ||
      !Number.isSafeInteger(baselineSeq) || baselineSeq <= 0 ||
      !Number.isSafeInteger(expectedHighestSeq) || expectedHighestSeq <= baselineSeq ||
      !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0 || timeoutMs > 60_000 ||
      typeof api.startSse !== 'function' || typeof api.ackSseEvent !== 'function' ||
      typeof api.stopSse !== 'function' || typeof api.onSseEvent !== 'function' ||
      typeof api.onSseEnd !== 'function' || typeof api.onSseError !== 'function') {
    return empty('tool_failure_input_invalid')
  }
  const exactKeys = (value, allowed) => value && typeof value === 'object' &&
    !Array.isArray(value) && Object.keys(value).every((key) => allowed.includes(key))
  const sha256Hex = async (value) => Array.from(new Uint8Array(
    await globalThis.crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  )).map((byte) => byte.toString(16).padStart(2, '0')).join('')
  const toolCategory = (name) => {
    if (name === 'read') return 'read'
    if (name === 'create_plan' || name === 'update_plan') return 'plan'
    if (name === 'todo_ops' || name === 'todo_write') return 'todo'
    if (['write', 'write_file', 'edit', 'edit_file', 'multi_edit', 'apply_patch'].includes(
      name
    )) return 'write'
    if (name === 'bash') return 'bash'
    if (['task', 'delegate_task', 'parallel_tasks'].includes(name)) return 'subagent'
    if (name === 'run_skill') return 'skill'
    if (name === 'analyze_account_flows' || name.startsWith('mcp__analytix_funds__') ||
        name.startsWith('mcp__analytix-fund-analysis__')) return 'fundsMcp'
    if (name.startsWith('mcp__')) return 'ordinaryMcp'
    return 'other'
  }
  const eventKeys = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'itemId', 'trace', 'stage',
    'label', 'message', 'attempt', 'maxAttempt', 'child', 'details'
  ]
  const detailKeys = [
    'visibleRecovery', 'recoveryKind', 'recoveryAttempt', 'maxRecoveryAttempt',
    'maxRecoveryAttempts', 'recoveryExhausted', 'partialToolStarted', 'maxModelSteps',
    'toolName', 'guardKind', 'stormCount', 'providerError', 'reasonCode'
  ]
  const terminalBatchKeys = [
    'kind', 'seq', 'timestamp', 'threadId', 'turnId', 'schemaVersion', 'purpose',
    'batchDigest', 'firstSeq', 'lastSeq', 'generalTerminalCommitId',
    'generalTerminalAuthorityKind', 'generalTerminalAuthorityDigest',
    'eventManifestDigest', 'projectedEventsDigest', 'transportAuthority',
    'evidenceAuthority', 'citationAuthority', 'factAnswerAllowed', 'events', 'eventManifest'
  ]
  const projectTerminal = (batch, lastSeenSeq) => {
    const digest = (value) => typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
    if (!exactKeys(batch, terminalBatchKeys) || batch.kind !== 'general_terminal_batch' ||
        batch.schemaVersion !== 1 ||
        batch.purpose !== 'analytix.general-terminal-delivery-batch/v1' ||
        batch.threadId !== exactThreadId || batch.turnId !== exactTurnId ||
        !digest(batch.batchDigest) || !digest(batch.generalTerminalCommitId) ||
        batch.generalTerminalAuthorityKind !== 'general_terminal_cas' ||
        !digest(batch.generalTerminalAuthorityDigest) || !digest(batch.eventManifestDigest) ||
        !digest(batch.projectedEventsDigest) || batch.transportAuthority !== 'host_batch_digest_v1' ||
        batch.evidenceAuthority !== false || batch.citationAuthority !== false ||
        batch.factAnswerAllowed !== false || !Number.isSafeInteger(batch.seq) ||
        batch.seq <= lastSeenSeq || batch.seq > expectedHighestSeq || batch.lastSeq !== batch.seq ||
        !Number.isSafeInteger(batch.firstSeq) || batch.firstSeq <= lastSeenSeq ||
        !Array.isArray(batch.events) || batch.events.length !== 3 ||
        batch.lastSeq - batch.firstSeq + 1 !== 3 || !Array.isArray(batch.eventManifest) ||
        batch.eventManifest.length !== 3) return null
    const slots = ['terminal-item', 'usage', 'terminal']
    for (let index = 0; index < 3; index += 1) {
      const event = batch.events[index]
      const manifest = batch.eventManifest[index]
      if (!event || typeof event !== 'object' || Array.isArray(event) ||
          event.threadId !== exactThreadId || event.turnId !== exactTurnId ||
          event.seq !== batch.firstSeq + index ||
          !exactKeys(manifest, ['slot', 'eventId', 'payloadDigest']) ||
          manifest.slot !== slots[index] || !digest(manifest.eventId) ||
          !digest(manifest.payloadDigest)) return null
    }
    const item = batch.events[0]
    const usage = batch.events[1]
    const terminal = batch.events[2]
    if (item.kind !== 'item_completed' || usage.kind !== 'usage' ||
        terminal.kind !== 'turn_failed' || terminal.status !== 'failed' ||
        terminal.terminalReason !== 'tool_failure' || ![
          'tool_failure_storm', 'tool_invalid_arguments_storm'
        ].includes(terminal.code)) {
      return null
    }
    return { terminalSeq: batch.seq, terminalReason: terminal.terminalReason, terminalCode: terminal.code }
  }
  const streamId = `milestone-a-tool-failure-${Math.random().toString(36).slice(2)}`
  const unsubscribers = []
  let started = false
  let settled = false
  let processing = Promise.resolve()
  let lastSeenSeq = baselineSeq
  let eventCount = 0
  let terminal = null
  const guards = []
  let finish
  const outcome = new Promise((resolve) => {
    finish = (observationCode, accepted = false) => {
      if (settled) return
      settled = true
      if (!accepted || !terminal || guards.length === 0) {
        resolve(empty(observationCode))
        return
      }
      const toolFailureGuards = guards.filter((guard) => guard.guardKind === 'tool_failure')
      const invalidArgumentGuards = guards.filter((guard) =>
        guard.guardKind === 'invalid_tool_arguments'
      )
      const lastIdentitySuffix = (records) => {
        const last = records.at(-1)
        if (!last) return []
        const suffix = [last]
        for (let index = records.length - 2; index >= 0; index -= 1) {
          const guard = records[index]
          const next = suffix[0]
          if (guard.toolCategory !== last.toolCategory ||
              guard.toolNameHash !== last.toolNameHash ||
              guard.stormCount + 1 !== next.stormCount) break
          suffix.unshift(guard)
        }
        return suffix
      }
      const invalidArgumentSuffix = lastIdentitySuffix(invalidArgumentGuards)
      const lastInvalidArgument = invalidArgumentSuffix.at(-1)
      const invalidArgumentHistory = {
        invalidArgumentGuardCount: invalidArgumentSuffix.length,
        invalidArgumentMaxStormCount: invalidArgumentSuffix.length === 0
          ? 0
          : Math.max(...invalidArgumentSuffix.map((guard) => guard.stormCount)),
        invalidArgumentToolCategory: lastInvalidArgument?.toolCategory || 'none',
        invalidArgumentToolNameHash: lastInvalidArgument?.toolNameHash || ''
      }
      if (toolFailureGuards.length === 0) {
        if (invalidArgumentGuards.length === 0) {
          resolve(empty('tool_failure_guard_missing'))
          return
        }
        resolve({
          observed: true,
          observationCode: 'tool_failure_guard_history_observed',
          threadId: exactThreadId,
          turnId: exactTurnId,
          baselineSeq,
          terminalSeq: terminal.terminalSeq,
          highestSeq: expectedHighestSeq,
          toolFailureGuardBound: false,
          guardCount: 0,
          maxStormCount: 0,
          guardKind: 'none',
          toolCategory: 'none',
          toolNameHash: '',
          ...invalidArgumentHistory,
          terminalReason: terminal.terminalReason,
          terminalCode: terminal.terminalCode
        })
        return
      }
      const last = toolFailureGuards.at(-1)
      const suffix = lastIdentitySuffix(toolFailureGuards)
      resolve({
        observed: true, observationCode: 'tool_failure_observed', threadId: exactThreadId,
        turnId: exactTurnId, baselineSeq, terminalSeq: terminal.terminalSeq,
        highestSeq: expectedHighestSeq, toolFailureGuardBound: true,
        guardCount: suffix.length,
        maxStormCount: Math.max(...suffix.map((guard) => guard.stormCount)),
        guardKind: last.guardKind, toolCategory: last.toolCategory,
        toolNameHash: last.toolNameHash, ...invalidArgumentHistory,
        terminalReason: terminal.terminalReason,
        terminalCode: terminal.terminalCode
      })
    }
  })
  const timer = setTimeout(() => finish('tool_failure_timeout'), timeoutMs)
  const handlePayload = async (payload) => {
    if (settled) return
    if (!exactKeys(payload, ['streamId', 'events']) || payload.streamId !== streamId ||
        !Array.isArray(payload.events) || payload.events.length < 1 ||
        payload.events.length > 128 || eventCount + payload.events.length > 4096) {
      finish('tool_failure_event_envelope_invalid')
      return
    }
    if (terminal || payload.events.some((event) => event?.kind === 'accepted_final_batch')) {
      finish('tool_failure_event_scope_invalid')
      return
    }
    let batchMaxSeq = lastSeenSeq
    for (const event of payload.events) {
      if (!event || typeof event !== 'object' || Array.isArray(event) ||
          !Number.isSafeInteger(event.seq) || event.seq <= lastSeenSeq ||
          event.seq > expectedHighestSeq) {
        finish('tool_failure_event_sequence_invalid')
        return
      }
      if (event.kind === 'general_terminal_batch') {
        if (payload.events.length !== 1) {
          finish('tool_failure_event_envelope_invalid')
          return
        }
        terminal = projectTerminal(event, lastSeenSeq)
        if (!terminal) {
          finish('tool_failure_terminal_invalid')
          return
        }
      } else {
        if (event.threadId !== exactThreadId ||
            (event.turnId !== undefined && event.turnId !== exactTurnId)) {
          finish('tool_failure_event_scope_invalid')
          return
        }
        if (event.kind === 'pipeline_stage' && event.stage === 'loop_guard') {
          const details = event.details
          const name = details?.toolName
          if (!exactKeys(event, eventKeys) || ![
            'Loop guard',
            'Loop guard nudged the model'
          ].includes(event.label) ||
              event.message !== undefined || event.child !== undefined ||
              event.attempt !== undefined || event.maxAttempt !== undefined ||
              !exactKeys(details, detailKeys) || details.visibleRecovery !== true ||
              !['tool_failure', 'invalid_tool_arguments'].includes(details.guardKind) ||
              !Number.isSafeInteger(details.stormCount) || details.stormCount <= 0 ||
              details.stormCount > 1_000_000 || typeof name !== 'string' ||
              !/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/u.test(name)) {
            finish('tool_failure_guard_invalid')
            return
          }
          guards.push({
            guardKind: details.guardKind,
            stormCount: details.stormCount,
            toolCategory: toolCategory(name),
            toolNameHash: await sha256Hex(name)
          })
        }
      }
      lastSeenSeq = event.seq
      batchMaxSeq = event.seq
      eventCount += 1
    }
    let acknowledged
    try { acknowledged = await api.ackSseEvent(streamId, batchMaxSeq) } catch {
      finish('tool_failure_ack_failed')
      return
    }
    if (acknowledged !== true) finish('tool_failure_ack_rejected')
  }
  try {
    const unsubscribeEvent = api.onSseEvent((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => handlePayload(payload))
        .catch(() => finish('tool_failure_observer_failed'))
    })
    const unsubscribeEnd = api.onSseEnd((payload) => {
      if (!payload || payload.streamId !== streamId) return
      processing = processing.then(() => {
        if (!exactKeys(payload, ['streamId'])) finish('tool_failure_event_envelope_invalid')
        else if (!terminal) finish('tool_failure_terminal_missing')
        else if (guards.length === 0) finish('tool_failure_guard_missing')
        else if (!started) finish('tool_failure_start_rejected')
        else finish('tool_failure_observed', true)
      }).catch(() => finish('tool_failure_observer_failed'))
    })
    const unsubscribeError = api.onSseError((payload) => {
      if (payload && payload.streamId === streamId) finish('tool_failure_stream_error')
    })
    for (const unsubscribe of [unsubscribeEvent, unsubscribeEnd, unsubscribeError]) {
      if (typeof unsubscribe === 'function') unsubscribers.push(unsubscribe)
    }
    if (unsubscribers.length !== 3) finish('tool_failure_listener_invalid')
    void Promise.resolve(api.startSse(exactThreadId, baselineSeq, streamId)).then((result) => {
      if (!result || result.streamId !== streamId) finish('tool_failure_start_rejected')
      else started = true
    }).catch(() => finish('tool_failure_start_failed'))
    return await outcome
  } catch {
    finish('tool_failure_observer_failed')
    return await outcome
  } finally {
    clearTimeout(timer)
    for (const unsubscribe of unsubscribers) {
      try { unsubscribe() } catch { /* best-effort diagnostic cleanup */ }
    }
    try { await api.stopSse(streamId) } catch { /* best-effort diagnostic cleanup */ }
  }
}

function toolFailureDiagnosticObservationExpression({
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  timeoutMs
}) {
  return `(async () => {
    const TOOL_FAILURE_OBSERVATION_CODES = ${JSON.stringify(TOOL_FAILURE_OBSERVATION_CODES)};
    const observeToolFailureDiagnosticViaSse = ${observeToolFailureDiagnosticViaSse.toString()};
    return observeToolFailureDiagnosticViaSse(
      window.analytix?.runtime,
      ${JSON.stringify(threadId)},
      ${JSON.stringify(baselineSeq)},
      ${JSON.stringify(expectedTurnId)},
      ${JSON.stringify(expectedHighestSeq)},
      ${JSON.stringify(timeoutMs)}
    );
  })()`
}

async function observeToolFailureDiagnostic({
  debugPort,
  threadId,
  baselineSeq,
  expectedTurnId,
  expectedHighestSeq,
  timeoutMs = 20_000
}) {
  const target = await waitForDebugTarget(debugPort, timeoutMs)
  return evaluateReadonlyCdp(
    target.webSocketDebuggerUrl,
    toolFailureDiagnosticObservationExpression({
      threadId,
      baselineSeq,
      expectedTurnId,
      expectedHighestSeq,
      timeoutMs: Math.min(timeoutMs, 60_000)
    }),
    timeoutMs
  )
}

export function readonlyThreadWorkspaceScopeMatches(thread, expectedThreadId, workspace) {
  const exactThreadId = typeof expectedThreadId === 'string' &&
    expectedThreadId.length > 0 && expectedThreadId === expectedThreadId.trim()
    ? expectedThreadId
    : ''
  const exactWorkspace = typeof workspace === 'string' &&
    workspace.length > 0 && workspace === workspace.trim()
    ? workspace
    : ''
  if (!exactThreadId || !exactWorkspace || !thread || typeof thread !== 'object' ||
      Array.isArray(thread) || thread.id !== exactThreadId) return false
  const hasWorkspace = Object.prototype.hasOwnProperty.call(thread, 'workspace')
  if (thread.historyAuthority === 'case_boundary_only_v1') {
    return !hasWorkspace
  }
  return hasWorkspace && thread.workspace === exactWorkspace
}

export function readonlyObservationExpression(
  workspace,
  exactThreadId = '',
  providerReceiptScope = null,
  acceptedFinalReceiptScope = null
) {
  return `(async () => {
    const PROVIDER_RECEIPT_REASON_CODES = ${JSON.stringify(PROVIDER_RECEIPT_REASON_CODES)};
    const observeProviderTerminalViaSse = ${observeProviderTerminalViaSse.toString()};
    const acceptedFinalPublicViewV3ShapeValid = ${acceptedFinalPublicViewV3ShapeValid.toString()};
    const observeAcceptedFinalOrdinaryViaSse = ${observeAcceptedFinalOrdinaryViaSse.toString()};
    const readonlyThreadWorkspaceScopeMatches = ${readonlyThreadWorkspaceScopeMatches.toString()};
    const responseBody = (response) => {
      try {
        if (!response || response.status < 200 || response.status >= 300) return null;
        return JSON.parse(response.body || '{}');
      } catch { return null; }
    };
    const emptyResult = (apiPresent, title = '') => ({
      apiPresent,
      title,
      composerPresent: false,
      primaryButtonPresent: false,
      providerRegistry: null,
      health: null,
      runtimeInfo: null,
      runtimeTools: null,
      runtimeSkills: null,
      runtimeThreadListProbeOk: false,
      runtimeThreadListProbeStatus: 0,
      settings: null,
      thread: null,
      summary: null,
      providerAttempts: [],
      providerTerminals: [],
      ordinaryResultReceipt: null,
      providerReceiptTrace: null
    });
    let api;
    try { api = window.analytix; } catch {
      return { readErrorStage: 'api_access' };
    }
    const result = {
      apiPresent: !!api,
      title: '',
      composerPresent: false,
      primaryButtonPresent: false,
      providerRegistry: null,
      health: null,
      runtimeInfo: null,
      runtimeTools: null,
      runtimeSkills: null,
      runtimeThreadListProbeOk: false,
      runtimeThreadListProbeStatus: 0,
      settings: null,
      thread: null,
      summary: null,
      providerAttempts: [],
      providerTerminals: [],
      ordinaryResultReceipt: null,
      providerReceiptTrace: null
    };
    try { result.title = document.title; } catch {
      return { ...emptyResult(result.apiPresent), readErrorStage: 'document_title' };
    }
    try {
      result.composerPresent = !!document.querySelector('.ProseMirror[contenteditable="true"]');
    } catch {
      return { ...result, readErrorStage: 'composer_query' };
    }
    try {
      result.primaryButtonPresent = !!document.querySelector('.ds-composer-primary-action-button');
    } catch {
      return { ...result, readErrorStage: 'primary_button_query' };
    }
    if (!api) return result;
    try {
      if (api.providerRegistry && typeof api.providerRegistry.request === 'function') {
        result.providerRegistry = await api.providerRegistry.request({ schemaVersion: 1, operation: 'list' });
      }
    } catch {
      return { ...result, readErrorStage: 'api_access' };
    }
    try {
      if (api.settings && typeof api.settings.getSettings === 'function') {
        try { result.settings = await api.settings.getSettings(); } catch {}
      }
    } catch {
      return { ...result, readErrorStage: 'api_access' };
    }
    try {
      if (!api.runtime || typeof api.runtime.runtimeRequest !== 'function') return result;
    } catch {
      return { ...result, readErrorStage: 'api_access' };
    }
    const runtimeJSON = async (path) => {
      const requiredStage = path.startsWith('/v1/threads/') &&
        !path.endsWith('/summary') ? 'thread_detail' : '';
      const requestedThreadId = requiredStage
        ? decodeURIComponent(path.slice('/v1/threads/'.length))
        : '';
      try {
        const response = await api.runtime.runtimeRequest(path, 'GET');
        const value = responseBody(response);
        if (requiredStage && (!value || typeof value !== 'object' ||
            Array.isArray(value) || value.id !== requestedThreadId)) {
          result.readErrorStage = requiredStage;
        }
        return value;
      } catch {
        if (requiredStage) result.readErrorStage = requiredStage;
        return null;
      }
    };
    const requiredRuntimeJSON = async (path, stage) => {
      const requestedThreadId = stage === 'thread_detail' && path.startsWith('/v1/threads/')
        ? decodeURIComponent(path.slice('/v1/threads/'.length))
        : '';
      try {
        const response = await api.runtime.runtimeRequest(path, 'GET');
        if (!response || response.status < 200 || response.status >= 300) {
          result.readErrorStage = stage;
          return null;
        }
        try {
          const value = JSON.parse(response.body || '{}');
          if (stage === 'thread_detail' && (!value || typeof value !== 'object' ||
              Array.isArray(value) || value.id !== requestedThreadId)) {
            result.readErrorStage = stage;
            return null;
          }
          return value;
        } catch {
          result.readErrorStage = stage;
          return null;
        }
      } catch {
        result.readErrorStage = stage;
        return null;
      }
    };
    result.health = await runtimeJSON('/health');
    result.runtimeInfo = await runtimeJSON('/v1/runtime/info');
    result.runtimeTools = await runtimeJSON('/v1/runtime/tools');
    result.runtimeSkills = await runtimeJSON('/v1/skills');
    try {
      const threadListProbe = await api.runtime.runtimeRequest('/v1/threads?limit=1', 'GET');
      const threadListStatus = Number(threadListProbe?.status || 0);
      const threadListBody = responseBody(threadListProbe);
      result.runtimeThreadListProbeStatus = Number.isSafeInteger(threadListStatus) &&
        threadListStatus >= 0 && threadListStatus <= 599
        ? threadListStatus
        : 0;
      result.runtimeThreadListProbeOk = threadListProbe?.ok === true &&
        result.runtimeThreadListProbeStatus >= 200 &&
        result.runtimeThreadListProbeStatus < 300 &&
        Array.isArray(threadListBody?.threads);
    } catch {}
    const exact = ${JSON.stringify(exactThreadId)};
    const workspace = ${JSON.stringify(workspace)};
    const providerReceiptScope = ${JSON.stringify(providerReceiptScope)};
    const acceptedFinalReceiptScope = ${JSON.stringify(acceptedFinalReceiptScope)};
    const exactThread = exact
      ? await runtimeJSON('/v1/threads/' + encodeURIComponent(exact))
      : null;
    if (result.readErrorStage) return result;
    if (exact && exactThread?.id !== exact) return result;
    const list = exact ? null : await runtimeJSON('/v1/threads?include=side');
    const threads = list && Array.isArray(list.threads) ? list.threads : [];
    const selected = exact ? null : threads
      .filter((item) => item && item.workspace === workspace)
      .sort((left, right) => String(right.updatedAt || '').localeCompare(String(left.updatedAt || '')))[0];
    const selectedId = exact || selected?.id || '';
    if (!selectedId) return result;
    result.thread = exactThread || await requiredRuntimeJSON(
      '/v1/threads/' + encodeURIComponent(selectedId),
      'thread_detail'
    );
    if (result.readErrorStage) return result;
    if (!readonlyThreadWorkspaceScopeMatches(result.thread, selectedId, workspace)) return result;
    let summaryResponse;
    try {
      summaryResponse = await api.runtime.runtimeRequest(
        '/v1/threads/' + encodeURIComponent(selectedId) + '/summary',
        'GET'
      );
    } catch {
      return { ...result, readErrorStage: 'thread_summary' };
    }
    result.summary = responseBody(summaryResponse);
    if (!result.summary || typeof result.summary !== 'object' ||
        Array.isArray(result.summary)) {
      return { ...result, readErrorStage: 'thread_summary' };
    }
    if (result.readErrorStage) return result;
    if (acceptedFinalReceiptScope &&
        Number.isSafeInteger(acceptedFinalReceiptScope.baselineSeq) &&
        typeof acceptedFinalReceiptScope.expectedTurnId === 'string' &&
        Number.isSafeInteger(acceptedFinalReceiptScope.expectedHighestSeq)) {
      let providerReplay;
      try {
        providerReplay = await observeAcceptedFinalOrdinaryViaSse(
          api.runtime,
          selectedId,
          acceptedFinalReceiptScope.baselineSeq,
          acceptedFinalReceiptScope.expectedTurnId,
          acceptedFinalReceiptScope.expectedHighestSeq,
          5000
        );
      } catch {
        return { ...result, readErrorStage: 'accepted_final_replay' };
      }
      result.providerAttempts = providerReplay.providerAttempts;
      result.providerTerminals = providerReplay.providerTerminals;
      result.providerReceiptTrace = providerReplay.providerReceiptTrace;
    } else if (providerReceiptScope &&
        Number.isSafeInteger(providerReceiptScope.baselineSeq) &&
        typeof providerReceiptScope.expectedTurnId === 'string' &&
        Number.isSafeInteger(providerReceiptScope.expectedHighestSeq)) {
      let providerReplay;
      try {
        providerReplay = await observeProviderTerminalViaSse(
        api.runtime,
        selectedId,
          providerReceiptScope.baselineSeq,
          providerReceiptScope.expectedTurnId,
          providerReceiptScope.expectedHighestSeq,
          5000
        );
      } catch {
        return { ...result, readErrorStage: 'provider_receipt_replay' };
      }
      result.providerAttempts = providerReplay.providerAttempts;
      result.providerTerminals = providerReplay.providerTerminals;
      result.ordinaryResultReceipt = providerReplay.ordinaryResultReceipt;
      result.providerReceiptTrace = providerReplay.providerReceiptTrace;
    }
    return result;
  })()`
}

async function observeRenderer({
  debugPort,
  workspace,
  exactThreadId = '',
  providerReceiptScope = null,
  acceptedFinalReceiptScope = null,
  timeoutMs = 20_000
}, internalDependencies = {}) {
  const waitForTarget = typeof internalDependencies.waitForDebugTarget === 'function'
    ? internalDependencies.waitForDebugTarget
    : waitForDebugTarget
  const evaluate = typeof internalDependencies.evaluateReadonlyCdp === 'function'
    ? internalDependencies.evaluateReadonlyCdp
    : evaluateReadonlyCdp
  const target = await waitForTarget(debugPort, timeoutMs)
  const observation = await evaluate(
    target.webSocketDebuggerUrl,
    readonlyObservationExpression(
      workspace,
      exactThreadId,
      providerReceiptScope,
      acceptedFinalReceiptScope
    ),
    timeoutMs
  )
  if (observation && typeof observation === 'object' && observation.readErrorStage !== undefined) {
    const expressionStage = safeReadonlyObservationReadErrorStage(observation.readErrorStage)
    const error = readonlyCdpFailureError('readonly_cdp_exception', { expressionStage })
    error.managedWorkbenchReadDiagnostic = managedWorkbenchReadDiagnostic(
      observation,
      target.pageTargetCount
    )
    throw error
  }
  return {
    ...observation,
    rendererTargetCount: target.pageTargetCount
  }
}

export { observeRenderer }

function hostToolExecutionObservationExpression({
  threadId,
  turnId,
  toolName,
  workspace,
  arguments: toolArguments
}) {
  const request = {
    threadId,
    turnId,
    toolName,
    workspace,
    arguments: toolArguments
  }
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

function hostToolExecutionObservationReasonCode(raw, transportStatus, observationValid) {
  const rawShapeValid = Boolean(raw && typeof raw === 'object' && !Array.isArray(raw) &&
    Object.keys(raw).sort().join(',') === 'observation,status')
  if (!rawShapeValid) return 'response_schema_invalid'
  if (transportStatus === 0) return 'transport_unavailable'
  if (transportStatus === 404) return 'not_observed'
  if (transportStatus !== 200) return 'http_error'
  return observationValid ? 'observed' : 'response_schema_invalid'
}

export function hostToolExecutionObservationEvidence(raw, expected, attemptCount = 1) {
  const transportStatus = Number.isSafeInteger(raw?.status) ? raw.status : 0
  const safeAttemptCount = Number.isSafeInteger(attemptCount) && attemptCount >= 0
    ? Math.min(attemptCount, HOST_TOOL_EXECUTION_MAX_404_RETRIES + 1)
    : 0
  const observation = raw?.observation
  const expectedToolName = String(expected?.toolName || '')
  const observationValid = Boolean(
    raw && typeof raw === 'object' && !Array.isArray(raw) &&
    Object.keys(raw).sort().join(',') === 'observation,status' &&
    typeof expected?.threadId === 'string' && expected.threadId &&
    expected.threadId === expected.threadId.trim() &&
    typeof expected?.turnId === 'string' && expected.turnId &&
    expected.turnId === expected.turnId.trim() &&
    transportStatus === 200 && observation && typeof observation === 'object' &&
    !Array.isArray(observation) &&
    Object.keys(observation).sort().join(',') === TOOL_EXECUTION_OBSERVATION_FIELDS.join(',') &&
    observation.schemaVersion === 1 &&
    observation.disclosure === 'metadata_only' &&
    observation.privatePayloadWithheld === true &&
    observation.threadId === expected?.threadId &&
    observation.turnId === expected?.turnId &&
    observation.toolName === expectedToolName &&
    (expectedToolName === 'bash' || expectedToolName === 'read') &&
    observation.status === 'completed' &&
    [observation.workId, observation.receiptId, observation.dispositionId,
      observation.executionGrantId, observation.resultItemDigest]
      .every((value) => /^[0-9a-f]{64}$/.test(String(value || ''))) &&
    /^item_result_[0-9a-f]{64}$/.test(String(observation.resultItemId || ''))
  )
  const base = {
    ok: false,
    blocker: 'host_owned_tool_invocation_binding_unavailable',
    reasonCode: hostToolExecutionObservationReasonCode(
      raw,
      transportStatus,
      observationValid
    ),
    transportStatus,
    attemptCount: safeAttemptCount,
    toolName: '',
    disclosure: '',
    privatePayloadWithheld: false,
    observationDigest: '',
    authorityBindingDigest: ''
  }
  if (!observationValid) return Object.freeze(base)
  const evidence = Object.freeze({
    ...base,
    ok: true,
    blocker: '',
    toolName: expectedToolName,
    disclosure: observation.disclosure,
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
  return evidence
}

function hostToolArgumentAuthorityKey(workspace, toolName, canonicalArguments) {
  return sha256(canonicalJSON({ canonicalArguments, toolName, workspace }))
}

function canonicalHostToolExecutionArguments(toolName, toolArguments, workspace) {
  if (!toolArguments || typeof toolArguments !== 'object' || Array.isArray(toolArguments)) {
    return ''
  }
  const keys = Object.keys(toolArguments).sort()
  if (toolName === 'bash' && keys.join(',') === 'command' &&
    typeof toolArguments.command === 'string') {
    const canonical = canonicalJSON({ command: toolArguments.command })
    if (externalHostToolExecutionArguments.has(hostToolArgumentAuthorityKey(
      workspace,
      'bash',
      canonical
    ))) return canonical
  }
  if (toolName === 'read' && keys.join(',') === 'path' &&
    typeof toolArguments.path === 'string') {
    const canonical = canonicalJSON({ path: toolArguments.path })
    if (externalHostToolExecutionArguments.has(hostToolArgumentAuthorityKey(
      workspace,
      'read',
      canonical
    ))) return canonical
  }
  if (toolName === 'read' && keys.join(',') === 'limit,path' &&
    typeof toolArguments.path === 'string' &&
    Number.isSafeInteger(toolArguments.limit) && toolArguments.limit > 0) {
    const canonical = canonicalJSON({
      path: toolArguments.path,
      limit: toolArguments.limit
    })
    if (externalHostToolExecutionArguments.has(hostToolArgumentAuthorityKey(
      workspace,
      'read',
      canonical
    ))) return canonical
  }
  return ''
}

function normalizeHostToolExecutionRequest(request, resolveWorkspaceRealPath = realpathSync) {
  if (!request || typeof request !== 'object' || Array.isArray(request) ||
    Object.keys(request).sort().join(',') !==
      'arguments,threadId,toolName,turnId,workspace') {
    return null
  }
  const threadId = typeof request?.threadId === 'string' ? request.threadId : ''
  const turnId = typeof request?.turnId === 'string' ? request.turnId : ''
  const toolName = typeof request?.toolName === 'string' ? request.toolName : ''
  const workspace = typeof request?.workspace === 'string' ? request.workspace : ''
  if (!threadId || threadId !== threadId.trim() ||
    !turnId || turnId !== turnId.trim() ||
    (toolName !== 'bash' && toolName !== 'read') ||
    !workspace || workspace !== workspace.trim() ||
    typeof resolveWorkspaceRealPath !== 'function') {
    return null
  }
  const workspaceRealPath = resolveWorkspaceRealPath(workspace)
  const workspaceStat = lstatSync(workspaceRealPath)
  if (!isAbsolute(workspaceRealPath) || !workspaceStat.isDirectory() ||
    workspaceStat.isSymbolicLink()) return null
  const canonicalArguments = canonicalHostToolExecutionArguments(
    toolName,
    request?.arguments,
    workspaceRealPath
  )
  if (!canonicalArguments) return null
  return Object.freeze({
    request: Object.freeze({
      threadId,
      turnId,
      toolName,
      workspace: workspaceRealPath,
      arguments: Object.freeze(JSON.parse(canonicalArguments))
    }),
    workspaceRealPath,
    canonicalArguments
  })
}

function privateHostToolExecutionBindingMatches(evidence, expected) {
  const binding = hostToolExecutionPrivateBindings.get(evidence)
  let normalized = null
  try {
    normalized = normalizeHostToolExecutionRequest(expected)
  } catch {
    normalized = null
  }
  return Boolean(binding && normalized && Object.isFrozen(evidence) &&
    evidence.ok === true &&
    evidence.toolName === normalized.request.toolName &&
    evidence.disclosure === 'metadata_only' &&
    evidence.privatePayloadWithheld === true &&
    binding.transportProvenance?.kind === 'cdp_window_analytix_runtime_request_v1' &&
    binding.transportProvenance?.path === TOOL_EXECUTION_OBSERVATION_PATH &&
    binding.transportProvenance?.method === 'POST' &&
    binding.transportProvenance?.status === 200 &&
    binding.transportProvenance?.rendererTargetCount === 1 &&
    /^[0-9a-f]{64}$/.test(String(binding.transportProvenance?.expressionDigest || '')) &&
    binding.threadId === normalized.request.threadId &&
    binding.turnId === normalized.request.turnId &&
    binding.toolName === normalized.request.toolName &&
    binding.workspaceRealPath === normalized.workspaceRealPath &&
    binding.canonicalArguments === normalized.canonicalArguments &&
    binding.observationDigest === evidence.observationDigest &&
    binding.authorityBindingDigest === evidence.authorityBindingDigest)
}

async function observeHostToolExecution(input, internalDependencies = {}) {
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
  const sleep = typeof internalDependencies.sleep === 'function'
    ? internalDependencies.sleep
    : (delayMs) => new Promise((resolveDelay) => setTimeout(resolveDelay, delayMs))
  let normalized = null
  try {
    normalized = normalizeHostToolExecutionRequest(request, resolveWorkspaceRealPath)
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
      expected,
      0
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
    let attemptCount = 0
    for (;;) {
      attemptCount += 1
      let raw
      try {
        raw = await evaluateBridge(
          target.webSocketDebuggerUrl,
          expression,
          timeoutMs
        )
      } catch {
        return hostToolExecutionObservationEvidence(
          { status: 0, observation: null },
          expected,
          attemptCount
        )
      }
      const evidence = hostToolExecutionObservationEvidence(raw, expected, attemptCount)
      if (evidence.ok || evidence.reasonCode !== 'not_observed' ||
          attemptCount > HOST_TOOL_EXECUTION_MAX_404_RETRIES) {
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
          toolName: normalized.request.toolName,
          workspaceRealPath: normalized.workspaceRealPath,
          canonicalArguments: normalized.canonicalArguments,
          observationDigest: evidence.observationDigest,
          authorityBindingDigest: evidence.authorityBindingDigest
        }))
        return evidence
      }
      const delayMs = HOST_TOOL_EXECUTION_404_RETRY_DELAYS_MS[attemptCount - 1]
      try {
        await sleep(Number.isSafeInteger(delayMs) ? delayMs : 0)
      } catch {
        return evidence
      }
    }
  } catch {
    return hostToolExecutionObservationEvidence(
      { status: 0, observation: null },
      expected,
      0
    )
  }
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
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) return []
  const children = new Map()
  for (const line of String(result.stdout || '').split(/\r?\n/)) {
    const match = line.trim().match(/^(\d+)\s+(\d+)$/)
    if (!match) continue
    const pid = Number(match[1])
    const parent = Number(match[2])
    const values = children.get(parent) || []
    values.push(pid)
    children.set(parent, values)
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

function listeningPids(port) {
  if (!Number.isInteger(port) || port <= 0 || process.platform === 'win32') return []
  const result = spawnSync('/usr/sbin/lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN'], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) return []
  return String(result.stdout || '')
    .split(/\r?\n/)
    .map((item) => Number(item.trim()))
    .filter((pid) => Number.isInteger(pid) && pid > 0)
}

function processExecutableEvidence(pid, expectedPath) {
  if (!Number.isInteger(pid) || pid <= 0 || process.platform !== 'darwin') {
    return { exactPackageExecutable: false, observedTextPathCount: 0 }
  }
  let expectedRealPath
  let expectedStat
  try {
    expectedRealPath = realpathSync(expectedPath)
    expectedStat = lstatSync(expectedRealPath)
    if (!expectedStat.isFile() || expectedStat.isSymbolicLink()) {
      return { exactPackageExecutable: false, observedTextPathCount: 0 }
    }
  } catch {
    return { exactPackageExecutable: false, observedTextPathCount: 0 }
  }
  const result = spawnSync('/usr/sbin/lsof', [
    '-a',
    '-p',
    String(pid),
    '-d',
    'txt',
    '-Fn'
  ], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) {
    return { exactPackageExecutable: false, observedTextPathCount: 0 }
  }
  const textPaths = [...new Set(String(result.stdout || '')
    .split(/\r?\n/)
    .filter((line) => line.startsWith('n/'))
    .map((line) => line.slice(1)))]
  const exactPackageExecutable = textPaths.some((candidate) => {
    try {
      const candidateRealPath = realpathSync(candidate)
      const candidateStat = lstatSync(candidateRealPath)
      return candidateRealPath === expectedRealPath &&
        candidateStat.isFile() &&
        !candidateStat.isSymbolicLink() &&
        sameFileIdentity(candidateStat, expectedStat)
    } catch {
      return false
    }
  })
  return {
    exactPackageExecutable,
    observedTextPathCount: textPaths.length
  }
}

export function classifyPackagedRuntimeProcessRole(command, expectedPath) {
  if (typeof command !== 'string' || typeof expectedPath !== 'string' ||
      !command || !expectedPath || expectedPath !== expectedPath.trim()) return 'unknown'
  const normalized = command.trim()
  const prefix = `${expectedPath} `
  if (!normalized.startsWith(prefix)) return 'unknown'
  const args = normalized.slice(prefix.length).trim()
  if (/^migration\s+migrate-desktop-private-history-v2(?:\s|$)/u.test(args)) {
    return 'desktop_private_history_migration'
  }
  if (/^bundled-plugin(?:\s|$)/u.test(args)) return 'bundled_plugin_materialization'
  if (/(?:^|\s)--addr(?:\s|=)/u.test(args) &&
      /(?:^|\s)--(?:durable-root|runtime-durable-root)(?:\s|=)/u.test(args)) {
    return 'runtime_server'
  }
  return 'unknown'
}

function packagedRuntimeProcessRole(pid, expectedPath) {
  if (!Number.isInteger(pid) || pid <= 0 || process.platform !== 'darwin') return 'unknown'
  const result = spawnSync('/bin/ps', ['-ww', '-o', 'command=', '-p', String(pid)], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) return 'unknown'
  return classifyPackagedRuntimeProcessRole(String(result.stdout || ''), expectedPath)
}

function runtimeBackendTopologyEvidence(rootPid, runtimePort, expectedRuntimeServerPath) {
  const listenerPids = [...new Set(listeningPids(runtimePort))]
  const descendants = descendantPids(rootPid)
  const listenerPid = listenerPids.length === 1 ? listenerPids[0] : 0
  const executable = processExecutableEvidence(listenerPid, expectedRuntimeServerPath)
  const exactRuntimeBackendPids = descendants.filter((pid) =>
    processExecutableEvidence(pid, expectedRuntimeServerPath).exactPackageExecutable
  )
  const runtimeProcessRoles = exactRuntimeBackendPids.map((pid) =>
    packagedRuntimeProcessRole(pid, expectedRuntimeServerPath)
  )
  const listenerIsTaskOwnedDescendant = listenerPid > 0 && descendants.includes(listenerPid)
  return {
    ok: listenerPids.length === 1 &&
      listenerIsTaskOwnedDescendant &&
      executable.exactPackageExecutable &&
      exactRuntimeBackendPids.length === 1 &&
      exactRuntimeBackendPids[0] === listenerPid,
    electronMainPid: rootPid,
    runtimeListenerPid: listenerPid,
    listenerCount: listenerPids.length,
    runtimeBackendProcessCount: exactRuntimeBackendPids.length,
    runtimeServerProcessCount: runtimeProcessRoles.filter((role) => role === 'runtime_server').length,
    desktopPrivateHistoryMigrationProcessCount: runtimeProcessRoles.filter(
      (role) => role === 'desktop_private_history_migration'
    ).length,
    bundledPluginMaterializationProcessCount: runtimeProcessRoles.filter(
      (role) => role === 'bundled_plugin_materialization'
    ).length,
    unknownRuntimeProcessCount: runtimeProcessRoles.filter((role) => role === 'unknown').length,
    listenerIsTaskOwnedDescendant,
    exactPackageExecutable: executable.exactPackageExecutable,
    observedTextPathCount: executable.observedTextPathCount
  }
}

export function packagedMilestoneASkillID(runtimeSkills) {
  if (runtimeSkills?.schemaVersion !== 2 ||
    runtimeSkills?.enabled !== true ||
    runtimeSkills?.available !== true ||
    runtimeSkills?.reasonCode !== 'available' ||
    runtimeSkills?.configuredRootCount !== 1 ||
    runtimeSkills?.skillCount !== 1 ||
    runtimeSkills?.validationErrorCount !== 0 ||
    !Array.isArray(runtimeSkills?.skills)) return ''
  return runtimeSkills.skills.length === 1 &&
    runtimeSkills.skills[0]?.id === PACKAGED_SKILL_ID
    ? PACKAGED_SKILL_ID
    : ''
}

export function runtimePublicSeamEvidence(observation, runtimePort) {
  const health = observation?.health
  const info = observation?.runtimeInfo
  const tools = observation?.runtimeTools
  const toolContracts = tools?.toolContracts
  const mcpServers = Array.isArray(tools?.mcpServers) ? tools.mcpServers : []
  const fundsDiagnostics = mcpServers.filter((item) => item?.id === 'analytix_funds')
  const ordinaryMCPDiagnostics = mcpServers.filter((item) =>
    item?.id !== 'analytix_funds' && item?.id !== 'analytix-fund-analysis'
  )
  const packagedScheduleDiagnostics = ordinaryMCPDiagnostics.filter((item) =>
    item?.id === PACKAGED_ORDINARY_MCP_SERVER_ID
  )
  const ordinaryMCPAvailable = ordinaryMCPDiagnostics.length === 1 &&
    packagedScheduleDiagnostics.length === 1 &&
    packagedScheduleDiagnostics.every((item) =>
      item?.enabled === true && item?.available === true && item?.connected === true &&
      item?.transport === 'stdio' && item?.authStatus === 'none' &&
      item?.trustScope === 'user' &&
      item?.toolCount === PACKAGED_ORDINARY_MCP_TOOL_COUNT &&
      item?.toolContractQuarantineCount === 0
    )
  const fundsExecutionUnavailable = fundsDiagnostics.length === 1 &&
    fundsDiagnostics[0]?.status === 'unavailable' &&
    [
      'funds_package_binding_changed',
      'funds_packaged_source_invalid',
      'funds_installed_state_invalid'
    ].includes(fundsDiagnostics[0]?.failureCode) &&
    fundsDiagnostics[0]?.enabled === false &&
    fundsDiagnostics[0]?.available === false &&
    Number.isSafeInteger(fundsDiagnostics[0]?.toolCount) &&
    fundsDiagnostics[0].toolCount === 0
  const fundsTransportAvailable = fundsDiagnostics.length === 1 &&
    fundsDiagnostics[0]?.enabled === true &&
    fundsDiagnostics[0]?.available === true &&
    fundsDiagnostics[0]?.connected === true &&
    Number(fundsDiagnostics[0]?.toolCount || 0) > 0
  // Runtime tools are global diagnostics, not a per-turn authority seam.
  // Milestone A readiness is independent from whether the optional funds
  // transport is absent, unavailable, or healthy and connected.
  const ordinaryCatalogNonempty = Number.isSafeInteger(toolContracts?.count) &&
    toolContracts.count > 0 &&
    /^[0-9a-f]{64}$/.test(String(toolContracts?.catalogHash || ''))
  const gitCommandAvailable = Array.isArray(tools?.commands) && tools.commands.some((item) =>
    item?.binary === 'git' && item?.found === true && item?.status === 'available'
  )
  const skills = observation?.runtimeSkills
  const skillCatalogAvailable = packagedMilestoneASkillID(skills) === PACKAGED_SKILL_ID
  const runtimeInfoOk = info?.schemaVersion === 2 &&
    info?.status === 'ready' &&
    info?.listenerScope === 'loopback' &&
    info?.port === runtimePort &&
    info?.insecure === false &&
    info?.storage?.configured === true &&
    info?.storage?.available === true &&
    info?.executionPolicy?.approvalPolicy === 'auto' &&
    info?.executionPolicy?.sandboxMode === 'danger-full-access'
  const runtimeToolsOk = tools?.schemaVersion === 2 &&
    Number.isSafeInteger(tools?.providerCount) &&
    tools.providerCount > 0 &&
    ordinaryCatalogNonempty
  const runtimeThreadListProbeStatus = Number.isSafeInteger(
    observation?.runtimeThreadListProbeStatus
  ) && observation.runtimeThreadListProbeStatus >= 0 &&
    observation.runtimeThreadListProbeStatus <= 599
    ? observation.runtimeThreadListProbeStatus
    : 0
  const runtimeThreadListProbeOk = observation?.runtimeThreadListProbeOk === true &&
    runtimeThreadListProbeStatus >= 200 && runtimeThreadListProbeStatus < 300
  return {
    ok: health?.service === 'analytix' && runtimeInfoOk && runtimeToolsOk &&
      runtimeThreadListProbeOk,
    healthOk: health?.service === 'analytix',
    runtimeInfoOk,
    runtimeToolsOk,
    runtimeThreadListProbeOk,
    runtimeThreadListProbeStatus,
    ordinaryCatalogNonempty,
    gitCommandAvailable,
    skillCatalogAvailable,
    skillCount: skillCatalogAvailable ? skills.skillCount : 0,
    ordinaryMCPAvailable,
    ordinaryMCPServerId: ordinaryMCPAvailable ? PACKAGED_ORDINARY_MCP_SERVER_ID : '',
    ordinaryMCPServerCount: ordinaryMCPDiagnostics.length,
    ordinaryMCPToolCount: ordinaryMCPAvailable
      ? packagedScheduleDiagnostics[0].toolCount
      : 0,
    fundsTransportAvailable,
    fundsExecutionUnavailable,
    fundsServerDiagnosticCount: fundsDiagnostics.length,
    ordinaryToolContractCount: ordinaryCatalogNonempty ? toolContracts.count : 0,
    ordinaryToolCatalogHash: ordinaryCatalogNonempty ? toolContracts.catalogHash : ''
  }
}

function safeNonNegativeCount(value) {
  return Number.isSafeInteger(value) && value >= 0 ? value : 0
}

function longContextIdleFailureWorkflowFields({
  idleFailure,
  observation,
  runtimePort,
  rootPid,
  expectedRuntimeServerPath
}) {
  const publicSeam = runtimePublicSeamEvidence(observation, runtimePort)
  let topology = {
    ok: false,
    listenerCount: 0,
    runtimeBackendProcessCount: 0,
    listenerIsTaskOwnedDescendant: false,
    exactPackageExecutable: false
  }
  try {
    topology = runtimeBackendTopologyEvidence(
      rootPid,
      runtimePort,
      expectedRuntimeServerPath
    )
  } catch {
    // Keep failure diagnostics bounded when process inspection is unavailable.
  }
  const rendererTargetCount = safeNonNegativeCount(observation?.rendererTargetCount)
  return Object.freeze({
    longContextIdleFailureObserved: Boolean(idleFailure),
    longContextIdleFailureEditorObserved: idleFailure?.editorObserved === true,
    longContextIdleFailureButtonObserved: idleFailure?.buttonObserved === true,
    longContextIdleFailureEditorEmptyObserved: idleFailure?.editorEmpty === true,
    longContextIdleFailureButtonNotLoadingObserved: idleFailure?.buttonNotLoading === true,
    longContextIdleFailureButtonDisabledObserved: idleFailure?.buttonDisabled === true,
    longContextIdleFailureButtonLabelAcceptedObserved: idleFailure?.buttonLabelAccepted === true,
    longContextIdleFailureWaitedMs: safeNonNegativeCount(idleFailure?.waitedMs),
    longContextIdleFailureHealthOk: publicSeam.healthOk === true,
    longContextIdleFailureRuntimeInfoOk: publicSeam.runtimeInfoOk === true,
    longContextIdleFailureRuntimeToolsOk: publicSeam.runtimeToolsOk === true,
    longContextIdleFailureRuntimeThreadListOk:
      publicSeam.runtimeThreadListProbeOk === true,
    longContextIdleFailureRuntimeThreadListStatus:
      safeNonNegativeCount(publicSeam.runtimeThreadListProbeStatus),
    longContextIdleFailureRuntimeBackendTopologyOk: topology.ok === true,
    longContextIdleFailureRuntimeListenerCount: safeNonNegativeCount(topology.listenerCount),
    longContextIdleFailureRuntimeBackendProcessCount:
      safeNonNegativeCount(topology.runtimeBackendProcessCount),
    longContextIdleFailureRuntimeListenerOwnedObserved:
      topology.listenerIsTaskOwnedDescendant === true,
    longContextIdleFailureExactPackageExecutableObserved:
      topology.exactPackageExecutable === true,
    longContextIdleFailureRendererTargetCount: rendererTargetCount
  })
}

async function waitForLocalProviderWorkbench({
  debugPort,
  workspace,
  runtimePort,
  runtimeDataDir,
  timeoutMs,
  exactThreadId = '',
  onFreshRegistry = null
}) {
  const deadline = Date.now() + timeoutMs
  let latest = null
  let normalLocalProviderSetupObserved = false
  let freshRegistryIncarnation = ''
  let readDiagnostic = managedWorkbenchReadDiagnostic(null, 0)
  while (Date.now() < deadline) {
    try {
      latest = await observeRenderer({
        debugPort,
        workspace,
        exactThreadId,
        timeoutMs: Math.min(10_000, Math.max(1000, deadline - Date.now()))
      })
      readDiagnostic = managedWorkbenchReadDiagnostic(null, 0)
      if (!freshRegistryIncarnation && freshLocalProviderRegistry(latest?.providerRegistry)) {
        freshRegistryIncarnation = latest.providerRegistry.registryIncarnation
        if (onFreshRegistry) await onFreshRegistry(Math.max(1, deadline - Date.now()))
      }
      normalLocalProviderSetupObserved = Boolean(freshRegistryIncarnation &&
        latest?.providerRegistry?.registryIncarnation === freshRegistryIncarnation &&
        latest?.providerRegistry?.registryRevision !== '0')
      const provider = milestoneALocalProvider(latest)
      const publicSeam = runtimePublicSeamEvidence(latest, runtimePort)
      if (provider.blocker === 'local_provider_formal_model_invalid') {
        return {
          ok: false,
          blocked: false,
          blocker: provider.blocker,
          observation: latest,
          provider,
          publicSeam,
          normalLocalProviderSetupObserved,
          readDiagnostic
        }
      }
      const settingsReady = latest?.settings?.workspaceRoot === workspace &&
        latest?.settings?.runtime?.dataDir === runtimeDataDir &&
        latest?.settings?.runtime?.executionPolicyVersion === 2 &&
        latest?.settings?.runtime?.approvalPolicy === 'auto' &&
        latest?.settings?.runtime?.sandboxMode === 'danger-full-access'
      const rendererReady = latest?.rendererTargetCount === 1 &&
        latest?.apiPresent === true &&
        latest?.composerPresent === true &&
        latest?.primaryButtonPresent === true
      if (rendererReady && settingsReady && provider.ok && publicSeam.ok) {
        return {
          ok: true,
          blocked: false,
          blocker: '',
          observation: latest,
          provider,
          publicSeam,
          normalLocalProviderSetupObserved,
          readDiagnostic
        }
      }
    } catch (error) {
      const candidate = error?.managedWorkbenchReadDiagnostic
      if (candidate?.observed === true) {
        readDiagnostic = candidate
      }
      // Normal local onboarding may reload while Registry and the sidecar settle.
    }
    await sleep(750)
  }
  const registry = latest?.providerRegistry
  const provider = milestoneALocalProvider(latest)
  const readiness = classifyLocalProviderWorkbenchReadiness(registry, provider, readDiagnostic)
  return {
    ok: false,
    ...readiness,
    observation: latest,
    provider,
    publicSeam: runtimePublicSeamEvidence(latest, runtimePort),
    normalLocalProviderSetupObserved,
    readDiagnostic
  }
}

export function classifyLocalProviderWorkbenchReadiness(registry, provider, readDiagnostic = null) {
  const readFailureBlocker = readDiagnostic?.observed === true
    ? readDiagnostic.readErrorStage === 'thread_detail'
      ? 'packaged_runtime_thread_recovery_not_ready'
      : readDiagnostic.readErrorStage === 'thread_summary'
        ? 'packaged_runtime_thread_summary_not_ready'
        : ['provider_receipt_replay', 'accepted_final_replay'].includes(readDiagnostic.readErrorStage)
          ? 'packaged_runtime_event_replay_not_ready'
          : 'packaged_renderer_observation_failed'
    : ''
  return Object.freeze({
    blocked: !readFailureBlocker && (freshLocalProviderRegistry(registry) || provider?.blocked === true),
    blocker: readFailureBlocker || provider?.blocker || 'packaged_workbench_not_ready'
  })
}

function taskOwnedProcessPids(sandboxRoot) {
  if (!sandboxRoot || process.platform === 'win32') return []
  const result = spawnSync('/bin/ps', ['-axo', 'pid=,command='], {
    cwd: process.cwd(),
    encoding: 'utf8',
    stdio: 'pipe'
  })
  if (result.status !== 0) return []
  return String(result.stdout || '')
    .split(/\r?\n/)
    .map((line) => {
      const match = line.trim().match(/^(\d+)\s+(.*)$/)
      if (!match || !match[2].includes(sandboxRoot)) return 0
      return Number(match[1])
    })
    .filter((pid) => Number.isInteger(pid) && pid > 0 && pid !== process.pid)
}

async function waitForExit(child, timeoutMs) {
  if (!child || child.exitCode !== null || child.signalCode) return true
  return Promise.race([
    new Promise((resolvePromise) => child.once('close', () => resolvePromise(true))),
    sleep(timeoutMs).then(() => false)
  ])
}

async function stopTaskOwnedChild(child) {
  if (!child || child.exitCode !== null || child.signalCode) return
  child.kill('SIGTERM')
  if (await waitForExit(child, 3000)) return
  if (child.exitCode === null && !child.signalCode) child.kill('SIGKILL')
  await waitForExit(child, 2000)
}

async function stopExactTaskOwnedProcesses(sandboxRoot) {
  const pids = taskOwnedProcessPids(sandboxRoot)
  for (const pid of pids) {
    try {
      process.kill(pid, 'SIGTERM')
    } catch {
      // Already gone.
    }
  }
  await sleep(500)
  for (const pid of taskOwnedProcessPids(sandboxRoot)) {
    try {
      process.kill(pid, 'SIGKILL')
    } catch {
      // Already gone.
    }
  }
}

function successfulToolExecutions(thread) {
  const callsByID = new Map()
  const results = []
  for (const turn of Array.isArray(thread?.turns) ? thread.turns : []) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      if (item?.kind === 'tool_call' &&
        item.status === 'completed' &&
        typeof item.callId === 'string' &&
        item.callId &&
        typeof item.toolName === 'string' &&
        item.toolName) {
        callsByID.set(item.callId, { item, turnId: turn?.id })
      } else if (item?.kind === 'tool_result') {
        results.push({ item, turnId: turn?.id })
      }
    }
  }
  return results.flatMap(({ item, turnId }) => {
    const call = callsByID.get(item?.callId)
    if (!call ||
      call.turnId !== turnId ||
      call.item.toolName !== item.toolName ||
      call.item.toolKind !== item.toolKind ||
      item.status !== 'completed' ||
      item.isError !== false ||
      item.output?.status !== 'completed') {
      return []
    }
    return [{
      callId: item.callId,
      turnId,
      toolName: item.toolName,
      toolKind: item.toolKind,
      resultProjectionKind: item.output?.projectionKind || '',
      resultMessageKey: item.output?.messageKey || '',
      callCreatedAt: call.item.createdAt || '',
      callFinishedAt: call.item.finishedAt || '',
      resultCreatedAt: item.createdAt || '',
      resultFinishedAt: item.finishedAt || ''
    }]
  })
}

function toolResultItemID(turnId, callId) {
  if (typeof turnId !== 'string' || !turnId ||
    !/^call_host_[0-9a-f]{64}$/u.test(String(callId || ''))) return ''
  const length = (value) => {
    const body = Buffer.from(value, 'utf8')
    const prefix = Buffer.alloc(8)
    prefix.writeBigUInt64BE(BigInt(body.length))
    return Buffer.concat([prefix, body])
  }
  return `item_result_${sha256(Buffer.concat([
    Buffer.from('analytix.tool-result-item/id/v1\0', 'utf8'),
    length(turnId),
    length(callId)
  ]))}`
}

export function milestoneAPlanArtifactEvidence(authority, thread, expectedTurnId = '') {
  const base = {
    ok: false,
    blocker: '',
    relativePath: '',
    contentHash: '',
    byteSize: 0,
    resultItemDigest: '',
    revisionSequenceDigest: '',
    fileIdentityDigest: '',
    excludeSha256: '',
    planEntryCount: 0,
    planCallCount: 0,
    planResultCount: 0,
    turnId: '',
    callIdHash: '',
    pairFailureCode: ''
  }
  if (!externalRepositoryAuthorityShapeValid(authority)) {
    return { ...base, blocker: 'milestone_a_plan_authority_invalid' }
  }
  const planCalls = []
  const planResults = []
  for (const turn of Array.isArray(thread?.turns) ? thread.turns : []) {
    if (expectedTurnId && turn?.id !== expectedTurnId) continue
    const items = Array.isArray(turn?.items) ? turn.items : []
    for (let itemIndex = 0; itemIndex < items.length; itemIndex += 1) {
      const item = items[itemIndex]
      if (item?.toolName !== 'create_plan') continue
      const record = {
        item,
        itemIndex,
        turnId: String(turn?.id || ''),
        turnThreadId: String(turn?.threadId || '')
      }
      if (item?.kind === 'tool_call') planCalls.push(record)
      if (item?.kind === 'tool_result') planResults.push(record)
    }
  }
  if (planCalls.length === 0 || planCalls.length !== planResults.length) {
    return {
      ...base,
      blocker: 'milestone_a_plan_execution_count_invalid',
      planCallCount: planCalls.length,
      planResultCount: planResults.length
    }
  }
  const countedBase = {
    ...base,
    planCallCount: planCalls.length,
    planResultCount: planResults.length
  }
  const resultsByCallID = new Map()
  for (const result of planResults) {
    const callId = String(result.item?.callId || '')
    if (!callId || resultsByCallID.has(callId)) {
      return {
        ...countedBase,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: callId
          ? 'plan_result_call_id_duplicate'
          : 'plan_result_call_id_missing',
        planCallCount: planCalls.length,
        planResultCount: planResults.length
      }
    }
    resultsByCallID.set(callId, result)
  }
  const seenCallIDs = new Set()
  const pairs = []
  for (const call of planCalls) {
    const callId = String(call.item?.callId || '')
    const result = resultsByCallID.get(callId)
    const turnId = call.turnId
    const prior = pairs.at(-1)
    let pairFailureCode = ''
    if (!/^call_host_[0-9a-f]{64}$/u.test(callId)) {
      pairFailureCode = 'plan_call_id_invalid'
    } else if (seenCallIDs.has(callId)) {
      pairFailureCode = 'plan_call_id_duplicate'
    } else if (!result) {
      pairFailureCode = 'plan_result_missing'
    } else if (expectedTurnId && turnId !== expectedTurnId) {
      pairFailureCode = 'plan_expected_turn_mismatch'
    } else if (!turnId || result.turnId !== turnId) {
      pairFailureCode = 'plan_pair_turn_mismatch'
    } else if (
      call.turnThreadId !== thread?.id || result.turnThreadId !== thread?.id ||
      thread?.id !== call.item?.threadId || thread.id !== result.item?.threadId ||
      call.item?.turnId !== turnId || result.item?.turnId !== turnId
    ) {
      pairFailureCode = 'plan_item_authority_mismatch'
    } else if (result.item?.isError === true) {
      pairFailureCode = 'plan_result_failed'
    } else if (result.item?.isError !== false) {
      pairFailureCode = 'plan_result_error_flag_invalid'
    } else if (call.item?.role !== 'tool' || result.item?.role !== 'tool') {
      pairFailureCode = 'plan_item_role_mismatch'
    } else if (call.item?.status !== 'completed') {
      pairFailureCode = 'plan_call_lifecycle_unsettled'
    } else if (result.item?.status !== 'completed') {
      pairFailureCode = 'plan_result_lifecycle_unsettled'
    } else if (call.item?.toolKind !== 'file_change' || result.item?.toolKind !== 'file_change') {
      pairFailureCode = 'plan_item_kind_mismatch'
    } else if (result.item?.callId !== callId) {
      pairFailureCode = 'plan_result_call_id_mismatch'
    } else if (result.item?.id !== toolResultItemID(turnId, callId)) {
      pairFailureCode = 'plan_result_item_id_mismatch'
    } else if (call.itemIndex >= result.itemIndex ||
      (prior && prior.result.itemIndex >= call.itemIndex)) {
      pairFailureCode = 'plan_item_order_invalid'
    }
    if (pairFailureCode) {
      return {
        ...countedBase,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode,
        planCallCount: planCalls.length,
        planResultCount: planResults.length
      }
    }
    seenCallIDs.add(callId)
    pairs.push({ call, result, callId, turnId })
  }
  const projections = pairs.map((pair) => ({
    ...pair,
    output: pair.result.item?.output,
    plan: pair.result.item?.output?.plan
  }))
  if (projections.some(({ output, plan }) => !exactObjectKeys(output, [
    'code', 'disclosure', 'evidenceAuthority', 'factAnswerAllowed', 'messageKey',
    'plan', 'privatePayloadWithheld', 'projectionKind', 'schemaVersion', 'status'
  ]) || !exactObjectKeys(plan, [
    'byteSize', 'contentHash', 'operation', 'planId', 'relativePath', 'savedAt'
  ]))) {
    return { ...countedBase, blocker: 'milestone_a_plan_projection_shape_invalid' }
  }
  const expectedRelativePath = authority.planArtifactRelativePath
  const savedAtValues = projections.map(({ plan }) => Date.parse(plan.savedAt))
  const projectionValid = projections.every(({ output, plan }, index) =>
    output.schemaVersion === 1 &&
    output.projectionKind === 'plan_status' && output.disclosure === 'metadata_only' &&
    output.messageKey === 'plan_updated' && output.status === 'completed' &&
    output.code === 'plan_updated' && output.privatePayloadWithheld === true &&
    output.factAnswerAllowed === false && output.evidenceAuthority === false &&
    plan.relativePath === expectedRelativePath && plan.operation === 'draft' &&
    plan.planId === `${authority.workspace}:${expectedRelativePath.toLowerCase()}` &&
    /^[0-9a-f]{64}$/u.test(String(plan.contentHash || '')) &&
    Number.isSafeInteger(plan.byteSize) && plan.byteSize > 0 &&
    plan.byteSize <= PLAN_ARTIFACT_MAX_BYTES &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/u.test(String(plan.savedAt || '')) &&
    Number.isFinite(savedAtValues[index]) &&
    (index === 0 || savedAtValues[index - 1] <= savedAtValues[index])
  )
  if (!projectionValid) {
    return { ...countedBase, blocker: 'milestone_a_plan_projection_binding_invalid' }
  }
  const revisionSequenceDigest = sha256(canonicalJSON(projections.map(({
    call,
    result,
    output
  }) => ({
    call: {
      id: call.item.id,
      threadId: call.item.threadId,
      turnId: call.item.turnId,
      toolName: call.item.toolName,
      callId: call.item.callId,
      toolKind: call.item.toolKind,
      status: call.item.status
    },
    result: {
      id: result.item.id,
      threadId: result.item.threadId,
      turnId: result.item.turnId,
      toolName: result.item.toolName,
      callId: result.item.callId,
      toolKind: result.item.toolKind,
      status: result.item.status,
      isError: result.item.isError,
      output
    }
  }))))
  const finalProjection = projections.at(-1)
  const { call, result, callId, turnId, output, plan } = finalProjection
  const pathEvidence = milestoneAPlanArtifactPathEvidence(
    authority.workspace,
    expectedRelativePath
  )
  const planDir = join(authority.workspace, PLAN_ARTIFACT_RELATIVE_DIR)
  let planEntries = []
  try {
    planEntries = readdirSync(planDir, { withFileTypes: true })
  } catch {
    planEntries = []
  }
  const expectedName = expectedRelativePath.slice(PLAN_ARTIFACT_RELATIVE_DIR.length + 1)
  const onlyExactPlan = pathEvidence.safe && pathEvidence.present &&
    planEntries.length === 1 && planEntries[0].name === expectedName &&
    planEntries[0].isFile() && !planEntries[0].isSymbolicLink()
  if (!onlyExactPlan) {
    return {
      ...countedBase,
      blocker: 'milestone_a_plan_artifact_tree_invalid',
      planEntryCount: planEntries.length
    }
  }
  const artifactPath = join(authority.workspace, ...expectedRelativePath.split('/'))
  const artifact = stableOwnerPrivateFile(artifactPath, PLAN_ARTIFACT_MAX_BYTES)
  const fileBound = artifact.regular && artifact.sha256 === plan.contentHash &&
    artifact.byteLength === plan.byteSize
  if (!fileBound) {
    return {
      ...countedBase,
      blocker: 'milestone_a_plan_artifact_digest_invalid',
      relativePath: expectedRelativePath,
      contentHash: artifact.sha256,
      byteSize: artifact.byteLength,
      planEntryCount: planEntries.length
    }
  }
  const exclude = milestoneAPlanExcludeEvidence(authority)
  if (!exclude.ok) {
    return { ...countedBase, blocker: 'milestone_a_plan_exclude_invalid' }
  }
  const resultItemDigest = sha256(canonicalJSON({
    id: result.item.id,
    threadId: result.item.threadId,
    turnId: result.item.turnId,
    toolName: result.item.toolName,
    callId: result.item.callId,
    toolKind: result.item.toolKind,
    status: result.item.status,
    isError: result.item.isError,
    output
  }))
  const fileIdentityDigest = sha256(canonicalJSON({
    dev: artifact.identity.dev,
    ino: artifact.identity.ino,
    uid: artifact.identity.uid,
    mode: artifact.identity.mode & 0o777,
    nlink: artifact.identity.nlink,
    size: artifact.byteLength
  }))
  return {
    ...countedBase,
    ok: true,
    relativePath: expectedRelativePath,
    contentHash: artifact.sha256,
    byteSize: artifact.byteLength,
    resultItemDigest,
    revisionSequenceDigest,
    fileIdentityDigest,
    excludeSha256: exclude.excludeSha256,
    planEntryCount: planEntries.length,
    planCallCount: planCalls.length,
    planResultCount: planResults.length,
    turnId,
    callIdHash: sha256(callId)
  }
}

function toolCallAttempts(thread) {
  const attempts = []
  for (const turn of Array.isArray(thread?.turns) ? thread.turns : []) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      if (item?.kind !== 'tool_call') continue
      attempts.push({
        callId: typeof item.callId === 'string' ? item.callId : '',
        turnId: typeof turn?.id === 'string' ? turn.id : '',
        toolName: typeof item.toolName === 'string' ? item.toolName : '',
        toolKind: typeof item.toolKind === 'string' ? item.toolKind : '',
        status: typeof item.status === 'string' ? item.status : ''
      })
    }
  }
  return attempts
}

function fundsToolName(value) {
  return value === ACCOUNT_FLOW_TOOL ||
    (typeof value === 'string' && (
      value.startsWith('mcp__analytix_funds__') ||
      value.startsWith('mcp__analytix-fund-analysis__')
    ))
}

export function protectedFundsUnavailableTurnEvidence(
  observation,
  expectedThreadId = '',
  expectedTurnId = ''
) {
  const thread = observation?.thread
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  const turn = expectedTurnId
    ? turns.find((item) => item?.id === expectedTurnId)
    : turns.at(-1)
  const view = turn?.acceptedFinalView
  const receiptMetadata = view?.receiptMetadata
  const deliveries = Array.isArray(thread?.acceptedFinalDeliveries)
    ? thread.acceptedFinalDeliveries
    : []
  const matchingDeliveries = deliveries.filter((candidate) =>
    candidate?.threadId === thread?.id && candidate?.turnId === turn?.id &&
    candidate?.publicationCommitId === view?.acceptedFinalDigest
  )
  const delivery = matchingDeliveries.length === 1 ? matchingDeliveries[0] : null
  const singularMatches = thread?.acceptedFinalDelivery === undefined || (
    deliveries.length === 1 &&
    canonicalJSON(thread.acceptedFinalDelivery) === canonicalJSON(delivery)
  )
  const successfulFundsExecutions = successfulToolExecutions(thread).filter((item) =>
    item.turnId === turn?.id && fundsToolName(item.toolName)
  )
  const ok = typeof thread?.id === 'string' && thread.id.length > 0 &&
    (!expectedThreadId || thread.id === expectedThreadId) &&
    typeof turn?.id === 'string' && turn.id.length > 0 &&
    (!expectedTurnId || turn.id === expectedTurnId) &&
    turn.status === 'completed' &&
    singularMatches && matchingDeliveries.length === 1 &&
    acceptedFinalDeliveryV2ShapeBound(delivery, thread.id, turn.id, view) &&
    view?.schemaVersion === 3 &&
    view?.publicationState === 'accepted' &&
    view?.variant === 'SourceUnavailableAnswer' &&
    view?.terminalReason === 'source_unavailable' &&
    view?.coverageStatus === 'unavailable' &&
    typeof view?.blockerCode === 'string' && view.blockerCode.length > 0 &&
    view?.claimCount === 0 &&
    Array.isArray(view?.claimTypes) && view.claimTypes.length === 0 &&
    receiptMetadata?.projection === 'masked_metadata_only' &&
    receiptMetadata?.count === 0 &&
    Array.isArray(receiptMetadata?.citations) && receiptMetadata.citations.length === 0 &&
    successfulFundsExecutions.length === 0
  return {
    ok,
    threadId: typeof thread?.id === 'string' ? thread.id : '',
    turnId: typeof turn?.id === 'string' ? turn.id : '',
    acceptedFinalDigest: String(view?.acceptedFinalDigest || ''),
    claimCount: Number(view?.claimCount || 0),
    receiptCount: Number(receiptMetadata?.count || 0),
    successfulFundsExecutionCount: successfulFundsExecutions.length
  }
}

export function protectedFundsWaitDisposition(
  observation,
  { previousTurnCount = 0, expectedThreadId = '' } = {}
) {
  const thread = observation?.thread
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0 ||
      !thread?.id || (expectedThreadId && thread.id !== expectedThreadId) ||
      turns.length <= previousTurnCount) {
    return 'pending'
  }
  const turn = turns.at(-1)
  if (!turn || !terminalThread(thread)) {
    return 'pending'
  }
  const evidence = protectedFundsUnavailableTurnEvidence(
    observation,
    expectedThreadId,
    turn.id
  )
  if (evidence.ok) return 'completed'
  const hasProjectedBoundary = turn.acceptedFinalView != null ||
    (Array.isArray(thread.acceptedFinalDeliveries) &&
      thread.acceptedFinalDeliveries.some((candidate) => candidate?.turnId === turn.id))
  if (turn.status !== 'completed' || hasProjectedBoundary) {
    return 'terminal_without_typed_boundary'
  }
  // Async terminal status can become visible before the immutable accepted
  // final and public view are hydrated. Keep observing that same terminal
  // turn; a plain assistant string is not sufficient boundary evidence.
  return 'pending'
}

function completedAssistantRecords(thread) {
  const records = []
  for (const turn of Array.isArray(thread?.turns) ? thread.turns : []) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      if (item?.kind === 'assistant_text' &&
        item.status === 'completed' &&
        typeof item.text === 'string') {
        records.push({ turnId: turn?.id, text: item.text })
      }
    }
  }
  return records
}

const GENERAL_COMPACTION_SUMMARY =
  'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.'
const CASE_COMPACTION_SUMMARY =
  'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'
const CASE_COMPACTION_PUBLIC_KEYS = new Set([
  'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt', 'kind',
  'summary', 'auto', 'pinnedConstraints', 'sourceDigest',
  'digestMarker', 'sourceItemIds', 'schemaVersion', 'reasoningExcluded',
  'reasoningExclusionProof', 'assistantProseExcluded', 'toolPayloadsExcluded',
  'caseFactsExcluded', 'caseHistoryProjectionVersion'
])
const NONZERO_COMPACTION_FAILURE_REASON_CODE =
  'nonzero_compaction_manual_marker_not_observed'
const COMPACTION_PROJECTION_CLASSES = new Set([
  'not_observed',
  'ordinary_exact',
  'case_bound_typed'
])

function compactionIdentity(item) {
  return {
    id: typeof item?.id === 'string' ? item.id : '',
    turnId: typeof item?.turnId === 'string' ? item.turnId : '',
    threadId: typeof item?.threadId === 'string' ? item.threadId : '',
    sourceDigest: typeof item?.sourceDigest === 'string' ? item.sourceDigest : '',
    digestMarker: typeof item?.digestMarker === 'string' ? item.digestMarker : '',
    schemaVersion: Number.isSafeInteger(item?.schemaVersion) ? item.schemaVersion : null,
    providerHistoryProjectionVersion:
      Number.isSafeInteger(item?.providerHistoryProjectionVersion)
        ? item.providerHistoryProjectionVersion
        : null,
    caseHistoryProjectionVersion:
      Number.isSafeInteger(item?.caseHistoryProjectionVersion)
        ? item.caseHistoryProjectionVersion
        : null,
    auto: typeof item?.auto === 'boolean' ? item.auto : null
  }
}

function compactionIdentityDigest(item) {
  return sha256(canonicalJSON(compactionIdentity(item)))
}

function compactionEvidenceItems(value) {
  return Array.isArray(value) ? value : Array.isArray(value?.compactions) ? value.compactions : []
}

export function compactionBaselineEvidence(compactions) {
  const items = compactionEvidenceItems(compactions)
  const identityDigests = items.map(compactionIdentityDigest)
  return Object.freeze({
    count: items.length,
    identityDigests: Object.freeze(identityDigests),
    digest: sha256(canonicalJSON(identityDigests))
  })
}

function validCompactionBaseline(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      !Number.isSafeInteger(value.count) || value.count < 0 ||
      !Array.isArray(value.identityDigests) ||
      value.identityDigests.length !== value.count ||
      value.identityDigests.some((item) => !/^[0-9a-f]{64}$/u.test(String(item))) ||
      new Set(value.identityDigests).size !== value.identityDigests.length ||
      value.digest !== sha256(canonicalJSON(value.identityDigests))) return false
  return true
}

function compactionProofFailure(reasonCode, extra = {}) {
  return {
    ok: false,
    reasonCode,
    projectionClass: 'not_observed',
    manualCompactionObserved: false,
    manualCompactionAuto: null,
    manualCompactionNonzeroBound: false,
    manualCompactionSourceAncestryBound: false,
    replacedTokens: 0,
    sourceDigest: '',
    sourceItemIdsDigest: '',
    candidateDigest: '',
    newAfterBaseline: false,
    ...extra
  }
}

function compactionProofForItem(item, candidateDigest) {
  if (!item || typeof item !== 'object' || Array.isArray(item)) {
    return compactionProofFailure('manual_compaction_schema_or_proof_invalid')
  }
  if (item.auto !== false) {
    return compactionProofFailure('manual_compaction_auto_true', {
      manualCompactionAuto: typeof item.auto === 'boolean' ? item.auto : null,
      candidateDigest
    })
  }
  const thread = { id: item.threadId }
  const turn = { id: item.turnId, status: item.status }
  if (!validCompactionItem(thread, turn, item)) {
    return compactionProofFailure('manual_compaction_schema_or_proof_invalid', {
      manualCompactionAuto: false,
      candidateDigest
    })
  }
  const sourceItemIds = Array.isArray(item.sourceItemIds) ? item.sourceItemIds : []
  const caseBound = item.caseHistoryProjectionVersion === 2
  const projectionClass = caseBound ? 'case_bound_typed' : 'ordinary_exact'
  return {
    ok: true,
    reasonCode: 'manual_compaction_observed',
    projectionClass,
    manualCompactionObserved: true,
    manualCompactionAuto: false,
    // Case-bound public markers intentionally omit the private replacement count.
    manualCompactionNonzeroBound: caseBound || item.replacedTokens > 0,
    manualCompactionSourceAncestryBound: caseBound || sourceItemIds.length > 0,
    replacedTokens: caseBound ? 0 : item.replacedTokens,
    sourceDigest: item.sourceDigest,
    sourceItemIdsDigest: caseBound ? '' : sha256(canonicalJSON(sourceItemIds)),
    candidateDigest,
    newAfterBaseline: true,
    item
  }
}

export function manualCompactionProofEvidence(evidence, baseline) {
  if (!validCompactionBaseline(baseline)) {
    return compactionProofFailure('manual_compaction_baseline_invalid')
  }
  const items = compactionEvidenceItems(evidence)
  const evidenceCount = evidence && typeof evidence === 'object' &&
    Number.isSafeInteger(evidence.compactionCount)
    ? evidence.compactionCount
    : items.length
  const evidenceDigest = evidence && typeof evidence === 'object' &&
    typeof evidence.compactionsDigest === 'string'
    ? evidence.compactionsDigest
    : sha256(canonicalJSON(items))
  if (evidenceCount !== items.length || evidenceDigest !== sha256(canonicalJSON(items))) {
    return compactionProofFailure('manual_compaction_evidence_invalid')
  }
  const baselineSet = new Set(baseline.identityDigests)
  const newItems = items
    .map((item) => ({ item, digest: compactionIdentityDigest(item) }))
    .filter(({ digest }) => !baselineSet.has(digest))
  if (newItems.length === 0) {
    return compactionProofFailure('manual_compaction_baseline_replay')
  }
  const manual = newItems
    .map(({ item, digest }) => compactionProofForItem(item, digest))
    .find((proof) => proof.ok === true)
  if (manual) return manual
  const firstFailure = compactionProofForItem(newItems[0].item, newItems[0].digest)
  return firstFailure.ok ? compactionProofFailure('manual_compaction_schema_or_proof_invalid') : firstFailure
}

export function compactionWaitDisposition(evidence, { baseline } = {}) {
  if (!validCompactionBaseline(baseline)) return 'failed'
  const proof = manualCompactionProofEvidence(evidence, baseline)
  if (proof.ok) return 'completed'
  if (proof.reasonCode === 'manual_compaction_baseline_replay') return 'pending'
  const count = Number.isSafeInteger(evidence?.compactionCount)
    ? evidence.compactionCount
    : compactionEvidenceItems(evidence).length
  return count > baseline.count ? 'failed' : 'pending'
}

export function nonzeroCompactionFailureReasonCode() {
  return NONZERO_COMPACTION_FAILURE_REASON_CODE
}

function validCompactionStringList(value, allowEmpty = false) {
  if (!Array.isArray(value) || (!allowEmpty && value.length === 0)) return false
  const normalized = value.map((item) =>
    typeof item === 'string' ? item.trim() : ''
  )
  return normalized.every(Boolean) && new Set(normalized).size === normalized.length
}

function validCompactionItem(thread, turn, item) {
  if (!item || typeof item !== 'object' || Array.isArray(item)) return false
  const sourceDigest = typeof item.sourceDigest === 'string'
    ? item.sourceDigest.trim()
    : ''
  const proofRecord = { ...item }
  delete proofRecord.reasoningExclusionProof
  const schemaVersion = item.schemaVersion
  const providerHistoryProjectionVersion = item.providerHistoryProjectionVersion
  const generalProjectionValid =
    Number.isSafeInteger(schemaVersion) &&
    Number.isSafeInteger(providerHistoryProjectionVersion) &&
    ((schemaVersion === 3 && providerHistoryProjectionVersion === 1) ||
      (schemaVersion === 4 && providerHistoryProjectionVersion === 2 && item.auto === true))
  const caseProjectionValid = schemaVersion === 3 &&
    item.caseHistoryProjectionVersion === 2 &&
    providerHistoryProjectionVersion === undefined &&
    item.taskContinuation === undefined &&
    item.sourceContextDigest === undefined &&
    item.caseCompactionBinding === undefined &&
    Object.keys(item).length === CASE_COMPACTION_PUBLIC_KEYS.size &&
    Object.keys(item).every((key) => CASE_COMPACTION_PUBLIC_KEYS.has(key)) &&
    typeof item.auto === 'boolean'
  const projectionVersionValid = generalProjectionValid || caseProjectionValid
  const createdAt = Date.parse(String(item.createdAt || ''))
  const finishedAt = Date.parse(String(item.finishedAt || ''))
  return item.kind === 'compaction' &&
    item.role === 'system' &&
    item.status === 'completed' &&
    turn?.status === 'completed' &&
    typeof thread?.id === 'string' && thread.id.length > 0 &&
    item.threadId === thread.id &&
    item.turnId === turn?.id &&
    typeof item.id === 'string' && item.id.trim().length > 0 &&
    item.summary === (caseProjectionValid ? CASE_COMPACTION_SUMMARY : GENERAL_COMPACTION_SUMMARY) &&
    (caseProjectionValid
      ? item.replacedTokens === undefined
      : Number.isSafeInteger(item.replacedTokens) && item.replacedTokens > 0) &&
    (!caseProjectionValid || item.finishedAt === item.createdAt) &&
    (caseProjectionValid || item.auto === false) &&
    item.pinnedConstraints?.length === 1 &&
    item.pinnedConstraints[0] === 'user: preserve recent turns' &&
    /^[0-9a-f]{64}$/.test(sourceDigest) &&
    item.digestMarker === `sha256:${sourceDigest.slice(0, 12)}` &&
    validCompactionStringList(item.sourceItemIds, caseProjectionValid) &&
    (!caseProjectionValid || item.sourceItemIds.length === 0) &&
    projectionVersionValid &&
    item.reasoningExcluded === true &&
    item.assistantProseExcluded === true &&
    item.toolPayloadsExcluded === true &&
    item.caseFactsExcluded === true &&
    item.reasoningExclusionProof === `sha256:${sha256(canonicalJSON(proofRecord))}` &&
    Number.isFinite(createdAt) &&
    Number.isFinite(finishedAt) &&
    finishedAt >= createdAt
}

export function compactionItems(thread) {
  const items = []
  for (const turn of Array.isArray(thread?.turns) ? thread.turns : []) {
    for (const item of Array.isArray(turn?.items) ? turn.items : []) {
      if (validCompactionItem(thread, turn, item)) items.push(item)
    }
  }
  return items
}

export function exactCompactionRecoveryMatches(compactedEvidence, recoveredEvidence) {
  const compacted = Array.isArray(compactedEvidence?.compactions)
    ? compactedEvidence.compactions
    : []
  const recovered = Array.isArray(recoveredEvidence?.compactions)
    ? recoveredEvidence.compactions
    : []
  return compacted.length > 0 &&
    recovered.length === compacted.length &&
    recoveredEvidence?.compactionCount === compactedEvidence?.compactionCount &&
    recoveredEvidence?.compactionsDigest === compactedEvidence?.compactionsDigest &&
    /^[0-9a-f]{64}$/.test(String(compactedEvidence?.compactionsDigest || '')) &&
    compacted.every((item, index) =>
      recovered[index]?.id === item.id &&
      recovered[index]?.sourceDigest === item.sourceDigest &&
      sha256(canonicalJSON(recovered[index])) === sha256(canonicalJSON(item))
    )
}

function terminalThread(thread) {
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  return turns.length > 0 &&
    turns.every((turn) => ['completed', 'failed', 'aborted'].includes(turn?.status))
}

function continuitySubagentString(value) {
  return typeof value === 'string' ? value : ''
}

function continuitySubagentInteger(value) {
  return Number.isSafeInteger(value) ? value : null
}

function continuitySubagentBoolean(value) {
  return typeof value === 'boolean' ? value : null
}

// Continuation binds stable lineage, lifecycle, and withholding semantics;
// exact relaunch recovery continues to use the complete public summary digest.
export function subagentContinuityProjection(subagents) {
  const values = Array.isArray(subagents) ? subagents : []
  return values.map((item) => {
    const value = item && typeof item === 'object' && !Array.isArray(item) ? item : {}
    const diagnostics = value.diagnostics && typeof value.diagnostics === 'object' &&
        !Array.isArray(value.diagnostics)
      ? value.diagnostics
      : {}
    const background = value.background === undefined
      ? false
      : continuitySubagentBoolean(value.background)
    return {
      schemaVersion: value.schemaVersion === 1 ? 1 : null,
      id: continuitySubagentString(value.id),
      key: continuitySubagentString(value.key),
      parentThreadId: continuitySubagentString(value.parentThreadId),
      parentTurnId: continuitySubagentString(value.parentTurnId),
      parentToolCallId: continuitySubagentString(value.parentToolCallId),
      childId: continuitySubagentString(value.childId),
      childRunId: continuitySubagentString(value.childRunId),
      taskJobId: continuitySubagentString(value.taskJobId),
      taskKind: continuitySubagentString(value.taskKind),
      childThreadId: continuitySubagentString(value.childThreadId),
      childTurnId: continuitySubagentString(value.childTurnId),
      model: continuitySubagentString(value.model),
      providerId: continuitySubagentString(value.providerId),
      endpointFormat: continuitySubagentString(value.endpointFormat),
      variant: continuitySubagentString(value.variant),
      modelSource: continuitySubagentString(value.modelSource),
      effort: continuitySubagentString(value.effort),
      profile: continuitySubagentString(value.profile),
      toolPolicy: continuitySubagentString(value.toolPolicy),
      maxModelSteps: continuitySubagentInteger(value.maxModelSteps),
      timeBudgetMs: continuitySubagentInteger(value.timeBudgetMs),
      status: continuitySubagentString(value.status),
      rawStatus: continuitySubagentString(value.rawStatus),
      background,
      foreground: background === false,
      diagnostics: {
        status: continuitySubagentString(diagnostics.status),
        terminal: continuitySubagentBoolean(diagnostics.terminal),
        paused: continuitySubagentBoolean(diagnostics.paused),
        background: continuitySubagentBoolean(diagnostics.background)
      },
      parallelGroupId: continuitySubagentString(value.parallelGroupId),
      parallelIndex: continuitySubagentInteger(value.parallelIndex),
      canOpenThread: continuitySubagentBoolean(value.canOpenThread),
      canKill: continuitySubagentBoolean(value.canKill),
      canRestart: continuitySubagentBoolean(value.canRestart),
      outputWithheld: continuitySubagentBoolean(value.outputWithheld),
      outputTrustStatus: continuitySubagentString(value.outputTrustStatus),
      factAnswerAllowed: continuitySubagentBoolean(value.factAnswerAllowed),
      evidenceAuthority: continuitySubagentBoolean(value.evidenceAuthority),
      canReadOutput: continuitySubagentBoolean(value.canReadOutput),
      canContinueParent: continuitySubagentBoolean(value.canContinueParent)
    }
  })
}

export function subagentContinuityDigest(subagents) {
  return sha256(canonicalJSON(subagentContinuityProjection(subagents)))
}

export function exactlyOneSubagentContinuityMatches(firstEvidence, laterEvidence) {
  const firstSubagents = Array.isArray(firstEvidence?.subagents)
    ? firstEvidence.subagents
    : []
  const laterSubagents = Array.isArray(laterEvidence?.subagents)
    ? laterEvidence.subagents
    : []
  const firstDigest = typeof firstEvidence?.subagentContinuityDigest === 'string'
    ? firstEvidence.subagentContinuityDigest
    : ''
  const laterDigest = typeof laterEvidence?.subagentContinuityDigest === 'string'
    ? laterEvidence.subagentContinuityDigest
    : ''
  return firstEvidence?.boundedSubagentPresent === true &&
    firstEvidence?.boundedSubagentCount === 1 &&
    firstSubagents.length === 1 &&
    laterSubagents.length === 1 &&
    /^[0-9a-f]{64}$/u.test(firstDigest) &&
    /^[0-9a-f]{64}$/u.test(laterDigest) &&
    laterDigest === firstDigest
}

export function workflowEvidence(
  observation,
  workspace,
  resultMarker = RESULT_MARKER,
  exactResultTurnId = ''
) {
  const thread = observation?.thread
  const summary = observation?.summary
  const successfulExecutions = successfulToolExecutions(thread)
  const callAttempts = toolCallAttempts(thread)
  const toolNames = successfulExecutions.map((item) => item.toolName)
  const toolGroupPassed = Object.fromEntries(
    Object.entries(REQUIRED_TOOL_GROUPS).map(([group, aliases]) => [
      group,
      aliases.some((name) => toolNames.includes(name))
    ])
  )
  const assistantRecords = completedAssistantRecords(thread)
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  const exactTurns = typeof exactResultTurnId === 'string'
    ? turns.filter((turn) => turn?.id === exactResultTurnId)
    : []
  const exactTurnId = typeof exactResultTurnId === 'string' && exactResultTurnId &&
      exactResultTurnId === exactResultTurnId.trim() && exactResultTurnId.length <= 256 &&
      exactTurns.length === 1 && exactTurns[0]?.status === 'completed'
    ? exactResultTurnId
    : ''
  const markerRecord = resultMarker
    ? assistantRecords.find((item) =>
        item.text.includes(resultMarker) && (!exactTurnId || item.turnId === exactTurnId)
      )
    : undefined
  const markerTurnId = typeof markerRecord?.turnId === 'string' &&
      turns.filter((turn) => turn?.id === markerRecord.turnId).length === 1
    ? markerRecord.turnId
    : ''
  const terminalTurn = turns.at(-1)
  const terminalTurnId = !exactTurnId && !markerTurnId &&
      typeof terminalTurn?.id === 'string' && terminalTurn.id &&
      turns.filter((turn) => turn?.id === terminalTurn.id).length === 1 &&
      ['completed', 'failed', 'aborted'].includes(terminalTurn.status)
    ? terminalTurn.id
    : ''
  const boundTurnId = exactTurnId || markerTurnId || terminalTurnId
  const exactTurnRecords = boundTurnId
    ? assistantRecords.filter((item) => item.turnId === boundTurnId)
    : []
  const finalRecord = exactTurnRecords.at(-1) || markerRecord
  const finalText = finalRecord?.text || ''
  const resultTurnId = boundTurnId
  const resultTurnSuccessfulExecutions = successfulExecutions.filter((item) =>
    item.turnId === resultTurnId
  )
  const resultTurnToolCallAttempts = callAttempts.filter((item) =>
    item.turnId === resultTurnId
  )
  const resultTurnToolExecutionCounts = {}
  for (const execution of resultTurnSuccessfulExecutions) {
    resultTurnToolExecutionCounts[execution.toolName] =
      Number(resultTurnToolExecutionCounts[execution.toolName] || 0) + 1
  }
  const todos = Array.isArray(thread?.todos?.items) ? thread.todos.items : []
  const subagents = Array.isArray(summary?.subagents) ? summary.subagents : []
  const successfulSubagentCallIDs = new Set(
    resultTurnSuccessfulExecutions
      .filter((item) => REQUIRED_TOOL_GROUPS.subagent.includes(item.toolName))
      .map((item) => item.callId)
  )
  const formalExecutionBounds = milestoneAFormalExecutionBounds()
  const boundedSubagentPresent = subagents.length === 1 &&
    subagents.every((item) =>
      item &&
      item.parentThreadId === thread?.id &&
      item.parentTurnId === resultTurnId &&
      successfulSubagentCallIDs.has(item.parentToolCallId) &&
      item.status === 'done' &&
      item.rawStatus === 'completed' &&
      item.diagnostics?.status === 'completed' &&
      item.diagnostics?.terminal === true &&
      item.diagnostics?.paused === false &&
      item.diagnostics?.background === false &&
      item.background !== true &&
      item.profile === PACKAGED_READ_ONLY_SUBAGENT_PROFILE &&
      item.toolPolicy === 'readOnly' &&
      item.maxModelSteps === formalExecutionBounds.childMaxModelSteps &&
      item.timeBudgetMs === formalExecutionBounds.childTimeBudgetMs &&
      typeof item.childRunId === 'string' &&
      item.childRunId.length > 0 &&
      typeof item.childThreadId === 'string' &&
      item.childThreadId.length > 0 &&
      typeof item.childTurnId === 'string' &&
      item.childTurnId.length > 0 &&
      item.canOpenThread === true &&
      item.canReadOutput === false &&
      item.outputWithheld === true &&
      item.factAnswerAllowed === false
    )
  const compactions = compactionItems(thread)
  const providerAttempts = Array.isArray(observation?.providerAttempts)
    ? observation.providerAttempts
    : []
  const providerTerminals = Array.isArray(observation?.providerTerminals)
    ? observation.providerTerminals
    : []
  const providerReceiptTrace = providerReceiptTraceEvidence(
    observation?.providerReceiptTrace || emptyProviderReceiptTrace()
  )
  const ordinaryResultReceipt = ordinaryResultReceiptEvidence(
    observation?.ordinaryResultReceipt || emptyOrdinaryResultReceipt()
  )
  return {
    thread,
    summary,
    threadId: typeof thread?.id === 'string' ? thread.id : '',
    terminal: terminalThread(thread),
    workspaceBound: thread?.workspace === workspace,
    resultMarkerObserved: Boolean(resultMarker && markerRecord &&
      markerRecord.turnId === resultTurnId && finalText.includes(resultMarker)),
    resultText: finalText,
    researchMarkerObserved: finalText.includes(RESEARCH_MARKER),
    writingMarkerObserved: finalText.includes(WRITING_MARKER),
    resultTurnId,
    resultDigest: finalText ? sha256(finalText) : '',
    turnCount: Array.isArray(thread?.turns) ? thread.turns.length : 0,
    toolNames,
    toolNameSetDigest: sha256(canonicalJSON([...new Set(toolNames)].sort())),
    successfulToolExecutionDigest: sha256(canonicalJSON(successfulExecutions)),
    resultTurnSuccessfulExecutions,
    resultTurnSuccessfulToolExecutionDigest:
      sha256(canonicalJSON(resultTurnSuccessfulExecutions)),
    resultTurnToolCallAttempts,
    resultTurnToolCallAttemptDigest: sha256(canonicalJSON(resultTurnToolCallAttempts)),
    resultTurnToolExecutionCounts,
    toolGroupPassed,
    todos,
    todosDigest: sha256(canonicalJSON(todos)),
    todosPresent: todos.length > 0,
    todosTerminal: todos.length > 0 && todos.every((todo) => todo?.status === 'completed'),
    subagents,
    subagentsDigest: sha256(canonicalJSON(subagents)),
    subagentContinuityDigest: subagentContinuityDigest(subagents),
    boundedSubagentPresent,
    boundedSubagentCount: boundedSubagentPresent ? 1 : 0,
    providerAttempts,
    providerTerminals,
    ordinaryResultReceipt,
    providerReceiptTrace,
    compactions,
    compactionsDigest: sha256(canonicalJSON(compactions)),
    compactionCount: compactions.length
  }
}

const FAILED_CHILD_DIAGNOSTIC_REASON_CODES = Object.freeze([
  'failed_child_not_observed',
  'failed_child_input_invalid',
  'failed_child_task_attempt_count_invalid',
  'failed_child_summary_count_invalid',
  'failed_child_task_execution_observed',
  'failed_child_parent_thread_mismatch',
  'failed_child_parent_turn_mismatch',
  'failed_child_parent_call_mismatch',
  'failed_child_status_invalid',
  'failed_child_profile_invalid',
  'failed_child_foreground_invalid',
  'failed_child_output_boundary_invalid',
  'failed_child_run_missing',
  'failed_child_thread_missing',
  'failed_child_turn_invalid',
  'failed_child_thread_scope_invalid',
  'failed_child_turn_duplicate',
  'failed_child_turn_not_terminal',
  'failed_child_turn_not_latest',
  'failed_child_latest_terminal_missing',
  'failed_child_latest_terminal_ambiguous',
  'failed_child_terminal_diagnostic_invalid',
  'failed_child_observation_failed',
  'failed_child_turn_selected',
  'failed_child_latest_terminal_selected',
  'failed_child_selected'
])

const FAILED_CHILD_TERMINAL_STATUSES = new Set([
  'failed',
  'aborted',
  'interrupted',
  'killed',
  'canceled',
  'timeout'
])

function failedChildSafeId(value) {
  return typeof value === 'string' &&
    /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,255}$/u.test(value)
}

function failedChildDiagnosticTarget(workflow) {
  const taskAttempts = Array.isArray(workflow?.resultTurnToolCallAttempts)
    ? workflow.resultTurnToolCallAttempts.filter((item) => item?.toolName === 'task')
    : []
  const taskExecutions = Array.isArray(workflow?.resultTurnSuccessfulExecutions)
    ? workflow.resultTurnSuccessfulExecutions.filter((item) => item?.toolName === 'task')
    : []
  const subagents = Array.isArray(workflow?.summary?.subagents)
    ? workflow.summary.subagents
    : []
  const counts = {
    taskAttemptCount: taskAttempts.length,
    taskExecutionCount: taskExecutions.length,
    summaryCount: subagents.length
  }
  const target = {
    ok: false,
    reasonCode: 'failed_child_input_invalid',
    ...counts,
    statusBound: false,
    profileBound: false,
    foregroundBound: false,
    attempt: null,
    summary: null,
    parentThreadId: '',
    parentTurnId: '',
    parentToolCallId: '',
    childRunId: '',
    childThreadId: '',
    childTurnId: ''
  }
  const fail = (reasonCode) => ({ ...target, reasonCode })
  const threadId = typeof workflow?.threadId === 'string'
    ? workflow.threadId
    : workflow?.thread?.id
  const resultTurnId = workflow?.resultTurnId
  if (!failedChildSafeId(threadId) || !failedChildSafeId(resultTurnId)) {
    return fail('failed_child_input_invalid')
  }
  if (taskAttempts.length !== 1) return fail('failed_child_task_attempt_count_invalid')
  if (subagents.length !== 1) return fail('failed_child_summary_count_invalid')
  if (taskExecutions.length !== 0) return fail('failed_child_task_execution_observed')
  const attempt = taskAttempts[0]
  const summary = subagents[0]
  const parentThreadId = typeof summary?.parentThreadId === 'string'
    ? summary.parentThreadId
    : ''
  const parentTurnId = typeof summary?.parentTurnId === 'string'
    ? summary.parentTurnId
    : ''
  const parentToolCallId = typeof summary?.parentToolCallId === 'string'
    ? summary.parentToolCallId
    : ''
  Object.assign(target, {
    attempt,
    summary,
    parentThreadId,
    parentTurnId,
    parentToolCallId
  })
  if (parentThreadId !== threadId) return fail('failed_child_parent_thread_mismatch')
  if (parentTurnId !== resultTurnId || parentTurnId !== attempt?.turnId) {
    return fail('failed_child_parent_turn_mismatch')
  }
  if (!failedChildSafeId(parentToolCallId) || parentToolCallId !== attempt?.callId) {
    return fail('failed_child_parent_call_mismatch')
  }
  target.statusBound = summary?.status === 'terminal' &&
    FAILED_CHILD_TERMINAL_STATUSES.has(summary?.rawStatus) &&
    summary?.diagnostics?.status === summary?.rawStatus &&
    summary?.diagnostics?.terminal === true &&
    summary?.diagnostics?.paused === false
  if (!target.statusBound) return fail('failed_child_status_invalid')
  target.profileBound = summary?.profile === PACKAGED_READ_ONLY_SUBAGENT_PROFILE &&
    summary?.toolPolicy === 'readOnly'
  if (!target.profileBound) return fail('failed_child_profile_invalid')
  target.foregroundBound = summary?.background !== true &&
    summary?.diagnostics?.background === false
  if (!target.foregroundBound) return fail('failed_child_foreground_invalid')
  if (summary?.canOpenThread !== true || summary?.outputWithheld !== true ||
      summary?.canReadOutput !== false || summary?.factAnswerAllowed !== false) {
    return fail('failed_child_output_boundary_invalid')
  }
  const childRunId = typeof summary?.childRunId === 'string' ? summary.childRunId : ''
  target.childRunId = childRunId
  if (!failedChildSafeId(childRunId)) return fail('failed_child_run_missing')
  const childThreadId = typeof summary?.childThreadId === 'string'
    ? summary.childThreadId
    : ''
  target.childThreadId = childThreadId
  if (!failedChildSafeId(childThreadId)) return fail('failed_child_thread_missing')
  const childTurnId = summary?.childTurnId === undefined ? '' : summary.childTurnId
  target.childTurnId = childTurnId
  if (childTurnId && !failedChildSafeId(childTurnId)) return fail('failed_child_turn_invalid')
  return { ...target, ok: true, reasonCode: 'failed_child_selected' }
}

export function failedChildDiagnosticEvidence(workflow) {
  const target = failedChildDiagnosticTarget(workflow)
  const safeHash = (value) => failedChildSafeId(value) ? sha256(value) : ''
  return Object.freeze({
    observed: target.ok === true,
    reasonCode: FAILED_CHILD_DIAGNOSTIC_REASON_CODES.includes(target.reasonCode)
      ? target.reasonCode
      : 'failed_child_input_invalid',
    taskAttemptCount: target.taskAttemptCount,
    taskExecutionCount: target.taskExecutionCount,
    summaryCount: target.summaryCount,
    statusBound: target.statusBound === true,
    profileBound: target.profileBound === true,
    foregroundBound: target.foregroundBound === true,
    childTurnIdAvailable: target.ok === true && Boolean(target.childTurnId),
    parentThreadIdHash: safeHash(target.parentThreadId),
    parentTurnIdHash: safeHash(target.parentTurnId),
    parentToolCallIdHash: safeHash(target.parentToolCallId),
    childRunIdHash: safeHash(target.childRunId),
    childThreadIdHash: safeHash(target.childThreadId),
    childTurnIdHash: safeHash(target.childTurnId)
  })
}

function failedChildTerminalTurnTarget(childThread, expectedThreadId, expectedTurnId = '') {
  const fail = (reasonCode) => ({
    ok: false,
    reasonCode,
    turnId: ''
  })
  if (!childThread || typeof childThread !== 'object' || Array.isArray(childThread) ||
      !failedChildSafeId(expectedThreadId) || childThread.id !== expectedThreadId) {
    return fail('failed_child_thread_scope_invalid')
  }
  const turns = Array.isArray(childThread.turns) ? childThread.turns : []
  const terminal = (turn) => ['completed', 'failed', 'aborted'].includes(turn?.status)
  const latest = turns.at(-1)
  if (expectedTurnId) {
    if (!failedChildSafeId(expectedTurnId)) return fail('failed_child_turn_invalid')
    const exact = turns.filter((turn) => turn?.id === expectedTurnId)
    if (exact.length !== 1) return fail('failed_child_turn_duplicate')
    if (!terminal(exact[0])) return fail('failed_child_turn_not_terminal')
    if (latest?.id !== expectedTurnId) return fail('failed_child_turn_not_latest')
    return { ok: true, reasonCode: 'failed_child_turn_selected', turnId: expectedTurnId }
  }
  if (!terminal(latest) || !failedChildSafeId(latest?.id)) {
    return fail('failed_child_latest_terminal_missing')
  }
  const latestMatches = turns.filter((turn) => turn?.id === latest.id)
  if (latestMatches.length !== 1) return fail('failed_child_latest_terminal_ambiguous')
  return {
    ok: true,
    reasonCode: 'failed_child_latest_terminal_selected',
    turnId: latest.id
  }
}

export function failedChildTerminalTurnEvidence(childThread, expectedThreadId, expectedTurnId = '') {
  const target = failedChildTerminalTurnTarget(childThread, expectedThreadId, expectedTurnId)
  return Object.freeze({
    observed: target.ok === true,
    reasonCode: target.reasonCode,
    turnIdHash: failedChildSafeId(target.turnId) ? sha256(target.turnId) : ''
  })
}

function emptyFailedChildTerminalDiagnostic(reasonCode = 'failed_child_not_observed') {
  return Object.freeze({
    observed: false,
    reasonCode,
    turnIdHash: '',
    turnStatus: 'not_observed',
    turnErrorCode: 'none',
    turnErrorCodeHash: '',
    toolAttemptCount: 0,
    successfulToolExecutionCount: 0,
    failedToolResultCount: 0,
    unsettledToolResultCount: 0,
    terminalReasonClass: 'none',
    terminalErrorItemCount: 0,
    terminalErrorItemAuthorityBound: false,
    toolInventoryAvailability: 'not_observed',
    providerReceiptAvailability: 'not_observed',
    inventoryDigest: ''
  })
}

export function failedChildTerminalDiagnosticEvidence(
  childObservation,
  expectedThreadId,
  expectedTurnId = ''
) {
  const target = failedChildTerminalTurnTarget(
    childObservation?.thread,
    expectedThreadId,
    expectedTurnId
  )
  if (!target.ok) return emptyFailedChildTerminalDiagnostic(target.reasonCode)
  const childWorkflow = workflowEvidence(
    childObservation,
    '',
    '__no_parent_marker__'
  )
  const diagnostic = terminalWorkflowDiagnostic(childWorkflow, {
    previousTurnCount: Math.max(0, childWorkflow.turnCount - 1)
  })
  if (diagnostic.observed !== true ||
      diagnostic.turnIdHash !== sha256(target.turnId)) {
    return emptyFailedChildTerminalDiagnostic(
      'failed_child_terminal_diagnostic_invalid'
    )
  }
  return Object.freeze({
    observed: true,
    reasonCode: target.reasonCode,
    turnIdHash: diagnostic.turnIdHash,
    turnStatus: diagnostic.turnStatus,
    turnErrorCode: diagnostic.turnErrorCode,
    turnErrorCodeHash: diagnostic.turnErrorCodeHash,
    toolAttemptCount: diagnostic.toolAttemptCount,
    successfulToolExecutionCount: diagnostic.successfulToolExecutionCount,
    failedToolResultCount: diagnostic.failedToolResultCount,
    unsettledToolResultCount: diagnostic.unsettledToolResultCount,
    terminalReasonClass: diagnostic.terminalReasonClass,
    terminalErrorItemCount: diagnostic.terminalErrorItemCount,
    terminalErrorItemAuthorityBound: diagnostic.terminalErrorItemAuthorityBound,
    toolInventoryAvailability: diagnostic.toolInventoryAvailability,
    providerReceiptAvailability: diagnostic.providerReceiptAvailability,
    inventoryDigest: diagnostic.inventoryDigest
  })
}

export function exactTurnReasoningEffortBound(workflow, expectedEffort) {
  if (!['auto', 'off', 'low', 'medium', 'high', 'max'].includes(expectedEffort) ||
      !workflow?.thread ||
      typeof workflow.resultTurnId !== 'string' || !workflow.resultTurnId) {
    return false
  }
  const turns = Array.isArray(workflow.thread.turns) ? workflow.thread.turns : []
  const exactTurns = turns.filter((turn) => turn?.id === workflow.resultTurnId)
  return exactTurns.length === 1 &&
    exactTurns[0]?.status === 'completed' &&
    exactTurns[0]?.reasoningEffort === expectedEffort
}

export function workflowWaitDisposition(
  evidence,
  { previousTurnCount = 0, expectedThreadId = '' } = {}
) {
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0) {
    return 'terminal_without_expected_marker'
  }
  if (!evidence?.threadId ||
      (expectedThreadId && evidence.threadId !== expectedThreadId) ||
      !Number.isSafeInteger(evidence.turnCount) ||
      evidence.turnCount <= previousTurnCount) {
    return 'pending'
  }
  const turns = Array.isArray(evidence.thread?.turns) ? evidence.thread.turns : []
  const latestTurn = turns.at(-1)
  if (!latestTurn || !['completed', 'failed', 'aborted'].includes(latestTurn.status)) {
    return 'pending'
  }
  return latestTurn.status === 'completed' &&
      evidence.resultMarkerObserved === true &&
      evidence.resultTurnId === latestTurn.id
    ? 'completed'
    : 'terminal_without_expected_marker'
}

export function ordinaryWorkflowResultEvidence(evidence) {
  const empty = Object.freeze({
    bound: false,
    source: 'not_observed',
    resultMarkerObserved: false,
    researchMarkerObserved: false,
    writingMarkerObserved: false,
    textSha256: ''
  })
  const threadId = typeof evidence?.threadId === 'string'
    ? evidence.threadId.trim()
    : ''
  const turnId = typeof evidence?.resultTurnId === 'string'
    ? evidence.resultTurnId.trim()
    : ''
  if (!threadId || threadId !== evidence?.threadId ||
      !turnId || turnId !== evidence?.resultTurnId) return empty

  const snapshotTextSha256 = typeof evidence?.resultDigest === 'string' &&
      /^[0-9a-f]{64}$/u.test(evidence.resultDigest)
    ? evidence.resultDigest
    : ''
  const resultText = typeof evidence?.resultText === 'string'
    ? evidence.resultText
    : ''
  const turns = Array.isArray(evidence?.thread?.turns) ? evidence.thread.turns : []
  const snapshotTurnBound = turns.filter((turn) =>
    turn?.id === turnId && turn?.status === 'completed'
  ).length === 1
  const snapshotBound = snapshotTurnBound && Boolean(resultText) &&
    snapshotTextSha256 === sha256(resultText)
  const receipt = ordinaryResultReceiptEvidence(
    evidence?.ordinaryResultReceipt || emptyOrdinaryResultReceipt()
  )
  const terminals = Array.isArray(evidence?.providerTerminals)
    ? evidence.providerTerminals
    : []
  const typedTerminalBound = terminals.length === 1 &&
    terminals[0]?.threadId === threadId && terminals[0]?.turnId === turnId &&
    terminals[0]?.kind === 'turn_completed' && terminals[0]?.status === 'completed'
  const typedBound = receipt.observed === true &&
    receipt.candidateOrigin === 'provider_ordinary_only' &&
    receipt.projectionClass === 'provider_ordinary_only' &&
    typedTerminalBound

  if (receipt.observed === true && !typedBound) return empty
  if (snapshotBound && typedBound && snapshotTextSha256 !== receipt.textSha256) {
    return empty
  }
  if (typedBound) {
    return Object.freeze({
      bound: true,
      source: snapshotBound
        ? 'snapshot_and_typed_terminal_receipt'
        : 'typed_terminal_receipt',
      resultMarkerObserved: receipt.resultMarkerObserved,
      researchMarkerObserved: receipt.researchMarkerObserved,
      writingMarkerObserved: receipt.writingMarkerObserved,
      textSha256: receipt.textSha256
    })
  }
  if (!snapshotBound) return empty
  return Object.freeze({
    bound: true,
    source: 'public_thread_snapshot',
    resultMarkerObserved: evidence?.resultMarkerObserved === true,
    researchMarkerObserved: evidence?.researchMarkerObserved === true,
    writingMarkerObserved: evidence?.writingMarkerObserved === true,
    textSha256: snapshotTextSha256
  })
}

export function ordinaryWorkflowWaitDisposition(
  evidence,
  options = {}
) {
  const previousTurnCount = options?.previousTurnCount
  const expectedThreadId = typeof options?.expectedThreadId === 'string'
    ? options.expectedThreadId
    : ''
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0) {
    return 'terminal_without_bound_result'
  }
  if (!evidence?.threadId ||
      (expectedThreadId && evidence.threadId !== expectedThreadId) ||
      !Number.isSafeInteger(evidence.turnCount) ||
      evidence.turnCount <= previousTurnCount) return 'pending'
  const turns = Array.isArray(evidence?.thread?.turns) ? evidence.thread.turns : []
  const latestTurn = turns.at(-1)
  if (!latestTurn || !['completed', 'failed', 'aborted'].includes(latestTurn.status)) {
    return 'pending'
  }
  const result = ordinaryWorkflowResultEvidence(evidence)
  return latestTurn?.status === 'completed' &&
      evidence?.resultTurnId === latestTurn.id && result.bound
    ? 'completed'
    : 'terminal_without_bound_result'
}

export function ordinaryWorkflowObservedDisposition(
  workflow,
  options = {}
) {
  const evidenceDisposition = ordinaryWorkflowWaitDisposition(
    workflow?.evidence,
    options
  )
  if (evidenceDisposition !== 'pending') return evidenceDisposition
  return workflow?.disposition === 'timeout' ? 'timeout' : 'pending'
}

export function latestTerminalProviderReceiptScopeEvidence(
  evidence,
  { previousTurnCount = 0, baselineSeq = 0, expectedThreadId = '' } = {}
) {
  const thread = evidence?.thread
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  const threadId = typeof evidence?.threadId === 'string' ? evidence.threadId : ''
  const turn = turns.at(-1)
  const turnId = typeof turn?.id === 'string' ? turn.id.trim() : ''
  const highestSeq = thread?.latestSeq
  const rejected = (reasonCode) => Object.freeze({
    ok: false,
    reasonCode,
    scope: null
  })
  if (!thread || !threadId || thread?.id !== threadId) {
    return rejected('provider_receipt_scope_thread_invalid')
  }
  if (expectedThreadId && threadId !== expectedThreadId) {
    return rejected('provider_receipt_scope_thread_mismatch')
  }
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0 ||
      !Number.isSafeInteger(evidence?.turnCount) || evidence.turnCount !== turns.length) {
    return rejected('provider_receipt_scope_turn_count_invalid')
  }
  if (turns.length <= previousTurnCount) {
    return rejected('provider_receipt_scope_new_turn_unavailable')
  }
  if (!turnId || turnId !== turn.id || turnId.length > 256) {
    return rejected('provider_receipt_scope_turn_id_invalid')
  }
  if (!['completed', 'failed', 'aborted'].includes(turn.status)) {
    return rejected('provider_receipt_scope_turn_not_terminal')
  }
  if (!Number.isSafeInteger(baselineSeq) || baselineSeq < 0) {
    return rejected('provider_receipt_scope_baseline_cursor_invalid')
  }
  if (!Number.isSafeInteger(highestSeq) || highestSeq < 0) {
    return rejected('provider_receipt_scope_transport_cursor_invalid')
  }
  if (highestSeq <= baselineSeq) {
    return rejected('provider_receipt_scope_cursor_not_advanced')
  }
  const scope = Object.freeze({
    baselineSeq,
    expectedTurnId: turnId,
    expectedHighestSeq: highestSeq
  })
  return Object.freeze({
    ok: true,
    reasonCode: 'provider_receipt_scope_observed',
    scope
  })
}

export function exactLatestTerminalProviderReceiptScope(evidence, options = {}) {
  return latestTerminalProviderReceiptScopeEvidence(evidence, options).scope
}

const WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES = Object.freeze([
  'read',
  'plan',
  'todo',
  'write',
  'bash',
  'subagent',
  'skill',
  'ordinaryMcp',
  'fundsMcp',
  'other'
])

function workflowDiagnosticToolCategory(toolName) {
  if (toolName === 'read') return 'read'
  if (toolName === 'create_plan' || toolName === 'update_plan') return 'plan'
  if (toolName === 'todo_ops' || toolName === 'todo_write') return 'todo'
  if (['write', 'write_file', 'edit', 'edit_file', 'multi_edit', 'apply_patch'].includes(
    toolName
  )) return 'write'
  if (toolName === 'bash') return 'bash'
  if (PACKAGED_SUBAGENT_DELEGATION_TOOLS.includes(toolName)) return 'subagent'
  if (toolName === 'run_skill') return 'skill'
  if (fundsToolName(toolName)) return 'fundsMcp'
  if (typeof toolName === 'string' && toolName.startsWith('mcp__')) return 'ordinaryMcp'
  return 'other'
}

function emptyWorkflowDiagnosticCategoryCounts() {
  return Object.fromEntries(WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES.map((key) => [key, 0]))
}

function emptyProviderAttemptStatusCounts() {
  return {
    succeeded: 0,
    failed: 0,
    cancelled: 0,
    timedOut: 0,
    streamAborted: 0
  }
}

function workflowProviderReceiptCounters(providerAttempts) {
  const empty = {
    bound: false,
    logicalCallCount: 0,
    attemptCount: 0,
    attemptStatusCounts: emptyProviderAttemptStatusCounts()
  }
  if (!Array.isArray(providerAttempts) || providerAttempts.length !== 1) return empty
  const receipt = providerAttempts[0]
  const statuses = receipt?.providerAttemptStatuses
  const statusKeys = Object.keys(empty.attemptStatusCounts)
  if (receipt?.providerAttemptTelemetrySchema !== 'provider-attempt-telemetry.v1' ||
      receipt?.providerAttemptTelemetryValid !== true ||
      !Number.isSafeInteger(receipt?.providerLogicalCallCount) ||
      receipt.providerLogicalCallCount < 0 ||
      !Number.isSafeInteger(receipt?.providerAttemptCount) ||
      receipt.providerAttemptCount < 0 ||
      !statuses || typeof statuses !== 'object' || Array.isArray(statuses) ||
      !exactObjectKeys(statuses, statusKeys) ||
      statusKeys.some((key) =>
        !Number.isSafeInteger(statuses[key]) || statuses[key] < 0
      )) return empty
  const statusCount = statusKeys.reduce((total, key) => total + statuses[key], 0)
  if (receipt.providerLogicalCallCount > receipt.providerAttemptCount ||
      statusCount !== receipt.providerAttemptCount ||
      statuses.succeeded > receipt.providerLogicalCallCount) return empty
  return {
    bound: true,
    logicalCallCount: receipt.providerLogicalCallCount,
    attemptCount: receipt.providerAttemptCount,
    attemptStatusCounts: Object.fromEntries(statusKeys.map((key) => [key, statuses[key]]))
  }
}

function safeWorkflowDiagnosticCode(value, fallback = 'none') {
  return typeof value === 'string' && /^[a-z][a-z0-9_]{0,127}$/u.test(value)
    ? value
    : fallback
}

function workflowTerminalErrorCode(value, turnStatus) {
  if (turnStatus === 'completed') return 'none'
  const safe = safeWorkflowDiagnosticCode(value, '')
  if (safe) return safe
  if (turnStatus === 'aborted') return 'terminal_aborted'
  return 'terminal_error_unclassified'
}

function acceptedFinalProviderReceiptAvailability(terminalReason) {
  return terminalReason === 'success'
    ? 'accepted_final_sse_replay'
    : 'accepted_final_sse_failure_diagnostic'
}

function trustedTerminalErrorDiagnostic(turn, thread) {
  const items = Array.isArray(turn?.items) ? turn.items : []
  const errorItems = items.filter((item) => item?.kind === 'error')
  const view = turn?.acceptedFinalView
  const acceptedDigest = String(view?.acceptedFinalDigest || '')
  const terminalReason = safeWorkflowDiagnosticCode(view?.terminalReason, '')
  const deliveries = Array.isArray(thread?.acceptedFinalDeliveries)
    ? thread.acceptedFinalDeliveries
    : []
  const matchingDeliveries = deliveries.filter((candidate) =>
    candidate?.threadId === thread?.id && candidate?.turnId === turn?.id &&
    candidate?.publicationCommitId === acceptedDigest
  )
  const delivery = matchingDeliveries.length === 1 ? matchingDeliveries[0] : null
  const singularMatches = thread?.acceptedFinalDelivery === undefined || (
    deliveries.length === 1 &&
    canonicalJSON(thread.acceptedFinalDelivery) === canonicalJSON(delivery)
  )
  const acceptedFinalProjection = singularMatches && matchingDeliveries.length === 1 &&
    acceptedFinalDeliveryV2ShapeBound(delivery, thread?.id, turn?.id, view)
  let item = null
  if (acceptedFinalProjection && terminalReason) {
    const bound = errorItems.filter((candidate) =>
      candidate?.acceptedFinalDigest === acceptedDigest
    )
    if (bound.length === 1 && errorItems.length === 1 &&
        bound[0]?.code === `case_terminal_${terminalReason}` &&
        bound[0]?.status === turn?.status) {
      item = bound[0]
    }
  } else if (errorItems.length === 1 && errorItems[0]?.status === turn?.status &&
      safeWorkflowDiagnosticCode(errorItems[0]?.code, '') === errorItems[0]?.code) {
    item = errorItems[0]
  }
  const code = typeof item?.code === 'string' ? item.code : ''
  return {
    code,
    codeHash: code ? sha256(code) : '',
    terminalReasonClass: terminalReason || 'none',
    errorItemCount: errorItems.length,
    errorItemAuthorityBound: Boolean(item),
    toolInventoryAvailability: acceptedFinalProjection
      ? 'public_projection_withheld'
      : 'public_projection_available',
    providerReceiptAvailability: acceptedFinalProjection
      ? acceptedFinalProviderReceiptAvailability(terminalReason)
      : 'general_terminal_sse_replay'
  }
}

export function terminalWorkflowDiagnostic(
  evidence,
  { previousTurnCount = 0 } = {}
) {
  const empty = {
    observed: false,
    turnIdHash: '',
    turnStatus: 'not_observed',
    turnErrorCode: 'none',
    turnErrorCodeHash: '',
    toolAttemptCount: 0,
    successfulToolExecutionCount: 0,
    failedToolResultCount: 0,
    unsettledToolResultCount: 0,
    toolAttemptCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
    successfulToolCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
    providerAttemptRecordCount: 0,
    providerTerminalRecordCount: 0,
    providerReceiptCountersBound: false,
    providerLogicalCallCount: 0,
    providerAttemptCount: 0,
    providerAttemptStatusCounts: emptyProviderAttemptStatusCounts(),
    terminalReasonClass: 'none',
    terminalErrorItemCount: 0,
    terminalErrorItemAuthorityBound: false,
    toolInventoryAvailability: 'not_observed',
    providerReceiptAvailability: 'not_observed',
    ordinaryResultObserved: false,
    ordinaryResultReasonCode: 'ordinary_result_not_observed',
    ordinaryResultCandidateOrigin: 'not_observed',
    ordinaryResultProjectionClass: 'not_observed',
    ordinaryResultDigest: '',
    ordinaryResultTextSha256: '',
    ordinaryResultMarkerObserved: false,
    ordinaryResultResearchMarkerObserved: false,
    ordinaryResultWritingMarkerObserved: false,
    inventoryDigest: ''
  }
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0 ||
      !Number.isSafeInteger(evidence?.turnCount) || evidence.turnCount <= previousTurnCount) {
    return empty
  }
  const turns = Array.isArray(evidence?.thread?.turns) ? evidence.thread.turns : []
  const turn = turns.at(-1)
  const turnId = typeof turn?.id === 'string' ? turn.id : ''
  if (!turnId || !['completed', 'failed', 'aborted'].includes(turn?.status)) return empty

  const attempts = toolCallAttempts(evidence.thread).filter((item) => item.turnId === turnId)
  const successfulExecutions = successfulToolExecutions(evidence.thread)
    .filter((item) => item.turnId === turnId)
  const toolResults = (Array.isArray(turn.items) ? turn.items : [])
    .filter((item) => item?.kind === 'tool_result')
  const failedToolResultCount = toolResults.filter((item) =>
    item?.isError === true || item?.output?.status === 'failed'
  ).length
  const settledToolResultCount = toolResults.filter((item) =>
    (item?.status === 'completed' && (
      (item?.isError === false && item?.output?.status === 'completed') ||
      item?.isError === true || item?.output?.status === 'failed'
    )) ||
    (item?.status === 'failed' && item?.isError === true &&
      item?.output?.status === 'failed')
  ).length
  const attemptCounts = emptyWorkflowDiagnosticCategoryCounts()
  const successCounts = emptyWorkflowDiagnosticCategoryCounts()
  for (const attempt of attempts) {
    attemptCounts[workflowDiagnosticToolCategory(attempt.toolName)] += 1
  }
  for (const execution of successfulExecutions) {
    successCounts[workflowDiagnosticToolCategory(execution.toolName)] += 1
  }
  const providerAttempts = (Array.isArray(evidence?.providerAttempts)
    ? evidence.providerAttempts
    : []).filter((item) => item?.threadId === evidence.threadId && item?.turnId === turnId)
  const providerTerminals = (Array.isArray(evidence?.providerTerminals)
    ? evidence.providerTerminals
    : []).filter((item) => item?.threadId === evidence.threadId && item?.turnId === turnId)
  const providerReceiptCounters = workflowProviderReceiptCounters(providerAttempts)
  const terminalError = trustedTerminalErrorDiagnostic(turn, evidence.thread)
  const ordinaryResult = ordinaryResultReceiptEvidence(
    evidence?.ordinaryResultReceipt || emptyOrdinaryResultReceipt()
  )
  const rawTurnErrorCode = terminalError.code
  const turnErrorCode = workflowTerminalErrorCode(rawTurnErrorCode, turn.status)
  const inventory = {
    turnStatus: turn.status,
    turnErrorCode,
    turnErrorCodeHash: terminalError.codeHash,
    toolAttemptCount: attempts.length,
    successfulToolExecutionCount: successfulExecutions.length,
    failedToolResultCount,
    unsettledToolResultCount: toolResults.length - settledToolResultCount,
    toolAttemptCategoryCounts: attemptCounts,
    successfulToolCategoryCounts: successCounts,
    providerAttemptRecordCount: providerAttempts.length,
    providerTerminalRecordCount: providerTerminals.length,
    providerReceiptCountersBound: providerReceiptCounters.bound,
    providerLogicalCallCount: providerReceiptCounters.logicalCallCount,
    providerAttemptCount: providerReceiptCounters.attemptCount,
    providerAttemptStatusCounts: providerReceiptCounters.attemptStatusCounts,
    terminalReasonClass: terminalError.terminalReasonClass,
    terminalErrorItemCount: terminalError.errorItemCount,
    terminalErrorItemAuthorityBound: terminalError.errorItemAuthorityBound,
    toolInventoryAvailability: terminalError.toolInventoryAvailability,
    providerReceiptAvailability: terminalError.providerReceiptAvailability,
    ordinaryResultObserved: ordinaryResult.observed,
    ordinaryResultReasonCode: ordinaryResult.reasonCode,
    ordinaryResultCandidateOrigin: ordinaryResult.candidateOrigin,
    ordinaryResultProjectionClass: ordinaryResult.projectionClass,
    ordinaryResultDigest: ordinaryResult.resultDigest,
    ordinaryResultTextSha256: ordinaryResult.textSha256,
    ordinaryResultMarkerObserved: ordinaryResult.resultMarkerObserved,
    ordinaryResultResearchMarkerObserved: ordinaryResult.researchMarkerObserved,
    ordinaryResultWritingMarkerObserved: ordinaryResult.writingMarkerObserved
  }
  return {
    observed: true,
    turnIdHash: sha256(turnId),
    ...inventory,
    inventoryDigest: sha256(canonicalJSON(inventory))
  }
}

export function workflowProgressDiagnostic(
  evidence,
  { previousTurnCount = 0, expectedThreadId = '' } = {}
) {
  const empty = Object.freeze({
    observed: false,
    terminal: false,
    turnIdHash: '',
    turnStatus: 'not_observed',
    toolAttemptCount: 0,
    successfulToolExecutionCount: 0,
    failedToolResultCount: 0,
    unsettledToolResultCount: 0,
    openToolCallCount: 0,
    activeToolCategory: 'none',
    toolInventoryAvailability: 'not_observed',
    toolAttemptCategoryCounts: Object.freeze(emptyWorkflowDiagnosticCategoryCounts()),
    successfulToolCategoryCounts: Object.freeze(emptyWorkflowDiagnosticCategoryCounts()),
    providerAttemptRecordCount: 0,
    providerTerminalRecordCount: 0,
    providerReceiptCountersBound: false,
    providerLogicalCallCount: 0,
    providerAttemptCount: 0,
    providerAttemptStatusCounts: Object.freeze(emptyProviderAttemptStatusCounts()),
    inventoryDigest: ''
  })
  if (!Number.isSafeInteger(previousTurnCount) || previousTurnCount < 0 ||
      !Number.isSafeInteger(evidence?.turnCount) ||
      evidence.turnCount <= previousTurnCount || !evidence?.threadId ||
      evidence?.thread?.id !== evidence.threadId ||
      (expectedThreadId && evidence.threadId !== expectedThreadId)) return empty
  const turns = Array.isArray(evidence?.thread?.turns) ? evidence.thread.turns : []
  if (turns.length !== evidence.turnCount) return empty
  const turn = turns.at(-1)
  const turnId = typeof turn?.id === 'string' ? turn.id.trim() : ''
  const allowedStatuses = new Set([
    'queued', 'pending', 'running', 'waiting', 'completed', 'failed', 'aborted'
  ])
  if (!turnId || turnId !== turn?.id || turnId.length > 256 ||
      !allowedStatuses.has(turn?.status)) return empty

  const attempts = toolCallAttempts(evidence.thread).filter((item) =>
    item.turnId === turnId
  )
  const successfulExecutions = successfulToolExecutions(evidence.thread)
    .filter((item) => item.turnId === turnId)
  const toolResults = (Array.isArray(turn.items) ? turn.items : [])
    .filter((item) => item?.kind === 'tool_result')
  const failedToolResultCount = toolResults.filter((item) =>
    item?.isError === true || item?.output?.status === 'failed'
  ).length
  const settledToolResultCount = toolResults.filter((item) =>
    (item?.status === 'completed' && (
      (item?.isError === false && item?.output?.status === 'completed') ||
      item?.isError === true || item?.output?.status === 'failed'
    )) ||
    (item?.status === 'failed' && item?.isError === true &&
      item?.output?.status === 'failed')
  ).length
  const attemptCounts = emptyWorkflowDiagnosticCategoryCounts()
  const successCounts = emptyWorkflowDiagnosticCategoryCounts()
  for (const attempt of attempts) {
    attemptCounts[workflowDiagnosticToolCategory(attempt.toolName)] += 1
  }
  for (const execution of successfulExecutions) {
    successCounts[workflowDiagnosticToolCategory(execution.toolName)] += 1
  }
  const openAttempts = attempts.filter((attempt) => !toolResults.some((result) =>
    result?.callId === attempt.callId && result?.toolName === attempt.toolName &&
    result?.toolKind === attempt.toolKind
  ))
  const activeToolCategory = openAttempts.length > 0
    ? workflowDiagnosticToolCategory(openAttempts.at(-1).toolName)
    : 'none'
  const providerAttempts = (Array.isArray(evidence?.providerAttempts)
    ? evidence.providerAttempts
    : []).filter((item) => item?.threadId === evidence.threadId && item?.turnId === turnId)
  const providerTerminals = (Array.isArray(evidence?.providerTerminals)
    ? evidence.providerTerminals
    : []).filter((item) => item?.threadId === evidence.threadId && item?.turnId === turnId)
  const providerReceiptCounters = workflowProviderReceiptCounters(providerAttempts)
  const inventory = {
    terminal: ['completed', 'failed', 'aborted'].includes(turn.status),
    turnStatus: turn.status,
    toolAttemptCount: attempts.length,
    successfulToolExecutionCount: successfulExecutions.length,
    failedToolResultCount,
    unsettledToolResultCount: toolResults.length - settledToolResultCount,
    openToolCallCount: openAttempts.length,
    activeToolCategory,
    toolInventoryAvailability: 'public_projection_available',
    toolAttemptCategoryCounts: attemptCounts,
    successfulToolCategoryCounts: successCounts,
    providerAttemptRecordCount: providerAttempts.length,
    providerTerminalRecordCount: providerTerminals.length,
    providerReceiptCountersBound: providerReceiptCounters.bound,
    providerLogicalCallCount: providerReceiptCounters.logicalCallCount,
    providerAttemptCount: providerReceiptCounters.attemptCount,
    providerAttemptStatusCounts: providerReceiptCounters.attemptStatusCounts
  }
  return Object.freeze({
    observed: true,
    turnIdHash: sha256(turnId),
    ...inventory,
    toolAttemptCategoryCounts: Object.freeze({ ...attemptCounts }),
    successfulToolCategoryCounts: Object.freeze({ ...successCounts }),
    providerAttemptStatusCounts: Object.freeze({
      ...providerReceiptCounters.attemptStatusCounts
    }),
    inventoryDigest: sha256(canonicalJSON(inventory))
  })
}

export function planningTurnDiagnosticProjection(
  evidence,
  {
    previousTurnCount = 0,
    baselineSeq = 0,
    expectedThreadId = '',
    disposition = 'pending'
  } = {}
) {
  const diagnostic = terminalWorkflowDiagnostic(evidence, { previousTurnCount })
  const receiptScope = latestTerminalProviderReceiptScopeEvidence(evidence, {
    previousTurnCount,
    baselineSeq,
    expectedThreadId
  })
  const safeDisposition = [
    'pending',
    'completed',
    'terminal_without_expected_marker',
    'timeout'
  ].includes(disposition)
    ? disposition
    : 'invalid'
  return Object.freeze({
    planWorkflowDisposition: safeDisposition,
    planCandidateTurnObserved: diagnostic.observed,
    planCandidateTurnStatus: diagnostic.turnStatus,
    planCandidateTurnIdHash: diagnostic.turnIdHash,
    planCandidateTurnErrorCode: diagnostic.turnErrorCode,
    planCandidateTurnErrorCodeHash: diagnostic.turnErrorCodeHash,
    planCandidateToolAttemptCount: diagnostic.toolAttemptCount,
    planCandidateSuccessfulToolExecutionCount:
      diagnostic.successfulToolExecutionCount,
    planCandidateFailedToolResultCount: diagnostic.failedToolResultCount,
    planCandidateUnsettledToolResultCount: diagnostic.unsettledToolResultCount,
    planCandidateToolAttemptCategoryCounts: Object.freeze({
      ...diagnostic.toolAttemptCategoryCounts
    }),
    planCandidateSuccessfulToolCategoryCounts: Object.freeze({
      ...diagnostic.successfulToolCategoryCounts
    }),
    planCandidateProviderAttemptRecordCount: diagnostic.providerAttemptRecordCount,
    planCandidateProviderTerminalRecordCount: diagnostic.providerTerminalRecordCount,
    planCandidateProviderReceiptCountersBound:
      diagnostic.providerReceiptCountersBound,
    planCandidateProviderLogicalCallCount: diagnostic.providerLogicalCallCount,
    planCandidateProviderAttemptCount: diagnostic.providerAttemptCount,
    planCandidateProviderAttemptStatusCounts: Object.freeze({
      ...diagnostic.providerAttemptStatusCounts
    }),
    planCandidateTerminalReasonClass: diagnostic.terminalReasonClass,
    planCandidateTerminalErrorItemCount: diagnostic.terminalErrorItemCount,
    planCandidateTerminalErrorItemAuthorityBound:
      diagnostic.terminalErrorItemAuthorityBound,
    planCandidateToolInventoryAvailability: diagnostic.toolInventoryAvailability,
    planCandidateProviderReceiptAvailability:
      diagnostic.providerReceiptAvailability,
    planCandidateInventoryDigest: diagnostic.inventoryDigest,
    planProviderReceiptScopeBound: receiptScope.ok,
    planProviderReceiptScopeReasonCode: receiptScope.reasonCode
  })
}

export const CONTINUATION_ACCEPTED_FINAL_REASON_CODES = Object.freeze([
  'input_invalid',
  'turn_invalid',
  'assistant_invalid',
  'marker_invalid',
  'accepted_final_invalid',
  'delivery_invalid',
  'provider_closure_invalid',
  'observed'
])

function continuationAcceptedFinalBase(requiredMarkerCount = 0) {
  return {
    ok: false,
    reasonCode: 'input_invalid',
    inputValid: false,
    turnBound: false,
    assistantBound: false,
    markerBound: false,
    acceptedFinalBound: false,
    deliveryBound: false,
    providerClosureBound: false,
    threadId: '',
    turnId: '',
    acceptedFinalDigest: '',
    renderedTextSha256: '',
    deliveryManifestDigest: '',
    providerClosureDigest: '',
    requiredMarkerCount
  }
}

export function acceptedFinalOrdinaryContinuationEvidence(
  observation,
  {
    expectedThreadId = '',
    expectedTurnId = '',
    requiredMarkers = [],
    provider = null
  } = {}
) {
  const markers = Array.isArray(requiredMarkers) ? requiredMarkers : []
  const base = continuationAcceptedFinalBase(markers.length)
  const inputValid = typeof expectedThreadId === 'string' &&
    expectedThreadId === expectedThreadId.trim() && expectedThreadId.length > 0 &&
    typeof expectedTurnId === 'string' &&
    expectedTurnId === expectedTurnId.trim() && expectedTurnId.length > 0 &&
    markers.every((marker) =>
      typeof marker === 'string' && marker === marker.trim() && marker.length > 0 &&
      marker.length <= 256 && !/[\0\r\n]/u.test(marker)
    ) && provider?.ok === true &&
    typeof provider.id === 'string' && provider.id.length > 0 &&
    typeof provider.model === 'string' && provider.model.length > 0
  if (!inputValid) return base
  const thread = observation?.thread
  const turns = Array.isArray(thread?.turns) ? thread.turns : []
  const turn = expectedTurnId
    ? turns.find((candidate) => candidate?.id === expectedTurnId)
    : turns.at(-1)
  const items = Array.isArray(turn?.items) ? turn.items : []
  const assistants = items.filter((item) => item?.kind === 'assistant_text')
  const view = turn?.acceptedFinalView
  const digest = String(view?.acceptedFinalDigest || '')
  const assistant = assistants.find((item) =>
    item?.threadId === thread?.id && item?.turnId === turn?.id
  )
  const text = typeof assistant?.text === 'string' ? assistant.text : ''
  const deliveries = Array.isArray(thread?.acceptedFinalDeliveries)
    ? thread.acceptedFinalDeliveries
    : []
  const matchingDeliveries = deliveries.filter((candidate) =>
    candidate?.threadId === thread?.id && candidate?.turnId === turn?.id &&
    candidate?.publicationCommitId === digest
  )
  const delivery = matchingDeliveries.length === 1 ? matchingDeliveries[0] : null
  const visibleAcceptedTurns = turns.filter((candidate) => candidate?.acceptedFinalView != null)
  const deliveryInventoryBound = deliveries.length === visibleAcceptedTurns.length &&
    new Set(deliveries.map((candidate) => candidate?.batchId)).size === deliveries.length &&
    new Set(deliveries.map((candidate) => candidate?.publicationAuthority?.sealId)).size ===
      deliveries.length &&
    new Set(deliveries.map((candidate) => candidate?.publicationCommitId)).size ===
      deliveries.length &&
    visibleAcceptedTurns.every((candidate) => {
      const candidateView = candidate.acceptedFinalView
      const matches = deliveries.filter((candidateDelivery) =>
        candidateDelivery?.threadId === thread?.id &&
        candidateDelivery?.turnId === candidate?.id &&
        candidateDelivery?.publicationCommitId === candidateView?.acceptedFinalDigest
      )
      return matches.length === 1 && acceptedFinalDeliveryV2ShapeBound(
        matches[0],
        thread?.id,
        candidate?.id,
        candidateView
      )
    })
  const singularAliasBound = thread?.acceptedFinalDelivery === undefined || (
    deliveries.length === 1 &&
    canonicalJSON(thread.acceptedFinalDelivery) === canonicalJSON(delivery)
  )
  const events = Array.isArray(delivery?.events) ? delivery.events : []
  const assistantEvent = events.find((event) => event?.publicationSlot === 'assistant-final')
  const usageEvent = events.find((event) => event?.publicationSlot === 'usage')
  const terminalEvent = events.find((event) => event?.publicationSlot === 'terminal')
  const diagnostics = usageEvent?.cacheDiagnostics
  const statuses = diagnostics?.providerAttemptStatuses
  const statusValues = statuses && typeof statuses === 'object' && !Array.isArray(statuses)
    ? ['succeeded', 'failed', 'cancelled', 'timedOut', 'streamAborted']
      .map((key) => statuses[key])
    : []
  const attemptStatusCount = statusValues.length === 5 &&
    statusValues.every((value) => Number.isSafeInteger(value) && value >= 0)
    ? statusValues.reduce((total, value) => total + value, 0)
    : -1
  const logicalCalls = diagnostics?.providerLogicalCallCount
  const attempts = diagnostics?.providerAttemptCount
  const usage = usageEvent?.usage
  const providerBound = provider?.ok === true &&
    thread?.providerId === provider.id && thread?.model === provider.model &&
    usageEvent?.model === provider.model &&
    (usageEvent?.providerId === undefined || usageEvent.providerId === provider.id)
  const providerClosure = providerBound &&
    diagnostics?.providerAttemptTelemetrySchema === 'provider-attempt-telemetry.v1' &&
    diagnostics?.providerAttemptTelemetryValid === true &&
    Object.keys(statuses || {}).sort().join(',') ===
      'cancelled,failed,streamAborted,succeeded,timedOut' &&
    Number.isSafeInteger(logicalCalls) && logicalCalls > 0 &&
    Number.isSafeInteger(attempts) && attempts >= logicalCalls &&
    attemptStatusCount === attempts && statuses?.succeeded === logicalCalls &&
    Number.isSafeInteger(usage?.promptTokens) && usage.promptTokens > 0 &&
    Number.isSafeInteger(usage?.completionTokens) && usage.completionTokens > 0 &&
    Number.isSafeInteger(usage?.totalTokens) &&
    usage.totalTokens === usage.promptTokens + usage.completionTokens &&
    usage?.turns === 1 &&
    usageEvent?.usageFinalStatus === 'completed'
  const publicationAuthority = delivery?.publicationAuthority
  const eventsBound = events.length === 3 &&
    events[0] === assistantEvent && events[1] === usageEvent && events[2] === terminalEvent &&
    events.every((event, index) =>
      event?.threadId === thread?.id && event?.turnId === turn?.id &&
      event?.seq === delivery?.firstSeq + index &&
      event?.acceptedFinalDigest === digest && event?.publicationCommitId === digest
    )
  const deliveryBound = deliveryInventoryBound && singularAliasBound &&
    matchingDeliveries.length === 1 &&
    delivery?.schemaVersion === 2 &&
    delivery?.purpose === 'analytix.accepted-final-delivery-batch/v2' &&
    delivery?.kind === 'accepted_final_batch' &&
    delivery?.threadId === thread?.id && delivery?.turnId === turn?.id &&
    delivery?.publicationCommitId === digest &&
    /^[0-9a-f]{64}$/.test(String(delivery?.batchId || '')) &&
    /^[0-9a-f]{64}$/.test(String(delivery?.eventManifestDigest || '')) &&
    Number.isSafeInteger(delivery?.firstSeq) && delivery.firstSeq > 0 &&
    Number.isSafeInteger(delivery?.lastSeq) && delivery.lastSeq === delivery.seq &&
    delivery.lastSeq === thread?.latestSeq &&
    delivery.lastSeq - delivery.firstSeq + 1 === events.length &&
    eventsBound &&
    publicationAuthority?.schemaVersion === 'accepted-final-delivery-seal.v1' &&
    publicationAuthority?.purpose === 'analytix.accepted-final-delivery-seal/v1' &&
    publicationAuthority?.threadId === thread?.id &&
    publicationAuthority?.turnId === turn?.id &&
    publicationAuthority?.publicationCommitId === digest &&
    publicationAuthority?.eventManifestDigest === delivery.eventManifestDigest &&
    publicationAuthority?.batchId === delivery.batchId &&
    publicationAuthority?.firstSeq === delivery.firstSeq &&
    publicationAuthority?.lastSeq === delivery.lastSeq &&
    publicationAuthority?.timestamp === delivery.timestamp &&
    publicationAuthority?.authorityAlgorithm === 'Ed25519' &&
    /^[0-9a-f]{64}$/.test(String(publicationAuthority?.acceptedFinalDispositionDigest || '')) &&
    /^[0-9a-f]{64}$/.test(String(publicationAuthority?.terminalDispositionId || '')) &&
    /^[0-9a-f]{64}$/.test(String(publicationAuthority?.sequencedEventsDigest || '')) &&
    /^[0-9a-f]{64}$/.test(String(publicationAuthority?.authorityKeyId || '')) &&
    /^[A-Za-z0-9_-]{43}$/.test(String(publicationAuthority?.authorityPublicKey || '')) &&
    /^[A-Za-z0-9_-]{86}$/.test(String(publicationAuthority?.authoritySignature || '')) &&
    /^[0-9a-f]{64}$/.test(String(publicationAuthority?.sealId || '')) &&
    assistantEvent?.acceptedFinalDigest === digest &&
    assistantEvent?.publicationCommitId === digest &&
    assistantEvent?.item?.acceptedFinal === undefined &&
    acceptedFinalPublicViewV3ShapeValid(
      assistantEvent?.item?.acceptedFinalView,
      digest,
      delivery?.timestamp
    ) &&
    canonicalJSON(assistantEvent.item.acceptedFinalView) === canonicalJSON(view) &&
    assistantEvent?.item?.text === text &&
    usageEvent?.acceptedFinalDigest === digest &&
    usageEvent?.publicationCommitId === digest &&
    terminalEvent?.acceptedFinalDigest === digest &&
    terminalEvent?.publicationCommitId === digest &&
    terminalEvent?.kind === 'turn_completed' && terminalEvent?.status === 'completed' &&
    terminalEvent?.terminalReason === 'success'
  const latestTurnIdBound = thread?.latestTurnId === undefined ||
    thread.latestTurnId === turn?.id
  const turnBound = Boolean(thread?.id && thread?.historyAuthority === 'case_boundary_only_v1' &&
    (!expectedThreadId || thread.id === expectedThreadId) &&
    turn === turns.at(-1) && latestTurnIdBound &&
    thread?.turnCount === turns.length &&
    turn?.status === 'completed' && (!expectedTurnId || turn.id === expectedTurnId))
  const assistantBound = Boolean(turnBound && items.length === 1 && assistants.length === 1 &&
    exactObjectKeys(assistant, [
      'id', 'turnId', 'threadId', 'role', 'status', 'createdAt', 'finishedAt',
      'kind', 'text', 'acceptedFinalView'
    ]) &&
    assistant?.threadId === thread.id && assistant?.turnId === turn.id &&
    assistant?.role === 'assistant' && assistant?.status === 'completed' &&
    assistant?.acceptedFinal === undefined && assistant?.acceptedFinalView &&
    canonicalJSON(assistant.acceptedFinalView) === canonicalJSON(view))
  const acceptedFinalBound = Boolean(assistantBound &&
    acceptedFinalPublicViewV3ShapeValid(view, digest, assistant.finishedAt) &&
    assistant.acceptedFinalView.acceptedFinalDigest === digest &&
    /^[0-9a-f]{64}$/.test(digest) &&
    view?.schemaVersion === 3 && view?.publicationState === 'accepted' &&
    view?.acceptedFinalDigest === digest && view?.terminalReason === 'success' &&
    view?.variant === 'NeedsEvidenceAnswer' &&
    view?.coverageStatus === 'unverified' &&
    view?.claimCount === 0 && Array.isArray(view?.claimTypes) && view.claimTypes.length === 0 &&
    view?.receiptMetadata?.projection === 'masked_metadata_only' &&
    view?.receiptMetadata?.count === 0 &&
    Array.isArray(view?.receiptMetadata?.citations) && view.receiptMetadata.citations.length === 0)
  const markerBound = Boolean(assistantBound && markers.every((marker) =>
    text.includes(marker)
  ))
  const providerClosureBound = Boolean(providerClosure)
  const reasonCode = !turnBound
    ? 'turn_invalid'
    : !assistantBound
      ? 'assistant_invalid'
      : !markerBound
        ? 'marker_invalid'
        : !acceptedFinalBound
          ? 'accepted_final_invalid'
          : !deliveryBound
            ? 'delivery_invalid'
            : !providerClosureBound
              ? 'provider_closure_invalid'
              : 'observed'
  const ok = reasonCode === 'observed'
  return {
    ...base,
    ok,
    reasonCode,
    inputValid: true,
    turnBound,
    assistantBound,
    markerBound,
    acceptedFinalBound,
    deliveryBound,
    providerClosureBound,
    threadId: typeof thread?.id === 'string' ? thread.id : '',
    turnId: typeof turn?.id === 'string' ? turn.id : '',
    acceptedFinalDigest: ok ? digest : '',
    renderedTextSha256: ok ? sha256(text) : '',
    deliveryManifestDigest: ok ? delivery.eventManifestDigest : '',
    providerClosureDigest: ok ? sha256(canonicalJSON({
      threadIdHash: sha256(thread.id),
      turnIdHash: sha256(turn.id),
      acceptedFinalDigest: digest,
      eventManifestDigest: delivery.eventManifestDigest,
      providerId: provider.id,
      model: usageEvent.model,
      providerLogicalCallCount: logicalCalls,
      providerAttemptCount: attempts,
      providerAttemptStatuses: statuses
    })) : ''
  }
}

const ORDINARY_WORKFLOW_FAILURE_CLASSIFICATIONS = Object.freeze([
  'not_observed',
  'host_post_response_failure',
  'provider_transport_succeeded_terminal_failure',
  'provider_pipeline_failure',
  'tool_failure',
  'invalid'
])

const HOST_POST_RESPONSE_FAILURE_CODES = new Set([
  'host_response_event_failed',
  'host_candidate_lifecycle_failed',
  'host_candidate_authority_failed',
  'host_candidate_steering_failed',
  'host_candidate_projection_failed',
  'host_candidate_publication_failed'
])

const PROVIDER_OUTPUT_POLICY_FAILURE_CODES = new Set([
  'provider_tool_arguments_invalid',
  'provider_reasoning_markup_invalid',
  'provider_empty_final'
])

const ORDINARY_WORKFLOW_FAILURE_PROJECTION_OBSERVATION_CODES = Object.freeze([
  'workflow_diagnostic_not_observed',
  'workflow_failure_diagnostic_invalid',
  'host_failure_phase_observed',
  'provider_output_policy_failure_observed',
  'provider_failure_diagnostic_missing',
  'provider_failure_observed',
  'provider_failure_terminal_observed',
  'tool_failure_not_observed',
  'tool_failure_observed'
])

const ORDINARY_WORKFLOW_TERMINAL_DIAGNOSTIC_KEYS = Object.freeze([
  'observed', 'turnIdHash', 'turnStatus', 'turnErrorCode', 'turnErrorCodeHash',
  'toolAttemptCount', 'successfulToolExecutionCount', 'failedToolResultCount',
  'unsettledToolResultCount', 'toolAttemptCategoryCounts',
  'successfulToolCategoryCounts', 'providerAttemptRecordCount',
  'providerTerminalRecordCount', 'providerReceiptCountersBound',
  'providerLogicalCallCount', 'providerAttemptCount',
  'providerAttemptStatusCounts', 'terminalReasonClass', 'terminalErrorItemCount',
  'terminalErrorItemAuthorityBound', 'toolInventoryAvailability',
  'providerReceiptAvailability', 'ordinaryResultObserved',
  'ordinaryResultReasonCode', 'ordinaryResultCandidateOrigin',
  'ordinaryResultProjectionClass', 'ordinaryResultDigest',
  'ordinaryResultTextSha256', 'ordinaryResultMarkerObserved',
  'ordinaryResultResearchMarkerObserved', 'ordinaryResultWritingMarkerObserved',
  'inventoryDigest'
])

const ORDINARY_WORKFLOW_PROVIDER_DIAGNOSTIC_KEYS = Object.freeze([
  'observed', 'observationCode', 'reasonCode', 'providerKind', 'providerStatus',
  'providerRetryable', 'providerAuthStatus', 'diagnosticDigest'
])

const ORDINARY_WORKFLOW_TOOL_DIAGNOSTIC_KEYS = Object.freeze([
  'observed', 'observationCode', 'evidenceCode', 'toolFailureGuardBound',
  'guardCount', 'maxStormCount', 'guardKind', 'toolCategory', 'toolNameHash',
  'invalidArgumentGuardCount', 'invalidArgumentMaxStormCount',
  'invalidArgumentToolCategory', 'invalidArgumentToolNameHash',
  'terminalReason', 'terminalCode', 'diagnosticDigest'
])

function ordinaryWorkflowFailureProjectionBase() {
  return {
    classification: 'invalid',
    observationCode: 'workflow_failure_diagnostic_invalid',
    providerTransportSucceeded: false,
    upstreamCauseConfirmed: false,
    diagnosticDigest: ''
  }
}

function ordinaryWorkflowFailureDigest(input) {
  return sha256(canonicalJSON({
    classification: input.classification,
    observationCode: input.observationCode,
    providerTransportSucceeded: input.providerTransportSucceeded,
    upstreamCauseConfirmed: input.upstreamCauseConfirmed,
    terminalDiagnosticDigest: input.terminalDiagnosticDigest,
    providerDiagnosticDigest: input.providerDiagnosticDigest,
    toolDiagnosticDigest: input.toolDiagnosticDigest
  }))
}

function ordinaryWorkflowFailureProjectionResult({
  classification,
  observationCode,
  providerTransportSucceeded,
  upstreamCauseConfirmed,
  terminalDiagnosticDigest,
  providerDiagnosticDigest,
  toolDiagnosticDigest
}) {
  if (!ORDINARY_WORKFLOW_FAILURE_CLASSIFICATIONS.includes(classification) ||
      !ORDINARY_WORKFLOW_FAILURE_PROJECTION_OBSERVATION_CODES.includes(observationCode)) {
    return Object.freeze(ordinaryWorkflowFailureProjectionBase())
  }
  const projected = {
    classification,
    observationCode,
    providerTransportSucceeded,
    upstreamCauseConfirmed,
    diagnosticDigest: classification === 'not_observed'
      ? ''
      : ordinaryWorkflowFailureDigest({
          classification,
          observationCode,
          providerTransportSucceeded,
          upstreamCauseConfirmed,
          terminalDiagnosticDigest,
          providerDiagnosticDigest,
          toolDiagnosticDigest
        })
  }
  return Object.freeze(projected)
}

function ordinaryWorkflowFailureNonnegativeInteger(value) {
  return Number.isSafeInteger(value) && value >= 0
}

function ordinaryWorkflowFailureDigestValid(value, { empty = false } = {}) {
  return empty
    ? value === ''
    : typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)
}

function ordinaryWorkflowFailureCategoryCountsValid(value) {
  return exactObjectKeys(value, WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES) &&
    WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES.every((key) =>
      ordinaryWorkflowFailureNonnegativeInteger(value[key])
    )
}

function ordinaryWorkflowFailureProviderStatusCountsValid(value) {
  const keys = ['succeeded', 'failed', 'cancelled', 'timedOut', 'streamAborted']
  return exactObjectKeys(value, keys) && keys.every((key) =>
    ordinaryWorkflowFailureNonnegativeInteger(value[key])
  )
}

function ordinaryWorkflowFailureTerminalDiagnosticValid(value) {
  const terminalErrorShapeValid = value?.turnStatus === 'completed'
    ? value?.turnErrorCode === 'none' && value?.turnErrorCodeHash === ''
    : typeof value?.turnErrorCode === 'string' &&
      /^[a-z][a-z0-9_]{0,127}$/u.test(value.turnErrorCode) &&
      ordinaryWorkflowFailureDigestValid(value.turnErrorCodeHash)
  if (!exactObjectKeys(value, ORDINARY_WORKFLOW_TERMINAL_DIAGNOSTIC_KEYS) ||
      value.observed !== true || !['completed', 'failed', 'aborted'].includes(value.turnStatus) ||
      !terminalErrorShapeValid ||
      !ordinaryWorkflowFailureDigestValid(value.turnIdHash) ||
      !ordinaryWorkflowFailureCategoryCountsValid(value.toolAttemptCategoryCounts) ||
      !ordinaryWorkflowFailureCategoryCountsValid(value.successfulToolCategoryCounts) ||
      !ordinaryWorkflowFailureProviderStatusCountsValid(value.providerAttemptStatusCounts) ||
      !ordinaryWorkflowFailureDigestValid(value.inventoryDigest) ||
      ![true, false].includes(value.providerReceiptCountersBound) ||
      ![true, false].includes(value.terminalErrorItemAuthorityBound) ||
      ![true, false].includes(value.ordinaryResultObserved) ||
      ![true, false].includes(value.ordinaryResultMarkerObserved) ||
      ![true, false].includes(value.ordinaryResultResearchMarkerObserved) ||
      ![true, false].includes(value.ordinaryResultWritingMarkerObserved) ||
      typeof value.ordinaryResultReasonCode !== 'string' ||
      typeof value.ordinaryResultCandidateOrigin !== 'string' ||
      typeof value.ordinaryResultProjectionClass !== 'string' ||
      !ORDINARY_RESULT_RECEIPT_REASON_CODES.includes(value.ordinaryResultReasonCode) ||
      !['not_observed', 'provider_ordinary_only', 'host_fixed_provider_result_withheld',
        'host_fixed_protected_fact_blocked'].includes(value.ordinaryResultProjectionClass) ||
      !['not_observed', 'provider_ordinary_only', 'host_fixed'].includes(
        value.ordinaryResultCandidateOrigin
      ) ||
      !ordinaryWorkflowFailureDigestValid(value.ordinaryResultDigest, {
        empty: !value.ordinaryResultObserved
      }) ||
      !ordinaryWorkflowFailureDigestValid(value.ordinaryResultTextSha256, {
        empty: !value.ordinaryResultObserved
      })) return false
  const integerKeys = [
    'toolAttemptCount', 'successfulToolExecutionCount', 'failedToolResultCount',
    'unsettledToolResultCount', 'providerAttemptRecordCount',
    'providerTerminalRecordCount', 'providerLogicalCallCount',
    'providerAttemptCount', 'terminalErrorItemCount'
  ]
  return integerKeys.every((key) => ordinaryWorkflowFailureNonnegativeInteger(value[key]))
}

function ordinaryWorkflowFailureProviderDiagnosticValid(value) {
  const providerKinds = [
    'auth', 'rate_limit', 'insufficient_balance', 'request', 'server', 'network',
    'http', 'invalid_model', 'unknown'
  ]
  const providerReasonCodes = [
    'provider_error', 'provider_authentication_failed', 'provider_rate_limited',
    'provider_insufficient_balance', 'provider_endpoint_not_found',
    'provider_request_rejected', 'provider_request_failed', 'provider_request_error',
    'provider_unavailable', 'provider_network_unavailable', 'provider_timeout',
    'provider_stream_interrupted', 'provider_model_invalid', 'provider_not_configured',
    'provider_tool_arguments_invalid', 'provider_reasoning_markup_invalid',
    'provider_empty_final', 'provider_stream_failed', 'turn_failed'
  ]
  if (!exactObjectKeys(value, ORDINARY_WORKFLOW_PROVIDER_DIAGNOSTIC_KEYS) ||
      typeof value.observed !== 'boolean' ||
      typeof value.observationCode !== 'string' ||
      !PROVIDER_FAILURE_OBSERVATION_CODES.includes(value.observationCode) ||
      typeof value.reasonCode !== 'string' ||
      typeof value.providerKind !== 'string' ||
      ![null, 'none', 'possible', 'required', 'unknown', 'not_observed'].includes(
        value.providerAuthStatus
      ) ||
      ![null, true, false].includes(value.providerRetryable) ||
      !(value.providerStatus === null || (
        ordinaryWorkflowFailureNonnegativeInteger(value.providerStatus) &&
        value.providerStatus <= 599
      )) ||
      !ordinaryWorkflowFailureDigestValid(value.diagnosticDigest, {
        empty: value.observed !== true
      })) return false
  if (value.observed === true) {
    return ['provider_failure_observed', 'provider_failure_terminal_observed']
      .includes(value.observationCode) &&
      providerKinds.includes(value.providerKind) && providerReasonCodes.includes(value.reasonCode)
  }
  return value.observationCode !== 'provider_failure_observed' &&
    value.observationCode !== 'provider_failure_terminal_observed' &&
    value.observationCode !== 'host_context_budget_observed' &&
    value.reasonCode === 'none' && value.providerKind === 'not_observed' &&
    value.providerStatus === null && value.providerRetryable === null &&
    value.providerAuthStatus === 'not_observed'
}

function ordinaryWorkflowFailureToolDiagnosticValid(value) {
  if (!exactObjectKeys(value, ORDINARY_WORKFLOW_TOOL_DIAGNOSTIC_KEYS) ||
      typeof value.observed !== 'boolean' ||
      typeof value.observationCode !== 'string' ||
      typeof value.evidenceCode !== 'string' ||
      !TOOL_FAILURE_OBSERVATION_CODES.includes(value.observationCode) ||
      !TOOL_FAILURE_EVIDENCE_CODES.includes(value.evidenceCode) ||
      ![true, false].includes(value.toolFailureGuardBound) ||
      !ordinaryWorkflowFailureDigestValid(value.diagnosticDigest, {
        empty: value.observed !== true
      })) return false
  const integerKeys = [
    'guardCount', 'maxStormCount', 'invalidArgumentGuardCount',
    'invalidArgumentMaxStormCount'
  ]
  if (!integerKeys.every((key) => ordinaryWorkflowFailureNonnegativeInteger(value[key])) ||
      typeof value.guardKind !== 'string' || typeof value.toolCategory !== 'string' ||
      typeof value.toolNameHash !== 'string' ||
      typeof value.invalidArgumentToolCategory !== 'string' ||
      typeof value.invalidArgumentToolNameHash !== 'string' ||
      typeof value.terminalReason !== 'string' || typeof value.terminalCode !== 'string') {
    return false
  }
  if (value.observed === true) {
    return value.observationCode === 'tool_failure_observed' &&
      value.evidenceCode === 'tool_failure_evidence_observed' &&
      value.toolFailureGuardBound === true && value.guardCount > 0 &&
      value.guardKind === 'tool_failure' &&
      WORKFLOW_DIAGNOSTIC_TOOL_CATEGORIES.includes(value.toolCategory) &&
      value.maxStormCount > 0 && value.maxStormCount <= 1_000_000 &&
      value.terminalReason === 'tool_failure' &&
      ['tool_failure_storm', 'tool_invalid_arguments_storm'].includes(value.terminalCode) &&
      /^[0-9a-f]{64}$/u.test(value.toolNameHash)
  }
  const absenceEvidenceBound = value.observationCode === 'tool_failure_not_observed'
    ? value.evidenceCode === 'tool_failure_evidence_not_observed'
    : value.evidenceCode === 'tool_failure_evidence_observer_rejected'
  return value.observationCode !== 'tool_failure_observed' &&
    value.observationCode !== 'tool_failure_guard_history_observed' &&
    absenceEvidenceBound &&
    value.toolFailureGuardBound === false && value.guardCount === 0 &&
    value.maxStormCount === 0 && value.guardKind === 'none' &&
    value.toolCategory === 'none' && value.toolNameHash === '' &&
    value.invalidArgumentGuardCount === 0 && value.invalidArgumentMaxStormCount === 0 &&
    value.invalidArgumentToolCategory === 'none' && value.invalidArgumentToolNameHash === '' &&
    value.terminalReason === 'none' && value.terminalCode === 'none'
}

export function ordinaryWorkflowFailureDiagnosticProjection(
  terminalWorkflowDiagnosticValue,
  providerFailureDiagnosticValue,
  toolFailureDiagnosticValue
) {
  const invalid = () => Object.freeze(ordinaryWorkflowFailureProjectionBase())
  if (!ordinaryWorkflowFailureTerminalDiagnosticValid(terminalWorkflowDiagnosticValue) ||
      !ordinaryWorkflowFailureProviderDiagnosticValid(providerFailureDiagnosticValue) ||
      !ordinaryWorkflowFailureToolDiagnosticValid(toolFailureDiagnosticValue)) return invalid()
  const terminal = terminalWorkflowDiagnosticValue
  const provider = providerFailureDiagnosticValue
  const tool = toolFailureDiagnosticValue
  const providerObserved = provider.observed === true
  const toolObserved = tool.observed === true
  if (terminal.turnStatus === 'completed') {
    if (providerObserved || toolObserved) return invalid()
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'not_observed',
      observationCode: 'workflow_diagnostic_not_observed',
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: false,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  if (terminal.turnStatus !== 'failed' || (providerObserved && toolObserved)) return invalid()
  const statuses = terminal.providerAttemptStatusCounts
  const providerTransportSucceeded = terminal.providerReceiptCountersBound === true &&
    terminal.providerAttemptRecordCount === 1 && terminal.providerTerminalRecordCount === 1 &&
    terminal.providerLogicalCallCount > 0 && terminal.providerAttemptCount > 0 &&
    statuses.succeeded === terminal.providerAttemptCount &&
    statuses.failed === 0 && statuses.cancelled === 0 && statuses.timedOut === 0 &&
    statuses.streamAborted === 0
  const toolAttemptCategoryTotal = Object.values(terminal.toolAttemptCategoryCounts)
    .reduce((total, count) => total + count, 0)
  const successfulToolCategoryTotal = Object.values(terminal.successfulToolCategoryCounts)
    .reduce((total, count) => total + count, 0)
  const providerStatusCount = Object.values(statuses)
    .reduce((total, count) => total + count, 0)
  if (terminal.providerLogicalCallCount > terminal.providerAttemptCount ||
      statuses.succeeded > terminal.providerLogicalCallCount ||
      providerStatusCount !== terminal.providerAttemptCount) return invalid()
  const closedHostFailure = terminal.terminalErrorItemCount === 1 &&
    terminal.terminalErrorItemAuthorityBound === true &&
    HOST_POST_RESPONSE_FAILURE_CODES.has(terminal.turnErrorCode) &&
    terminal.turnErrorCodeHash === sha256(terminal.turnErrorCode) &&
    terminal.terminalReasonClass === 'none' &&
    terminal.toolInventoryAvailability === 'public_projection_available' &&
    terminal.providerReceiptAvailability === 'general_terminal_sse_replay' &&
    terminal.ordinaryResultObserved === false &&
    terminal.ordinaryResultReasonCode === 'ordinary_result_absent' &&
    terminal.toolAttemptCount === terminal.successfulToolExecutionCount &&
    toolAttemptCategoryTotal === terminal.toolAttemptCount &&
    successfulToolCategoryTotal === terminal.successfulToolExecutionCount &&
    terminal.failedToolResultCount === 0 && terminal.unsettledToolResultCount === 0 &&
    provider.observed === false &&
    provider.observationCode === 'provider_failure_terminal_invalid' &&
    tool.observed === false && tool.observationCode === 'tool_failure_not_observed' &&
    providerTransportSucceeded
  if (closedHostFailure) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'host_post_response_failure',
      observationCode: 'host_failure_phase_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  const providerOutputPolicyFailureObserved = terminal.terminalErrorItemCount === 1 &&
    terminal.terminalErrorItemAuthorityBound === true &&
    PROVIDER_OUTPUT_POLICY_FAILURE_CODES.has(provider.reasonCode) &&
    [provider.reasonCode, 'provider_error'].includes(terminal.turnErrorCode) &&
    terminal.turnErrorCodeHash === sha256(terminal.turnErrorCode) &&
    terminal.terminalReasonClass === 'none' &&
    terminal.toolInventoryAvailability === 'public_projection_available' &&
    terminal.providerReceiptAvailability === 'general_terminal_sse_replay' &&
    terminal.ordinaryResultObserved === false &&
    terminal.ordinaryResultReasonCode === 'ordinary_result_absent' &&
    terminal.toolAttemptCount === terminal.successfulToolExecutionCount &&
    toolAttemptCategoryTotal === terminal.toolAttemptCount &&
    successfulToolCategoryTotal === terminal.successfulToolExecutionCount &&
    terminal.failedToolResultCount === 0 && terminal.unsettledToolResultCount === 0 &&
    providerObserved && [
      'provider_failure_observed',
      'provider_failure_terminal_observed'
    ].includes(provider.observationCode) &&
    tool.observed === false && tool.observationCode === 'tool_failure_not_observed' &&
    providerTransportSucceeded
  if (providerOutputPolicyFailureObserved) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_output_policy_failure_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  const formal21 = terminal.terminalErrorItemCount === 1 &&
    terminal.terminalErrorItemAuthorityBound === true &&
    terminal.turnErrorCode === 'provider_error' &&
    terminal.terminalReasonClass === 'none' &&
    terminal.toolInventoryAvailability === 'public_projection_available' &&
    terminal.providerReceiptAvailability === 'general_terminal_sse_replay' &&
    terminal.ordinaryResultObserved === false &&
    terminal.ordinaryResultReasonCode === 'ordinary_result_absent' &&
    terminal.toolAttemptCount > 0 &&
    terminal.toolAttemptCount ===
      terminal.successfulToolExecutionCount + terminal.failedToolResultCount &&
    toolAttemptCategoryTotal === terminal.toolAttemptCount &&
    successfulToolCategoryTotal === terminal.successfulToolExecutionCount &&
    terminal.unsettledToolResultCount === 0 &&
    provider.observed === false &&
    provider.observationCode === 'provider_failure_diagnostic_missing' &&
    tool.observed === false && tool.observationCode === 'tool_failure_not_observed' &&
    providerTransportSucceeded
  if (formal21) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: provider.observationCode,
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: false,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  const providerOutputPolicyFailure = terminal.terminalErrorItemCount === 1 &&
    terminal.terminalErrorItemAuthorityBound === true &&
    terminal.turnErrorCode === 'tool_private_arguments' &&
    terminal.terminalReasonClass === 'none' &&
    terminal.toolInventoryAvailability === 'public_projection_available' &&
    terminal.providerReceiptAvailability === 'general_terminal_sse_replay' &&
    terminal.ordinaryResultObserved === false &&
    terminal.ordinaryResultReasonCode === 'ordinary_result_absent' &&
    terminal.toolAttemptCount > 0 &&
    terminal.toolAttemptCount === terminal.successfulToolExecutionCount &&
    toolAttemptCategoryTotal === terminal.toolAttemptCount &&
    successfulToolCategoryTotal === terminal.successfulToolExecutionCount &&
    terminal.failedToolResultCount === 0 && terminal.unsettledToolResultCount === 0 &&
    provider.observed === false &&
    provider.observationCode === 'provider_failure_terminal_invalid' &&
    tool.observed === false && tool.observationCode === 'tool_failure_not_observed' &&
    providerTransportSucceeded
  if (providerOutputPolicyFailure) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_output_policy_failure_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  const providerPipelineFailure = providerObserved &&
    terminal.terminalErrorItemCount === 1 &&
    terminal.terminalErrorItemAuthorityBound === true &&
    terminal.turnErrorCodeHash === sha256(terminal.turnErrorCode) &&
    [provider.reasonCode, 'provider_error'].includes(terminal.turnErrorCode) &&
    terminal.terminalReasonClass === 'none' &&
    terminal.toolInventoryAvailability === 'public_projection_available' &&
    terminal.providerReceiptAvailability === 'general_terminal_sse_replay' &&
    terminal.ordinaryResultObserved === false &&
    terminal.ordinaryResultReasonCode === 'ordinary_result_absent' &&
    terminal.failedToolResultCount === 0 && terminal.unsettledToolResultCount === 0
  if (providerPipelineFailure) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'provider_pipeline_failure',
      observationCode: provider.observationCode,
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: true,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  if (toolObserved) {
    return ordinaryWorkflowFailureProjectionResult({
      classification: 'tool_failure',
      observationCode: tool.observationCode,
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: true,
      terminalDiagnosticDigest: terminal.inventoryDigest,
      providerDiagnosticDigest: provider.diagnosticDigest,
      toolDiagnosticDigest: tool.diagnosticDigest
    })
  }
  return invalid()
}

export function ordinaryWorkflowFailureCode({
  disposition,
  ordinaryPassed,
  repositoryBlocker = '',
  terminalStatus = '',
  ordinaryFunctionalPassed = false,
  providerReceiptBound = false,
  workflowFailureClassification = ''
} = {}) {
  if (ordinaryPassed === true) return ''
  if (workflowFailureClassification === 'host_post_response_failure' &&
      terminalStatus === 'failed' &&
      ['terminal_without_expected_marker', 'terminal_without_bound_result'].includes(
        disposition
      )) {
    return 'packaged_ordinary_host_post_response_failure'
  }
  if (workflowFailureClassification === 'provider_transport_succeeded_terminal_failure' &&
      terminalStatus === 'failed' &&
      ['terminal_without_expected_marker', 'terminal_without_bound_result'].includes(
        disposition
      )) {
    return 'packaged_ordinary_provider_transport_succeeded_terminal_failure'
  }
  if (disposition === 'timeout') return 'packaged_ordinary_workflow_timeout'
  if (disposition === 'terminal_without_bound_result') {
    if (terminalStatus === 'failed') return 'packaged_ordinary_workflow_failed'
    if (terminalStatus === 'aborted') return 'packaged_ordinary_workflow_aborted'
    return 'packaged_ordinary_workflow_result_unbound'
  }
  if (disposition !== 'completed') {
    if (terminalStatus === 'completed') {
      return 'packaged_ordinary_workflow_completed_without_expected_marker'
    }
    if (terminalStatus === 'failed') {
      return 'packaged_ordinary_workflow_failed_without_expected_marker'
    }
    if (terminalStatus === 'aborted') {
      return 'packaged_ordinary_workflow_aborted_without_expected_marker'
    }
    return 'packaged_ordinary_workflow_terminal_without_expected_marker'
  }
  if (repositoryBlocker) {
    return safeWorkflowDiagnosticCode(
      repositoryBlocker,
      'packaged_ordinary_agent_contract_failed'
    )
  }
  if (ordinaryFunctionalPassed === true && providerReceiptBound !== true) {
    return 'packaged_ordinary_provider_receipt_failed'
  }
  return 'packaged_ordinary_agent_contract_failed'
}

export function longContextContinuationFailureCode({
  disposition = '',
  externallyBlocked = false,
  acceptedFinalReasonCode = 'input_invalid',
  hostReadBound = false,
  reasoningEffortBound = false,
  providerReceiptReplayObserved = false,
  repositoryBound = false,
  subagentStateBound = false,
  caseWorkspaceScopeBound = true
} = {}) {
  if (externallyBlocked === true) return 'external_prerequisite'
  const acceptedReasonCodes = new Set(CONTINUATION_ACCEPTED_FINAL_REASON_CODES)
  if (acceptedFinalReasonCode !== 'observed' && acceptedReasonCodes.has(acceptedFinalReasonCode)) {
    return `continuation_${acceptedFinalReasonCode}`
  }
  if (disposition === 'timeout') return 'continuation_timeout'
  if (disposition !== 'completed') return 'continuation_terminal_without_completion'
  if (!caseWorkspaceScopeBound) return 'continuation_case_workspace_scope_invalid'
  if (!hostReadBound) return 'continuation_read_missing'
  if (!reasoningEffortBound) return 'continuation_reasoning_effort_invalid'
  if (!providerReceiptReplayObserved) return 'continuation_provider_receipt_invalid'
  if (!repositoryBound) return 'continuation_repository_invalid'
  if (!subagentStateBound) return 'continuation_subagent_state_invalid'
  return ''
}

export function packagedResultTurnContractEvidence(
  workflow,
  runtimeSkills,
  publicSeam,
  artifact,
  scheduleConfig
) {
  const executions = Array.isArray(workflow?.resultTurnSuccessfulExecutions)
    ? workflow.resultTurnSuccessfulExecutions
    : []
  const attempts = Array.isArray(workflow?.resultTurnToolCallAttempts)
    ? workflow.resultTurnToolCallAttempts
    : []
  const executionCount = (toolName) =>
    executions.filter((item) => item?.toolName === toolName).length
  const attemptCount = (toolName) =>
    attempts.filter((item) => item?.toolName === toolName).length
  const mcpExecutions = executions.filter((item) =>
    typeof item?.toolName === 'string' && item.toolName.startsWith('mcp__')
  )
  const mcpAttempts = attempts.filter((item) =>
    typeof item?.toolName === 'string' && item.toolName.startsWith('mcp__')
  )
  const fundsMCPAttempts = mcpAttempts.filter((item) => fundsToolName(item.toolName))
  const subagentDelegationAttempts = attempts.filter((item) =>
    PACKAGED_SUBAGENT_DELEGATION_TOOLS.includes(item?.toolName)
  )
  const malformedToolCallAttempts = attempts.filter((item) =>
    !item?.callId || !item?.toolName || !item?.toolKind
  )
  const taskAttemptCount = attemptCount('task')
  const runSkillAttemptCount = attemptCount('run_skill')
  const bashAttemptCount = attemptCount('bash')
  const ordinaryMCPAttemptCount = attemptCount(PACKAGED_ORDINARY_MCP_TOOL)
  const taskExecutionCount = executionCount('task')
  const runSkillExecutionCount = executionCount('run_skill')
  const bashExecutionCount = executionCount('bash')
  const ordinaryMCPExecutionCount = executionCount(PACKAGED_ORDINARY_MCP_TOOL)
  const taskSummary = Array.isArray(workflow?.subagents) && workflow.subagents.length === 1
    ? workflow.subagents[0]
    : null
  const taskProfileBound = workflow?.boundedSubagentPresent === true &&
    taskSummary?.parentTurnId === workflow?.resultTurnId &&
    taskSummary?.profile === PACKAGED_READ_ONLY_SUBAGENT_PROFILE &&
    taskSummary?.toolPolicy === 'readOnly'
  const taskForegroundBound = taskProfileBound &&
    taskSummary?.background !== true &&
    taskSummary?.diagnostics?.background === false
  const formalExecutionBounds = milestoneAFormalExecutionBounds()
  const taskStepLimitBound = taskProfileBound &&
    taskSummary?.maxModelSteps === formalExecutionBounds.childMaxModelSteps
  const taskTimeBudgetBound = taskProfileBound &&
    taskSummary?.timeBudgetMs === formalExecutionBounds.childTimeBudgetMs
  const skillCatalogBound =
    packagedMilestoneASkillID(runtimeSkills) === PACKAGED_SKILL_ID &&
    artifact?.packagedSkillSourceMatched === true &&
    /^[0-9a-f]{64}$/.test(String(artifact?.sourceSkillSha256 || '')) &&
    artifact?.sourceSkillSha256 === artifact?.packagedSkillSha256
  const packageAuthorityBound = artifact?.ok === true &&
    artifact?.worktreeSnapshotBinding?.ok === true &&
    /^[0-9a-f]{64}$/.test(String(artifact?.appAsarSha256 || '')) &&
    artifact?.packagedScheduleSourceContractBound === true &&
    artifact?.packagedScheduleNonMutatingListImplementationBound === true
  const packageCatalogBound = publicSeam?.ordinaryMCPAvailable === true &&
    publicSeam?.ordinaryMCPServerId === PACKAGED_ORDINARY_MCP_SERVER_ID &&
    publicSeam?.ordinaryMCPServerCount === 1 &&
    publicSeam?.ordinaryMCPToolCount === PACKAGED_ORDINARY_MCP_TOOL_COUNT &&
    /^[0-9a-f]{64}$/.test(String(publicSeam?.ordinaryToolCatalogHash || ''))
  const packageConfigBound = scheduleConfig?.ok === true &&
    scheduleConfig?.exactConfigBound === true &&
    scheduleConfig?.configMode === 0o600 &&
    scheduleConfig?.configLinkCount === 1 &&
    /^[0-9a-f]{64}$/.test(String(scheduleConfig?.configSha256 || '')) &&
    /^[0-9a-f]{64}$/.test(String(scheduleConfig?.helperExecutableSha256 || ''))
  const strictEmptyInputSchemaBound = packageAuthorityBound &&
    artifact?.packagedScheduleStrictEmptyInputSchemaBound === true
  const exactOrdinaryMCPResultBound = ordinaryMCPExecutionCount === 1 &&
    ordinaryMCPAttemptCount === 1 &&
    mcpExecutions.length === 1 && mcpAttempts.length === 1 &&
    fundsMCPAttempts.length === 0 &&
    mcpExecutions[0]?.turnId === workflow?.resultTurnId
  const emptyInputBoundByStrictSchema =
    strictEmptyInputSchemaBound && exactOrdinaryMCPResultBound
  const ok = taskAttemptCount === 1 && taskExecutionCount === 1 &&
    subagentDelegationAttempts.length === 1 &&
    runSkillAttemptCount === 1 && runSkillExecutionCount === 1 &&
    bashAttemptCount === 2 && bashExecutionCount === 2 &&
    malformedToolCallAttempts.length === 0 &&
    taskProfileBound &&
    taskForegroundBound &&
    taskStepLimitBound &&
    taskTimeBudgetBound &&
    skillCatalogBound &&
    packageAuthorityBound &&
    packageCatalogBound &&
    packageConfigBound &&
    emptyInputBoundByStrictSchema
  return {
    ok,
    taskAttemptCount,
    taskExecutionCount,
    subagentDelegationAttemptCount: subagentDelegationAttempts.length,
    runSkillAttemptCount,
    runSkillExecutionCount,
    bashAttemptCount,
    bashExecutionCount,
    ordinaryMCPAttemptCount,
    ordinaryMCPExecutionCount,
    allMCPAttemptCount: mcpAttempts.length,
    allMCPExecutionCount: mcpExecutions.length,
    fundsMCPAttemptCount: fundsMCPAttempts.length,
    malformedToolCallAttemptCount: malformedToolCallAttempts.length,
    taskProfileBound,
    taskForegroundBound,
    taskStepLimitBound,
    taskTimeBudgetBound,
    skillCatalogBound,
    packageAuthorityBound,
    packageCatalogBound,
    packageConfigBound,
    strictEmptyInputSchemaBound,
    exactOrdinaryMCPResultBound,
    emptyInputBoundByStrictSchema
  }
}

export function providerTurnEvidence(workflow, provider) {
  const providerAttempts = Array.isArray(workflow?.providerAttempts)
    ? workflow.providerAttempts
    : []
  const providerTerminals = Array.isArray(workflow?.providerTerminals)
    ? workflow.providerTerminals
    : []
  const sseTrace = providerReceiptTraceEvidence(
    workflow?.providerReceiptTrace || emptyProviderReceiptTrace()
  )
  const empty = {
    reasonCode: 'provider_receipt_not_evaluated',
    sseReasonCode: sseTrace.reasonCode,
    networkTurnCompleted: false,
    threadProviderBound: false,
    threadModelBound: false,
    resultTurnProviderReceiptBound: false,
    providerAttemptTelemetryValid: false,
    providerAttemptReceiptDigest: '',
    providerLogicalCallCount: 0,
    providerAttemptCount: 0,
    successfulProviderAttemptCount: 0,
    providerAttemptRecordCount: providerAttempts.length,
    providerTerminalRecordCount: providerTerminals.length
  }
  if (!workflow?.thread) {
    return { ...empty, reasonCode: 'provider_receipt_workflow_unavailable' }
  }
  if (!provider?.ok) {
    return { ...empty, reasonCode: 'provider_receipt_provider_unavailable' }
  }
  if (!workflow.resultTurnId) {
    return { ...empty, reasonCode: 'provider_receipt_result_turn_unavailable' }
  }
  const threadProviderBound = workflow.thread.providerId === provider.id
  const threadModelBound = workflow.thread.model === provider.model
  if (!threadProviderBound) {
    return { ...empty, reasonCode: 'provider_receipt_thread_provider_mismatch' }
  }
  if (!threadModelBound) {
    return {
      ...empty,
      reasonCode: 'provider_receipt_thread_model_mismatch',
      threadProviderBound
    }
  }
  const resultTurn = (Array.isArray(workflow.thread.turns) ? workflow.thread.turns : [])
    .find((turn) => turn?.id === workflow.resultTurnId)
  const attempts = providerAttempts.filter((item) =>
      item?.kind === 'usage' &&
      item.threadId === workflow.threadId &&
      item.turnId === workflow.resultTurnId &&
      item.model === provider.model
    )
  const attemptReason = (item) => {
    const statuses = item?.providerAttemptStatuses
    if (!statuses || typeof statuses !== 'object') {
      return 'provider_receipt_attempt_status_invalid'
    }
    const counts = [
      statuses.succeeded,
      statuses.failed,
      statuses.cancelled,
      statuses.timedOut,
      statuses.streamAborted
    ]
    if (!counts.every((value) => Number.isSafeInteger(value) && value >= 0)) {
      return 'provider_receipt_attempt_status_invalid'
    }
    const attemptsTotal = counts.reduce((total, value) => total + value, 0)
    if (resultTurn?.status !== 'completed') return 'provider_receipt_turn_not_completed'
    if (item.usageFinalStatus !== undefined && item.usageFinalStatus !== 'completed') {
      return 'provider_receipt_usage_status_invalid'
    }
    if (!Number.isSafeInteger(item.seq) || item.seq <= 0) {
      return 'provider_receipt_usage_sequence_invalid'
    }
    const providerAttemptTelemetryValid =
      item.providerAttemptTelemetrySchema === 'provider-attempt-telemetry.v1' &&
      item.providerAttemptTelemetryValid === true
    if (!providerAttemptTelemetryValid) {
      return 'provider_receipt_usage_telemetry_invalid'
    }
    if (!Number.isSafeInteger(item.providerLogicalCallCount) ||
        item.providerLogicalCallCount <= 0 ||
        !Number.isSafeInteger(item.providerAttemptCount) ||
        item.providerAttemptCount < item.providerLogicalCallCount ||
        attemptsTotal !== item.providerAttemptCount ||
        statuses.succeeded !== item.providerLogicalCallCount) {
      return 'provider_receipt_attempt_counts_invalid'
    }
    if (!Number.isSafeInteger(item.promptTokens) || item.promptTokens <= 0 ||
        !Number.isSafeInteger(item.completionTokens) || item.completionTokens <= 0 ||
        !Number.isSafeInteger(item.totalTokens) ||
        item.totalTokens !== item.promptTokens + item.completionTokens ||
        item.turns !== 1) return 'provider_receipt_usage_tokens_invalid'
    return 'provider_receipt_observed'
  }
  let attempt = null
  let reasonCode = attempts.length > 0
    ? 'provider_receipt_attempt_invalid'
    : sseTrace.reasonCode
  for (const candidate of attempts) {
    const candidateReason = attemptReason(candidate)
    if (candidateReason === 'provider_receipt_observed') {
      attempt = candidate
      reasonCode = candidateReason
      break
    }
    reasonCode = candidateReason
  }
  const terminal = attempt && providerTerminals.find((item) =>
    item?.kind === 'turn_completed' &&
    item.threadId === workflow.threadId &&
    item.turnId === workflow.resultTurnId &&
    item.status === 'completed' &&
    Number.isSafeInteger(item.seq) &&
      item.seq === attempt.seq + 1)
  const resultTurnProviderReceiptBound =
    threadProviderBound && threadModelBound && Boolean(attempt) && Boolean(terminal)
  if (attempt && !terminal) reasonCode = 'provider_receipt_terminal_pair_missing'
  return {
    reasonCode,
    sseReasonCode: sseTrace.reasonCode,
    networkTurnCompleted: resultTurnProviderReceiptBound,
    threadProviderBound,
    threadModelBound,
    resultTurnProviderReceiptBound,
    providerAttemptTelemetryValid: attempt?.providerAttemptTelemetryValid === true,
    providerAttemptReceiptDigest: attempt && terminal
      ? sha256(canonicalJSON({ attempt, terminal }))
      : '',
    providerLogicalCallCount: attempt?.providerLogicalCallCount || 0,
    providerAttemptCount: attempt?.providerAttemptCount || 0,
    successfulProviderAttemptCount: attempt?.providerAttemptStatuses?.succeeded || 0,
    providerAttemptRecordCount: providerAttempts.length,
    providerTerminalRecordCount: providerTerminals.length
  }
}

async function waitForWorkflow({
  debugPort,
  workspace,
  timeoutMs,
  resultMarker = '',
  previousTurnCount = 0,
  expectedThreadId = ''
}) {
  const deadline = Date.now() + timeoutMs
  let latest = null
  while (Date.now() < deadline) {
    try {
      latest = await observeRenderer({
        debugPort,
        workspace,
        exactThreadId: expectedThreadId,
        timeoutMs: Math.min(20_000, Math.max(1000, deadline - Date.now()))
      })
      const evidence = workflowEvidence(latest, workspace, resultMarker)
      const disposition = resultMarker
        ? workflowWaitDisposition(evidence, { previousTurnCount, expectedThreadId })
        : ordinaryWorkflowWaitDisposition(evidence, { previousTurnCount, expectedThreadId })
      if (disposition !== 'pending') {
        return { observation: latest, evidence, disposition }
      }
    } catch {
      // The app may reload while the runtime starts; retry only read-only observation.
    }
    await sleep(750)
  }
  return {
    observation: latest,
    evidence: workflowEvidence(latest, workspace, resultMarker),
    disposition: 'timeout'
  }
}

async function waitForNewTerminalTurn({
  debugPort,
  workspace,
  threadId,
  previousTurnCount,
  timeoutMs
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
      const disposition = protectedFundsWaitDisposition(latest, {
        previousTurnCount,
        expectedThreadId: threadId
      })
      if (disposition !== 'pending') {
        const turn = turns.at(-1)
        return {
          observation: latest,
          turn,
          evidence: protectedFundsUnavailableTurnEvidence(latest, threadId, turn?.id)
        }
      }
    } catch {
      // The host boundary and public projection may settle at different times.
    }
    await sleep(750)
  }
  const turns = Array.isArray(latest?.thread?.turns) ? latest.thread.turns : []
  const turn = turns.at(-1)
  return {
    observation: latest,
    turn,
    evidence: protectedFundsUnavailableTurnEvidence(latest, threadId, turn?.id)
  }
}

async function waitForCompaction({
  debugPort,
  workspace,
  threadId,
  baseline,
  timeoutMs
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
      const evidence = workflowEvidence(latest, workspace)
      const proof = manualCompactionProofEvidence(evidence, baseline)
      const disposition = compactionWaitDisposition(evidence, { baseline })
      if (disposition !== 'pending') {
        return { observation: latest, evidence, proof, disposition }
      }
    } catch {
      // Retry read-only observation.
    }
    await sleep(500)
  }
  return {
    observation: latest,
    evidence: workflowEvidence(latest, workspace),
    proof: manualCompactionProofEvidence(
      workflowEvidence(latest, workspace),
      baseline
    ),
    disposition: 'timeout'
  }
}

const TEST_RECEIPT_KEYS = [
  'ancestorChainDigest',
  'assertionCount',
  'baselineCommit',
  'baselineTree',
  'completedAt',
  'contract',
  'expectedSourceSha256',
  'gitStatusSha256',
  'marker',
  'nonce',
  'npmCommand',
  'npmLifecycleEvent',
  'npmLifecycleScriptSha256',
  'npmPackageName',
  'npmTestAncestorObserved',
  'observedSourceSha256',
  'packageScriptParentObserved',
  'protectedInputsDigest',
  'startedAt',
  'validatorSha256',
  'workspaceSha256'
].sort()

function timeValue(value) {
  const parsed = Date.parse(String(value || ''))
  return Number.isFinite(parsed) ? parsed : NaN
}

function receiptBashExecutionBinding(thread, expectedTurnId, receipt) {
  const receiptStartedAt = timeValue(receipt?.startedAt)
  const receiptCompletedAt = timeValue(receipt?.completedAt)
  if (!Number.isFinite(receiptStartedAt) || !Number.isFinite(receiptCompletedAt) ||
    receiptCompletedAt < receiptStartedAt) {
    return { ok: false, executionCount: 0, callIdHash: '', turnIdHash: '' }
  }
  const executions = successfulToolExecutions(thread).filter((item) => {
    if (item.toolName !== 'bash' || (expectedTurnId && item.turnId !== expectedTurnId)) return false
    const callCreatedAt = timeValue(item.callCreatedAt)
    const resultFinishedAt = timeValue(item.resultFinishedAt)
    return Number.isFinite(callCreatedAt) && Number.isFinite(resultFinishedAt) &&
      receiptStartedAt >= callCreatedAt - 1000 &&
      receiptCompletedAt <= resultFinishedAt + 1000
  })
  const execution = executions.length === 1 ? executions[0] : null
  return {
    ok: Boolean(execution),
    executionCount: executions.length,
    callIdHash: execution ? sha256(execution.callId) : '',
    turnIdHash: execution ? sha256(execution.turnId) : ''
  }
}

// A same-UID unrestricted shell can read/replay the external validator and
// manufacture every receipt field. Keep this parser for hostile regression
// diagnostics only: neither its shape nor timestamp correlation is authority
// for the Agent's exact bash invocation.
export function testReceiptEvidence(
  authority,
  thread,
  expectedTurnId = '',
  hostObservation = null
) {
  const external = externalRepositoryAuthorityShapeValid(authority)
  const base = {
    passed: false,
    count: 0,
    sha256: '',
    externalValidatorBound: false,
    protectedInputsBound: false,
    onlyIntendedSourceChanged: false,
    npmLifecycleBound: false,
    npmTestProcessLineageBound: false,
    actualBashToolResultBound: false,
    hostOwnedToolInvocationBound: false,
    selfReportedReceiptObserved: false,
    selfReportedReceiptShapeValid: false,
    selfReportedReceiptAuthoritative: false,
    untrustedTimingCorrelationObserved: false,
    bashExecutionCount: 0,
    bashCallIdHash: '',
    bashTurnIdHash: '',
    hostObservationDigest: '',
    hostAuthorityBindingDigest: '',
    blocker: 'host_owned_tool_invocation_binding_unavailable'
  }
  if (!repositoryAuthorityShapeValid(authority) && !external) {
    return { ...base, blocker: 'repository_acceptance_authority_invalid' }
  }
  const repository = verifyRepositoryAcceptance(authority, 'final')
  let authorityWorkspaceRealPath = ''
  try {
    authorityWorkspaceRealPath = realpathSync(authority.workspace)
  } catch {
    authorityWorkspaceRealPath = ''
  }
  const hostOwnedToolInvocationBound = repository.ok &&
    typeof thread?.id === 'string' &&
    privateHostToolExecutionBindingMatches(hostObservation, {
      threadId: thread.id,
      turnId: expectedTurnId,
      toolName: 'bash',
      workspace: authorityWorkspaceRealPath,
      arguments: { command: external ? authority.testCommand : 'npm test' }
    })
  const authoritative = {
    ...base,
    passed: hostOwnedToolInvocationBound,
    count: hostOwnedToolInvocationBound ? 1 : 0,
    protectedInputsBound: repository.protectedInputsBound,
    onlyIntendedSourceChanged: repository.onlyIntendedSourceChanged,
    actualBashToolResultBound: hostOwnedToolInvocationBound,
    hostOwnedToolInvocationBound,
    hostObservationDigest: hostOwnedToolInvocationBound
      ? hostObservation.observationDigest
      : '',
    hostAuthorityBindingDigest: hostOwnedToolInvocationBound
      ? hostObservation.authorityBindingDigest
      : '',
    blocker: hostOwnedToolInvocationBound
      ? ''
      : repository.ok
        ? 'host_owned_tool_invocation_binding_unavailable'
        : repository.blocker || 'repository_acceptance_failed'
  }
  if (external) return authoritative
  const receiptFile = hashRegularFile(authority.receiptPath, {
    capture: true,
    maximumBytes: 64 * 1024
  })
  if (!receiptFile.regular || !receiptFile.content) {
    return authoritative
  }
  let receipt
  try {
    receipt = JSON.parse(receiptFile.content.toString('utf8'))
  } catch {
    return {
      ...authoritative,
      sha256: receiptFile.sha256,
      selfReportedReceiptObserved: true
    }
  }
  const keysValid = receipt && typeof receipt === 'object' && !Array.isArray(receipt) &&
    Object.keys(receipt).sort().join(',') === TEST_RECEIPT_KEYS.join(',')
  const canonical = receiptFile.content.toString('utf8') === `${JSON.stringify(receipt)}\n`
  const externalValidatorBound = keysValid && canonical &&
    receipt.contract === TEST_RECEIPT_CONTRACT &&
    receipt.marker === TEST_MARKER &&
    receipt.nonce === authority.nonce &&
    receipt.workspaceSha256 === authority.workspaceSha256 &&
    receipt.baselineCommit === authority.baselineCommit &&
    receipt.baselineTree === authority.baselineTree &&
    receipt.protectedInputsDigest === authority.protectedInputsDigest &&
    receipt.expectedSourceSha256 === authority.expectedSourceSha256 &&
    receipt.observedSourceSha256 === authority.expectedSourceSha256 &&
    receipt.validatorSha256 === authority.validatorSha256 &&
    receipt.gitStatusSha256 === authority.expectedStatusSha256 &&
    receipt.assertionCount === 3 &&
    /^[0-9a-f]{64}$/.test(String(receipt.ancestorChainDigest || ''))
  const npmLifecycleBound = externalValidatorBound &&
    receipt.npmLifecycleEvent === 'test' &&
    receipt.npmCommand === 'test' &&
    receipt.npmPackageName === 'analytix-milestone-a-isolated-repo' &&
    receipt.npmLifecycleScriptSha256 === authority.testScriptSha256
  const npmTestProcessLineageBound = npmLifecycleBound &&
    receipt.npmTestAncestorObserved === true &&
    receipt.packageScriptParentObserved === true
  const bashBinding = receiptBashExecutionBinding(thread, expectedTurnId, receipt)
  const diagnosticShapeValid = repository.ok && externalValidatorBound && npmLifecycleBound &&
    npmTestProcessLineageBound
  return {
    ...authoritative,
    sha256: receiptFile.sha256,
    selfReportedReceiptObserved: true,
    selfReportedReceiptShapeValid: diagnosticShapeValid,
    untrustedTimingCorrelationObserved: bashBinding.ok,
    bashExecutionCount: bashBinding.executionCount,
    bashCallIdHash: bashBinding.callIdHash,
    bashTurnIdHash: bashBinding.turnIdHash
  }
}

function sourceRepairEvidence(authority) {
  const repository = verifyRepositoryAcceptance(authority, 'final')
  return {
    changed: repository.ok && repository.expectedSourceBound,
    sha256: repository.sourceSha256,
    repository
  }
}


function check(id, passed, message, statusWhenFalse = 'failed') {
  return {
    id,
    status: passed ? 'passed' : statusWhenFalse,
    message
  }
}

const CHECK_STATUS_ORDER = Object.freeze({
  passed: 0,
  skipped: 1,
  live_blocked: 2,
  failed: 3
})
const MILESTONE_A_AUXILIARY_CHECK_IDS = new Set([
  'external-real-repository-contract',
  'parent-owned-repository-test-cross-check',
  'protected-funds-source-unavailable'
])

const MILESTONE_A_IDLE_FAILURE_STAGE_FIELDS = new Set([
  'longContextIdleFailureHealthOk',
  'longContextIdleFailureRuntimeInfoOk',
  'longContextIdleFailureRuntimeToolsOk',
  'longContextIdleFailureRuntimeThreadListOk',
  'longContextIdleFailureRuntimeThreadListStatus',
  'longContextIdleFailureRuntimeBackendTopologyOk',
  'longContextIdleFailureWaitedMs'
])

const MILESTONE_A_HOST_OBSERVATION_REASON_STAGE_FIELDS = new Set([
  'bashHostObservationReasonCode',
  'gitHostObservationReasonCode'
])
const MILESTONE_A_HOST_OBSERVATION_TRANSPORT_STATUS_STAGE_FIELDS = new Set([
  'bashHostObservationTransportStatus',
  'gitHostObservationTransportStatus'
])
const MILESTONE_A_RUNTIME_HTTP_STATUS_STAGE_FIELDS = new Set([
  'longContextIdleFailureRuntimeThreadListStatus'
])
const MILESTONE_A_HOST_OBSERVATION_ATTEMPT_COUNT_STAGE_FIELDS = new Set([
  'bashHostObservationAttemptCount',
  'gitHostObservationAttemptCount'
])
const MILESTONE_A_HOST_OBSERVATION_DIGEST_STAGE_FIELDS = new Set([
  'bashHostObservationDigest',
  'bashHostAuthorityBindingDigest',
  'gitHostObservationDigest',
  'gitHostAuthorityBindingDigest'
])
const MILESTONE_A_PROJECTION_CLASS_STAGE_FIELDS = new Set([
  'manualCompactionProjectionClass'
])

function milestoneAStageFieldAllowed(key) {
  return MILESTONE_A_HOST_OBSERVATION_REASON_STAGE_FIELDS.has(key) ||
    MILESTONE_A_PROJECTION_CLASS_STAGE_FIELDS.has(key) ||
    /^(?:[a-z][A-Za-z0-9]*(?:Digest|Hash|Count|Observed|Bound|Completed|Recovered|Restored|Changed|Passed|Selected|Auto|Status|Effort|Model|Ancestry|Agent|Tokens|AfterBaseline))$/u.test(key) ||
    MILESTONE_A_IDLE_FAILURE_STAGE_FIELDS.has(key)
}

function safeStageSnapshotValue(value, key = '') {
  if (MILESTONE_A_HOST_OBSERVATION_TRANSPORT_STATUS_STAGE_FIELDS.has(key) ||
      MILESTONE_A_RUNTIME_HTTP_STATUS_STAGE_FIELDS.has(key)) {
    return Number.isSafeInteger(value) && value >= 0 && value <= 599
      ? value
      : undefined
  }
  if (MILESTONE_A_HOST_OBSERVATION_ATTEMPT_COUNT_STAGE_FIELDS.has(key)) {
    return Number.isSafeInteger(value) && value >= 0 &&
      value <= HOST_TOOL_EXECUTION_MAX_404_RETRIES + 1
      ? value
      : undefined
  }
  if (MILESTONE_A_PROJECTION_CLASS_STAGE_FIELDS.has(key)) {
    return COMPACTION_PROJECTION_CLASSES.has(value) ? value : undefined
  }
  if (typeof value === 'boolean' || (typeof value === 'number' && Number.isSafeInteger(value))) {
    return value
  }
  if (MILESTONE_A_HOST_OBSERVATION_REASON_STAGE_FIELDS.has(key) &&
      HOST_TOOL_EXECUTION_OBSERVATION_REASON_CODES.includes(value)) {
    return value
  }
  if (value === '' && MILESTONE_A_HOST_OBSERVATION_DIGEST_STAGE_FIELDS.has(key)) {
    return value
  }
  if (typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value)) return value
  return undefined
}

export function sanitizeMilestoneAStageSnapshot(
  snapshot,
  knownCheckIds = [...REQUIRED_CHECK_IDS, ...MILESTONE_A_AUXILIARY_CHECK_IDS]
) {
  if (!snapshot || typeof snapshot !== 'object' || Array.isArray(snapshot)) return null
  const stage = typeof snapshot.stage === 'string' && /^[a-z0-9][a-z0-9._-]{0,127}$/u.test(snapshot.stage)
    ? snapshot.stage
    : 'unknown'
  const checkIds = Array.isArray(snapshot.checkIds)
    ? [...new Set(snapshot.checkIds
      .map((value) => typeof value === 'string' ? value.trim() : '')
      .filter((value) => value && knownCheckIds.includes(value)))]
    : []
  const completedFields = {}
  if (snapshot.completedFields && typeof snapshot.completedFields === 'object' &&
      !Array.isArray(snapshot.completedFields)) {
    for (const [key, value] of Object.entries(snapshot.completedFields)) {
      const safe = safeStageSnapshotValue(value, key)
      if (safe !== undefined && milestoneAStageFieldAllowed(key)) {
        completedFields[key] = safe
      }
    }
  }
  const projected = {
    stage,
    checkIds: Object.freeze(checkIds),
    completedFields: Object.freeze(completedFields)
  }
  return Object.freeze({
    ...projected,
    snapshotDigest: sha256(canonicalJSON(projected))
  })
}

function generatedMilestoneStageSnapshot(report, stage, checkIds) {
  const workflow = report?.workflow && typeof report.workflow === 'object'
    ? report.workflow
    : {}
  const completedFields = {}
  for (const [key, value] of Object.entries(workflow)) {
    const safe = safeStageSnapshotValue(value, key)
    if (safe !== undefined && milestoneAStageFieldAllowed(key)) {
      completedFields[key] = safe
    }
  }
  return sanitizeMilestoneAStageSnapshot({ stage, checkIds, completedFields })
}

function safeHostObservationReasonCode(value) {
  return HOST_TOOL_EXECUTION_OBSERVATION_REASON_CODES.includes(value)
    ? value
    : 'transport_unavailable'
}

function safeHostObservationTransportStatus(value) {
  return Number.isSafeInteger(value) && value >= 0 && value <= 599 ? value : 0
}

function safeHostObservationAttemptCount(value) {
  return Number.isSafeInteger(value) && value >= 0 &&
    value <= HOST_TOOL_EXECUTION_MAX_404_RETRIES + 1
    ? value
    : 0
}

function safeHostObservationDigest(value) {
  return typeof value === 'string' && /^[0-9a-f]{64}$/u.test(value) ? value : ''
}

export function ordinaryTestBindingFailureStageSnapshot(hostObservation) {
  return sanitizeMilestoneAStageSnapshot({
    stage: 'ordinary-test-binding-failure',
    checkIds: ['ordinary-agent-workflow', 'real-repository-test'],
    completedFields: {
      bashHostObservationReasonCode: safeHostObservationReasonCode(
        hostObservation?.reasonCode
      ),
      bashHostObservationTransportStatus: safeHostObservationTransportStatus(
        hostObservation?.transportStatus
      ),
      bashHostObservationAttemptCount: safeHostObservationAttemptCount(
        hostObservation?.attemptCount
      ),
      bashHostObservationDigest: safeHostObservationDigest(
        hostObservation?.observationDigest
      ),
      bashHostAuthorityBindingDigest: safeHostObservationDigest(
        hostObservation?.authorityBindingDigest
      )
    }
  })
}

function recoverWorkflowFromStageSnapshots(workflow, snapshots) {
  const recovered = workflow && typeof workflow === 'object' && !Array.isArray(workflow)
    ? { ...workflow }
    : {}
  for (const snapshot of Array.isArray(snapshots) ? snapshots : []) {
    const fields = snapshot?.completedFields
    if (!fields || typeof fields !== 'object' || Array.isArray(fields)) continue
    for (const [key, value] of Object.entries(fields)) {
      const current = recovered[key]
      if (typeof value === 'boolean') {
        if (current === undefined || current === null ||
            (value && current === false && !key.endsWith('Auto'))) {
          recovered[key] = value
        }
        continue
      }
      if (typeof value === 'number' && Number.isSafeInteger(value)) {
        if (!Number.isSafeInteger(current) || value > current) recovered[key] = value
        continue
      }
      if (typeof value === 'string' &&
          (typeof current !== 'string' || current.length === 0 ||
            (key === 'manualCompactionProjectionClass' &&
              current === 'not_observed' && value !== 'not_observed'))) {
        recovered[key] = value
      }
    }
  }
  return recovered
}

const FIRST_LAUNCH_PREREQUISITE_CHECK_IDS = Object.freeze([
  'external-real-repository-contract',
  'trusted-cache-tmpdir',
  'formal-packaged-artifact',
  'isolated-profile-and-repository',
  'configured-network-provider',
  'packaged-first-launch'
])

function firstLaunchPrerequisiteStageSnapshot(report) {
  if (report?.workflow?.firstLaunchObserved !== true) return null
  return sanitizeMilestoneAStageSnapshot({
    stage: 'first-launch-prerequisites',
    checkIds: FIRST_LAUNCH_PREREQUISITE_CHECK_IDS,
    completedFields: { firstLaunchObserved: true }
  })
}

const COMPLETED_PLAN_STAGE_WORKFLOW_FIELDS = Object.freeze([
  'planModeSelected',
  'planTurnObserved',
  'planProviderReceiptBound',
  'planProviderReceiptScopeBound',
  'planTaskInspectionReadsBound',
  'planTaskInspectionExpectedReadCount',
  'planTaskInspectionReadAttemptCount',
  'planTaskInspectionReadAttemptDigest',
  'planArtifactBound',
  'planArtifactContentHash',
  'planArtifactByteSize',
  'planArtifactResultItemDigest',
  'planArtifactRevisionSequenceDigest',
  'planArtifactFileIdentityDigest',
  'planArtifactExcludeSha256',
  'planArtifactCallCount',
  'planArtifactResultCount',
  'planThreadIdHash'
])

export function completedPlanStageSnapshot(report) {
  const workflow = report?.workflow && typeof report.workflow === 'object' &&
      !Array.isArray(report.workflow)
    ? report.workflow
    : {}
  const planReasoningEffortBound = report?.provider?.planReasoningEffortBound === true
  if (!planReasoningEffortBound || ![
    'planModeSelected',
    'planTurnObserved',
    'planProviderReceiptBound',
    'planProviderReceiptScopeBound',
    'planTaskInspectionReadsBound',
    'planArtifactBound'
  ].every((key) => workflow[key] === true)) return null
  const completedFields = { planReasoningEffortBound }
  for (const key of COMPLETED_PLAN_STAGE_WORKFLOW_FIELDS) {
    if (Object.prototype.hasOwnProperty.call(workflow, key)) {
      completedFields[key] = workflow[key]
    }
  }
  return sanitizeMilestoneAStageSnapshot({
    stage: 'plan-turn',
    checkIds: ['ordinary-agent-workflow'],
    completedFields
  })
}

const ORDINARY_FAILURE_SNAPSHOT_CHECK_IDS = Object.freeze([
  'ordinary-agent-workflow',
  'parent-owned-repository-test-cross-check',
  'real-repository-test',
  'bounded-subagent',
  'git-skill-mcp-research-writing'
])

const ORDINARY_FAILURE_SNAPSHOT_WORKFLOW_FIELDS = Object.freeze([
  'ordinaryCandidateProgressObserved',
  'ordinaryCandidateTerminalObserved',
  'ordinaryCandidateToolAttemptCount',
  'ordinaryCandidateSuccessfulToolExecutionCount',
  'ordinaryCandidateFailedToolResultCount',
  'ordinaryCandidateUnsettledToolResultCount',
  'ordinaryCandidateOpenToolCallCount',
  'ordinaryCandidateProgressDigest',
  'ordinaryCandidateProviderAttemptRecordCount',
  'ordinaryCandidateProviderTerminalRecordCount',
  'ordinaryCandidateProviderReceiptCountersBound',
  'ordinaryCandidateProviderLogicalCallCount',
  'ordinaryCandidateProviderAttemptCount',
  'readObserved',
  'planObserved',
  'todoObserved',
  'writeObserved',
  'realTestObserved',
  'subagentObserved',
  'taskProfileBound',
  'taskForegroundBound',
  'taskStepLimitBound',
  'taskTimeBudgetBound',
  'successfulToolResultsObserved',
  'todosCompleted',
  'exactlyOneBoundedSubagentCompleted',
  'isolatedGitRepositoryObserved',
  'protectedRepositoryInputsBound',
  'onlyIntendedSourceChanged',
  'parentOwnedTestCrossCheckPassed',
  'gitCommandObserved',
  'skillObserved',
  'ordinaryMCPObserved',
  'researchWritingObserved',
  'childSubagentProviderReceiptBound',
  'childSubagentReviewReadAttemptCount',
  'childSubagentReviewReadsBound'
])

function ordinaryWorkflowFailureStageSnapshot(report) {
  const workflow = report?.workflow && typeof report.workflow === 'object' &&
      !Array.isArray(report.workflow)
    ? report.workflow
    : {}
  const repository = report?.repository && typeof report.repository === 'object' &&
      !Array.isArray(report.repository)
    ? report.repository
    : {}
  const completedFields = {}
  for (const key of ORDINARY_FAILURE_SNAPSHOT_WORKFLOW_FIELDS) {
    if (Object.prototype.hasOwnProperty.call(workflow, key)) {
      completedFields[key] = workflow[key]
    }
  }
  for (const key of [
    'finalOnlyIntendedSourceChanged',
    'finalExpectedSourceBound',
    'finalExpectedSourceFileCount',
    'finalExpectedSourceMismatchCount',
    'finalExpectedSourceDigest',
    'finalObservedSourceDigest'
  ]) {
    if (Object.prototype.hasOwnProperty.call(repository, key)) {
      completedFields[key] = repository[key]
    }
  }
  return sanitizeMilestoneAStageSnapshot({
    stage: 'ordinary-agent-workflow-failure',
    checkIds: ORDINARY_FAILURE_SNAPSHOT_CHECK_IDS,
    completedFields
  })
}

export function finalizeMilestoneAReport(input, { requiredCheckIds = REQUIRED_CHECK_IDS } = {}) {
  const report = input && typeof input === 'object' && !Array.isArray(input)
    ? JSON.parse(JSON.stringify(input))
    : {}
  const required = [...new Set((Array.isArray(requiredCheckIds) ? requiredCheckIds : REQUIRED_CHECK_IDS)
    .map((value) => typeof value === 'string' ? value.trim() : '')
    .filter(Boolean))]
  const ownedCheckIds = new Set([...required, ...MILESTONE_A_AUXILIARY_CHECK_IDS])
  const rawChecks = Array.isArray(report.checks) ? report.checks : []
  const safeDiagnosticCode = (value) => typeof value === 'string' &&
    /^[A-Za-z0-9_.:-]{1,128}$/u.test(value)
    ? value
    : ''
  const directFailureBlocker = rawChecks.find((raw) =>
    raw && typeof raw === 'object' && !Array.isArray(raw) &&
    ownedCheckIds.has(typeof raw.id === 'string' ? raw.id.trim() : '') &&
    raw.status === 'failed' && safeDiagnosticCode(raw.message)
  )?.message || rawChecks.find((raw) =>
    raw && typeof raw === 'object' && !Array.isArray(raw) &&
    ownedCheckIds.has(typeof raw.id === 'string' ? raw.id.trim() : '') &&
    raw.status === 'live_blocked' && safeDiagnosticCode(raw.message)
  )?.message || ''
  const directNonzeroCompactionFailure = rawChecks.find((raw) =>
    raw && typeof raw === 'object' && !Array.isArray(raw) &&
    raw.id === 'nonzero-compaction' && raw.status === 'failed' &&
    safeDiagnosticCode(raw.message)
  ) ? NONZERO_COMPACTION_FAILURE_REASON_CODE : ''
  const safeExecutionBlocker = safeDiagnosticCode(directNonzeroCompactionFailure) ||
    safeDiagnosticCode(report.executionBlocker) ||
    safeDiagnosticCode(directFailureBlocker)
  const firstLaunchPrerequisitesPassed = FIRST_LAUNCH_PREREQUISITE_CHECK_IDS.every((id) =>
    rawChecks.some((raw) => raw?.id === id && raw?.status === 'passed')
  )
  const hasComposerTerminalCheck = rawChecks.some((raw) =>
    raw?.id === 'composer-workflow-submit' && raw?.status !== 'skipped'
  )
  if (report.executionBlockerClassification === 'product_or_harness_failure' &&
      firstLaunchPrerequisitesPassed && !hasComposerTerminalCheck) {
    const skippedComposerCheck = rawChecks.find((raw) =>
      raw?.id === 'composer-workflow-submit' && raw?.status === 'skipped'
    )
    if (skippedComposerCheck) {
      skippedComposerCheck.status = 'failed'
      skippedComposerCheck.message = safeExecutionBlocker || 'milestone_a_execution_failed'
    } else {
      rawChecks.push({
        id: 'composer-workflow-submit',
        status: 'failed',
        message: safeExecutionBlocker || 'milestone_a_execution_failed'
      })
    }
  }
  const merged = new Map()
  for (const raw of rawChecks) {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) continue
    const rawId = typeof raw.id === 'string' ? raw.id.trim() : ''
    const id = /^[a-z0-9][a-z0-9-]{0,127}$/u.test(rawId)
      ? rawId
      : rawId
        ? 'invalid-check-id'
        : ''
    if (!id) continue
    const status = Object.hasOwn(CHECK_STATUS_ORDER, raw.status) ? raw.status : 'failed'
    const projectedMessage = typeof raw.message === 'string' && raw.message.length <= 512
      ? raw.message
      : ''
    const candidate = {
      id,
      status,
      message: status === 'passed'
        ? 'passed'
        : id === 'nonzero-compaction' && status === 'failed'
          ? NONZERO_COMPACTION_FAILURE_REASON_CODE
        : /^[A-Za-z0-9_.:-]+$/u.test(projectedMessage)
          ? projectedMessage
          : safeExecutionBlocker || 'check_projection_rejected'
    }
    const previous = merged.get(id)
    if (!previous) {
      merged.set(id, candidate)
      continue
    }
    if (CHECK_STATUS_ORDER[candidate.status] > CHECK_STATUS_ORDER[previous.status]) {
      merged.set(id, {
        ...candidate,
        message: previous.status === candidate.status
          ? candidate.message
          : 'check status normalized'
      })
    }
  }
  const fallbackStatus = report.executionBlockerClassification === 'external_prerequisite'
    ? 'live_blocked'
    : 'skipped'
  for (const id of required) {
    if (!merged.has(id)) {
      merged.set(id, {
        id,
        status: fallbackStatus,
        message: 'stage did not reach this required check'
      })
    }
  }
  report.checks = [...merged.values()]
  const allowed = ownedCheckIds
  const unknown = report.checks.some((item) => !allowed.has(item.id))
  if (unknown) {
    report.checks = report.checks.map((item) => allowed.has(item.id)
      ? item
      : { ...item, status: 'failed', message: 'unknown check id rejected' })
  }
  const failedCheckIds = report.checks.filter((item) => item.status === 'failed').map((item) => item.id)
  const skippedCheckIds = report.checks.filter((item) => item.status === 'skipped').map((item) => item.id)
  const liveBlockedCheckIds = report.checks.filter((item) => item.status === 'live_blocked').map((item) => item.id)
  const nonPass = report.checks
    .filter((item) => item.status !== 'passed')
    .map((item) => item.id)
  report.failedCheckIds = failedCheckIds
  report.skippedCheckIds = skippedCheckIds
  report.liveBlockedCheckIds = liveBlockedCheckIds
  report.failureCheckIds = nonPass
  report.failureCheckIdsSemantics = 'all_non_pass'
  report.blockers = [...nonPass]
  delete report.outputPath
  report.missingExternalInputs = liveBlockedCheckIds.map((id) => ({
    id,
    status: 'live_blocked',
    reason: 'required evidence is externally blocked'
  }))
  const sanitizedSnapshots = Array.isArray(report.stageSnapshots)
    ? report.stageSnapshots.map((snapshot) => sanitizeMilestoneAStageSnapshot(
      snapshot,
      [...required, ...MILESTONE_A_AUXILIARY_CHECK_IDS]
    )).filter(Boolean)
    : []
  const snapshots = []
  const snapshotIndexes = new Map()
  for (const snapshot of sanitizedSnapshots) {
    const existingIndex = snapshotIndexes.get(snapshot.stage)
    if (existingIndex === undefined) {
      snapshotIndexes.set(snapshot.stage, snapshots.length)
      snapshots.push(snapshot)
      continue
    }
    const existing = snapshots[existingIndex]
    const merged = sanitizeMilestoneAStageSnapshot({
      stage: snapshot.stage,
      checkIds: [...existing.checkIds, ...snapshot.checkIds],
      completedFields: recoverWorkflowFromStageSnapshots(
        existing.completedFields,
        [snapshot]
      )
    }, [...required, ...MILESTONE_A_AUXILIARY_CHECK_IDS])
    if (merged) snapshots[existingIndex] = merged
  }
  const workflow = recoverWorkflowFromStageSnapshots(
    report.workflow && typeof report.workflow === 'object' ? report.workflow : {},
    snapshots
  )
  workflow.planObservationFailureReasonCode = planObservationReasonCode(
    workflow.planObservationFailureReasonCode
  )
  workflow.planObservationFailurePhase = safePlanObservationPhase(
    workflow.planObservationFailurePhase === undefined
      ? 'none'
      : workflow.planObservationFailurePhase
  )
  workflow.planObservationFailureOperation = safePlanObservationOperation(
    workflow.planObservationFailureOperation === undefined
      ? 'none'
      : workflow.planObservationFailureOperation
  )
  workflow.planObservationFailureExpressionStage = planObservationExpressionStage(
    workflow.planObservationFailureExpressionStage === undefined
      ? 'none'
      : workflow.planObservationFailureExpressionStage
  )
  workflow.bashHostObservationReasonCode =
    HOST_TOOL_EXECUTION_OBSERVATION_REASON_CODES.includes(
      workflow.bashHostObservationReasonCode
    )
      ? workflow.bashHostObservationReasonCode
      : 'not_observed'
  workflow.bashHostObservationTransportStatus = safeHostObservationTransportStatus(
    workflow.bashHostObservationTransportStatus
  )
  workflow.bashHostObservationAttemptCount = safeHostObservationAttemptCount(
    workflow.bashHostObservationAttemptCount
  )
  workflow.bashHostObservationDigest = safeHostObservationDigest(
    workflow.bashHostObservationDigest
  )
  workflow.bashHostAuthorityBindingDigest = safeHostObservationDigest(
    workflow.bashHostAuthorityBindingDigest
  )
  workflow.gitHostObservationReasonCode =
    HOST_TOOL_EXECUTION_OBSERVATION_REASON_CODES.includes(
      workflow.gitHostObservationReasonCode
    )
      ? workflow.gitHostObservationReasonCode
      : 'not_observed'
  workflow.gitHostObservationTransportStatus = safeHostObservationTransportStatus(
    workflow.gitHostObservationTransportStatus
  )
  workflow.gitHostObservationAttemptCount = safeHostObservationAttemptCount(
    workflow.gitHostObservationAttemptCount
  )
  workflow.gitHostObservationDigest = safeHostObservationDigest(
    workflow.gitHostObservationDigest
  )
  workflow.gitHostAuthorityBindingDigest = safeHostObservationDigest(
    workflow.gitHostAuthorityBindingDigest
  )
  const passedCheck = (id) => report.checks.some((item) => item.id === id && item.status === 'passed')
  if (passedCheck('ordinary-agent-workflow')) {
    workflow.ordinaryWorkflowCompleted = true
    workflow.agentModeRestored = true
    workflow.sameThreadPlanAgent = true
    workflow.readObserved = true
    workflow.planObserved = true
    workflow.todoObserved = true
    workflow.writeObserved = true
    workflow.successfulToolResultsObserved = true
    workflow.todosCompleted = true
    workflow.isolatedGitRepositoryObserved = true
    workflow.protectedRepositoryInputsBound = true
    workflow.onlyIntendedSourceChanged = true
  }
  if (passedCheck('parent-owned-repository-test-cross-check')) {
    workflow.parentOwnedTestCrossCheckPassed = true
  }
  if (passedCheck('real-repository-test')) workflow.realTestObserved = true
  if (passedCheck('bounded-subagent')) {
    workflow.subagentObserved = true
    workflow.exactlyOneBoundedSubagentCompleted = true
  }
  if (passedCheck('git-skill-mcp-research-writing')) {
    workflow.gitCommandObserved = true
    workflow.skillObserved = true
    workflow.ordinaryMCPObserved = true
    workflow.researchWritingObserved = true
  }
  if (passedCheck('long-context-continuation')) workflow.longContextContinuationObserved = true
  if (passedCheck('normal-first-quit')) workflow.normalFirstQuitObserved = true
  if (passedCheck('protected-funds-source-unavailable')) {
    workflow.protectedFundsSourceUnavailableObserved = true
  }
  report.workflow = workflow
  const projectionClass = COMPACTION_PROJECTION_CLASSES.has(
    workflow.manualCompactionProjectionClass
  )
    ? workflow.manualCompactionProjectionClass
    : workflow.manualCompactionReplacedTokens > 0
      ? 'ordinary_exact'
      : 'not_observed'
  const ordinaryCompactionProofClosed = projectionClass === 'ordinary_exact' &&
    workflow.manualCompactionAuto === false &&
    Number.isSafeInteger(workflow.manualCompactionReplacedTokens) &&
    workflow.manualCompactionReplacedTokens > 0 &&
    /^[0-9a-f]{64}$/u.test(String(workflow.manualCompactionSourceDigest || '')) &&
    /^[0-9a-f]{64}$/u.test(String(workflow.manualCompactionSourceItemIdsDigest || '')) &&
    workflow.manualCompactionSourceAncestryBound === true
  const caseCompactionProofClosed = projectionClass === 'case_bound_typed' &&
    workflow.manualCompactionAuto === false &&
    workflow.manualCompactionReplacedTokens === 0 &&
    /^[0-9a-f]{64}$/u.test(String(workflow.manualCompactionSourceDigest || '')) &&
    workflow.manualCompactionSourceItemIdsDigest === '' &&
    workflow.manualCompactionNonzeroBound === true &&
    workflow.manualCompactionSourceAncestryBound === true
  const compactionBaselineClosed = workflow.manualCompactionBaselineBound === true &&
    Number.isSafeInteger(workflow.manualCompactionBaselineCount) &&
    workflow.manualCompactionBaselineCount >= 0 &&
    /^[0-9a-f]{64}$/u.test(String(workflow.manualCompactionBaselineDigest || '')) &&
    /^[0-9a-f]{64}$/u.test(String(workflow.manualCompactionCandidateDigest || '')) &&
    workflow.manualCompactionNewAfterBaseline === true
  const compactionProofClosed = passedCheck('nonzero-compaction') &&
    compactionBaselineClosed &&
    workflow.manualCompactionObserved === true &&
    (ordinaryCompactionProofClosed || caseCompactionProofClosed)
  // A failed/non-typed compaction stage is diagnostic only; it must not look
  // like a completed composer-compaction-submit snapshot after finalization.
  if (!compactionProofClosed) {
    for (let index = snapshots.length - 1; index >= 0; index -= 1) {
      if (snapshots[index]?.stage === 'composer-compaction-submit') snapshots.splice(index, 1)
    }
  }
  const upsertCompletedStageSnapshot = (stage, checkIds, completedFields) => {
    const completion = sanitizeMilestoneAStageSnapshot({
      stage,
      checkIds,
      completedFields
    }, [...required, ...MILESTONE_A_AUXILIARY_CHECK_IDS])
    if (!completion) return
    const existingIndex = snapshots.findIndex((snapshot) => snapshot.stage === stage)
    if (existingIndex < 0) {
      snapshots.push(completion)
      return
    }
    const existing = snapshots[existingIndex]
    const merged = sanitizeMilestoneAStageSnapshot({
      stage,
      checkIds: [...existing.checkIds, ...completion.checkIds],
      completedFields: {
        ...existing.completedFields,
        ...completion.completedFields
      }
    }, [...required, ...MILESTONE_A_AUXILIARY_CHECK_IDS])
    if (merged) snapshots[existingIndex] = merged
  }
  if (passedCheck('long-context-continuation') &&
      workflow.longContextContinuationObserved === true) {
    upsertCompletedStageSnapshot(
      'long-context-continuation',
      ['long-context-continuation'],
      { longContextContinuationObserved: true }
    )
  }
  if (passedCheck('normal-first-quit') && workflow.normalFirstQuitObserved === true) {
    upsertCompletedStageSnapshot(
      'normal-first-quit',
      ['normal-first-quit'],
      { normalFirstQuitObserved: true }
    )
  }
  const snapshotStages = new Set(snapshots.map((snapshot) => snapshot.stage))
  if (!snapshotStages.has('plan-turn')) {
    const snapshot = completedPlanStageSnapshot(report)
    if (snapshot) snapshots.push(snapshot)
  }
  if (workflow.firstLaunchObserved === true &&
      FIRST_LAUNCH_PREREQUISITE_CHECK_IDS.every(passedCheck) &&
      !snapshotStages.has('first-launch-prerequisites')) {
    const snapshot = firstLaunchPrerequisiteStageSnapshot(report)
    if (snapshot) snapshots.push(snapshot)
  }
  if (report.checks.some((item) =>
        item.id === 'ordinary-agent-workflow' && item.status === 'failed'
      ) && (workflow.ordinaryCandidateProgressObserved === true ||
        workflow.ordinaryCandidateTerminalObserved === true ||
        workflow.ordinaryWorkflowDisposition === 'completed')) {
    const snapshot = ordinaryWorkflowFailureStageSnapshot(report)
    if (snapshot) {
      upsertCompletedStageSnapshot(
        snapshot.stage,
        snapshot.checkIds,
        snapshot.completedFields
      )
    }
  }
  const ordinaryFunctionalCheckIds = [
    'real-repository-test',
    'bounded-subagent',
    'git-skill-mcp-research-writing'
  ]
  const ordinaryFunctionalCompleted =
    !passedCheck('ordinary-agent-workflow') &&
    passedCheck('parent-owned-repository-test-cross-check') &&
    ordinaryFunctionalCheckIds.every(passedCheck) &&
    [
      'agentModeRestored',
      'sameThreadPlanAgent',
      'readObserved',
      'planObserved',
      'todoObserved',
      'writeObserved',
      'realTestObserved',
      'subagentObserved',
      'successfulToolResultsObserved',
      'todosCompleted',
      'exactlyOneBoundedSubagentCompleted',
      'isolatedGitRepositoryObserved',
      'protectedRepositoryInputsBound',
      'onlyIntendedSourceChanged',
      'parentOwnedTestCrossCheckPassed',
      'gitCommandObserved',
      'skillObserved',
      'ordinaryMCPObserved',
      'researchWritingObserved'
    ].every((key) => workflow[key] === true)
  if (ordinaryFunctionalCompleted &&
      !snapshotStages.has('ordinary-agent-functional-workflow')) {
    snapshots.push(generatedMilestoneStageSnapshot(
      report,
      'ordinary-agent-functional-workflow',
      ordinaryFunctionalCheckIds
    ))
  }
  if (workflow.ordinaryWorkflowCompleted === true && !snapshotStages.has('ordinary-agent-workflow')) {
    snapshots.push(generatedMilestoneStageSnapshot(report, 'ordinary-agent-workflow', [
      'ordinary-agent-workflow', 'real-repository-test', 'bounded-subagent'
    ]))
  }
  if (workflow.longContextContinuationObserved === true && !snapshotStages.has('long-context-continuation')) {
    snapshots.push(generatedMilestoneStageSnapshot(report, 'long-context-continuation', [
      'long-context-continuation'
    ]))
  }
  if (workflow.protectedFundsSourceUnavailableObserved === true &&
      !snapshotStages.has('protected-funds-source-unavailable')) {
    snapshots.push(generatedMilestoneStageSnapshot(
      report,
      'protected-funds-source-unavailable',
      ['protected-funds-source-unavailable']
    ))
  }
  if (compactionProofClosed && !snapshotStages.has('composer-compaction-submit')) {
    snapshots.push(generatedMilestoneStageSnapshot(report, 'composer-compaction-submit', [
      'composer-compaction-submit', 'nonzero-compaction'
    ]))
  }
  if (workflow.relaunchProviderContinuationObserved === true && !snapshotStages.has('relaunch-provider-continuation')) {
    snapshots.push(generatedMilestoneStageSnapshot(report, 'relaunch-provider-continuation', [
      'fresh-packaged-relaunch', 'relaunch-provider-continuation'
    ]))
  }
  report.stageSnapshots = snapshots.filter(Boolean)
  const requiredPassed = required.every((id) => report.checks.some((item) => item.id === id && item.status === 'passed'))
  report.passed = nonPass.length === 0 && requiredPassed
  report.status = report.passed ? 'passed' : failedCheckIds.length > 0 ? 'failed' : 'live_blocked'
  report.executionBlocker = report.passed
    ? ''
    : safeExecutionBlocker || 'milestone_a_execution_failed'
  return report
}

function commercialReleaseEvidence(artifact, releaseAuthority) {
  const nativeReceiptStatus = artifact?.developmentNativeDisposition === true || artifact?.blocked === true
    ? 'live_blocked'
    : 'failed'
  const checks = [
    check(
      'formal-release-publication-authority',
      releaseAuthority?.ok === true,
      releaseAuthority?.blocker || 'formal release publication authority verified',
      releaseAuthority?.blocked === true ? 'live_blocked' : 'failed'
    ),
    check(
      'controlled-release-native-receipt',
      (artifact?.controlledReleaseNativeReceipt === true || artifact?.controlledCoreQualification === true),
      artifact?.developmentNativeDisposition === true
        ? 'development package is valid for Milestone A but requires a controlled native receipt before commercial publication'
        : 'commercial publication requires the exact controlled Full receipt or Core qualification',
      nativeReceiptStatus
    )
  ]
  const failed = checks.some((item) => item.status === 'failed')
  const passed = REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS.every((id) =>
    checks.some((item) => item.id === id && item.status === 'passed')
  )
  return {
    requiredForMilestoneA: false,
    requiredForCommercialPublication: true,
    status: passed ? 'passed' : failed ? 'failed' : 'live_blocked',
    passed,
    checks,
    failureCheckIds: checks
      .filter((item) => item.status !== 'passed')
      .map((item) => item.id),
    missingExternalInputs: checks
      .filter((item) => item.status === 'live_blocked')
      .map((item) => ({ id: item.id, status: item.status, reason: item.message }))
  }
}

export function emptyPackagedStartupTraceEvidence(enabled = false) {
  return Object.freeze({
    enabled: enabled === true,
    checkpointCount: 0,
    observedCheckpointCodes: Object.freeze([]),
    lastCheckpoint: 'not_observed',
    lastElapsedMs: 0,
    checkpointElapsedMs: Object.freeze({})
  })
}

export function createPackagedStartupTraceRecorder() {
  const partial = { stdout: '', stderr: '' }
  const observedCheckpointCodes = []
  const checkpointElapsedMs = Object.create(null)
  let checkpointCount = 0
  let lastCheckpoint = 'not_observed'
  let lastElapsedMs = 0

  const acceptLine = (line) => {
    if (typeof line !== 'string' || line.length === 0) return
    const safePrefix = line.slice(0, 256)
    const match = safePrefix.match(
      /^\[analytix\] \[startup\] \[\+\s*([0-9]{1,9})ms\] (.+?)(?: — detail: |$)/u
    )
    if (!match) return
    if (!Object.hasOwn(PACKAGED_STARTUP_TRACE_CHECKPOINT_CODES, match[2])) return
    const checkpoint = PACKAGED_STARTUP_TRACE_CHECKPOINT_CODES[match[2]]
    const elapsedMs = Number(match[1])
    if (typeof checkpoint !== 'string' ||
        !Number.isSafeInteger(elapsedMs) || elapsedMs < 0) return
    checkpointCount = Math.min(checkpointCount + 1, Number.MAX_SAFE_INTEGER)
    if (!Object.hasOwn(checkpointElapsedMs, checkpoint)) {
      observedCheckpointCodes.push(checkpoint)
    }
    checkpointElapsedMs[checkpoint] = elapsedMs
    lastCheckpoint = checkpoint
    lastElapsedMs = elapsedMs
  }

  const accept = (source, chunk) => {
    if (source !== 'stdout' && source !== 'stderr') return
    const text = Buffer.isBuffer(chunk) ? chunk.toString('utf8') : String(chunk ?? '')
    const lines = `${partial[source]}${text}`.split(/\r?\n/u)
    const remainder = lines.pop() || ''
    partial[source] = remainder.length <= PACKAGED_STARTUP_TRACE_MAX_PARTIAL_LENGTH
      ? remainder
      : ''
    for (const line of lines) acceptLine(line)
  }

  const finish = (source) => {
    if (source !== 'stdout' && source !== 'stderr') return
    acceptLine(partial[source])
    partial[source] = ''
  }

  const evidence = () => Object.freeze({
    enabled: true,
    checkpointCount,
    observedCheckpointCodes: Object.freeze([...observedCheckpointCodes]),
    lastCheckpoint,
    lastElapsedMs,
    checkpointElapsedMs: Object.freeze({ ...checkpointElapsedMs })
  })

  return Object.freeze({ accept, finish, evidence })
}

export function packagedStartupTraceEvidence(child) {
  return packagedStartupTraceRecorders.get(child)?.evidence() ||
    emptyPackagedStartupTraceEvidence()
}

function launchPackagedApp({ appPath, target, debugPort, childEnv }) {
  const startupTrace = createPackagedStartupTraceRecorder()
  const child = spawn(
    executablePath(appPath, target),
    [`--remote-debugging-port=${debugPort}`],
    {
      cwd: process.cwd(),
      env: {
        ...childEnv,
        ANALYTIX_STARTUP_TRACE: '1'
      },
      stdio: ['ignore', 'pipe', 'pipe']
    }
  )
  packagedStartupTraceRecorders.set(child, startupTrace)
  child.stdout?.on('data', (chunk) => startupTrace.accept('stdout', chunk))
  child.stdout?.once('end', () => startupTrace.finish('stdout'))
  child.stderr?.on('data', (chunk) => startupTrace.accept('stderr', chunk))
  child.stderr?.once('end', () => startupTrace.finish('stderr'))
  return child
}

function harnessManifestEvidence() {
  const entries = [
    ['runtime-go-packaged-milestone-a.mjs', fileURLToPath(import.meta.url)],
    ['local-provider-acceptance.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-acceptance.mjs')],
    ['local-provider-credential-scan.mjs', resolve(process.cwd(), 'scripts/lib/local-provider-credential-scan.mjs')],
    ['runtime-go-packaged-qa.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-qa.mjs')],
    ['runtime-go-live-evidence-collector.mjs', resolve(process.cwd(), 'scripts/runtime-go-live-evidence-collector.mjs')],
    ['runtime-go-validation-command.mjs', resolve(process.cwd(), 'scripts/runtime-go-validation-command.mjs')],
    ['runtime-go-cutover-report.mjs', resolve(process.cwd(), 'scripts/runtime-go-cutover-report.mjs')],
    ['runtime-go-packaged-gui-smoke.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-gui-smoke.mjs')],
    ['runtime-go-packaged-session-soak.mjs', resolve(process.cwd(), 'scripts/runtime-go-packaged-session-soak.mjs')],
    ['runtime-go-rollback-retirement-evidence.mjs', resolve(process.cwd(), 'scripts/runtime-go-rollback-retirement-evidence.mjs')],
    ['packaging-config.test.ts', resolve(process.cwd(), 'src/main/packaging-config.test.ts')],
    ['after-pack.cjs', resolve(process.cwd(), 'scripts/after-pack.cjs')],
    ['packaged-release-publication-authority.mjs', resolve(process.cwd(), 'scripts/lib/packaged-release-publication-authority.mjs')],
    ['publish-r2.mjs', resolve(process.cwd(), 'scripts/publish-r2.mjs')],
    ['strict-json.cjs', resolve(process.cwd(), 'scripts/lib/strict-json.cjs')],
    ['macos-signing-policy.cjs', resolve(process.cwd(), 'scripts/macos-signing-policy.cjs')],
    ['macos-signing-policy.json', resolve(process.cwd(), 'scripts/macos-signing-policy.json')],
    ['entitlements.mac.native-helper.plist', resolve(process.cwd(), 'build/entitlements.mac.native-helper.plist')],
    ['package.json', resolve(process.cwd(), 'package.json')],
    ['package-lock.json', resolve(process.cwd(), 'package-lock.json')],
    ...CACHE_HELPER_RUNTIME_SOURCE_CLOSURE.map((relativePath) => [
      relativePath,
      repositorySourcePath(relativePath)
    ])
  ].map(([name, path]) => {
    const evidence = hashRegularFile(path)
    return {
      name,
      regular: evidence.regular,
      byteLength: evidence.byteLength,
      sha256: evidence.sha256
    }
  })
  const harnessSourceRootBound = (() => {
    try {
      return realpathSync(fileURLToPath(import.meta.url)) ===
        realpathSync(repositorySourcePath('scripts/runtime-go-packaged-milestone-a.mjs'))
    } catch {
      return false
    }
  })()
  return {
    scriptSha256:
      entries.find((item) => item.name === 'runtime-go-packaged-milestone-a.mjs')?.sha256 || '',
    contractManifestSha256: sha256(canonicalJSON(entries)),
    contractManifestBound: harnessContractManifestBound(entries, harnessSourceRootBound),
    harnessSourceRootBound,
    sourceClosureBound: false,
    sourceClosureSnapshotDigest: '',
    packagedSourceClosureSnapshotDigest: '',
    entries
  }
}

export function harnessContractManifestBound(entries, harnessSourceRootBound) {
  if (!harnessSourceRootBound || !Array.isArray(entries) || entries.length === 0) return false
  const counts = new Map()
  for (const entry of entries) {
    if (!entry || typeof entry.name !== 'string' || !entry.name ||
      entry.regular !== true || !Number.isSafeInteger(entry.byteLength) ||
      entry.byteLength <= 0 || !/^[0-9a-f]{64}$/.test(String(entry.sha256 || ''))) {
      return false
    }
    counts.set(entry.name, (counts.get(entry.name) || 0) + 1)
  }
  if ([...counts.values()].some((count) => count !== 1)) return false
  return CACHE_HELPER_RUNTIME_SOURCE_CLOSURE.every((relativePath) =>
    counts.get(relativePath) === 1
  )
}

export function harnessSourceClosureBound(manifest, artifact, sourceCommit) {
  return harnessContractManifestBound(
    manifest?.entries,
    manifest?.harnessSourceRootBound === true
  ) && manifest?.contractManifestBound === true &&
    /^[0-9a-f]{40}$/.test(String(sourceCommit || '')) &&
    /^[0-9a-f]{64}$/.test(String(manifest?.contractManifestSha256 || '')) &&
    manifest.contractManifestSha256 === sha256(canonicalJSON(manifest.entries)) &&
    artifact?.worktreeSnapshotBinding?.ok === true &&
    artifact.worktreeSnapshotBinding?.matched === true &&
    artifact.sourceCommit === sourceCommit
}

export function configuredExternalRepositoryAcceptance(options = {}) {
  const configuredPath = (key, optionName, environmentName) => {
    const supplied = typeof options[key] === 'string' ? options[key].trim() : ''
    return supplied || optionValue(optionName, process.env[environmentName] || '').trim()
  }
  const repositoryPath = configuredPath(
    'repositoryPath',
    '--repository-path',
    'ANALYTIX_RUNTIME_GO_MILESTONE_A_REPOSITORY_PATH'
  )
  const contractPath = configuredPath(
    'contractPath',
    '--repository-contract',
    'ANALYTIX_RUNTIME_GO_MILESTONE_A_REPOSITORY_CONTRACT'
  )
  const provenancePath = configuredPath(
    'provenancePath',
    '--repository-provenance',
    'ANALYTIX_RUNTIME_GO_MILESTONE_A_REPOSITORY_PROVENANCE'
  )
  const ownerRoot = configuredPath(
    'ownerRoot',
    '--repository-owner-root',
    'ANALYTIX_RUNTIME_GO_MILESTONE_A_REPOSITORY_OWNER_ROOT'
  )
  const verificationMode = options.verificationMode === 'synthetic-parser-only'
    ? 'synthetic-parser-only'
    : 'formal-fresh-origin-verified'
  const preflightOnly = options.preflightOnly === true
  const base = {
    ok: false,
    blocked: false,
    blocker: '',
    mode: 'external-preexisting-real-repository',
    repositoryPathHash: repositoryPath ? sha256(repositoryPath) : '',
    contractPathHash: contractPath ? sha256(contractPath) : '',
    provenancePathHash: provenancePath ? sha256(provenancePath) : '',
    ownerRootPathHash: ownerRoot ? sha256(ownerRoot) : '',
    contractSha256: '',
    provenanceSha256: '',
    baselineCommit: '',
    baselineTree: '',
    expectedChangedFileCount: 0,
    inspectPathCount: 0,
    minimumCommitCount: 0,
    originBound: false,
    freshOriginVerified: false,
    formalAcceptance: false,
    planArtifactRelativePath: '',
    planArtifactPathHash: '',
    planArtifactExcludeSha256: '',
    planArtifactExcludeBeforeSha256: '',
    preflightValidated: false,
    mutationApplied: false,
    freshOriginFetchAttemptCount: 0,
    freshOriginFetchExitCodes: [],
    authority: null
  }
  if (!repositoryPath || !contractPath || !provenancePath || !ownerRoot) {
    return {
      ...base,
      blocked: true,
      blocker: 'external_repository_path_contract_provenance_and_owner_root_required'
    }
  }
  try {
    if (preflightOnly) {
      const preflight = preflightMilestoneAExternalRepositoryAcceptance(
        repositoryPath,
        contractPath,
        provenancePath,
        ownerRoot,
        { verificationMode }
      )
      return {
        ...base,
        ok: preflight.ok === true,
        contractSha256: preflight.contractSha256,
        provenanceSha256: preflight.provenanceSha256,
        baselineCommit: preflight.baselineCommit,
        baselineTree: preflight.baselineTree,
        expectedChangedFileCount: preflight.expectedChangedFileCount,
        inspectPathCount: preflight.inspectPathCount,
        minimumCommitCount: preflight.minimumCommitCount,
        originBound: true,
        freshOriginVerified: preflight.freshOriginVerified === true,
        freshOriginFetchAttemptCount: preflight.freshOriginFetchAttemptCount,
        freshOriginFetchExitCodes: [...preflight.freshOriginFetchExitCodes],
        formalAcceptance: preflight.formalAcceptance === true,
        ownerRootBound: preflight.ownerRootBound === true,
        ownerOnlyIsolationBound: preflight.ownerOnlyIsolationBound === true,
        gitCommonDirContained: preflight.gitCommonDirContained === true,
        symlinkHardlinkIsolationBound: preflight.symlinkHardlinkIsolationBound === true,
        planArtifactExcludeBound: preflight.planArtifactExcludeBound === true,
        planArtifactPathSafe: preflight.planArtifactPathSafe === true,
        planArtifactPathHash: preflight.planArtifactPathHash,
        planArtifactExcludeBeforeSha256: preflight.planArtifactExcludeBeforeSha256,
        preflightValidated: preflight.ok === true,
        mutationApplied: preflight.mutationApplied === true
      }
    }
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      repositoryPath,
      contractPath,
      provenancePath,
      ownerRoot,
      { verificationMode }
    )
    return {
      ...base,
      ok: true,
      contractSha256: authority.contractSha256,
      provenanceSha256: authority.provenanceSha256,
      baselineCommit: authority.baselineCommit,
      baselineTree: authority.baselineTree,
      expectedChangedFileCount: authority.expectedChangedFiles.length,
      inspectPathCount: authority.inspectPaths.length,
      minimumCommitCount: authority.minimumCommitCount,
      originBound: true,
      freshOriginVerified: authority.freshOriginVerified === true,
      freshOriginFetchAttemptCount: authority.freshOriginFetchAttemptCount,
      freshOriginFetchExitCodes: [...authority.freshOriginFetchExitCodes],
      formalAcceptance: authority.acceptanceClass === 'formal-fresh-origin-verified' &&
        authority.freshOriginVerified === true,
      planArtifactRelativePath: authority.planArtifactRelativePath,
      planArtifactExcludeSha256: authority.planArtifactExcludeSha256,
      planArtifactExcludeBeforeSha256: authority.planArtifactExcludeBeforeSha256,
      mutationApplied: true,
      authority
    }
  } catch (error) {
    const code = error instanceof Error && /^[a-z0-9_]+$/u.test(error.message)
      ? error.message
      : 'external_repository_acceptance_unavailable'
    return {
      ...base,
      blocker: code,
      freshOriginFetchAttemptCount:
        Number.isSafeInteger(error?.fetchAttemptCount) ? error.fetchAttemptCount : 0,
      freshOriginFetchExitCodes: Array.isArray(error?.fetchExitCodes)
        ? error.fetchExitCodes.filter((value) => value === null || Number.isInteger(value))
        : []
    }
  }
}

function safeReportSkeleton({
  target,
  appPath,
  timeoutMs,
  sourceCommit,
  credentialEntry = configuredCredentialEntryDeclaration()
}) {
  return {
    schemaVersion: 1,
    id: 'runtime-go-packaged-milestone-a',
    stage: 'packaged-general-agent-milestone-a',
    generatedAt: new Date().toISOString(),
    sourceCommit,
    testHarnessCommit: currentGitCommit(),
    harness: {
      ...harnessManifestEvidence(),
      formalExecutionBounds: milestoneAFormalExecutionBounds()
    },
    status: 'live_blocked',
    passed: false,
    timeoutMs,
    credentialSecretsRecorded: false,
    rawProviderPayloadRecorded: false,
    syntheticProviderUsed: false,
    localProviderUsed: false,
    directRuntimeTurnDriverUsed: false,
    composerDomAndPrimaryButtonRequired: true,
    cdpAndBridgeObservationOnly: true,
    operatorCheckpoint: {
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: credentialEntry.declared,
      method: credentialEntry.method,
      automatedCredentialEntryUsed: credentialEntry.automatedCredentialEntryUsed,
      status: 'pending'
    },
    stageSnapshots: [],
    app: {
      targetKey: target.key,
      appPathHash: sha256(appPath)
    },
    repository: {
      required: true,
      mode: 'external-preexisting-real-repository',
      configured: false,
      validated: false,
      baselineValidated: false,
      finalValidated: false,
      finalOnlyIntendedSourceChanged: false,
      finalExpectedSourceBound: false,
      finalExpectedSourceFileCount: 0,
      finalExpectedSourceMismatchCount: 0,
      finalExpectedSourceDigest: '',
      finalObservedSourceDigest: '',
      finalGitStatusSha256: '',
      fixtureSubstituteRejected: false,
      repositoryPathHash: '',
      contractPathHash: '',
      provenancePathHash: '',
      ownerRootPathHash: '',
      contractSha256: '',
      provenanceSha256: '',
      baselineCommit: '',
      baselineTree: '',
      minimumCommitCount: 0,
      originBound: false,
      freshOriginVerified: false,
      formalAcceptance: false,
      preflightValidated: false,
      mutationApplied: false,
      freshOriginFetchAttemptCount: 0,
      freshOriginFetchExitCodes: [],
      ownerRootBound: false,
      ownerOnlyIsolationBound: false,
      gitCommonDirContained: false,
      symlinkHardlinkIsolationBound: false,
      planArtifactExcludeBound: false,
      planArtifactPathSafe: false,
      planArtifactPathHash: '',
      planArtifactExcludeSha256: '',
      planArtifactExcludeBeforeSha256: '',
      expectedChangedFileCount: 0,
      inspectPathCount: 0,
      preservedAfterHarnessCleanup: false,
      blocker: 'external_repository_path_contract_provenance_and_owner_root_required'
    },
    artifactRevalidation: {
      beforeFirstLaunch: { phase: 'before-first-launch', ok: false, blocker: 'not_run' },
      afterFirstExit: { phase: 'after-first-exit', ok: false, blocker: 'not_run' },
      beforeSecondLaunch: { phase: 'before-second-launch', ok: false, blocker: 'not_run' },
      afterFinalExit: { phase: 'after-final-exit', ok: false, blocker: 'not_run' }
    },
    releasePublicationAuthority: {
      requested: false,
      ok: false,
      blocked: false,
      failed: false,
      classification: 'unverified',
      blocker: 'formal_release_publication_authority_unverified'
    },
    commercialRelease: {
      requiredForMilestoneA: false,
      requiredForCommercialPublication: true,
      status: 'unverified',
      passed: false,
      checks: REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS.map((id) => ({
        id,
        status: 'unverified',
        message: 'commercial publication row was not verified'
      })),
      failureCheckIds: [...REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS],
      missingExternalInputs: []
    },
    provider: {
      configured: false,
      credentialConfigured: false,
      credentialAuthorityBound: false,
      credentialRecorded: false,
      credentialAuthority: LOCAL_PROVIDER_AUTHORITY,
      normalLocalProviderSetupObserved: false,
      family: '',
      providerIdHash: '',
      modelHash: '',
      requiredModelHash: sha256(FORMAL_MODEL),
      baseUrlOriginHash: '',
      endpointFormat: '',
      contextWindowTokensBound: false,
      settingsContextWindowTokens: 0,
      runtimeContextWindowTokens: 0,
      softThresholdTokens: 0,
      hardThresholdTokens: 0,
      reasoningEffort: FORMAL_REASONING_EFFORT,
      firstLaunchReasoningEffortSelectedThroughVisibleUi: false,
      firstLaunchReasoningControlTitleHash: '',
      secondLaunchReasoningEffortSelectedThroughVisibleUi: false,
      secondLaunchReasoningControlTitleHash: '',
      planReasoningEffortBound: false,
      ordinaryReasoningEffortBound: false,
      longContextReasoningEffortBound: false,
      relaunchReasoningEffortBound: false,
      networkTurnCompleted: false,
      threadProviderBound: false,
      threadModelBound: false,
      resultTurnProviderReceiptBound: false,
      providerAttemptTelemetryValid: false,
      providerAttemptReceiptDigest: '',
      providerLogicalCallCount: 0,
      providerAttemptCount: 0,
      successfulProviderAttemptCount: 0,
      ordinaryEvidence: null,
      longContextEvidence: null
    },
    visualEvidence: {
      firstCompleted: {
        captured: false,
        sha256: '',
        width: 0,
        height: 0,
        viewport: { width: 0, height: 0, scale: 0 }
      },
      recoveredAfterRelaunch: {
        captured: false,
        sha256: '',
        width: 0,
        height: 0,
        viewport: { width: 0, height: 0, scale: 0 }
      },
      screenshotsUsedAsFunctionalVerdict: false,
      credentialScanCovered: false
    },
    isolation: {
      cacheTmpdirVerified: false,
      systemUserDataTouched: false,
      isolatedHomeUsedByChild: false,
      isolatedLoginKeychainCreated: false,
      isolatedLoginKeychainDefaultBound: false,
      isolatedLoginKeychainUnlockCount: 0,
      isolatedLoginKeychainReusedForTwoLaunches: false,
      isolatedLoginKeychainPasswordRecorded: false,
      isolatedLoginKeychainPathHash: '',
      isolatedLoginKeychainDatabasePathHash: '',
      isolatedLoginKeychainExpectExecutableSha256: '',
      isolatedLoginKeychainSecurityExecutableSha256: '',
      chromiumTempBoundToTrustedCache: false,
      chromiumTempPathHash: '',
      userDataPathHash: '',
      runtimeDataPathHash: '',
      workspacePathHash: '',
      sameDataDirsOnRelaunch: false,
      sandboxRemoved: false
    },
    caseCapability: {
      additiveNotReplacement: true,
      bundledFundsMissingAuthorityPreconditionEstablished: false,
      bundledFundsMaterializationStateRootSha256: '',
      bundledFundsMaterializationAuthorityPathSha256: '',
      firstLaunchBundledFundsMaterializationFailureObserved: false,
      secondLaunchBundledFundsMaterializationFailureObserved: false,
      fundsTransportAbsentOnBothLaunches: false,
      privateAuthorityInputsAbsent: false,
      privateAuthorityEnvironmentKeyCount: 0,
      privateAuthoritySettingsKeyCount: 0,
      preLaunchUnexpectedUserDataEntryCount: 0,
      firstLaunchFundsExecutionUnavailable: false,
      secondLaunchFundsExecutionUnavailable: false,
      fundsDiagnosticsIndependent: false,
      protectedFundsSourceUnavailable: false,
      protectedFundsAcceptedFinalDigest: '',
      protectedFundsClaimCount: 0,
      protectedFundsReceiptCount: 0,
      protectedFundsSuccessfulExecutionCount: 0,
      restartedProtectedFundsSourceUnavailable: false,
      restartedProtectedFundsAcceptedFinalDigest: '',
      ordinaryContinuedAfterProtectedFundsBlock: false,
      ordinaryContinuedAfterRestart: false,
      ordinaryCatalogAvailable: false,
      ordinaryWorkflowCompleted: false,
      unavailableWhileOrdinaryCapabilitiesRetained: false
    },
    workflow: {
      firstLaunchObserved: false,
      composerWorkflowSubmitted: false,
      planComposerSubmitted: false,
      composerModeFailureReasonCode: '',
      composerModeFailurePhase: 'none',
      composerModeFailureOperation: 'none',
      composerModeFailureExpressionStage: 'none',
      composerSubmitFailureReasonCode: '',
      composerSubmitFailurePhase: 'none',
      composerSubmitFailureOperation: 'none',
      composerSubmitFailureExpressionStage: 'none',
      planObservationFailureReasonCode: '',
      planObservationFailurePhase: 'none',
      planObservationFailureOperation: 'none',
      planObservationFailureExpressionStage: 'none',
      planModeSelected: false,
      planTurnObserved: false,
      planProviderReceiptBound: false,
      planProviderReceiptDigest: '',
      planProviderReceiptTrace: {
        reasonCode: 'provider_receipt_not_evaluated',
        sseReasonCode: 'provider_receipt_not_observed',
        started: false,
        acknowledged: false,
        ended: false,
        eventCount: 0,
        attemptCount: 0,
        terminalCount: 0
      },
      planWorkflowDisposition: 'not_started',
      planCandidateTurnObserved: false,
      planCandidateTurnStatus: 'not_observed',
      planCandidateTurnIdHash: '',
      planCandidateTurnErrorCode: 'none',
      planCandidateTurnErrorCodeHash: '',
      planCandidateToolAttemptCount: 0,
      planCandidateSuccessfulToolExecutionCount: 0,
      planCandidateFailedToolResultCount: 0,
      planCandidateUnsettledToolResultCount: 0,
      planCandidateToolAttemptCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
      planCandidateSuccessfulToolCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
      planCandidateProviderAttemptRecordCount: 0,
      planCandidateProviderTerminalRecordCount: 0,
      planCandidateProviderReceiptCountersBound: false,
      planCandidateProviderLogicalCallCount: 0,
      planCandidateProviderAttemptCount: 0,
      planCandidateProviderAttemptStatusCounts: emptyProviderAttemptStatusCounts(),
      planCandidateTerminalReasonClass: 'none',
      planCandidateTerminalErrorItemCount: 0,
      planCandidateTerminalErrorItemAuthorityBound: false,
      planCandidateToolInventoryAvailability: 'not_observed',
      planCandidateProviderReceiptAvailability: 'not_observed',
      planCandidateInventoryDigest: '',
      planProviderReceiptScopeBound: false,
      planProviderReceiptScopeReasonCode: 'provider_receipt_scope_not_evaluated',
      planThreadIdHash: '',
      planArtifactBound: false,
      planArtifactPathHash: '',
      planArtifactContentHash: '',
      planArtifactByteSize: 0,
      planArtifactResultItemDigest: '',
      planArtifactRevisionSequenceDigest: '',
      planArtifactFileIdentityDigest: '',
      planArtifactExcludeSha256: '',
      planArtifactCallCount: 0,
      planArtifactResultCount: 0,
      exactPlanArtifactRecovered: false,
      planTaskInspectionReadsBound: false,
      planTaskInspectionReadObservationDigest: '',
      planTaskInspectionExpectedReadCount: 0,
      planTaskInspectionReadAttemptCount: 0,
      planTaskInspectionReadAttemptDigest: '',
      planLongContextReadAbsent: false,
      agentModeRestored: false,
      sameThreadPlanAgent: false,
      threadIdHash: '',
      ordinaryWorkflowDisposition: 'not_started',
      ordinaryWorkflowFailureCode: '',
      ordinaryCandidateProgressObserved: false,
      ordinaryCandidateTerminalObserved: false,
      ordinaryCandidateTurnStatus: 'not_observed',
      ordinaryCandidateTurnIdHash: '',
      ordinaryCandidateTurnErrorCode: 'none',
      ordinaryCandidateTurnErrorCodeHash: '',
      ordinaryCandidateToolAttemptCount: 0,
      ordinaryCandidateSuccessfulToolExecutionCount: 0,
      ordinaryCandidateFailedToolResultCount: 0,
      ordinaryCandidateUnsettledToolResultCount: 0,
      ordinaryCandidateOpenToolCallCount: 0,
      ordinaryCandidateActiveToolCategory: 'none',
      ordinaryCandidateToolAttemptCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
      ordinaryCandidateSuccessfulToolCategoryCounts: emptyWorkflowDiagnosticCategoryCounts(),
      ordinaryCandidateProviderAttemptRecordCount: 0,
      ordinaryCandidateProviderTerminalRecordCount: 0,
      ordinaryCandidateProviderReceiptCountersBound: false,
      ordinaryCandidateProviderLogicalCallCount: 0,
      ordinaryCandidateProviderAttemptCount: 0,
      ordinaryCandidateProviderAttemptStatusCounts: emptyProviderAttemptStatusCounts(),
      ordinaryCandidateTerminalReasonClass: 'none',
      ordinaryCandidateTerminalErrorItemCount: 0,
      ordinaryCandidateTerminalErrorItemAuthorityBound: false,
      ordinaryCandidateToolInventoryAvailability: 'not_observed',
      ordinaryCandidateProviderReceiptAvailability: 'not_observed',
      ordinaryCandidateTypedResultObserved: false,
      ordinaryCandidateTypedResultReasonCode: 'ordinary_result_not_observed',
      ordinaryCandidateTypedResultOrigin: 'not_observed',
      ordinaryCandidateTypedResultProjectionClass: 'not_observed',
      ordinaryCandidateTypedResultDigest: '',
      ordinaryCandidateTypedResultTextSha256: '',
      ordinaryCandidateTypedResultMarkerObserved: false,
      ordinaryCandidateTypedResultResearchMarkerObserved: false,
      ordinaryCandidateTypedResultWritingMarkerObserved: false,
      ordinaryCandidateInventoryDigest: '',
      ordinaryCandidateProgressDigest: '',
      ordinaryCandidateFailureClassification: 'not_observed',
      ordinaryCandidateFailureObservationCode: 'workflow_diagnostic_not_observed',
      ordinaryCandidateProviderTransportSucceeded: false,
      ordinaryCandidateUpstreamCauseConfirmed: false,
      ordinaryCandidateFailureDiagnosticDigest: '',
      ordinaryCandidateProviderFailureObserved: false,
      ordinaryCandidateProviderFailureObservationCode: 'provider_failure_not_observed',
      ordinaryCandidateProviderFailureReasonCode: 'none',
      ordinaryCandidateProviderFailureKind: 'not_observed',
      ordinaryCandidateProviderFailureStatus: null,
      ordinaryCandidateProviderFailureRetryable: null,
      ordinaryCandidateProviderFailureAuthStatus: 'not_observed',
      ordinaryCandidateProviderFailureDiagnosticDigest: '',
      ordinaryCandidateToolFailureObserved: false,
      ordinaryCandidateToolFailureObservationCode: 'tool_failure_not_observed',
      ordinaryCandidateToolFailureEvidenceCode: 'tool_failure_evidence_not_observed',
      ordinaryCandidateToolFailureGuardBound: false,
      ordinaryCandidateToolFailureGuardCount: 0,
      ordinaryCandidateToolFailureMaxStormCount: 0,
      ordinaryCandidateToolFailureGuardKind: 'none',
      ordinaryCandidateToolFailureToolCategory: 'none',
      ordinaryCandidateToolFailureToolNameHash: '',
      ordinaryCandidateInvalidToolArgumentGuardCount: 0,
      ordinaryCandidateInvalidToolArgumentMaxStormCount: 0,
      ordinaryCandidateInvalidToolArgumentToolCategory: 'none',
      ordinaryCandidateInvalidToolArgumentToolNameHash: '',
      ordinaryCandidateToolFailureTerminalReason: 'none',
      ordinaryCandidateToolFailureTerminalCode: 'none',
      ordinaryCandidateToolFailureDiagnosticDigest: '',
      readObserved: false,
      planObserved: false,
      todoObserved: false,
      writeObserved: false,
      realTestObserved: false,
      parentOwnedTestCrossCheckPassed: false,
      parentOwnedTestCommandDigest: '',
      parentOwnedTestRepositoryStable: false,
      isolatedGitRepositoryObserved: false,
      protectedRepositoryInputsBound: false,
      onlyIntendedSourceChanged: false,
      externalTestValidatorBound: false,
      npmTestLifecycleBound: false,
      npmTestProcessLineageBound: false,
      actualBashTestResultBound: false,
      hostOwnedToolInvocationBound: false,
      bashHostObservationReasonCode: 'not_observed',
      bashHostObservationTransportStatus: 0,
      bashHostObservationAttemptCount: 0,
      bashHostObservationDigest: '',
      bashHostAuthorityBindingDigest: '',
      selfReportedTestReceiptAuthoritative: false,
      selfReportedTestReceiptObserved: false,
      selfReportedTestReceiptShapeValid: false,
      subagentObserved: false,
      childSubagentProviderReceiptBound: false,
      childSubagentProviderReceiptDigest: '',
      childSubagentProviderReceiptReasonCode: 'provider_receipt_not_evaluated',
      childSubagentProviderReceiptSseReasonCode: 'provider_receipt_not_observed',
      childSubagentReviewReadAttemptCount: 0,
      childSubagentReviewReadsBound: false,
      childSubagentReviewReadObservationDigest: '',
      failedChildDiagnosticObserved: false,
      failedChildDiagnosticReasonCode: 'failed_child_not_observed',
      failedChildDiagnosticTaskAttemptCount: 0,
      failedChildDiagnosticTaskExecutionCount: 0,
      failedChildDiagnosticSummaryCount: 0,
      failedChildDiagnosticStatusBound: false,
      failedChildDiagnosticProfileBound: false,
      failedChildDiagnosticForegroundBound: false,
      failedChildDiagnosticChildTurnIdAvailable: false,
      failedChildDiagnosticParentThreadIdHash: '',
      failedChildDiagnosticParentTurnIdHash: '',
      failedChildDiagnosticParentToolCallIdHash: '',
      failedChildDiagnosticChildRunIdHash: '',
      failedChildDiagnosticChildThreadIdHash: '',
      failedChildDiagnosticChildTurnIdHash: '',
      failedChildDiagnosticThreadObserved: false,
      failedChildDiagnosticTurnReasonCode: 'failed_child_not_observed',
      failedChildDiagnosticTurnStatus: 'not_observed',
      failedChildDiagnosticTurnErrorCode: 'none',
      failedChildDiagnosticTurnErrorCodeHash: '',
      failedChildDiagnosticToolAttemptCount: 0,
      failedChildDiagnosticSuccessfulToolExecutionCount: 0,
      failedChildDiagnosticFailedToolResultCount: 0,
      failedChildDiagnosticUnsettledToolResultCount: 0,
      failedChildDiagnosticTerminalReasonClass: 'none',
      failedChildDiagnosticTerminalErrorItemCount: 0,
      failedChildDiagnosticTerminalErrorItemAuthorityBound: false,
      failedChildDiagnosticToolInventoryAvailability: 'not_observed',
      failedChildDiagnosticProviderReceiptAvailability: 'not_observed',
      failedChildDiagnosticInventoryDigest: '',
      gitCommandObserved: false,
      gitHostObservationReasonCode: 'not_observed',
      gitHostObservationTransportStatus: 0,
      gitHostObservationAttemptCount: 0,
      gitHostObservationDigest: '',
      gitHostAuthorityBindingDigest: '',
      skillObserved: false,
      ordinaryMCPObserved: false,
      ordinaryMCPResultTurnExecutionCount: 0,
      ordinaryMCPResultTurnAttemptCount: 0,
      ordinaryMCPStrictEmptyInputBound: false,
      ordinaryMCPPackageCatalogBound: false,
      ordinaryMCPPackageConfigBound: false,
      ordinaryMCPConfigSha256: '',
      ordinaryMCPHelperExecutableSha256: '',
      ordinaryMCPArgumentsDigest: '',
      resultTurnTaskAttemptCount: 0,
      resultTurnTaskExecutionCount: 0,
      resultTurnSubagentDelegationAttemptCount: 0,
      resultTurnRunSkillAttemptCount: 0,
      resultTurnRunSkillExecutionCount: 0,
      resultTurnBashAttemptCount: 0,
      resultTurnBashExecutionCount: 0,
      resultTurnMCPAttemptCount: 0,
      resultTurnMCPExecutionCount: 0,
      resultTurnFundsMCPAttemptCount: 0,
      resultTurnMalformedToolCallAttemptCount: 0,
      taskProfileBound: false,
      taskForegroundBound: false,
      taskStepLimitBound: false,
      taskTimeBudgetBound: false,
      researchWritingObserved: false,
      successfulToolResultsObserved: false,
      todosCompleted: false,
      exactlyOneBoundedSubagentCompleted: false,
      longContextContinuationObserved: false,
      longContextIdleFailureObserved: false,
      longContextIdleFailureEditorObserved: false,
      longContextIdleFailureButtonObserved: false,
      longContextIdleFailureEditorEmptyObserved: false,
      longContextIdleFailureButtonNotLoadingObserved: false,
      longContextIdleFailureButtonDisabledObserved: false,
      longContextIdleFailureButtonLabelAcceptedObserved: false,
      longContextIdleFailureWaitedMs: 0,
      longContextIdleFailureHealthOk: false,
      longContextIdleFailureRuntimeInfoOk: false,
      longContextIdleFailureRuntimeToolsOk: false,
      longContextIdleFailureRuntimeThreadListOk: false,
      longContextIdleFailureRuntimeThreadListStatus: 0,
      longContextIdleFailureRuntimeBackendTopologyOk: false,
      longContextIdleFailureRuntimeListenerCount: 0,
      longContextIdleFailureRuntimeBackendProcessCount: 0,
      longContextIdleFailureRuntimeListenerOwnedObserved: false,
      longContextIdleFailureExactPackageExecutableObserved: false,
      longContextIdleFailureRendererTargetCount: 0,
      longContextCaseWorkspaceScopeBound: false,
      longContextSubagentContinuityBound: false,
      longContextSubagentContinuityBaselineDigest: '',
      longContextSubagentContinuityProtectedFundsDigest: '',
      longContextSubagentContinuityDigest: '',
      longContextByteLength: 0,
      longContextReadLineLimit: 0,
      longContextWorkflowDisposition: 'not_started',
      longContextCandidateTurnStatus: 'not_observed',
      longContextCandidateTurnIdHash: '',
      longContextCandidateTurnErrorCode: 'none',
      longContextCandidateTurnErrorCodeHash: '',
      longContextPublicToolInventoryAvailability: 'not_observed',
      longContextPublicToolAttemptCount: 0,
      longContextPublicSuccessfulToolExecutionCount: 0,
      longContextCandidateFailedToolResultCount: 0,
      longContextCandidateUnsettledToolResultCount: 0,
      longContextCandidateProviderAttemptRecordCount: 0,
      longContextCandidateProviderTerminalRecordCount: 0,
      longContextCandidateProviderReceiptCountersBound: false,
      longContextCandidateProviderLogicalCallCount: 0,
      longContextCandidateProviderAttemptCount: 0,
      longContextCandidateTerminalReasonClass: 'none',
      longContextCandidateTerminalErrorItemCount: 0,
      longContextCandidateTerminalErrorItemAuthorityBound: false,
      longContextCandidateInventoryDigest: '',
      longContextProviderFailureObserved: false,
      longContextProviderFailureObservationCode: 'provider_failure_not_observed',
      longContextProviderFailureReasonCode: 'none',
      longContextProviderFailureKind: 'not_observed',
      longContextProviderFailureStatus: null,
      longContextProviderFailureRetryable: null,
      longContextProviderFailureAuthStatus: 'not_observed',
      longContextProviderFailureDiagnosticDigest: '',
      longContextProviderReceiptBound: false,
      longContextAcceptedFinalBound: false,
      longContextAcceptedFinalDigest: '',
      longContextProviderClosureDigest: '',
      longContextProviderReceiptReplayAttempted: false,
      longContextProviderReceiptReplayObserved: false,
      longContextProviderReceiptReplayReasonCode: 'not_attempted',
      longContextContinuationReasonCode: 'input_invalid',
      longContextContinuationFailureCode: 'continuation_input_invalid',
      longContextHostReadBound: false,
      longContextHostObservationTransportStatus: 0,
      longContextHostObservationDigest: '',
      manualCompactionBaselineBound: false,
      manualCompactionBaselineCount: 0,
      manualCompactionBaselineDigest: '',
      manualCompactionCandidateDigest: '',
      manualCompactionNewAfterBaseline: false,
      compactionCount: 0,
      manualCompactionObserved: false,
      manualCompactionAuto: null,
      manualCompactionReplacedTokens: 0,
      manualCompactionSourceDigest: '',
      manualCompactionSourceItemIdsDigest: '',
      manualCompactionSourceAncestryBound: false,
      manualCompactionNonzeroBound: false,
      manualCompactionProjectionClass: 'not_observed',
      normalFirstQuitObserved: false,
      normalFirstQuitReasonCode: 'normal_quit_not_observed',
      normalFirstQuitResidualOwnedProcessCount: 0,
      freshProcessRelaunchObserved: false,
      relaunchProviderContinuationObserved: false,
      relaunchProviderReceiptDigest: '',
      relaunchAcceptedFinalBound: false,
      relaunchAcceptedFinalDigest: '',
      relaunchProviderClosureDigest: '',
      exactThreadRecovered: false,
      exactTodosRecovered: false,
      exactSubagentsRecovered: false,
      exactCompactionsRecovered: false,
      exactResultRecovered: false,
      rendererVisibleThreadRecovered: false,
      rendererVisibleTodosRecovered: false,
      rendererVisibleSubagentRecovered: false,
      rendererVisibleCompactionRecovered: false,
      rendererVisibleResultRecovered: false,
      normalFinalQuitObserved: false,
      normalFinalQuitReasonCode: 'normal_quit_not_observed',
      zeroResidualProcesses: false
    },
    parentOwnedRepositoryTest: {
      passed: false,
      blocker: 'not_run',
      parentProcessOwned: true,
      command: 'contract-bound-test-command',
      commandDigest: '',
      executableSha256: '',
      cwdSha256: '',
      environmentKeyDigest: '',
      environmentDigest: '',
      exitCode: null,
      signal: '',
      durationMs: 0,
      stdoutSha256: '',
      stderrSha256: '',
      repositoryStableBeforeAndAfter: false,
      sandboxCleanupSucceeded: false,
      substitutesForAgentInvocationBinding: false
    },
    publicSeams: {
      firstLaunch: {
        healthOk: false,
        runtimeInfoOk: false,
        runtimeToolsOk: false,
        ordinaryCatalogNonempty: false,
        gitCommandAvailable: false,
        skillCatalogAvailable: false,
        skillCount: 0,
        ordinaryMCPAvailable: false,
        ordinaryMCPServerCount: 0,
        ordinaryMCPToolCount: 0,
        fundsExecutionUnavailable: false,
        fundsServerDiagnosticCount: 0,
        ordinaryToolContractCount: 0,
        ordinaryToolCatalogHash: '',
        rendererTargetCount: 0,
        electronMainPid: 0,
        runtimeListenerPid: 0,
        runtimeListenerCount: 0,
        runtimeBackendProcessCount: 0,
        runtimeServerProcessCount: 0,
        desktopPrivateHistoryMigrationProcessCount: 0,
        bundledPluginMaterializationProcessCount: 0,
        unknownRuntimeProcessCount: 0,
        rendererReadFailureObserved: false,
        rendererReadFailureStage: 'unknown',
        rendererReadFailureTargetCount: 0,
        startupTrace: emptyPackagedStartupTraceEvidence(),
        runtimeListenerIsTaskOwnedDescendant: false,
        exactPackagedRuntimeExecutable: false
      },
      secondLaunch: {
        healthOk: false,
        runtimeInfoOk: false,
        runtimeToolsOk: false,
        ordinaryCatalogNonempty: false,
        gitCommandAvailable: false,
        skillCatalogAvailable: false,
        skillCount: 0,
        ordinaryMCPAvailable: false,
        ordinaryMCPServerCount: 0,
        ordinaryMCPToolCount: 0,
        fundsExecutionUnavailable: false,
        fundsServerDiagnosticCount: 0,
        ordinaryToolContractCount: 0,
        ordinaryToolCatalogHash: '',
        rendererTargetCount: 0,
        electronMainPid: 0,
        runtimeListenerPid: 0,
        runtimeListenerCount: 0,
        runtimeBackendProcessCount: 0,
        runtimeServerProcessCount: 0,
        desktopPrivateHistoryMigrationProcessCount: 0,
        bundledPluginMaterializationProcessCount: 0,
        unknownRuntimeProcessCount: 0,
        rendererReadFailureObserved: false,
        rendererReadFailureStage: 'unknown',
        rendererReadFailureTargetCount: 0,
        startupTrace: emptyPackagedStartupTraceEvidence(),
        runtimeListenerIsTaskOwnedDescendant: false,
        exactPackagedRuntimeExecutable: false
      }
    },
    checks: []
  }
}

export async function runMilestoneA({ credentialScanSource = null, entryAttemptId = '' } = {}) {
  let source = typeof credentialScanSource === 'function' ? null : credentialScanSource
  try {
    return await runMilestoneAWithCredentialSource({
      entryAttemptId,
      credentialScanSource: typeof credentialScanSource === 'function'
        ? async (context) => { source = await credentialScanSource(context); return source }
        : source
    })
  } finally {
    disposeLocalCredentialScanSource(source)
  }
}

async function runMilestoneAWithCredentialSource({ credentialScanSource, entryAttemptId }) {
  let privateCredentialSource = null
  const target = packagedTarget()
  const appPath = packagedAppPath(target)
  const timeoutMs = Number(optionValue(
    '--timeout-ms',
    process.env.ANALYTIX_RUNTIME_GO_MILESTONE_A_TIMEOUT_MS || DEFAULT_TIMEOUT_MS
  ))
  const sourceCommit = expectedPackagedSourceCommit()
  const credentialEntry = configuredCredentialEntryDeclaration()
  const report = safeReportSkeleton({
    target,
    appPath,
    timeoutMs,
    sourceCommit,
    credentialEntry
  })
  const repositoryInput = configuredExternalRepositoryAcceptance({ preflightOnly: dryRun })
  report.repository = {
    ...report.repository,
    configured: Boolean(
      repositoryInput.repositoryPathHash &&
      repositoryInput.contractPathHash &&
      repositoryInput.provenancePathHash &&
      repositoryInput.ownerRootPathHash
    ),
    validated: repositoryInput.ok,
    baselineValidated: repositoryInput.ok,
    fixtureSubstituteRejected: repositoryInput.ok,
    repositoryPathHash: repositoryInput.repositoryPathHash,
    contractPathHash: repositoryInput.contractPathHash,
    provenancePathHash: repositoryInput.provenancePathHash,
    ownerRootPathHash: repositoryInput.ownerRootPathHash,
    contractSha256: repositoryInput.contractSha256,
    provenanceSha256: repositoryInput.provenanceSha256,
    baselineCommit: repositoryInput.baselineCommit,
    baselineTree: repositoryInput.baselineTree,
    minimumCommitCount: repositoryInput.minimumCommitCount,
    originBound: repositoryInput.originBound,
    freshOriginVerified: repositoryInput.freshOriginVerified,
    formalAcceptance: repositoryInput.formalAcceptance,
    preflightValidated: repositoryInput.preflightValidated,
    mutationApplied: repositoryInput.mutationApplied,
    freshOriginFetchAttemptCount: repositoryInput.freshOriginFetchAttemptCount,
    freshOriginFetchExitCodes: [...repositoryInput.freshOriginFetchExitCodes],
    planArtifactPathHash: repositoryInput.planArtifactPathHash ||
      (repositoryInput.planArtifactRelativePath
        ? sha256(repositoryInput.planArtifactRelativePath)
        : ''),
    planArtifactExcludeSha256: repositoryInput.planArtifactExcludeSha256,
    planArtifactExcludeBeforeSha256: repositoryInput.planArtifactExcludeBeforeSha256,
    ownerRootBound: repositoryInput.ownerRootBound === true,
    ownerOnlyIsolationBound: repositoryInput.ownerOnlyIsolationBound === true,
    gitCommonDirContained: repositoryInput.gitCommonDirContained === true,
    symlinkHardlinkIsolationBound: repositoryInput.symlinkHardlinkIsolationBound === true,
    planArtifactExcludeBound: repositoryInput.planArtifactExcludeBound === true,
    planArtifactPathSafe: repositoryInput.planArtifactPathSafe === true,
    expectedChangedFileCount: repositoryInput.expectedChangedFileCount,
    inspectPathCount: repositoryInput.inspectPathCount,
    blocker: repositoryInput.blocker
  }
  if (dryRun) {
    const tempAuthority = trustedCacheTempRoot()
    const artifact = formalPackagedArtifactEvidence(appPath, target, sourceCommit)
    report.app = { ...report.app, ...artifact }
    report.harness = {
      ...report.harness,
      sourceClosureBound: harnessSourceClosureBound(report.harness, artifact, sourceCommit),
      sourceClosureSnapshotDigest:
        artifact.worktreeSnapshotBinding?.current?.digest || '',
      packagedSourceClosureSnapshotDigest:
        artifact.worktreeSnapshotBinding?.packaged?.digest || ''
    }
    report.isolation.cacheTmpdirVerified = tempAuthority.ok
    const preflightChecks = [
      check(
        'external-real-repository-contract',
        repositoryInput.ok && repositoryInput.formalAcceptance &&
          repositoryInput.freshOriginVerified,
        repositoryInput.blocker ||
          'dry-run verified the exact owner-isolated repository, contract, provenance, git common directory, absent plan artifact, and fresh origin',
        repositoryInput.blocked ? 'live_blocked' : 'failed'
      ),
      check(
        'trusted-cache-tmpdir',
        tempAuthority.ok,
        tempAuthority.blocker ||
          'dry-run verified the trusted cache mount without creating a run sandbox',
        'live_blocked'
      ),
      check(
        'formal-packaged-artifact',
        artifact.ok && report.harness.sourceClosureBound,
        artifact.blocker ||
          'dry-run verified the exact package, source commit, worktree snapshot, harness closure, executable bindings, and code signature',
        artifact.blocked ? 'live_blocked' : 'failed'
      )
    ]
    report.checks = [
      ...preflightChecks,
      ...REQUIRED_CHECK_IDS
        .filter((id) => !preflightChecks.some((item) => item.id === id))
        .map((id) => check(
          id,
          false,
          'dry run; packaged Milestone A public seam was not executed',
          'live_blocked'
        ))
    ]
    const firstFailedPreflight = preflightChecks.find((item) => item.status !== 'passed')
    report.executionBlocker = firstFailedPreflight?.message ||
      'dry_run_packaged_public_seam_not_executed'
    return finalizeMilestoneAReport(report)
  }
  if (target.platform !== 'darwin' || process.platform !== 'darwin') {
    report.checks = REQUIRED_CHECK_IDS.map((id) =>
      check(id, false, 'current harness requires a macOS packaged target and host', 'live_blocked')
    )
    report.blockers = ['milestone_a_macos_host_required']
    return finalizeMilestoneAReport(report)
  }

  const tempAuthority = trustedCacheTempRoot()
  const artifact = formalPackagedArtifactEvidence(appPath, target, sourceCommit)
  report.app = { ...report.app, ...artifact }
  report.harness = {
    ...report.harness,
    sourceClosureBound: harnessSourceClosureBound(report.harness, artifact, sourceCommit),
    sourceClosureSnapshotDigest:
      artifact.worktreeSnapshotBinding?.current?.digest || '',
    packagedSourceClosureSnapshotDigest:
      artifact.worktreeSnapshotBinding?.packaged?.digest || ''
  }
  const releaseAuthority = await verifyPackagedReleasePublicationAuthority({
    requested: true,
    config: releasePublicationConfiguration(rawArgs),
    target,
    candidate: {
      ok: artifact.ok && report.harness.sourceClosureBound,
      sha256: artifact.authoritySha256,
      authorityDigest: artifact.authorityDigest,
      sourceCommit: artifact.sourceCommit,
      targetKey: artifact.targetKey
    }
  })
  report.releasePublicationAuthority = releaseAuthority
  report.commercialRelease = commercialReleaseEvidence(artifact, releaseAuthority)

  const prerequisites = [
    check(
      'external-real-repository-contract',
      repositoryInput.ok && repositoryInput.formalAcceptance &&
        repositoryInput.freshOriginVerified,
      repositoryInput.blocker ||
        'an independently pre-admitted real origin was freshly fetched and bound to the exact isolated clone, commit, tree, and acceptance contract',
      repositoryInput.blocked ? 'live_blocked' : 'failed'
    ),
    check(
      'trusted-cache-tmpdir',
      tempAuthority.ok,
      tempAuthority.blocker ||
        'TMPDIR is writable, contained by /Volumes/AnalytixCache, and bound to its trusted APFS device',
      'live_blocked'
    ),
    check(
      'formal-packaged-artifact',
      artifact.ok && report.harness.sourceClosureBound,
      artifact.blocker ||
        'formal package artifact, commit-bound harness source closure, build authority, exact executable bindings, and code signature verified',
      artifact.blocked ? 'live_blocked' : 'failed'
    )
  ]
  report.checks.push(...prerequisites)
  if (prerequisites.some((item) => item.status !== 'passed')) {
    const prerequisiteStatus = prerequisites.some((item) => item.status === 'failed')
      ? 'skipped'
      : 'live_blocked'
    for (const id of REQUIRED_CHECK_IDS) {
      if (report.checks.some((item) => item.id === id)) continue
      report.checks.push(check(
        id,
        false,
        'formal package prerequisites prevented this public-seam check',
        prerequisiteStatus
      ))
    }
    report.status = prerequisites.some((item) => item.status === 'failed')
      ? 'failed'
      : 'live_blocked'
    report.blockers = prerequisites
      .filter((item) => item.status !== 'passed')
      .map((item) => item.message)
    return finalizeMilestoneAReport(report)
  }

  const revalidatePackagedArtifact = (field, phase) => {
    const observed = formalPackagedArtifactEvidence(appPath, target, sourceCommit)
    const evidence = packagedArtifactRevalidationEvidence(artifact, observed, phase)
    report.artifactRevalidation[field] = evidence
    return evidence
  }

  let sandboxRoot = ''
  let firstChild = null
  let secondChild = null
  let isolatedLoginKeychain = null
  let settingsPath = ''
  let runtimePort = 0
  let schedulePort = 0
  let firstDebugPort = 0
  let secondDebugPort = 0
  let firstEvidence = null
  let compactedEvidence = null
  let recoveredEvidence = null
  let repositoryAuthority = null
  let repositoryBaseline = null
  let repositoryAcceptance = null
  let bundledFundsMaterializationSeam = null
  let firstFundsMaterializationFailure = { ok: false }
  let secondFundsMaterializationFailure = { ok: false }
  let privateCaseInputs = {
    privateAuthorityInputsAbsent: false,
    privateAuthorityEnvironmentKeyCount: 0,
    privateAuthoritySettingsKeyCount: 0,
    preLaunchUnexpectedUserDataEntryCount: 0
  }
  let firstFundsExecutionUnavailable = false
  let protectedFundsUnavailable = protectedFundsUnavailableTurnEvidence(null)
  let restartedProtectedFundsUnavailable = protectedFundsUnavailableTurnEvidence(null)
  let rendererRecovery = {
    ok: false,
    blocker: 'renderer_visible_recovery_not_observed',
    timelineVisible: false,
    threadVisible: false,
    resultVisible: false,
    compactionVisible: false,
    todoVisible: false,
    subagentVisible: false
  }
  let firstNormalQuit = false
  let firstNormalQuitReasonCode = 'normal_quit_not_observed'
  let finalNormalQuit = false
  let finalNormalQuitReasonCode = 'normal_quit_not_observed'
  let cleanupSucceeded = false
  let externalRepositoryPreserved = false
  let credentialScan = {
    scannedFileCount: 0,
    findingCount: 0,
    settingsFindingCount: 0,
    reportFindingCount: 0,
    unsafeEntryCount: 0,
    symlinkCount: 0,
    sourceSecretCount: 0,
    uniqueSecretCount: 0,
    expectedSecretCount: 1,
    sourceBound: false,
    status: 'blocked'
  }
  let provider = null
  let providerEvidence = providerTurnEvidence(null, provider)
  let relaunchProviderContinuationObserved = false
  let firstCompletedScreenshot = report.visualEvidence.firstCompleted
  let recoveredScreenshot = report.visualEvidence.recoveredAfterRelaunch

  try {
    sandboxRoot = mkdtempSync(join(tempAuthority.path, 'analytix-milestone-a-'))
    chmodSync(sandboxRoot, 0o700)
    const isolatedHome = join(sandboxRoot, 'home')
    const chromiumTempDir = join(sandboxRoot, 'chromium-tmp')
    const userDataDir = join(sandboxRoot, 'electron-user-data')
    const runtimeDataDir = join(sandboxRoot, 'runtime-data')
    const workspace = repositoryInput.authority.workspace
    const writeWorkspace = join(sandboxRoot, 'write-workspace')
    const scheduleWorkspace = join(sandboxRoot, 'schedule-workspace')
    const clawWorkspace = join(sandboxRoot, 'claw-workspace')
    mkdirSync(isolatedHome, { recursive: true, mode: 0o700 })
    mkdirSync(chromiumTempDir, { mode: 0o700 })
    repositoryAuthority = repositoryInput.authority
    repositoryBaseline = verifyRepositoryAcceptance(repositoryAuthority, 'baseline')
    report.repository = projectMilestoneARepositoryBaseline(report.repository, repositoryBaseline)
    runtimePort = await getFreePort()
    schedulePort = await getFreePort()
    while (schedulePort === runtimePort) schedulePort = await getFreePort()
    firstDebugPort = await getFreePort()
    while (firstDebugPort === runtimePort || firstDebugPort === schedulePort) {
      firstDebugPort = await getFreePort()
    }
    settingsPath = writeIsolatedSettings({
      userDataDir,
      runtimeDataDir,
      workspace,
      writeWorkspace,
      scheduleWorkspace,
      clawWorkspace,
      runtimePort,
      schedulePort,
      packagedSkillRoot: bundledComputerUseSkillRoot(appPath, target)
    })
    bundledFundsMaterializationSeam =
      prepareMissingBundledFundsMaterializationAuthoritySeam(runtimeDataDir)
    report.caseCapability = {
      ...report.caseCapability,
      bundledFundsMissingAuthorityPreconditionEstablished:
        bundledFundsMaterializationSeam.evidence.preconditionEstablished,
      bundledFundsMaterializationStateRootSha256:
        bundledFundsMaterializationSeam.evidence.stateRootSha256,
      bundledFundsMaterializationAuthorityPathSha256:
        bundledFundsMaterializationSeam.evidence.authorityPathSha256
    }
    const childEnv = safeChildEnvironment({
      isolatedHome,
      userDataDir,
      chromiumTempDir
    })
    isolatedLoginKeychain = await createIsolatedDarwinLoginKeychain(isolatedHome)
    const createdLoginKeychain = isolatedLoginKeychain.evidence()
    privateCaseInputs = privateCaseAuthorityInputEvidence(childEnv, settingsPath)
    report.caseCapability = {
      ...report.caseCapability,
      ...privateCaseInputs
    }
    report.isolation = {
      cacheTmpdirVerified: true,
      systemUserDataTouched: false,
      isolatedHomeUsedByChild: childEnv.HOME === isolatedHome,
      isolatedLoginKeychainCreated: createdLoginKeychain.created,
      isolatedLoginKeychainDefaultBound: createdLoginKeychain.defaultKeychainBound,
      isolatedLoginKeychainUnlockCount: createdLoginKeychain.unlockCount,
      isolatedLoginKeychainReusedForTwoLaunches:
        createdLoginKeychain.reusedForTwoLaunches,
      isolatedLoginKeychainPasswordRecorded:
        createdLoginKeychain.passwordRecorded,
      isolatedLoginKeychainPathHash: createdLoginKeychain.requestedPathHash,
      isolatedLoginKeychainDatabasePathHash:
        createdLoginKeychain.databasePathHash,
      isolatedLoginKeychainExpectExecutableSha256:
        createdLoginKeychain.expectExecutableSha256,
      isolatedLoginKeychainSecurityExecutableSha256:
        createdLoginKeychain.securityExecutableSha256,
      chromiumTempBoundToTrustedCache:
        childEnv.TMPDIR === chromiumTempDir &&
        childEnv.TEMP === chromiumTempDir &&
        childEnv.TMP === chromiumTempDir &&
        (process.platform !== 'darwin' ||
          childEnv.MAC_CHROMIUM_TMPDIR === chromiumTempDir),
      chromiumTempPathHash: sha256(chromiumTempDir),
      userDataPathHash: sha256(userDataDir),
      runtimeDataPathHash: sha256(runtimeDataDir),
      workspacePathHash: sha256(workspace),
      sameDataDirsOnRelaunch: true,
      sandboxRemoved: false
    }
    report.checks.push(check(
      'isolated-profile-and-repository',
      [sandboxRoot, isolatedHome, chromiumTempDir, userDataDir, runtimeDataDir]
        .every((path) => path === tempAuthority.path || path.startsWith(`${tempAuthority.path}${sep}`)) &&
        childEnv.HOME === isolatedHome &&
        childEnv.USERPROFILE === isolatedHome &&
        childEnv.TMPDIR === chromiumTempDir &&
        childEnv.TEMP === chromiumTempDir &&
        childEnv.TMP === chromiumTempDir &&
        (process.platform !== 'darwin' ||
          childEnv.MAC_CHROMIUM_TMPDIR === chromiumTempDir) &&
        createdLoginKeychain.ok &&
        createdLoginKeychain.directoriesOwnerOnly &&
        createdLoginKeychain.passwordInArgv === false &&
        createdLoginKeychain.passwordInEnvironment === false &&
        createdLoginKeychain.passwordInFile === false &&
        createdLoginKeychain.passwordRecorded === false &&
        pathIsOutside(sandboxRoot, workspace) &&
        repositoryBaseline.ok &&
        repositoryBaseline.gitRepository &&
        repositoryBaseline.protectedInputsBound &&
        repositoryBaseline.acceptanceContractBound &&
        repositoryBaseline.baselineHistoryBound &&
        repositoryBaseline.originBound &&
        repositoryBaseline.fixtureSubstituteRejected &&
        repositoryBaseline.formalAcceptance &&
        repositoryBaseline.freshOriginVerified &&
        repositoryBaseline.ownerRootBound &&
        repositoryBaseline.ownerOnlyIsolationBound &&
        repositoryBaseline.gitCommonDirContained &&
        repositoryBaseline.symlinkHardlinkIsolationBound &&
        repositoryBaseline.planArtifactExcludeBound &&
        repositoryBaseline.planArtifactPathSafe,
      'profile/runtime/home, Chromium temp, and the task-owned default login keychain are isolated below trusted TMPDIR; the keychain password crosses only process memory, stdin, and a no-log PTY; the separately supplied clone is owner-isolated under /Volumes/AnalytixCache with contained Git common-dir, no trusted-path symlinks/hardlinks, fresh origin proof, an exact external acceptance contract, and one exact local plan-artifact exclusion'
    ))

    const beforeFirstLaunch = revalidatePackagedArtifact(
      'beforeFirstLaunch',
      'before-first-launch'
    )
    if (!beforeFirstLaunch.ok) {
      report.checks.push(check(
        'packaged-first-launch',
        false,
        beforeFirstLaunch.blocker || 'formal package changed before first launch'
      ))
      throw new Error(beforeFirstLaunch.blocker || 'formal_package_changed_before_first_launch')
    }
    const firstKeychainUnlock = await isolatedLoginKeychain.unlockForLaunch()
    report.isolation = {
      ...report.isolation,
      isolatedLoginKeychainUnlockCount: firstKeychainUnlock.unlockCount,
      isolatedLoginKeychainDefaultBound: firstKeychainUnlock.defaultKeychainBound,
      isolatedLoginKeychainReusedForTwoLaunches:
        firstKeychainUnlock.reusedForTwoLaunches
    }
    firstChild = launchPackagedApp({
      appPath,
      target,
      debugPort: firstDebugPort,
      childEnv
    })
    if (!jsonOutput) {
      console.log('LIVE CHECKPOINT: complete visible protected local Provider onboarding normally.')
    }
    const firstWorkbench = await waitForLocalProviderWorkbench({
      debugPort: firstDebugPort,
      workspace,
      runtimePort,
      runtimeDataDir,
      timeoutMs,
      onFreshRegistry: async (remainingMs) => {
        privateCredentialSource = await coordinateLocalCredentialEntry(
          credentialScanSource, { runId: sandboxRoot, entryAttemptId }, remainingMs
        )
      }
    })
    const firstReady = firstWorkbench.observation
    provider = firstWorkbench.provider
    firstFundsMaterializationFailure =
      missingBundledFundsMaterializationAuthorityEvidence(
        bundledFundsMaterializationSeam
      )
    report.caseCapability = {
      ...report.caseCapability,
      firstLaunchBundledFundsMaterializationFailureObserved:
        firstFundsMaterializationFailure.ok === true
    }
    const loginCredentialAuthority = readLocalCredentialScanSource(privateCredentialSource, {
      runId: sandboxRoot, entryAttemptId, provider
    })
    const secretStore = localProviderSecretStoreEvidence(runtimeDataDir)
    const normalLocalCredentialReady = firstWorkbench.ok &&
      firstWorkbench.normalLocalProviderSetupObserved === true &&
      loginCredentialAuthority.ok && secretStore.ok &&
      loginCredentialAuthority.entryMethod === credentialEntry.method &&
      loginCredentialAuthority.automatedCredentialEntryUsed === credentialEntry.automatedCredentialEntryUsed
    const providerConfigured = normalLocalCredentialReady && provider?.ok === true
    const credentialCheckpointPassed = providerConfigured &&
      credentialEntry.declared === true
    report.operatorCheckpoint.status = credentialCheckpointPassed
      ? 'completed'
      : !credentialEntry.declared || firstWorkbench.blocked ||
          (firstWorkbench.ok && !normalLocalCredentialReady)
        ? 'blocked'
        : 'failed'
    const firstTopology = runtimeBackendTopologyEvidence(
      firstChild.pid,
      runtimePort,
      runtimeServerPath(appPath, target)
    )
    const configuredNetworkProviderPassed = providerConfigured &&
      credentialEntry.declared === true
    report.provider = {
      ...report.provider,
      configured: providerConfigured,
      credentialConfigured: providerConfigured && provider.credentialConfigured === true,
      credentialAuthorityBound: providerConfigured,
      credentialRecorded: false,
      credentialAuthority: LOCAL_PROVIDER_AUTHORITY,
      normalLocalProviderSetupObserved: firstWorkbench.normalLocalProviderSetupObserved,
      localCredentialEvidence: { ...loginCredentialAuthority, protectedStoreOwnerPrivate: secretStore.ok },
      family: providerConfigured ? provider.family : '',
      providerIdHash: providerConfigured ? sha256(provider.id) : '',
      modelHash: providerConfigured ? sha256(provider.model) : '',
      baseUrlOriginHash: providerConfigured ? sha256(new URL(provider.baseUrl).origin) : '',
      endpointFormat: providerConfigured ? provider.endpointFormat : '',
      contextWindowTokensBound: providerConfigured &&
        provider.contextWindowTokensBound === true,
      settingsContextWindowTokens: providerConfigured
        ? provider.settingsContextWindowTokens
        : 0,
      runtimeContextWindowTokens: providerConfigured
        ? provider.runtimeContextWindowTokens
        : 0,
      softThresholdTokens: providerConfigured ? provider.softThresholdTokens : 0,
      hardThresholdTokens: providerConfigured ? provider.hardThresholdTokens : 0
    }
    report.checks.push(check(
      'configured-network-provider',
      configuredNetworkProviderPassed,
      (!credentialEntry.declared
        ? 'formal Milestone A requires visible-human or visible-computer-use local Provider credential entry; visible-automation and undeclared methods are not accepted'
        : firstWorkbench.blocker) ||
        (firstWorkbench.ok && !normalLocalCredentialReady
          ? 'fresh visible local onboarding must bind the selected Registry, protected Secret Store, and the same private credential entry; missing or reused input cannot complete the checkpoint'
          : 'visible normal local onboarding bound Registry and protected credentials without settings secrets'),
      !credentialEntry.declared || firstWorkbench.blocked ||
        (firstWorkbench.ok && !normalLocalCredentialReady)
        ? 'live_blocked'
        : 'failed'
    ))
    report.publicSeams.firstLaunch = {
      ...firstWorkbench.publicSeam,
      rendererTargetCount: firstReady?.rendererTargetCount || 0,
      electronMainPid: firstTopology.electronMainPid,
      runtimeListenerPid: firstTopology.runtimeListenerPid,
      runtimeListenerCount: firstTopology.listenerCount,
      runtimeBackendProcessCount: firstTopology.runtimeBackendProcessCount,
      runtimeServerProcessCount: firstTopology.runtimeServerProcessCount,
      desktopPrivateHistoryMigrationProcessCount:
        firstTopology.desktopPrivateHistoryMigrationProcessCount,
      bundledPluginMaterializationProcessCount:
        firstTopology.bundledPluginMaterializationProcessCount,
      unknownRuntimeProcessCount: firstTopology.unknownRuntimeProcessCount,
      rendererReadFailureObserved: firstWorkbench.readDiagnostic?.observed === true,
      rendererReadFailureStage: firstWorkbench.readDiagnostic?.readErrorStage || 'unknown',
      rendererReadFailureTargetCount:
        safeNonNegativeCount(firstWorkbench.readDiagnostic?.rendererTargetCount),
      startupTrace: packagedStartupTraceEvidence(firstChild),
      runtimeListenerIsTaskOwnedDescendant: firstTopology.listenerIsTaskOwnedDescendant,
      exactPackagedRuntimeExecutable: firstTopology.exactPackageExecutable
    }
    report.caseCapability = {
      ...report.caseCapability,
      fundsDiagnosticsIndependent: true,
      ordinaryCatalogAvailable: firstWorkbench.publicSeam?.ordinaryCatalogNonempty === true
    }
    const firstLaunchObserved = firstKeychainUnlock.ok &&
      firstWorkbench.ok && firstTopology.ok
    report.checks.push(check(
      'packaged-first-launch',
      firstLaunchObserved,
      firstWorkbench.blocker ||
        'one packaged renderer must expose health/info/tools and one task-owned listener running the exact packaged Go runtime executable',
      firstWorkbench.blocked ? 'live_blocked' : 'failed'
    ))
    report.workflow.firstLaunchObserved = firstLaunchObserved
    if (!firstLaunchObserved) {
      throw Object.assign(
        new Error(firstWorkbench.blocker || 'packaged_first_launch_topology_failed'),
        { liveBlocked: firstWorkbench.blocked }
      )
    }
    if (FIRST_LAUNCH_PREREQUISITE_CHECK_IDS.every((id) =>
      report.checks.some((item) => item.id === id && item.status === 'passed')
    )) {
      const snapshot = firstLaunchPrerequisiteStageSnapshot(report)
      if (snapshot) report.stageSnapshots.push(snapshot)
    }

    const firstReasoningSelection = await selectComposerReasoningEffort(
      firstDebugPort,
      FORMAL_MODEL,
      FORMAL_REASONING_EFFORT
    )
    report.provider = {
      ...report.provider,
      firstLaunchReasoningEffortSelectedThroughVisibleUi: firstReasoningSelection.ok,
      firstLaunchReasoningControlTitleHash:
        firstReasoningSelection.controlTitleSha256
    }
    if (!firstReasoningSelection.ok) {
      report.checks.push(check(
        'composer-workflow-submit',
        false,
        firstReasoningSelection.blocker ||
          'the formal reasoning effort must be selected through the visible composer',
        firstReasoningSelection.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(firstReasoningSelection.blocker), {
        liveBlocked: firstReasoningSelection.blocked
      })
    }

    const planMode = await selectComposerMode(firstDebugPort, 'plan')
    if (!planMode.ok) {
      applyComposerModeDiagnostic(report, planMode)
      report.checks.push(check(
        'composer-workflow-submit',
        false,
        planMode.blocker || 'real composer Plan mode must be selected through visible UI',
        planMode.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(planMode.blocker), { liveBlocked: planMode.blocked })
    }
    report.workflow.planModeSelected = planMode.ok
    const planInspectPaths = milestoneAPlanInspectPaths(repositoryAuthority)
    const planPrompt = milestoneAPlanPrompt(repositoryAuthority)
    const planBaselineEvidence = workflowEvidence(firstReady, workspace, PLAN_MARKER)
    const planBaselineSeq = Number.isSafeInteger(firstReady?.thread?.latestSeq)
      ? firstReady.thread.latestSeq
      : 0
    const planSubmit = await cdpComposerSubmit(firstDebugPort, planPrompt, ['Send'])
    if (!planSubmit.ok) {
      applyComposerSubmitDiagnostic(report, planSubmit)
      report.checks.push(check(
        'composer-workflow-submit',
        false,
        planSubmit.blocker || 'planning turn entered through the real composer and primary Send button',
        planSubmit.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(planSubmit.blocker), { liveBlocked: planSubmit.blocked })
    }
    report.workflow.planComposerSubmitted = true
    const planned = await waitForWorkflow({
      debugPort: firstDebugPort,
      workspace,
      timeoutMs,
      resultMarker: PLAN_MARKER,
      previousTurnCount: planBaselineEvidence.turnCount,
      expectedThreadId: planBaselineEvidence.threadId
    })
    let planEvidence = planned.evidence
    const planReceiptScopeEvidence = latestTerminalProviderReceiptScopeEvidence(
      planEvidence,
      {
        previousTurnCount: planBaselineEvidence.turnCount,
        baselineSeq: planBaselineSeq,
        expectedThreadId: planBaselineEvidence.threadId
      }
    )
    const planReceiptScope = planReceiptScopeEvidence.scope
    if (planReceiptScope) {
      planEvidence = workflowEvidence(
        planned.observation,
        workspace,
        PLAN_MARKER,
        planReceiptScope.expectedTurnId
      )
      let receiptObservation
      try {
        receiptObservation = await observeRenderer({
          debugPort: firstDebugPort,
          workspace,
          exactThreadId: planEvidence.threadId,
          providerReceiptScope: planReceiptScope,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
      } catch (error) {
        const reasonCode = readonlyCdpFailureReasonCode(error)
        const expressionStage = safeReadonlyObservationReadErrorStage(
          error?.readonlyCdpExpressionStage
        )
        applyPlanObservationDiagnostic(report, {
          reasonCode,
          phase: 'provider_receipt_replay',
          operation: 'read',
          expressionStage
        })
        throw readonlyCdpFailureError(reasonCode, { expressionStage })
      }
      planEvidence = workflowEvidence(
        receiptObservation,
        workspace,
        PLAN_MARKER,
        planReceiptScope.expectedTurnId
      )
    }
    const planReadHostObservations = []
    for (const path of planInspectPaths) {
      planReadHostObservations.push(await observeHostToolExecution({
        debugPort: firstDebugPort,
        threadId: planEvidence.threadId,
        turnId: planEvidence.resultTurnId,
        toolName: 'read',
        workspace,
        arguments: { path },
        timeoutMs: Math.min(timeoutMs, 10_000)
      }))
    }
    const planReadHostObservationMatches = planReadHostObservations
      .map((observation, index) =>
        privateHostToolExecutionBindingMatches(observation, {
          threadId: planEvidence.threadId,
          turnId: planEvidence.resultTurnId,
          toolName: 'read',
          workspace,
          arguments: { path: planInspectPaths[index] }
        })
      )
    const planReadSet = milestoneAPlanReadSetEvidence(
      repositoryAuthority,
      planEvidence,
      planReadHostObservationMatches
    )
    const planTaskInspectionReadsBound = planReadSet.ok
    const planArtifact = milestoneAPlanArtifactEvidence(
      repositoryAuthority,
      planEvidence.thread,
      planEvidence.resultTurnId
    )
    const planTurns = Array.isArray(planEvidence.thread?.turns)
      ? planEvidence.thread.turns
      : []
    const planResultTurns = planTurns.filter((turn) =>
      turn?.id === planReceiptScope?.expectedTurnId
    )
    const exactPlanTurnBound = Boolean(planReceiptScope &&
      planEvidence.resultTurnId === planReceiptScope.expectedTurnId &&
      planResultTurns.length === 1 &&
      planResultTurns[0]?.status === 'completed' &&
      planTurns.at(-1)?.id === planReceiptScope.expectedTurnId)
    const planReasoningEffortBound = exactTurnReasoningEffortBound(
      planEvidence,
      FORMAL_REASONING_EFFORT
    )
    const planTurnPassed = exactPlanTurnBound &&
      planReasoningEffortBound &&
      planEvidence.threadId &&
      planEvidence.workspaceBound &&
      planEvidence.terminal &&
      planEvidence.toolGroupPassed.read &&
      planEvidence.toolGroupPassed.plan &&
      planTaskInspectionReadsBound &&
      planArtifact.ok
    const planProviderEvidence = providerTurnEvidence(planEvidence, provider)
    const planProviderReceiptBound = planProviderEvidence.networkTurnCompleted === true
    const planThreadIdHash = planEvidence.threadId ? sha256(planEvidence.threadId) : ''
    const planProviderReceiptTrace = {
      reasonCode: planProviderEvidence.reasonCode,
      sseReasonCode: planProviderEvidence.sseReasonCode,
      started: planEvidence.providerReceiptTrace.started,
      acknowledged: planEvidence.providerReceiptTrace.acknowledged,
      ended: planEvidence.providerReceiptTrace.ended,
      eventCount: planEvidence.providerReceiptTrace.eventCount,
      attemptCount: planProviderEvidence.providerAttemptRecordCount,
      terminalCount: planProviderEvidence.providerTerminalRecordCount
    }
    const planDiagnostic = planningTurnDiagnosticProjection(planEvidence, {
      previousTurnCount: planBaselineEvidence.turnCount,
      baselineSeq: planBaselineSeq,
      expectedThreadId: planBaselineEvidence.threadId,
      disposition: planned.disposition
    })
    report.workflow = {
      ...report.workflow,
      ...planDiagnostic
    }
    report.workflow.planModeSelected = planMode.ok
    report.workflow.planTurnObserved = planTurnPassed
    report.provider.planReasoningEffortBound = planReasoningEffortBound
    report.workflow.planThreadIdHash = planThreadIdHash
    report.workflow.threadIdHash = planThreadIdHash
    report.workflow.planProviderReceiptTrace = planProviderReceiptTrace
    report.workflow.planTaskInspectionReadsBound = planTaskInspectionReadsBound
    report.workflow.planTaskInspectionReadObservationDigest = planTaskInspectionReadsBound
      ? sha256(canonicalJSON(planReadHostObservations.map((item) => ({
          observationDigest: item.observationDigest,
          authorityBindingDigest: item.authorityBindingDigest
        }))))
      : ''
    report.workflow.planTaskInspectionExpectedReadCount = planReadSet.expectedReadCount
    report.workflow.planTaskInspectionReadAttemptCount = planReadSet.readAttemptCount
    report.workflow.planTaskInspectionReadAttemptDigest = planReadSet.readAttemptDigest
    report.workflow.planLongContextReadAbsent = planReadSet.longContextReadAbsent
    report.workflow.planProviderReceiptBound = planProviderReceiptBound
    report.workflow.planProviderReceiptDigest = planProviderReceiptBound
      ? planProviderEvidence.providerAttemptReceiptDigest
      : ''
    report.workflow.planArtifactBound = planArtifact.ok
    report.workflow.planArtifactRelativePath = planArtifact.relativePath
    report.workflow.planArtifactContentHash = planArtifact.contentHash
    report.workflow.planArtifactByteSize = planArtifact.byteSize
    report.workflow.planArtifactResultItemDigest = planArtifact.resultItemDigest
    report.workflow.planArtifactRevisionSequenceDigest = planArtifact.revisionSequenceDigest
    report.workflow.planArtifactFileIdentityDigest = planArtifact.fileIdentityDigest
    report.workflow.planArtifactExcludeSha256 = planArtifact.excludeSha256
    report.workflow.planArtifactCallCount = planArtifact.planCallCount
    report.workflow.planArtifactResultCount = planArtifact.planResultCount
    if (!planTurnPassed || !planProviderReceiptBound) {
      const planTerminalFailureCode = planDiagnostic.planCandidateTurnObserved &&
          planDiagnostic.planCandidateTurnStatus !== 'completed'
        ? planDiagnostic.planCandidateTurnErrorCode !== 'none'
          ? planDiagnostic.planCandidateTurnErrorCode
          : `milestone_a_plan_turn_${planDiagnostic.planCandidateTurnStatus}`
        : planned.disposition === 'timeout'
          ? 'milestone_a_plan_turn_timeout'
          : ''
      const planBlocker = planTerminalFailureCode ||
        (!planDiagnostic.planProviderReceiptScopeBound
          ? planDiagnostic.planProviderReceiptScopeReasonCode
          : '') ||
        (planArtifact.blocker
        ? `${planArtifact.blocker}${planArtifact.pairFailureCode
            ? `:${planArtifact.pairFailureCode}`
            : ''}`
        : !exactPlanTurnBound
          ? 'milestone_a_plan_exact_terminal_turn_invalid'
          : !planReasoningEffortBound
            ? 'milestone_a_plan_reasoning_effort_unbound'
          : !planEvidence.workspaceBound || !planEvidence.terminal
            ? 'milestone_a_plan_thread_authority_invalid'
            : !planEvidence.toolGroupPassed.read
              ? 'milestone_a_plan_read_execution_missing'
              : !planEvidence.toolGroupPassed.plan
                ? 'milestone_a_plan_execution_missing'
                : !planTaskInspectionReadsBound
                  ? 'milestone_a_plan_read_set_invalid'
                  : (planTurnPassed
                      ? `configured provider terminal receipt failed: ${planProviderEvidence.reasonCode}`
                      : 'milestone_a_plan_contract_failed'))
      report.checks.push(check(
        'ordinary-agent-workflow',
        false,
        planBlocker,
        'failed'
      ))
      throw new Error(planTurnPassed
        ? 'packaged_plan_provider_receipt_failed'
        : 'packaged_plan_turn_failed')
    }
    const planStageSnapshot = completedPlanStageSnapshot(report)
    if (planStageSnapshot) report.stageSnapshots.push(planStageSnapshot)

    const agentMode = await selectComposerMode(firstDebugPort, 'agent')
    if (!agentMode.ok) {
      applyComposerModeDiagnostic(report, agentMode)
      report.checks.push(check(
        'composer-workflow-submit',
        false,
        agentMode.blocker || 'real composer Agent mode must be restored through visible UI',
        agentMode.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(agentMode.blocker), { liveBlocked: agentMode.blocked })
    }
    const skillId = packagedMilestoneASkillID(firstReady?.runtimeSkills)
    const catalogFlowAvailable =
      firstWorkbench.publicSeam?.gitCommandAvailable === true &&
      firstWorkbench.publicSeam?.skillCatalogAvailable === true &&
      firstWorkbench.publicSeam?.ordinaryMCPAvailable === true &&
      Boolean(skillId)
    if (!catalogFlowAvailable) {
      report.checks.push(check(
        'git-skill-mcp-research-writing',
        false,
        'the formal packaged catalog must expose Git, the exact packaged Analytix Computer Use Skill, and the exact connected packaged GUI Schedule MCP server before the mixed research/writing flow starts',
        'live_blocked'
      ))
      throw Object.assign(new Error('packaged_general_agent_catalog_flow_unavailable'), {
        liveBlocked: true
      })
    }
    const childReviewPaths = milestoneAChildReviewPaths(repositoryAuthority)
    const workflowPrompt = milestoneAWorkflowPrompt(repositoryAuthority, skillId)
    const agentBaselineEvidence = workflowEvidence(planned.observation, workspace, '')
    const agentBaselineSeq = planned.observation?.thread?.latestSeq
    const submit = await cdpComposerSubmit(firstDebugPort, workflowPrompt, ['Send'])
    if (!submit.ok) applyComposerSubmitDiagnostic(report, submit)
    report.checks.push(check(
      'composer-workflow-submit',
      firstReasoningSelection.ok && planMode.ok && planSubmit.ok && agentMode.ok && submit.ok,
      submit.blocker ||
        'the fixed formal reasoning effort plus Plan and Agent turns entered through the real visible composer controls and primary Send button',
      submit.blocked ? 'live_blocked' : 'failed'
    ))
    report.workflow.composerWorkflowSubmitted =
      firstReasoningSelection.ok && planMode.ok && planSubmit.ok && agentMode.ok && submit.ok
    if (!submit.ok) throw Object.assign(new Error(submit.blocker), { liveBlocked: submit.blocked })

    const workflow = await waitForWorkflow({
      debugPort: firstDebugPort,
      workspace,
      timeoutMs,
      previousTurnCount: agentBaselineEvidence.turnCount,
      expectedThreadId: planEvidence.threadId
    })
    firstEvidence = workflow.evidence
    const agentReceiptScope = exactLatestTerminalProviderReceiptScope(firstEvidence, {
      previousTurnCount: agentBaselineEvidence.turnCount,
      baselineSeq: agentBaselineSeq,
      expectedThreadId: planEvidence.threadId
    })
    if (agentReceiptScope) {
      const receiptObservation = await observeRenderer({
        debugPort: firstDebugPort,
        workspace,
        exactThreadId: firstEvidence.threadId,
        providerReceiptScope: agentReceiptScope,
        timeoutMs: Math.min(timeoutMs, 20_000)
      })
      firstEvidence = workflowEvidence(receiptObservation, workspace, '')
    }
    const ordinaryDisposition = ordinaryWorkflowObservedDisposition(workflow, {
      previousTurnCount: agentBaselineEvidence.turnCount,
      expectedThreadId: planEvidence.threadId
    })
    const ordinaryResultEvidence = ordinaryWorkflowResultEvidence(firstEvidence)
    const firstRenderedResult = ordinaryDisposition === 'completed'
      ? await rendererResultDigestEvidence(firstDebugPort)
      : { ok: false, resultDigest: '' }
    const ordinaryReasoningEffortBound = exactTurnReasoningEffortBound(
      firstEvidence,
      FORMAL_REASONING_EFFORT
    )
    report.provider.ordinaryReasoningEffortBound = ordinaryReasoningEffortBound
    const groups = firstEvidence.toolGroupPassed
    const sourceRepair = sourceRepairEvidence(repositoryAuthority)
    repositoryAcceptance = sourceRepair.repository
    const workflowDiagnostic = terminalWorkflowDiagnostic(firstEvidence, {
      previousTurnCount: agentBaselineEvidence.turnCount
    })
    const workflowProgress = workflowProgressDiagnostic(firstEvidence, {
      previousTurnCount: agentBaselineEvidence.turnCount,
      expectedThreadId: planEvidence.threadId
    })
    let providerFailureDiagnostic = emptyProviderFailureDiagnostic()
    let toolFailureDiagnostic = emptyToolFailureDiagnostic()
    const latestAgentTurn = Array.isArray(firstEvidence.thread?.turns)
      ? firstEvidence.thread.turns.at(-1)
      : null
    const agentHighestSeq = firstEvidence.thread?.latestSeq
    if (ordinaryDisposition !== 'completed' && latestAgentTurn?.status === 'failed' &&
        Number.isSafeInteger(agentBaselineSeq) && agentBaselineSeq > 0 &&
        Number.isSafeInteger(agentHighestSeq) && agentHighestSeq > agentBaselineSeq) {
      try {
        const rawProviderFailure = await observeProviderFailureDiagnostic({
          debugPort: firstDebugPort,
          threadId: firstEvidence.threadId,
          baselineSeq: agentBaselineSeq,
          expectedTurnId: latestAgentTurn.id,
          expectedHighestSeq: agentHighestSeq,
          priorCompletedTurnId: agentBaselineEvidence.thread?.turns?.at(-1)?.id,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
        providerFailureDiagnostic = providerFailureDiagnosticEvidence(rawProviderFailure, {
          expectedThreadId: firstEvidence.threadId,
          expectedTurnId: latestAgentTurn.id,
          expectedBaselineSeq: agentBaselineSeq,
          expectedHighestSeq: agentHighestSeq
        })
      } catch {
        providerFailureDiagnostic = emptyProviderFailureDiagnostic(
          'provider_failure_observer_failed'
        )
      }
    }
    if (ordinaryDisposition !== 'completed' && latestAgentTurn?.status === 'failed' &&
        ['tool_failure_storm', 'tool_invalid_arguments_storm'].includes(
          workflowDiagnostic.turnErrorCode
        ) &&
        Number.isSafeInteger(agentBaselineSeq) && agentBaselineSeq > 0 &&
        Number.isSafeInteger(agentHighestSeq) && agentHighestSeq > agentBaselineSeq) {
      try {
        const rawToolFailure = await observeToolFailureDiagnostic({
          debugPort: firstDebugPort,
          threadId: firstEvidence.threadId,
          baselineSeq: agentBaselineSeq,
          expectedTurnId: latestAgentTurn.id,
          expectedHighestSeq: agentHighestSeq,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
        toolFailureDiagnostic = toolFailureDiagnosticEvidence(rawToolFailure, {
          expectedThreadId: firstEvidence.threadId,
          expectedTurnId: latestAgentTurn.id,
          expectedBaselineSeq: agentBaselineSeq,
          expectedHighestSeq: agentHighestSeq
        })
      } catch {
        toolFailureDiagnostic = emptyToolFailureDiagnostic(
          'tool_failure_observer_failed',
          'tool_failure_evidence_observer_rejected'
        )
      }
    }
    const workflowFailureProjection = ordinaryWorkflowFailureDiagnosticProjection(
      workflowDiagnostic,
      providerFailureDiagnostic,
      toolFailureDiagnostic
    )
    const failedChildTarget = failedChildDiagnosticTarget(firstEvidence)
    const failedChildSelection = failedChildDiagnosticEvidence(firstEvidence)
    let failedChildTerminalDiagnostic = emptyFailedChildTerminalDiagnostic()
    let failedChildThreadObserved = false
    if (failedChildTarget.ok === true) {
      try {
        const childObservation = await observeRenderer({
          debugPort: firstDebugPort,
          workspace,
          exactThreadId: failedChildTarget.childThreadId,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
        failedChildThreadObserved = childObservation?.thread?.id ===
          failedChildTarget.childThreadId
        failedChildTerminalDiagnostic = failedChildTerminalDiagnosticEvidence(
          childObservation,
          failedChildTarget.childThreadId,
          failedChildTarget.childTurnId
        )
      } catch {
        failedChildTerminalDiagnostic = emptyFailedChildTerminalDiagnostic(
          'failed_child_observation_failed'
        )
      }
    }
    report.workflow = {
      ...report.workflow,
      ordinaryWorkflowDisposition: ordinaryDisposition,
      ordinaryCandidateProgressObserved: workflowProgress.observed,
      ordinaryCandidateTerminalObserved: workflowDiagnostic.observed,
      ordinaryCandidateTurnStatus: workflowDiagnostic.observed
        ? workflowDiagnostic.turnStatus
        : workflowProgress.turnStatus,
      ordinaryCandidateTurnIdHash: workflowDiagnostic.observed
        ? workflowDiagnostic.turnIdHash
        : workflowProgress.turnIdHash,
      ordinaryCandidateTurnErrorCode: workflowDiagnostic.turnErrorCode,
      ordinaryCandidateTurnErrorCodeHash: workflowDiagnostic.turnErrorCodeHash,
      ordinaryCandidateToolAttemptCount: workflowDiagnostic.observed
        ? workflowDiagnostic.toolAttemptCount
        : workflowProgress.toolAttemptCount,
      ordinaryCandidateSuccessfulToolExecutionCount:
        workflowDiagnostic.observed
          ? workflowDiagnostic.successfulToolExecutionCount
          : workflowProgress.successfulToolExecutionCount,
      ordinaryCandidateFailedToolResultCount: workflowDiagnostic.observed
        ? workflowDiagnostic.failedToolResultCount
        : workflowProgress.failedToolResultCount,
      ordinaryCandidateUnsettledToolResultCount: workflowDiagnostic.observed
        ? workflowDiagnostic.unsettledToolResultCount
        : workflowProgress.unsettledToolResultCount,
      ordinaryCandidateOpenToolCallCount: workflowProgress.openToolCallCount,
      ordinaryCandidateActiveToolCategory: workflowProgress.activeToolCategory,
      ordinaryCandidateToolAttemptCategoryCounts:
        workflowDiagnostic.observed
          ? workflowDiagnostic.toolAttemptCategoryCounts
          : workflowProgress.toolAttemptCategoryCounts,
      ordinaryCandidateSuccessfulToolCategoryCounts:
        workflowDiagnostic.observed
          ? workflowDiagnostic.successfulToolCategoryCounts
          : workflowProgress.successfulToolCategoryCounts,
      ordinaryCandidateProviderAttemptRecordCount:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerAttemptRecordCount
          : workflowProgress.providerAttemptRecordCount,
      ordinaryCandidateProviderTerminalRecordCount:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerTerminalRecordCount
          : workflowProgress.providerTerminalRecordCount,
      ordinaryCandidateProviderReceiptCountersBound:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerReceiptCountersBound
          : workflowProgress.providerReceiptCountersBound,
      ordinaryCandidateProviderLogicalCallCount:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerLogicalCallCount
          : workflowProgress.providerLogicalCallCount,
      ordinaryCandidateProviderAttemptCount:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerAttemptCount
          : workflowProgress.providerAttemptCount,
      ordinaryCandidateProviderAttemptStatusCounts:
        workflowDiagnostic.observed
          ? workflowDiagnostic.providerAttemptStatusCounts
          : workflowProgress.providerAttemptStatusCounts,
      ordinaryCandidateTerminalReasonClass:
        workflowDiagnostic.terminalReasonClass,
      ordinaryCandidateTerminalErrorItemCount:
        workflowDiagnostic.terminalErrorItemCount,
      ordinaryCandidateTerminalErrorItemAuthorityBound:
        workflowDiagnostic.terminalErrorItemAuthorityBound,
      ordinaryCandidateToolInventoryAvailability:
        workflowDiagnostic.observed
          ? workflowDiagnostic.toolInventoryAvailability
          : workflowProgress.toolInventoryAvailability,
      ordinaryCandidateProviderReceiptAvailability:
        workflowDiagnostic.providerReceiptAvailability,
      ordinaryCandidateTypedResultObserved:
        workflowDiagnostic.ordinaryResultObserved,
      ordinaryCandidateTypedResultReasonCode:
        workflowDiagnostic.ordinaryResultReasonCode,
      ordinaryCandidateTypedResultOrigin:
        workflowDiagnostic.ordinaryResultCandidateOrigin,
      ordinaryCandidateTypedResultProjectionClass:
        workflowDiagnostic.ordinaryResultProjectionClass,
      ordinaryCandidateTypedResultDigest:
        workflowDiagnostic.ordinaryResultDigest,
      ordinaryCandidateTypedResultTextSha256:
        workflowDiagnostic.ordinaryResultTextSha256,
      ordinaryCandidateTypedResultMarkerObserved:
        workflowDiagnostic.ordinaryResultMarkerObserved,
      ordinaryCandidateTypedResultResearchMarkerObserved:
        workflowDiagnostic.ordinaryResultResearchMarkerObserved,
      ordinaryCandidateTypedResultWritingMarkerObserved:
        workflowDiagnostic.ordinaryResultWritingMarkerObserved,
      ordinaryCandidateInventoryDigest: workflowDiagnostic.observed
        ? workflowDiagnostic.inventoryDigest
        : workflowProgress.inventoryDigest,
      ordinaryCandidateProgressDigest: workflowProgress.inventoryDigest,
      ordinaryCandidateFailureClassification: workflowDiagnostic.observed
        ? workflowFailureProjection.classification
        : 'not_observed',
      ordinaryCandidateFailureObservationCode: workflowDiagnostic.observed
        ? workflowFailureProjection.observationCode
        : 'workflow_diagnostic_not_observed',
      ordinaryCandidateProviderTransportSucceeded:
        workflowDiagnostic.observed && workflowFailureProjection.providerTransportSucceeded,
      ordinaryCandidateUpstreamCauseConfirmed:
        workflowDiagnostic.observed && workflowFailureProjection.upstreamCauseConfirmed,
      ordinaryCandidateFailureDiagnosticDigest: workflowDiagnostic.observed
        ? workflowFailureProjection.diagnosticDigest
        : '',
      ordinaryCandidateProviderFailureObserved: providerFailureDiagnostic.observed,
      ordinaryCandidateProviderFailureObservationCode:
        providerFailureDiagnostic.observationCode,
      ordinaryCandidateProviderFailureReasonCode: providerFailureDiagnostic.reasonCode,
      ordinaryCandidateProviderFailureKind: providerFailureDiagnostic.providerKind,
      ordinaryCandidateProviderFailureStatus: providerFailureDiagnostic.providerStatus,
      ordinaryCandidateProviderFailureRetryable: providerFailureDiagnostic.providerRetryable,
      ordinaryCandidateProviderFailureAuthStatus: providerFailureDiagnostic.providerAuthStatus,
      ordinaryCandidateProviderFailureDiagnosticDigest:
        providerFailureDiagnostic.diagnosticDigest,
      failedChildDiagnosticObserved: failedChildSelection.observed,
      failedChildDiagnosticReasonCode: failedChildSelection.reasonCode,
      failedChildDiagnosticTaskAttemptCount: failedChildSelection.taskAttemptCount,
      failedChildDiagnosticTaskExecutionCount: failedChildSelection.taskExecutionCount,
      failedChildDiagnosticSummaryCount: failedChildSelection.summaryCount,
      failedChildDiagnosticStatusBound: failedChildSelection.statusBound,
      failedChildDiagnosticProfileBound: failedChildSelection.profileBound,
      failedChildDiagnosticForegroundBound: failedChildSelection.foregroundBound,
      failedChildDiagnosticChildTurnIdAvailable:
        failedChildSelection.childTurnIdAvailable,
      failedChildDiagnosticParentThreadIdHash: failedChildSelection.parentThreadIdHash,
      failedChildDiagnosticParentTurnIdHash: failedChildSelection.parentTurnIdHash,
      failedChildDiagnosticParentToolCallIdHash:
        failedChildSelection.parentToolCallIdHash,
      failedChildDiagnosticChildRunIdHash: failedChildSelection.childRunIdHash,
      failedChildDiagnosticChildThreadIdHash: failedChildSelection.childThreadIdHash,
      failedChildDiagnosticChildTurnIdHash: failedChildTerminalDiagnostic.observed
        ? failedChildTerminalDiagnostic.turnIdHash
        : failedChildSelection.childTurnIdHash,
      failedChildDiagnosticThreadObserved: failedChildThreadObserved,
      failedChildDiagnosticTurnReasonCode: failedChildTerminalDiagnostic.reasonCode,
      failedChildDiagnosticTurnStatus: failedChildTerminalDiagnostic.turnStatus,
      failedChildDiagnosticTurnErrorCode: failedChildTerminalDiagnostic.turnErrorCode,
      failedChildDiagnosticTurnErrorCodeHash:
        failedChildTerminalDiagnostic.turnErrorCodeHash,
      failedChildDiagnosticToolAttemptCount:
        failedChildTerminalDiagnostic.toolAttemptCount,
      failedChildDiagnosticSuccessfulToolExecutionCount:
        failedChildTerminalDiagnostic.successfulToolExecutionCount,
      failedChildDiagnosticFailedToolResultCount:
        failedChildTerminalDiagnostic.failedToolResultCount,
      failedChildDiagnosticUnsettledToolResultCount:
        failedChildTerminalDiagnostic.unsettledToolResultCount,
      failedChildDiagnosticTerminalReasonClass:
        failedChildTerminalDiagnostic.terminalReasonClass,
      failedChildDiagnosticTerminalErrorItemCount:
        failedChildTerminalDiagnostic.terminalErrorItemCount,
      failedChildDiagnosticTerminalErrorItemAuthorityBound:
        failedChildTerminalDiagnostic.terminalErrorItemAuthorityBound,
      failedChildDiagnosticToolInventoryAvailability:
        failedChildTerminalDiagnostic.toolInventoryAvailability,
      failedChildDiagnosticProviderReceiptAvailability:
        failedChildTerminalDiagnostic.providerReceiptAvailability,
      failedChildDiagnosticInventoryDigest: failedChildTerminalDiagnostic.inventoryDigest,
      ordinaryCandidateToolFailureObserved: toolFailureDiagnostic.observed,
      ordinaryCandidateToolFailureObservationCode: toolFailureDiagnostic.observationCode,
      ordinaryCandidateToolFailureEvidenceCode: toolFailureDiagnostic.evidenceCode,
      ordinaryCandidateToolFailureGuardBound: toolFailureDiagnostic.toolFailureGuardBound,
      ordinaryCandidateToolFailureGuardCount: toolFailureDiagnostic.guardCount,
      ordinaryCandidateToolFailureMaxStormCount: toolFailureDiagnostic.maxStormCount,
      ordinaryCandidateToolFailureGuardKind: toolFailureDiagnostic.guardKind,
      ordinaryCandidateToolFailureToolCategory: toolFailureDiagnostic.toolCategory,
      ordinaryCandidateToolFailureToolNameHash: toolFailureDiagnostic.toolNameHash,
      ordinaryCandidateInvalidToolArgumentGuardCount:
        toolFailureDiagnostic.invalidArgumentGuardCount,
      ordinaryCandidateInvalidToolArgumentMaxStormCount:
        toolFailureDiagnostic.invalidArgumentMaxStormCount,
      ordinaryCandidateInvalidToolArgumentToolCategory:
        toolFailureDiagnostic.invalidArgumentToolCategory,
      ordinaryCandidateInvalidToolArgumentToolNameHash:
        toolFailureDiagnostic.invalidArgumentToolNameHash,
      ordinaryCandidateToolFailureTerminalReason: toolFailureDiagnostic.terminalReason,
      ordinaryCandidateToolFailureTerminalCode: toolFailureDiagnostic.terminalCode,
      ordinaryCandidateToolFailureDiagnosticDigest: toolFailureDiagnostic.diagnosticDigest
    }
    providerEvidence = providerTurnEvidence(firstEvidence, provider)
    const ordinaryProviderEvidenceSnapshot = Object.freeze({ ...providerEvidence })
    report.provider = {
      ...report.provider,
      ...providerEvidence,
      ordinaryEvidence: ordinaryProviderEvidenceSnapshot
    }
    report.workflow = {
      ...report.workflow,
      readObserved: groups.read,
      planObserved: groups.plan,
      todoObserved: groups.todo,
      writeObserved: groups.write,
      successfulToolResultsObserved: Object.values(groups).every(Boolean),
      todosCompleted: firstEvidence.todosPresent && firstEvidence.todosTerminal,
      exactlyOneBoundedSubagentCompleted:
        firstEvidence.boundedSubagentPresent && firstEvidence.boundedSubagentCount === 1,
      isolatedGitRepositoryObserved: repositoryAcceptance.gitRepository,
      protectedRepositoryInputsBound:
        repositoryAcceptance.protectedInputsBound &&
        repositoryAcceptance.acceptanceContractBound,
      onlyIntendedSourceChanged: repositoryAcceptance.onlyIntendedSourceChanged
    }
    if (ordinaryDisposition !== 'completed') {
      const preHostFailureSnapshot = ordinaryWorkflowFailureStageSnapshot(report)
      if (preHostFailureSnapshot) report.stageSnapshots.push(preHostFailureSnapshot)
    }
    if (ordinaryDisposition === 'completed') {
      firstCompletedScreenshot = await captureRendererScreenshot(
        firstDebugPort,
        join(sandboxRoot, 'visual-evidence', 'first-completed.png')
      )
    }
    report.workflow.firstRendererResultDigestBound = firstRenderedResult.ok
    report.workflow.firstRendererResultDigest = firstRenderedResult.resultDigest
    const bashHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId: firstEvidence.threadId,
      turnId: firstEvidence.resultTurnId,
      toolName: 'bash',
      workspace,
      arguments: { command: repositoryAuthority.testCommand },
      timeoutMs: Math.min(timeoutMs, 20_000)
    })
    const gitHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId: firstEvidence.threadId,
      turnId: firstEvidence.resultTurnId,
      toolName: 'bash',
      workspace,
      arguments: { command: repositoryAuthority.gitCheckCommand },
      timeoutMs: Math.min(timeoutMs, 20_000)
    })
    const gitHostInvocationBound = privateHostToolExecutionBindingMatches(
      gitHostObservation,
      {
        threadId: firstEvidence.threadId,
        turnId: firstEvidence.resultTurnId,
        toolName: 'bash',
        workspace,
        arguments: { command: repositoryAuthority.gitCheckCommand }
      }
    )
    const childSummary = firstEvidence.boundedSubagentPresent
      ? firstEvidence.subagents[0]
      : null
    let childProviderEvidence = providerTurnEvidence(null, provider)
    let childSubagentReviewReadAttemptCount = 0
    let childSubagentReviewReadsBound = false
    let childSubagentReviewReadObservationDigest = ''
    if (childSummary?.childThreadId && childSummary?.childTurnId) {
      let childObservation = await observeRenderer({
        debugPort: firstDebugPort,
        workspace,
        exactThreadId: childSummary.childThreadId,
        timeoutMs: Math.min(timeoutMs, 20_000)
      })
      const childHighestSeq = childObservation?.thread?.latestSeq
      if (Number.isSafeInteger(childHighestSeq) && childHighestSeq > 0) {
        childObservation = await observeRenderer({
          debugPort: firstDebugPort,
          workspace,
          exactThreadId: childSummary.childThreadId,
          providerReceiptScope: {
            baselineSeq: 0,
            expectedTurnId: childSummary.childTurnId,
            expectedHighestSeq: childHighestSeq
          },
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
      }
      const childWorkflow = workflowEvidence(childObservation, workspace, '__no_parent_marker__')
      childProviderEvidence = providerTurnEvidence({
        ...childWorkflow,
        resultTurnId: childSummary.childTurnId
      }, provider)
      const childReadHostObservations = []
      for (const path of childReviewPaths) {
        childReadHostObservations.push(await observeHostToolExecution({
          debugPort: firstDebugPort,
          threadId: childSummary.childThreadId,
          turnId: childSummary.childTurnId,
          toolName: 'read',
          workspace,
          arguments: { path },
          timeoutMs: Math.min(timeoutMs, 10_000)
        }))
      }
      const childReadHostObservationMatches = childReadHostObservations.map(
        (observation, index) => privateHostToolExecutionBindingMatches(observation, {
          threadId: childSummary.childThreadId,
          turnId: childSummary.childTurnId,
          toolName: 'read',
          workspace,
          arguments: { path: childReviewPaths[index] }
        })
      )
      const childReviewReadSet = milestoneAChildReviewReadSetEvidence(
        repositoryAuthority,
        childObservation.thread,
        childSummary.childTurnId,
        childReadHostObservationMatches
      )
      childSubagentReviewReadAttemptCount = childReviewReadSet.readAttemptCount
      childSubagentReviewReadsBound = childReviewReadSet.ok
      childSubagentReviewReadObservationDigest = childSubagentReviewReadsBound
        ? sha256(canonicalJSON(childReadHostObservations.map((item) => ({
            observationDigest: item.observationDigest,
            authorityBindingDigest: item.authorityBindingDigest
          }))))
        : ''
    }
    const childSubagentProviderReceiptBound =
      childProviderEvidence.networkTurnCompleted === true
    const scheduleConfigEvidence = packagedScheduleMcpConfigEvidence({
      isolatedHome,
      appPath,
      target,
      schedulePort
    })
    const resultTurnContract = packagedResultTurnContractEvidence(
      firstEvidence,
      firstReady?.runtimeSkills,
      firstWorkbench.publicSeam,
      artifact,
      scheduleConfigEvidence
    )
    const ordinaryMCPObserved =
      resultTurnContract.exactOrdinaryMCPResultBound &&
      resultTurnContract.emptyInputBoundByStrictSchema &&
      resultTurnContract.packageCatalogBound &&
      resultTurnContract.packageConfigBound
    const researchWritingObserved = ordinaryResultEvidence.bound &&
      groups.read && childSubagentReviewReadsBound && gitHostInvocationBound &&
      resultTurnContract.runSkillExecutionCount === 1 && ordinaryMCPObserved
    const parentOwnedTest = runParentOwnedRepositoryTest(repositoryAuthority)
    report.parentOwnedRepositoryTest = {
      ...parentOwnedTest,
      substitutesForAgentInvocationBinding: false
    }
    const testReceipt = testReceiptEvidence(
      repositoryAuthority,
      firstEvidence.thread,
      firstEvidence.resultTurnId,
      bashHostObservation
    )
    report.workflow = {
      ...report.workflow,
      parentOwnedTestCrossCheckPassed: parentOwnedTest.passed,
      parentOwnedTestCommandDigest: parentOwnedTest.commandDigest,
      parentOwnedTestRepositoryStable: parentOwnedTest.repositoryStableBeforeAndAfter,
      actualBashTestResultBound: testReceipt.actualBashToolResultBound,
      hostOwnedToolInvocationBound: testReceipt.hostOwnedToolInvocationBound,
      bashHostObservationReasonCode: safeHostObservationReasonCode(
        bashHostObservation.reasonCode
      ),
      bashHostObservationTransportStatus: safeHostObservationTransportStatus(
        bashHostObservation.transportStatus
      ),
      bashHostObservationAttemptCount: safeHostObservationAttemptCount(
        bashHostObservation.attemptCount
      ),
      bashHostObservationDigest: testReceipt.hostObservationDigest,
      bashHostAuthorityBindingDigest: testReceipt.hostAuthorityBindingDigest,
      gitCommandObserved: gitHostInvocationBound,
      gitHostObservationDigest: gitHostInvocationBound
        ? gitHostObservation.observationDigest
        : '',
      gitHostAuthorityBindingDigest: gitHostInvocationBound
        ? gitHostObservation.authorityBindingDigest
        : '',
      gitHostObservationReasonCode: safeHostObservationReasonCode(
        gitHostObservation.reasonCode
      ),
      gitHostObservationTransportStatus: safeHostObservationTransportStatus(
        gitHostObservation.transportStatus
      ),
      gitHostObservationAttemptCount: safeHostObservationAttemptCount(
        gitHostObservation.attemptCount
      ),
      skillObserved: resultTurnContract.runSkillExecutionCount === 1 &&
        resultTurnContract.runSkillAttemptCount === 1 &&
        resultTurnContract.skillCatalogBound,
      ordinaryMCPObserved,
      ordinaryMCPResultTurnAttemptCount:
        resultTurnContract.ordinaryMCPAttemptCount,
      ordinaryMCPResultTurnExecutionCount:
        resultTurnContract.ordinaryMCPExecutionCount,
      ordinaryMCPStrictEmptyInputBound:
        resultTurnContract.emptyInputBoundByStrictSchema,
      ordinaryMCPPackageCatalogBound: resultTurnContract.packageCatalogBound,
      ordinaryMCPPackageConfigBound: resultTurnContract.packageConfigBound,
      ordinaryMCPConfigSha256: scheduleConfigEvidence.configSha256,
      ordinaryMCPHelperExecutableSha256:
        scheduleConfigEvidence.helperExecutableSha256,
      ordinaryMCPArgumentsDigest: scheduleConfigEvidence.argumentsDigest,
      resultTurnTaskAttemptCount: resultTurnContract.taskAttemptCount,
      resultTurnTaskExecutionCount: resultTurnContract.taskExecutionCount,
      resultTurnSubagentDelegationAttemptCount:
        resultTurnContract.subagentDelegationAttemptCount,
      resultTurnRunSkillAttemptCount: resultTurnContract.runSkillAttemptCount,
      resultTurnRunSkillExecutionCount: resultTurnContract.runSkillExecutionCount,
      resultTurnBashAttemptCount: resultTurnContract.bashAttemptCount,
      resultTurnBashExecutionCount: resultTurnContract.bashExecutionCount,
      resultTurnMCPAttemptCount: resultTurnContract.allMCPAttemptCount,
      resultTurnMCPExecutionCount: resultTurnContract.allMCPExecutionCount,
      resultTurnFundsMCPAttemptCount: resultTurnContract.fundsMCPAttemptCount,
      resultTurnMalformedToolCallAttemptCount:
        resultTurnContract.malformedToolCallAttemptCount,
      taskProfileBound: resultTurnContract.taskProfileBound,
      taskForegroundBound: resultTurnContract.taskForegroundBound,
      taskStepLimitBound: resultTurnContract.taskStepLimitBound,
      taskTimeBudgetBound: resultTurnContract.taskTimeBudgetBound,
      researchWritingObserved,
      childSubagentProviderReceiptBound,
      childSubagentProviderReceiptDigest: childSubagentProviderReceiptBound
        ? childProviderEvidence.providerAttemptReceiptDigest
        : '',
      childSubagentProviderReceiptReasonCode: childProviderEvidence.reasonCode,
      childSubagentProviderReceiptSseReasonCode: childProviderEvidence.sseReasonCode,
      childSubagentReviewReadAttemptCount,
      childSubagentReviewReadsBound,
      childSubagentReviewReadObservationDigest,
      selfReportedTestReceiptAuthoritative: false,
      selfReportedTestReceiptObserved: testReceipt.selfReportedReceiptObserved,
      selfReportedTestReceiptShapeValid: testReceipt.selfReportedReceiptShapeValid
    }
    if (ordinaryDisposition === 'completed' &&
        testReceipt.blocker === 'host_owned_tool_invocation_binding_unavailable') {
      const failureSnapshot = ordinaryTestBindingFailureStageSnapshot(bashHostObservation)
      if (failureSnapshot) report.stageSnapshots.push(failureSnapshot)
    }
    const ordinaryFunctionalPassed = ordinaryDisposition === 'completed' &&
      firstRenderedResult.ok &&
      ordinaryReasoningEffortBound &&
      firstEvidence.threadId &&
      firstEvidence.threadId === planEvidence.threadId &&
      planArtifact.ok &&
      firstEvidence.workspaceBound &&
      firstEvidence.terminal &&
      ordinaryResultEvidence.bound &&
      groups.read &&
      groups.plan &&
      groups.todo &&
      groups.write &&
      groups.test &&
      groups.subagent &&
      groups.skills &&
      resultTurnContract.ok &&
      firstEvidence.todosPresent &&
      firstEvidence.todosTerminal &&
      firstEvidence.boundedSubagentPresent &&
      childSubagentProviderReceiptBound &&
      childSubagentReviewReadsBound &&
      gitHostInvocationBound &&
      ordinaryMCPObserved &&
      researchWritingObserved &&
      sourceRepair.changed &&
      repositoryAcceptance.onlyIntendedSourceChanged &&
      repositoryAcceptance.protectedInputsBound &&
      repositoryAcceptance.acceptanceContractBound &&
      testReceipt.passed
    const ordinaryPassed =
      ordinaryFunctionalPassed && providerEvidence.networkTurnCompleted
    report.caseCapability = {
      ...report.caseCapability,
      ordinaryWorkflowCompleted: ordinaryPassed
    }
    report.checks.push(check(
      'ordinary-agent-workflow',
      ordinaryPassed,
      ordinaryFunctionalPassed && !providerEvidence.networkTurnCompleted
        ? 'public SSE replay lacks an exact configured provider/model attempt telemetry plus terminal receipt for the result turn'
        : 'same packaged thread must use UI Plan then Agent mode with completed tools/Todos, a provider-attempt receipt, provider-origin typed result, source repair, real test, bounded subagent, and evidence-backed written summary'
    ))
    report.checks.push(check(
      'parent-owned-repository-test-cross-check',
      parentOwnedTest.passed,
      parentOwnedTest.blocker ||
        'the harness parent independently reran the contract-bound real test in the exact repository with a sanitized cache-bound environment; this does not substitute for Agent invocation binding'
    ))
    report.checks.push(check(
      'real-repository-test',
      groups.test && sourceRepair.changed && repositoryAcceptance.ok && testReceipt.passed,
      (parentOwnedTest.passed ? testReceipt.blocker : parentOwnedTest.blocker) ||
        'the parent-owned contract-bound test cross-check may prove the changed repository, but the Agent test row remains failed until the runtime supplies a host-owned exact tool invocation/command/result binding'
    ))
    report.checks.push(check(
      'bounded-subagent',
      resultTurnContract.taskAttemptCount === 1 &&
        resultTurnContract.taskExecutionCount === 1 &&
        resultTurnContract.taskProfileBound &&
        resultTurnContract.taskForegroundBound &&
        resultTurnContract.taskStepLimitBound &&
        resultTurnContract.taskTimeBudgetBound &&
        firstEvidence.boundedSubagentPresent &&
        childSubagentProviderReceiptBound &&
        childSubagentReviewReadsBound,
      'the result turn must contain exactly one explicitly step/time-bounded foreground task child bound to the exact configured readOnly profile, its own configured-provider terminal receipt, and host-bound reads of every changed source path; run_skill proves Skills separately and is not delegation',
      resultTurnContract.taskAttemptCount === 1 &&
        resultTurnContract.taskExecutionCount === 1 &&
        resultTurnContract.taskProfileBound &&
        resultTurnContract.taskStepLimitBound &&
        resultTurnContract.taskTimeBudgetBound &&
        firstEvidence.boundedSubagentPresent &&
        !childSubagentProviderReceiptBound ? 'live_blocked' : 'failed'
    ))
    report.checks.push(check(
      'git-skill-mcp-research-writing',
      gitHostInvocationBound && resultTurnContract.runSkillAttemptCount === 1 &&
        resultTurnContract.runSkillExecutionCount === 1 &&
        resultTurnContract.skillCatalogBound &&
        resultTurnContract.bashAttemptCount === 2 &&
        resultTurnContract.bashExecutionCount === 2 && ordinaryMCPObserved &&
        researchWritingObserved,
      'the same packaged Agent result turn must execute exactly two expected bash calls, exactly one package-bound Skill, exactly one package-bound non-mutating list canary under its strict empty-input schema, and evidence-backed research/writing output markers'
    ))

    report.workflow = {
      ...report.workflow,
      ordinaryWorkflowCompleted: ordinaryPassed,
      agentModeRestored: agentMode.ok,
      sameThreadPlanAgent: firstEvidence.threadId === planEvidence.threadId,
      readObserved: groups.read,
      planObserved: groups.plan,
      todoObserved: groups.todo,
      writeObserved: groups.write,
      realTestObserved: groups.test && sourceRepair.changed &&
        repositoryAcceptance.ok && testReceipt.passed,
      subagentObserved: resultTurnContract.taskAttemptCount === 1 &&
        resultTurnContract.taskExecutionCount === 1 &&
        resultTurnContract.taskProfileBound && resultTurnContract.taskForegroundBound &&
        resultTurnContract.taskStepLimitBound && resultTurnContract.taskTimeBudgetBound &&
        firstEvidence.boundedSubagentPresent && childSubagentProviderReceiptBound &&
        childSubagentReviewReadsBound,
      successfulToolResultsObserved: Object.values(groups).every(Boolean),
      todosCompleted: firstEvidence.todosPresent && firstEvidence.todosTerminal,
      exactlyOneBoundedSubagentCompleted:
        firstEvidence.boundedSubagentPresent && firstEvidence.boundedSubagentCount === 1,
      isolatedGitRepositoryObserved: repositoryAcceptance.gitRepository,
      protectedRepositoryInputsBound:
        repositoryAcceptance.protectedInputsBound &&
        repositoryAcceptance.acceptanceContractBound,
      onlyIntendedSourceChanged: repositoryAcceptance.onlyIntendedSourceChanged,
      parentOwnedTestCrossCheckPassed: parentOwnedTest.passed,
      gitCommandObserved: gitHostInvocationBound,
      skillObserved: resultTurnContract.runSkillAttemptCount === 1 &&
        resultTurnContract.runSkillExecutionCount === 1 &&
        resultTurnContract.skillCatalogBound,
      ordinaryMCPObserved,
      researchWritingObserved
    }
    if (ordinaryFunctionalPassed) {
      report.stageSnapshots.push(generatedMilestoneStageSnapshot(
        report,
        'ordinary-agent-functional-workflow',
        ['real-repository-test', 'bounded-subagent', 'git-skill-mcp-research-writing']
      ))
    }
    if (ordinaryPassed) {
      report.stageSnapshots.push(generatedMilestoneStageSnapshot(
        report,
        'ordinary-agent-workflow',
        ['ordinary-agent-workflow', 'real-repository-test', 'bounded-subagent']
      ))
    }

    const ordinaryFailureCode = ordinaryWorkflowFailureCode({
      disposition: ordinaryDisposition,
      ordinaryPassed,
      repositoryBlocker: repositoryAcceptance.blocker,
      terminalStatus: workflowDiagnostic.turnStatus,
      ordinaryFunctionalPassed,
      providerReceiptBound: providerEvidence.networkTurnCompleted,
      workflowFailureClassification: workflowFailureProjection.classification
    })
    report.workflow.ordinaryWorkflowFailureCode = ordinaryFailureCode
    if (ordinaryFailureCode) throw new Error(ordinaryFailureCode)

    const protectedFundsSubmit = await cdpComposerSubmit(
      firstDebugPort,
      PROTECTED_FUNDS_REQUEST_PROMPT,
      ['Send']
    )
    if (!protectedFundsSubmit.ok) {
      applyComposerSubmitDiagnostic(report, protectedFundsSubmit)
      report.checks.push(check(
        'case-dependencies-unavailable-with-ordinary-capabilities',
        false,
        protectedFundsSubmit.blocker ||
          'the protected funds request must be submitted through the real composer',
        protectedFundsSubmit.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(protectedFundsSubmit.blocker), {
        liveBlocked: protectedFundsSubmit.blocked
      })
    }
    const protectedFundsResult = await waitForNewTerminalTurn({
      debugPort: firstDebugPort,
      workspace,
      threadId: firstEvidence.threadId,
      previousTurnCount: firstEvidence.turnCount,
      timeoutMs: Math.min(timeoutMs, 120_000)
    })
    protectedFundsUnavailable = protectedFundsResult.evidence
    firstFundsExecutionUnavailable = protectedFundsUnavailable.ok === true
    report.caseCapability = {
      ...report.caseCapability,
      firstLaunchFundsExecutionUnavailable: firstFundsExecutionUnavailable,
      protectedFundsSourceUnavailable: protectedFundsUnavailable.ok === true,
      protectedFundsAcceptedFinalDigest: protectedFundsUnavailable.ok
        ? protectedFundsUnavailable.acceptedFinalDigest
        : '',
      protectedFundsClaimCount: protectedFundsUnavailable.claimCount,
      protectedFundsReceiptCount: protectedFundsUnavailable.receiptCount,
      protectedFundsSuccessfulExecutionCount:
        protectedFundsUnavailable.successfulFundsExecutionCount
    }
    if (!protectedFundsUnavailable.ok) {
      report.checks.push(check(
        'case-dependencies-unavailable-with-ordinary-capabilities',
        false,
        'the protected funds request must terminate as typed SourceUnavailableAnswer with zero claims, receipts, and successful funds executions'
      ))
      throw new Error('packaged_protected_funds_effect_did_not_fail_closed')
    }
    const protectedFundsBaselineEvidence = workflowEvidence(
      protectedFundsResult.observation,
      workspace,
      '',
      firstEvidence.resultTurnId
    )
    const protectedFundsBaselineBound =
      protectedFundsBaselineEvidence.threadId === firstEvidence.threadId &&
      protectedFundsBaselineEvidence.turnCount === firstEvidence.turnCount + 1 &&
      protectedFundsBaselineEvidence.terminal === true &&
      protectedFundsBaselineEvidence.resultTurnId === firstEvidence.resultTurnId &&
      protectedFundsBaselineEvidence.thread?.turns?.at(-1)?.id ===
        protectedFundsUnavailable.turnId
    const protectedFundsSourceUnavailableObserved = protectedFundsBaselineBound &&
      protectedFundsUnavailable.ok === true &&
      protectedFundsUnavailable.claimCount === 0 &&
      protectedFundsUnavailable.receiptCount === 0 &&
      protectedFundsUnavailable.successfulFundsExecutionCount === 0
    report.workflow = {
      ...report.workflow,
      protectedFundsSourceUnavailableObserved,
      protectedFundsBaselineBound,
      protectedFundsAcceptedFinalDigest: protectedFundsUnavailable.ok
        ? protectedFundsUnavailable.acceptedFinalDigest
        : '',
      protectedFundsClaimCount: protectedFundsUnavailable.claimCount,
      protectedFundsReceiptCount: protectedFundsUnavailable.receiptCount,
      protectedFundsSuccessfulExecutionCount:
        protectedFundsUnavailable.successfulFundsExecutionCount
    }
    report.checks.push(check(
      'protected-funds-source-unavailable',
      protectedFundsSourceUnavailableObserved,
      protectedFundsSourceUnavailableObserved
        ? 'protected funds source-unavailable stage completed'
        : 'packaged_protected_funds_turn_baseline_unbound'
    ))
    report.stageSnapshots.push(generatedMilestoneStageSnapshot(
      report,
      'protected-funds-source-unavailable',
      ['protected-funds-source-unavailable']
    ))
    if (!protectedFundsSourceUnavailableObserved) {
      throw new Error('packaged_protected_funds_turn_baseline_unbound')
    }

    const contextMarkers = privateRepositoryContextMarkers(repositoryAuthority)
    const contextRelativePath = repositoryAuthority.context.path
    const contextPath = join(workspace, contextRelativePath)
    const contextFile = hashRegularFile(contextPath, {
      capture: true,
      maximumBytes: repositoryAuthority.context.maxBytes || 2 * 1024 * 1024
    })
    const contextBinding = continuationReadBindingEvidence(
      contextFile,
      repositoryAuthority.context
    )
    const contextReadArguments = Object.freeze({
      path: contextRelativePath,
      limit: repositoryAuthority.context.lineLimit || contextBinding.lineLimit
    })
    // Legacy parser/source closure remains historical-only; formal
    // continuation uses this bounded authority.
    report.workflow = {
      ...report.workflow,
      longContextByteLength: contextFile.byteLength,
      longContextReadLineLimit: contextReadArguments.limit
    }
    if (!contextBinding.ok ||
      !Number.isSafeInteger(contextReadArguments.limit) ||
      contextReadArguments.limit <= 0) {
      throw new Error('packaged_long_context_pre_submit_binding_failed')
    }
    const contextPrompt = boundedContinuationPrompt({
      path: contextRelativePath,
      limit: contextReadArguments.limit
    })
    if (!contextPrompt) throw new Error('packaged_continuation_prompt_invalid')
    const continuationReasoningSelection = await selectComposerReasoningEffort(
      firstDebugPort,
      FORMAL_MODEL,
      FORMAL_REASONING_EFFORT
    )
    if (!continuationReasoningSelection.ok) {
      report.checks.push(check(
        'long-context-continuation',
        false,
        continuationReasoningSelection.blocker ||
          'continuation reasoning effort was not selected through the visible composer',
        continuationReasoningSelection.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(
        continuationReasoningSelection.blocker || 'continuation_reasoning_effort_selection_failed'
      ), { liveBlocked: continuationReasoningSelection.blocked })
    }
    const contextSubmit = await cdpComposerSubmit(
      firstDebugPort,
      contextPrompt,
      ['Send'],
      Math.min(timeoutMs, 60_000)
    )
    if (!contextSubmit.ok) {
      applyComposerSubmitDiagnostic(report, contextSubmit)
      if (contextSubmit.idleFailure) {
        let contextFailureObservation = null
        try {
          contextFailureObservation = await observeRenderer({
            debugPort: firstDebugPort,
            workspace,
            exactThreadId: firstEvidence.threadId,
            timeoutMs: Math.min(5_000, Math.max(1_000, timeoutMs))
          })
        } catch {
          // Preserve the idle diagnostic even when the readonly runtime probe is unavailable.
        }
        report.workflow = {
          ...report.workflow,
          ...longContextIdleFailureWorkflowFields({
            idleFailure: contextSubmit.idleFailure,
            observation: contextFailureObservation,
            runtimePort,
            rootPid: firstChild?.pid,
            expectedRuntimeServerPath: runtimeServerPath(appPath, target)
          })
        }
        report.stageSnapshots.push(generatedMilestoneStageSnapshot(
          report,
          'long-context-idle-failure',
          ['long-context-continuation']
        ))
      }
      report.checks.push(check(
        'long-context-continuation',
        false,
        contextSubmit.blocker || 'long-context continuation entered through the real composer',
        contextSubmit.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(contextSubmit.blocker), { liveBlocked: contextSubmit.blocked })
    }
    const contextWorkflow = await waitForWorkflow({
      debugPort: firstDebugPort,
      workspace,
      timeoutMs,
      resultMarker: '',
      previousTurnCount: protectedFundsBaselineEvidence.turnCount,
      expectedThreadId: firstEvidence.threadId
    })
    let contextEvidence = contextWorkflow.evidence
    let contextObservation = contextWorkflow.observation
    const initialContextTurns = Array.isArray(contextEvidence.thread?.turns)
      ? contextEvidence.thread.turns
      : []
    const initialContextCandidateTurn = contextEvidence.threadId === firstEvidence.threadId &&
      contextEvidence.turnCount > protectedFundsBaselineEvidence.turnCount &&
      ['completed', 'failed', 'aborted'].includes(initialContextTurns.at(-1)?.status)
      ? initialContextTurns.at(-1)
      : null
    const contextReceiptScope = initialContextCandidateTurn?.status === 'completed'
      ? exactLatestTerminalProviderReceiptScope(contextEvidence, {
        previousTurnCount: protectedFundsBaselineEvidence.turnCount,
        baselineSeq: protectedFundsBaselineEvidence.thread?.latestSeq,
        expectedThreadId: firstEvidence.threadId
      })
      : null
    let contextReceiptReplayAttempted = false
    let contextReceiptReplayObserved = false
    let contextReceiptReplayReasonCode = initialContextCandidateTurn &&
      initialContextCandidateTurn.status !== 'completed'
      ? 'not_applicable_terminal_failure'
      : 'not_attempted'
    let contextProviderEvidence = providerTurnEvidence(null, provider)
    if (contextReceiptScope) {
      contextReceiptReplayAttempted = true
      contextReceiptReplayReasonCode = 'replay_unavailable'
      try {
        const receiptObservation = await observeRenderer({
          debugPort: firstDebugPort,
          workspace,
          exactThreadId: firstEvidence.threadId,
          acceptedFinalReceiptScope: contextReceiptScope,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
        const replayThread = receiptObservation?.thread
        const replayTurns = Array.isArray(replayThread?.turns) ? replayThread.turns : []
        const replayTurn = replayTurns.find((turn) => turn?.id === contextReceiptScope.expectedTurnId)
        if (replayThread?.id === firstEvidence.threadId && replayTurn?.id === contextReceiptScope.expectedTurnId) {
          contextObservation = receiptObservation
          contextEvidence = workflowEvidence(
            receiptObservation,
            workspace,
            '',
            replayTurn.status === 'completed' ? contextReceiptScope.expectedTurnId : ''
          )
          contextProviderEvidence = providerTurnEvidence(contextEvidence, provider)
          contextReceiptReplayObserved = contextProviderEvidence.networkTurnCompleted === true
          contextReceiptReplayReasonCode = contextProviderEvidence.reasonCode
        }
      } catch {
        contextReceiptReplayReasonCode = 'replay_failed'
      }
    }
    report.provider = {
      ...report.provider,
      longContextEvidence: Object.freeze({ ...contextProviderEvidence })
    }
    const contextDiagnostic = terminalWorkflowDiagnostic(contextEvidence, {
      previousTurnCount: protectedFundsBaselineEvidence.turnCount
    })
    const contextTurns = Array.isArray(contextEvidence.thread?.turns)
      ? contextEvidence.thread.turns
      : []
    const contextCandidateTurn = contextEvidence.threadId === firstEvidence.threadId &&
      contextEvidence.turnCount > protectedFundsBaselineEvidence.turnCount &&
      ['completed', 'failed', 'aborted'].includes(contextTurns.at(-1)?.status)
      ? contextTurns.at(-1)
      : null
    const contextCandidateTurnId = typeof contextCandidateTurn?.id === 'string'
      ? contextCandidateTurn.id
      : ''
    const longContextReasoningEffortBound = continuationReasoningSelection.ok &&
      exactTurnReasoningEffortBound(contextEvidence, FORMAL_REASONING_EFFORT)
    report.provider.longContextReasoningEffortBound = longContextReasoningEffortBound
    const contextBaselineSeq = protectedFundsBaselineEvidence.thread?.latestSeq
    const contextHighestSeq = contextEvidence.thread?.latestSeq
    let contextProviderFailureDiagnostic = emptyProviderFailureDiagnostic()
    if (contextWorkflow.disposition !== 'completed' &&
        contextCandidateTurn?.status === 'failed' &&
        Number.isSafeInteger(contextBaselineSeq) && contextBaselineSeq > 0 &&
        Number.isSafeInteger(contextHighestSeq) && contextHighestSeq > contextBaselineSeq) {
      try {
        const rawProviderFailure = await observeProviderFailureDiagnostic({
          debugPort: firstDebugPort,
          threadId: contextEvidence.threadId,
          baselineSeq: contextBaselineSeq,
          expectedTurnId: contextCandidateTurnId,
          expectedHighestSeq: contextHighestSeq,
          priorCompletedTurnId: protectedFundsBaselineEvidence.thread?.turns?.at(-1)?.id,
          timeoutMs: Math.min(timeoutMs, 20_000)
        })
        contextProviderFailureDiagnostic = providerFailureDiagnosticEvidence(
          rawProviderFailure,
          {
            expectedThreadId: contextEvidence.threadId,
            expectedTurnId: contextCandidateTurnId,
            expectedBaselineSeq: contextBaselineSeq,
            expectedHighestSeq: contextHighestSeq
          }
        )
      } catch {
        contextProviderFailureDiagnostic = emptyProviderFailureDiagnostic(
          'provider_failure_observer_failed'
        )
      }
    }
    report.workflow = {
      ...report.workflow,
      longContextWorkflowDisposition: contextWorkflow.disposition,
      longContextCandidateTurnStatus: contextDiagnostic.turnStatus,
      longContextCandidateTurnIdHash: contextDiagnostic.turnIdHash,
      longContextCandidateTurnErrorCode: contextDiagnostic.turnErrorCode,
      longContextCandidateTurnErrorCodeHash: contextDiagnostic.turnErrorCodeHash,
      longContextPublicToolInventoryAvailability:
        contextDiagnostic.toolInventoryAvailability,
      longContextPublicToolAttemptCount: contextDiagnostic.toolAttemptCount,
      longContextPublicSuccessfulToolExecutionCount:
        contextDiagnostic.successfulToolExecutionCount,
      longContextCandidateFailedToolResultCount:
        contextDiagnostic.failedToolResultCount,
      longContextCandidateUnsettledToolResultCount:
        contextDiagnostic.unsettledToolResultCount,
      longContextCandidateProviderAttemptRecordCount:
        contextDiagnostic.providerAttemptRecordCount,
      longContextCandidateProviderTerminalRecordCount:
        contextDiagnostic.providerTerminalRecordCount,
      longContextCandidateProviderReceiptCountersBound:
        contextDiagnostic.providerReceiptCountersBound,
      longContextCandidateProviderLogicalCallCount:
        contextDiagnostic.providerLogicalCallCount,
      longContextCandidateProviderAttemptCount:
        contextDiagnostic.providerAttemptCount,
      longContextCandidateTerminalReasonClass:
        contextDiagnostic.terminalReasonClass,
      longContextCandidateTerminalErrorItemCount:
        contextDiagnostic.terminalErrorItemCount,
      longContextCandidateTerminalErrorItemAuthorityBound:
        contextDiagnostic.terminalErrorItemAuthorityBound,
      longContextCandidateInventoryDigest: contextDiagnostic.inventoryDigest,
      longContextProviderFailureObserved: contextProviderFailureDiagnostic.observed,
      longContextProviderFailureObservationCode:
        contextProviderFailureDiagnostic.observationCode,
      longContextProviderFailureReasonCode:
        contextProviderFailureDiagnostic.reasonCode,
      longContextProviderFailureKind: contextProviderFailureDiagnostic.providerKind,
      longContextProviderFailureStatus: contextProviderFailureDiagnostic.providerStatus,
      longContextProviderFailureRetryable:
        contextProviderFailureDiagnostic.providerRetryable,
      longContextProviderFailureAuthStatus:
        contextProviderFailureDiagnostic.providerAuthStatus,
      longContextProviderFailureDiagnosticDigest:
        contextProviderFailureDiagnostic.diagnosticDigest,
      longContextProviderReceiptReplayAttempted: contextReceiptReplayAttempted,
      longContextProviderReceiptReplayObserved: contextReceiptReplayObserved,
      longContextProviderReceiptReplayReasonCode: contextReceiptReplayReasonCode,
      longContextProviderReceiptDigest: contextReceiptReplayObserved
        ? contextProviderEvidence.providerAttemptReceiptDigest
        : ''
    }
    const contextRecord = completedAssistantRecords(contextEvidence.thread).find((item) =>
      item.turnId === contextCandidateTurnId && item.text.trim().length > 0
    )
    const contextHostObservation = await observeHostToolExecution({
      debugPort: firstDebugPort,
      threadId: contextEvidence.threadId,
      turnId: contextCandidateTurnId,
      toolName: 'read',
      workspace,
      arguments: contextReadArguments,
      timeoutMs: Math.min(timeoutMs, 20_000)
    })
    let contextWorkspaceRealPath = ''
    try {
      contextWorkspaceRealPath = realpathSync(repositoryAuthority.workspace)
    } catch {
      contextWorkspaceRealPath = ''
    }
    const contextThreadId = typeof contextEvidence.thread?.id === 'string' &&
      contextEvidence.thread.id === contextEvidence.threadId
      ? contextEvidence.thread.id
      : ''
    const longContextHostReadBound = privateHostToolExecutionBindingMatches(
      contextHostObservation,
      {
        threadId: contextThreadId,
        turnId: contextCandidateTurnId,
        toolName: 'read',
        workspace: contextWorkspaceRealPath,
        arguments: contextReadArguments
      }
    )
    report.workflow = {
      ...report.workflow,
      longContextHostReadBound,
      longContextHostObservationTransportStatus:
        contextHostObservation.transportStatus,
      longContextHostObservationDigest: longContextHostReadBound
        ? contextHostObservation.observationDigest
        : ''
    }
    const contextAcceptedFinalEvidence = acceptedFinalOrdinaryContinuationEvidence(
      contextObservation,
      {
        expectedThreadId: firstEvidence.threadId,
        expectedTurnId: contextCandidateTurnId,
        requiredMarkers: [],
        provider
      }
    )
    const longContextCaseWorkspaceScopeBound = readonlyThreadWorkspaceScopeMatches(
      contextEvidence.thread,
      firstEvidence.threadId,
      workspace
    )
    const longContextSubagentContinuityBound =
      exactlyOneSubagentContinuityMatches(firstEvidence, protectedFundsBaselineEvidence) &&
      exactlyOneSubagentContinuityMatches(firstEvidence, contextEvidence)
    const postContextRepositoryAcceptance = verifyRepositoryAcceptance(
      repositoryAuthority,
      'final'
    )
    repositoryAcceptance = postContextRepositoryAcceptance
    const longContextContinuationObserved = contextWorkflow.disposition === 'completed' &&
      longContextReasoningEffortBound &&
      longContextCaseWorkspaceScopeBound &&
      contextFile.regular &&
      contextReadArguments.limit > 0 &&
      contextFile.byteLength <= (repositoryAuthority.context.maxBytes || 2 * 1024 * 1024) &&
      contextEvidence.threadId === firstEvidence.threadId &&
      contextEvidence.turnCount === protectedFundsBaselineEvidence.turnCount + 1 &&
      contextEvidence.terminal &&
      Boolean(contextRecord) &&
      longContextHostReadBound &&
      contextReceiptReplayObserved &&
      contextAcceptedFinalEvidence.ok &&
      postContextRepositoryAcceptance.ok &&
      longContextSubagentContinuityBound
    const ordinaryContinuedAfterProtectedFundsBlock =
      longContextContinuationObserved && protectedFundsUnavailable.ok === true &&
      contextAcceptedFinalEvidence.threadId === protectedFundsUnavailable.threadId &&
      contextAcceptedFinalEvidence.turnId !== protectedFundsUnavailable.turnId
    report.caseCapability = {
      ...report.caseCapability,
      ordinaryContinuedAfterProtectedFundsBlock
    }
    const contextExternallyBlocked = contextSubmit.blocked === true
    const longContextContinuationReasonCode = contextAcceptedFinalEvidence.reasonCode
    const longContextContinuationFailure = longContextContinuationFailureCode({
      disposition: contextWorkflow.disposition,
      externallyBlocked: contextExternallyBlocked,
      acceptedFinalReasonCode: longContextContinuationReasonCode,
      hostReadBound: longContextHostReadBound,
      reasoningEffortBound: longContextReasoningEffortBound,
      providerReceiptReplayObserved: contextReceiptReplayObserved,
      repositoryBound: postContextRepositoryAcceptance.ok,
      subagentStateBound: longContextSubagentContinuityBound,
      caseWorkspaceScopeBound: longContextCaseWorkspaceScopeBound
    })
    report.workflow = {
      ...report.workflow,
      longContextContinuationObserved,
      longContextCaseWorkspaceScopeBound,
      longContextSubagentContinuityBound,
      longContextSubagentContinuityBaselineDigest: firstEvidence.subagentContinuityDigest,
      longContextSubagentContinuityProtectedFundsDigest:
        protectedFundsBaselineEvidence.subagentContinuityDigest,
      longContextSubagentContinuityDigest: contextEvidence.subagentContinuityDigest,
      longContextContinuationReasonCode,
      longContextContinuationFailureCode: longContextContinuationFailure,
      longContextProviderReceiptBound: contextReceiptReplayObserved,
      longContextProviderReceiptDigest: contextReceiptReplayObserved
        ? contextProviderEvidence.providerAttemptReceiptDigest
        : '',
      longContextAcceptedFinalBound: contextAcceptedFinalEvidence.ok,
      longContextAcceptedFinalDigest: contextAcceptedFinalEvidence.acceptedFinalDigest,
      longContextProviderClosureDigest:
        contextAcceptedFinalEvidence.providerClosureDigest
    }
    report.stageSnapshots.push(generatedMilestoneStageSnapshot(
      report,
      'long-context-continuation',
      ['long-context-continuation']
    ))
    report.checks.push(check(
      'long-context-continuation',
      longContextContinuationObserved,
      longContextContinuationObserved
        ? 'ordinary bounded continuation evidence observed'
        : longContextContinuationFailure,
      contextExternallyBlocked ? 'live_blocked' : 'failed'
    ))
    if (!longContextContinuationObserved) throw new Error('packaged_long_context_continuation_failed')

    // Bind the exact set of durable compactions observed before /compact.  A
    // replay of this set is not evidence that the new manual operation settled.
    const compactionBaseline = compactionBaselineEvidence(firstEvidence.compactions)
    report.workflow = {
      ...report.workflow,
      manualCompactionBaselineBound: true,
      manualCompactionBaselineCount: compactionBaseline.count,
      manualCompactionBaselineDigest: compactionBaseline.digest
    }
    const compactSubmit = await cdpComposerSubmit(firstDebugPort, '/compact', [
      'Apply command',
      'Send'
    ])
    if (!compactSubmit.ok) applyComposerSubmitDiagnostic(report, compactSubmit)
    report.checks.push(check(
      'composer-compaction-submit',
      compactSubmit.ok,
      compactSubmit.blocker || '/compact entered through the real composer and primary action button',
      compactSubmit.blocked ? 'live_blocked' : 'failed'
    ))
    if (!compactSubmit.ok) {
      throw Object.assign(new Error(compactSubmit.blocker), { liveBlocked: compactSubmit.blocked })
    }
    const compacted = await waitForCompaction({
      debugPort: firstDebugPort,
      workspace,
      threadId: firstEvidence.threadId,
      baseline: compactionBaseline,
      timeoutMs: Math.min(timeoutMs, 120_000)
    })
    compactedEvidence = compacted.evidence
    const manualCompactionProof = compacted.proof || manualCompactionProofEvidence(
      compactedEvidence,
      compactionBaseline
    )
    const nonzeroCompactionPassed = manualCompactionProof.ok === true &&
      manualCompactionProof.newAfterBaseline === true
    report.workflow = {
      ...report.workflow,
      compactionCount: compactedEvidence.compactionCount,
      manualCompactionObserved: manualCompactionProof.manualCompactionObserved === true,
      manualCompactionAuto: manualCompactionProof.manualCompactionAuto,
      manualCompactionReplacedTokens: manualCompactionProof.replacedTokens || 0,
      manualCompactionSourceDigest: manualCompactionProof.sourceDigest || '',
      manualCompactionSourceItemIdsDigest:
        manualCompactionProof.sourceItemIdsDigest || '',
      manualCompactionSourceAncestryBound:
        manualCompactionProof.manualCompactionSourceAncestryBound === true,
      manualCompactionNonzeroBound:
        manualCompactionProof.manualCompactionNonzeroBound === true,
      manualCompactionProjectionClass:
        COMPACTION_PROJECTION_CLASSES.has(manualCompactionProof.projectionClass)
          ? manualCompactionProof.projectionClass
          : 'not_observed',
      manualCompactionCandidateDigest: manualCompactionProof.candidateDigest || '',
      manualCompactionNewAfterBaseline: nonzeroCompactionPassed
    }
    report.checks.push(check(
      'nonzero-compaction',
      nonzeroCompactionPassed,
      nonzeroCompactionPassed
        ? 'manual compaction proof observed after baseline'
        : NONZERO_COMPACTION_FAILURE_REASON_CODE
    ))
    if (!nonzeroCompactionPassed) {
      // This is a harness/product failure, not a Hub prerequisite.  Fail at
      // the check boundary before normal quit can overwrite the diagnosis.
      throw new Error(NONZERO_COMPACTION_FAILURE_REASON_CODE)
    }
    if (compactSubmit.ok && nonzeroCompactionPassed) {
      const compactionSnapshot = generatedMilestoneStageSnapshot(
        report,
        'composer-compaction-submit',
        ['composer-compaction-submit', 'nonzero-compaction']
      )
      if (compactionSnapshot) report.stageSnapshots.push(compactionSnapshot)
    }

    const firstProcessIds = [firstChild.pid, ...descendantPids(firstChild.pid)]
    const firstQuitRequest = await cdpNormalQuit(firstDebugPort)
    const firstExited = firstQuitRequest.ok && await waitForExit(firstChild, 30_000)
    await sleep(1000)
    const afterFirstExit = revalidatePackagedArtifact(
      'afterFirstExit',
      'after-first-exit'
    )
    const firstExitCodeZero = firstChild.exitCode === 0
    const firstSignalNone = firstChild.signalCode === null
    const firstNoResidual = firstProcessIds.every((pid) => !processExists(pid))
    const firstTaskOwnedResidualCount = taskOwnedProcessPids(sandboxRoot).length
    const firstDebugPortClosed = listeningPids(firstDebugPort).length === 0
    const firstRuntimePortClosed = listeningPids(runtimePort).length === 0
    const firstArtifactRevalidated = afterFirstExit.ok
    const firstQuitLifecycle = normalQuitLifecycleEvidence({
      requestOk: firstQuitRequest.ok,
      processExited: firstExited,
      exitCode: firstExitCodeZero ? 0 : firstChild.exitCode,
      signal: firstSignalNone ? null : firstChild.signalCode,
      residualOwnedProcess: !firstNoResidual || firstTaskOwnedResidualCount > 0,
      debugPortOpen: !firstDebugPortClosed,
      runtimePortOpen: !firstRuntimePortClosed,
      artifactRevalidationOk: firstArtifactRevalidated,
      blocked: firstQuitRequest.blocked
    })
    firstNormalQuit = firstQuitLifecycle.ok
    firstNormalQuitReasonCode = firstQuitLifecycle.reasonCode
    report.workflow = {
      ...report.workflow,
      normalFirstQuitObserved: firstNormalQuit,
      normalFirstQuitReasonCode: firstNormalQuitReasonCode,
      normalFirstQuitResidualOwnedProcessCount: firstTaskOwnedResidualCount,
      firstExitCode: firstChild.exitCode,
      firstExitSignal: firstChild.signalCode
    }
    report.checks.push(check(
      'normal-first-quit',
      firstNormalQuit,
      firstQuitLifecycle.blocker ||
        'Alt+F4 must close the first packaged process and its Go sidecar normally, then the artifact/worktree/codesign binding must revalidate',
      firstQuitLifecycle.blocked ? 'live_blocked' : 'failed'
    ))
    if (firstNormalQuit) {
      report.stageSnapshots.push(generatedMilestoneStageSnapshot(
        report,
        'normal-first-quit',
        ['normal-first-quit']
      ))
    }
    if (!firstNormalQuit) {
      throw Object.assign(
        new Error(firstQuitLifecycle.reasonCode),
        { liveBlocked: firstQuitLifecycle.blocked }
      )
    }

    secondDebugPort = await getFreePort()
    while (secondDebugPort === runtimePort || secondDebugPort === schedulePort ||
      secondDebugPort === firstDebugPort) {
      secondDebugPort = await getFreePort()
    }
    const beforeSecondLaunch = revalidatePackagedArtifact(
      'beforeSecondLaunch',
      'before-second-launch'
    )
    if (!beforeSecondLaunch.ok) {
      report.checks.push(check(
        'fresh-packaged-relaunch',
        false,
        beforeSecondLaunch.blocker || 'formal package changed before relaunch'
      ))
      throw new Error(beforeSecondLaunch.blocker || 'formal_package_changed_before_relaunch')
    }
    const secondKeychainUnlock = await isolatedLoginKeychain.unlockForLaunch()
    report.isolation = {
      ...report.isolation,
      isolatedLoginKeychainUnlockCount: secondKeychainUnlock.unlockCount,
      isolatedLoginKeychainDefaultBound: secondKeychainUnlock.defaultKeychainBound,
      isolatedLoginKeychainReusedForTwoLaunches:
        secondKeychainUnlock.reusedForTwoLaunches
    }
    secondChild = launchPackagedApp({
      appPath,
      target,
      debugPort: secondDebugPort,
      childEnv
    })
    const secondWorkbench = await waitForLocalProviderWorkbench({
      debugPort: secondDebugPort,
      workspace,
      runtimePort,
      runtimeDataDir,
      exactThreadId: firstEvidence.threadId,
      timeoutMs: Math.min(timeoutMs, 120_000)
    })
    const recoveredObservation = secondWorkbench.observation
    secondFundsMaterializationFailure =
      missingBundledFundsMaterializationAuthorityEvidence(
        bundledFundsMaterializationSeam
      )
    report.caseCapability = {
      ...report.caseCapability,
      secondLaunchBundledFundsMaterializationFailureObserved:
        secondFundsMaterializationFailure.ok === true,
      fundsTransportAbsentOnBothLaunches:
        firstWorkbench.publicSeam?.fundsServerDiagnosticCount === 0 &&
        firstWorkbench.publicSeam?.fundsTransportAvailable === false &&
        secondWorkbench.publicSeam?.fundsServerDiagnosticCount === 0 &&
        secondWorkbench.publicSeam?.fundsTransportAvailable === false
    }
    const secondTopology = runtimeBackendTopologyEvidence(
      secondChild.pid,
      runtimePort,
      runtimeServerPath(appPath, target)
    )
    const secondProvider = secondWorkbench.provider
    const sameManagedProvider = sameLocalProviderAuthority(provider, secondProvider) &&
      readLocalCredentialScanSource(privateCredentialSource, {
        runId: sandboxRoot, entryAttemptId, provider: secondProvider
      }).ok
    const recoveredPlanArtifact = milestoneAPlanArtifactEvidence(
      repositoryAuthority,
      recoveredObservation?.thread,
      planEvidence.resultTurnId
    )
    const exactPlanArtifactRecovered = recoveredPlanArtifact.ok &&
      recoveredPlanArtifact.relativePath === planArtifact.relativePath &&
      recoveredPlanArtifact.contentHash === planArtifact.contentHash &&
      recoveredPlanArtifact.byteSize === planArtifact.byteSize &&
      recoveredPlanArtifact.resultItemDigest === planArtifact.resultItemDigest &&
      recoveredPlanArtifact.revisionSequenceDigest === planArtifact.revisionSequenceDigest &&
      recoveredPlanArtifact.planCallCount === planArtifact.planCallCount &&
      recoveredPlanArtifact.planResultCount === planArtifact.planResultCount &&
      recoveredPlanArtifact.fileIdentityDigest === planArtifact.fileIdentityDigest &&
      recoveredPlanArtifact.excludeSha256 === planArtifact.excludeSha256
    report.publicSeams.secondLaunch = {
      ...secondWorkbench.publicSeam,
      rendererTargetCount: recoveredObservation?.rendererTargetCount || 0,
      electronMainPid: secondTopology.electronMainPid,
      runtimeListenerPid: secondTopology.runtimeListenerPid,
      runtimeListenerCount: secondTopology.listenerCount,
      runtimeBackendProcessCount: secondTopology.runtimeBackendProcessCount,
      runtimeServerProcessCount: secondTopology.runtimeServerProcessCount,
      desktopPrivateHistoryMigrationProcessCount:
        secondTopology.desktopPrivateHistoryMigrationProcessCount,
      bundledPluginMaterializationProcessCount:
        secondTopology.bundledPluginMaterializationProcessCount,
      unknownRuntimeProcessCount: secondTopology.unknownRuntimeProcessCount,
      rendererReadFailureObserved: secondWorkbench.readDiagnostic?.observed === true,
      rendererReadFailureStage: secondWorkbench.readDiagnostic?.readErrorStage || 'unknown',
      rendererReadFailureTargetCount:
        safeNonNegativeCount(secondWorkbench.readDiagnostic?.rendererTargetCount),
      startupTrace: packagedStartupTraceEvidence(secondChild),
      runtimeListenerIsTaskOwnedDescendant: secondTopology.listenerIsTaskOwnedDescendant,
      exactPackagedRuntimeExecutable: secondTopology.exactPackageExecutable
    }
    recoveredEvidence = workflowEvidence(
      recoveredObservation,
      workspace,
      '',
      firstEvidence.resultTurnId
    )
    const recoveredOrdinaryResultEvidence = ordinaryWorkflowResultEvidence(recoveredEvidence)
    const recoveredRepositoryAcceptance = verifyRepositoryAcceptance(
      repositoryAuthority,
      'final'
    )
    repositoryAcceptance = recoveredRepositoryAcceptance
    const freshRelaunch = secondKeychainUnlock.ok &&
      secondKeychainUnlock.reusedForTwoLaunches &&
      secondChild.pid !== firstChild.pid &&
      secondWorkbench.ok &&
      secondTopology.ok &&
      sameManagedProvider &&
      exactPlanArtifactRecovered &&
      recoveredRepositoryAcceptance.ok
    report.checks.push(check(
      'fresh-packaged-relaunch',
      freshRelaunch,
      secondWorkbench.blocker ||
        recoveredRepositoryAcceptance.blocker ||
        recoveredPlanArtifact.blocker || 'a fresh packaged process must recover one renderer, the same local Registry provider, the exact packaged Go listener, the unchanged acceptance repository authority, and the byte-identical durable plan artifact',
      secondWorkbench.blocked ? 'live_blocked' : 'failed'
    ))
    if (!freshRelaunch) {
      throw Object.assign(
        new Error(secondWorkbench.blocker || 'fresh_packaged_relaunch_topology_failed'),
        { liveBlocked: secondWorkbench.blocked }
      )
    }
    const secondReasoningSelection = await selectComposerReasoningEffort(
      secondDebugPort,
      FORMAL_MODEL,
      FORMAL_REASONING_EFFORT
    )
    report.provider = {
      ...report.provider,
      secondLaunchReasoningEffortSelectedThroughVisibleUi: secondReasoningSelection.ok,
      secondLaunchReasoningControlTitleHash:
        secondReasoningSelection.controlTitleSha256
    }
    if (!secondReasoningSelection.ok) {
      report.checks.push(check(
        'relaunch-provider-continuation',
        false,
        secondReasoningSelection.blocker ||
          'the relaunched process must reselect the fixed formal reasoning effort through the visible composer',
        secondReasoningSelection.blocked ? 'live_blocked' : 'failed'
      ))
      throw Object.assign(new Error(secondReasoningSelection.blocker), {
        liveBlocked: secondReasoningSelection.blocked
      })
    }
    report.caseCapability = {
      ...report.caseCapability,
      ordinaryWorkflowCompleted: ordinaryPassed
    }
    recoveredScreenshot = await captureRendererScreenshot(
      secondDebugPort,
      join(sandboxRoot, 'visual-evidence', 'recovered-after-relaunch.png')
    )
    const visualEvidenceComplete = [firstCompletedScreenshot, recoveredScreenshot].every((item) =>
      item?.captured === true &&
      /^[0-9a-f]{64}$/.test(item.sha256 || '') &&
      Number.isSafeInteger(item.width) &&
      item.width > 0 &&
      Number.isSafeInteger(item.height) &&
      item.height > 0 &&
      Number(item.viewport?.width || 0) > 0 &&
      Number(item.viewport?.height || 0) > 0 &&
      Number(item.viewport?.scale || 0) > 0
    )
    report.checks.push(check(
      'renderer-visual-evidence',
      visualEvidenceComplete,
      'CDP must capture non-empty first-completed and recovered renderer screenshots; screenshots are supplementary evidence, not functional verdicts'
    ))
    const exactThreadRecovered = recoveredEvidence.threadId === firstEvidence.threadId
    const exactTodosRecovered = recoveredEvidence.todosDigest === compactedEvidence.todosDigest
    const exactSubagentsRecovered =
      recoveredEvidence.subagentsDigest === compactedEvidence.subagentsDigest
    const exactCompactionsRecovered = exactCompactionRecoveryMatches(
      compactedEvidence,
      recoveredEvidence
    )
    const exactResultRecovered =
      recoveredOrdinaryResultEvidence.bound &&
      recoveredOrdinaryResultEvidence.textSha256 === ordinaryResultEvidence.textSha256 &&
      exactPlanArtifactRecovered
    report.checks.push(
      check('exact-thread-recovery', exactThreadRecovered, 'exact original thread id must recover'),
      check('exact-todo-recovery', exactTodosRecovered, 'exact Todo public state must recover'),
      check('exact-subagent-recovery', exactSubagentsRecovered, 'exact subagent summary state must recover'),
      check('exact-compaction-recovery', exactCompactionsRecovered, 'the same completed compaction item and source digest must recover'),
      check('exact-result-recovery', exactResultRecovered, 'the exact provider-origin result digest and plan artifact must recover')
    )

    rendererRecovery = await rendererVisibleRecoveryEvidence({
      debugPort: secondDebugPort,
      expectedTodoCount: recoveredEvidence.todos.length,
      expectedSubagentCount: recoveredEvidence.subagents.length,
      expectedResultDigest: report.workflow.firstRendererResultDigest,
      timeoutMs: Math.min(timeoutMs, 30_000)
    })
    report.checks.push(
      check(
        'renderer-visible-thread-recovery',
        exactThreadRecovered && rendererRecovery.timelineVisible === true &&
          rendererRecovery.threadVisible === true,
        rendererRecovery.blocker || 'the relaunched renderer timeline must visibly show unique markers from the exact recovered thread'
      ),
      check(
        'renderer-visible-todo-recovery',
        exactTodosRecovered && rendererRecovery.todoVisible === true,
        rendererRecovery.blocker || 'the relaunched renderer Todo panel must visibly show every recovered completed Todo'
      ),
      check(
        'renderer-visible-subagent-recovery',
        exactSubagentsRecovered && rendererRecovery.subagentVisible === true,
        rendererRecovery.blocker || 'the relaunched renderer summary and subagent inspector must visibly show the recovered terminal subagent'
      ),
      check(
        'renderer-visible-compaction-recovery',
        exactCompactionsRecovered && rendererRecovery.compactionVisible === true,
        rendererRecovery.blocker || 'the relaunched renderer timeline must visibly show the durable compaction marker'
      ),
      check(
        'renderer-visible-result-recovery',
        exactResultRecovered && rendererRecovery.resultVisible === true,
        rendererRecovery.blocker || 'the relaunched renderer timeline must visibly reproduce the first-launch result rendering digest'
      )
    )

    restartedProtectedFundsUnavailable = protectedFundsUnavailableTurnEvidence(
      recoveredObservation,
      firstEvidence.threadId,
      protectedFundsUnavailable.turnId
    )
    report.caseCapability = {
      ...report.caseCapability,
      secondLaunchFundsExecutionUnavailable: restartedProtectedFundsUnavailable.ok === true,
      restartedProtectedFundsSourceUnavailable:
        restartedProtectedFundsUnavailable.ok === true,
      restartedProtectedFundsAcceptedFinalDigest: restartedProtectedFundsUnavailable.ok
        ? restartedProtectedFundsUnavailable.acceptedFinalDigest
        : ''
    }
    if (!restartedProtectedFundsUnavailable.ok) {
      report.checks.push(check(
        'case-dependencies-unavailable-with-ordinary-capabilities',
        false,
        'after restart the exact protected funds turn must recover as typed SourceUnavailableAnswer with zero claims, receipts, and successful funds executions'
      ))
      throw new Error('packaged_protected_funds_effect_did_not_recover')
    }

    const relaunchContinuationSubmit = await cdpComposerSubmit(secondDebugPort, [
      'Continue in this exact recovered thread after the packaged relaunch.',
      'Do not change files, Todos, subagents, or compaction state.',
      'Confirm in one concise natural-language sentence that the prior implementation result is recovered.'
    ].join(' '), ['Send'])
    if (!relaunchContinuationSubmit.ok) {
      applyComposerSubmitDiagnostic(report, relaunchContinuationSubmit)
    }
    let relaunchContinuationEvidence = workflowEvidence(null, workspace, '')
    let relaunchContinuationDisposition = 'not_submitted'
    let relaunchAcceptedFinalEvidence = acceptedFinalOrdinaryContinuationEvidence(null)
    if (relaunchContinuationSubmit.ok) {
      const relaunchContinuation = await waitForWorkflow({
        debugPort: secondDebugPort,
        workspace,
        timeoutMs: Math.min(timeoutMs, 120_000),
        resultMarker: '',
        previousTurnCount: recoveredEvidence.turnCount,
        expectedThreadId: recoveredEvidence.threadId
      })
      relaunchContinuationEvidence = relaunchContinuation.evidence
      relaunchContinuationDisposition = relaunchContinuation.disposition
      relaunchAcceptedFinalEvidence = acceptedFinalOrdinaryContinuationEvidence(
        relaunchContinuation.observation,
        {
          expectedThreadId: recoveredEvidence.threadId,
          expectedTurnId: relaunchContinuationEvidence.resultTurnId,
          requiredMarkers: [],
          provider: secondProvider
        }
      )
    }
    const relaunchReasoningEffortBound = exactTurnReasoningEffortBound(
      relaunchContinuationEvidence,
      FORMAL_REASONING_EFFORT
    )
    report.provider.relaunchReasoningEffortBound = relaunchReasoningEffortBound
    relaunchProviderContinuationObserved = secondReasoningSelection.ok &&
      relaunchReasoningEffortBound &&
      relaunchContinuationSubmit.ok &&
      relaunchContinuationDisposition === 'completed' &&
      relaunchContinuationEvidence.threadId === firstEvidence.threadId &&
      relaunchContinuationEvidence.resultTurnId &&
      relaunchContinuationEvidence.resultTurnId !== firstEvidence.resultTurnId &&
      relaunchContinuationEvidence.terminal &&
      relaunchAcceptedFinalEvidence.ok === true
    report.checks.push(check(
      'relaunch-provider-continuation',
      relaunchProviderContinuationObserved,
      relaunchContinuationSubmit.blocker ||
        'the relaunched packaged renderer must submit a new ordinary continuation in the exact recovered case-bound thread and bind that turn to its configured provider plus one signed success accepted-final delivery',
      relaunchContinuationSubmit.blocked ||
        (relaunchContinuationSubmit.ok && !relaunchAcceptedFinalEvidence.ok)
        ? 'live_blocked'
        : 'failed'
    ))
    if (!relaunchProviderContinuationObserved) {
      throw Object.assign(new Error(
        relaunchContinuationSubmit.blocker || 'packaged_relaunch_provider_continuation_failed'
      ), {
        liveBlocked: relaunchContinuationSubmit.blocked ||
          !relaunchAcceptedFinalEvidence.ok
      })
    }
    const caseDependenciesUnavailableWithOrdinaryCapabilities =
      bundledFundsMaterializationSeam?.evidence?.preconditionEstablished === true &&
      firstFundsMaterializationFailure.ok === true &&
      secondFundsMaterializationFailure.ok === true &&
      firstWorkbench.publicSeam?.fundsServerDiagnosticCount === 0 &&
      firstWorkbench.publicSeam?.fundsTransportAvailable === false &&
      secondWorkbench.publicSeam?.fundsServerDiagnosticCount === 0 &&
      secondWorkbench.publicSeam?.fundsTransportAvailable === false &&
      privateCaseInputs.privateAuthorityInputsAbsent === true &&
      protectedFundsUnavailable.ok === true &&
      protectedFundsUnavailable.threadId === firstEvidence.threadId &&
      protectedFundsUnavailable.claimCount === 0 &&
      protectedFundsUnavailable.receiptCount === 0 &&
      protectedFundsUnavailable.successfulFundsExecutionCount === 0 &&
      restartedProtectedFundsUnavailable.ok === true &&
      restartedProtectedFundsUnavailable.threadId === firstEvidence.threadId &&
      restartedProtectedFundsUnavailable.turnId === protectedFundsUnavailable.turnId &&
      restartedProtectedFundsUnavailable.acceptedFinalDigest ===
        protectedFundsUnavailable.acceptedFinalDigest &&
      restartedProtectedFundsUnavailable.claimCount === 0 &&
      restartedProtectedFundsUnavailable.receiptCount === 0 &&
      restartedProtectedFundsUnavailable.successfulFundsExecutionCount === 0 &&
      firstWorkbench.publicSeam?.ordinaryCatalogNonempty === true &&
      secondWorkbench.publicSeam?.ordinaryCatalogNonempty === true &&
      ordinaryPassed &&
      ordinaryContinuedAfterProtectedFundsBlock &&
      relaunchProviderContinuationObserved
    report.caseCapability = {
      ...report.caseCapability,
      ordinaryContinuedAfterRestart: relaunchProviderContinuationObserved,
      unavailableWhileOrdinaryCapabilitiesRetained:
        caseDependenciesUnavailableWithOrdinaryCapabilities
    }
    report.checks.push(check(
      'case-dependencies-unavailable-with-ordinary-capabilities',
      caseDependenciesUnavailableWithOrdinaryCapabilities,
      'a real packaged bundled-funds materialization failure must leave the funds transport absent while one protected funds request fails closed as typed SourceUnavailableAnswer with zero claims, receipts, and successful funds executions, recovers unchanged, and allows the same Agent/thread to complete ordinary work before and after the protected effect plus restart'
    ))

    // Re-read while the authenticated renderer is still alive; never scan
    // against the cached first-ready identity after later credential changes.
    const finalCredentialProvider = milestoneALocalProvider(await observeRenderer({
      debugPort: secondDebugPort, workspace, timeoutMs: Math.min(timeoutMs, 10_000)
    }))
    const secondProcessIds = [secondChild.pid, ...descendantPids(secondChild.pid)]
    const finalQuitRequest = await cdpNormalQuit(secondDebugPort)
    const secondExited = finalQuitRequest.ok && await waitForExit(secondChild, 30_000)
    await sleep(1000)
    const afterFinalExit = revalidatePackagedArtifact(
      'afterFinalExit',
      'after-final-exit'
    )
    const finalPlanArtifact = milestoneAPlanArtifactEvidence(
      repositoryAuthority,
      relaunchContinuationEvidence.thread,
      planEvidence.resultTurnId
    )
    const exactFinalPlanArtifact = finalPlanArtifact.ok &&
      finalPlanArtifact.relativePath === planArtifact.relativePath &&
      finalPlanArtifact.contentHash === planArtifact.contentHash &&
      finalPlanArtifact.byteSize === planArtifact.byteSize &&
      finalPlanArtifact.resultItemDigest === planArtifact.resultItemDigest &&
      finalPlanArtifact.revisionSequenceDigest === planArtifact.revisionSequenceDigest &&
      finalPlanArtifact.planCallCount === planArtifact.planCallCount &&
      finalPlanArtifact.planResultCount === planArtifact.planResultCount &&
      finalPlanArtifact.fileIdentityDigest === planArtifact.fileIdentityDigest &&
      finalPlanArtifact.excludeSha256 === planArtifact.excludeSha256
    const finalRepositoryAcceptance = verifyRepositoryAcceptance(
      repositoryAuthority,
      'final'
    )
    repositoryAcceptance = finalRepositoryAcceptance
    const finalExitCodeZero = secondChild.exitCode === 0
    const finalSignalNone = secondChild.signalCode === null
    const finalNoResidual = secondProcessIds.every((pid) => !processExists(pid))
    const finalDebugPortClosed = listeningPids(secondDebugPort).length === 0
    const finalRuntimePortClosed = listeningPids(runtimePort).length === 0
    const finalArtifactRevalidated = afterFinalExit.ok &&
      exactFinalPlanArtifact &&
      finalRepositoryAcceptance.ok
    const finalQuitLifecycle = normalQuitLifecycleEvidence({
      requestOk: finalQuitRequest.ok,
      processExited: secondExited,
      exitCode: finalExitCodeZero ? 0 : secondChild.exitCode,
      signal: finalSignalNone ? null : secondChild.signalCode,
      residualOwnedProcess: !finalNoResidual,
      debugPortOpen: !finalDebugPortClosed,
      runtimePortOpen: !finalRuntimePortClosed,
      artifactRevalidationOk: finalArtifactRevalidated,
      blocked: finalQuitRequest.blocked
    })
    finalNormalQuit = finalQuitLifecycle.ok
    finalNormalQuitReasonCode = finalQuitLifecycle.reasonCode
    report.workflow = {
      ...report.workflow,
      normalFinalQuitObserved: finalNormalQuit,
      normalFinalQuitReasonCode: finalNormalQuitReasonCode,
      finalExitCode: secondChild.exitCode,
      finalExitSignal: secondChild.signalCode
    }
    report.checks.push(check(
      'normal-final-quit',
      finalNormalQuit,
      finalQuitLifecycle.blocker ||
        'Alt+F4 must close the relaunched package and Go sidecar normally, then the package, repository, and durable plan bindings must revalidate',
      finalQuitLifecycle.blocked ? 'live_blocked' : 'failed'
    ))

    const zeroResidual = finalNormalQuit &&
      taskOwnedProcessPids(sandboxRoot).length === 0 &&
      listeningPids(firstDebugPort).length === 0 &&
      listeningPids(secondDebugPort).length === 0 &&
      listeningPids(runtimePort).length === 0
    report.checks.push(check(
      'zero-residual-processes',
      zeroResidual,
      'tracked app/renderer/Go processes and allocated listening ports must be absent'
    ))

    credentialScan = await scanLocalCredentialIsolation({
      source: privateCredentialSource, runId: sandboxRoot, entryAttemptId, provider: finalCredentialProvider,
      root: sandboxRoot, settingsPath, reportSnapshot: report
    })
    const finalSecretStore = localProviderSecretStoreEvidence(runtimeDataDir)
    const credentialsIsolated = credentialScan.ok && finalSecretStore.ok
    report.provider.localCredentialEvidence = {
      ...credentialScan, protectedStoreOwnerPrivate: finalSecretStore.ok
    }
    report.checks.push(check(
      'credential-redaction',
      credentialsIsolated,
      'the same entered local credential and its encodings must be absent from settings, runtime data, logs, history, screenshots, and report'
    ))
    report.provider = { ...report.provider, ...providerEvidence }
    report.visualEvidence = {
      firstCompleted: firstCompletedScreenshot,
      recoveredAfterRelaunch: recoveredScreenshot,
      screenshotsUsedAsFunctionalVerdict: false,
      credentialScanCovered: true
    }
    report.workflow = {
      firstLaunchObserved,
      composerWorkflowSubmitted:
        firstReasoningSelection.ok && planMode.ok && planSubmit.ok && agentMode.ok && submit.ok,
      planComposerSubmitted:
        report.workflow?.planComposerSubmitted === true || planSubmit.ok === true,
      planObservationFailureReasonCode:
        report.workflow?.planObservationFailureReasonCode || '',
      planObservationFailurePhase:
        report.workflow?.planObservationFailurePhase || 'none',
      planObservationFailureOperation:
        report.workflow?.planObservationFailureOperation || 'none',
      planObservationFailureExpressionStage:
        report.workflow?.planObservationFailureExpressionStage || 'none',
      planModeSelected: planMode.ok,
      planTurnObserved: planTurnPassed,
      planProviderReceiptBound,
      planProviderReceiptDigest: planProviderReceiptBound
        ? planProviderEvidence.providerAttemptReceiptDigest
        : '',
      planProviderReceiptTrace,
      planThreadIdHash,
      planArtifactBound: planArtifact.ok,
      planArtifactPathHash: sha256(planArtifact.relativePath),
      planArtifactContentHash: planArtifact.contentHash,
      planArtifactByteSize: planArtifact.byteSize,
      planArtifactResultItemDigest: planArtifact.resultItemDigest,
      planArtifactRevisionSequenceDigest: planArtifact.revisionSequenceDigest,
      planArtifactFileIdentityDigest: planArtifact.fileIdentityDigest,
      planArtifactExcludeSha256: planArtifact.excludeSha256,
      planArtifactCallCount: planArtifact.planCallCount,
      planArtifactResultCount: planArtifact.planResultCount,
      exactPlanArtifactRecovered,
      planTaskInspectionReadsBound,
      planTaskInspectionReadObservationDigest: report.workflow
        .planTaskInspectionReadObservationDigest,
      planTaskInspectionExpectedReadCount: planReadSet.expectedReadCount,
      planTaskInspectionReadAttemptCount: planReadSet.readAttemptCount,
      planTaskInspectionReadAttemptDigest: planReadSet.readAttemptDigest,
      planLongContextReadAbsent: planReadSet.longContextReadAbsent,
      agentModeRestored: agentMode.ok,
      sameThreadPlanAgent: firstEvidence.threadId === planEvidence.threadId,
      threadIdHash: sha256(firstEvidence.threadId),
      ordinaryWorkflowDisposition: ordinaryDisposition,
      ordinaryWorkflowFailureCode: '',
      ordinaryCandidateTurnStatus: workflowDiagnostic.turnStatus,
      ordinaryCandidateTurnIdHash: workflowDiagnostic.turnIdHash,
      ordinaryCandidateTurnErrorCode: workflowDiagnostic.turnErrorCode,
      ordinaryCandidateTurnErrorCodeHash: workflowDiagnostic.turnErrorCodeHash,
      ordinaryCandidateToolAttemptCount: workflowDiagnostic.toolAttemptCount,
      ordinaryCandidateSuccessfulToolExecutionCount:
        workflowDiagnostic.successfulToolExecutionCount,
      ordinaryCandidateFailedToolResultCount: workflowDiagnostic.failedToolResultCount,
      ordinaryCandidateUnsettledToolResultCount: workflowDiagnostic.unsettledToolResultCount,
      ordinaryCandidateToolAttemptCategoryCounts:
        workflowDiagnostic.toolAttemptCategoryCounts,
      ordinaryCandidateSuccessfulToolCategoryCounts:
        workflowDiagnostic.successfulToolCategoryCounts,
      ordinaryCandidateProviderAttemptRecordCount:
        workflowDiagnostic.providerAttemptRecordCount,
      ordinaryCandidateProviderTerminalRecordCount:
        workflowDiagnostic.providerTerminalRecordCount,
      ordinaryCandidateProviderReceiptCountersBound:
        workflowDiagnostic.providerReceiptCountersBound,
      ordinaryCandidateProviderLogicalCallCount:
        workflowDiagnostic.providerLogicalCallCount,
      ordinaryCandidateProviderAttemptCount:
        workflowDiagnostic.providerAttemptCount,
      ordinaryCandidateProviderAttemptStatusCounts:
        workflowDiagnostic.providerAttemptStatusCounts,
      ordinaryCandidateTerminalReasonClass:
        workflowDiagnostic.terminalReasonClass,
      ordinaryCandidateTerminalErrorItemCount:
        workflowDiagnostic.terminalErrorItemCount,
      ordinaryCandidateTerminalErrorItemAuthorityBound:
        workflowDiagnostic.terminalErrorItemAuthorityBound,
      ordinaryCandidateToolInventoryAvailability:
        workflowDiagnostic.toolInventoryAvailability,
      ordinaryCandidateProviderReceiptAvailability:
        workflowDiagnostic.providerReceiptAvailability,
      ordinaryCandidateTypedResultObserved:
        workflowDiagnostic.ordinaryResultObserved,
      ordinaryCandidateTypedResultReasonCode:
        workflowDiagnostic.ordinaryResultReasonCode,
      ordinaryCandidateTypedResultOrigin:
        workflowDiagnostic.ordinaryResultCandidateOrigin,
      ordinaryCandidateTypedResultProjectionClass:
        workflowDiagnostic.ordinaryResultProjectionClass,
      ordinaryCandidateTypedResultDigest:
        workflowDiagnostic.ordinaryResultDigest,
      ordinaryCandidateTypedResultTextSha256:
        workflowDiagnostic.ordinaryResultTextSha256,
      ordinaryCandidateTypedResultMarkerObserved:
        workflowDiagnostic.ordinaryResultMarkerObserved,
      ordinaryCandidateTypedResultResearchMarkerObserved:
        workflowDiagnostic.ordinaryResultResearchMarkerObserved,
      ordinaryCandidateTypedResultWritingMarkerObserved:
        workflowDiagnostic.ordinaryResultWritingMarkerObserved,
      ordinaryCandidateInventoryDigest: workflowDiagnostic.inventoryDigest,
      ordinaryCandidateFailureClassification: workflowFailureProjection.classification,
      ordinaryCandidateFailureObservationCode: workflowFailureProjection.observationCode,
      ordinaryCandidateProviderTransportSucceeded:
        workflowFailureProjection.providerTransportSucceeded,
      ordinaryCandidateUpstreamCauseConfirmed:
        workflowFailureProjection.upstreamCauseConfirmed,
      ordinaryCandidateFailureDiagnosticDigest: workflowFailureProjection.diagnosticDigest,
      ordinaryCandidateProviderFailureObserved: providerFailureDiagnostic.observed,
      ordinaryCandidateProviderFailureObservationCode:
        providerFailureDiagnostic.observationCode,
      ordinaryCandidateProviderFailureReasonCode: providerFailureDiagnostic.reasonCode,
      ordinaryCandidateProviderFailureKind: providerFailureDiagnostic.providerKind,
      ordinaryCandidateProviderFailureStatus: providerFailureDiagnostic.providerStatus,
      ordinaryCandidateProviderFailureRetryable: providerFailureDiagnostic.providerRetryable,
      ordinaryCandidateProviderFailureAuthStatus: providerFailureDiagnostic.providerAuthStatus,
      ordinaryCandidateProviderFailureDiagnosticDigest:
        providerFailureDiagnostic.diagnosticDigest,
      ordinaryCandidateToolFailureObserved: toolFailureDiagnostic.observed,
      ordinaryCandidateToolFailureObservationCode: toolFailureDiagnostic.observationCode,
      ordinaryCandidateToolFailureEvidenceCode: toolFailureDiagnostic.evidenceCode,
      ordinaryCandidateToolFailureGuardBound: toolFailureDiagnostic.toolFailureGuardBound,
      ordinaryCandidateToolFailureGuardCount: toolFailureDiagnostic.guardCount,
      ordinaryCandidateToolFailureMaxStormCount: toolFailureDiagnostic.maxStormCount,
      ordinaryCandidateToolFailureGuardKind: toolFailureDiagnostic.guardKind,
      ordinaryCandidateToolFailureToolCategory: toolFailureDiagnostic.toolCategory,
      ordinaryCandidateToolFailureToolNameHash: toolFailureDiagnostic.toolNameHash,
      ordinaryCandidateInvalidToolArgumentGuardCount:
        toolFailureDiagnostic.invalidArgumentGuardCount,
      ordinaryCandidateInvalidToolArgumentMaxStormCount:
        toolFailureDiagnostic.invalidArgumentMaxStormCount,
      ordinaryCandidateInvalidToolArgumentToolCategory:
        toolFailureDiagnostic.invalidArgumentToolCategory,
      ordinaryCandidateInvalidToolArgumentToolNameHash:
        toolFailureDiagnostic.invalidArgumentToolNameHash,
      ordinaryCandidateToolFailureTerminalReason: toolFailureDiagnostic.terminalReason,
      ordinaryCandidateToolFailureTerminalCode: toolFailureDiagnostic.terminalCode,
      ordinaryCandidateToolFailureDiagnosticDigest: toolFailureDiagnostic.diagnosticDigest,
      readObserved: groups.read,
      planObserved: groups.plan,
      todoObserved: groups.todo && firstEvidence.todosPresent && firstEvidence.todosTerminal,
      writeObserved: groups.write && sourceRepair.changed,
      realTestObserved: groups.test && sourceRepair.changed && testReceipt.passed,
      parentOwnedTestCrossCheckPassed: parentOwnedTest.passed,
      parentOwnedTestCommandDigest: parentOwnedTest.commandDigest,
      parentOwnedTestRepositoryStable: parentOwnedTest.repositoryStableBeforeAndAfter,
      isolatedGitRepositoryObserved: repositoryAcceptance.gitRepository,
      protectedRepositoryInputsBound:
        repositoryAcceptance.protectedInputsBound &&
        repositoryAcceptance.acceptanceContractBound,
      externalRepositoryContractBound: repositoryAcceptance.acceptanceContractBound,
      onlyIntendedSourceChanged: repositoryAcceptance.onlyIntendedSourceChanged,
      externalTestValidatorBound: testReceipt.externalValidatorBound,
      npmTestLifecycleBound: testReceipt.npmLifecycleBound,
      npmTestProcessLineageBound: testReceipt.npmTestProcessLineageBound,
      actualBashTestResultBound: testReceipt.actualBashToolResultBound,
      hostOwnedToolInvocationBound: testReceipt.hostOwnedToolInvocationBound,
      bashHostObservationReasonCode: safeHostObservationReasonCode(
        bashHostObservation.reasonCode
      ),
      bashHostObservationTransportStatus: safeHostObservationTransportStatus(
        bashHostObservation.transportStatus
      ),
      bashHostObservationAttemptCount: safeHostObservationAttemptCount(
        bashHostObservation.attemptCount
      ),
      bashHostObservationDigest: testReceipt.hostObservationDigest,
      bashHostAuthorityBindingDigest: testReceipt.hostAuthorityBindingDigest,
      selfReportedTestReceiptAuthoritative: testReceipt.selfReportedReceiptAuthoritative,
      selfReportedTestReceiptObserved: testReceipt.selfReportedReceiptObserved,
      selfReportedTestReceiptShapeValid: testReceipt.selfReportedReceiptShapeValid,
      subagentObserved: groups.subagent && firstEvidence.boundedSubagentPresent,
      childSubagentProviderReceiptBound,
      childSubagentProviderReceiptDigest: childSubagentProviderReceiptBound
        ? childProviderEvidence.providerAttemptReceiptDigest
        : '',
      childSubagentProviderReceiptReasonCode: childProviderEvidence.reasonCode,
      childSubagentProviderReceiptSseReasonCode: childProviderEvidence.sseReasonCode,
      childSubagentReviewReadAttemptCount,
      childSubagentReviewReadsBound,
      childSubagentReviewReadObservationDigest,
      gitCommandObserved: gitHostInvocationBound,
      gitHostObservationReasonCode: safeHostObservationReasonCode(
        gitHostObservation.reasonCode
      ),
      gitHostObservationTransportStatus: safeHostObservationTransportStatus(
        gitHostObservation.transportStatus
      ),
      gitHostObservationAttemptCount: safeHostObservationAttemptCount(
        gitHostObservation.attemptCount
      ),
      gitHostObservationDigest: gitHostInvocationBound
        ? gitHostObservation.observationDigest
        : '',
      gitHostAuthorityBindingDigest: gitHostInvocationBound
        ? gitHostObservation.authorityBindingDigest
        : '',
      skillObserved: resultTurnContract.runSkillAttemptCount === 1 &&
        resultTurnContract.runSkillExecutionCount === 1 &&
        resultTurnContract.skillCatalogBound,
      ordinaryMCPObserved,
      ordinaryMCPResultTurnAttemptCount:
        resultTurnContract.ordinaryMCPAttemptCount,
      ordinaryMCPResultTurnExecutionCount:
        resultTurnContract.ordinaryMCPExecutionCount,
      ordinaryMCPStrictEmptyInputBound:
        resultTurnContract.emptyInputBoundByStrictSchema,
      ordinaryMCPPackageCatalogBound: resultTurnContract.packageCatalogBound,
      ordinaryMCPPackageConfigBound: resultTurnContract.packageConfigBound,
      ordinaryMCPConfigSha256: scheduleConfigEvidence.configSha256,
      ordinaryMCPHelperExecutableSha256:
        scheduleConfigEvidence.helperExecutableSha256,
      ordinaryMCPArgumentsDigest: scheduleConfigEvidence.argumentsDigest,
      resultTurnTaskAttemptCount: resultTurnContract.taskAttemptCount,
      resultTurnTaskExecutionCount: resultTurnContract.taskExecutionCount,
      resultTurnSubagentDelegationAttemptCount:
        resultTurnContract.subagentDelegationAttemptCount,
      resultTurnRunSkillAttemptCount: resultTurnContract.runSkillAttemptCount,
      resultTurnRunSkillExecutionCount: resultTurnContract.runSkillExecutionCount,
      resultTurnBashAttemptCount: resultTurnContract.bashAttemptCount,
      resultTurnBashExecutionCount: resultTurnContract.bashExecutionCount,
      resultTurnMCPAttemptCount: resultTurnContract.allMCPAttemptCount,
      resultTurnMCPExecutionCount: resultTurnContract.allMCPExecutionCount,
      resultTurnFundsMCPAttemptCount: resultTurnContract.fundsMCPAttemptCount,
      resultTurnMalformedToolCallAttemptCount:
        resultTurnContract.malformedToolCallAttemptCount,
      taskProfileBound: resultTurnContract.taskProfileBound,
      taskForegroundBound: resultTurnContract.taskForegroundBound,
      taskStepLimitBound: resultTurnContract.taskStepLimitBound,
      taskTimeBudgetBound: resultTurnContract.taskTimeBudgetBound,
      researchWritingObserved,
      successfulToolResultsObserved: Object.values(groups).every(Boolean),
      todosCompleted: firstEvidence.todosPresent && firstEvidence.todosTerminal,
      exactlyOneBoundedSubagentCompleted:
        firstEvidence.boundedSubagentPresent && firstEvidence.boundedSubagentCount === 1,
      longContextContinuationObserved,
      longContextCaseWorkspaceScopeBound,
      longContextByteLength: contextFile.byteLength,
      longContextReadLineLimit: contextReadArguments.limit,
      longContextWorkflowDisposition: contextWorkflow.disposition,
      longContextCandidateTurnStatus: contextDiagnostic.turnStatus,
      longContextCandidateTurnIdHash: contextDiagnostic.turnIdHash,
      longContextCandidateTurnErrorCode: contextDiagnostic.turnErrorCode,
      longContextCandidateTurnErrorCodeHash: contextDiagnostic.turnErrorCodeHash,
      longContextPublicToolInventoryAvailability:
        contextDiagnostic.toolInventoryAvailability,
      longContextPublicToolAttemptCount: contextDiagnostic.toolAttemptCount,
      longContextPublicSuccessfulToolExecutionCount:
        contextDiagnostic.successfulToolExecutionCount,
      longContextCandidateFailedToolResultCount:
        contextDiagnostic.failedToolResultCount,
      longContextCandidateUnsettledToolResultCount:
        contextDiagnostic.unsettledToolResultCount,
      longContextCandidateProviderAttemptRecordCount:
        contextDiagnostic.providerAttemptRecordCount,
      longContextCandidateProviderTerminalRecordCount:
        contextDiagnostic.providerTerminalRecordCount,
      longContextCandidateProviderReceiptCountersBound:
        contextDiagnostic.providerReceiptCountersBound,
      longContextCandidateProviderLogicalCallCount:
        contextDiagnostic.providerLogicalCallCount,
      longContextCandidateProviderAttemptCount:
        contextDiagnostic.providerAttemptCount,
      longContextCandidateTerminalReasonClass:
        contextDiagnostic.terminalReasonClass,
      longContextCandidateTerminalErrorItemCount:
        contextDiagnostic.terminalErrorItemCount,
      longContextCandidateTerminalErrorItemAuthorityBound:
        contextDiagnostic.terminalErrorItemAuthorityBound,
      longContextCandidateInventoryDigest: contextDiagnostic.inventoryDigest,
      longContextProviderFailureObserved: contextProviderFailureDiagnostic.observed,
      longContextProviderFailureObservationCode:
        contextProviderFailureDiagnostic.observationCode,
      longContextProviderFailureReasonCode:
        contextProviderFailureDiagnostic.reasonCode,
      longContextProviderFailureKind: contextProviderFailureDiagnostic.providerKind,
      longContextProviderFailureStatus: contextProviderFailureDiagnostic.providerStatus,
      longContextProviderFailureRetryable:
        contextProviderFailureDiagnostic.providerRetryable,
      longContextProviderFailureAuthStatus:
        contextProviderFailureDiagnostic.providerAuthStatus,
      longContextProviderFailureDiagnosticDigest:
        contextProviderFailureDiagnostic.diagnosticDigest,
      longContextProviderReceiptReplayAttempted: contextReceiptReplayAttempted,
      longContextProviderReceiptReplayObserved: contextReceiptReplayObserved,
      longContextProviderReceiptReplayReasonCode: contextReceiptReplayReasonCode,
      longContextContinuationReasonCode,
      longContextContinuationFailureCode: longContextContinuationFailure,
      longContextProviderReceiptBound: contextReceiptReplayObserved,
      longContextProviderReceiptDigest: contextReceiptReplayObserved
        ? contextProviderEvidence.providerAttemptReceiptDigest
        : '',
      longContextAcceptedFinalBound: contextAcceptedFinalEvidence.ok,
      longContextAcceptedFinalDigest: contextAcceptedFinalEvidence.acceptedFinalDigest,
      longContextProviderClosureDigest:
        contextAcceptedFinalEvidence.providerClosureDigest,
      longContextHostReadBound,
      longContextHostObservationTransportStatus:
        contextHostObservation.transportStatus,
      longContextHostObservationDigest: longContextHostReadBound
        ? contextHostObservation.observationDigest
        : '',
      compactionCount: compactedEvidence.compactionCount,
      manualCompactionBaselineBound:
        report.workflow.manualCompactionBaselineBound,
      manualCompactionBaselineCount:
        report.workflow.manualCompactionBaselineCount,
      manualCompactionBaselineDigest:
        report.workflow.manualCompactionBaselineDigest,
      manualCompactionCandidateDigest:
        report.workflow.manualCompactionCandidateDigest,
      manualCompactionNewAfterBaseline:
        report.workflow.manualCompactionNewAfterBaseline,
      manualCompactionObserved: report.workflow.manualCompactionObserved,
      manualCompactionAuto: report.workflow.manualCompactionAuto,
      manualCompactionReplacedTokens: report.workflow.manualCompactionReplacedTokens,
      manualCompactionSourceDigest: report.workflow.manualCompactionSourceDigest,
      manualCompactionSourceItemIdsDigest:
        report.workflow.manualCompactionSourceItemIdsDigest,
      manualCompactionSourceAncestryBound:
        report.workflow.manualCompactionSourceAncestryBound,
      manualCompactionNonzeroBound:
        report.workflow.manualCompactionNonzeroBound,
      manualCompactionProjectionClass:
        report.workflow.manualCompactionProjectionClass,
      normalFirstQuitObserved: firstNormalQuit,
      normalFirstQuitReasonCode: firstNormalQuitReasonCode,
      firstExitCode: firstChild.exitCode,
      firstExitSignal: firstChild.signalCode,
      freshProcessRelaunchObserved: freshRelaunch,
      relaunchProviderContinuationObserved,
      relaunchProviderReceiptDigest:
        relaunchAcceptedFinalEvidence.providerClosureDigest,
      relaunchAcceptedFinalBound: relaunchAcceptedFinalEvidence.ok,
      relaunchAcceptedFinalDigest: relaunchAcceptedFinalEvidence.acceptedFinalDigest,
      relaunchProviderClosureDigest:
        relaunchAcceptedFinalEvidence.providerClosureDigest,
      exactThreadRecovered,
      exactTodosRecovered,
      exactSubagentsRecovered,
      exactCompactionsRecovered,
      exactResultRecovered,
      rendererVisibleThreadRecovered:
        exactThreadRecovered && rendererRecovery.timelineVisible === true &&
        rendererRecovery.threadVisible === true,
      rendererVisibleTodosRecovered:
        exactTodosRecovered && rendererRecovery.todoVisible === true,
      rendererVisibleSubagentRecovered:
        exactSubagentsRecovered && rendererRecovery.subagentVisible === true,
      rendererVisibleCompactionRecovered:
        exactCompactionsRecovered && rendererRecovery.compactionVisible === true,
      rendererVisibleResultRecovered:
        exactResultRecovered && rendererRecovery.resultVisible === true,
      normalFinalQuitObserved: finalNormalQuit,
      normalFinalQuitReasonCode: finalNormalQuitReasonCode,
      finalExitCode: secondChild.exitCode,
      finalExitSignal: secondChild.signalCode,
      zeroResidualProcesses: zeroResidual,
      toolNameSetDigest: firstEvidence.toolNameSetDigest,
      successfulToolExecutionDigest: firstEvidence.successfulToolExecutionDigest,
      todoStateDigest: firstEvidence.todosDigest,
      subagentStateDigest: firstEvidence.subagentsDigest,
      compactionStateDigest: compactedEvidence.compactionsDigest,
      resultDigest: ordinaryResultEvidence.textSha256,
      sourceRepairSha256: sourceRepair.sha256,
      protectedRepositoryInputsDigest: repositoryAcceptance.protectedInputsDigest,
      externalRepositoryContractSha256: repositoryAcceptance.contractSha256,
      externalTestValidatorSha256: repositoryAcceptance.validatorSha256,
      realTestReceiptSha256: testReceipt.sha256,
      realTestExecutionCount: testReceipt.count,
      realTestBashCallIdHash: testReceipt.bashCallIdHash,
      realTestBashTurnIdHash: testReceipt.bashTurnIdHash
    }
    report.redaction = {
      status: credentialsIsolated ? 'passed' : 'failed',
      secretMaterialFound:
        credentialScan.findingCount > 0 || credentialScan.reportFindingCount > 0,
      scannedFileCount: credentialScan.scannedFileCount,
      settingsFindingCount: credentialScan.settingsFindingCount,
      reportFindingCount: credentialScan.reportFindingCount,
      unsafeEntryCount: credentialScan.unsafeEntryCount,
      symlinkCount: credentialScan.symlinkCount,
      sourceSecretCount: credentialScan.sourceSecretCount,
      uniqueSecretCount: credentialScan.uniqueSecretCount,
      expectedSecretCount: credentialScan.expectedSecretCount,
      sourceBound: credentialScan.sourceBound,
      encodingCoverage: credentialScan.encodingCoverage,
      credentialRecorded: false
    }
  } catch (error) {
    report.executionBlocker = safeErrorDiagnosticCode(error)
    report.executionBlockerClassification = error?.liveBlocked === true
      ? 'external_prerequisite'
      : 'product_or_harness_failure'
  } finally {
    disposeLocalCredentialScanSource(privateCredentialSource)
    await stopTaskOwnedChild(firstChild)
    await stopTaskOwnedChild(secondChild)
    if (firstChild) {
      report.publicSeams.firstLaunch = {
        ...report.publicSeams.firstLaunch,
        startupTrace: packagedStartupTraceEvidence(firstChild)
      }
    }
    if (secondChild) {
      report.publicSeams.secondLaunch = {
        ...report.publicSeams.secondLaunch,
        startupTrace: packagedStartupTraceEvidence(secondChild)
      }
    }
    if (sandboxRoot) await stopExactTaskOwnedProcesses(sandboxRoot)
    if (isolatedLoginKeychain) isolatedLoginKeychain.dispose()
    if (sandboxRoot) {
      try {
        if (taskOwnedProcessPids(sandboxRoot).length === 0) {
          rmSync(sandboxRoot, { recursive: true, force: true })
          cleanupSucceeded = !existsSync(sandboxRoot)
        }
      } catch {
        cleanupSucceeded = false
      }
    }
    try {
      externalRepositoryPreserved = Boolean(
        repositoryInput.authority &&
        realpathSync(repositoryInput.authority.workspace) === repositoryInput.authority.workspace &&
        lstatSync(repositoryInput.authority.workspace).isDirectory()
      )
    } catch {
      externalRepositoryPreserved = false
    }
  }

  report.isolation.sandboxRemoved = cleanupSucceeded
  if (repositoryAcceptance) {
    report.repository = projectMilestoneARepositoryAcceptance(
      report.repository,
      repositoryAcceptance
    )
  }
  report.repository.preservedAfterHarnessCleanup = externalRepositoryPreserved
  report.checks.push(check(
    'sandbox-cleanup',
    cleanupSucceeded && externalRepositoryPreserved,
    'task-owned profile/runtime/credential data must be removed while the separately supplied acceptance repository remains intact for audit'
  ))
  for (const id of REQUIRED_CHECK_IDS) {
    if (!report.checks.some((item) => item.id === id)) {
      report.checks.push(check(
        id,
        false,
        report.executionBlocker || 'prerequisite failure prevented this check',
        report.executionBlockerClassification === 'external_prerequisite'
          ? 'live_blocked'
          : 'skipped'
      ))
    }
  }
  return finalizeMilestoneAReport(report)
}

async function main() {
  let report
  try {
    report = await runMilestoneA()
  } catch (error) {
    const target = { platform: process.platform, arch: process.arch, key: `${process.platform}-${process.arch}` }
    report = safeReportSkeleton({
      target,
      appPath: optionValue('--app-path', ''),
      timeoutMs: Number(optionValue('--timeout-ms', DEFAULT_TIMEOUT_MS)),
      sourceCommit: expectedPackagedSourceCommit()
    })
    report.status = 'failed'
    report.executionBlocker = safeErrorDiagnosticCode(
      error,
      'milestone_a_unhandled_failure'
    )
    report.checks = REQUIRED_CHECK_IDS.map((id) =>
      check(id, false, report.executionBlocker, 'failed')
    )
    report = finalizeMilestoneAReport(report)
  }
  if (!noWrite) {
    const output = resolve(process.cwd(), optionValue(
      '--output',
      process.env.ANALYTIX_RUNTIME_GO_PACKAGED_MILESTONE_A_EVIDENCE || DEFAULT_OUTPUT
    ))
    mkdirSync(dirname(output), { recursive: true })
    writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`, 'utf8')
    report.outputPathHash = sha256(output)
  }
  if (jsonOutput) {
    console.log(JSON.stringify(report, null, 2))
  } else {
    console.log(`${report.status.toUpperCase()} ${report.id}`)
    for (const item of report.checks.filter((candidate) => candidate.status !== 'passed')) {
      console.log(`${item.status.toUpperCase()} ${item.id}: ${item.message}`)
    }
  }
  if (!reportOnly && !report.passed) process.exitCode = 1
}

function isDirectExecution() {
  if (!process.argv[1]) return false
  try {
    return realpathSync(resolve(process.argv[1])) === realpathSync(fileURLToPath(import.meta.url))
  } catch {
    return false
  }
}

if (isDirectExecution()) {
  if (rawArgs.includes('--help') || rawArgs.includes('-h')) {
    process.stdout.write(HELP_TEXT)
  } else {
    await main()
  }
}
