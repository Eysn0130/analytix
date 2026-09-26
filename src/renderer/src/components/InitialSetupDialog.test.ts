import { describe, expect, it, vi } from 'vitest'
import {
  canCloseInitialSetup,
  completeInitialSetupAfterSave
} from './InitialSetupDialog'

describe('InitialSetupDialog completion flow', () => {
  it('keeps required first-run setup modal-only until the runtime is ready, then opens Code', async () => {
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
    expect(probeRuntime).toHaveBeenCalledWith('user', { restart: true })
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
    expect(probeRuntime).toHaveBeenCalledWith('user', { restart: true })
    expect(openCode).not.toHaveBeenCalled()
    expect(selectSavedModel).not.toHaveBeenCalled()
    expect(closeInitialSetup).not.toHaveBeenCalled()
    expect(setDialogError).toHaveBeenCalledWith('Port is busy.')
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
