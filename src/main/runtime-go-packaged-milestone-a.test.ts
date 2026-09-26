import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  linkSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { join, relative } from 'node:path'
import { pathToFileURL } from 'node:url'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { AcceptedFinalDeliveryBatchV2Schema } from '../../packages/runtime/src/contracts/events.js'
import { ThreadDetailResponseV1Schema } from '../../packages/runtime/src/contracts/thread-detail.js'
import type { AnalytixRuntimeApi } from '../shared/analytix-api'
import { buildPlanRelativePath, planFeatureNameFromRequest } from '../shared/gui-plan'
import { isStrictPublicRuntimeSseIpcPayload } from '../shared/public-runtime-sse'
import {
  findKeyboardShortcutCommand,
  keyboardEventToShortcut,
  resolveKeyboardShortcutBindings
} from '../shared/keyboard-shortcuts'

const FORMAL_RUNTIME_REQUEST_METHOD = 'runtimeRequest' satisfies keyof AnalytixRuntimeApi

const milestoneScriptPath = join(
  process.cwd(),
  'scripts/runtime-go-packaged-milestone-a.mjs'
)
const requireFromTest = createRequire(import.meta.url)
const fixtureCacheRoot = realpathSync(tmpdir())
const fixtureModuleRoot = mkdtempSync(join(fixtureCacheRoot, 'milestone-a-module-fixture-'))
const fixtureCacheHelper = join(fixtureModuleRoot, 'synthetic-cache-preflight.zsh')
const fixtureModules = new Map<boolean, Promise<Record<string, any>>>()

beforeAll(() => {
  vi.stubEnv('TMPDIR', fixtureCacheRoot)
  // Parser/parent-process fixtures are not Owner-volume qualification. Exercise
  // a real sourced preflight against the parent's disposable storage instead.
  // The production helper and its physical-volume requirements stay unchanged.
  writeFileSync(fixtureCacheHelper, [
    '# Synthetic test-only storage preflight; not formal cache acceptance.',
    '[[ -d "$HOME" && -w "$HOME" && -d "$TMPDIR" && -w "$TMPDIR" ]] || return 1',
    '[[ -d "$NPM_CONFIG_CACHE" && -w "$NPM_CONFIG_CACHE" ]] || return 1',
    '[[ "$TMPDIR" == "${HOME:h}/tmp" && "$NPM_CONFIG_CACHE" == "${HOME:h}/npm-cache" ]] || return 1',
    ''
  ].join('\n'), { mode: 0o600 })
})
afterAll(() => {
  vi.unstubAllEnvs()
  rmSync(fixtureModuleRoot, { recursive: true, force: true })
})

function source(path: string): string {
  return readFileSync(join(process.cwd(), path), 'utf8')
}

function milestoneCliEnvironment(): NodeJS.ProcessEnv {
  const env = { ...process.env }
  delete env.ANALYTIX_RUNTIME_GO_MILESTONE_A_CREDENTIAL_ENTRY_METHOD
  return env
}

function sha256(value: string | Buffer): string {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value: any): string {
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(',')}]`
  if (value && typeof value === 'object') {
    return `{${Object.entries(value)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSON(item)}`)
      .join(',')}}`
  }
  return JSON.stringify(value)
}

function workflowDiagnosticCategories(overrides: Record<string, number> = {}): Record<string, number> {
  return Object.fromEntries([
    'read', 'plan', 'todo', 'write', 'bash', 'subagent', 'skill',
    'ordinaryMcp', 'fundsMcp', 'other'
  ].map((key) => [key, overrides[key] || 0]))
}

function formal21TerminalWorkflowDiagnostic(): Record<string, any> {
  return {
    observed: true,
    turnIdHash: '1'.repeat(64),
    turnStatus: 'failed',
    turnErrorCode: 'provider_error',
    turnErrorCodeHash: sha256('provider_error'),
    toolAttemptCount: 6,
    successfulToolExecutionCount: 6,
    failedToolResultCount: 0,
    unsettledToolResultCount: 0,
    toolAttemptCategoryCounts: workflowDiagnosticCategories({ read: 3, todo: 2, write: 1 }),
    successfulToolCategoryCounts: workflowDiagnosticCategories({ read: 3, todo: 2, write: 1 }),
    providerAttemptRecordCount: 1,
    providerTerminalRecordCount: 1,
    providerReceiptCountersBound: true,
    providerLogicalCallCount: 5,
    providerAttemptCount: 5,
    providerAttemptStatusCounts: {
      succeeded: 5,
      failed: 0,
      cancelled: 0,
      timedOut: 0,
      streamAborted: 0
    },
    terminalReasonClass: 'none',
    terminalErrorItemCount: 1,
    terminalErrorItemAuthorityBound: true,
    toolInventoryAvailability: 'public_projection_available',
    providerReceiptAvailability: 'general_terminal_sse_replay',
    ordinaryResultObserved: false,
    ordinaryResultReasonCode: 'ordinary_result_absent',
    ordinaryResultCandidateOrigin: 'not_observed',
    ordinaryResultProjectionClass: 'not_observed',
    ordinaryResultDigest: '',
    ordinaryResultTextSha256: '',
    ordinaryResultMarkerObserved: false,
    ordinaryResultResearchMarkerObserved: false,
    ordinaryResultWritingMarkerObserved: false,
    inventoryDigest: '3'.repeat(64)
  }
}

function providerFailureDiagnosticMissing(): Record<string, any> {
  return {
    observed: false,
    observationCode: 'provider_failure_diagnostic_missing',
    reasonCode: 'none',
    providerKind: 'not_observed',
    providerStatus: null,
    providerRetryable: null,
    providerAuthStatus: 'not_observed',
    diagnosticDigest: ''
  }
}

function toolFailureDiagnosticNotObserved(): Record<string, any> {
  return {
    observed: false,
    observationCode: 'tool_failure_not_observed',
    evidenceCode: 'tool_failure_evidence_not_observed',
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

function runtimePublicSeamObservation(mcpServers: Record<string, unknown>[]): Record<string, any> {
  return {
    health: { service: 'analytix' },
    runtimeInfo: {
      schemaVersion: 2,
      status: 'ready',
      listenerScope: 'loopback',
      port: 43210,
      insecure: false,
      storage: { configured: true, available: true },
      executionPolicy: {
        approvalPolicy: 'auto',
        sandboxMode: 'danger-full-access'
      }
    },
    runtimeTools: {
      schemaVersion: 2,
      providerCount: 1,
      toolContracts: { count: 4, catalogHash: 'a'.repeat(64) },
      mcpServers,
      commands: []
    },
    runtimeThreadListProbeOk: true,
    runtimeThreadListProbeStatus: 200,
    runtimeSkills: null
  }
}

function protectedFundsUnavailableObservation(): Record<string, any> {
  const threadId = 'thread_general_additive_1'
  const turnId = 'turn_protected_funds_denied_1'
  const fixture = acceptedFinalOrdinaryContinuationObservation()
  const thread = fixture.observation.thread
  const turn = thread.turns[0]
  const assistant = turn.items[0]
  const delivery = thread.acceptedFinalDelivery
  const digest = 'd'.repeat(64)
  thread.id = threadId
  thread.latestTurnId = turnId
  turn.id = turnId
  turn.threadId = threadId
  assistant.threadId = threadId
  assistant.turnId = turnId
  assistant.text = '案件事实源当前不可用；本轮未发布任何案件事实。'
  Object.assign(turn.acceptedFinalView, {
    acceptedFinalDigest: digest,
    variant: 'SourceUnavailableAnswer',
    terminalReason: 'source_unavailable',
    blockerCode: 'current_case_source_unavailable',
    coverageStatus: 'unavailable',
    checkedScopeDigest: '',
    missingScopeCount: 0,
    claimCount: 0,
    claimTypes: [],
    noHitWording: ''
  })
  delivery.threadId = threadId
  delivery.turnId = turnId
  delivery.publicationCommitId = digest
  delivery.publicationAuthority.threadId = threadId
  delivery.publicationAuthority.turnId = turnId
  delivery.publicationAuthority.publicationCommitId = digest
  delivery.events.forEach((event: Record<string, any>) => {
    event.threadId = threadId
    event.turnId = turnId
    event.acceptedFinalDigest = digest
    event.publicationCommitId = digest
  })
  delivery.events[0].item.threadId = threadId
  delivery.events[0].item.turnId = turnId
  delivery.events[0].item.text = assistant.text
  delivery.events[2].terminalReason = 'source_unavailable'
  thread.acceptedFinalDeliveries = [delivery]
  return { thread }
}

function acceptedFinalOrdinaryContinuationObservation(): Record<string, any> {
  const threadId = 'thread-case-bound-ordinary-continuation'
  const turnId = 'turn-case-bound-ordinary-continuation'
  const providerId = 'analytix-hub'
  const model = 'managed-model'
  const timestamp = '2026-08-02T00:00:00Z'
  const markers = [
    'MILESTONE_A_CONTEXT_FIRST_TEST',
    'MILESTONE_A_CONTEXT_OK_TEST',
    'MILESTONE_A_CONTEXT_LAST_TEST'
  ]
  const text = `Continuation completed: ${markers.join(' ')}`
  const acceptedFinalDigest = 'a'.repeat(64)
  const acceptedFinalView = {
    schemaVersion: 3,
    acceptedFinalDigest,
    publicationState: 'accepted',
    variant: 'NeedsEvidenceAnswer',
    terminalReason: 'success',
    blockerCode: 'evidence_required',
    coverageStatus: 'unverified',
    checkedScopeDigest: '',
    missingScopeCount: 1,
    claimCount: 0,
    claimTypes: [],
    receiptMetadata: {
      projection: 'masked_metadata_only',
      count: 0,
      setDigest: 'e'.repeat(64),
      citations: []
    },
    noHitWording: '',
    acceptedAt: timestamp
  }
  const assistant = {
    id: 'item-case-bound-ordinary-continuation',
    turnId,
    threadId,
    role: 'assistant',
    status: 'completed',
    createdAt: timestamp,
    finishedAt: timestamp,
    kind: 'assistant_text',
    text,
    acceptedFinalView
  }
  const firstSeq = 51
  const common = {
    timestamp,
    threadId,
    turnId,
    acceptedFinalDigest,
    publicationCommitId: acceptedFinalDigest
  }
  const events = [{
    ...common,
    kind: 'item_completed',
    seq: firstSeq,
    itemId: assistant.id,
    publicationSlot: 'assistant-final',
    publicationEventId: '2'.repeat(64),
    publicationPayloadDigest: '3'.repeat(64),
    item: assistant
  }, {
    ...common,
    kind: 'usage',
    seq: firstSeq + 1,
    publicationSlot: 'usage',
    model,
    usageFinalStatus: 'completed',
    usage: {
      promptTokens: 80,
      completionTokens: 20,
      reasoningTokens: 0,
      totalTokens: 100,
      cachedTokens: 0,
      cacheHitTokens: 0,
      cacheMissTokens: 80,
      cacheHitRate: 0,
      cacheableTokenHitRate: 0,
      totalInputTokenHitRate: 0,
      cacheMissReasons: [],
      cacheSuggestions: [],
      priceConfigured: false,
      costUsd: 0,
      costCny: 0,
      cacheSavingsUsd: 0,
      cacheSavingsCny: 0,
      tokenEconomySavingsTokens: 0,
      turns: 1
    },
    cacheDiagnostics: {
      providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
      providerAttemptTelemetryValid: true,
      providerLogicalCallCount: 2,
      providerAttemptCount: 3,
      providerAttemptStatuses: {
        succeeded: 2,
        failed: 1,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      },
      providerAttemptInputTokens: {
        complete: true,
        knownObservationCount: 3,
        observationCount: 3,
        value: 80
      },
      providerAttemptOutputTokens: {
        complete: true,
        knownObservationCount: 3,
        observationCount: 3,
        value: 20
      },
      providerAttemptCacheHitTokens: {
        complete: true,
        knownObservationCount: 3,
        observationCount: 3,
        value: 0
      },
      providerAttemptCacheMissTokens: {
        complete: true,
        knownObservationCount: 3,
        observationCount: 3,
        value: 80
      },
      providerAttemptCacheRate: {
        known: true,
        numerator: 0,
        denominator: 80
      }
    },
    publicationEventId: '4'.repeat(64),
    publicationPayloadDigest: '5'.repeat(64)
  }, {
    ...common,
    kind: 'turn_completed',
    seq: firstSeq + 2,
    publicationSlot: 'terminal',
    publicationEventId: '6'.repeat(64),
    publicationPayloadDigest: '7'.repeat(64),
    status: 'completed',
    terminalReason: 'success'
  }]
  const lastSeq = firstSeq + events.length - 1
  const eventManifestDigest = 'c'.repeat(64)
  const batchId = 'd'.repeat(64)
  const acceptedFinalDelivery = {
    schemaVersion: 2,
    purpose: 'analytix.accepted-final-delivery-batch/v2',
    kind: 'accepted_final_batch',
    batchId,
    threadId,
    turnId,
    seq: lastSeq,
    firstSeq,
    lastSeq,
    timestamp,
    publicationCommitId: acceptedFinalDigest,
    eventManifestDigest,
    publicationAuthority: {
      schemaVersion: 'accepted-final-delivery-seal.v1',
      purpose: 'analytix.accepted-final-delivery-seal/v1',
      sealId: 'e'.repeat(64),
      threadId,
      turnId,
      publicationCommitId: acceptedFinalDigest,
      acceptedFinalDispositionDigest: 'f'.repeat(64),
      terminalDispositionId: '1'.repeat(64),
      eventManifestDigest,
      sequencedEventsDigest: '2'.repeat(64),
      batchId,
      firstSeq,
      lastSeq,
      timestamp,
      authorityAlgorithm: 'Ed25519',
      authorityKeyId: '3'.repeat(64),
      authorityPublicKey: 'A'.repeat(43),
      authoritySignature: 'B'.repeat(86)
    },
    events
  }
  return {
    observation: {
      thread: {
        id: threadId,
        title: 'Case-bound ordinary continuation',
        providerId,
        model,
        mode: 'agent',
        status: 'idle',
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write',
        relation: 'primary',
        createdAt: timestamp,
        updatedAt: timestamp,
        historyAuthority: 'case_boundary_only_v1',
        latestSeq: lastSeq,
        turns: [{
          id: turnId,
          threadId,
          status: 'completed',
          model,
          reasoningEffort: 'high',
          createdAt: timestamp,
          finishedAt: timestamp,
          items: [assistant],
          acceptedFinalView
        }],
        turnCount: 1,
        latestTurnId: turnId,
        pendingApprovalIds: [],
        pendingUserInputIds: [],
        messageCount: 1,
        acceptedFinalDelivery,
        acceptedFinalDeliveries: [acceptedFinalDelivery]
      }
    },
    provider: { ok: true, id: providerId, model },
    markers,
    threadId,
    turnId,
    acceptedFinalDigest,
    eventManifestDigest
  }
}

function validCompactionThread(): Record<string, any> {
  const item: Record<string, any> = {
    id: 'compaction_thread_1_1',
    turnId: 'turn_thread_1_compaction_1',
    threadId: 'thread_1',
    role: 'system',
    status: 'completed',
    createdAt: '2026-07-29T01:02:03.000Z',
    finishedAt: '2026-07-29T01:02:03.000Z',
    kind: 'compaction',
    summary: 'Prior conversation compacted. Assistant prose, tool payloads, case facts, and private reasoning were excluded.',
    replacedTokens: 2048,
    auto: false,
    pinnedConstraints: ['user: preserve recent turns'],
    sourceDigest: 'b'.repeat(64),
    digestMarker: `sha256:${'b'.repeat(12)}`,
    sourceItemIds: ['item_user_1', 'item_result_1'],
    schemaVersion: 3,
    reasoningExcluded: true,
    assistantProseExcluded: true,
    toolPayloadsExcluded: true,
    caseFactsExcluded: true,
    providerHistoryProjectionVersion: 1
  }
  item.reasoningExclusionProof = `sha256:${sha256(canonicalJSON(item))}`
  return {
    id: item.threadId,
    turns: [{
      id: item.turnId,
      threadId: item.threadId,
      status: 'completed',
      items: [item]
    }]
  }
}

function validCaseCompactionThread(): Record<string, any> {
  const thread = validCompactionThread()
  const item = thread.turns[0].items[0]
  item.summary = 'Case-bound history compacted. No case facts, assistant prose, tool output, evidence authority, or prior compaction prose were carried into the new context epoch.'
  delete item.replacedTokens
  item.sourceItemIds = []
  item.caseHistoryProjectionVersion = 2
  delete item.providerHistoryProjectionVersion
  delete item.reasoningExclusionProof
  item.reasoningExclusionProof = `sha256:${sha256(canonicalJSON(item))}`
  return thread
}

function refreshCompactionProof(item: Record<string, any>): void {
  const proofRecord = { ...item }
  delete proofRecord.reasoningExclusionProof
  item.reasoningExclusionProof = `sha256:${sha256(canonicalJSON(proofRecord))}`
}

async function milestoneModule(): Promise<Record<string, any>> {
  return fixtureMilestoneModule(false)
}

async function instrumentedMilestoneModule(): Promise<Record<string, any>> {
  return fixtureMilestoneModule(true)
}

function fixtureMilestoneModule(exposeInternals: boolean): Promise<Record<string, any>> {
  const cached = fixtureModules.get(exposeInternals)
  if (cached) return cached
  const instrumentedPath = join(fixtureModuleRoot, `milestone-a-${exposeInternals}.mjs`)
  const originalModuleUrl = pathToFileURL(milestoneScriptPath).href
  const publicationAuthorityUrl = pathToFileURL(join(
    process.cwd(),
    'scripts/lib/packaged-release-publication-authority.mjs'
  )).href
  const originalSource = readFileSync(milestoneScriptPath, 'utf8')
  // Existing source-level test instrumentation supplies only the synthetic
  // cache mount, child preflight and source-relative module bindings. Do not change production
  // cache policy, parser validation, freshness checks or acceptance classes.
  const cacheDeclaration = "const CACHE_MOUNT = '/Volumes/AnalytixCache'"
  if (!originalSource.includes(cacheDeclaration)) throw new Error('milestone_a_cache_fixture_binding_missing')
  const helperPathExpression = 'repositorySourcePath(\n    CACHE_HELPER_SOURCE_RELATIVE_PATH\n  )'
  if (originalSource.split(helperPathExpression).length !== 2) throw new Error('milestone_a_helper_fixture_binding_missing')
  let instrumentedSource = originalSource
    .replace(cacheDeclaration, `const CACHE_MOUNT = ${JSON.stringify(fixtureCacheRoot)}`)
    .replace(helperPathExpression, JSON.stringify(fixtureCacheHelper))
    .replaceAll('import.meta.url', JSON.stringify(originalModuleUrl))
    .replace("'./lib/local-provider-acceptance.mjs'", JSON.stringify(pathToFileURL(join(
      process.cwd(), 'scripts/lib/local-provider-acceptance.mjs'
    )).href))
    .replace("'./lib/local-provider-credential-scan.mjs'", JSON.stringify(pathToFileURL(join(
      process.cwd(), 'scripts/lib/local-provider-credential-scan.mjs'
    )).href))
    .replace("'./development-keychain.mjs'", JSON.stringify(pathToFileURL(join(
      process.cwd(), 'scripts/development-keychain.mjs'
    )).href))
    .replace(
      "'./lib/packaged-release-publication-authority.mjs'",
      JSON.stringify(publicationAuthorityUrl)
    )
  if (exposeInternals) instrumentedSource = instrumentedSource.replace(
      'async function observeHostToolExecution(',
      'export async function observeHostToolExecution('
    )
    .replace(
      'function privateHostToolExecutionBindingMatches(',
      'export function privateHostToolExecutionBindingMatches('
    )
  if (instrumentedSource === originalSource || exposeInternals && (
    !instrumentedSource.includes('export async function observeHostToolExecution(') ||
    !instrumentedSource.includes('export function privateHostToolExecutionBindingMatches('))) {
    throw new Error('milestone_a_test_instrumentation_failed')
  }
  writeFileSync(instrumentedPath, instrumentedSource, 'utf8')
  const loaded = import(pathToFileURL(instrumentedPath).href)
  fixtureModules.set(exposeInternals, loaded)
  return loaded
}

async function packagedAuthorityInternals(): Promise<Record<string, any>> {
  return requireFromTest(join(process.cwd(), 'scripts/after-pack.cjs'))._internals
}

async function afterExtractInternals(): Promise<Record<string, any>> {
  return requireFromTest(join(process.cwd(), 'scripts/after-extract.cjs'))._internals
}

function packageLifecycleContext(
  root: string,
  arch: 'arm64' | 'x64' = 'arm64',
  config: Record<string, unknown> = {
    afterExtract: './scripts/after-extract.cjs',
    afterPack: './scripts/after-pack.cjs',
    asar: true
  }
): Record<string, any> {
  return {
    appOutDir: join(root, 'package-output'),
    outDir: join(root, 'dist'),
    electronPlatformName: 'darwin',
    arch,
    targets: [{ name: 'dir' }],
    packager: {
      appInfo: { productFilename: 'analytix' },
      config,
      packagerOptions: {}
    }
  }
}

function runGit(cwd: string, args: string[]): void {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8', stdio: 'pipe' })
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr}`)
  }
}

function gitOutput(cwd: string, args: string[]): string {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8', stdio: 'pipe' })
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.stderr}`)
  }
  return result.stdout.trim()
}

function externalRepositoryAcceptanceFixture(
  root: string,
  packageName = 'analytix-external-acceptance-repository'
): {
  workspace: string
  contractPath: string
  provenancePath: string
  ownerRoot: string
  repairedSource: string
  contract: Record<string, any>
  provenance: Record<string, any>
} {
  mkdirSync(root, { recursive: true })
  chmodSync(root, 0o700)
  const workspace = join(root, 'preexisting-real-repository')
  mkdirSync(join(workspace, 'src'), { recursive: true })
  mkdirSync(join(workspace, 'test'), { recursive: true })
  mkdirSync(join(workspace, 'docs'), { recursive: true })
  writeFileSync(join(workspace, 'README.md'), '# Multiplication library\n', 'utf8')
  runGit(workspace, ['-c', 'init.defaultBranch=main', 'init', '--quiet', '.'])
  const originUrl = 'https://example.com/analytix/external-acceptance-repository.git'
  runGit(workspace, ['remote', 'add', 'origin', originUrl])
  runGit(workspace, ['add', 'README.md'])
  runGit(workspace, [
    '-c', 'user.name=External Repository Author',
    '-c', 'user.email=external-repository@analytix.invalid',
    '-c', 'commit.gpgsign=false',
    'commit', '--quiet', '-m', 'Document the library'
  ])

  const brokenSource = 'export function multiply(left, right) {\n  return left + right\n}\n'
  const repairedSource = 'export function multiply(left, right) {\n  return left * right\n}\n'
  writeFileSync(join(workspace, 'src', 'math.mjs'), brokenSource, 'utf8')
  writeFileSync(
    join(workspace, 'test', 'math.test.mjs'),
    "import test from 'node:test'\n" +
      "import assert from 'node:assert/strict'\n" +
      "import { multiply } from '../src/math.mjs'\n\n" +
      "test('multiplies signed integers', () => {\n" +
      '  assert.equal(multiply(3, 4), 12)\n' +
      '  assert.equal(multiply(-2, 3), -6)\n' +
      '})\n',
    'utf8'
  )
  writeFileSync(join(workspace, 'package.json'), `${JSON.stringify({
    name: packageName,
    private: true,
    type: 'module'
  }, null, 2)}\n`, 'utf8')
  // Formal repositories bind anchors that already exist in real source files;
  // they are not required to carry synthetic harness marker syntax.
  const firstMarker = 'from __future__ import annotations'
  const completionMarker = 'def canonical_https_url(value: str) -> str:'
  const lastMarker = 'return tuple(dict(row) for row in rows)'
  const contextLines = Array.from({ length: 2101 }, (_, index) => {
    const marker = index === 0
      ? firstMarker
      : index === 1050
        ? completionMarker
        : index === 2100
          ? lastMarker
          : `EXTERNAL_REPOSITORY_CONTEXT_ROW_${String(index).padStart(4, '0')}`
    return `${marker} preserve the exact repository task, test evidence, Todo, subagent, compaction, restart, and recovered result.\n`
  })
  writeFileSync(join(workspace, 'docs', 'acceptance-context.txt'), contextLines.join(''), 'utf8')
  runGit(workspace, ['add', '--all'])
  runGit(workspace, [
    '-c', 'user.name=External Repository Author',
    '-c', 'user.email=external-repository@analytix.invalid',
    '-c', 'commit.gpgsign=false',
    'commit', '--quiet', '-m', 'Add multiplication implementation and tests'
  ])
  const contract = {
    contract: 'analytix.milestone-a.real-repository-contract.v1',
    repository: {
      kind: 'preexisting-isolated-real-code-repository',
      baselineCommit: gitOutput(workspace, ['rev-parse', 'HEAD']),
      baselineTree: gitOutput(workspace, ['rev-parse', 'HEAD^{tree}']),
      minimumCommitCount: 2,
      originUrlSha256: sha256(originUrl)
    },
    task: {
      request: 'Correct the multiplication implementation while preserving the existing public API.',
      inspectPaths: [
        'README.md',
        'package.json',
        'src/math.mjs',
        'test/math.test.mjs',
        'docs/acceptance-context.txt'
      ],
      expectedChangedFiles: [{
        path: 'src/math.mjs',
        sha256: sha256(repairedSource)
      }]
    },
    test: {
      executable: 'node',
      arguments: ['--test', 'test/math.test.mjs'],
      timeoutMs: 60_000
    },
    longContext: {
      path: 'docs/acceptance-context.txt',
      sha256: sha256(contextLines.join('')),
      minimumBytes: 64 * 1024,
      firstMarker,
      completionMarker,
      lastMarker
    }
  }
  const contractPath = join(root, 'external-repository-contract.json')
  writeFileSync(contractPath, `${JSON.stringify(contract, null, 2)}\n`, 'utf8')
  const provenance = {
    contract: 'analytix.milestone-a.external-repository-provenance.v1',
    admissionKind: 'independent-pre-admission',
    originUrl,
    baselineCommit: contract.repository.baselineCommit,
    baselineTree: contract.repository.baselineTree
  }
  const provenancePath = join(root, 'external-repository-provenance.json')
  writeFileSync(provenancePath, `${JSON.stringify(provenance, null, 2)}\n`, 'utf8')
  return {
    workspace,
    contractPath,
    provenancePath,
    ownerRoot: root,
    repairedSource,
    contract,
    provenance
  }
}

function resultItemId(turnId: string, callId: string): string {
  const part = (value: string): Buffer => {
    const body = Buffer.from(value, 'utf8')
    const length = Buffer.alloc(8)
    length.writeBigUInt64BE(BigInt(body.length))
    return Buffer.concat([length, body])
  }
  return `item_result_${sha256(Buffer.concat([
    Buffer.from('analytix.tool-result-item/id/v1\0', 'utf8'),
    part(turnId),
    part(callId)
  ]))}`
}

function materializeClosedPlanArtifact(
  authority: Record<string, any>,
  markdown = '# Milestone A plan\n\nVerify the exact repository repair.\n'
): { thread: Record<string, any>; artifactPath: string; turnId: string } {
  const threadId = 'thread-milestone-a-plan'
  const turnId = 'turn-milestone-a-plan'
  const callId = `call_host_${'a'.repeat(64)}`
  const artifactPath = join(authority.workspace, ...authority.planArtifactRelativePath.split('/'))
  mkdirSync(join(authority.workspace, '.analytixsdd', 'plan'), { recursive: true })
  writeFileSync(artifactPath, markdown, { encoding: 'utf8', mode: 0o644 })
  chmodSync(artifactPath, 0o644)
  const output = {
    schemaVersion: 1,
    projectionKind: 'plan_status',
    disclosure: 'metadata_only',
    messageKey: 'plan_updated',
    status: 'completed',
    code: 'plan_updated',
    privatePayloadWithheld: true,
    factAnswerAllowed: false,
    evidenceAuthority: false,
    plan: {
      planId: `${authority.workspace}:${authority.planArtifactRelativePath.toLowerCase()}`,
      relativePath: authority.planArtifactRelativePath,
      operation: 'draft',
      contentHash: sha256(markdown),
      byteSize: Buffer.byteLength(markdown),
      savedAt: '2026-08-01T12:00:00.123456789Z'
    }
  }
  return {
    artifactPath,
    turnId,
    thread: {
      id: threadId,
      turns: [{
        id: turnId,
        threadId,
        status: 'completed',
        items: [{
          id: `item_call_${'b'.repeat(64)}`,
          threadId,
          turnId,
          kind: 'tool_call',
          role: 'tool',
          status: 'completed',
          toolName: 'create_plan',
          toolKind: 'file_change',
          callId
        }, {
          id: resultItemId(turnId, callId),
          threadId,
          turnId,
          kind: 'tool_result',
          role: 'tool',
          status: 'completed',
          toolName: 'create_plan',
          toolKind: 'file_change',
          callId,
          isError: false,
          output
        }]
      }]
    }
  }
}

const currentSnapshot = {
  ok: true,
  blocked: false,
  blocker: '',
  snapshot: { snapshotDigest: 'a'.repeat(64), state: 'dirty' },
  digest: 'a'.repeat(64),
  classification: 'dirty'
}

function successfulHostToolObservationRaw(
  threadId: string,
  turnId: string,
  toolName: 'bash' | 'read'
): Record<string, unknown> {
  const digest = (label: string): string => sha256(`${toolName}:${label}`)
  return {
    status: 200,
    observation: {
      schemaVersion: 1,
      disclosure: 'metadata_only',
      privatePayloadWithheld: true,
      threadId,
      turnId,
      toolName,
      status: 'completed',
      workId: digest('work'),
      receiptId: digest('receipt'),
      dispositionId: digest('disposition'),
      executionGrantId: digest('grant'),
      resultItemId: `item_result_${digest('result-item')}`,
      resultItemDigest: digest('result')
    }
  }
}

function toolFailureGuardFixture({
  seq,
  threadId,
  turnId,
  toolName,
  guardKind = 'tool_failure',
  stormCount
}: {
  seq: number
  threadId: string
  turnId: string
  toolName: string
  guardKind?: 'tool_failure' | 'invalid_tool_arguments' | string
  stormCount: number
}): Record<string, unknown> {
  return {
    kind: 'pipeline_stage',
    seq,
    timestamp: '2026-08-04T00:00:00Z',
    threadId,
    turnId,
    stage: 'loop_guard',
    label: 'Loop guard nudged the model',
    details: {
      visibleRecovery: true,
      toolName,
      guardKind,
      stormCount
    }
  }
}

function toolFailureTerminalBatchFixture({
  seq,
  threadId,
  turnId,
  terminalCode = 'tool_failure_storm'
}: {
  seq: number
  threadId: string
  turnId: string
  terminalCode?: 'tool_failure_storm' | 'tool_invalid_arguments_storm'
}): Record<string, unknown> {
  return {
    kind: 'general_terminal_batch',
    seq,
    timestamp: '2026-08-04T00:00:01Z',
    threadId,
    turnId,
    schemaVersion: 1,
    purpose: 'analytix.general-terminal-delivery-batch/v1',
    batchDigest: '1'.repeat(64),
    firstSeq: seq - 2,
    lastSeq: seq,
    generalTerminalCommitId: '2'.repeat(64),
    generalTerminalAuthorityKind: 'general_terminal_cas',
    generalTerminalAuthorityDigest: '3'.repeat(64),
    eventManifestDigest: '4'.repeat(64),
    projectedEventsDigest: '5'.repeat(64),
    transportAuthority: 'host_batch_digest_v1',
    evidenceAuthority: false,
    citationAuthority: false,
    factAnswerAllowed: false,
    events: [{
      kind: 'item_completed', seq: seq - 2, threadId, turnId,
      item: { message: 'PRIVATE_TOOL_ERROR_MUST_NOT_PROJECT' }
    }, {
      kind: 'usage', seq: seq - 1, threadId, turnId
    }, {
      kind: 'turn_failed', seq, threadId, turnId,
      status: 'failed', terminalReason: 'tool_failure', code: terminalCode,
      message: 'PRIVATE_TERMINAL_MESSAGE_MUST_NOT_PROJECT'
    }],
    eventManifest: [0, 1, 2].map((index) => ({
      slot: ['terminal-item', 'usage', 'terminal'][index],
      eventId: String(6 + index).repeat(64),
      payloadDigest: String(9 - index).repeat(64)
    }))
  }
}

async function observeToolFailureFixture(
  observer: (...args: any[]) => Promise<Record<string, any>>,
  {
    threadId,
    turnId,
    baselineSeq,
    highestSeq,
    guardEvents,
    terminalCode = 'tool_failure_storm'
  }: {
    threadId: string
    turnId: string
    baselineSeq: number
    highestSeq: number
    guardEvents: Record<string, unknown>[]
    terminalCode?: 'tool_failure_storm' | 'tool_invalid_arguments_storm'
  }
): Promise<{ raw: Record<string, any>; cleanup: ReturnType<typeof vi.fn>[] }> {
  const terminalSeq = highestSeq - 1
  const callbacks: Record<string, (payload: any) => void> = {}
  const cleanup = [vi.fn(), vi.fn(), vi.fn()]
  const api = {
    onSseEvent: (handler: (payload: any) => void) => {
      callbacks.event = handler
      return cleanup[0]
    },
    onSseEnd: (handler: (payload: any) => void) => {
      callbacks.end = handler
      return cleanup[1]
    },
    onSseError: (handler: (payload: any) => void) => {
      callbacks.error = handler
      return cleanup[2]
    },
    startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
      callbacks.event({ streamId, events: guardEvents })
      callbacks.event({
        streamId,
        events: [toolFailureTerminalBatchFixture({
          seq: terminalSeq, threadId, turnId, terminalCode
        })]
      })
      return { streamId }
    }),
    ackSseEvent: vi.fn(async (streamId: string, seq: number) => {
      if (seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
      return true
    }),
    stopSse: vi.fn(async () => true)
  }
  return {
    raw: await observer(api, threadId, baselineSeq, turnId, highestSeq, 100),
    cleanup
  }
}

const taskOwnedSandboxes: string[] = []

function taskOwnedSandbox(): string {
  const trustedTmpdir = realpathSync(tmpdir())
  const path = mkdtempSync(join(trustedTmpdir, 'milestone-a-integrity-test-'))
  taskOwnedSandboxes.push(path)
  return path
}

afterEach(() => {
  while (taskOwnedSandboxes.length > 0) {
    const path = taskOwnedSandboxes.pop()
    if (path) rmSync(path, { recursive: true, force: true })
  }
})

describe('packaged general Agent Milestone A public-seam harness', () => {
  it('retains only allowlisted packaged startup checkpoints without raw process output', async () => {
    const { createPackagedStartupTraceRecorder } = await milestoneModule()
    const recorder = createPackagedStartupTraceRecorder()

    recorder.accept(
      'stdout',
      Buffer.from('untrusted output /Users/private credential=do-not-retain\n', 'utf8')
    )
    recorder.accept(
      'stderr',
      Buffer.from('[analytix] [startup] [+    17ms] settings load:', 'utf8')
    )
    recorder.accept(
      'stderr',
      Buffer.from('start — detail: {"path":"/Users/private"}\n', 'utf8')
    )
    recorder.accept(
      'stdout',
      Buffer.from('[analytix] [startup] [+    29ms] not an allowlisted stage\n', 'utf8')
    )
    recorder.accept(
      'stdout',
      Buffer.from('[analytix] [startup] [+    30ms] toString\n', 'utf8')
    )
    recorder.accept(
      'stdout',
      Buffer.from(`private-fragment-${'x'.repeat(4096)}`, 'utf8')
    )
    recorder.accept(
      'stdout',
      Buffer.from('[analytix] [startup] [+    41ms] desktop private history migration:start\n', 'utf8')
    )
    recorder.finish('stdout')
    recorder.finish('stderr')

    const evidence = recorder.evidence()
    expect(evidence).toEqual({
      enabled: true,
      checkpointCount: 2,
      observedCheckpointCodes: [
        'settings_load_start',
        'desktop_private_history_migration_start'
      ],
      lastCheckpoint: 'desktop_private_history_migration_start',
      lastElapsedMs: 41,
      checkpointElapsedMs: {
        settings_load_start: 17,
        desktop_private_history_migration_start: 41
      }
    })
    expect(JSON.stringify(evidence)).not.toContain('/Users/private')
    expect(JSON.stringify(evidence)).not.toContain('do-not-retain')
    expect(JSON.stringify(evidence)).not.toContain('not an allowlisted stage')
    expect(JSON.stringify(evidence)).not.toContain('toString')
  })

  it('wires bounded sanitized startup tracing into both packaged launches and stable defaults', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')

    expect(script).toContain("ANALYTIX_STARTUP_TRACE: '1'")
    expect(script).toContain("stdio: ['ignore', 'pipe', 'pipe']")
    expect(script).toContain('packagedStartupTraceEvidence(firstChild)')
    expect(script).toContain('packagedStartupTraceEvidence(secondChild)')
    expect(script).toContain('startupTrace: emptyPackagedStartupTraceEvidence()')
  })

  it('rejects cross-device paths before trusting the APFS cache boundary', async () => {
    const { trustedCacheDeviceBindingEvidence } = await milestoneModule()

    expect(trustedCacheDeviceBindingEvidence(
      { dev: 42 },
      [{ dev: 42 }, { dev: 42 }]
    )).toEqual({ ok: true, reasonCode: 'trusted_cache_device_bound' })
    expect(trustedCacheDeviceBindingEvidence(
      { dev: 42 },
      [{ dev: 42 }, { dev: 43 }]
    )).toEqual({ ok: false, reasonCode: 'trusted_cache_device_mismatch' })
    expect(trustedCacheDeviceBindingEvidence(
      { dev: Number.NaN },
      [{ dev: 42 }]
    )).toEqual({ ok: false, reasonCode: 'trusted_cache_device_identity_invalid' })

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    expect(script).toContain('analytix_cache_tmpdir_device_mismatch')
    expect(script).toContain('external_repository_trusted_cache_device_mismatch')
  })

  it('prints stable help without running the harness or writing evidence', () => {
    const expectedHelp = [
      'Usage: node scripts/runtime-go-packaged-milestone-a.mjs [options]',
      '',
      'Options:',
      '  -h, --help  Show this help message.'
    ].join('\n') + '\n'

    for (const flag of ['--help', '-h']) {
      const cwd = taskOwnedSandbox()
      const result = spawnSync(process.execPath, [milestoneScriptPath, flag], {
        cwd,
        env: milestoneCliEnvironment(),
        encoding: 'utf8',
        stdio: 'pipe'
      })

      expect(result.status).toBe(0)
      expect(result.signal).toBeNull()
      expect(result.stdout).toBe(expectedHelp)
      expect(result.stderr).toBe('')
      expect(existsSync(join(
        cwd,
        'docs/analytix/upstreams/runtime-go-live-evidence/packaged-milestone-a.json'
      ))).toBe(false)
      expect(readdirSync(cwd)).toEqual([])
    }
  })

  it('records the declared visible credential-entry method without receiving credentials', async () => {
    const { credentialEntryDeclaration } = await milestoneModule()
    expect(credentialEntryDeclaration('visible-human')).toEqual({
      declared: true,
      method: 'visible-human',
      automatedCredentialEntryUsed: false
    })
    expect(credentialEntryDeclaration('visible-automation')).toEqual({
      declared: false,
      method: 'visible-automation',
      automatedCredentialEntryUsed: true
    })
    expect(credentialEntryDeclaration('')).toEqual({
      declared: false,
      method: 'undeclared',
      automatedCredentialEntryUsed: null
    })
    expect(credentialEntryDeclaration('browser-extension')).toEqual({
      declared: false,
      method: 'undeclared',
      automatedCredentialEntryUsed: null
    })

    const cliCheckpoint = (method: string) => {
      const result = spawnSync(process.execPath, [
        milestoneScriptPath,
        '--dry-run',
        '--json',
        '--no-write',
        '--no-gate',
        `--credential-entry-method=${method}`
      ], {
        cwd: process.cwd(),
        env: milestoneCliEnvironment(),
        encoding: 'utf8',
        stdio: 'pipe'
      })
      expect(result.status).toBe(0)
      expect(result.stderr).toBe('')
      return JSON.parse(result.stdout).operatorCheckpoint
    }
    expect(cliCheckpoint('visible-human')).toEqual({
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: true,
      method: 'visible-human',
      automatedCredentialEntryUsed: false,
      status: 'pending'
    })
    expect(cliCheckpoint('visible-automation')).toEqual({
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: false,
      method: 'visible-automation',
      automatedCredentialEntryUsed: true,
      status: 'pending'
    })
    expect(cliCheckpoint('browser-extension')).toEqual({
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: false,
      method: 'undeclared',
      automatedCredentialEntryUsed: null,
      status: 'pending'
    })
  })

  it('accepts visible computer-use entry for formal A0 with exact checkpoint fields', async () => {
    const { credentialEntryDeclaration } = await milestoneModule()
    expect(credentialEntryDeclaration('visible-computer-use')).toEqual({
      declared: true,
      method: 'visible-computer-use',
      automatedCredentialEntryUsed: true
    })

    const result = spawnSync(process.execPath, [
      milestoneScriptPath,
      '--dry-run',
      '--json',
      '--no-write',
      '--no-gate',
      '--credential-entry-method=visible-computer-use'
    ], {
      cwd: process.cwd(),
      env: milestoneCliEnvironment(),
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout).operatorCheckpoint).toEqual({
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: true,
      method: 'visible-computer-use',
      automatedCredentialEntryUsed: true,
      status: 'pending'
    })
  })

  it('pins formal A0 to the flash model and visible high reasoning evidence', async () => {
    const { exactTurnReasoningEffortBound } = await milestoneModule()
    const workflow = {
      resultTurnId: 'turn-1',
      thread: {
        turns: [{ id: 'turn-1', status: 'completed', reasoningEffort: 'high' }]
      }
    }
    expect(exactTurnReasoningEffortBound(workflow, 'high')).toBe(true)
    expect(exactTurnReasoningEffortBound({
      ...workflow,
      thread: {
        ...workflow.thread,
        turns: [{ id: 'turn-1', status: 'completed', reasoningEffort: 'max' }]
      }
    }, 'high')).toBe(false)

    const result = spawnSync(process.execPath, [
      milestoneScriptPath,
      '--dry-run',
      '--json',
      '--no-write',
      '--no-gate'
    ], {
      cwd: process.cwd(),
      env: milestoneCliEnvironment(),
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    expect(JSON.parse(result.stdout).provider).toEqual(expect.objectContaining({
      requiredModelHash: sha256('deepseek-v4-flash'),
      reasoningEffort: 'high',
      firstLaunchReasoningEffortSelectedThroughVisibleUi: false,
      secondLaunchReasoningEffortSelectedThroughVisibleUi: false,
      planReasoningEffortBound: false,
      ordinaryReasoningEffortBound: false,
      longContextReasoningEffortBound: false,
      relaunchReasoningEffortBound: false
    }))

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    expect(script).toContain("const FORMAL_MODEL = 'deepseek-v4-flash'")
    expect(script).toContain("const FORMAL_REASONING_EFFORT = 'high'")
    expect(script).toContain('button[aria-label="Model and reasoning settings"]')
    expect(script).toContain('button[role="menuitemradio"]')
    expect(script).toContain("requiredModel: FORMAL_MODEL")
  })

  it('binds the local Registry context profile from settings into Go runtime info', async () => {
    const { milestoneALocalProvider } = await milestoneModule()
    const model = 'deepseek-v4-flash'
    const observation = {
      providerRegistry: {
        schemaVersion: 1, registryRevision: '1', registryIncarnation: 'inc_' + 'b'.repeat(43),
        selectedProviderId: 'deepseek', providers: [{
          id: 'deepseek', kind: 'deepseek', endpoint: 'https://hub.example.invalid/v1',
          models: [model], mediaModels: [], selectedModel: model, selectedRoutes: [],
          credentialConfigured: true, credentialPurpose: 'api-key', revision: '1', generation: '1',
          incarnation: 'inc_' + 'a'.repeat(43), tombstone: false
        }]
      },
      account: {
        authenticated: true,
        gatewayConfigured: true,
        accountReady: true,
        source: 'hub'
      },
      settings: {
        provider: {
          apiKey: '',
          activeProviderId: 'deepseek',
          providers: [{
            id: 'deepseek',
            apiKey: '',
            baseUrl: 'https://hub.example.invalid/v1',
            endpointFormat: 'chat_completions',
            models: [model],
            modelProfiles: {
              [model]: { contextWindowTokens: 131_072 }
            }
          }]
        },
        runtime: {
          providerId: 'deepseek',
          model,
          apiKey: '',
          runtimeToken: ''
        }
      },
      runtimeInfo: {
        provider: {
          id: 'deepseek',
          model,
          endpointFormat: 'chat_completions',
          available: true,
          apiKeyConfigured: true,
          baseUrlConfigured: true,
          contextWindowTokens: 131_072
        }
      }
    }

    expect(milestoneALocalProvider(observation)).toEqual(expect.objectContaining({
      ok: true,
      contextWindowTokensBound: true,
      settingsContextWindowTokens: 131_072,
      runtimeContextWindowTokens: 131_072,
      softThresholdTokens: 98_304,
      hardThresholdTokens: 111_411
    }))
    expect(milestoneALocalProvider({
      ...observation,
      runtimeInfo: {
        provider: {
          ...observation.runtimeInfo.provider,
          contextWindowTokens: 1_000_000
        }
      }
    })).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'local_provider_context_window_profile_drift',
      contextWindowTokensBound: false
    }))
  })

  it('accepts an exact already-selected visible reasoning control without reopening its menu', async () => {
    const { composerReasoningClosedSelectionEvidence } = await milestoneModule()
    const exact = {
      controlCount: 1,
      control: { x: 320, y: 640, width: 160, height: 36 },
      controlTitle: 'deepseek-v4-flash / High',
      controlExpanded: false,
      expectedSelectedTitle: 'deepseek-v4-flash / High'
    }

    expect(composerReasoningClosedSelectionEvidence(exact)).toEqual({
      controlValid: true,
      selected: true,
      controlTitleSha256: sha256('deepseek-v4-flash / High')
    })
    expect(composerReasoningClosedSelectionEvidence({
      ...exact,
      controlTitle: 'deepseek-v4-flash / Medium'
    })).toEqual({
      controlValid: true,
      selected: false,
      controlTitleSha256: ''
    })
    expect(composerReasoningClosedSelectionEvidence({
      ...exact,
      controlCount: 2
    }).controlValid).toBe(false)
    expect(composerReasoningClosedSelectionEvidence({
      ...exact,
      controlExpanded: true
    }).controlValid).toBe(false)

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const selector = script.slice(
      script.indexOf('async function selectComposerReasoningEffort'),
      script.indexOf('async function cdpComposerSubmit')
    )
    expect(selector).toContain('composerReasoningClosedSelectionEvidence(closed.value)')
    expect(selector.indexOf('if (closedSelection.selected)')).toBeLessThan(
      selector.indexOf('await clickCdpGeometry(closed.target, closed.value.control)')
    )
  })

  it('fails fast only after the newly submitted workflow turn is terminal', async () => {
    const { workflowWaitDisposition } = await milestoneModule()
    const evidence = (status: string, markerTurnId = '') => ({
      threadId: 'thread-1',
      turnCount: 2,
      resultMarkerObserved: Boolean(markerTurnId),
      resultTurnId: markerTurnId,
      thread: {
        turns: [
          { id: 'turn-1', status: 'completed' },
          { id: 'turn-2', status }
        ]
      }
    })
    const baseline = { previousTurnCount: 1, expectedThreadId: 'thread-1' }

    expect(workflowWaitDisposition({
      ...evidence('completed'),
      turnCount: 1,
      thread: { turns: [{ id: 'turn-1', status: 'completed' }] }
    }, baseline)).toBe('pending')
    expect(workflowWaitDisposition(evidence('running'), baseline)).toBe('pending')
    expect(workflowWaitDisposition(evidence('failed'), baseline)).toBe(
      'terminal_without_expected_marker'
    )
    expect(workflowWaitDisposition(evidence('failed', 'turn-2'), baseline)).toBe(
      'terminal_without_expected_marker'
    )
    expect(workflowWaitDisposition(evidence('aborted', 'turn-2'), baseline)).toBe(
      'terminal_without_expected_marker'
    )
    expect(workflowWaitDisposition(evidence('completed', 'turn-1'), baseline)).toBe(
      'terminal_without_expected_marker'
    )
    expect(workflowWaitDisposition(evidence('completed', 'turn-2'), baseline)).toBe('completed')
    expect(workflowWaitDisposition(evidence('completed', 'turn-2'), {
      ...baseline,
      expectedThreadId: 'thread-other'
    })).toBe('pending')
  })

  it('closes ordinary workflow evidence from the exact typed terminal receipt without raw prose', async () => {
    const {
      ordinaryWorkflowResultEvidence,
      ordinaryWorkflowWaitDisposition
    } = await milestoneModule()
    const threadId = 'thread-typed-result'
    const turnId = 'turn-typed-result'
    const evidence = {
      threadId,
      turnCount: 2,
      resultMarkerObserved: false,
      researchMarkerObserved: false,
      writingMarkerObserved: false,
      resultTurnId: turnId,
      resultDigest: '',
      thread: {
        turns: [
          { id: 'turn-plan', status: 'completed' },
          { id: turnId, status: 'completed' }
        ]
      },
      providerTerminals: [{
        kind: 'turn_completed',
        seq: 42,
        threadId,
        turnId,
        status: 'completed'
      }],
      ordinaryResultReceipt: {
        observed: true,
        reasonCode: 'ordinary_result_observed',
        candidateOrigin: 'provider_ordinary_only',
        projectionClass: 'provider_ordinary_only',
        resultDigest: '1'.repeat(64),
        textSha256: '2'.repeat(64),
        resultMarkerObserved: false,
        researchMarkerObserved: false,
        writingMarkerObserved: false
      }
    }
    const baseline = { previousTurnCount: 1, expectedThreadId: threadId }

    expect(ordinaryWorkflowResultEvidence(evidence)).toEqual({
      bound: true,
      source: 'typed_terminal_receipt',
      resultMarkerObserved: false,
      researchMarkerObserved: false,
      writingMarkerObserved: false,
      textSha256: '2'.repeat(64)
    })
    expect(ordinaryWorkflowWaitDisposition(evidence, baseline)).toBe('completed')
    expect(ordinaryWorkflowWaitDisposition({
      ...evidence,
      providerTerminals: [{
        ...evidence.providerTerminals[0],
        turnId: 'turn-other'
      }]
    }, baseline)).toBe('terminal_without_bound_result')
    expect(ordinaryWorkflowWaitDisposition({
      ...evidence,
      ordinaryResultReceipt: {
        ...evidence.ordinaryResultReceipt,
        candidateOrigin: 'host_fixed',
        projectionClass: 'host_fixed_provider_result_withheld'
      }
    }, baseline)).toBe('terminal_without_bound_result')
    expect(ordinaryWorkflowWaitDisposition(evidence, baseline)).toBe('completed')
    expect(ordinaryWorkflowWaitDisposition({
      ...evidence,
      resultMarkerObserved: true,
      researchMarkerObserved: true,
      writingMarkerObserved: true,
      resultDigest: '3'.repeat(64),
      ordinaryResultReceipt: {
        ...evidence.ordinaryResultReceipt,
        candidateOrigin: 'host_fixed',
        projectionClass: 'host_fixed_provider_result_withheld',
        resultMarkerObserved: false,
        researchMarkerObserved: false,
        writingMarkerObserved: false
      }
    }, baseline)).toBe('terminal_without_bound_result')
  })

  it('retains an ordinary timeout and projects only bounded in-flight progress', async () => {
    const {
      ordinaryWorkflowObservedDisposition,
      workflowProgressDiagnostic
    } = await milestoneModule()
    const secret = 'PRIVATE_PROMPT_PATH_AND_TOOL_OUTPUT'
    const threadId = 'thread-in-flight-timeout'
    const turnId = 'turn-in-flight-timeout'
    const evidence = {
      threadId,
      turnCount: 2,
      thread: {
        id: threadId,
        turns: [
          { id: 'turn-plan', status: 'completed', items: [] },
          {
            id: turnId,
            threadId,
            status: 'running',
            prompt: secret,
            items: [{
              kind: 'tool_call',
              callId: 'call-read',
              toolName: 'read',
              toolKind: 'builtin',
              status: 'completed',
              arguments: { path: secret }
            }, {
              kind: 'tool_result',
              callId: 'call-read',
              toolName: 'read',
              toolKind: 'builtin',
              status: 'completed',
              isError: false,
              output: { status: 'completed', raw: secret }
            }, {
              kind: 'tool_call',
              callId: 'call-task',
              toolName: 'task',
              toolKind: 'builtin',
              status: 'running',
              arguments: { prompt: secret }
            }]
          }
        ]
      },
      providerAttempts: [],
      providerTerminals: []
    }
    const baseline = { previousTurnCount: 1, expectedThreadId: threadId }

    expect(ordinaryWorkflowObservedDisposition({
      disposition: 'timeout',
      evidence
    }, baseline)).toBe('timeout')
    const progress = workflowProgressDiagnostic(evidence, baseline)
    expect(progress).toEqual(expect.objectContaining({
      observed: true,
      terminal: false,
      turnStatus: 'running',
      turnIdHash: sha256(turnId),
      toolAttemptCount: 2,
      successfulToolExecutionCount: 1,
      failedToolResultCount: 0,
      unsettledToolResultCount: 0,
      openToolCallCount: 1,
      activeToolCategory: 'subagent',
      toolInventoryAvailability: 'public_projection_available',
      inventoryDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(progress.toolAttemptCategoryCounts).toEqual(expect.objectContaining({
      read: 1,
      subagent: 1
    }))
    expect(progress.successfulToolCategoryCounts).toEqual(expect.objectContaining({
      read: 1,
      subagent: 0
    }))
    expect(JSON.stringify(progress)).not.toContain(secret)
    expect(workflowProgressDiagnostic({
      ...evidence,
      thread: { ...evidence.thread, id: 'foreign-thread' }
    }, baseline).observed).toBe(false)
    expect(workflowProgressDiagnostic({
      ...evidence,
      turnCount: 3
    }, baseline).observed).toBe(false)
  })

  it('freezes a completed planning stage before later ordinary failure', async () => {
    const { completedPlanStageSnapshot } = await milestoneModule()
    const report = {
      provider: { planReasoningEffortBound: true },
      workflow: {
        planModeSelected: true,
        planTurnObserved: true,
        planProviderReceiptBound: true,
        planProviderReceiptScopeBound: true,
        planTaskInspectionReadsBound: true,
        planTaskInspectionExpectedReadCount: 4,
        planTaskInspectionReadAttemptCount: 4,
        planTaskInspectionReadAttemptDigest: '1'.repeat(64),
        planArtifactBound: true,
        planArtifactContentHash: '2'.repeat(64),
        planArtifactByteSize: 512,
        planThreadIdHash: '3'.repeat(64),
        rawPrompt: 'PRIVATE_PLAN_PROMPT'
      }
    }

    const snapshot = completedPlanStageSnapshot(report)
    expect(snapshot).toEqual(expect.objectContaining({
      stage: 'plan-turn',
      checkIds: ['ordinary-agent-workflow'],
      completedFields: expect.objectContaining({
        planModeSelected: true,
        planTurnObserved: true,
        planProviderReceiptBound: true,
        planProviderReceiptScopeBound: true,
        planTaskInspectionReadsBound: true,
        planArtifactBound: true,
        planReasoningEffortBound: true,
        planThreadIdHash: '3'.repeat(64)
      }),
      snapshotDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(JSON.stringify(snapshot)).not.toContain('PRIVATE_PLAN_PROMPT')
    expect(completedPlanStageSnapshot({
      ...report,
      provider: { planReasoningEffortBound: false }
    })).toBeNull()
  })

  it('finalizes a timed-out ordinary stage without losing completed plan or repository progress', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      provider: { planReasoningEffortBound: true },
      repository: {
        finalOnlyIntendedSourceChanged: true,
        finalExpectedSourceBound: true,
        finalExpectedSourceFileCount: 3,
        finalExpectedSourceMismatchCount: 0,
        finalExpectedSourceDigest: '4'.repeat(64),
        finalObservedSourceDigest: '4'.repeat(64)
      },
      workflow: {
        planModeSelected: true,
        planTurnObserved: true,
        planProviderReceiptBound: true,
        planProviderReceiptScopeBound: true,
        planTaskInspectionReadsBound: true,
        planArtifactBound: true,
        ordinaryWorkflowDisposition: 'timeout',
        ordinaryWorkflowFailureCode: 'packaged_ordinary_workflow_timeout',
        ordinaryCandidateProgressObserved: true,
        ordinaryCandidateTerminalObserved: false,
        ordinaryCandidateToolAttemptCount: 7,
        ordinaryCandidateSuccessfulToolExecutionCount: 6,
        ordinaryCandidateFailedToolResultCount: 0,
        ordinaryCandidateUnsettledToolResultCount: 0,
        ordinaryCandidateOpenToolCallCount: 1,
        ordinaryCandidateProgressDigest: '5'.repeat(64)
      },
      checks: [{
        id: 'ordinary-agent-workflow',
        status: 'failed',
        message: 'packaged_ordinary_workflow_timeout'
      }, {
        id: 'real-repository-test',
        status: 'skipped',
        message: 'stage did not reach this required check'
      }],
      stageSnapshots: []
    }, {
      requiredCheckIds: ['ordinary-agent-workflow', 'real-repository-test']
    })

    expect(report.failedCheckIds).toEqual(['ordinary-agent-workflow'])
    expect(report.skippedCheckIds).toEqual(['real-repository-test'])
    expect(report.liveBlockedCheckIds).toEqual([])
    expect(report.executionBlocker).toBe('packaged_ordinary_workflow_timeout')
    expect(report.workflow).toEqual(expect.objectContaining({
      planTurnObserved: true,
      planProviderReceiptBound: true,
      ordinaryWorkflowDisposition: 'timeout',
      ordinaryCandidateProgressObserved: true,
      ordinaryCandidateTerminalObserved: false,
      ordinaryCandidateToolAttemptCount: 7,
      ordinaryCandidateOpenToolCallCount: 1
    }))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({ stage: 'plan-turn' }),
      expect.objectContaining({
        stage: 'ordinary-agent-workflow-failure',
        completedFields: expect.objectContaining({
          ordinaryCandidateProgressObserved: true,
          ordinaryCandidateToolAttemptCount: 7,
          ordinaryCandidateOpenToolCallCount: 1,
          finalOnlyIntendedSourceChanged: true,
          finalExpectedSourceBound: true,
          finalExpectedSourceFileCount: 3,
          finalExpectedSourceMismatchCount: 0
        })
      })
    ]))
  })

  it('retains the exact terminal turn identity when a required result marker is absent', async () => {
    const { workflowEvidence } = await milestoneModule()
    const workspace = '/isolated/repository'
    const threadId = 'thread-marker-diagnostic'
    const turnId = 'turn-marker-diagnostic'
    const text = 'Ordinary work completed, but the requested marker was omitted.'
    const evidence = workflowEvidence({
      thread: {
        id: threadId,
        workspace,
        providerId: 'analytix-hub',
        model: 'deepseek-v4-flash',
        latestSeq: 14,
        turns: [{ id: 'turn-plan', status: 'completed', items: [] }, {
          id: turnId,
          status: 'completed',
          reasoningEffort: 'high',
          items: [{
            kind: 'assistant_text',
            status: 'completed',
            text
          }]
        }],
        todos: { items: [] }
      },
      summary: { subagents: [] },
      providerAttempts: [{
        kind: 'usage',
        seq: 13,
        threadId,
        turnId,
        model: 'deepseek-v4-flash',
        usageFinalStatus: 'completed',
        promptTokens: 10,
        completionTokens: 4,
        totalTokens: 14,
        turns: 1,
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 1,
        providerAttemptCount: 1,
        providerAttemptStatuses: {
          succeeded: 1,
          failed: 0,
          cancelled: 0,
          timedOut: 0,
          streamAborted: 0
        }
      }],
      providerTerminals: [{
        kind: 'turn_completed',
        seq: 14,
        threadId,
        turnId,
        status: 'completed'
      }],
      providerReceiptTrace: {
        reasonCode: 'provider_receipt_observed',
        started: true,
        acknowledged: true,
        ended: true,
        eventCount: 3
      }
    }, workspace)

    expect(evidence).toEqual(expect.objectContaining({
      resultMarkerObserved: false,
      resultTurnId: turnId,
      resultText: text
    }))
    expect(evidence.providerAttempts).toHaveLength(1)
    expect(evidence.providerTerminals).toHaveLength(1)
  })

  it('retains only safe terminal workflow diagnostics and preserves the first blocker', async () => {
    const {
      ordinaryWorkflowFailureCode,
      terminalWorkflowDiagnostic,
      workflowEvidence
    } = await milestoneModule()
    const secret = 'COMPLETE_PRIVATE_VALUE_123456789'
    const failureFixture = acceptedFinalOrdinaryContinuationObservation()
    const thread = failureFixture.observation.thread
    thread.workspace = '/isolated/repository'
    const failedTurn = thread.turns[0]
    const failedAssistant = failedTurn.items[0]
    const acceptedFinalDigest = failedTurn.acceptedFinalView.acceptedFinalDigest
    failedTurn.status = 'failed'
    failedTurn.acceptedFinalView.terminalReason = 'provider_failure'
    failedAssistant.text = secret
    failedAssistant.acceptedFinalView.terminalReason = 'provider_failure'
    const delivery = thread.acceptedFinalDelivery
    delivery.events[0].item.text = secret
    delivery.events[0].item.acceptedFinalView.terminalReason = 'provider_failure'
    const terminalErrorItem = {
      id: `item_${failedTurn.id}_case_terminal`,
      turnId: failedTurn.id,
      threadId: thread.id,
      role: 'system',
      status: 'failed',
      createdAt: delivery.timestamp,
      finishedAt: delivery.timestamp,
      kind: 'error',
      code: 'case_terminal_provider_failure',
      message: '案件分析未完成；未经核验的案件事实未发布。',
      severity: 'error',
      acceptedFinalDigest
    }
    const terminalErrorEvent = {
      kind: 'item_completed',
      seq: delivery.firstSeq + 1,
      timestamp: delivery.timestamp,
      threadId: thread.id,
      turnId: failedTurn.id,
      itemId: terminalErrorItem.id,
      item: terminalErrorItem,
      acceptedFinalDigest,
      publicationCommitId: acceptedFinalDigest,
      publicationEventId: '8'.repeat(64),
      publicationSlot: 'terminal-error-item',
      publicationPayloadDigest: '9'.repeat(64)
    }
    delivery.events.splice(1, 0, terminalErrorEvent)
    delivery.events.forEach((event: Record<string, any>, index: number) => {
      event.seq = delivery.firstSeq + index
    })
    delivery.events[2].usageFinalStatus = 'failed'
    Object.assign(delivery.events[3], {
      kind: 'turn_failed',
      status: 'failed',
      terminalReason: 'provider_failure',
      error: terminalErrorItem.message,
      message: terminalErrorItem.message,
      code: terminalErrorItem.code,
      itemId: terminalErrorItem.id
    })
    delivery.lastSeq = delivery.firstSeq + delivery.events.length - 1
    delivery.seq = delivery.lastSeq
    delivery.publicationAuthority.lastSeq = delivery.lastSeq
    thread.latestSeq = delivery.lastSeq
    failedTurn.items.push(terminalErrorItem)
    thread.turns.unshift({ id: 'turn-plan', status: 'completed', items: [] })
    thread.turnCount = thread.turns.length
    thread.acceptedFinalDeliveries = [delivery]
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(delivery).success).toBe(true)
    const evidence = workflowEvidence({
      thread,
      summary: { subagents: [] },
      providerAttempts: [],
      providerTerminals: []
    }, '/isolated/repository')
    const diagnostic = terminalWorkflowDiagnostic(evidence, { previousTurnCount: 1 })

    expect(diagnostic).toEqual(expect.objectContaining({
      observed: true,
      turnStatus: 'failed',
      turnErrorCode: 'case_terminal_provider_failure',
      turnErrorCodeHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      toolAttemptCount: 0,
      successfulToolExecutionCount: 0,
      failedToolResultCount: 0,
      unsettledToolResultCount: 0,
      providerAttemptRecordCount: 0,
      providerTerminalRecordCount: 0,
      providerReceiptCountersBound: false,
      providerLogicalCallCount: 0,
      providerAttemptCount: 0,
      providerAttemptStatusCounts: {
        succeeded: 0,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      },
      terminalReasonClass: 'provider_failure',
      terminalErrorItemCount: 1,
      terminalErrorItemAuthorityBound: true,
      toolInventoryAvailability: 'public_projection_withheld',
      providerReceiptAvailability: 'accepted_final_sse_failure_diagnostic',
      turnIdHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      inventoryDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(Object.values(diagnostic.toolAttemptCategoryCounts).every((value) => value === 0))
      .toBe(true)
    expect(Object.values(diagnostic.successfulToolCategoryCounts).every((value) => value === 0))
      .toBe(true)
    expect(JSON.stringify(diagnostic)).not.toContain(secret)
    expect(JSON.stringify(diagnostic)).not.toContain('turn-agent')

    const diagnosticFor = (mutate: (value: Record<string, any>) => void) => {
      const changed = structuredClone(thread)
      mutate(changed)
      return terminalWorkflowDiagnostic(workflowEvidence({
        thread: changed,
        summary: { subagents: [] },
        providerAttempts: [],
        providerTerminals: []
      }, '/isolated/repository'), { previousTurnCount: 1 })
    }
    for (const invalid of [
      diagnosticFor((value) => {
        value.turns[1].items[1].acceptedFinalDigest = '4'.repeat(64)
      }),
      diagnosticFor((value) => {
        value.turns[1].items.push(structuredClone(value.turns[1].items[1]))
      }),
      diagnosticFor((value) => {
        value.turns[1].items[1].code = 'case_terminal_timeout'
      }),
      diagnosticFor((value) => {
        value.turns[1].items = value.turns[1].items.slice(0, 1)
        value.turns[1].error = { code: 'case_terminal_provider_failure', message: secret }
      })
    ]) {
      expect(invalid).toEqual(expect.objectContaining({
        turnErrorCode: 'terminal_error_unclassified',
        terminalErrorItemAuthorityBound: false,
        toolInventoryAvailability: 'public_projection_withheld',
        providerReceiptAvailability: 'accepted_final_sse_failure_diagnostic'
      }))
    }
    const successFixture = acceptedFinalOrdinaryContinuationObservation()
    const successDiagnostic = terminalWorkflowDiagnostic(
      workflowEvidence(successFixture.observation, '/isolated/repository'),
      { previousTurnCount: 0 }
    )
    expect(successDiagnostic.providerReceiptAvailability).toBe('accepted_final_sse_replay')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_expected_marker',
      ordinaryPassed: false,
      repositoryBlocker: 'external_repository_expected_result_mismatch',
      terminalStatus: 'failed'
    })).toBe('packaged_ordinary_workflow_failed_without_expected_marker')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_expected_marker',
      ordinaryPassed: false,
      terminalStatus: 'completed'
    })).toBe('packaged_ordinary_workflow_completed_without_expected_marker')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_expected_marker',
      ordinaryPassed: false,
      terminalStatus: 'aborted'
    })).toBe('packaged_ordinary_workflow_aborted_without_expected_marker')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'completed',
      ordinaryPassed: false,
      repositoryBlocker: 'external_repository_expected_result_mismatch'
    })).toBe('external_repository_expected_result_mismatch')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'completed',
      ordinaryPassed: true,
      repositoryBlocker: 'external_repository_expected_result_mismatch'
    })).toBe('')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'completed',
      ordinaryPassed: false,
      ordinaryFunctionalPassed: true,
      providerReceiptBound: false
    })).toBe('packaged_ordinary_provider_receipt_failed')

    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const hostEvidence = milestone.indexOf(
      'const bashHostObservation = await observeHostToolExecution'
    )
    const completedEvidence = milestone.indexOf('readObserved: groups.read', hostEvidence)
    const ordinaryGate = milestone.indexOf('if (ordinaryFailureCode)')
    const protectedFundsGate = milestone.indexOf('const protectedFundsSubmit')
    const contextContinuation = milestone.indexOf(
      'const contextMarkers = privateRepositoryContextMarkers(repositoryAuthority)'
    )
    expect(hostEvidence).toBeGreaterThan(0)
    expect(completedEvidence).toBeGreaterThan(hostEvidence)
    expect(ordinaryGate).toBeGreaterThan(completedEvidence)
    expect(protectedFundsGate).toBeGreaterThan(ordinaryGate)
    expect(contextContinuation).toBeGreaterThan(protectedFundsGate)
    expect(milestone).toContain(
      "const agentBaselineEvidence = workflowEvidence(planned.observation, workspace, '')"
    )
    const ordinaryProviderSnapshot = milestone.indexOf(
      'const ordinaryProviderEvidenceSnapshot = Object.freeze({ ...providerEvidence })'
    )
    const protectedFundsSubmission = milestone.indexOf(
      'const protectedFundsSubmit = await cdpComposerSubmit',
      ordinaryProviderSnapshot
    )
    expect(ordinaryProviderSnapshot).toBeLessThan(ordinaryGate)
    expect(protectedFundsSubmission).toBeGreaterThan(ordinaryProviderSnapshot)
    expect(milestone.slice(ordinaryProviderSnapshot, protectedFundsSubmission))
      .toContain('ordinaryEvidence: ordinaryProviderEvidenceSnapshot')
    const contextReceiptReplay = milestone.indexOf('const contextReceiptScope =')
    const contextSubmit = milestone.indexOf('const contextSubmit = await cdpComposerSubmit')
    const continuationReasoning = milestone.indexOf(
      'const continuationReasoningSelection = await selectComposerReasoningEffort'
    )
    expect(continuationReasoning).toBeGreaterThan(contextContinuation)
    expect(contextSubmit).toBeGreaterThan(continuationReasoning)
    expect(contextReceiptReplay).toBeGreaterThan(contextSubmit)
    const contextReplayEnd = milestone.indexOf('const contextDiagnostic', contextReceiptReplay)
    expect(contextReplayEnd).toBeGreaterThan(contextReceiptReplay)
    expect(milestone).toContain(
      'export async function observeAcceptedFinalOrdinaryViaSse'
    )
    expect(milestone.slice(contextReceiptReplay, contextReplayEnd)).toContain(
      'acceptedFinalReceiptScope: contextReceiptScope'
    )
    expect(milestone.slice(contextReceiptReplay, contextReplayEnd)).not.toContain(
      'observeProviderTerminalViaSse'
    )
    expect(milestone.slice(contextReceiptReplay, contextReplayEnd)).toContain(
      'exactLatestTerminalProviderReceiptScope(contextEvidence'
    )
    expect(milestone.slice(contextReceiptReplay, contextReplayEnd)).toContain(
      "initialContextCandidateTurn?.status === 'completed'"
    )
    expect(milestone.slice(contextReceiptReplay, contextReplayEnd)).toContain(
      "'not_applicable_terminal_failure'"
    )
  })

  it('classifies formal21 transport-success terminal failures without confirming an upstream cause', async () => {
    const {
      finalizeMilestoneAReport,
      ordinaryWorkflowFailureCode,
      ordinaryWorkflowFailureDiagnosticProjection
    } = await milestoneModule()
    const terminal = formal21TerminalWorkflowDiagnostic()
    const provider = providerFailureDiagnosticMissing()
    const tool = toolFailureDiagnosticNotObserved()
    const projection = ordinaryWorkflowFailureDiagnosticProjection(
      terminal,
      provider,
      tool
    )

    expect(projection).toEqual(expect.objectContaining({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_failure_diagnostic_missing',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: false,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(Object.isFrozen(projection)).toBe(true)
    expect(JSON.stringify(projection)).not.toContain('PRIVATE_PROMPT')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_bound_result',
      ordinaryPassed: false,
      terminalStatus: terminal.turnStatus,
      workflowFailureClassification: projection.classification
    })).toBe('packaged_ordinary_provider_transport_succeeded_terminal_failure')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'completed',
      ordinaryPassed: false,
      terminalStatus: 'failed',
      workflowFailureClassification: projection.classification
    })).not.toBe('packaged_ordinary_provider_transport_succeeded_terminal_failure')
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_expected_marker',
      ordinaryPassed: true,
      terminalStatus: 'failed',
      workflowFailureClassification: projection.classification
    })).toBe('')

    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      executionBlocker: 'packaged_ordinary_provider_transport_succeeded_terminal_failure',
      workflow: {
        ordinaryWorkflowCompleted: false,
        ordinaryCandidateFailureClassification: projection.classification,
        ordinaryCandidateFailureObservationCode: projection.observationCode,
        ordinaryCandidateProviderTransportSucceeded: projection.providerTransportSucceeded,
        ordinaryCandidateUpstreamCauseConfirmed: projection.upstreamCauseConfirmed,
        ordinaryCandidateFailureDiagnosticDigest: projection.diagnosticDigest
      },
      checks: [
        {
          id: 'ordinary-agent-workflow',
          status: 'failed',
          message: 'packaged_ordinary_provider_transport_succeeded_terminal_failure'
        },
        { id: 'real-repository-test', status: 'skipped', message: 'stage did not reach this required check' },
        { id: 'bounded-subagent', status: 'skipped', message: 'stage did not reach this required check' }
      ]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow', 'real-repository-test', 'bounded-subagent']
    })
    expect(report.failedCheckIds).toEqual(['ordinary-agent-workflow'])
    expect(report.skippedCheckIds).toEqual(['real-repository-test', 'bounded-subagent'])
    expect(report.executionBlocker).toBe(
      'packaged_ordinary_provider_transport_succeeded_terminal_failure'
    )
    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'ordinary-agent-workflow',
        status: 'failed',
        message: 'packaged_ordinary_provider_transport_succeeded_terminal_failure'
      })
    ]))
    expect(report.workflow).toEqual(expect.objectContaining({
      ordinaryCandidateFailureClassification: 'provider_transport_succeeded_terminal_failure',
      ordinaryCandidateFailureObservationCode: 'provider_failure_diagnostic_missing',
      ordinaryCandidateProviderTransportSucceeded: true,
      ordinaryCandidateUpstreamCauseConfirmed: false,
      ordinaryCandidateFailureDiagnosticDigest: projection.diagnosticDigest
    }))
    expect(report.stageSnapshots).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ stage: 'ordinary-agent-workflow' })
    ]))
  })

  it('keeps provider transport proof when a settled tool failure has no closed upstream cause', async () => {
    const {
      ordinaryWorkflowFailureCode,
      ordinaryWorkflowFailureDiagnosticProjection
    } = await milestoneModule()
    const terminal = {
      ...formal21TerminalWorkflowDiagnostic(),
      turnStatus: 'failed',
      toolAttemptCount: 9,
      successfulToolExecutionCount: 8,
      failedToolResultCount: 1,
      toolAttemptCategoryCounts: workflowDiagnosticCategories({
        read: 2,
        todo: 4,
        write: 1,
        bash: 2
      }),
      successfulToolCategoryCounts: workflowDiagnosticCategories({
        read: 2,
        todo: 3,
        write: 1,
        bash: 2
      }),
      providerLogicalCallCount: 9,
      providerAttemptCount: 9,
      providerAttemptStatusCounts: {
        succeeded: 9,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      }
    }
    const projection = ordinaryWorkflowFailureDiagnosticProjection(
      terminal,
      providerFailureDiagnosticMissing(),
      toolFailureDiagnosticNotObserved()
    )

    expect(projection).toEqual(expect.objectContaining({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_failure_diagnostic_missing',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: false,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_bound_result',
      ordinaryPassed: false,
      terminalStatus: terminal.turnStatus,
      workflowFailureClassification: projection.classification
    })).toBe('packaged_ordinary_provider_transport_succeeded_terminal_failure')

    for (const invalidTerminal of [{
      ...terminal,
      failedToolResultCount: 2
    }, {
      ...terminal,
      unsettledToolResultCount: 1
    }, {
      ...terminal,
      successfulToolCategoryCounts: workflowDiagnosticCategories({
        read: 2,
        todo: 4,
        write: 1,
        bash: 2
      })
    }]) {
      expect(ordinaryWorkflowFailureDiagnosticProjection(
        invalidTerminal,
        providerFailureDiagnosticMissing(),
        toolFailureDiagnosticNotObserved()
      )).toEqual(expect.objectContaining({
        classification: 'invalid',
        observationCode: 'workflow_failure_diagnostic_invalid',
        providerTransportSucceeded: false,
        upstreamCauseConfirmed: false,
        diagnosticDigest: ''
      }))
    }
  })

  it('confirms a closed post-provider host phase without inventing a provider failure', async () => {
    const {
      finalizeMilestoneAReport,
      ordinaryWorkflowFailureCode,
      ordinaryWorkflowFailureDiagnosticProjection
    } = await milestoneModule()
    const terminal = {
      ...formal21TerminalWorkflowDiagnostic(),
      turnStatus: 'failed',
      turnErrorCode: 'host_candidate_publication_failed',
      turnErrorCodeHash: sha256('host_candidate_publication_failed')
    }
    const projection = ordinaryWorkflowFailureDiagnosticProjection(
      terminal,
      {
        ...providerFailureDiagnosticMissing(),
        observationCode: 'provider_failure_terminal_invalid'
      },
      toolFailureDiagnosticNotObserved()
    )

    expect(projection).toEqual(expect.objectContaining({
      classification: 'host_post_response_failure',
      observationCode: 'host_failure_phase_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureCode({
      disposition: 'terminal_without_bound_result',
      ordinaryPassed: false,
      terminalStatus: terminal.turnStatus,
      workflowFailureClassification: projection.classification
    })).toBe('packaged_ordinary_host_post_response_failure')

    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      executionBlocker: 'packaged_ordinary_host_post_response_failure',
      workflow: {
        ordinaryWorkflowCompleted: false,
        ordinaryCandidateTurnErrorCode: terminal.turnErrorCode,
        ordinaryCandidateFailureClassification: projection.classification,
        ordinaryCandidateFailureObservationCode: projection.observationCode,
        ordinaryCandidateProviderTransportSucceeded: projection.providerTransportSucceeded,
        ordinaryCandidateUpstreamCauseConfirmed: projection.upstreamCauseConfirmed,
        ordinaryCandidateFailureDiagnosticDigest: projection.diagnosticDigest
      },
      checks: [{
        id: 'ordinary-agent-workflow',
        status: 'failed',
        message: 'packaged_ordinary_host_post_response_failure'
      }]
    }, { requiredCheckIds: ['ordinary-agent-workflow', 'real-repository-test'] })
    expect(report).toEqual(expect.objectContaining({
      status: 'failed',
      passed: false,
      executionBlocker: 'packaged_ordinary_host_post_response_failure',
      failedCheckIds: ['ordinary-agent-workflow'],
      skippedCheckIds: ['real-repository-test'],
      liveBlockedCheckIds: []
    }))
    expect(report.workflow).toEqual(expect.objectContaining({
      ordinaryCandidateTurnErrorCode: 'host_candidate_publication_failed',
      ordinaryCandidateFailureClassification: 'host_post_response_failure',
      ordinaryCandidateFailureObservationCode: 'host_failure_phase_observed',
      ordinaryCandidateProviderTransportSucceeded: true,
      ordinaryCandidateUpstreamCauseConfirmed: true
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      toolAttemptCount: 0,
      successfulToolExecutionCount: 0,
      toolAttemptCategoryCounts: workflowDiagnosticCategories(),
      successfulToolCategoryCounts: workflowDiagnosticCategories()
    }, {
      ...providerFailureDiagnosticMissing(),
      observationCode: 'provider_failure_terminal_invalid'
    }, toolFailureDiagnosticNotObserved())).toEqual(expect.objectContaining({
      classification: 'host_post_response_failure',
      observationCode: 'host_failure_phase_observed'
    }))

    const providerTerminalInvalid = {
      ...providerFailureDiagnosticMissing(),
      observationCode: 'provider_failure_terminal_invalid'
    }
    for (const invalidTerminal of [{
      ...terminal,
      turnErrorCode: 'host_invented_failed',
      turnErrorCodeHash: sha256('host_invented_failed')
    }, {
      ...terminal,
      turnErrorCodeHash: sha256('host_candidate_authority_failed')
    }]) {
      expect(ordinaryWorkflowFailureDiagnosticProjection(
        invalidTerminal,
        providerTerminalInvalid,
        toolFailureDiagnosticNotObserved()
      )).toEqual(expect.objectContaining({
        classification: 'invalid',
        observationCode: 'workflow_failure_diagnostic_invalid',
        diagnosticDigest: ''
      }))
    }
  })

  it('classifies a runtime-blocked private tool argument without retaining its body', async () => {
    const { ordinaryWorkflowFailureDiagnosticProjection } = await milestoneModule()
    const terminal = {
      ...formal21TerminalWorkflowDiagnostic(),
      turnErrorCode: 'tool_private_arguments',
      turnErrorCodeHash: sha256('tool_private_arguments'),
      toolAttemptCount: 7,
      successfulToolExecutionCount: 7,
      toolAttemptCategoryCounts: workflowDiagnosticCategories({ read: 5, todo: 2 }),
      successfulToolCategoryCounts: workflowDiagnosticCategories({ read: 5, todo: 2 }),
      providerLogicalCallCount: 3,
      providerAttemptCount: 3,
      providerAttemptStatusCounts: {
        succeeded: 3,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      }
    }
    const provider = {
      ...providerFailureDiagnosticMissing(),
      observationCode: 'provider_failure_terminal_invalid'
    }
    const tool = toolFailureDiagnosticNotObserved()

    const projection = ordinaryWorkflowFailureDiagnosticProjection(terminal, provider, tool)
    expect(projection).toEqual(expect.objectContaining({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_output_policy_failure_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(JSON.stringify(projection)).not.toContain('PRIVATE_TOOL_ARGUMENT_BODY')

    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      turnErrorCode: 'tool_schema_invalid',
      turnErrorCodeHash: sha256('tool_schema_invalid')
    }, provider, tool)).toEqual(expect.objectContaining({
      classification: 'invalid',
      observationCode: 'workflow_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
  })

  it('binds a post-transport provider output policy failure to its closed code', async () => {
    const { ordinaryWorkflowFailureDiagnosticProjection } = await milestoneModule()
    const terminal = {
      ...formal21TerminalWorkflowDiagnostic(),
      turnErrorCode: 'provider_reasoning_markup_invalid',
      turnErrorCodeHash: sha256('provider_reasoning_markup_invalid')
    }
    const provider = {
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_reasoning_markup_invalid',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed',
      diagnosticDigest: '5'.repeat(64)
    }

    expect(ordinaryWorkflowFailureDiagnosticProjection(
      terminal,
      provider,
      toolFailureDiagnosticNotObserved()
    )).toEqual(expect.objectContaining({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_output_policy_failure_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection(
      formal21TerminalWorkflowDiagnostic(),
      {
        ...provider,
        observationCode: 'provider_failure_terminal_observed',
        reasonCode: 'provider_empty_final'
      },
      toolFailureDiagnosticNotObserved()
    )).toEqual(expect.objectContaining({
      classification: 'provider_transport_succeeded_terminal_failure',
      observationCode: 'provider_output_policy_failure_observed',
      providerTransportSucceeded: true,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      turnErrorCode: 'provider_empty_final',
      turnErrorCodeHash: sha256('provider_empty_final')
    }, provider, toolFailureDiagnosticNotObserved())).toEqual(expect.objectContaining({
      classification: 'invalid',
      observationCode: 'workflow_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
  })

  it('keeps provider pipeline and tool failures closed and rejects conflicts, incoherent statuses, and raw schema', async () => {
    const { ordinaryWorkflowFailureDiagnosticProjection } = await milestoneModule()
    const terminal = formal21TerminalWorkflowDiagnostic()
    const provider = providerFailureDiagnosticMissing()
    const tool = toolFailureDiagnosticNotObserved()
    const providerObserved = {
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_unavailable',
      providerKind: 'server',
      providerStatus: 503,
      providerRetryable: true,
      providerAuthStatus: 'none',
      diagnosticDigest: '4'.repeat(64)
    }
    const toolObserved = {
      observed: true,
      observationCode: 'tool_failure_observed',
      evidenceCode: 'tool_failure_evidence_observed',
      toolFailureGuardBound: true,
      guardCount: 1,
      maxStormCount: 1,
      guardKind: 'tool_failure',
      toolCategory: 'bash',
      toolNameHash: '5'.repeat(64),
      invalidArgumentGuardCount: 0,
      invalidArgumentMaxStormCount: 0,
      invalidArgumentToolCategory: 'none',
      invalidArgumentToolNameHash: '',
      terminalReason: 'tool_failure',
      terminalCode: 'tool_failure_storm',
      diagnosticDigest: '6'.repeat(64)
    }

    expect(ordinaryWorkflowFailureDiagnosticProjection(terminal, providerObserved, tool))
      .toEqual(expect.objectContaining({
        classification: 'provider_pipeline_failure',
        observationCode: 'provider_failure_observed',
        providerTransportSucceeded: false,
        upstreamCauseConfirmed: true,
        diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
      }))
    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      providerLogicalCallCount: 6,
      providerAttemptCount: 6,
      providerAttemptStatusCounts: {
        succeeded: 5,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 1
      }
    }, {
      ...providerObserved,
      observationCode: 'provider_failure_terminal_observed',
      reasonCode: 'provider_stream_interrupted',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }, tool)).toEqual(expect.objectContaining({
      classification: 'provider_pipeline_failure',
      observationCode: 'provider_failure_terminal_observed',
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      turnErrorCode: 'tool_failure_storm',
      terminalReasonClass: 'none',
      failedToolResultCount: 1
    }, provider, toolObserved)).toEqual(expect.objectContaining({
      classification: 'tool_failure',
      observationCode: 'tool_failure_observed',
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: true,
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection(terminal, providerObserved, toolObserved))
      .toEqual(expect.objectContaining({
        classification: 'invalid',
        observationCode: 'workflow_failure_diagnostic_invalid',
        providerTransportSucceeded: false,
        upstreamCauseConfirmed: false,
        diagnosticDigest: ''
      }))
    expect(ordinaryWorkflowFailureDiagnosticProjection({
      ...terminal,
      providerAttemptStatusCounts: {
        succeeded: 0,
        failed: 1,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      }
    }, provider, tool)).toEqual(expect.objectContaining({
      classification: 'invalid',
      observationCode: 'workflow_failure_diagnostic_invalid',
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: false,
      diagnosticDigest: ''
    }))
    expect(ordinaryWorkflowFailureDiagnosticProjection(terminal, provider, {
      ...tool,
      evidenceCode: 'tool_failure_evidence_observer_rejected'
    })).toEqual(expect.objectContaining({
      classification: 'invalid',
      observationCode: 'workflow_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
    const raw = {
      ...terminal,
      prompt: 'PRIVATE_RAW_PROMPT',
      toolOutput: 'PRIVATE_RAW_TOOL_OUTPUT'
    }
    const invalid = ordinaryWorkflowFailureDiagnosticProjection(raw, provider, tool)
    expect(invalid).toEqual(expect.objectContaining({
      classification: 'invalid',
      observationCode: 'workflow_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
    expect(JSON.stringify(invalid)).not.toContain('PRIVATE_RAW_')
  })

  it('keeps a safe default projection on the ordinary success path', async () => {
    const { ordinaryWorkflowFailureDiagnosticProjection } = await milestoneModule()
    const projection = ordinaryWorkflowFailureDiagnosticProjection(
      {
        ...formal21TerminalWorkflowDiagnostic(),
        turnStatus: 'completed',
        turnErrorCode: 'none',
        turnErrorCodeHash: '',
        terminalErrorItemCount: 0,
        terminalErrorItemAuthorityBound: false,
        terminalReasonClass: 'none',
        toolInventoryAvailability: 'public_projection_available',
        providerReceiptAvailability: 'accepted_final_sse_replay'
      },
      providerFailureDiagnosticMissing(),
      toolFailureDiagnosticNotObserved()
    )
    expect(projection).toEqual(expect.objectContaining({
      classification: 'not_observed',
      observationCode: 'workflow_diagnostic_not_observed',
      providerTransportSucceeded: false,
      upstreamCauseConfirmed: false,
      diagnosticDigest: ''
    }))
  })

  it('classifies a host-fixed typed ordinary result without retaining its text', async () => {
    const {
      ordinaryResultReceiptEvidence,
      terminalWorkflowDiagnostic,
      workflowEvidence
    } = await milestoneModule()
    const threadId = 'thread-host-fixed-diagnostic'
    const turnId = 'turn-host-fixed-diagnostic'
    const receipt = {
      observed: true,
      reasonCode: 'ordinary_result_observed',
      candidateOrigin: 'host_fixed',
      projectionClass: 'host_fixed_provider_result_withheld',
      resultDigest: 'a'.repeat(64),
      textSha256: 'b'.repeat(64),
      resultMarkerObserved: false,
      researchMarkerObserved: false,
      writingMarkerObserved: false
    }
    const evidence = workflowEvidence({
      thread: {
        id: threadId,
        workspace: '/isolated/repository',
        turns: [{ id: 'turn-plan', status: 'completed', items: [] }, {
          id: turnId,
          status: 'completed',
          items: [{
            kind: 'assistant_text',
            status: 'completed',
            text: 'PRIVATE_HOST_FIXED_TEXT_MUST_NOT_ENTER_DIAGNOSTIC'
          }]
        }],
        todos: { items: [] }
      },
      summary: { subagents: [] },
      providerAttempts: [],
      providerTerminals: [],
      ordinaryResultReceipt: receipt
    }, '/isolated/repository')
    const diagnostic = terminalWorkflowDiagnostic(evidence, { previousTurnCount: 1 })

    expect(diagnostic).toEqual(expect.objectContaining({
      ordinaryResultObserved: true,
      ordinaryResultReasonCode: 'ordinary_result_observed',
      ordinaryResultCandidateOrigin: 'host_fixed',
      ordinaryResultProjectionClass: 'host_fixed_provider_result_withheld',
      ordinaryResultDigest: 'a'.repeat(64),
      ordinaryResultTextSha256: 'b'.repeat(64),
      ordinaryResultMarkerObserved: false,
      ordinaryResultResearchMarkerObserved: false,
      ordinaryResultWritingMarkerObserved: false
    }))
    expect(JSON.stringify(diagnostic)).not.toContain('PRIVATE_HOST_FIXED_TEXT')
    expect(ordinaryResultReceiptEvidence({
      ...receipt,
      resultMarkerObserved: true
    })).toEqual(expect.objectContaining({
      observed: false,
      reasonCode: 'ordinary_result_receipt_invalid',
      resultDigest: ''
    }))

    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const providerSnapshot = milestone.indexOf(
      'const ordinaryProviderEvidenceSnapshot = Object.freeze({ ...providerEvidence })'
    )
    const hostEvidence = milestone.indexOf(
      'const bashHostObservation = await observeHostToolExecution',
      providerSnapshot
    )
    const terminalGate = milestone.indexOf('if (ordinaryFailureCode)', hostEvidence)
    expect(providerSnapshot).toBeGreaterThan(0)
    expect(hostEvidence).toBeGreaterThan(providerSnapshot)
    expect(terminalGate).toBeGreaterThan(hostEvidence)
  })

  it('counts canonical failed tool results as settled terminal evidence', async () => {
    const { terminalWorkflowDiagnostic, workflowEvidence } = await milestoneModule()
    const thread = {
      id: 'thread-failed-tool-settlement',
      workspace: '/isolated/repository',
      turns: [{ id: 'turn-before', status: 'completed', items: [] }, {
        id: 'turn-failed-tool',
        status: 'failed',
        items: [{
          kind: 'tool_call',
          callId: 'call-read-failed',
          toolName: 'read',
          status: 'completed'
        }, {
          kind: 'tool_result',
          callId: 'call-read-failed',
          toolName: 'read',
          status: 'failed',
          isError: true,
          output: { status: 'failed' }
        }, {
          kind: 'error',
          status: 'failed',
          code: 'tool_failure_storm'
        }]
      }],
      todos: { items: [] }
    }
    const diagnostic = terminalWorkflowDiagnostic(workflowEvidence({
      thread,
      summary: { subagents: [] },
      providerAttempts: [{
        threadId: thread.id,
        turnId: 'turn-failed-tool',
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 9,
        providerAttemptCount: 10,
        providerAttemptStatuses: {
          succeeded: 9,
          failed: 1,
          cancelled: 0,
          timedOut: 0,
          streamAborted: 0
        }
      }],
      providerTerminals: [{
        threadId: thread.id,
        turnId: 'turn-failed-tool',
        status: 'failed'
      }]
    }, '/isolated/repository'), { previousTurnCount: 1 })

    expect(diagnostic).toEqual(expect.objectContaining({
      observed: true,
      turnStatus: 'failed',
      failedToolResultCount: 1,
      unsettledToolResultCount: 0,
      toolAttemptCount: 1,
      turnErrorCode: 'tool_failure_storm',
      providerAttemptRecordCount: 1,
      providerTerminalRecordCount: 1,
      providerReceiptCountersBound: true,
      providerLogicalCallCount: 9,
      providerAttemptCount: 10,
      providerAttemptStatusCounts: {
        succeeded: 9,
        failed: 1,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      }
    }))
  })

  it.each(['write_file', 'edit_file'])(
    'counts the canonical %s alias as write evidence',
    async (toolName) => {
      const { terminalWorkflowDiagnostic, workflowEvidence } = await milestoneModule()
      const turnId = `turn-${toolName}`
      const callId = `call-${toolName}`
      const thread = {
        id: `thread-${toolName}`,
        workspace: '/isolated/repository',
        turns: [{ id: 'turn-before', status: 'completed', items: [] }, {
          id: turnId,
          status: 'failed',
          items: [{
            kind: 'tool_call', callId, toolName, toolKind: 'host', status: 'completed'
          }, {
            kind: 'tool_result', callId, toolName, toolKind: 'host', status: 'completed',
            isError: false, output: { status: 'completed' }
          }, {
            kind: 'error', status: 'failed', code: 'host_candidate_publication_failed'
          }]
        }],
        todos: { items: [] }
      }
      const diagnostic = terminalWorkflowDiagnostic(workflowEvidence({
        thread,
        summary: { subagents: [] },
        providerAttempts: [],
        providerTerminals: []
      }, '/isolated/repository'), { previousTurnCount: 1 })

      expect(diagnostic.toolAttemptCategoryCounts).toEqual(
        workflowDiagnosticCategories({ write: 1 })
      )
      expect(diagnostic.successfulToolCategoryCounts).toEqual(
        workflowDiagnosticCategories({ write: 1 })
      )
    }
  )

  it('accepts a schema-valid public post-case ordinary terminal from accepted-final delivery', async () => {
    const { acceptedFinalOrdinaryContinuationEvidence } = await milestoneModule()
    const fixture = acceptedFinalOrdinaryContinuationObservation()
    const publicThread = ThreadDetailResponseV1Schema.parse(fixture.observation.thread)
    const accepted = acceptedFinalOrdinaryContinuationEvidence(
      { thread: publicThread },
      {
        expectedThreadId: fixture.threadId,
        expectedTurnId: fixture.turnId,
        requiredMarkers: fixture.markers,
        provider: fixture.provider
      }
    )

    expect(accepted).toEqual(expect.objectContaining({
      ok: true,
      threadId: fixture.threadId,
      turnId: fixture.turnId,
      acceptedFinalDigest: fixture.acceptedFinalDigest,
      renderedTextSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      deliveryManifestDigest: fixture.eventManifestDigest,
      providerClosureDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      requiredMarkerCount: fixture.markers.length
    }))
    const assistant = fixture.observation.thread.turns[0].items[0]
    expect(assistant).not.toHaveProperty('ordinaryResult')
    expect(assistant).not.toHaveProperty('acceptedFinal')
    expect(fixture.observation.thread.turns[0]).not.toHaveProperty('acceptedFinal')

    const explicitProvider = structuredClone(fixture)
    explicitProvider.observation.thread.acceptedFinalDelivery.events[1].providerId =
      explicitProvider.provider.id
    expect(acceptedFinalOrdinaryContinuationEvidence(
      explicitProvider.observation,
      {
        expectedThreadId: explicitProvider.threadId,
        expectedTurnId: explicitProvider.turnId,
        requiredMarkers: explicitProvider.markers,
        provider: explicitProvider.provider
      }
    ).ok).toBe(true)

    const multiHistory = structuredClone(fixture)
    const multiThread = multiHistory.observation.thread
    const currentTurn = multiThread.turns[0]
    const currentDelivery = multiThread.acceptedFinalDelivery
    const priorTurn = structuredClone(currentTurn)
    const priorDelivery = structuredClone(currentDelivery)
    const priorTurnId = 'turn-case-bound-ordinary-prior'
    const priorDigest = 'b'.repeat(64)
    priorTurn.id = priorTurnId
    priorTurn.items[0].id = 'item-case-bound-ordinary-prior'
    priorTurn.items[0].turnId = priorTurnId
    priorTurn.items[0].text = 'Earlier delivery-sealed boundary result.'
    priorTurn.acceptedFinalView.acceptedFinalDigest = priorDigest
    priorDelivery.turnId = priorTurnId
    priorDelivery.publicationCommitId = priorDigest
    priorDelivery.batchId = '4'.repeat(64)
    priorDelivery.firstSeq = 1
    priorDelivery.lastSeq = 3
    priorDelivery.seq = 3
    Object.assign(priorDelivery.publicationAuthority, {
      sealId: '5'.repeat(64),
      turnId: priorTurnId,
      publicationCommitId: priorDigest,
      batchId: priorDelivery.batchId,
      firstSeq: 1,
      lastSeq: 3
    })
    priorDelivery.events.forEach((event: Record<string, any>, index: number) => {
      event.turnId = priorTurnId
      event.seq = index + 1
      event.acceptedFinalDigest = priorDigest
      event.publicationCommitId = priorDigest
      event.publicationEventId = String(index + 6).repeat(64)
      event.publicationPayloadDigest = String(index + 1).repeat(64)
    })
    priorDelivery.events[0].item = priorTurn.items[0]
    priorDelivery.events[0].itemId = priorTurn.items[0].id
    multiThread.turns.unshift(priorTurn)
    multiThread.turnCount = 2
    multiThread.acceptedFinalDeliveries = [priorDelivery, currentDelivery]
    delete multiThread.acceptedFinalDelivery
    const multiObserved = acceptedFinalOrdinaryContinuationEvidence(
      multiHistory.observation,
      {
        expectedThreadId: multiHistory.threadId,
        expectedTurnId: multiHistory.turnId,
        requiredMarkers: multiHistory.markers,
        provider: multiHistory.provider
      }
    )
    expect(multiObserved.ok, JSON.stringify(multiObserved)).toBe(true)

    const hostilePriorView = structuredClone(multiHistory)
    hostilePriorView.observation.thread.turns[0].acceptedFinalView.acceptedFinalDigest =
      'c'.repeat(64)
    expect(acceptedFinalOrdinaryContinuationEvidence(
      hostilePriorView.observation,
      {
        expectedThreadId: hostilePriorView.threadId,
        expectedTurnId: hostilePriorView.turnId,
        requiredMarkers: hostilePriorView.markers,
        provider: hostilePriorView.provider
      }
    ).ok).toBe(false)

    const invalidMutations: Array<(value: Record<string, any>) => void> = [
      (value) => { delete value.observation.thread.acceptedFinalDeliveries },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.eventManifestDigest = '9'.repeat(64)
      },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.events[1].model = 'foreign-model'
      },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.events[1].providerId =
          'foreign-provider'
      },
      (value) => { value.observation.thread.historyAuthority = 'ordinary_history_v1' },
      (value) => { value.observation.thread.latestTurnId = 'foreign-turn' },
      (value) => {
        value.observation.thread.turns[0].items.push(
          structuredClone(value.observation.thread.turns[0].items[0])
        )
      },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.events.reverse()
      },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.events[1].usage.totalTokens = 101
      },
      (value) => {
        value.observation.thread.acceptedFinalDelivery.events[2].terminalReason =
          'provider_failure'
      },
      (value) => {
        value.observation.thread.turns[0].acceptedFinalView.publicViewDigest =
          '8'.repeat(64)
      },
      (value) => { value.markers.push('MILESTONE_A_CONTEXT_MISSING_TEST') }
    ]
    for (const [index, mutate] of invalidMutations.entries()) {
      const invalid = structuredClone(fixture)
      mutate(invalid)
      expect(acceptedFinalOrdinaryContinuationEvidence(
        invalid.observation,
        {
          expectedThreadId: invalid.threadId,
          expectedTurnId: invalid.turnId,
          requiredMarkers: invalid.markers,
          provider: invalid.provider
        }
      ).ok, `invalid accepted-final continuation mutation ${index}`).toBe(false)
    }
  })

  it('accepts the real Go case accepted-final shape without latestTurnId but rejects a present mismatch', async () => {
    const {
      acceptedFinalOrdinaryContinuationEvidence,
      exactTurnReasoningEffortBound
    } = await milestoneModule()
    const fixture = acceptedFinalOrdinaryContinuationObservation()
    const withoutLatest = structuredClone(fixture)
    delete withoutLatest.observation.thread.latestTurnId

    const publicThread: any = ThreadDetailResponseV1Schema.parse(withoutLatest.observation.thread)
    expect(publicThread).not.toHaveProperty('latestTurnId')
    expect(exactTurnReasoningEffortBound({
      thread: publicThread,
      resultTurnId: withoutLatest.turnId
    }, 'high')).toBe(true)
    const accepted = acceptedFinalOrdinaryContinuationEvidence(
      { thread: publicThread },
      {
        expectedThreadId: withoutLatest.threadId,
        expectedTurnId: withoutLatest.turnId,
        requiredMarkers: withoutLatest.markers,
        provider: withoutLatest.provider
      }
    )
    expect(accepted).toEqual(expect.objectContaining({
      ok: true,
      threadId: withoutLatest.threadId,
      turnId: withoutLatest.turnId,
      acceptedFinalDigest: withoutLatest.acceptedFinalDigest
    }))

    const mismatched = structuredClone(withoutLatest)
    mismatched.observation.thread.latestTurnId = 'foreign-turn'
    expect(acceptedFinalOrdinaryContinuationEvidence(
      mismatched.observation,
      {
        expectedThreadId: mismatched.threadId,
        expectedTurnId: mismatched.turnId,
        requiredMarkers: mismatched.markers,
        provider: mismatched.provider
      }
    )).toEqual(expect.objectContaining({
      ok: false,
      reasonCode: 'turn_invalid',
      turnBound: false
    }))
  })

  it('projects safe accepted-final continuation reason codes without retaining content', async () => {
    const { acceptedFinalOrdinaryContinuationEvidence } = await milestoneModule()
    const fixture = acceptedFinalOrdinaryContinuationObservation()
    const options = {
      expectedThreadId: fixture.threadId,
      expectedTurnId: fixture.turnId,
      requiredMarkers: fixture.markers,
      provider: fixture.provider
    }
    const valid = acceptedFinalOrdinaryContinuationEvidence(fixture.observation, options)
    expect(valid).toEqual(expect.objectContaining({
      ok: true,
      reasonCode: 'observed',
      inputValid: true,
      turnBound: true,
      assistantBound: true,
      markerBound: true,
      acceptedFinalBound: true,
      deliveryBound: true,
      providerClosureBound: true
    }))
    const cases: Array<{
      reasonCode: string
      mutate: (value: Record<string, any>) => void
    }> = [
      {
        reasonCode: 'turn_invalid',
        mutate: (value) => { value.options.expectedTurnId = 'turn-other' }
      },
      {
        reasonCode: 'assistant_invalid',
        mutate: (value) => {
          value.fixture.observation.thread.turns[0].items[0].role = 'user'
        }
      },
      {
        reasonCode: 'assistant_invalid',
        mutate: (value) => {
          value.fixture.observation.thread.turns[0].items[0].acceptedFinal = {
            schemaVersion: 5,
            recordDigest: value.fixture.acceptedFinalDigest
          }
        }
      },
      {
        reasonCode: 'marker_invalid',
        mutate: (value) => { value.options.requiredMarkers = ['MISSING_MARKER'] }
      },
      {
        reasonCode: 'accepted_final_invalid',
        mutate: (value) => {
          value.fixture.observation.thread.turns[0].acceptedFinalView.publicationState =
            'pending'
          value.fixture.observation.thread.turns[0].items[0].acceptedFinalView.publicationState =
            'pending'
          value.fixture.observation.thread.acceptedFinalDelivery.events[0]
            .item.acceptedFinalView.publicationState = 'pending'
        }
      },
      {
        reasonCode: 'delivery_invalid',
        mutate: (value) => {
          value.fixture.observation.thread.acceptedFinalDelivery.events.reverse()
        }
      },
      {
        reasonCode: 'delivery_invalid',
        mutate: (value) => {
          value.fixture.observation.thread.acceptedFinalDelivery.events[1]
            .usageFinalStatus = 'failed'
        }
      }
    ]
    for (const candidate of cases) {
      const value = {
        fixture: structuredClone(fixture),
        options: structuredClone(options)
      }
      candidate.mutate(value)
      const observed = acceptedFinalOrdinaryContinuationEvidence(
        value.fixture.observation,
        value.options
      )
      expect(observed).toEqual(expect.objectContaining({
        ok: false,
        reasonCode: candidate.reasonCode,
        inputValid: true
      }))
      expect(JSON.stringify(observed)).not.toContain('Continuation completed')
      expect(JSON.stringify(observed)).not.toContain('MILESTONE_A_CONTEXT_')
    }
  })

  it('builds a bounded ordinary continuation prompt with one exact read and no risky classification language', async () => {
    const { boundedContinuationPrompt } = await milestoneModule()
    const prompt = boundedContinuationPrompt({
      path: 'docs/continuation.txt',
      limit: 3
    })
    expect(prompt).toContain('ordinary repository continuation')
    expect(prompt).toContain('exactly one read')
    expect(prompt).toContain('"path":"docs/continuation.txt","limit":3')
    expect(prompt).toContain('no other tool')
    expect(prompt).toContain('no delegation')
    expect(prompt).toContain('Only after the read succeeds')
    expect(prompt).not.toContain('MILESTONE_A_')
    for (const forbidden of ['protected funds', 'case', 'host-owned']) {
      expect(prompt.toLowerCase()).not.toContain(forbidden)
    }
  })

  it('preserves the ordinary provider snapshot when continuation later fails and never fabricates it in finalization', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const providerEvidence = {
      networkTurnCompleted: true,
      threadProviderBound: true,
      threadModelBound: true,
      resultTurnProviderReceiptBound: true,
      providerAttemptTelemetryValid: true,
      providerAttemptReceiptDigest: 'a'.repeat(64),
      providerLogicalCallCount: 1,
      providerAttemptCount: 1,
      successfulProviderAttemptCount: 1
    }
    const report = finalizeMilestoneAReport({
      provider: {
        configured: true,
        modelHash: 'b'.repeat(64),
        ordinaryEvidence: Object.freeze({ ...providerEvidence }),
        ...providerEvidence
      },
      workflow: { ordinaryWorkflowCompleted: true },
      checks: [
        { id: 'ordinary-agent-workflow', status: 'passed' },
        { id: 'long-context-continuation', status: 'failed', message: 'continuation_failed' }
      ]
    }, { requiredCheckIds: ['ordinary-agent-workflow', 'long-context-continuation'] })
    expect(report.provider).toEqual(expect.objectContaining(providerEvidence))
    expect(report.provider.ordinaryEvidence).toEqual(providerEvidence)
    const noProvider = finalizeMilestoneAReport({
      provider: {},
      checks: [{ id: 'ordinary-agent-workflow', status: 'passed' }]
    }, { requiredCheckIds: ['ordinary-agent-workflow'] })
    expect(noProvider.provider.networkTurnCompleted).not.toBe(true)
    expect(noProvider.provider.providerAttemptReceiptDigest || '').not.toMatch(/^[0-9a-f]{64}$/)
  })

  it('preserves completed continuation safety fields when the continuation check fails', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const acceptedFinalDigest = 'a'.repeat(64)
    const providerClosureDigest = 'b'.repeat(64)
    const report = finalizeMilestoneAReport({
      workflow: {
        longContextContinuationObserved: false,
        longContextContinuationReasonCode: 'provider_closure_invalid',
        longContextContinuationFailureCode: 'continuation_provider_closure_invalid',
        longContextAcceptedFinalBound: true,
        longContextAcceptedFinalDigest: acceptedFinalDigest,
        longContextProviderClosureDigest: providerClosureDigest,
        longContextCaseWorkspaceScopeBound: true
      },
      checks: [
        { id: 'ordinary-agent-workflow', status: 'passed' },
        { id: 'long-context-continuation', status: 'failed', message: 'continuation_provider_closure_invalid' }
      ],
      stageSnapshots: [{
        stage: 'long-context-continuation',
        checkIds: ['long-context-continuation'],
        completedFields: {
          longContextAcceptedFinalBound: true,
          longContextAcceptedFinalDigest: acceptedFinalDigest,
          longContextProviderClosureDigest: providerClosureDigest,
          longContextCaseWorkspaceScopeBound: true
        }
      }]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow', 'long-context-continuation']
    })

    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'ordinary-agent-workflow', status: 'passed' }),
      expect.objectContaining({ id: 'long-context-continuation', status: 'failed' })
    ]))
    expect(report.workflow).toEqual(expect.objectContaining({
      ordinaryWorkflowCompleted: true,
      longContextContinuationObserved: false,
      longContextAcceptedFinalBound: true,
      longContextAcceptedFinalDigest: acceptedFinalDigest,
      longContextProviderClosureDigest: providerClosureDigest,
      longContextCaseWorkspaceScopeBound: true,
      longContextContinuationFailureCode: 'continuation_provider_closure_invalid'
    }))
    const snapshot = report.stageSnapshots.find((item: Record<string, any>) =>
      item.stage === 'long-context-continuation'
    )
    expect(snapshot?.completedFields).toEqual(expect.objectContaining({
      longContextAcceptedFinalBound: true,
      longContextAcceptedFinalDigest: acceptedFinalDigest,
      longContextProviderClosureDigest: providerClosureDigest,
      longContextCaseWorkspaceScopeBound: true
    }))
  })

  it('keeps exact terminal-turn workflow evidence after a continuation receipt replay', async () => {
    const {
      workflowEvidence,
      exactLatestTerminalProviderReceiptScope,
      providerTurnEvidence
    } = await milestoneModule()
    const threadId = 'thread-continuation-replay'
    const turnId = 'turn-continuation-replay'
    const observation = {
      threadId,
      turnCount: 2,
      thread: {
        id: threadId,
        providerId: 'managed-provider',
        model: 'managed-model',
        workspace: '/isolated/repository',
        latestSeq: 18,
        turns: [
          { id: 'turn-before', status: 'completed', items: [] },
          {
            id: turnId,
            status: 'completed',
            items: [{
              kind: 'assistant_text',
              status: 'completed',
              text: 'ordinary repository read marker'
            }]
          }
        ],
        todos: { items: [] }
      },
      summary: { subagents: [] },
      providerAttempts: [{
        kind: 'usage',
        seq: 17,
        threadId,
        turnId,
        model: 'managed-model',
        usageFinalStatus: 'completed',
        promptTokens: 10,
        completionTokens: 4,
        totalTokens: 14,
        turns: 1,
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 1,
        providerAttemptCount: 1,
        providerAttemptStatuses: {
          succeeded: 1,
          failed: 0,
          cancelled: 0,
          timedOut: 0,
          streamAborted: 0
        }
      }],
      providerTerminals: [{
        kind: 'turn_completed',
        seq: 18,
        threadId,
        turnId,
        status: 'completed'
      }],
      providerReceiptTrace: {
        reasonCode: 'provider_receipt_observed',
        started: true,
        acknowledged: true,
        ended: true,
        eventCount: 2
      }
    }
    expect(exactLatestTerminalProviderReceiptScope(observation, {
      previousTurnCount: 1,
      baselineSeq: 10,
      expectedThreadId: threadId
    })).toEqual({ baselineSeq: 10, expectedTurnId: turnId, expectedHighestSeq: 18 })
    const evidence = workflowEvidence(
      observation,
      '/isolated/repository',
      'missing-marker',
      turnId
    )
    expect(evidence.resultTurnId).toBe(turnId)
    expect(evidence.providerAttempts).toHaveLength(1)
    expect(evidence.providerAttempts[0].turnId).toBe(turnId)
    expect(evidence.providerTerminals).toHaveLength(1)
    const providerEvidence = providerTurnEvidence(evidence, {
      ok: true,
      id: 'managed-provider',
      model: 'managed-model'
    })
    expect(providerEvidence).toEqual(expect.objectContaining({
      networkTurnCompleted: true,
      resultTurnProviderReceiptBound: true,
      reasonCode: 'provider_receipt_observed'
    }))
    expect(providerEvidence.providerAttemptReceiptDigest).toMatch(/^[0-9a-f]{64}$/)

    const missingReceipt = providerTurnEvidence({
      ...evidence,
      providerAttempts: [],
      providerTerminals: []
    }, {
      ok: true,
      id: 'managed-provider',
      model: 'managed-model'
    })
    expect(missingReceipt.networkTurnCompleted).toBe(false)
  })

  it('classifies completed continuation evidence gaps as direct failures, not external blocks', async () => {
    const { longContextContinuationFailureCode } = await milestoneModule()
    expect(longContextContinuationFailureCode({
      disposition: 'completed',
      externallyBlocked: false,
      acceptedFinalReasonCode: 'observed',
      hostReadBound: false,
      reasoningEffortBound: true,
      providerReceiptReplayObserved: true,
      repositoryBound: true,
      subagentStateBound: true
    })).toBe('continuation_read_missing')
    expect(longContextContinuationFailureCode({
      disposition: 'completed',
      externallyBlocked: false,
      acceptedFinalReasonCode: 'provider_closure_invalid',
      hostReadBound: true,
      reasoningEffortBound: true,
      providerReceiptReplayObserved: true,
      repositoryBound: true,
      subagentStateBound: true
    })).toBe('continuation_provider_closure_invalid')
    expect(longContextContinuationFailureCode({
      disposition: 'completed',
      externallyBlocked: true,
      acceptedFinalReasonCode: 'observed',
      hostReadBound: false
    })).toBe('external_prerequisite')
    expect(longContextContinuationFailureCode({
      disposition: 'completed',
      externallyBlocked: false,
      acceptedFinalReasonCode: 'observed',
      hostReadBound: true,
      reasoningEffortBound: true,
      providerReceiptReplayObserved: true,
      repositoryBound: true,
      subagentStateBound: true
    })).toBe('')
  })

  it('creates and reuses only a task-owned isolated macOS login keychain', async () => {
    if (process.platform !== 'darwin') return
    const root = taskOwnedSandbox()
    const isolatedHome = join(root, 'home')
    mkdirSync(isolatedHome, { mode: 0o700 })
    chmodSync(isolatedHome, 0o700)
    const { createIsolatedDarwinLoginKeychain } = await milestoneModule()
    const taskKeychain = join(isolatedHome, 'Library', 'Keychains', 'login.keychain')
    const userDefaultBefore = spawnSync('/usr/bin/security', ['default-keychain', '-d', 'user'], {
      encoding: 'utf8'
    })
    expect(userDefaultBefore.status).toBe(0)
    const controller = await createIsolatedDarwinLoginKeychain(isolatedHome)
    try {
      const created = controller.evidence()
      expect(created).toEqual(expect.objectContaining({
        ok: true,
        created: true,
        directoriesOwnerOnly: true,
        defaultKeychainBound: true,
        unlockCount: 0,
        reusedForTwoLaunches: false,
        passwordTransport: 'process-memory-to-stdin-to-task-owned-pty',
        passwordInArgv: false,
        passwordInEnvironment: false,
        passwordInFile: false,
        passwordRecorded: false
      }))
      expect(existsSync(join(
        isolatedHome,
        'Library',
        'Keychains',
        'login.keychain-db'
      ))).toBe(true)

      const firstLaunch = await controller.unlockForLaunch()
      expect(firstLaunch).toEqual(expect.objectContaining({
        ok: true,
        unlockCount: 1,
        reusedForTwoLaunches: false
      }))
      const secondLaunch = await controller.unlockForLaunch()
      expect(secondLaunch).toEqual(expect.objectContaining({
        ok: true,
        unlockCount: 2,
        reusedForTwoLaunches: true
      }))
      expect(secondLaunch.requestedPathHash).toBe(created.requestedPathHash)
      expect(secondLaunch.databasePathHash).toBe(created.databasePathHash)
      expect(secondLaunch.observedDefaultPathHash).toBe(created.observedDefaultPathHash)
    } finally {
      controller.dispose()
    }
    expect(existsSync(`${taskKeychain}-db`)).toBe(false)
    const searchListAfter = spawnSync('/usr/bin/security', ['list-keychains', '-d', 'user'], {
      encoding: 'utf8'
    })
    const userDefaultAfter = spawnSync('/usr/bin/security', ['default-keychain', '-d', 'user'], {
      encoding: 'utf8'
    })
    expect(searchListAfter.status).toBe(0)
    expect(searchListAfter.stdout).not.toContain(taskKeychain)
    expect(userDefaultAfter.status).toBe(0)
    expect(userDefaultAfter.stdout).toBe(userDefaultBefore.stdout)
  })

  it('provisions the non-login Core task keychain without changing user Keychain selection', async () => {
    if (process.platform !== 'darwin') return
    const taskRoot = taskOwnedSandbox()
    const homeRoot = join(taskRoot, 'home')
    mkdirSync(homeRoot, { mode: 0o700 })
    const snapshot = () => ({
      defaultKeychain: spawnSync('/usr/bin/security', ['default-keychain', '-d', 'user'], { encoding: 'utf8' }),
      searchList: spawnSync('/usr/bin/security', ['list-keychains', '-d', 'user'], { encoding: 'utf8' })
    })
    const before = snapshot()
    expect(before.defaultKeychain.status).toBe(0)
    expect(before.searchList.status).toBe(0)
    const { createIsolatedDarwinTaskKeychain } = await milestoneModule()
    const controller = await createIsolatedDarwinTaskKeychain(taskRoot, homeRoot)
    try {
      expect(existsSync(join(taskRoot, 'darwin-secret-store-keychain', 'analytix-task.keychain-db'))).toBe(true)
      await controller.unlockForLaunch()
    } finally {
      controller.dispose()
    }
    const after = snapshot()
    expect(after.defaultKeychain.status).toBe(0)
    expect(after.searchList.status).toBe(0)
    expect(after.defaultKeychain.stdout).toBe(before.defaultKeychain.stdout)
    expect(after.searchList.stdout).toBe(before.searchList.stdout)
  })

  it('keeps the isolated keychain password out of argv, env, files, and reports', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const guiSmoke = source('scripts/runtime-go-packaged-gui-smoke.mjs')
    const keychainStart = script.indexOf('function createRandomHexPasswordBuffer')
    const keychainEnd = script.indexOf(
      'function privateCaseAuthorityInputEvidence',
      keychainStart
    )
    const keychainSource = script.slice(keychainStart, keychainEnd)
    const formalRunStart = script.indexOf('export async function runMilestoneA(')
    const formalRun = script.slice(
      formalRunStart,
      script.indexOf('async function main()', formalRunStart)
    )

    expect(script).toContain("const DARWIN_EXPECT_PATH = '/usr/bin/expect'")
    expect(script).toContain("const DARWIN_SECURITY_PATH = '/usr/bin/security'")
    expect(keychainSource).toContain('set secret [gets stdin]')
    expect(keychainSource).toContain("stdio: ['pipe', 'ignore', 'ignore']")
    expect(keychainSource).toContain('child.stdin.write(password)')
    expect(keychainSource).toContain("operation: 'create-keychain'")
    expect(keychainSource).toContain("operation: 'unlock-keychain'")
    expect(keychainSource).not.toContain("'-p'")
    expect(keychainSource).not.toContain("'-s'")
    expect(keychainSource).not.toContain('list-keychains')
    expect(keychainSource).not.toContain('login-keychain')
    expect(keychainSource).not.toContain('use-mock-keychain')
    expect(keychainSource).not.toContain('password-store')
    expect(keychainSource).not.toContain('EnableCookieEncryption')
    expect(formalRun.match(/isolatedLoginKeychain\.unlockForLaunch\(\)/gu)).toHaveLength(2)
    expect(formalRun.indexOf('await stopExactTaskOwnedProcesses(sandboxRoot)')).toBeLessThan(
      formalRun.indexOf('isolatedLoginKeychain.dispose()')
    )
    expect(formalRun.indexOf('isolatedLoginKeychain.dispose()')).toBeLessThan(
      formalRun.indexOf('rmSync(sandboxRoot, { recursive: true, force: true })')
    )
    expect(guiSmoke).toContain("from './runtime-go-packaged-milestone-a.mjs'")
    expect(guiSmoke).toContain('await createIsolatedDarwinLoginKeychain(tempHome)')
    expect(guiSmoke).toContain('await isolatedLoginKeychain.unlockForLaunch()')
    expect(guiSmoke).toContain(
      'mkdirSync(dirname(userDataDir), { recursive: true, mode: 0o700 })'
    )
    expect(guiSmoke).toContain('MAC_CHROMIUM_TMPDIR: chromiumTempDir')
    expect(guiSmoke).toContain("const PACKAGED_RENDERER_SCHEME = 'analytix-app:'")
    expect(guiSmoke).toContain('exact packaged renderer target unavailable')
    expect(guiSmoke).toContain('renderer?.packagedRendererUrlExact === true')
    expect(guiSmoke).toContain('renderer?.fileRendererProtocolUsed === false')
    expect(guiSmoke).toContain('parsedInfo.schemaVersion === 2')
    expect(guiSmoke).toContain(
      "!Object.prototype.hasOwnProperty.call(parsedInfo, 'dataDir')"
    )
    expect(guiSmoke).toContain('isolatedLaunchAuthorityReady')
    expect(script).toContain('TMPDIR: chromiumTempDir')
    expect(script).toContain('TEMP: chromiumTempDir')
    expect(script).toContain('TMP: chromiumTempDir')
    expect(script).toContain('MAC_CHROMIUM_TMPDIR: chromiumTempDir')
    expect(script).toContain('chromiumTempBoundToTrustedCache:')
    expect(guiSmoke.indexOf('await isolatedLoginKeychain.unlockForLaunch()')).toBeLessThan(
      guiSmoke.indexOf('child = spawn(')
    )
    expect(guiSmoke.indexOf('mkdirSync(dirname(userDataDir)')).toBeLessThan(
      guiSmoke.indexOf('child = spawn(')
    )
    expect(guiSmoke.indexOf('await stopChild(child)')).toBeLessThan(
      guiSmoke.indexOf('isolatedLoginKeychain?.dispose()')
    )
    expect(guiSmoke.indexOf('isolatedLoginKeychain?.dispose()')).toBeLessThan(
      guiSmoke.indexOf('rmSync(tempHome, { recursive: true, force: true })')
    )
  })

  it('is a distinct validation command and package entry', () => {
    expect(source('scripts/runtime-go-validation-command.mjs')).toContain(
      "'packaged-milestone-a': { script: 'runtime-go-packaged-milestone-a.mjs' }"
    )
    expect(JSON.parse(source('package.json')).scripts).toEqual(expect.objectContaining({
      'runtime:go:packaged-milestone-a':
        'node ./scripts/runtime-go-validation-command.mjs packaged-milestone-a'
    }))
    expect(source('scripts/runtime-go-packaged-qa.mjs')).toContain("id: 'milestone-a'")
    expect(source('scripts/runtime-go-live-evidence-collector.mjs')).toContain(
      'packagedGeneralAgentMilestoneA'
    )
  })

  it('uses the real composer through CDP Input while keeping Runtime.evaluate read-only', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const preload = source('src/preload/index.ts')
    const sharedApi = source('src/shared/analytix-api.ts')
    const observationStart = script.indexOf('function readonlyObservationExpression')
    const observationEnd = script.indexOf('async function observeRenderer', observationStart)
    const observation = script.slice(observationStart, observationEnd)
    const sseObserverStart = script.indexOf(
      'export async function observeProviderTerminalViaSse'
    )
    const sseObserver = script.slice(sseObserverStart, observationStart)
    const providerFailureObserverStart = script.indexOf(
      'export async function observeProviderFailureDiagnosticViaSse'
    )
    const providerFailureObserverEnd = script.indexOf(
      'function providerFailureDiagnosticObservationExpression',
      providerFailureObserverStart
    )
    const providerFailureObserver = script.slice(
      providerFailureObserverStart,
      providerFailureObserverEnd
    )

    expect(script).toContain("method: 'Input.dispatchMouseEvent'")
    expect(script).toContain("method: 'Input.insertText'")
    expect(script).toContain("method: 'Input.dispatchKeyEvent'")
    expect(script).toContain("method: 'Page.captureScreenshot'")
    expect(script).toContain("document.querySelectorAll('.ProseMirror[contenteditable=\"true\"]')")
    expect(script).toContain("composer.querySelector('.ds-composer-primary-action-button')")
    expect(script).not.toContain('.click(')
    expect(sharedApi).toContain(`| '${FORMAL_RUNTIME_REQUEST_METHOD}'`)
    expect(preload).toMatch(new RegExp(
      `runtime:\\s*\\{[\\s\\S]*?${FORMAL_RUNTIME_REQUEST_METHOD}:\\s*flatApi\\.${FORMAL_RUNTIME_REQUEST_METHOD}`
    ))
    expect(observation).toContain(
      `api.runtime.${FORMAL_RUNTIME_REQUEST_METHOD}(path, 'GET')`
    )
    expect(script).not.toContain('api.runtime.request')
    expect(observation).toContain("runtimeJSON('/health')")
    expect(observation).toContain("runtimeJSON('/v1/runtime/info')")
    expect(observation).toContain("runtimeJSON('/v1/runtime/tools')")
    expect(observation).toContain(
      'const observeProviderTerminalViaSse = ${observeProviderTerminalViaSse.toString()}'
    )
    expect(observation).toContain('await observeProviderTerminalViaSse(')
    expect(observation).toContain('providerReceiptScope.baselineSeq')
    expect(observation).toContain('providerReceiptScope.expectedTurnId')
    expect(observation).toContain('providerReceiptScope.expectedHighestSeq')
    expect(sseObserver).toContain('api.onSseEvent')
    expect(sseObserver).toContain('api.onSseError')
    expect(sseObserver).toContain('api.onSseEnd')
    expect(sseObserver).toContain('api.startSse(exactThreadId, baselineSeq, streamId)')
    expect(sseObserver).toContain('api.ackSseEvent(streamId, batchMaxSeq)')
    expect(sseObserver).toContain("event.kind === 'general_terminal_batch'")
    expect(sseObserver).toContain("event.kind === 'accepted_final_batch'")
    expect(sseObserver).toContain('batch.seq > expectedHighestSeq')
    expect(sseObserver).toContain('api.stopSse(streamId)')
    const workflowReplayStart = script.indexOf('const agentReceiptScope =')
    const workflowReplayEnd = script.indexOf('const groups =', workflowReplayStart)
    const workflowReplay = script.slice(workflowReplayStart, workflowReplayEnd)
    expect(workflowReplayStart).toBeGreaterThan(0)
    expect(workflowReplay).toContain('exactLatestTerminalProviderReceiptScope(firstEvidence')
    expect(workflowReplay).toContain('previousTurnCount: agentBaselineEvidence.turnCount')
    expect(workflowReplay).toContain('providerReceiptScope: agentReceiptScope')
    expect(workflowReplay).not.toContain('expectedTurnId: firstEvidence.resultTurnId')
    expect(providerFailureObserver).toContain(
      'api.startSse(exactThreadId, baselineSeq, streamId)'
    )
    expect(providerFailureObserver).toContain('await api.ackSseEvent(streamId, batchMaxSeq)')
    expect(providerFailureObserver).toContain("event.stage !== 'provider_error'")
    expect(providerFailureObserver).toContain("event.stage === 'provider_admission_rejected'")
    expect(providerFailureObserver).toContain('event.details.providerAttemptCount === 0')
    expect(providerFailureObserver).toContain('priorCompletedTurnId')
    expect(providerFailureObserver).toContain('exactPriorCompletedTurnId')
    expect(providerFailureObserver).toContain('context_window_hard_limit')
    expect(providerFailureObserver).toContain('event.seq <= expectedHighestSeq')
    expect(providerFailureObserver).toContain("batch.kind !== 'general_terminal_batch'")
    expect(providerFailureObserver).not.toContain('since_seq=0')
    expect(observation).not.toContain('/events?since_seq=0')
    expect(sseObserver).not.toContain('/events?since_seq=0')
    expect(observation).toContain("api.providerRegistry.request({ schemaVersion: 1, operation: 'list' })")
    expect(observation).not.toContain('api.account.getSnapshot()')
    expect(observation).toContain("api.settings.getSettings()")
    expect(observation).not.toContain("'POST'")
    expect(observation).not.toContain('createTurn')
    const hostObservationStart = script.indexOf('function hostToolExecutionObservationExpression')
    const hostObservationEnd = script.indexOf(
      'export function hostToolExecutionObservationEvidence',
      hostObservationStart
    )
    const hostObservation = script.slice(hostObservationStart, hostObservationEnd)
    expect(hostObservation).toContain('TOOL_EXECUTION_OBSERVATION_PATH')
    expect(hostObservation).toContain("'POST'")
    expect(hostObservation).toContain('JSON.stringify(request)')
    const publicParserStart = script.indexOf(
      'export function hostToolExecutionObservationEvidence'
    )
    const publicParserEnd = script.indexOf(
      'function canonicalHostToolExecutionArguments',
      publicParserStart
    )
    const publicParser = script.slice(publicParserStart, publicParserEnd)
    const privateObserverStart = script.indexOf('async function observeHostToolExecution')
    const privateObserverEnd = script.indexOf('function processExists', privateObserverStart)
    const privateObserver = script.slice(privateObserverStart, privateObserverEnd)
    expect(script).toContain('const hostToolExecutionPrivateBindings = new WeakMap()')
    expect(publicParser).not.toContain('hostToolExecutionPrivateBindings.set')
    expect(privateObserver).toContain('hostToolExecutionPrivateBindings.set')
    expect(script).not.toContain('export async function observeHostToolExecution')
    expect(script).not.toContain('export function privateHostToolExecutionBindingMatches')
    expect(script).toContain('directRuntimeTurnDriverUsed: false')
    expect(script).toContain('cdpAndBridgeObservationOnly: true')
  })

  it('observes an exact child thread through its bound detail route without list rediscovery', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const observationStart = script.indexOf('function readonlyObservationExpression')
    const observationEnd = script.indexOf('async function observeRenderer', observationStart)
    const observation = script.slice(observationStart, observationEnd)

    expect(observation).toContain("const exactThread = exact\n      ? await runtimeJSON('/v1/threads/' + encodeURIComponent(exact))")
    expect(observation).toContain("if (exact && exactThread?.id !== exact) return result")
    expect(observation).toContain("const selectedId = exact || selected?.id || ''")
    expect(observation).toContain(
      'readonlyThreadWorkspaceScopeMatches(result.thread, selectedId, workspace)'
    )
    expect(observation).toContain("'/v1/threads/' + encodeURIComponent(selectedId) + '/summary'")
    expect(observation).toContain('await observeProviderTerminalViaSse(\n        api.runtime,\n        selectedId,')
    expect(observation).not.toContain("threads.find((item) => item && item.id === exact)")
  })

  it('closes readonly exact-thread workspace scope for ordinary and case public projections', async () => {
    const { readonlyThreadWorkspaceScopeMatches } = await milestoneModule()
    const workspace = '/isolated/repository'
    const caseThreadId = 'thread-case-bound-readonly'
    const ordinaryThreadId = 'thread-ordinary-readonly'
    const caseThread = {
      id: caseThreadId,
      historyAuthority: 'case_boundary_only_v1'
    }
    expect(readonlyThreadWorkspaceScopeMatches(caseThread, caseThreadId, workspace)).toBe(true)
    expect(readonlyThreadWorkspaceScopeMatches(caseThread, 'foreign-thread', workspace)).toBe(false)
    expect(readonlyThreadWorkspaceScopeMatches({
      ...caseThread,
      workspace
    }, caseThreadId, workspace)).toBe(false)

    const ordinaryThread = { id: ordinaryThreadId, workspace }
    expect(readonlyThreadWorkspaceScopeMatches(ordinaryThread, ordinaryThreadId, workspace))
      .toBe(true)
    expect(readonlyThreadWorkspaceScopeMatches(
      { id: ordinaryThreadId },
      ordinaryThreadId,
      workspace
    )).toBe(false)
    expect(readonlyThreadWorkspaceScopeMatches(
      { id: ordinaryThreadId, workspace: '/foreign/repository' },
      ordinaryThreadId,
      workspace
    )).toBe(false)
  })

  it('injects readonly case scope before summary and accepted-final replay', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const observationStart = script.indexOf('function readonlyObservationExpression')
    const observationEnd = script.indexOf('async function observeRenderer', observationStart)
    const observation = script.slice(observationStart, observationEnd)
    expect(script).toContain('export function readonlyThreadWorkspaceScopeMatches')
    expect(observation).toContain(
      'const readonlyThreadWorkspaceScopeMatches = ${readonlyThreadWorkspaceScopeMatches.toString()};'
    )
    const scopeGate = observation.indexOf(
      'if (!readonlyThreadWorkspaceScopeMatches(result.thread, selectedId, workspace)) return result'
    )
    const summaryFetch = observation.indexOf('result.summary = responseBody(', scopeGate)
    const acceptedFinalReplay = observation.indexOf(
      'await observeAcceptedFinalOrdinaryViaSse(',
      summaryFetch
    )
    expect(scopeGate).toBeGreaterThan(0)
    expect(summaryFetch).toBeGreaterThan(scopeGate)
    expect(acceptedFinalReplay).toBeGreaterThan(summaryFetch)
    expect(observation).not.toContain('result.thread?.workspace !== workspace')

    const continuationFailure = script.indexOf(
      'const longContextContinuationFailure = longContextContinuationFailureCode({'
    )
    const continuationField = script.indexOf(
      'longContextAcceptedFinalDigest: contextAcceptedFinalEvidence.acceptedFinalDigest',
      continuationFailure
    )
    const continuationThrow = script.indexOf(
      "if (!longContextContinuationObserved) throw new Error('packaged_long_context_continuation_failed')",
      continuationFailure
    )
    const continuationSnapshot = script.indexOf(
      "'long-context-continuation',\n      ['long-context-continuation']",
      continuationField
    )
    expect(continuationFailure).toBeGreaterThan(0)
    expect(continuationField).toBeGreaterThan(continuationFailure)
    expect(continuationField).toBeLessThan(continuationThrow)
    expect(continuationSnapshot).toBeGreaterThan(continuationField)
    expect(continuationSnapshot).toBeLessThan(continuationThrow)
    expect(script.slice(continuationFailure, continuationThrow)).toContain(
      'longContextCaseWorkspaceScopeBound'
    )
  })

  it('waits for the idle same-shell composer before inserting the next prompt', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const geometryStart = script.indexOf('function readonlyComposerGeometryExpression')
    const geometryEnd = script.indexOf('async function readComposerGeometry', geometryStart)
    const geometry = script.slice(geometryStart, geometryEnd)
    const idleEvidenceStart = script.indexOf('export function composerPrimaryIdleEvidence')
    const idleEvidenceEnd = script.indexOf('async function readComposerGeometry', idleEvidenceStart)
    const idleEvidence = script.slice(idleEvidenceStart, idleEvidenceEnd)
    const submitStart = script.indexOf('async function cdpComposerSubmit')
    const submitEnd = script.indexOf('function readonlyRecoveredTimelineExpression', submitStart)
    const submit = script.slice(submitStart, submitEnd)
    const insertText = submit.indexOf("method: 'Input.insertText'")
    const readyEnabled = submit.indexOf('buttonDisabled === false', insertText)
    const readyLabelAccepted = submit.indexOf('buttonLabelAccepted === true', insertText)

    expect(geometry).toContain("document.querySelectorAll('.ProseMirror[contenteditable=\"true\"]')")
    expect(geometry).toContain("editor.closest('.ds-composer-shell')")
    expect(geometry).toContain("composer.querySelector('.ds-composer-primary-action-button')")
    expect(geometry).toContain("button.querySelector('.ds-composer-primary-action-icon.animate-spin')")
    expect(geometry).toContain('editorTextLength')
    expect(idleEvidence).toContain('editorEmpty: value?.editorTextLength === 0')
    expect(idleEvidence).toContain('buttonNotLoading: value?.buttonLoading === false')
    expect(idleEvidence).toContain('buttonDisabled: value?.buttonDisabled === true')
    expect(idleEvidence).toContain('buttonLabelAccepted: value?.buttonLabelAccepted === true')
    expect(submit).toContain('composerPrimaryIdleEvidence(observed.value).ok')
    expect(submit.indexOf('composerPrimaryIdleEvidence(observed.value).ok'))
      .toBeLessThan(insertText)
    expect(submit).toContain('idleFailure: composerIdleFailureDiagnostic(')
    const readyText = submit.indexOf('editorTextLength > 0', insertText)
    const readyNotLoading = submit.indexOf('buttonLoading === false', insertText)
    expect(readyText).toBeGreaterThan(insertText)
    expect(readyText).toBeLessThan(readyNotLoading)
    expect(readyNotLoading).toBeLessThan(readyEnabled)
    expect(readyEnabled).toBeGreaterThan(insertText)
    expect(readyEnabled).toBeLessThan(readyLabelAccepted)
    expect(submit).toContain('idleTimeoutMs = 10_000')
    expect(submit).toContain('const idleDeadline = Date.now() + idleTimeoutMs')
    expect(submit).toContain('const readyDeadline = Date.now() + 10_000')
    expect(script).toContain("contextPrompt,\n      ['Send'],\n      Math.min(timeoutMs, 60_000)")
    expect(submit).toContain("'composer_primary_button_not_idle'")
    expect(submit).toContain("'composer_input_not_observed'")
  })

  it('classifies composer idle failures without exposing labels or editor text', async () => {
    const { composerPrimaryIdleEvidence } = await milestoneModule()
    const idle = {
      editor: { x: 1, y: 1 },
      button: { x: 2, y: 2 },
      editorTextLength: 0,
      buttonDisabled: true,
      buttonLoading: false,
      buttonLabelAccepted: true
    }
    expect(composerPrimaryIdleEvidence(idle)).toEqual({
      ok: true,
      blocker: '',
      editorObserved: true,
      buttonObserved: true,
      editorEmpty: true,
      buttonNotLoading: true,
      buttonDisabled: true,
      buttonLabelAccepted: true
    })
    expect(composerPrimaryIdleEvidence({ ...idle, buttonDisabled: false }).blocker)
      .toBe('composer_primary_button_busy')
    expect(composerPrimaryIdleEvidence({ ...idle, buttonLoading: true }).blocker)
      .toBe('composer_runtime_not_ready')
    expect(composerPrimaryIdleEvidence({ ...idle, buttonLabelAccepted: false }).blocker)
      .toBe('composer_primary_button_label_not_idle')
    expect(composerPrimaryIdleEvidence({ ...idle, editorTextLength: 1 }).blocker)
      .toBe('composer_editor_not_empty')
    expect(composerPrimaryIdleEvidence({ ...idle, editor: null }).blocker)
      .toBe('composer_dom_element_missing')
    expect(JSON.stringify(composerPrimaryIdleEvidence({
      ...idle,
      buttonDisabled: false,
      buttonLabel: 'PRIVATE_LABEL',
      editorText: 'PRIVATE_PROMPT'
    }))).not.toContain('PRIVATE')
  })

  it('projects an immutable idle-timeout diagnostic with only safe booleans and elapsed milliseconds', async () => {
    const { composerIdleFailureDiagnostic } = await milestoneModule()
    const diagnostic = composerIdleFailureDiagnostic({
      editor: { x: 1, y: 1 },
      button: { x: 2, y: 2 },
      editorTextLength: 0,
      buttonLoading: true,
      buttonDisabled: true,
      buttonLabelAccepted: true,
      buttonLabel: 'PRIVATE_LABEL',
      editorText: 'PRIVATE_PROMPT'
    }, 1234)

    expect(diagnostic).toEqual({
      editorObserved: true,
      buttonObserved: true,
      editorEmpty: true,
      buttonNotLoading: false,
      buttonDisabled: true,
      buttonLabelAccepted: true,
      waitedMs: 1234
    })
    expect(Object.isFrozen(diagnostic)).toBe(true)
    expect(JSON.stringify(diagnostic)).not.toContain('PRIVATE')
  })

  it('retains long-context idle/runtime health diagnostics in immutable stage snapshots without changing check status', async () => {
    const { finalizeMilestoneAReport, sanitizeMilestoneAStageSnapshot } = await milestoneModule()
    const completedFields = {
      longContextIdleFailureObserved: true,
      longContextIdleFailureEditorObserved: true,
      longContextIdleFailureButtonObserved: true,
      longContextIdleFailureEditorEmptyObserved: true,
      longContextIdleFailureButtonNotLoadingObserved: false,
      longContextIdleFailureButtonDisabledObserved: true,
      longContextIdleFailureButtonLabelAcceptedObserved: true,
      longContextIdleFailureWaitedMs: 1234,
      longContextIdleFailureHealthOk: true,
      longContextIdleFailureRuntimeInfoOk: true,
      longContextIdleFailureRuntimeToolsOk: false,
      longContextIdleFailureRuntimeThreadListOk: false,
      longContextIdleFailureRuntimeThreadListStatus: 502,
      longContextIdleFailureRuntimeBackendTopologyOk: true,
      longContextIdleFailureRuntimeListenerCount: 1,
      longContextIdleFailureRuntimeBackendProcessCount: 1,
      longContextIdleFailureRuntimeListenerOwnedObserved: true,
      longContextIdleFailureExactPackageExecutableObserved: true,
      longContextIdleFailureRendererTargetCount: 1
    }
    const snapshot = sanitizeMilestoneAStageSnapshot({
      stage: 'long-context-continuation',
      checkIds: ['long-context-continuation'],
      completedFields: {
        ...completedFields,
        unsafePrompt: 'PRIVATE_PROMPT',
        unsafeOk: true,
        unsafeMs: 99
      }
    })
    expect(snapshot).not.toBeNull()
    expect(snapshot?.completedFields).toEqual(completedFields)
    expect(Object.isFrozen(snapshot)).toBe(true)
    expect(Object.isFrozen(snapshot?.completedFields)).toBe(true)
    const invalidStatusSnapshot = sanitizeMilestoneAStageSnapshot({
      stage: 'long-context-continuation',
      checkIds: ['long-context-continuation'],
      completedFields: {
        longContextIdleFailureRuntimeThreadListStatus: 600
      }
    })
    expect(invalidStatusSnapshot?.completedFields)
      .not.toHaveProperty('longContextIdleFailureRuntimeThreadListStatus')

    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      executionBlocker: 'composer_runtime_not_ready',
      workflow: {
        ...completedFields,
        longContextContinuationObserved: false
      },
      checks: [
        { id: 'long-context-continuation', status: 'failed', message: 'composer_runtime_not_ready' }
      ],
      stageSnapshots: [snapshot]
    }, { requiredCheckIds: ['long-context-continuation'] })

    expect(report.status).toBe('failed')
    expect(report.passed).toBe(false)
    expect(report.failedCheckIds).toEqual(['long-context-continuation'])
    expect(report.workflow).toEqual(expect.objectContaining(completedFields))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'long-context-continuation',
        completedFields: expect.objectContaining(completedFields)
      })
    ]))
    expect(JSON.stringify(report)).not.toContain('PRIVATE_PROMPT')
  })

  it('captures the long-context idle failure snapshot before the failed check exits the stage', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const failureBranch = script.indexOf('if (!contextSubmit.ok) {')
    const workflowProjection = script.indexOf(
      '...longContextIdleFailureWorkflowFields({',
      failureBranch
    )
    const snapshot = script.indexOf(
      "'long-context-idle-failure',\n          ['long-context-continuation']",
      workflowProjection
    )
    const failedCheck = script.indexOf(
      "report.checks.push(check(\n        'long-context-continuation'",
      workflowProjection
    )
    const exit = script.indexOf(
      'throw Object.assign(new Error(contextSubmit.blocker)',
      failedCheck
    )

    expect(failureBranch).toBeGreaterThan(0)
    expect(workflowProjection).toBeGreaterThan(failureBranch)
    expect(snapshot).toBeGreaterThan(workflowProjection)
    expect(failedCheck).toBeGreaterThan(snapshot)
    expect(exit).toBeGreaterThan(failedCheck)
  })

  it('binds the exact ordinary terminal when a later child event owns thread latest seq', async () => {
    const { observeProviderTerminalViaSse } = await milestoneModule()
    const threadId = 'thread-provider-receipt'
    const turnId = 'turn-provider-receipt'
    const terminalSeq = 42
    const highestSeq = terminalSeq + 1
    const baselineSeq = terminalSeq - 4
    const replaySeq = highestSeq - 3
    const timestamp = '2026-08-13T02:00:00Z'
    const ordinaryText = 'PRIVATE_PROVIDER_BODY_MUST_NOT_PROJECT'
    const batch = {
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      kind: 'general_terminal_batch',
      batchDigest: 'a'.repeat(64),
      threadId,
      turnId,
      seq: terminalSeq,
      firstSeq: terminalSeq - 2,
      lastSeq: terminalSeq,
      timestamp,
      generalTerminalCommitId: 'b'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: 'c'.repeat(64),
      eventManifestDigest: 'd'.repeat(64),
      projectedEventsDigest: 'e'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'item_completed',
        seq: terminalSeq - 2,
        timestamp,
        threadId,
        turnId,
        itemId: 'item-provider-receipt',
        item: {
          id: 'item-provider-receipt',
          threadId,
          turnId,
          role: 'assistant',
          status: 'completed',
          createdAt: timestamp,
          finishedAt: timestamp,
          kind: 'assistant_text',
          text: ordinaryText,
          ordinaryResult: {
            schemaVersion: 1,
            purpose: 'analytix.ordinary-result/v1',
            projectionVersion: 'analytix.ordinary-output-projection/v1',
            logicalEffect: 'ordinary',
            ordinaryWork: true,
            candidateOrigin: 'provider_ordinary_only',
            evidenceAuthority: false,
            citationAuthority: false,
            factAnswerAllowed: false,
            text: ordinaryText,
            textSha256: '1'.repeat(64),
            resultDigest: '2'.repeat(64)
          }
        }
      }, {
        kind: 'usage',
        seq: terminalSeq - 1,
        timestamp,
        threadId,
        turnId,
        model: 'deepseek-chat',
        usageFinalStatus: 'completed',
        usage: {
          promptTokens: 101,
          completionTokens: 23,
          totalTokens: 124,
          turns: 1
        },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 2,
          providerAttemptCount: 3,
          providerAttemptStatuses: {
            succeeded: 2,
            failed: 1,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          },
          privateDiagnostic: 'PRIVATE_DIAGNOSTIC_MUST_NOT_PROJECT'
        }
      }, {
        kind: 'turn_completed',
        seq: terminalSeq,
        timestamp,
        threadId,
        turnId,
        status: 'completed',
        privateError: 'PRIVATE_TERMINAL_MUST_NOT_PROJECT'
      }],
      eventManifest: [{
        slot: 'terminal-item',
        eventId: '3'.repeat(64),
        payloadDigest: '4'.repeat(64)
      }, {
        slot: 'usage',
        eventId: '5'.repeat(64),
        payloadDigest: '6'.repeat(64)
      }, {
        slot: 'terminal',
        eventId: '7'.repeat(64),
        payloadDigest: '8'.repeat(64)
      }]
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const order: string[] = []
    const cleanup = [vi.fn(), vi.fn(), vi.fn()]
    const startSse = vi.fn(async (
      observedThreadId: string,
      sinceSeq: number,
      streamId: string
    ) => {
      order.push(`start:${observedThreadId}:${sinceSeq}`)
      callbacks.event({ streamId, events: [batch] })
      return { streamId }
    })
    const ackSseEvent = vi.fn(async (streamId: string, seq: number) => {
      order.push(`ack:${seq}`)
      if (seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
      return true
    })
    const stopSse = vi.fn(async () => false)
    const runtimeApi = {
      onSseEvent: (handler: (payload: any) => void) => {
        order.push('listen:event')
        callbacks.event = handler
        return cleanup[0]
      },
      onSseEnd: (handler: (payload: any) => void) => {
        order.push('listen:end')
        callbacks.end = handler
        return cleanup[1]
      },
      onSseError: (handler: (payload: any) => void) => {
        order.push('listen:error')
        callbacks.error = handler
        return cleanup[2]
      },
      startSse,
      ackSseEvent,
      stopSse
    }

    const observed = await observeProviderTerminalViaSse(
      runtimeApi,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      100
    )

    expect(order.slice(0, 4)).toEqual([
      'listen:event',
      'listen:end',
      'listen:error',
      `start:${threadId}:${replaySeq}`
    ])
    expect(startSse).toHaveBeenCalledWith(threadId, replaySeq, expect.any(String))
    expect(ackSseEvent.mock.calls.map(([, seq]) => seq)).toEqual([terminalSeq])
    expect(observed).toEqual({
      providerAttempts: [{
        kind: 'usage',
        seq: terminalSeq - 1,
        threadId,
        turnId,
        model: 'deepseek-chat',
        usageFinalStatus: 'completed',
        promptTokens: 101,
        completionTokens: 23,
        totalTokens: 124,
        turns: 1,
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 2,
        providerAttemptCount: 3,
        providerAttemptStatuses: {
          succeeded: 2,
          failed: 1,
          cancelled: 0,
          timedOut: 0,
          streamAborted: 0
        }
      }],
      providerTerminals: [{
        kind: 'turn_completed',
        seq: terminalSeq,
        threadId,
        turnId,
        status: 'completed'
      }],
      ordinaryResultReceipt: {
        observed: true,
        reasonCode: 'ordinary_result_observed',
        candidateOrigin: 'provider_ordinary_only',
        projectionClass: 'provider_ordinary_only',
        resultDigest: '2'.repeat(64),
        textSha256: '1'.repeat(64),
        resultMarkerObserved: false,
        researchMarkerObserved: false,
        writingMarkerObserved: false
      },
      providerReceiptTrace: {
        reasonCode: 'provider_receipt_observed',
        started: true,
        acknowledged: true,
        ended: true,
        eventCount: 3
      }
    })
    expect(JSON.stringify(observed)).not.toContain('PRIVATE_')
    expect(cleanup.every((item) => item.mock.calls.length === 1)).toBe(true)
    expect(stopSse).toHaveBeenCalledTimes(1)
  })

  it('binds a fresh child provider receipt after the live SSE control heartbeat at seq zero', async () => {
    const { observeProviderTerminalViaSse } = await milestoneModule()
    const threadId = 'thread-provider-fresh-child'
    const turnId = 'turn-provider-fresh-child'
    const terminalSeq = 2
    const timestamp = '2026-08-08T00:00:00Z'
    const batch = {
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      kind: 'general_terminal_batch',
      batchDigest: '1'.repeat(64),
      threadId,
      turnId,
      seq: terminalSeq,
      firstSeq: 1,
      lastSeq: terminalSeq,
      timestamp,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: 1,
        timestamp,
        threadId,
        turnId,
        model: 'deepseek-v4-flash',
        usageFinalStatus: 'completed',
        usage: {
          promptTokens: 11,
          completionTokens: 7,
          totalTokens: 18,
          turns: 1
        },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 1,
            failed: 0,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          }
        }
      }, {
        kind: 'turn_completed',
        seq: terminalSeq,
        timestamp,
        threadId,
        turnId,
        status: 'completed'
      }],
      eventManifest: [{
        slot: 'usage', eventId: '6'.repeat(64), payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal', eventId: '8'.repeat(64), payloadDigest: '9'.repeat(64)
      }]
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const ackSseEvent = vi.fn(async (streamId: string, seq: number) => {
      if (seq === 0) {
        queueMicrotask(() => callbacks.event({ streamId, events: [batch] }))
      } else if (seq === terminalSeq) {
        queueMicrotask(() => callbacks.end({ streamId }))
      }
      return true
    })
    const runtimeApi = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse: async (_thread: string, _since: number, streamId: string) => {
        callbacks.event({
          streamId,
          events: [{
            kind: 'heartbeat',
            seq: 0,
            timestamp: '2026-08-08T00:00:00.000Z',
            threadId
          }]
        })
        return { streamId }
      },
      ackSseEvent,
      stopSse: async () => false
    }

    const observed = await observeProviderTerminalViaSse(
      runtimeApi,
      threadId,
      0,
      turnId,
      terminalSeq,
      100
    )

    expect(ackSseEvent.mock.calls.map(([, seq]) => seq)).toEqual([0, terminalSeq])
    expect(observed.providerReceiptTrace).toEqual({
      reasonCode: 'provider_receipt_observed',
      started: true,
      acknowledged: true,
      ended: true,
      eventCount: 2
    })
    expect(observed.providerAttempts).toHaveLength(1)
    expect(observed.providerTerminals).toHaveLength(1)
  })

  it('isolates the target terminal suffix from unrelated ordinary progress replay', async () => {
    const { observeProviderTerminalViaSse } = await milestoneModule()
    const threadId = 'thread-provider-batched-replay'
    const turnId = 'turn-provider-batched-replay'
    const baselineSeq = 10
    const progressEvents = Array.from({ length: 129 }, (_, index) => ({
      kind: 'cursor_advanced',
      seq: baselineSeq + index + 1,
      timestamp: '2026-08-13T00:00:00Z',
      threadId,
      reason: 'restricted_content_removed'
    }))
    const firstTerminalSeq = baselineSeq + progressEvents.length + 1
    const terminalSeq = firstTerminalSeq + 1
    const timestamp = '2026-08-13T00:00:00Z'
    const terminalBatch = {
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      kind: 'general_terminal_batch',
      batchDigest: '1'.repeat(64),
      threadId,
      turnId,
      seq: terminalSeq,
      firstSeq: firstTerminalSeq,
      lastSeq: terminalSeq,
      timestamp,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: firstTerminalSeq,
        timestamp,
        threadId,
        turnId,
        model: 'deepseek-v4-flash',
        usageFinalStatus: 'completed',
        usage: {
          promptTokens: 21,
          completionTokens: 8,
          totalTokens: 29,
          turns: 1
        },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 1,
            failed: 0,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          }
        }
      }, {
        kind: 'turn_completed',
        seq: terminalSeq,
        timestamp,
        threadId,
        turnId,
        status: 'completed'
      }],
      eventManifest: [{
        slot: 'usage', eventId: '6'.repeat(64), payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal', eventId: '8'.repeat(64), payloadDigest: '9'.repeat(64)
      }]
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const ackSseEvent = vi.fn(async (streamId: string, seq: number) => {
      if (seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
      return true
    })
    const startSse = vi.fn(async (_thread: string, _since: number, streamId: string) => {
      callbacks.event({ streamId, events: [terminalBatch] })
      return { streamId }
    })
    const runtimeApi = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse,
      ackSseEvent,
      stopSse: async () => false
    }

    const observed = await observeProviderTerminalViaSse(
      runtimeApi,
      threadId,
      baselineSeq,
      turnId,
      terminalSeq,
      100
    )

    expect(observed.providerReceiptTrace.reasonCode).toBe('provider_receipt_observed')
    expect(observed.providerAttempts).toHaveLength(1)
    expect(observed.providerTerminals).toHaveLength(1)
    expect(startSse).toHaveBeenCalledWith(
      threadId,
      terminalSeq - 3,
      expect.any(String)
    )
    expect(ackSseEvent.mock.calls.map(([, seq]) => seq)).toEqual([terminalSeq])
  })

  it.each([
    [
      {
        code: 'sse_event_rejected',
        message: 'Runtime request failed (sse_event_rejected).',
        reasonCode: 'general_terminal_batch_integrity_invalid'
      },
      'provider_receipt_sse_general_terminal_batch_integrity_invalid'
    ],
    [
      { code: 'sse_event_rejected', message: 'Runtime request failed (sse_event_rejected).' },
      'provider_receipt_stream_event_rejected'
    ],
    [
      { code: 'sse_stream_error', message: 'Runtime request failed (sse_stream_error).' },
      'provider_receipt_stream_transport_error'
    ],
    [
      { code: 'sse_setup_error', message: 'Runtime request failed (sse_setup_error).' },
      'provider_receipt_stream_setup_error'
    ],
    [
      { status: 503 },
      'provider_receipt_stream_http_error'
    ]
  ])('retains the closed SSE failure class for provider receipt diagnosis', async (
    publicError,
    expectedReasonCode
  ) => {
    const { observeProviderTerminalViaSse } = await milestoneModule()
    const callbacks: Record<string, (payload: any) => void> = {}
    const runtimeApi = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse: async (_thread: string, _since: number, streamId: string) => {
        callbacks.error({ streamId, ...publicError })
        return { streamId }
      },
      ackSseEvent: vi.fn(async () => true),
      stopSse: vi.fn(async () => false)
    }

    const observed = await observeProviderTerminalViaSse(
      runtimeApi,
      'thread-provider-stream-error',
      1,
      'turn-provider-stream-error',
      2,
      100
    )

    expect(observed).toEqual({
      providerAttempts: [],
      providerTerminals: [],
      ordinaryResultReceipt: {
        observed: false,
        reasonCode: 'ordinary_result_not_observed',
        candidateOrigin: 'not_observed',
        projectionClass: 'not_observed',
        resultDigest: '',
        textSha256: '',
        resultMarkerObserved: false,
        researchMarkerObserved: false,
        writingMarkerObserved: false
      },
      providerReceiptTrace: {
        reasonCode: expectedReasonCode,
        started: false,
        acknowledged: false,
        ended: false,
        eventCount: 0
      }
    })
    expect(JSON.stringify(observed)).not.toContain('Runtime request failed')
  })

  it('derives an exact provider receipt scope for any newly terminal turn without a result marker', async () => {
    const { exactLatestTerminalProviderReceiptScope } = await milestoneModule()
    const threadId = 'thread-provider-scope'
    const evidence = (status: string, overrides: Record<string, any> = {}) => {
      const { thread: threadOverrides = {}, ...evidenceOverrides } = overrides
      return {
        threadId,
        turnCount: 2,
        ...evidenceOverrides,
        thread: {
          id: threadId,
          latestSeq: 17,
          turns: [{ id: 'turn-plan', status: 'completed' }, { id: 'turn-agent', status }],
          ...threadOverrides
        }
      }
    }
    for (const status of ['completed', 'failed', 'aborted']) {
      const scope = exactLatestTerminalProviderReceiptScope(evidence(status), {
        previousTurnCount: 1,
        baselineSeq: 9,
        expectedThreadId: threadId
      })
      expect(scope).toEqual({
        baselineSeq: 9,
        expectedTurnId: 'turn-agent',
        expectedHighestSeq: 17
      })
      expect(Object.isFrozen(scope)).toBe(true)
    }
    for (const candidate of [
      evidence('running'),
      evidence('failed', { turnCount: 1 }),
      evidence('failed', { threadId: 'thread-foreign' }),
      evidence('failed', { thread: { latestSeq: 9 } }),
      evidence('failed', {
        thread: {
          turns: [{ id: 'turn-plan', status: 'completed' }, { id: ' ', status: 'failed' }]
        }
      })
    ]) {
      expect(exactLatestTerminalProviderReceiptScope(candidate, {
        previousTurnCount: 1,
        baselineSeq: 9,
        expectedThreadId: threadId
      })).toBeNull()
    }
  })

  it('projects a failed planning turn and cursor scope without leaking provider payloads', async () => {
    const {
      finalizeMilestoneAReport,
      planningTurnDiagnosticProjection,
      workflowEvidence
    } = await milestoneModule()
    const secret = 'RAW_PROVIDER_BODY_MUST_NOT_SURVIVE'
    const threadId = 'thread-plan-diagnostic-private'
    const turnId = 'turn-plan-diagnostic-private'
    const evidence = workflowEvidence({
      thread: {
        id: threadId,
        workspace: '/isolated/repository',
        latestSeq: 9,
        turns: [
          { id: 'turn-before-plan', status: 'completed', items: [] },
          {
            id: turnId,
            status: 'failed',
            items: [{
              kind: 'error',
              status: 'failed',
              code: 'host_tool_call_protocol_invalid',
              message: secret
            }]
          }
        ],
        todos: { items: [] }
      },
      summary: { subagents: [] },
      providerAttempts: [{
        threadId,
        turnId,
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 1,
        providerAttemptCount: 1,
        providerAttemptStatuses: {
          succeeded: 1,
          failed: 0,
          cancelled: 0,
          timedOut: 0,
          streamAborted: 0
        },
        endpoint: 'https://private-provider.invalid/v1',
        rawBody: secret
      }],
      providerTerminals: [{
        threadId,
        turnId,
        status: 'failed',
        rawBody: secret
      }]
    }, '/isolated/repository')
    const projection = planningTurnDiagnosticProjection(evidence, {
      previousTurnCount: 1,
      baselineSeq: 9,
      expectedThreadId: threadId,
      disposition: 'terminal_without_expected_marker'
    })

    expect(projection).toEqual(expect.objectContaining({
      planWorkflowDisposition: 'terminal_without_expected_marker',
      planCandidateTurnObserved: true,
      planCandidateTurnStatus: 'failed',
      planCandidateTurnErrorCode: 'host_tool_call_protocol_invalid',
      planCandidateToolAttemptCount: 0,
      planCandidateSuccessfulToolExecutionCount: 0,
      planCandidateProviderAttemptRecordCount: 1,
      planCandidateProviderTerminalRecordCount: 1,
      planCandidateProviderReceiptCountersBound: true,
      planCandidateProviderLogicalCallCount: 1,
      planCandidateProviderAttemptCount: 1,
      planCandidateProviderAttemptStatusCounts: {
        succeeded: 1,
        failed: 0,
        cancelled: 0,
        timedOut: 0,
        streamAborted: 0
      },
      planCandidateTerminalErrorItemAuthorityBound: true,
      planProviderReceiptScopeBound: false,
      planProviderReceiptScopeReasonCode: 'provider_receipt_scope_cursor_not_advanced'
    }))
    expect(projection.planCandidateTurnIdHash).toMatch(/^[0-9a-f]{64}$/)
    expect(projection.planCandidateTurnErrorCodeHash).toMatch(/^[0-9a-f]{64}$/)
    expect(projection.planCandidateInventoryDigest).toMatch(/^[0-9a-f]{64}$/)
    expect(JSON.stringify(projection)).not.toContain(secret)
    expect(JSON.stringify(projection)).not.toContain(threadId)
    expect(JSON.stringify(projection)).not.toContain(turnId)
    expect(JSON.stringify(projection)).not.toContain('private-provider.invalid')

    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      executionBlocker: 'packaged_plan_turn_failed',
      checks: [
        { id: 'external-real-repository-contract', status: 'passed', message: 'passed' },
        { id: 'trusted-cache-tmpdir', status: 'passed', message: 'passed' },
        { id: 'formal-packaged-artifact', status: 'passed', message: 'passed' },
        { id: 'isolated-profile-and-repository', status: 'passed', message: 'passed' },
        { id: 'configured-network-provider', status: 'passed', message: 'passed' },
        { id: 'packaged-first-launch', status: 'passed', message: 'passed' },
        {
          id: 'ordinary-agent-workflow',
          status: 'failed',
          message: 'host_tool_call_protocol_invalid'
        }
      ],
      workflow: {
        firstLaunchObserved: true,
        ...projection
      },
      stageSnapshots: []
    }, {
      requiredCheckIds: [
        'external-real-repository-contract',
        'trusted-cache-tmpdir',
        'formal-packaged-artifact',
        'isolated-profile-and-repository',
        'configured-network-provider',
        'packaged-first-launch',
        'ordinary-agent-workflow'
      ]
    })
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'first-launch-prerequisites',
        snapshotDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
      })
    ]))
    expect(JSON.stringify(report)).not.toContain(secret)
  })

  it('binds a completed Plan turn by exact state identity when its synthetic marker is absent', async () => {
    const { workflowEvidence } = await milestoneModule()
    const workspace = '/isolated/repository'
    const threadId = 'thread-plan-without-marker'
    const turnId = 'turn-plan-without-marker'
    const callId = `call_host_${'a'.repeat(64)}`
    const observation = {
      thread: {
        id: threadId,
        workspace,
        latestSeq: 9,
        turns: [
          { id: 'turn-baseline', status: 'completed', items: [] },
          {
            id: turnId,
            status: 'completed',
            items: [
              {
                kind: 'tool_call',
                status: 'completed',
                callId,
                toolName: 'read',
                toolKind: 'read'
              },
              {
                kind: 'assistant_text',
                status: 'completed',
                text: 'The structured plan is recorded.'
              }
            ]
          }
        ],
        todos: { items: [] }
      },
      summary: { subagents: [] },
      providerAttempts: [],
      providerTerminals: []
    }

    const markerOnly = workflowEvidence(observation, workspace, 'MISSING_PLAN_MARKER')
    expect(markerOnly.resultMarkerObserved).toBe(false)
    expect(markerOnly.resultTurnId).toBe(turnId)
    expect(markerOnly.resultTurnToolCallAttempts).toEqual([
      expect.objectContaining({ callId, turnId, toolName: 'read' })
    ])

    const exactTurn = workflowEvidence(
      observation,
      workspace,
      'MISSING_PLAN_MARKER',
      turnId
    )
    expect(exactTurn).toEqual(expect.objectContaining({
      resultMarkerObserved: false,
      resultTurnId: turnId,
      resultText: 'The structured plan is recorded.'
    }))
    expect(exactTurn.resultTurnToolCallAttempts).toEqual([
      expect.objectContaining({ callId, turnId, toolName: 'read' })
    ])

    const duplicateTurn = structuredClone(observation)
    duplicateTurn.thread.turns.splice(1, 0, {
      id: turnId,
      status: 'completed',
      items: []
    })
    expect(workflowEvidence(
      duplicateTurn,
      workspace,
      'MISSING_PLAN_MARKER',
      turnId
    ).resultTurnId).toBe('')
    duplicateTurn.thread.turns[1].items = [{
      kind: 'assistant_text',
      status: 'completed',
      text: 'DUPLICATE_PLAN_MARKER'
    }]
    expect(workflowEvidence(
      duplicateTurn,
      workspace,
      'DUPLICATE_PLAN_MARKER'
    ).resultTurnId).toBe('')

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const planStart = script.indexOf('const planned = await waitForWorkflow({')
    const planEnd = script.indexOf("const agentMode = await selectComposerMode", planStart)
    const planFlow = script.slice(planStart, planEnd)
    expect(planStart).toBeGreaterThan(0)
    expect(planEnd).toBeGreaterThan(planStart)
    expect(planFlow).toContain('latestTerminalProviderReceiptScopeEvidence(\n      planEvidence')
    expect(planFlow).toContain('planReceiptScope.expectedTurnId')
    expect(planFlow).toContain('planningTurnDiagnosticProjection(planEvidence')
    expect(planFlow).not.toContain('planEvidence.resultMarkerObserved')
  })

  it('fails closed on non-ordinary, unacknowledged, mixed, or unterminated SSE delivery', async () => {
    const { observeProviderTerminalViaSse } = await milestoneModule()
    const threadId = 'thread-provider-negative'
    const turnId = 'turn-provider-negative'
    const latestSeq = 8
    const baselineSeq = latestSeq - 2
    const timestamp = '2026-08-13T03:00:00Z'
    const ordinaryBatch = {
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      kind: 'general_terminal_batch',
      batchDigest: '1'.repeat(64),
      threadId,
      turnId,
      seq: latestSeq,
      firstSeq: latestSeq - 1,
      lastSeq: latestSeq,
      timestamp,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: latestSeq - 1,
        timestamp,
        threadId,
        turnId,
        model: 'deepseek-chat',
        usageFinalStatus: 'completed',
        usage: { promptTokens: 1, completionTokens: 1, totalTokens: 2, turns: 1 },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 1,
            failed: 0,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          }
        }
      }, {
        kind: 'turn_completed',
        seq: latestSeq,
        timestamp,
        threadId,
        turnId,
        status: 'completed'
      }],
      eventManifest: [{
        slot: 'usage', eventId: '6'.repeat(64), payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal', eventId: '8'.repeat(64), payloadDigest: '9'.repeat(64)
      }]
    }
    const run = async ({ events, ack = true, end = true }: {
      events: any[]
      ack?: boolean
      end?: boolean
    }) => {
      const callbacks: Record<string, (payload: any) => void> = {}
      const ackSseEvent = vi.fn(async (streamId: string) => {
        if (end) queueMicrotask(() => callbacks.end({ streamId }))
        return ack
      })
      const api = {
        onSseEvent: (handler: (payload: any) => void) => {
          callbacks.event = handler
          return vi.fn()
        },
        onSseEnd: (handler: (payload: any) => void) => {
          callbacks.end = handler
          return vi.fn()
        },
        onSseError: (handler: (payload: any) => void) => {
          callbacks.error = handler
          return vi.fn()
        },
        startSse: async (_thread: string, _since: number, streamId: string) => {
          callbacks.event({ streamId, events })
          return { streamId }
        },
        ackSseEvent,
        stopSse: async () => false
      }
      return {
        observed: await observeProviderTerminalViaSse(
          api,
          threadId,
          baselineSeq,
          turnId,
          latestSeq,
          10
        ),
        ackSseEvent
      }
    }

    const acceptedFinal = await run({
      events: [{ kind: 'accepted_final_batch', threadId, turnId, seq: latestSeq }]
    })
    expect(acceptedFinal.observed).toEqual(expect.objectContaining({
      providerAttempts: [],
      providerTerminals: [],
      providerReceiptTrace: expect.objectContaining({
        reasonCode: 'provider_receipt_non_general_terminal'
      })
    }))
    expect(acceptedFinal.ackSseEvent).not.toHaveBeenCalled()

    const foreignTurnId = 'turn-provider-negative-child'
    const foreignBatch = {
      ...ordinaryBatch,
      turnId: foreignTurnId,
      events: ordinaryBatch.events.map((event) => ({
        ...event,
        turnId: foreignTurnId
      }))
    }
    const foreignTerminal = await run({ events: [foreignBatch] })
    expect(foreignTerminal.observed.providerReceiptTrace.reasonCode).toBe(
      'provider_receipt_event_scope_invalid'
    )
    expect(foreignTerminal.ackSseEvent).not.toHaveBeenCalled()

    const unacknowledged = await run({ events: [ordinaryBatch], ack: false })
    expect(unacknowledged.observed.providerReceiptTrace.reasonCode).toBe(
      'provider_receipt_ack_rejected'
    )

    const mixed = await run({ events: [ordinaryBatch, ordinaryBatch] })
    expect(mixed.observed.providerReceiptTrace.reasonCode).toBe(
      'provider_receipt_event_envelope_invalid'
    )
    expect(mixed.ackSseEvent).not.toHaveBeenCalled()

    const authorityClaim = await run({
      events: [{ ...ordinaryBatch, evidenceAuthority: true }]
    })
    expect(authorityClaim.observed.providerReceiptTrace.reasonCode).toBe(
      'provider_receipt_terminal_batch_invalid'
    )
    expect(authorityClaim.ackSseEvent).not.toHaveBeenCalled()

    const unterminated = await run({ events: [ordinaryBatch], end: false })
    expect(unterminated.observed.providerReceiptTrace).toEqual(expect.objectContaining({
      reasonCode: 'provider_receipt_timeout',
      started: true,
      acknowledged: true,
      ended: false,
      eventCount: 2
    }))
  })

  it('observes a typed accepted-final SSE batch independently of general-terminal receipts', async () => {
    const { observeAcceptedFinalOrdinaryViaSse } = await milestoneModule()
    const fixture = acceptedFinalOrdinaryContinuationObservation()
    const threadId = fixture.threadId
    const turnId = fixture.turnId
    const baselineSeq = 50
    const firstSeq = 52
    const highestSeq = 54
    const timestamp = fixture.observation.thread.acceptedFinalDelivery.timestamp
    const delivery = structuredClone(fixture.observation.thread.acceptedFinalDelivery)
    const makeBatch = (mutate?: (batch: Record<string, any>) => void) => {
      const batch = structuredClone(delivery)
      batch.firstSeq = firstSeq
      batch.lastSeq = highestSeq
      batch.seq = highestSeq
      batch.publicationAuthority.firstSeq = firstSeq
      batch.publicationAuthority.lastSeq = highestSeq
      batch.events.forEach((event: Record<string, any>, index: number) => {
        event.seq = firstSeq + index
      })
      mutate?.(batch)
      return batch
    }
    const heartbeat = {
      kind: 'heartbeat',
      seq: baselineSeq,
      timestamp,
      threadId,
      trace: { sse_sent_at: 1, sse_live_emitted_at: 1 }
    }
    const cursorAdvanced = {
      kind: 'cursor_advanced',
      seq: baselineSeq + 1,
      timestamp,
      threadId,
      reason: 'restricted_content_removed'
    }
    const run = async (
      payloads: any[][],
      ackMode: 'exact' | 'missing' | 'mismatched' = 'exact',
      expectedSnapshotSeq = highestSeq
    ) => {
      const callbacks: Record<string, (payload: any) => void> = {}
      const ackSseEvent = vi.fn(async (
        streamId: string,
        seq: number,
        acceptedFinal?: Record<string, string>
      ) => {
        const expectedAcceptedFinal = seq === highestSeq && ackMode !== 'missing'
          ? {
              batchId: delivery.batchId,
              threadId,
              turnId: ackMode === 'mismatched' ? 'foreign-turn' : turnId,
              publicationCommitId: delivery.publicationCommitId
            }
          : undefined
        if (JSON.stringify(acceptedFinal) !== JSON.stringify(expectedAcceptedFinal)) {
          return false
        }
        if (seq === highestSeq) queueMicrotask(() => callbacks.end({ streamId }))
        return true
      })
      const api = {
        onSseEvent: (handler: (payload: any) => void) => {
          callbacks.event = handler
          return vi.fn()
        },
        onSseEnd: (handler: (payload: any) => void) => {
          callbacks.end = handler
          return vi.fn()
        },
        onSseError: (handler: (payload: any) => void) => {
          callbacks.error = handler
          return vi.fn()
        },
        startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
          for (const events of payloads) callbacks.event({ streamId, events })
          return { streamId }
        }),
        ackSseEvent,
        stopSse: vi.fn(async () => false)
      }
      return {
        observed: await observeAcceptedFinalOrdinaryViaSse(
          api,
          threadId,
          baselineSeq,
          turnId,
          expectedSnapshotSeq,
          100
        ),
        ackSseEvent
      }
    }

    const accepted = await run([
      [heartbeat],
      [cursorAdvanced],
      [makeBatch()]
    ])
    expect(accepted.observed).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'accepted_final_observed',
      reasonCode: 'accepted_final_observed',
      threadId,
      turnId,
      baselineSeq,
      firstSeq,
      terminalSeq: highestSeq,
      highestSeq,
      acceptedFinalDigest: delivery.publicationCommitId,
      deliveryManifestDigest: delivery.eventManifestDigest,
      terminalReason: 'success'
    }))
    expect(accepted.ackSseEvent.mock.calls.map(([, seq]) => seq)).toEqual([
      baselineSeq,
      baselineSeq + 1,
      highestSeq
    ])
    expect(accepted.ackSseEvent.mock.calls[0]).toHaveLength(2)
    expect(accepted.ackSseEvent.mock.calls[1]).toHaveLength(2)
    expect(accepted.ackSseEvent.mock.calls[2]).toEqual([
      expect.any(String),
      highestSeq,
      {
        batchId: delivery.batchId,
        threadId,
        turnId,
        publicationCommitId: delivery.publicationCommitId
      }
    ])
    expect(JSON.stringify(accepted.observed)).not.toContain('Continuation completed')
    expect(JSON.stringify(accepted.observed)).not.toContain('MILESTONE_A_CONTEXT_')

    const missingCursorAdvance = await run([[heartbeat], [makeBatch()]])
    expect(missingCursorAdvance.observed).toEqual(expect.objectContaining({
      observed: false,
      reasonCode: 'accepted_final_event_sequence_invalid'
    }))
    expect(missingCursorAdvance.ackSseEvent.mock.calls.map(([, seq]) => seq))
      .not.toContain(highestSeq)

    const snapshotAhead = await run([
      [heartbeat],
      [cursorAdvanced],
      [makeBatch()]
    ], 'exact', highestSeq + 1)
    expect(snapshotAhead.observed).toEqual(expect.objectContaining({
      observed: false,
      reasonCode: 'accepted_final_event_sequence_invalid'
    }))

    for (const ackMode of ['missing', 'mismatched'] as const) {
      const rejectedAck = await run([
        [heartbeat],
        [cursorAdvanced],
        [makeBatch()]
      ], ackMode)
      expect(rejectedAck.observed).toEqual(expect.objectContaining({
        observed: false,
        reasonCode: 'accepted_final_ack_rejected'
      }))
      expect(rejectedAck.ackSseEvent.mock.calls.at(-1)).toHaveLength(3)
    }

    const invalidCases: Array<{
      name: string
      payloads: any[][]
      reasonCode: string
    }> = [
      {
        name: 'duplicate',
        payloads: [[heartbeat], [cursorAdvanced], [makeBatch(), makeBatch()]],
        reasonCode: 'accepted_final_duplicate'
      },
      {
        name: 'cross-thread',
        payloads: [[makeBatch((batch) => { batch.threadId = 'foreign-thread' })]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'cross-turn',
        payloads: [[makeBatch((batch) => { batch.turnId = 'foreign-turn' })]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-cross-thread',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId: 'foreign-thread', turnId, status: 'running'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-cross-turn',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId, turnId: 'foreign-turn', status: 'running'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-not-running',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId, turnId, status: 'completed'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-noncontiguous',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 2, timestamp,
          threadId, turnId, status: 'running'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-extra-field',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId, turnId, status: 'running', raw: 'PRIVATE_TURN_START'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-invalid-trace',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId, turnId, status: 'running',
          trace: { sse_sent_at: 1, sse_live_emitted_at: 1, raw: 'PRIVATE_TRACE' }
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-duplicate',
        payloads: [[{
          kind: 'turn_started', seq: baselineSeq + 1, timestamp,
          threadId, turnId, status: 'running'
        }], [{
          kind: 'turn_started', seq: baselineSeq + 2, timestamp,
          threadId, turnId, status: 'running'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'turn-start-after-cursor-advance',
        payloads: [[cursorAdvanced], [{
          kind: 'turn_started', seq: baselineSeq + 2, timestamp,
          threadId, turnId, status: 'running'
        }]],
        reasonCode: 'accepted_final_event_scope_invalid'
      },
      {
        name: 'invalid-schema',
        payloads: [
          [heartbeat],
          [cursorAdvanced],
          [makeBatch((batch) => { batch.schemaVersion = 1 })]
        ],
        reasonCode: 'accepted_final_schema_invalid'
      },
      {
        name: 'terminal-missing',
        payloads: [
          [heartbeat],
          [cursorAdvanced],
          [makeBatch((batch) => {
            batch.events = batch.events.slice(0, 2)
            batch.lastSeq = firstSeq + 1
            batch.seq = batch.lastSeq
            batch.publicationAuthority.lastSeq = batch.lastSeq
          })]
        ],
        reasonCode: 'accepted_final_terminal_missing'
      },
      {
        name: 'invalid-usage-tokens',
        payloads: [
          [heartbeat],
          [cursorAdvanced],
          [makeBatch((batch) => {
            batch.events[1].usage.promptTokens = -1
          })]
        ],
        reasonCode: 'accepted_final_schema_invalid'
      }
    ]
    for (const candidate of invalidCases) {
      const rejected = await run(candidate.payloads)
      expect(rejected.observed, candidate.name).toEqual(expect.objectContaining({
        observed: false,
        reasonCode: candidate.reasonCode
      }))
      expect(rejected.ackSseEvent.mock.calls.map(([, seq]) => seq)).not.toContain(highestSeq)
      expect(JSON.stringify(rejected.observed)).not.toContain('Continuation completed')
      expect(JSON.stringify(rejected.observed)).not.toContain('MILESTONE_A_CONTEXT_')
    }
  })

  it('observes the real Go case public SSE turn_started prelude before accepted-final delivery', async () => {
    const { observeAcceptedFinalOrdinaryViaSse } = await milestoneModule()
    const fixture = acceptedFinalOrdinaryContinuationObservation()
    const threadId = fixture.threadId
    const turnId = fixture.turnId
    const baselineSeq = 70
    const turnStartedSeq = baselineSeq + 1
    const cursorSeq = turnStartedSeq + 1
    const firstSeq = cursorSeq + 1
    const highestSeq = firstSeq + 2
    const timestamp = fixture.observation.thread.acceptedFinalDelivery.timestamp
    const delivery = structuredClone(fixture.observation.thread.acceptedFinalDelivery)
    delivery.firstSeq = firstSeq
    delivery.lastSeq = highestSeq
    delivery.seq = highestSeq
    delivery.publicationAuthority.firstSeq = firstSeq
    delivery.publicationAuthority.lastSeq = highestSeq
    delivery.events.forEach((event: Record<string, any>, index: number) => {
      event.seq = firstSeq + index
    })
    const turnStarted = {
      kind: 'turn_started',
      seq: turnStartedSeq,
      timestamp,
      threadId,
      turnId,
      status: 'running',
      trace: { sse_sent_at: 1, sse_live_emitted_at: 1 }
    }
    const cursorAdvanced = {
      kind: 'cursor_advanced',
      seq: cursorSeq,
      timestamp,
      threadId,
      reason: 'restricted_content_removed',
      trace: { sse_sent_at: 2, sse_live_emitted_at: 2 }
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const ackSseEvent = vi.fn(async (
      streamId: string,
      seq: number,
      acceptedFinal?: Record<string, string>
    ) => {
      const expectedBinding = seq === highestSeq
        ? {
            batchId: delivery.batchId,
            threadId,
            turnId,
            publicationCommitId: delivery.publicationCommitId
          }
        : undefined
      if (JSON.stringify(acceptedFinal) !== JSON.stringify(expectedBinding)) return false
      if (![turnStartedSeq, cursorSeq, highestSeq].includes(seq)) return false
      if (seq === highestSeq) queueMicrotask(() => callbacks.end({ streamId }))
      return true
    })
    const api = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
        callbacks.event({ streamId, events: [turnStarted] })
        callbacks.event({ streamId, events: [cursorAdvanced] })
        callbacks.event({ streamId, events: [delivery] })
        return { streamId }
      }),
      ackSseEvent,
      stopSse: vi.fn(async () => false)
    }
    const observed = await observeAcceptedFinalOrdinaryViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      100
    )

    expect(observed).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'accepted_final_observed',
      reasonCode: 'accepted_final_observed',
      threadId,
      turnId,
      baselineSeq,
      firstSeq,
      terminalSeq: highestSeq,
      highestSeq,
      acceptedFinalDigest: delivery.publicationCommitId,
      deliveryManifestDigest: delivery.eventManifestDigest,
      terminalReason: 'success'
    }))
    expect(ackSseEvent.mock.calls.map(([, seq]) => seq)).toEqual([
      turnStartedSeq,
      cursorSeq,
      highestSeq
    ])
    expect(ackSseEvent.mock.calls.at(-1)).toEqual([
      expect.any(String),
      highestSeq,
      {
        batchId: delivery.batchId,
        threadId,
        turnId,
        publicationCommitId: delivery.publicationCommitId
      }
    ])
  })

  it('replays the exact failed Agent turn and retains only a closed provider diagnostic', async () => {
    const {
      observeProviderFailureDiagnosticViaSse,
      providerFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-provider-failure'
    const turnId = 'turn-provider-failure'
    const baselineSeq = 10
    const terminalSeq = 15
    const highestSeq = 16
    const callbacks: Record<string, (payload: any) => void> = {}
    const ackOrder: number[] = []
    const cleanup = [vi.fn(), vi.fn(), vi.fn()]
    const batches = [[{
      kind: 'turn_started',
      seq: 11,
      threadId,
      turnId
    }, {
      kind: 'pipeline_stage',
      seq: 12,
      timestamp: '2026-08-03T04:00:00Z',
      threadId,
      turnId,
      stage: 'provider_error',
      label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_unavailable',
        providerError: {
          kind: 'server',
          endpointFormat: 'chat_completions',
          authStatus: 'none',
          hasApiKey: true,
          retryable: true,
          status: 503,
          retryAfterMs: 500
        }
      }
    }], [{
      kind: 'cursor_advanced',
      seq: 13,
      threadId,
      timestamp: '2026-08-03T04:00:00Z',
      reason: 'restricted_content_removed'
    }], [{
      kind: 'general_terminal_batch',
      seq: terminalSeq,
      threadId,
      turnId,
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      timestamp: '2026-08-03T04:00:00Z',
      batchDigest: '1'.repeat(64),
      firstSeq: 14,
      lastSeq: terminalSeq,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: 14,
        threadId,
        turnId,
        timestamp: '2026-08-03T04:00:00Z',
        model: 'deepseek-chat',
        usage: { promptTokens: 10, completionTokens: 0, totalTokens: 10, turns: 1 },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 0,
            failed: 1,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          }
        },
        usageFinalStatus: 'failed'
      }, {
        kind: 'turn_failed',
        seq: terminalSeq,
        threadId,
        turnId,
        timestamp: '2026-08-03T04:00:00Z',
        status: 'failed',
        terminalReason: 'provider_failure',
        code: 'provider_unavailable',
        message: 'The model provider is temporarily unavailable.'
      }],
      eventManifest: [{
        slot: 'usage',
        eventId: '6'.repeat(64),
        payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal',
        eventId: '8'.repeat(64),
        payloadDigest: '9'.repeat(64)
      }]
    }]]
    const startSse = vi.fn(async (
      observedThreadId: string,
      sinceSeq: number,
      streamId: string
    ) => {
      expect(observedThreadId).toBe(threadId)
      expect(sinceSeq).toBe(baselineSeq)
      for (const events of batches) callbacks.event({ streamId, events })
      return { streamId }
    })
    const api = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return cleanup[0]
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return cleanup[1]
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return cleanup[2]
      },
      startSse,
      ackSseEvent: vi.fn(async (streamId: string, seq: number) => {
        ackOrder.push(seq)
        if (seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
        return true
      }),
      stopSse: vi.fn(async () => true)
    }

    const raw = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      100
    )
    const evidence = providerFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })

    expect(startSse).toHaveBeenCalledWith(threadId, baselineSeq, expect.any(String))
    expect(ackOrder).toEqual([12, 13, 15])
    expect(evidence).toEqual({
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_unavailable',
      providerKind: 'server',
      providerStatus: 503,
      providerRetryable: true,
      providerAuthStatus: 'none',
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    })
    expect(JSON.stringify(evidence)).not.toContain('thread-provider-failure')
    expect(cleanup.every((item) => item.mock.calls.length === 1)).toBe(true)

    const changedStatus = providerFailureDiagnosticEvidence({
      ...raw,
      providerStatus: 504
    }, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })
    expect(changedStatus.diagnosticDigest).not.toBe(evidence.diagnosticDigest)
    expect(providerFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: 'turn-other',
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
    expect(providerFailureDiagnosticEvidence({ ...raw, observed: false }, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_invalid',
      diagnosticDigest: ''
    }))
  })

  it('accepts a preload-valid provider-failure replay batch larger than 128 events', async () => {
    const { observeProviderFailureDiagnosticViaSse } = await milestoneModule()
    const threadId = 'thread-provider-large-replay'
    const turnId = 'turn-provider-large-replay'
    const baselineSeq = 100
    const timestamp = '2026-08-15T04:00:00Z'
    const progressEvents = Array.from({ length: 129 }, (_, index) => ({
      kind: 'pipeline_stage',
      seq: baselineSeq + index + 1,
      timestamp,
      threadId,
      turnId,
      stage: 'pre_send',
      label: 'Pre-Send'
    }))
    const providerSeq = baselineSeq + progressEvents.length + 1
    const terminalSeq = providerSeq + 2
    const providerEvent = {
      kind: 'pipeline_stage',
      seq: providerSeq,
      timestamp,
      threadId,
      turnId,
      stage: 'provider_error',
      label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_unavailable',
        providerError: {
          kind: 'server',
          endpointFormat: 'chat_completions',
          authStatus: 'none',
          hasApiKey: true,
          retryable: true,
          status: 503
        }
      }
    }
    const terminalBatch = {
      kind: 'general_terminal_batch',
      seq: terminalSeq,
      timestamp,
      threadId,
      turnId,
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      batchDigest: '1'.repeat(64),
      firstSeq: providerSeq + 1,
      lastSeq: terminalSeq,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: providerSeq + 1,
        timestamp,
        threadId,
        turnId,
        usageFinalStatus: 'failed',
        usage: { promptTokens: 10, completionTokens: 0, totalTokens: 10, turns: 1 },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 0,
            failed: 1,
            cancelled: 0,
            timedOut: 0,
            streamAborted: 0
          }
        }
      }, {
        kind: 'turn_failed',
        seq: terminalSeq,
        timestamp,
        threadId,
        turnId,
        status: 'failed',
        terminalReason: 'provider_failure',
        code: 'provider_unavailable'
      }],
      eventManifest: [{
        slot: 'usage', eventId: '6'.repeat(64), payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal', eventId: '8'.repeat(64), payloadDigest: '9'.repeat(64)
      }]
    }
    expect(isStrictPublicRuntimeSseIpcPayload({
      streamId: 'preload-valid-large-replay',
      events: progressEvents
    })).toBe(true)

    const callbacks: Record<string, (payload: any) => void> = {}
    const acked: number[] = []
    const api = {
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
        callbacks.event({ streamId, events: progressEvents })
        callbacks.event({ streamId, events: [providerEvent] })
        callbacks.event({ streamId, events: [terminalBatch] })
        return { streamId }
      }),
      ackSseEvent: vi.fn(async (streamId: string, seq: number) => {
        acked.push(seq)
        if (seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
        return true
      }),
      stopSse: vi.fn(async () => true)
    }

    const observed = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      terminalSeq,
      100
    )
    expect(observed).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_unavailable',
      providerKind: 'server',
      providerStatus: 503,
      providerRetryable: true,
      providerAuthStatus: 'none'
    }))
    expect(acked).toEqual([
      baselineSeq + progressEvents.length,
      providerSeq,
      terminalSeq
    ])
  })

  it('fails the provider diagnostic closed on open, duplicate, cross-turn, or incomplete replay', async () => {
    const {
      observeProviderFailureDiagnosticViaSse,
      providerFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-provider-diagnostic-negative'
    const turnId = 'turn-provider-diagnostic-negative'
    const baselineSeq = 20
    const terminalSeq = 24
    const highestSeq = 25
    const providerEvent = {
      kind: 'pipeline_stage',
      seq: 21,
      timestamp: '2026-08-03T04:00:01Z',
      threadId,
      turnId,
      stage: 'provider_error',
      label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_authentication_failed',
        providerError: {
          kind: 'auth',
          authStatus: 'required',
          hasApiKey: true,
          retryable: false,
          status: 401,
          endpointFormat: 'chat_completions'
        }
      }
    }
    const terminalBatch = {
      kind: 'general_terminal_batch',
      seq: terminalSeq,
      threadId,
      turnId,
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      timestamp: '2026-08-03T04:00:01Z',
      batchDigest: 'a'.repeat(64),
      firstSeq: 23,
      lastSeq: terminalSeq,
      generalTerminalCommitId: 'b'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: 'c'.repeat(64),
      eventManifestDigest: 'd'.repeat(64),
      projectedEventsDigest: 'e'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: 23,
        threadId,
        turnId,
        timestamp: '2026-08-03T04:00:01Z'
      }, {
        kind: 'turn_failed',
        seq: terminalSeq,
        threadId,
        turnId,
        timestamp: '2026-08-03T04:00:01Z',
        status: 'failed',
        terminalReason: 'provider_failure',
        code: 'provider_authentication_failed'
      }] as Array<Record<string, any>>,
      eventManifest: [{
        slot: 'usage',
        eventId: 'f'.repeat(64),
        payloadDigest: '1'.repeat(64)
      }, {
        slot: 'terminal',
        eventId: '2'.repeat(64),
        payloadDigest: '3'.repeat(64)
      }]
    }
    const run = async ({
      payloads,
      ack = true,
      end = true
    }: {
      payloads: any[][]
      ack?: boolean
      end?: boolean
    }) => {
      const callbacks: Record<string, (payload: any) => void> = {}
      const api = {
        onSseEvent: (handler: (payload: any) => void) => {
          callbacks.event = handler
          return vi.fn()
        },
        onSseEnd: (handler: (payload: any) => void) => {
          callbacks.end = handler
          return vi.fn()
        },
        onSseError: (handler: (payload: any) => void) => {
          callbacks.error = handler
          return vi.fn()
        },
        startSse: async (_thread: string, _since: number, streamId: string) => {
          for (const events of payloads) callbacks.event({ streamId, events })
          return { streamId }
        },
        ackSseEvent: async (streamId: string, seq: number) => {
          if (end && seq === terminalSeq) queueMicrotask(() => callbacks.end({ streamId }))
          return ack
        },
        stopSse: async () => true
      }
      return observeProviderFailureDiagnosticViaSse(
        api,
        threadId,
        baselineSeq,
        turnId,
        highestSeq,
        15
      )
    }

    const cases = [{
      expected: 'provider_failure_event_scope_invalid',
      payloads: [[{ ...providerEvent, turnId: 'turn-other' }], [terminalBatch]]
    }, {
      expected: 'provider_failure_duplicate',
      payloads: [[providerEvent], [{ ...providerEvent, seq: 22 }], [terminalBatch]]
    }, {
      expected: 'provider_failure_event_scope_invalid',
      payloads: [[{ kind: 'accepted_final_batch', seq: 21, threadId, turnId }]]
    }, {
      expected: 'provider_failure_diagnostic_invalid',
      payloads: [[{
        ...providerEvent,
        details: {
          providerError: {
            ...providerEvent.details.providerError,
            body: 'PRIVATE_PROVIDER_BODY_MUST_NOT_PROJECT'
          }
        }
      }], [terminalBatch]]
    }, {
      expected: 'provider_failure_diagnostic_invalid',
      payloads: [[{
        ...providerEvent,
        details: {
          providerError: { ...providerEvent.details.providerError, status: 600 }
        }
      }], [terminalBatch]]
    }, {
      expected: 'provider_failure_terminal_invalid',
      payloads: [[providerEvent], [{
        ...terminalBatch,
        events: [terminalBatch.events[0], {
          ...terminalBatch.events[1],
          terminalReason: 'success'
        }]
      }]]
    }]
    for (const candidate of cases) {
      const observed = await run({ payloads: candidate.payloads })
      expect(observed).toEqual(expect.objectContaining({
        observed: false,
        observationCode: candidate.expected,
        reasonCode: 'none'
      }))
      expect(JSON.stringify(observed)).not.toContain('PRIVATE_PROVIDER_BODY')
    }
    expect(await run({ payloads: [[terminalBatch]] })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_missing',
      reasonCode: 'none'
    }))
    const streamAbortedTerminal = structuredClone(terminalBatch)
    streamAbortedTerminal.events = streamAbortedTerminal.events.map(
      (event: Record<string, any>) => event.kind === 'usage'
        ? {
            ...event,
            model: 'deepseek-chat',
            usageFinalStatus: 'failed',
            usage: {
              promptTokens: 610,
              completionTokens: 90,
              totalTokens: 700,
              turns: 6
            },
            cacheDiagnostics: {
              providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
              providerAttemptTelemetryValid: true,
              providerLogicalCallCount: 6,
              providerAttemptCount: 6,
              providerAttemptStatuses: {
                succeeded: 5,
                failed: 0,
                cancelled: 0,
                timedOut: 0,
                streamAborted: 1
              },
              providerAttemptInputTokens: {
                complete: true,
                knownObservationCount: 6,
                observationCount: 6,
                value: 610
              },
              providerAttemptOutputTokens: {
                complete: true,
                knownObservationCount: 6,
                observationCount: 6,
                value: 90
              },
              providerAttemptCacheHitTokens: {
                complete: false,
                knownObservationCount: 0,
                observationCount: 6
              },
              providerAttemptCacheMissTokens: {
                complete: false,
                knownObservationCount: 0,
                observationCount: 6
              },
              providerAttemptCacheRate: {
                known: false,
                numerator: 0,
                denominator: 0
              }
            }
          }
        : event.kind === 'turn_failed'
          ? { ...event, code: 'provider_error' }
          : event
    )
    const streamAbortedObserved = await run({ payloads: [[streamAbortedTerminal]] })
    expect(streamAbortedObserved).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_terminal_observed',
      reasonCode: 'provider_stream_interrupted',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))
    expect(providerFailureDiagnosticEvidence(streamAbortedObserved, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_terminal_observed',
      reasonCode: 'provider_stream_interrupted',
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    const ambiguousAttemptStatuses = structuredClone(streamAbortedTerminal)
    ambiguousAttemptStatuses.events[0].cacheDiagnostics.providerAttemptCount = 7
    ambiguousAttemptStatuses.events[0].cacheDiagnostics.providerAttemptStatuses.failed = 1
    expect(await run({ payloads: [[ambiguousAttemptStatuses]] })).toEqual(
      expect.objectContaining({
        observed: false,
        observationCode: 'provider_failure_diagnostic_missing',
        reasonCode: 'none'
      })
    )
    const terminalOnlyOutputPolicy = structuredClone(terminalBatch)
    terminalOnlyOutputPolicy.events = terminalOnlyOutputPolicy.events.map(
      (event: Record<string, any>) => event.kind === 'turn_failed'
        ? { ...event, code: 'provider_empty_final' }
        : event
    )
    const terminalOnlyObserved = await run({ payloads: [[terminalOnlyOutputPolicy]] })
    expect(terminalOnlyObserved).toEqual(
      expect.objectContaining({
        observed: true,
        observationCode: 'provider_failure_terminal_observed',
        reasonCode: 'provider_empty_final',
        providerKind: 'unknown',
        providerStatus: null,
        providerRetryable: null,
        providerAuthStatus: 'not_observed'
      })
    )
    expect(providerFailureDiagnosticEvidence(terminalOnlyObserved, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_terminal_observed',
      reasonCode: 'provider_empty_final',
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(providerFailureDiagnosticEvidence({
      ...terminalOnlyObserved,
      observed: false
    }, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_invalid'
    }))
    expect(await run({ payloads: [[providerEvent], [terminalBatch]], ack: false }))
      .toEqual(expect.objectContaining({
        observed: false,
        observationCode: 'provider_failure_ack_rejected'
      }))
    expect(await run({ payloads: [[providerEvent], [terminalBatch]], end: false }))
      .toEqual(expect.objectContaining({
        observed: false,
        observationCode: 'provider_failure_timeout'
      }))
  })

  it('observes the real sanitized provider-failure SSE shape across snapshot rewind, heartbeat, and split terminal delivery', async () => {
    const {
      observeProviderFailureDiagnosticViaSse,
      providerFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-provider-real-shape'
    const turnId = 'turn-provider-real-shape'
    const priorTurnId = 'turn-provider-prior'
    const baselineSeq = 10
    const highestSeq = 17
    const timestamp = '2026-08-05T04:00:00Z'
    const prior = acceptedFinalOrdinaryContinuationObservation().observation.thread
      .acceptedFinalDelivery
    const priorAcceptedFinal = structuredClone(prior)
    priorAcceptedFinal.threadId = threadId
    priorAcceptedFinal.turnId = priorTurnId
    priorAcceptedFinal.firstSeq = 12
    priorAcceptedFinal.lastSeq = 14
    priorAcceptedFinal.seq = 14
    priorAcceptedFinal.events = priorAcceptedFinal.events.map(
      (event: Record<string, any>, index: number) => ({
        ...event,
        threadId,
        turnId: priorTurnId,
        seq: 12 + index,
        ...(index === 0
          ? { item: { ...event.item, threadId, turnId: priorTurnId } }
          : {})
      })
    )
    priorAcceptedFinal.publicationAuthority = {
      ...priorAcceptedFinal.publicationAuthority,
      threadId,
      turnId: priorTurnId,
      firstSeq: 12,
      lastSeq: 14
    }
    expect(priorAcceptedFinal.lastSeq - priorAcceptedFinal.firstSeq + 1)
      .toBe(priorAcceptedFinal.events.length)
    const providerError = {
      kind: 'pipeline_stage',
      seq: 15,
      timestamp,
      threadId,
      turnId,
      stage: 'provider_error',
      label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_unavailable'
      }
    }
    const cursorAdvanced = {
      kind: 'cursor_advanced',
      seq: 11,
      timestamp,
      threadId,
      reason: 'restricted_content_removed'
    }
    const heartbeat = {
      kind: 'heartbeat',
      seq: 10,
      timestamp,
      threadId
    }
    const terminalBatch = {
      kind: 'general_terminal_batch',
      seq: 17,
      timestamp,
      threadId,
      turnId,
      schemaVersion: 1,
      purpose: 'analytix.general-terminal-delivery-batch/v1',
      batchDigest: '1'.repeat(64),
      firstSeq: 16,
      lastSeq: 17,
      generalTerminalCommitId: '2'.repeat(64),
      generalTerminalAuthorityKind: 'general_terminal_cas',
      generalTerminalAuthorityDigest: '3'.repeat(64),
      eventManifestDigest: '4'.repeat(64),
      projectedEventsDigest: '5'.repeat(64),
      transportAuthority: 'host_batch_digest_v1',
      evidenceAuthority: false,
      citationAuthority: false,
      factAnswerAllowed: false,
      events: [{
        kind: 'usage',
        seq: 16,
        timestamp,
        threadId,
        turnId,
        model: 'deepseek-v4-flash',
        usageFinalStatus: 'failed',
        usage: { promptTokens: 4, completionTokens: 0, totalTokens: 4, turns: 1 },
        cacheDiagnostics: {
          providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
          providerAttemptTelemetryValid: true,
          providerLogicalCallCount: 1,
          providerAttemptCount: 1,
          providerAttemptStatuses: {
            succeeded: 0, failed: 1, cancelled: 0, timedOut: 0, streamAborted: 0
          }
        }
      }, {
        kind: 'turn_failed',
        seq: 17,
        timestamp,
        threadId,
        turnId,
        status: 'failed',
        terminalReason: 'provider_failure',
        code: 'provider_unavailable'
      }],
      eventManifest: [{
        slot: 'usage', eventId: '6'.repeat(64), payloadDigest: '7'.repeat(64)
      }, {
        slot: 'terminal', eventId: '8'.repeat(64), payloadDigest: '9'.repeat(64)
      }]
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const acked: Array<{
      seq: number
      acceptedFinal?: {
        batchId: string
        threadId: string
        turnId: string
        publicationCommitId: string
      }
    }> = []
    let replayPrior: Record<string, any> | null = priorAcceptedFinal
    let replayProviderError: Record<string, any> = providerError
    let replayCursor = cursorAdvanced
    let replayTerminal: Record<string, any> = terminalBatch
    const api = {
      onSseEvent: (handler: (payload: any) => void) => { callbacks.event = handler; return vi.fn() },
      onSseEnd: (handler: (payload: any) => void) => { callbacks.end = handler; return vi.fn() },
      onSseError: (handler: (payload: any) => void) => { callbacks.error = handler; return vi.fn() },
      startSse: vi.fn(async (_thread: string, since: number, streamId: string) => {
        expect(since).toBe(baselineSeq)
        callbacks.event({ streamId, events: [heartbeat] })
        callbacks.event({ streamId, events: [replayCursor] })
        if (replayPrior) callbacks.event({ streamId, events: [replayPrior] })
        callbacks.event({ streamId, events: [replayProviderError] })
        callbacks.event({ streamId, events: [replayTerminal] })
        return { streamId }
      }),
      ackSseEvent: vi.fn(async (
        streamId: string,
        seq: number,
        acceptedFinal?: {
          batchId: string
          threadId: string
          turnId: string
          publicationCommitId: string
        }
      ) => {
        acked.push({ seq, acceptedFinal })
        if (replayPrior && seq === replayPrior.lastSeq && (
          acceptedFinal?.batchId !== replayPrior.batchId ||
          acceptedFinal?.threadId !== replayPrior.threadId ||
          acceptedFinal?.turnId !== replayPrior.turnId ||
          acceptedFinal?.publicationCommitId !== replayPrior.publicationCommitId
        )) return false
        if (seq === highestSeq) queueMicrotask(() => callbacks.end({ streamId }))
        return true
      }),
      stopSse: vi.fn(async () => true)
    }
    const observed = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(observed).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_observed',
      threadId,
      turnId,
      baselineSeq,
      terminalSeq: 17,
      highestSeq,
      reasonCode: 'provider_unavailable',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))
    expect(acked.map(({ seq }) => seq)).toEqual([11, 14, 15, 17])
    expect(acked[1]?.acceptedFinal).toEqual({
      batchId: priorAcceptedFinal.batchId,
      threadId: priorAcceptedFinal.threadId,
      turnId: priorAcceptedFinal.turnId,
      publicationCommitId: priorAcceptedFinal.publicationCommitId
    })
    expect(JSON.stringify(observed)).not.toContain('providerId')
    expect(JSON.stringify(observed)).not.toContain('deepseek-v4-flash')

    replayPrior = null
    replayCursor = { ...cursorAdvanced, seq: 14 }
    const noPriorReplayObserved = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(noPriorReplayObserved).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_unavailable'
    }))

    replayCursor = cursorAdvanced
    const foreignPrior = structuredClone(priorAcceptedFinal)
    foreignPrior.turnId = 'turn-provider-foreign-prior'
    foreignPrior.publicationAuthority.turnId = foreignPrior.turnId
    foreignPrior.events = foreignPrior.events.map((event: Record<string, any>) => ({
      ...event,
      turnId: foreignPrior.turnId
    }))
    replayPrior = foreignPrior
    const foreignPriorObserved = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(foreignPriorObserved).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_event_scope_invalid'
    }))

    replayPrior = priorAcceptedFinal
    replayCursor = { ...cursorAdvanced, seq: highestSeq + 1 }
    const transportCursorMismatch = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(transportCursorMismatch).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_event_sequence_invalid'
    }))

    replayCursor = cursorAdvanced
    replayProviderError = {
      ...providerError,
      stage: 'provider_admission_rejected',
      label: 'Provider admission rejected',
      details: {
        reasonCode: 'context_window_hard_limit',
        projectedRequestTokens: 900,
        hardThresholdTokens: 850,
        providerAttemptCount: 0
      }
    }
    replayTerminal = structuredClone(terminalBatch)
    replayTerminal.events = replayTerminal.events.map((event: Record<string, any>) =>
      event.kind === 'turn_failed'
        ? { ...event, terminalReason: 'semantic_failure', code: 'context_window_hard_limit' }
        : event
    )
    const hostContextObserved = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(hostContextObserved).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'host_context_budget_observed',
      reasonCode: 'context_window_hard_limit',
      providerKind: 'host_context_budget',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))
    expect(providerFailureDiagnosticEvidence(hostContextObserved, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'host_context_budget_observed',
      reasonCode: 'context_window_hard_limit',
      providerKind: 'host_context_budget'
    }))
    expect(providerFailureDiagnosticEvidence({
      ...hostContextObserved,
      observationCode: 'provider_failure_observed',
      providerKind: 'unknown'
    }, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_invalid'
    }))

    replayProviderError = {
      ...providerError,
      details: {
        reasonCode: 'provider_empty_final',
        visibleRecovery: true,
        recoveryKind: 'empty_final',
        recoveryAttempt: 1,
        maxRecoveryAttempts: 1,
        recoveryExhausted: true
      }
    }
    replayTerminal = structuredClone(terminalBatch)
    replayTerminal.events = replayTerminal.events.map((event: Record<string, any>) =>
      event.kind === 'turn_failed'
        ? { ...event, code: 'provider_empty_final' }
        : event
    )
    const emptyFinalObserved = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(emptyFinalObserved).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_observed',
      reasonCode: 'provider_empty_final',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))

    replayProviderError = {
      ...providerError,
      details: { reasonCode: 'provider_reason_was_invented' }
    }
    const unknownReasonObserved = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )
    expect(unknownReasonObserved).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'provider_failure_diagnostic_invalid'
    }))
  })

  it('binds host context admission and provider diagnostics to exact case failed accepted-final batches', async () => {
    const { observeProviderFailureDiagnosticViaSse } = await milestoneModule()
    const threadId = 'thread-provider-case-terminal'
    const turnId = 'turn-provider-case-terminal'
    const priorTurnId = 'turn-provider-case-terminal-prior'
    const baselineSeq = 20
    const timestamp = '2026-08-05T05:00:00Z'
    const publicationCommitId = 'a'.repeat(64)
    const firstSeq = 23
    const highestSeq = 26
    const terminalItemId = `item_${turnId}_case_terminal`
    const terminalMessage = '案件分析未完成；未经核验的案件事实未发布。'
    const acceptedFinalView = {
      schemaVersion: 3,
      acceptedFinalDigest: publicationCommitId,
      publicationState: 'accepted',
      variant: 'NeedsEvidenceAnswer',
      terminalReason: 'provider_failure',
      blockerCode: 'provider_failure',
      coverageStatus: 'unverified',
      checkedScopeDigest: '',
      missingScopeCount: 1,
      claimCount: 0,
      claimTypes: [],
      receiptMetadata: {
        projection: 'masked_metadata_only',
        count: 0,
        setDigest: 'e'.repeat(64),
        citations: []
      },
      noHitWording: '',
      acceptedAt: timestamp
    }
    const common = {
      timestamp,
      threadId,
      turnId,
      acceptedFinalDigest: publicationCommitId,
      publicationCommitId
    }
    const events: Array<Record<string, any>> = [{
      ...common,
      kind: 'item_completed',
      seq: firstSeq,
      itemId: `item_${turnId}_assistant`,
      publicationSlot: 'assistant-final',
      publicationEventId: '1'.repeat(64),
      publicationPayloadDigest: '2'.repeat(64),
      item: {
        id: `item_${turnId}_assistant`,
        threadId,
        turnId,
        role: 'assistant',
        status: 'completed',
        kind: 'assistant_text',
        createdAt: timestamp,
        finishedAt: timestamp,
        text: terminalMessage,
        acceptedFinalView
      }
    }, {
      ...common,
      kind: 'item_completed',
      seq: firstSeq + 1,
      itemId: terminalItemId,
      publicationSlot: 'terminal-error-item',
      publicationEventId: '3'.repeat(64),
      publicationPayloadDigest: '4'.repeat(64),
      item: {
        id: terminalItemId,
        threadId,
        turnId,
        role: 'system',
        status: 'failed',
        kind: 'error',
        createdAt: timestamp,
        finishedAt: timestamp,
        code: 'case_terminal_provider_failure',
        message: terminalMessage,
        severity: 'error',
        acceptedFinalDigest: publicationCommitId
      }
    }, {
      ...common,
      kind: 'usage',
      seq: firstSeq + 2,
      publicationSlot: 'usage',
      publicationEventId: '5'.repeat(64),
      publicationPayloadDigest: '6'.repeat(64),
      model: 'deepseek-v4-flash',
      usageFinalStatus: 'failed',
      usage: {
        promptTokens: 0,
        completionTokens: 0,
        reasoningTokens: 0,
        totalTokens: 0,
        cacheHitRate: null,
        cacheableTokenHitRate: null,
        totalInputTokenHitRate: null,
        cacheMissReasons: [],
        cacheSuggestions: [],
        costUsd: 0,
        costCny: 0,
        priceConfigured: false,
        cacheSavingsUsd: 0,
        cacheSavingsCny: 0,
        tokenEconomySavingsTokens: 0,
        turns: 1
      },
      cacheDiagnostics: {
        providerAttemptTelemetrySchema: 'provider-attempt-telemetry.v1',
        providerAttemptTelemetryValid: true,
        providerLogicalCallCount: 0,
        providerAttemptCount: 0,
        providerAttemptStatuses: {
          succeeded: 0, failed: 0, cancelled: 0, timedOut: 0, streamAborted: 0
        },
        providerAttemptInputTokens: {
          complete: true, knownObservationCount: 0, observationCount: 0, value: 0
        },
        providerAttemptOutputTokens: {
          complete: true, knownObservationCount: 0, observationCount: 0, value: 0
        },
        providerAttemptCacheHitTokens: {
          complete: true, knownObservationCount: 0, observationCount: 0, value: 0
        },
        providerAttemptCacheMissTokens: {
          complete: true, knownObservationCount: 0, observationCount: 0, value: 0
        },
        providerAttemptCacheRate: { known: false, numerator: 0, denominator: 0 }
      }
    }, {
      ...common,
      kind: 'turn_failed',
      seq: highestSeq,
      publicationSlot: 'terminal',
      publicationEventId: '7'.repeat(64),
      publicationPayloadDigest: '8'.repeat(64),
      status: 'failed',
      terminalReason: 'provider_failure',
      code: 'case_terminal_provider_failure',
      message: terminalMessage,
      error: terminalMessage,
      itemId: terminalItemId
    }]
    const candidateAcceptedFinal = {
      schemaVersion: 2,
      purpose: 'analytix.accepted-final-delivery-batch/v2',
      kind: 'accepted_final_batch',
      batchId: '9'.repeat(64),
      threadId,
      turnId,
      seq: highestSeq,
      firstSeq,
      lastSeq: highestSeq,
      timestamp,
      publicationCommitId,
      eventManifestDigest: 'b'.repeat(64),
      publicationAuthority: {
        schemaVersion: 'accepted-final-delivery-seal.v1',
        purpose: 'analytix.accepted-final-delivery-seal/v1',
        sealId: 'c'.repeat(64),
        threadId,
        turnId,
        publicationCommitId,
        acceptedFinalDispositionDigest: 'd'.repeat(64),
        terminalDispositionId: 'e'.repeat(64),
        eventManifestDigest: 'b'.repeat(64),
        sequencedEventsDigest: 'f'.repeat(64),
        batchId: '9'.repeat(64),
        firstSeq,
        lastSeq: highestSeq,
        timestamp,
        authorityAlgorithm: 'Ed25519',
        authorityKeyId: '0'.repeat(64),
        authorityPublicKey: 'A'.repeat(43),
        authoritySignature: 'B'.repeat(86)
      },
      events
    }
    expect(AcceptedFinalDeliveryBatchV2Schema.safeParse(candidateAcceptedFinal).success).toBe(true)
    const contextAcceptedFinal = structuredClone(candidateAcceptedFinal)
    contextAcceptedFinal.events[0].item.acceptedFinalView.terminalReason = 'semantic_failure'
    contextAcceptedFinal.events[1].item.code = 'case_terminal_semantic_failure'
    contextAcceptedFinal.events[3].terminalReason = 'semantic_failure'
    contextAcceptedFinal.events[3].code = 'case_terminal_semantic_failure'
    const contextAdmission = {
      kind: 'pipeline_stage', seq: 22, timestamp, threadId, turnId,
      stage: 'provider_admission_rejected', label: 'Provider admission rejected',
      details: {
        reasonCode: 'context_window_hard_limit',
        projectedRequestTokens: 900,
        hardThresholdTokens: 850,
        providerAttemptCount: 0
      }
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const acked: Array<{ seq: number; acceptedFinal?: Record<string, string> }> = []
    let replayEvents: Array<Record<string, any>> = [{
      kind: 'heartbeat', seq: baselineSeq, timestamp, threadId
    }, {
      kind: 'turn_started', seq: 21, timestamp, threadId, turnId, status: 'running'
    }, contextAdmission, contextAcceptedFinal]
    const api = {
      onSseEvent: (handler: (payload: any) => void) => { callbacks.event = handler; return vi.fn() },
      onSseEnd: (handler: (payload: any) => void) => { callbacks.end = handler; return vi.fn() },
      onSseError: (handler: (payload: any) => void) => { callbacks.error = handler; return vi.fn() },
      startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
        for (const event of replayEvents) callbacks.event({ streamId, events: [event] })
        return { streamId }
      }),
      ackSseEvent: vi.fn(async (
        streamId: string,
        seq: number,
        acceptedFinal?: Record<string, string>
      ) => {
        acked.push({ seq, acceptedFinal })
        if (seq === highestSeq && (
          acceptedFinal?.batchId !== candidateAcceptedFinal.batchId ||
          acceptedFinal?.threadId !== candidateAcceptedFinal.threadId ||
          acceptedFinal?.turnId !== candidateAcceptedFinal.turnId ||
          acceptedFinal?.publicationCommitId !== candidateAcceptedFinal.publicationCommitId
        )) return false
        if (seq === highestSeq) queueMicrotask(() => callbacks.end({ streamId }))
        return true
      }),
      stopSse: vi.fn(async () => true)
    }

    const observed = await observeProviderFailureDiagnosticViaSse(
      api,
      threadId,
      baselineSeq,
      turnId,
      highestSeq,
      priorTurnId,
      100
    )

    expect(observed).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'host_context_budget_observed',
      threadId,
      turnId,
      baselineSeq,
      terminalSeq: highestSeq,
      highestSeq,
      reasonCode: 'context_window_hard_limit',
      providerKind: 'host_context_budget',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))
    expect(acked.map(({ seq }) => seq)).toEqual([21, 22, highestSeq])
    expect(acked.at(-1)?.acceptedFinal).toEqual({
      batchId: candidateAcceptedFinal.batchId,
      threadId: candidateAcceptedFinal.threadId,
      turnId: candidateAcceptedFinal.turnId,
      publicationCommitId: candidateAcceptedFinal.publicationCommitId
    })
    expect(JSON.stringify(observed)).not.toContain('deepseek-v4-flash')
    expect(JSON.stringify(observed)).not.toContain(terminalMessage)

    const run = async (events: Array<Record<string, any>>) => {
      replayEvents = events
      acked.length = 0
      return observeProviderFailureDiagnosticViaSse(
        api,
        threadId,
        baselineSeq,
        turnId,
        highestSeq,
        priorTurnId,
        100
      )
    }
    const providerError = {
      kind: 'pipeline_stage', seq: 22, timestamp, threadId, turnId,
      stage: 'provider_error', label: 'Provider stream failed',
      details: { reasonCode: 'provider_unavailable' }
    }
    const started = replayEvents[1]
    const heartbeat = replayEvents[0]
    const cursorAdvanced = {
      kind: 'cursor_advanced',
      seq: 22,
      timestamp,
      threadId,
      reason: 'restricted_content_removed'
    }
    const streamAbortedAcceptedFinal = structuredClone(candidateAcceptedFinal)
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerLogicalCallCount = 6
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptCount = 6
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptStatuses = {
      succeeded: 5,
      failed: 0,
      cancelled: 0,
      timedOut: 0,
      streamAborted: 1
    }
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptInputTokens = {
      complete: true, knownObservationCount: 6, observationCount: 6, value: 610
    }
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptOutputTokens = {
      complete: true, knownObservationCount: 6, observationCount: 6, value: 90
    }
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptCacheHitTokens = {
      complete: false, knownObservationCount: 0, observationCount: 6
    }
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptCacheMissTokens = {
      complete: false, knownObservationCount: 0, observationCount: 6
    }
    streamAbortedAcceptedFinal.events[2].cacheDiagnostics.providerAttemptCacheRate = {
      known: false, numerator: 0, denominator: 0
    }
    expect(await run([
      heartbeat,
      started,
      cursorAdvanced,
      streamAbortedAcceptedFinal
    ])).toEqual(expect.objectContaining({
      observed: true,
      observationCode: 'provider_failure_terminal_observed',
      reasonCode: 'provider_stream_interrupted',
      providerKind: 'unknown',
      providerStatus: null,
      providerRetryable: null,
      providerAuthStatus: 'not_observed'
    }))
    const invalidCases = [{
      name: 'cross-thread',
      events: [heartbeat, started, providerError, { ...candidateAcceptedFinal, threadId: 'thread-other' }],
      observationCode: 'provider_failure_event_scope_invalid'
    }, {
      name: 'cross-turn',
      events: [heartbeat, started, providerError, { ...candidateAcceptedFinal, turnId: 'turn-other' }],
      observationCode: 'provider_failure_event_scope_invalid'
    }, {
      name: 'duplicate-failure',
      events: [heartbeat, started, providerError, { ...providerError, seq: 23 }, candidateAcceptedFinal],
      observationCode: 'provider_failure_duplicate'
    }, {
      name: 'invalid-current-schema',
      events: [heartbeat, started, providerError, { ...candidateAcceptedFinal, schemaVersion: 1 }],
      observationCode: 'provider_failure_event_scope_invalid'
    }, {
      name: 'candidate-terminal-missing',
      events: [heartbeat, started, providerError],
      observationCode: 'provider_failure_terminal_missing'
    }, {
      name: 'provider-diagnostic-missing',
      events: [heartbeat, started, cursorAdvanced, candidateAcceptedFinal],
      observationCode: 'provider_failure_diagnostic_missing'
    }, {
      name: 'thread-snapshot-cursor-mismatch',
      events: [
        { ...heartbeat, seq: baselineSeq + 1 },
        started,
        providerError,
        candidateAcceptedFinal
      ],
      observationCode: 'provider_failure_event_sequence_invalid'
    }]
    for (const candidate of invalidCases) {
      expect(await run(candidate.events), candidate.name).toEqual(expect.objectContaining({
        observed: false,
        observationCode: candidate.observationCode
      }))
    }
  })

  it('rejects provider-failure replay when terminal authority, scope, schema, or closure is not exact', async () => {
    const { observeProviderFailureDiagnosticViaSse } = await milestoneModule()
    const threadId = 'thread-provider-real-negative'
    const turnId = 'turn-provider-real-negative'
    const baselineSeq = 20
    const highestSeq = 25
    const providerEvent = {
      kind: 'pipeline_stage', seq: 21, timestamp: '2026-08-05T04:00:00Z', threadId, turnId,
      stage: 'provider_error', label: 'Provider stream failed',
      details: {
        reasonCode: 'provider_request_rejected',
        providerError: { kind: 'request', status: 400, retryable: false, authStatus: 'none' }
      }
    }
    const callbacks: Record<string, (payload: any) => void> = {}
    const run = async (events: any[], end = true) => {
      const api = {
        onSseEvent: (handler: (payload: any) => void) => { callbacks.event = handler; return vi.fn() },
        onSseEnd: (handler: (payload: any) => void) => { callbacks.end = handler; return vi.fn() },
        onSseError: (handler: (payload: any) => void) => { callbacks.error = handler; return vi.fn() },
        startSse: vi.fn(async (_thread: string, _since: number, streamId: string) => {
          for (const event of events) callbacks.event({ streamId, events: [event] })
          return { streamId }
        }),
        ackSseEvent: vi.fn(async (streamId: string, seq: number) => {
          if (end && (seq === highestSeq || (events.length === 1 && seq === 21))) {
            queueMicrotask(() => callbacks.end({ streamId }))
          }
          return true
        }),
        stopSse: vi.fn(async () => true)
      }
      return observeProviderFailureDiagnosticViaSse(api, threadId, baselineSeq, turnId, highestSeq, 15)
    }
    expect(await run([{ ...providerEvent, threadId: 'thread-other' }])).toEqual(expect.objectContaining({
      observed: false, observationCode: 'provider_failure_event_scope_invalid'
    }))
    expect(await run([{ ...providerEvent, details: { providerError: { ...providerEvent.details.providerError, body: 'PRIVATE_BODY' } } }])).toEqual(expect.objectContaining({
      observed: false, observationCode: 'provider_failure_diagnostic_invalid'
    }))
    expect(await run([providerEvent, { ...providerEvent, seq: 22 }])).toEqual(expect.objectContaining({
      observed: false, observationCode: 'provider_failure_duplicate'
    }))
    expect(await run([providerEvent], false)).toEqual(expect.objectContaining({
      observed: false, observationCode: 'provider_failure_terminal_missing'
    }))
    expect(await run([{ kind: 'accepted_final_batch', seq: 21, threadId, turnId }])).toEqual(expect.objectContaining({
      observed: false, observationCode: 'provider_failure_event_scope_invalid'
    }))
  })

  it('binds the terminal to the last contiguous tool-failure suffix and reports invalid-argument history', async () => {
    const {
      observeToolFailureDiagnosticViaSse,
      toolFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-tool-failure'
    const turnId = 'turn-tool-failure'
    const baselineSeq = 30
    const highestSeq = 42
    const privateToolName = 'read'
    const { raw, cleanup } = await observeToolFailureFixture(
      observeToolFailureDiagnosticViaSse,
      {
        threadId,
        turnId,
        baselineSeq,
        highestSeq,
        guardEvents: [
          toolFailureGuardFixture({
            seq: 31, threadId, turnId, toolName: 'bash',
            guardKind: 'invalid_tool_arguments', stormCount: 1
          }),
          toolFailureGuardFixture({
            seq: 32, threadId, turnId, toolName: 'bash', stormCount: 3
          }),
          toolFailureGuardFixture({
            seq: 33, threadId, turnId, toolName: privateToolName, stormCount: 3
          }),
          toolFailureGuardFixture({
            seq: 34, threadId, turnId, toolName: privateToolName, stormCount: 4
          }),
          toolFailureGuardFixture({
            seq: 35, threadId, turnId, toolName: privateToolName,
            guardKind: 'invalid_tool_arguments', stormCount: 2
          }),
          toolFailureGuardFixture({
            seq: 36, threadId, turnId, toolName: privateToolName, stormCount: 5
          }),
          toolFailureGuardFixture({
            seq: 37, threadId, turnId, toolName: privateToolName, stormCount: 6
          })
        ]
      }
    )
    const evidence = toolFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })

    expect(evidence).toEqual({
      observed: true,
      observationCode: 'tool_failure_observed',
      evidenceCode: 'tool_failure_evidence_observed',
      toolFailureGuardBound: true,
      guardCount: 4,
      maxStormCount: 6,
      guardKind: 'tool_failure',
      toolCategory: 'read',
      toolNameHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      invalidArgumentGuardCount: 1,
      invalidArgumentMaxStormCount: 2,
      invalidArgumentToolCategory: 'read',
      invalidArgumentToolNameHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      terminalReason: 'tool_failure',
      terminalCode: 'tool_failure_storm',
      diagnosticDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    })
    expect(raw).not.toHaveProperty('toolName')
    expect(JSON.stringify(raw)).not.toContain(`"toolName":"${privateToolName}"`)
    expect(JSON.stringify(raw)).not.toContain('PRIVATE_')
    expect(JSON.stringify(evidence)).not.toContain(threadId)
    expect(cleanup.every((item) => item.mock.calls.length === 1)).toBe(true)

    expect(toolFailureDiagnosticEvidence({ ...raw, terminalCode: 'turn_failed' }, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'tool_failure_observed',
      evidenceCode: 'tool_failure_evidence_binding_invalid',
      diagnosticDigest: ''
    }))
    expect(toolFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: 'turn-other',
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'tool_failure_observed',
      evidenceCode: 'tool_failure_evidence_scope_invalid'
    }))
  })

  it.each(['write_file', 'edit_file'])(
    'classifies sanitized %s failure diagnostics as write',
    async (toolName) => {
      const { observeToolFailureDiagnosticViaSse } = await milestoneModule()
      const threadId = `thread-tool-category-${toolName}`
      const turnId = `turn-tool-category-${toolName}`
      const baselineSeq = 70
      const highestSeq = 77
      const { raw } = await observeToolFailureFixture(observeToolFailureDiagnosticViaSse, {
        threadId,
        turnId,
        baselineSeq,
        highestSeq,
        guardEvents: [toolFailureGuardFixture({
          seq: baselineSeq + 1,
          threadId,
          turnId,
          toolName,
          stormCount: 3
        })]
      })

      expect(raw).toEqual(expect.objectContaining({
        observed: true,
        observationCode: 'tool_failure_observed',
        toolCategory: 'write'
      }))
      expect(raw).not.toHaveProperty('toolName')
    }
  )

  it.each([
    'tool_failure_storm',
    'tool_invalid_arguments_storm'
  ] as const)('reports invalid-argument history without treating %s as loop-guard authority', async (terminalCode) => {
    const {
      observeToolFailureDiagnosticViaSse,
      toolFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-invalid-arguments'
    const turnId = 'turn-invalid-arguments'
    const baselineSeq = 50
    const highestSeq = 57
    const { raw } = await observeToolFailureFixture(observeToolFailureDiagnosticViaSse, {
      threadId,
      turnId,
      baselineSeq,
      highestSeq,
      terminalCode,
      guardEvents: [1, 2].map((stormCount) => toolFailureGuardFixture({
        seq: baselineSeq + stormCount,
        threadId,
        turnId,
        toolName: 'bash',
        guardKind: 'invalid_tool_arguments',
        stormCount
      }))
    })
    const evidence = toolFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })

    expect(evidence).toEqual(expect.objectContaining({
      observed: false,
      observationCode: 'tool_failure_guard_history_observed',
      evidenceCode: 'tool_failure_evidence_guard_history_only',
      toolFailureGuardBound: false,
      guardCount: 0,
      maxStormCount: 0,
      guardKind: 'none',
      toolCategory: 'none',
      toolNameHash: '',
      invalidArgumentGuardCount: 2,
      invalidArgumentMaxStormCount: 2,
      invalidArgumentToolCategory: 'bash',
      invalidArgumentToolNameHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      terminalCode
    }))
    expect(JSON.stringify(evidence)).not.toContain(threadId)
  })

  it.each([
    ['malformed guard', 'tool_failure_guard_invalid', {
      guardKind: 'unknown_guard', turnId: 'turn-observer-negative'
    }],
    ['outer scope mismatch', 'tool_failure_event_scope_invalid', {
      guardKind: 'tool_failure', turnId: 'turn-other'
    }]
  ])('rejects %s with a safe observer-layer code', async (_label, code, mutation) => {
    const {
      observeToolFailureDiagnosticViaSse,
      toolFailureDiagnosticEvidence
    } = await milestoneModule()
    const threadId = 'thread-observer-negative'
    const turnId = 'turn-observer-negative'
    const baselineSeq = 60
    const highestSeq = 67
    const { raw } = await observeToolFailureFixture(observeToolFailureDiagnosticViaSse, {
      threadId,
      turnId,
      baselineSeq,
      highestSeq,
      guardEvents: [toolFailureGuardFixture({
        seq: baselineSeq + 1,
        threadId,
        turnId: mutation.turnId,
        toolName: 'read',
        guardKind: mutation.guardKind,
        stormCount: 3
      })]
    })
    const evidence = toolFailureDiagnosticEvidence(raw, {
      expectedThreadId: threadId,
      expectedTurnId: turnId,
      expectedBaselineSeq: baselineSeq,
      expectedHighestSeq: highestSeq
    })

    expect(raw).toEqual(expect.objectContaining({ observed: false, observationCode: code }))
    expect(evidence).toEqual(expect.objectContaining({
      observed: false,
      observationCode: code,
      evidenceCode: 'tool_failure_evidence_observer_rejected',
      diagnosticDigest: ''
    }))
    expect(JSON.stringify(raw)).not.toContain('PRIVATE_')
  })

  it('records provider failure replays on failed formal turns without changing their gates', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const baseline = script.indexOf(
      'const agentBaselineSeq = planned.observation?.thread?.latestSeq'
    )
    const submit = script.indexOf('const submit = await cdpComposerSubmit', baseline)
    const failedBranch = script.indexOf(
      "if (ordinaryDisposition !== 'completed' && latestAgentTurn?.status === 'failed'",
      submit
    )
    const replay = script.indexOf(
      'const rawProviderFailure = await observeProviderFailureDiagnostic({',
      failedBranch
    )
    const hostEvidence = script.indexOf(
      'const bashHostObservation = await observeHostToolExecution',
      replay
    )
    const formalGate = script.indexOf('if (ordinaryFailureCode)', hostEvidence)
    const formalThrow = script.indexOf('throw new Error(ordinaryFailureCode)', formalGate)

    expect(baseline).toBeGreaterThan(0)
    expect(baseline).toBeLessThan(submit)
    expect(failedBranch).toBeGreaterThan(submit)
    expect(replay).toBeGreaterThan(failedBranch)
    expect(hostEvidence).toBeGreaterThan(replay)
    expect(formalGate).toBeGreaterThan(hostEvidence)
    expect(formalThrow).toBeGreaterThan(formalGate)
    expect(script.match(/const rawProviderFailure = await observeProviderFailureDiagnostic\(\{/g))
      .toHaveLength(2)
    expect(script).toContain(
      'priorCompletedTurnId: agentBaselineEvidence.thread?.turns?.at(-1)?.id'
    )
    expect(script).toContain(
      'priorCompletedTurnId: protectedFundsBaselineEvidence.thread?.turns?.at(-1)?.id'
    )
    expect(script).toContain('ordinaryCandidateProviderFailureObserved')
    expect(script).toContain('ordinaryCandidateProviderFailureDiagnosticDigest')
    expect(script).toContain('failedChildDiagnosticTurnErrorCode')
    expect(script).toContain('failedChildDiagnosticTerminalErrorItemAuthorityBound')
    expect(script).toContain('failedChildDiagnosticInventoryDigest')
    expect(script).toContain('failedChildDiagnosticChildThreadIdHash')
    expect(script).toContain('failedChildDiagnosticTarget(firstEvidence)')
    expect(script).toContain('failedChildTerminalDiagnosticEvidence(')
    expect(script).toContain('exactThreadId: failedChildTarget.childThreadId')
    expect(script).not.toContain('failedChildProviderFailureScope(')
    expect(script).not.toContain('failedChildDiagnosticChildThreadId:')
    expect(script).toContain(
      "['tool_failure_storm', 'tool_invalid_arguments_storm'].includes("
    )
    expect(script).toContain('const rawToolFailure = await observeToolFailureDiagnostic({')
    expect(script).toContain('ordinaryCandidateToolFailureObservationCode')
    expect(script).toContain('ordinaryCandidateToolFailureEvidenceCode')
    expect(script).toContain('ordinaryCandidateToolFailureGuardBound')
    expect(script).toContain('ordinaryCandidateToolFailureGuardCount')
    expect(script).toContain('ordinaryCandidateToolFailureMaxStormCount')
    expect(script).toContain('ordinaryCandidateToolFailureToolCategory')
    expect(script).toContain('ordinaryCandidateToolFailureToolNameHash')
    expect(script).toContain('ordinaryCandidateInvalidToolArgumentGuardCount')
    expect(script).toContain('ordinaryCandidateInvalidToolArgumentMaxStormCount')
    expect(script).toContain('ordinaryCandidateInvalidToolArgumentToolCategory')
    expect(script).toContain('ordinaryCandidateInvalidToolArgumentToolNameHash')
    expect(script).toContain('ordinaryCandidateToolFailureDiagnosticDigest')
    expect(script).toContain('longContextCandidateTurnErrorCode')
    expect(script).toContain('longContextCandidateTerminalReasonClass')
    expect(script).toContain('longContextCandidateInventoryDigest')
    expect(script).toContain('longContextProviderFailureObserved')
    expect(script).toContain('longContextProviderFailureObservationCode')
    expect(script).toContain('longContextProviderFailureReasonCode')
    expect(script).toContain('longContextProviderFailureDiagnosticDigest')
    expect(script).not.toContain('ordinaryCandidateProviderFailureBody')
    expect(script).not.toContain('ordinaryCandidateProviderFailureMessage')
    expect(script).not.toContain('ordinaryCandidateToolFailureToolName:')
    expect(script).not.toContain('longContextProviderFailureBody')
    expect(script).not.toContain('longContextProviderFailureMessage')
  })

  it('keeps ordinary runtime readiness independent from funds transport diagnostics', async () => {
    const { runtimePublicSeamEvidence } = await milestoneModule()

    const noDiagnostics = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([]),
      43210
    )
    expect(noDiagnostics).toEqual(expect.objectContaining({
      ok: true,
      runtimeToolsOk: true,
      runtimeThreadListProbeOk: true,
      runtimeThreadListProbeStatus: 200,
      ordinaryCatalogNonempty: true,
      fundsTransportAvailable: false,
      fundsExecutionUnavailable: false,
      fundsServerDiagnosticCount: 0
    }))

    const connectedFunds = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'analytix_funds',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        connectable: true,
        toolCount: 2
      }]),
      43210
    )
    expect(connectedFunds).toEqual(expect.objectContaining({
      ok: true,
      runtimeToolsOk: true,
      fundsTransportAvailable: true,
      fundsExecutionUnavailable: false,
      fundsServerDiagnosticCount: 1
    }))

    const explicitHostRejection = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'analytix_funds',
        status: 'unavailable',
        failureCode: 'funds_installed_state_invalid',
        enabled: false,
        available: false,
        toolCount: 0
      }]),
      43210
    )
    expect(explicitHostRejection).toEqual(expect.objectContaining({
      ok: true,
      runtimeToolsOk: true,
      fundsTransportAvailable: false,
      fundsExecutionUnavailable: true,
      fundsServerDiagnosticCount: 1
    }))

    const unrecognizedRejection = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'analytix_funds',
        status: 'unavailable',
        failureCode: 'unrecognized_or_unbounded_failure',
        enabled: false,
        available: false,
        toolCount: 0
      }]),
      43210
    )
    expect(unrecognizedRejection.fundsExecutionUnavailable).toBe(false)

    const unavailableThreadList = runtimePublicSeamEvidence({
      ...runtimePublicSeamObservation([]),
      runtimeThreadListProbeOk: false,
      runtimeThreadListProbeStatus: 502
    }, 43210)
    expect(unavailableThreadList).toEqual(expect.objectContaining({
      ok: false,
      healthOk: true,
      runtimeInfoOk: true,
      runtimeToolsOk: true,
      runtimeThreadListProbeOk: false,
      runtimeThreadListProbeStatus: 502
    }))

    const accidentalHostServer = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'host-specific-docs',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        toolCount: 3,
        toolContractQuarantineCount: 0
      }]),
      43210
    )
    expect(accidentalHostServer).toEqual(expect.objectContaining({
      ok: true,
      ordinaryMCPAvailable: false,
      ordinaryMCPServerCount: 1,
      ordinaryMCPServerId: ''
    }))

    const packagedSchedule = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'gui_schedule',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        transport: 'stdio',
        authStatus: 'none',
        trustScope: 'user',
        schemaHintAvailable: true,
        connectable: true,
        toolCount: 8,
        toolContractQuarantineCount: 0
      }]),
      43210
    )
    expect(packagedSchedule).toEqual(expect.objectContaining({
      ok: true,
      ordinaryMCPAvailable: true,
      ordinaryMCPServerCount: 1,
      ordinaryMCPServerId: 'gui_schedule',
      ordinaryMCPToolCount: 8
    }))

    const quarantinedSchedule = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'gui_schedule',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        transport: 'stdio',
        authStatus: 'none',
        trustScope: 'user',
        schemaHintAvailable: true,
        connectable: true,
        toolCount: 8,
        toolContractQuarantineCount: 1
      }]),
      43210
    )
    expect(quarantinedSchedule.ordinaryMCPAvailable).toBe(false)

    const unexpectedSecondOrdinaryServer = runtimePublicSeamEvidence(
      runtimePublicSeamObservation([{
        id: 'gui_schedule',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        transport: 'stdio',
        authStatus: 'none',
        trustScope: 'user',
        schemaHintAvailable: true,
        connectable: true,
        toolCount: 8,
        toolContractQuarantineCount: 0
      }, {
        id: 'host-specific-docs',
        status: 'connected',
        enabled: true,
        available: true,
        connected: true,
        toolCount: 1,
        toolContractQuarantineCount: 0
      }]),
      43210
    )
    expect(unexpectedSecondOrdinaryServer).toEqual(expect.objectContaining({
      ordinaryMCPAvailable: false,
      ordinaryMCPServerCount: 2,
      ordinaryMCPServerId: ''
    }))
  })

  it('establishes a real missing bundled-funds authority seam without creating authority state', async () => {
    const {
      missingBundledFundsMaterializationAuthorityEvidence,
      prepareMissingBundledFundsMaterializationAuthoritySeam
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const runtimeDataDir = join(root, 'runtime-data')
    mkdirSync(runtimeDataDir, { mode: 0o700 })

    const seam = prepareMissingBundledFundsMaterializationAuthoritySeam(runtimeDataDir)
    expect(seam.evidence).toEqual(expect.objectContaining({
      preconditionEstablished: true,
      stateRootSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      authorityPathSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      stateDirectoryEmpty: true,
      authorityAbsent: true
    }))
    expect(missingBundledFundsMaterializationAuthorityEvidence(seam)).toEqual(
      expect.objectContaining({
        ok: true,
        stateDirectoryIdentityPreserved: true,
        stateDirectoryEmpty: true,
        authorityAbsent: true
      })
    )

    mkdirSync(join(runtimeDataDir, 'private', 'plugin-materialization-authority'), {
      recursive: true,
      mode: 0o700
    })
    writeFileSync(seam.authorityPath, '{}\n', { encoding: 'utf8', mode: 0o600 })
    expect(missingBundledFundsMaterializationAuthorityEvidence(seam)).toEqual(
      expect.objectContaining({ ok: false, authorityAbsent: false })
    )
  })

  it('accepts only the isolated one-root packaged Skill catalog', async () => {
    const {
      packagedMilestoneASkillID,
      packagedSkillSourceBindingEvidence
    } = await milestoneModule()
    const packagedSkill = source(
      'vendor/analytix-computer-use/plugins/analytix-computer-use/skills/analytix-computer-use/SKILL.md'
    )
    const catalog = {
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 1,
      skillCount: 1,
      validationErrorCount: 0,
      skills: [{ id: 'analytix-computer-use' }]
    }

    expect(packagedMilestoneASkillID(catalog)).toBe('analytix-computer-use')
    expect(packagedMilestoneASkillID({
      ...catalog,
      skills: [{ id: 'host-dependent-first' }]
    })).toBe('')
    expect(packagedMilestoneASkillID({
      ...catalog,
      skillCount: 2,
      skills: [
        { id: 'analytix-computer-use' },
        { id: 'host-dependent-second' }
      ]
    })).toBe('')
    expect(packagedMilestoneASkillID({
      ...catalog,
      configuredRootCount: 2
    })).toBe('')
    expect(packagedMilestoneASkillID({
      ...catalog,
      validationErrorCount: 1
    })).toBe('')
    expect(packagedSkill).toContain('name: analytix-computer-use')
    expect(packagedSkill).not.toMatch(/^runAs:/m)
    expect(packagedSkill).not.toMatch(/^context:/m)

    const root = taskOwnedSandbox()
    const sourcePath = join(root, 'source-skill.md')
    const packagedPath = join(root, 'packaged-skill.md')
    writeFileSync(sourcePath, packagedSkill, 'utf8')
    writeFileSync(packagedPath, packagedSkill, 'utf8')
    expect(packagedSkillSourceBindingEvidence(sourcePath, packagedPath)).toEqual(
      expect.objectContaining({
        ok: true,
        sourceSha256: sha256(packagedSkill),
        packagedSha256: sha256(packagedSkill)
      })
    )
    writeFileSync(packagedPath, `${packagedSkill}\nchanged`, 'utf8')
    expect(packagedSkillSourceBindingEvidence(sourcePath, packagedPath).ok).toBe(false)
  })

  it('pins the package-bound GUI schedule non-mutating list canary and strict empty schema', async () => {
    const {
      packagedScheduleMcpConfigEvidence,
      packagedScheduleSourceContractEvidence
    } = await milestoneModule()
    const harness = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const scheduleConfig = source('src/main/claw-schedule-mcp-config.ts')
    const scheduleServer = source('src/main/claw-schedule-mcp-server.ts')
    const listRegistrationStart = scheduleServer.indexOf('const registerListTool')
    const listRegistrationEnd = scheduleServer.indexOf(
      'const registerCreateTool',
      listRegistrationStart
    )
    const listRegistration = scheduleServer.slice(
      listRegistrationStart,
      listRegistrationEnd
    )

    expect(harness).toContain("PACKAGED_ORDINARY_MCP_SERVER_ID = 'gui_schedule'")
    expect(harness).toContain("'mcp__gui_schedule__gui_schedule_list'")
    expect(harness).not.toContain('ordinaryMCPHostObservation')
    expect(harness).not.toContain('ordinaryMCPHostInvocationBound')
    expect(scheduleConfig).toContain("GUI_SCHEDULE_MCP_SERVER_NAME = 'gui_schedule'")
    expect(scheduleConfig).toContain("GUI_SCHEDULE_MCP_NODE_ENTRY = 'out/main/claw-schedule-mcp-node-entry.js'")
    expect(listRegistration).toContain('registerListTool("gui_schedule_list")')
    expect(listRegistration).toContain('readOnlyHint: true')
    expect(listRegistration).toContain('destructiveHint: false')
    expect(listRegistration).toContain('idempotentHint: true')
    expect(listRegistration).toContain('openWorldHint: false')
    expect(packagedScheduleSourceContractEvidence()).toEqual(expect.objectContaining({
      ok: true,
      strictEmptyInputSchemaBound: true,
      nonMutatingListImplementationBound: true,
      packagedEntrypointBound: true,
      configSourceSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      serverSourceSha256: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))

    const root = taskOwnedSandbox()
    const isolatedHome = join(root, 'home')
    const packagedExecutable = join(root, 'package', 'analytix')
    const schedulePort = 43123
    const entrypoint = join(
      root,
      'package',
      'resources',
      'app.asar',
      'out',
      'main',
      'claw-schedule-mcp-node-entry.js'
    )
    const mcpConfigPath = join(isolatedHome, '.analytix', 'mcp.json')
    mkdirSync(join(isolatedHome, '.analytix'), { recursive: true, mode: 0o700 })
    mkdirSync(join(root, 'package'), { recursive: true, mode: 0o700 })
    writeFileSync(packagedExecutable, 'packaged-electron-helper', { mode: 0o700 })
    const actualMcpConfig = {
      timeouts: {
        connect_timeout: 10,
        execute_timeout: 60,
        read_timeout: 120
      },
      servers: {
        gui_schedule: {
          command: packagedExecutable,
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
      }
    }
    writeFileSync(mcpConfigPath, `${JSON.stringify(actualMcpConfig, null, 2)}\n`, {
      mode: 0o600
    })
    const target = { platform: 'linux', arch: 'x64', key: 'linux-x64' }
    expect(packagedScheduleMcpConfigEvidence({
      isolatedHome,
      appPath: packagedExecutable,
      target,
      schedulePort
    })).toEqual(expect.objectContaining({
      ok: true,
      exactConfigBound: true,
      configMode: 0o600,
      configLinkCount: 1,
      configSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      helperExecutableSha256: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    actualMcpConfig.servers.gui_schedule.args[0] = '/host/accidental/helper.mjs'
    writeFileSync(mcpConfigPath, `${JSON.stringify(actualMcpConfig, null, 2)}\n`, {
      mode: 0o600
    })
    expect(packagedScheduleMcpConfigEvidence({
      isolatedHome,
      appPath: packagedExecutable,
      target,
      schedulePort
    })).toEqual(expect.objectContaining({ ok: false, exactConfigBound: false }))
  })

  it('requires exact result-turn task, Skill, bash, and package-bound MCP canary counts', async () => {
    const {
      packagedResultTurnContractEvidence,
      workflowEvidence
    } = await milestoneModule()
    const toolPair = (callId: string, toolName: string): Record<string, any>[] => [{
      kind: 'tool_call',
      status: 'completed',
      callId,
      toolName,
      toolKind: 'builtin'
    }, {
      kind: 'tool_result',
      status: 'completed',
      callId,
      toolName,
      toolKind: 'builtin',
      isError: false,
      output: { status: 'completed' }
    }]
    const failedToolPair = (callId: string, toolName: string): Record<string, any>[] => [{
      kind: 'tool_call',
      status: 'completed',
      callId,
      toolName,
      toolKind: 'builtin'
    }, {
      kind: 'tool_result',
      status: 'completed',
      callId,
      toolName,
      toolKind: 'builtin',
      isError: true,
      output: { status: 'failed' }
    }]
    const thread = {
      id: 'thread_packaged_task_1',
      workspace: '/isolated/repository',
      turns: [{
        id: 'turn_packaged_task_1',
        status: 'completed',
        items: [
          ...toolPair('call_skill_1', 'run_skill'),
          ...toolPair('call_task_1', 'task'),
          ...toolPair('call_bash_test_1', 'bash'),
          ...toolPair('call_bash_git_1', 'bash'),
          ...toolPair('call_schedule_list_1', 'mcp__gui_schedule__gui_schedule_list'),
          {
            kind: 'assistant_text',
            status: 'completed',
            text: 'MILESTONE_A_RESULT_OK'
          }
        ]
      }],
      todos: { items: [] }
    }
    const summary = {
      subagents: [{
        parentThreadId: thread.id,
        parentTurnId: thread.turns[0].id,
        parentToolCallId: 'call_task_1',
        status: 'done',
        rawStatus: 'completed',
        diagnostics: {
          status: 'completed',
          terminal: true,
          paused: false,
          background: false
        },
        toolPolicy: 'readOnly',
        profile: 'milestone-a-readonly',
        maxModelSteps: 8,
        timeBudgetMs: 180_000,
        childRunId: 'child_run_1',
        childThreadId: 'child_thread_1',
        childTurnId: 'child_turn_1',
        canOpenThread: true,
        canReadOutput: false,
        outputWithheld: true,
        factAnswerAllowed: false
      }]
    }
    const runtimeSkills = {
      schemaVersion: 2,
      enabled: true,
      available: true,
      reasonCode: 'available',
      configuredRootCount: 1,
      skillCount: 1,
      validationErrorCount: 0,
      skills: [{ id: 'analytix-computer-use' }]
    }
    const publicSeam = {
      ordinaryMCPAvailable: true,
      ordinaryMCPServerId: 'gui_schedule',
      ordinaryMCPServerCount: 1,
      ordinaryMCPToolCount: 8,
      ordinaryToolCatalogHash: 'a'.repeat(64)
    }
    const artifact = {
      ok: true,
      worktreeSnapshotBinding: { ok: true },
      appAsarSha256: 'b'.repeat(64),
      sourceSkillSha256: 'c'.repeat(64),
      packagedSkillSha256: 'c'.repeat(64),
      packagedSkillSourceMatched: true,
      packagedScheduleSourceContractBound: true,
      packagedScheduleStrictEmptyInputSchemaBound: true,
      packagedScheduleNonMutatingListImplementationBound: true
    }
    const scheduleConfigEvidence = {
      ok: true,
      exactConfigBound: true,
      configMode: 0o600,
      configLinkCount: 1,
      configSha256: 'd'.repeat(64),
      helperExecutableSha256: 'e'.repeat(64)
    }

    const evidence = workflowEvidence({ thread, summary }, '/isolated/repository')
    expect(evidence.toolGroupPassed).toEqual(expect.objectContaining({
      skills: true,
      subagent: true
    }))
    expect(evidence.boundedSubagentPresent).toBe(true)
    expect(packagedResultTurnContractEvidence(
      evidence,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: true,
      taskAttemptCount: 1,
      taskExecutionCount: 1,
      subagentDelegationAttemptCount: 1,
      runSkillAttemptCount: 1,
      runSkillExecutionCount: 1,
      bashAttemptCount: 2,
      bashExecutionCount: 2,
      ordinaryMCPAttemptCount: 1,
      ordinaryMCPExecutionCount: 1,
      allMCPAttemptCount: 1,
      allMCPExecutionCount: 1,
      fundsMCPAttemptCount: 0,
      malformedToolCallAttemptCount: 0,
      taskProfileBound: true,
      taskForegroundBound: true,
      taskStepLimitBound: true,
      taskTimeBudgetBound: true,
      skillCatalogBound: true,
      packageCatalogBound: true,
      packageConfigBound: true,
      strictEmptyInputSchemaBound: true,
      emptyInputBoundByStrictSchema: true
    }))

    const runSkillOnlyThread = structuredClone(thread)
    runSkillOnlyThread.turns[0].items = runSkillOnlyThread.turns[0].items.filter(
      (item: Record<string, any>) => item.toolName !== 'task'
    )
    const runSkillOnly = workflowEvidence(
      { thread: runSkillOnlyThread, summary },
      '/isolated/repository'
    )
    expect(runSkillOnly.toolGroupPassed.skills).toBe(true)
    expect(runSkillOnly.toolGroupPassed.subagent).toBe(false)
    expect(runSkillOnly.boundedSubagentPresent).toBe(false)

    const wrongProfileSummary = structuredClone(summary)
    wrongProfileSummary.subagents[0].profile = 'wrong-profile'
    const wrongProfile = workflowEvidence(
      { thread, summary: wrongProfileSummary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      wrongProfile,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    ).ok).toBe(false)

    for (const [field, value] of [
      ['maxModelSteps', 64],
      ['timeBudgetMs', 0]
    ] as const) {
      const unboundedSummary = structuredClone(summary)
      unboundedSummary.subagents[0][field] = value
      const unbounded = workflowEvidence(
        { thread, summary: unboundedSummary },
        '/isolated/repository'
      )
      expect(unbounded.boundedSubagentPresent).toBe(false)
      expect(packagedResultTurnContractEvidence(
        unbounded,
        runtimeSkills,
        publicSeam,
        artifact,
        scheduleConfigEvidence
      ).ok).toBe(false)
    }

    const duplicateTaskThread = structuredClone(thread)
    duplicateTaskThread.turns[0].items.splice(
      -1,
      0,
      ...toolPair('call_task_2', 'task')
    )
    const duplicateTask = workflowEvidence(
      { thread: duplicateTaskThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      duplicateTask,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({ ok: false, taskExecutionCount: 2 }))

    const duplicateSkillThread = structuredClone(thread)
    duplicateSkillThread.turns[0].items.splice(
      -1,
      0,
      ...toolPair('call_skill_2', 'run_skill')
    )
    const duplicateSkill = workflowEvidence(
      { thread: duplicateSkillThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      duplicateSkill,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({ ok: false, runSkillExecutionCount: 2 }))

    expect(packagedResultTurnContractEvidence(
      evidence,
      {
        ...runtimeSkills,
        skills: [{ id: 'host-dependent-skill' }]
      },
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({ ok: false, skillCatalogBound: false }))

    const backgroundSummary = {
      ...structuredClone(summary),
      subagents: structuredClone(summary.subagents).map((subagent) => ({
        ...subagent,
        background: false
      }))
    }
    backgroundSummary.subagents[0].background = true
    backgroundSummary.subagents[0].diagnostics.background = true
    const backgroundTask = workflowEvidence(
      { thread, summary: backgroundSummary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      backgroundTask,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      taskProfileBound: false,
      taskForegroundBound: false
    }))

    expect(packagedResultTurnContractEvidence(
      evidence,
      runtimeSkills,
      publicSeam,
      {
        ...artifact,
        packagedScheduleStrictEmptyInputSchemaBound: false
      },
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      strictEmptyInputSchemaBound: false,
      emptyInputBoundByStrictSchema: false
    }))

    expect(packagedResultTurnContractEvidence(
      evidence,
      runtimeSkills,
      publicSeam,
      artifact,
      { ...scheduleConfigEvidence, exactConfigBound: false }
    )).toEqual(expect.objectContaining({
      ok: false,
      packageConfigBound: false
    }))

    const extraMCPThread = structuredClone(thread)
    extraMCPThread.turns[0].items.splice(
      -1,
      0,
      ...toolPair('call_unexpected_mcp_1', 'mcp__host_docs__search')
    )
    const extraMCP = workflowEvidence(
      { thread: extraMCPThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      extraMCP,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      ordinaryMCPExecutionCount: 1,
      allMCPExecutionCount: 2,
      exactOrdinaryMCPResultBound: false
    }))

    const extraBashThread = structuredClone(thread)
    extraBashThread.turns[0].items.splice(
      -1,
      0,
      ...toolPair('call_unexpected_bash_1', 'bash')
    )
    const extraBash = workflowEvidence(
      { thread: extraBashThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      extraBash,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({ ok: false, bashExecutionCount: 3 }))

    const failedTaskThenSuccessThread = structuredClone(thread)
    failedTaskThenSuccessThread.turns[0].items.splice(
      -1,
      0,
      ...failedToolPair('call_task_failed_1', 'task')
    )
    const failedTaskThenSuccess = workflowEvidence(
      { thread: failedTaskThenSuccessThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      failedTaskThenSuccess,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      taskAttemptCount: 2,
      taskExecutionCount: 1
    }))

    const failedAlternateDelegationThread = structuredClone(thread)
    failedAlternateDelegationThread.turns[0].items.splice(
      -1,
      0,
      ...failedToolPair('call_parallel_failed_1', 'parallel_tasks')
    )
    const failedAlternateDelegation = workflowEvidence(
      { thread: failedAlternateDelegationThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      failedAlternateDelegation,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      taskAttemptCount: 1,
      subagentDelegationAttemptCount: 2
    }))

    const failedOtherMCPThread = structuredClone(thread)
    failedOtherMCPThread.turns[0].items.splice(
      -1,
      0,
      ...failedToolPair('call_other_mcp_failed_1', 'mcp__host_docs__search')
    )
    const failedOtherMCP = workflowEvidence(
      { thread: failedOtherMCPThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      failedOtherMCP,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      allMCPAttemptCount: 2,
      allMCPExecutionCount: 1,
      exactOrdinaryMCPResultBound: false
    }))

    const failedFundsMCPThread = structuredClone(thread)
    failedFundsMCPThread.turns[0].items.splice(
      -1,
      0,
      ...failedToolPair(
        'call_funds_mcp_failed_1',
        'mcp__analytix_funds__analyze_account_flows'
      )
    )
    const failedFundsMCP = workflowEvidence(
      { thread: failedFundsMCPThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      failedFundsMCP,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      allMCPAttemptCount: 2,
      fundsMCPAttemptCount: 1,
      exactOrdinaryMCPResultBound: false
    }))

    const unsettledExtraBashThread = structuredClone(thread)
    unsettledExtraBashThread.turns[0].items.splice(-1, 0, {
      kind: 'tool_call',
      status: 'pending',
      callId: 'call_unsettled_bash_1',
      toolName: 'bash',
      toolKind: 'builtin'
    })
    const unsettledExtraBash = workflowEvidence(
      { thread: unsettledExtraBashThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      unsettledExtraBash,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      bashAttemptCount: 3,
      bashExecutionCount: 2
    }))

    const malformedAttemptThread = structuredClone(thread)
    malformedAttemptThread.turns[0].items.splice(-1, 0, {
      kind: 'tool_call',
      status: 'failed'
    })
    const malformedAttempt = workflowEvidence(
      { thread: malformedAttemptThread, summary },
      '/isolated/repository'
    )
    expect(packagedResultTurnContractEvidence(
      malformedAttempt,
      runtimeSkills,
      publicSeam,
      artifact,
      scheduleConfigEvidence
    )).toEqual(expect.objectContaining({
      ok: false,
      malformedToolCallAttemptCount: 1
    }))
  })

  it('selects one failed child for safe diagnosis without satisfying the bounded-subagent contract', async () => {
    const {
      failedChildDiagnosticEvidence,
      failedChildTerminalDiagnosticEvidence,
      failedChildTerminalTurnEvidence,
      finalizeMilestoneAReport,
      packagedResultTurnContractEvidence,
      workflowEvidence
    } = await milestoneModule()
    const workspace = '/isolated/repository'
    const parentThreadId = 'thread-failed-child-parent'
    const parentTurnId = 'turn-failed-child-parent'
    const parentToolCallId = 'call-failed-child-parent'
    const thread = {
      id: parentThreadId,
      workspace,
      turns: [{ id: parentTurnId, status: 'completed', items: [
        { kind: 'tool_call', status: 'completed', callId: parentToolCallId,
          toolName: 'task', toolKind: 'builtin' },
        { kind: 'tool_result', status: 'failed', callId: parentToolCallId,
          toolName: 'task', toolKind: 'builtin', isError: true,
          output: { status: 'failed' } }
      ] }]
    }
    const summary = {
      subagents: [{
        parentThreadId,
        parentTurnId,
        parentToolCallId,
        childRunId: 'child-run-failed-1',
        childThreadId: 'thread-child-failed-1',
        profile: 'milestone-a-readonly',
        toolPolicy: 'readOnly',
        status: 'terminal',
        rawStatus: 'failed',
        background: false,
        diagnostics: { status: 'failed', terminal: true, paused: false, background: false },
        canOpenThread: true,
        canReadOutput: false,
        outputWithheld: true,
        factAnswerAllowed: false
      }]
    }
    const evidence = workflowEvidence({ thread, summary }, workspace, '')
    expect(evidence.resultTurnId).toBe(parentTurnId)
    expect(evidence.boundedSubagentPresent).toBe(false)
    expect(packagedResultTurnContractEvidence(evidence, {}, {}, {}, {})).toEqual(expect.objectContaining({
      ok: false,
      taskAttemptCount: 1,
      taskExecutionCount: 0
    }))
    const safe = failedChildDiagnosticEvidence(evidence)
    expect(safe).toEqual(expect.objectContaining({
      observed: true, reasonCode: 'failed_child_selected', taskAttemptCount: 1,
      taskExecutionCount: 0, summaryCount: 1, statusBound: true,
      profileBound: true, foregroundBound: true, childTurnIdAvailable: false,
      parentThreadIdHash: sha256(parentThreadId),
      parentTurnIdHash: sha256(parentTurnId),
      parentToolCallIdHash: sha256(parentToolCallId),
      childRunIdHash: sha256('child-run-failed-1'),
      childThreadIdHash: sha256('thread-child-failed-1'), childTurnIdHash: ''
    }))

    const diagnosticCases: Array<[string, (next: Record<string, any>) => void]> = [
      ['failed_child_parent_thread_mismatch', (next) => { next.subagents[0].parentThreadId = 'thread-foreign-parent' }],
      ['failed_child_parent_turn_mismatch', (next) => { next.subagents[0].parentTurnId = 'turn-foreign-parent' }],
      ['failed_child_parent_call_mismatch', (next) => { next.subagents[0].parentToolCallId = 'call-foreign-parent' }],
      ['failed_child_thread_missing', (next) => { delete next.subagents[0].childThreadId }]
    ]
    for (const [reasonCode, mutate] of diagnosticCases) {
      const nextSummary = structuredClone(summary)
      mutate(nextSummary)
      expect(failedChildDiagnosticEvidence(
        workflowEvidence({ thread, summary: nextSummary }, workspace, '')
      )).toEqual(expect.objectContaining({ observed: false, reasonCode }))
    }
    const duplicateTaskThread = structuredClone(thread)
    duplicateTaskThread.turns[0].items.unshift({ kind: 'tool_call', status: 'completed',
      callId: 'call-failed-child-duplicate', toolName: 'task', toolKind: 'builtin' })
    expect(failedChildDiagnosticEvidence(
      workflowEvidence({ thread: duplicateTaskThread, summary }, workspace, '')
    )).toEqual(expect.objectContaining({ observed: false,
      reasonCode: 'failed_child_task_attempt_count_invalid' }))
    expect(failedChildDiagnosticEvidence(workflowEvidence({
      thread,
      summary: { subagents: [...summary.subagents, structuredClone(summary.subagents[0])] }
    }, workspace, ''))).toEqual(expect.objectContaining({ observed: false,
      reasonCode: 'failed_child_summary_count_invalid' }))

    const childThread = {
      id: 'thread-child-failed-1',
      providerId: 'analytix-hub',
      model: 'deepseek-v4-flash',
      latestSeq: 4,
      turns: [{ id: 'child-turn-before', status: 'completed' }, {
        id: 'child-turn-failed',
        status: 'failed',
        items: [
          { kind: 'assistant_text', status: 'failed',
            text: 'PRIVATE_CHILD_PROVIDER_OUTPUT' },
          { kind: 'error', status: 'failed', code: 'provider_unavailable',
            message: 'PRIVATE_PROVIDER_BODY' }
        ]
      }]
    }
    expect(failedChildTerminalTurnEvidence(childThread, childThread.id)).toEqual({
      observed: true, reasonCode: 'failed_child_latest_terminal_selected',
      turnIdHash: sha256('child-turn-failed')
    })
    expect(failedChildTerminalTurnEvidence(
      childThread,
      childThread.id,
      'child-turn-failed'
    )).toEqual({
      observed: true, reasonCode: 'failed_child_turn_selected',
      turnIdHash: sha256('child-turn-failed')
    })
    const childTerminal = failedChildTerminalDiagnosticEvidence(
      { thread: childThread, summary: { subagents: [] } },
      childThread.id
    )
    expect(childTerminal).toEqual(expect.objectContaining({
      observed: true,
      reasonCode: 'failed_child_latest_terminal_selected',
      turnIdHash: sha256('child-turn-failed'),
      turnStatus: 'failed',
      turnErrorCode: 'provider_unavailable',
      turnErrorCodeHash: sha256('provider_unavailable'),
      terminalReasonClass: 'none',
      terminalErrorItemCount: 1,
      terminalErrorItemAuthorityBound: true,
      providerReceiptAvailability: 'general_terminal_sse_replay'
    }))
    expect(childTerminal.inventoryDigest).toMatch(/^[0-9a-f]{64}$/)
    expect(failedChildTerminalDiagnosticEvidence(
      { thread: childThread },
      'thread-child-foreign'
    )).toEqual(expect.objectContaining({
      observed: false,
      reasonCode: 'failed_child_thread_scope_invalid'
    }))
    expect(failedChildTerminalDiagnosticEvidence(
      { thread: childThread },
      childThread.id,
      'child-turn-before'
    )).toEqual(expect.objectContaining({
      observed: false,
      reasonCode: 'failed_child_turn_not_latest'
    }))
    const completedChildTerminal = failedChildTerminalDiagnosticEvidence({
      thread: {
        id: childThread.id,
        turns: [{ id: 'child-turn-completed-without-summary', status: 'completed', items: [] }]
      }
    }, childThread.id)
    expect(completedChildTerminal).toEqual(expect.objectContaining({
      observed: true,
      turnStatus: 'completed',
      turnErrorCode: 'none',
      terminalErrorItemAuthorityBound: false
    }))
    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      workflow: {
        failedChildDiagnosticObserved: safe.observed,
        failedChildDiagnosticReasonCode: safe.reasonCode,
        failedChildDiagnosticChildThreadIdHash: safe.childThreadIdHash,
        failedChildDiagnosticChildTurnIdHash: childTerminal.turnIdHash,
        failedChildDiagnosticThreadObserved: true,
        failedChildDiagnosticTurnReasonCode: childTerminal.reasonCode,
        failedChildDiagnosticTurnStatus: childTerminal.turnStatus,
        failedChildDiagnosticTurnErrorCode: childTerminal.turnErrorCode,
        failedChildDiagnosticTurnErrorCodeHash: childTerminal.turnErrorCodeHash,
        failedChildDiagnosticTerminalReasonClass: childTerminal.terminalReasonClass,
        failedChildDiagnosticTerminalErrorItemCount: childTerminal.terminalErrorItemCount,
        failedChildDiagnosticTerminalErrorItemAuthorityBound:
          childTerminal.terminalErrorItemAuthorityBound,
        failedChildDiagnosticProviderReceiptAvailability:
          childTerminal.providerReceiptAvailability,
        failedChildDiagnosticInventoryDigest: childTerminal.inventoryDigest
      },
      checks: [{ id: 'bounded-subagent', status: 'failed', message: 'failed_child_selected' }]
    }, { requiredCheckIds: ['bounded-subagent'] })
    const serializedReport = JSON.stringify(report)
    for (const forbidden of [childThread.id, 'child-turn-failed',
      'PRIVATE_CHILD_PROVIDER_OUTPUT', 'PRIVATE_PROVIDER_BODY',
      'PRIVATE_PROMPT', '/private/secret/path', 'secret-token-value']) {
      expect(serializedReport).not.toContain(forbidden)
    }
    expect(report.workflow).toEqual(expect.objectContaining({
      failedChildDiagnosticObserved: true,
      failedChildDiagnosticTurnStatus: 'failed',
      failedChildDiagnosticTurnErrorCode: 'provider_unavailable',
      failedChildDiagnosticTerminalErrorItemAuthorityBound: true
    }))
  })

  it('preserves subagent continuity across mutable public-summary enrichment', async () => {
    const { exactlyOneSubagentContinuityMatches, subagentContinuityDigest, workflowEvidence } =
      await milestoneModule()
    const completedSubagent = {
      schemaVersion: 1,
      id: 'run:child-1',
      key: 'run:child-1',
      parentThreadId: 'thread-parent-1',
      parentTurnId: 'turn-parent-1',
      parentToolCallId: 'call-task-1',
      childId: 'child-1',
      childRunId: 'child-1',
      taskJobId: 'job-1',
      taskKind: 'task',
      childThreadId: 'thread-child-1',
      childTurnId: 'turn-child-1',
      model: 'managed-model',
      providerId: 'analytix-hub',
      endpointFormat: 'responses',
      variant: 'default',
      modelSource: 'subagent-profile',
      effort: 'high',
      profile: 'milestone-a-readonly',
      toolPolicy: 'readOnly',
      maxModelSteps: 8,
      timeBudgetMs: 180_000,
      status: 'done',
      rawStatus: 'completed',
      background: false,
      diagnostics: {
        status: 'completed',
        terminal: true,
        paused: false,
        background: false,
        updatedAt: '2026-08-02T00:00:03Z',
        finishedAt: '2026-08-02T00:00:03Z'
      },
      parallelGroupId: 'parallel-1',
      parallelIndex: 0,
      canOpenThread: true,
      canKill: false,
      canRestart: false,
      outputWithheld: true,
      outputTrustStatus: 'untrusted_child_output',
      factAnswerAllowed: false,
      evidenceAuthority: false,
      canReadOutput: false,
      canContinueParent: false,
      displayName: 'Reviewer',
      agentNickname: 'Reviewer',
      title: 'Reviewer',
      label: 'Reviewer',
      evidenceLedgered: true,
      updatedAt: '2026-08-02T00:00:03Z',
      durationMs: 100,
      queuedMs: 20,
      totalTokens: 100,
      cacheHitRate: 0.1,
      costUsd: 0.01,
      costCny: 0.07
    }
    const enrichedSubagent = {
      ...completedSubagent,
      displayName: 'Reviewer (completed)',
      agentNickname: 'Reviewer (completed)',
      title: 'Reviewer (completed)',
      label: 'Reviewer (completed)',
      updatedAt: '2026-08-02T00:00:05Z',
      diagnostics: {
        ...completedSubagent.diagnostics,
        updatedAt: '2026-08-02T00:00:05Z',
        finishedAt: '2026-08-02T00:00:05Z'
      },
      durationMs: 200,
      queuedMs: 30,
      totalTokens: 120,
      cacheHitRate: 0.2,
      costUsd: 0.02,
      costCny: 0.14
    }
    expect(sha256(canonicalJSON([completedSubagent]))).not.toBe(
      sha256(canonicalJSON([enrichedSubagent]))
    )
    expect(subagentContinuityDigest([completedSubagent])).toBe(
      subagentContinuityDigest([enrichedSubagent])
    )
    const absentOptionalBackground = Object.fromEntries(
      Object.entries(enrichedSubagent).filter(([key]) => key !== 'background')
    )
    expect(subagentContinuityDigest([completedSubagent])).toBe(
      subagentContinuityDigest([absentOptionalBackground])
    )
    expect(subagentContinuityDigest([{
      ...completedSubagent,
      background: 'invalid'
    }])).not.toBe(subagentContinuityDigest([completedSubagent]))
    const thread = {
      id: 'thread-parent-1',
      workspace: '/isolated/repository',
      turns: [{
        id: 'turn-parent-1',
        status: 'completed',
        items: [
          {
            kind: 'tool_call',
            status: 'completed',
            callId: 'call-task-1',
            toolName: 'task',
            toolKind: 'builtin'
          },
          {
            kind: 'tool_result',
            status: 'completed',
            callId: 'call-task-1',
            toolName: 'task',
            toolKind: 'builtin',
            isError: false,
            output: { status: 'completed' }
          },
          {
            kind: 'assistant_text',
            status: 'completed',
            text: 'MILESTONE_A_RESULT_OK'
          }
        ]
      }]
    }
    const firstEvidence = workflowEvidence({
      thread,
      summary: { subagents: [completedSubagent] }
    }, '/isolated/repository')
    const laterThread = structuredClone(thread)
    laterThread.turns.push({
      id: 'turn-continuation-1',
      status: 'completed',
      items: [{
        kind: 'assistant_text',
        status: 'completed',
        text: 'MILESTONE_A_CONTEXT_OK_TEST'
      }]
    })
    const laterEvidence = workflowEvidence({
      thread: laterThread,
      summary: { subagents: [enrichedSubagent] }
    }, '/isolated/repository', 'MILESTONE_A_CONTEXT_OK_TEST')
    expect(firstEvidence.resultTurnId).toBe('turn-parent-1')
    expect(laterEvidence.resultTurnId).toBe('turn-continuation-1')
    expect(laterEvidence.boundedSubagentPresent).toBe(false)
    expect(exactlyOneSubagentContinuityMatches(firstEvidence, laterEvidence)).toBe(true)
    expect(exactlyOneSubagentContinuityMatches(firstEvidence, {
      ...laterEvidence,
      subagentContinuityDigest: subagentContinuityDigest([{
        ...enrichedSubagent,
        childThreadId: 'thread-child-2'
      }])
    })).toBe(false)
    for (const [field, value] of [
      ['schemaVersion', 2],
      ['id', 'run:child-2'],
      ['key', 'run:child-2'],
      ['parentThreadId', 'thread-parent-2'],
      ['parentTurnId', 'turn-parent-2'],
      ['parentToolCallId', 'call-task-2'],
      ['childId', 'child-2'],
      ['childRunId', 'child-2'],
      ['taskJobId', 'job-2'],
      ['taskKind', 'subagent'],
      ['childThreadId', 'thread-child-2'],
      ['childTurnId', 'turn-child-2'],
      ['model', 'other-model'],
      ['providerId', 'other-provider'],
      ['endpointFormat', 'chat_completions'],
      ['variant', 'other-variant'],
      ['modelSource', 'explicit-input'],
      ['effort', 'max'],
      ['profile', 'other-profile'],
      ['toolPolicy', 'workspaceWrite'],
      ['maxModelSteps', 7],
      ['timeBudgetMs', 120_000],
      ['status', 'terminal'],
      ['rawStatus', 'failed'],
      ['background', true],
      ['parallelGroupId', 'parallel-2'],
      ['parallelIndex', 1],
      ['canOpenThread', false],
      ['canKill', true],
      ['canRestart', true],
      ['outputWithheld', false],
      ['outputTrustStatus', 'private_tool_output'],
      ['factAnswerAllowed', true],
      ['evidenceAuthority', true],
      ['canReadOutput', true],
      ['canContinueParent', true]
    ] as const) {
      expect(subagentContinuityDigest([{
        ...completedSubagent,
        [field]: value
      }])).not.toBe(subagentContinuityDigest([completedSubagent]))
    }
    expect(subagentContinuityDigest([{
      ...completedSubagent,
      diagnostics: { ...completedSubagent.diagnostics, terminal: false }
    }])).not.toBe(subagentContinuityDigest([completedSubagent]))
    for (const field of [
      'canOpenThread',
      'canKill',
      'canRestart',
      'outputWithheld',
      'factAnswerAllowed',
      'evidenceAuthority',
      'canReadOutput',
      'canContinueParent'
    ] as const) {
      const missingRequiredBoolean = Object.fromEntries(
        Object.entries(completedSubagent).filter(([key]) => key !== field)
      )
      expect(subagentContinuityDigest([missingRequiredBoolean])).not.toBe(
        subagentContinuityDigest([completedSubagent])
      )
    }
    for (const field of ['terminal', 'paused', 'background'] as const) {
      const missingRequiredDiagnostic = {
        ...completedSubagent,
        diagnostics: Object.fromEntries(
          Object.entries(completedSubagent.diagnostics).filter(([key]) => key !== field)
        )
      }
      expect(subagentContinuityDigest([missingRequiredDiagnostic])).not.toBe(
        subagentContinuityDigest([completedSubagent])
      )
    }
  })

  it('requires a typed per-effect funds denial with zero claims, receipts, and successful execution', async () => {
    const {
      protectedFundsUnavailableTurnEvidence,
      protectedFundsWaitDisposition,
      workflowEvidence
    } = await milestoneModule()
    const observation = protectedFundsUnavailableObservation()
    const accepted = protectedFundsUnavailableTurnEvidence(
      observation,
      'thread_general_additive_1',
      'turn_protected_funds_denied_1'
    )
    expect(accepted).toEqual(expect.objectContaining({
      ok: true,
      claimCount: 0,
      receiptCount: 0,
      successfulFundsExecutionCount: 0,
      acceptedFinalDigest: 'd'.repeat(64)
    }))

    const mixedObservation = structuredClone(observation)
    mixedObservation.thread.workspace = '/isolated/repository'
    mixedObservation.thread.turns.unshift({
      id: 'turn_ordinary_result_1',
      threadId: 'thread_general_additive_1',
      status: 'completed',
      items: [{
        id: 'item_ordinary_result_1',
        turnId: 'turn_ordinary_result_1',
        threadId: 'thread_general_additive_1',
        role: 'assistant',
        status: 'completed',
        kind: 'assistant_text',
        text: 'MILESTONE_A_RESULT_OK'
      }]
    })
    const mixedWorkflow = workflowEvidence(
      mixedObservation,
      '/isolated/repository'
    )
    expect(mixedWorkflow.resultTurnId).toBe('turn_ordinary_result_1')
    expect(mixedWorkflow.thread?.turns?.at(-1)?.id).toBe(
      'turn_protected_funds_denied_1'
    )
    expect(protectedFundsWaitDisposition(mixedObservation, {
      previousTurnCount: 1,
      expectedThreadId: 'thread_general_additive_1'
    })).toBe('completed')
    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const baselineGate = milestone.slice(
      milestone.indexOf('const protectedFundsBaselineEvidence ='),
      milestone.indexOf('if (!protectedFundsBaselineBound)')
    )
    expect(baselineGate).toContain(
      'protectedFundsBaselineEvidence.thread?.turns?.at(-1)?.id'
    )
    expect(baselineGate).toContain(
      'protectedFundsBaselineEvidence.resultTurnId === firstEvidence.resultTurnId'
    )
    expect(baselineGate).not.toContain(
      'protectedFundsBaselineEvidence.resultTurnId'
        + ' === protectedFundsUnavailable.turnId'
    )
    expect(protectedFundsWaitDisposition(observation, {
      previousTurnCount: 0,
      expectedThreadId: 'thread_general_additive_1'
    })).toBe('completed')

    const projectionPending = structuredClone(observation)
    delete projectionPending.thread.turns[0].acceptedFinalView
    projectionPending.thread.turns[0].items = []
    delete projectionPending.thread.acceptedFinalDelivery
    delete projectionPending.thread.acceptedFinalDeliveries
    expect(protectedFundsWaitDisposition(projectionPending, {
      previousTurnCount: 0,
      expectedThreadId: 'thread_general_additive_1'
    })).toBe('pending')

    const wrongTypedBoundary = structuredClone(observation)
    wrongTypedBoundary.thread.turns[0].acceptedFinalView.variant = 'GeneralGuidanceAnswer'
    expect(protectedFundsWaitDisposition(wrongTypedBoundary, {
      previousTurnCount: 0,
      expectedThreadId: 'thread_general_additive_1'
    })).toBe('terminal_without_typed_boundary')

    const failedWithoutProjection = structuredClone(projectionPending)
    failedWithoutProjection.thread.turns[0].status = 'failed'
    expect(protectedFundsWaitDisposition(failedWithoutProjection, {
      previousTurnCount: 0,
      expectedThreadId: 'thread_general_additive_1'
    })).toBe('terminal_without_typed_boundary')

    const invalidMutations: Array<(value: Record<string, any>) => void> = [
      (value) => { value.thread.turns[0].status = 'failed' },
      (value) => { value.thread.turns[0].acceptedFinalView.variant = 'GeneralGuidanceAnswer' },
      (value) => { value.thread.turns[0].acceptedFinalView.claimCount = 1 },
      (value) => { value.thread.turns[0].acceptedFinalView.receiptMetadata.count = 1 },
      (value) => {
        value.thread.turns[0].items = [{
          kind: 'tool_call',
          status: 'completed',
          callId: 'call_funds_1',
          toolName: 'mcp__analytix_funds__analyze_account_flows',
          toolKind: 'mcp'
        }, {
          kind: 'tool_result',
          status: 'completed',
          callId: 'call_funds_1',
          toolName: 'mcp__analytix_funds__analyze_account_flows',
          toolKind: 'mcp',
          isError: false,
          output: { status: 'completed' }
        }]
      }
    ]
    for (const mutate of invalidMutations) {
      const invalid = structuredClone(observation)
      mutate(invalid)
      expect(protectedFundsUnavailableTurnEvidence(
        invalid,
        'thread_general_additive_1',
        'turn_protected_funds_denied_1'
      ).ok).toBe(false)
    }
  })

  it('binds one complete long-context read to the exact normalized line limit', async () => {
    const {
      externalRepositoryLongContextBindingEvidence,
      fullTextReadLineLimit
    } = await milestoneModule()
    expect(fullTextReadLineLimit(Buffer.from('first\nlast\n'))).toBe(3)
    expect(fullTextReadLineLimit(Buffer.from('first\r\nlast\r\n'))).toBe(3)
    expect(fullTextReadLineLimit(Buffer.from('first\rlast\r'))).toBe(3)
    expect(fullTextReadLineLimit(Buffer.from('only'))).toBe(1)
    expect(fullTextReadLineLimit(Buffer.from('line\n'.repeat(3911)))).toBe(3912)
    expect(fullTextReadLineLimit(Buffer.alloc(0))).toBe(0)
    expect(fullTextReadLineLimit(null)).toBe(0)

    const markers = {
      firstMarker: 'FIRST_ANCHOR_UNIQUE',
      completionMarker: 'COMPLETION_ANCHOR_UNIQUE',
      lastMarker: 'LAST_ANCHOR_UNIQUE'
    }
    const content = Buffer.from(
      `${markers.firstMarker}\n${'bounded filler row\n'.repeat(4096)}` +
      `${markers.completionMarker}\n${markers.lastMarker}\n`
    )
    const expected = {
      ...markers,
      minimumBytes: 64 * 1024,
      sha256: sha256(content)
    }
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: content.byteLength,
      sha256: sha256(content),
      content
    }, expected)).toEqual(expect.objectContaining({
      ok: true,
      lineLimit: 4100,
      nonEmptyLineCount: 4099,
      firstLineNumber: 1,
      completionLineNumber: 4098,
      lastLineNumber: 4099
    }))
    const duplicate = Buffer.from(content.toString('utf8').replace(
      markers.firstMarker,
      `${markers.firstMarker} ${markers.firstMarker}`
    ))
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: duplicate.byteLength,
      sha256: sha256(duplicate),
      content: duplicate
    }, { ...expected, sha256: sha256(duplicate) }).ok).toBe(false)

    const trailingBoundary = Buffer.from(
      `${markers.firstMarker}\n${'bounded filler row\n'.repeat(4096)}` +
      `${markers.completionMarker}\n${markers.lastMarker}\n*/\n`
    )
    const trailingExpected = { ...markers, minimumBytes: 64 * 1024, sha256: sha256(trailingBoundary) }
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: trailingBoundary.byteLength,
      sha256: trailingExpected.sha256,
      content: trailingBoundary
    }, trailingExpected)).toEqual(expect.objectContaining({
      ok: true,
      lineLimit: 4101,
      nonEmptyLineCount: 4100,
      firstLineNumber: 1,
      completionLineNumber: 4098,
      lastLineNumber: 4099
    }))

    const beyondTrailingBoundary = Buffer.from(
      trailingBoundary.toString('utf8').replace('*/\n', 'closing one\nclosing two\n*/\n')
    )
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: beyondTrailingBoundary.byteLength,
      sha256: sha256(beyondTrailingBoundary),
      content: beyondTrailingBoundary
    }, { ...trailingExpected, sha256: sha256(beyondTrailingBoundary) })).toEqual(
      expect.objectContaining({
        ok: false,
        lineLimit: 4103,
        firstLineNumber: 1,
        completionLineNumber: 4098,
        lastLineNumber: 4099
      })
    )

    const outOfOrder = Buffer.from(
      `${markers.firstMarker}\n${'bounded filler row\n'.repeat(4096)}` +
      `${markers.lastMarker}\n${markers.completionMarker}`
    )
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: outOfOrder.byteLength,
      sha256: sha256(outOfOrder),
      content: outOfOrder
    }, { ...markers, minimumBytes: 64 * 1024, sha256: sha256(outOfOrder) }).ok).toBe(false)

    const firstAnchorDisplaced = Buffer.from(`prefix\n${content.toString('utf8')}`)
    expect(externalRepositoryLongContextBindingEvidence({
      regular: true,
      byteLength: firstAnchorDisplaced.byteLength,
      sha256: sha256(firstAnchorDisplaced),
      content: firstAnchorDisplaced
    }, { ...expected, sha256: sha256(firstAnchorDisplaced) }).ok).toBe(false)

    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const contextRead = milestone.slice(
      milestone.indexOf(
        'const contextMarkers = privateRepositoryContextMarkers(repositoryAuthority)'
      ),
      milestone.indexOf("const compactSubmit =")
    )
    expect(contextRead).toContain('capture: true')
    expect(contextRead).toContain('continuationReadBindingEvidence(')
    expect(contextRead).not.toContain('externalRepositoryLongContextBindingEvidence(')
    expect(contextRead).toContain(
      'limit: repositoryAuthority.context.lineLimit || contextBinding.lineLimit'
    )
    expect(contextRead).toContain('const contextPrompt = boundedContinuationPrompt({')
    expect(contextRead).toContain('arguments: contextReadArguments')
    expect(contextRead).toContain('contextReadArguments.limit > 0')
    expect(contextRead).not.toContain('contextBinding.completionLineNumber')
    expect(contextRead).not.toContain('contextBinding.firstLineNumber')
    expect(contextRead).not.toContain('contextBinding.lastLineNumber')
    expect(contextRead).not.toContain('contextFile.byteLength >= 64 * 1024')
    expect(contextRead).toContain('contextCandidateTurnId')
    expect(contextRead).toContain('contextDiagnostic.toolInventoryAvailability')
    expect(contextRead).toContain('contextHostObservation.transportStatus')
    expect(contextRead).not.toContain('longContextToolSet.ok')
    expect(contextRead).toContain('if (!contextBinding.ok ||')
    expect(contextRead).not.toContain('omitting offset and limit')
  })

  it('accepts only completed production-schema compaction items and exact recovery', async () => {
    const { compactionItems, exactCompactionRecoveryMatches } = await milestoneModule()
    const thread = validCompactionThread()
    const items = compactionItems(thread)
    expect(items).toHaveLength(1)
    expect(items[0]).toEqual(expect.objectContaining({
      status: 'completed',
      replacedTokens: 2048,
      sourceDigest: 'b'.repeat(64),
      sourceItemIds: ['item_user_1', 'item_result_1'],
      schemaVersion: 3,
      providerHistoryProjectionVersion: 1
    }))

    const invalidMutations: Array<(item: Record<string, any>) => void> = [
      (item) => { item.status = 'in_progress' },
      (item) => { item.replacedTokens = 0 },
      (item) => { item.sourceDigest = '' },
      (item) => { item.digestMarker = 'sha256:invalid' },
      (item) => { item.sourceItemIds = [] },
      (item) => { item.sourceItemIds = ['item_user_1', 'item_user_1'] }
    ]
    for (const mutate of invalidMutations) {
      const invalid = structuredClone(thread)
      mutate(invalid.turns[0].items[0])
      expect(compactionItems(invalid)).toEqual([])
    }

    const compactedEvidence = {
      compactions: items,
      compactionCount: items.length,
      compactionsDigest: sha256(canonicalJSON(items))
    }
    const recoveredItems = compactionItems(structuredClone(thread))
    const recoveredEvidence = {
      compactions: recoveredItems,
      compactionCount: recoveredItems.length,
      compactionsDigest: sha256(canonicalJSON(recoveredItems))
    }
    expect(exactCompactionRecoveryMatches(compactedEvidence, recoveredEvidence)).toBe(true)

    const changedDigestItems = structuredClone(recoveredItems)
    changedDigestItems[0].sourceDigest = 'c'.repeat(64)
    changedDigestItems[0].digestMarker = `sha256:${'c'.repeat(12)}`
    expect(exactCompactionRecoveryMatches(compactedEvidence, {
      compactions: changedDigestItems,
      compactionCount: changedDigestItems.length,
      compactionsDigest: sha256(canonicalJSON(changedDigestItems))
    })).toBe(false)

    const caseThread = validCaseCompactionThread()
    expect(compactionItems(caseThread)).toHaveLength(1)
    for (const mutate of [
      (item: Record<string, any>) => { item.taskContinuation = { schemaVersion: 1 } },
      (item: Record<string, any>) => { item.sourceContextDigest = 'd'.repeat(64) },
      (item: Record<string, any>) => { item.caseCompactionBinding = { schemaVersion: 1 } },
      (item: Record<string, any>) => { item.privateReasoning = 'must-not-pass' },
      (item: Record<string, any>) => { item.replacedTokens = 2048 },
      (item: Record<string, any>) => { item.finishedAt = '2026-07-29T01:02:04.000Z' },
      (item: Record<string, any>) => { item.pinnedConstraints = [] },
      (item: Record<string, any>) => { item.sourceItemIds = ['private-item'] },
      (item: Record<string, any>) => { item.summary = 'case history compacted' }
    ]) {
      const invalid = structuredClone(caseThread)
      mutate(invalid.turns[0].items[0])
      expect(compactionItems(invalid)).toEqual([])
    }
  })

  it('requires manual compaction evidence to be auto=false with nonzero replacement and source ancestry', async () => {
    const { compactionItems } = await milestoneModule()
    const valid = validCompactionThread()
    expect(compactionItems(valid)).toHaveLength(1)
    const automatic = structuredClone(valid)
    automatic.turns[0].items[0].auto = true
    expect(compactionItems(automatic)).toEqual([])
    const zeroReplacement = structuredClone(valid)
    zeroReplacement.turns[0].items[0].replacedTokens = 0
    expect(compactionItems(zeroReplacement)).toEqual([])
    const noAncestry = structuredClone(valid)
    noAncestry.turns[0].items[0].sourceItemIds = []
    expect(compactionItems(noAncestry)).toEqual([])
  })

  it('waits past the exact compaction baseline and accepts ordinary or case-bound typed proof only', async () => {
    const {
      compactionBaselineEvidence,
      compactionWaitDisposition,
      manualCompactionProofEvidence,
      nonzeroCompactionFailureReasonCode
    } = await milestoneModule()
    const old = validCompactionThread().turns[0].items[0]
    const baseline = compactionBaselineEvidence([old])
    expect(baseline).toEqual(expect.objectContaining({
      count: 1,
      digest: expect.stringMatching(/^[0-9a-f]{64}$/),
      identityDigests: [expect.stringMatching(/^[0-9a-f]{64}$/)]
    }))

    const replay = {
      compactions: [old],
      compactionCount: 1,
      compactionsDigest: sha256(canonicalJSON([old]))
    }
    expect(compactionWaitDisposition(replay, { baseline })).toBe('pending')
    expect(nonzeroCompactionFailureReasonCode()).toBe(
      'nonzero_compaction_manual_marker_not_observed'
    )

    const ordinaryThread = validCompactionThread()
    ordinaryThread.turns[0].items[0].id = 'compaction_thread_1_2'
    const ordinary = ordinaryThread.turns[0].items[0]
    refreshCompactionProof(ordinary)
    const ordinaryEvidence = {
      compactions: [old, ordinary],
      compactionCount: 2,
      compactionsDigest: sha256(canonicalJSON([old, ordinary]))
    }
    expect(compactionWaitDisposition(ordinaryEvidence, { baseline })).toBe('completed')
    expect(manualCompactionProofEvidence(ordinaryEvidence, baseline)).toEqual(
      expect.objectContaining({
        ok: true,
        projectionClass: 'ordinary_exact',
        manualCompactionNonzeroBound: true,
        manualCompactionSourceAncestryBound: true,
        replacedTokens: 2048,
        sourceItemIdsDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
        newAfterBaseline: true
      })
    )

    const caseThread = validCaseCompactionThread()
    caseThread.turns[0].items[0].id = 'compaction_case_1_2'
    const typedCase = caseThread.turns[0].items[0]
    refreshCompactionProof(typedCase)
    const caseEvidence = {
      compactions: [old, typedCase],
      compactionCount: 2,
      compactionsDigest: sha256(canonicalJSON([old, typedCase]))
    }
    expect(compactionWaitDisposition(caseEvidence, { baseline })).toBe('completed')
    expect(manualCompactionProofEvidence(caseEvidence, baseline)).toEqual(
      expect.objectContaining({
        ok: true,
        projectionClass: 'case_bound_typed',
        manualCompactionNonzeroBound: true,
        manualCompactionSourceAncestryBound: true,
        replacedTokens: 0,
        sourceItemIdsDigest: '',
        newAfterBaseline: true
      })
    )

    const automatic = structuredClone(caseEvidence)
    automatic.compactions[1].auto = true
    automatic.compactionsDigest = sha256(canonicalJSON(automatic.compactions))
    expect(manualCompactionProofEvidence(automatic, baseline)).toEqual(
      expect.objectContaining({ ok: false, reasonCode: 'manual_compaction_auto_true' })
    )
    for (const mutate of [
      (item: Record<string, any>) => { delete item.schemaVersion },
      (item: Record<string, any>) => { delete item.reasoningExclusionProof },
      (item: Record<string, any>) => { item.sourceDigest = '' }
    ]) {
      const invalid = structuredClone(caseEvidence)
      mutate(invalid.compactions[1])
      expect(manualCompactionProofEvidence(invalid, baseline)).toEqual(
        expect.objectContaining({ ok: false })
      )
    }
  })

  it('keeps the direct nonzero-compaction failure reason and rejects failed completion snapshots', async () => {
    const {
      finalizeMilestoneAReport,
      nonzeroCompactionFailureReasonCode,
      sanitizeMilestoneAStageSnapshot
    } = await milestoneModule()
    const reasonCode = nonzeroCompactionFailureReasonCode()
    const snapshot = sanitizeMilestoneAStageSnapshot({
      stage: 'composer-compaction-submit',
      checkIds: ['composer-compaction-submit', 'nonzero-compaction'],
      completedFields: {
        manualCompactionBaselineBound: true,
        manualCompactionBaselineCount: 0,
        manualCompactionBaselineDigest: '1'.repeat(64),
        manualCompactionCandidateDigest: '2'.repeat(64),
        manualCompactionNewAfterBaseline: true,
        compactionCount: 1,
        manualCompactionObserved: false,
        manualCompactionAuto: null,
        manualCompactionReplacedTokens: 0,
        manualCompactionSourceDigest: ''
      }
    })
    const report = finalizeMilestoneAReport({
      status: 'failed',
      passed: false,
      executionBlocker: 'hub_blocker_must_not_replace_direct_reason',
      workflow: {
        compactionCount: 1,
        manualCompactionObserved: false,
        manualCompactionAuto: null,
        manualCompactionReplacedTokens: 0,
        manualCompactionSourceDigest: '',
        manualCompactionSourceItemIdsDigest: '',
        manualCompactionSourceAncestryBound: false,
        manualCompactionNonzeroBound: false,
        manualCompactionProjectionClass: 'not_observed'
      },
      checks: [
        { id: 'composer-compaction-submit', status: 'passed' },
        { id: 'nonzero-compaction', status: 'failed', message: reasonCode }
      ],
      stageSnapshots: snapshot ? [snapshot] : []
    }, {
      requiredCheckIds: ['composer-compaction-submit', 'nonzero-compaction']
    })
    expect(report.executionBlocker).toBe(reasonCode)
    expect(report.checks).toEqual(expect.arrayContaining([
      { id: 'nonzero-compaction', status: 'failed', message: reasonCode }
    ]))
    expect(report.stageSnapshots).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ stage: 'composer-compaction-submit' })
    ]))

    const incompleteTypedProof = finalizeMilestoneAReport({
      workflow: {
        compactionCount: 1,
        manualCompactionObserved: true,
        manualCompactionAuto: false,
        manualCompactionReplacedTokens: 0,
        manualCompactionSourceDigest: 'a'.repeat(64),
        manualCompactionSourceItemIdsDigest: '',
        manualCompactionSourceAncestryBound: true,
        manualCompactionNonzeroBound: true,
        manualCompactionProjectionClass: 'case_bound_typed'
      },
      checks: [
        { id: 'composer-compaction-submit', status: 'passed' },
        { id: 'nonzero-compaction', status: 'passed' }
      ],
      stageSnapshots: [{
        stage: 'composer-compaction-submit',
        checkIds: ['composer-compaction-submit', 'nonzero-compaction'],
        completedFields: {
          compactionCount: 1,
          manualCompactionObserved: true,
          manualCompactionAuto: false,
          manualCompactionSourceDigest: 'a'.repeat(64),
          manualCompactionSourceAncestryBound: true,
          manualCompactionNonzeroBound: true,
          manualCompactionProjectionClass: 'case_bound_typed'
        }
      }]
    }, {
      requiredCheckIds: ['composer-compaction-submit', 'nonzero-compaction']
    })
    expect(incompleteTypedProof.stageSnapshots).not.toEqual(expect.arrayContaining([
      expect.objectContaining({ stage: 'composer-compaction-submit' })
    ]))
  })

  it('uses the isolated default Alt+F4 input and closes normal-quit diagnostics to safe reason codes', async () => {
    const defaultBindings = resolveKeyboardShortcutBindings()
    expect(findKeyboardShortcutCommand(
      defaultBindings,
      keyboardEventToShortcut({ key: 'F4', altKey: true })
    )).toBe('quit')
    expect(findKeyboardShortcutCommand(
      defaultBindings,
      keyboardEventToShortcut({ key: 'q', metaKey: true })
    )).toBeNull()

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const quitStart = script.indexOf('async function cdpNormalQuit')
    const quitEnd = script.indexOf('\n}\n\nfunction emptyProviderReceiptTrace', quitStart)
    const quit = script.slice(quitStart, quitEnd)
    expect(quit).toContain("key: 'F4'")
    expect(quit).toContain("code: 'F4'")
    expect(quit).toContain('modifiers: 1')
    expect(quit).not.toContain("key: 'q'")
    expect(quit).not.toContain("code: 'KeyQ'")
    expect(quit).not.toContain('modifiers: 4')

    const { normalQuitLifecycleEvidence } = await milestoneModule()
    const vectors = [
      ['normal_quit_request_failed', { requestOk: false }],
      ['normal_quit_process_not_exited', { requestOk: true, processExited: false }],
      ['normal_quit_exit_status_missing', { requestOk: true, processExited: true }],
      ['normal_quit_nonzero_exit', { requestOk: true, processExited: true, exitCode: 1 }],
      ['normal_quit_signal_exit', {
        requestOk: true, processExited: true, exitCode: null, signal: 'SIGTERM'
      }],
      ['normal_quit_residual_owned_process', {
        requestOk: true, processExited: true, exitCode: 0, residualOwnedProcess: true
      }],
      ['normal_quit_debug_port_open', {
        requestOk: true, processExited: true, exitCode: 0, debugPortOpen: true
      }],
      ['normal_quit_runtime_port_open', {
        requestOk: true, processExited: true, exitCode: 0, runtimePortOpen: true
      }],
      ['normal_quit_artifact_revalidation_failed', {
        requestOk: true, processExited: true, exitCode: 0, artifactRevalidationOk: false
      }]
    ] as const
    for (const [reasonCode, input] of vectors) {
      expect(normalQuitLifecycleEvidence(input)).toEqual(expect.objectContaining({
        ok: false,
        reasonCode,
        blocker: reasonCode
      }))
    }
    const diagnostic = normalQuitLifecycleEvidence({
      requestOk: true,
      processExited: true,
      exitCode: 0,
      signal: null,
      residualOwnedProcess: false,
      debugPortOpen: false,
      runtimePortOpen: false,
      artifactRevalidationOk: true
    })
    expect(diagnostic).toEqual(expect.objectContaining({
      ok: true,
      reasonCode: 'normal_quit_observed',
      blocker: ''
    }))
    expect(JSON.stringify(diagnostic)).not.toMatch(/(?:\/|command|argv|cmd|path|provider|prompt|body)/iu)

    const compactionStart = script.indexOf('const manualCompaction =')
    const baselineStart = script.indexOf('const compactionBaseline =')
    const firstQuitStart = script.indexOf('const firstProcessIds =', baselineStart)
    const compactionAndFirstQuit = script.slice(baselineStart, firstQuitStart)
    const firstQuitLifecycle = script.slice(
      firstQuitStart,
      script.indexOf('secondDebugPort = await getFreePort()', firstQuitStart)
    )
    expect(compactionStart).toBe(-1)
    expect(firstQuitLifecycle).toContain(
      'const firstTaskOwnedResidualCount = taskOwnedProcessPids(sandboxRoot).length'
    )
    expect(firstQuitLifecycle).toContain(
      'residualOwnedProcess: !firstNoResidual || firstTaskOwnedResidualCount > 0'
    )
    expect(compactionAndFirstQuit).toContain('compactionBaselineEvidence(firstEvidence.compactions)')
    expect(script).toContain('const disposition = compactionWaitDisposition')
    expect(compactionAndFirstQuit).toContain('NONZERO_COMPACTION_FAILURE_REASON_CODE')
    expect(compactionAndFirstQuit).toContain('throw new Error(NONZERO_COMPACTION_FAILURE_REASON_CODE)')
    const finalWorkflowStart = script.lastIndexOf('report.workflow = {')
    const finalWorkflowEnd = script.indexOf('\n    report.redaction = {', finalWorkflowStart)
    const finalWorkflow = script.slice(finalWorkflowStart, finalWorkflowEnd)
    for (const field of [
      'manualCompactionBaselineBound',
      'manualCompactionBaselineCount',
      'manualCompactionBaselineDigest',
      'manualCompactionCandidateDigest',
      'manualCompactionNewAfterBaseline',
      'manualCompactionNonzeroBound',
      'manualCompactionProjectionClass'
    ]) {
      expect(finalWorkflow).toMatch(new RegExp(
        `${field}:\\s*report\\.workflow\\.${field}`
      ))
    }
    const firstQuitEnd = script.indexOf('secondDebugPort = await getFreePort()', firstQuitStart)
    expect(script.slice(firstQuitStart, firstQuitEnd)).toContain(
      'normalFirstQuitReasonCode: firstNormalQuitReasonCode'
    )
  })

  it('recovers passed manual compaction evidence from an immutable stage snapshot after quit failure', async () => {
    const {
      finalizeMilestoneAReport,
      sanitizeMilestoneAStageSnapshot
    } = await milestoneModule()
    const compactionSnapshot = sanitizeMilestoneAStageSnapshot({
      stage: 'composer-compaction-submit',
      checkIds: ['composer-compaction-submit', 'nonzero-compaction'],
      completedFields: {
        manualCompactionBaselineBound: true,
        manualCompactionBaselineCount: 0,
        manualCompactionBaselineDigest: '1'.repeat(64),
        manualCompactionCandidateDigest: '2'.repeat(64),
        manualCompactionNewAfterBaseline: true,
        compactionCount: 1,
        manualCompactionObserved: true,
        manualCompactionAuto: false,
        manualCompactionReplacedTokens: 2048,
        manualCompactionSourceDigest: 'b'.repeat(64),
        manualCompactionSourceItemIdsDigest: 'c'.repeat(64),
        manualCompactionSourceAncestryBound: true,
        manualCompactionNonzeroBound: true,
        manualCompactionProjectionClass: 'ordinary_exact',
        unsafePrompt: 'DO_NOT_PERSIST'
      }
    })
    expect(compactionSnapshot).not.toBeNull()
    expect(Object.isFrozen(compactionSnapshot)).toBe(true)
    expect(Object.isFrozen(compactionSnapshot?.checkIds)).toBe(true)
    expect(Object.isFrozen(compactionSnapshot?.completedFields)).toBe(true)
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      status: 'failed',
      passed: false,
      executionBlocker: 'normal_quit_process_not_exited',
      workflow: {
        compactionCount: 0,
        manualCompactionBaselineBound: false,
        manualCompactionBaselineCount: 0,
        manualCompactionBaselineDigest: '',
        manualCompactionCandidateDigest: '',
        manualCompactionNewAfterBaseline: false,
        manualCompactionObserved: false,
        manualCompactionAuto: null,
        manualCompactionReplacedTokens: 0,
        manualCompactionSourceDigest: '',
        manualCompactionSourceItemIdsDigest: '',
        manualCompactionSourceAncestryBound: false,
        manualCompactionNonzeroBound: false,
        manualCompactionProjectionClass: 'not_observed'
      },
      checks: [
        { id: 'composer-compaction-submit', status: 'passed' },
        { id: 'nonzero-compaction', status: 'passed' },
        { id: 'normal-first-quit', status: 'failed', message: 'normal_quit_process_not_exited' },
        { id: 'fresh-packaged-relaunch', status: 'skipped' },
        { id: 'normal-final-quit', status: 'skipped' }
      ],
      stageSnapshots: [compactionSnapshot]
    }, {
      requiredCheckIds: [
        'composer-compaction-submit',
        'nonzero-compaction',
        'normal-first-quit',
        'fresh-packaged-relaunch',
        'normal-final-quit'
      ]
    })
    expect(report.workflow).toEqual(expect.objectContaining({
      compactionCount: 1,
      manualCompactionObserved: true,
      manualCompactionAuto: false,
      manualCompactionReplacedTokens: 2048,
      manualCompactionBaselineBound: true,
      manualCompactionBaselineCount: 0,
      manualCompactionBaselineDigest: '1'.repeat(64),
      manualCompactionCandidateDigest: '2'.repeat(64),
      manualCompactionNewAfterBaseline: true,
      manualCompactionSourceDigest: 'b'.repeat(64),
      manualCompactionSourceItemIdsDigest: 'c'.repeat(64),
      manualCompactionSourceAncestryBound: true,
      manualCompactionNonzeroBound: true,
      manualCompactionProjectionClass: 'ordinary_exact'
    }))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'composer-compaction-submit',
        completedFields: expect.objectContaining({
          compactionCount: 1,
          manualCompactionBaselineBound: true,
          manualCompactionBaselineCount: 0,
          manualCompactionBaselineDigest: '1'.repeat(64),
          manualCompactionCandidateDigest: '2'.repeat(64),
          manualCompactionNewAfterBaseline: true,
          manualCompactionObserved: true,
          manualCompactionAuto: false,
          manualCompactionReplacedTokens: 2048,
          manualCompactionSourceDigest: 'b'.repeat(64),
          manualCompactionSourceItemIdsDigest: 'c'.repeat(64),
          manualCompactionSourceAncestryBound: true,
          manualCompactionNonzeroBound: true,
          manualCompactionProjectionClass: 'ordinary_exact'
        })
      })
    ]))
    expect(report.failedCheckIds).toEqual(['normal-first-quit'])
    expect(report.skippedCheckIds).toEqual(['fresh-packaged-relaunch', 'normal-final-quit'])
    expect(report.liveBlockedCheckIds).toEqual([])
    expect(report.failureCheckIds).toEqual([
      'normal-first-quit',
      'fresh-packaged-relaunch',
      'normal-final-quit'
    ])
    expect(report.failureCheckIds).toEqual(
      report.checks.filter((item: any) => item.status !== 'passed').map((item: any) => item.id)
    )
    expect(JSON.stringify(report)).not.toContain('DO_NOT_PERSIST')

    const existingEvidence = finalizeMilestoneAReport({
      workflow: {
        compactionCount: 3,
        manualCompactionBaselineBound: true,
        manualCompactionBaselineCount: 0,
        manualCompactionBaselineDigest: '1'.repeat(64),
        manualCompactionCandidateDigest: '3'.repeat(64),
        manualCompactionNewAfterBaseline: true,
        manualCompactionObserved: true,
        manualCompactionAuto: false,
        manualCompactionReplacedTokens: 4096,
        manualCompactionSourceDigest: 'd'.repeat(64),
        manualCompactionSourceItemIdsDigest: 'e'.repeat(64),
        manualCompactionSourceAncestryBound: true,
        manualCompactionNonzeroBound: true,
        manualCompactionProjectionClass: 'ordinary_exact'
      },
      checks: [
        { id: 'composer-compaction-submit', status: 'passed' },
        { id: 'nonzero-compaction', status: 'passed' }
      ],
      stageSnapshots: [compactionSnapshot]
    }, {
      requiredCheckIds: ['composer-compaction-submit', 'nonzero-compaction']
    })
    expect(existingEvidence.workflow).toEqual(expect.objectContaining({
      compactionCount: 3,
      manualCompactionReplacedTokens: 4096,
      manualCompactionSourceDigest: 'd'.repeat(64),
      manualCompactionSourceItemIdsDigest: 'e'.repeat(64)
    }))

    const conflictingAutoSnapshot = sanitizeMilestoneAStageSnapshot({
      stage: 'composer-compaction-submit',
      checkIds: ['composer-compaction-submit', 'nonzero-compaction'],
      completedFields: { manualCompactionAuto: true }
    })
    const autoConflict = finalizeMilestoneAReport({
      workflow: { manualCompactionAuto: false },
      checks: [
        { id: 'composer-compaction-submit', status: 'passed' },
        { id: 'nonzero-compaction', status: 'passed' }
      ],
      stageSnapshots: [conflictingAutoSnapshot]
    }, {
      requiredCheckIds: ['composer-compaction-submit', 'nonzero-compaction']
    })
    expect(autoConflict.workflow.manualCompactionAuto).toBe(false)
  })

  it('collects immutable ordinary-stage evidence before rejecting a failed terminal turn', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const providerEvidenceStart = script.indexOf(
      'providerEvidence = providerTurnEvidence(firstEvidence, provider)'
    )
    const bashObservationStart = script.indexOf(
      'const bashHostObservation = await observeHostToolExecution',
      providerEvidenceStart
    )
    const preHostSnapshotStart = script.indexOf(
      'const preHostFailureSnapshot = ordinaryWorkflowFailureStageSnapshot(report)',
      providerEvidenceStart
    )
    const completedEvidenceStart = script.indexOf(
      'readObserved: groups.read',
      bashObservationStart
    )
    const terminalFailureStart = script.indexOf(
      'if (ordinaryFailureCode) throw new Error(ordinaryFailureCode)',
      completedEvidenceStart
    )

    expect(providerEvidenceStart).toBeGreaterThan(-1)
    expect(preHostSnapshotStart).toBeGreaterThan(providerEvidenceStart)
    expect(preHostSnapshotStart).toBeLessThan(bashObservationStart)
    expect(bashObservationStart).toBeGreaterThan(providerEvidenceStart)
    expect(script.slice(providerEvidenceStart, bashObservationStart))
      .not.toContain('throw new Error(')
    expect(completedEvidenceStart).toBeGreaterThan(bashObservationStart)
    expect(terminalFailureStart).toBeGreaterThan(completedEvidenceStart)
  })

  it('closes failed, skipped, and live-blocked projections without losing completed stage snapshots', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      status: 'failed',
      passed: false,
      executionBlocker: 'continuation_observer_failed',
      executionBlockerClassification: 'product_or_harness_failure',
      outputPath: '/private/task/report.json',
      blockers: ['/private/task/provider-body.txt'],
      workflow: {
        ordinaryWorkflowCompleted: false,
        agentModeRestored: false,
        sameThreadPlanAgent: false,
        readObserved: false,
        planObserved: false,
        todoObserved: false,
        writeObserved: false,
        realTestObserved: false,
        subagentObserved: false,
        successfulToolResultsObserved: false,
        todosCompleted: false,
        exactlyOneBoundedSubagentCompleted: false,
        isolatedGitRepositoryObserved: false,
        protectedRepositoryInputsBound: false,
        onlyIntendedSourceChanged: false,
        parentOwnedTestCrossCheckPassed: false,
        gitCommandObserved: false,
        skillObserved: false,
        ordinaryMCPObserved: false,
        researchWritingObserved: false,
        longContextContinuationObserved: false,
        compactionCount: 0
      },
      checks: [
        { id: 'ordinary-agent-workflow', status: 'passed' },
        { id: 'parent-owned-repository-test-cross-check', status: 'passed' },
        { id: 'real-repository-test', status: 'passed' },
        { id: 'bounded-subagent', status: 'passed' },
        { id: 'git-skill-mcp-research-writing', status: 'passed' },
        { id: 'protected-funds-source-unavailable', status: 'passed' },
        { id: 'fresh-packaged-relaunch', status: 'live_blocked' },
        { id: 'long-context-continuation', status: 'failed', message: 'continuation failed' },
        { id: 'composer-compaction-submit', status: 'skipped' }
      ],
      stageSnapshots: [
        {
          stage: 'protected-funds-source-unavailable',
          checkIds: ['protected-funds-source-unavailable'],
          completedFields: {
            protectedFundsSourceUnavailableObserved: true,
            protectedFundsAcceptedFinalDigest: 'b'.repeat(64),
            protectedFundsClaimCount: 0,
            protectedFundsReceiptCount: 0,
            protectedFundsSuccessfulExecutionCount: 0
          }
        },
        {
          stage: 'diagnostic-snapshot',
          checkIds: ['long-context-continuation'],
          completedFields: {
            sourceDigest: 'a'.repeat(64),
            rawPrompt: 'PRIVATE_PROMPT_MUST_NOT_SURVIVE'
          }
        }
      ]
    }, {
      requiredCheckIds: [
        'ordinary-agent-workflow',
        'parent-owned-repository-test-cross-check',
        'real-repository-test',
        'bounded-subagent',
        'git-skill-mcp-research-writing',
        'long-context-continuation',
        'composer-compaction-submit',
        'fresh-packaged-relaunch'
      ]
    })
    expect(report.passed).toBe(false)
    expect(report.status).toBe('failed')
    expect(report.failedCheckIds).toEqual(['long-context-continuation'])
    expect(report.skippedCheckIds).toEqual(['composer-compaction-submit'])
    expect(report.liveBlockedCheckIds).toEqual(['fresh-packaged-relaunch'])
    expect(report.failureCheckIds).toEqual([
      'fresh-packaged-relaunch',
      'long-context-continuation',
      'composer-compaction-submit'
    ])
    expect(report.failureCheckIds).toEqual(
      report.checks.filter((item: any) => item.status !== 'passed').map((item: any) => item.id)
    )
    expect(report.failureCheckIdsSemantics).toBe('all_non_pass')
    expect(report.checks.filter((item: any) => item.id === 'ordinary-agent-workflow')).toHaveLength(1)
    expect(report.checks.find((item: any) => item.id === 'ordinary-agent-workflow')?.status)
      .toBe('passed')
    expect(report.checks.find((item: any) => item.id === 'protected-funds-source-unavailable'))
      .toEqual({
        id: 'protected-funds-source-unavailable',
        status: 'passed',
        message: 'passed'
      })
    expect(report.workflow).toEqual(expect.objectContaining({
      ordinaryWorkflowCompleted: true,
      agentModeRestored: true,
      sameThreadPlanAgent: true,
      readObserved: true,
      planObserved: true,
      todoObserved: true,
      writeObserved: true,
      realTestObserved: true,
      subagentObserved: true,
      successfulToolResultsObserved: true,
      todosCompleted: true,
      exactlyOneBoundedSubagentCompleted: true,
      isolatedGitRepositoryObserved: true,
      protectedRepositoryInputsBound: true,
      onlyIntendedSourceChanged: true,
      parentOwnedTestCrossCheckPassed: true,
      gitCommandObserved: true,
      skillObserved: true,
      ordinaryMCPObserved: true,
      researchWritingObserved: true,
      longContextContinuationObserved: false
    }))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'ordinary-agent-workflow',
        checkIds: ['ordinary-agent-workflow', 'real-repository-test', 'bounded-subagent'],
        completedFields: expect.objectContaining({
          ordinaryWorkflowCompleted: true,
          agentModeRestored: true,
          sameThreadPlanAgent: true,
          readObserved: true,
          realTestObserved: true,
          subagentObserved: true,
          successfulToolResultsObserved: true,
          todosCompleted: true,
          exactlyOneBoundedSubagentCompleted: true,
          isolatedGitRepositoryObserved: true,
          protectedRepositoryInputsBound: true,
          onlyIntendedSourceChanged: true,
          parentOwnedTestCrossCheckPassed: true,
          gitCommandObserved: true,
          skillObserved: true,
          ordinaryMCPObserved: true,
          researchWritingObserved: true
        })
      }),
      expect.objectContaining({
        stage: 'protected-funds-source-unavailable',
        checkIds: ['protected-funds-source-unavailable'],
        completedFields: expect.objectContaining({
          protectedFundsSourceUnavailableObserved: true,
          protectedFundsAcceptedFinalDigest: 'b'.repeat(64),
          protectedFundsClaimCount: 0,
          protectedFundsReceiptCount: 0,
          protectedFundsSuccessfulExecutionCount: 0
        })
      })
    ]))
    expect(report.executionBlocker).toBe('continuation_observer_failed')
    expect(report).not.toHaveProperty('outputPath')
    expect(report.blockers).toEqual(report.failureCheckIds)
    expect(JSON.stringify(report)).not.toContain('PRIVATE_PROMPT_MUST_NOT_SURVIVE')
    expect(JSON.stringify(report)).not.toContain('/private/task')
    expect(JSON.stringify(report)).not.toContain('continuation failed')
  })

  it('keeps completed ordinary functional evidence when its provider receipt observer fails', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      status: 'failed',
      passed: false,
      executionBlocker: 'packaged_ordinary_provider_receipt_failed',
      executionBlockerClassification: 'product_or_harness_failure',
      workflow: {
        ordinaryWorkflowCompleted: false,
        agentModeRestored: true,
        sameThreadPlanAgent: true,
        readObserved: true,
        planObserved: true,
        todoObserved: true,
        writeObserved: true,
        realTestObserved: true,
        subagentObserved: true,
        successfulToolResultsObserved: true,
        todosCompleted: true,
        exactlyOneBoundedSubagentCompleted: true,
        isolatedGitRepositoryObserved: true,
        protectedRepositoryInputsBound: true,
        onlyIntendedSourceChanged: true,
        parentOwnedTestCrossCheckPassed: true,
        gitCommandObserved: true,
        skillObserved: true,
        ordinaryMCPObserved: true,
        researchWritingObserved: true,
        longContextContinuationObserved: false,
        compactionCount: 0
      },
      checks: [
        {
          id: 'ordinary-agent-workflow',
          status: 'failed',
          message: 'packaged_ordinary_provider_receipt_failed'
        },
        { id: 'parent-owned-repository-test-cross-check', status: 'passed' },
        { id: 'real-repository-test', status: 'passed' },
        { id: 'bounded-subagent', status: 'passed' },
        { id: 'git-skill-mcp-research-writing', status: 'passed' },
        { id: 'long-context-continuation', status: 'skipped' }
      ]
    }, {
      requiredCheckIds: [
        'ordinary-agent-workflow',
        'real-repository-test',
        'bounded-subagent',
        'git-skill-mcp-research-writing',
        'long-context-continuation'
      ]
    })

    expect(report.status).toBe('failed')
    expect(report.failedCheckIds).toEqual(['ordinary-agent-workflow'])
    expect(report.skippedCheckIds).toEqual(['long-context-continuation'])
    expect(report.liveBlockedCheckIds).toEqual([])
    expect(report.workflow).toEqual(expect.objectContaining({
      ordinaryWorkflowCompleted: false,
      readObserved: true,
      planObserved: true,
      todoObserved: true,
      writeObserved: true,
      realTestObserved: true,
      subagentObserved: true,
      successfulToolResultsObserved: true,
      todosCompleted: true,
      exactlyOneBoundedSubagentCompleted: true,
      gitCommandObserved: true,
      skillObserved: true,
      ordinaryMCPObserved: true,
      researchWritingObserved: true,
      longContextContinuationObserved: false
    }))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'ordinary-agent-functional-workflow',
        checkIds: [
          'real-repository-test',
          'bounded-subagent',
          'git-skill-mcp-research-writing'
        ],
        completedFields: expect.objectContaining({
          readObserved: true,
          planObserved: true,
          todoObserved: true,
          writeObserved: true,
          realTestObserved: true,
          subagentObserved: true,
          successfulToolResultsObserved: true,
          todosCompleted: true,
          exactlyOneBoundedSubagentCompleted: true,
          gitCommandObserved: true,
          skillObserved: true,
          ordinaryMCPObserved: true,
          researchWritingObserved: true
        })
      })
    ]))
    expect(JSON.stringify(report)).not.toContain('provider body')
  })

  it('retains a redacted partial ordinary-stage snapshot when source authority rejects a completed turn', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      executionBlocker: 'external_repository_expected_result_mismatch',
      executionBlockerClassification: 'product_or_harness_failure',
      repository: {
        blocker: 'external_repository_expected_result_mismatch',
        finalExpectedSourceBound: false,
        finalExpectedSourceFileCount: 1,
        finalExpectedSourceMismatchCount: 1,
        finalExpectedSourceDigest: 'a'.repeat(64),
        finalObservedSourceDigest: 'b'.repeat(64)
      },
      workflow: {
        ordinaryWorkflowDisposition: 'completed',
        ordinaryWorkflowCompleted: false,
        ordinaryCandidateToolAttemptCount: 19,
        ordinaryCandidateSuccessfulToolExecutionCount: 14,
        ordinaryCandidateFailedToolResultCount: 5,
        ordinaryCandidateUnsettledToolResultCount: 3,
        ordinaryCandidateProviderAttemptCount: 16,
        ordinaryCandidateProviderReceiptCountersBound: true,
        readObserved: true,
        planObserved: true,
        todoObserved: true,
        writeObserved: false,
        realTestObserved: false,
        subagentObserved: true,
        gitCommandObserved: true,
        skillObserved: true,
        ordinaryMCPObserved: true,
        researchWritingObserved: true,
        protectedRepositoryInputsBound: true,
        onlyIntendedSourceChanged: true
      },
      checks: [
        {
          id: 'ordinary-agent-workflow',
          status: 'failed',
          message: 'external_repository_expected_result_mismatch'
        },
        {
          id: 'parent-owned-repository-test-cross-check',
          status: 'failed',
          message: 'external_repository_expected_result_mismatch'
        },
        {
          id: 'real-repository-test',
          status: 'failed',
          message: 'external_repository_expected_result_mismatch'
        },
        { id: 'bounded-subagent', status: 'passed' },
        {
          id: 'git-skill-mcp-research-writing',
          status: 'failed',
          message: 'milestone_a_execution_failed'
        },
        { id: 'long-context-continuation', status: 'skipped' }
      ],
      stageSnapshots: [{
        stage: 'unsafe-diagnostic-input',
        checkIds: ['ordinary-agent-workflow'],
        completedFields: { rawPrompt: 'PRIVATE_PROMPT_MUST_NOT_SURVIVE' }
      }]
    }, {
      requiredCheckIds: [
        'ordinary-agent-workflow',
        'parent-owned-repository-test-cross-check',
        'real-repository-test',
        'bounded-subagent',
        'git-skill-mcp-research-writing',
        'long-context-continuation'
      ]
    })

    const snapshot = report.stageSnapshots.find((item: Record<string, any>) =>
      item.stage === 'ordinary-agent-workflow-failure'
    )
    expect(snapshot).toEqual(expect.objectContaining({
      checkIds: [
        'ordinary-agent-workflow',
        'parent-owned-repository-test-cross-check',
        'real-repository-test',
        'bounded-subagent',
        'git-skill-mcp-research-writing'
      ],
      completedFields: expect.objectContaining({
        ordinaryCandidateToolAttemptCount: 19,
        ordinaryCandidateSuccessfulToolExecutionCount: 14,
        ordinaryCandidateFailedToolResultCount: 5,
        ordinaryCandidateUnsettledToolResultCount: 3,
        ordinaryCandidateProviderAttemptCount: 16,
        ordinaryCandidateProviderReceiptCountersBound: true,
        readObserved: true,
        planObserved: true,
        todoObserved: true,
        writeObserved: false,
        realTestObserved: false,
        subagentObserved: true,
        protectedRepositoryInputsBound: true,
        onlyIntendedSourceChanged: true,
        finalExpectedSourceBound: false,
        finalExpectedSourceFileCount: 1,
        finalExpectedSourceMismatchCount: 1,
        finalExpectedSourceDigest: 'a'.repeat(64),
        finalObservedSourceDigest: 'b'.repeat(64)
      }),
      snapshotDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(Object.isFrozen(snapshot)).toBe(true)
    expect(Object.isFrozen(snapshot?.completedFields)).toBe(true)
    expect(report.failedCheckIds).toEqual([
      'ordinary-agent-workflow',
      'parent-owned-repository-test-cross-check',
      'real-repository-test',
      'git-skill-mcp-research-writing'
    ])
    expect(report.skippedCheckIds).toEqual(['long-context-continuation'])
    expect(JSON.stringify(report)).not.toContain('PRIVATE_PROMPT_MUST_NOT_SURVIVE')
  })

  it('closes passed continuation and first-quit snapshots before a later relaunch block', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      executionBlocker: 'packaged_hub_account_readiness_required',
      executionBlockerClassification: 'external_prerequisite',
      workflow: {
        longContextContinuationObserved: false,
        normalFirstQuitObserved: true
      },
      checks: [
        { id: 'long-context-continuation', status: 'passed' },
        { id: 'normal-first-quit', status: 'passed' },
        {
          id: 'fresh-packaged-relaunch',
          status: 'live_blocked',
          message: 'packaged_hub_account_readiness_required'
        }
      ],
      stageSnapshots: [{
        stage: 'long-context-continuation',
        checkIds: ['long-context-continuation'],
        completedFields: {
          longContextContinuationObserved: false,
          longContextHostReadBound: true
        }
      }]
    }, {
      requiredCheckIds: [
        'long-context-continuation',
        'normal-first-quit',
        'fresh-packaged-relaunch'
      ]
    })

    expect(report.liveBlockedCheckIds).toEqual(['fresh-packaged-relaunch'])
    expect(report.workflow).toEqual(expect.objectContaining({
      longContextContinuationObserved: true,
      normalFirstQuitObserved: true
    }))
    expect(report.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'long-context-continuation',
        checkIds: ['long-context-continuation'],
        completedFields: expect.objectContaining({
          longContextContinuationObserved: true,
          longContextHostReadBound: true
        })
      }),
      expect.objectContaining({
        stage: 'normal-first-quit',
        checkIds: ['normal-first-quit'],
        completedFields: expect.objectContaining({
          normalFirstQuitObserved: true
        })
      })
    ]))
  })

  it('captures successful continuation and first-quit workflow fields at their stage boundaries', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const continuationOutcome = script.indexOf('const longContextContinuationObserved =')
    const continuationProjection = script.indexOf('report.workflow = {', continuationOutcome)
    const continuationSnapshot = script.indexOf(
      "'long-context-continuation',\n      ['long-context-continuation']",
      continuationProjection
    )
    expect(continuationOutcome).toBeGreaterThan(0)
    expect(continuationProjection).toBeGreaterThan(continuationOutcome)
    expect(continuationSnapshot).toBeGreaterThan(continuationProjection)
    expect(script.slice(continuationProjection, continuationSnapshot))
      .toContain('longContextContinuationObserved')

    const firstQuitCheck = script.indexOf(
      "report.checks.push(check(\n      'normal-first-quit'"
    )
    const firstQuitExit = script.indexOf('if (!firstNormalQuit)', firstQuitCheck)
    expect(firstQuitCheck).toBeGreaterThan(0)
    expect(firstQuitExit).toBeGreaterThan(firstQuitCheck)
    expect(script.slice(firstQuitCheck, firstQuitExit)).toContain(
      "'normal-first-quit',\n        ['normal-first-quit']"
    )
  })

  it('classifies missing local setup separately from invalid configured authority without activating Hub', async () => {
    const { classifyLocalProviderWorkbenchReadiness, readonlyObservationExpression } = await milestoneModule()
    const empty = { schemaVersion: 1, registryRevision: '0',
      registryIncarnation: 'inc_' + 'a'.repeat(43), providers: [] }
    expect(classifyLocalProviderWorkbenchReadiness(empty, {
      blocker: 'local_provider_registry_authority_not_ready', blocked: true
    })).toEqual({ blocked: true, blocker: 'local_provider_registry_authority_not_ready' })
    expect(classifyLocalProviderWorkbenchReadiness({ ...empty, registryRevision: '1' }, {
      blocker: 'local_provider_settings_invalid', blocked: false
    })).toEqual({ blocked: false, blocker: 'local_provider_settings_invalid' })
    let hubCalls = 0
    const observation = await new Function('window', 'document',
      'return (' + readonlyObservationExpression('/isolated/repository') + ')'
    )({ analytix: {
      account: { getSnapshot: async () => { hubCalls++; throw new Error('must not activate Hub') } },
      providerRegistry: { request: async (request: unknown) => {
        expect(request).toEqual({ schemaVersion: 1, operation: 'list' })
        return empty
      } }
    } }, { title: 'Analytix', querySelector: () => null })
    expect(observation.providerRegistry).toEqual(empty)
    expect(hubCalls).toBe(0)
  })

  it('preserves every harness-owned auxiliary check without treating it as required or unknown', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      workflow: {},
      checks: [
        { id: 'external-real-repository-contract', status: 'passed' },
        { id: 'ordinary-agent-workflow', status: 'passed' },
        { id: 'parent-owned-repository-test-cross-check', status: 'passed' }
      ]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow']
    })
    expect(report.passed).toBe(true)
    expect(report.failedCheckIds).toEqual([])
    expect(report.checks).toEqual(expect.arrayContaining([
      { id: 'external-real-repository-contract', status: 'passed', message: 'passed' },
      { id: 'parent-owned-repository-test-cross-check', status: 'passed', message: 'passed' }
    ]))
  })

  it('projects a safe execution blocker instead of a rejected human failure message', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      executionBlocker: 'external_repository_expected_result_mismatch',
      executionBlockerClassification: 'product_or_harness_failure',
      workflow: {},
      checks: [{
        id: 'ordinary-agent-workflow',
        status: 'failed',
        message: 'human diagnostic that must not be copied into the formal report'
      }]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow']
    })
    expect(report.checks).toContainEqual({
      id: 'ordinary-agent-workflow',
      status: 'failed',
      message: 'external_repository_expected_result_mismatch'
    })
    expect(JSON.stringify(report)).not.toContain('human diagnostic')
  })

  it('derives a safe execution blocker from the direct preflight failure before projecting skipped checks', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const report = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      workflow: {},
      checks: [
        {
          id: 'external-real-repository-contract',
          status: 'failed',
          message: 'external_repository_plan_exclude_preconfigured'
        },
        {
          id: 'trusted-cache-tmpdir',
          status: 'passed'
        },
        {
          id: 'formal-packaged-artifact',
          status: 'passed'
        },
        {
          id: 'ordinary-agent-workflow',
          status: 'skipped',
          message: 'formal package prerequisites prevented this public-seam check'
        }
      ]
    }, {
      requiredCheckIds: [
        'trusted-cache-tmpdir',
        'formal-packaged-artifact',
        'ordinary-agent-workflow'
      ]
    })
    expect(report.executionBlocker).toBe('external_repository_plan_exclude_preconfigured')
    expect(report.failedCheckIds).toEqual(['external-real-repository-contract'])
    expect(report.skippedCheckIds).toEqual(['ordinary-agent-workflow'])
    expect(report.checks.find((item: any) => item.id === 'ordinary-agent-workflow'))
      .toEqual({
        id: 'ordinary-agent-workflow',
        status: 'skipped',
        message: 'external_repository_plan_exclude_preconfigured'
      })
    expect(JSON.stringify(report)).not.toContain('check_projection_rejected')
    expect(JSON.stringify(report)).not.toContain('formal package prerequisites')

    const unsafe = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      workflow: {},
      checks: [{
        id: 'ordinary-agent-workflow',
        status: 'failed',
        message: '/private/task/provider-body.txt contains credential material'
      }]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow']
    })
    expect(unsafe.executionBlocker).toBe('milestone_a_execution_failed')
    expect(JSON.stringify(unsafe)).not.toContain('/private/task')
    expect(JSON.stringify(unsafe)).not.toContain('credential material')

    const unknown = finalizeMilestoneAReport({
      schemaVersion: 1,
      id: 'runtime-go-packaged-milestone-a',
      workflow: {},
      checks: [{
        id: 'untrusted-check',
        status: 'failed',
        message: 'attacker_controlled_code'
      }]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow']
    })
    expect(unknown.executionBlocker).toBe('milestone_a_execution_failed')
    expect(JSON.stringify(unknown)).not.toContain('attacker_controlled_code')
  })

  it('projects readonly CDP protocol and exception details to a safe closed reason code', async () => {
    const { finalizeMilestoneAReport, readonlyCdpFailureReasonCode } = await milestoneModule()
    const protocol = readonlyCdpFailureReasonCode({
      error: {
        code: -32602,
        message: 'Invalid parameters: PRIVATE_PROTOCOL_MESSAGE'
      }
    })
    const executionContext = readonlyCdpFailureReasonCode({
      error: {
        code: -32000,
        message: 'Execution context was destroyed: PRIVATE_CONTEXT_MESSAGE'
      }
    })
    const exception = readonlyCdpFailureReasonCode({
      result: {
        exceptionDetails: {
          text: 'PRIVATE_EXCEPTION_TEXT',
          exception: { description: 'PRIVATE_EXCEPTION_DESCRIPTION' }
        }
      }
    })

    expect(protocol).toBe('readonly_cdp_protocol_invalid_params')
    expect(executionContext).toBe('readonly_cdp_execution_context_unavailable')
    expect(exception).toBe('readonly_cdp_exception')
    expect(JSON.stringify({ protocol, executionContext, exception })).not.toContain('PRIVATE_')

    const report = finalizeMilestoneAReport({
      executionBlocker: 'Execution context was destroyed: PRIVATE_REPORT_MESSAGE',
      executionBlockerClassification: 'product_or_harness_failure',
      checks: [{
        id: 'composer-workflow-submit',
        status: 'failed',
        message: 'Execution context was destroyed: PRIVATE_REPORT_MESSAGE'
      }]
    }, { requiredCheckIds: ['composer-workflow-submit'] })
    expect(JSON.stringify(report)).not.toContain('PRIVATE_REPORT_MESSAGE')
  })

  it('retries one transient readonly CDP execution-context failure with a fresh target only', async () => {
    const { readComposerReasoningGeometry } = await milestoneModule()
    const targetIds: string[] = []
    const evaluations: string[] = []
    const sleeps: number[] = []
    const observed = await readComposerReasoningGeometry(
      43210,
      'managed-model',
      'Reasoning',
      5000,
      {
        waitForDebugTarget: async () => {
          const id = `target-${targetIds.length + 1}`
          targetIds.push(id)
          return {
            webSocketDebuggerUrl: `ws://milestone-a.invalid/devtools/page/${id}`,
            pageTargetCount: 1
          }
        },
        evaluateReadonlyCdp: async (webSocketDebuggerUrl: string) => {
          evaluations.push(webSocketDebuggerUrl)
          if (evaluations.length === 1) {
            throw Object.assign(new Error('PRIVATE_CONTEXT_MESSAGE'), {
              readonlyCdpReasonCode: 'readonly_cdp_execution_context_unavailable'
            })
          }
          return { controlCount: 1, control: { x: 1, y: 1 }, controlExpanded: false }
        },
        sleep: async (delayMs: number) => {
          sleeps.push(delayMs)
        }
      }
    )

    expect(observed.target.webSocketDebuggerUrl).toContain('/target-2')
    expect(targetIds).toEqual(['target-1', 'target-2'])
    expect(evaluations).toHaveLength(2)
    expect(sleeps).toHaveLength(1)
    expect(sleeps[0]).toBeGreaterThan(0)

    let nonTransientEvaluations = 0
    await expect(readComposerReasoningGeometry(
      43210,
      'managed-model',
      'Reasoning',
      5000,
      {
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/non-transient',
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => {
          nonTransientEvaluations += 1
          throw Object.assign(new Error('PRIVATE_INVALID_PARAMS'), {
            readonlyCdpReasonCode: 'readonly_cdp_protocol_invalid_params'
          })
        },
        sleep: async () => {
          throw new Error('non-transient CDP failures must not retry')
        }
      }
    )).rejects.toThrow('readonly_cdp_protocol_invalid_params')
    expect(nonTransientEvaluations).toBe(1)
  })

  it('returns a domReady=false sentinel when readonly composer expressions have no DOM global', async () => {
    const {
      readonlyComposerGeometryExpression,
      readonlyComposerModeGeometryExpression,
      readonlyComposerReasoningGeometryExpression
    } = await milestoneModule()
    const evaluateWithoutDocument = (expression: string) =>
      new Function(`return (${expression})`)()

    expect(evaluateWithoutDocument(readonlyComposerGeometryExpression(['Send']))).toEqual(
      expect.objectContaining({ domReady: false })
    )
    expect(evaluateWithoutDocument(readonlyComposerModeGeometryExpression())).toEqual(
      expect.objectContaining({ domReady: false })
    )
    expect(evaluateWithoutDocument(
      readonlyComposerReasoningGeometryExpression('managed-model', 'Reasoning')
    )).toEqual(expect.objectContaining({ domReady: false }))
  })

  it('fails closed on every fixed readonly observation stage without leaking raw renderer text', async () => {
    const {
      READONLY_OBSERVATION_READ_ERROR_STAGES,
      observeRenderer,
      readonlyObservationExpression
    } = await milestoneModule()
    const stages = [
      'api_access',
      'document_title',
      'composer_query',
      'primary_button_query',
      'thread_detail',
      'thread_summary',
      'provider_receipt_replay',
      'accepted_final_replay',
      'unknown'
    ]
    expect(READONLY_OBSERVATION_READ_ERROR_STAGES).toEqual(
      expect.arrayContaining(stages)
    )

    for (const stage of stages) {
      let evaluationCount = 0
      const privateText = `PRIVATE_OBSERVATION_${stage}`
      await expect(observeRenderer({
        debugPort: 43210,
        workspace: '/isolated/repository',
        timeoutMs: 5000
      }, {
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/observation',
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => {
          evaluationCount += 1
          return { readErrorStage: stage, privateText }
        }
      })).rejects.toEqual(expect.objectContaining({
        readonlyCdpReasonCode: 'readonly_cdp_exception',
        readonlyCdpExpressionStage: stage
      }))
      expect(evaluationCount).toBe(1)
    }

    const evaluate = (
      windowValue: any,
      documentValue: any,
      providerReceiptScope: Record<string, any> | null = null,
      acceptedFinalReceiptScope: Record<string, any> | null = null
    ) => {
      const expression = readonlyObservationExpression(
        '/isolated/repository',
        'thread-observation',
        providerReceiptScope,
        acceptedFinalReceiptScope
      )
      return new Function('window', 'document', `return (${expression})`)(
        windowValue,
        documentValue
      )
    }
    const privateText = 'PRIVATE_RAW_RENDERER_EXCEPTION'
    const apiAccessWindow = {}
    Object.defineProperty(apiAccessWindow, 'analytix', {
      get: () => { throw new Error(privateText) }
    })
    await expect(evaluate(apiAccessWindow, {})).resolves.toEqual(
      expect.objectContaining({ readErrorStage: 'api_access' })
    )

    const titleDocument = {
      get title() { throw new Error(privateText) },
      querySelector: () => null
    }
    await expect(evaluate({ analytix: null }, titleDocument)).resolves.toEqual(
      expect.objectContaining({ readErrorStage: 'document_title' })
    )

    let composerQueryCount = 0
    await expect(evaluate({ analytix: null }, {
      title: 'Analytix',
      querySelector: () => {
        composerQueryCount += 1
        throw new Error(privateText)
      }
    })).resolves.toEqual(expect.objectContaining({ readErrorStage: 'composer_query' }))
    expect(composerQueryCount).toBe(1)

    let primaryQueryCount = 0
    await expect(evaluate({ analytix: null }, {
      title: 'Analytix',
      querySelector: () => {
        primaryQueryCount += 1
        if (primaryQueryCount === 1) return null
        throw new Error(privateText)
      }
    })).resolves.toEqual(expect.objectContaining({
      readErrorStage: 'primary_button_query'
    }))
    expect(primaryQueryCount).toBe(2)

    const managedSettings = {
      provider: {
        activeProviderId: 'analytix-hub',
        providers: [{ id: 'analytix-hub', apiKey: '' }]
      },
      runtime: { providerId: 'analytix-hub' }
    }
    const baseDocument = { title: 'Analytix', querySelector: () => null }
    const runtimeResponse = (body: any = {}) => ({
      ok: true,
      status: 200,
      body: JSON.stringify(body)
    })
    const managedApi = (runtimeRequest: (path: string) => Promise<any>, runtimeOverrides = {}) => ({
      account: { getSnapshot: async () => ({
        authenticated: true,
        gatewayConfigured: true,
        accountReady: true,
        source: 'hub'
      }) },
      settings: { getSettings: async () => managedSettings },
      runtime: Object.assign(Object.create(runtimeOverrides), { runtimeRequest })
    })
    const commonRuntimeRequest = async (path: string) => {
      if (path === '/v1/threads/thread-observation') {
        return runtimeResponse({ id: 'thread-observation', workspace: '/isolated/repository' })
      }
      if (path.endsWith('/summary')) return runtimeResponse({})
      if (path === '/v1/threads?limit=1') return runtimeResponse({ threads: [] })
      return runtimeResponse({})
    }
    await expect(evaluate({
      analytix: managedApi(async (path) => {
        if (path === '/v1/threads/thread-observation') throw new Error(privateText)
        return commonRuntimeRequest(path)
      })
    }, baseDocument)).resolves.toEqual(expect.objectContaining({
      readErrorStage: 'thread_detail'
    }))
    await expect(evaluate({
      analytix: managedApi(async (path) => {
        if (path.endsWith('/summary')) throw new Error(privateText)
        return commonRuntimeRequest(path)
      })
    }, baseDocument)).resolves.toEqual(expect.objectContaining({
      readErrorStage: 'thread_summary'
    }))

    const replayRuntime = {
      get startSse() { throw new Error(privateText) },
      ackSseEvent: async () => true,
      stopSse: async () => {},
      onSseEvent: () => () => {},
      onSseEnd: () => () => {},
      onSseError: () => () => {}
    }
    await expect(evaluate({
      analytix: managedApi(commonRuntimeRequest, replayRuntime)
    }, baseDocument, {
      baselineSeq: 1,
      expectedTurnId: 'turn-observation',
      expectedHighestSeq: 3
    })).resolves.toEqual(expect.objectContaining({
      readErrorStage: 'provider_receipt_replay'
    }))
    await expect(evaluate({
      analytix: managedApi(commonRuntimeRequest, replayRuntime)
    }, baseDocument, null, {
      baselineSeq: 1,
      expectedTurnId: 'turn-observation',
      expectedHighestSeq: 3
    })).resolves.toEqual(expect.objectContaining({
      readErrorStage: 'accepted_final_replay'
    }))
    expect(JSON.stringify(await evaluate({ analytix: null }, titleDocument))).not.toContain(
      privateText
    )
  })

  it('keeps the serialized provider observer self-contained in an isolated renderer context', async () => {
    const { readonlyObservationExpression } = await milestoneModule()
    const threadId = 'thread-serialized-provider-observer'
    const response = (body: any) => ({
      ok: true,
      status: 200,
      body: JSON.stringify(body)
    })
    const runtimeRequest = vi.fn(async (path: string) => {
      if (path === `/v1/threads/${threadId}`) {
        return response({ id: threadId, workspace: '/isolated/repository' })
      }
      if (path.endsWith('/summary')) return response({})
      if (path === '/v1/threads?limit=1') return response({ threads: [] })
      return response({})
    })
    const callbacks: Record<string, (payload: any) => void> = {}
    const runtime = {
      runtimeRequest,
      onSseEvent: (handler: (payload: any) => void) => {
        callbacks.event = handler
        return vi.fn()
      },
      onSseEnd: (handler: (payload: any) => void) => {
        callbacks.end = handler
        return vi.fn()
      },
      onSseError: (handler: (payload: any) => void) => {
        callbacks.error = handler
        return vi.fn()
      },
      startSse: vi.fn(async (_observedThreadId: string, _sinceSeq: number, streamId: string) => {
        callbacks.event({
          streamId,
          events: [{
            kind: 'heartbeat',
            seq: 1,
            timestamp: '2026-08-14T00:00:00Z',
            threadId
          }]
        })
        callbacks.end({ streamId })
        return { streamId }
      }),
      ackSseEvent: vi.fn(async () => true),
      stopSse: vi.fn(async () => false)
    }
    const expression = readonlyObservationExpression(
      '/isolated/repository',
      threadId,
      { baselineSeq: 1, expectedTurnId: 'turn-serialized-provider-observer', expectedHighestSeq: 3 }
    )
    const evaluate = new Function('window', 'document', `return (${expression})`)
    const observation = await evaluate(
      {
        analytix: {
          account: {
            getSnapshot: async () => ({
              authenticated: true,
              gatewayConfigured: true,
              accountReady: true,
              source: 'hub'
            })
          },
          settings: {
            getSettings: async () => ({
              provider: {
                activeProviderId: 'analytix-hub',
                providers: [{ id: 'analytix-hub', apiKey: '' }]
              },
              runtime: { providerId: 'analytix-hub' }
            })
          },
          runtime
        }
      },
      { title: 'Analytix', querySelector: () => null }
    )

    expect(observation).not.toHaveProperty('readErrorStage')
    expect(observation.providerReceiptTrace).toEqual(expect.objectContaining({
      reasonCode: 'provider_receipt_target_turn_missing'
    }))
    expect(runtime.startSse).toHaveBeenCalledWith(threadId, 1, expect.any(String))
  })

  it('maps exception details by known context wording and preserves a separate plan observation diagnostic', async () => {
    const {
      applyPlanObservationDiagnostic,
      finalizeMilestoneAReport,
      readonlyCdpFailureReasonCode
    } = await milestoneModule()
    const context = readonlyCdpFailureReasonCode({
      result: {
        exceptionDetails: { text: 'Execution context was destroyed: PRIVATE_CONTEXT' }
      }
    })
    const generic = readonlyCdpFailureReasonCode({
      result: {
        exceptionDetails: { text: 'PRIVATE_GENERIC_EXCEPTION' }
      }
    })
    expect(context).toBe('readonly_cdp_execution_context_unavailable')
    expect(generic).toBe('readonly_cdp_exception')
    expect(JSON.stringify({ context, generic })).not.toContain('PRIVATE_')

    const report = { workflow: {} as Record<string, any> }
    applyPlanObservationDiagnostic(report, {
      reasonCode: 'readonly_cdp_exception',
      phase: 'provider_receipt_replay',
      operation: 'read',
      expressionStage: 'thread_summary'
    })
    expect(report.workflow).toEqual({
      planObservationFailureReasonCode: 'readonly_cdp_exception',
      planObservationFailurePhase: 'provider_receipt_replay',
      planObservationFailureOperation: 'read',
      planObservationFailureExpressionStage: 'thread_summary'
    })

    const finalized = finalizeMilestoneAReport({
      workflow: report.workflow,
      executionBlocker: 'readonly_cdp_exception',
      executionBlockerClassification: 'product_or_harness_failure',
      checks: [{
        id: 'composer-workflow-submit',
        status: 'failed',
        message: 'readonly_cdp_exception'
      }]
    }, { requiredCheckIds: ['composer-workflow-submit'] })
    expect(finalized.workflow).toEqual(expect.objectContaining(report.workflow))

    const success = { workflow: {} as Record<string, any> }
    applyPlanObservationDiagnostic(success, null)
    expect(success.workflow).toEqual({
      planObservationFailureReasonCode: '',
      planObservationFailurePhase: 'none',
      planObservationFailureOperation: 'none',
      planObservationFailureExpressionStage: 'none'
    })

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    expect(script).toContain('planComposerSubmitted: false')
    const modeSnapshot = script.indexOf('report.workflow.planModeSelected = planMode.ok')
    const planPrompt = script.indexOf('const planPrompt = milestoneAPlanPrompt')
    const submitSnapshot = script.indexOf(
      'report.workflow.planComposerSubmitted = true'
    )
    expect(modeSnapshot).toBeGreaterThan(0)
    expect(modeSnapshot).toBeLessThan(planPrompt)
    expect(submitSnapshot).toBeGreaterThan(script.indexOf('if (!planSubmit.ok)'))
  })

  it('retries domReady=false once for each readonly composer geometry helper and keeps other failures closed', async () => {
    const {
      readComposerGeometry,
      readComposerModeGeometry,
      readComposerReasoningGeometry
    } = await milestoneModule()
    const readCases = [
      {
        read: (dependencies: Record<string, any>) => readComposerGeometry(
          43210,
          ['Send'],
          5000,
          dependencies
        ),
        readyValue: {
          domReady: true,
          editor: { x: 1, y: 1 },
          editorTextLength: 0,
          button: { x: 2, y: 2 },
          buttonDisabled: true,
          buttonLoading: false,
          buttonLabelAccepted: true
        }
      },
      {
        read: (dependencies: Record<string, any>) => readComposerModeGeometry(
          43210,
          5000,
          dependencies
        ),
        readyValue: {
          domReady: true,
          menuButton: { x: 1, y: 1 },
          planButton: { x: 2, y: 2 },
          planChecked: false
        }
      },
      {
        read: (dependencies: Record<string, any>) => readComposerReasoningGeometry(
          43210,
          'managed-model',
          'Reasoning',
          5000,
          dependencies
        ),
        readyValue: {
          domReady: true,
          controlCount: 1,
          control: { x: 1, y: 1 },
          controlTitle: 'managed-model / Reasoning',
          controlExpanded: false,
          expectedSelectedTitle: 'managed-model / Reasoning',
          optionCount: 0,
          option: null,
          optionChecked: null
        }
      }
    ]

    for (const candidate of readCases) {
      let targetCount = 0
      let evaluationCount = 0
      const sleepDelays: number[] = []
      const observed = await candidate.read({
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: `ws://milestone-a.invalid/devtools/page/${++targetCount}`,
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => {
          evaluationCount += 1
          return evaluationCount === 1 ? { domReady: false } : candidate.readyValue
        },
        sleep: async (delayMs: number) => {
          sleepDelays.push(delayMs)
        }
      })
      expect(observed.value).toEqual(candidate.readyValue)
      expect(targetCount).toBe(2)
      expect(evaluationCount).toBe(2)
      expect(sleepDelays).toEqual([expect.any(Number)])
    }

    let invalidParamsEvaluationCount = 0
    await expect(readComposerGeometry(43210, ['Send'], 5000, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/invalid-params',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        invalidParamsEvaluationCount += 1
        throw Object.assign(new Error('PRIVATE_INVALID_PARAMS'), {
          readonlyCdpReasonCode: 'readonly_cdp_protocol_invalid_params'
        })
      },
      sleep: async () => {
        throw new Error('non-transient readonly CDP failures must not retry')
      }
    })).rejects.toThrow('readonly_cdp_protocol_invalid_params')
    expect(invalidParamsEvaluationCount).toBe(1)
  })

  it('returns a fixed readErrorStage for every readonly composer mode DOM operation', async () => {
    const { readonlyComposerModeGeometryExpression } = await milestoneModule()
    const evaluate = (documentValue: any) => new Function(
      'document',
      `return (${readonlyComposerModeGeometryExpression()})`
    )(documentValue)
    const rect = { width: 20, height: 20, x: 10, y: 10 }
    const menuButton = { getBoundingClientRect: () => rect }
    const cases = [
      {
        stage: 'document_query',
        document: {
          querySelector: () => { throw new Error('PRIVATE_DOCUMENT_QUERY') },
          querySelectorAll: () => []
        }
      },
      {
        stage: 'dom_collection',
        document: {
          querySelector: () => null,
          querySelectorAll: () => ({
            [Symbol.iterator]: () => { throw new Error('PRIVATE_DOM_COLLECTION') }
          })
        }
      },
      {
        stage: 'button_query',
        document: {
          querySelector: () => null,
          querySelectorAll: () => [{
            querySelector: () => { throw new Error('PRIVATE_BUTTON_QUERY') }
          }]
        }
      },
      {
        stage: 'geometry',
        document: {
          querySelector: () => ({
            getBoundingClientRect: () => { throw new Error('PRIVATE_GEOMETRY') }
          }),
          querySelectorAll: () => []
        }
      },
      {
        stage: 'attribute',
        document: {
          querySelector: () => menuButton,
          querySelectorAll: () => [{
            innerText: 'Plan mode',
            getBoundingClientRect: () => rect,
            querySelector: () => ({
              getAttribute: () => { throw new Error('PRIVATE_ATTRIBUTE') }
            })
          }]
        }
      }
    ]

    for (const candidate of cases) {
      const observed = evaluate(candidate.document)
      expect(observed).toEqual({ domReady: true, readErrorStage: candidate.stage })
      expect(JSON.stringify(observed)).not.toContain('PRIVATE_')
    }
  })

  it('fails closed on a composer mode readErrorStage without retrying', async () => {
    const { readComposerModeGeometry } = await milestoneModule()
    let evaluationCount = 0
    let sleepCount = 0
    let thrown: any
    try {
      await readComposerModeGeometry(43210, 5000, {
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/read-error-stage',
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => {
          evaluationCount += 1
          return { domReady: true, readErrorStage: 'geometry' }
        },
        sleep: async () => {
          sleepCount += 1
        }
      })
    } catch (error) {
      thrown = error
    }
    expect(thrown).toEqual(expect.objectContaining({
      readonlyCdpReasonCode: 'readonly_cdp_exception',
      readonlyCdpExpressionStage: 'geometry'
    }))
    expect(String(thrown?.message)).not.toContain('PRIVATE_')
    expect(evaluationCount).toBe(1)
    expect(sleepCount).toBe(0)
  })

  it('keeps each composer mode read phase and operation in a safe diagnostic', async () => {
    const {
      selectComposerMode,
      applyComposerModeDiagnostic,
      finalizeMilestoneAReport
    } = await milestoneModule()
    const phases = [
      'initial_closed',
      'open',
      'closed_after_selection',
      'verified'
    ]
    for (const expectedPhase of phases) {
      let readCount = 0
      const result = await selectComposerMode(43210, 'plan', {
        readComposerModeGeometry: async () => {
          readCount += 1
          if (readCount === phases.indexOf(expectedPhase) + 1) {
            throw Object.assign(new Error('PRIVATE_MODE_EXCEPTION'), {
              readonlyCdpReasonCode: 'readonly_cdp_exception',
              readonlyCdpExpressionStage: 'geometry'
            })
          }
          return {
            value: {
              domReady: true,
              menuButton: { x: 1, y: 1 },
              planButton: { x: 2, y: 2 },
              planChecked: false
            },
            target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/mode' }
          }
        },
        clickCdpGeometry: async () => {},
        sleep: async () => {}
      })
      expect(result.ok).toBe(false)
      expect(result.blocker).toBe('readonly_cdp_exception')
      expect(result.diagnostic).toEqual({
        reasonCode: 'readonly_cdp_exception',
        phase: expectedPhase,
        operation: 'read',
        expressionStage: 'geometry'
      })
      expect(JSON.stringify(result)).not.toContain('PRIVATE_MODE_EXCEPTION')

      const report = { workflow: {} as Record<string, any> }
      applyComposerModeDiagnostic(report, result)
      expect(report.workflow).toEqual({
        composerModeFailureReasonCode: 'readonly_cdp_exception',
        composerModeFailurePhase: expectedPhase,
        composerModeFailureOperation: 'read',
        composerModeFailureExpressionStage: 'geometry'
      })
      expect(JSON.stringify(report)).not.toContain('PRIVATE_MODE_EXCEPTION')

      const finalized = finalizeMilestoneAReport({
        ...report,
        executionBlocker: 'readonly_cdp_exception',
        executionBlockerClassification: 'product_or_harness_failure',
        checks: [
          ...[
            'external-real-repository-contract',
            'trusted-cache-tmpdir',
            'formal-packaged-artifact',
            'isolated-profile-and-repository',
            'configured-network-provider',
            'packaged-first-launch'
          ].map((id) => ({ id, status: 'passed' })),
          {
            id: 'composer-workflow-submit',
            status: 'failed',
            message: 'readonly_cdp_exception'
          }
        ]
      }, {
        requiredCheckIds: [
          'external-real-repository-contract',
          'trusted-cache-tmpdir',
          'formal-packaged-artifact',
          'isolated-profile-and-repository',
          'configured-network-provider',
          'packaged-first-launch',
          'composer-workflow-submit'
        ]
      })
      expect(finalized.workflow).toEqual(expect.objectContaining(report.workflow))
      expect(JSON.stringify(finalized)).not.toContain('PRIVATE_MODE_EXCEPTION')
    }

    const success = await selectComposerMode(43210, 'plan', {
      readComposerModeGeometry: async () => ({
        value: {
          domReady: true,
          menuButton: { x: 1, y: 1 },
          planButton: { x: 2, y: 2 },
          planChecked: true
        },
        target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/mode-success' }
      }),
      clickCdpGeometry: async () => {},
      sleep: async () => {}
    })
    expect(success).toEqual(expect.objectContaining({
      ok: true,
      diagnostic: {
        reasonCode: '',
        phase: 'none',
        operation: 'none',
        expressionStage: 'none'
      }
    }))
  })

  it('keeps each composer mode click and sleep operation in a safe diagnostic', async () => {
    const { selectComposerMode } = await milestoneModule()
    const failures = [
      { label: 'initial-menu', clickAt: 1, sleepAt: 0, phase: 'initial_closed', operation: 'menu_click' },
      { label: 'initial-sleep', clickAt: 0, sleepAt: 1, phase: 'initial_closed', operation: 'sleep' },
      { label: 'plan-click', clickAt: 2, sleepAt: 0, phase: 'open', operation: 'plan_click' },
      { label: 'plan-sleep', clickAt: 0, sleepAt: 2, phase: 'open', operation: 'sleep' },
      {
        label: 'verification-menu',
        clickAt: 3,
        sleepAt: 0,
        phase: 'closed_after_selection',
        operation: 'menu_click'
      },
      {
        label: 'verification-sleep',
        clickAt: 0,
        sleepAt: 3,
        phase: 'closed_after_selection',
        operation: 'sleep'
      },
      { label: 'cleanup-click', clickAt: 4, sleepAt: 0, phase: 'verified', operation: 'cleanup_click' },
      { label: 'cleanup-sleep', clickAt: 0, sleepAt: 4, phase: 'verified', operation: 'sleep' }
    ]

    for (const expected of failures) {
      let readCount = 0
      let clickCount = 0
      let sleepCount = 0
      const result = await selectComposerMode(43210, 'plan', {
        readComposerModeGeometry: async () => {
          readCount += 1
          return {
            value: {
              domReady: true,
              menuButton: { x: 1, y: 1 },
              planButton: { x: 2, y: 2 },
              planChecked: readCount >= 4
            },
            target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/mode-operation' }
          }
        },
        clickCdpGeometry: async () => {
          clickCount += 1
          if (clickCount === expected.clickAt) throw new Error(`PRIVATE_${expected.label}`)
        },
        sleep: async () => {
          sleepCount += 1
          if (sleepCount === expected.sleepAt) throw new Error(`PRIVATE_${expected.label}`)
        }
      })
      expect(result).toEqual({
        ok: false,
        blocked: false,
        blocker: 'composer_mode_selection_failed',
        diagnostic: {
          reasonCode: 'composer_mode_selection_failed',
          phase: expected.phase,
          operation: expected.operation,
          expressionStage: 'none'
        }
      })
      expect(JSON.stringify(result)).not.toContain('PRIVATE_')
    }
  })

  it('projects every composer geometry DOM failure to a fixed stage without raw exception text', async () => {
    const { readonlyComposerGeometryExpression } = await milestoneModule()
    const evaluate = (documentValue: any) => new Function(
      'document',
      `return (${readonlyComposerGeometryExpression(['Send'])})`
    )(documentValue)
    const rect = { left: 0, top: 0, width: 20, height: 20 }
    const baseEditor = {
      getBoundingClientRect: () => rect,
      closest: () => ({
        querySelector: () => ({
          getBoundingClientRect: () => rect,
          getAttribute: () => 'Send',
          querySelector: () => null
        })
      }),
      innerText: '',
      textContent: ''
    }
    const cases = [
      {
        stage: 'document_query',
        document: {
          get querySelectorAll() {
            throw new Error('PRIVATE_DOCUMENT_QUERY')
          }
        }
      },
      {
        stage: 'dom_collection',
        document: {
          querySelectorAll: () => ({
            [Symbol.iterator]: () => { throw new Error('PRIVATE_DOM_COLLECTION') }
          })
        }
      },
      {
        stage: 'editor_query',
        document: {
          querySelectorAll: () => [{
            get getBoundingClientRect() {
              throw new Error('PRIVATE_EDITOR_QUERY')
            }
          }]
        }
      },
      {
        stage: 'composer_query',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => rect,
            closest: () => { throw new Error('PRIVATE_COMPOSER_QUERY') }
          }]
        }
      },
      {
        stage: 'button_query',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => rect,
            closest: () => ({ querySelector: () => { throw new Error('PRIVATE_BUTTON_QUERY') } })
          }]
        }
      },
      {
        stage: 'geometry',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => { throw new Error('PRIVATE_GEOMETRY') }
          }]
        }
      },
      {
        stage: 'attribute',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => rect,
            closest: () => ({
              querySelector: () => ({
                getBoundingClientRect: () => rect,
                getAttribute: () => { throw new Error('PRIVATE_ATTRIBUTE') }
              })
            })
          }]
        }
      },
      {
        stage: 'text',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => rect,
            closest: () => null,
            get innerText() {
              throw new Error('PRIVATE_TEXT')
            },
            textContent: ''
          }]
        }
      },
      {
        stage: 'state',
        document: {
          querySelectorAll: () => [{
            getBoundingClientRect: () => rect,
            closest: () => ({
              querySelector: () => ({
                getBoundingClientRect: () => rect,
                getAttribute: () => 'Send',
                querySelector: () => null,
                get disabled() {
                  throw new Error('PRIVATE_STATE')
                }
              })
            }),
            innerText: '',
            textContent: ''
          }]
        }
      }
    ]

    // Keep the fixture's ordinary editor shape visible to reviewers; every
    // failure case above intentionally overrides only the operation under test.
    expect(baseEditor.innerText).toBe('')
    for (const candidate of cases) {
      const observed = evaluate(candidate.document)
      expect(observed).toEqual({ domReady: true, readErrorStage: candidate.stage })
      expect(JSON.stringify(observed)).not.toContain('PRIVATE_')
    }
  })

  it('fails closed on a composer geometry readErrorStage without retrying', async () => {
    const { readComposerGeometry } = await milestoneModule()
    let evaluationCount = 0
    let sleepCount = 0
    let thrown: any
    try {
      await readComposerGeometry(43210, ['Send'], 5000, {
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/submit-read-error',
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => {
          evaluationCount += 1
          return { domReady: false, readErrorStage: 'geometry' }
        },
        sleep: async () => {
          sleepCount += 1
        }
      })
    } catch (error) {
      thrown = error
    }
    expect(thrown).toEqual(expect.objectContaining({
      readonlyCdpReasonCode: 'readonly_cdp_exception',
      readonlyCdpExpressionStage: 'geometry'
    }))
    expect(String(thrown?.message)).not.toContain('PRIVATE_')
    expect(evaluationCount).toBe(1)
    expect(sleepCount).toBe(0)
  })

  it('keeps every composer submit phase and operation in a safe diagnostic and preserves plan failure after finalize', async () => {
    const {
      cdpComposerSubmit,
      applyComposerSubmitDiagnostic,
      finalizeMilestoneAReport
    } = await milestoneModule()
    const ready = {
      value: {
        domReady: true,
        editor: { x: 1, y: 1 },
        editorTextLength: 0,
        button: { x: 2, y: 2 },
        buttonDisabled: true,
        buttonLoading: false,
        buttonLabelAccepted: true
      },
      target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/submit' }
    }
    const filled = {
      value: {
        ...ready.value,
        editorTextLength: 4,
        buttonDisabled: false
      },
      target: ready.target
    }
    const cases = [
      {
        label: 'initial-read',
        phase: 'initial_idle',
        operation: 'read',
        read: async () => {
          throw Object.assign(new Error('PRIVATE_INITIAL_READ'), {
            readonlyCdpReasonCode: 'readonly_cdp_exception',
            readonlyCdpExpressionStage: 'geometry'
          })
        },
        dispatch: async () => {},
        sleep: async () => {}
      },
      {
        label: 'initial-sleep',
        phase: 'initial_idle',
        operation: 'sleep',
        read: async () => ({ ...ready, value: { ...ready.value, editor: null } }),
        dispatch: async () => {},
        sleep: async () => { throw new Error('PRIVATE_INITIAL_SLEEP') }
      },
      {
        label: 'editor-dispatch',
        phase: 'editor_input',
        operation: 'editor_input_dispatch',
        read: async () => ready,
        dispatch: async (_url: string, commands: any[]) => {
          if (commands.some((command) => command.method === 'Input.insertText')) {
            throw new Error('PRIVATE_EDITOR_DISPATCH')
          }
        },
        sleep: async () => {}
      },
      {
        label: 'post-input-read',
        phase: 'post_input_ready',
        operation: 'read',
        read: (() => {
          let count = 0
          return async () => {
            count += 1
            return count === 1 ? ready : (() => {
              throw Object.assign(new Error('PRIVATE_POST_INPUT_READ'), {
                readonlyCdpReasonCode: 'readonly_cdp_exception',
                readonlyCdpExpressionStage: 'text'
              })
            })()
          }
        })(),
        dispatch: async () => {},
        sleep: async () => {}
      },
      {
        label: 'primary-dispatch',
        phase: 'primary_button',
        operation: 'primary_button_dispatch',
        read: (() => {
          let count = 0
          return async () => {
            count += 1
            return count === 1 ? ready : filled
          }
        })(),
        dispatch: async (_url: string, commands: any[]) => {
          if (commands.length === 2) throw new Error('PRIVATE_PRIMARY_DISPATCH')
        },
        sleep: async () => {}
      }
    ]

    for (const candidate of cases) {
      const result = await cdpComposerSubmit(43210, 'prompt', ['Send'], 5000, {
        readComposerGeometry: candidate.read,
        dispatchCdpCommands: candidate.dispatch,
        sleep: candidate.sleep
      })
      expect(result.ok).toBe(false)
      expect(result.diagnostic).toEqual(expect.objectContaining({
        phase: candidate.phase,
        operation: candidate.operation
      }))
      expect(JSON.stringify(result)).not.toContain('PRIVATE_')

      const report = { workflow: {} as Record<string, any> }
      applyComposerSubmitDiagnostic(report, result)
      const finalized = finalizeMilestoneAReport({
        ...report,
        executionBlocker: result.blocker,
        executionBlockerClassification: 'product_or_harness_failure',
        checks: [
          ...[
            'external-real-repository-contract',
            'trusted-cache-tmpdir',
            'formal-packaged-artifact',
            'isolated-profile-and-repository',
            'configured-network-provider',
            'packaged-first-launch'
          ].map((id) => ({ id, status: 'passed' })),
          { id: 'composer-workflow-submit', status: 'failed', message: result.blocker }
        ]
      }, {
        requiredCheckIds: [
          'external-real-repository-contract',
          'trusted-cache-tmpdir',
          'formal-packaged-artifact',
          'isolated-profile-and-repository',
          'configured-network-provider',
          'packaged-first-launch',
          'composer-workflow-submit'
        ]
      })
      expect(finalized.workflow).toEqual(expect.objectContaining(report.workflow))
    }
  })

  it('uses a fixed fallback for unknown composer submit exceptions and leaves success diagnostic empty', async () => {
    const { cdpComposerSubmit } = await milestoneModule()
    const unknown = await cdpComposerSubmit(43210, 'prompt', ['Send'], 5000, {
      readComposerGeometry: async () => {
        throw new Error('PRIVATE_UNKNOWN_SUBMIT_EXCEPTION')
      },
      dispatchCdpCommands: async () => {},
      sleep: async () => {}
    })
    expect(unknown).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'composer_submit_failed',
      diagnostic: {
        reasonCode: 'composer_submit_failed',
        phase: 'initial_idle',
        operation: 'read',
        expressionStage: 'none'
      }
    }))
    expect(JSON.stringify(unknown)).not.toContain('PRIVATE_UNKNOWN_SUBMIT_EXCEPTION')

    const success = await cdpComposerSubmit(43210, 'prompt', ['Send'], 5000, {
      readComposerGeometry: (() => {
        let count = 0
        return async () => {
          count += 1
          return count === 1
            ? {
                value: {
                  domReady: true,
                  editor: { x: 1, y: 1 },
                  editorTextLength: 0,
                  button: { x: 2, y: 2 },
                  buttonDisabled: true,
                  buttonLoading: false,
                  buttonLabelAccepted: true
                },
                target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/page/success' }
              }
            : {
                value: {
                  domReady: true,
                  editor: { x: 1, y: 1 },
                  editorTextLength: 4,
                  button: { x: 2, y: 2 },
                  buttonDisabled: false,
                  buttonLoading: false,
                  buttonLabelAccepted: true
                },
                target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/page/success' }
              }
        }
      })(),
      dispatchCdpCommands: async () => {},
      sleep: async () => {}
    })
    expect(success).toEqual({
      ok: true,
      blocked: false,
      blocker: '',
      diagnostic: {
        reasonCode: '',
        phase: 'none',
        operation: 'none',
        expressionStage: 'none'
      }
    })
  })

  it('retains only bounded renderer readiness diagnostics when a relaunch read fails', async () => {
    const {
      classifyLocalProviderWorkbenchReadiness,
      observeRenderer
    } = await milestoneModule()
    const privateText = 'PRIVATE_RELAUNCH_RENDERER_TEXT'
    let observedError: Record<string, any> | null = null

    try {
      await observeRenderer({
        debugPort: 43210,
        workspace: '/isolated/repository',
        exactThreadId: 'thread-relaunch',
        timeoutMs: 5000
      }, {
        waitForDebugTarget: async () => ({
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/relaunch',
          pageTargetCount: 1
        }),
        evaluateReadonlyCdp: async () => ({
          readErrorStage: 'thread_detail',
          apiPresent: true,
          composerPresent: true,
          primaryButtonPresent: true,
          account: {
            authenticated: true,
            gatewayConfigured: true,
            accountReady: true,
            userAccountReady: true,
            source: 'hub',
            email: privateText
          },
          health: null,
          runtimeInfo: null,
          runtimeTools: null,
          runtimeThreadListProbeOk: false,
          runtimeThreadListProbeStatus: 0,
          settings: {
            workspaceRoot: `/private/${privateText}`
          },
          privateText
        })
      })
    } catch (error) {
      observedError = error as Record<string, any>
    }

    expect(observedError).toEqual(expect.objectContaining({
      readonlyCdpReasonCode: 'readonly_cdp_exception',
      readonlyCdpExpressionStage: 'thread_detail',
      managedWorkbenchReadDiagnostic: {
        observed: true,
        rendererTargetCount: 1,
        readErrorStage: 'thread_detail',
        apiPresent: true,
        composerPresent: true,
        primaryButtonPresent: true,
        accountObserved: true,
        accountAuthenticated: true,
        accountGatewayConfigured: true,
        accountReady: true,
        accountUserReady: true,
        accountSourceHub: true,
        healthObserved: false,
        runtimeInfoObserved: false,
        runtimeToolsObserved: false,
        runtimeThreadListProbeOk: false,
        runtimeThreadListProbeStatus: 0
      }
    }))
    expect(JSON.stringify(observedError)).not.toContain(privateText)
    expect(classifyLocalProviderWorkbenchReadiness(
      {
        authenticated: false,
        gatewayConfigured: false,
        accountReady: false,
        userAccountReady: false,
        source: 'none'
      },
      { blocker: 'managed_hub_provider_settings_invalid', settingsCredentialFree: true },
      observedError?.managedWorkbenchReadDiagnostic
    )).toEqual({
      blocked: false,
      blocker: 'packaged_runtime_thread_recovery_not_ready'
    })
  })

  it('classifies exact packaged runtime process roles without retaining argv or paths', async () => {
    const { classifyPackagedRuntimeProcessRole } = await milestoneModule()
    const privateText = 'PRIVATE_RUNTIME_PROCESS_ARGUMENT'
    const executable = '/Volumes/AnalytixCache/Fresh Package/analytix.app/Contents/Resources/runtime-go/bin/runtime-server'
    const vectors = [
      [
        `${executable} migration migrate-desktop-private-history-v2 --data-dir /private/runtime`,
        'desktop_private_history_migration'
      ],
      [
        `${executable} --addr 127.0.0.1:43210 --durable-root /private/runtime --data-dir /private/runtime`,
        'runtime_server'
      ],
      [
        `${executable} bundled-plugin materialize-funds-v1 --data-dir /private/runtime`,
        'bundled_plugin_materialization'
      ],
      [`${executable} unknown-private-command ${privateText}`, 'unknown'],
      [`/different/runtime-server --addr 127.0.0.1:43210`, 'unknown']
    ] as const

    for (const [command, expected] of vectors) {
      const role = classifyPackagedRuntimeProcessRole(command, executable)
      expect(role).toBe(expected)
      expect(role).not.toContain('/private')
      expect(role).not.toContain(executable)
    }
  })

  it('attributes composer readiness failures to the final read instead of the preceding sleep', async () => {
    const { cdpComposerSubmit } = await milestoneModule()
    const idleGeometry = {
      value: {
        domReady: true,
        editor: { x: 1, y: 1 },
        editorTextLength: 0,
        button: { x: 2, y: 2 },
        buttonDisabled: true,
        buttonLoading: false,
        buttonLabelAccepted: true
      },
      target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/page/read-attribution' }
    }

    let now = vi.spyOn(Date, 'now').mockReturnValue(1000)
    try {
      now.mockReturnValueOnce(0).mockReturnValueOnce(0).mockReturnValueOnce(0)
      const initialIdleFailure = await cdpComposerSubmit(43210, 'prompt', ['Send'], 1000, {
        readComposerGeometry: async () => ({
          ...idleGeometry,
          value: { ...idleGeometry.value, editor: null, button: null }
        }),
        dispatchCdpCommands: async () => {},
        sleep: async () => {}
      })
      expect(initialIdleFailure).toEqual(expect.objectContaining({
        ok: false,
        blocker: 'composer_dom_element_missing',
        diagnostic: expect.objectContaining({ phase: 'initial_idle', operation: 'read' })
      }))
    } finally {
      now.mockRestore()
    }

    now = vi.spyOn(Date, 'now').mockReturnValue(10_000)
    try {
      for (let index = 0; index < 7; index += 1) now.mockReturnValueOnce(0)
      let readCount = 0
      const postInputFailure = await cdpComposerSubmit(43210, 'prompt', ['Send'], 5000, {
        readComposerGeometry: async () => {
          readCount += 1
          return readCount === 1
            ? idleGeometry
            : {
                ...idleGeometry,
                value: {
                  ...idleGeometry.value,
                  buttonDisabled: false
                }
              }
        },
        dispatchCdpCommands: async () => {},
        sleep: async () => {}
      })
      expect(postInputFailure).toEqual(expect.objectContaining({
        ok: false,
        blocker: 'composer_input_not_observed',
        diagnostic: expect.objectContaining({ phase: 'post_input_ready', operation: 'read' })
      }))
    } finally {
      now.mockRestore()
    }
  })

  it('attributes a completed verification mismatch to the verified mode read', async () => {
    const { selectComposerMode } = await milestoneModule()
    const result = await selectComposerMode(43210, 'plan', {
      readComposerModeGeometry: async () => ({
        value: {
          domReady: true,
          menuButton: { x: 1, y: 1 },
          planButton: { x: 2, y: 2 },
          planChecked: false
        },
        target: { webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/mode-mismatch' }
      }),
      clickCdpGeometry: async () => {},
      sleep: async () => {}
    })
    expect(result).toEqual({
      ok: false,
      blocked: false,
      blocker: 'composer_plan_mode_not_selected',
      diagnostic: {
        reasonCode: 'composer_plan_mode_not_selected',
        phase: 'verified',
        operation: 'read',
        expressionStage: 'none'
      }
    })
  })

  it('records composer workflow as a direct failure after passed first-launch prerequisites', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const prerequisiteIds = [
      'external-real-repository-contract',
      'trusted-cache-tmpdir',
      'formal-packaged-artifact',
      'isolated-profile-and-repository',
      'configured-network-provider',
      'packaged-first-launch'
    ]
    const report = finalizeMilestoneAReport({
      workflow: { firstLaunchObserved: true },
      executionBlocker: 'readonly_cdp_protocol_error',
      executionBlockerClassification: 'product_or_harness_failure',
      checks: [
        ...prerequisiteIds.map((id) => ({ id, status: 'passed' })),
        {
          id: 'composer-workflow-submit',
          status: 'skipped',
          message: 'readonly_cdp_protocol_error'
        }
      ]
    }, {
      requiredCheckIds: [
        ...prerequisiteIds,
        'composer-workflow-submit',
        'ordinary-agent-workflow',
        'long-context-continuation'
      ]
    })

    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({
        id: 'composer-workflow-submit',
        status: 'failed',
        message: 'readonly_cdp_protocol_error'
      }),
      expect.objectContaining({ id: 'ordinary-agent-workflow', status: 'skipped' }),
      expect.objectContaining({ id: 'long-context-continuation', status: 'skipped' })
    ]))
    expect(report.failedCheckIds).toContain('composer-workflow-submit')
    expect(report.status).toBe('failed')
    expect(report.executionBlocker).toBe('readonly_cdp_protocol_error')
  })

  it('keeps external prerequisite blocks and explicit composer failures unchanged', async () => {
    const { finalizeMilestoneAReport } = await milestoneModule()
    const prerequisiteIds = [
      'external-real-repository-contract',
      'trusted-cache-tmpdir',
      'formal-packaged-artifact',
      'isolated-profile-and-repository',
      'configured-network-provider',
      'packaged-first-launch'
    ]
    const external = finalizeMilestoneAReport({
      workflow: { firstLaunchObserved: true },
      executionBlocker: 'login_required',
      executionBlockerClassification: 'external_prerequisite',
      checks: prerequisiteIds.map((id) => ({ id, status: 'passed' }))
    }, {
      requiredCheckIds: [...prerequisiteIds, 'composer-workflow-submit', 'ordinary-agent-workflow']
    })
    expect(external.status).toBe('live_blocked')
    expect(external.failedCheckIds).toEqual([])
    expect(external.checks).toContainEqual(expect.objectContaining({
      id: 'composer-workflow-submit',
      status: 'live_blocked'
    }))

    const explicit = finalizeMilestoneAReport({
      workflow: { firstLaunchObserved: true },
      executionBlocker: 'outer_failure',
      executionBlockerClassification: 'product_or_harness_failure',
      checks: [
        ...prerequisiteIds.map((id) => ({ id, status: 'passed' })),
        {
          id: 'composer-workflow-submit',
          status: 'failed',
          message: 'existing_direct_failure'
        }
      ]
    }, {
      requiredCheckIds: [...prerequisiteIds, 'composer-workflow-submit', 'ordinary-agent-workflow']
    })
    expect(explicit.checks).toContainEqual({
      id: 'composer-workflow-submit',
      status: 'failed',
      message: 'existing_direct_failure'
    })
    expect(explicit.failedCheckIds).toEqual(['composer-workflow-submit'])
  })

  it('accepts a bounded continuationRead contract without a long-context byte or three-anchor precondition', async () => {
    const { continuationReadBindingEvidence, externalRepositoryContractShapeValid } = await milestoneModule()
    const content = Buffer.from('bounded ordinary continuation\n', 'utf8')
    const expected = {
      path: 'docs/continuation.txt',
      sha256: sha256(content),
      lineLimit: 1,
      maxBytes: 4096,
      successMarker: 'MILESTONE_A_CONTINUATION_OK'
    }
    expect(content.byteLength).toBeLessThan(64 * 1024)
    expect(continuationReadBindingEvidence({
      regular: true,
      byteLength: content.byteLength,
      sha256: sha256(content),
      content
    }, expected)).toEqual(expect.objectContaining({
      ok: true,
      lineLimit: 1,
      byteLength: content.byteLength
    }))
    const contract = {
      contract: 'analytix.milestone-a.real-repository-contract.v1',
      repository: {
        kind: 'preexisting-isolated-real-code-repository',
        baselineCommit: 'a'.repeat(40),
        baselineTree: 'b'.repeat(40),
        minimumCommitCount: 2,
        originUrlSha256: 'c'.repeat(64)
      },
      task: {
        request: 'Correct the bounded continuation implementation while preserving the API.',
        inspectPaths: ['README.md', 'src/math.mjs', 'docs/continuation.txt'],
        expectedChangedFiles: [{ path: 'src/math.mjs', sha256: 'd'.repeat(64) }]
      },
      test: { executable: 'node', arguments: ['--test', 'test/math.test.mjs'], timeoutMs: 60_000 },
      continuationRead: expected
    }
    expect(externalRepositoryContractShapeValid(contract)).toBe(true)
    expect(source('scripts/runtime-go-packaged-milestone-a.mjs'))
      .not.toContain('host-observed exact read of at least 64 KiB')
  })

  it('rejects a tested semantic equivalent when the authority has only one source hash', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    materializeClosedPlanArtifact(authority)
    const sourcePath = join(input.workspace, 'src', 'math.mjs')
    const equivalentSource =
      'export function multiply(left, right) {\n  return (left * right)\n}\n'
    writeFileSync(sourcePath, equivalentSource, 'utf8')

    const directTest = spawnSync(authority.testExecutable, authority.testArguments, {
      cwd: authority.workspace,
      encoding: 'utf8',
      env: process.env,
      stdio: 'pipe',
      timeout: authority.testTimeoutMs
    })
    expect(directTest.status).toBe(0)
    const acceptance = verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')
    expect(acceptance).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'external_repository_expected_result_mismatch',
      expectedSourceBound: false
    }))
    expect(acceptance.expectedSourceDigest)
      .not.toBe(acceptance.observedSourceDigest)
  }, 30_000)

  it('binds synthetic parent storage preflight to the same isolated home', { tags: ['macos-integration'] }, () => {
    const root = taskOwnedSandbox()
    const home = join(root, 'home')
    const temporary = join(root, 'tmp')
    const npmCache = join(root, 'npm-cache')
    for (const directory of [home, temporary, npmCache]) mkdirSync(directory, { mode: 0o700 })
    const env = { HOME: home, TMPDIR: temporary, NPM_CONFIG_CACHE: npmCache }
    const invoke = (environment: NodeJS.ProcessEnv) => spawnSync('/bin/zsh', ['-c', 'source "$1"', 'fixture', fixtureCacheHelper], {
      env: environment, encoding: 'utf8', timeout: 5_000
    })
    expect(invoke(env).status).toBe(0)
    expect(invoke({ ...env, TMPDIR: root }).status).toBe(1)
    expect(invoke({ ...env, NPM_CONFIG_CACHE: root }).status).toBe(1)
    expect(invoke({ ...env, HOME: join(root, 'missing') }).status).toBe(1)
  })

  it('accepts a legal equivalent hash and keeps the parent-owned test bound', { tags: ['macos-integration'], timeout: 30_000 }, async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      runParentOwnedRepositoryTest,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const mainSha256 = input.contract.task.expectedChangedFiles[0].sha256
    const equivalentSource =
      'export function multiply(left, right) {\n  return (left * right)\n}\n'
    const equivalentSha256 = sha256(equivalentSource)
    const contract = {
      ...input.contract,
      task: {
        ...input.contract.task,
        expectedChangedFiles: [{
          path: 'src/math.mjs',
          sha256: mainSha256,
          equivalentSha256s: [equivalentSha256]
        }]
      }
    }
    writeFileSync(input.contractPath, `${JSON.stringify(contract, null, 2)}\n`, 'utf8')
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    materializeClosedPlanArtifact(authority)
    expect(authority.expectedChangedFiles[0]).toEqual(expect.objectContaining({
      path: 'src/math.mjs',
      sha256: mainSha256,
      acceptedSha256s: [mainSha256, equivalentSha256]
    }))
    writeFileSync(join(input.workspace, 'src', 'math.mjs'), equivalentSource, 'utf8')

    expect(runParentOwnedRepositoryTest(authority)).toEqual(
      expect.objectContaining({ passed: true })
    )
    const acceptance = verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')
    expect(acceptance).toEqual(expect.objectContaining({
      ok: true,
      expectedSourceBound: true,
      expectedSourceDigest: acceptance.observedSourceDigest
    }))
  })

  it('rejects an unlisted source hash while keeping its observed digest distinct', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const mainSha256 = input.contract.task.expectedChangedFiles[0].sha256
    const listedSource =
      'export function multiply(left, right) {\n  return (left * right)\n}\n'
    const unlistedSource =
      'export function multiply(left, right) {\n  return left * (right)\n}\n'
    const contract = {
      ...input.contract,
      task: {
        ...input.contract.task,
        expectedChangedFiles: [{
          path: 'src/math.mjs',
          sha256: mainSha256,
          equivalentSha256s: [sha256(listedSource)]
        }]
      }
    }
    writeFileSync(input.contractPath, `${JSON.stringify(contract, null, 2)}\n`, 'utf8')
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    materializeClosedPlanArtifact(authority)
    writeFileSync(join(input.workspace, 'src', 'math.mjs'), unlistedSource, 'utf8')

    const acceptance = verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')
    expect(acceptance).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'external_repository_expected_result_mismatch',
      expectedSourceBound: false
    }))
    expect(acceptance.expectedSourceDigest)
      .not.toBe(acceptance.observedSourceDigest)
  }, 30_000)

  it('keeps singleton contracts and rejects malformed equivalent hash collections', async () => {
    const { externalRepositoryContractShapeValid } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const mainSha256 = input.contract.task.expectedChangedFiles[0].sha256
    expect(externalRepositoryContractShapeValid(input.contract)).toBe(true)

    const valid = (equivalentSha256s: string[]) => ({
      ...input.contract,
      task: {
        ...input.contract.task,
        expectedChangedFiles: [{
          path: 'src/math.mjs',
          sha256: mainSha256,
          equivalentSha256s
        }]
      }
    })
    expect(externalRepositoryContractShapeValid(valid(['b'.repeat(64)]))).toBe(true)
    for (const equivalentSha256s of [
      [],
      [mainSha256],
      ['b'.repeat(64), 'b'.repeat(64)],
      ['g'.repeat(64)],
      ['1'.repeat(64), '2'.repeat(64), '3'.repeat(64), '4'.repeat(64)]
    ]) {
      expect(externalRepositoryContractShapeValid(valid(equivalentSha256s))).toBe(false)
    }
  })

  it('rejects a baseline hash in any allowed equivalent collection during load', async () => {
    const { loadMilestoneAExternalRepositoryAcceptance } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const baselineSha256 = sha256(readFileSync(join(input.workspace, 'src', 'math.mjs')))
    const mainSha256 = input.contract.task.expectedChangedFiles[0].sha256
    for (const expectedChangedFile of [
      {
        path: 'src/math.mjs',
        sha256: baselineSha256,
        equivalentSha256s: ['b'.repeat(64)]
      },
      {
        path: 'src/math.mjs',
        sha256: mainSha256,
        equivalentSha256s: [baselineSha256]
      }
    ]) {
      const contract = {
        ...input.contract,
        task: {
          ...input.contract.task,
          expectedChangedFiles: [expectedChangedFile]
        }
      }
      writeFileSync(input.contractPath, `${JSON.stringify(contract, null, 2)}\n`, 'utf8')
      expect(() => loadMilestoneAExternalRepositoryAcceptance(
        input.workspace,
        input.contractPath,
        input.provenancePath,
        input.ownerRoot,
        { verificationMode: 'synthetic-parser-only' }
      )).toThrow('external_repository_expected_change_invalid')
    }
  })

  it('fails closed when an authority equivalent hash collection is tampered', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const mainSha256 = input.contract.task.expectedChangedFiles[0].sha256
    const equivalentSha256 = sha256(
      'export function multiply(left, right) {\n  return (left * right)\n}\n'
    )
    const contract = {
      ...input.contract,
      task: {
        ...input.contract.task,
        expectedChangedFiles: [{
          path: 'src/math.mjs',
          sha256: mainSha256,
          equivalentSha256s: [equivalentSha256]
        }]
      }
    }
    writeFileSync(input.contractPath, `${JSON.stringify(contract, null, 2)}\n`, 'utf8')
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    const tamperedAuthority = {
      ...authority,
      expectedChangedFiles: authority.expectedChangedFiles.map((file: Record<string, any>) => ({
        ...file,
        acceptedSha256s: [file.acceptedSha256s[0], 'c'.repeat(64)]
      }))
    }
    expect(verifyMilestoneAExternalRepositoryAcceptance(
      tamperedAuthority,
      'baseline'
    )).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'external_repository_authority_invalid'
    }))
  })

  it('binds normal local Provider authority, exact package topology, additive ordinary tools, workflow, recovery, and exit evidence', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    expect(source('scripts/lib/local-provider-acceptance.mjs')).toContain('local_provider_registry_authority_not_ready')
    const localAcceptance = source('scripts/lib/local-provider-acceptance.mjs')
    for (const guard of ["selected.id !== 'analytix-hub'", 'value.settingsFindingCount === 0',
      'value.reportFindingCount === 0', 'value.symlinkCount === 0']) expect(localAcceptance).toContain(guard)
    const settingsStart = script.indexOf('function writeIsolatedSettings')
    const settingsEnd = script.indexOf('function safeChildEnvironment', settingsStart)
    const isolatedSettings = script.slice(settingsStart, settingsEnd)

    for (const required of [
      'verifyPackagedReleasePublicationAuthority',
      'formalPackagedArtifactEvidence',
      'captureCurrentWorktreeSnapshotEvidence',
      'packagedWorktreeSnapshotBindingEvidence',
      'current_worktree_snapshot_unstable',
      'current_worktree_snapshot_missing',
      'packaged_build_authority_worktree_snapshot_missing',
      'packaged_build_authority_worktree_snapshot_mismatch',
      'worktreeSnapshotBinding.ok',
      'readLocalCredentialScanSource',
      'LIVE CHECKPOINT: complete visible protected local Provider onboarding normally.',
      "kind: 'visible-normal-local-provider-setup'",
      "'--credential-entry-method'",
      "method === 'visible-automation'",
      'const configuredNetworkProviderPassed =',
      'exactThreadId: expectedThreadId',
      'previousTurnCount: planBaselineEvidence.turnCount',
      'expectedThreadId: planBaselineEvidence.threadId',
      'previousTurnCount: agentBaselineEvidence.turnCount',
      'previousTurnCount: firstEvidence.turnCount',
      'previousTurnCount: recoveredEvidence.turnCount',
      'const exactPlanTurnBound = Boolean(planReceiptScope &&',
      'planEvidence.resultTurnId === planReceiptScope.expectedTurnId',
      'report.workflow.planModeSelected = planMode.ok',
      'report.workflow.planTurnObserved = planTurnPassed',
      'report.workflow.planThreadIdHash = planThreadIdHash',
      'report.workflow.planProviderReceiptTrace = planProviderReceiptTrace',
      "'packaged_plan_provider_receipt_failed'",
      "ordinaryDisposition === 'completed'",
      "contextWorkflow.disposition === 'completed'",
      "relaunchContinuationDisposition === 'completed'",
      'milestoneALocalProvider',
      'freshLocalProviderRegistry',
      'LOCAL_PROVIDER_AUTHORITY',
      'ANALYTIX_USER_DATA_DIR',
      'createIsolatedDarwinLoginKeychain',
      'isolatedLoginKeychain.unlockForLaunch()',
      'process-memory-to-stdin-to-task-owned-pty',
      'isolatedLoginKeychainReusedForTwoLaunches',
      'runtimeDataDir',
      'writeWorkspace',
      'scheduleWorkspace',
      'clawWorkspace',
      'MILESTONE_A_REAL_TEST_EXECUTED',
      "'--repository-path'",
      "'--repository-contract'",
      "'--repository-provenance'",
      "'--repository-owner-root'",
      'EXTERNAL_REPOSITORY_CONTRACT',
      'external_repository_path_contract_provenance_and_owner_root_required',
      'EXTERNAL_REPOSITORY_PROVENANCE_CONTRACT',
      'verifyFreshExternalRepositoryOrigin',
      'external_repository_origin_fetch_failed',
      'freshOriginFetchAttemptCount',
      'freshOriginFetchExitCodes',
      'external_repository_git_common_dir_invalid',
      'external_repository_link_isolation_invalid',
      'external_repository_fixture_substitute_rejected',
      'loadMilestoneAExternalRepositoryAcceptance',
      'verifyMilestoneAExternalRepositoryAcceptance',
      'createMilestoneARepositoryAcceptance',
      'verifyMilestoneARepositoryAcceptance',
      'isolated_repository_git_initialization_failed',
      "'status', '--porcelain=v1', '-z', '--untracked-files=all'",
      'protectedInputsWriteProtected',
      'validatorOutsideRepository',
      'host_owned_tool_invocation_binding_unavailable',
      'hostToolExecutionObservationEvidence',
      'privateHostToolExecutionBindingMatches',
      'hostToolExecutionPrivateBindings',
      "'/v1/runtime/tool-executions/observe'",
      'runParentOwnedRepositoryTest',
      'parent-owned-repository-test-cross-check',
      "command: 'contract-bound-test-command'",
      'arguments: { command: repositoryAuthority.testCommand }',
      'substitutesForAgentInvocationBinding: false',
      'npmTestAncestorObserved',
      'packageScriptParentObserved',
      'actualBashToolResultBound',
      'bashHostObservationDigest',
      'bashHostAuthorityBindingDigest',
      'selfReportedReceiptAuthoritative: false',
      'packagedArtifactRevalidationEvidence',
      "'before-first-launch'",
      "'after-first-exit'",
      "'before-second-launch'",
      "'after-final-exit'",
      'MILESTONE_A_RESULT_OK',
      'MILESTONE_A_PLAN_OK',
      'MILESTONE_A_CONTEXT_OK_',
      'MILESTONE_A_CONTEXT_FIRST_',
      'MILESTONE_A_CONTEXT_LAST_',
      'executionPolicyVersion: 2',
      "sandboxMode: 'danger-full-access'",
      "info?.executionPolicy?.sandboxMode === 'danger-full-access'",
      "runtimeJSON('/v1/runtime/info')",
      "runtimeJSON('/v1/runtime/tools')",
      'pageTargets.length === 1',
      'runtimeBackendTopologyEvidence',
      'runtimeListenerPid: listenerPid',
      'listenerIsTaskOwnedDescendant',
      'runtimeBackendProcessCount: exactRuntimeBackendPids.length',
      "spawnSync('/usr/sbin/lsof'",
      'sameFileIdentity(candidateStat, expectedStat)',
      'runtimeServerPath(appPath, target)',
      'ordinaryCatalogNonempty',
      'fundsTransportAvailable',
      'fundsExecutionUnavailable',
      'protectedFundsUnavailableTurnEvidence',
      'protectedFundsSourceUnavailable',
      'protectedFundsSuccessfulExecutionCount',
      'ordinaryContinuedAfterProtectedFundsBlock',
      'ordinaryContinuedAfterRestart',
      'privateCaseAuthorityInputEvidence',
      'case-dependencies-unavailable-with-ordinary-capabilities',
      "thread?.todos?.items",
      "item.output?.status !== 'completed'",
      "item.status === 'done'",
      "item.rawStatus === 'completed'",
      "item.diagnostics?.terminal === true",
      "item.profile === PACKAGED_READ_ONLY_SUBAGENT_PROFILE",
      "item.toolPolicy === 'readOnly'",
      "providerAttemptTelemetrySchema === 'provider-attempt-telemetry.v1'",
      'resultTurnProviderReceiptBound',
      'first-completed.png',
      'recovered-after-relaunch.png',
      'screenshotsUsedAsFunctionalVerdict: false',
      "selectComposerMode(firstDebugPort, 'plan')",
      "selectComposerMode(firstDebugPort, 'agent')",
      'Use no tools other than those exact read calls and one create_plan call.',
      'Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.',
      'Call create_plan exactly once after those reads complete.',
      'After create_plan returns successfully, end the turn immediately without another tool call.',
      'milestoneAPlanReadSetEvidence',
      'planTaskInspectionReadsBound',
      'planLongContextReadAbsent',
      'milestoneAPlanFeatureName',
      'milestoneAPlanArtifactEvidence',
      'planArtifactExcludeBeforeSha256',
      'planArtifactExcludeSha256',
      'planArtifactResultItemDigest',
      'planArtifactRevisionSequenceDigest',
      'planArtifactFileIdentityDigest',
      'planArtifactCallCount',
      'planArtifactResultCount',
      'exactPlanArtifactRecovered',
      'recoveredPlanArtifact.revisionSequenceDigest === planArtifact.revisionSequenceDigest',
      'recoveredPlanArtifact.planCallCount === planArtifact.planCallCount',
      'recoveredPlanArtifact.planResultCount === planArtifact.planResultCount',
      'finalPlanArtifact.revisionSequenceDigest === planArtifact.revisionSequenceDigest',
      'finalPlanArtifact.planCallCount === planArtifact.planCallCount',
      'finalPlanArtifact.planResultCount === planArtifact.planResultCount',
      'planProviderReceiptBound',
      'childSubagentProviderReceiptBound',
      'milestoneAChildTaskArguments',
      'milestoneAChildReviewReadSetEvidence',
      'childSubagentReviewReadsBound',
      'childSubagentReviewReadObservationDigest',
      'relaunch-provider-continuation',
      'relaunchProviderContinuationObserved',
      'git-skill-mcp-research-writing',
      'gitHostInvocationBound',
      'skillCatalogAvailable',
      'ordinaryMCPAvailable',
      'ordinaryMCPServerId',
      'PACKAGED_ORDINARY_MCP_SERVER_ID',
      'mcp__gui_schedule__gui_schedule_list',
      'packagedMilestoneASkillID',
      'PACKAGED_READ_ONLY_SUBAGENT_PROFILE',
      'packagedSkillSourceBindingEvidence',
      'packagedSkillSourceMatched',
      'sourceSkillSha256',
      'packagedSkillSha256',
      'packagedScheduleSourceContractBound',
      'packagedScheduleStrictEmptyInputSchemaBound',
      'packagedResultTurnContractEvidence',
      'CACHE_HELPER_RUNTIME_SOURCE_CLOSURE',
      'cacheHelperCommandPrefix',
      'harnessContractManifestBound',
      'harnessSourceClosureBound',
      'contractManifestBound',
      'harnessSourceRootBound',
      'sourceClosureBound',
      'packagedSourceClosureSnapshotDigest',
      'MILESTONE_A_RESEARCH_OK',
      'MILESTONE_A_WRITING_OK',
      'Use a thread Todo list to execute the recorded plan.',
      'Use run_skill exactly once with the packaged Skill',
      'Use task exactly once as the only child subagent',
      'Pass exactly this JSON object as its arguments',
      "profile: PACKAGED_READ_ONLY_SUBAGENT_PROFILE",
      "toolPolicy: 'readOnly'",
      'run_in_background: false',
      'run_skill is not delegation',
      'ordinaryMCPStrictEmptyInputBound',
      'ordinaryMCPPackageCatalogBound',
      'ordinaryMCPResultTurnAttemptCount',
      'resultTurnTaskAttemptCount',
      'resultTurnTaskExecutionCount',
      'resultTurnSubagentDelegationAttemptCount',
      'resultTurnRunSkillAttemptCount',
      'resultTurnRunSkillExecutionCount',
      'resultTurnBashAttemptCount',
      'resultTurnBashExecutionCount',
      'resultTurnFundsMCPAttemptCount',
      'resultTurnMalformedToolCallAttemptCount',
      'export function subagentContinuityProjection',
      'export function subagentContinuityDigest',
      'export function exactlyOneSubagentContinuityMatches',
      'subagentContinuityDigest: subagentContinuityDigest(subagents)',
      'longContextSubagentContinuityBound',
      'longContextSubagentContinuityBaselineDigest',
      'longContextSubagentContinuityProtectedFundsDigest',
      'longContextSubagentContinuityDigest',
      'exactlyOneSubagentContinuityMatches(firstEvidence, contextEvidence)',
      'longContextContinuationObserved',
      'longContextCandidateTurnErrorCode',
      'longContextCandidateTerminalReasonClass',
      'longContextCandidateInventoryDigest',
      'longContextProviderFailureObserved',
      'longContextProviderFailureObservationCode',
      'longContextProviderFailureReasonCode',
      'longContextProviderFailureDiagnosticDigest',
      'longContextProviderReceiptBound',
      'longContextAcceptedFinalBound',
      'longContextAcceptedFinalDigest',
      'longContextProviderClosureDigest',
      'longContextHostReadBound',
      'longContextHostObservationDigest',
      'longContextPublicToolInventoryAvailability',
      'longContextHostObservationTransportStatus',
      'acceptedFinalOrdinaryContinuationEvidence',
      'relaunchAcceptedFinalBound',
      'relaunchAcceptedFinalDigest',
      'relaunchProviderClosureDigest',
      'arguments: contextReadArguments',
      'external_repository_formal_continuation_read_required',
      "cdpComposerSubmit(firstDebugPort, '/compact'",
      'exactThreadRecovered',
      'exactTodosRecovered',
      'exactSubagentsRecovered',
      'recoveredEvidence.subagentsDigest === compactedEvidence.subagentsDigest',
      'exactCompactionsRecovered',
      'exactResultRecovered',
      'rendererVisibleRecoveryEvidence',
      '[data-analytix-message-timeline-scroller]',
      '.ds-right-sidebar-pane[data-open="true"]',
      '[data-summary-panel][data-visible="true"]',
      '[data-subagent-inspector-panel]',
      'renderer-visible-thread-recovery',
      'renderer-visible-todo-recovery',
      'renderer-visible-subagent-recovery',
      'renderer-visible-compaction-recovery',
      'renderer-visible-result-recovery',
      'firstChild.exitCode === 0',
      'firstChild.signalCode === null',
      'secondChild.exitCode === 0',
      'secondChild.signalCode === null',
      'zero-residual-processes',
      'scanLocalCredentialIsolation',
      'localProviderSecretStoreEvidence',
      'credentialScan.ok && finalSecretStore.ok',
      'scanLocalCredentialIsolation',
      'disposeLocalCredentialScanSource',
      'credentialAuthorityBound',
      'contractManifestSha256'
    ]) {
      expect(script).toContain(required)
    }
    const continuityGateStart = script.indexOf('const longContextSubagentContinuityBound')
    const continuityGateEnd = script.indexOf('const postContextRepositoryAcceptance', continuityGateStart)
    expect(continuityGateStart).toBeGreaterThanOrEqual(0)
    expect(continuityGateEnd).toBeGreaterThan(continuityGateStart)
    expect(script.slice(continuityGateStart, continuityGateEnd)).toContain(
      'exactlyOneSubagentContinuityMatches'
    )
    expect(script.slice(continuityGateStart, continuityGateEnd)).not.toContain(
      'subagentsDigest ==='
    )
    const exactRecoveryStart = script.indexOf('const exactSubagentsRecovered')
    expect(script.slice(exactRecoveryStart, exactRecoveryStart + 256)).toContain(
      'recoveredEvidence.subagentsDigest === compactedEvidence.subagentsDigest'
    )
    expect(isolatedSettings).toContain("activeProviderId: ''")
    expect(isolatedSettings).toContain("providers: []")
    expect(isolatedSettings).toContain('extraDirs: [packagedSkillRoot]')
    expect(isolatedSettings).toContain("defaultToolPolicy: 'readOnly'")
    expect(isolatedSettings).toContain("toolPolicy: 'readOnly'")
    expect(isolatedSettings).toContain('computerUse: {')
    expect(isolatedSettings).toContain('enabled: false')
    expect(isolatedSettings).toContain("mode: 'off'")
    expect(isolatedSettings).toContain('disabledDirs: [...ISOLATED_DISABLED_SKILL_DIRS]')
    expect(isolatedSettings).not.toContain('credential')
    expect(script).not.toContain('ANALYTIX_RUNTIME_GO_MILESTONE_A_PROVIDER_CREDENTIAL')
    expect(script).not.toContain('ANALYTIX_DESKTOP_AUTH_TEST_BOOTSTRAP')
    expect(script).not.toContain('ANALYTIX_HUB_ACCOUNT_TEST_BOOTSTRAP')
    expect(script).not.toContain('api.account.login(')
    expect(script).not.toContain('protectedFundsAbsent')
    expect(script).not.toContain("endpointFormat: 'local-contract'")
    expect(script).not.toContain('syntheticProviderUsed: true')
    expect(script).not.toContain('localProviderUsed: true')
    expect(script).not.toContain(
      'source /Users/sun/Projects/analytix/scripts/use-analytix-cache.sh'
    )
    expect(script).toContain('artifact.ok && report.harness.sourceClosureBound')
    expect(source('scripts/runtime-go-packaged-qa.mjs')).toContain(
      'milestoneAHarnessEntriesExactlyBound'
    )
    expect(source('scripts/runtime-go-packaged-qa.mjs')).toContain(
      "['scripts/use-analytix-cache.sh', resolve(process.cwd(), 'scripts/use-analytix-cache.sh')]"
    )
    expect(source('scripts/runtime-go-packaged-qa.mjs')).toContain(
      "['scripts/analytix-cache-storage.zsh', resolve(process.cwd(), 'scripts/analytix-cache-storage.zsh')]"
    )
    const fixtureStart = script.indexOf('function writeFixtureRepository')
    const fixtureEnd = script.indexOf('function gitCapture', fixtureStart)
    const fixture = script.slice(fixtureStart, fixtureEnd)
    expect(fixture).toContain('assert.equal(add(2, 3), 5)')
    expect(fixture).not.toContain('appendFileSync')
    expect(script).toContain("const FIXTURE_TEST_SCRIPT = 'node --test test/counter.test.mjs'")
    expect(script).not.toContain('ANALYTIX_MILESTONE_A_VALIDATOR_PATH: validatorPath')
    const formalRunStart = script.indexOf('export async function runMilestoneA(')
    const formalRun = script.slice(
      formalRunStart,
      script.indexOf('async function main()', formalRunStart)
    )
    expect(formalRun).toContain('repositoryInput.authority')
    expect(formalRun).not.toContain('createMilestoneARepositoryAcceptance(')
  })

  it('binds the normalized desktop execution policy to the production runtime-server argv', () => {
    const adapter = source('src/main/runtime/analytix-adapter.ts')
    const launchStart = adapter.indexOf('async function startGoConformanceSidecarOnce')
    const launchEnd = adapter.indexOf('export function buildGoRuntimeProviderArgs', launchStart)
    const launch = adapter.slice(launchStart, launchEnd)
    const builderStart = adapter.indexOf('export function buildGoRuntimeProviderArgs')
    const builderEnd = adapter.indexOf('export function buildGoRuntimeSidecarEnv', builderStart)
    const builder = adapter.slice(builderStart, builderEnd)

    expect(launchStart).toBeGreaterThanOrEqual(0)
    expect(builderStart).toBeGreaterThan(launchStart)
    expect(launch).toContain('args.push(...buildGoRuntimeProviderArgs(settings, runtime))')
    expect(builder).toContain("'--approval-policy'")
    expect(builder).toContain('runtime.approvalPolicy')
    expect(builder).toContain("'--sandbox-mode'")
    expect(builder).toContain('runtime.sandboxMode')
    expect(builder).not.toContain('runtime.apiKey')
    expect(builder).not.toContain('runtime.runtimeToken')
  })

  it('accepts an exact current-to-packaged worktree snapshot digest match', async () => {
    const { packagedWorktreeSnapshotBindingEvidence } = await milestoneModule()
    const evidence = packagedWorktreeSnapshotBindingEvidence(currentSnapshot, {
      classification: 'development_dirty_non_publishable',
      worktreeSnapshot: {
        snapshotDigest: currentSnapshot.digest,
        state: currentSnapshot.classification
      }
    })

    expect(evidence).toEqual({
      ok: true,
      blocker: '',
      matched: true,
      current: {
        digest: currentSnapshot.digest,
        classification: 'dirty'
      },
      packaged: {
        digest: currentSnapshot.digest,
        classification: 'dirty',
        authorityClassification: 'development_dirty_non_publishable'
      }
    })
  })

  it('fails closed when the packaged worktree snapshot digest mismatches', async () => {
    const { packagedWorktreeSnapshotBindingEvidence } = await milestoneModule()
    const evidence = packagedWorktreeSnapshotBindingEvidence(currentSnapshot, {
      classification: 'development_dirty_non_publishable',
      worktreeSnapshot: {
        snapshotDigest: 'b'.repeat(64),
        state: 'dirty'
      }
    })

    expect(evidence).toEqual(expect.objectContaining({
      ok: false,
      matched: false,
      blocker: 'packaged_build_authority_worktree_snapshot_mismatch',
      current: expect.objectContaining({ digest: 'a'.repeat(64), classification: 'dirty' }),
      packaged: expect.objectContaining({ digest: 'b'.repeat(64), classification: 'dirty' })
    }))
  })

  it('fails closed when the fresh current worktree snapshot is unstable', async () => {
    const {
      captureCurrentWorktreeSnapshotEvidence,
      packagedWorktreeSnapshotBindingEvidence
    } = await milestoneModule()
    const current = captureCurrentWorktreeSnapshotEvidence({
      repoRoot: process.cwd(),
      collectSnapshot: () => {
        throw new Error('[after-pack] Worktree changed while the package snapshot was captured')
      }
    })
    const evidence = packagedWorktreeSnapshotBindingEvidence(current, {
      classification: 'development_dirty_non_publishable',
      worktreeSnapshot: { snapshotDigest: 'a'.repeat(64), state: 'dirty' }
    })

    expect(current).toEqual(expect.objectContaining({
      ok: false,
      blocked: true,
      blocker: 'current_worktree_snapshot_unstable',
      digest: '',
      classification: 'unstable'
    }))
    expect(evidence).toEqual(expect.objectContaining({
      ok: false,
      matched: false,
      blocker: 'current_worktree_snapshot_unstable',
      current: { digest: '', classification: 'unstable' },
      packaged: expect.objectContaining({
        digest: 'a'.repeat(64),
        classification: 'dirty'
      })
    }))
  })

  it('fails closed when either current or packaged worktree snapshot is missing', async () => {
    const {
      captureCurrentWorktreeSnapshotEvidence,
      packagedWorktreeSnapshotBindingEvidence
    } = await milestoneModule()
    const missingCurrent = captureCurrentWorktreeSnapshotEvidence({
      repoRoot: process.cwd(),
      collectSnapshot: () => null,
      validateSnapshot: () => false
    })

    expect(missingCurrent).toEqual(expect.objectContaining({
      ok: false,
      blocked: true,
      blocker: 'current_worktree_snapshot_missing',
      digest: '',
      classification: 'missing'
    }))
    expect(packagedWorktreeSnapshotBindingEvidence(currentSnapshot, {})).toEqual(
      expect.objectContaining({
        ok: false,
        matched: false,
        blocker: 'packaged_build_authority_worktree_snapshot_missing',
        current: { digest: currentSnapshot.digest, classification: 'dirty' },
        packaged: {
          digest: '',
          classification: 'missing',
          authorityClassification: 'missing'
        }
      })
    )
  })

  it('binds the helper runtime source closure to the exact harness manifest and commit', async () => {
    const {
      harnessContractManifestBound,
      harnessSourceClosureBound
    } = await milestoneModule()
    const entries = [
      {
        name: 'scripts/use-analytix-cache.sh',
        regular: true,
        byteLength: 10,
        sha256: '1'.repeat(64)
      },
      {
        name: 'scripts/analytix-cache-storage.zsh',
        regular: true,
        byteLength: 10,
        sha256: '2'.repeat(64)
      }
    ]
    const sourceCommit = '3'.repeat(40)
    const manifest = {
      entries,
      harnessSourceRootBound: true,
      contractManifestBound: true,
      contractManifestSha256: sha256(canonicalJSON(entries))
    }
    const artifact = {
      sourceCommit,
      worktreeSnapshotBinding: {
        ok: true,
        matched: true
      }
    }

    expect(harnessContractManifestBound(entries, true)).toBe(true)
    expect(harnessSourceClosureBound(manifest, artifact, sourceCommit)).toBe(true)

    const invalidManifestMutations = [
      { ...manifest, entries: entries.slice(1) },
      {
        ...manifest,
        entries: entries.map((entry, index) => index === 0
          ? { ...entry, regular: false }
          : entry)
      },
      {
        ...manifest,
        entries: entries.map((entry, index) => index === 0
          ? { ...entry, sha256: '' }
          : entry)
      },
      { ...manifest, harnessSourceRootBound: false },
      { ...manifest, contractManifestBound: false },
      { ...manifest, contractManifestSha256: '4'.repeat(64) }
    ]
    for (const invalidManifest of invalidManifestMutations) {
      expect(harnessSourceClosureBound(invalidManifest, artifact, sourceCommit)).toBe(false)
    }
    expect(harnessSourceClosureBound(manifest, {
      ...artifact,
      worktreeSnapshotBinding: { ok: false, matched: true }
    }, sourceCommit)).toBe(false)
    expect(harnessSourceClosureBound(manifest, {
      ...artifact,
      worktreeSnapshotBinding: { ok: true, matched: false }
    }, sourceCommit)).toBe(false)
    expect(harnessSourceClosureBound(manifest, artifact, '5'.repeat(40))).toBe(false)
    expect(harnessSourceClosureBound(manifest, { ...artifact, sourceCommit: '' }, '')).toBe(false)
  })

  it('requires artifact, worktree, and codesign identity to remain stable at every revalidation', async () => {
    const { packagedArtifactRevalidationEvidence } = await milestoneModule()
    const baseline = {
      ok: true,
      blocker: '',
      appAsarSha256: '1'.repeat(64),
      authorityDigest: '2'.repeat(64),
      authoritySha256: '3'.repeat(64),
      codeSignatureVerified: true,
      executableSha256: '4'.repeat(64),
      runtimeServerSha256: '5'.repeat(64),
      sourceCommit: '6'.repeat(40),
      targetKey: 'darwin-arm64',
      worktreeSnapshotBinding: {
        ok: true,
        current: { digest: '7'.repeat(64) },
        packaged: { digest: '7'.repeat(64) }
      }
    }

    expect(packagedArtifactRevalidationEvidence(
      baseline,
      structuredClone(baseline),
      'before-first-launch'
    )).toEqual(expect.objectContaining({
      ok: true,
      codeSignatureVerified: true,
      stableFields: true,
      worktreeStable: true
    }))
    expect(packagedArtifactRevalidationEvidence(
      baseline,
      { ...structuredClone(baseline), appAsarSha256: '8'.repeat(64) },
      'after-first-exit'
    )).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'formal_package_artifact_changed_between_revalidations',
      stableFields: false
    }))
    expect(packagedArtifactRevalidationEvidence(
      baseline,
      { ...structuredClone(baseline), codeSignatureVerified: false },
      'before-second-launch'
    )).toEqual(expect.objectContaining({ ok: false, codeSignatureVerified: false }))
  })

  it('fails closed when the matching same-process afterExtract snapshot is missing', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'missing-prebuild-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const context = packageLifecycleContext(root)

    expect(() => contract.consumePackagedAfterExtractSnapshot(context, workspace)).toThrow(
      /Required same-process afterExtract snapshot is missing/
    )
  })

  it('consumes the afterExtract snapshot before rejecting a source mismatch and replay', async () => {
    const contract = await packagedAuthorityInternals()
    const afterExtract = await afterExtractInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'mismatched-prebuild-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const context = packageLifecycleContext(root)
    afterExtract.captureAfterExtractSnapshot(context, { repoRoot: workspace })
    writeFileSync(join(workspace, 'untracked.svg'), '<svg/>\n', 'utf8')

    expect(() => contract.consumePackagedAfterExtractSnapshot(context, workspace)).toThrow(
      /Source inputs changed between afterExtract and afterPack entry/
    )
    expect(() => contract.consumePackagedAfterExtractSnapshot(context, workspace)).toThrow(
      /AfterExtract snapshot replay detected/
    )
  })

  it('keeps concurrent afterExtract snapshots isolated by exact target key', async () => {
    const contract = await packagedAuthorityInternals()
    const afterExtract = await afterExtractInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'concurrent-target-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const arm64Context = packageLifecycleContext(root, 'arm64')
    const x64Context = packageLifecycleContext(root, 'x64')
    const arm64 = afterExtract.captureAfterExtractSnapshot(arm64Context, { repoRoot: workspace })
    const x64 = afterExtract.captureAfterExtractSnapshot(x64Context, { repoRoot: workspace })

    expect(x64.targetKey).toBe('darwin-x64')
    expect(arm64.targetKey).toBe('darwin-arm64')
    expect(contract.consumePackagedAfterExtractSnapshot(x64Context, workspace).lifecycle)
      .toEqual(x64)
    expect(contract.consumePackagedAfterExtractSnapshot(arm64Context, workspace).lifecycle)
      .toEqual(arm64)
  })

  it('rejects prepackaged input and dirty formal release source at afterExtract', async () => {
    const afterExtract = await afterExtractInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'after-extract-release-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])

    const prepackaged = packageLifecycleContext(root)
    prepackaged.packager.packagerOptions.prepackaged = join(root, 'prepackaged')
    expect(() => afterExtract.captureAfterExtractSnapshot(prepackaged, {
      repoRoot: workspace
    })).toThrow(/Prepackaged electron-builder input is not eligible/)

    writeFileSync(join(workspace, 'untracked.txt'), 'dirty\n', 'utf8')
    expect(() => afterExtract.captureAfterExtractSnapshot(packageLifecycleContext(root), {
      repoRoot: workspace,
      env: { ANALYTIX_RELEASE_BUILD: '1' }
    })).toThrow(/Formal packaged release requires a clean Git worktree/)
  })

  it('binds every non-ignored untracked regular file regardless of extension', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'snapshot-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const clean = contract.collectPackagedWorktreeSnapshotV1(workspace)
    writeFileSync(join(workspace, 'logo.svg'), '<svg/>\n', 'utf8')
    const svg = contract.collectPackagedWorktreeSnapshotV1(workspace)
    writeFileSync(join(workspace, 'LICENSE.local'), 'extensionless-ish\n', 'utf8')
    writeFileSync(join(workspace, 'NOTICE'), 'no extension\n', 'utf8')
    const extensionless = contract.collectPackagedWorktreeSnapshotV1(workspace)

    expect(svg.untrackedSource.count).toBe(1)
    expect(svg.snapshotDigest).not.toBe(clean.snapshotDigest)
    expect(extensionless.untrackedSource.count).toBe(3)
    expect(extensionless.snapshotDigest).not.toBe(svg.snapshotDigest)
  })

  it('does not filter tracked changes by generated-looking directory names', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'tracked-generated-looking-repository')
    mkdirSync(join(workspace, 'dist'), { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'dist', 'logo.svg'), '<svg>v1</svg>\n', 'utf8')
    runGit(workspace, ['add', 'dist/logo.svg'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const before = contract.collectPackagedWorktreeSnapshotV1(workspace)
    writeFileSync(join(workspace, 'dist', 'logo.svg'), '<svg>v2</svg>\n', 'utf8')
    const after = contract.collectPackagedWorktreeSnapshotV1(workspace)

    expect(after.unstagedPatch.count).toBe(1)
    expect(after.snapshotDigest).not.toBe(before.snapshotDigest)
  })

  it('fails closed on an untracked symlink instead of hashing its target', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'snapshot-symlink-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, 'base.txt'), 'tracked\n', 'utf8')
    runGit(workspace, ['add', 'base.txt'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    symlinkSync('base.txt', join(workspace, 'untracked.svg'))

    expect(() => contract.collectPackagedWorktreeSnapshotV1(workspace)).toThrow(
      /Untrusted regular file identity/
    )
  })

  it('keeps ignored dependency and build output outside the Git source closure', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'snapshot-git-closure-repository')
    mkdirSync(workspace, { recursive: true })
    runGit(workspace, ['init', '--quiet'])
    writeFileSync(join(workspace, '.gitignore'), 'out/\nnode_modules/\n', 'utf8')
    writeFileSync(join(workspace, 'package.json'), '{"name":"closure-fixture"}\n', 'utf8')
    runGit(workspace, ['add', '.gitignore', 'package.json'])
    runGit(workspace, [
      '-c', 'user.name=Analytix Test',
      '-c', 'user.email=test@analytix.invalid',
      'commit', '--quiet', '-m', 'fixture'
    ])
    const before = contract.collectPackagedWorktreeSnapshotV1(workspace)
    mkdirSync(join(workspace, 'out'), { recursive: true })
    mkdirSync(join(workspace, 'node_modules', 'native-addon'), { recursive: true })
    writeFileSync(join(workspace, 'out', 'addon.node'), 'native-bytes-v1', 'utf8')
    writeFileSync(
      join(workspace, 'node_modules', 'native-addon', 'implicit.node'),
      'implicit-native-bytes-v1',
      'utf8'
    )
    const after = contract.collectPackagedWorktreeSnapshotV1(workspace)

    expect(after.untrackedSource.count).toBe(0)
    expect(after.snapshotDigest).toBe(before.snapshotDigest)
  })

  it('binds the effective builder config and exact target without hashing secret values', async () => {
    const contract = await packagedAuthorityInternals()
    const root = taskOwnedSandbox()
    const first = contract.collectEffectiveBuilderContextV1(packageLifecycleContext(
      root,
      'arm64',
      { asar: true, cscKeyPassword: 'first-secret' }
    ))
    const redactedEquivalent = contract.collectEffectiveBuilderContextV1(packageLifecycleContext(
      root,
      'arm64',
      { asar: true, cscKeyPassword: 'second-secret' }
    ))
    const changed = contract.collectEffectiveBuilderContextV1(packageLifecycleContext(
      root,
      'x64',
      { asar: false, cscKeyPassword: 'second-secret' }
    ))

    expect(first.contextDigest).toBe(redactedEquivalent.contextDigest)
    expect(first.effectiveConfigSha256).toBe(redactedEquivalent.effectiveConfigSha256)
    expect(changed.contextDigest).not.toBe(first.contextDigest)
    expect(changed.target).toEqual(expect.objectContaining({
      key: 'darwin-x64',
      platform: 'darwin',
      arch: 'x64',
      digest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(() => contract.collectEffectiveBuilderContextV1(packageLifecycleContext(
      root,
      'arm64',
      { electronFuses: { runAsNode: true } }
    ))).toThrow(/Automatic electron-builder fuse mutation must be disabled/)
  })

  it('derives the reserved packaged plan path with the production renderer algorithm', async () => {
    const {
      milestoneAChildReviewPaths,
      milestoneAChildReviewReadSetEvidence,
      milestoneAChildTaskArguments,
      milestoneAWorkflowPrompt,
      milestoneAFormalExecutionBounds,
      milestoneAPlanFeatureName,
      milestoneAPlanInspectPaths,
      milestoneAPlanPrompt,
      milestoneAPlanReadSetEvidence,
      milestoneAPlanRelativePath
    } = await milestoneModule()
    const authority = {
      taskRequest: 'Correct the multiplication implementation while preserving the public API.',
      inspectPaths: ['README.md', 'src/math.mjs', 'test/math.test.mjs'],
      expectedChangedFiles: [{ path: 'src/math.mjs' }],
      testCommand: 'node --test test/math.test.mjs'
    }
    const prompt = milestoneAPlanPrompt(authority)
    expect(prompt).toContain(
      'Use no tools other than those exact read calls and one create_plan call.'
    )
    expect(prompt).toContain(
      'Do not call list, search, code index, web fetch, bash, Todo, subagent/delegation, skill, MCP, or read any other path.'
    )
    expect(prompt).toContain(
      'Do not retry or duplicate a read call; if one fails, end the turn and report that failure without another tool call.'
    )
    expect(prompt).toContain(
      'Call create_plan exactly once after those reads complete.'
    )
    expect(prompt).toContain(
      'After create_plan returns successfully, end the turn immediately without another tool call.'
    )
    expect(milestoneAPlanFeatureName(prompt)).toBe(planFeatureNameFromRequest(prompt))
    expect(milestoneAPlanRelativePath(authority)).toBe(
      buildPlanRelativePath(planFeatureNameFromRequest(prompt))
    )

    const externalAuthority = {
      ...authority,
      inspectPaths: [...authority.inspectPaths, 'logo/glob.svg'],
      context: { path: 'logo/glob.svg' }
    }
    expect(milestoneAPlanInspectPaths(externalAuthority)).toEqual(authority.inspectPaths)
    expect(milestoneAPlanPrompt(externalAuthority)).not.toContain('"logo/glob.svg"')
    expect(milestoneAPlanPrompt(externalAuthority)).toContain(
      'acceptance harness validates it in a dedicated later bounded continuation'
    )
    expect(milestoneAPlanPrompt(externalAuthority)).toContain(
      'If every required read succeeds, do not send a final response or the completion marker until create_plan has returned successfully.'
    )
    expect(milestoneAChildReviewPaths(externalAuthority)).toEqual([
      'src/math.mjs',
      'README.md',
      'test/math.test.mjs'
    ])
    expect(milestoneAFormalExecutionBounds()).toEqual({
      userGlobalMaxModelSteps: 32,
      plannerMaxModelSteps: 12,
      childMaxModelSteps: 8,
      childTimeBudgetMs: 180_000
    })
    const childTaskArguments = milestoneAChildTaskArguments(externalAuthority)
    expect(Object.keys(childTaskArguments)).toEqual([
      'prompt',
      'profile',
      'toolPolicy',
      'max_steps',
      'time_budget_ms',
      'run_in_background'
    ])
    expect(childTaskArguments).toEqual(expect.objectContaining({
      profile: 'milestone-a-readonly',
      toolPolicy: 'readOnly',
      max_steps: 8,
      time_budget_ms: 180_000,
      run_in_background: false
    }))
    for (const value of [
      authority.taskRequest,
      authority.testCommand,
      ...authority.inspectPaths
    ]) {
      expect(childTaskArguments.prompt).toContain(value)
    }
    expect(childTaskArguments.prompt).not.toContain('logo/glob.svg')
    expect(childTaskArguments.prompt).toContain(
      'Do not mutate state. Do not run commands. Do not delegate or inspect any other path.'
    )
    expect(childTaskArguments.prompt).not.toContain(
      'Do not mutate state, run commands, delegate, or inspect any other path.'
    )
    const workflowPrompt = milestoneAWorkflowPrompt(externalAuthority, 'analytix-computer-use')
    expect(workflowPrompt).toContain('Only after every required tool has completed successfully')
    expect(workflowPrompt).toContain('MILESTONE_A_RESULT_OK')
    expect(workflowPrompt).toContain('MILESTONE_A_RESEARCH_OK')
    expect(workflowPrompt).toContain('MILESTONE_A_WRITING_OK')
    expect(workflowPrompt).toContain('Do not emit any marker when a required tool fails.')
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const isolatedSettings = script.slice(
      script.indexOf('function writeIsolatedSettings({'),
      script.indexOf('export function prepareMissingBundledFundsMaterializationAuthoritySeam')
    )
    expect(isolatedSettings).toContain('runtimeTuning: {')
    expect(isolatedSettings).toContain(
      'userGlobalMaxModelSteps: formalExecutionBounds.userGlobalMaxModelSteps'
    )
    expect(isolatedSettings).toContain(
      'plannerMaxModelSteps: formalExecutionBounds.plannerMaxModelSteps'
    )

    const childTurnId = 'turn-child-review-1'
    const expectedChildReadItems = milestoneAChildReviewPaths(externalAuthority).map(
      (path: string, index: number) => ({
        kind: 'tool_call',
        callId: `call_child_read_${index}`,
        toolName: 'read',
        toolKind: 'builtin',
        status: 'completed',
        arguments: { path }
      })
    )
    const childThread = {
      turns: [{ id: childTurnId, items: expectedChildReadItems }]
    }
    expect(milestoneAChildReviewReadSetEvidence(
      externalAuthority,
      childThread,
      childTurnId,
      [true, true, true]
    )).toEqual(expect.objectContaining({
      ok: true,
      expectedReadCount: 3,
      toolAttemptCount: 3,
      readAttemptCount: 3,
      exactReadOnlyInventory: true,
      expectedPathsBound: true,
      toolAttemptDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))

    const childThreadWithUnexpectedCommand = {
      turns: [{
        id: childTurnId,
        items: [
          ...expectedChildReadItems,
          {
            kind: 'tool_call',
            callId: 'call_child_bash',
            toolName: 'bash',
            toolKind: 'builtin',
            status: 'completed',
            arguments: { command: 'true' }
          }
        ]
      }]
    }
    expect(milestoneAChildReviewReadSetEvidence(
      externalAuthority,
      childThreadWithUnexpectedCommand,
      childTurnId,
      [true, true, true]
    )).toEqual(expect.objectContaining({
      ok: false,
      expectedReadCount: 3,
      toolAttemptCount: 4,
      readAttemptCount: 3,
      exactReadOnlyInventory: false,
      expectedPathsBound: true
    }))

    const expectedAttempts = authority.inspectPaths.map((path, index) => ({
      callId: `call_read_${index}`,
      turnId: 'turn-plan-1',
      toolName: 'read',
      toolKind: 'builtin',
      status: 'completed',
      path
    }))
    expect(milestoneAPlanReadSetEvidence(
      externalAuthority,
      { resultTurnToolCallAttempts: expectedAttempts },
      [true, true, true]
    )).toEqual(expect.objectContaining({
      ok: true,
      expectedReadCount: 3,
      readAttemptCount: 3,
      expectedPathsBound: true,
      exactReadAttemptCount: true,
      longContextReadAbsent: true,
      readAttemptDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))

    const unexpectedLongContextAttempt = [
      ...expectedAttempts,
      {
        callId: 'call_read_context',
        turnId: 'turn-plan-1',
        toolName: 'read',
        toolKind: 'builtin',
        status: 'completed',
        path: externalAuthority.context.path
      }
    ]
    expect(milestoneAPlanReadSetEvidence(
      externalAuthority,
      { resultTurnToolCallAttempts: unexpectedLongContextAttempt },
      [true, true, true]
    )).toEqual(expect.objectContaining({
      ok: false,
      expectedReadCount: 3,
      readAttemptCount: 4,
      exactReadAttemptCount: false,
      longContextReadAbsent: false
    }))
  })

  it('parses a synthetic multi-commit repository contract without granting formal acceptance', { tags: ['macos-integration'], timeout: 120_000 }, async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      milestoneAPlanArtifactEvidence,
      verifyMilestoneAExternalRepositoryAcceptance,
      runParentOwnedRepositoryTest
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot
    )).toThrow('external_repository_formal_continuation_read_required')
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )

    expect(authority).toEqual(expect.objectContaining({
      contract: 'analytix.milestone-a.real-repository-authority.v1',
      mode: 'external-preexisting-real-repository',
      acceptanceClass: 'synthetic-parser-only',
      freshOriginVerified: false,
      workspace: realpathSync(input.workspace),
      contractSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      baselineCommit: input.contract.repository.baselineCommit,
      baselineTree: input.contract.repository.baselineTree,
      minimumCommitCount: 2,
      planArtifactRelativePath: expect.stringMatching(/^\.analytixsdd\/plan\/[^/]+\.md$/),
      planArtifactExcludeSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      planArtifactExcludeBeforeSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      testCommand:
        `source '${fixtureCacheHelper}' && 'node' '--test' 'test/math.test.mjs'`
    }))
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'baseline')).toEqual(
      expect.objectContaining({
        ok: true,
        acceptanceContractBound: true,
        baselineHistoryBound: true,
        originBound: true,
        formalAcceptance: false,
        freshOriginVerified: false,
        ownerRootBound: true,
        ownerOnlyIsolationBound: true,
        gitCommonDirContained: true,
        symlinkHardlinkIsolationBound: true,
        provenanceBound: true,
        planArtifactExcludeBound: true,
        planArtifactPathSafe: true,
        fixtureSubstituteRejected: true,
        onlyIntendedSourceChanged: true,
        expectedSourceBound: true
      })
    )
    expect(existsSync(join(input.workspace, 'src', 'counter.mjs'))).toBe(false)
    expect(existsSync(join(input.workspace, '.gitignore'))).toBe(false)
    const excludeText = readFileSync(authority.planArtifactExcludePath, 'utf8')
    expect(excludeText.split(/\r?\n/).filter((line: string) =>
      line.trim() && !line.trim().startsWith('#')
    )).toEqual([authority.planArtifactExcludeRule])
    expect(lstatSync(authority.planArtifactExcludePath).mode & 0o777).toBe(0o600)

    const plan = materializeClosedPlanArtifact(authority)
    const planEvidence = milestoneAPlanArtifactEvidence(
      authority,
      plan.thread,
      plan.turnId
    )
    expect(planEvidence).toEqual(expect.objectContaining({
      ok: true,
      relativePath: authority.planArtifactRelativePath,
      contentHash: expect.stringMatching(/^[0-9a-f]{64}$/),
      byteSize: expect.any(Number),
      resultItemDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      revisionSequenceDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      fileIdentityDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      excludeSha256: authority.planArtifactExcludeSha256,
      planEntryCount: 1,
      planCallCount: 1,
      planResultCount: 1
    }))
    const planWithUnrelatedTurn = structuredClone(plan.thread)
    planWithUnrelatedTurn.turns.push({
      id: 'turn-unrelated-plan',
      threadId: plan.thread.id,
      status: 'completed',
      items: structuredClone(plan.thread.turns[0].items)
    })
    expect(milestoneAPlanArtifactEvidence(
      authority,
      planWithUnrelatedTurn,
      plan.turnId
    )).toEqual(planEvidence)

    const multiRevisionPlan = structuredClone(plan.thread)
    const earlierCallId = `call_host_${'c'.repeat(64)}`
    const earlierCall = structuredClone(plan.thread.turns[0].items[0])
    earlierCall.id = `item_call_${'d'.repeat(64)}`
    earlierCall.callId = earlierCallId
    const earlierResult = structuredClone(plan.thread.turns[0].items[1])
    earlierResult.id = resultItemId(plan.turnId, earlierCallId)
    earlierResult.callId = earlierCallId
    earlierResult.output.plan.contentHash = 'e'.repeat(64)
    earlierResult.output.plan.byteSize = 64
    earlierResult.output.plan.savedAt = '2026-08-01T11:59:59.123456789Z'
    multiRevisionPlan.turns[0].items.unshift(earlierCall, earlierResult)
    const multiRevisionEvidence = milestoneAPlanArtifactEvidence(
      authority,
      multiRevisionPlan,
      plan.turnId
    )
    expect(multiRevisionEvidence).toEqual(expect.objectContaining({
      ok: true,
      contentHash: planEvidence.contentHash,
      resultItemDigest: planEvidence.resultItemDigest,
      revisionSequenceDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      planEntryCount: 1,
      planCallCount: 2,
      planResultCount: 2
    }))
    expect(multiRevisionEvidence.revisionSequenceDigest)
      .not.toBe(planEvidence.revisionSequenceDigest)

    const missingEarlierRevision = structuredClone(multiRevisionPlan)
    missingEarlierRevision.turns[0].items.splice(0, 2)
    const missingEarlierRevisionEvidence = milestoneAPlanArtifactEvidence(
      authority,
      missingEarlierRevision,
      plan.turnId
    )
    expect(missingEarlierRevisionEvidence).toEqual(expect.objectContaining({
      ok: true,
      resultItemDigest: multiRevisionEvidence.resultItemDigest,
      planCallCount: 1,
      planResultCount: 1
    }))
    expect(missingEarlierRevisionEvidence.revisionSequenceDigest)
      .not.toBe(multiRevisionEvidence.revisionSequenceDigest)

    const changedEarlierRevision = structuredClone(multiRevisionPlan)
    changedEarlierRevision.turns[0].items[1].output.plan.contentHash = 'f'.repeat(64)
    const changedEarlierRevisionEvidence = milestoneAPlanArtifactEvidence(
      authority,
      changedEarlierRevision,
      plan.turnId
    )
    expect(changedEarlierRevisionEvidence).toEqual(expect.objectContaining({
      ok: true,
      resultItemDigest: multiRevisionEvidence.resultItemDigest,
      planCallCount: 2,
      planResultCount: 2
    }))
    expect(changedEarlierRevisionEvidence.revisionSequenceDigest)
      .not.toBe(multiRevisionEvidence.revisionSequenceDigest)

    const duplicateInPlanTurn = structuredClone(plan.thread)
    duplicateInPlanTurn.turns[0].items.push(
      ...structuredClone(plan.thread.turns[0].items)
    )
    expect(milestoneAPlanArtifactEvidence(
      authority,
      duplicateInPlanTurn,
      plan.turnId
    )).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'milestone_a_plan_execution_pair_invalid',
      pairFailureCode: 'plan_result_call_id_duplicate',
      planCallCount: 2,
      planResultCount: 2
    }))
    expect(gitOutput(input.workspace, ['status', '--porcelain=v1', '--untracked-files=all']))
      .toBe('')

    writeFileSync(join(input.workspace, 'src', 'math.mjs'), input.repairedSource, 'utf8')
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')).toEqual(
      expect.objectContaining({
        ok: true,
        acceptanceContractBound: true,
        planArtifactExcludeBound: true,
        planArtifactPathSafe: true,
        onlyIntendedSourceChanged: true,
        expectedSourceBound: true
      })
    )
    expect(runParentOwnedRepositoryTest(authority)).toEqual(expect.objectContaining({
      passed: true,
      command: 'contract-bound-test-command',
      commandDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      exitCode: 0,
      repositoryStableBeforeAndAfter: true,
      sandboxCleanupSucceeded: true
    }))
    expect(milestoneAPlanArtifactEvidence(
      authority,
      structuredClone(plan.thread),
      plan.turnId
    )).toEqual(planEvidence)
  })

  it('closes an authorized-path expected-byte mismatch without exposing repository content', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      projectMilestoneARepositoryAcceptance,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    materializeClosedPlanArtifact(authority)
    const mismatchedSource =
      'export function multiply(left, right) {\n  return left - right\n}\n'
    writeFileSync(join(input.workspace, 'src', 'math.mjs'), mismatchedSource, 'utf8')

    const acceptance = verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')
    expect(acceptance).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'external_repository_expected_result_mismatch',
      onlyIntendedSourceChanged: true,
      expectedSourceBound: false,
      expectedSourceFileCount: 1,
      expectedSourceMismatchCount: 1,
      expectedSourceDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      observedSourceDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(acceptance.expectedSourceDigest).not.toBe(acceptance.observedSourceDigest)

    const projected = projectMilestoneARepositoryAcceptance({
      mode: 'external-preexisting-real-repository'
    }, acceptance)
    expect(projected).toEqual(expect.objectContaining({
      mode: 'external-preexisting-real-repository',
      finalValidated: false,
      blocker: 'external_repository_expected_result_mismatch',
      finalOnlyIntendedSourceChanged: true,
      finalExpectedSourceBound: false,
      finalExpectedSourceFileCount: 1,
      finalExpectedSourceMismatchCount: 1,
      finalExpectedSourceDigest: acceptance.expectedSourceDigest,
      finalObservedSourceDigest: acceptance.observedSourceDigest,
      finalGitStatusSha256: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    const serialized = JSON.stringify(projected)
    expect(serialized).not.toContain('src/math.mjs')
    expect(serialized).not.toContain(mismatchedSource.trim())
    expect(serialized).not.toContain(input.workspace)
  }, 30_000)

  it('retains passed baseline repository isolation evidence before final acceptance runs', async () => {
    const { projectMilestoneARepositoryBaseline } = await milestoneModule()
    const repository = {
      baselineValidated: false,
      preflightValidated: false,
      ownerRootBound: false,
      ownerOnlyIsolationBound: false,
      gitCommonDirContained: false,
      symlinkHardlinkIsolationBound: false,
      planArtifactExcludeBound: false,
      planArtifactPathSafe: false,
      finalValidated: false,
      finalOnlyIntendedSourceChanged: false,
      finalExpectedSourceBound: false,
      finalExpectedSourceFileCount: 0,
      finalExpectedSourceMismatchCount: 0,
      finalExpectedSourceDigest: '',
      finalObservedSourceDigest: '',
      finalGitStatusSha256: ''
    }
    const baseline = {
      ok: true,
      phase: 'baseline',
      ownerRootBound: true,
      ownerOnlyIsolationBound: true,
      gitCommonDirContained: true,
      symlinkHardlinkIsolationBound: true,
      planArtifactExcludeBound: true,
      planArtifactPathSafe: true
    }

    const projected = projectMilestoneARepositoryBaseline(repository, baseline)

    expect(projected).toEqual(expect.objectContaining({
      baselineValidated: true,
      preflightValidated: true,
      ownerRootBound: true,
      ownerOnlyIsolationBound: true,
      gitCommonDirContained: true,
      symlinkHardlinkIsolationBound: true,
      planArtifactExcludeBound: true,
      planArtifactPathSafe: true,
      finalValidated: false,
      finalOnlyIntendedSourceChanged: false,
      finalExpectedSourceBound: false,
      finalExpectedSourceFileCount: 0,
      finalExpectedSourceMismatchCount: 0,
      finalExpectedSourceDigest: '',
      finalObservedSourceDigest: '',
      finalGitStatusSha256: ''
    }))
  })

  it('loads the bounded continuation contract through the repository authority path', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const boundedContract = input.contract as Record<string, any>
    const contextPath = join(input.workspace, boundedContract.longContext.path)
    const boundedContent = 'bounded ordinary continuation\n'
    writeFileSync(contextPath, boundedContent, 'utf8')
    runGit(input.workspace, ['add', '--', boundedContract.longContext.path])
    runGit(input.workspace, [
      '-c', 'user.name=External Repository Author',
      '-c', 'user.email=external-repository@analytix.invalid',
      '-c', 'commit.gpgsign=false',
      'commit', '--quiet', '-m', 'Bound ordinary continuation input'
    ])
    const contextRelativePath = boundedContract.longContext.path
    delete boundedContract.longContext
    boundedContract.continuationRead = {
      path: contextRelativePath,
      sha256: sha256(boundedContent),
      lineLimit: 1,
      maxBytes: 4096,
      successMarker: 'MILESTONE_A_CONTINUATION_OK'
    }
    boundedContract.repository.baselineCommit = gitOutput(input.workspace, ['rev-parse', 'HEAD'])
    boundedContract.repository.baselineTree = gitOutput(input.workspace, ['rev-parse', 'HEAD^{tree}'])
    input.provenance.baselineCommit = boundedContract.repository.baselineCommit
    input.provenance.baselineTree = boundedContract.repository.baselineTree
    writeFileSync(input.contractPath, `${JSON.stringify(boundedContract, null, 2)}\n`, 'utf8')
    writeFileSync(input.provenancePath, `${JSON.stringify(input.provenance, null, 2)}\n`, 'utf8')

    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    expect(authority.context).toEqual(expect.objectContaining({
      path: contextRelativePath,
      lineLimit: 1,
      maxBytes: 4096,
      deprecated: false
    }))
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'baseline'))
      .toEqual(expect.objectContaining({ ok: true, formalAcceptance: false }))
  }, 30_000)

  it('preflights the complete bounded repository contract without installing the plan exclusion', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      preflightMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const boundedContract = input.contract as Record<string, any>
    const contextRelativePath = boundedContract.longContext.path
    const contextPath = join(input.workspace, contextRelativePath)
    const boundedContent = 'first bounded line\nsecond bounded line'
    writeFileSync(contextPath, boundedContent, 'utf8')
    runGit(input.workspace, ['add', '--', contextRelativePath])
    runGit(input.workspace, [
      '-c', 'user.name=External Repository Author',
      '-c', 'user.email=external-repository@analytix.invalid',
      '-c', 'commit.gpgsign=false',
      'commit', '--quiet', '-m', 'Add physical continuation lines'
    ])
    delete boundedContract.longContext
    boundedContract.continuationRead = {
      path: contextRelativePath,
      sha256: sha256(boundedContent),
      lineLimit: 1,
      maxBytes: 4096,
      successMarker: 'MILESTONE_A_PREFLIGHT_OK'
    }
    boundedContract.repository.baselineCommit = gitOutput(input.workspace, ['rev-parse', 'HEAD'])
    boundedContract.repository.baselineTree = gitOutput(input.workspace, ['rev-parse', 'HEAD^{tree}'])
    input.provenance.baselineCommit = boundedContract.repository.baselineCommit
    input.provenance.baselineTree = boundedContract.repository.baselineTree
    writeFileSync(input.contractPath, `${JSON.stringify(boundedContract, null, 2)}\n`, 'utf8')
    writeFileSync(input.provenancePath, `${JSON.stringify(input.provenance, null, 2)}\n`, 'utf8')

    const excludePath = join(input.workspace, '.git', 'info', 'exclude')
    const excludeBefore = readFileSync(excludePath, 'utf8')
    expect(() => preflightMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_continuation_read_contract_mismatch')
    expect(readFileSync(excludePath, 'utf8')).toBe(excludeBefore)

    boundedContract.continuationRead.lineLimit = 2
    boundedContract.repository.minimumCommitCount = 10_000
    writeFileSync(input.contractPath, `${JSON.stringify(boundedContract, null, 2)}\n`, 'utf8')
    expect(() => preflightMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_history_insufficient')
    expect(readFileSync(excludePath, 'utf8')).toBe(excludeBefore)

    boundedContract.repository.minimumCommitCount = 2
    writeFileSync(input.contractPath, `${JSON.stringify(boundedContract, null, 2)}\n`, 'utf8')
    expect(preflightMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toEqual(expect.objectContaining({
      contract: 'analytix.milestone-a.real-repository-preflight.v1',
      ok: true,
      mutationApplied: false,
      acceptanceClass: 'synthetic-parser-only',
      baselineCommit: boundedContract.repository.baselineCommit,
      baselineTree: boundedContract.repository.baselineTree,
      contextLineLimit: 2,
      freshOriginVerified: false,
      formalAcceptance: false,
      ownerRootBound: true,
      ownerOnlyIsolationBound: true,
      gitCommonDirContained: true,
      symlinkHardlinkIsolationBound: true,
      planArtifactExcludeBound: true,
      planArtifactPathSafe: true
    }))
    expect(readFileSync(excludePath, 'utf8')).toBe(excludeBefore)

    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    expect(authority.planArtifactExcludeSha256).toMatch(/^[0-9a-f]{64}$/)
    expect(readFileSync(excludePath, 'utf8')).not.toBe(excludeBefore)
  }, 30_000)

  it('retries the fresh-origin transport once without weakening commit and tree binding', async () => {
    const { verifyFreshExternalRepositoryOrigin } = await milestoneModule()
    const root = taskOwnedSandbox()
    const commit = 'a'.repeat(40)
    const tree = 'b'.repeat(40)
    const provenance = {
      contract: 'analytix.milestone-a.external-repository-provenance.v1',
      admissionKind: 'independent-pre-admission',
      originUrl: 'https://example.com/analytix/fresh-origin.git',
      baselineCommit: commit,
      baselineTree: tree
    }
    const probeRepositories: string[] = []
    let fetchAttemptCount = 0
    const runner = vi.fn((command: string, arguments_: string[]) => {
      expect(command).toBe('git')
      if (arguments_[0] === 'init') {
        probeRepositories.push(arguments_.at(-1) || '')
        return { status: 0, stdout: '', stderr: '' }
      }
      if (arguments_.includes('fetch')) {
        fetchAttemptCount += 1
        return fetchAttemptCount === 1
          ? { status: 128, stdout: '', stderr: 'PRIVATE_TRANSPORT_BODY' }
          : { status: 0, stdout: '', stderr: '' }
      }
      if (arguments_.at(-1) === 'FETCH_HEAD^{commit}') {
        return { status: 0, stdout: `${commit}\n`, stderr: '' }
      }
      if (arguments_.at(-1) === 'FETCH_HEAD^{tree}') {
        return { status: 0, stdout: `${tree}\n`, stderr: '' }
      }
      throw new Error(`unexpected git invocation: ${arguments_.join(' ')}`)
    })

    const evidence = verifyFreshExternalRepositoryOrigin(provenance, {
      trustedCacheTempRoot: () => ({ ok: true, path: root }),
      spawnSync: runner
    })
    expect(evidence).toEqual(expect.objectContaining({
      ok: true,
      blocker: '',
      baselineCommit: commit,
      baselineTree: tree,
      fetchAttemptCount: 2,
      fetchExitCodes: [128, 0],
      cleanupSucceeded: true
    }))
    expect(probeRepositories).toHaveLength(2)
    expect(new Set(probeRepositories).size).toBe(2)
    expect(JSON.stringify(evidence)).not.toContain('PRIVATE_TRANSPORT_BODY')
    expect(probeRepositories.every((path) => !existsSync(path))).toBe(true)

    fetchAttemptCount = 0
    const exhausted = verifyFreshExternalRepositoryOrigin(provenance, {
      trustedCacheTempRoot: () => ({ ok: true, path: root }),
      spawnSync: (command: string, arguments_: string[]) => {
        expect(command).toBe('git')
        if (arguments_[0] === 'init') return { status: 0, stdout: '', stderr: '' }
        if (arguments_.includes('fetch')) {
          fetchAttemptCount += 1
          return { status: 128, stdout: '', stderr: 'PRIVATE_TRANSPORT_BODY' }
        }
        throw new Error(`unexpected git invocation: ${arguments_.join(' ')}`)
      }
    })
    expect(exhausted).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'external_repository_origin_fetch_failed',
      fetchAttemptCount: 2,
      fetchExitCodes: [128, 128],
      cleanupSucceeded: true
    }))
    expect(JSON.stringify(exhausted)).not.toContain('PRIVATE_TRANSPORT_BODY')
  }, 30_000)

  it('keeps the configured repository dry-run preflight mutation-free', async () => {
    const { configuredExternalRepositoryAcceptance } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const excludePath = join(input.workspace, '.git', 'info', 'exclude')
    const excludeBefore = readFileSync(excludePath, 'utf8')

    const acceptance = configuredExternalRepositoryAcceptance({
      repositoryPath: input.workspace,
      contractPath: input.contractPath,
      provenancePath: input.provenancePath,
      ownerRoot: input.ownerRoot,
      verificationMode: 'synthetic-parser-only',
      preflightOnly: true
    })

    expect(acceptance).toEqual(expect.objectContaining({
      ok: true,
      formalAcceptance: false,
      preflightValidated: true,
      mutationApplied: false,
      ownerRootBound: true,
      ownerOnlyIsolationBound: true,
      gitCommonDirContained: true,
      symlinkHardlinkIsolationBound: true,
      planArtifactExcludeBound: true,
      planArtifactPathSafe: true,
      authority: null
    }))
    expect(readFileSync(excludePath, 'utf8')).toBe(excludeBefore)

    const execution = configuredExternalRepositoryAcceptance({
      repositoryPath: input.workspace,
      contractPath: input.contractPath,
      provenancePath: input.provenancePath,
      ownerRoot: input.ownerRoot,
      verificationMode: 'synthetic-parser-only'
    })
    expect(execution).toEqual(expect.objectContaining({
      ok: true,
      formalAcceptance: false,
      preflightValidated: false,
      mutationApplied: true,
      authority: expect.objectContaining({
        acceptanceClass: 'synthetic-parser-only'
      })
    }))
    expect(readFileSync(excludePath, 'utf8')).not.toBe(excludeBefore)
  }, 30_000)

  it('rejects external long-context markers that do not bind three distinct lines', async () => {
    const { loadMilestoneAExternalRepositoryAcceptance } = await milestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const contextPath = join(input.workspace, input.contract.longContext.path)
    const oneLineContext = readFileSync(contextPath, 'utf8').replace(/[\r\n]+/gu, ' ')
    writeFileSync(contextPath, oneLineContext, 'utf8')
    runGit(input.workspace, ['add', '--', input.contract.longContext.path])
    runGit(input.workspace, [
      '-c', 'user.name=External Repository Author',
      '-c', 'user.email=external-repository@analytix.invalid',
      '-c', 'commit.gpgsign=false',
      'commit', '--quiet', '-m', 'Collapse acceptance context'
    ])
    input.contract.repository.baselineCommit = gitOutput(input.workspace, ['rev-parse', 'HEAD'])
    input.contract.repository.baselineTree = gitOutput(input.workspace, ['rev-parse', 'HEAD^{tree}'])
    input.contract.longContext.sha256 = sha256(oneLineContext)
    input.provenance.baselineCommit = input.contract.repository.baselineCommit
    input.provenance.baselineTree = input.contract.repository.baselineTree
    writeFileSync(input.contractPath, `${JSON.stringify(input.contract, null, 2)}\n`, 'utf8')
    writeFileSync(input.provenancePath, `${JSON.stringify(input.provenance, null, 2)}\n`, 'utf8')

    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_long_context_contract_mismatch')
  }, 30_000)

  it('rejects broad plan exclusions and any drift between the closed plan result and artifact', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      milestoneAPlanArtifactEvidence,
      verifyMilestoneAExternalRepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const preconfigured = externalRepositoryAcceptanceFixture(join(root, 'preconfigured'))
    const preconfiguredExclude = join(preconfigured.workspace, '.git', 'info', 'exclude')
    writeFileSync(
      preconfiguredExclude,
      `${readFileSync(preconfiguredExclude, 'utf8')}/.analytixsdd/\n`,
      'utf8'
    )
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      preconfigured.workspace,
      preconfigured.contractPath,
      preconfigured.provenancePath,
      preconfigured.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_plan_exclude_preconfigured')

    const input = externalRepositoryAcceptanceFixture(join(root, 'closed-artifact'))
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    const plan = materializeClosedPlanArtifact(authority)
    const accepted = milestoneAPlanArtifactEvidence(authority, plan.thread, plan.turnId)
    expect(accepted.ok).toBe(true)

    const wrongCallRole = structuredClone(plan.thread)
    wrongCallRole.turns[0].items[0].role = 'assistant'
    expect(milestoneAPlanArtifactEvidence(authority, wrongCallRole, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_item_role_mismatch'
      })
    )

    const wrongResultRole = structuredClone(plan.thread)
    wrongResultRole.turns[0].items[1].role = 'assistant'
    expect(milestoneAPlanArtifactEvidence(authority, wrongResultRole, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_item_role_mismatch'
      })
    )

    const failedResult = structuredClone(plan.thread)
    failedResult.turns[0].items[0].status = 'failed'
    failedResult.turns[0].items[1].status = 'failed'
    failedResult.turns[0].items[1].isError = true
    expect(milestoneAPlanArtifactEvidence(authority, failedResult, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_result_failed'
      })
    )

    const resultWithoutErrorFlag = structuredClone(plan.thread)
    delete resultWithoutErrorFlag.turns[0].items[1].isError
    expect(milestoneAPlanArtifactEvidence(authority, resultWithoutErrorFlag, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_result_error_flag_invalid'
      })
    )

    const unsettledCall = structuredClone(plan.thread)
    unsettledCall.turns[0].items[0].status = 'running'
    expect(milestoneAPlanArtifactEvidence(authority, unsettledCall, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_call_lifecycle_unsettled'
      })
    )

    const unsettledResult = structuredClone(plan.thread)
    unsettledResult.turns[0].items[1].status = 'running'
    expect(milestoneAPlanArtifactEvidence(authority, unsettledResult, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_result_lifecycle_unsettled'
      })
    )

    const concurrentRevision = structuredClone(plan.thread)
    const concurrentCallId = `call_host_${'c'.repeat(64)}`
    const concurrentCall = structuredClone(plan.thread.turns[0].items[0])
    concurrentCall.id = `item_call_${'d'.repeat(64)}`
    concurrentCall.callId = concurrentCallId
    const concurrentResult = structuredClone(plan.thread.turns[0].items[1])
    concurrentResult.id = resultItemId(plan.turnId, concurrentCallId)
    concurrentResult.callId = concurrentCallId
    concurrentRevision.turns[0].items.splice(1, 0, concurrentCall)
    concurrentRevision.turns[0].items.push(concurrentResult)
    expect(milestoneAPlanArtifactEvidence(authority, concurrentRevision, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_item_order_invalid'
      })
    )

    const artifactBody = readFileSync(plan.artifactPath)
    rmSync(plan.artifactPath)
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'final')).toEqual(
      expect.objectContaining({ ok: false, blocker: 'external_repository_plan_artifact_unsafe' })
    )
    writeFileSync(plan.artifactPath, artifactBody, { mode: 0o644 })
    chmodSync(plan.artifactPath, 0o644)

    const extraPlan = join(input.workspace, '.analytixsdd', 'plan', 'extra.md')
    writeFileSync(extraPlan, '# unexpected\n', 'utf8')
    expect(milestoneAPlanArtifactEvidence(authority, plan.thread, plan.turnId)).toEqual(
      expect.objectContaining({ ok: false, blocker: 'milestone_a_plan_artifact_tree_invalid' })
    )
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'baseline')).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'external_repository_plan_artifact_unsafe',
        onlyIntendedSourceChanged: false
      })
    )
    rmSync(extraPlan)

    const invalidResultId = structuredClone(plan.thread)
    invalidResultId.turns[0].items[1].id = `item_result_${'f'.repeat(64)}`
    expect(milestoneAPlanArtifactEvidence(authority, invalidResultId, plan.turnId)).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'milestone_a_plan_execution_pair_invalid',
        pairFailureCode: 'plan_result_item_id_mismatch'
      })
    )
    const extraProjectionField = structuredClone(plan.thread)
    extraProjectionField.turns[0].items[1].output.unsafe = true
    expect(milestoneAPlanArtifactEvidence(authority, extraProjectionField, plan.turnId)).toEqual(
      expect.objectContaining({ ok: false, blocker: 'milestone_a_plan_projection_shape_invalid' })
    )
    const wrongDigest = structuredClone(plan.thread)
    wrongDigest.turns[0].items[1].output.plan.contentHash = 'b'.repeat(64)
    expect(milestoneAPlanArtifactEvidence(authority, wrongDigest, plan.turnId)).toEqual(
      expect.objectContaining({ ok: false, blocker: 'milestone_a_plan_artifact_digest_invalid' })
    )

    writeFileSync(
      authority.planArtifactExcludePath,
      `${readFileSync(authority.planArtifactExcludePath, 'utf8')}/.analytixsdd/**\n`,
      { encoding: 'utf8', mode: 0o600 }
    )
    expect(milestoneAPlanArtifactEvidence(authority, plan.thread, plan.turnId)).toEqual(
      expect.objectContaining({ ok: false, blocker: 'milestone_a_plan_exclude_invalid' })
    )
    expect(verifyMilestoneAExternalRepositoryAcceptance(authority, 'baseline')).toEqual(
      expect.objectContaining({ ok: false, blocker: 'external_repository_plan_exclude_changed' })
    )
  }, 30_000)

  it('rejects invalid, dirty, in-worktree, and built-in fixture repository substitutes', async () => {
    const { loadMilestoneAExternalRepositoryAcceptance } = await milestoneModule()
    const root = taskOwnedSandbox()
    const invalid = externalRepositoryAcceptanceFixture(join(root, 'invalid'))
    invalid.contract.test.executable = '../node'
    writeFileSync(
      invalid.contractPath,
      `${JSON.stringify(invalid.contract, null, 2)}\n`,
      'utf8'
    )
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      invalid.workspace,
      invalid.contractPath,
      invalid.provenancePath,
      invalid.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_contract_invalid')

    const dirty = externalRepositoryAcceptanceFixture(join(root, 'dirty'))
    writeFileSync(join(dirty.workspace, 'unexpected.txt'), 'dirty\n', 'utf8')
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      dirty.workspace,
      dirty.contractPath,
      dirty.provenancePath,
      dirty.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_clean_baseline_mismatch')

    const substitute = externalRepositoryAcceptanceFixture(
      join(root, 'substitute'),
      'analytix-milestone-a-isolated-repo'
    )
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      substitute.workspace,
      substitute.contractPath,
      substitute.provenancePath,
      substitute.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_fixture_substitute_rejected')

    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      process.cwd(),
      substitute.contractPath,
      substitute.provenancePath,
      substitute.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_must_be_preexisting_and_outside_source_worktree')

    const permissive = externalRepositoryAcceptanceFixture(join(root, 'permissive'))
    chmodSync(permissive.ownerRoot, 0o770)
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      permissive.workspace,
      permissive.contractPath,
      permissive.provenancePath,
      permissive.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_permissions_invalid')

    const symlinked = externalRepositoryAcceptanceFixture(join(root, 'symlinked'))
    const provenanceSymlink = join(symlinked.ownerRoot, 'provenance-link.json')
    symlinkSync(symlinked.provenancePath, provenanceSymlink)
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      symlinked.workspace,
      symlinked.contractPath,
      provenanceSymlink,
      symlinked.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_input_type_invalid')

    const hardlinked = externalRepositoryAcceptanceFixture(join(root, 'hardlinked'))
    linkSync(hardlinked.contractPath, join(hardlinked.ownerRoot, 'contract-hardlink.json'))
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      hardlinked.workspace,
      hardlinked.contractPath,
      hardlinked.provenancePath,
      hardlinked.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_input_type_invalid')

    const linkedWorktree = externalRepositoryAcceptanceFixture(join(root, 'linked-worktree'))
    const linkedWorkspace = join(linkedWorktree.ownerRoot, 'linked-checkout')
    runGit(linkedWorktree.workspace, ['worktree', 'add', '--quiet', '--detach', linkedWorkspace])
    expect(() => loadMilestoneAExternalRepositoryAcceptance(
      linkedWorkspace,
      linkedWorktree.contractPath,
      linkedWorktree.provenancePath,
      linkedWorktree.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )).toThrow('external_repository_git_common_dir_invalid')
  }, 30_000)

  it('binds host observations only to the external contract test and required read arguments', async () => {
    const {
      fullTextReadLineLimit,
      loadMilestoneAExternalRepositoryAcceptance,
      observeHostToolExecution,
      privateHostToolExecutionBindingMatches
    } = await instrumentedMilestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    const dependencies = (toolName: 'bash' | 'read', turnId: string) => ({
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => successfulHostToolObservationRaw(
        'thread-external',
        turnId,
        toolName
      ),
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    const bashRequest = {
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: 'thread-external',
      turnId: 'turn-test',
      toolName: 'bash',
      workspace: input.workspace,
      arguments: { command: authority.testCommand }
    }
    const bash = await observeHostToolExecution(
      bashRequest,
      dependencies('bash', 'turn-test')
    )
    expect(privateHostToolExecutionBindingMatches(bash, {
      threadId: bashRequest.threadId,
      turnId: bashRequest.turnId,
      toolName: bashRequest.toolName,
      workspace: bashRequest.workspace,
      arguments: bashRequest.arguments
    })).toBe(true)

    const readRequest = {
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: 'thread-external',
      turnId: 'turn-read',
      toolName: 'read',
      workspace: input.workspace,
      arguments: { path: 'src/math.mjs' }
    }
    const read = await observeHostToolExecution(
      readRequest,
      dependencies('read', 'turn-read')
    )
    expect(privateHostToolExecutionBindingMatches(read, {
      threadId: readRequest.threadId,
      turnId: readRequest.turnId,
      toolName: readRequest.toolName,
      workspace: readRequest.workspace,
      arguments: readRequest.arguments
    })).toBe(true)

    const contextReadLineLimit = fullTextReadLineLimit(readFileSync(
      join(input.workspace, authority.context.path)
    ))
    expect(contextReadLineLimit).toBe(2102)
    const contextReadRequest = {
      ...readRequest,
      turnId: 'turn-context-read',
      arguments: {
        path: authority.context.path,
        limit: contextReadLineLimit
      }
    }
    const contextRead = await observeHostToolExecution(
      contextReadRequest,
      dependencies('read', contextReadRequest.turnId)
    )
    expect(privateHostToolExecutionBindingMatches(contextRead, {
      threadId: contextReadRequest.threadId,
      turnId: contextReadRequest.turnId,
      toolName: contextReadRequest.toolName,
      workspace: contextReadRequest.workspace,
      arguments: contextReadRequest.arguments
    })).toBe(true)

    for (const invalidArguments of [
      { path: authority.context.path },
      { path: authority.context.path, limit: contextReadLineLimit - 1 },
      { path: authority.context.path, limit: contextReadLineLimit, offset: 1 }
    ]) {
      const invalidContextRead = await observeHostToolExecution({
        ...contextReadRequest,
        arguments: invalidArguments
      }, dependencies('read', contextReadRequest.turnId))
      expect(invalidContextRead).toEqual(expect.objectContaining({
        ok: false,
        blocker: 'host_owned_tool_invocation_binding_unavailable'
      }))
    }

    const uncontracted = await observeHostToolExecution({
      ...readRequest,
      arguments: { path: 'src/uncontracted.mjs' }
    }, dependencies('read', 'turn-read'))
    expect(uncontracted).toEqual(expect.objectContaining({
      ok: false,
      blocker: 'host_owned_tool_invocation_binding_unavailable'
    }))
  }, 30_000)

  it('retries only sanitized not-observed host responses and keeps transport diagnostics bounded', async () => {
    const {
      loadMilestoneAExternalRepositoryAcceptance,
      observeHostToolExecution,
      privateHostToolExecutionBindingMatches
    } = await instrumentedMilestoneModule()
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const authority = loadMilestoneAExternalRepositoryAcceptance(
      input.workspace,
      input.contractPath,
      input.provenancePath,
      input.ownerRoot,
      { verificationMode: 'synthetic-parser-only' }
    )
    const request = {
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: 'thread-host-retry',
      turnId: 'turn-host-retry',
      toolName: 'bash',
      workspace: input.workspace,
      arguments: { command: authority.testCommand }
    }
    let evaluationCount = 0
    const sleepDelays: number[] = []
    const retried = await observeHostToolExecution(request, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        evaluationCount += 1
        return evaluationCount < 3
          ? { status: 404, observation: null }
          : successfulHostToolObservationRaw(request.threadId, request.turnId, 'bash')
      },
      sleep: async (delayMs: number) => {
        sleepDelays.push(delayMs)
      },
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })

    expect(retried).toEqual(expect.objectContaining({
      ok: true,
      reasonCode: 'observed',
      transportStatus: 200,
      attemptCount: 3,
      observationDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      authorityBindingDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(evaluationCount).toBe(3)
    expect(sleepDelays).toHaveLength(2)
    expect(privateHostToolExecutionBindingMatches(retried, {
      threadId: request.threadId,
      turnId: request.turnId,
      toolName: request.toolName,
      workspace: request.workspace,
      arguments: request.arguments
    })).toBe(true)

    let rejectedEvaluationCount = 0
    const rejected = await observeHostToolExecution(request, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        rejectedEvaluationCount += 1
        return { status: 400, observation: null }
      },
      sleep: async () => {
        throw new Error('non-404 responses must not sleep')
      },
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    expect(rejected).toEqual(expect.objectContaining({
      ok: false,
      reasonCode: 'http_error',
      transportStatus: 400,
      attemptCount: 1,
      observationDigest: '',
      authorityBindingDigest: ''
    }))
    expect(rejectedEvaluationCount).toBe(1)

    let exhaustedEvaluationCount = 0
    const exhausted = await observeHostToolExecution(request, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        exhaustedEvaluationCount += 1
        return { status: 404, observation: null }
      },
      sleep: async () => {},
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    expect(exhausted).toEqual(expect.objectContaining({
      ok: false,
      reasonCode: 'not_observed',
      transportStatus: 404,
      attemptCount: 3
    }))
    expect(exhaustedEvaluationCount).toBe(3)

    let malformedEvaluationCount = 0
    const malformed = await observeHostToolExecution(request, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        malformedEvaluationCount += 1
        return { status: 404 }
      },
      sleep: async () => {
        throw new Error('invalid response schema must not sleep')
      },
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    expect(malformed).toEqual(expect.objectContaining({
      ok: false,
      reasonCode: 'response_schema_invalid',
      transportStatus: 404,
      attemptCount: 1
    }))
    expect(malformedEvaluationCount).toBe(1)

    let transportEvaluationCount = 0
    const transportFailure = await observeHostToolExecution(request, {
      waitForDebugTarget: async () => ({
        webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
        pageTargetCount: 1
      }),
      evaluateReadonlyCdp: async () => {
        transportEvaluationCount += 1
        throw new Error('transport unavailable')
      },
      sleep: async () => {
        throw new Error('transport failures must not sleep')
      },
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    expect(transportFailure).toEqual(expect.objectContaining({
      ok: false,
      reasonCode: 'transport_unavailable',
      transportStatus: 0,
      attemptCount: 1
    }))
    expect(transportEvaluationCount).toBe(1)
  }, 30_000)

  it('retains an immutable, redacted ordinary test-binding failure snapshot', async () => {
    const {
      ordinaryTestBindingFailureStageSnapshot,
      sanitizeMilestoneAStageSnapshot
    } = await milestoneModule()
    const snapshot = ordinaryTestBindingFailureStageSnapshot({
      reasonCode: 'not_observed',
      transportStatus: 404,
      attemptCount: 3,
      observationDigest: '',
      authorityBindingDigest: ''
    })
    expect(snapshot).toEqual(expect.objectContaining({
      stage: 'ordinary-test-binding-failure',
      checkIds: ['ordinary-agent-workflow', 'real-repository-test'],
      completedFields: {
        bashHostObservationReasonCode: 'not_observed',
        bashHostObservationTransportStatus: 404,
        bashHostObservationAttemptCount: 3,
        bashHostObservationDigest: '',
        bashHostAuthorityBindingDigest: ''
      },
      snapshotDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(Object.isFrozen(snapshot)).toBe(true)
    expect(Object.isFrozen(snapshot?.checkIds)).toBe(true)
    expect(Object.isFrozen(snapshot?.completedFields)).toBe(true)
    expect(JSON.stringify(snapshot)).not.toContain('command')
    expect(JSON.stringify(snapshot)).not.toContain('arguments')
    expect(JSON.stringify(snapshot)).not.toContain('workspace')

    const report = {
      workflow: {
        ordinaryWorkflowDisposition: 'completed',
        bashHostObservationReasonCode: 'not_observed',
        bashHostObservationTransportStatus: 404,
        bashHostObservationAttemptCount: 3,
        bashHostObservationDigest: '',
        bashHostAuthorityBindingDigest: ''
      },
      checks: [
        { id: 'ordinary-agent-workflow', status: 'failed', message: 'host_owned_tool_invocation_binding_unavailable' },
        { id: 'real-repository-test', status: 'skipped', message: 'stage did not reach this required check' }
      ],
      stageSnapshots: [snapshot]
    }
    const finalized = (await milestoneModule()).finalizeMilestoneAReport(report, {
      requiredCheckIds: ['ordinary-agent-workflow', 'real-repository-test']
    })
    expect(finalized.stageSnapshots).toEqual(expect.arrayContaining([
      expect.objectContaining({
        stage: 'ordinary-test-binding-failure',
        completedFields: expect.objectContaining({
          bashHostObservationReasonCode: 'not_observed',
          bashHostObservationTransportStatus: 404,
          bashHostObservationAttemptCount: 3
        })
      })
    ]))
    expect(finalized.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'ordinary-agent-workflow', status: 'failed' }),
      expect.objectContaining({ id: 'real-repository-test', status: 'skipped' })
    ]))

    const unsafe = sanitizeMilestoneAStageSnapshot({
      stage: 'ordinary-test-binding-failure',
      checkIds: ['ordinary-agent-workflow'],
      completedFields: {
        bashHostObservationReasonCode: 'PRIVATE_REASON',
        bashHostObservationTransportStatus: 404,
        bashHostObservationAttemptCount: 3,
        bashHostObservationDigest: 'a'.repeat(64)
      }
    })
    expect(unsafe?.completedFields).toEqual({
      bashHostObservationTransportStatus: 404,
      bashHostObservationAttemptCount: 3,
      bashHostObservationDigest: 'a'.repeat(64)
    })

    const outOfRange = sanitizeMilestoneAStageSnapshot({
      stage: 'ordinary-test-binding-failure',
      checkIds: ['ordinary-agent-workflow'],
      completedFields: {
        bashHostObservationTransportStatus: 600,
        bashHostObservationAttemptCount: 4,
        gitHostObservationTransportStatus: -1,
        gitHostObservationAttemptCount: 99
      }
    })
    expect(outOfRange?.completedFields).toEqual({})

    const defaults = (await milestoneModule()).finalizeMilestoneAReport({
      workflow: {},
      checks: [{ id: 'ordinary-agent-workflow', status: 'skipped' }]
    }, {
      requiredCheckIds: ['ordinary-agent-workflow']
    })
    expect(defaults.workflow).toEqual(expect.objectContaining({
      bashHostObservationReasonCode: 'not_observed',
      bashHostObservationTransportStatus: 0,
      bashHostObservationAttemptCount: 0,
      bashHostObservationDigest: '',
      bashHostAuthorityBindingDigest: '',
      gitHostObservationReasonCode: 'not_observed',
      gitHostObservationTransportStatus: 0,
      gitHostObservationAttemptCount: 0,
      gitHostObservationDigest: '',
      gitHostAuthorityBindingDigest: ''
    }))

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const skeletonStart = script.indexOf('function safeReportSkeleton')
    const runStart = script.indexOf('async function runMilestoneA', skeletonStart)
    const skeleton = script.slice(skeletonStart, runStart)
    const finalWorkflowStart = script.indexOf(
      'report.workflow = {\n      firstLaunchObserved',
      runStart
    )
    const finalWorkflowEnd = script.indexOf('\n    report.redaction = {', finalWorkflowStart)
    const finalWorkflow = script.slice(finalWorkflowStart, finalWorkflowEnd)
    for (const field of [
      'bashHostObservationReasonCode',
      'bashHostObservationTransportStatus',
      'bashHostObservationAttemptCount',
      'bashHostObservationDigest',
      'bashHostAuthorityBindingDigest',
      'gitHostObservationReasonCode',
      'gitHostObservationTransportStatus',
      'gitHostObservationAttemptCount',
      'gitHostObservationDigest',
      'gitHostAuthorityBindingDigest'
    ]) {
      expect(skeleton, `${field} skeleton default`).toContain(field)
      expect(finalWorkflow, `${field} final workflow`).toContain(field)
    }
  })

  it('initializes a bound Git baseline and accepts only the exact source repair', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      verifyMilestoneARepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const harnessRoot = join(root, 'harness-owned-validator')
    const authority = createMilestoneARepositoryAcceptance(workspace, harnessRoot)
    const baseline = verifyMilestoneARepositoryAcceptance(authority, 'baseline')

    expect(baseline).toEqual(expect.objectContaining({
      ok: true,
      gitRepository: true,
      baselineCommitBound: true,
      baselineTreeBound: true,
      protectedInputsBound: true,
      protectedInputsWriteProtected: true,
      validatorOutsideRepository: true,
      validatorBound: true,
      onlyIntendedSourceChanged: true,
      expectedSourceBound: true
    }))
    expect(relative(realpathSync(workspace), realpathSync(authority.validatorPath)))
      .toMatch(/^\.\.(?:\/|$)/)
    const topLevel = spawnSync('git', ['rev-parse', '--show-toplevel'], {
      cwd: workspace,
      encoding: 'utf8',
      stdio: 'pipe'
    })
    expect(topLevel.status).toBe(0)
    expect(topLevel.stdout.trim()).toBe(realpathSync(workspace))

    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )
    expect(verifyMilestoneARepositoryAcceptance(authority, 'final')).toEqual(
      expect.objectContaining({
        ok: true,
        protectedInputsBound: true,
        protectedInputsWriteProtected: true,
        onlyIntendedSourceChanged: true,
        expectedSourceBound: true
      })
    )
  })

  it('creates fresh private long-context markers without placing their values in the prompt or authority', async () => {
    const { createMilestoneARepositoryAcceptance } = await milestoneModule()
    const root = taskOwnedSandbox()
    const firstWorkspace = join(root, 'first-repository')
    const secondWorkspace = join(root, 'second-repository')
    const firstAuthority = createMilestoneARepositoryAcceptance(
      firstWorkspace,
      join(root, 'first-validator')
    )
    createMilestoneARepositoryAcceptance(
      secondWorkspace,
      join(root, 'second-validator')
    )
    const markerTokens = (workspace: string): string[] => readFileSync(
      join(workspace, 'docs', 'acceptance-context.txt'),
      'utf8'
    ).split(/\r?\n/).flatMap((line) => {
      const token = line.split(' ', 1)[0] || ''
      return /^MILESTONE_A_CONTEXT_(?:FIRST|OK|LAST)_[0-9a-f]{48}$/.test(token)
        ? [token]
        : []
    })
    const first = markerTokens(firstWorkspace)
    const second = markerTokens(secondWorkspace)

    expect(first).toHaveLength(3)
    expect(new Set(first).size).toBe(3)
    expect(second).toHaveLength(3)
    expect(first).not.toEqual(second)
    for (const marker of first) {
      expect(JSON.stringify(firstAuthority)).not.toContain(marker)
    }

    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const promptStart = script.indexOf('const contextPrompt = boundedContinuationPrompt({')
    const promptEnd = script.indexOf('const contextSubmit =', promptStart)
    const prompt = script.slice(promptStart, promptEnd)
    expect(prompt).not.toContain('contextMarkers.')
    expect(prompt).not.toContain('MILESTONE_A_CONTEXT_')
    expect(prompt).toContain('path: contextRelativePath')
    expect(prompt).toContain('limit: contextReadArguments.limit')
    expect(prompt).not.toContain('contextBinding.firstLineNumber')
    expect(prompt).not.toContain('contextBinding.completionLineNumber')
    expect(prompt).not.toContain('contextBinding.lastLineNumber')
    expect(prompt).not.toContain('contract-bound first, embedded completion, and last anchors')
    expect(prompt).not.toMatch(/\b(?:subagent|delegate|child|background)\b/i)
  }, 15_000)

  it('binds renderer recovery to the first result digest without repository content anchors', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const recoveryStart = script.indexOf('async function rendererVisibleRecoveryEvidence({')
    const recoveryEnd = script.indexOf('async function cdpNormalQuit', recoveryStart)
    const recovery = script.slice(recoveryStart, recoveryEnd)

    expect(recovery).toContain('expectedResultDigest')
    expect(recovery).toContain('renderer_recovery_result_digest_invalid')
    expect(recovery).not.toContain('expectedContextMarker')
    expect(recovery).not.toContain('contextMarker')
  })

  it('rejects protected-input tampering and any change beyond the intended source file', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      verifyMilestoneARepositoryAcceptance
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const authority = createMilestoneARepositoryAcceptance(
      workspace,
      join(root, 'harness-owned-validator')
    )
    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )
    writeFileSync(join(workspace, 'unexpected.txt'), 'not accepted\n', 'utf8')
    expect(verifyMilestoneARepositoryAcceptance(authority, 'final')).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'isolated_repository_unexpected_change',
        onlyIntendedSourceChanged: false
      })
    )

    const packagePath = join(workspace, 'package.json')
    chmodSync(packagePath, 0o600)
    writeFileSync(packagePath, '{}\n', 'utf8')
    expect(verifyMilestoneARepositoryAcceptance(authority, 'final')).toEqual(
      expect.objectContaining({
        ok: false,
        blocker: 'isolated_repository_protected_input_changed',
        protectedInputsBound: false
      })
    )
  })

  it('runs a parent-owned fixed npm test as cross-evidence without upgrading Agent invocation evidence', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      runParentOwnedRepositoryTest,
      testReceiptEvidence
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const authority = createMilestoneARepositoryAcceptance(
      workspace,
      join(root, 'harness-owned-validator')
    )
    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )

    const parentOwned = runParentOwnedRepositoryTest(authority)
    expect(parentOwned).toEqual(expect.objectContaining({
      passed: true,
      parentProcessOwned: true,
      command: 'npm test',
      commandDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      executableSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      environmentKeyDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      environmentDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      exitCode: 0,
      signal: '',
      repositoryStableBeforeAndAfter: true,
      sandboxCleanupSucceeded: true
    }))
    expect(existsSync(authority.receiptPath)).toBe(false)
    expect(testReceiptEvidence(authority, { turns: [] }, 'turn-unbound')).toEqual(
      expect.objectContaining({
        passed: false,
        count: 0,
        externalValidatorBound: false,
        protectedInputsBound: true,
        onlyIntendedSourceChanged: true,
        actualBashToolResultBound: false,
        hostOwnedToolInvocationBound: false,
        selfReportedReceiptAuthoritative: false,
        blocker: 'host_owned_tool_invocation_binding_unavailable'
      })
    )
  })

  it('keeps a synthetically parsed closed host observation non-authoritative', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      hostToolExecutionObservationEvidence,
      testReceiptEvidence
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const authority = createMilestoneARepositoryAcceptance(
      workspace,
      join(root, 'harness-owned-validator')
    )
    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )
    const expected = { threadId: 'thread-host', turnId: 'turn-host', toolName: 'bash' }
    const hostObservation = hostToolExecutionObservationEvidence(
      successfulHostToolObservationRaw(expected.threadId, expected.turnId, 'bash'),
      expected
    )
    const accepted = testReceiptEvidence(
      authority,
      { id: expected.threadId, turns: [] },
      expected.turnId,
      hostObservation
    )

    expect(hostObservation).toEqual(expect.objectContaining({
      ok: true,
      blocker: '',
      transportStatus: 200,
      toolName: 'bash',
      disclosure: 'metadata_only',
      privatePayloadWithheld: true,
      observationDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      authorityBindingDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(accepted).toEqual(expect.objectContaining({
      passed: false,
      count: 0,
      protectedInputsBound: true,
      onlyIntendedSourceChanged: true,
      actualBashToolResultBound: false,
      hostOwnedToolInvocationBound: false,
      selfReportedReceiptObserved: false,
      selfReportedReceiptAuthoritative: false,
      hostObservationDigest: '',
      hostAuthorityBindingDigest: '',
      blocker: 'host_owned_tool_invocation_binding_unavailable'
    }))
    const serialized = JSON.stringify({ hostObservation, accepted })
    for (const forbidden of [
      'npm test',
      'docs/acceptance-context.txt',
      workspace,
      'command',
      'arguments',
      'output',
      'account',
      'card'
    ]) {
      expect(serialized).not.toContain(forbidden)
    }

    expect(testReceiptEvidence(
      authority,
      { id: expected.threadId, turns: [] },
      expected.turnId,
      structuredClone(hostObservation)
    )).toEqual(expect.objectContaining({
      passed: false,
      actualBashToolResultBound: false,
      hostOwnedToolInvocationBound: false,
      blocker: 'host_owned_tool_invocation_binding_unavailable'
    }))

    writeFileSync(join(workspace, 'unexpected.txt'), 'repository drift\n', 'utf8')
    expect(testReceiptEvidence(
      authority,
      { id: expected.threadId, turns: [] },
      expected.turnId,
      hostObservation
    )).toEqual(expect.objectContaining({
      passed: false,
      actualBashToolResultBound: false,
      hostOwnedToolInvocationBound: false,
      blocker: 'isolated_repository_unexpected_change'
    }))
  })

  it('upgrades only evidence minted by the internal exact CDP bridge observation', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      hostToolExecutionObservationEvidence,
      observeHostToolExecution,
      privateHostToolExecutionBindingMatches,
      testReceiptEvidence
    } = await instrumentedMilestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const authority = createMilestoneARepositoryAcceptance(
      workspace,
      join(root, 'harness-owned-validator')
    )
    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )
    const expected = { threadId: 'thread-host', turnId: 'turn-host', toolName: 'bash' }
    const controlledDependencies = (
      toolName: 'bash' | 'read',
      threadId = expected.threadId,
      turnId = expected.turnId
    ) => ({
      waitForDebugTarget: async (debugPort: number, timeoutMs: number) => {
        expect(debugPort).toBe(43210)
        expect(timeoutMs).toBe(5000)
        return {
          webSocketDebuggerUrl: 'ws://milestone-a.invalid/devtools/page/1',
          pageTargetCount: 1
        }
      },
      evaluateReadonlyCdp: async (
        webSocketDebuggerUrl: string,
        expression: string,
        timeoutMs: number
      ) => {
        expect(webSocketDebuggerUrl).toBe(
          'ws://milestone-a.invalid/devtools/page/1'
        )
        expect(timeoutMs).toBe(5000)
        expect(expression).toContain('const api = window.analytix')
        expect(expression).toContain(`api.runtime.${FORMAL_RUNTIME_REQUEST_METHOD}`)
        expect(expression).toContain("'POST'")
        return successfulHostToolObservationRaw(threadId, turnId, toolName)
      },
      resolveWorkspaceRealPath: (value: string) => realpathSync(value)
    })
    const hostObservation = await observeHostToolExecution({
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: expected.toolName,
      workspace,
      arguments: { command: 'npm test' }
    }, controlledDependencies('bash'))
    const thread = { id: expected.threadId, turns: [] }
    const accepted = testReceiptEvidence(
      authority,
      thread,
      expected.turnId,
      hostObservation
    )

    expect(accepted).toEqual(expect.objectContaining({
      passed: true,
      count: 1,
      protectedInputsBound: true,
      onlyIntendedSourceChanged: true,
      actualBashToolResultBound: true,
      hostOwnedToolInvocationBound: true,
      hostObservationDigest: hostObservation.observationDigest,
      hostAuthorityBindingDigest: hostObservation.authorityBindingDigest,
      blocker: ''
    }))
    expect(privateHostToolExecutionBindingMatches(hostObservation, {
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: 'bash',
      workspace,
      arguments: { command: 'npm test' }
    })).toBe(true)

    for (const [name, binding] of Object.entries({
      thread: {
        threadId: 'thread-other',
        turnId: expected.turnId,
        toolName: 'bash',
        workspace,
        arguments: { command: 'npm test' }
      },
      turn: {
        threadId: expected.threadId,
        turnId: 'turn-other',
        toolName: 'bash',
        workspace,
        arguments: { command: 'npm test' }
      },
      arguments: {
        threadId: expected.threadId,
        turnId: expected.turnId,
        toolName: 'bash',
        workspace,
        arguments: { command: 'npm test -- --watch' }
      }
    })) {
      expect(privateHostToolExecutionBindingMatches(hostObservation, binding), name).toBe(false)
    }
    expect(testReceiptEvidence(
      authority,
      { id: 'thread-other', turns: [] },
      expected.turnId,
      hostObservation
    )).toEqual(expect.objectContaining({
      passed: false,
      hostOwnedToolInvocationBound: false
    }))
    expect(testReceiptEvidence(
      authority,
      thread,
      'turn-other',
      hostObservation
    )).toEqual(expect.objectContaining({
      passed: false,
      hostOwnedToolInvocationBound: false
    }))
    expect(testReceiptEvidence(
      authority,
      thread,
      expected.turnId,
      structuredClone(hostObservation)
    )).toEqual(expect.objectContaining({
      passed: false,
      hostOwnedToolInvocationBound: false
    }))

    const otherWorkspace = join(root, 'other-workspace')
    mkdirSync(otherWorkspace, { recursive: true })
    expect(privateHostToolExecutionBindingMatches(hostObservation, {
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: 'bash',
      workspace: otherWorkspace,
      arguments: { command: 'npm test' }
    })).toBe(false)
    const wrongWorkspaceObservation = await observeHostToolExecution({
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: 'bash',
      workspace: otherWorkspace,
      arguments: { command: 'npm test' }
    }, controlledDependencies('bash'))
    expect(testReceiptEvidence(
      authority,
      thread,
      expected.turnId,
      wrongWorkspaceObservation
    )).toEqual(expect.objectContaining({
      passed: false,
      hostOwnedToolInvocationBound: false
    }))

    const wrongArgumentsObservation = await observeHostToolExecution({
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: 'bash',
      workspace,
      arguments: { command: 'npm test -- --watch' }
    }, controlledDependencies('bash'))
    expect(wrongArgumentsObservation).toEqual(expect.objectContaining({
      ok: false,
      observationDigest: '',
      authorityBindingDigest: ''
    }))
    expect(testReceiptEvidence(
      authority,
      thread,
      expected.turnId,
      wrongArgumentsObservation
    )).toEqual(expect.objectContaining({
      passed: false,
      hostOwnedToolInvocationBound: false
    }))

    const syntheticObservation = hostToolExecutionObservationEvidence(
      successfulHostToolObservationRaw(expected.threadId, expected.turnId, 'bash'),
      expected
    )
    expect(privateHostToolExecutionBindingMatches(syntheticObservation, {
      threadId: expected.threadId,
      turnId: expected.turnId,
      toolName: 'bash',
      workspace,
      arguments: { command: 'npm test' }
    })).toBe(false)

    const readTurnId = 'turn-context'
    const readObservation = await observeHostToolExecution({
      debugPort: 43210,
      timeoutMs: 5000,
      threadId: expected.threadId,
      turnId: readTurnId,
      toolName: 'read',
      workspace,
      arguments: { path: 'docs/acceptance-context.txt' }
    }, controlledDependencies('read', expected.threadId, readTurnId))
    expect(privateHostToolExecutionBindingMatches(readObservation, {
      threadId: expected.threadId,
      turnId: readTurnId,
      toolName: 'read',
      workspace,
      arguments: { path: 'docs/acceptance-context.txt' }
    })).toBe(true)
    expect(privateHostToolExecutionBindingMatches(readObservation, {
      threadId: expected.threadId,
      turnId: readTurnId,
      toolName: 'read',
      workspace,
      arguments: { path: 'docs/other.txt' }
    })).toBe(false)

    const serialized = JSON.stringify({ hostObservation, accepted, readObservation })
    for (const forbidden of [
      'npm test',
      'docs/acceptance-context.txt',
      workspace,
      expected.threadId,
      expected.turnId,
      'command',
      'arguments',
      'output',
      'account',
      'card'
    ]) {
      expect(serialized).not.toContain(forbidden)
    }
  })

  it('separately validates the exact long-context read observation without retaining its path', async () => {
    const { hostToolExecutionObservationEvidence } = await milestoneModule()
    const expected = { threadId: 'thread-context', turnId: 'turn-context', toolName: 'read' }
    const readObservation = hostToolExecutionObservationEvidence(
      successfulHostToolObservationRaw(expected.threadId, expected.turnId, 'read'),
      expected
    )
    expect(readObservation).toEqual(expect.objectContaining({
      ok: true,
      toolName: 'read',
      disclosure: 'metadata_only',
      privatePayloadWithheld: true,
      observationDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      authorityBindingDigest: expect.stringMatching(/^[0-9a-f]{64}$/)
    }))
    expect(JSON.stringify(readObservation)).not.toContain('acceptance-context.txt')
    expect(JSON.stringify(readObservation)).not.toContain('path')

    const withExtraField = successfulHostToolObservationRaw(
      expected.threadId,
      expected.turnId,
      'read'
    ) as { observation: Record<string, unknown> }
    withExtraField.observation.output = 'private-output'
    expect(hostToolExecutionObservationEvidence(withExtraField, expected)).toEqual(
      expect.objectContaining({
        ok: false,
        observationDigest: '',
        authorityBindingDigest: ''
      })
    )
  })

  it('rejects a complete forged receipt even when every self-reported field and timestamp correlate', async () => {
    const {
      createMilestoneARepositoryAcceptance,
      testReceiptEvidence
    } = await milestoneModule()
    const root = taskOwnedSandbox()
    const workspace = join(root, 'ordinary-repository')
    const authority = createMilestoneARepositoryAcceptance(
      workspace,
      join(root, 'harness-owned-validator')
    )
    writeFileSync(
      join(workspace, 'src', 'counter.mjs'),
      'export function add(left, right) {\n  return left + right\n}\n',
      'utf8'
    )
    const startedAt = new Date().toISOString()
    const completedAt = new Date(Date.now() + 10).toISOString()
    writeFileSync(authority.receiptPath, JSON.stringify({
      contract: 'analytix.milestone-a.external-test-receipt.v1',
      marker: 'MILESTONE_A_REAL_TEST_EXECUTED',
      nonce: authority.nonce,
      workspaceSha256: authority.workspaceSha256,
      baselineCommit: authority.baselineCommit,
      baselineTree: authority.baselineTree,
      protectedInputsDigest: authority.protectedInputsDigest,
      expectedSourceSha256: authority.expectedSourceSha256,
      observedSourceSha256: authority.expectedSourceSha256,
      validatorSha256: authority.validatorSha256,
      npmLifecycleEvent: 'test',
      npmLifecycleScriptSha256: authority.testScriptSha256,
      npmCommand: 'test',
      npmPackageName: 'analytix-milestone-a-isolated-repo',
      npmTestAncestorObserved: true,
      packageScriptParentObserved: true,
      gitStatusSha256: authority.expectedStatusSha256,
      assertionCount: 3,
      startedAt,
      completedAt,
      ancestorChainDigest: 'f'.repeat(64)
    }) + '\n', 'utf8')
    const thread = {
      turns: [{
        id: 'turn-forged',
        items: [{
          kind: 'tool_call',
          status: 'completed',
          callId: 'call-forged',
          toolName: 'bash',
          toolKind: 'command_execution',
          createdAt: new Date(Date.parse(startedAt) - 1000).toISOString(),
          finishedAt: new Date(Date.parse(completedAt) + 1000).toISOString()
        }, {
          kind: 'tool_result',
          status: 'completed',
          callId: 'call-forged',
          toolName: 'bash',
          toolKind: 'command_execution',
          isError: false,
          createdAt: new Date(Date.parse(startedAt) - 1000).toISOString(),
          finishedAt: new Date(Date.parse(completedAt) + 1000).toISOString(),
          output: { status: 'completed' }
        }]
      }]
    }

    expect(testReceiptEvidence(authority, thread, 'turn-forged')).toEqual(
      expect.objectContaining({
        passed: false,
        count: 0,
        externalValidatorBound: false,
        actualBashToolResultBound: false,
        hostOwnedToolInvocationBound: false,
        selfReportedReceiptObserved: true,
        selfReportedReceiptShapeValid: true,
        selfReportedReceiptAuthoritative: false,
        untrustedTimingCorrelationObserved: true,
        bashExecutionCount: 1,
        blocker: 'host_owned_tool_invocation_binding_unavailable'
      })
    )
  })

  it('rejects caller-controlled self-trust for formal release publication evidence', async () => {
    const {
      releasePublicationConfiguration,
      verifyPackagedReleasePublicationAuthority
    } = await import(pathToFileURL(join(
      process.cwd(),
      'scripts/lib/packaged-release-publication-authority.mjs'
    )).href)
    const config = releasePublicationConfiguration([], {
      ANALYTIX_RELEASE_AUTHORITY: '/controlled/release-authority.json',
      ANALYTIX_RELEASE_AUTHORITY_PUBLIC_KEY: '/controlled/release-authority.pub',
      ANALYTIX_RELEASE_AUTHORITY_TRUSTED_PUBLIC_KEY_SHA256: 'f'.repeat(64),
      ANALYTIX_RELEASE_TAG: 'v1.2.3',
      ANALYTIX_RELEASE_CHANNEL: 'stable',
      ANALYTIX_RELEASE_DIST: '/controlled/dist'
    })
    expect(config).not.toHaveProperty('trustedPublicKeySha256')
    await expect(verifyPackagedReleasePublicationAuthority({
      requested: true,
      target: { platform: 'win32', key: 'win32-x64' },
      candidate: {
        ok: true,
        sha256: 'a'.repeat(64),
        authorityDigest: 'b'.repeat(64),
        sourceCommit: 'c'.repeat(40),
        targetKey: 'win32-x64'
      },
      config
    })).resolves.toEqual(expect.objectContaining({
      ok: false,
      blocked: true,
      failed: false,
      classification: 'external_prerequisite',
      blocker: 'release_publication_authority_trust_anchor_unavailable'
    }))
  })

  it('keeps functional Milestone A independent from commercial publication while the aggregate release gate stays closed', () => {
    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const qa = source('scripts/runtime-go-packaged-qa.mjs')
    const collector = source('scripts/runtime-go-live-evidence-collector.mjs')
    const functionalIds = milestone.slice(
      milestone.indexOf('const REQUIRED_CHECK_IDS'),
      milestone.indexOf('const REQUIRED_COMMERCIAL_RELEASE_CHECK_IDS')
    )
    const prerequisites = milestone.slice(
      milestone.indexOf('const prerequisites = ['),
      milestone.indexOf('report.checks.push(...prerequisites)')
    )
    const artifactStart = milestone.indexOf('function formalPackagedArtifactEvidence')
    const artifactVerdict = milestone.slice(
      milestone.indexOf('const valid =', artifactStart),
      milestone.indexOf('return {', milestone.indexOf('const valid =', artifactStart))
    )
    const desktopPredicate = qa.slice(
      qa.indexOf('const actualPackagedDesktopQAPassed'),
      qa.indexOf('const actualPackagedAppLaunched')
    )

    expect(functionalIds).not.toContain('formal-release-publication-authority')
    expect(functionalIds).not.toContain('controlled-release-native-receipt')
    expect(prerequisites).not.toContain('formal-release-publication-authority')
    expect(prerequisites).not.toContain('controlled-release-native-receipt')
    expect(artifactVerdict).not.toContain('controlledReleaseNativeReceipt')
    expect(desktopPredicate).not.toContain('releasePublicationAuthority')
    expect(desktopPredicate).not.toContain('controlledReleaseNativeReceipt')
    expect(milestone).toContain('requiredForMilestoneA: false')
    expect(milestone).toContain('requiredForCommercialPublication: true')
    expect(milestone).toContain("'controlled-release-native-receipt'")
    expect(qa).toContain('checks.push(...commercialReleaseChecks)')
    expect(qa).toContain('commercialReleaseReady')
    expect(qa).toContain("missingFinalCoverage.add(id)")
    expect(collector).toContain('packaged.commercialReleaseReady')
    expect(collector).toContain('packaged.formalReleasePublicationAuthorityPassed')
    expect(collector).toContain('packaged.controlledReleaseNativeReceiptPassed')
  })

  it('requires visible recovery and proves protected funds denial is per-effect and additive', () => {
    const milestone = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const collector = source('scripts/runtime-go-live-evidence-collector.mjs')

    for (const required of [
      'privateAuthorityInputsAbsent',
      'preLaunchUnexpectedUserDataEntryCount',
      'firstLaunchFundsExecutionUnavailable',
      'secondLaunchFundsExecutionUnavailable',
      'ordinaryWorkflowCompleted',
      'unavailableWhileOrdinaryCapabilitiesRetained',
      'rendererVisibleThreadRecovered',
      'rendererVisibleTodosRecovered',
      'rendererVisibleSubagentRecovered',
      'rendererVisibleCompactionRecovered',
      'rendererVisibleResultRecovered'
    ]) {
      expect(milestone).toContain(required)
      expect(collector).toContain(required)
    }
    for (const required of [
      'bundledFundsMissingAuthorityPreconditionEstablished',
      'firstLaunchBundledFundsMaterializationFailureObserved',
      'secondLaunchBundledFundsMaterializationFailureObserved',
      'fundsTransportAbsentOnBothLaunches',
      'fundsDiagnosticsIndependent',
      'protectedFundsSourceUnavailable',
      'protectedFundsAcceptedFinalDigest',
      'protectedFundsClaimCount',
      'protectedFundsReceiptCount',
      'protectedFundsSuccessfulExecutionCount',
      'restartedProtectedFundsSourceUnavailable',
      'restartedProtectedFundsAcceptedFinalDigest',
      'ordinaryCandidateTerminalReasonClass',
      'ordinaryCandidateTerminalErrorItemCount',
      'ordinaryCandidateTerminalErrorItemAuthorityBound',
      'ordinaryCandidateToolInventoryAvailability',
      'ordinaryCandidateProviderReceiptAvailability',
      'ordinaryContinuedAfterProtectedFundsBlock',
      'ordinaryContinuedAfterRestart'
    ]) {
      expect(milestone).toContain(required)
    }
    expect(milestone.match(/PROTECTED_FUNDS_REQUEST_PROMPT/g)).toHaveLength(2)
    expect(milestone).toContain("timelineText.includes('Compacted context')")
    expect(milestone).toContain("timelineText.includes('已压缩上下文')")
    expect(milestone).toContain("label === 'Done' || label === '完成'")
    expect(milestone).toContain("[role=\"tablist\"][aria-label=\"子智能体\"]")
    const additiveGate = milestone.slice(
      milestone.lastIndexOf('const caseDependenciesUnavailableWithOrdinaryCapabilities ='),
      milestone.lastIndexOf("report.checks.push(check(\n      'case-dependencies-unavailable-with-ordinary-capabilities'")
    )
    expect(additiveGate).toContain(
      'bundledFundsMaterializationSeam?.evidence?.preconditionEstablished === true'
    )
    expect(additiveGate).toContain('firstFundsMaterializationFailure.ok === true')
    expect(additiveGate).toContain('secondFundsMaterializationFailure.ok === true')
    expect(additiveGate).toContain('fundsServerDiagnosticCount === 0')
    expect(additiveGate).toContain('protectedFundsUnavailable.ok === true')
    expect(additiveGate).toContain(
      'restartedProtectedFundsUnavailable.acceptedFinalDigest ==='
    )
    expect(additiveGate).toContain('ordinaryContinuedAfterProtectedFundsBlock')
    expect(additiveGate).toContain('relaunchProviderContinuationObserved')
    expect(additiveGate).not.toContain('firstFundsExecutionUnavailable')
    expect(additiveGate).not.toContain('secondFundsExecutionUnavailable')
    expect(additiveGate).not.toContain('fundsExecutionUnavailable')
    expect(milestone).not.toContain('.click(')
  })

  it('dry-run fails closed and binds the exact current harness bytes', () => {
    const script = source('scripts/runtime-go-packaged-milestone-a.mjs')
    const result = spawnSync(process.execPath, [
      milestoneScriptPath,
      '--dry-run',
      '--json',
      '--no-write',
      '--no-gate'
    ], {
      cwd: process.cwd(),
      env: milestoneCliEnvironment(),
      encoding: 'utf8',
      stdio: 'pipe'
    })

    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    const report = JSON.parse(result.stdout) as Record<string, any>
    expect(report).toEqual(expect.objectContaining({
      id: 'runtime-go-packaged-milestone-a',
      stage: 'packaged-general-agent-milestone-a',
      status: 'live_blocked',
      passed: false,
      syntheticProviderUsed: false,
      localProviderUsed: false,
      directRuntimeTurnDriverUsed: false,
      credentialSecretsRecorded: false
    }))
    expect(report.provider).toEqual(expect.objectContaining({
      credentialAuthority: 'local-provider-registry-secret-store',
      normalLocalProviderSetupObserved: false,
      credentialConfigured: false,
      credentialRecorded: false
    }))
    expect(report.operatorCheckpoint).toEqual({
      kind: 'visible-normal-local-provider-setup',
      required: true,
      declared: false,
      method: 'undeclared',
      automatedCredentialEntryUsed: null,
      status: 'pending'
    })
    for (const launch of ['firstLaunch', 'secondLaunch']) {
      expect(report.publicSeams[launch]).toEqual(expect.objectContaining({
        runtimeServerProcessCount: 0,
        desktopPrivateHistoryMigrationProcessCount: 0,
        bundledPluginMaterializationProcessCount: 0,
        unknownRuntimeProcessCount: 0,
        rendererReadFailureObserved: false,
        rendererReadFailureStage: 'unknown',
        rendererReadFailureTargetCount: 0,
        startupTrace: {
          enabled: false,
          checkpointCount: 0,
          observedCheckpointCodes: [],
          lastCheckpoint: 'not_observed',
          lastElapsedMs: 0,
          checkpointElapsedMs: {}
        }
      }))
    }
    expect(report.repository).toEqual(expect.objectContaining({
      required: true,
      mode: 'external-preexisting-real-repository',
      configured: false,
      baselineValidated: false,
      finalValidated: false,
      blocker: 'external_repository_path_contract_provenance_and_owner_root_required'
    }))
    expect(report.executionBlocker).toBe(
      'external_repository_path_contract_provenance_and_owner_root_required'
    )
    expect(report.harness).toEqual(expect.objectContaining({
      scriptSha256: sha256(script),
      contractManifestSha256: expect.stringMatching(/^[0-9a-f]{64}$/),
      contractManifestBound: true,
      harnessSourceRootBound: true,
      sourceClosureBound: false,
      sourceClosureSnapshotDigest: expect.stringMatching(/^[0-9a-f]{64}$/),
      packagedSourceClosureSnapshotDigest: ''
    }))
    expect(report.harness.entries.map((item: { name: string }) => item.name)).toEqual([
      'runtime-go-packaged-milestone-a.mjs',
      'local-provider-acceptance.mjs',
      'local-provider-credential-scan.mjs',
      'runtime-go-packaged-qa.mjs',
      'runtime-go-live-evidence-collector.mjs',
      'runtime-go-validation-command.mjs',
      'runtime-go-cutover-report.mjs',
      'runtime-go-packaged-gui-smoke.mjs',
      'runtime-go-packaged-session-soak.mjs',
      'runtime-go-rollback-retirement-evidence.mjs',
      'packaging-config.test.ts',
      'after-pack.cjs',
      'packaged-release-publication-authority.mjs',
      'publish-r2.mjs',
      'strict-json.cjs',
      'macos-signing-policy.cjs',
      'macos-signing-policy.json',
      'entitlements.mac.native-helper.plist',
      'package.json',
      'package-lock.json',
      'scripts/use-analytix-cache.sh',
      'scripts/analytix-cache-storage.zsh'
    ])
    for (const relativePath of [
      'scripts/use-analytix-cache.sh',
      'scripts/analytix-cache-storage.zsh'
    ]) {
      expect(report.harness.entries).toContainEqual({
        name: relativePath,
        regular: true,
        byteLength: Buffer.byteLength(source(relativePath)),
        sha256: sha256(source(relativePath))
      })
    }
    expect(report.harness.entries).toEqual(expect.arrayContaining([
      expect.objectContaining({
        name: 'after-pack.cjs',
        regular: true,
        byteLength: expect.any(Number),
        sha256: expect.stringMatching(/^[0-9a-f]{64}$/)
      }),
      expect.objectContaining({
        name: 'packaged-release-publication-authority.mjs',
        regular: true,
        byteLength: expect.any(Number),
        sha256: expect.stringMatching(/^[0-9a-f]{64}$/)
      }),
      expect.objectContaining({
        name: 'publish-r2.mjs',
        regular: true,
        byteLength: expect.any(Number),
        sha256: expect.stringMatching(/^[0-9a-f]{64}$/)
      })
    ]))
    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'configured-network-provider', status: 'live_blocked' }),
      expect.objectContaining({ id: 'packaged-first-launch', status: 'live_blocked' }),
      expect.objectContaining({ id: 'composer-workflow-submit', status: 'live_blocked' }),
      expect.objectContaining({ id: 'real-repository-test', status: 'live_blocked' }),
      expect.objectContaining({ id: 'bounded-subagent', status: 'live_blocked' }),
      expect.objectContaining({
        id: 'case-dependencies-unavailable-with-ordinary-capabilities',
        status: 'live_blocked'
      }),
      expect.objectContaining({ id: 'long-context-continuation', status: 'live_blocked' }),
      expect.objectContaining({ id: 'normal-first-quit', status: 'live_blocked' }),
      expect.objectContaining({ id: 'exact-result-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'renderer-visible-thread-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'renderer-visible-todo-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'renderer-visible-subagent-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'renderer-visible-compaction-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'renderer-visible-result-recovery', status: 'live_blocked' }),
      expect.objectContaining({ id: 'normal-final-quit', status: 'live_blocked' }),
      expect.objectContaining({ id: 'sandbox-cleanup', status: 'live_blocked' })
    ]))
    expect(report.checks).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: 'external-real-repository-contract' }),
      expect.objectContaining({ id: 'trusted-cache-tmpdir' }),
      expect.objectContaining({ id: 'formal-packaged-artifact' })
    ]))
    expect(report.commercialRelease).toEqual(expect.objectContaining({
      requiredForMilestoneA: false,
      requiredForCommercialPublication: true,
      status: 'unverified',
      passed: false
    }))
  })

  it('keeps synthetic repository provenance parser-only at the formal CLI boundary', async () => {
    const root = taskOwnedSandbox()
    const input = externalRepositoryAcceptanceFixture(root)
    const { parseMilestoneAExternalRepositoryProvenance } = await milestoneModule()
    expect(parseMilestoneAExternalRepositoryProvenance(input.provenance)).toEqual(
      input.provenance
    )
    expect(parseMilestoneAExternalRepositoryProvenance({
      ...input.provenance,
      originUrl: 'https://user:secret@example.com/repository.git'
    })).toBeNull()
    const result = spawnSync(process.execPath, [
      milestoneScriptPath,
      '--dry-run',
      '--json',
      '--no-write',
      '--no-gate',
      '--repository-path',
      input.workspace,
      '--repository-contract',
      input.contractPath
    ], {
      cwd: process.cwd(),
      env: process.env,
      encoding: 'utf8',
      stdio: 'pipe'
    })

    expect(result.status).toBe(0)
    expect(result.stderr).toBe('')
    const report = JSON.parse(result.stdout) as Record<string, any>
    expect(report.repository).toEqual(expect.objectContaining({
      required: true,
      configured: false,
      validated: false,
      baselineValidated: false,
      finalValidated: false,
      fixtureSubstituteRejected: false,
      formalAcceptance: false,
      freshOriginVerified: false,
      blocker: 'external_repository_path_contract_provenance_and_owner_root_required'
    }))
    expect(report.executionBlocker).toBe(
      'external_repository_path_contract_provenance_and_owner_root_required'
    )
  }, 30_000)
})


it('R130-F1 binds A0 first-ready to the actual entry completion and rejects same-id K2 replacement', async () => {
  const { milestoneALocalProvider } = await milestoneModule()
  // @ts-expect-error The private harness module is JavaScript.
  const scanner = await import('../../scripts/lib/local-provider-credential-scan.mjs')
  const model = 'deepseek-v4-flash'
  const provider = { id: 'deepseek', kind: 'deepseek', endpoint: 'https://api.example.invalid/v1',
    models: [model], mediaModels: [], selectedModel: model, selectedRoutes: [],
    credentialConfigured: true, credentialPurpose: 'api_key', revision: '1',
    generation: '1', incarnation: 'inc_' + 'a'.repeat(43), tombstone: false }
  const observation = {
    providerRegistry: { schemaVersion: 1, registryRevision: '1',
      registryIncarnation: 'inc_' + 'b'.repeat(43), selectedProviderId: provider.id, providers: [provider] },
    settings: {
      provider: { apiKey: '', activeProviderId: provider.id, providers: [{
        id: provider.id, apiKey: '', baseUrl: provider.endpoint, endpointFormat: 'chat_completions',
        models: [model], modelProfiles: { [model]: { contextWindowTokens: 131072 } }
      }] },
      runtime: { providerId: provider.id, model, apiKey: '', runtimeToken: '' }
    },
    runtimeInfo: { provider: { id: provider.id, model, endpointFormat: 'chat_completions',
      available: true, apiKeyConfigured: true, baseUrlConfigured: true, contextWindowTokens: 131072 } }
  }
  expect(milestoneALocalProvider(observation)).toMatchObject({
    ok: true, credentialAuthorityBound: true, contextWindowTokensBound: true
  })
  const credential = Buffer.from('synthetic-A0-K1-entry')
  const context = { runId: 'a0-synthetic', entryAttemptId: 'a0-entry' }
  const handle = await scanner.captureLocalCredentialEntry({ ...context, providerId: provider.id,
    entryMethod: 'visible-computer-use', credential }, async () => ({
    completed: true, providerRegistry: observation.providerRegistry
  }))
  try {
    expect(scanner.readLocalCredentialScanSource(handle, { ...context,
      provider: milestoneALocalProvider(observation) }).ok).toBe(true)
    observation.providerRegistry.registryRevision = '2'
    provider.revision = '2'
    provider.generation = '2'
    const firstReady = milestoneALocalProvider(observation)
    expect(firstReady.ok).toBe(true)
    expect(scanner.readLocalCredentialScanSource(handle, { ...context, provider: firstReady }))
      .toMatchObject({ ok: false, sourceBound: false, sourceSecretCount: 0 })
  } finally {
    scanner.disposeLocalCredentialScanSource(handle)
    credential.fill(0)
  }
})

it('R130 rejects same-id authority replacement and endpoint-path drift on recovery', async () => {
  // @ts-expect-error The production acceptance helper is JavaScript.
  const { sameLocalProviderAuthority } = await import('../../scripts/lib/local-provider-acceptance.mjs')
  const before = { ok: true, id: 'deepseek', model: 'deepseek-v4-flash',
    baseUrl: 'https://api.example.invalid/v1', endpointFormat: 'chat_completions',
    registryRevision: '2', registryIncarnation: 'inc_' + 'a'.repeat(43),
    providerRevision: '2', providerGeneration: '1', providerIncarnation: 'inc_' + 'b'.repeat(43) }
  expect(sameLocalProviderAuthority(before, { ...before })).toBe(true)
  for (const key of ['id', 'model', 'baseUrl', 'endpointFormat', 'registryRevision',
    'registryIncarnation', 'providerRevision', 'providerGeneration', 'providerIncarnation']) {
    expect(sameLocalProviderAuthority(before, { ...before, [key]: before[key as keyof typeof before] + '-changed' })).toBe(false)
  }
  expect(sameLocalProviderAuthority(before, { ...before, baseUrl: 'https://api.example.invalid/other' })).toBe(false)
  expect(sameLocalProviderAuthority(before, { ...before, ok: false })).toBe(false)
  expect(source('scripts/runtime-go-packaged-milestone-a.mjs'))
    .toContain('sameLocalProviderAuthority(provider, secondProvider)')
})
