import { useId, useRef, type Dispatch, type KeyboardEvent, type ReactElement, type SetStateAction } from 'react'

export type SettingsCategory =
  | 'account'
  | 'general'
  | 'providers'
  | 'write'
  | 'imageGeneration'
  | 'speechToText'
  | 'agents'
  | 'archives'
  | 'permissions'
  | 'browser'
  | 'computerUse'
  | 'worktree'
  | 'memory'
  | 'shortcuts'
  | 'claw'
  | 'updates'

export type SettingsTabId = 'general' | 'models' | 'workspace' | 'tools'

type SettingsTab = {
  id: SettingsTabId
  labelKey: string
  categories: readonly SettingsCategory[]
}

export const SETTINGS_TABS: readonly SettingsTab[] = [
  { id: 'general', labelKey: 'settingsTabGeneral', categories: ['general', 'shortcuts', 'updates'] },
  { id: 'models', labelKey: 'settingsTabModels', categories: ['providers', 'imageGeneration', 'speechToText'] },
  { id: 'workspace', labelKey: 'settingsTabWorkspace', categories: ['write', 'worktree', 'memory', 'archives'] },
  { id: 'tools', labelKey: 'settingsTabTools', categories: ['agents', 'permissions', 'browser', 'computerUse', 'claw'] }
]

const CATEGORY_LABEL_KEYS: Record<SettingsCategory, string> = {
  account: 'deprecatedHubCompatibility',
  general: 'general',
  providers: 'providers',
  write: 'write',
  imageGeneration: 'imageGen',
  speechToText: 'speechToText',
  agents: 'agents',
  archives: 'archives',
  permissions: 'permissions',
  browser: 'browser',
  computerUse: 'computerUse',
  worktree: 'worktree',
  memory: 'memory',
  shortcuts: 'keyboardShortcuts',
  claw: 'claw',
  updates: 'updates'
}

export function settingsCategoryLabelKey(category: SettingsCategory): string {
  return CATEGORY_LABEL_KEYS[category] ?? 'title'
}

export function settingsTabForCategory(category: SettingsCategory): SettingsTab {
  return SETTINGS_TABS.find(tab => tab.categories.includes(category)) ?? SETTINGS_TABS[0]!
}

export function SettingsSidebar({
  category,
  setCategory,
  t
}: {
  category: SettingsCategory
  goBack: () => void
  setCategory: Dispatch<SetStateAction<SettingsCategory>>
  t: (key: string) => string
}): ReactElement {
  const sectionId = useId()
  const activeTab = settingsTabForCategory(category)
  const tabButtons = useRef<Partial<Record<SettingsTabId, HTMLButtonElement | null>>>({})
  const activateTab = (tab: SettingsTab): void => {
    if (tab.id !== activeTab.id) setCategory(tab.categories[0]!)
  }
  const handleTabKey = (event: KeyboardEvent<HTMLButtonElement>, index: number): void => {
    if (event.altKey || event.ctrlKey || event.metaKey || event.nativeEvent.isComposing) return
    const next = event.key === 'Home' ? 0
      : event.key === 'End' ? SETTINGS_TABS.length - 1
        : event.key === 'ArrowRight' ? (index + 1) % SETTINGS_TABS.length
          : event.key === 'ArrowLeft' ? (index - 1 + SETTINGS_TABS.length) % SETTINGS_TABS.length
            : null
    if (next === null) return
    event.preventDefault()
    const tab = SETTINGS_TABS[next]!
    activateTab(tab)
    tabButtons.current[tab.id]?.focus()
  }

  return (
    <nav className="ax-settings-navigation" aria-label={t('title')}>
      <div className="ax-settings-tabs" role="tablist" aria-label={t('title')}>
        {SETTINGS_TABS.map((tab, index) => (
          <button
            key={tab.id}
            ref={element => { tabButtons.current[tab.id] = element }}
            type="button"
            role="tab"
            id={`ax-settings-tab-${tab.id}`}
            aria-controls="ax-settings-tabpanel"
            className="ax-settings-tab"
            aria-selected={activeTab.id === tab.id}
            tabIndex={activeTab.id === tab.id ? 0 : -1}
            onClick={() => activateTab(tab)}
            onKeyDown={event => handleTabKey(event, index)}
          >
            {t(tab.labelKey)}
          </button>
        ))}
      </div>
      <div className="ax-settings-section-picker">
        <label htmlFor={sectionId}>{t('settingsSectionLabel')}</label>
        <select
          id={sectionId}
          value={category}
          onChange={event => setCategory(event.target.value as SettingsCategory)}
        >
          {activeTab.categories.map(section => (
            <option key={section} value={section}>{t(settingsCategoryLabelKey(section))}</option>
          ))}
          {category === 'account' ? (
            <option value="account">{t(settingsCategoryLabelKey('account'))}</option>
          ) : null}
        </select>
      </div>
    </nav>
  )
}
