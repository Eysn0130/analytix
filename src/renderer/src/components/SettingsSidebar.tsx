import { useMemo, useState, type Dispatch, type ReactElement, type SetStateAction } from 'react'
import {
  Archive,
  AudioLines,
  Bot,
  BrainCircuit,
  Bug,
  ChevronLeft,
  Code2,
  Globe,
  Globe2,
  ImageIcon,
  Keyboard,
  Mic,
  Monitor,
  PencilLine,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Smartphone,
  Sparkles,
  UserCircle,
  X
} from 'lucide-react'

type SettingsCategory =
  | 'account'
  | 'general'
  | 'providers'
  | 'write'
  | 'imageGeneration'
  | 'mediaGeneration'
  | 'speechToText'
  | 'agents'
  | 'archives'
  | 'permissions'
  | 'browser'
  | 'computerUse'
  | 'worktree'
  | 'memory'
  | 'shortcuts'
  | 'easterEgg'
  | 'claw'
  | 'updates'
  | 'debug'

type SidebarIcon = typeof UserCircle

type NavItem = {
  category: SettingsCategory
  labelKey: string
  icon: SidebarIcon
  keywords: string[]
}

type NavGroup = {
  headingKey: string
  items: NavItem[]
}

const NAV_GROUPS: NavGroup[] = [
  {
    headingKey: 'settingsNavPersonal',
    items: [
      { category: 'account', labelKey: 'deprecatedHubCompatibility', icon: UserCircle, keywords: ['deprecated', 'compatibility', 'account', 'login', 'hub'] },
      { category: 'general', labelKey: 'general', icon: Globe, keywords: ['theme', 'locale', 'workspace', 'startup'] },
      { category: 'write', labelKey: 'write', icon: PencilLine, keywords: ['editor', 'completion', 'typing'] },
      { category: 'imageGeneration', labelKey: 'imageGen', icon: ImageIcon, keywords: ['image', 'generation'] },
      { category: 'mediaGeneration', labelKey: 'mediaGeneration', icon: AudioLines, keywords: ['media', 'video', 'audio'] },
      { category: 'speechToText', labelKey: 'speechToText', icon: Mic, keywords: ['speech', 'transcription', 'voice'] },
      { category: 'easterEgg', labelKey: 'easterEgg', icon: Sparkles, keywords: ['mascot', 'cameo', 'ui plugin'] },
      { category: 'updates', labelKey: 'updates', icon: RefreshCw, keywords: ['version', 'release', 'update'] }
    ]
  },
  {
    headingKey: 'settingsNavIntegrations',
    items: [
      { category: 'providers', labelKey: 'providers', icon: Globe2, keywords: ['model', 'api', 'provider', 'local'] },
      { category: 'browser', labelKey: 'browser', icon: Monitor, keywords: ['browser', 'web', 'chrome', 'permissions'] },
      { category: 'computerUse', labelKey: 'computerUse', icon: Monitor, keywords: ['computer', 'screen', 'mouse', 'keyboard', 'analytix-computer-use'] },
      { category: 'claw', labelKey: 'claw', icon: Smartphone, keywords: ['phone', 'connect', 'im'] }
    ]
  },
  {
    headingKey: 'settingsNavCoding',
    items: [
      { category: 'agents', labelKey: 'agents', icon: Bot, keywords: ['assistant', 'skill', 'mcp', 'runtime'] },
      { category: 'permissions', labelKey: 'permissions', icon: ShieldCheck, keywords: ['approval', 'sandbox', 'access'] },
      { category: 'worktree', labelKey: 'worktree', icon: Code2, keywords: ['git', 'branch', 'worktree'] },
      { category: 'memory', labelKey: 'memory', icon: BrainCircuit, keywords: ['memory', 'remember', 'context'] },
      { category: 'shortcuts', labelKey: 'keyboardShortcuts', icon: Keyboard, keywords: ['keyboard', 'hotkey', 'shortcut'] },
      { category: 'debug', labelKey: 'debug', icon: Bug, keywords: ['logs', 'diagnostics', 'troubleshooting'] }
    ]
  },
  {
    headingKey: 'settingsNavArchived',
    items: [
      { category: 'archives', labelKey: 'archives', icon: Archive, keywords: ['archive', 'history', 'threads'] }
    ]
  }
]

export function SettingsSidebar({
  category,
  goBack,
  setCategory,
  t
}: {
  category: SettingsCategory
  goBack: () => void
  setCategory: Dispatch<SetStateAction<SettingsCategory>>
  t: (key: string) => string
}): ReactElement {
  const [query, setQuery] = useState('')
  const normalizedQuery = query.trim().toLowerCase()

  const visibleGroups = useMemo(() => {
    if (!normalizedQuery) return NAV_GROUPS
    return NAV_GROUPS
      .map((group) => {
        const heading = t(group.headingKey).toLowerCase()
        const items = group.items.filter((item) => {
          const label = t(item.labelKey).toLowerCase()
          return [heading, label, ...item.keywords].some((entry) => entry.toLowerCase().includes(normalizedQuery))
        })
        return { ...group, items }
      })
      .filter((group) => group.items.length > 0)
  }, [normalizedQuery, t])

  const catCls = (c: SettingsCategory): string =>
    `flex h-9 w-full items-center gap-2.5 rounded-lg px-2.5 text-left text-[13px] font-medium transition ${
      category === c
        ? 'bg-ds-subtle text-ds-ink shadow-sm ring-1 ring-ds-border-muted'
        : 'text-ds-muted hover:bg-ds-hover hover:text-ds-ink'
    }`

  const renderNavButton = (item: NavItem, headingKey?: string): ReactElement => {
    const Icon = item.icon
    return (
      <button
        key={item.category}
        type="button"
        className={catCls(item.category)}
        onClick={() => setCategory(item.category)}
      >
        <Icon className="h-4 w-4 shrink-0 opacity-70" strokeWidth={1.75} />
        <span className="min-w-0 flex-1 truncate">{t(item.labelKey)}</span>
        {headingKey && normalizedQuery ? (
          <span className="shrink-0 truncate text-[11px] font-medium text-ds-faint">{t(headingKey)}</span>
        ) : null}
      </button>
    )
  }

  return (
    <aside className="ds-drag flex w-[264px] shrink-0 flex-col border-r border-ds-border bg-ds-sidebar backdrop-blur-md">
      <div className="px-3 pb-3 pt-3">
        <div aria-hidden className="ds-titlebar-safe-block" />
        <button
          type="button"
          onClick={goBack}
          className="ds-no-drag flex items-center gap-2 rounded-lg px-2 py-2 text-[14px] text-ds-muted hover:bg-ds-hover hover:text-ds-ink"
        >
          <ChevronLeft className="h-4 w-4" strokeWidth={1.75} />
          {t('back')}
        </button>
        <div className="ds-no-drag relative mt-3">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ds-faint" strokeWidth={1.75} />
          <input
            aria-label={t('settingsSearchLabel')}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t('settingsSearchPlaceholder')}
            className="h-9 w-full rounded-lg border border-ds-border bg-ds-card/80 pl-8 pr-8 text-[13px] text-ds-ink shadow-sm outline-none placeholder:text-ds-faint focus:border-accent/40 focus:ring-1 focus:ring-accent/25"
          />
          {query ? (
            <button
              type="button"
              aria-label={t('settingsSearchClear')}
              title={t('settingsSearchClear')}
              onClick={() => setQuery('')}
              className="absolute right-1.5 top-1/2 flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-md text-ds-faint transition hover:bg-ds-hover hover:text-ds-ink"
            >
              <X className="h-3.5 w-3.5" strokeWidth={1.9} />
            </button>
          ) : null}
        </div>
      </div>
      <nav className="ds-no-drag min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {visibleGroups.length === 0 ? (
          <div className="rounded-lg border border-ds-border-muted bg-ds-main/40 px-3 py-3 text-[12.5px] leading-5 text-ds-muted">
            {t('settingsSearchEmpty')}
          </div>
        ) : normalizedQuery ? (
          <div className="flex flex-col gap-1">
            {visibleGroups.flatMap((group) => group.items.map((item) => renderNavButton(item, group.headingKey)))}
          </div>
        ) : (
          <div className="flex flex-col gap-4">
            {visibleGroups.map((group) => (
              <section key={group.headingKey} className="space-y-1">
                <div className="px-2 text-[11px] font-semibold uppercase tracking-normal text-ds-faint">
                  {t(group.headingKey)}
                </div>
                <div className="space-y-1">
                  {group.items.map((item) => renderNavButton(item))}
                </div>
              </section>
            ))}
          </div>
        )}
      </nav>
      <div className="ds-no-drag mt-auto border-t border-ds-border p-3">
        <div className="flex items-center gap-2 rounded-lg px-2 py-2">
          <div className="flex h-8 w-8 items-center justify-center rounded-full bg-ds-subtle text-ds-muted">
            <Settings className="h-4 w-4" strokeWidth={1.75} />
          </div>
          <div className="min-w-0 text-[12px] text-ds-muted">
            <div className="truncate font-medium text-ds-ink">Analytix</div>
            <div className="truncate">{t('settingsFooter')}</div>
          </div>
        </div>
      </div>
    </aside>
  )
}
