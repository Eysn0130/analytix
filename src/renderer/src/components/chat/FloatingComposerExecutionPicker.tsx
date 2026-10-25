/* Presentation adapted from DeepSeek Harness 687f282e8625c6b8d11ff58211a90524be2e4317.
 * Copyright (c) 2026 DeepSeek. MIT; see THIRD_PARTY_NOTICES.md.
 * Analytix retains its own catalog, settings/next-turn owners and bridge. */
import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type ReactElement } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, Settings } from '../../design/AnalytixUiIcons'
import { useTranslation } from 'react-i18next'
import type { ApprovalPolicy, SandboxMode } from '@shared/app-settings'
import { hasChatTokenScope } from './presentation/Tooltip'
import './model-picker.css'

export type ComposerExecutionSettings = {
  approvalPolicy: ApprovalPolicy
  sandboxMode: SandboxMode
}

type Props = {
  value: ComposerExecutionSettings | null
  applying?: boolean
  unavailable?: boolean
  disabled?: boolean
  onChange: (patch: Partial<ComposerExecutionSettings>) => void
  onOpenPermissionSettings?: () => void
  /** The caller has identified an unversioned legacy pair; do not infer a new preset. */
  legacy?: boolean
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

type PermissionOption = {
  value: 'read-only' | 'workspace-write' | 'full-access'
  settings: ComposerExecutionSettings
  labelKey: string
  buttonLabelKey?: string
  descriptionKey: string
  Icon: (props: { className?: string }) => ReactElement
}

type ExecutionMenuAnchorRect = Pick<DOMRect, 'bottom' | 'left' | 'top' | 'width'>

type ExecutionMenuPlacement = {
  left: number
  top: number
  width: number
  maxHeight: number
}

const EXECUTION_MENU_MARGIN = 12
const EXECUTION_MENU_GAP = 4
const EXECUTION_MENU_WIDTH = 240
const EXECUTION_MENU_ESTIMATED_HEIGHT = 110

// These are explicit next-turn pairs. Legacy/global permission presets keep their own mapper.
const PERMISSION_OPTIONS: PermissionOption[] = [
  {
    value: 'read-only',
    settings: { approvalPolicy: 'never', sandboxMode: 'read-only' },
    labelKey: 'composerPermissionReadOnly',
    descriptionKey: 'composerPermissionReadOnlyDesc',
    Icon: PermissionReadOnlyIcon
  },
  {
    value: 'workspace-write',
    settings: { approvalPolicy: 'on-request', sandboxMode: 'workspace-write' },
    labelKey: 'composerPermissionWorkspaceWrite',
    descriptionKey: 'composerPermissionWorkspaceWriteDesc',
    Icon: PermissionWorkspaceWriteIcon
  },
  {
    value: 'full-access',
    settings: { approvalPolicy: 'auto', sandboxMode: 'danger-full-access' },
    labelKey: 'composerPermissionFullAccess',
    buttonLabelKey: 'composerPermissionFullAccessShort',
    descriptionKey: 'composerPermissionFullAccessDesc',
    Icon: PermissionFullAccessIcon
  }
]

function permissionOption(value: ComposerExecutionSettings): PermissionOption | undefined {
  return PERMISSION_OPTIONS.find((option) => option.settings.approvalPolicy === value.approvalPolicy && option.settings.sandboxMode === value.sandboxMode)
}

export function FloatingComposerExecutionPicker({
  value,
  applying = false,
  unavailable = false,
  disabled = false,
  onChange,
  legacy = false,
  open: controlledOpen,
  onOpenChange
}: Props): ReactElement {
  const { t } = useTranslation('common')
  const [internalOpen, setInternalOpen] = useState(false)
  const openChangeRef = useRef(onOpenChange)
  openChangeRef.current = onOpenChange
  const open = controlledOpen ?? internalOpen
  const setOpen = useCallback((next: boolean): void => {
    setInternalOpen(next)
    openChangeRef.current?.(next)
  }, [])
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({})
  const rootRef = useRef<HTMLDivElement | null>(null)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const entryFocusRef = useRef<'selected' | 'first' | 'last'>('selected')
  const menuOpen = open && !disabled && !applying
  const currentOption = legacy || !value ? undefined : permissionOption(value)
  const CurrentIcon = currentOption?.Icon ?? Settings
  const currentButtonLabel = t(currentOption?.buttonLabelKey ?? currentOption?.labelKey ?? 'composerPermissionCustom')
  const description = currentOption ? t(currentOption.descriptionKey)
    : value ? `${t('composerPermissionThreadCustomDesc')} (${value.approvalPolicy} / ${value.sandboxMode})` : t('composerPermissionUnavailable')
  const title = `${t('composerPermissionButton')}: ${currentButtonLabel}. ${description} ${unavailable && value ? t('composerPermissionUnavailable') : ''} ${t('composerPermissionNextTurnHint')}`

  const updateMenuPosition = useCallback((): void => {
    const rect = buttonRef.current?.getBoundingClientRect()
    if (!rect) return
    const menuHeight = menuRef.current?.offsetHeight || EXECUTION_MENU_ESTIMATED_HEIGHT
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
  }, [setOpen])

  const handleMenuKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>): void => {
    if (event.nativeEvent.isComposing || event.keyCode === 229) return
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
  }, [open, disabled, applying, updateMenuPosition, setOpen])

  const update = (patch: Partial<ComposerExecutionSettings>): void => {
    closeMenu(true)
    onChange(patch)
  }

  const selectOption = (option: PermissionOption): void => {
    if (disabled || applying) return
    update({ ...option.settings })
  }

  const menu = menuOpen && typeof document !== 'undefined' ? (
    <div
      ref={menuRef}
      role="menu"
      data-composer-execution-menu
      data-analytix-chat-token-scope={rootRef.current && hasChatTokenScope(rootRef.current) || undefined}
      aria-label={t('composerPermissionButton')}
      onKeyDown={handleMenuKeyDown}
      style={menuStyle}
      className="ds-model-menu ds-composer-picker-menu ds-execution-menu"
    >
      {PERMISSION_OPTIONS.map((option) => (
        <ExecutionRow
          key={option.value}
          selected={!unavailable && currentOption?.value === option.value}
          label={t(option.labelKey)}
          description={t(option.descriptionKey)}
          Icon={option.Icon}
          onClick={() => selectOption(option)}
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
            setOpen(!open)
          }}
          onKeyDown={(event) => {
            if (disabled || applying || event.nativeEvent.isComposing || event.keyCode === 229) return
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
          className={`ds-composer-picker-trigger ds-execution-trigger ${menuOpen ? 'ds-picker-trigger-open' : ''}`}
          title={title}
          aria-expanded={menuOpen}
          aria-haspopup="menu"
          aria-label={t('composerPermissionButton')}
        >
          <CurrentIcon className="ds-execution-trigger-icon" aria-hidden="true" />
          <span className="ds-execution-trigger-label">
            {applying ? t('composerExecutionApplying') : currentButtonLabel}
          </span>
          <ChevronDown className={`ds-picker-chevron ${menuOpen ? 'ds-picker-chevron-open' : ''}`} aria-hidden="true" />
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
  Icon: (props: { className?: string }) => ReactElement
  onClick: () => void
}): ReactElement {
  return (
    <button
      type="button"
      role="menuitemradio"
      tabIndex={-1}
      aria-checked={selected}
      onClick={onClick}
      className="ds-execution-option"
      title={description}
    >
      <Icon className="ds-execution-option-icon" aria-hidden="true" />
      <span className="ds-model-option-label">{label}</span>
      <span className="ds-model-check" aria-hidden="true">{selected ? <Check /> : null}</span>
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
  const width = Math.min(menuWidth, Math.max(0, normalizedViewportWidth - EXECUTION_MENU_MARGIN * 2))
  const maxHeight = Math.max(0, normalizedViewportHeight - EXECUTION_MENU_MARGIN * 2)
  const visibleHeight = Math.min(menuHeight, maxHeight)
  const anchorLeft = normalizedAnchorRect.left
  const topAbove = normalizedAnchorRect.top - visibleHeight - EXECUTION_MENU_GAP
  const top = topAbove >= EXECUTION_MENU_MARGIN
    ? topAbove
    : normalizedAnchorRect.bottom + EXECUTION_MENU_GAP

  return {
    top: executionMenuClamp(
      top,
      EXECUTION_MENU_MARGIN,
      Math.max(EXECUTION_MENU_MARGIN, normalizedViewportHeight - visibleHeight - EXECUTION_MENU_MARGIN)
    ),
    left: executionMenuClamp(
      anchorLeft,
      EXECUTION_MENU_MARGIN,
      Math.max(EXECUTION_MENU_MARGIN, normalizedViewportWidth - width - EXECUTION_MENU_MARGIN)
    ),
    width,
    maxHeight
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

// Selected regular permission artwork from ui-primitives/src/PermissionIcon.tsx at the pinned source above.
function PermissionReadOnlyIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg width={16} height={16} className={className} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
      <path d="M5.08545 8.13775L7.18455 10.2368C7.26636 10.3187 7.4003 10.3142 7.47649 10.2271L11.5148 5.61194" stroke="currentColor" />
      <path d="M6.59624 2.14853C7.50155 1.80917 8.49914 1.80919 9.40444 2.14859L13.9245 3.84317V7.11961C13.9245 11.6089 10.5565 13.5975 8.00035 14.5779C5.44423 13.5975 2.07544 11.6089 2.07544 7.11961V3.84317L6.59624 2.14853Z" stroke="currentColor" strokeLinejoin="round" />
    </svg>
  )
}

function PermissionWorkspaceWriteIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg width={16} height={16} className={className} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
      <path d="M6.4209 1.68067C7.43922 1.299 8.56177 1.29898 9.58008 1.68067L14.0996 3.375C14.2946 3.44811 14.4236 3.63455 14.4238 3.84278V6.89063C14.1115 6.71853 13.7761 6.58312 13.4238 6.48926V4.18946L9.22852 2.61621C8.43657 2.31947 7.56341 2.31939 6.77148 2.61621L2.5752 4.18946V7.11914C2.5752 11.1796 5.52369 13.056 8 14.0391C8.27653 13.9293 8.55827 13.8067 8.8418 13.6729C9.07101 13.9468 9.33228 14.1929 9.62012 14.4053C9.12409 14.6579 8.63578 14.8696 8.17871 15.0449C8.0637 15.0889 7.93628 15.0889 7.82129 15.0449C5.22011 14.0472 1.5752 11.9381 1.5752 7.11914V3.84278C1.57541 3.63469 1.70463 3.44821 1.89941 3.375L6.4209 1.68067Z" fill="currentColor" />
      <path d="M5.26392 6.60339H10.7361" stroke="currentColor" />
      <path d="M5.26392 9.86902H8.32833" stroke="currentColor" />
      <path d="M10.0317 13.2229C10.263 13.3929 10.4943 13.563 10.7256 13.733C10.7932 13.6482 10.8608 13.5634 10.9284 13.4786C12.1455 11.9522 13.3626 10.4258 14.5798 8.89935C14.6474 8.81455 14.715 8.72975 14.7826 8.64495C14.4143 8.37419 14.046 8.10344 13.6777 7.83268C13.6169 7.92252 13.5562 8.01236 13.4954 8.10219C12.4016 9.71926 11.3078 11.3363 10.214 12.9534C10.1532 13.0432 10.0924 13.1331 10.0317 13.2229Z" fill="currentColor" />
      <path d="M12.6516 12.6696C12.6516 12.925 12.6516 13.1804 12.6516 13.4359C12.6952 13.4378 12.7387 13.4398 12.7823 13.4417C13.5663 13.4768 14.3504 13.5118 15.1345 13.5469C15.178 13.5488 15.2216 13.5508 15.2651 13.5527C15.2651 13.2194 15.2651 12.8861 15.2651 12.5527C15.2216 12.5547 15.178 12.5566 15.1345 12.5586C14.3504 12.5936 13.5663 12.6287 12.7823 12.6637C12.7387 12.6657 12.6952 12.6676 12.6516 12.6696Z" fill="currentColor" />
    </svg>
  )
}

function PermissionFullAccessIcon({ className }: { className?: string }): ReactElement {
  return (
    <svg width={16} height={16} className={className} viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" strokeWidth={1}>
      <path d="M6.59624 2.14853C7.50155 1.80917 8.49914 1.80919 9.40444 2.14859L13.9245 3.84317V7.11961C13.9245 11.6089 10.5565 13.5975 8.00035 14.5779C5.44423 13.5975 2.07544 11.6089 2.07544 7.11961V3.84317L6.59624 2.14853Z" stroke="currentColor" strokeLinejoin="round" />
      <path d="M8 4.39209V9.89209" stroke="currentColor" />
      <path d="M8 10.8081V11.8081" stroke="currentColor" />
    </svg>
  )
}
