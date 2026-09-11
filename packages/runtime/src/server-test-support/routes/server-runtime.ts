import type { ThreadService } from '../../services-test-support/thread-service.js'
import type { TurnService } from '../../services-test-support/turn-service.js'
import type { UsageService } from '../../services-test-support/usage-service.js'
import type { ReviewService } from '../../services-test-support/review-service.js'
import type { CheckpointRewindService } from '../../services-test-support/checkpoint-rewind-service.js'
import type { EventBus } from '../../ports/event-bus.js'
import type { SessionStore } from '../../ports/session-store.js'
import type { ApprovalGate } from '../../ports/approval-gate.js'
import type { UserInputGate } from '../../ports/user-input-gate.js'
import type { WorkspaceInspector } from '../../ports/workspace-inspector.js'
import type { ToolHost } from '../../ports/tool-host.js'
import type { RuntimeEventRecorder } from '../../services-test-support/runtime-event-recorder.js'
import type { ThreadTitleService } from '../../services-test-support/thread-title-service.js'
import type { LlmDebugRecorder } from '../../services-test-support/llm-debug-recorder.js'
import type { RuntimeInfoResponse } from '../../contracts/runtime-info.js'
import type { RuntimeToolsResponse } from '../../contracts/runtime-tools.js'
import type { SkillRuntimeDiagnostics } from '../../skills/skill-runtime.js'
import type { AttachmentStore } from '../../attachments/attachment-store.js'
import type { MemoryStore } from '../../memory/memory-store.js'
import type { ReviewTarget } from '../../contracts/review.js'
import type { RemoteEntryControlPort } from '../../ports/remote-entry-control.js'
import type { DurableTaskJobManager } from '../../delegation-test-support/job-manager.js'
import type { DelegationRuntime } from '../../delegation-test-support/delegation-runtime.js'

/**
 * Dependencies that the HTTP router needs. Bundled into a single
 * type so callers can compose the runtime from the in-memory or
 * file-backed adapters without leaking concrete types into routes.
 */
export type ServerRuntime = {
  threadService: ThreadService
  threadTitleService?: ThreadTitleService
  turnService: TurnService
  usageService: UsageService
  reviewService?: ReviewService
  checkpointRewindService: CheckpointRewindService
  eventBus: EventBus
  sessionStore: SessionStore
  events: RuntimeEventRecorder
  /** Optional troubleshooting buffer of the most recent LLM rounds (in-memory). */
  llmDebug?: LlmDebugRecorder
  approvalGate: ApprovalGate
  userInputGate: UserInputGate
  workspaceInspector: WorkspaceInspector
  toolHost?: ToolHost
  attachmentStore?: AttachmentStore
  memoryStore?: MemoryStore
  delegationRuntime?: DelegationRuntime
  taskJobs?: DurableTaskJobManager
  remoteEntryControl: RemoteEntryControlPort
  runTurn(threadId: string, turnId: string): Promise<'completed' | 'failed' | 'aborted'> | void
  resumeInterruptedGoals?(threadIds: readonly string[]): Promise<number> | number
  runReview?(input: {
    threadId: string
    turnId: string
    reviewItemId: string
    target: ReviewTarget
    model?: string
    providerId?: string
  }): Promise<'completed' | 'failed' | 'aborted'> | void
  runtimeToken: string
  insecure: boolean
  allocateSeq: (threadId: string) => number
  nowIso: () => string
  info(): RuntimeInfoResponse
  toolDiagnostics?(): RuntimeToolsResponse | Promise<RuntimeToolsResponse>
  skills?(): SkillRuntimeDiagnostics | Promise<SkillRuntimeDiagnostics>
  shutdown?(): Promise<void>
}
