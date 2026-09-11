import { app, net, protocol } from 'electron'
import { stat } from 'node:fs/promises'
import { join, relative, resolve, sep } from 'node:path'
import { pathToFileURL } from 'node:url'

export const PACKAGED_RENDERER_SCHEME = 'analytix-app'
export const PACKAGED_RENDERER_HOST = 'renderer'
export const PACKAGED_RENDERER_ORIGIN =
  `${PACKAGED_RENDERER_SCHEME}://${PACKAGED_RENDERER_HOST}`
export const PACKAGED_RENDERER_ENTRY_URL = `${PACKAGED_RENDERER_ORIGIN}/index.html`

const MAX_RENDERER_REQUEST_URL_LENGTH = 8 * 1024
const MAX_RENDERER_PATH_SEGMENTS = 128
const MAX_RENDERER_PATH_SEGMENT_LENGTH = 255
const MAX_THREAD_ID_LENGTH = 512
const ALLOWED_RENDERER_DIRECTORIES = new Set(['assets', 'flow-runtime'])

function rawPathContainsUnsafeSegment(requestURL: string): boolean {
  const authorityStart = requestURL.indexOf('://')
  if (authorityStart < 0) return true
  const pathStart = requestURL.indexOf('/', authorityStart + 3)
  if (pathStart < 0) return true
  const suffixStart = requestURL.slice(pathStart).search(/[?#]/u)
  const rawPath = suffixStart < 0
    ? requestURL.slice(pathStart)
    : requestURL.slice(pathStart, pathStart + suffixStart)
  const rawSegments = rawPath.split('/').slice(1)
  if (
    rawSegments.length === 0 ||
    rawSegments.length > MAX_RENDERER_PATH_SEGMENTS ||
    rawSegments.some((segment) => !segment)
  ) {
    return true
  }
  for (const rawSegment of rawSegments) {
    let segment: string
    try {
      segment = decodeURIComponent(rawSegment)
    } catch {
      return true
    }
    if (
      segment === '.' ||
      segment === '..' ||
      segment.includes('/') ||
      segment.includes('\\') ||
      segment.includes('\0') ||
      segment.includes(':')
    ) {
      return true
    }
  }
  return false
}

export function registerPackagedRendererScheme(): void {
  if (!app.isPackaged) return
  protocol.registerSchemesAsPrivileged([{
    scheme: PACKAGED_RENDERER_SCHEME,
    privileges: {
      standard: true,
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true
    }
  }])
}

export function packagedRendererURL(threadId = ''): string {
  const url = new URL(PACKAGED_RENDERER_ENTRY_URL)
  if (threadId) url.searchParams.set('threadId', threadId)
  return url.toString()
}

export function isPackagedRendererNavigationURL(requestURL: string): boolean {
  if (!requestURL || requestURL.length > MAX_RENDERER_REQUEST_URL_LENGTH) return false
  let parsed: URL
  try {
    parsed = new URL(requestURL)
  } catch {
    return false
  }
  if (
    parsed.protocol !== `${PACKAGED_RENDERER_SCHEME}:` ||
    parsed.hostname !== PACKAGED_RENDERER_HOST ||
    parsed.port ||
    parsed.username ||
    parsed.password ||
    parsed.pathname !== '/index.html' ||
    parsed.hash
  ) {
    return false
  }
  const entries = [...parsed.searchParams.entries()]
  return entries.length === 0 ||
    (
      entries.length === 1 &&
      entries[0]?.[0] === 'threadId' &&
      Boolean(entries[0]?.[1]) &&
      entries[0]![1].length <= MAX_THREAD_ID_LENGTH
    )
}

export function resolvePackagedRendererAssetPath(
  requestURL: string,
  rendererRoot: string
): string | undefined {
  if (
    !requestURL ||
    requestURL.length > MAX_RENDERER_REQUEST_URL_LENGTH ||
    rawPathContainsUnsafeSegment(requestURL)
  ) {
    return undefined
  }
  let parsed: URL
  try {
    parsed = new URL(requestURL)
  } catch {
    return undefined
  }
  if (
    parsed.protocol !== `${PACKAGED_RENDERER_SCHEME}:` ||
    parsed.hostname !== PACKAGED_RENDERER_HOST ||
    parsed.port ||
    parsed.username ||
    parsed.password
  ) {
    return undefined
  }
  const rawSegments = parsed.pathname.split('/').slice(1)
  if (
    rawSegments.length === 0 ||
    rawSegments.length > MAX_RENDERER_PATH_SEGMENTS ||
    rawSegments.some((segment) => !segment)
  ) {
    return undefined
  }
  const segments: string[] = []
  for (const rawSegment of rawSegments) {
    let segment: string
    try {
      segment = decodeURIComponent(rawSegment)
    } catch {
      return undefined
    }
    if (
      !segment ||
      segment === '.' ||
      segment === '..' ||
      segment.length > MAX_RENDERER_PATH_SEGMENT_LENGTH ||
      segment.includes('/') ||
      segment.includes('\\') ||
      segment.includes('\0') ||
      segment.includes(':')
    ) {
      return undefined
    }
    segments.push(segment)
  }
  if (
    !(segments.length === 1 && segments[0] === 'index.html') &&
    !(segments.length > 1 && ALLOWED_RENDERER_DIRECTORIES.has(segments[0]!))
  ) {
    return undefined
  }
  const root = resolve(rendererRoot)
  const candidate = resolve(root, ...segments)
  const childPath = relative(root, candidate)
  if (!childPath || childPath === '..' || childPath.startsWith(`..${sep}`)) return undefined
  return candidate
}

function rejectedStaticResponse(status: number): Response {
  return new Response(null, {
    status,
    headers: {
      'Cache-Control': 'no-store',
      'Cross-Origin-Resource-Policy': 'same-origin',
      'X-Content-Type-Options': 'nosniff'
    }
  })
}

export function installPackagedRendererProtocol(): void {
  if (!app.isPackaged) return
  const rendererRoot = join(app.getAppPath(), 'out', 'renderer')
  protocol.handle(PACKAGED_RENDERER_SCHEME, async (request) => {
    if (request.method !== 'GET' && request.method !== 'HEAD') {
      return rejectedStaticResponse(405)
    }
    const assetPath = resolvePackagedRendererAssetPath(request.url, rendererRoot)
    if (!assetPath) return rejectedStaticResponse(404)
    try {
      const metadata = await stat(assetPath)
      if (!metadata.isFile()) return rejectedStaticResponse(404)
    } catch {
      return rejectedStaticResponse(404)
    }
    let response: Response
    try {
      response = await net.fetch(pathToFileURL(assetPath).toString())
    } catch {
      return rejectedStaticResponse(404)
    }
    if (!response.ok) return rejectedStaticResponse(response.status === 404 ? 404 : 502)
    const headers = new Headers(response.headers)
    headers.set('Cross-Origin-Resource-Policy', 'same-origin')
    headers.set('X-Content-Type-Options', 'nosniff')
    return new Response(request.method === 'HEAD' ? null : response.body, {
      status: response.status,
      statusText: response.statusText,
      headers
    })
  })
}
