import type { ApprovalDecisionRequest, ApprovalDecisionResponse } from '../contracts/approvals.js'
import type {
  InterruptTurnRequest,
  InterruptTurnResponse,
  StartTurnRequest,
  StartTurnResponse,
  SteerTurnRequest,
  SteerTurnResponse,
  Turn
} from '../contracts/turns.js'
import type { RuntimeInfoResponse } from '../contracts/runtime-info.js'
import type { UserInputAnswer } from './user-input-gate.js'

export type RemoteEntryLifecycleControl = {
  info(): RuntimeInfoResponse
}

export type RemoteEntryStartTurnRequest = Pick<
  StartTurnRequest,
  'prompt' | 'displayText' | 'attachmentIds' | 'disableUserInput'
>

export type RemoteEntryTurnControl = {
  startTurn(threadId: string, request: RemoteEntryStartTurnRequest): Promise<StartTurnResponse>
  steerTurn(threadId: string, turnId: string, request: SteerTurnRequest): Promise<SteerTurnResponse>
  interruptTurn(
    threadId: string,
    turnId: string,
    request?: InterruptTurnRequest
  ): Promise<InterruptTurnResponse>
  getTurn(threadId: string, turnId: string): Promise<Turn | null>
}

export type RemoteEntryUserInputResolveRequest =
  | { cancelled: true; answers?: UserInputAnswer[] }
  | { cancelled?: false; answers?: UserInputAnswer[] }

export type RemoteEntryUserInputResolveResponse =
  | { inputId: string; status: 'cancelled' }
  | { inputId: string; status: 'submitted'; answers: UserInputAnswer[] }

export type RemoteEntryApprovalControl = {
  decideApproval(approvalId: string, request: ApprovalDecisionRequest): Promise<ApprovalDecisionResponse>
  resolveUserInput(
    inputId: string,
    request: RemoteEntryUserInputResolveRequest
  ): Promise<RemoteEntryUserInputResolveResponse>
}

/**
 * Narrow runtime surface for desktop-owned remote entrants such as scheduled
 * or bot-like relays. It intentionally omits thread storage, goals,
 * checkpoints, memory, tool hosts, and debug services.
 */
export type RemoteEntryControlPort = {
  lifecycle: RemoteEntryLifecycleControl
  turns: RemoteEntryTurnControl
  approvals: RemoteEntryApprovalControl
}
