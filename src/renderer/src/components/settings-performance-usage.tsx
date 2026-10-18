/* Ported and adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md and the pinned provenance ledger. */
import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import type { AppSettingsPatch, PerformanceUsageMode } from '@shared/app-settings'
import { DshCheckOutlineRegular, DshChevronDownOutlineRegular } from './chat/presentation/DshToolIcons'
import { overlayTopMargin } from './chat/presentation/overlay-top-margin'
import { useDismissOnOutsidePointer } from './chat/presentation/useDismissOnOutsidePointer'
import { publishPerformanceUsageChoice, usePerformanceUsageMode } from './chat/presentation/chat/TurnUsagePanel'
import css from './settings-performance-usage.module.css'

type Translate = (key: string) => string
const options = ['compact', 'detailed'] as const

export function SettingsPerformanceUsage({ update, t }: {
  update: (patch: AppSettingsPatch) => void; t: Translate
}) {
  const mode = usePerformanceUsageMode()
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<CSSProperties | null>(null)
  const rootRef = useRef<HTMLSpanElement>(null)
  const triggerRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const backingRef = useRef<HTMLDivElement>(null)
  const focusCleanup = useRef<(() => void) | null>(null)
  const walkIndex = useRef<number | null>(null)
  const id = useId()
  const menuId = `performance-usage-${id}`
  const anchorStyle: CSSProperties & { '--dsh-menu-anchor': string } = {
    '--dsh-menu-anchor': `--dsh-performance-usage-${id.replaceAll(':', '')}`
  }
  const label = (value: PerformanceUsageMode): string => t(value === 'compact' ? 'performanceUsageCompact' : 'performanceUsageDetailed')
  const refocus = useCallback((navigation = false): void => {
    const button = triggerRef.current
    if (!button) return
    focusCleanup.current?.()
    if (navigation) { button.focus({ preventScroll: true }); return }
    const release = (): void => {
      button.removeAttribute('data-dsh-automatic-focus')
      button.removeEventListener('blur', release)
      button.removeEventListener('keydown', navigate, true)
      focusCleanup.current = null
    }
    const navigate = (event: KeyboardEvent): void => {
      if (!event.isComposing && !event.ctrlKey && !event.altKey && !event.metaKey &&
        ['Tab', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) release()
    }
    focusCleanup.current = release
    button.setAttribute('data-dsh-automatic-focus', '')
    button.addEventListener('blur', release)
    button.addEventListener('keydown', navigate, true)
    button.focus({ preventScroll: true })
    if (document.activeElement !== button) release()
  }, [])
  useEffect(() => () => focusCleanup.current?.(), [])
  useDismissOnOutsidePointer(rootRef, open, setOpen, menuRef)
  useLayoutEffect(() => {
    if (!open) { setPosition(null); walkIndex.current = null; return }
    // CSS anchor positioning requires the opaque backing after its menu anchor.
    if (backingRef.current) document.body.appendChild(backingRef.current)
    let frame = 0
    const place = (): void => {
      const rect = rootRef.current?.getBoundingClientRect(), menu = menuRef.current
      if (!rect || !menu) return
      const width = menu.offsetWidth, height = menu.offsetHeight, margin = overlayTopMargin(12)
      const left = width > 0 ? Math.min(Math.max(rect.right - width, 12), window.innerWidth - width - 12) : rect.right - width
      const top = height > 0 ? Math.min(Math.max(rect.bottom + 4, margin), window.innerHeight - height - 12) : rect.bottom + 4
      setPosition(previous => previous?.left === left && previous?.top === top ? previous : { left, top })
    }
    const track = (): void => { place(); frame = requestAnimationFrame(track) }
    place(); frame = requestAnimationFrame(track)
    window.addEventListener('scroll', place, true); window.addEventListener('resize', place)
    return () => { cancelAnimationFrame(frame); window.removeEventListener('scroll', place, true); window.removeEventListener('resize', place) }
  }, [open])
  useEffect(() => {
    if (!open) return
    let composing = false, ended = false
    const start = (): void => { composing = true }
    const end = (): void => { composing = false; ended = true }
    const release = (): void => { ended = false }
    const blur = (): void => {
      composing = false; ended = false
      if (document.activeElement instanceof HTMLIFrameElement) setOpen(false)
    }
    const key = (event: KeyboardEvent): void => {
      const guarded = composing || ended || event.isComposing || event.keyCode === 229
      ended = false
      if (guarded || event.defaultPrevented || event.ctrlKey || event.altKey || event.metaKey) return
      const foreground = Array.from(document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]'))
        .filter(dialog => !dialog.hidden && dialog.getClientRects().length > 0).at(-1)
      if (foreground && !foreground.contains(rootRef.current)) return
      const focused = document.activeElement
      const inside = menuRef.current?.contains(focused) === true
      const anchored = rootRef.current?.contains(focused) === true || inside
      if (event.key === 'Escape' && !event.shiftKey) {
        event.preventDefault()
        if (event.repeat) return
        setOpen(false)
        if (anchored) refocus()
        return
      }
      if (!anchored) return
      const buttons = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
      if (event.key === 'Tab') {
        if (event.shiftKey) { event.preventDefault(); setOpen(false); refocus(true); return }
        if (inside && focused instanceof HTMLElement && focused.getAttribute('role') === 'menuitem') {
          event.preventDefault(); focused.click()
        } else if (!inside && buttons[0]) { event.preventDefault(); buttons[0].focus(); walkIndex.current = 0 }
        return
      }
      if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || !buttons.length) return
      const current = buttons.indexOf(focused as HTMLButtonElement)
      const from = current >= 0 ? current : walkIndex.current
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : from === null
        ? event.key === 'ArrowDown' ? 0 : buttons.length - 1 : (from + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
      event.preventDefault(); walkIndex.current = next; buttons[next]?.focus()
    }
    const escape = (event: KeyboardEvent): void => { if (event.key === 'Escape') key(event) }
    const other = (event: KeyboardEvent): void => { if (event.key !== 'Escape') key(event) }
    document.addEventListener('compositionstart', start, true); document.addEventListener('compositionend', end, true)
    document.addEventListener('keyup', release, true); document.addEventListener('keydown', escape, true)
    document.addEventListener('keydown', other); window.addEventListener('blur', blur)
    return () => {
      document.removeEventListener('compositionstart', start, true); document.removeEventListener('compositionend', end, true)
      document.removeEventListener('keyup', release, true); document.removeEventListener('keydown', escape, true)
      document.removeEventListener('keydown', other); window.removeEventListener('blur', blur)
    }
  }, [open, refocus])
  const select = (value: PerformanceUsageMode): void => {
    triggerRef.current?.focus({ preventScroll: true })
    setOpen(false)
    if (value === mode) return
    publishPerformanceUsageChoice(value)
    update({ performanceUsageMode: value })
  }
  const fixedStyle = position ?? { visibility: 'hidden' as const, left: 0, top: 0 }
  return <div className={css.row}>
    <div className={css.rowText}><div className={css.title}>{t('performanceUsage')}</div><div className={css.desc}>{t('performanceUsageDesc')}</div></div>
    <span ref={rootRef} className={css.root}>
      <button ref={triggerRef} type="button" className={css.selector} aria-label={t('performanceUsage')}
        aria-haspopup="menu" aria-expanded={open} aria-controls={open ? menuId : undefined} onClick={() => setOpen(!open)}>
        {label(mode)}<DshChevronDownOutlineRegular size={14} className={css.chevron} />
      </button>
      {open && createPortal(<>
        <div ref={menuRef} id={menuId} role="menu" aria-label={t('performanceUsage')} data-menu-material="translucent"
          className={css.menu} style={{ ...fixedStyle, ...anchorStyle }} onClick={event => event.stopPropagation()}>
          <div className={css.material} aria-hidden="true" /><div className={css.viewport}>
            {options.map(value => <button type="button" role="menuitem" key={value} className={css.item}
              aria-label={label(value)} onClick={() => select(value)}><span className={css.itemText}>{label(value)}</span>
              {mode === value ? <DshCheckOutlineRegular size={14} className={css.check} /> : null}</button>)}
          </div>
        </div>
        <div ref={backingRef} aria-hidden="true" data-menu-backing="" className={css.backing} style={{ ...anchorStyle, visibility: fixedStyle.visibility }} />
      </>, document.body)}
    </span>
  </div>
}
