import { createElement, type ComponentProps } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import type {
  CoreThreadSummaryResponseJson,
  CoreThreadSummarySubagentJson,
  CoreThreadSummaryTaskJson
} from '../../agent/analytix-contract'
import { WorkbenchTopBar } from '../chat/WorkbenchTopBar'
import {
  ThreadSummaryPanelView,
  isThreadSummaryPublicationBlocked,
  shouldRefreshThreadSummaryForSeq
} from './ThreadSummaryPanel'
import type { ChatBlock } from '../../agent/types'
import { buildRuntimeDiagnosticsEntries } from './RuntimeDiagnosticsPanel'
import { subagentDisplayLabel } from './SubagentSummaryRows'
import { useChatStore } from '../../store/chat-store'

const generatedAt = '2026-06-29T00:00:00.000Z'

type TaskJobSummary = Extract<
  CoreThreadSummaryTaskJson,
  { outputTrustStatus: 'untrusted_child_output' }
>

type CommandSummary = Extract<
  CoreThreadSummaryTaskJson,
  { outputTrustStatus: 'private_tool_output' }
>

function subagent(
  patch: Partial<CoreThreadSummarySubagentJson> = {}
): CoreThreadSummarySubagentJson {
  const id = patch.id ?? 'run:run_1'
  return {
    schemaVersion: 1,
    id,
    key: patch.key ?? id,
    parentThreadId: 'thr_1',
    status: 'done',
    rawStatus: 'completed',
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canContinueParent: false,
    canReadOutput: false,
    canOpenThread: false,
    canKill: false,
    canRestart: false,
    updatedAt: generatedAt,
    ...patch
  }
}

function taskJob(patch: Partial<TaskJobSummary> = {}): TaskJobSummary {
  return {
    schemaVersion: 1,
    id: 'taskjob:job_1',
    kind: 'task',
    status: 'completed',
    background: false,
    active: false,
    terminal: true,
    outputWithheld: true,
    outputTrustStatus: 'untrusted_child_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false,
    ...patch
  }
}

function commandTask(patch: Partial<CommandSummary> = {}): CommandSummary {
  return {
    schemaVersion: 1,
    id: 'command:call_1',
    kind: 'command',
    status: 'completed',
    background: false,
    active: false,
    terminal: true,
    outputWithheld: true,
    outputTrustStatus: 'private_tool_output',
    factAnswerAllowed: false,
    evidenceAuthority: false,
    canReadOutput: false,
    canContinueParent: false,
    ...patch
  }
}

function summary(patch: Partial<CoreThreadSummaryResponseJson> = {}): CoreThreadSummaryResponseJson {
  return {
    threadId: 'thr_1',
    generatedAt,
    latestSeq: 7,
    subagents: [],
    tasks: [],
    outputs: [],
    sources: [],
    sideChats: [],
    backgroundProcesses: [],
    ...patch
  }
}

function renderSummaryView(
  props: Partial<ComponentProps<typeof ThreadSummaryPanelView>> = {}
): string {
  const baseProps: ComponentProps<typeof ThreadSummaryPanelView> = {
    state: { status: 'ready', data: summary(), error: null },
    busyTaskId: null,
    outputState: { status: 'closed' },
    onCollapse: () => {},
    onCloseOutput: () => {},
    onOpenThread: () => {}
  }
  return renderToStaticMarkup(createElement(ThreadSummaryPanelView, { ...baseProps, ...props }))
}

describe('ThreadSummaryPanelView', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh')
  })

  afterEach(() => {
    useChatStore.setState({ threads: [] })
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('does not render marker-free arbitrary task output even if a hostile caller injects a legacy state', () => {
    const privateOutputSentinel = 'SOL_PRIVATE_REASONING_SENTINEL_7F3C'
    const html = renderSummaryView({
      outputState: {
        status: 'ready',
        title: 'legacy task output',
        output: privateOutputSentinel
      } as unknown as ComponentProps<typeof ThreadSummaryPanelView>['outputState']
    })

    expect(html).not.toContain(privateOutputSentinel)
  })

  it('keeps command-task summaries metadata-only', () => {
    const data = summary({
      tasks: [commandTask({
        status: 'running',
        active: true,
        terminal: false
      })]
    })

    const html = renderSummaryView({ state: { status: 'ready', data, error: null } })
    expect(html).toContain('Command')
    expect(html).toContain('command:call_1 · 输出已隔离')
    expect(html).not.toContain('npm run long')
    expect(html).not.toContain('aria-label="打开输出"')
    expect(html).not.toContain('aria-label="停止任务"')
    expect(html).not.toContain('aria-label="重新启动"')
  })
  it('refreshes the open summary panel when the active thread receives new runtime events', () => {
    const marker = { threadId: null, seq: 0 }

    expect(shouldRefreshThreadSummaryForSeq(marker, 'thr_1', 7)).toBe(false)
    expect(marker).toEqual({ threadId: 'thr_1', seq: 7 })
    expect(shouldRefreshThreadSummaryForSeq(marker, 'thr_1', 7)).toBe(false)
    expect(shouldRefreshThreadSummaryForSeq(marker, 'thr_1', 8)).toBe(true)
    expect(marker).toEqual({ threadId: 'thr_1', seq: 8 })
    expect(shouldRefreshThreadSummaryForSeq(marker, 'thr_2', 2)).toBe(false)
    expect(marker).toEqual({ threadId: 'thr_2', seq: 2 })
    expect(shouldRefreshThreadSummaryForSeq(marker, null, 9)).toBe(false)
    expect(marker).toEqual({ threadId: null, seq: 0 })
  })

  it('trusts runtime subagent display labels including recovered prompts and profiles', () => {
    expect(subagentDisplayLabel(subagent({
      id: 'run:run_1',
      parentThreadId: 'thr_1',
      childId: 'run_1',
      childRunId: 'run_1',
      displayName: '基于目录结构和媒体文件信息生成摘要',
      title: '基于目录结构和媒体文件信息生成摘要',
      label: '基于目录结构和媒体文件信息生成摘要',
      updatedAt: generatedAt
    }))).toBe('基于目录结构和媒体文件信息生成摘要')
    expect(subagentDisplayLabel(subagent({
      id: 'run:run_2',
      parentThreadId: 'thr_1',
      childId: 'run_2',
      childRunId: 'run_2',
      displayName: 'task',
      agentNickname: 'Nash',
      label: 'task',
      status: 'active',
      rawStatus: 'running',
      updatedAt: generatedAt
    }))).toBe('Nash')
    expect(subagentDisplayLabel(subagent({
      id: 'run:run_3',
      parentThreadId: 'thr_1',
      childId: 'run_3',
      childRunId: 'run_3',
      displayName: 'reviewer',
      agentNickname: 'reviewer',
      title: '分析桌面文件',
      label: '分析桌面文件',
      profile: 'reviewer',
      updatedAt: generatedAt
    }))).toBe('reviewer')
    expect(subagentDisplayLabel(subagent({
      id: 'run:job-16',
      parentThreadId: 'thr_1',
      childId: 'job-16',
      childRunId: 'job-16',
      displayName: 'design-reviewer',
      agentNickname: 'design-reviewer',
      title: 'Child agent: design-reviewer',
      label: 'task',
      profile: 'design-reviewer',
      parallelIndex: 1,
      updatedAt: generatedAt
    }))).toBe('design-reviewer')
  })

  it('renders terminal subagent lifecycle states in the pinned summary', () => {
    const html = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary({
          subagents: [
            subagent({
              id: 'run:run_failed',
              parentThreadId: 'thr_1',
              childRunId: 'run_failed',
              childThreadId: 'thr_failed',
              title: 'Verifier',
              status: 'terminal',
              rawStatus: 'failed',
              canOpenThread: true,
              updatedAt: generatedAt
            }),
            subagent({
              id: 'run:run_killed',
              parentThreadId: 'thr_1',
              childRunId: 'run_killed',
              childThreadId: 'thr_killed',
              title: 'Runner',
              status: 'terminal',
              rawStatus: 'killed',
              canOpenThread: true,
              updatedAt: generatedAt
            }),
            subagent({
              id: 'run:run_interrupted',
              parentThreadId: 'thr_1',
              childRunId: 'run_interrupted',
              childThreadId: 'thr_interrupted',
              title: 'Researcher',
              status: 'terminal',
              rawStatus: 'interrupted',
              canOpenThread: true,
              updatedAt: generatedAt
            })
          ]
        })
      }
    })

    expect(html).toContain('Verifier')
    expect(html).toContain('失败')
    expect(html).not.toContain('provider returned a terminal error')
    expect(html).toContain('Runner')
    expect(html).toContain('已停止')
    expect(html).not.toContain('operator stopped the child run')
    expect(html).toContain('Researcher')
    expect(html).toContain('已中断')
    expect(html).not.toContain('parent turn was interrupted')
    expect(html).not.toContain('ds-summary-shimmer-text')
  })

  it('renders the case data overview with compact Chinese units', () => {
    const html = renderSummaryView({
      caseOverview: {
        status: 'ready',
        caseId: 'case_1',
        error: null,
        data: {
          case_id: 'case_1',
          fact_answer_allowed: true,
          account_status: 'verified',
          account_blocker: '',
          account_count: 123456,
          personal_account_count: 12,
          corporate_account_count: 3,
          unknown_account_count: 1,
          transaction_status: 'verified',
          transaction_blocker: '',
          transaction_count: 123456,
          amount_status: 'verified',
          amount_blocker: '',
          inflow_amount: 123456789,
          outflow_amount: 9876543.21,
          amount_total_rows: 123456,
          amount_present_rows: 123456,
          amount_missing_rows: 0,
          amount_parse_failed_rows: 0,
          direction_covered_rows: 123456,
          source_table: 'analysis_txn_daily_agg',
          account_source_table: 'analysis_account_dim',
          source_revision: 2,
          generated_at: generatedAt
        }
      }
    })

    expect(html).toContain('案件数据概览')
    expect(html).toContain('12.35万个')
    expect(html).toContain('个人 12个 · 对公 3个 · 未识别 1个')
    expect(html).toContain('12.35万条')
    expect(html).toContain('1.23亿元')
    expect(html).toContain('987.65万元')
    expect(html).toContain('未识别 1个')
    expect(html).not.toContain('刷新摘要')
  })

  it('never renders unavailable or partial case metrics as zero facts', () => {
    const partial = renderSummaryView({
      caseOverview: {
        status: 'ready',
        caseId: 'case_partial',
        error: null,
        data: {
          case_id: 'case_partial',
          fact_answer_allowed: false,
          account_status: 'unavailable',
          account_blocker: 'account_source_unavailable',
          account_count: null,
          personal_account_count: null,
          corporate_account_count: null,
          unknown_account_count: null,
          transaction_status: 'verified',
          transaction_blocker: '',
          transaction_count: 2,
          amount_status: 'partial',
          amount_blocker: 'amount_or_direction_coverage_partial',
          inflow_amount: null,
          outflow_amount: null,
          amount_total_rows: 2,
          amount_present_rows: 1,
          amount_missing_rows: 1,
          amount_parse_failed_rows: 0,
          direction_covered_rows: 2,
          source_table: 'analysis_txn_detail_idx',
          account_source_table: '',
          source_revision: 2,
          generated_at: generatedAt
        }
      }
    })

    expect(partial).toContain('账户统计不可用')
    expect(partial).toContain('交易统计不可用')
    expect(partial).toContain('金额统计不可用')
    expect(partial).not.toContain('主端交易明细 2条')
    expect(partial).not.toContain('金额可用 1 / 2 条')
    expect(partial).not.toContain('主端进账资金 0元')
    expect(partial).not.toContain('主端出账资金 0元')

    const unavailable = renderSummaryView({
      caseOverview: {
        status: 'ready',
        caseId: 'case_unavailable',
        error: null,
        data: {
          case_id: 'case_unavailable',
          fact_answer_allowed: false,
          account_status: 'unavailable',
          account_blocker: 'materialization_unavailable',
          account_count: null,
          personal_account_count: null,
          corporate_account_count: null,
          unknown_account_count: null,
          transaction_status: 'unavailable',
          transaction_blocker: 'materialization_unavailable',
          transaction_count: null,
          amount_status: 'unavailable',
          amount_blocker: 'materialization_unavailable',
          inflow_amount: null,
          outflow_amount: null,
          amount_total_rows: null,
          amount_present_rows: null,
          amount_missing_rows: null,
          amount_parse_failed_rows: null,
          direction_covered_rows: null,
          source_table: '',
          account_source_table: '',
          source_revision: 1,
          generated_at: generatedAt
        }
      }
    })

    expect(unavailable).toContain('交易统计不可用')
    expect(unavailable).toContain('金额统计不可用')
    expect(unavailable).not.toContain('主端交易明细 0条')
    expect(unavailable).not.toContain('主端进账资金 0元')
  })

  it('shows preheated case overview while the thread summary is still loading', () => {
    const html = renderSummaryView({
      state: { status: 'loading', data: null, error: null },
      caseOverview: {
        status: 'ready',
        caseId: 'case_1',
        error: null,
        data: {
          case_id: 'case_1',
          fact_answer_allowed: true,
          account_status: 'verified',
          account_blocker: '',
          account_count: 280,
          personal_account_count: 21,
          corporate_account_count: 259,
          unknown_account_count: 0,
          transaction_status: 'verified',
          transaction_blocker: '',
          transaction_count: 2645472,
          amount_status: 'verified',
          amount_blocker: '',
          inflow_amount: 150267349928.16,
          outflow_amount: 150791804127.6,
          amount_total_rows: 2645472,
          amount_present_rows: 2645472,
          amount_missing_rows: 0,
          amount_parse_failed_rows: 0,
          direction_covered_rows: 2645472,
          source_table: 'analysis_txn_daily_agg',
          account_source_table: 'analysis_account_dim',
          source_revision: 5,
          generated_at: generatedAt
        }
      }
    })

    expect(html).toContain('案件数据概览')
    expect(html).toContain('账户数')
    expect(html).toContain('264.55万条')
    expect(html).toContain('1,502.67亿元')
    expect(html).not.toContain('正在加载摘要')
  })

  it('renders loading, empty, running, done, and error states', () => {
    const loading = renderSummaryView({
      state: { status: 'loading', data: null, error: null }
    })
    expect(loading).toContain('正在加载摘要')

    const empty = renderSummaryView()
    expect(empty).toContain('来源')
    expect(empty).toContain('暂无来源')

    const running = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary({
          subagents: [subagent({
            id: 'run:run_1',
            parentThreadId: 'thr_1',
            parentTurnId: 'turn_1',
            childId: 'child_1',
            childRunId: 'run_1',
            taskJobId: 'run_1',
            taskKind: 'subagent',
            childThreadId: 'thr_child',
            title: 'Research child',
            model: 'claude-sonnet-4',
            providerId: 'anthropic',
            endpointFormat: 'messages',
            variant: 'fast',
            modelSource: 'subagent-profile',
            status: 'active',
            rawStatus: 'running',
            cacheHitRate: 0.5,
            totalTokens: 12345,
            canOpenThread: true,
            updatedAt: generatedAt
          })],
          tasks: [taskJob({
            id: 'taskjob:job_1',
            status: 'running',
            active: true,
            terminal: false
          })]
        })
      },
      busyTaskId: 'taskjob:run_1'
    })
    expect(running).toContain('Research child')
    expect(running).not.toContain('目标证据已记录')
    expect(running).toContain('缓存命中 50%')
    expect(running).toContain('tokens 12.3k')
    expect(running).toContain('可打开 thr_child')
    expect(running).toContain('anthropic')
    expect(running).toContain('claude-sonnet-4')
    expect(running).toContain('messages')
    expect(running).toContain('subagent-profile')
    expect(running).toContain('taskjob:job_1 · 输出已隔离')
    expect(running).not.toContain('child sample: parallel a')
    expect(running).toContain('运行中')
    expect(running.match(/aria-label="打开子线程"/g) ?? []).toHaveLength(1)
    expect(running).not.toContain('aria-label="打开输出"')
    expect(running).not.toContain('aria-label="停止任务"')
    expect(running).not.toContain('aria-label="停止子智能体"')
    expect(running).not.toContain('aria-label="重新启动"')
    expect(running).toContain('shape-rendering="crispEdges"')
    expect(running).toContain('ds-agent-identicon-filled-scan')
    expect(running).toContain('ds-agent-identicon-empty-scan')
    expect(running).toContain('animate-spin')
    expect(running).not.toContain('运行时诊断')
    expect(running).toContain('后台任务')

    const done = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary({
          tasks: [taskJob({
            id: 'taskjob:job_2',
            status: 'completed',
            active: false,
            terminal: true
          })]
        })
      }
    })
    expect(done).toContain('后台任务')
    expect(done).toContain('aria-expanded="true"')
    expect(done).not.toContain('animate-spin')

    const stoppedCommand = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary({
          tasks: [commandTask({
            id: 'command:call_restart',
            status: 'stopped',
            active: false,
            terminal: false
          })]
        })
      }
    })
    expect(stoppedCommand).toContain('Command')
    expect(stoppedCommand).toContain('command:call_restart · 输出已隔离')
    expect(stoppedCommand).not.toContain('printf restarted')
    expect(stoppedCommand).not.toContain('aria-label="打开输出"')
    expect(stoppedCommand).not.toContain('aria-label="重新启动"')
    expect(stoppedCommand).not.toContain('aria-label="停止任务"')

    const error = renderSummaryView({
      state: { status: 'error', data: summary(), error: 'summary exploded' }
    })
    expect(error).toContain('summary exploded')
  })

  it('centralizes runtime diagnostics from summary jobs and timeline delivery ledger rows', () => {
    const deliveryBlock: ChatBlock = {
      kind: 'tool',
      id: 'tool_background_delivery_dead_letter_delivery_turn_1_job_1',
      createdAt: '2026-06-29T00:00:05.000Z',
      summary: 'Background result delivery dead-letter',
      status: 'error',
      meta: {
        diagnostics: {
          notificationKind: 'background_job_delivery',
          jobId: 'job_1',
          childThreadId: 'thr_child',
          childTurnId: 'turn_child',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          deliveryId: 'delivery_turn_parent_job_1',
          deliveryStatus: 'dead_letter',
          deliveryError: 'parent thread missing',
          deadLetterReason: 'parent_thread_missing',
          completionDeliveryAttempt: 3
        }
      }
    }
    const autoContinueBlock: ChatBlock = {
      kind: 'tool',
      id: 'tool_background_auto_continue_skipped_job_1',
      createdAt: '2026-06-29T00:00:06.000Z',
      summary: 'Background auto-continue skipped',
      status: 'success',
      meta: {
        diagnostics: {
          notificationKind: 'background_job_auto_continue',
          jobId: 'job_1',
          parentThreadId: 'thr_parent',
          parentTurnId: 'turn_parent',
          deliveryStatus: 'delivered',
          autoContinueStatus: 'skipped',
          autoContinueReason: 'parent_turn_not_latest'
        }
      }
    }
    const entries = buildRuntimeDiagnosticsEntries(summary({
      subagents: [subagent({
        id: 'run:job_1',
        parentThreadId: 'thr_1',
        parentTurnId: 'turn_parent',
        childRunId: 'job_1',
        taskJobId: 'job_1',
        childThreadId: 'thr_child',
        childTurnId: 'turn_child',
        title: 'Ledger child',
        status: 'terminal',
        rawStatus: 'failed',
        diagnostics: {
          status: 'failed',
          terminal: true,
          background: false,
          paused: false,
          updatedAt: '2026-06-29T00:00:04.000Z'
        },
        canOpenThread: true,
        updatedAt: '2026-06-29T00:00:04.000Z'
      })]
    }), [deliveryBlock, autoContinueBlock])

    expect(entries.map((entry) => entry.key)).toEqual([
      'timeline:auto:job_1:skipped:',
      'timeline:delivery:delivery_turn_parent_job_1:dead_letter',
      'subagent:run:job_1'
    ])
    expect(entries[0]?.statusLabel).toBe('自动续写 已跳过')

    const defaultHtml = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary()
      },
      runtimeDiagnostics: entries
    })
    expect(defaultHtml).not.toContain('运行时诊断')

    const html = renderSummaryView({
      state: {
        status: 'ready',
        error: null,
        data: summary({
          subagents: [subagent({
            id: 'run:job_1',
            parentThreadId: 'thr_1',
            parentTurnId: 'turn_parent',
            childRunId: 'job_1',
            taskJobId: 'job_1',
            childThreadId: 'thr_child',
            childTurnId: 'turn_child',
            title: 'Ledger child',
            status: 'terminal',
            rawStatus: 'failed',
            diagnostics: {
              status: 'failed',
              terminal: true,
              background: false,
              paused: false,
              updatedAt: '2026-06-29T00:00:04.000Z'
            },
            canOpenThread: true,
            updatedAt: '2026-06-29T00:00:04.000Z'
          })]
        })
      },
      runtimeDiagnostics: entries,
      diagnosticsFocus: { status: 'all' }
    })

    expect(html).not.toContain('运行时诊断')
    expect(html).not.toContain('后台自动续写 job_1')
    expect(html).not.toContain('后台结果投递 job_1')
    expect(html).not.toContain('delivery_turn_parent_job_1')
    expect(html).toContain('Ledger child')
    expect(html).toContain('失败')
    expect(html).toContain('打开子线程')
    expect(html).not.toContain('打开父线程')
  })

  it('does not render runtime diagnostics in the pinned summary even when focused', () => {
    const entries = buildRuntimeDiagnosticsEntries(summary({
      subagents: [
        subagent({
          id: 'run:job_running',
          parentThreadId: 'thr_1',
          parentTurnId: 'turn_parent',
          childRunId: 'job_running',
          taskJobId: 'job_running',
          childThreadId: 'thr_child_running',
          title: 'Running child',
          status: 'active',
          rawStatus: 'running',
          canOpenThread: true,
          updatedAt: '2026-06-29T00:00:05.000Z'
        }),
        subagent({
          id: 'run:job_failed',
          parentThreadId: 'thr_1',
          parentTurnId: 'turn_parent',
          childRunId: 'job_failed',
          taskJobId: 'job_failed',
          childThreadId: 'thr_child_failed',
          title: 'Failed child',
          status: 'terminal',
          rawStatus: 'failed',
          canOpenThread: true,
          updatedAt: '2026-06-29T00:00:04.000Z'
        }),
        subagent({
          id: 'run:job_paused',
          parentThreadId: 'thr_1',
          childRunId: 'job_paused',
          taskJobId: 'job_paused',
          childThreadId: 'thr_child_paused',
          title: 'Paused child',
          status: 'active',
          rawStatus: 'paused',
          canOpenThread: true,
          updatedAt: '2026-06-29T00:00:03.000Z'
        }),
        subagent({
          id: 'run:job_done',
          parentThreadId: 'thr_1',
          childRunId: 'job_done',
          taskJobId: 'job_done',
          title: 'Done child',
          updatedAt: '2026-06-29T00:00:02.000Z'
        })
      ]
    }), [])

    const html = renderSummaryView({
      state: { status: 'ready', error: null, data: summary() },
      runtimeDiagnostics: entries,
      diagnosticsFocus: { status: 'all' },
      onRuntimeDiagnosticAction: () => {}
    })

    expect(entries).toHaveLength(4)
    expect(html).not.toContain('运行时诊断')
    expect(html).not.toContain('stale / 可能停滞')
    expect(html).not.toContain('paused / 已暂停')
    expect(html).not.toContain('aria-label="暂停"')
    expect(html).not.toContain('aria-label="追加任务"')
    expect(html).not.toContain('aria-label="输出"')
    expect(html).not.toContain('aria-label="恢复运行"')
    expect(html).not.toContain('aria-label="恢复"')
    expect(html).not.toContain('aria-label="重启"')
  })
})

describe('WorkbenchTopBar summary toggle', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh')
  })

  it('renders the top-right Toggle Summary action with the active pressed state', () => {
    const html = renderToStaticMarkup(createElement(WorkbenchTopBar, {
      rightPanelMode: 'summary',
      onToggleRightPanelMode: () => {}
    }))

    expect(html).toContain('切换置顶摘要')
    expect(html).toMatch(/aria-label="切换置顶摘要"[^>]*aria-pressed="true"/)
  })
})
