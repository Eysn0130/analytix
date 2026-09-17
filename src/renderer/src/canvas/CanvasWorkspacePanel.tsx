import { useEffect, useRef, useState, type ReactElement, type KeyboardEvent } from 'react'
import { Copy, FolderOpen, RotateCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { WorkspaceTab } from '../store/workspace-tabs-store'
import { createCanvasPreviewSession } from './canvas-preview-session'
import { decodeCanvasPreview, type CanvasPreview } from './canvas-preview'
import './canvas-workspace.css'

// One active preview lease in this renderer, including across route remounts.
// The Go Host still authorizes every open; this queue only orders UI cleanup.
const session = createCanvasPreviewSession(request => window.analytix.canvas.request(request))
type Loaded = CanvasPreview & { key: string; url: string }

export function CanvasWorkspacePanel({ threadId, workspaceRoot, activeTab, visible, onOpenObject }: {
  threadId: string | null
  workspaceRoot: string
  activeTab: WorkspaceTab | null
  visible: boolean
  onOpenObject: (workspace: string, path: string) => void
}): ReactElement {
  const { t } = useTranslation('canvas')
  const path = activeTab?.path
  const inWorkspace = !path || activeTab?.workspaceRoot === workspaceRoot
  const key = JSON.stringify([threadId, workspaceRoot, path])
  const current = useRef({ key, visible, epoch: 0 })
  if (current.current.key !== key || current.current.visible !== visible) current.current = { key, visible, epoch: current.current.epoch + 1 }
  const [loaded, setLoaded] = useState<Loaded | null>(null)
  const [errorKey, setErrorKey] = useState<string | null>(null)
  const [reload, setReload] = useState(0)
  const [selection, setSelection] = useState<{ key: string; id: string } | null>(null)
  const [copyState, setCopyState] = useState<{ key: string; id: string; ok: boolean } | null>(null)
  const [picking, setPicking] = useState(false)
  const pickingRef = useRef(false)
  const mounted = useRef(true)
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; current.current.epoch++ } }, [])
  const opening = useRef(0)
  useEffect(() => {
    const ticket = ++opening.current
    let cancelled = false
    let url: string | null = null
    setLoaded(null); setErrorKey(null); setSelection(null); setCopyState(null)
    const kind = path && /\.canvas$/i.test(path) ? 'canvas' : path && /\.png$/i.test(path) ? 'png' : null
    if (visible && threadId && workspaceRoot && inWorkspace && path && kind) {
      void (async () => {
        const document = await session.open({ threadId, workspace: workspaceRoot, path, kind })
        if (cancelled || opening.current !== ticket) return
        if (!document) { setErrorKey(key); return }
        try {
          const decoded = await decodeCanvasPreview(document)
          if (cancelled || opening.current !== ticket || !current.current.visible || current.current.key !== key) return
          url = URL.createObjectURL(decoded.blob)
          setLoaded({ ...decoded, key, url })
        } catch {
          if (!cancelled && opening.current === ticket) { setErrorKey(key); void session.close() }
        }
      })()
    } else if (visible && threadId && workspaceRoot && inWorkspace && path && !kind) setErrorKey(key)
    return () => { cancelled = true; if (url) URL.revokeObjectURL(url); void session.close() }
  }, [key, path, threadId, workspaceRoot, visible, inWorkspace, reload])
  // Gate the render itself, not just an effect after paint, when scope changes.
  const preview = visible && inWorkspace && loaded?.key === key ? loaded : null
  const selected = selection?.key === key ? [...(preview?.scene?.facts.nodes ?? []), ...(preview?.scene?.facts.edges ?? [])].find(item => item.id === selection.id) : null
  const title = path?.replaceAll('\\', '/').split('/').at(-1) || t('canvasWorkspace')
  const pick = async (): Promise<void> => {
    if (pickingRef.current || !threadId || !workspaceRoot || !visible) return
    const origin = key, epoch = current.current.epoch
    const stillCurrent = () => mounted.current && current.current.visible && current.current.key === origin && current.current.epoch === epoch
    pickingRef.current = true; setPicking(true); setErrorKey(null)
    try {
      const result = await window.analytix.canvas.pickFile({ workspace: workspaceRoot })
      if (!stillCurrent()) return
      if (!result.ok) { setErrorKey(origin); return }
      if (result.path && /\.(canvas|png)$/i.test(result.path)) onOpenObject(workspaceRoot, result.path)
      else if (result.path) setErrorKey(origin)
    } catch { if (stillCurrent()) setErrorKey(origin) }
    finally { pickingRef.current = false; if (mounted.current) setPicking(false) }
  }
  const select = (id: string): void => { setSelection({ key, id }); setCopyState(null) }
  const copy = async (): Promise<void> => {
    if (!selected || !preview) return
    const origin = key
    try {
      await navigator.clipboard.writeText(JSON.stringify(selected, null, 2))
      if (mounted.current && current.current.visible && current.current.key === origin) setCopyState({ key: origin, id: selected.id, ok: true })
    } catch { if (mounted.current && current.current.visible && current.current.key === origin) setCopyState({ key: origin, id: selected.id, ok: false }) }
  }
  const activate = (event: KeyboardEvent<SVGElement>, id: string): void => {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return
    if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(id) }
  }
  const viewBox = preview?.layout?.viewBox
  return <section className="canvas-workspace" inert={!visible} aria-label={t('canvasWorkspace')}
    onKeyDown={event => { if (!event.nativeEvent.isComposing && event.key === 'Escape' && selected) { event.preventDefault(); setSelection(null) } }}>
    <header className="canvas-workspace-header">
      <span className="canvas-workspace-title" title={title}>{title}</span>
      <span className="text-xs text-ds-muted">{t('canvasLocalPreview')}</span>
      <button type="button" className="ds-toolbar-icon-button" disabled={!path || !threadId || !inWorkspace} onClick={() => setReload(value => value + 1)} title={t('canvasReload')} aria-label={t('canvasReload')}><RotateCw size={16} /></button>
      <button type="button" className="ds-toolbar-icon-button" disabled={picking || !threadId || !workspaceRoot} onClick={() => void pick()} title={t('canvasOpen')} aria-label={t('canvasOpen')}><FolderOpen size={16} /></button>
    </header>
    {!threadId || !workspaceRoot ? <p className="canvas-workspace-message">{t('canvasThreadRequired')}</p>
      : !inWorkspace ? <p className="canvas-workspace-message">{t('canvasWorkspaceMismatch')}</p>
      : errorKey === key ? <p role="alert" className="canvas-workspace-message">{t('canvasPreviewFailed')}</p>
      : !path ? <div className="canvas-workspace-message"><p>{t('canvasPickHint')}</p><button type="button" className="mt-3 rounded border border-ds-border px-3 py-2" disabled={picking} onClick={() => void pick()}>{t('canvasOpen')}</button></div>
      : !preview ? <p role="status" className="canvas-workspace-message">{t('common:loading')}</p>
      : <div className="canvas-workspace-content">
        <div className="canvas-workspace-visual">
          {viewBox && preview.scene && preview.layout ? <svg viewBox={viewBox.join(' ')} aria-label={title}>
            <image href={preview.url} x={viewBox[0]} y={viewBox[1]} width={viewBox[2]} height={viewBox[3]} onError={() => { if (current.current.key === key && current.current.visible) setErrorKey(key) }} />
            {preview.layout.connections.map(edge => <polyline key={edge.id} points={edge.points.map(point => point.join(',')).join(' ')}
              className="canvas-hit canvas-hit-edge" data-selected={selected?.id === edge.id} fill="none" stroke="transparent" strokeWidth={14} vectorEffect="non-scaling-stroke"
              role="button" tabIndex={0} aria-pressed={selected?.id === edge.id} aria-label={preview.scene!.facts.edges.find(fact => fact.id === edge.id)?.label || edge.id}
              onClick={() => select(edge.id)} onKeyDown={event => activate(event, edge.id)} />)}
            {preview.layout.components.map(node => <rect key={node.id} x={node.x} y={node.y} width={node.width} height={node.height}
              className="canvas-hit canvas-hit-node" data-selected={selected?.id === node.id} fill="transparent" vectorEffect="non-scaling-stroke"
              role="button" tabIndex={0} aria-pressed={selected?.id === node.id} aria-label={preview.scene!.facts.nodes.find(fact => fact.id === node.id)?.label || node.id}
              onClick={() => select(node.id)} onKeyDown={event => activate(event, node.id)} />)}
          </svg> : <img src={preview.url} alt={title} onError={() => { if (current.current.key === key && current.current.visible) setErrorKey(key) }} />}
        </div>
        {preview.scene ? <details className="canvas-workspace-inspector" open={Boolean(selected)}>
          <summary>{t('canvasObjects', { count: preview.scene.facts.nodes.length + preview.scene.facts.edges.length })}</summary>
          <select aria-label={t('canvasSelectObject')} value={selected?.id ?? ''} onChange={event => select(event.target.value)}>
            <option value="">{t('canvasSelectObject')}</option>
            {[...preview.scene.facts.nodes, ...preview.scene.facts.edges].map(fact => <option key={fact.id} value={fact.id}>{fact.label || fact.id} · {fact.id}</option>)}
          </select>
          {selected ? <div>
            <div className="flex items-center justify-between gap-2"><strong>{selected.label || selected.id}</strong><button type="button" className="ds-toolbar-icon-button" onClick={() => void copy()} title={t('canvasCopySelection')} aria-label={t('canvasCopySelection')}><Copy size={15} /></button></div>
            {selected.assumption ? <p>{t('canvasAssumption')}</p> : null}
            <pre>{JSON.stringify(selected, null, 2)}</pre>
            {copyState?.key === key && copyState.id === selected.id ? <p role="status">{t(copyState.ok ? 'canvasCopied' : 'canvasCopyFailed')}</p> : null}
          </div> : <p className="text-xs text-ds-muted">{t('canvasSelectHint')}</p>}
        </details> : null}
      </div>}
  </section>
}
