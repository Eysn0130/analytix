import { z } from 'zod'
import { ApprovalDecisionRequest, type ApprovalDecisionResponse } from '../contracts/approvals.js'
import { InterruptTurnRequest, StartTurnRequest, SteerTurnRequest, type SteerTurnResponse } from '../contracts/turns.js'
import type { RuntimeInfoResponse } from '../contracts/runtime-info.js'
import type {
  RemoteEntryControlPort,
  RemoteEntryUserInputResolveResponse
} from '../ports/remote-entry-control.js'
import type { ApprovalGate } from '../ports/approval-gate.js'
import type { UserInputGate, UserInputResolution } from '../ports/user-input-gate.js'
import type { RuntimeEventRecorder } from './runtime-event-recorder.js'
import type { TurnService } from './turn-service.js'

const RemoteEntryUserInputAnswerSchema = z.object({
  id: z.string().min(1),
  label: z.string().min(1),
  value: z.string().default('')
})

const RemoteEntryUserInputResolveRequest = z.object({
  answers: z.array(RemoteEntryUserInputAnswerSchema).optional(),
  cancelled: z.boolean().optional()
})

const RemoteEntryStartTurnRequest = StartTurnRequest.pick({
  prompt: true,
  displayText: true,
  attachmentIds: true,
  disableUserInput: true
}).strict()

export type RemoteEntryControlPortDeps = {
  info: () => RuntimeInfoResponse
  turns: Pick<TurnService, 'startTurn' | 'steerTurn' | 'interruptTurn' | 'getTurn'>
  approvals: ApprovalGate
  userInputs: UserInputGate
  events: Pick<RuntimeEventRecorder, 'record'>
  runTurn?: (threadId: string, turnId: string) => Promise<'completed' | 'failed' | 'aborted'> | void
}

export function createRemoteEntryControlPort(deps: RemoteEntryControlPortDeps): RemoteEntryControlPort {
  return {
    lifecycle: {
      info: deps.info
    },
    turns: {
      async startTurn(threadId, request) {
        const parsed = RemoteEntryStartTurnRequest.parse(request)
        const response = await deps.turns.startTurn({ threadId, request: parsed })
        deps.runTurn?.(response.threadId, response.turnId)
        return response
      },
      async steerTurn(threadId, turnId, request) {
        const parsed = SteerTurnRequest.parse(request)
        const response: SteerTurnResponse = await deps.turns.steerTurn({ threadId, turnId, request: parsed })
        return response
      },
      async interruptTurn(threadId, turnId, request = {}) {
        const parsed = InterruptTurnRequest.parse(request)
        const result = await deps.turns.interruptTurn({ threadId, turnId, discard: parsed.discard })
        return { threadId, turnId, status: result.status }
      },
      getTurn(threadId, turnId) {
        return deps.turns.getTurn(threadId, turnId)
      }
    },
    approvals: {
      async decideApproval(approvalId, request) {
        const parsed = ApprovalDecisionRequest.parse(request)
        const approval = deps.approvals.get(approvalId)
        if (!approval) throw new Error(`approval not found: ${approvalId}`)
        const ok = deps.approvals.decide(approvalId, parsed.decision, parsed.reason)
        if (!ok) throw new Error(`approval already decided: ${approvalId}`)
        const response: ApprovalDecisionResponse = {
          approvalId,
          decision: parsed.decision,
          status: parsed.decision === 'allow' ? 'allowed' : 'denied'
        }
        await deps.events.record({
          kind: 'approval_resolved',
          threadId: approval.threadId,
          turnId: approval.turnId,
          itemId: undefined,
          approvalId,
          toolName: approval.toolName,
          status: response.status,
          summary: approval.summary
        })
        return response
      },
      async resolveUserInput(inputId, request) {
        const parsed = RemoteEntryUserInputResolveRequest.parse(request)
        const pending = deps.userInputs.get(inputId)
        if (!pending) throw new Error(`user input not found: ${inputId}`)
        const resolution: UserInputResolution = parsed.cancelled
          ? { status: 'cancelled' }
          : { status: 'submitted', answers: parsed.answers ?? [] }
        const ok = deps.userInputs.resolve(inputId, resolution)
        if (!ok) throw new Error(`user input already resolved: ${inputId}`)
        await deps.events.record({
          kind: 'user_input_resolved',
          threadId: pending.threadId,
          turnId: pending.turnId,
          itemId: pending.itemId,
          inputId,
          status: resolution.status,
          prompt: pending.prompt
        })
        return userInputResponse(inputId, resolution)
      }
    }
  }
}

function userInputResponse(
  inputId: string,
  resolution: UserInputResolution
): RemoteEntryUserInputResolveResponse {
  if (resolution.status === 'cancelled') {
    return { inputId, status: 'cancelled' }
  }
  return { inputId, status: 'submitted', answers: resolution.answers }
}
