import { MemoryCreateRequest, MemoryUpdateRequest } from '../../contracts/memory.js'
import { PublicMemoryDiagnosticsV2 } from '../../contracts/runtime-tools.js'
import type { MemoryStore } from '../../memory/memory-store.js'
import { jsonResponse, type JsonResponse } from '../response.js'
import { readJsonBody } from '../read-json-body.js'
import { ERRORS } from './runtime-error.js'

export async function listMemories(store: MemoryStore | undefined, request: Request): Promise<JsonResponse> {
  if (!store) return ERRORS.unavailable('memory store is unavailable')
  const url = new URL(request.url)
  return jsonResponse({
    memories: await store.list({
      workspace: url.searchParams.get('workspace') ?? undefined,
      includeDeleted: url.searchParams.get('include_deleted') === 'true'
    })
  })
}

export async function createMemory(store: MemoryStore | undefined, request: Request): Promise<JsonResponse | Response> {
  if (!store) return ERRORS.unavailable('memory store is unavailable')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = MemoryCreateRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.validation('invalid memory create body', parsed.error.issues)
  return jsonResponse({ memory: await store.create(parsed.data) }, 201)
}

export async function updateMemory(store: MemoryStore | undefined, id: string, request: Request): Promise<JsonResponse | Response> {
  if (!store) return ERRORS.unavailable('memory store is unavailable')
  const body = await readJsonBody(request)
  if (!body.ok) return body.response
  const parsed = MemoryUpdateRequest.safeParse(body.value)
  if (!parsed.success) return ERRORS.validation('invalid memory update body', parsed.error.issues)
  try {
    return jsonResponse({ memory: await store.update(id, parsed.data) })
  } catch (error) {
    return ERRORS.notFound(errorMessage(error))
  }
}

export async function deleteMemory(store: MemoryStore | undefined, id: string): Promise<JsonResponse> {
  if (!store) return ERRORS.unavailable('memory store is unavailable')
  try {
    return jsonResponse({ memory: await store.delete(id) })
  } catch (error) {
    return ERRORS.notFound(errorMessage(error))
  }
}

export async function memoryDiagnostics(store: MemoryStore | undefined): Promise<JsonResponse> {
  if (!store) {
    return jsonResponse(PublicMemoryDiagnosticsV2.parse({
      enabled: false,
      status: 'unavailable',
      reasonCode: 'memory_store_unavailable'
    }))
  }
  try {
    const internal = await store.diagnostics()
    return jsonResponse(PublicMemoryDiagnosticsV2.parse({
      enabled: internal.enabled,
      status: 'ok',
      activeCount: internal.activeCount,
      tombstoneCount: internal.tombstoneCount
    }))
  } catch {
    return jsonResponse(PublicMemoryDiagnosticsV2.parse({
      enabled: true,
      status: 'unavailable',
      reasonCode: 'memory_store_read_failed'
    }))
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}
