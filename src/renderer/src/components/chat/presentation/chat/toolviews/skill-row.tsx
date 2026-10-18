/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useContext, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { DisclosureRow } from '../../DisclosureRow'
import { DshChevronDownOutlineRegular, DshSkillOutlineRegular, DshInspectOutlineRegular } from '../../DshToolIcons'
import { TextShimmer } from '../../TextShimmer'
import { ProtectedToolResult, ProtectedToolResultScope } from '../../ProtectedToolResult'
import { SubagentDetailBody } from '../../../SubagentCallCard'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import type { ToolViewProps } from '../ToolViews'
import type { ToolRowState } from '../ToolRow'
import { useDisclosure } from '../use-disclosure'
import { hasPublicFocus } from '../searchable-hidden'
import { toolRowState } from '../models/tool-call-model'
import { skillCardModel } from '../models/recorded-tool-card-model'
import { ToolDetails } from './ToolDetails'
import { SkillChildInspectionContext, type SkillChildInspectionRequest } from './skill-child-inspection'
import css from './skill-row.module.css'

function SkillChildButton({ request }: { request: SkillChildInspectionRequest }) {
  const { t } = useTranslation('common')
  const inspect = useContext(SkillChildInspectionContext)
  const controller = useRef<AbortController | null>(null), opening = useRef(false)
  const [pending, setPending] = useState(false), [unavailable, setUnavailable] = useState(false)
  const identity = JSON.stringify([request.parentThreadId, request.parentTurnId, request.parentToolCallId,
    request.resultItemId, request.child.jobId, request.child.childThreadId, request.child.childTurnId])
  useLayoutEffect(() => {
    opening.current = false; setPending(false); setUnavailable(false)
    return () => { controller.current?.abort(); controller.current = null; opening.current = false }
  }, [identity, inspect])
  if (!inspect || !request.child.childThreadId) return null
  const open = async () => {
    if (opening.current) return
    opening.current = true
    controller.current?.abort()
    const current = new AbortController()
    controller.current = current
    setPending(true); setUnavailable(false)
    try {
      const admitted = await inspect(request, current.signal)
      if (!current.signal.aborted && controller.current === current) setUnavailable(!admitted)
    } catch {
      if (!current.signal.aborted && controller.current === current) setUnavailable(true)
    } finally {
      if (!current.signal.aborted && controller.current === current) { opening.current = false; setPending(false) }
    }
  }
  return <div className={css.childInspection}>
    <button type="button" className={css.childInspectButton} disabled={pending} data-skill-child-inspect
      onClick={() => { void open() }}><DshInspectOutlineRegular />{t('subagentOutputAction')}</button>
    {pending || unavailable ? <span role="status">{t(pending ? 'protectedToolLoading' : 'protectedToolUnavailable')}</span> : null}
  </div>
}

function SkillHeader({ state, summary, open = false, expandable = false, onToggle = () => undefined, bodyId, actions, children, inspect }: {
  state: ToolRowState; summary?: ReactNode; open?: boolean; expandable?: boolean; onToggle?: () => void; bodyId?: string; actions?: ReactNode; children?: ReactNode
  inspect?: { label: string; run: () => void }
}) {
  const { t } = useTranslation('common')
  const leading = open ? <DshChevronDownOutlineRegular /> : expandable ? <>
    <span className={css.iconIdle}><DshSkillOutlineRegular /></span><DshChevronDownOutlineRegular className={css.chevronHover} />
  </> : <DshSkillOutlineRegular />
  const stopped = state === 'stopped', failed = state === 'error', running = state === 'running' || state === 'preparing'
  return <div className={css.card} data-tool="run_skill" data-state={state}>
    {(running || failed || stopped || state === 'unknown') ? <span className={css.visuallyHidden}>{t('dshRecordedTool.skill.' + (state === 'error' ? 'failed' : state))}</span> : null}
    <DisclosureRow title={t('dshRecordedTool.skill.title')} icon={leading} open={open} expandable={expandable}
      onToggle={onToggle} bodyId={bodyId} running={running} expandOnRowClick showDisclosureIcon={false}
      rowClassName={css.row} leadingClassName={css.leading} titleClassName={css.title} actions={actions}
      summary={summary ? <><span className={css.separator} data-shimmer-decoration aria-hidden="true" />
        <span className={`${css.summary} ${failed ? css.errorSummary : stopped ? css.stoppedSummary : ''}`}><TextShimmer>{summary}</TextShimmer></span></> : undefined}>
      <div className={css.bodyWrap}>{children}
        {inspect ? <button type="button" className={css.inspectButton} onClick={inspect.run}><DshInspectOutlineRegular />{inspect.label}</button> : null}
      </div>
    </DisclosureRow>
  </div>
}
/** Independent Skill chrome with the existing protected lease and real child control owner. */
export function SkillRow({ block, visible, resetKey, publicVisible = visible, publicResetKey = resetKey, privateGeneration, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common'), bodyId = useId()
  const threadId = useContext(ProtectedToolResultScope)
  const publicRoot = useRef<HTMLDivElement>(null), publicReset = useRef(JSON.stringify([publicResetKey, privateGeneration]))
  const publicFocused = hasPublicFocus(publicRoot.current)
  if (!publicFocused) publicReset.current = JSON.stringify([publicResetKey, privateGeneration])
  const disclosure = useDisclosure(block.id + ':' + publicReset.current)
  const shown = publicVisible || publicFocused
  if (block.meta?.toolName !== 'run_skill' || block.toolKind !== 'tool_call') return null
  const state = toolRowState(block)
  // Preserve native controls; capture observations enter the separate, summary-admitted Inspector.
  const childBody = block.meta?.child ? <SubagentDetailBody block={block} /> : null
  const actions = <RuntimeDiagnosticsAction block={block} t={t} />
  const fallback = <SkillHeader state={state} summary={state === 'stopped' ? t('dshRecordedTool.skill.stopped') :
    visible && state !== 'running' ? t('protectedToolUnavailable') : undefined} actions={actions}
    open={shown && disclosure.open} expandable={shown && Boolean(childBody)} onToggle={disclosure.toggle} bodyId={bodyId} />
  return <div ref={publicRoot} data-skill-row data-tool-view="skill" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && state !== 'running' ? <ProtectedToolResult preload
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
      renderContent={display => {
        if (display.capture.kind !== 'skill') return null
        const capture = display.capture, model = skillCardModel(capture.skill, capture.body, t)
        const identity = block.meta?.toolResultIdentity, reference = block.meta?.localResult
        const childRequest = capture.skill.child && identity && reference && identity.threadId === threadId &&
          identity.turnId === reference.turnId && identity.callId === reference.callId && identity.itemId === display.resultItemId
          ? { parentThreadId: identity.threadId, parentTurnId: identity.turnId, parentToolCallId: identity.callId,
            resultItemId: identity.itemId, child: capture.skill.child } : null
        return <>
          {model.instructions !== null ? <section className={css.instructionsCard} aria-label={t('dshRecordedTool.skill.instructions')}>
            <div className={css.instructionsHeader}>{t('dshRecordedTool.skill.instructions')}</div>
            <pre className={css.instructions} data-error={capture.status === 'failed' || undefined}>{model.instructions}</pre>
          </section> : <ToolDetails model={model.details} />}
          {model.instructions !== null && capture.skill.error ? <ToolDetails model={model.details} /> : null}
          {capture.truncated ? <p className={css.limit}>{t('protectedToolTruncated')}</p> : null}
          {childRequest ? <SkillChildButton key={display.snapshotDigest} request={childRequest} /> : null}
        </>
      }} renderView={controls => {
        const capture = controls.display?.capture
        const model = capture?.kind === 'skill' ? skillCardModel(capture.skill, capture.body, t) : null
        const currentState = controls.state ?? state
        const open = controls.open || Boolean(childBody) && shown && disclosure.open
        const toggle = () => {
          if (open) { disclosure.setOpen(false); if (controls.open) controls.onToggle() }
          else { if (childBody) disclosure.setOpen(true); if (controls.canOpen) controls.onToggle() }
        }
        return <SkillHeader state={currentState} summary={currentState === 'stopped' ? t('dshRecordedTool.skill.stopped') : model ?
          <span data-protected-tool-summary>{model.summary}</span> : t('protectedToolUnavailable')}
          open={open} expandable={controls.canOpen || Boolean(childBody)} onToggle={toggle}
          bodyId={childBody ? controls.bodyId + ' ' + bodyId : controls.bodyId}
          inspect={controls.inspect}
          actions={<>{actions}{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>}>
          {controls.body}
        </SkillHeader>
      }} /> : fallback}
    {/* The public child keeps one mounted owner when the protected lease retires. */}
    {shown && disclosure.open && childBody ? <div id={bodyId} className={css.bodyWrap}>{childBody}</div> : null}
  </div>
}
