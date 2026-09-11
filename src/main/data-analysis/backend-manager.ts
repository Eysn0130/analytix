import { app, type WebContents, type WebFrameMain } from 'electron'
import { createHmac, randomBytes, timingSafeEqual } from 'node:crypto'
import type { DataAnalysisBackendRuntimeState, DataAnalysisRuntimeInfo } from '../../shared/data-analysis'
import {
  PACKAGED_RENDERER_ENTRY_URL,
  PACKAGED_RENDERER_HOST,
  PACKAGED_RENDERER_ORIGIN,
  PACKAGED_RENDERER_SCHEME
} from '../packaged-renderer-protocol'

export const DATA_ANALYSIS_AUTH_HEADER = 'X-Analytix-Data-Analysis-Token'
export const DATA_ANALYSIS_AUTH_ENV = 'ANALYTIX_DATA_ANALYSIS_AUTH_TOKEN'
export const DATA_ANALYSIS_LAUNCH_ID_ENV = 'ANALYTIX_DATA_ANALYSIS_LAUNCH_ID'
export const DATA_ANALYSIS_LAUNCH_READY_PROTOCOL = 'analytix-data-analysis-launch-ready-v1'
export const DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER = 'data_analysis_native_authority_unavailable'

const DATA_ANALYSIS_TOKEN_BYTES = 32
const DATA_ANALYSIS_LAUNCH_VALUE_PATTERN = /^[A-Za-z0-9_-]{43}$/u
const DATA_ANALYSIS_LAUNCH_PROOF_PATTERN = /^[a-f0-9]{64}$/u
const LOOPBACK_RENDERER_HOSTS = new Set(['127.0.0.1', '::1', 'localhost'])

type StateListener = (state: DataAnalysisBackendRuntimeState) => void

type RendererAuthRequestDetails = {
  url: string
  requestHeaders: Record<string, string>
  resourceType?: string
  webContentsId?: number
  webContents?: Pick<WebContents, 'id' | 'getURL' | 'isDestroyed' | 'mainFrame'>
  frame?: Pick<WebFrameMain, 'url'> | null
}

type RendererHeadersReceivedDetails = {
  url: string
  statusCode: number
  resourceType?: string
}

type RendererAuthPolicy = {
  corsOrigins: string[]
  rendererEntryUrl: string
  rendererOrigin?: string
}

type RendererAuthority = {
  contents: Pick<WebContents, 'id' | 'getURL' | 'isDestroyed' | 'mainFrame'>
  frame: Pick<WebFrameMain, 'url'>
  rendererEntryUrl: string
  generation: number
}

type RegisteredRendererAuthority = RendererAuthority & {
  contents: WebContents
  frame: WebFrameMain
  leases: Set<AbortController>
  onDestroyed: () => void
  onDidStartNavigation: (event: Electron.Event<Electron.WebContentsDidStartNavigationEventParams>) => void
}

export type DataAnalysisRendererAuthorityLease = {
  signal: AbortSignal
  isCurrent: () => boolean
  release: () => void
}

type LaunchReadyProof = {
  protocol: string
  service: string
  status: string
  launchId: string
  challenge: string
  pid: number
  proof: string
}

function nowIso(): string {
  return new Date().toISOString()
}

function loopbackRendererOrigin(rawUrl: string | undefined): string | undefined {
  const value = rawUrl?.trim()
  if (!value) return undefined
  try {
    const parsed = new URL(value)
    if (!['http:', 'https:'].includes(parsed.protocol)) return undefined
    if (!LOOPBACK_RENDERER_HOSTS.has(parsed.hostname.toLowerCase())) return undefined
    if (parsed.username || parsed.password) return undefined
    return parsed.origin
  } catch {
    return undefined
  }
}

function normalizedRendererEntryUrl(rawUrl: string): string | undefined {
  try {
    const parsed = new URL(rawUrl)
    parsed.search = ''
    parsed.hash = ''
    return parsed.toString()
  } catch {
    return undefined
  }
}

function packagedRendererOrigin(rawUrl: string | undefined): string | undefined {
  const value = rawUrl?.trim()
  if (!value) return undefined
  try {
    const parsed = new URL(value)
    if (
      parsed.protocol !== `${PACKAGED_RENDERER_SCHEME}:` ||
      parsed.hostname !== PACKAGED_RENDERER_HOST ||
      parsed.port ||
      parsed.username ||
      parsed.password
    ) {
      return undefined
    }
    return PACKAGED_RENDERER_ORIGIN
  } catch {
    return undefined
  }
}

export function resolveDataAnalysisRendererAuthPolicy(
  rendererUrl = process.env.ELECTRON_RENDERER_URL,
  isPackaged = app.isPackaged
): RendererAuthPolicy {
  const rendererOrigin = loopbackRendererOrigin(isPackaged ? undefined : rendererUrl)
  return rendererOrigin
    ? {
        corsOrigins: [rendererOrigin],
        rendererOrigin,
        rendererEntryUrl: normalizedRendererEntryUrl(String(rendererUrl)) ?? ''
      }
    : {
        corsOrigins: [PACKAGED_RENDERER_ORIGIN],
        rendererOrigin: PACKAGED_RENDERER_ORIGIN,
        rendererEntryUrl: PACKAGED_RENDERER_ENTRY_URL
      }
}

export function createDataAnalysisLaunchToken(): string {
  return randomBytes(DATA_ANALYSIS_TOKEN_BYTES).toString('base64url')
}

export function createDataAnalysisLaunchProof(options: {
  token: string
  launchId: string
  challenge: string
  service: string
  status: string
  pid: number
}): string {
  const payload = [
    DATA_ANALYSIS_LAUNCH_READY_PROTOCOL,
    options.launchId,
    options.challenge,
    options.service,
    options.status,
    String(options.pid)
  ].join('\n')
  return createHmac('sha256', options.token).update(payload, 'utf8').digest('hex')
}

export function withDataAnalysisAuthHeader(init: RequestInit, token: string): RequestInit {
  const headers = new Headers(init.headers)
  headers.set(DATA_ANALYSIS_AUTH_HEADER, token)
  return { ...init, redirect: 'manual', headers }
}

function headerValue(headers: Record<string, string>, name: string): string {
  const entry = Object.entries(headers).find(([key]) => key.toLowerCase() === name.toLowerCase())
  return entry?.[1] ?? ''
}

function withoutRecordAuthHeader(headers: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(headers).filter(([key]) => key.toLowerCase() !== DATA_ANALYSIS_AUTH_HEADER.toLowerCase())
  )
}

function rendererRequestIsAuthorized(
  details: RendererAuthRequestDetails,
  policy: RendererAuthPolicy,
  authority: RendererAuthority | undefined
): boolean {
  const webContents = details.webContents
  const frame = details.frame
  if (
    !authority ||
    !webContents ||
    !frame ||
    webContents.isDestroyed() ||
    details.webContentsId !== authority.contents.id ||
    webContents.id !== authority.contents.id ||
    webContents !== authority.contents ||
    frame !== authority.frame ||
    webContents.mainFrame !== authority.frame ||
    !['xhr', 'webSocket'].includes(String(details.resourceType || ''))
  ) {
    return false
  }
  let rendererUrl = ''
  try {
    rendererUrl = webContents.getURL()
  } catch {
    return false
  }
  const normalizedContentsUrl = normalizedRendererEntryUrl(rendererUrl)
  const normalizedFrameUrl = normalizedRendererEntryUrl(frame.url)
  if (
    !normalizedContentsUrl ||
    normalizedContentsUrl !== authority.rendererEntryUrl ||
    normalizedFrameUrl !== authority.rendererEntryUrl ||
    authority.rendererEntryUrl !== policy.rendererEntryUrl
  ) {
    return false
  }
  const requestOrigin = headerValue(details.requestHeaders, 'Origin')
  if (policy.rendererOrigin) {
    const contentsOrigin = policy.rendererOrigin === PACKAGED_RENDERER_ORIGIN
      ? packagedRendererOrigin(rendererUrl)
      : loopbackRendererOrigin(rendererUrl)
    const frameOrigin = policy.rendererOrigin === PACKAGED_RENDERER_ORIGIN
      ? packagedRendererOrigin(frame.url)
      : loopbackRendererOrigin(frame.url)
    return (
      contentsOrigin === policy.rendererOrigin &&
      frameOrigin === policy.rendererOrigin &&
      requestOrigin === policy.rendererOrigin
    )
  }
  return false
}

export function injectDataAnalysisRendererAuthHeader(
  details: RendererAuthRequestDetails,
  options: {
    apiBase: string
    token: string
    policy: RendererAuthPolicy
    authority?: RendererAuthority
  }
): Record<string, string> {
  const sanitizedHeaders = withoutRecordAuthHeader(details.requestHeaders)
  let target: URL
  let api: URL
  try {
    target = new URL(details.url)
    api = new URL(options.apiBase)
  } catch {
    return sanitizedHeaders
  }
  const expectedProtocol = target.protocol === 'ws:' ? 'ws:' : api.protocol
  if (
    target.origin !== `${expectedProtocol}//${api.host}` ||
    !rendererRequestIsAuthorized(details, options.policy, options.authority)
  ) {
    return sanitizedHeaders
  }
  return { ...sanitizedHeaders, [DATA_ANALYSIS_AUTH_HEADER]: options.token }
}

export function shouldCancelDataAnalysisWebSocketRedirect(
  details: RendererHeadersReceivedDetails,
  apiBase: string
): boolean {
  if (details.resourceType !== 'webSocket' || details.statusCode < 300 || details.statusCode >= 400) return false
  try {
    const target = new URL(details.url)
    const api = new URL(apiBase)
    return target.protocol === 'ws:' && target.host === api.host
  } catch {
    return false
  }
}

function isExactLaunchReadyProof(value: unknown): value is LaunchReadyProof {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  return (
    Object.keys(record).sort().join(',') === 'challenge,launchId,pid,proof,protocol,service,status' &&
    typeof record.protocol === 'string' &&
    typeof record.service === 'string' &&
    typeof record.status === 'string' &&
    typeof record.launchId === 'string' &&
    typeof record.challenge === 'string' &&
    Number.isSafeInteger(record.pid) &&
    typeof record.proof === 'string'
  )
}

export function verifyDataAnalysisLaunchReadyProof(
  value: unknown,
  expected: { token: string; launchId: string; challenge: string; pid: number }
): boolean {
  if (!isExactLaunchReadyProof(value)) return false
  if (
    value.protocol !== DATA_ANALYSIS_LAUNCH_READY_PROTOCOL ||
    value.service !== 'analytix-data-analysis' ||
    value.status !== 'ok' ||
    value.launchId !== expected.launchId ||
    value.challenge !== expected.challenge ||
    value.pid !== expected.pid ||
    !DATA_ANALYSIS_LAUNCH_VALUE_PATTERN.test(value.launchId) ||
    !DATA_ANALYSIS_LAUNCH_VALUE_PATTERN.test(value.challenge) ||
    !DATA_ANALYSIS_LAUNCH_PROOF_PATTERN.test(value.proof)
  ) {
    return false
  }
  const proof = createDataAnalysisLaunchProof({
    token: expected.token,
    launchId: expected.launchId,
    challenge: expected.challenge,
    service: value.service,
    status: value.status,
    pid: value.pid
  })
  return timingSafeEqual(Buffer.from(proof, 'ascii'), Buffer.from(value.proof, 'ascii'))
}

function unavailableState(): DataAnalysisBackendRuntimeState {
  return {
    phase: 'failed',
    generation: 0,
    detail: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
    blocker: DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER,
    authority: 'unavailable',
    terminal: true,
    restartCount: 0,
    updatedAt: nowIso()
  }
}

export class DataAnalysisBackendManager {
  private readonly state = unavailableState()
  private rendererAuthorityGeneration = 0
  private readonly rendererAuthorities = new Map<number, RegisteredRendererAuthority>()
  private readonly listeners = new Set<StateListener>()

  onState(listener: StateListener): () => void {
    this.listeners.add(listener)
    try {
      listener(this.getState())
    } catch {
      // A renderer notification failure cannot mutate backend authority.
    }
    return () => this.listeners.delete(listener)
  }

  registerRenderer(contents: WebContents, frame: WebFrameMain | null = contents.mainFrame): boolean {
    if (!frame || contents.isDestroyed() || frame !== contents.mainFrame) return false
    const policy = resolveDataAnalysisRendererAuthPolicy()
    if (
      normalizedRendererEntryUrl(contents.getURL()) !== policy.rendererEntryUrl ||
      normalizedRendererEntryUrl(frame.url) !== policy.rendererEntryUrl
    ) {
      return false
    }
    const existing = this.rendererAuthorities.get(contents.id)
    if (
      existing &&
      existing.contents === contents &&
      existing.frame === frame &&
      existing.rendererEntryUrl === policy.rendererEntryUrl
    ) {
      return true
    }
    if (existing) this.revokeRenderer(contents.id, existing.contents)
    const generation = ++this.rendererAuthorityGeneration
    const onDestroyed = (): void => this.revokeRenderer(contents.id, contents)
    const onDidStartNavigation = (event: Electron.Event<Electron.WebContentsDidStartNavigationEventParams>): void => {
      if (event.isMainFrame && !event.isSameDocument) this.revokeRenderer(contents.id, contents)
    }
    contents.once('destroyed', onDestroyed)
    contents.on('did-start-navigation', onDidStartNavigation)
    this.rendererAuthorities.set(contents.id, {
      contents,
      frame,
      rendererEntryUrl: policy.rendererEntryUrl,
      generation,
      leases: new Set(),
      onDestroyed,
      onDidStartNavigation
    })
    return true
  }

  validateRenderer(contents: WebContents, frame: WebFrameMain | null = contents.mainFrame): boolean {
    return this.getRendererAuthorityGeneration(contents, frame) !== undefined
  }

  getRendererAuthorityGeneration(
    contents: WebContents,
    frame: WebFrameMain | null = contents.mainFrame
  ): number | undefined {
    if (!frame || contents.isDestroyed() || frame !== contents.mainFrame) return undefined
    const authority = this.rendererAuthorities.get(contents.id)
    if (!authority || authority.contents !== contents || authority.frame !== frame) return undefined
    let contentsUrl = ''
    let frameUrl = ''
    try {
      contentsUrl = contents.getURL()
      frameUrl = frame.url
    } catch {
      this.revokeRenderer(contents.id, contents)
      return undefined
    }
    if (
      normalizedRendererEntryUrl(contentsUrl) !== authority.rendererEntryUrl ||
      normalizedRendererEntryUrl(frameUrl) !== authority.rendererEntryUrl ||
      authority.rendererEntryUrl !== resolveDataAnalysisRendererAuthPolicy().rendererEntryUrl
    ) {
      this.revokeRenderer(contents.id, contents)
      return undefined
    }
    return authority.generation
  }

  getAuthorizedRenderers(): WebContents[] {
    const renderers: WebContents[] = []
    for (const authority of this.rendererAuthorities.values()) {
      if (this.validateRenderer(authority.contents, authority.frame)) renderers.push(authority.contents)
    }
    return renderers
  }

  acquireRendererAuthorityLease(
    webContentsId: number,
    rendererGeneration: number
  ): DataAnalysisRendererAuthorityLease | null {
    if (!Number.isSafeInteger(webContentsId) || webContentsId <= 0 ||
      !Number.isSafeInteger(rendererGeneration) || rendererGeneration <= 0) {
      return null
    }
    const authority = this.rendererAuthorities.get(webContentsId)
    if (!authority || authority.generation !== rendererGeneration ||
      this.getRendererAuthorityGeneration(authority.contents, authority.frame) !== rendererGeneration) {
      return null
    }
    const controller = new AbortController()
    authority.leases.add(controller)
    let released = false
    return Object.freeze({
      signal: controller.signal,
      isCurrent: () => !released && !controller.signal.aborted &&
        this.rendererAuthorities.get(webContentsId) === authority &&
        this.getRendererAuthorityGeneration(authority.contents, authority.frame) === rendererGeneration,
      release: () => {
        if (released) return
        released = true
        authority.leases.delete(controller)
      }
    })
  }

  getState(): DataAnalysisBackendRuntimeState {
    return { ...this.state }
  }

  getRuntimeInfo(): DataAnalysisRuntimeInfo {
    return {
      isPackaged: app.isPackaged,
      backend: {
        ...this.getState(),
        managed: false
      }
    }
  }

  async ensureBackend(): Promise<DataAnalysisBackendRuntimeState> {
    return this.getState()
  }

  async request(_path: string, _init: RequestInit = {}): Promise<Response> {
    throw new Error(DATA_ANALYSIS_NATIVE_AUTHORITY_BLOCKER)
  }

  async stopAndWait(): Promise<void> {
    for (const authority of [...this.rendererAuthorities.values()]) {
      this.revokeRenderer(authority.contents.id, authority.contents)
    }
    return undefined
  }

  private revokeRenderer(id: number, expected: WebContents): void {
    const authority = this.rendererAuthorities.get(id)
    if (!authority || authority.contents !== expected) return
    this.rendererAuthorities.delete(id)
    for (const lease of authority.leases) lease.abort()
    authority.leases.clear()
    try {
      authority.contents.removeListener('did-start-navigation', authority.onDidStartNavigation)
      authority.contents.removeListener('destroyed', authority.onDestroyed)
    } catch {
      // The authority is already revoked.
    }
  }
}
