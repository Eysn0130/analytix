import { afterEach, describe, expect, it, vi } from 'vitest'
import { applyCursorSpotlight, applyDocumentLocale, applyMotionPreference, isShellMotionReduced } from './apply-theme'

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
