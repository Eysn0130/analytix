// @vitest-environment jsdom
import sidebarSource from './svg/Sidebar.tsx?raw'
import newChatSource from './svg/ComposeEditSquare.tsx?raw'
import searchSource from './svg/Search.tsx?raw'
import plusSource from './svg/PlusComposer.tsx?raw'
import micSource from './svg/MicLgDictate.tsx?raw'
import license from './LICENSE?raw'
import { act, createElement, createRef } from 'react'
import { createRoot } from 'react-dom/client'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { OpenAiUiIcons } from './OpenAiUiIcons'
import provenance from './provenance.json'

describe('bounded official icon candidates', () => {
  it('retains exactly five admitted source objects and their MIT notice', async () => {
    const { createHash } = await vi.importActual<{
      createHash: (algorithm: string) => { update: (value: string) => { digest: (encoding: 'hex') => string } }
    }>('node:crypto')
    const sources: Record<string, string> = {
      Sidebar: sidebarSource, ComposeEditSquare: newChatSource, Search: searchSource,
      PlusComposer: plusSource, MicLgDictate: micSource
    }
    expect(provenance.icons).toHaveLength(5)
    for (const icon of provenance.icons) {
      const source = sources[icon.name]
      const byteLength = new TextEncoder().encode(source).length
      expect(createHash('sha1').update('blob ' + byteLength + '\0' + source).digest('hex')).toBe(icon.sourceBlobSha)
      expect(createHash('sha256').update(source).digest('hex')).toBe(icon.sourceSha256)
      expect(icon.exactWebPathOrPixelMatch).toBe(false)
    }
    expect(license).toContain('Copyright 2025 OpenAI')
  })

  it('keeps native vector painting while accepting existing size and accessibility props', () => {
    for (const Icon of Object.values(OpenAiUiIcons)) {
      const html = renderToStaticMarkup(createElement(Icon, {
        size: 18, strokeWidth: 9, fill: 'none', absoluteStrokeWidth: true,
        'aria-label': 'Existing operation', className: 'existing-icon'
      }))
      expect(html).toContain('viewBox="0 0 24 24"')
      expect(html).toContain('width="18"')
      expect(html).toContain('height="18"')
      expect(html).toContain('fill="currentColor"')
      expect(html).toContain('stroke="none"')
      expect(html).not.toContain('stroke-width="9"')
      expect(html).toContain('aria-label="Existing operation"')
      expect(html).toContain('class="existing-icon"')
    }
  })

  it('preserves the SVG ref, event handler and explicit dimensions', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true)
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const ref = createRef<SVGSVGElement>()
    const onClick = vi.fn()
    try {
      await act(async () => root.render(createElement(OpenAiUiIcons.search, {
        ref, onClick, size: 18, width: 14, height: 16
      })))
      expect(ref.current).toBe(container.querySelector('svg'))
      expect(ref.current?.getAttribute('width')).toBe('14')
      expect(ref.current?.getAttribute('height')).toBe('16')
      await act(async () => ref.current?.dispatchEvent(new MouseEvent('click', { bubbles: true })))
      expect(onClick).toHaveBeenCalledTimes(1)
    } finally {
      await act(async () => root.unmount())
      container.remove()
      vi.unstubAllGlobals()
    }
  })
})
