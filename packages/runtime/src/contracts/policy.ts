import { z } from 'zod'

export const APPROVAL_POLICIES = [
  'always',
  'on-request',
  'untrusted',
  'never',
  'auto',
  'suggest'
] as const
export const DEFAULT_APPROVAL_POLICY = 'on-request'

export const ApprovalPolicySchema = z.enum(APPROVAL_POLICIES)
export type ApprovalPolicy = z.infer<typeof ApprovalPolicySchema>

export const SANDBOX_MODES = [
  'read-only',
  'workspace-write',
  'danger-full-access',
  'external-sandbox'
] as const
export const DEFAULT_SANDBOX_MODE = 'workspace-write'

/**
 * Version of the persisted approval/sandbox default semantics. This is kept
 * separate from the application settings schema version so policy migrations
 * remain explicit and narrowly scoped.
 */
export const CURRENT_EXECUTION_POLICY_VERSION = 2 as const

export const SandboxModeSchema = z.enum(SANDBOX_MODES)
export type SandboxMode = z.infer<typeof SandboxModeSchema>
