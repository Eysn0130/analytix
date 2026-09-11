import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { shellToolbarIconButtonClass, ToolbarTooltip } from './ShellToolbar'
import shellToolbarSource from './ShellToolbar.tsx?raw'

describe('ShellToolbar', () => {
  it('keeps toolbar icon button classes stable', () => {
    expect(shellToolbarIconButtonClass()).toBe('ds-toolbar-icon-button')
    expect(shellToolbarIconButtonClass(true)).toBe('ds-toolbar-icon-button is-active')
    expect(shellToolbarIconButtonClass(false, 'relative')).toBe('ds-toolbar-icon-button relative')
    expect(shellToolbarIconButtonClass(true, 'ds-toolbar-icon-button-menu')).toBe(
      'ds-toolbar-icon-button is-active ds-toolbar-icon-button-menu'
    )
  })

  it('hides empty tooltip labels', () => {
    const html = renderToStaticMarkup(
      createElement(
        ToolbarTooltip,
        { label: '   ' },
        createElement('button', { type: 'button' }, 'Tool')
      )
    )

    expect(html).toContain('ds-toolbar-tooltip-anchor')
    expect(html).toContain('data-tooltip-hidden="true"')
  })

  it('resets activation tooltip suppression after pointer leave or blur', () => {
    expect(shellToolbarSource).toContain('onClickCapture={() => setSuppressed(true)}')
    expect(shellToolbarSource).toContain("event.key === 'Enter' || event.key === ' '")
    expect(shellToolbarSource).toContain('onPointerLeave={() => setSuppressed(false)}')
    expect(shellToolbarSource).toContain('onBlurCapture')
  })
})
