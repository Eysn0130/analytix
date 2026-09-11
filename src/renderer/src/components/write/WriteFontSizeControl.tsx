import { useState, type ReactElement } from 'react'
import { Minus, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  DEFAULT_WRITE_EDITOR_FONT_SIZE_PX,
  WRITE_EDITOR_FONT_SIZE_MAX,
  WRITE_EDITOR_FONT_SIZE_MIN
} from '@shared/app-settings'
import { ToolbarTooltip } from '../shell/ShellToolbar'

const FONT_SIZE_VAR = '--write-editor-font-size'

function readEditorFontSize(): number {
  if (typeof window === 'undefined') return DEFAULT_WRITE_EDITOR_FONT_SIZE_PX
  const raw = getComputedStyle(document.documentElement).getPropertyValue(FONT_SIZE_VAR)
  const parsed = Number.parseInt(raw, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : DEFAULT_WRITE_EDITOR_FONT_SIZE_PX
}

/**
 * Steps the editor font size by `delta`, clamped to the supported range. The CSS
 * variable is the live source of truth (set by applyWriteTypography), so we read
 * it back, nudge it for instant feedback, and persist the new value silently so
 * it survives restarts and stays in sync with the settings panel.
 */
function bumpEditorFontSize(delta: number): number {
  const next = Math.max(
    WRITE_EDITOR_FONT_SIZE_MIN,
    Math.min(WRITE_EDITOR_FONT_SIZE_MAX, readEditorFontSize() + delta)
  )
  document.documentElement.style.setProperty(FONT_SIZE_VAR, `${next}px`)
  void window.analytix?.settings?.saveSettingsSilent?.({ write: { typography: { fontSizePx: next } } })
  return next
}

/**
 * Compact in-toolbar stepper for the writing font size — the quick, in-context
 * counterpart to the slider in Settings. Self-contained: it drives the shared
 * `--write-editor-font-size` variable and persists through the silent settings
 * API, so it needs no store wiring.
 */
export function WriteFontSizeControl(): ReactElement {
  const { t } = useTranslation('common')
  const [size, setSize] = useState<number>(() => readEditorFontSize())

  const buttonClass =
    'flex h-6 w-6 items-center justify-center rounded-[7px] text-ds-faint transition hover:bg-[var(--ax-toolbar-hover-bg)] hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-40'

  return (
    <div
      className="flex h-[var(--ax-toolbar-button-size)] items-center gap-0.5 rounded-[8px] px-0.5 text-ds-faint"
      role="group"
      aria-label={t('writeFontSizeControl')}
    >
      <ToolbarTooltip label={t('writeFontSizeDecrease')}>
        <button
          type="button"
          onClick={() => setSize(bumpEditorFontSize(-1))}
          disabled={size <= WRITE_EDITOR_FONT_SIZE_MIN}
          className={buttonClass}
          aria-label={t('writeFontSizeDecrease')}
        >
          <Minus className="h-3.5 w-3.5" strokeWidth={2} />
        </button>
      </ToolbarTooltip>
      <span className="min-w-[26px] text-center text-[12px] font-medium tabular-nums text-ds-muted">
        {size}
      </span>
      <ToolbarTooltip label={t('writeFontSizeIncrease')}>
        <button
          type="button"
          onClick={() => setSize(bumpEditorFontSize(1))}
          disabled={size >= WRITE_EDITOR_FONT_SIZE_MAX}
          className={buttonClass}
          aria-label={t('writeFontSizeIncrease')}
        >
          <Plus className="h-3.5 w-3.5" strokeWidth={2} />
        </button>
      </ToolbarTooltip>
    </div>
  )
}
