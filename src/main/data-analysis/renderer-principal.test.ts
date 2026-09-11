import { EventEmitter } from 'node:events'
import { describe, expect, it, vi } from 'vitest'
import { bindDataAnalysisRendererPrincipal } from './renderer-principal'

describe('data analysis renderer principal host binding', () => {
  it('registers only from the host-owned load lifecycle and stops after disposal', () => {
    const contents = Object.assign(new EventEmitter(), {
      mainFrame: { url: 'file:///app/out/renderer/index.html' },
      isDestroyed: vi.fn(() => false)
    })
    const window = {
      webContents: contents,
      isDestroyed: vi.fn(() => false)
    }
    const manager = { registerRenderer: vi.fn(() => true) }

    const dispose = bindDataAnalysisRendererPrincipal(manager as never, window as never)
    expect(manager.registerRenderer).not.toHaveBeenCalled()

    contents.emit('did-finish-load')
    expect(manager.registerRenderer).toHaveBeenCalledTimes(1)
    expect(manager.registerRenderer).toHaveBeenCalledWith(contents, contents.mainFrame)

    dispose()
    contents.emit('did-finish-load')
    expect(manager.registerRenderer).toHaveBeenCalledTimes(1)
  })
})
