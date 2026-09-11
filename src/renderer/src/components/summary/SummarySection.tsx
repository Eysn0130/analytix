import type { CSSProperties, ReactElement, ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'

const AUTO_COLLAPSE_DELAY_MS = 30_000

export function SummarySection({
  title,
  count,
  autoCollapse = false,
  defaultCollapsed = false,
  after,
  children
}: {
  title: string
  count: number
  autoCollapse?: boolean
  defaultCollapsed?: boolean
  after?: ReactNode
  children: ReactNode
}): ReactElement {
  const defaultOpen = !(defaultCollapsed && count > 0)
  const [manualOpen, setManualOpen] = useState<boolean | null>(null)
  const [autoCollapsed, setAutoCollapsed] = useState(false)
  const [contentHeight, setContentHeight] = useState(0)
  const contentInnerRef = useRef<HTMLDivElement>(null)
  const open = manualOpen ?? (autoCollapse ? !autoCollapsed : defaultOpen)

  useLayoutEffect(() => {
    const element = contentInnerRef.current
    if (!element) return undefined
    let frameId: number | null = null
    const syncHeight = (): void => {
      if (frameId !== null) window.cancelAnimationFrame(frameId)
      frameId = window.requestAnimationFrame(() => {
        frameId = null
        setContentHeight(Math.ceil(element.scrollHeight))
      })
    }
    syncHeight()
    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', syncHeight)
      return () => {
        if (frameId !== null) window.cancelAnimationFrame(frameId)
        window.removeEventListener('resize', syncHeight)
      }
    }
    const observer = new ResizeObserver(syncHeight)
    observer.observe(element)
    return () => {
      if (frameId !== null) window.cancelAnimationFrame(frameId)
      observer.disconnect()
    }
  }, [])

  useEffect(() => {
    if (manualOpen !== null) return undefined
    if (!autoCollapse || count <= 0) {
      setAutoCollapsed(false)
      return undefined
    }
    const timer = window.setTimeout(() => setAutoCollapsed(true), AUTO_COLLAPSE_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [autoCollapse, count, manualOpen])

  const toggleOpen = (): void => {
    setManualOpen(!open)
  }

  return (
    <section className="relative z-0 flex flex-col pb-3 after:absolute after:inset-x-4 after:bottom-0 after:h-px after:scale-y-50 after:bg-ds-border-muted after:content-[''] last:pb-0 last:after:hidden">
      <button
        type="button"
        className="group/summary-section-toggle sticky top-0 z-10 flex h-7 w-full min-w-0 items-center gap-1.5 bg-ds-card-strong pb-0.5 pe-2.5 ps-4 text-left text-base font-semibold text-ds-faint"
        aria-expanded={open}
        onClick={toggleOpen}
      >
        <span className="min-w-0 truncate">{title}</span>
        {!open ? (
          <>
            <span className="text-base text-ds-faint opacity-70">{count}</span>
            <ChevronRight className="h-4 w-4 shrink-0 text-ds-muted transition-transform duration-300 ease-[cubic-bezier(0.19,1,0.22,1)] group-hover/summary-section-toggle:text-ds-ink" strokeWidth={2.25} />
          </>
        ) : null}
        {after ? <span className="ms-auto flex shrink-0 items-center">{after}</span> : null}
      </button>
      <div
        className="ds-summary-section-content"
        data-expanded={open ? 'true' : 'false'}
        aria-hidden={!open}
        style={{ '--ds-summary-section-content-height': `${contentHeight}px` } as CSSProperties}
      >
        <div ref={contentInnerRef} className="ds-summary-section-content-inner flex flex-col gap-0.5 px-4">
          {children}
        </div>
      </div>
    </section>
  )
}
