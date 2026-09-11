import { z } from 'zod'
import { ThreadSummarySchema } from './threads.js'

export const CaseProjectSummaryV1Schema = z.object({
  id: z.string().min(1),
  name: z.string(),
  rootPath: z.string().min(1),
  updatedAt: z.string(),
  threadCount: z.number().int().nonnegative(),
  runningCount: z.number().int().nonnegative(),
  archivedCount: z.number().int().nonnegative(),
  lastThreadId: z.string(),
  lastPreview: z.string(),
  dataSizeEstimate: z.number().int().nonnegative(),
  status: z.literal('ready')
}).strict()
export type CaseProjectSummaryV1 = z.infer<typeof CaseProjectSummaryV1Schema>

export const CaseProjectThreadSummaryV1Schema = ThreadSummarySchema.extend({
  historyAuthority: z.literal('case_boundary_only_v1').optional(),
  archived: z.boolean().optional(),
  pinned: z.boolean().optional(),
  caseProjectId: z.string().min(1).optional(),
  caseId: z.string().min(1).optional(),
  latestTurnId: z.string().min(1).optional(),
  hasRunningTurn: z.boolean().optional()
}).strict()
export type CaseProjectThreadSummaryV1 = z.infer<typeof CaseProjectThreadSummaryV1Schema>

export const CaseProjectListResponseV1Schema = z.object({
  caseProjects: z.array(CaseProjectSummaryV1Schema),
  indexStatus: z.enum(['ready', 'building'])
}).strict()
export type CaseProjectListResponseV1 = z.infer<typeof CaseProjectListResponseV1Schema>

export const CaseProjectThreadsResponseV1Schema = z.object({
  threads: z.array(CaseProjectThreadSummaryV1Schema)
}).strict()
export type CaseProjectThreadsResponseV1 = z.infer<typeof CaseProjectThreadsResponseV1Schema>

export const CaseProjectDetailResponseV1Schema = z.object({
  project: CaseProjectSummaryV1Schema,
  threads: z.array(CaseProjectThreadSummaryV1Schema)
}).strict()
export type CaseProjectDetailResponseV1 = z.infer<typeof CaseProjectDetailResponseV1Schema>
