import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import i18n from '../../i18n'
import { describe, expect, it } from 'vitest'
import type { WorkspaceEntry } from '@shared/workspace-file'
import {
  ChatFileTreePanel,
  formatChatFileTreeUnsupportedMessage,
  isChatFileTreeIgnoredDirectory,
  isChatFileTreePreviewableEntry
} from './ChatFileTreePanel'

function entry(overrides: Partial<WorkspaceEntry> & Pick<WorkspaceEntry, 'name' | 'type'>): WorkspaceEntry {
  return {
    name: overrides.name,
    type: overrides.type,
    path: overrides.path ?? `/tmp/project/${overrides.name}`,
    ext: overrides.ext ?? ''
  }
}

describe('ChatFileTreePanel helpers', () => {
  it('ignores heavyweight dependency and VCS directories', () => {
    expect(isChatFileTreeIgnoredDirectory('.git')).toBe(true)
    expect(isChatFileTreeIgnoredDirectory('node_modules')).toBe(true)
    expect(isChatFileTreeIgnoredDirectory('src')).toBe(false)
  })

  it('opens native office, document and code files through the shared browser', () => {
    expect(isChatFileTreePreviewableEntry(entry({ name: 'main.ts', type: 'file' }))).toBe(true)
    for (const name of ['report.docx', 'budget.xlsx', 'deck.pptx', 'notes.md', 'notes.txt', 'logo.png']) {
      expect(isChatFileTreePreviewableEntry(entry({ name, type: 'file' }))).toBe(true)
    }
    expect(isChatFileTreePreviewableEntry(entry({ name: 'archive.zip', type: 'file' }))).toBe(false)
    expect(isChatFileTreePreviewableEntry(entry({ name: 'src', type: 'directory' }))).toBe(false)
  })

  it('formats unsupported preview titles without leaking UI state', () => {
    expect(formatChatFileTreeUnsupportedMessage('logo.png')).toContain('logo.png')
  })
})


it.each([
  ['en', 'Workspace files', 'Add workspace'],
  ['zh', '工作区文件', '添加工作区']
])('renders the workspace browser without duplicate controls or old writing labels (%s)', async (language, heading, addWorkspace) => {
  await i18n.changeLanguage(language)
  const html = renderToStaticMarkup(createElement(ChatFileTreePanel, {
    workspaceRoot: '/synthetic/workspace', onPreviewFile: () => undefined,
    onAddReference: () => undefined, fill: true
  }))
  expect(html).not.toContain('ds-right-panel-topbar')
  expect(html).not.toContain('ds-panel-collapse-button')
  expect(html).not.toContain('Collapse right sidebar')
  expect(html).toContain(heading)
  expect(html).toContain(addWorkspace)
  expect(html).not.toMatch(/Writing spaces|Add writing space|写作空间/i)
})
