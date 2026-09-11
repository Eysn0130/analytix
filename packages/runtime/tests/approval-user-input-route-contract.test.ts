import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { dispatchRequest } from '../src/server-test-support/http-server.js'
import { createApprovalRequest } from '../src/domain/approval.js'
import { makeApprovalItem, makeUserInputItem } from '../src/domain/item.js'
import { ApprovalUserInputRouteContract } from '../src/conformance/runtime-parity-fixtures.js'
import {
  buildHarness,
  readJson,
  readSseEvents
} from './http-server-test-harness.js'

const contractUrl = new URL(
  '../src/conformance/fixtures/approval-user-input-route-contract.json',
  import.meta.url
)

function loadContract(): ApprovalUserInputRouteContract {
  return ApprovalUserInputRouteContract.parse(
    JSON.parse(readFileSync(contractUrl, 'utf8'))
  )
}

function authorizedRequest(
  url: string,
  token: string,
  init: RequestInit = {}
): Request {
  const headers = new Headers(init.headers)
  headers.set('authorization', `Bearer ${token}`)
  if (init.body && !headers.has('content-type')) {
    headers.set('content-type', 'application/json')
  }
  return new Request(url, { ...init, headers })
}

function eventKindsFromFrames(frames: string[]): string[] {
  return frames.flatMap((frame) =>
    frame
      .split('\n')
      .filter((line) => line.startsWith('event:'))
      .map((line) => line.slice(7).trim())
  )
}

function eventSeqsFromFrames(frames: string[]): number[] {
  return frames.flatMap((frame) =>
    frame
      .split('\n')
      .filter((line) => line.startsWith('id:'))
      .map((line) => Number(line.slice(3).trim()))
  )
}

function eventPayloadsFromFrames(frames: string[]): Array<Record<string, unknown>> {
  return frames.flatMap((frame) =>
    frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => JSON.parse(line.slice(5).trim()) as Record<string, unknown>)
  )
}

describe('approval/user-input route conformance contracts', () => {
  it('pins deny, cancel, replay, resume, and remote-entry gate behavior at the API boundary', async () => {
    const contract = loadContract()
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp/analytix-gates',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    }, { id: contract.threadId, title: 'GUI gate contracts' })
    const started = await h.turnService.startTurn({
      threadId: contract.threadId,
      request: { prompt: 'requires GUI gates' }
    })

    await h.turnService.applyItem(
      contract.threadId,
      makeApprovalItem({
        id: contract.approval.itemId,
        threadId: contract.threadId,
        turnId: started.turnId,
        approvalId: contract.approval.id,
        toolName: contract.approval.toolName,
        summary: contract.approval.summary
      })
    )
    await h.runtime.events.record({
      kind: 'approval_requested',
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: contract.approval.itemId,
      approvalId: contract.approval.id,
      toolName: contract.approval.toolName,
      status: 'pending',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      summary: contract.approval.summary
    })
    const approvalPending = h.approvalGate.request(createApprovalRequest({
      id: contract.approval.id,
      threadId: contract.threadId,
      turnId: started.turnId,
      toolName: contract.approval.toolName,
      summary: contract.approval.summary
    }))
    expect(h.approvalGate.pending(contract.threadId)).toHaveLength(contract.approval.pendingBefore)

    const deny = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/approvals/${contract.approval.id}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify(contract.approval.decisionRequest)
      })
    )
    expect(deny.status).toBe(contract.approval.expectedResponse.status)
    expect(await readJson(deny)).toEqual(contract.approval.expectedResponse.body)
    await expect(approvalPending).resolves.toBe('deny')
    expect(h.approvalGate.pending(contract.threadId)).toHaveLength(contract.approval.pendingAfter)

    const denyAgain = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/approvals/${contract.approval.id}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify({ decision: 'allow' })
      })
    )
    expect(denyAgain.status).toBe(contract.approval.secondDecisionStatus)

    await h.turnService.applyItem(
      contract.threadId,
      makeUserInputItem({
        id: contract.userInput.itemId,
        threadId: contract.threadId,
        turnId: started.turnId,
        inputId: contract.userInput.id,
        prompt: contract.userInput.prompt,
        questions: contract.userInput.questions
      })
    )
    await h.runtime.events.record({
      kind: 'user_input_requested',
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: contract.userInput.itemId,
      inputId: contract.userInput.id,
      status: 'pending',
      prompt: contract.userInput.prompt,
      questions: contract.userInput.questions
    })
    const userInputPending = h.userInputGate.request({
      id: contract.userInput.id,
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: contract.userInput.itemId,
      prompt: contract.userInput.prompt,
      questions: contract.userInput.questions
    })
    expect(h.userInputGate.pending(contract.threadId)).toHaveLength(contract.userInput.pendingBefore)

    const cancel = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/user-inputs/${contract.userInput.id}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify(contract.userInput.resolveRequest)
      })
    )
    expect(cancel.status).toBe(contract.userInput.expectedResponse.status)
    expect(await readJson(cancel)).toEqual(contract.userInput.expectedResponse.body)
    await expect(userInputPending).resolves.toEqual({ status: 'cancelled' })
    expect(h.userInputGate.pending(contract.threadId)).toHaveLength(contract.userInput.pendingAfter)

    const cancelAgain = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/user-inputs/${contract.userInput.id}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify(contract.userInput.resolveRequest)
      })
    )
    expect(cancelAgain.status).toBe(contract.userInput.secondResolveStatus)

    const eventStream = await dispatchRequest(
      h.router,
      authorizedRequest(
        `http://localhost/v1/threads/${contract.threadId}/events?since_seq=${contract.replay.sinceSeq}`,
        contract.runtimeToken
      )
    )
    const frames = await readSseEvents(eventStream)
    const kinds = eventKindsFromFrames(frames)
      .filter((kind) => contract.replay.expectedKindsInOrder.includes(kind))
    expect(kinds).toEqual(contract.replay.expectedKindsInOrder)
    const seqs = eventSeqsFromFrames(frames)
    expect(new Set(seqs).size).toBe(seqs.length)

    await h.runtime.events.record({
      kind: 'approval_requested',
      threadId: contract.threadId,
      turnId: started.turnId,
      approvalId: contract.abortCleanup.approvalId,
      toolName: contract.approval.toolName,
      status: 'pending',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write',
      summary: 'approval interrupted before GUI decision'
    })
    const abortApprovalPending = h.approvalGate.request(createApprovalRequest({
      id: contract.abortCleanup.approvalId,
      threadId: contract.threadId,
      turnId: started.turnId,
      toolName: contract.approval.toolName,
      summary: 'approval interrupted before GUI decision'
    }))
    expect(h.approvalGate.expire(contract.abortCleanup.approvalId, 'turn aborted while awaiting approval'))
      .toBe(true)
    await h.runtime.events.record({
      kind: 'approval_resolved',
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: undefined,
      approvalId: contract.abortCleanup.approvalId,
      toolName: contract.approval.toolName,
      status: contract.abortCleanup.expectedApprovalStatus,
      summary: 'approval interrupted before GUI decision'
    })
    await expect(abortApprovalPending).rejects.toThrow('turn aborted while awaiting approval')
    expect(h.approvalGate.get(contract.abortCleanup.approvalId)?.status)
      .toBe(contract.abortCleanup.expectedApprovalStatus)
    const lateApproval = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/approvals/${contract.abortCleanup.approvalId}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify({ decision: 'allow' })
      })
    )
    expect(lateApproval.status).toBe(contract.abortCleanup.lateApprovalDecisionStatus)

    await h.runtime.events.record({
      kind: 'user_input_requested',
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: `item_${contract.abortCleanup.userInputId}`,
      inputId: contract.abortCleanup.userInputId,
      status: 'pending',
      prompt: 'input interrupted before GUI answer',
      questions: []
    })
    const abortUserInputPending = h.userInputGate.request({
      id: contract.abortCleanup.userInputId,
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: `item_${contract.abortCleanup.userInputId}`,
      prompt: 'input interrupted before GUI answer',
      questions: []
    })
    expect(h.userInputGate.resolve(contract.abortCleanup.userInputId, { status: 'cancelled' }))
      .toBe(true)
    await h.runtime.events.record({
      kind: 'user_input_resolved',
      threadId: contract.threadId,
      turnId: started.turnId,
      itemId: `item_${contract.abortCleanup.userInputId}`,
      inputId: contract.abortCleanup.userInputId,
      status: contract.abortCleanup.expectedUserInputStatus,
      prompt: 'input interrupted before GUI answer',
      questions: []
    })
    await expect(abortUserInputPending).resolves.toEqual({ status: 'cancelled' })
    const lateUserInput = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/user-inputs/${contract.abortCleanup.userInputId}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify({ answers: [] })
      })
    )
    expect(lateUserInput.status).toBe(contract.abortCleanup.lateUserInputResolveStatus)
    const cleanupKinds = (await h.sessionStore.loadEventsSince(contract.threadId, 0))
      .filter((event) =>
        ('approvalId' in event && event.approvalId === contract.abortCleanup.approvalId) ||
        ('inputId' in event && event.inputId === contract.abortCleanup.userInputId)
      )
      .map((event) => event.kind)
    expect(cleanupKinds).toEqual(contract.abortCleanup.expectedReplayKinds)

    await h.threadService.create({
      workspace: '/tmp/analytix-resume-gates',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request'
    }, { id: contract.resumePendingGates.sourceThreadId, title: 'Resume pending gates' })
    const pendingResumeTurn = await h.turnService.startTurn({
      threadId: contract.resumePendingGates.sourceThreadId,
      request: { prompt: 'pending gates should not stay actionable after resume' }
    })
    await h.turnService.applyItem(
      contract.resumePendingGates.sourceThreadId,
      makeApprovalItem({
        id: 'item_resume_approval',
        threadId: contract.resumePendingGates.sourceThreadId,
        turnId: pendingResumeTurn.turnId,
        approvalId: contract.resumePendingGates.approvalId,
        toolName: 'bash',
        summary: 'pending approval before resume'
      })
    )
    await h.turnService.applyItem(
      contract.resumePendingGates.sourceThreadId,
      makeUserInputItem({
        id: 'item_resume_input',
        threadId: contract.resumePendingGates.sourceThreadId,
        turnId: pendingResumeTurn.turnId,
        inputId: contract.resumePendingGates.userInputId,
        prompt: 'pending user input before resume'
      })
    )
    const source = await h.threadService.get(contract.resumePendingGates.sourceThreadId)
    const sourceItems = source?.turns.at(-1)?.items ?? []
    expect(sourceItems.find((item) => item.kind === 'approval')?.status)
      .toBe(contract.resumePendingGates.expectedSourceStatuses.approval)
    expect(sourceItems.find((item) => item.kind === 'user_input')?.status)
      .toBe(contract.resumePendingGates.expectedSourceStatuses.userInput)

    const resume = await dispatchRequest(
      h.router,
      authorizedRequest(
        `http://localhost/v1/sessions/${contract.resumePendingGates.sourceThreadId}/resume-thread`,
        contract.runtimeToken,
        {
          method: 'POST',
          body: JSON.stringify({ workspace: '/tmp/analytix-resumed' })
        }
      )
    )
    expect(resume.status).toBe(201)
    const resumeBody = await readJson(resume) as { thread_id: string }
    const resumed = await h.threadService.get(resumeBody.thread_id)
    const resumedItems = resumed?.turns.at(-1)?.items ?? []
    expect(resumedItems.find((item) => item.kind === 'approval')?.status)
      .toBe(contract.resumePendingGates.expectedResumedStatuses.approval)
    expect(resumedItems.find((item) => item.kind === 'user_input')?.status)
      .toBe(contract.resumePendingGates.expectedResumedStatuses.userInput)

    await h.threadService.create({
      workspace: '/tmp/analytix-remote-entry',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request',
      sandboxMode: 'workspace-write'
    }, { id: contract.remoteEntry.threadId, title: 'Remote entry gates' })
    const port = h.runtime.remoteEntryControl
    expect(Object.keys(port).sort()).toEqual([...contract.remoteEntry.expectedPortKeys].sort())
    const portRecord = port as unknown as Record<string, unknown>
    for (const key of contract.remoteEntry.forbiddenPortKeys) {
      expect(portRecord[key]).toBeUndefined()
    }
    const remoteStart = await port.turns.startTurn(contract.remoteEntry.threadId, contract.remoteEntry.startRequest)
    expect(remoteStart.threadId).toBe(contract.remoteEntry.threadId)
    const remoteThread = await h.threadService.get(contract.remoteEntry.threadId)
    expect(remoteThread?.approvalPolicy).toBe('on-request')
    expect(remoteThread?.sandboxMode).toBe('workspace-write')
    await expect(port.turns.startTurn(contract.remoteEntry.threadId, {
      ...contract.remoteEntry.startRequest,
      ...contract.remoteEntry.rejectedOverride
    } as never)).rejects.toThrow()
  })

  it('pins submitted user-input answers to the HTTP response without replaying answers in SSE', async () => {
    const contract = loadContract()
    const submitted = contract.submittedUserInput
    const h = buildHarness()
    await h.threadService.create({
      workspace: '/tmp/analytix-submit-gates',
      model: 'deepseek-chat',
      mode: 'agent',
      approvalPolicy: 'on-request'
    }, { id: submitted.threadId, title: 'GUI submitted input fixture' })
    const started = await h.turnService.startTurn({
      threadId: submitted.threadId,
      request: { prompt: 'submit GUI input' }
    })

    await h.turnService.applyItem(
      submitted.threadId,
      makeUserInputItem({
        id: submitted.itemId,
        threadId: submitted.threadId,
        turnId: started.turnId,
        inputId: submitted.id,
        prompt: submitted.prompt,
        questions: submitted.questions
      })
    )
    await h.runtime.events.record({
      kind: 'user_input_requested',
      threadId: submitted.threadId,
      turnId: started.turnId,
      itemId: submitted.itemId,
      inputId: submitted.id,
      status: 'pending',
      prompt: submitted.prompt,
      questions: submitted.questions
    })
    const pending = h.userInputGate.request({
      id: submitted.id,
      threadId: submitted.threadId,
      turnId: started.turnId,
      itemId: submitted.itemId,
      prompt: submitted.prompt,
      questions: submitted.questions
    })
    expect(h.userInputGate.pending(submitted.threadId)).toHaveLength(submitted.pendingBefore)

    const response = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/user-inputs/${submitted.id}`, contract.runtimeToken, {
        method: 'POST',
        body: JSON.stringify(submitted.resolveRequest)
      })
    )
    expect(response.status).toBe(submitted.expectedResponse.status)
    expect(await readJson(response)).toEqual(submitted.expectedResponse.body)
    await expect(pending).resolves.toEqual(submitted.expectedResolution)
    expect(h.userInputGate.pending(submitted.threadId)).toHaveLength(submitted.pendingAfter)

    const eventStream = await dispatchRequest(
      h.router,
      authorizedRequest(`http://localhost/v1/threads/${submitted.threadId}/events?since_seq=0`, contract.runtimeToken)
    )
    const payloads = eventPayloadsFromFrames(await readSseEvents(eventStream))
    const resolved = payloads.find((event) =>
      event.kind === submitted.resolvedEvent.kind && event.inputId === submitted.id
    )
    expect(resolved).toMatchObject({
      kind: submitted.resolvedEvent.kind,
      inputId: submitted.id,
      status: submitted.resolvedEvent.status
    })
    if (submitted.resolvedEvent.includesAnswers) {
      expect(resolved).toHaveProperty('answers')
    } else {
      expect(resolved).not.toHaveProperty('answers')
    }
  })
})
