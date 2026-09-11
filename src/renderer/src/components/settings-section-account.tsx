import { useEffect, useMemo, useRef, useState, type ChangeEvent, type FormEvent, type ReactElement } from 'react'
import {
  Box,
  Brain,
  Check,
  FileText,
  LockKeyhole,
  MousePointer2,
  Palette,
  PencilLine,
  Share2,
  Sparkles,
  type LucideIcon,
  X
} from 'lucide-react'
import type {
  HubAccountSnapshot,
  HubAvatarStyle,
  HubGatewayProfile,
  HubProfileEventSyncItem,
  HubProfileInvocation
} from '@shared/hub-account'
import { avatarDisplayUrl, shouldShowHubPhotoAvatar } from '../account/hub-avatar'
import {
  accountInitials,
  displayAccountName,
  productEditionLabel,
  useHubAccountStore
} from '../account/hub-account-store'
import type { ChatBlock, NormalizedThread, ThreadUsageSnapshot } from '../agent/types'
import { useChatStore } from '../store/chat-store'
import xiezhiFigure from '../../../asset/img/xiezhi_profile.png'
import fundAnalysisIcon from '../../../../plugins/analytix-fund-analysis/assets/icon.png'
import computerUseIcon from '../../../../plugins/analytix-computer-use/assets/icon.png'

type AccountSettingsContext = {
  locale?: string
  showTopNotice?: (notice: { tone: 'success' | 'error' | 'info'; message: string }) => void
}

type HeatmapView = 'daily' | 'weekly' | 'cumulative'

const HEATMAP_COLUMNS = 52
const HEATMAP_ROWS = 7
const HEATMAP_CELLS = HEATMAP_COLUMNS * HEATMAP_ROWS
const DAY_MS = 24 * 60 * 60 * 1000
const USERNAME_PATTERN = /^[a-z0-9][a-z0-9._-]{1,28}[a-z0-9]$/u
const DEFAULT_AVATAR_COLOR = '#22C55E'
const AVATAR_COLORS = ['#22C55E', '#2563EB', '#7C3AED', '#E11D48', '#F97316', '#0F172A'] as const
const MAX_AVATAR_IMAGE_BYTES = 3 * 1024 * 1024
const MAX_AVATAR_IMAGE_PIXELS = 16_000_000
const AVATAR_CANVAS_SIZE = 512
const AVATAR_ALPHA_THRESHOLD = 8
const PHOTO_AVATAR_BACKGROUND = '#F8FAFC'
const AVATAR_IMAGE_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp'])

function pad2(value: number): string {
  return String(value).padStart(2, '0')
}

function localDayKey(date: Date): string {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

function parseDayKey(key: string): Date {
  const [year, month, day] = key.split('-').map((part) => Number.parseInt(part, 10))
  return new Date(year, month - 1, day)
}

function shiftDayKey(key: string, offset: number): string {
  const date = parseDayKey(key)
  date.setDate(date.getDate() + offset)
  return localDayKey(date)
}

function formatCompactNumber(value: number | null | undefined): string {
  if (value == null) return '官网未同步'
  if (!Number.isFinite(value) || value <= 0) return '0'
  return new Intl.NumberFormat('zh-CN', {
    notation: value >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: value >= 10_000 ? 1 : 0
  }).format(value)
}

function formatDuration(ms: number | null | undefined): string {
  if (ms == null) return '官网未同步'
  if (!Number.isFinite(ms) || ms <= 0) return '0 分'
  const totalMinutes = Math.max(1, Math.round(ms / 60_000))
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  if (hours <= 0) return `${minutes} 分`
  if (minutes <= 0) return `${hours} 小时`
  return `${hours} 小时 ${minutes} 分`
}

function normalizedLocale(locale?: string): string {
  return locale === 'en' ? 'en-US' : 'zh-CN'
}

function normalizeUsername(value: string): string {
  return value.trim().replace(/^@+/u, '').toLowerCase()
}

function accountHandle(snapshot: HubAccountSnapshot | null): string {
  const username = normalizeUsername(snapshot?.user?.username ?? '')
  if (username) return `@${username}`
  const email = snapshot?.user?.email?.trim() || ''
  if (email) return `@${email.split('@')[0]}`
  const name = displayAccountName(snapshot)
  return `@${name.replace(/\s+/g, '').toLowerCase() || 'analytix'}`
}

function normalizeAvatarColorValue(value: string | undefined): string {
  const color = value?.trim()
  return color && /^#[0-9a-f]{6}$/iu.test(color) ? color.toUpperCase() : DEFAULT_AVATAR_COLOR
}

function avatarColor(snapshot: HubAccountSnapshot | null): string {
  return normalizeAvatarColorValue(snapshot?.user?.avatarColor)
}

function avatarUrl(snapshot: HubAccountSnapshot | null): string {
  return snapshot?.user?.avatarUrl?.trim() ?? ''
}

function supportedAvatarImageType(file: File): boolean {
  const type = file.type.trim().toLowerCase()
  if (AVATAR_IMAGE_TYPES.has(type)) return true
  return /\.(?:jpe?g|png|webp)$/iu.test(file.name)
}

function loadAvatarImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const objectUrl = URL.createObjectURL(file)
    const image = new Image()
    image.onload = () => {
      URL.revokeObjectURL(objectUrl)
      resolve(image)
    }
    image.onerror = () => {
      URL.revokeObjectURL(objectUrl)
      reject(new Error('头像照片解码失败，请重新选择。'))
    }
    image.src = objectUrl
  })
}

function canvasContext(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) throw new Error('当前环境无法处理头像照片。')
  return context
}

type AvatarVisibleBounds = {
  left: number
  top: number
  right: number
  bottom: number
  visiblePixels: number
}

function avatarVisibleBounds(imageData: ImageData): AvatarVisibleBounds | null {
  const { data, width, height } = imageData
  let left = width
  let top = height
  let right = 0
  let bottom = 0
  let visiblePixels = 0

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const alpha = data[(y * width + x) * 4 + 3]
      if (alpha <= AVATAR_ALPHA_THRESHOLD) continue
      visiblePixels += 1
      left = Math.min(left, x)
      top = Math.min(top, y)
      right = Math.max(right, x + 1)
      bottom = Math.max(bottom, y + 1)
    }
  }

  if (visiblePixels <= 0) return null
  return { left, top, right, bottom, visiblePixels }
}

function normalizeAvatarImageFile(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    void (async () => {
      if (!supportedAvatarImageType(file)) {
        throw new Error('头像照片仅支持 PNG、JPG 或 WebP。')
      }
      if (file.size > MAX_AVATAR_IMAGE_BYTES) {
        throw new Error('头像照片不能超过 3MB。')
      }

      const image = await loadAvatarImage(file)
      const sourceWidth = image.naturalWidth || image.width
      const sourceHeight = image.naturalHeight || image.height
      if (sourceWidth <= 0 || sourceHeight <= 0) {
        throw new Error('头像照片尺寸无效，请重新选择。')
      }
      if (sourceWidth * sourceHeight > MAX_AVATAR_IMAGE_PIXELS) {
        throw new Error('头像照片分辨率过高，请选择 1600 万像素以内的图片。')
      }

      const sourceCanvas = document.createElement('canvas')
      sourceCanvas.width = sourceWidth
      sourceCanvas.height = sourceHeight
      const sourceContext = canvasContext(sourceCanvas)
      sourceContext.drawImage(image, 0, 0, sourceWidth, sourceHeight)

      const imageData = sourceContext.getImageData(0, 0, sourceWidth, sourceHeight)
      const visibleBounds = avatarVisibleBounds(imageData)
      if (!visibleBounds) {
        throw new Error('头像照片没有可见内容，请重新选择。')
      }

      const targetCanvas = document.createElement('canvas')
      targetCanvas.width = AVATAR_CANVAS_SIZE
      targetCanvas.height = AVATAR_CANVAS_SIZE
      const targetContext = canvasContext(targetCanvas)
      targetContext.fillStyle = PHOTO_AVATAR_BACKGROUND
      targetContext.fillRect(0, 0, AVATAR_CANVAS_SIZE, AVATAR_CANVAS_SIZE)

      const totalPixels = sourceWidth * sourceHeight
      const boundsWidth = visibleBounds.right - visibleBounds.left
      const boundsHeight = visibleBounds.bottom - visibleBounds.top
      const sparseOrTransparent =
        visibleBounds.visiblePixels < totalPixels * 0.98 ||
        boundsWidth * boundsHeight < totalPixels * 0.92

      if (sparseOrTransparent) {
        const padding = Math.round(Math.max(boundsWidth, boundsHeight) * 0.12)
        const sx = Math.max(0, visibleBounds.left - padding)
        const sy = Math.max(0, visibleBounds.top - padding)
        const ex = Math.min(sourceWidth, visibleBounds.right + padding)
        const ey = Math.min(sourceHeight, visibleBounds.bottom + padding)
        const sw = Math.max(1, ex - sx)
        const sh = Math.max(1, ey - sy)
        const scale = Math.min(AVATAR_CANVAS_SIZE / sw, AVATAR_CANVAS_SIZE / sh)
        const dw = Math.max(1, Math.round(sw * scale))
        const dh = Math.max(1, Math.round(sh * scale))
        targetContext.drawImage(
          sourceCanvas,
          sx,
          sy,
          sw,
          sh,
          Math.round((AVATAR_CANVAS_SIZE - dw) / 2),
          Math.round((AVATAR_CANVAS_SIZE - dh) / 2),
          dw,
          dh
        )
      } else {
        const side = Math.min(sourceWidth, sourceHeight)
        targetContext.drawImage(
          sourceCanvas,
          Math.round((sourceWidth - side) / 2),
          Math.round((sourceHeight - side) / 2),
          side,
          side,
          0,
          0,
          AVATAR_CANVAS_SIZE,
          AVATAR_CANVAS_SIZE
        )
      }

      resolve(targetCanvas.toDataURL('image/png'))
    })().catch((error) => reject(error instanceof Error ? error : new Error('头像照片处理失败，请重新选择。')))
  })
}

function pluginKey(value: string): string {
  const raw = value.trim()
  if (!raw) return ''
  const pluginUri = raw.match(/^plugin:\/\/([^@\s)\]]+)/iu)?.[1]
  const base = pluginUri || raw.replace(/_/gu, '-')
  if (base.startsWith('@') || base.startsWith('$')) return base.toLowerCase()
  return `@${base.toLowerCase()}`
}

type PluginDisplayMeta = {
  displayName: string
  secondaryName?: string
  iconUrl?: string
  icon?: LucideIcon
  accentClassName: string
}

const PLUGIN_DISPLAY_META: Record<string, PluginDisplayMeta> = {
  '@analytix-fund-analysis': {
    displayName: 'Analytix 涉案资金研判',
    secondaryName: '@analytix-fund-analysis',
    iconUrl: fundAnalysisIcon,
    accentClassName: 'bg-blue-50 text-blue-600'
  },
  '@analytix-funds': {
    displayName: 'Analytix 涉案资金研判',
    secondaryName: '@analytix-fund-analysis',
    iconUrl: fundAnalysisIcon,
    accentClassName: 'bg-blue-50 text-blue-600'
  },
  '@analytix-computer-use': {
    displayName: 'Analytix Computer Use',
    secondaryName: '@analytix-computer-use',
    iconUrl: computerUseIcon,
    accentClassName: 'bg-indigo-50 text-indigo-600'
  },
  '@computer-use': {
    displayName: 'Analytix Computer Use',
    secondaryName: '@analytix-computer-use',
    iconUrl: computerUseIcon,
    accentClassName: 'bg-indigo-50 text-indigo-600'
  },
  '@memory': {
    displayName: 'Memory',
    secondaryName: '@memory',
    icon: Brain,
    accentClassName: 'bg-emerald-50 text-emerald-600'
  },
  '@playwright': {
    displayName: 'Playwright',
    secondaryName: '@playwright',
    icon: MousePointer2,
    accentClassName: 'bg-sky-50 text-sky-600'
  },
  '@documents': {
    displayName: 'Documents',
    secondaryName: '@documents',
    icon: FileText,
    accentClassName: 'bg-orange-50 text-orange-600'
  },
  '@product-design': {
    displayName: 'Product Design',
    secondaryName: '@product-design',
    icon: Sparkles,
    accentClassName: 'bg-fuchsia-50 text-fuchsia-600'
  },
  '$product-design': {
    displayName: 'Product Design',
    secondaryName: '@product-design',
    icon: Sparkles,
    accentClassName: 'bg-fuchsia-50 text-fuchsia-600'
  }
}

function titleCasePluginName(value: string): string {
  return value
    .replace(/^[@$]/u, '')
    .split('-')
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1))
    .join(' ')
}

function pluginDisplayMeta(value: string): PluginDisplayMeta {
  const key = pluginKey(value)
  const known = PLUGIN_DISPLAY_META[key]
  if (known) return known
  return {
    displayName: titleCasePluginName(key || value) || value,
    secondaryName: key || value,
    icon: Box,
    accentClassName: 'bg-ds-hover text-ds-muted'
  }
}

function monthLabel(date: Date, locale?: string): string {
  return new Intl.DateTimeFormat(normalizedLocale(locale), { month: 'short' }).format(date)
}

function heatmapLevel(value: number, max: number): number {
  if (value <= 0 || max <= 0) return 0
  if (value >= max * 0.78) return 4
  if (value >= max * 0.48) return 3
  if (value >= max * 0.2) return 2
  return 1
}

function heatmapCellClass(level: number): string {
  if (level >= 4) return 'bg-sky-500'
  if (level === 3) return 'bg-sky-300'
  if (level === 2) return 'bg-sky-200 dark:bg-sky-500/45'
  if (level === 1) return 'bg-sky-100 dark:bg-sky-500/25'
  return 'bg-ds-hover/70'
}

function buildDailyValues(profile: HubGatewayProfile | undefined): Map<string, number> {
  const values = new Map<string, number>()
  for (const item of profile?.dailyUsage ?? []) {
    if (!/^\d{4}-\d{2}-\d{2}$/u.test(item.date)) continue
    values.set(item.date, Math.max(0, Number(item.tokens || 0)))
  }
  return values
}

function buildHeatmapCells(values: Map<string, number>, todayKey: string, view: HeatmapView): Array<{ key: string; level: number; value: number }> {
  const startKey = shiftDayKey(todayKey, -(HEATMAP_CELLS - 1))
  const days = Array.from({ length: HEATMAP_CELLS }, (_, index) => {
    const key = shiftDayKey(startKey, index)
    return { key, value: values.get(key) ?? 0 }
  })

  if (view === 'daily') {
    const max = Math.max(0, ...days.map((item) => item.value))
    return days.map((item) => ({ ...item, level: heatmapLevel(item.value, max) }))
  }

  const weeklyValues = Array.from({ length: HEATMAP_COLUMNS }, (_, column) => {
    const week = days.slice(column * HEATMAP_ROWS, column * HEATMAP_ROWS + HEATMAP_ROWS)
    return week.reduce((sum, item) => sum + item.value, 0)
  })
  const cumulativeValues = weeklyValues.reduce<number[]>((list, value, index) => {
    list.push((list[index - 1] ?? 0) + value)
    return list
  }, [])
  const source = view === 'weekly' ? weeklyValues : cumulativeValues
  const max = Math.max(0, ...source)

  return days.map((item, index) => {
    const weekValue = source[Math.floor(index / HEATMAP_ROWS)] ?? 0
    return { ...item, value: weekValue, level: heatmapLevel(weekValue, max) }
  })
}

function monthLabels(todayKey: string, locale?: string): string[] {
  const end = parseDayKey(todayKey)
  const start = new Date(end.getTime() - (HEATMAP_CELLS - 1) * DAY_MS)
  const labels: string[] = []
  const cursor = new Date(start.getFullYear(), start.getMonth(), 1)
  while (cursor <= end) {
    labels.push(monthLabel(cursor, locale))
    cursor.setMonth(cursor.getMonth() + 1)
  }
  return labels.slice(-12)
}

function stableProfileHash(value: string): string {
  let hash = 2166136261
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }
  return (hash >>> 0).toString(36)
}

function addInvocation(counts: Map<string, number>, name: string | undefined, count = 1): void {
  const label = String(name ?? '').trim().slice(0, 160)
  if (!label || count <= 0) return
  counts.set(label, (counts.get(label) ?? 0) + count)
}

function invocationList(counts: Map<string, number>): HubProfileInvocation[] {
  return [...counts.entries()]
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
    .slice(0, 40)
    .map(([name, count]) => ({ name, count }))
}

function pluginNameFromProfileToolName(toolName: string): string {
  const normalized = toolName.trim()
  if (!normalized.startsWith('mcp__')) return ''
  if (normalized.startsWith('mcp__analytix_funds__')) return '@analytix-fund-analysis'
  if (normalized.startsWith('mcp__analytix-computer-use__')) return '@analytix-computer-use'
  const match = normalized.match(/^mcp__(.+?)__/u)
  const serverName = (match?.[1] ?? '').trim().replace(/_/gu, '-')
  if (!serverName) return ''
  if (serverName === 'analytix-funds') return '@analytix-fund-analysis'
  return `@${serverName}`
}

function extractProfileSkillNames(block: ChatBlock): string[] {
  const names: string[] = []
  if (block.kind === 'user') {
    for (const skillId of block.meta?.activeSkillIds ?? []) {
      names.push(skillId)
    }
    const pluginMatches = block.text.matchAll(/plugin:\/\/([^\s)\]]+)/giu)
    for (const match of pluginMatches) {
      if (match[1]) names.push(`@${match[1].split('@')[0]}`)
    }
    const skillMatches = block.text.matchAll(/\/skill:([A-Za-z0-9._-]+)/gu)
    for (const match of skillMatches) {
      if (match[1]) names.push(match[1])
    }
  }
  if (block.kind === 'tool') {
    const meta = block.meta ?? {}
    const child = meta.child && typeof meta.child === 'object' ? meta.child as Record<string, unknown> : {}
    const childName = typeof child.childName === 'string' ? child.childName : ''
    const childLabel = typeof child.childLabel === 'string' ? child.childLabel : ''
    if (childName) names.push(childName)
    if (childLabel) names.push(childLabel)
    const pluginName = pluginNameFromProfileToolName(toolNameForProfile(block))
    if (pluginName) names.push(pluginName)
  }
  return names
}

function toolNameForProfile(block: ChatBlock): string {
  if (block.kind !== 'tool') return ''
  const meta = block.meta ?? {}
  const child = meta.child && typeof meta.child === 'object' ? meta.child as Record<string, unknown> : {}
  const metaName = typeof meta.toolName === 'string' ? meta.toolName : ''
  const childName = typeof child.childName === 'string' ? child.childName : ''
  return metaName || childName || block.toolKind || block.summary
}

function buildProfileSyncEvents(params: {
  threads: NormalizedThread[]
  activeThreadId: string | null
  blocks: ChatBlock[]
  lastTurnUsage: { threadId: string; snapshot: ThreadUsageSnapshot } | null
  turnDurationByUserId: Record<string, number>
}): HubProfileEventSyncItem[] {
  const now = new Date().toISOString()
  const events: HubProfileEventSyncItem[] = []

  if (!params.activeThreadId || params.blocks.length === 0) {
    return events
  }

  const active = params.threads.find((thread) => thread.id === params.activeThreadId)
  const toolCounts = new Map<string, number>()
  const skillCounts = new Map<string, number>()
  let occurredAt = active?.updatedAt || now
  let durationMs = 0

  for (const block of params.blocks) {
    if (block.createdAt && block.createdAt > occurredAt) occurredAt = block.createdAt
    if (block.kind === 'tool') addInvocation(toolCounts, toolNameForProfile(block))
    for (const skillName of extractProfileSkillNames(block)) {
      addInvocation(skillCounts, skillName)
    }
    if (block.kind === 'user') {
      durationMs = Math.max(durationMs, Math.max(0, Number(params.turnDurationByUserId[block.id] || 0)))
    }
  }

  const threadHash = stableProfileHash(params.activeThreadId)
  events.push({
    eventKey: `thread-detail:${threadHash}`,
    source: 'desktop',
    eventKind: 'thread_detail',
    occurredAt,
    threadIdHash: threadHash,
    mode: active?.mode || 'agent',
    reasoningEffort: params.lastTurnUsage?.threadId === params.activeThreadId
      ? params.lastTurnUsage.snapshot.effort || 'auto'
      : 'auto',
    durationMs,
    toolInvocations: invocationList(toolCounts),
    skillInvocations: invocationList(skillCounts)
  })

  return events
}

function profileSyncKey(events: HubProfileEventSyncItem[]): string {
  return events
    .map((event) => [
      event.eventKey,
      event.occurredAt,
      event.mode,
      event.reasoningEffort,
      event.durationMs,
      event.toolInvocations?.map((item) => `${item.name}:${item.count}`).join(',') ?? '',
      event.skillInvocations?.map((item) => `${item.name}:${item.count}`).join(',') ?? ''
    ].join('|'))
    .join('\n')
}

function StatCell({ label, value }: { label: string; value: string }): ReactElement {
  return (
    <div className="flex min-w-0 flex-1 flex-col items-center justify-center overflow-hidden px-3 py-2.5 text-center">
      <div className="max-w-full truncate text-[16px] font-medium leading-6 text-ds-ink">{value}</div>
      <div className="mt-0.5 max-w-full truncate text-[13px] font-medium leading-5 text-ds-muted">{label}</div>
    </div>
  )
}

function ProfileAction({
  children,
  onClick,
  staticLabel = false
}: {
  children: ReactElement | string | Array<ReactElement | string>
  onClick?: () => void
  staticLabel?: boolean
}): ReactElement {
  if (staticLabel) {
    return (
      <span className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[13px] font-semibold text-ds-muted">
        {children}
      </span>
    )
  }
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[13px] font-semibold text-ds-ink transition hover:bg-ds-hover"
    >
      {children}
    </button>
  )
}

function PluginIcon({ meta }: { meta: PluginDisplayMeta }): ReactElement {
  if (meta.iconUrl) {
    return (
      <span className="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-ds-border bg-ds-card shadow-sm">
        <img src={meta.iconUrl} alt="" className="h-full w-full object-cover" draggable={false} />
      </span>
    )
  }

  const Icon = meta.icon ?? Box
  return (
    <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-xl border border-ds-border shadow-sm ${meta.accentClassName}`}>
      <Icon className="h-[18px] w-[18px]" strokeWidth={1.9} />
    </span>
  )
}

type ProfileDraft = {
  displayName: string
  username: string
  avatarColor: string
  avatarStyle: HubAvatarStyle
  avatarUrl: string
  avatarImageDataUrl: string
}

function profileDraftFromSnapshot(snapshot: HubAccountSnapshot | null): ProfileDraft {
  return {
    displayName: displayAccountName(snapshot),
    username: accountHandle(snapshot).replace(/^@/u, ''),
    avatarColor: avatarColor(snapshot),
    avatarStyle: snapshot?.user?.avatarStyle ?? 'initials',
    avatarUrl: avatarUrl(snapshot),
    avatarImageDataUrl: ''
  }
}

function initialsFromText(value: string): string {
  const source = value.trim() || 'Analytix'
  const ascii = source.match(/[A-Za-z0-9]/gu)?.join('').slice(0, 2)
  if (ascii) return ascii.toUpperCase()
  return source.slice(0, 2).toUpperCase()
}

export function AccountProfileSettingsSection({ ctx }: { ctx: AccountSettingsContext }): ReactElement {
  const [activityView, setActivityView] = useState<HeatmapView>('daily')
  const [isEditProfileOpen, setIsEditProfileOpen] = useState(false)
  const [profileDraft, setProfileDraft] = useState<ProfileDraft>(() => profileDraftFromSnapshot(null))
  const [profileSaveError, setProfileSaveError] = useState('')
  const [isSavingProfile, setIsSavingProfile] = useState(false)
  const [failedAvatarUrls, setFailedAvatarUrls] = useState<string[]>([])
  const avatarRetryTimers = useRef<Map<string, number>>(new Map())
  const avatarFileInputRef = useRef<HTMLInputElement | null>(null)
  const lastProfileSyncKey = useRef('')
  const localProfileSyncAccountKey = useRef('')
  const snapshot = useHubAccountStore((state) => state.snapshot)
  const usage = useHubAccountStore((state) => state.usage)
  const referral = useHubAccountStore((state) => state.referral)
  const refresh = useHubAccountStore((state) => state.refresh)
  const updateProfile = useHubAccountStore((state) => state.updateProfile)
  const syncProfileEvents = useHubAccountStore((state) => state.syncProfileEvents)
  const syncLocalProfileEvents = useHubAccountStore((state) => state.syncLocalProfileEvents)
  const loadUsage = useHubAccountStore((state) => state.loadUsage)
  const loadReferral = useHubAccountStore((state) => state.loadReferral)
  const threads = useChatStore((state) => state.threads)
  const activeThreadId = useChatStore((state) => state.activeThreadId)
  const blocks = useChatStore((state) => state.blocks)
  const lastTurnUsage = useChatStore((state) => state.lastTurnUsage)
  const turnDurationByUserId = useChatStore((state) => state.turnDurationByUserId)

  const todayKey = localDayKey(new Date())
  const profileEvents = useMemo(
    () => buildProfileSyncEvents({ threads, activeThreadId, blocks, lastTurnUsage, turnDurationByUserId }),
    [activeThreadId, blocks, lastTurnUsage, threads, turnDurationByUserId]
  )
  const profileEventsKey = useMemo(() => profileSyncKey(profileEvents), [profileEvents])

  useEffect(() => {
    if (snapshot) return
    void refresh().catch(() => undefined)
  }, [refresh, snapshot])

  useEffect(() => () => {
    for (const timer of avatarRetryTimers.current.values()) {
      window.clearTimeout(timer)
    }
    avatarRetryTimers.current.clear()
  }, [])

  useEffect(() => {
    if (!snapshot?.authenticated) return
    const syncCurrentProfileEvents = async (): Promise<void> => {
      if (profileEvents.length > 0 && lastProfileSyncKey.current !== profileEventsKey) {
        lastProfileSyncKey.current = profileEventsKey
        await syncProfileEvents({ events: profileEvents })
      }
      if (!usage) await loadUsage()
    }

    const accountKey = snapshot.user?.id || snapshot.user?.email || 'authenticated'
    if (localProfileSyncAccountKey.current !== accountKey) {
      localProfileSyncAccountKey.current = accountKey
      void syncLocalProfileEvents()
        .then(() => syncCurrentProfileEvents())
        .then(() => loadUsage())
        .catch(() => loadUsage().catch(() => undefined))
    } else {
      void syncCurrentProfileEvents().catch(() => loadUsage().catch(() => undefined))
    }
    if (!referral) void loadReferral().catch(() => undefined)
  }, [
    loadReferral,
    loadUsage,
    profileEvents,
    profileEventsKey,
    referral,
    snapshot?.authenticated,
    snapshot?.user?.email,
    snapshot?.user?.id,
    syncLocalProfileEvents,
    syncProfileEvents,
    usage
  ])

  const profile = usage?.gatewayProfile
  const profileSummary = profile?.summary
  const profileInsights = profile?.activityInsights
  const dailyValues = useMemo(() => buildDailyValues(profile), [profile])
  const heatmapCells = useMemo(
    () => buildHeatmapCells(dailyValues, todayKey, activityView),
    [activityView, dailyValues, todayKey]
  )
  const months = useMemo(() => monthLabels(todayKey, ctx.locale), [ctx.locale, todayKey])
  const topTools = profile?.topInvocations ?? []

  const plan = snapshot?.entitlement?.plan ?? snapshot?.user?.plan ?? 'normal'
  const edition = productEditionLabel(snapshot)
  const name = displayAccountName(snapshot)
  const handle = accountHandle(snapshot)
  const currentAvatarColor = avatarColor(snapshot)
  const currentAvatarUrl = avatarDisplayUrl(avatarUrl(snapshot))
  const showCurrentPhotoAvatar = shouldShowHubPhotoAvatar(snapshot?.user?.avatarStyle, currentAvatarUrl) && !failedAvatarUrls.includes(currentAvatarUrl)
  const draftAvatarUrl = avatarDisplayUrl(profileDraft.avatarImageDataUrl || profileDraft.avatarUrl)
  const showDraftPhotoAvatar = shouldShowHubPhotoAvatar(profileDraft.avatarStyle, draftAvatarUrl) && !failedAvatarUrls.includes(draftAvatarUrl)
  const totalTokens = profileSummary?.totalTextTokens
  const peakTokens = profileSummary?.peakTokens
  const longestRequestMs = profileSummary?.longestRequestDurationMs
  const quickModeLabel = profileInsights?.fastModeUsagePercentage == null
    ? '官网未同步'
    : `${Math.round(profileInsights.fastModeUsagePercentage)}%`
  const effortLabel = profileInsights?.mostUsedReasoningEffort
    ? `${profileInsights.mostUsedReasoningEffort}${
        profileInsights.mostUsedReasoningEffortPercentage == null
          ? ''
          : ` · ${Math.round(profileInsights.mostUsedReasoningEffortPercentage)}%`
      }`
    : '官网未同步'
  const uniqueSkillsLabel = profileInsights?.uniqueSkillsUsed == null ? '官网未同步' : String(profileInsights.uniqueSkillsUsed)
  const totalSkillsLabel = profileInsights?.totalSkillsUsed == null ? '官网未同步' : String(profileInsights.totalSkillsUsed)

  const shareProfile = async (): Promise<void> => {
    const shareUrl = referral?.referral.shareUrl || 'https://analytix.top/user-console'
    await navigator.clipboard?.writeText(shareUrl).catch(() => undefined)
    ctx.showTopNotice?.({ tone: 'success', message: '个人账户链接已复制。' })
  }

  const openEditProfile = (): void => {
    setProfileDraft(profileDraftFromSnapshot(snapshot))
    setProfileSaveError('')
    setIsEditProfileOpen(true)
  }

  const closeEditProfile = (): void => {
    if (isSavingProfile) return
    setIsEditProfileOpen(false)
    setProfileSaveError('')
  }

  const openAvatarFilePicker = (): void => {
    avatarFileInputRef.current?.click()
  }

  const markAvatarUrlFailed = (url: string): void => {
    if (!url) return
    setFailedAvatarUrls((current) => current.includes(url) ? current : [...current.slice(-8), url])
    if (avatarRetryTimers.current.has(url)) return
    const timer = window.setTimeout(() => {
      avatarRetryTimers.current.delete(url)
      setFailedAvatarUrls((current) => current.filter((item) => item !== url))
    }, 1800)
    avatarRetryTimers.current.set(url, timer)
  }

  const clearAvatarUrlFailure = (url: string): void => {
    if (!url) return
    const timer = avatarRetryTimers.current.get(url)
    if (timer !== undefined) {
      window.clearTimeout(timer)
      avatarRetryTimers.current.delete(url)
    }
    setFailedAvatarUrls((current) => current.filter((item) => item !== url))
  }

  const onAvatarFileChange = async (event: ChangeEvent<HTMLInputElement>): Promise<void> => {
    const file = event.currentTarget.files?.[0]
    event.currentTarget.value = ''
    if (!file) return

    setProfileSaveError('')
    try {
      const dataUrl = await normalizeAvatarImageFile(file)
      setProfileDraft((current) => ({
        ...current,
        avatarStyle: 'photo',
        avatarImageDataUrl: dataUrl
      }))
    } catch (error) {
      setProfileSaveError(error instanceof Error ? error.message : '头像照片读取失败，请重新选择。')
    }
  }

  const saveProfile = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault()
    const displayName = profileDraft.displayName.trim().replace(/\s+/gu, ' ')
    const username = normalizeUsername(profileDraft.username)
    const nextAvatarColor = normalizeAvatarColorValue(profileDraft.avatarColor)
    const nextAvatarStyle = profileDraft.avatarStyle === 'photo' || profileDraft.avatarStyle === 'xiezhi'
      ? profileDraft.avatarStyle
      : 'initials'

    if (displayName.length < 1 || displayName.length > 40) {
      setProfileSaveError('显示名称需为 1 到 40 个字符。')
      return
    }

    if (!USERNAME_PATTERN.test(username)) {
      setProfileSaveError('用户名需为 3 到 30 位小写字母、数字、点、短横线或下划线，且首尾必须是字母或数字。')
      return
    }

    setIsSavingProfile(true)
    setProfileSaveError('')
    try {
      const result = await updateProfile({
        displayName,
        username,
        avatarColor: nextAvatarColor,
        avatarStyle: nextAvatarStyle,
        avatarImageDataUrl: profileDraft.avatarImageDataUrl || undefined
      })
      if (!result) {
        setProfileSaveError('个人资料保存失败，请稍后重试。')
        return
      }
      clearAvatarUrlFailure(avatarDisplayUrl(result.user.avatarUrl))
      setIsEditProfileOpen(false)
      ctx.showTopNotice?.({ tone: 'success', message: '个人资料已同步。' })
    } catch (error) {
      setProfileSaveError(error instanceof Error ? error.message : '个人资料保存失败，请稍后重试。')
    } finally {
      setIsSavingProfile(false)
    }
  }

  return (
    <section className="flex min-w-0 flex-col gap-10 pb-8">
      <div className="flex items-center justify-between gap-4 text-[14px] font-semibold text-ds-ink">
        <h1 className="font-semibold">个人资料</h1>
        <div className="flex items-center gap-2">
          <ProfileAction onClick={() => void shareProfile()}>
            <Share2 className="h-4 w-4" strokeWidth={1.9} />
            分享
          </ProfileAction>
          <ProfileAction staticLabel>
            <LockKeyhole className="h-4 w-4" strokeWidth={1.9} />
            私有
          </ProfileAction>
          <ProfileAction onClick={openEditProfile}>
            <PencilLine className="h-4 w-4" strokeWidth={1.9} />
            编辑
          </ProfileAction>
        </div>
      </div>

      <div className="flex flex-col items-center pt-12 text-center">
        <div className="relative mb-4 h-20 w-20">
          <div
            className="flex h-20 w-20 items-center justify-center overflow-hidden rounded-full text-[27px] font-medium text-white shadow-sm"
            style={{ backgroundColor: showCurrentPhotoAvatar ? PHOTO_AVATAR_BACKGROUND : currentAvatarColor }}
          >
            {showCurrentPhotoAvatar ? (
              <img
                src={currentAvatarUrl}
                alt=""
                className="h-full w-full object-cover"
                draggable={false}
                onError={() => markAvatarUrlFailed(currentAvatarUrl)}
              />
            ) : (
              accountInitials(snapshot)
            )}
          </div>
          <img
            src={xiezhiFigure}
            alt=""
            className="pointer-events-none absolute -bottom-1 -right-7 h-12 w-12 object-contain drop-shadow-sm"
            draggable={false}
          />
        </div>
        <h2 className="text-[24px] font-normal leading-8 text-ds-ink">{name}</h2>
        <div className="mt-1 flex min-h-7 items-center justify-center gap-1.5 text-[15px] leading-5 text-ds-muted">
          <span>{handle}</span>
          <span aria-hidden="true">·</span>
          <span className="rounded-full border border-ds-border px-2 py-0.5 text-[13px] leading-5 text-ds-muted">
            {edition === '普通版' ? 'Standard' : 'Pro'}
          </span>
        </div>
      </div>

      <div className="flex flex-col items-center justify-center overflow-hidden rounded-2xl border border-ds-border bg-transparent">
        <div className="flex w-full items-stretch">
          <StatCell label="累计 Token 数" value={formatCompactNumber(totalTokens)} />
          <div className="my-3 w-px shrink-0 self-stretch rounded-sm bg-ds-border-muted" />
          <StatCell label="峰值 Token 数" value={formatCompactNumber(peakTokens)} />
          <div className="my-3 w-px shrink-0 self-stretch rounded-sm bg-ds-border-muted" />
          <StatCell label="最长任务时长" value={formatDuration(longestRequestMs)} />
          <div className="my-3 w-px shrink-0 self-stretch rounded-sm bg-ds-border-muted" />
          <StatCell label="当前连续天数" value={profileSummary ? `${profileSummary.currentStreakDays} 天` : '官网未同步'} />
          <div className="my-3 w-px shrink-0 self-stretch rounded-sm bg-ds-border-muted" />
          <StatCell label="最长连续天数" value={profileSummary ? `${profileSummary.longestStreakDays} 天` : '官网未同步'} />
        </div>
      </div>

      <section className="flex min-w-0 flex-col gap-3">
        <div className="flex items-center justify-between gap-4">
          <h2 className="text-[16px] font-semibold text-ds-ink">Token 活动</h2>
          <div className="flex items-center gap-4 text-[14px] font-semibold">
            {(['daily', 'weekly', 'cumulative'] as const).map((view) => (
              <button
                key={view}
                type="button"
                onClick={() => setActivityView(view)}
                className={`transition hover:text-ds-ink ${
                  activityView === view ? 'text-ds-ink' : 'text-ds-faint'
                }`}
              >
                {view === 'daily' ? '每日' : view === 'weekly' ? '每周' : '累计'}
              </button>
            ))}
          </div>
        </div>
        <div
          className="grid grid-flow-col grid-rows-[repeat(7,minmax(1px,1fr))] gap-[3px] overflow-hidden"
          style={{ gridTemplateColumns: `repeat(${HEATMAP_COLUMNS}, minmax(1px, 1fr))` }}
          aria-label="Token activity heatmap"
        >
          {heatmapCells.map((cell) => (
            <span key={cell.key} title={`${cell.key}: ${formatCompactNumber(cell.value)}`} className="aspect-square w-full">
              <span className={`block h-full w-full rounded-[4px] transition-colors duration-500 ${heatmapCellClass(cell.level)}`} />
            </span>
          ))}
        </div>
        <div className="flex justify-between text-[12px] font-medium text-ds-faint">
          {months.map((label, index) => (
            <span key={`${label}-${index}`}>{label}</span>
          ))}
        </div>
      </section>

      <section className="grid gap-10 md:grid-cols-2">
        <div>
          <h2 className="mb-4 text-[16px] font-semibold text-ds-ink">活动洞察</h2>
          <dl className="flex flex-col gap-2 text-[14px] leading-6">
            <div className="flex items-center justify-between gap-4">
              <dt className="text-ds-muted">快速模式</dt>
              <dd className="font-semibold text-ds-ink">{quickModeLabel}</dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-ds-muted">最常用的推理强度</dt>
              <dd className="font-semibold text-ds-ink">{effortLabel}</dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-ds-muted">已探索的技能</dt>
              <dd className="font-semibold text-ds-ink">{uniqueSkillsLabel}</dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-ds-muted">使用的技能总数</dt>
              <dd className="font-semibold text-ds-ink">{totalSkillsLabel}</dd>
            </div>
            <div className="flex items-center justify-between gap-4">
              <dt className="text-ds-muted">请求总数</dt>
              <dd className="font-semibold text-ds-ink">{profileSummary ? profileSummary.totalRequests : '官网未同步'}</dd>
            </div>
          </dl>
        </div>

        <div>
          <h2 className="mb-4 text-[16px] font-semibold text-ds-ink">最常用的插件</h2>
          {topTools.length > 0 ? (
            <div className="flex flex-col gap-3">
              {topTools.map((tool) => {
                const meta = pluginDisplayMeta(tool.name)
                return (
                  <div key={tool.name} className="flex min-w-0 items-center justify-between gap-3 text-[14px] leading-6">
                    <div className="flex min-w-0 items-center gap-3">
                      <PluginIcon meta={meta} />
                      <span className="min-w-0">
                        <span className="block truncate font-medium text-ds-ink">{meta.displayName}</span>
                        {meta.secondaryName && meta.secondaryName !== meta.displayName ? (
                          <span className="block truncate text-[12px] leading-4 text-ds-faint">{meta.secondaryName}</span>
                        ) : null}
                      </span>
                    </div>
                    <span className="shrink-0 text-ds-muted">{tool.count} 次运行</span>
                  </div>
                )
              })}
            </div>
          ) : (
            <div className="rounded-xl border border-dashed border-ds-border-muted px-4 py-8 text-center text-[13px] text-ds-faint">
              {profile?.dataCompleteness.pluginInvocations ? '暂无插件运行记录' : '官网尚未同步插件运行记录'}
            </div>
          )}
        </div>
      </section>

      {isEditProfileOpen ? (
        <div className="fixed inset-0 z-[80] flex items-center justify-center px-5 py-8">
          <button
            type="button"
            className="absolute inset-0 cursor-default bg-black/28 backdrop-blur-[6px]"
            aria-label="关闭编辑个人资料"
            onClick={closeEditProfile}
          />
          <form
            className="relative flex max-h-[92vh] w-full max-w-[680px] flex-col overflow-hidden rounded-[28px] border border-ds-border bg-ds-card text-ds-ink shadow-[0_28px_80px_rgba(15,23,42,0.22)]"
            onSubmit={(event) => void saveProfile(event)}
          >
            <div className="flex items-center justify-between gap-4 px-7 pb-4 pt-7">
              <h2 className="text-[24px] font-semibold leading-8 tracking-normal">编辑个人资料</h2>
              <button
                type="button"
                onClick={closeEditProfile}
                disabled={isSavingProfile}
                className="flex h-9 w-9 items-center justify-center rounded-full border border-ds-border bg-ds-card text-ds-muted transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
                aria-label="关闭"
              >
                <X className="h-5 w-5" strokeWidth={1.9} />
              </button>
            </div>

            <div className="min-h-0 overflow-y-auto px-7 pb-6">
              <div className="flex flex-col items-center pb-8 pt-7">
                <div className="relative">
                  <div
                    className="flex h-36 w-36 items-center justify-center overflow-hidden rounded-full text-[48px] font-medium text-white shadow-sm"
                    style={{ backgroundColor: showDraftPhotoAvatar ? PHOTO_AVATAR_BACKGROUND : profileDraft.avatarColor }}
                  >
                    {showDraftPhotoAvatar ? (
                      <img
                        src={draftAvatarUrl}
                        alt=""
                        className="h-full w-full object-cover"
                        draggable={false}
                        onError={() => markAvatarUrlFailed(draftAvatarUrl)}
                      />
                    ) : (
                      initialsFromText(profileDraft.displayName)
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={openAvatarFilePicker}
                    className="absolute bottom-1 right-1 flex h-11 w-11 items-center justify-center rounded-full border border-white/70 bg-white/80 text-ds-ink shadow-[0_10px_26px_rgba(15,23,42,0.16)] backdrop-blur transition hover:bg-white hover:scale-[1.03] focus:outline-none focus:ring-2 focus:ring-ds-ink/20"
                    aria-label="上传头像照片"
                  >
                    <PencilLine className="h-5 w-5" strokeWidth={1.9} />
                  </button>
                  <input
                    ref={avatarFileInputRef}
                    type="file"
                    accept="image/png,image/jpeg,image/webp"
                    className="sr-only"
                    onChange={(event) => void onAvatarFileChange(event)}
                  />
                </div>
                <div className="mt-5 flex items-center gap-2">
                  <Palette className="h-4 w-4 text-ds-faint" strokeWidth={1.9} />
                  <div className="flex items-center gap-2">
                    {AVATAR_COLORS.map((color) => {
                      const selected = profileDraft.avatarColor.toUpperCase() === color
                      return (
                        <button
                          key={color}
                          type="button"
                          onClick={() => setProfileDraft((current) => ({
                            ...current,
                            avatarColor: color,
                            avatarStyle: 'initials',
                            avatarImageDataUrl: ''
                          }))}
                          className={`relative flex h-8 w-8 items-center justify-center rounded-full border transition ${
                            selected ? 'border-ds-ink shadow-[0_0_0_3px_rgba(15,23,42,0.08)]' : 'border-ds-border hover:border-ds-muted'
                          }`}
                          aria-label={`选择头像颜色 ${color}`}
                        >
                          <span className="h-5 w-5 rounded-full" style={{ backgroundColor: color }} />
                          {selected ? <Check className="absolute h-3.5 w-3.5 text-white" strokeWidth={2.4} /> : null}
                        </button>
                      )
                    })}
                  </div>
                </div>
              </div>

              <div className="overflow-hidden rounded-[22px] border border-ds-border bg-ds-card">
                <label className="grid grid-cols-[150px_minmax(0,1fr)] items-center gap-5 border-b border-ds-border px-6 py-5">
                  <span className="text-[16px] font-semibold text-ds-ink">显示名称</span>
                  <input
                    autoFocus
                    value={profileDraft.displayName}
                    onChange={(event) => setProfileDraft((current) => ({ ...current, displayName: event.target.value }))}
                    className="h-12 min-w-0 rounded-[14px] border border-ds-border bg-ds-card px-4 text-[16px] text-ds-ink outline-none transition placeholder:text-ds-faint focus:border-ds-muted focus:bg-ds-hover/40"
                    maxLength={40}
                    placeholder="显示名称"
                  />
                </label>
                <label className="grid grid-cols-[150px_minmax(0,1fr)] items-center gap-5 px-6 py-5">
                  <span className="text-[16px] font-semibold text-ds-ink">用户名</span>
                  <span className="relative min-w-0">
                    <span className="pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-[16px] text-ds-faint">@</span>
                    <input
                      value={profileDraft.username}
                      onChange={(event) => setProfileDraft((current) => ({ ...current, username: normalizeUsername(event.target.value) }))}
                      className="h-12 w-full min-w-0 rounded-[14px] border border-ds-border bg-ds-card px-4 pl-8 text-[16px] text-ds-ink outline-none transition placeholder:text-ds-faint focus:border-ds-muted focus:bg-ds-hover/40"
                      maxLength={30}
                      placeholder="username"
                    />
                  </span>
                </label>
              </div>

              {profileSaveError ? (
                <div className="mt-4 rounded-[16px] border border-red-200 bg-red-50 px-4 py-3 text-[13px] leading-5 text-red-700">
                  {profileSaveError}
                </div>
              ) : null}
            </div>

            <div className="flex items-center justify-end gap-3 px-7 pb-7 pt-2">
              <button
                type="button"
                onClick={closeEditProfile}
                disabled={isSavingProfile}
                className="rounded-full px-6 py-2.5 text-[15px] font-semibold text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink disabled:cursor-not-allowed disabled:opacity-60"
              >
                取消
              </button>
              <button
                type="submit"
                disabled={isSavingProfile}
                className="rounded-full bg-ds-ink px-7 py-2.5 text-[15px] font-semibold text-ds-card transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {isSavingProfile ? '保存中...' : '保存'}
              </button>
            </div>
          </form>
        </div>
      ) : null}

      <div className="sr-only">当前账号版本：{plan}</div>
    </section>
  )
}
