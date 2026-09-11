import type {
  CSSProperties,
  MouseEvent as ReactMouseEvent,
  ReactElement,
  ReactNode,
  PointerEvent as ReactPointerEvent,
  TransitionEvent as ReactTransitionEvent
} from 'react'
import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Command, Search, X } from 'lucide-react'
import { AnalytixIconRegistry } from '../../design/AnalytixIconRegistry'

function cx(...parts: Array<string | false | null | undefined>): string {
  return parts.filter(Boolean).join(' ')
}

type SidebarFrameProps = {
  title: string
  children: ReactNode
  footer?: ReactNode
  onCollapse?: () => void
  className?: string
}

type SidebarCollapseMotionProps = {
  open: boolean
  children: ReactNode
  className?: string
  innerClassName?: string
  preserveAfterOpen?: boolean
  settleMode?: 'auto' | 'measured'
}

type SidebarFlowCollapseProps = {
  open: boolean
  children: ReactNode
  className?: string
  innerClassName?: string
  preserveAfterOpen?: boolean
}

const SIDEBAR_COLLAPSE_SETTLE_MS = 540

type SidebarCollapsePhase = 'closed' | 'opening-ready' | 'opening' | 'open' | 'closing-ready' | 'closing'

type SidebarFooterActionsProps = {
  settingsLabel: string
  connectPhoneLabel: string
  onOpenSettings: () => void
  onToggleConnectPhone: () => void
  connectPhoneActive?: boolean
}

function onNextFrame(callback: () => void): () => void {
  if (typeof globalThis.requestAnimationFrame === 'function') {
    const frame = globalThis.requestAnimationFrame(callback)
    return () => globalThis.cancelAnimationFrame(frame)
  }
  const timeout = globalThis.setTimeout(callback, 16)
  return () => globalThis.clearTimeout(timeout)
}

function measureSidebarCollapseHeight(node: HTMLDivElement | null): number {
  if (!node) return 0
  const rectHeight = node.getBoundingClientRect().height
  return Math.ceil(Math.max(rectHeight, node.scrollHeight))
}

export function SidebarCollapseMotion({
  open,
  children,
  className,
  innerClassName,
  preserveAfterOpen = false,
  settleMode = 'auto'
}: SidebarCollapseMotionProps): ReactElement | null {
  const motionRef = useRef<HTMLDivElement | null>(null)
  const contentRef = useRef<HTMLDivElement | null>(null)
  const phaseRef = useRef<SidebarCollapsePhase>(open ? 'open' : 'closed')
  const [phase, setPhase] = useState<SidebarCollapsePhase>(open ? 'open' : 'closed')
  const [contentHeight, setContentHeight] = useState(0)
  const [hasOpened, setHasOpened] = useState(open)

  useLayoutEffect(() => {
    phaseRef.current = phase
  }, [phase])

  useLayoutEffect(() => {
    let cancelFrame: (() => void) | undefined
    let timeout: number | undefined

    if (open) {
      setHasOpened(true)
      if (phaseRef.current === 'open') {
        setContentHeight(measureSidebarCollapseHeight(contentRef.current))
        return
      }
      setContentHeight(motionRef.current?.getBoundingClientRect().height ?? 0)
      setPhase('opening-ready')
      cancelFrame = onNextFrame(() => {
        setContentHeight(measureSidebarCollapseHeight(contentRef.current))
        setPhase('opening')
      })
      timeout = globalThis.setTimeout(() => {
        setPhase((current) => current === 'opening' ? 'open' : current)
      }, SIDEBAR_COLLAPSE_SETTLE_MS)
      return () => {
        cancelFrame?.()
        if (timeout !== undefined) globalThis.clearTimeout(timeout)
      }
    }

    if (phaseRef.current === 'closed') {
      return
    }

    const node = contentRef.current
    setContentHeight(motionRef.current?.getBoundingClientRect().height || measureSidebarCollapseHeight(node))
    setPhase('closing-ready')
    cancelFrame = onNextFrame(() => setPhase('closing'))
    timeout = globalThis.setTimeout(() => {
      setPhase((current) => current === 'closing' ? 'closed' : current)
    }, SIDEBAR_COLLAPSE_SETTLE_MS)
    return () => {
      cancelFrame?.()
      if (timeout !== undefined) globalThis.clearTimeout(timeout)
    }
  }, [open])

  useLayoutEffect(() => {
    if (!open || (phase !== 'opening' && !(phase === 'open' && settleMode === 'measured'))) return
    const node = contentRef.current
    if (!node) return
    const updateHeight = (): void => {
      setContentHeight(measureSidebarCollapseHeight(node))
    }
    updateHeight()
    if (typeof ResizeObserver === 'undefined') return
    const observer = new ResizeObserver(updateHeight)
    observer.observe(node)
    return () => observer.disconnect()
  }, [open, phase, settleMode])

  const handleTransitionEnd = (event: ReactTransitionEvent<HTMLDivElement>): void => {
    if (event.currentTarget !== event.target || event.propertyName !== 'height') return
    setPhase((current) => {
      if (current === 'opening') return 'open'
      if (current === 'closing') return 'closed'
      return current
    })
  }

  if (!open && phase === 'closed' && !(preserveAfterOpen && hasOpened)) return null

  const visuallyOpen = phase === 'opening-ready' || phase === 'opening' || phase === 'open'

  return (
    <div
      ref={motionRef}
      className={cx('ds-sidebar-collapse-motion', className)}
      data-open={visuallyOpen ? 'true' : 'false'}
      data-phase={phase}
      data-settled={phase === 'open' ? 'true' : 'false'}
      data-settle-mode={settleMode}
      aria-hidden={!open}
      onTransitionEnd={handleTransitionEnd}
      style={{ '--ds-sidebar-collapse-height': `${contentHeight}px` } as CSSProperties}
    >
      <div ref={contentRef} className={cx('ds-sidebar-collapse-motion-inner', innerClassName)}>
        {children}
      </div>
    </div>
  )
}

export function SidebarFlowCollapse({
  open,
  children,
  className,
  innerClassName,
  preserveAfterOpen = false
}: SidebarFlowCollapseProps): ReactElement | null {
  const phaseRef = useRef(open ? 'open' : 'closed')
  const [phase, setPhase] = useState<'closed' | 'open'>(open ? 'open' : 'closed')
  const [hasOpened, setHasOpened] = useState(open)

  useLayoutEffect(() => {
    phaseRef.current = phase
  }, [phase])

  useLayoutEffect(() => {
    if (open) {
      setHasOpened(true)
      if (phaseRef.current === 'open') return
      return onNextFrame(() => setPhase('open'))
    }
    if (phaseRef.current !== 'closed') setPhase('closed')
  }, [open])

  if (!open && !(preserveAfterOpen && hasOpened)) return null
  const flowOpen = phase === 'open'

  return (
    <div
      className={cx('ds-sidebar-flow-collapse', className)}
      data-open={flowOpen ? 'true' : 'false'}
      aria-hidden={!open}
    >
      <div className={cx('ds-sidebar-flow-collapse-inner', innerClassName)}>
        {children}
      </div>
    </div>
  )
}

type SidebarTitlebarToggleButtonProps = {
  title: string
  ariaLabel?: string
  onClick: () => void
  onPointerLeave?: (event: ReactPointerEvent<HTMLButtonElement>) => void
  className?: string
  children?: ReactNode
  collapsed?: boolean
  cursorSpotlight?: boolean
}

export function SidebarTitlebarToggleButton({
  title,
  ariaLabel,
  onClick,
  onPointerLeave,
  className,
  children,
  collapsed = false,
  cursorSpotlight = true
}: SidebarTitlebarToggleButtonProps): ReactElement {
  const [hoverSuppressed, setHoverSuppressed] = useState(false)
  const ToggleIcon = collapsed ? AnalytixIconRegistry.icons.sidebarShow : AnalytixIconRegistry.icons.sidebarHide
  const handleClick = (event: ReactMouseEvent<HTMLButtonElement>): void => {
    setHoverSuppressed(true)
    event.currentTarget.blur()
    onClick()
  }
  const handlePointerLeave = (event: ReactPointerEvent<HTMLButtonElement>): void => {
    setHoverSuppressed(false)
    onPointerLeave?.(event)
  }

  return (
    <button
      type="button"
      data-cursor-spotlight-target={cursorSpotlight ? true : undefined}
      onClick={handleClick}
      onPointerLeave={handlePointerLeave}
      title={title}
      aria-label={ariaLabel ?? title}
      className={cx('ds-titlebar-sidebar-toggle ds-no-drag', hoverSuppressed && 'is-hover-suppressed', className)}
    >
      {children ?? <ToggleIcon className="ds-sidebar-toggle-icon" />}
    </button>
  )
}

export function SidebarFrame({
  title,
  children,
  footer,
  onCollapse,
  className
}: SidebarFrameProps): ReactElement {
  return (
    <aside
      className={cx(
        'ds-no-drag ds-sidebar-shell relative flex h-full w-full shrink-0 flex-col overflow-hidden px-4 pb-3',
        className
      )}
    >
      <div className="ds-sidebar-titlebar-spacer shrink-0 pb-5 pt-3">
        <div className="ds-sidebar-titlebar-row flex min-h-[34px] items-start justify-between">
          <div aria-hidden className="ds-titlebar-safe-block min-w-[86px]" />
          {onCollapse ? (
            <SidebarTitlebarToggleButton
              onClick={onCollapse}
              title={title}
              ariaLabel={title}
              className="ds-sidebar-titlebar-toggle mt-[5px]"
              collapsed={false}
            />
          ) : null}
        </div>
      </div>

      {children}

      {footer ? (
        <div className="ds-no-drag -mx-2 mt-auto px-0 pt-0">
          {footer}
        </div>
      ) : null}
    </aside>
  )
}

type SidebarCommandRowProps = {
  icon: ReactElement
  label: string
  onClick?: () => void
  disabled?: boolean
  disabledHint?: string
  shortcut?: string
  variant?: 'flat' | 'accent' | 'footer'
  trailing?: ReactNode
  active?: boolean
  showChevron?: boolean
}

export function SidebarCommandRow({
  icon,
  label,
  onClick,
  disabled,
  disabledHint,
  shortcut,
  variant = 'flat',
  trailing,
  active = false,
  showChevron = false
}: SidebarCommandRowProps): ReactElement {
  const accent = variant === 'accent'
  const footer = variant === 'footer'
  return (
    <button
      type="button"
      data-cursor-spotlight-target
      disabled={disabled}
      title={disabled ? disabledHint : undefined}
      onClick={onClick}
      className={cx(
        'flex min-h-[34px] w-full items-center gap-2.5 rounded-[8px] px-3 py-1.5 text-[13px] font-normal transition',
        disabled
          ? 'cursor-not-allowed text-[#a8a8a8] opacity-55'
          : active
            ? 'bg-[var(--ds-sidebar-row-active)] text-[#1f1f1f] shadow-[inset_0_0_0_1px_var(--ds-sidebar-row-ring)] dark:text-white'
            : footer
              ? 'text-[#4f4f4f] hover:bg-[var(--ds-sidebar-row-hover)] hover:text-[#1f1f1f] dark:text-white/70 dark:hover:text-white'
              : accent
                ? 'text-[#1f1f1f] hover:bg-[var(--ds-sidebar-row-hover)] dark:text-white'
                : 'text-[#343434] hover:bg-[var(--ds-sidebar-row-hover)] hover:text-[#1f1f1f] dark:text-white/75 dark:hover:text-white'
      )}
    >
      <span
        className={cx(
          'flex h-5 w-5 shrink-0 items-center justify-center',
          accent ? 'text-[#1f1f1f] dark:text-white' : footer ? 'text-[#888888]' : 'text-[#343434] dark:text-white/75'
        )}
      >
        {icon}
      </span>
      <span className="min-w-0 flex-1 truncate text-left">{label}</span>
      {shortcut ? (
        <kbd className="ds-kbd hidden items-center gap-0.5 rounded-md px-1.5 py-0.5 font-mono text-[11.5px] font-medium text-ds-faint sm:inline-flex">
          <Command className="h-2.5 w-2.5" strokeWidth={2} />
          {shortcut.replace('⌘', '')}
        </kbd>
      ) : null}
      {trailing ?? null}
      {showChevron ? <AnalytixIconRegistry.icons.disclosureRight className="h-3.5 w-3.5 text-ds-faint" /> : null}
    </button>
  )
}

function SidebarSettingsIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden
    >
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M9.99944 7.24939C11.5169 7.2495 12.7473 8.47995 12.7475 9.99744C12.7475 11.5151 11.517 12.7454 9.99944 12.7455C8.48176 12.7455 7.2514 11.5151 7.2514 9.99744C7.25155 8.47988 8.48186 7.24939 9.99944 7.24939ZM9.99944 8.57947C9.2164 8.57947 8.58163 9.21442 8.58148 9.99744C8.58148 10.7806 9.2163 11.4154 9.99944 11.4154C10.7825 11.4153 11.4174 10.7805 11.4174 9.99744C11.4173 9.21449 10.7824 8.57958 9.99944 8.57947Z"
        fill="currentColor"
      />
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M10.6391 1.67517C11.2939 1.67532 11.8991 2.02577 12.226 2.59314L13.2485 4.36755H15.2963C15.9505 4.36758 16.555 4.71709 16.8823 5.28357L17.5219 6.39001C17.8489 6.95668 17.8481 7.65542 17.5209 8.22205L16.4975 9.99451L17.5239 11.7689C17.8519 12.3357 17.8521 13.0347 17.5248 13.6019L16.8862 14.7084C16.559 15.2747 15.9543 15.6243 15.3002 15.6244H13.2514L12.2299 17.3988C11.9029 17.9663 11.297 18.3168 10.642 18.3168L9.3637 18.3158C8.71064 18.3155 8.10718 17.9678 7.77972 17.4027L6.74847 15.6234L4.69964 15.6244C4.04558 15.6242 3.44087 15.2747 3.1137 14.7084L2.47503 13.6019C2.14791 13.0349 2.14836 12.3366 2.47601 11.7699L3.50237 9.99548L2.47894 8.22205C2.15175 7.65533 2.15174 6.95673 2.47894 6.39001L3.11761 5.28259C3.44458 4.71663 4.04894 4.36813 4.70257 4.36755L6.75042 4.36658L7.77581 2.59119C8.10301 2.02476 8.7076 1.67527 9.36175 1.67517H10.6391ZM9.36273 3.00623C9.1835 3.00623 9.01679 3.10199 8.92718 3.2572L7.82659 5.16345C7.63652 5.49253 7.28473 5.69529 6.90472 5.69568L4.70355 5.69763C4.52451 5.69782 4.3585 5.79355 4.26898 5.94861L3.6303 7.05505C3.54091 7.2102 3.54077 7.40192 3.6303 7.55701L4.73089 9.46326C4.92108 9.7929 4.92135 10.1992 4.73089 10.5287L3.62737 12.4359C3.5378 12.591 3.53792 12.7817 3.62737 12.9369L4.26605 14.0433C4.35567 14.1982 4.52067 14.2932 4.69964 14.2933L6.90276 14.2943C7.28242 14.2946 7.63335 14.497 7.82366 14.8256L8.93011 16.7357C9.01984 16.8905 9.18578 16.9857 9.36468 16.9857H10.642C10.8213 16.9857 10.987 16.89 11.0766 16.7347L12.1752 14.8275C12.3653 14.4975 12.7182 14.2943 13.0991 14.2943H15.3002C15.4794 14.2942 15.6452 14.1985 15.7348 14.0433L16.3725 12.9379C16.4621 12.7826 16.4621 12.5911 16.3725 12.4359L15.27 10.5287C15.1032 10.2404 15.0808 9.89331 15.2055 9.59021L15.269 9.46326L16.3696 7.55701C16.4591 7.40189 16.459 7.21022 16.3696 7.05505L15.7309 5.94861C15.6412 5.79363 15.4754 5.69863 15.2963 5.69861L13.0951 5.69763L12.9535 5.68884C12.6751 5.65158 12.4217 5.50519 12.2504 5.28259L12.1723 5.16443L11.0737 3.2572C10.9841 3.10175 10.8175 3.00525 10.6381 3.00525L9.36273 3.00623Z"
        fill="currentColor"
      />
    </svg>
  )
}

function SidebarConnectPhoneIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg
      width={16}
      height={16}
      viewBox="0 0 16 16"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className={className}
      aria-hidden
    >
      <path
        d="M8.72559 3.86914C8.97392 3.86932 9.17474 4.07097 9.1748 4.31934C9.1748 4.56776 8.97396 4.76936 8.72559 4.76953H7.34277C7.09425 4.76953 6.89258 4.56786 6.89258 4.31934C6.89264 4.07086 7.09429 3.86914 7.34277 3.86914H8.72559Z"
        fill="currentColor"
      />
      <path
        fillRule="evenodd"
        clipRule="evenodd"
        d="M9.93359 1.5498C11.2129 1.54984 12.2498 2.58698 12.25 3.86621V12.1338C12.2498 13.413 11.2129 14.4502 9.93359 14.4502H6.06641C4.78731 14.4499 3.75025 13.4129 3.75 12.1338V3.86621C3.75025 2.58711 4.78731 1.55005 6.06641 1.5498H9.93359ZM6.06641 2.4502C5.28436 2.45044 4.65064 3.08417 4.65039 3.86621V12.1338C4.65064 12.9158 5.28436 13.5496 6.06641 13.5498H9.93359C10.7158 13.5498 11.3503 12.916 11.3506 12.1338V3.86621C11.3503 3.08404 10.7158 2.45023 9.93359 2.4502H6.06641Z"
        fill="currentColor"
      />
    </svg>
  )
}

export function SidebarFooterActions({
  settingsLabel,
  connectPhoneLabel,
  onOpenSettings,
  onToggleConnectPhone,
  connectPhoneActive = false
}: SidebarFooterActionsProps): ReactElement {
  return (
    <div className="ds-sidebar-footer-actions">
      <div className="ds-sidebar-footer-row">
        <div className="ds-sidebar-footer-settings-cell">
          <button
            type="button"
            data-cursor-spotlight-target
            className="ds-sidebar-footer-settings-button"
            onClick={onOpenSettings}
          >
            <span className="ds-sidebar-footer-settings-content">
              <SidebarSettingsIcon className="ds-sidebar-footer-settings-icon" />
              <span className="ds-sidebar-footer-label">{settingsLabel}</span>
            </span>
          </button>
        </div>
        <button
          type="button"
          data-cursor-spotlight-target
          className="ds-sidebar-footer-phone-button"
          title={connectPhoneLabel}
          aria-label={connectPhoneLabel}
          aria-expanded={connectPhoneActive}
          onClick={onToggleConnectPhone}
        >
          <SidebarConnectPhoneIcon className="ds-sidebar-footer-phone-icon" />
        </button>
      </div>
    </div>
  )
}

type SidebarSectionHeaderProps = {
  label: string
  actions?: ReactNode
}

export function SidebarSectionHeader({
  label,
  actions
}: SidebarSectionHeaderProps): ReactElement {
  return (
    <div className="flex items-center justify-between px-2.5 pb-2 pt-5">
      <span className="min-w-0 truncate text-[12px] font-normal text-[#9aa5b5] dark:text-white/35">
        {label}
      </span>
      {actions ? <div className="flex shrink-0 items-center gap-0.5">{actions}</div> : null}
    </div>
  )
}

type SidebarIconButtonProps = {
  title: string
  children: ReactNode
  onClick?: () => void
  ariaLabel?: string
  disabled?: boolean
  active?: boolean
  tone?: 'default' | 'accent' | 'danger'
  className?: string
  stopPropagation?: boolean
}

export function SidebarIconButton({
  title,
  children,
  onClick,
  ariaLabel,
  disabled,
  active,
  tone = 'default',
  className,
  stopPropagation = false
}: SidebarIconButtonProps): ReactElement {
  const toneClass =
    tone === 'danger'
      ? 'hover:bg-red-500/10 hover:text-red-600 dark:hover:text-red-300'
      : tone === 'accent'
        ? 'hover:bg-[var(--ds-sidebar-row-hover)] hover:text-[#1f1f1f] dark:hover:text-white'
        : 'hover:bg-[var(--ds-sidebar-row-hover)] hover:text-[#1f1f1f] dark:hover:text-white'

  return (
    <button
      type="button"
      disabled={disabled}
      onPointerDown={(event) => {
        if (stopPropagation) event.stopPropagation()
      }}
      onClick={(event) => {
        if (stopPropagation) event.stopPropagation()
        onClick?.()
      }}
      className={cx(
        'ds-sidebar-icon-button ds-no-drag inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-transparent text-[#9a9a9a] transition disabled:cursor-not-allowed disabled:opacity-40 dark:text-white/45',
        active ? 'is-active text-[#1f1f1f] dark:text-white' : toneClass,
        className
      )}
      title={title}
      aria-label={ariaLabel ?? title}
      aria-pressed={active}
    >
      {children}
    </button>
  )
}

type SidebarSearchFieldProps = {
  value: string
  placeholder: string
  clearLabel: string
  onChange: (value: string) => void
}

export function SidebarSearchField({
  value,
  placeholder,
  clearLabel,
  onChange
}: SidebarSearchFieldProps): ReactElement {
  return (
    <label className="relative min-w-0 flex-1">
      <Search
        className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-ds-faint"
        strokeWidth={1.8}
      />
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="h-8 w-full rounded-[8px] border border-transparent bg-[var(--ds-sidebar-field-bg)] pl-7 pr-7 text-[13px] text-[#1f1f1f] outline-none transition placeholder:text-[#9aa5b5] focus:bg-[var(--ds-sidebar-field-focus)] dark:text-white"
      />
      {value.trim() ? (
        <button
          type="button"
          onClick={() => onChange('')}
          className="absolute right-1 top-1/2 flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-md text-[#9a9a9a] transition hover:bg-[var(--ds-sidebar-row-hover)] hover:text-[#1f1f1f] dark:hover:text-white"
          title={clearLabel}
          aria-label={clearLabel}
        >
          <X className="h-3.5 w-3.5" strokeWidth={1.9} />
        </button>
      ) : null}
    </label>
  )
}

type SidebarTreeRowProps = {
  children: ReactNode
  onClick?: () => void
  title?: string
  ariaLabel?: string
  onContextMenu?: (event: ReactMouseEvent<HTMLDivElement>) => void
  disabled?: boolean
  active?: boolean
  activeVariant?: 'rail' | 'outline'
  trailing?: ReactNode
  actions?: ReactNode
  actionsVisibility?: 'hidden' | 'subtle' | 'visible'
  actionsLayout?: 'inline' | 'overlay'
  className?: string
  buttonClassName?: string
  buttonStyle?: CSSProperties
}

export function SidebarTreeRow({
  children,
  onClick,
  title,
  ariaLabel,
  onContextMenu,
  disabled,
  active = false,
  activeVariant = 'rail',
  trailing,
  actions,
  actionsVisibility = 'subtle',
  actionsLayout = 'inline',
  className,
  buttonClassName,
  buttonStyle
}: SidebarTreeRowProps): ReactElement {
  const outlined = active && activeVariant === 'outline'
  const rail = activeVariant === 'rail'
  const actionsClass =
    actionsVisibility === 'hidden'
      ? 'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100'
      : actionsVisibility === 'visible'
        ? 'opacity-100'
        : 'opacity-55 group-hover:opacity-100 focus-within:opacity-100'
  const actionsWrapClass =
    actionsLayout === 'overlay'
      ? 'absolute inset-y-0 right-1.5 flex items-center gap-0.5'
      : 'mr-1.5 flex shrink-0 items-center gap-0.5'
  const trailingWrapClass =
    actionsLayout === 'overlay'
      ? 'mr-1.5 flex shrink-0 items-center gap-0.5 transition group-hover:opacity-0 group-focus-within:opacity-0'
      : 'flex shrink-0 items-center gap-0.5'

  return (
    <div
      className={cx(
        'group relative flex w-full items-center overflow-hidden rounded-[8px] text-[13px] font-normal transition',
        outlined
          ? 'bg-[var(--ds-sidebar-row-active)] text-[#1f1f1f] shadow-[inset_0_0_0_1px_var(--ds-sidebar-row-ring)] dark:text-white'
          : active
            ? 'bg-[var(--ds-sidebar-row-active)] text-[#1f1f1f] shadow-[inset_0_0_0_1px_var(--ds-sidebar-row-ring)] dark:text-white'
            : 'text-[#343434] hover:bg-[var(--ds-sidebar-row-hover)] dark:text-white/75',
        className
      )}
      title={title}
      onContextMenu={onContextMenu}
    >
      {rail ? (
        <span
          aria-hidden
          className={cx(
            'absolute bottom-1 left-0 top-1 w-[2px] rounded-full transition',
            active ? 'bg-transparent opacity-0' : 'bg-transparent opacity-0'
          )}
        />
      ) : null}
      <button
        type="button"
        onClick={onClick}
        disabled={disabled}
        aria-label={ariaLabel}
        className={cx(
          'flex min-w-0 flex-1 text-left disabled:cursor-not-allowed',
          buttonClassName ?? 'items-center gap-2 px-2.5 py-2'
        )}
        style={buttonStyle}
      >
        {children}
      </button>
      {trailing ? <div className={trailingWrapClass}>{trailing}</div> : null}
      {actions ? (
        <div className={actionsWrapClass}>
          <div className={cx('flex shrink-0 items-center gap-0.5 transition', actionsClass)}>
            {actions}
          </div>
        </div>
      ) : null}
    </div>
  )
}
