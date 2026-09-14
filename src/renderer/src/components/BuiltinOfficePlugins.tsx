import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileText, Presentation, RefreshCw, Sheet } from 'lucide-react'
import type { PluginPackageView } from '../../../../packages/runtime/src/contracts/plugin-package-host'

/** State comes exclusively from the signed Core package host, including after
 * an uncertain mutation. Marketplace installation flags are unrelated. */
export function BuiltinOfficePlugins({ query }: { query: string }) {
  const { t } = useTranslation('common')
  const [packages, setPackages] = useState<PluginPackageView[]>([])
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const mounted = useRef(false)
  const inFlight = useRef(false)
  const refresh = useCallback(async () => {
    try {
      const result = await window.analytix.packageHost.request({ action: 'list' })
      if (!mounted.current) return
      setFailed(!result.ok || !('packages' in result))
      setPackages(result.ok && 'packages' in result ? result.packages : [])
    } catch {
      if (mounted.current) { setFailed(true); setPackages([]) }
    }
  }, [])
  const run = useCallback(async (operation: () => Promise<void>) => {
    if (inFlight.current) return
    inFlight.current = true
    setBusy(true)
    try { await operation() } finally {
      inFlight.current = false
      if (mounted.current) setBusy(false)
    }
  }, [])
  useEffect(() => {
    mounted.current = true
    void run(refresh)
    return () => { mounted.current = false }
  }, [refresh, run])
  const changeState = (pkg: PluginPackageView) => run(async () => {
    let mutationFailed = false
    try {
      const result = await window.analytix.packageHost.request({ action: 'setDesiredState',
        packageId: pkg.packageId, generationId: pkg.generationId,
        expectedRevision: pkg.activationRevision,
        desiredState: pkg.desiredState === 'enabled' ? 'disabled' : 'enabled' })
      mutationFailed = !result.ok
    } catch { mutationFailed = true }
    // Refresh after success or failure: persistence may have succeeded even if
    // the acknowledgement was lost. Never optimistically flip the switch.
    await refresh()
    if (mounted.current && mutationFailed) setFailed(true)
  })
  const visible = packages.filter((pkg) => pkg.displayName.toLowerCase().includes(query.trim().toLowerCase()))
  return <section className="mt-8" aria-label={t('officePluginsTitle')} aria-busy={busy}>
    <div className="flex items-center justify-between gap-3 border-b border-ds-border pb-2">
      <h2 className="text-lg text-ds-ink">{t('officePluginsTitle')}</h2>
      <button type="button" disabled={busy} onClick={() => void run(refresh)}
        aria-label={t('officePluginsRefresh')}
        className="rounded-md p-2 text-ds-muted hover:bg-ds-hover focus-visible:outline focus-visible:outline-2 disabled:opacity-50">
        <RefreshCw className={`h-4 w-4 ${busy ? 'animate-spin' : ''}`} />
      </button>
    </div>
    {failed ? <p role="status" className="py-3 text-sm text-ds-muted">{t('officePluginsUnavailable')}</p> : null}
    {!failed && !busy && packages.length === 0 ? <p className="py-3 text-sm text-ds-muted">{t('officePluginsNotConfigured')}</p> : null}
    <div className="divide-y divide-ds-border">
      {visible.map((pkg) => {
        const Icon = pkg.packageId === 'analytix-documents' ? FileText : pkg.packageId === 'analytix-spreadsheets' ? Sheet : Presentation
        const status = pkg.available ? 'officePluginsReady' : pkg.activationState === 'unset' ? 'officePluginsOff' :
          pkg.desiredState === 'disabled' ? 'officePluginsDisabled' : 'officePluginsEditorUnavailable'
        return <div key={pkg.packageId} className="flex items-center gap-3 py-4">
          <Icon className="h-6 w-6 shrink-0 text-ds-muted" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <h3 className="text-sm font-medium text-ds-ink">{pkg.displayName}</h3>
            <p className="mt-1 text-xs text-ds-muted">{t(status)}</p>
          </div>
          <button type="button" role="switch" aria-checked={pkg.desiredState === 'enabled'}
            aria-label={t('officePluginsToggle', { name: pkg.displayName })}
            disabled={busy || !pkg.materialized || pkg.activationState === 'unavailable'}
            onClick={() => void changeState(pkg)}
            className="rounded-md border border-ds-border px-3 py-1.5 text-sm text-ds-ink hover:bg-ds-hover focus-visible:outline focus-visible:outline-2 disabled:opacity-50">
            {t(pkg.desiredState === 'enabled' ? 'officePluginsDisable' : 'officePluginsEnable')}
          </button>
        </div>
      })}
    </div>
  </section>
}
