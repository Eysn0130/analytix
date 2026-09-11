import { describe, expect, it } from 'vitest'
import { createApprovalRequest } from '../src/domain/approval.js'
import { createRemoteEntryControlPort, type RemoteEntryControlPortDeps } from '../src/services-test-support/remote-entry-control-port.js'
import type { RemoteEntryControlPort } from '../src/ports/remote-entry-control.js'
import { buildHarness } from './http-server-test-harness.js'

function assertRemoteEntryTypeBoundary(
  port: RemoteEntryControlPort,
  deps: RemoteEntryControlPortDeps
): void {
  void port.lifecycle.info
  void port.turns.startTurn
  void port.turns.steerTurn
  void port.turns.interruptTurn
  void port.turns.getTurn
  void port.approvals.decideApproval
  void port.approvals.resolveUserInput

  // @ts-expect-error remote entries must not receive goal controls
  void port.goal
  // @ts-expect-error remote entries must not receive checkpoint controls
  void port.checkpointRewindService
  // @ts-expect-error remote entries must not receive memory controls
  void port.memoryStore
  // @ts-expect-error remote entries must not receive storage/session controls
  void port.sessionStore
  // @ts-expect-error the narrow factory deps intentionally exclude the full thread service
  void deps.threadService
  // @ts-expect-error the narrow factory deps intentionally exclude checkpoint restore/apply
  void deps.checkpointRewindService
  // @ts-expect-error the narrow factory deps intentionally exclude long-term memory mutation
  void deps.memoryStore
  // @ts-expect-error the narrow factory deps intentionally exclude raw storage access
  void deps.sessionStore

  const startRequest: Parameters<RemoteEntryControlPort['turns']['startTurn']>[1] = {
    prompt: 'remote turn',
    disableUserInput: true
  }
  void startRequest
  // @ts-expect-error remote entries must not override approval policy
  startRequest.approvalPolicy = 'never'
  // @ts-expect-error remote entries must not override sandbox mode
  startRequest.sandboxMode = 'danger-full-access'
  // @ts-expect-error remote entries must not select a provider directly
  startRequest.providerId = 'external-provider'
  // @ts-expect-error remote entries must not attach arbitrary local paths
  startRequest.attachments = [{ path: '/tmp/secret', name: 'secret' }]
}

void assertRemoteEntryTypeBoundary

describe('remote entry control port', () => {
  it('exposes only lifecycle, turn control, approvals, and user-input gates', () => {
    const h = buildHarness()
    const port = h.runtime.remoteEntryControl

    expect(Object.keys(port).sort()).toEqual(['approvals', 'lifecycle', 'turns'])
    expect(Object.keys(port.lifecycle).sort()).toEqual(['info'])
    expect(Object.keys(port.turns).sort()).toEqual(['getTurn', 'interruptTurn', 'startTurn', 'steerTurn'])
    expect(Object.keys(port.approvals).sort()).toEqual(['decideApproval', 'resolveUserInput'])

    const record = port as unknown as Record<string, unknown>
    expect(record.threadService).toBeUndefined()
    expect(record.goal).toBeUndefined()
    expect(record.checkpointRewindService).toBeUndefined()
    expect(record.memoryStore).toBeUndefined()
    expect(record.sessionStore).toBeUndefined()
    expect(record.threadStore).toBeUndefined()
    expect(record.toolHost).toBeUndefined()
  })

  it('can drive lifecycle and turn controls without exposing unrelated control planes', async () => {
    const h = buildHarness()
    const port = createRemoteEntryControlPort({
      info: h.runtime.info,
      turns: h.turnService,
      approvals: h.approvalGate,
      userInputs: h.userInputGate,
      events: h.runtime.events
    })
    const thread = await h.threadService.create({
      title: 'Remote thread',
      workspace: '/tmp/analytix-remote',
      model: 'deepseek-chat',
      mode: 'agent'
    })

    expect(port.lifecycle.info()).toMatchObject({
      schemaVersion: 2,
      provider: {
        model: 'deepseek-chat'
      },
      executionPolicy: {
        approvalPolicy: 'on-request',
        sandboxMode: 'workspace-write'
      }
    })

    const started = await port.turns.startTurn(thread.id, {
      prompt: 'remote turn',
      disableUserInput: true
    })
    expect(started.threadId).toBe(thread.id)

    await expect(port.turns.startTurn(thread.id, {
      prompt: 'remote override',
      approvalPolicy: 'never'
    } as never)).rejects.toThrow()

    await expect(port.turns.steerTurn(thread.id, started.turnId, { text: 'adjust course' })).resolves.toMatchObject({
      ok: true,
      threadId: thread.id,
      turnId: started.turnId
    })
    await expect(port.turns.interruptTurn(thread.id, started.turnId, { discard: true })).resolves.toMatchObject({
      threadId: thread.id,
      turnId: started.turnId,
      status: 'aborted'
    })
    await expect(port.turns.getTurn(thread.id, started.turnId)).resolves.toMatchObject({
      id: started.turnId,
      status: 'aborted'
    })

    const events = await h.sessionStore.loadEventsSince(thread.id, 0)
    expect(events.map((event) => event.kind)).toEqual(
      expect.arrayContaining(['turn_started', 'turn_steered', 'turn_aborted'])
    )
  })

  it('resolves approvals and user input while keeping goal/checkpoint/memory storage unreachable', async () => {
    const h = buildHarness()
    const port = createRemoteEntryControlPort({
      info: h.runtime.info,
      turns: h.turnService,
      approvals: h.approvalGate,
      userInputs: h.userInputGate,
      events: h.runtime.events
    })
    const thread = await h.threadService.create({
      title: 'Remote gates',
      workspace: '/tmp/analytix-remote',
      model: 'deepseek-chat',
      mode: 'agent'
    })
    const started = await port.turns.startTurn(thread.id, { prompt: 'needs gates' })

    const approval = h.approvalGate.request(createApprovalRequest({
      id: 'approval_remote_1',
      threadId: thread.id,
      turnId: started.turnId,
      toolName: 'bash',
      summary: 'Run a command'
    }))
    await expect(port.approvals.decideApproval('approval_remote_1', {
      decision: 'deny',
      reason: 'remote entry denied'
    })).resolves.toEqual({
      approvalId: 'approval_remote_1',
      decision: 'deny',
      status: 'denied'
    })
    await expect(approval).resolves.toBe('deny')
    await expect(port.approvals.decideApproval('approval_remote_1', {
      decision: 'allow'
    })).rejects.toThrow('approval already decided: approval_remote_1')

    const input = h.userInputGate.request({
      id: 'input_remote_1',
      threadId: thread.id,
      turnId: started.turnId,
      itemId: 'item_input_remote_1',
      prompt: 'Pick one',
      questions: [{
        header: 'Choice',
        id: 'choice',
        question: 'Which option?',
        options: [
          { label: 'A', description: 'First option' },
          { label: 'B', description: 'Second option' }
        ]
      }]
    })
    const answers = [{ id: 'choice', label: 'A', value: 'A' }]
    await expect(port.approvals.resolveUserInput('input_remote_1', { answers })).resolves.toEqual({
      inputId: 'input_remote_1',
      status: 'submitted',
      answers
    })
    await expect(input).resolves.toEqual({ status: 'submitted', answers })

    const invalidInput = h.userInputGate.request({
      id: 'input_remote_invalid',
      threadId: thread.id,
      turnId: started.turnId,
      itemId: 'item_input_remote_invalid',
      prompt: 'Pick one again',
      questions: []
    })
    await expect(port.approvals.resolveUserInput('input_remote_invalid', {
      answers: [{ id: '', label: 'A', value: 'A' }]
    })).rejects.toThrow()
    await expect(port.approvals.resolveUserInput('input_remote_invalid', {
      cancelled: true,
      answers
    })).resolves.toEqual({
      inputId: 'input_remote_invalid',
      status: 'cancelled'
    })
    await expect(invalidInput).resolves.toEqual({ status: 'cancelled' })
    await expect(port.approvals.resolveUserInput('input_remote_invalid', {
      answers
    })).rejects.toThrow('user input not found: input_remote_invalid')

    const events = await h.sessionStore.loadEventsSince(thread.id, 0)
    expect(events.map((event) => event.kind)).toEqual(
      expect.arrayContaining(['approval_resolved', 'user_input_resolved'])
    )
    expect(events.filter((event) => event.kind === 'approval_resolved')).toHaveLength(1)
    expect(events.filter((event) => event.kind === 'user_input_resolved')).toHaveLength(2)
    expect((port as unknown as Record<string, unknown>).goal).toBeUndefined()
    expect((port as unknown as Record<string, unknown>).checkpointRewindService).toBeUndefined()
    expect((port as unknown as Record<string, unknown>).memoryStore).toBeUndefined()
    expect((port as unknown as Record<string, unknown>).sessionStore).toBeUndefined()
  })
})
