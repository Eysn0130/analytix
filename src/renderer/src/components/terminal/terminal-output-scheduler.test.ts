import { describe, expect, it, vi } from 'vitest'
import { TerminalOutputScheduler } from './terminal-output-scheduler'

describe('TerminalOutputScheduler', () => {
  it('batches terminal output into one write per frame', () => {
    const frames: Array<() => void> = []
    const write = vi.fn()
    const scheduler = new TerminalOutputScheduler({
      write,
      scheduleFrame: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancelFrame: () => undefined
    })

    scheduler.enqueue('a')
    scheduler.enqueue('b')
    scheduler.enqueue('c')

    expect(frames).toHaveLength(1)
    expect(write).not.toHaveBeenCalled()
    frames[0]?.()
    expect(write).toHaveBeenCalledWith('abc')
  })

  it('drops pending output after dispose', () => {
    const frames: Array<() => void> = []
    const write = vi.fn()
    const scheduler = new TerminalOutputScheduler({
      write,
      scheduleFrame: (callback) => {
        frames.push(callback)
        return frames.length
      },
      cancelFrame: () => undefined
    })

    scheduler.enqueue('pending')
    scheduler.dispose()
    frames[0]?.()
    expect(write).not.toHaveBeenCalled()
  })
})
