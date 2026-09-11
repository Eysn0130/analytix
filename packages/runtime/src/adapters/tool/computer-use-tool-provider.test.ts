import { describe, expect, it } from 'vitest'
import { buildComputerUseToolProviders } from '../../tool-test-support/tool/computer-use-tool-provider.js'
import type { HostAppState, HostControlBackend, HostControlTarget, HostScreenshot } from '../computer-use/host-control.js'
import type { ToolHostContext } from '../../ports/tool-host.js'

const SHOT: HostScreenshot = { mimeType: 'image/png', dataBase64: 'PNGDATA', width: 1280, height: 800 }
const STATE: HostAppState = {
  backendId: 'analytix-computer-use',
  app: { name: 'Finder', pid: 1 },
  window: { title: 'Home', bounds: { x: 0, y: 0, width: 1280, height: 800 } },
  screenshot: SHOT,
  accessibilityTree: [{ role: 'button', name: 'Open', element_index: 3 }],
  elements: [{ element_index: 3, role: 'button', name: 'Open' }]
}

function fakeController(overrides: Partial<Record<string, unknown>> = {}): {
  controller: HostControlBackend
  calls: string[]
} {
  const calls: string[] = []
  const controller = {
    id: 'analytix-computer-use',
    ensureReady: async () => ({ available: true }),
    capture: async () => SHOT,
    getAppState: async () => STATE,
    listApps: async () => [],
    screenSize: async () => ({ width: 1280, height: 800 }),
    cursorPosition: async () => ({ x: 10, y: 20 }),
    moveTo: async (x: number, y: number) => void calls.push(`move:${x},${y}`),
    click: async (target: HostControlTarget, button: string, count: number) =>
      void calls.push(`click:${target.elementIndex ?? `${target.x},${target.y}`},${button},${count}`),
    drag: async (start: HostControlTarget, end: HostControlTarget) =>
      void calls.push(`drag:${start.x},${start.y}-${end.x},${end.y}`),
    scroll: async (_target: HostControlTarget | undefined, direction: string, amount: number) =>
      void calls.push(`scroll:${direction},${amount}`),
    typeText: async (text: string) => void calls.push(`type:${text}`),
    pressHotkey: async (key: string) => void calls.push(`key:${key}`),
    pressKey: async (key: string) => void calls.push(`key:${key}`),
    setValue: async (value: string, target?: HostControlTarget) => void calls.push(`set:${target?.elementIndex ?? ''}:${value}`),
    wait: async (ms: number) => void calls.push(`wait:${ms}`),
    ...overrides
  } as unknown as HostControlBackend
  return { controller, calls }
}

function context(image: boolean): ToolHostContext {
  return {
    threadId: 'thread_1',
    turnId: 'turn_1',
    workspace: '/ws',
    approvalPolicy: 'auto',
    sandboxMode: 'danger-full-access',
    abortSignal: new AbortController().signal,
    awaitApproval: async () => 'allow',
    model: {
      id: 'm',
      inputModalities: image ? ['text', 'image'] : ['text'],
      outputModalities: ['text'],
      supportsToolCalling: true,
      messageParts: image ? ['text', 'image_url'] : ['text']
    }
  }
}

async function buildTool(configOverrides = {}, controllerOverrides = {}) {
  const { controller, calls } = fakeController(controllerOverrides)
  const result = await buildComputerUseToolProviders(
    { enabled: true, mode: 'auto', maxImageDimension: 1280, maxActionsPerTurn: 40, allowWhenLocked: false, ...configOverrides },
    { controller }
  )
  const tool = result.providers[0]?.tools[0]
  return { result, tool, calls }
}

describe('buildComputerUseToolProviders', () => {
  it('produces no providers when disabled or off', async () => {
    expect((await buildComputerUseToolProviders({
      enabled: false,
      mode: 'auto',
      maxImageDimension: 1280,
      maxActionsPerTurn: 40,
      allowWhenLocked: false
    })).providers).toHaveLength(0)
    expect((await buildComputerUseToolProviders({
      enabled: true,
      mode: 'off',
      maxImageDimension: 1280,
      maxActionsPerTurn: 40,
      allowWhenLocked: false
    })).providers).toHaveLength(0)
  })

  it('reports unavailable when the backend cannot load', async () => {
    const result = await buildComputerUseToolProviders(
      { enabled: true, mode: 'auto', maxImageDimension: 1280, maxActionsPerTurn: 40, allowWhenLocked: false },
      { controller: { ensureReady: async () => ({ available: false, reason: 'no backend' }) } as unknown as HostControlBackend }
    )
    expect(result.available).toBe(false)
    expect(result.providers[0]?.available).toBe(false)
    expect(result.providers[0]?.tools).toHaveLength(0)
  })

  it('auto mode advertises only to vision models', async () => {
    const { tool } = await buildTool()
    expect(tool?.shouldAdvertise?.(context(true))).toBe(true)
    expect(tool?.shouldAdvertise?.(context(false))).toBe(false)
  })

  it('auto mode advertises to text models when Vision Bridge is configured', async () => {
    const { controller } = fakeController()
    const result = await buildComputerUseToolProviders(
      { enabled: true, mode: 'auto', maxImageDimension: 1280, maxActionsPerTurn: 40, allowWhenLocked: false },
      {
        controller,
        visionBridge: {
          enabled: true,
          mode: 'auto',
          providerId: 'xiaomi',
          baseUrl: 'https://api.xiaomimimo.com/v1',
          apiKey: 'sk',
          endpointFormat: 'chat_completions',
          model: 'mimo-v2.5',
          maxImageDimension: 1280,
          maxImageBytes: 1500000,
          maxScreenshotsPerTurn: 4,
          observationCacheTtlMs: 120000,
          injectPolicy: 'observation_text',
          fallbackWhenPrimaryImageUnsupported: true,
          semanticProbeStatus: 'supported'
        }
      }
    )
    expect(result.providers[0]?.tools[0]?.shouldAdvertise?.(context(false))).toBe(true)
  })

  it('does not advertise Vision Bridge before semantic probe passes', async () => {
    const { controller } = fakeController()
    const result = await buildComputerUseToolProviders(
      { enabled: true, mode: 'auto', maxImageDimension: 1280, maxActionsPerTurn: 40, allowWhenLocked: false },
      {
        controller,
        visionBridge: {
          enabled: true,
          mode: 'auto',
          providerId: 'xiaomi',
          baseUrl: 'https://api.xiaomimimo.com/v1',
          apiKey: 'sk',
          endpointFormat: 'chat_completions',
          model: 'mimo-v2.5',
          maxImageDimension: 1280,
          maxImageBytes: 1500000,
          maxScreenshotsPerTurn: 4,
          observationCacheTtlMs: 120000,
          injectPolicy: 'observation_text',
          fallbackWhenPrimaryImageUnsupported: true,
          semanticProbeStatus: 'unknown'
        }
      }
    )
    expect(result.providers[0]?.tools[0]?.shouldAdvertise?.(context(false))).toBe(false)
  })

  it('does not advertise when the active model cannot use tool calling', async () => {
    const { tool } = await buildTool({ mode: 'always' })
    const ctx = context(true)
    ctx.model = { ...ctx.model!, supportsToolCalling: false }
    expect(tool?.shouldAdvertise?.(ctx)).toBe(false)
  })

  it('always mode advertises regardless of modality', async () => {
    const { tool } = await buildTool({ mode: 'always' })
    expect(tool?.shouldAdvertise?.(context(false))).toBe(true)
  })
})

describe('computer_use execution', () => {
  it('returns a screenshot image for screenshot action', async () => {
    const { tool } = await buildTool()
    const out = await tool!.execute({ action: 'screenshot' }, context(true)) as {
      output: { kind: string; images: { data_base64: string }[] }
    }
    expect(out.output.kind).toBe('computer_screenshot')
    expect(out.output.images[0]?.data_base64).toBe('PNGDATA')
  })

  it('clicks at a coordinate then returns a fresh screenshot', async () => {
    const { tool, calls } = await buildTool()
    const out = await tool!.execute({ action: 'left_click', coordinate: [100, 200] }, context(true)) as {
      output: { kind: string }
    }
    expect(calls).toContain('click:100,200,left,1')
    expect(out.output.kind).toBe('computer_screenshot')
  })

  it('returns app state with accessibility tree and backend metadata', async () => {
    const { tool } = await buildTool()
    const out = await tool!.execute({ action: 'get_app_state', app: 'Finder' }, context(true)) as {
      output: { kind: string; backendId: string; accessibilityTree: unknown[]; elements: unknown[]; images: { data_base64: string }[] }
    }
    expect(out.output.kind).toBe('computer_app_state')
    expect(out.output.backendId).toBe('analytix-computer-use')
    expect(out.output.accessibilityTree).toHaveLength(1)
    expect(out.output.elements).toHaveLength(1)
    expect(out.output.images[0]?.data_base64).toBe('PNGDATA')
  })

  it('routes type / key / scroll / drag / double_click', async () => {
    const { tool, calls } = await buildTool()
    const ctx = context(true)
    await tool!.execute({ action: 'type', text: 'hi' }, ctx)
    await tool!.execute({ action: 'press_key', text: 'ctrl+c' }, ctx)
    await tool!.execute({ action: 'scroll', coordinate: [5, 5], scroll_direction: 'down', scroll_amount: 2 }, ctx)
    await tool!.execute({ action: 'drag', start_coordinate: [1, 2], coordinate: [3, 4] }, ctx)
    await tool!.execute({ action: 'set_value', element_index: 3, value: 'next' }, ctx)
    await tool!.execute({ action: 'double_click', coordinate: [7, 8] }, ctx)
    expect(calls).toEqual(expect.arrayContaining([
      'type:hi',
      'key:ctrl+c',
      'scroll:down,2',
      'drag:1,2-3,4',
      'set:3:next',
      'click:7,8,left,2'
    ]))
  })

  it('enforces the per-turn action budget', async () => {
    const { tool } = await buildTool({ maxActionsPerTurn: 2 })
    const ctx = context(true)
    await tool!.execute({ action: 'screenshot' }, ctx)
    await tool!.execute({ action: 'screenshot' }, ctx)
    const third = await tool!.execute({ action: 'screenshot' }, ctx) as {
      isError?: boolean
      output: { error?: string }
    }
    expect(third.isError).toBe(true)
    expect(third.output.error).toBe('action_budget_exhausted')
  })
})
