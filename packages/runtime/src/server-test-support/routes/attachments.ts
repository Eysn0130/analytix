import { AttachmentUploadRequest, type AttachmentMetadata } from '../../contracts/attachments.js'
import type { AttachmentStore } from '../../attachments/attachment-store.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import { readJsonBody } from '../read-json-body.js'
import { ERRORS } from './runtime-error.js'

export async function uploadAttachment(
  store: AttachmentStore | undefined,
  request: Request
): Promise<JsonResponse | Response> {
  if (!store) return ERRORS.unavailable('attachment store is unavailable')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = AttachmentUploadRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.attachmentValidation('invalid attachment upload body', parsed.error.issues)
  try {
    const data = decodeBase64(parsed.data.dataBase64)
    const attachment = await store.create({
      name: parsed.data.name,
      mimeType: parsed.data.mimeType,
      data,
      documentText: parsed.data.documentText,
      pageCount: parsed.data.pageCount,
      textFallback: parsed.data.textFallback,
      threadId: parsed.data.threadId,
      workspace: parsed.data.workspace
    })
    return jsonResponse({ attachment: publicAttachmentMetadata(attachment) }, 201)
  } catch (error) {
    return ERRORS.attachmentValidation(errorMessage(error))
  }
}

function decodeBase64(value: string): Buffer {
  const normalized = value.replace(/\s/g, '')
  if (normalized.length === 0 || normalized.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(normalized)) {
    throw new Error('attachment data is not valid base64')
  }
  const data = Buffer.from(normalized, 'base64')
  if (data.toString('base64') !== normalized) throw new Error('attachment data is not valid base64')
  return data
}

export async function getAttachmentMetadata(
  store: AttachmentStore | undefined,
  id: string,
  request: Request
): Promise<JsonResponse> {
  if (!store) return ERRORS.unavailable('attachment store is unavailable')
  const scope = attachmentScope(request)
  if (!scope) return ERRORS.attachmentValidation('thread_id and workspace are required')
  try {
    const attachment = await store.resolveContent(id, scope)
    return jsonResponse({ attachment: publicAttachmentMetadata(attachment) })
  } catch (error) {
    const message = errorMessage(error)
    return /not authorized/i.test(message) ? ERRORS.forbidden(message) : ERRORS.notFound(message)
  }
}

export async function getAttachmentContent(
  store: AttachmentStore | undefined,
  id: string,
  request: Request
): Promise<JsonResponse> {
  if (!store) return ERRORS.unavailable('attachment store is unavailable')
  const scope = attachmentScope(request)
  if (!scope) return ERRORS.attachmentValidation('thread_id and workspace are required')
  try {
    const attachment = await store.resolveContent(id, scope)
    return jsonResponse({
      attachment: publicAttachmentMetadata(attachment),
      dataBase64: attachment.data.toString('base64')
    })
  } catch (error) {
    const message = errorMessage(error)
    return /not authorized/i.test(message) ? ERRORS.forbidden(message) : ERRORS.notFound(message)
  }
}

function attachmentScope(request: Request): { threadId: string; workspace: string } | null {
  const url = new URL(request.url)
  const keys = [...url.searchParams.keys()]
  const threadIds = url.searchParams.getAll('thread_id')
  const workspaces = url.searchParams.getAll('workspace')
  if (keys.length !== 2 || new Set(keys).size !== 2 || threadIds.length !== 1 || workspaces.length !== 1) {
    return null
  }
  const threadId = threadIds[0]?.trim() ?? ''
  const workspace = workspaces[0]?.trim() ?? ''
  return threadId && workspace ? { threadId, workspace } : null
}

function publicAttachmentMetadata(attachment: AttachmentMetadata): Record<string, unknown> {
  return {
    id: attachment.id,
    name: attachment.name,
    kind: attachment.kind,
    mimeType: attachment.mimeType,
    byteSize: attachment.byteSize,
    scope: 'thread',
    ...(attachment.width !== undefined ? { width: attachment.width } : {}),
    ...(attachment.height !== undefined ? { height: attachment.height } : {}),
    ...(attachment.pageCount !== undefined ? { pageCount: attachment.pageCount } : {}),
    ...(attachment.truncated !== undefined ? { truncated: attachment.truncated } : {}),
    createdAt: attachment.createdAt,
    updatedAt: attachment.updatedAt
  }
}

export async function attachmentDiagnostics(
  store: AttachmentStore | undefined
): Promise<JsonResponse> {
  if (!store) {
    return jsonResponse({ enabled: false, rootDir: '', count: 0, totalBytes: 0 })
  }
  return jsonResponse(await store.diagnostics())
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
