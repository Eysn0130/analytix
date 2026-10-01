import { afterEach, beforeEach, expect, test, vi } from 'vitest'

vi.mock('electron', () => ({
  app: { getFileIcon: vi.fn() },
  shell: { openPath: vi.fn().mockResolvedValue(''), showItemInFolder: vi.fn() }
}))
vi.mock('node:child_process', () => ({
  execFile: vi.fn((_file: string, _args: string[], _options: unknown, callback: (error: Error) => void) => callback(new Error('not installed')))
}))
vi.mock('./workspace-paths', () => ({
  pathExists: vi.fn().mockResolvedValue(false),
  resolveOpenTargetPath: vi.fn().mockResolvedValue('/resolved/workspace/report.docx')
}))

import { shell } from 'electron'
import { listEditorsResult, openEditorPath } from './workspace-editors'
import { resolveOpenTargetPath } from './workspace-paths'

const platformDescriptor = Object.getOwnPropertyDescriptor(process, 'platform')!
beforeEach(() => { vi.clearAllMocks() })
afterEach(() => { Object.defineProperty(process, 'platform', platformDescriptor) })

test.each(['darwin', 'win32', 'linux'])('file-manager is always available and reveals the resolved file on %s', async platform => {
  Object.defineProperty(process, 'platform', { value: platform, configurable: true })
  const catalog = await listEditorsResult()
  expect(catalog.editors).toContainEqual(expect.objectContaining({ id: 'file-manager', kind: 'viewer', available: true }))
  expect(catalog.defaultEditorId).toBe('system')
  expect(await openEditorPath({ path: 'report.docx', workspaceRoot: '/workspace', editorId: 'file-manager' })).toEqual({ ok: true, path: '/resolved/workspace/report.docx', editorId: 'file-manager' })
  expect(resolveOpenTargetPath).toHaveBeenCalledWith('report.docx', '/workspace')
  expect(shell.showItemInFolder).toHaveBeenCalledExactlyOnceWith('/resolved/workspace/report.docx')
  expect(shell.openPath).not.toHaveBeenCalled()
})

test('system and unknown-editor fallback still open the file with its default application', async () => {
  for (const editorId of ['system', 'unknown-editor']) {
    expect(await openEditorPath({ path: 'report.docx', editorId })).toMatchObject({ ok: true, editorId: 'system' })
  }
  expect(shell.openPath).toHaveBeenCalledTimes(2)
  expect(shell.openPath).toHaveBeenLastCalledWith('/resolved/workspace/report.docx')
  expect(shell.showItemInFolder).not.toHaveBeenCalled()
})

test('Finder remains an alias for revealing a file on macOS', async () => {
  Object.defineProperty(process, 'platform', { value: 'darwin', configurable: true })
  expect(await openEditorPath({ path: 'report.docx', editorId: 'finder' })).toMatchObject({ ok: true, editorId: 'finder' })
  expect(shell.showItemInFolder).toHaveBeenCalledExactlyOnceWith('/resolved/workspace/report.docx')
  expect(shell.openPath).not.toHaveBeenCalled()
})

test.each(['file-manager', 'system'])('a path guard rejection prevents any shell action for %s', async editorId => {
  vi.mocked(resolveOpenTargetPath).mockRejectedValueOnce(new Error('Path must stay within the selected workspace.'))
  expect(await openEditorPath({ path: '../outside.docx', workspaceRoot: '/workspace', editorId })).toEqual({ ok: false, message: 'Path must stay within the selected workspace.' })
  expect(shell.showItemInFolder).not.toHaveBeenCalled()
  expect(shell.openPath).not.toHaveBeenCalled()
})
