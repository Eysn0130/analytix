// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceEntry } from '@shared/workspace-file'
import i18n from '../../i18n'
import { WriteFileTree } from './WriteFileTree'

let root: Root
let container: HTMLDivElement
const names = ['report.docx', 'budget.xlsx', 'deck.pptx', 'notes.md', 'notes.txt', 'main.ts']
const entries: WorkspaceEntry[] = names.map(name => ({ name, path: `/workspace/${name}`, type: 'file', ext: name.slice(name.lastIndexOf('.')) }))
entries.push({ name: 'src', path: '/workspace/src', type: 'directory', ext: '' })
const onSelectFile = vi.fn()
const onAddReference = vi.fn()
const onCreateFile = vi.fn()
const onCreateDirectory = vi.fn()
const onRenameEntry = vi.fn()
const onDeleteEntry = vi.fn()
const onToggleDir = vi.fn()
const onRefresh = vi.fn()

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
  vi.clearAllMocks()
  await i18n.changeLanguage('zh')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
  await act(async () => root.render(createElement(WriteFileTree, {
    rootDirectory: '/workspace', entriesByDir: { '/workspace': entries }, expandedDirs: new Set<string>(), loadingDirs: {},
    selectedFilePath: null, error: null, onSelectFile, onAddReference, onCreateFile, onCreateDirectory,
    onRenameEntry, onDeleteEntry, onToggleDir, onRefresh
  })))
})
afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
  vi.unstubAllGlobals()
})
function button(title: string): HTMLButtonElement {
  return [...container.querySelectorAll<HTMLButtonElement>('button')].find(item => item.title === title || item.textContent === title)!
}
async function click(element: HTMLElement): Promise<void> {
  await act(async () => element.click())
}

describe('shared workspace file tree', () => {
  it('dispatches DOCX, XLSX, PPTX, markdown, plain text and code through the same entry callback', async () => {
    for (const name of names) await click(button(name))
    expect(onSelectFile.mock.calls.map(([path]) => path)).toEqual(names.map(name => `/workspace/${name}`))
    expect(onToggleDir).not.toHaveBeenCalled()
  })
  it('keeps file and directory references separate from opening and preserves file operations', async () => {
    await click(button(i18n.t('common:fileTreeAddFileReference')))
    expect(onAddReference).toHaveBeenLastCalledWith(entries[0])
    await click(button(i18n.t('common:fileTreeAddFolderReference')))
    expect(onAddReference).toHaveBeenLastCalledWith(entries.at(-1))
    await click(button(i18n.t('common:writeRenameEntry')))
    expect(onRenameEntry).toHaveBeenCalledExactlyOnceWith(entries[0])
    await click(button(i18n.t('common:writeDeleteFile')))
    expect(onDeleteEntry).toHaveBeenCalledExactlyOnceWith(entries[0])
    await click(button(i18n.t('common:writeCreateFile')))
    await click(button(i18n.t('common:writeCreateFolder')))
    expect(onCreateFile).toHaveBeenCalledExactlyOnceWith()
    expect(onCreateDirectory).toHaveBeenCalledExactlyOnceWith()
    expect(onSelectFile).not.toHaveBeenCalled()
  })
  it('exposes a reference action in the context menu and dismisses it after dispatch', async () => {
    await act(async () => button('main.ts').dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 24, clientY: 24 })))
    const menuItem = container.querySelector<HTMLButtonElement>('[role="menuitem"]')!
    await click(menuItem)
    expect(onAddReference).toHaveBeenCalledExactlyOnceWith(entries[5])
    expect(container.querySelector('[role="menu"]')).toBeNull()
    expect(onSelectFile).not.toHaveBeenCalled()
  })
})
