import { describe, expect, it } from 'vitest'
import { createGuiPlanArtifact } from '../plan/plan-store'
import {
  buildDraftGuiPlanTurnOverrides,
  buildGuiPlanTurnOverrides,
  latestSuccessfulPlanBlockId,
  resolvePlanTurnWorkspaceRoot
} from './workbench-plan-controller'

describe('workbench plan controller helpers', () => {
  it('prefers an explicit target workspace over stale workbench state', () => {
    expect(resolvePlanTurnWorkspaceRoot('/Users/codex/sdd-workspace/', '/Users/codex/stale-workspace')).toBe(
      '/Users/codex/sdd-workspace'
    )
    expect(resolvePlanTurnWorkspaceRoot(undefined, '/Users/codex/current-workspace/')).toBe(
      '/Users/codex/current-workspace'
    )
  })

  it('builds refine context only for the current plan workspace and thread', () => {
    const plan = createGuiPlanArtifact({
      workspaceRoot: '/Users/codex/app/',
      threadId: 'thread-current',
      relativePath: '.analytixsdd/plan/checkout.md',
      sourceRequest: 'Improve checkout',
      now: 1
    })

    expect(buildGuiPlanTurnOverrides(plan, '/Users/codex/app', 'thread-current')).toMatchObject({
      guiPlan: {
        operation: 'refine',
        workspaceRoot: '/Users/codex/app',
        relativePath: '.analytixsdd/plan/checkout.md',
        planId: '/Users/codex/app:.analytixsdd/plan/checkout.md',
        sourceRequest: 'Improve checkout'
      }
    })
    expect(buildGuiPlanTurnOverrides(plan, '/Users/codex/app', 'thread-stale')).toBeUndefined()
    expect(buildGuiPlanTurnOverrides(plan, '/Users/codex/other', 'thread-current')).toBeUndefined()
  })

  it('builds draft context for first-class GUI plan turns', () => {
    const result = buildDraftGuiPlanTurnOverrides({
      request: 'Build Login: OAuth / SSO?',
      workspaceRoot: '/Users/codex/app/',
      activeThreadId: 'thread-current',
      existingRelativePaths: ['.analytixsdd/plan/build-login-oauth-sso.md']
    })

    expect(result.guiPlan).toEqual({
      operation: 'draft',
      workspaceRoot: '/Users/codex/app',
      relativePath: '.analytixsdd/plan/build-login-oauth-sso-2.md',
      planId: '/Users/codex/app:.analytixsdd/plan/build-login-oauth-sso-2.md',
      sourceRequest: 'Build Login: OAuth / SSO?',
      title: 'build-login-oauth-sso-2'
    })
  })

  it('selects only the latest successful plan block id for workbench subscriptions', () => {
    expect(
      latestSuccessfulPlanBlockId([
        { kind: 'user', id: 'u1', text: 'make a plan' },
        {
          kind: 'tool',
          id: 'tool_running',
          summary: 'create_plan',
          status: 'running',
          meta: {
            plan: {
              plan_id: 'plan-running',
              workspace_root: '/tmp/app',
              relative_path: '.analytixsdd/plan/running.md',
              operation: 'draft'
            }
          }
        },
        {
          kind: 'tool',
          id: 'tool_done',
          summary: 'create_plan',
          status: 'success',
          meta: {
            plan: {
              plan_id: 'plan-done',
              workspace_root: '/tmp/app',
              relative_path: '.analytixsdd/plan/done.md',
              operation: 'draft'
            }
          }
        }
      ])
    ).toBe('tool_done')
  })
})
