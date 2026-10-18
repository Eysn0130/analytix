/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useId, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { DshChecklistOutlineRegular, DshGoalOutlineRegular } from '../../DshToolIcons'
import { ProtectedToolResult, useRecordedToolInspectorSelection } from '../../ProtectedToolResult'
import type { ToolViewProps } from '../ToolViews'
import { ToolRow } from '../ToolRow'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import { useDisclosure } from '../use-disclosure'
import { hasPublicFocus } from '../searchable-hidden'
import { toolRowState } from '../models/tool-call-model'
import { detailsCardModel } from '../models/details-card-model'
import { goalCardModel, protectedJobCardModel, recordedJobCardModel, recordedJobData, recordedJobNames, type RecordedJobName } from '../models/recorded-tool-card-model'
import css from '../ToolRow.module.css'
import { todoCardModel } from '../models/todo-card-model'
import { ToolDetails } from './ToolDetails'
import { ToolResultInspector } from './ToolResultInspector'

export const goalToolNames = ['create_goal', 'get_goal', 'update_goal'] as const
/** Same DSH entity-list row; its typed observation belongs to the protected lease. */
export function RecordedDomainRow({ domain, block, visible, resetKey, privateGeneration, capture, onInspect }: ToolViewProps & { domain: 'goal' | 'todo' }) {
  const { t } = useTranslation('common')
  const name = typeof block.meta?.toolName === 'string' ? block.meta.toolName : ''
  if (block.toolKind !== 'tool_call' || (domain === 'todo' ? name !== 'todo_write' : !goalToolNames.some(value => value === name))) return null
  const state = toolRowState(block)
  const rowProps = { title: t(domain === 'goal' ? 'dshRecordedTool.goal.' + name + 'Title' : 'dshRecordedTool.todo.title'), toolName: String(name), variant: domain,
    icon: domain === 'goal' ? <DshGoalOutlineRegular /> : <DshChecklistOutlineRegular /> }
  const actions = <RuntimeDiagnosticsAction block={block} t={t} />
  return <div data-recorded-domain-row={domain} data-tool-view={domain} data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && state !== 'running' ? <ProtectedToolResult preload
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
      renderContent={display => {
        const capture = display.capture
        const model = domain === 'goal' && capture.kind === 'goal' ? goalCardModel(capture.goal, t)
          : domain === 'todo' && capture.kind === 'todo' ? todoCardModel(capture.todo, t) : null
        return model ? <ToolDetails model={model} hasInspect /> : null
      }} renderView={controls => {
        const capture = controls.display?.capture
        const model = domain === 'goal' && capture?.kind === 'goal' ? goalCardModel(capture.goal, t)
          : domain === 'todo' && capture?.kind === 'todo' ? todoCardModel(capture.todo, t) : null
        return <ToolRow {...rowProps} state={controls.state ?? state}
          summary={model ? <span data-protected-tool-summary>{model.summary}</span> : t('protectedToolUnavailable')}
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
export const GoalRow = (props: ToolViewProps) => <RecordedDomainRow {...props} domain="goal" />

/** A receipt reports only its returned Job snapshot, never a guessed operation outcome. */
export function RecordedJobRow({ block, visible, resetKey, publicVisible = visible, publicResetKey = resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common')
  const id = useId()
  const name = block.meta?.toolName
  const data = recordedJobData(block)
  const state = toolRowState(block)
  const runningCapture = state === 'running' && visible ? capture : undefined
  const privateResultVisible = visible && Boolean(block.meta?.localResult) && state !== 'running'
  const publicRoot = useRef<HTMLDivElement>(null), publicReset = useRef(publicResetKey)
  const publicFocused = Boolean(data) && hasPublicFocus(publicRoot.current)
  if (!publicFocused) publicReset.current = publicResetKey
  const shown = data ? publicVisible || publicFocused : visible
  const disclosureKey = data ? publicReset.current : resetKey + ':' + privateGeneration
  const disclosure = useDisclosure(block.id + ':' + disclosureKey)
  const turnId = typeof block.meta?.turnId === 'string' ? block.meta.turnId : ''
  const callId = typeof block.meta?.callId === 'string' ? block.meta.callId : ''
  const resultItemId = typeof block.meta?.sourceItemId === 'string' ? block.meta.sourceItemId : ''
  const inspector = useRecordedToolInspectorSelection(id + ':' + block.id + ':' + disclosureKey + ':' +
    callId + ':' + resultItemId, shown && Boolean(data) && !privateResultVisible)
  if (typeof name !== 'string' || !Object.hasOwn(recordedJobNames, name) || block.toolKind !== 'command_execution') return null
  const operation = recordedJobNames[name as RecordedJobName]
  const model = data ? recordedJobCardModel(data, t) : detailsCardModel(block, t)
  const inspect = data ? { label: t('dshToolInspect.inspect'), run: () => { if (onInspect) onInspect(turnId, callId); else inspector.inspect() } } : undefined
  return <div ref={publicRoot} data-recorded-job-row={operation} data-tool-view="job" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && state !== 'running' ? <ProtectedToolResult preload
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
      renderContent={display => {
        if (display.capture.kind !== 'job') return null
        const job = display.capture.job
        return <><ToolDetails model={protectedJobCardModel(job, t)} hasInspect />
          {job.operation === 'output' ? job.jobs.map(record => <div key={record.jobId} className={css.ioCard} data-job-output data-protected-tool-readonly>
            <div className={css.ioSection}><span className={css.ioLabel}>{t('dshRecordedTool.job.outputTitle')} · {record.jobId}</span>
              {record.output.availability === 'available' ? <><pre className={css.ioText}>{record.output.body}</pre>
                {record.output.truncated ? <p>{t('dshToolInspect.captureLimit')}</p> : null}</>
                : <p>{t('dshRecordedTool.job.outputUnavailable')}</p>}</div>
          </div>) : null}</>
      }} renderView={controls => {
        const job = controls.display?.capture.kind === 'job' ? controls.display.capture.job : null
        const privateModel = job ? protectedJobCardModel(job, t) : null
        return <ToolRow title={t('dshRecordedTool.job.' + operation + 'Title')} toolName={name} variant="job" state={controls.state ?? state}
          icon={<DshChecklistOutlineRegular />}
          summary={privateModel ? <span data-protected-tool-summary>{privateModel.summary}</span> : t('protectedToolUnavailable')}
          summarySuffix={privateModel?.expandedSummary ? <span data-protected-tool-summary data-job-receipt>{privateModel.expandedSummary}</span> : undefined}
          expanded={controls.open} expandable={controls.canOpen} onToggle={controls.onToggle}
          body={controls.body ? { kind: 'details', content: controls.body } : undefined} inspect={controls.inspect}
          actions={<><RuntimeDiagnosticsAction block={block} t={t} />{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />
      }} /> : <ToolRow title={t('dshRecordedTool.job.' + operation + 'Title')} toolName={name} variant="job" state={state}
      icon={<DshChecklistOutlineRegular />}
      summary={data ? model?.summary : state !== 'running' ? t('protectedToolUnavailable') : undefined}
      expanded={state === 'running' ? runningCapture?.expanded : shown && disclosure.open}
      expandable={state === 'running' ? runningCapture?.expandable : shown && Boolean(model)} onToggle={() => {
        if (state === 'running') { runningCapture?.setOpen(!runningCapture.expanded); return }
        if (disclosure.open) inspector.close()
        disclosure.toggle()
      }}
      body={state === 'running' ? runningCapture?.expanded ? { kind: 'details', content: runningCapture.body } : undefined
        : model ? { kind: 'details', content: <ToolDetails model={model} hasInspect={Boolean(inspect)} /> } : undefined} inspect={inspect}
      actions={<RuntimeDiagnosticsAction block={block} t={t} />} />}
    {inspector.selected && data && !privateResultVisible ? <div data-public-job-inspector>
      <ToolResultInspector key={callId + ':' + resultItemId} publicJob={block}
        callId={callId} state={state} onClose={inspector.close} />
    </div> : null}
  </div>
}
