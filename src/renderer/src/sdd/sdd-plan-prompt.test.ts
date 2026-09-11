import { describe, expect, it } from 'vitest'
import { buildSddDraftToPlanPrompt } from './sdd-plan-prompt'
import type { SddDraftImageReference } from './sdd-draft-images'

function image(partial: Partial<SddDraftImageReference> = {}): SddDraftImageReference {
  return {
    index: 1,
    alt: 'wireframe',
    markdownPath: '../../img/wireframe.png',
    relativePath: '.analytixsdd/img/wireframe.png',
    mimeType: 'image/png',
    dataBase64: 'ZmFrZS1pbWFnZQ==',
    byteSize: 10,
    width: 320,
    height: 240,
    ...partial
  }
}

describe('buildSddDraftToPlanPrompt', () => {
  it('keeps Markdown image syntax and maps visual attachments by image number', () => {
    const prompt = buildSddDraftToPlanPrompt({
      workspaceRoot: '/tmp/ws',
      draftRelativePath: '.analytixsdd/draft/draft-1/requirement.md',
      planRelativePath: '.analytixsdd/plan/sdd-draft-1.md',
      draftMarkdown: '# Need login\n\n![wireframe](../../img/wireframe.png)',
      imageMode: 'attachments',
      images: [image({ attachmentId: 'att_1' })]
    })

    expect(prompt).toContain('![wireframe](../../img/wireframe.png)')
    expect(prompt).toContain('Image Reference Map:')
    expect(prompt).toContain('Image 1: ../../img/wireframe.png')
    expect(prompt).toContain('Attachment: att_1')
    expect(prompt).toContain('You MUST use the `create_plan` tool exactly once')
    expect(prompt).toContain('Reserved plan file: .analytixsdd/plan/sdd-draft-1.md')
    expect(prompt).toContain('`plan_relative_path` to `.analytixsdd/plan/sdd-draft-1.md`')
  })

  it('includes base64 fallback when visual attachments are unavailable', () => {
    const prompt = buildSddDraftToPlanPrompt({
      workspaceRoot: '/tmp/ws',
      draftRelativePath: '.analytixsdd/draft/draft-1/requirement.md',
      planRelativePath: '.analytixsdd/plan/sdd-draft-1.md',
      draftMarkdown: '# Need login\n\n![wireframe](../../img/wireframe.png)',
      imageMode: 'base64',
      images: [image()]
    })

    expect(prompt).toContain('base64 text')
    expect(prompt).toContain('MIME: image/png')
    expect(prompt).toContain('Dimensions: 320x240')
    expect(prompt).toContain('```base64\nZmFrZS1pbWFnZQ==\n```')
  })

  it('includes sidebar Requirement AI conversation context when provided', () => {
    const prompt = buildSddDraftToPlanPrompt({
      workspaceRoot: '/tmp/ws',
      draftRelativePath: '.analytixsdd/draft/draft-1/requirement.md',
      planRelativePath: '.analytixsdd/plan/sdd-draft-1.md',
      draftMarkdown: '# Need login',
      assistantContext: 'Requirement AI:\nConfirm OAuth edge cases.',
      imageMode: 'none',
      images: []
    })

    expect(prompt).toContain('Requirement AI conversation context:')
    expect(prompt).toContain('Confirm OAuth edge cases.')
  })

  it('injects pre-mortem and prioritization framework guidance', () => {
    const prompt = buildSddDraftToPlanPrompt({
      workspaceRoot: '/tmp/ws',
      draftRelativePath: '.analytixsdd/draft/draft-1/requirement.md',
      planRelativePath: '.analytixsdd/plan/sdd-draft-1.md',
      draftMarkdown: '# Need login',
      imageMode: 'none',
      images: []
    })

    expect(prompt).toContain('## Pre-mortem')
    expect(prompt).toContain('Tigers')
    expect(prompt).toContain('## Sequencing & prioritization')
    expect(prompt).toContain('RICE')
  })

  it('injects design context into the plan prompt', () => {
    const prompt = buildSddDraftToPlanPrompt({
      workspaceRoot: '/tmp/ws',
      draftRelativePath: '.analytixsdd/draft/draft-1/requirement.md',
      planRelativePath: '.analytixsdd/plan/sdd-draft-1.md',
      draftMarkdown: '# Need login',
      imageMode: 'none',
      images: [],
      designContext: {
        designType: 'brand',
        brandColor: '#3b82d8',
        tone: ['专业', '科技感']
      }
    })

    expect(prompt).toContain('Design context (honor it in every visual decision):')
    expect(prompt).toContain('Brand color anchor: #3b82d8')
    expect(prompt).toContain('Tone: 专业、科技感')
    expect(prompt).toContain('purple-blue gradients')
  })
})
