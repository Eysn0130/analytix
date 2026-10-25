import {
  DEFAULT_APP_MOTION_PREFERENCE,
  normalizeAppMotionPreference,
  normalizeAppAccentColor,
  writeFontStackFor,
  type AppMotionPreference,
  type AppAccentColor,
  type WriteTypographySettingsV1
} from '@shared/app-settings'

export type ThemePreference = 'system' | 'light' | 'dark'
export type UiFontScale = 'small' | 'medium' | 'large'
export type MotionPreference = AppMotionPreference

let removeSystemListener: (() => void) | null = null
let removeMotionListener: (() => void) | null = null
const MOTION_QUERY = '(prefers-reduced-motion: reduce)'

// Fixed, contrast-paired presets: fill, foreground, text, hover, pressed, soft.
type AccentPalette = readonly [string, string, string, string, string, string]
const ACCENT_PALETTES: Record<AppAccentColor, Record<'light' | 'dark', AccentPalette>> = {
  blue: {
    light: ['#3659ef', '#ffffff', '#3154db', '#2949da', '#203bb8', '#edf1ff'],
    dark: ['#7291ff', '#101a36', '#a5b8ff', '#86a1ff', '#6482ed', '#303c64']
  },
  teal: {
    light: ['#0c7b8a', '#ffffff', '#096b78', '#096b79', '#075b67', '#e6f5f7'],
    dark: ['#67cad7', '#0b282d', '#93dce5', '#7cd6e0', '#57b5c2', '#223f45']
  },
  orange: {
    light: ['#ce7919', '#140b02', '#9b4d06', '#c16f13', '#b96810', '#fff1df'],
    dark: ['#ffb667', '#2b1a08', '#ffc98f', '#ffc180', '#eba153', '#4b3521']
  },
  magenta: {
    light: ['#ad287b', '#ffffff', '#96216a', '#98206b', '#82195a', '#fceaf5'],
    dark: ['#e78ac1', '#35152b', '#f3b2d7', '#efa0cf', '#d778b0', '#4c2940']
  },
  green: {
    light: ['#168348', '#ffffff', '#126d3d', '#116f3d', '#0b5c32', '#e7f5ed'],
    dark: ['#68cd93', '#0c2919', '#91dfb1', '#7bd9a3', '#58b781', '#243f31']
  }
}
const ACCENT_PROPERTIES = ['--ax-primary', '--ax-on-primary', '--ax-accent-text',
  '--ax-primary-hover', '--ax-primary-pressed', '--ax-primary-soft'] as const

function resolvedMode(pref: ThemePreference): 'light' | 'dark' {
  if (pref === 'dark') return 'dark'
  if (pref === 'light') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

/**
 * Applies `data-theme` on `<html>` for Tailwind `dark:` variants and CSS variables.
 */
export function applyTheme(pref: ThemePreference, accentColor?: AppAccentColor): void {
  removeSystemListener?.()
  removeSystemListener = null

  const root = document.documentElement
  const accent = normalizeAppAccentColor(accentColor)
  const apply = (): void => {
    const mode = resolvedMode(pref)
    root.setAttribute('data-theme', mode)
    root.setAttribute('data-accent', accent)
    ACCENT_PROPERTIES.forEach((property, index) => {
      root.style.setProperty(property, ACCENT_PALETTES[accent][mode][index])
    })
    // Independent surfaces choose their own mode from this same preset table.
    // CSS inheritance keeps them current without another theme state/listener.
    for (const surfaceMode of ['light', 'dark'] as const) {
      ACCENT_PROPERTIES.forEach((property, index) => {
        root.style.setProperty(`${property}-${surfaceMode}`, ACCENT_PALETTES[accent][surfaceMode][index])
      })
    }
    // The saved brand preference also wins over legacy theme-plugin root aliases.
    for (const [property, value] of Object.entries({
      '--ds-accent': 'var(--ax-primary)',
      '--ds-accent-soft': 'var(--ax-primary-soft)',
      '--ds-primary-button-fg': 'var(--ax-on-primary)',
      '--ds-composer-primary-action-bg': 'var(--ax-primary)',
      '--ds-composer-primary-action-fg': 'var(--ax-on-primary)',
      '--ax-border-focus': 'var(--ax-accent-text)',
      '--ds-selection': 'var(--ax-primary-soft)'
    })) root.style.setProperty(property, value)
  }

  if (pref === 'system') {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = (): void => {
      apply()
    }
    mq.addEventListener('change', onChange)
    removeSystemListener = (): void => {
      mq.removeEventListener('change', onChange)
    }
  }

  apply()
}

export function applyUiFontScale(scale: UiFontScale): void {
  const root = document.documentElement
  const factor =
    scale === 'small'
      ? '0.82'
      : scale === 'large'
        ? '1'
        : '0.88'
  root.style.setProperty('--ds-ui-scale', factor)
  // DSH has a 14px content baseline rather than these three preference tiers.
  // Retain Analytix's existing ratios and saved selection; small is its default.
  root.style.setProperty('--dsh-content-font-size', `${14 * Number(factor) / 0.82}px`)
}

export function applyCursorSpotlight(enabled: boolean): void {
  document.documentElement.dataset.cursorSpotlight = enabled ? 'on' : 'off'
}

function systemPrefersReducedMotion(): boolean {
  return typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia(MOTION_QUERY).matches
}

export function resolveMotionReduced(pref: MotionPreference | undefined): boolean {
  const normalized = normalizeAppMotionPreference(pref ?? DEFAULT_APP_MOTION_PREFERENCE)
  if (normalized === 'on') return false
  if (normalized === 'off') return true
  return systemPrefersReducedMotion()
}

export function applyMotionPreference(pref: MotionPreference | undefined): void {
  removeMotionListener?.()
  removeMotionListener = null

  const normalized = normalizeAppMotionPreference(pref ?? DEFAULT_APP_MOTION_PREFERENCE)
  const root = document.documentElement
  const apply = (): void => {
    root.dataset.motionPreference = normalized
    root.dataset.motionReduced = resolveMotionReduced(normalized) ? 'true' : 'false'
  }

  if (normalized === 'system' && typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    const mq = window.matchMedia(MOTION_QUERY)
    const onChange = (): void => {
      apply()
    }
    mq.addEventListener('change', onChange)
    removeMotionListener = (): void => {
      mq.removeEventListener('change', onChange)
    }
  }

  apply()
}

export function isShellMotionReduced(): boolean {
  const value = document.documentElement.dataset.motionReduced
  if (value === 'true') return true
  if (value === 'false') return false
  return systemPrefersReducedMotion()
}

/**
 * Pushes the Write editor typography onto CSS variables consumed by the rich
 * editor, the CodeMirror live appearance, and the markdown preview. Setting the
 * variables on `<html>` keeps chat surfaces untouched (only `.write-*` and the
 * editor theme read them) and live-updates open editors without a rebuild.
 */
export function applyWriteTypography(typography: WriteTypographySettingsV1): void {
  const root = document.documentElement.style
  root.setProperty('--write-editor-font-family', writeFontStackFor(typography.fontPreset, typography.customFontFamily))
  root.setProperty('--write-editor-font-size', `${typography.fontSizePx}px`)
  root.setProperty('--write-editor-line-height', String(typography.lineHeight))
  root.setProperty('--write-editor-text-align', typography.textAlign ?? 'left')
}

/**
 * Mirrors the active i18n locale onto `<html lang>` so screen readers,
 * browser spellcheck, and CSS `:lang()` selectors match the visible UI.
 */
export function applyDocumentLocale(locale: 'en' | 'zh'): void {
  const lang = locale === 'zh' ? 'zh-CN' : 'en'
  if (document.documentElement.getAttribute('lang') !== lang) {
    document.documentElement.setAttribute('lang', lang)
  }
}
