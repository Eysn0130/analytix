import { z } from 'zod'

const Sha256HexV1 = z.string().regex(/^[0-9a-f]{64}$/)
const OpaqueObservationIDV1 = z.string()
  .min(1)
  .max(4096)
  .refine((value) => value === value.trim())

// Closed metadata-only projection of one host-verified ordinary tool
// execution. Commands, paths, arguments, output, case data, and PII are not
// part of this public contract.
export const SuccessfulToolExecutionObservationV1 = z.object({
  schemaVersion: z.literal(1),
  disclosure: z.literal('metadata_only'),
  privatePayloadWithheld: z.literal(true),
  threadId: OpaqueObservationIDV1,
  turnId: OpaqueObservationIDV1,
  toolName: z.enum(['bash', 'read']),
  status: z.literal('completed'),
  workId: Sha256HexV1,
  receiptId: Sha256HexV1,
  dispositionId: Sha256HexV1,
  executionGrantId: Sha256HexV1,
  resultItemId: z.string().regex(/^item_result_[0-9a-f]{64}$/),
  resultItemDigest: Sha256HexV1
}).strict()

export type SuccessfulToolExecutionObservationV1 = z.infer<
  typeof SuccessfulToolExecutionObservationV1
>
