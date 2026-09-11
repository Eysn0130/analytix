import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from 'react'
import {
  AlignCenter,
  AlignJustify,
  AlignLeft,
  AlignRight,
  Check,
  ChevronDown,
  Pilcrow,
  ScrollText
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  WRITE_FONT_PRESETS,
  defaultWriteTypography,
  normalizeWriteTypography,
  type WriteFontPreset,
  type WriteTextAlign,
  type WriteTypographySettingsV1
} from '@shared/app-settings'
import {
  WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY,
  isWriteOfficialDocumentTypography
} from '@shared/write-official-document'
import { rendererRuntimeClient } from '../../agent/runtime-client'
import { applyWriteTypography } from '../../lib/apply-theme'
import { ToolbarTooltip } from '../shell/ShellToolbar'

type TypographyMenu = 'font' | 'size' | 'lineHeight' | 'align'

const QUICK_FONT_PRESETS = WRITE_FONT_PRESETS.filter((preset) => preset !== 'custom')
const FONT_PRESET_LABEL_KEYS: Record<WriteFontPreset, string> = {
  system: 'writeFontSystem',
  sourceHanSans: 'writeFontSourceHanSans',
  yahei: 'writeFontYahei',
  pingfang: 'writeFontPingfang',
  simhei: 'writeFontSimhei',
  simsun: 'writeFontSimsun',
  kaiti: 'writeFontKaiti',
  custom: 'writeFontCustom'
}

const FONT_PRESET_SHORT_LABEL_KEYS: Record<WriteFontPreset, string> = {
  system: 'writeTypographyFontShortSystem',
  sourceHanSans: 'writeTypographyFontShortSourceHanSans',
  yahei: 'writeTypographyFontShortYahei',
  pingfang: 'writeTypographyFontShortPingfang',
  simhei: 'writeTypographyFontShortSimhei',
  simsun: 'writeTypographyFontShortSimsun',
  kaiti: 'writeTypographyFontShortKaiti',
  custom: 'writeTypographyFontShortCustom'
}

const FONT_SIZE_OPTIONS = [12, 14, 16, 18, 20, 22, 24, 28]
const LINE_HEIGHT_OPTIONS = [1.4, 1.5, 1.8, 2, 2.2]

const ALIGN_OPTIONS: Array<{
  value: WriteTextAlign
  labelKey: string
  icon: typeof AlignLeft
}> = [
  { value: 'left', labelKey: 'writeAlignLeft', icon: AlignLeft },
  { value: 'center', labelKey: 'writeAlignCenter', icon: AlignCenter },
  { value: 'right', labelKey: 'writeAlignRight', icon: AlignRight },
  { value: 'justify', labelKey: 'writeAlignJustify', icon: AlignJustify }
]

function optionButtonClass(active: boolean): string {
  return `flex w-full items-center justify-between gap-3 rounded-xl px-3 py-2 text-left text-[13px] transition ${
    active ? 'bg-accent/12 text-accent' : 'text-ds-ink hover:bg-ds-hover/80'
  }`
}

function typographySegmentButtonClass(active: boolean, extraClass = ''): string {
  return `write-typography-segment-button${active ? ' is-active' : ''}${extraClass ? ` ${extraClass}` : ''}`
}

type Props = {
  onApplyOfficialDocumentFormat?: () => void
  onApplyTextAlign?: (alignment: WriteTextAlign) => void
  onTypographyChange?: (typography: WriteTypographySettingsV1) => void
  onResetOfficialDocumentFormat?: () => void
  typographyResetSignal?: number
}

export function WriteTypographyControls({
  onApplyOfficialDocumentFormat,
  onApplyTextAlign,
  onTypographyChange,
  onResetOfficialDocumentFormat,
  typographyResetSignal
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const { t: tSettings } = useTranslation('settings')
  const rootRef = useRef<HTMLDivElement>(null)
  const lastTypographyResetSignalRef = useRef(typographyResetSignal)
  const [openMenu, setOpenMenu] = useState<TypographyMenu | null>(null)
  const [typography, setTypography] = useState<WriteTypographySettingsV1>(() => defaultWriteTypography())
  const [lastTextAlign, setLastTextAlign] = useState<WriteTextAlign>(() => defaultWriteTypography().textAlign)

  useEffect(() => {
    if (typeof window === 'undefined') return undefined
    let cancelled = false
    void rendererRuntimeClient
      .getSettings()
      .then((settings) => {
        if (cancelled) return
        const next = normalizeWriteTypography(settings.write?.typography)
        setTypography(next)
        setLastTextAlign(next.textAlign)
        applyWriteTypography(next)
        onTypographyChange?.(next)
      })
      .catch(() => null)
    return () => {
      cancelled = true
    }
  }, [onTypographyChange])

  useEffect(() => {
    if (!openMenu || typeof window === 'undefined') return undefined
    const onPointerDown = (event: PointerEvent): void => {
      const target = event.target
      if (target instanceof Node && rootRef.current?.contains(target)) return
      setOpenMenu(null)
    }
    window.addEventListener('pointerdown', onPointerDown)
    return () => window.removeEventListener('pointerdown', onPointerDown)
  }, [openMenu])

  const fontPresetOptions = useMemo<WriteFontPreset[]>(
    () => (typography.fontPreset === 'custom' ? [...QUICK_FONT_PRESETS, 'custom'] : QUICK_FONT_PRESETS),
    [typography.fontPreset]
  )
  const fontButtonLabel = t(FONT_PRESET_SHORT_LABEL_KEYS[typography.fontPreset])
  const alignOption = ALIGN_OPTIONS.find((item) => item.value === lastTextAlign) ?? ALIGN_OPTIONS[0]
  const AlignIcon = alignOption.icon
  const officialDocumentFormatActive = isWriteOfficialDocumentTypography(typography)

  const updateTypography = useCallback((patch: Partial<WriteTypographySettingsV1>): void => {
    const next = normalizeWriteTypography({ ...typography, ...patch })
    setTypography(next)
    applyWriteTypography(next)
    onTypographyChange?.(next)
    void window.analytix?.settings?.saveSettingsSilent?.({ write: { typography: patch } })
      .then(() => rendererRuntimeClient.invalidateSettings())
      .catch(() => null)
    setOpenMenu(null)
  }, [onTypographyChange, typography])

  useEffect(() => {
    if (typographyResetSignal === undefined) return
    if (lastTypographyResetSignalRef.current === typographyResetSignal) return
    lastTypographyResetSignalRef.current = typographyResetSignal
    setLastTextAlign(defaultWriteTypography().textAlign)
    updateTypography(defaultWriteTypography())
  }, [typographyResetSignal, updateTypography])

  const updateTextAlign = (alignment: WriteTextAlign): void => {
    setLastTextAlign(alignment)
    onApplyTextAlign?.(alignment)
    setOpenMenu(null)
  }

  const applyOfficialDocumentFormat = (): void => {
    if (officialDocumentFormatActive) {
      updateTypography(defaultWriteTypography())
      onResetOfficialDocumentFormat?.()
      return
    }
    updateTypography(WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY)
    onApplyOfficialDocumentFormat?.()
  }

  const officialDocumentTooltip = officialDocumentFormatActive
    ? t('writeOfficialDocumentFormatResetTooltip')
    : t('writeOfficialDocumentFormatTooltip')

  const toggleMenu = (menu: TypographyMenu): void => {
    setOpenMenu((current) => (current === menu ? null : menu))
  }

  return (
    <div
      ref={rootRef}
      className="write-typography-controls"
      role="group"
      aria-label={t('writeTypographyControls')}
    >
      <div className="write-typography-segment relative flex items-center">
        <ToolbarTooltip label={officialDocumentTooltip}>
          <button
            type="button"
            onClick={applyOfficialDocumentFormat}
            className={typographySegmentButtonClass(
              officialDocumentFormatActive,
              'write-typography-official-button'
            )}
            aria-label={officialDocumentTooltip}
            aria-pressed={officialDocumentFormatActive}
          >
            <ScrollText className="write-typography-icon" strokeWidth={1.85} />
            <span className="write-typography-button-label">{t('writeOfficialDocumentFormat')}</span>
          </button>
        </ToolbarTooltip>
      </div>

      <div className="write-typography-segment relative flex items-center">
        <ToolbarTooltip label={t('writeTypographyFont')} hidden={openMenu === 'font'}>
          <button
            type="button"
            onClick={() => toggleMenu('font')}
            className={typographySegmentButtonClass(openMenu === 'font', 'write-typography-font-button')}
            aria-label={t('writeTypographyFont')}
            aria-haspopup="menu"
            aria-expanded={openMenu === 'font'}
            data-state={openMenu === 'font' ? 'open' : 'closed'}
          >
            <span className="write-typography-font-mark" aria-hidden="true">
              Aa
            </span>
            <span className="write-typography-button-label">{fontButtonLabel}</span>
            <ChevronDown className="write-typography-chevron" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>
        {openMenu === 'font' ? (
          <div
            role="menu"
            className="absolute right-0 top-full z-30 mt-2 min-w-[176px] overflow-hidden rounded-2xl border border-ds-border bg-ds-card/95 p-1.5 shadow-[0_22px_48px_rgba(20,47,95,0.16)] backdrop-blur-xl"
          >
            {fontPresetOptions.map((preset) => {
              const active = typography.fontPreset === preset
              return (
                <button
                  key={preset}
                  type="button"
                  role="menuitem"
                  onClick={() => updateTypography({ fontPreset: preset })}
                  className={optionButtonClass(active)}
                >
                  <span>{tSettings(FONT_PRESET_LABEL_KEYS[preset])}</span>
                  {active ? <Check className="h-3.5 w-3.5" strokeWidth={2} /> : null}
                </button>
              )
            })}
          </div>
        ) : null}
      </div>

      <div className="write-typography-segment relative flex items-center">
        <ToolbarTooltip label={t('writeTypographySize')} hidden={openMenu === 'size'}>
          <button
            type="button"
            onClick={() => toggleMenu('size')}
            className={typographySegmentButtonClass(openMenu === 'size', 'write-typography-size-button')}
            aria-label={t('writeTypographySize')}
            aria-haspopup="menu"
            aria-expanded={openMenu === 'size'}
            data-state={openMenu === 'size' ? 'open' : 'closed'}
          >
            <span className="write-typography-button-label tabular-nums">
              {typography.fontSizePx}
            </span>
            <ChevronDown className="write-typography-chevron" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>
        {openMenu === 'size' ? (
          <div
            role="menu"
            className="absolute right-0 top-full z-30 mt-2 min-w-[116px] overflow-hidden rounded-2xl border border-ds-border bg-ds-card/95 p-1.5 shadow-[0_22px_48px_rgba(20,47,95,0.16)] backdrop-blur-xl"
          >
            {FONT_SIZE_OPTIONS.map((size) => {
              const active = typography.fontSizePx === size
              return (
                <button
                  key={size}
                  type="button"
                  role="menuitem"
                  onClick={() => updateTypography({ fontSizePx: size })}
                  className={optionButtonClass(active)}
                >
                  <span className="tabular-nums">{t('writeTypographySizeValue', { value: size })}</span>
                  {active ? <Check className="h-3.5 w-3.5" strokeWidth={2} /> : null}
                </button>
              )
            })}
          </div>
        ) : null}
      </div>

      <div className="write-typography-segment relative flex items-center">
        <ToolbarTooltip label={t('writeTypographyLineHeight')} hidden={openMenu === 'lineHeight'}>
          <button
            type="button"
            onClick={() => toggleMenu('lineHeight')}
            className={typographySegmentButtonClass(openMenu === 'lineHeight', 'write-typography-line-height-button')}
            aria-label={t('writeTypographyLineHeight')}
            aria-haspopup="menu"
            aria-expanded={openMenu === 'lineHeight'}
            data-state={openMenu === 'lineHeight' ? 'open' : 'closed'}
          >
            <Pilcrow className="write-typography-icon" strokeWidth={1.85} />
            <span className="write-typography-button-label tabular-nums">
              {typography.lineHeight.toFixed(1)}
            </span>
            <ChevronDown className="write-typography-chevron" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>
        {openMenu === 'lineHeight' ? (
          <div
            role="menu"
            className="absolute right-0 top-full z-30 mt-2 min-w-[128px] overflow-hidden rounded-2xl border border-ds-border bg-ds-card/95 p-1.5 shadow-[0_22px_48px_rgba(20,47,95,0.16)] backdrop-blur-xl"
          >
            {LINE_HEIGHT_OPTIONS.map((lineHeight) => {
              const active = typography.lineHeight === lineHeight
              return (
                <button
                  key={lineHeight}
                  type="button"
                  role="menuitem"
                  onClick={() => updateTypography({ lineHeight })}
                  className={optionButtonClass(active)}
                >
                  <span className="tabular-nums">
                    {t('writeTypographyLineHeightValue', { value: lineHeight.toFixed(1) })}
                  </span>
                  {active ? <Check className="h-3.5 w-3.5" strokeWidth={2} /> : null}
                </button>
              )
            })}
          </div>
        ) : null}
      </div>

      <div className="write-typography-segment relative flex items-center">
        <ToolbarTooltip label={t('writeTypographyAlign')} hidden={openMenu === 'align'}>
          <button
            type="button"
            onClick={() => toggleMenu('align')}
            className={typographySegmentButtonClass(openMenu === 'align', 'write-typography-align-button')}
            aria-label={t('writeTypographyAlign')}
            aria-haspopup="menu"
            aria-expanded={openMenu === 'align'}
            data-state={openMenu === 'align' ? 'open' : 'closed'}
          >
            <AlignIcon className="write-typography-icon" strokeWidth={1.85} />
            <ChevronDown className="write-typography-chevron" strokeWidth={1.9} />
          </button>
        </ToolbarTooltip>
        {openMenu === 'align' ? (
          <div
            role="menu"
            className="absolute right-0 top-full z-30 mt-2 min-w-[148px] overflow-hidden rounded-2xl border border-ds-border bg-ds-card/95 p-1.5 shadow-[0_22px_48px_rgba(20,47,95,0.16)] backdrop-blur-xl"
          >
            {ALIGN_OPTIONS.map((item) => {
              const active = lastTextAlign === item.value
              const Icon = item.icon
              return (
                <button
                  key={item.value}
                  type="button"
                  role="menuitem"
                  onClick={() => updateTextAlign(item.value)}
                  className={optionButtonClass(active)}
                >
                  <span className="flex items-center gap-2">
                    <Icon className="h-4 w-4" strokeWidth={1.85} />
                    <span>{t(item.labelKey)}</span>
                  </span>
                  {active ? <Check className="h-3.5 w-3.5" strokeWidth={2} /> : null}
                </button>
              )
            })}
          </div>
        ) : null}
      </div>
    </div>
  )
}
