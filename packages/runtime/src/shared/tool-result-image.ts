import type { ModelHistoryItem } from '../ports/model-client.js'

export type ToolResultImage = {
  mimeType: string
  dataBase64: string
  width?: number
  height?: number
}

export const IMAGE_TOOL_RESULT_TOKEN_ESTIMATE = 1_200

const MODEL_VISIBLE_IMAGE_KINDS = new Set(['image', 'computer_screenshot', 'computer_app_state'])

const EVICTED_IMAGE_PLACEHOLDER =
  '[older screenshot omitted to save context; take another screenshot if you need the current view]'

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

function toImage(value: unknown): ToolResultImage | null {
  if (!isRecord(value)) return null
  const dataBase64 = typeof value.data_base64 === 'string' ? value.data_base64 : ''
  const mimeType = typeof value.mime_type === 'string' ? value.mime_type : ''
  if (!dataBase64 || !mimeType) return null
  const width = typeof value.width === 'number' ? value.width : undefined
  const height = typeof value.height === 'number' ? value.height : undefined
  return {
    mimeType,
    dataBase64,
    ...(width !== undefined ? { width } : {}),
    ...(height !== undefined ? { height } : {})
  }
}

export function extractToolResultImages(output: unknown): ToolResultImage[] {
  if (!isRecord(output)) return []
  const kind = typeof output.kind === 'string' ? output.kind : ''
  if (!MODEL_VISIBLE_IMAGE_KINDS.has(kind)) return []
  const images: ToolResultImage[] = []
  if (Array.isArray(output.images)) {
    for (const entry of output.images) {
      const image = toImage(entry)
      if (image) images.push(image)
    }
  }
  const single = toImage(output)
  if (single && !images.some((image) => image.dataBase64 === single.dataBase64)) {
    images.push(single)
  }
  return images
}

export function isModelVisibleImageOutput(output: unknown): boolean {
  return extractToolResultImages(output).length > 0
}

export function toolResultTextWithoutImages(output: unknown): string {
  if (typeof output === 'string') return output
  if (!isRecord(output)) {
    try {
      return JSON.stringify(output) ?? ''
    } catch {
      return String(output)
    }
  }
  const clone: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(output)) {
    if (key === 'data_base64' || key === 'images') continue
    clone[key] = value
  }
  try {
    return JSON.stringify(clone)
  } catch {
    return ''
  }
}

function stripImagesFromOutput(output: unknown): unknown {
  if (!isRecord(output)) return output
  const clone: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(output)) {
    if (key === 'data_base64') {
      clone[key] = EVICTED_IMAGE_PLACEHOLDER
      continue
    }
    if (key === 'images') {
      clone.images_omitted = Array.isArray(value) ? value.length : 1
      continue
    }
    clone[key] = value
  }
  if (typeof clone.note !== 'string') clone.note = EVICTED_IMAGE_PLACEHOLDER
  return clone
}

export function capToolResultImages(history: ModelHistoryItem[], maxKept: number): ModelHistoryItem[] {
  const keep = Math.max(0, Math.floor(maxKept))
  const imageIndexes: number[] = []
  for (let index = 0; index < history.length; index += 1) {
    const item = history[index]
    if (item?.kind === 'tool_result' && isModelVisibleImageOutput(item.output)) {
      imageIndexes.push(index)
    }
  }
  if (imageIndexes.length <= keep) return history
  const evict = new Set(imageIndexes.slice(0, imageIndexes.length - keep))
  return history.map((item, index) => {
    if (!evict.has(index) || item.kind !== 'tool_result') return item
    return { ...item, output: stripImagesFromOutput(item.output) }
  })
}
