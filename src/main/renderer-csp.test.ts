import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('renderer content security policy', () => {
  it('allows blob image URLs for local attachment previews', () => {
    const html = readFileSync(resolve('src/renderer/index.html'), 'utf8')
    const csp = html.match(/Content-Security-Policy"[\s\S]*?content="([^"]+)"/)?.[1] ?? ''
    const imgSrc = csp.match(/img-src\s+([^;]+)/)?.[1] ?? ''

    expect(imgSrc.split(/\s+/)).toContain('blob:')
  })

  it('allows official Hub avatar uploads in renderer images', () => {
    const html = readFileSync(resolve('src/renderer/index.html'), 'utf8')
    const csp = html.match(/Content-Security-Policy"[\s\S]*?content="([^"]+)"/)?.[1] ?? ''
    const imgSrc = csp.match(/img-src\s+([^;]+)/)?.[1] ?? ''
    const sources = imgSrc.split(/\s+/)

    expect(sources).toEqual(
      expect.arrayContaining([
        'https://analytix.top',
        'http://127.0.0.1:*',
        'http://localhost:*',
      ])
    )
  })

  it('allows loopback API and websocket connections for local runtimes', () => {
    const html = readFileSync(resolve('src/renderer/index.html'), 'utf8')
    const csp = html.match(/Content-Security-Policy"[\s\S]*?content="([^"]+)"/)?.[1] ?? ''
    const connectSrc = csp.match(/connect-src\s+([^;]+)/)?.[1] ?? ''
    const sources = connectSrc.split(/\s+/)

    expect(sources).toEqual(
      expect.arrayContaining([
        "'self'",
        'http://127.0.0.1:*',
        'ws://127.0.0.1:*',
        'http://localhost:*',
        'ws://localhost:*',
      ])
    )
  })
})
