import { describe, expect, it } from 'vitest'
import { detectFrontend, isFrontendPath, listDesignRules } from './detect.js'

function ruleIds(source: string, options = {}): string[] {
  return detectFrontend(source, { filePath: 'x.css', ...options }).map((finding) => finding.ruleId)
}

describe('design quality detection', () => {
  it('gates detection to frontend-like paths', () => {
    expect(isFrontendPath('src/App.tsx')).toBe(true)
    expect(isFrontendPath('styles.css')).toBe(true)
    expect(isFrontendPath('server.ts')).toBe(false)
    expect(detectFrontend('bg-gradient-to-r from-violet-500 to-blue-500', { filePath: 'a.ts' })).toEqual([])
  })

  it('flags high-signal default design patterns', () => {
    const source = [
      '<div className="bg-gradient-to-r from-violet-500 to-blue-500">',
      '  <h1 className="text-[7rem] tracking-[-0.06em]">Launch</h1>',
      '  <span style={{ animation: "fade 1s" }}>powerful features</span>',
      '</div>'
    ].join('\n')
    const standard = ruleIds(source, { filePath: 'a.tsx' })
    const strict = ruleIds(source, { filePath: 'a.tsx', strictness: 'strict' })
    expect(standard).toContain('slop-purple-blue-gradient')
    expect(standard).toContain('quality-hero-font-ceiling')
    expect(standard).toContain('quality-display-tracking-floor')
    expect(standard).toContain('quality-missing-reduced-motion')
    expect(strict).toContain('slop-purple-blue-gradient')
  })

  it('honors ignoreRules and maxFindings', () => {
    const source = Array.from({ length: 8 }, () => '<div className="z-[9999]">modal</div>').join('\n')
    expect(ruleIds(source, { filePath: 'a.tsx', ignoreRules: ['quality-arbitrary-z-index'] })).not.toContain(
      'quality-arbitrary-z-index'
    )
    expect(detectFrontend(source, { filePath: 'a.tsx', maxFindings: 3 })).toHaveLength(3)
  })

  it('lists rule metadata', () => {
    const rules = listDesignRules()
    expect(rules.length).toBeGreaterThanOrEqual(7)
    expect(rules.every((rule) => rule.id && rule.title && rule.category && rule.severity)).toBe(true)
  })
})
