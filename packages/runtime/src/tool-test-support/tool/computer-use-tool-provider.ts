import type { AnalytixCapabilitiesConfig } from '../../contracts/capabilities.js'
import type { ToolHostContext } from '../../ports/tool-host.js'
import type { CapabilityToolProvider } from './capability-registry.js'
import { LocalToolHost } from './local-tool-host.js'
import {
  type HostAppState,
  type HostControlBackend,
  type HostControlTarget,
  type HostScreenshot,
  type MouseButton,
  type ScrollDirection
} from '../../adapters/computer-use/host-control.js'
import {
  selectHostControlBackend,
  type HostControlBackendSelection
} from '../../adapters/computer-use/backend-factory.js'

export type ComputerUseToolProviderOptions = {
  controller?: HostControlBackend
  visionBridge?: AnalytixCapabilitiesConfig['visionBridge']
}

export type ComputerUseToolProviderDiagnostic = {
  id: 'computerUse'
  enabled: boolean
  available: boolean
  backendId?: string
  preferredBackendId?: string
  fallbackReason?: string
  reason?: string
}

export type ComputerUseToolProviderBuildResult = {
  providers: CapabilityToolProvider[]
  diagnostics: ComputerUseToolProviderDiagnostic[]
  available: boolean
  reason?: string
}

const COMPUTER_USE_ACTIONS = [
  'get_app_state',
  'list_apps',
  'click',
  'perform_secondary_action',
  'drag',
  'type_text',
  'press_key',
  'set_value',
  'screenshot',
  'cursor_position',
  'mouse_move',
  'left_click',
  'right_click',
  'middle_click',
  'double_click',
  'left_click_drag',
  'scroll',
  'type',
  'key',
  'wait'
] as const

const TOOL_DESCRIPTION = [
  'Control the host computer through screenshots and synthesized mouse/keyboard input.',
  'Workflow: call `get_app_state` or `screenshot`, reason about what is on screen, then act, then screenshot again to verify.',
  'Prefer element_index or semantic actions when get_app_state reports them; use coordinates as a fallback.',
  'Coordinates are pixel positions in the MOST RECENT screenshot you took; the screenshot result reports width and height.',
  'Always screenshot before clicking the first time so you know the current resolution and layout.',
  'Use `key` for shortcuts/special keys such as "ctrl+c", "Return", or "Escape"; use `type` for literal text.',
  'This drives the real desktop: act deliberately, prefer the smallest action that makes progress, and stop once the task is done.'
].join(' ')

const INPUT_SCHEMA = {
  type: 'object',
  properties: {
    action: {
      type: 'string',
      enum: [...COMPUTER_USE_ACTIONS],
      description:
        'get_app_state/list_apps/click/type_text/press_key/set_value style actions, or legacy screenshot/cursor_position/mouse_move/click/drag/scroll/type/key/wait.'
    },
    element_index: {
      type: 'number',
      description:
        'Preferred semantic target from get_app_state accessibility tree. Requires a semantic backend such as Analytix Computer Use; coordinates are the fallback.'
    },
    app: {
      type: 'string',
      description: 'Optional application/window name for semantic backends, such as TextEdit, Finder, Notepad, or Explorer.'
    },
    coordinate: {
      type: 'array',
      items: { type: 'number' },
      minItems: 2,
      maxItems: 2,
      description: '[x, y] target in screenshot pixels.'
    },
    start_coordinate: {
      type: 'array',
      items: { type: 'number' },
      minItems: 2,
      maxItems: 2,
      description: '[x, y] start point in screenshot pixels for left_click_drag.'
    },
    text: {
      type: 'string',
      description:
        'For `type`/`type_text`/`set_value`: literal text. For `key`/`press_key`: a key or + separated chord. For click actions: optional modifier keys.'
    },
    value: {
      type: 'string',
      description: 'Value for set_value; falls back to text when omitted.'
    },
    scroll_direction: {
      type: 'string',
      enum: ['up', 'down', 'left', 'right']
    },
    scroll_amount: {
      type: 'number',
      description: 'Number of wheel clicks for scroll (default 3).'
    },
    duration: {
      type: 'number',
      description: 'Seconds to pause for wait (default 1, max 60).'
    }
  },
  required: ['action'],
  additionalProperties: false
} as const

function toolError(code: string, message: string): { output: unknown; isError: true } {
  return { output: { kind: 'computer_action', error: code, message }, isError: true }
}

function readCoordinate(value: unknown): [number, number] | undefined {
  if (!Array.isArray(value) || value.length < 2) return undefined
  const x = Number(value[0])
  const y = Number(value[1])
  if (!Number.isFinite(x) || !Number.isFinite(y)) return undefined
  return [Math.round(x), Math.round(y)]
}

function modifiersFromText(value: unknown): string[] {
  if (typeof value !== 'string' || !value.trim()) return []
  return value
    .split(/[\s+]+/)
    .map((part) => part.trim())
    .filter(Boolean)
}

function readElementIndex(value: unknown): number | undefined {
  const num = Number(value)
  return Number.isInteger(num) && num >= 0 ? num : undefined
}

function readApp(value: unknown): string | undefined {
  return typeof value === 'string' && value.trim() ? value.trim() : undefined
}

function targetFromArgs(args: Record<string, unknown>, coordinate?: [number, number]): HostControlTarget {
  return {
    ...(readApp(args.app) ? { app: readApp(args.app) } : {}),
    ...(readElementIndex(args.element_index) !== undefined ? { elementIndex: readElementIndex(args.element_index) } : {}),
    ...(coordinate ? { x: coordinate[0], y: coordinate[1] } : {})
  }
}

function screenshotOutput(action: string, shot: HostScreenshot, note?: string): { output: unknown } {
  return {
    output: {
      kind: 'computer_screenshot',
      action,
      screen: { width: shot.width, height: shot.height },
      note:
        note ??
        `Screenshot is ${shot.width}x${shot.height}px. Coordinates for the next action use this pixel space (top-left is 0,0).`,
      images: [
        {
          mime_type: shot.mimeType,
          data_base64: shot.dataBase64,
          width: shot.width,
          height: shot.height
        }
      ]
    }
  }
}

function appStateOutput(action: string, state: HostAppState): { output: unknown } {
  return {
    output: {
      kind: 'computer_app_state',
      action,
      backendId: state.backendId,
      screen: { width: state.screenshot.width, height: state.screenshot.height },
      coordinateSpace: {
        origin: 'top-left',
        width: state.screenshot.width,
        height: state.screenshot.height
      },
      ...(state.app !== undefined ? { app: state.app } : {}),
      ...(state.window !== undefined ? { window: state.window } : {}),
      accessibilityTree: state.accessibilityTree ?? [],
      elements: state.elements ?? [],
      ...(state.warnings?.length ? { warnings: state.warnings } : {}),
      images: [
        {
          mime_type: state.screenshot.mimeType,
          data_base64: state.screenshot.dataBase64,
          width: state.screenshot.width,
          height: state.screenshot.height
        }
      ]
    }
  }
}

function actionOutput(action: string, backendId: string, shot: HostScreenshot): { output: unknown } {
  const base = screenshotOutput(action, shot).output as Record<string, unknown>
  return { output: { ...base, backendId } }
}

function visionBridgeReady(config: AnalytixCapabilitiesConfig['visionBridge'] | undefined): boolean {
  if (!config?.enabled || config.mode === 'off') return false
  if (!config.baseUrl?.trim() || !config.model?.trim()) return false
  if (requiresApiKey(config.baseUrl) && !config.apiKey?.trim()) return false
  return config.semanticProbeStatus === 'supported'
}

function requiresApiKey(baseUrl: string): boolean {
  return !/^https?:\/\/(?:localhost|127\.0\.0\.1|\[::1\])(?::\d+)?(?:\/|$)/i.test(baseUrl.trim())
}

export async function buildComputerUseToolProviders(
  config: AnalytixCapabilitiesConfig['computerUse'] | undefined,
  options: ComputerUseToolProviderOptions = {}
): Promise<ComputerUseToolProviderBuildResult> {
  if (!config?.enabled || config.mode === 'off') {
    return { providers: [], diagnostics: [], available: false }
  }

  const selection: HostControlBackendSelection = options.controller
    ? {
        backend: options.controller,
        readiness: await options.controller.ensureReady(),
        preferredBackendId: options.controller.id,
        selectedBackendId: options.controller.id
      }
    : await selectHostControlBackend({
        maxImageDimension: config.maxImageDimension,
        maxImageBytes: options.visionBridge?.maxImageBytes
      })
  const controller = selection.backend
  const readiness = selection.readiness
  if (!readiness.available) {
    const reason = readiness.reason ?? 'computer-use backend is unavailable'
    return {
      providers: [{ id: 'computerUse', kind: 'gui', enabled: true, available: false, reason, tools: [] }],
      diagnostics: [{
        id: 'computerUse',
        enabled: true,
        available: false,
        backendId: selection.selectedBackendId,
        preferredBackendId: selection.preferredBackendId,
        fallbackReason: selection.fallbackReason,
        reason
      }],
      available: false,
      reason
    }
  }

  const mode = config.mode
  const maxActionsPerTurn = config.maxActionsPerTurn
  const actionsByTurn = new Map<string, number>()

  const bridgeAvailable = visionBridgeReady(options.visionBridge)

  const shouldAdvertise = (context: ToolHostContext): boolean => {
    if (context.model?.supportsToolCalling === false) return false
    if (mode === 'always') return true
    return Boolean(context.model?.inputModalities?.includes('image') || bridgeAvailable)
  }

  const tool = LocalToolHost.defineTool({
    name: 'computer_use',
    description: TOOL_DESCRIPTION,
    inputSchema: INPUT_SCHEMA as unknown as Record<string, unknown>,
    toolKind: 'command_execution',
    policy: 'on-request',
    shouldAdvertise,
    execute: async (args, context) => {
      const action = typeof args.action === 'string' ? args.action : ''
      if (!action) return toolError('invalid_action', 'action is required')

      const turnKey = `${context.threadId}:${context.turnId}`
      const used = actionsByTurn.get(turnKey) ?? 0
      if (used >= maxActionsPerTurn) {
        return toolError(
          'action_budget_exhausted',
          `reached the computer_use action limit (${maxActionsPerTurn}) for this turn; summarize progress or ask the user how to proceed`
        )
      }
      actionsByTurn.set(turnKey, used + 1)
      if (actionsByTurn.size > 64) {
        for (const key of actionsByTurn.keys()) {
          if (key !== turnKey) {
            actionsByTurn.delete(key)
            break
          }
        }
      }

      if (context.abortSignal.aborted) {
        return toolError('aborted', 'the turn was cancelled before this action ran')
      }

      const ready = await controller.ensureReady()
      if (!ready.available) {
        return toolError('computer_use_unavailable', ready.reason ?? 'computer-use backend is unavailable')
      }

      const coordinate = readCoordinate(args.coordinate)
      try {
        switch (action) {
          case 'get_app_state': {
            const state = await controller.getAppState(readApp(args.app))
            return appStateOutput('get_app_state', state)
          }

          case 'list_apps':
            return {
              output: {
                kind: 'computer_apps',
                backendId: controller.id,
                apps: await controller.listApps()
              }
            }

          case 'screenshot':
            return actionOutput('screenshot', controller.id, await controller.capture(readApp(args.app)))

          case 'cursor_position': {
            const pos = await controller.cursorPosition()
            const size = await controller.screenSize()
            return {
              output: {
                kind: 'computer_action',
                action,
                backendId: controller.id,
                cursor: [pos.x, pos.y],
                screen: size
              }
            }
          }

          case 'mouse_move': {
            if (!coordinate) return toolError('missing_coordinate', 'mouse_move requires coordinate [x,y]')
            await controller.moveTo(coordinate[0], coordinate[1])
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'click':
          case 'left_click':
          case 'right_click':
          case 'middle_click':
          case 'double_click': {
            const button: MouseButton =
              action === 'right_click'
                ? 'right'
                : action === 'middle_click'
                  ? 'middle'
                  : 'left'
            const count = action === 'double_click' ? 2 : 1
            await controller.click(
              targetFromArgs(args, coordinate),
              button,
              count,
              modifiersFromText(args.text)
            )
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'perform_secondary_action': {
            const secondaryAction = typeof args.text === 'string' && args.text.trim()
              ? args.text.trim()
              : typeof args.value === 'string' && args.value.trim()
                ? args.value.trim()
                : ''
            if (!secondaryAction) return toolError('missing_action', 'perform_secondary_action requires text or value with the secondary action name')
            if (typeof controller.performSecondaryAction !== 'function') {
              return toolError('unsupported_action', 'selected computer_use backend does not support semantic secondary actions')
            }
            await controller.performSecondaryAction(targetFromArgs(args, coordinate), secondaryAction)
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'drag':
          case 'left_click_drag': {
            const start = readCoordinate(args.start_coordinate)
            if (!start || !coordinate) {
              return toolError('missing_coordinate', 'left_click_drag requires start_coordinate and coordinate')
            }
            await controller.drag(targetFromArgs(args, start), targetFromArgs(args, coordinate))
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'scroll': {
            const direction = typeof args.scroll_direction === 'string' ? args.scroll_direction : ''
            if (!['up', 'down', 'left', 'right'].includes(direction)) {
              return toolError('missing_scroll_direction', 'scroll requires scroll_direction (up/down/left/right)')
            }
            const amount = Number.isFinite(Number(args.scroll_amount)) ? Number(args.scroll_amount) : 3
            await controller.scroll(targetFromArgs(args, coordinate), direction as ScrollDirection, amount)
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'type':
          case 'type_text': {
            const text = typeof args.text === 'string' ? args.text : ''
            if (!text) return toolError('missing_text', 'type requires text')
            await controller.typeText(text, targetFromArgs(args, coordinate))
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'key':
          case 'press_key': {
            const text = typeof args.text === 'string' ? args.text : ''
            if (!text) return toolError('missing_text', 'key requires text')
            await controller.pressKey(text, targetFromArgs(args, coordinate))
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'set_value': {
            const value = typeof args.value === 'string'
              ? args.value
              : typeof args.text === 'string'
                ? args.text
                : ''
            if (!value) return toolError('missing_text', 'set_value requires value or text')
            await controller.setValue(value, targetFromArgs(args, coordinate))
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          case 'wait': {
            const seconds = Number.isFinite(Number(args.duration)) ? Number(args.duration) : 1
            await controller.wait(Math.max(0, seconds) * 1000, context.abortSignal)
            return actionOutput(action, controller.id, await controller.capture(readApp(args.app)))
          }

          default:
            return toolError('unsupported_action', `unsupported computer_use action: ${action}`)
        }
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error)
        return toolError(
          'execution_failed',
          `computer_use ${action} failed: ${message}. On macOS this usually means Screen Recording and Accessibility permission have not been granted to the app.`
        )
      }
    }
  })

  return {
    providers: [{ id: 'computerUse', kind: 'gui', enabled: true, available: true, tools: [tool] }],
    diagnostics: [{
      id: 'computerUse',
      enabled: true,
      available: true,
      backendId: selection.selectedBackendId,
      preferredBackendId: selection.preferredBackendId,
      fallbackReason: selection.fallbackReason
    }],
    available: true
  }
}
