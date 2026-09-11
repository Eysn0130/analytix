import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { NormalizedThread } from '../agent/types'
import { SettingsSidebar } from './SettingsSidebar'
import { ArchivedThreadsSettingsSection, filterArchivedThreads } from './settings-section-archives'

function thread(overrides: Partial<NormalizedThread> & Pick<NormalizedThread, 'id'>): NormalizedThread {
  return {
    id: overrides.id,
    title: overrides.title ?? overrides.id,
    updatedAt: overrides.updatedAt ?? '2026-06-01T00:00:00.000Z',
    model: overrides.model ?? 'deepseek-chat',
    mode: overrides.mode ?? 'agent',
    workspace: overrides.workspace ?? '/Users/zxy/project-a',
    ...(overrides.archived !== undefined ? { archived: overrides.archived } : {}),
    ...(overrides.preview ? { preview: overrides.preview } : {})
  }
}

const labels: Record<string, string> = {
  back: 'Back',
  general: 'General',
  providers: 'Providers',
  write: 'Write',
  imageGen: 'Image generation',
  mediaGeneration: 'Media generation',
  speechToText: 'Speech to text',
  agents: 'AI assistant',
  permissions: 'Permissions',
  browser: 'Browser',
  computerUse: 'Computer control',
  worktree: 'Worktrees',
  memory: 'Memory',
  archives: 'Archived chats',
  keyboardShortcuts: 'Keyboard shortcuts',
  easterEgg: 'Mode workshop',
  updates: 'Version & updates',
  debug: 'Troubleshooting',
  claw: 'Connect phone',
  settingsFooter: 'Settings',
  settingsNavPersonal: 'Personal',
  settingsNavIntegrations: 'Integrations',
  settingsNavCoding: 'Coding',
  settingsNavArchived: 'Archived group',
  settingsSearchLabel: 'Search settings',
  settingsSearchPlaceholder: 'Search settings',
  settingsSearchClear: 'Clear search',
  settingsSearchEmpty: 'No matching settings',
  archivesTitle: 'Archived chats',
  archivesOverview: 'Archived chat history',
  archivesOverviewDesc: 'Review archived chats.',
  archivesSearchPlaceholder: 'Search archived chats',
  archivesCount: '{{count}} archived',
  archivesWorkspaceCount: '{{count}} chats',
  archivesEmpty: 'No archived chats yet.',
  archivesSearchEmpty: 'No archived chats match your search.',
  archivesOffline: 'Connect the local runtime to refresh archived chats.',
  archivesUntitled: 'Untitled chat',
  archivesRestore: 'Restore',
  archivesDelete: 'Delete archived chat',
  sidebarThreadRestore: 'Restore thread',
  sidebarThreadDelete: 'Delete thread'
}

function t(key: string, options?: Record<string, unknown>): string {
  const template = labels[key] ?? key
  return template.replace(/\{\{(\w+)\}\}/g, (_, name: string) => String(options?.[name] ?? ''))
}

describe('ArchivedThreadsSettingsSection', () => {
  it('filters archived threads by title, preview, workspace, and model', () => {
    const archived = thread({
      id: 'archived',
      title: 'Plan launch',
      preview: 'Contains release checklist',
      archived: true,
      model: 'deepseek-v4-pro'
    })
    const active = thread({
      id: 'active',
      title: 'Plan launch',
      archived: false
    })

    expect(filterArchivedThreads([archived, active], 'release').map((item) => item.id)).toEqual(['archived'])
    expect(filterArchivedThreads([archived, active], 'deepseek-v4').map((item) => item.id)).toEqual(['archived'])
    expect(filterArchivedThreads([archived, active], 'Plan').map((item) => item.id)).toEqual(['archived'])
  })

  it('renders archived chats with restore and delete actions', () => {
    const html = renderToStaticMarkup(createElement(ArchivedThreadsSettingsSection, {
      ctx: {
        t,
        tCommon: t,
        threads: [
          thread({
            id: 'archived-a',
            title: 'Archived feature work',
            archived: true,
            preview: 'Move archived conversations into settings'
          })
        ],
        runtimeReady: true,
        locale: 'en-US',
        refreshThreads: async () => undefined,
        openCode: async () => undefined,
        selectThread: async () => undefined,
        archiveThread: async () => undefined,
        deleteThread: async () => undefined
      }
    }))

    expect(html).toContain('Archived chats')
    expect(html).toContain('Archived feature work')
    expect(html).toContain('Move archived conversations into settings')
    expect(html).toContain('Restore')
    expect(html).toContain('Delete archived chat')
  })

  it('places archived chats in the archived navigation group', () => {
    const html = renderToStaticMarkup(createElement(SettingsSidebar, {
      category: 'archives',
      goBack: () => undefined,
      setCategory: () => undefined,
      t
    }))

    const codingIndex = html.indexOf('Coding')
    const archivedGroupIndex = html.indexOf('Archived group')
    const archivesIndex = html.indexOf('Archived chats')
    expect(html).toContain('Search settings')
    expect(html).toContain('Browser')
    expect(html).toContain('Computer control')
    expect(codingIndex).toBeGreaterThanOrEqual(0)
    expect(archivedGroupIndex).toBeGreaterThan(codingIndex)
    expect(archivesIndex).toBeGreaterThan(archivedGroupIndex)
  })
})
