import { describe, expect, it } from 'vitest'
import {
  buildClawScheduledTaskDetectionPrompt,
  detectClawScheduledTaskRequest,
  looksLikeClawScheduledTaskCandidate
} from './claw-scheduled-task-detector'

describe('detectClawScheduledTaskRequest', () => {
  it('builds key-free detector intent without reading Provider settings', () => {
    expect(looksLikeClawScheduledTaskCandidate('remind me tomorrow to stretch')).toBe(true)
    expect(looksLikeClawScheduledTaskCandidate('hello there')).toBe(false)
    const prompt = buildClawScheduledTaskDetectionPrompt(
      'remind me tomorrow to stretch',
      new Date('2026-06-09T12:00:00+08:00')
    )
    expect(prompt).toContain('remind me tomorrow to stretch')
    expect(prompt).not.toMatch(/apiKey|credentialRef|Authorization|baseUrl|endpointFormat/)
  })

  it('parses only the Go runtime assistant response and owns no direct network path', async () => {
    await expect(detectClawScheduledTaskRequest(
      'remind me tomorrow to stretch',
      JSON.stringify({
        shouldCreateTask: true,
        scheduleAt: '2026-06-10T09:00:00+08:00',
        reminderBody: 'stretch',
        taskName: 'Stretch'
      }),
      new Date('2026-06-09T12:00:00+08:00')
    )).resolves.toMatchObject({
      reminderBody: 'stretch',
      scheduleAt: '2026-06-10T09:00:00+08:00'
    })
  })
})
