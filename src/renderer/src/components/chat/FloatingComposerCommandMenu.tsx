/* Command menu presentation adapted from DeepSeek Harness
 * 687f282e8625c6b8d11ff58211a90524be2e4317, Copyright (c) 2026 DeepSeek, MIT.
 * See THIRD_PARTY_NOTICES.md and docs/analytix/upstreams/code-reuse-provenance.md.
 * Analytix keeps its command handlers, editor focus and disclosure state. */
import { cloneElement, forwardRef, Fragment, useCallback, useId, useLayoutEffect, useRef, useState, type ForwardedRef, type KeyboardEventHandler, type ReactElement, type RefAttributes } from 'react'
import { DshCheckOutlineRegular, DshChevronRightOutlineRegular } from './presentation/DshToolIcons'
import type { ComposerCommandRow } from './floating-composer-commands'

/** Presentation only: each composer owner retains its original item and action type. */
export type FloatingComposerMenuItem = {
  id: string
  title: string
  description: string
  icon: ReactElement
  canonicalName?: string
  section?: string
  sectionLabel?: string
  disabled?: boolean
  disabledReason?: string
  checked?: boolean
  drill?: boolean
}

export type FloatingComposerCommandMenuProps<Item extends FloatingComposerMenuItem = ComposerCommandRow> = {
  items: readonly Item[]
  selectedIndex?: number
  onSelect: (item: Item, index: number) => void
  onHover?: (index: number) => void
  onDrill?: (item: Item, index: number) => void
  drillLabel?: string
  drillHint?: string
  sections?: readonly { id: string; label: string; pending?: boolean }[]
  breadcrumbs?: readonly { label: string; value: string; current?: boolean }[]
  breadcrumbsLabel?: string
  onBreadcrumb?: (value: string) => void
  groupLabels?: Partial<Record<string, string>>
  role?: 'menu' | 'listbox'
  loading?: boolean
  id?: string
  ariaLabel: string
  emptyLabel: string
  className?: string
  onKeyDown?: KeyboardEventHandler<HTMLDivElement>
}

/** Stable option identity survives catalog reordering and localized title changes. */
export function composerCommandOptionId(menuId: string, commandId: string): string {
  return `${menuId}-command-${encodeURIComponent(commandId)}`
}

/** Space above the composer anchor, keeping a 12px viewport inset and the 4px menu gap. */
export function composerCommandMenuMaxHeight(anchorTop: number): number {
  return Number.isFinite(anchorTop) ? Math.max(0, Math.min(400, anchorTop - 12 - 4)) : 0
}

function FloatingComposerCommandMenuInner<Item extends FloatingComposerMenuItem>({
    items, selectedIndex = -1, onSelect, onHover, onDrill, drillLabel, drillHint,
    sections, breadcrumbs, breadcrumbsLabel, onBreadcrumb, groupLabels,
    role = 'menu', loading = false, id, ariaLabel, emptyLabel, className, onKeyDown
  }: FloatingComposerCommandMenuProps<Item>, ref: ForwardedRef<HTMLDivElement>) {
    const generatedId = useId()
    const menuId = id ?? generatedId
    const selected = items[selectedIndex]
    const selectedId = selected?.id
    const menuRef = useRef<HTMLDivElement | null>(null)
    const viewportRef = useRef<HTMLDivElement | null>(null)
    const [overflowBelow, setOverflowBelow] = useState(false)
    const updateOverflow = useCallback(() => {
      const viewport = viewportRef.current
      setOverflowBelow(Boolean(viewport && viewport.scrollTop + viewport.clientHeight < viewport.scrollHeight - 1))
    }, [])
    const setMenuRef = useCallback((element: HTMLDivElement | null) => {
      menuRef.current = element
      if (typeof ref === 'function') ref(element)
      else if (ref) ref.current = element
    }, [ref])
    useLayoutEffect(() => {
      const menu = menuRef.current
      const anchor = menu?.parentElement
      if (!menu || !anchor) return
      let retired = false
      const place = (): void => {
        if (retired) return
        menu.style.maxHeight = `${composerCommandMenuMaxHeight(anchor.getBoundingClientRect().top)}px`
        updateOverflow()
      }
      place()
      window.addEventListener('resize', place)
      window.addEventListener('scroll', place, true)
      const observer = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(place)
      observer?.observe(anchor)
      return () => {
        retired = true
        observer?.disconnect()
        window.removeEventListener('resize', place)
        window.removeEventListener('scroll', place, true)
      }
    }, [updateOverflow])
    useLayoutEffect(updateOverflow, [items, sections, breadcrumbs, loading, updateOverflow])
    // DSH MenuView (also present at 5badb15009ae1756c3afe0ae0cef1faafc290ccc):
    // pointer and keyboard share one selected row, kept visible in the scroll viewport.
    useLayoutEffect(() => {
      if (selectedId == null) return
      document.getElementById(composerCommandOptionId(menuId, selectedId))
        ?.scrollIntoView?.({ block: 'nearest' })
    }, [menuId, selectedId, selectedIndex])
    const roster: readonly { id: string; label: string; pending?: boolean }[] = sections ?? [...new Set(items.map(item => item.section ?? ''))].map(section => ({
      id: section, label: items.find(item => item.section === section)?.sectionLabel ?? groupLabels?.[section] ?? section
    }))
    const hasCrumbs = Boolean(breadcrumbs?.length)
    const listProps = {
      id: menuId, role, 'aria-label': ariaLabel, 'aria-busy': loading || undefined,
      'aria-activedescendant': role === 'listbox' && selected ? composerCommandOptionId(menuId, selected.id) : undefined
    }
    return (
      <div
        ref={setMenuRef}
        {...hasCrumbs ? {} : listProps}
        data-composer-menu=""
        data-composer-command-menu=""
        data-overflow-below={overflowBelow || undefined}
        className={`dsh-input-menu dsh-composer-command-menu${className ? ` ${className}` : ''}`}
        onKeyDown={onKeyDown}
      >
        {hasCrumbs ? <nav className="dsh-composer-command-crumbs" aria-label={breadcrumbsLabel}>
          {breadcrumbs!.map((crumb, index) => <Fragment key={crumb.value}>
            {index > 0 ? <span className="dsh-composer-command-crumb-separator" aria-hidden="true"><DshChevronRightOutlineRegular size={12} /></span> : null}
            <button type="button" className="dsh-composer-command-crumb" aria-current={crumb.current ? 'location' : undefined}
              tabIndex={-1} onMouseDown={event => event.preventDefault()}
              onClick={() => { if (!crumb.current) onBreadcrumb?.(crumb.value) }}>{crumb.label}</button>
          </Fragment>)}
        </nav> : null}
        <div ref={viewportRef} className="dsh-composer-command-viewport" onScroll={updateOverflow} {...hasCrumbs ? listProps : {}}>
          {items.length === 0 && !loading ? <div className="dsh-composer-command-empty" role="status">{emptyLabel}</div> : null}
          {items.length === 0 && loading ? <span className="sr-only" role="status">{emptyLabel}</span> : null}
          {(roster.length ? roster : loading ? [{ id: '', label: '', pending: true }] : []).map(group => <Fragment key={group.id}>
            {group.id ? <div className="dsh-composer-command-section" role="presentation" data-command-section={group.id}>{group.label}</div> : null}
            {group.pending && !items.some(item => (item.section ?? '') === group.id) ? [0, 1].map(index => (
              <div key={index} className="dsh-composer-command-skeleton-row" aria-hidden="true"><span style={{ width: index ? '40%' : '60%' }} /></div>
            )) : null}
            {items.map((item, index) => {
            if ((item.section ?? '') !== group.id) return null
            const active = index === selectedIndex
            const canonicalName = item.canonicalName ?? item.id
            const showAlias = item.title.toLowerCase() !== canonicalName.toLowerCase()
            const description = item.disabled && item.disabledReason ? item.disabledReason : item.description
            const optionId = composerCommandOptionId(menuId, item.id)
            return (
              <Fragment key={item.id}>
                <button
                  id={optionId}
                  type="button"
                  role={role === 'listbox' ? 'option' : item.checked === undefined ? 'menuitem' : 'menuitemcheckbox'}
                  aria-selected={role === 'listbox' ? active : undefined}
                  aria-checked={role === 'menu' ? item.checked : undefined}
                  aria-disabled={group.pending || undefined}
                  aria-describedby={description ? `${optionId}-description` : undefined}
                  tabIndex={-1}
                  disabled={item.disabled}
                  title={item.disabled ? item.disabledReason : undefined}
                  data-command-id={item.id}
                  data-active={active || undefined}
                  className="dsh-composer-command-item"
                  onMouseDown={event => event.preventDefault()}
                  onMouseMove={active || item.disabled || group.pending ? undefined : () => onHover?.(index)}
                  onClick={() => { if (!item.disabled && !group.pending) onSelect(item, index) }}
                >
                  <span className="dsh-composer-command-icon" aria-hidden="true">
                    {cloneElement(item.icon as ReactElement<{ size?: number; width?: number; height?: number }>, { size: 14, width: 14, height: 14 })}
                  </span>
                  <span className="dsh-composer-command-title">{item.title}</span>
                  {showAlias ? <span className="dsh-composer-command-alias">{canonicalName}</span> : null}
                  {description ? <span id={`${optionId}-description`} className="dsh-composer-command-description">{description}</span> : null}
                  {item.checked ? <span className="dsh-composer-command-check" aria-hidden="true"><DshCheckOutlineRegular size={14} /></span> : null}
                  {item.drill && onDrill ? <span className="dsh-composer-command-trailing">
                    <span className="dsh-composer-command-drill-hint" aria-hidden="true">{drillHint} <kbd>Tab</kbd></span>
                    <span role="button" aria-label={drillLabel} aria-disabled={group.pending || undefined} tabIndex={-1}
                      className="dsh-composer-command-drill" onMouseDown={event => { event.preventDefault(); event.stopPropagation() }}
                      onClick={event => { event.stopPropagation(); if (!group.pending && !item.disabled) onDrill(item, index) }}>
                      <DshChevronRightOutlineRegular size={12} />
                    </span>
                  </span> : null}
                </button>
              </Fragment>
            )
          })}</Fragment>)}
        </div>
      </div>
    )
}

export const FloatingComposerCommandMenu = forwardRef(FloatingComposerCommandMenuInner) as
  <Item extends FloatingComposerMenuItem = ComposerCommandRow>(props: FloatingComposerCommandMenuProps<Item> & RefAttributes<HTMLDivElement>) => ReactElement
