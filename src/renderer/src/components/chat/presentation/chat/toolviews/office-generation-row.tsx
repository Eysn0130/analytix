/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useTranslation } from 'react-i18next'
import { generatedArtifactMetadataSchema } from '../../../../../../../../packages/runtime/src/contracts/generated-artifact'
import { DisclosureRow } from '../../DisclosureRow'
import { DshDeliverDocRegular } from '../../DeliverableIcons'
import { ProtectedToolResult, type ProtectedToolResultViewControls } from '../../ProtectedToolResult'
import type { ToolViewProps } from '../ToolViews'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import { toolRowState } from '../models/tool-call-model'
import type { ToolRowState } from '../ToolRow'
import css from './office-generation-row.module.css'

/** PresentRow chrome consumes Analytix's actual Office producer and existing Inspect authority. */
export function OfficeGenerationRow({ block, visible, resetKey, privateGeneration, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common')
  if (block.meta?.toolName !== 'generate_office_document' || block.toolKind !== 'file_change') return null
  const state = toolRowState(block)
  const parsed = block.status === 'success' ? generatedArtifactMetadataSchema.safeParse(block.meta?.generatedArtifact) : null
  const receipt = parsed?.success ? parsed.data : null
  const summary = receipt ? receipt.kind.toUpperCase() + ' · ' + Math.ceil(receipt.byteSize / 1024) + ' KB' : ''
  const row = (rowState: ToolRowState, controls?: ProtectedToolResultViewControls) => <div data-tool="generate_office_document" data-state={rowState}>
    <DisclosureRow title={t('dshOfficeGeneration.title')} icon={<DshDeliverDocRegular />}
      bodyId={controls?.bodyId} open={Boolean(controls?.open)} expandable={Boolean(controls?.canOpen)}
      onToggle={controls?.onToggle ?? (() => undefined)} expandOnRowClick keepContentWhenOpen
      error={rowState === 'error'} collapsedContent={<span className={css.summary}>
        <span>{t('dshOfficeGeneration.' + rowState)}</span>{summary ? <span className={css.metadata}>{summary}</span> : null}
      </span>}
      actions={<><RuntimeDiagnosticsAction block={block} t={t} />{controls?.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>}>
      {controls?.body}
      {controls?.open && controls.inspect ? <button type="button" className={css.inspect} onClick={controls.inspect.run}>{controls.inspect.label}</button> : null}
    </DisclosureRow>
  </div>
  return <div data-office-generation-row data-tool-view="office-delivery" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && state !== 'running' && block.meta?.localResult
      ? <ProtectedToolResult key={block.id + ':' + resetKey + ':' + privateGeneration} block={block} onInspect={onInspect}
        renderView={controls => row(controls.state ?? state, controls)} />
      : row(state)}
  </div>
}
