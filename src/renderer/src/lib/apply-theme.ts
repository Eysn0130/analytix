import {
  DEFAULT_APP_MOTION_PREFERENCE,
  normalizeAppMotionPreference,
  writeFontStackFor,
  type AppMotionPreference,
  type WriteTypographySettingsV1
} from '@shared/app-settings'

export type ThemePreference = 'system' | 'light' | 'dark'
export type UiFontScale = 'small' | 'medium' | 'large'
export type MotionPreference = AppMotionPreference

let removeSystemListener: (() => void) | null = null
let removeMotionListener: (() => void) | null = null
const MOTION_QUERY = '(prefers-reduced-motion: reduce)'

function resolvedMode(pref: ThemePreference): 'light' | 'dark' {
  if (pref === 'dark') return 'dark'
  if (pref === 'light') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

/**
 * Applies `data-theme` on `<html>` for Tailwind `dark:` variants and CSS variables.
 */
export function applyTheme(pref: ThemePreference): void {
  removeSystemListener?.()
  removeSystemListener = null

  const root = document.documentElement
  const apply = (): void => {
    const mode = resolvedMode(pref)
    root.setAttribute('data-theme', mode)
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
