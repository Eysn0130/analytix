import { validateScheduleCalendarRule } from '../../../packages/runtime/src/contracts/schedule-rule'
import { scheduleTaskExpectedFields } from '../../shared/schedule-task-mutation'
import { objectExportBindingSchema } from '../../../packages/runtime/src/contracts/object-editing'
import { z } from 'zod'
export { writePrototypeAuthorizePayloadSchema } from '../../shared/write-prototype'
import { childInputRequestSchemaV1 } from '../../shared/child-input-contract'
import { UserInputResolutionRequest, UserInputWaitContinueRequestSchemaV1 } from '../../../packages/runtime/src/contracts/approvals'
import { toolResultLocalEffectRequestSchemaV1, toolResultLocalPresentFileRequestSchemaV1 } from '../../../packages/runtime/src/contracts/tool-result-local-display'
import {
  MessageFeedbackPutRequestV1Schema,
  MessageFeedbackDeleteRequestV1Schema
} from '../../../packages/runtime/src/contracts/message-feedback'
import {
  ANALYTIX_APPROVAL_TEMPLATE,
  ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE,
  ANALYTIX_ATTACHMENT_DIAGNOSTICS_TEMPLATE,
  ANALYTIX_ATTACHMENTS_TEMPLATE,
  ANALYTIX_ATTACHMENT_TEMPLATE,
  ANALYTIX_CASE_PROJECT_DETAIL_TEMPLATE,
  ANALYTIX_CASE_PROJECT_THREADS_TEMPLATE,
  ANALYTIX_CASE_PROJECTS_TEMPLATE,
  ANALYTIX_HEALTH_TEMPLATE,
  ANALYTIX_MEMORY_DIAGNOSTICS_TEMPLATE,
  ANALYTIX_MEMORY_RECORD_TEMPLATE,
  ANALYTIX_MEMORY_TEMPLATE,
  ANALYTIX_MESSAGE_FEEDBACK_TEMPLATE,
  ANALYTIX_MESSAGE_FEEDBACK_ITEM_TEMPLATE,
  ANALYTIX_RUNTIME_INFO_TEMPLATE,
  ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_KILL_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_LIST_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_RESTART_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_RESUME_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_STEER_TEMPLATE,
  ANALYTIX_RUNTIME_TASK_JOBS_WAIT_TEMPLATE,
  ANALYTIX_RUNTIME_TOOLS_TEMPLATE,
  ANALYTIX_SESSION_RESUME_TEMPLATE,
  ANALYTIX_SKILLS_TEMPLATE,
  ANALYTIX_THREADS_TEMPLATE,
  ANALYTIX_THREAD_COMPACT_TEMPLATE,
  ANALYTIX_THREAD_EVENTS_TEMPLATE,
  ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE,
  ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE,
  ANALYTIX_THREAD_FORK_TEMPLATE,
  ANALYTIX_THREAD_GOAL_TEMPLATE,
  ANALYTIX_THREAD_REWIND_TEMPLATE,
  ANALYTIX_THREAD_REVIEW_TEMPLATE,
  ANALYTIX_THREAD_SUMMARY_TASK_KILL_TEMPLATE,
  ANALYTIX_THREAD_SUMMARY_TASK_OUTPUT_TEMPLATE,
  ANALYTIX_THREAD_SUMMARY_TASK_RESTART_TEMPLATE,
  ANALYTIX_THREAD_SUMMARY_TEMPLATE,
  ANALYTIX_THREAD_TODOS_TEMPLATE,
  ANALYTIX_THREAD_INTERRUPT_TEMPLATE,
  ANALYTIX_THREAD_STEER_TEMPLATE,
  ANALYTIX_THREAD_TURNS_TEMPLATE,
  ANALYTIX_THREAD_TEMPLATE,
  ANALYTIX_USER_INPUT_TEMPLATE,
  ANALYTIX_USAGE_TEMPLATE,
  ANALYTIX_WORKSPACE_STATUS_TEMPLATE,
  ANALYTIX_DEBUG_LLM_ROUNDS_TEMPLATE
} from '../../shared/analytix-endpoints'
import {
  APP_MOTION_PREFERENCES,
  IMAGE_GENERATION_PROTOCOLS,
  MUSIC_GENERATION_PROTOCOLS,
  MODEL_ENDPOINT_FORMATS,
  MODEL_PROVIDER_INPUT_MODALITIES,
  MODEL_PROVIDER_MESSAGE_PARTS,
  MODEL_REASONING_EFFORTS,
  MODEL_REASONING_REQUEST_PROTOCOLS,
  NETWORK_PROXY_PROTOCOLS,
  SCHEDULE_MODEL_IDS,
  SCHEDULE_REASONING_EFFORT_IDS,
  SPEECH_TO_TEXT_PROTOCOLS,
  TEXT_TO_SPEECH_PROTOCOLS,
  VIDEO_GENERATION_PROTOCOLS,
  WRITE_INLINE_COMPLETION_MODEL_IDS
} from '../../shared/app-settings'
import { DESKTOP_COMMANDS } from '../../shared/analytix-api'
import { providerRegistryOAuthBindingSchemaV1 } from '../../../packages/runtime/src/contracts/provider-registry.js'
import { inlineCompletionDocumentSchema } from '../../../packages/runtime/src/contracts/inline-completion'

const oauthProviderIdSchema = z.string().regex(/^[a-z0-9][a-z0-9._-]{0,95}$/)
const oauthAccountComponentSchema = z.string().trim().min(1).max(256).refine(
  (value) => !value.includes('\0') && !/[\r\n\t]/.test(value)
)

export const providerOAuthBeginPayloadSchema = z.object({
  providerId: oauthProviderIdSchema
}).strict()

export const providerOAuthConfigurePayloadSchema = z.object({
  providerId: oauthProviderIdSchema,
  oauthBinding: providerRegistryOAuthBindingSchemaV1
}).strict()

export const mcpOAuthBeginPayloadSchema = z.object({
  serverId: oauthAccountComponentSchema,
  accountId: oauthAccountComponentSchema
}).strict()

export const extensionOAuthBeginPayloadSchema = z.object({
  pluginId: oauthAccountComponentSchema,
  serverId: oauthAccountComponentSchema,
  accountId: oauthAccountComponentSchema
}).strict()

export const oauthAuthorizationIdPayloadSchema = z.object({
  authorizationId: z.string().regex(/^oauth_[A-Za-z0-9_-]{20,192}$/)
}).strict()

export const providerOAuthSubscriptionPayloadSchema = z.object({
  providerId: oauthProviderIdSchema,
  subscriptionToken: z.string().min(1).max(32 * 1024).refine((value) => !value.includes('\0') && !/[\r\n]/.test(value))
}).strict()
import { GUI_UPDATE_CHANNELS } from '../../shared/gui-update'
import { THREAD_TRACE_EVENT_NAMES } from '../../shared/thread-trace'
import { APP_ACCENT_COLORS, WINDOW_CLOSE_ACTIONS } from '../../shared/app-settings'
import { KEYBOARD_SHORTCUT_COMMANDS } from '../../shared/keyboard-shortcuts'
import { WRITE_EXPORT_FORMATS } from '../../shared/write-export'
import { WRITE_INFOGRAPHIC_MAX_TEXT_CHARS } from '../../shared/write-infographic'
import { SPEECH_TRANSCRIPTION_MAX_BASE64_CHARS, SPEECH_TRANSCRIPTION_MAX_DURATION_MS } from '../../shared/speech-to-text'
import {
  TERMINAL_DEFAULT_COLS,
  TERMINAL_DEFAULT_ROWS,
  TERMINAL_MAX_COLS,
  TERMINAL_MAX_CWD_LENGTH,
  TERMINAL_MAX_DATA_WRITE_BYTES,
  TERMINAL_MAX_ROWS,
  TERMINAL_MAX_SESSION_ID_LENGTH
} from '../../shared/terminal'
import type { ThreadHandoffRequest } from '../../shared/thread-handoff'
import { AttachmentUploadRequest } from '../../../packages/runtime/src/contracts/attachments.js'
import {
  acceptedSlotDisplayRequestSchemaV1,
  cleaningDiffPreviewRequestSchemaV1,
  directSourcePreviewRequestSchemaV1,
  importMappingPreviewRequestSchemaV1,
  typedLocalDataSurfaceResponseSchemaV1
} from '../../../packages/runtime/src/contracts/typed-local-data-surface.js'

const MAX_BODY_BYTES = 2_000_000
const MAX_ATTACHMENT_BODY_BYTES = 24 << 20
const MAX_PATH_LENGTH = 4_096
const MAX_URL_LENGTH = 4_096
const MAX_ID_LENGTH = 256
const MAX_BRANCH_LENGTH = 255
const MAX_EDITOR_ID_LENGTH = 64
const MAX_NOTIFICATION_TITLE_LENGTH = 200
const MAX_NOTIFICATION_BODY_LENGTH = 5_000
const MAX_CHANNEL_TEXT_LENGTH = 100_000
const MAX_SKILL_FILE_BYTES = 1_000_000
const MAX_CONFIG_FILE_BYTES = 2_000_000
const MAX_DEVICE_CODE_LENGTH = 8_192
const MAX_EDITOR_COMPLETION_TEXT = 200_000
const MAX_SAVE_FILE_BASE64_BYTES = 64 * 1024 * 1024

const SAFE_OPEN_EXTERNAL_PROTOCOLS = new Set(['http:', 'https:', 'mailto:'])

function trimmedString(max: number): z.ZodString {
  return z.string().trim().min(1).max(max)
}

function optionalTrimmedString(max: number): z.ZodOptional<z.ZodString> {
  return z.string().trim().max(max).optional()
}

export function isSafeOpenExternalUrl(value: string): boolean {
  try {
    const parsed = new URL(value)
    return SAFE_OPEN_EXTERNAL_PROTOCOLS.has(parsed.protocol)
  } catch {
    return false
  }
}

export const defaultPathSchema = optionalTrimmedString(MAX_PATH_LENGTH)

export const confirmDialogPayloadSchema = z
  .object({
    message: trimmedString(4_000),
    detail: z.string().max(8_000).optional(),
    confirmLabel: z.string().trim().max(200).optional(),
    cancelLabel: z.string().trim().max(200).optional()
  })
  .strict()

export const openThreadWindowPayloadSchema = z
  .object({
    threadId: trimmedString(MAX_ID_LENGTH)
  })
  .strict()

export const queryCacheInvalidatePayloadSchema = z
  .object({
    queryKey: z.array(z.string().max(512)).min(1).max(16),
    sourceClientId: z.string().trim().max(128).optional()
  })
  .strict()

export const legacySessionImportPayloadSchema = z
  .object({
    sourceDir: defaultPathSchema
  })
  .strict()

export const providerCapabilityProbePayloadSchema = z
  .object({
    providerId: z.string().trim().min(1).max(128),
    model: z.string().trim().min(1).max(256),
    baseUrl: trimmedString(MAX_URL_LENGTH),
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS)
  })
  .strict()

export const hubLoginPayloadSchema = z
  .object({
    email: trimmedString(320).transform((value) => value.toLowerCase()),
    password: z.string().min(1).max(1_024),
    authChallengeProof: optionalTrimmedString(1_024),
    rememberForDays: z.number().int().min(0).max(365).optional()
  })
  .strict()

export const hubRegisterPayloadSchema = z
  .object({
    fullName: trimmedString(120),
    organization: optionalTrimmedString(120),
    phoneNumber: trimmedString(64),
    email: trimmedString(320).transform((value) => value.toLowerCase()),
    password: z.string().min(1).max(1_024),
    verificationCode: trimmedString(32),
    authChallengeProof: optionalTrimmedString(1_024),
    referralCode: optionalTrimmedString(64),
    rememberForDays: z.number().int().min(0).max(365).optional()
  })
  .strict()

export const hubVerificationCodePayloadSchema = z
  .object({
    email: trimmedString(320).transform((value) => value.toLowerCase()),
    authChallengeProof: optionalTrimmedString(1_024)
  })
  .strict()

export const hubPasswordResetConfirmPayloadSchema = z
  .object({
    email: trimmedString(320).transform((value) => value.toLowerCase()),
    verificationCode: trimmedString(32),
    nextPassword: z.string().min(1).max(1_024)
  })
  .strict()

export const hubAuthChallengeModeSchema = z.enum(['login', 'register'])

export const hubAuthChallengeVerifyPayloadSchema = z
  .object({
    mode: hubAuthChallengeModeSchema,
    challengeId: trimmedString(128),
    selectedTileIds: z.array(trimmedString(128)).min(1).max(12)
  })
  .strict()

export const hubProfileUpdatePayloadSchema = z
  .object({
    displayName: z.string().trim().max(40).optional(),
    username: z.string().trim().max(30).optional(),
    avatarColor: z.string().trim().max(16).optional(),
    avatarStyle: z.enum(['initials', 'xiezhi', 'photo']).optional(),
    avatarImageDataUrl: z.string().trim().max(5_000_000).optional()
  })
  .strict()

const hubProfileInvocationPayloadSchema = z
  .object({
    name: trimmedString(160),
    count: z.number().int().min(1).max(1_000_000)
  })
  .strict()

export const hubProfileEventSyncPayloadSchema = z
  .object({
    events: z.array(z
      .object({
        eventKey: trimmedString(160),
        occurredAt: trimmedString(64),
        source: optionalTrimmedString(64),
        eventKind: optionalTrimmedString(64),
        threadIdHash: optionalTrimmedString(128),
        mode: optionalTrimmedString(64),
        reasoningEffort: optionalTrimmedString(64),
        durationMs: z.number().int().min(0).max(24 * 60 * 60 * 1000).optional(),
        toolInvocations: z.array(hubProfileInvocationPayloadSchema).max(80).optional(),
        skillInvocations: z.array(hubProfileInvocationPayloadSchema).max(80).optional()
      })
      .strict()).max(200)
  })
  .strict()

interface EndpointTemplate {
  /** Compiled path matcher. */
  match(path: string): boolean
  allowedMethods: readonly string[]
}

function compileEndpoint(
  template: string,
  allowedMethods: readonly string[]
): EndpointTemplate {
  // Build a regex from the template by escaping the literal parts and
  // substituting the `{id}` / `{turn}` placeholders with `[^/]+`. The
  // template fragments are URL-encoded by the path helpers, so they
  // contain only characters that are safe to escape directly.
  const pattern = template.replace(/[.+*?^$()|[\]\\]/g, '\\$&').replace(/\{(?:id|turn|checkpoint|task|message)\}/g, '[^/]+')
  const regex = new RegExp(`^${pattern}$`)
  return {
    match: (path: string) => regex.test(path),
    allowedMethods
  }
}

const ENDPOINTS: readonly EndpointTemplate[] = [
  compileEndpoint(ANALYTIX_HEALTH_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_RUNTIME_INFO_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_RUNTIME_TOOLS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_LIST_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_WAIT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_OUTPUT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_KILL_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_RESTART_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_RECOVER_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_STEER_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_PAUSE_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_RESUME_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REVIEW_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REJECT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CLEANUP_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_ACCEPT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_CONFLICT_REPORT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_CHECK_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_ISOLATION_REPAIR_ACCEPT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_PROJECT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_REJECT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_RUNTIME_TASK_JOBS_CHILD_TODOS_ACCEPT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_SKILLS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_ATTACHMENTS_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_ATTACHMENT_DIAGNOSTICS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_ATTACHMENT_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_ATTACHMENT_CONTENT_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_MEMORY_TEMPLATE, ['GET', 'POST']),
  compileEndpoint(ANALYTIX_MEMORY_DIAGNOSTICS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_MEMORY_RECORD_TEMPLATE, ['PATCH', 'DELETE']),
  compileEndpoint(ANALYTIX_WORKSPACE_STATUS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_CASE_PROJECTS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_CASE_PROJECT_THREADS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_CASE_PROJECT_DETAIL_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_THREADS_TEMPLATE, ['GET', 'POST']),
  compileEndpoint(ANALYTIX_THREAD_TEMPLATE, ['GET', 'PATCH', 'DELETE']),
  compileEndpoint(ANALYTIX_MESSAGE_FEEDBACK_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_MESSAGE_FEEDBACK_ITEM_TEMPLATE, ['PUT', 'DELETE']),
  compileEndpoint(ANALYTIX_THREAD_SUMMARY_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_THREAD_SUMMARY_TASK_OUTPUT_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_THREAD_SUMMARY_TASK_KILL_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_SUMMARY_TASK_RESTART_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_EVENTS_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_THREAD_FORK_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_GOAL_TEMPLATE, ['GET', 'POST', 'DELETE']),
  compileEndpoint(ANALYTIX_THREAD_TODOS_TEMPLATE, ['GET', 'POST', 'DELETE']),
  compileEndpoint(ANALYTIX_THREAD_COMPACT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_REWIND_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_REVIEW_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_CHECKPOINT_REWIND_PLAN_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_CHECKPOINT_REWIND_APPLY_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_TURNS_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_STEER_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_THREAD_INTERRUPT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_APPROVAL_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_USER_INPUT_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_SESSION_RESUME_TEMPLATE, ['POST']),
  compileEndpoint(ANALYTIX_USAGE_TEMPLATE, ['GET']),
  compileEndpoint(ANALYTIX_DEBUG_LLM_ROUNDS_TEMPLATE, ['GET'])
]

function isAllowedRuntimeRequest(value: { path: string; method?: string }): boolean {
  try {
    const url = new URL(value.path, 'http://localhost')
    const path = url.pathname
    const method = value.method ?? 'GET'
    for (const endpoint of ENDPOINTS) {
      if (endpoint.match(path)) {
        return endpoint.allowedMethods.includes(method)
      }
    }
    return false
  } catch {
    return false
  }
}

export const runtimeRequestPayloadSchema = z
  .object({
    path: trimmedString(MAX_URL_LENGTH).transform((value) =>
      value.startsWith('/') ? value : `/${value}`
    ),
    method: z.enum(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']).optional(),
    body: z.string().max(MAX_ATTACHMENT_BODY_BYTES).optional()
  })
  .refine((payload) => isAllowedRuntimeRequest(payload), {
    message: 'runtime request path is not allowed'
  })
  .superRefine((payload, context) => {
    const url = new URL(payload.path, 'http://localhost')
    const method = payload.method ?? 'GET'
    if (url.pathname === ANALYTIX_RUNTIME_TASK_JOBS_STEER_TEMPLATE) {
      try {
        if (payload.path !== ANALYTIX_RUNTIME_TASK_JOBS_STEER_TEMPLATE || method !== 'POST' ||
            new TextEncoder().encode(payload.body ?? '').length > 24 * 1024 ||
            !childInputRequestSchemaV1.safeParse(JSON.parse(payload.body ?? '') as unknown).success) {
          context.addIssue({ code: 'custom', message: 'child input request is invalid', path: ['body'] })
        }
      } catch {
        context.addIssue({ code: 'custom', message: 'child input request is invalid', path: ['body'] })
      }
      return
    }
    if (/^\/v1\/threads\/[^/]+\/message-feedback(?:\/|$)/.test(url.pathname)) {
      const route = /^\/v1\/threads\/([^/]+)\/message-feedback(?:\/([^/]+))?$/.exec(payload.path)
      const canonicalId = (value: string): boolean => {
        try {
          const decoded = decodeURIComponent(value)
          return /^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$/.test(decoded) && !decoded.includes('..')
        } catch { return false }
      }
      if (!route || !canonicalId(route[1]) || route[2] && !canonicalId(route[2])) {
        context.addIssue({ code: 'custom', message: 'message feedback target is invalid', path: ['path'] })
        return
      }
      if (!route[2]) {
        if (method !== 'GET' || payload.body !== undefined) {
          context.addIssue({ code: 'custom', message: 'message feedback list request is invalid', path: ['body'] })
        }
        return
      }
      try {
        const schema = method === 'PUT' ? MessageFeedbackPutRequestV1Schema : MessageFeedbackDeleteRequestV1Schema
        if ((method !== 'PUT' && method !== 'DELETE') || (payload.body?.length ?? 0) > 65536 ||
            !schema.safeParse(JSON.parse(payload.body ?? '') as unknown).success) {
          context.addIssue({ code: 'custom', message: 'message feedback body is invalid', path: ['body'] })
        }
      } catch {
        context.addIssue({ code: 'custom', message: 'message feedback body is invalid', path: ['body'] })
      }
      return
    }
    if (url.pathname === ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_TEMPLATE &&
        (payload.path !== ANALYTIX_RUNTIME_TOOL_EXECUTIONS_OBSERVE_TEMPLATE || method !== 'POST')) {
      context.addIssue({
        code: 'custom',
        message: 'tool execution observation target is invalid',
        path: ['path']
      })
      return
    }
    if (/^\/v1\/user-inputs\/[^/]+$/.test(url.pathname) && method === 'POST') {
      try {
        if (!z.union([UserInputResolutionRequest, UserInputWaitContinueRequestSchemaV1]).safeParse(JSON.parse(payload.body ?? '') as unknown).success) {
          context.addIssue({ code: 'custom', message: 'user input response body is invalid', path: ['body'] })
        }
      } catch {
        context.addIssue({ code: 'custom', message: 'user input response body is invalid', path: ['body'] })
      }
      return
    }
    if (url.pathname === '/v1/attachments' && method === 'POST') {
      try {
        const parsed = JSON.parse(payload.body ?? '') as unknown
        if (!AttachmentUploadRequest.safeParse(parsed).success) {
          context.addIssue({ code: 'custom', message: 'attachment upload body is invalid', path: ['body'] })
        }
      } catch {
        context.addIssue({ code: 'custom', message: 'attachment upload body is invalid', path: ['body'] })
      }
      return
    }
    if (/^\/v1\/attachments\/[^/]+(?:\/content)?$/.test(url.pathname) && method === 'GET') {
      const keys = [...url.searchParams.keys()]
      if (keys.length !== 2 || new Set(keys).size !== 2 ||
        url.searchParams.getAll('thread_id').length !== 1 ||
        url.searchParams.getAll('workspace').length !== 1 ||
        !url.searchParams.get('thread_id')?.trim() ||
        !url.searchParams.get('workspace')?.trim()) {
        context.addIssue({ code: 'custom', message: 'attachment scope query is invalid', path: ['path'] })
      }
    }
    if ((payload.body?.length ?? 0) > MAX_BODY_BYTES) {
      context.addIssue({ code: 'custom', message: 'runtime request body is too large', path: ['body'] })
    }
  })
  .strict()

export const importMappingPreviewRequestSchema = importMappingPreviewRequestSchemaV1
export const cleaningDiffPreviewRequestSchema = cleaningDiffPreviewRequestSchemaV1
export const directSourcePreviewRequestSchema = directSourcePreviewRequestSchemaV1
export const acceptedSlotDisplayRequestSchema = acceptedSlotDisplayRequestSchemaV1
export const localDisplayResponseSchema = typedLocalDataSurfaceResponseSchemaV1

const localeSchema = z.enum(['en', 'zh'])
const themeSchema = z.enum(['system', 'light', 'dark'])
const uiFontScaleSchema = z.enum(['small', 'medium', 'large'])
const motionPreferenceSchema = z.enum(APP_MOTION_PREFERENCES)
const approvalPolicySchema = z.enum(['always', 'on-request', 'untrusted', 'never', 'auto', 'suggest'])
const sandboxModeSchema = z.enum(['read-only', 'workspace-write', 'danger-full-access', 'external-sandbox'])
const mcpSearchModeSchema = z.enum(['direct', 'search', 'auto'])
const analytixStorageBackendSchema = z.enum(['hybrid', 'file'])
const analytixCompactionSummaryModeSchema = z.enum(['heuristic', 'model'])
const clawRunModeSchema = z.enum(['agent', 'plan'])
const clawImProviderSchema = z.enum(['feishu', 'weixin', 'telegram'])
const clawScheduleKindSchema = z.enum(['manual', 'interval', 'daily', 'at'])
const clawTaskStatusSchema = z.enum(['idle', 'running', 'success', 'error'])
const scheduleReasoningEffortSchema = z.enum(SCHEDULE_REASONING_EFFORT_IDS)
const writeInlineCompletionModelSchema = z.union([
  z.enum(WRITE_INLINE_COMPLETION_MODEL_IDS),
  trimmedString(128)
])
const modelEndpointFormatSchema = z.enum(MODEL_ENDPOINT_FORMATS)
const networkProxyProtocolSchema = z.enum(NETWORK_PROXY_PROTOCOLS)
const networkProxySchema = z.object({
  enabled: z.boolean().optional(),
  url: z.string().trim().max(MAX_URL_LENGTH).optional()
}).strict().superRefine((value, ctx) => {
  const url = value.url?.trim()
  if (!url) return
  try {
    const parsed = new URL(url)
    const protocol = parsed.protocol.replace(/:$/, '').toLowerCase()
    if (!networkProxyProtocolSchema.safeParse(protocol).success || !parsed.hostname || !parsed.port) {
      ctx.addIssue({ code: 'custom', path: ['url'], message: 'Invalid proxy URL.' })
    }
  } catch {
    ctx.addIssue({ code: 'custom', path: ['url'], message: 'Invalid proxy URL.' })
  }
})
const imageGenerationProtocolSchema = z.enum(IMAGE_GENERATION_PROTOCOLS)
const speechToTextProtocolSchema = z.enum(SPEECH_TO_TEXT_PROTOCOLS)
const textToSpeechProtocolSchema = z.enum(TEXT_TO_SPEECH_PROTOCOLS)
const musicGenerationProtocolSchema = z.enum(MUSIC_GENERATION_PROTOCOLS)
const videoGenerationProtocolSchema = z.enum(VIDEO_GENERATION_PROTOCOLS)
const speechToTextSettingsSchema = z.object({
  enabled: z.boolean(),
  providerId: z.string().trim().max(64),
  protocol: speechToTextProtocolSchema,
  baseUrl: z.string().trim().max(MAX_URL_LENGTH),
  model: z.string().trim().max(128),
  language: z.string().trim().max(16),
  timeoutMs: z.number().int().positive().max(600_000)
}).strict()
const modelProviderInputModalitySchema = z.enum(MODEL_PROVIDER_INPUT_MODALITIES)
const modelProviderMessagePartSchema = z.enum(MODEL_PROVIDER_MESSAGE_PARTS)
const modelReasoningEffortSchema = z.enum(MODEL_REASONING_EFFORTS)
const modelReasoningRequestProtocolSchema = z.enum(MODEL_REASONING_REQUEST_PROTOCOLS)
const modelProfilePatchSchema = z.object({
  aliases: z.array(z.string().trim().min(1).max(128)).max(50).optional(),
  contextWindowTokens: z.number().int().positive().max(10_000_000).optional(),
  inputModalities: z.array(modelProviderInputModalitySchema).max(8).optional(),
  outputModalities: z.array(modelProviderInputModalitySchema).max(8).optional(),
  supportsToolCalling: z.boolean().optional(),
  messageParts: z.array(modelProviderMessagePartSchema).max(8).optional(),
  reasoning: z.object({
    supportedEfforts: z.array(modelReasoningEffortSchema).min(1).max(8),
    defaultEffort: modelReasoningEffortSchema,
    requestProtocol: modelReasoningRequestProtocolSchema
  }).strict().optional(),
  endpointFormat: modelEndpointFormatSchema.optional(),
  price: z.object({
    cacheHit: z.number().nonnegative().max(1_000_000).optional(),
    input: z.number().nonnegative().max(1_000_000).optional(),
    output: z.number().nonnegative().max(1_000_000).optional(),
    currency: z.enum(['USD', 'CNY']).optional()
  }).strict().optional()
}).strict()

const modelCapabilityProbeStatusSchema = z.enum([
  'unknown',
  'supported',
  'unsupported',
  'semantic_failed',
  'auth_failed',
  'http_failed',
  'timeout',
  'failed',
  'stale'
])

const modelCapabilityProbeResultSchema = z.object({
  key: z.string().trim().min(1).max(1024),
  providerId: z.string().trim().min(1).max(128),
  model: z.string().trim().min(1).max(256),
  endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS),
  sanitizedBaseUrl: z.string().trim().min(1).max(MAX_URL_LENGTH),
  requestUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  probedAt: z.string().trim().min(1).max(64),
  staleAfter: z.string().trim().min(1).max(64),
  imageInput: modelCapabilityProbeStatusSchema,
  toolCalling: modelCapabilityProbeStatusSchema,
  toolResultImage: modelCapabilityProbeStatusSchema,
  status: modelCapabilityProbeStatusSchema,
  httpStatus: z.number().int().min(100).max(599).optional(),
  errorSummary: z.string().trim().max(1000).optional()
}).strict()

const providerPricingSchema = z.object({
  cacheHit: z.number().nonnegative().max(1_000_000).optional(),
  input: z.number().nonnegative().max(1_000_000).optional(),
  output: z.number().nonnegative().max(1_000_000).optional(),
  currency: z.enum(['USD', 'CNY']).optional()
}).strict()

function hasModelPreferenceControls(value: string): boolean {
  for (const character of value) {
    const code = character.charCodeAt(0)
    if (code < 32 || code === 127) return true
  }
  return false
}

const modelPreferenceTextSchema = z.string().min(1).max(128).refine((value) =>
  value === value.trim() && !hasModelPreferenceControls(value))
const modelPreferenceSchema = z.object({
  aliases: z.array(modelPreferenceTextSchema).max(50).refine((values) =>
    new Set(values).size === values.length).optional(),
  reasoning: z.object({
    supportedEfforts: z.array(modelReasoningEffortSchema).min(1).max(6).refine((values) =>
      new Set(values).size === values.length),
    defaultEffort: modelReasoningEffortSchema,
    requestProtocol: modelReasoningRequestProtocolSchema
  }).strict().refine((value) => value.supportedEfforts.includes(value.defaultEffort)).nullable().optional()
}).strict()
const modelPreferenceRecordSchema = z.unknown().transform((value, ctx) => {
  const invalid = () => {
    ctx.addIssue({ code: 'custom', message: 'Invalid model preference record.' })
    return z.NEVER
  }
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return invalid()
  const entries = Object.entries(value)
  if (entries.length > 200) return invalid()
  const keys = new Set<string>()
  const output: Record<string, z.infer<typeof modelPreferenceSchema>> = Object.create(null)
  for (const [key, profile] of entries) {
    if (!modelPreferenceTextSchema.safeParse(key).success || keys.has(key.toLowerCase())) return invalid()
    const parsed = modelPreferenceSchema.safeParse(profile)
    if (!parsed.success) return invalid()
    keys.add(key.toLowerCase())
    output[key] = parsed.data
  }
  return output
})
const modelPreferenceRuntimeByteLength = (json: string): number => {
  const encoded = json.replace(/[<>&\u2028\u2029]/g, (value) =>
    `\\u${value.charCodeAt(0).toString(16).padStart(4, '0')}`)
  return new TextEncoder().encode(encoded).byteLength
}
const modelPreferencesSchema = z.array(z.object({
  id: z.string().regex(/^[a-z0-9][a-z0-9._-]{0,95}$/),
  baseUrl: z.string().min(1).max(2048).refine((value) => {
    if (hasModelPreferenceControls(value) || /[\s?#\\]/.test(value)) return false
    try {
      const url = new URL(value)
      return ['http:', 'https:'].includes(url.protocol) && Boolean(url.host) &&
        !url.username && !url.password && !url.search && !url.hash
    } catch { return false }
  }),
  endpointFormat: modelEndpointFormatSchema,
  modelProfiles: modelPreferenceRecordSchema
}).strict()).max(128).refine((values) => new Set(values.map((value) => value.id)).size === values.length)
  .refine((values) => modelPreferenceRuntimeByteLength(JSON.stringify({ providers: values
    .filter((entry) => Object.keys(entry.modelProfiles).length > 0).map((entry) => ({
    ...entry, modelProfiles: Object.fromEntries(Object.entries(entry.modelProfiles).map(([model, profile]) => [model, {
      ...profile, ...(profile.reasoning === null ? { reasoning: {
        supportedEfforts: ['off'], defaultEffort: 'off', requestProtocol: 'none'
      } } : {})
    }]))
  })) })) <= 1_048_576)

const modelProviderPatchSchema = z.object({
  modelPreferences: modelPreferencesSchema.optional(),
  activeProviderId: z.string().trim().max(64).optional(),
  baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  proxy: networkProxySchema.optional(),
  providers: z.array(z.object({
    id: z.string().trim().min(1).max(64).optional(),
    name: z.string().trim().min(1).max(80).optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    endpointFormat: modelEndpointFormatSchema.optional(),
    models: z.array(z.string().trim().min(1).max(128)).max(200).optional(),
    price: providerPricingSchema.optional(),
    prices: z.record(z.string().trim().min(1).max(128), providerPricingSchema).optional(),
    // 兼容旧版保存的视觉识别能力字段。当前能力已经迁移到 modelProfiles 的 inputModalities/messageParts。
    imageRecognition: z.unknown().optional(),
    modelProfiles: z.record(
      z.string().trim().min(1).max(128),
      modelProfilePatchSchema.nullable()
    ).optional(),
    image: z.object({
      protocol: imageGenerationProtocolSchema.optional(),
      baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
      models: z.array(z.string().trim().min(1).max(128)).max(50).optional()
    }).strict().nullable().optional(),
    speech: z.object({
      protocol: speechToTextProtocolSchema.optional(),
      baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
      models: z.array(z.string().trim().min(1).max(128)).max(50).optional()
    }).strict().nullable().optional(),
    textToSpeech: z.object({
      protocol: textToSpeechProtocolSchema.optional(),
      baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
      models: z.array(z.string().trim().min(1).max(128)).max(50).optional()
    }).strict().nullable().optional(),
    music: z.object({
      protocol: musicGenerationProtocolSchema.optional(),
      baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
      models: z.array(z.string().trim().min(1).max(128)).max(50).optional()
    }).strict().nullable().optional(),
    video: z.object({
      protocol: videoGenerationProtocolSchema.optional(),
      baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
      models: z.array(z.string().trim().min(1).max(128)).max(50).optional()
    }).strict().nullable().optional()
  }).strict()).max(50).optional()
}).strict()

const analytixRuntimePatchSchema = z.object({
  binaryPath: defaultPathSchema,
  port: z.number().int().min(1).max(65_535).optional(),
  autoStart: z.boolean().optional(),
  baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  providerId: z.string().trim().max(64).optional(),
  endpointFormat: modelEndpointFormatSchema.optional(),
  runtimeToken: z.string().max(MAX_BODY_BYTES).optional(),
  dataDir: defaultPathSchema,
  model: z.string().trim().min(1).max(128).optional(),
  executionPolicyVersion: z.literal(2).optional(),
  approvalPolicy: approvalPolicySchema.optional(),
  sandboxMode: sandboxModeSchema.optional(),
  tokenEconomyMode: z.boolean().optional(),
  tokenEconomy: z.object({
    enabled: z.boolean().optional(),
    compressToolDescriptions: z.boolean().optional(),
    compressToolResults: z.boolean().optional(),
    conciseResponses: z.boolean().optional(),
    historyHygiene: z.object({
      maxToolResultLines: z.number().int().positive().max(100_000).optional(),
      maxToolResultBytes: z.number().int().positive().max(8 * 1024 * 1024).optional(),
      maxToolResultTokens: z.number().int().positive().max(256_000).optional(),
      maxToolArgumentStringBytes: z.number().int().positive().max(8 * 1024 * 1024).optional(),
      maxToolArgumentStringTokens: z.number().int().positive().max(64_000).optional(),
      maxArrayItems: z.number().int().positive().max(10_000).optional(),
      maxCumulativeToolResultTokens: z.number().int().nonnegative().max(2_000_000).optional(),
      keepRecentToolResults: z.number().int().nonnegative().max(10_000).optional()
    }).strict().optional()
  }).strict().optional(),
  insecure: z.boolean().optional(),
  mcpSearch: z.object({
    enabled: z.boolean().optional(),
    mode: mcpSearchModeSchema.optional(),
    autoThresholdToolCount: z.number().int().positive().optional(),
    topKDefault: z.number().int().positive().optional(),
    topKMax: z.number().int().positive().optional(),
    minScore: z.number().nonnegative().optional()
  }).strict().optional(),
  storage: z.object({
    backend: analytixStorageBackendSchema.optional(),
    sqlitePath: defaultPathSchema
  }).strict().optional(),
  contextCompaction: z.object({
    defaultSoftThreshold: z.number().int().positive().optional(),
    defaultHardThreshold: z.number().int().positive().optional(),
    summaryMode: analytixCompactionSummaryModeSchema.optional(),
    summaryTimeoutMs: z.number().int().positive().max(120_000).optional(),
    summaryMaxTokens: z.number().int().positive().max(16_000).optional(),
    summaryInputMaxBytes: z.number().int().positive().max(8 * 1024 * 1024).optional()
  }).strict().optional(),
  userInput: z.object({
    policy: z.enum(['legacy', 'timed']),
    defaultTimeoutSeconds: z.number().int().refine(value => value === -1 || value >= 1 && value <= 2147483).nullable().optional()
  }).strict().optional(),
  runtimeTuning: z.object({
    streamIdleTimeoutMs: z.number().int().min(0).max(3_600_000).optional(),
    stepLimits: z.object({
      defaultMaxModelSteps: z.number().int().min(0).max(10_000).optional(),
      userGlobalMaxModelSteps: z.number().int().min(0).max(10_000).optional(),
      plannerMaxModelSteps: z.number().int().min(0).max(10_000).optional(),
      headlessMaxModelSteps: z.number().int().min(0).max(10_000).optional()
    }).strict().optional(),
    toolStorm: z.object({
      enabled: z.boolean().optional(),
      windowSize: z.number().int().positive().max(128).optional(),
      threshold: z.number().int().min(2).max(128).optional()
    }).strict().optional(),
    toolArgumentRepair: z.object({
      maxStringBytes: z.number().int().positive().max(16 * 1024 * 1024).optional()
    }).strict().optional()
  }).strict().optional(),
  quality: z.object({
    enabled: z.boolean().optional(),
    strictness: z.enum(['relaxed', 'standard', 'strict']).optional(),
    ignoreRules: z.array(z.string().trim().min(1).max(128)).max(200).optional(),
    ignoreFiles: z.array(z.string().trim().min(1).max(256)).max(200).optional(),
    maxFindings: z.number().int().positive().max(100).optional()
  }).strict().optional(),
  imageGeneration: z.object({
    enabled: z.boolean().optional(),
    providerId: z.string().trim().max(64).optional(),
    protocol: imageGenerationProtocolSchema.optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    model: z.string().trim().max(128).optional(),
    defaultSize: z.string().trim().max(16).optional(),
    timeoutMs: z.number().int().positive().max(600_000).optional()
  }).strict().optional(),
  speechToText: z.object({
    enabled: z.boolean().optional(),
    providerId: z.string().trim().max(64).optional(),
    protocol: speechToTextProtocolSchema.optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    model: z.string().trim().max(128).optional(),
    language: z.string().trim().max(16).optional(),
    timeoutMs: z.number().int().positive().max(600_000).optional()
  }).strict().optional(),
  textToSpeech: z.object({
    enabled: z.boolean().optional(),
    providerId: z.string().trim().max(64).optional(),
    protocol: textToSpeechProtocolSchema.optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    model: z.string().trim().max(128).optional(),
    voice: z.string().trim().max(128).optional(),
    format: z.string().trim().max(16).optional(),
    timeoutMs: z.number().int().positive().max(900_000).optional()
  }).strict().optional(),
  musicGeneration: z.object({
    enabled: z.boolean().optional(),
    providerId: z.string().trim().max(64).optional(),
    protocol: musicGenerationProtocolSchema.optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    model: z.string().trim().max(128).optional(),
    format: z.string().trim().max(16).optional(),
    timeoutMs: z.number().int().positive().max(1_800_000).optional()
  }).strict().optional(),
  videoGeneration: z.object({
    enabled: z.boolean().optional(),
    providerId: z.string().trim().max(64).optional(),
    protocol: videoGenerationProtocolSchema.optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    model: z.string().trim().max(128).optional(),
    defaultDuration: z.number().int().positive().max(30).optional(),
    defaultResolution: z.string().trim().max(32).optional(),
    timeoutMs: z.number().int().positive().max(3_600_000).optional(),
    pollIntervalMs: z.number().int().positive().max(120_000).optional()
  }).strict().optional(),
  computerUse: z.object({
    enabled: z.boolean().optional(),
    mode: z.enum(['auto', 'always', 'off']).optional(),
    maxImageDimension: z.number().int().positive().max(4096).optional(),
    maxActionsPerTurn: z.number().int().positive().max(1000).optional(),
    allowWhenLocked: z.boolean().optional()
  }).strict().optional(),
  browserUse: z.object({
    enabled: z.boolean().optional(),
    localUrlOpenTarget: z.enum(['analytix', 'system']).optional(),
    annotationScreenshotsMode: z.enum(['always', 'necessary', 'off']).optional(),
    approvalMode: z.enum(['alwaysAsk', 'neverAsk']).optional(),
    fullCdpAccess: z.boolean().optional(),
    chromeControlEnabled: z.boolean().optional(),
    sitePermissions: z.array(z.object({
      origin: z.string().trim().min(1).max(512).optional(),
      approvalMode: z.enum(['alwaysAsk', 'neverAsk']).optional(),
      downloadApprovalMode: z.enum(['alwaysAsk', 'neverAsk']).optional(),
      uploadApprovalMode: z.enum(['alwaysAsk', 'neverAsk']).optional(),
      fullCdpAccess: z.boolean().optional()
    }).strict().nullable()).max(100).optional()
  }).strict().optional(),
  visionBridge: z.object({
    enabled: z.boolean().optional(),
    mode: z.enum(['auto', 'always', 'off']).optional(),
    providerId: z.string().trim().max(64).optional(),
    model: z.string().trim().max(128).optional(),
    baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
    endpointFormat: z.enum(MODEL_ENDPOINT_FORMATS).optional(),
    maxImageDimension: z.number().int().positive().max(4096).optional(),
    maxImageBytes: z.number().int().positive().max(8 * 1024 * 1024).optional(),
    maxScreenshotsPerTurn: z.number().int().positive().max(16).optional(),
    observationCacheTtlMs: z.number().int().positive().max(3_600_000).optional(),
    injectPolicy: z.literal('observation_text').optional(),
    fallbackWhenPrimaryImageUnsupported: z.boolean().optional()
  }).strict().optional(),
  // 兼容旧版保存的独立视觉识别设置。当前能力已经迁移到 provider modelProfiles。
  imageRecognition: z.unknown().optional(),
  modelProfiles: z.record(
    z.string().trim().min(1).max(128),
    modelProfilePatchSchema.nullable()
  ).optional(),
  modelCapabilityProbes: z.record(
    z.string().trim().min(1).max(1024),
    modelCapabilityProbeResultSchema.nullable()
  ).optional(),
  subagents: z.object({
    enabled: z.boolean().optional(),
    maxParallel: z.number().int().min(0).max(128).optional(),
    maxChildRuns: z.number().int().min(0).max(10_000).optional(),
    defaultToolPolicy: z.enum(['readOnly', 'inherit']).optional(),
    defaultProfile: z.string().trim().max(128).optional(),
    profiles: z.record(
      z.string().trim().min(1).max(128),
      z.object({
        prompt: z.string().trim().max(16 * 1024).optional(),
        model: z.string().trim().max(256).optional(),
        effort: z.enum(['off', 'low', 'medium', 'high', 'max']).or(z.literal('')).optional(),
        toolPolicy: z.enum(['readOnly', 'inherit']).optional(),
        tools: z.array(z.string().trim().min(1).max(128)).max(128).optional()
      }).strict().nullable()
    ).optional()
  }).strict().optional(),
  memoryEnabled: z.boolean().optional()
}).strict()

const logPatchSchema = z.object({
  enabled: z.boolean().optional(),
  retentionDays: z.number().int().min(1).max(365).optional()
}).strict()

const notificationsPatchSchema = z.object({
  turnComplete: z.boolean().optional()
}).strict()

const appBehaviorPatchSchema = z.object({
  openAtLogin: z.boolean().optional(),
  startMinimized: z.boolean().optional(),
  closeAction: z.enum(WINDOW_CLOSE_ACTIONS).optional(),
  closeToTray: z.boolean().optional()
}).strict()

const keyboardShortcutCommandIds = KEYBOARD_SHORTCUT_COMMANDS.map((command) => command.id) as [
  typeof KEYBOARD_SHORTCUT_COMMANDS[number]['id'],
  ...Array<typeof KEYBOARD_SHORTCUT_COMMANDS[number]['id']>
]

const keyboardShortcutsPatchSchema = z.object({
  bindings: z.partialRecord(
    z.enum(keyboardShortcutCommandIds),
    z.array(z.string().trim().max(64)).max(4)
  ).optional()
}).strict()

const writeInlineCompletionPatchSchema = z.object({
  enabled: z.boolean().optional(),
  retrievalEnabled: z.boolean().optional(),
  longCompletionEnabled: z.boolean().optional(),
  inheritProvider: z.boolean().optional(),
  providerId: z.string().trim().max(64).optional(),
  baseUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  inheritModel: z.boolean().optional(),
  model: writeInlineCompletionModelSchema.optional(),
  debounceMs: z.number().int().min(150).max(5_000).optional(),
  longDebounceMs: z.number().int().min(1_000).max(15_000).optional(),
  minAcceptScore: z.number().min(0.1).max(0.95).optional(),
  longMinAcceptScore: z.number().min(0.1).max(0.95).optional(),
  maxTokens: z.number().int().min(16).max(512).optional(),
  longMaxTokens: z.number().int().min(64).max(1_024).optional()
}).strict()

const writeQuickActionSchema = z.object({
  id: trimmedString(64),
  label: z.string().max(64).optional(),
  prompt: z.string().max(4_000).optional(),
  mode: z.enum(['edit', 'chat']).optional()
}).strict()

const writeSelectionAssistPatchSchema = z.object({
  infographicPrompt: z.string().max(4_000).optional(),
  designDraftPrompt: z.string().max(4_000).optional(),
  prototypePrompt: z.string().max(4_000).optional(),
  quickActions: z.array(writeQuickActionSchema).max(24).optional()
}).strict()

const writeTypographyPatchSchema = z.object({
  fontPreset: z.string().max(32).optional(),
  customFontFamily: z.string().max(200).optional(),
  fontSizePx: z.number().optional(),
  lineHeight: z.number().optional(),
  textAlign: z.string().max(16).optional()
}).strict()

const writeAgentPresetSchema = z.object({
  id: trimmedString(64),
  name: z.string().max(64).optional(),
  emoji: z.string().max(16).optional(),
  persona: z.string().max(4_000).optional()
}).strict()

const writeSettingsPatchSchema = z.object({
  defaultWorkspaceRoot: defaultPathSchema,
  activeWorkspaceRoot: defaultPathSchema,
  workspaces: z.array(trimmedString(MAX_PATH_LENGTH)).max(256).optional(),
  inlineCompletion: writeInlineCompletionPatchSchema.optional(),
  selectionAssist: writeSelectionAssistPatchSchema.optional(),
  typography: writeTypographyPatchSchema.optional(),
  agentPresets: z.array(writeAgentPresetSchema).max(24).optional()
}).strict()

const clawSkillPatchSchema = z.object({
  defaultNames: z.array(trimmedString(128)).max(128).optional(),
  extraDirs: z.array(trimmedString(MAX_PATH_LENGTH)).max(128).optional(),
  disabledDirs: z.array(trimmedString(MAX_PATH_LENGTH)).max(128).optional(),
  promptPrefix: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional()
}).strict()

const clawImPatchSchema = z.object({
  enabled: z.boolean().optional(),
  provider: clawImProviderSchema.optional(),
  port: z.number().int().min(1024).max(65_535).optional(),
  path: trimmedString(MAX_PATH_LENGTH).optional(),
  secret: z.string().max(MAX_BODY_BYTES).optional(),
  weixinBridgeUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  openClawGatewayUrl: z.string().trim().max(MAX_URL_LENGTH).optional(),
  workspaceRoot: defaultPathSchema,
  providerId: z.string().trim().max(64).optional(),
  model: z.string().trim().min(1).max(128).optional(),
  mode: clawRunModeSchema.optional(),
  responseTimeoutMs: z.number().int().min(5_000).max(600_000).optional()
}).strict()

const clawImAgentProfilePatchSchema = z.object({
  name: z.string().max(200).optional(),
  description: z.string().max(2_000).optional(),
  identity: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  personality: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  userContext: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  replyRules: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional()
}).strict()

const clawImPlatformAccountPatchSchema = z.union([
  z.object({
    kind: z.literal('feishu'),
    accountId: z.string().trim().min(1).max(512),
    appId: z.string().max(512).optional(),
    domain: z.string().max(512).optional(),
    createdAt: z.string().max(128).optional()
  }).strict(),
  z.object({
    kind: z.literal('weixin'),
    accountId: z.string().trim().min(1).max(512),
    createdAt: z.string().max(128).optional()
  }).strict(),
  z.object({
    kind: z.literal('telegram'),
    accountId: z.string().trim().min(1).max(512),
    allowedChatIds: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
    botUsername: z.string().trim().max(128).optional(),
    createdAt: z.string().max(128).optional()
  }).strict()
])

const clawImRemoteSessionPatchSchema = z.object({
  chatId: z.string().max(MAX_ID_LENGTH).optional(),
  messageId: z.string().max(MAX_ID_LENGTH).optional(),
  threadId: z.string().max(MAX_ID_LENGTH).optional(),
  senderId: z.string().max(MAX_ID_LENGTH).optional(),
  senderName: z.string().max(512).optional(),
  updatedAt: z.string().max(128).optional()
}).strict()

const clawImConversationPatchSchema = z.object({
  id: z.string().max(MAX_ID_LENGTH).optional(),
  chatId: z.string().max(MAX_ID_LENGTH).optional(),
  remoteThreadId: z.string().max(MAX_ID_LENGTH).optional(),
  latestMessageId: z.string().max(MAX_ID_LENGTH).optional(),
  senderId: z.string().max(MAX_ID_LENGTH).optional(),
  senderName: z.string().max(512).optional(),
  localThreadId: z.string().max(MAX_ID_LENGTH).optional(),
  workspaceRoot: defaultPathSchema,
  createdAt: z.string().max(128).optional(),
  updatedAt: z.string().max(128).optional()
}).strict()

const clawImChannelPatchSchema = z.object({
  id: z.string().max(MAX_ID_LENGTH).optional(),
  provider: clawImProviderSchema.optional(),
  label: z.string().max(512).optional(),
  enabled: z.boolean().optional(),
  providerId: z.string().trim().max(64).optional(),
  model: z.string().trim().min(1).max(128).optional(),
  threadId: z.string().max(MAX_ID_LENGTH).optional(),
  workspaceRoot: defaultPathSchema,
  agentProfile: clawImAgentProfilePatchSchema.optional(),
  platformAccount: clawImPlatformAccountPatchSchema.optional(),
  remoteSession: clawImRemoteSessionPatchSchema.optional(),
  conversations: z.array(clawImConversationPatchSchema).max(512).optional(),
  welcomeSentAt: z.string().max(128).optional(),
  createdAt: z.string().max(128).optional(),
  updatedAt: z.string().max(128).optional(),
  feishuStream: z.boolean().optional()
}).strict()

const clawTaskSchedulePatchSchema = z.object({
  kind: clawScheduleKindSchema.optional(),
  everyMinutes: z.number().int().min(1).max(10_080).optional(),
  timeOfDay: z.string().max(16).optional(),
  atTime: z.string().max(128).optional()
}).strict()

const clawTaskPatchSchema = z.object({
  id: z.string().max(MAX_ID_LENGTH).optional(),
  title: z.string().max(512).optional(),
  enabled: z.boolean().optional(),
  prompt: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  workspaceRoot: defaultPathSchema,
  clawChannelId: z.string().trim().max(MAX_ID_LENGTH).optional(),
  providerId: z.string().trim().max(64).optional(),
  model: z.string().trim().min(1).max(128).optional(),
  reasoningEffort: scheduleReasoningEffortSchema.optional(),
  mode: clawRunModeSchema.optional(),
  schedule: clawTaskSchedulePatchSchema.optional(),
  createdAt: z.string().max(128).optional(),
  updatedAt: z.string().max(128).optional(),
  lastRunAt: z.string().max(128).optional(),
  nextRunAt: z.string().max(128).optional(),
  lastStatus: clawTaskStatusSchema.optional(),
  lastMessage: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  lastThreadId: z.string().max(MAX_ID_LENGTH).optional()
}).strict()

const clawSettingsPatchSchema = z.object({
  enabled: z.boolean().optional(),
  skills: clawSkillPatchSchema.optional(),
  im: clawImPatchSchema.optional(),
  channels: z.array(clawImChannelPatchSchema).max(512).optional(),
  tasks: z.array(clawTaskPatchSchema).max(512).optional()
}).strict()

const scheduleSkillPatchSchema = z.object({
  defaultNames: z.array(trimmedString(128)).max(128).optional(),
  extraDirs: z.array(trimmedString(MAX_PATH_LENGTH)).max(128).optional(),
  disabledDirs: z.array(trimmedString(MAX_PATH_LENGTH)).max(128).optional()
}).strict()

const scheduleInternalPatchSchema = z.object({
  port: z.number().int().min(1024).max(65_535).optional(),
  secret: z.string().max(MAX_BODY_BYTES).optional()
}).strict()

const scheduleKindSchema = z.enum(['manual', 'interval', 'daily', 'at', 'weekly', 'cron'])
const scheduleRuleFields = {
  kind: scheduleKindSchema,
  everyMinutes: z.number().int().min(1).max(10_080).optional(),
  timeOfDay: z.string().max(16).optional(),
  atTime: z.string().max(128).optional(),
  timeZone: z.string().max(255).optional(),
  weekdays: z.array(z.number().int().min(1).max(7)).max(7).optional(),
  expression: z.string().max(512).optional()
}
const scheduleRuleCheck = (allowLegacyDaily: boolean) => (value: { kind?: unknown; timeOfDay?: unknown; timeZone?: unknown; weekdays?: unknown; expression?: unknown }, ctx: z.RefinementCtx) => {
  try { validateScheduleCalendarRule(value, allowLegacyDaily) }
  catch (error) { ctx.addIssue({ code: 'custom', message: error instanceof Error ? error.message : 'Invalid schedule rule.' }) }
}
const scheduledTaskSchedulePatchSchema = z.object(scheduleRuleFields).strict().superRefine(scheduleRuleCheck(false))
const scheduledTaskScheduleExpectedSchema = z.object({ ...scheduleRuleFields,
  everyMinutes: scheduleRuleFields.everyMinutes.unwrap(), timeOfDay: scheduleRuleFields.timeOfDay.unwrap(), atTime: scheduleRuleFields.atTime.unwrap()
}).strict().superRefine(scheduleRuleCheck(true))

export const scheduleTaskEditPatchSchema = z.object({
  title: z.string().max(512).optional(),
  enabled: z.boolean().optional(),
  prompt: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  workspaceRoot: defaultPathSchema,
  clawChannelId: z.string().trim().max(MAX_ID_LENGTH).optional(),
  providerId: z.string().trim().max(64).optional(),
  model: z.string().trim().min(1).max(128).optional(),
  reasoningEffort: scheduleReasoningEffortSchema.optional(),
  mode: clawRunModeSchema.optional(),
  schedule: scheduledTaskSchedulePatchSchema.optional()
}).strict()

const scheduleTaskExpectedSchema = scheduleTaskEditPatchSchema.extend({
  schedule: scheduledTaskScheduleExpectedSchema.optional()
})

export const scheduleTaskHistoryPayloadSchema = z.object({
  taskId: trimmedString(MAX_ID_LENGTH), limit: z.literal(20), before: trimmedString(MAX_ID_LENGTH).optional()
}).strict()

export const scheduleTaskCreatePayloadSchema = scheduleTaskEditPatchSchema.extend({
  title: z.string().trim().min(1).max(512),
  prompt: z.string().max(MAX_CHANNEL_TEXT_LENGTH),
  schedule: scheduledTaskSchedulePatchSchema
})

export const scheduleTaskUpdatePayloadSchema = z.object({
  taskId: trimmedString(MAX_ID_LENGTH),
  patch: scheduleTaskEditPatchSchema,
  expected: scheduleTaskExpectedSchema
}).strict().superRefine((request, ctx) => {
  for (const field of scheduleTaskExpectedFields(request.patch)) {
    if (!Object.prototype.hasOwnProperty.call(request.expected, field) || request.expected[field] === undefined) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['expected', field], message: 'Expected current editable value is required.' })
    }
  }
})

export const scheduleTaskDeletePayloadSchema = z.object({
  taskId: trimmedString(MAX_ID_LENGTH),
  expected: scheduleTaskExpectedSchema.required()
}).strict()

const scheduleSettingsPatchSchema = z.object({
  enabled: z.boolean().optional(),
  defaultWorkspaceRoot: defaultPathSchema,
  providerId: z.string().trim().max(64).optional(),
  model: z.union([z.enum(SCHEDULE_MODEL_IDS), trimmedString(128)]).optional(),
  mode: clawRunModeSchema.optional(),
  promptPrefix: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  skills: scheduleSkillPatchSchema.optional(),
  keepAwake: z.boolean().optional(),
  internal: scheduleInternalPatchSchema.optional()
}).strict()

function stripLegacySettingsPatchKeys(payload: unknown): unknown {
  if (typeof payload !== 'object' || payload === null || Array.isArray(payload)) return payload
  const source = payload as Record<string, unknown>
  const next: Record<string, unknown> = { ...source }

  delete next.agent
  delete next.agentProvider
  delete next.agents
  delete next.autoPlan
  delete next.auto_plan
  delete next.deepseek
  delete next.reasonix
  delete next.quickChat

  return next
}

const settingsPatchObjectSchema = z.object({
  version: z.literal(1).optional(),
  locale: localeSchema.optional(),
  theme: themeSchema.optional(),
  accentColor: z.enum(APP_ACCENT_COLORS).optional(),
  uiFontScale: uiFontScaleSchema.optional(),
  motionPreference: motionPreferenceSchema.optional(),
  performanceUsageMode: z.enum(['compact', 'detailed']).optional(),
  cursorSpotlight: z.boolean().optional(),
  provider: modelProviderPatchSchema.optional(),
  runtime: analytixRuntimePatchSchema.optional(),
  workspaceRoot: defaultPathSchema,
  log: logPatchSchema.optional(),
  notifications: notificationsPatchSchema.optional(),
  appBehavior: appBehaviorPatchSchema.optional(),
  keyboardShortcuts: keyboardShortcutsPatchSchema.optional(),
  write: writeSettingsPatchSchema.optional(),
  claw: clawSettingsPatchSchema.optional(),
  schedule: scheduleSettingsPatchSchema.optional(),
  guiUpdate: z.object({
    channel: z.enum(GUI_UPDATE_CHANNELS).optional()
  }).strict().optional(),
  codePromptPrefix: z.string().max(MAX_CHANNEL_TEXT_LENGTH).optional(),
  disabledSkillIds: z.array(trimmedString(128)).max(512).optional()
}).strict()

export const settingsPatchSchema = z.preprocess(stripLegacySettingsPatchKeys, settingsPatchObjectSchema)

export const skillSaveFilePayloadSchema = z
  .object({
    rootPath: trimmedString(MAX_PATH_LENGTH),
    skillName: trimmedString(128),
    content: z.string().max(MAX_SKILL_FILE_BYTES)
  })
  .strict()

export const skillDeletePayloadSchema = z
  .object({
    rootPath: trimmedString(MAX_PATH_LENGTH),
    skillName: trimmedString(128)
  })
  .strict()

export const skillListPayloadSchema = z
  .object({
    workspaceRoot: z.string().trim().max(MAX_PATH_LENGTH).optional()
  })
  .strict()

export const rootPathSchema = trimmedString(MAX_PATH_LENGTH)
export const deepseekConfigContentSchema = z.string().max(MAX_CONFIG_FILE_BYTES)

export const workspaceRootSchema = trimmedString(MAX_PATH_LENGTH)
export const gitBranchPayloadSchema = z
  .object({
    workspaceRoot: workspaceRootSchema,
    branch: trimmedString(MAX_BRANCH_LENGTH)
  })
  .strict()
export const gitCheckpointCreatePayloadSchema = z
  .object({
    workspaceRoot: workspaceRootSchema,
    threadId: trimmedString(MAX_ID_LENGTH),
    timeoutMs: z.number().int().min(100).max(30_000).optional()
  })
  .strict()
export const gitCheckpointRestorePayloadSchema = z
  .object({
    checkpointId: trimmedString(MAX_ID_LENGTH),
    allowPartialRestore: z.boolean().optional()
  })
  .strict()

export const worktreeOptionalRootSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH),
  poolIndex: z.number().int().min(0).max(2),
  taskId: trimmedString(MAX_BRANCH_LENGTH),
  force: z.boolean().optional(),
  worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreePoolSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH),
  worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreePoolIndexSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH),
  poolIndex: z.number().int().min(0).max(2),
  worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreeMergeSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH),
  poolIndex: z.number().int().min(0).max(2),
  commitMessage: optionalTrimmedString(4_000),
  worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreePathSchema = z.object({
  worktreePath: trimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreeProjectPathSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH)
}).strict()

export const worktreeContinueMergeSchema = z.object({
  projectPath: trimmedString(MAX_PATH_LENGTH),
  message: optionalTrimmedString(4_000)
}).strict()

export const worktreeCommitSchema = z.object({
  worktreePath: trimmedString(MAX_PATH_LENGTH),
  message: trimmedString(4_000)
}).strict()

const threadHandoffDirectionSchema = z.enum(['to-worktree', 'to-local', 'to-host-worktree'])

export const threadHandoffStartPayloadSchema: z.ZodType<ThreadHandoffRequest> = z
  .discriminatedUnion('direction', [
    z.object({
      direction: z.literal('to-worktree'),
      sourceThreadId: trimmedString(MAX_ID_LENGTH),
      sourceWorkspace: trimmedString(MAX_PATH_LENGTH),
      sourceBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      localBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      worktreeBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
    }).strict(),
    z.object({
      direction: z.literal('to-local'),
      sourceThreadId: trimmedString(MAX_ID_LENGTH),
      sourceWorkspace: trimmedString(MAX_PATH_LENGTH),
      localWorkspace: trimmedString(MAX_PATH_LENGTH),
      projectPath: trimmedString(MAX_PATH_LENGTH),
      poolIndex: z.number().int().min(0).max(2).optional(),
      sourceBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      localBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      worktreeRoot: optionalTrimmedString(MAX_PATH_LENGTH)
    }).strict(),
    z.object({
      direction: z.literal('to-host-worktree'),
      sourceThreadId: trimmedString(MAX_ID_LENGTH),
      sourceWorkspace: trimmedString(MAX_PATH_LENGTH),
      destinationHostId: trimmedString(MAX_ID_LENGTH),
      destinationLabel: trimmedString(200),
      destinationWorkspaceRoot: trimmedString(MAX_PATH_LENGTH),
      sourceBranch: optionalTrimmedString(MAX_BRANCH_LENGTH),
      worktreeBranch: optionalTrimmedString(MAX_BRANCH_LENGTH)
    }).strict()
  ])

export const threadHandoffOperationIdPayloadSchema = z.object({
  operationId: trimmedString(MAX_ID_LENGTH)
}).strict()

export const threadHandoffGetPayloadSchema = z.object({
  operationId: optionalTrimmedString(MAX_ID_LENGTH)
}).strict()

export const threadHandoffCompleteSwitchPayloadSchema = z.object({
  operationId: trimmedString(MAX_ID_LENGTH),
  targetThreadId: optionalTrimmedString(MAX_ID_LENGTH)
}).strict()

export const threadHandoffFailSwitchPayloadSchema = z.object({
  operationId: trimmedString(MAX_ID_LENGTH),
  message: trimmedString(8_000)
}).strict()

export const threadHandoffDirectionPayloadSchema = z.object({
  direction: threadHandoffDirectionSchema
}).strict()

export const openEditorPathPayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: optionalTrimmedString(MAX_PATH_LENGTH),
    editorId: optionalTrimmedString(MAX_EDITOR_ID_LENGTH),
    line: z.number().int().positive().max(1_000_000).optional(),
    column: z.number().int().positive().max(1_000_000).optional()
  })
  .strict()

export const workspaceFileTargetPayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: optionalTrimmedString(MAX_PATH_LENGTH),
    line: z.number().int().positive().max(1_000_000).optional(),
    column: z.number().int().positive().max(1_000_000).optional()
  })
  .strict()

// Protected reads have identities only; the ordinary path request is unchanged.
export const workspaceProtectedFileReadPayloadSchema = z.object({ protectedRead: toolResultLocalEffectRequestSchemaV1 }).strict()
export const workspaceProtectedPresentFileReadPayloadSchema = z.object({ protectedPresent: toolResultLocalPresentFileRequestSchemaV1 }).strict()
export const workspaceFileReadPayloadSchema = z.union([workspaceProtectedFileReadPayloadSchema, workspaceProtectedPresentFileReadPayloadSchema, workspaceFileTargetPayloadSchema])
export const workspacePresentMediaReadPayloadSchema = z.union([workspaceProtectedPresentFileReadPayloadSchema, workspaceFileTargetPayloadSchema])

export const localPdfTextTargetPayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const workspaceDirectoryTargetPayloadSchema = z
  .object({
    path: optionalTrimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const workspaceFileWritePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: optionalTrimmedString(MAX_PATH_LENGTH),
    content: z.string().max(MAX_BODY_BYTES)
  })
  .strict()

export const workspaceFileSaveAsPayloadSchema = z
  .object({
    suggestedName: optionalTrimmedString(255),
    sourcePath: optionalTrimmedString(MAX_PATH_LENGTH),
    workspaceRoot: optionalTrimmedString(MAX_PATH_LENGTH),
    dataBase64: z.string().max(MAX_SAVE_FILE_BASE64_BYTES).optional(),
    mimeType: optionalTrimmedString(255)
  })
  .strict()
  .refine((payload) => Boolean(payload.sourcePath || payload.dataBase64), {
    message: 'Either sourcePath or dataBase64 is required.'
  })

export const workspaceFileCreatePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH),
    content: z.string().max(MAX_BODY_BYTES).optional()
  })
  .strict()

export const workspaceDirectoryCreatePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const workspaceClipboardImageSavePayloadSchema = z
  .object({
    workspaceRoot: trimmedString(MAX_PATH_LENGTH),
    currentFilePath: trimmedString(MAX_PATH_LENGTH),
    imageDirectory: optionalTrimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const workspaceEntryRenamePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH),
    newName: trimmedString(255)
  })
  .strict()

export const workspaceEntryDeletePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const workspaceFileWatchPayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const writeRetrievalPayloadSchema = z
  .object({
    threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/).optional(),
    workspaceRoot: defaultPathSchema,
    currentFilePath: defaultPathSchema,
    query: z.string().trim().min(1).max(MAX_CHANNEL_TEXT_LENGTH),
    maxSnippets: z.number().int().min(1).max(8).optional(),
    includeCurrentFile: z.boolean().optional()
  })
  .strict()

export const writeExportPayloadSchema = objectExportBindingSchema.extend({
  format: z.enum(WRITE_EXPORT_FORMATS),
  typography: writeTypographyPatchSchema.optional()
}).strict()

export const writeRichClipboardPayloadSchema = objectExportBindingSchema

const writeInlineEditRecentEditSchema = z
  .object({
    source: z.enum(['user', 'inline-edit']),
    ageMs: z.number().int().min(0).max(24 * 60 * 60 * 1_000),
    filePath: optionalTrimmedString(MAX_PATH_LENGTH),
    from: z.number().int().min(0).max(MAX_BODY_BYTES),
    to: z.number().int().min(0).max(MAX_BODY_BYTES),
    deletedText: z.string().max(8_000),
    insertedText: z.string().max(8_000),
    beforeContext: z.string().max(4_000),
    afterContext: z.string().max(4_000),
    instruction: z.string().trim().min(1).max(10_000).optional(),
    scopeKind: z.enum(['selection', 'paragraph']).optional()
  })
  .strict()
  .refine((edit) => edit.to >= edit.from, {
    message: 'Recent edit end must be greater than or equal to start.'
  })

const writeInlineCompletionEditCandidateSchema = z
  .object({
    kind: z.enum(['selection', 'paragraph']),
    from: z.number().int().min(0).max(MAX_BODY_BYTES),
    to: z.number().int().min(0).max(MAX_BODY_BYTES),
    startLine: z.number().int().positive().max(1_000_000),
    startColumn: z.number().int().positive().max(1_000_000),
    endLine: z.number().int().positive().max(1_000_000),
    endColumn: z.number().int().positive().max(1_000_000),
    original: z.string().max(MAX_EDITOR_COMPLETION_TEXT),
    selectedText: z.string().max(50_000).optional()
  })
  .strict()
  .refine((scope) => scope.to >= scope.from, {
    message: 'Completion edit candidate end must be greater than or equal to start.'
  })

export const writeInlineCompletionPayloadSchema = z
  .object({
    threadId: z.string().regex(/^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/).optional(),
    requestId: z.string().uuid().optional(),
    document: inlineCompletionDocumentSchema.optional(),
    prefix: z.string().max(MAX_EDITOR_COMPLETION_TEXT),
    suffix: z.string().max(MAX_EDITOR_COMPLETION_TEXT),
    mode: z.enum(['short', 'long', 'edit']).optional(),
    workspaceRoot: optionalTrimmedString(MAX_PATH_LENGTH),
    currentFilePath: optionalTrimmedString(MAX_PATH_LENGTH),
    cursor: z
      .object({
        line: z.number().int().positive().max(1_000_000),
        column: z.number().int().min(0).max(1_000_000)
      })
      .strict(),
    context: z
      .object({
        language: trimmedString(64),
        currentLinePrefix: z.string().max(20_000),
        currentLineSuffix: z.string().max(20_000),
        previousLine: z.string().max(20_000),
        previousNonEmptyLine: z.string().max(20_000),
        nextLine: z.string().max(20_000),
        indentation: z.string().max(2_000),
        signals: z
          .object({
            list: z.boolean(),
            quote: z.boolean(),
            heading: z.boolean(),
            table: z.boolean(),
            atLineEnd: z.boolean(),
            endsWithSentencePunctuation: z.boolean(),
            previousLineEndsWithSentencePunctuation: z.boolean(),
            prefersNewLineCompletion: z.boolean(),
            paragraphBreakOpportunity: z.boolean()
          })
          .strict()
      })
      .strict(),
    policy: z
      .object({
        name: trimmedString(128),
        instruction: z.string().max(50_000),
        acceptanceCriteria: z.array(z.string().max(5_000)).max(12),
        rejectionCriteria: z.array(z.string().max(5_000)).max(12)
      })
      .strict(),
    preview: z
      .object({
        local: z.string().max(5_000),
        documentTail: z.string().max(20_000)
      })
      .strict(),
    editCandidate: writeInlineCompletionEditCandidateSchema.optional(),
    recentEdits: z.array(writeInlineEditRecentEditSchema).max(12).optional(),
    model: optionalTrimmedString(128)
  })
  .strict()

export const writeInlineCompletionCancelSchema = z.object({ requestId: z.string().uuid() }).strict()

export const writeInfographicPayloadSchema = z
  .object({
    text: trimmedString(WRITE_INFOGRAPHIC_MAX_TEXT_CHARS),
    filePath: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH),
    imageDir: optionalTrimmedString(MAX_PATH_LENGTH),
    kind: z.enum(['infographic', 'design']).optional(),
    referenceImagePath: optionalTrimmedString(MAX_PATH_LENGTH)
  })
  .strict()

export const writePrototypeFilePayloadSchema = z
  .object({
    path: trimmedString(MAX_PATH_LENGTH),
    workspaceRoot: trimmedString(MAX_PATH_LENGTH),
    mode: z.enum(['write-prototype', 'workspace-html']).optional()
  })
  .strict()

export const writePrototypeRevokePayloadSchema = z.object({ grantId: z.string().regex(/^[a-f0-9]{48}$/) }).strict()

export const speechTranscribePayloadSchema = z
  .object({
    audioBase64: z.string().min(1).max(SPEECH_TRANSCRIPTION_MAX_BASE64_CHARS),
    mimeType: trimmedString(64),
    durationMs: z.number().int().positive().max(SPEECH_TRANSCRIPTION_MAX_DURATION_MS).optional()
  })
  .strict()

export const shellOpenExternalUrlSchema = trimmedString(MAX_URL_LENGTH).refine(
  isSafeOpenExternalUrl,
  { message: 'Only http, https, and mailto URLs are allowed.' }
)

export const notificationPayloadSchema = z
  .object({
    threadId: optionalTrimmedString(MAX_ID_LENGTH),
    title: trimmedString(MAX_NOTIFICATION_TITLE_LENGTH),
    body: trimmedString(MAX_NOTIFICATION_BODY_LENGTH)
  })
  .strict()

export const guiUpdateChannelSchema = z.enum(GUI_UPDATE_CHANNELS).optional()

export const desktopCommandSchema = z.enum(DESKTOP_COMMANDS)

export const computerUsePermissionKindSchema = z.enum(['accessibility', 'screenRecording'])

export const chromeBrowserUseExtensionPageTargetSchema = z.enum(['webstore', 'settings'])

const RENDERER_LOG_EVENT_CODE_BY_CATEGORY: Readonly<Record<string, string>> = {
  'sse-event-active': 'renderer_sse_event_active',
  'sse-event-aborted': 'renderer_sse_event_aborted',
  'sse-event-closed': 'renderer_sse_event_closed',
  'sse-ack_false-active': 'renderer_sse_ack_false_active',
  'sse-ack_false-aborted': 'renderer_sse_ack_false_aborted',
  'sse-ack_false-closed': 'renderer_sse_ack_false_closed',
  'sse-ack_transport-active': 'renderer_sse_ack_transport_active',
  'sse-ack_transport-aborted': 'renderer_sse_ack_transport_aborted',
  'sse-ack_transport-closed': 'renderer_sse_ack_transport_closed',
  approval: 'renderer_approval_failed',
  'create-thread': 'renderer_create_thread_failed',
  'git-checkpoint': 'renderer_git_checkpoint_failed',
  'gui-update': 'renderer_gui_update_failed',
  interrupt: 'renderer_interrupt_failed',
  notification: 'renderer_notification_failed',
  renderer: 'renderer_uncaught_error',
  'send-message': 'renderer_send_message_failed',
  'user-input': 'renderer_user_input_failed'
}

export const logErrorPayloadSchema = z
  .object({
    category: trimmedString(128),
    message: trimmedString(2_000),
    detail: z.unknown().optional()
  })
  .strict()
  .transform((payload) => {
    if (payload.category.startsWith('sse-') &&
        Object.prototype.hasOwnProperty.call(RENDERER_LOG_EVENT_CODE_BY_CATEGORY, payload.category)) {
      return { code: RENDERER_LOG_EVENT_CODE_BY_CATEGORY[payload.category] }
    }
    const inputDetail = payload.detail && typeof payload.detail === 'object' && !Array.isArray(payload.detail)
      ? payload.detail as Record<string, unknown>
      : {}
    const retryable = typeof inputDetail.retryable === 'boolean'
      ? inputDetail.retryable
      : undefined
    const resultCount = typeof inputDetail.resultCount === 'number' &&
      Number.isSafeInteger(inputDetail.resultCount) && inputDetail.resultCount >= 0
      ? inputDetail.resultCount
      : undefined
    return {
      code: RENDERER_LOG_EVENT_CODE_BY_CATEGORY[payload.category] ?? 'renderer_diagnostic_failed',
      ...(retryable === undefined ? {} : { retryable }),
      ...(resultCount === undefined ? {} : { resultCount })
    }
  })

export const threadTraceEventPayloadSchema = z
  .object({
    name: z.enum(THREAD_TRACE_EVENT_NAMES),
    timestamp: z.number().int().nonnegative(),
    threadId: optionalTrimmedString(MAX_ID_LENGTH),
    turnId: optionalTrimmedString(MAX_ID_LENGTH),
    data: z
      .record(
        z.string().trim().min(1).max(64),
        z.union([z.number().finite(), z.boolean(), z.null()])
      )
      .optional()
  })
  .strict()

export const clawMirrorPayloadSchema = z
  .object({
    threadId: trimmedString(MAX_ID_LENGTH),
    text: z.string().trim().min(1).max(MAX_CHANNEL_TEXT_LENGTH),
    direction: z.enum(['user', 'assistant'])
  })
  .strict()

export const clawTaskFromTextPayloadSchema = z
  .object({
    text: z.string().trim().min(1).max(MAX_CHANNEL_TEXT_LENGTH),
    channelId: z.string().trim().min(1).max(MAX_ID_LENGTH).nullable().optional(),
    providerId: z.string().trim().max(64).nullable().optional(),
    modelHint: z.string().trim().min(1).max(128).nullable().optional(),
    reasoningEffort: scheduleReasoningEffortSchema.nullable().optional(),
    mode: z.enum(['agent', 'plan']).nullable().optional()
  })
  .strict()

export const scheduleTaskFromTextPayloadSchema = z
  .object({
    text: z.string().trim().min(1).max(MAX_CHANNEL_TEXT_LENGTH),
    workspaceRoot: defaultPathSchema,
    clawChannelId: z.string().trim().min(1).max(MAX_ID_LENGTH).nullable().optional(),
    providerId: z.string().trim().max(64).nullable().optional(),
    modelHint: z.string().trim().min(1).max(128).nullable().optional(),
    reasoningEffort: scheduleReasoningEffortSchema.nullable().optional(),
    mode: z.enum(['agent', 'plan']).nullable().optional()
  })
  .strict()

export const clawImInstallPollPayloadSchema = z
  .object({
    provider: z.enum(['feishu', 'weixin']),
    deviceCode: trimmedString(MAX_DEVICE_CODE_LENGTH)
  })
  .strict()

export const clawImTelegramTokenPayloadSchema = z
  .object({
    botToken: z.string().trim().min(1),
    allowedChatIds: z.string().trim().optional().default('')
  })
  .strict()

export const sseStartPayloadSchema = z
  .object({
    threadId: trimmedString(MAX_ID_LENGTH),
    sinceSeq: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER),
    streamId: optionalTrimmedString(MAX_ID_LENGTH),
    replayThroughSeq: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER).optional()
  })
  .strict()
  .refine(value => value.replayThroughSeq === undefined || value.replayThroughSeq >= value.sinceSeq, {
    message: 'Replay boundary precedes its cursor.'
  })

export const streamIdSchema = trimmedString(MAX_ID_LENGTH)

const ordinarySseAckPayloadSchema = z.object({
  streamId: streamIdSchema,
  seq: z.number().int().min(0).max(Number.MAX_SAFE_INTEGER)
}).strict()

const acceptedFinalSseAckPayloadSchema = z.object({
  streamId: streamIdSchema,
  seq: z.number().int().positive().max(Number.MAX_SAFE_INTEGER),
  batchId: z.string().regex(/^[a-f0-9]{64}$/),
  threadId: trimmedString(MAX_ID_LENGTH),
  turnId: trimmedString(MAX_ID_LENGTH),
  publicationCommitId: z.string().regex(/^[a-f0-9]{64}$/)
}).strict()

export const sseAckPayloadSchema = z.union([
  ordinarySseAckPayloadSchema,
  acceptedFinalSseAckPayloadSchema
])

export const uiPluginIdPayloadSchema = z
  .object({
    id: z.string().trim().regex(/^[a-z0-9][a-z0-9-]{1,39}$/)
  })
  .strict()

export const hubAgentMarketplaceSyncPayloadSchema = z
  .object({
    forceRefresh: z.boolean().optional(),
    mode: z.enum(['cache', 'catalog', 'full']).optional()
  })
  .strict()

export const hubAgentPluginMutationPayloadSchema = z
  .object({
    pluginName: trimmedString(MAX_ID_LENGTH),
    version: optionalTrimmedString(MAX_ID_LENGTH),
    upstreamMarketplaceName: optionalTrimmedString(MAX_ID_LENGTH)
  })
  .strict()

export const hubAgentSkillMarkdownPayloadSchema = z
  .object({
    id: optionalTrimmedString(MAX_ID_LENGTH),
    skillName: trimmedString(MAX_ID_LENGTH),
    displayName: optionalTrimmedString(MAX_ID_LENGTH),
    pluginName: optionalTrimmedString(MAX_ID_LENGTH),
    version: optionalTrimmedString(MAX_ID_LENGTH),
    upstreamMarketplaceName: optionalTrimmedString(MAX_ID_LENGTH),
    skillPath: optionalTrimmedString(MAX_PATH_LENGTH),
    sourceKind: z.enum(['runtime-system', 'runtime-user', 'plugin']).optional()
  })
  .strict()

export const terminalSessionIdSchema = trimmedString(TERMINAL_MAX_SESSION_ID_LENGTH)

export const terminalCreatePayloadSchema = z
  .object({
    sessionId: trimmedString(TERMINAL_MAX_SESSION_ID_LENGTH),
    cwd: optionalTrimmedString(TERMINAL_MAX_CWD_LENGTH),
    cols: z.number().int().min(1).max(TERMINAL_MAX_COLS).optional(),
    rows: z.number().int().min(1).max(TERMINAL_MAX_ROWS).optional()
  })
  .strict()

export const terminalWritePayloadSchema = z
  .object({
    sessionId: trimmedString(TERMINAL_MAX_SESSION_ID_LENGTH),
    data: z.string().min(1).max(TERMINAL_MAX_DATA_WRITE_BYTES)
  })
  .strict()

export const terminalResizePayloadSchema = z
  .object({
    sessionId: trimmedString(TERMINAL_MAX_SESSION_ID_LENGTH),
    cols: z.number().int().min(1).max(TERMINAL_MAX_COLS).default(TERMINAL_DEFAULT_COLS),
    rows: z.number().int().min(1).max(TERMINAL_MAX_ROWS).default(TERMINAL_DEFAULT_ROWS)
  })
  .strict()

const backgroundTaskStatusSchema = z.enum([
  'registered',
  'starting',
  'running',
  'completed',
  'failed',
  'stopped',
  'missing',
  'killed'
])

const backgroundTaskSourceSchema = z.enum(['summary-command', 'manual', 'restored-process'])

export const backgroundTaskThreadPayloadSchema = z.object({
  threadId: trimmedString(MAX_ID_LENGTH)
}).strict()

export const backgroundTaskActionPayloadSchema = z.object({
  threadId: trimmedString(MAX_ID_LENGTH),
  taskId: trimmedString(MAX_ID_LENGTH)
}).strict()

export const backgroundTaskOutputPayloadSchema = backgroundTaskActionPayloadSchema.extend({
  offset: z.number().int().nonnegative().optional(),
  limit: z.number().int().positive().max(2_000_000).optional()
}).strict()

export const backgroundTaskRegisterPayloadSchema = z.object({
  record: z.object({
    id: trimmedString(MAX_ID_LENGTH),
    threadId: trimmedString(MAX_ID_LENGTH),
    turnId: optionalTrimmedString(MAX_ID_LENGTH),
    itemId: optionalTrimmedString(MAX_ID_LENGTH),
    pid: z.number().int().positive().nullable().optional(),
    processId: optionalTrimmedString(MAX_ID_LENGTH),
    attemptId: optionalTrimmedString(MAX_ID_LENGTH),
    status: backgroundTaskStatusSchema,
    source: backgroundTaskSourceSchema,
    outputBytes: z.number().int().nonnegative().optional(),
    startedAt: z.string().trim().max(128).optional(),
    updatedAt: z.string().trim().max(128).optional(),
    finishedAt: z.string().trim().max(128).optional()
  }).strict()
}).strict()
