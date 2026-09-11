import type { ChatBlock, NormalizedThread } from '../agent/types'
import { sanitizePublicSerializedText } from '@shared/public-runtime-content'
import { projectOrdinaryPublicText } from '@shared/ordinary-log-pii-projection'

function publicMarkdownText(value: string): string {
  return projectOrdinaryPublicText(sanitizePublicSerializedText(value))
}

function threadBlockToMarkdown(block: ChatBlock): string | null {
  if (block.kind === 'user') {
    const text = projectOrdinaryPublicText(block.text)
    return text.trim() ? `## User\n\n${text}` : null
  }
  if (block.kind === 'assistant') {
    const text = publicMarkdownText(block.text)
    return text ? `## Assistant\n\n${text}` : null
  }
  if (block.kind === 'system') return `## System\n\n${publicMarkdownText(block.text)}`
  if (block.kind === 'tool') {
    const detail = publicMarkdownText(block.detail ?? '')
    return `## Tool\n\n${publicMarkdownText(block.summary)}${detail ? `\n\n\`\`\`text\n${detail}\n\`\`\`` : ''}`
  }
  if (block.kind === 'compaction') {
    const summary = publicMarkdownText(block.summary)
    return summary ? `## Compaction\n\n${summary}` : null
  }
  if (block.kind === 'review') return `## Review\n\n${publicMarkdownText(block.reviewText ?? block.title)}`
  if (block.kind === 'approval') return `## Approval\n\n${publicMarkdownText(block.summary)}`
  if (block.kind === 'user_input') return `## User Input\n\n${publicMarkdownText(block.questions.map((q) => q.question).join('\n'))}`
  return null
}

export function buildThreadMarkdown(thread: NormalizedThread, workspaceLabel: string, blocks: ChatBlock[]): string {
  const sections = blocks
    .map(threadBlockToMarkdown)
    .filter((value): value is string => Boolean(value && value.trim()))

  return [
    `# ${projectOrdinaryPublicText(thread.title)}`,
    '',
    `- Thread ID: ${thread.id}`,
    `- Workspace: ${projectOrdinaryPublicText(thread.workspace ?? workspaceLabel)}`,
    `- Updated: ${thread.updatedAt}`,
    '',
    ...sections
  ].join('\n')
}
