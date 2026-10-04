import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type ReactElement } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, Hand, LockKeyholeOpen, Settings, ShieldQuestion } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import {
  analytixToolPermissionModeFromSettings,
  analytixToolPermissionModeSettings,
  type AnalytixToolPermissionMode,
  type AnalytixToolPermissionPreset,
  type ApprovalPolicy,
  type SandboxMode
} from '@shared/app-settings'

export type ComposerExecutionSettings = {
  approvalPolicy: ApprovalPolicy
  sandboxMode: SandboxMode
}

type Props = {
  value: ComposerExecutionSettings
  applying?: boolean
  disabled?: boolean
  onChange: (patch: Partial<ComposerExecutionSettings>) => void
  onOpenPermissionSettings?: () => void
}

type PermissionOption = {
  value: AnalytixToolPermissionMode
  labelKey: string
  buttonLabelKey?: string
  descriptionKey: string
  Icon: typeof Hand
}

type ExecutionMenuAnchorRect = Pick<DOMRect, 'bottom' | 'left' | 'top' | 'width'>

type ExecutionMenuPlacement = {
  left: number
  top: number
  width: number
}

const EXECUTION_MENU_MARGIN = 12
const EXECUTION_MENU_GAP = 8
const EXECUTION_MENU_WIDTH = 332
const EXECUTION_MENU_ESTIMATED_HEIGHT = 286

const PERMISSION_OPTIONS: PermissionOption[] = [
  {
    value: 'request-approval',
    labelKey: 'composerPermissionRequestApproval',
    descriptionKey: 'composerPermissionRequestApprovalDesc',
    Icon: Hand
  },
  {
    value: 'auto-approval',
    labelKey: 'composerPermissionAutoApproval',
    descriptionKey: 'composerPermissionAutoApprovalDesc',
    Icon: ShieldQuestion
  },
  {
    value: 'full-access',
    labelKey: 'composerPermissionFullAccess',
    buttonLabelKey: 'composerPermissionFullAccessShort',
    descriptionKey: 'composerPermissionFullAccessDesc',
    Icon: LockKeyholeOpen
  },
  {
    value: 'custom',
    labelKey: 'composerPermissionCustom',
    descriptionKey: 'composerPermissionCustomDesc',
    Icon: Settings
  }
]

function permissionOption(mode: AnalytixToolPermissionMode): PermissionOption {
  return PERMISSION_OPTIONS.find((option) => option.value === mode) ?? PERMISSION_OPTIONS[2]
}

export function FloatingComposerExecutionPicker({
  value,
  applying = false,
  disabled = false,
  onChange,
  onOpenPermissionSettings
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(false)
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({})
  const rootRef = useRef<HTMLDivElement | null>(null)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const entryFocusRef = useRef<'selected' | 'first' | 'last'>('selected')
  const menuOpen = open && !disabled && !applying
  const permissionMode = analytixToolPermissionModeFromSettings(value)
  const currentOption = permissionOption(permissionMode)
  const currentButtonLabel = t(currentOption.buttonLabelKey ?? currentOption.labelKey)
  const title = `${t('composerPermissionButton')}: ${currentButtonLabel}. ${t(currentOption.descriptionKey)}`

  const updateMenuPosition = useCallback((): void => {
    const rect = buttonRef.current?.getBoundingClientRect()
    if (!rect) return
    const menuHeight = menuRef.current?.offsetHeight ?? EXECUTION_MENU_ESTIMATED_HEIGHT
    setMenuStyle(calculateExecutionMenuPlacement({
      anchorRect: rect,
      menuWidth: EXECUTION_MENU_WIDTH,
      menuHeight,
      viewportHeight: window.innerHeight,
      viewportWidth: window.innerWidth,
      coordinateScale: currentBodyZoom()
    }))
  }, [])

  const closeMenu = useCallback((restoreFocus = false): void => {
    setOpen(false)
    const button = buttonRef.current
    if (restoreFocus && button?.isConnected && !button.disabled) {
      button.focus({ preventScroll: true })
    }
  }, [])

  const handleMenuKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>): void => {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      closeMenu(true)
      return
    }
    if (event.key === 'Tab') {
      // Resume the toolbar's native Tab order rather than the body's portal order.
      closeMenu(true)
      return
    }
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
    const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>(
      "[role='menuitemradio']:not(:disabled)"
    ))
    if (items.length === 0) return
    event.preventDefault()
    const current = items.indexOf(document.activeElement as HTMLButtonElement)
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
      : (current + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length
    items[next]?.focus({ preventScroll: true })
  }

  useEffect(() => {
    if (!open) return
    if (disabled || applying) {
      setOpen(false)
      return
    }
    updateMenuPosition()
    const frame = window.requestAnimationFrame(() => {
      updateMenuPosition()
      const items = menuRef.current?.querySelectorAll<HTMLButtonElement>("[role='menuitemradio']:not(:disabled)")
      if (!items?.length || buttonRef.current?.disabled) return
      const item = entryFocusRef.current === 'last' ? items[items.length - 1]
        : entryFocusRef.current === 'first' ? items[0]
        : menuRef.current?.querySelector<HTMLButtonElement>("[role='menuitemradio'][aria-checked='true']:not(:disabled)") ?? items[0]
      item?.focus({ preventScroll: true })
    })
    const onPointerDown = (event: Event): void => {
      const target = event.target
      if (target instanceof Node && rootRef.current?.contains(target)) return
      if (target instanceof Node && menuRef.current?.contains(target)) return
      setOpen(false)
    }
    const onUpdatePosition = (): void => updateMenuPosition()
    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('focusin', onPointerDown)
    window.addEventListener('resize', onUpdatePosition)
    window.addEventListener('scroll', onUpdatePosition, true)
    return () => {
      window.cancelAnimationFrame(frame)
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('focusin', onPointerDown)
      window.removeEventListener('resize', onUpdatePosition)
      window.removeEventListener('scroll', onUpdatePosition, true)
    }
  }, [open, disabled, applying, updateMenuPosition])

  const update = (patch: Partial<ComposerExecutionSettings>): void => {
    closeMenu(true)
    onChange(patch)
  }

  const selectOption = (mode: AnalytixToolPermissionMode): void => {
    if (disabled || applying) return
    if (mode === 'custom') {
      setOpen(false)
      onOpenPermissionSettings?.()
      return
    }
    update(analytixToolPermissionModeSettings(mode as AnalytixToolPermissionPreset))
  }

  const menu = menuOpen && typeof document !== 'undefined' ? (
    <div
      ref={menuRef}
      role="menu"
      data-composer-execution-menu
      aria-label={t('composerPermissionButton')}
      onKeyDown={handleMenuKeyDown}
      style={menuStyle}
      className="fixed z-50 overflow-hidden rounded-[20px] border border-ds-border bg-ds-elevated p-1.5 text-[13px] text-ds-ink [box-shadow:var(--ax-shadow-popover)]"
    >
      {PERMISSION_OPTIONS.map((option) => (
        <ExecutionRow
          key={option.value}
          selected={permissionMode === option.value}
          label={t(option.labelKey)}
          description={t(option.descriptionKey)}
          Icon={option.Icon}
          onClick={() => selectOption(option.value)}
        />
      ))}
    </div>
  ) : null

  return (
    <>
      <div ref={rootRef} className="ds-no-drag relative inline-flex shrink-0">
        <button
          ref={buttonRef}
          type="button"
          disabled={disabled || applying}
          onClick={() => {
            entryFocusRef.current = 'selected'
            setOpen((current) => !current)
          }}
          onKeyDown={(event) => {
            if (disabled || applying) return
            if (event.key === 'Escape' && menuOpen) {
              event.preventDefault()
              event.stopPropagation()
              closeMenu(true)
            } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
              event.preventDefault()
              entryFocusRef.current = event.key === 'ArrowUp' ? 'last' : 'first'
              setOpen(true)
            }
          }}
          className={`inline-flex h-8 max-w-[132px] shrink-0 items-center gap-1.5 rounded-full px-2.5 text-[13px] font-medium text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-55 ${
            menuOpen ? 'bg-ds-hover text-ds-ink' : ''
          }`}
          title={title}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          aria-label={t('composerPermissionButton')}
        >
          <Settings className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
          <span className="min-w-0 truncate">
            {applying ? t('composerExecutionApplying') : currentButtonLabel}
          </span>
          <ChevronDown className="h-3.5 w-3.5 shrink-0" strokeWidth={1.9} />
        </button>
      </div>
      {menu ? createPortal(menu, document.body) : null}
    </>
  )
}

function ExecutionRow({
  selected,
  label,
  description,
  Icon,
  onClick
}: {
  selected: boolean
  label: string
  description: string
  Icon: typeof Hand
  onClick: () => void
}): ReactElement {
  return (
    <button
      type="button"
      role="menuitemradio"
      tabIndex={-1}
      aria-checked={selected}
      onClick={onClick}
      className={`flex w-full cursor-pointer items-start gap-3 rounded-[14px] px-3 py-2.5 text-left transition ${
        selected ? 'bg-ds-hover text-ds-ink' : 'text-ds-muted hover:bg-ds-hover/70 hover:text-ds-ink'
      }`}
    >
      <span className="mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center text-ds-muted">
        <Icon className="h-4 w-4" strokeWidth={1.9} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[14px] font-semibold leading-5 text-ds-ink">{label}</span>
        <span className="mt-0.5 block text-[12px] leading-snug text-ds-muted">{description}</span>
      </span>
      {selected ? <Check className="mt-1 h-4 w-4 shrink-0 text-ds-ink" strokeWidth={2} /> : null}
    </button>
  )
}

export function calculateExecutionMenuPlacement({
  anchorRect,
  menuWidth,
  menuHeight,
  viewportHeight,
  viewportWidth,
  coordinateScale = 1
}: {
  anchorRect: ExecutionMenuAnchorRect
  menuWidth: number
  menuHeight: number
  viewportHeight: number
  viewportWidth: number
  coordinateScale?: number
}): ExecutionMenuPlacement {
  const scale = Number.isFinite(coordinateScale) && coordinateScale > 0 ? coordinateScale : 1
  const normalizedAnchorRect = {
    bottom: anchorRect.bottom / scale,
    left: anchorRect.left / scale,
    top: anchorRect.top / scale,
    width: anchorRect.width / scale
  }
  const normalizedViewportHeight = viewportHeight / scale
  const normalizedViewportWidth = viewportWidth / scale
  const anchorLeft = normalizedAnchorRect.left + normalizedAnchorRect.width / 2 - menuWidth / 2
  const topAbove = normalizedAnchorRect.top - menuHeight - EXECUTION_MENU_GAP
  const top = topAbove >= EXECUTION_MENU_MARGIN
    ? topAbove
    : normalizedAnchorRect.bottom + EXECUTION_MENU_GAP

  return {
    top: executionMenuClamp(
      top,
      EXECUTION_MENU_MARGIN,
      Math.max(EXECUTION_MENU_MARGIN, normalizedViewportHeight - menuHeight - EXECUTION_MENU_MARGIN)
    ),
    left: executionMenuClamp(
      anchorLeft,
      EXECUTION_MENU_MARGIN,
      Math.max(EXECUTION_MENU_MARGIN, normalizedViewportWidth - menuWidth - EXECUTION_MENU_MARGIN)
    ),
    width: menuWidth
  }
}

function currentBodyZoom(): number {
  if (typeof window === 'undefined') return 1
  const zoom = window.getComputedStyle(document.body).zoom
  const parsed = Number.parseFloat(zoom)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1
}

function executionMenuClamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}
