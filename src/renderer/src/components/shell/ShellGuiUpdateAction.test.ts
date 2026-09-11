import { describe, expect, it } from 'vitest'
import type { GuiUpdateInfo, GuiUpdateState } from '@shared/gui-update'
import {
  shellGuiUpdateActionInfo,
  shellGuiUpdateActionMode,
  shellGuiUpdateBusy
} from './ShellGuiUpdateAction'

const updateInfo: Extract<GuiUpdateInfo, { ok: true }> = {
  ok: true,
  currentVersion: '0.1.0',
  latestVersion: '0.1.1',
  hasUpdate: true,
  releaseUrl: 'https://example.test/analytix',
  channel: 'stable' as const
}

describe('ShellGuiUpdateAction', () => {
  it('shows only actionable update states in the shell controls', () => {
    expect(shellGuiUpdateActionInfo({ status: 'idle' })).toBeNull()
    expect(shellGuiUpdateActionInfo({
      status: 'not_available',
      info: { ...updateInfo, hasUpdate: false }
    })).toBeNull()
    expect(shellGuiUpdateActionInfo({ status: 'available', info: updateInfo })).toEqual(updateInfo)
    expect(shellGuiUpdateActionInfo({ status: 'downloaded', info: { ...updateInfo, downloaded: true } }))
      .toMatchObject({ downloaded: true })
    expect(shellGuiUpdateActionInfo({
      status: 'error',
      message: 'network',
      info: { ...updateInfo, manualOnly: true }
    })).toMatchObject({ manualOnly: true })
  })

  it('treats download, install, and local apply states as busy', () => {
    const downloading: GuiUpdateState = {
      status: 'downloading',
      info: updateInfo,
      progress: {
        total: 100,
        delta: 10,
        transferred: 50,
        percent: 50,
        bytesPerSecond: 1000
      }
    }
    expect(shellGuiUpdateBusy({ status: 'idle' }, false)).toBe(false)
    expect(shellGuiUpdateBusy({ status: 'idle' }, true)).toBe(true)
    expect(shellGuiUpdateBusy(downloading, false)).toBe(true)
    expect(shellGuiUpdateBusy({ status: 'installing', info: updateInfo }, false)).toBe(true)
  })

  it('uses a compact icon in the open sidebar and text when collapsed', () => {
    expect(shellGuiUpdateActionMode(false)).toBe('icon')
    expect(shellGuiUpdateActionMode(true)).toBe('text')
  })
})
