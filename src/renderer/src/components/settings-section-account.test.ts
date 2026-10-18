import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import { avatarDisplayUrl } from '../account/hub-avatar'
import { useHubAccountStore } from '../account/hub-account-store'
import { SettingsSidebar, settingsCategoryLabelKey } from './SettingsSidebar'
import { AccountProfileSettingsSection } from './settings-section-account'

const labels: Record<string, string> = {
  account: 'Account',
  deprecatedHubCompatibility: 'Deprecated Hub compatibility',
  back: 'Back',
  general: 'General',
  settingsTabGeneral: 'General',
  settingsTabModels: 'Models',
  settingsTabWorkspace: 'Workspace',
  settingsTabTools: 'Tools & permissions',
  settingsSectionLabel: 'Settings section',
  write: 'Write',
  imageGen: 'Image generation',
  mediaGeneration: 'Media generation',
  speechToText: 'Speech to text',
  agents: 'AI assistant',
  archives: 'Archived chats',
  permissions: 'Permissions',
  worktree: 'Worktrees',
  memory: 'Memory',
  keyboardShortcuts: 'Keyboard shortcuts',
  easterEgg: 'Mode workshop',
  updates: 'Version & updates',
  claw: 'Connect phone',
  debug: 'Troubleshooting',
  settingsFooter: 'Settings'
}

function t(key: string): string {
  return labels[key] ?? key
}

describe('AccountProfileSettingsSection', () => {
  beforeEach(() => {
    useHubAccountStore.setState({
      snapshot: null,
      usage: null,
      referral: null,
      status: 'idle',
      error: ''
    })
  })

  it('renders the upstream-style profile layout and explicit sync empty states', () => {
    const html = renderToStaticMarkup(createElement(AccountProfileSettingsSection, {
      ctx: {
        locale: 'zh'
      }
    }))

    expect(html).toContain('个人资料')
    expect(html).toContain('Analytix')
    expect(html).toContain('@analytix')
    expect(html).toContain('Standard')
    expect(html).toContain('Token 活动')
    expect(html).toContain('活动洞察')
    expect(html).toContain('最常用的插件')
    expect(html).toContain('累计 Token 数')
    expect(html).toContain('官网未同步')
    expect(html).toContain('官网尚未同步插件运行记录')
  })

  it('normalizes Hub upload avatar URLs before rendering photo avatars', () => {
    expect(avatarDisplayUrl('/uploads/avatars/user-1.png')).toBe('https://analytix.top/uploads/avatars/user-1.png')
    expect(avatarDisplayUrl('https://analytix.top/uploads/avatars/user-1.png')).toBe('https://analytix.top/uploads/avatars/user-1.png')
    expect(avatarDisplayUrl('data:image/png;base64,abc')).toBe('data:image/png;base64,abc')
    expect(avatarDisplayUrl('uploads/avatars/user-1.png')).toBe('')
  })

  it('keeps deprecated Hub compatibility out of primary navigation while preserving its route label', () => {
    for (const category of ['general', 'providers', 'write', 'agents'] as const) {
      const primaryHtml = renderToStaticMarkup(createElement(SettingsSidebar, {
        category,
        goBack: () => undefined,
        setCategory: () => undefined,
        t
      }))
      expect(primaryHtml.match(/role="tab"/g)).toHaveLength(4)
      expect(primaryHtml).not.toContain('Deprecated Hub compatibility')
      expect(primaryHtml).not.toContain('value="account"')
    }

    const html = renderToStaticMarkup(createElement(SettingsSidebar, {
      category: 'account',
      goBack: () => undefined,
      setCategory: () => undefined,
      t
    }))

    expect(html.match(/role="tab"/g)).toHaveLength(4)
    expect(html.match(/<button[^>]*aria-selected="true"[^>]*>([^<]*)<\/button>/)?.[1]).toBe('General')
    expect(html.match(/value="account"/g)).toHaveLength(1)
    expect(html.match(/<option[^>]*selected=""[^>]*>([^<]*)<\/option>/)?.[1]).toBe('Deprecated Hub compatibility')
    expect(settingsCategoryLabelKey('account')).toBe('deprecatedHubCompatibility')
  })
})
