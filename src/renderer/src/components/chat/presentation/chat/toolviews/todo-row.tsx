/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import { DshChecklistOutlineRegular } from '../../DshToolIcons'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import type { ToolViewProps } from '../ToolViews'
import { ToolRow } from '../ToolRow'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import { toolRowState } from '../models/tool-call-model'
import { todoCardModel } from '../models/todo-card-model'
import { ToolDetails } from './ToolDetails'
import css from '../ToolRow.module.css'

/** A successful immutable Todo capture owns its diff; failed/stopped calls only
 * show their original authorized IN, inside the same mounted protected lease. */
export function TodoRow({ block, visible, resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common')
  if (block.toolKind !== 'tool_call' || block.meta?.toolName !== 'todo_write') return null
  const state = toolRowState(block)
  const rowProps = { title: t('dshRecordedTool.todo.title'), toolName: 'todo_write', variant: 'todo',
    icon: <DshChecklistOutlineRegular /> }
  const actions = <RuntimeDiagnosticsAction block={block} t={t} />
  return <div data-recorded-domain-row="todo" data-tool-view="todo" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && state !== 'running' ? <ProtectedToolResult preload
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
      renderContent={display => {
        if (display.capture.kind === 'todo') return <ToolDetails model={todoCardModel(display.capture.todo, t)} hasInspect />
        if (display.capture.kind !== 'inspect' || !['failed', 'timeout', 'canceled'].includes(display.capture.status)) return null
        // Preserve authorized input verbatim; JSON parsing rounds large integers.
        const input = display.inspect?.inputJSON
        return <div className={css.ioCard} style={{ margin: 0 }} data-todo-input data-protected-tool-readonly>
          <div className={css.ioSection}><span className={css.ioLabel}>IN</span><span className={css.ioText}>
            {input ?? t(display.inspect?.inputTruncated ? 'dshToolInspect.captureLimit' : 'dshToolInspect.unavailable')}
          </span></div>
        </div>
      }} renderView={controls => {
        const capture = controls.display?.capture
        const model = capture?.kind === 'todo' ? todoCardModel(capture.todo, t) : null
        const failure = capture?.kind === 'inspect' && ['failed', 'timeout', 'canceled'].includes(capture.status)
        return <ToolRow {...rowProps} state={controls.state ?? state}
          summary={model ? <span data-protected-tool-summary>{model.summary}</span>
            : failure ? t(capture.status === 'canceled' ? 'dshTurnStopped' : 'protectedToolFailed') : t('protectedToolUnavailable')}
          summarySuffix={model?.expandedSummary ? <span data-protected-tool-summary>{model.expandedSummary}</span> : undefined}
          expanded={controls.open} expandable={controls.canOpen} onToggle={controls.onToggle}
          body={controls.body ? { kind: 'details', content: controls.body } : undefined}
          inspect={controls.inspect}
          actions={<>{actions}{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />
      }} /> : <ToolRow {...rowProps} state={state} summary={visible && state !== 'running' ? t('protectedToolUnavailable') : undefined} actions={actions}
        expanded={visible && capture?.expanded} expandable={visible && capture?.expandable}
        onToggle={() => capture?.setOpen(!capture.expanded)} body={visible && capture?.expanded ? { kind: 'details', content: capture.body } : undefined} />}
  </div>
}
