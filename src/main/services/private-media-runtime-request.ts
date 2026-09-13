import type { AppSettingsV1 } from '../../shared/app-settings'
import type { ManagedFinalPublicationAuthorityPinV1 } from '../runtime/analytix-adapter'

const PRIVATE_MEDIA_PATH = '/v1/runtime/_private/media-execution'
const MAX_MEDIA_BYTES = 24 << 20
const FAILURE_CODES = new Set(['invalid_request', 'unavailable', 'privacy_unavailable', 'authority_changed', 'provider_failed'])

type MediaTransportResult = { ok: boolean; status: number; body: string }
export type PrivateMediaRuntimeRequest = (body: string) => Promise<MediaTransportResult>

type PrivateMediaTransportOptions = {
  loadSettings: () => Promise<AppSettingsV1>
  ensureRuntime: (settings: AppSettingsV1) => Promise<AppSettingsV1 | void>
  captureAuthorityPin: () => ManagedFinalPublicationAuthorityPinV1 | null
  isCurrentAuthorityPin: (pin: ManagedFinalPublicationAuthorityPinV1) => boolean
  getBaseUrl: (settings: AppSettingsV1) => string
  getProfileBinding: (settings: AppSettingsV1) => string
  authHeaders: (settings: AppSettingsV1) => Headers
}

function failure(status: number, code: string): MediaTransportResult {
  return { ok: false, status, body: JSON.stringify({ code }) }
}

async function readBoundedMediaResponse(response: Response): Promise<string | null> {
  const declaredLength = response.headers.get('content-length')
  if (declaredLength !== null && (!/^\d+$/.test(declaredLength) || Number(declaredLength) > MAX_MEDIA_BYTES)) {
    await response.body?.cancel()
    return null
  }
  if (!response.body) return null
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let size = 0
  try {
    while (true) {
      const chunk = await reader.read()
      if (chunk.done) break
      size += chunk.value.byteLength
      if (size > MAX_MEDIA_BYTES) {
        await reader.cancel()
        return null
      }
      chunks.push(chunk.value)
    }
    return new TextDecoder('utf-8', { fatal: true }).decode(Buffer.concat(chunks, size))
  } finally {
    reader.releaseLock()
  }
}

/** Fixed Main-only route: Core owns Registry selection and privacy projection.
 * Private media bytes never enter the generic renderer/store response projector.
 */
export function createPrivateMediaRuntimeRequest(options: PrivateMediaTransportOptions): PrivateMediaRuntimeRequest {
  return async (body) => {
    if (!body || Buffer.byteLength(body, 'utf8') > MAX_MEDIA_BYTES) return failure(400, 'invalid_request')
    let timeoutMs: number
    try {
      const request = JSON.parse(body) as Record<string, unknown>
      const keys = request.operation === 'speech.transcribe'
        ? ['schemaVersion', 'operation', 'audioBase64', 'mimeType', 'language', 'timeoutMs']
        : ['schemaVersion', 'operation', 'prompt', 'size', 'images', 'timeoutMs']
      if (request.schemaVersion !== 1 || !['image.generate', 'image.edit', 'speech.transcribe'].includes(String(request.operation)) ||
          Object.keys(request).some((key) => !keys.includes(key)) ||
          typeof request.timeoutMs !== 'number' || !Number.isInteger(request.timeoutMs) || request.timeoutMs <= 0 || request.timeoutMs > 300_000) {
        return failure(400, 'invalid_request')
      }
      timeoutMs = request.timeoutMs + 5_000
    } catch {
      return failure(400, 'invalid_request')
    }
    try {
      const initialSettings = await options.loadSettings()
      const settings = await options.ensureRuntime(initialSettings) ?? initialSettings
      const pin = options.captureAuthorityPin()
      if (!pin || !options.isCurrentAuthorityPin(pin)) return failure(503, 'unavailable')
      const baseUrl = options.getBaseUrl(settings)
      const base = new URL(baseUrl)
      if (base.origin !== pin.runtimeUrl || base.pathname !== '/' || base.search || base.hash || base.username || base.password ||
          base.protocol !== 'http:' || !['127.0.0.1', '[::1]', 'localhost'].includes(base.hostname)) {
        return failure(409, 'authority_changed')
      }
      const profileBinding = options.getProfileBinding(settings)
      const headers = options.authHeaders(settings)
      const authorization = headers.get('Authorization')
      if (!profileBinding || !authorization) return failure(503, 'unavailable')
      const authorityIsCurrent = async (): Promise<boolean> => {
        const currentSettings = await options.loadSettings()
        return options.isCurrentAuthorityPin(pin) &&
          options.getBaseUrl(currentSettings) === baseUrl &&
          options.getProfileBinding(currentSettings) === profileBinding &&
          options.authHeaders(currentSettings).get('Authorization') === authorization
      }
      if (!await authorityIsCurrent()) return failure(409, 'authority_changed')
      headers.set('Accept', 'application/json')
      headers.set('Content-Type', 'application/json')
      const response = await fetch(new URL(PRIVATE_MEDIA_PATH, base), {
        method: 'POST', headers, body, redirect: 'error', cache: 'no-store',
        signal: AbortSignal.timeout(timeoutMs)
      })
      const responseBody = await readBoundedMediaResponse(response)
      if (!await authorityIsCurrent()) return failure(409, 'authority_changed')
      if (responseBody === null || !/^application\/json(?:\s*;|$)/i.test(response.headers.get('content-type') ?? '')) {
        return failure(502, 'provider_failed')
      }
      if (!response.ok) {
        // Core's public HTTP projector normalizes several private codes. Keep
        // the original typed media status fallback without forwarding its body.
        let code = response.status === 400 ? 'invalid_request'
          : response.status === 409 ? 'authority_changed'
            : response.status === 503 ? 'unavailable' : 'provider_failed'
        try {
          const parsed = JSON.parse(responseBody) as { code?: unknown }
          if (typeof parsed.code === 'string' && FAILURE_CODES.has(parsed.code) &&
              (parsed.code !== 'privacy_unavailable' || response.status === 503)) code = parsed.code
        } catch {
          // Raw failure bodies and thrown network details never leave Main.
        }
        return failure(response.status, code)
      }
      return { ok: true, status: response.status, body: responseBody }
    } catch {
      return failure(503, 'unavailable')
    }
  }
}
