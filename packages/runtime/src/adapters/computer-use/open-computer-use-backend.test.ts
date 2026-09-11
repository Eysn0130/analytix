import { describe, expect, it } from 'vitest'
import { join } from 'node:path'
import { selectHostControlBackend } from './backend-factory.js'
import { OpenComputerUseBackend, _internals } from './open-computer-use-backend.js'

function fakeMcp(calls: Array<{ name: string; arguments: Record<string, unknown> }>) {
  return {
    listTools: async () => ({
      tools: [
        { name: 'list_apps' },
        { name: 'get_app_state' },
        { name: 'click' },
        { name: 'set_value' }
      ]
    }),
    callTool: async (input: { name: string; arguments: Record<string, unknown> }) => {
      calls.push(input)
      if (input.name === 'list_apps') {
        return {
          content: [{ type: 'text', text: 'TextEdit — com.apple.TextEdit [running]\nFinder — com.apple.finder [frontmost, running]' }]
        }
      }
      if (input.name === 'get_app_state') {
        return {
          structuredContent: {
            app: { name: input.arguments.app ?? 'TextEdit', pid: 42 },
            window: { title: 'Untitled', bounds: { x: 0, y: 0, width: 800, height: 600 } },
            accessibilityTree: [{ role: 'button', name: 'OK', element_index: 7 }],
            elements: [{ role: 'button', name: 'OK', element_index: 7 }],
            screenshot: { mime_type: 'image/png', data_base64: 'PNGDATA', width: 800, height: 600 }
          }
        }
      }
      return { content: [{ type: 'text', text: JSON.stringify({ ok: true }) }] }
    },
    close: async () => undefined
  }
}

function pngHeaderBase64(width: number, height: number): string {
  const header = Buffer.alloc(24)
  Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]).copy(header, 0)
  header.writeUInt32BE(13, 8)
  header.write('IHDR', 12, 'ascii')
  header.writeUInt32BE(width, 16)
  header.writeUInt32BE(height, 20)
  return header.toString('base64')
}

describe('OpenComputerUseBackend', () => {
  it('prefers the packaged Windows native executable before npm command shims', () => {
    const candidates = _internals.openComputerUseCommandCandidates(undefined, {
      platform: 'win32',
      arch: 'x64',
      env: { ANALYTIX_APP_ROOT: '/app', PATH: '' },
      cwd: '/app',
      resourcesPath: '/installed/resources',
      execPath: '/installed/analytix.exe'
    })
    const native = join('/installed/resources', 'app.asar.unpacked', 'packages', 'runtime', 'node_modules', 'analytix-computer-use', 'dist', 'windows', 'amd64', 'analytix-computer-use.exe')
    const shim = join('/installed/resources', 'app.asar.unpacked', 'packages', 'runtime', 'node_modules', '.bin', 'analytix-computer-use.cmd')

    expect(candidates).toContain(native)
    expect(candidates).toContain(shim)
    expect(candidates.indexOf(native)).toBeLessThan(candidates.indexOf(shim))
    expect(candidates).not.toContain(join('/app', 'node_modules', '.bin', 'analytix-computer-use.cmd'))
    expect(candidates).not.toContain(join('/app', 'node_modules', '.bin', 'open-computer-use.cmd'))
    expect(candidates).not.toContain('analytix-computer-use')
    expect(candidates).not.toContain('open-computer-use')
  })

  it('prefers the native executable when a Windows npm shim is configured explicitly', () => {
    const shim = join('/app', 'packages', 'runtime', 'node_modules', '.bin', 'analytix-computer-use.cmd')
    const native = join('/app', 'packages', 'runtime', 'node_modules', 'analytix-computer-use', 'dist', 'windows', 'amd64', 'analytix-computer-use.exe')
    const candidates = _internals.openComputerUseCommandCandidates(shim, {
      platform: 'win32',
      arch: 'x64',
      env: { PATH: '' },
      cwd: '/app',
      resourcesPath: '',
      execPath: '/installed/analytix.exe'
    })

    expect(candidates).toContain(native)
    expect(candidates).toContain(shim)
    expect(candidates.indexOf(native)).toBeLessThan(candidates.indexOf(shim))
  })

  it('uses the MCP app state contract and preserves accessibility metadata', async () => {
    const calls: Array<{ name: string; arguments: Record<string, unknown> }> = []
    const backend = new OpenComputerUseBackend({
      command: 'analytix-computer-use',
      clientFactory: async () => fakeMcp(calls)
    })

    await expect(backend.ensureReady()).resolves.toEqual({ available: true })
    const apps = await backend.listApps()
    const state = await backend.getAppState('TextEdit')

    expect(apps).toEqual([
      { name: 'TextEdit', bundleId: 'com.apple.TextEdit', running: true, frontmost: false, flags: ['running'] },
      { name: 'Finder', bundleId: 'com.apple.finder', running: true, frontmost: true, flags: ['frontmost', 'running'] }
    ])
    expect(state.backendId).toBe('analytix-computer-use')
    expect(state.screenshot.dataBase64).toBe('PNGDATA')
    expect(state.accessibilityTree).toEqual([{ role: 'button', name: 'OK', element_index: 7 }])
    expect(state.elements).toEqual([{ role: 'button', name: 'OK', element_index: 7 }])
    expect(calls.find((call) => call.name === 'get_app_state')?.arguments).toMatchObject({ app: 'TextEdit' })
  })

  it('passes element_index through semantic actions', async () => {
    const calls: Array<{ name: string; arguments: Record<string, unknown> }> = []
    const backend = new OpenComputerUseBackend({
      command: 'analytix-computer-use',
      clientFactory: async () => fakeMcp(calls)
    })

    await backend.click({ app: 'TextEdit', elementIndex: 7 }, 'left', 1)
    await backend.setValue('done', { app: 'TextEdit', elementIndex: 7 })

    expect(calls.find((call) => call.name === 'click')?.arguments).toMatchObject({
      app: 'TextEdit',
      element_index: '7'
    })
    expect(calls.find((call) => call.name === 'set_value')?.arguments).toMatchObject({
      app: 'TextEdit',
      element_index: '7',
      value: 'done'
    })
  })

  it('derives screenshot dimensions from image bytes when MCP omits metadata', async () => {
    const backend = new OpenComputerUseBackend({
      command: 'analytix-computer-use',
      clientFactory: async () => ({
        listTools: async () => ({ tools: [{ name: 'list_apps' }, { name: 'get_app_state' }] }),
        callTool: async () => ({
          content: [
            { type: 'text', text: 'App=com.apple.TextEdit (pid 123)' },
            { type: 'image', data: pngHeaderBase64(640, 360), mimeType: 'image/png' }
          ]
        }),
        close: async () => undefined
      })
    })

    const state = await backend.getAppState('TextEdit')

    expect(state.screenshot.width).toBe(640)
    expect(state.screenshot.height).toBe(360)
  })

  it('parses text accessibility tree output from Analytix Computer Use', async () => {
    const calls: Array<{ name: string; arguments: Record<string, unknown> }> = []
    const backend = new OpenComputerUseBackend({
      command: 'analytix-computer-use',
      clientFactory: async () => ({
        listTools: async () => ({ tools: [{ name: 'list_apps' }, { name: 'get_app_state' }] }),
        callTool: async (input: { name: string; arguments: Record<string, unknown> }) => {
          calls.push(input)
          if (input.name === 'list_apps') return { content: [{ type: 'text', text: 'Finder — com.apple.finder [running]' }] }
          return {
            content: [
              {
                type: 'text',
                text: [
                  'App=com.apple.finder (pid 439)',
                  'Window: "Projects", App: 访达.',
                  '0 标准窗口 Projects ID: FinderWindow, Secondary Actions: Raise',
                  '\t1 分离组',
                  '\t\t2 滚动区 Secondary Actions: Scroll Up, Scroll Down',
                  '\t\t\t35 row 江苏航案件分析',
                  '\t\t\t41 文本栏 (settable, string) Value: analytix-hub, Secondary Actions: Open'
                ].join('\n')
              },
              { type: 'image', data: 'PNGDATA', mimeType: 'image/png', width: 800, height: 600 }
            ]
          }
        },
        close: async () => undefined
      })
    })

    const state = await backend.getAppState('访达')

    expect(state.app).toMatchObject({ name: '访达', bundleId: 'com.apple.finder', pid: 439 })
    expect(state.window).toMatchObject({ title: 'Projects', appName: '访达' })
    expect(state.accessibilityTree).toHaveLength(5)
    expect(state.elements).toEqual(state.accessibilityTree)
    expect(state.elements?.[4]).toMatchObject({
      element_index: 41,
      role: '文本栏',
      name: '',
      settable: true,
      metadata: { value: 'analytix-hub' }
    })
  })
})

describe('selectHostControlBackend', () => {
  it('selects Analytix Computer Use before nut-js when the semantic backend is ready', async () => {
    const calls: Array<{ name: string; arguments: Record<string, unknown> }> = []
    const selection = await selectHostControlBackend({
      openComputerUse: {
        command: 'analytix-computer-use',
        clientFactory: async () => fakeMcp(calls)
      }
    })

    expect(selection.preferredBackendId).toBe('analytix-computer-use')
    expect(selection.selectedBackendId).toBe('analytix-computer-use')
    expect(selection.readiness.available).toBe(true)
  })
})
