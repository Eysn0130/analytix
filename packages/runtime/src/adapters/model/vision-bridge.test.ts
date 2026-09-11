import { describe, expect, it } from 'vitest'
import { parseVisionBridgeObservation } from '../../shared/vision-bridge.js'

describe('Vision Bridge observation parsing', () => {
  it('normalizes strict JSON observations', () => {
    const observation = parseVisionBridgeObservation(JSON.stringify({
      screen_summary: 'Desktop with a dialog',
      visible_text: ['Cancel', 'OK'],
      interactive_elements: [{
        label: 'OK',
        kind: 'button',
        element_index: 2,
        bbox: [10, 20, 60, 40],
        state: null,
        confidence: 0.95
      }],
      spatial_notes: ['OK is bottom right'],
      recommended_next_action: 'click OK',
      uncertainties: [],
      confidence: 0.9
    }))

    expect(observation.screen_summary).toBe('Desktop with a dialog')
    expect(observation.interactive_elements[0]).toMatchObject({
      label: 'OK',
      element_index: 2,
      bbox: [10, 20, 60, 40],
      confidence: 0.95
    })
  })

  it('degrades non-JSON model output into a low-confidence observation', () => {
    const observation = parseVisionBridgeObservation('I can see a settings page but forgot JSON.')

    expect(observation.confidence).toBe(0)
    expect(observation.screen_summary).toContain('settings page')
    expect(observation.uncertainties).toContain('vision model did not return valid JSON')
  })
})
