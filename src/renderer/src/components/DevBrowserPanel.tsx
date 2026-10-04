import type { FormEvent, ReactElement } from 'react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Globe2,
  Loader2,
  MoreHorizontal,
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
  onSubmitPrompt
}: {
  blocks?: ChatBlock[]
  detectedUrls?: string[]
  preferredUrl?: string | null
  className?: string
  onCollapse: () => void
  onSubmitPrompt?: (prompt: string, references: NativeReference[]) => void
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
      <div className="shrink-0 border-b border-ds-border-muted bg-ds-canvas">
        <form onSubmit={submitUrl} className="ds-browser-addressbar flex h-12 min-w-0 items-center gap-1.5 px-2">
          <div className="ds-browser-history flex shrink-0 items-center gap-1 rounded-full p-0.5">
            <button
              type="button"
              onClick={goBack}
              disabled={!canNavigateBack}
              className="inline-flex h-7 w-7 items-center justify-center rounded-full text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-default disabled:opacity-35"
              aria-label={t('browserBack')}
              title={t('browserBack')}
            >
              <ArrowLeft className="h-3.5 w-3.5" strokeWidth={1.8} />
            </button>
            <button
              type="button"
              onClick={goForward}
              disabled={!canNavigateForward}
              className="inline-flex h-7 w-7 items-center justify-center rounded-full text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-default disabled:opacity-35"
              aria-label={t('browserForward')}
              title={t('browserForward')}
            >
              <ArrowRight className="h-3.5 w-3.5" strokeWidth={1.8} />
            </button>
            <button
              type="button"
              onClick={reload}
              disabled={!activeUrl}
              className="inline-flex h-7 w-7 items-center justify-center rounded-full text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-default disabled:opacity-35"
              aria-label={t('browserReload')}
              title={t('browserReload')}
            >
              {loading ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" strokeWidth={1.8} />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" strokeWidth={1.8} />
              )}
            </button>
          </div>

          <div title={tabLabel}
            className="ds-browser-address-field flex h-8 min-w-0 flex-1 items-center gap-2 rounded-full border border-ds-border-muted px-3 text-ds-muted transition">
            <Globe2 className="ds-browser-address-icon h-3.5 w-3.5 shrink-0" strokeWidth={1.75} />
            <input
              value={draftUrl}
              onChange={(event) => setDraftUrl(event.target.value)}
              className="h-full w-full min-w-0 bg-transparent text-[13px] font-medium text-ds-ink outline-none"
              aria-label={t('browserAddressPlaceholder')}
              placeholder={t('browserAddressPlaceholder')}
              spellCheck={false}
            />
          </div>

          <div className="flex shrink-0 items-center gap-1">
            <button
              type="submit"
              className="ds-browser-open-inline inline-flex h-8 w-8 items-center justify-center rounded-full text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
              aria-label={t('browserOpen')}
              title={t('browserOpen')}
            >
              <Send className="h-3.5 w-3.5" strokeWidth={1.8} />
            </button>
            <details className="ds-browser-actions relative" onKeyDown={(event) => {
              if (event.key !== 'Escape') return
              event.preventDefault(); event.stopPropagation()
              event.currentTarget.open = false
              event.currentTarget.querySelector('summary')?.focus()
            }} onBlur={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget as Node | null)) event.currentTarget.open = false
            }} onClick={(event) => {
              if (event.target instanceof Element && event.target.closest('button')) event.currentTarget.open = false
            }}>
              <summary className="flex h-8 w-8 cursor-pointer list-none items-center justify-center rounded-full text-ds-faint hover:bg-ds-hover hover:text-ds-ink [&::-webkit-details-marker]:hidden"
                aria-label={t('browserMore')} title={t('browserMore')}>
                <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
              </summary>
              <div style={{ width: 'min(14rem, calc(100cqw - 16px))' }} className="absolute right-0 top-full z-50 mt-1 flex flex-col gap-1 rounded-xl border border-ds-border bg-ds-elevated p-1 text-[12px] [box-shadow:var(--ax-shadow-popover)]">
                <div className="ds-browser-compact-actions flex-col gap-1">
                  <button type="button" onClick={goBack} disabled={!canNavigateBack}
                    className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-35"
                    aria-label={t('browserBack')}>
                    <ArrowLeft className="h-3.5 w-3.5" aria-hidden="true" />{t('browserBack')}
                  </button>
                  <button type="button" onClick={goForward} disabled={!canNavigateForward}
                    className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-35"
                    aria-label={t('browserForward')}>
                    <ArrowRight className="h-3.5 w-3.5" aria-hidden="true" />{t('browserForward')}
                  </button>
                  <button type="button" onClick={reload} disabled={!activeUrl}
                    className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-35"
                    aria-label={t('browserReload')}>
                    <RefreshCw className="h-3.5 w-3.5" aria-hidden="true" />{t('browserReload')}
                  </button>
                  <button type="submit"
                    className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover"
                    aria-label={t('browserOpen')}>
                    <Send className="h-3.5 w-3.5" aria-hidden="true" />{t('browserOpen')}
                  </button>
                </div>
                <button type="button" onClick={() => setAutoFollow((value) => !value)}
                  className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover"
                  aria-label={t('browserAutoFollow')} aria-pressed={autoFollow}>
                  <Sparkles className="h-3.5 w-3.5" aria-hidden="true" />{t('browserAutoFollow')}
                </button>
                <button type="button" onClick={resetPreview}
                  className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover"
                  aria-label={t('browserReset')}>
                  <Plus className="h-3.5 w-3.5" aria-hidden="true" />{t('browserReset')}
                </button>
                <button type="button" onClick={openExternal} disabled={!activeUrl}
                  className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-35"
                  aria-label={t('browserOpenExternal')}>
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />{t('browserOpenExternal')}
                </button>
                {useElectronWebview ? <button type="button" onClick={() => void captureSelection()} disabled={capturingSelection}
                  className="min-h-8 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-50">{t('browserQuoteSelection')}</button> : null}
                {useElectronWebview && onSubmitPrompt ? <button type="button" onClick={() => void captureSelection(t('browserExplainPrompt'))} disabled={capturingSelection}
                  className="min-h-8 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover disabled:opacity-50">{t('browserExplainSelection')}</button> : null}
                <button type="button" onClick={onCollapse}
                  className="flex min-h-8 items-center gap-2 rounded-lg px-2 text-left text-ds-muted hover:bg-ds-hover"
                  aria-label={t('browserCloseTab')}>
                  <X className="h-3.5 w-3.5" aria-hidden="true" />{t('browserCloseTab')}
                </button>
              </div>
            </details>
          </div>
        </form>

        {pageTitle ? (
          <div className="min-w-0 truncate px-3 pb-2 text-[11px] font-medium leading-4 text-ds-muted">
            {pageTitle}
          </div>
        ) : null}

        {detectedUrls.length > 0 ? (
          <div className="flex min-w-0 gap-1.5 overflow-x-auto px-3 pb-2">
            {detectedUrls.map((url) => (
              <button
                key={url}
                type="button"
                onClick={() => loadUrl(url, { keepAutoFollow: url === latestDetectedUrl })}
                className="shrink-0 rounded-full border border-ds-border-muted bg-ds-surface-subtle px-2.5 py-1 text-[10.5px] font-medium text-ds-muted transition hover:border-ds-border-strong hover:text-ds-ink dark:bg-white/[0.06]"
                title={url}
              >
                {formatDevPreviewUrlLabel(url)}
              </button>
            ))}
          </div>
        ) : null}
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
