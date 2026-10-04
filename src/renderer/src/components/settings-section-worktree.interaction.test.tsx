// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { beforeEach, afterEach, expect, it, vi } from 'vitest'
import { useWorktreeStore } from '../stores/worktree-store'
import { WorktreeSettingsSection } from './settings-section-worktree'
import type { WorktreePoolStatus } from '@shared/worktree'

let host: HTMLDivElement, root: Root
const list = vi.fn(), acquire = vi.fn()
const status = (projectPath: string, isGitRepo = true): WorktreePoolStatus => ({ projectPath, poolDir: projectPath + '/pool', mainBranch: 'main', headCommit: '', isGitRepo, worktrees: [], inUseCount: 0 })
const render = async (path = '/synthetic/a') => act(async () => root.render(createElement(WorktreeSettingsSection, { ctx: { t: (key: string) => key, form: { workspaceRoot: path } } })))
const creates = () => [...host.querySelectorAll<HTMLButtonElement>('button')].filter(b => b.textContent === 'worktreeCreate')
beforeEach(() => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
  host = document.createElement('div'); document.body.append(host); root = createRoot(host)
  useWorktreeStore.getState().reset(); list.mockReset(); acquire.mockReset(); acquire.mockResolvedValue({})
  Object.assign(window, { analytix: { workspace: { listWorktrees: list, acquireWorktree: acquire } } })
})
afterEach(async () => { await act(async () => root.unmount()); host.remove(); useWorktreeStore.getState().reset(); vi.restoreAllMocks() })

it('blocks nonGit and associates the existing unavailable reason with every pool', async () => {
  list.mockResolvedValue(status('/synthetic/a', false)); await render()
  expect(creates()).toHaveLength(3)
  for (const [i, b] of creates().entries()) {
    expect(b.disabled).toBe(true); expect(b.getAttribute('aria-label')).toBe(`worktreeCreate · worktreePool ${i}`)
    expect(document.getElementById(b.getAttribute('aria-describedby')!)?.textContent).toBe('worktreeNotGitRepo')
    await act(async () => b.click())
  }
  expect(acquire).not.toHaveBeenCalled()
})
it('blocks pending, error and empty projects without acquiring', async () => {
  let reject!: (error: Error) => void
  list.mockReturnValue(new Promise((_resolve, fail) => { reject = fail }))
  await render(); expect(creates().every(b => b.disabled)).toBe(true)
  await act(async () => reject(new Error('Synthetic unavailable')))
  expect(creates().every(b => b.disabled)).toBe(true)
  await render(''); expect(creates().every(b => b.disabled)).toBe(true)
  expect(acquire).not.toHaveBeenCalled()
})
it('ignores a previous project response arriving after the current nonGit result', async () => {
  let resolveOld!: (value: WorktreePoolStatus) => void
  list.mockImplementation(({ projectPath }) => projectPath === '/synthetic/a'
    ? new Promise(resolve => { resolveOld = resolve }) : Promise.resolve(status(projectPath, false)))
  await render(); await render('/synthetic/b')
  await act(async () => resolveOld(status('/synthetic/a')))
  expect(useWorktreeStore.getState().poolStatus?.projectPath).toBe('/synthetic/b')
  expect(creates().every(b => b.disabled)).toBe(true)
  expect(acquire).not.toHaveBeenCalled()
})
it('allows the current ready Git project and retains the confirmed force continuation while busy', async () => {
  list.mockResolvedValue(status('/synthetic/a'))
  acquire.mockRejectedValueOnce(new Error('WORKTREE_HAS_CHANGES:0:2')).mockResolvedValue({})
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  await render(); expect(creates()[0].disabled).toBe(false)
  await act(async () => creates()[0].click())
  expect(acquire).toHaveBeenCalledTimes(2)
  expect(acquire.mock.calls[0][0]).toMatchObject({ projectPath: '/synthetic/a', poolIndex: 0 })
  expect(acquire.mock.calls[1][0]).toMatchObject({ projectPath: '/synthetic/a', poolIndex: 0, force: true })
})

it('does not refresh a previous project after its in-flight Create completes', async () => {
  let finish!: () => void
  list.mockImplementation(({ projectPath }) => Promise.resolve(status(projectPath)))
  acquire.mockReturnValue(new Promise<void>(resolve => { finish = resolve }))
  await render()
  await act(async () => { creates()[0].click() })
  await render('/synthetic/b')
  expect(creates()[0].disabled).toBe(false)
  const before = list.mock.calls.length
  await act(async () => finish())
  expect(list).toHaveBeenCalledTimes(before)
  expect(useWorktreeStore.getState().poolStatus?.projectPath).toBe('/synthetic/b')
  expect(creates()[0].disabled).toBe(false)
})
