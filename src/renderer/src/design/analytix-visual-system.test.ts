import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { AnalytixIconRegistry, AnalytixIconSizes } from './AnalytixIconRegistry'
import {
  AnalytixSurfaceTokens,
  BorderTokens,
  ElevationTokens,
  MotionTokens,
  RadiusTokens
} from './analytix-visual-tokens'

describe('Analytix visual system baseline', () => {
  it('declares the approved icon sizes', () => {
    expect(AnalytixIconSizes).toEqual({
      xs: 14,
      sm: 16,
      md: 18,
      lg: 20,
      xl: 24
    })
  })

  it('exposes brand assets through the icon registry', () => {
    expect(Object.keys(AnalytixIconRegistry.brand).sort()).toEqual([
      'appIcon512',
      'splashImage',
      'symbolColor',
      'symbolLoaderMask',
      'symbolMonoBlack',
      'symbolMonoWhite',
      'symbolReversed'
    ])
    expect(AnalytixIconRegistry.icons.workspace).toBeTruthy()
    expect(AnalytixIconRegistry.icons.settings).toBeTruthy()
    expect(AnalytixIconRegistry.icons.disclosureDown).toBeTruthy()
    expect(AnalytixIconRegistry.icons.disclosureRight).toBeTruthy()
    expect(AnalytixIconRegistry.icons.sidebarHide).toBeTruthy()
    expect(AnalytixIconRegistry.icons.sidebarShow).toBeTruthy()
    expect(AnalytixIconRegistry.icons.browser).toBeTruthy()
    expect(AnalytixIconRegistry.icons.comment).toBeTruthy()
    expect(AnalytixIconRegistry.icons.pin).toBeTruthy()
    expect(AnalytixIconRegistry.icons.pinFilled).toBeTruthy()
    expect(AnalytixIconRegistry.icons.plan).toBeTruthy()
    expect(AnalytixIconRegistry.icons.terminal).toBeTruthy()
  })

  it('uses the CodexDesktop pin icon geometry for pinned thread states', () => {
    const outline = renderToStaticMarkup(createElement(AnalytixIconRegistry.icons.pin))
    const filled = renderToStaticMarkup(createElement(AnalytixIconRegistry.icons.pinFilled))

    expect(outline).toContain('width="20"')
    expect(outline).toContain('viewBox="0 0 20 20"')
    expect(outline).toContain('M11.8349 12.5C11.8349')
    expect(filled).toContain('width="24"')
    expect(filled).toContain('viewBox="0 0 24 24"')
    expect(filled).toContain('M12.8636 3.26029C13.9444')
  })

  it('maps surface, elevation, border, radius, and motion tokens to CSS variables', () => {
    expect(AnalytixSurfaceTokens.card).toBe('var(--ds-card-strong)')
    expect(ElevationTokens.popover).toBe('var(--ax-shadow-popover)')
    expect(ElevationTokens.modal).toBe('var(--ax-shadow-modal)')
    expect(BorderTokens.focus).toBe('var(--ax-border-focus)')
    expect(RadiusTokens.panel).toBe('var(--ax-radius-panel)')
    expect(MotionTokens.hover).toBe('var(--ax-motion-hover)')
    expect(MotionTokens.sidebarCollapse).toBe('var(--ax-motion-sidebar-collapse)')
    expect(MotionTokens.easingCodex).toBe('var(--ax-ease-codex)')
  })
})
