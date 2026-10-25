// @vitest-environment jsdom
import { act, createElement } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import i18n from '../../../i18n'
import { GoalBar, GoalFacts } from './GoalBar'

const legacy={threadId:'thread-a',objective:'Recorded objective',status:'paused' as const,tokensUsed:0,timeUsedSeconds:0,createdAt:'2026-10-11',updatedAt:'2026-10-11'}
describe('Goal recorded facts', () => {
  it('shows absent legacy counts and activation as unknown without borrowing defaults', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await i18n.changeLanguage('en')
    const host=document.createElement('div');document.body.append(host);const root=createRoot(host)
    try {
      await act(async()=>root.render(createElement(GoalFacts,{goal:legacy,activation:null})))
      expect(host.textContent).toContain('Rounds started: Unknown')
      expect(host.textContent).toContain('Round limit: Unknown')
      expect(host.textContent).toContain('Automatic continuation: Unknown')
      expect(host.textContent).not.toContain('256')
    } finally {await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals()}
  })
  it('renders an immutable historical snapshot without live actions or activation', async () => {
    vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await i18n.changeLanguage('en')
    const host=document.createElement('div');document.body.append(host);const root=createRoot(host)
    try {
      await act(async()=>root.render(createElement(GoalBar,{goal:{...legacy,id:'goal-a',revision:2,maxGoalRounds:8,roundsStarted:3},operation:'edit'})))
      expect(host.textContent).toContain('Recorded objective')
      expect(host.textContent).toContain('Rounds started: 3')
      expect(host.textContent).toContain('Round limit: 8')
      expect(host.textContent).not.toContain('Automatic continuation')
      expect(host.querySelector('button,input')).toBeNull()
      await act(async()=>root.render(createElement(GoalBar,{goal:null,operation:'clear'})))
      expect(host.textContent).toContain(i18n.t('common:goalClearedTimeline'))
      expect(host.textContent).not.toContain('Recorded objective')
    } finally {await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals()}
  })
})
