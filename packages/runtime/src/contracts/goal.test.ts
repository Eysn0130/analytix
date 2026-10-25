import { describe, expect, it } from 'vitest'
import { GoalActivationChangedEventV1, RuntimeEvent } from './events'
import { GoalRoundSourceSchemaV1 } from './goal'
import { SetThreadGoalRequest, ThreadGoalResponse, countedGoalRefV1 } from './threads'

const goal = { id:'goal-a',revision:2,maxGoalRounds:3,roundsStarted:1,threadId:'thread-a',objective:'Verify delivery',
  status:'active' as const,tokensUsed:0,timeUsedSeconds:0,createdAt:'2026-10-11T00:00:00Z',updatedAt:'2026-10-11T00:00:00Z' }
describe('Goal lifecycle and transient metadata contracts', () => {
  it('preserves legacy unknown facts and requires matching live activation', () => {
    const {id: _id,revision:_revision,maxGoalRounds:_max,roundsStarted:_started,...legacy}=goal
    const parsed=ThreadGoalResponse.parse({goal:legacy,activation:null,history:[]})
    expect(countedGoalRefV1(parsed.goal)).toBeNull()
    expect(parsed.goal).not.toHaveProperty('roundsStarted')
    expect(ThreadGoalResponse.safeParse({goal,activation:{id:'goal-a',revision:1,activation:'armed'}}).success).toBe(false)
    expect(ThreadGoalResponse.safeParse({goal,activation:{id:'goal-a',revision:2,activation:'armed'},history:[]}).success).toBe(true)
  })
  it('does not downgrade an invalid lifecycle command into a legacy patch', () => {
    expect(SetThreadGoalRequest.safeParse({operation:'resume',status:'active'}).success).toBe(false)
    expect(SetThreadGoalRequest.safeParse({operation:'edit',ref:{id:'goal-a',revision:2},maxGoalRounds:4}).success).toBe(true)
    expect(SetThreadGoalRequest.safeParse({operation:'edit',ref:{id:'goal-a',revision:2}}).success).toBe(false)
  })
  it('keeps activation out of durable events and source closed', () => {
    const event={kind:'goal_activation_changed',threadId:'thread-a',timestamp:'2026-10-11T00:00:00Z',activation:{id:'goal-a',revision:2,activation:'armed'}}
    expect(GoalActivationChangedEventV1.safeParse(event).success).toBe(true)
    expect(RuntimeEvent.safeParse(event).success).toBe(false)
    expect(GoalActivationChangedEventV1.safeParse({...event,seq:3}).success).toBe(false)
    const source={kind:'goal',goalId:'goal-a',revision:2,round:1}
    expect(GoalRoundSourceSchemaV1.safeParse(source).success).toBe(true)
    expect(GoalRoundSourceSchemaV1.safeParse({...source,prompt:'private'}).success).toBe(false)
  })
})
