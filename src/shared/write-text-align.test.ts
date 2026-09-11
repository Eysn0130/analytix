import { describe, expect, it } from 'vitest'
import {
  stripWriteTextAlignDirectivesFromMarkdown,
  applyWriteTextAlignToMarkdownLines,
  applyWriteMarkdownAlignmentDirectivesToTree
} from './write-text-align'

describe('write text alignment helpers', () => {
  it('rewrites selected lines without keeping stale alignment directives', () => {
    expect(
      applyWriteTextAlignToMarkdownLines(['# Title', '{: align=center}', 'Body'], 'left')
    ).toEqual(['# Title', 'Body'])
    expect(applyWriteTextAlignToMarkdownLines(['Body'], 'right')).toEqual(['Body', '{: align=right}'])
  })

  it('strips stale alignment directives from markdown content', () => {
    expect(
      stripWriteTextAlignDirectivesFromMarkdown('# Title\n{: align=center}\r\n\r\nBody\n{: align=justify}')
    ).toBe('# Title\n\nBody')
  })

  it('moves markdown alignment directives into render properties', () => {
    const tree: any = {
      children: [
        { type: 'paragraph', children: [{ type: 'text', value: 'Body' }] },
        { type: 'paragraph', children: [{ type: 'text', value: '{: align=justify}' }] }
      ]
    }

    applyWriteMarkdownAlignmentDirectivesToTree(tree)

    expect(tree.children).toHaveLength(1)
    expect(tree.children[0]?.data?.hProperties?.['data-write-align']).toBe('justify')
  })
})
