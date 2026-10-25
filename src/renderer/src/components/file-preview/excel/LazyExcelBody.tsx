// Port/adapt of packages/client/ui-sidebar-documentpreview/src/client/excel/LazyExcelBody.tsx
// DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc; MIT, Copyright (c) 2026 DeepSeek.
// Complete notice: THIRD_PARTY_NOTICES.md.
/** Load the spreadsheet renderer only when a supported current workbook is opened. */
import { lazy, Suspense, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { ExcelFormat } from '@shared/excel/format'
import type { ExcelLimits } from '@shared/excel/model'
import type { WorkspaceFileTarget } from '@shared/workspace-file'
import { Loader2 } from '../../../design/AnalytixUiIcons'

/** Complete Host-read bytes and the current preview owner. */
export type ExcelBodyProps = {
  readonly owner: WorkspaceFileTarget
  readonly data: Uint8Array<ArrayBuffer>
  readonly format: ExcelFormat
  readonly limits: ExcelLimits
  /** Local chrome anchor; it carries no file or selection authority. */
  readonly zoomControlTarget?: HTMLElement | null
  /** Undefined keeps ordinary Copy. Null or a callback always takes over protected Copy. */
  readonly onProtectedCopy?: ((sheetIndex: number, sheetId: string, ranges: Array<{ row: [number, number]; column: [number, number] }>) => boolean) | null
}

export type LoadedExcelBodyProps = ExcelBodyProps & { readonly loading: ReactNode }

const LoadedExcelBody = lazy(async () => ({ default: (await import('./ExcelBody')).ExcelBody }))

/** Load the heavy renderer while preserving the current Host-selected format. */
export function LazyExcelBody(props: ExcelBodyProps): ReactNode {
  const { t } = useTranslation('excel')
  const loading = <div className="flex flex-1 items-center justify-center gap-2 text-[12px] text-ds-muted" role="status">
    <Loader2 className="h-4 w-4 animate-spin" strokeWidth={1.8} />
    {t('loading')}
  </div>
  return <Suspense fallback={loading}>
    <LoadedExcelBody {...props} loading={loading} />
  </Suspense>
}
