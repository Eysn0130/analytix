import { createElement } from 'react'
import type { ErrorInfo } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppErrorBoundary } from './AppErrorBoundary'

describe('AppErrorBoundary', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders children when no error occurs', () => {
    const html = renderToStaticMarkup(
      createElement(AppErrorBoundary, null, createElement('div', { 'data-testid': 'child' }, 'hello'))
    )
    expect(html).toContain('hello')
    expect(html).not.toContain('appErrorTitle')
  })

  it('renders without throwing when given no children', () => {
    const result = renderToStaticMarkup(createElement(AppErrorBoundary, null, null))
    expect(typeof result).toBe('string')
  })

  it('withholds raw render errors from console, generic log args, state, and fallback HTML', () => {
    const logError = vi.fn().mockResolvedValue(undefined)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    vi.stubGlobal('window', { analytix: { logs: { error: logError } } })
    const boundary = new AppErrorBoundary({ children: null })
    const canaries = [
      'NEUTRAL_RENDER_ERROR_CANARY_7F3C',
      '/private/case/source.csv',
      `cer1_${'a'.repeat(64)}`,
      '6222020202020202020',
      'PRIVATE_RENDER_STACK_CANARY'
    ]
    const error = new Error(canaries.slice(0, 4).join(' | '))
    error.stack = `Error: ${canaries[4]}\n    at ${canaries[1]}`
    const info = { componentStack: `\n    at Child (${canaries.join(' | ')})` } as ErrorInfo

    boundary.componentDidCatch(error, info)
    boundary.state = AppErrorBoundary.getDerivedStateFromError(error)
    const html = renderToStaticMarkup(boundary.render())

    expect(logError).toHaveBeenCalledOnce()
    expect(logError).toHaveBeenCalledWith('renderer', 'Uncaught render error')
    expect(consoleError).not.toHaveBeenCalled()
    expect(boundary.state).toEqual({ hasError: true })
    expect(html).toContain('<button')
    for (const canary of canaries) {
      expect(JSON.stringify(logError.mock.calls)).not.toContain(canary)
      expect(JSON.stringify(consoleError.mock.calls)).not.toContain(canary)
      expect(JSON.stringify(boundary.state)).not.toContain(canary)
      expect(html).not.toContain(canary)
    }
  })
})
