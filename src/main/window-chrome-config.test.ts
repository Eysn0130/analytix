import { readFile } from 'node:fs/promises'
import { describe, expect, it } from 'vitest'

import { MACOS_TRAFFIC_LIGHT_POSITION } from '../shared/window-chrome'

async function readMainSource(): Promise<string> {
  return readFile(new URL('./index.ts', import.meta.url), 'utf8')
}

describe('main window chrome configuration', () => {
  it('uses CodexDesktop-style macOS hiddenInset chrome with explicit traffic light coordinates', async () => {
    const source = await readMainSource()

    expect(MACOS_TRAFFIC_LIGHT_POSITION).toEqual({ x: 16, y: 16 })
    expect(source).toContain("titleBarStyle: process.platform === 'darwin' ? 'hiddenInset'")
    expect(source).not.toContain('frame: false')
    expect(source).toContain('trafficLightPosition: process.platform === \'darwin\' ? MACOS_TRAFFIC_LIGHT_POSITION : undefined')
    expect(source).toContain('acceptFirstMouse: true')
  })
})
