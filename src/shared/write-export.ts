import type { ObjectExportBinding } from '../../packages/runtime/src/contracts/object-editing'
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

export type WriteExportPayload = ObjectExportBinding & {
  format: WriteExportFormat
  typography?: WriteExportTypographyPayload | Partial<WriteTypographySettingsV1>
}

export type WriteRichClipboardPayload = ObjectExportBinding

/** Core-resolved bytes for the Main renderer service, never an IPC input. */
export type WriteExportDocument = {
  path: string
  content: string
  format: WriteExportFormat
  typography?: WriteExportPayload['typography']
}
export type WriteClipboardDocument = { path: string; content: string; workspaceRoot?: string }


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
