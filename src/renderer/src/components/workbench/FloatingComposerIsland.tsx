import type { ComponentProps, ReactElement } from 'react'
import { useMemo } from 'react'
import { collectComposerChangeSummary } from '../../lib/composer-change-summary'
import { useChatStore } from '../../store/chat-store'
import { FloatingComposer } from '../chat/FloatingComposer'

type FloatingComposerIslandProps = Omit<
  ComponentProps<typeof FloatingComposer>,
  'changedFiles' | 'changedFileStats'
> & {
  activeSkillWorkspace: string
}

export function FloatingComposerIsland({
  activeSkillWorkspace,
  ...props
}: FloatingComposerIslandProps): ReactElement {
  const blocks = useChatStore((s) => s.blocks)
  const composerChangeSummary = useMemo(
    () => collectComposerChangeSummary(blocks, activeSkillWorkspace),
    [activeSkillWorkspace, blocks]
  )
  return (
    <FloatingComposer
      {...props}
      changedFiles={composerChangeSummary?.files}
      changedFileStats={composerChangeSummary}
    />
  )
}
