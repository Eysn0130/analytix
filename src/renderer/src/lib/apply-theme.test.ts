import { afterEach, describe, expect, it, vi } from 'vitest'
import { APP_ACCENT_COLORS } from '@shared/app-settings'
import { applyTheme, applyCursorSpotlight, applyDocumentLocale, applyMotionPreference, applyUiFontScale, isShellMotionReduced } from './apply-theme'

function stubMotionEnvironment(initialMatches: boolean): {
  dataset: Record<string, string>
  setMatches: (matches: boolean) => void
  addEventListener: ReturnType<typeof vi.fn>
  removeEventListener: ReturnType<typeof vi.fn>
} {
  const dataset: Record<string, string> = {}
  let matches = initialMatches
  let changeListener: (() => void) | null = null
  const addEventListener = vi.fn((_event: string, listener: () => void) => {
    changeListener = listener
  })
  const removeEventListener = vi.fn((_event: string, listener: () => void) => {
    if (changeListener === listener) changeListener = null
  })
  const mq = {
    get matches() {
      return matches
    },
    addEventListener,
    removeEventListener
  }

  vi.stubGlobal('document', {
    documentElement: { dataset }
  })
  vi.stubGlobal('window', {
    matchMedia: vi.fn(() => mq)
  })

  return {
    dataset,
    setMatches: (nextMatches) => {
      matches = nextMatches
      changeListener?.()
    },
    addEventListener,
    removeEventListener
  }
}

describe('applyDocumentLocale', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('writes a BCP-47 tag onto <html lang> for each supported locale', () => {
    const attributes = new Map<string, string>()
    vi.stubGlobal('document', {
      documentElement: {
        getAttribute: (name: string) => attributes.get(name) ?? null,
        setAttribute: (name: string, value: string) => {
          attributes.set(name, value)
        }
      }
    })

    applyDocumentLocale('en')
    expect(attributes.get('lang')).toBe('en')

    applyDocumentLocale('zh')
    expect(attributes.get('lang')).toBe('zh-CN')
  })

  it('does not touch the attribute when the locale already matches', () => {
    let writes = 0
    vi.stubGlobal('document', {
      documentElement: {
        getAttribute: () => 'en',
        setAttribute: () => {
          writes += 1
        }
      }
    })

    applyDocumentLocale('en')
    expect(writes).toBe(0)
  })
})

describe('applyCursorSpotlight', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('mirrors the cursor spotlight setting onto <html>', () => {
    const dataset: Record<string, string> = {}
    vi.stubGlobal('document', {
      documentElement: { dataset }
    })

    applyCursorSpotlight(true)
    expect(dataset.cursorSpotlight).toBe('on')

    applyCursorSpotlight(false)
    expect(dataset.cursorSpotlight).toBe('off')
  })
})

describe('applyMotionPreference', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('forces shell motion on even when the system prefers reduced motion', () => {
    const { dataset, addEventListener } = stubMotionEnvironment(true)

    applyMotionPreference('on')

    expect(dataset.motionPreference).toBe('on')
    expect(dataset.motionReduced).toBe('false')
    expect(isShellMotionReduced()).toBe(false)
    expect(addEventListener).not.toHaveBeenCalled()
  })

  it('defaults to shell motion on when no preference is saved', () => {
    const { dataset } = stubMotionEnvironment(true)

    applyMotionPreference(undefined)

    expect(dataset.motionPreference).toBe('on')
    expect(dataset.motionReduced).toBe('false')
    expect(isShellMotionReduced()).toBe(false)
  })

  it('forces shell motion off regardless of the system preference', () => {
    const { dataset } = stubMotionEnvironment(false)

    applyMotionPreference('off')

    expect(dataset.motionPreference).toBe('off')
    expect(dataset.motionReduced).toBe('true')
    expect(isShellMotionReduced()).toBe(true)
  })

  it('mirrors system reduced-motion changes when set to system', () => {
    const { dataset, setMatches, addEventListener } = stubMotionEnvironment(false)

    applyMotionPreference('system')
    expect(addEventListener).toHaveBeenCalledWith('change', expect.any(Function))
    expect(dataset.motionPreference).toBe('system')
    expect(dataset.motionReduced).toBe('false')

    setMatches(true)
    expect(dataset.motionReduced).toBe('true')
    expect(isShellMotionReduced()).toBe(true)
  })
})

describe('applyUiFontScale content typography', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function styleProperties() {
    const properties = new Map<string, string>()
    vi.stubGlobal('document', { documentElement: {
      style: { setProperty: (name: string, value: string) => properties.set(name, value) }
    } })
    return properties
  }

  it('aligns the existing default small tier with the DSH 14px content baseline', () => {
    const properties = styleProperties()
    applyUiFontScale('small')
    expect(properties.get('--ds-ui-scale')).toBe('0.82')
    expect(properties.get('--dsh-content-font-size')).toBe('14px')
    expect([...properties.keys()]).toEqual(['--ds-ui-scale', '--dsh-content-font-size'])
  })

  it('retains explicit medium and large ratios and reapplies a later user selection', () => {
    const properties = styleProperties()
    // A saved medium selection can be the first call; no default is written first.
    applyUiFontScale('medium')
    expect(properties.get('--ds-ui-scale')).toBe('0.88')
    const medium = Number.parseFloat(properties.get('--dsh-content-font-size')!)
    expect(medium).toBeCloseTo(15.024390243902438, 10)
    applyUiFontScale('large')
    expect(properties.get('--ds-ui-scale')).toBe('1')
    const large = Number.parseFloat(properties.get('--dsh-content-font-size')!)
    expect(large).toBeCloseTo(17.073170731707318, 10)
    expect(large / medium).toBeCloseTo(1 / 0.88, 10)
    applyUiFontScale('small')
    expect(properties.get('--dsh-content-font-size')).toBe('14px')
    expect(properties.get('--ds-ui-scale')).toBe('0.82')
  })
})


describe('applyTheme accent preference', () => {
  const fixture = () => {
    const attributes = new Map<string, string>()
    const properties = new Map<string, string>()
    const listeners = new Set<() => void>()
    let dark = false
    const media = {
      get matches() { return dark },
      addEventListener: vi.fn((_event: string, listener: () => void) => listeners.add(listener)),
      removeEventListener: vi.fn((_event: string, listener: () => void) => listeners.delete(listener))
    }
    vi.stubGlobal('document', { documentElement: {
      setAttribute: (key: string, value: string) => attributes.set(key, value),
      style: { setProperty: (key: string, value: string) => properties.set(key, value) }
    } })
    vi.stubGlobal('window', { matchMedia: () => media })
    return { attributes, properties, listeners, media,
      setDark: (value: boolean) => { dark = value; listeners.forEach(listener => listener()) } }
  }
  afterEach(() => { applyTheme('light'); vi.unstubAllGlobals() })
  it('retains the current preset across system changes and replaces the old listener', () => {
    const f = fixture()
    applyTheme('system', 'orange')
    expect(f.attributes.get('data-accent')).toBe('orange')
    expect(f.properties.get('--ax-primary')).toBe('#ce7919')
    f.setDark(true)
    expect(f.properties.get('--ax-primary')).toBe('#ffb667')
    expect(f.properties.get('--ax-on-primary')).toBe('#2b1a08')
    applyTheme('system', 'teal')
    expect(f.listeners.size).toBe(1)
    f.setDark(false)
    expect(f.properties.get('--ax-primary')).toBe('#0c7b8a')
    applyTheme('light')
    expect(f.attributes.get('data-accent')).toBe('blue')
    expect(f.listeners.size).toBe(0)
  })
  it('keeps all preset foregrounds readable in normal, hover and pressed states', () => {
    const f = fixture()
    const luminance = (hex: string) => {
      const rgb = [1, 3, 5].map(index => Number.parseInt(hex.slice(index, index + 2), 16) / 255)
        .map(value => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)
      return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722
    }
    const contrast = (a: string, b: string) => {
      const [low, high] = [luminance(a), luminance(b)].sort((x, y) => x - y)
      return (high + 0.05) / (low + 0.05)
    }
    for (const accent of APP_ACCENT_COLORS) for (const mode of ['light', 'dark'] as const) {
      applyTheme(mode, accent)
      const ink = f.properties.get('--ax-on-primary')!
      for (const state of ['--ax-primary', '--ax-primary-hover', '--ax-primary-pressed']) {
        expect(contrast(ink, f.properties.get(state)!)).toBeGreaterThanOrEqual(4.5)
      }
      expect(contrast(f.properties.get('--ax-accent-text')!, f.properties.get('--ax-primary-soft')!)).toBeGreaterThanOrEqual(4.5)
    }
  })
})
