import type { WriteTypographySettingsV1 } from './app-settings-types'

export const WRITE_EXPORT_FORMATS = ['html', 'pdf', 'doc', 'docx'] as const

export type WriteExportFormat = (typeof WRITE_EXPORT_FORMATS)[number]

export type WriteExportTypographyPayload = {
  fontPreset?: string
  customFontFamily?: string
  fontSizePx?: number
  lineHeight?: number
  textAlign?: string
}

export type WriteExportPayload = {
  path: string
  format: WriteExportFormat
  content: string
  typography?: WriteExportTypographyPayload | Partial<WriteTypographySettingsV1>
}

export type WriteRichClipboardPayload = {
  path: string
  workspaceRoot?: string
  content: string
}

export type WriteExportResult =
  | {
      ok: true
      path: string
      format: WriteExportFormat
      exportedAt: string
    }
  | {
      ok: false
      canceled: true
      message?: string
    }
  | {
      ok: false
      canceled: false
      message: string
    }

export type WriteRichClipboardResult =
  | {
      ok: true
      copiedAt: string
    }
  | {
      ok: false
      message: string
    }
