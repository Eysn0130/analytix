/* Command menu presentation adapted from DeepSeek Harness
 * 687f282e8625c6b8d11ff58211a90524be2e4317, Copyright (c) 2026 DeepSeek, MIT.
 * See THIRD_PARTY_NOTICES.md and docs/analytix/upstreams/code-reuse-provenance.md.
 * Analytix keeps its command handlers, editor focus and disclosure state. */
import { cloneElement, forwardRef, Fragment, useCallback, useId, useLayoutEffect, useRef, type KeyboardEventHandler, type ReactElement } from 'react'
import { DshCheckOutlineRegular } from './presentation/DshToolIcons'
import type { ComposerCommandGroup, ComposerCommandRow, SlashCommand } from './floating-composer-commands'

export type FloatingComposerCommandMenuProps = {
  items: readonly ComposerCommandRow[]
  selectedIndex?: number
  onSelect: (command: SlashCommand, index: number) => void
  onHover?: (index: number) => void
  groupLabels: Record<ComposerCommandGroup, string>
  role?: 'menu' | 'listbox'
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

export const FloatingComposerCommandMenu = forwardRef<HTMLDivElement, FloatingComposerCommandMenuProps>(
  function FloatingComposerCommandMenu({
    items, selectedIndex = -1, onSelect, onHover, groupLabels,
    role = 'menu', id, ariaLabel, emptyLabel, className, onKeyDown
  }, ref) {
    const generatedId = useId()
    const menuId = id ?? generatedId
    const selected = items[selectedIndex]
    const menuRef = useRef<HTMLDivElement | null>(null)
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
    }, [])
    return (
      <div
        ref={setMenuRef}
        id={menuId}
        role={role}
        aria-label={ariaLabel}
        aria-activedescendant={role === 'listbox' && selected ? composerCommandOptionId(menuId, selected.id) : undefined}
        data-composer-menu=""
        data-composer-command-menu=""
        className={`dsh-input-menu dsh-composer-command-menu${className ? ` ${className}` : ''}`}
        onKeyDown={onKeyDown}
      >
        <div className="dsh-composer-command-viewport">
          {items.length === 0 ? <div className="dsh-composer-command-empty" role="status">{emptyLabel}</div> : items.map((item, index) => {
            const active = index === selectedIndex
            const canonicalName = item.canonicalName ?? item.id
            const showAlias = item.title.toLowerCase() !== canonicalName.toLowerCase()
            const description = item.disabled && item.disabledReason ? item.disabledReason : item.description
            const optionId = composerCommandOptionId(menuId, item.id)
            return (
              <Fragment key={item.id}>
                {item.section && item.section !== items[index - 1]?.section ? (
                  <div className="dsh-composer-command-section" role="presentation" data-command-section={item.section}>
                    {groupLabels[item.section]}
                  </div>
                ) : null}
                <button
                  id={optionId}
                  type="button"
                  role={role === 'listbox' ? 'option' : item.checked === undefined ? 'menuitem' : 'menuitemcheckbox'}
                  aria-selected={role === 'listbox' ? active : undefined}
                  aria-checked={role === 'menu' ? item.checked : undefined}
                  aria-describedby={description ? `${optionId}-description` : undefined}
                  tabIndex={-1}
                  disabled={item.disabled}
                  title={item.disabled ? item.disabledReason : undefined}
                  data-command-id={item.id}
                  data-active={active || undefined}
                  className="dsh-composer-command-item"
                  onMouseDown={event => event.preventDefault()}
                  onMouseMove={active || item.disabled ? undefined : () => onHover?.(index)}
                  onClick={() => { if (!item.disabled) onSelect(item, index) }}
                >
                  <span className="dsh-composer-command-icon" aria-hidden="true">
                    {cloneElement(item.icon as ReactElement<{ size?: number; width?: number; height?: number }>, { size: 14, width: 14, height: 14 })}
                  </span>
                  <span className="dsh-composer-command-title">{item.title}</span>
                  {showAlias ? <span className="dsh-composer-command-alias">{canonicalName}</span> : null}
                  {description ? <span id={`${optionId}-description`} className="dsh-composer-command-description">{description}</span> : null}
                  {item.checked ? <span className="dsh-composer-command-check" aria-hidden="true"><DshCheckOutlineRegular size={14} /></span> : null}
                </button>
              </Fragment>
            )
          })}
        </div>
      </div>
    )
  }
)
