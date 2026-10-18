/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useId, useLayoutEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { DshAgentPresetOutlineRegular, DshInspectOutlineRegular } from '../../DshToolIcons'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import { childMetaFromBlock, SubagentDetailBody, SubagentNativeControls } from '../../../SubagentCallCard'
import { ToolAttachmentPreviews } from '../../../message-timeline-bubbles'
import { ToolRow } from '../ToolRow'
import type { ToolViewProps } from '../ToolViews'
import { RuntimeDiagnosticsAction, RuntimeMetadata, RuntimeMetadataDetails } from '../ToolViewActions'
import { useDisclosure } from '../use-disclosure'
import { hasPublicFocus, useSearchableHidden } from '../searchable-hidden'
import { subagentCardModel, subagentNativeIdentity, subagentPublicSummary, subagentRowState } from '../models/subagent-card-model'
import { ToolDetails } from './ToolDetails'
import css from './subagent-row.module.css'
import rowCss from '../ToolRow.module.css'

/** DSH started receipts and native child lifecycle keep their own authority. */
export function SubagentRow({ block, visible, resetKey, publicVisible = visible, publicResetKey = resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common'), bodyId = useId()
  const child = childMetaFromBlock(block)
  const publicRoot = useRef<HTMLDivElement>(null), publicReset = useRef(publicResetKey)
  const publicFocused = hasPublicFocus(publicRoot.current)
  if (!publicFocused) publicReset.current = publicResetKey
  const disclosure = useDisclosure(block.id + ':' + publicReset.current)
  const shown = publicVisible || publicFocused
  const publicBody = useSearchableHidden(!shown || !disclosure.open, () => disclosure.setOpen(true))
  const publicOpen = shown && disclosure.open
  const runningCapture = block.status === 'running' && visible ? capture : undefined
  const privateOpenIntent = useRef(false), privateIntentGeneration = useRef(privateGeneration)
  useLayoutEffect(() => {
    if (!visible || block.status !== 'running' || privateIntentGeneration.current !== privateGeneration) privateOpenIntent.current = false
    privateIntentGeneration.current = privateGeneration
    if (privateOpenIntent.current && publicOpen && runningCapture?.expandable && !runningCapture.expanded) runningCapture.setOpen(true)
  }, [visible, block.status, privateGeneration, publicOpen, runningCapture])
  const hasPublicBody = Boolean(child || block.meta?.diagnostics || ['attachmentIds', 'attachments', 'activeSkillIds', 'injectedMemoryIds', 'sources']
    .some(key => Array.isArray(block.meta?.[key]) && (block.meta![key] as unknown[]).length > 0))
  const rowProps = { title: t('dshToolFamily.subagent'), toolName: typeof block.meta?.toolName === 'string' ? block.meta.toolName : undefined,
    variant: 'subagent', icon: <DshAgentPresetOutlineRegular size={14} /> }
  const state = subagentRowState(block, child)
  const actions = <RuntimeDiagnosticsAction block={block} t={t} />
  const publicSummary = subagentPublicSummary(block, child, t)
  return <div ref={publicRoot} data-subagent-row data-tool-view="subagent" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && block.status !== 'running' ? <ProtectedToolResult preload
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
      renderContent={display => {
        const model = subagentCardModel(block, display, t)
        return model ? <div data-subagent-start-receipt data-protected-tool-readonly><ToolDetails model={model} hasInspect /></div> : null
      }} renderView={controls => {
        const model = controls.display ? subagentCardModel(block, controls.display, t) : null
        const open = controls.open || hasPublicBody && publicOpen
        const toggle = () => {
          if (open) { disclosure.setOpen(false); if (controls.open) controls.onToggle() }
          else { if (hasPublicBody) disclosure.setOpen(true); if (controls.canOpen) controls.onToggle() }
        }
        return <><ToolRow {...rowProps} state={subagentRowState(block, child, controls.state)}
          summary={model ? <span data-protected-tool-summary>{model.summary}</span> : publicSummary}
          summarySuffix={model?.expandedSummary ? <span data-protected-tool-summary>{model.expandedSummary}</span> : undefined}
          expanded={open} expandable={controls.canOpen || hasPublicBody} onToggle={toggle}
          externalBodyId={hasPublicBody ? controls.bodyId + ' ' + bodyId : controls.bodyId}
          actions={<>{actions}{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />
          {open ? <div className={css.publicBody}>{controls.body}
            {controls.inspect ? <button type="button" className={rowCss.inspectButton} onClick={controls.inspect.run}>
              <DshInspectOutlineRegular />{controls.inspect.label}</button> : null}
          </div> : null}</>
      }} /> : <ToolRow {...rowProps} state={state} summary={publicSummary} actions={actions}
      expanded={publicOpen || visible && capture?.expanded} expandable={hasPublicBody || visible && capture?.expandable}
      externalBodyId={visible && capture?.expandable ? bodyId + ' ' + bodyId + '-input' : bodyId}
      onToggle={() => {
        const next = !(publicOpen || visible && capture?.expanded)
        privateOpenIntent.current = visible && block.status === 'running' && next
        if (hasPublicBody) disclosure.setOpen(next)
        if (visible && capture?.expandable) capture.setOpen(next)
      }}
      />}
    {block.status === 'running' && visible && capture?.expanded ? <div id={bodyId + '-input'} className={css.publicBody}>{capture.body}</div> : null}
    {/* This stable sibling cannot inherit a private leaf's revoke/fold key. */}
    <div ref={publicBody} id={bodyId} className={css.publicBody} data-subagent-public-controls>
      {publicOpen ? <><RuntimeMetadataDetails block={block} /><RuntimeMetadata block={block} t={t} /><ToolAttachmentPreviews meta={block.meta} />
        {child ? <SubagentDetailBody block={block} showControls={false} /> : null}</> : null}
      {child ? <SubagentNativeControls key={subagentNativeIdentity(child)} block={block} /> : null}
    </div>
  </div>
}
