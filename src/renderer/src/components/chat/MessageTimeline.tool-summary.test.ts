import { beforeEach, describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import type { ChatBlock, NormalizedThread, ToolBlock } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { appendActiveStreamDeltas, clearActiveStream, resetActiveStream } from '../../thread/streaming/active-stream-store'
import {
  MessageTimeline,
  goalTimelinePaddingClass,
  liveTurnProgressClass,
  shouldShowReturnToBottomButton,
  summarizeToolBlock
} from './MessageTimeline'
import { GeneratedFilesPanel, MessageBubble } from './message-timeline-bubbles'
import { ProcessSectionRow, groupProcessSections } from './message-timeline-process'
import { projectSubagentActionResponse, SubagentCallCard } from './SubagentCallCard'
import { resolveInjectedMemoryTooltipLines } from './injected-memory-lookup'

const labels: Record<string, string> = {
  toolActionCommand: 'Ran command',
  toolBuiltinRead: 'Read',
  toolBuiltinWrite: 'Write',
  toolBuiltinEdit: 'Edit',
  toolBuiltinGrep: 'Search',
  toolBuiltinFind: 'Find',
  toolBuiltinLs: 'List',
  toolBuiltinBash: 'Bash',
  toolChildAgent: 'Child agent',
  toolChildBackground: 'background',
  toolChildParallel: 'parallel',
  toolChildTools: 'tools',
  toolChildDuration: 'duration',
  toolChildQueued: 'queued',
  toolChildTokens: 'tokens',
  toolChildCache: 'cache',
  toolChildLabel: 'label',
  toolChildStatus: 'status',
  toolChildId: 'child',
  toolChildJob: 'job',
  toolChildRun: 'run',
  toolChildThread: 'thread',
  toolChildTurn: 'turn',
  toolChildParent: 'parent',
  toolChildProfile: 'profile',
  toolChildProfileMode: 'mode',
  toolChildProfileDescription: 'profile note',
  toolChildEffort: 'effort',
  toolChildToolPolicy: 'tools',
  toolChildReturnFormat: 'return',
  toolChildTokenBudget: 'token budget',
  toolChildTimeBudget: 'time budget',
  toolChildBudgetExceeded: 'budget exceeded',
  toolChildEvidence: 'evidence',
  toolChildSeq: 'seq',
  toolChildWorktree: 'worktree',
  toolChildWorktreePath: 'worktree path',
  toolChildBaseCommit: 'base commit',
  toolChildCurrentCommit: 'current commit',
  toolChildChangedFiles: 'changed files',
  toolChildMergeStatus: 'merge status',
  subagentWorktreeIsolated: 'isolated worktree',
  subagentWorktreeChangedFiles: '{{count}} changed files',
  subagentChildTodoCountChip: '{{count}} child todos',
  subagentChildTodoProjectionChip: 'todo projection {{status}}',
  subagentChildTodoSummary: '{{count}} child todos: {{completed}} done, {{inProgress}} active, {{pending}} pending, {{blocked}} blocked, {{canceled}} canceled',
  subagentChildTodoProjectionStatus: 'projection {{status}}',
  subagentChildTodoProjectionItems: '{{count}} projected items',
  subagentChildTodoProjectionEvidence: '{{count}} evidence ids',
  subagentChildTodoProjectionMapped: '{{completed}} completed mapped to parent todos',
  subagentChildTodoProjectionDecision: 'decision {{decision}}: {{accepted}} accepted, {{skipped}} skipped',
  subagentChildTodoAcceptAction: 'Accept child todo projection',
  subagentChildTodoAccepted: 'Child todo projection accepted by parent; parent goal is unchanged',
  subagentChildTodoAcceptFailed: 'Child todo projection accept failed',
  subagentChildTodoRejectAction: 'Reject child todo projection',
  subagentChildTodoProposalHint: 'Child todo projection is a proposal; parent approval is required',
  subagentChildTodoRejected: 'Child todo projection rejected; parent todos were unchanged',
  subagentChildTodoRejectFailed: 'Child todo projection reject failed',
  subagentReviewInspectAction: 'Inspect isolated diff',
  subagentReviewInspectHint: 'Inspect isolated worktree diff without merging',
  subagentReviewRequested: 'Diff review ready: {{count}} changed files',
  subagentReviewRejected: 'Diff review was rejected',
  subagentReviewFailed: 'Diff review failed',
  subagentDecisionHint: 'Parent decision only; no automatic cleanup',
  subagentAcceptAction: 'Accept clean isolated diff',
  subagentAcceptRequested: 'Clean diff accepted into parent workspace',
  subagentAcceptFailed: 'Accept request failed',
  subagentRejectAction: 'Reject isolated result',
  subagentRejectRequested: 'Child result rejected; isolated files retained for audit',
  subagentRejectFailed: 'Reject request failed',
  subagentCleanupAction: 'Clean up isolated worktree',
  subagentCleanupRequested: 'Isolated worktree cleanup recorded',
  subagentCleanupFailed: 'Cleanup request failed',
  subagentConflictReportAction: 'Record conflict report',
  subagentConflictReportRequested: 'Conflict report recorded without touching the parent workspace',
  subagentConflictReportFailed: 'Conflict report failed',
  subagentRepairHint: 'Repair check is dry-run only; Accept repair requires parent approval',
  subagentRepairPatchInputLabel: 'Repair patch',
  subagentRepairPatchPlaceholder: 'Paste a repair patch to dry-run against the parent workspace',
  subagentRepairCheckAction: 'Dry-run repair patch',
  subagentRepairDryRunClean: 'Repair patch dry-run is clean; nothing has been applied yet',
  subagentRepairDryRunConflicted: 'Repair patch still conflicts; nothing was applied',
  subagentRepairCheckFailed: 'Repair dry-run failed',
  subagentRepairAcceptAction: 'Accept clean repair patch',
  subagentRepairAccepted: 'Approved repair patch applied; worktree was not cleaned up',
  subagentRepairAcceptFailed: 'Repair accept failed',
  subagentOutputAction: 'Inspect child output',
  subagentWaitAction: 'Poll child status',
  subagentRecoverAction: 'Recover child run',
  subagentRestartAction: 'Restart child run',
  subagentRecoveryKillAction: 'Kill expired child run',
  subagentRecoveryHint: 'Runtime state needs attention; inspect output, recover, restart, or stop the old run',
  subagentOutputReady: 'Output {{bytes}}',
  subagentWaitRequested: 'Wait result refreshed',
  subagentRecoverRequested: 'Recovery requested',
  subagentRestartRequested: 'Restart requested',
  subagentRestartRequestedWithJob: 'Restarted as {{jobId}}',
  subagentRecoveryActionRejected: 'Request was rejected',
  subagentRecoveryActionFailed: 'Request failed',
  toolMetaYes: 'yes',
  toolJobDiagnostics: 'Job',
  toolJobStatus: 'status',
  toolJobWarningCode: 'warning code',
  toolJobWarning: 'warning',
  toolJobSuggestion: 'suggestion',
  toolJobArtifact: 'artifact',
  toolJobStalled: 'stalled',
  toolJobIdle: 'idle',
  toolJobHeartbeat: 'heartbeat',
  toolJobHeartbeatStatus: 'runtime status',
  toolJobHeartbeatRunning: 'running',
  toolJobHeartbeatStale: 'possibly stalled',
  toolJobHeartbeatLeaseExpired: 'lease expired',
  toolJobHeartbeatOrphaned: 'orphaned',
  toolJobHeartbeatRecovering: 'recovering',
  toolJobHeartbeatRecovered: 'recovered',
  toolJobHeartbeatDeadLettered: 'unrecoverable',
  toolJobLastHeartbeat: 'last heartbeat',
  toolJobLeaseOwner: 'lease owner',
  toolJobLeaseExpires: 'lease expires',
  toolJobLeaseExpired: 'lease expired',
  toolJobStaleAfter: 'stale after',
  toolJobOrphaned: 'orphaned',
  toolJobRecoveryStatus: 'recovery status',
  toolJobRecoveryAttempt: 'recovery attempt',
  toolJobRecoveryReason: 'recovery reason',
  toolJobRecoveryUpdated: 'recovery updated',
  toolJobDeadLetterReason: 'dead-letter reason',
  toolJobOutput: 'output',
  toolJobOutputPreview: 'output preview',
  toolJobNextOffset: 'next',
  toolJobSuggestedTools: 'tools',
  toolJobNotification: 'notification',
  toolJobReason: 'reason',
  toolAutoContinueStatus: 'auto-continue',
  toolAutoContinueTurn: 'continued turn',
  toolAutoContinueReason: 'auto reason',
  toolAutoContinueError: 'auto error',
  toolLateCompletionReason: 'late completion',
  toolDeliveryId: 'delivery',
  toolDeliveryStatus: 'delivery status',
  toolDeliveryItem: 'delivery item',
  toolDeliveryReason: 'delivery reason',
  toolDeliveryError: 'delivery error',
  toolDeliveryAttempt: 'delivery attempts',
  toolDeliveryAt: 'delivered at',
  toolDeliveryDeadLetterAt: 'dead-lettered at',
  openRuntimeDiagnostics: 'Open runtime diagnostics',
  toolParentThread: 'parent thread',
  toolParentTurn: 'parent turn',
  backgroundAutoContinueStarted: 'Background result woke the parent thread',
  backgroundAutoContinueStartedWithTurn: 'Background result woke the parent thread · {{turnId}}',
  backgroundAutoContinueSkipped: 'Background result did not auto-continue',
  backgroundAutoContinueSkippedWithReason: 'Background result did not auto-continue · {{reason}}',
  backgroundAutoContinueFailed: 'Background auto-continue failed',
  backgroundAutoContinueFailedWithReason: 'Background auto-continue failed · {{reason}}',
  backgroundAutoContinueStatus: 'Background auto-continue {{status}}',
  backgroundDeliveryPending: 'Background result delivery pending',
  backgroundDeliveryRetry: 'Background result delivery retrying',
  backgroundDeliveryRetryWithReason: 'Background result delivery retrying · {{reason}}',
  backgroundDeliveryDelivered: 'Background result delivered to the parent thread',
  backgroundDeliverySkipped: 'Background result delivery skipped',
  backgroundDeliverySkippedWithReason: 'Background result delivery skipped · {{reason}}',
  backgroundDeliveryDeadLetter: 'Background result could not be delivered and was moved to diagnostics',
  backgroundDeliveryDeadLetterWithReason: 'Background result could not be delivered and was moved to diagnostics · {{reason}}',
  backgroundDeliveryStatus: 'Background result delivery {{status}}',
  groupChildAgents: 'Ran {{count}} child agents',
  groupChildAgent: 'Ran 1 child agent',
  toolProviderDiagnostics: 'Provider',
  toolProviderStatus: 'status',
  toolProviderEndpointFormat: 'endpoint',
  toolProviderAuth: 'auth',
  toolProviderApiKeyConfigured: 'API key configured',
  toolProviderApiKeyMissing: 'API key missing',
  toolProviderRecovery: 'recovery',
  toolProviderRecoveryExhausted: 'exhausted',
  toolProviderRetry: 'retry',
  toolProviderRetryable: 'retryable'
}

const t = (key: string, opts?: Record<string, unknown>) => {
  const template = labels[key] ?? (key === 'toolActionCommand' ? 'Ran command' : key)
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(opts?.[name] ?? `{{${name}}}`))
}

const activeThread: NormalizedThread = {
  id: 'thr_1',
  title: 'Thread',
  updatedAt: '2026-06-07T00:00:00.000Z',
  model: 'deepseek-chat',
  mode: 'code',
  workspace: '/tmp/project'
}

function toolBlock(overrides: Partial<ToolBlock>): ToolBlock {
  return {
    kind: 'tool',
    id: 'tool_1',
    summary: 'tool',
    status: 'success',
    ...overrides
  }
}

describe('MessageTimeline tool summaries', () => {
  it('summarizes built-in read/write/edit tools with their file path', () => {
    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'read: file',
          meta: { toolName: 'read' },
          filePath: '/tmp/readme.md'
        }),
        t
      )
    ).toBe('Read /tmp/readme.md')

    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'write: file',
          meta: { toolName: 'write' },
          filePath: '/tmp/out.ts'
        }),
        t
      )
    ).toBe('Write /tmp/out.ts')

    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'edit: file',
          meta: { toolName: 'edit' },
          filePath: '/tmp/app.ts'
        }),
        t
      )
    ).toBe('Edit /tmp/app.ts')
  })

  it('summarizes built-in grep/find with pattern context', () => {
    const grep = summarizeToolBlock(
      toolBlock({
        summary: 'grep: search',
        meta: { toolName: 'grep', pattern: 'needle' },
        filePath: '/tmp/src'
      }),
      t
    )
    expect(grep).toBe('Search needle · /tmp/src')

    const find = summarizeToolBlock(
      toolBlock({
        summary: 'find: files',
        meta: { toolName: 'find', pattern: '*.ts' },
        filePath: '/tmp/src'
      }),
      t
    )
    expect(find).toBe('Find *.ts · /tmp/src')
  })

  it('summarizes built-in ls with its path and bash with its command', () => {
    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'ls: list',
          meta: { toolName: 'ls' },
          filePath: '/tmp/project'
        }),
        t
      )
    ).toBe('List /tmp/project')

    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'bash: exec',
          toolKind: 'command_execution',
          meta: { toolName: 'bash', command: 'npm test' }
        }),
        t
      )
    ).toBe('Ran command npm test')
  })

  it('titles subagent tool rows as child-agent work', () => {
    expect(
      summarizeToolBlock(
        toolBlock({
          summary: 'task: profile child',
          toolKind: 'subagent',
          meta: {
            toolName: 'task',
            child: {
              parentThreadId: 'thr_parent',
              parentTurnId: 'turn_parent',
              childId: 'child_1',
              childStatus: 'running'
            }
          }
        }),
        t
      )
    ).toContain('Child agent')
  })
})

describe('MessageTimeline Analytix runtime metadata smoke', () => {
  beforeEach(() => {
    clearActiveStream()
    useChatStore.setState({
      route: 'chat',
      workspaceRoot: '/tmp/project',
      activeThreadId: 'thr_1',
      activeThreadTodos: null,
      threads: [activeThread],
      busy: false,
      currentTurnUserId: null,
      turnStartedAtByUserId: {},
      turnDurationByUserId: {},
      clawChannels: [],
      activeClawChannelId: ''
    })
  })

  it('renders user image attachments as thumbnails instead of attachment chips', () => {
    const block: ChatBlock = {
      kind: 'user',
      id: 'user_1',
      text: '为什么图片完全没有识别啊',
      meta: {
        attachmentIds: ['att_1'],
        attachments: [{
          id: 'att_1',
          name: 'image.png',
          mimeType: 'image/png',
          previewUrl: 'data:image/png;base64,abc'
        }]
      }
    }

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('<img')
    expect(html).toContain('src="data:image/png;base64,abc"')
    expect(html).toContain('为什么图片完全没有识别啊')
    expect(html).not.toContain('Attachments 1')
    expect(html).not.toContain('ds-media-printer-reveal')
  })

  it('renders generated image previews with the printer reveal effect', () => {
    const block: ToolBlock = toolBlock({
      id: 'tool_img',
      summary: 'generate_image',
      meta: {
        generatedFiles: [
          {
            name: 'painting.png',
            mimeType: 'image/png',
            previewUrl: 'data:image/png;base64,paint',
            localFilePath: '/tmp/project/painting.png'
          }
        ]
      }
    })

    const html = renderToStaticMarkup(createElement(GeneratedFilesPanel, { blocks: [block] }))

    expect(html).toContain('<img')
    expect(html).toContain('src="data:image/png;base64,paint"')
    expect(html).toContain('ds-media-printer-reveal')
    expect(html).toContain('painting.png')
  })

  it('renders managed Claw prompts as the user-visible message', () => {
    const block: ChatBlock = {
      kind: 'user',
      id: 'user_claw',
      text: [
        '[Connect Phone managed instructions]',
        '',
        '[Connect Phone agent instructions]',
        '',
        '[Agent name]',
        'analytix',
        '',
        '---',
        '[Current user request]',
        '[Feishu / Lark inbound message]',
        'Chat type: p2p',
        'Sender: user-1',
        '',
        'hi'
      ].join('\n')
    }

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('hi')
    expect(html).not.toContain('Connect Phone managed instructions')
    expect(html).not.toContain('Agent name')
    expect(html).not.toContain('Feishu / Lark inbound message')
  })

  it('renders plugin mentions inline in user bubbles without exposing raw markdown', () => {
    const block: ChatBlock = {
      kind: 'user',
      id: 'user_plugin',
      text: '请使用 @[Product Design](plugin://product-design@openai-curated-remote) 完成'
    }

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('data-user-message-inline-mention="plugin"')
    expect(html).toContain('Product Design')
    expect(html).not.toContain('plugin://product-design')
    expect(html).not.toContain('@[')
  })

  it('renders attachment, Skill, memory, web source, and child-agent chips in bubbles', () => {
    const block: ToolBlock = toolBlock({
      summary: 'web_search: docs',
      meta: {
        attachmentIds: ['att_1'],
        activeSkillIds: ['skill_docs'],
        injectedMemoryIds: ['mem_1'],
        diagnostics: {
          status: 'running',
          heartbeatStatus: 'stale',
          background: true,
          stalled: true,
          idleMs: 45_000,
          heartbeatAt: '2026-06-03T10:00:10.000Z',
          heartbeatAgeMs: 15_000,
          outputBytes: 21,
          artifactPath: '/tmp/project/.analytix/jobs/job_1.log',
          warning: 'task job has not produced progress recently',
          warningCode: 'task_job_stale',
          suggestedTools: ['bash_output', 'wait', 'kill_shell'],
          suggestion: 'Use bash_output(jobId="job_1") to inspect recent output.'
        },
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'child_research',
          childRunId: 'job_1',
          jobId: 'job_1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          childLabel: 'research',
          childStatus: 'running',
          heartbeatStatus: 'stale',
          heartbeatAgeMs: 15_000,
          childSeq: 2,
          childProfile: 'researcher',
          childProfileMode: 'subagent',
          childProfileDescription: 'research profile',
          childToolPolicy: 'readOnly',
          returnFormat: 'evidence',
          tokenBudget: 1024,
          timeBudgetMs: 30_000,
          budgetExceeded: true,
          evidenceBundleStatus: 'not_found',
          evidenceCount: 0,
          toolInvocations: 3,
          durationMs: 1250,
          queuedMs: 50,
          background: true,
          parallelGroupId: 'parallel_1',
          parallelIndex: 2,
          totalTokens: 42,
          cacheHitRate: 0.5
        },
        sources: [
          {
            title: 'Analytix docs',
            url: 'https://example.com/analytix'
          }
        ]
      }
    })

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('Attachments 1')
    expect(html).toContain('Skills 1')
    expect(html).toContain('Memories 1')
    expect(html).toContain('Child agent')
    expect(html).toContain('research')
    expect(html).toContain('running')
    expect(html).toContain('#2')
    expect(html).toContain('3 tools')
    expect(html).toContain('duration 1.3s')
    expect(html).toContain('queued 50ms')
    expect(html).toContain('background')
    expect(html).toContain('parallel 2')
    expect(html).toContain('42 tokens')
    expect(html).toContain('return evidence')
    expect(html).toContain('budget exceeded')
    expect(html).toContain('evidence not_found (0)')
    expect(html).toContain('cache 50%')
    expect(html).toContain('childRun:job_1')
    expect(html).toContain('job:job_1')
    expect(html).toContain('parentThread:thr_parent')
    expect(html).toContain('parentTurn:turn_parent')
    expect(html).toContain('parentCall:call_parent')
    expect(html).toContain('thr_child')
    expect(html).toContain('profile:researcher')
    expect(html).toContain('profileMode:subagent')
    expect(html).toContain('profileDescription:research profile')
    expect(html).toContain('toolPolicy:readOnly')
    expect(html).toContain('tokenBudget:1024')
    expect(html).toContain('timeBudget:30s')
    expect(html).toContain('parallelGroup:parallel_1')
    expect(html).toContain('Job')
    expect(html).toContain('runtime status possibly stalled')
    expect(html).toContain('idle 45s')
    expect(html).toContain('heartbeat 15s')
    expect(html).toContain('output 21B')
    expect(html).not.toContain('bash_output')
    expect(html).toContain('task_job_stale')
    expect(html).not.toContain('job_1.log')
    expect(html).toContain('Sources 1')
    expect(html).toContain('https://example.com/analytix')
  })

  it('renders the same runtime metadata on process timeline rows', () => {
    const block: ToolBlock = toolBlock({
      summary: 'delegate: research',
      meta: {
        attachmentIds: ['att_1'],
        activeSkillIds: ['skill_docs'],
        injectedMemoryIds: ['mem_1'],
        diagnostics: {
          status: 'completed',
          terminal: true,
          idleMs: 2_500,
          outputBytes: 128,
          artifactPath: '/tmp/project/.analytix/jobs/job_2.log'
        },
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'child_research',
          childRunId: 'job_2',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          childLabel: 'research',
          childStatus: 'completed',
          childSeq: 4,
          childProfile: 'reviewer',
          childToolPolicy: 'readOnly',
          toolInvocations: 2,
          durationMs: 2500,
          queuedMs: 100,
          background: true,
          parallelGroupId: 'parallel_2',
          parallelIndex: 3,
          totalTokens: 128,
          cacheHitRate: 75
        },
        sources: [
          {
            title: 'Analytix docs',
            url: 'https://example.com/analytix'
          }
        ]
      }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-tool_1', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Attachments 1')
    expect(html).toContain('Skills 1')
    expect(html).toContain('Memories 1')
    expect(html).toContain('Child agent')
    expect(html).toContain('research')
    expect(html).toContain('completed')
    expect(html).toContain('#4')
    expect(html).toContain('2 tools')
    expect(html).toContain('duration 2.5s')
    expect(html).toContain('queued 100ms')
    expect(html).toContain('background')
    expect(html).toContain('parallel 3')
    expect(html).toContain('128 tokens')
    expect(html).toContain('cache 75%')
    expect(html).toContain('childRun:job_2')
    expect(html).toContain('parentThread:thr_parent')
    expect(html).toContain('parentTurn:turn_parent')
    expect(html).toContain('parentCall:call_parent')
    expect(html).toContain('thr_child')
    expect(html).toContain('profile:reviewer')
    expect(html).toContain('toolPolicy:readOnly')
    expect(html).toContain('parallelGroup:parallel_2')
    expect(html).toContain('Job')
    expect(html).toContain('completed')
    expect(html).toContain('idle 2.5s')
    expect(html).toContain('output 128B')
    expect(html).not.toContain('job_2.log')
    expect(html).toContain('Sources 1')
  })

  it('shows child-agent work in collapsed execution section summaries', () => {
    const childBlock: ToolBlock = toolBlock({
      id: 'tool_child',
      summary: 'task: research',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          childLabel: 'research',
          childRunId: 'job_3',
          childStatus: 'running'
        }
      }
    })
    const tool = toolBlock({
      id: 'tool_search',
      summary: 'grep: search',
      meta: { toolName: 'grep', pattern: 'needle' }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-group', kind: 'execution', blocks: [childBlock, tool] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Ran 1 child agent')
    expect(html).toContain('Explored workspace')
  })

  it('renders grouped child-agent work as low-noise single-line process rows', () => {
    const childBlock: ToolBlock = toolBlock({
      id: 'tool_child_card',
      summary: 'task: research workspace',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_card',
          childRunId: 'job_card',
          childThreadId: 'thr_child_card',
          childLabel: 'research',
          childStatus: 'completed',
          childProfile: 'researcher',
          toolInvocations: 2,
          totalTokens: 42,
          cacheHitRate: 0.5
        }
      }
    })
    const secondChildBlock: ToolBlock = toolBlock({
      id: 'tool_child_card_2',
      summary: 'task: audit message timeline',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_card_2',
          childRunId: 'job_card_2',
          childThreadId: 'thr_child_card_2',
          childLabel: 'audit',
          childStatus: 'completed',
          childProfile: 'reviewer',
          childSeq: 2,
          toolInvocations: 1
        }
      }
    })

    const [section] = groupProcessSections([childBlock, secondChildBlock])
    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: section!,
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(section?.kind).toBe('subagent')
    expect(html).toContain('ds-subagent-mount')
    expect(html).toContain('Created 2 agents')
    expect(html).toContain('research')
    expect(html).toContain('researcher')
    expect(html).toContain('Created research (researcher) with: research workspace')
    expect(html).toContain('Created audit (reviewer) with: audit message timeline')
    expect(html).toContain('2 tools')
    expect(html).toContain('42 tokens')
    expect(html).toContain('cache 50%')
    expect(html).not.toContain('rounded-[20px]')
    expect(html).not.toContain('StatusPill')
  })

  it('shows child thread navigation only when a real childThreadId is present', () => {
    const withThread = toolBlock({
      id: 'tool_child_with_thread',
      summary: 'task: inspect',
      toolKind: 'subagent',
      detail: JSON.stringify({
        childId: 'job_without_thread_fallback',
        childThreadId: 'thr_child_real',
        input: 'inspect'
      }),
      meta: { toolName: 'task' }
    })
    const withoutThread = toolBlock({
      id: 'tool_child_without_thread',
      summary: 'task: inspect',
      toolKind: 'subagent',
      detail: JSON.stringify({
        childId: 'job_only',
        input: 'inspect'
      }),
      meta: { toolName: 'task' }
    })

    const withThreadHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: withThread }))
    const withoutThreadHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: withoutThread }))

    expect(withThreadHtml).toMatch(/subagentOpenSession|Open child session/)
    expect(withoutThreadHtml).not.toMatch(/subagentOpenSession|Open child session/)
  })

  it('shows a steer affordance only for running background child jobs that can accept steer', () => {
    const running = toolBlock({
      id: 'tool_child_steer_running',
      summary: 'task: inspect',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_steer',
          childRunId: 'job_steer',
          jobId: 'job_steer',
          childThreadId: 'thr_child_steer',
          childLabel: 'inspect',
          childStatus: 'running',
          background: true,
          canAcceptSteer: true,
          pendingSteers: 1,
          steerStatus: 'queued'
        }
      }
    })
    const completed = toolBlock({
      ...running,
      id: 'tool_child_steer_completed',
      meta: {
        ...running.meta,
        child: {
          ...(running.meta?.child as Record<string, unknown>),
          childStatus: 'completed',
          canAcceptSteer: false
        }
      }
    })

    const runningHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: running }))
    const completedHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: completed }))

    expect(runningHtml).toMatch(/subagentSteerPlaceholder|Message child at next safe point/)
    expect(runningHtml).toMatch(/subagentSteerSend|Send steer message/)
    expect(runningHtml).toContain('steer queued')
    expect(runningHtml).toMatch(/subagentSteerPending|1 steer queued/)
    expect(completedHtml).not.toMatch(/subagentSteerPlaceholder|Message child at next safe point/)
    expect(completedHtml).not.toMatch(/subagentSteerSend|Send steer message/)
  })

  it('shows pause and resume controls only for eligible background child jobs', () => {
    const running = toolBlock({
      id: 'tool_child_pause_running',
      summary: 'task: inspect',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_pause',
          childRunId: 'job_pause',
          jobId: 'job_pause',
          childThreadId: 'thr_child_pause',
          childLabel: 'inspect',
          childStatus: 'running',
          background: true,
          canPause: true,
          canResume: false
        }
      }
    })
    const paused = toolBlock({
      ...running,
      id: 'tool_child_pause_paused',
      meta: {
        ...running.meta,
        child: {
          ...(running.meta?.child as Record<string, unknown>),
          childStatus: 'paused',
          canPause: false,
          canResume: true,
          paused: true,
          pauseStatus: 'paused',
          pauseRequestId: 'pause_1'
        }
      }
    })
    const completed = toolBlock({
      ...running,
      id: 'tool_child_pause_completed',
      meta: {
        ...running.meta,
        child: {
          ...(running.meta?.child as Record<string, unknown>),
          childStatus: 'completed',
          canPause: false,
          canResume: false
        }
      }
    })

    const runningHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: running }))
    const pausedHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: paused }))
    const completedHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: completed }))

    expect(runningHtml).toMatch(/subagentPauseAction|Pause at safe boundary/)
    expect(runningHtml).toMatch(/subagentKillAction|Kill child run/)
    expect(runningHtml).toMatch(/subagentPauseHint|Pauses at the next safe child boundary/)
    expect(runningHtml).not.toMatch(/subagentResumeAction|Resume child run/)
    expect(pausedHtml).toMatch(/subagentResumeAction|Resume child run/)
    expect(pausedHtml).toMatch(/subagentKillAction|Kill paused child run/)
    expect(pausedHtml).toContain('pause paused')
    expect(completedHtml).not.toMatch(/subagentPauseAction|Pause at safe boundary/)
    expect(completedHtml).not.toMatch(/subagentResumeAction|Resume child run/)
    expect(completedHtml).not.toMatch(/subagentKillAction|Kill paused child run/)
  })

  it('shows recovery controls instead of pause/resume when a child job lease is unsafe', () => {
    const leaseExpired = toolBlock({
      id: 'tool_child_lease_expired',
      summary: 'task: inspect',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_lease',
          childRunId: 'job_lease',
          jobId: 'job_lease',
          childThreadId: 'thr_child_lease',
          childLabel: 'inspect',
          childStatus: 'running',
          background: true,
          canPause: true,
          canResume: false,
          canAcceptSteer: true,
          heartbeatStatus: 'lease_expired',
          leaseOwner: 'analytix-runtime:123',
          leaseExpiresAt: '2026-07-07T00:00:00.000Z'
        }
      }
    })
    const completed = toolBlock({
      ...leaseExpired,
      id: 'tool_child_lease_expired_completed',
      meta: {
        ...leaseExpired.meta,
        child: {
          ...(leaseExpired.meta?.child as Record<string, unknown>),
          childStatus: 'completed'
        }
      }
    })

    const leaseHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: leaseExpired }))
    const completedHtml = renderToStaticMarkup(createElement(SubagentCallCard, { block: completed }))

    expect(leaseHtml).not.toMatch(/subagentOutputAction|Inspect child output/)
    expect(leaseHtml).toMatch(/subagentRecoverAction|Recover child run/)
    expect(leaseHtml).toMatch(/subagentRestartAction|Restart child run/)
    expect(leaseHtml).toMatch(/subagentRecoveryKillAction|Kill expired child run/)
    expect(leaseHtml).toMatch(/subagentRecoveryHint|Runtime state needs attention/)
    expect(leaseHtml).not.toContain('inspect output')
    expect(leaseHtml).toContain('lease expired')
    expect(leaseHtml).not.toMatch(/subagentPauseAction|Pause at safe boundary/)
    expect(leaseHtml).not.toMatch(/subagentResumeAction|Resume child run/)
    expect(leaseHtml).not.toMatch(/subagentSteerPlaceholder|Message child at next safe point/)
    expect(completedHtml).not.toMatch(/subagentRestartAction|Restart child run/)
    expect(completedHtml).not.toMatch(/subagentRecoveryKillAction|Kill expired child run/)
  })

  it('shows worktree isolation metadata without merge controls', () => {
    const block = toolBlock({
      id: 'tool_child_worktree',
      summary: 'task: edit safely',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_worktree',
          childRunId: 'job_worktree',
          jobId: 'job_worktree',
          childThreadId: 'thr_child_worktree',
          childLabel: 'edit safely',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_worktree',
          worktreeBranch: 'codex/subagent/thr_parent/job_worktree',
          baseCommit: 'abc1234567890',
          currentCommit: 'def1234567890',
          changedFileCount: 2,
          changedFiles: [{ path: 'src/app.ts', status: 'M' }, { path: 'src/new.ts', status: '??' }],
          mergeStatus: 'not_requested'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('isolated worktree')
    expect(html).toContain('codex/subagent/thr_parent/job_worktree')
    expect(html).toContain('/tmp/analytix/subagent-worktrees/thr_parent/job_worktree')
    expect(html).toContain('not_requested')
    expect(html).not.toContain('Inspect isolated diff')
    expect(html).not.toMatch(/Accept|Reject isolated result|Clean up isolated worktree|Merge now/)
  })

  it('does not expose worktree mutation controls for clean reviewed isolated child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_reviewed_worktree',
      summary: 'task: review finished changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_reviewed',
          childRunId: 'job_reviewed',
          jobId: 'job_reviewed',
          childThreadId: 'thr_child_reviewed',
          childLabel: 'review finished changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_reviewed',
          worktreeBranch: 'codex/subagent/thr_parent/job_reviewed',
          changedFileCount: 1,
          mergeStatus: 'review_requested'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Inspect isolated diff')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
    expect(html).not.toContain('Clean up isolated worktree')
    expect(html).not.toContain('Parent decision only; no automatic cleanup')
    expect(html).not.toMatch(/Merge now/)
  })

  it('does not expose worktree controls for conflicted isolated child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_conflicted_worktree',
      summary: 'task: review conflicted changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_conflicted',
          childRunId: 'job_conflicted',
          jobId: 'job_conflicted',
          childThreadId: 'thr_child_conflicted',
          childLabel: 'review conflicted changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_conflicted',
          worktreeBranch: 'codex/subagent/thr_parent/job_conflicted',
          changedFileCount: 1,
          changedFiles: [{ path: 'src/app.ts', status: 'UU' }],
          mergeStatus: 'conflicted'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Inspect isolated diff')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
    expect(html).not.toContain('Clean up isolated worktree')
    expect(html).not.toContain('Record conflict report')
    expect(html).not.toContain('Accept clean repair patch')
  })

  it('does not expose repair dry-run controls for conflicted child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_repair_check_worktree',
      summary: 'task: repair conflicted changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_repair_check',
          childRunId: 'job_repair_check',
          jobId: 'job_repair_check',
          childThreadId: 'thr_child_repair_check',
          childLabel: 'repair conflicted changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_repair_check',
          worktreeBranch: 'codex/subagent/thr_parent/job_repair_check',
          changedFileCount: 1,
          changedFiles: [{ path: 'src/app.ts', status: 'UU' }],
          conflictReportId: 'conflict-1',
          conflictSummary: 'src/app.ts needs repair',
          conflictFileCount: 1,
          mergeStatus: 'conflicted'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Record conflict report')
    expect(html).not.toContain('Paste a repair patch to dry-run against the parent workspace')
    expect(html).not.toContain('Dry-run repair patch')
    expect(html).not.toContain('Accept clean repair patch')
  })

  it('does not expose repair accept after a clean repair review', () => {
    const block = toolBlock({
      id: 'tool_child_repair_accept_worktree',
      summary: 'task: accept repaired changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_repair_accept',
          childRunId: 'job_repair_accept',
          jobId: 'job_repair_accept',
          childThreadId: 'thr_child_repair_accept',
          childLabel: 'accept repaired changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_repair_accept',
          worktreeBranch: 'codex/subagent/thr_parent/job_repair_accept',
          changedFileCount: 1,
          conflictReportId: 'conflict-1',
          repairReviewId: 'repair-review-1',
          repairDryRunStatus: 'clean',
          repairChangedFileCount: 1,
          mergeStatus: 'repair_checked'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Accept clean repair patch')
    expect(html).not.toContain('Repair check is dry-run only; Accept repair requires parent approval')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
  })

  it('does not show cleanup for active isolated child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_active_worktree',
      summary: 'task: still editing',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_active',
          childRunId: 'job_active',
          jobId: 'job_active',
          childThreadId: 'thr_child_active',
          childLabel: 'still editing',
          childStatus: 'running',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_active',
          worktreeBranch: 'codex/subagent/thr_parent/job_active',
          changedFileCount: 1,
          mergeStatus: 'review_requested'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Inspect isolated diff')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
    expect(html).not.toContain('Clean up isolated worktree')
    expect(html).not.toContain('Record conflict report')
    expect(html).not.toContain('Accept clean repair patch')
  })

  it('does not expose cleanup for accepted isolated child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_accepted_worktree',
      summary: 'task: accepted changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_accepted',
          childRunId: 'job_accepted',
          jobId: 'job_accepted',
          childThreadId: 'thr_child_accepted',
          childLabel: 'accepted changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_accepted',
          worktreeBranch: 'codex/subagent/thr_parent/job_accepted',
          changedFileCount: 1,
          mergeStatus: 'accepted',
          acceptDecisionId: 'accept-1',
          appliedPatchDigest: 'sha256:abc'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('accepted accept-1')
    expect(html).toContain('patch sha256:abc')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
    expect(html).not.toContain('Clean up isolated worktree')
  })

  it('hides isolation decision actions for cleaned child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_cleaned_worktree',
      summary: 'task: cleaned changes',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_cleaned',
          childRunId: 'job_cleaned',
          jobId: 'job_cleaned',
          childThreadId: 'thr_child_cleaned',
          childLabel: 'cleaned changes',
          childStatus: 'completed',
          childToolPolicy: 'inherit',
          background: true,
          isolationMode: 'worktree',
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent/job_cleaned',
          worktreeBranch: 'codex/subagent/thr_parent/job_cleaned',
          changedFileCount: 1,
          mergeStatus: 'cleaned',
          acceptDecisionId: 'accept-1',
          cleanupReceiptId: 'cleanup-1',
          cleanupRemoved: true
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('cleanup cleanup-1')
    expect(html).not.toContain('Accept clean isolated diff')
    expect(html).not.toContain('Reject isolated result')
    expect(html).not.toContain('Clean up isolated worktree')
    expect(html).not.toContain('Record conflict report')
    expect(html).not.toContain('Accept clean repair patch')
  })

  it('does not show isolated diff inspection for ordinary child jobs', () => {
    const block = toolBlock({
      id: 'tool_child_plain',
      summary: 'task: inspect safely',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_plain',
          childRunId: 'job_plain',
          jobId: 'job_plain',
          childThreadId: 'thr_child_plain',
          childLabel: 'inspect safely',
          childStatus: 'completed',
          childToolPolicy: 'readOnly',
          background: true
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).not.toContain('Inspect isolated diff')
  })

  it('shows child todo projections as parent-owned proposals', () => {
    useChatStore.setState({
      activeThreadTodos: {
        threadId: 'thr_parent',
        updatedAt: '2026-07-07T00:00:00Z',
        items: [
          {
            id: 'parent-todo',
            content: 'Parent mapped todo',
            status: 'pending',
            createdAt: '2026-07-07T00:00:00Z',
            updatedAt: '2026-07-07T00:00:00Z'
          }
        ]
      }
    })
    const block = toolBlock({
      id: 'tool_child_todo_projection',
      summary: 'task: child todos',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_child_todos',
          childRunId: 'job_child_todos',
          jobId: 'job_child_todos',
          childThreadId: 'thr_child_todos',
          childLabel: 'child todos',
          childStatus: 'completed',
          childToolPolicy: 'readOnly',
          background: true,
          childTodoListId: 'child-todos-1',
          childTodoScope: 'child',
          childTodoCount: 3,
          childTodoCompletedCount: 1,
          childTodoInProgressCount: 1,
          childTodoPendingCount: 1,
          childTodoProjectionId: 'projection-1',
          childTodoProjectionStatus: 'proposed',
          childTodoProjectionSummary: '3 child todos: 1 completed, 1 in progress, 1 pending, 0 blocked, 0 canceled',
          childTodoProjectionItemCount: 3,
          childTodoProjectionEvidenceCount: 2,
          childTodoProjectionMappedCount: 1,
          childTodoProjectionCompletedMappedCount: 1
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('3 child todos')
    expect(html).toContain('projection proposed')
    expect(html).toContain('3 projected items')
    expect(html).toContain('2 evidence ids')
    expect(html).toContain('1 completed mapped to parent todos')
    expect(html).toContain('Child todo projection is a proposal; parent approval is required')
    expect(html).toContain('Accept child todo projection')
    expect(html).toContain('Reject child todo projection')
    expect(html).not.toMatch(/Complete parent goal|Parent todo completed/)
  })

  it('hides active child todo projection controls after rejection', () => {
    const block = toolBlock({
      id: 'tool_child_todo_rejected',
      summary: 'task: rejected child todos',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_child_todos_rejected',
          childRunId: 'job_child_todos_rejected',
          jobId: 'job_child_todos_rejected',
          childThreadId: 'thr_child_todos',
          childLabel: 'child todos rejected',
          childStatus: 'completed',
          childToolPolicy: 'readOnly',
          background: true,
          childTodoListId: 'child-todos-1',
          childTodoScope: 'child',
          childTodoCount: 1,
          childTodoProjectionId: 'projection-1',
          childTodoProjectionStatus: 'rejected',
          childTodoProjectionItemCount: 1,
          childTodoProjectionMappedCount: 1,
          childTodoProjectionCompletedMappedCount: 1,
          childTodoProjectionRejectedAt: '2026-07-07T00:01:00.000Z'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('projection rejected')
    expect(html).not.toContain('Child todo projection is a proposal; parent approval is required')
    expect(html).not.toContain('Accept child todo projection')
    expect(html).not.toContain('Reject child todo projection')
  })

  it('shows accepted child todo projection decision metadata without active accept controls', () => {
    const block = toolBlock({
      id: 'tool_child_todo_accepted',
      summary: 'task: accepted child todos',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          parentToolCallId: 'call_parent',
          childId: 'job_child_todos_accepted',
          childRunId: 'job_child_todos_accepted',
          jobId: 'job_child_todos_accepted',
          childThreadId: 'thr_child_todos',
          childLabel: 'child todos accepted',
          childStatus: 'completed',
          childToolPolicy: 'readOnly',
          background: true,
          childTodoCount: 1,
          childTodoProjectionId: 'projection-1',
          childTodoProjectionStatus: 'accepted',
          childTodoProjectionDecisionId: 'projection-decision-1',
          childTodoProjectionDecision: 'accepted',
          childTodoProjectionAcceptedItemCount: 1,
          childTodoProjectionSkippedItemCount: 0,
          childTodoProjectionItemCount: 1,
          childTodoProjectionAcceptedAt: '2026-07-07T00:01:00.000Z'
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('projection accepted')
    expect(html).toContain('decision accepted: 1 accepted, 0 skipped')
    expect(html).not.toContain('Accept child todo projection')
    expect(html).not.toContain('Reject child todo projection')
  })

  it('coalesces non-adjacent child-agent lifecycle rows into one parent-turn group', () => {
    const firstRunning: ToolBlock = toolBlock({
      id: 'tool_child_split_a_running',
      summary: 'task: 文件结构优化',
      status: 'running',
      toolKind: 'subagent',
      detail: JSON.stringify({ input: '文件结构优化 - 分析目录结构和存储优化方案' }),
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_a',
          childRunId: 'job_a',
          childLabel: '文件结构优化',
          childStatus: 'running',
          childProfile: 'design-reviewer',
          parallelIndex: 3,
          childSeq: 3
        }
      }
    })
    const interleavedTool: ToolBlock = toolBlock({
      id: 'tool_read_between_children',
      summary: 'Read README',
      toolKind: 'tool_call',
      meta: {
        toolName: 'read',
        turnId: 'turn_parent'
      }
    })
    const secondRunning: ToolBlock = toolBlock({
      id: 'tool_child_split_b_running',
      summary: 'task: 深度安全威胁分析',
      status: 'running',
      toolKind: 'subagent',
      detail: JSON.stringify({ input: '深度安全威胁分析 - 分析可疑 mp4.js 文件、系统安全风险' }),
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_b',
          childRunId: 'job_b',
          childLabel: '深度安全威胁分析',
          childStatus: 'running',
          childProfile: 'design-reviewer',
          parallelIndex: 1,
          childSeq: 1
        }
      }
    })
    const firstCompleted: ToolBlock = toolBlock({
      ...firstRunning,
      id: 'tool_child_split_a_completed',
      status: 'success',
      meta: {
        ...firstRunning.meta,
        child: {
          ...(firstRunning.meta?.child as Record<string, unknown>),
          childStatus: 'completed',
          toolInvocations: 0
        }
      }
    })

    const sections = groupProcessSections([firstRunning, interleavedTool, secondRunning, firstCompleted])
    const html = renderToStaticMarkup(
      createElement('div', null,
        sections.map((section) => createElement(ProcessSectionRow, {
          key: section.id,
          section,
          processing: true,
          viewportRef: { current: null }
        }))
      )
    )

    expect(sections.map((section) => section.kind)).toEqual(['subagent', 'execution'])
    expect(sections[0]?.blocks).toHaveLength(2)
    expect(html).toContain('Running 2 agents')
    expect(html).not.toContain('Running 1 agents')
    expect(html).toContain('Created 文件结构优化 (design-reviewer) with: 文件结构优化 - 分析目录结构和存储优化方案')
    expect(html).toContain('Running 深度安全威胁分析 (design-reviewer) with: 深度安全威胁分析 - 分析可疑 mp4.js 文件、系统安全风险')
    expect(html).toContain('Read README')
  })

  it('renders a single aggregated parallel_tasks result as compact child-agent rows', () => {
    const block: ToolBlock = toolBlock({
      id: 'tool_parallel_tasks_done',
      summary: 'parallel_tasks',
      status: 'success',
      toolKind: 'subagent',
      detail: JSON.stringify({
        kind: 'parallel_tasks',
        status: 'completed',
        taskCount: 4,
        tasks: [
          { childRunId: 'job_1', label: 'Heisenberg', profile: 'explorer', prompt: 'readonly audit Kun' },
          { childRunId: 'job_2', label: 'Pascal', profile: 'explorer', prompt: 'readonly audit Reasonix' },
          { childRunId: 'job_3', label: 'Volta', profile: 'explorer', prompt: 'readonly audit claw-code' },
          { childRunId: 'job_4', label: 'Kuhn', profile: 'explorer', prompt: 'readonly audit opencode' }
        ]
      }),
      meta: { toolName: 'parallel_tasks' }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block }))

    expect(html).toContain('Created 4 agents')
    expect(html).not.toContain('Created 1 agents')
    expect(html).toContain('Created Heisenberg (explorer) with: readonly audit Kun')
    expect(html).toContain('Created Pascal (explorer) with: readonly audit Reasonix')
    expect(html).toContain('Created Volta (explorer) with: readonly audit claw-code')
    expect(html).toContain('Created Kuhn (explorer) with: readonly audit opencode')
    expect(html).not.toContain('Input')
    expect(html).not.toContain('Result')
  })

  it('shows a single failed child-agent as expanded multi-line diagnostics', () => {
    const childBlock: ToolBlock = toolBlock({
      id: 'tool_child_failed',
      summary: 'task: readonly audit',
      status: 'error',
      toolKind: 'subagent',
      detail: JSON.stringify({
        input: '只读审计任务 A：对照 Kun 检查 timeline 体感。',
        error: '当前线程的历史子代理数量已经到上限'
      }),
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_failed',
          childRunId: 'job_failed',
          childLabel: 'task A',
          childStatus: 'failed',
          childProfile: 'explorer'
        }
      }
    })

    const [section] = groupProcessSections([childBlock])
    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: section!,
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(section?.kind).toBe('subagent')
    expect(html).toContain('Failed to create 1 agents')
    expect(html).toContain('Creation failed')
    expect(html).toContain('Input')
    expect(html).toContain('只读审计任务 A')
    expect(html).toContain('Error')
    expect(html).toContain('子任务执行失败')
    expect(html).not.toContain('历史子代理数量已经到上限')
    expect(html).toContain('aria-expanded="true"')
  })

  it('does not expose background completion output preview as child-agent result detail', () => {
    const privateOutputSentinel = 'PRIVATE_CHILD_RESULT_7F3C'
    const childBlock: ToolBlock = toolBlock({
      id: 'tool_child_completed_preview',
      summary: 'task: background wake',
      status: 'success',
      toolKind: 'subagent',
      detail: JSON.stringify({ input: '只回复一行 BG_WAKE_DONE' }),
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'child_preview',
          childRunId: 'job_preview',
          childLabel: 'background wake',
          childStatus: 'completed',
          childTodoProjectionId: 'projection_preview'
        },
        diagnostics: {
          notificationKind: 'background_job_completion',
          status: 'completed',
          terminal: true,
          background: true,
          canContinueParent: true,
          outputPreview: privateOutputSentinel
        }
      }
    })

    const html = renderToStaticMarkup(createElement(SubagentCallCard, { block: childBlock }))

    expect(html).not.toContain('Result')
    expect(html).not.toContain(privateOutputSentinel)
  })

  it('renders closed child-agent groups with the closed summary style', () => {
    const closedBlocks: ToolBlock[] = ['Dirac', 'Franklin'].map((name, index) => toolBlock({
      id: `tool_child_closed_${index}`,
      summary: `task: ${name}`,
      status: 'error',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: `child_closed_${index}`,
          childRunId: `job_closed_${index}`,
          childLabel: name,
          childStatus: 'killed',
          childProfile: 'explorer',
          childSeq: index + 1
        }
      }
    }))

    const [section] = groupProcessSections(closedBlocks)
    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: section!,
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(section?.kind).toBe('subagent')
    expect(html).toContain('Closed 2 agents')
    expect(html).toContain('Closed Dirac (explorer)')
    expect(html).toContain('Closed Franklin (explorer)')
  })

  it('groups background shell child events into a background shell card section', () => {
    const shellBlock: ToolBlock = toolBlock({
      id: 'tool_shell_background',
      summary: 'bash: npm test',
      status: 'running',
      toolKind: 'command_execution',
      meta: {
        toolName: 'bash',
        child: {
          kind: 'background-shell',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          childId: 'job_shell',
          childRunId: 'job_shell',
          childLabel: 'npm test',
          childName: 'bash',
          childStatus: 'running',
          background: true
        },
        diagnostics: {
          status: 'running',
          background: true,
          outputBytes: 2048,
          artifactPath: '/tmp/project/.analytix/jobs/job_shell.log'
        }
      }
    })

    const [section] = groupProcessSections([shellBlock])
    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: section!,
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(section?.kind).toBe('background_shell')
    expect(html).toContain('ds-subagent-mount')
    expect(html).toContain('npm test')
    expect(html).toContain('running')
    expect(html).not.toContain('output 2.0KB')
    expect(html).not.toContain('job_shell.log')
  })

  it('resolves injected memory tooltip text from runtime summaries before ids', () => {
    const lines = resolveInjectedMemoryTooltipLines(
      {
        injectedMemorySummaries: [
          {
            id: 'mem_1',
            content: 'Remembered workspace preference'
          }
        ]
      },
      ['mem_1', 'mem_2'],
      new Map([['mem_2', 'Fallback memory preview']])
    )

    expect(lines).toEqual([
      '1. Remembered workspace preference',
      '2. Fallback memory preview'
    ])
  })

  it('auto-expands structured subagent and job diagnostics in the process timeline', () => {
    const block: ToolBlock = toolBlock({
      id: 'tool_child_detail',
      summary: 'task: research',
      toolKind: 'subagent',
      meta: {
        toolName: 'task',
        child: {
          childLabel: 'research',
          childId: 'child_4',
          childRunId: 'job_4',
          jobId: 'job_4',
          childThreadId: 'thr_child_4',
          childTurnId: 'turn_child_4',
          childStatus: 'completed',
          childProfile: 'reviewer',
          childProfileMode: 'subagent',
          childProfileDescription: 'review profile',
          childEffort: 'high',
          childToolPolicy: 'readOnly',
          returnFormat: 'evidence',
          tokenBudget: 2048,
          timeBudgetMs: 60000,
          budgetExceeded: true,
          evidenceBundleStatus: 'parsed',
          evidenceCount: 2,
          parentThreadId: 'thr_parent_4',
          parentTurnId: 'turn_parent_4',
          parentToolCallId: 'call_parent_4',
          parallelIndex: 2,
          parallelGroupId: 'parallel_4',
          childSeq: 7,
          toolInvocations: 3,
          durationMs: 2500,
          queuedMs: 120,
          totalTokens: 321,
          cacheHitRate: 0.8,
          background: true,
          worktreePath: '/tmp/analytix/subagent-worktrees/thr_parent_4/job_4',
          worktreeBranch: 'codex/subagent/thr_parent_4/job_4',
          baseCommit: 'abc1234567890',
          currentCommit: 'def1234567890',
          changedFileCount: 2,
          mergeStatus: 'not_requested'
        },
        diagnostics: {
          status: 'completed',
          heartbeatStatus: 'lease_expired',
          warningCode: 'slow-output',
          warning: 'output delayed',
          suggestion: 'wait again',
          artifactPath: 'job_4.log',
          idleMs: 500,
          heartbeatAt: '2026-06-03T10:00:10.000Z',
          lastHeartbeatAt: '2026-06-03T10:00:10.000Z',
          heartbeatAgeMs: 750,
          leaseOwner: 'analytix-runtime',
          leaseExpiresAt: '2026-06-03T10:00:00.000Z',
          leaseExpired: true,
          staleAfterMs: 30000,
          recoveryStatus: 'recovering',
          recoveryAttempt: 2,
          recoveryReason: 'runtime restart scan',
          recoveryUpdatedAt: '2026-06-03T10:00:11.000Z',
          deadLetterReason: 'parent missing',
          outputBytes: 2048,
          nextOffset: 4096,
          stalled: true,
          background: true,
          suggestedTools: ['wait', 'bash_output']
        }
      }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-tool_child_detail', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('aria-expanded="true"')
    expect(html).toContain('ds-work-timeline-detail')
    expect(html).toMatch(/<dt[^>]*>run<\/dt><dd[^>]*>job_4<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>job<\/dt><dd[^>]*>job_4<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>parent<\/dt><dd[^>]*>thr_parent_4 · turn_parent_4 · call_parent_4<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>profile<\/dt><dd[^>]*>reviewer<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>mode<\/dt><dd[^>]*>subagent<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>profile note<\/dt><dd[^>]*>review profile<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>return<\/dt><dd[^>]*>evidence<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>token budget<\/dt><dd[^>]*>2048<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>time budget<\/dt><dd[^>]*>1m 0s<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>budget exceeded<\/dt><dd[^>]*>yes<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>evidence<\/dt><dd[^>]*>parsed \(2\)<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>cache<\/dt><dd[^>]*>80%<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>worktree<\/dt><dd[^>]*>codex\/subagent\/thr_parent_4\/job_4<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>worktree path<\/dt><dd[^>]*>\/tmp\/analytix\/subagent-worktrees\/thr_parent_4\/job_4<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>changed files<\/dt><dd[^>]*>2<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>merge status<\/dt><dd[^>]*>not_requested<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>runtime status<\/dt><dd[^>]*>lease expired<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>last heartbeat<\/dt><dd[^>]*>2026-06-03T10:00:10.000Z<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>lease owner<\/dt><dd[^>]*>analytix-runtime<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>lease expires<\/dt><dd[^>]*>2026-06-03T10:00:00.000Z<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>stale after<\/dt><dd[^>]*>30s<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>recovery status<\/dt><dd[^>]*>recovering<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>recovery attempt<\/dt><dd[^>]*>2<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>recovery updated<\/dt><dd[^>]*>2026-06-03T10:00:11.000Z<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>heartbeat<\/dt><dd[^>]*>2026-06-03T10:00:10.000Z · 750ms<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>lease expired<\/dt><dd[^>]*>yes<\/dd>/)
    expect(html).toMatch(/<dt[^>]*>output<\/dt><dd[^>]*>2.0KB<\/dd>/)
    expect(html).not.toContain('runtime restart scan')
    expect(html).not.toContain('parent missing')
    expect(html).not.toContain('job_4.log')
    expect(html).not.toContain('wait, bash_output')
  })

  it('renders provider diagnostics on runtime status bubbles and process rows', () => {
    const block: ChatBlock = {
      kind: 'system',
      id: 'runtime_status_turn_provider_error_1',
      text: 'Provider stream failed',
      meta: {
        providerError: {
          providerId: 'deepseek-prod sk-providerSecret1234567890',
          endpointFormat: 'openai-chat',
          status: 401,
          kind: 'auth',
          hasApiKey: true,
          authStatus: 'token=providerAuthSecret123456',
          retryable: false
        }
      }
    }

    const bubbleHtml = renderToStaticMarkup(createElement(MessageBubble, { block }))
    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-runtime_status_turn_provider_error_1', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(bubbleHtml).toContain('Provider')
    expect(bubbleHtml).toContain('deepseek-prod')
    expect(bubbleHtml).toContain('&lt;redacted&gt;')
    expect(bubbleHtml).toContain('status 401')
    expect(bubbleHtml).toContain('endpoint openai-chat')
    expect(bubbleHtml).toContain('auth token=&lt;redacted&gt;')
    expect(bubbleHtml).not.toContain('providerSecret1234567890')
    expect(bubbleHtml).not.toContain('providerAuthSecret123456')
    expect(html).toContain('Provider')
    expect(html).toContain('deepseek-prod')
    expect(html).toContain('&lt;redacted&gt;')
    expect(html).toContain('status 401')
    expect(html).toContain('endpoint openai-chat')
    expect(html).toContain('auth token=&lt;redacted&gt;')
    expect(html).not.toContain('providerSecret1234567890')
    expect(html).not.toContain('providerAuthSecret123456')
  })

  it('keeps successful runtime metadata collapsed while retaining diagnostics chips', () => {
    const block = toolBlock({
      id: 'tool_background_output_done_job_75',
      summary: 'bash_output: job-75',
      meta: {
        toolName: 'bash_output',
        diagnostics: {
          jobId: 'job-75',
          status: 'completed',
          heartbeatStatus: 'completed',
          deliveryId: 'delivery_turn_421_job-75',
          deliveryStatus: 'delivered',
          completionDeliveryAttempt: 2,
          outputBytes: 275
        }
      }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-success-runtime-meta', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('delivery status delivered')
    expect(html).toContain('delivery attempts 2')
    expect(html).not.toContain('ds-work-timeline-detail')
    expect(html).not.toMatch(/<dt[^>]*>delivery<\/dt>/)
  })

  it('renders background auto-continue started as an explicit process row with diagnostics', () => {
    const block: ToolBlock = toolBlock({
      id: 'tool_background_auto_continue_started_job_65',
      summary: 'background job woke the parent thread',
      meta: {
        toolName: 'background_auto_continue',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-65',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          autoContinueStatus: 'started',
          autoContinueTurnId: 'turn_400',
          deliveryId: 'delivery_turn_parent_job_65',
          deliveryStatus: 'delivered'
        }
      }
    })

    expect(summarizeToolBlock(block, t)).toBe('Background result woke the parent thread · turn_400')
    expect(groupProcessSections([block])).toHaveLength(1)
    expect(groupProcessSections([block])[0]?.kind).toBe('execution')

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-auto-started', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Background')
    expect(html).toContain('result woke the parent thread · turn_400')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('auto-continue started')
    expect(html).toContain('delivery status delivered')
  })

  it('renders background auto-continue skipped without private reason text', () => {
    const block: ChatBlock = toolBlock({
      id: 'tool_background_auto_continue_skipped_job_67',
      summary: 'background job did not auto-continue the parent',
      meta: {
        toolName: 'background_auto_continue',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-67',
          childThreadId: 'thr_child_b',
          autoContinueStatus: 'skipped',
          reason: 'parent_turn_not_latest',
          lateCompletionSuppressed: false
        }
      }
    })

    expect(summarizeToolBlock(block, t)).toBe('Background result did not auto-continue')

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-auto-skipped', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Background')
    expect(html).toContain('result did not auto-continue')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('auto-continue skipped')
    expect(html).not.toContain('parent_turn_not_latest')
  })

  it('keeps adjacent background auto-continue notifications as separate rows', () => {
    const skipped = toolBlock({
      id: 'tool_background_auto_continue_skipped_job_72',
      summary: 'background job did not auto-continue the parent',
      meta: {
        toolName: 'background_auto_continue',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-72',
          autoContinueStatus: 'skipped',
          reason: 'auto_continue_already_starting'
        }
      }
    })
    const started = toolBlock({
      id: 'tool_background_auto_continue_started_job_71',
      summary: 'background job woke the parent thread',
      meta: {
        toolName: 'background_auto_continue',
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job-71',
          autoContinueStatus: 'started',
          autoContinueTurnId: 'turn_416'
        }
      }
    })

    const sections = groupProcessSections([skipped, started])

    expect(sections).toHaveLength(2)
    expect(sections[0]?.blocks).toHaveLength(1)
    expect(sections[1]?.blocks).toHaveLength(1)
    expect(summarizeToolBlock(sections[0]!.blocks[0] as ToolBlock, t)).toBe(
      'Background result did not auto-continue'
    )
    expect(summarizeToolBlock(sections[1]!.blocks[0] as ToolBlock, t)).toBe(
      'Background result woke the parent thread · turn_416'
    )
  })

  it('renders background delivery ledger rows collapsed with expandable diagnostics', () => {
    const block = toolBlock({
      id: 'tool_background_delivery_delivered_job_71',
      summary: 'background job completion delivered',
      meta: {
        toolName: 'background_delivery',
        diagnostics: {
          notificationKind: 'background_job_delivery',
          jobId: 'job-71',
          childRunId: 'job-71',
          childThreadId: 'thr_child_delivery',
          childTurnId: 'turn_child_delivery',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          deliveryId: 'delivery_turn_parent_job_71',
          deliveryStatus: 'delivered',
          deliveryItemId: 'item_progress_turn_parent_background_job_completed_job_71',
          completionDeliveryAttempt: 2,
          completionDeliveryAt: '2026-07-07T00:00:02.000Z'
        }
      }
    })

    expect(summarizeToolBlock(block, t)).toBe('Background result delivered to the parent thread')

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-delivery-delivered', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Background')
    expect(html).toContain('result delivered to the parent thread')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('data-analytix-process-block-id="tool_background_delivery_delivered_job_71"')
    expect(html).toContain('aria-label="Open runtime diagnostics"')
    expect(html).toContain('delivery status delivered')
    expect(html).toContain('delivery attempts 2')
    expect(html).toContain('delivered at 2026-07-07T00:00:02.000Z')
  })

  it('renders background delivery dead-letter rows without private reason text', () => {
    const block = toolBlock({
      id: 'tool_background_delivery_dead_job_73',
      summary: 'background job completion delivery dead-lettered',
      status: 'error',
      meta: {
        toolName: 'background_delivery',
        diagnostics: {
          notificationKind: 'background_job_delivery',
          jobId: 'job-73',
          childRunId: 'job-73',
          childThreadId: 'thr_child_dead',
          parentThreadId: 'thr_missing',
          parentTurnId: 'turn_missing',
          deliveryId: 'delivery_turn_missing_job_73',
          deliveryStatus: 'dead_letter',
          deliveryReason: 'parent_thread_missing',
          recoveryAttempt: 1,
          deadLetterReason: 'parent_thread_missing',
          completionDeadLetterAt: '2026-07-07T00:00:03.000Z'
        }
      }
    })

    expect(summarizeToolBlock(block, t)).toBe(
      'Background result could not be delivered and was moved to diagnostics'
    )

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-delivery-dead-letter', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('text-orange')
    expect(html).not.toContain('parent_thread_missing')
    expect(html).toContain('delivery status dead_letter')
    expect(html).toContain('dead-lettered at 2026-07-07T00:00:03.000Z')
  })

  it('renders provider retry attempt diagnostics on runtime status rows', () => {
    const block: ChatBlock = {
      kind: 'system',
      id: 'runtime_status_turn_provider_retrying_1',
      text: 'Retrying provider stream',
      meta: {
        providerError: {
          providerId: 'openai-compatible',
          status: 503,
          retryable: true
        },
        providerRetry: {
          attempt: 2,
          maxAttempt: 3
        }
      }
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-runtime_status_turn_provider_retrying_1', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Provider')
    expect(html).toContain('openai-compatible')
    expect(html).toContain('status 503')
    expect(html).toContain('retry 2/3')
    expect(html).toContain('retryable')
  })

  it('renders provider recovery attempt diagnostics on runtime status rows', () => {
    const block: ChatBlock = {
      kind: 'system',
      id: 'runtime_status_turn_empty_final_recovered_1',
      text: 'Provider returned an empty final response',
      meta: {
        providerRecovery: {
          kind: 'empty_final',
          attempt: 1,
          maxAttempt: 1
        }
      }
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-runtime_status_turn_empty_final_recovered_1', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Provider')
    expect(html).toContain('recovery 1/1')
  })

  it('renders exhausted provider recovery diagnostics on runtime status rows', () => {
    const block: ChatBlock = {
      kind: 'system',
      id: 'runtime_status_turn_empty_provider_error_2',
      text: 'Provider returned an empty final response after recovery',
      meta: {
        providerRecovery: {
          kind: 'empty_final',
          attempt: 1,
          maxAttempt: 1,
          recoveryExhausted: true
        }
      }
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-runtime_status_turn_empty_provider_error_2', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Provider')
    expect(html).toContain('recovery 1/1')
    expect(html).toContain('exhausted')
  })

  it('does not render zero child duration as a fake elapsed time', () => {
    const block: ChatBlock = toolBlock({
      summary: 'task: running',
      toolKind: 'subagent',
      meta: {
        child: {
          childId: 'child_running',
          childLabel: 'running child',
          childStatus: 'running',
          durationMs: 0,
          queuedMs: 0,
          toolInvocations: 0
        }
      }
    })

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('Child agent')
    expect(html).toContain('0 tools')
    expect(html).not.toContain('duration 1ms')
    expect(html).not.toContain('queued 1ms')
  })

  it('keeps running tool calls collapsed by default while showing active status', () => {
    const block: ChatBlock = toolBlock({
      summary: 'read: file',
      status: 'running',
      detail: 'partial tool output while running',
      meta: { toolName: 'read' },
      filePath: '/tmp/readme.md'
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-tool_1', kind: 'execution', blocks: [block] },
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Read')
    expect(html).toContain('/tmp/readme.md')
    expect(html).not.toContain('ds-work-logo')
    expect(html).toContain('ds-shiny-text')
    expect(html).not.toContain('partial tool output while running')
    expect(html).toContain('ds-process-file-reference')
  })

  it('keeps completed failed tool details collapsed by default', () => {
    const block: ChatBlock = toolBlock({
      summary: 'Recognize image recognize_image',
      status: 'error',
      detail: 'model request failed with status 401',
      meta: { toolName: 'recognize_image' }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-tool_error', kind: 'execution', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Recognize image recognize_image')
    expect(html).toContain('text-orange-700')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain('model request failed with status 401')
    expect(html).toContain('role="button"')
  })

  it('keeps failed tool details visible while the turn is still processing', () => {
    const block: ChatBlock = toolBlock({
      summary: 'Recognize image recognize_image',
      status: 'error',
      detail: 'model request failed with status 401',
      meta: { toolName: 'recognize_image' }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-tool_error_live', kind: 'execution', blocks: [block] },
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Recognize image recognize_image')
    expect(html).toContain('text-orange-700')
    expect(html).toContain('aria-expanded="true"')
    expect(html).toContain('model request failed with status 401')
  })

  it('keeps completed failed tool bubbles collapsed and orange toned', () => {
    const block: ChatBlock = toolBlock({
      summary: 'List ~/Desktop',
      status: 'error',
      detail: 'tool failed with a long diagnostic payload',
      meta: { toolName: 'ls' }
    })

    const html = renderToStaticMarkup(createElement(MessageBubble, { block }))

    expect(html).toContain('List ~/Desktop')
    expect(html).toContain('border-orange-300')
    expect(html).not.toContain('tool failed with a long diagnostic payload')
  })

  it('renders assistant output process text directly without an output header', () => {
    const block: ChatBlock = {
      kind: 'assistant',
      id: 'assistant_preface',
      text: 'Intermediate note before the final answer.'
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'output-assistant_preface', kind: 'output', blocks: [block] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Intermediate note before the final answer.')
    expect(html).not.toContain('processTextLabel')
    expect(html).not.toContain('>Output<')
    expect(html).not.toContain('>文本输出<')
    expect(html).not.toContain('aria-expanded')
    expect(html).not.toContain('border-l-2')
  })

  it('keeps same-batch tool calls collapsed by default', () => {
    const readBlock: ChatBlock = toolBlock({
      id: 'tool_read',
      summary: 'read: file',
      detail: 'read detail should stay tucked away',
      meta: { toolName: 'read' },
      filePath: '/tmp/readme.md'
    })
    const grepBlock: ChatBlock = toolBlock({
      id: 'tool_grep',
      summary: 'grep: search',
      detail: 'grep detail should stay tucked away',
      meta: { toolName: 'grep', pattern: 'needle' },
      filePath: '/tmp/src'
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-batch', kind: 'execution', blocks: [readBlock, grepBlock] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Explored workspace')
    expect(html).not.toContain('ds-work-stack')
    expect(html).not.toContain('/tmp/readme.md')
    expect(html).not.toContain('needle')
    expect(html).not.toContain('read detail should stay tucked away')
    expect(html).not.toContain('grep detail should stay tucked away')
  })

  it('keeps generic same-batch tool calls grouped separately from workspace exploration', () => {
    const searchBlock: ChatBlock = toolBlock({
      id: 'tool_grep',
      summary: 'grep: search',
      meta: { toolName: 'grep', pattern: 'needle' },
      filePath: '/tmp/src'
    })
    const genericBlock: ChatBlock = toolBlock({
      id: 'tool_generic',
      summary: 'fetch_url: fetch',
      meta: { toolName: 'fetch_url' }
    })

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-batch', kind: 'execution', blocks: [searchBlock, genericBlock] },
        processing: false,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('Explored workspace')
    expect(html).toContain('Used 1 tool')
  })

  it('auto-expands pending request_user_input while keeping other tool details tucked away', () => {
    const readBlock: ChatBlock = toolBlock({
      id: 'tool_read',
      summary: 'read: file',
      detail: 'read detail should stay tucked away',
      meta: { toolName: 'read' },
      filePath: '/tmp/readme.md'
    })
    const inputBlock: ChatBlock = {
      kind: 'user_input',
      id: 'ui_1',
      requestId: 'input_1',
      status: 'pending',
      questions: [
        {
          header: 'Dinner',
          id: 'dinner',
          question: 'What should we eat tonight?',
          options: [
            {
              label: 'Noodles',
              description: 'Fast and warm'
            }
          ]
        }
      ]
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-batch', kind: 'execution', blocks: [readBlock, inputBlock] },
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('ds-work-stack')
    expect(html).toContain('What should we eat tonight?')
    expect(html).toContain('Noodles')
    expect(html).not.toContain('read detail should stay tucked away')
  })

  it('auto-expands pending approvals while keeping other tool details tucked away', () => {
    const readBlock: ChatBlock = toolBlock({
      id: 'tool_read',
      summary: 'read: file',
      detail: 'read detail should stay tucked away',
      meta: { toolName: 'read' },
      filePath: '/tmp/readme.md'
    })
    const approvalBlock: ChatBlock = {
      kind: 'approval',
      id: 'approval_appr_1',
      approvalId: 'appr_1',
      status: 'pending',
      toolName: 'edit',
      summary: 'Run edit(path="/tmp/app.ts")'
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-batch', kind: 'execution', blocks: [readBlock, approvalBlock] },
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('ds-work-stack')
    expect(html).toContain('Run edit(path=&quot;/tmp/app.ts&quot;)')
    expect(html).toMatch(/Approval required|需要审批|approvalTitle/)
    expect(html).toMatch(/Allow|允许|approvalAllow/)
    expect(html).not.toContain('read detail should stay tucked away')
  })

  it('renders request_user_input without options as a freeform answer field', () => {
    const inputBlock: ChatBlock = {
      kind: 'user_input',
      id: 'ui_freeform',
      requestId: 'input_freeform',
      status: 'pending',
      questions: [
        {
          header: 'Input',
          id: 'direction',
          question: '你更想去南方还是北方？',
          options: []
        }
      ]
    }

    const html = renderToStaticMarkup(
      createElement(ProcessSectionRow, {
        section: { id: 'execution-input', kind: 'execution', blocks: [inputBlock] },
        processing: true,
        viewportRef: { current: null }
      })
    )

    expect(html).toContain('你更想去南方还是北方？')
    expect(html).toContain('<textarea')
    expect(html).not.toContain('userInputOther')
    expect(html).not.toContain('其他')
  })

  it('expands the live work timeline by default while keeping tool details collapsed', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'user_1',
        text: 'inspect this file'
      },
      toolBlock({
        summary: 'read: file',
        status: 'running',
        detail: 'running timeline detail should stay collapsed',
        meta: { toolName: 'read' },
        filePath: '/tmp/project/src/app.ts'
      })
    ]
    useChatStore.setState({
      busy: true,
      currentTurnUserId: 'user_1',
      turnStartedAtByUserId: { user_1: Date.now() }
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('aria-expanded="true"')
    expect(html).toContain('Read')
    expect(html).toContain('/tmp/project/src/app.ts')
    expect(html).not.toContain('running timeline detail should stay collapsed')
  })

  it('renders Codex-style user-message navigation rail anchors for long conversations', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'first question' },
      { kind: 'assistant', id: 'assistant_1', text: 'first answer' },
      { kind: 'user', id: 'user_2', text: 'second question' },
      { kind: 'assistant', id: 'assistant_2', text: 'second answer' },
      { kind: 'user', id: 'user_3', text: 'third question' },
      { kind: 'assistant', id: 'assistant_3', text: 'third answer' },
      { kind: 'user', id: 'user_4', text: 'fourth question' },
      { kind: 'assistant', id: 'assistant_4', text: 'fourth answer' }
    ]

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('timeline-user-message-navigation-rail')
    expect(html).toContain('data-thread-user-message-navigation-item-id="user_1:user"')
    expect(html).toContain('data-content-search-unit-key="user_1:user"')
    expect(html).toContain('data-user-message-bubble="true"')
    expect(html).not.toContain('timeline-jump-rail-button')
  })

  it('expands live Go tool work before the final answer in the active turn', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'Read the README' },
      {
        kind: 'tool',
        id: 'tool_call_read',
        summary: 'Read README',
        status: 'running',
        meta: {
          callId: 'call_read',
          toolName: 'read',
          runtimeStatus: 'tool_call_started'
        }
      },
      { kind: 'assistant', id: 'assistant_1', text: 'The README says hello.' }
    ]
    useChatStore.setState({
      busy: true,
      currentTurnUserId: 'user_1',
      turnStartedAtByUserId: { user_1: Date.now() - 1200 },
      turnDurationByUserId: {},
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('Read README')
    expect(html).toContain('The README says hello.')
    expect(html.indexOf('Read README')).toBeLessThan(html.indexOf('The README says hello.'))
  })

  it('shows live assistant text in the normal answer flow while the turn is processing', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'Explain the plan' }
    ]
    useChatStore.setState({
      busy: true,
      currentTurnUserId: 'user_1',
      turnStartedAtByUserId: { user_1: Date.now() - 800 },
      turnDurationByUserId: {},
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: 'Here is the answer as it streams.',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).not.toContain('private reasoning')
    expect(html).toContain('Here is the answer as it streams.')
    expect(html.indexOf('Explain the plan')).toBeLessThan(
      html.indexOf('Here is the answer as it streams.')
    )
  })

  it('withholds all active-stream candidate bytes while retaining the progress shell', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'user_1',
        text: 'Explain the plan',
        meta: { turnId: 'turn_1' }
      }
    ]
    useChatStore.setState({
      busy: true,
      currentTurnId: 'turn_1',
      currentTurnUserId: 'user_1',
      turnStartedAtByUserId: { user_1: Date.now() - 800 },
      turnDurationByUserId: {},
    })
    resetActiveStream('thr_1', { turnId: 'turn_1' })
    appendActiveStreamDeltas({
      threadId: 'thr_1',
      turnId: 'turn_1',
      assistant: '<think>assistant think delta</think>Visible streamed answer.'
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).not.toContain('runtime reasoning delta')
    expect(html).not.toContain('assistant think delta')
    expect(html).not.toContain('Visible streamed answer.')
    expect(html).not.toContain('&lt;think&gt;')
    expect(html).not.toContain('data-analytix-live-assistant')
  })

  it('collapses completed Go tool work before the final answer by default', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'Read the config' },
      { kind: 'assistant', id: 'assistant_preface', text: 'I will inspect the file first.' },
      {
        kind: 'tool',
        id: 'tool_call_read',
        summary: 'Read config',
        status: 'success',
        detail: 'config file output',
        meta: {
          callId: 'call_read',
          toolName: 'read',
          runtimeStatus: 'tool_call_finished'
        }
      },
      { kind: 'assistant', id: 'assistant_final', text: 'The config enables the requested feature.' }
    ]
    useChatStore.setState({
      busy: false,
      currentTurnUserId: null,
      turnStartedAtByUserId: {},
      turnDurationByUserId: {}
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('Work process (2 steps)')
    expect(html).not.toContain('I will inspect the file first.')
    expect(html).not.toContain('Read config')
    expect(html).toContain('The config enables the requested feature.')
    expect(html.indexOf('Work process (2 steps)')).toBeLessThan(html.indexOf('The config enables the requested feature.'))
  })

  it('collapses completed failed tool work once the final answer is available', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'List the desktop' },
      {
        kind: 'tool',
        id: 'tool_ls',
        summary: 'List ~/Desktop',
        status: 'error',
        detail: 'tool failed',
        meta: {
          callId: 'call_ls',
          toolName: 'ls',
          runtimeStatus: 'tool_call_failed'
        }
      },
      { kind: 'assistant', id: 'assistant_final', text: 'I could not list that folder, but here is the next step.' }
    ]
    useChatStore.setState({
      busy: false,
      currentTurnUserId: null,
      turnStartedAtByUserId: {},
      turnDurationByUserId: {}
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('Work process (1 steps)')
    expect(html).not.toContain('tool failed')
    expect(html).toContain('I could not list that folder')
  })

  it('keeps safe runtime error metadata visible without rendering provider bodies', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'user_1',
        text: 'draw this'
      },
      {
        kind: 'system',
        id: 'error_1',
        text: 'model request failed with status 400',
        detail: [
          'Code: http_400',
          '',
          'Severity: error',
          '',
          'Details:',
          '{"status":400,"providerId":"deepseek"}'
        ].join('\n'),
        code: 'http_400',
        severity: 'error'
      }
    ]
    useChatStore.setState({
      busy: false,
      currentTurnUserId: null,
      turnStartedAtByUserId: {}
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('request failed with status 400')
    expect(html).toContain('Code: http_400')
    expect(html).toContain('providerId')
    expect(html).not.toContain('full provider body')
  })

  it('withholds child-thread candidate bytes through the shared timeline override', () => {
    clearActiveStream()
    appendActiveStreamDeltas({
      threadId: 'thr_child',
      turnId: 'turn_child_live',
      assistant: 'child inspector final answer',
      lastSeq: 17
    })
    useChatStore.setState({
      activeThreadId: 'thr_1',
      busy: false,
      currentTurnId: null,
      currentTurnUserId: null,
      turnStartedAtByUserId: {},
      turnDurationByUserId: {},
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks: [],
        live: '',
        activeThreadId: 'thr_child',
        runtimeConnection: 'ready',
        runtimeStateOverride: {
          busy: true,
          currentTurnId: 'turn_child_live',
          currentTurnUserId: null,
          turnStartedAtByUserId: {},
          turnDurationByUserId: {},
        },
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).not.toContain('child inspector thinking')
    expect(html).not.toContain('child inspector final answer')
    expect(html).not.toContain('data-analytix-live-process')
    expect(html).not.toContain('data-analytix-live-assistant')
    expect(html).toContain('data-analytix-live-progress')
  })

  it('keeps child-thread elapsed timing visible and process collapsed after re-entry', () => {
    clearActiveStream()
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'user_child_done',
        text: '继续审计子智能体线程',
        meta: { turnId: 'turn_child_done' }
      },
      {
        kind: 'tool',
        id: 'tool_child_done',
        createdAt: '2026-07-06T06:00:02.000Z',
        summary: '列出 src/renderer',
        status: 'success',
        toolKind: 'tool_call',
        meta: { turnId: 'turn_child_done', toolName: 'ls' }
      },
      {
        kind: 'assistant',
        id: 'assistant_child_done',
        createdAt: '2026-07-06T06:00:05.000Z',
        text: 'child persisted final answer',
        meta: { turnId: 'turn_child_done' }
      }
    ]

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_child',
        runtimeConnection: 'ready',
        runtimeStateOverride: {
          busy: false,
          currentTurnId: null,
          currentTurnUserId: null,
          turnStartedAtByUserId: {},
          turnDurationByUserId: { user_child_done: 12_000 }
        },
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toContain('child persisted final answer')
    expect(html).toContain('Processed 12s')
    expect(html).not.toContain('Thought for')
    expect(html).toContain('aria-expanded="false"')
    expect(html).not.toContain('列出 src/renderer')
  })

  it('adds extra bottom padding only for chat timelines with an active goal banner', () => {
    expect(goalTimelinePaddingClass('chat', true)).toBe('pb-32 md:pb-40')
    expect(goalTimelinePaddingClass('chat', false)).toBe('pb-10')
    expect(goalTimelinePaddingClass('claw', true)).toBe('pb-10')
  })

  it('pushes the live progress row above the goal banner when a goal is active', () => {
    expect(liveTurnProgressClass(true)).toContain('mb-16 md:mb-20')
    expect(liveTurnProgressClass(false)).not.toContain('mb-16 md:mb-20')
  })

  it('keeps the return-to-bottom affordance available after streaming completes', () => {
    expect(shouldShowReturnToBottomButton({ bottomDistance: 40, hasContent: true })).toBe(true)
    expect(shouldShowReturnToBottomButton({ bottomDistance: 12, hasContent: true })).toBe(false)
    expect(shouldShowReturnToBottomButton({ bottomDistance: 400, hasContent: false })).toBe(false)
  })

  it('does not show the return-to-bottom affordance while the latest response is still within the spacer', () => {
    expect(
      shouldShowReturnToBottomButton({
        bottomDistance: 90,
        hasContent: true,
        responseSpacerHeightPx: 72,
      })
    ).toBe(false)
    expect(
      shouldShowReturnToBottomButton({
        bottomDistance: 110,
        hasContent: true,
        responseSpacerHeightPx: 72,
      })
    ).toBe(true)
  })

  it('renders the workspace rollback action on completed assistant responses with a git checkpoint', () => {
    const blocks: ChatBlock[] = [
      {
        kind: 'user',
        id: 'user_1',
        text: 'change files',
        meta: { turnId: 'turn_1', workspaceCheckpointId: 'gcp_1' }
      },
      {
        kind: 'assistant',
        id: 'assistant_1',
        text: 'done'
      }
    ]
    useChatStore.setState({
      busy: false,
      currentTurnUserId: null,
      forkThreadFromTurn: async () => undefined,
      rollbackWorkspaceToCheckpoint: async () => undefined
    })

    const html = renderToStaticMarkup(
      createElement(MessageTimeline, {
        blocks,
        live: '',
        activeThreadId: 'thr_1',
        runtimeConnection: 'ready',
        onRetryConnection: () => undefined,
        onOpenSettings: () => undefined
      })
    )

    expect(html).toMatch(/rollbackWorkspace|Rollback commit|回滚提交/)
    expect(html).toMatch(/rollbackWorkspaceFromAssistantResponse|Rollback this response&#x27;s Git commit|只回滚这条回答对应的 Git 提交/)
    expect(html).toMatch(/forkResponse|Fork response|分叉回答/)
    expect(html).toMatch(/forkFromAssistantResponse|Fork the conversation from this response|从这条回答分叉会话/)
  })

  it('drops marker-free action response reason and error text', () => {
    const marker = 'SENTINEL_ACTION_REASON_4F2D'

    const rawReason = projectSubagentActionResponse(JSON.stringify({
      status: 'rejected',
      reason: marker,
      error: `${marker}:error`
    }))
    const unknownFields = projectSubagentActionResponse(JSON.stringify({
      status: 'invented',
      reasonCode: marker,
      conflictReportId: `${marker}/unsafe`
    }))
    const closedReason = projectSubagentActionResponse(JSON.stringify({
      status: 'rejected',
      code: 'conflict',
      reason: marker
    }))

    expect(rawReason).toEqual({ status: 'rejected', reasonCode: 'unknown' })
    expect(unknownFields).toEqual({ reasonCode: 'unknown' })
    expect(closedReason).toEqual({ status: 'rejected', reasonCode: 'conflict' })
    expect(JSON.stringify([rawReason, unknownFields, closedReason])).not.toContain(marker)
  })
})
