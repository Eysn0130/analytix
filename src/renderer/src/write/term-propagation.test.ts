import { describe, expect, it } from 'vitest'
import {
  buildWriteCanonicalTermPropagationChanges,
  buildWriteTermPropagationChanges
} from './term-propagation'

function applyChanges(
  content: string,
  changes: Array<{ from: number; to: number; insert: string }>
): string {
  let next = content
  for (const change of [...changes].sort((a, b) => b.from - a.from)) {
    next = `${next.slice(0, change.from)}${change.insert}${next.slice(change.to)}`
  }
  return next
}

describe('write term propagation', () => {
  it('propagates a case-only phrase replacement within the same paragraph', () => {
    const content = [
      'i build Project GUI, li is amazing ui production.',
      'project gui can write paper, also can code. project gui is use',
      'deepseek api, but it not only that.',
      '',
      'project gui in another paragraph stays untouched.'
    ].join('\n')
    const seedFrom = content.indexOf('Project GUI')

    const changes = buildWriteTermPropagationChanges(content, {
      from: seedFrom,
      to: seedFrom + 'Project GUI'.length,
      deletedText: 'project gui',
      insertedText: 'Project GUI'
    })

    expect(changes).toHaveLength(2)
    expect(applyChanges(content, changes)).toBe([
      'i build Project GUI, li is amazing ui production.',
      'Project GUI can write paper, also can code. Project GUI is use',
      'deepseek api, but it not only that.',
      '',
      'project gui in another paragraph stays untouched.'
    ].join('\n'))
  })

  it('propagates a term rename such as project gui to DXGUI', () => {
    const content = 'DXGUI is here. project gui is there. project gui again.'
    const changes = buildWriteTermPropagationChanges(content, {
      from: 0,
      to: 'DXGUI'.length,
      deletedText: 'project gui',
      insertedText: 'DXGUI'
    })

    expect(applyChanges(content, changes)).toBe('DXGUI is here. DXGUI is there. DXGUI again.')
  })

  it('does not replace partial word matches', () => {
    const content = 'Project GUI works. myproject gui should not. project gui should.'
    const seedFrom = content.indexOf('Project GUI')

    const changes = buildWriteTermPropagationChanges(content, {
      from: seedFrom,
      to: seedFrom + 'Project GUI'.length,
      deletedText: 'project gui',
      insertedText: 'Project GUI'
    })

    expect(applyChanges(content, changes)).toBe(
      'Project GUI works. myproject gui should not. Project GUI should.'
    )
  })

  it('propagates canonical casing after an incremental case edit', () => {
    const content = 'Project GUI works. project gui should follow. deepseek api should not.'
    const seedFrom = content.indexOf('Project GUI')

    const changes = buildWriteCanonicalTermPropagationChanges(content, {
      from: seedFrom,
      to: seedFrom + 1,
      deletedText: 'd',
      insertedText: 'D'
    })

    expect(applyChanges(content, changes)).toBe(
      'Project GUI works. Project GUI should follow. deepseek api should not.'
    )
  })
})
