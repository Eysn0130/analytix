import type { ReactElement } from 'react'
import { ExternalLink, FileText } from 'lucide-react'
import type { CoreThreadSummaryOutputJson } from '../../agent/analytix-contract'
import { SummaryRow } from './SummaryRow'

export function OutputSummaryRows({
  outputs
}: {
  outputs: CoreThreadSummaryOutputJson[]
}): ReactElement {
  if (outputs.length === 0) return <div className="px-2 py-3 text-[12px] text-ds-faint">暂无相关输出</div>
  return (
    <>
      {outputs.map((output) => {
        const target = output.absolutePath ?? output.localFilePath ?? output.path ?? output.url
        return (
          <SummaryRow
            key={output.id}
            icon={<FileText className="h-4 w-4" />}
            label={output.label}
            detail={output.relativePath ?? output.path ?? output.url ?? output.kind}
            actions={target ? (
              <button
                type="button"
                className="rounded p-1 text-ds-muted hover:bg-ds-hover hover:text-ds-ink"
                aria-label="打开输出"
                title="打开输出"
                onClick={() => void openOutputTarget(target, output)}
              >
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            ) : null}
          />
        )
      })}
    </>
  )
}

async function openOutputTarget(target: string, output: CoreThreadSummaryOutputJson): Promise<void> {
  if (/^https?:\/\//i.test(target)) {
    await window.analytix?.app?.openExternal(target)
    return
  }
  if (output.absolutePath || output.localFilePath || output.path) {
    await window.analytix?.workspace?.openEditorPath({
      path: target
    })
  }
}
