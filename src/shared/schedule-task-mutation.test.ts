import { describe, expect, it } from 'vitest'
import { normalizeAppSettings, type AppSettingsV1 } from './app-settings'
import { buildScheduleTaskUpdate, ordinarySettingsPatch, rejectScheduleTaskArrayWrite, scheduleTaskEditable, scheduleTaskEditMatches } from './schedule-task-mutation'

const settings = normalizeAppSettings({ schedule: { tasks: [{ id: 'actual-task', title: 'Before', prompt: 'Synthetic prompt',
  schedule: { kind: 'interval', everyMinutes: 15 } }] } } as AppSettingsV1)
const baseline = settings.schedule.tasks[0]

describe('closed schedule editable contract', () => {
  it('compares title edits against title while preserving host progress and one-time disabling', () => {
    const request = buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), title: 'After' })
    expect(request).toEqual({ taskId: baseline.id, patch: { title: 'After' }, expected: { title: 'Before' } })
    expect(scheduleTaskEditMatches({ ...baseline, enabled: false, lastThreadId: 'actual-thread', updatedAt: 'host-progress' }, request.expected)).toBe(true)
    expect(scheduleTaskEditMatches({ ...baseline, title: 'Other edit' }, request.expected)).toBe(false)
  })

  it('compares related scheduling and model configuration groups without runtime fields', () => {
    const enabled = buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), enabled: false })
    expect(enabled.expected).toEqual({ enabled: true, schedule: baseline.schedule })
    const model = buildScheduleTaskUpdate(baseline, { ...scheduleTaskEditable(baseline), model: 'another-model' })
    expect(Object.keys(model.expected).sort()).toEqual(['model', 'providerId', 'reasoningEffort'])
    expect(scheduleTaskEditMatches({ ...baseline, schedule: { ...baseline.schedule, everyMinutes: 60 } }, enabled.expected)).toBe(false)
  })

  it('projects ordinary settings writes without tasks and rejects old task-array entries', () => {
    const projected = ordinarySettingsPatch(settings)
    expect(projected.schedule).not.toHaveProperty('tasks')
    expect(projected.schedule?.keepAwake).toBe(settings.schedule.keepAwake)
    expect(settings.schedule.tasks).toHaveLength(1)
    expect(() => rejectScheduleTaskArrayWrite({ schedule: { tasks: [] } })).toThrow()
    expect(() => rejectScheduleTaskArrayWrite(projected)).not.toThrow()
  })
})
