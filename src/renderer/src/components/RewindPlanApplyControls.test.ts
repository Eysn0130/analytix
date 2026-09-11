import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'
import type {
  CoreCheckpointRewindApplyResultJson,
  CoreCheckpointRewindPlanJson
} from '../agent/analytix-contract'
import {
  RewindPlanApplyControls,
  applyCheckpointRewindPlanWithConfirmation
} from './RewindPlanApplyControls'

function rewindPlan(
  overrides: Partial<CoreCheckpointRewindPlanJson> = {}
): CoreCheckpointRewindPlanJson {
  return {
    schemaVersion: 1,
    planId: 'plan_1',
    checkpointId: 'axcp_1',
    threadId: 'thr_1',
    planDigest: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    workspace: '/workspace/analytix',
    createdAt: '2026-06-28T00:00:00.000Z',
    scope: 'combined',
    applyMode: 'plan_only',
    destructive: false,
    checkpoint: {
      schemaVersion: 1,
      checkpointId: 'axcp_1',
      threadId: 'thr_1',
      turnId: 'turn_1',
      workspace: '/workspace/analytix',
      createdAt: '2026-06-28T00:00:00.000Z',
      status: 'rewind_planned',
      changedFiles: [
        {
          relativePath: 'src/app.ts',
          changeKind: 'created',
          afterHash: 'sha256:after'
        }
      ]
    },
    files: [
      {
        relativePath: 'src/app.ts',
        changeKind: 'created',
        action: 'delete_created_file',
        status: 'ready',
        risk: 'low',
        reason: 'created after checkpoint',
        contentSource: 'checkpoint_metadata_only',
        afterHash: 'sha256:after'
      }
    ],
    conversation: {
      status: 'ready',
      retainedEventCount: 2,
      removedEventCount: 3,
      removedTurnIds: ['turn_2'],
      projection: {
        latestSeq: 12,
        turnCount: 2,
        itemCount: 4,
        checkpointCount: 1
      }
    },
    summary: {
      fileCount: 1,
      readyFileCount: 1,
      manualReviewFileCount: 0,
      blockedFileCount: 0,
      retainedEventCount: 2,
      removedEventCount: 3,
      removedTurnCount: 1,
      containsRawPrompt: false,
      containsFullFileContent: false,
      containsSecretValue: false
    },
    ...overrides
  }
}

function applyResult(
  overrides: Partial<CoreCheckpointRewindApplyResultJson> = {}
): CoreCheckpointRewindApplyResultJson {
  return {
    schemaVersion: 1,
    applyId: 'apply_1',
    planId: 'plan_1',
    checkpointId: 'axcp_1',
    threadId: 'thr_1',
    workspace: '/workspace/analytix',
    createdAt: '2026-06-28T00:01:00.000Z',
    scope: 'combined',
    status: 'applied',
    destructive: true,
    files: [
      {
        relativePath: 'src/app.ts',
        action: 'delete_created_file',
        status: 'applied',
        reason: 'created file hash matches checkpoint-created hash',
        afterHash: 'sha256:after',
        currentHash: 'sha256:after'
      }
    ],
    conversation: {
      status: 'audit_recorded',
      retainedEventCount: 2,
      removedEventCount: 3,
      removedTurnIds: ['turn_2']
    },
    summary: {
      fileAppliedCount: 1,
      fileNoopCount: 0,
      fileManualReviewCount: 0,
      fileBlockedCount: 0,
      fileFailedCount: 0
    },
    ...overrides
  }
}

const t = (key: string): string => {
  const labels: Record<string, string> = {
    rewindApplyUnavailable: 'unavailable',
    rewindApplyPrompt: 'confirm',
    rewindApplyPhraseMismatch: 'mismatch',
    rewindApplyApplied: 'applied',
    rewindApplyAlready: 'already',
    rewindApplyBlocked: 'blocked',
    rewindApplyFailed: 'failed'
  }
  return labels[key] ?? key
}

describe('RewindPlanApplyControls', () => {
  it('renders an enabled apply control for ready metadata-only rewind plans', () => {
    const html = renderToStaticMarkup(
      createElement(RewindPlanApplyControls, {
        plan: rewindPlan()
      })
    )

    expect(html).toContain('Apply rewind')
    expect(html).toContain('Requires explicit confirmation')
    expect(html).not.toContain('disabled=""')
  })

  it('enables snapshot-backed restore plans when runtime private CAS evidence is available', () => {
    const plan = rewindPlan({
      checkpoint: {
        schemaVersion: 1,
        checkpointId: 'axcp_1',
        threadId: 'thr_1',
        turnId: 'turn_1',
        workspace: '/workspace/analytix',
        createdAt: '2026-06-28T00:00:00.000Z',
        status: 'captured',
        snapshotStorage: 'runtime_private_cas',
        changedFiles: [
          {
            relativePath: 'src/app.ts',
            changeKind: 'modified',
            beforeHash: 'sha256:before',
            afterHash: 'sha256:after'
          }
        ]
      },
      files: [
        {
          relativePath: 'src/app.ts',
          changeKind: 'modified',
          action: 'restore_previous_version',
          status: 'ready',
          risk: 'medium',
          reason: 'runtime private snapshot can restore the previous file content',
          contentSource: 'checkpoint_metadata_only',
          beforeHash: 'sha256:before',
          afterHash: 'sha256:after'
        }
      ]
    })

    const html = renderToStaticMarkup(
      createElement(RewindPlanApplyControls, {
        plan
      })
    )

    expect(html).toContain('Apply rewind')
    expect(html).toContain('Requires explicit confirmation')
    expect(html).not.toContain('Snapshot content is not available')
    expect(html).not.toContain('disabled=""')
  })

  it('keeps restore plans disabled when runtime snapshot evidence is unavailable', () => {
    const plan = rewindPlan({
      checkpoint: {
        schemaVersion: 1,
        checkpointId: 'axcp_1',
        threadId: 'thr_1',
        turnId: 'turn_1',
        workspace: '/workspace/analytix',
        createdAt: '2026-06-28T00:00:00.000Z',
        status: 'captured',
        changedFiles: [
          {
            relativePath: 'src/app.ts',
            changeKind: 'modified',
            beforeHash: 'sha256:before',
            afterHash: 'sha256:after'
          }
        ]
      },
      files: [
        {
          relativePath: 'src/app.ts',
          changeKind: 'modified',
          action: 'restore_previous_version',
          status: 'ready',
          risk: 'medium',
          reason: 'manual snapshot evidence is unavailable',
          contentSource: 'checkpoint_metadata_only',
          beforeHash: 'sha256:before',
          afterHash: 'sha256:after'
        }
      ]
    })

    const html = renderToStaticMarkup(
      createElement(RewindPlanApplyControls, {
        plan
      })
    )

    expect(html).toContain('Apply rewind')
    expect(html).toContain('File content restore requires snapshot evidence')
    expect(html).toContain('disabled=""')
  })

  it('keeps blocked plans visible but disabled instead of silently dropping the product control', () => {
    const plan = rewindPlan({
      files: [
        {
          relativePath: 'src/secret.ts',
          changeKind: 'modified',
          action: 'blocked',
          status: 'blocked',
          risk: 'high',
          reason: 'staged change present',
          contentSource: 'checkpoint_metadata_only'
        }
      ],
      summary: {
        fileCount: 1,
        readyFileCount: 0,
        manualReviewFileCount: 0,
        blockedFileCount: 1,
        retainedEventCount: 2,
        removedEventCount: 3,
        removedTurnCount: 1,
        containsRawPrompt: false,
        containsFullFileContent: false,
        containsSecretValue: false
      }
    })

    const html = renderToStaticMarkup(
      createElement(RewindPlanApplyControls, {
        plan
      })
    )

    expect(html).toContain('Apply rewind')
    expect(html).toContain('Resolve blocked or review-required files before applying.')
    expect(html).toContain('disabled=""')
  })

  it('does not call the provider when the destructive confirmation phrase is missing', async () => {
    const applyCheckpointRewind = vi.fn(async () => applyResult())

    const result = await applyCheckpointRewindPlanWithConfirmation({
      plan: rewindPlan(),
      provider: { applyCheckpointRewind },
      prompt: vi.fn(() => 'nope'),
      t
    })

    expect(result).toEqual({ result: null, message: 'mismatch' })
    expect(applyCheckpointRewind).not.toHaveBeenCalled()
  })

  it('calls the Analytix provider apply contract after explicit confirmation', async () => {
    const plan = rewindPlan()
    const applied = applyResult()
    const applyCheckpointRewind = vi.fn(async () => applied)

    const result = await applyCheckpointRewindPlanWithConfirmation({
      plan,
      provider: { applyCheckpointRewind },
      prompt: vi.fn(() => 'APPLY_CHECKPOINT_REWIND'),
      t
    })

    expect(applyCheckpointRewind).toHaveBeenCalledWith('thr_1', 'axcp_1', plan)
    expect(result).toEqual({ result: applied, message: 'applied' })
  })
})
