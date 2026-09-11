import type { WriteTypographySettingsV1 } from './app-settings-types'

export const WRITE_OFFICIAL_DOCUMENT_FONT_STACK =
  "'FangSong', 'FangSong_GB2312', 'STFangsong', 'Noto Serif SC', 'SimSun', 'Songti SC', serif"

export const WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY = {
  fontPreset: 'custom',
  customFontFamily: WRITE_OFFICIAL_DOCUMENT_FONT_STACK,
  fontSizePx: 21,
  lineHeight: 2
} satisfies Partial<WriteTypographySettingsV1>

export function isWriteOfficialDocumentTypography(
  typography: Pick<WriteTypographySettingsV1, 'fontPreset' | 'customFontFamily' | 'fontSizePx' | 'lineHeight'>
): boolean {
  return (
    typography.fontPreset === WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY.fontPreset &&
    typography.customFontFamily === WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY.customFontFamily &&
    typography.fontSizePx === WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY.fontSizePx &&
    typography.lineHeight === WRITE_OFFICIAL_DOCUMENT_TYPOGRAPHY.lineHeight
  )
}
