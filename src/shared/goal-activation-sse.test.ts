import { describe, expect, it } from 'vitest'
import { PublicRuntimeEventFilter } from './public-runtime-content'
import { projectPublicRuntimeSseBlock } from './public-runtime-sse'

const event={kind:'goal_activation_changed',threadId:'thread-a',timestamp:'2026-10-11T00:00:00Z',activation:{id:'goal-a',revision:2,activation:'armed'}}
function project(value: Record<string,unknown>, id?:number) {
  return projectPublicRuntimeSseBlock(`${id === undefined ? '' : `id: ${id}\n`}event: ${value.kind}\ndata: ${JSON.stringify(value)}`,
    'thread-a',new PublicRuntimeEventFilter())
}
describe('Goal activation metadata through the public SSE boundary', () => {
  it('delivers strict transient metadata without durable id or payload seq', () => {
    const result=project(event)
    expect(result?.status).toBe('emit')
    if(result?.status==='emit') { expect(result.seq).toBe(0);expect(result.event).toEqual(event);expect(result.event).not.toHaveProperty('seq') }
    expect(project({...event,activation:null})?.status).toBe('emit')
  })
  it('rejects cursor, identity and prose expansion and retains the Goal prose refusal', () => {
    for (const value of [{...event,seq:3},{...event,threadId:'thread-b'},{...event,goal:{objective:'private prose'}},
      {...event,activation:{...event.activation,objective:'private prose'}}]) expect(project(value)?.status).toBe('invalid')
    expect(project(event,3)?.status).toBe('invalid')
    expect(project({...event,kind:'goal_updated',seq:3,goal:{objective:'private prose'}},3)?.status).toBe('invalid')
  })
})
