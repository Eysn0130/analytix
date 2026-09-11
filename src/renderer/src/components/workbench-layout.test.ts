import { afterEach, describe, expect, it } from 'vitest'
import {
  WORKBENCH_TIMELINE_MIN_HEIGHT,
  clampWorkbenchTerminalHeight,
  createWorkbenchScrollReserve,
  maxWorkbenchTerminalHeight,
  readWorkbenchLayoutStorage,
  workbenchLayoutStorageKey,
  workbenchLayoutStorageScope
} from './workbench-layout'

class MemoryStorage {
  private values = new Map<string, string>()

  getItem(key: string): string | null {
    return this.values.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value)
  }

  removeItem(key: string): void {
    this.values.delete(key)
  }
}

const originalLocalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')

function stubLocalStorage(): MemoryStorage {
  const storage = new MemoryStorage()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: storage
  })
  return storage
}

function restoreLocalStorage(): void {
  if (originalLocalStorage) {
    Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
  } else {
    Reflect.deleteProperty(globalThis, 'localStorage')
  }
}

afterEach(() => {
  restoreLocalStorage()
})

describe('workbench layout contracts', () => {
  it('describes no bottom reserve when the bottom panel is closed', () => {
    const reserve = createWorkbenchScrollReserve({
      bottomPanelOpen: false,
      bottomPanelHeight: 360
    })

    expect(reserve.bottomPanelOpen).toBe(false)
    expect(reserve.bottomPanelHeight).toBe(0)
    expect(reserve.bottomPanelReservePx).toBe(0)
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-height']).toBe('0px')
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-reserve']).toBe('0px')
  })

  it('includes the bottom resize handle in the timeline reserve', () => {
    const reserve = createWorkbenchScrollReserve({
      bottomPanelOpen: true,
      bottomPanelHeight: 360
    })

    expect(reserve.bottomPanelHeight).toBe(360)
    expect(reserve.bottomPanelReservePx).toBe(364)
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-height']).toBe('360px')
    expect(reserve.timelineReserveStyle['--analytix-bottom-panel-reserve']).toBe('364px')
  })

  it('clamps the terminal without consuming the minimum timeline space', () => {
    const containerHeight = 700

    expect(maxWorkbenchTerminalHeight(containerHeight)).toBe(
      containerHeight - WORKBENCH_TIMELINE_MIN_HEIGHT
    )
    expect(clampWorkbenchTerminalHeight(900, containerHeight)).toBe(440)
    expect(clampWorkbenchTerminalHeight(120, containerHeight)).toBe(220)
  })

  it('prefers workspace-scoped layout values while preserving legacy fallback', () => {
    const storage = stubLocalStorage()
    const workspaceRoot = '/Users/sun/Projects/analytix'
    const scope = workbenchLayoutStorageScope(workspaceRoot)
    storage.setItem('analytix.layout.leftSidebarWidth', '312')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.leftSidebarWidth', scope), '428')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.terminalOpen', scope), '1')

    const scopedLayout = readWorkbenchLayoutStorage(workspaceRoot)
    const fallbackLayout = readWorkbenchLayoutStorage('/Users/sun/Projects/other')

    expect(scopedLayout.leftSidebarWidth).toBe(428)
    expect(scopedLayout.terminalOpen).toBe(true)
    expect(fallbackLayout.leftSidebarWidth).toBe(312)
    expect(fallbackLayout.terminalOpen).toBe(false)
  })

  it('lets a scoped closed right panel override a legacy open panel', () => {
    const storage = stubLocalStorage()
    const workspaceRoot = '/Users/sun/Projects/analytix'
    const scope = workbenchLayoutStorageScope(workspaceRoot)
    storage.setItem('analytix.layout.rightPanelMode', 'browser')
    storage.setItem(workbenchLayoutStorageKey('analytix.layout.rightPanelMode', scope), 'none')

    expect(readWorkbenchLayoutStorage(workspaceRoot).rightPanelMode).toBeNull()
  })
})
