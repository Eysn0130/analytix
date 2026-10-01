import { describe, expect, it } from 'vitest'
import type { WorkspaceEntry } from '@shared/workspace-file'
import {
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
