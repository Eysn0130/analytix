import type { ReactElement } from 'react'
import { ExternalLink, Globe2, Wrench } from 'lucide-react'
import type {
  CoreThreadSummarySideChatJson,
  CoreThreadSummarySourceJson
} from '../../agent/analytix-contract'
import { SummaryRow } from './SummaryRow'

export function SourceSummaryRows({
  sources
}: {
  sources: CoreThreadSummarySourceJson[]
}): ReactElement {
  if (sources.length === 0) return <div className="px-2 py-3 text-[12px] text-ds-faint">暂无来源</div>
  return (
    <>
      {sources.map((source) => (
        <SummaryRow
          key={source.id}
          icon={source.url ? <Globe2 className="h-4 w-4" /> : <Wrench className="h-4 w-4" />}
          label={source.title ?? source.label}
          detail={source.url ?? source.toolName ?? source.serverName ?? source.kind}
          actions={source.url ? (
            <button
              type="button"
              className="rounded p-1 text-ds-muted hover:bg-ds-hover hover:text-ds-ink"
              aria-label="打开来源"
              title="打开来源"
              onClick={() => void window.analytix?.app?.openExternal(source.url!)}
            >
              <ExternalLink className="h-3.5 w-3.5" />
            </button>
          ) : null}
        />
      ))}
    </>
  )
}

export function SideChatSummaryRows({
  sideChats,
  onOpenThread
}: {
  sideChats: CoreThreadSummarySideChatJson[]
  onOpenThread: (
    threadId: string,
    options?: { title?: string; source?: 'side' | 'background-agent' }
  ) => void
}): ReactElement {
  if (sideChats.length === 0) return <div className="px-2 py-3 text-[12px] text-ds-faint">暂无侧聊</div>
  return (
    <>
      {sideChats.map((side) => (
        <SummaryRow
          key={side.threadId}
          icon={<ExternalLink className="h-4 w-4" />}
          label={side.title}
          detail={`${side.turnCount ?? 0} turns`}
          trailing={side.status === 'running' ? '运行中' : undefined}
          trailingVisible={side.status === 'running'}
          interactive
          onClick={() => onOpenThread(side.threadId, { title: side.title, source: 'side' })}
        />
      ))}
    </>
  )
}
