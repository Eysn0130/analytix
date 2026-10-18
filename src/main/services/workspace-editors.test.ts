import { afterEach, beforeEach, expect, test, vi } from 'vitest'

vi.mock('electron', () => ({
  app: { getFileIcon: vi.fn() },
  shell: { openPath: vi.fn().mockResolvedValue(''), showItemInFolder: vi.fn() }
}))
const nativeExec = vi.hoisted(() => vi.fn(async (..._args: unknown[]): Promise<{ stdout: string; stderr: string }> => { throw new Error('not installed') }))
vi.mock('node:child_process', () => {
  const execFile = vi.fn()
  Object.defineProperty(execFile, Symbol.for('nodejs.util.promisify.custom'), { value: nativeExec })
  return { execFile }
})
vi.mock('./workspace-paths', () => ({
  pathExists: vi.fn().mockResolvedValue(false),
  resolveOpenTargetPath: vi.fn().mockResolvedValue('/resolved/workspace/report.docx')
}))

import { app, shell } from 'electron'
import { listEditorsResult, openEditorPath, listPresentFileApplications, openPresentFileApplication } from './workspace-editors'
import { resolveOpenTargetPath } from './workspace-paths'

const platformDescriptor = Object.getOwnPropertyDescriptor(process, 'platform')!
beforeEach(() => {
  vi.clearAllMocks()
  nativeExec.mockReset().mockRejectedValue(new Error('not installed'))
})
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

const associatedApps = [
  { applicationId: 'org.example.Other', label: 'Other application', isDefault: false, appPath: '/Applications/Other.app' },
  { applicationId: 'org.example.Actual', label: 'Actual default', isDefault: true, appPath: '/Applications/Actual.app' }
]
function darwinApplications() {
  Object.defineProperty(process, 'platform', { value: 'darwin', configurable: true })
  nativeExec.mockImplementation(async (command: unknown) => ({ stdout: command === '/usr/bin/osascript' ? JSON.stringify(associatedApps) : '', stderr: '' }))
}
test('Present queries actual file associations with fixed source/argv and strips private application URLs', async () => {
  darwinApplications()
  const result = await listPresentFileApplications('/private/report with spaces.md', async () => true)
  expect(result).toEqual({ ok: true, applications: associatedApps.map(({ appPath: _appPath, ...visible }) => visible) })
  expect(JSON.stringify(result)).not.toContain('/Applications/')
  expect(nativeExec).toHaveBeenCalledWith('/usr/bin/osascript', expect.arrayContaining(['/private/report with spaces.md']), expect.objectContaining({ timeout: 5000 }))
  const args = nativeExec.mock.calls[0][1] as string[]
  expect(args[3]).toContain('URLsForApplicationsToOpenURL')
  expect(args[3]).toContain('NSFileManager.defaultManager.displayNameAtPath')
  expect(args[3]).not.toContain('/private/report with spaces.md')
  expect(shell.openPath).not.toHaveBeenCalled()
})
test.each([undefined, 'org.example.Other'])('Present opens the current default or exact associated identity %s', async applicationId => {
  darwinApplications()
  expect(await openPresentFileApplication('/private/current.md', applicationId, async () => true)).toEqual({ ok: true })
  expect(nativeExec).toHaveBeenLastCalledWith('/usr/bin/open', ['-a', applicationId ? '/Applications/Other.app' : '/Applications/Actual.app', '/private/current.md'], expect.objectContaining({ timeout: 10000 }))
  expect(shell.openPath).not.toHaveBeenCalled()
})
test('Present unknown identities and launch errors never invoke an ordinary fallback or second launch', async () => {
  darwinApplications()
  expect(await openPresentFileApplication('/private/current.md', 'system', async () => true)).toEqual({ ok: false, code: 'unsupported' })
  expect(nativeExec).toHaveBeenCalledTimes(1)
  nativeExec.mockImplementation(async (command: unknown) => {
    if (command === '/usr/bin/osascript') return { stdout: JSON.stringify(associatedApps), stderr: '' }
    throw new Error('launch failed')
  })
  expect(await openPresentFileApplication('/private/current.md', undefined, async () => true)).toEqual({ ok: false, code: 'unavailable' })
  expect(nativeExec.mock.calls.filter(([command]) => command === '/usr/bin/open')).toHaveLength(1)
  expect(shell.openPath).not.toHaveBeenCalled()
})
test('Present query retirement suppresses the late catalog and launch', async () => {
  darwinApplications()
  let current = true
  nativeExec.mockImplementation(async () => { current = false; return { stdout: JSON.stringify(associatedApps), stderr: '' } })
  expect(await openPresentFileApplication('/private/current.md', undefined, async () => current)).toEqual({ ok: false, code: 'unavailable' })
  expect(nativeExec.mock.calls.some(([command]) => command === '/usr/bin/open')).toBe(false)
  expect(await listPresentFileApplications('/private/current.md', async () => false)).toEqual({ ok: false, code: 'unavailable' })
})
test.each(['win32', 'linux'])('Present reports unsupported native association on %s without execution', async platform => {
  Object.defineProperty(process, 'platform', { value: platform, configurable: true })
  expect(await listPresentFileApplications('/private/current.md', async () => true)).toEqual({ ok: false, code: 'unsupported' })
  expect(await openPresentFileApplication('/private/current.md', undefined, async () => true)).toEqual({ ok: false, code: 'unsupported' })
  expect(nativeExec).not.toHaveBeenCalled()
})


test('Present observes no default, duplicate installed identities and malformed native catalogs without fallback', async () => {
  darwinApplications()
  nativeExec.mockResolvedValue({ stdout: JSON.stringify([{ ...associatedApps[0], isDefault: false }]), stderr: '' })
  expect(await openPresentFileApplication('/private/current.md', undefined, async () => true)).toEqual({ ok: false, code: 'unsupported' })
  expect(nativeExec.mock.calls.some(([command]) => command === '/usr/bin/open')).toBe(false)
  nativeExec.mockResolvedValue({ stdout: JSON.stringify([{ ...associatedApps[1], isDefault: false, appPath: '/Applications/Duplicate.app' }, associatedApps[1]]), stderr: '' })
  expect(await listPresentFileApplications('/private/current.md', async () => true)).toEqual({ ok: true, applications: [{ applicationId: associatedApps[1].applicationId, label: associatedApps[1].label, isDefault: true }] })
  for (const stdout of ['not-json', JSON.stringify([{ ...associatedApps[0], unknown: true }]), JSON.stringify(associatedApps.map(value => ({ ...value, isDefault: true })))]) {
    nativeExec.mockResolvedValue({ stdout, stderr: '' })
    expect(await listPresentFileApplications('/private/current.md', async () => true)).toEqual({ ok: false, code: 'unavailable' })
  }
  expect(shell.openPath).not.toHaveBeenCalled()
})
test('Present retires after icon hydration and at the final guard before native launch', async () => {
  darwinApplications(); let current = true
  vi.mocked(app.getFileIcon).mockImplementationOnce(async () => { current = false; return { toDataURL: () => 'data:image/png;base64,AAAA' } as Electron.NativeImage })
  expect(await listPresentFileApplications('/private/current.md', async () => current)).toEqual({ ok: false, code: 'unavailable' })
  nativeExec.mockClear(); let checks = 0
  expect(await openPresentFileApplication('/private/current.md', undefined, async () => ++checks < 3)).toEqual({ ok: false, code: 'unavailable' })
  expect(nativeExec.mock.calls.some(([command]) => command === '/usr/bin/open')).toBe(false)
})


test.each(['darwin', 'win32', 'linux'] as const)('native file icons use the supported platform size on %s', async platform => {
  Object.defineProperty(process, 'platform', { value: platform, configurable: true })
  const png = Buffer.alloc(64, 1)
  const image = {
    isEmpty: () => false,
    resize() { return this },
    toPNG: () => png
  } as unknown as Electron.NativeImage
  const iconDataUrl = `data:image/png;base64,${png.toString('base64')}`
  vi.mocked(app.getFileIcon).mockResolvedValueOnce(image)
  if (platform === 'darwin') {
    darwinApplications()
    vi.mocked(app.getFileIcon).mockResolvedValueOnce(image)
    expect(await listPresentFileApplications('/private/synthetic.md', async () => true)).toEqual({
      ok: true,
      applications: associatedApps.map(({ appPath: _appPath, ...visible }) => ({ ...visible, iconDataUrl }))
    })
    expect(vi.mocked(app.getFileIcon).mock.calls).toEqual(associatedApps.map(application => [application.appPath, { size: 'normal' }]))
  } else {
    nativeExec.mockImplementation(async (command: unknown, args: unknown) => {
      if (command === (platform === 'win32' ? 'where' : 'which') && (args as string[])[0] === 'code') {
        return { stdout: '/synthetic/editor-code', stderr: '' }
      }
      throw new Error('not installed')
    })
    const catalog = await listEditorsResult()
    expect(catalog.editors.find(editor => editor.id === 'vscode')?.iconDataUrl).toBe(iconDataUrl)
    expect(app.getFileIcon).toHaveBeenCalledExactlyOnceWith('/synthetic/editor-code', { size: 'large' })
  }
})
