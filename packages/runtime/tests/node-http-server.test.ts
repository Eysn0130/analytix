import { existsSync } from 'node:fs'
import { readFile } from 'node:fs/promises'
import { describe, expect, it } from 'vitest'

describe('Node HTTP server', () => {
  it('keeps the retired TypeScript Node serve wrapper deleted', async () => {
    const nodeHttpServer = new URL('../src/server/node-http-server.ts', import.meta.url)
    const serveEntry = await readFile(new URL('../src/cli/serve-entry.ts', import.meta.url), 'utf8')

    expect(existsSync(nodeHttpServer)).toBe(false)
    expect(serveEntry).toContain('startGoRuntimeServe')
    expect(serveEntry).not.toContain('startNodeHttpServer')
  })
})
