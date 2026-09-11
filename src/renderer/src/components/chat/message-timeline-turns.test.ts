import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import { groupTurns, sameTurnContent, splitThink, stableTurnKey } from './message-timeline-turns'

describe('message timeline turns', () => {
  it('uses stable ids for user and assistant-only turns', () => {
    const blocks: ChatBlock[] = [
      { kind: 'assistant', id: 'assistant_intro', text: 'Welcome' },
      { kind: 'user', id: 'user_1', text: 'Hello' },
      { kind: 'assistant', id: 'assistant_1', text: 'Hi' }
    ]

    const turns = groupTurns(blocks)

    expect(stableTurnKey(turns[0], 0)).toBe('assistant_intro')
    expect(stableTurnKey(turns[1], 1)).toBe('user_1')
  })

  it('treats rebuilt turn arrays as the same content when block references are unchanged', () => {
    const blocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'Hello' },
      { kind: 'assistant', id: 'assistant_1', text: 'Hi' }
    ]

    const first = groupTurns(blocks)[0]
    const second = groupTurns(blocks)[0]

    expect(first).not.toBe(second)
    expect(sameTurnContent(first, second)).toBe(true)
  })

  it('detects updates to a block inside an otherwise stable turn', () => {
    const firstBlocks: ChatBlock[] = [
      { kind: 'user', id: 'user_1', text: 'Hello' },
      { kind: 'assistant', id: 'assistant_1', text: 'Hi' }
    ]
    const nextBlocks: ChatBlock[] = [
      firstBlocks[0],
      { kind: 'assistant', id: 'assistant_1', text: 'Hi again' }
    ]

    expect(sameTurnContent(groupTurns(firstBlocks)[0], groupTurns(nextBlocks)[0])).toBe(false)
  })

  it('withholds the whole answer when it contains a closed think block', () => {
    expect(splitThink('\n <think>reasoning</think>\nanswer')).toEqual({
      think: '',
      content: ''
    })
  })

  it('drops an unclosed leading think block', () => {
    expect(splitThink('<think>still thinking')).toEqual({
      think: '',
      content: ''
    })
  })

  it('fails closed on an unclosed mid-answer think tag', () => {
    const text = 'The model may emit <think> tags in examples.'
    expect(splitThink(text)).toEqual({ think: '', content: '' })
  })

  it('withholds closed internal references from renderer assistant blocks', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const sourceRowRef = `srow1_${'b'.repeat(64)}`

    expect(splitThink(authorityRef)).toEqual({ think: '', content: '' })
    expect(splitThink(`prefix ${sourceRowRef} suffix`)).toEqual({ think: '', content: '' })
  })

  it('preserves clean bytes and non-marker tag names', () => {
    expect(splitThink('  <thinker>safe</thinker>\n')).toEqual({
      think: '',
      content: '  <thinker>safe</thinker>\n'
    })
  })
})
