import { app } from 'electron'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { createHash, randomBytes } from 'node:crypto'
import {
  chmodSync,
  closeSync,
  existsSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readdirSync,
  readFileSync,
  realpathSync,
  renameSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { mkdtemp, rm } from 'node:fs/promises'
import { homedir, tmpdir } from 'node:os'
import { delimiter, dirname, join, resolve } from 'node:path'
import { z } from 'zod'
import {
  DEFAULT_ANALYTIX_DATA_DIR,
  getAnalytixRuntimeSettings,
  isAnalytixRuntimeInsecure,
  resolveAnalytixRuntimeSettings,
  type AppSettingsV1
} from '../../shared/app-settings'
import {
  ANALYTIX_PROVIDER_REGISTRY_PATH,
  ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH,
  ANALYTIX_PROVIDER_REGISTRY_RECOVER_PATH,
  ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH
} from '../../shared/analytix-endpoints'
import { redactSecretText } from '../../shared/secret-redaction'
import {
  containsPrivateAcceptedFinalAuthority,
  containsPrivateRuntimeDiagnosticContent,
  containsPrivateReasoningContent,
  PublicRuntimeEventFilter,
  sanitizePublicAssistantText,
  sanitizePublicSerializedText,
  sanitizePublicRuntimeValue
} from '../../shared/public-runtime-content'
import {
  projectPublicRuntimeSseBlock,
  takePublicRuntimeSseBlock
} from '../../shared/public-runtime-sse'
import { projectPublicRuntimeHTTPError } from '../../shared/runtime-error'
import {
  buildAnalytixServeArgs,
  resolveAnalytixExecutable
} from '../resolve-analytix-binary'
import {
  reclaimAnalytixPort,
  resolveAnalytixDataDir,
  resolveAvailableAnalytixPort,
  syncGuiManagedAnalytixConfig,
  type AnalytixUnexpectedExitInfo
} from '../analytix-process'
import { getAnalytixBaseUrl } from '../analytix-base-url'
import type { RuntimeHostScheduleMcpBindingV1 } from '../claw-schedule-mcp-config'
import type { DesktopExternalStateBoundary } from '../desktop-external-state-isolation'
import { validOrdinaryResultSlotV1 } from '../general-terminal-publication'
import { logWarn } from '../logger'
import {
  bindBundledFundsMaterializationToCurrentRuntimeV1,
  clearBundledFundsMaterializationCurrentRuntimeV1,
  materializeBundledFundsBeforeRuntimeV1,
  type BundledFundsMaterializationBindingV1
} from './bundled-funds-materialization'
import {
  authorityAnchorProjectionV1,
  takeMainOwnedRuntimeAuthorityEnvelopeV1,
  type MainOwnedRuntimeAuthorityEnvelopeV1
} from './main-owned-authority-envelope-v1'
import {
  isOwnedProcessRunning,
  ownSpawnedProcess,
  stopOwnedProcess,
  type OwnedProcessHandle
} from './owned-process'
import {
  selectRuntimeHostScheduleMcpBindingV1,
  writeRuntimeStartupPrivateFrameV1
} from './runtime-startup-private-frame-v1'
import {
  resolveDarwinSecretStoreKeychainBindingV1
} from './darwin-secret-store-keychain-binding-v1'
import { isAnalytixHealthResponseBody } from '../analytix-health'
import {
  TaskJobKillResponseV1Schema,
  TaskJobListResponseV1Schema,
  TaskJobOutputResponseV1Schema,
  TaskJobWaitResponseV1Schema,
  threadSummaryTaskIdentityMatchesV1,
  ThreadSummaryTaskOutputResponseV1Schema
} from '../../../packages/runtime/src/contracts/task-job-output.js'
import {
  AttachmentContentResponse,
  AttachmentMetadataResponse,
  AttachmentUploadResponse
} from '../../../packages/runtime/src/contracts/attachments.js'
import { RuntimeInfoResponse as RuntimeInfoResponseSchema } from '../../../packages/runtime/src/contracts/runtime-info.js'
import {
  AttachmentDiagnosticsResponseV2 as AttachmentDiagnosticsResponseSchema,
  MemoryDiagnosticsResponseV2 as MemoryDiagnosticsResponseSchema,
  RuntimeToolsResponse as RuntimeToolsResponseSchema
} from '../../../packages/runtime/src/contracts/runtime-tools.js'
import { RuntimeSkillsResponseV2 as RuntimeSkillsResponseSchema } from '../../../packages/runtime/src/contracts/runtime-skills.js'
import {
  SuccessfulToolExecutionObservationV1 as SuccessfulToolExecutionObservationResponseV1Schema
} from '../../../packages/runtime/src/contracts/tool-execution-observation.js'
import {
  AcceptedFinalPublicAssistantTextTurnItemV3,
  AcceptedFinalPublicViewV3Schema,
  AssistantTextTurnItem
} from '../../../packages/runtime/src/contracts/items.js'
import { ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1 } from '../../../packages/runtime/src/contracts/events.js'
import {
  ClearThreadTodosResponse,
  ThreadSummaryResponse as ThreadSummaryResponseSchema,
  ThreadSummaryTaskMutationResponse as ThreadSummaryTaskMutationResponseSchema,
  DeleteThreadResponse,
  ListThreadsResponse,
  ThreadSchema,
  ThreadTodosResponse
} from '../../../packages/runtime/src/contracts/threads.js'
import { ThreadDetailResponseV1Schema } from '../../../packages/runtime/src/contracts/thread-detail.js'
import { MemoryRecord as MemoryRecordSchema } from '../../../packages/runtime/src/contracts/memory.js'
import {
  CaseProjectDetailResponseV1Schema,
  CaseProjectListResponseV1Schema,
  CaseProjectThreadsResponseV1Schema
} from '../../../packages/runtime/src/contracts/case-projects.js'
import {
  InterruptTurnResponse,
  CompactResponse,
  RewindThreadResponse,
  StartTurnResponse,
  SteerTurnResponse
} from '../../../packages/runtime/src/contracts/turns.js'
import { StartReviewResponse } from '../../../packages/runtime/src/contracts/review.js'
import { ApprovalDecisionResponse } from '../../../packages/runtime/src/contracts/approvals.js'
import {
  DailyUsageResponseSchema,
  ModelUsageResponseSchema,
  RuntimeUsageResponseSchema,
  ThreadUsageResponseSchema
} from '../../../packages/runtime/src/contracts/usage.js'
import {
  providerRegistryAccountObservationResponseSchemaV1,
  providerRegistryDeletedResponseSchemaV1,
  providerRegistryFailureSchemaV1,
  providerRegistryPortableManifestExportResponseSchemaV1,
  providerRegistryPortableManifestImportResponseSchemaV1,
  providerRegistryProbeResponseSchemaV1,
  providerRegistryProviderResponseSchemaV1,
  providerRegistryRecoveredResponseSchemaV1,
  providerRegistrySnapshotResponseSchemaV1
} from '../../../packages/runtime/src/contracts/provider-registry.js'
import {
  CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
  CONTROLLED_ARTIFACT_HOST_V2_URL_ENV
} from '../controlled-artifact/host-v2'
import { parseStrictJsonObject } from '../controlled-artifact/strict-json'
import {
  sha256Hex,
  verifiedAcceptedFinalDeliveryBatch,
  verifyAcceptedFinalRecordIntegrity,
  type FinalPublicationAuthorityPinV1
} from '../accepted-final-publication'

const ANALYTIX_RUNTIME_ID = 'analytix' as const
const GO_CONFORMANCE_READY_PREFIX = 'ANALYTIX_SIDECAR_READY '
const GO_RUNTIME_SERVER_READY_PREFIX = 'ANALYTIX_RUNTIME_SERVER_READY '
const DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2 = 'ANALYTIX_DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2\n'
const GO_CONFORMANCE_STARTUP_TIMEOUT_MS = 60_000
const MAX_GO_READY_STDOUT_BYTES = 64 << 10
const MAX_GO_MIGRATION_OUTPUT_BYTES = 4 << 10
// The private-history barrier hashes and validates the complete managed
// persistence inventory. Real desktop histories can take materially longer
// than a normal runtime probe, so keep this bounded independently from the
// short process-start deadlines.
export const DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2 = 30 * 60_000
// Production runtime composition repeats the same authenticated semantic
// startup work before it can issue its ready line. Do not apply the short
// conformance-sidecar deadline to that persistence-bound path.
export const GO_RUNTIME_SERVER_STARTUP_TIMEOUT_MS_V1 = DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2
const GO_RUNTIME_CANDIDATE_DURABLE_ROOT_MARKER = 'analytix-go-runtime-candidate'
const GO_RUNTIME_CANDIDATE_BACKEND_ID = 'go-runtime-candidate'
const DEV_GO_RUNTIME_CACHE_MANIFEST_V1 = 'runtime-server.cache-manifest-v1.json'
const RETIRED_CONTROLLED_ARTIFACT_HOST_URL_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_URL'
const RETIRED_CONTROLLED_ARTIFACT_HOST_TOKEN_ENV = 'ANALYTIX_CONTROLLED_ARTIFACT_HOST_TOKEN'
const PROTECTED_AUTHORITY_ENVIRONMENT_V1 = [
  'ANALYTIX_AUTHORITY_ANCHOR_V1',
  'ANALYTIX_AUTHORITY_MANIFEST_ROOT',
  'ANALYTIX_AUTHORITY_CREDENTIAL_PROFILE_ROOT',
  'ANALYTIX_AUTHORITY_CREDENTIAL_BUNDLE_ROOT'
] as const
const CANDIDATE_BACKEND_FIELD = ['runtime', 'Backend'].join('')
const GO_RUNTIME_DEFAULT_BACKEND_IDS = new Set(['', 'analytix', 'go', 'go-runtime', 'go-runtime-default'])
const RETIRED_INTERNAL_RUNTIME_IDS = new Set(['go-conformance', 'go-production-candidate', 'go-runtime-candidate'])
const OPERATOR_GATE_EVIDENCE_IDS = new Set([
  'runtime-go-operator-gate'
])
const PROVIDER_CREDENTIAL_GROUPS = [
  [
    ['ANALYTIX_RUNTIME_DEEPSEEK_API_KEY', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_API_KEY'],
    ['ANALYTIX_RUNTIME_DEEPSEEK_BASE_URL', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_BASE_URL'],
    ['ANALYTIX_RUNTIME_DEEPSEEK_MODEL', 'ANALYTIX_RUNTIME_GO_DEEPSEEK_MODEL']
  ],
  [
    ['ANALYTIX_RUNTIME_OPENAI_COMPAT_API_KEY', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_API_KEY'],
    ['ANALYTIX_RUNTIME_OPENAI_COMPAT_BASE_URL', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_BASE_URL'],
    ['ANALYTIX_RUNTIME_OPENAI_COMPAT_MODEL', 'ANALYTIX_RUNTIME_GO_OPENAI_COMPAT_MODEL']
  ],
  [
    ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_API_KEY', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_API_KEY'],
    ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_BASE_URL', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_BASE_URL'],
    ['ANALYTIX_RUNTIME_ANTHROPIC_COMPAT_MODEL', 'ANALYTIX_RUNTIME_GO_ANTHROPIC_COMPAT_MODEL']
  ],
  [
    ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_API_KEY', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_API_KEY'],
    ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_URL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_URL'],
    ['ANALYTIX_RUNTIME_CUSTOM_ENDPOINT_MODEL', 'ANALYTIX_RUNTIME_GO_CUSTOM_ENDPOINT_MODEL']
  ]
] as const
const PROVIDER_EVIDENCE_IDS = new Set(['runtime-go-provider-matrix'])
const MCP_EVIDENCE_IDS = new Set(['runtime-go-mcp-execution'])
const PACKAGED_QA_EVIDENCE_IDS = new Set(['runtime-go-packaged-qa'])

export type AnalytixRuntimeGoBackendId =
  | 'go-conformance'
  | 'go-production-candidate'
  | 'go-runtime-default'
  | 'go-runtime-candidate'

export type AnalytixRuntimeBackendId = AnalytixRuntimeGoBackendId | 'unsupported'

export type AnalytixRuntimeBackendGate = {
  backend: AnalytixRuntimeBackendId
  requestedBackend: string
  conformanceGateEnabled: boolean
  productionCandidateGateEnabled: boolean
  runtimeCandidateGateEnabled: boolean
  g6Readiness: GoRuntimeG6ReadinessStatus
  reason: string
  unsupportedBackend?: {
    code: 'retired_backend' | 'unsupported_backend'
    requestedBackend: string
    diagnostic: string
  }
}

export type AnalytixRuntimeBackendGateOptions = {
  appIsPackaged?: boolean
}

export type GoRuntimeG6ReadinessCheck = {
  status: 'missing' | 'skipped' | 'failed' | 'passed'
  required: boolean
  evidence?: string
  message?: string
}

export type GoRuntimeG6ReadinessStatus = {
  schemaVersion: 1
  ready: boolean
  explicitReadyGate: boolean
  durableRestartEvidence: GoRuntimeG6ReadinessCheck
  providerMatrix: GoRuntimeG6ReadinessCheck
  mcpMatrix: GoRuntimeG6ReadinessCheck
  packagedQa: GoRuntimeG6ReadinessCheck
  operatorGate: GoRuntimeG6ReadinessCheck
  missingRequiredChecks: string[]
  defaultGoBackendEnabled: true
  rendererVisibleGoSwitcher: false
}

export type GoConformanceRuntimeCanaryResult =
  | {
      ok: true
      baseUrl: string
      checks: string[]
    }
  | {
      ok: false
      baseUrl: string
      failedCheck: string
      message: string
    }

export type GoRuntimeLaunchTarget = {
  command: string
  argsPrefix: string[]
  cwd?: string
  mode: 'bundled-binary' | 'go-run-source'
}

let goSidecarOwnedProcess: OwnedProcessHandle | null = null
let goSidecarStartPromise: Promise<void> | null = null
let goSidecarStopPromise: Promise<void> | null = null
let goSidecarStopFailure: Error | null = null
let goSidecarRuntimeToken = ''
let goSidecarDurableTempDir: string | null = null
let goSidecarOwnsDurableRoot = false
let goSidecarStderrTail = ''
export type ManagedFinalPublicationAuthorityPinV1 = FinalPublicationAuthorityPinV1 & Readonly<{
  runtimePid: number
  runtimeUrl: string
  generation: number
}>
let goSidecarFinalPublicationAuthorityPin: ManagedFinalPublicationAuthorityPinV1 | null = null
let goSidecarGeneration = 0
let desktopExternalStateBoundaryForGoRuntime: DesktopExternalStateBoundary = Object.freeze({
  isolated: false,
  isolationRoot: '',
  userDataRoot: '',
  stateHomeRoot: ''
})

export function configureGoRuntimeDesktopExternalStateBoundary(
  boundary: DesktopExternalStateBoundary
): void {
  desktopExternalStateBoundaryForGoRuntime = Object.freeze({ ...boundary })
}
const intentionalGoSidecarStops = new WeakSet<ChildProcess>()
const readyGoSidecars = new WeakSet<ChildProcess>()
let onUnexpectedGoRuntimeExit: ((info: AnalytixUnexpectedExitInfo) => void) | null = null
let activeBackend: AnalytixRuntimeGoBackendId = 'go-runtime-default'
let lastBackendFallbackReason = ''

export type OptionalRuntimeCapability =
  | 'bundled_funds'
  | 'case_authority'

export async function settleOptionalRuntimeCapability<T>(
  capability: OptionalRuntimeCapability,
  operation: () => Promise<T>,
  reportUnavailable: (capability: OptionalRuntimeCapability) => void =
    reportOptionalRuntimeCapabilityUnavailable
): Promise<T | null> {
  try {
    return await operation()
  } catch {
    try {
      reportUnavailable(capability)
    } catch {
      // Optional capability diagnostics cannot become a general startup gate.
    }
    return null
  }
}

export async function takeMainOwnedRuntimeAuthorityForLaunchV1(
  take: () => Promise<MainOwnedRuntimeAuthorityEnvelopeV1 | null> =
    takeMainOwnedRuntimeAuthorityEnvelopeV1,
  reportUnavailable: () => void = () =>
    reportOptionalRuntimeCapabilityUnavailable('case_authority')
): Promise<MainOwnedRuntimeAuthorityEnvelopeV1 | null> {
  return settleOptionalRuntimeCapability(
    'case_authority',
    take,
    () => reportUnavailable()
  )
}

function reportOptionalRuntimeCapabilityUnavailable(
  capability: OptionalRuntimeCapability
): void {
  logWarn(
    'runtime-capability',
    'Optional runtime capability is unavailable; continuing with remaining Agent capabilities.',
    { capability }
  )
}

function appRoot(): string {
  const electronApp = app as unknown as { isPackaged?: boolean; getAppPath?: () => string } | undefined
  const appPath = typeof electronApp?.getAppPath === 'function' ? electronApp.getAppPath() : process.cwd()
  return electronApp?.isPackaged === true
    ? appPath.replace(/app\.asar$/, 'app.asar.unpacked')
    : appPath
}

function appResourcesPath(): string {
  const electronApp = app as unknown as { isPackaged?: boolean; getAppPath?: () => string } | undefined
  const processWithResources = process as NodeJS.Process & { resourcesPath?: string }
  if (electronApp?.isPackaged === true && typeof processWithResources.resourcesPath === 'string') {
    return processWithResources.resourcesPath
  }
  const appPath = typeof electronApp?.getAppPath === 'function' ? electronApp.getAppPath() : process.cwd()
  return appPath.endsWith('app.asar') || appPath.endsWith('app.asar.unpacked')
    ? dirname(appPath)
    : appPath
}

export function resolveAnalytixRuntimeBackendGate(
  env: NodeJS.ProcessEnv = process.env,
  options: AnalytixRuntimeBackendGateOptions = {}
): AnalytixRuntimeBackendGate {
  const requestedBackend = (env.ANALYTIX_RUNTIME_BACKEND ?? '').trim().toLowerCase()
  const conformanceGateEnabled = (env.ANALYTIX_GO_RUNTIME_CONFORMANCE ?? '').trim() === '1'
  const productionCandidateGateEnabled =
    (env.ANALYTIX_GO_RUNTIME_PRODUCTION_CANDIDATE ?? '').trim() === '1'
  const runtimeCandidateGateEnabled =
    (env.ANALYTIX_GO_RUNTIME_CANDIDATE ?? '').trim() === '1'
  const g6Readiness = resolveGoRuntimeG6ReadinessStatus(env)
  const appIsPackaged = options.appIsPackaged ?? app?.isPackaged ?? false

  if (requestedBackend === 'typescript') {
    const diagnostic = 'TypeScript agent runtime backend is retired. Go runtime is the only production agent runtime; unset ANALYTIX_RUNTIME_BACKEND or use ANALYTIX_RUNTIME_BACKEND=go-runtime-default.'
    return {
      backend: 'unsupported',
      requestedBackend,
      conformanceGateEnabled,
      productionCandidateGateEnabled,
      runtimeCandidateGateEnabled,
      g6Readiness,
      reason: diagnostic,
      unsupportedBackend: {
        code: 'retired_backend',
        requestedBackend,
        diagnostic
      }
    }
  }

  if (!requestedBackend ||
    requestedBackend === 'analytix' ||
    requestedBackend === 'go' ||
    requestedBackend === 'go-runtime' ||
    requestedBackend === 'go-runtime-default') {
    return {
      backend: 'go-runtime-default',
      requestedBackend,
      conformanceGateEnabled,
      productionCandidateGateEnabled,
      runtimeCandidateGateEnabled,
      g6Readiness,
      reason: 'Go runtime server is the default runtime core; live provider/MCP/packaged validation is post-cutover evidence.'
    }
  }

  if (RETIRED_INTERNAL_RUNTIME_IDS.has(requestedBackend)) {
    const diagnostic = appIsPackaged
      ? `Internal ${requestedBackend} backend is retired in packaged Analytix. Go runtime-default is the only production backend.`
      : `Internal ${requestedBackend} backend is retired from Electron runtime selection. Use runtime-go validation scripts directly for conformance checks.`
    return {
      backend: 'unsupported',
      requestedBackend,
      conformanceGateEnabled,
      productionCandidateGateEnabled,
      runtimeCandidateGateEnabled,
      g6Readiness,
      reason: diagnostic,
      unsupportedBackend: {
        code: 'retired_backend',
        requestedBackend,
        diagnostic
      }
    }
  }

  const diagnostic = `Unsupported ANALYTIX_RUNTIME_BACKEND=${requestedBackend}. Go runtime is the only production agent runtime; unset ANALYTIX_RUNTIME_BACKEND or use ANALYTIX_RUNTIME_BACKEND=go-runtime-default.`
  return {
    backend: 'unsupported',
    requestedBackend,
    conformanceGateEnabled,
    productionCandidateGateEnabled,
    runtimeCandidateGateEnabled,
    g6Readiness,
    reason: diagnostic,
    unsupportedBackend: {
      code: 'unsupported_backend',
      requestedBackend,
      diagnostic
    }
  }
}

export function resolveGoRuntimeG6ReadinessStatus(
  env: NodeJS.ProcessEnv = process.env
): GoRuntimeG6ReadinessStatus {
  const durableRestartEvidence = readinessCheckFromEnvKeys(env, [
    'ANALYTIX_RUNTIME_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_GO_DURABLE_RESTART_STATUS',
    'ANALYTIX_RUNTIME_DURABLE_RESTART_EVIDENCE'
  ])
  const providerMatrix = credentialedProviderMatrixCheckFromEnv(env)
  const mcpMatrix = credentialedMCPMatrixCheckFromEnv(env)
  const packagedQa = packagedQaCheckFromEnv(env)
  const operatorGate = operatorGateCheckFromEnv(env)
  const checks = {
    durableRestartEvidence,
    providerMatrix,
    mcpMatrix,
    packagedQa,
    operatorGate
  }
  const missingRequiredChecks = Object.entries(checks)
    .filter(([, check]) => check.status !== 'passed')
    .map(([key]) => key)
  const explicitReadyGate = firstConfiguredEnv(env, [
    'ANALYTIX_RUNTIME_READY',
    'ANALYTIX_GO_RUNTIME_G6_READY'
  ]) === '1'
  return {
    schemaVersion: 1,
    ready: explicitReadyGate && operatorGate.status === 'passed' && missingRequiredChecks.length === 0,
    explicitReadyGate,
    durableRestartEvidence,
    providerMatrix,
    mcpMatrix,
    packagedQa,
    operatorGate,
    missingRequiredChecks,
    defaultGoBackendEnabled: true,
    rendererVisibleGoSwitcher: false
  }
}

function credentialedProviderMatrixCheckFromEnv(env: NodeJS.ProcessEnv): GoRuntimeG6ReadinessCheck {
  const check = readinessCheckFromEnvKeys(env, [
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS'
  ])
  if (check.status !== 'passed') return check
  if (!hasCompleteProviderCredentialEnv(env)) {
    return {
      ...check,
      status: 'failed',
      message: 'provider matrix status is passed without complete credentialed provider env; local-only or scripted matrix evidence does not count'
    }
  }
  const evidence = evidencePathFromEnv(env, [
    'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE'
  ])
  const validation = validateProviderEvidence(evidence)
  if (validation.ok) return { ...check, evidence, message: validation.message }
  return {
    ...check,
    status: 'failed',
    ...(evidence ? { evidence } : {}),
    message: validation.message
  }
}

function credentialedMCPMatrixCheckFromEnv(env: NodeJS.ProcessEnv): GoRuntimeG6ReadinessCheck {
  const check = readinessCheckFromEnvKeys(env, [
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS'
  ])
  if (check.status !== 'passed') return check
  if (!firstConfiguredEnv(env, [
    'ANALYTIX_RUNTIME_MCP_COMMAND',
    'ANALYTIX_RUNTIME_GO_MCP_COMMAND'
  ]) &&
    !firstConfiguredEnv(env, [
      'ANALYTIX_RUNTIME_MCP_URL',
      'ANALYTIX_RUNTIME_GO_MCP_URL'
    ])) {
    return {
      ...check,
      status: 'failed',
      message: 'MCP matrix status is passed without configured runtime-go MCP command or URL; local-only or scripted MCP evidence does not count'
    }
  }
  const evidence = evidencePathFromEnv(env, [
    'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE'
  ])
  const validation = validateMCPEvidence(evidence)
  if (validation.ok) return { ...check, evidence, message: validation.message }
  return {
    ...check,
    status: 'failed',
    ...(evidence ? { evidence } : {}),
    message: validation.message
  }
}

function packagedQaCheckFromEnv(env: NodeJS.ProcessEnv): GoRuntimeG6ReadinessCheck {
  const check = readinessCheckFromEnvKeys(env, [
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS'
  ])
  if (check.status !== 'passed') return check
  const evidence = evidencePathFromEnv(env, [
    'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE'
  ])
  const validation = validatePackagedEvidence(evidence)
  if (validation.ok) return { ...check, evidence, message: validation.message }
  return {
    ...check,
    status: 'failed',
    ...(evidence ? { evidence } : {}),
    message: validation.message
  }
}

type OperatorEvidenceKey = 'provider' | 'mcp' | 'packaged'

type OperatorEvidencePaths = Partial<Record<OperatorEvidenceKey, string>>

type EvidenceCommitContext = {
  commit: string
  allowExactDirtyMatch: boolean
}

function operatorGateCheckFromEnv(env: NodeJS.ProcessEnv): GoRuntimeG6ReadinessCheck {
  const explicitReadyGate = firstConfiguredEnv(env, [
    'ANALYTIX_RUNTIME_READY',
    'ANALYTIX_GO_RUNTIME_G6_READY'
  ]) === '1'
  if (!explicitReadyGate) {
    return {
      status: 'missing',
      required: true,
      message: 'ANALYTIX_RUNTIME_READY is not set to 1'
    }
  }
  const evidence = evidencePathFromEnv(env, [
    'ANALYTIX_RUNTIME_READY_EVIDENCE',
    'ANALYTIX_GO_RUNTIME_G6_READY_EVIDENCE',
    'ANALYTIX_RUNTIME_GO_OPERATOR_GATE_EVIDENCE'
  ])
  const validation = validateOperatorGateEvidence(evidence, env, operatorEvidencePathsFromEnv(env))
  if (validation.ok) return { status: 'passed', required: true, evidence, message: validation.message }
  return {
    status: 'failed',
    required: true,
    ...(evidence ? { evidence } : {}),
    message: validation.message
  }
}

function operatorEvidencePathsFromEnv(env: NodeJS.ProcessEnv): OperatorEvidencePaths {
  return {
    provider: evidencePathFromEnv(env, [
      'ANALYTIX_RUNTIME_PROVIDER_MATRIX_STATUS_EVIDENCE',
      'ANALYTIX_RUNTIME_GO_PROVIDER_MATRIX_STATUS_EVIDENCE'
    ]),
    mcp: evidencePathFromEnv(env, [
      'ANALYTIX_RUNTIME_MCP_MATRIX_STATUS_EVIDENCE',
      'ANALYTIX_RUNTIME_GO_MCP_MATRIX_STATUS_EVIDENCE'
    ]),
    packaged: evidencePathFromEnv(env, [
      'ANALYTIX_RUNTIME_PACKAGED_QA_STATUS_EVIDENCE',
      'ANALYTIX_RUNTIME_GO_PACKAGED_QA_STATUS_EVIDENCE'
    ])
  }
}

function currentEvidenceCommit(env: NodeJS.ProcessEnv = process.env): EvidenceCommitContext {
  const override = (env.ANALYTIX_RUNTIME_GO_CURRENT_COMMIT ?? '').trim()
  if (override && testEvidenceCommitOverrideAllowed(env)) {
    return { commit: override, allowExactDirtyMatch: true }
  }
  const child = spawnSync('git', ['rev-parse', 'HEAD'], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  return {
    commit: child.status === 0 ? child.stdout.trim() : '',
    allowExactDirtyMatch: false
  }
}

function testEvidenceCommitOverrideAllowed(env: NodeJS.ProcessEnv): boolean {
  return env.ANALYTIX_TEST_ALLOW_CURRENT_COMMIT_OVERRIDE === '1' &&
    (env.NODE_ENV === 'test' ||
      env.VITEST === 'true' ||
      Boolean(env.VITEST_WORKER_ID) ||
      process.env.NODE_ENV === 'test' ||
      process.env.VITEST === 'true' ||
      Boolean(process.env.VITEST_WORKER_ID))
}

function hasCompleteProviderCredentialEnv(env: NodeJS.ProcessEnv): boolean {
  return PROVIDER_CREDENTIAL_GROUPS.every((provider) =>
    provider.every((aliases) =>
      aliases.some((key) => (env[key] ?? '').trim() !== '')
    )
  )
}

function readinessCheckFromEnvKeys(
  env: NodeJS.ProcessEnv,
  keys: string[]
): GoRuntimeG6ReadinessCheck {
  for (const key of keys) {
    if ((env[key] ?? '').trim()) return readinessCheckFromEnv(env, key)
  }
  return readinessCheckFromEnv(env, keys[0] || '')
}

function readinessCheckFromEnv(
  env: NodeJS.ProcessEnv,
  key: string
): GoRuntimeG6ReadinessCheck {
  const raw = (env[key] ?? '').trim().toLowerCase()
  if (!raw) {
    return { status: 'missing', required: true, message: `${key} is not set` }
  }
  const status: GoRuntimeG6ReadinessCheck['status'] =
    raw === 'passed' || raw === 'pass' || raw === 'ok' || raw === '1'
      ? 'passed'
      : raw === 'skipped' || raw === 'skip'
        ? 'skipped'
        : 'failed'
  const evidence = (env[`${key}_EVIDENCE`] ?? '').trim()
  return {
    status,
    required: true,
    ...(evidence ? { evidence } : {}),
    message: `${key}=${raw}`
  }
}

function evidencePathFromEnv(env: NodeJS.ProcessEnv, keys: string[]): string {
  for (const key of keys) {
    const value = (env[key] ?? '').trim()
    if (value) return resolve(process.cwd(), value)
  }
  return ''
}

function firstConfiguredEnv(env: NodeJS.ProcessEnv, keys: string[]): string {
  for (const key of keys) {
    const value = (env[key] ?? '').trim()
    if (value) return value
  }
  return ''
}

function readJSONEvidence(filePath: string): { ok: true, value: unknown } | { ok: false, message: string } {
  if (!filePath) return { ok: false, message: 'evidence path is not set' }
  if (!existsSync(filePath)) return { ok: false, message: `evidence path does not exist: ${filePath}` }
  try {
    return { ok: true, value: JSON.parse(readFileSync(filePath, 'utf8')) }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    return { ok: false, message: `evidence JSON is unreadable: ${message}` }
  }
}

function recordFromUnknown(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}
}

function listFromUnknown(value: unknown): unknown[] {
  return Array.isArray(value) ? value : []
}

function credentialedProbePassed(value: unknown): boolean {
  const item = recordFromUnknown(value)
  return item.status === 'passed' &&
    item.credentialed === true &&
    item.skipped !== true
}

function providerProbeMissing(value: unknown): string[] {
  const item = recordFromUnknown(value)
  const requestShape = recordFromUnknown(item.requestShape)
  const requestShapeValidation = recordFromUnknown(item.requestShapeValidation)
  const streamParsing = recordFromUnknown(item.streamParsing)
  const usageParsing = recordFromUnknown(item.usageParsing)
  const cacheTelemetry = recordFromUnknown(item.cacheTelemetry)
  const errorHandling = recordFromUnknown(item.errorHandling)
  const redaction = recordFromUnknown(item.redaction)
  const missing: string[] = []
  if (!credentialedProbePassed(item)) missing.push('credentialed-pass')
  if (!item.endpointFamily) missing.push('endpointFamily')
  if (!item.endpointFormat) missing.push('endpointFormat')
  if (requestShape.method !== 'POST') missing.push('requestShape.method')
  if (requestShape.requestUrl !== item.requestUrl) missing.push('requestShape.requestUrl')
  if (requestShape.endpointFamily !== item.endpointFamily) missing.push('requestShape.endpointFamily')
  if (requestShape.endpointFormat !== item.endpointFormat) missing.push('requestShape.endpointFormat')
  if (requestShape.credentialConfigured !== true) missing.push('requestShape.credentialConfigured')
  if (requestShape.credentialValueRecorded !== false) missing.push('requestShape.credentialValueRecorded:false')
  if (requestShape.deepSeekSpecificRequestFieldsPresent !== false) missing.push('requestShape.deepSeekSpecificRequestFieldsPresent:false')
  if (requestShapeValidation.status !== 'passed') missing.push('requestShapeValidation')
  if (requestShape.streamRequested !== true) missing.push('requestShape.streamRequested')
  const messageCount = typeof requestShape.messageCount === 'number' ? requestShape.messageCount : NaN
  if ((!Number.isFinite(messageCount) || messageCount <= 0) && requestShape.inputConfigured !== true) {
    missing.push('requestShape.messageOrInput')
  }
  if (!requestShape.maxTokensField) missing.push('requestShape.maxTokensField')
  if (streamParsing.status !== 'passed') missing.push('streamParsing')
  if (usageParsing.status !== 'passed') missing.push('usageParsing')
  if (cacheTelemetry.status !== 'passed') missing.push('cacheTelemetry')
  if (errorHandling.status !== 'passed') missing.push('errorHandling')
  if (errorHandling.errorProbePassed !== true) missing.push('errorHandling.errorProbePassed')
  const errorStatus = typeof errorHandling.httpStatus === 'number' ? errorHandling.httpStatus : NaN
  if (!Number.isFinite(errorStatus) || errorStatus < 400 || errorStatus >= 500) {
    missing.push('errorHandling.httpStatus4xx')
  }
  if (recordFromUnknown(errorHandling.requestShape).intentionallyInvalidBody !== true) {
    missing.push('errorHandling.requestShape.intentionallyInvalidBody')
  }
  if (recordFromUnknown(errorHandling.requestShape).credentialValueRecorded !== false) {
    missing.push('errorHandling.requestShape.credentialValueRecorded:false')
  }
  if (errorHandling.rawResponseRecorded !== false) missing.push('errorHandling.rawResponseRecorded:false')
  if (errorHandling.credentialValueRecorded !== false) missing.push('errorHandling.credentialValueRecorded:false')
  if (redaction.status !== 'passed' || redaction.credentialValueRecorded !== false) missing.push('redaction')
  if (item.id === 'deepseek' && cacheTelemetry.providerScoped !== true) {
    missing.push('deepseek.cacheTelemetry.providerScoped')
  }
  if (item.id !== 'deepseek' && cacheTelemetry.deepSeekSpecificRequestFieldsPresent !== false) {
    missing.push(`${String(item.id)}.cacheTelemetry.deepSeekSpecificRequestFieldsPresent:false`)
  }
  if (item.id === 'custom-endpoint') {
    if (item.endpointFamily !== 'custom-full-endpoint') missing.push('custom.endpointFamily')
    if (item.endpointFormat !== 'custom-full-endpoint') missing.push('custom.endpointFormat')
    if (requestShape.urlPolicy !== 'custom-full-endpoint-no-append') missing.push('custom.urlPolicy')
  }
  return missing
}

function validateProviderEvidence(filePath: string): { ok: boolean, message: string } {
  const parsed = readJSONEvidence(filePath)
  if (!parsed.ok) return parsed
  const secretFinding = firstEvidenceSecretFinding(parsed.value)
  if (secretFinding) return { ok: false, message: `credentialed provider evidence must not contain secret material (${secretFinding})` }
  const root = recordFromUnknown(parsed.value)
  if (!PROVIDER_EVIDENCE_IDS.has(String(root.id || '')) ||
    root.status !== 'passed' ||
    root.passed !== true ||
    root.credentialSecretsRecorded !== false ||
    root.credentialedMatrixEnvGated !== true ||
    root.readsRealApiKeysByDefault !== false ||
    recordFromUnknown(root.redaction).status !== 'passed' ||
    recordFromUnknown(root.redaction).secretMaterialFound !== false) {
    return {
      ok: false,
      message: 'credentialed provider evidence must be a passed runtime-go provider matrix report with env-gated root redaction'
    }
  }
  const probes = listFromUnknown(root.credentialedProbes)
  const requiredIds = new Set(['deepseek', 'openai-compatible', 'anthropic-compatible', 'custom-endpoint'])
  const invalid: string[] = []
  for (const probe of probes) {
    const item = recordFromUnknown(probe)
    if (credentialedProbePassed(item) && typeof item.id === 'string') {
      requiredIds.delete(item.id)
    }
    if (typeof item.id === 'string' && ['deepseek', 'openai-compatible', 'anthropic-compatible', 'custom-endpoint'].includes(item.id)) {
      const missingFields = providerProbeMissing(item)
      if (missingFields.length > 0) invalid.push(`${item.id}:${missingFields.join('/')}`)
    }
  }
  if (requiredIds.size > 0) {
    return {
      ok: false,
      message: `credentialed provider evidence is incomplete; missing passed probes: ${[...requiredIds].join(', ')}`
    }
  }
  if (invalid.length > 0) {
    return {
      ok: false,
      message: `credentialed provider evidence has incomplete runtime-go provider protocol fields: ${invalid.join(', ')}`
    }
  }
  return { ok: true, message: 'credentialed provider evidence file includes all required runtime-go provider protocol fields' }
}

function validateMCPEvidence(filePath: string): { ok: boolean, message: string } {
  const parsed = readJSONEvidence(filePath)
  if (!parsed.ok) return parsed
  const secretFinding = firstEvidenceSecretFinding(parsed.value)
  if (secretFinding) return { ok: false, message: `credentialed MCP evidence must not contain secret material (${secretFinding})` }
  const root = recordFromUnknown(parsed.value)
  const probes = listFromUnknown(root.credentialedProbes)
  if (!MCP_EVIDENCE_IDS.has(String(root.id || '')) ||
    root.status !== 'passed' ||
    root.passed !== true ||
    root.credentialedExecution !== true) {
    return {
      ok: false,
      message: 'credentialed MCP evidence must be a passed runtime-go MCP execution report'
    }
  }
  if (root.topLevelMcpIndexerExposed !== false || root.reasonixPublicProtocolUsed !== false) {
    return {
      ok: false,
      message: 'credentialed MCP evidence must explicitly keep MCP-indexer and upstream protocol surfaces hidden'
    }
  }
  const missingCoverage = missingMCPCoverage(root, probes)
  if (missingCoverage.length > 0) {
    return {
      ok: false,
      message: `credentialed MCP evidence is incomplete; missing passed coverage: ${missingCoverage.join(', ')}`
    }
  }
  const probe = probes.map(recordFromUnknown).find((item) => item.id === 'credentialed-mcp')
  if (!probe) {
    return {
      ok: false,
      message: 'credentialed MCP evidence missing credentialed-mcp probe details'
    }
  }
  const legacyApprovalUserInputKey = ['approvalUserInput', 'Pr', 'oof'].join('')
  const approvalUserInputEvidence = recordFromUnknown(
    probe.approvalUserInputEvidence ?? probe[legacyApprovalUserInputKey]
  )
  const missingProbe = []
  if (!credentialedProbePassed(probe)) missingProbe.push('credentialed-pass')
  if (probe.credentialedExecution !== true) missingProbe.push('credentialedExecution')
  if (probe.connect !== true) missingProbe.push('connect')
  if (probe.toolDiscoverySearch !== true) missingProbe.push('toolDiscoverySearch')
  if (probe.toolCall !== true) missingProbe.push('toolCall')
  if (probe.approvalUserInput !== true) missingProbe.push('approvalUserInput')
  if (probe.reconnect !== true) missingProbe.push('reconnect')
  if (probe.credentialRedaction !== true && probe.redaction !== true) missingProbe.push('credentialRedaction')
  if (probe.commandConfigured !== true && probe.urlConfigured !== true) missingProbe.push('command-or-url-configured')
  if (probe.toolNameConfigured !== true) missingProbe.push('toolNameConfigured')
  if (!isSha256(probe.calledToolNameHash)) missingProbe.push('calledToolNameHash')
  if (!isSha256(probe.toolCatalogDigest)) missingProbe.push('toolCatalogDigest')
  if (!isSha256(probe.reconnectToolNameHash)) missingProbe.push('reconnectToolNameHash')
  if (!isSha256(probe.reconnectToolCatalogDigest)) missingProbe.push('reconnectToolCatalogDigest')
  if (typeof probe.reconnectToolCount !== 'number' || !Number.isFinite(probe.reconnectToolCount) || probe.reconnectToolCount < 1) {
    missingProbe.push('reconnectToolCount')
  }
  const evidenceMissing = approvalUserInputEvidenceMissing(approvalUserInputEvidence)
  if (evidenceMissing.length > 0) missingProbe.push(`approvalUserInputEvidence:${evidenceMissing.join('|')}`)
  if (missingProbe.length > 0) {
    return {
      ok: false,
      message: `credentialed MCP evidence missing runtime-go probe details: ${missingProbe.join(', ')}`
    }
  }
  return { ok: true, message: 'credentialed MCP evidence file includes real connect/search/call/approval/reconnect/redaction coverage' }
}

function approvalUserInputEvidenceMissing(evidence: Record<string, unknown>): string[] {
  const missing: string[] = []
  if (evidence.status !== 'passed') missing.push('status')
  if (typeof evidence.evidencePath !== 'string' || evidence.evidencePath.trim().length === 0) {
    missing.push('evidencePath')
  } else {
    const evidencePath = resolve(process.cwd(), evidence.evidencePath)
    if (!existsSync(evidencePath)) {
      missing.push('evidencePathExists')
    } else if (isSha256(evidence.evidenceSha256) && sha256(readFileSync(evidencePath, 'utf8')) !== evidence.evidenceSha256) {
      missing.push('evidenceSha256:match')
    }
  }
  if (!isSha256(evidence.evidenceSha256)) missing.push('evidenceSha256')
  if (evidence.evidenceDigestAlgorithm !== 'sha256:file-bytes-v1') missing.push('evidenceDigestAlgorithm')
  if (evidence.rawValueRecorded !== false) missing.push('rawValueRecorded:false')
  if (evidence.credentialSecretsRecorded !== false) missing.push('credentialSecretsRecorded:false')
  return missing
}

function validatePackagedEvidence(filePath: string): { ok: boolean, message: string } {
  const parsed = readJSONEvidence(filePath)
  if (!parsed.ok) return parsed
  const secretFinding = firstEvidenceSecretFinding(parsed.value)
  if (secretFinding) return { ok: false, message: `packaged QA evidence must not contain secret material (${secretFinding})` }
  const root = recordFromUnknown(parsed.value)
  const checks = listFromUnknown(root.checks)
  const requiredIds = new Set([
    'packaged-app-startup',
    'health',
    'runtime-info',
    'thread-list',
    'turn-create',
    'sse-replay',
    'go-runtime-default-gate',
    'typescript-retired-backend'
  ])
  const failed = checks.filter((item) => recordFromUnknown(item).status !== 'passed')
  if (
    !PACKAGED_QA_EVIDENCE_IDS.has(String(root.id || '')) ||
    root.passed !== true ||
    root.status !== 'passed' ||
    failed.length > 0
  ) {
    return {
      ok: false,
      message: 'packaged QA evidence must be a passed runtime-go packaged QA report with all checks passed'
    }
  }
  for (const check of checks) {
    const id = recordFromUnknown(check).id
    if (typeof id === 'string' && recordFromUnknown(check).status === 'passed') {
      requiredIds.delete(id)
    }
  }
  if (requiredIds.size > 0) {
    return {
      ok: false,
      message: `packaged QA evidence is missing required check coverage: ${[...requiredIds].join(', ')}`
    }
  }
  const startup = recordFromUnknown(root.startup)
  const runtime = recordFromUnknown(root.runtime)
  const legacyDefaultRuntimeGateKey = ['candidate', 'Gate'].join('')
  const legacyCandidateEnvSetKey = ['candidate', 'Env', 'Set'].join('')
  const legacyCandidateStoppedKey = ['candidate', 'Stopped'].join('')
  const legacyCandidateLaunchCommandHashKey = ['candidate', 'LaunchCommandHash'].join('')
  const defaultRuntimeGate = recordFromUnknown(root.defaultRuntimeGate ?? root[legacyDefaultRuntimeGateKey])
  const legacyRollbackKey = ['rollback', 'Pr', 'oof'].join('')
  const retiredBackendEvidence = recordFromUnknown(root.retiredBackendEvidence ?? root[legacyRollbackKey])
  const explicitRuntimeBackendOverrideEnvSet = defaultRuntimeGate.explicitRuntimeBackendOverrideEnvSet ?? defaultRuntimeGate[legacyCandidateEnvSetKey]
  const defaultRuntimeStopped = retiredBackendEvidence.defaultRuntimeStopped ?? retiredBackendEvidence[legacyCandidateStoppedKey]
  const defaultRuntimeLaunchCommandHash = retiredBackendEvidence.defaultRuntimeLaunchCommandHash ?? retiredBackendEvidence[legacyCandidateLaunchCommandHashKey]
  const missingEvidence: string[] = []
  const rootRedaction = recordFromUnknown(root.redaction)
  if (root.credentialSecretsRecorded !== false) missingEvidence.push('credentialSecretsRecorded:false')
  if (rootRedaction.status !== 'passed' || rootRedaction.secretMaterialFound !== false) missingEvidence.push('redaction')
  if (startup.appPathExists !== true) missingEvidence.push('startup.appPathExists')
  if (startup.launchCommandConfigured !== true) missingEvidence.push('startup.launchCommandConfigured')
  if (startup.launchCommandReferencesAppPath !== true) missingEvidence.push('startup.launchCommandReferencesAppPath')
  if (startup.exitCode !== 0) missingEvidence.push('startup.exitCode')
  if (!isSha256(startup.launchCommandHash)) missingEvidence.push('startup.launchCommandHash')
  if (!runtime.runtimeUrl) missingEvidence.push('runtime.runtimeUrl')
  if (runtime.runtimeTokenConfigured !== true) missingEvidence.push('runtime.runtimeTokenConfigured')
  if (runtime.healthOK !== true) missingEvidence.push('runtime.healthOK')
  const runtimeInfo = recordFromUnknown(runtime.runtimeInfo)
  if (runtimeInfo.candidateContract !== true) missingEvidence.push('runtime.runtimeInfo.candidateContract')
  if (runtimeInfo.hostLocalhost !== true) missingEvidence.push('runtime.runtimeInfo.hostLocalhost')
  if (runtimeInfo.dataDirConfigured !== true) missingEvidence.push('runtime.runtimeInfo.dataDirConfigured')
  if (!isSha256(runtimeInfo.dataDirHash)) missingEvidence.push('runtime.runtimeInfo.dataDirHash')
  if (typeof runtimeInfo.mcpAvailable !== 'boolean' || runtimeInfo.mcpDiagnosticHonest !== true) {
    missingEvidence.push('runtime.runtimeInfo.mcpDiagnosticHonest')
  }
  if (runtimeInfo.subagentsAvailable !== true) {
    missingEvidence.push('runtime.runtimeInfo.subagentsAvailable')
  }
  if (runtimeInfo.subagentsDiagnosticHonest !== true) {
    missingEvidence.push('runtime.runtimeInfo.subagentsDiagnosticHonest')
  }
  if (runtimeInfo.productRuntimeInfoHidesUpstreamEvidence !== true) missingEvidence.push('runtime.runtimeInfo.productRuntimeInfoHidesUpstreamEvidence')
  if (runtimeInfo.reasonixUpstreamAbsorptionPresent !== false) missingEvidence.push('runtime.runtimeInfo.reasonixUpstreamAbsorptionPresent:false')
  if (runtimeInfo.rawValueRecorded !== false) missingEvidence.push('runtime.runtimeInfo.rawValueRecorded:false')
  if (!isSha256(runtimeInfo.infoShapeDigest)) missingEvidence.push('runtime.runtimeInfo.infoShapeDigest')
  if (!isSha256(runtime.threadIdHash)) missingEvidence.push('runtime.threadIdHash')
  if (!isSha256(runtime.turnIdHash)) missingEvidence.push('runtime.turnIdHash')
  if (!GO_RUNTIME_DEFAULT_BACKEND_IDS.has(String(defaultRuntimeGate[CANDIDATE_BACKEND_FIELD] || ''))) {
    missingEvidence.push(`defaultRuntimeGate.${CANDIDATE_BACKEND_FIELD}:go-runtime-default`)
  }
  if (explicitRuntimeBackendOverrideEnvSet !== false) missingEvidence.push('defaultRuntimeGate.explicitRuntimeBackendOverrideEnvSet:false')
  if (defaultRuntimeGate.goDefaultBackendEnabled !== true) missingEvidence.push('defaultRuntimeGate.goDefaultBackendEnabled:true')
  if (retiredBackendEvidence.requestedBackend !== 'typescript') missingEvidence.push('retiredBackendEvidence.requestedBackend:typescript')
  if (retiredBackendEvidence.code !== 'retired_backend') missingEvidence.push('retiredBackendEvidence.code:retired_backend')
  if (retiredBackendEvidence.activeBackendAfterRequest !== 'go-runtime-default') missingEvidence.push('retiredBackendEvidence.activeBackendAfterRequest')
  if (defaultRuntimeStopped !== false) missingEvidence.push('retiredBackendEvidence.defaultRuntimeStopped:false')
  if (retiredBackendEvidence.tsStarted !== false) missingEvidence.push('retiredBackendEvidence.tsStarted:false')
  if (retiredBackendEvidence.runtimeHealthAfterRequest !== true) missingEvidence.push('retiredBackendEvidence.runtimeHealthAfterRequest')
  if (!isSha256(defaultRuntimeLaunchCommandHash)) missingEvidence.push('retiredBackendEvidence.defaultRuntimeLaunchCommandHash')
  else if (defaultRuntimeLaunchCommandHash !== startup.launchCommandHash) {
    missingEvidence.push('retiredBackendEvidence.defaultRuntimeLaunchCommandHash:match')
  }
  if (retiredBackendEvidence.preRequestRuntimeUrl !== runtime.runtimeUrl) missingEvidence.push('retiredBackendEvidence.preRequestRuntimeUrl:match')
  if (retiredBackendEvidence.postRequestRuntimeUrl !== runtime.runtimeUrl) missingEvidence.push('retiredBackendEvidence.postRequestRuntimeUrl:match')
  if (!retiredBackendEvidence.verifiedAt) missingEvidence.push('retiredBackendEvidence.verifiedAt')
  if (retiredBackendEvidence.credentialSecretsRecorded !== false) missingEvidence.push('retiredBackendEvidence.credentialSecretsRecorded:false')
  const retiredBackendRedaction = recordFromUnknown(retiredBackendEvidence.redaction)
  if (retiredBackendRedaction.status !== 'passed' || retiredBackendRedaction.secretMaterialFound !== false) {
    missingEvidence.push('retiredBackendEvidence.redaction')
  }
  if (missingEvidence.length > 0) {
    return {
      ok: false,
      message: `packaged QA evidence is missing runtime-go process/runtime/rollback evidence: ${missingEvidence.join(', ')}`
    }
  }
  return { ok: true, message: 'packaged QA evidence file includes all required runtime-go checks and evidence bindings' }
}

function validateOperatorGateEvidence(
  filePath: string,
  env: NodeJS.ProcessEnv = process.env,
  expectedEvidencePaths: OperatorEvidencePaths = {}
): { ok: boolean, message: string } {
  const parsed = readJSONEvidence(filePath)
  if (!parsed.ok) return parsed
  const secretFinding = firstEvidenceSecretFinding(parsed.value)
  if (secretFinding) return { ok: false, message: `operator gate evidence must not contain secret material (${secretFinding})` }
  const root = recordFromUnknown(parsed.value)
  const envGate = root.operatorGate === 'ANALYTIX_RUNTIME_READY=1' ||
    root.envGate === 'ANALYTIX_RUNTIME_READY=1' ||
    root.operatorGate === 'ANALYTIX_GO_RUNTIME_G6_READY=1' ||
    root.envGate === 'ANALYTIX_GO_RUNTIME_G6_READY=1'
  const reviewed = root.credentialedEvidenceReviewed === true
  const legacyGoDefaultApprovedKey = ['goDefault', 'Candidate', 'Approved'].join('')
  const legacyDefaultApprovedKey = ['default', 'Candidate', 'Approved'].join('')
  const approved = root.goDefaultApproved === true ||
    root.defaultRuntimeApproved === true ||
    root[legacyGoDefaultApprovedKey] === true ||
    root[legacyDefaultApprovedKey] === true
  const missing: string[] = []
  if (!OPERATOR_GATE_EVIDENCE_IDS.has(String(root.id || ''))) missing.push('id')
  if (root.status !== 'passed') missing.push('status')
  if (root.passed !== true) missing.push('passed')
  if (!envGate) missing.push('operatorGate')
  if (root.explicitEnvGate !== true) missing.push('explicitEnvGate')
  if (!reviewed) missing.push('credentialedEvidenceReviewed')
  if (!approved) missing.push('goDefaultApproved')
  if (root.typeScriptFallbackRetained !== false) missing.push('typeScriptFallbackRetained:false')
  if (root.goDefaultBackendEnabled !== true) missing.push('goDefaultBackendEnabled:true')
  const evidenceCommit = root.evidenceTargetCommit || root.commitHash || root.candidateCommit || root.sourceCommit
  if (!evidenceCommit) missing.push('evidenceTargetCommit')
  else {
    const currentCommit = currentEvidenceCommit(env)
    if (!operatorCommitCoversCurrent(evidenceCommit, currentCommit)) {
      missing.push(`commit:${String(evidenceCommit)} does not cover ${currentCommit.commit || 'unknown'}`)
    }
  }
  if (root.evidenceDigestAlgorithm !== 'sha256:canonical-json-v1') missing.push('evidenceDigestAlgorithm')
  const reportPaths = recordFromUnknown(root.reportPaths)
  const evidenceDigests = recordFromUnknown(root.evidenceDigests)
  const resolvedReportPaths: OperatorEvidencePaths = {}
  for (const key of ['provider', 'mcp', 'packaged'] as const) {
    const digest = evidenceDigests[key]
    const reportPath = reportPaths[key]
    const expectedPath = expectedEvidencePaths[key]
    if (!isSha256(digest)) missing.push(`evidenceDigests.${key}`)
    if (typeof reportPath !== 'string' || !reportPath.trim()) {
      missing.push(`reportPaths.${key}`)
      continue
    }
    const resolvedReportPath = resolve(process.cwd(), reportPath)
    resolvedReportPaths[key] = resolvedReportPath
    if (expectedPath && resolvedReportPath !== expectedPath) {
      missing.push(`reportPaths.${key}:match`)
    }
    const parsedEvidence = readJSONEvidence(expectedPath || resolvedReportPath)
    if (!parsedEvidence.ok) {
      missing.push(`reportPaths.${key}:readable`)
      continue
    }
    if (digest !== sha256(canonicalJSONString(parsedEvidence.value))) {
      missing.push(`evidenceDigests.${key}:match`)
    }
  }
  const reviewedRows = listFromUnknown(root.evidenceReviewed).map(recordFromUnknown)
  const requiredReviewRows: Array<[string, OperatorEvidenceKey]> = [
    ['provider-matrix-credentialed', 'provider'],
    ['mcp-matrix-credentialed', 'mcp'],
    ['packaged-qa', 'packaged']
  ]
  for (const [id, key] of requiredReviewRows) {
    const item = reviewedRows.find((row) => row.id === id)
    if (!item) missing.push(`evidenceReviewed.${id}`)
    else if (item.status !== 'passed') missing.push(`evidenceReviewed.${id}.status`)
    else {
      const reviewedPath = typeof item.path === 'string' && item.path.trim()
        ? resolve(process.cwd(), item.path)
        : ''
      const expectedPath = expectedEvidencePaths[key] || resolvedReportPaths[key] || ''
      if (!reviewedPath) missing.push(`evidenceReviewed.${id}.path`)
      else if (expectedPath && reviewedPath !== expectedPath) missing.push(`evidenceReviewed.${id}.path:match`)
    }
  }
  if (missing.length > 0) {
    return {
      ok: false,
      message: `operator gate evidence must be a passed runtime-go operator gate with explicit env gate, digest bindings, evidence review, and Go default approval: ${missing.join(', ')}`
    }
  }
  return { ok: true, message: 'operator gate evidence file authorizes ANALYTIX_RUNTIME_READY=1 with runtime-go digest bindings' }
}

function operatorCommitCoversCurrent(targetCommit: unknown, currentCommit: EvidenceCommitContext): boolean {
  const target = String(targetCommit || '').trim()
  const current = currentCommit.commit
  if (!target || !current) return false
  if (!looksLikeGitCommit(target) || !looksLikeGitCommit(current)) return false
  if (target === current) {
    return currentCommit.allowExactDirtyMatch || relevantWorkingTreeClean()
  }
  const mergeBase = spawnSync('git', ['merge-base', '--is-ancestor', target, current], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  if (mergeBase.status !== 0) return false
  const committed = spawnSync('git', ['diff', '--quiet', `${target}..${current}`, '--', ...relevantEvidencePaths], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  if (committed.status !== 0) return false
  return relevantWorkingTreeClean()
}

function relevantWorkingTreeClean(): boolean {
  const unstaged = spawnSync('git', ['diff', '--quiet', '--', ...relevantEvidencePaths], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  if (unstaged.status !== 0) return false
  const staged = spawnSync('git', ['diff', '--cached', '--quiet', '--', ...relevantEvidencePaths], {
    cwd: process.cwd(),
    encoding: 'utf8'
  })
  return staged.status === 0
}

function looksLikeGitCommit(value: unknown): boolean {
  return /^[0-9a-f]{7,40}$/i.test(String(value || ''))
}

function missingMCPCoverage(root: Record<string, unknown>, probes: unknown[]): string[] {
  const sources = [root, ...probes.map(recordFromUnknown)]
  const required: Array<[string, string[]]> = [
    ['connect', ['connect', 'mcpConnect']],
    ['tool-discovery-search', ['toolDiscoverySearch', 'toolDiscovery', 'search']],
    ['tool-call', ['toolCall', 'call', 'approvedCallExecutes']],
    ['approval-user-input', ['approvalUserInput', 'approval', 'userInput']],
    ['reconnect', ['reconnect']],
    ['redaction', ['redaction', 'credentialRedaction']]
  ]
  return required
    .filter(([, keys]) => !sources.some((source) =>
      keys.some((key) => source[key] === true || source[key] === 'passed')
    ))
    .map(([id]) => id)
}

function firstEvidenceSecretFinding(value: unknown, path = '$'): string {
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      const finding = firstEvidenceSecretFinding(value[index], `${path}[${index}]`)
      if (finding) return finding
    }
    return ''
  }
  if (value && typeof value === 'object') {
    for (const [key, item] of Object.entries(value as Record<string, unknown>)) {
      if (isSecretEvidenceKey(key)) return `${path}.${key}`
      const finding = firstEvidenceSecretFinding(item, `${path}.${key}`)
      if (finding) return finding
    }
    return ''
  }
  if (typeof value === 'string' && stringLooksLikeSecret(value)) return path
  return ''
}

function isSecretEvidenceKey(key: string): boolean {
  const normalized = key.replace(/[-_]/g, '').toLowerCase()
  return [
    'apikey',
    'xapikey',
    'apikeyheader',
    'xapikeyheader',
    'authorization',
    'authorizationheader',
    'accesstoken',
    'refreshtoken',
    'runtimetoken',
    'token',
    'password',
    'secret',
    'secretkey',
    'privatekey',
    'clientsecret',
    'signature',
    'sig',
    'auth',
    'credential',
    'jwt'
  ].includes(normalized)
}

function canonicalJSONString(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map((item) => canonicalJSONString(item)).join(',')}]`
  if (value && typeof value === 'object') {
    const entries = Object.entries(value).sort(([a], [b]) => a.localeCompare(b))
    return `{${entries.map(([key, item]) => `${JSON.stringify(key)}:${canonicalJSONString(item)}`).join(',')}}`
  }
  return JSON.stringify(value)
}

function sha256(value: string): string {
  return createHash('sha256').update(value).digest('hex')
}

function isSha256(value: unknown): boolean {
  return typeof value === 'string' && /^[a-f0-9]{64}$/i.test(value)
}

const relevantEvidencePaths = [
  'src/main/runtime/analytix-adapter.ts',
  'scripts/runtime-go-validation-command.mjs',
  'scripts/runtime-go-live-validation.mjs',
  'scripts/runtime-go-preflight.mjs',
  'scripts/runtime-go-default-readiness-report.mjs',
  'scripts/runtime-go-cutover-report.mjs',
  'scripts/runtime-go-engine-absorption-report.mjs',
  'scripts/runtime-go-live-evidence-collector.mjs',
  'scripts/runtime-go-packaged-qa.mjs',
  'scripts/runtime-go-packaged-gui-smoke.mjs',
  'scripts/runtime-go-packaged-session-soak.mjs',
  'scripts/runtime-go-rollback-retirement-evidence.mjs',
  'scripts/runtime-go-rollback-retirement-report.mjs',
  'src/main/runtime/runtime-go-engine-absorption-report.test.ts',
  'src/main/runtime/runtime-go-packaged-contract-report.test.ts',
  'src/main/runtime/analytix-adapter.test.ts',
  'docs/analytix/upstreams/kun-reasonix-feature-delta-audit.md',
  'packages/runtime-go',
  'packages/runtime/src'
]

function stringLooksLikeSecret(value: string): boolean {
  return /\bBearer\s+[A-Za-z0-9._~+/-]+=*/.test(value) ||
    /\bBasic\s+[A-Za-z0-9._~+/-]+=*/.test(value) ||
    /\bsk-[A-Za-z0-9_-]{12,}\b/.test(value) ||
    /(?:api[_-]?key|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)=(?!\[?redacted\]?|<redacted>|\[REDACTED\])[^&\s]+/i.test(value) ||
    /(?:x-api-key|api[_-]?key|authorization|access[_-]?token|refresh[_-]?token|runtime[_-]?token|token|key|signature|sig|auth|credential|jwt)\s*:\s*(?!\[?redacted\]?|<redacted>|\[REDACTED\])[^\s,;]+/i.test(value)
}

export function getAnalytixRuntimeBackendStatus(): {
  activeBackend: AnalytixRuntimeGoBackendId
  gate: AnalytixRuntimeBackendGate
  fallbackReason: string
} {
  const gate = resolveAnalytixRuntimeBackendGate()
  return {
    activeBackend: resolveReportedActiveBackend(activeBackend, gate),
    gate,
    fallbackReason: lastBackendFallbackReason
  }
}

export function resolveReportedActiveBackend(
  currentActiveBackend: AnalytixRuntimeGoBackendId,
  gate: AnalytixRuntimeBackendGate
): AnalytixRuntimeGoBackendId {
  void gate
  return currentActiveBackend
}

function getGoRuntimeDir(): string {
  const override = process.env.ANALYTIX_GO_RUNTIME_DIR?.trim()
  return override || join(appRoot(), 'packages/runtime-go')
}

function getGoRuntimeFixturesDir(): string {
  const override = process.env.ANALYTIX_GO_RUNTIME_FIXTURES_DIR?.trim()
  return override || join(appRoot(), 'packages/runtime/src/conformance/fixtures')
}

export function buildDevGoToolchainEnvV1(
  env: NodeJS.ProcessEnv = process.env
): NodeJS.ProcessEnv {
  const result: NodeJS.ProcessEnv = {}
  for (const name of [
    'HOME',
    'USERPROFILE',
    'APPDATA',
    'LOCALAPPDATA',
    'XDG_CACHE_HOME',
    'TMPDIR',
    'TEMP',
    'TMP',
    'SystemRoot',
    'SYSTEMROOT',
    'WINDIR',
    'PATH',
    'LANG',
    'LC_ALL',
    'TZ'
  ]) {
    if (env[name] !== undefined) result[name] = env[name]
  }
  const cacheEnvironment: NodeJS.ProcessEnv = {}
  if (env.ANALYTIX_DEV_CACHE_ROOT) {
    const cache = require(join(appRoot(), 'scripts/lib/development-cache-environment.cjs')) as {
      projectGoDevelopmentCacheEnvironment: (input: NodeJS.ProcessEnv) => { authorizedModuleCache: string }
      DEVELOPMENT_CACHE_ENVIRONMENT: Record<string, string>
    }
    // Retain only the existing repository-authorized cache layout. Ambient
    // compiler/proxy/credential overrides still never enter the subprocess.
    const admitted = cache.projectGoDevelopmentCacheEnvironment(env)
    cacheEnvironment.GOCACHE = cache.DEVELOPMENT_CACHE_ENVIRONMENT.GOCACHE
    cacheEnvironment.GOMODCACHE = admitted.authorizedModuleCache
    cacheEnvironment.GOTMPDIR = cache.DEVELOPMENT_CACHE_ENVIRONMENT.GOTMPDIR
  }
  return {
    ...result,
    ...cacheEnvironment,
    CGO_ENABLED: '0',
    GOENV: 'off',
    GOINSECURE: '',
    GONOPROXY: '',
    GONOSUMDB: '',
    GOPRIVATE: '',
    GOPROXY: 'https://proxy.golang.org',
    GOSUMDB: 'sum.golang.org',
    GOTOOLCHAIN: 'local',
    GOVCS: 'public:git|hg,private:off',
    GOWORK: 'off'
  }
}

function resolveGoBinary(
  env: NodeJS.ProcessEnv = process.env,
  fixedCandidates: string[] = [
    '/usr/local/go/bin/go',
    '/opt/homebrew/bin/go',
    '/tmp/analytix-go-toolchain/go/bin/go'
  ]
): string {
  const explicit = env.ANALYTIX_GO_BIN?.trim()
  if (explicit) {
    if (!existsSync(explicit)) {
      throw new Error(`ANALYTIX_GO_BIN does not exist: ${explicit}`)
    }
    return explicit
  }
  const binaryName = process.platform === 'win32' ? 'go.exe' : 'go'
  for (const dir of (env.PATH ?? '').split(delimiter)) {
    if (!dir) continue
    const candidate = join(dir, binaryName)
    if (goBinaryLooksUsable(candidate, env)) return candidate
  }
  for (const candidate of fixedCandidates) {
    if (goBinaryLooksUsable(candidate, env)) return candidate
  }
  throw new Error('Go binary not found; set ANALYTIX_GO_BIN or install the temporary analytix Go toolchain.')
}

function goBinaryLooksUsable(candidate: string, env: NodeJS.ProcessEnv): boolean {
  if (!existsSync(candidate)) return false
  const result = spawnSync(candidate, ['env', 'GOROOT'], {
    encoding: 'utf8',
    env: buildDevGoToolchainEnvV1(env),
    timeout: 5_000
  })
  if (result.status !== 0) return false
  const goroot = result.stdout.trim()
  return goroot !== '' && existsSync(join(goroot, 'src', 'context', 'context.go'))
}

export function resolveBundledGoRuntimeServerPath(
  appPath: string = app.getAppPath(),
  platform: NodeJS.Platform = process.platform
): string {
  const resourcesRoot = appPath.endsWith('app.asar') || appPath.endsWith('app.asar.unpacked')
    ? dirname(appPath)
    : appPath
  return join(resourcesRoot, 'runtime-go', 'bin', runtimeServerBinaryName(platform))
}

function runtimeServerBinaryName(platform: NodeJS.Platform = process.platform): string {
  return platform === 'win32' ? 'runtime-server.exe' : 'runtime-server'
}

function devGoRuntimeServerCacheRoot(runtimeGoDir: string, env: NodeJS.ProcessEnv): string {
  const explicit = env.ANALYTIX_GO_RUNTIME_SERVER_CACHE_DIR?.trim()
  if (explicit) return explicit
  if (env.ANALYTIX_DEV_CACHE_ROOT) {
    buildDevGoToolchainEnvV1(env) // Validate the marker and exact cache inputs first.
    return join(env.ANALYTIX_DEV_CACHE_ROOT, 'runtime-go', sha256(realpathSync(runtimeGoDir)))
  }
  return join(appRoot(), '.cache', 'runtime-go', sha256(realpathSync(runtimeGoDir)))
}

function runtimeGoSourceFingerprint(runtimeGoDir: string): string {
  const entries: string[] = []
  const root = realpathSync(runtimeGoDir)
  const rootStat = lstatSync(root)
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) {
    throw new Error('Go runtime source root must be a real directory.')
  }
  appendRuntimeGoSourceFingerprintEntries(root, '', entries)
  if (entries.length === 0) {
    throw new Error('Go runtime source root contains no build inputs.')
  }
  return sha256(entries.sort().join('\n'))
}

function appendRuntimeGoSourceFingerprintEntries(root: string, relativeDir: string, entries: string[]): void {
  const dir = relativeDir ? join(root, relativeDir) : root
  const dirEntries = readdirSync(dir)
  for (const entry of dirEntries) {
    if (entry === '.git' || entry === '.cache' || entry === 'bin' || entry === 'node_modules') {
      continue
    }
    const relativePath = relativeDir ? join(relativeDir, entry) : entry
    const fullPath = join(root, relativePath)
    const stat = lstatSync(fullPath)
    if (stat.isSymbolicLink()) {
      throw new Error(`Go runtime source must not contain symbolic links: ${relativePath}`)
    }
    if (stat.isDirectory()) {
      appendRuntimeGoSourceFingerprintEntries(root, relativePath, entries)
      continue
    }
    if (!entry.endsWith('.go') && entry !== 'go.mod' && entry !== 'go.sum') {
      continue
    }
    if (!stat.isFile()) {
      throw new Error(`Go runtime build input must be a regular file: ${relativePath}`)
    }
    const canonicalPath = relativePath.replaceAll('\\', '/')
    entries.push(`${canonicalPath}:${stat.size}:${sha256File(fullPath)}`)
  }
}

type DevGoRuntimeCacheManifestV1 = {
  schemaVersion: 1
  sourceDigestSha256: string
  toolchainDigestSha256: string
  buildContractDigestSha256: string
  buildMode: 'go-build-trimpath-analytix-prod-v1'
  platform: NodeJS.Platform
  arch: string
  binaryName: string
  binarySha256: string
  binaryBytes: number
}

function sha256File(filePath: string): string {
  return createHash('sha256').update(readFileSync(filePath)).digest('hex')
}

function ensurePrivateDevGoCacheDirectoryV1(directory: string): string {
  mkdirSync(directory, { recursive: true, mode: 0o700 })
  const originalStat = lstatSync(directory)
  if (!originalStat.isDirectory() || originalStat.isSymbolicLink()) {
    throw new Error('Development Go runtime cache root must be a real directory.')
  }
  const canonical = realpathSync(directory)
  const stat = lstatSync(canonical)
  if (!stat.isDirectory() || stat.isSymbolicLink()) {
    throw new Error('Development Go runtime cache root must resolve to a real directory.')
  }
  if (process.platform !== 'win32') {
    if (typeof process.getuid === 'function' && stat.uid !== process.getuid()) {
      throw new Error('Development Go runtime cache root must be owned by the current user.')
    }
    chmodSync(canonical, 0o700)
    const privateStat = lstatSync(canonical)
    if ((privateStat.mode & 0o077) !== 0) {
      throw new Error('Development Go runtime cache root must not be accessible by group or other users.')
    }
  }
  return canonical
}

function syncDevGoCacheFileV1(filePath: string): void {
  const descriptor = openSync(filePath, 'r')
  try {
    fsyncSync(descriptor)
  } finally {
    closeSync(descriptor)
  }
}

function syncDevGoCacheDirectoryV1(directory: string): void {
  if (process.platform === 'win32') return
  syncDevGoCacheFileV1(directory)
}

function parseDevGoRuntimeCacheManifestV1(value: unknown): DevGoRuntimeCacheManifestV1 | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const record = value as Record<string, unknown>
  const expectedKeys = [
    'arch',
    'binaryBytes',
    'binaryName',
    'binarySha256',
    'buildContractDigestSha256',
    'buildMode',
    'platform',
    'schemaVersion',
    'sourceDigestSha256',
    'toolchainDigestSha256'
  ]
  if (Object.keys(record).sort().join('\n') !== expectedKeys.join('\n')) return null
  if (record.schemaVersion !== 1 || record.buildMode !== 'go-build-trimpath-analytix-prod-v1') return null
  if (!isSha256(record.sourceDigestSha256) || !isSha256(record.toolchainDigestSha256) ||
      !isSha256(record.buildContractDigestSha256) || !isSha256(record.binarySha256)) return null
  if (typeof record.platform !== 'string' || typeof record.arch !== 'string' || !record.arch) return null
  if (typeof record.binaryName !== 'string' || !record.binaryName) return null
  if (typeof record.binaryBytes !== 'number' || !Number.isSafeInteger(record.binaryBytes) || record.binaryBytes <= 0) return null
  return record as DevGoRuntimeCacheManifestV1
}

function verifyDevGoRuntimeCacheV1(
  outputDir: string,
  expected: Omit<DevGoRuntimeCacheManifestV1, 'binarySha256' | 'binaryBytes'>
): string {
  const canonicalOutputDir = ensurePrivateDevGoCacheDirectoryV1(outputDir)
  const output = join(canonicalOutputDir, expected.binaryName)
  const manifestPath = join(canonicalOutputDir, DEV_GO_RUNTIME_CACHE_MANIFEST_V1)
  let manifest: DevGoRuntimeCacheManifestV1 | null = null
  try {
    const manifestStat = lstatSync(manifestPath)
    const outputStat = lstatSync(output)
    if (!manifestStat.isFile() || manifestStat.isSymbolicLink() ||
        !outputStat.isFile() || outputStat.isSymbolicLink()) {
      throw new Error('cache files must be regular files')
    }
    if (process.platform !== 'win32') {
      if (typeof process.getuid === 'function' &&
          (manifestStat.uid !== process.getuid() || outputStat.uid !== process.getuid())) {
        throw new Error('cache files must be owned by the current user')
      }
      if ((outputStat.mode & 0o111) === 0) throw new Error('cached runtime server is not executable')
    }
    manifest = parseDevGoRuntimeCacheManifestV1(JSON.parse(readFileSync(manifestPath, 'utf8')))
    if (!manifest) throw new Error('cache manifest is invalid')
    for (const [key, value] of Object.entries(expected)) {
      if (manifest[key as keyof DevGoRuntimeCacheManifestV1] !== value) {
        throw new Error(`cache manifest field mismatch: ${key}`)
      }
    }
    if (outputStat.size !== manifest.binaryBytes || sha256File(output) !== manifest.binarySha256) {
      throw new Error('cached runtime server digest mismatch')
    }
  } catch {
    throw new Error('Cached Go runtime server failed closed identity verification.')
  }
  return output
}

function ensureDevGoRuntimeServerBinary(options: {
  runtimeGoDir: string
  env: NodeJS.ProcessEnv
  goBinaryCandidates?: string[]
  platform?: NodeJS.Platform
}): string {
  const goBinary = resolveGoBinary(options.env, options.goBinaryCandidates)
  const canonicalGoBinary = realpathSync(goBinary)
  const toolchainStat = lstatSync(canonicalGoBinary)
  if (!toolchainStat.isFile() || toolchainStat.isSymbolicLink()) {
    throw new Error('Go toolchain entrypoint must resolve to a regular file.')
  }
  const sourceDigestSha256 = runtimeGoSourceFingerprint(options.runtimeGoDir)
  const toolchainDigestSha256 = sha256File(canonicalGoBinary)
  const platform = options.platform ?? process.platform
  const binaryName = runtimeServerBinaryName(platform)
  const arch = process.arch
  const buildContractDigestSha256 = sha256(canonicalJSONString({
    schemaVersion: 1,
    sourceDigestSha256,
    toolchainDigestSha256,
    buildMode: 'go-build-trimpath-analytix-prod-v1',
    cachePolicy: options.env.ANALYTIX_DEV_CACHE_ROOT ? 'repository-cache-v1' : 'ordinary-v1',
    platform,
    arch,
    binaryName
  }))
  const expected = {
    schemaVersion: 1 as const,
    sourceDigestSha256,
    toolchainDigestSha256,
    buildContractDigestSha256,
    buildMode: 'go-build-trimpath-analytix-prod-v1' as const,
    platform,
    arch,
    binaryName
  }
  const cacheVolume = options.env.ANALYTIX_DEV_CACHE_ROOT
    ? (require(join(appRoot(), 'scripts/lib/development-cache-environment.cjs')) as {
      openVerifiedDevelopmentCacheVolume: () => { verify: () => unknown; close: () => void }
    }).openVerifiedDevelopmentCacheVolume() : undefined
  let buildDir = ''
  try {
    const cacheRoot = ensurePrivateDevGoCacheDirectoryV1(
      devGoRuntimeServerCacheRoot(options.runtimeGoDir, options.env)
    )
    const outputDir = join(cacheRoot, buildContractDigestSha256)
    if (existsSync(outputDir)) return verifyDevGoRuntimeCacheV1(outputDir, expected)
    buildDir = ensurePrivateDevGoCacheDirectoryV1(mkdtempSync(join(cacheRoot, '.build-')))
    cacheVolume?.verify()
    const output = join(buildDir, binaryName)
    const result = spawnSync(canonicalGoBinary, [
      'build',
      '-trimpath',
      '-tags',
      'analytix_prod',
      '-o',
      output,
      './cmd/runtime-server'
    ], {
      cwd: realpathSync(options.runtimeGoDir),
      encoding: 'utf8',
      env: buildDevGoToolchainEnvV1(options.env),
      timeout: 120_000
    })
    cacheVolume?.verify()
    if (result.status !== 0 || !existsSync(output)) {
      const stderr = String(result.stderr || result.stdout || '').slice(-2_000)
      throw new Error(`Failed to build cached Go runtime server binary: ${stderr ? privateTextDiagnosticText(stderr) : `exit ${result.status}`}`)
    }
    const outputStat = lstatSync(output)
    if (!outputStat.isFile() || outputStat.isSymbolicLink() || outputStat.size <= 0) {
      throw new Error('Go build did not produce a regular non-empty runtime server binary.')
    }
    if (process.platform !== 'win32') chmodSync(output, 0o700)
    if (runtimeGoSourceFingerprint(options.runtimeGoDir) !== sourceDigestSha256 ||
        sha256File(canonicalGoBinary) !== toolchainDigestSha256) {
      throw new Error('Go runtime source or toolchain changed during the development build.')
    }
    const builtStat = lstatSync(output)
    const manifest: DevGoRuntimeCacheManifestV1 = {
      ...expected,
      binarySha256: sha256File(output),
      binaryBytes: builtStat.size
    }
    const manifestPath = join(buildDir, DEV_GO_RUNTIME_CACHE_MANIFEST_V1)
    writeFileSync(manifestPath, `${canonicalJSONString(manifest)}\n`, {
      encoding: 'utf8',
      flag: 'wx',
      mode: 0o600
    })
    syncDevGoCacheFileV1(output)
    syncDevGoCacheFileV1(manifestPath)
    syncDevGoCacheDirectoryV1(buildDir)
    try {
      renameSync(buildDir, outputDir)
      buildDir = ''
      syncDevGoCacheDirectoryV1(cacheRoot)
    } catch (error) {
      if (!existsSync(outputDir)) throw error
    }
    return verifyDevGoRuntimeCacheV1(outputDir, expected)
  } finally {
    try { if (buildDir) rmSync(buildDir, { recursive: true, force: true }) }
    finally { cacheVolume?.close() }
  }
}

export function resolveGoRuntimeLaunchTarget(options: {
  runtimeServer: boolean
  appIsPackaged?: boolean
  appPath?: string
  platform?: NodeJS.Platform
  env?: NodeJS.ProcessEnv
  runtimeGoDir?: string
  goBinaryCandidates?: string[]
}): GoRuntimeLaunchTarget {
  const env = options.env ?? process.env
  const explicitServerBin = options.runtimeServer
    ? env.ANALYTIX_GO_RUNTIME_SERVER_BIN?.trim() ?? ''
    : ''
  const appIsPackaged = options.appIsPackaged ?? app.isPackaged
  if (!options.runtimeServer && appIsPackaged) {
    throw new Error('Go contract sidecar is test/conformance-only and is disabled in packaged Analytix.')
  }
  if (appIsPackaged && explicitServerBin) {
    throw new Error('Packaged Analytix rejects ANALYTIX_GO_RUNTIME_SERVER_BIN; the bundled runtime-server is the only production Agent Core.')
  }
  if (explicitServerBin || (options.runtimeServer && appIsPackaged)) {
    const command = explicitServerBin || resolveBundledGoRuntimeServerPath(
      options.appPath ?? app.getAppPath(),
      options.platform ?? process.platform
    )
    if (!existsSync(command)) {
      throw new Error(`Packaged Go runtime server binary is missing at ${command}`)
    }
    return {
      command,
      argsPrefix: [],
      mode: 'bundled-binary'
    }
  }

  const runtimeGoDir = options.runtimeGoDir ?? getGoRuntimeDir()
  if (options.runtimeServer) {
    return {
      command: ensureDevGoRuntimeServerBinary({
        runtimeGoDir,
        env,
        goBinaryCandidates: options.goBinaryCandidates,
        platform: options.platform
      }),
      argsPrefix: [],
      mode: 'bundled-binary'
    }
  }

	const command = './cmd/contract-sidecar'
  return {
    command: resolveGoBinary(env, options.goBinaryCandidates),
    argsPrefix: [
      'run',
      ...(options.runtimeServer ? ['-tags', 'analytix_prod'] : []),
      command
    ],
    cwd: runtimeGoDir,
    mode: 'go-run-source'
  }
}

export function buildDesktopPrivateHistoryMigrationArgsV2(
  dataDir: string,
  userDataDir: string
): string[] {
  const exactDataDir = resolve(dataDir)
  return [
    'migration',
    'migrate-desktop-private-history-v2',
    '--data-dir',
    exactDataDir,
    '--durable-root',
    exactDataDir,
    ...buildGoRuntimeStartupUserDataArgsV1(userDataDir)
  ]
}

export function buildGoRuntimeStartupUserDataArgsV1(userDataDir: string): string[] {
  return ['--user-data-dir', resolve(userDataDir)]
}

export function buildDesktopPrivateHistoryMigrationEnvV2(
  env: NodeJS.ProcessEnv = process.env
): NodeJS.ProcessEnv {
  const childEnvironment: NodeJS.ProcessEnv = {}
  for (const name of [
    'HOME',
    'USERPROFILE',
    'APPDATA',
    'LOCALAPPDATA',
    'XDG_CONFIG_HOME',
    'TMPDIR',
    'TEMP',
    'TMP',
    'SystemRoot',
    'SYSTEMROOT',
    'WINDIR',
    'PATH',
    'LANG',
    'LC_ALL',
    'TZ'
  ]) {
    if (env[name] !== undefined) childEnvironment[name] = env[name]
  }
  childEnvironment.ANALYTIX_APP_ROOT = appRoot()
  childEnvironment.ANALYTIX_RESOURCES_PATH = appResourcesPath()
  return childEnvironment
}

export function isExactDesktopPrivateHistoryMigrationReadyV2(stdout: Buffer): boolean {
  return Buffer.isBuffer(stdout) &&
    stdout.equals(Buffer.from(DESKTOP_PRIVATE_HISTORY_MIGRATION_READY_V2, 'utf8'))
}

export async function migrateDesktopPrivateHistoryBeforeStartV2(
  settings: AppSettingsV1,
  userDataDir: string
): Promise<void> {
  const runtime = resolveAnalytixRuntimeSettings(settings)
  const dataDir = resolveAnalytixDataDir(runtime)
  const launchTarget = resolveGoRuntimeLaunchTarget({
    runtimeServer: true,
    runtimeGoDir: getGoRuntimeDir()
  })
  resolveDarwinSecretStoreKeychainBindingV1({
    boundary: desktopExternalStateBoundaryForGoRuntime,
    dataDir
  })
  const child = spawn(
    launchTarget.command,
    [...launchTarget.argsPrefix, ...buildDesktopPrivateHistoryMigrationArgsV2(dataDir, userDataDir)],
    {
      cwd: launchTarget.cwd,
      env: buildDesktopPrivateHistoryMigrationEnvV2(process.env),
      stdio: ['ignore', 'pipe', 'pipe']
    }
  )
  await waitForDesktopPrivateHistoryMigrationV2(child)
}

export function waitForDesktopPrivateHistoryMigrationV2(
  child: ChildProcess,
  timeoutMs = DESKTOP_PRIVATE_HISTORY_MIGRATION_TIMEOUT_MS_V2,
  terminationGraceMs = 5_000
): Promise<void> {
	return new Promise((resolveReady, rejectReady) => {
		let stdout = Buffer.alloc(0)
		let stderrBytes = 0
		let settled = false
		let failureRequested = false
		let timeout: NodeJS.Timeout | undefined
		let terminationTimeout: NodeJS.Timeout | undefined
		const cleanup = (): void => {
			if (timeout) clearTimeout(timeout)
			if (terminationTimeout) clearTimeout(terminationTimeout)
			child.stdout?.off('data', onStdout)
			child.stderr?.off('data', onStderr)
			child.off('error', onError)
			child.off('close', onClose)
		}
		const finishAfterClose = (error?: Error): void => {
			if (settled) return
			settled = true
			cleanup()
			stdout.fill(0)
			stdout = Buffer.alloc(0)
			if (error) rejectReady(error)
			else resolveReady()
		}
		const requestFailure = (): void => {
			if (failureRequested || settled) return
			failureRequested = true
			if (child.exitCode === null && child.signalCode === null) {
				try {
					child.kill('SIGKILL')
				} catch {
					// The bounded termination deadline below still keeps startup fail-closed.
				}
			}
			terminationTimeout = setTimeout(() => {
				if (settled) return
				child.stdout?.destroy()
				child.stderr?.destroy()
				try {
					child.unref()
				} catch {
					// Rejection remains authoritative even for incomplete ChildProcess test doubles.
				}
				finishAfterClose(new Error('Desktop private history migration termination could not be confirmed.'))
			}, Math.max(1, terminationGraceMs))
		}
		const onStdout = (chunk: Buffer | string): void => {
			const incoming = Buffer.isBuffer(chunk) ? Buffer.from(chunk) : Buffer.from(chunk, 'utf8')
			if (settled) {
				incoming.fill(0)
				return
			}
			if (failureRequested || stdout.length + incoming.length > MAX_GO_MIGRATION_OUTPUT_BYTES) {
				incoming.fill(0)
				requestFailure()
				return
			}
			const combined = Buffer.concat([stdout, incoming])
			stdout.fill(0)
			incoming.fill(0)
			stdout = combined
		}
		const onStderr = (chunk: Buffer | string): void => {
			if (settled) return
			stderrBytes = Math.min(
				MAX_GO_MIGRATION_OUTPUT_BYTES + 1,
				stderrBytes + Buffer.byteLength(chunk)
			)
			if (stderrBytes > MAX_GO_MIGRATION_OUTPUT_BYTES) requestFailure()
		}
		const onError = (): void => requestFailure()
		const onClose = (code: number | null, signal: NodeJS.Signals | null): void => {
			const valid = !failureRequested && code === 0 && signal === null && stderrBytes === 0 &&
				isExactDesktopPrivateHistoryMigrationReadyV2(stdout)
			finishAfterClose(valid ? undefined : new Error('Desktop private history migration did not complete.'))
		}
		child.stdout?.on('data', onStdout)
		child.stderr?.on('data', onStderr)
		child.once('error', onError)
		child.once('close', onClose)
		timeout = setTimeout(requestFailure, Math.max(1, timeoutMs))
	})
}

function isGoSidecarRunning(): boolean {
  return isOwnedProcessRunning(goSidecarOwnedProcess)
}

export function captureCurrentFinalPublicationAuthorityPin(): ManagedFinalPublicationAuthorityPinV1 | null {
  const pin = goSidecarFinalPublicationAuthorityPin
  if (!pin || !isGoSidecarRunning() || goSidecarOwnedProcess?.pid !== pin.runtimePid) return null
  return { ...pin }
}

export function isCurrentFinalPublicationAuthorityPin(
  pin: ManagedFinalPublicationAuthorityPinV1 | null
): pin is ManagedFinalPublicationAuthorityPinV1 {
  const current = goSidecarFinalPublicationAuthorityPin
  return Boolean(pin && current && isGoSidecarRunning() &&
    goSidecarOwnedProcess?.pid === pin.runtimePid && pin.runtimePid === current.runtimePid &&
    pin.runtimeUrl === current.runtimeUrl && pin.generation === current.generation &&
    pin.keyId === current.keyId && pin.publicKey === current.publicKey)
}

function appendGoStderrTail(chunk: string): void {
  goSidecarStderrTail = `${goSidecarStderrTail}${chunk}`.slice(-16_384)
}

export function resolveGoRuntimeConfiguredDurableRoot(options: {
  backend: AnalytixRuntimeGoBackendId
  dataDir: string
  candidateDurableRoot?: string
}): { durableRoot: string, ownsDurableRoot: false } | null {
  const dataDir = options.dataDir.trim()
  if (options.backend === 'go-runtime-default') {
    return { durableRoot: dataDir, ownsDurableRoot: false }
  }
  const candidateDurableRoot = options.backend === 'go-runtime-candidate'
    ? options.candidateDurableRoot?.trim() ?? ''
    : ''
  if (!candidateDurableRoot) return null
  if (!candidateDurableRoot.includes(GO_RUNTIME_CANDIDATE_DURABLE_ROOT_MARKER)) {
    throw new Error(`ANALYTIX_GO_RUNTIME_CANDIDATE_DURABLE_ROOT must include ${GO_RUNTIME_CANDIDATE_DURABLE_ROOT_MARKER}`)
  }
  return { durableRoot: candidateDurableRoot, ownsDurableRoot: false }
}

function privateTextDiagnostic(text: string): Pick<AnalytixUnexpectedExitInfo, 'stderrBytes' | 'stderrSha256'> {
  const stderrBytes = Buffer.byteLength(text, 'utf8')
  return {
    stderrBytes,
    stderrSha256: createHash('sha256').update(text, 'utf8').digest('hex')
  }
}

function privateTextDiagnosticText(text: string): string {
  const diagnostic = privateTextDiagnostic(text)
  return `stderr_bytes=${diagnostic.stderrBytes}, stderr_sha256=${diagnostic.stderrSha256}`
}

function goStderrDiagnostic(): Pick<AnalytixUnexpectedExitInfo, 'stderrBytes' | 'stderrSha256'> {
  return privateTextDiagnostic(goSidecarStderrTail)
}

function goStderrDiagnosticText(): string {
  const diagnostic = goStderrDiagnostic()
  return diagnostic.stderrBytes > 0
    ? `stderr_bytes=${diagnostic.stderrBytes}, stderr_sha256=${diagnostic.stderrSha256}`
    : 'stderr_bytes=0'
}

export function setGoRuntimeUnexpectedExitHandler(
  handler: ((info: AnalytixUnexpectedExitInfo) => void) | null
): void {
  onUnexpectedGoRuntimeExit = handler
}

export type GoSidecarExitObserver = {
  markReady: () => void
  markIntentionalStop: () => void
}

export function observeGoSidecarExit(
  child: ChildProcess,
  options: { superviseUnexpectedExit: boolean }
): GoSidecarExitObserver {
  child.once('exit', (code, signal) => {
    const reportUnexpectedExit =
      options.superviseUnexpectedExit &&
      readyGoSidecars.has(child) &&
      !intentionalGoSidecarStops.has(child)
    const ownedProcess = goSidecarOwnedProcess?.process === child
      ? goSidecarOwnedProcess
      : null
    if (ownedProcess) {
      goSidecarRuntimeToken = ''
      goSidecarFinalPublicationAuthorityPin = null
      clearBundledFundsMaterializationCurrentRuntimeV1()
      void stopGoConformanceSidecarAndWait().catch((error) => {
        lastBackendFallbackReason = redactedErrorMessage(error)
      })
    }
    if (reportUnexpectedExit) {
      onUnexpectedGoRuntimeExit?.({
        code: code ?? null,
        signal: signal ?? null,
        ...goStderrDiagnostic()
      })
    }
  })
  return {
    markReady: () => {
      readyGoSidecars.add(child)
    },
    markIntentionalStop: () => {
      intentionalGoSidecarStops.add(child)
    }
  }
}

function waitForGoConformanceReady(
  child: ChildProcess,
  readyPrefix = GO_CONFORMANCE_READY_PREFIX,
  runtimeLabel = 'Go conformance sidecar',
  requireTokenEcho = true,
  timeoutMs = GO_CONFORMANCE_STARTUP_TIMEOUT_MS
): Promise<{
  url: string
  runtimePid?: number
  runtimeToken?: string
  runtimeTokenConfigured?: boolean
  persistenceRootsConfigured?: boolean
  productionRuntime?: boolean
  controlledArtifactHostV2Configured?: boolean
  controlledArtifactHostV2Ready?: boolean
  controlledArtifactHostV2BackendGeneration?: number
  controlledArtifactHostV2LaunchBindingProof?: string
  finalPublicationAuthorityKeyId?: string
  finalPublicationAuthorityPublicKey?: string
  witnessedAuthorityV2Configured?: boolean
  witnessedAuthorityInstallationId?: string
  witnessedAuthorityKeyId?: string
  witnessedAuthorityManifestDigest?: string
  datasetSnapshotSelectionV2Configured?: boolean
  datasetSnapshotAdmissionV2State?: 'absent'
  datasetSnapshotAdmissionV2InstallationId?: string
  datasetSnapshotAdmissionV2RuntimeLaunchNonce?: string
  datasetSnapshotAdmissionV2StagingBindingDigest?: string
  datasetSnapshotAdmissionV2SelectionDigest?: string
  datasetSnapshotAdmissionV2SnapshotId?: string
  datasetSnapshotAdmissionV2AuthorityRecordDigest?: string
  datasetSnapshotAdmissionV2AckHmacSha256?: string
}> {
  return new Promise((resolve, reject) => {
    if (!child.stdout) {
      reject(new Error(`${runtimeLabel} stdout is unavailable.`))
      return
    }
    const stdoutStream = child.stdout
    let stdout = Buffer.alloc(0)
    let settled = false
    const settle = (fn: () => void): void => {
      if (settled) return
      settled = true
      clearTimeout(timer)
      stdoutStream.off('data', onStdout)
      child.off('exit', onExit)
      child.off('error', onError)
      stdout.fill(0)
      stdout = Buffer.alloc(0)
      fn()
    }
    const onStdout = (chunk: Buffer | string): void => {
      const incoming = Buffer.isBuffer(chunk) ? Buffer.from(chunk) : Buffer.from(chunk, 'utf8')
      if (stdout.length + incoming.length > MAX_GO_READY_STDOUT_BYTES) {
        incoming.fill(0)
        settle(() => reject(new Error(`${runtimeLabel} emitted an invalid ready payload.`)))
        return
      }
      const combined = Buffer.concat([stdout, incoming])
      stdout.fill(0)
      incoming.fill(0)
      stdout = combined
      let line: string | null
      try {
        line = takeExactGoRuntimeReadyLine(stdout, readyPrefix)
      } catch {
        settle(() => reject(new Error(`${runtimeLabel} emitted an invalid ready payload.`)))
        return
      }
      if (line === null) return
      settle(() => {
        try {
          const parsed = (requireTokenEcho
            ? parseStrictJsonObject(Buffer.from(line.slice(readyPrefix.length), 'utf8'), {
                maxBytes: 4096,
                maxDepth: 2,
                maxTokens: 96,
                maxStringBytes: 2048,
                maxNumberBytes: 32
              })
            : parseGoRuntimeServerReadyPayload(line.slice(readyPrefix.length))) as Record<string, unknown>
          if (
            typeof parsed.url !== 'string' || !runtimeReadyURLIsLoopback(parsed.url) ||
            (requireTokenEcho
              ? typeof parsed.runtimeToken !== 'string'
              : false)
          ) {
            reject(new Error(`${runtimeLabel} emitted an invalid ready payload.`))
            return
          }
          const onUnexpectedStdout = (value: Buffer | string): void => {
            if (Buffer.byteLength(value) === 0 || child.exitCode !== null || child.signalCode !== null) return
            void stopGoConformanceSidecarAndWait().catch((error) => {
              lastBackendFallbackReason = redactedErrorMessage(error)
            })
          }
          stdoutStream.on('data', onUnexpectedStdout)
          child.once('exit', () => stdoutStream.off('data', onUnexpectedStdout))
          resolve({
            url: parsed.url,
            ...(typeof parsed.runtimePid === 'number' ? { runtimePid: parsed.runtimePid } : {}),
            ...(typeof parsed.runtimeToken === 'string' ? { runtimeToken: parsed.runtimeToken } : {}),
            ...(typeof parsed.runtimeTokenConfigured === 'boolean'
              ? { runtimeTokenConfigured: parsed.runtimeTokenConfigured }
              : {}),
            ...(typeof parsed.persistenceRootsConfigured === 'boolean'
              ? { persistenceRootsConfigured: parsed.persistenceRootsConfigured }
              : {}),
            ...(typeof parsed.productionRuntime === 'boolean'
              ? { productionRuntime: parsed.productionRuntime }
              : {}),
            ...(typeof parsed.controlledArtifactHostV2Configured === 'boolean'
              ? { controlledArtifactHostV2Configured: parsed.controlledArtifactHostV2Configured }
              : {}),
            ...(typeof parsed.controlledArtifactHostV2Ready === 'boolean'
              ? { controlledArtifactHostV2Ready: parsed.controlledArtifactHostV2Ready }
              : {}),
            ...(typeof parsed.controlledArtifactHostV2BackendGeneration === 'number'
              ? { controlledArtifactHostV2BackendGeneration: parsed.controlledArtifactHostV2BackendGeneration }
              : {}),
            ...(typeof parsed.controlledArtifactHostV2LaunchBindingProof === 'string'
              ? { controlledArtifactHostV2LaunchBindingProof: parsed.controlledArtifactHostV2LaunchBindingProof }
              : {}),
            ...(typeof parsed.finalPublicationAuthorityKeyId === 'string'
              ? { finalPublicationAuthorityKeyId: parsed.finalPublicationAuthorityKeyId }
              : {}),
            ...(typeof parsed.finalPublicationAuthorityPublicKey === 'string'
              ? { finalPublicationAuthorityPublicKey: parsed.finalPublicationAuthorityPublicKey }
              : {}),
            ...(typeof parsed.witnessedAuthorityV2Configured === 'boolean'
              ? { witnessedAuthorityV2Configured: parsed.witnessedAuthorityV2Configured }
              : {}),
            ...(typeof parsed.witnessedAuthorityInstallationId === 'string'
              ? { witnessedAuthorityInstallationId: parsed.witnessedAuthorityInstallationId }
              : {}),
            ...(typeof parsed.witnessedAuthorityKeyId === 'string'
              ? { witnessedAuthorityKeyId: parsed.witnessedAuthorityKeyId }
              : {}),
            ...(typeof parsed.witnessedAuthorityManifestDigest === 'string'
              ? { witnessedAuthorityManifestDigest: parsed.witnessedAuthorityManifestDigest }
              : {}),
            ...(typeof parsed.datasetSnapshotSelectionV2Configured === 'boolean'
              ? { datasetSnapshotSelectionV2Configured: parsed.datasetSnapshotSelectionV2Configured }
              : {}),
            ...(parsed.datasetSnapshotAdmissionV2State === 'absent'
              ? { datasetSnapshotAdmissionV2State: parsed.datasetSnapshotAdmissionV2State }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2InstallationId === 'string'
              ? { datasetSnapshotAdmissionV2InstallationId: parsed.datasetSnapshotAdmissionV2InstallationId }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2RuntimeLaunchNonce === 'string'
              ? { datasetSnapshotAdmissionV2RuntimeLaunchNonce: parsed.datasetSnapshotAdmissionV2RuntimeLaunchNonce }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2StagingBindingDigest === 'string'
              ? { datasetSnapshotAdmissionV2StagingBindingDigest: parsed.datasetSnapshotAdmissionV2StagingBindingDigest }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2SelectionDigest === 'string'
              ? { datasetSnapshotAdmissionV2SelectionDigest: parsed.datasetSnapshotAdmissionV2SelectionDigest }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2SnapshotId === 'string'
              ? { datasetSnapshotAdmissionV2SnapshotId: parsed.datasetSnapshotAdmissionV2SnapshotId }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2AuthorityRecordDigest === 'string'
              ? { datasetSnapshotAdmissionV2AuthorityRecordDigest: parsed.datasetSnapshotAdmissionV2AuthorityRecordDigest }
              : {}),
            ...(typeof parsed.datasetSnapshotAdmissionV2AckHmacSha256 === 'string'
              ? { datasetSnapshotAdmissionV2AckHmacSha256: parsed.datasetSnapshotAdmissionV2AckHmacSha256 }
              : {})
          })
        } catch (error) {
          reject(error)
        }
      })
    }
    const onExit = (code: number | null): void => {
      settle(() => {
        reject(
          new Error(
            `${runtimeLabel} exited before ready with code ${code ?? 'unknown'}. ${goStderrDiagnosticText()}`
          )
        )
      })
    }
    const onError = (): void => {
      settle(() => {
        reject(new Error(`${runtimeLabel} failed before ready. ${goStderrDiagnosticText()}`))
      })
    }
    const timer = setTimeout(() => {
      settle(() => {
        reject(new Error(`${runtimeLabel} did not become ready. ${goStderrDiagnosticText()}`))
      })
    }, Math.max(1, timeoutMs))
    stdoutStream.on('data', onStdout)
    child.once('exit', onExit)
    child.once('error', onError)
  })
}

export function takeExactGoRuntimeReadyLine(body: Buffer, readyPrefix: string): string | null {
  if (!Buffer.isBuffer(body) || body.length > MAX_GO_READY_STDOUT_BYTES ||
    typeof readyPrefix !== 'string' || readyPrefix === '' || readyPrefix.includes('\n') || readyPrefix.includes('\r')) {
    throw new Error('Go runtime ready output is invalid.')
  }
  const newline = body.indexOf(0x0a)
  if (newline < 0) return null
  if (newline === 0 || newline !== body.length - 1 || body.indexOf(0x0a, newline + 1) >= 0) {
    throw new Error('Go runtime ready output is invalid.')
  }
  const lineBytes = body.subarray(0, newline)
  const line = lineBytes.toString('utf8')
  if (line.includes('\r') || !line.startsWith(readyPrefix) ||
    !Buffer.from(line, 'utf8').equals(lineBytes)) {
    throw new Error('Go runtime ready output is invalid.')
  }
  return line
}

export type GoRuntimeServerReadyPayloadV2 = {
  url: string
  runtimePid: number
  runtimeTokenConfigured: boolean
  persistenceRootsConfigured: true
  productionRuntime: true
  controlledArtifactHostV2Configured: boolean
  controlledArtifactHostV2Ready: boolean
  controlledArtifactHostV2BackendGeneration: number
  controlledArtifactHostV2LaunchBindingProof: string
  finalPublicationAuthorityKeyId: string
  finalPublicationAuthorityPublicKey: string
  witnessedAuthorityV2Configured: boolean
  witnessedAuthorityInstallationId: string
  witnessedAuthorityKeyId: string
  witnessedAuthorityManifestDigest: string
  datasetSnapshotSelectionV2Configured: false
  datasetSnapshotAdmissionV2State: 'absent'
  datasetSnapshotAdmissionV2InstallationId: ''
  datasetSnapshotAdmissionV2RuntimeLaunchNonce: ''
  datasetSnapshotAdmissionV2StagingBindingDigest: ''
  datasetSnapshotAdmissionV2SelectionDigest: ''
  datasetSnapshotAdmissionV2SnapshotId: ''
  datasetSnapshotAdmissionV2AuthorityRecordDigest: ''
  datasetSnapshotAdmissionV2AckHmacSha256: ''
}

export function parseGoRuntimeServerReadyPayload(input: string): GoRuntimeServerReadyPayloadV2 {
  const parsed = parseStrictJsonObject(Buffer.from(input, 'utf8'), {
    maxBytes: 4096,
    maxDepth: 2,
    maxTokens: 96,
    maxStringBytes: 2048,
    maxNumberBytes: 32
  })
  const keys = [
    'controlledArtifactHostV2BackendGeneration',
    'controlledArtifactHostV2Configured',
    'controlledArtifactHostV2LaunchBindingProof',
    'controlledArtifactHostV2Ready',
    'datasetSnapshotAdmissionV2AckHmacSha256',
    'datasetSnapshotAdmissionV2AuthorityRecordDigest',
    'datasetSnapshotAdmissionV2InstallationId',
    'datasetSnapshotAdmissionV2RuntimeLaunchNonce',
    'datasetSnapshotAdmissionV2SelectionDigest',
    'datasetSnapshotAdmissionV2SnapshotId',
    'datasetSnapshotAdmissionV2StagingBindingDigest',
    'datasetSnapshotAdmissionV2State',
    'datasetSnapshotSelectionV2Configured',
    'finalPublicationAuthorityKeyId',
    'finalPublicationAuthorityPublicKey',
    'persistenceRootsConfigured',
    'productionRuntime',
    'runtimePid',
    'runtimeTokenConfigured',
    'url',
    'witnessedAuthorityInstallationId',
    'witnessedAuthorityKeyId',
    'witnessedAuthorityManifestDigest',
    'witnessedAuthorityV2Configured'
  ]
  const exactKeys = Object.keys(parsed).sort()
  const authorityPublicKey = typeof parsed.finalPublicationAuthorityPublicKey === 'string'
    ? Buffer.from(parsed.finalPublicationAuthorityPublicKey, 'base64url')
    : Buffer.alloc(0)
  const authorityShapeValid = typeof parsed.finalPublicationAuthorityKeyId === 'string' &&
    /^[a-f0-9]{64}$/.test(parsed.finalPublicationAuthorityKeyId) && authorityPublicKey.length === 32 &&
    authorityPublicKey.toString('base64url') === parsed.finalPublicationAuthorityPublicKey &&
    createHash('sha256').update(authorityPublicKey).digest('hex') === parsed.finalPublicationAuthorityKeyId
  const controlledArtifactShapeValid = parsed.controlledArtifactHostV2Configured === false
    ? parsed.controlledArtifactHostV2BackendGeneration === 0 &&
      parsed.controlledArtifactHostV2Ready === false &&
      parsed.controlledArtifactHostV2LaunchBindingProof === ''
    : parsed.controlledArtifactHostV2Configured === true &&
      typeof parsed.controlledArtifactHostV2Ready === 'boolean' &&
      Number.isSafeInteger(parsed.controlledArtifactHostV2BackendGeneration) &&
      Number(parsed.controlledArtifactHostV2BackendGeneration) > 0 &&
      /^[a-f0-9]{64}$/.test(String(parsed.controlledArtifactHostV2LaunchBindingProof))
  const witnessedAuthorityShapeValid = parsed.witnessedAuthorityV2Configured === false
    ? parsed.witnessedAuthorityInstallationId === '' &&
      parsed.witnessedAuthorityKeyId === '' &&
      parsed.witnessedAuthorityManifestDigest === ''
    : parsed.witnessedAuthorityV2Configured === true &&
      /^[a-f0-9]{64}$/.test(String(parsed.witnessedAuthorityInstallationId)) &&
      /^[a-f0-9]{64}$/.test(String(parsed.witnessedAuthorityKeyId)) &&
      /^[a-f0-9]{64}$/.test(String(parsed.witnessedAuthorityManifestDigest))
  const datasetSnapshotAbsent =
    parsed.datasetSnapshotSelectionV2Configured === false &&
    parsed.datasetSnapshotAdmissionV2State === 'absent' &&
    parsed.datasetSnapshotAdmissionV2InstallationId === '' &&
    parsed.datasetSnapshotAdmissionV2RuntimeLaunchNonce === '' &&
    parsed.datasetSnapshotAdmissionV2StagingBindingDigest === '' &&
    parsed.datasetSnapshotAdmissionV2SelectionDigest === '' &&
    parsed.datasetSnapshotAdmissionV2SnapshotId === '' &&
    parsed.datasetSnapshotAdmissionV2AuthorityRecordDigest === '' &&
    parsed.datasetSnapshotAdmissionV2AckHmacSha256 === ''
  if (
    exactKeys.length !== keys.length || exactKeys.some((key, index) => key !== keys[index]) ||
    typeof parsed.url !== 'string' || !runtimeReadyURLIsLoopback(parsed.url) ||
    !Number.isSafeInteger(parsed.runtimePid) || Number(parsed.runtimePid) <= 0 ||
    typeof parsed.runtimeTokenConfigured !== 'boolean' ||
    parsed.persistenceRootsConfigured !== true || parsed.productionRuntime !== true ||
    !controlledArtifactShapeValid || !authorityShapeValid ||
    !witnessedAuthorityShapeValid || !datasetSnapshotAbsent
  ) {
    throw new Error('Go runtime server emitted an invalid ready payload.')
  }
  return parsed as GoRuntimeServerReadyPayloadV2
}

export function verifyWitnessedAuthorityStartupIdentity(
  ready: Pick<
  GoRuntimeServerReadyPayloadV2,
  | 'witnessedAuthorityV2Configured'
  | 'witnessedAuthorityInstallationId'
  | 'witnessedAuthorityKeyId'
  | 'witnessedAuthorityManifestDigest'
  >,
  expectedAuthority: ReturnType<typeof authorityAnchorProjectionV1> | null
): void {
  if (ready.witnessedAuthorityV2Configured !== Boolean(expectedAuthority) ||
    ready.witnessedAuthorityInstallationId !== (expectedAuthority?.installationId ?? '') ||
    ready.witnessedAuthorityKeyId !== (expectedAuthority?.authorityKeyId ?? '') ||
    ready.witnessedAuthorityManifestDigest !== (expectedAuthority?.currentManifestDigest ?? '')) {
    throw new Error(
      'Go runtime witnessed authority identity does not match the main-owned startup frame.'
    )
  }
}

function runtimeReadyURLIsLoopback(value: string): boolean {
  try {
    const parsed = new URL(value)
    const hostname = parsed.hostname.toLowerCase().replace(/^\[|\]$/g, '')
    const port = Number(parsed.port)
    return parsed.protocol === 'http:' && parsed.username === '' && parsed.password === '' &&
      parsed.pathname === '/' && parsed.search === '' && parsed.hash === '' &&
      (hostname === '127.0.0.1' || hostname === '::1' || hostname === 'localhost') &&
      Number.isInteger(port) && port > 0 && port <= 65535
  } catch {
    return false
  }
}

async function startGoConformanceSidecar(
  settings: AppSettingsV1,
  backend: AnalytixRuntimeGoBackendId
): Promise<void> {
  if (goSidecarStopPromise) await goSidecarStopPromise
  if (goSidecarStopFailure) {
    throw new Error(
      `Previous Go runtime cleanup was not confirmed. ${redactedErrorMessage(goSidecarStopFailure)}`
    )
  }
  if (goSidecarStartPromise) return goSidecarStartPromise
  if (isGoSidecarRunning()) return
  if (goSidecarOwnedProcess) await stopGoConformanceSidecarAndWait()

  const promise = startGoConformanceSidecarOnce(settings, backend).finally(() => {
    if (goSidecarStartPromise === promise) goSidecarStartPromise = null
  })
  goSidecarStartPromise = promise
  return promise
}

export function shouldRetryWithoutOptionalCaseAuthorityV1(input: {
  runtimeServer: boolean
  authorityAttempted: boolean
  readyPayloadReceived: boolean
  fallbackAlreadyUsed: boolean
}): boolean {
  return input.runtimeServer &&
    input.authorityAttempted &&
    !input.readyPayloadReceived &&
    !input.fallbackAlreadyUsed
}

export async function retryWithoutOptionalCaseAuthorityV1<T>(
  retry: boolean,
  originalError: unknown,
  fallback: () => Promise<T>,
  report: () => void = () => {
    logWarn(
      'runtime-capability',
      'Optional case authority activation failed; restarting the same Agent without that capability.',
      { capability: 'case_authority' }
    )
  }
): Promise<T> {
  if (!retry) throw originalError
  try {
    report()
  } catch {
    // Optional capability diagnostics cannot become a general startup gate.
  }
  return fallback()
}

async function startGoConformanceSidecarOnce(
  settings: AppSettingsV1,
  backend: AnalytixRuntimeGoBackendId,
  fallbackAlreadyUsed = false
): Promise<void> {
  const runtime = resolveAnalytixRuntimeSettings(settings)
  const dataDir = resolveAnalytixDataDir(runtime)
  const runtimeInsecure = isAnalytixRuntimeInsecure(runtime)
  const runtimeToken = resolveManagedGoRuntimeToken(runtime)

  const runtimeGoDir = getGoRuntimeDir()
  const isRuntimeServer = backend === 'go-runtime-candidate' || backend === 'go-runtime-default'
  const isRuntimeDefault = backend === 'go-runtime-default'
  const launchTarget = resolveGoRuntimeLaunchTarget({
    runtimeServer: isRuntimeServer,
    runtimeGoDir
  })
  const darwinSecretStoreKeychainBindingV1 = isRuntimeServer
    ? resolveDarwinSecretStoreKeychainBindingV1({
        boundary: desktopExternalStateBoundaryForGoRuntime,
        dataDir
      })
    : null
  if (launchTarget.mode === 'go-run-source' && !existsSync(join(runtimeGoDir, 'go.mod'))) {
    throw new Error(`Go runtime source is missing at ${runtimeGoDir}`)
  }
  let hostScheduleMcpBindingV1: RuntimeHostScheduleMcpBindingV1 | null = null
  const syncRuntimeConfig = async (
    bundledFundsMaterialization?: BundledFundsMaterializationBindingV1
  ): Promise<void> => {
    const synced = await syncGuiManagedAnalytixConfig(dataDir, runtime, {
      scheduleMcp: {
        settings,
        launch: {
          appPath: app.getAppPath(),
          execPath: process.execPath,
          isPackaged: app.isPackaged
        }
      },
      ...(bundledFundsMaterialization ? { bundledFundsMaterialization } : {})
    })
    hostScheduleMcpBindingV1 = synced.hostScheduleMcpBindingV1
  }
  let bundledFundsConfigSynced = false
  const bundledFundsMaterialization = isRuntimeServer
    ? await settleOptionalRuntimeCapability('bundled_funds', async () => {
        const materialization = await materializeBundledFundsBeforeRuntimeV1({
          appIsPackaged: app.isPackaged,
          launchTarget,
          dataDir
        })
        if (materialization) {
          await syncRuntimeConfig(materialization)
          bundledFundsConfigSynced = true
        }
        return materialization
      })
    : null
  const mainOwnedAuthority = isRuntimeServer && !fallbackAlreadyUsed
    ? await takeMainOwnedRuntimeAuthorityForLaunchV1()
    : null
  const expectedWitnessedAuthority = mainOwnedAuthority
    ? authorityAnchorProjectionV1(mainOwnedAuthority.authorityAnchorV1)
    : null
  const fixturesDir = isRuntimeServer ? '' : getGoRuntimeFixturesDir()
  if (!isRuntimeServer && !existsSync(fixturesDir)) {
    throw new Error(`Go conformance fixtures are missing at ${fixturesDir}`)
  }
  const configuredDurableRoot = resolveGoRuntimeConfiguredDurableRoot({
    backend,
    dataDir,
    candidateDurableRoot: process.env.ANALYTIX_GO_RUNTIME_CANDIDATE_DURABLE_ROOT
  })
  if (configuredDurableRoot) {
    goSidecarDurableTempDir = configuredDurableRoot.durableRoot
    goSidecarOwnsDurableRoot = configuredDurableRoot.ownsDurableRoot
  } else {
    goSidecarDurableTempDir = await mkdtemp(join(
      tmpdir(),
      isRuntimeServer ? 'analytix-go-runtime-candidate-' : 'analytix-go-conformance-'
    ))
    goSidecarOwnsDurableRoot = true
  }
  if (!goSidecarDurableTempDir) {
    throw new Error('Go runtime durable root was not initialized.')
  }
  const durableRoot = goSidecarDurableTempDir
  goSidecarStderrTail = ''
  const args = [
    ...launchTarget.argsPrefix,
    '--addr',
    `127.0.0.1:${runtime.port}`
  ]
  if (!isRuntimeServer) {
    args.push('--runtime-token', runtimeToken)
    args.push('--fixtures-dir', fixturesDir)
  }
  if (runtimeInsecure) {
    args.push('--insecure')
  }
  let hasPrivateStartupFrame = false
  if (isRuntimeServer) {
    if (!bundledFundsConfigSynced) await syncRuntimeConfig()
    hostScheduleMcpBindingV1 = selectRuntimeHostScheduleMcpBindingV1(
      mainOwnedAuthority,
      hostScheduleMcpBindingV1,
      darwinSecretStoreKeychainBindingV1
    )
    hasPrivateStartupFrame = Boolean(
      mainOwnedAuthority || hostScheduleMcpBindingV1 || darwinSecretStoreKeychainBindingV1
    )
    if (isRuntimeDefault) {
      args.push('--durable-root', durableRoot)
    } else {
      args.push('--runtime-durable-root', durableRoot)
    }
    args.push('--data-dir', dataDir)
    args.push(...buildGoRuntimeStartupUserDataArgsV1(app.getPath('userData')))
    args.push(...buildGoRuntimeProviderArgs(settings, runtime))
    args.push('--mcp-config-path', resolveGoRuntimeMCPConfigPath(dataDir))
    if (hasPrivateStartupFrame) {
      args.push('--private-startup-frame-v1')
    }
  } else {
    args.push('--durable-temp-dir', durableRoot)
  }
  const child = spawn(
    launchTarget.command,
    args,
    {
      cwd: launchTarget.cwd,
      env: buildGoRuntimeSidecarEnv(
        settings,
        runtime,
        dataDir,
        process.env,
        runtimeToken
      ),
      stdio: [
        hasPrivateStartupFrame ? 'pipe' : 'ignore',
        'pipe',
        'pipe'
      ],
      detached: process.platform !== 'win32'
    }
  )
  const ownedProcess = ownSpawnedProcess(child, {
    detached: process.platform !== 'win32'
  })
  goSidecarOwnedProcess = ownedProcess
  goSidecarStopFailure = null
  goSidecarRuntimeToken = runtimeToken
  goSidecarFinalPublicationAuthorityPin = null
  const launchGeneration = ++goSidecarGeneration
  child.stderr?.on('data', (chunk) => appendGoStderrTail(String(chunk)))
  const exitObserver = observeGoSidecarExit(child, { superviseUnexpectedExit: isRuntimeServer })
  let readyPayloadReceived = false

  try {
    if (hasPrivateStartupFrame) {
      await writeRuntimeStartupPrivateFrameV1(child.stdin, {
        ...(mainOwnedAuthority ? { protectedAuthorityV1: mainOwnedAuthority } : {}),
        ...(darwinSecretStoreKeychainBindingV1
          ? { darwinSecretStoreKeychainBindingV1 }
          : {}),
        ...(hostScheduleMcpBindingV1 ? { hostScheduleMcpBindingV1 } : {})
      })
    }
    const runtimeLabel = isRuntimeServer ? 'Go runtime server' : 'Go conformance sidecar'
    const ready = await waitForGoConformanceReady(
      child,
      isRuntimeServer ? GO_RUNTIME_SERVER_READY_PREFIX : GO_CONFORMANCE_READY_PREFIX,
      runtimeLabel,
      !isRuntimeServer,
      isRuntimeServer ? GO_RUNTIME_SERVER_STARTUP_TIMEOUT_MS_V1 : GO_CONFORMANCE_STARTUP_TIMEOUT_MS
    )
    readyPayloadReceived = true
    if (!isRuntimeServer && ready.runtimeToken !== runtimeToken) {
      throw new Error(`${runtimeLabel} ready token does not match the requested runtime token.`)
    }
    if (isRuntimeServer && (!Number.isSafeInteger(child.pid) || Number(child.pid) <= 0 ||
      ready.runtimePid !== child.pid)) {
      throw new Error(`${runtimeLabel} ready process identity does not match the spawned runtime.`)
    }
    if (isRuntimeServer && ready.runtimeTokenConfigured !== (runtimeToken !== '' || !runtimeInsecure)) {
      throw new Error(`${runtimeLabel} ready token configuration does not match the requested mode.`)
    }
    if (isRuntimeServer && (ready.persistenceRootsConfigured !== true || ready.productionRuntime !== true)) {
      throw new Error(`${runtimeLabel} did not prove production persistence readiness.`)
    }
    if (isRuntimeServer) {
      verifyWitnessedAuthorityStartupIdentity(
        ready as GoRuntimeServerReadyPayloadV2,
        expectedWitnessedAuthority
      )
    }
    if (isRuntimeServer && new URL(ready.url).origin !== new URL(`http://127.0.0.1:${runtime.port}`).origin) {
      throw new Error(`${runtimeLabel} ready endpoint does not match the requested runtime endpoint.`)
    }
    if (isRuntimeServer && (typeof ready.finalPublicationAuthorityKeyId !== 'string' ||
      typeof ready.finalPublicationAuthorityPublicKey !== 'string')) {
      throw new Error(`${runtimeLabel} omitted its final publication authority identity.`)
    }
    if (isRuntimeServer) {
      const authorityKeyId = ready.finalPublicationAuthorityKeyId as string
      const authorityPublicKey = ready.finalPublicationAuthorityPublicKey as string
      goSidecarFinalPublicationAuthorityPin = {
        keyId: authorityKeyId,
        publicKey: authorityPublicKey,
        runtimePid: ready.runtimePid as number,
        runtimeUrl: new URL(ready.url).origin,
        generation: launchGeneration
      }
    }
    exitObserver.markReady()
    if (isRuntimeDefault) {
      if (bundledFundsMaterialization) {
        bindBundledFundsMaterializationToCurrentRuntimeV1(bundledFundsMaterialization)
      } else {
        clearBundledFundsMaterializationCurrentRuntimeV1()
      }
      activeBackend = backend
      lastBackendFallbackReason = ''
      return
    }
    const canary = await probeGoConformanceRuntimeCanary(settings, ready.url, backend)
    if (!canary.ok) {
      throw new Error(`${canary.failedCheck}: ${canary.message}`)
    }
    activeBackend = backend
    lastBackendFallbackReason = ''
  } catch (error) {
    const retryWithoutAuthority = shouldRetryWithoutOptionalCaseAuthorityV1({
      runtimeServer: isRuntimeServer,
      authorityAttempted: mainOwnedAuthority !== null,
      readyPayloadReceived,
      fallbackAlreadyUsed
    })
    await stopGoConformanceSidecarAndWait()
    return retryWithoutOptionalCaseAuthorityV1(
      retryWithoutAuthority,
      error,
      () => startGoConformanceSidecarOnce(settings, backend, true)
    )
  }
}

async function stopGoConformanceSidecarAndWait(): Promise<void> {
  if (goSidecarStopPromise) return goSidecarStopPromise
  const current = goSidecarOwnedProcess
  const promise = stopGoConformanceSidecarOnce(current)
    .catch((error) => {
      goSidecarStopFailure = error instanceof Error
        ? error
        : new Error('Go runtime process-tree cleanup failed.')
      throw error
    })
    .finally(() => {
      if (goSidecarStopPromise === promise) goSidecarStopPromise = null
    })
  goSidecarStopPromise = promise
  return promise
}

async function stopGoConformanceSidecarOnce(
  current: OwnedProcessHandle | null
): Promise<void> {
  goSidecarRuntimeToken = ''
  goSidecarFinalPublicationAuthorityPin = null
  clearBundledFundsMaterializationCurrentRuntimeV1()
  activeBackend = 'go-runtime-default'

  if (current) {
    intentionalGoSidecarStops.add(current.process)
    await stopOwnedProcess(current)
    if (goSidecarOwnedProcess === current) goSidecarOwnedProcess = null
  }

  if (goSidecarDurableTempDir && goSidecarOwnsDurableRoot) {
    await rm(goSidecarDurableTempDir, { recursive: true, force: true }).catch(() => undefined)
  }
  goSidecarDurableTempDir = null
  goSidecarOwnsDurableRoot = false
  goSidecarStopFailure = null
}

async function fetchTextWithTimeout(
  url: string,
  headers?: Headers,
  init?: Omit<RequestInit, 'headers' | 'signal'>
): Promise<{ status: number; ok: boolean; text: string }> {
  const res = await fetch(url, {
    ...init,
    headers,
    signal: AbortSignal.timeout(2_000)
  })
  return { status: res.status, ok: res.ok, text: await res.text() }
}

function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {}
}

function runtimeCapabilityDiagnosticValid(value: unknown): boolean {
  const capability = recordValue(value)
  const available = capability.available
  const enabled = capability.enabled
  const reasonCode = capability.reasonCode
  if (typeof available !== 'boolean') return false
  if (enabled !== undefined && typeof enabled !== 'boolean') return false
  if (available === true) return reasonCode === 'available'
  return reasonCode === 'disabled_by_config' || reasonCode === 'unavailable'
}

function parseJSONRecord(text: string): Record<string, unknown> {
  return recordValue(JSON.parse(text) as unknown)
}

type StrictRuntimeSseReplay =
  | { ok: true; payloads: Record<string, unknown>[] }
  | { ok: false; reason: string }

function strictRuntimeSseReplay(text: string, threadId: string): StrictRuntimeSseReplay {
  const filter = new PublicRuntimeEventFilter()
  const payloads: Record<string, unknown>[] = []
  let buffer = text
  let previousSeq = 0
  while (true) {
    const next = takePublicRuntimeSseBlock(buffer)
    if (next === null) break
    buffer = next.rest
    const decision = projectPublicRuntimeSseBlock(next.block, threadId, filter)
    if (decision === null) continue
    if (decision.status !== 'emit') {
      const eventName = /^event: ([a-z][a-z0-9_]*)$/m.exec(next.block)?.[1] ?? 'unknown'
      const reason = decision.status === 'invalid' ? decision.reason : decision.status
      return { ok: false, reason: `${reason}:${eventName}:${payloads.length + 1}` }
    }
    if (decision.event.kind === 'accepted_final_batch' ||
        decision.event.kind === 'general_terminal_batch') {
      const firstSeq = decision.event.firstSeq
      const lastSeq = decision.event.lastSeq
      if (typeof firstSeq !== 'number' || typeof lastSeq !== 'number' ||
          firstSeq !== previousSeq + 1 || lastSeq !== decision.seq) {
        return { ok: false, reason: 'non_contiguous_sequence' }
      }
    } else if (decision.seq !== previousSeq + 1) {
      return { ok: false, reason: 'non_contiguous_sequence' }
    }
    previousSeq = decision.seq
    payloads.push(decision.event)
  }
  if (buffer.trim() !== '') return { ok: false, reason: 'truncated_frame' }
  return { ok: true, payloads }
}

function runtimeJSONHeaders(headers: Headers): Headers {
  const next = new Headers(headers)
  next.set('Content-Type', 'application/json')
  next.set('Accept', 'application/json')
  return next
}

function goRuntimeG6ReadinessPassed(status: GoRuntimeG6ReadinessStatus): boolean {
  return status.ready === true &&
    status.explicitReadyGate === true &&
    status.defaultGoBackendEnabled === true &&
    status.rendererVisibleGoSwitcher === false &&
    status.durableRestartEvidence.status === 'passed' &&
    status.providerMatrix.status === 'passed' &&
    status.mcpMatrix.status === 'passed' &&
    status.packagedQa.status === 'passed' &&
    status.operatorGate.status === 'passed' &&
    status.missingRequiredChecks.length === 0
}

function goRuntimeProductDefaultReady(runtimeInfoBody: Record<string, unknown>): { ok: boolean, missing: string[] } {
  const serializedInfo = JSON.stringify(runtimeInfoBody)
  const capabilities = recordValue(runtimeInfoBody.capabilities)
  const mcp = recordValue(capabilities.mcp)
  const subagents = recordValue(capabilities.subagents)
  const missing: string[] = []
  if (capabilities.upstreamAbsorption !== undefined) missing.push('capabilities.upstreamAbsorption:hidden')
  const legacyMcpLocalMarker = ['mcp', 'Local', 'Pr', 'oof'].join('')
  if (serializedInfo.includes(legacyMcpLocalMarker)) missing.push(`${legacyMcpLocalMarker}:hidden`)
  if (serializedInfo.includes('reasonix-capability-audit')) missing.push('reasonix-capability-audit:hidden')
  if (serializedInfo.includes('Reasonix')) missing.push('Reasonix:hidden')
  if (runtimeInfoBody.schemaVersion !== 2) missing.push('schemaVersion')
  if (runtimeInfoBody.listenerScope !== 'loopback') missing.push('listenerScope')
  if (typeof capabilities.contractVersion !== 'number') missing.push('capabilities.contractVersion')
  if (typeof runtimeInfoBody.startedAt !== 'string') missing.push('startedAt')
  const storage = recordValue(runtimeInfoBody.storage)
  if (storage.configured !== true || storage.available !== true) missing.push('storage')
  if (mcp.available !== true && !runtimeCapabilityDiagnosticValid(mcp)) missing.push('capabilities.mcp.diagnostic')
  if (subagents.available !== true && !runtimeCapabilityDiagnosticValid(subagents)) missing.push('capabilities.subagents.diagnostic')
  return { ok: missing.length === 0, missing }
}

async function fetchRuntimeJSON(
  baseUrl: string,
  path: string,
  headers: Headers,
  init?: Omit<RequestInit, 'headers' | 'signal'>
): Promise<{ status: number; ok: boolean; text: string; body: Record<string, unknown> }> {
  const response = await fetchTextWithTimeout(
    `${baseUrl}${path}`,
    init?.body ? runtimeJSONHeaders(headers) : headers,
    init
  )
  return {
    ...response,
    body: response.text.trim() ? parseJSONRecord(response.text) : {}
  }
}

function redactedExcerpt(text: string, maxLength: number): string {
  return sanitizePublicSerializedText(redactSecretText(text)).slice(0, maxLength)
}

function redactedErrorMessage(error: unknown): string {
  return sanitizePublicSerializedText(redactSecretText(error instanceof Error ? error.message : String(error)))
}

export async function probeGoConformanceRuntimeCanary(
  settings: AppSettingsV1,
  baseUrl = getRuntimeBaseUrlForSettings(settings),
  backend: AnalytixRuntimeGoBackendId = 'go-conformance',
  g6Readiness: GoRuntimeG6ReadinessStatus = resolveGoRuntimeG6ReadinessStatus()
): Promise<GoConformanceRuntimeCanaryResult> {
  const checks: string[] = []
  const headers = runtimeAuthHeaders(settings)

  try {
    const health = await fetchTextWithTimeout(`${baseUrl}/health`)
    if (!health.ok || !isAnalytixHealthResponseBody(health.text)) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'health',
        message: `expected analytix health response, got ${health.status}: ${redactedExcerpt(health.text, 200)}`
      }
    }
    checks.push('health')

    const threads = await fetchTextWithTimeout(`${baseUrl}/v1/threads?limit=1`, headers)
    const threadsBody = threads.ok ? parseJSONRecord(threads.text) : {}
    if (!threads.ok || !Array.isArray(threadsBody.threads)) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'thread-api',
        message: `expected thread list response, got ${threads.status}: ${redactedExcerpt(threads.text, 200)}`
      }
    }
    checks.push('thread-api')

    if (backend === 'go-runtime-candidate' || backend === 'go-runtime-default') {
      const runtimeSettings = resolveAnalytixRuntimeSettings(settings)
      const canaryModel = runtimeSettings.model.trim() || 'deepseek-chat'
      const canaryProviderId = runtimeSettings.providerId.trim()
      const canaryModelRequest = {
        model: canaryModel,
        ...(canaryProviderId ? { providerId: canaryProviderId } : {})
      }
      const runtimeInfo = await fetchRuntimeJSON(baseUrl, '/v1/runtime/info', headers)
      const parsedRuntimeInfo = RuntimeInfoResponseSchema.safeParse(runtimeInfo.body)
      const capabilities = parsedRuntimeInfo.success
        ? recordValue(parsedRuntimeInfo.data.capabilities)
        : {}
      const infoCheckId = backend === 'go-runtime-default' ? 'go-runtime-default-info' : 'go-runtime-candidate-info'
      if (!runtimeInfo.ok ||
        !parsedRuntimeInfo.success ||
        parsedRuntimeInfo.data.listenerScope !== 'loopback' ||
        !runtimeCapabilityDiagnosticValid(capabilities.mcp) ||
        !runtimeCapabilityDiagnosticValid(capabilities.subagents)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: infoCheckId,
          message: `expected Go runtime public diagnostics v2 contract, got status ${runtimeInfo.status}`
        }
      }
      checks.push(infoCheckId)
      const runtimeTools = await fetchRuntimeJSON(baseUrl, '/v1/runtime/tools', headers)
      const parsedRuntimeTools = RuntimeToolsResponseSchema.safeParse(runtimeTools.body)
      const toolsCheckId = backend === 'go-runtime-default' ? 'go-runtime-default-tools' : 'go-runtime-candidate-tools'
      if (!runtimeTools.ok || !parsedRuntimeTools.success) {
        return {
          ok: false,
          baseUrl,
          failedCheck: toolsCheckId,
          message: `expected Go runtime public tool diagnostics v2 contract, got status ${runtimeTools.status}`
        }
      }
      checks.push(toolsCheckId)
      if (backend === 'go-runtime-default') {
        const productDefault = goRuntimeProductDefaultReady(recordValue(parsedRuntimeInfo.data))
        if (!productDefault.ok) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-default-product-readiness',
            message: `expected product-clean Go default readiness, missing ${productDefault.missing.join(', ')}`
          }
        }
        checks.push('go-runtime-default-product-readiness', 'post-cutover-live-validation-pending')
      } else if (!goRuntimeG6ReadinessPassed(g6Readiness)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-g6-readiness',
          message: `expected formal Go runtime readiness evidence before accepting Go runtime-candidate, got ${redactedExcerpt(JSON.stringify(g6Readiness), 600)}`
        }
      } else {
        checks.push(
          'go-runtime-candidate-g6-readiness',
          'durable-restart-evidence',
          'provider-matrix-status',
          'mcp-matrix-status',
          'packaged-qa-status'
        )
      }

      const runtimeCanaryPrefix = backend === 'go-runtime-default' ? 'go-runtime-default' : 'go-runtime-candidate'
      const canaryTitle = backend === 'go-runtime-default' ? 'Go Runtime Default Canary' : 'Go Runtime Candidate Canary'
      const canaryThread = await fetchRuntimeJSON(
        baseUrl,
        '/v1/threads',
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            title: canaryTitle,
            workspace: '/tmp/analytix-go-runtime-candidate',
            ...canaryModelRequest,
            mode: 'agent'
          })
        }
      )
      const threadId = typeof canaryThread.body.id === 'string' ? canaryThread.body.id : ''
      if (!canaryThread.ok || canaryThread.status !== 201 || !threadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-thread-create`,
          message: `expected isolated canary thread creation, got ${canaryThread.status}: ${redactedExcerpt(canaryThread.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-thread-create`)
      const thread = await fetchRuntimeJSON(baseUrl, `/v1/threads/${encodeURIComponent(threadId)}`, headers)
      if (!thread.ok || thread.body.id !== threadId || typeof thread.body.latestSeq !== 'number') {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-thread-read`,
          message: `expected thread read contract, got ${thread.status}: ${redactedExcerpt(thread.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-thread-read`)

      const fork = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}/fork`,
        headers,
        { method: 'POST', body: JSON.stringify({ relation: 'side', title: `${canaryTitle} Side` }) }
      )
      const forkThreadId = typeof fork.body.id === 'string' ? fork.body.id : ''
      if (!fork.ok || fork.status !== 201 || fork.body.relation !== 'side' || fork.body.parentThreadId !== threadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-fork`,
          message: `expected fork contract, got ${fork.status}: ${redactedExcerpt(fork.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-fork`)

      const resume = await fetchRuntimeJSON(
        baseUrl,
        `/v1/sessions/${encodeURIComponent(threadId)}/resume-thread`,
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            workspace: '/tmp/analytix-go-runtime-candidate',
            ...canaryModelRequest,
            mode: 'agent'
          })
        }
      )
      const resumedThreadId = typeof resume.body.thread_id === 'string' ? resume.body.thread_id : ''
      if (!resume.ok || resume.status !== 201 || resume.body.session_id !== threadId || !resumedThreadId) {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-resume`,
          message: `expected resume contract, got ${resume.status}: ${redactedExcerpt(resume.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-resume`)

      const patch = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}`,
        headers,
        { method: 'PATCH', body: JSON.stringify({ title: canaryTitle, status: 'archived' }) }
      )
      if (!patch.ok || patch.body.title !== canaryTitle || patch.body.status !== 'archived') {
        return {
          ok: false,
          baseUrl,
          failedCheck: `${runtimeCanaryPrefix}-patch`,
          message: `expected patch contract, got ${patch.status}: ${redactedExcerpt(patch.text, 400)}`
        }
      }
      checks.push(`${runtimeCanaryPrefix}-patch`)

      if (backend === 'go-runtime-default') {
        const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
        const internalCandidateRoute = await fetchTextWithTimeout(`${baseUrl}/v1/internal/go-production-candidate/boundary`, headers)
        if (forbiddenRoute.status !== 404 || internalCandidateRoute.status !== 404) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-default-hidden-routes',
            message: `expected Go public/internal candidate routes hidden, got /v1/runtime/go ${forbiddenRoute.status} and internal candidate ${internalCandidateRoute.status}`
          }
        }
        checks.push('renderer-visible-go-route-hidden', 'go-production-internal-route-hidden')

        const cleanupIds = [...new Set([threadId, forkThreadId, resumedThreadId].filter(Boolean))]
        for (const cleanupId of cleanupIds) {
          const cleanup = await fetchRuntimeJSON(
            baseUrl,
            `/v1/threads/${encodeURIComponent(cleanupId)}`,
            headers,
            { method: 'DELETE' }
          )
          if (!cleanup.ok || cleanup.body.deleted !== true) {
            return {
              ok: false,
              baseUrl,
              failedCheck: 'go-runtime-default-thread-delete',
              message: `expected canary thread cleanup for ${cleanupId}, got ${cleanup.status}: ${redactedExcerpt(cleanup.text, 400)}`
            }
          }
        }
        checks.push('go-runtime-default-thread-delete')

        return { ok: true, baseUrl, checks }
      }

      const turn = await fetchRuntimeJSON(
        baseUrl,
        `/v1/threads/${encodeURIComponent(threadId)}/turns`,
        headers,
        {
          method: 'POST',
          body: JSON.stringify({
            prompt: 'Run the Go runtime candidate contract canary.',
            ...canaryModelRequest
          })
        }
      )
      const turnId = typeof turn.body.turnId === 'string' ? turn.body.turnId : ''
      if (!turn.ok || turn.status !== 202 || turn.body.threadId !== threadId || !turnId || typeof turn.body.userMessageItemId !== 'string') {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-turn',
          message: `expected turn-create contract, got ${turn.status}: ${redactedExcerpt(turn.text, 400)}`
        }
      }
      checks.push('go-runtime-candidate-turn')

      const replay = await fetchTextWithTimeout(`${baseUrl}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, headers)
      const requiredReplayTokens = [
        'event: turn_started',
        'event: item_created',
        'event: approval_requested',
        'event: user_input_requested',
        'event: tool_catalog_changed',
        'event: accepted_final_batch',
        '"publicationSlot":"assistant-final"',
        '"publicationSlot":"usage"',
        '"cacheHitTokens":700',
        'event: pipeline_stage',
        '"parentThreadId":"' + threadId + '"',
        '"publicationSlot":"terminal"'
      ]
      if (replay.text.includes('event: assistant_text_delta')) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-assistant-draft-exposed',
          message: 'runtime candidate replay exposed a non-authoritative assistant draft'
        }
      }
      if (containsPrivateReasoningContent(replay.text)) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-private-reasoning',
          message: 'runtime candidate replay exposed private reasoning'
        }
      }
      const strictReplay = strictRuntimeSseReplay(replay.text, threadId)
      if (!strictReplay.ok) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-sse-replay',
          message: `runtime candidate returned an invalid SSE replay: ${strictReplay.reason}`
        }
      }
      const replayPayloads = strictReplay.payloads
      const targetBatches = replayPayloads.filter((payload) =>
        payload.kind === 'accepted_final_batch' &&
        payload.threadId === threadId &&
        payload.turnId === turnId
      )
      const targetBatch = targetBatches[0]
      const targetBatchEvents = Array.isArray(targetBatch?.events)
        ? targetBatch.events.filter((event): event is Record<string, unknown> => (
            Boolean(event) && typeof event === 'object' && !Array.isArray(event)
          ))
        : []
      const atomicAssistantFinals = targetBatchEvents.filter((payload) => {
        const item = recordValue(payload.item)
        return payload.kind === 'item_completed' &&
          payload.threadId === threadId &&
          payload.turnId === turnId &&
          payload.itemId === `item_${turnId}_assistant` &&
          item.id === payload.itemId &&
          item.threadId === threadId &&
          item.turnId === turnId &&
          item.kind === 'assistant_text' &&
          item.role === 'assistant' &&
          item.status === 'completed' &&
          typeof item.text === 'string' && item.text.length > 0
      })
      const targetUsages = targetBatchEvents.filter((payload) => {
        const usage = recordValue(payload.usage)
        return payload.kind === 'usage' &&
          payload.threadId === threadId &&
          payload.turnId === turnId &&
          payload.publicationSlot === 'usage' &&
          usage.cacheHitTokens === 700
      })
      const targetTerminals = targetBatchEvents.filter((payload) =>
        payload.kind === 'turn_completed' &&
        payload.threadId === threadId &&
        payload.turnId === turnId &&
        payload.status === 'completed' &&
        payload.publicationSlot === 'terminal'
      )
      if (!replay.ok || !requiredReplayTokens.every((token) => replay.text.includes(token)) ||
        targetBatches.length !== 1 || targetBatchEvents.length !== 3 ||
        atomicAssistantFinals.length !== 1 || targetUsages.length !== 1 || targetTerminals.length !== 1) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-sse-replay',
          message: `expected one sealed assistant-final/usage/terminal publication batch in the turn event replay contract, got ${replay.status}: ${redactedExcerpt(replay.text, 600)}`
        }
      }
      if (replayPayloads.some((payload) => payload.kind === 'assistant_text_delta' || (
        payload.kind === 'item_completed' && recordValue(payload.item).kind === 'assistant_text'
      ))) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-assistant-draft-exposed',
          message: 'runtime candidate replay exposed a non-authoritative assistant draft'
        }
      }
      if (replayPayloads.some((payload) => containsPrivateReasoningContent(payload))) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-private-reasoning',
          message: 'runtime candidate replay exposed private reasoning'
        }
      }
      checks.push('go-runtime-candidate-sse-replay')

      const approval = await fetchRuntimeJSON(
        baseUrl,
        `/v1/approvals/${encodeURIComponent(`appr_${turnId}`)}`,
        headers,
        { method: 'POST', body: JSON.stringify({ decision: 'deny' }) }
      )
      const userInput = await fetchRuntimeJSON(
        baseUrl,
        `/v1/user-inputs/${encodeURIComponent(`input_${turnId}`)}`,
        headers,
        { method: 'POST', body: JSON.stringify({ answers: [{ id: 'q1', label: 'Ship', value: 'yes' }] }) }
      )
      const resolvedReplay = await fetchTextWithTimeout(`${baseUrl}/v1/threads/${encodeURIComponent(threadId)}/events?since_seq=0`, headers)
      if (!approval.ok ||
        approval.body.status !== 'denied' ||
        !userInput.ok ||
        userInput.body.status !== 'submitted' ||
        !resolvedReplay.ok ||
        !resolvedReplay.text.includes('event: approval_resolved') ||
        !resolvedReplay.text.includes('event: user_input_resolved') ||
        resolvedReplay.text.includes('"answers"')) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-gates',
          message: `expected approval/user-input gate contract, got approval ${approval.status}, input ${userInput.status}, replay ${resolvedReplay.status}`
        }
      }
      checks.push('go-runtime-candidate-gates')

      const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
      const internalCandidateRoute = await fetchTextWithTimeout(`${baseUrl}/v1/internal/go-production-candidate/boundary`, headers)
      if (forbiddenRoute.status !== 404 || internalCandidateRoute.status !== 404) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-runtime-candidate-hidden-routes',
          message: `expected Go public/internal candidate routes hidden, got /v1/runtime/go ${forbiddenRoute.status} and internal candidate ${internalCandidateRoute.status}`
        }
      }
      checks.push('renderer-visible-go-route-hidden', 'go-production-internal-route-hidden')

      const cleanupIds = [...new Set([threadId, forkThreadId, resumedThreadId].filter(Boolean))]
      for (const cleanupId of cleanupIds) {
        const cleanup = await fetchRuntimeJSON(
          baseUrl,
          `/v1/threads/${encodeURIComponent(cleanupId)}`,
          headers,
          { method: 'DELETE' }
        )
        if (!cleanup.ok || cleanup.body.deleted !== true) {
          return {
            ok: false,
            baseUrl,
            failedCheck: 'go-runtime-candidate-thread-delete',
            message: `expected canary thread cleanup for ${cleanupId}, got ${cleanup.status}: ${redactedExcerpt(cleanup.text, 400)}`
          }
        }
      }
      checks.push('go-runtime-candidate-thread-delete')

      return { ok: true, baseUrl, checks }
    }

    const boundary = await fetchTextWithTimeout(`${baseUrl}/v1/conformance/loop/boundary`, headers)
    const boundaryBody = boundary.ok ? parseJSONRecord(boundary.text) : {}
    if (!boundary.ok ||
      boundaryBody.testConformanceOnly !== true ||
      boundaryBody.minimalAgentLoopPrototype !== true ||
      boundaryBody.fixtureBackedLoopOnly !== true ||
      boundaryBody.defaultGoBackendEnabled !== false ||
      boundaryBody.rendererVisibleGoRoutesAllowed !== false ||
      boundaryBody.reasonixPublicProtocolAllowed !== false ||
      boundaryBody.electronMainConnected !== false) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'go-conformance-boundary',
        message: `expected fixture-only Go boundary, got ${boundary.status}: ${redactedExcerpt(boundary.text, 400)}`
      }
    }
    checks.push('go-conformance-boundary')

    const forbiddenRoute = await fetchTextWithTimeout(`${baseUrl}/v1/runtime/go`, headers)
    if (forbiddenRoute.status !== 404) {
      return {
        ok: false,
        baseUrl,
        failedCheck: 'renderer-visible-go-route',
        message: `expected /v1/runtime/go to stay hidden, got ${forbiddenRoute.status}`
      }
    }
    checks.push('renderer-visible-go-route-hidden')

    if (backend === 'go-production-candidate') {
      const productionBoundary = await fetchTextWithTimeout(
        `${baseUrl}/v1/internal/go-production-candidate/boundary`,
        headers
      )
      const productionBoundaryBody = productionBoundary.ok
        ? parseJSONRecord(productionBoundary.text)
        : {}
      const hasRuntimeContractSlice = productionBoundaryBody.runtimeGoContractParitySlice === true ||
        productionBoundaryBody.productionCandidateGoRuntimeParitySlice === true
      const usesContractReplayProvider = productionBoundaryBody.usesContractReplayProviderServer === true
      if (!productionBoundary.ok ||
        !hasRuntimeContractSlice ||
        productionBoundaryBody.internalGateOnly !== true ||
        !usesContractReplayProvider ||
        productionBoundaryBody.usesRealDurableEventSink !== true ||
        productionBoundaryBody.defaultGoBackendEnabled !== false ||
        productionBoundaryBody.rendererVisibleGoRoutesAllowed !== false ||
        productionBoundaryBody.reasonixPublicProtocolAllowed !== false) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-production-candidate-boundary',
          message: `expected production-candidate Go boundary, got ${productionBoundary.status}: ${redactedExcerpt(productionBoundary.text, 400)}`
        }
      }
      checks.push('go-production-candidate-boundary')

      const productionCanary = await fetchTextWithTimeout(
        `${baseUrl}/v1/internal/go-production-candidate/canary`,
        headers
      )
      const productionCanaryBody = productionCanary.ok
        ? parseJSONRecord(productionCanary.text)
        : {}
      const productionChecks = Array.isArray(productionCanaryBody.checks)
        ? productionCanaryBody.checks.filter((item): item is string => typeof item === 'string')
        : []
      const requiredProductionChecks: Array<string | string[]> = [
        'contract-provider-server',
        'durable-replay',
        'approval-user-input-manager',
        'contract-mcp-manager',
        'job-lineage',
        'single-baseline-checklist'
      ]
      if (!productionCanary.ok ||
        productionCanaryBody.ok !== true ||
        productionCanaryBody.defaultGoBackendEnabled !== false ||
        productionCanaryBody.rendererVisibleGoRoutesAllowed !== false ||
        productionCanaryBody.reasonixPublicProtocolAllowed !== false ||
        !requiredProductionChecks.every((check) => {
          const aliases = Array.isArray(check) ? check : [check]
          return aliases.some((alias) => productionChecks.includes(alias))
        })) {
        return {
          ok: false,
          baseUrl,
          failedCheck: 'go-production-candidate-canary',
          message: `expected production-candidate live provider/durable/gate/MCP/job canary, got ${productionCanary.status}: ${redactedExcerpt(productionCanary.text, 600)}`
        }
      }
      checks.push(...requiredProductionChecks.map((check) => Array.isArray(check) ? check[0] : check))
    }

    return { ok: true, baseUrl, checks }
  } catch (error) {
    return {
      ok: false,
      baseUrl,
      failedCheck: 'fetch',
      message: redactedErrorMessage(error)
    }
  }
}

export async function ensureGoDefaultBackend(
  settings: AppSettingsV1,
  startGoRuntime: (settings: AppSettingsV1, backend: AnalytixRuntimeGoBackendId) => Promise<void> = startGoConformanceSidecar
): Promise<void> {
  try {
    await startGoRuntime(settings, 'go-runtime-default')
  } catch (error) {
    const message = redactedErrorMessage(error)
    lastBackendFallbackReason = message
    await stopGoConformanceSidecarAndWait()
    activeBackend = 'go-runtime-default'
    lastBackendFallbackReason = message
    throw new Error(`Default Go runtime failed to start; TypeScript runtime fallback is retired. Cause: ${message}`)
  }
}

export const analytixRuntimeAdapter = {
  id: ANALYTIX_RUNTIME_ID,

  async resolveExecutable(settings: AppSettingsV1): Promise<string> {
    void settings
    const gate = resolveAnalytixRuntimeBackendGate()
    if (gate.backend === 'go-runtime-default') {
      try {
        const target = resolveGoRuntimeLaunchTarget({
          runtimeServer: true
        })
        return `Go default runtime server (${target.command})`
      } catch {
        return 'Go default runtime server (unresolved)'
      }
    }
    return `Unsupported Analytix runtime backend (${gate.requestedBackend || 'empty'}): ${gate.reason}`
  },

  ensureRunning(settings: AppSettingsV1): Promise<void> {
    const gate = resolveAnalytixRuntimeBackendGate()
    if (gate.backend === 'go-runtime-default') {
      return ensureGoDefaultBackend(settings)
    }
    lastBackendFallbackReason = gate.reason
    return Promise.reject(new Error(gate.reason))
  },

  stopAndWait(): Promise<void> {
    return stopGoConformanceSidecarAndWait()
  },

  isChildRunning(): boolean {
    return isGoSidecarRunning()
  },

  getBaseUrl(settings: AppSettingsV1): string {
    const runtime = getAnalytixRuntimeSettings(settings)
    return getAnalytixBaseUrl(runtime.port)
  },

  reclaimPort(port: number): Promise<{ ok: true } | { ok: false; message: string }> {
    return reclaimAnalytixPort(port)
  },

  resolveAvailablePort(port: number): Promise<{ port: number; changed: boolean; message?: string }> {
    return resolveAvailableAnalytixPort(port)
  }
}

export function getRuntimeBaseUrlForSettings(settings: AppSettingsV1): string {
  return analytixRuntimeAdapter.getBaseUrl(settings)
}

/** Build the bearer-token authorization header for Analytix requests. */
export function runtimeAuthHeaders(settings: AppSettingsV1): Headers {
  const runtime = getAnalytixRuntimeSettings(settings)
  const headers = new Headers()
  const runtimeToken = runtime.runtimeToken.trim() || (isGoSidecarRunning() ? goSidecarRuntimeToken : '')
  if (runtimeToken) {
    headers.set('Authorization', `Bearer ${runtimeToken}`)
  }
  return headers
}

export type RuntimeRequestInit = {
  method?: string
  body?: string
  headers?: Record<string, string>
}

function debugFingerprint(value: unknown): { sha256: string; bytes: number } | undefined {
  if (value === undefined || value === null) return undefined
  let serialized = ''
  try {
    serialized = JSON.stringify(value)
  } catch {
    serialized = String(value)
  }
  return {
    sha256: createHash('sha256').update(serialized).digest('hex'),
    bytes: Buffer.byteLength(serialized)
  }
}

function numericUsage(value: unknown): Record<string, number> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
  const usage: Record<string, number> = {}
  for (const [key, entry] of Object.entries(value as Record<string, unknown>)) {
    if (typeof entry === 'number' && Number.isFinite(entry)) usage[key] = entry
  }
  return Object.keys(usage).length > 0 ? usage : undefined
}

function sanitizeLlmDebugResponse(value: unknown): unknown {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return { rounds: [] }
  const rounds = (value as Record<string, unknown>).rounds
  if (!Array.isArray(rounds)) return { rounds: [] }
  return {
    rounds: rounds
      .filter((entry): entry is Record<string, unknown> =>
        Boolean(entry) && typeof entry === 'object' && !Array.isArray(entry)
      )
      .map((round) => {
        const output = round.output && typeof round.output === 'object' && !Array.isArray(round.output)
          ? round.output as Record<string, unknown>
          : {}
        const stopReason = output.stopReason === 'stop' || output.stopReason === 'tool_calls' ||
          output.stopReason === 'length' || output.stopReason === 'error'
          ? output.stopReason
          : ''
        const errorCode = output.error && typeof output.error === 'object' && !Array.isArray(output.error)
          ? 'provider_request_failed'
          : ''
        const safeFingerprint = (entry: unknown): { sha256: string; bytes: number } | undefined => {
          if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return undefined
          const record = entry as Record<string, unknown>
          return typeof record.sha256 === 'string' && /^[a-f0-9]{64}$/i.test(record.sha256)
            && typeof record.bytes === 'number' && Number.isFinite(record.bytes) && record.bytes >= 0
            ? { sha256: record.sha256.toLowerCase(), bytes: Math.floor(record.bytes) }
            : undefined
        }
        const request = safeFingerprint(round.request) ?? debugFingerprint(round.requestBody)
        const response = safeFingerprint(round.response) ?? debugFingerprint(round.output)
        const toolCallCount = typeof round.toolCallCount === 'number' && Number.isFinite(round.toolCallCount)
          ? Math.max(0, Math.floor(round.toolCallCount))
          : Array.isArray(output.toolCalls) ? output.toolCalls.length : 0
        const usage = numericUsage(round.usage) ?? numericUsage(output.usage)
        return {
          id: typeof round.id === 'number' ? round.id : 0,
          durationMs: typeof round.durationMs === 'number' && Number.isFinite(round.durationMs)
            ? Math.max(0, round.durationMs)
            : 0,
          status: round.status === 'failed' || output.error ? 'failed' : 'completed',
          ...(request ? { request } : {}),
          ...(response ? { response } : {}),
          ...(toolCallCount > 0 ? { toolCallCount } : {}),
          ...(usage ? { usage } : {}),
          ...(stopReason ? { stopReason } : {}),
          ...(errorCode ? { errorCode } : {})
        }
      })
  }
}

export function sanitizeRuntimeResponseBody(
  body: string,
  pathAndQuery = '',
  pin: FinalPublicationAuthorityPinV1 | null = captureCurrentFinalPublicationAuthorityPin()
): string {
  try {
    const parsed = JSON.parse(body) as unknown
    if (containsPrivateAcceptedFinalAuthority(parsed)) return JSON.stringify(null)
    if (pathAndQuery.split('?', 1)[0] === '/v1/debug/llm-rounds') {
      return JSON.stringify(sanitizeLlmDebugResponse(parsed))
    }
    const sanitized = sanitizePublicRuntimeValue(projectVerifiedAcceptedFinalHTTPValue(parsed, pin))
    return JSON.stringify(sanitized ?? null)
  } catch {
    return sanitizePublicAssistantText(body)
  }
}

function projectVerifiedAcceptedFinalHTTPValue(
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null,
  inheritedCaseBound = false
): unknown {
  if (Array.isArray(value)) {
    return value
      .map((entry) => projectVerifiedAcceptedFinalHTTPValue(
        entry,
        pin,
        inheritedCaseBound
      ))
      .filter((entry) => entry !== undefined)
  }
  if (!value || typeof value !== 'object') return value
  const record = value as Record<string, unknown>
  const caseBound = inheritedCaseBound || record.historyAuthority === 'case_boundary_only_v1'
  const claimsAcceptedFinal = Object.prototype.hasOwnProperty.call(record, 'acceptedFinal') ||
    Object.prototype.hasOwnProperty.call(record, 'acceptedFinalView')
  const claimsAcceptedFinalDelivery = Object.prototype.hasOwnProperty.call(record, 'acceptedFinalDelivery') ||
    Object.prototype.hasOwnProperty.call(record, 'acceptedFinalDeliveries')
  const acceptedFinalDeliverySet = claimsAcceptedFinalDelivery
    ? verifiedThreadAcceptedFinalDeliveriesV1(record, pin)
    : null
  if (claimsAcceptedFinalDelivery && !acceptedFinalDeliverySet) return undefined
  const directAcceptedFinalBatch = record.kind === 'accepted_final_batch'
    ? verifiedAcceptedFinalDeliveryBatch(record, pin)
    : null
  if (record.kind === 'accepted_final_batch' && !directAcceptedFinalBatch) return undefined
  if (directAcceptedFinalBatch) return directAcceptedFinalBatch.batch

  if (acceptedFinalDeliverySet) {
    return projectVerifiedAcceptedFinalThreadV1(record, pin, caseBound, acceptedFinalDeliverySet)
  }

  if (record.kind === 'assistant_text') {
    if (!claimsAcceptedFinal) {
      if (!caseBound) return value
      const parsed = AssistantTextTurnItem.safeParse(record)
      const ordinaryResult = parsed.success ? parsed.data.ordinaryResult : undefined
      if (!parsed.success || !ordinaryResult ||
          containsPrivateReasoningContent(parsed.data.text) ||
          !validOrdinaryResultSlotV1(ordinaryResult)) {
        return undefined
      }
      return parsed.data
    }
    const parsed = AcceptedFinalPublicAssistantTextTurnItemV3.safeParse(record)
    if (
      !parsed.success ||
      containsPrivateReasoningContent(parsed.data.text) ||
      parsed.data.acceptedFinal !== undefined ||
      parsed.data.acceptedFinalView.schemaVersion !== 3
    ) {
      return undefined
    }
    // A shape-valid V3 item is not authority. It is admitted only by the exact
    // thread/turn/item position bound by projectVerifiedAcceptedFinalThreadV1.
    return undefined
  }

  const genericView = AcceptedFinalPublicViewV3Schema.safeParse(record.acceptedFinalView)
  const acceptedProjectionValid = claimsAcceptedFinal &&
    record.acceptedFinal === undefined &&
    genericView.success &&
    (typeof record.finishedAt !== 'string' || record.finishedAt === genericView.data.acceptedAt)

  const projected: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(record)) {
    if (key === 'acceptedFinalDelivery') {
      continue
    }
    if (key === 'acceptedFinalDeliveries') {
      continue
    }
    if ((key === 'acceptedFinal' || key === 'acceptedFinalView') && claimsAcceptedFinal) {
      continue
    }
    const child = projectVerifiedAcceptedFinalHTTPValue(entry, pin, caseBound)
    if (child !== undefined) projected[key] = child
  }
  if (acceptedProjectionValid) return undefined
  return projected
}

function projectVerifiedAcceptedFinalThreadV1(
  thread: Record<string, unknown>,
  pin: FinalPublicationAuthorityPinV1 | null,
  caseBound: boolean,
  deliveries: VerifiedThreadAcceptedFinalDeliveriesV1
): Record<string, unknown> | undefined {
  if (!Array.isArray(thread.turns)) return undefined
  const projectedTurns: unknown[] = []
  for (const value of thread.turns) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined
    const turn = value as Record<string, unknown>
    const turnID = typeof turn.id === 'string' ? turn.id : ''
    const attestation = deliveries.byTurn.get(turnID)
    const claimsAcceptedFinal = Object.prototype.hasOwnProperty.call(turn, 'acceptedFinal') ||
      Object.prototype.hasOwnProperty.call(turn, 'acceptedFinalView') ||
      (Array.isArray(turn.items) && turn.items.some((item) => (
        Boolean(item) && typeof item === 'object' && !Array.isArray(item) && (
          Object.prototype.hasOwnProperty.call(item, 'acceptedFinal') ||
          Object.prototype.hasOwnProperty.call(item, 'acceptedFinalView')
        )
      )))
    if (claimsAcceptedFinal !== Boolean(attestation)) return undefined
    if (!attestation) {
      const projected = projectVerifiedAcceptedFinalHTTPValue(turn, pin, caseBound)
      if (projected === undefined) return undefined
      projectedTurns.push(projected)
      continue
    }
    if (!acceptedFinalRecordHasExactDeliveryAttestationV1(turn, deliveries.byTurn) ||
        !Array.isArray(turn.items)) return undefined

    const projectedTurn: Record<string, unknown> = {}
    for (const [key, entry] of Object.entries(turn)) {
      if (key === 'acceptedFinal') return undefined
      if (key === 'acceptedFinalView') {
        projectedTurn[key] = entry
        continue
      }
      if (key === 'items') {
        const projectedItems: unknown[] = []
        for (const itemValue of turn.items) {
          if (!itemValue || typeof itemValue !== 'object' || Array.isArray(itemValue)) return undefined
          const item = itemValue as Record<string, unknown>
          const itemClaimsAcceptedFinal = Object.prototype.hasOwnProperty.call(item, 'acceptedFinal') ||
            Object.prototype.hasOwnProperty.call(item, 'acceptedFinalView')
          if (itemClaimsAcceptedFinal) {
            const parsed = AcceptedFinalPublicAssistantTextTurnItemV3.safeParse(item)
            if (!parsed.success || !acceptedFinalRecordHasExactDeliveryAttestationV1(item, deliveries.byTurn) ||
                containsPrivateReasoningContent(parsed.data.text) || parsed.data.acceptedFinal !== undefined) {
              return undefined
            }
            projectedItems.push(parsed.data)
            continue
          }
          const projected = projectVerifiedAcceptedFinalHTTPValue(item, pin, caseBound)
          if (projected === undefined) return undefined
          projectedItems.push(projected)
        }
        projectedTurn[key] = projectedItems
        continue
      }
      const projected = projectVerifiedAcceptedFinalHTTPValue(entry, pin, caseBound)
      if (projected !== undefined) projectedTurn[key] = projected
    }
    projectedTurns.push(projectedTurn)
  }

  const projectedThread: Record<string, unknown> = {}
  for (const [key, entry] of Object.entries(thread)) {
    if (key === 'turns') {
      projectedThread[key] = projectedTurns
      continue
    }
    if (key === 'acceptedFinalDelivery') {
      if (deliveries.latest) projectedThread[key] = deliveries.latest
      continue
    }
    if (key === 'acceptedFinalDeliveries') {
      projectedThread[key] = deliveries.deliveries
      continue
    }
    const projected = projectVerifiedAcceptedFinalHTTPValue(entry, pin, caseBound)
    if (projected !== undefined) projectedThread[key] = projected
  }
  return projectedThread
}

type AcceptedFinalDeliveryAttestationV1 = {
  threadId: string
  turnId: string
  publicationCommitId: string
  assistantItemCanonical: string
}

type VerifiedThreadAcceptedFinalDeliveriesV1 = {
  latest?: Record<string, unknown>
  deliveries: Record<string, unknown>[]
  byTurn: Map<string, AcceptedFinalDeliveryAttestationV1>
}

function acceptedFinalAttestationsForVerifiedBatchesV1(
  verifiedBatches: Array<NonNullable<ReturnType<typeof verifiedAcceptedFinalDeliveryBatch>>>
): Map<string, AcceptedFinalDeliveryAttestationV1> {
  const byTurn = new Map<string, AcceptedFinalDeliveryAttestationV1>()
  for (const verified of verifiedBatches) {
    const assistantItem = verified.events[0]?.item
    const threadId = verified.batch.threadId
    const turnId = verified.batch.turnId
    if (typeof threadId !== 'string' || typeof turnId !== 'string' ||
        !assistantItem || typeof assistantItem !== 'object' || Array.isArray(assistantItem) ||
        byTurn.has(turnId)) {
      return new Map()
    }
    byTurn.set(turnId, {
      threadId,
      turnId,
      publicationCommitId: verified.publicationCommitId,
      assistantItemCanonical: canonicalRuntimeBoundaryValue(assistantItem)
    })
  }
  return byTurn
}

function acceptedFinalRecordHasExactDeliveryAttestationV1(
  record: Record<string, unknown>,
  attestations: ReadonlyMap<string, AcceptedFinalDeliveryAttestationV1>
): boolean {
  const view = AcceptedFinalPublicViewV3Schema.safeParse(record.acceptedFinalView)
  if (!view.success || typeof record.threadId !== 'string') return false
  const turnId = record.kind === 'assistant_text' ? record.turnId : record.id
  if (typeof turnId !== 'string') return false
  const attestation = attestations.get(turnId)
  if (!attestation || attestation.threadId !== record.threadId ||
      attestation.publicationCommitId !== view.data.acceptedFinalDigest) return false
  return record.kind !== 'assistant_text' ||
    canonicalRuntimeBoundaryValue(record) === attestation.assistantItemCanonical
}

function verifiedThreadAcceptedFinalDeliveriesV1(
  thread: Record<string, unknown>,
  pin: FinalPublicationAuthorityPinV1 | null
): VerifiedThreadAcceptedFinalDeliveriesV1 | null {
  if (!Array.isArray(thread.acceptedFinalDeliveries) || thread.acceptedFinalDeliveries.length === 0 ||
      thread.acceptedFinalDeliveries.length > ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1 ||
      !Array.isArray(thread.turns) || typeof thread.id !== 'string' ||
      !Number.isSafeInteger(thread.latestSeq)) {
    return null
  }

  const verifiedBatches = thread.acceptedFinalDeliveries.map((delivery) =>
    verifiedAcceptedFinalDeliveryBatch(delivery, pin)
  )
  if (verifiedBatches.some((entry) => entry === null)) return null
  const verified = verifiedBatches as Array<NonNullable<(typeof verifiedBatches)[number]>>
  const claimsLegacyLatest = Object.prototype.hasOwnProperty.call(thread, 'acceptedFinalDelivery')
  const verifiedLatest = claimsLegacyLatest
    ? verifiedAcceptedFinalDeliveryBatch(thread.acceptedFinalDelivery, pin)
    : null
  if (verified.length > 1 && claimsLegacyLatest) return null
  if (verified.length === 1 && claimsLegacyLatest && (!verifiedLatest ||
      canonicalRuntimeBoundaryValue(verifiedLatest.batch) !==
        canonicalRuntimeBoundaryValue(verified[0].batch))) return null

  const acceptedTurns = thread.turns.filter((turn): turn is Record<string, unknown> => (
    Boolean(turn) &&
    typeof turn === 'object' &&
    !Array.isArray(turn) &&
    AcceptedFinalPublicViewV3Schema.safeParse((turn as Record<string, unknown>).acceptedFinalView).success
  ))
  if (acceptedTurns.length !== verified.length) return null
  let previousLastSeq = 0
  const seenCommits = new Set<string>()
  for (let index = 0; index < verified.length; index += 1) {
    const current = verified[index]
    const acceptedTurn = acceptedTurns[index]
    const acceptedView = AcceptedFinalPublicViewV3Schema.safeParse(acceptedTurn.acceptedFinalView)
    const acceptedTurnItems = acceptedTurn.items
    if (!acceptedView.success || current.batch.threadId !== thread.id ||
        current.batch.turnId !== acceptedTurn.id ||
        current.publicationCommitId !== acceptedView.data.acceptedFinalDigest ||
        current.firstSeq <= previousLastSeq || current.lastSeq > (thread.latestSeq as number) ||
        seenCommits.has(current.publicationCommitId) || !Array.isArray(acceptedTurnItems)) {
      return null
    }
    previousLastSeq = current.lastSeq
    seenCommits.add(current.publicationCommitId)

    const matchingThreadItems = (itemId: unknown): Record<string, unknown>[] =>
      acceptedTurnItems.filter((item): item is Record<string, unknown> =>
        Boolean(item) && typeof item === 'object' && !Array.isArray(item) &&
        (item as Record<string, unknown>).id === itemId
      )
    const assistantEvent = current.events[0]
    const assistantItem = assistantEvent?.item
    if (!assistantItem || typeof assistantItem !== 'object' || Array.isArray(assistantItem)) return null
    const matchingAssistantItems = matchingThreadItems(assistantEvent.itemId)
    if (matchingAssistantItems.length !== 1 ||
        canonicalRuntimeBoundaryValue(matchingAssistantItems[0]) !==
          canonicalRuntimeBoundaryValue(assistantItem)) {
      return null
    }

    if (current.events.length === 4) {
      const terminalErrorEvent = current.events[1]
      const terminalErrorItem = terminalErrorEvent?.item
      if (!terminalErrorItem || typeof terminalErrorItem !== 'object' ||
          Array.isArray(terminalErrorItem)) return null
      const matchingTerminalErrorItems = matchingThreadItems(terminalErrorEvent.itemId)
      if (matchingTerminalErrorItems.length !== 1 ||
          canonicalRuntimeBoundaryValue(matchingTerminalErrorItems[0]) !==
            canonicalRuntimeBoundaryValue(terminalErrorItem)) {
        return null
      }
    }
  }
  const lastAcceptedTurn = acceptedTurns[acceptedTurns.length - 1]
  const lastThreadTurn = thread.turns[thread.turns.length - 1]
  if (lastAcceptedTurn && lastThreadTurn && typeof lastThreadTurn === 'object' &&
      !Array.isArray(lastThreadTurn) && lastThreadTurn.id === lastAcceptedTurn.id &&
      verified[verified.length - 1]?.lastSeq !== thread.latestSeq) {
    return null
  }
  const byTurn = acceptedFinalAttestationsForVerifiedBatchesV1(verified)
  if (byTurn.size !== verified.length) return null
  return {
    ...(verifiedLatest ? { latest: verifiedLatest.batch } : {}),
    deliveries: verified.map((entry) => entry.batch),
    byTurn
  }
}

function runtimeResponseRequiresBody(path: string): boolean {
  return path === '/v1/runtime/info' ||
    path === '/v1/runtime/tools' ||
    path === ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH ||
    path === '/v1/skills' ||
    path === '/v1/attachments/diagnostics' ||
    path === '/v1/memory/diagnostics'
}

const PublicThreadHTTPResponseV1Schema = ThreadSchema.extend({
  historyAuthority: z.literal('case_boundary_only_v1').optional()
}).strict()

const RuntimeHealthResponseV1Schema = z.object({
  status: z.literal('ok'),
  service: z.literal('analytix'),
  mode: z.literal('serve')
}).strict()

const MemoryTombstoneResponseV1Schema = z.object({
  id: z.string().min(1),
  createdAt: z.string().min(1),
  updatedAt: z.string().min(1),
  deletedAt: z.string().min(1)
}).strict()

const MemoryPublicRecordResponseV1Schema = z.union([
  MemoryRecordSchema,
  MemoryTombstoneResponseV1Schema
])

const MemoryListResponseV1Schema = z.object({
  memories: z.array(MemoryPublicRecordResponseV1Schema)
}).strict()

const MemoryMutationResponseV1Schema = z.object({
  memory: MemoryPublicRecordResponseV1Schema
}).strict()

type RuntimeResponseSchemaV1 = z.ZodTypeAny

function runtimeResponseSchemasV1(path: string, method: string): RuntimeResponseSchemaV1[] | null {
  if (path === '/health') return [RuntimeHealthResponseV1Schema]
  if (path === '/v1/runtime/info') return [RuntimeInfoResponseSchema]
  if (path === '/v1/runtime/tools') return [RuntimeToolsResponseSchema]
  if (path === ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_PATH && method === 'POST') {
    return [SuccessfulToolExecutionObservationResponseV1Schema]
  }
  if (path === '/v1/skills') return [RuntimeSkillsResponseSchema]
  if (path === '/v1/attachments/diagnostics') return [AttachmentDiagnosticsResponseSchema]
  if (path === '/v1/memory/diagnostics') return [MemoryDiagnosticsResponseSchema]
  if (path === '/v1/memory') {
    if (method === 'GET') return [MemoryListResponseV1Schema]
    if (method === 'POST') return [MemoryMutationResponseV1Schema]
    return [MemoryListResponseV1Schema, MemoryMutationResponseV1Schema]
  }
  if (/^\/v1\/memory\/[^/]+$/.test(path)) return [MemoryMutationResponseV1Schema]
  if (path === '/v1/runtime/task-jobs/output') return [TaskJobOutputResponseV1Schema]
  if (path === '/v1/runtime/task-jobs/list') return [TaskJobListResponseV1Schema]
  if (path === '/v1/runtime/task-jobs/wait') return [TaskJobWaitResponseV1Schema]
  if (path === '/v1/runtime/task-jobs/kill') return [TaskJobKillResponseV1Schema]
  if (path === '/v1/attachments') return [AttachmentUploadResponse]
  if (/^\/v1\/attachments\/[^/]+\/content$/.test(path)) return [AttachmentContentResponse]
  if (/^\/v1\/attachments\/[^/]+$/.test(path)) return [AttachmentMetadataResponse]
  if (path === '/v1/case-projects') return [CaseProjectListResponseV1Schema]
  if (/^\/v1\/case-projects\/[^/]+\/threads$/.test(path)) return [CaseProjectThreadsResponseV1Schema]
  if (/^\/v1\/case-projects\/[^/]+\/detail$/.test(path)) return [CaseProjectDetailResponseV1Schema]
  if (/^\/v1\/threads\/[^/]+\/summary$/.test(path)) return [ThreadSummaryResponseSchema]
  if (/^\/v1\/threads\/[^/]+\/todos$/.test(path)) {
    if (method === 'DELETE') return [ClearThreadTodosResponse]
    return [ThreadTodosResponse]
  }
  if (/^\/v1\/threads\/[^/]+\/compact$/.test(path)) return [CompactResponse]
  if (/^\/v1\/threads\/[^/]+\/summary\/tasks\/[^/]+\/(kill|restart)$/.test(path)) {
    return [ThreadSummaryTaskMutationResponseSchema]
  }
  if (/^\/v1\/threads\/[^/]+\/summary\/tasks\/[^/]+\/output$/.test(path)) {
    return [ThreadSummaryTaskOutputResponseV1Schema]
  }
  if (path === '/v1/threads') {
    if (method === 'GET') return [ListThreadsResponse]
    if (method === 'POST') return [PublicThreadHTTPResponseV1Schema]
    return [ListThreadsResponse, PublicThreadHTTPResponseV1Schema]
  }
  if (/^\/v1\/threads\/[^/]+$/.test(path)) {
    if (method === 'DELETE') return [DeleteThreadResponse]
    if (method === 'GET') return [ThreadDetailResponseV1Schema]
    return [PublicThreadHTTPResponseV1Schema]
  }
  if (/^\/v1\/threads\/[^/]+\/fork$/.test(path)) return [PublicThreadHTTPResponseV1Schema]
  if (/^\/v1\/threads\/[^/]+\/turns$/.test(path)) return [StartTurnResponse]
  if (/^\/v1\/threads\/[^/]+\/turns\/[^/]+\/steer$/.test(path)) return [SteerTurnResponse]
  if (/^\/v1\/threads\/[^/]+\/turns\/[^/]+\/interrupt$/.test(path)) return [InterruptTurnResponse]
  if (/^\/v1\/threads\/[^/]+\/rewind$/.test(path)) return [RewindThreadResponse]
  if (/^\/v1\/threads\/[^/]+\/review$/.test(path)) return [StartReviewResponse]
  if (/^\/v1\/approvals\/[^/]+$/.test(path)) return [ApprovalDecisionResponse]
  if (path === '/v1/usage') {
    return [DailyUsageResponseSchema, ThreadUsageResponseSchema, RuntimeUsageResponseSchema, ModelUsageResponseSchema]
  }
  if (path === ANALYTIX_PROVIDER_REGISTRY_PATH) {
    if (method === 'GET') return [providerRegistrySnapshotResponseSchemaV1]
    if (method === 'POST') return [providerRegistryProviderResponseSchemaV1]
    return null
  }
  if (path === ANALYTIX_PROVIDER_REGISTRY_PORTABLE_MANIFEST_PATH) {
    if (method === 'GET') return [providerRegistryPortableManifestExportResponseSchemaV1]
    if (method === 'POST') return [providerRegistryPortableManifestImportResponseSchemaV1]
    return null
  }
  if (path === ANALYTIX_PROVIDER_REGISTRY_RECOVER_PATH && method === 'POST') {
    return [providerRegistryRecoveredResponseSchemaV1]
  }
  const providerRegistryProviderRoute = /^\/v1\/provider-registry\/providers\/[^/]+(?:\/(select|disconnect|credential|probe|discover-models|account-observation))?$/.exec(path)
  if (providerRegistryProviderRoute) {
    switch (providerRegistryProviderRoute[1]) {
      case undefined:
        if (method === 'DELETE') return [providerRegistryDeletedResponseSchemaV1]
        if (method === 'GET' || method === 'PATCH') return [providerRegistryProviderResponseSchemaV1]
        return null
      case 'probe':
        return method === 'POST' ? [providerRegistryProbeResponseSchemaV1] : null
      case 'account-observation':
        return method === 'POST' ? [providerRegistryAccountObservationResponseSchemaV1] : null
      case 'select':
      case 'disconnect':
      case 'discover-models':
        return method === 'POST' ? [providerRegistryProviderResponseSchemaV1] : null
      case 'credential':
        return method === 'PUT' ? [providerRegistryProviderResponseSchemaV1] : null
    }
  }
  return null
}

function canonicalRuntimeBoundaryValue(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(canonicalRuntimeBoundaryValue).join(',')}]`
  if (value && typeof value === 'object') {
    const record = value as Record<string, unknown>
    return `{${Object.keys(record).sort().map((key) => (
      `${JSON.stringify(key)}:${canonicalRuntimeBoundaryValue(record[key])}`
    )).join(',')}}`
  }
  return JSON.stringify(value)
}

function exactRuntimeResponseValueV1(
  schemas: RuntimeResponseSchemaV1[],
  value: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): unknown | null {
  const inputCanonical = canonicalRuntimeBoundaryValue(value)
  for (const schema of schemas) {
    const validated = schema.safeParse(value)
    if (!validated.success || canonicalRuntimeBoundaryValue(validated.data) !== inputCanonical) continue
    const sanitized = restoreVerifiedAcceptedFinalPublicDeliveryGroupsV2(
      validated.data,
      sanitizePublicRuntimeValue(validated.data),
      pin
    )
    if (sanitized === undefined || canonicalRuntimeBoundaryValue(sanitized) !== inputCanonical) continue
    if (containsPrivateReasoningContent(sanitized) || containsPrivateRuntimeDiagnosticContent(sanitized)) continue
    return validated.data
  }
  return null
}

function restoreVerifiedAcceptedFinalPublicDeliveryGroupsV2(
  value: unknown,
  sanitized: unknown,
  pin: FinalPublicationAuthorityPinV1 | null
): unknown {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      !sanitized || typeof sanitized !== 'object' || Array.isArray(sanitized)) {
    return sanitized
  }
  const sourceThread = value as Record<string, unknown>
  const publicThread = sanitized as Record<string, unknown>
  const verifiedDeliveries = verifiedThreadAcceptedFinalDeliveriesV1(sourceThread, pin)
  if (!verifiedDeliveries) return sanitized
  const sourceTurns = sourceThread.turns
  const publicTurns = publicThread.turns
  if (!Array.isArray(sourceTurns) || !Array.isArray(publicTurns) ||
      sourceTurns.length !== publicTurns.length) return sanitized
  const publicDeliveries = publicThread.acceptedFinalDeliveries
  if (!Array.isArray(publicDeliveries) ||
      publicDeliveries.length !== verifiedDeliveries.deliveries.length) return sanitized

  for (let deliveryIndex = 0; deliveryIndex < verifiedDeliveries.deliveries.length; deliveryIndex += 1) {
    const sourceBatch = verifiedDeliveries.deliveries[deliveryIndex]
    const sourceEvents = sourceBatch.events
    if (!Array.isArray(sourceEvents) || (sourceEvents.length !== 3 && sourceEvents.length !== 4)) {
      return sanitized
    }
    const assistantEvent = sourceEvents[0]
    if (!assistantEvent || typeof assistantEvent !== 'object' || Array.isArray(assistantEvent)) {
      return sanitized
    }
    const assistantEventRecord = assistantEvent as Record<string, unknown>
    const assistantItem = assistantEventRecord.item
    if (!assistantItem || typeof assistantItem !== 'object' || Array.isArray(assistantItem)) {
      return sanitized
    }
    const exactAuthorityItemIds = new Set<unknown>([assistantEventRecord.itemId])
    if (sourceEvents.length === 4) {
      const terminalErrorEvent = sourceEvents[1]
      if (!terminalErrorEvent || typeof terminalErrorEvent !== 'object' ||
          Array.isArray(terminalErrorEvent)) return sanitized
      const terminalErrorEventRecord = terminalErrorEvent as Record<string, unknown>
      const terminalErrorItem = terminalErrorEventRecord.item
      if (!terminalErrorItem || typeof terminalErrorItem !== 'object' ||
          Array.isArray(terminalErrorItem)) return sanitized
      exactAuthorityItemIds.add(terminalErrorEventRecord.itemId)
    }

    const sourceTurnIndex = sourceTurns.findIndex((turn) =>
      Boolean(turn) && typeof turn === 'object' && !Array.isArray(turn) &&
      (turn as Record<string, unknown>).id === sourceBatch.turnId
    )
    if (sourceTurnIndex < 0) return sanitized
    const sourceTurn = sourceTurns[sourceTurnIndex] as Record<string, unknown>
    const publicTurn = publicTurns[sourceTurnIndex]
    if (!publicTurn || typeof publicTurn !== 'object' || Array.isArray(publicTurn) ||
        !Array.isArray(sourceTurn.items) ||
        !Array.isArray((publicTurn as Record<string, unknown>).items)) return sanitized
    const sourceItems = sourceTurn.items as unknown[]
    for (const sourceItem of sourceItems) {
      if (!sourceItem || typeof sourceItem !== 'object' || Array.isArray(sourceItem)) return sanitized
      const sourceItemRecord = sourceItem as Record<string, unknown>
      if (exactAuthorityItemIds.has(sourceItemRecord.id)) continue
      const ordinaryProjection = sanitizePublicRuntimeValue(sourceItem)
      if (canonicalRuntimeBoundaryValue(ordinaryProjection) !==
          canonicalRuntimeBoundaryValue(sourceItem)) return sanitized
    }
    const sourceView = AcceptedFinalPublicViewV3Schema.safeParse(sourceTurn.acceptedFinalView)
    const assistantItemRecord = assistantItem as Record<string, unknown>
    if (!sourceView.success || sourceTurn.acceptedFinal !== undefined ||
        canonicalRuntimeBoundaryValue(assistantItemRecord.acceptedFinalView) !==
          canonicalRuntimeBoundaryValue(sourceView.data)) return sanitized
    ;(publicTurn as Record<string, unknown>).items = sourceItems
    ;(publicTurn as Record<string, unknown>).acceptedFinalView = sourceView.data
  }
  publicThread.acceptedFinalDeliveries = verifiedDeliveries.deliveries
  if (verifiedDeliveries.latest) publicThread.acceptedFinalDelivery = verifiedDeliveries.latest
  return sanitized
}

type ThreadSummaryRouteIdentityV1 = {
  threadId: string
  taskId?: string
  action?: 'output' | 'kill' | 'restart'
}

function decodeClosedRuntimeRouteSegmentV1(value: string): string | null {
  try {
    const decoded = decodeURIComponent(value)
    return decoded && !decoded.includes('/') && !decoded.includes('\\') ? decoded : null
  } catch {
    return null
  }
}

function threadSummaryRouteIdentityV1(path: string): ThreadSummaryRouteIdentityV1 | null {
  const match = /^\/v1\/threads\/([^/]+)\/summary(?:\/tasks\/([^/]+)\/(output|kill|restart))?$/.exec(path)
  if (!match) return null
  const threadId = decodeClosedRuntimeRouteSegmentV1(match[1])
  if (!threadId) return null
  if (!match[2]) return { threadId }
  const taskId = decodeClosedRuntimeRouteSegmentV1(match[2])
  if (!taskId) return null
  return { threadId, taskId, action: match[3] as ThreadSummaryRouteIdentityV1['action'] }
}

function threadSummaryResponseIdentityMatchesV1(path: string, value: unknown): boolean {
  const route = threadSummaryRouteIdentityV1(path)
  if (!route) return !/^\/v1\/threads\/[^/]+\/summary(?:\/|$)/.test(path)
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  if (!route.taskId) return record.threadId === route.threadId
  if (route.action === 'output') {
    return threadSummaryTaskIdentityMatchesV1(record.taskId, route.taskId)
  }
  const task = record.task
  return Boolean(task && typeof task === 'object' && !Array.isArray(task) &&
    typeof (task as Record<string, unknown>).id === 'string' &&
    threadSummaryTaskIdentityMatchesV1((task as Record<string, unknown>).id, route.taskId))
}

export function sanitizeRuntimeResponse(
  response: { ok: boolean; status: number; body: string },
  pathAndQuery = '',
  pin: FinalPublicationAuthorityPinV1 | null = captureCurrentFinalPublicationAuthorityPin(),
  requestMethod = ''
): { ok: boolean; status: number; body: string } {
  const path = pathAndQuery.split('?', 1)[0]
  const method = requestMethod.trim().toUpperCase()
  if (!response.body.trim()) {
    if (response.ok && !runtimeResponseRequiresBody(path)) return response
    return response.ok
      ? {
          ok: false,
          status: 502,
          body: JSON.stringify({
            code: 'runtime_response_schema_invalid',
            message: 'Runtime response failed schema validation.'
          })
        }
      : { ...response, body: JSON.stringify(projectPublicRuntimeHTTPError(response.status, null)) }
  }
  let canonicalValue: unknown
  let schemaValidated = false
  try {
    const parsed = JSON.parse(response.body) as unknown
    if (!response.ok) {
      if (path === '/v1/provider-registry' || path.startsWith('/v1/provider-registry/')) {
        // The downstream typed IPC validates status/operation as well. Keep
        // only this strict, key-free envelope with canonical error messages.
        const failure = providerRegistryFailureSchemaV1.safeParse(parsed)
        if (failure.success) return { ...response, body: JSON.stringify(failure.data) }
      }
      return { ...response, body: JSON.stringify(projectPublicRuntimeHTTPError(response.status, parsed)) }
    }
    if (containsPrivateAcceptedFinalAuthority(parsed) || containsPrivateRuntimeDiagnosticContent(parsed)) {
      return {
        ok: false,
        status: 502,
        body: JSON.stringify({
          code: 'runtime_response_not_public',
          message: 'Runtime response was blocked at the public boundary.'
        })
      }
    }
    if (path === '/v1/debug/llm-rounds') {
      canonicalValue = sanitizeLlmDebugResponse(parsed)
      schemaValidated = true
    } else {
      const outputSchemas = runtimeResponseSchemasV1(path, method)
      if (!outputSchemas) {
        return {
          ok: false,
          status: 502,
          body: JSON.stringify({
            code: 'runtime_response_schema_invalid',
            message: 'Runtime response failed schema validation.'
          })
        }
      }
      const authorityProjected = projectVerifiedAcceptedFinalHTTPValue(parsed, pin)
      if (canonicalRuntimeBoundaryValue(authorityProjected) !== canonicalRuntimeBoundaryValue(parsed)) {
        return {
          ok: false,
          status: 502,
          body: JSON.stringify({
            code: 'runtime_response_not_public',
            message: 'Runtime response was blocked at the public boundary.'
          })
        }
      }
      const validated = exactRuntimeResponseValueV1(outputSchemas, authorityProjected, pin)
      if (validated === null) {
        return {
          ok: false,
          status: 502,
          body: JSON.stringify({
            code: 'runtime_response_schema_invalid',
            message: 'Runtime response failed schema validation.'
          })
        }
      }
      const requiresExactPublicProjection = path.startsWith('/v1/runtime/task-jobs/') ||
        /^\/v1\/threads\/[^/]+\/summary(?:\/|$)/.test(path)
      if (requiresExactPublicProjection) {
        const sanitized = sanitizePublicRuntimeValue(validated)
        const sanitizedMatches = sanitized !== undefined &&
          canonicalRuntimeBoundaryValue(sanitized) === canonicalRuntimeBoundaryValue(validated)
        if (!sanitizedMatches || !threadSummaryResponseIdentityMatchesV1(path, validated)) {
          return {
            ok: false,
            status: 502,
            body: JSON.stringify({
              code: 'runtime_response_not_public',
              message: 'Runtime response was blocked at the public boundary.'
            })
          }
        }
        canonicalValue = validated
      } else {
        canonicalValue = validated
      }
      schemaValidated = true
    }
  } catch {
    return {
      ok: false,
      status: 502,
      body: JSON.stringify({
        code: 'runtime_response_not_public',
        message: 'Runtime response was blocked at the public boundary.'
      })
    }
  }
  if (schemaValidated) {
    if (containsPrivateReasoningContent(canonicalValue) || containsPrivateRuntimeDiagnosticContent(canonicalValue)) {
      return {
        ok: false,
        status: 502,
        body: JSON.stringify({
          code: 'runtime_response_not_public',
          message: 'Runtime response was blocked at the public boundary.'
        })
      }
    }
    return { ...response, body: JSON.stringify(canonicalValue) }
  }
  return {
    ok: false,
    status: 502,
    body: JSON.stringify({
      code: 'runtime_response_schema_invalid',
      message: 'Runtime response failed schema validation.'
    })
  }
}

export async function runtimeRequestViaHost(
  settings: AppSettingsV1,
  pathAndQuery: string,
  init: RuntimeRequestInit,
  ensureRuntime: (settings: AppSettingsV1) => Promise<AppSettingsV1 | void>
): Promise<{ ok: boolean; status: number; body: string }> {
  const ensuredSettings = await ensureRuntime(settings)
  const requestSettings = ensuredSettings ?? settings
  const base = getRuntimeBaseUrlForSettings(requestSettings)
  const pathNorm = pathAndQuery.startsWith('/') ? pathAndQuery : `/${pathAndQuery}`
  const url = `${base}${pathNorm}`
  const authorityPin = captureCurrentFinalPublicationAuthorityPin()
  const requestAuthorityPin = authorityPin && new URL(base).origin === authorityPin.runtimeUrl
    ? authorityPin
    : null
  const hdrs = runtimeAuthHeaders(requestSettings)
  for (const [key, value] of Object.entries(init.headers ?? {})) {
    hdrs.set(key, value)
  }
  hdrs.set('Accept', 'application/json')
  if (init.body && !hdrs.has('Content-Type')) {
    hdrs.set('Content-Type', 'application/json')
  }
  try {
    const res = await fetch(url, {
      method: init.method ?? 'GET',
      headers: hdrs,
      body: init.body,
      signal: AbortSignal.timeout(init.method === 'POST' ? 60_000 : 15_000)
    })
    return sanitizeRuntimeResponse({
      ok: res.ok,
      status: res.status,
      body: await res.text()
    }, pathNorm, isCurrentFinalPublicationAuthorityPin(requestAuthorityPin) ? requestAuthorityPin : null, init.method ?? 'GET')
  } catch {
    throw new Error(JSON.stringify({
      code: 'runtime_unavailable',
      message: 'The Analytix runtime is unavailable.'
    }))
  }
}

export { buildAnalytixServeArgs, resolveAnalytixExecutable }

/**
 * Default data directory used when the user has not provided one.
 * The path lives under the app user-data directory so packaged
 * installs do not need write access to the install folder.
 */
export function defaultAnalytixDataDir(): string {
  return DEFAULT_ANALYTIX_DATA_DIR.replace(/^~(?=$|[\\/])/, homedir())
}

export function resolveGoRuntimeMCPConfigPath(dataDir: string): string {
  return join(dataDir, 'config.json')
}

export function buildGoRuntimeProviderArgs(
  _settings: AppSettingsV1,
  runtime: ReturnType<typeof resolveAnalytixRuntimeSettings>
): string[] {
  return [
    '--approval-policy',
    runtime.approvalPolicy,
    '--sandbox-mode',
    runtime.sandboxMode
  ]
}

export function buildGoRuntimeSidecarEnv(
  settings: AppSettingsV1,
  runtime: ReturnType<typeof resolveAnalytixRuntimeSettings>,
  dataDir: string,
  env: NodeJS.ProcessEnv = process.env,
  runtimeToken: string = runtime.runtimeToken
): NodeJS.ProcessEnv {
  const childEnvironment = { ...env }
  const reservedNames = new Set<string>([
    'ANALYTIX_API_KEY',
    'ANALYTIX_MODEL_PROVIDERS',
    'ANALYTIX_HUB_TEST_GATEWAY_TOKEN',
    'ANALYTIX_HUB_TEST_DESKTOP_AUTH_TOKEN',
    ...PROVIDER_CREDENTIAL_GROUPS.flat(2),
    RETIRED_CONTROLLED_ARTIFACT_HOST_URL_ENV,
    RETIRED_CONTROLLED_ARTIFACT_HOST_TOKEN_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_URL_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_TOKEN_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_BACKEND_GENERATION_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_ALLOCATION_RECORD_DIGEST_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_TLS_ROOT_CERT_DER_ENV,
    CONTROLLED_ARTIFACT_HOST_V2_TLS_LEAF_SPKI_SHA256_ENV,
    ...PROTECTED_AUTHORITY_ENVIRONMENT_V1,
    'ANALYTIX_RUNTIME_TOKEN',
    'ANALYTIX_MCP_CONFIG_PATH',
    'ANALYTIX_APP_ROOT',
    'ANALYTIX_RESOURCES_PATH'
  ])
  // Windows folds environment names when spawning. Remove every alias before
  // installing host-owned values, without rewriting unrelated Path/SystemRoot.
  for (const name of Object.keys(childEnvironment)) {
    if (reservedNames.has(name.toUpperCase())) delete childEnvironment[name]
  }
  return {
    ...childEnvironment,
    ANALYTIX_RUNTIME_TOKEN: runtimeToken,
    ANALYTIX_MCP_CONFIG_PATH: resolveGoRuntimeMCPConfigPath(dataDir),
    ANALYTIX_APP_ROOT: appRoot(),
    ANALYTIX_RESOURCES_PATH: appResourcesPath()
  }
}

export function resolveManagedGoRuntimeToken(
  runtime: Pick<ReturnType<typeof resolveAnalytixRuntimeSettings>, 'runtimeToken' | 'insecure'>,
  generate: () => string = () => randomBytes(32).toString('base64url')
): string {
  const configured = runtime.runtimeToken.trim()
  if (configured || isAnalytixRuntimeInsecure(runtime)) return configured
  const generated = generate().trim()
  if (!generated) throw new Error('Managed runtime token generation failed.')
  return generated
}
