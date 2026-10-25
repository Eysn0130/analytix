import type { WorkspaceFileTarget } from '@shared/workspace-file'
import { writePrototypeAuthorizeResultSchema } from '@shared/write-prototype'
import i18n from '../i18n'
import { resolveWriteMarkdownResourcePath } from '@shared/write-markdown-resource'

export type HtmlEmbedContext = {
  rawSrc: string
  alt: string
  filePath: string | null
  workspaceRoot: string | null
  mode?: 'write-prototype' | 'workspace-html'
  protectedPresent?: WorkspaceFileTarget['protectedPresent']
}
export type HtmlEmbedHandle = { element: HTMLElement; run: () => void; dispose: () => void }

/** Shared explicit-run surface for Markdown embeds and workspace HTML. A
 * detached widget retires its exact grant, including late authorizations. */
export function createHtmlEmbedHandle(context: HtmlEmbedContext): HtmlEmbedHandle {
  const root = document.createElement('span')
  root.className = 'write-html-embed'
  root.contentEditable = 'false'
  root.dataset.rawSrc = context.rawSrc
  const header = document.createElement('span')
  header.className = 'write-html-embed-header'
  const title = document.createElement('span')
  title.className = 'write-html-embed-title'
  title.textContent = context.alt.trim() || i18n.t('common:writePrototypeAlt')
  const file = document.createElement('span')
  file.className = 'write-html-embed-file'
  file.textContent = context.rawSrc.replaceAll('\\', '/').split('/').filter(Boolean).pop() ?? context.rawSrc
  const actions = document.createElement('span')
  actions.className = 'write-html-embed-actions'
  const reload = document.createElement('button')
  reload.type = 'button'; reload.className = 'write-html-embed-action'
  reload.textContent = i18n.t('common:writeHtmlEmbedReload'); reload.hidden = true
  const external = document.createElement('button')
  external.type = 'button'; external.className = 'write-html-embed-action'
  external.textContent = i18n.t('common:writeHtmlEmbedOpenExternal')
  actions.append(reload)
  if (context.mode !== 'workspace-html') actions.append(external)
  header.append(title, file, actions)
  const body = document.createElement('span'); body.className = 'write-html-embed-body'
  const status = document.createElement('span'); status.className = 'write-html-embed-status'
  status.setAttribute('role', 'status')
  root.append(header, body, status)
  const absolutePath = resolveWriteMarkdownResourcePath(context.rawSrc, context.filePath)
  const workspaceRoot = context.workspaceRoot?.trim()
  let disposed = false, epoch = 0
  let release: (() => void) | undefined
  let ancestors = new Set<Node>()
  const setStatus = (message: string): void => { status.textContent = message }
  const retire = (): void => {
    observer.disconnect()
    epoch++; const previous = release; release = undefined
    body.replaceChildren(); reload.hidden = true
    previous?.()
  }
  const observer = new MutationObserver(records => {
    if (records.some(record => [...record.removedNodes].some(node => ancestors.has(node)))) {
      retire(); cover()
    }
  })
  const cover = (): void => {
    if (disposed) return
    const button = document.createElement('button')
    button.type = 'button'; button.className = 'write-html-embed-cover'
    button.textContent = i18n.t('common:writeHtmlEmbedRun')
    button.onclick = event => { event.preventDefault(); event.stopPropagation(); run() }
    body.replaceChildren(button)
  }
  const run = (): void => {
    if (disposed || !root.isConnected) return
    observer.takeRecords()
    retire(); setStatus('')
    const api = window.analytix?.write
    const revoke = api?.revokeWritePrototype
    if ((!context.protectedPresent && (!absolutePath || !workspaceRoot)) || !api?.authorizeWritePrototype || !revoke) {
      setStatus(i18n.t('common:writeHtmlEmbedLoadFailed')); cover(); return
    }
    const attempt = epoch
    ancestors = new Set<Node>()
    for (let node: Node | null = root; node; node = node.parentNode) ancestors.add(node)
    observer.observe(document, { childList: true, subtree: true })
    const revokeGrant = (grantId: string): void => { void revoke({ grantId }).catch(() => undefined) }
    const request = context.protectedPresent
      ? { protectedPresent: { ...context.protectedPresent }, mode: 'workspace-html' as const }
      : { path: absolutePath!, workspaceRoot: workspaceRoot!, mode: context.mode ?? 'write-prototype' }
    void api.authorizeWritePrototype(request)
      .then(raw => {
        const parsed = writePrototypeAuthorizeResultSchema.safeParse(raw)
        const result = parsed.success ? parsed.data : { ok: false as const, message: i18n.t('common:writeHtmlEmbedLoadFailed') }
        if (observer.takeRecords().some(record => [...record.removedNodes].some(node => ancestors.has(node)))) { retire(); cover() }
        if (!result.ok) {
          if (!disposed && attempt === epoch && root.isConnected) { setStatus(result.message); cover() }
          return
        }
        if (disposed || attempt !== epoch || !root.isConnected) { revokeGrant(result.grantId); return }
        release = () => revokeGrant(result.grantId)
        const webview = document.createElement('webview')
        // Partition precedes src and attachment, so the guest never uses the
        // renderer/default session. Main verifies both against this grant.
        webview.setAttribute('partition', result.partition)
        webview.setAttribute('webpreferences', 'contextIsolation=yes,nodeIntegration=no,sandbox=yes')
        webview.setAttribute('src', result.fileUrl)
        webview.className = 'write-html-embed-webview'
        webview.addEventListener('did-fail-load', event => {
          if ((event as unknown as { errorCode?: number }).errorCode === -3 || attempt !== epoch) return
          retire(); setStatus(i18n.t('common:writeHtmlEmbedLoadFailed')); cover()
        })
        body.replaceChildren(webview); reload.hidden = false
      }).catch(() => {
        if (!disposed && attempt === epoch && root.isConnected) { setStatus(i18n.t('common:writeHtmlEmbedLoadFailed')); cover() }
      })
  }
  reload.onclick = event => { event.preventDefault(); event.stopPropagation(); run() }
  external.onclick = event => {
    event.preventDefault(); event.stopPropagation()
    if (!absolutePath || !workspaceRoot) return
    void window.analytix?.write?.openWritePrototype?.({ path: absolutePath, workspaceRoot }).then(result => {
      if (!disposed && root.isConnected && !result.ok) setStatus(result.message ?? '')
    }).catch(() => undefined)
  }
  cover()
  return { element: root, run, dispose: () => { disposed = true; observer.disconnect(); retire() } }
}

export function createHtmlEmbedElement(context: HtmlEmbedContext): HTMLElement {
  return createHtmlEmbedHandle(context).element
}
