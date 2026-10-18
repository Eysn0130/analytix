/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { cloneElement, useId, useLayoutEffect, useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DshApiOutlineRegular, DshChevronDownOutlineRegular, DshChevronUpOutlineRegular, DshInspectOutlineRegular } from '../../DshToolIcons'
import { TextShimmer } from '../../TextShimmer'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import type { ToolRowState } from '../ToolRow'
import type { ToolViewProps } from '../ToolViews'
import { PublicToolBody } from '../ToolViews'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import { describeProcessBlock, getProcessDetail, publicToolOperationTitle } from '../chat-view-model'
import { toolRowState } from '../models/tool-call-model'
import { useDisclosure } from '../use-disclosure'
import { hasPublicFocus } from '../searchable-hidden'
import css from './bash-row.module.css'
function BashHeader({ toolName, title, summary, state, expanded, expandable, onToggle, bodyId, actions }: {
  toolName?: string; title: string; summary: string; state: ToolRowState; expanded: boolean; expandable: boolean; onToggle: () => void; bodyId: string; actions: ReactNode
}) {
  const { t } = useTranslation('common')
  const running = state === 'running' || state === 'preparing'
  const status = state === 'stopped' ? t('dshTurnStopped') : state === 'error' ? t('protectedToolFailed')
    : state === 'unknown' ? t('protectedToolUnknown') : running ? t('inspectorStatusRunning') : null
  const keyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (!expandable || event.repeat || event.keyCode === 229 || event.nativeEvent.isComposing || event.target !== event.currentTarget || !['Enter', ' '].includes(event.key)) return
    event.preventDefault(); onToggle()
  }
  const settlement = state === 'stopped' ? t('dshTurnStopped') : state === 'error' ? summary.split(/\r\n|\r|\n/).find(line => line.trim()) : summary
  return <div className={css.root} data-sample="bash" data-tool={toolName} data-variant="bash" data-state={state}
    data-disclosure-row data-expandable={expandable || undefined} role={expandable ? 'button' : undefined} tabIndex={expandable ? 0 : undefined}
    aria-expanded={expandable ? expanded : undefined} aria-controls={expandable ? bodyId : undefined}
    onClick={expandable ? onToggle : undefined} onKeyDown={expandable ? keyboard : undefined}>
    <span className={css.leading}>{expanded ? <DshChevronUpOutlineRegular size={14} className={css.chevron} /> : expandable ? <>
      <span className={css.iconIdle}><DshApiOutlineRegular size={14} /></span><DshChevronDownOutlineRegular size={14} className={`${css.chevron} ${css.chevronHover}`} /></> : <DshApiOutlineRegular size={14} />}</span>
    {status ? <span className={css.visuallyHidden}>{status}</span> : null}
    <TextShimmer active={running} className={css.title}>{title}</TextShimmer>
    {settlement ? <><span className={css.sep} aria-hidden /><TextShimmer active={running}
      className={`${css.summary} ${state === 'error' ? css.errorSummary : state === 'stopped' ? css.stoppedSummary : ''}`}>{settlement}</TextShimmer></> : null}
    <span className={css.actions} onClick={event => event.stopPropagation()}>{actions}</span>
  </div>
}
/** Independent keyed Bash implementation; Host states and the existing protected authority own its terminal. */
export function BashRow({ block, processing, visible, resetKey, publicVisible = visible, publicResetKey = resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common'), bodyId = useId()
  const publicRoot = useRef<HTMLDivElement>(null), publicReset = useRef(publicResetKey)
  const publicFocused = hasPublicFocus(publicRoot.current)
  if (!publicFocused) publicReset.current = publicResetKey
  const disclosure = useDisclosure(block.id + ':' + publicReset.current)
  const shown = publicVisible || publicFocused
  const activeCapture = visible ? capture : undefined
  const privateOpenIntent = useRef(false), intentGeneration = useRef(privateGeneration)
  useLayoutEffect(() => {
    if (!visible || intentGeneration.current !== privateGeneration) privateOpenIntent.current = false
    intentGeneration.current = privateGeneration
    if (visible && privateOpenIntent.current && disclosure.open && capture?.expandable && !capture.expanded) capture.setOpen(true)
  }, [visible, disclosure.open, capture, privateGeneration])
  const toolName = typeof block.meta?.toolName === 'string' ? block.meta.toolName : undefined
  const state = toolRowState(block), title = publicToolOperationTitle(block, t) ?? t('dshToolFamily.shell')
  const description = describeProcessBlock(block, t), summary = description.startsWith(title + ' ') ? description.slice(title.length + 1) : description === title ? '' : description
  const detail = getProcessDetail(block, description)
  // Preserve the existing pure action element, including its authorization, label and handler.
  const diagnosticAction = RuntimeDiagnosticsAction({ block, t })
  const diagnostics = diagnosticAction === null ? null : cloneElement(diagnosticAction, undefined, <DshInspectOutlineRegular size={12} />)
  const toggle = () => { const next = !disclosure.open; privateOpenIntent.current = visible && next; disclosure.setOpen(next); if (activeCapture?.expandable) activeCapture.setOpen(next) }
  const expandable = state !== 'preparing' && (detail.kind !== 'none' || Boolean(activeCapture?.expandable))
  const open = shown && disclosure.open && expandable
  return <div ref={publicRoot} className={css.card} data-analytix-process-block-id={block.id} data-analytix-process-block-kind={block.kind}
    data-analytix-process-block-status={block.status} data-tool-view="shell">
    {block.meta?.localResult && block.status !== 'running' ? visible ? <ProtectedToolResult block={block} onInspect={onInspect}
      renderTrigger={controls => <BashHeader toolName={toolName} title={title} summary={summary} state={controls.state ?? state} expanded={controls.open} expandable={controls.canOpen}
        bodyId={controls.bodyId} onToggle={controls.onToggle} actions={<>{diagnostics}{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />} />
      : <BashHeader toolName={toolName} title={title} summary={summary} state={state} expanded={false} expandable={false} bodyId={bodyId} onToggle={() => undefined} actions={diagnostics} />
      : <><BashHeader toolName={toolName} title={title} summary={summary} state={state} expanded={open} expandable={expandable && shown} bodyId={bodyId} onToggle={toggle} actions={diagnostics} />
        {open ? <div id={bodyId} className={css.bodyWrap}>{activeCapture?.expandable ? activeCapture.body
          : <PublicToolBody block={block} detail={detail} processing={processing} error={state === 'error'} />}</div> : null}</>}
  </div>
}
