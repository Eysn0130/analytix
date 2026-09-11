import { describe, expect, it } from 'vitest'
import type { ModelHistoryItem, PrivateModelToolResultItem } from '../ports/model-client.js'
import {
  capToolResultImages,
  extractToolResultImages,
  isModelVisibleImageOutput,
  toolResultTextWithoutImages
} from '../shared/tool-result-image.js'

function toolResult(id: string, output: unknown): PrivateModelToolResultItem {
  return {
    id,
    turnId: 'turn_1',
    threadId: 'thread_1',
    role: 'tool',
    status: 'completed',
    createdAt: '2026-01-01T00:00:00.000Z',
    kind: 'tool_result',
    toolName: 'computer_use',
    callId: id,
    toolKind: 'command_execution',
    output,
    isError: false
  }
}

function screenshot(data: string): Record<string, unknown> {
  return {
    kind: 'computer_screenshot',
    action: 'screenshot',
    screen: { width: 1280, height: 800 },
    images: [{ mime_type: 'image/png', data_base64: data, width: 1280, height: 800 }]
  }
}

function appState(data: string): Record<string, unknown> {
  return {
    kind: 'computer_app_state',
    action: 'get_app_state',
    screen: { width: 1280, height: 800 },
    coordinateSpace: { origin: 'top-left', width: 1280, height: 800 },
    accessibilityTree: [],
    elements: [],
    images: [{ mime_type: 'image/png', data_base64: data, width: 1280, height: 800 }]
  }
}

describe('tool result images', () => {
  it('extracts read-tool and computer-use image shapes', () => {
    expect(
      extractToolResultImages({
        kind: 'image',
        mime_type: 'image/png',
        data_base64: 'AAA',
        width: 10,
        height: 20
      })
    ).toEqual([{ mimeType: 'image/png', dataBase64: 'AAA', width: 10, height: 20 }])
    expect(extractToolResultImages(screenshot('BBB'))).toEqual([
      { mimeType: 'image/png', dataBase64: 'BBB', width: 1280, height: 800 }
    ])
    expect(extractToolResultImages(appState('CCC'))).toEqual([
      { mimeType: 'image/png', dataBase64: 'CCC', width: 1280, height: 800 }
    ])
  })

  it('drops image payloads from text fallbacks but preserves metadata', () => {
    const text = toolResultTextWithoutImages(screenshot('HUGE_BASE64_PAYLOAD'))
    expect(text).not.toContain('HUGE_BASE64_PAYLOAD')
    expect(text).toContain('computer_screenshot')
    expect(text).toContain('1280')
  })

  it('keeps only the most recent image results inline', () => {
    const history: ModelHistoryItem[] = [
      toolResult('a', screenshot('IMG_A')),
      toolResult('b', screenshot('IMG_B')),
      toolResult('c', screenshot('IMG_C')),
      toolResult('d', screenshot('IMG_D'))
    ]

    const capped = capToolResultImages(history, 2)
    const kept = capped.filter(
      (item) => item.kind === 'tool_result' && isModelVisibleImageOutput(item.output)
    )
    expect(kept).toHaveLength(2)
    expect(isModelVisibleImageOutput((capped[0] as { output: unknown }).output)).toBe(false)
    expect(isModelVisibleImageOutput((capped[1] as { output: unknown }).output)).toBe(false)
    expect(extractToolResultImages((capped[3] as { output: unknown }).output)[0]?.dataBase64).toBe(
      'IMG_D'
    )
  })
})
