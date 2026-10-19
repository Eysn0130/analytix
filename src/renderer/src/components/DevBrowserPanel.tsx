import type { FormEvent, ReactElement } from 'react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe2,
  Loader2,
  MessageSquarePlus,
  Plus,
  RefreshCw,
  ArrowRight as Send,
  Sparkles,
  X
} from '../design/AnalytixUiIcons'
import type { NativeReference } from '../office/native-reference-store'
import type { ChatBlock } from '../agent/types'
import {
  DEFAULT_DEV_PREVIEW_URL,
  normalizeDevPreviewUrlInput
} from '@shared/dev-preview-url'
import {
  extractDetectedDevPreviewUrls,
  formatDevPreviewUrlLabel
} from '../lib/dev-preview-detection'
import {
  readBrowserStorageItem,
  removeBrowserStorageItem,
  writeBrowserStorageItem
} from '../lib/browser-storage'
import { useChatStore } from '../store/chat-store'
import { useNativeReferenceStore } from '../office/native-reference-store'
import { browserScopeSchema } from '../../../../packages/runtime/src/contracts/browser-selection'
import { WorkspaceViewerHeader, WorkspaceViewerIconButton, WorkspaceViewerMenu, type WorkspaceViewerControls } from './workbench/WorkspaceViewerHeader'

type DevWebviewTag = HTMLElement & {
  getWebContentsId(): number
  canGoBack(): boolean
  canGoForward(): boolean
  getURL(): string
  goBack(): void
  goForward(): void
  reloadIgnoringCache(): void
}

type WebviewNavigateEvent = Event & {
  url: string
}

type WebviewFailLoadEvent = Event & {
  errorCode: number
  errorDescription: string
  isMainFrame: boolean
}

type WebviewTitleEvent = Event & {
  title: string
}

const PREVIEW_URL_STORAGE_KEY = 'analytix.devPreview.url'
const PREVIEW_AUTO_FOLLOW_STORAGE_KEY = 'analytix.devPreview.autoFollow'

function readStoredUrl(): string | null {
  try {
    const raw = readBrowserStorageItem(PREVIEW_URL_STORAGE_KEY)
    const normalized = raw ? normalizeDevPreviewUrlInput(raw) : null
    if (!normalized) return null
    if (normalized === DEFAULT_DEV_PREVIEW_URL) {
      removeBrowserStorageItem(PREVIEW_URL_STORAGE_KEY)
      return null
    }
    const parsed = new URL(normalized)
    const pathname = decodeURIComponent(parsed.pathname).toLowerCase()
    if (/^\/(?:health|metrics|readyz?|livez?|v\d+)(?:\/|$)/.test(pathname)) return null
    if (/\/(?:health|metrics|readyz?|livez?)(?:\/|$)/.test(pathname)) return null
    return normalized
  } catch {
    return null
  }
}

function persistUrl(url: string): void {
  writeBrowserStorageItem(PREVIEW_URL_STORAGE_KEY, url)
}

function readStoredAutoFollow(): boolean {
  const raw = readBrowserStorageItem(PREVIEW_AUTO_FOLLOW_STORAGE_KEY)
  return raw == null ? true : raw === 'true'
}

function persistAutoFollow(value: boolean): void {
  writeBrowserStorageItem(PREVIEW_AUTO_FOLLOW_STORAGE_KEY, String(value))
}

function formatAddressInput(url: string): string {
  try {
    const parsed = new URL(url)
    const path = parsed.pathname === '/' ? '' : parsed.pathname
    return `${parsed.host}${path}${parsed.search}${parsed.hash}`
  } catch {
    return url
  }
}

type LoadOptions = {
  keepAutoFollow?: boolean
}

export function resolveInitialDevBrowserUrl(input: {
  normalizedPreferredUrl?: string | null
  storedUrl?: string | null
  latestDetectedUrl?: string | null
}): string | null {
  return input.normalizedPreferredUrl ?? input.storedUrl ?? input.latestDetectedUrl ?? null
}

export function canUseElectronWebviewEnvironment(input: {
  openExternalAvailable: boolean
  userAgent: string
}): boolean {
  return input.openExternalAvailable && /\bElectron\//.test(input.userAgent)
}

export function DevBrowserPanel({
  blocks = [],
  detectedUrls: providedDetectedUrls,
  preferredUrl,
  className,
  onCollapse,
  onSubmitPrompt,
  viewerControls,
  visible = true
}: {
  blocks?: ChatBlock[]
  detectedUrls?: string[]
  preferredUrl?: string | null
  className?: string
  onCollapse: () => void
  onSubmitPrompt?: (prompt: string, references: NativeReference[]) => void
  viewerControls?: WorkspaceViewerControls
  visible?: boolean
}): ReactElement {
  const { t } = useTranslation('common')
  const webviewRef = useRef<DevWebviewTag | null>(null)
  const captureEpoch = useRef(0)
  const [capturingSelection, setCapturingSelection] = useState(false)
  useEffect(() => {
    const epoch = captureEpoch
    const unsubscribe = useChatStore.subscribe((state, previous) => {
      if (state.activeThreadId !== previous.activeThreadId || state.workspaceRoot !== previous.workspaceRoot) captureEpoch.current++
    })
    return () => { epoch.current++; unsubscribe() }
  }, [])
  const captureSelection = async (actionPrompt?: string) => {
    const guest = webviewRef.current, threadId = useChatStore.getState().activeThreadId
    if (!guest || !threadId || capturingSelection) return
    const epoch = captureEpoch.current
    setCapturingSelection(true)
    try {
      const response = await window.analytix.browserSelection.request({ action: 'capture', guestId: guest.getWebContentsId(), threadId })
      if (!response.ok && response.error === 'child-frame-unsupported') { setLoadError(t('browserChildSelectionUnsupported')); return }
      const scope = response.ok ? browserScopeSchema.safeParse(response.scope) : null
      if (!scope?.success) throw Error('selection-unavailable')
      if (epoch !== captureEpoch.current || guest !== webviewRef.current || useChatStore.getState().activeThreadId !== threadId) {
        void window.analytix.browserSelection.request({ action: 'revoke', scope: scope.data })
        return
      }
      const reference: NativeReference = { id: crypto.randomUUID(), kind: 'browser-selection', threadId, workspace: scope.data.workspace,
        objectId: scope.data.documentId, revision: scope.data.selectionId, scopeId: scope.data.scopeId,
        browserScope: scope.data, editable: false, path: '', text: '', label: t('browserSelectedText') }
      if (actionPrompt) onSubmitPrompt?.(actionPrompt, [reference])
      else useNativeReferenceStore.getState().add(reference)
    } catch { setLoadError(t('browserSelectionUnavailable')) }
    finally { setCapturingSelection(false) }
  }
  const iframeLoadedUrlRef = useRef<string | null>(null)
  const detectedUrls = useMemo(
    () => providedDetectedUrls ?? extractDetectedDevPreviewUrls(blocks),
    [blocks, providedDetectedUrls]
  )
  const latestDetectedUrl = detectedUrls[0] ?? null
  const useElectronWebview = canUseElectronWebviewEnvironment({
    openExternalAvailable: typeof window.analytix?.app?.openExternal === 'function',
    userAgent: window.navigator.userAgent
  })
  const normalizedPreferredUrl = useMemo(
    () => (preferredUrl ? normalizeDevPreviewUrlInput(preferredUrl) : null),
    [preferredUrl]
  )
  const initialUrl = resolveInitialDevBrowserUrl({
    normalizedPreferredUrl,
    storedUrl: readStoredUrl(),
    latestDetectedUrl
  })
  const preferredUrlRef = useRef<string | null>(normalizedPreferredUrl)
  const [activeUrl, setActiveUrl] = useState<string | null>(initialUrl)
  const [draftUrl, setDraftUrl] = useState(() => (initialUrl ? formatAddressInput(initialUrl) : ''))
  const [autoFollow, setAutoFollow] = useState(readStoredAutoFollow)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [pageTitle, setPageTitle] = useState('')
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [iframeBackStack, setIframeBackStack] = useState<string[]>([])
  const [iframeForwardStack, setIframeForwardStack] = useState<string[]>([])
  const [iframeReloadNonce, setIframeReloadNonce] = useState(0)
  const [previewInstanceNonce, setPreviewInstanceNonce] = useState(0)
  const canNavigateBack = useElectronWebview ? canGoBack : iframeBackStack.length > 0
  const canNavigateForward = useElectronWebview ? canGoForward : iframeForwardStack.length > 0

  useEffect(() => {
    persistAutoFollow(autoFollow)
  }, [autoFollow])

  useEffect(() => {
    if (activeUrl) persistUrl(activeUrl)
  }, [activeUrl])

  useEffect(() => {
    if (!normalizedPreferredUrl || preferredUrlRef.current === normalizedPreferredUrl) return
    preferredUrlRef.current = normalizedPreferredUrl
    setAutoFollow(true)
    setActiveUrl(normalizedPreferredUrl)
    setDraftUrl(formatAddressInput(normalizedPreferredUrl))
    setPageTitle('')
    setLoading(true)
    setLoadError(null)
  }, [normalizedPreferredUrl])

  useEffect(() => {
    if (!autoFollow || !latestDetectedUrl || latestDetectedUrl === activeUrl) return
    setActiveUrl(latestDetectedUrl)
    setDraftUrl(formatAddressInput(latestDetectedUrl))
    setPageTitle('')
    setLoading(true)
    setLoadError(null)
  }, [activeUrl, autoFollow, latestDetectedUrl])

  useEffect(() => {
    const webview = webviewRef.current
    if (!useElectronWebview || !activeUrl || !webview) return

    const syncNavigationState = (): void => {
      try {
        setCanGoBack(webview.canGoBack())
        setCanGoForward(webview.canGoForward())
        const currentUrl = normalizeDevPreviewUrlInput(webview.getURL())
        if (currentUrl) {
          setActiveUrl(currentUrl)
          setDraftUrl(formatAddressInput(currentUrl))
        }
      } catch {
        /* webview may not be attached yet */
      }
    }

    const handleStartLoading = (): void => {
      setLoading(true)
      setLoadError(null)
    }
    const handleStopLoading = (): void => {
      setLoading(false)
      syncNavigationState()
    }
    const handleNavigate: EventListener = (event): void => {
      const currentUrl = normalizeDevPreviewUrlInput((event as WebviewNavigateEvent).url)
      if (!currentUrl) return
      setActiveUrl(currentUrl)
      setDraftUrl(formatAddressInput(currentUrl))
      setLoadError(null)
      syncNavigationState()
    }
    const handleFailLoad: EventListener = (event): void => {
      const failEvent = event as WebviewFailLoadEvent
      if (!failEvent.isMainFrame || failEvent.errorCode === -3) return
      setLoading(false)
      setLoadError(failEvent.errorDescription || t('browserLoadFailed'))
      syncNavigationState()
    }
    const handleTitle: EventListener = (event): void => {
      setPageTitle((event as WebviewTitleEvent).title)
    }

    webview.addEventListener('did-start-loading', handleStartLoading)
    webview.addEventListener('did-stop-loading', handleStopLoading)
    webview.addEventListener('did-navigate', handleNavigate)
    webview.addEventListener('did-navigate-in-page', handleNavigate)
    webview.addEventListener('did-fail-load', handleFailLoad)
    webview.addEventListener('page-title-updated', handleTitle)

    return () => {
      webview.removeEventListener('did-start-loading', handleStartLoading)
      webview.removeEventListener('did-stop-loading', handleStopLoading)
      webview.removeEventListener('did-navigate', handleNavigate)
      webview.removeEventListener('did-navigate-in-page', handleNavigate)
      webview.removeEventListener('did-fail-load', handleFailLoad)
      webview.removeEventListener('page-title-updated', handleTitle)
    }
  }, [activeUrl, previewInstanceNonce, t, useElectronWebview])

  useEffect(() => {
    if (useElectronWebview || !activeUrl) return
    iframeLoadedUrlRef.current = null
    setLoading(true)
    setLoadError(null)

    const timeout = window.setTimeout(() => {
      if (iframeLoadedUrlRef.current === activeUrl) return
      setLoading(false)
      setLoadError(t('browserLoadFailed'))
    }, 10000)

    return () => window.clearTimeout(timeout)
  }, [activeUrl, iframeReloadNonce, t, useElectronWebview])

  const loadUrl = (value: string, options: LoadOptions = {}): void => {
    const normalized = normalizeDevPreviewUrlInput(value)
    if (!normalized) {
      setLoadError(t('browserInvalidUrl'))
      return
    }
    if (!options.keepAutoFollow) setAutoFollow(false)
    setLoadError(null)
    setPageTitle('')
    setDraftUrl(formatAddressInput(normalized))
    if (normalized === activeUrl) {
      reload()
      return
    }
    if (!useElectronWebview && activeUrl && normalized !== activeUrl) {
      setIframeBackStack((stack) => [...stack, activeUrl].slice(-30))
      setIframeForwardStack([])
    }
    setLoading(true)
    setActiveUrl(normalized)
  }

  const submitUrl = (event: FormEvent<HTMLFormElement>): void => {
    event.preventDefault()
    loadUrl(draftUrl)
  }

  const reload = (): void => {
    if (!activeUrl) return
    if (!useElectronWebview) {
      const normalized = normalizeDevPreviewUrlInput(activeUrl)
      if (!normalized) return
      iframeLoadedUrlRef.current = null
      setIframeReloadNonce((nonce) => nonce + 1)
      setLoading(true)
      setLoadError(null)
      return
    }
    setLoading(true)
    setLoadError(null)
    const webview = webviewRef.current
    if (!webview) {
      setPreviewInstanceNonce((nonce) => nonce + 1)
      return
    }
    try {
      webview.reloadIgnoringCache()
    } catch {
      setPreviewInstanceNonce((nonce) => nonce + 1)
    }
  }

  const resetPreview = (): void => {
    setAutoFollow(false)
    setLoadError(null)
    setPageTitle('')
    setCanGoBack(false)
    setCanGoForward(false)
    setIframeBackStack([])
    setIframeForwardStack([])
    iframeLoadedUrlRef.current = null
    setIframeReloadNonce(0)
    setPreviewInstanceNonce((nonce) => nonce + 1)
    setDraftUrl('')
    setLoading(false)
    setActiveUrl(null)
    removeBrowserStorageItem(PREVIEW_URL_STORAGE_KEY)
  }

  const openExternal = (): void => {
    if (!activeUrl) return
    const normalized = normalizeDevPreviewUrlInput(activeUrl)
    if (!normalized) return
    if (typeof window.analytix?.app?.openExternal === 'function') {
      void window.analytix.app.openExternal(normalized).catch(() => undefined)
      return
    }
    window.open(normalized, '_blank', 'noopener,noreferrer')
  }

  const goBack = (): void => {
    if (!activeUrl) return
    if (!useElectronWebview) {
      const previousUrl = iframeBackStack.at(-1)
      if (!previousUrl) return
      setIframeBackStack((stack) => stack.slice(0, -1))
      setIframeForwardStack((stack) => [activeUrl, ...stack].slice(0, 30))
      setLoadError(null)
      setPageTitle('')
      setLoading(true)
      setActiveUrl(previousUrl)
      setDraftUrl(formatAddressInput(previousUrl))
      return
    }
    try {
      if (webviewRef.current?.canGoBack()) webviewRef.current.goBack()
    } catch {
      /* ignore unavailable webview navigation */
    }
  }

  const goForward = (): void => {
    if (!activeUrl) return
    if (!useElectronWebview) {
      const nextUrl = iframeForwardStack[0]
      if (!nextUrl) return
      setIframeForwardStack((stack) => stack.slice(1))
      setIframeBackStack((stack) => [...stack, activeUrl].slice(-30))
      setLoadError(null)
      setPageTitle('')
      setLoading(true)
      setActiveUrl(nextUrl)
      setDraftUrl(formatAddressInput(nextUrl))
      return
    }
    try {
      if (webviewRef.current?.canGoForward()) webviewRef.current.goForward()
    } catch {
      /* ignore unavailable webview navigation */
    }
  }

  const primaryDetectedUrl = latestDetectedUrl ?? detectedUrls[0] ?? null
  const tabLabel = activeUrl
    ? pageTitle || formatDevPreviewUrlLabel(activeUrl)
    : t('browserNewTab')

  return (
    <aside
      style={{ containerType: 'inline-size' }}
      className={`ds-browser-pane ds-no-drag flex min-h-0 flex-col border-l border-ds-border-muted bg-ds-canvas backdrop-blur-xl ${className ?? ''}`}
    >
      <div className="shrink-0 bg-ds-canvas">
        <form onSubmit={submitUrl} className="ds-browser-addressbar">
          <WorkspaceViewerHeader title={tabLabel} viewerControls={visible ? viewerControls : undefined} status={pageTitle || undefined}
            leading={<div className="ds-browser-history flex shrink-0 items-center gap-1">
              <WorkspaceViewerIconButton compact onClick={goBack} disabled={!canNavigateBack} label={t('browserBack')}>
                <ArrowLeft aria-hidden="true" />
              </WorkspaceViewerIconButton>
              <WorkspaceViewerIconButton compact onClick={goForward} disabled={!canNavigateForward} label={t('browserForward')}>
                <ArrowRight aria-hidden="true" />
              </WorkspaceViewerIconButton>
              <WorkspaceViewerIconButton compact onClick={reload} disabled={!activeUrl} label={t('browserReload')}>
                {loading ? <Loader2 className="animate-spin" aria-hidden="true" /> : <RefreshCw aria-hidden="true" />}
              </WorkspaceViewerIconButton>
            </div>}
            titleControl={<div title={tabLabel}
              className="ds-browser-address-field flex h-8 w-full min-w-0 items-center gap-2 rounded-full border border-ds-border-muted px-3 text-ds-muted transition">
              <Globe2 className="ds-browser-address-icon h-3.5 w-3.5 shrink-0" strokeWidth={1.75} aria-hidden="true" />
              <input value={draftUrl} onChange={event => setDraftUrl(event.target.value)}
                className="h-full w-full min-w-0 bg-transparent text-[13px] font-medium text-ds-ink outline-none"
                aria-label={t('browserAddressPlaceholder')} placeholder={t('browserAddressPlaceholder')} spellCheck={false} />
            </div>}
            actions={<>
              <WorkspaceViewerIconButton compact type="submit" className="ds-browser-open-inline" label={t('browserOpen')}>
                <Send aria-hidden="true" />
              </WorkspaceViewerIconButton>
              <WorkspaceViewerMenu label={t('browserMore')} ownerKey={JSON.stringify([activeUrl, useChatStore.getState().activeThreadId, useChatStore.getState().workspaceRoot])} enabled={visible}>
                <div className="ds-browser-compact-actions !flex flex-col gap-1">
                  <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={goBack} disabled={!canNavigateBack} label={t('browserBack')}>
                    <ArrowLeft aria-hidden="true" /><span>{t('browserBack')}</span>
                  </WorkspaceViewerIconButton>
                  <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={goForward} disabled={!canNavigateForward} label={t('browserForward')}>
                    <ArrowRight aria-hidden="true" /><span>{t('browserForward')}</span>
                  </WorkspaceViewerIconButton>
                  <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={reload} disabled={!activeUrl} label={t('browserReload')}>
                    <RefreshCw aria-hidden="true" /><span>{t('browserReload')}</span>
                  </WorkspaceViewerIconButton>
                  <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={() => loadUrl(draftUrl)} label={t('browserOpen')}>
                    <Send aria-hidden="true" /><span>{t('browserOpen')}</span>
                  </WorkspaceViewerIconButton>
                </div>
                {detectedUrls.map(url => <WorkspaceViewerIconButton key={url} role="menuitem" className="!w-full !justify-start"
                  data-browser-detected-url={url} label={url} onClick={() => loadUrl(url, { keepAutoFollow: url === latestDetectedUrl })}>
                  <Globe2 aria-hidden="true" /><span className="min-w-0 truncate">{formatDevPreviewUrlLabel(url)}</span>
                </WorkspaceViewerIconButton>)}
                <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={() => setAutoFollow(value => !value)}
                  label={t('browserAutoFollow')} aria-pressed={autoFollow}>
                  <Sparkles aria-hidden="true" /><span>{t('browserAutoFollow')}</span>
                </WorkspaceViewerIconButton>
                <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={resetPreview} label={t('browserReset')}>
                  <Plus aria-hidden="true" /><span>{t('browserReset')}</span>
                </WorkspaceViewerIconButton>
                <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={openExternal} disabled={!activeUrl} label={t('browserOpenExternal')}>
                  <ExternalLink aria-hidden="true" /><span>{t('browserOpenExternal')}</span>
                </WorkspaceViewerIconButton>
                {useElectronWebview ? <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={() => void captureSelection()}
                  disabled={capturingSelection} label={t('browserQuoteSelection')}>
                  <MessageSquarePlus aria-hidden="true" /><span>{t('browserQuoteSelection')}</span>
                </WorkspaceViewerIconButton> : null}
                {useElectronWebview && onSubmitPrompt ? <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={() => void captureSelection(t('browserExplainPrompt'))}
                  disabled={capturingSelection} label={t('browserExplainSelection')}>
                  <Sparkles aria-hidden="true" /><span>{t('browserExplainSelection')}</span>
                </WorkspaceViewerIconButton> : null}
                {!viewerControls ? <WorkspaceViewerIconButton role="menuitem" className="!w-full !justify-start" onClick={onCollapse} label={t('browserCloseTab')}>
                  <X aria-hidden="true" /><span>{t('browserCloseTab')}</span>
                </WorkspaceViewerIconButton> : null}
              </WorkspaceViewerMenu>
            </>} />
        </form>

      </div>

      {loadError ? (
        <div className="shrink-0 border-b border-red-200/70 bg-red-50/85 px-3 py-2 text-[11px] leading-5 text-red-800 dark:border-red-900/50 dark:bg-red-950/35 dark:text-red-100">
          {loadError}
        </div>
      ) : null}

      <div className="relative min-h-0 flex-1 bg-ds-canvas">
        {!activeUrl ? (
          <div className="ds-browser-empty-state flex h-full flex-col items-center justify-center px-6 text-center">
            <Globe2 className="h-16 w-16 text-zinc-400 dark:text-zinc-500" strokeWidth={1.45} />
            <div className="mt-7 text-[14px] font-semibold text-ds-ink">
              {t('browserEmptyTitle')}
            </div>
            <div className="mt-2 text-[12px] font-medium text-ds-faint">
              {t('browserEmptySubtitle')}
            </div>
            <button
              type="button"
              onClick={() => {
                if (primaryDetectedUrl) {
                  loadUrl(primaryDetectedUrl, { keepAutoFollow: primaryDetectedUrl === latestDetectedUrl })
                  return
                }
                setAutoFollow(true)
              }}
              className="mt-6 inline-flex h-8 items-center justify-center rounded-full bg-ds-surface-subtle px-3 text-[12px] font-semibold text-ds-ink transition hover:bg-ds-hover dark:bg-white/[0.08] dark:hover:bg-white/[0.12]"
            >
              {t('browserShowAll')}
            </button>
          </div>
        ) : useElectronWebview ? (
          <webview
            key={`webview:${previewInstanceNonce}`}
            ref={webviewRef}
            src={activeUrl}
            partition="persist:analytix-dev-browser-v2"
            webpreferences="contextIsolation=yes,nodeIntegration=no,sandbox=yes"
            className="flex h-full w-full bg-white"
          />
        ) : (
          <iframe
            key={`iframe:${previewInstanceNonce}:${activeUrl}:${iframeReloadNonce}`}
            src={activeUrl}
            title={pageTitle || t('browserTitle')}
            sandbox="allow-downloads allow-forms allow-modals allow-popups allow-same-origin allow-scripts"
            referrerPolicy="no-referrer"
            onLoad={() => {
              iframeLoadedUrlRef.current = activeUrl
              setLoading(false)
              setLoadError(null)
            }}
            className="block h-full w-full border-0 bg-white"
          />
        )}
      </div>
    </aside>
  )
}
