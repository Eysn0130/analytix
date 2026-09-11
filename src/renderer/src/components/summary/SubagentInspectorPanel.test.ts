import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import type { CoreThreadSummarySubagentJson } from '../../agent/analytix-contract'
import i18n from '../../i18n'
import { SubagentInspectorPanel } from './SubagentInspectorPanel'

const updatedAt = '2026-06-29T00:00:00.000Z'

function subagent(
  patch: Partial<CoreThreadSummarySubagentJson> = {}
): CoreThreadSummarySubagentJson {
  const id = patch.id ?? 'run:run_1'
  return {
    schemaVersion: 1,
    id,
    key: patch.key ?? id,
    parentThreadId: 'thr_parent',
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
    updatedAt,
    ...patch
  }
}

describe('SubagentInspectorPanel', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh')
  })

  it('renders subagent tabs with the compact composer and without side-conversation affordances', () => {
    const html = renderToStaticMarkup(createElement(SubagentInspectorPanel, {
      subagents: [
        subagent({
          id: 'run:run_1',
          parentThreadId: 'thr_parent',
          childRunId: 'run_1',
          childThreadId: 'thr_child_1',
          canOpenThread: true,
          title: 'Lagrange',
          model: 'gpt-test',
          providerId: 'anthropic-main',
          endpointFormat: 'messages',
          variant: 'sonnet:202606',
          modelSource: 'thread',
          profile: 'research',
          status: 'active',
          rawStatus: 'running',
          totalTokens: 1200,
          cacheHitRate: 0.5,
          updatedAt
        }),
        subagent({
          id: 'run:run_2',
          parentThreadId: 'thr_parent',
          childRunId: 'run_2',
          childThreadId: 'thr_child_2',
          canOpenThread: true,
          title: 'Nash',
          updatedAt: '2026-06-29T00:00:01.000Z'
        }),
        subagent({
          id: 'run:run_3',
          parentThreadId: 'thr_parent',
          childRunId: 'run_3',
          childThreadId: 'thr_child_3',
          canOpenThread: true,
          title: 'task',
          label: 'task',
          profile: 'source-audit',
          updatedAt: '2026-06-29T00:00:02.000Z'
        })
      ],
      selectedKey: 'run:run_1',
      runtimeConnection: 'ready',
      composerModel: 'gpt-test',
      composerProviderId: 'test-provider',
      composerPickList: ['gpt-test'],
      composerModelGroups: [],
      composerReasoningEffort: 'medium',
      setComposerModel: () => {},
      setComposerReasoningEffort: () => {},
      onSelectSubagent: () => {},
      onCollapse: () => {},
      onRetryConnection: () => {},
      onOpenSettings: () => {},
      onConfigureProviders: () => {}
    }))

    expect(html).toContain('data-subagent-inspector-panel="true"')
    expect(html).toContain('role="tablist"')
    expect(html).toContain('aria-selected="true"')
    expect(html).toContain('Lagrange')
    expect(html).toContain('Nash')
    expect(html).toContain('Noether')
    expect(html).not.toContain('source-audit')
    expect(html).not.toContain('>task<')
    expect(html).toContain('shape-rendering="crispEdges"')
    expect(html).toContain('模型')
    expect(html).toContain('gpt-test')
    expect(html).toContain('anthropic-main')
    expect(html).toContain('messages')
    expect(html).toContain('sonnet:202606')
    expect(html).toContain('thread')
    expect(html).not.toContain('目标证据已记录')
    expect(html).toContain('ds-floating-composer')
    expect(html).toContain('data-composer-prompt-editor="true"')
    expect(html).not.toContain('data-above-composer-portal')
    expect(html).not.toContain('写作助手待命中')
    expect(html).not.toContain('打开旁支对话')
  })

  it('keeps failed child lifecycle visible in the inspector chrome and metadata', () => {
    const html = renderToStaticMarkup(createElement(SubagentInspectorPanel, {
      subagents: [
        subagent({
          id: 'run:run_failed',
          parentThreadId: 'thr_parent',
          childRunId: 'run_failed',
          childThreadId: 'thr_child_failed',
          canOpenThread: true,
          title: 'Verifier',
          status: 'terminal',
          rawStatus: 'failed',
          model: 'gpt-test',
          providerId: 'openai-main',
          endpointFormat: 'responses',
          modelSource: 'thread',
          updatedAt: '2026-06-29T00:00:03.000Z'
        })
      ],
      selectedKey: 'run:run_failed',
      runtimeConnection: 'ready',
      composerModel: 'gpt-test',
      composerProviderId: 'test-provider',
      composerPickList: ['gpt-test'],
      composerModelGroups: [],
      composerReasoningEffort: 'medium',
      setComposerModel: () => {},
      setComposerReasoningEffort: () => {},
      onSelectSubagent: () => {},
      onCollapse: () => {},
      onRetryConnection: () => {},
      onOpenSettings: () => {},
      onConfigureProviders: () => {}
    }))

    expect(html).toContain('Verifier')
    expect(html).toContain('失败')
    expect(html).toContain('状态')
    expect(html).not.toContain('provider stream failed after partial output')
    expect(html).toContain('openai-main')
    expect(html).toContain('responses')
    expect(html).not.toContain('已结束')
    expect(html).not.toContain('运行中')
  })
})
