import { useState, type ReactElement, type RefObject } from 'react'
import {
  BookOpen,
  ChevronDown,
  Copy,
  Download,
  FileCode2,
  Loader2,
  Save,
  Sparkles
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { WriteTextAlign, WriteTypographySettingsV1 } from '@shared/app-settings'
import type { WriteExportFormat } from '@shared/write-export'
import type { WritePreviewMode, WriteSaveStatus } from '../../write/write-workspace-store'
import { ToolbarTooltip } from '../shell/ShellToolbar'
import { WriteTypographyControls } from './WriteTypographyControls'
import {
  WRITE_EXPORT_FORMATS,
  exportFormatLabel,
  toolbarIconButtonClass,
  type WriteModeMenuItem
} from './write-workspace-view-utils'

type Props = {
  activeFileIsImage: boolean
  activeFileIsPdf?: boolean
  activeFileIsText: boolean
  activeFileLabel: string
  activeFileName: string
  activeFilePath: string
  documentStatsLabel: string | null
  assistantOpen: boolean
  exportInFlight: boolean
  exportMenuOpen: boolean
  exportMenuRef: RefObject<HTMLDivElement | null>
  leftSidebarCollapsed: boolean
  liveModeActive: boolean
  modeMenuItems: WriteModeMenuItem[]
  modeMenuOpen: boolean
  modeMenuRef: RefObject<HTMLDivElement | null>
  onApplyOfficialDocumentFormat: () => void
  onApplyTextAlign: (alignment: WriteTextAlign) => void
  onResetOfficialDocumentFormat: () => void
  onTypographyChange: (typography: WriteTypographySettingsV1) => void
  onCopyRichText: () => void
  onExportFile: (format: WriteExportFormat) => void
  onSave: () => void
  readOnly: boolean
  saveLabel: string
  saveStatus: WriteSaveStatus
  reviewActive?: boolean
  setAssistantOpen: (open: boolean) => void
  setExportMenuOpen: (open: boolean | ((open: boolean) => boolean)) => void
  setModeMenuOpen: (open: boolean | ((open: boolean) => boolean)) => void
  setPreviewMode: (mode: WritePreviewMode) => void
}

export function WriteWorkspaceToolbar({
  activeFileIsImage,
  activeFileIsPdf = false,
  activeFileIsText,
  activeFileLabel,
  activeFileName,
  activeFilePath,
  documentStatsLabel,
  assistantOpen,
  exportInFlight,
  exportMenuOpen,
  exportMenuRef,
  leftSidebarCollapsed,
  liveModeActive,
  modeMenuItems,
  modeMenuOpen,
  modeMenuRef,
  onApplyOfficialDocumentFormat,
  onApplyTextAlign,
  onResetOfficialDocumentFormat,
  onTypographyChange,
  onCopyRichText,
  onExportFile,
  onSave,
  readOnly,
  saveLabel,
  saveStatus,
  reviewActive = false,
  setAssistantOpen,
  setExportMenuOpen,
  setModeMenuOpen,
  setPreviewMode
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const [typographyResetSignal, setTypographyResetSignal] = useState(0)
  const saveIconClass =
    reviewActive
      ? 'text-accent'
      : readOnly
        ? ''
        : saveStatus === 'error'
          ? 'text-red-500'
          : saveStatus === 'dirty'
            ? 'text-amber-500'
            : saveStatus === 'saving'
              ? 'text-sky-500'
              : saveStatus === 'saved'
                ? 'write-save-icon-saved'
                : ''
  const saveButtonLabel = reviewActive ? t('writeReviewPending') : saveLabel
  const saveTooltipLabel = activeFileIsPdf
    ? t('writePdfSaveDisabled')
    : activeFileIsImage
      ? t('writeImageSaveDisabled')
      : readOnly
        ? t('writeReadOnlySaveDisabled')
        : saveButtonLabel
  const exportTooltipLabel = exportInFlight ? t('writeExporting') : t('writeExport')
  const resetTypographyToDefault = (): void => {
    setTypographyResetSignal((signal) => signal + 1)
  }
  const handleLiveModeClick = (): void => {
    setPreviewMode('live')
    resetTypographyToDefault()
    onResetOfficialDocumentFormat()
  }

  if (activeFileIsPdf) {
    return (
      <div className="-mx-3 shrink-0 sm:-mx-4 md:-mx-6 lg:-mx-8">
        <header className="chat-topbar ds-chat-shell-header ds-topbar-surface write-pdf-topbar relative z-10 flex min-h-[46px] w-full shrink-0 items-stretch overflow-visible">
          <div aria-hidden="true" className="chat-topbar-drag-region" />
          <div className="chat-topbar-grid write-pdf-topbar-grid grid w-full min-w-0 items-center gap-2.5 px-3 py-2 sm:px-4 md:pl-5 md:pr-2">
            <div
              className={`chat-topbar-session ds-shell-controls-safe-motion flex min-w-0 items-center gap-2.5 ${
                leftSidebarCollapsed ? 'ds-shell-controls-safe-inset' : ''
              }`}
            >
              <div className="session-header-compact flex min-h-0 min-w-0 flex-1 items-center gap-2 text-left">
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 items-center gap-1.5">
                    <div className="min-w-0 truncate text-[13px] font-semibold leading-[17px] tracking-[-0.01em] text-ds-ink opacity-95">
                      {activeFileName}
                    </div>
                    <span className="ds-session-actions-anchor invisible pointer-events-none" aria-hidden="true">
                      <span className="ds-session-actions-trigger" />
                    </span>
                  </div>
                  <div className="session-header-compact-meta flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10.5px] leading-[14px] text-ds-faint">
                    <span className="session-meta-workspace max-w-[min(42vw,240px)] truncate">{activeFileLabel}</span>
                  </div>
                </div>
              </div>
            </div>

            <div className="chat-topbar-actions write-pdf-topbar-actions chat-workbench-topbar ds-no-drag flex min-w-0 flex-nowrap items-center justify-end gap-1.5 self-center">
              <div className="write-pdf-topbar-status">
                <BookOpen className="ds-toolbar-icon-svg" strokeWidth={1.85} />
                <span>{t('writePdfPreview')}</span>
                <span className="write-pdf-topbar-dot" aria-hidden="true" />
                <span>{t('writeReadOnly')}</span>
              </div>
              <ToolbarTooltip label={t('writeToggleAssistant')}>
                <button
                  type="button"
                  onClick={() => setAssistantOpen(!assistantOpen)}
                  className={toolbarIconButtonClass(assistantOpen)}
                  aria-label={t('writeToggleAssistant')}
                >
                  <Sparkles className="ds-toolbar-icon-svg" strokeWidth={1.85} />
                </button>
              </ToolbarTooltip>
            </div>
          </div>
        </header>
      </div>
    )
  }

  return (
    <div className="-mx-3 shrink-0 sm:-mx-4 md:-mx-6 lg:-mx-8">
      <header className="chat-topbar ds-chat-shell-header ds-topbar-surface relative z-10 flex min-h-[46px] w-full shrink-0 items-stretch overflow-visible">
        <div aria-hidden="true" className="chat-topbar-drag-region" />
        <div className="chat-topbar-grid write-workspace-toolbar-grid grid w-full min-w-0 items-center gap-2.5 px-3 py-2 sm:px-4 md:pl-5 md:pr-2">
          <div
            className={`chat-topbar-session ds-shell-controls-safe-motion flex min-w-0 items-center gap-2.5 ${
              leftSidebarCollapsed ? 'ds-shell-controls-safe-inset' : ''
            }`}
          >
            <div className="session-header-compact flex min-h-0 min-w-0 flex-1 items-center gap-2 text-left">
              <div className="min-w-0 flex-1">
                <div className="flex min-w-0 items-center gap-1.5">
                  <div className="min-w-0 truncate text-[13px] font-semibold leading-[17px] tracking-[-0.01em] text-ds-ink opacity-95">
                    {activeFileName}
                  </div>
                  <span className="ds-session-actions-anchor invisible pointer-events-none" aria-hidden="true">
                    <span className="ds-session-actions-trigger" />
                  </span>
                </div>
                <div className="session-header-compact-meta flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10.5px] leading-[14px] text-ds-faint">
                  <span className="session-meta-workspace max-w-[min(42vw,240px)] truncate">{activeFileLabel}</span>
                  {documentStatsLabel ? (
                    <>
                      <span className="session-meta-workspace-separator shrink-0 opacity-70">·</span>
                      <span className="session-meta-time shrink-0 tabular-nums">{documentStatsLabel}</span>
                    </>
                  ) : null}
                </div>
              </div>
            </div>
          </div>

          <div className="chat-topbar-actions write-workspace-toolbar-actions chat-workbench-topbar ds-no-drag flex min-w-0 shrink-0 flex-nowrap items-center justify-end gap-3 self-center">
            <div
              ref={modeMenuRef}
              className="write-workspace-toolbar-modes relative flex min-w-0 items-center justify-start"
            >
              <div
                className={`write-mode-split-control ${
                  liveModeActive ? 'is-live-active' : ''
                } ${modeMenuOpen ? 'is-menu-open' : ''} ${!activeFileIsText ? 'is-disabled' : ''}`}
              >
                <ToolbarTooltip label={t('writeModeLive')}>
                  <button
                    type="button"
                    onClick={handleLiveModeClick}
                    disabled={!activeFileIsText}
                    className="write-mode-split-live"
                    data-state={liveModeActive ? 'open' : 'closed'}
                    aria-label={t('writeModeLive')}
                    aria-pressed={liveModeActive}
                  >
                    <FileCode2 className="ds-toolbar-icon-svg write-mode-split-live-icon" strokeWidth={1.85} />
                    <span className="write-mode-split-label">{t('writeModeLiveShort')}</span>
                  </button>
                </ToolbarTooltip>
                <span className="write-mode-split-divider" aria-hidden="true" />
                <ToolbarTooltip label={t('writeModePreview')} hidden={modeMenuOpen}>
                  <button
                    type="button"
                    onClick={() => setModeMenuOpen((open) => !open)}
                    disabled={!activeFileIsText}
                    className="write-mode-split-menu"
                    data-state={modeMenuOpen ? 'open' : 'closed'}
                    aria-label={t('writeModePreview')}
                    aria-haspopup="menu"
                    aria-expanded={modeMenuOpen}
                  >
                    <ChevronDown
                      className="ds-toolbar-icon-svg write-mode-split-menu-icon"
                      strokeWidth={1.9}
                    />
                  </button>
                </ToolbarTooltip>
              </div>
              {modeMenuOpen ? (
                <div
                  role="menu"
                  className="absolute left-0 top-full z-30 mt-2 min-w-[188px] overflow-hidden rounded-2xl border border-slate-200 bg-white p-1.5 shadow-[0_18px_40px_rgba(20,47,95,0.12)] dark:border-white/10 dark:bg-[#131722]"
                >
                  {modeMenuItems.map((item) => (
                    <button
                      key={item.mode}
                      type="button"
                      role="menuitem"
                      disabled={!activeFileIsText}
                      onClick={() => {
                        setPreviewMode(item.mode)
                        setModeMenuOpen(false)
                      }}
                      className={`flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left text-[13px] transition ${
                        item.active
                          ? 'bg-accent/12 text-accent'
                          : 'text-ds-ink hover:bg-slate-100'
                      } ${!activeFileIsText ? 'cursor-not-allowed opacity-40' : ''}`}
                    >
                      <span className="flex items-center gap-2">
                        {item.icon}
                        <span>{item.shortLabel}</span>
                      </span>
                      {item.active ? (
                        <span className="text-[11px] font-semibold uppercase tracking-[0.08em]">
                          ON
                        </span>
                      ) : null}
                    </button>
                  ))}
                </div>
              ) : null}
            </div>
            {activeFileIsText ? (
              <WriteTypographyControls
                onApplyOfficialDocumentFormat={onApplyOfficialDocumentFormat}
                onApplyTextAlign={onApplyTextAlign}
                onTypographyChange={onTypographyChange}
                onResetOfficialDocumentFormat={onResetOfficialDocumentFormat}
                typographyResetSignal={typographyResetSignal}
              />
            ) : null}
            <div className="write-toolbar-action-icons flex min-w-0 items-center gap-1.5">
              <ToolbarTooltip label={t('writeToggleAssistant')}>
                <button
                  type="button"
                  onClick={() => setAssistantOpen(!assistantOpen)}
                  className={toolbarIconButtonClass(assistantOpen)}
                  aria-label={t('writeToggleAssistant')}
                >
                  <Sparkles className="ds-toolbar-icon-svg" strokeWidth={1.85} />
                </button>
              </ToolbarTooltip>
              <ToolbarTooltip label={saveTooltipLabel}>
                <button
                  type="button"
                  onClick={onSave}
                  disabled={!activeFilePath || !activeFileIsText || readOnly}
                  className={toolbarIconButtonClass()}
                  aria-label={saveTooltipLabel}
                >
                  <Save className={`ds-toolbar-icon-svg ${saveIconClass}`} strokeWidth={1.85} />
                </button>
              </ToolbarTooltip>
              <div ref={exportMenuRef} className="relative flex items-center">
                <ToolbarTooltip label={exportTooltipLabel} hidden={exportMenuOpen}>
                  <button
                    type="button"
                    onClick={() => setExportMenuOpen((open) => !open)}
                    disabled={!activeFilePath || !activeFileIsText || exportInFlight}
                    className={toolbarIconButtonClass(exportMenuOpen)}
                    aria-label={exportTooltipLabel}
                    aria-haspopup="menu"
                    aria-expanded={exportMenuOpen}
                  >
                    {exportInFlight ? (
                      <Loader2 className="ds-toolbar-icon-svg animate-spin" strokeWidth={1.85} />
                    ) : (
                      <Download className="ds-toolbar-icon-svg" strokeWidth={1.85} />
                    )}
                  </button>
                </ToolbarTooltip>
                {exportMenuOpen ? (
                  <div
                    role="menu"
                    className="absolute right-0 top-full z-30 mt-2 w-52 max-w-[calc(100vw-2rem)] overflow-hidden rounded-2xl border border-ds-border bg-ds-card/95 p-1.5 shadow-[0_22px_48px_rgba(20,47,95,0.16)] backdrop-blur-xl"
                  >
                    <button
                      type="button"
                      role="menuitem"
                      onClick={onCopyRichText}
                      className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-[13px] text-ds-ink transition hover:bg-ds-hover/80"
                    >
                      <span>{t('writeCopyRichText')}</span>
                      <Copy className="h-3.5 w-3.5 text-ds-faint" strokeWidth={1.9} />
                    </button>
                    <div className="my-1 h-px bg-ds-border-muted" />
                    {WRITE_EXPORT_FORMATS.map((format) => (
                      <button
                        key={format}
                        type="button"
                        role="menuitem"
                        onClick={() => onExportFile(format)}
                        className="flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-[13px] text-ds-ink transition hover:bg-ds-hover/80"
                      >
                        <span>{exportFormatLabel(format, t)}</span>
                        <span className="font-mono text-[11px] uppercase tracking-[0.08em] text-ds-faint">
                          {format}
                        </span>
                      </button>
                    ))}
                  </div>
                ) : null}
              </div>
            </div>
          </div>
        </div>
      </header>
    </div>
  )
}
