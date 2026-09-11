import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import {
  buildAnalytixServeArgs,
  resolveAnalytixExecutable,
  type AnalytixBinaryResolution
} from './resolve-analytix-binary'

const tempRoots: string[] = []

function tempRoot(): string {
  const root = mkdtempSync(join(tmpdir(), 'analytix-resolver-'))
  tempRoots.push(root)
  return root
}

function touch(path: string): void {
  mkdirSync(join(path, '..'), { recursive: true })
  writeFileSync(path, '', 'utf8')
}

afterEach(() => {
  while (tempRoots.length > 0) {
    const root = tempRoots.pop()
    if (root) rmSync(root, { recursive: true, force: true })
  }
})

describe('resolveAnalytixExecutable', () => {
  it('resolves the built Analytix entry from the app root', () => {
    const root = tempRoot()
    const entry = join(root, 'packages/runtime/dist/cli/serve-entry.js')
    touch(entry)

    const resolution = resolveAnalytixExecutable(root, '')

    expect(resolution).toEqual({
      kind: 'node-script',
      command: process.execPath,
      args: [entry],
      dataDir: ''
    })
  })

  it('does not fall back to TypeScript source files that Node cannot execute', () => {
    const root = tempRoot()
    touch(join(root, 'analytix/src/cli/serve-entry.ts'))

    const resolution = resolveAnalytixExecutable(root, '')

    expect(resolution).toEqual({
      kind: 'node-script',
      command: process.execPath,
      args: [join(root, 'packages/runtime/dist/cli/serve-entry.js')],
      dataDir: ''
    })
  })

  it('ignores retired custom package directories and keeps the bundled Go launcher', () => {
    const root = tempRoot()
    const appRoot = tempRoot()
    const bundledEntry = join(appRoot, 'packages/runtime/dist/cli/serve-entry.js')
    touch(join(root, 'dist/cli/serve-entry.js'))
    touch(bundledEntry)

    const resolution = resolveAnalytixExecutable(appRoot, root)

    expect(resolution).toEqual({
      kind: 'node-script',
      command: process.execPath,
      args: [bundledEntry],
      dataDir: ''
    })
  })

  it('ignores retired custom executables and keeps the bundled Go launcher candidate', () => {
    const resolution = resolveAnalytixExecutable('/app', '/usr/local/bin/analytix')

    expect(resolution).toEqual({
      kind: 'node-script',
      command: process.execPath,
      args: ['/app/packages/runtime/dist/cli/serve-entry.js'],
      dataDir: ''
    })
  })
})

describe('buildAnalytixServeArgs', () => {
  it('does not place runtime secrets on the child process argv', () => {
    const resolution: AnalytixBinaryResolution = {
      kind: 'node-script',
      command: '/usr/bin/node',
      args: ['/app/packages/runtime/dist/cli/serve-entry.js'],
      dataDir: ''
    }

    const args = buildAnalytixServeArgs({
      resolution,
      host: '127.0.0.1',
      port: 8899,
      dataDir: '/tmp/analytix',
      baseUrl: 'https://api.deepseek.com/beta',
      modelProxyUrl: 'socks5://proxy-user:proxy-secret@127.0.0.1:7890',
      mcpProxyUrl: 'socks5://proxy-user:proxy-secret@127.0.0.1:7890',
      endpointFormat: 'responses',
      model: 'deepseek-chat',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      tokenEconomyMode: false,
      insecure: false
    })

    expect(args).not.toContain('--api-key')
    expect(args).not.toContain('--runtime-token')
    expect(args).toContain('--endpoint-format')
    expect(args).toContain('responses')
    expect(args).toContain('--model-proxy-url')
    expect(args).toContain('--mcp-proxy-url')
    expect(args).toContain('--token-economy-mode')
    expect(args).toContain('false')
  })
})
