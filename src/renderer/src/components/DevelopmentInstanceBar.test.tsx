import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { DevelopmentInstanceDetails } from './DevelopmentInstanceBar'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

describe('development identity UI', () => {
  it('keeps packaged UI free of development labels', () => {
    expect(renderToStaticMarkup(createElement(DevelopmentInstanceDetails, { environment: { mode: 'packaged' } }))).toBe('')
  })
  it('shows an explicit Mock label and inspectable identity without guessing a build', () => {
    const html = renderToStaticMarkup(createElement(DevelopmentInstanceDetails, { environment: {
      mode: 'development', mock: true, isolated: true, profileId: 'abcdef123456', version: '1.0.6'
    } }))
    expect(html).toContain('developmentInstanceMock')
    expect(html).toContain('abcdef123456')
    expect(html).toContain('developmentInstanceUnknown')
    expect(html).toContain('<summary')
    expect(html).not.toContain('/Users/')
  })
})
