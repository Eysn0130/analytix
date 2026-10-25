// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/excel.tsx
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Read-only spreadsheet surface backed by one cancellable parser Worker. */
import { useCallback, useEffect, useRef, useState, type ComponentProps, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Workbook } from '@fortune-sheet/react'
import fortuneCss from '@fortune-sheet/react/dist/index.css?inline'
import type { ExcelFormat } from '@shared/excel/format'
import type { ExcelLimits, ExcelPreview } from '@shared/excel/model'
import type { WorkspaceFileTarget } from '@shared/workspace-file'
import { DshExcelWarningTriangle } from '../../chat/presentation/DeliverableIcons'
import { Tooltip } from '../../chat/presentation/Tooltip'
import type { LoadedExcelBodyProps } from './LazyExcelBody'
import { parseExcel } from './parse'
import css from './ExcelBody.module.css'

type State = {
  owner: WorkspaceFileTarget
  data: Uint8Array<ArrayBuffer>
  format: ExcelFormat
  limits: ExcelLimits
  attempt: number
} & ({ value: ExcelPreview } | { error: string })
const scopedStyles = `@scope (:is([data-excel-preview], [data-excel-preview-controls])) { ${fortuneCss} }`

/** Display saved values and formats; protected Copy stays behind the original Host lease. */
export function ExcelBody({ owner, data, format, limits, onProtectedCopy, zoomControlTarget, loading }: LoadedExcelBodyProps): ReactNode {
  const { t } = useTranslation('excel')
  const [state, setState] = useState<State>()
  const [attempt, setAttempt] = useState(0)
  const workbookRef = useRef<HTMLDivElement>(null)
  const active = useRef(true)
  const current = useRef({ owner, data, format, limits, attempt, state, onProtectedCopy })
  current.current = { owner, data, format, limits, attempt, state, onProtectedCopy }
  useEffect(() => {
    active.current = true
    return () => { active.current = false }
  }, [])
  // The callback is stable within this parsed workbook lifetime. An old
  // Fortune handler cannot borrow the next owner or retry. Returning false
  // still takes over Copy; the library must never use native clipboard fallback.
  // Requires the separately reviewed core/react protectedCopy patch and its types.
  const protectedCopy = useCallback<NonNullable<ComponentProps<typeof Workbook>['protectedCopy']>>(selection => {
    const input = current.current
    const parsed = state
    if (!active.current || !input.onProtectedCopy || !parsed || !('value' in parsed) || parsed !== input.state
      || parsed.owner !== input.owner || parsed.data !== input.data || parsed.format !== input.format
      || parsed.limits !== input.limits || parsed.attempt !== input.attempt) return false
    const sheetIndex = parsed.value.sheets.findIndex(sheet => sheet.id === selection.sheetId)
    if (sheetIndex < 0) return false
    try {
      return input.onProtectedCopy(sheetIndex, selection.sheetId, selection.ranges.map(range => ({
        row: [...range.row] as [number, number], column: [...range.column] as [number, number]
      }))) === true
    } catch { return false }
  }, [state])
  useEffect(() => {
    const controller = new AbortController()
    setState(undefined)
    void parseExcel(data, format, limits, controller.signal).then(
      value => { if (!controller.signal.aborted) setState({ owner, data, format, limits, attempt, value }) },
      (error: unknown) => { if (!controller.signal.aborted) setState({ owner, data, format, limits, attempt,
        error: error instanceof Error ? error.message : 'invalid' }) },
    )
    return () => { controller.abort() }
  }, [owner, data, format, limits, attempt])
  useEffect(() => {
    const element = workbookRef.current
    if (element === null || typeof ResizeObserver === 'undefined') return
    // FortuneSheet measures its canvas on window resize; pane resizing has no such event.
    const observer = new ResizeObserver(() => { window.dispatchEvent(new Event('resize')) })
    observer.observe(element)
    return () => { observer.disconnect() }
  }, [state])
  if (state?.owner !== owner || state.data !== data || state.format !== format
    || state.limits !== limits || state.attempt !== attempt) return loading
  if ('error' in state) return <div className={css.status} role="alert">
    <span>{t(state.error === 'tooLarge' || state.error === 'timeout' || state.error === 'encoding' ? state.error : 'invalid')}</span>
    <button type="button" className="rounded-md border border-ds-border px-3 py-1 text-[12px] text-ds-ink"
      onClick={() => { setAttempt(value => value + 1) }}>{t('retry')}</button>
  </div>
  const hasFormulas = state.value.sheets.some(sheet => sheet.celldata?.some(cell => cell.v?.f !== undefined))
  return <section className={css.body} data-excel-preview aria-label={t('title')}>
    <style>{scopedStyles}</style>
    {state.value.unsupportedFeatures.length > 0 && <div className={css.notice} role="note" data-excel-unsupported-notice>
      <DshExcelWarningTriangle size={16} />
      <span>{t('unsupportedNotice', { features: state.value.unsupportedFeatures.map(feature => t(feature)).join(t('featureSeparator')) })}</span>
    </div>}
    <div ref={workbookRef} className={`${css.workbook} ${hasFormulas ? css.withFormulaWarning : ''}`}>
      {hasFormulas && <Tooltip portal label={t('formulaWarning')} side="bottom" delayMs={500}>
        <button type="button" className={css.formulaWarning} aria-label={t('formulaWarning')} data-excel-formula-warning>
          <DshExcelWarningTriangle size={14} />
        </button>
      </Tooltip>}
      <Workbook key={`${attempt}:${t('language')}`} data={state.value.sheets} lang={t('language')}
        allowEdit={false} showToolbar={false} showFormulaBar showSheetTabs forceCalculation={false}
        readonlyViewport zoomControlTarget={zoomControlTarget}
        cellContextMenu={['copy']} headerContextMenu={[]} sheetTabContextMenu={[]} filterContextMenu={[]}
        protectedCopy={owner.protectedPresent || onProtectedCopy !== undefined ? protectedCopy : undefined} />
    </div>
  </section>
}
