import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { ToolbarTooltip } from './ShellToolbar'

describe('ToolbarTooltip', () => {
  it('marks the wrapper as no-drag for frameless Windows hit testing', () => {
    const html = renderToStaticMarkup(
      createElement(ToolbarTooltip, { label: 'Back' }, createElement('button', { type: 'button' }, 'Back'))
    )

    expect(html).toContain('ds-toolbar-tooltip-anchor ds-no-drag')
  })
})
