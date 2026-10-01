import { createElement, createRef } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '../../i18n'
import { WriteWorkspaceToolbar } from './WriteWorkspaceToolbar'

describe('WriteWorkspaceToolbar', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('keeps shell navigation controls out of the write toolbar', () => {
    const html = renderToStaticMarkup(
      createElement(WriteWorkspaceToolbar, {
        activeFileIsImage: false,
        activeFileIsText: true,
        activeFilePath: '/workspace/notes.md',
        documentStatsLabel: null,
        assistantOpen: false,
        exportInFlight: false,
        exportMenuOpen: false,
        exportMenuRef: createRef<HTMLDivElement>(),
        liveModeActive: true,
        modeMenuItems: [],
        modeMenuOpen: false,
        modeMenuRef: createRef<HTMLDivElement>(),
        onApplyOfficialDocumentFormat: vi.fn(),
        onApplyTextAlign: vi.fn(),
        onResetOfficialDocumentFormat: vi.fn(),
        onTypographyChange: vi.fn(),
        onCopyRichText: vi.fn(),
        onExportFile: vi.fn(),
        onSave: vi.fn(),
        readOnly: false,
        saveLabel: 'Saved',
        saveStatus: 'saved',
        setAssistantOpen: vi.fn(),
        setExportMenuOpen: vi.fn(),
        setModeMenuOpen: vi.fn(),
        setPreviewMode: vi.fn()
      })
    )

    expect(html).not.toContain('chat-topbar-drag-region')
    expect(html).not.toContain('session-header-compact')
    expect(html).toContain('aria-label="Typography tools"')
    expect(html).toContain('ds-toolbar-icon-svg')
    expect(html).toContain('write-workspace-toolbar-modes relative flex min-w-0 items-center justify-start')
    expect(html).toContain('write-mode-split-control')
    expect(html).toContain('is-live-active')
    expect(html).toContain('write-mode-split-live')
    expect(html).toContain('write-mode-split-live-icon')
    expect(html).toContain('write-mode-split-label')
    expect(html).toContain('write-mode-split-divider')
    expect(html).toContain('write-mode-split-menu')
    expect(html).toContain('write-mode-split-menu-icon')
    expect(html).toContain('aria-label="Live editor"')
    expect(html).toContain('aria-pressed="true"')
    expect(html).toContain('aria-label="Preview only"')
    expect(html).toContain('write-typography-controls')
    expect(html).toContain('write-typography-segment-button')
    expect(html).toContain('write-typography-official-button')
    expect(html).toContain('write-typography-font-mark')
    expect(html).toContain('write-typography-chevron')
    expect(html).toContain('aria-label="Typography tools"')
    expect(html).toContain('aria-label="Apply official document format"')
    expect(html).toContain('Official')
    expect(html).toContain('aria-label="Font"')
    expect(html).toContain('aria-label="Font size"')
    expect(html).toContain('aria-label="Line spacing"')
    expect(html).toContain('aria-label="Alignment"')
    expect(html).toContain('aria-haspopup="menu"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('write-workspace-toolbar-actions ds-no-drag flex w-full min-w-0 flex-wrap items-center gap-2')
    expect(html).toContain('write-toolbar-action-icons')
    expect(html).toContain('write-save-icon-saved')
    expect(html).toContain('ds-toolbar-tooltip-anchor')
    expect(html).toContain('role="tooltip"')
    expect(html).not.toContain('ds-toolbar-icon-svg text-emerald-600')
    expect(html).not.toContain('ds-toolbar-icon-button ds-toolbar-icon-button-menu write-toolbar-menu-button')
    expect(html).not.toContain('ds-toolbar-icon-button write-mode-button')
    expect(html).not.toContain('inline-flex min-w-[64px] justify-center rounded-lg')
    expect(html).not.toContain('hidden text-[12.5px] font-semibold sm:inline')
    expect(html).not.toContain('hidden lg:inline')
    expect(html).not.toContain('More view modes')
    expect(html).not.toContain('title=')
    expect(html).not.toContain('mt-3')
    expect(html).not.toContain('rounded-[24px]')
    expect(html).not.toContain('h-8 w-8')
    expect(html).not.toContain('ds-chat-sidebar-toggle')
    expect(html).not.toContain('data-cursor-spotlight-target')
  })
})
