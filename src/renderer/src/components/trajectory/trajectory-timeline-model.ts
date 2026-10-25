/* Adapted from DeepSeek Harness 5badb15009ae1756c3afe0ae0cef1faafc290ccc
 * ui-trajectory timeline.ts. Copyright (c) 2026 DeepSeek. MIT; THIRD_PARTY_NOTICES.md. */
import { trajectoryDuration, type TrajectoryRecord } from './trajectory-adapter'
export type TrajectoryRange = { start: number; end: number; mode: 'sequence' | 'duration' }
export type TrajectorySpan = { record: TrajectoryRecord; start: number; end: number; lane: number; point: boolean }
export type TrajectoryTimelineModel = { start: number; end: number; spans: TrajectorySpan[] }
export function deriveTrajectoryTimeline(records: readonly TrajectoryRecord[], duration: boolean, sequenceOffset = 0, coordinateRecords: readonly TrajectoryRecord[] = records): TrajectoryTimelineModel {
  const cells = records.filter(record => record.kind !== 'message')
  const lane = (record: TrajectoryRecord) => ({ input: 0, model: 1, tool: 2 })[record.lane]
  if (!duration) return { start: sequenceOffset, end: sequenceOffset + Math.max(1, cells.length), spans: cells.map((record, index) => ({
    record, lane: lane(record), start: record.kind === 'user' ? sequenceOffset + index + .5 : sequenceOffset + index,
    end: record.kind === 'user' ? sequenceOffset + index + .5 : sequenceOffset + index + 1, point: record.kind === 'user'
  })) }
  const retainedIds = new Set(cells.map(record => record.id))
  const raw = coordinateRecords.filter(record => record.kind !== 'message').flatMap(record => {
    const at = record.startedAt ?? record.at
    if (!at) return []
    const start = Date.parse(at), elapsed = trajectoryDuration(record)
    return [{ record, lane: lane(record), start, end: start + (elapsed ?? 0), point: elapsed === null || record.kind === 'user' }]
  })
  // The visible Duration toggle compresses idle gaps; it does not invent an end
  // for ready/running/legacy records. Host timestamps remain unchanged in details.
  const offsets = new Map<string, number>()
  let removed = 0, covered: number | null = null
  for (const span of [...raw].sort((a, b) => a.start - b.start || a.end - b.end)) {
    if (covered !== null && span.start > covered) removed += span.start - covered
    offsets.set(span.record.id, removed)
    covered = Math.max(covered ?? span.end, span.end)
  }
  const spans = raw.filter(span => retainedIds.has(span.record.id)).map(span => ({ ...span, start: span.start - offsets.get(span.record.id)!, end: span.end - offsets.get(span.record.id)! }))
  const start = spans.length ? Math.min(...spans.map(span => span.start)) : 0
  return { start, end: Math.max(start + 1, ...spans.map(span => span.end)), spans }
}
export function trajectoryFocusIds(model: TrajectoryTimelineModel, range: TrajectoryRange | null): ReadonlySet<string> | null {
  return range === null ? null : new Set(model.spans.filter(span => span.start <= range.end && span.end >= range.start).map(span => span.record.id))
}
