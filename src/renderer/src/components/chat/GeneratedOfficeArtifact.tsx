import { useRef, useState } from 'react'
import { FileText, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { GeneratedArtifactMetadata } from '../../../../../packages/runtime/src/contracts/generated-artifact'
import { openGeneratedArtifact } from '../../office/open-generated-artifact'

export function GeneratedOfficeArtifact({ artifact }: { artifact: GeneratedArtifactMetadata }) {
  const { t } = useTranslation('common')
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const inFlight = useRef(false)
  const open = async () => {
    if (inFlight.current) return
    inFlight.current = true
    setBusy(true)
    setFailed(false)
    try { setFailed(!(await openGeneratedArtifact(artifact.artifactId))) }
    finally { inFlight.current = false; setBusy(false) }
  }
  return <div className="min-w-0 max-w-md">
    <button type="button" onClick={() => void open()} disabled={busy} aria-busy={busy}
      className="flex w-full items-center gap-3 rounded-lg border border-ds-border px-4 py-3 text-left hover:bg-ds-hover focus-visible:outline focus-visible:outline-2 disabled:opacity-60">
      {busy ? <Loader2 className="h-5 w-5 animate-spin motion-reduce:animate-none" aria-hidden="true" /> : <FileText className="h-5 w-5 text-blue-600 dark:text-blue-400" aria-hidden="true" />}
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-medium text-ds-ink">{t('generatedOfficeDocument')}</span>
        <span className="block text-xs text-ds-muted">DOCX · {Math.ceil(artifact.byteSize / 1024)} KB</span>
      </span>
      <span className="text-xs text-ds-muted">{t('nativeOfficeOpen')}</span>
    </button>
    {failed ? <p role="status" className="mt-1 text-xs text-ds-muted">{t('generatedOfficeUnavailable')}</p> : null}
  </div>
}
