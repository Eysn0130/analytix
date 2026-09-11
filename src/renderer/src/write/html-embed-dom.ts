import i18n from '../i18n'
import { resolveWriteMarkdownResourcePath } from '@shared/write-markdown-resource'

/**
 * Inline embed for generated HTML prototypes (`![alt](../../proto/x.html)`).
 *
 * Renders a cover card and only mounts a `<webview>` after the user clicks
 * 运行原型: every DOM reattach reloads the guest page and each guest is its
 * own process, so editors that rebuild widgets (CodeMirror viewport,
 * TipTap undo) must not pay that cost passively. The webview src is set
 * only after the main process authorizes the path (write:authorize-prototype),
 * which also allow-lists it for the `will-attach-webview` guard. Plain DOM so
 * the same markup serves the TipTap node view, the CodeMirror live-preview
 * widget and the split markdown preview. Styles live in
 * styles/write-editor.css under `.write-html-embed`.
 */

export type HtmlEmbedContext = {
  rawSrc: string
  alt: string
  /** Absolute path of the markdown document embedding the prototype. */
  filePath: string | null
  workspaceRoot: string | null
}

type EmbedElements = {
  body: HTMLElement
  status: HTMLElement
  reloadButton: HTMLButtonElement
}

function fileNameFromSrc(rawSrc: string): string {
  const normalized = rawSrc.replaceAll('\\', '/').trim()
  const parts = normalized.split('/').filter(Boolean)
  return parts[parts.length - 1] || normalized
}

function setStatus(elements: EmbedElements, message: string, tone: 'idle' | 'error'): void {
  elements.status.textContent = message
  elements.status.dataset.tone = tone
}

function showCover(elements: EmbedElements, onRun: () => void): void {
  elements.body.replaceChildren()
  elements.reloadButton.hidden = true
  const cover = document.createElement('button')
  cover.type = 'button'
  cover.className = 'write-html-embed-cover'
  const icon = document.createElement('span')
  icon.className = 'write-html-embed-cover-icon'
  icon.textContent = '▶'
  const label = document.createElement('span')
  label.className = 'write-html-embed-cover-label'
  label.textContent = i18n.t('common:writeHtmlEmbedRun')
  cover.append(icon, label)
  cover.addEventListener('click', (event) => {
    event.preventDefault()
    event.stopPropagation()
    onRun()
  })
  elements.body.replaceChildren(cover)
}

function mountWebview(elements: EmbedElements, fileUrl: string): void {
  elements.body.replaceChildren()
  const webview = document.createElement('webview')
  webview.setAttribute('src', fileUrl)
  // Non-persistent partition keeps prototype guests away from the dev
  // browser and the default session.
  webview.setAttribute('partition', 'analytix-proto')
  webview.setAttribute('webpreferences', 'contextIsolation=yes,nodeIntegration=no,sandbox=yes')
  webview.className = 'write-html-embed-webview'
  webview.addEventListener('did-fail-load', (event) => {
    const failure = event as unknown as { errorCode?: number }
    // -3 (aborted) fires on reload races; everything else is a real failure.
    if (failure.errorCode === -3) return
    setStatus(elements, i18n.t('common:writeHtmlEmbedLoadFailed'), 'error')
  })
  elements.body.replaceChildren(webview)
  elements.reloadButton.hidden = false
  elements.reloadButton.onclick = (event) => {
    event.preventDefault()
    event.stopPropagation()
    setStatus(elements, '', 'idle')
    ;(webview as unknown as { reload?: () => void }).reload?.()
  }
}

export function createHtmlEmbedElement(context: HtmlEmbedContext): HTMLElement {
  const root = document.createElement('span')
  root.className = 'write-html-embed'
  root.contentEditable = 'false'
  root.dataset.rawSrc = context.rawSrc

  const header = document.createElement('span')
  header.className = 'write-html-embed-header'

  const title = document.createElement('span')
  title.className = 'write-html-embed-title'
  title.textContent = context.alt.trim() || i18n.t('common:writePrototypeAlt')
  const fileName = document.createElement('span')
  fileName.className = 'write-html-embed-file'
  fileName.textContent = fileNameFromSrc(context.rawSrc)

  const actions = document.createElement('span')
  actions.className = 'write-html-embed-actions'
  const reloadButton = document.createElement('button')
  reloadButton.type = 'button'
  reloadButton.className = 'write-html-embed-action'
  reloadButton.textContent = i18n.t('common:writeHtmlEmbedReload')
  reloadButton.hidden = true
  const openButton = document.createElement('button')
  openButton.type = 'button'
  openButton.className = 'write-html-embed-action'
  openButton.textContent = i18n.t('common:writeHtmlEmbedOpenExternal')
  actions.append(reloadButton, openButton)

  header.append(title, fileName, actions)

  const body = document.createElement('span')
  body.className = 'write-html-embed-body'

  const status = document.createElement('span')
  status.className = 'write-html-embed-status'

  root.append(header, body, status)

  const elements: EmbedElements = { body, status, reloadButton }

  const absolutePath = resolveWriteMarkdownResourcePath(context.rawSrc, context.filePath) ?? null
  const workspaceRoot = context.workspaceRoot?.trim() || null

  if (!absolutePath || !workspaceRoot) {
    setStatus(elements, i18n.t('common:writeHtmlEmbedMissing'), 'error')
    return root
  }

  const activate = (): void => {
    if (typeof window.analytix?.write?.authorizeWritePrototype !== 'function') {
      setStatus(elements, i18n.t('common:writeHtmlEmbedLoadFailed'), 'error')
      return
    }
    setStatus(elements, '', 'idle')
    void window.analytix
      .write.authorizeWritePrototype({ path: absolutePath, workspaceRoot })
      .then((result) => {
        if (!root.isConnected) return
        if (!result.ok) {
          setStatus(elements, result.message, 'error')
          return
        }
        mountWebview(elements, result.fileUrl)
      })
      .catch((error: unknown) => {
        if (!root.isConnected) return
        setStatus(elements, error instanceof Error ? error.message : String(error), 'error')
      })
  }

  openButton.addEventListener('click', (event) => {
    event.preventDefault()
    event.stopPropagation()
    if (typeof window.analytix?.write?.openWritePrototype !== 'function') return
    void window.analytix
      .write.openWritePrototype({ path: absolutePath, workspaceRoot })
      .then((result) => {
        if (!root.isConnected) return
        if (!result.ok) setStatus(elements, result.message ?? '', 'error')
      })
      .catch(() => undefined)
  })

  showCover(elements, activate)
  return root
}
