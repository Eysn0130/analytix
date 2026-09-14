import { z } from 'zod'

// Protected local-display transport, consumed only by Electron Main. This is a
// current private installation witness, never publication or file-edit authority.
export const officePrivateAdmissionPath = '/v1/local-display/office-private-admission'
export const officePrivateAdmissionRequestSchema = z.object({requestId:z.string().uuid()}).strict()
export const officePrivateAdmissionResponseSchema = z.discriminatedUnion('ok', [
  z.object({ok:z.literal(true),requestId:z.string().uuid(),root:z.string().min(1).max(32768).refine(value=>!value.includes('\0')),qualificationDigest:z.string().regex(/^[a-f0-9]{64}$/)}).strict(),
  z.object({ok:z.literal(false),code:z.literal('unavailable')}).strict()
])
export type OfficePrivateAdmissionRequest = z.infer<typeof officePrivateAdmissionRequestSchema>
export type OfficePrivateAdmissionResponse = z.infer<typeof officePrivateAdmissionResponseSchema>
