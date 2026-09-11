import type { DelegationRuntime } from '../../delegation-test-support/delegation-runtime.js'
import type { CapabilityToolProvider } from './capability-registry.js'
import { LocalToolHost } from './local-tool-host.js'

export function buildDelegationToolProviders(runtime: DelegationRuntime | undefined): CapabilityToolProvider[] {
  if (!runtime) return []
  const profiles = runtime.listProfiles()
  const profileNames = profiles.map((profile) => profile.name)
  return [{
    id: 'delegation',
    kind: 'delegation',
    enabled: true,
    available: true,
    tools: [
      LocalToolHost.defineTool({
        name: 'delegate_task',
        description: buildDelegateTaskDescription(runtime, profiles),
        inputSchema: {
          type: 'object',
          properties: {
            label: { type: 'string', description: 'Short label for this subagent run.' },
            prompt: { type: 'string', description: 'The task for the child agent.' },
            workspace: { type: 'string' },
            model: { type: 'string', description: 'Override the child model. Defaults to the profile model or server default.' },
            effort: { type: 'string', enum: ['off', 'low', 'medium', 'high', 'max'], description: 'Override the child reasoning effort. Defaults to the profile effort.' },
            tools: { type: 'array', items: { type: 'string' }, description: 'Optional explicit child tool scope. Read-only profiles still clamp to safe read tools.' },
            max_steps: { type: 'integer', minimum: 0, description: 'Optional child model/tool loop budget. 0 means unlimited.' },
            profile: profileNames.length
              ? { type: 'string', enum: profileNames, description: 'Subagent role to apply (model, effort, preamble, tool policy).' }
              : { type: 'string', description: 'Subagent role to apply (model, effort, preamble, tool policy).' }
          },
          required: ['prompt'],
          additionalProperties: false
        },
        policy: 'auto',
        toolKind: 'subagent',
        execute: async (args, context) => {
          const prompt = typeof args.prompt === 'string' ? args.prompt.trim() : ''
          if (!prompt) return { output: { error: 'prompt is required' }, isError: true }
          const maxModelSteps = resolveChildMaxModelSteps(context, args.max_steps)
          const record = await runtime.runChild({
            parentThreadId: context.threadId,
            parentTurnId: context.turnId,
            ...(context.toolCallId ? { parentToolCallId: context.toolCallId } : {}),
            label: typeof args.label === 'string' ? args.label : undefined,
            prompt,
            workspace: typeof args.workspace === 'string' ? args.workspace : context.workspace,
            ...(typeof args.model === 'string' ? { model: args.model } : {}),
            ...(context.modelExecution ? { modelExecution: context.modelExecution } : {}),
            ...(typeof args.effort === 'string' ? { effort: args.effort } : {}),
            ...(stringListArg(args.tools).length ? { tools: stringListArg(args.tools) } : {}),
            approvalPolicy: context.approvalPolicy,
            ...(context.sandboxMode ? { sandboxMode: context.sandboxMode } : {}),
            ...(maxModelSteps !== undefined ? { maxModelSteps } : {}),
            ...(typeof args.profile === 'string' ? { profile: args.profile } : {}),
            signal: context.abortSignal
          })
          return {
            output: {
              childId: record.id,
              status: record.status,
              summary: record.summary,
              error: record.error,
              usage: record.usage,
              ...(record.maxModelSteps !== undefined ? { maxModelSteps: record.maxModelSteps } : {}),
              ...(record.providerId ? { providerId: record.providerId } : {}),
              ...(record.endpointFormat ? { endpointFormat: record.endpointFormat } : {}),
              ...(record.variant ? { variant: record.variant } : {}),
              ...(record.modelSource ? { modelSource: record.modelSource } : {}),
              ...(record.modelExecution ? { modelExecution: record.modelExecution } : {}),
              ...(record.profile ? { profile: record.profile } : {}),
              ...(record.effort ? { effort: record.effort } : {}),
              ...(record.toolPolicy ? { toolPolicy: record.toolPolicy } : {}),
              ...(record.toolScope?.length ? { toolScope: record.toolScope } : {}),
              ...(record.toolInvocations !== undefined ? { toolInvocations: record.toolInvocations } : {}),
              ...(record.evidenceLedgered !== undefined ? { evidenceLedgered: record.evidenceLedgered } : {}),
              ...(record.evidenceLedgerError ? { evidenceLedgerError: record.evidenceLedgerError } : {}),
              ...(record.durationMs !== undefined ? { durationMs: record.durationMs } : {}),
              ...(record.queuedMs ? { queuedMs: record.queuedMs } : {})
            },
            isError: record.status === 'failed' || record.status === 'aborted'
          }
        }
      })
    ]
  }]
}

function buildDelegateTaskDescription(
  runtime: DelegationRuntime,
  profiles: {
    name: string
    toolPolicy: string
    providerId?: string
    model?: string
    variant?: string
    endpointFormat?: string
    effort?: string
    tools?: string[]
  }[]
): string {
  const lines = [
    'Run a bounded child agent task and return its summary.',
    'Issue several delegate_task calls in one message to investigate in parallel; runs queue once the parallel budget is full.',
    `Children default to the "${runtime.defaultToolPolicy}" tool policy (read-only children may only read/grep/find/ls and cannot edit, run shell, or delegate further).`
  ]
  if (profiles.length) {
    const summary = profiles
      .map((profile) => `${profile.name} (${profile.toolPolicy}${profile.providerId ? `, provider: ${profile.providerId}` : ''}${profile.model ? `, ${profile.model}` : ''}${profile.endpointFormat ? `, ${profile.endpointFormat}` : ''}${profile.variant ? `, ${profile.variant}` : ''}${profile.effort ? `, ${profile.effort}` : ''}${profile.tools?.length ? `, tools: ${profile.tools.join('/')}` : ''})`)
      .join('; ')
    lines.push(`Available profiles: ${summary}.`)
  }
  if (runtime.defaultProfileName) {
    lines.push(`Default profile when omitted: ${runtime.defaultProfileName}.`)
  }
  return lines.join(' ')
}

function stringListArg(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const tools: string[] = []
  for (const item of value) {
    if (typeof item !== 'string') continue
    const name = item.trim()
    if (!name || seen.has(name)) continue
    seen.add(name)
    tools.push(name)
  }
  return tools.sort()
}

export function resolveChildMaxModelSteps(context: {
  runtimeStepLimits?: { currentMaxModelSteps: number }
}, explicitValue: unknown): number | undefined {
  if (typeof explicitValue === 'number' && Number.isFinite(explicitValue) && explicitValue >= 0) {
    return Math.floor(explicitValue)
  }
  const parentMax = context.runtimeStepLimits?.currentMaxModelSteps
  if (typeof parentMax !== 'number' || !Number.isFinite(parentMax)) return undefined
  if (parentMax <= 0) return 0
  return Math.max(5, Math.floor(parentMax / 2))
}
