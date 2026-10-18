import { useTranslation } from 'react-i18next'
import { DshBrowseOutlineRegular, DshEditOutlineRegular } from '../../DshToolIcons'
import { ProtectedToolResult } from '../../ProtectedToolResult'
import { ToolRow } from '../ToolRow'
import type { ToolViewProps } from '../ToolViews'
import { RuntimeDiagnosticsAction } from '../ToolViewActions'
import { fileResultCardModel, fileResultToolKind } from '../models/file-result-card-model'
import { toolRowState } from '../models/tool-call-model'
import { ToolDetails } from './ToolDetails'
import css from '../ToolRow.module.css'

/** Seven admitted existing effects; no generic result or filesystem fallback. */
export function FileResultRow({ block, visible, resetKey, privateGeneration, capture, onInspect }: ToolViewProps) {
  const { t } = useTranslation('common'), name = block.meta?.toolName
  const expected = fileResultToolKind(name)
  if (!expected || block.toolKind !== expected || typeof name !== 'string') return null
  const state = toolRowState(block)
  const rowProps = { title: t('dshFileResult.tools.' + name), toolName: name, variant: 'file-result',
    icon: expected === 'file_change' ? <DshEditOutlineRegular /> : <DshBrowseOutlineRegular /> }
  const actions = <RuntimeDiagnosticsAction block={block} t={t} />
  return <div data-file-result-row={name} data-tool-view="file-result" data-analytix-process-block-id={block.id}
    data-analytix-process-block-kind={block.kind} data-analytix-process-block-status={block.status}>
    {visible && block.meta?.localResult && state !== 'running' ? <ProtectedToolResult preload block={block} onInspect={onInspect}
      key={block.id + ':' + resetKey + ':' + privateGeneration}
      renderContent={display => {
        const model = fileResultCardModel(block, display, t)
        if (!model) return <p>{t('dshToolInspect.unavailable')}</p>
        return <div data-protected-tool-readonly>
          <ToolDetails model={model} hasInspect />
          {model.diff?.patch !== undefined ? <pre className={css.ioText}>{model.diff.patch}</pre> : null}
          {model.diff?.patchUnavailable ? <p>{t('dshFileResult.patchUnavailable')}</p> : null}
          {model.diff?.omittedReason ? <p>{t('dshFileResult.diffOmitted.' + model.diff.omittedReason)}</p> : null}
          {model.producerTruncated ? <p>{t('dshFileResult.producerTruncated')}</p> : null}
          {display.capture.truncated ? <p>{t('dshFileResult.captureTruncated')}</p> : null}
        </div>
      }} renderView={controls => {
        const model = controls.display ? fileResultCardModel(block, controls.display, t) : null
        return <ToolRow {...rowProps} state={controls.state ?? state}
          summary={model ? <span data-protected-tool-summary data-protected-tool-readonly>{model.summary}</span> : undefined}
          summarySuffix={model?.expandedSummary ? <span data-protected-tool-summary data-protected-tool-readonly>{model.expandedSummary}</span> : undefined}
          expanded={controls.open} expandable={controls.canOpen} onToggle={controls.onToggle} inspect={controls.inspect}
          body={controls.body ? { kind: 'details', content: controls.body } : undefined}
          actions={<>{actions}{controls.pending ? <span role="status">{t('protectedToolLoading')}</span> : null}</>} />
      }} /> : <ToolRow {...rowProps} state={state} actions={actions}
        expanded={visible && capture?.expanded} expandable={visible && capture?.expandable}
        onToggle={() => capture?.setOpen(!capture.expanded)}
        body={visible && capture?.expanded ? { kind: 'details', content: capture.body } : undefined} />}
  </div>
}
