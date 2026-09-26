import { z } from 'zod'
import {
  ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1,
  AcceptedFinalDeliveryBatchV2Schema,
  AcceptedFinalTerminalErrorItemV1Schema
} from './events.js'
import {
  acceptedFinalPublicViewsMatch,
  AcceptedFinalPublicAssistantTextTurnItemV3,
  AcceptedFinalPublicViewSchema,
  TurnItem
} from './items.js'
import { ThreadSchema, ThreadSummarySchema } from './threads.js'
import { TurnReasoningEffortSchema, TurnStatus } from './turns.js'

const PublicThreadDetailUsageV1Schema = z.object({
  promptTokens: z.number().int().nonnegative(),
  completionTokens: z.number().int().nonnegative(),
  reasoningTokens: z.number().int().nonnegative(),
  totalTokens: z.number().int().nonnegative(),
  cachedTokens: z.number().int().nonnegative(),
  cacheHitTokens: z.number().int().nonnegative(),
  cacheMissTokens: z.number().int().nonnegative(),
  cacheHitRate: z.number().min(0).max(1).nullable(),
  cacheableTokenHitRate: z.number().min(0).max(1).nullable(),
  totalInputTokenHitRate: z.number().min(0).max(1).nullable(),
  cacheMissReasons: z.array(z.string()),
  cacheSuggestions: z.array(z.string()),
  turns: z.number().int().nonnegative(),
  costUsd: z.number().nonnegative(),
  costCny: z.number().nonnegative(),
  priceConfigured: z.boolean(),
  cacheSavingsUsd: z.number().nonnegative(),
  cacheSavingsCny: z.number().nonnegative(),
  tokenEconomySavingsTokens: z.number().int().nonnegative(),
  tokenEconomySavingsUsd: z.number().nonnegative(),
  tokenEconomySavingsCny: z.number().nonnegative(),
  last_turn_cache_hit_rate: z.number().min(0).max(1).nullable(),
  last_turn_cacheable_hit_rate: z.number().min(0).max(1).nullable(),
  last_turn_total_input_hit_rate: z.number().min(0).max(1).nullable(),
  last_cache_miss_reasons: z.array(z.string()),
  last_cache_suggestions: z.array(z.string()),
  lastTurnCacheHitRate: z.number().min(0).max(1).nullable(),
  cache_hit_tokens: z.number().int().nonnegative(),
  cache_miss_tokens: z.number().int().nonnegative(),
  input_tokens: z.number().int().nonnegative(),
  output_tokens: z.number().int().nonnegative(),
  reasoning_tokens: z.number().int().nonnegative(),
  total_tokens: z.number().int().nonnegative(),
  cost_usd: z.number().nonnegative(),
  cost_cny: z.number().nonnegative(),
  price_configured: z.boolean(),
  token_economy_savings_tokens: z.number().int().nonnegative()
}).strict()

const CaseBoundaryTurnV1Schema = z.object({
  factHistoryState: z.literal('retained_snapshot').optional(),
  id: z.string().min(1),
  threadId: z.string().min(1),
  status: TurnStatus,
  model: z.string().optional(),
  reasoningEffort: TurnReasoningEffortSchema.optional(),
  createdAt: z.string(),
  startedAt: z.string().optional(),
  finishedAt: z.string().optional(),
  items: z.array(z.union([
    AcceptedFinalTerminalErrorItemV1Schema,
    AcceptedFinalPublicAssistantTextTurnItemV3,
    TurnItem
  ])),
  acceptedFinal: z.never().optional(),
  acceptedFinalView: AcceptedFinalPublicViewSchema.optional()
}).strict().superRefine((turn, ctx) => {
  const acceptedItems = turn.items.filter((item) =>
    item.kind === 'assistant_text' &&
    (('acceptedFinal' in item && item.acceptedFinal !== undefined) ||
      ('acceptedFinalView' in item && item.acceptedFinalView !== undefined))
  )
  if (turn.factHistoryState && !turn.acceptedFinalView) {
    ctx.addIssue({ code: 'custom', path: ['factHistoryState'], message: 'retained history requires accepted-final authority' })
  }
  if (!turn.acceptedFinal && !turn.acceptedFinalView) {
    if (acceptedItems.length !== 0) {
      ctx.addIssue({ code: 'custom', path: ['items'], message: 'case turn accepted-final authority is torn' })
    }
    return
  }
  const view = turn.acceptedFinalView
  const item = acceptedItems[0]
  const itemView = item?.kind === 'assistant_text' && 'acceptedFinalView' in item
    ? item.acceptedFinalView
    : undefined
  if (!view || turn.acceptedFinal !== undefined ||
      !AcceptedFinalPublicViewSchema.safeParse(view).success ||
      acceptedItems.length !== 1 || item?.kind !== 'assistant_text' ||
      item.threadId !== turn.threadId || item.turnId !== turn.id ||
      !acceptedFinalPublicViewsMatch(view, itemView)) {
    ctx.addIssue({ code: 'custom', path: ['acceptedFinal'], message: 'case turn accepted-final authority is invalid' })
  }
})

const ThreadDetailComputedV1 = {
  latestSeq: z.number().int().nonnegative().safe(),
  usage: PublicThreadDetailUsageV1Schema.optional(),
  pendingApprovalIds: z.array(z.string().min(1)),
  pendingUserInputIds: z.array(z.string().min(1)),
  preview: z.string().optional(),
  messageCount: z.number().int().nonnegative(),
  turnCount: z.number().int().nonnegative(),
  latestTurnId: z.string().min(1).optional(),
  acceptedFinalDelivery: AcceptedFinalDeliveryBatchV2Schema.optional(),
  acceptedFinalDeliveries: z.array(AcceptedFinalDeliveryBatchV2Schema)
    .max(ACCEPTED_FINAL_DELIVERY_GROUP_LIMIT_V1).optional()
} as const

const OrdinaryThreadDetailResponseV1Schema = ThreadSchema.extend({
  ...ThreadDetailComputedV1
}).strict()

const CaseThreadDetailResponseV1Schema = ThreadSummarySchema.omit({
  goal: true
}).extend({
  ...ThreadDetailComputedV1,
  historyAuthority: z.literal('case_boundary_only_v1'),
  turns: z.array(CaseBoundaryTurnV1Schema),
  goal: z.never().optional()
}).strict()

function visibleAcceptedFinals(
  turns: Array<{
    id: string
    items: Array<{ kind: string; acceptedFinalView?: { schemaVersion: 3; acceptedFinalDigest: string } }>
    acceptedFinalView?: { schemaVersion: 3; acceptedFinalDigest: string }
  }>
): Array<{ id: string; digest: string; assistantItem: unknown }> {
  const accepted: Array<{ id: string; digest: string; assistantItem: unknown }> = []
  for (const turn of turns) {
    const digest = turn.acceptedFinalView?.acceptedFinalDigest
    if (!digest) continue
    const assistantItems = turn.items.filter((item) =>
      item.kind === 'assistant_text' && item.acceptedFinalView?.acceptedFinalDigest === digest
    )
    if (assistantItems.length !== 1) continue
    accepted.push({ id: turn.id, digest, assistantItem: assistantItems[0] })
  }
  return accepted
}

function refineAcceptedFinalHydration(
  value: z.infer<typeof OrdinaryThreadDetailResponseV1Schema> |
    z.infer<typeof CaseThreadDetailResponseV1Schema>,
  ctx: z.RefinementCtx
): void {
  const accepted = visibleAcceptedFinals(value.turns)
  const latestDelivery = value.acceptedFinalDelivery
  const deliveries = value.acceptedFinalDeliveries
  if (accepted.length === 0 && !latestDelivery && !deliveries) return
  if (accepted.length === 0 || !deliveries || deliveries.length !== accepted.length ||
      (deliveries.length === 1 && latestDelivery !== undefined &&
        JSON.stringify(latestDelivery) !== JSON.stringify(deliveries[0])) ||
      (deliveries.length > 1 && latestDelivery !== undefined)) {
    ctx.addIssue({
      code: 'custom',
      path: ['acceptedFinalDelivery'],
      message: 'accepted-final hydration is detached from the thread snapshot'
    })
    return
  }

  let previousLastSeq = 0
  const seenTurns = new Set<string>()
  const seenCommits = new Set<string>()
  for (let index = 0; index < deliveries.length; index += 1) {
    const delivery = deliveries[index]
    const reference = accepted[index]
    const deliveryTurn = value.turns.find((turn) => turn.id === delivery.turnId)
    const assistantEvent = delivery.events[0]
    const assistantItem = assistantEvent?.kind === 'item_completed' ? assistantEvent.item : null
    const invalidIdentity = delivery.threadId !== value.id ||
      delivery.turnId !== reference.id ||
      delivery.publicationCommitId !== reference.digest ||
      delivery.firstSeq <= previousLastSeq ||
      delivery.lastSeq > value.latestSeq ||
      seenTurns.has(delivery.turnId) ||
      seenCommits.has(delivery.publicationCommitId) ||
      assistantItem === null ||
      JSON.stringify(assistantItem) !== JSON.stringify(reference.assistantItem)
    if (invalidIdentity) {
      ctx.addIssue({
        code: 'custom',
        path: ['acceptedFinalDeliveries', index],
        message: 'accepted-final delivery is detached from its exact public turn'
      })
      return
    }
    seenTurns.add(delivery.turnId)
    seenCommits.add(delivery.publicationCommitId)
    previousLastSeq = delivery.lastSeq

    const terminalErrorEvent = delivery.events.length === 4 ? delivery.events[1] : null
    const deliveryErrorItem = terminalErrorEvent?.kind === 'item_completed'
      ? terminalErrorEvent.item
      : null
    const matchingErrorItems = deliveryTurn?.items.filter((item) =>
      item.kind === 'error' &&
      'acceptedFinalDigest' in item &&
      item.acceptedFinalDigest === delivery.publicationCommitId
    ) ?? []
    if ((deliveryErrorItem === null && matchingErrorItems.length !== 0) ||
        (deliveryErrorItem !== null && (matchingErrorItems.length !== 1 ||
          JSON.stringify(matchingErrorItems[0]) !== JSON.stringify(deliveryErrorItem)))) {
      ctx.addIssue({
        code: 'custom',
        path: ['acceptedFinalDeliveries', index, 'events'],
        message: 'accepted-final terminal error item is detached from the thread snapshot'
      })
      return
    }
  }
  const lastAccepted = accepted[accepted.length - 1]
  const lastTurn = value.turns[value.turns.length - 1]
  const lastDelivery = deliveries[deliveries.length - 1]
  if (lastAccepted && lastTurn?.id === lastAccepted.id && lastDelivery.lastSeq !== value.latestSeq) {
    ctx.addIssue({
      code: 'custom',
      path: ['latestSeq'],
      message: 'latest accepted-final delivery is detached from the thread event frontier'
    })
  }
}

export const ThreadDetailResponseV1Schema = z.union([
  OrdinaryThreadDetailResponseV1Schema,
  CaseThreadDetailResponseV1Schema
]).superRefine(refineAcceptedFinalHydration)

export type ThreadDetailResponseV1 = z.infer<typeof ThreadDetailResponseV1Schema>
