import { composeFrameworkGuidance } from './pm-skill-frameworks'

export function composeSddAssistantPrompt(options: {
  userPrompt: string
  draftMarkdown: string
  draftRelativePath: string
  workspaceRoot: string
  /** Ids of PM-skill frameworks (see pm-skill-frameworks.ts) to apply this turn. */
  frameworkIds?: string[]
}): string {
  const frameworkGuidance = composeFrameworkGuidance(options.frameworkIds ?? [])
  return [
    'You are helping clarify and improve an SDD requirement draft inside Analytix.',
    `Workspace: ${options.workspaceRoot}`,
    `Draft file: ${options.draftRelativePath}`,
    '',
    ...(frameworkGuidance ? [frameworkGuidance, ''] : []),
    'Current draft:',
    '```markdown',
    options.draftMarkdown.trim() || '(empty draft)',
    '```',
    '',
    'User request:',
    options.userPrompt.trim(),
    '',
    'Answer with concrete requirement improvements, research notes, or questions.',
    'If the user asks you to update the draft, edit the draft file directly and keep the Markdown concise.'
  ].join('\n')
}
