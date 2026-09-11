import type { MenuItemConstructorOptions } from 'electron'
import type { AppSettingsV1 } from '../shared/app-settings'
import type { ProviderRegistryPublicProvider, ProviderRegistryResult } from '../shared/analytix-api'

export type TrayThreadSummary = {
  id: string
  title: string
  workspace?: string
  status: string
  updatedAt: string
}

export type TrayProviderObservation = {
  providerId: string
  status: 'available' | 'unavailable' | 'auth_failed' | 'timeout' | 'redirect_blocked' | 'provider_error' | 'invalid_response'
  observedAt: string
  expiresAt: string
  quota?: number
  usage?: number
  remaining?: number
}

type ProviderRegistrySnapshot = Extract<ProviderRegistryResult, { providers: unknown }>
type ProviderAccountObservation = Extract<ProviderRegistryResult, { observedAt: unknown }>

export function currentTrayProviderObservation(input: {
  expectedSnapshot: ProviderRegistrySnapshot
  expectedProvider: ProviderRegistryPublicProvider
  observation: ProviderAccountObservation
  currentSnapshot: ProviderRegistrySnapshot
  nowMs?: number
}): TrayProviderObservation | null {
  const { expectedSnapshot, expectedProvider, observation, currentSnapshot } = input
  const currentProvider = currentSnapshot.providers.find((provider) => provider.id === expectedProvider.id)
  const observedAtMs = Date.parse(observation.observedAt)
  const expiresAtMs = Date.parse(observation.expiresAt)
  const nowMs = input.nowMs ?? Date.now()
  if (!currentProvider || currentProvider.tombstone ||
    expectedSnapshot.selectedProviderId !== expectedProvider.id ||
    currentSnapshot.selectedProviderId !== expectedProvider.id ||
    observation.providerId !== expectedProvider.id ||
    observation.registryRevision !== expectedSnapshot.registryRevision ||
    observation.registryIncarnation !== expectedSnapshot.registryIncarnation ||
    observation.providerRevision !== expectedProvider.revision ||
    observation.providerGeneration !== expectedProvider.generation ||
    observation.providerIncarnation !== expectedProvider.incarnation ||
    observation.providerCredentialPurpose !== expectedProvider.credentialPurpose ||
    currentSnapshot.registryRevision !== observation.registryRevision ||
    currentSnapshot.registryIncarnation !== observation.registryIncarnation ||
    currentProvider.revision !== observation.providerRevision ||
    currentProvider.generation !== observation.providerGeneration ||
    currentProvider.incarnation !== observation.providerIncarnation ||
    currentProvider.credentialPurpose !== expectedProvider.credentialPurpose ||
    !currentProvider.credentialConfigured ||
    JSON.stringify(currentProvider.accountObservation ?? null) !==
      JSON.stringify(expectedProvider.accountObservation ?? null) ||
    !Number.isFinite(observedAtMs) || !Number.isFinite(expiresAtMs) || observedAtMs > nowMs || expiresAtMs <= nowMs) {
    return null
  }
  return {
    providerId: observation.providerId,
    status: observation.status,
    observedAt: observation.observedAt,
    expiresAt: observation.expiresAt,
    ...(observation.quota !== undefined ? { quota: observation.quota } : {}),
    ...(observation.usage !== undefined ? { usage: observation.usage } : {}),
    ...(observation.remaining !== undefined ? { remaining: observation.remaining } : {})
  }
}

type TrayMenuActions = {
  openThread: (threadId: string) => void
  newChat: () => void
  openApp: () => void
  quit: () => void
}

const PRIMARY_GROUP_LIMIT = 5
const MORE_GROUP_LIMIT = 10
const MAX_THREAD_LABEL_LENGTH = 48

export function parseTrayThreads(body: string): TrayThreadSummary[] {
  try {
    const value = JSON.parse(body) as { threads?: unknown }
    if (!Array.isArray(value.threads)) return []
    return value.threads.flatMap((candidate) => {
      if (!candidate || typeof candidate !== 'object') return []
      const thread = candidate as Record<string, unknown>
      if (typeof thread.id !== 'string' || typeof thread.updatedAt !== 'string') return []
      const status = typeof thread.status === 'string' ? thread.status : 'idle'
      if (status === 'archived' || status === 'deleted') return []
      return [{
        id: thread.id,
        title: typeof thread.title === 'string' ? thread.title : '',
        workspace: typeof thread.workspace === 'string' ? thread.workspace : undefined,
        status,
        updatedAt: thread.updatedAt
      }]
    }).sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
  } catch {
    return []
  }
}

export function buildTrayMenuTemplate(input: {
  locale: AppSettingsV1['locale']
  threads: TrayThreadSummary[]
  providerObservation?: TrayProviderObservation | null
  actions: TrayMenuActions
}): MenuItemConstructorOptions[] {
  const labels = traySessionLabels(input.locale)
  const running = input.threads.filter((thread) => thread.status === 'running')
  const recent = input.threads.filter((thread) => thread.status !== 'running')
  const template: MenuItemConstructorOptions[] = []

  if (input.providerObservation) {
    template.push({
      label: `${labels.providerQuota}: ${input.providerObservation.providerId}`,
      sublabel: providerObservationLabel(input.providerObservation, labels),
      enabled: false
    })
  }

  appendThreadGroup(template, labels.running, running.slice(0, PRIMARY_GROUP_LIMIT), input.actions, labels)
  appendThreadGroup(template, labels.recent, recent.slice(0, PRIMARY_GROUP_LIMIT), input.actions, labels)

  const overflow = [...running.slice(PRIMARY_GROUP_LIMIT), ...recent.slice(PRIMARY_GROUP_LIMIT)]
    .slice(0, MORE_GROUP_LIMIT)
  if (overflow.length > 0) {
    template.push({
      label: labels.more,
      submenu: overflow.map((thread) => threadMenuItem(thread, input.actions, labels))
    })
  }
  if (template.length > 0) template.push({ type: 'separator' })
  template.push(
    { label: labels.newChat, click: input.actions.newChat },
    { type: 'separator' },
    { label: labels.openApp, click: input.actions.openApp },
    { type: 'separator' },
    { label: labels.quit, click: input.actions.quit }
  )
  return template
}

function providerObservationLabel(
  observation: TrayProviderObservation,
  labels: TraySessionLabels
): string {
  if (observation.status !== 'available') return labels.quotaUnavailable
  if (observation.remaining !== undefined && observation.quota !== undefined) {
    return `${formatObservationValue(observation.remaining)} / ${formatObservationValue(observation.quota)}`
  }
  if (observation.remaining !== undefined) return `${labels.quotaRemaining}: ${formatObservationValue(observation.remaining)}`
  if (observation.usage !== undefined) return `${labels.quotaUsage}: ${formatObservationValue(observation.usage)}`
  return labels.quotaUnavailable
}

function formatObservationValue(value: number): string {
  return Number.isFinite(value) && value >= 0 ? new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(value) : '—'
}

function appendThreadGroup(
  template: MenuItemConstructorOptions[],
  label: string,
  threads: TrayThreadSummary[],
  actions: TrayMenuActions,
  labels: TraySessionLabels
): void {
  if (threads.length === 0) return
  if (template.length > 0) template.push({ type: 'separator' })
  template.push({ label, enabled: false })
  template.push(...threads.map((thread) => threadMenuItem(thread, actions, labels)))
}

function threadMenuItem(
  thread: TrayThreadSummary,
  actions: TrayMenuActions,
  labels: TraySessionLabels
): MenuItemConstructorOptions {
  return {
    label: truncateMenuLabel(thread.title.trim() || labels.untitled),
    sublabel: workspaceLabel(thread.workspace),
    click: () => actions.openThread(thread.id)
  }
}

function workspaceLabel(workspace: string | undefined): string | undefined {
  const value = workspace?.trim().replace(/[\\/]+$/, '')
  if (!value) return undefined
  const segments = value.split(/[\\/]+/)
  return segments[segments.length - 1] || value
}

function truncateMenuLabel(value: string): string {
  if (value.length <= MAX_THREAD_LABEL_LENGTH) return value
  return `${value.slice(0, MAX_THREAD_LABEL_LENGTH - 1)}…`
}

type TraySessionLabels = {
  running: string
  recent: string
  more: string
  newChat: string
  openApp: string
  quit: string
  untitled: string
  providerQuota: string
  quotaUnavailable: string
  quotaRemaining: string
  quotaUsage: string
}

function traySessionLabels(locale: AppSettingsV1['locale']): TraySessionLabels {
  return locale === 'zh'
    ? {
        running: '运行中',
        recent: '最近会话',
        more: '更多',
        newChat: '新建会话',
        openApp: '打开 Analytix',
        quit: '退出',
        untitled: '未命名会话',
        providerQuota: 'Provider 配额',
        quotaUnavailable: '不可用',
        quotaRemaining: '剩余',
        quotaUsage: '已用'
      }
    : {
        running: 'Running',
        recent: 'Recent',
        more: 'More',
        newChat: 'New Chat',
        openApp: 'Open Analytix',
        quit: 'Quit',
        untitled: 'Untitled',
        providerQuota: 'Provider quota',
        quotaUnavailable: 'Unavailable',
        quotaRemaining: 'Remaining',
        quotaUsage: 'Used'
      }
}
