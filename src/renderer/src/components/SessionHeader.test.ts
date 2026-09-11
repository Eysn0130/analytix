import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import i18n from '../i18n'
import { useChatStore } from '../store/chat-store'
import sessionHeaderSource from './SessionHeader.tsx?raw'
import { isTogglePinnedThreadShortcut, SessionHeader } from './SessionHeader'

const initialChatState = useChatStore.getState()

async function readBaseShellSource(): Promise<string> {
  const nodeFs = 'node:fs/promises'
  const { readFile } = await import(/* @vite-ignore */ nodeFs)
  return readFile(new URL('../styles/base-shell.css', import.meta.url), 'utf8')
}

describe('SessionHeader', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
    useChatStore.setState({
      ...initialChatState,
      workspaceLabel: 'Working directory',
      activeThreadId: 'thread-1',
      threads: [{
        id: 'thread-1',
        title: 'Fix drag region',
        updatedAt: '2026-06-10T10:00:00.000Z',
        model: 'deepseek-chat',
        mode: 'chat',
        workspace: '/workspace/analytix'
      }]
    })
  })

  afterEach(() => {
    useChatStore.setState(initialChatState)
  })

  it('keeps the compact session title area draggable in desktop shells', () => {
    const html = renderToStaticMarkup(createElement(SessionHeader, { compact: true }))

    expect(html).toContain('session-header-compact flex')
    expect(html).not.toContain('session-header-compact ds-no-drag')
    expect(html).toContain('ds-session-title-compact')
    expect(html).toContain('Working directory')
  })

  it('renders the session action menu outside topbar layout and caps compact title width', async () => {
    const baseShellSource = await readBaseShellSource()

    expect(sessionHeaderSource).toContain("import { createPortal } from 'react-dom'")
    expect(sessionHeaderSource).toContain('const actionsMenuPortalTarget')
    expect(sessionHeaderSource).toContain('actionsMenuPortalRef')
    expect(sessionHeaderSource).toContain('createPortal(renderThreadActionsMenu(true), actionsMenuPortalTarget)')
    expect(sessionHeaderSource).toContain("className={`ds-session-actions-menu${floating ? ' ds-session-actions-menu-floating ds-no-drag' : ''}`}")
    expect(baseShellSource).toMatch(/\.ds-session-title-compact \{\s+max-width: clamp\(88px, 16vw, 180px\);\s+\}/)
    expect(baseShellSource).toMatch(/\.ds-session-actions-menu-floating \{[\s\S]*?position: fixed;[\s\S]*?z-index: 10000;[\s\S]*?overflow: visible;/)
    const floatingMenuRule = baseShellSource.match(/\.ds-session-actions-menu-floating \{[\s\S]*?\n\}/)?.[0] ?? ''
    expect(floatingMenuRule).not.toContain('overflow-y: auto')
    expect(floatingMenuRule).not.toContain('overscroll-behavior')
  })

  it('mounts direct source preview from the product session action surface', () => {
    expect(sessionHeaderSource).toContain('DirectSourcePreview,')
    expect(sessionHeaderSource).toContain('ImportMappingPreview,')
    expect(sessionHeaderSource).toContain("label={t('sessionActionDataImport')}")
    expect(sessionHeaderSource).toContain('<DirectSourcePreview />')
    expect(sessionHeaderSource).toContain('<ImportMappingPreview selector={stagedFundsImport.items[selectedImportItem].selector} />')
    expect(sessionHeaderSource).not.toContain('<DirectSourcePreview workspaceRoot=')
    expect(sessionHeaderSource).not.toContain('entityRef')
  })

  it('offers Main-owned staging and selector-only status, confirm, and cancel actions', () => {
    expect(sessionHeaderSource).toContain("label={t('sessionActionDataImport')}")
    expect(sessionHeaderSource).toContain("t('fundsCSVStageButton')")
    expect(sessionHeaderSource).toContain('window.analytix.runtime.stageFundsCSVSnapshot()')
    expect(sessionHeaderSource).not.toMatch(/stageFundsCSVSnapshot\([^)]/)
    expect(sessionHeaderSource).toContain('window.analytix.runtime.statusFundsCSVImport(selector)')
    expect(sessionHeaderSource).toContain('window.analytix.runtime.confirmFundsCSVSnapshot(selector)')
    expect(sessionHeaderSource).toContain('window.analytix.runtime.cancelFundsCSVImport(selector)')
    expect(sessionHeaderSource).not.toMatch(/(?:statusFundsCSVImport|confirmFundsCSVSnapshot|cancelFundsCSVImport)\([^)]*(?:workspace|case|snapshot|principal|grant|sourcePath)/i)
    expect(sessionHeaderSource).toContain("stagedFundsImport.status !== 'ready'")
  })

  it('matches the CodexDesktop pinned-thread shortcut', () => {
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true })).toBe(true)
    expect(isTogglePinnedThreadShortcut({ key: 'P', altKey: true, metaKey: true })).toBe(true)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true })).toBe(false)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true, ctrlKey: true })).toBe(false)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true, shiftKey: true })).toBe(false)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true, repeat: true })).toBe(false)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true, isComposing: true })).toBe(false)
    expect(isTogglePinnedThreadShortcut({ key: 'p', altKey: true, metaKey: true, defaultPrevented: true })).toBe(false)
  })
})
