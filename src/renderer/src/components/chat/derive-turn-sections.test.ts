import { describe, expect, it } from 'vitest'
import type { ChatBlock } from '../../agent/types'
import { deriveTurnSections } from './derive-turn-sections'
import type { Turn } from './message-timeline-turns'

function sections(blocks: ChatBlock[]) {
  return deriveTurnSections({
    turn: { blocks } satisfies Turn,
    isProcessing: false,
    liveProcessText: '',
    liveContent: '',
    workspaceRoot: '/tmp'
  })
}

function processingSections(input: {
  blocks?: ChatBlock[]
  liveProcessText?: string
  liveContent?: string
}) {
  return deriveTurnSections({
    turn: { blocks: input.blocks ?? [] } satisfies Turn,
    isProcessing: true,
    liveProcessText: input.liveProcessText ?? '',
    liveContent: input.liveContent ?? '',
    workspaceRoot: '/tmp'
  })
}

describe('deriveTurnSections', () => {
  it('shows only successful strictly typed generated Office objects', () => {
    const artifact = { artifactId: 'a'.repeat(64), kind: 'docx', contentHash: 'b'.repeat(64), byteSize: 1000, savedAt: '2026-09-15T01:00:00Z' }
    const blocks: ChatBlock[] = [
      { kind: 'tool', id: 'created', summary: 'generate_office_document', status: 'success', meta: { generatedArtifact: artifact } },
      { kind: 'tool', id: 'failed', summary: 'generate_office_document', status: 'error', meta: { generatedArtifact: artifact } },
      { kind: 'tool', id: 'untyped', summary: 'generate_office_document', status: 'success', meta: { generatedArtifact: { ...artifact, path: '/private/path' } } }
    ]
    expect(sections(blocks).generatedFileBlocks.map(block => block.id)).toEqual(['created'])
  })
	it('renders the final assistant answer and drops persisted reasoning', () => {
    const hostileLegacyReasoning = {
      kind: 'reasoning',
      id: 'reasoning',
      text: 'The user greeted me.'
    } as unknown as ChatBlock
    const result = sections([
      { kind: 'assistant', id: 'answer', text: '你好！' },
      hostileLegacyReasoning
    ])

    expect(result.assistantContentBlocks).toEqual([
      { kind: 'assistant', id: 'answer', text: '你好！' }
    ])
	 expect(result.processBlocks).toEqual([])
  })

  it('uses the last assistant text as final content without duplicating it in process work', () => {
    const result = sections([
      { kind: 'assistant', id: 'preface', text: '我先检查一下。' },
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'read',
        status: 'success',
        toolKind: 'tool_call'
      }
    ])

    expect(result.assistantContentBlocks).toEqual([
      { kind: 'assistant', id: 'preface', text: '我先检查一下。' }
    ])
    expect(result.processBlocks.map((block) => block.kind)).toEqual(['tool'])
  })

  it('drops restricted assistant blocks before renderer turn sections are built', () => {
    const authorityRef = `cer1_${'a'.repeat(64)}`
    const canonicalEvidenceV3 = JSON.stringify({
      canonicalEvidence: {
        schemaVersion: 3,
        purpose: 'analytix.canonical-evidence/v3',
        facts: []
      }
    })
    const result = sections([
      { kind: 'assistant', id: 'private-ref', text: `prefix ${authorityRef} suffix` },
      { kind: 'assistant', id: 'private-v3', text: `prefix ${canonicalEvidenceV3} suffix` }
    ])

    expect(result.assistantContentBlocks).toEqual([])
    expect(result.processBlocks).toEqual([])
  })

  it('keeps assistant narration before tool work inside the process timeline', () => {
    const result = sections([
      { kind: 'assistant', id: 'intro', text: 'I found the likely cause.' },
      {
        kind: 'tool',
        id: 'tool_read',
        summary: 'read: source',
        status: 'success',
        toolKind: 'tool_call',
        detail: 'read output'
      },
      {
        kind: 'assistant',
        id: 'analysis',
        text: [
          'Here is the detailed analysis:',
          '',
          '```txt',
          'command output line 1',
          'command output line 2',
          '```'
        ].join('\n')
      },
      {
        kind: 'tool',
        id: 'tool_issue',
        summary: 'web_fetch: issue',
        status: 'success',
        toolKind: 'tool_call',
        detail: 'https://github.com/XingYu-Zhong/analytix/issues/96'
      },
      { kind: 'assistant', id: 'next', text: 'The issue link above should still be visible.' }
    ])

    expect(result.assistantContentBlocks.map((block) => block.id)).toEqual(['next'])
    expect(result.processBlocks.map((block) => block.id)).toEqual([
      'intro',
      'tool_read',
      'analysis',
      'tool_issue'
    ])
    expect(result.processBlocks.map((block) => 'text' in block ? block.text : '').join('\n\n')).toContain(
      'command output line 2'
    )
  })

  it('shows tool process before the final answer after a completed Go tool loop', () => {
    const result = sections([
      { kind: 'assistant', id: 'preface', text: 'I will inspect the file first.' },
      {
        kind: 'tool',
        id: 'tool_read',
        summary: 'Read file',
        status: 'success',
        toolKind: 'tool_call',
        detail: 'read output'
      },
      { kind: 'assistant', id: 'final', text: 'The file contains the requested value.' }
    ])

    expect(result.assistantContentBlocks).toEqual([
      { kind: 'assistant', id: 'final', text: 'The file contains the requested value.' }
    ])
    expect(result.processBlocks.map((block) => block.id)).toEqual(['preface', 'tool_read'])
  })

  it('does not create assistant content from tool-only process work', () => {
    const result = sections([
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'read',
        status: 'success',
        toolKind: 'tool_call'
      }
    ])

    expect(result.assistantContentBlocks).toEqual([])
    expect(result.processBlocks.map((block) => block.kind)).toEqual(['tool'])
  })

  it('keeps compaction markers out of work process step counts', () => {
    const result = sections([
      {
        kind: 'compaction',
        id: 'compact_1',
        summary: 'Context compacted',
        status: 'success',
        messagesBefore: 10,
        messagesAfter: 2
      },
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'read',
        status: 'success',
        toolKind: 'tool_call'
      }
    ])

    expect(result.processBlocks.map((block) => block.id)).toEqual(['tool_1'])
    expect(result.compactionBlocks.map((block) => block.id)).toEqual(['compact_1'])
  })

  it('surfaces generated media blocks outside the collapsed process work', () => {
    const result = sections([
      {
        kind: 'tool',
        id: 'tool_img',
        summary: 'generate_image',
        status: 'success',
        toolKind: 'tool_call',
        meta: {
          attachments: [{ id: 'att_img', name: 'img.png', mimeType: 'image/png' }],
          generatedFiles: [{ relativePath: '.analytix-images/img.png', mimeType: 'image/png' }]
        }
      },
      {
        kind: 'tool',
        id: 'tool_read',
        summary: 'read',
        status: 'success',
        toolKind: 'tool_call'
      }
    ])

    expect(result.generatedFileBlocks.map((block) => block.id)).toEqual(['tool_img'])
    expect(result.processBlocks.map((block) => block.id)).toEqual(['tool_img', 'tool_read'])
  })

  it('keeps generated media visible while the turn is still processing', () => {
    const result = processingSections({
      blocks: [
        {
          kind: 'tool',
          id: 'tool_img',
          summary: 'generate_image',
          status: 'success',
          toolKind: 'tool_call',
          meta: {
            generatedFiles: [
              {
                relativePath: '.analytix-images/img.png',
                mimeType: 'image/png'
              }
            ]
          }
        },
        {
          kind: 'tool',
          id: 'tool_next',
          summary: 'read',
          status: 'running',
          toolKind: 'tool_call'
        }
      ]
    })

    expect(result.generatedFileBlocks.map((block) => block.id)).toEqual(['tool_img'])
    expect(result.processBlocks.map((block) => block.id)).toEqual(['tool_img', 'tool_next'])
  })

  it('extracts file changes from JSON-wrapped tool output diffs', () => {
    const patch = [
      'diff --git a/demo.ts b/demo.ts',
      '--- a/demo.ts',
      '+++ b/demo.ts',
      '@@ -1,1 +1,1 @@',
      '-old',
      '+new'
    ].join('\n')
    const result = sections([
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'Edit',
        status: 'success',
        toolKind: 'file_change',
        filePath: '/tmp/demo.ts',
        detail: JSON.stringify({ path: '/tmp/demo.ts', diff: patch }, null, 2)
      }
    ])

    expect(result.turnFileChanges).toMatchObject([
      {
        id: 'tool_1',
        detail: patch,
        filePath: 'demo.ts'
      }
    ])
  })

  it('merges repeated file changes for the same displayed path', () => {
    const firstPatch = [
      'diff --git a/.analytixsdd/draft/plan/requirement.md b/.analytixsdd/draft/plan/requirement.md',
      '--- a/.analytixsdd/draft/plan/requirement.md',
      '+++ b/.analytixsdd/draft/plan/requirement.md',
      '@@ -1,1 +1,1 @@',
      '-old title',
      '+new title'
    ].join('\n')
    const secondPatch = [
      'diff --git a/.analytixsdd/draft/plan/requirement.md b/.analytixsdd/draft/plan/requirement.md',
      '--- a/.analytixsdd/draft/plan/requirement.md',
      '+++ b/.analytixsdd/draft/plan/requirement.md',
      '@@ -4,1 +4,2 @@',
      ' context',
      '+new detail'
    ].join('\n')
    const result = sections([
      {
        kind: 'tool',
        id: 'tool_first_edit',
        summary: 'Edit requirement',
        status: 'success',
        toolKind: 'file_change',
        filePath: '/tmp/.analytixsdd/draft/plan/requirement.md',
        detail: firstPatch
      },
      {
        kind: 'tool',
        id: 'tool_second_edit',
        summary: 'Edit requirement again',
        status: 'success',
        toolKind: 'file_change',
        filePath: '/tmp/.analytixsdd/draft/plan/requirement.md',
        detail: secondPatch
      }
    ])

    expect(result.turnFileChanges).toHaveLength(1)
    expect(result.turnFileChanges[0]).toMatchObject({
      id: 'tool_first_edit',
      filePath: '.analytixsdd/draft/plan/requirement.md'
    })
    expect(result.turnFileChanges[0]?.detail).toContain('+new title')
    expect(result.turnFileChanges[0]?.detail).toContain('+new detail')
  })

  it('surfaces rewind restore plans as audit-only file changes without a diff', () => {
    const result = sections([
      {
        kind: 'tool',
        id: 'tool_rewind_plan',
        summary: 'Checkpoint rewind plan',
        status: 'success',
        toolKind: 'file_change',
        meta: {
          rewindPlan: {
            schemaVersion: 1,
            planId: 'axrp_123',
            planDigest: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
            checkpointId: 'axcp_123',
            threadId: 'thr_1',
            workspace: '/tmp',
            createdAt: '2026-06-20T10:00:00.000Z',
            scope: 'combined',
            applyMode: 'plan_only',
            destructive: false,
            checkpoint: {
              schemaVersion: 1,
              checkpointId: 'axcp_123',
              threadId: 'thr_1',
              turnId: 'turn_2',
              workspace: '/tmp',
              createdAt: '2026-06-20T10:00:00.000Z',
              status: 'captured',
              changedFiles: []
            },
            files: [
              {
                relativePath: 'src/app.ts',
                changeKind: 'modified',
                action: 'restore_previous_version',
                status: 'ready',
                risk: 'medium',
                reason: 'metadata only',
                contentSource: 'checkpoint_metadata_only'
              }
            ],
            summary: {
              fileCount: 1,
              readyFileCount: 1,
              manualReviewFileCount: 0,
              blockedFileCount: 0,
              retainedEventCount: 3,
              removedEventCount: 2,
              removedTurnCount: 1,
              containsRawPrompt: false,
              containsFullFileContent: false,
              containsSecretValue: false
            }
          }
        }
      }
    ])

    expect(result.turnFileChanges).toHaveLength(1)
    expect(result.turnFileChanges[0]).toMatchObject({
      id: 'tool_rewind_plan',
      detail: '',
      filePath: 'Checkpoint rewind plan (1 files)'
    })
    expect(result.turnFileChanges[0]?.meta?.rewindPlan?.destructive).toBe(false)
  })

	it('keeps private live process text out of the active process timeline', () => {
    const result = processingSections({
      liveProcessText: 'private reasoning',
      liveContent: '这里是正在生成的回答。'
    })

    expect(result.assistantContentBlocks).toEqual([])
	 expect(result.processBlocks).toEqual([])
  })

  it('keeps assistant content in chronological process order while a later tool is still running', () => {
    const result = processingSections({
      blocks: [
        { kind: 'assistant', id: 'answer', text: '先给你一部分结果。' },
        {
          kind: 'tool',
          id: 'tool_1',
          summary: 'read',
          status: 'running',
          toolKind: 'tool_call'
        }
      ]
    })

    expect(result.assistantContentBlocks).toEqual([])
    expect(result.processBlocks).toEqual([
      { kind: 'assistant', id: 'answer', text: '先给你一部分结果。' },
      {
        kind: 'tool',
        id: 'tool_1',
        summary: 'read',
        status: 'running',
        toolKind: 'tool_call'
      }
    ])
  })

  it('places assistant output between process steps while processing', () => {
    const result = processingSections({
      blocks: [
        {
          kind: 'tool',
          id: 'tool_1',
          summary: 'read',
          status: 'success',
          toolKind: 'tool_call'
        },
        { kind: 'assistant', id: 'answer', text: '读完了，下一步继续查。' },
        {
          kind: 'tool',
          id: 'tool_2',
          summary: 'grep',
          status: 'running',
          toolKind: 'tool_call'
        }
      ]
    })

    expect(result.assistantContentBlocks).toEqual([])
    expect(result.processBlocks.map((block) => block.id)).toEqual(['tool_1', 'answer', 'tool_2'])
  })
})
