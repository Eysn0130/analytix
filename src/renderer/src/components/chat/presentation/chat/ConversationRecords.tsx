/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toolResultLocalSelectorSchemaV1 } from '../../../../../../../packages/runtime/src/contracts/tool-result-local-display'
import type { ChatBlock } from '../../../../agent/types'
import { ProtectedToolResult, ProtectedToolResultScope } from '../ProtectedToolResult'
import { ToolResultInspector, type ToolResultInspectorPresentation } from './toolviews/ToolResultInspector'
import { recordedJobData } from './models/recorded-tool-card-model'
import { conversationCallRecord, type ConversationCallRecord, type ConversationInspectRequest } from './models/conversation-record-model'
import { toolRowState } from './models/tool-call-model'
import css from './ConversationRecords.module.css'

/** These rows have a real turn parent. No nearest Assistant/PTC association is inferred. */
export function ConversationRecords({ threadId, turnId, blocks, request, selected, onInspect }: {
  threadId: string; turnId: string; blocks: readonly ChatBlock[]; request: ConversationInspectRequest | null
  selected: string | null; onInspect: (record: ConversationCallRecord) => void
}) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(true)
  useLayoutEffect(() => { if (request?.threadId === threadId && request.turnId === turnId) setOpen(true) }, [threadId, turnId, request?.version, request?.threadId, request?.turnId])
  return <section className={css.turn} data-conversation-record-turn={turnId}>
    <button type="button" className={css.heading} aria-expanded={open} onClick={() => setOpen(value => !value)}>
      <span aria-hidden>{open ? '▾' : '▸'}</span><span>{t('dshRecords.turn')}</span><code>{turnId}</code>
    </button>
    {open ? <div className={css.rows}>{blocks.map((block, index) => {
      const record = conversationCallRecord(threadId, block)
      if (record && record.turnId === turnId) return <button type="button" key={block.id + ':' + index}
        className={css.row} aria-pressed={selected === record.key} data-conversation-call-key={record.key}
        data-call-item-id={record.toolCallItemId} data-result-item-id={record.toolResultItemId} onClick={() => onInspect(record)}>
        <span aria-hidden>↳</span><span><span className={css.name}>{typeof record.block.meta?.toolName === 'string' ? record.block.meta.toolName : t('dshRecords.call')}</span>
          <span className={css.identity}>{record.callId}</span></span><span>{block.kind === 'tool' ? block.status : ''}</span>
      </button>
      if ((block.kind === 'user' || block.kind === 'assistant' || block.kind === 'compaction') && block.meta?.turnId === turnId)
        return <div key={block.id + ':' + index} className={css.context} data-conversation-source-item={block.id}>
          <span>{t('dshRecords.' + block.kind)} · <code>{block.id}</code></span>
          {/* Only the existing admitted public text, never a raw item/prompt/draft. */}
          {block.kind !== 'compaction' ? <p className={css.text}>{block.text}</p> : null}
        </div>
      return null
    })}</div> : null}
  </section>
}

type ConversationRecordDetailsProps = {
  record: ConversationCallRecord; requestVersion: number; onClose: () => void; onRetire: () => void
  initialView?: 'result' | 'inspect'
  presentation?: ToolResultInspectorPresentation
  onViewChange?: (view: 'result' | 'inspect') => void
}

/** Stable details seat, outside virtual rows; private data stays in its original lease owner. */
export function ConversationRecordDetails(props: ConversationRecordDetailsProps) {
  return <ConversationRecordDetailsSeat key={props.record.key + ':' + props.requestVersion} {...props} />
}

function ConversationRecordDetailsSeat({ record, requestVersion, onClose, onRetire, initialView = 'result', onViewChange, presentation = 'overlay' }: ConversationRecordDetailsProps) {
  const { t } = useTranslation('common')
  const retire = useRef(onRetire)
  retire.current = onRetire
  const retirementEpoch = useRef(0)
  useLayoutEffect(() => {
    const epoch = ++retirementEpoch.current
    if (presentation === 'embedded') return () => {
      // StrictMode may immediately set this seat up again. The protected leaf
      // still clears its lease synchronously; only outer intent waits one microtask.
      queueMicrotask(() => { if (retirementEpoch.current === epoch) retire.current() })
    }
  }, [presentation])
  const { block } = record
  const job = block.meta?.recordThreadId === record.threadId && block.meta?.turnId === record.turnId &&
    block.meta?.callId === record.callId && block.meta?.sourceItemId === record.toolResultItemId ? recordedJobData(block) : null
  const local = toolResultLocalSelectorSchemaV1.omit({ threadId: true }).safeParse(block.meta?.localResult)
  const canInspectJob = local.success && toolResultLocalSelectorSchemaV1.safeParse({ ...local.data, threadId: record.threadId }).success &&
    local.data.turnId === record.turnId && local.data.callId === record.callId &&
    local.data.resultItemId === record.toolResultItemId
  const [inspectJob, setInspectJob] = useState(initialView === 'inspect')
  useLayoutEffect(() => {
    if (!canInspectJob && inspectJob) { setInspectJob(false); onViewChange?.('result') }
  }, [canInspectJob, inspectJob, onViewChange])
  const changeView = (view: 'result' | 'inspect') => { setInspectJob(view === 'inspect'); onViewChange?.(view) }
  const privateView = job ? inspectJob && canInspectJob : Boolean(block.meta?.localResult)
  return <ProtectedToolResultScope.Provider value={record.threadId}>
    <div data-conversation-record-details>
      {job && canInspectJob ? <div className={css.toolbar}>
        <button type="button" data-conversation-private-inspect={!privateView || undefined}
          data-conversation-public-result={privateView || undefined} onClick={() => changeView(privateView ? 'result' : 'inspect')}>
          {t(privateView ? 'dshToolInspect.result' : 'dshToolInspect.inspect')}
        </button>
      </div> : null}
      {privateView ? <ProtectedToolResult key={record.key + ':' + requestVersion} block={block}
        inspectorOnly inspectorPresentation={presentation} inspectRequest={requestVersion} onInspectorClose={onClose}
        onInspectorRetire={presentation === 'embedded' ? onRetire : job ? () => changeView('result') : onRetire} />
        : job ? <ToolResultInspector key={record.key + ':' + requestVersion} publicJob={block} callId={record.callId} presentation={presentation}
          state={toolRowState(block)} onClose={onClose} />
          : <ToolResultInspector key={record.key + ':' + requestVersion} publicRecord={block} callId={record.callId} presentation={presentation}
            state={toolRowState(block)} onClose={onClose} />}
      {!job || !privateView ? <p className={css.notice}>{t('dshRecords.scope')}</p> : null}
    </div>
  </ProtectedToolResultScope.Provider>
}

export type ConversationViewMode = 'conversation' | 'records' | 'trajectory'

export function ConversationRecordsToolbar({ mode, onModeChange, pending }: {
  mode: ConversationViewMode; onModeChange: (mode: ConversationViewMode) => void; pending?: 'missing' | 'ambiguous'
}) {
  const { t } = useTranslation('common')
  return <div className={css.toolbar}>
    <button type="button" aria-pressed={mode === 'conversation'} onClick={() => onModeChange('conversation')}>{t('dshRecords.conversation')}</button>
    <button type="button" aria-pressed={mode === 'records'} onClick={() => onModeChange('records')}>{t('dshRecords.records')}</button>
    <button type="button" aria-pressed={mode === 'trajectory'} onClick={() => onModeChange('trajectory')}>{t('dshTrajectory.title')}</button>
    {pending ? <span role="status">{t('dshRecords.' + pending)}</span> : null}
  </div>
}
