import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText, RotateCcw } from 'lucide-react'
import type { NativeOfficeRequest } from '@shared/native-office'
import { useNativeOfficeStore } from './native-office-store'

export function NativeOfficePanel({ visible }: { visible: boolean }) {
  const { t } = useTranslation('common')
  const target = useNativeOfficeStore((s) => s.target)
  const view = useNativeOfficeStore((s) => s.view)
  const error = useNativeOfficeStore((s) => s.error)
  const loading = !error && (!view || view.status === 'loading' || (target && target.path !== view.path))
  const surface = useRef<HTMLDivElement>(null)
  const appearance = () => ({ theme: document.documentElement.dataset.theme === 'dark' ? 'dark' as const : 'light' as const, reducedMotion: document.documentElement.dataset.motionReduced === 'true' })
  const bounds = () => {
    const rect = surface.current?.getBoundingClientRect()
    return { x: Math.max(0, Math.round(rect?.x ?? 0)), y: Math.max(0, Math.round(rect?.y ?? 0)),
      width: Math.max(1, Math.round(rect?.width ?? 1)), height: Math.max(1, Math.round(rect?.height ?? 1)) }
  }
  const invoke = async (request: NativeOfficeRequest) => {
    if (request.action === 'open') {
      request = { ...request, appearance: appearance() }
      useNativeOfficeStore.setState({ view: null, error: null })
    }
    try {
      const result = await window.analytix.office.request(request)
      useNativeOfficeStore.setState({ view: result.view, error: result.ok ? null : result.error ?? 'unavailable' })
    } catch { useNativeOfficeStore.setState({ error: 'unavailable' }) }
  }
  useEffect(() => window.analytix.office.onChange((next) => useNativeOfficeStore.setState({ view: next, error: next?.error ?? null })), [])
  useEffect(() => {
    if (!target) return
    void invoke({ action: 'open', ...target, bounds: bounds() })
  }, [target]) // The Main controller serializes the single read-only preview.
  useEffect(() => {
    const element = surface.current
    if (!element) return
    const hide = () => { void window.analytix.office.request({ action: 'hide' }).catch(() => undefined) }
    if (!visible) { hide(); return }
    let stopped = false
    let lastBounds: string | undefined
    const update = () => {
      if (stopped) return
      const next = bounds()
      const style = appearance()
      const key = `${next.x},${next.y},${next.width},${next.height},${style.theme},${style.reducedMotion}`
      if (key === lastBounds) return
      lastBounds = key
      void window.analytix.office.request({ action: 'bounds', bounds: next, appearance: style }).then((result) => {
        if (!result.ok && lastBounds === key) lastBounds = undefined
      }).catch(() => { if (lastBounds === key) lastBounds = undefined })
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    // Dock animation changes the ancestor's width while this fixed-width
    // surface only moves. Observe that boundary to refresh viewport coordinates.
    const dock = element.closest('.ds-right-sidebar-pane')
    if (dock && dock !== element) observer.observe(dock)
    const appearanceObserver = new MutationObserver(update)
    appearanceObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-motion-reduced'] })
    window.addEventListener('resize', update)
    return () => {
      stopped = true
      observer.disconnect()
      appearanceObserver.disconnect()
      window.removeEventListener('resize', update)
      hide()
    }
  }, [visible, view?.objectId])
  return <div className="office-preview-host relative flex min-h-0 flex-1 flex-col" aria-busy={!!loading}>
    <div ref={surface} className="min-h-0 flex-1" aria-label={t('nativeOfficeEditor')} />
    {loading || error ? <div className="office-preview-state" role={error ? 'alert' : 'status'}>
      <FileText className="office-preview-state-icon" aria-hidden="true" />
      <p>{t(error ? 'nativeOfficeOperationFailed' : 'nativeOfficeStatus_loading')}</p>
      {error && target ? <button type="button" className="ds-toolbar-icon-button" aria-label={t('nativeOfficeRetry')} title={t('nativeOfficeRetry')} onClick={() => void invoke({ action: 'open', ...target, bounds: bounds() })}><RotateCcw className="h-4 w-4" /></button> : null}
    </div> : null}
  </div>
}
