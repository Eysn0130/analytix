import { createContext } from 'react'
import type { CoreThreadSummarySubagentJson } from '../../../../../agent/analytix-contract'
import type { ToolResultLocalSkillDataV1 } from '../../../../../../../../packages/runtime/src/contracts/tool-result-local-display'

/** A protected Skill observation supplies identity; the fresh parent summary admits the existing Inspector. */
export type SkillChildInspectionRequest = {
  parentThreadId: string
  parentTurnId: string
  parentToolCallId: string
  resultItemId: string
  child: NonNullable<ToolResultLocalSkillDataV1['child']>
}

export const SkillChildInspectionContext = createContext<((request: SkillChildInspectionRequest, signal: AbortSignal) => Promise<boolean>) | null>(null)

export function skillChildRelationMatches(agent: CoreThreadSummarySubagentJson, request: SkillChildInspectionRequest): boolean {
  const child = request.child
  return Boolean(child.childThreadId && agent.canOpenThread && (agent.taskKind === undefined || agent.taskKind === 'unknown' || agent.taskKind === 'subagent') &&
    agent.parentThreadId === request.parentThreadId && agent.parentTurnId === request.parentTurnId &&
    agent.parentToolCallId === request.parentToolCallId && agent.childRunId === child.jobId &&
    (agent.taskJobId === undefined || agent.taskJobId === child.jobId) && (agent.childId === undefined || agent.childId === child.jobId) &&
    agent.childThreadId === child.childThreadId && (child.childTurnId === null || agent.childTurnId === child.childTurnId))
}
