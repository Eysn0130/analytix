import { useCallback, useEffect, useLayoutEffect, useRef, useState, type ReactElement } from 'react'
import { createPortal } from 'react-dom'
import {
  ChevronRight,
  Gauge,
  LogOut,
  Mail,
  Settings,
  Smartphone,
  UserCircle,
  UserPlus
} from 'lucide-react'
import type { SettingsRouteSection } from '../../store/chat-store'
import {
  accountInitials,
  displayAccountName,
  productEditionLabel,
  useHubAccountStore
} from '../../account/hub-account-store'
import { avatarDisplayUrl, shouldShowHubPhotoAvatar } from '../../account/hub-avatar'
import type { HubGatewayQuotaWindow, HubUsageResult } from '@shared/hub-account'
import {
  formatQuotaAmount,
  quotaWindowProgress,
  selectUsageQuotaWindows,
  usageQuotaEmptyState
} from './sidebar-account-usage'
import '../../account/startup-auth.css'

type SidebarAccountMenuProps = {
  connectPhoneActive: boolean
  connectPhoneLabel: string
  onOpenSettings: (section?: SettingsRouteSection) => void
  onToggleConnectPhone: () => void
}

type PopoverPosition = {
  left: number
  bottom: number
  width: number
  maxHeight: number
}

type ActiveDetail = 'referral' | 'usage' | null

function clampPopover(left: number, width: number): number {
  if (typeof window === 'undefined') return left
  return Math.min(Math.max(14, left), Math.max(14, window.innerWidth - width - 14))
}

function bodyCoordinateScale(): number {
  if (typeof window === 'undefined') return 1
  const raw = window.getComputedStyle(document.body).getPropertyValue('zoom') || '1'
  const scale = Number.parseFloat(raw)
  return Number.isFinite(scale) && scale > 0 ? scale : 1
}

const POPOVER_MIN_WIDTH = 280
const POPOVER_MAX_WIDTH = 320
const POPOVER_MARGIN = 8

function resetLabelForQuotaWindow(window: HubGatewayQuotaWindow): string {
  if (window.windowKind === 'session_5h') return '5h 窗口'
  if (window.windowKind === 'day') return '今日'
  if (window.windowKind === 'week') return '本周'
  if (window.windowKind === 'month') return '本月'
  return '周期'
}

function formatQuotaResetTime(window: HubGatewayQuotaWindow): string {
  const rawReset = window.resetAt || window.windowEnd
  if (!rawReset) return '重置时间同步中'
  const resetDate = new Date(rawReset)
  if (!Number.isFinite(resetDate.getTime())) return '重置时间同步中'

  const timeLabel = new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  }).format(resetDate)
  if (window.windowKind === 'day') return `明日 ${timeLabel} 重置`
  if (window.windowKind === 'week') {
    const weekDay = new Intl.DateTimeFormat('zh-CN', { weekday: 'short' }).format(resetDate)
    return `下${weekDay} ${timeLabel} 重置`
  }
  if (window.windowKind === 'month') {
    const monthDay = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' }).format(resetDate)
    return `${monthDay} ${timeLabel} 重置`
  }

  const now = new Date()
  const sameDay =
    resetDate.getFullYear() === now.getFullYear() &&
    resetDate.getMonth() === now.getMonth() &&
    resetDate.getDate() === now.getDate()
  const formatter = new Intl.DateTimeFormat('zh-CN', {
    ...(sameDay ? {} : { month: 'numeric', day: 'numeric' }),
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  })
  return `重置 ${formatter.format(resetDate)}`
}

function SidebarQuotaWindow({ window }: { window: HubGatewayQuotaWindow }): ReactElement {
  const progress = quotaWindowProgress(window)
  return (
    <div className="sidebar-account-popover__quota-window" data-exhausted={progress.exhausted ? 'true' : undefined}>
      <div className="sidebar-account-popover__quota-head">
        <span>{window.label || resetLabelForQuotaWindow(window)}</span>
        <span>已用 {progress.usedPercent}%</span>
      </div>
      <div className="sidebar-account-popover__quota-bar">
        <span style={{ width: `${progress.usedPercent}%` }} />
      </div>
      <div className="sidebar-account-popover__quota-foot">
        <span>{formatQuotaResetTime(window)}</span>
        <span>余量 {formatQuotaAmount(progress.remaining)}</span>
      </div>
    </div>
  )
}

export function SidebarAccountMenu({
  connectPhoneActive,
  connectPhoneLabel,
  onOpenSettings,
  onToggleConnectPhone
}: SidebarAccountMenuProps): ReactElement {
  const anchorRef = useRef<HTMLDivElement | null>(null)
  const popoverRef = useRef<HTMLDivElement | null>(null)
  const openRef = useRef(false)
  const usageLoadPromiseRef = useRef<Promise<HubUsageResult | null> | null>(null)
  const usagePendingOpenRef = useRef(false)
  const avatarRetryTimerRef = useRef<number | null>(null)
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState<PopoverPosition>({
    left: 18,
    bottom: 18,
    width: POPOVER_MIN_WIDTH,
    maxHeight: 420
  })
  const [activeDetail, setActiveDetail] = useState<ActiveDetail>(null)
  const [referralLoading, setReferralLoading] = useState(false)
  const [usageLoading, setUsageLoading] = useState(false)
  const [loggingOut, setLoggingOut] = useState(false)
  const [failedAvatarUrl, setFailedAvatarUrl] = useState('')
  const snapshot = useHubAccountStore((state) => state.snapshot)
  const refresh = useHubAccountStore((state) => state.refresh)
  const logout = useHubAccountStore((state) => state.logout)
  const usage = useHubAccountStore((state) => state.usage)
  const referral = useHubAccountStore((state) => state.referral)
  const accountError = useHubAccountStore((state) => state.error)
  const loadUsage = useHubAccountStore((state) => state.loadUsage)
  const loadReferral = useHubAccountStore((state) => state.loadReferral)
  const user = snapshot?.user
  const name = displayAccountName(snapshot)
  const edition = productEditionLabel(snapshot)
  const accountAvatarUrl = avatarDisplayUrl(user?.avatarUrl)
  const showAccountPhotoAvatar = shouldShowHubPhotoAvatar(user?.avatarStyle, accountAvatarUrl) && failedAvatarUrl !== accountAvatarUrl
  const usageQuotaWindows = selectUsageQuotaWindows(usage?.gatewayQuotaWindows ?? [])
  const usageEmptyState = usage && usageQuotaWindows.length === 0 ? usageQuotaEmptyState(usage) : null
  const usageDetailOpen = activeDetail === 'usage'
  const shouldRenderUsageDetail = usageDetailOpen || Boolean(usage)
  const portalTarget = typeof document === 'undefined' ? null : document.body

  useEffect(() => {
    setFailedAvatarUrl('')
    if (avatarRetryTimerRef.current !== null) {
      window.clearTimeout(avatarRetryTimerRef.current)
      avatarRetryTimerRef.current = null
    }
  }, [accountAvatarUrl])

  useEffect(() => () => {
    if (avatarRetryTimerRef.current !== null) {
      window.clearTimeout(avatarRetryTimerRef.current)
      avatarRetryTimerRef.current = null
    }
  }, [])

  useLayoutEffect(() => {
    if (!open) return
    let frameId = 0
    const updatePosition = () => {
      window.cancelAnimationFrame(frameId)
      frameId = window.requestAnimationFrame(() => {
        const rect = anchorRef.current?.getBoundingClientRect()
        if (!rect) return
        const scale = bodyCoordinateScale()
        const viewportWidth = Math.min(
          POPOVER_MAX_WIDTH,
          Math.max(POPOVER_MIN_WIDTH, rect.width + 20),
          window.innerWidth - 28
        )
        const viewportLeft = clampPopover(rect.left + rect.width / 2 - viewportWidth / 2, viewportWidth)
        const availableAbove = Math.max(180, rect.top - POPOVER_MARGIN - 14)
        const viewportBottom = Math.max(14, window.innerHeight - rect.top + POPOVER_MARGIN)
        setPosition({
          left: viewportLeft / scale,
          width: viewportWidth / scale,
          bottom: viewportBottom / scale,
          maxHeight: availableAbove / scale
        })
      })
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.cancelAnimationFrame(frameId)
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target
      if (!(target instanceof Node)) return
      if (anchorRef.current?.contains(target)) return
      const popover = popoverRef.current
      if (popover?.contains(target)) return
      setOpen(false)
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('pointerdown', handlePointerDown)
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown)
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [open])

  useEffect(() => {
    openRef.current = open
    if (open) return
    usagePendingOpenRef.current = false
    setActiveDetail(null)
  }, [open])

  const warmUsage = useCallback(() => {
    if (usageLoadPromiseRef.current) return usageLoadPromiseRef.current
    setUsageLoading(true)
    const promise = loadUsage()
      .catch(() => null)
      .finally(() => {
        usageLoadPromiseRef.current = null
        setUsageLoading(false)
      })
    usageLoadPromiseRef.current = promise
    return promise
  }, [loadUsage])

  useEffect(() => {
    if (!open || !snapshot?.authenticated) return
    void warmUsage()
  }, [open, snapshot?.authenticated, warmUsage])

  const openHubAccount = () => {
    void window.analytix?.app?.openExternal?.('https://analytix.top/user-console').catch(() => undefined)
    setOpen(false)
  }

  const handleOpenSettings = () => {
    onOpenSettings('general')
    setOpen(false)
  }

  const handleOpenAccountSettings = () => {
    onOpenSettings('account')
    setOpen(false)
  }

  const handleToggleAccountMenu = () => {
    const opening = !open
    setOpen(opening)
    if (opening && !snapshot) void refresh().catch(() => undefined)
  }

  const handleToggleReferral = () => {
    const expanding = activeDetail !== 'referral'
    usagePendingOpenRef.current = false
    setActiveDetail(expanding ? 'referral' : null)
    if (!expanding || referral) return
    setReferralLoading(true)
    void loadReferral().catch(() => undefined).finally(() => setReferralLoading(false))
  }

  const handleToggleUsage = () => {
    const expanding = activeDetail !== 'usage'
    if (!expanding) {
      usagePendingOpenRef.current = false
      setActiveDetail(null)
      return
    }

    if (usage) {
      usagePendingOpenRef.current = false
      setActiveDetail('usage')
      void warmUsage()
      return
    }

    usagePendingOpenRef.current = true
    void warmUsage().then((result) => {
      if (!usagePendingOpenRef.current || !openRef.current) return
      usagePendingOpenRef.current = false
      setActiveDetail('usage')
      if (!result) return
    })
  }

  const handleLogout = async () => {
    if (loggingOut) return
    setLoggingOut(true)
    await logout().catch(() => undefined)
    setLoggingOut(false)
    setOpen(false)
  }

  const copyReferral = async () => {
    const shareUrl = referral?.referral.shareUrl
    if (!shareUrl) return
    await navigator.clipboard?.writeText(shareUrl).catch(() => undefined)
  }

  const markAvatarFailed = (): void => {
    if (!accountAvatarUrl) return
    setFailedAvatarUrl(accountAvatarUrl)
    if (avatarRetryTimerRef.current !== null) return
    avatarRetryTimerRef.current = window.setTimeout(() => {
      avatarRetryTimerRef.current = null
      setFailedAvatarUrl('')
    }, 1800)
  }

  const popover = open && portalTarget ? createPortal(
    <div
      ref={popoverRef}
      className="sidebar-account-popover"
      style={{ left: position.left, bottom: position.bottom, width: position.width, maxHeight: position.maxHeight }}
      role="menu"
      aria-label="Analytix account"
    >
      <div className="sidebar-account-popover__section">
        <button className="sidebar-account-popover__item" type="button" disabled role="menuitem">
          <Mail size={21} strokeWidth={1.9} />
          <span>{user?.email || '未登录'}</span>
          <span />
        </button>
        <button className="sidebar-account-popover__item" type="button" onClick={handleOpenAccountSettings} role="menuitem">
          <UserCircle size={22} strokeWidth={1.9} />
          <span>个人账户</span>
          <ChevronRight size={20} strokeWidth={1.9} />
        </button>
      </div>
      <div className="sidebar-account-popover__divider" />
      <div className="sidebar-account-popover__section">
        <button className="sidebar-account-popover__item" type="button" onClick={handleOpenSettings} role="menuitem">
          <Settings size={22} strokeWidth={1.9} />
          <span>设置</span>
          <span>⌘,</span>
        </button>
        <button
          className="sidebar-account-popover__item"
          type="button"
          onClick={handleToggleReferral}
          role="menuitem"
          aria-expanded={activeDetail === 'referral'}
        >
          <UserPlus size={22} strokeWidth={1.9} />
          <span>邀请好友</span>
          <ChevronRight
            className="sidebar-account-popover__chevron"
            data-open={activeDetail === 'referral'}
            size={20}
            strokeWidth={1.9}
          />
        </button>
        {activeDetail === 'referral' && referral?.referral.shareUrl ? (
          <div className="sidebar-account-popover__detail">
            <span>奖励额度：{referral.referral.rewardTokenGrant.toLocaleString()} tokens</span>
            <div className="sidebar-account-popover__copy">
              <input readOnly value={referral.referral.shareUrl} data-copy-allowed="true" />
              <button type="button" onClick={() => void copyReferral()}>复制</button>
            </div>
          </div>
        ) : activeDetail === 'referral' ? (
          <div className="sidebar-account-popover__detail sidebar-account-popover__detail--status">
            <span>{referralLoading ? '正在获取邀请链接...' : accountError || '暂无邀请链接，请稍后重试。'}</span>
            {!referralLoading && accountError ? (
              <button className="sidebar-account-popover__detail-button" type="button" onClick={openHubAccount}>
                打开个人账户
              </button>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="sidebar-account-popover__divider" />
      <div className="sidebar-account-popover__section">
        <button
          className="sidebar-account-popover__item"
          type="button"
          onClick={handleToggleUsage}
          role="menuitem"
          aria-expanded={activeDetail === 'usage'}
        >
          <Gauge size={22} strokeWidth={1.9} />
          <span>剩余用量</span>
          <ChevronRight
            className="sidebar-account-popover__chevron"
            data-open={activeDetail === 'usage'}
            size={20}
            strokeWidth={1.9}
          />
        </button>
        {shouldRenderUsageDetail ? (
          <div
            className={`sidebar-account-popover__detail sidebar-account-popover__detail--usage${usage && usageQuotaWindows.length ? '' : ' sidebar-account-popover__detail--status'}`}
            data-open={usageDetailOpen}
            aria-hidden={!usageDetailOpen}
          >
            {usage ? (
              usageQuotaWindows.length ? usageQuotaWindows.map((window) => (
                <SidebarQuotaWindow key={window.id} window={window} />
              )) : usageEmptyState ? (
                <div className="sidebar-account-popover__usage-empty">
                  <span className="sidebar-account-popover__usage-empty-title">{usageEmptyState.title}</span>
                  <span>{usageEmptyState.detail}</span>
                </div>
              ) : null
            ) : usageDetailOpen ? (
              <span>{usageLoading ? '正在获取剩余用量...' : accountError || '正在同步最新用量，请稍后重试。'}</span>
            ) : null}
          </div>
        ) : null}
      </div>
      <div className="sidebar-account-popover__divider" />
      <div className="sidebar-account-popover__section">
        <button
          className="sidebar-account-popover__item"
          type="button"
          onClick={() => void handleLogout()}
          disabled={loggingOut}
          role="menuitem"
        >
          <LogOut size={22} strokeWidth={1.9} />
          <span>{loggingOut ? '正在退出...' : '退出登录'}</span>
          <span />
        </button>
      </div>
    </div>,
    portalTarget
  ) : null

  return (
    <div ref={anchorRef} className="sidebar-account-menu ds-no-drag" data-open={open}>
      <button
        type="button"
        className="sidebar-account-trigger"
        aria-expanded={open}
        onClick={handleToggleAccountMenu}
      >
        <span className="sidebar-account-avatar" data-photo={showAccountPhotoAvatar ? 'true' : undefined}>
          {showAccountPhotoAvatar ? (
            <img
              src={accountAvatarUrl}
              alt=""
              draggable={false}
              onLoad={() => setFailedAvatarUrl('')}
              onError={markAvatarFailed}
            />
          ) : (
            accountInitials(snapshot)
          )}
        </span>
        <span>
          <span className="sidebar-account-name">{name}</span>
          <span className="sidebar-account-edition">{edition}</span>
        </span>
      </button>
      <button
        className="sidebar-account-phone"
        title={connectPhoneLabel}
        aria-label={connectPhoneLabel}
        aria-pressed={connectPhoneActive}
        type="button"
        onClick={() => {
          setOpen(false)
          onToggleConnectPhone()
        }}
      >
        <Smartphone size={22} strokeWidth={1.8} />
      </button>
      {popover}
    </div>
  )
}

export default SidebarAccountMenu
