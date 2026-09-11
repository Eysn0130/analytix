import { randomBytes } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, isAbsolute, join, relative, resolve } from 'node:path'
import { canonicalPath, normalizePathSeparators, resolveTargetPathWithinWorkspace } from './workspace-paths'
import {
  normalizeWriteSettings,
  type AppSettingsV1,
  type WriteSettingsPatchV1
} from '../../shared/app-settings'
import {
  WRITE_DESIGN_DRAFT_DEFAULT_PROMPT,
  WRITE_INFOGRAPHIC_DEFAULT_PROMPT,
  WRITE_INFOGRAPHIC_MAX_TEXT_CHARS,
  type WriteInfographicKind,
  type WriteInfographicRequest,
  type WriteInfographicResult
} from '../../shared/write-infographic'
import { detectImage, mapImageSize } from './image-generation-client'

const INFOGRAPHIC_IMAGE_DIR = 'img'
const IMAGE_SIZE_TIER = '1K'
const MEDIA_PROMPT_MAX_CHARS = 1_500
const MAX_REFERENCE_IMAGE_BYTES = 10 * 1024 * 1024
const MAX_GENERATED_IMAGE_BYTES = 16 * 1024 * 1024
const REFERENCE_MIME_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])
const KIND_ASPECT_RATIO: Record<WriteInfographicKind, string> = {
  infographic: '3:4',
  design: '4:3'
}
const KIND_FILE_PREFIX: Record<WriteInfographicKind, string> = {
  infographic: 'infographic',
  design: 'design'
}
const KIND_DEFAULT_PROMPT: Record<WriteInfographicKind, string> = {
  infographic: WRITE_INFOGRAPHIC_DEFAULT_PROMPT,
  design: WRITE_DESIGN_DRAFT_DEFAULT_PROMPT
}

export type PrivateMediaImageRequest = {
  operation: 'image.generate' | 'image.edit'
  prompt: string
  size?: string
  timeoutMs: number
  images?: Array<{ name: string; mimeType: string; dataBase64: string }>
}

export type PrivateMediaImageResult =
  | { ok: true; imageBase64: string; mimeType: string }
  | { ok: false; code: 'invalid_request' | 'unavailable' | 'authority_changed' | 'provider_failed' }

export type ExecutePrivateMediaImageRequest = (
  request: PrivateMediaImageRequest
) => Promise<PrivateMediaImageResult>

export function buildWriteInfographicPrompt(
  text: string,
  customPrompt = '',
  kind: WriteInfographicKind = 'infographic',
  options: { maxPromptChars?: number } = {}
): string {
  const clipped = text.trim().slice(0, WRITE_INFOGRAPHIC_MAX_TEXT_CHARS)
  const prefix = customPrompt.trim() || KIND_DEFAULT_PROMPT[kind]
  const maxPromptChars = options.maxPromptChars
  if (typeof maxPromptChars === 'number' && Number.isFinite(maxPromptChars) && maxPromptChars > 0) {
    return fitPromptToMaxChars(prefix, clipped, maxPromptChars)
  }
  return `${prefix}\n\n${clipped}`
}

function fitPromptToMaxChars(prefix: string, text: string, maxChars: number): string {
  const separator = '\n\n'
  const max = Math.max(1, Math.floor(maxChars))
  const fittedPrefix = prefix.slice(0, Math.max(0, max - separator.length)).trimEnd()
  const textBudget = Math.max(0, max - fittedPrefix.length - separator.length)
  const fittedText = text.slice(0, textBudget).trimEnd()
  return fittedText ? `${fittedPrefix}${separator}${fittedText}` : fittedPrefix
}

async function readReferenceImage(
  workspaceRoot: string,
  rawPath: string | undefined
): Promise<{ image?: { name: string; mimeType: string; data: Buffer }; error?: string }> {
  const input = rawPath?.trim()
  if (!input) return {}

  const absolutePath = isAbsolute(input) ? resolve(input) : resolve(workspaceRoot, input)
  const rel = relative(workspaceRoot, absolutePath)
  if (!rel || rel.startsWith('..') || isAbsolute(rel)) {
    return { error: 'reference image must be inside the write workspace' }
  }

  let data: Buffer
  try {
    data = await readFile(absolutePath)
  } catch {
    return { error: 'reference image not found' }
  }
  if (data.byteLength > MAX_REFERENCE_IMAGE_BYTES) {
    return { error: `reference image exceeds ${MAX_REFERENCE_IMAGE_BYTES} byte limit` }
  }
  const detected = detectImage(data)
  if (!detected || !REFERENCE_MIME_TYPES.has(detected.mimeType)) {
    return { error: 'reference image must be png, jpeg, or webp' }
  }
  return {
    image: {
      name: basename(absolutePath),
      mimeType: detected.mimeType,
      data
    }
  }
}

function mediaIntentFromSettings(settings: AppSettingsV1): { defaultSize: string; timeoutMs: number } {
  const candidate = (settings as {
    runtime?: { imageGeneration?: { defaultSize?: unknown; timeoutMs?: unknown } }
  }).runtime?.imageGeneration
  const defaultSize = typeof candidate?.defaultSize === 'string' && candidate.defaultSize.length <= 32
    ? candidate.defaultSize.trim()
    : ''
  const timeoutMs = typeof candidate?.timeoutMs === 'number' && Number.isInteger(candidate.timeoutMs) &&
    candidate.timeoutMs > 0 && candidate.timeoutMs <= 300_000
    ? candidate.timeoutMs
    : 180_000
  return { defaultSize, timeoutMs }
}

export async function requestWriteInfographic(
  settings: AppSettingsV1,
  request: WriteInfographicRequest,
  options: { executeMediaRequest?: ExecutePrivateMediaImageRequest } = {}
): Promise<WriteInfographicResult> {
  const text = request.text.trim()
  if (!text) return { ok: false, message: 'selection text is empty' }

  const workspaceRoot = resolve(request.workspaceRoot)
  const filePath = resolve(request.filePath)
  const relativeToRoot = relative(workspaceRoot, filePath)
  if (!relativeToRoot || relativeToRoot.startsWith('..') || isAbsolute(relativeToRoot)) {
    return { ok: false, message: 'document must be inside the write workspace' }
  }
  if (!options.executeMediaRequest) {
    return { ok: false, message: 'image generation provider is not configured' }
  }

  const kind: WriteInfographicKind = request.kind ?? 'infographic'
  const mediaIntent = mediaIntentFromSettings(settings)
  const size = mediaIntent.defaultSize || mapImageSize(KIND_ASPECT_RATIO[kind], IMAGE_SIZE_TIER, undefined)
  const selectionAssist = normalizeWriteSettings(
    (settings as { write?: WriteSettingsPatchV1 }).write
  ).selectionAssist
  const customPrompt = kind === 'design'
    ? selectionAssist.designDraftPrompt
    : selectionAssist.infographicPrompt
  const reference = await readReferenceImage(workspaceRoot, request.referenceImagePath)
  if (reference.error) return { ok: false, message: reference.error }

  let executed: PrivateMediaImageResult
  try {
    executed = await options.executeMediaRequest({
      operation: reference.image ? 'image.edit' : 'image.generate',
      prompt: buildWriteInfographicPrompt(text, customPrompt, kind, { maxPromptChars: MEDIA_PROMPT_MAX_CHARS }),
      ...(size && size !== 'auto' ? { size } : {}),
      timeoutMs: mediaIntent.timeoutMs,
      ...(reference.image
        ? { images: [{ name: reference.image.name, mimeType: reference.image.mimeType, dataBase64: reference.image.data.toString('base64') }] }
        : {})
    })
  } catch {
    return { ok: false, message: 'image generation failed' }
  }
  if (!executed.ok) {
    return {
      ok: false,
      message: executed.code === 'unavailable'
        ? 'image generation provider is not configured'
        : executed.code === 'authority_changed'
          ? 'image generation provider changed during the request'
          : 'image generation failed'
    }
  }

  let image: Buffer
  try {
    image = Buffer.from(executed.imageBase64, 'base64')
  } catch {
    return { ok: false, message: 'image generation failed' }
  }
  const detected = detectImage(image)
  if (!detected || detected.mimeType !== executed.mimeType || image.byteLength === 0 || image.byteLength > MAX_GENERATED_IMAGE_BYTES) {
    return { ok: false, message: 'image generation failed' }
  }

  const ext = detected.mimeType === 'image/jpeg' ? 'jpg' : detected.mimeType === 'image/webp' ? 'webp' : 'png'
  const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 14)
  const fileName = `${KIND_FILE_PREFIX[kind]}-${stamp}-${randomBytes(2).toString('hex')}.${ext}`
  let absolutePath: string
  let markdownPath: string
  try {
    const imageDirSetting = request.imageDir?.trim() || INFOGRAPHIC_IMAGE_DIR
    const imageDir = await resolveTargetPathWithinWorkspace(imageDirSetting, workspaceRoot)
    await mkdir(imageDir, { recursive: true })
    absolutePath = join(imageDir, fileName)
    await writeFile(absolutePath, image)
    const canonicalRoot = await canonicalPath(workspaceRoot)
    const documentDir = join(canonicalRoot, dirname(relativeToRoot))
    markdownPath = normalizePathSeparators(relative(documentDir, absolutePath))
  } catch {
    return { ok: false, message: 'image output write failed' }
  }

  return {
    ok: true,
    relativePath: markdownPath,
    absolutePath,
    fileName
  }
}
