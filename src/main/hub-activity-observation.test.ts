import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  observeHubActivity,
  recordHubActivity,
  resetHubActivityForTests
} from './hub-activity-observation'

describe('main-private Hub activity observation', () => {
  beforeEach(() => {
    resetHubActivityForTests()
  })

  it('starts with closed zero counters and booleans only', () => {
    const observation = observeHubActivity()

    expect(observation).toEqual({
      schemaVersion: 1,
      moduleLoad: 0,
      serviceInstance: 0,
      refreshTimer: 0,
      tokenRead: 0,
      request: 0,
      fallback: 0,
      anyActivity: false
    })
    expect(Object.values(observation).every((value) =>
      typeof value === 'number' || typeof value === 'boolean'
    )).toBe(true)
  })

  it('records only the six closed activity dimensions', () => {
    recordHubActivity('moduleLoad')
    recordHubActivity('serviceInstance')
    recordHubActivity('refreshTimer')
    recordHubActivity('tokenRead')
    recordHubActivity('request')
    recordHubActivity('fallback')
    recordHubActivity('request')

    expect(observeHubActivity()).toEqual({
      schemaVersion: 1,
      moduleLoad: 1,
      serviceInstance: 1,
      refreshTimer: 1,
      tokenRead: 1,
      request: 2,
      fallback: 1,
      anyActivity: true
    })
  })

  it('emits no values unless the explicit observation switch is enabled', async () => {
    const info = vi.spyOn(console, 'info').mockImplementation(() => undefined)
    recordHubActivity('moduleLoad')

    expect(info).not.toHaveBeenCalled()

    process.env.ANALYTIX_HUB_ACTIVITY_OBSERVATION = '1'
    vi.resetModules()
    const enabled = await import('./hub-activity-observation')
    enabled.resetHubActivityForTests()
    enabled.recordHubActivity('moduleLoad')

    expect(info).toHaveBeenCalledTimes(1)
    const [marker, payload] = info.mock.calls[0]
    expect(marker).toBe('[hub-activity-observation]')
    expect(JSON.parse(String(payload))).toEqual({
      stage: 'activity_changed',
      schemaVersion: 1,
      moduleLoad: 1,
      serviceInstance: 0,
      refreshTimer: 0,
      tokenRead: 0,
      request: 0,
      fallback: 0,
      anyActivity: true
    })

    delete process.env.ANALYTIX_HUB_ACTIVITY_OBSERVATION
    info.mockRestore()
  })
})
