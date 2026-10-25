import { describe, expect, it, vi } from 'vitest'
import {
  canCloseInitialSetup,
  completeInitialSetupAfterSave
} from './InitialSetupDialog'

describe('InitialSetupDialog completion flow', () => {
  it.each(['reload', 'probe', 'openCode'] as const)('retires completion after newer edits while %s is pending', async (phase) => {
    let release!: () => void
    const gate = new Promise<void>((resolve) => { release = resolve })
    let current = true
    const step = (name: typeof phase) => vi.fn(async () => { if (phase === name) await gate })
    const reloadUiSettings = step('reload')
    const probeRuntime = step('probe')
    const openCode = step('openCode')
    const selectSavedModel = vi.fn()
    const closeInitialSetup = vi.fn()
    const setDialogError = vi.fn()
    const completion = completeInitialSetupAfterSave({
      mode: 'required', reloadUiSettings, probeRuntime, openCode,
      selectSavedModel, closeInitialSetup, setDialogError,
      getState: () => ({ runtimeConnection: 'ready', error: null }),
      fallbackRuntimeError: 'Synthetic runtime failure', restartRuntimeError: 'Synthetic startup failure',
      isCurrent: () => current
    })
    const pending = phase === 'reload' ? reloadUiSettings : phase === 'probe' ? probeRuntime : openCode
    await vi.waitFor(() => expect(pending).toHaveBeenCalledTimes(1))
    current = false; release()
    await expect(completion).resolves.toBe(false)
    expect(selectSavedModel).not.toHaveBeenCalled()
    expect(closeInitialSetup).not.toHaveBeenCalled()
    expect(setDialogError).not.toHaveBeenCalled()
    if (phase === 'reload') expect(probeRuntime).not.toHaveBeenCalled()
    if (phase !== 'openCode') expect(openCode).not.toHaveBeenCalled()
  })

  it('checks the committed Provider without a second restart, then opens Code', async () => {
    const reloadUiSettings = vi.fn(async () => undefined)
    const probeRuntime = vi.fn(async () => undefined)
    const openCode = vi.fn(async () => undefined)
    const closeInitialSetup = vi.fn()
    const selectSavedModel = vi.fn()
    const setDialogError = vi.fn()

    const completed = await completeInitialSetupAfterSave({
      mode: 'required',
      reloadUiSettings,
      probeRuntime,
      openCode,
      closeInitialSetup,
      selectSavedModel,
      getState: () => ({ runtimeConnection: 'ready', error: null }),
      setDialogError,
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    expect(completed).toBe(true)
    expect(reloadUiSettings).toHaveBeenCalledTimes(1)
    expect(probeRuntime).toHaveBeenCalledWith('user')
    expect(openCode).toHaveBeenCalledTimes(1)
    expect(selectSavedModel).toHaveBeenCalledTimes(1)
    expect(selectSavedModel.mock.invocationCallOrder[0]).toBeGreaterThan(openCode.mock.invocationCallOrder[0])
    expect(selectSavedModel.mock.invocationCallOrder[0]).toBeLessThan(closeInitialSetup.mock.invocationCallOrder[0])
    expect(closeInitialSetup).toHaveBeenCalledTimes(1)
    expect(setDialogError).not.toHaveBeenCalled()
  })

  it('does not close required first-run setup when the runtime cannot connect', async () => {
    const closeInitialSetup = vi.fn()
    const selectSavedModel = vi.fn()
    const openCode = vi.fn(async () => undefined)
    const setDialogError = vi.fn()
    const probeRuntime = vi.fn(async () => undefined)

    const completed = await completeInitialSetupAfterSave({
      mode: 'required',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime,
      openCode,
      closeInitialSetup,
      selectSavedModel,
      getState: () => ({ runtimeConnection: 'offline', error: 'Port is busy.' }),
      setDialogError,
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    expect(completed).toBe(false)
    expect(probeRuntime).toHaveBeenCalledWith('user')
    expect(openCode).not.toHaveBeenCalled()
    expect(selectSavedModel).not.toHaveBeenCalled()
    expect(closeInitialSetup).not.toHaveBeenCalled()
    expect(setDialogError).toHaveBeenCalledWith('Port is busy.')
  })

  it('waits for the readiness probe before closing setup', async () => {
    let finishProbe: (() => void) | undefined
    const pendingProbe = new Promise<void>((resolve) => { finishProbe = resolve })
    let runtimeConnection: 'idle' | 'ready' = 'idle'
    const closeInitialSetup = vi.fn()
    const probeRuntime = vi.fn(async () => {
      await pendingProbe
      runtimeConnection = 'ready'
    })
    const completion = completeInitialSetupAfterSave({
      mode: 'required',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime,
      openCode: vi.fn(async () => undefined),
      selectSavedModel: vi.fn(),
      closeInitialSetup,
      getState: () => ({ runtimeConnection, error: null }),
      setDialogError: vi.fn(),
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    await vi.waitFor(() => expect(probeRuntime).toHaveBeenCalledWith('user'))
    expect(closeInitialSetup).not.toHaveBeenCalled()
    finishProbe?.()
    await expect(completion).resolves.toBe(true)
    expect(closeInitialSetup).toHaveBeenCalledTimes(1)
  })

  it('shows first-run restart guidance instead of the generic fetch failure after saving credentials', async () => {
    const closeInitialSetup = vi.fn()
    const selectSavedModel = vi.fn()
    const openCode = vi.fn(async () => undefined)
    const setDialogError = vi.fn()

    const completed = await completeInitialSetupAfterSave({
      mode: 'required',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => undefined),
      openCode,
      closeInitialSetup,
      selectSavedModel,
      getState: () => ({ runtimeConnection: 'offline', error: 'Could not reach Analytix.' }),
      setDialogError,
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    expect(completed).toBe(false)
    expect(openCode).not.toHaveBeenCalled()
    expect(selectSavedModel).not.toHaveBeenCalled()
    expect(closeInitialSetup).not.toHaveBeenCalled()
    expect(setDialogError).toHaveBeenCalledWith('Analytix could not start after saving the API key.')
  })

  it('keeps redacted runtime detail with first-run startup guidance', async () => {
    const setDialogError = vi.fn()

    const completed = await completeInitialSetupAfterSave({
      mode: 'required',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime: vi.fn(async () => undefined),
      openCode: vi.fn(async () => undefined),
      closeInitialSetup: vi.fn(),
      selectSavedModel: vi.fn(),
      getState: () => ({
        runtimeConnection: 'offline',
        error: 'fetch: The operation was aborted due to timeout',
        runtimeErrorDetail: 'Code: fetch_failed\n\nMessage:\nfetch: The operation was aborted due to timeout'
      }),
      setDialogError,
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    expect(completed).toBe(false)
    expect(setDialogError).toHaveBeenCalledWith(
      'Analytix could not start after saving the API key.\n\nCode: fetch_failed\n\nMessage:\nfetch: The operation was aborted due to timeout'
    )
  })

  it('keeps preview setup dismissible and avoids forcing the user into Code', async () => {
    const probeRuntime = vi.fn(async () => undefined)
    const openCode = vi.fn(async () => undefined)
    const closeInitialSetup = vi.fn()
    const selectSavedModel = vi.fn()

    const completed = await completeInitialSetupAfterSave({
      mode: 'preview',
      reloadUiSettings: vi.fn(async () => undefined),
      probeRuntime,
      openCode,
      closeInitialSetup,
      selectSavedModel,
      getState: () => ({ runtimeConnection: 'offline', error: null }),
      setDialogError: vi.fn(),
      fallbackRuntimeError: 'Could not reach Analytix.',
      restartRuntimeError: 'Analytix could not start after saving the API key.'
    })

    expect(completed).toBe(true)
    expect(probeRuntime).toHaveBeenCalledWith('background')
    expect(openCode).not.toHaveBeenCalled()
    expect(selectSavedModel).not.toHaveBeenCalled()
    expect(closeInitialSetup).toHaveBeenCalledTimes(1)
  })

  it('only allows manual close in preview mode', () => {
    expect(canCloseInitialSetup('required')).toBe(false)
    expect(canCloseInitialSetup('preview')).toBe(true)
  })
})
