import { describe, expect, it } from 'vitest'
import { TrajectoryProjection } from './trajectory-adapter'
import { deriveTrajectoryTimeline, trajectoryFocusIds } from './trajectory-timeline-model'
import { stepEvent, toolEvent, userEvent } from './trajectory-test-fixtures'
describe('recorded trajectory coordinates', () => {
  it('uses equal sequence slots and input points without turning the metrics close into a bar end', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept([userEvent(1), stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_assembled', 3, 2), toolEvent(false, 4), toolEvent(true, 7), stepEvent('logical_step_closed', 8)])
    const model = deriveTrajectoryTimeline(projection.getSnapshot().records, false)
    expect(model.spans.map(span => [span.lane, span.start, span.end])).toEqual([[0, .5, .5], [1, 1, 2], [2, 2, 3]])
    expect(trajectoryFocusIds(model, { start: 1.1, end: 1.9, mode: 'sequence' })).toEqual(new Set([model.spans[1].record.id]))
  })
  it('compresses actual idle gaps but preserves true interval lengths and unknown legacy ends', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept([userEvent(1), stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_assembled', 3, 2), toolEvent(false, 4), toolEvent(true, 7)])
    const model = deriveTrajectoryTimeline(projection.getSnapshot().records, true)
    expect(model.end - model.start).toBe(5000)
    expect(model.spans[1].end - model.spans[1].start).toBe(1000)
    expect(model.spans[2].end - model.spans[2].start).toBe(4000)
    const legacy = new TrajectoryProjection('trajectory-thread')
    legacy.accept([stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_closed', 8, 8)])
    const [unknown] = deriveTrajectoryTimeline(legacy.getSnapshot().records, true).spans
    expect(unknown.point).toBe(true)
    expect(unknown.end).toBe(unknown.start)
  })
  it('keeps retained coordinates stable when the local resident window prepends records', () => {
    const projection = new TrajectoryProjection('trajectory-thread')
    projection.accept([userEvent(1), stepEvent('logical_step_started', 2, 1), stepEvent('logical_step_assembled', 3, 2), toolEvent(false, 4), toolEvent(true, 7)])
    const records = projection.getSnapshot().records, resident = records.slice(1)
    for (const duration of [false, true]) {
      const before = deriveTrajectoryTimeline(resident, duration, 1, records)
      const after = deriveTrajectoryTimeline(records, duration)
      expect(before.spans.map(span => [span.record.id, span.start, span.end])).toEqual(after.spans.slice(1).map(span => [span.record.id, span.start, span.end]))
    }
  })

})
