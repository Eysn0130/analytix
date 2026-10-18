/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { DshChevronDownOutlineRegular as ChevronDown, DshChevronUpOutlineRegular as ChevronUp } from '../DshToolIcons'
import { TextShimmer } from '../TextShimmer'
import { useSearchableHidden } from './searchable-hidden'
import css from './ChatGroupSeat.module.css'
type ProcessScrollMetrics = { top: number; floor: number }
const SCROLL_KEYS = new Set(['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '])
const scrollMetrics = (node: HTMLElement): ProcessScrollMetrics => ({ top: node.scrollTop, floor: Math.max(0, node.scrollHeight - node.clientHeight) })
class ProcessScrollFollow {
  private following = false
  private target: number | null = null
  private sampledTop: number | undefined
  get active(): boolean { return this.following }
  get animating(): boolean { return this.target !== null }
  private nearBottom(metrics: ProcessScrollMetrics): boolean { return metrics.floor - metrics.top <= 1 }
  reset(): void { this.following = false; this.target = null; this.sampledTop = undefined }
  stopFollowing(): void { this.following = false; this.target = null }
  sample(metrics: ProcessScrollMetrics, movedByReader = this.sampledTop === undefined || Math.abs(metrics.top - this.sampledTop) > 0.5): void {
    this.sampledTop = metrics.top
    if (!this.animating && movedByReader) this.following = this.nearBottom(metrics)
  }
  settle(metrics: ProcessScrollMetrics): void {
    const target = this.target
    this.target = null
    this.sample(metrics, target === null ? undefined : Math.abs(metrics.top - Math.min(target, metrics.floor)) > 1)
  }
  jump(node: HTMLElement, metrics: ProcessScrollMetrics, top: number): ProcessScrollMetrics {
    const animated = this.animating
    this.target = null
    const target = Math.max(0, Math.min(metrics.floor, top))
    if (animated) node.scrollTo({ top: target, behavior: 'instant' })
    else if (target !== metrics.top) node.scrollTop = target
    const landed = { ...metrics, top: node.scrollTop }
    this.sampledTop = landed.top
    this.following = this.nearBottom(landed)
    return landed
  }
  toBottom(node: HTMLElement, metrics: ProcessScrollMetrics): ProcessScrollMetrics {
    this.following = true
    if (metrics.top >= metrics.floor || (!this.animating && this.nearBottom(metrics))
      || document.documentElement.dataset.motionReduced === 'true'
      || typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches) {
      return this.jump(node, metrics, metrics.floor)
    }
    if (this.target === null) {
      this.target = metrics.floor
      node.scrollTo({ top: metrics.floor, behavior: 'smooth' })
    }
    return metrics
  }
  interrupt(node: HTMLElement, metrics: ProcessScrollMetrics): void {
    if (!this.animating) return
    this.target = null
    this.sampledTop = metrics.top
    node.scrollTo({ top: metrics.top, behavior: 'instant' })
  }
}
type LiveTitle = { activity: string; detail: string; preparing: boolean; icon: ReactNode; iconKey: string }
function useStableLiveProcessTitle(desired: LiveTitle, active: boolean): LiveTitle {
  const { activity, detail, preparing, icon, iconKey } = desired
  const [displayed, setDisplayed] = useState(desired)
  const displayedRef = useRef(displayed), desiredRef = useRef(desired), displayedAtRef = useRef(Date.now())
  useEffect(() => {
    desiredRef.current = { activity, detail, preparing, icon, iconKey }
    if (!active || displayedRef.current.activity === activity && displayedRef.current.detail === detail && displayedRef.current.preparing === preparing && displayedRef.current.iconKey === iconKey) return
    const remaining = 150 - (Date.now() - displayedAtRef.current)
    const commit = () => { const next = desiredRef.current; displayedRef.current = next; displayedAtRef.current = Date.now(); setDisplayed(next) }
    if (remaining <= 0) { commit(); return }
    const timer = setTimeout(commit, remaining); return () => clearTimeout(timer)
  }, [active, activity, detail, preparing, icon, iconKey])
  return active ? displayed : desired
}
export function ChatGroupSeat({ groupKey, grouped, open, visible, running, activity, preparing = false, detail, protectedDetail, label, settledTitle, icon, iconKey = activity, onToggle, onReveal, children }: {
  groupKey: string; grouped: boolean; open: boolean; visible: boolean; running: boolean; activity: string; preparing?: boolean; detail?: string; protectedDetail?: ReactNode
  label: (activity: string, preparing: boolean) => string; settledTitle?: string; icon: ReactNode; iconKey?: string; onToggle: () => void; onReveal?: () => void; children: ReactNode
}) {
  const bodyId = useId(), bodyRef = useRef<HTMLDivElement>(null), contentRef = useRef<HTMLDivElement>(null)
  const reveal = useCallback(() => onReveal?.(), [onReveal])
  useSearchableHidden(!visible, reveal, bodyRef)
  const initialPosition = useRef<'top' | 'bottom' | null>(null)
  const ownerKey = useRef(groupKey)
  const [follow] = useState(() => new ProcessScrollFollow())
  const [edges, setEdges] = useState({ up: false, down: false })
  const live = useStableLiveProcessTitle({ activity, detail: detail ?? '', preparing, icon, iconKey }, running)
  const interrupt = useCallback(() => {
    const node = bodyRef.current
    if (node && follow.animating) follow.interrupt(node, scrollMetrics(node))
  }, [follow])
  const sync = useCallback((cause: 'resize' | 'scroll' | 'scrollend') => {
    const node = bodyRef.current
    let next = { up: false, down: false }
    if (node && visible && open && grouped && node.closest('[hidden], [data-group-expanded-mode]') === null) {
      let metrics = scrollMetrics(node)
      const initial = cause === 'resize' ? initialPosition.current : null
      if (initial !== null) {
        metrics = follow.jump(node, metrics, initial === 'bottom' ? metrics.floor : 0)
        if (initial === 'top') follow.stopFollowing()
        initialPosition.current = null
      } else {
        const wasAnimating = follow.animating
        if (cause === 'scrollend') follow.settle(metrics)
        else follow.sample(metrics)
        // Native animation samples retain intent; growth waits for its current target.
        if (follow.active && (cause === 'resize' || cause === 'scrollend' && wasAnimating)) metrics = follow.toBottom(node, metrics)
      }
      next = { up: metrics.top > 1, down: metrics.top < metrics.floor - 1 }
    } else {
      interrupt()
      follow.reset()
    }
    setEdges(previous => next.up === previous.up && next.down === previous.down ? previous : next)
  }, [visible, open, grouped, follow, interrupt])
  useLayoutEffect(() => {
    interrupt()
    follow.reset()
    if (ownerKey.current !== groupKey || !visible || !open || !grouped) initialPosition.current = null
    ownerKey.current = groupKey
  }, [groupKey, visible, open, grouped, follow, interrupt])
  useLayoutEffect(() => {
    const node = bodyRef.current
    if (!node || !visible || !open || !grouped) return
    const onScrollEnd = (event: Event) => { if (event.target === node) sync('scrollend') }
    node.addEventListener('scrollend', onScrollEnd)
    const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(() => sync('resize'))
    observer?.observe(node)
    if (contentRef.current) observer?.observe(contentRef.current)
    return () => {
      observer?.disconnect()
      node.removeEventListener('scrollend', onScrollEnd)
      if (follow.animating) follow.interrupt(node, scrollMetrics(node))
      follow.reset()
    }
  }, [groupKey, visible, open, grouped, follow, sync])
  useLayoutEffect(() => {
    if (typeof ResizeObserver === 'undefined') sync('resize')
  }, [children, sync])
  const toggle = () => {
    if (!open) initialPosition.current = running ? 'bottom' : 'top'
    onToggle()
  }
  const titleIsCurrent = live.activity === activity && live.preparing === preparing && live.detail === (detail ?? '') && live.iconKey === iconKey
  const title = !running && settledTitle !== undefined ? settledTitle : `${label(live.activity, live.preparing)}${live.detail ? ' · ' + live.detail : ''}`
  return <div className={css.root} data-process-group={groupKey} data-chat-group-key={groupKey} data-step-process
    data-group-expanded-mode={!grouped || undefined}>
    <div hidden={!grouped}>
      {grouped ? <button type="button" className={css.title} data-disclosure-row data-process-activity={live.activity} data-process-preparing={live.preparing || undefined}
        aria-expanded={open} aria-controls={bodyId} onClick={event => { event.currentTarget.focus(); toggle() }}>
        <span className={css.leading} aria-hidden><span className={css.activityIcon}>{live.icon}</span>
          <span className={css.chevron}>{open ? <ChevronUp /> : <ChevronDown />}</span></span>
        <TextShimmer active={running} className={css.label}>{title}{running && titleIsCurrent && protectedDetail ? <> · {protectedDetail}</> : null}</TextShimmer>
      </button> : null}
    </div>
    <div ref={bodyRef} id={bodyId} data-step-process-body data-scroll-up={edges.up || undefined} data-scroll-down={edges.down || undefined}
      onScroll={() => sync('scroll')} onWheel={interrupt} onTouchStart={interrupt} onPointerDown={interrupt}
      onKeyDown={event => { if (!event.defaultPrevented && SCROLL_KEYS.has(event.key)) interrupt() }}
      className={[css.body, !grouped ? css.expandedBody : '', grouped && edges.up ? css.fadeTop : '', grouped && edges.down ? css.fadeBottom : ''].join(' ')}>
      <div ref={contentRef} className={css.content} data-step-process-content>{children}</div>
    </div>
  </div>
}
