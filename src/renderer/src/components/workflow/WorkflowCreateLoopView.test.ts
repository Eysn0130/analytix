import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeEach, describe, expect, it } from 'vitest'
import '../../i18n'
import i18n from '../../i18n'
import type { CreateLoopRun } from '../../workflow/create-loop-runtime'
import { WorkflowCreateLoopView } from './WorkflowCreateLoopView'

function run(patch: Partial<CreateLoopRun> = {}): CreateLoopRun {
  return {
    id: 'run-1',
    workflowId: 'analytix-create-loop',
    title: 'Create Loop',
    objective: 'Ship workflow UI',
    status: 'waiting',
    threadId: 'thread-1',
    model: 'model-a',
    providerId: 'provider-a',
    createdAt: '2026-06-20T00:00:00.000Z',
    updatedAt: '2026-06-20T00:00:00.000Z',
    lastMessage: 'Approve the plan',
    steps: [
      {
        id: 'plan',
        title: 'Plan',
        status: 'waiting',
        waitingFor: 'approval',
        waitingItemId: 'approval-1',
        message: 'Approve the plan',
        turnId: 'turn-1'
      },
      { id: 'execute', title: 'Execute', status: 'pending' },
      { id: 'review', title: 'Review', status: 'pending' }
    ],
    ...patch
  }
}

function render(initialRuns: CreateLoopRun[] = []): string {
  return renderToStaticMarkup(
    createElement(WorkflowCreateLoopView, {
      leftSidebarCollapsed: false,
      onOpenThread: () => {},
      initialRuns
    })
  )
}

describe('WorkflowCreateLoopView', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('renders the create-loop entry state', () => {
    const html = render()

    expect(html).toContain('Workflow')
    expect(html).toContain('Objective')
    expect(html).toContain('Run Create Loop')
    expect(html).toContain('No workflow runs yet.')
  })

  it('renders waiting run state with resume and thread access', () => {
    const html = render([run()])

    expect(html).toContain('Ship workflow UI')
    expect(html).toContain('approval')
    expect(html).toContain('Resume')
    expect(html).toContain('Open thread')
    expect(html).toContain('turn-1')
  })
})
