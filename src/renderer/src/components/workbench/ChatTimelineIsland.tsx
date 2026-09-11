import type { ComponentProps, ReactElement } from 'react'
import { useShallow } from 'zustand/react/shallow'
import type { ChatBlock } from '../../agent/types'
import { useChatStore } from '../../store/chat-store'
import { MessageTimeline } from '../chat/MessageTimeline'

type ChatTimelineIslandProps = Omit<
  ComponentProps<typeof MessageTimeline>,
  'blocks' | 'live'
>

export function useChatTimelineBlocks(): {
  blocks: ChatBlock[]
} {
  return useChatStore(
    useShallow((s) => ({
      blocks: s.blocks
    }))
  )
}

export function useChatTimelinePanelState(): {
  blocks: ChatBlock[]
  hasLiveStream: boolean
} {
  return useChatStore(
    useShallow((s) => ({
      blocks: s.blocks,
      hasLiveStream: Boolean(
        s.currentTurnId || s.liveAssistant.trim()
      )
    }))
  )
}

export function ChatTimelineIsland(props: ChatTimelineIslandProps): ReactElement {
  const { blocks } = useChatTimelineBlocks()
  return (
    <MessageTimeline
      {...props}
      edgeAlignedScroll
      blocks={blocks}
      live=""
    />
  )
}

function devPreviewBlocksFromStream(blocks: ChatBlock[], liveAssistant: string): ChatBlock[] {
  const liveText = liveAssistant.trim()
  if (!liveText) return blocks
  return [
    ...blocks,
    {
      kind: 'assistant',
      id: '__live-assistant-dev-preview',
      text: liveAssistant
    }
  ]
}

export function useDevPreviewUrls(
  extractor: (blocks: ChatBlock[]) => string[]
): string[] {
  return useChatStore(
    useShallow((s) => extractor(devPreviewBlocksFromStream(s.blocks, s.liveAssistant)))
  )
}
