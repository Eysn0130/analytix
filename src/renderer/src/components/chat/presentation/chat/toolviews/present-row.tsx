/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import { DisclosureRow } from '../../DisclosureRow'
import { DshDeliverDocRegular } from '../../DeliverableIcons'
import { ProtectedToolResult, type ProtectedToolResultViewControls } from '../../ProtectedToolResult'
import type { ToolViewProps } from '../ToolViews'
import { toolRowState } from '../models/tool-call-model'
import css from './office-generation-row.module.css'

/** The real Present call remains a process row; its tail is a separate owner. */
export function PresentRow({ block, visible, resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common')
  if (block.meta?.toolName !== 'present' || block.toolKind !== 'tool_call') return null
  const state = toolRowState(block)
  const row = (controls?: ProtectedToolResultViewControls) => {
    const receipt = controls?.display?.capture.kind === 'present' ? controls.display.capture.present : null
    const error = controls?.display?.inspect?.outputError
    const details = Boolean(receipt || error?.availability === 'available')
    const summary = receipt ? receipt.files.map(file => file.path).join(', ') : visible ? capture?.preview : null
    const shownState = controls?.state ?? state
    return <div data-tool="present" data-state={shownState}>
      <DisclosureRow title={t('dshPresent.title')} icon={<DshDeliverDocRegular size={14} />}
        running={shownState === 'running'} error={shownState === 'error'} bodyId={controls?.bodyId}
        open={details && Boolean(controls?.open)} expandable={details && Boolean(controls?.canOpen)}
        onToggle={controls?.onToggle ?? (() => undefined)} expandOnRowClick keepContentWhenOpen
        collapsedContent={<span className={css.summary} data-protected-tool-summary={receipt ? true : undefined}>
          <span>{t('dshPresent.' + shownState)}</span>{summary ? <span className={css.metadata}>{summary}</span> : null}
        </span>}>
        {details ? controls?.body : null}
        {details && controls?.open && controls.inspect ? <button type="button" className={css.inspect} onClick={controls.inspect.run}>{controls.inspect.label}</button> : null}
      </DisclosureRow>
    </div>
  }
  return <div data-present-row data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && state !== 'running' && block.meta?.localResult ? <ProtectedToolResult
      key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} preload onInspect={onInspect}
      renderView={row} renderContent={display => display.capture.kind === 'present'
        ? <div className={css.output}>{display.capture.present.files.map(file =>
          file.path + (file.description === undefined ? '' : '\n' + file.description)).join('\n\n')}</div> : null} /> : row()}
  </div>
}
