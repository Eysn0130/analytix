import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { NativeOfficeRequest } from '@shared/native-office'
import { useNativeOfficeStore } from './native-office-store'

export function NativeOfficePanel({ visible }: { visible: boolean }) {
  const { t } = useTranslation('common')
  const target = useNativeOfficeStore((s) => s.target)
  const view = useNativeOfficeStore((s) => s.view)
  const error = useNativeOfficeStore((s) => s.error)
  const surface = useRef<HTMLDivElement>(null)
  const bounds = () => {
    const rect = surface.current?.getBoundingClientRect()
    return { x: Math.max(0, Math.round(rect?.x ?? 0)), y: Math.max(0, Math.round(rect?.y ?? 0)),
      width: Math.max(1, Math.round(rect?.width ?? 1)), height: Math.max(1, Math.round(rect?.height ?? 1)) }
  }
  const invoke = async (request: NativeOfficeRequest) => {
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
      const key = `${next.x},${next.y},${next.width},${next.height}`
      if (key === lastBounds) return
      lastBounds = key
      void window.analytix.office.request({ action: 'bounds', bounds: next }).then((result) => {
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
    window.addEventListener('resize', update)
    return () => {
      stopped = true
      observer.disconnect()
      window.removeEventListener('resize', update)
      hide()
    }
  }, [visible, view?.objectId])
  return <div className="flex min-h-0 flex-1 flex-col">
    {error ? <p role="alert" className="shrink-0 px-3 py-2 text-sm text-ds-muted">{t('nativeOfficeOperationFailed')}</p> : null}
    <div ref={surface} className="min-h-0 flex-1" aria-label={t('nativeOfficeEditor')} />
  </div>
}
