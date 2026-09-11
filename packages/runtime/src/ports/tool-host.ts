import type { ApprovalPolicy, SandboxMode } from '../contracts/policy.js'
import type { ApprovalRequest } from '../domain/approval.js'
import type { TurnItem } from '../contracts/items.js'
import type { ModelToolSnipHint } from './model-client.js'
import type { ModelCapabilityMetadata } from '../contracts/capabilities.js'
import type { ModelExecutionRef } from '../contracts/model-execution-ref.js'
import type { InternalToolResultItem } from '../domain/item.js'
import type {
  UserInputRequest,
  UserInputResolution
} from './user-input-gate.js'

export type ToolProviderKind =
  | 'built-in'
  | 'mcp'
  | 'web'
  | 'skill'
  | 'memory'
  | 'gui'
  | 'delegation'
  | 'image'
  | 'audio'
  | 'video'

export type ToolProviderPolicy = {
  id: string
  kind: ToolProviderKind
  enabled: boolean
  available: boolean
  reason?: string
}

export type ToolKind = 'tool_call' | 'command_execution' | 'file_change' | 'subagent'

export type ToolPolicy = 'auto' | 'on-request' | 'suggest' | 'never' | 'untrusted'

export type ToolContractEntry = {
  name: string
  description: string
  inputSchema: Record<string, unknown>
  outputSchema?: Record<string, unknown>
  toolKind?: ToolKind
  providerId: string
  providerKind: ToolProviderKind
  toolPolicy: ToolPolicy
  providerEnabled: boolean
  providerAvailable: boolean
  providerReason?: string
}

/**
 * Optional GUI plan context advertised by the renderer when starting
 * draft or refine plan turns. When present, Analytix exposes the
 * `create_plan` tool to the model and gates the corresponding tool
 * adapter to this exact path/workspace. The struct is stable across
 * reconnects so replays reproduce the same gating.
 */
export type GuiPlanContext = {
  /** Operation that triggered the plan tool exposure. */
  operation: 'draft' | 'refine'
  /** Workspace root the plan must be written under. */
  workspaceRoot: string
  /** Reserved plan relative path the tool is allowed to write to. */
  relativePath: string
  /** Stable plan id; matches `GuiPlanArtifact.id` on the GUI side. */
  planId: string
  /** Original user request that originated the plan turn. */
  sourceRequest?: string
  /** Display title for the plan. */
  title?: string
  /** Optional turn id for debugging. */
  turnId?: string
}

export type ToolHostContext = {
  threadId: string
  turnId: string
  /** Current tool call id when executing a tool. Omitted while listing tools. */
  toolCallId?: string
  workspace: string
  /**
   * Thread mode advertised by the GUI. Analytix restricts plan tools
   * to `plan` threads plus `planDraft`/`planRefine` turn kinds. The
   * field is optional for backward compatibility with older call sites.
   */
  threadMode?: 'agent' | 'plan'
  /** Optional GUI plan context (see above). */
  guiPlan?: GuiPlanContext
  /** Active model capability metadata used by capability-aware providers. */
  model?: ModelCapabilityMetadata
  /** Provider/model execution selected for this turn; delegated children inherit it unless explicitly overridden. */
  modelExecution?: ModelExecutionRef
  /** Skill ids activated for this turn, if the Skill runtime is enabled. */
  activeSkillIds?: readonly string[]
  /** Optional memory recall/mutation policy for this turn. */
  memoryPolicy?: {
    enabled: boolean
    scopes?: readonly string[]
  }
  /** Optional delegation policy for this turn. */
  delegationPolicy?: {
    enabled: boolean
    maxParallel?: number
    maxChildRuns?: number
  }
  /** Runtime-only loop budget metadata for child/headless tool providers. */
  runtimeStepLimits?: {
    currentMaxModelSteps: number
  }
  /** Optional provider allow-list. When set, other providers are not advertised or executed. */
  allowedProviderIds?: readonly string[]
  /** Optional tool-name allow-list. When set, other tools are not advertised or executed. */
  allowedToolNames?: readonly string[]
  approvalPolicy: ApprovalPolicy
  /** Filesystem/command sandbox selected for this turn. Defaults at execution time for old callers. */
  sandboxMode?: SandboxMode
  abortSignal: AbortSignal
  /** Resolves a pending approval with the user's decision. */
  awaitApproval: (approval: ApprovalRequest) => Promise<'allow' | 'deny'>
  /** Resolves structured GUI input requested by a tool call. */
  awaitUserInput?: (
    input: Omit<UserInputRequest, 'threadId' | 'turnId'>
  ) => Promise<UserInputResolution>
}

export type ToolCallLike = {
  callId: string
  toolName: string
  providerId?: string
  toolKind?: ToolKind
  arguments: Record<string, unknown>
}

export type ToolExecutionUpdate = {
  output: unknown
  isError?: boolean
}

export type ToolHostResult = {
  item: InternalToolResultItem | Extract<TurnItem, { kind: 'approval' }>
  /** True if the call was decided by an approval. */
  approved: boolean
}

/**
 * Port for executing tool calls. The local tool host uses approval
 * boundaries and abort-signal cancellation; a remote host can fan out
 * to a sandboxed environment. The loop and tests only see the port.
 */
export interface ToolHost {
  readonly id: string
  /**
   * List tools available for the current turn. Tool hosts MAY scope
   * the list by mode/GUI plan context (e.g. only expose `create_plan`
   * during plan turns) so the model is not tempted to call gated
   * tools in normal agent turns.
   */
  listTools(context?: ToolHostContext): Promise<{
    name: string
    description: string
    inputSchema: Record<string, unknown>
    toolKind?: ToolKind
    snipHint?: ModelToolSnipHint
    providerId?: string
    providerKind?: ToolProviderKind
  }[]>
  /**
   * Provider-visible contract snapshot used by diagnostics and drift checks.
   * Runtime-only metadata such as snip hints must not be included here.
   */
  contractEntries?(context?: ToolHostContext): ToolContractEntry[] | Promise<ToolContractEntry[]>
  execute(
    call: ToolCallLike,
    context: ToolHostContext,
    onUpdate?: (item: InternalToolResultItem) => Promise<void> | void
  ): Promise<ToolHostResult>
  /** Optional runtime hygiene hook used when compaction/discard invalidates read context. */
  clearReadTracker?(threadId?: string): void
}
